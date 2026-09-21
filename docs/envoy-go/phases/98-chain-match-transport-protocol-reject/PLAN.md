# Phase 98 — `chain-match-transport-protocol-reject` — PLAN

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` (recommended)
> or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Stage:** PLAN (lifecycle **2 -> 3**). Worktree off master `bd303d87`, branch `phase-98-plan`.
**Spec:** `docs/envoy-go/phases/98-chain-match-transport-protocol-reject/SPEC.md` (548 lines) — the plan
argues from the spec; executors read both.
**Governs:** `BRAINSTORM.md` (527 lines) — evidence only, and `SPEC.md` §0 refuted ten of its claims.

**Goal.** Make `filter_chain_match.transport_protocol` behave as the pinned reference does, on both of the
two measured divergences: accept any string at parse, and stamp `raw_buffer` as the detected transport
protocol on a TCP connection that no listener filter classified.

**Architecture.** Two edits in one production file, `internal/listener/manager.go`, `+4 / -9` by
`--numstat`: replace `parseChainSpec`'s four-literal `switch` with a plain assignment, and default an
empty `inputs.TransportProtocol` to `raw_buffer` immediately before `listenerfilter.SelectChain` in
`serveConnection`. `quic.go` is byte-untouched. Everything else in this plan is the surface that makes
those seven lines falsifiable.

**Tech stack.** Go; `go-control-plane` v3 protos; the project's own `listenerfilter` chain-match package;
the differential harness against `envoyproxy/envoy:contrib-v1.37.2` by digest.

---

## Global Constraints

Every task's requirements implicitly include this section.

- **Reference pin:** `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
  (`contrib-v1.37.2`). **Run BY DIGEST**, after verifying `docs/envoy-go/ENVOY_TARGET.md` lines 3-4.
  Re-verified in use at this PLAN.
- **`grep` is a `ugrep` shell function in EVERY shell here**, controller and subagent alike; it honours
  `.gitignore`, and `.gitignore:2` lists `next-prompt.txt`, which is **tracked anyway**. Use
  `/usr/bin/grep`, `command grep`, `git grep`, or a direct path. `command grep` does **not** survive
  `xargs`. **Name the binary, not the shell.**
- **`git -C <abs-worktree>` for every git command.** The Bash tool's cwd silently resets; it fired twice
  during this stage with an explicit `Shell cwd was reset` line. Shell variables do not survive between
  tool calls — re-export every time.
- **`-count=1` is not optional** on any `go test` invocation. The differential's failure mode is a
  **silent pass**.
- **Cost figures come from `git diff --numstat`, never `--stat`** (which is a sum, not additions).
- **`gofmt -l` never exits non-zero — gate on OUTPUT.** `golangci-lint`'s misspell runs in locale **US**:
  sweep British spellings in `.go` comments before the gate. Markdown prose may use them freely.
- **Ports:** `net.ipv4.ip_local_port_range` is `32768 60999`; the differential harness reserves
  `20000..31007` and `11000..14999`; `18080-19000` is held by sibling `curl-world` containers. The ad-hoc
  band is `15000-19000` minus those. **This stage took `16100-16199`** (see §2.4). **For a unit table, use
  port 0.** **`net.Pipe` deadlocks a client-cert handshake — use a loopback TCP pair.**
- **Never tear down a container this session did not create**, and only ever BY NAME. A `reaper_*`
  testcontainers Ryuk container is created by the differential itself and is REUSED — leave it alone.
- **Build with `-o` into scratch.** `go build ./cmd/envoy-go/` drops an untracked binary in the worktree
  root.
- **envoy-go quirks:** boots with `-c`, not `--config-path`; validate mode is `-mode validate` (single
  dash) where the reference's is `--mode validate`; an **omitted `clusters` key boot-REJECTS** it, so every
  probe subject needs a placeholder static cluster and `BackendCount()` must be **>= 1**; it rejects
  `match.headers`; `access_log[].log_format` is boot-rejected. Choose **non-1xx** sentinel statuses.
- **The row registers no stat name.** It owes a `+0, UNCHANGED` `BEHAVIOR_CONTRACT.md` ledger entry in the
  phase-96/97 form, **quoting no absolute** — three mutually inconsistent stat-surface absolutes are live
  in this tree at one tip.

---

## 0. What this PLAN refuted, by execution

Every item was produced by running something at this stage's own tip. **FOURTEEN** — counted with a
command, not recalled (`/usr/bin/grep -cE '^### (\U0001F534|\u26a0\ufe0f) 0\.'`). **Nine of the fourteen came from this
stage's own agents**, and three of those refuted a figure or an instruction in the brief the controller
gave them. ⚠️ **Expect to be refuted by your own review seam, not only by your successor** — fourth
consecutive row.

### 🔴 0.1 — `listener_filters_timeout` IS NOT ENFORCED ON THE SUBJECT. `BEHAVIOR_CONTRACT.md:4359` IS FALSE.

`BEHAVIOR_CONTRACT.md:4359`, located by literal text, reads:

> Per-pipeline timeout (`Listener.listener_filters_timeout`); default 15s; honored in [1s, 60s]
> envelope; `continue_on_listener_filters_timeout` honored as proto-documented.

**Measured, through the real binary, at the un-fixed tip.** One listener, `tls_inspector`,
`listener_filters_timeout: 1s`, `continue_on_listener_filters_timeout: true`, `filter_chains[0]` matching
`transport_protocol: raw_buffer`, a default chain, admin on `16110`, listener on `16111`. Validate rc=0
(`configuration OK`); boot reached `envoy-go listener l_tmo ready on 127.0.0.1:16111`.

| arm | client behaviour | body served | at |
|---|---|---|---|
| 1 — control | sends the request immediately | `INDEXED` | 0 ms |
| 2 | silent 2500 ms, then sends | `INDEXED` | 2501 ms |
| 3 | **silent 10000 ms**, then sends | **`INDEXED`** | **10009 ms** |

`listener.127_0_0_1_16111.downstream_cx_total: 3` — **all three connections were accepted and served.**
`http.chain_indexed.downstream_rq_total: 3`, `http.chain_default.downstream_rq_total: 0`.

**On a listener whose `listener_filters_timeout` is one second, the subject waited ten seconds.** The
timeout never fired. **Mechanism, read by symbol:** `peekerConn.Peek(n)` in
`internal/listener/listenerfilter/callbacks.go` delegates straight to `bufio.Reader.Peek(n)`;
`git grep -n 'SetReadDeadline' -- 'internal/listener/**/*.go' ':!*_test.go'` reads **zero hits**; and
`Pipeline.Run` sets a `context.WithTimeout` but checks `ctx.Err()` only **after `Inspect` returns**. A
client that never sends parks a goroutine in a deadline-free `bufio` read.

**The reference, measured this session on the byte-equivalent shape**, timed out at 1000 ms
(`listener filter times out after 1000 ms`, `fallback to default listener filter`), booked
`downstream_pre_cx_timeout: 1`, and under `continue_on_listener_filters_timeout: false` **dropped the
connection** (`downstream_cx_total: 0`). The subject has **no `pre_cx_timeout` stat at all**:
`git grep -c 'pre_cx' -- '*.go'` reads **0 files** (positive control: `downstream_cx_total` matches 3).

⚠️ **BANKED, NOT CHARTERED.** It is a third divergence on an adjacent dimension. This row has already been
widened once, from one divergence to two; widening it again would put the split gate in play for a defect
whose reference side is measured but whose subject repair is a different mechanism (socket deadlines)
in a different file. **Recorded, not repaired**, and §10 banks it with everything a BRAINSTORM needs.

### 🔴 0.2 — §5.2's ARM (s4) IS **NOT CONSTRUCTIBLE**, AND §4.1(2)'s FALL-THROUGH JUSTIFICATION IS STRUCTURALLY UNOBSERVABLE

`SPEC.md` §5.2 lists **(s4)**, the `continue_on_listener_filters_timeout` fall-through, "**only if** the
PLAN can construct a pipeline that times out without classifying". It cannot. **The foreclosing mechanism,
named before any control was run** (method note 7d):

1. `Pipeline.Run` returns `nil` **immediately** when `len(filters) == 0`. With no listener filters there
   is no timeout path at all.
2. **`tls_inspector` is the only `ListenerFilter` implementation in the tree.** Three independent matchers
   agree: `git grep -ln 'listenerfilter.ListenerFilterStatus, error)' -- '*.go' ':!*_test.go'` returns
   **one file**; `ListenerFilter interface` is declared once, at `listenerfilter/types.go:72`; and the
   package has no other `Register` site.
3. **Every return path of `Inspect` writes the field.** Counted: **5** `return` statements and **5**
   `inputs.TransportProtocol = ` writes (`"raw_buffer"` on four, `"tls"` on one). It never returns an
   error. Its own comment says why: *"ctx is not plumbed into the socket read — so every zero-byte error
   is a non-TLS classification, not an abort."*

⇒ Whenever the fall-through at `manager.go:1364` is reached, `inputs.TransportProtocol` has **already been
written non-empty** by the only filter that can be installed. **The stamp's `== ""` guard can never fire
on the timeout path.**

**Confirmed by execution, not only by reading:** §0.1's arm 2 and arm 3 both served `INDEXED` **at the
un-fixed tip**, because `tls_inspector` had already written `raw_buffer`. A timeout arm would be **GREEN
before and after the fix** — the false-agreement class (method note 58) inside our own suite.

**What changes and what survives.** The **placement** in §4.1(2) survives and is right: one install
immediately before `SelectChain` covers every path, and costs nothing. What dies is the **justification** —
"including the `continue_on_listener_filters_timeout` fall-through" implies the fall-through is a path the
guard distinguishes, and today it is not. The install is still anchored on the entry of selection
(method note 3f) because a **future** listener filter that leaves the field empty would make the guard
live, and NC roster row 5b exists to keep the guard honest. **(s4) is struck from the test design with a
named mechanism, never as a bare absence** (method note 73).

### 🔴 0.3 — §2.3's TIMEOUT ARM IS NOW **MEASURED**, AND THE INFERENCE WAS RIGHT — FOR A REASON WORTH STATING

`SPEC.md` §2.3 records the listener-filter timeout path as **inferred, not measured**. It was measured this
stage, on the reference, by this stage's Docker agent.

| reference arm (`listener_filters_timeout: 1s`, `tls_inspector`) | body | `chain_indexed` | `chain_default` | `pre_cx_timeout` |
|---|---|---|---|---|
| `raw_buffer` chain, client silent 2500 ms | **`INDEXED`** | 1 | 0 | 1 |
| `tls` chain (matched negative, 2-line diff) | `DEFAULT` | 0 | 1 | 1 |
| `raw_buffer`, `continue_on…: false` (positive control) | **connection dropped** | 0 | 0 | 1, `cx_total` **0** |
| `raw_buffer`, client sends immediately (control) | `INDEXED` | 1→2 | 0 | **stayed 1** |

The timeout provably fired: `listener filter times out after 1000 ms` / `fallback to default listener
filter` in the log, and `downstream_pre_cx_timeout` incremented only on the slow arms. The matched negative
proves the dimension **is** enforced on that path rather than chain 0 winning by index.

⚠️ **The agreement between the timeout arm and the immediate arm is agreement by TWO MECHANISMS, not one
path.** The immediate arm reaches `raw_buffer` by the inspector **actively** classifying a non-TLS stream
(`parseClientHello failed: … HTTP_REQUEST`); the timeout arm reaches it with **no classification at all**.
The discriminators that separate them are the `downstream_pre_cx_timeout` delta and the presence of the
`tls inspector: recv` log lines. **The repair's "stamp after the pipeline regardless of how it ended"
matches the reference's observable** — that is what is established, and no more.

### 🔴 0.4 — THE FIXTURE FLOORS `1359` / `1393` ARE **NOT** THE TWO MOST RECENT, AND BOTH ARE PKI-INFLATED

`SPEC.md` §14 item 6 and the router both quote *"the fixture floors are `1359` and `1393` added lines"*.
Measured with `git show --numstat --format='' <sha> -- test/fixtures/<dir>/`:

| fixture dir | add-commit | **dir-only added** | files | whole commit added |
|---|---|---|---|---|
| `0119-grpc-unary-trailers/` | `28749069` | 1134 | 7 | 1310 |
| `0120-tls-connection-error/` | `0a985a35` | **1359** | 10 | 2638 |
| `0121-listener-default-chain-tls/` | `c7bd2880` | **1393** | 9 | 2550 |
| **`0122-quic-chain-selection/`** — the newest | `d4940d68` | **946** | 3 | 5833 |

**Three refutations.** (1) `1359` and `1393` are the **third- and second-most-recent** additions; the most
recent, `0122`, is **946** — below both. (2) **Both quoted fixtures are TLS fixtures whose dir cost is
PKI-inflated**: `0120` ships 5 `pki/` files, `0121` ships 4 plus a 172-line `pki/gen/main.go`, **201 lines
of PKI** that a plaintext fixture does not spend. `0123` is plaintext-only. (3) `0122` is cheap for a
structural reason the PLAN must decide on: **it ships no committed YAML at all** (README 238,
`driver/driver.go` 534, `expectations.yaml` 174), rendering both configs in the driver, where `0121` ships
`envoy.yaml` + `envoy-go.yaml` on disk.

⇒ **The honest dir-only floor for `0123` is ~946** on the `0122` driver-rendered shape, or `1393 − 201 =
~1192` on the `0121` committed-YAML shape minus PKI. The inherited figure overstates a plaintext fixture's
floor by roughly 150–450 lines. §9 prices it on the shape this plan actually chartered.

### ⚠️ 0.5 — `SPEC.md` §7.1's MULTI-LISTENER PRECEDENT IS ARITY-MISMATCHED; BETTER ONES EXIST

§7.1 cites `0008-listener-chain-match` as the precedent for several TCP listeners in one fixture
(`ReferenceListenerPorts` returns two). `0123` needs **three**. **Three-listener precedents already
exist**: `0018-http-rbac/inputs/driver.go:272` returns `[]int{refLATestPort, refLBTestPort,
refLATestTLSPort}` against `SubjectListenerNames() = {"l_test_a","l_test_b","l_test_a_tls"}`; likewise
`0020-http-ext-authz-http`, `0021-http-ext-authz-grpc`, `0022-http-ext-proc-grpc`.
`0013-http-local-ratelimit` returns **four**. `0008` additionally carries an `AlternateConfigDriver` (the
`c4` variant), a `backends/` directory and `sockopts.go`, **none of which `0123` needs**. Cite `0018` or
`0020` for arity and `0122` for shape.

**The dispatch contract, read at `test/differential/runner_test.go:1235-1249`:** the runner type-asserts
`fixture.MultiListenerDriver`; it `t.Fatalf`s when `len(SubjectListenerNames()) != len(ReferenceListenerPorts())`,
builds `refAddrs` by **index-wise zip** of the two, and calls `DriveReferenceMulti` / `DriveSubjectMulti`.
⚠️ **The single-address `SubjectListenerName()` / `ReferenceListenerPort()` must STILL be implemented**,
returning listener[0] — the admin-probe path uses them.

### ⚠️ 0.6 — §14 ITEM 4 IS DISCHARGED, AND ITS FRAMING IS REPLACED BY A SHARPER HAZARD

The worry was that a chain serving **zero** requests might not emit its `http.<prefix>.downstream_rq_total`
name at all, making a cross-side pin silently vacuous (method note 46). **Measured on both sides this
stage.**

| side | `/stats` (flat) | `/stats/prometheus` | zero-valued name present? |
|---|---|---|---|
| reference | `http.chain_default.downstream_rq_total: 0` | `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="chain_default"} 0` | **YES, both** |
| subject | `http.chain_indexed.downstream_rq_total: 0` | `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="chain_indexed"} 0` | **YES, both** |

Both sides emit both names on both endpoints, and the mirror arms (one string apart) produce the mirror
values. **But the reference emits them at `0` BEFORE ANY TRAFFIC AT ALL** — a pre-traffic scrape read
**168** lines matching `chain_(indexed|default)`, including the whole `downstream_cx_*` family, with zero
connections made. The per-chain HCM stat scope is created at **config** time, not at first request.

⇒ **A pin that asserts the NAME is present is VACUOUS on both sides — it is satisfied from boot.** The only
non-vacuous assertion is the **VALUE PAIR** (served chain `1`, non-served chain `0`) **plus its mirror
arm**, which is exactly what `0123`'s three listeners provide. ⚠️ And the project note *"stats sinks emit
only USED stats"* **does not transfer to the admin endpoints** on either side.

⚠️ **Two spelling traps the driver must not step in.** (a) The prometheus spelling **differs from the flat
one** — `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="<prefix>"}` — and within the family
the **order differs too**: flat `/stats` is alphabetical (`chain_default` before `chain_indexed`),
prometheus is **registration order**. A driver that parses into a map is safe; one that pins line order is
not. (b) On the subject, **`/stats?format=json` is NOT a JSON surface** — `cmp` shows it byte-identical to
`/stats`; `/stats/json` **404s**. The roster of admin paths is eight: `/ready`, `/stats`,
`/stats/prometheus`, `/config_dump`, `/clusters`, `/listeners`, `/server_info`, `/drain_listeners`.
**Every fixture driver that cross-asserts stats scrapes `/stats/prometheus`** and projects back to the
internal name; the precedent is `test/fixtures/0005-prometheus-stats/driver/driver.go:489`. `0123` follows
it.

