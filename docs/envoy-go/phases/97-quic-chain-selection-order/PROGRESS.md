# Phase 97 — QUIC chain selection order — IMPL PROGRESS

## 0. What this IMPL refuted, by execution — FOURTEEN claims, NINE of them from its own agents

Every stage's job is to refute its predecessor by execution. The phase-97 PLAN refuted SIXTEEN, three by
its own agents. ⚠️ **NINE of the fourteen below came from THIS stage's own agents, several of them
refuting the controller's own brief** — the fourth consecutive row on which a stage's review seam, not its
successor, is the source. **Expect to be refuted by your own agents.**

1. 🔴 **THE TEN ARMS WENT ALL-GREEN AT PRODUCTION EDIT 2 OF 3, LEAVING EDIT 3 WITH ZERO COVERAGE.** Arm j
   is the only arm touching `quicTLSConfig`, and its own precondition hard-requires a PLAINTEXT default
   slot — so the pre-repair body had exactly ONE TLS-bearing candidate and agreed **by single-candidacy,
   not by ordering**. Measured with an ephemeral both-TLS probe run against both bodies. ⇒ **arm k and NC
   roster row 10 were ADDED.** The committed roster would not have failed had edit 3 been skipped.
2. 🔴 **NC ROSTER ROW 6's TARGET ARM IS NOT CONSTRUCTIBLE.** The resolve assignment runs on both sides of
   the move; only its ORDER differs, and **nothing between the two candidate positions reads `rt.addr`**.
   The exploiting config cannot be written — it would have to name the OS-picked port — and the escape
   route is closed because a chain naming `destination_port: 0` means *unspecified* and SKIPS the
   dimension. **Row DELETED as vacuous. No fake arm was built.**
3. 🔴 **NC ROSTER ROW 9 REDDENS ONE ARM, NOT THE FOUR ITS ROSTER NAMES.** Three of the four call the
   accessor with a nil conn while the mutation is gated on a non-nil one, so they execute the unmutated
   path byte for byte. **Three vacuous cells**, predicted before the run and reported as run.
4. 🔴 **"`chainByName` IS NO LONGER CONSULTED AT ALL" IS FALSE** — and the controller's brief asserted it.
   What died is the map-ORDER ITERATION; the map survives as a name→chain lookup. Writing the brief's
   wording would have replaced one stale claim with a fresh false one — the exact half-repair failure the
   PLAN warned about.
5. 🔴 **THE STALE-PRECEDENCE OCCURRENCE SET IS SIX SITES, NOT FIVE**, and one of the five is misattributed:
   the site the PLAN calls *"ADR-0319 §Context"* sits inside **ADR-0318 §Consequences (b)**, below
   ADR-0318's heading and above ADR-0319's. A sixth carrier lives in ADR-0318 §Context. **A seventh** —
   `startQUIC`'s own doc calling it *"the single chain's"* config — was found by the same agent, reported
   rather than silently fixed, and corrected separately.
6. 🔴 **THE FOLD-IN ROSTER'S SECOND LINE WAS NOT A CARRIER.** `ROADMAP.md:139` already reads *"its
   \"swallowed-panic boot-hang\" mechanism is refuted too"*. **Repairing it would have "corrected" a
   sentence that was already right.** Only `:136` carried the claim; `:229` stays because it sits inside a
   sentinel window on a margin of one.
7. 🔴 **THE BYTE-UNTOUCHED / EDIT ROSTER PAIR IS NOT A PARTITION.** `BEHAVIOR_CONTRACT.md` is touched and
   sits on NEITHER roster — a roster-DOCUMENTATION gap, since `SPEC.md` §11 left the ledger question open
   and Task 21 decided it affirmatively without amending the roster.
8. 🔴 **THE PACKAGE BASELINE IS 240 / 238, NOT 239 / 237** — and the cause was identified rather than
   guessed: the difference against `master` is exactly this row's own new fixture driver package.
9. 🔴 **THE `^[0-9]{4}-` FIXTURE COUNTER READS 122, NOT 121.** It drops exactly two directories from 124.
   The inherited figure contradicted its own stated explanation.
10. 🔴 **THE PACKAGE'S THREE HTTP/3 CLIENTS SEND NO SNI AT ALL** — proven ON THE WIRE, not by grep, with a
    scratch listener recording `ClientHelloInfo.ServerName` and a two-armed run discriminating the legacy
    shape from the new one. **Both `server_names` arms would have been vacuous.** The control survives
    in-tree as a precondition of both arms. ⚠️ **The claim is true only as scoped to the HTTP/3 clients**;
    four TCP clients elsewhere in the package DO set it.
11. ⚠️ **THE EXISTING POINTER-POSTURE HELPER IS THE WRONG GATE FOR A QUIC ARM** — its counters are never
    dereferenced on that path. The QUIC accept goroutine dereferences different ones, and a new helper
    asserts those.
12. ⚠️ **THE STAMPED ALPN CONSTANT IS A NEW ASSUMPTION, NOT A REUSED INVARIANT.** A repo-wide search found
    the value in no production Go file. It is correct for this listener kind, because negotiation can only
    yield that value or fail — but **a listener configured with a different ALPN would have its
    `application_protocols` dimension evaluated against a hardcoded one.** ⚠️ **BANKED, NOT REPAIRED**: the
    chosen shape was measured against the reference and changing it here would encode an unmeasured parity
    answer.
13. ⚠️ **THE SENTENCE THE BRIEF CALLED "A 40k+ CHARACTER LINE" IS UNDER 6000.** The literal-text
    re-location method was right; the stated magnitude was not.
14. 🔴 **THE DRIVER-OWNED RECEIVER PORT RACE FIRED — TWICE, AT TWO DIFFERENT FIXTURES, IN ONE SESSION**,
    after never having fired before. See §Standing hazards below.

---

## 0.1 The receiver port race — its trigger FIRED, and it is DRIVER-WIDE

⚠️ **A LONG-BANKED HAZARD WHOSE TRIGGER HAD "STILL NOT FIRED" FIRED TWICE DURING THIS ROW.**

| when | fixture | port | effect |
|---|---|---|---|
| Task 20's first attempt | `0085-otlp-access-log-operators` | **41177** | `panic: … bind: address already in use` — **ABORTED THE BINARY** at 86 fixtures / 254s |
| Task 23's gate (a) | `0082-grpc-access-log-buffering` | **41561** | same panic — aborted at 84 fixtures, **MASKING 40**, including this row's own `0122` |

Both ports sit inside `net.ipv4.ip_local_port_range` (32768-60999). ⚠️ **IT FIRED AT TWO DIFFERENT
FIXTURES, WHICH IS EVIDENCE THE HAZARD IS DRIVER-WIDE AND NOT ONE FIXTURE'S DEFECT.** It is a HOST-PORT
bind by the Go driver process, **not** a container publish: container counts were equal before and after,
and the sibling session's containers were untouched. ⚠️ **THE FAILURE MODE IS MASKING** — the abort stops
the run, so everything after it is unmeasured, including the row's own subject.

**Neither run was re-run until green.** Gate (a) is recorded as **PASSING at Task 19** (124 PASS / 0 FAIL /
0 SKIP, 404s, foreground, `-count=1`, fixture set asserted by name in both directions) and **FAILING at
Task 23** on this pre-existing race. Both are recorded. The row's own fixture passed in the Task 19 full
run and in a separately-labelled narrow run.


## Task 1 — un-fixed-tip baseline and panic-gate liveness

Every figure below is stated beside the command that produced it. Nothing here is
quoted from the SPEC, the BRAINSTORM or the PLAN; every number was RUN.

### Worktree / tip

| What | Command | Output |
| --- | --- | --- |
| Worktree | — | `/home/esa/git/wt-97-impl` |
| Branch | `git -C /home/esa/git/wt-97-impl rev-parse --abbrev-ref HEAD` | `phase-97-impl` |
| Tip SHA | `git -C /home/esa/git/wt-97-impl rev-parse HEAD` | `5757edf19eb360c2d4cd1b1b25c356997be63734` |
| `quic.go` sha256 | `sha256sum /home/esa/git/wt-97-impl/internal/listener/quic.go` | `071cb7a536872da1605f3ef213187141b7aa19029ffd001569e4bda7845b0edb` |

Environment check — `type grep` reports **`grep is a function`** wrapping
`ugrep` (`exec -a ugrep "$_cc_bin" -G --ignore-files --hidden -I …`), i.e. it
honours `.gitignore`. Every grep figure in this document was therefore produced
with `/usr/bin/grep`.

### Selector resolution

`cd /home/esa/git/wt-97-impl && go list ./internal/listener/...`

```
github.com/pgdad/envoy-go/internal/listener
github.com/pgdad/envoy-go/internal/listener/listenerfilter
github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector
```

Three packages. The selector resolves — a later `FAIL … [setup failed]` from
this selector would therefore be a real failure, not a nonexistent-package
artifact.

### Un-fixed-tip package baseline

`cd /home/esa/git/wt-97-impl && go test ./internal/listener/... -count=1 -v > $SCRATCH/base.txt 2>&1; RC=$?`

| Figure | Command | Value |
| --- | --- | --- |
| RC | `RC=$?` immediately after the `go test` | **0** |
| RUN | `/usr/bin/grep -c '^=== RUN' $SCRATCH/base.txt \|\| true` | **226** |
| anchored FAIL | `/usr/bin/grep -cE '^(FAIL\|--- FAIL)\|^ *--- FAIL' $SCRATCH/base.txt \|\| true` | **0** |
| anchored panic gate | `/usr/bin/grep -cE '^panic:\|DATA RACE\|SIGSEGV' $SCRATCH/base.txt \|\| true` | **0** |

`RUN=226` beside `RC=0` — the green is NOT vacuous: 226 tests actually ran.

### Panic gate PROVEN LIVE — probe 1, ISOLATED: `quicChain`

Mutation: `panic("PROBE")` inserted as the FIRST statement of
`func (rt *listenerRuntime) quicChain() *chainInfo` in
`internal/listener/quic.go`, and **nothing else** changed —
`git diff --numstat` read exactly `1	0	internal/listener/quic.go`.

`cd /home/esa/git/wt-97-impl && go test ./internal/listener/ -count=1 -run 'TestQUICListener' -v > $SCRATCH/probe_quicchain.txt 2>&1`

- RC = **1**
- anchored panic gate — `/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV' $SCRATCH/probe_quicchain.txt || true` = **1** (>= 1 as required)
- anchored FAIL count = **2**
- the stack trace NAMES `quicChain`:

```
=== RUN   TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives
panic: PROBE

goroutine 82 [running]:
github.com/pgdad/envoy-go/internal/listener.(*listenerRuntime).quicChain(...)
	/home/esa/git/wt-97-impl/internal/listener/quic.go:75
github.com/pgdad/envoy-go/internal/listener.(*listenerRuntime).serveQUICConnection(0x4607b0?, {0xe5fa20?, 0x13749f2c9888?}, 0x13749f190fb8?)
	/home/esa/git/wt-97-impl/internal/listener/quic.go:124 +0x55
created by github.com/pgdad/envoy-go/internal/listener.(*listenerRuntime).quicAcceptLoop in goroutine 70
	/home/esa/git/wt-97-impl/internal/listener/quic.go:105 +0x59
FAIL	github.com/pgdad/envoy-go/internal/listener	0.010s
FAIL
```

The panic fires on a **background goroutine** created by `quicAcceptLoop`, so it
is unrecovered and aborts the whole test binary — there is no `--- FAIL:` line
for the test itself, only the package-level `FAIL`. The anchored gate still
reads it.

Revert: `git -C /home/esa/git/wt-97-impl checkout -- internal/listener/quic.go`,
then `sha256sum -c quic.go.sha256` printed **verbatim**:

```
/home/esa/git/wt-97-impl/internal/listener/quic.go: OK
```

### Panic gate PROVEN LIVE — probe 2, SEPARATE RUN: `quicTLSConfig`

Run separately from probe 1 — a fail-fast control aborts on the FIRST panic and
says nothing about a second, so the two sites must never share a run.

Mutation: `panic("PROBE")` as the FIRST statement of
`func (rt *listenerRuntime) quicTLSConfig() *stdtls.Config`, nothing else —
`git diff --numstat` again read exactly `1	0	internal/listener/quic.go`.

`cd /home/esa/git/wt-97-impl && go test ./internal/listener/ -count=1 -run 'TestQUICListener' -v > $SCRATCH/probe_quictlsconfig.txt 2>&1`

- RC = **1**
- anchored panic gate = **1** (>= 1 as required)
- anchored FAIL count = **3**
- the stack trace NAMES `quicTLSConfig`:

```
=== RUN   TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives
--- FAIL: TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives (0.00s)
panic: PROBE [recovered, repanicked]
...
github.com/pgdad/envoy-go/internal/listener.(*listenerRuntime).quicTLSConfig(...)
	/home/esa/git/wt-97-impl/internal/listener/quic.go:57
github.com/pgdad/envoy-go/internal/listener.(*listenerRuntime).startQUIC(0x0?, {0x0?, 0x0?}, 0x0?)
	/home/esa/git/wt-97-impl/internal/listener/quic.go:32 +0x76
github.com/pgdad/envoy-go/internal/listener.(*Manager).Start(0x9ce4811dc40, {0x1334a18, 0x9ce4806c0e0})
	/home/esa/git/wt-97-impl/internal/listener/manager.go:1166 +0x1ae
github.com/pgdad/envoy-go/internal/listener.startQUICHCMListener(0x9ce47e9f448, {0x1334a18, 0x9ce4806c0e0})
	/home/esa/git/wt-97-impl/internal/listener/quic_negative_test.go:48 +0x225
github.com/pgdad/envoy-go/internal/listener.TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives(0x9ce47e9f448)
```

Here the panic fires on the **test goroutine**, so `testing` recovers and
repanics — there IS a `--- FAIL:` line, which is why the anchored FAIL count is
3 rather than 2.

Revert + `sha256sum -c quic.go.sha256`, **verbatim**:

```
/home/esa/git/wt-97-impl/internal/listener/quic.go: OK
```

### Accessor reachability — the PLAN's claim VERIFIED

`/usr/bin/grep -rn 'quicTLSConfig\|quicChain' /home/esa/git/wt-97-impl/internal/listener/`

`quicTLSConfig` — **direct test call sites CONFIRMED at exactly the two lines the
PLAN named**:

- `internal/listener/manager_test.go:1042` — `cfg := rt.quicTLSConfig()`
- `internal/listener/manager_test.go:1075` — `cfg := rt.quicTLSConfig()`

plus non-call references (comments at `manager_test.go:977`, `:1010`,
`quic_test.go:237`, `manager.go:396`, `:740`; message text at
`manager_test.go:1044`, `:1046`, `:1077`) and the production call sites
`quic.go:32` and `quic.go:144`.

`quicChain` — **ZERO test call sites CONFIRMED**. Every hit is production or
comment:

- `quic.go:68` — doc comment
- `quic.go:74` — the definition
- `quic.go:123` — the ONLY call site, inside `serveQUICConnection`

No `_test.go` file in `internal/listener/` mentions `quicChain` at all. It is
reachable only through the DRIVEN H3 tests, exactly as the PLAN stated. Probe 1
independently corroborates this: the trace reached `quicChain` from
`serveQUICConnection` on an accept-loop goroutine, never from a test frame.

**Both PLAN claims are CONFIRMED. Nothing in this task refutes them.**

#### Qualifications on what these two probes actually measured

Stated so a later task does not over-read them:

1. Both probes aborted inside the SAME single test,
   `TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives` — the first of the
   five tests matched by `-run 'TestQUICListener'`
   (`/usr/bin/grep -rn '^func TestQUIC' internal/listener/` lists
   `TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives`,
   `TestQUICListener_GarbageDatagrams_ListenerSurvives`,
   `TestQUICListener_HandshakeALPNh3`, `TestQUICListener_ServesH3GET`,
   `TestQUICListener_RegistersSSLNamesAtZero`, plus the non-matching
   `TestQUICGoModuleWired`). The fail-fast abort means each probe measured ONE
   site in ONE test; it proves the gate is LIVE, and it does NOT enumerate every
   test that reaches either accessor.
2. Probe 1 shows that `TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives`
   — despite the `_Refused` in its name — does drive a connection all the way
   into `serveQUICConnection`, and hence into `quicChain`. Worth remembering
   before assuming that test exercises only the rejection path.

### End-of-task cleanliness

`git -C /home/esa/git/wt-97-impl status --porcelain --untracked-files=all` printed
**empty output** after both reverts. No `.go` change survives Task 1; the only
file this task adds is this PROGRESS.md.

## Task 2 — chain-selection fixture builder + arm (a), RED at the un-fixed tip

Task 2 lands at the **un-fixed tip**. No production file was touched; the
divergence is captured as a failing test, not repaired.

### Builder signature and placement

```go
func mkQUICListenerChains(t *testing.T, fcm *listenerv3.FilterChainMatch, body string,
	withDefault bool, defaultTLS bool, defaultBody string) *listenerv3.Listener
```

placed in `internal/listener/manager_test.go` immediately after
`mkQUICListenerDefaultChain`, together with its HCM helper

```go
func mkHCMFilterQUICChain(t *testing.T, statPrefix, body string) *listenerv3.Filter
```

placed immediately after `mkHCMFilterWithCodec`. `mkHCMFilterWithCodec` was NOT
mutated — it hard-wires `stat_prefix: "ingress_http"` and a `direct_response`
200 `"OK\n"`, and other tests in the package depend on those exact values.

The builder parents on the **`mkQUICListenerHCM`** shape (a real HCM terminal),
not on `mkQUICListener`: the latter's terminal is a `tcp_proxy`, which serves no
H3 at all — `serveQUICConnection` logs `chain terminal is not H3-capable` and
closes — so an arm built on it could never observe which chain served.

`direct_response` status is **222**, a non-1xx sentinel (a `111` was reported to
come back as `200` on the H3 path; `222` comes back as `222`). Chain identity is
carried by the **body** (`body` vs `defaultBody`), not by the status.

### The stat_prefix boot-panic hazard (documented in the builder)

Two HCMs sharing one `stat_prefix` across two chains of ONE listener register
`http.<prefix>.downstream_rq_total` twice and the stats registry panics at boot.
There is no `recover()` anywhere in non-test `internal/listener`, so such a panic
**aborts the test binary** rather than failing a test. The builder therefore
assigns `quic_fc0` to `filter_chains[0]` and `quic_dfc` to the default slot, and
its doc comment says so.

### The two asymmetries the builder CONSUMES (doc-commented, not repaired)

1. A `filter_chains[i]` with no `transport_socket` **boot-rejects** on a QUIC
   listener — `manager.go:660` returns
   `quic listener requires a transport_socket (mandatory TLS)`. The
   `default_filter_chain` slot (`manager.go:730-758` — the `if ts :=
   dfc.GetTransportSocket(); ts != nil` branch through `dfcTLS = dc.TLSConfig` at
   `manager.go:757`) carries **no such kind check**: its `transport_socket` branch is simply skipped and `dfcTLS` stays
   nil, and the listener builds. So `defaultTLS == false` is expressible ONLY in
   the default slot; `filter_chains[0]` here is always QUIC-TLS-wrapped.
2. That asymmetry is the banked **D2-QUICTS** divergence. This builder does not
   fix it — it USES it as a fixture. Any repair of D2-QUICTS must revisit the
   `defaultTLS=false` callers.

Also verified while writing the helper: `internal/filter/hcm/config.go:242-249`
gates `codec_type HTTP3` **solely** on `lc.IsQUIC`, not on `HasTLS`, so the same
HTTP3 HCM filter builds in the `transport_socket`-less default slot.

### Placement decision for arm (a)

Arm (a) is `TestQUICChainSelection_IndexedChainWinsOverDefaultSlot` in
`internal/listener/quic_test.go`. Reason: its subject is `quic.go`'s
`quicChain()` accessor, which is where `quic_test.go`'s other subjects live.
VERIFIED (not assumed) that this works: `head -1` of both
`internal/listener/quic_test.go` and `internal/listener/manager_test.go` reads
`package listener`, so every `mk*` builder in `manager_test.go` is in scope.
House style honoured: one straight-line `func Test…` per arm, **no table** —
both QUIC test files are table-free.

Assertion shape: an **exact pointer equality** of `rt.quicChain()` against
`rt.chainByName["quic_listener_chains/filter_chains[0]"]`, not a non-nil floor.
`chainInfo` has exactly three fields (`serverNames`, `tlsCfg`,
`netChainFactory`) and **no `Name`**, so the map key is the only binding between
a `ChainSpec` and its `*chainInfo` — identity is the only thing that can name
the selected chain. Two anti-vacuity **preconditions** (`t.Fatalf`) guard it:
both map keys must be present, and the two `*chainInfo` must be DISTINCT
pointers (equal pointers would make the equality vacuous). Both properties use
`t.Errorf`, each naming its own property, so neither can be collapsed onto the
other's failure line.

### Arm (a) — RED at the un-fixed tip

`cd /home/esa/git/wt-97-impl && go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection_IndexedChainWinsOverDefaultSlot' -v`

| Figure | Value |
| --- | --- |
| RC | **1** |
| `=== RUN` count | **1** (the `-run` selector MATCHED — a non-matching selector prints `[no tests to run]` and exits 0) |
| anchored panic gate `/usr/bin/grep -cE '^panic:\|DATA RACE\|SIGSEGV'` | **0** |

```
=== RUN   TestQUICChainSelection_IndexedChainWinsOverDefaultSlot
    quic_test.go:383: selection identity: quicChain() = 0x17c5d97a4330, want filter_chains[0] = 0x17c5d973bd10 (an empty-match indexed chain is eligible for every connection and must be selected)
    quic_test.go:391: last-resort ordering: quicChain() returned the default_filter_chain 0x17c5d97a4330, but default_filter_chain is the LAST-RESORT slot and must not pre-empt the eligible filter_chains[0] 0x17c5d973bd10
--- FAIL: TestQUICChainSelection_IndexedChainWinsOverDefaultSlot (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	0.007s
FAIL
```

The failure is exactly the predicted one: property 2 prints the returned pointer
`0x17c5d97a4330` and the `default_filter_chain` pointer as the **same value** —
`quicChain()` returned the DEFAULT chain instead of the indexed one, which is
`quic.go:75-77`'s `if rt.defaultChain != nil { return rt.defaultChain }`.

### NC of the arm itself — the assertion is LIVE, not never-reached

The two assertions were temporarily INVERTED (assert that the DEFAULT chain
wins). `diff -u` against the pre-inversion copy reported **8** changed lines
(4 removed + 4 added = the two `if` lines and the two `t.Errorf` lines); nothing
else in the file changed, and no production file was touched.

`go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection_IndexedChainWinsOverDefaultSlot' -v`

| Figure | Value |
| --- | --- |
| RC | **0** |
| `=== RUN` count | **1** |
| anchored panic gate | **0** |

```
=== RUN   TestQUICChainSelection_IndexedChainWinsOverDefaultSlot
--- PASS: TestQUICChainSelection_IndexedChainWinsOverDefaultSlot (0.00s)
PASS
ok  	github.com/pgdad/envoy-go/internal/listener	0.006s
```

The inversion goes GREEN, so the arm is executable, reached, and its assertion is
live — the RED is a real refutation, not a never-executed function.

### Inversion reverted — RED restored

The file was restored from the pre-inversion copy; `diff -q` reported the files
IDENTICAL.

| Figure | Value |
| --- | --- |
| RC | **1** |
| `=== RUN` count | **1** |
| anchored panic gate | **0** |

