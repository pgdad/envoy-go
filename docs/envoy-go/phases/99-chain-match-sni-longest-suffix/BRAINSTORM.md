# Phase 99 — `chain-match-sni-longest-suffix` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle **DONE -> 1**). **Self-picked** under the 2026-07-12 standing
directive, with no human consulted and no banked mid-lifecycle work to advance first (check (1) was
SILENT at `want=130` before this stage's ADD — §8.1).

**Subject in one sentence.** When two `filter_chains` both match a client's SNI through `*.`-wildcard
`server_names` of different lengths (`*.b.foo.test` and `*.foo.test` for SNI `a.b.foo.test`), reference
Envoy serves the chain with the **longest matching suffix, independent of declaration order**, while
envoy-go's `SelectChain` returns `ErrAmbiguousChainMatch` and the TCP accept path **CLOSES THE
CONNECTION** — even when a `default_filter_chain` is configured — on a config that passes
`-mode validate` and boots.

**Both sides were MEASURED at this stage's own tip (`b9aee47d`), end to end, by served response** —
not by `SelectChain` return value and not by handshake completion (method note 74) — each dimension
with a matched negative on a byte-identical config and the precedence dimension with the
**declaration-order reversal** that alone separates "longest wins" from "first wins" (method note 70).

---

## 0. What this stage refuted — FOURTEEN claims, by execution

Every item was produced by running something. Where a figure is quoted, the command is named.
**Six of the fourteen refute a claim THIS SESSION'S ROUTER (`next-prompt.txt`) or `STATE.md` asserts.**

### 0.1 🔴 "`golangci-lint` CANNOT RUN AT ALL IN THIS ENVIRONMENT" — FALSE. It runs, the tip is CLEAN, and the gate is LIVE.

The router, `STATE.md` §Current and the phase-98 `PROGRESS.md` all record the lint gate as unrunnable
(v1.64.8 built with go1.26.2 dies on `sync/atomic`: `export data version 4 is greater than maximum
supported version 2`). **The variable is the `go` command the linter shells out to, not the linter.**
Measured at `b9aee47d`, repo root, the installed binary, the repo's own `.golangci.yml` unchanged:

| command | rc | findings |
|---|---|---|
| `golangci-lint run ./internal/listener/...` (ambient go1.27.1) | 1 | the `sync/atomic` export-data typecheck error — the router's symptom, REPRODUCED |
| `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...` | **0** | **0 lines of output** (18.8 s wall) |

**A zero needs a positive control (method note 7i).** A throwaway file planted in
`internal/listener/` of a detached scratch worktree (an unchecked `os.Remove`, an ineffectual
assignment, an undocumented exported func) under the same `GOTOOLCHAIN=go1.26.2` command fired
**`errcheck`, `revive` and `ineffassign`**, rc=1; the file was deleted and the worktree removed. **The
gate is live and the tip is lint-clean under the pinned config.** ⇒ **The "SEVENTH standing
departure" is not environmental in the sense claimed: it is a one-variable invocation fix**, and the
phase-99 IMPL can run the lint gate for real. ⚠️ `GOTOOLCHAIN=go1.26.2` downloads a toolchain on
first use — it worked here; a network-less session would need the cached copy.

### 0.2 🔴 GATE (b)'s RED at `internal/filter/http/compressor` IS THE SAME VARIABLE

`go test -count=1 -v -run 'TestEncodeData_LevelMapping_DifferentGzippedSizes' ./internal/filter/http/compressor/`:

| toolchain | result |
|---|---|
| `GOTOOLCHAIN=go1.26.2` | `--- PASS` |
| ambient go1.27.1 | `--- FAIL` … *"both = 2121"* — the router's exact figure, REPRODUCED |

**Both "standing departures" are one event: the local toolchain moved 1.26.2 -> 1.27.1 under two
gates.** The router banked them as "the same class"; they are the same **instance**. `go.mod` reads
`go 1.23.0` with no `toolchain` line, so nothing in the repo pins either.

### 0.3 Rebuilding the SAME linter under go1.27.1 does NOT fix it — the obvious repair is wrong

`GOBIN=<scratch> go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8` (reports
`built with go1.27.1`), full run: rc=1, **621** output lines, **207** `(typecheck)` findings across
**59** directories — including the SAME `sync/atomic` export-data error (37 hits). **The export-data
reader is vendored `x/tools`, not the build toolchain.** A "rebuild the linter" fix encodes a wrong
answer (method note 37, in tooling).

### 0.4 A v2 upgrade is a RE-BASELINE, not a fix — 97 new findings, all rule-set expansion

`golangci-lint@latest` = **v2.13.2** built with go1.27.1, config via `golangci-lint migrate` in a
throwaway worktree: rc=1, **97 issues, all `staticcheck`** — `SA1019` **41** (deprecated proto fields
used in TESTS), `QF1008` 18, `ST1005` 16, `QF1006` 5, `QF1001` 5, `QF1011` 4, and eight singletons
(`ST1023 ST1013 ST1012 ST1008 S1035 S1031 S1016 S1009`). v2 folds `gosimple`/`stylecheck`/quickfix
into `staticcheck`; **the v1 baseline over the same tree is ZERO** (§0.1), so these are the rule set
widening, not regressions the dead gate let through. **A v2 move is its own decision (the `.golangci.yml`
header says removals from the baseline require an ADR).**

### 0.5 The router's phase-98 BRAINSTORM commit id is WRONG

The router's "What the next session owes" names *"the phase-98 BRAINSTORM (`8194291e`)"*. `git show
8194291e` is `phase 97 next-prompt: the IMPL's own roll left FOUR blocks …`, touching only
`next-prompt.txt` (`31 16`) — it is the BASE the phase-98 BRAINSTORM ran on. The BRAINSTORM itself is
**`45728a81`**, found by the slug form `git log --grep '^phase 98 (chain-match-transport-protocol-reject) BRAINSTORM'`
and cross-checked against the loose form `--grep '^phase 98'` (six commits, read by subject). Its
scope, and the phase-97 BRAINSTORM `a45f1914`'s, are **identical five-path shapes**:
`ROADMAP.md 1 0`, `STATE.md 10 10`, `STATE_HISTORY.md 2 0`, the new `BRAINSTORM.md`, `next-prompt.txt`.

### 0.6 "Expect only `master`" in `git worktree list` — FALSE at this tip

`git worktree list` at session start: the canonical root **plus `/home/esa/git/wt-phase-98-impl` at
`8cf8052c [phase-98-impl]`**. It is clean (`status --porcelain --untracked-files=all` empty), its branch
is not an ancestor of `master` (the IMPL was squashed as `884e4b7c`), and `git diff phase-98-impl
master` is `next-prompt.txt 1 1` only — the later router-fix commit `b9aee47d`. **Nothing unmerged
lives there. It outlived its stage close** (method note 9). This stage removes the WORKTREE, not the
branch (the branch graveyard is deliberate history, note 9).

### 0.7 "The newest fixture `0122` is 946" — true of `0122`, false as "newest"

`git log --numstat --format= -- test/fixtures/<dir> | awk '{a+=$1} END{print a}'`: `0122` **946**,
`0123` **1361** — and **`0123` carries NO `pki/`** (3 tracked files), so "PKI-inflated" does not
explain the spread. The comparable-SHAPE fixtures for THIS row (TLS listener, several chains,
driver-set SNI) are `0045-sni-cluster` **849** (4 pki files), `0002-tls-tcp` **1137** (11 pki
files), `0121-listener-default-chain-tls` **1393** (4 pki files). **Re-derive a floor by shape.**

### 0.8 `ErrAmbiguousChainMatch`'s doc comment is false for EVERY shape this row is about

`internal/listener/listenerfilter/chainmatch.go:54-59` says the manager *"detects this at
NewManager-build time … and rejects the bootstrap"*. Arms A, B and E (§2) all pass `-mode validate`
(rc=0) and boot; the error surfaces **per connection**. Only structurally identical specs (arm H) are
caught at build. Same false claim, normative: `DECISIONS.md:3157` (ADR-0081 clause 5, *"Error at
`NewManager`-build time"*) and `BEHAVIOR_CONTRACT.md:4368` (*"Final ties … error at `NewManager`-build
time"*). **Phase 98 §0.3 found the comment; nobody found the two NORMATIVE copies.**

### 0.9 🔴 A `default_filter_chain` does NOT rescue the ambiguous case — the reference serves, envoy-go drops

Arm E (§2): with the two wildcard chains PLUS a default chain, SNI `a.b.foo.test` is **closed**
(curl rc=35, log `chain-match: ambiguous filter_chain selection`) on the subject. `nomatch.example`
on the same config serves DEFAULT. **An operator who adds a default chain "to be safe" is not safe.**
On the reference the same SNI serves **LONG**.

### 0.10 The existing suite is BLIND to the whole slot, in BOTH directions

`go test -count=1 -v ./cmd/envoy-go/... ./internal/admin/... ./internal/boot/... ./internal/listener/... ./validate/...`
— **the same five selectors every run**, rc via `PIPESTATUS[0]`, FAIL via
`^(FAIL|--- FAIL)|^ *--- FAIL`:

| tree | rc | `=== RUN` | RED |
|---|---|---|---|
| unpatched `b9aee47d` | 0 | 398 | 0 |
| A2 prototype (longest wins) | 0 | 398 | 0 |
| **A2 INVERTED (shortest wins)** | 0 | 398 | **0** |
| pattern case-fold prototype | 0 | 398 | 0 |

The inverted binary **really does serve SHORT** for `a.b.foo.test` end to end (the mutation is live),
and nothing reddens. **Identical greens under fix and un-fix mean the suite is blind** (method note
50). `RUN=398` in every arm, so no arm dropped tests.

### 0.11 `sniSpecificityRank` ranks a chain's WHOLE pattern set, not the pattern that MATCHED

`sniSpecificityRank(patterns)` returns the best rank over **all** of a chain's `server_names`. A chain
`["x.test", "*.foo.test"]` ranks **0 (exact)** even when SNI `a.foo.test` matched only its wildcard.
**This bites the repair, not only the status quo**: the A2 prototype compares suffix lengths only
when BOTH chains rank 1, so X=`["x.test","*.foo.test"]` vs Y=`["*.b.foo.test"]` on `a.b.foo.test`
would still pick X on rank. The reference compares by what matched (exact map, then longest
wildcard). **INFERRED, NOT MEASURED on either side — owed to the SPEC as a both-sides arm (§10).**

### 0.12 envoy-go's precedence is a FLAT BITMASK, not the reference's NESTED DESCENT — and A2 does not change that

`specificityScore` sets one bit per SPECIFIED dimension (MSB `destination_port`), compares scores
numerically, and consults finer grain (`breakTie` slots 1, 2, 6) **only on exactly equal bitmasks**.
The reference walks the dimensions as a nested descent and **does not backtrack** — measured, arm G3
(§2): LONG `*.b.foo.test` vs H2SHORT `*.foo.test`+`application_protocols:[h2]`, SNI `x.foo.test`
offering only `http/1.1` → the reference **closes** (it committed to `*.foo.test` at the SNI level and
the ALPN level then failed). Shapes that would diverge under the flat model, **inferred, not driven**:
`*.b.foo.test` vs `*.foo.test`+`transport_protocol: tls` (envoy-go: more bits wins → `*.foo`, **even
with A2**); `/24` alone vs `/16`+`server_names`. **This is a larger row, banked (§4.3); the present row
must not claim it.**

### 0.13 The reference's duplicate-matcher reject has TWO messages and is case-insensitive

Arm H: identical `["*.foo.test"]` ⇒ `filter chain 'fc_TWO' has the same matching rules defined as
'fc_ONE'. duplicate matcher is: {"server_names":["*.foo.test"]}`; overlapping-but-not-identical sets
(`["*.foo.test","a.test"]` vs `["*.foo.test"]`) and case-variant sets (`*.FOO.test` vs `*.foo.test`)
⇒ `multiple filter chains with overlapping matching rules are defined`. rc=1 in `--mode validate` and
at boot. envoy-go rejects only the identical shape (`filter_chains[0] and filter_chains[1] have
identical filter_chain_match — ambiguous selection`) and **boots the overlapping one**. A boot-parity
sibling; banked (§4.5).

### 0.14 A no-match close is NOT counted in the reference's `downstream_cx_total`

Arm A1 on the reference: three connections, `downstream_cx_total` **2**, `no_filter_chain_match`
**1**. **Any cross-side `downstream_cx_total` pin on an arm that includes a no-match connection would
be off by the no-match count.** envoy-go emits no `no_filter_chain_match` at all (phase 98 §1.3).
Recorded for the SPEC's pin selection.

---

## 1. The pick, and why it is defensible as "smallest first"

### 1.1 Charter, in one sentence

Among eligible filter chains that tie on envoy-go's specificity vector and whose `server_names`
matched the SNI through `*.` wildcards, select the chain whose **matching wildcard suffix is
longest**, independent of declaration order — so the connection the reference serves is served, not
closed.

### 1.2 Why "smallest defensible" selects it — a trade-off, stated, not a ranking

| candidate | prod floor (`--numstat`, built + run) | RED / 398 | reference measured at this tip? | cross-side surface | severity |
|---|---|---|---|---|---|
| toolchain fold (§0.1-0.4) | `GOTOOLCHAIN` in a gate command + one test re-point | n/a | n/a — no reference side | **none** | gates, not traffic |
| **SNI longest-suffix (THIS ROW)** | **`22 0`, one file** | **0** (§0.10) | **YES — A/B/C/D/E, both orders** | **serve vs CLOSE, cross-side** | live traffic dropped |
| SNI case-insensitivity | `4 1` pattern side **+ an unmeasured input side** | 0 | YES (F1-F3) | serve vs DEFAULT | wrong chain served |
| `listener_filters_timeout` enforcement | not prototyped | — | YES (phase-98 PLAN) | timeout + drop + a NEW stat name | resource exhaustion |
| nested descent (§0.12) | not prototyped; rewrites `SelectChain` | — | ONE arm (G3) | several | wrong chain / close |

The toolchain fold is cheapest by every figure and is **not chartered**: it has no cross-side surface
(the `catchAllCount` precedent, phase 98 §4.1). It is instead **owed to this row's IMPL as a fold-in**
(§10 item 8), because every future IMPL's gates (b) and lint are otherwise red for a reason no row owns.

Of the rows WITH a cross-side surface, this is the smallest with a complete both-sides measurement,
the only one whose repair is a single slot in a single function, and the most severe — a connection the
reference serves is closed, silently past `--mode validate`, with a default chain present. It has been
the banked front-runner for two phases; **this stage re-measured it rather than inheriting it (method
note 69) and the banked framing held** — plus four facts no document had (§0.9, §0.11, §0.12, §0.14).

### 1.3 Why case-insensitivity is NOT folded in — against the router's "land WITH, not alone"

The router says case-sensitivity *"should land WITH the longest-suffix row, not alone."* Measured,
it should not:

1. **Different repair sites.** Precedence is one `breakTie` slot. Case parity needs the PATTERN folded
   (`parseChainSpec`, `4 1`) **and the INPUT folded** — F2 with an uppercase SNI on the wire
   (`openssl s_client -servername BAR.FOO.TEST` + a raw HTTP/1.1 request) still served **DEFAULT** under
   the pattern-only prototype. The input side runs through `tls_inspector` **and** the QUIC SNI path —
   a wider blast radius than this row's.
2. **Independent gates.** No precedence arm needs mixed case; no case arm needs two wildcards.
3. **The reference's case-insensitivity extends to its DUPLICATE check** (§0.13, H3), which drags in
   the boot-parity sibling. A case row is honestly three edits; bundling it triples this row.

**The one real coupling is recorded for the SPEC:** the longest-suffix comparison must be written so a
later case fold composes (compare lengths, not raw strings). Banked in §4.2 with its measurements.

### 1.4 What this row does NOT buy — stated plainly

- It does **not** make precedence a nested descent (§0.12). Mixed-dimension shapes stay flat-bitmask.
- It does **not** fold case (§1.3), add `no_filter_chain_match` (§0.14), or change the duplicate-matcher
  reject (§0.13).
- It does **not** touch the universal `"*"` tier or partial-wildcard acceptance (phase 98 §0.6-0.7).

---

## 2. The defect, MEASURED — both sides, matched negatives, reversal controls

### 2.1 The rigs and their controls, stated BEFORE the results

- **Reference:** `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
  (`contrib-v1.37.2`; the local `docker images --digests` digest was verified equal, every container
  ran BY DIGEST). One container per config, named `p99ref-<ARM>`, `-p 16201:10000 -p 16202:9901`,
  `/ready` polled to LIVE, torn down BY NAME (`docker ps -a --filter name=p99ref-` → 0 afterwards; no
  other container touched — sibling `cpj-*`/`cp-*` sessions were live throughout). Certs via
  `inline_string:`.
- **Subject:** `go build -o <scratch>/envoy-go ./cmd/envoy-go` at `b9aee47d`, booted `-c <file>`, ports
  `16250-16262`, certs via `filename:`, killed by captured PID. **Each chain needs its own
  `stat_prefix`** — two HCMs sharing one on a listener panic (`duplicate metric registration`, rc=2,
  boot and validate): the banked row-78-class panic, reproduced incidentally (§4.6).
- **Both:** `listener_filters: [tls_inspector]`; every chain is an HCM whose `direct_response` body
  names the chain (`LONG`, `SHORT`, `EXACT`, `P`, `DEFAULT`, …); client `curl -sS -k --resolve
  SNI:PORT:127.0.0.1 https://SNI:PORT/`. **The observable is the served BODY, or the connection failure
  (curl rc + error)** — never handshake completion (method note 7h).
- ⚠️ **The probe client rewrites an input:** curl LOWERCASES the SNI (captured by a local `openssl
  s_server -tlsextdebug`: `--resolve BAR.FOO.TEST` sent `bar.foo.test`). Every mixed-case arm was driven
  with `openssl s_client -servername <SNI> -quiet` plus a `GET / HTTP/1.1` written after the
  handshake (method note 7h: name the inputs the probe SENDS).

### 2.2 The result

| arm | config (declared order) | SNI | reference | subject @ `b9aee47d` |
|---|---|---|---|---|
| A1 | LONG `*.b.foo.test`, SHORT `*.foo.test` | `a.b.foo.test` | **LONG** | **CLOSED** (rc=35, `ambiguous`) |
| A1 | ″ | `x.foo.test` (matched negative: SHORT is live) | SHORT | SHORT |
| A1 | ″ | `nomatch.example` (no default) | closed, `no_filter_chain_match` +1 | closed |
| **A2** | **SHORT, LONG — pure reorder** | `a.b.foo.test` | **LONG** | **CLOSED** |
| A2 | ″ | `x.foo.test` | SHORT | SHORT |
| B1 | `*.c.b.foo.test`, `*.b.foo.test`, `*.foo.test` | `z.c.b.foo.test` / `y.b.foo.test` / `x.foo.test` | L3 / L2 / L1 | **CLOSED / CLOSED** / L1 |
| B2 | reversed | ″ | L3 / L2 / L1 | **CLOSED / CLOSED** / L1 |
| C1 | WILD `*.foo.test`, EXACT `a.foo.test` | `a.foo.test` / `q.foo.test` | EXACT / WILD | EXACT / WILD — **AGREE** |
| C2 | reversed | ″ | EXACT / WILD | (C1 only driven on subject) |
| D1/D2 | P {`destination_port`, `*.foo.test`}, Q {`*.b.foo.test`}, both orders | `a.b.foo.test` | **P** | **P** — **AGREE** |
| D3 | P′ {**other** port, `*.foo.test`}, Q (matched negative: Q is live) | `a.b.foo.test` | Q | not driven |
| **E** | A's two chains **+ `default_filter_chain`** | `nomatch.example` | DEFAULT | DEFAULT |
| **E** | ″ | `a.b.foo.test` | **LONG** | **CLOSED** |
| H1 | two chains, identical `["*.foo.test"]` | — | rc=1 validate + boot | rc=1 validate + boot — **AGREE** (different message) |

**After the A2 prototype** (§6), the subject re-driven end to end: A1 and A2 → **LONG** / SHORT; B1 and
B2 → L3 / L2 / L1; E → DEFAULT / **LONG**. C, D, F, H unchanged. **Every chartered arm then matches the
reference.** Arms C and D agree at the un-fixed tip **for a reason the mechanism can supply** (exact
rank 0 < wildcard rank 1; the port bit dominates) — not false agreement (method note 58), but they
carry no information about this repair, and the SPEC must treat them as regression guards, not evidence.

### 2.3 The mechanism

`SelectChain` (`chainmatch.go`; three production call sites: its own definition, `manager.go` TCP just
after the phase-98 `raw_buffer` stamp, `quic.go`) filters to eligible chains, scores each by the
presence bitmask, and on an exact score tie calls `breakTie`. `breakTie`'s slot 2 compares
`sniSpecificityRank` — exact **0**, any `*.` **1**, `*` **2** — and on equal rank falls through to slot
6 and returns `nil`, which `SelectChain` turns into `ErrAmbiguousChainMatch`; `manager.go` logs
`chain-match: ambiguous filter_chain selection` and closes. **Slots 1 and 6 compare CIDR prefixes by
LENGTH; slot 2 refuses to compare wildcard suffixes by length** — the asymmetry phase 98 §0.2 named,
confirmed unchanged at this tip.

⚠️ **The QUIC path shares `SelectChain`**, so the repair changes QUIC selection too. That is parity-
correct if the reference applies the same server_names ordering on QUIC listeners — **measured on TCP
only here; owed to the SPEC (§10).**

### 2.4 Arms NOT run — recorded, not glossed

- The **mixed-set** arm of §0.11, on either side.
- C2 and D3 on the subject (both agree by mechanism; not evidence either way).
- The G arms (ALPN) on the subject — G3 is the nested-descent witness for a banked row, not this one.
- Any QUIC listener, either side.
- Bare `foo.test` against `*.foo.test` on the subject (the reference does NOT match it — C1).

---

## 3. Hazards for the SPEC

### 3.1 The repair's SHAPE encodes an unmeasured parity answer — twice

(a) The prototype compares lengths **only when both chains rank 1**; the reference compares **the
matched pattern** (§0.11). Choosing "whole-set rank, then suffix length" vs "rank of the matched
pattern" is a parity answer; **measure the mixed-set arm on the reference before choosing** (method
note 37). (b) The prototype measures `len(p)` of the pattern, i.e. including `*.`; that orders
identically to the suffix length, but the SPEC must say which it pins.

### 3.2 A green suite is not evidence here — the NC roster must score PER ARM

§0.10: fix, un-fix and baseline are all green at 398. **Every new unit arm must be shown RED under the
inverted patch** (shortest wins) **and** under the un-fixed tip, per arm, with the inverted patch's
liveness proven end to end as this stage did.

### 3.3 Three normative documents state the false build-time claim

`chainmatch.go:54-59`, `DECISIONS.md:3157` (ADR-0081 clause 5) and `BEHAVIOR_CONTRACT.md:4368`. The
agent's occurrence union (four case-insensitive matchers — `ambiguous`; the symbol set
`ErrAmbiguousChainMatch|breakTie|sniSpecificityRank|chainSpecificityRank`; `specificity|more-specific|most-specific`;
`suffix|longest`) also hits `chainmatch.go` 25-27, 106, 188, 197, 213, 215-216, 304-314;
`manager.go` 731, 1090-1091, 1299; `chainmatch_test.go` 87, 89, 143, 173-184; `fuzz_test.go` 38-40;
`manager_test.go` 1336, 1370-1405, 3417-3500; `quic_test.go` 1129-1130, 1424; `DECISIONS.md` 1024 and
1040 (ADR-0033), 2881 and 2893 (ADR-0077), 3154 and 3157 (ADR-0081), 3208 and 3219 (ADR-0078);
`BEHAVIOR_CONTRACT.md` 4367-4368. ⚠️ **This is ONE agent's union under its own vocabulary — method
notes 49, 77, 78: inherit it as a FLOOR, re-run for GROWTH, and never let a re-derivation shrink it.**
The `ROADMAP.md:160` hit is the closed phase-98 row narrating this candidate — historical.