### ⚠️ 0.7 — THE ROUTER'S METHOD NOTE 3e IS STALE, AND THE ROUTER CONTRADICTS ITSELF

Method note 3e states *"At this tip: dirs 123 = imports 123 (`test/differential/runner_test.go`), both
`comm` directions EMPTY, split 99 `driver/` + 24 `inputs/`."* Run **verbatim** at `bd303d87` with the
note's own extractor:

```
imports: 124    dirs: 124    comm -23: EMPTY    comm -13: EMPTY
driver/ arm: 100    inputs/ arm: 24
```

**124 = 124, split 100 + 24.** The **gate is sound** — both `comm` directions are empty, so the
reconciliation is perfect — but the **numbers rotted** when phase 97 added `0122` via `driver/`. ⚠️ **And
the router disagrees with itself inside one document**: its own LIVE FIGURES bullet already reads
*"fixtures **124**"* while note 3e reads 123. Method note 62's class, one document lower.

### ⚠️ 0.8 — METHOD NOTE 57 RE-CONFIRMED, AND SHARPENED: IT IS THE **STAGE-WORD FILTER** THAT BLINDS YOU

`git log --grep '^phase 95 (.*) IMPL'` reads **ZERO** at this tip; phase 95's IMPL is `0f3b98b3`, whose
subject is `phase 95 (tls-alpn-mismatch-fallback): an ALPN-mismatching client is now SERVED…` — no stage
token. **But the loose form `--grep '^phase 95'` reads 6 and DOES contain it.** So the blindness is not in
the loose form; it is in **any filter that requires the stage word**, whether that filter lives in the grep
pattern or in a `| grep -i IMPL` after it. ⚠️ **Running both forms is not enough if you post-filter both on
the same word.** Reconcile the *sets*, not the greps.

### 🔴 0.9 — `SPEC.md` §6's MATCHER M1 IS BLIND TO THE ONE PRODUCTION SITE IT EXISTS TO FIND

M1 is declared as the literal `must be "tls"`. Run against the file it is named for:
`git grep -nF -- 'must be "tls"' -- internal/listener/manager.go` → **rc=1, no output.** The Go source
**escapes the quotes**, so the hit is reachable only through
`git grep -nF -- 'must be \"tls\"' -- internal/listener/manager.go` → **`manager.go:998`**.
⚠️ **Run BOTH spellings, or M1 silently reports "zero production sites"** — an absence that reads as
permission. Method note 49, one level lower than usual: the matcher's vocabulary was wrong about the
**escaping**, not about the words.

### 🔴 0.10 — ALL SIX MATCHERS ARE BLIND TO `types.go:44-46` — THE ONE K2 **CODE** SITE THE ROW MUST EDIT

`SPEC.md` §6.2's first row is `internal/listener/listenerfilter/types.go:44-46`, dispositioned **EDIT**.
Every matcher was run scoped to that file: **M1a, M1b, M2, M3, M4, M5a, M5b, M5c all rc=1**; M6's declared
scope excludes the file by construction. **Positive control that the file is grep-reachable at all:**
`git grep -nF -- 'no listener' -- internal/listener/listenerfilter/types.go` → `types.go:45`, rc=0.

**Cause: line-splitting.** The claim reads `…or "" if no listener` on `:45` and
`// filter inspected the connection.` on `:46`. `git grep` is **line-based**, so M5a's `no listener.?filter`
cannot span the break. The same class hides `DECISIONS.md:19085`, another anchor §6.1 names:
`grep -c 'DECISIONS.md:19085'` over the union → **0**.

⇒ 🔴 **`SPEC.md` §6's TABLES ARE A SUPERSET OF WHAT ITS OWN MATCHERS PRODUCE.** They were built partly by
reading. **`SPEC.md` §14 item 1 tells the PLAN to "re-run §6's matchers — the union may grow"; obeying that
instruction LITERALLY would have LOST a mandated code edit.** The correct procedure, and the one Task 11
prescribes: **inherit the TABLES, use the matchers only to look for GROWTH**, and never let a re-derivation
shrink an inherited roster. **A re-derivation that loses a row is not a re-derivation, it is a deletion.**

### ⚠️ 0.11 — M6 OVER `BEHAVIOR_CONTRACT.md` IS THE EMPTY SET

`git grep -c -- 'raw_buffer' -- docs/envoy-go/BEHAVIOR_CONTRACT.md` → **rc=1, prints `0`**.
Discriminating positive control, same command shape, same file:
`git grep -c -- 'transport_protocol' -- docs/envoy-go/BEHAVIOR_CONTRACT.md` → **`:3`**, rc=0.
Widened spelling `raw.?buffer`, case-insensitive → still rc=1.
⇒ **The string `raw_buffer` appears ZERO times in `BEHAVIOR_CONTRACT.md`.** This **corroborates** §6.2's
verdict that the §Chain-match algorithm section carries an **omission, not a false claim** — but it also
means **M6 contributes nothing over half its declared scope, and the PLAN must not quote it as coverage.**
The same census over `test/` is also empty (positive control: `transport_protocol` hits
`0002-tls-tcp/driver/driver.go`), consistent with fixture `0123` being new.

### 🔴 0.12 — A HARD CONSTRAINT NO DOCUMENT STATES: THE `chainmatch.go` COMMENT EDIT MUST BE **LINE-COUNT-NEUTRAL**

`internal/listener/quic_test.go` carries **15** `chainmatch.go:<line>` citations:

```
2 chainmatch.go:87    2 chainmatch.go:119   1 chainmatch.go:125
5 chainmatch.go:128   4 chainmatch.go:131   1 chainmatch.go:293
```

**Every one points below line 32** — the minimum cited line is **87**. The §6.1-mandated comment-only edit
sits at **`:30-32`**. ⇒ **If that edit is not line-count-neutral, all fifteen cites go stale in the same
commit that "reconciles" the prose.** §6 does not state this. **Task 12 requires the replacement to read
exactly `3	3` on `git diff --numstat`**, and the gate in §7 asserts it alongside the comment-only property.
A 3/3 parity-correct rewording was demonstrated during this stage, so the constraint costs nothing — but it
is invisible unless someone looks for it.

### 🔴 0.13 — NC ROSTER ROW 6 IS **BLIND TO EVERY ARM THE SPEC CHARTERS**, AND CONSTRUCTIBLE ONLY BY WIDENING THE EDIT SET

`SPEC.md` §11 row 6 — *"stamp moved into `SelectChain`"* — is left UNADJUDICATED with the instruction
*"name the arm, or delete the row as vacuous"*. **Adjudicated here by naming the mechanism first, then
running it** (method note 7d).

**Mechanism.** `SelectChain(inputs ChainMatchInputs, …)` at `chainmatch.go:80` takes `inputs` **BY VALUE**.
In `serveConnection`, `inputs` is touched at exactly three places — built at `:1326`, passed as `&inputs`
to the pipeline at `:1358`, passed by value to `SelectChain` at `:1368` — and **nowhere afterwards**
(checked by `awk` over the whole function). So moving the stamp from just-before the call to just-inside
the callee is **observationally identical for the TCP caller**. For the QUIC caller it is dormant:
`quicChainMatchInputs` (`quic.go:141-174`) builds a **composite literal** that always sets
`TransportProtocol: quicTransportProtocol` (`= "quic"`, `quic.go:127`), and `grep -n 'TransportProtocol'
internal/listener/quic.go` shows only a comment, the const, and that one literal — **on QUIC the `== ""`
guard can never fire.**

**Run, not reasoned.** The mutation was applied (`gofmt -l` clean) and the seven reverse-dependency
selectors run with `-count=1`: **rc=0 on every one; ZERO existing tests redden.** The un-mutated control
run is identical. ⚠️ **Identical greens under the mutation and the un-mutation mean the suite is BLIND**
(method note 50). The two candidate discriminators were read and are structurally blind:
`FuzzFilterChainMatch` builds `inputs` with no `TransportProtocol` **and** no chain spelling one, and
`chainmatch_test.go`'s `TestSelectChainTransportProtocol` passes `"tls"`, never `""`.

**But an arm IS constructible**, and it was built and run both ways:
`TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain` in
`internal/listener/listenerfilter/chainmatch_test.go`, asserting
`SelectChain(ChainMatchInputs{}, []*ChainSpec{{Name:"rb", TransportProtocol:"raw_buffer"}}, nil)` returns
`(nil, ErrNoChainMatched)`. **PASS un-mutated; FAIL under the mutation** (verbatim: *"SelectChain must NOT
default the input"*); and its **matched negative** (`TransportProtocol: "tls"` picking the `tls` chain)
**stayed PASS under both**, so the arm is not a blanket detector.

⇒ **VERDICT: row 6 SURVIVES, and it costs a file the SPEC did not budget.** `chainmatch_test.go` is not in
§6.3's edit set and not on the byte-untouched roster — adding it is legal, but **it must be declared, not
allowed to appear.** Task 8 lands the arm; §8 keeps row 6 with the arm named. The alternative — keeping row
6 without it — is keeping a control that **cannot fire**, which this project treats as worse than deleting
it.

⚠️ **AND `SPEC.md` §4.2(B)'s REJECTION REASON IS FORWARD-LOOKING, NOT A MEASURED DIVERGENCE.** §4.2(B)
rejects placing the stamp in `SelectChain` because it would be *"dormant today and wrong the day a QUIC
input arrives empty"* — which says so itself. The measurement above confirms it: **today the two placements
are indistinguishable on every production path.** The design argument stands; **the empirical one does not,
and no task may cite §4.2(B) as if it recorded observed behaviour.**

### ⚠️ 0.14 — `ChainSpec.Name` IS **INDEX-DERIVED**, NOT THE PROTO `Name` — MEASURED

The §5.1 test must identify which chain `SelectChain` returned. Built the §5.1 shape through real
`NewManager` and logged the specs:

```
chainSpecs[0]: Name="l_tp/filter_chains[0]" Empty=false TP="tls" SourceTypeLocal=false
chainSpecs[1]: Name="l_tp/filter_chains[1]" Empty=false TP=""    SourceTypeLocal=true
```

**A `Name` set on the proto `FilterChain` does NOT propagate.** Any assertion written against `"fc0"` /
`"fc1"` would fail for a reason that has nothing to do with chain matching — the vacuous-pin class in
reverse. **Assert `"l_tp/filter_chains[0]"` and `"l_tp/filter_chains[1]"`.**

Two further §5.1 premises, both **confirmed by execution** rather than by reading the ranking table:
`specificityScore(transport_protocol only) = 16` against `specificityScore(source_type only) = 4`, so
`transport_protocol` **strictly outranks** `source_type`; and `ChainMatchInputs.IsLoopbackSource()` is
`c.SourceIP != nil && c.SourceIP.IsLoopback()`, so **a nil `SourceIP` is NOT loopback** — §5.1's "every
probe input carries loopback `SourceIP`" is **mandatory, not stylistic**, and omitting it would silently
make the sibling chain ineligible and the whole test vacuous.

Finally, **§0.8 of the SPEC is CONFIRMED by execution with its reject quoted**: the `raw_buffer`-sibling
shape it rejected really does boot-reject, verbatim —
`listener: "l_tp": at most one filter_chain may omit filter_chain_match.server_names (catch-all); got 2` —
while the `source_type` sibling shape boots (`len(chainSpecs)=2`, `defaultSpec=<nil>`) **and still boots
after NC (ii) empties fc[0]**, so properties (b) and (d) can fail cleanly instead of being masked.

---

## 1. Stage scope, MEASURED

### 1.1 What THIS PLAN commit touches — FOUR files

Measured with `git show --numstat --format='' <sha>` across **five** precedent PLAN commits, located by
BOTH forms and reconciled: `git log --grep '^phase 9[3-7] (.*) PLAN'` returns five; the loose
`--grep '^phase 9[3-7]'` returns 38, of which eight contain `PLAN`, the three extra being
`675e4ece`, `820fc145`, `12e57e8d` — all `next-prompt` commits whose **subject prose** mentions the PLAN.
**Strict is a subset of loose, and loose adds no PLAN-stage commit. The sets reconcile.**

| PLAN commit | phase | `STATE.md` | `STATE_HISTORY.md` | the new `PLAN.md` | `next-prompt.txt` | paths |
|---|---|---|---|---|---|---|
| `90010c4c` | 93 | `9 8` | **`2 0`** | `1031 0` | `76 65` | **4** |
| `db539e7d` | 94 | `9 9` | **`2 0`** | `1662 0` | `93 106` | **4** |
| `f647dd72` | 95 | `10 10` | **`2 0`** | `865 0` | `88 78` | **4** |
| `0eab42e3` | 96 | `10 10` | **`2 0`** | `1666 0` | `49 43` | **4** |
| `01e9d72b` | 97 | `10 10` | **`2 0`** | `1555 0` | `92 59` | **4** |

**The claim holds on all five**, with no exception and no extra path, and `STATE_HISTORY.md` reads exactly
`2 0` in every one. ⚠️ **This is a DIFFERENT path set from a SPEC's** (`SPEC.md` §10.2 measured five paths,
including `DECISIONS.md`): **a PLAN touches `DECISIONS.md` in NONE of the five precedents.** That is why
this stage leaves `ADR-0320` alone and the house `PROPOSED` guard **ARMED** (§2.5).