```
=== RUN   TestQUICChainSelection_IndexedChainWinsOverDefaultSlot
    quic_test.go:383: selection identity: quicChain() = 0x2fb8692c07e0, want filter_chains[0] = 0x2fb8692b1bc0 (an empty-match indexed chain is eligible for every connection and must be selected)
    quic_test.go:391: last-resort ordering: quicChain() returned the default_filter_chain 0x2fb8692c07e0, but default_filter_chain is the LAST-RESORT slot and must not pre-empt the eligible filter_chains[0] 0x2fb8692b1bc0
--- FAIL: TestQUICChainSelection_IndexedChainWinsOverDefaultSlot (0.00s)
FAIL
FAIL	github.com/pgdad/envoy-go/internal/listener	0.005s
FAIL
```

(The pointer values differ between the two RED runs — they are heap addresses
from separate processes. What is load-bearing and IDENTICAL across both runs is
that the returned pointer EQUALS the `default_filter_chain` pointer and DIFFERS
from the `filter_chains[0]` pointer.)

### Full-package run — exactly one new failure

`go test ./internal/listener/... -count=1 -v`

| Figure | Command | Value |
| --- | --- | --- |
| RC | `$?` | **1** |
| RUN | `/usr/bin/grep -c '^=== RUN'` | **227** (Task 1 baseline was 226; +1 = arm (a)) |
| anchored panic gate | `/usr/bin/grep -cE '^panic:\|DATA RACE\|SIGSEGV'` | **0** |
| anchored FAIL lines | `/usr/bin/grep -E '^(FAIL\|--- FAIL)\|^ *--- FAIL'` | only `--- FAIL: TestQUICChainSelection_IndexedChainWinsOverDefaultSlot`, plus the package-level `FAIL` lines |

The panic gate is **0** across every run in this task — the `stat_prefix`
collision was avoided, not worked around.

### Hygiene

| Gate | Command | Result |
| --- | --- | --- |
| gofmt | `gofmt -l /home/esa/git/wt-97-impl/internal/listener/` | **empty output** |
| vet | `go vet ./internal/listener/...` | RC **0** |
| golangci-lint | `golangci-lint run ./internal/listener/...` | RC **0**, no output |

### Production code byte-unchanged

Digests captured BEFORE any edit, verified after:

`sha256sum -c $SCRATCH/baseline.sha256`

```
internal/listener/quic.go: OK
internal/listener/manager.go: OK
```

RC **0**. `git status --porcelain -- internal/listener/quic.go internal/listener/manager.go`
printed **empty output**.

### Contradictions to the brief

None. Every claim in the brief that this task relied on was re-verified and held:
the `filter_chains[%d]` / `default_filter_chain` spec-name spellings
(`manager.go:618`, `manager.go:777`), `chainByName` containing the default slot
(`manager.go:779`), `chainInfo`'s three fields and absent `Name`, `quicChain()`'s
nullary `defaultChain`-first body (`quic.go:74-81`), the QUIC mandatory-TLS
reject present in the indexed loop and absent from the default slot, and both
QUIC test files being `package listener` and table-free.

## Task 3 — arms (b) and (c): the destination_port eligibility pair

Two arms appended to `internal/listener/quic_test.go`, both against the
**un-fixed tip**. Production code untouched.

| Arm | Test | Shape | Predicted | **ACTUAL** |
| --- | --- | --- | --- | --- |
| b | `TestQUICChainSelection_IneligibleIndexedChainFallsBackToDefaultSlot` | `filter_chains[0]` with `destination_port: 65000` + QUIC-TLS default slot | 🟢 GREEN | 🟢 **GREEN** |
| c | `TestQUICChainSelection_IneligibleIndexedChainNoDefaultSlotSelectsNothing` | same ineligible chain, **NO** default slot | 🔴 RED | 🔴 **RED** |

Both matched the prediction. No contradiction.

### Firing assertions on the RED arm (c)

Both properties fired — the failure is not a single divergence masking a later one:

```
quic_test.go:624: no-match selects nothing: quicChain() = 0x115a129a870, want nil (filter_chains[0] names destination_port 65000 against listener address "127.0.0.1:0" and there is no default_filter_chain, so NO chain is selectable)
quic_test.go:633: ineligible chain is not the fallback: quicChain() returned filter_chains[0] 0x115a129a870, whose filter_chain_match names destination_port 65000 — with no default_filter_chain the absence of an eligible chain must yield nil, which is what makes serveQUICConnection close the connection
```

### Why arm (b) is green at the un-fixed tip, and why that is NOT coverage

`quicChain()` is nullary and consults nothing (`quic.go:74-81`): with a default
slot present it returns `rt.defaultChain` unconditionally. Arm (b)'s correct
answer *is* the default slot, so the subject agrees by coincidence — it
evaluates no dimension of any `filter_chain_match`. Recorded in the arm's own
doc comment so a reader who sees it pass before the fix cannot mistake that for
enforcement of the `destination_port` dimension.