### 3.4 `TestSelectChainAmbiguousReturnsError` SURVIVES — read, not assumed (a draft of this section was wrong)

The first draft of this section said the test "must be RE-POINTED". **Reading its arms refutes that:**
`chainmatch_test.go:173-186` builds two chains carrying only `TransportProtocol: "tls"`, identical on
every dimension, with no `server_names` at all — slot 2 never engages, so the repair cannot move it.
**It stays green and stays meaningful.** ⚠️ It is also the ONLY unit pin on `ErrAmbiguousChainMatch`,
and it is not a server_names shape, so it is no evidence about this row in either direction.

### 3.5 The fixture needs committed PKI and one identifier trap

The reference container cannot read `filename:` certs; every TLS fixture commits `pki/` plus
`gen/main.go` and templates `inline_string:` (0121's own yaml comment at `:39`). **Each chain needs a
distinct `stat_prefix`** or the subject panics (§2.1) — the same class as fixture `0122`'s load-bearing
`chain_indexed` ≠ `chain_default`.

---

## 4. Rejected alternatives — every cost RE-DERIVED at this tip

### 4.1 The toolchain fold (§0.1-0.4) — **REJECTED as a row: NO cross-side surface. OWED as a fold-in (§10 item 8).**

Lint: invoke under `GOTOOLCHAIN=go1.26.2` (rc=0, live). Compressor: re-point the assertion so it tests
that the level is **plumbed**, not that the stdlib's output differs — the form that cannot rot again
(router's own split, option (b)). A v2 linter move is a separate, ADR-gated re-baseline (§0.4).

### 4.2 SNI case-insensitivity — **REJECTED; banked with its measurements (§1.3)**

Reference: case-insensitive for pattern and input, wildcard and exact (F1-F3, all three spellings).
Subject: neither side folded. Pattern-only fold `4 1` in `manager.go` fixes F1 and **not** F2-on-the-wire.
0 RED at baseline and under the prototype — **nothing pins case at all**.

### 4.3 Nested-descent precedence (§0.12) — **REJECTED for size; banked with ONE reference arm measured (G3)**

The subject side of G3 and every mixed-dimension shape are unmeasured. The repair rewrites
`SelectChain`'s scoring model and ADR-0081's clause 3. The largest chain-match divergence known.

### 4.4 `listener_filters_timeout` non-enforcement — **REJECTED; banked, both sides measured at the phase-98 PLAN**

Not prototyped this stage. Full parity needs a NEW stat name (`downstream_pre_cx_timeout`, **0** Go
files) and timing-dependent arms; it is a resource-exhaustion characteristic that argues for its own
row. **Next in line after this one.**

### 4.5 Duplicate-matcher reject parity (§0.13) — **REJECTED; new, banked.** Boot-parity sibling.

### 4.6 The HCM `stat_prefix` duplicate-registration panic — **REJECTED; NOT re-derived, reproduced incidentally**

Phase 97 §4.1 adjudicated it. This stage's subject agent hit it on its first config (rc=2, boot AND
validate, `duplicate metric registration`), which re-confirms it is live and one config-line away from
any multi-chain fixture — not a re-measurement of its cost.

### 4.7 `server_names` partial-wildcard acceptance and the `"*"` tier — **REJECTED; unchanged from phase 98 §4.3.**

### 4.8 The driver-owned receiver port race, the dead-port false-green, `ROADMAP.md:230`'s stale claim — **REJECTED as rows; unchanged.** `:230` sits inside a sentinel window on a margin of one: **recorded, not tidied.**

---

## 5. Family attribution

**A Listener / chain-match MAINTENANCE row claiming NO family ordinal**, on the row-85-through-91 and
95-98 precedent. It extends no family charter and opens none.

---

## 6. The cost FLOOR — prototyped, run, reverted

`22 0` in `internal/listener/listenerfilter/chainmatch.go` (`git diff --numstat` in a throwaway
worktree, since removed): a `longestWildcardSuffix(patterns, sni)` helper and a length comparison inside
`breakTie` slot 2 when both chains rank 1. Built and run end to end (§2.2) and against the five-selector
set (§0.10: rc=0, RUN=398, 0 RED).

⚠️ **THIS IS A FLOOR, NOT AN ESTIMATE** (`reference_measured_prototype_is_a_lower_bound`, now twenty
rows). The row additionally owes: the §3.1 shape decision (possibly a wider slot-2 rewrite), unit arms
that the inverted patch reddens, the §3.3 occurrence-set reconciliation across code
comments and three normative documents, a new fixture (comparable-shape floor **849-1393** added lines,
§0.7), an ADR, the ledger entry, and the §10 item-8 fold-in.

---

## 7. The differential measurement

### 7.1 There is no existing gate — stated plainly

`git grep -l server_names -- test/fixtures` → **`0002-tls-tcp` only**, with EXACT names (`alpha`,
`beta`). **No fixture has two wildcards, so no fixture can see this defect** on either side.

### 7.2 What the gate must do, and the trap in it

A new fixture **`0124`** (reference port **`15124`**, reserved for it in prose at
`0123/README.md:293` and `0123/driver/driver.go:115` — its only two hits under `test/ internal/ cmd/`;
`15225` and `15324` read **0**, re-census before use). Arms: A1 and A2 as **two listeners that differ
only in declaration order** (the reversal pair, in one fixture), each with `a.b.foo.test` → LONG and the
matched negative `x.foo.test` → SHORT; a default-chain listener carrying E. ⚠️ **The trap:** any
`downstream_cx_total` pin on an arm with a no-match connection is off by the reference's no-match count
(§0.14) — pin the served BODY per arm and avoid no-match connections in a counted listener.
⚠️ **The driver must set SNI explicitly** (method note 7h; the `0104` precedent). Prediction: the stat
surface moves **+0 NAMES**.

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT, BOTH SIDES OF THIS STAGE'S OWN ADD

All commands copied verbatim from `next-prompt.txt`, `/usr/bin/grep` named.

### 8.1 PRE-ADD, at `b9aee47d` (`ROADMAP.md` 248 lines, tail row 98 `done` at `:160`)

(1) **SILENT** · (2) **SIX** at `:208 :214 :220 :230 :236 :244` · (3) **SILENT**.

### 8.2 The four NCs and the check-(2) positive control, PRE-ADD — ALL FIRED

NC-A: substitution inspected first, `NC LANDED? [ in-progress ]`, then **ONE** line, `NOT DONE: row 62`
· NC-B (`want=129`): **ONE**, `GATE FAIL: examined 130 data rows, expected 129` · NC-C: residual **0**,
`NEVER OPENED: gRPC   <- NC FIRED` · NC-D: **96 / 68** under `--` · check-(2) positive control:
residual **0**, **6** substitutions asserted.

### 8.3 Escape-aware malformed set and per-line digests, PRE-ADD

`sed 's/\\|//g' ROADMAP.md | awk -F'|' '/^\| *[0-9]/ && NF!=8'` → exactly **{57, 69}** at file lines
**119** (NF 9) and **131** (NF 10). Per-line md5, **trailing newline INCLUDED** (`sed -n 'Np' f |
md5sum`, first 12 hex): `208 10d7807bf02d` · `214 4a92f7e62fc6` · `220 2a7eb298b9fd` ·
`230 242e53c6f7a3` · `236 b2680e6f4fbf` · `244 6caa1c3ce0e7` — **byte-identical to the phase-98 close.**

### 8.4 POST-ADD — measured on the other side of this stage's own ADD

Row 99 installed after `:160` as `in-progress`, gated FIRST in a scratch file: **8 fields naive, 8
escape-aware**, and **zero** hits for either sentinel match phrase (and zero for the bare word
`deferred`). `ROADMAP.md` **248 -> 249** (`git diff --numstat`: `1 0`). Re-run at `want=131`:

| check | pre-ADD (§8.1-8.2) | **post-ADD** |
|---|---|---|
| (1) | SILENT | **ONE** — `NOT DONE: row 99` |
| (2) | SIX at `:208 :214 :220 :230 :236 :244` | **SIX at `:209 :215 :221 :231 :237 :245`** — every window shifted +1, as an insertion ABOVE them must |
| (3) | SILENT | SILENT |
| NC-A (row 62 doctored) | ONE | **TWO** — `NOT DONE: row 62`, `NOT DONE: row 99` (substitution inspected: `NC LANDED? [ in-progress ]`) |
| NC-B | ONE at `want=129` | **TWO at `want=130`** — `NOT DONE: row 99`, `GATE FAIL: examined 131 data rows, expected 130` |
| NC-C | FIRED, residual 0 | FIRED, residual 0 |
| NC-D (`--`) | 96 / 68 | **96 / 68** — unmoved; the new cell spells no `-family row` |
| check-(2) positive control | residual 0, 6 substitutions | residual 0, 6 substitutions |
| escape-aware malformed set | {57, 69} at `:119`, `:131` | **{57, 69} at `:119`, `:131`** — unmoved (both above the insert) |
| row 99 NF | — | **8 naive, 8 escape-aware** |

Per-line md5 at the SHIFTED lines, trailing newline INCLUDED: `209 10d7807bf02d` · `215 4a92f7e62fc6` ·
`221 2a7eb298b9fd` · `231 242e53c6f7a3` · `237 b2680e6f4fbf` · `245 6caa1c3ce0e7` — **all six
byte-identical to §8.3: the windows MOVED and did not CHANGE.**

Fixture registration, re-run verbatim (the blank-import extractor): **dirs 125 = imports 125**, both
`comm` directions EMPTY, split **101 `driver/` + 24 `inputs/`** — unmoved, as a BRAINSTORM adds no
fixture. Rename NC (one import renamed in a scratch copy) → `comm -23` **1**, `comm -13` **1**.

⇒ **THE SENTINEL DOES NOT FIRE on either side of this ADD. `stop` was evaluated and NOT created.**

---

## 9. Findings the next stage must not re-learn

1. **Lint runs under `GOTOOLCHAIN=go1.26.2`** and the tip is clean; the ambient go1.27.1 is what breaks
   it (§0.1). Rebuilding the linter does not help (§0.3).
2. **Gate (b)'s compressor RED is the same toolchain move** (§0.2).
3. **curl lowercases SNI**; mixed-case arms need `openssl s_client -servername` (§2.1).
4. **Every multi-chain probe config needs a distinct `stat_prefix` per chain** (§2.1, §4.6).
5. **The reference's no-match close does not count in `downstream_cx_total`** (§0.14).
6. **A default chain does not rescue the subject's ambiguity close** (§0.9).
7. **The suite is blind to slot 2 in both directions** (§0.10).
8. **The QUIC path shares `SelectChain`** (§2.3).

---

## 10. What the SPEC owes

1. **Measure the mixed-set arm on BOTH sides** (§0.11): X=`["x.test","*.foo.test"]` vs Y=`["*.b.foo.test"]`,
   SNI `a.b.foo.test`, both orders — and choose the repair shape from it (§3.1).
2. **Measure the same precedence on a QUIC listener on the reference** (§2.3), or state that the SPEC
   defers it and why the shared `SelectChain` change is still safe there.
3. **Re-derive the occurrence set** (§3.3) as a FLOOR — inherit the union, look for growth, resolve every
   docs hit to its ADR — and decide per hit: repair, re-tense, or leave (with a reason).
4. **Confirm `TestSelectChainAmbiguousReturnsError` survives** (§3.4) and decide which shape, if any,
   remains reachable as `ErrAmbiguousChainMatch` on the server_names axis after the repair.
5. **Specify unit arms that go RED at the un-fixed tip AND under the inverted patch**, per arm (§3.2).
6. **Charter fixture `0124`** (§7.2) — ports censused, per-chain `stat_prefix`, PKI, SNI set by the
   driver, body pins, no counted no-match — and a floor re-derived by SHAPE (§0.7).
7. **Draft ADR-0321 §Context** in the house `> **STATUS: PROPOSED` block form, amending ADR-0081
   clauses 4-5 (and noting ADR-0078's sub-ordering), and **predict the ledger delta (+0 names)**.
8. **Charter the toolchain fold-in** (§4.1): the IMPL's lint gate runs under `GOTOOLCHAIN=go1.26.2`
   (with the §0.1 positive control repeated), and the compressor assertion is re-pointed to "level is
   plumbed", RED-first under go1.27.1, green under both toolchains.
9. **State what the row does NOT buy** (§1.4) in the ADR's §Consequences.

---

## 11. Probe hygiene

- **Worktrees:** stage worktree `/home/esa/git/wt-phase-99-brainstorm` (branch `phase-99-brainstorm`, off
  `b9aee47d`); throwaway `lintwt` (config-migration probe) and the subject agent's `subjwt` both
  **created and removed**, verified by `git worktree list`. The stale `wt-phase-98-impl` (§0.6) is
  removed at this stage's close.
- **Docker:** one agent only (the reference agent), containers `p99ref-*`, torn down by name; no other
  container touched.
- **Ports:** band `16200-16299`, censused first (`git grep -In '162[0-9][0-9]' -- test/ internal/ cmd/`
  → rc=1, zero text hits; nothing bound under `ss -tan`/`ss -uan`). Reference `16201`, `16202`, `16210`;
  subject `16250-16262`. ⚠️ **This paragraph now spells band numbers; it is a hit in the next census.**
- **Scratch:** everything under the session scratchpad (`ref/`, `subj/`, `gobin*/`, `lint*.txt`); nothing
  in any worktree. **No production `.go` written, none of the six gates run — a BRAINSTORM's SCOPE, not
  an omission.** The three standing departures (no `REVIEW.md` for 93-98, lint, gate (b)) are **not
  fixed by this stage** — two of them are re-diagnosed (§0.1, §0.2) and owed to this row's IMPL.