⇒ **This stage touches exactly:** `STATE.md`, `STATE_HISTORY.md`, this `PLAN.md`, `next-prompt.txt`.
`ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, `DECISIONS.md`, `internal/**` and `test/**` are **byte-untouched**,
asserted per path with `git diff master --numstat` at the close.

Precedent `PLAN.md` line counts, all byte-identical at HEAD (zero post-commit drift): 93 → **1031**,
94 → **1662**, 95 → **865**, 96 → **1666**, 97 → **1555**. Range **865–1666**, median **1555**.

### 1.2 What the IMPL will touch — the scope the spine is sized against

`git show --numstat --format='' <sha>`, the three most recent IMPLs. ⚠️ **Phase 95's IMPL is invisible to
every stage-word-filtered locate form** (§0.8) and is excluded here for that reason, stated rather than
hidden.

| IMPL commit | phase | paths | added | deleted |
|---|---|---|---|---|
| `0a985a35` | 94 | 28 | 2638 | 181 |
| `c7bd2880` | 96 | 20 | 2550 | 170 |
| `d4940d68` | 97 | **15** | 5833 | 99 |

Phase 97's fifteen paths are the nearest template: `BEHAVIOR_CONTRACT.md` `3 1`, `DECISIONS.md` `139 6`,
`ROADMAP.md` `2 2`, `STATE.md` `10 10`, `STATE_HISTORY.md` `2 0`, `PROGRESS.md` `3023 0`,
`manager.go` `11 2`, `manager_test.go` `145 3`, `quic.go` `157 26`, `quic_test.go` `1332 6`,
`next-prompt.txt` `62 43`, `runner_test.go` `1 0`, and the three `0122` fixture files.

### 1.3 The split gate — EVALUATED, NOT SPLIT, WITH MARGIN ON BOTH AXES

`BOOTSTRAP_PROMPT.md` §6.1, read at the repo root at this tip (`:283-292`), triggers a split if `PLAN.md`
exceeds **~25 numbered tasks** OR estimates exceed **~1500 lines of code** of net change. ⚠️ **§5 appears
VERBATIM TWICE** — §5 at `:209` and the Skill Routing Appendix later; both were located **by heading**,
never by arithmetic, because the second copy's offset is non-constant.

**Task count: 20** (§6) — **DERIVED here**, not inherited; `SPEC.md` §14 quotes none.

**LoC, anchored on precedents measured first-hand at this tip.** Every basis cell names a file that exists
now and was measured with `wc -l` or `git show --numstat`.

| component | basis | estimate |
|---|---|---|
| `internal/listener/manager.go` — both production edits | **MEASURED**: the SPEC's built-run-and-reverted prototype, `--numstat` | **+4 / −9** |
| `internal/listener/manager_test.go` — the §5.1 re-pointed test | the block it replaces is 28 lines (`:1688-1715` — a 3-line doc comment plus a 25-line func); the replacement pins four properties over a real `SelectChain` | **+95 / −28** |
| `internal/listener/manager_test.go` — §5.2 arms (s1), (s2) | **MEASURED template**: `TestUnifiedDispatchPlaintextChainSelectByDestPort` spans `:3459-3538`, **80 lines**, and already carries the per-chain discriminator (`startTaggedBackend`) | **+170** |
| `internal/listener/manager_test.go` — §5.2 arm (s3), TLS + `tls_inspector` | the same template plus a TLS client; the package already declares `testAlphaCertPEM` / `testAlphaKeyPEM` at `:587` / `:600`, so **no PKI is shipped** | **+110** |
| `internal/listener/quic_test.go` — the four narration sites | four comment/message blocks | **+14 / −10** |
| `internal/listener/listenerfilter/chainmatch.go` — COMMENT-ONLY | one comment block, under the §7 gate | **+5 / −3** |
| `internal/listener/listenerfilter/types.go` — one comment | one comment block | **+5 / −3** |
| `test/fixtures/0123-…/driver/driver.go` | `0122`'s measures **534** with one rendered config pair; `0123` renders three listeners and a three-way stat projection | **+620** |
| `test/fixtures/0123-…/expectations.yaml` | `0122`'s measures **174** | **+200** |
| `test/fixtures/0123-…/README.md` | `0122`'s measures **238** | **+260** |
| `test/differential/runner_test.go` | the blank import | **+1** |
| `docs/envoy-go/DECISIONS.md` — ADR-0320 §Decision + §Consequences | precedent: phase 97's `d4940d68` reads **139 / 6** | **+120 / −6** |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` — ledger entry + §Chain-match bullet | precedent: `d4940d68` reads **3 / 1** | **+8 / −1** |
| `docs/envoy-go/ROADMAP.md` — row 98 flip | precedent: `d4940d68` reads **2 / 2** | **+2 / −2** |

⚠️ **THIS ROW SHIPS NO PKI.** `0120` spent 45 lines on `pki/**` and `0121` spent **201** (four PEMs plus a
172-line `pki/gen/main.go`). `0123` is plaintext-only on every listener, and its one TLS unit arm reuses
the package's existing inline PEM pair. **That is the single largest reason §0.4's inherited floor does not
apply to this row.**

**Totals, each labelled with its accounting — a figure without its measure is meaningless:**

| accounting | phase 98 estimate | phase 97 estimate | phase 96 MEASURED (`c7bd2880`) | phase 94 MEASURED (`0a985a35`) | verdict |
|---|---|---|---|---|---|
| `.go` only | ≈ **+1024 / −53** | ≈ +1218 | **+1285** | **+1143** | **under all three** |
| `.go` + `expectations.yaml` | ≈ **+1224** | ≈ +1328 | +1725 | +1635 | **under all three** |
| the above + `README.md` | ≈ **+1484** | ≈ +1518 | +1980 | — | **under ~1500 — where phase 97 was OVER** |

`PROGRESS.md` and `next-prompt.txt` are excluded from every column, on the same exclusion phases 94, 96 and
97 are measured under: a transcript and a router are not lines of code.

**DECISION: DO NOT SPLIT.** Four grounds, in order of weight:

1. **The row is under the line on EVERY accounting, including the widest** — ≈ `+1484` against `~1500` —
   which is the accounting phase 97 was **over** on (≈ `+1518`) and was still, correctly, not split.
2. **The only clean seam is {unit layer} / {fixture `0123`}, and it defeats the row in both directions.**
   `SPEC.md` §3.1 measured the consequence: **the stamp turns ZERO tests RED** in a 392-test reverse-
   dependency sweep. A `98.1` shipping the production edit with only the fixture would gate nothing at
   unit level; a `98.2` shipping the arms alone would gate nothing cross-side. **Splitting the pins away
   from the code is the one split that defeats this row.**
3. **The production edit is SEVEN LINES in ONE FILE**, measured at `+4 / −9`. Everything else is the
   surface that makes it falsifiable.
4. **20 tasks against ~25 — a margin of FIVE**, where phase 97's was **one**. ⚠️ **Sub-step counts
   MEASURED, not asserted** (§12): the per-task histogram is
   `4 5 5 5 5 6 6 6 6 6 6 6 6 7 7 7 7 7 7 8`, **122 sub-steps over 20 tasks, maximum EIGHT** — **no task
   reaches §6.1's ~10 line**, where phase 97 carried two at ELEVEN.

⚠️ **AND THE ESTIMATE IS A LOWER BOUND.** `reference_measured_prototype_is_a_lower_bound` has now fired
**nineteen consecutive rows**, and at phase 98 it fired on the **shape**: the BRAINSTORM's `+1 / −6` became
`+4 / −9` because its positive control never drove the subject. **Treat every cell above as a floor**, and
if any single task's sub-steps blow past ~10 once contact with reality reveals complexity,
`BOOTSTRAP_PROMPT.md` §6.1's **mid-execution** trigger is the remedy — the split happens THEN, not by a
retroactive re-reading of this verdict. The measured candidates are **Task 7** (the fixture driver, EIGHT — this plan's largest) and
**Task 9** (the four registration gates, SEVEN), because those are exactly the two that ran ELEVEN at
phase 97.

---

## 2. Sentinel — RUN MECHANICALLY AT THIS STAGE'S OWN TIP, ACTUAL OUTPUT

A PLAN adds no ROADMAP row and flips none, so `want` stays **130**, check (1) stays **one** line and NC-A
and NC-B each stay **two**. **That prediction was MEASURED, not inherited** — the SPEC made the same claim
and measured it too, and NC shapes have changed under this project before (at the phase-98 BRAINSTORM's row
ADD, check (1) went SILENT → ONE and NC-A/NC-B ONE → TWO). ⚠️ **The IMPL's row FLIP will change them
again**, back to SILENT / ONE / ONE. **Do not inherit these across the flip.**

Everything below was run with `/usr/bin/grep`, at `bd303d87` plus this stage's docs.

### 2.1 The three checks

- **Check (1)**, `want=130`: **ONE** line — `NOT DONE: row 98`. That is this row's own `in-progress`
  status and the normal mid-phase state, not a fault.
- **Check (2): SIX**, at `:208 :214 :220 :230 :236 :244`. Per-line md5, **trailing newline INCLUDED**
  (`sed -n 'Np' f | md5sum`, first 12 hex) — **all six byte-identical to the phase-98 SPEC close**:
  `208 10d7807bf02d` · `214 4a92f7e62fc6` · `220 2a7eb298b9fd` · `230 242e53c6f7a3` ·
  `236 b2680e6f4fbf` · `244 6caa1c3ce0e7`. ⚠️ **State the method whenever you quote the digest** — the six
  match only with the trailing newline included.
- **Check (3): SILENT.**

⇒ **THE SENTINEL DOES NOT FIRE.** `stop` was evaluated and **deliberately NOT created**; verified absent at
the git root and in this stage's worktree. ⚠️⚠️ **THE MARGIN IS STILL ONE.** Check (2)'s six is the only
structural barrier; check (1) is merely borrowing a voice from the open row 98. **Deleting the last
deferred-candidate line ends the project. Do not "tidy" one, and do not "fix" the six.**

### 2.2 The four NCs and the check-(2) positive control — ALL RUN, ALL FIRED

- **NC-A** — row 62 doctored to `in-progress`. **Substitution inspected BEFORE the result was trusted:**
  `NC LANDED? [ in-progress ]`. Then check (1) on the doctored copy: **TWO** lines, `NOT DONE: row 62` and
  `NOT DONE: row 98`.
- **NC-B** — `want=129` on the real file: **TWO** lines, `NOT DONE: row 98` and
  `GATE FAIL: examined 130 data rows, expected 129`.
- **NC-C** — the mandatory check-(3) NC, because check (3) is silent: residual **0**, and
  `NEVER OPENED: gRPC   <- NC FIRED`.
- **NC-D** — `-family row` **under `--`** (the flag trap: a leading `-` makes the pattern look like an
  option, and the surrounding arithmetic then prints `0`, which reads exactly like "no change"):
  occurrences **96**, lines **68**. **Unmoved.**
- **Check-(2) positive control** — ⚠️ **the longer phrase does not CONTAIN the shorter one as a substring**,
  so a control substituting only one reports a residual of 5 and reads like a finding. Both substituted:
  residual **0**, `candidatesXX` **6** — **the substitution count ASSERTED, not assumed.**
- **Escape-aware malformed set:** exactly **{57, 69}**, at file lines **119** and **131**, NF **9** and
  **10**. Run as `sed 's/\\|//g' F | awk -F'|' '/^\| *[0-9]/ && NF!=8'` with **no file argument to awk** —
  passing the file makes awk ignore stdin and print the naive count under the escape-aware label.

### 2.3 Row-shape baseline — measured BEFORE this stage's edits, for the IMPL to diff against

`ROADMAP.md` **248** lines, **130** data rows, tail row **98** at file line **160**, status
**`in-progress`**. Row 98 reads **NF=8 under BOTH the naive and the escape-aware form**, and
`grep -coE` of the two sentinel match phrases against that one line reads **0** — **it spells neither**.
⚠️ **The unescaped-`|` trap is DORMANT at this stage (a PLAN edits no row) and LIVE the moment the IMPL
flips one.** Count fields under both forms before installing or rewriting any row, and reword a pipe AWAY
rather than escaping it.

### 2.4 Probe hygiene and the port band

**This stage took `16100-16199`**, censused first and **each hit named**:
`/usr/bin/grep -rnE '\b161[0-9][0-9]\b'` over `test/ internal/ cmd/ docs/` returned **zero hits of any
kind** — not a port, not a fuzz rate, not a byte size — and `ss -tan` / `ss -uan` over all states returned
zero sockets. ⚠️ **This sentence now spells `16100` and `16199` and is therefore a hit in the next census**
(the self-incrementing class: this file's `156xx` census sentence claimed two and re-read three, because it
spelled both numbers). **Name what each hit IS, and expect your own sentence to be one of them.**

Split three ways on the Docker axis: the reference agent alone used Docker (`16100-16149`, containers
`p98plan-ref-*`, removed BY NAME, `docker ps -a | grep -c '^p98plan-ref-'` → **0**, none foreign touched);
the subject agent used no Docker (`16150-16199`); the controller reused the freed reference band
(`16110-16111`) **after** that agent had exited. Fixture ports `15123 15124 15125 15126 15223 15224 15225`
each read **zero files** under `git grep -l '\b<port>\b' -- test/ internal/ cmd/` and zero sockets —
**re-censused at this tip, not inherited.** Three throwaway worktrees were created and removed; every
config, log and binary lives in scratch.

### 2.5 The `PROPOSED` guard — ARMED, AND THIS STAGE LEAVES IT ARMED

**Copied, never paraphrased** — a guard re-spelled from memory has on this project reported a parenthetical
**ZERO** and a DISARMED gate that was in fact ARMED, and an absence reads as permission.

| form | at this tip | resolves to |
|---|---|---|
| `^> \*\*STATUS: PROPOSED` — **the house form** | **ARMED**, one hit at `:19217` | backward `^## ADR-` search → **`## ADR-0320`** (heading at `:19215`) |
| `^\*\*Status:\*\* PROPOSED` — **the ADR-0231 decoy** | one hit at `:14866` | backward search → **`## ADR-0231`** |
| `^\*\*Status:\*\*.*PROPOSED` — the middle ground | a large number | ⚠️ **NEVER A GATE** |

⚠️⚠️ **BOTH FORMS HIT RIGHT NOW, FOR UNRELATED REASONS.** The decoy alone reads as a disarmed guard.
**Verify BY LINE AND BY ADR, never by the count.** ⚠️ **A PLAN drafts no ADR and touches no
`DECISIONS.md`** (§1.1 measured that across five precedents) — **this stage leaves the guard ARMED, and
that is correct.** The IMPL appends §Decision + §Consequences **after** the retained italic footer and
flips `PROPOSED` → `ACCEPTED`, with **no `**Status:**` line, no renumber and no `---`**.

### 2.6 The eviction instruments — a PAIR, run on BOTH files

⚠️ **The bare forms answer nothing** — both read the same value on `STATE.md` and both are invariant under
*which* entry is evicted. **Only the LABEL-BOUND form discriminates**, on both files: the evictee's own
label goes `STATE.md` **1 → 0** and `STATE_HISTORY.md` **0 → 1**. A **fabricated-label NC** must read 0 in
both, and the **positive control must name a label that IS in the archive and ABSENT from §Recent** — a
§Recent label proves nothing.

**The pre-roll shape, READ not inherited.** §Recent in list order, with dates:

```
1  phase 98 (chain-match-transport-protocol-reject) BRAINSTORM done   (2026-09-12)
2  phase 97 (quic-chain-selection-order) IMPL done                    (2026-09-09)
3  phase 97 (quic-chain-selection-order) PLAN done                    (2026-09-09)
4  phase 97 (quic-chain-selection-order) SPEC done                    (2026-09-09)
5  phase 97 (quic-chain-selection-order) BRAINSTORM done              (2026-09-09)
```

Histogram: **one `09-12`, four `09-09`** — a **FOUR-WIDE TIE AT THE TAIL**, exactly as the phase-98 SPEC
close predicted. ⚠️ **The date read CANNOT pick alone here; LIST POSITION picks**, and it picks entry **5**,
`phase 97 (quic-chain-selection-order) BRAINSTORM done`. **Read the shape every time; inherit none of them,
including this one.**

Archive as **ONE INLINE LINE** in the **parenthetical** form. Raw delta is **`+2`, not `+1`** — a blank
line plus the entry line. Strict guard **DELTA 0**. ⚠️ **Name the LABEL, never the COUNT**: an archive line
that names a label as its positive control changes that label's own count in the very file it measures, in
the same commit.

### 2.7 The archive guard triple — re-derived, and it reconciles

`H=docs/envoy-go/STATE_HISTORY.md` at **574** lines:

```sh
/usr/bin/grep -oE '^- \*\*prior active-phase:\*\* ' $H | wc -l   # strict         163  <- DELTA 0 across a correct close
/usr/bin/grep -oE '^- \*\*prior active-phase \('    $H | wc -l   # parenthetical   74  <- +1 per close
/usr/bin/grep -oE '^- \*\*prior active-phase'       $H | wc -l   # loose          237
```

**163 + 74 = 237 exactly.** ⚠️ **The strict 163 is NOT the entry count** — it is a subset, and the guard is
sound as a **DELTA-0 SHAPE check**, not as a total. The unanchored `grep -c 'prior active-phase'` counts
LINES and coincides by accident; the colon form is a fourth regex. ⚠️ **`grep -F` with a pattern starting
`- ` hits the flag trap — pass `--` first.** **Name the form or the figure is meaningless.**

---

## 3. STABLE ANCHORS — use these, never line numbers

⚠️ **Line anchors DRIFT; symbol and literal-text anchors do not.** Every anchor below was re-derived at
`bd303d87` by enclosing symbol or by `grep -nF`. The line numbers are given **only** so a reader can find
the neighbourhood; **relocate by the LITERAL before editing**, and expect this stage's own numbers to move
under the IMPL's own edits — it happened twice inside one stage at phase 96.

| # | anchor (use THIS) | at `bd303d87` | what lands here |
|---|---|---|---|
| A1 | `func parseChainSpec(name string, fm *listenerv3.FilterChainMatch) (*listenerfilter.ChainSpec, error) {` | `manager.go:969` | enclosing symbol for edit 1 |
| A2 | `// transport_protocol: validate against the v3 enum domain. "quic" is` | `manager.go:991` | **first of 3 comment lines DELETED** |
| A3 | `switch tp := fm.GetTransportProtocol(); tp {` | `manager.go:994` | **first of 6 switch lines DELETED** |
| A4 | `return nil, fmt.Errorf("transport_protocol %q must be \"tls\", \"raw_buffer\", \"quic\", or empty", tp)` | `manager.go:998` | the reject message that dies |
| A5 | `func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {` | `manager.go:1322` | enclosing symbol for edit 2 |
| A6 | `// continue_on_listener_filters_timeout=true: fall through with partial inputs.` | `manager.go:1364` | the fall-through the install must cover |
| A7 | `// (5) Run chain-match algorithm.` | `manager.go:1367` | **the stamp is installed IMMEDIATELY ABOVE this line** |
| A8 | `selectedSpec, err := listenerfilter.SelectChain(inputs, rt.chainSpecs, rt.defaultSpec)` | `manager.go:1368` | the call the stamp must precede |
| A9 | `func TestParseChainSpecRejectsUnknownTransportProtocol(t *testing.T) {` | `manager_test.go:1691` | **RE-POINTED**; block `:1688-1715` = 3 doc + 25 func = **28 lines** |
| A10 | `func TestUnifiedDispatchPlaintextChainSelectByDestPort(t *testing.T) {` | `manager_test.go:3459` | **the driven-arm TEMPLATE**, span `:3459-3538` = 80 lines |
| A11 | `func TestUnifiedDispatchDefaultFilterChainFallback(t *testing.T) {` | `manager_test.go:3612` | the fallback template |
| A12 | `const testAlphaCertPEM = ` | `manager_test.go:587` | the inline PEM pair arm (s3) reuses — **ship no PKI** |
| A13 | `func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Peeker, inputs *ChainMatchInputs, timeoutMs uint32) (retErr error) {` | `listenerfilter/pipeline.go:32` | the `len(filters) == 0` early return of §0.2 |
| A14 | `func (p *peekerConn) Peek(n int) ([]byte, error) {` | `listenerfilter/callbacks.go` | the deadline-free `bufio` read of §0.1 |
| A15 | `type ListenerFilter interface {` | `listenerfilter/types.go:72` | the one-implementation proof of §0.2 |

**The arithmetic the two edits must reconcile to.** A2 opens a **3-line** comment (`:991-993`); A3 opens a
**6-line** `switch` (`:994-999`). **3 + 6 = 9 deleted**, replaced by **one** assignment line ⇒ `+1 / −9` for
edit 1. Edit 2 adds **3** lines at A7 ⇒ `+3 / −0`. **Total `+4 / −9`**, which is exactly the SPEC's measured
prototype. ⚠️ **If your diff does not read `4 9` on `--numstat`, one of the two edits is wrong** — that is
the cheapest possible gate on the production change and Task 9 and Task 10 each assert it.

⚠️ **ONE install at A7 covers BOTH paths.** The `continue_on_listener_filters_timeout` fall-through at A6
does **not** return — it falls out of the enclosing `if` block and reaches A8 like any other connection.
So anchoring on the **entry of selection** rather than "after the pipeline" satisfies method note 3f with a
single line. ⚠️ **But per §0.2 that coverage is structurally unobservable today**: install it there because
it is correct and free, not because an arm proves it.

⚠️ **THE STALE ANCHOR THE SPEC FLAGGED IS REAL AND IT IS WRONG IN BOTH HALVES.** `quic_test.go`'s two
comments cite `manager.go:985-991` for the enum switch. At this tip the **comment** begins at `:991` and
the **switch** occupies `:994-999`. The cited range names neither. **Both citations die with the switch**
(§7).

---

## 4. The three adjudications `SPEC.md` §14 item 3 demands — EACH EXPLICIT

| # | question the SPEC left open | **VERDICT** | mechanism / measurement |
|---|---|---|---|
| a | §11 **NC roster row 6** — stamp moved into `SelectChain` | **SURVIVES**, with a **new** arm named, in a **new** file | §0.13: zero existing tests redden (BLIND); `TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain` built and run RED under the mutation, GREEN un-mutated, matched negative GREEN both ways. Costs `chainmatch_test.go`, declared in §9. |
| b | §2.3's **listener-filter TIMEOUT arm** | **MEASURED on the reference; STRUCK as a test arm** | §0.3: reference serves the `raw_buffer` chain on the timeout path, matched negative falls to default, `continue_on…: false` drops the connection. §0.2: on the subject the arm is **not constructible** — `tls_inspector` is the only listener filter and **all five of its return paths write `TransportProtocol`**, so the `== ""` guard can never fire there. §0.1 proves it by execution: the arm is `INDEXED` at the **un-fixed** tip. |
| c | §5.3's **QUIC unit arm** for a bogus value | **ADD ONE**, and say what it is for | §0.13's call-site enumeration plus §5.3 below. It is the **only committed arm that reddens under NC roster row 1 via the QUIC listener-construction path**, and without it the four `quic_test.go` narration edits are an unpinned prose change and §2.2's Q2/Q3 reference evidence has **zero** falsifiable coverage. |

⚠️ **(b) is struck with a NAMED MECHANISM, never as a bare absence** (method note 73). The *placement* at
anchor A7 is retained unchanged — it is correct and free, and NC roster row 5b exists precisely to keep the
`== ""` guard honest against a future listener filter that leaves the field empty.

⚠️ **The honest counter-argument to (c), recorded rather than smoothed:** under NC row 1, §5.1 property (a)
**also** reddens, so the QUIC arm's *marginal* discriminating power is limited to a kind-scoped restoration
of the gate — a mutation nobody would plausibly write. It is ~60 lines of test. **It is still ADDED**,
because this row edits four QUIC narration sites and a prose edit with no arm behind it is exactly the
shape this project's method notes exist to prevent. **The PLAN records the weakness rather than pretending
the arm is stronger than it is.**

---

## 5. Test design, re-derived against the REAL API

⚠️ **The SPEC's agent drafted §5.1's source in a scratch directory that is GONE with that session. Do NOT
look for it.** Everything below was re-derived at this tip by reading the API and by execution.

### 5.1 The re-pointed parse test

`TestParseChainSpecRejectsUnknownTransportProtocol` (block `:1688-1715`) is **REPLACED** by
`TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue`. **Re-pointed, not relaxed:** the old
test pinned only that a message named the bad value; the new one pins storage, non-eligibility against
three realistic inputs, and enforcement.

**The listener.** `l_tp`, plaintext, port 0. `filter_chains[0]`: `transport_protocol: "sctp"`.
`filter_chains[1]`: `source_type: SAME_IP_OR_LOOPBACK` and **nothing else**. No `default_filter_chain`.
⚠️ **The sibling MUST be `source_type`, not a `raw_buffer` or empty-match chain** — the latter boot-rejects,
verbatim: `listener: "l_tp": at most one filter_chain may omit filter_chain_match.server_names (catch-all);
got 2` (§0.14, confirmed by execution).

**The API it drives**, re-derived:

```go
// internal/listener/listenerfilter/chainmatch.go:80
func SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error)
// sentinels: ErrNoChainMatched (:52), ErrAmbiguousChainMatch (:59)

// internal/listener/listenerfilter/types.go:32-52 — fields IN ORDER
type ChainMatchInputs struct {
	DestinationIP        net.IP
	DestinationPort      uint32
	SourceIP             net.IP      // MUST be loopback on every probe input
	SourcePort           uint32
	ServerName           string
	TransportProtocol    string
	ApplicationProtocols []string
}

// internal/listener/manager.go:160-163
chainSpecs   []*listenerfilter.ChainSpec
defaultSpec  *listenerfilter.ChainSpec
```

The precedent caller shape already in the package is `manager_test.go:1236`:
`listenerfilter.SelectChain(inputs, rt.chainSpecs, rt.defaultSpec)`.

**Four properties, one `t.Errorf` each, each message naming its own property** (method note 7e — a shared
`t.Helper()` body collapses every property onto one line):

| prop | asserts | caught by (NC roster) |
|---|---|---|
| (a) | `NewManager` succeeds — `t.Fatalf`, because nothing below is reachable without a manager | **row 1** (restore the enum switch); RED at the un-fixed tip |
| (b) | the parsed `TransportProtocol` is **byte-exactly** `"sctp"` | **row 2** (parse stores `""`) |
| (c) | for detected `""`, `"tls"`, `"raw_buffer"`, real `SelectChain` does **not** pick `filter_chains[0]` **and does** pick `filter_chains[1]` | **row 3** (delete the clause in `matches()`) |
| (d) | detected `"sctp"` **does** pick `filter_chains[0]` — the dimension is enforced, not ignored | **row 2** |

⚠️ **(c) is BLIND to row 2 and (d) is BLIND to row 3 — NEITHER HALF IS A GATE ALONE; THE PAIR IS.** An
emptied `filter_chains[0]` loses to the sibling on specificity, so (c) still passes; with the clause gone,
`"sctp"` still wins on specificity, so (d) still passes. **Score the NC roster PER PROPERTY, never per
run.**

⚠️ **Assert the chain by its INDEX-DERIVED name** — `"l_tp/filter_chains[0]"` and
`"l_tp/filter_chains[1]"`, **not** any `Name` set on the proto (§0.14).

**Reproduced at the un-fixed tip, at the `SelectChain` layer** (property (a) is the un-fixed RED, so
`NewManager` cannot be used for (c)/(d) until edit 1 lands):

```
(c) detected=""           -> l_tp/filter_chains[1]
(c) detected="tls"        -> l_tp/filter_chains[1]
(c) detected="raw_buffer" -> l_tp/filter_chains[1]
(d) detected="sctp"       -> l_tp/filter_chains[0]
```

**All four §5.1 expectations reproduce.** Specificity confirmed by execution, not by reading the priority
table: `specificityScore(transport_protocol only) = 16` against `specificityScore(source_type only) = 4`.

### 5.2 The stamp-site arms — OWED, because the stamp turns ZERO tests RED

`SPEC.md` §3.1 measured it: the `+3`-line stamp reddens **nothing** in a 392-test sweep. That is a
**coverage finding, not a clean bill** (method note 50). These arms are its only unit-level falsifiability.

**The template is in the tree** — `TestUnifiedDispatchPlaintextChainSelectByDestPort`
(`manager_test.go:3459-3538`, **80 lines**). It already does everything these arms need: binds a probe
listener at port 0 to learn the port, builds a two-chain listener, `Start`s a real manager, dials a **real
loopback TCP** connection, and discriminates which chain served via `startTaggedBackend(t, 'A')` /
`'B'` — a **per-chain discriminator the test controls**, exactly as §5.2 requires. ⚠️ **Never
`net.Pipe`**, and never "the connection was not closed" — a default chain also keeps it open.

| arm | shape | expected | proves |
|---|---|---|---|
| **(s1)** | chain `transport_protocol: raw_buffer`, **no `listener_filters`**, plaintext client | the `raw_buffer` chain serves | **RED at the un-fixed tip.** The divergence itself. |
| **(s2)** | chain `transport_protocol: tls`, byte-identical otherwise | the **default** chain serves | **Matched negative of (s1)**: green for a stamp writing `raw_buffer`; RED for a stamp writing any constant a `tls` chain equals, and RED for a selector ignoring the dimension. |
| **(s3)** | chain `transport_protocol: tls` terminating TLS, **WITH `tls_inspector`**, a **TLS** client | the `tls` chain serves | **The stamp does not OVERWRITE a classified input.** |
| ~~(s4)~~ | ~~the timeout fall-through~~ | — | **STRUCK — NOT CONSTRUCTIBLE**, §0.2 / §4(b). |

⚠️ **(s3) CANNOT be a plaintext arm, and the reason is mechanical:** `tls_inspector` stamps `raw_buffer` on
plaintext, so an unconditional `raw_buffer` stamp would overwrite it **with the same value** and every
plaintext arm would stay green. Only an input the inspector classified as something **other** than the
default discriminates. This is the error `SPEC.md` §0.10 caught in its own first draft — **do not
reintroduce it.**
⚠️ **(s3) ships NO PKI:** reuse `testAlphaCertPEM` / `testAlphaKeyPEM`, already declared at
`manager_test.go:587` / `:600`.

### 5.3 The QUIC arm — ADDED (§4(c))

`TestQUICChainSelection_TransportProtocolBogusDoesNotMatch`, a **third sibling** to the existing pair
`TestQUICChainSelection_TransportProtocolQUICMatches` (`quic_test.go:695`) and
`…TransportProtocolTLSDoesNotMatch` (`:772`). Built with the in-scope helper `mkQUICListenerChains`
(`manager_test.go:1049`), value `"totally_bogus_value"`, expecting the **QUIC-TLS default slot**.

**What it does and does not add.** At the **runtime** layer it adds nothing new: `matches()`
(`chainmatch.go:128`) compares `c.TransportProtocol != inputs.TransportProtocol`, and `"tls" != "quic"` and
`"totally_bogus_value" != "quic"` traverse the **identical** branch, already pinned by the `tls` sibling.
At the **parse** layer it adds nothing either: `parseChainSpec(name string, fm *listenerv3.FilterChainMatch)`
takes **no listener-kind argument**, so a QUIC parse arm executes byte-identical code to §5.1's TCP arm.
**What it uniquely traverses is the QUIC-kind `buildListenerRuntime` boot path with a bogus value** —
which is exactly where the un-fixed tip fails (`SPEC.md` §3.3: un-fixed FAIL at the boot step, prototype
PASS).

### 5.4 What is deliberately NOT added

- **No `TestNoNewStat*` guard.** The row registers no stat name (§9): the parse edit touches no registry,
  and the stamp writes a local struct field read only by `SelectChain`.
- **No case-folding arm.** The reference is case-**SENSITIVE** on this dimension (`SPEC.md` §2.1 T5,
  `RAW_BUFFER` → `DEFAULT`), unlike SNI. ⚠️ **Agreeing case semantics on one dimension do not generalise to
  another.**
- **No timeout arm** — §4(b), with its mechanism named.
- **No cross-side `downstream_pre_cx_timeout` pin.** The subject emits no such name at all
  (`git grep -c 'pre_cx' -- '*.go'` → **0 files**; positive control `downstream_cx_total` → 3 files), so a
  cross-side pin would be **silently vacuous, not RED** (method note 46). This belongs to the banked row in
  §10, not to this one.

---

## 6. Tasks

**ORDERING IS LOAD-BEARING.** Tasks 1-10 land and RUN at the **UN-FIXED TIP**, because that tip **IS** NC
roster row 1's evidence and is unrecoverable afterwards. `SPEC.md` §14 item 2 mandates it and phase 97
confirmed the ordering was load-bearing in execution. **Record which arms are RED and which are
structurally GREEN, PER ARM, not per run** (method note 61: a pre-fix green may be the false-agreement
class inside our own suite, not a bug in the arm).

**Every task ends with a commit** using explicit pathspecs. Append to `PROGRESS.md` on each task
completion (BOOTSTRAP §5 state 3).

---

### Task 1: Prove the anchored panic gate LIVE, and record the un-fixed baseline

**Files:**
- Create: `docs/envoy-go/phases/98-chain-match-transport-protocol-reject/PROGRESS.md`
- Read-only: `internal/listener/manager.go`

**Interfaces:**
- Produces: the un-fixed baseline roster that Task 10 diffs against, and a **proven-live** panic gate every
  later task reuses.

- [ ] **Step 1: Resolve the selectors before believing any FAIL.** A `go test` selector naming a package
      that does not exist prints `FAIL … [setup failed]` and exits 1.

```sh
go list ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...
```

- [ ] **Step 2: Record the un-fixed baseline.** `-count=1 -v`; rc from `PIPESTATUS[0]`, **never** from a pipe.

```sh
go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... \
        ./internal/listener/... ./validate/... > /tmp/base.txt 2>&1
rc=${PIPESTATUS[0]}; echo "RC=$rc"
grep -c '=== RUN' /tmp/base.txt            # expect ~392; RUN=0 beside RC=0 is a VACUOUS GREEN
grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL' /tmp/base.txt   # anchored; the unanchored form reads nonzero on a green tree
grep -cE '^panic:|DATA RACE|SIGSEGV' /tmp/base.txt      # the anchored panic gate
```

- [ ] **Step 3: Prove the panic gate FIRES.** A gate that reads 0 has not been shown to work. Insert a bare
      `panic("p98 gate probe")` at anchor **A5** (`serveConnection`'s first line), run **one** driven test,
      confirm the gate reads **>= 1**, then revert and confirm it reads 0 again. ⚠️ **There is no
      `recover()` in non-test `internal/listener`, so a panic aborts the binary.** ⚠️ **A fail-fast control
      measures ONE site per run — isolate.**

- [ ] **Step 4: Expect and record the known flake.** `TestEnvoyGoBinary_TwoListenerCutover` (`cmd/envoy-go`)
      may fail with `bind 127.0.0.1:<port>: address already in use` on a port inside `32768-60999`. It fired
      **twice at two tips in one session** at the phase-98 SPEC. **It is not this row's regression** — it
      fires at the un-fixed tip too — and **a green rerun clears nothing.** Record the port; do not chase it.

- [ ] **Step 5: Create `PROGRESS.md`** with the Task 1 entry: the resolved selector list, `RC`, the
      `=== RUN` count, the anchored FAIL count, the panic-gate reading **before, during and after** the
      probe, and any flake port.

- [ ] **Step 6: Commit.**

```sh
git add docs/envoy-go/phases/98-chain-match-transport-protocol-reject/PROGRESS.md
git commit -m "phase 98 (chain-match-transport-protocol-reject) IMPL Task 1: un-fixed baseline + panic gate proven live"
```

---

### Task 2: The §5.1 re-pointed parse test — RED at the un-fixed tip

**Files:**
- Modify: `internal/listener/manager_test.go` — **replace** the block at `:1688-1715` (anchor **A9**)

**Interfaces:**
- Consumes: `listenerfilter.SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error)`; `listenerfilter.ErrNoChainMatched`; `rt.chainSpecs`, `rt.defaultSpec`.
- Produces: `TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue`, referenced by NC roster
  rows 1, 2 and 3.

- [ ] **Step 1: Delete the old block by LITERAL anchor**, not by line number:
      `grep -nF -- 'func TestParseChainSpecRejectsUnknownTransportProtocol(t *testing.T) {' internal/listener/manager_test.go`,
      then remove the 3-line doc comment above it through the closing brace (28 lines total).

- [ ] **Step 2: Write the replacement.** Listener `l_tp`, plaintext, port 0; `filter_chains[0]`
      `FilterChainMatch{TransportProtocol: "sctp"}`; `filter_chains[1]`
      `FilterChainMatch{SourceType: listenerv3.FilterChainMatch_SAME_IP_OR_LOOPBACK}` and nothing else; no
      `default_filter_chain`. Four properties, **one `t.Errorf` each, each message naming its property**:

```go
// (a) NewManager succeeds — t.Fatalf: nothing below is reachable without a manager.
mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
if err != nil {
	t.Fatalf("(a) NewManager must ACCEPT transport_protocol \"sctp\": %v", err)
}
rt := mgr.runtimeFor("l_tp") // resolve by the package's own accessor; see manager_test.go:1236
// (b) storage is byte-exact.
if got := rt.chainSpecs[0].TransportProtocol; got != "sctp" {
	t.Errorf("(b) parsed TransportProtocol: got %q, want byte-exactly %q", got, "sctp")
}
// (c) three realistic detected values must NOT pick fc[0] and MUST pick fc[1].
for _, detected := range []string{"", "tls", "raw_buffer"} {
	in := listenerfilter.ChainMatchInputs{SourceIP: net.ParseIP("127.0.0.1"), TransportProtocol: detected}
	got, err := listenerfilter.SelectChain(in, rt.chainSpecs, rt.defaultSpec)
	if err != nil {
		t.Errorf("(c) detected=%q: SelectChain: %v", detected, err)
		continue
	}
	if got.Name != "l_tp/filter_chains[1]" {
		t.Errorf("(c) detected=%q: picked %q, want %q — an unknown value must be INELIGIBLE, not a wildcard",
			detected, got.Name, "l_tp/filter_chains[1]")
	}
}
// (d) the dimension is ENFORCED, not ignored.
in := listenerfilter.ChainMatchInputs{SourceIP: net.ParseIP("127.0.0.1"), TransportProtocol: "sctp"}
got, err := listenerfilter.SelectChain(in, rt.chainSpecs, rt.defaultSpec)
if err != nil || got.Name != "l_tp/filter_chains[0]" {
	t.Errorf("(d) detected=\"sctp\": picked %v (err %v), want %q — the dimension must be ENFORCED",
		got, err, "l_tp/filter_chains[0]")
}
```

- [ ] **Step 3: Assert the chain by its INDEX-DERIVED name.** `"l_tp/filter_chains[0]"` /
      `"l_tp/filter_chains[1]"`. ⚠️ **A `Name` set on the proto `FilterChain` does NOT propagate** (§0.14) —
      an assertion against `"fc0"` fails for a reason unrelated to chain matching.

- [ ] **Step 4: Every probe input carries loopback `SourceIP`.** `IsLoopbackSource()` is
      `c.SourceIP != nil && c.SourceIP.IsLoopback()`, so **nil is NOT loopback** — omitting it makes the
      sibling ineligible and the whole test vacuous.

- [ ] **Step 5: Run it; expect RED at (a).**

```sh
go test -count=1 -v -run TestParseChainSpecAcceptsUnknownTransportProtocolAsNonMatchingValue ./internal/listener/
```
      Expected: **FAIL at (a)** with `transport_protocol "sctp" must be "tls", "raw_buffer", "quic", or empty`.
      ⚠️ **A `-run` selector matching nothing prints `[no tests to run]` and EXITS 0** — confirm the test
      actually ran by checking for its `=== RUN` line.

- [ ] **Step 6: Record and commit** the verbatim failure into `PROGRESS.md`, then
      `git add internal/listener/manager_test.go docs/…/PROGRESS.md && git commit`.

---

### Task 3: §5.2 arms (s1) and (s2) — a MATCHED PAIR, one string apart

**Files:**
- Modify: `internal/listener/manager_test.go` — append after the Task 2 block

**Interfaces:**
- Consumes: `startTaggedBackend(t, 'A')`, `twoClusterMgr`, `mkTcpProxyFilter`, `mkBoot`, `NewManager`,
  `mgr.Start(ctx)`, `mgr.Listeners()[0].Addr` — all used by the template at **A10**.
- Produces: `TestServeConnection_NoListenerFilter_RawBufferChainServes` (s1) and
  `TestServeConnection_NoListenerFilter_TLSChainDoesNotServe` (s2), referenced by NC roster rows 3, 4, 5, 5b.

- [ ] **Step 1: Clone the template.** `TestUnifiedDispatchPlaintextChainSelectByDestPort`
      (`manager_test.go:3459-3538`, 80 lines) already binds a probe listener at port 0, builds a two-chain
      listener, starts a real manager, dials a **real loopback TCP** connection, and discriminates the
      served chain by a **tagged backend**. Copy that structure.

- [ ] **Step 2: (s1).** `filter_chains[0]`: `FilterChainMatch{TransportProtocol: "raw_buffer"}` →
      `startTaggedBackend(t, 'A')`. `default_filter_chain` → `startTaggedBackend(t, 'B')`.
      **No `listener_filters` on the listener.** Dial, write a byte, read the tag. **Assert the tag is
      `'A'`**, naming the property in the message.

- [ ] **Step 3: (s2), byte-identical except ONE string.** Same shape with
      `TransportProtocol: "tls"`. **Assert the tag is `'B'`.** ⚠️ **This is the matched negative that makes
      (s1) mean anything** — a single positive arm cannot distinguish "the input was stamped and matched"
      from "the dimension is unenforced" (method note 55).

- [ ] **Step 4: Use a discriminator the test CONTROLS.** The tagged backend, or distinct response bytes —
      **never** "the connection was not closed": a default chain also keeps it open. ⚠️ **Never
      `net.Pipe`** — it deadlocks a client-cert handshake and is the wrong shape here regardless.

- [ ] **Step 5: Run both; record the verdict PER ARM.**

```sh
go test -count=1 -v -run 'TestServeConnection_NoListenerFilter_' ./internal/listener/
```
      Expected at the un-fixed tip: **(s1) RED** (serves `'B'`, the default chain — the divergence itself)
      and **(s2) GREEN**. ⚠️ **(s2)'s green is STRUCTURAL, not evidence**: the subject answers "default
      chain" here for the same reason it answers it in (s1), and *default* happens to be the right answer.
      **Record it as structurally-green, not as a pass** (method note 61).

- [ ] **Step 6: Commit** with the per-arm roster recorded in `PROGRESS.md`.

---

### Task 4: §5.2 arm (s3) — the stamp must NOT overwrite a classified input

**Files:**
- Modify: `internal/listener/manager_test.go`

**Interfaces:**
- Consumes: `testAlphaCertPEM` / `testAlphaKeyPEM` (`manager_test.go:587` / `:600`), `testLFRegistry()`,
  `tls_inspector.TypeURL` = `type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector`.
- Produces: `TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten`, referenced by NC roster rows
  5 and 5b.

- [ ] **Step 1: Build the listener.** `listener_filters: [tls_inspector]`; `filter_chains[0]`
      `FilterChainMatch{TransportProtocol: "tls"}` with a `DownstreamTlsContext` using the **inline** alpha
      PEM pair → tagged backend `'A'`; `default_filter_chain` → tagged backend `'B'`. **Ship no PKI.**

- [ ] **Step 2: Drive a REAL TLS client** over a loopback TCP pair (`crypto/tls.Client` with
      `InsecureSkipVerify` against the alpha cert, or the package's existing helper if one exists). ⚠️ **A
      probe client can withhold the very thing the arm exists to test** — the ClientHello must actually be
      sent, and the arm must read the served tag **after** the handshake, not merely observe that the
      handshake completed.

- [ ] **Step 3: Assert the tag is `'A'`.** The `tls` chain serves, because `tls_inspector` classified the
      input as `"tls"` and the stamp's `== ""` guard did not fire.

- [ ] **Step 4: Write the WHY into the test's doc comment**, because the next reader will try to simplify
      it: ⚠️ **a PLAINTEXT arm cannot carry this proof.** `tls_inspector` stamps `raw_buffer` on plaintext,
      so an unconditional `raw_buffer` stamp would overwrite it **with the same value** and every plaintext
      arm would stay green. Only an input the inspector classified as something **other** than the default
      discriminates. This is the error `SPEC.md` §0.10 caught in its own first draft.

- [ ] **Step 5: Run it.** Expected at the un-fixed tip: **GREEN** — `tls_inspector` already writes `"tls"`,
      and nothing in this arm depends on the stamp. ⚠️ **Its only falsifiability is NC roster rows 5 and
      5b** (stamp unconditionally with `"tls"`; drop the `== ""` guard). **Score it there, per arm.**

- [ ] **Step 6: Commit.**

---

### Task 5: §5.3 — the QUIC bogus-value arm

**Files:**
- Modify: `internal/listener/quic_test.go`

**Interfaces:**
- Consumes: `mkQUICListenerChains` (`manager_test.go:1049`); siblings
  `TestQUICChainSelection_TransportProtocolQUICMatches` (`quic_test.go:695`) and
  `…TransportProtocolTLSDoesNotMatch` (`:772`).
- Produces: `TestQUICChainSelection_TransportProtocolBogusDoesNotMatch`, referenced by NC roster row 1.

- [ ] **Step 1: Write it as a THIRD SIBLING** to the existing pair, same shape, value
      `"totally_bogus_value"`, expecting the **QUIC-TLS default slot**. Follow the two siblings' house
      style — ⚠️ **both QUIC test files are table-FREE under every matcher form; do not introduce a table.**

- [ ] **Step 2: Put the JUSTIFICATION in its doc comment**, in one sentence, because §4(c) records that its
      marginal power is narrow: *it is the only committed arm that reddens under NC roster row 1 via the
      QUIC listener-construction path, and without it the four `quic_test.go` narration edits of Task 12
      are an unpinned prose change.*

- [ ] **Step 3: Run it; expect RED at the BOOT step** at the un-fixed tip, with the
      `transport_protocol "totally_bogus_value" must be …` message. ⚠️ **Prove the reproduction REACHED the
      site by the MESSAGE, never by the exit code** — an unrelated boot reject would mask it.

- [ ] **Step 4: Commit.**

---

### Task 6: NC roster row 6's arm — the `listenerfilter` purity pin

**Files:**
- Modify: `internal/listener/listenerfilter/chainmatch_test.go`

**Interfaces:**
- Produces: `TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain`, the **only** arm in the tree
  that can fire NC roster row 6.

- [ ] **Step 1: Declare the scope widening.** ⚠️ `chainmatch_test.go` is **not** in `SPEC.md` §6.3's edit
      set. Adding it is legal — it is not on the byte-untouched roster either — **but it must be declared in
      `PROGRESS.md`, not allowed to appear** (§0.13).

- [ ] **Step 2: Write the arm.**

```go
// TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain pins that
// SelectChain does NOT default an empty detected transport protocol. The TCP
// default lives in serveConnection (ADR-0320), deliberately, because
// SelectChain is shared with the QUIC path (quic.go:187), whose input is
// always "quic". This is the ONLY arm in the tree that fires NC roster row 6.
func TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain(t *testing.T) {
	chains := []*ChainSpec{{Name: "rb", TransportProtocol: "raw_buffer"}}
	got, err := SelectChain(ChainMatchInputs{}, chains, nil)
	if !errors.Is(err, ErrNoChainMatched) {
		t.Errorf("SelectChain(empty TP, raw_buffer chain, no default) = (%v, %v); want (nil, ErrNoChainMatched) — SelectChain must NOT default the input", got, err)
	}
}
```

- [ ] **Step 3: Add its MATCHED NEGATIVE** in the same function or a sibling: with
      `ChainMatchInputs{TransportProtocol: "tls"}` and a `tls` chain, `SelectChain` **must** pick it. ⚠️
      **This proves the arm is not a blanket detector** — measured during the PLAN, it stays GREEN under
      both the mutation and the un-mutation.

- [ ] **Step 4: Run; expect GREEN at the un-fixed tip and GREEN after both production edits.** Its
      falsifiability is **entirely** NC roster row 6 — Task 16 scores it there. **A `+0/+0` arm like this
      can be silently deleted with every gate staying green: Task 19 diffs the ARM ROSTER, not the
      counters.**

- [ ] **Step 5: Commit.**

---

### Task 7: Fixture `0123-listener-transport-protocol` — directory, driver, rendered configs

**Files:**
- Create: `test/fixtures/0123-listener-transport-protocol/driver/driver.go`

**Interfaces:**
- Consumes: `fixture.RegisterFixture`, `fixture.MultiListenerDriver`, `fixture.StatsAsserter`.
- Produces: `SubjectListenerNames() []string`, `ReferenceListenerPorts() []int`,
  `SubjectListenerName() string`, `ReferenceListenerPort() int`, `DriveReferenceMulti`, `DriveSubjectMulti`,
  `BackendCount() int`, `ReferenceConfig()`, `SubjectConfig()`.

- [ ] **Step 1: Take the SHAPE from `0122` and the ARITY from `0018`.** `0122-quic-chain-selection` is
      **three files / 946 lines** with **no committed YAML** — both configs are rendered in the driver; copy
      that. ⚠️ **`SPEC.md` §7.1 cites `0008` as the multi-listener precedent, but `0008` returns TWO ports**
      and additionally carries an `AlternateConfigDriver`, a `backends/` directory and `sockopts.go`, none
      of which this fixture needs. **The arity-matched precedent is `0018-http-rbac`**
      (`inputs/driver.go:272` returns three) or `0020`.

- [ ] **Step 2: Implement the multi-listener contract EXACTLY.** The runner
      (`test/differential/runner_test.go:1235-1249`) type-asserts `fixture.MultiListenerDriver`,
      **`t.Fatalf`s when `len(SubjectListenerNames()) != len(ReferenceListenerPorts())`**, and zips the two
      **index-wise**. ⚠️ **The single-address `SubjectListenerName()` / `ReferenceListenerPort()` must STILL
      be implemented**, returning listener[0] — the admin-probe path uses them.

- [ ] **Step 3: Three plaintext TCP listeners, NO `listener_filters` on ANY of them.**

| listener | `filter_chains[0]` match | expected BOTH sides | what it falsifies |
|---|---|---|---|
| `l_bogus` | `transport_protocol: totally_bogus_value` | `DEFAULT` | the un-fixed subject **boot-rejects** — the whole fixture fails |
| `l_raw` | `transport_protocol: raw_buffer` | **`INDEXED`** | a reject-lift-only subject |
| `l_tls` | `transport_protocol: tls` | `DEFAULT` | a subject that ignores the dimension, or stamps a constant `tls` matches |

- [ ] **Step 4: Every listener carries a default chain**, with **distinct per-chain `stat_prefix`es** and
      **distinct `direct_response` bodies at status 200**. ⚠️ **envoy-go emits no `no_filter_chain_match`
      counter**, so no arm may depend on it.

- [ ] **Step 5: Ports — RE-CENSUS, do not inherit.** `15123` (primary, on the `15000 + <index>`
      convention), plus `15223` and `15224` off-convention so `0124`/`0125` keep theirs. At this tip all
      seven of `15123 15124 15125 15126 15223 15224 15225` read **zero files** under
      `git grep -l '\b<port>\b' -- test/ internal/ cmd/` and zero sockets under `ss -tan`/`ss -uan`.
      **Re-run both before use.**

- [ ] **Step 6: `BackendCount()` must be >= 1.** The runner rejects 0, and an omitted `clusters` key
      boot-rejects envoy-go — give each side a placeholder static cluster even though every route is a
      `direct_response`.

- [ ] **Step 7: Write the WARNING into the driver's doc comment.** ⚠️ **Adding `tls_inspector` to any
      listener silently DISARMS `l_raw`** — with the inspector present both sides agree even at the un-fixed
      tip (`SPEC.md` §2.1 T2), producing a **GREEN fixture over a live divergence in the exact dimension the
      fixture claims to cover.**

- [ ] **Step 8: Commit.**

---

### Task 8: Fixture `0123` — `expectations.yaml` and `README.md`

**Files:**
- Create: `test/fixtures/0123-listener-transport-protocol/expectations.yaml`
- Create: `test/fixtures/0123-listener-transport-protocol/README.md`

- [ ] **Step 1: Pin the VALUE PAIR, never the NAME.** ⚠️ **A name-presence assertion is VACUOUS on BOTH
      sides** — §0.6 measured that both sides publish both per-chain counters **at `0` before any traffic**
      (the reference pre-traffic scrape read **168** `chain_(indexed|default)` lines with zero connections).
      Per listener, assert **(served chain = 1, non-served chain = 0)**, and rely on the `l_raw` / `l_tls`
      pair — one string apart — as the mirror.

- [ ] **Step 2: Scrape `/stats/prometheus` and project back to the internal name.** ⚠️ **The prometheus
      spelling DIFFERS** from the flat one: `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="<prefix>"}`.
      The precedent is `test/fixtures/0005-prometheus-stats/driver/driver.go:489`, which re-projects
      `envoy_http_downstream_rq_total` onto `http.<prefix>.downstream_rq_total`. **Parse into a map — do not
      pin line order**: flat `/stats` is alphabetical while prometheus is **registration order**.
      ⚠️ On the subject, `/stats?format=json` is **not** a JSON surface (byte-identical to `/stats`) and
      `/stats/json` **404s**.

- [ ] **Step 3: Assert the response BODY per listener** as the primary discriminator, with the counters as
      corroboration. A body is controlled by the fixture; a counter is not.

- [ ] **Step 4: `README.md` must state the two disarming hazards** in its own words: adding a
      `listener_filter` to any listener disarms `l_raw`, and `l_bogus` **alone** is a false-agreement arm
      (a subject that ignores the field answers `DEFAULT` too) — **only the `l_raw` / `l_tls` pair excludes
      a constant answer.**

- [ ] **Step 5: Commit.**

---

### Task 9: The FOUR registration gates, PROVEN and NC'd

**Files:**
- Modify: `test/differential/runner_test.go` — the blank import

- [ ] **Step 1: Satisfy all FOUR gates**, not three (method note 60): (1) `fixture.RegisterFixture` in the
      driver's `init()`; (2) the blank import in `test/differential/runner_test.go`; (3) **byte-identity**
      between the directory name and the registered string; (4) the `NNNN-` directory-name shape
      `discoverFixtures` enumerates. ⚠️ **Gates 1-3 converge on a `t.Skipf`; gate 4 produces no subtest and
      therefore not even a skip line.**

- [ ] **Step 2: Score on the fixture-set SET-DIFFERENCE, never on the exit code.**

```sh
extract () { /usr/bin/grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" \
  | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
extract test/differential/runner_test.go > /tmp/imports.txt
ls -d test/fixtures/*/ | sed -E 's#test/fixtures/##; s#/$##' | sort > /tmp/dirs.txt
comm -23 /tmp/imports.txt /tmp/dirs.txt    # registered but no dir — must be EMPTY
comm -13 /tmp/imports.txt /tmp/dirs.txt    # dir but not registered — must be EMPTY
```
      At the un-fixed tip this reads **124 = 124**, split **100 `driver/` + 24 `inputs/`**, both directions
      empty. ⚠️ **The router's method note 3e says 123/123 and 99+24 — it is STALE** (§0.7). After this task
      it must read **125 = 125**, split **101 + 24**.

- [ ] **Step 3: NC the extractor by RENAME.** Rename one import in a **scratch copy** and confirm both
      `comm` directions fire. ⚠️ **A COUNT-ONLY check is vacuous — the import count is invariant under a
      rename.**

- [ ] **Step 4: NC the extractor by DELETION** in a scratch copy. ⚠️ **A deletion fires ONLY `comm -23`** —
      "both directions" belongs to the rename control alone. **Run both NCs; they are not the same control.**

- [ ] **Step 5: Run the fixture alone**, `-count=1`, and confirm it appears as a real subtest rather than a
      skip:

```sh
go test -count=1 -v -run 'TestDifferential/0123-listener-transport-protocol' ./test/differential/ 2>&1 | tee /tmp/f123.txt
grep -c '=== RUN' /tmp/f123.txt      # a -run matching NOTHING prints [no tests to run] and EXITS 0
grep -c 'SKIP' /tmp/f123.txt         # must be 0 — a registration miss SKIPS, it does not FAIL
```

- [ ] **Step 6: Expect the fixture to FAIL at the un-fixed tip, and say HOW.** `l_bogus` makes the subject
      **boot-reject**, so the whole fixture fails before any arm is scored. **That is the correct un-fixed
      outcome.** Record it.

- [ ] **Step 7: Commit.**

---

### Task 10: RECORD the un-fixed-tip roster — the negative control, SPENT

**Files:**
- Modify: `docs/envoy-go/phases/98-chain-match-transport-protocol-reject/PROGRESS.md`

- [ ] **Step 1: Re-run the full reverse-dependency sweep** exactly as Task 1 did, `-count=1 -v`, rc from
      `PIPESTATUS[0]`.

- [ ] **Step 2: Diff the sorted `=== RUN` ROSTER against Task 1's**, not the counts. ⚠️ **A change measured
      by COUNT is weaker and must be labelled as such** — `SPEC.md` §3.1 records that its own combined-shape
      run was checked by count (392) rather than by roster diff, and says so.

- [ ] **Step 3: Write the PER-ARM roster into `PROGRESS.md`** — this is the evidence Task 16 scores NC
      roster row 1 against, and **it is unrecoverable after Task 11.**

| arm | expected at the un-fixed tip | why |
|---|---|---|
| §5.1 (a) | **RED** | the enum switch still rejects `"sctp"` |
| §5.1 (b)(c)(d) | unreachable | (a) is a `t.Fatalf` |
| §5.2 (s1) | **RED** | the empty input never matches `raw_buffer` |
| §5.2 (s2) | **structurally GREEN** | the default chain is the right answer for the wrong reason |
| §5.2 (s3) | **GREEN** | `tls_inspector` already writes `"tls"`; independent of the stamp |
| §5.3 QUIC | **RED at boot** | the enum switch rejects `"totally_bogus_value"` |
| Task 6 purity arm | **GREEN** | it pins that `SelectChain` does *not* default; true before and after |
| fixture `0123` | **FAILS at boot** | `l_bogus` boot-rejects the subject |

- [ ] **Step 4: Confirm the panic gate still reads 0** and record the flake port if
      `TestEnvoyGoBinary_TwoListenerCutover` fired again.

- [ ] **Step 5: Commit.** ⚠️ **Nothing after this point can recreate this measurement.**

---

### Task 11: Production edit 1 of 2 — LIFT THE REJECT

**Files:**
- Modify: `internal/listener/manager.go` — anchors **A2**, **A3**, **A4**, inside **A1**

- [ ] **Step 1: Relocate by LITERAL, not by line.**
      `grep -nF -- '// transport_protocol: validate against the v3 enum domain. "quic" is' internal/listener/manager.go`
      and `grep -nF -- 'switch tp := fm.GetTransportProtocol(); tp {' internal/listener/manager.go`.
      ⚠️ **Anchor on the ENCLOSING SYMBOL** `func parseChainSpec(` — an anchor can be non-unique.

- [ ] **Step 2: Delete the 3-line comment and the 6-line switch; write one line.**

```go
	spec.TransportProtocol = fm.GetTransportProtocol()
```
      **No case folding, no trimming, no validation.** The reference is case-SENSITIVE and accepts any
      string.

- [ ] **Step 3: Assert the diff shape BEFORE running anything.**

```sh
git diff --numstat -- internal/listener/manager.go    # MUST read exactly: 1	9
```
      ⚠️ **A build is not evidence the edit landed.** If this does not read `1	9`, the comment or the
      switch boundary was misjudged — stop and re-locate.

- [ ] **Step 4: Build, then assert the SYMBOL is gone.**

```sh
go build ./... && echo BUILD_OK
git grep -c -- 'must be \"tls\"' -- internal/listener/manager.go   # want 0 — NOTE THE ESCAPED QUOTES (§0.9)
git grep -c -- 'must be "tls"'  -- internal/listener/manager.go    # the UNESCAPED form reads 0 even BEFORE the edit — it is NOT the gate
```

- [ ] **Step 5: Run the arms. The KEY ASSERTION of this task is what must STAY RED.**

| arm | after edit 1 | why |
|---|---|---|
| §5.1 (a) | **turns GREEN** | the reject is gone |
| §5.1 (b)(c)(d) | **GREEN** | now reachable, and the values already behave |
| §5.3 QUIC | **turns GREEN** | boot no longer rejects |
| **§5.2 (s1)** | **MUST STAY RED** | the stamp has not landed; the input is still `""` |
| **fixture `0123` `l_raw`** | **MUST STAY RED** | same reason |
| §5.2 (s2), (s3), Task 6 arm | **GREEN** | unchanged |

      ⚠️ **If (s1) or `l_raw` turns green here, edit 2 has no falsifiable coverage and something is
      wrong** — that is the whole reason the SPEC ordered the two edits separately.

- [ ] **Step 6: Run the full reverse-dependency sweep**, `-count=1 -v`, and confirm the only newly-RED
      thing is nothing, and the only newly-GREEN things are the four listed above. Record the roster.

- [ ] **Step 7: Commit** `internal/listener/manager.go` and `PROGRESS.md`.

---

### Task 12: Production edit 2 of 2 — THE STAMP

**Files:**
- Modify: `internal/listener/manager.go` — install immediately above anchor **A7**, inside **A5**

- [ ] **Step 1: Relocate by LITERAL.** `grep -nF -- '// (5) Run chain-match algorithm.' internal/listener/manager.go`.
      ⚠️ **This line MOVED under Task 11's own edit** (`−9 / +1` above it) — **re-locate, never arithmetic.**

- [ ] **Step 2: Install three lines immediately ABOVE that comment.**

```go
	// ADR-0320: a TCP connection no listener filter classified is raw_buffer on
	// the reference. Default it here, at the ENTRY of selection, so every path
	// that reaches SelectChain is covered.
	if inputs.TransportProtocol == "" {
		inputs.TransportProtocol = "raw_buffer"
	}
```
      ⚠️ Write it as **3 net added lines** so the total reads `4	9`; fold the comment to fit.

- [ ] **Step 3: Understand WHY it goes here and record it.** The
      `continue_on_listener_filters_timeout` fall-through at **A6** does **not** return — it falls out of
      the enclosing `if` and reaches **A8** like any other connection. **One install at the entry of
      selection covers every path** (method note 3f). ⚠️ **But per §0.2 that fall-through coverage is
      STRUCTURALLY UNOBSERVABLE today**, because `tls_inspector` is the only listener filter and all five
      of its return paths write the field. **Install it because it is correct and free, not because an arm
      proves it**, and do not write a comment claiming the timeout path is covered *by test*.

- [ ] **Step 4: Assert the cumulative diff shape.**

```sh
git diff master --numstat -- internal/listener/manager.go   # MUST read exactly: 4	9
go build ./... && go vet ./internal/listener/... && echo OK
gofmt -l internal/listener/manager.go                       # gate on OUTPUT — gofmt NEVER exits non-zero
```

- [ ] **Step 5: Run the arms. Now they turn.**

| arm | after edit 2 |
|---|---|
| **§5.2 (s1)** | **GREEN** — the `raw_buffer` chain serves |
| **fixture `0123` `l_raw`** | **GREEN** — `INDEXED` on both sides |
| §5.2 (s2), (s3) | **stay GREEN** |
| everything from Task 11 | **stays GREEN** |

- [ ] **Step 6: Commit.**

---

### Task 13: ALL ARMS GREEN, plus the build, race and lint gates

**Files:** none — this task only runs and records.

- [ ] **Step 1: The full reverse-dependency sweep**, `-count=1 -v`, rc from `PIPESTATUS[0]`, anchored FAIL
      matcher `^(FAIL|--- FAIL)|^ *--- FAIL`, anchored panic gate `^panic:|DATA RACE|SIGSEGV`.

- [ ] **Step 2: `-race` on the FULL `internal/listener` package.** ⚠️ **`-race` on the DIFFERENTIAL suite
      is VACUOUS** — the subject there is an unraced subprocess. Run it where the code actually is:

```sh
go test -race -count=1 ./internal/listener/... ; echo "RC=$?"
```

- [ ] **Step 3: `golangci-lint` over every touched package.** ⚠️ **misspell runs in locale US** — sweep
      British spellings in `.go` comments **before** the gate, not after it fails.

- [ ] **Step 4: Confirm `go.mod` / `go.sum` are untouched.** `go mod tidy -diff` must be EMPTY and
      `git diff master -- go.mod go.sum` must be EMPTY. ⚠️ **An import LINE is not a go.mod MODULE**, and
      this row adds no sub-package, so `reference_new_subpackage_pulls_transitive_module` does not bite —
      **check anyway.**

- [ ] **Step 5: Record every command and its ACTUAL output in `PROGRESS.md`,** then commit.

---

### Task 14: The §6 occurrence-set reconciliation — SIX sites, and `chainmatch.go` under a DOUBLE gate

**Files:**
- Modify: `internal/listener/quic_test.go` (four sites)
- Modify: `internal/listener/listenerfilter/types.go` (one site)
- Modify: `internal/listener/listenerfilter/chainmatch.go` (one site, **COMMENT-ONLY and 3/3**)

- [ ] **Step 1: INHERIT THE TABLES, DO NOT RE-DERIVE FROM THE MATCHERS.** 🔴 **`SPEC.md` §14 item 1 says to
      re-run §6's matchers; obeying that literally LOSES a mandated code edit.** All six matchers are
      **blind** to `types.go:44-46` — the one K2 code site — because the claim spans a line break and
      `git grep` is line-based (§0.10, with a positive control proving the file is grep-reachable). **Use
      §6's tables as the roster; use the matchers only to look for GROWTH. A re-derivation that loses a row
      is a deletion.**

- [ ] **Step 2: `quic_test.go`, four sites**, each relocated by literal text:
      `:690-694` comment (`PARSE PRECONDITION … accepts exactly {…} (manager.go:985-991)`) — **keep the
      precondition** (`"quic"` still parses), **drop "exactly"** and **drop or re-point the stale anchor**;
      `:702` `t.Fatalf` text (`parseChainSpec's enum gate must accept`) — **no enum gate exists after the
      repair**; `:768-771` comment — same treatment (⚠️ **the SPEC's range `:768-770` is SHORT BY ONE**; the
      block runs to `// listener, not by reading the switch.`); `:779` `t.Fatalf` — same as `:702`.
      ⚠️ **Both comments cite `manager.go:985-991` for a switch that at the un-fixed tip was at `:994-999`
      with its comment at `:991-993` — the cite named NEITHER, and after Task 11 the switch does not exist
      at all.** **LEAD WITH WHAT SURVIVES:** the precondition survives; the exhaustive-over-four-values
      claim dies.

- [ ] **Step 3: `types.go:44-46`** — `… or "" if no listener filter inspected the connection`. **True when
      the pipeline returns, false by the time selection reads it. Say BOTH**, and name `serveConnection` as
      the second writer.

- [ ] **Step 4: `chainmatch.go:30-32` — COMMENT-ONLY *and* EXACTLY 3/3.** Current wording, verbatim:

```go
	// TransportProtocol: "" means unspecified; "tls" or "raw_buffer" means
	// the chain requires the listener-filter pipeline to have set
	// inputs.TransportProtocol to the matching value.
```
      Both halves are falsified: the closed set dies, and after Task 12 the TCP default is set by
      `serveConnection`, **not** by the pipeline. 🔴 **The replacement MUST be exactly three lines**, because
      `quic_test.go` carries **15** `chainmatch.go:<line>` citations and **every one points below line 32**
      (minimum **87**) — a non-neutral edit staleys all fifteen in the commit that "reconciles" the prose
      (§0.12). **No document states this; this task does.**

- [ ] **Step 5: Run the DOUBLE gate** (§7) and paste both readings into `PROGRESS.md`:

```sh
git diff --numstat -- internal/listener/listenerfilter/chainmatch.go   # MUST read exactly: 3	3
# then the comment-only gate, run INLINE from §7 — see the note below
```
      ⚠️ **Run the §7 gate INLINE; do NOT commit it as a script.** The repo has **zero** tracked `.sh`
      files (`git ls-files | /usr/bin/grep -cE '\.sh$'` → **0**) and no `scripts/` directory, so a committed
      gate would introduce a file class this tree has never carried and would land on **neither** the edit
      set nor the byte-untouched roster. **Paste the command and its output into `PROGRESS.md` instead** —
      the record is the artefact, not the script.
      ⚠️ **A gate that reads "0 violations" over an EMPTY diff is vacuous** — the gate prints
      `inspected N changed line(s)` for exactly this reason. **Assert N > 0.**

- [ ] **Step 6: Re-run the six matchers for GROWTH ONLY**, both M1 spellings (§0.9), and record which hits
      are deliberately LEFT and why: ADR-0279 `:16736`/`:16740` and ADR-0319 `:19085`/`:19210` are
      **accurate about their own phases**; `BEHAVIOR_CONTRACT.md:5881` is **still true, a subset**; every
      hit under `docs/envoy-go/phases/**` is the **governing document of a closed stage** and is history.
      ⚠️ **Say which hits you are leaving and why** — an unexplained residual reads as a miss.

- [ ] **Step 7: Build and run the full sweep again**, then commit all three files together.

---

### Task 15: NC roster rows 1, 2 and 3 — scored PER PROPERTY

**Files:** none permanently — each mutation is applied, measured, and **reverted**.

- [ ] **Step 1: NEUTRALISE, never revert to the old code.** Each mutation must still **compile**, and you
      must confirm the mutated line is actually **executable** — a mutation behind a dead branch measures
      nothing.

- [ ] **Step 2: Row 1 — restore the enum `switch`.** Must redden **§5.1 (a)**, **§5.3's QUIC arm**, and
      fixture `l_bogus` (at boot). Must redden nothing else.

- [ ] **Step 3: Row 2 — parse stores `""`.** Must redden **§5.1 (b)** and **(d)**; must **NOT** redden
      **§5.1 (c)**. ⚠️ **(c) is BLIND to this mutation** — an emptied `fc[0]` loses to the sibling on
      specificity, so (c) still passes. **That is expected and must be recorded as such, not as a gap.**

- [ ] **Step 4: Row 3 — delete the `TransportProtocol` clause in `matches()`.** Must redden **§5.1 (c)**,
      **§5.2 (s2)** and fixture `l_tls`; must **NOT** redden **§5.1 (d)** — with the clause gone, `"sctp"`
      still wins on specificity. ⚠️ **Rows 2 and 3 are a PAIR: neither half is a gate alone.**

- [ ] **Step 5: Score PER PROPERTY and record the matrix**, with each mutation's `git diff --numstat` and
      the verbatim failure line. ⚠️ **An NC that leaves your control GREEN is not evidence the control does
      any work** — for each row, name the arm that must stay green and confirm it did.

- [ ] **Step 6: Revert every mutation and prove the tree clean**, then commit `PROGRESS.md`.

---

### Task 16: NC roster rows 4, 5, 5b and 6 — each with a COMPANION THAT MUST STAY GREEN

**Files:** none permanently.

- [ ] **Step 1: Row 4 — delete the stamp.** Must redden **§5.2 (s1)** and fixture `l_raw`. Must **NOT**
      redden **(s2)** or **(s3)**.

- [ ] **Step 2: Row 5 — stamp unconditionally with `"tls"`.** Must redden **(s1)**, **(s2)**, and fixture
      `l_raw` **and** `l_tls`. Must **NOT** redden **(s3)** — `tls_inspector` already wrote `"tls"` there.

- [ ] **Step 3: Row 5b — drop the `== ""` guard, stamp `"raw_buffer"` unconditionally.** Must redden
      **(s3) ONLY**. Must **NOT** redden (s1), (s2), or **any fixture listener** — all three are plaintext,
      and the inspector would have written the same `raw_buffer`. ⚠️ **This row is the entire reason (s3)
      uses a TLS client**; a plaintext (s3) would leave row 5b vacuous.

- [ ] **Step 4: Row 6 — move the stamp into `SelectChain`.** Must redden **Task 6's purity arm and nothing
      else.** ⚠️ **Measured during the PLAN: this mutation reddens ZERO pre-existing tests across all seven
      reverse-dependency selectors** — identical greens under the mutation and the un-mutation mean the
      suite is BLIND (§0.13). **Task 6's arm is the only thing standing between row 6 and vacuity.**
      Confirm its matched negative (`"tls"` input picking the `tls` chain) stays **GREEN under both**.

- [ ] **Step 5: Record the full matrix**, then revert everything and prove the tree clean.

- [ ] **Step 6: Commit `PROGRESS.md`.**

---

### Task 17: `ADR-0320` completed IN PLACE, and `BEHAVIOR_CONTRACT.md` — TWO files, ONE edit each

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: Append §Decision and §Consequences to `ADR-0320`, IN PLACE**, **after** the RETAINED italic
      footer, in the ADR-0294-through-0319 shared block form: **no `**Status:**` line, no renumber, and NO
      `---` separator.** ⚠️ **ADR-0044 does NOT contain the ADR-drafting discipline this project cites it
      for** — the discipline is the block form itself, and **no ADR documents it.**

- [ ] **Step 2: Flip the house guard.** `^> \*\*STATUS: PROPOSED` → `ACCEPTED`. **Verify BY LINE AND BY
      ADR** with a backward `^## ADR-` heading search — **never by the count**:

```sh
/usr/bin/grep -nE '^> \*\*STATUS: PROPOSED' docs/envoy-go/DECISIONS.md   # must be EMPTY after the flip
awk 'NR<=<HITLINE> && /^## ADR-/ {h=$0; n=NR} END {print n, h}' docs/envoy-go/DECISIONS.md
```
      ⚠️ **The ADR-0231 decoy at `:14866` (`^\*\*Status:\*\* PROPOSED`) is a DIFFERENT matcher and must stay
      BYTE-UNTOUCHED.** Running the decoy form alone reads as a disarmed guard. **Never gate on the
      unanchored `^\*\*Status:\*\*.*PROPOSED` form.**

- [ ] **Step 3: §Consequences must name the ONE behaviour change that can move traffic on a config that
      booted before this row:** a no-listener-filter TCP connection now matches `transport_protocol:
      raw_buffer` chains, so such a config changes which chain serves — **from the fallback to the
      `raw_buffer` chain. That is the parity direction**, and it must not be omitted.

- [ ] **Step 4: `BEHAVIOR_CONTRACT.md` — the `+0, UNCHANGED` ledger entry**, in the phase-96/97 form,
      **quoting NO absolute.** Neither edit registers, renames or removes a stat name: the parse edit
      touches no registry, and the stamp writes a local struct field read only by `SelectChain`. ⚠️ **A `+0`
      row DOES earn a chain entry** — phases 44.2, 44.3, 45.2, 47.1, 51, 96 and 97 each carry one. **Do not
      re-litigate this; it is settled.** ⚠️ **Three mutually inconsistent stat-surface absolutes are live in
      this tree at one tip — on a contested count, the answer is NO NUMBER.**

- [ ] **Step 5: `BEHAVIOR_CONTRACT.md` — ADD the §Chain-match algorithm bullet** at `:4363-4368`. That
      section states **no** transport-protocol input semantics today — **an omission, not a false claim**
      (corroborated: the string `raw_buffer` appears **zero** times in the whole file, with a positive
      control on `transport_protocol` proving the census reaches it). The bullet says: the input is
      whatever the listener-filter pipeline wrote, **or `raw_buffer` on TCP when the pipeline wrote
      nothing**, and the comparison is exact and **case-sensitive**.

- [ ] **Step 6: Do NOT repair `BEHAVIOR_CONTRACT.md:4359`.** §0.1 falsified it — `listener_filters_timeout`
      is not enforced on the subject — but that is a **banked** row (§10), not this one. **Recording a
      falsehood you are deliberately leaving is honest; repairing it here would smuggle an unmeasured
      third divergence into this row's scope.**

- [ ] **Step 7: Commit both files.**

---

### Task 18: `ROADMAP.md` — row 98 -> `done`, under the FIELD-COUNT gate

**Files:**
- Modify: `docs/envoy-go/ROADMAP.md` — row 98, currently at `:160`

- [ ] **Step 1: Relocate row 98 by ID, not by line.**
      `awk -F'|' '/^\| *98 /{print NR": "$0}' docs/envoy-go/ROADMAP.md`.

- [ ] **Step 2: Flip status `in-progress` -> `done` and rewrite the summary cell.** ⚠️ **This is a FLIP,
      not an ADD** — `want` stays **130** and `ROADMAP.md` stays **248** lines.

- [ ] **Step 3: COUNT THE FIELDS UNDER BOTH FORMS BEFORE INSTALLING** (⚠️ the trap is **LIVE** at this
      task — it is dormant only for stages that edit no row, and it fired against the phase-96 IMPL's own
      author, who wrote a Go `||` into a row's narrative):

```sh
sed -n '<ROW>p' docs/envoy-go/ROADMAP.md | awk -F'|' '{print "naive NF="NF}'                 # want 8
sed -n '<ROW>p' docs/envoy-go/ROADMAP.md | sed 's/\\|//g' | awk -F'|' '{print "escape NF="NF}' # want 8
```
      **Reword a pipe AWAY rather than escaping it.**

- [ ] **Step 4: ASSERT THE NEW CELL SPELLS NEITHER SENTINEL PHRASE.** ⚠️ **Check (2) matches the WHOLE
      FILE, not only the six windows** — a row cell spelling either phrase would mint a **seventh** hit and
      read as a finding:

```sh
sed -n '<ROW>p' docs/envoy-go/ROADMAP.md | /usr/bin/grep -coE 'deferred candidates:|remaining deferred \(not-yet-chartered\) candidates:'   # want 0
```

- [ ] **Step 5: Re-run the sentinel and ALL FOUR NCs.** ⚠️ **THE SHAPES CHANGE AT THIS FLIP — DO NOT
      INHERIT THEM.** After the flip: check (1) goes **SILENT**, NC-A drops to **ONE** line
      (`NOT DONE: row 62`), NC-B drops to **ONE** (`GATE FAIL: examined 130 … expected 129`). Check (2) must
      still read **SIX** and check (3) must still be **SILENT**. ⚠️⚠️ **DO NOT TOUCH THE SIX
      DEFERRED-CANDIDATE WINDOWS. THE MARGIN IS ONE, AND DELETING THE LAST ONE ENDS THE PROJECT.**

- [ ] **Step 6: Verify the six windows are BYTE-IDENTICAL** with per-line md5, **trailing newline
      INCLUDED** (`sed -n 'Np' f | md5sum`): `208 10d7807bf02d` · `214 4a92f7e62fc6` · `220 2a7eb298b9fd` ·
      `230 242e53c6f7a3` · `236 b2680e6f4fbf` · `244 6caa1c3ce0e7`. ⚠️ **State the method whenever you quote
      the digest.**

- [ ] **Step 7: Commit.**

---

### Task 19: The byte-untouched roster, the ARM ROSTER, and the SIX-GATE sweep

**Files:** none — this task only verifies and records.

- [ ] **Step 1: Assert the byte-untouched roster PER PATH** with `git diff master --numstat -- <path>`,
      each expected EMPTY: `internal/listener/quic.go`,
      `internal/listener/listenerfilter/tls_inspector/**`, every fixture directory other than `0123`,
      `go.mod`, `go.sum`. ⚠️ **`chainmatch.go` is on the roster for CODE only** — its comment-only
      constraint is gated by Task 14 Step 5, and that gate was shown to FIRE (§7).
      ⚠️ **The byte-untouched roster and the edit roster are NOT a partition** — at phase 97
      `BEHAVIOR_CONTRACT.md` sat on neither. **Name any path on neither.**

- [ ] **Step 2: DIFF THE ARM ROSTER, not the counters.** ⚠️ **A `+0/+0` negative-control arm can be
      silently deleted with every gate staying green** — Task 6's purity arm is exactly that shape.
      Enumerate the test function names added by this row and confirm each is present:

```sh
git diff master -- internal/listener/ | /usr/bin/grep -E '^\+func (Test|Fuzz)' | sort
```

- [ ] **Step 3: Gate (a) — the differential suite, `-count=1`.** ⚠️ **ASSERT THE FIXTURE SET BY NAME, BOTH
      `comm` DIRECTIONS** — never the exit code. Expect **125 PASS / 0 FAIL / 0 SKIP**; the full suite takes
      ~400s. ⚠️ **A driver-owned receiver PORT RACE can ABORT the binary and MASK every fixture after it**
      (it fired twice at two fixtures in one session at the phase-97 IMPL and masked 40). **If the run
      aborts, everything after the abort is UNMEASURED — rerun, and say so.**

- [ ] **Step 4: Gate (b) — the non-Docker package sweep**, gated on `PIPESTATUS[0]` **plus a SET
      RECONCILIATION**, not a count. ⚠️ **Exclude BOTH Docker drivers:**

```sh
go list ./... | /usr/bin/grep -vE '/test/differential$|/test/conformance/h2spec$'
```
      The denominator read **238** at this tip. ⚠️ **It was 237 for two rows and phase 97's own fixture
      package moved it** — **reconcile the SET, do not trust the number.**

- [ ] **Step 5: Gates (c), (d), (e).** h2spec `95 tests, 94 passed, 1 skipped, 0 failed` (the skip is
      6.9.2/2, invariant) — ⚠️ **a NON-verbose run prints NO summary line at all, only `ok … Ns`.**
      Fuzzers: **56 TARGETS / 48 FILES** — ⚠️ **do not conflate them**; this row adds **+0** (it consumes no
      new config field, so there is no parse arm to fuzz). The anchored panic gate
      `^panic:|DATA RACE|SIGSEGV` must read **0** — ⚠️ **and at the phase-97 IMPL it legitimately read 1,
      which was the gate FIRING on a real abort, not malfunctioning.**

- [ ] **Step 6: Gate (f) — no `REVIEW.md`.** This is a **STANDING DEPARTURE**, not an omission: 37 of 139
      phase directories carry one and **none of 93-98 do.** **Name it; do not claim compliance.**

- [ ] **Step 7: Record every gate's ACTUAL output in `PROGRESS.md`,** including the §0.1 flake if it fired,
      and commit.

---

### Task 20: The close — `PROGRESS.md`, `STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt`

**Files:**
- Modify: `docs/envoy-go/phases/98-chain-match-transport-protocol-reject/PROGRESS.md`
- Modify: `docs/envoy-go/STATE.md`
- Modify: `docs/envoy-go/STATE_HISTORY.md`
- Modify: `next-prompt.txt`

- [ ] **Step 1: `STATE.md` — edit §Current pointer IN PLACE.** ⚠️ **Never prepend a new block above it** —
      that is the exact failure ADR-0288 exists to prevent, and it once left `grep 'next-skill:'` returning
      two stale hits and zero live ones. Set lifecycle-state **3 -> DONE**, `next-skill` to the next stage.

- [ ] **Step 2: Evict the oldest §Recent entry using the LABEL-BOUND PAIR on BOTH files.** ⚠️ **The bare
      forms answer nothing.** The evictee's own label must go `STATE.md` **1 -> 0** and `STATE_HISTORY.md`
      **0 -> 1**. Run a **fabricated-label NC** (must read 0 in both) and a **positive control naming a
      label that IS in the archive and ABSENT from §Recent**. ⚠️ **READ THE SHAPE at your own tip** — the
      histogram changes every close, and at this stage's close it is a **four-wide tie at the tail** where
      the date read cannot pick alone.

- [ ] **Step 3: Archive as ONE INLINE LINE in the PARENTHETICAL form.** Raw delta **`+2`, not `+1`** (a
      blank line plus the entry line). The **strict** guard must read **DELTA 0**. ⚠️ **Name the LABEL,
      never the COUNT** — an archive line that names a label as its positive control changes that label's
      own count in the file it measures, in the same commit.

- [ ] **Step 4: Roll the §Recent PREAMBLE SENTENCE too**, without spelling the evictee's label.

- [ ] **Step 5: Roll `next-prompt.txt`** (`git add -f` — it is **tracked but gitignored**). ⚠️⚠️ **GREP IT
      FOR `YOUR STAGE`, FOR YOUR OWN STAGE WORD, AND FOR EVERY FIGURE YOUR ROW MOVED**, then read the result
      as the next reader, who has no idea which sentences you meant to leave. **A roller that leaves a spent
      stage's imperative standing has minted a stale INSTRUCTION, which is worse than a stale figure because
      a reader OBEYS it.** ⚠️ **Fix method note 3e's `123 / 99+24` to `125 / 101+24`** — it is stale already
      (§0.7) and this row moves it again.

- [ ] **Step 6: Re-derive EVERY line count you quote IN THE SAME COMMIT as the edit that moves it**, and
      squash to ONE commit whose subject carries **both the FULL SLUG and the STAGE WORD**. Merge and push.

---

## 7. The `chainmatch.go` comment-only gate — BUILT, AND SHOWN TO FIRE

`SPEC.md` §6.3 resolves the one intersection between the edit set and the byte-untouched roster by a
**comment-only** constraint on `chainmatch.go`, and requires it to be **mechanically gated** with the gate
itself **run over an input known to trip it**. Here is the gate, and both controls, run during this stage.

### 7.1 The hazard question, answered BEFORE the gate was written

A naive `^[+-]\s*//` matcher is fooled by a `//` inside a Go string literal. **Does that hazard exist in
this file?** `/usr/bin/grep -n -- '"[^"]*//' internal/listener/listenerfilter/chainmatch.go` → **rc=1, no
output**; the backtick form likewise. **Discriminating positive control, same pattern, wider scope:**
`git grep -n -- '"[^"]*//' -- '*.go'` → `cmd/envoy-go/main_test.go:410`, `:779`, `:907`, … rc=0 — **so the
matcher does find the hazard where it exists.** A full literal census of `chainmatch.go` shows its only
double-quoted strings are `"errors" "net" "strings"`, two `errors.New(…)` messages, `""`, `"*"` and `"*."`.
⇒ **The hazard does not exist in this file, and a line-prefix matcher is safe HERE.** ⚠️ **It would not be
safe as a general-purpose gate, and this plan does not claim it is.**

### 7.2 The gate — run INLINE at Task 14, never committed

```sh
git diff --no-color --unified=0 "$RANGE" -- internal/listener/listenerfilter/chainmatch.go \
| awk '
    /^--- /          { next }          # old-file header — the TRAILING SPACE matters
    /^\+\+\+ /       { next }          # new-file header
    /^\\ No newline/ { next }
    /^[+-]/ {
      insp++
      s = substr($0, 2)
      sub(/^[[:blank:]]+/, "", s)
      if (s == "")     next            # blank line
      if (s ~ /^\/\//) next            # Go line comment
      printf "VIOLATION (non-comment %s line): %s\n", (substr($0,1,1)=="+" ? "ADDED" : "REMOVED"), $0
      v++
    }
    END {
      printf "GATE: chainmatch.go comment-only -- inspected %d changed line(s), %d violation(s)\n", insp+0, v+0
      if (v+0 > 0) exit 1
    }'
```

Three construction notes, each load-bearing: the `+++ ` / `--- ` headers are excluded **with their trailing
space**, so a removed content line beginning `--` is never mistaken for a header; **`[[:blank:]]` is used
instead of `[ \t]`**, because a bracketed `\t` inside a GNU ERE is a literal backslash and `t`, not a tab;
and the **`inspected N`** counter exists so that **an empty diff cannot read as a pass** — a bare
"0 violations" is exactly the vacuous-green shape this project rejects.

### 7.3 The controls — ACTUAL output, all four run

| # | input | output | rc |
|---|---|---|---|
| **A** | baseline, **empty** diff | `GATE: … inspected 0 changed line(s), 0 violation(s)` | 0 |
| **B** | **POSITIVE CONTROL** — a real 3/3 comment-only rewording of `:30-32`; `--numstat` `3	3`; `go build ./internal/listener/...` rc=0 | `GATE: … inspected 6 changed line(s), 0 violation(s)` | **0** |
| **C** | **NEGATIVE CONTROL** — a code edit that still compiles (local `w` → `wantEntry` in `alpnMatchAny`); `--numstat` `2	2` | four `VIOLATION …` lines, each naming its line, then `inspected 4 … 4 violation(s)` | **1** |
| **D** | **STACKED CONTROL** — the comment edit **and** the code edit together; `--numstat` `5	5` | the same four violations, then `inspected 10 … 4 violation(s)` | **1** |

⚠️ **Control A is why the counter exists** — without it, A and a working gate are indistinguishable.
⚠️ **Control D is why a gate must not stop at the first legal line**: it saw six legal comment lines and
**still failed on the four code lines**. A gate that short-circuits on the first `//` would have passed D.
After revert, the gate reads `inspected 0 … 0 violation(s)`, rc=0, and the tree is clean.

### 7.4 The SECOND, independent constraint on the same edit

§0.12: the replacement must also read **exactly `3	3`** on `--numstat`, because `quic_test.go` carries
**15** `chainmatch.go:<line>` cites and **all 15 point below line 32** (minimum **87**). **The comment-only
gate does not catch a line-count change** — a 4-line comment replacing a 3-line one is comment-only and
passes §7.2 while staleing fifteen citations. **The two gates are independent and Task 14 runs both.**

---

## 8. The negative-control roster the IMPL inherits — CORRECTED

**Neutralise, never revert.** Every mutation must compile, and you must confirm the mutated line is
**executable** before believing a green.

| # | mutation (compiles) | must redden | must NOT redden |
|---|---|---|---|
| 1 | restore the enum `switch` | §5.1 (a); **§5.3's QUIC arm**; fixture `l_bogus` (at boot) | — |
| 2 | parse stores `""` | §5.1 (b), (d) | §5.1 (c) |
| 3 | delete the `TransportProtocol` clause in `matches()` | §5.1 (c); §5.2 (s2); fixture `l_tls` | §5.1 (d) |
| 4 | delete the stamp | §5.2 (s1); fixture `l_raw` | §5.2 (s2), (s3) |
| 5 | stamp unconditionally with `"tls"` | §5.2 (s1), (s2); fixture `l_raw`, `l_tls` | §5.2 (s3) |
| 5b | drop the `== ""` guard — stamp `"raw_buffer"` unconditionally | §5.2 (s3) **ONLY** | §5.2 (s1), (s2); **every fixture listener** (all plaintext) |
| **6** | stamp moved into `SelectChain` | **Task 6's purity arm — AND NOTHING ELSE** | every other arm |

**Two corrections to `SPEC.md` §11, both by execution:**

1. 🔴 **Row 6 is no longer UNADJUDICATED — it SURVIVES, and it acquires the only arm that can fire it.**
   §0.13 measured that the mutation reddens **zero** pre-existing tests across all seven reverse-dependency
   selectors. Without Task 6's arm the row is a control that **cannot fire**. **The arm costs
   `chainmatch_test.go`, a file outside `SPEC.md` §6.3's edit set — declared in §9, not smuggled.**
2. 🔴 **Row 1 gains `§5.3's QUIC arm`**, which did not exist when §11 was written. It is the only arm that
   reddens under row 1 via the **QUIC listener-construction** path.

⚠️ **Rows 2 and 3 are a PAIR; neither half is a gate alone** — (c) is blind to row 2 and (d) is blind to
row 3, both for the same reason: specificity. **Score the roster PER PROPERTY and PER ARM, never per run.**
⚠️ **An NC that leaves your negative control GREEN is not evidence that control does any work** — every row
above names what must stay green, and Tasks 15 and 16 confirm each one did.

---

## 9. Counts — RE-DERIVED AT THIS PLAN'S OWN TIP

⚠️ **Every figure below was produced by running its command in the commit that ships these edits.** A
section headed *"re-derived at this stage's own tip"* is false if it names a commit that is not the one it
ships on.

### 9.1 Figures this stage MOVES

| figure | before | after | command |
|---|---|---|---|
| `STATE.md` | 65 | rolled **IN PLACE** | `wc -l` |
| `STATE_HISTORY.md` | 574 | **576** (`+2`, a blank line **plus** the entry) | `wc -l` |
| this `PLAN.md` | — | **2007** | `wc -l`, run in the publishing commit |

⚠️ **THE `PLAN.md` FIGURE IS SELF-REFERENTIAL AND WAS PATCHED IN LAST**, after the final section was
appended, then **re-verified before committing** — the phase-97 SPEC's own line count was handled exactly
this way, and a figure written before the last append is false at its own publishing commit.
⚠️ **At 2007 lines this PLAN is LONGER THAN ALL FIVE PRECEDENTS** (865-1666, median 1555).
**That is not a §6.1 gate** — §6.1 gates on TASKS and LoC, both of which clear (§1.3) — but it is stated
rather than smoothed, and the reason is §0: fourteen refutations, four of which required their own
measured evidence tables.
| `next-prompt.txt` | 332 | rolled | `wc -l` |

### 9.2 Figures this stage must NOT move — the baseline the close proves against

| figure | value at `bd303d87` | command |
|---|---|---|
| `ROADMAP.md` | **248** lines / **130** data rows / tail row **98** `in-progress` at `:160` | `wc -l`; the §2 awk |
| `DECISIONS.md` | **19235** lines · `^---$` **216** · `^## ADR-` **319** · bare `^## ` **327** · tail **ADR-0320** · next-free **ADR-0321** | `wc -l`; `/usr/bin/grep -cE`; **tail-derived**, never heading-count |
| `BEHAVIOR_CONTRACT.md` | **5993** | `wc -l` |
| active differential fixtures | **124**, tail `0122-quic-chain-selection` | `ls -d test/fixtures/*/ \| wc -l` |
| registered fixtures | **124 = 124**, both `comm` directions EMPTY, split **100 `driver/` + 24 `inputs/`** | §6 Task 9's extractor |
| non-Docker package denominator | **238** (raw `go list` 240) | `go list ./... \| /usr/bin/grep -vE '…'` |
| fuzzers | **56 TARGETS / 48 FILES** | `git grep -c '^func Fuzz' -- '*.go'` |
| phase directories | **139**, tail `98-chain-match-transport-protocol-reject/` | `ls -d docs/envoy-go/phases/*/ \| wc -l` |
| `STATE_HISTORY.md` guard triple | strict **163** · parenthetical **74** · loose **237**; **163 + 74 = 237 exactly** | §2.7 |

⚠️ **Next-free ADR is derived from the TAIL, never from the heading count** — the id space is sparse
(`/usr/bin/grep -cE '^## ADR-0209'` → **0**, the one known gap), and the heading regex
`^## ADR-[0-9]+[:—]` is itself holed by `## ADR-0127 v2`. ⚠️ **No count of either `PROPOSED` matcher is
written in any prose the grep matches.**

### 9.3 Cost — MEASURED and ESTIMATED, labelled separately

**MEASURED** (the SPEC's built-run-and-reverted prototype, `git diff --numstat`): production
`internal/listener/manager.go` **`+4 / −9`**. That is the only measured cell.

**ESTIMATED**, from the §1.3 table: `.go` only ≈ **+1024 / −53**; `.go` + `expectations.yaml` ≈ **+1224**;
plus `README.md` ≈ **+1484**. ⚠️ **Every one is a FLOOR.**

**The fixture floor, corrected** (§0.4): `0122`'s dir-only cost is **946** across **three** files with **no
committed YAML**; `0121`'s is **1393** of which **201 is PKI**. `0123` is plaintext-only and ships no PKI,
so its shape-matched floor is **~946**, not the `1359`/`1393` the SPEC and the router quote.

**Declared scope widening beyond `SPEC.md` §6.3's edit set — TWO files, named here rather than discovered
later:**

| file | why | task |
|---|---|---|
| `internal/listener/listenerfilter/chainmatch_test.go` | NC roster row 6's only possible arm (§0.13) | Task 6 |
| `docs/envoy-go/phases/98-…/PROGRESS.md` | BOOTSTRAP §5 state 3 requires it | Task 1 |

⚠️ **Neither is on the byte-untouched roster, so adding them is legal** — but `SPEC.md` §6.3's edit set
lists neither, and **the byte-untouched roster and the edit roster are not a partition** (at phase 97
`BEHAVIOR_CONTRACT.md` sat on neither). **Task 19 Step 1 requires the IMPL to name any path on neither.**

---

## 10. Deferred — surfaced by THIS PLAN, none chartered

### 🔴 NEW AND STRONG: `listener_filters_timeout` IS NOT ENFORCED ON THE SUBJECT

**Everything a BRAINSTORM needs is in §0.1 and §0.3, and BOTH SIDES ARE ALREADY MEASURED.**

- **Subject:** on a listener with `listener_filters_timeout: 1s` and `tls_inspector`, a client that stays
  silent for **10 seconds** is still served (`downstream_cx_total: 3`, all three arms `INDEXED`). The
  timeout never fires. **Mechanism:** `peekerConn.Peek` → `bufio.Reader.Peek` with **no deadline**; **zero**
  `SetReadDeadline` sites in non-test `internal/listener`; `Pipeline.Run` checks `ctx.Err()` only **after**
  `Inspect` returns. A silent client parks a goroutine indefinitely.
- **Reference:** times out at 1000 ms (`listener filter times out after 1000 ms`,
  `fallback to default listener filter`), books `downstream_pre_cx_timeout: 1`, and under
  `continue_on_listener_filters_timeout: false` **drops the connection** (`downstream_cx_total: 0`).
- **It falsifies `BEHAVIOR_CONTRACT.md:4359`** (located by literal text), which claims the timeout is
  *"honored in [1s, 60s] envelope"* and `continue_on_listener_filters_timeout` *"honored as
  proto-documented"*. ⚠️ **RECORDED, NOT REPAIRED** — Task 17 Step 6 forbids repairing it in this row.
- **Three sub-divergences, split before pricing:** (a) the timeout is advisory-only; (b)
  `continue_on…: false` is unreachable for a silent client, so the reference's **drop** never happens;
  (c) the subject emits **no `downstream_pre_cx_timeout` stat at all** (`git grep -c 'pre_cx' -- '*.go'`
  → **0 files**; positive control `downstream_cx_total` → 3), so that counter **cannot** be pinned
  cross-side without adding a name.
- **Why not chartered here:** the repair is a different mechanism (socket deadlines or a ctx-aware peeker)
  in a different file, this row has **already been widened once**, and folding in a third divergence would
  put the split gate in play. **It is also a resource-exhaustion characteristic, which argues for its own
  row, not a fold-in.**

### Carried forward, unchanged

- 🔴 **SNI LONGEST-SUFFIX PRECEDENCE** — still the strongest banked candidate, measured on both sides
  (`24 / 0`, 0 tests RED, reference order-independent under the reversal control). **The reference SERVES
  and envoy-go CLOSES THE CONNECTION.** Everything a BRAINSTORM needs is in `BRAINSTORM.md` §0.2, §0.3,
  §0.9. **SNI case-sensitivity should land WITH it, not alone.**
- ⚠️ **`server_names` PARTIAL-WILDCARD ACCEPTANCE** — documented at eight sites, the union of **two**
  matchers, neither complete alone.
- ⚠️ **`TestEnvoyGoBinary_TwoListenerCutover` PORT FLAKE** — fired twice at two tips in one session at the
  phase-98 SPEC (`:36601`, `:33215`), both inside `32768-60999`; three isolated reruns green. **A green
  rerun clears nothing.** Absent from every register. Task 1 Step 4 and Task 19 Step 7 carry it into the
  gate posture (`SPEC.md` §14 item 7).
- ⚠️ **THE DRIVER-OWNED RECEIVER PORT RACE** — evidenced, not hypothetical; its failure mode is **MASKING**
  (one abort masked 40 fixtures). Task 19 Step 3 carries it.
- ⚠️ **`catchAllCount` IS DEAD CODE** — `0 / 7`, NOT CONSTRUCTIBLE, its only guard vacuous (satisfied by its
  own fixture's listener name). **A fold-in, not a phase — it has no cross-side surface.**
- ⚠️ **THE HCM `stat_prefix` DUPLICATE-REGISTRATION PANIC** · **the `0066`/`0067`/`0068`/`0071` dead-port
  false-green** · **the harness's inability to carry two UDP listeners** · **per-connection TLS identity
  dispatch** · **the stamped QUIC ALPN constant's hardcoded `h3`** · **duplicate listener ADDRESSES** ·
  **D2-QUICTS** — all unchanged, all in `next-prompt.txt`.
- ⚠️ **`ROADMAP.md:139` and `:230` carry a stale swallowed-panic claim**, and `:230` sits **inside a
  sentinel window on a margin of one**. **RECORD IT, DO NOT TIDY IT.**

---

## 11. `SPEC.md` §14 coverage — every owed item, item by item

| § 14 item | where it is discharged | verdict |
|---|---|---|
| **1.** Re-derive every §6 anchor by LITERAL TEXT; re-run §6's matchers — the union may grow | §0.9, §0.10, §0.11; Task 14 Steps 1, 2, 6 | ✅ **DONE — and the INSTRUCTION ITSELF IS REFUTED.** All 14 anchors still match byte-for-byte; `quic_test.go:768-770` is **short by one** (`:768-771`); both `manager.go:985-991` cites confirmed stale. 🔴 **But re-deriving from the matchers alone LOSES `types.go:44-46`, a mandated code edit** — all six are blind to it. **Inherit the TABLES; use the matchers only for GROWTH.** |
| **2.** Order the spine so the UN-FIXED tip is measured FIRST; run RED; record RED vs structurally-GREEN per arm; then edit 1, RUN, then edit 2 | §6 Tasks 1-10 (un-fixed), Task 11 (edit 1), Task 12 (edit 2); the per-arm roster in Task 10 Step 3 | ✅ **DONE.** `l_raw` and (s1) **must STAY RED after edit 1** and turn GREEN only at edit 2 — Task 11 Step 5 makes that the task's key assertion. |
| **3.** Adjudicate §11 NC row 6, §2.3's timeout arm and §5.3's QUIC unit arm — each explicitly | §4, the three-row table | ✅ **DONE.** Row 6 **SURVIVES** with a new arm in a new file; the timeout arm is **MEASURED on the reference and STRUCK on the subject** with a named foreclosing mechanism; the QUIC arm is **ADDED**, with its own weakness recorded. |
| **4.** Prove both per-chain counter NAMES exist on BOTH sides before `0123` pins them | §0.6; Task 8 Steps 1-2 | ✅ **DONE — and the framing is REPLACED.** Both sides emit both names on both endpoints — **but at `0` BEFORE ANY TRAFFIC**, so a **name-presence pin is VACUOUS on both sides.** The honest pin is the **value pair plus the mirror arm**. |
| **5.** Gate the `chainmatch.go` comment-only constraint mechanically, NC'd against a code-touching probe | §7, with four controls run | ✅ **DONE.** Positive (0 violations over a real 3/3 comment diff), negative (4 violations, rc=1, each line named), **stacked** (10 inspected, 4 violations), and an **empty-diff** control proving the counter is needed. 🔴 **Plus a SECOND, independent constraint no document stated: the edit must be 3/3 or 15 `quic_test.go` cites go stale.** |
| **6.** Budget honestly; evaluate BOOTSTRAP §6.1 **with a command** | §1.3, §9.3; the §12 measurement | ✅ **DONE.** **20 tasks** (margin 5), **122 sub-steps, maximum 8** — measured by `awk`, and **no task reaches the ~10 line.** LoC under `~1500` on **every** accounting including the widest. 🔴 **And the inherited `1359`/`1393` floors are REFUTED** — they are `0120`/`0121` dir-only, PKI-inflated, and the newest fixture is **946**. |
| **7.** Carry §0.9's flake into the gate posture | §10; Task 1 Step 4; Task 19 Step 7 | ✅ **DONE.** A `TestEnvoyGoBinary_TwoListenerCutover` bind failure inside `32768-60999` is **not this row's regression**, and **a green rerun clears nothing.** |

---

## 12. Self-review — run against the SPEC with fresh eyes

**1. Spec coverage.** Every `SPEC.md` section maps to a task: §4.1(1) → Task 11; §4.1(2) → Task 12; §5.1 →
Task 2; §5.2 (s1)(s2) → Task 3, (s3) → Task 4, **(s4) → STRUCK with a mechanism** (§4b); §5.3 → Task 5;
§6.1/§6.2 → Task 14; §6.3 → Task 19 Step 1 and §7; §7 (fixture) → Tasks 7-9; §8 (ADR) → Task 17; §9
(ledger) → Task 17 Steps 4-5; §11 (NC roster) → Tasks 15-16, corrected in §8. **One gap found and closed
during this review:** §11 row 6 had no arm anywhere in the tree, so Task 6 was added.

**2. Placeholder scan.** No `TBD`, no "implement later", no "similar to Task N", no "add appropriate error
handling". Every code step carries real code or a real command. The two places this plan says "write it"
without pasting a full function — Task 7's driver and Task 8's expectations — **name the precedent file and
line to copy from** (`0122` for shape, `0018` for arity, `0005/driver.go:489` for the stat projection),
which is the repo's own convention for a ~600-line generated artefact.

**3. Type consistency.** `SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec)`
is spelled identically in §5.1 and Tasks 2 and 6. `ChainMatchInputs`'s seven fields are listed once, in
order, and `SourceIP net.IP` is used consistently. `rt.chainSpecs` / `rt.defaultSpec` match
`manager.go:160-163`. Test names are spelled identically in §5, §6 and §8: `…AcceptsUnknownTransportProtocolAsNonMatchingValue`,
`TestServeConnection_NoListenerFilter_RawBufferChainServes`, `…_TLSChainDoesNotServe`,
`TestServeConnection_TLSInspector_ClassifiedInputNotOverwritten`,
`TestQUICChainSelection_TransportProtocolBogusDoesNotMatch`,
`TestSelectChain_EmptyTransportProtocolDoesNotMatchRawBufferChain`.

**4. Thresholds near their line — STATED, not smoothed.** ⚠️ Three were checked and **all three clear**:
tasks **20 / ~25**; widest LoC **≈ +1484 / ~1500** — the tightest of the three, and a floor; sub-steps
**max 8 / ~10**. ⚠️ **The LoC margin is roughly one percent and the estimate is a LOWER BOUND**
(`reference_measured_prototype_is_a_lower_bound`, nineteen consecutive rows). **If the fixture driver
overshoots `0122`'s 534 by much, the widest accounting crosses `~1500` — BOOTSTRAP §6.1's mid-execution
trigger is then the remedy, at that moment, not a retroactive re-reading of §1.3.**

**5. What this plan could still be wrong about.** Stated rather than hidden: (a) the fixture cost is an
estimate anchored on a **one-listener** fixture and `0123` has **three**; (b) §0.6's reference measurement
of pre-traffic name presence was taken on **one** config shape — **two agreeing measurements do not
generalise to the class** (method note 34); (c) §4(c)'s QUIC arm is added on a **judgement**, not a
measurement, and its weakness is recorded in §4; (d) the §0.3 reference timeout arms were run by one agent
on one rig, and **a green rerun clears nothing** applies to reference measurements too.

---