**The pair is the gate.** Arm (a) alone is satisfiable by "always return
`filter_chains[0]`"; arm (b) alone by "always return the default slot" — which
is literally the tip's behavior. The two listeners differ in ONE field
(`filter_chains[0]`'s `filter_chain_match`: absent vs `destination_port:
65000`); no constant answer is right on both. Arms (b)/(c) in turn differ only
in `withDefault`, forcing the selector to distinguish ELIGIBILITY from
PRESENCE.

### `withDefault=false` / `defaultTLS=false` — first execution

Task 2 noted these builder branches were unexercised. Arm (c) is the first
caller. **No surprise**: `NewManager` succeeded, `rt.defaultChain == nil`,
`len(rt.chainByName) == 1`, and the `default_filter_chain` key was absent. All
three are asserted as preconditions in the arm.

### Port-question verdict (roster row 6) — **NO SUCH ARM IS CONSTRUCTIBLE**

Row 6 wants to neutralise moving `rt.addr = udpConn.LocalAddr().String()` to
*above* the `quicTLSConfig()` call inside `startQUIC`, and wants "the port-0
variant of arm (b)" to redden. **That arm cannot exist.** Two independent
mechanisms, both re-read at this tip rather than assumed:

1. **The move has no observation window.** The assignment executes on both
   sides of the move; only its ORDER within `startQUIC` differs. Nothing
   between the two candidate positions reads `rt.addr` — the span is
   `quic.Listen`, `rt.udpConn = udpConn`, `rt.quicCloser = ql`. `quicTLSConfig()`
   consults only `rt.defaultChain` / `rt.chainByName` (`quic.go:56-66`), never
   `rt.addr`. `registerListenerMetrics`, the sole `rt.addr` reader in
   `startQUIC` (via `normalizeAddr`, `manager.go:408`), already sits after both
   positions. And no chain selection runs while `startQUIC` runs:
   `serveQUICConnection` is reached only from the accept goroutine, launched
   last.

2. **The exploiting config cannot be written.** Even given a window, reddening
   needs a chain whose `destination_port` equals the OS-picked resolved port —
   a value that does not exist until the bind, and so cannot appear in a
   listener config `NewManager` consumed before `Start`. The complement is
   closed too: `destination_port: 0` means *unspecified* and SKIPS the
   dimension entirely (`chainmatch.go:119`), so no chain can spell "the port is
   still 0". Pinning a fixed non-zero port instead of 0 removes the difference
   altogether — `ResolveUDPAddr` and `LocalAddr()` then agree on the port, and
   the move is a no-op by construction.

**A roster row whose target arm does not exist is a vacuous control.** No fake
arm was built to satisfy it.

**What row 6 CAN still falsify, restated as a DELETION** (never assigning the
resolved address at all, rather than moving the assignment): the two existing
live-handshake arms `TestQUICListener_HandshakeSucceeds` and
`TestQUICListener_RegistersSSLNamesAtZero` both take their dial target from
`Listeners()[0].Addr`, which is `rt.addr`. A deleted resolve makes them dial
`127.0.0.1:0` and fail at `quic.DialAddr`. No new arm is needed for that, and
none was added.

### Scope caveat recorded in the arms

`mkQUICListenerChains` configures `port_value: 0` and `manager.go:837` builds
`rt.addr` from the CONFIGURED address+port, so a never-Started runtime carries
`rt.addr == "127.0.0.1:0"` — asserted as a precondition in both arms rather
than assumed. Consequently these arms prove *"a `destination_port` unequal to
the listener's port makes the chain ineligible"*, **not** *"the selector reads
the RESOLVED port"*. The second claim needs a Started listener and is not
constructible from config, per the verdict above.

### Gates — Task 3

| Gate | Command | Result |
| --- | --- | --- |
| new arms | `go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection_' -v` | RC **1**, 3 `=== RUN`, arms (a) and (c) RED, arm (b) GREEN |
| full package | `go test ./internal/listener/... -count=1 -v` | RC **1** |
| `=== RUN` count | full package | **229** |
| anchored FAIL | `^(FAIL\|--- FAIL)\|^ *--- FAIL` | **5** lines = 2 `--- FAIL` (arms a, c) + 3 package/summary lines |
| panic gate | `^panic:\|DATA RACE\|SIGSEGV` | **0** |
| gofmt | `gofmt -l /home/esa/git/wt-97-impl/internal/listener/` | **empty output** |
| vet | `go vet ./internal/listener/...` | RC **0** |
| golangci-lint | `golangci-lint run ./internal/listener/...` | RC **0**, no output |

Sibling packages green: `listenerfilter` **ok**, `listenerfilter/tls_inspector` **ok**.

### Production code byte-unchanged — Task 3

```
/home/esa/git/wt-97-impl/internal/listener/quic.go: OK
/home/esa/git/wt-97-impl/internal/listener/manager.go: OK
```

RC **0**.

## Task 4 — arms (d) and (e): the transport_protocol pair

| Arm | Test | Shape | Predicted | **ACTUAL** |
| --- | --- | --- | --- | --- |
| d | `TestQUICChainSelection_TransportProtocolQUICMatches` | `filter_chains[0]` `transport_protocol: "quic"` + QUIC-TLS default slot | 🔴 RED | 🔴 **RED** |
| e | `TestQUICChainSelection_TransportProtocolTLSDoesNotMatch` | byte-identical but `"tls"` | 🟢 GREEN | 🟢 **GREEN** |

Both matched the prediction. No contradiction.

### Firing assertions on the RED arm (d)

Both properties fired:

```
quic_test.go:723: transport_protocol match selects indexed: quicChain() = 0x259434690330, want filter_chains[0] = 0x259434627d10 (a QUIC connection's transport protocol is "quic", which this chain names, so the chain is eligible)
quic_test.go:730: eligible chain pre-empts last resort: quicChain() returned the default_filter_chain 0x259434690330, but filter_chains[0] 0x259434627d10 names transport_protocol "quic" and is eligible, so the last-resort slot must not be consulted
```

### Parse confirmation — BOTH values, verified by BUILDING

`parseChainSpec`'s enum gate is `case "", "tls", "raw_buffer", "quic":`
(`manager.go:985-991`). Rather than read the switch, **both listeners were
built**: each arm's `NewManager` call is guarded by a `t.Fatalf` that names a
boot-reject explicitly, and **neither fired**. So neither arm is a rejection
pin; both are genuine runtime arms.

Each arm additionally asserts, as a precondition, that its configured string
SURVIVED into `rt.chainSpecs[...].TransportProtocol` (`"quic"` for (d), `"tls"`
for (e)) — via the new `quicChainSpecByName` helper. A silently-dropped value
would leave `""`, which `chainmatch.go:128` SKIPS, making the chain
universally eligible and both arms green for the wrong reason. Neither
precondition fired.

### Why arm (e) is green at the un-fixed tip, and why that is NOT coverage

Same mechanism as arm (b): `quicChain()` is nullary and returns
`rt.defaultChain` unconditionally (`quic.go:74-81`). Arm (e)'s correct answer
*is* the default slot, so the subject agrees by coincidence, evaluating no
dimension. Recorded in the arm's doc comment.

### Arm (d) alone is not the gate

Arm (d) alone cannot distinguish "the constant was stamped and matched" from
"the dimension is unenforced": a selector that DROPS the `transport_protocol`
comparison (or a `ChainSpec` whose value never parsed through) makes every
chain eligible on this dimension, and `"quic"` was the right answer anyway.
Arm (e) — one string apart — is the half that catches it, since it is correct
only if a non-matching value actually excludes the chain. Conversely arm (e)
alone is green under "never select an indexed chain", which is the tip. Stated
in both doc comments.

### The asymmetry, and why the coming fix must stamp the constant

`matches` compares by exact, case-sensitive `!=` (`chainmatch.go:128`). The
CHAIN's empty string means "unspecified" and skips the dimension — but there is
**no reverse wildcard**: an empty `inputs.TransportProtocol` is just a value
that nothing equals. A chain spelling `"quic"` against an unset input is
therefore **INELIGIBLE**, not universally eligible. The QUIC selector cannot
leave the input blank and rely on a wildcard; it must stamp the literal
constant `"quic"`, or every `transport_protocol: "quic"` chain on every QUIC
listener silently falls through to the default slot — the exact shape of the
bug this phase pins. Recorded in arm (d)'s doc comment.

### Gates — Task 4

| Gate | Command | Result |
| --- | --- | --- |
| new arms | `go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection_TransportProtocol' -v` | RC **1**, 2 `=== RUN`, (d) RED, (e) GREEN |
| full package | `go test ./internal/listener/... -count=1 -v` | RC **1** |
| `=== RUN` count | full package | **231** |
| anchored FAIL | `^(FAIL\|--- FAIL)\|^ *--- FAIL` | **6** lines = 3 `--- FAIL` (arms a, c, d) + 3 package/summary lines |
| panic gate | `^panic:\|DATA RACE\|SIGSEGV` | **0** |
| gofmt | `gofmt -l /home/esa/git/wt-97-impl/internal/listener/` | **empty output** |
| vet | `go vet ./internal/listener/...` | RC **0** |
| golangci-lint | `golangci-lint run ./internal/listener/...` | RC **0**, no output |

Sibling packages green: `listenerfilter` **ok**, `listenerfilter/tls_inspector` **ok**.

### Production code byte-unchanged — Task 4

```
/home/esa/git/wt-97-impl/internal/listener/quic.go: OK
/home/esa/git/wt-97-impl/internal/listener/manager.go: OK
```

RC **0**.

## Task 5 — arms (f) and (g): the application_protocols pair

| Arm | Test | Shape | Predicted | **ACTUAL** |
| --- | --- | --- | --- | --- |
| f | `TestQUICChainSelection_ApplicationProtocolsH3Matches` | `filter_chains[0]` `application_protocols: ["h3"]` + QUIC-TLS default slot | 🔴 RED | 🔴 **RED** |
| g | `TestQUICChainSelection_ApplicationProtocolsH2DoesNotMatch` | byte-identical but `["h2"]` | 🟢 GREEN | 🟢 **GREEN** |

Both matched the prediction. No contradiction.

### Firing assertions on the RED arm (f)

Both properties fired:

```
quic_test.go:918: ALPN match selects indexed: quicChain() = 0x1937e2adc330, want filter_chains[0] = 0x1937e2a73d10 (a connection reaching the QUIC serve path negotiated "h3", which this chain names, so the chain is eligible)
quic_test.go:925: eligible chain pre-empts last resort: quicChain() returned the default_filter_chain 0x1937e2adc330, but filter_chains[0] 0x1937e2a73d10 names application_protocols ["h3"] and is eligible, so the last-resort slot must not be consulted
```

### On QUIC this input is a listener CONSTANT, not the client's offer list

Recorded in both arms' doc comments. On **TCP**,
`ChainMatchInputs.ApplicationProtocols` is populated by the `tls_inspector`
listener filter from the ClientHello's ALPN extension — the client's full
OFFER LIST, several entries wide — and `alpnMatchAny`
(`chainmatch.go:293-302`) is a genuine any-of intersection over it.

On **QUIC** there is no ClientHello inspection at all: the handshake is already
complete when quic-go's `Accept` returns, and the only ALPN fact available is
`crypto/tls.ConnectionState.NegotiatedProtocol` — a **SCALAR**, the single
negotiated value. `mkQUICListenerChains` hard-wires the QUIC config's
`NextProtos` to exactly `["h3"]`, so the negotiation has exactly one possible
outcome and every connection reaching `serveQUICConnection` negotiated `"h3"`
**by construction**. These arms therefore agree with the pinned reference **by
a different mechanism than the TCP path's**: a one-element input derived from a
listener constant, not an intersection against a client-supplied set. Arm
(g)'s `"h2"` is unreachable structurally, not because a client declined to
offer it.

### ⚠️ A future row adding a second QUIC ALPN would break that constant silently

The moment `NextProtos` becomes e.g. `["h3", "h3-29"]`, the negotiated value is
no longer determined by the listener config; arm (f) keeps passing while no
longer testing what its name says (`"h3"` becomes ONE possible outcome rather
than THE outcome), and arm (g) flips from "structurally impossible" to
"happens not to occur". **Nothing in the selection assertions fails when that
happens.** Stated in both doc comments, and armed as an executable tripwire:
each arm asserts `default_filter_chain tlsCfg.NextProtos == ["h3"]` exactly, as
a `t.Fatalf` precondition. Any row that widens the QUIC ALPN list fails there
instead of silently passing.

Each arm also asserts its configured list survived into
`rt.chainSpecs[...].ApplicationProtocols` — an empty list would SKIP the
dimension (`chainmatch.go:131`) and make both arms green for the wrong reason.
No precondition fired.

### Why arm (g) is green at the un-fixed tip, and why that is NOT coverage

Same mechanism as arms (b) and (e): `quicChain()` is nullary and returns
`rt.defaultChain` unconditionally (`quic.go:74-81`). Arm (g)'s correct answer
*is* the default slot, so the subject agrees by coincidence. Recorded in the
arm's doc comment. **What the (f)/(g) pair rules out:** (f) alone is green
under a selector that never compares `application_protocols` at all; (g) alone
is green under a selector that never selects an indexed chain — which is the
tip. Only the pair, one ALPN string apart, excludes both.

### Gates — Task 5

| Gate | Command | Result |
| --- | --- | --- |
| new arms | `go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection_ApplicationProtocols' -v` | RC **1**, 2 `=== RUN`, (f) RED, (g) GREEN |
| full package | `go test ./internal/listener/... -count=1 -v` | RC **1** |
| `=== RUN` count | full package | **233** |
| anchored FAIL | `^(FAIL\|--- FAIL)\|^ *--- FAIL` | **7** lines = 4 `--- FAIL` (arms a, c, d, f) + 3 package/summary lines |
| panic gate | `^panic:\|DATA RACE\|SIGSEGV` | **0** |
| gofmt | `gofmt -l /home/esa/git/wt-97-impl/internal/listener/` | **empty output** |
| vet | `go vet ./internal/listener/...` | RC **0** |
| golangci-lint | `golangci-lint run ./internal/listener/...` | RC **0**, no output |

Sibling packages green: `listenerfilter` **ok**, `listenerfilter/tls_inspector` **ok**.

### Production code byte-unchanged — Task 5 (final)

```
/home/esa/git/wt-97-impl/internal/listener/quic.go: OK
/home/esa/git/wt-97-impl/internal/listener/manager.go: OK
```

RC **0**. `git status --porcelain -- internal/listener/quic.go internal/listener/manager.go`
printed **empty output**.

### Tasks 3-5 roll-up: predicted vs ACTUAL

| Arm | Predicted | ACTUAL | Match |
| --- | --- | --- | --- |
| b — ineligible `destination_port` + default slot | 🟢 GREEN | 🟢 GREEN | ✅ |
| c — ineligible `destination_port`, no default slot | 🔴 RED | 🔴 RED | ✅ |
| d — `transport_protocol: "quic"` | 🔴 RED | 🔴 RED | ✅ |
| e — `transport_protocol: "tls"` | 🟢 GREEN | 🟢 GREEN | ✅ |
| f — `application_protocols: ["h3"]` | 🔴 RED | 🔴 RED | ✅ |
| g — `application_protocols: ["h2"]` | 🟢 GREEN | 🟢 GREEN | ✅ |

Six for six. **No contradiction of the brief was found in Tasks 3-5** — with
the single exception of roster row 6, whose target arm is proven not to exist
(see the Task 3 port-question verdict). Every RED arm fired BOTH of its
properties, so in no case did a first divergence mask a later one.

## Task 6 — arms (h) and (i): the DRIVEN `server_names` pair

Arms (a)-(g) are nil-conn arms: they call `quicChain()` on a runtime that was
never Started. Arms (h) and (i) **cannot be**. `ServerName` exists only on a
real ClientHello, and there is no production path that hands `quicChain()` an
SNI without a connection — a synthesized one would be an INVENTED INPUT,
measuring a signature this phase has not landed rather than a behavior the
listener has. Both arms therefore `mgr.Start(ctx)`, dial real HTTP/3 against the
bound UDP address, and read WHICH CHAIN SERVED out of the response BODY.

### 🔴 The SNI-absence finding, verified by EXECUTION

The brief predicted that no HTTP/3 client in this package sets `ServerName`.
**Confirmed, and then confirmed a second time on the wire.**

Static check — `/usr/bin/grep -rni 'servername' internal/listener/` returns
`ServerName` hits in `tls_handshake_negative_test.go` (all four are **TCP** TLS
clients, not H3) and in `manager_test.go`, and **zero** hits in any of the three
HTTP/3 client sites:

| H3 client site | client `*stdtls.Config` | `ServerName` |
| --- | --- | --- |
| `driveH3` (`quic_test.go:97`) | `{NextProtos: ["h3"], InsecureSkipVerify: true}` | **absent** |
| inline block in `TestQUICListener_ServesH3GET` | same literal | **absent** |
| `h3GetHealth` (`quic_negative_test.go:61`) | same literal | **absent** |

All three dial a `127.0.0.1:<port>` literal, and `crypto/tls` omits the
`server_name` extension entirely for an IP literal (RFC 6066 §3).

**A static grep is not a wire observation**, so a throwaway quic-go listener
whose `*stdtls.Config.GetConfigForClient` records
`ClientHelloInfo.ServerName` was stood up in a scratch test and dialed twice
from the same process:

```
ARM1 legacy-shape ClientHello ServerName = ""
ARM2 mkH3ClientTLS   ClientHello ServerName = "alpha.envoy-go.test"
```

ARM1 is the package's existing three-site client literal; it sends **NO SNI AT
ALL**. ARM2 is the new `mkH3ClientTLS`. That two-armed run is also the
**discrimination proof** for the control itself: the observer distinguishes `""`
from a real name, so a green from it is not a green from a blind probe. The
scratch file was deleted before commit; the control survives in-tree as
`assertH3ClientTransmitsSNI`, which arms (h) and (i) each run as a
PRECONDITION before the subject listener is even built.

Without it, arm (h)'s red and arm (i)'s green are BOTH explainable by a client
that transmitted nothing — the pair would be vacuous in both directions.

### `testAlphaCertPEM`, verified not assumed

`openssl x509 -noout -subject -ext subjectAltName -dates`:

```
subject=CN = alpha.envoy-go.test
X509v3 Subject Alternative Name:
    DNS:alpha.envoy-go.test
notBefore=Jan  1 00:00:00 2026 GMT
notAfter=Jan  1 00:00:00 2046 GMT
```

Exactly ONE SAN. Arm (i) dials `other.envoy-go.test`, which that certificate does
not carry — which is precisely why `InsecureSkipVerify` is **mandatory**, not
convenient: a verifying client would abort at CERTIFICATE VALIDATION, a
handshake outcome, and arm (i) would report a green that never reached chain
selection at all. Arm (i) must fail the MATCH, not the HANDSHAKE.

### The pre-traffic pointer gate — and a DEPARTURE from the brief

The brief pointed at `assertDefaultChainTLSPosture` (`manager_test.go:6412`) as
the model. Its **discipline** was followed; its **assertions** were not, and the
reason is a measurement:

`assertDefaultChainTLSPosture` asserts `rt.tlsMode` plus the FIVE `ssl.*`
pointers, because on the TCP path `serveConnection` Inc's them. **On the QUIC
path nothing ever does.** `Manager.Start` never launches `serveConnection` for
`kindQUIC` — which is exactly what the pre-existing
`TestQUICListener_RegistersSSLNamesAtZero` pins (all five registered, all five
still **zero** after a real H3 round trip). Asserting those five here would be
asserting pointers nothing dereferences: a green that buys no crash safety.

The pointers the QUIC accept goroutine actually dereferences are
`rt.downstreamCxTotal` and `rt.downstreamCxActive` (`quic.go:102-103`), and
`registerListenerMetrics` registers both UNCONDITIONALLY, **above** the
`if rt.tlsMode` gate (`manager.go:409-410`) — so the new
`assertQUICAcceptCounterPointers` is not a restatement of the `tlsMode`
computation either. It runs while the listener is still idle, before the first
byte. There is no `recover()` anywhere in non-test `internal/listener`, so a nil
`*stats.Counter` `Inc` there is a PROCESS CRASH that erases every other arm's
result in the same binary — which is also why this gate uses `t.Fatalf` where
`assertDefaultChainTLSPosture` uses `t.Errorf`: there, the crash is the point
being demonstrated; here, stopping is what prevents it.

### THE BODY, not the dial

`mkHCMFilterQUICChain` gives the two chains DIFFERENT `direct_response` bodies
(`FC0\n` / `DFC\n`) and the SAME status (**222**). On this listener the TLS
config comes from `quicTLSConfig()` and the filters come from `quicChain()` —
two different accessors — so **the handshake completes under either selection**.
"the dial returned" answers "did the handshake complete", which is true no
matter which chain served. The body is the ONLY observable that names the chain,
and it is what both arms assert. Status 222 is asserted separately as a coarse
"a configured route answered at all" property; it names no chain.

Each arm additionally polls `downstream_cx_total >= 1`, proving the traffic went
through THIS listener's accept path.

### Predicted vs ACTUAL — Task 6

| Arm | Predicted | ACTUAL | Match |
| --- | --- | --- | --- |
| h — matching SNI ⇒ indexed chain | 🔴 RED | 🔴 RED | ✅ |
| i — non-matching SNI ⇒ default slot | 🟢 GREEN | 🟢 GREEN | ✅ |

**Arm (h), the firing assertions, quoted verbatim from the run** — BOTH
properties fired, so no first divergence masked a later one:

```
quic_test.go:1298: SNI selection: body = "DFC\n", want "FC0\n" — the ClientHello
carried server_name "alpha.envoy-go.test" which matches
filter_chains[0].filter_chain_match.server_names, so the eligible indexed chain
must serve, not the last-resort default slot

quic_test.go:1305: last-resort ordering: the default_filter_chain served (body
"DFC\n") while filter_chains[0] was ELIGIBLE for this connection (server_names
matched SNI "alpha.envoy-go.test"); default_filter_chain is consulted only when
no indexed chain is eligible
```

Properties 1 (status 222) and 4 (`downstream_cx_total`) did NOT fire — the chain
terminal was reached and the connection was accounted. The failure is a
SELECTION failure, cleanly isolated.

**🟢 Arm (i) is GREEN at the un-fixed tip and that is NOT coverage** — it is the
false-agreement class, the same one arm (b) sits in. At this tip `quicChain()`
is nullary: `if rt.defaultChain != nil { return rt.defaultChain }`. It answers
"the default chain" on EVERY QUIC connection, having evaluated no dimension of
any `filter_chain_match` and having never looked at the SNI. Arm (i)'s correct
answer HAPPENS TO BE the default chain. **The pair is the gate, not either arm**:
arm (h) alone is satisfiable by a selector that always returns
`filter_chains[0]`; arm (i) alone is satisfiable by a selector that always
returns the default slot — which is exactly what the tip does. The two arms'
LISTENERS are identical; their two CLIENTS differ in one string.

**What is still NOT proven at this tip:** that the SNI reached *this listener's*
selector. At this tip the selector reads nothing at all, so no arm can show it.
The control proves the name reached *a* server's ClientHello; the rest becomes
observable only once the fix lands.

### Flake characterisation — Task 6

The two driven arms were run **5 consecutive times** with `-count=1`:
`h` FAIL / `i` PASS on **5 of 5** iterations, no variation in outcome or in
which assertion fired. No flake observed. (Both arms log
`quic: serve: accepting stream failed: Application error 0x0 (remote)` — that is
`http3.Server` observing the client's own clean connection close after the
response, on EVERY iteration including the green one; it is not an error path
either arm asserts on.)

### Gates — Task 6

| Gate | Command | Result |
| --- | --- | --- |
| new arms, isolated | `go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection_(MatchingServerNameSelectsIndexedChain\|NonMatchingServerNameFallsBackToDefaultSlot)' -v` | RC **1**, **2** `=== RUN`, (h) RED, (i) GREEN |
| full package | `go test ./internal/listener/... -count=1 -v` | RC **1** |
| `=== RUN` count | full package | **235** (was 233 at Task 5; +2) |
| anchored FAIL | `^(FAIL\|--- FAIL)\|^ *--- FAIL` | **8** lines = 5 `--- FAIL` (arms a, c, d, f, **h**) + 3 package/summary lines |
| panic gate | `^panic:\|DATA RACE\|SIGSEGV` | **0** |
| gofmt | `gofmt -l internal/listener/` | **empty output** |
| vet | `go vet ./internal/listener/...` | RC **0** |
| golangci-lint | `golangci-lint run ./internal/listener/...` | RC **0**, no output |

Sibling packages green: `listenerfilter` **ok**, `listenerfilter/tls_inspector` **ok**.

⚠️ **golangci-lint caught four British spellings on the first pass** —
`behaviour` ×2, `behaviours`, `synthesised` — in the new doc comments
(`misspell`, locale US). Corrected; `git diff --numstat` then reported
`392 0 internal/listener/quic_test.go`, i.e. **zero deleted lines**, proving the
correction touched only newly-added lines and rewrote nothing pre-existing.

### Production code byte-unchanged — Task 6

```
internal/listener/quic.go: OK
internal/listener/manager.go: OK
```

`sha256sum -c` RC **0**.

## Task 7 — arm (j): the accessor-identity pin

Arms (a)-(i) each exercise exactly ONE of quic.go's two accessors. Arm (j) is
the only arm that asks whether they describe the **same chain**.
`serveQUICConnection` draws the filter chain from `quicChain()` (`quic.go:123`)
and hands `http3.Server` a `TLSConfig` from `quicTLSConfig()` (`quic.go:144`);
`startQUIC` hands `quic.Listen` the same `quicTLSConfig()` (`quic.go:32`). If
those resolve to different chains, the connection is TERMINATED with one chain's
certificate and SERVED by another chain's filters — a split every other arm in
this phase is structurally blind to.

### The fixture is the arm — and BOTH of its constraints were CONTROLLED, not assumed

The two accessors differ in exactly ONE conjunct at this tip:

```
quicTLSConfig:  if rt.defaultChain != nil && rt.defaultChain.tlsCfg != nil
quicChain:      if rt.defaultChain != nil
```

**🔴 Constraint 1 — the default slot must be PLAINTEXT.** If both slots carried
TLS, the extra `tlsCfg != nil` would be true, both accessors would take their
default-slot branch, and they would agree. **Measured, not argued** — a scratch
control built the both-TLS variant and printed the two accessors:

```
BOTH-TLS: quicChain()=0x…442330 tlsCfg=0x…184b40 ; quicTLSConfig()=0x…184b40 ;
          ACCESSORS_AGREE=true
BOTH-TLS: indexed=0x…3d9d10 default=0x…442330
```

`ACCESSORS_AGREE=true` at the UN-FIXED tip. Built that way, arm (j)'s
property 1 would be a **VACUOUS GREEN**. (Precisely: property 1 only — the
selection property 2 would still be red there, since `quicChain()` still returns
the default slot. The vacuity is confined to the accessor-identity assertion,
which is the entire reason arm (j) exists.) The arm now asserts
`dflt.tlsCfg == nil` as a `t.Fatalf` PRECONDITION, so a future edit that
re-adds TLS to the default slot stops the arm rather than silently greening it.

**🔴 Constraint 2 — the indexed chain must be ELIGIBLE.** With an INELIGIBLE
`filter_chains[0]`, the CORRECT post-fix answer is that `quicChain()` selects
the default slot (nil TLS) while `quicTLSConfig()` still finds the indexed
chain's config — the accessors would legitimately DISAGREE, and this pin would
be **RED AGAINST CORRECT CODE**, which is exactly as bad as vacuous. A second
scratch control confirmed the guard is ARMED rather than decorative:

```
INELIGIBLE: chainSpecs[0].Name="quic_listener_chains/filter_chains[0]" Empty=false
```

`Empty=false` is what the arm's `if !rt.chainSpecs[0].Empty { t.Fatalf }`
precondition fires on. Both scratch controls were deleted before commit.

### Why this shape builds — a known divergence CONSUMED as a fixture

A QUIC listener normally cannot express a plaintext chain: the `filter_chains[]`
loop rejects a missing `transport_socket` with `"quic listener requires a
transport_socket (mandatory TLS)"` (`manager.go:658`). **That reject covers
`filter_chains[i]` ONLY.** The `default_filter_chain` branch carries no kind
check at all — its `transport_socket` branch is simply skipped, `dfcTLS` is left
nil, and the listener builds. That asymmetry is the banked D2-QUICTS divergence,
deliberately unrepaired. Arm (j) does not fix it; it USES it, because a TLS-less
default slot is the only fixture that can separate the two accessors. Any future
repair of D2-QUICTS must revisit this arm alongside `mkQUICListenerChains`'s
`defaultTLS=false` callers.

### The assertion, as written

`quicTLSConfig()` returns a `*stdtls.Config`, **not** a `*chainInfo`, so the two
accessors cannot be compared directly. The comparison is by POINTER, between the
config the listener will actually use and the config belonging to the chain that
will actually serve:

```go
selected := rt.quicChain()
if got := rt.quicTLSConfig(); got != selected.tlsCfg {
```

A "both non-nil" check would prove nothing about agreement.

### Predicted vs ACTUAL — Task 7

| Arm | Predicted | ACTUAL | Match |
| --- | --- | --- | --- |
| j — accessor identity, plaintext default slot | 🔴 RED | 🔴 RED | ✅ |

**The firing assertions, quoted verbatim** — BOTH properties fired, so no first
divergence masked a later one, and the pointer values confirm the predicted
split exactly:

```
quic_test.go:1531: accessor identity: quicTLSConfig() = 0x3f029c2045a0 but
quicChain()'s chain carries tlsCfg 0x0 — the connection would be TERMINATED with
one chain's TLS config and SERVED by another chain's filters
(filter_chains[0].tlsCfg=0x3f029c2045a0, default_filter_chain.tlsCfg=0x0)

quic_test.go:1540: selection identity: quicChain() = 0x3f029c089da0, want
filter_chains[0] = 0x3f029c089d10 (it is empty-match and therefore eligible for
every connection; default_filter_chain 0x3f029c089da0 is the LAST-RESORT slot)
```

`quicTLSConfig()` returned `0x…45a0`, which is **`filter_chains[0].tlsCfg`**;
`quicChain()` returned `0x…89da0`, which is **the default slot**, whose `tlsCfg`
is `0x0`. TLS from `filter_chains[0]`, filters from the default slot — the
predicted split, observed. All five preconditions (both chains present, distinct,
default `tlsCfg` nil, indexed `tlsCfg` non-nil, indexed chain empty-match) held.

Arm (j) was run **5 consecutive times** with `-count=1`: FAIL on 5 of 5, no
variation. It is a nil-conn arm — no sockets, no goroutines — so there is no
flake surface.

### Gates — Task 7

| Gate | Command | Result |
| --- | --- | --- |
| new arm, isolated | `go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection_TLSConfigAndChainAgreeOnOneChain' -v` | RC **1**, **1** `=== RUN`, (j) RED |
| full package | `go test ./internal/listener/... -count=1 -v` | RC **1** |
| `=== RUN` count | full package | **236** (was 235 at Task 6; +1) |
| anchored FAIL | `^(FAIL\|--- FAIL)\|^ *--- FAIL` | **9** lines = 6 `--- FAIL` (arms a, c, d, f, h, **j**) + 3 package/summary lines |
| panic gate | `^panic:\|DATA RACE\|SIGSEGV` | **0** |
| gofmt | `gofmt -l internal/listener/` | **empty output** |
| vet | `go vet ./internal/listener/...` | RC **0** |
| golangci-lint | `golangci-lint run ./internal/listener/...` | RC **0**, no output |

Sibling packages green: `listenerfilter` **ok**, `listenerfilter/tls_inspector` **ok**.

### Production code byte-unchanged — Task 7 (final)

```
internal/listener/quic.go: OK
internal/listener/manager.go: OK
```

`sha256sum -c` RC **0**, against hashes captured BEFORE any edit in this
session. `git status --porcelain -- internal/listener/quic.go
internal/listener/manager.go` printed **empty output**.

### Tasks 6-7 roll-up: predicted vs ACTUAL

| Arm | Predicted | ACTUAL | Match |
| --- | --- | --- | --- |
| h — matching SNI ⇒ indexed chain (DRIVEN) | 🔴 RED | 🔴 RED | ✅ |
| i — non-matching SNI ⇒ default slot (DRIVEN) | 🟢 GREEN | 🟢 GREEN | ✅ |
| j — accessor identity | 🔴 RED | 🔴 RED | ✅ |

Three for three, and ten for ten across the whole IMPL spine (a-j).

**Contradictions of the Task 6/7 brief, stated loudly:**

1. **`assertDefaultChainTLSPosture` is the WRONG pointer gate for a QUIC arm.**
   Its five `ssl.*` pointers are never dereferenced on the QUIC path —
   `serveConnection` is not reached for `kindQUIC` — so asserting them would be
   a green that buys no crash safety. The QUIC accept goroutine dereferences
   `downstreamCxTotal` and `downstreamCxActive`; those are what the new gate
   asserts. The brief's DISCIPLINE was right; its named helper was not.

2. **`ServerName` is not entirely absent from `internal/listener/`.** The brief
   said none of the package's clients sets it. Four TCP TLS clients in
   `tls_handshake_negative_test.go` DO set it (`alpha.envoy-go.test`,
   `gamma.envoy-go.test`). The claim is true **as scoped to the three HTTP/3
   client sites**, which is what matters here — but the unqualified form is
   false, and it was checked rather than repeated.

3. **`driveH3` could not have been reused for these arms even with an SNI
   added**: it hard-asserts `status == 200`, and `mkHCMFilterQUICChain`'s
   `direct_response` status is **222**. A reuse would have failed on the status
   line and never reached a body comparison.

## Task 8 — the UN-FIXED-TIP ROSTER, the negative control SPENT

⚠️ **THIS TABLE IS NC ROSTER ROW 1's EVIDENCE AND CANNOT BE REPRODUCED AFTER TASK 9.** Row 1's
mutation — making the selector prefer `rt.defaultChain` first again — reproduces exactly this tip, so
row 1 is scored ARM BY ARM against this table, never as a single pass/fail.

**Measured at tip `e9b204d9437fe8b98b7fc500f1c4adc49d0c3042`**, one selection over all ten arms:

```sh
cd /home/esa/git/wt-97-impl && go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection' -v
```

`RC=1` · `=== RUN` **10** · `--- PASS` **4** · `--- FAIL` **6** · anchored panic gate
`^panic:|DATA RACE|SIGSEGV` **0** (PROVEN LIVE at Task 1, both accessors, one site per run).

⚠️ **`RUN=10` is the selector-matched proof.** A `-run` selector matching nothing prints
`[no tests to run]` and EXITS 0; ten `=== RUN` lines is what rules that out.

| arm | test | dimension | expected selection | AT THIS TIP | |
|---|---|---|---|---|---|
| a | `TestQUICChainSelection_IndexedChainWinsOverDefaultSlot` | empty match | indexed | default | 🔴 RED |
| b | `TestQUICChainSelection_IneligibleIndexedChainFallsBackToDefaultSlot` | `destination_port` | default | default | 🟢 GREEN |
| c | `TestQUICChainSelection_IneligibleIndexedChainNoDefaultSlotSelectsNothing` | `destination_port`, no default slot | nil ⇒ closed | the ineligible chain | 🔴 RED |
| d | `TestQUICChainSelection_TransportProtocolQUICMatches` | `transport_protocol` | indexed | default | 🔴 RED |
| e | `TestQUICChainSelection_TransportProtocolTLSDoesNotMatch` | `transport_protocol` | default | default | 🟢 GREEN |
| f | `TestQUICChainSelection_ApplicationProtocolsH3Matches` | `application_protocols` | indexed | default | 🔴 RED |
| g | `TestQUICChainSelection_ApplicationProtocolsH2DoesNotMatch` | `application_protocols` | default | default | 🟢 GREEN |
| h | `TestQUICChainSelection_MatchingServerNameSelectsIndexedChain` | `server_names`, DRIVEN | indexed | default | 🔴 RED |
| i | `TestQUICChainSelection_NonMatchingServerNameFallsBackToDefaultSlot` | `server_names`, DRIVEN | default | default | 🟢 GREEN |
| j | `TestQUICChainSelection_TLSConfigAndChainAgreeOnOneChain` | accessor identity | one chain | TLS indexed, filters default | 🔴 RED |

**RED = {a, c, d, f, h, j} — SIX. GREEN = {b, e, g, i} — FOUR.** This is `PLAN.md` §0.10's prediction,
reproduced exactly, arm for arm.

### The four GREENs are NOT a TDD failure

The subject answers *"default chain"* on **every** QUIC arm because `quicChain()` is nullary and
consults nothing. It therefore agrees with the correct answer **by coincidence**, wherever *default*
happens to be correct. **b, e, g and i have NO falsifiability at this tip** — their only falsifiability
is NC roster row 2, which is why that row is scored PER ARM. A reader who sees them pass before Task 9
must not read that as coverage.

### WHICH assertion fired, per RED arm — both properties, every arm

⚠️ **No first divergence masked a later one: all six RED arms fired BOTH of their properties**, twelve
assertions in total. Each message names its own property, so none collapsed onto a shared line.

| arm | firing assertions |
|---|---|
| a | `:385` selection identity — returned pointer EQUALS the default slot · `:393` last-resort ordering |
| c | `:625` no-match selects nothing — returned the chain whose `destination_port` 65000 rules it out · `:634` ineligible chain is not the fallback |
| d | `:723` transport_protocol match selects indexed · `:730` eligible chain pre-empts last resort |
| f | `:918` ALPN match selects indexed · `:925` eligible chain pre-empts last resort |
| h | `:1298` SNI selection — body `DFC` served where `FC0` was eligible · `:1305` last-resort ordering |
| j | `:1531` accessor identity — `quicTLSConfig()` non-nil while the selected chain carries `tlsCfg` **0x0** · `:1540` selection identity |

⚠️ **Arm h's status-222 and `downstream_cx_total` properties did NOT fire** — it is a clean SELECTION
failure, not a transport or handshake failure. Arm j's `:1531` is the cross-wiring pinned: the
connection would be TERMINATED with one chain's TLS config and SERVED by another chain's filters.

### The roster is the unit of comparison, not the counters

⚠️ **DIFF THE ARM ROSTER — the ten names above — at every later checkpoint, never the counts.** A
`+0/+0` arm can be silently deleted with every gate staying green.

---

## Task 9 — production edit 1 of 3: the resolve MOVES above `quicTLSConfig`

Commit `fe253512`. One file: `internal/listener/quic.go`.

`rt.addr = udpConn.LocalAddr().String()` moved from after a successful `quic.Listen` to immediately
after a successful `net.ListenUDP`, i.e. ABOVE `tlsCfg := rt.quicTLSConfig()`. Task 11 puts the
Start-time chain selection inside `quicTLSConfig()`, and that selection derives its
`destination_port` / `prefix_ranges` inputs from `rt.addr` — pre-bind that string may still read
`:0` (OS-pick), which would make every port-bearing `filter_chain_match` evaluate against port 0.

### The ONE behaviour change, declared

On `startQUIC`'s two failure paths — `tlsCfg == nil`, and a `quic.Listen` error — `rt.addr` now
holds the RESOLVED address where it previously held the configured one. `Manager.Start`'s
`listener: %q: bind %s: %w` text therefore changes for a `port_value: 0` listener from `:0` to the
resolved port. **Accepted deliberately: the socket IS bound on both paths**, so the resolved address
is the more accurate one.

### Pin search — re-run at this tip, NOT inherited

| string | executable hits | enclosing symbol | doc-only hits | test/fixture hits |
|---|---|---|---|---|
| `bind %s` | `internal/listener/manager.go:1170`, `:1180` | `func (m *Manager) Start(ctx context.Context) error` (QUIC branch and TCP branch of the same format string) | 5 (`61-http3-downstream-listener/PLAN-61.1.md:579`; `97-.../SPEC.md:449`,`:455`; `97-.../PLAN.md:990`,`:992`; `02-tcp-proxy/PLAN.md:1315`) | **0** |
| `quic listener has no TLS config` | `internal/listener/quic.go:35` | `func (rt *listenerRuntime) startQUIC` | 8 (phases 61.1, 74, 95, 96, 97) | **0** |

⚠️ **Neither message is pinned by any test or fixture.** The claim was re-derived here, not carried
in from the SPEC.

### By-symbol landing proof

A build is not evidence an edit landed. In `internal/listener/quic.go`:

```
45:	rt.addr = udpConn.LocalAddr().String() // resolved (port 0 → OS pick)
46:	tlsCfg := rt.quicTLSConfig()
```

**45 < 46** — the resolve is strictly above the config call.

Gates: `gofmt -l internal/listener/` empty · `go vet ./internal/listener/...` rc 0.

---

## Task 10 — production edit 2 of 3: inputs builder, selector, `quicChain(conn)`

Commit `b10b3295`. Two files: `internal/listener/quic.go` (production) and
`internal/listener/quic_test.go` (eight one-token call sites).

### `quicChainMatchInputs(conn *quic.Conn)`

ALWAYS stamps `TransportProtocol: "quic"` and `ApplicationProtocols: []string{"h3"}` (new package
constants `quicTransportProtocol` / `quicApplicationProtocol`). ⚠️ **There is no reverse wildcard:**
`matches` treats the CHAIN's empty `TransportProtocol` as "unspecified", but a chain SPELLING
`transport_protocol: quic` against an UNSET input is INELIGIBLE. Leaving the input blank would
silently disable the dimension.

- `conn == nil` (the Start-time moment): destination from `rt.addr` via `net.ResolveUDPAddr`;
  `ServerName` empty; source unset.
- `conn != nil`: destination from `conn.LocalAddr()`, source from `conn.RemoteAddr()`, **both
  comma-ok asserted to `*net.UDPAddr`**; `ServerName` from `conn.ConnectionState().TLS.ServerName`.
  Filling `SourceIP` activates `source_type` for free — `IsLoopbackSource()` derives it.

🔴 **`localIP` / `localPort` / `remoteIP` / `remotePort` are NOT reused and NOT widened.** Those four
`manager.go` helpers comma-ok assert `*net.TCPAddr` and return `nil` / `0` for a UDP address
**silently** — reusing them would compile, run, and make every `destination_port` dimension evaluate
against port 0. Widening them would put executable lines into `manager.go`, which stays
byte-untouched in these three tasks.

### `selectQUICChain(conn)` and `quicChain(conn)`

`listenerfilter.SelectChain(inputs, rt.chainSpecs, rt.defaultSpec)` → on error nil → otherwise
`rt.chainByName[spec.Name]`. SelectChain returns a `*ChainSpec`, **never** a `*chainInfo`, so the map
lookup is mandatory, exactly as `serveConnection` does it. `serveQUICConnection`'s existing
nil-to-`CloseWithError` is KEPT — a nil selection must close, which is what the reference does with
*"no filter chain found"*, and arm (c) pins it. The error is logged as
`listener %q: chain-match: %v` before the close, mirroring the TCP path.

⚠️ **ONE DEVIATION, DECLARED.** That log is suppressed for the Start-time (`conn == nil`) call.
`quicTLSConfig()` treats a nil Start-time selection as a normal fall-through to its own fallbacks,
and it is invoked AGAIN per connection from the `&http3.Server{...}` literal — logging there would
emit a `chain-match` line per connection on a perfectly healthy listener. The connection path logs.

### The test-file diff is ONE TOKEN, eight times

`git diff --numstat` for this task reads `8  8  internal/listener/quic_test.go`. Every hunk is
`rt.quicChain()` → `rt.quicChain(nil)`, at lines **381, 527, 621, 719, 796, 914, 1005, 1522**. No
assertion, message, or expectation was touched. The ninth textual hit, `quic_test.go:562`, sits
inside a `//` comment quoting `quic.go` and is not a call site; it was left alone rather than widen
the diff.

Gates: `gofmt -l` empty · `go vet` rc 0 · `go build ./...` rc 0.

---

## Task 11 — production edit 3 of 3: `quicTLSConfig` stays connection-INDEPENDENT, both loops die

This commit (the section below was appended to it by amend). One file: `internal/listener/quic.go`.

`quicTLSConfig()` KEEPS its nullary signature. `quic.Listen` demands one config before any connection
exists, so a repair making it depend on per-connection state would be unimplementable at
`startQUIC`'s call site. ⚠️ **It is called at TWO MOMENTS** — `startQUIC`, and the
`&http3.Server{...}` literal in `serveQUICConnection`. Connection-independence is what guarantees
both return the same pointer; if they could diverge, a connection would be TERMINATED with one
chain's TLS config and SERVED by another chain's filters.

New body, in order: (1) the Start-time selection `selectQUICChain(nil)`'s `tlsCfg`; (2) the first
TLS-bearing chain in `rt.chainSpecs` **SLICE ORDER**; (3) the default slot's `tlsCfg`; (4) nil.

⚠️ **The fallback order is not arbitrary**, and the comment says so: `rt.chainSpecs` is in the
config's `filter_chains[]` order, so step 2 is deterministic and reproduces the operator's ordering;
the default slot is consulted **LAST** because it IS the last-resort slot.

### Both `chainByName` loops are DELETED

🔴 `rt.chainByName` is a MAP and it **CONTAINS the default slot**
(`chainByName[defaultSpec.Name] = defaultChain`, `manager.go:779`). Both accessors' loops therefore
ranged over the UNION of indexed and default chains in nondeterministic order and could return the
default chain by map-order accident while reading as if they were choosing an indexed one. The
iteration is REMOVED, not documented — documenting a nondeterministic loop does not make it
deterministic.

Gate: `/usr/bin/grep -c 'for _, ci := range rt.chainByName' internal/listener/quic.go` reads **0**.
The eight surviving `range rt.chainByName` hits in the package are all in `manager_test.go` /
`quic_test.go`.

### The existing pin, confirmed BY NAME

`go test ./internal/listener/ -count=1 -run 'TestBuildListenerRuntime_QUICDefaultFilterChain_QUICWrappedKeepsNextProtos$' -v`
→ rc 0, `=== RUN` = **1**, `--- PASS`. It did not silently skip. It calls `quicTLSConfig()` on a
NEVER-STARTED runtime, so the new step-1 selection resolves `rt.addr` as the CONFIGURED string and
`net.ResolveUDPAddr` parses it.

### 🔴 COVERAGE FINDING — the ten-arm roster is BLIND to Task 11

**All ten arms were already GREEN at the Task-10 commit `b10b3295`, before this task landed.**
Arm (j) `TLSConfigAndChainAgreeOnOneChain` is the only arm that touches `quicTLSConfig`, and its own
preconditions FORBID the shape this task changes: it hard-requires `dflt.tlsCfg == nil`
(`quic_test.go:1505`), so the OLD body skipped its `defaultChain` branch and its map loop had
exactly ONE TLS-bearing candidate to find. Agreement by there being one candidate, not by ordering.

To avoid landing the deletion on an unmeasured claim, an **ephemeral probe** (written, run, and
DELETED — it is in no commit) built the both-TLS shape arm (j) excludes:
`mkQUICListenerChains(t, nil, "FC0\n", true, true, "DFC\n")`, empty-match eligible
`filter_chains[0]`, two DISTINCT non-nil `*stdtls.Config` pointers. **Both arms of the control were
RUN:**

| tip | `quicTLSConfig()` returns | probe |
|---|---|---|
| Task 11 (this commit) | `indexed.tlsCfg` | **PASS** |
| Task 10 (`quic.go` stashed back) | `dflt.tlsCfg` | **FAIL** |

The un-fixed arm is RED and the fixed arm is GREEN, so this task IS a real behaviour change — and
the committed roster cannot see it. ⚠️ **No permanent arm was added** (the brief scoped
`quic_test.go` to one call-site token), so this gap is recorded, not closed.

---

## Tasks 9-11 — after all three

### The ten-arm ROSTER (not the counters)

`go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection' -v` → **rc 0**, `=== RUN` = **10**,
anchored FAIL = **0**.

| arm | name | Task 8 (un-fixed) | now |
|---|---|---|---|
| a | `TestQUICChainSelection_IndexedChainWinsOverDefaultSlot` | RED | **PASS** |
| b | `TestQUICChainSelection_IneligibleIndexedChainFallsBackToDefaultSlot` | green (false agreement) | **PASS** |
| c | `TestQUICChainSelection_IneligibleIndexedChainNoDefaultSlotSelectsNothing` | RED | **PASS** |
| d | `TestQUICChainSelection_TransportProtocolQUICMatches` | RED | **PASS** |
| e | `TestQUICChainSelection_TransportProtocolTLSDoesNotMatch` | green (false agreement) | **PASS** |
| f | `TestQUICChainSelection_ApplicationProtocolsH3Matches` | RED | **PASS** |
| g | `TestQUICChainSelection_ApplicationProtocolsH2DoesNotMatch` | green (false agreement) | **PASS** |
| h | `TestQUICChainSelection_MatchingServerNameSelectsIndexedChain` | RED | **PASS** |
| i | `TestQUICChainSelection_NonMatchingServerNameFallsBackToDefaultSlot` | green (false agreement) | **PASS** |
| j | `TestQUICChainSelection_TLSConfigAndChainAgreeOnOneChain` | RED | **PASS** |

The ROSTER is identical — ten names in, ten names out. No arm was deleted, renamed, or edited beyond
the single `quicChain()` → `quicChain(nil)` token.

### Full package

`go test ./internal/listener/... -count=1 -v` → **rc 0** (`PIPESTATUS[0]`), `=== RUN` = **236**,
anchored FAIL (`^(FAIL|--- FAIL)|^ *--- FAIL`) = **0**, anchored panic gate
(`^panic:|DATA RACE|SIGSEGV`) = **0**.

### `manager.go` byte-unchanged

```
/home/esa/git/wt-97-impl/internal/listener/manager.go: OK
```

`git diff --numstat master -- internal/listener/manager.go` returns **no row at all** — the file is
byte-identical to `master`.

### Actual change size

`git diff --numstat master -- internal/listener/quic.go` → **`146  22  internal/listener/quic.go`**.

⚠️ The measured prototype floor was `+87 / -23`. The landed insert count is **+146**, well above the
floor, which is a FLOOR and not a target — the surplus is comment, not code: the two behaviour
declarations (the `bind %s` change, the `chainByName` map hazard) and the fallback-order rationale
the brief required be written down. The delete count is **-22**, one BELOW the prototype's `-23`;
nothing was trimmed to hit a number, and the figure is reported as measured.

### ⚠️ One defect found by a gate the brief did not name

`golangci-lint run ./internal/listener/...` runs **misspell in locale US**, and Task 9's comment
spelled `BEHAVIOUR CHANGE`. That is a lint FAILURE (`quic.go:40:5: 'BEHAVIOUR' is a misspelling of
'BEHAVIOR' (misspell)`, rc 1) which `gofmt`, `go vet` and `go build` all passed over. Corrected to
`BEHAVIOR CHANGE`; `golangci-lint` now rc **0**.

⚠️ **The correction rides in the Task 11 commit, not Task 9's**, because only HEAD is amendable
without an interactive rebase (unsupported in this environment). Stated rather than hidden: the
Task-9 commit as recorded contains a lint-failing comment word, fixed one commit later.

## Task 12 — the eleven-arm suite, and the coverage gap arm (k) closes

### Part A — arm (k): why a NEW arm was required

Task 11 landed a production edit (`quicTLSConfig`'s resolution order) that **no committed arm could
see**. That was measured, not suspected: the ten-arm roster (a)-(j) went **ALL-GREEN at Task 10**,
before Task 11's rewrite existed, and Task 11's own ephemeral inverting probe — since deleted —
showed the two bodies return DIFFERENT pointers on a both-TLS listener.

The mechanism of the blindness is arm (j)'s own precondition. Arm (j) is the only arm that calls
`quicTLSConfig()`, and it **hard-requires `dflt.tlsCfg == nil`** (a PLAINTEXT default slot) —
because a TLS-bearing default slot would make `quicTLSConfig()` and `quicChain()` agree trivially
and turn arm (j) into a vacuous green. But that same precondition leaves arm (j)'s listener with
exactly **ONE TLS-bearing candidate**, and with one candidate *every* resolution order returns the
same pointer. Arm (j) agrees by **SINGLE-CANDIDACY, not by ordering**.

A production edit with zero permanent coverage is not acceptable, so arm (k) was added.

**`TestQUICChainSelection_TLSConfigPrefersIndexedChainOverTLSDefaultSlot`** (arm k),
`internal/listener/quic_test.go`:

- **SHAPE:** `mkQUICListenerChains(t, nil, "FC0\n", true, true, "DFC\n")` — an empty-match (therefore
  ELIGIBLE) `filter_chains[0]` with its own QUIC transport socket, **AND** a QUIC-TLS default slot.
  Two TLS-bearing candidates.
- **PROPERTY 1 (the assertion):** `rt.quicTLSConfig() == indexed.tlsCfg` by POINTER — the Start-time
  selection wins and the last-resort default slot does NOT pre-empt it.
- **PROPERTY 2:** stated separately — `got == dflt.tlsCfg` must be false, so a failure names WHICH
  slot supplied the config rather than only that the pointer was wrong.
- **Preconditions (`t.Fatalf`), anti-vacuity:** both slots present and distinct `*chainInfo`; BOTH
  `tlsCfg` pointers **non-nil**; the two `tlsCfg` pointers **NOT EQUAL to each other** (if they were
  one object the assertion would hold under every possible order); `len(rt.chainSpecs) == 1` and
  `rt.chainSpecs[0].Empty` (an ineligible indexed chain would make the default slot's config the
  CORRECT answer and this pin RED AGAINST CORRECT CODE).

House style held: one straight-line `func Test…`, no table, `t.Errorf` per property with each
message naming its property, `t.Fatalf` only for a broken precondition.

### NC roster row 10 — the mechanism, NAMED BEFORE the run

> The mutation deletes the `selectQUICChain(nil)` step and tries `rt.defaultChain.tlsCfg` FIRST,
> then ranges `rt.chainByName`. Arm (k)'s listener carries a non-nil default-slot `tlsCfg` that is a
> DIFFERENT pointer from `filter_chains[0].tlsCfg` (asserted as a precondition), so the mutated body
> returns the default slot's pointer. Arm (k)'s property 1 is an exact pointer comparison against
> `indexed.tlsCfg` → it fires; property 2 fires too. Arms (a)-(i) never call `quicTLSConfig` — they
> exercise `quicChain()` / `serveQUICConnection`, a different function the mutation does not touch.
> Arm (j)'s default slot has `tlsCfg == nil`, so the mutated first branch is SKIPPED and the map loop
> finds the single TLS-bearing candidate — the same pointer either way, so arm (j) stays green.

NEUTRALISED, not reverted: the package still compiled (`go build ./internal/listener/` OK,
`gofmt -l` empty) and the `-run` selector matched — **`=== RUN` = 11** under the mutation, so the
control is not a `[no tests to run]` false pass.

**Per-arm scoring under NC row 10** (`go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection' -v`, rc **1**):

| arm | name | under NC row 10 |
| --- | --- | --- |
| a | `IndexedChainWinsOverDefaultSlot` | PASS |
| b | `IneligibleIndexedChainFallsBackToDefaultSlot` | PASS |
| c | `IneligibleIndexedChainNoDefaultSlotSelectsNothing` | PASS |
| d | `TransportProtocolQUICMatches` | PASS |
| e | `TransportProtocolTLSDoesNotMatch` | PASS |
| f | `ApplicationProtocolsH3Matches` | PASS |
| g | `ApplicationProtocolsH2DoesNotMatch` | PASS |
| h | `MatchingServerNameSelectsIndexedChain` | PASS |
| i | `NonMatchingServerNameFallsBackToDefaultSlot` | PASS |
| j | `TLSConfigAndChainAgreeOnOneChain` | PASS |
| **k** | **`TLSConfigPrefersIndexedChainOverTLSDefaultSlot`** | **FAIL** |

Arm (k) is the **ONLY** red. That arms (a)-(j) stayed green is not an assumption carried over from
Task 11 — it was RUN here, and it CONFIRMS the blindness rather than merely predicting it. The
observed failure text, verbatim:

```
    quic_test.go:1636: quicTLSConfig ordering: got 0x1a189280bc20, want filter_chains[0].tlsCfg 0x1a189280ba40 — filter_chains[0] is empty-match and therefore eligible for the Start-time selection, and the last-resort default_filter_chain (tlsCfg 0x1a189280bc20) must never pre-empt an eligible indexed chain
    quic_test.go:1642: default slot pre-empted: quicTLSConfig() = 0x1a189280bc20 = default_filter_chain.tlsCfg — the default slot was consulted BEFORE the Start-time chain selection, which is the pre-Task-11 resolution order this arm exists to exclude
```

Both properties fired, and the pointers name exactly the predicted swap.

**Restore, verified by digest** — `sha256sum -c` against digests captured BEFORE the mutation:

```
internal/listener/quic.go: OK
internal/listener/quic_test.go: OK
internal/listener/manager.go: OK
```

Re-run after restore: **11 `--- PASS`**, rc 0.

### Part B — the eleven-arm roster, by NAME

`go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection' -v` → rc **0**,
**`=== RUN` = 11**, all eleven `--- PASS`. Roster diffed by NAME, not by counter — the ten inherited
names are present, unrenamed and unedited, and arm (k) is the only addition:

| arm | name | result |
| --- | --- | --- |
| a | `TestQUICChainSelection_IndexedChainWinsOverDefaultSlot` | PASS |
| b | `TestQUICChainSelection_IneligibleIndexedChainFallsBackToDefaultSlot` | PASS |
| c | `TestQUICChainSelection_IneligibleIndexedChainNoDefaultSlotSelectsNothing` | PASS |
| d | `TestQUICChainSelection_TransportProtocolQUICMatches` | PASS |
| e | `TestQUICChainSelection_TransportProtocolTLSDoesNotMatch` | PASS |
| f | `TestQUICChainSelection_ApplicationProtocolsH3Matches` | PASS |
| g | `TestQUICChainSelection_ApplicationProtocolsH2DoesNotMatch` | PASS |
| h | `TestQUICChainSelection_MatchingServerNameSelectsIndexedChain` | PASS |
| i | `TestQUICChainSelection_NonMatchingServerNameFallsBackToDefaultSlot` | PASS |
| j | `TestQUICChainSelection_TLSConfigAndChainAgreeOnOneChain` | PASS |
| k | `TestQUICChainSelection_TLSConfigPrefersIndexedChainOverTLSDefaultSlot` | PASS |

### Full package

`go test ./internal/listener/... -count=1 -v` (rc via `PIPESTATUS[0]`):

| measure | value |
| --- | --- |
| rc | **0** |
| `^=== RUN` | **237** |
| anchored FAIL `^(FAIL\|--- FAIL)\|^ *--- FAIL` | **0** |
| anchored panic gate `^panic:\|DATA RACE\|SIGSEGV` | **0** |

237 = Task 11's measured 236 **+ 1** (arm k). Reported as measured, not adjusted to a prediction.

### `-race` on the FULL package

`go test -race ./internal/listener/... -count=1` — the **whole** package, never a `-run`-narrowed
selection (a selector matching nothing exits 0 and would read as a pass). The driven arms (h)/(i)
start a manager and a background accept goroutine, which is the shape only a full-package race run
catches.

| measure | value |
| --- | --- |
| rc | **0** |
| `DATA RACE` occurrences | **0** |
| anchored FAIL | **0** |
| wall time | **7s** (`internal/listener` 4.414s, `listenerfilter` 1.050s, `tls_inspector` 1.011s) |

### Gates

| gate | result |
| --- | --- |
| `gofmt -l internal/listener/` | **empty** (gated on OUTPUT, not exit code) |
| `go vet ./internal/listener/...` | rc **0** |
| `golangci-lint run ./internal/listener/...` | rc **0** |
| `go build ./...` | rc **0** |

New comment prose was swept for British spellings before commit (`golangci-lint`'s misspell runs in
locale US and caught `BEHAVIOUR` one task ago); `neutralized`/`behavior` are spelled US.

### `manager.go` byte-unchanged

```
/home/esa/git/wt-97-impl/internal/listener/manager.go: OK
```

### ⚠️ A GREEN SUITE IS NOT A PASS UNTIL THE NCs RUN

Task 12's green means something **only because Tasks 14-16 are coming**. This package was previously
measured **green under a patch that REVERSES chain selection** — the Task-8 un-fixed-tip roster and
Task 11's ephemeral probe both showed arms passing for reasons the mechanism could not supply. NC
row 10 above discharges exactly ONE row of the roster: the `quicTLSConfig` ordering. Every other
dimension (destination_port, transport_protocol, application_protocols, server_names, accessor
identity) still rests on rows that have NOT been run yet. Until they are, "eleven green arms" is a
statement about the arms, not about the production code.

## Task 13 — reconciling the prose the repair FALSIFIED, under a mechanically-gated comment-only constraint

**THE SENTENCE, AND ITS TWO INDEPENDENT HALVES.** The carried claim is *"`quicTLSConfig()`
(`quic.go:56-58`) returns `rt.defaultChain.tlsCfg` FIRST, before consulting `chainByName`"*. It is
falsified twice over, and a half-repair would have minted a new stale claim:

- **Half A — the PRECEDENCE. DEAD.** The default slot is now consulted **LAST**, behind (1) the
  Start-time `selectQUICChain(nil)` filter_chain_match selection and (2) `rt.chainSpecs` in **SLICE**
  order. `default_filter_chain` can no longer pre-empt an eligible indexed chain.
- **Half B — the MECHANISM. DEAD AS STATED, but NOT in the shape the brief predicted.** ⚠️ **The
  Task-13 brief says `chainByName` "is no longer consulted at all". THAT IS FALSE AT THIS TIP.**
  `rt.chainByName` is still read at **two** production sites — `quicTLSConfig`'s step 2
  (`rt.chainByName[spec.Name]`, `internal/listener/quic.go:103`) and `selectQUICChain`'s closing
  `return rt.chainByName[spec.Name]` (`:194`). What died is the **map-ORDER ITERATION**: both
  `for _, ci := range rt.chainByName` loops are gone (Task 11), so `chainByName` survives as a
  `name -> *chainInfo` **lookup table** and never as an ordering. Every rewrite below says exactly
  that, because "no longer consulted at all" would itself have been a fresh false claim.
- **THE THIRD HALF — the `quic.go:56-58` ANCHOR. DEAD TWICE OVER.** Those lines are no longer
  `quicTLSConfig`'s body (it now begins at `:98`) and no longer contain that logic. Every rewrite
  **drops** the anchor rather than renumbering it; line anchors rot.

**WHAT SURVIVES — and every rewrite LEADS WITH IT.** At all three sites the *conclusion* is
untouched. The zero-`filter_chains[]` + QUIC-wrapped-`default_filter_chain` shape still resolves to
the default slot's `tlsCfg`, because `SelectChain` over an **EMPTY** `chainSpecs` with a non-nil
`defaultSpec` returns the default spec (`listenerfilter/chainmatch.go:88-92`) — so the Start-time
selection returns it at **step 1**, not by fall-through to step 3. The ADR-0318 repeal therefore
holds under BOTH orders.

### The adjudication — every `quicTLSConfig` hit in the tree

`/usr/bin/grep -rni --exclude-dir=.git 'quicTLSConfig' /home/esa/git/wt-97-impl`

| file | hits | verdict | action |
|---|---|---|---|
| `internal/listener/manager.go:396` | 1 | **CARRIER**, both halves + anchor | **EDITED (site 2)** |
| `internal/listener/quic_test.go:240` | 1 | **CARRIER**, both halves + anchor | **EDITED (site 3)** |
| `internal/listener/manager_test.go:977` | 1 | **CARRIER — REASON only; conclusion survives** | **EDITED (site 4)** |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md:1973` | 1 of 2 | **CARRIER**, both halves | **LEFT — deferred**; a later task adds a ledger entry to this same file, so both edits land in one commit and keep the numstat legible |
| `docs/envoy-go/DECISIONS.md:19020` | 1 of 11 | **CARRIER, tense only** | **LEFT — deferred to the ADR completion** ⚠️ see the mis-attribution finding below |
| `docs/envoy-go/DECISIONS.md:18960` | 1 of 11 | **CARRIER, tense only** — ADR-0318 §Context ¶6 | **LEFT** ⚠️ **the brief's five-site enumeration does not list this one** |
| `docs/envoy-go/DECISIONS.md` (9 others: `:16767`, `:16783`, `:16806`, `:17276`, `:17296`, `:18886`, `:18968`, `:19046`, `:19066`) | 9 | **NON-carrier** — ADR-0279/0281/0282/0296-family/0317/0318 §Consequences banking, and ADR-0319 §Context ¶5 (the two-moment split, **still TRUE**) | LEFT |
| `internal/tls/config.go:620` | 1 | **NON-CARRIER — verified by reading the whole comment.** Narrates the phase-95 finding-F1 hole in the **PAST** tense (*"was FALSE until the phase-95 final review"*, *"reached THIS function and `quicTLSConfig()` then handed the result to `quic.Listen`"*, *"BOTH listener build sites now route `kindQUIC` to `NewQUICDownstreamConfig`"*). Asserts **NO** precedence between `defaultChain` and `chainByName`. | **LEFT — `internal/tls/**` stays byte-untouched** |
| `internal/listener/manager.go:740` | 1 | **NON-CARRIER — verified by reading the whole comment.** The default-filter-chain parse's `kind == kindQUIC` branch, narrating the same phase-95 F1 hole in the **PAST** tense (*"Before this, the default chain called the TCP builder UNCONDITIONALLY … and `quicTLSConfig()` handed that config straight to `quic.Listen`"*). Asserts **NO** precedence. | **LEFT** (it lives inside an EDITED file and was deliberately not touched) |
| `internal/listener/quic.go` (`:34`, `:67`, `:98`, `:142`, `:184`, `:268`) | 6 | **CURRENT** — code + the Task-9/10/11 rewritten docs | LEFT |
| `internal/listener/quic_test.go` (13 others) | 13 | **CURRENT** — the phase-97 arms' own docs, written at this tip | LEFT |
| `internal/listener/manager_test.go` (5 others: `:1084`, `:1116`, `:1118`, `:1120`, `:1149`) | 5 | **CURRENT** — the phase-95 F1 regression pair; call sites + failure strings, no precedence claim | LEFT |
| `docs/envoy-go/phases/61-http3-downstream-listener/**` | 18 | **HISTORICAL RECORD** of what was true at phase 61 | LEFT |
| `docs/envoy-go/phases/74-tls-handshake-outcome-stats/**` | 2 | **HISTORICAL RECORD** (phase 74) | LEFT |
| `docs/envoy-go/phases/95-tls-alpn-mismatch-fallback/**` | 2 | **HISTORICAL RECORD** (phase 95) | LEFT |
| `docs/envoy-go/phases/96-listener-default-chain-tlsmode/**` | 8 | **HISTORICAL RECORD** (phase 96) | LEFT |
| `docs/envoy-go/phases/97-quic-chain-selection-order/**` | 91 | **THIS ROW's own SPEC/PLAN/BRAINSTORM/PROGRESS** — the record of what was measured when | LEFT |
| `docs/envoy-go/ROADMAP.md` | 3 | rows 74/95/96 narrative — historical, no live precedence assertion | LEFT |
| `next-prompt.txt` | 2 | the session's own lesson text about this very sentence | LEFT |

⚠️ **CONTRADICTION OF THE BRIEF, REPORTED RATHER THAN SILENTLY ABSORBED.** The brief names site 5 as
*"`docs/envoy-go/DECISIONS.md`, ADR-0319 §Context"*. `DECISIONS.md:19020` sits **BELOW** ADR-0318's
header (`:18944`) and **ABOVE** ADR-0319's (`:19052`), inside `### Consequences (landed at the
phase-96 IMPL)` (`:19005`). It is therefore **ADR-0318 §Consequences (b)**, not ADR-0319 §Context —
wrong ADR **and** wrong section. ADR-0319's body (`:19052-19080`) contains **no** precedence sentence
at all; its single `quicTLSConfig` mention (`:19066`, §Context ¶5) states the two-moment split and is
**TRUE at this tip**. The deferral instruction is honoured either way — nothing in `DECISIONS.md` was
touched — but the later task must be told which ADR it is actually editing.

### The three rewrites, before and after

**Site 2 — `internal/listener/manager.go`, `registerListenerMetrics`'s doc.**
BEFORE: *"REPEALED. `quicTLSConfig()` (`quic.go:56-58`) returns `rt.defaultChain.tlsCfg` FIRST,
before consulting `chainByName`, so a QUIC listener with zero `filter_chains[]` and a QUIC-wrapped
`default_filter_chain` satisfied `startQUIC`'s mandatory-TLS check and still built with
`tlsMode == false`, registering none of the five names."*
AFTER: *"REPEALED, and it STAYS repealed: a QUIC listener with zero `filter_chains[]` and a
QUIC-wrapped `default_filter_chain` satisfied `startQUIC`'s mandatory-TLS check and still built with
`tlsMode == false`, registering none of the five names. Only the MECHANISM behind that has changed.
It used to be `quicTLSConfig()`'s resolution order — `rt.defaultChain.tlsCfg` FIRST, then a range
over the `rt.chainByName` MAP. Phase 97 deleted that map range and moved the default slot LAST,
behind a Start-time filter_chain_match selection and then `rt.chainSpecs` in SLICE order;
`rt.chainByName` survives only as a `name -> *chainInfo` lookup, never as an iteration order. Under
the new order that same shape still resolves to the default slot's `tlsCfg` — the Start-time
selection over an EMPTY `chainSpecs` returns the default spec — so `quic.Listen` still receives a
non-nil config, `startQUIC`'s mandatory-TLS reject still does not fire, and the repealed claim is no
more true now than it was then."*

**Site 3 — `internal/listener/quic_test.go`, `TestQUICListener_RegistersSSLNamesAtZero`'s doc.**
BEFORE: *"That is FALSE and is repealed by ADR-0318: `quicTLSConfig()` (`quic.go:56-58`) returns
`rt.defaultChain.tlsCfg` BEFORE consulting `chainByName`, so a QUIC listener … registering NONE of
the five."*
AFTER: *"That is FALSE, is repealed by ADR-0318, and phase 97 does NOT un-repeal it: a QUIC listener
with zero `filter_chains[]` and a QUIC-wrapped `default_filter_chain` satisfied `startQUIC`'s
mandatory-TLS check and still built with `tlsMode == false`, registering NONE of the five. Only the
MECHANISM moved. `quicTLSConfig()` used to try `rt.defaultChain.tlsCfg` BEFORE ranging over the
`rt.chainByName` MAP; it now runs a Start-time filter_chain_match selection first, then
`rt.chainSpecs` in SLICE order, and consults the default slot LAST — the map range is deleted and
`rt.chainByName` is only a `name -> *chainInfo` lookup. For the zero-`filter_chains[]` shape the
Start-time selection returns the default spec anyway, so the default slot's `tlsCfg` still reaches
`quic.Listen` and the repeal holds under BOTH orders. (No `quic.go` line anchor is cited here on
purpose; the previous one rotted.)"*
The surrounding *"THE ASSERTION ABOVE IS CORRECT AND UNCHANGED — five names registered, permanently
zero"* opening is untouched: that is the conclusion, and it survives.

**Site 4 — `internal/listener/manager_test.go`, `mkQUICListenerDefaultChain`'s doc.**
BEFORE: *"… must contribute at least one chain), and `(*listenerRuntime).quicTLSConfig` returns
`rt.defaultChain.tlsCfg` FIRST — so whatever this chain's `transport_socket` builds is what reaches
`quic.Listen`."*
AFTER: *"… must contribute at least one chain), and whatever this chain's `transport_socket` builds
is what reaches `quic.Listen`. ⚠️ THAT CONCLUSION IS UNCHANGED BY PHASE 97; ITS REASON IS NOT. The
reason used to be `(*listenerRuntime).quicTLSConfig`'s defaultChain-FIRST precedence. It is now the
Start-time chain selection: with ZERO `filter_chains[]` the `rt.chainSpecs` slice is empty, so
`SelectChain` finds no eligible indexed chain and returns the default spec, and `quicTLSConfig`
returns that chain's `tlsCfg` from its FIRST step. The default slot now being consulted LAST does not
change the outcome here, because it is the only candidate there is."*

### The comment-only gate on `manager.go` — RUN AGAINST AN INPUT KNOWN TO TRIP IT

`manager.go` was byte-untouched for this entire phase; it moves to the edit roster under a
comment-only constraint. **A gate that has only ever read 0 has not been shown to work**, so it was
run three times:

```sh
git -C /home/esa/git/wt-97-impl diff -- internal/listener/manager.go \
  | /usr/bin/grep -E '^[+-]' | /usr/bin/grep -vE '^(\+\+\+|---)' \
  | /usr/bin/grep -vE '^[+-][[:space:]]*//' | wc -l
```

| reading | tree state | result |
|---|---|---|
| **A** | the site-2 comment edit only | **0** |
| **B** | **POSITIVE CONTROL** — plus one throwaway executable line (`_ = prefix // TASK13 THROWAWAY POSITIVE CONTROL`) inserted after `prefix := "listener." + normalizeAddr(rt.addr) + "."` | **1**, and it printed the offending line |
| **C** | throwaway removed (file restored from a pre-insert copy) | **0** |

The gate DISCRIMINATES. Residue check: `/usr/bin/grep -c 'TASK13 THROWAWAY' internal/listener/manager.go` = **0**.
`git status --porcelain -- internal/tls/` prints **nothing** — `internal/tls/**` is byte-untouched.

### Accessor docs in `quic.go` — confirmed, NOT redone

Both were rewritten at Tasks 10-11 and were re-read here rather than re-edited:

- **(a) two-moment split — PRESENT.** `quicTLSConfig`'s doc: *"IT KEEPS ITS NULLARY SIGNATURE AND IS
  CONNECTION-INDEPENDENT ON PURPOSE. `quic.Listen` demands one config before any connection exists …
  Connection-independence is also what keeps the TWO call moments — `startQUIC`, and the
  `&http3.Server{...}` literal in `serveQUICConnection` — returning the same pointer."* `quicChain`'s
  doc: *"returns the `*chainInfo` that must serve `conn` … `conn == nil` is the Start-time moment."*
- **(b) the "supports exactly one chain" precondition — GONE.** At `eaef171a` (phase 61.2) the
  `quicChain` doc read *"the minimal QUIC slice supports exactly one chain"*; no such precondition
  survives at this tip. **No boot-time chain-count reject was added** — none exists, and adding one
  would refuse configs the pinned reference ACCEPTS and SERVES.
- **(c) PAST tense for the old shape — PRESENT.** *"The previous shape did the opposite — it tried
  the default chain FIRST and then ranged over `rt.chainByName`, a MAP that CONTAINS the default
  slot"*; *"Phase 97 replaced the 61.2 single-chain accessor … the old shape evaluated NO dimension
  of any `filter_chain_match`."*

⚠️ **RESIDUAL STALE PHRASE, REPORTED NOT EDITED (scope discipline).** `startQUIC`'s **own** doc still
opens *"stands a quic-go listener over it (with **the single chain's** `*stdtls.Config`, ALPN h3)"*
(`internal/listener/quic.go:20`). That is a leftover of the 61.2 single-chain framing and is no longer
accurate — the config now comes from a chain **selection**. It is outside this task's three named
sites, so it was **left**; it wants a line in a later task rather than a silent scope widening.

### Gates

| gate | figure |
|---|---|
| comment-only gate, readings A / B / C | **0 / 1 / 0** |
| `git status --porcelain -- internal/tls/` | **empty** |
| `gofmt -l internal/listener/` | **empty** |
| `go vet ./internal/listener/...` | rc **0** |
| `golangci-lint run ./internal/listener/...` | rc **0** |
| `go build ./...` | rc **0** |
| `go test ./internal/listener/... -count=1 -v` | `PIPESTATUS[0]` = **0** |
| `=== RUN` | **237** |
| anchored FAIL `^(FAIL\|--- FAIL)\|^ *--- FAIL` | **0** |
| anchored panic `^panic:\|^\[signal ` | **0** |

### The eleven `TestQUICChainSelection_*` arms — ROSTER diffed, not counted

The `^func TestQUICChainSelection_` roster (11) and the `^--- PASS:` roster (11) were sorted and
`diff`ed: **identical, empty diff**. By name, all `--- PASS`:

1. `TestQUICChainSelection_ApplicationProtocolsH2DoesNotMatch`
2. `TestQUICChainSelection_ApplicationProtocolsH3Matches`
3. `TestQUICChainSelection_IndexedChainWinsOverDefaultSlot`
4. `TestQUICChainSelection_IneligibleIndexedChainFallsBackToDefaultSlot`
5. `TestQUICChainSelection_IneligibleIndexedChainNoDefaultSlotSelectsNothing`
6. `TestQUICChainSelection_MatchingServerNameSelectsIndexedChain`
7. `TestQUICChainSelection_NonMatchingServerNameFallsBackToDefaultSlot`
8. `TestQUICChainSelection_TLSConfigAndChainAgreeOnOneChain`
9. `TestQUICChainSelection_TLSConfigPrefersIndexedChainOverTLSDefaultSlot`
10. `TestQUICChainSelection_TransportProtocolQUICMatches`
11. `TestQUICChainSelection_TransportProtocolTLSDoesNotMatch`

### ⚠️ What this task does NOT establish

This is a **prose** task. Nothing here is evidence about production behaviour: the suite was already
green before these edits and would be green after any wording. The three edits are comment-only in
every file (the two `_test.go` files are doc comments on a helper and a test), so the identical
`237 / 0 / 0` figures before and after are **expected**, not confirmatory. The Task-12 caveat stands
unchanged: eleven green arms is a statement about the arms.

---

## Task 14 — negative-control roster, rows 1-2 (the selector's own answer)

⚠️ **THIS PACKAGE WAS PREVIOUSLY MEASURED GREEN UNDER A PATCH THAT REVERSES CHAIN SELECTION.** A
passing eleven-arm suite therefore proves nothing on its own. Rows 1-2 are what make the green mean
something.

### Baseline, before any mutation

`go test ./internal/listener/ -count=1 -run 'TestQUICChainSelection' -v` → rc **0**, `=== RUN` **11**,
`no tests to run` **0**, anchored panic gate `^panic:|DATA RACE|SIGSEGV` **0**, all eleven `--- PASS`.

⚠️ **`RUN=11` is the selector-matched proof.** A `-run` selector matching nothing prints
`[no tests to run]` and EXITS 0.

`sha256sum internal/listener/quic.go` =
`c84380d40d83ff43f85adc30fe4f3763d37b8881bff5f2ed8566f56cc913c52c` — captured BEFORE row 1 and
`sha256sum -c`'d after every row.

---

### Row 1 — the selector prefers `rt.defaultChain` first again

**MECHANISM, NAMED BEFORE THE RUN.** Arms (a), (d), (f) and (h) each configure an ELIGIBLE
`filter_chains[0]` alongside a `default_filter_chain` and assert the indexed chain is selected. A
`defaultChain`-first accessor returns the last-resort slot on every one of those four listeners, so
each fires its selection-identity property. Arm (c) has NO default slot, so the mutation falls
through to the `chainByName` map walk and returns the sole — INELIGIBLE — indexed chain where `nil`
is required. Arm (j) is the accessor split: `quicChain()` returns the PLAINTEXT default slot
(`tlsCfg == nil`) while `quicTLSConfig()` is UNTOUCHED by this row and still resolves
`filter_chains[0]`'s config, so the pointer-identity assertion fires. Arms (b), (e), (g), (i) expect
the default slot and receive it — **no mechanism can redden them under this row**, which is exactly
why row 2 exists.

**MUTATION** (`internal/listener/quic.go`, `quicChain`; NEUTRALISE, not revert — the package still
compiles and the selector still runs):

```diff
 func (rt *listenerRuntime) quicChain(conn *quic.Conn) *chainInfo {
-	return rt.selectQUICChain(conn)
+	// NC ROW 1 (temporary): reproduce the pre-fix defaultChain-first accessor.
+	_ = conn
+	if rt.defaultChain != nil {
+		return rt.defaultChain
+	}
+	for _, ci := range rt.chainByName {
+		return ci
+	}
+	return nil
 }
```

That body is copied from the un-fixed tip `e9b204d9:internal/listener/quic.go` (`git show`), not
re-spelled from memory.

**RESULT.** rc **1**, `=== RUN` **11**, `no tests to run` **0**, panic gate **0**.

#### Scored ARM BY ARM against Task 8's un-fixed-tip table

| arm | test | Task 8 (un-fixed tip) | row 1 | reproduces? |
|---|---|---|---|---|
| a | `IndexedChainWinsOverDefaultSlot` | 🔴 RED | 🔴 **FAIL** | ✅ |
| b | `IneligibleIndexedChainFallsBackToDefaultSlot` | 🟢 GREEN | 🟢 **PASS** | ✅ |
| c | `IneligibleIndexedChainNoDefaultSlotSelectsNothing` | 🔴 RED | 🔴 **FAIL** | ✅ |
| d | `TransportProtocolQUICMatches` | 🔴 RED | 🔴 **FAIL** | ✅ |
| e | `TransportProtocolTLSDoesNotMatch` | 🟢 GREEN | 🟢 **PASS** | ✅ |
| f | `ApplicationProtocolsH3Matches` | 🔴 RED | 🔴 **FAIL** | ✅ |
| g | `ApplicationProtocolsH2DoesNotMatch` | 🟢 GREEN | 🟢 **PASS** | ✅ |
| h | `MatchingServerNameSelectsIndexedChain` | 🔴 RED | 🔴 **FAIL** | ✅ |
| i | `NonMatchingServerNameFallsBackToDefaultSlot` | 🟢 GREEN | 🟢 **PASS** | ✅ |
| j | `TLSConfigAndChainAgreeOnOneChain` | 🔴 RED | 🔴 **FAIL** | ✅ |
| k | `TLSConfigPrefersIndexedChainOverTLSDefaultSlot` | *(did not exist)* | 🟢 **PASS** | n/a |

**RED = {a, c, d, f, h, j} — SIX. GREEN = {b, e, g, i} — FOUR.** The Task 8 table is reproduced
**arm for arm**, and each RED arm fired **BOTH** of its properties — **twelve assertions**, the same
count Task 8 recorded. Arm (k) is green because `quicTLSConfig()` is not touched by this row.

⚠️ Arm (h)'s status-222 and `downstream_cx_total` properties did NOT fire — a clean SELECTION
failure, not a transport failure. Same as Task 8.

#### Firing assertions, verbatim, per RED arm

| arm | firing assertions (line, message) |
|---|---|
| a | `:393` `selection identity: quicChain() = 0x27fe6a836330, want filter_chains[0] = 0x27fe6a7cdd10 (an empty-match indexed chain is eligible for every connection and must be selected)` · `:401` `last-resort ordering: quicChain() returned the default_filter_chain 0x27fe6a836330, but default_filter_chain is the LAST-RESORT slot and must not pre-empt the eligible filter_chains[0] 0x27fe6a7cdd10` |
| c | `:633` `no-match selects nothing: quicChain() = 0x27fe6a850870, want nil (filter_chains[0] names destination_port 65000 against listener address "127.0.0.1:0" and there is no default_filter_chain, so NO chain is selectable)` · `:642` `ineligible chain is not the fallback: quicChain() returned filter_chains[0] 0x27fe6a850870, whose filter_chain_match names destination_port 65000 — with no default_filter_chain the absence of an eligible chain must yield nil, which is what makes serveQUICConnection close the connection` |
| d | `:731` `transport_protocol match selects indexed: quicChain() = 0x27fe6a851530, want filter_chains[0] = 0x27fe6a851200 (a QUIC connection's transport protocol is "quic", which this chain names, so the chain is eligible)` · `:738` `eligible chain pre-empts last resort: quicChain() returned the default_filter_chain 0x27fe6a851530, but filter_chains[0] 0x27fe6a851200 names transport_protocol "quic" and is eligible, so the last-resort slot must not be consulted` |
| f | `:926` `ALPN match selects indexed: quicChain() = 0x27fe6a97c810, want filter_chains[0] = 0x27fe6a97c510 (a connection reaching the QUIC serve path negotiated "h3", which this chain names, so the chain is eligible)` · `:933` `eligible chain pre-empts last resort: quicChain() returned the default_filter_chain 0x27fe6a97c810, but filter_chains[0] 0x27fe6a97c510 names application_protocols ["h3"] and is eligible, so the last-resort slot must not be consulted` |
| h | `:1306` `SNI selection: body = "DFC\n", want "FC0\n" — the ClientHello carried server_name "alpha.envoy-go.test" which matches filter_chains[0].filter_chain_match.server_names, so the eligible indexed chain must serve, not the last-resort default slot` · `:1313` `last-resort ordering: the default_filter_chain served (body "DFC\n") while filter_chains[0] was ELIGIBLE for this connection (server_names matched SNI "alpha.envoy-go.test"); default_filter_chain is consulted only when no indexed chain is eligible` |
| j | `:1539` `accessor identity: quicTLSConfig() = 0x27fe6ab84d20 but quicChain()'s chain carries tlsCfg 0x0 — the connection would be TERMINATED with one chain's TLS config and SERVED by another chain's filters (filter_chains[0].tlsCfg=0x27fe6ab84d20, default_filter_chain.tlsCfg=0x0)` · `:1548` `selection identity: quicChain() = 0x27fe6a6e7710, want filter_chains[0] = 0x27fe6a6e7680 (it is empty-match and therefore eligible for every connection; default_filter_chain 0x27fe6a6e7710 is the LAST-RESORT slot)` |

⚠️ **THE LINE ANCHORS MOVED, THE ARMS DID NOT.** Task 8 cited a=`:385/:393`, c=`:625/:634`,
d=`:723/:730`, f=`:918/:925`, h=`:1298/:1305`, j=`:1531/:1540`. Every one is **+8** here, uniformly.
The cause is MEASURED, not guessed: `git diff e9b204d9 HEAD -- internal/listener/quic_test.go` puts
its first hunk at `@@ -236,13 +236,21 @@ func TestQUICListener_ServesH3GET` — a **+8 net insert above
every arm** — and no other hunk lands before arm (j)'s assertions. (Arm (k), added at Task 12, sits
at the END of the file and shifts nothing.) **The comparison above is by ARM NAME and MESSAGE TEXT,
never by line number** — a banded line shift is not an arm change.

**RESTORE.** `sha256sum -c` → `internal/listener/quic.go: OK`. Re-run of the eleven arms: rc **0**,
`=== RUN` **11**, all `--- PASS`.

---

### Row 2 — the selector prefers `rt.chainSpecs[0]` unconditionally

⚠️ **THIS ROW IS THE ONLY FALSIFIABILITY ARMS (b), (e), (g) AND (i) HAVE.** They are green at the
un-fixed tip by COINCIDENCE — the pre-fix subject answered "default slot" everywhere and those four
arms happen to expect the default slot. Row 2 is the matched negative that rules the constant out.

**MECHANISM, NAMED BEFORE THE RUN.** Arms (b), (c), (e) and (g) each carry an INELIGIBLE
`filter_chains[0]` (`destination_port: 65000`, `transport_protocol: "tls"`,
`application_protocols: ["h2"]`) and require the selector to REJECT it — (b), (e), (g) onto the
default slot, (c) onto `nil` because there is no default slot. An accessor that answers
`chainSpecs[0]` unconditionally hands back the very chain whose match block excludes the connection,
firing each arm's "ineligibility honored" property. Arm (i) is DRIVEN with a NON-matching SNI, so the
same constant answer serves `FC0\n` where `DFC\n` is required. Arms (a), (d), (f), (h), (j), (k) all
expect `filter_chains[0]` to win, so the constant AGREES with them — **no mechanism can redden
them**, and their staying green is the point of the row, not a gap in it.

**MUTATION** (`internal/listener/quic.go`, `quicChain`):

```diff
 func (rt *listenerRuntime) quicChain(conn *quic.Conn) *chainInfo {
+	// NC ROW 2 (temporary): answer chainSpecs[0] unconditionally.
+	if len(rt.chainSpecs) > 0 {
+		if ci := rt.chainByName[rt.chainSpecs[0].Name]; ci != nil {
+			return ci
+		}
+	}
 	return rt.selectQUICChain(conn)
 }
```

**RESULT.** rc **1**, `=== RUN` **11**, `no tests to run` **0**, panic gate **0**.

| arm | row 2 | required by roster | firing assertion |
|---|---|---|---|
| a | 🟢 PASS | — | — |
| **b** | 🔴 **FAIL** | **must redden** ✅ | `:540` `fallback identity: quicChain() = 0x2c38ce0cced0, want default_filter_chain = 0x2c38ce0cd380 (filter_chains[0] names destination_port 65000, which this listener is not bound to, so no indexed chain is eligible and the last-resort slot must be selected)` · `:549` `ineligibility honored: quicChain() returned filter_chains[0] 0x2c38ce0cced0, but its filter_chain_match names destination_port 65000 and the listener's address is "127.0.0.1:0" — an ineligible chain must never be selected` |
| **c** | 🔴 **FAIL** | **must redden** ✅ | `:633` `no-match selects nothing: quicChain() = 0x2c38ce0e6870, want nil (…)` · `:642` `ineligible chain is not the fallback: quicChain() returned filter_chains[0] 0x2c38ce0e6870, whose filter_chain_match names destination_port 65000 — …` |
| d | 🟢 PASS | — | — |
| **e** | 🔴 **FAIL** | *(bonus)* | `:809` `transport_protocol mismatch falls back: quicChain() = 0x2c38ce0e7a70, want default_filter_chain = 0x2c38ce0e7da0 (a QUIC connection's transport protocol is "quic", not "tls", and the comparison is exact and case-sensitive)` · `:816` `non-matching chain not selected: … names transport_protocol "tls" — a QUIC connection is "quic" and there is no reverse wildcard, so this chain must be ineligible` |
| f | 🟢 PASS | — | — |
| **g** | 🔴 **FAIL** | *(bonus)* | `:1017` `ALPN mismatch falls back: quicChain() = 0x2c38cdccad50, want default_filter_chain = 0x2c38cdccb050 (this listener offers only "h3", so no connection negotiated "h2" and filter_chains[0] is ineligible)` · `:1024` `non-matching chain not selected: … names application_protocols ["h2"] — the negotiated protocol on this listener is always "h3", so this chain must be ineligible` |
| h | 🟢 PASS | — | — |
| **i** | 🔴 **FAIL** | *(bonus)* | `:1405` `SNI fall-through: body = "FC0\n", want "DFC\n" — the ClientHello carried server_name "other.envoy-go.test", which matches none of filter_chains[0].filter_chain_match.server_names ["alpha.envoy-go.test"], so the indexed chain is ineligible and the last-resort default_filter_chain must serve` · `:1410` `ineligible chain served: body = "FC0\n" — filter_chains[0] requires SNI "alpha.envoy-go.test" and the connection presented "other.envoy-go.test"; a chain whose server_names do not match must never be selected` |
| j | 🟢 PASS | — | — |
| k | 🟢 PASS | — | — |

**RED = {b, c, e, g, i} — FIVE.** Both roster targets (b, c) reddened; **no finding**. The three
bonus reds are the falsifiability arms (e), (g) and (i) had never been given, delivered here.

⚠️ **THE PAIR OF ROWS IS WHAT CLOSES THE CONSTANT-ANSWER HOLE.** Row 1's constant ("default slot")
reddens {a,c,d,f,h,j} and leaves {b,e,g,i} green; row 2's constant ("`chainSpecs[0]`") reddens
{b,c,e,g,i} and leaves {a,d,f,h,j,k} green. **Every one of the eleven arms is red under at least one
of the two rows** — union = {a,b,c,d,e,f,g,h,i,j} = ten of eleven; only arm (k) is red under
NEITHER, which is expected and already covered: (k) is the `quicTLSConfig()` ORDERING arm and NC row
10 (Task 12) reddened it alone. No arm in this file is unfalsifiable.

**RESTORE.** `sha256sum -c` → `internal/listener/quic.go: OK`. Re-run of the eleven arms: rc **0**,
`=== RUN` **11**, all `--- PASS`.

---

## Task 15 — negative-control roster, rows 3-5 (the three chain-match INPUTS)

Rows 1-2 falsified the selector's *answer*. Rows 3-5 falsify the three INPUTS the answer is computed
from: each drops exactly one field of `listenerfilter.ChainMatchInputs` in `quicChainMatchInputs` and
must redden the arm whose chain names that dimension while leaving the arm whose chain names a
NON-matching value of the SAME dimension green. **That second half is what separates "the dimension
is read" from "the chain is ineligible for some other reason."**

`sha256sum internal/listener/quic.go` =
`c84380d40d83ff43f85adc30fe4f3763d37b8881bff5f2ed8566f56cc913c52c`, `sha256sum -c`'d after each row.

---

### Row 3 — drop `TransportProtocol: "quic"` from the inputs

**MECHANISM, NAMED BEFORE THE RUN.** `matches` compares
`c.TransportProtocol != "" && c.TransportProtocol != inputs.TransportProtocol` and has **NO REVERSE
WILDCARD**: an empty *inputs* value is a value nothing equals, not a wildcard. Arm (d)'s
`filter_chains[0]` spells `transport_protocol: "quic"`, so with the input blank the chain flips from
ELIGIBLE to INELIGIBLE and `SelectChain` hands back the last-resort default slot where the indexed
chain is required. Arm (e)'s chain spells `"tls"` — it was ineligible against `"quic"` and stays
ineligible against `""`, so its expected answer (the default slot) is unchanged and **no mechanism
can redden it**. Nothing else in the roster names `transport_protocol`.

**MUTATION** (`internal/listener/quic.go`, `quicChainMatchInputs`; the `_ =` keeps the constant used
so the package still COMPILES — neutralise, never revert):

```diff
 func (rt *listenerRuntime) quicChainMatchInputs(conn *quic.Conn) listenerfilter.ChainMatchInputs {
+	// NC ROW 3 (temporary): TransportProtocol input DROPPED.
 	inputs := listenerfilter.ChainMatchInputs{
-		TransportProtocol:    quicTransportProtocol,
 		ApplicationProtocols: []string{quicApplicationProtocol},
 	}
+	_ = quicTransportProtocol
```

**RESULT.** rc **1**, `=== RUN` **11**, `no tests to run` **0**, panic gate **0**.
**RED = {d} — exactly one.** Every other arm `--- PASS`, arm (e) included.

| arm | required | observed | firing assertion |
|---|---|---|---|
| **d** | **must REDDEN** | 🔴 **FAIL** ✅ | `:731` `transport_protocol match selects indexed: quicChain() = 0x389c811a8660, want filter_chains[0] = 0x389c811a8000 (a QUIC connection's transport protocol is "quic", which this chain names, so the chain is eligible)` · `:738` `eligible chain pre-empts last resort: quicChain() returned the default_filter_chain 0x389c811a8660, but filter_chains[0] 0x389c811a8000 names transport_protocol "quic" and is eligible, so the last-resort slot must not be consulted` |
| **e** | **must STAY GREEN** | 🟢 **PASS** ✅ | — |
| a, b, c, f, g, h, i, j, k | — | 🟢 PASS | — |

⚠️ **BOTH HALVES RECORDED.** The pair did NOT both redden — had it, the row would not be isolating
`transport_protocol` and that would be a finding. It is not.

**RESTORE.** `sha256sum -c` → `internal/listener/quic.go: OK`; re-run rc **0**, `=== RUN` **11**,
`--- PASS` **11**.

---

### Row 4 — drop `ApplicationProtocols: ["h3"]` from the inputs

**MECHANISM, NAMED BEFORE THE RUN.** Same asymmetry, one dimension over: an EMPTY inputs list makes
every chain that names any `application_protocols` ineligible. Arm (f)'s chain names `["h3"]` and
flips to INELIGIBLE, so the default slot is selected where the indexed chain is required. Arm (g)'s
chain names `["h2"]`, which this listener never negotiates (`NextProtos` is exactly `["h3"]`, asserted
as arm (g)'s own tripwire precondition) — it was ineligible before and stays ineligible, so its
expected answer is unchanged and **no mechanism can redden it**.

**MUTATION:**

```diff
 func (rt *listenerRuntime) quicChainMatchInputs(conn *quic.Conn) listenerfilter.ChainMatchInputs {
+	// NC ROW 4 (temporary): ApplicationProtocols input DROPPED.
 	inputs := listenerfilter.ChainMatchInputs{
-		TransportProtocol:    quicTransportProtocol,
-		ApplicationProtocols: []string{quicApplicationProtocol},
+		TransportProtocol: quicTransportProtocol,
 	}
+	_ = quicApplicationProtocol
```

**RESULT.** rc **1**, `=== RUN` **11**, `no tests to run` **0**, panic gate **0**.
**RED = {f} — exactly one.**

| arm | required | observed | firing assertion |
|---|---|---|---|
| **f** | **must REDDEN** | 🔴 **FAIL** ✅ | `:926` `ALPN match selects indexed: quicChain() = 0x3ecc0d6840, want filter_chains[0] = 0x3ecc0d6540 (a connection reaching the QUIC serve path negotiated "h3", which this chain names, so the chain is eligible)` · `:933` `eligible chain pre-empts last resort: quicChain() returned the default_filter_chain 0x3ecc0d6840, but filter_chains[0] 0x3ecc0d6540 names application_protocols ["h3"] and is eligible, so the last-resort slot must not be consulted` |
| **g** | **must STAY GREEN** | 🟢 **PASS** ✅ | — |
| a, b, c, d, e, h, i, j, k | — | 🟢 PASS | — |

**RESTORE.** `sha256sum -c` → `internal/listener/quic.go: OK`; re-run rc **0**, `=== RUN` **11**,
`--- PASS` **11**.

---

### Row 5 — drop `ServerName` from the PER-CONNECTION inputs

**MECHANISM, NAMED BEFORE THE RUN.** `inputs.ServerName = conn.ConnectionState().TLS.ServerName` is
the only line that reads the SNI, and it sits on the `conn != nil` branch. Arm (h) is DRIVEN: its
`filter_chains[0]` names `server_names: ["alpha.envoy-go.test"]`, the client sends exactly that name,
and with the input blank the chain is INELIGIBLE, so `serveQUICConnection` draws the default slot and
the response body is `DFC\n` where `FC0\n` is required. Arm (i) is driven with a NON-matching name
(`other.envoy-go.test`): its chain was already ineligible and a blank name leaves it ineligible, so
its expected answer (the default slot) is unchanged. **That is precisely why arm (h) is the
discriminating half — arm (i) passes under a blank name too.**

⚠️ **THE MUTATION MUST NOT BREAK THE HANDSHAKE.** Only the INPUTS struct is touched; the client's
TLS config is not, so the SNI is still transmitted on the wire and is simply not read. This is
confirmed by WHICH assertions fired: arm (h)'s status-222 property (`chain terminal reached`) and its
`downstream_cx_total` property did **NOT** fire — `/usr/bin/grep -c 'chain terminal reached'` over the
row-5 log = **0**. A clean SELECTION failure, not a transport or handshake failure.

**MUTATION:**

```diff
-	inputs.ServerName = conn.ConnectionState().TLS.ServerName
+	// NC ROW 5 (temporary): ServerName input DROPPED. The client's TLS config
+	// is UNTOUCHED — the SNI is still transmitted, it is simply not read into
+	// the chain-match inputs.
+	_ = conn.ConnectionState().TLS.ServerName
 	return inputs
```

**RESULT.** rc **1**, `=== RUN` **11**, `no tests to run` **0**, panic gate **0**.
**RED = {h} — exactly one.**

| arm | required | observed | firing assertion |
|---|---|---|---|
| **h** | **must REDDEN** | 🔴 **FAIL** ✅ | `:1306` `SNI selection: body = "DFC\n", want "FC0\n" — the ClientHello carried server_name "alpha.envoy-go.test" which matches filter_chains[0].filter_chain_match.server_names, so the eligible indexed chain must serve, not the last-resort default slot` · `:1313` `last-resort ordering: the default_filter_chain served (body "DFC\n") while filter_chains[0] was ELIGIBLE for this connection (server_names matched SNI "alpha.envoy-go.test"); default_filter_chain is consulted only when no indexed chain is eligible` |
| **i** | **must STAY GREEN** | 🟢 **PASS** ✅ | — |
| a, b, c, d, e, f, g, j, k | — | 🟢 PASS | — |

**RESTORE.** `sha256sum -c` → `internal/listener/quic.go: OK`; re-run rc **0**, `=== RUN` **11**,
`--- PASS` **11**.

---

### What rows 3-5 establish, and what they do NOT

Each of the three dimensions `transport_protocol`, `application_protocols` and `server_names` is
**READ** by the selector: removing its input, and nothing else, reddens the arm that depends on it
and NO other arm. Three matched negatives, three isolated reds, three confirmed-green companions.

⚠️ **WHAT THEY DO NOT ESTABLISH.** Rows 3 and 4 mutate constants stamped at **BOTH** call moments
(the `conn == nil` Start-time moment and the per-connection one), and their target arms (d) and (f)
are `quicChain(nil)` arms. So rows 3-5 prove three FIELDS are read; **nothing here proves the inputs
struct is built on the CONNECTION path at all.** That is exactly the hole NC row 9 (Task 16) is for.

---

## Task 16 — negative-control roster, row 7, row 9, and the row-6 DELETION record

`sha256sum internal/listener/quic.go` =
`c84380d40d83ff43f85adc30fe4f3763d37b8881bff5f2ed8566f56cc913c52c`, `sha256sum -c`'d after each row.

---

### Row 6 — DELETED AS VACUOUS. NO ARM WAS BUILT FOR IT.

Row 6 wanted to neutralise Task 9's `rt.addr` resolve MOVE and redden a "port-0 variant of arm (b)".
**That row is refuted by execution earlier in this phase, on two INDEPENDENT grounds, and it is
recorded here as deleted rather than quietly skipped or replaced with a manufactured pass.**

1. **The mutation has no observation window.** `rt.addr = udpConn.LocalAddr().String()` executes on
   BOTH sides of the move — only its ORDER within `startQUIC` changes. Nothing between the two
   candidate positions reads `rt.addr`: the span is `quic.Listen`, `rt.udpConn = udpConn`,
   `rt.quicCloser = ql`. `registerListenerMetrics`, the one `rt.addr` reader in `startQUIC`, already
   sits AFTER both positions, and no chain selection runs while `startQUIC` runs because
   `serveQUICConnection` is reached only from the accept goroutine, launched last.
2. **The exploiting config CANNOT BE WRITTEN.** Exploiting the window would need a chain whose
   `destination_port` EQUALS the OS-picked resolved port — a value that does not exist until the
   bind, and so cannot appear in a listener config `NewManager` already consumed. Nor can the
   pre-resolve state be pinned from the other side: `destination_port: 0` means *unspecified* and
   **SKIPS** the dimension (`chainmatch.go:119`), so no chain can spell "the port is still 0".

⚠️ **A ROSTER ROW WHOSE TARGET ARM DOES NOT EXIST IS A VACUOUS CONTROL**, and the correct action is
to delete the row and say so — not to invent an arm that reddens for some other reason and score it
as row 6. **Row 6 contributes ZERO evidence to this phase.** What Task 9's move *is* pinned by is
stated in the arm-(b) doc comment: restated as a DELETION (never assigning the resolved address at
all) it breaks `TestQUICListener_HandshakeSucceeds` and
`TestQUICListener_RegistersSSLNamesAtZero`, which take their dial target from `Listeners()[0].Addr`.
**No new arm was added for that either, and none is claimed.**

---

### Row 7 — `quicTLSConfig()` returns the default slot unconditionally

**MECHANISM, NAMED BEFORE THE RUN.** Arm (j)'s listener is a TLS-bearing empty-match
`filter_chains[0]` plus a **PLAINTEXT** `default_filter_chain` (`dflt.tlsCfg == nil`, asserted by the
arm's own separator precondition). This row touches `quicTLSConfig()` ONLY, so `quicChain(nil)` still
selects the indexed chain; `quicTLSConfig()` now answers `rt.defaultChain.tlsCfg`, which is **nil**.
Arm (j)'s PROPERTY 1 is pointer identity `quicTLSConfig() == selected.tlsCfg` — `nil` against the
indexed chain's non-nil config — so it fires. **PROPERTY 2 (selection identity) must NOT fire**,
because `quicChain` is untouched; that asymmetry is the row's own discrimination check and it was
verified, not assumed.

**MUTATION** (`internal/listener/quic.go`, `quicTLSConfig`; the `rt.defaultChain != nil` guard is
what keeps a no-default-slot listener from nil-dereferencing, i.e. neutralise, not break):

```diff
 func (rt *listenerRuntime) quicTLSConfig() *stdtls.Config {
+	// NC ROW 7 (temporary): answer the default slot UNCONDITIONALLY.
+	if rt.defaultChain != nil {
+		return rt.defaultChain.tlsCfg
+	}
 	if ci := rt.selectQUICChain(nil); ci != nil && ci.tlsCfg != nil {
 		return ci.tlsCfg
```

**RESULT.** rc **1**, `=== RUN` **11**, `no tests to run` **0**, panic gate **0**.
**RED = {j, k}.**

| arm | required | observed | firing assertion |
|---|---|---|---|
| **j** | **must REDDEN** | 🔴 **FAIL** ✅ | `:1539` `accessor identity: quicTLSConfig() = 0x0 but quicChain()'s chain carries tlsCfg 0x1699004621e0 — the connection would be TERMINATED with one chain's TLS config and SERVED by another chain's filters (filter_chains[0].tlsCfg=0x1699004621e0, default_filter_chain.tlsCfg=0x0)` |
| k | *(not listed by the roster)* | 🔴 **FAIL** | `:1644` `quicTLSConfig ordering: got 0x1699004625a0, want filter_chains[0].tlsCfg 0x1699004623c0 — filter_chains[0] is empty-match and therefore eligible for the Start-time selection, and the last-resort default_filter_chain (tlsCfg 0x1699004625a0) must never pre-empt an eligible indexed chain` · `:1650` `default slot pre-empted: quicTLSConfig() = 0x1699004625a0 = default_filter_chain.tlsCfg — the default slot was consulted BEFORE the Start-time chain selection, which is the pre-Task-11 resolution order this arm exists to exclude` |
| a, b, c, d, e, f, g, h, i | — | 🟢 PASS | — |

⚠️ **ARM (j) FIRED PROPERTY 1 AND *ONLY* PROPERTY 1**, exactly as the mechanism predicted:
`/usr/bin/grep -c 'selection identity' ` over the row-7 log = **0**. `quicChain` was untouched, so the
chain that serves is still `filter_chains[0]`; only the TLS config diverged. **A row that reddened
both properties would have been reddening for the wrong reason and is a finding — this one is not.**

⚠️ **ARM (k) ALSO REDDENED, AND THIS IS DECLARED RATHER THAN GLOSSED.** Row 7 lists no
"must stay green" companion, so (k)'s red is not a roster contradiction — but it is a real overlap
worth naming: (k)'s listener carries TLS in BOTH slots with DISTINCT configs, so a default-first
`quicTLSConfig()` returns the wrong pointer there too. **Row 7 is therefore NOT an isolating control
for arm (j) alone.** The isolating control for (k) is NC row 10 (Task 12), which reddened (k) and
nothing else. Row 7's independent contribution is arm (j)'s PROPERTY-1-only red.

⚠️ Arms (h) and (i) stand up a LIVE listener and complete a real handshake under this row and stayed
green: with a TLS-bearing default slot the wrong-but-usable config still terminates the connection,
and the SERVING chain comes from `quicChain(conn)`, which this row does not touch.

**RESTORE.** `sha256sum -c` → `internal/listener/quic.go: OK`; re-run rc **0**, `=== RUN` **11**,
`--- PASS` **11**.

---

### Row 9 — `quicChainMatchInputs` returns the ZERO struct when `conn != nil`

**MECHANISM, NAMED BEFORE THE RUN — AND IT CONTRADICTS THE ROSTER AS WRITTEN.**

The roster row says *"must REDDEN **a, d, f, h**; must STAY GREEN **b, c, e, g, i**."* **That is
wrong for a, d and f, and the mechanism says so before any command is run.** Arms (a), (d) and (f)
call `rt.quicChain(nil)` on a **never-Started** runtime. The mutation is gated on `conn != nil`, so
on those three arms the executed code path is **byte-for-byte the unmutated one**. There is no
mechanism by which a construction-time arm can observe a mutation that only fires when a connection
exists. **Naming a mechanism was required BEFORE running the row, and for a, d and f no mechanism
exists — those three cells of the row are VACUOUS.**

The roster has exactly **TWO DRIVEN arms**, (h) and (i). Of those, (h) is the positive: its chain
names `server_names: ["alpha.envoy-go.test"]`, and a zero inputs struct blanks the SNI, so the chain
is INELIGIBLE and the default slot serves `DFC\n` where `FC0\n` is required. Arm (i) is driven with a
non-matching name and already expects the default slot, so it stays green. **Prediction, recorded
before the run: RED = {h}, and h ALONE.**

**MUTATION:**

```diff
 func (rt *listenerRuntime) quicChainMatchInputs(conn *quic.Conn) listenerfilter.ChainMatchInputs {
+	if conn != nil {
+		// NC ROW 9 (temporary): the PER-CONNECTION builder returns the ZERO
+		// struct. The Start-time (conn == nil) moment is untouched.
+		return listenerfilter.ChainMatchInputs{}
+	}
 	inputs := listenerfilter.ChainMatchInputs{
 		TransportProtocol:    quicTransportProtocol,
```

**RESULT.** rc **1**, `=== RUN` **11**, `no tests to run` **0**, panic gate **0**.
**RED = {h} — exactly one, and the prediction is confirmed.**

| arm | roster says | observed | verdict |
|---|---|---|---|
| **a** | must REDDEN | 🟢 **PASS** | 🔴 **ROSTER CONTRADICTED — the cell is VACUOUS** (nil-conn arm) |
| b | stay green | 🟢 PASS | ✅ (nil-conn arm; could not have reddened either) |
| c | stay green | 🟢 PASS | ✅ (nil-conn arm) |
| **d** | must REDDEN | 🟢 **PASS** | 🔴 **ROSTER CONTRADICTED — VACUOUS** (nil-conn arm) |
| e | stay green | 🟢 PASS | ✅ (nil-conn arm) |
| **f** | must REDDEN | 🟢 **PASS** | 🔴 **ROSTER CONTRADICTED — VACUOUS** (nil-conn arm) |
| g | stay green | 🟢 PASS | ✅ (nil-conn arm) |
| **h** | must REDDEN | 🔴 **FAIL** | ✅ the ONE real target |
| i | stay green | 🟢 PASS | ✅ genuine matched negative — DRIVEN, and blank-SNI-tolerant by design |
| j | — | 🟢 PASS | (nil-conn arm) |
| k | — | 🟢 PASS | (nil-conn arm) |

Arm (h)'s firing assertions: `:1306` `SNI selection: body = "DFC\n", want "FC0\n" — the ClientHello
carried server_name "alpha.envoy-go.test" which matches
filter_chains[0].filter_chain_match.server_names, so the eligible indexed chain must serve, not the
last-resort default slot` · `:1313` `last-resort ordering: the default_filter_chain served (body
"DFC\n") while filter_chains[0] was ELIGIBLE for this connection (server_names matched SNI
"alpha.envoy-go.test"); default_filter_chain is consulted only when no indexed chain is eligible`.

**THE ROW IS REPORTED AS RUN, NOT ADJUSTED TO FIT.** Per the brief's own instruction, the difference
is explained from the mechanism: rows 3 and 4 mutate constants stamped at BOTH moments, which is why
they could reach the nil-conn arms (d) and (f); row 9 is gated on `conn != nil` and therefore cannot.

#### ⚠️ WHAT ROW 9 ACTUALLY ESTABLISHES — AND THE COVERAGE HOLE IT EXPOSES

- It **does** prove the per-connection inputs struct is BUILT AND READ on the connection path: zero it
  out, and a live handshake selects a different chain. That claim was previously supported by
  nothing.
- It is **observationally IDENTICAL to row 5** — both redden {h} and only {h}. **The entire
  connection-path story in this roster rests on ONE arm.** Row 9 is not redundant in intent (row 5
  drops one field, row 9 zeroes the whole struct) but it adds no new arm-level discrimination.
- ⚠️ **`DestinationIP` / `DestinationPort` / `SourceIP` / `SourcePort` are zeroed by this row and NO
  ARM NOTICED.** The only arms naming `destination_port` are (b) and (c), and both are
  `quicChain(nil)` arms. **On the CONNECTION path those four inputs are entirely unpinned** — a
  future defect in `conn.LocalAddr()` / `conn.RemoteAddr()` extraction would be invisible to all
  eleven arms. Recorded as a measured gap, not repaired here (adding arms is not this task's scope).

**RESTORE.** `sha256sum -c` → `internal/listener/quic.go: OK`; re-run rc **0**, `=== RUN` **11**,
`--- PASS` **11**.

---

## The negative-control roster, closed out

| # | mutation | roster target | OBSERVED RED | companions confirmed GREEN | digest restored |
|---|---|---|---|---|---|
| 1 | `quicChain` prefers `rt.defaultChain` first again | a | **a, c, d, f, h, j** (Task 8 reproduced arm for arm, 12 assertions) | b, e, g, i, k | ✅ OK |
| 2 | `quicChain` answers `chainSpecs[0]` unconditionally | b, c | **b, c, e, g, i** | a, d, f, h, j, k | ✅ OK |
| 3 | `TransportProtocol` input dropped | d | **d** | **e** ✅ | ✅ OK |
| 4 | `ApplicationProtocols` input dropped | f | **f** | **g** ✅ | ✅ OK |
| 5 | per-connection `ServerName` dropped | h | **h** | **i** ✅ | ✅ OK |
| 6 | *(resolve-move)* | — | **DELETED AS VACUOUS — not run, no arm built** | — | — |
| 7 | `quicTLSConfig` answers the default slot unconditionally | j | **j** (property 1 only) **+ k** | a-i | ✅ OK |
| 9 | `quicChainMatchInputs` zero-struct on `conn != nil` | a, d, f, h | **h ONLY** — a/d/f cells VACUOUS, roster CONTRADICTED | b, c, e, g, i (+ a, d, f, j, k) | ✅ OK |

Rows 8 and 10 are out of scope here: row 10 ran at Task 12 (reddened arm (k) alone); row 8 is a
differential-fixture control landing later.

**EVERY ARM IS FALSIFIABLE.** Union over every control this phase has RUN: (a) row 1 · (b) row 2 · (c) rows 1, 2 ·
(d) rows 1, 3 · (e) row 2 · (f) rows 1, 4 · (g) row 2 · (h) rows 1, 5, 9 · (i) row 2 · (j) rows 1, 7 ·
(k) rows 7, 10. **No arm in this file is green under every control.**

### Final gates, at the fully restored tree

| gate | figure |
|---|---|
| `go test ./internal/listener/... -count=1 -v`, `PIPESTATUS[0]` | **0** |
| `=== RUN` | **237** |
| anchored FAIL `^(FAIL\|--- FAIL)\|^ *--- FAIL` | **0** |
| anchored panic `^panic:\|DATA RACE\|SIGSEGV` | **0** |
| `no tests to run` | **0** |
| `gofmt -l internal/listener/` | **empty** |
| `go vet ./internal/listener/...` | rc **0** |
| `golangci-lint run ./internal/listener/...` | rc **0** |
| eleven-arm ROSTER diff (declared `^func TestQUICChainSelection_` vs `^--- PASS:`) | **11 / 11, EMPTY DIFF** |
| NC residue `/usr/bin/grep -cE 'NC ROW' internal/listener/quic.go` | **0** |
| `git status --porcelain` (before the PROGRESS.md edit) | **EMPTY** |

⚠️ The ROSTER was diffed by NAME, not counted — a `+0/+0` arm deletion passes every counter above.

## Tasks 17 and 18 — fixture `0122-quic-chain-selection`

⚠️ **THESE TWO SECTIONS WERE MISSING AND WERE BACK-FILLED AT THE CLOSE.** Tasks 17 and 18 landed as
commits `3ac24ee0` and `c05eee93` but wrote no `PROGRESS.md` section, so their evidence lived only in
commit messages. The Task 19 agent caught the omission and reported it. **Recorded here rather than left
— but the back-fill is stated as a back-fill, because a section written at the close is not the same
artefact as one written at the task.**

**Shape.** Three files, no `.yaml` bootstraps and no `pki/` — the `0104-http3-downstream-get` shape, both
bootstraps inline as `const referenceTmpl` / `const subjectTmpl`, cert and key delivered `inline_string:`
from the existing `testAlphaCertPEM` / `testAlphaKeyPEM` pair. ONE UDP/QUIC listener `l_qcs`:
`filter_chains[0]` named `fc_indexed` carrying **no `filter_chain_match` key at all**, its own
`envoy.transport_sockets.quic` socket, HCM `stat_prefix: chain_indexed`, `direct_response` **222**
`"chain-indexed\n"`; plus a `default_filter_chain` named `fc_default` with its OWN quic socket, HCM
`stat_prefix: chain_default`, `direct_response` **222** `"chain-default\n"`.

🔴 **THE TWO `stat_prefix` VALUES DIFFER, AND THAT IS LOAD-BEARING.** Two HCMs sharing one prefix in a
single listener across two chains panic the process at boot on `duplicate metric registration`. **The
fixture sits ONE IDENTIFIER away from a banked defect**, and the README says so.

**Reference port `15122`**, free at this tip. Registered name byte-identity proven by hashing the
extracted `const fixtureName` against the directory basename — **both `b4bd5937d1374e1e…`**, `cmp` EQUAL.

### The `== 0` pin is REAL, not vacuous — MEASURED before it was written

⚠️ **A `== 0` PIN ON A NAME THE SUBJECT NEVER EMITS IS SILENTLY VACUOUS, NOT RED** — a scrape map returns
the zero value for a missing key. Both sides were scraped FIRST, the reference run BY DIGEST with the
digest verified against `ENVOY_TARGET.md` lines 3-4:

| side | `http.chain_indexed.downstream_rq_total` | `http.chain_default.downstream_rq_total` |
|---|---|---|
| reference | **1** | **0**, PRESENT |
| subject | **1** | **0**, PRESENT |

⇒ **present-at-zero on both sides, not absent.** The pin stands as written. Each pin additionally carries
an explicit PRESENCE assertion, so a side that stops emitting the scope reddens on absence rather than
passing silently. An **inverting control** fired all four value assertions.

**Scope figures:** reference `http.chain_*` **156** lines against the subject's **10**; the reference also
emits a listener-qualified scope of **12** lines the subject does not emit at all; and the listener address
token differs cross-side, so any assertion keyed on the full name is cross-side infeasible. **A NAMED
SUBSET IS THE ONLY VIABLE PIN.**

### 🔴 CONTRADICTION: the port-census claim is true only when scoped to PORTS

The inherited claim that `15104` is *"the only `15xxx` literal at or above 15100"* is **FALSE as worded**:
the same census reads **two** at or above 15100, and the second is not a port at all but a
`request_*_total_size` **byte figure** in an unrelated fixture. Widening the census to all of `test/` adds
another byte figure. **The PORT-scoped form of the claim holds; the literal-scoped form does not.**


## Task 19 — fixture `0122` ACTIVATED, and the check it would have been scored on is VACUOUS under a rename

The fixture existed and passed at Task 18, but it was **INERT**: the driver's blank import had been added
temporarily, run, and reverted. Measured at the Task-18 tip: `ls -d test/fixtures/*/ | wc -l` = **124**,
extractor over `test/differential/runner_test.go` = **123**, `/usr/bin/grep -c '0122' test/differential/runner_test.go`
= **0**. This task adds the one line that closes the gap, at the block's sorted position (after `0121-…/driver`).

### The four registration gates — and why NONE of them can be scored on the exit code

| gate | what it is | evidence | failure signature |
|---|---|---|---|
| 1 | `fixture.RegisterFixture(fixtureName, &qcsDriver{})` in the driver `init()` | `driver.go:24` `const fixtureName = "0122-quic-chain-selection"`, `driver.go:56` inside `func init()` | `t.Skipf` |
| 2 | blank import in `runner_test.go` | present, 1 occurrence, `gofmt -l` empty | `t.Skipf` |
| 3 | **byte-identity** dir name ⟷ registered string | see below | `t.Skipf` |
| 4 | `NNNN-` / `NNNN<letter>-` shape `discoverFixtures` enumerates | prefix `0122-`; `len>=5`, `isNumeric(name[:4])`, `name[4]=='-'` | **NO SUBTEST AT ALL** |

**Gate 3, proven MECHANICALLY, not by eye.** The registered value was extracted from the Go source
(`sed -n 's/^const fixtureName = "\(.*\)"$/\1/p'`) and compared against the directory basename:

```
registered sha256: b4bd5937d1374e1ee26780d9b191a6c0475b1069bbd613d3f815b32dcfb734f4
dirname    sha256: b4bd5937d1374e1ee26780d9b191a6c0475b1069bbd613d3f815b32dcfb734f4
cmp -> EQUAL
```

⚠️ Gates 1-3 converge on the **same** `t.Skipf` at `runner_test.go:203`; gate 4 emits no subtest line at all.
**A missing gate is indistinguishable from a pass by exit code.** Every gate below is therefore scored on the
FIXTURE-SET SET-DIFFERENCE.

### The extractor, and BOTH `comm` directions

| figure | before | after |
|---|---|---|
| `ls -d test/fixtures/*/ \| wc -l` | 124 | 124 |
| extractor over `runner_test.go` | 123 | **124** |
| `comm -23` (dir present, not imported) | `0122-quic-chain-selection` | **EMPTY** |
| `comm -13` (imported, no dir) | EMPTY | **EMPTY** |
| `/driver"$` imports | 99 | **100** |
| `/inputs"$` imports | 24 | 24 |

### ⚠️ The counter that silently drops two fixtures

`ls -d test/fixtures/*/ | wc -l` = **124** is the correct counter. The plausible `^[0-9]{4}-` character class
reads **122**, dropping exactly `0007a-cors` and `0007b-iteration-probe` — the phase-07.1 split shape that
`discoverFixtures` explicitly accepts via `isLowerLetter`. 124 − 2 = **122**.

> 🔴 **CONTRADICTION OF THE TASK BRIEF, RECORDED.** The brief predicted the character class reads **121**
> while naming exactly two dropped directories. It reads **122**. The brief's figure is inconsistent with its
> own explanation; the measured figure is 122 and the two dropped names are as the brief named them.

### NC-ing the extractor: two controls, firing on DIFFERENT axes

Run on scratch copies of `runner_test.go`, never on the tree.

| control | import count | `comm -23` fires | `comm -13` fires |
|---|---|---|---|
| **RENAME** `0119-grpc-unary-trailers` → `0119-grpc-unary-TRAILERS-XX` | **124 — INVARIANT** | ✅ `0119-grpc-unary-trailers` | ✅ `0119-grpc-unary-TRAILERS-XX` |
| **DELETE** the `0119` import line | 123 | ✅ `0119-grpc-unary-trailers` | ❌ **EMPTY** |

🔴 **A COUNT-ONLY CHECK IS VACUOUS.** Under the rename the count does not move at all — a check that compared
only `124 == 124` would have read GREEN over a roster that no longer matches the directory list. "Both
directions fire" belongs to the RENAME control **alone**; a pure deletion fires `comm -23` only.

### Full differential suite — FOREGROUND, `-count=1`

| figure | value |
|---|---|
| `PIPESTATUS[0]` | **0** |
| wall clock | **404.587s** |
| package-wide `--- PASS` | 141 |
| `TestDifferential/` subtests PASS | **124** |
| `TestDifferential/` subtests FAIL | **0** |
| `TestDifferential/` subtests **SKIP** | **0** |
| ran-set vs directory list, `comm -23` | **EMPTY** |
| ran-set vs directory list, `comm -13` | **EMPTY** |
| `0122-quic-chain-selection` in the ran set, BY NAME | ✅ **PASS (1.77s)** |

**123 → 124, asserted by name in both directions.** A SKIP is the failure mode for gates 1-3, so the skip
count is reported explicitly: **zero fixtures skipped, no skip list.** No recurrence of the reserved-band
startup flake in this run.

## Task 20 — NC roster row 8: the control that is GREEN when it fails

| | |
|---|---|
| **row** | 8 |
| **mutation** | delete the `0122-quic-chain-selection/driver` blank import from `runner_test.go` |
| **mechanism** | the driver package is never linked ⇒ its `init()` never runs ⇒ `fixture.DriverRegistry["0122-quic-chain-selection"]` misses at `runner_test.go:200` ⇒ `t.Skipf` at `:203`. The mutation changes exactly one map key; no other fixture's registration, port or config is touched. |
| **rc** | **0** |
| **verdict** | ✅ OK — **scored on the set-difference, NOT on rc** |

### 🔴 rc IS NOT THE EVIDENCE

Under the mutation the suite printed `PASS` / `ok …` and returned **rc = 0**, with **no FAIL line anywhere**.
That is *exactly* what a correctly-registered fixture also produces. **rc cannot distinguish "0122 ran and
passed" from "0122 was never registered and was silently skipped"** — the skip is a pass as far as the exit
code is concerned. This is the roster row most likely to be mis-scored, and rc is discarded as evidence here.

**What actually discriminates**, two independent witnesses:

1. **Static set-difference** over the extractor, under the mutation: imports **123** vs dirs **124**, and
   `comm -23` names **`0122-quic-chain-selection`**; `comm -13` EMPTY. (Matches the DELETE control's
   signature from Task 19 exactly.)
2. **Live subtest status**: the only trace in the run is
   `--- SKIP: TestDifferential/0122-quic-chain-selection (0.00s)` plus
   `runner_test.go:202: no driver registered for fixture "0122-quic-chain-selection" (driver package not yet blank-imported in runner_test.go)`.
   Selector-footgun check: `no tests to run` does **not** appear, so the `-run` selector genuinely matched — a
   SKIP, not a silent no-match.

**Scope of the row-8 run, stated exactly.** `go test ./test/differential/ -count=1 -v -run 'TestDifferential/0122-quic-chain-selection'`.
Justified because the mutation's mechanism can only alter one registry lookup (above); the narrow run exercises
precisely that lookup, and the static witness covers the roster as a whole.

### ⚠️ The full-suite row-8 attempt was ABORTED by an unrelated known flake — recorded, not re-run to green

The first row-8 attempt was the FULL `TestDifferential`. It returned **rc = 1** with a failure in a fixture
this row cannot touch:

```
panic: driver: start OTLP receiver on 0.0.0.0:41177: listen tcp 0.0.0.0:41177: bind: address already in use
  .../0085-otlp-access-log-operators/driver.(*otlpDriver).ensureServer  driver.go:194
--- FAIL: TestDifferential/0085-otlp-access-log-operators (0.10s)
```

This is the known **driver-receiver port race**: `ensureServer` **panics** on a bind failure rather than
retrying, which **aborts the test binary**. Port **41177 lies inside the ephemeral range**
(`/proc/sys/net/ipv4/ip_local_port_range` = `32768 60999`), i.e. a recurrence **inside the reserved band** —
a **finding**, not noise. The abort masked the row's own subject: the run died at 86 passing fixtures / 254s
and **never reached `0122` at all** (zero `0122` lines in that log). Not a port collision with the sibling
session's `curl-world-*` containers — this is a host-port bind by the Go driver process, not a container
publish. The row was **not re-run until green**; it was re-run at the narrow scope justified above, which does
not start the `0085` receiver.

### Restore

| check | result |
|---|---|
| `runner_test.go` sha256 after restore | `64ace445756e7be71df30ba2fac579343843eb1d40356a9996613728b62ecb3f` |
| byte-identical to the state the full suite ran on | ✅ `cmp` EQUAL |
| `git status --porcelain` for the file | EMPTY |
| extractor imports vs dirs | **124 = 124** |
| `comm -23` / `comm -13` | **BOTH EMPTY** |
| live re-run of `0122` | `--- PASS: TestDifferential/0122-quic-chain-selection (2.03s)`, **0 skips**, rc 0 |

### ⚠️ ARM ROSTER diffed BY NAME, not counted

A `+0/+0` arm deletion passes every counter in this document, so the rosters were set-differenced against the
pre-Task-19 tip `c05eee93`:

| roster | base | head | `comm -23` | `comm -13` |
|---|---|---|---|---|
| `^func TestQUICChainSelection_` in `internal/listener/quic_test.go` | 11 | 11 | EMPTY | EMPTY |
| NC roster rows in this file (rows 1, 2, 3, 4, 5, 6, 7, 9) | 8 | 8 | **`diff` IDENTICAL row-for-row** | — |

`git diff --stat c05eee93..HEAD -- internal/listener/` is **EMPTY** — the eleven arms and the production file
were not touched by either task. Task 19's change-set is `1 0 test/differential/runner_test.go`, a single
inserted line.

> 📝 Recorded in passing: **Tasks 17 and 18 landed no `## Task 17` / `## Task 18` sections in this file.**
> Their evidence lives only in their commit messages (`3ac24ee0`, `c05eee93`).

---

## Task 21 — ADR-0319 completed, and the two carriers a previous agent correctly deferred

**Scope: exactly two files.** `git status --porcelain` names `docs/envoy-go/DECISIONS.md` and
`docs/envoy-go/BEHAVIOR_CONTRACT.md` and nothing else; `internal/**` and `test/**` are byte-untouched.
`go build ./...` rc **0** (no code was touched — the gate is run to prove it, not to discover it).

### Structural figures, BEFORE and AFTER

| figure | BEFORE | AFTER |
|---|---|---|
| `DECISIONS.md` `wc -l` | 19080 | 19213 |
| `^---$` | **216** | **216 — UNMOVED** |
| `^## ADR-` | **318** | **318 — UNMOVED** |
| bare `^## ` | **326** | **326 — UNMOVED** |
| tail ADR | `ADR-0319` | `ADR-0319` |
| next-free, **derived from the TAIL** | `ADR-0320` | `ADR-0320` |
| `BEHAVIOR_CONTRACT.md` `wc -l` | 5991 | 5993 |
| `git diff --numstat` | — | `139 6 DECISIONS.md` · `3 1 BEHAVIOR_CONTRACT.md` |

⚠️ **Next-free is derived from the TAIL heading, never from the heading count** — the id space is sparse at
a single gap, so heading arithmetic yields a TAKEN id. No renumber, no `**Status:**` line, no `---`
separator was added.

### The house `PROPOSED` guard — resolved BY LINE AND BY ADR, in BOTH forms

Both forms were run. Reporting only the second would read as *“the guard is disarmed”* even when it is armed:
they are different matchers over different records.

| matcher | before | resolves to | after | resolves to |
|---|---|---|---|---|
| **house form** `^> \*\*STATUS: PROPOSED` | `:19054` | backward `^## ADR-` ⇒ `:19052` = **ADR-0319** | **no hits** | — |
| **decoy form** `^\*\*Status:\*\* PROPOSED` | `:14866` | backward `^## ADR-` ⇒ `:14864` = **ADR-0231** | `:14866` | `:14864` = **ADR-0231** |

The decoy is a **different matcher entirely**, resolving to a much older record. It is **BYTE-IDENTICAL**
to `HEAD`: `git show HEAD:… | sed -n '14866p'` vs the working line, `cmp` **EQUAL**. Disarming the house
form is what flipping ADR-0319 to ACCEPTED does; the decoy is untouched and stays armed over ADR-0231.

🔴 **No count of either matcher is written in prose the grep matches** — a previous stage falsified itself
doing exactly that.

### ADR-0319 — what landed

`> **STATUS: PROPOSED` → `ACCEPTED` **in place** (and *“to be APPENDED”* → *“APPENDED”*, the ADR-0318 form).
`### Decision (landed at the phase-97 IMPL)` and `### Consequences (landed at the phase-97 IMPL)` appended
**AFTER** the retained italic footer, which is still present at `:19087` and was never replaced.

⚠️ **Reported, not absorbed:** the flip falsifies two *self-referential* clauses inside that same status
line — *“IS RE-ARMED … AND IS DISARMED BY THE PHASE-97 IMPL”* and *“this line is itself a hit of the strict
form”*. Both were re-tensed in the same in-place edit rather than left standing as false present-tense
claims. Nothing else on the line moved.

**§Context ¶4 re-tensed.** ⚠️ **The brief calls this *“the TLS-accessor sentence”*; measured, it is ¶4's
BOTH-accessors mechanism clause.** ADR-0319 §Context contains no precedence sentence at all, and its only
other `quicTLSConfig` mention — ¶5's two-moment split — was checked against `quic.go` (the Start-time call
site and the `&http3.Server{…}` literal both still call it) and is **TRUE at this tip**, so it was left.
What ¶4 said in the present tense (*“both accessors index `rt.chainByName` directly … the fallback loops
range over the union”*) is now false; the re-tense **leads with the diagnosis**, which is unchanged and is
exactly what §Decision repairs, and explicitly refuses the phrasing *“the map is no longer consulted”*.

### The TWO ADR-0318 carriers — attributions RE-VERIFIED here, not taken on trust

Each was resolved by backward `^## ADR-` heading search before being edited:

| carrier | line (pre-edit) | backward heading | verdict |
|---|---|---|---|
| §Context ¶6 | `:18960` | `:18944` = **ADR-0318** | ✅ as briefed |
| §Consequences (b) | `:19020` | `:18944` = **ADR-0318** | ✅ as briefed — 🔴 **and the PLAN's “ADR-0319 §Context” is WRONG on BOTH axes** |

`:19020` sits **below** ADR-0318's heading (`:18944`) and **above** ADR-0319's (`:19052`), inside
`### Consequences (landed at the phase-96 IMPL)`. The Task-13 agent flagged this mis-attribution and
deferred; it is confirmed here by re-measurement rather than by citation.

**Which halves die.** Both carriers asserted, in the present tense, *“`quicTLSConfig()` returns
`rt.defaultChain.tlsCfg` before consulting `chainByName`”*.

- **DIES — the PRECEDENCE.** The default slot is consulted **LAST**, behind (1) the Start-time
  `filter_chain_match` selection and (2) `rt.chainSpecs` in **SLICE** order.
- **DIES — the map-ORDER ITERATION.** Both `for _, ci := range rt.chainByName` loops are gone.
- **SURVIVES — everything ADR-0318 concluded.** Its decision, its diagnosis, the crashing shape, the
  `tlsMode` widening and the boot outcome are all untouched. The outcome holds under **BOTH** orders: with
  an EMPTY `chainSpecs` and a non-nil default spec the Start-time selection returns the default spec at its
  **FIRST** step rather than by fall-through.
- ⚠️ **DELIBERATELY NOT WRITTEN — *“the map is no longer consulted”*.** That would be a NEW false claim.
  `rt.chainByName` survives as a `name -> *chainInfo` **lookup table**.

Both were corrected **in place** in the house style for amending a landed record — the same bracketed
`**[… CORRECTED at the phase-… IMPL: …]**` form ADR-0318 §Context ¶6 and ¶7 already carry. ⚠️ **ADR-0044 is
NOT cited for that discipline**: read in full it is *“BEHAVIOR_CONTRACT HTTP/1.1 subsection”* (`:1419`) and
contains no record-drafting discipline. The real precedent is the shared block form of the recent records.
The line in this file that misattributes it is the ADR-0231 decoy itself, which is out of scope and stays
byte-untouched; **no count of the misattributing lines is quoted**, because any figure would rot.

### `BEHAVIOR_CONTRACT.md` — two edits, one file

**Edit 1 — the ledger chain entry**, appended to `### Stat surface` (`:5071`) immediately after the phase-96
entry, on the **phase-96 `+0, UNCHANGED` form**, NOT the sibling `A → B` form. It quotes **no absolute**:
the only numerals in it are `+0`, ADR ids and phase ordinals (`git`-checked — no `NNNN → NNNN` and no
four-digit stat total appears in the line). The row lands **+0 stat NAMES** and the entry says so plainly:
it changes WHICH chain serves, not which counters exist; `registerListenerMetrics` and the `rt.tlsMode`
gate are byte-untouched. Enforcement is restated as the per-phase delta guards, never a total.

**Edit 2 — the stale mechanism sentence** at `:1973`, re-located by LITERAL text. ⚠️ **CONTRADICTION OF THE
BRIEF, reported:** the brief calls this *“a single 40k+ character line”*. Measured, the longest line in the
file is **9808** characters (`:5073`) and this one is **5797**. It is a very long single line — the
re-location method the brief prescribes was still the right one — but the stated magnitude is wrong.

BEFORE: *“The gate IS still `rt.tlsMode` alone and that decision is correct; but `quicTLSConfig()`
(`quic.go:56-58`) returns `rt.defaultChain.tlsCfg` FIRST, before consulting `chainByName`, so a QUIC
listener whose only TLS is a QUIC-wrapped `default_filter_chain` booted with `tlsMode == false` and
registered NONE of the five.”*

AFTER: the clause leads with what survives — the repeal, and the booted-with-`tlsMode == false` outcome —
then a bracketed `[MECHANISM CORRECTED at phase 97/ADR-0319]` note killing all three parts: **(i)** the
precedence, **(ii)** the map-ORDER iteration (explicitly **not** *“the map is no longer consulted”*), and
**(iii)** the `quic.go:56-58` anchor, which is **DROPPED rather than renumbered** — it was wrong twice over.
The conclusion the sentence supports — the five listener-scope TLS counter names registered and permanently
zero on QUIC, and that being **PARITY** — is stated as untouched.

⚠️ **A SECOND CONTRADICTION, reported.** The brief (and Task 13) say `rt.chainByName` *“is still read at
two production sites”*. Re-measured at this tip with `git grep -n 'chainByName' -- 'internal/**/*.go'
':!*_test.go'`, `rt.chainByName[…]` is read at **three**: `quic.go:110`, `quic.go:201` **and**
`manager.go:1374` (the TCP path's own spec→info resolve). Task 13's figure was scoped to `quic.go` but
stated unqualified. **No count is written into either document** — both new texts say the map is read *“on
the TCP and the QUIC selection paths”*, which is true under either reading and cannot rot.

### Gates

| gate | figure |
|---|---|
| `git status --porcelain` | exactly `docs/envoy-go/DECISIONS.md` + `docs/envoy-go/BEHAVIOR_CONTRACT.md` |
| `go build ./...` | rc **0** |
| `^---$` / `^## ADR-` / `^## ` before ⇒ after | `216/318/326` ⇒ `216/318/326` |
| house-form `PROPOSED` hits, before ⇒ after | `:19054` (ADR-0319) ⇒ **none** |
| decoy-form hits, before ⇒ after | `:14866` (ADR-0231) ⇒ `:14866` (ADR-0231), `cmp` **EQUAL** to `HEAD` |
| retained italic footer | present at `:19087`, **never replaced** |

### ⚠️ What this task does NOT establish

This is a **prose** task. Nothing here is evidence about production behaviour — `go build` was green before
these edits and would be green after any wording. The residual recorded at ADR-0319 §Consequences (d) —
per-connection selection against a Start-time certificate — is **REASONED FROM THE MECHANISM AND WAS NOT
MEASURED**, and is written that way in the record rather than as a result.

## Task 22 — `ROADMAP.md` row 97 flipped `in-progress` -> `done`

**Field count, want 8, measured on BOTH SIDES of the flip and under BOTH forms:**

| moment | naive `awk -F'|' 'NR==159{print NF}'` | escape-aware `sed 's/\\|//g' \| awk` |
|---|---|---|
| BEFORE | **8** | **8** |
| AFTER | **8** | **8** |

`ROADMAP.md` stays **247** lines — the flip and the fold-in are both in-line edits. Escape-aware
malformed set is still exactly **{57, 69}** at file lines **119** and **131**, so the new summary cell
introduced no unescaped pipe. ⚠️ **An unescaped `|` passes check (1) and silently breaks the field
count**; it fired against the phase-96 IMPL's own author, who wrote a Go `||` into row 96's narrative.

### The sentinel, RE-RUN AT THIS STAGE'S OWN TIP — and the NC shapes CHANGED, as promised

| check | before the flip | AFTER the flip |
|---|---|---|
| (1) rows not `done` | ONE line, `NOT DONE: row 97` | **SILENT** |
| (2) candidate windows | SIX at `:207 :213 :219 :229 :235 :243` | **SIX**, same lines |
| (3) families never opened | SILENT | **SILENT** |
| NC-A (doctor row 62) | TWO lines | **ONE** line, `NOT DONE: row 62` |
| NC-B (denominator 128) | TWO lines | **ONE** line, `GATE FAIL: examined 129 … expected 128` |
| NC-C (gRPC family) | FIRED, residual 0 | **FIRED**, residual 0 |
| NC-D (`-family row` under `--`) | 96 / 68 | **96 / 68** |
| check-(2) positive control | 6 subs, residual 0 | **6 subs asserted, residual 0** |

⚠️ **NC-A and NC-B EACH DROPPED FROM TWO LINES TO ONE**, because row 97 was check (1)'s only voice.
**The shapes were measured on both sides of this stage's own edit and inherited from neither side.**

Six window per-line md5, **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex) —
**ALL SIX BYTE-IDENTICAL** to the phase-97 PLAN, SPEC and BRAINSTORM closes:
`207 10d7807bf02d` · `213 4a92f7e62fc6` · `219 2a7eb298b9fd` · `229 242e53c6f7a3` ·
`235 b2680e6f4fbf` · `243 6caa1c3ce0e7`

⇒ **THE SENTINEL DOES NOT FIRE.** Check (2) prints SIX lines, so not all three checks are silent.
**`stop` was evaluated and deliberately NOT created**, verified absent at the git root and in the stage
worktree. ⚠️ **THE MARGIN REMAINS ONE** — check (2)'s six is the only structural barrier left, and
**no sentinel match phrase was spelled in the new summary cell.**

### The `+0 fixtures` cell is wrong for this row — it lands **+1**

Stated in the row: fixtures **123 -> 124** via `0122-quic-chain-selection`.

### 🔴 THE FOLD-IN ROSTER IS REFUTED: `:139` IS NOT A CARRIER

`PLAN.md` Task 22 assigns *"`ROADMAP.md:136` and `:139` carry the same stale swallowed-panic claim as
the unfixable `:229`"*. **Measured, line by line, before editing anything:**

| line | verdict | action |
|---|---|---|
| `:136` | **CARRIER.** Asserts *"a **swallowed-panic BOOT HANG** … so the process neither crashes nor boots"* | **REPAIRED** |
| `:139` | 🔴 **NOT A CARRIER — IT IS ALREADY THE REFUTATION.** It reads *"its \"swallowed-panic boot-hang\" mechanism is refuted too (no `recover()` anywhere in the boot path)\"* | **LEFT, correctly** |
| `:159` | **NOT A CARRIER.** Row 97's own meta-narration ABOUT the `:229` claim, correct as written | rewritten as part of the flip |
| `:229` | **CARRIER, INSIDE SENTINEL WINDOW** | **DELIBERATELY LEFT** |

⚠️ **Repairing `:139` would have "corrected" a sentence that was already right** — the second half of
a two-item fold-in roster that nobody re-measured. **`:229` stays** because it sits inside a sentinel
window on a margin of one; `:136` sits inside no gate at all, which is exactly why it was repairable.

The `:136` repair **LEADS WITH WHAT SURVIVES**: the duplicate-`stat_prefix` hazard is real and remains
a direct hazard to any multi-listener fixture — which is why fixture `0122` gives its two connection
managers different prefixes. Only the MECHANISM died: row 78 repaired the swallowing on 2026-07-27 and
this phase re-measured the live behaviour as **rc=2 in 0 seconds on BOTH the boot and validate paths**.

---

## Task 23 — the six-gate sweep and the byte-untouched roster

🔴 **POSTURE: this section NAMES DEPARTURES. It does not claim compliance.** Two gates did not come
back clean and both are stated below with their raw output, unweakened. **Gate (a) FAILED.**

### Part A — the byte-untouched roster, asserted as a PARTITION

`internal/listener/manager.go` is **NOT** on the byte-untouched roster — PLAN §0.5 moved it to the edit
roster under a **comment-only** constraint. That gate is what replaces its digest, and it still reads **0**:

```
git diff master -- internal/listener/manager.go \
  | grep -E '^[+-]' | grep -vE '^(\+\+\+|---)' | grep -vE '^[+-][[:space:]]*//' | wc -l
0
```

Its raw change-set is `11 2 internal/listener/manager.go` — **13 touched lines, ALL of them comments.**

| byte-untouched roster entry | `git diff master --numstat -- <path>` | verdict |
| --- | --- | --- |
| `internal/listener/listenerfilter/**` | EMPTY | ✅ byte-identical to `master` |
| `internal/tls/**` | EMPTY | ✅ byte-identical to `master` |
| `internal/stats/**` | EMPTY | ✅ byte-identical to `master` |
| `test/fixtures/0104-http3-downstream-get/**` | EMPTY | ✅ byte-identical to `master` |

#### The partition, computed MECHANICALLY (not by reading)

⚠️ `--numstat`, never `--stat` — `--stat` is a SUM, not additions. The branch diff touches **12** paths.
The byte-untouched roster expands to **54** tracked files; the declared edit roster (SPEC §11's ten rows
plus `manager.go` via PLAN §0.5, with the `0122/**` row expanded) to **11**.

| direction | result |
| --- | --- |
| byte-untouched ∩ edit roster (**must be EMPTY**) | **EMPTY** — the two rosters are disjoint ✅ |
| byte-untouched ∩ touched (**must be EMPTY**) | **EMPTY** — no byte-untouched path is touched ✅ |
| edit roster \ touched | **EMPTY** — every declared edit actually landed ✅ |
| touched \ (byte-untouched ∪ edit roster) | 🔴 **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — **NOT EMPTY** |

🔴 **FINDING — THE ROSTER IS NOT A PARTITION OF THE TOUCHED SET.** One touched path,
`docs/envoy-go/BEHAVIOR_CONTRACT.md` (`3 1`), is on **NEITHER** roster.

**This is a roster-DOCUMENTATION gap, not an unauthorised edit, and the distinction is load-bearing.**
`SPEC.md` §11 says in terms: *"`BEHAVIOR_CONTRACT.md` IS NOT ON THE IMPL ROSTER EITHER … Whether a
`+0, UNCHANGED` ledger entry is owed is a question for the IMPL against the phase-96 precedent, and
this SPEC does not decide it."* **Task 21 (`fbb33f6d`) decided it in the affirmative** and landed the
`Phase 97 — +0, UNCHANGED` ledger entry plus a mechanism correction to the QUIC parity paragraph. The
edit is deliberate, reasoned and on the phase-96 precedent. **What was never done is amending SPEC §11's
roster to record the decision** — so a mechanical partition check finds an uncovered path. The file was
never on the byte-untouched roster, so **nothing was violated**; the roster is simply stale.

### Part B — THE SIX GATES

| # | gate | expected | **ACTUAL** | verdict |
| --- | --- | --- | --- | --- |
| (a) | differential, FULL suite, `-count=1` | 124 fixtures | **ABORTED at `0082`; 84 reported, 40 MASKED** | 🔴 **FAIL** |
| (b) | non-Docker sweep | 237 pkgs | **238 pkgs, rc 0, 0 FAIL** | ✅ (count departs — see below) |
| (c) | h2spec | `95 tests, 94 passed, 1 skipped, 0 failed` | **exactly that**, skip = 6.9.2/2 | ✅ |
| (d) | fuzzers | 56 targets / 48 FILES, +0 | **56 / 48, delta +0/+0** | ✅ |
| (e) | anchored panic gate | 0 | **1** (the gate CAUGHT gate (a)'s abort) | 🔴 **DEPARTURE** |
| (f) | no `REVIEW.md` | absent | **absent** | 🔴 **STANDING DEPARTURE** |

#### (a) Differential — 🔴 **FAILED: THE DRIVER-OWNED RECEIVER PORT RACE RECURRED**

`go test -count=1 -timeout 30m -v ./test/differential/` — FOREGROUND. **rc=1**, 248.347s.

```
--- FAIL: TestDifferential/0082-grpc-access-log-buffering (0.10s)
panic: driver: start ALS receiver on 0.0.0.0:41561: listen 0.0.0.0:41561:
       listen tcp 0.0.0.0:41561: bind: address already in use [recovered, repanicked]
FAIL	github.com/pgdad/envoy-go/test/differential	248.347s
```

🔴 **REPORTED AS A RECURRENCE, WITH THE PORT: `41561`** — inside the ephemeral range **32768-60999**
([[reference_probe_port_bands_inside_ephemeral_range]]). This is the long-banked
[[reference_driver_receiver_port_race_aborts_binary]]. **It is a host-port bind by the Go driver
process, NOT a container publish**, and it is **NOT** attributable to the 17 `curl-world-*` containers
of the sibling session (container count was 21 before the run and 21 after — the run orphaned nothing).

⚠️ **THE RECURRENCE IS AT A DIFFERENT SITE FROM THE ONE ALREADY OBSERVED THIS SESSION.** The earlier
firing was `0085-otlp-access-log-operators`; this one is `0082-grpc-access-log-buffering` (ALS
receiver). **Same class, different fixture and different port** — which is evidence the hazard is
driver-WIDE rather than a single fixture's defect.

**PER THE BRIEF, THE SUITE WAS NOT RE-RUN UNTIL GREEN.** The failure stands as measured.

| measure | value |
| --- | --- |
| fixtures reported (any verdict) | **84** |
| PASS | **83** |
| FAIL | **1** (`0082`, the panicking fixture) |
| **SKIP** | **0** — ⚠️ *a skip is the failure mode, and there were none* |
| **MASKED by the abort (never reached)** | **40** |
| `RAN \ IMPORTED` (`comm -13`) | **EMPTY** — nothing ran that is not on the roster |

🔴 **THE FIXTURE-SET ASSERTION CANNOT BE COMPLETED FROM THIS RUN.** `IMPORTED \ RAN` is **40** paths,
`0083`…`0122` — the abort masked everything after `0082`, **including the row's own new fixture
`0122-quic-chain-selection`.** A count of 124 is **NOT** claimed for this run.

**What IS proven about the roster, statically** (both `comm` directions, and these DID come back clean):

| direction | result |
| --- | --- |
| fixture dirs on disk under `test/fixtures/` | **124** |
| unique fixture dirs blank-imported by `runner_test.go` | **124** (from 147 import LINES — some fixtures import both `driver` and `inputs`) |
| `ondisk \ imported` | **EMPTY** ✅ |
| `imported \ ondisk` | **EMPTY** ✅ |

The `0122` blank import is present at `runner_test.go:149` — NC row 8's gate is armed.

##### A NARROW, SEPARATELY-LABELLED measurement — NOT a re-run of the aborted gate

Because `0122` was masked, its own liveness was measured on its own. **This is a different question
from gate (a) and is not scored as gate (a).**

```
go test -count=1 -timeout 10m -v -run 'TestDifferential/0122-quic-chain-selection' ./test/differential/
rc=0
--- PASS: TestDifferential/0122-quic-chain-selection (1.98s)
ok  github.com/pgdad/envoy-go/test/differential  2.049s
```

⚠️ **The `-run` footgun was checked, not assumed** ([[reference_differential_run_selector]]): the log
carries **no** `no tests to run` line and DOES carry a `--- PASS: TestDifferential/0122-…` subtest
verdict, so the selector genuinely matched. Panic gate on this log: **0**.

#### (b) Non-Docker sweep — 🔴 **THE BASELINE COUNT IN THE BRIEF IS WRONG, AND THE CAUSE IS IDENTIFIED**

```
go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'
PIPESTATUS[0]=0
go test -count=1 <238 packages>   rc=0
```

| measure | brief says | **ACTUAL** |
| --- | --- | --- |
| `go list ./...` | 239 | **240** |
| after the exclusion | 237 | **238** |

🔴 **CONTRADICTION REPORTED LOUDLY — AND RESOLVED.** The brief's 239/237 are the **`master`-tip**
figures, re-derived here: `go list ./...` at `master` reads **239**, at this branch **240**, and
`branch \ master` is exactly one package —
`github.com/pgdad/envoy-go/test/fixtures/0122-quic-chain-selection/driver`, **this row's own new
fixture driver**. `master \ branch` is EMPTY. The brief's baseline was simply taken before Task 6
added the fixture. **The correct figures at the IMPL tip are 240 and 238.**

**BOTH Docker drivers were excluded**, verified by set difference rather than by count:

```
comm -23 <all> <selected>
github.com/pgdad/envoy-go/test/conformance/h2spec
github.com/pgdad/envoy-go/test/differential
```

**SET RECONCILIATION — a count alone is not a reconciliation.** The package set `go test` reported
against was extracted from its own output and set-differenced against the selected set:

| direction | result |
| --- | --- |
| packages SELECTED | **238** |
| packages that REPORTED | **238** (125 `ok` + 113 `? [no test files]`) |
| `SELECTED \ REPORTED` | **EMPTY** ✅ |
| `REPORTED \ SELECTED` | **EMPTY** ✅ |
| `^FAIL` lines | **0** |
| anchored panic gate on this log | **0** |

#### (c) h2spec

`go test -count=1 -v ./test/conformance/h2spec/...` — **rc 0**.

```
Finished in 0.6080 seconds
95 tests, 94 passed, 1 skipped, 0 failed
h2spec conformance report: 95 total tests, 0 failures
```

⚠️ **The non-verbose run prints NO summary line** — only `ok … 2.731s`. The figure was obtained from a
`-v` run, and the skip was then identified rather than assumed: the **sole** numbered case in the whole
report lacking a `✔` is

```
6.9.2. Initial Flow-Control Window Size
  2: Sends a SETTINGS frame for window size to be negative
```

**6.9.2/2 — the invariant skip.** Confirmed, not inherited. Panic gate on this log: **0**.

#### (d) Fuzzers — +0, as anticipated

⚠️ **48 is FILES and 56 is TARGETS. They are not conflated.**

| measure | command | this tip | `master` | delta |
| --- | --- | --- | --- | --- |
| **TARGETS** | `git grep -hE '^func Fuzz' -- '*_test.go' \| wc -l` | **56** | 56 | **+0** |
| **FILES** | `git grep -lE '^func Fuzz' -- '*_test.go' \| wc -l` | **48** | 48 | **+0** |

The row consumes no new config field, so there is no new parse arm. Both figures match the brief.

#### (e) The anchored panic gate — 🔴 **READS 1, NOT 0**

`^panic:|DATA RACE|SIGSEGV`

| log | count |
| --- | --- |
| gate (b), the 238-package sweep | **0** |
| gate (c), h2spec | **0** |
| the narrow `0122` run | **0** |
| **gate (a), the full differential** | 🔴 **1** |

🔴 **NAMED, NOT ROUNDED AWAY.** The single hit is gate (a)'s `panic: driver: start ALS receiver on
0.0.0.0:41561`. **The gate did exactly what it exists to do: it caught an abort that a `^FAIL`-only
reading would have under-reported.** This is not a gate malfunction — it is a gate FIRING.

⚠️ **This gate was PROVEN LIVE at Task 1**, not merely observed at 0: a panic was inserted at each QUIC
accessor in **two separate runs** (`quicChain`, then `quicTLSConfig`), and the gate read **1** in each.
A gate that has only ever read 0 has not been shown to work; this one has — **and it has now fired on
an unplanted defect as well.**

#### (f) No `REVIEW.md` — 🔴 **STANDING DEPARTURE, NAMED**

`docs/envoy-go/phases/97-quic-chain-selection-order/REVIEW.md` is **ABSENT**. The phase dir holds
`BRAINSTORM.md`, `PLAN.md`, `PROGRESS.md`, `SPEC.md` and nothing else.

**Named rather than silently omitted, and stated with its scope:** **37** of **138** phase directories
carry a `REVIEW.md`, and **none of phases 93, 94, 95, 96 or 97 does.** The departure is house-wide and
long-standing, not introduced by this row.

#### `-race` — RESTATED, NOT RE-RUN

⚠️ **`-race` ON THE DIFFERENTIAL SUITE IS VACUOUS** — the subject there is an unraced subprocess
([[reference_differential_subject_is_unraced_subprocess]]). It was **not** re-run there. The race gate
is **Task 12's** full-package run, restated from its record:
`go test -race ./internal/listener/... -count=1` — **rc 0**, **`DATA RACE` occurrences 0**, anchored
FAIL 0, wall time 7s.

### Part C — module and lint hygiene

| gate | result |
| --- | --- |
| `go mod tidy -diff` | rc **0**, **OUTPUT EMPTY** ✅ |
| `git diff master --numstat -- go.mod go.sum` | **EMPTY** ✅ |

The row adds no sub-package to an existing module path, so
[[reference_new_subpackage_pulls_transitive_module]] did not bite — **re-checked anyway, as instructed.**

#### `go.mod` require entries — BOTH figures, with the METHOD named

⚠️ **A bare "67" would be meaningless without saying how it was counted.**

| method | figure |
| --- | --- |
| **structural `awk` extractor** — in-`require`-block, non-blank, non-comment lines | **67** |
| plausible character-class form `^\s+[a-z0-9./-]+ v[0-9]` | **62** |

**The gap is exactly 5**, and the five lines the character class cannot spell were **printed, not
inferred** — one underscore and four uppercase initials, exactly as the brief predicted:

```
github.com/prometheus/client_model v0.6.1
github.com/AdaLogics/go-fuzz-headers v0.0.0-20240806141605-e8a1dd7889d6 // indirect
github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
github.com/Microsoft/go-winio v0.6.1 // indirect
github.com/Microsoft/hcsshim v0.11.4 // indirect
```

**67 is the correct figure.** The character-class form is digit-and-case blind and under-reads by 5.

#### Lint and format

Touched Go trees, derived from the diff rather than listed by hand: `internal/listener`,
`test/differential`, `test/fixtures/0122-quic-chain-selection/driver`.

| gate | result |
| --- | --- |
| `golangci-lint run` over the three touched packages (v1.64.8) | rc **0**, **output EMPTY** ✅ |
| `gofmt -l` over the touched trees | **OUTPUT EMPTY** ✅ |
| `gofmt -l internal cmd test validate` (whole tree, for context) | **EMPTY** ✅ |

⚠️ **`gofmt -l` is gated on OUTPUT, never on exit code** — its exit code was 0, which proves nothing;
the empty output is the gate. ⚠️ golangci-lint's **misspell runs in locale US**
([[reference_golangci_misspell_locale_us.md]]) — British spellings in `.go` comments fail it while
gofmt, vet and build all pass. It came back clean.

### 🔴 THE EXPLICIT LIST OF STANDING DEPARTURES

1. 🔴 **Gate (a), the full differential suite, FAILED** — the driver-owned receiver port race recurred
   at **port 41561** on fixture `0082-grpc-access-log-buffering`, aborting the test binary and masking
   **40** fixtures, `0083`…`0122`. **The 124-fixture set assertion is NOT satisfied by any run in this
   task.** Not re-run until green, per instruction.
2. 🔴 **Gate (e), the anchored panic gate, reads 1, not 0** — the same abort. Reported as a firing, not
   filed as noise.
3. 🔴 **Gate (f), no `REVIEW.md`** — absent for phase 97, and for 93-96; 37 of 138 phase dirs carry one.
4. 🔴 **The roster is not a partition of the touched set** — `docs/envoy-go/BEHAVIOR_CONTRACT.md` is on
   neither roster. A deliberate, reasoned Task 21 edit that SPEC §11's roster was never amended to
   record. Nothing was violated; the roster is stale.
5. 🔴 **The brief's package baseline (239/237) is a `master`-tip figure and is wrong at this tip.** The
   IMPL-tip figures are **240/238**; the +1 is this row's own `0122` fixture driver package.
6. ⚠️ **`0122-quic-chain-selection` has never been proven green *inside a full-suite run* at this tip.**
   It passes in isolation (rc 0, real subtest verdict, selector confirmed to match). Departure 1 is why.

**Everything else in the sweep came back clean, and is recorded above with the command that produced it.**
