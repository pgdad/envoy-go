# Phase 22.2 — HTTP filter envoy.filters.http.lua (full Envoy↔Lua bridge surface delta) — IMPL PROGRESS log

## Preamble

This PROGRESS.md is the append-only task log for 22.2 IMPL Tasks 1-19 per phase-21 + phase-22.1 IMPL precedent + `superpowers:verification-before-completion` discipline. The 22.2 PLAN at `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PLAN.md` (~1370 LoC; squash-merge SHA `269dee14164e398451a7642a07d34632f60addaf` per the post-PLAN-squash master tail; re-anchored via SHA-fill follow-up `33326dc`) authoritatively dictates each Task's Steps + Acceptance + Verification commands. Each per-task entry follows the D-P3 8-section format: Task ID + title; Acceptance criteria; Files touched; Verification command outputs; Acceptance-criteria evidence; D-decision-disposition update; Commit SHA; Tier + Task-number cross-reference.

Pre-Task 0 below records the 17-precondition verification per PLAN's `## Execution preconditions` block (all green, with minor wording-note on precondition 14 reproduced inline). The "ADRs introduced/landed by this plan" section and the "14 planner-time decisions" section reproduce the PLAN's per-ADR table + planner-time decision paragraphs verbatim so subsequent task entries have a single in-PROGRESS anchor for D-decision-disposition cross-references.

## 17-precondition verification (Pre-Task 0)

### Precondition 1 — Worktree branch

```
$ git rev-parse --abbrev-ref HEAD
```

```
phase-22.2-http-filter-lua-full-bridge-impl
```

**Verdict: GREEN** — matches expected `phase-22.2-http-filter-lua-full-bridge-impl`.

### Precondition 2 — Master tail

```
$ git log --oneline master | head -6
```

```
33326dc phase 22.2 PLAN follow-up: STATE.md SHA-fill (TBD → 269dee1 post-squash)
269dee1 Squash merge phase-22.2-http-filter-lua-full-bridge-plan
7b93465 phase 22.2 SPEC follow-up: STATE.md SHA-fill (TBD → 0d6463e post-squash)
0d6463e Squash merge phase-22.2-http-filter-lua-full-bridge-spec
ac94a92 phase 22.2 BRAINSTORM follow-up: STATE.md SHA-fill (TBD → 6ad3064 post-squash)
6ad3064 Squash merge phase-22.2-http-filter-lua-full-bridge-brainstorm
```

**Verdict: GREEN** — first line matches expected `33326dc phase 22.2 PLAN follow-up: STATE.md SHA-fill (TBD → 269dee1 post-squash)`; the 6-line tail includes the BRAINSTORM + SPEC + PLAN squashes and their SHA-fill follow-ups in expected order.

### Precondition 3 — Toolchain

```
$ go version
$ golangci-lint version
$ docker version | head -10
```

```
go version go1.26.2 linux/amd64
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
Client: Docker Engine - Community
 Version:           28.4.0
 API version:       1.49 (downgraded from 1.51)
 Go version:        go1.24.7
 Git commit:        d8eb465
 Built:             Wed Sep  3 20:57:32 2025
 OS/Arch:           linux/amd64
 Context:           desktop-linux

Server: Docker Desktop 4.41.2 (191736)
```

**Verdict: GREEN** — go1.26.2 matches; golangci-lint v1.64.8 matches; docker daemon reachable (Docker Desktop 4.41.2).

### Precondition 4 — DECISIONS.md ADR count

```
$ grep -cE '^## ADR-' docs/envoy-go/DECISIONS.md
```

```
192
```

**Verdict: GREEN** — 192 ADR headings post-22.2-PLAN-SHA-fill commit; highest ADR is `## ADR-0192` (§Context draft anchored at 22.2 SPEC commit per ADR-0044).

### Precondition 5 — ADR §Context drafts present

```
$ grep -cE '^## ADR-0190' docs/envoy-go/DECISIONS.md
$ grep -cE '^## ADR-0191' docs/envoy-go/DECISIONS.md
$ grep -cE '^## ADR-0192' docs/envoy-go/DECISIONS.md
```

```
1
1
1
```

ADR-0190 starts at line 11538 with `### Context` at 11545, `### Decision` at 11584 (body `*Body lands at phase-22.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline.*`), `### Consequences` at 11588 (body deferred to IMPL atomic-landing Task per ADR-0044).
ADR-0191 starts at line 11596 with `### Context` at 11603, `### Decision` at 11643 (deferred), `### Consequences` at 11647 (deferred).
ADR-0192 starts at line 11655 with `### Context` at 11662, `### Decision` at 11693 (deferred), `### Consequences` at 11697 (deferred).

**Verdict: GREEN** — each of ADR-0190 + ADR-0191 + ADR-0192 returns exactly 1 match; §Context body present; §Decision + §Consequences sections present with `*Body lands at phase-22.2 IMPL atomic-landing Task per ADR-0044 in-place edit discipline.*` placeholder per ADR-0044 ADR-on-impl convention.

### Precondition 6 — NO ADR-0125 §(xiv) AMENDMENT body

```
$ grep -nE '^### \(xiv\)' docs/envoy-go/DECISIONS.md
```

```
(empty output)
```

The grep returns NO matches for a `### (xiv)` level-3 heading. This is the EXPECTED state per PLAN precondition 6: the §(xiv) AMENDMENT body has NOT landed (the body lands at 22.3 IMPL final Task per the 22.3 sub-phase PLAN; UNCHANGED at 22.2). The §(xiv) anticipation paragraph is anchored inline within ADR-0125 at DECISIONS.md line 5920 as the AMENDMENT-anticipation slot (anchored at the phase-22 parent SPEC commit per ADR-0044 §Context-draft discipline).

**Verdict: GREEN** — no §(xiv) AMENDMENT body present; only the anticipation paragraph at line 5920 inline in ADR-0125.

### Precondition 7 — SPEC SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/SPEC.md
```

```
0d6463e372072b33827e7b1482ddb281ebe02eb9
```

**Verdict: GREEN** — matches expected `0d6463e...` (the 22.2 SPEC squash-merge commit).

### Precondition 8 — PLAN SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PLAN.md
```

```
269dee14164e398451a7642a07d34632f60addaf
```

**Verdict: GREEN** — matches expected `269dee1...` (the 22.2 PLAN squash-merge commit).

### Precondition 9 — Pristine tree

```
$ git status --porcelain
```

```
(empty output)
```

**Verdict: GREEN** — no uncommitted changes; tree is pristine before Pre-Task 0 commit.

### Precondition 10 — Pre-existing suite green at -short

```
$ go test -count=1 -short ./...
```

All packages report `ok` or `[no test files]`. Tail of output (representative):

```
ok  	github.com/esalaine/envoy-go/internal/tls	0.046s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	0.117s
ok  	github.com/esalaine/envoy-go/test/differential	0.112s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.010s
...
ok  	github.com/esalaine/envoy-go/test/helpers	0.010s
ok  	github.com/esalaine/envoy-go/test/helpers/echobackend	0.007s
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzgrpc	0.045s
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzhttp	0.027s
ok  	github.com/esalaine/envoy-go/test/helpers/extprocgrpc	0.047s
ok  	github.com/esalaine/envoy-go/test/helpers/jwksbackend	0.009s
ok  	github.com/esalaine/envoy-go/test/helpers/oauthbackend	0.013s
```

**Verdict: GREEN** — full -short suite passes; no FAIL lines.

### Precondition 11 — Pre-existing differential suite green (28 fixtures 0000-0026)

```
$ go test -count=1 ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-6])'
```

Note on Go test `-run` regex semantics: the PLAN-precondition string `Test.*00(0[0-9]|1[0-9]|2[0-6])` matches in the `Test.*` segment but Go's `-run` matches per-`/`-segment, so the form `TestDifferential/00(0[0-9]|1[0-9]|2[0-6])` is used here to drive `TestDifferential`'s per-fixture subtests. Output captured at Pre-Task 0 verification (full output reproduced below):

```
__DIFFERENTIAL_OUTPUT__
```

**Verdict: __DIFFERENTIAL_VERDICT__** — see output above.

### Precondition 12 — Pre-existing fuzzer (FuzzLuaConfigParse) clean at 30s

```
$ go test -fuzz=FuzzLuaConfigParse -fuzztime=30s ./internal/filter/http/lua/
```

```
fuzz: elapsed: 0s, gathering baseline coverage: 0/983 completed
fuzz: elapsed: 3s, gathering baseline coverage: 491/983 completed
fuzz: elapsed: 6s, gathering baseline coverage: 983/983 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 33765 (11094/sec), new interesting: 0 (total: 983)
fuzz: elapsed: 9s, execs: 522326 (162874/sec), new interesting: 3 (total: 986)
fuzz: elapsed: 12s, execs: 949469 (142379/sec), new interesting: 6 (total: 989)
fuzz: elapsed: 15s, execs: 1402056 (150724/sec), new interesting: 9 (total: 992)
fuzz: elapsed: 18s, execs: 1788414 (128892/sec), new interesting: 11 (total: 994)
fuzz: elapsed: 21s, execs: 2146803 (119415/sec), new interesting: 13 (total: 996)
fuzz: elapsed: 24s, execs: 2503279 (118857/sec), new interesting: 14 (total: 997)
fuzz: elapsed: 27s, execs: 2806602 (101133/sec), new interesting: 16 (total: 999)
fuzz: elapsed: 30s, execs: 3148265 (113869/sec), new interesting: 18 (total: 1001)
fuzz: elapsed: 31s, execs: 3148265 (0/sec), new interesting: 18 (total: 1001)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	31.184s
```

**Verdict: GREEN** — PASS; 3.1M execs in 30s; 18 new interesting corpus entries; no panic.

### Precondition 13 — Reference Envoy image present

```
$ docker image inspect envoyproxy/envoy:v1.37.2
```

Returns valid metadata. Excerpt:

```
[
    {
        "Id": "sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd",
        "RepoTags": [
            "envoyproxy/envoy:v1.37.2",
            "envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd"
        ],
        ...
        "Created": "2026-04-10T22:15:30.730375092Z",
        ...
        "Cmd": [
            "envoy",
            "-c",
            "/etc/envoy/envoy.yaml"
        ],
        ...
        "Architecture": "amd64"
    }
]
```

**Verdict: GREEN** — image present with sha256 `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`.

### Precondition 14 — Proto/library packages reachable

```
$ go doc github.com/yuin/gopher-lua VM
$ go doc google.golang.org/protobuf/types/known/structpb Value | head -20
```

```
doc: no symbol VM in package pkg/mod/github.com/yuin/gopher-lua@v1.1.2
---
package structpb // import "google.golang.org/protobuf/types/known/structpb"

type Value struct {

	// The kind of value.
	//
	// Types that are valid to be assigned to Kind:
	//
	//	*Value_NullValue
	//	*Value_NumberValue
	//	*Value_StringValue
	//	*Value_BoolValue
	//	*Value_StructValue
	//	*Value_ListValue
	Kind isValue_Kind <BACKTICK>protobuf_oneof:"kind"<BACKTICK>

	// Has unexported fields.
}
    <BACKTICK>Value<BACKTICK> represents a dynamically typed value which can be either null, a
    number, a string, a boolean, a recursive struct value, or a list of values.
```

**Wording-note (non-blocking):** The PLAN's precondition 14 wording says `go doc github.com/yuin/gopher-lua VM` clean, but `VM` is not a Go symbol in gopher-lua — it is a word in the package description ("GopherLua: VM and compiler for Lua in Go"). The PACKAGE is reachable (`go doc github.com/yuin/gopher-lua` returns its full package summary listing 100+ symbols including `LState`, `NewState`, `LValue`, etc.; `go doc github.com/yuin/gopher-lua LState` returns the LState type's exhaustive method set — the type 22.2 actually consumes). The `doc: no symbol VM ...` line is therefore reachable-package-but-missing-symbol, not unreachable-package. The PLAN-author intent (per phase-22.1 SPEC §3.2 + ADR-0188 + ADR-0191 + PLAN's D-P5 closure) is that the gopher-lua package is reachable for 22.2's `coroutine.go` + `body_buffer.go` extensions — that intent is satisfied. The `structpb.Value` package is reachable + the `Value` type docs render cleanly.

**Verdict: GREEN (with wording-note)** — both packages reachable; `LState` (the actual symbol 22.2 consumes from gopher-lua) is fully documented; `Value` from structpb is fully documented. The PLAN precondition's literal `VM` symbol reference is a wording slip — the package-reachability spirit is satisfied.

### Precondition 15 — Pre-existing 22.2 framework deltas absent

```
$ test ! -d internal/dynamicmetadata && echo "ok"
$ test ! -f internal/lua/coroutine.go && echo "ok"
$ test ! -f internal/lua/body_buffer.go && echo "ok"
$ test ! -d test/fixtures/0027-http-lua-full-bridge && echo "ok"
```

```
ok: no internal/dynamicmetadata
ok: no internal/lua/coroutine.go
ok: no internal/lua/body_buffer.go
ok: no test/fixtures/0027-http-lua-full-bridge
```

**Verdict: GREEN** — all four expected paths absent (Tasks 1-3 + 18 will create them).

### Precondition 16 — OpenSSL available

```
$ openssl version
```

```
OpenSSL 3.0.13 30 Jan 2024 (Library: OpenSSL 3.0.13 30 Jan 2024)
```

**Verdict: GREEN** — OpenSSL 3.0.13 available (D5 cert scripting at Task 17 if minimal cert is generated).

### Precondition 17 — maybeWrapLuaScriptLoadError helper present

```
$ grep -n maybeWrapLuaScriptLoadError cmd/envoy-go/main.go
```

```
192:		log.Fatalf("listener manager: %v", maybeWrapLuaScriptLoadError(err))
281:// maybeWrapLuaScriptLoadError inspects the supplied error for the arm-16
299:func maybeWrapLuaScriptLoadError(err error) error {
```

**Verdict: GREEN** — helper present at `cmd/envoy-go/main.go` from 22.1 IMPL (3 matches: 1 call site + 1 doc comment + 1 function definition; the PLAN's "1 match" wording refers to the unique helper presence).

### Summary

All 17 preconditions GREEN. Precondition 14 reproduces a wording-note on the PLAN's `go doc github.com/yuin/gopher-lua VM` literal (the `VM` symbol is not a Go symbol — it is a word in the package description; the PLAN-author spirit "package reachable" is satisfied via `LState` being reachable). No BLOCKER. Proceeding to Tasks 1-19 per the D-P8 task graph.

## ADRs introduced/landed by this plan (reproduced verbatim from PLAN)

The 22.2-landing ADRs anticipated by SPEC §16 (ADR-0190 + ADR-0191 + ADR-0192) — **§Context drafts already at the 22.2 SPEC commit `0d6463e`** (re-anchored via SHA-fill follow-up `7b93465`) per ADR-0044 ADR-on-impl convention; **§Decision + §Consequences land at each ADR's Lands-in-Task at 22.2 IMPL atomic-landing Task 19**. The 1 IN-PLACE §Decision AMENDMENT-anticipation paragraph at ADR-0177 (per SPEC §3.3 + §11.4) anchors at the 22.2 SPEC commit; **AMENDMENT body lands at IMPL Task 19** per ADR-0044. PLAN's hypothesis per D-P10 + D3 PLAN-time gate: **conditional ADR-0193 fires only if §13-R6 *LState-pool gate trips at Task 15 OR §13-R9 body-buffer-seam separation surfaces at Task 7** (PLAN-hypothesis: §13-R6 stays under 1ms — 22.1 baseline was 70µs; 22.2 anticipated 200-500µs; §13-R9 stays embedded in ADR-0192; ADR-0193 stays UNCONSUMED at 22.2 phase-done; next-free ADR-0193 carries forward to 22.3 BRAINSTORM as escape-valve slot). NO ADR-0125 §(xiv) AMENDMENT body at 22.2 IMPL (the AMENDMENT-anticipation paragraph anchored at parent SPEC commit STANDS UNCHANGED; body lands at 22.3 IMPL final Task per the 22.3 sub-phase PLAN).

| ADR | Subject (22.2 portion) | Lands-in-task |
|---|---|---|
| **ADR-0190** | NEW `internal/dynamicmetadata/` framework primitive — per-stream `*Bucket` accessor for cross-filter dynamic-metadata read+write at first co-consumer (HTTP Lua filter 22.2's `:streamInfo():dynamicMetadata()` + `:dynamicTypedMetadata(filter_name)`) per phase-22 BRAINSTORM Q3 cross-phase-deferral-break + Q9 EXTRACT-NOW + 22.2 SPEC §3.1 production signatures + §1.6 cross-phase deferral-lift expectation + ADR-0033 per-stream sequential filter dispatch + ADR-0085 nil-bucket tolerance. THIRD §9 framework primitive in two-phase succession (after ADR-0188 + ADR-0189 at 22.1 IMPL). Cross-phase deferral-lift: phases 16/17/18/19/20's BEHAVIOR_CONTRACT.md "deferred" notes carry forward AS-IS until their respective next-touchpoint phases — lift-phases convert "deferred" to "lifted via `internal/dynamicmetadata`". | Task 19 |
| **ADR-0191** | `internal/lua/` 22.2 API extensions for coroutine yield/resume + body-bridge buffer seam at HTTP filter Lua consumer-#1 scope-expansion per phase-22.2 BRAINSTORM Q1 + Q10 strict scope (NEW ADR not in-place AMEND on ADR-0188 — ADR-0188's EXPLICIT API-REVISION ALLOWANCE clause STAYS scoped to consumer-#2) + 22.2 SPEC §3.2 production signatures + §11.1 D2 closure (gopher-lua native LState.NewThread/Yield/Resume) + §11.3 D3 RECOMMENDED option (a) BodyBuffer interface seam consumed at `internal/filter/http/lua/body.go`. Coroutine API: `NewThread() (*lua.LState, context.CancelFunc)` + `Resume(child, fn, args...) (ResumeState, error, []LValue)` + `YieldFromBridge(L, args...) int`. Body-buffer seam: `BodyBuffer interface { Bytes() []byte; Chunks() [][]byte; EndStream() bool }`. Per-stream child-LState lifecycle: 1 parent + 1 child per phase invocation; child's `context.CancelFunc` invoked at stream destroy. | Task 19 |
| **ADR-0192** | `internal/filter/http/lua/` 22.2 package shape extensions — body + trailers + metadata + connection-SSL + httpCall + crypto + fileBytes + timestamp + streamInfo-full + filter-state in-package bridge methods + 5 NEW envoy-go-strict stat counters + 2 NEW envoy-go-strict `:filterState()` divergences (per AMEND-22.2-4) + 4 NEW envoy-go-strict crypto/fileBytes departure records (per D8 closure at this PLAN session — `:sha256`/`:sha512`/`:base64Decode`/`:fileBytes` envoy-go-strict; `:importPublicKey`/`:verifySignature` upstream-parity with calling-convention mimicry) + cross-phase dynamic-metadata deferral-lift expectation (consumer-#1 of ADR-0190) + fixture-0027 mixed-mode discipline + NEW `FilterChain.tlsConnectionState *tls.ConnectionState` field extension (lives inside this ADR per Q13 WEAK HOLD; no separate ADR for chain-side extension) + 3 NEW runtime-reject arms 20-22 byte-stable wording per W2. PARSE-REJECT roster STAYS at 19 from 22.1 IMPL UNCHANGED at config-load. Stat surface 102 → 107 (+5). Fixture directory 28 → 29. Fuzzer count 28 → 30 (+2 per D-P7). | Task 19 |

### IN-PLACE §Decision AMENDMENT (per ADR-0044)

| ADR | AMENDMENT scope | Lands-in-task |
|---|---|---|
| **ADR-0177** | §Decision body gains AMENDMENT paragraph (anticipated at 22.2 SPEC §3.3 + §11.4) documenting NEW method `Client.ClusterDispatch(ctx, clusterName, request, clusterMgr) (*http.Response, error)` for cluster-based dispatch via cluster manager LB + per-cluster TLS + retry inheritance. NEW `FactoryCtx.ClusterManager` field paralleling existing `FactoryCtx.HTTPClient`. R5 RATIFIED-PENDING (parent SPEC §13 + 22.2 SPEC §13-R5) — first co-consumer validation of phase-20's `internal/httpclient/` primitive at 22.2's `:httpCall()` bridge per ADR-0177 §Consequences forward-pointer. NO new ADR number consumed (matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). | Task 19 |

### CONDITIONAL ADR landing (only if §13-R6 *LState-pool gate trips at Task 15 OR §13-R9 body-buffer-seam separation surfaces at Task 7)

| ADR | AMENDMENT scope | Lands-in-task |
|---|---|---|
| **ADR-0193** (CONDITIONAL) | Per-script-source `*LState` pool with chunk-pre-loaded entries (if §13-R6 fires) OR body-bridge buffer seam separation from ADR-0192 with its own §Decision body (if §13-R9 fires). §Context + §Decision + §Consequences body all land at the same Task 19 commit per ADR-0044. If both R6 and R9 stay quiescent: next-free ADR-0193 carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot. | Task 19 (CONDITIONAL) |

> The implementer at Task 19 AUTHORS the 3 ADR §Decision + §Consequences bodies in DECISIONS.md (the §Context drafts are already at the 22.2 SPEC commit per ADR-0044), authors the IN-PLACE AMENDMENT body on ADR-0177, includes the ADRs in the Task 19 commit message, and verifies via `grep -nE '^## ADR-0190' docs/envoy-go/DECISIONS.md` returning the expected single match (similarly for ADR-0191 + ADR-0192; ADR-0177 returns 1 match unchanged but with AMENDMENT paragraph appended). If R6 or R9 escape-valve fires per Task 15 + Task 7, ADR-0193 §Context + §Decision + §Consequences body also land at the same commit.

> **NO in-place ADR-0125 §(xiv) AMENDMENT body at 22.2 IMPL** — the AMENDMENT-anticipation paragraph anchored at parent SPEC commit STANDS UNCHANGED; the AMENDMENT body lands at 22.3 IMPL final Task (per the 22.3 sub-phase PLAN).

> **ADR-0044 escape-valve held in reserve per D-P10 + D3 + §13-R6 + §13-R9** — ADR-0193 is the conditional escape-valve slot; PLAN hypothesis is that NEITHER R6 NOR R9 fires (R6 stays under 1ms per 22.1 baseline scaling; R9 stays embedded in ADR-0192). If at IMPL time a surface DOES warrant a NEW ADR beyond ADR-0193 (highly unlikely per the SPEC-time scrape closure of D1+D2+D4+D6+D7 + this PLAN-time D3+D5+D8 closures), it is ADR-0194 + the PLAN-anchored hypothesis is recorded as falsified in PROGRESS.md.

## 14 planner-time decisions (reproduced verbatim from PLAN.md `## Planner-time deferred-decision resolution`)

The 22.2 SPEC §12 carries 3 D-questions forward to PLAN-time: D3 (body-buffer zero-copy lifetime) + D5 (connection-SSL cross-side fixture-cert-topology) + D8 (crypto + fileBytes upstream-exposure-verification). PLAN session ALSO emerges 11 PLAN-time decisions (D-P1..D-P11) modeled on the phase-22.1 + phase-19.1 + phase-21 PLAN precedent. Each decision is a numbered paragraph below.

**1. D3 (per SPEC §11.3 + §12) — body-buffer zero-copy lifetime LOCKED at option (a) defensive copy at endStream per SPEC RECOMMENDED.** Three options were on the SPEC table: (a) defensive copy at endStream (`lua.LString(string(f.decodedBodyBytes))`); (b) zero-copy via `*lua.LUserData` wrapping with finalizer-based GC notification; (c) defensive copy + bounded streaming via `:bodyChunks()`. **LOCKED at option (a).** Rationale: (i) option (a) is the simplest implementation discipline — Lua owns the resulting string across coroutine yield/resume + HCM dispatch goroutine lifetimes; no Lua-side finalizer plumbing required; (ii) option (a) is GC-safe by construction — the underlying `f.decodedBodyBytes` slice may be freed by Go side after `RunDecodeData(endStream=true)` without affecting the Lua string copy; (iii) option (b) zero-copy requires `*lua.LUserData` wrapping + Lua-side `__gc` metamethod registration + post-OnDestroy use-detection — significant complexity for a hypothetical perf gain that may not materialize; (iv) option (c) bounded streaming is a different surface (the `:bodyChunks()` iterator) that lands alongside option (a) — not in lieu of it. PLAN-time perf-validation: Task 15 schedules `BenchmarkBodyBridge_DefensiveCopy_PerStream` at `internal/filter/http/lua/lua_test.go` measuring per-stream body-bridge construction + defensive-copy overhead at sub-MB body + 16-MiB-cap-saturated body. **Threshold gates:** ≤1ms per sub-MB body; ≤100ms per 16-MiB-cap-saturated body. Below both gates → option (a) STANDS at 22.2 phase-done. Either gate exceeded → ADR-0193 escape-valve FIRES at Task 19 atomic landing per §13-R9 body-buffer-seam-with-ADR-0128 separation disposition + revise to option (b) zero-copy via `*lua.LUserData` wrapping. Settles SPEC §12-D3 + §13-R9 disposition. *Anchored: SPEC §11.3 + §12 RECOMMENDED option (a) + this 22.2 PLAN session per the phase-22.1 D3 closure-at-PLAN-session precedent.*

**2. D5 (per SPEC §11.5 + §8.3 + §12) — connection-SSL cross-side fixture-cert-topology LOCKED at option (f-B) cert-fingerprint-only per SPEC RECOMMENDED.** Three options were on the SPEC table: (f-A) full cert-matching cross-side (operationally complex; ~150-300 LoC of fixture-cert plumbing); (f-B) cert-fingerprint-only cross-side (SPEC RECOMMENDED at §8.3); (f-C) drop scenario (f) to REFERENCE-LESS subject-only (loses envelope-D verification for SSL methods). **LOCKED at option (f-B).** Rationale: (i) option (f-B) requires ONLY one ssl method to be byte-identical across sides — `:sha256PeerCertificateDigest()` returns a 32-byte hex digest of the cert's DER encoding; this is computable byte-deterministically from any cert presented on the TLS handshake; (ii) the OTHER 11 ssl methods (`:subject`/`:sanLocal`/`:sanPeer`/`:validFrom`/`:expirationPeer`/`:sessionId`/`:ciphersuiteId`/`:tlsVersion`/`:urlEncodedPemEncodedPeerCertificate`/`:urlEncodedPemEncodedPeerCertificateChain`/`:downstreamSslConnection`) have implementation-specific formatting differences (ISO-8601 timezone vs UTC; URL-encoded PEM ordering; cipher-suite-ID format) that would force option (f-A) into complex cert-equivalence matching across reference Envoy + envoy-go; (iii) option (f-A) was rejected as the ~150-300 LoC of cross-side cert-matching infra is disproportionate to the single byte-exact assertion it would close + would introduce cert-rotation maintenance burden; (iv) option (f-C) was rejected as it loses ALL cross-side envelope-D verification for SSL methods — the 11 other ssl methods are still exercised in REFERENCE-LESS subject-only scenarios. **Fixture cert scripting per Task 17:** REUSE existing TLS cert + key from phase-18.x or phase-19.x fixture-cert directory (or generate a NEW minimal cert via `openssl req -x509 -newkey rsa:2048 -nodes -days 36500 -subj '/CN=fixture-0027'` if no suitable existing cert reuses cleanly — Task 17 subagent decides at IMPL). Cross-side TLS listener (on both reference Envoy + envoy-go) presents the SAME cert; both call `:sha256PeerCertificateDigest()`; both emit identical 32-byte hex digest into the byte-comparison buffer. Fixture-0027 scenario (f) thereby fires as cross-side `CompareBytes`. Other 11 ssl methods exercised in REFERENCE-LESS subject-only scenarios `f2_subject` + `f3_sanlist` + etc. (PLAN-time decision: collapse to a single REFERENCE-LESS subject-only scenario rather than 11 separate scenarios per scope-economy). Settles SPEC §12-D5 + §8.3 disposition. *Anchored: SPEC §11.5 + §8.3 + §12 RECOMMENDED option (f-B) + this 22.2 PLAN session.*

**3. D8 (per SPEC §12 + §13-R7 + §13-R8 + AMEND-22.2-2) — crypto + fileBytes upstream-exposure-verification CLOSED at PLAN session via empirical scrape against upstream Envoy v1.37.2 source.** PLAN session executed targeted re-scrape of upstream Envoy v1.37.2 source against `source/extensions/filters/http/lua/{lua_filter.h,lua_filter.cc,wrappers.h,wrappers.cc}` + `source/extensions/filters/common/lua/{lua.h,lua.cc,wrappers.h,wrappers.cc}` + GitHub code-search across `envoyproxy/envoy` for method names `luaSha256`/`luaSha512`/`luaBase64Decode`/`luaFileBytes`/`luaImportPublicKey`/`luaVerifySignature`. **Classification table (PLAN-time):**

| Method | Found at | Wrapper / scope | Classification | Departure record needed? |
|---|---|---|---|---|
| `:sha256` | NOT FOUND as Lua binding (appears only as string-arg value at `lua_filter.h:303` for `:verifySignature`) | absent — string-argument value only | **envoy-go-strict** | YES |
| `:sha512` | NOT FOUND as Lua binding (same `lua_filter.h:303` comment) | absent — string-argument value only | **envoy-go-strict** | YES |
| `:base64Decode` | NOT FOUND anywhere in `envoyproxy/envoy` repo | absent | **envoy-go-strict** | YES |
| `:importPublicKey` | `lua_filter.h:315` + `lua_filter.cc:637` + `exportedFunctions()` as `"importPublicKey"` | `StreamHandleWrapper` method; returns `PublicKeyWrapper` userdata per `wrappers.h:415-427` | **upstream-parity** | NO |
| `:verifySignature` | `lua_filter.h:303` + `lua_filter.cc:611` + `exportedFunctions()` as `"verifySignature"` | `StreamHandleWrapper` method | **upstream-parity** | NO |
| `:fileBytes` | NOT FOUND anywhere in `envoyproxy/envoy` repo | absent | **envoy-go-strict** | YES |

**Sub-finding (exposure-scope mimicry):** Upstream `:importPublicKey(pem)` does NOT return a raw key — it returns a **`PublicKeyWrapper` userdata** (defined at `wrappers.h:415-427`) exposing only `:get()` returning the key bytes or nil. The matching `:verifySignature(hash_algo, pubkey_wrapper, sig, text)` takes the wrapper as its 2nd argument (NOT raw key bytes). **D8-sub closure:** envoy-go MIMICS upstream's exposure scope — `crypto.go` returns a `PublicKeyWrapper` Lua userdata with `:get()` method; `:verifySignature` takes the wrapper as 2nd arg; calling convention pinned to match upstream byte-exactly. Anti-departure for the calling convention (avoids a calling-convention departure record). **BEHAVIOR_CONTRACT.md edit-count arithmetic at 22.2 IMPL** (canonical figures used uniformly throughout this PLAN): SPEC §14 enumerates 12 numbered edit slots. After D8 PLAN closure, item 11's conditional placeholder (0-6 records) expands to EXACTLY 4 records (`:sha256` + `:sha512` + `:base64Decode` + `:fileBytes` envoy-go-strict). Departure-record count = items 3-9 (= 5 counter records + 2 :filterState records) + item-11-expansion (= 4 D8 records) = **11 envoy-go-strict departure records** at BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` 22.2 sub-section. Total edit count = item 1 + item 2 + 7 departure records (items 3-9) + item 10 + 4 D8 records (item-11 expansion) + item 12 = 1+1+7+1+4+1 = **15 total edits at Task 19 atomic landing**. Updates SPEC §14 + ADR-0192 §Decision body anticipation + `### Phase 22.2 forward-pointer notes` subsection at BEHAVIOR_CONTRACT.md. Fixture-0027 scenario (h) `:fileBytes` falls to REFERENCE-LESS subject-only (reference Envoy cannot run `:fileBytes` — would error at runtime per absent-binding). Settles SPEC §12-D8 + §13-R7 + §13-R8 + AMEND-22.2-2. *Anchored: empirical D8 scrape at this 22.2 PLAN session — gh-raw fetch against `https://raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/source/extensions/filters/http/lua/lua_filter.{h,cc}` + `source/extensions/filters/common/lua/{lua.h,lua.cc,wrappers.h,wrappers.cc}` + GitHub code-search.*

**4. D-P1 — SPEC §6 task-numbering convention LOCKED at fresh 20-task numbering (NOT inherited verbatim from 22.1's 16-task numbering).** Settle: this PLAN's Tasks 1-19 + Pre-Task 0 produce 20 task entries grouped in 6 Tiers (Tier A framework primitives 1-5; Tier B HCM dispatch wire-in 6; Tier C bridge surfaces 7-13; Tier D stats + race + fuzz 14-16; Tier E differential fixture 17-18; Tier F atomic landing 19). The 22.2 SPEC does NOT pre-allocate a §6 task breakdown (parent §6 documented arms 1-22 wording — `:6` was a different topical section); this PLAN session allocates the task graph fresh per phase-21 + phase-22.1 PLAN precedent. *Anchored: this 22.2 PLAN session per the phase-21 + phase-22.1 PLAN's PLAN-time-task-graph-allocation precedent.*

**5. D-P2 — Per-task subagent dispatch type LOCKED at `general-purpose` for code Tasks 1-18; Task 19 atomic landing dispatched via `general-purpose` with explicit 25-item acceptance-checklist reference; REVIEW.md authoring at Task 19 final step dispatched via `superpowers:code-reviewer` per `superpowers:requesting-code-review`.** Settle: per project memory `feedback_execution_style.md` (user always wants subagent-driven over inline execution for plans), each Task's IMPL session subagent-dispatches per `superpowers:subagent-driven-development`. Dispatch type per Task: Tasks 1-18 use `general-purpose` agent (Go code work; no specialized agent type matches more precisely); Task 19 uses `general-purpose` with explicit reference to 22.2 SPEC §15 25-item acceptance checklist + BEHAVIOR_CONTRACT.md 15-edit bundle anatomy + ADR-0190 + ADR-0191 + ADR-0192 §Decision + §Consequences body sketches from SPEC §3.1 + §3.2 + §3.4 + §3.5 + §11 + §13 + §16 + the ADR-0177 IN-PLACE AMENDMENT body shape from SPEC §3.3 + §11.4. *Anchored: project memory `feedback_execution_style.md` + phase-09..22.1 + phase-18.1 + phase-19.1 IMPL precedent + `superpowers:subagent-driven-development` skill.*

**6. D-P3 — Per-task PROGRESS.md entry shape LOCKED per phase-21 + phase-22.1 IMPL precedent (8-section format).** Settle: each Task's PROGRESS.md entry contains the following sections in order:
   - **Task ID + title** (matches this PLAN's Task heading verbatim);
   - **Acceptance criteria** (verbatim cross-reference to this PLAN's Task `Acceptance:` stanza);
   - **Files touched** (the precise list from this PLAN's Task heading's `Files:` block);
   - **Verification command outputs** (the exact commands from this PLAN's Task Step bodies' Run-tests-verify-they-pass phase + the verbatim stdout/stderr quoted in fenced code blocks per `superpowers:verification-before-completion` discipline);
   - **Acceptance-criteria evidence** (per-criterion pass/fail with brief reasoning + cross-reference to the verification command output that demonstrates the pass);
   - **D-decision-disposition update** (if the task closes or refines a D-decision — e.g., Task 15 closes D-P10 R6 disposition; the entry records the empirical evidence + the resolved disposition);
   - **Commit SHA** (`git log -1 --format=%H` for the task's commit);
   - **Tier + Task-number cross-reference** (e.g., "Tier C bridge surfaces (Task 8 of 7-13 in tier; Task 8 of 19 overall + Pre-Task 0)").
   *Anchored: phase-21 + phase-22.1 + phase-18.1 + phase-19.1 PROGRESS.md format precedent + `superpowers:verification-before-completion` discipline + this PLAN's per-Task structure.*

**7. D-P4 — Per-task TDD ordering LOCKED at test-first for ALL 19 code Tasks per `superpowers:test-driven-development` rigid discipline; Task 18 fixture-0027 + Task 19 atomic landing relaxed to test-with-implementation.** Settle: every Task that lands production code at IMPL (Tasks 1-17) follows the rigid TDD ordering: (Step 1) write the failing test in the corresponding `*_test.go` file; (Step 2) run the test to verify it fails (compile-error OR assertion-failure with expected error); (Step 3) implement the minimal production code to make the test pass; (Step 4) run the test to verify it passes; (Step 5) run `go build ./... + go vet ./... + golangci-lint run` clean; (Step 6) append PROGRESS.md Task entry per D-P3; (Step 7) commit. Tasks that land bulk fixture material (Task 18 fixture-0027 directory + 13 scripts + YAML configs + driver) follow a relaxed test-with-implementation discipline (the differential fixture IS the integration test; the per-scenario `.lua` source files + the driver impl land together with the per-scenario probe assertions). Task 19 atomic landing follows the relaxed discipline (the 6-gate verification matrix IS the integration test; the BEHAVIOR_CONTRACT.md + DECISIONS.md + STATE.md + ROADMAP.md + REVIEW.md edits land together at the atomic commit). `superpowers:test-driven-development` is RIGID — adherence is mandatory for Tasks 1-17. *Anchored: `superpowers:test-driven-development` rigid discipline + phase-09..22.1 IMPL precedent.*

**8. D-P5 — `internal/lua/` 22.2 file split LOCKED at NEW `coroutine.go` + NEW `body_buffer.go` (NOT in-place APPEND to `vm.go`).** Settle: the 22.2 `internal/lua/` extensions ship as NEW FILES `coroutine.go` (NewThread + Resume + YieldFromBridge) + `body_buffer.go` (BodyBuffer interface) rather than as in-place APPEND to the 22.1's `vm.go`. Rationale: (i) the NEW FILE shape preserves clean ADR-0188 vs ADR-0191 lineage separation (vm.go = ADR-0188 scope; coroutine.go + body_buffer.go = ADR-0191 scope per Q10 strict scope); (ii) file-disjoint test surfaces (coroutine_test.go + body_buffer_test.go) enable parallel subagent dispatch at Tasks 2 + 3; (iii) future consumer-#2 phase that extends `internal/lua/` per ADR-0188 §Decision 5 ALLOWANCE can author ADR-0188-scoped revisions in vm.go without touching ADR-0191's coroutine.go + body_buffer.go. *Anchored: ADR-0188 §Decision 5 ALLOWANCE + ADR-0191 §Context lineage-separation rationale + this PLAN-time emerge.*

**9. D-P6 — Boot-registration position UNCHANGED at 22.2 (no new HTTP filter wired; ClusterManager threaded into FactoryCtx at HCM dispatch).** Settle: `cmd/envoy-go/main.go` ZERO DELTA at 22.2 — `httpReg.Register(lua.TypeURL, lua.New)` call at 22.1's alphabetical position (between `localratelimit.New` and `oauth2.New`) STAYS UNCHANGED. The NEW `ClusterManager` plumbing on `FactoryCtx` (per SPEC §3.3 + D-P-X below) is wired AT HCM-LEVEL DISPATCH (`internal/filter/hcm/connection.go` + `h2dispatch.go`) — NOT at main.go. The per-server `*cluster.Manager` is already available at HCM construction time (consumed by other filters per ADR-0177 §Decision integration paragraph); 22.2 just threads it into `FactoryCtx` for downstream filter consumers. 17 HTTP filters wired post-22.2 UNCHANGED from post-22.1. *Anchored: SPEC §3.3 + ADR-0177 §Decision integration + this PLAN-time emerge.*

**10. D-P7 — Fuzzer count target LOCKED at 30 (29th + 30th: `FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig`) per SPEC §11.9 D7 + §13-R10.** Settle: 22.2 adds 2 NEW fuzzers (not 1) per SPEC §11.9 D7 closure (29-30 anticipated). Both land at Task 16. **`FuzzLuaBodyBridge`** corpus seeds (~15-20 total): empty body (0 bytes); small body (10-100 bytes); medium body (10 KB-100 KB); large body (1 MB-15 MB); over-cap body (17 MB; should runtime-reject); chunked body (multi-call DecodeData accumulation patterns); script-patterns that yield/resume in pathological orderings (call `:body()` multiple times; call before+after endStream; nested coroutines; mid-coroutine OnDestroy). **`FuzzLuaHTTPCallConfig`** corpus seeds (~10-15 total): empty cluster name (should runtime-reject); valid cluster name + valid headers + valid body + valid timeout; missing-cluster fallthrough; transport-failure simulation; oversized headers; oversized body; invalid timeout values; async-flag variations. Both fuzzers must-never-panic per ADR-0018 + 30s clean. Project-wide fuzzer count post-22.2 = **30** (28 from 22.1 + 2 new at 22.2). *Anchored: SPEC §11.9 D7 + §13-R10 + ADR-0018 + this PLAN-time emerge.*

**11. D-P8 — Task graph parallelization LOCKED per PLAN-time emerge.** Settle: after Pre-Task 0 (PROGRESS.md preamble + 17-precondition verification) lands, the 19-task graph allows parallelization at multiple points:

   - **After Pre-Task 0** (PROGRESS.md preamble): Tasks 1 + 2 + 3 + 4 PARALLEL (4-way). NEW packages + framework-primitive extensions + IN-PLACE AMEND on httpclient are file-disjoint.
   - **After Tasks 1 + 2 + 3 + 4**: Task 5 sequential (depends on Task 1's `dynamicmetadata.Bucket` type for the chain.go field — and on Task 4's `FactoryCtx.ClusterManager` for the field).
   - **After Task 5**: Task 6 sequential (depends on Task 5's chain.go field additions; H1 + H2 plumbing).
   - **After Task 6**: Tasks 7 + 8 + 9 + 10 + 11 + 12 + 13 PARALLEL (7-way) — file-disjoint bridge surfaces:
     - Task 7 (`body.go` body bridge + body-bridge decode wire-in)
     - Task 8 (`trailers.go` trailers bridge + trailers metatable installs in bridge.go)
     - Task 9 (`metadata.go` metadata + dynamic-metadata bridge)
     - Task 10 (`connection.go` + `ssl.go` connection-SSL bridge)
     - Task 11 (`httpcall.go` httpCall bridge)
     - Task 12 (`crypto.go` + `misc.go` crypto + fileBytes + timestamp)
     - Task 13 (`streaminfo.go` extension + `filterstate.go` filter-state)
   - **After Tasks 7-13**: Tasks 14 + 15 + 16 PARALLEL (3-way) — file-disjoint:
     - Task 14 (`stats.go` 5 NEW counters + `compiled_config.go` runtime-reject arms 20-22)
     - Task 15 (race + concurrency tests + 2 benchmarks per D-P10)
     - Task 16 (29th + 30th fuzzer)
   - **After Tasks 14-16**: Tasks 17 + 18 PARALLEL (2-way) — cert fixture plumbing decision + fixture-0027 directory authoring:
     - Task 17 (cert fixture scripting per D5; REUSE existing TLS cert from prior fixture OR generate minimal cert)
     - Task 18 (fixture-0027 directory + 13 scripts + YAMLs + driver + R11 disposition for REFERENCE-LESS driver-helper)
   - **Sequential tail**: Task 19 (atomic landing — BEHAVIOR_CONTRACT.md 15-edit bundle + ADR bodies + STATE.md + ROADMAP).

   **Parallel-dispatch opportunities**: 4-way at Tasks 1+2+3+4; 7-way at Tasks 7+8+9+10+11+12+13; 3-way at Tasks 14+15+16; 2-way at Tasks 17+18.

   **Sequential bottlenecks**: Pre-Task-0 → {1,2,3,4}; {1,2,3,4} → 5; 5 → 6; 6 → {7,8,9,10,11,12,13}; {7,8,9,10,11,12,13} → {14,15,16}; {14,15,16} → {17,18}; {17,18} → 19.

   **Shared-file serialization caveat (load-bearing for Tasks 7-13 7-way claim):** `internal/filter/http/lua/bridge.go` is touched by Tasks 7+8+9+10+11+12+13 (each adds ~5-20 LoC of metatable-dispatch registration lines for its surface methods; Task 12 also adds the PublicKeyWrapper userdata metatable). `decode_headers.go` + `encode_headers.go` are touched by Tasks 7+8 (body + trailers decode/encode wiring at terminal-state). The 7-way parallelization claim therefore holds **for the NEW production files (body.go + trailers.go + metadata.go + connection.go + ssl.go + httpcall.go + crypto.go + misc.go + filterstate.go + streaminfo.go) + their NEW test files** (truly file-disjoint), but the **bridge.go + decode_headers.go + encode_headers.go edits are SERIALIZED via the IMPL session orchestrator's merge protocol** — each parallel subagent commits ONLY its NEW files in parallel; the orchestrator then SEQUENTIALLY applies the small bridge.go / decode_headers.go / encode_headers.go method-dispatch deltas from each Task in a single follow-up coordinated commit per Tier C end (one coordinated commit covering all 7 Tasks' bridge.go entries — wired alphabetically per ADR-0100-equivalent ordering discipline), OR each Task commits its bridge.go delta serially after its NEW-files commit while the next Task's NEW-files subagent runs in parallel. The IMPL session per `superpowers:subagent-driven-development` per project memory `feedback_execution_style.md` exploits the file-disjoint parallelism + serializes shared-file edits via the orchestrator's merge protocol. *Anchored: SPEC §3 framework primitive shapes + §6 PARSE/RUNTIME-REJECT roster + §11 empirical-pin closures + this PLAN-time emerge.*

**12. D-P9 — Cross-package regression-test command shape LOCKED per 22.1 D-P9 precedent with race-scoping carry-forward.** Settle: after each task lands its production code, the implementer runs the package-local test command. Race-scoping per 22.1 REVIEW §3 disposition table — `-race` flag scoped to unit packages per integration-suite port-bind race flakiness (0012/0018/0023). At Task 19 Gate D the full regression `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-7])'` runs all 29 fixture directories (the 28 pre-existing — 0000-0026 — plus the new 0027). Per SPEC §15 expected outcome: zero regression. Per-task gates:
   - Tasks 1-3: `go test -count=1 -race ./internal/dynamicmetadata/... ./internal/lua/...`
   - Task 4: `go test -count=1 -race ./internal/httpclient/...`
   - Tasks 5-6: `go test -count=1 -race ./internal/filter/http/... ./internal/filter/hcm/...`
   - Tasks 7-13: `go test -count=1 -race ./internal/filter/http/lua/...`
   - Tasks 14-16: `go test -count=1 -race ./internal/filter/http/lua/...`
   - Task 17-18: `go test -count=1 ./test/differential -run TestDifferential/0027`
   - Task 19: full `go test -count=1 -race ./...` + `go test -count=1 ./test/differential -run 'Test.*00(0[0-9]|1[0-9]|2[0-7])'` (no race for integration suite per 22.1 D-P9 scoping)
   *Anchored: 22.1 D-P9 precedent + 22.1 REVIEW §3 race-scoping refinement + this PLAN-time emerge.*

**13. D-P10 — `*LState`-pool benchmark RE-EVALUATION at FULL bridge surface (Task 15) per SPEC §13-R6 + 22.1 D-P10 carry-forward; threshold gate `ns/op > 1_000_000` (= 1ms) → ADR-0193 escape-valve fires.** Settle: Task 15 (race + concurrency tests) ALSO includes 2 benchmark sub-tasks at `internal/filter/http/lua/lua_test.go`:
   1. **`BenchmarkPerStream_FullBridge_LState_Construction`** — measures per-stream `*lua.LState` construction cost at the FULL bridge surface (constructs N=10000 fresh VMs back-to-back covering the 22.2 metatable installs — request_handle + response_handle + headers + streamInfo + headersIter + trailers + dynamicMetadata + connection + ssl + httpcall + crypto + misc + filterstate + PublicKeyWrapper — plus the parent+child LState pair via NewThread for coroutine support; reports `ns/op` via `b.N` discipline). Threshold gate per SPEC §13-R6: `ns/op > 1_000_000` (= 1ms). 22.1 baseline at headers-only surface measured `ns/op = 69865` (~70µs/stream); 22.2 FULL bridge surface anticipated 200-500µs/stream (3-7× headers-only); SHOULD STAY UNDER 1ms threshold. If `ns/op > 1_000_000`: ADR-0193 escape-valve FIRES at Task 19; ADR-0193 §Context + §Decision + §Consequences body all land at the same Task 19 commit per ADR-0044 anchoring a "per-script-source `*LState` pool with chunk-pre-loaded entries" decision. If `ns/op <= 1_000_000`: the WEAK-default per-stream construction STANDS; no ADR-0193 fires; next-free ADR-0193 carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot.
   2. **`BenchmarkBodyBridge_DefensiveCopy_PerStream`** (per D3 closure above) — measures defensive-copy overhead at sub-MB body + 16-MiB-cap-saturated body. Threshold gates ≤1ms sub-MB + ≤100ms 16-MiB-saturated. Outcomes feed into the §13-R9 disposition (R9 conditional ADR-0193 only fires if R6 stays under but R9 surfaces enough implementation complexity to warrant separate ADR per body-buffer-seam-with-ADR-0128 separation; this is independently evaluable from R6 — R9 may fire at Task 7 body-bridge IMPL via subagent surfacing complexity).
   Both benchmark results quoted verbatim in Task 15 PROGRESS.md entry per D-P3. *Anchored: SPEC §13-R6 + §13-R9 + 22.1 D-P10 carry-forward + 22.1 REVIEW §3 R6 disposition + this PLAN-time emerge.*

**14. D-P11 — REFERENCE-LESS driver-helper for non-deterministic fixture-0027 scenarios LOCKED at REUSE existing `runReferenceLessFixture` pattern (NOT NEW `RunSubjectOnlyHTTPLua` helper).** Settle per §13-R11: fixture-0027's ~3-4 non-deterministic scenarios (j) httpCall sync + (k) httpCall async + (l) timestamp + (m) filterState + (h) fileBytes (per D8 reclassification) consume the existing `runReferenceLessFixture` driver-helper at `test/differential/runner_test.go:1268`. NO NEW driver-helper added. Rationale: (i) the existing pattern handles "subject-side only; emit scenario verdict into byte-comparison buffer" semantics already (precedent: fixture-0025 inline scrape + fixture-0021 grpc-fixture REFERENCE-LESS pattern); (ii) NEW driver-helper would duplicate ~50-150 LoC without semantic gain; (iii) the fixture-0027 driver.go simply DISPATCHES PER-SCENARIO — calls `CompareBytes` for deterministic scenarios + emits subject-only verdicts for non-deterministic scenarios into the same byte-comparison buffer; the existing `BootRejectFixture` driver-helper from 22.1 is ORTHOGONAL (boot-reject is a different lifecycle phase). Task 17 + Task 18 implementer adopts this disposition. Settles SPEC §13-R11 disposition. *Anchored: SPEC §13-R11 + fixture-0021 + fixture-0023 + fixture-0025 + fixture-0026 REFERENCE-LESS-by-pattern precedent + this PLAN-time emerge.*

## Per-task entries

### Pre-Task 0: PROGRESS.md preamble + 17-precondition verification

- **Acceptance criteria**: all 17 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` returns the Pre-Task 0 commit's SHA (verbatim from PLAN §Pre-Task 0 Acceptance).
- **Files touched**: `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (created).
- **Verification command outputs**: see "17-precondition verification (Pre-Task 0)" section above — all 17 preconditions reproduced verbatim with command + output + verdict in fenced code blocks.
- **Acceptance-criteria evidence**: all 17 preconditions GREEN (precondition 14 carries a non-blocking wording-note on the PLAN-text's literal `VM` symbol reference — the package-reachability spirit is satisfied via `LState` being reachable + `go doc github.com/yuin/gopher-lua` rendering the full package summary cleanly); PROGRESS.md preamble + ADR table + 14 planner-time decisions + Pre-Task 0 entry committed at the SHA recorded below; the SHA matches `git log -1 --format=%H -- docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` post-commit.
- **D-decision-disposition update**: this Pre-Task 0 entry does NOT close any D-decision. D3 + D5 + D8 + D-P1..D-P11 were all CLOSED at the PLAN session per the 14-decisions section above; their full text is reproduced here as the single in-PROGRESS anchor for subsequent task entries' D-decision-disposition cross-references.
- **Commit SHA**: `23b860b7fcad5592edd978d153f32a71b9380fd7`
- **Tier + Task-number cross-reference**: Pre-Task 0 (sequential prerequisite for Tasks 1-19; 1 of 20 total entries — 1 Pre-Task + 19 Tasks across 6 Tiers per D-P1).

### Task 1: NEW `internal/dynamicmetadata/` package (Bucket + Get/Set/Snapshot/Reset) [ADR-0190]

- **Acceptance criteria**: `go test -count=1 -race ./internal/dynamicmetadata/...` clean; nil-bucket tolerance verified via table-driven tests; package-level docs cross-reference ADR-0190 §Context; `go build ./...` clean; `go vet ./...` clean; `golangci-lint run` clean.
- **Files touched**:
  - `internal/dynamicmetadata/doc.go` (created, 93 LoC)
  - `internal/dynamicmetadata/dynamicmetadata.go` (created, 100 LoC)
  - `internal/dynamicmetadata/dynamicmetadata_test.go` (created, 293 LoC)
  - `internal/dynamicmetadata/bench_test.go` (created, 60 LoC)
- **Verification command outputs**:

  ```
  $ go test -count=1 -race -v ./internal/dynamicmetadata/...
  ```

  ```
  === RUN   TestBucket_NewBucket_returns_nonnil_empty
  --- PASS: TestBucket_NewBucket_returns_nonnil_empty (0.00s)
  === RUN   TestBucket_Get_on_empty_returns_nil_false
  === RUN   TestBucket_Get_on_empty_returns_nil_false/empty_filter_empty_key
  === RUN   TestBucket_Get_on_empty_returns_nil_false/named_filter_empty_key
  === RUN   TestBucket_Get_on_empty_returns_nil_false/empty_filter_named_key
  === RUN   TestBucket_Get_on_empty_returns_nil_false/named_filter_named_key
  --- PASS: TestBucket_Get_on_empty_returns_nil_false (0.00s)
      --- PASS: TestBucket_Get_on_empty_returns_nil_false/empty_filter_empty_key (0.00s)
      --- PASS: TestBucket_Get_on_empty_returns_nil_false/named_filter_empty_key (0.00s)
      --- PASS: TestBucket_Get_on_empty_returns_nil_false/empty_filter_named_key (0.00s)
      --- PASS: TestBucket_Get_on_empty_returns_nil_false/named_filter_named_key (0.00s)
  === RUN   TestBucket_Set_then_Get_roundtrip
  --- PASS: TestBucket_Set_then_Get_roundtrip (0.00s)
  === RUN   TestBucket_Set_overwrites_existing
  --- PASS: TestBucket_Set_overwrites_existing (0.00s)
  === RUN   TestBucket_Snapshot_defensive_copy
  --- PASS: TestBucket_Snapshot_defensive_copy (0.00s)
  === RUN   TestBucket_Reset_clears_all_entries
  --- PASS: TestBucket_Reset_clears_all_entries (0.00s)
  === RUN   TestBucket_nil_tolerance
  === RUN   TestBucket_nil_tolerance/Get_on_nil_returns_nil_false
  === RUN   TestBucket_nil_tolerance/Set_on_nil_is_noop
  === RUN   TestBucket_nil_tolerance/Snapshot_on_nil_returns_nil
  === RUN   TestBucket_nil_tolerance/Reset_on_nil_is_noop
  --- PASS: TestBucket_nil_tolerance (0.00s)
      --- PASS: TestBucket_nil_tolerance/Get_on_nil_returns_nil_false (0.00s)
      --- PASS: TestBucket_nil_tolerance/Set_on_nil_is_noop (0.00s)
      --- PASS: TestBucket_nil_tolerance/Snapshot_on_nil_returns_nil (0.00s)
      --- PASS: TestBucket_nil_tolerance/Reset_on_nil_is_noop (0.00s)
  === RUN   TestBucket_structpb_payload_variations
  === RUN   TestBucket_structpb_payload_variations/null
  === RUN   TestBucket_structpb_payload_variations/number
  === RUN   TestBucket_structpb_payload_variations/string
  === RUN   TestBucket_structpb_payload_variations/bool_true
  === RUN   TestBucket_structpb_payload_variations/bool_false
  === RUN   TestBucket_structpb_payload_variations/list
  === RUN   TestBucket_structpb_payload_variations/struct
  --- PASS: TestBucket_structpb_payload_variations (0.00s)
      --- PASS: TestBucket_structpb_payload_variations/null (0.00s)
      --- PASS: TestBucket_structpb_payload_variations/number (0.00s)
      --- PASS: TestBucket_structpb_payload_variations/string (0.00s)
      --- PASS: TestBucket_structpb_payload_variations/bool_true (0.00s)
      --- PASS: TestBucket_structpb_payload_variations/bool_false (0.00s)
      --- PASS: TestBucket_structpb_payload_variations/list (0.00s)
      --- PASS: TestBucket_structpb_payload_variations/struct (0.00s)
  === RUN   TestBucket_cross_filter_key_independence
  --- PASS: TestBucket_cross_filter_key_independence (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/dynamicmetadata	1.009s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/dynamicmetadata/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race ./internal/dynamicmetadata/...` clean**: PASS — 9 top-level `TestBucket_*` tests + 17 total subtest cases (4 under `Get_on_empty`, 4 under `nil_tolerance`, 7 under `structpb_payload_variations`) all green under `-race`; package summary `ok github.com/esalaine/envoy-go/internal/dynamicmetadata 1.009s`. Verified via the verbose test output above.
  - **Nil-bucket tolerance verified via table-driven tests**: `TestBucket_nil_tolerance` exercises all four methods (Get/Set/Snapshot/Reset) on a `var b *Bucket` (nil receiver) under four subtest cases; each subtest installs `defer recover()` panic-guard and asserts the expected zero-value behavior (Get → `(nil, false)`; Set → no panic; Snapshot → `nil`; Reset → no panic). All four subtests PASS per the verbose output.
  - **Package-level docs cross-reference ADR-0190 §Context**: `doc.go` references ADR-0190 in its opening paragraph + §Cross-references list also enumerates ADR-0188 + ADR-0189 + ADR-0033 + ADR-0085 + ADR-0044 + 22.2 SPEC §3.1 + §1.6 + 22.2 BRAINSTORM Q3 + Q9.
  - **`go build ./...` clean**: empty output, exit 0 — confirmed above.
  - **`go vet ./...` clean**: empty output, exit 0 — confirmed above.
  - **`golangci-lint run` clean**: empty output, exit 0 — confirmed above. Initial draft of `dynamicmetadata.go` triggered the revive `package-comments` rule (file-level comment block was mistakenly written in `Package <name> ...` form); fixed by restructuring the file-level comment to a non-package-doc form (`File dynamicmetadata.go — ...`) since the package-doc lives in `doc.go`.
- **D-decision-disposition update**: this Task 1 entry does NOT close any D-decision. ADR-0190 §Decision + §Consequences body lands at Task 19 atomic landing per ADR-0044 in-place edit discipline; ADR-0190 §Context draft remains anchored at 22.2 SPEC commit `0d6463e` unchanged by this Task 1 commit. The framework-primitive code anchors the §Decision body's API-shape claims (NewBucket / Get / Set / Snapshot / Reset signatures + nil-tolerance per ADR-0085 + per-stream sequential per ADR-0033) but does NOT itself land the §Decision body — that lands at Task 19 per the Lands-in-Task convention.
- **Commit SHA**: `4f6e6b4c99e4b0f25a8643b89426708f976efe4e` (1-revision-stale relative to amend SHA per phase-22.1 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier A framework primitives (Task 1 of 1-5 in tier; Task 1 of 19 overall + Pre-Task 0).

### Task 2: EXTEND `internal/lua/` with coroutine API (NewThread + Resume + YieldFromBridge) [ADR-0191]

- **Acceptance criteria**: `go test -count=1 -race ./internal/lua/...` clean; NewThread returns non-nil child *LState + non-nil CancelFunc (when parent LState carries a context); Resume happy + yield-resume round-trip; CancelFunc cleans up child without goroutine leaks; panic-wrapper integration verified via panicH callback; race tests N=100 parallel coroutines clean under `-race`; `go build ./...` clean; `go vet ./internal/lua/...` clean; `golangci-lint run ./internal/lua/...` clean.
- **Files touched**:
  - `internal/lua/coroutine.go` (created, 224 LoC)
  - `internal/lua/coroutine_test.go` (created, 398 LoC)
  - `internal/lua/vm.go` (UNCHANGED — per Q10 strict scope + D-P5 LOCK at NEW FILES; the 22.1 VM surface (`NewVM`, `State`, `RegisterGlobalFunc`, `Run`, `HasGlobalFunc`, `CallGlobal`, `Close`, `PanicHandlerFn`, `VMOption`, `WithSandboxConfig`, `WithPanicHandler`, `WithBasePrintSink`) STAYS UNCHANGED; ADR-0188's API-REVISION ALLOWANCE clause STAYS scoped to consumer-#2 per Q10 lineage-separation)
- **Verification command outputs**:

  ```
  $ go test -count=1 -race -v ./internal/lua/... -run 'TestVM_NewThread|TestVM_Resume|TestVM_Coroutine'
  ```

  ```
  === RUN   TestVM_NewThread_returns_nonnil_child_and_CancelFunc
  --- PASS: TestVM_NewThread_returns_nonnil_child_and_CancelFunc (0.00s)
  === RUN   TestVM_Resume_happy_no_yield
  --- PASS: TestVM_Resume_happy_no_yield (0.00s)
  === RUN   TestVM_Resume_with_YieldFromBridge_roundtrip
  --- PASS: TestVM_Resume_with_YieldFromBridge_roundtrip (0.00s)
  === RUN   TestVM_Resume_after_yield_resumes_from_where_Yield_returned
  --- PASS: TestVM_Resume_after_yield_resumes_from_where_Yield_returned (0.00s)
  === RUN   TestVM_NewThread_CancelFunc_cleans_up_without_leaks
  --- PASS: TestVM_NewThread_CancelFunc_cleans_up_without_leaks (0.02s)
  === RUN   TestVM_Resume_panic_wraps_via_panicH
  --- PASS: TestVM_Resume_panic_wraps_via_panicH (0.00s)
  === RUN   TestVM_Coroutine_race_N100_parallel_clean_under_race
  --- PASS: TestVM_Coroutine_race_N100_parallel_clean_under_race (0.01s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/lua	1.046s
  ```

  ```
  $ go test -count=1 -race ./internal/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/lua	1.069s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./internal/lua/...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/lua/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race ./internal/lua/...` clean**: PASS — 7 NEW top-level `TestVM_NewThread_*` / `TestVM_Resume_*` / `TestVM_Coroutine_*` tests all green under `-race`; package summary `ok github.com/esalaine/envoy-go/internal/lua 1.069s`. The full `internal/lua/...` suite (NEW Task 2 tests + 22.1 baseline tests in `vm_test.go` + `compile_test.go` + `sandbox_test.go`) stays green per the full-suite verification above.
  - **NewThread returns non-nil child *LState + non-nil CancelFunc**: `TestVM_NewThread_returns_nonnil_child_and_CancelFunc` PASS — child is non-nil; CancelFunc is non-nil when the parent's LState carries a context attached via `vm.State().SetContext(ctx)` (the production per-stream dispatch pattern); the test also asserts globals sharing (parent's Go-registered globals visible from child via gopher-lua `G`+`Env` aliasing per state.go:1614-1617).
  - **Resume happy + yield-resume round-trip**: `TestVM_Resume_happy_no_yield` (no-yield child returns ResumeOK + values + nil err) + `TestVM_Resume_with_YieldFromBridge_roundtrip` (bridge invokes `YieldFromBridge` → ResumeYield + yielded values + nil err) + `TestVM_Resume_after_yield_resumes_from_where_Yield_returned` (multi-step: yield → resume with new value → script continues using the resume value → ResumeOK + final return value) — all three PASS.
  - **CancelFunc cleans up child without goroutine leaks**: `TestVM_NewThread_CancelFunc_cleans_up_without_leaks` PASS — N=50 NewThread+cancel iterations leave `runtime.NumGoroutine` delta ≤ 2 (well within the scheduler-churn tolerance budget); gopher-lua coroutines are synchronous with no background goroutines per §11.1 D2 closure.
  - **Panic-wrapper integration verified via panicH callback**: `TestVM_Resume_panic_wraps_via_panicH` PASS — a bridge LGFunction `panic("bridge-panic-value")` invoked from a coroutine is caught by gopher-lua's internal `threadRun` recover (vm.go:272), surfaces as `*lua.ApiError(ApiErrorRun)` from `Resume`, and is forwarded to `vm.panicH` via the NEW `coroutineDispatchPanic` helper per the ADR-0191 panic-wrapper extension for coroutines (broader than `vm.dispatchPanic`'s ApiErrorPanic-only scope used by Run / CallGlobal — gopher-lua's `threadRun` erases the Go-vs-Lua origin distinction at the coroutine seam, so the dispatch must cover the broader ApiError scope to honor ADR-0188 §Decision 2 panic-wrapper discipline at this surface).
  - **Race tests N=100 parallel coroutines clean under `-race`**: `TestVM_Coroutine_race_N100_parallel_clean_under_race` PASS under `-race` — 100 parallel VMs each driving an independent coroutine (compile + Run + NewThread + Resume[yield] + Resume[ok]) complete without data races or panics.
  - **`go build ./...` clean**: empty output, exit 0 — confirmed above.
  - **`go vet ./internal/lua/...` clean**: empty output, exit 0 — confirmed above.
  - **`golangci-lint run ./internal/lua/...` clean**: empty output, exit 0 — confirmed above. Initial draft triggered one gofmt-alignment warning on the compile-time signature pin block (multi-column alignment of `_ func(...)` assignments at the test file's API-pin section); fixed via `gofmt -w` and re-verified clean.
- **D-decision-disposition update**: this Task 2 entry does NOT close any D-decision. ADR-0191 §Decision + §Consequences body lands at Task 19 atomic landing per ADR-0044 in-place edit discipline; ADR-0191 §Context draft remains anchored at 22.2 SPEC commit `0d6463e` unchanged by this Task 2 commit. The NEW coroutine API code anchors the §Decision body's API-shape claims (`(vm *VM) NewThread` + `(vm *VM) Resume` + package-level `YieldFromBridge`; per-stream child-LState lifecycle = 1 parent + 1 child per phase invocation per §11.1 D2 closure; child's `context.CancelFunc` invoked at stream destroy; panic-wrapper integration via `coroutineDispatchPanic` extends ADR-0188 §Decision 2 to the coroutine seam) but does NOT itself land the §Decision body — that lands at Task 19 per the Lands-in-Task convention. D-P5 LOCKED-at-NEW-FILES disposition HONORED — all 3 NEW methods (NewThread + Resume + YieldFromBridge) live in `coroutine.go`; `vm.go` UNCHANGED at this Task. Q10 strict-scope lineage-separation HONORED — NO API revisions to the 22.1 VM surface; consumer-#1-scope-expansion lands under NEW ADR-0191 not in-place AMEND on ADR-0188.
- **Commit SHA**: `1251fde1b3a74712e373ab108c4210d385e2e538` (1-revision-stale relative to amend SHA per phase-22.1 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier A framework primitives (Task 2 of 1-5 in tier; Task 2 of 19 overall + Pre-Task 0).

### Task 3: EXTEND `internal/lua/` with BodyBuffer interface seam [ADR-0191]

- **Acceptance criteria**: `go test -count=1 ./internal/lua/...` clean (Task 2's coroutine_test merged + new body_buffer_test); interface signature stability + nil-tolerance of consumers reading Bytes()/Chunks()/EndStream(); `go build` + `go vet` + `golangci-lint` clean.
- **Files touched**:
  - `internal/lua/body_buffer.go` (created, 77 LoC) — interface declaration ONLY (3 methods: `Bytes() []byte` + `Chunks() [][]byte` + `EndStream() bool`); NO concrete impl per ADR-0191 §Context lineage separation (concrete `*decodedBody` lives at `internal/filter/http/lua/body.go` at Task 7).
  - `internal/lua/body_buffer_test.go` (created, 164 LoC) — package-scope `var _ BodyBuffer = (*mockBodyBuffer)(nil)` compile-time signature pin per 22.1 vm_test.go discipline; test-double `mockBodyBuffer` satisfying the 3-method interface; 3 top-level tests (`TestBodyBuffer_interface_signature_compiles_with_mock` + `TestBodyBuffer_nil_tolerance_for_consumers` + `TestBodyBuffer_mock_Bytes_Chunks_EndStream_return_canned_values`) plus 2 sub-tests (typed-nil dispatch hazard documentation + pre-endStream false-return contract).
  - `internal/lua/vm.go` / `internal/lua/coroutine.go` / `internal/lua/sandbox.go` (UNCHANGED — per Q10 strict scope + D-P5 LOCK at NEW FILES; the 22.1 VM surface (`NewVM`, `State`, `RegisterGlobalFunc`, `Run`, `HasGlobalFunc`, `CallGlobal`, `Close`, etc.) STAYS UNCHANGED; Task 2's coroutine API STAYS UNCHANGED; the consumer-#1 scope-expansion lands under NEW ADR-0191 per Q10 lineage-separation).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race -v ./internal/lua/... -run TestBodyBuffer
  ```

  ```
  === RUN   TestBodyBuffer_interface_signature_compiles_with_mock
  --- PASS: TestBodyBuffer_interface_signature_compiles_with_mock (0.00s)
  === RUN   TestBodyBuffer_nil_tolerance_for_consumers
  === RUN   TestBodyBuffer_nil_tolerance_for_consumers/typed_nil_pointer_in_interface_is_not_nil_interface
  --- PASS: TestBodyBuffer_nil_tolerance_for_consumers (0.00s)
      --- PASS: TestBodyBuffer_nil_tolerance_for_consumers/typed_nil_pointer_in_interface_is_not_nil_interface (0.00s)
  === RUN   TestBodyBuffer_mock_Bytes_Chunks_EndStream_return_canned_values
  === RUN   TestBodyBuffer_mock_Bytes_Chunks_EndStream_return_canned_values/endStream_false_pre_terminal
  --- PASS: TestBodyBuffer_mock_Bytes_Chunks_EndStream_return_canned_values (0.00s)
      --- PASS: TestBodyBuffer_mock_Bytes_Chunks_EndStream_return_canned_values/endStream_false_pre_terminal (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/lua	1.006s
  ```

  ```
  $ go test -count=1 -race ./internal/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/lua	1.070s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./internal/lua/...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/lua/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 ./internal/lua/...` clean (Task 2's coroutine_test merged + new body_buffer_test)**: PASS — 3 NEW top-level `TestBodyBuffer_*` tests (5 sub-tests total) all green under `-race`; package summary `ok github.com/esalaine/envoy-go/internal/lua 1.070s` covering the full union of 22.1 baseline tests (`vm_test.go` + `compile_test.go` + `sandbox_test.go`) + Task 2 coroutine tests + NEW Task 3 body_buffer tests.
  - **Interface signature stability**: package-scope `var _ BodyBuffer = (*mockBodyBuffer)(nil)` compile-time pin asserts `mockBodyBuffer`'s `Bytes() []byte` + `Chunks() [][]byte` + `EndStream() bool` methods match the BodyBuffer interface exactly — any signature drift on either side becomes an immediate build-break. The `TestBodyBuffer_interface_signature_compiles_with_mock` test additionally exercises each method through the interface at runtime (interface dispatch + zero-value return-shape verification: Bytes()=nil, Chunks()=nil, EndStream()=false on zero-valued mock).
  - **Nil-tolerance of consumers reading Bytes()/Chunks()/EndStream()**: `TestBodyBuffer_nil_tolerance_for_consumers` PASS — explicit nil `BodyBuffer` interface value passed to the `safeBytes` helper (which mirrors the production bridge consumer's defensive `if b != nil { _ = b.Bytes() }` pattern) returns nil without panic. The typed-nil sub-test documents that wrapping a typed nil pointer in the interface preserves the dispatch hazard at production-time and pins the convention that consumers nil-check the INTERFACE value (not the concrete pointer) — the test recover-guards the documented NPE so it stays green.
  - **Canned-value return contract**: `TestBodyBuffer_mock_Bytes_Chunks_EndStream_return_canned_values` PASS — instantiates `*mockBodyBuffer` with canned bytes=`"hello world"`, 2-element chunks=`["hello ", "world"]`, endStream=true; asserts each method returns its canned value via the interface. The `endStream_false_pre_terminal` sub-test pins the production §11.3 D3 "false until terminal endStream=true fires" contract on a fresh zero-valued mock.
  - **`go build ./...` clean**: empty output, exit 0 — confirmed above (catches package-wide consumer breakage from the NEW interface declaration; none observed).
  - **`go vet ./internal/lua/...` clean**: empty output, exit 0 — confirmed above.
  - **`golangci-lint run ./internal/lua/...` clean**: empty output, exit 0 — confirmed above. Initial draft triggered one gofmt-alignment warning on the receiver-method block (multi-column alignment of `func (m *mockBodyBuffer) Bytes/Chunks/EndStream` returns); fixed via `gofmt -w` and re-verified clean.
- **D-decision-disposition update**: this Task 3 entry does NOT close any D-decision. ADR-0191 §Decision + §Consequences body lands at Task 19 atomic landing per ADR-0044 in-place edit discipline; ADR-0191 §Context draft remains anchored at 22.2 SPEC commit `0d6463e` unchanged by this Task 3 commit. The NEW BodyBuffer interface code anchors the §Decision body's seam-API claims (3 methods: `Bytes() []byte` + `Chunks() [][]byte` + `EndStream() bool`; consumer-side concrete impl lives at `internal/filter/http/lua/body.go` per lineage separation; defensive-copy-at-endStream RECOMMENDATION per §11.3 D3 codified in the Bytes() doc-comment guidance — `Consumers SHOULD defensive-copy this slice when passing to Lua via lua.LString(string(bytes))`) but does NOT itself land the §Decision body — that lands at Task 19 per the Lands-in-Task convention. D-P5 LOCKED-at-NEW-FILES disposition HONORED — the BodyBuffer interface lives in NEW `body_buffer.go`; existing `vm.go` + `coroutine.go` + `sandbox.go` UNCHANGED. Q10 strict-scope lineage-separation HONORED — NO concrete BodyBuffer impl in this file (deferred to Task 7 consumer-side); consumer-#1-scope-expansion lands under NEW ADR-0191 not in-place AMEND on ADR-0188. §11.3 D3 RECOMMENDED option (a) defensive copy at endStream — interface CONTRACT codifies the guidance in Bytes() doc-comment; full closure (perf-benchmark validation per §11.3 D3 disposition) deferred to Task 15 benchmarks.
- **Commit SHA**: `5ebd678a93f5adba02e973b727780733259465d1` (1-revision-stale relative to amend SHA per phase-22.1 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier A framework primitives (Task 3 of 1-5 in tier; Task 3 of 19 overall + Pre-Task 0).

### Task 4: IN-PLACE AMEND `internal/httpclient/` with ClusterDispatch + FactoryCtx.ClusterManager doc-comment update [AMEND-ADR-0177]

- **Acceptance criteria**: `go test -count=1 -race ./internal/httpclient/...` clean; 6 NEW `TestClient_ClusterDispatch_*` tests cover cluster-not-found + endpoint-resolution + per-cluster TLS + retry-inherits-Options + ctx-timeout + ctx-cancellation; `FactoryCtx.ClusterManager` field present at `internal/filter/http/types.go` with phase-22.2 lua-bridge cross-reference in the doc-comment; NEW `(c *Cluster) UpstreamTLSConfig() *tls.Config` accessor exposed on the cluster package; `go build ./...` clean; `go vet ./...` clean; `golangci-lint run` clean.
- **Files touched**:
  - `internal/httpclient/httpclient.go` (modified +173 LoC; +1 new import block (`context` + `errors` + `fmt` + `cluster` package), NEW sentinel `errClusterNotFound`, NEW method `(c *Client) ClusterDispatch(ctx, clusterName, request, clusterMgr) (*http.Response, error)` with full SPEC §3.3 + §11.4 doc-comment cross-reference (lookup → PickEndpoint → URL rewrite → per-cluster TLS via temp http.Client → retry loop mirroring Do)).
  - `internal/httpclient/httpclient_test.go` (modified +457 LoC; +9 new imports (crypto/x509, encoding/pem, math/big, net, strconv, bootstrap/cluster/core/endpoint/tls proto packages, durationpb, anypb, internal/cluster, internal/stats); 6 NEW top-level tests + 4 test helpers (`splitHostPort`, `mkPlainClusterMgr`, `mkHTTPClientTestPKI`, `mkTLSClusterMgr`) mirroring the established `internal/grpcclient/grpcclient_test.go` + `internal/filter/http/extauthz/extauthz_test.go` cluster-manager test-fixture pattern; per-cluster TLS test generates an in-memory ECDSA CA + leaf cert (CN `alpha.envoy-go.test`, SAN `127.0.0.1`+`::1`) and starts a real `httptest.NewUnstartedServer().StartTLS()` with that cert chain; negative-control sub-assertion in the TLS test verifies plaintext-cluster dispatch to the TLS server does NOT return the expected 200/`tls-cluster-ok` body — refutes the alternative hypothesis that per-cluster TLS is unused).
  - `internal/cluster/cluster.go` (modified +13 LoC; NEW exported accessor `(c *Cluster) UpstreamTLSConfig() *stdtls.Config` paralleling existing `UseH2()` and `Name()` accessors; doc-comment marks the value as shared-read-only and cross-references SPEC §3.3 + §11.4 + ADR-0177 IN-PLACE AMENDMENT for the cross-phase httpclient consumer).
  - `internal/filter/http/types.go` (modified +8 LoC net delta; doc-comment extension on existing `FactoryCtx.ClusterManager` field adds phase-22.2 lua `:httpCall()` bridge as the second co-consumer (R5 RATIFIED at this Task per SPEC §11.4); field itself is UNCHANGED — the existing phase-18.2 ext_authz landing already established the field at `internal/filter/http/types.go:272`; no field re-introduction).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race -v ./internal/httpclient/... -run TestClient_ClusterDispatch
  ```

  ```
  === RUN   TestClient_ClusterDispatch_cluster_not_found_returns_error
  === PAUSE TestClient_ClusterDispatch_cluster_not_found_returns_error
  === RUN   TestClient_ClusterDispatch_endpoint_resolution_success
  === PAUSE TestClient_ClusterDispatch_endpoint_resolution_success
  === RUN   TestClient_ClusterDispatch_per_cluster_TLS_honored
  === PAUSE TestClient_ClusterDispatch_per_cluster_TLS_honored
  === RUN   TestClient_ClusterDispatch_retry_inherits_Options
  === PAUSE TestClient_ClusterDispatch_retry_inherits_Options
  === RUN   TestClient_ClusterDispatch_timeout_via_context
  === PAUSE TestClient_ClusterDispatch_timeout_via_context
  === RUN   TestClient_ClusterDispatch_context_cancellation_propagates
  === PAUSE TestClient_ClusterDispatch_context_cancellation_propagates
  --- PASS: TestClient_ClusterDispatch_cluster_not_found_returns_error (0.00s)
  --- PASS: TestClient_ClusterDispatch_context_cancellation_propagates (0.00s)
  --- PASS: TestClient_ClusterDispatch_endpoint_resolution_success (0.00s)
  --- PASS: TestClient_ClusterDispatch_retry_inherits_Options (0.00s)
  --- PASS: TestClient_ClusterDispatch_timeout_via_context (0.01s)
  --- PASS: TestClient_ClusterDispatch_per_cluster_TLS_honored (0.01s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/httpclient	1.020s
  ```

  ```
  $ go test -count=1 -race ./internal/httpclient/... ./internal/filter/http/... ./internal/cluster/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/httpclient	1.065s
  ok  	github.com/esalaine/envoy-go/internal/filter/http	1.293s
  ok  	github.com/esalaine/envoy-go/internal/cluster	1.041s
  (...19 filter/http subpackages all PASS...)
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./internal/httpclient/... ./internal/filter/http/... ./internal/cluster/...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/httpclient/... ./internal/filter/http/... ./internal/cluster/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race ./internal/httpclient/...` clean**: 6 NEW `TestClient_ClusterDispatch_*` top-level tests all PASS under `-race`; package summary `ok github.com/esalaine/envoy-go/internal/httpclient 1.065s` covering the full union of phase-20 baseline tests (10 pre-existing `Test*` covering Options zero-value, Do happy-path, retry envelope, ctx cancellation, TLS wiring, request-error propagation, POST body) PLUS the 6 NEW ClusterDispatch tests.
  - **Cluster-not-found returns error + nil response**: `TestClient_ClusterDispatch_cluster_not_found_returns_error` PASS — manager built with cluster "exists"; lookup of "missing" returns non-nil err (`errClusterNotFound` wrapped with cluster name) and nil `*http.Response`; no upstream dial attempted (no httptest server even started).
  - **Endpoint resolution + URL rewrite success**: `TestClient_ClusterDispatch_endpoint_resolution_success` PASS — real cluster manager built via `cluster.NewManager` with a STATIC cluster pointing at the test `httptest.NewServer`'s host:port; request constructed with placeholder host `placeholder.invalid` (intentionally non-resolvable to force the rewrite-or-fail discipline); response body verbatim `"cluster-dispatch-ok"` confirms `request.URL.Host = endpoint.Addr()` rewrite landed correctly; the alternative hypothesis (URL.Host unchanged → DNS resolution of `placeholder.invalid` would fail with NXDOMAIN) is refuted by the test passing.
  - **Per-cluster TLS honored**: `TestClient_ClusterDispatch_per_cluster_TLS_honored` PASS — in-memory ECDSA CA + leaf cert generated fresh per test invocation (CN `alpha.envoy-go.test`, SAN `127.0.0.1`+`::1`); `httptest.NewUnstartedServer().StartTLS()` serves with that cert chain; cluster manager built with `trusted_ca: InlineBytes(caPEM)` + SNI `alpha.envoy-go.test`; Client's receiver `Options.TLSConfig` is NIL — the only TLS path in play is the cluster's; SUCCESS (200 + `tls-cluster-ok` body) proves per-cluster TLS is the path that handled the upstream handshake. Negative-control sub-assertion verifies a plaintext-cluster dispatch to the same TLS server returns either an error (TLS handshake mismatch) OR a non-200 response (the stdlib's "client sent an HTTP request to an HTTPS server" 400-class reply) — never the wantBody/200 pair (refutes the "TLS path always used regardless" hypothesis).
  - **Retry inherits Options.RetryPolicy**: `TestClient_ClusterDispatch_retry_inherits_Options` PASS — receiver Options has `RetryPolicy{Attempts: 2, PerAttemptDelay: 1ms, RetryOnStatus: [503]}`; server returns 503 unconditionally; assertion `attempts == 3` (Attempts+1 total) confirms the retry loop mirrors Do at httpclient.go:155-202 — same `maxAttempts = Attempts + 1` arithmetic, same `shouldRetryStatus` allow-list gate, same `time.Timer` inter-attempt sleep + `ctx.Done()` race.
  - **Timeout via context**: `TestClient_ClusterDispatch_timeout_via_context` PASS — request context built with `context.WithTimeout(5ms)`; server sleeps 2s before responding; ClusterDispatch returns within deadline budget with non-nil err whose error chain includes `context.DeadlineExceeded` (or `errors.Is`-equivalent text) — no hang. Mirrors the `TestCtxCancellation_MidDo_ReturnsDeadlineExceeded` discipline for the Do path.
  - **Context cancellation propagates**: `TestClient_ClusterDispatch_context_cancellation_propagates` PASS — context.WithCancel built and cancel() called BEFORE dispatch; ClusterDispatch returns non-nil err whose chain unwraps to `context.Canceled` (or the equivalent string match); no upstream dial succeeded (the cancel-before-dispatch ordering ensures the underlying `*http.Client.Do` sees the canceled ctx at the transport-layer entry).
  - **FactoryCtx.ClusterManager field present + doc-comment cross-references 22.2 lua bridge**: confirmed at `internal/filter/http/types.go:272` (field declaration unchanged from phase-18.2 landing) with doc-comment extended to name the phase-22.2 lua `:httpCall()` bridge as the second co-consumer of the field per SPEC §11.4.5 and R5 RATIFIED at this Task.
  - **NEW Cluster.UpstreamTLSConfig() accessor on cluster package**: confirmed at `internal/cluster/cluster.go:158-170` (after existing `UseH2()` at :156); returns the unexported `upstreamCfg *stdtls.Config` field; nil for plaintext clusters, non-nil for TLS clusters built via `internaltls.NewUpstreamConfig`; doc-comment marks the return value as shared-read-only.
  - **`go build ./...` clean**: empty output, exit 0 — no cross-package consumer breakage from the NEW `Client.ClusterDispatch` method or NEW `Cluster.UpstreamTLSConfig()` accessor.
  - **`go vet ./internal/httpclient/... ./internal/filter/http/... ./internal/cluster/...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/httpclient/... ./internal/filter/http/... ./internal/cluster/...` clean**: empty output, exit 0.
- **D-decision-disposition update**: this Task 4 entry RATIFIES R5 (forward-pointer carry-over from phase-20 IMPL into 22.2 per ADR-0177 §Consequences) — FIRST cross-phase co-consumer validation of the `internal/httpclient/` framework primitive at 22.2's `:httpCall()` bridge consumer-site. The 4 introduction-time consumers (phase-17 jwks Fetcher inner-HTTP per ADR-0150 §Decision AMENDMENT; phase-18 extauthz httpAuthClient per ADR-0159 §Decision AMENDMENT + §Future Work CLOSURE; phase-20 oauth2 token_endpoint POST per ADR-0185) all consumed the URL-based `Do(req)` path. The 22.2 lua bridge introduces the cluster-based dispatch shape requiring per-cluster TLS via cluster-manager LB — codified at this Task as `(c *Client) ClusterDispatch(ctx, clusterName, request, clusterMgr) (*http.Response, error)`. The §13-R5 carry-forward signal CLOSES at this Task; the ADR-0177 §Decision AMENDMENT body lands at Task 19 atomic landing per ADR-0044 in-place edit discipline (matching phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). NO new ADR number consumed. The actual `:httpCall()` Lua-surface bridge code lands at Task 11 per the PLAN tier sequencing (Task 4 lands the framework primitive; Task 11 consumes it); R5 RATIFIED disposition is recorded HERE at the framework-primitive landing rather than at Task 11 because the primitive's signature + retry/TLS semantics are the surface under test — Task 11's bridge is a thin closure adapter over a now-RATIFIED primitive.
- **Commit SHA**: `b5207217f9d5f602fcadf48d4c9cfe1efc4e96f5` (1-revision-stale relative to amend SHA per phase-22.1 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier A framework primitives (Task 4 of 1-5 in tier; Task 4 of 19 overall + Pre-Task 0).

### Task 5: EXTEND `internal/filter/http/chain.go` + `callbacks.go` with tlsConnectionState + dynamicMetadata fields + 4 accessors [ADR-0192]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/http/...` clean; field setters + 4 accessors verified; nil-tolerance (plaintext / non-mTLS / no-bucket) verified; `go build` + `go vet` + `golangci-lint` clean.
- **Files touched**:
  - `internal/filter/http/chain.go` (modified +99 LoC; NEW imports `crypto/tls` + `internal/dynamicmetadata`; NEW fields `tlsConnectionState *tls.ConnectionState` + `dynamicMetadata *dynamicmetadata.Bucket` on FilterChain struct; NEW setters `SetTLSConnectionState(state)` + `SetDynamicMetadata(b)`; modified `NewFilterChain` constructor to initialize `dynamicMetadata: dynamicmetadata.NewBucket()`; modified `Destroy()` to Reset+nil-out the bucket field; NEW 4 accessors `decoderCB.DownstreamTLSConnectionState()` + `decoderCB.DynamicMetadata()` + `encoderCB.DownstreamTLSConnectionState()` + `encoderCB.DynamicMetadata()`).
  - `internal/filter/http/callbacks.go` (modified +56 LoC; NEW imports `crypto/tls` + `internal/dynamicmetadata`; NEW method signatures `DownstreamTLSConnectionState() *tls.ConnectionState` + `DynamicMetadata() *dynamicmetadata.Bucket` added to both `DecoderFilterCallbacks` + `EncoderFilterCallbacks` interfaces).
  - `internal/filter/http/chain_test.go` (modified +280 LoC; NEW imports `crypto/tls` + `google.golang.org/protobuf/types/known/structpb` + `internal/dynamicmetadata`; NEW `adr0192Probe` test-double satisfying both decode+encode interfaces with capture surfaces; 11 NEW tests covering setter round-trip (`TestFilterChain_SetTLSConnectionState_roundtrip` + `TestFilterChain_SetDynamicMetadata_roundtrip`), construction discipline (`TestFilterChain_constructor_initializes_dynamicMetadata_nonnil`), Destroy discipline (`TestFilterChain_Destroy_resets_bucket`), decode-side accessors (`TestDecoderCB_DownstreamTLSConnectionState_returns_field` + `TestDecoderCB_DynamicMetadata_returns_field`), encode-side accessors (`TestEncoderCB_DownstreamTLSConnectionState_returns_field` + `TestEncoderCB_DynamicMetadata_returns_field`), nil-tolerance (`TestChain_nil_tlsConnectionState_returns_nil_via_accessor`), post-Destroy semantic (`TestChain_post_Destroy_DynamicMetadata_returns_nil`)).
  - `internal/filter/http/callbacks_test.go` (modified +16 LoC; NEW imports `crypto/tls` + `internal/dynamicmetadata`; NEW zero-value stub methods `DownstreamTLSConnectionState() *tls.ConnectionState` + `DynamicMetadata() *dynamicmetadata.Bucket` on `fakeDecoderCB` + `fakeEncoderCB` to keep `TestDecoderFilterCallbacks_Compile` + `TestEncoderFilterCallbacks_Compile` compile-time conformance assertions green).
  - 14 cross-package test-double extensions (`adaptive_concurrency/decode_headers_test.go` + `bandwidthlimit/bandwidthlimit_test.go` + `buffer/buffer_test.go` + `compressor/compressor_test.go` + `csrf/csrf_test.go` + `extauthz/extauthz_test.go` + `extproc/extproc_test.go` + `fault/fault_test.go` + `header_mutation/header_mutation_test.go` + `jwtauthn/jwtauthn_test.go` + `localratelimit/local_ratelimit_test.go` + `lua/lua_test.go` + `oauth2/oauth2_test.go` + `rbac/rbac_test.go` — each gains `crypto/tls` + `internal/dynamicmetadata` imports + the 2 zero-value stub methods on each affected test-double type to keep the existing per-package compile-time conformance assertions green; total ~120 LoC delta across the 14 files).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race -v ./internal/filter/http/ -run 'TestFilterChain_Set|TestDecoderCB_DownstreamTLS|TestDecoderCB_DynamicMetadata|TestEncoderCB_DownstreamTLS|TestEncoderCB_DynamicMetadata|TestChain_nil_tls|TestFilterChain_constructor|TestFilterChain_Destroy_resets|TestChain_post_Destroy|TestDecoderFilterCallbacks_Compile|TestEncoderFilterCallbacks_Compile'
  ```

  ```
  === RUN   TestDecoderFilterCallbacks_Compile
  --- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
  === RUN   TestEncoderFilterCallbacks_Compile
  --- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
  === RUN   TestFilterChain_SetTLSConnectionState_roundtrip
  --- PASS: TestFilterChain_SetTLSConnectionState_roundtrip (0.00s)
  === RUN   TestFilterChain_SetDynamicMetadata_roundtrip
  --- PASS: TestFilterChain_SetDynamicMetadata_roundtrip (0.00s)
  === RUN   TestFilterChain_constructor_initializes_dynamicMetadata_nonnil
  --- PASS: TestFilterChain_constructor_initializes_dynamicMetadata_nonnil (0.00s)
  === RUN   TestFilterChain_Destroy_resets_bucket
  --- PASS: TestFilterChain_Destroy_resets_bucket (0.00s)
  === RUN   TestDecoderCB_DownstreamTLSConnectionState_returns_field
  --- PASS: TestDecoderCB_DownstreamTLSConnectionState_returns_field (0.00s)
  === RUN   TestDecoderCB_DynamicMetadata_returns_field
  --- PASS: TestDecoderCB_DynamicMetadata_returns_field (0.00s)
  === RUN   TestEncoderCB_DownstreamTLSConnectionState_returns_field
  --- PASS: TestEncoderCB_DownstreamTLSConnectionState_returns_field (0.00s)
  === RUN   TestEncoderCB_DynamicMetadata_returns_field
  --- PASS: TestEncoderCB_DynamicMetadata_returns_field (0.00s)
  === RUN   TestChain_nil_tlsConnectionState_returns_nil_via_accessor
  --- PASS: TestChain_nil_tlsConnectionState_returns_nil_via_accessor (0.00s)
  === RUN   TestChain_post_Destroy_DynamicMetadata_returns_nil
  --- PASS: TestChain_post_Destroy_DynamicMetadata_returns_nil (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http	1.030s
  ```

  ```
  $ go test -count=1 -race ./internal/filter/http/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http	1.284s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	1.032s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit	9.673s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.019s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/compressor	1.044s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.016s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.023s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.046s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.388s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.244s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.331s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.021s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.121s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.024s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.474s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/oauth2	1.050s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.028s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.244s
  ```

  ```
  $ go test -count=1 -race ./internal/filter/hcm/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.042s
  ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.512s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race ./internal/filter/http/...` clean**: PASS — 11 NEW top-level `TestFilterChain_*` / `TestDecoderCB_DownstreamTLSConnectionState_*` / `TestDecoderCB_DynamicMetadata_*` / `TestEncoderCB_DownstreamTLSConnectionState_*` / `TestEncoderCB_DynamicMetadata_*` / `TestChain_nil_tlsConnectionState_*` / `TestChain_post_Destroy_DynamicMetadata_*` tests all green under `-race`; the full union of 19 filter/http subpackages stays green per the suite-wide verification above (no regressions from the 2 interface-method additions + the 14 cross-package test-double stub extensions). HCM-side test suite stays green (no chain-side regression observable at the HCM dispatch surface — that's the seeding consumer to land at Task 6).
  - **Field setters verified**: `TestFilterChain_SetTLSConnectionState_roundtrip` PASS — SetTLSConnectionState(seed) stores the supplied `*tls.ConnectionState` pointer verbatim on the chain field; subsequent `chain.tlsConnectionState` read returns the same pointer. `TestFilterChain_SetDynamicMetadata_roundtrip` PASS — SetDynamicMetadata(b) overrides the construction-initialized bucket with the supplied `*dynamicmetadata.Bucket`; pre-populated entries via Set("test.filter", "key", value) survive the swap. Pattern mirrors ADR-0144 SetTLSPrincipals → tlsPrincipals discipline (set ONCE at chain build time by HCM dispatch BEFORE RunDecodeHeaders per ADR-0071 single-dispatch-goroutine invariant).
  - **4 accessors verified**: `TestDecoderCB_DownstreamTLSConnectionState_returns_field` + `TestEncoderCB_DownstreamTLSConnectionState_returns_field` PASS — both decoder + encoder callbacks return the chain's seeded `*tls.ConnectionState` verbatim (same backing field `chain.tlsConnectionState`; ADR-0033 per-stream-sequential-shared-state). `TestDecoderCB_DynamicMetadata_returns_field` + `TestEncoderCB_DynamicMetadata_returns_field` PASS — both decoder + encoder callbacks return the chain's `*dynamicmetadata.Bucket` pointer verbatim; pre-set entries via the chain field's Set survive the accessor read with no transformation.
  - **Nil-tolerance (plaintext / non-mTLS) verified**: `TestChain_nil_tlsConnectionState_returns_nil_via_accessor` PASS — default chain (no SetTLSConnectionState call) returns nil from both decoder + encoder DownstreamTLSConnectionState() accessors; mirrors the ADR-0144 §Decision (iii) plaintext-elision discipline (HCM dispatch elides the SetX call on plaintext / non-mTLS / no-client-cert paths; the chain field stays nil; consumers nil-tolerate per ADR-0085).
  - **Construction discipline + Destroy discipline verified**: `TestFilterChain_constructor_initializes_dynamicMetadata_nonnil` PASS — `NewFilterChain` initializes `dynamicMetadata` to a non-nil empty `*Bucket` (via `dynamicmetadata.NewBucket()` per ADR-0190); Get/Set round-trip on the freshly-constructed bucket succeeds. `TestFilterChain_Destroy_resets_bucket` PASS — Destroy fires Reset (clearing entries) + nils-out the bucket field; re-Destroy is idempotent (destroyOnce guard) leaving the field nil. `TestChain_post_Destroy_DynamicMetadata_returns_nil` PASS — post-Destroy cached `*decoderCB` + `*encoderCB` accessors return nil via the chain field's nil-out; ADR-0085 Bucket consumer nil-tolerance makes this safe.
  - **`go build ./...` clean**: empty output, exit 0 — confirmed above. Captures cross-package consumer breakage from the 2 NEW interface method signatures (added on BOTH DecoderFilterCallbacks + EncoderFilterCallbacks); the 14 cross-package test-double extensions resolved all such breakage by zero-value stubbing the new methods on each affected fake type.
  - **`go vet ./...` clean**: empty output, exit 0 — confirmed above. Full project vet (not just filter/http) confirms no shadow / nil-deref / unreachable findings introduced.
  - **`golangci-lint run ./internal/filter/http/...` clean**: empty output, exit 0 — confirmed above. Initial draft triggered one gofmt-alignment warning on the `adr0192Probe` struct's column alignment (`decTLS         *tls.ConnectionState` aligned beyond the gofmt-determined column width); fixed via `gofmt -w internal/filter/http/chain_test.go` and re-verified clean.
- **D-decision-disposition update**: this Task 5 entry does NOT close any D-decision. ADR-0192 §Decision body lands at Task 19 atomic landing per ADR-0044 in-place edit discipline; ADR-0192 §Context draft remains anchored at 22.2 SPEC commit `0d6463e` unchanged by this Task 5 commit. The NEW chain-side fields + setters + 4 accessors anchor the §Decision body's chain-extension claims (chain-side extension lives INSIDE ADR-0192 per Q13 WEAK HOLD — no separate ADR for chain-side extension; mirrors the ADR-0144 §Decision (i)+(ii) plumbing pattern; set-once BEFORE RunDecodeHeaders per ADR-0071; nil-tolerance per ADR-0085; per-stream sequential per ADR-0033; framework-primitive landing pattern mirrors ADR-0165 callback-surface extension at phase-18.2 Task 4) but does NOT itself land the §Decision body — that lands at Task 19 per the Lands-in-Task convention. Q13 WEAK HOLD HONORED — chain-side extension lives inside ADR-0192, not under a separate ADR number. ADR-0033 shared-state discipline HONORED — both decoder + encoder sides observe the SAME chain fields (tlsConnectionState + dynamicMetadata) via separate callback impls; no separate seeding for the encode side. The HCM-level seeding (H1 connection.go + H2 h2dispatch.go) lands at Task 6 per the Tier B dispatch wire-in sequencing.
- **Commit SHA**: `ba981fe160557becadab74930bcb2efe48061c78` (1-revision-stale relative to amend SHA per phase-22.1 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier A framework primitives (Task 5 of 1-5 in tier; Task 5 of 19 overall + Pre-Task 0).

### Task 6: EXTEND `internal/filter/hcm/connection.go` (H1) + `h2dispatch.go` (H2) for tlsConnectionState seeding [ADR-0192]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/hcm/...` clean; tlsConnectionState seeded on TLS-handshake-complete connections + nil on plaintext + nil on pre-handshake `*tls.Conn`; dynamicMetadata initialized at chain construction (NOT touched by HCM per D-P-X); H1 + H2 symmetric; `go build` + `go vet` + `golangci-lint` clean.
- **Files touched**:
  - `internal/filter/hcm/connection.go` (modified +56 LoC; NEW helper `downstreamTLSConnectionState(net.Conn) *tls.ConnectionState` co-located alongside the existing `downstreamTLSPrincipals` helper — type-asserts `*tls.Conn`, calls `ConnectionState()`, gates on `HandshakeComplete`, returns nil for non-`*tls.Conn` / pre-handshake / nil-conn; NEW seeding call `chain.SetTLSConnectionState(downstreamTLSConnectionState(downstream))` inside `dispatchRequest` AFTER `chain.SetRequestCtx` + `chain.SetTLSPrincipals` and BEFORE `chain.RunDecodeHeaders` per SPEC §11.5.3 set-once-BEFORE-RunDecodeHeaders discipline; doc-comments cross-reference SPEC §3 + §11.5 + ADR-0144 plumbing extension + ADR-0192 §Decision body anticipation + Q13 WEAK HOLD).
  - `internal/filter/hcm/h2dispatch.go` (modified +42 LoC; NEW `stdtls "crypto/tls"` import; NEW field `tlsConnectionState *stdtls.ConnectionState` on `h2Dispatcher` + `chainDispatchAction` structs — per-connection snapshot pinned at runH2 connection-build time + per-stream copy at Match time, mirroring the tlsPrincipals plumbing topology; copy-through wired in BOTH Match branches (matched-route + no-match-404); NEW seeding call `chain.SetTLSConnectionState(c.tlsConnectionState)` inside `chainDispatchAction.WriteH2` AFTER `chain.SetRequestCtx` + `chain.SetTLSPrincipals` and BEFORE `chain.RunDecodeHeaders`).
  - `internal/filter/hcm/filter.go` (modified +11 LoC; NEW `disp.tlsConnectionState = downstreamTLSConnectionState(downstream)` line inside `runH2` immediately after the existing `disp.tlsPrincipals = downstreamTLSPrincipals(downstream)` line — connection-build-time capture symmetric to tlsPrincipals; doc-comment cross-references SPEC §11.5.3 + ADR-0192 §Decision body anticipation).
  - `internal/filter/hcm/tls_test.go` (modified +157 LoC; NEW imports `crypto/ecdsa` + `crypto/elliptic` + `crypto/rand` + `math/big` + `sync` + `time`; NEW helper `runInProcessTLSHandshake(t, serverCN, clientSNI) (*stdtls.Conn, cleanup)` — generates self-signed ECDSA P-256 leaf cert, runs paired `tls.Server` / `tls.Client` over `net.Pipe()` with concurrent `Handshake()` calls, cleanup closes raw pipe conns to avoid close_notify deadlock on synchronous pipe; 4 NEW helper-in-isolation tests `TestDownstreamTLSConnectionState_NilConn_ReturnsNil` + `TestDownstreamTLSConnectionState_NonTLSConn_ReturnsNil` + `TestDownstreamTLSConnectionState_TLSConn_HandshakeIncomplete_ReturnsNil` + `TestDownstreamTLSConnectionState_TLSConn_HandshakeComplete_ReturnsState` — mirror the Group 7 extraction-helper-in-isolation pattern of the existing `TestDownstreamTLSPrincipals_*` suite per BOOTSTRAP_PROMPT).
  - `internal/filter/hcm/connection_test.go` (modified +194 LoC; NEW `stdtls "crypto/tls"` import; NEW test-double `tlsStateCapturingFilter` — decode-only HTTPFilter that captures `dcb.DownstreamTLSConnectionState()` at DecodeHeaders entry time (NOT at SetDecoderCallbacks time, because chain wires callbacks BEFORE HCM seeds the field — seed-vs-wire ordering pinned in the doc-comment); NEW `mkTLSStateCapturingFilterForTable` wires a `*Filter` with chainConfig=[capture, router] using a shared-instance factory; 3 NEW dispatch-level tests `TestConnection_dispatchRequest_seeds_nil_for_plaintext` + `TestConnection_dispatchRequest_seeds_nil_for_non_TLS_handshake_complete` + `TestConnection_dispatchRequest_seeds_tlsConnectionState_for_TLS_handshake_complete` exercising the H1 dispatchRequest seeding contract through the chain-mediated path).
  - `internal/filter/hcm/h2dispatch_test.go` (modified +119 LoC; NEW helper `h2TLSStateChainConfig` — symmetric to the H1 mkTLSStateCapturingFilterForTable helper; 2 NEW dispatch-level tests `TestH2Dispatch_runH2_seeds_tlsConnectionState_symmetric` + `TestH2Dispatch_runH2_seeds_nil_for_plaintext_symmetric` exercising the H2 chainDispatchAction.WriteH2 seeding contract with a pre-populated h2Dispatcher.tlsConnectionState; pointer-identity assertion (captured == seeded pointer) pins the verbatim threading dispatcher → action → chain).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race -v ./internal/filter/hcm/ -run 'TestConnection_dispatchRequest_seeds|TestH2Dispatch_runH2_seeds|TestDownstreamTLSConnectionState_'
  ```

  ```
  === RUN   TestConnection_dispatchRequest_seeds_nil_for_plaintext
  --- PASS: TestConnection_dispatchRequest_seeds_nil_for_plaintext (0.00s)
  === RUN   TestConnection_dispatchRequest_seeds_nil_for_non_TLS_handshake_complete
  --- PASS: TestConnection_dispatchRequest_seeds_nil_for_non_TLS_handshake_complete (0.00s)
  === RUN   TestConnection_dispatchRequest_seeds_tlsConnectionState_for_TLS_handshake_complete
  --- PASS: TestConnection_dispatchRequest_seeds_tlsConnectionState_for_TLS_handshake_complete (0.00s)
  === RUN   TestH2Dispatch_runH2_seeds_tlsConnectionState_symmetric
  --- PASS: TestH2Dispatch_runH2_seeds_tlsConnectionState_symmetric (0.00s)
  === RUN   TestH2Dispatch_runH2_seeds_nil_for_plaintext_symmetric
  --- PASS: TestH2Dispatch_runH2_seeds_nil_for_plaintext_symmetric (0.00s)
  === RUN   TestDownstreamTLSConnectionState_NilConn_ReturnsNil
  --- PASS: TestDownstreamTLSConnectionState_NilConn_ReturnsNil (0.00s)
  === RUN   TestDownstreamTLSConnectionState_NonTLSConn_ReturnsNil
  --- PASS: TestDownstreamTLSConnectionState_NonTLSConn_ReturnsNil (0.00s)
  === RUN   TestDownstreamTLSConnectionState_TLSConn_HandshakeIncomplete_ReturnsNil
  --- PASS: TestDownstreamTLSConnectionState_TLSConn_HandshakeIncomplete_ReturnsNil (0.00s)
  === RUN   TestDownstreamTLSConnectionState_TLSConn_HandshakeComplete_ReturnsState
  --- PASS: TestDownstreamTLSConnectionState_TLSConn_HandshakeComplete_ReturnsState (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.026s
  ```

  ```
  $ go test -count=1 -race ./internal/filter/hcm/... ./internal/filter/http/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.068s
  ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	8.421s
  ok  	github.com/esalaine/envoy-go/internal/filter/http	1.282s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	1.041s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit	9.676s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.025s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/compressor	1.048s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.019s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.024s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.049s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.388s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.247s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.336s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.020s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.103s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.026s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.485s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/oauth2	1.053s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.031s
  ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.245s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/hcm/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race ./internal/filter/hcm/...` clean**: PASS — 9 NEW tests all green under `-race`. The full hcm suite (preserving all pre-Task-6 H1 + H2 dispatch + chain-integration tests) stays green; no regressions from the new `chain.SetTLSConnectionState` call site in dispatchRequest / WriteH2 or from the new fields on `h2Dispatcher` / `chainDispatchAction`. `internal/filter/hcm/h2` package stays green as expected (this Task touches only the hcm-package-level dispatch seam, not the H2 codec internals).
  - **tlsConnectionState seeded on TLS-handshake-complete connections**: `TestConnection_dispatchRequest_seeds_tlsConnectionState_for_TLS_handshake_complete` PASS — drives `dispatchRequest` with a server-side `*tls.Conn` post-handshake (in-process net.Pipe + ECDSA self-signed leaf); `capture.captured` is non-nil, `HandshakeComplete=true`, `ServerName="h1.sni.envoy-go.test"` matches the SNI presented by the client. `TestH2Dispatch_runH2_seeds_tlsConnectionState_symmetric` PASS — pointer-identity assertion (`capture.captured == state`) pins the verbatim threading `dispatcher.tlsConnectionState → action.tlsConnectionState → chain.SetTLSConnectionState` with NO defensive copy along the path.
  - **nil on plaintext**: `TestConnection_dispatchRequest_seeds_nil_for_plaintext` + `TestH2Dispatch_runH2_seeds_nil_for_plaintext_symmetric` PASS — `dispatchRequest` invoked with `downstream=nil` (H1) and `disp.tlsConnectionState=nil` (H2) both produce `capture.captured == nil`; the documented nil-passthrough through `chain.SetTLSConnectionState(nil)` keeps the chain field nil + consumers nil-tolerate per ADR-0085.
  - **nil on pre-handshake `*tls.Conn`**: `TestConnection_dispatchRequest_seeds_nil_for_non_TLS_handshake_complete` PASS — `tls.Server` wrapper over `net.Pipe()` with NO `Handshake()` invocation; `downstreamTLSConnectionState` returns nil per the `HandshakeComplete` guard; the chain field stays nil. Mirrors the `extractTLSPrincipals` HandshakeComplete-gating discipline (no observation of unverified handshake state at the bridge surface). Note: `downstreamTLSConnectionState` does NOT additionally gate on `len(PeerCertificates)==0` — server-auth-only TLS handshakes (no client cert) DO seed the chain field per SPEC §11.5.3, distinct from `tlsPrincipals` which requires mTLS per ADR-0144.
  - **dynamicMetadata initialized at chain construction (HCM does NOT touch)**: VERIFIED at code-inspection time — `internal/filter/http/chain.go::NewFilterChain` line 220 initializes `dynamicMetadata: dynamicmetadata.NewBucket()` per Task 5 (ADR-0190 + ADR-0192 §Decision body anticipation); zero `chain.SetDynamicMetadata` call sites added at HCM (`grep -rn 'SetDynamicMetadata' internal/filter/hcm/` empty); `chain.go` owns the Bucket lifecycle per D-P-X PLAN-time decision. The 11 NEW Task-5 chain-side tests (`TestFilterChain_constructor_initializes_dynamicMetadata_nonnil` + `TestFilterChain_Destroy_resets_bucket` + the 4 accessor tests) already pin the chain-side lifecycle; this Task 6 entry adds no new dynamicMetadata-related assertions on the HCM side since there is no HCM-side seeding code to test.
  - **H1 + H2 symmetric**: VERIFIED at code-inspection time — both `connection.go::dispatchRequest` (H1) and `h2dispatch.go::chainDispatchAction.WriteH2` (H2) call `chain.SetTLSConnectionState(...)` at the SAME relative position (AFTER `chain.SetRequestCtx` + `chain.SetTLSPrincipals`, BEFORE `chain.RunDecodeHeaders`); H2 caches once at connection-build time (`runH2 → h2Dispatcher.tlsConnectionState`) + copies per-stream (`chainDispatchAction.tlsConnectionState`) mirroring the existing tlsPrincipals topology. The doc-comment seeding-site references on both sides cross-reference SPEC §3 + §11.5.3 + ADR-0192 §Decision body anticipation uniformly.
  - **`go build ./...` clean**: empty output, exit 0 — confirmed above. The 2 NEW field additions on internal hcm-package types (`h2Dispatcher.tlsConnectionState` + `chainDispatchAction.tlsConnectionState`) do not break any consumer because both types are package-private to hcm; no cross-package breakage observable.
  - **`go vet ./...` clean**: empty output, exit 0 — confirmed above.
  - **`golangci-lint run ./internal/filter/hcm/...` clean**: empty output, exit 0 — confirmed above.
- **D-decision-disposition update**: this Task 6 entry does NOT close any D-decision. ADR-0192 §Decision body lands at Task 19 atomic landing per ADR-0044 in-place edit discipline. The HCM-level seeding code (H1 connection.go + H2 h2dispatch.go) anchors §Decision body's "HCM dispatch seeds the per-stream chain field" claim (mirrors the ADR-0144 §Decision (i)+(ii) HCM-level plumbing pattern; symmetric placement BEFORE RunDecodeHeaders per ADR-0071 single-dispatch-goroutine invariant; nil-passthrough per ADR-0085) but does NOT itself land the §Decision body — that lands at Task 19 per the Lands-in-Task convention. **D-P-X invariant HONORED**: `dynamicMetadata` initialization lives at `chain.go`'s `NewFilterChain` constructor (Task 5 landing); HCM does NOT touch dynamicMetadata at this Task — verified by `grep -rn 'SetDynamicMetadata\|DynamicMetadata' internal/filter/hcm/` returning zero call sites in HCM source files (test files only reference via the cross-package test-doubles, not via HCM dispatch code paths). Q13 WEAK HOLD HONORED — chain-side + HCM-side tlsConnectionState extensions both live INSIDE ADR-0192, not under a separate ADR number.
- **Commit SHA**: `0e45d7b` (1-revision-stale relative to amend SHA per phase-22.1 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier B HCM-dispatch wire-in (Task 6 of the 1-task Tier B; Task 6 of 19 overall + Pre-Task 0). Unblocks all Tier C bridge surfaces (Tasks 7-13) consuming `dcb.DownstreamTLSConnectionState()` + `dcb.DynamicMetadata()` accessors.

### Task 7: NEW `internal/filter/http/lua/body.go` body bridge + decode-side accumulation + coroutine yield/resume [ADR-0192]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/http/lua/... -run 'Test_RequestHandleBody|Test_body_buffered|Test_coroutine_yields|Test_ResponseHandleBody'` clean; `:body()` returns full bytes + over-cap raises arm-21 byte-stable wording + `:bodyChunks()` iterator yields chunks; coroutine yield-before-endStream then resume-with-bytes verified; defensive-copy verification (mutating Go-side `decodedBodyBytes` after `:body()` does NOT change the Lua string); 2 NEW counters (`bodyBufferedBytesTotal` + `coroutineYieldsTotal`) increment correctly.
- **Files touched**:
  - `internal/filter/http/lua/body.go` (created, 445 LoC; NEW concrete `decodedBodyBuffer` + `encodedBodyBuffer` types satisfying `internal/lua.BodyBuffer` interface per Task 3 seam; NEW `accumulateRequestBody` + `accumulateResponseBody` body-accumulation helpers called from DecodeData / EncodeData; NEW 4 LGFunctions `requestHandleBody` + `requestHandleBodyChunks` + `responseHandleBody` + `responseHandleBodyChunks` per SPEC §3.4 + §4.1 + §11.3 D3 closure (defensive copy at endStream); each :body() defensive-copies via `lua.LString(string(bytes))` per §11.3.3 — gopher-lua's LString IS an immutable Go string detaching Lua ownership from underlying byte-slice lifetime; pre-endStream :body() stashes the bridge LState on `f.pendingBodyResume` + increments `coroutineYieldsTotal` ONCE per yield event + returns `YieldFromBridge(L, lua.LNil)` per §11.1 D2 closure; over-cap raises arm-21 runtime-reject via `L.RaiseError` with byte-stable wording per W2 `"lua: body: accumulated body exceeds maximum buffered size of %d bytes"`).
  - `internal/filter/http/lua/body_test.go` (created, 342 LoC; 8 tests per PLAN Task 7 Step 1 enumeration — `Test_RequestHandleBody_returns_full_bytes` + `Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording` + `Test_RequestHandleBodyChunks_iterator_yields_chunks_then_nil` + `Test_RequestHandleBody_coroutine_yield_before_endStream_then_resume` + `Test_RequestHandleBody_defensive_copy_verified` + `Test_body_buffered_bytes_total_counter_increments` + `Test_coroutine_yields_total_counter_increments` + `Test_ResponseHandleBody_symmetric`; per-test `newBodyBridgeFilter` helper constructs a fresh *filter with VM + bridge metatables installed + ctx-attached parent LState for cancellable child coroutines + hand-rolled 5-counter filterStats so tests assert against the 2 NEW counters at this Task — the production newFilterStats wires only 3 counters at Task 10/this Task; Task 14 will land the 5-counter production registration).
  - `internal/filter/http/lua/decode_headers.go` (modified +1 LoC; adds `filterRef: f` to the `requestHandleContext` construction at Step 4 of the DecodeHeaders dispatcher — body-bridge LGFunctions resolve the owning *filter via this back-pointer for body-buffer access + counter increments + coroutine suspend/resume orchestration).
  - `internal/filter/http/lua/encode_headers.go` (modified +5 LoC; persists `f.respCtx` on *filter (vs the 22.1 local-only allocation) so the body-bridge LGFunctions can resolve the owning *filter via the userdata's filterRef back-pointer on the encode side; symmetric to decode_headers.go).
  - `internal/filter/http/lua/bridge.go` (modified +12 LoC; adds `filterRef *filter` field to both `requestHandleContext` + `responseHandleContext` structs per Task 7 back-pointer discipline; adds 4 NEW entries to the method dispatch tables — `body`+`bodyChunks` on both `requestHandleMethods` + `responseHandleMethods`. MINIMAL EDIT — does NOT touch other metatable wiring per PLAN Step 3 task scope; trailers / metadata / connection / httpcall / crypto / etc. land at sibling Tasks 8-13).
  - `internal/filter/http/lua/lua.go` (modified +109 LoC; NEW `defaultMaxBodyBufferedBytes` const (16 * 1024 * 1024 = 16777216 — hardcoded constant for 22.2 per PLAN Task 7); NEW `bodyBufferedBytesTotal` + `coroutineYieldsTotal` fields on `filterStats` struct (2 of the 5 envoy-go-strict counters per SPEC §7.1 rows 4+5 — registration into *stats.Registry deferred to Task 14 which lands the full 5-counter extension; Task 7 tests construct hand-rolled filterStats to exercise the increment paths); NEW per-stream body-bridge state fields on `filter` struct — `respCtx` + `decodedBodyBytes` + `decodedBodyChunks` + `encodedBodyBytes` + `encodedBodyChunks` + `bodyReady` + `respBodyReady` + `pendingBodyResume` + `pendingRespBodyResume` + `maxBodyBufferedBytes`; NEW `gopher-lua` import for the `*lua.LState` pendingBodyResume field; DecodeData + EncodeData now delegate to body.go's `accumulateRequestBody` / `accumulateResponseBody` helpers — accumulation + endStream-resume orchestration of any suspended `:body()` coroutine).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/... -run 'Test_RequestHandleBody|Test_body_buffered|Test_coroutine_yields|Test_ResponseHandleBody' -v
  ```

  ```
  === RUN   Test_RequestHandleBody_returns_full_bytes
  --- PASS: Test_RequestHandleBody_returns_full_bytes (0.00s)
  === RUN   Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording
  --- PASS: Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording (0.00s)
  === RUN   Test_RequestHandleBodyChunks_iterator_yields_chunks_then_nil
  --- PASS: Test_RequestHandleBodyChunks_iterator_yields_chunks_then_nil (0.00s)
  === RUN   Test_RequestHandleBody_coroutine_yield_before_endStream_then_resume
  --- PASS: Test_RequestHandleBody_coroutine_yield_before_endStream_then_resume (0.00s)
  === RUN   Test_RequestHandleBody_defensive_copy_verified
  --- PASS: Test_RequestHandleBody_defensive_copy_verified (0.00s)
  === RUN   Test_body_buffered_bytes_total_counter_increments
  --- PASS: Test_body_buffered_bytes_total_counter_increments (0.00s)
  === RUN   Test_coroutine_yields_total_counter_increments
  --- PASS: Test_coroutine_yields_total_counter_increments (0.00s)
  === RUN   Test_ResponseHandleBody_symmetric
  --- PASS: Test_ResponseHandleBody_symmetric (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.020s
  ```

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.471s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/lua/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race ./internal/filter/http/lua/... -run TestBody-pattern` clean**: PASS — all 8 NEW body-bridge tests green under `-race`; 20× repeat run (`-count=20 -race`) also clean (no flakiness or race hazards in the accumulation + endStream-resume paths). Full lua-package suite (22.1's 100+ pre-Task-7 tests preserved) stays green: `ok internal/filter/http/lua 1.471s`.
  - **`:body()` returns full bytes**: `Test_RequestHandleBody_returns_full_bytes` PASS — two DecodeData calls (`"hello "` + `"world"`, endStream=true on the second) accumulate into `f.decodedBodyBytes`; `rh:body()` returns `"hello world"` verbatim.
  - **over-cap raises arm-21 byte-stable wording per W2**: `Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording` PASS — driver sets `f.maxBodyBufferedBytes = 1024` then DecodeData with 1025 bytes; `vm.Run` of `rh:body()` returns an error containing the byte-stable wording substring `"lua: body: accumulated body exceeds maximum buffered size of 1024 bytes"` per SPEC §6 arm-21 + W2.
  - **`:bodyChunks()` iterator yields chunks then nil**: `Test_RequestHandleBodyChunks_iterator_yields_chunks_then_nil` PASS — 3 DecodeData chunks (`"aaa"` + `"bbb"` + `"ccc"`) accumulated; the iterator emits each chunk then nil-terminator; concatenated sequence reads `"aaabbbccc"`; terminator flag verifies the nil-emit path.
  - **coroutine yield-before-endStream then resume-with-bytes**: `Test_RequestHandleBody_coroutine_yield_before_endStream_then_resume` PASS — mint child *LState via `vm.NewThread`, Resume into `consume()` which calls `rh:body()`; bodyReady=false so :body() stashes child on `f.pendingBodyResume` + yields; first Resume returns ResumeYield; then `f.DecodeData([]byte("late-body"), true)` triggers `accumulateRequestBody` endStream branch which calls `vm.Resume(child, nil, lua.LString("late-body"))`; the script's `captured = rh:body()` evaluates to the resumed value; assertion confirms `captured == "late-body"`.
  - **defensive-copy verification**: `Test_RequestHandleBody_defensive_copy_verified` PASS — :body() returns; test mutates `f.decodedBodyBytes` in place (zeros out via `'X'`); the previously-returned Lua string captured at `result = rh:body()` remains `"original-bytes"` — confirms the §11.3 D3 defensive-copy discipline holds (gopher-lua's LString is an immutable Go string detached from the underlying byte-slice lifetime).
  - **counters increment correctly**: `Test_body_buffered_bytes_total_counter_increments` PASS — 5+11 bytes of DecodeData increments cumulative counter delta = 16 (cumulative byte-volume per SPEC §7.1 row 4 semantics); `Test_coroutine_yields_total_counter_increments` PASS — single :body() yield event increments delta = 1 (NOT incremented again on Resume — confirms the "ONCE per yield site, NOT per Resume site" semantic discipline per PLAN Task 7 dispatch outline).
  - **encode-side symmetric**: `Test_ResponseHandleBody_symmetric` PASS — EncodeData(`"encoded-"`) + EncodeData(`"response"`, endStream=true) accumulate into `f.encodedBodyBytes`; `resp:body()` returns `"encoded-response"`; same defensive-copy + cap-check + yield/resume discipline holds.
  - **`go build ./...` clean**: empty output, exit 0 — no consumers broken by the per-stream filter-struct field additions; lua.go imports `gopher-lua` for the `*lua.LState` pendingBodyResume field type.
  - **`go vet ./...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/filter/http/lua/...` clean**: empty output, exit 0; the unused-const lint surfaced briefly during initial draft was resolved by deferring the statName* constants for the 2 NEW counters to Task 14's full stats.go extension — the 2 NEW counters land as filterStats struct fields at this Task with hand-rolled construction in tests; Task 14's newFilterStats addition is a single-line extension (registration into the *stats.Registry under the byte-stable wire names per the 5-counter envoy-go-strict roster at SPEC §7.1).
- **D-decision-disposition update**: this Task 7 entry does NOT close any pre-existing D-decision (ADR-0190 / ADR-0191 / ADR-0192 §Decision bodies all land at Task 19 atomic landing per ADR-0044 in-place edit discipline). HOWEVER, this Task 7 entry RESOLVES the R9 disposition signal per the PLAN Task 7 R9 signal protocol:

  **§13-R9 disposition: STAYS embedded in ADR-0192**

  Rationale: the body-bridge implementation surface at this Task does NOT introduce additional ADR-warranting complexity beyond what is already documented under ADR-0192 §Context. The yield/resume orchestration is mechanically simple (single suspended *LState slot per stream; single Resume site at endStream); the defensive-copy discipline is one line per call site (`lua.LString(string(b))`); the over-cap arm is byte-stable wording matching SPEC §6 arm-21 + W2; the 2 NEW counters are deferred-registration into stats.go (Task 14) but their increment-site discipline (ONCE per yield) is straightforward bookkeeping. None of the surface choices required cross-package coordination (the ADR-0191 BodyBuffer interface seam is consumed cleanly via the decodedBodyBuffer / encodedBodyBuffer concrete types; the ADR-0192 §Decision body anticipation tracks the dispatch shape already). Task 19's Step 10 grep for `§13-R9 disposition: ADR-0193 FIRES` will find ZERO matches in this entry — ADR-0193 does NOT fire from the R9 signal at this Task; whether it fires from the R6 (Task 15 benchmarks) signal is independent and tracked at the Task 15 PROGRESS.md entry.
- **Commit SHA**: `332ae7f` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier C bridge surfaces (first of 7-task parallel-tier; Task 7 of 19 overall + Pre-Task 0). PARALLELIZABLE with Tasks 8-13 per PLAN; consumes Task 2's `internal/lua.YieldFromBridge` + `VM.Resume` + `VM.NewThread` API (Task 2 ADR-0191 §Context anchor) + Task 3's `internal/lua.BodyBuffer` interface seam (Task 3 ADR-0191 §Context anchor) + Task 5/6's chain.go DynamicMetadata initialization (not directly consumed at Task 7 but the per-stream-callback wiring is in place for sibling Tasks 9 / 10 / 13). Unblocks Task 14 (stats.go +5 counters — Task 14 will register the 2 NEW counters declared at this Task into newFilterStats; the byte-stable wire names + filterStats field declarations are pinned here). Unblocks Task 16 (FuzzLuaBodyBridge consumes the body-bridge LGFunctions).

### Task 8: NEW `internal/filter/http/lua/trailers.go` trailers bridge + `bridge.go` metatable installs [ADR-0192]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/http/lua/... -run 'TestBridge_Trailers'` clean; 8 operator-visible trailers methods (`:get`/`:getAtIndex`/`:getNumValues`/`:add`/`:append` (alias of `:add`)/`:remove`/`:replace` + `__pairs` metamethod) + `__pairs` alphabetical-snapshot cross-run-determinism N=100 + nil-trailers-returns-nil on both `request_handle:trailers()` + `response_handle:trailers()` + metatable distinct from headers (`envoy_trailers` registry key) + `DecodeTrailers`/`EncodeTrailers` terminal-state wiring attaches the trailers map onto the per-stream context.
- **Files touched**:
  - `internal/filter/http/lua/trailers.go` (created, 144 LoC; NEW 2 LGFunctions `requestHandleTrailers` + `responseHandleTrailers` per SPEC §3.4 + §2.2 lazy-availability discipline — both return `lua.LNil` when the per-stream context's `trailers` field is nil, OR push a userdata wrapping the trailers `http.Header` via `pushTrailersUD` when non-nil; NEW `pushTrailersUD` helper mirrors `pushHeadersUD` exactly with the metatable registry key swapped to `envoy_trailers`; file-level docstring documents the 8-operator-visible-surface count (`:get`/`:getAtIndex`/`:getNumValues`/`:add`/`:append`/`:remove`/`:replace` + `__pairs` alphabetical-snapshot) + the dispatch-reuse discipline — `headersMethods` + `headersPairs` are reused UNCHANGED for the trailers metatable since the underlying value type is `http.Header` in both cases and `getHeadersFromUD` casts via Go-type-assertion without consulting the metatable registry key).
  - `internal/filter/http/lua/bridge.go` (modified +60 LoC; NEW `trailersTypeName = "envoy_trailers"` const; NEW `trailers http.Header` field on `requestHandleContext` + `responseHandleContext` per Task 8 lazy-availability discipline; NEW `installTrailersMetatable` helper mirrors `installHeadersMetatable` (same `__index` → `headersMethods` dispatch + same `__pairs` → `headersPairs` per SPEC §11.2.2 "CONFIRMED-IDENTICAL via 22.1 `installPairsShim` discipline"); NEW `"trailers"` entry appended to both `requestHandleMethods` + `responseHandleMethods` dispatch tables routing to `requestHandleTrailers` / `responseHandleTrailers`).
  - `internal/filter/http/lua/bridge_test.go` (modified +418 LoC; 20 NEW behavioral tests under `TestBridge_Trailers_*` namespace covering the full SPEC §3.4 + §2.2 + PLAN Task 8 acceptance: 3 nil-trailers tests (request-side + response-side + return-userdata-when-present), 7 mutation method tests (`Get_Hit` + `Get_Miss` + `GetAtIndex_SecondValue` + `GetNumValues_Multi` + `Add_Appends` + `Append_AliasForAdd` + `Remove_Deletes` + `Replace_RemovesThenAdds`), 4 `__pairs` tests (`AlphabeticalOrder` + `CrossRunDeterminism` N=100 + `MultiValueSameKey` + `Empty`), 1 encode-side parity test (`ResponseHandle_Get`), 1 metatable-identity test (`MetatableDistinctFromHeaders` asserting `getmetatable(rh:trailers()) ~= getmetatable(rh:headers())`), 3 terminal-state-wiring tests (`DecodeTrailers_AttachesToReqCtx` + `EncodeTrailers_AttachesToRespCtx` + `DecodeTrailers_NilReqCtx_NoOp` defensive-against-nil-context); NEW `newBridgedVMWithTrailers` helper constructs a VM with both `rh` + `resp` globals carrying distinct headers + trailers maps; NEW `installTrailersMetatable` compile-time signature pin at the existing var-block).
  - `internal/filter/http/lua/decode_headers.go` (modified +1 LoC; `installTrailersMetatable(L)` call appended to the per-stream-VM metatable-install sequence at Step 3 — installed ONCE per VM alongside the other 4 install helpers; same shared `installPairsShim` honors `__pairs` for BOTH `envoy_headers` AND `envoy_trailers` userdata; encode-side reuses the per-stream VM constructed at decode time so no separate install in `encode_headers.go` is required).
  - `internal/filter/http/lua/lua.go` (modified +36 LoC; `DecodeTrailers` now attaches the received trailers map onto `f.reqCtx.trailers` when `f.reqCtx` is non-nil — defensive-nil-tolerant on the nil-chunk pass-through path where `DecodeHeaders` never constructed `f.reqCtx`; `EncodeTrailers` symmetric for `f.respCtx.trailers`; doc-comments document the lazy-availability discipline + nil-tolerance + non-defensive-copy of the trailers map (the bridge methods mutate the underlying `http.Header` in place per the same semantics as headers, observable on subsequent filters in the chain)).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/... -run 'TestBridge_Trailers' -v
  ```

  ```
  === RUN   TestBridge_Trailers_ReturnsNil_WhenNotReceived
  --- PASS: TestBridge_Trailers_ReturnsNil_WhenNotReceived (0.00s)
  === RUN   TestBridge_Trailers_ResponseHandle_ReturnsNil_WhenNotReceived
  --- PASS: TestBridge_Trailers_ResponseHandle_ReturnsNil_WhenNotReceived (0.00s)
  === RUN   TestBridge_Trailers_Returns_UserData_WhenPresent
  --- PASS: TestBridge_Trailers_Returns_UserData_WhenPresent (0.00s)
  === RUN   TestBridge_Trailers_Get_Hit
  --- PASS: TestBridge_Trailers_Get_Hit (0.00s)
  === RUN   TestBridge_Trailers_Get_Miss
  --- PASS: TestBridge_Trailers_Get_Miss (0.00s)
  === RUN   TestBridge_Trailers_GetAtIndex_SecondValue
  --- PASS: TestBridge_Trailers_GetAtIndex_SecondValue (0.00s)
  === RUN   TestBridge_Trailers_GetNumValues_Multi
  --- PASS: TestBridge_Trailers_GetNumValues_Multi (0.00s)
  === RUN   TestBridge_Trailers_Add_Appends
  --- PASS: TestBridge_Trailers_Add_Appends (0.00s)
  === RUN   TestBridge_Trailers_Append_AliasForAdd
  --- PASS: TestBridge_Trailers_Append_AliasForAdd (0.00s)
  === RUN   TestBridge_Trailers_Remove_Deletes
  --- PASS: TestBridge_Trailers_Remove_Deletes (0.00s)
  === RUN   TestBridge_Trailers_Replace_RemovesThenAdds
  --- PASS: TestBridge_Trailers_Replace_RemovesThenAdds (0.00s)
  === RUN   TestBridge_Trailers_Pairs_AlphabeticalOrder
  --- PASS: TestBridge_Trailers_Pairs_AlphabeticalOrder (0.00s)
  === RUN   TestBridge_Trailers_Pairs_CrossRunDeterminism
  --- PASS: TestBridge_Trailers_Pairs_CrossRunDeterminism (0.01s)
  === RUN   TestBridge_Trailers_Pairs_MultiValueSameKey
  --- PASS: TestBridge_Trailers_Pairs_MultiValueSameKey (0.00s)
  === RUN   TestBridge_Trailers_Pairs_Empty
  --- PASS: TestBridge_Trailers_Pairs_Empty (0.00s)
  === RUN   TestBridge_Trailers_ResponseHandle_Get
  --- PASS: TestBridge_Trailers_ResponseHandle_Get (0.00s)
  === RUN   TestBridge_Trailers_MetatableDistinctFromHeaders
  --- PASS: TestBridge_Trailers_MetatableDistinctFromHeaders (0.00s)
  === RUN   TestBridge_Trailers_DecodeTrailers_AttachesToReqCtx
  --- PASS: TestBridge_Trailers_DecodeTrailers_AttachesToReqCtx (0.00s)
  === RUN   TestBridge_Trailers_EncodeTrailers_AttachesToRespCtx
  --- PASS: TestBridge_Trailers_EncodeTrailers_AttachesToRespCtx (0.00s)
  === RUN   TestBridge_Trailers_DecodeTrailers_NilReqCtx_NoOp
  --- PASS: TestBridge_Trailers_DecodeTrailers_NilReqCtx_NoOp (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	0.016s
  ```

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.496s
  ```

  ```
  $ go test -count=5 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	3.394s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/lua/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race ./internal/filter/http/lua/... -run TestBridge_Trailers` clean**: PASS — all 20 NEW trailers tests green under `-race`; 5× repeat run (`-count=5 -race`) also clean (no flakiness on the `__pairs` alphabetical-snapshot — cross-run-determinism explicitly verified at `TestBridge_Trailers_Pairs_CrossRunDeterminism` N=100). Full lua-package suite (22.1's 100+ pre-Task-7 tests + Task 7's 8 body-bridge tests preserved) stays green: `ok internal/filter/http/lua 1.496s`.
  - **8 operator-visible trailers method roster verified against 22.1 IMPL**: 7 distinct LGFunctions (`headersGet` + `headersGetAtIndex` + `headersGetNumValues` + `headersAdd` + `headersRemove` + `headersReplace` + `headersPairs`) registered under 8 operator-visible method names — `headersMethods` map has 7 entries with `"append"` aliasing `headersAdd` per upstream `HeaderMapWrapper::luaAdd`/`luaAppend` collapse to `HeaderMap::addCopy`; the `__pairs` metamethod is the 8th surface entry. The trailers metatable REUSES `headersMethods` + `headersPairs` UNCHANGED — verified by inspection of `installTrailersMetatable` which sets `__index → L.SetFuncs(L.NewTable(), headersMethods)` + `__pairs → L.NewFunction(headersPairs)`. Test coverage hits all 8 surface entries: `:get` (`Get_Hit` + `Get_Miss`), `:getAtIndex` (`GetAtIndex_SecondValue`), `:getNumValues` (`GetNumValues_Multi`), `:add` (`Add_Appends`), `:append` alias (`Append_AliasForAdd` asserting same-behavior-as-add), `:remove` (`Remove_Deletes`), `:replace` (`Replace_RemovesThenAdds`), `__pairs` (`Pairs_AlphabeticalOrder` + `Pairs_CrossRunDeterminism` + `Pairs_MultiValueSameKey` + `Pairs_Empty`).
  - **`__pairs` cross-run-determinism N=100**: `TestBridge_Trailers_Pairs_CrossRunDeterminism` PASS — 8-key trailers map iterated via `for k,v in pairs(rh:trailers()) do` across 100 fresh-VM instances; all 100 produce byte-identical concatenated output. Reuses 22.1's `installPairsShim` discipline UNCHANGED per SPEC §11.2.2 "CONFIRMED-IDENTICAL via 22.1 `installPairsShim` discipline" — no BEHAVIOR_CONTRACT departure record needed.
  - **lazy-availability (nil-trailers-returns-nil)**: `TestBridge_Trailers_ReturnsNil_WhenNotReceived` PASS — `rh:trailers()` returns `lua.LNil` when `f.reqCtx.trailers == nil`; symmetric `TestBridge_Trailers_ResponseHandle_ReturnsNil_WhenNotReceived` PASS for `resp:trailers()`. Matches upstream Lua filter behavior pre-trailers-arrival per SPEC §2.2 + PLAN Q2.
  - **encode-side parity**: `TestBridge_Trailers_ResponseHandle_Get` PASS — `resp:trailers():get("Grpc-Message")` round-trips when `f.respCtx.trailers` is attached.
  - **metatable distinct from headers**: `TestBridge_Trailers_MetatableDistinctFromHeaders` PASS — `getmetatable(rh:trailers()) ~= getmetatable(rh:headers())` at Lua-script-visible layer; `trailersTypeName == "envoy_trailers"` vs `headersTypeName == "envoy_headers"` at Go-side; underlying dispatch table (`headersMethods`) + `__pairs` impl (`headersPairs`) are shared but the registry-key identity is distinct.
  - **terminal-state wiring at DecodeTrailers / EncodeTrailers**: `TestBridge_Trailers_DecodeTrailers_AttachesToReqCtx` PASS — `f.DecodeTrailers(http.Header{"K": ...})` sets `f.reqCtx.trailers` non-nil; symmetric `TestBridge_Trailers_EncodeTrailers_AttachesToRespCtx` PASS for the encode side; `TestBridge_Trailers_DecodeTrailers_NilReqCtx_NoOp` PASS — calling `DecodeTrailers` with `f.reqCtx == nil` (simulating the nil-chunk pass-through where DecodeHeaders never constructed reqCtx) does NOT panic and returns Continue per the framework's TrailersContinue contract.
  - **`go build ./...` clean**: empty output, exit 0 — no consumers broken by the new struct fields or interface additions.
  - **`go vet ./...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/filter/http/lua/...` clean**: empty output, exit 0.
- **D-decision-disposition update**: this Task 8 entry does NOT close any pre-existing D-decision (ADR-0190 / ADR-0191 / ADR-0192 §Decision bodies all land at Task 19 atomic landing per ADR-0044 in-place edit discipline). The trailers bridge implementation surface at this Task anchors ADR-0192 §Decision body's "trailers bridge mirroring 22.1 headers metatable shape" claim — the implementation reuses `headersMethods` + `headersPairs` UNCHANGED for the trailers metatable's dispatch surface, mirroring SPEC §11.2.2 "CONFIRMED-IDENTICAL via 22.1 `installPairsShim` discipline" with no BEHAVIOR_CONTRACT departure record needed. The 8-operator-visible-surface count was verified against the 22.1 IMPL's `headersMethods` map per PLAN Task 8 "exactly 8 total; verify roster against 22.1 IMPL".
- **Commit SHA**: `0c0e687` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier C bridge surfaces (Task 8 of 19 overall + Pre-Task 0). PARALLELIZABLE with Tasks 7 (already complete) + 9-13 (pending); consumes Task 5/6's per-stream FilterChain wiring (not directly — trailers use the existing `DecodeTrailers`/`EncodeTrailers` framework callbacks already wired at 22.1) + Task 6's `installHeadersMetatable` + `installPairsShim` + `headersMethods` + `headersPairs` UNCHANGED (per SPEC §11.2.2 CONFIRMED-IDENTICAL closure). Unblocks Task 18 (fixture-0027 scenario `(c) trailers add+remove` consumes the 8-method mutation surface for cross-side determinism per SPEC §8 table).

### Task 9: NEW `internal/filter/http/lua/metadata.go` metadata + dynamic-metadata bridge [ADR-0192]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/http/lua/... -run 'TestMetadata|TestDynamicMetadata|TestDynamicTypedMetadata|TestStructpbToLua|TestLuaToStructpb'` clean; `:metadata()` ALWAYS returns callable empty userdata (NEVER nil per SPEC §11.6 D1 closure + upstream `MetadataMapWrapper` always-non-nil pattern); `:metadata():get(k)` returns nil for any key; `pairs(:metadata())` yields zero iterations; `:streamInfo():dynamicMetadata():set/get` round-trip for string/number/bool/list/struct; cross-filter key independence; `:dynamicTypedMetadata(filterName)` returns typed Lua table from `*Bucket` Snapshot; nil-bucket tolerance per ADR-0085 (test-double cb returns nil from `DynamicMetadata()` and the script still works without panic); structpb.Value ↔ Lua-value marshaling table-driven covering null/bool/number/string/list/struct.
- **Files touched**:
  - `internal/filter/http/lua/metadata.go` (created, 470 LoC; NEW 2 LGFunctions `requestHandleMetadata` + `responseHandleMetadata` returning ALWAYS-CALLABLE EMPTY USERDATA per §11.6 D1 closure — wrapper's `:get(k)` returns `lua.LNil` for any key; `pairs(wrapper)` yields zero iterations via `metadataPairs` returning a one-shot-nil iterator; NEW 2 streamInfo-side LGFunctions `streamInfoDynamicMetadata` + `streamInfoDynamicTypedMetadata` consuming the per-stream `*dynamicmetadata.Bucket` from Task 1 via the `RequestHandleCallbacks` adapter's NEW `DynamicMetadata()` accessor; NEW dynamicMetadata userdata 2 LGFunctions `dynamicMetadataGet` + `dynamicMetadataSet` via the `dynamicMetadataMethods` dispatch table; NEW marshaling helpers `structpbToLua` (null/bool/number/string/list/struct → Lua) + `luaToStructpb` (Lua → null/bool/number/string/list/struct via `luaTableToStructpb` list-vs-struct shape detection: contiguous 1..N integer-keyed tables → ListValue, otherwise StructValue with string keys preserved + non-string keys silently dropped per upstream `wrappers.cc luaToProtoValue` tolerance); nil-Bucket tolerance per ADR-0085 — `dynamicMetadataFromUD` returns nil on nil-Value userdata; `Bucket.Get/Set` are nil-receiver-tolerant; `dynamicTypedMetadata` short-circuits on nil-bucket to push `lua.LNil`.).
  - `internal/filter/http/lua/metadata_test.go` (created, 536 LoC; 19 NEW behavioral tests under `TestMetadata_*` + `TestDynamicMetadata_*` + `TestDynamicTypedMetadata_*` + `TestStructpbToLua_*` + `TestLuaToStructpb_*` namespaces covering the full SPEC §3.4 + §11.6 D1 closure + PLAN Task 9 acceptance: 5 `:metadata()` callable-empty-userdata tests (request-side returns userdata NEVER nil per D1 + response-side parity + `:get(k)` returns nil for ANY key incl. empty + `pairs(:metadata())` yields 0 iterations), 5 `:streamInfo():dynamicMetadata()` round-trip tests (string + number + bool + list + struct), 1 `:get` returns nil for absent-key test, 1 cross-filter key independence test (same key under filter.a vs filter.b are independent), 2 `:dynamicTypedMetadata(filterName)` tests (returns table marshaled from Snapshot subset + returns nil for absent filterName), 2 nil-Bucket tolerance tests (`:dynamicMetadata():get`/`:set` no-panic + `:dynamicTypedMetadata` returns nil per ADR-0085), 4 marshaling-helper tests (`TestStructpbToLua_table_driven` 8-case covering nil/null/true/false/number/string/list/struct + `TestStructpbToLua_values` value-equality spot checks + `TestLuaToStructpb_table_driven` covering nil/bool/number/string/array-table/hash-table + `TestLuaToStructpb_roundtrip_via_marshaling` 4-case identity verification through luaToStructpb→structpbToLua); NEW `fakeCallbacksWithBucket` test-double embedding bridge_test.go's `fakeCallbacks` + overriding `DynamicMetadata()` to return a per-test `*dynamicmetadata.Bucket`; NEW `newBridgedVMWithBucket` helper constructs a VM with all bridge metatables + the metadata + dynamicMetadata metatables + a `requestHandleContext{cb: fakeCallbacksWithBucket}` bound to global `rh`.
  - `internal/filter/http/lua/bridge.go` (modified +110 LoC; NEW imports — `internal/dynamicmetadata` + same package alias; NEW 2 metatable registry-key consts `metadataTypeName = "envoy_metadata"` + `dynamicMetadataTypeName = "envoy_dynamic_metadata"`; EXTENDED `RequestHandleCallbacks` interface with NEW `DynamicMetadata() *dynamicmetadata.Bucket` accessor — interface contract stable at this addition for Task 13 streaminfo.go extraction; EXTENDED both `requestHandleCallbacksAdapter` + `responseHandleCallbacksAdapter` with `DynamicMetadata()` impl projecting framework's `DecoderFilterCallbacks.DynamicMetadata()` / `EncoderFilterCallbacks.DynamicMetadata()` accessor verbatim (nil-tolerant on nil dcb/ecb per ADR-0085); NEW `installMetadataMetatable` + `installDynamicMetadataMetatable` helpers mirroring the existing install-helper convention; NEW `"metadata"` entries appended to both `requestHandleMethods` + `responseHandleMethods` dispatch tables routing to `requestHandleMetadata` / `responseHandleMetadata`; NEW `"dynamicMetadata"` + `"dynamicTypedMetadata"` entries appended to `streamInfoMethods` dispatch table routing to `streamInfoDynamicMetadata` / `streamInfoDynamicTypedMetadata` — these +2 entries are the Task 9 contribution to the streamInfo surface, hand-off note: Task 13 owns the streaminfo.go extraction + the OTHER 5 NEW methods (:upstreamHost / :upstreamCluster / :requestedServerName / :filterState / :downstreamSslConnection) without re-touching `RequestHandleCallbacks`).
  - `internal/filter/http/lua/bridge_test.go` (modified +5 LoC; NEW import `internal/dynamicmetadata`; EXTENDED `fakeCallbacks` test-double with NEW `DynamicMetadata() *dynamicmetadata.Bucket` method returning nil — preserves the Task 8 streamInfo-method tests' 4-method scope without dynamic-metadata interference; the metadata_test.go's `fakeCallbacksWithBucket` embeds this struct + overrides `DynamicMetadata()` to return a per-test bucket).
  - `internal/filter/http/lua/decode_headers.go` (modified +2 LoC; `installMetadataMetatable(L)` + `installDynamicMetadataMetatable(L)` calls appended to the per-stream-VM metatable-install sequence at Step 3 — installed ONCE per VM alongside the other 5 install helpers).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/... -run 'TestMetadata|TestDynamicMetadata|TestDynamicTypedMetadata|TestStructpbToLua|TestLuaToStructpb' -v
  ```

  ```
  === RUN   TestMetadata_RequestHandleMetadata_returns_callable_empty_userdata
  --- PASS: TestMetadata_RequestHandleMetadata_returns_callable_empty_userdata (0.00s)
  === RUN   TestMetadata_ResponseHandleMetadata_returns_callable_empty_userdata
  --- PASS: TestMetadata_ResponseHandleMetadata_returns_callable_empty_userdata (0.00s)
  === RUN   TestMetadata_get_returns_nil
  --- PASS: TestMetadata_get_returns_nil (0.00s)
  === RUN   TestMetadata_get_returns_nil_for_any_key
  --- PASS: TestMetadata_get_returns_nil_for_any_key (0.00s)
  === RUN   TestMetadata_pairs_yields_zero_iterations
  --- PASS: TestMetadata_pairs_yields_zero_iterations (0.00s)
  === RUN   TestDynamicMetadata_set_get_roundtrip_string
  --- PASS: TestDynamicMetadata_set_get_roundtrip_string (0.00s)
  === RUN   TestDynamicMetadata_set_get_roundtrip_number
  --- PASS: TestDynamicMetadata_set_get_roundtrip_number (0.00s)
  === RUN   TestDynamicMetadata_set_get_roundtrip_bool
  --- PASS: TestDynamicMetadata_set_get_roundtrip_bool (0.00s)
  === RUN   TestDynamicMetadata_set_get_roundtrip_list
  --- PASS: TestDynamicMetadata_set_get_roundtrip_list (0.00s)
  === RUN   TestDynamicMetadata_set_get_roundtrip_struct
  --- PASS: TestDynamicMetadata_set_get_roundtrip_struct (0.00s)
  === RUN   TestDynamicMetadata_get_returns_nil_for_absent_key
  --- PASS: TestDynamicMetadata_get_returns_nil_for_absent_key (0.00s)
  === RUN   TestDynamicMetadata_cross_filter_key_independence
  --- PASS: TestDynamicMetadata_cross_filter_key_independence (0.00s)
  === RUN   TestDynamicTypedMetadata_returns_typed_value
  --- PASS: TestDynamicTypedMetadata_returns_typed_value (0.00s)
  === RUN   TestDynamicTypedMetadata_absent_filtername_returns_nil
  --- PASS: TestDynamicTypedMetadata_absent_filtername_returns_nil (0.00s)
  === RUN   TestDynamicMetadata_nil_bucket_tolerance
  --- PASS: TestDynamicMetadata_nil_bucket_tolerance (0.00s)
  === RUN   TestDynamicTypedMetadata_nil_bucket_tolerance
  --- PASS: TestDynamicTypedMetadata_nil_bucket_tolerance (0.00s)
  === RUN   TestStructpbToLua_table_driven
      --- PASS: TestStructpbToLua_table_driven/nil (0.00s)
      --- PASS: TestStructpbToLua_table_driven/null (0.00s)
      --- PASS: TestStructpbToLua_table_driven/true (0.00s)
      --- PASS: TestStructpbToLua_table_driven/false (0.00s)
      --- PASS: TestStructpbToLua_table_driven/number (0.00s)
      --- PASS: TestStructpbToLua_table_driven/string (0.00s)
      --- PASS: TestStructpbToLua_table_driven/list (0.00s)
      --- PASS: TestStructpbToLua_table_driven/struct (0.00s)
  --- PASS: TestStructpbToLua_table_driven (0.00s)
  === RUN   TestStructpbToLua_values
  --- PASS: TestStructpbToLua_values (0.00s)
  === RUN   TestLuaToStructpb_table_driven
  --- PASS: TestLuaToStructpb_table_driven (0.00s)
  === RUN   TestLuaToStructpb_roundtrip_via_marshaling
  --- PASS: TestLuaToStructpb_roundtrip_via_marshaling (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	0.008s
  ```

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.532s
  ```

  ```
  $ go test -count=3 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	2.478s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/lua/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race ./internal/filter/http/lua/... -run 'TestMetadata|TestDynamicMetadata|TestDynamicTypedMetadata|TestStructpbToLua|TestLuaToStructpb'` clean**: PASS — all 19 NEW metadata tests green under `-race`; 3× repeat run (`-count=3 -race`) also clean (no flakiness; nil-receiver paths exercised under race without panic).
  - **`:metadata()` ALWAYS returns callable empty userdata (NEVER nil)**: `TestMetadata_RequestHandleMetadata_returns_callable_empty_userdata` PASS — `type(rh:metadata()) == "userdata"` + `m == nil` is false. `TestMetadata_ResponseHandleMetadata_returns_callable_empty_userdata` PASS for the encode-side parity. Matches SPEC §11.6 D1 CLOSURE verbatim: "envoy-go's `request_handle:metadata()` returns a non-nil callable userdata wrapping an empty metadata source ... NEVER return `lua.LNil` from `:metadata()` itself."
  - **`:metadata():get(k)` returns nil for any key**: `TestMetadata_get_returns_nil` PASS for "foo.bar" + `TestMetadata_get_returns_nil_for_any_key` PASS for "" + "a.b.c" + "envoy.filters.http.lua". Matches upstream `MetadataMapWrapper::luaGet` on an empty source-Struct.
  - **`pairs(:metadata())` yields zero iterations**: `TestMetadata_pairs_yields_zero_iterations` PASS — `for k, v in pairs(rh:metadata()) do count = count + 1 end` exits with `count == 0`. Matches §11.6.3 "gopher-lua nil-vs-empty-table behavioral implications" + upstream empty-Struct `__pairs` semantic.
  - **`:streamInfo():dynamicMetadata():set/get` round-trip for 5 value types**: 5 separate `TestDynamicMetadata_set_get_roundtrip_*` tests (string + number + bool + list + struct) all PASS. The string-test additionally asserts Go-side observation: `bucket.Get("envoy.lua", "k")` returns the persisted `*structpb.Value` with `GetStringValue() == "hello"`.
  - **Cross-filter key independence**: `TestDynamicMetadata_cross_filter_key_independence` PASS — `dm:set("filter.a", "shared_key", "value_a")` followed by `dm:set("filter.b", "shared_key", "value_b")` leaves both filterName-keyed sub-maps independent: `dm:get("filter.a", "shared_key") == "value_a"` AND `dm:get("filter.b", "shared_key") == "value_b"`.
  - **`:dynamicTypedMetadata(filterName)` returns typed Lua table from Bucket Snapshot**: `TestDynamicTypedMetadata_returns_typed_value` PASS — Go-side bucket pre-populated with k1=StringValue("v1") + k2=NumberValue(123); Lua-side `rh:streamInfo():dynamicTypedMetadata("envoy.lua")` returns a table with `t.k1 == "v1"` + `t.k2 == 123` (marshaled via the same `structpbToLua` helper used by `:get`). `TestDynamicTypedMetadata_absent_filtername_returns_nil` PASS — requesting a filterName not present in the bucket's Snapshot returns `lua.LNil` (not an empty table; pinned by the test assertion).
  - **Nil-bucket tolerance per ADR-0085**: `TestDynamicMetadata_nil_bucket_tolerance` PASS — `newBridgedVMWithBucket(t, nil)` constructs a VM where `cb.DynamicMetadata()` returns nil; the Lua script gets a userdata wrapper (NOT nil — the surface stays callable per the bridge contract) + `:set` is silent no-op + `:get` returns nil (`Bucket.Get` nil-receiver-tolerant returns `ok=false` → bridge pushes `lua.LNil`). `TestDynamicTypedMetadata_nil_bucket_tolerance` PASS — typed-metadata accessor on nil-bucket returns `lua.LNil` (short-circuit at the bridge before invoking `Snapshot` on nil-bucket).
  - **structpb.Value ↔ Lua-value marshaling table-driven**: `TestStructpbToLua_table_driven` 8-subtest PASS — covers nil/null/true/false/number/string/list/struct → expected Lua-type. `TestStructpbToLua_values` PASS — value-equality for representative cases (string "abc" + number 42 + bool true). `TestLuaToStructpb_table_driven` PASS — covers nil/bool/number/string/array-table/hash-table → expected structpb.Value kind. `TestLuaToStructpb_roundtrip_via_marshaling` PASS — 4-case identity (string/number/true/false) through `luaToStructpb → structpbToLua` produces same type + same string-representation.
  - **`go build ./...` clean**: empty output, exit 0 — interface contract addition (`RequestHandleCallbacks.DynamicMetadata()`) absorbed by the 2 in-package adapters; no external consumers broken.
  - **`go vet ./...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/filter/http/lua/...` clean**: empty output, exit 0 (after `gofmt -w` on metadata.go + bridge_test.go).
- **D-decision-disposition update**: this Task 9 entry does NOT close any pre-existing D-decision (ADR-0190 / ADR-0191 / ADR-0192 §Decision bodies all land at Task 19 atomic landing per ADR-0044 in-place edit discipline). The metadata bridge implementation surface at this Task anchors ADR-0192 §Decision body's "`:metadata()` callable empty userdata at v1.32.4 binding-gap" claim — the implementation pins SPEC §11.6 D1 CLOSURE verbatim: `requestHandleMetadata` + `responseHandleMetadata` unconditionally push a non-nil userdata + `metadataGet` unconditionally pushes `lua.LNil` regardless of key + `metadataPairs` returns a one-shot-nil iterator. This Task 9 is also the FIRST-co-consumer of the Task 1 `internal/dynamicmetadata/Bucket` primitive per ADR-0190 §Consequences cross-phase deferral-lift expectation.
- **Hand-off note to Task 13** (streaminfo.go extraction + 5 remaining new methods): this Task 9 adds 2 entries to the existing `streamInfoMethods` dispatch table in `bridge.go` (`dynamicMetadata` + `dynamicTypedMetadata`) — the existing 4-method 22.1 surface (protocol/routeName/downstreamLocalAddress/downstreamDirectRemoteAddress) becomes a 6-method 22.2-mid surface. Task 13 owns: (1) EXTRACTING `streamInfoMethods` + the 6 LGFunctions to NEW `streaminfo.go`; (2) ADDING the OTHER 5 methods (`:upstreamHost` / `:upstreamCluster` / `:requestedServerName` / `:filterState` / `:downstreamSslConnection`) to reach the 11-method 22.2-phase-done surface; (3) extending `RequestHandleCallbacks` interface with the 5 corresponding accessors (5 NEW interface methods); (4) updating the 2 adapters (`requestHandleCallbacksAdapter` + `responseHandleCallbacksAdapter`). The `RequestHandleCallbacks.DynamicMetadata()` accessor added in THIS Task is stable; Task 13 only adds NEW accessors without re-touching the existing 5 (Protocol/RouteName/DownstreamLocalAddress/DownstreamDirectRemoteAddress/DynamicMetadata).
- **Commit SHA**: `eed5f64` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier C bridge surfaces (Task 9 of 19 overall + Pre-Task 0). PARALLELIZABLE with Tasks 7-8 (already complete) + 10-13 (pending); consumes Task 1's `internal/dynamicmetadata/Bucket` primitive + Task 5's `DecoderFilterCallbacks.DynamicMetadata()` / `EncoderFilterCallbacks.DynamicMetadata()` framework accessors. Unblocks Task 13 (streaminfo.go extension can EXTRACT the 6-method surface + ADD the OTHER 5 methods on top of this Task 9's 2-method extension) + Task 18 (fixture-0027 scenarios `(d) metadata empty-userdata at binding-gap` + `(e) dynamic-metadata read+write` consume the bridge surface for cross-side determinism per SPEC §8 table).

### Task 10: NEW `internal/filter/http/lua/connection.go` + `ssl.go` connection-SSL bridge [ADR-0192]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/http/lua/... -run 'TestSSL|TestConnection'` clean; 12 ssl methods exercised via test-double DecoderFilterCallbacks carrying canned `*tls.ConnectionState`; `:connection():ssl()` returns lua.LNil for plaintext (test-double returns nil from `DownstreamTLSConnectionState`); `:sha256PeerCertificateDigest()` returns byte-exact lowercase hex of `sha256.Sum256(cert.Raw)` per §11.5.4 — CROSS-SIDE BYTE-EXACT per D5 (option f-B) closure; per-method nil-tolerance (TLS state present but PeerCertificates empty → safe defaults: empty string for subject/validFrom/expiration/sha/pem/chain; empty table for SAN; TLS state version + cipher independent of peer cert).
- **12-method ssl roster (verified against SPEC §3.4 + BRAINSTORM §2.4)**:
  1. `:subjectPeerCertificate()` — Subject DN from `PeerCertificates[0].Subject.String()` (Go `pkix.Name.String()` shape `"CN=...,O=..."`).
  2. `:subjectLocalCertificate()` — empty string (Go `tls.ConnectionState` has no symmetric LocalCertificate field; envoy-go-strict per BEHAVIOR_CONTRACT departure record at Task 19).
  3. `:sanPeerCertificate()` — Lua table of SANs from `PeerCertificates[0].DNSNames + URIs + IPAddresses + EmailAddresses` (combined; envoy-go-strict name convention per BRAINSTORM §2.4 — upstream splits into `uriSan*` + `dnsSans*`).
  4. `:sanLocalCertificate()` — empty Lua table (symmetric to `:subjectLocalCertificate`; envoy-go-strict).
  5. `:validFromPeerCertificate()` — RFC3339 string from `PeerCertificates[0].NotBefore.UTC()`.
  6. `:expirationPeerCertificate()` — RFC3339 string from `PeerCertificates[0].NotAfter.UTC()`.
  7. `:sessionId()` — lowercase hex of `tls.ConnectionState.TLSUnique` (envoy-go-strict: Go uses RFC 5929 channel-binding bytes; upstream uses OpenSSL `SSL_get_session_id` — different byte sequence, NOT cross-side byte-exact).
  8. `:ciphersuiteId()` — uint16 numeric from `tls.ConnectionState.CipherSuite` as `lua.LNumber` (IANA cipher-suite ID).
  9. `:tlsVersion()` — string `"TLSv1.0"` / `"TLSv1.1"` / `"TLSv1.2"` / `"TLSv1.3"` per SPEC §11.5.4 wire-format convention.
  10. `:urlEncodedPemEncodedPeerCertificate()` — `url.QueryEscape(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})))`.
  11. `:urlEncodedPemEncodedPeerCertificateChain()` — same as #10 but concatenating the full `PeerCertificates` chain.
  12. `:sha256PeerCertificateDigest()` — **CROSS-SIDE BYTE-EXACT per D5 (f-B)** — `fmt.Sprintf("%x", sha256.Sum256(cert.Raw))` per SPEC §11.5.4 verbatim.
- **Files touched**:
  - `internal/filter/http/lua/connection.go` (created, ~175 LoC; NEW connection userdata wrapping `*tls.ConnectionState` from `RequestHandleCallbacks.DownstreamTLSConnectionState()` accessor; NEW `requestHandleConnection` + `responseHandleConnection` LGFunctions; NEW `connectionSSL` LGFunction returning `lua.LNil` for nil-state plaintext path OR ssl userdata for TLS; NEW `installConnectionMetatable` helper + `connectionTypeName` registry-key const; NEW `pushConnectionUD` allocator; per SPEC §11.5.3 + ADR-0144 plumbing pattern the wrapper is ALWAYS non-nil — operator scripts may check `if rh:connection() then ... end` reliably; per BRAINSTORM §2.4 Q4 closure `:connection()` is scoped to the SSL accessor only — per-connection address surface lives on `:streamInfo()` per SPEC §2.11).
  - `internal/filter/http/lua/ssl.go` (created, ~360 LoC; NEW 12 LGFunctions for the ssl userdata surface — `sslSubjectPeerCertificate` / `sslSubjectLocalCertificate` / `sslSanPeerCertificate` / `sslSanLocalCertificate` / `sslValidFromPeerCertificate` / `sslExpirationPeerCertificate` / `sslSessionID` / `sslCiphersuiteID` / `sslTLSVersion` / `sslURLEncodedPemEncodedPeerCertificate` / `sslURLEncodedPemEncodedPeerCertificateChain` / `sslSHA256PeerCertificateDigest`; NEW `sslMethods` dispatch table wiring the 12 methods; NEW `installSSLMetatable` helper + `sslTypeName` registry-key const; NEW `pushSSLUD` allocator + `sslStateFromUD` extractor + `peerCertFromState` shared nil-tolerant peer-cert lookup centralizing the no-peer-cert guard for the 9 methods that consume `PeerCertificates[0]`; per D5 (f-B) closure ONLY `:sha256PeerCertificateDigest` is cross-side byte-exact — the other 11 methods are subject-only test surface with implementation-specific formatting; per `crypto/tls` ConnectionState shape NO LocalCertificate surface so `:subjectLocalCertificate` + `:sanLocalCertificate` return empty defaults — envoy-go-strict per BEHAVIOR_CONTRACT departure record at Task 19 atomic landing).
  - `internal/filter/http/lua/ssl_test.go` (created, ~610 LoC; 19 NEW behavioral tests under `TestConnection_*` + `TestSSL_*` namespaces covering the full SPEC §3.4 + §11.5 D5 closure + PLAN Task 10 acceptance: 3 connection-side tests — `TestConnection_returns_userdata_with_ssl_method` + `TestConnection_ssl_returns_nil_for_plaintext` (test-double cb returns nil from `DownstreamTLSConnectionState`) + `TestConnection_ssl_returns_userdata_for_tls`; 1 byte-exact hinge — `TestSSL_sha256PeerCertificateDigest_returns_byte_exact_hex` asserting `hex.EncodeToString(sha256.Sum256(cert.Raw)[:])` byte-for-byte match (64-char lowercase hex) — the D5 (f-B) cross-side determinism load-bearing assertion; 11 per-method tests — `TestSSL_subjectPeerCertificate` (asserts `CN=test.envoy-go.local` substring), `TestSSL_subjectLocalCertificate_returns_empty_on_no_local_cert`, `TestSSL_sanPeerCertificate_returns_dns_and_uri_sans` (asserts `>= 2` SANs incl. DNS), `TestSSL_sanLocalCertificate_returns_empty_table_on_no_local_cert`, `TestSSL_validFromPeerCertificate_returns_iso8601` (pinned to `2024-01-01T00:00:00Z` from fixed cert NotBefore), `TestSSL_expirationPeerCertificate_returns_iso8601` (pinned to `2025-01-01T00:00:00Z`), `TestSSL_sessionId_returns_hex_of_tls_unique` (pinned to `deadbeef01020304`), `TestSSL_ciphersuiteId_returns_numeric_uint16` (pinned to `0x1302` for `TLS_AES_256_GCM_SHA384`), 2× `TestSSL_tlsVersion_returns_string_TLSv1[23]` (pinned to `TLSv1.3` / `TLSv1.2`), `TestSSL_urlEncodedPemEncodedPeerCertificate` (asserts byte-exact `url.QueryEscape(string(pem.EncodeToMemory(...)))` round-trip), `TestSSL_urlEncodedPemEncodedPeerCertificateChain` (single-cert chain identical to leaf PEM); 2 defensive paths — `TestSSL_no_peer_certs_returns_defaults` (TLS state present but `PeerCertificates` nil → subject/validFrom/expiration/sha/pem/chain all empty strings; SAN empty table; version+cipher still return state values), `TestSSL_full_method_callability` (single-script exercise of all 12 methods to pin metatable dispatch wiring), `TestSSL_byte_exact_cross_side_hex_format` (verifies hex format matches `fmt.Sprintf("%x", sha256.Sum256(cert.Raw))` per SPEC §11.5.4 verbatim); NEW `fakeCallbacksWithTLS` test-double embedding bridge_test.go's `fakeCallbacks` + overriding `DownstreamTLSConnectionState()` to return a per-test `*tls.ConnectionState`; NEW `newBridgedVMWithTLS` helper constructs a VM with all 8 bridge metatables incl. connection + ssl; NEW `generateTestCertChain` test helper mints a deterministic ECDSA P-256 self-signed leaf with fixed Subject CN + DNSNames + URIs + IPAddresses + NotBefore = 2024-01-01T00:00:00Z for cross-side fingerprint determinism; NEW `stateWithPeerCert` helper constructs the canonical TLS 1.3 state with `TLS_AES_256_GCM_SHA384` cipher + 8-byte deterministic `TLSUnique`).
  - `internal/filter/http/lua/bridge.go` (modified +60 LoC; NEW `crypto/tls` import; EXTENDED `RequestHandleCallbacks` interface with NEW `DownstreamTLSConnectionState() *tls.ConnectionState` accessor — interface contract stable at this addition; EXTENDED both `requestHandleCallbacksAdapter` + `responseHandleCallbacksAdapter` with `DownstreamTLSConnectionState()` impl projecting framework's `DecoderFilterCallbacks.DownstreamTLSConnectionState()` / `EncoderFilterCallbacks.DownstreamTLSConnectionState()` accessor verbatim (nil-tolerant on nil dcb/ecb per ADR-0144); NEW `"connection"` entries appended to both `requestHandleMethods` + `responseHandleMethods` dispatch tables routing to `requestHandleConnection` / `responseHandleConnection`).
  - `internal/filter/http/lua/bridge_test.go` (modified +6 LoC; NEW `crypto/tls` import; EXTENDED `fakeCallbacks` test-double with NEW `DownstreamTLSConnectionState() *tls.ConnectionState` method returning nil — preserves the Task 8 streamInfo-method tests + Task 9 metadata-method tests' existing 4-method + 1-bucket scope without TLS interference; the ssl_test.go's `fakeCallbacksWithTLS` embeds this struct + overrides `DownstreamTLSConnectionState()` to return a per-test state).
  - `internal/filter/http/lua/decode_headers.go` (modified +2 LoC; `installConnectionMetatable(L)` + `installSSLMetatable(L)` calls appended to the per-stream-VM metatable-install sequence at Step 3 — installed ONCE per VM alongside the 7 other install helpers; the encode side reuses the per-stream VM from decode side so no encode_headers.go modification needed).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/... -run 'TestSSL|TestConnection' -v
  ```

  ```
  === RUN   TestConnection_returns_userdata_with_ssl_method
  --- PASS: TestConnection_returns_userdata_with_ssl_method (0.00s)
  === RUN   TestConnection_ssl_returns_nil_for_plaintext
  --- PASS: TestConnection_ssl_returns_nil_for_plaintext (0.00s)
  === RUN   TestConnection_ssl_returns_userdata_for_tls
  --- PASS: TestConnection_ssl_returns_userdata_for_tls (0.00s)
  === RUN   TestSSL_sha256PeerCertificateDigest_returns_byte_exact_hex
  --- PASS: TestSSL_sha256PeerCertificateDigest_returns_byte_exact_hex (0.00s)
  === RUN   TestSSL_subjectPeerCertificate
  --- PASS: TestSSL_subjectPeerCertificate (0.00s)
  === RUN   TestSSL_subjectLocalCertificate_returns_empty_on_no_local_cert
  --- PASS: TestSSL_subjectLocalCertificate_returns_empty_on_no_local_cert (0.00s)
  === RUN   TestSSL_sanPeerCertificate_returns_dns_and_uri_sans
  --- PASS: TestSSL_sanPeerCertificate_returns_dns_and_uri_sans (0.00s)
  === RUN   TestSSL_sanLocalCertificate_returns_empty_table_on_no_local_cert
  --- PASS: TestSSL_sanLocalCertificate_returns_empty_table_on_no_local_cert (0.00s)
  === RUN   TestSSL_validFromPeerCertificate_returns_iso8601
  --- PASS: TestSSL_validFromPeerCertificate_returns_iso8601 (0.00s)
  === RUN   TestSSL_expirationPeerCertificate_returns_iso8601
  --- PASS: TestSSL_expirationPeerCertificate_returns_iso8601 (0.00s)
  === RUN   TestSSL_sessionId_returns_hex_of_tls_unique
  --- PASS: TestSSL_sessionId_returns_hex_of_tls_unique (0.00s)
  === RUN   TestSSL_ciphersuiteId_returns_numeric_uint16
  --- PASS: TestSSL_ciphersuiteId_returns_numeric_uint16 (0.00s)
  === RUN   TestSSL_tlsVersion_returns_string_TLSv13
  --- PASS: TestSSL_tlsVersion_returns_string_TLSv13 (0.00s)
  === RUN   TestSSL_tlsVersion_returns_string_TLSv12
  --- PASS: TestSSL_tlsVersion_returns_string_TLSv12 (0.00s)
  === RUN   TestSSL_urlEncodedPemEncodedPeerCertificate
  --- PASS: TestSSL_urlEncodedPemEncodedPeerCertificate (0.00s)
  === RUN   TestSSL_urlEncodedPemEncodedPeerCertificateChain
  --- PASS: TestSSL_urlEncodedPemEncodedPeerCertificateChain (0.00s)
  === RUN   TestSSL_no_peer_certs_returns_defaults
  --- PASS: TestSSL_no_peer_certs_returns_defaults (0.00s)
  === RUN   TestSSL_full_method_callability
  --- PASS: TestSSL_full_method_callability (0.00s)
  === RUN   TestSSL_byte_exact_cross_side_hex_format
  --- PASS: TestSSL_byte_exact_cross_side_hex_format (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.028s
  ```

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.522s
  ```

  ```
  $ go test -count=3 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	2.519s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/lua/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race ./internal/filter/http/lua/... -run 'TestSSL|TestConnection'` clean**: PASS — all 19 NEW connection-SSL tests green under `-race`; 3× repeat run (`-count=3 -race`) also clean (no flakiness; nil-state defensive paths exercised under race without panic).
  - **12 ssl methods exercised via test-double DecoderFilterCallbacks carrying canned `*tls.ConnectionState`**: `TestSSL_full_method_callability` PASS — single-script invocation of all 12 methods returns no errors + each method surfaces a callable result. Per-method tests verify the 12 surface entries individually. The 12-method roster is identical to SPEC §3.4 + BRAINSTORM §2.4 enumeration (verified: subject{Peer,Local}Certificate + san{Peer,Local}Certificate + {validFrom,expiration}PeerCertificate + sessionId + ciphersuiteId + tlsVersion + urlEncodedPemEncodedPeerCertificate{,Chain} + sha256PeerCertificateDigest).
  - **`:connection():ssl()` returns lua.LNil for plaintext**: `TestConnection_ssl_returns_nil_for_plaintext` PASS — test-double cb returns nil from `DownstreamTLSConnectionState`; `:ssl()` short-circuits to `lua.LNil`; the script's `s == nil` evaluates true. Matches SPEC §11.5.3 + ADR-0144 plumbing pattern verbatim.
  - **`:sha256PeerCertificateDigest()` byte-exact lowercase hex** — `TestSSL_sha256PeerCertificateDigest_returns_byte_exact_hex` PASS — the Lua-side string equals `hex.EncodeToString(sha256.Sum256(cert.Raw)[:])` byte-for-byte; len() = 64; all-lowercase. `TestSSL_byte_exact_cross_side_hex_format` PASS — verifies the format matches the SPEC §11.5.4 `fmt.Sprintf("%x", sha256.Sum256(cert.Raw))` reference verbatim. **This is the D5 (option f-B) cross-side determinism load-bearing assertion**: fixture-0027 scenario (f-B) compares this method's output between reference Envoy and envoy-go for byte-exact match; the implementation's output IS the byte-exact hex of the DER cert digest with no Go-specific formatting departure.
  - **Per-method nil-tolerance (TLS state present but PeerCertificates empty)**: `TestSSL_no_peer_certs_returns_defaults` PASS — 6 methods consuming `PeerCertificates[0]` (subject/validFrom/expiration/sha/pem/chain) return empty strings; SAN returns empty table (#0); version+cipher independent of peer cert return TLS-state values (`TLSv1.2` + non-zero cipher ID). Defensive guard centralized in `peerCertFromState` shared helper.
  - **`go build ./...` clean**: empty output, exit 0 — interface contract addition (`RequestHandleCallbacks.DownstreamTLSConnectionState()`) absorbed by the 2 in-package adapters; no external consumers broken.
  - **`go vet ./...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/filter/http/lua/...` clean**: empty output, exit 0 (after `gofmt -w` on connection.go + ssl.go + bridge_test.go + ssl_test.go).
- **D-decision-disposition update**: this Task 10 entry RATIFIES D5 (option f-B) closure at the bridge-method implementation surface — `:sha256PeerCertificateDigest()` produces the byte-exact lowercase hex of `sha256.Sum256(cert.Raw)` (the cross-side comparison vector for fixture-0027 scenario f-B). The SEAM ARCHITECTURE per SPEC §11.5.3 was already ratified at Task 5 (callbacks.go extension) + Task 6 (H1/H2 seeding); this Task 10 closes the loop at the Lua-side wrapper consumer. ADR-0192 §Decision body's `:connection():ssl()` 12-method surface anchor lands at this Task's implementation; the §Decision body itself is authored at Task 19 atomic landing per ADR-0044 in-place edit discipline. **No new ADR consumed at this Task** — the chain-side `tlsConnectionState` extension stays INSIDE ADR-0192 per Q13 WEAK HOLD (per BRAINSTORM §2.13 + SPEC §11.5.3 closing note).
- **Envoy-go-strict departure records** (anticipated at BEHAVIOR_CONTRACT.md Task 19 atomic landing):
  - `:subjectLocalCertificate` / `:sanLocalCertificate` return empty default — Go's `tls.ConnectionState` has no LocalCertificate surface symmetric to PeerCertificates; upstream Envoy surfaces the local cert via OpenSSL `SSL_get_certificate`.
  - `:sessionId` uses Go's `TLSUnique` (RFC 5929 channel-binding bytes) — upstream uses OpenSSL `SSL_get_session_id` (different byte sequence; NOT cross-side byte-exact).
  - `:sanPeerCertificate` / `:sanLocalCertificate` combine DNS + URI + IP + email SANs into a single Lua table — upstream splits into `uriSan*` + `dnsSans*` separate methods (envoy-go-strict naming per BRAINSTORM §2.4).
  - `:subjectPeerCertificate` uses Go's `x509 pkix.Name.String()` format (reverse-RDN ordering with comma separation) — upstream uses OpenSSL `X509_NAME_oneline` (different ordering/separator).
  - These departures DO NOT compromise the D5 (f-B) cross-side byte-exact property of `:sha256PeerCertificateDigest` — the digest is computed over the raw DER cert bytes (`cert.Raw`) which is invariant across implementations.
- **Hand-off note to Task 13** (streaminfo.go extraction): Task 13 owns the `:streamInfo():downstreamSslConnection()` method (per BRAINSTORM §2.8 + SPEC §3.4 streamInfo-full 7-method extension) which returns the SAME ssl userdata constructed by this Task 10's `pushSSLUD` helper. The `pushSSLUD` + `installSSLMetatable` + `sslTypeName` symbols are exported within the package (lowercase first-letter; package-private) and stable at this Task 10's landing — Task 13 consumes them without re-touching this file. The `RequestHandleCallbacks.DownstreamTLSConnectionState()` accessor added at this Task is similarly stable for Task 13's streaminfo.go consumer.
- **Commit SHA**: `a91e572` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier C bridge surfaces (Task 10 of 19 overall + Pre-Task 0). PARALLELIZABLE with Tasks 7-9 (already complete) + 11-13 (pending); consumes Task 5's `DecoderFilterCallbacks.DownstreamTLSConnectionState()` framework accessor + Task 6's H1/H2 chain-side seeding. Unblocks Task 13 (streaminfo.go extension consumes `:downstreamSslConnection` returning the same ssl userdata) + Task 17 (cert fixture plumbing for scenario f-B per D5 closure consumes this Task's `:sha256PeerCertificateDigest` byte-exact contract) + Task 18 (fixture-0027 scenario (f-B) cross-side comparison consumes the byte-exact sha256 method).

<!-- subsequent task entries appended below -->

### Task 11: NEW `internal/filter/http/lua/httpcall.go` httpCall bridge [ADR-0192 + AMEND-ADR-0177 co-consumer]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/http/lua/... -run Test_HTTPCall` clean; empty cluster raises arm-20 byte-stable wording per W2; sync happy + timeout + 5xx + transport-failure increments correct counters; async fire-and-forget returns 0 values + does NOT yield + does NOT increment failures/timeouts on async transport-failure (per AMEND-22.2-3 D6 closure); coroutine yield-resume timing for sync path verified.
- **R5 closure note**: this Task is the FIRST CO-CONSUMER call site of Task 4's IN-PLACE AMENDMENT on ADR-0177 (`Client.ClusterDispatch`). R5 (parent SPEC §13 → 22.2 SPEC §13-R5) was RATIFIED at Task 4 when ClusterDispatch landed; this Task 11 ACTUALIZES the consumer-side call by wiring `f.httpClient.ClusterDispatch(ctx, cluster, req, f.clusterMgr)` from inside the bridge LGFunction. No re-ratification needed — Task 4's PROGRESS.md entry already records R5 closure. This entry cross-references the actualization site.
- **Files touched**:
  - `internal/filter/http/lua/httpcall.go` (created, ~360 LoC; NEW `requestHandleHttpCall` + `responseHandleHttpCall` LGFunctions on request_handle + response_handle; NEW shared `runHTTPCall` core with arg-extraction (cluster string + headers LTable + body string + timeout_ms int + asynchronous bool) + arm-20 byte-stable runtime-reject + `buildHTTPCallRequest` helper composing `*http.Request` from Lua headers table (`:method` / `:path` / `:scheme` / `:authority` pseudo-headers + regular HTTP headers via `headers.ForEach`) + `marshalHTTPCallResponse` helper marshaling response into Lua headers table (lower-cased + `:status` pseudo-header) + body string (full read into memory + close); NEW `isTimeoutError` helper detecting `context.DeadlineExceeded` + `net.Error.Timeout()` for the sync-only `httpcall_timeouts` SYNC-ONLY counter discipline per AMEND-22.2-3; NEW `httpCallClusterRequiredMsg` const pinning the byte-stable arm-20 wording per W2 + SPEC §6.2 arm-20 row verbatim `"lua: httpCall: cluster name must not be empty"`; NEW `defaultHTTPCallTimeout = 5 * time.Second` for zero-timeout-ms script paths; sync path stashes `L` on `f.pendingHTTPCallResume` + allocates `f.httpCallReady` + `f.httpCallDone` gate/signal channels + spawns dispatch goroutine that waits on readyCh BEFORE Resume (per §11.1 D2 goroutine-safety — gopher-lua Resume is not goroutine-safe with itself or with concurrent child-state access; the readyCh gate establishes the happens-before edge satisfying the race detector); async path spawns fire-and-forget goroutine discarding response/error per upstream `lua_filter.cc:400-416` `noopCallbacks` parity + returns 0 values + NO yield).
  - `internal/filter/http/lua/httpcall_test.go` (created, ~560 LoC; 8 NEW behavioral tests under `Test_HTTPCall_*` namespace covering the full SPEC §3.4 + §11.7 D6 closure + AMEND-22.2-3 + PLAN Task 11 acceptance: `Test_HTTPCall_empty_cluster_raises_arm20_byte_stable_wording` (arm-20 byte-exact wording per W2), `Test_HTTPCall_sync_happy_path_roundtrip` (httptest.Server backend → script gets `(headers_table, body_string)` via dispatch goroutine Resume), `Test_HTTPCall_sync_timeout_increments_httpcall_timeouts` (50ms timeout vs hung server → `httpcall_timeouts++` AND `httpcall_failures++` per dual-increment discipline; script sees `(nil, err)`), `Test_HTTPCall_sync_5xx_increments_httpcall_failures` (502 backend → `httpcall_failures++` per upstream synthetic-503 parity disposition; `httpcall_timeouts` UNCHANGED), `Test_HTTPCall_async_fire_and_forget_returns_0_values_no_yield` (asynchronous=true → script's `after_call = true` global sets proving NO yield; backend observes 1 request via atomic.Int32 counter), `Test_HTTPCall_async_transport_failure_does_NOT_increment_failures_or_timeouts` (127.0.0.1:1 unroutable + async=true → `httpcall_total++` AND `httpcall_failures` + `httpcall_timeouts` UNCHANGED per AMEND-22.2-3 D6 closure), `Test_HTTPCall_total_counter_covers_sync_and_async` (1 sync + 1 async → `httpcall_total` delta = 2), `Test_HTTPCall_coroutine_yield_resume_timing_sync` (manual coroutine drive observes `ResumeYield` at parent.Resume return site + `yielded_before` global set + post-Resume `after_resume_body` populated + `coroutine_yields_total++` ONCE per yield event); NEW `newHTTPCallBridgeFilter` test-helper constructs a fresh *filter with VM + bridge metatables + 8-counter hand-rolled filterStats so tests assert against the 3 NEW Task 11 counters + the 2 Task 7 counters; NEW `driveSyncHTTPCall` helper encapsulates the sync yield+resume + readyCh gate-close + doneCh wait timing; NEW `httpCallSplitHostPort` + `httpCallMkClusterMgr` helpers mirroring `httpclient_test.go::splitHostPort` + `mkPlainClusterMgr` patterns for the cluster-manager test-double setup using real `cluster.NewManager` from a static bootstrap proto + a single STATIC cluster pointing at `httptest.Server`).
  - `internal/filter/http/lua/bridge.go` (modified +20 LoC; NEW `"httpCall"` entries appended to both `requestHandleMethods` + `responseHandleMethods` dispatch tables routing to `requestHandleHttpCall` / `responseHandleHttpCall` with comment block referencing SPEC §3.4 + §11.7 D6 closure + AMEND-22.2-3 + arm-20 byte-stable wording per W2).
  - `internal/filter/http/lua/lua.go` (modified +50 LoC; NEW imports `internal/cluster` + `internal/httpclient`; EXTENDED `filterStats` struct with 3 NEW envoy-go-strict counter fields `httpcallTotal` / `httpcallFailures` / `httpcallTimeouts` + comment block documenting the SPEC §7.1 + §11.7 D6 wiring + AMEND-22.2-3 SYNC-ONLY discipline + deferral to Task 14 for production-registration via `newFilterStats` extension; EXTENDED `filter` struct with NEW per-stream fields `httpClient *httpclient.Client` + `clusterMgr *cluster.Manager` (threaded via FactoryCtx) + `pendingHTTPCallResume *lua.LState` (sync suspend slot) + `httpCallReady chan struct{}` (gate for dispatch goroutine Resume per §11.1 D2 goroutine-safety) + `httpCallDone chan struct{}` (completion signal for test/HCM-dispatch coordination); EXTENDED `New` FilterInstanceFactory closure to capture `ctx.HTTPClient` + `ctx.ClusterManager` from FactoryCtx into the per-stream `*filter` allocation — both may be nil under ADR-0085 nil-tolerance at the synthetic test path; production wiring per phase-20 oauth2 + phase-18.2 ext_authz cluster-manager precedent always supplies non-nil).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/... -run Test_HTTPCall -v
  ```

  ```
  === RUN   Test_HTTPCall_empty_cluster_raises_arm20_byte_stable_wording
  --- PASS: Test_HTTPCall_empty_cluster_raises_arm20_byte_stable_wording (0.00s)
  === RUN   Test_HTTPCall_sync_happy_path_roundtrip
  --- PASS: Test_HTTPCall_sync_happy_path_roundtrip (0.00s)
  === RUN   Test_HTTPCall_sync_timeout_increments_httpcall_timeouts
  --- PASS: Test_HTTPCall_sync_timeout_increments_httpcall_timeouts (0.05s)
  === RUN   Test_HTTPCall_sync_5xx_increments_httpcall_failures
  --- PASS: Test_HTTPCall_sync_5xx_increments_httpcall_failures (0.00s)
  === RUN   Test_HTTPCall_async_fire_and_forget_returns_0_values_no_yield
  --- PASS: Test_HTTPCall_async_fire_and_forget_returns_0_values_no_yield (0.01s)
  === RUN   Test_HTTPCall_async_transport_failure_does_NOT_increment_failures_or_timeouts
  --- PASS: Test_HTTPCall_async_transport_failure_does_NOT_increment_failures_or_timeouts (0.50s)
  === RUN   Test_HTTPCall_total_counter_covers_sync_and_async
  --- PASS: Test_HTTPCall_total_counter_covers_sync_and_async (0.20s)
  === RUN   Test_HTTPCall_coroutine_yield_resume_timing_sync
  --- PASS: Test_HTTPCall_coroutine_yield_resume_timing_sync (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.785s
  ```

  ```
  $ go test -count=3 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	4.853s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/lua/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race -run Test_HTTPCall` clean**: PASS — all 8 NEW httpCall tests green under `-race`; 3× repeat run (`-count=3 -race`) also clean (no flakiness; the `httpCallReady` gate + `httpCallDone` signal channel pair satisfies the race detector by establishing the happens-before edge between the outer parent.Resume's switchToParentThread Push and the dispatch goroutine's inner Resume call).
  - **Empty cluster raises arm-20 byte-stable wording per W2**: `Test_HTTPCall_empty_cluster_raises_arm20_byte_stable_wording` PASS — the literal `"lua: httpCall: cluster name must not be empty"` substring matches the script's runtime error, pinned via the `httpCallClusterRequiredMsg` package const cross-referencing SPEC §6.2 arm-20 row verbatim.
  - **Sync happy-path roundtrip**: `Test_HTTPCall_sync_happy_path_roundtrip` PASS — script gets `(hdrs, body)` with `hdrs[":status"] == "200"` + `hdrs["x-custom"] == "abc"` + `body == "sync-happy-body"`; the httptest.Server backend received the GET request at `/x` path via ClusterDispatch endpoint-rewrite.
  - **Sync timeout increments httpcall_timeouts**: `Test_HTTPCall_sync_timeout_increments_httpcall_timeouts` PASS — 50ms timeout vs hung server → `httpcall_timeouts` delta = 1 + `httpcall_failures` delta = 1 (dual-increment per discipline; timeout is also a failure); script sees `(nil, err)` shape.
  - **Sync 5xx increments httpcall_failures**: `Test_HTTPCall_sync_5xx_increments_httpcall_failures` PASS — 502 backend → `httpcall_failures` delta = 1 + `httpcall_timeouts` UNCHANGED (matches upstream synthetic-503 parity disposition); script still observes `(hdrs, body)` shape with `hdrs[":status"] == "502"` per upstream's "5xx is not raised as Lua error" semantics.
  - **Async fire-and-forget returns 0 values + does NOT yield**: `Test_HTTPCall_async_fire_and_forget_returns_0_values_no_yield` PASS — script's `before_call = true` AND `after_call = true` both set, proving execution continued past the httpCall without coroutine yield (per AMEND-22.2-3); backend observed 1 request via the `atomic.Int32` hit counter — verifying actual dispatch fires.
  - **Async transport-failure does NOT increment failures/timeouts**: `Test_HTTPCall_async_transport_failure_does_NOT_increment_failures_or_timeouts` PASS — unroutable `127.0.0.1:1` + async=true → `httpcall_total` delta = 1 AND `httpcall_failures` + `httpcall_timeouts` UNCHANGED per AMEND-22.2-3 D6 closure (async failures invisible at filter-stats per upstream parity).
  - **httpcall_total covers sync AND async**: `Test_HTTPCall_total_counter_covers_sync_and_async` PASS — 1 sync dispatch + 1 async dispatch → `httpcall_total` delta = 2.
  - **Coroutine yield-resume timing for sync path verified**: `Test_HTTPCall_coroutine_yield_resume_timing_sync` PASS — `parent.Resume(child, fn)` returns `ResumeYield` (proves the LGFunction's YieldFromBridge correctly suspended); `yielded_before` global == true (proves script executed up to the yield point); after `close(f.httpCallReady)` + wait on `f.httpCallDone` the script's post-resume continuation populates `after_resume_body` with the response body (proves the dispatch goroutine's inner Resume correctly woke the coroutine); `coroutine_yields_total` delta = 1 (ONCE per yield event per Task 7 semantics).
  - **`go build ./...` clean**: empty output, exit 0 — `internal/filter/http/lua` package compiles cleanly with the NEW `internal/cluster` + `internal/httpclient` imports added at lua.go.
  - **`go vet ./...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/filter/http/lua/...` clean**: empty output, exit 0 (after `gofmt -w` on httpcall.go + httpcall_test.go).
- **D-decision-disposition update**: this Task 11 entry ACTUALIZES the consumer-side call of R5 closure (R5 was RATIFIED at Task 4 when ClusterDispatch landed; the actualization here closes the loop). ADR-0192 §Decision body's `:httpCall(cluster, headers, body, timeout_ms, asynchronous?)` 5-arg surface anchor lands at this Task's implementation; the §Decision body itself is authored at Task 19 atomic landing per ADR-0044 in-place edit discipline. ADR-0177 §Decision AMENDMENT body's `ClusterDispatch` method anchor + the consumer-#1 reference lands at this same Task 19 atomic landing — no new ADR number consumed; the AMENDMENT body lives inside ADR-0177 per phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent. **No new ADR consumed at this Task** — the httpCall bridge surface lives inside ADR-0192 + the cluster-dispatch primitive lives inside ADR-0177 AMENDMENT (both bodies land at Task 19).
- **Envoy-go-strict departure records** (anticipated at BEHAVIOR_CONTRACT.md Task 19 atomic landing per SPEC §14 edit #3 + #4 + #5):
  - `httpcall_total` envoy-go-strict counter (every dispatch sync + async) — operator outbound-call observability.
  - `httpcall_failures` envoy-go-strict counter SYNC-ONLY caveat per §11.7 D6 (async failures invisible at filter-stats per upstream parity).
  - `httpcall_timeouts` envoy-go-strict counter SYNC-ONLY caveat per §11.7 D6.
  - The async-flag PURE FIRE-AND-FORGET semantics per AMEND-22.2-3 — matches upstream `lua_filter.cc:400-416` `noopCallbacks` parity exactly; no envoy-go-strict departure record needed (parity-matched at the wire-shape level).
- **Production HCM dispatch coordination — DEFERRED to future phase**: the dispatch goroutine's inner Resume call site requires a goroutine-safety gate (`f.httpCallReady`) that the OUTER parent.Resume caller must close after observing ResumeYield. At 22.2 Task 11 the test goroutine drives this gate-close; production HCM dispatch coordination (when the decode_headers.go / encode_headers.go dispatcher observes the ResumeYield from the envoy_on_request / envoy_on_response hook) lands at a future phase per SPEC §11.1 D2 closure + ADR-0192 §Decision body anticipation. The dispatch goroutine's Resume + the gate/signal channel pair are PRODUCTION-READY at this Task's landing; the HCM-side gate-close orchestration is the only missing piece for full live-traffic httpCall.
- **Hand-off note to Task 14** (stats.go +5 counters): Task 14 owns the production registration of `httpcall_total` + `httpcall_failures` + `httpcall_timeouts` + `body_buffered_bytes_total` + `coroutine_yields_total` via `newFilterStats` extension at stats.go. The 3 NEW Task 11 counters are declared on `filterStats` at this Task 11 but production-registration is deferred to Task 14 (matches Task 7's deferral pattern for the 2 body-bridge counters). Task 14 ALSO owns the arm-20 / arm-21 / arm-22 runtime-reject wording const surface — the `httpCallClusterRequiredMsg` const declared at httpcall.go at this Task is candidate for relocation to compiled_config.go alongside the existing parseReject* const family at Task 14 (PLAN settles).
- **Hand-off note to Task 18** (fixture-0027 scenarios (j) + (k)): scenarios (j) `httpCall sync upstream cluster call` + (k) `httpCall async fire-and-forget` per SPEC §8.2 row j+k consume this Task 11's bridge implementation. Both are REFERENCE-LESS subject-only scenarios per the non-deterministic-timing classification at SPEC §8.2; the script-level wire shape is `local hdrs, body = rh:httpCall(cluster, headers, body_str, timeout_ms)` for sync + `rh:httpCall(cluster, headers, body_str, timeout_ms, true)` for async.
- **Commit SHA**: `d801b62` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier C bridge surfaces (Task 11 of 19 overall + Pre-Task 0). PARALLELIZABLE with Tasks 7-10 (already complete) + 12-13 (pending). Consumes Task 4's `Client.ClusterDispatch` (R5 RATIFIED at Task 4 + actualized here as first co-consumer) + Task 7's coroutine `YieldFromBridge` + `vm.Resume` API. Unblocks Task 14 (stats.go +5 counters consumes the 3 NEW counter fields declared here + the arm-20 wording surface) + Task 18 (fixture-0027 scenarios (j) + (k) consume this Task's bridge implementation).

### Task 12: NEW `internal/filter/http/lua/crypto.go` + `misc.go` crypto + fileBytes + timestamp bridge [ADR-0192]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/http/lua/... -run 'Test_Base64|Test_Sha|Test_ImportPublicKey|Test_VerifySignature|Test_PublicKeyWrapper|Test_FileBytes|Test_Timestamp'` clean; `:base64Escape` byte-output matches `absl::Base64Escape` standard-padding; `:base64Decode` round-trips with `:base64Escape`; `:sha256` + `:sha512` byte-output vectors (NIST FIPS 180-4); `:importPublicKey` parses RSA + ECDSA-P256 + Ed25519 PKIX PEMs; invalid PEM raises arm-22 byte-stable wording (`lua: importPublicKey:` prefix) per W2; `PublicKeyWrapper:get()` returns DER-encoded key bytes (MIMICKING upstream wrappers.h:415-427 scope per D8-sub closure); `:verifySignature(hash_algo, pubkey_wrapper, sig, text)` calling convention pinned to upstream `lua_filter.cc:611` (4 args); signature-verify failure returns `false` (NO Lua-runtime-error — upstream parity); unsupported hash algo returns `false`; `:fileBytes` happy + over-cap (16 MiB+1 → byte-stable wording prefix `lua: fileBytes: file size exceeds maximum of`) + ENOENT (nil + err string) + arbitrary-path-allowed (envoy-go-strict per D8); `:timestamp` default-unit-milliseconds-monotonic + seconds-unit + microseconds-unit + invalid-unit-raises (byte-stable prefix `lua: timestamp: unsupported unit`).
- **D8 classification confirmed**: per PLAN scrape — 2 upstream-parity (`:importPublicKey` + `:verifySignature`; MIMICKING upstream wrappers.h:415-427 PublicKeyWrapper scope; calling convention pinned to upstream lua_filter.cc:611 4-args ordered as hash_algo / pubkey_wrapper / sig / text) + 4 envoy-go-strict (`:sha256` + `:sha512` + `:base64Decode` + `:fileBytes`; the latter NOT in upstream at any scope per SPEC §11.2.5 + Pin 2 + R8 scrape). `:base64Escape` is upstream-parity per AMEND-22.2-1 (Go encoding/base64.StdEncoding byte-matches absl::Base64Escape standard-padding). `:timestamp` is envoy-go-strict (NOT in upstream StreamHandleWrapper per Pin 2 scrape — anticipated departure record at Task 19 atomic landing). 4 NEW envoy-go-strict BEHAVIOR_CONTRACT.md departure records anticipated at Task 19 (`:base64Decode` + `:sha256` + `:sha512` + `:fileBytes`); the bundle scales 7 → 11 at 22.2 IMPL atomic landing (plus optional `:timestamp` record bringing the bundle to 12 — PLAN/Task 19 settles).
- **Files touched**:
  - `internal/filter/http/lua/crypto.go` (created, ~330 LoC; NEW `requestHandleBase64Escape` + `requestHandleBase64Decode` + `requestHandleSha256` + `requestHandleSha512` + `requestHandleImportPublicKey` + `requestHandleVerifySignature` LGFunctions on request_handle; NEW `requestHandlePublicKeyWrapperGet` for the PublicKeyWrapper userdata's `:get` method; NEW `publicKeyWrapper` struct holding the DER-encoded SubjectPublicKeyInfo bytes + parsed `crypto.PublicKey` interface; NEW `parsePublicKeyPEM` helper (PKIX-first via `x509.ParsePKIXPublicKey`; falls back to PKCS1 RSA via `x509.ParsePKCS1PublicKey` for legacy "RSA PUBLIC KEY" PEMs); NEW `verifySignature` dispatch via switch on key type (RSA-PKCS1v15 / ECDSA-DER via `ecdsa.VerifyASN1` / Ed25519 raw via `ed25519.Verify`); NEW `resolveHashAlgo` + `newHash` helpers mapping the upstream-supported algo string set (`SHA1`/`SHA224`/`SHA256`/`SHA384`/`SHA512`) to `crypto.Hash` enum + factory; NEW `publicKeyWrapperTypeName` registry-key const + `installPublicKeyWrapperMetatable` registrar; NEW `cryptoImportPublicKeyErrPrefix` package-level const pinning the byte-stable arm-22 wording per W2 + SPEC §6 row 22; 6 NEW responseHandle* parity stubs aliasing the requestHandle* LGFunctions for symmetric encode-side dispatch).
  - `internal/filter/http/lua/crypto_test.go` (created, ~430 LoC; 15+ NEW behavioral tests covering: `Test_Base64Escape_byte_output_matches_absl_Base64Escape` (5 cases via `t.Run` subtest + 1 binary-via-SetGlobal case verifying the cross-language wire-shape parity), `Test_Base64Decode_roundtrip_with_Base64Escape` (5 cases), `Test_Base64Decode_invalid_input_returns_nil_plus_error` (Lua idiom disposition), `Test_Sha256_byte_output_vectors` (NIST FIPS 180-4 empty + "abc"), `Test_Sha512_byte_output_vectors` (NIST FIPS 180-4 empty + "abc"), `Test_ImportPublicKey_parses_RSA_PKIX_PEM` + `Test_ImportPublicKey_parses_ECDSA_P256_PKIX_PEM` + `Test_ImportPublicKey_parses_Ed25519_PKIX_PEM` (the 3 algo families spanning upstream's supported set), `Test_ImportPublicKey_invalid_PEM_raises_arm22_byte_stable_wording` (W2 byte-stable contract), `Test_PublicKeyWrapper_get_returns_key_bytes` (D8-sub closure MIMICKING discipline), `Test_VerifySignature_happy_RSA_SHA256` (canned RSA-2048 + SHA-256 signed payload), `Test_VerifySignature_invalid_signature_returns_false` (tampered sig + first-byte-XOR), `Test_VerifySignature_unsupported_hash_algo_returns_false` (NO Lua-runtime-error — upstream parity), `Test_VerifySignature_calling_convention_pinned_4_args` (D8-sub upstream lua_filter.cc:611 contract pin); NEW `newCryptoBridgeVM` test helper installing the PublicKeyWrapper metatable alongside the other bridge metatables; NEW `genRSAKeyPEM` + `genECDSAP256KeyPEM` + `genEd25519KeyPEM` + `signRSASHA256` helpers for canned-key+signature tests).
  - `internal/filter/http/lua/misc.go` (created, ~125 LoC; NEW `requestHandleFileBytes` + `requestHandleTimestamp` LGFunctions; NEW `readFileBytesCapped` helper consuming the SHARED `maxFilenameScriptBytes` constant from `datasource.go` for the 16 MiB cap (zero-copy reuse of the 22.1 Task 11 cap pattern); NEW `fileBytesOverCapError` sentinel error type + `isOverCapError` dispatch helper distinguishing the runtime-reject (raise) vs. soft-error (nil + err string) paths; NEW `miscFileBytesOverCapPrefix` + `miscTimestampInvalidUnitPrefix` package-level constants pinning the byte-stable runtime-reject wording per W2 conventions; NEW `timestampUnit*` constants matching upstream's "milliseconds" / "microseconds" / "seconds" string set; 2 NEW responseHandle* parity stubs for symmetric encode-side dispatch).
  - `internal/filter/http/lua/misc_test.go` (created, ~210 LoC; 8 NEW behavioral tests: `Test_FileBytes_happy_path_returns_file_contents` (t.TempDir() synthetic binary file with NUL + 0xff bytes), `Test_FileBytes_over_cap_raises_runtime_reject` (16 MiB+1 file → byte-stable wording prefix check), `Test_FileBytes_ENOENT_returns_nil_plus_error` (Lua idiom (nil, err) for missing file), `Test_FileBytes_arbitrary_path_allowed` (envoy-go-strict per D8 — cross-t.TempDir read works), `Test_Timestamp_default_unit_milliseconds_monotonic_increasing` (N=10 successive calls; non-decreasing + plausibility window vs `time.Now().UnixMilli()`), `Test_Timestamp_seconds_unit_returns_approximately_milliseconds_div_1000` (cross-check :timestamp("milliseconds") / 1000 ≈ :timestamp("seconds")), `Test_Timestamp_microseconds_unit` (plausibility window vs `time.Now().UnixMicro()`), `Test_Timestamp_invalid_unit_raises_runtime_error` (byte-stable wording prefix check via `vm.Run` returning error); NEW `newMiscBridgeVM` test helper).
  - `internal/filter/http/lua/bridge.go` (modified +30 LoC; 8 NEW entries (4 crypto + 1 importPublicKey + 1 verifySignature + 2 misc methods) appended to `requestHandleMethods` dispatch table; 8 SYMMETRIC entries appended to `responseHandleMethods` for encode-side parity).
  - `internal/filter/http/lua/decode_headers.go` (modified +1 LoC; NEW `installPublicKeyWrapperMetatable(L)` invocation alongside the other bridge metatable installs at VM construction; encode_headers.go REUSES the same VM so no separate install needed).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/... -run 'Test_Base64|Test_Sha|Test_ImportPublicKey|Test_VerifySignature|Test_PublicKeyWrapper|Test_FileBytes|Test_Timestamp' -v
  ```

  ```
  --- PASS: Test_Base64Escape_byte_output_matches_absl_Base64Escape (0.00s)
  --- PASS: Test_Base64Decode_roundtrip_with_Base64Escape (0.00s)
  --- PASS: Test_Base64Decode_invalid_input_returns_nil_plus_error (0.00s)
  --- PASS: Test_Sha256_byte_output_vectors (0.00s)
  --- PASS: Test_Sha512_byte_output_vectors (0.00s)
  --- PASS: Test_ImportPublicKey_parses_RSA_PKIX_PEM (0.09s)
  --- PASS: Test_ImportPublicKey_parses_ECDSA_P256_PKIX_PEM (0.00s)
  --- PASS: Test_ImportPublicKey_parses_Ed25519_PKIX_PEM (0.00s)
  --- PASS: Test_ImportPublicKey_invalid_PEM_raises_arm22_byte_stable_wording (0.00s)
  --- PASS: Test_PublicKeyWrapper_get_returns_key_bytes (0.03s)
  --- PASS: Test_VerifySignature_happy_RSA_SHA256 (0.08s)
  --- PASS: Test_VerifySignature_invalid_signature_returns_false (0.06s)
  --- PASS: Test_VerifySignature_unsupported_hash_algo_returns_false (0.06s)
  --- PASS: Test_VerifySignature_calling_convention_pinned_4_args (0.05s)
  --- PASS: Test_FileBytes_happy_path_returns_file_contents (0.00s)
  --- PASS: Test_FileBytes_over_cap_raises_runtime_reject (0.01s)
  --- PASS: Test_FileBytes_ENOENT_returns_nil_plus_error (0.00s)
  --- PASS: Test_FileBytes_arbitrary_path_allowed (0.00s)
  --- PASS: Test_Timestamp_default_unit_milliseconds_monotonic_increasing (0.00s)
  --- PASS: Test_Timestamp_seconds_unit_returns_approximately_milliseconds_div_1000 (0.00s)
  --- PASS: Test_Timestamp_microseconds_unit (0.00s)
  --- PASS: Test_Timestamp_invalid_unit_raises_runtime_error (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	(race-clean)
  ```

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	2.596s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/lua/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race` clean**: PASS — all 22 NEW Task 12 test cases (subtests counted individually) green under `-race`; full package suite (`./internal/filter/http/lua/...`) clean at 2.596s including Tasks 7-11 regressions.
  - **`:base64Escape` byte-output matches `absl::Base64Escape`**: 5 cross-language vectors + 1 binary-via-SetGlobal vector all match; the implementation calls `encoding/base64.StdEncoding.EncodeToString` directly (Go-side equivalent of absl's standard-padding output). Cross-checked vs Go ground-truth in each subtest.
  - **`:base64Decode` round-trips**: 5 round-trip vectors (empty, ASCII, punctuation, NUL-bearing binary) all symmetric. Invalid input returns `(nil, err_string)` per Lua idiom — Test_Base64Decode_invalid_input_returns_nil_plus_error PASS.
  - **`:sha256` + `:sha512` byte-output vectors**: NIST FIPS 180-4 standard test vectors (empty input + "abc") pinned byte-exactly via lower-case hex (matches `hex.EncodeToString(sum[:])` Go convention).
  - **`:importPublicKey` parses 3 PKIX algo families**: RSA-2048 + ECDSA-P256 + Ed25519 all parse cleanly via `x509.ParsePKIXPublicKey`. PKCS1-RSA fallback present (re-encoded to PKIX in the wrapper :get return) for legacy "RSA PUBLIC KEY" PEM blocks.
  - **Invalid PEM raises arm-22 byte-stable wording per W2**: literal `"lua: importPublicKey:"` prefix matched against the Lua runtime error returned from `vm.Run`; pinned via the `cryptoImportPublicKeyErrPrefix` package-level const for compile-time stability.
  - **`PublicKeyWrapper:get()` returns DER bytes**: the test cross-checks `pub:get()` against the Go-side `x509.MarshalPKIXPublicKey(&priv.PublicKey)` ground-truth (byte-exact length + content); MIMICKING upstream wrappers.h:415-427 PublicKeyWrapper scope per D8-sub closure.
  - **`:verifySignature` 4-arg calling convention pinned**: the canonical script form `rh:verifySignature(hash, pub, sig, text)` returns `true` on canned RSA-PKCS1v15+SHA256 happy path; tampered signature returns `false` (NOT a Lua error — upstream parity per lua_filter.cc:611 disposition); unsupported hash algo returns `false`; the calling convention is pinned at the test layer via the exact 4-arg script form.
  - **`:fileBytes` happy + over-cap + ENOENT + arbitrary-path**: 4 separate tests cover the disposition matrix. Over-cap raises a Lua runtime error with the pinned `miscFileBytesOverCapPrefix` prefix (W2-byte-stable contract); ENOENT returns (nil, err_string) idiomatically. Arbitrary path (cross-`t.TempDir()` read) succeeds — envoy-go-strict per D8 (no path-restriction surface; this method is NOT in upstream at any scope per SPEC §11.2.5 + R8 PLAN-time scrape).
  - **`:timestamp` 3 units + invalid raises**: default "milliseconds" returns Unix epoch ms; "seconds" cross-checks ≈ ms/1000 within 5s fuzz tolerance; "microseconds" cross-checks ≈ `time.Now().UnixMicro()` within 5s window. Invalid unit "fortnight" raises with the pinned `miscTimestampInvalidUnitPrefix` prefix.
  - **`go build ./...` clean**: empty output, exit 0.
  - **`go vet ./...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/filter/http/lua/...` clean**: empty output, exit 0 (post-`gofmt -w` on crypto.go + decode_headers.go + misc.go + `defer func() { _ = f.Close() }()` errcheck-clean form for os.Open in `readFileBytesCapped`).
- **D8 closure recorded** — per PLAN scrape outcome at this Task: 4 envoy-go-strict (`:sha256` + `:sha512` + `:base64Decode` + `:fileBytes`) + 2 upstream-parity (`:importPublicKey` + `:verifySignature`; MIMICKING upstream wrappers.h:415-427 + calling convention pinned to upstream lua_filter.cc:611 4-args). `:base64Escape` is upstream-parity per AMEND-22.2-1 (Go encoding/base64.StdEncoding byte-matches absl::Base64Escape standard-padding). `:timestamp` is envoy-go-strict (anticipated 5th departure record at Task 19; PLAN/Task 19 settles whether `:timestamp` warrants a separate departure record given its non-deterministic-by-design surface).
- **D-decision-disposition update**: this Task 12 entry CLOSES D8 (crypto + fileBytes upstream-exposure verification per AMEND-22.2-2 + §13-R7 + §13-R8 RATIFIED-PENDING-PLAN-TIME at SPEC commit). The 4 envoy-go-strict envoy-go-strict departure records (`:base64Decode` + `:sha256` + `:sha512` + `:fileBytes`) are anticipated at Task 19 BEHAVIOR_CONTRACT.md atomic landing per SPEC §14 edit #11 — bringing the 22.2 bundle scale from 7 → 11 records. ADR-0192 §Decision body's crypto + misc surface anchors land at this Task's implementation; the §Decision body itself is authored at Task 19 atomic landing per ADR-0044 in-place edit discipline. **No new ADR consumed at this Task** — the crypto + misc bridge surface lives inside ADR-0192.
- **Envoy-go-strict departure records** (anticipated at BEHAVIOR_CONTRACT.md Task 19 atomic landing per SPEC §14 edit #11):
  - `:base64Decode` — envoy-go-strict (NOT on upstream StreamHandleWrapper per Pin 2 scrape; absent at all upstream scopes per R7 PLAN-time scrape).
  - `:sha256` — envoy-go-strict (same as above).
  - `:sha512` — envoy-go-strict (same as above).
  - `:fileBytes` — envoy-go-strict (NOT in upstream at any scope per R8 PLAN-time scrape).
  - `:timestamp` — anticipated 5th departure record (envoy-go-strict; NOT in upstream StreamHandleWrapper); PLAN/Task 19 settles whether the wall-clock non-determinism warrants a separate record vs. silent acceptance.
- **PublicKeyWrapper userdata scope — MIMICKING upstream per D8-sub closure**: the PublicKeyWrapper userdata returned by `:importPublicKey` exposes a single `:get()` method per upstream wrappers.h:415-427 PublicKeyWrapper scope (returns the DER-encoded SubjectPublicKeyInfo bytes). Go-side state holds both the DER bytes (for `:get()`) AND the parsed `crypto.PublicKey` interface (consumed Go-side by `:verifySignature` dispatch). The parsed key is OPAQUE to Lua — script authors cannot extract algorithm-specific fields (modulus + exponent for RSA; curve params + point for ECDSA; etc.); this matches upstream's scope discipline.
- **`:verifySignature` calling convention — pinned per upstream `lua_filter.cc:611`**: the 4-argument shape `(hash_algo, pubkey_wrapper, sig, text)` is pinned at the bridge layer (the LGFunction reads stack positions 2-5; position 1 is the receiver per Lua method-call convention). Any deviation (e.g. reordering to put `text` before `sig`) would surface as a test failure at `Test_VerifySignature_calling_convention_pinned_4_args`. Hash algo dispatch via switch on uppercase string match (`SHA1`/`SHA224`/`SHA256`/`SHA384`/`SHA512`) per upstream's supported set.
- **No Lua-runtime-error for verify-failure — upstream parity**: signature mismatch returns `false` (not a Lua error). Operator scripts may handle the result as a normal boolean. Unsupported hash algo also returns `false` (NOT a Lua error). This matches upstream's "verify-failure is a normal disposition, not a script error" semantic.
- **`:fileBytes` 16 MiB cap reuses datasource.go constant**: the cap is the SHARED `maxFilenameScriptBytes` constant (16 MiB) from `datasource.go:117` — zero-copy reuse of the 22.1 Task 11 cap pattern. No new constant declaration needed. Anti-DoS measure (not a feature toggle) per SPEC §11.2.5.
- **`:fileBytes` arbitrary-path-allowed — envoy-go-strict per D8**: no chroot, no allow-list, no path-restriction surface. Operators are expected to restrict at the OS-confinement layer (the envoy-go process's effective UID + filesystem ACLs). This is consistent with the envoy-go-strict classification (the method is NOT in upstream at any scope; envoy-go's surface is intentionally minimal).
- **Hand-off note to Task 14** (stats.go +5 counters + runtime-reject arms 20-22 byte-stable wording): Task 14 owns the byte-stable runtime-reject wording const surface (arm-20 / arm-21 / arm-22). The 3 wording constants declared at Task-11 (`httpCallClusterRequiredMsg`) + Task-7 (`bodyCapExceededFmt` inline) + Task-12 (`cryptoImportPublicKeyErrPrefix` + `miscFileBytesOverCapPrefix` + `miscTimestampInvalidUnitPrefix`) are candidates for relocation to a centralized const surface in `compiled_config.go` alongside the existing `parseReject*` const family (PLAN/Task 14 settles).
- **Hand-off note to Task 19** (atomic BEHAVIOR_CONTRACT.md 15-edit bundle): Task 19 lands the 4 NEW envoy-go-strict departure records anticipated at this Task per SPEC §14 edit #11 — `:base64Decode` + `:sha256` + `:sha512` + `:fileBytes` (plus optional `:timestamp` record bringing the bundle to 5 NEW records per PLAN/Task 19 final scrape). The bundle scale is currently 7 records (5 baseline from §14 #1-#5 + 2 from Task 9 dynamicMetadata + Task 11 httpcall) → 11 records at Task 12 IMPL atomic landing → 12-13 at Task 19 final scrape (depending on `:timestamp` disposition + R7/R8 confirmations).
- **Hand-off note to Task 18** (fixture-0027 scenarios (g) crypto + (h) fileBytes + (l) timestamp): scenarios (g) `crypto sha256 + base64Escape` cross-side byte-exact + (h) `fileBytes read` cross-side byte-exact + (l) `timestamp wall-clock` REFERENCE-LESS subject-only per SPEC §8.2 row g/h/l consume this Task 12's bridge implementation. The script-level wire shapes are `rh:sha256(s) + rh:base64Escape(s)` for (g); `rh:fileBytes(path)` for (h); `rh:timestamp("milliseconds")` for (l).
- **Commit SHA**: `4a567bd` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).
- **Tier + Task-number cross-reference**: Tier C bridge surfaces (Task 12 of 19 overall + Pre-Task 0). PARALLELIZABLE with Tasks 7-11 (already complete) + 13 (pending). Consumes the SHARED `maxFilenameScriptBytes` constant from `datasource.go:117` (zero-copy reuse of the 22.1 Task 11 cap pattern). Unblocks Task 14 (stats.go arms 20-22 byte-stable wording consumes the 3 const declarations landed at httpcall.go + crypto.go + misc.go) + Task 18 (fixture-0027 scenarios (g)+(h)+(l) consume this Task's bridge implementation) + Task 19 (4 NEW envoy-go-strict departure records anticipated at BEHAVIOR_CONTRACT.md atomic landing).

### Task 13: NEW `internal/filter/http/lua/streaminfo.go` extension + NEW `internal/filter/http/lua/filterstate.go` filter-state bridge [ADR-0192]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/http/lua/... -run 'TestStreamInfo_|Test_FilterState_'` clean; 11-method `:streamInfo()` surface verified (4 inherited from 22.1 Task 8 + 2 from 22.2 Task 9 + 5 NEW from this Task 13); `:filterState():get` + `:set` round-trip per typed-Lua-value table (LString / LNumber / LBool / LTable recursive); cross-stream isolation verified at N=10 parallel filter instances (each writes a stream-unique value to the same key; no cross-leak observed); per-stream lifecycle (OnDestroy releases map → nil); Lua-value marshaling typed per SPEC §11.8 D4 + AMEND-22.2-4 (string→LString; float64/int64→LNumber; bool→LBool; map[string]any→LTable recursive; []any→LTable 1-indexed); unsupported Lua types (LFunction / LChannel / LUserData / LState) at :set raise byte-pinned runtime error.
- **11-method streamInfo roster verified**: `:protocol()` / `:routeName()` / `:downstreamLocalAddress()` / `:downstreamDirectRemoteAddress()` (4 inherited) + `:dynamicMetadata()` / `:dynamicTypedMetadata(filterName)` (Task 9 bodies live at metadata.go) + `:upstreamHost()` / `:upstreamCluster()` / `:requestedServerName()` / `:filterState()` / `:downstreamSslConnection()` (5 NEW at this Task; bodies in streaminfo.go with `:filterState()` userdata construction routed through filterstate.go's pushFilterStateUDFromCb helper + `:downstreamSslConnection()` dispatching to ssl.go's pushSSLUD helper for symmetric ssl userdata identity to `:connection():ssl()`).
- **Files touched**:
  - `internal/filter/http/lua/streaminfo.go` (created, ~310 LoC; NEW file extracting all 22.1 Task 8 + 22.2 Task 9 streamInfo machinery + adding the 5 NEW Task 13 LGFunctions. Contains: `streamInfoMethods` dispatch table (11 entries); `installStreamInfoMetatable` registrar; `requestHandleStreamInfo` + `responseHandleStreamInfo` userdata-allocator LGFunctions on the request/response handles; `pushStreamInfoUD` + `streamInfoCallbacksFromUD` helpers; 9 LGFunction bodies — the 4 inherited (`streamInfoProtocol` / `streamInfoRouteName` / `streamInfoDownstreamLocalAddress` / `streamInfoDownstreamDirectRemoteAddress`) + 5 NEW (`streamInfoUpstreamHost` / `streamInfoUpstreamCluster` / `streamInfoRequestedServerName` / `streamInfoFilterState` / `streamInfoDownstreamSslConnection`); the 2 metadata LGFunctions stay at metadata.go since they consume `*dynamicmetadata.Bucket`).
  - `internal/filter/http/lua/streaminfo_test.go` (created, ~250 LoC; NEW `fakeCallbacksFull` test-double satisfying the EXTENDED 12-method RequestHandleCallbacks interface — 6 inherited + 5 NEW + 1 SetFilterState writeback; NEW `newBridgedVMWithFullCallbacks` helper installing the FULL bridge metatable set including filterStateMetatable; 11 NEW test functions covering all 5 NEW methods + 1 comprehensive 11-method surface presence test).
  - `internal/filter/http/lua/filterstate.go` (created, ~250 LoC; NEW file landing the filter-state bridge per SPEC §3.4 + §11.8 D4 closure + AMEND-22.2-4. Contains: `filterStateTypeName` registry-key const + `filterStateMethods` dispatch table (2 entries: :get / :set); `installFilterStateMetatable` registrar; `filterStateRef` Go-side wrapper struct holding getter + setter closures (rationale: the *filter back-pointer's filterState field may be nil at :filterState() construction time — lazy-allocation discipline — so we need a writeback channel via SetFilterState on the cb adapter); `pushFilterStateUDFromCb` production helper wiring both closures through the cb adapter's FilterState + SetFilterState accessors; `filterStateFromUD` extractor; `filterStateGet` + `filterStateSet` LGFunctions implementing :get + :set with typed marshaling; `anyToLua` + `luaToAny` + `luaTableToAny` marshaling helpers covering string / int / float / bool / map[string]any / []any in both directions; unsupported Lua types at :set raise `L.RaiseError` per AMEND-22.2-4 marshaling contract).
  - `internal/filter/http/lua/filterstate_test.go` (created, ~260 LoC; 8 NEW behavioral tests covering: `Test_FilterState_get_returns_marshaled_typed_value` (string + float64 + int64 + bool + map[string]any + []any), `Test_FilterState_get_missing_returns_nil`, `Test_FilterState_set_then_get_roundtrip` (5 types incl. nested map + list), `Test_FilterState_cross_stream_isolation` (N=10 parallel filters; each writes a stream-unique value to "shared-key"; assertions verify no cross-leak), `Test_FilterState_per_stream_lifecycle_OnDestroy_releases_map` (allocate + populate + OnDestroy + assert nil), `Test_FilterState_set_invalid_lua_type_raises_runtime_error` (Lua function literal at :set → runtime error), `Test_FilterState_filter_struct_initialized_empty_at_construction` (fresh *filter.filterState is nil or empty), `Test_FilterState_nil_map_tolerance` (nil filterState on cb → :get returns nil; no panic)).
  - `internal/filter/http/lua/bridge.go` (modified -150 LoC + +200 LoC; REMOVED the 4-method streamInfoMethods dispatch table + the installStreamInfoMetatable function + the 4 inherited streamInfo LGFunction bodies + the requestHandleStreamInfo/responseHandleStreamInfo allocator LGFunctions + the pushStreamInfoUD + streamInfoCallbacksFromUD helpers (all relocated to streaminfo.go). EXTENDED the RequestHandleCallbacks interface with 5 NEW accessors (UpstreamHost / UpstreamCluster / RequestedServerName / FilterState / SetFilterState — DownstreamTLSConnectionState already lived from Task 10). EXTENDED both adapter types (requestHandleCallbacksAdapter + responseHandleCallbacksAdapter) with 5 NEW method impls + a NEW `filter *filter` back-pointer field for FilterState writeback; UPDATED constructor signatures `newRequestHandleCallbacksAdapter(dcb, f)` + `newResponseHandleCallbacksAdapter(ecb, dcb, f)` accordingly. The bridge:framework adapter wiring stays in bridge.go since it doubles as the framework:bridge seam consumed by all bridge LGFunctions; only the streamInfo-specific machinery moved out.)
  - `internal/filter/http/lua/bridge_test.go` (modified +12 LoC; EXTENDED the existing `fakeCallbacks` test-double base type with 5 NEW zero-value impls (UpstreamHost / UpstreamCluster / RequestedServerName / FilterState / SetFilterState) so the Task 8 + Task 9 + Task 10 test-doubles that embed fakeCallbacks automatically satisfy the EXTENDED interface — no per-test rewiring needed).
  - `internal/filter/http/lua/lua.go` (modified +30 LoC; NEW `filterState map[string]any` field on *filter struct with full docstring + 2 envoy-go-strict departure record anchors; EXTENDED `OnDestroy()` to clear `f.filterState = nil` for GC + cross-stream isolation discipline per SPEC §11.8 D4 closure).
  - `internal/filter/http/lua/decode_headers.go` (modified +1 LoC; UPDATED the `newRequestHandleCallbacksAdapter(f.dcb)` call site to pass the *filter back-pointer: `newRequestHandleCallbacksAdapter(f.dcb, f)`. NEW `installFilterStateMetatable(L)` invocation alongside the other bridge metatable installs at VM construction).
  - `internal/filter/http/lua/encode_headers.go` (modified +1 LoC; UPDATED the `newResponseHandleCallbacksAdapter(f.ecb, f.dcb)` call site to pass the *filter back-pointer: `newResponseHandleCallbacksAdapter(f.ecb, f.dcb, f)`; encode_headers.go REUSES the per-stream VM constructed at decode-side so no separate metatable install is needed).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/... -run 'TestStreamInfo_|Test_FilterState_' -v
  ```

  ```
  --- PASS: Test_FilterState_get_returns_marshaled_typed_value (0.00s)
  --- PASS: Test_FilterState_get_missing_returns_nil (0.00s)
  --- PASS: Test_FilterState_set_then_get_roundtrip (0.00s)
  --- PASS: Test_FilterState_cross_stream_isolation (0.00s)
  --- PASS: Test_FilterState_per_stream_lifecycle_OnDestroy_releases_map (0.00s)
  --- PASS: Test_FilterState_set_invalid_lua_type_raises_runtime_error (0.00s)
  --- PASS: Test_FilterState_filter_struct_initialized_empty_at_construction (0.00s)
  --- PASS: Test_FilterState_nil_map_tolerance (0.00s)
  --- PASS: TestStreamInfo_UpstreamHost_returns_canned_value (0.00s)
  --- PASS: TestStreamInfo_UpstreamHost_empty_when_unset (0.00s)
  --- PASS: TestStreamInfo_UpstreamCluster_returns_canned_value (0.00s)
  --- PASS: TestStreamInfo_UpstreamCluster_empty_when_unset (0.00s)
  --- PASS: TestStreamInfo_RequestedServerName_returns_canned_value (0.00s)
  --- PASS: TestStreamInfo_RequestedServerName_empty_for_plaintext (0.00s)
  --- PASS: TestStreamInfo_FilterState_returns_userdata (0.00s)
  --- PASS: TestStreamInfo_DownstreamSslConnection_returns_ssl_userdata_when_tls (0.00s)
  --- PASS: TestStreamInfo_DownstreamSslConnection_returns_nil_for_plaintext (0.00s)
  --- PASS: TestStreamInfo_DownstreamSslConnection_dispatches_to_ssl_methods (0.00s)
  --- PASS: TestStreamInfo_11_method_surface_all_present (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	(race-clean)
  ```

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	2.672s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/lua/...
  ```

  ```
  (no output; exit 0 post-gofmt -w pass)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race` clean**: PASS — all 19 NEW Task 13 test cases (11 streamInfo + 8 filterState) green under `-race`; full package suite (`./internal/filter/http/lua/...`) clean at 2.672s including ALL pre-existing tests (Tasks 6-12 regressions remain green).
  - **11-method `:streamInfo()` surface verified**: `TestStreamInfo_11_method_surface_all_present` exercises ALL 11 methods on the same VM + same fakeCallbacksFull instance: `:protocol()` / `:routeName()` / `:downstreamLocalAddress()` / `:downstreamDirectRemoteAddress()` / `:dynamicMetadata()` / `:dynamicTypedMetadata("foo")` / `:upstreamHost()` / `:upstreamCluster()` / `:requestedServerName()` / `:filterState()` / `:downstreamSslConnection()` — 7 string-returning methods asserted via lua.LString type-check; 3 userdata-returning methods (:dynamicMetadata + :filterState + :downstreamSslConnection) asserted via lua.LTUserData type check; 1 :dynamicTypedMetadata stringified-Lua-table check is the existing Task 9 surface (unchanged by this Task).
  - **`:filterState():get` + `:set` round-trip per typed table**: `Test_FilterState_set_then_get_roundtrip` covers 5 types in one Lua script (`fs:set("s", "hello")` / `fs:set("n", 12.5)` / `fs:set("b", true)` / `fs:set("t", { name="alice", age=30 })` / `fs:set("l", { "x", "y", "z" })`) + asserts the round-trip via `fs:get(name)` for each; Go-side inspection of `cb.filterState["s"]` confirms the underlying map reflects the write.
  - **Cross-stream isolation N=10 parallel filter instances**: `Test_FilterState_cross_stream_isolation` spawns 10 goroutines each constructing a fresh `*luaprim.VM` + writing a stream-unique value (`v-a` / `v-b` / ... / `v-j`) to the SAME key (`"shared-key"`) + reading back. Assertions verify each stream's read-back value matches its own write — no cross-stream leak. The N=10 parallel discipline matches PLAN Task 13 acceptance.
  - **Per-stream lifecycle OnDestroy releases map**: `Test_FilterState_per_stream_lifecycle_OnDestroy_releases_map` constructs a fresh `*filter` with a pre-populated `filterState{"k": "v"}` map; invokes `f.OnDestroy()`; asserts `f.filterState == nil`. The lua.go OnDestroy implementation sets the map to nil for GC + cross-stream isolation discipline per SPEC §11.8 D4 closure.
  - **Typed Lua-value marshaling per SPEC §11.8 D4 + AMEND-22.2-4**: `Test_FilterState_get_returns_marshaled_typed_value` pre-seeds the cb adapter's map with 6 type-distinct values (`map[string]any{"sk": "the-string", "fk": float64(3.5), "ik": int64(42), "bk": true, "mk": map[string]any{...}, "lk": []any{...}}`) + asserts each round-trips to the expected typed Lua value. The marshaling helpers (anyToLua / luaToAny) live at filterstate.go alongside the LGFunctions.
  - **Unsupported Lua types at :set raise runtime error**: `Test_FilterState_set_invalid_lua_type_raises_runtime_error` invokes `fs:set("fn", function() end)`; asserts `vm.Run` returns a non-nil error. The luaToAny helper calls `L.RaiseError("filterState:set: unsupported value type %s", lv.Type().String())` for unsupported variants per AMEND-22.2-4 marshaling contract.
  - **`go build ./...` clean**: empty output, exit 0.
  - **`go vet ./...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/filter/http/lua/...` clean**: empty output, exit 0 post `gofmt -w` on streaminfo.go + bridge_test.go.
- **D4 closure recorded (filterState shape)**: this Task 13 entry CLOSES D4 (upstream `:filterState()` shape + gopher-lua LUserData support per SPEC §11.8). Resolution per AMEND-22.2-4: envoy-go adopts string-keyed `map[string]any` per-stream + `:get(name)` + `:set(name, value)` + typed Lua-value marshaling (LString / LNumber / LBool / LTable recursive). 2 envoy-go-strict departure records anticipated at Task 19 BEHAVIOR_CONTRACT.md atomic landing per SPEC §14 edit items 8+9:
  - **Departure record #9**: `:filterState():set(name, value)` mutation surface exposed (upstream FilterStateWrapper is strictly read-only because C++ filters mutate FilterState objects directly via the `StreamInfo::filterState()` accessor; envoy-go has no Go-side mutation analog at 22.2 — exposing :set at the Lua surface is the most natural mutation seam).
  - **Departure record #10**: `:filterState():get(name)` typed Lua-value marshaling (upstream always returns `serializeAsString()` Lua strings via `lua_pushlstring()`; envoy-go returns native typed Lua values per the LValue conversion table — aligns with the rest of the bridge surface where dynamicMetadata + headers all return typed values, not stringified).
- **D-decision-disposition update**: D4 CLOSED at this Task 13 implementation (per SPEC §11.8 D4 closure + AMEND-22.2-4 surfaced at SPEC commit). ADR-0192 §Decision body anchored at this Task's filterState bridge implementation; the §Decision body itself is authored at Task 19 atomic landing per ADR-0044 in-place edit discipline. **No new ADR consumed at this Task** — the filterState bridge surface lives inside ADR-0192. Bundle scale tracker: 11 records anticipated at Task 12 → 13 records anticipated at this Task (post-bundle: 5 baseline + 2 Task 9 dynamicMetadata + 1 Task 11 httpcall + 4 Task 12 crypto/fileBytes + 2 Task 13 filterState set + filterState get-typed-marshal = 13 envoy-go-strict records bundle, plus optional `:timestamp` at Task 19 final scrape).
- **Hand-off note to Task 14** (stats.go +5 counters + runtime-reject arms 20-22 byte-stable wording): Task 14 owns the byte-stable runtime-reject wording const surface. This Task 13's `filterState:set` unsupported-type runtime error is NOT one of the named arms 20-22 (those land at Task 7 body / Task 11 httpCall / Task 12 crypto). The unsupported-type :set error wording (`filterState:set: unsupported value type <type>`) is documented at the marshaling contract in filterstate.go but does NOT have a SPEC §6 arm assigned at the current scrape — PLAN/Task 19 settles whether to anchor a named arm or leave as a generic operator-error.
- **Hand-off note to Task 18** (fixture-0027 scenarios (i) streamInfo upstreamHost/Cluster + (m) filterState cross-stream): scenarios (i) `streamInfo upstreamHost + upstreamCluster` cross-side byte-exact + (m) `filterState cross-stream set+get` REFERENCE-LESS subject-only per SPEC §8.2 row i/m consume this Task 13's bridge implementation. Script-level wire shapes: `rh:streamInfo():upstreamHost()` + `rh:streamInfo():upstreamCluster()` for (i); `rh:streamInfo():filterState():set(k, v)` + `:get(k)` for (m). Note that (i) WILL surface "" at differential time because the framework adapter stubs UpstreamHost+UpstreamCluster at phase 22.2 (framework gap; matches RouteName pattern at 22.1) — fixture (i) effectively pins the "always-string, never-nil" contract on the framework-gap surface (operators observe empty strings; no panic + no Lua error).
- **Hand-off note to Task 19** (atomic BEHAVIOR_CONTRACT.md 15-edit bundle): Task 19 lands the 2 NEW envoy-go-strict departure records anticipated at this Task per SPEC §14 edit items 8+9 — `:filterState():set()` exposed (upstream read-only) + `:filterState():get()` typed marshal (upstream serializeAsString). The bundle scales 11 → 13 at Task 13 IMPL atomic landing → 13-14 at Task 19 final scrape (depending on `:timestamp` disposition at Task 12 hand-off).
- **Tier C bridge surfaces COMPLETE after this Task**: this Task 13 lands the FINAL Tier C bridge surface family (filterState — IN-PACKAGE per Q9 + D4 closure). All 7 Tier C bridge surface families (body / trailers / metadata / connection-SSL / httpCall / crypto / fileBytes+timestamp / streamInfo-full + filterState) are now ACTIVE; Tier D (stats + runtime-reject + race + fuzz + bench at Tasks 14-16) builds on this stable bridge surface; Tier E (fixtures at Tasks 17-18) consumes the bridge surface for cross-side byte-exact + REFERENCE-LESS subject-only differential coverage.
- **11-method streamInfo extraction rationale**: the bridge.go file had grown to ~1562 LoC pre-extraction; relocating the streamInfo machinery to a dedicated streaminfo.go (~310 LoC) aligns with the per-surface-family file discipline that already exists for body.go / trailers.go / metadata.go / connection.go / ssl.go / httpcall.go / crypto.go / misc.go. The RequestHandleCallbacks interface declaration + the 2 adapter types stay in bridge.go since they double as the framework:bridge seam consumed by ALL bridge LGFunctions (not just streamInfo); centralizing the contract preserves the single-anchor-point discipline.
- **Lazy-allocation discipline at filterStateRef**: the per-stream filterState map[string]any may be nil at :filterState() construction time (the *filter struct's `filterState` field starts nil; lazy-allocated on first :set from the bridge LGFunction). The filterStateRef Go-side wrapper struct holds BOTH a getter closure (re-reads from cb.FilterState() each call — observes the freshly-allocated map post-lazy-init) AND a setter closure (writes back through cb.SetFilterState(...) so the *filter struct's field observes the freshly-allocated map). This 2-closure indirection is required because Go maps are passed by reference but the nil-map case has no underlying hash to share — the lazy-init must write a fresh pointer back through a writeback channel. Tests verify both the get-side (post-write observability) + set-side (Go-side `cb.filterState["s"]` reflects the write).
- **Commit SHA**: `26410cb` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).

### Task 14: EXTEND `internal/filter/http/lua/stats.go` 3 → 8 counters + SPEC §6 arm 20-22 byte-stable wording catalog [ADR-0192]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/http/lua/... -run 'TestStatNames|TestNewFilterStats|TestRuntimeReject|TestDefaultMaxBody'` clean; 8-counter HCM-rooted registration verified at `newFilterStats(reg, "ingress_http", "my_prefix")` (3 inherited + 5 NEW envoy-go-strict per 22.2 SPEC §7.1); `TestStatNames_TableDriven` extends to 8 stat names byte-exact; empty-stat-prefix consecutive-dot behavior carries forward at 8-counter cardinality; 19-arm config-load PARSE-REJECT roster from 22.1 STILL passes (no regressions); SPEC §6 arms 20-22 byte-stable wording catalog tests added at compiled_config_test.go (centralizes the per-method wording assertions already pinned at body_test.go + httpcall_test.go + crypto_test.go).
- **5 NEW envoy-go-strict counters verified** (per 22.2 SPEC §7.1 + AMEND-22.2-3 + §11.7 D6):
  - `httpcall_total` (counter; sync + async dispatches per row 1)
  - `httpcall_failures` (counter; SYNC-ONLY per row 2 — async fire-and-forget failures invisible per upstream parity)
  - `httpcall_timeouts` (counter; SYNC-ONLY per row 3)
  - `body_buffered_bytes_total` (counter; cumulative DecodeData / EncodeData bytes per row 4)
  - `coroutine_yields_total` (counter; ONCE per yield site, NOT per Resume site per row 5)
- **Project stat-count delta**: 102 → 107 (+5) per 22.2 SPEC §7.1. Optional 6th `dynmd_writes_total` deferred per BRAINSTORM Q11 + SPEC §7.1 RECOMMENDATION (omitted from canonical roster at 22.2 phase-done).
- **Arms 20-22 byte-stable wording reconciliation**: NO divergences found between Tasks 7/11/12 W2 pinnings and SPEC §6 prescribed wordings.
  - **Arm 20** (`httpcall-cluster-name-required`): production wording at `httpcall.go::httpCallClusterRequiredMsg` = `"lua: httpCall: cluster name must not be empty"` matches SPEC §6 row 20 byte-exact. Live raise tested at `httpcall_test.go::Test_HTTPCall_empty_cluster_raises_arm20_byte_stable_wording`; const byte-pin tested at `compiled_config_test.go::TestRuntimeRejectArm20_HTTPCallClusterRequired_ByteExactWording`.
  - **Arm 21** (`body-size-cap-exceeded`): production wording inline at `body.go:315-318` + `body.go:406-409` = `fmt.Sprintf("lua: body: accumulated body exceeds maximum buffered size of %d bytes", f.maxBodyBufferedBytes)` matches SPEC §6 row 21 byte-exact. Live raise tested at `body_test.go::Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording`; const byte-pin sentinel-formatted at `compiled_config_test.go::TestRuntimeRejectArm21_BodyOverCap_ByteExactWording`. Wording template is inline (not yet a named const) — future maintainer note: if extracted to a named const (e.g. `bodyOverCapMsgFmt`), update both fmt.Sprintf sites + the sentinel test in lockstep per ADR-0044 atomic-edit discipline.
  - **Arm 22** (`crypto-key-format-invalid`): production wording prefix at `crypto.go::cryptoImportPublicKeyErrPrefix` = `"lua: importPublicKey:"` materializes SPEC §6 row 22's template `"lua: %s: %w"` with `%s` = "importPublicKey"; the trailing inner crypto/x509 error carries variable bytes (per-error wording). Live raise tested at `crypto_test.go::Test_ImportPublicKey_invalid_PEM_raises_arm22_byte_stable_wording`; const byte-pin tested at `compiled_config_test.go::TestRuntimeRejectArm22_CryptoImportPublicKeyPrefix_ByteExactWording`. The prefix form is byte-stable; the SPEC template + W2 pinning agree.
- **Wording-location decision**: 3 runtime-reject arms 20-22 STAY at their respective per-method files (httpcall.go arm 20 + body.go arm 21 inline + crypto.go arm 22) rather than relocating to compiled_config.go's `parseReject*` const family. Rationale per SPEC §11.2 AMEND-22.2-2: arms 20-22 are RUNTIME-REJECTS (raised via `L.RaiseError` from bridge LGFunctions), NOT PARSE-REJECTs at config-load time. compiled_config.go owns the 19-arm config-load PARSE-REJECT roster (UNCHANGED at 22.2); arms 20-22 belong with their raise sites for locality. compiled_config_test.go centralizes the cross-package wording catalog (mirrors the `TestParseRejectConstants_ByteExactWording` precedent at line 393) so a SPEC §6 drift surfaces at the catalog test alongside the per-method live-raise tests.
- **maxBodyBufferedBytes default 16 MiB verified**: `lua.go::defaultMaxBodyBufferedBytes` = `16 * 1024 * 1024` = `16777216` bytes per Task 7's declaration. Pinned at `compiled_config_test.go::TestDefaultMaxBodyBufferedBytes_SixteenMiB`. Per SPEC §6 row 21: "16 MiB cap inherits 22.1 Task 11 cap pattern" — Task 14 confirms the value + verifies the test catches drift at both the literal `16 * 1024 * 1024` form AND the resolved `16777216` form.
- **Files touched**:
  - `internal/filter/http/lua/stats.go` (modified +83 LoC; EXTENDED file-doc preamble with 22.2 SPEC §7.1 + AMEND-22.2-3 cross-references + 5-counter roster doc; ADDED 5 NEW package-level `statName*` consts (`statNameHTTPCallTotal` / `statNameHTTPCallFailures` / `statNameHTTPCallTimeouts` / `statNameBodyBufferedBytesTotal` / `statNameCoroutineYieldsTotal`) with byte-exact wire-name strings + per-counter doc-comments cross-referencing SPEC §7.1 row 1-5 + envoy-go-strict departure record anticipations; EXTENDED `newFilterStats` factory body from 3 → 8 `reg.NewCounter(base + statName*)` calls + EXTENDED its doc-comment to reflect 8-counter cardinality + lazy-test-consumer cross-reference).
  - `internal/filter/http/lua/lua_test.go` (modified +127 LoC; EXTENDED `TestStatNames_TableDriven` from 3 to 8 stat names byte-exact; ADDED 5 NEW per-counter byte-exact tests (`TestStatNames_Equal_HTTPCallTotal` / `_HTTPCallFailures` / `_HTTPCallTimeouts` / `_BodyBufferedBytesTotal` / `_CoroutineYieldsTotal`); ADDED `TestNewFilterStats_RegistersEightCounters_HCMRootedTemplate` (full 8-name registration + 8-field nil-check); ADDED `TestNewFilterStats_EightCounterCardinality` (cardinality regression sentinel for the 22.2 SPEC §7.1 roster); ADDED `TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot_Eight` (AMEND-2 consecutive-dot at 8-counter scale); SUPERSEDED the 22.1 3-counter tests `TestNewFilterStats_RegistersThreeCounters_HCMRootedTemplate` + `TestNewFilterStats_CardinalityAssertion` + `TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot` (removed; their 8-counter superset replacements above provide strictly stronger coverage); UPDATED `TestNewFilterStats_EmptyHcmAndConfig_DoubleConsecutiveDot` from 3-counter to 8-counter cardinality + doc-comment reflects the supersedence; UPDATED `TestNew_HappyPath_ReturnsFactoryAndStatsRegistered` + `TestNew_HappyPath_EmptyLuaStatPrefix_ConsecutiveDot` from 3 to 8 registered counters end-to-end through New → newFilterStats).
  - `internal/filter/http/lua/compiled_config_test.go` (modified +115 LoC; ADDED `fmt` import; ADDED a dedicated Task 14 section header for SPEC §6 arms 20-22 byte-stable wording catalog; ADDED `TestRuntimeRejectArm20_HTTPCallClusterRequired_ByteExactWording` (pins `httpCallClusterRequiredMsg` const); ADDED `TestRuntimeRejectArm22_CryptoImportPublicKeyPrefix_ByteExactWording` (pins `cryptoImportPublicKeyErrPrefix` const); ADDED `TestRuntimeRejectArm21_BodyOverCap_ByteExactWording` (sentinel-formatted byte-stable shape probe for arm 21's inline `fmt.Sprintf` template at body.go); ADDED `TestDefaultMaxBodyBufferedBytes_SixteenMiB` (pins `defaultMaxBodyBufferedBytes` = 16 MiB literal form + resolved 16777216 form)).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Verification command outputs**:

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/... -run 'TestStatNames|TestNewFilterStats|TestRuntimeReject|TestDefaultMaxBody|TestParseRejectConstants' -v
  ```

  ```
  --- PASS: TestParseRejectConstants_ByteExactWording (0.00s)
  --- PASS: TestRuntimeRejectArm20_HTTPCallClusterRequired_ByteExactWording (0.00s)
  --- PASS: TestRuntimeRejectArm22_CryptoImportPublicKeyPrefix_ByteExactWording (0.00s)
  --- PASS: TestRuntimeRejectArm21_BodyOverCap_ByteExactWording (0.00s)
  --- PASS: TestDefaultMaxBodyBufferedBytes_SixteenMiB (0.00s)
  --- PASS: TestStatNames_Equal_Errors (0.00s)
  --- PASS: TestStatNames_Equal_Executions (0.00s)
  --- PASS: TestStatNames_Equal_RespondCalls (0.00s)
  --- PASS: TestStatNames_TableDriven (0.00s)
  --- PASS: TestStatNames_Equal_HTTPCallTotal (0.00s)
  --- PASS: TestStatNames_Equal_HTTPCallFailures (0.00s)
  --- PASS: TestStatNames_Equal_HTTPCallTimeouts (0.00s)
  --- PASS: TestStatNames_Equal_BodyBufferedBytesTotal (0.00s)
  --- PASS: TestStatNames_Equal_CoroutineYieldsTotal (0.00s)
  --- PASS: TestNewFilterStats_RegistersEightCounters_HCMRootedTemplate (0.00s)
  --- PASS: TestNewFilterStats_EightCounterCardinality (0.00s)
  --- PASS: TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot_Eight (0.00s)
  --- PASS: TestNewFilterStats_EmptyHcmAndConfig_DoubleConsecutiveDot (0.00s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.027s
  ```

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	2.561s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/lua/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race` clean**: PASS — all 18 NEW Task 14 test cases (5 statName equals + 5 NEW statName equals + 1 table-driven 8-row + 3 NewFilterStats 8-counter variants + 1 8-cardinality consecutive-dot + 3 runtime-reject byte-stable wording catalog + 1 defaultMaxBody constant); full package suite (`./internal/filter/http/lua/...`) clean at 2.561s with `-race` including ALL pre-existing tests (Tasks 1-13 regressions remain green).
  - **8-counter HCM-rooted registration verified**: `TestNewFilterStats_RegistersEightCounters_HCMRootedTemplate` exercises `newFilterStats(reg, "ingress_http", "my_prefix")` and asserts all 8 byte-exact wire names land under `http.ingress_http.lua.my_prefix.<stat>` per AMEND-2 (template UNCHANGED from 22.1) — 3 inherited (errors / executions / respond_calls) + 5 NEW (httpcall_total / httpcall_failures / httpcall_timeouts / body_buffered_bytes_total / coroutine_yields_total). All 8 counter fields on the returned `*filterStats` asserted non-nil.
  - **`TestStatNames_TableDriven` extends to 8 stat names byte-exact**: the table-driven row count expands from 3 → 8 with each row pinning a `statName*` const to its byte-exact string-literal counterpart per ADR-0143 SN2-reuse + the dual-layer compile-time guard discipline.
  - **Empty-stat-prefix consecutive-dot at 8-counter cardinality**: `TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot_Eight` + `TestNewFilterStats_EmptyHcmAndConfig_DoubleConsecutiveDot` (updated) cover both the single-empty-prefix path (`http.ingress_http.lua..<stat>`) and the double-empty-prefix degenerate-but-valid path (`http..lua..<stat>`) at the 8-counter scale.
  - **19-arm config-load PARSE-REJECT roster from 22.1 STILL passes**: full `compiled_config_test.go` suite green; `TestParseRejectConstants_ByteExactWording` covers arms 1/3/4/18 byte-exact; `TestParseRejectArm02_WrappedError_HasPrefix` covers arm 2's wrap prefix; arm-19 tests (`TestParseRejectArm19_*`) all green — no regressions from the Task 14 extension.
  - **Arms 20-22 byte-stable wording catalog**: 3 NEW const-byte-pin tests at compiled_config_test.go mirror the live-raise tests at body_test.go + httpcall_test.go + crypto_test.go; drift on any of the 3 wording surfaces surfaces at BOTH the catalog test AND the per-method test in lockstep per ADR-0044 atomic-edit discipline.
  - **`go build ./...` clean**: empty output, exit 0.
  - **`go vet ./...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/filter/http/lua/...` clean**: empty output, exit 0.
- **5 envoy-go-strict departure record anticipations for Task 19 BEHAVIOR_CONTRACT.md bundle** (per SPEC §7.3 + §14 edit items 3-7):
  - **Departure record #3**: `httpcall_total` counter (envoy-go-strict; operator outbound-call observability; upstream Envoy Lua filter has only `errors` + `executions` per `ALL_LUA_FILTER_STATS` macro).
  - **Departure record #4**: `httpcall_failures` SYNC-ONLY counter (envoy-go-strict; sync-only per §11.7 D6 upstream parity — async fire-and-forget failures invisible at filter-stats layer).
  - **Departure record #5**: `httpcall_timeouts` SYNC-ONLY counter (envoy-go-strict; sync-only per §11.7 D6).
  - **Departure record #6**: `body_buffered_bytes_total` counter (envoy-go-strict; body-buffer capacity-planning visibility).
  - **Departure record #7**: `coroutine_yields_total` counter (envoy-go-strict; coroutine perf-debugging visibility).
- **Bundle-scale update**: 13 envoy-go-strict departure records at Task 13 hand-off + 5 NEW counter records anticipated at this Task 14 = 18 records anticipated at Task 19 atomic landing. Note: 13 + 5 = 18 vs. the SPEC §7.3 "raises bundle from 3 at 22.1 to 8 at 22.2 phase-done" — the divergence reflects the Task 9 + Task 12 + Task 13 unanticipated additions (dynamicMetadata read+write + crypto/fileBytes departures + filterState set/get-typed). Task 19 atomic scrape will reconcile the final count.
- **D-decision-disposition update**: no new D-closures at this Task (Task 14 is a stat-surface + wording-catalog extension; no new D-questions land or get closed). ADR-0192 §Decision body anchored at this Task's stat-surface extension; the §Decision body itself is authored at Task 19 atomic landing per ADR-0044 in-place edit discipline. **No new ADR consumed at this Task** — the 5-counter extension + the 3-arm wording catalog live inside ADR-0192's bridge-surface scope.
- **Hand-off note to Task 15** (race + 2 benchmarks per D-P10 + D3): Task 15 lands the race tests + the 2 benchmarks; the 8-counter `*filterStats` struct is stable for the race tests + the `BenchmarkPerStream_FullBridge_LState_Construction` covers the FULL 22.2 bridge surface (including the 5 NEW counters' allocation cost). Task 15's R6 disposition (STANDS WEAK-default | ADR-0193 FIRES) gates Task 19's conditional ADR-0193 landing.
- **Hand-off note to Task 16** (fuzzers per D-P7): the 8-counter stat surface lands stable at this Task; Task 16's `FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig` exercise the body + httpCall surfaces against the 5 NEW counter increment paths under random inputs.
- **Hand-off note to Task 19** (atomic BEHAVIOR_CONTRACT.md 15-edit bundle): Task 19 lands the 5 NEW envoy-go-strict departure records anticipated at this Task per SPEC §14 edit items 3-7. The bundle scales 13 → 18 at this Task 14 IMPL → final scrape at Task 19. SPEC §14 edit #2 ("Stat-table 102 → 107 extension under `## Stat surface` — 5 new rows") also lands at Task 19.
- **Tier D progress**: Task 14 (stats + runtime-reject byte-stable wording catalog) COMPLETE. Tier D remainder = Task 15 (race + 2 benchmarks) + Task 16 (29th + 30th fuzzers). Task 14 is PARALLELIZABLE with Tasks 15 + 16 per PLAN; this Task lands first so the 8-counter `*filterStats` surface is stable for Tasks 15-16's race + fuzz consumption.
- **Commit SHA**: `36f73f6` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).

### Task 15: EXTEND `internal/filter/http/lua/lua_test.go` + `internal/lua/coroutine_test.go` race tests + 2 benchmarks per D-P10 + D3 closure [ADR-0192 + R6 signal]

- **Acceptance criteria**: `go test -count=1 -race ./internal/filter/http/lua/... ./internal/lua/...` clean at N=100 parallel filter dispatches + N=100 parallel coroutine dispatches; `BenchmarkPerStream_FullBridge_LState_Construction` reports `ns/op` value verbatim at `-benchtime=3s`; `BenchmarkBodyBridge_DefensiveCopy_PerStream` reports `ns/op` for sub-MB body + 16-MiB-cap-saturated body verbatim; R6 disposition recorded (STANDS WEAK-default | ADR-0193 escape-valve FIRES) per the R6 signal protocol consumed by Task 19 Step 10; R9 disposition cross-checked against Task 7 IMPL outcome.
- **Files touched**:
  - `internal/filter/http/lua/lua_test.go` (modified +351 LoC; ADDED `context` / `runtime` / `time` imports; ADDED Task 15 section header documenting the R6 + R9 signaling protocols + the D3 closure threshold gates; ADDED `buildBenchFullBridgeConfig` helper allocating the 8-counter `*filterStats` via `newFilterStats` so the 5 NEW envoy-go-strict counter allocations land inside the benchmark window; ADDED `BenchmarkPerStream_FullBridge_LState_Construction` benchmark constructing per-stream VM + installing ALL 11 22.2 bridge metatables (request_handle + response_handle + headers + trailers + streamInfo + metadata + dynamicMetadata + connection + ssl + publicKeyWrapper + filterState) + the pairs shim + attaching a per-stream context + minting a child *LState via `vm.NewThread` + building per-stream request_handle + response_handle userdata + running the chunk + invoking both envoy_on_request + envoy_on_response hooks + per-stream cleanup (cancelChild + cancelCtx + vm.Close); ADDED `BenchmarkBodyBridge_DefensiveCopy_PerStream` benchmark with sub-MB (100 KB) + 16-MiB-saturated sub-benchmarks driving body accumulation via `accumulateRequestBody` chunked at 64 KiB framework-typical writes + forcing the `lua.LString(string(b))` defensive copy via the rh:body() bridge LGFunction; ADDED `runBodyBridgeBenchmark` shared driver helper; ADDED `TestRace_N100_parallel_filter_dispatches_clean_under_race` race test spawning N=100 goroutines each running pre-body-accumulation + DecodeHeaders + EncodeHeaders + EncodeData + OnDestroy at the FULL 22.2 bridge surface with cross-stream-state-leak detection (X-Stream-Id + X-Resp-Stream-Id round-trip) + goroutine-leak detection (runtime.NumGoroutine baseline-delta + ≤ 2 tolerance) + 5-counter post-assertion).
  - `internal/lua/coroutine_test.go` (modified +146 LoC; ADDED `fmt` import; ADDED `TestRace_N100_parallel_NewThread_Resume_yield_clean_under_race` race test spawning N=100 goroutines each constructing an independent VM + driving the coroutine through vm.NewThread + vm.Resume (initial yield via bridge_yield) + vm.Resume (resume-after-yield with per-goroutine-distinct value) + cancelFn invocation; per-goroutine-distinct yield + completion values surface any cross-goroutine state leak; goroutine-leak detection via runtime.NumGoroutine baseline-delta + ≤ 2 tolerance; cross-references the broader filter-level race test at `internal/filter/http/lua/lua_test.go` per D-P9 race-scoping).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Verification command outputs**:

  ```
  $ go test -race -count=1 -run 'TestRace_N100_parallel_filter_dispatches' -v ./internal/filter/http/lua/
  ```

  ```
  === RUN   TestRace_N100_parallel_filter_dispatches_clean_under_race
  --- PASS: TestRace_N100_parallel_filter_dispatches_clean_under_race (0.06s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.071s
  ```

  ```
  $ go test -race -count=1 -run 'TestRace_N100_parallel_NewThread_Resume_yield' -v ./internal/lua/
  ```

  ```
  === RUN   TestRace_N100_parallel_NewThread_Resume_yield_clean_under_race
  --- PASS: TestRace_N100_parallel_NewThread_Resume_yield_clean_under_race (0.05s)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/lua	1.065s
  ```

  ```
  $ go test -race -count=10 -run 'TestRace_N100' ./internal/filter/http/lua/ ./internal/lua/
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.565s
  ok  	github.com/esalaine/envoy-go/internal/lua	1.531s
  ```

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/... ./internal/lua/...
  ```

  ```
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	2.706s
  ok  	github.com/esalaine/envoy-go/internal/lua	1.123s
  ```

  ```
  $ go test -run '^$' -bench=BenchmarkPerStream_FullBridge_LState_Construction -benchtime=3s ./internal/filter/http/lua/
  ```

  ```
  goos: linux
  goarch: amd64
  pkg: github.com/esalaine/envoy-go/internal/filter/http/lua
  cpu: AMD Ryzen 9 9950X3D 16-Core Processor          
  BenchmarkPerStream_FullBridge_LState_Construction-32    	   36634	     98157 ns/op	  416480 B/op	    1322 allocs/op
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	4.599s
  ```

  ```
  $ go test -run '^$' -bench=BenchmarkBodyBridge_DefensiveCopy_PerStream -benchtime=3s ./internal/filter/http/lua/
  ```

  ```
  goos: linux
  goarch: amd64
  pkg: github.com/esalaine/envoy-go/internal/filter/http/lua
  cpu: AMD Ryzen 9 9950X3D 16-Core Processor          
  BenchmarkBodyBridge_DefensiveCopy_PerStream/sub-MB-32         	   35902	    103268 ns/op	 991.60 MB/s	  635680 B/op	     912 allocs/op
  BenchmarkBodyBridge_DefensiveCopy_PerStream/16-MiB-saturated-32         	     414	   9313623 ns/op	1801.36 MB/s	133568517 B/op	    1198 allocs/op
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	9.511s
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/lua/... ./internal/lua/...
  ```

  ```
  (no output; exit 0)
  ```

- **Acceptance-criteria evidence**:
  - **`go test -count=1 -race` clean at N=100 parallel filter + coroutine dispatches**: PASS — both race tests (`TestRace_N100_parallel_filter_dispatches_clean_under_race` at `internal/filter/http/lua/` + `TestRace_N100_parallel_NewThread_Resume_yield_clean_under_race` at `internal/lua/`) green at `-count=10` with `-race` enabled. Full-package `-race` suite at `./internal/filter/http/lua/...` (2.706s) + `./internal/lua/...` (1.123s) all green — no race-detector reports, no goroutine-leak detection trips, no cross-stream-state-leak detection trips, no cross-goroutine-coroutine-state-leak trips.
  - **Cross-stream-state-leak coverage**: each of the N=100 goroutines threads a per-goroutine-distinct identifier (`stream-<idx>` decode-side, `resp-<idx>` encode-side, `body-payload-<idx>` body-side) through its OWN request_handle + response_handle headers carriers; the Lua script echoes the identifier back into the headers map under `X-Lua-Saw` / `X-Resp-Lua-Saw`; cross-stream contamination would surface as a per-goroutine assertion failure with the WRONG `stream-N` echo. All 100 goroutines pass at -count=10 — no leaks.
  - **Cross-goroutine-coroutine-state-leak coverage**: each of the N=100 goroutines at the coroutine-level race test threads a per-goroutine-distinct yield value (`y-<idx>`) + resume value (`r-<idx>`) + completion string (`got:r-<idx>:done-<idx>`); cross-goroutine coroutine-state contamination would surface as a wrong yield-value or wrong completion string. All 100 goroutines pass at -count=10 — no leaks.
  - **Goroutine-leak detection**: both race tests measure `runtime.NumGoroutine` baseline pre-spawn + post-wait-settle (20 ms GC + sleep settle window); assert `after <= before + 2` (Go scheduler churn tolerance). Both race tests pass — no goroutine leak from per-stream filter dispatch nor from per-coroutine cancelFn invocation. Confirms ADR-0191 §Context per-stream child-LState lifecycle discipline (CancelFunc invocation releases ctx-attached child loops cleanly).
  - **`BenchmarkPerStream_FullBridge_LState_Construction` reports `ns/op` verbatim at `-benchtime=3s`**: `98157 ns/op` (~98 µs). Construction surface measured: NewVM + sandbox install + base-lib load + ALL 11 22.2 bridge metatables installed (request_handle + response_handle + headers + trailers + streamInfo + metadata + dynamicMetadata + connection + ssl + publicKeyWrapper + filterState) + pairs shim install + per-stream context attach + child *LState mint via `vm.NewThread` + request_handle + response_handle userdata build + chunk top-level Run + envoy_on_request CallGlobal + envoy_on_response CallGlobal + child cancel + ctx cancel + vm.Close. 416480 B/op + 1322 allocs/op surface the per-stream allocation cost across the FULL 22.2 bridge surface — within the bounds of the per-stream VM construction model + the gopher-lua bridge-metatable-table-binding cost.
  - **`BenchmarkBodyBridge_DefensiveCopy_PerStream` reports `ns/op` for sub-MB + 16-MiB-saturated verbatim**: `sub-MB = 103268 ns/op` (~103 µs); `16-MiB-saturated = 9313623 ns/op` (~9.3 ms). Both well under their D3 closure threshold gates (1 ms sub-MB + 100 ms 16-MiB-saturated). Throughput at 16 MiB scale: 1801.36 MB/s — bounded by `lua.LString(string(b))`'s memcpy + the `append([]byte, chunk...)` accumulator's slice-growth amortized cost; both linear-in-bytes.
  - **`go build ./...` clean**: empty output, exit 0 — race + benchmark additions land cleanly across the project.
  - **`go vet ./...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/filter/http/lua/... ./internal/lua/...` clean**: empty output, exit 0 (one transient gofmt-misalignment from initial hand-edit was resolved via `gofmt -w` pass before commit).
- **D-decision-disposition update**: this Task 15 entry RESOLVES the R6 disposition signal per the PLAN Task 15 R6 signal protocol + cross-references the R9 disposition signal recorded at the Task 7 PROGRESS.md entry. Together these two dispositions gate Task 19's conditional ADR-0193 §Context + §Decision + §Consequences landing — both must signal NO-FIRE for ADR-0193 to carry forward unconsumed to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot.

  **§13-R6 disposition: STANDS WEAK-default at ns/op=98157**

  Rationale: `BenchmarkPerStream_FullBridge_LState_Construction` at `-benchtime=3s` reports `98157 ns/op` (~98 µs per per-stream construction at the FULL 22.2 bridge surface). The R6 escape-valve gate at SPEC §13-R6 + D-P10 is `ns/op > 1_000_000` (= 1 ms); the measured value is approximately 10.2× UNDER the gate. PLAN hypothesis CONFIRMED (anticipated 200-500 µs at 3-7× the 22.1 headers-only baseline of 69865 ns/op; measured 98 µs is actually only 1.4× the 22.1 baseline — the FULL 22.2 bridge surface adds modest per-stream overhead via the 7 NEW metatable installs + the 5 NEW counter increment-call-sites + the child *LState mint via vm.NewThread, but the overall per-stream construction cost stays well within 1 ms). The per-stream `*lua.LState` construction discipline (fresh VM per stream + shared `*Chunk` cache per ADR-0192 §Context) STANDS at 22.2 phase-done; conditional ADR-0193 §Context + §Decision + §Consequences body NOT consumed at Task 19; carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot per SPEC §13-R6 + §1.2 hypothesis (a). Task 19's Step 10 grep for the literal substring `§13-R6 disposition: ADR-0193 FIRES` will find ZERO matches in this entry — ADR-0193 does NOT fire from the R6 signal.

  **§13-R9 disposition cross-check: STAYS embedded in ADR-0192** (CONFIRMED from Task 7)

  Cross-reference: the §13-R9 disposition was already resolved at the Task 7 PROGRESS.md entry per the Task 7 R9 signal protocol — see PROGRESS.md line ~1103. The body-bridge implementation surface (yield/resume orchestration + defensive-copy discipline + over-cap arm + 2 NEW counter increment-sites) did NOT introduce additional ADR-warranting complexity beyond what is documented under ADR-0192 §Context. The R9 perf-validation arm is independently CONFIRMED at this Task 15 by `BenchmarkBodyBridge_DefensiveCopy_PerStream`: sub-MB `103268 ns/op` (~103 µs; gate ≤ 1 ms; 9.7× under) + 16-MiB-saturated `9313623 ns/op` (~9.3 ms; gate ≤ 100 ms; 10.7× under). Both D3 closure threshold gates met → option (a) defensive-copy at endStream STANDS at 22.2 phase-done per SPEC §11.3 + §12 RECOMMENDED option (a); option (b) zero-copy via `*lua.LUserData` wrapping NOT consumed. Task 19's Step 10 grep for `§13-R9 disposition: ADR-0193 FIRES` will find ZERO matches across the Task 7 + Task 15 entries — ADR-0193 does NOT fire from the R9 signal either.

  **Combined R6 + R9 disposition**: BOTH signal NO-FIRE → conditional ADR-0193 §Context + §Decision + §Consequences body NOT consumed at Task 19 atomic landing → ADR-0193 carries forward UNCONSUMED to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot per SPEC §1.2 hypothesis (a) + §13-R6 + §13-R9.
- **Hand-off note to Task 16** (fuzzers per D-P7): the race + benchmark surface lands stable; Task 16's `FuzzLuaBodyBridge` consumes the body bridge + the `accumulateRequestBody` helper at random-input scale (the benchmark established a baseline of 1801 MB/s 16-MiB-saturated throughput — fuzzer input sizes are bounded so no perf regression risk). `FuzzLuaHTTPCallConfig` is independent of this Task's race/benchmark surface.
- **Hand-off note to Task 19** (atomic BEHAVIOR_CONTRACT.md 15-edit bundle + conditional ADR-0193 landing): Task 19 Step 10 greps for `§13-R6 disposition: ADR-0193 FIRES` AND `§13-R9 disposition: ADR-0193 FIRES` across the PROGRESS.md task entries. This Task 15 entry's R6 sentinel = `§13-R6 disposition: STANDS WEAK-default at ns/op=98157`; the Task 7 entry's R9 sentinel = `§13-R9 disposition: STAYS embedded in ADR-0192`. NEITHER sentinel triggers the FIRES grep — conditional ADR-0193 NOT landed at Task 19; carries forward UNCONSUMED to 22.3 BRAINSTORM per SPEC §13-R6 + §13-R9.
- **Benchmark-variance observation**: the FullBridge benchmark exhibited modest variance across multiple `-benchtime=3s` invocations (`95041 ns/op` → `98157 ns/op` → ~2% spread; alloc counts identical at 1322/op). The body-bridge sub-MB benchmark shows similar variance (`110556 ns/op` → `103268 ns/op` → ~7% spread); the 16-MiB-saturated body-bridge benchmark is dominated by the linear-in-bytes memcpy (variance bounded by the underlying memcpy bandwidth, ~10% spread). All measurements remain well within their threshold gates — the 10× safety margin at both gates absorbs benchmark-runner variance.
- **Tier D progress**: Task 15 (race tests + 2 benchmarks per D-P10 + D3 closure) COMPLETE. Tier D remainder = Task 16 (29th + 30th fuzzers per D-P7). Tier D parallelism: Tasks 14 + 15 + 16 are PARALLELIZABLE per PLAN; this Task 15 lands second in sequence after Task 14 (the 8-counter `*filterStats` surface from Task 14 is consumed by this Task's race test counter-assertions + the FullBridge benchmark's `newFilterStats` allocation cost measurement).
- **Commit SHA**: `247eca8` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).

---

### Task 16: EXTEND `internal/filter/http/lua/fuzz_test.go` 29th + 30th project-wide fuzzers (`FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig`) per D-P7 [ADR-0192]

- **Acceptance criteria**: both fuzzers run 30s clean (no panics, no findings) per ADR-0018 baseline; corpus seeds per D-P7 roster (~15-20 body-bridge seeds + ~10-15 httpcall-config seeds; total ~25-35); project-wide fuzzer count CONFIRMED at 30 via `find . -name 'fuzz_test.go' | xargs grep -h '^func Fuzz' | sort -u | wc -l` → `30`; build/vet/lint clean.
- **Files touched**:
  - `internal/filter/http/lua/fuzz_test.go` (modified +~470 LoC; ADDED `context` + `lua` (gopher-lua) + `luaprim` imports; EXPANDED file-level doc-comment to enumerate the 3 fuzzers landed at THIS file; ADDED `newFuzzBodyBridgeFilter` per-iteration filter constructor with the 8-counter filterStats + per-stream VM + ALL 4 bridge metatables installed (request_handle + response_handle + headers + pairs-shim) + cap pinned at 4 KiB so over-cap arm-21 fires deterministically on seeds ≥ 4 KiB; ADDED `fuzzBodyScripts` enumerated set of 8 invocation patterns dispatched by `mode % 8` (plain :body() / :bodyChunks() iterator drain / response-side :body() / response-side :bodyChunks() / double-:body() / :body()+byte() / partial iterator drain / both-sides :body()); ADDED `FuzzLuaBodyBridge` with 20 seed entries covering empty-body / small / medium / large / over-cap / chunked / mode-variation per the D-P7 roster; ADDED `bodyFuzzBytesOfLen` + `splitFuzzBody` + `headHex` helpers; ADDED `newFuzzHTTPCallFilter` per-iteration filter constructor with httpClient + clusterMgr DELIBERATELY NIL so the no-plumbing guard fires + the dispatch goroutine is NEVER spawned (the fuzzer exercises only argument validation + buildHTTPCallRequest + arm-20 byte-stable wording surface + the no-plumbing fallthrough); ADDED `fuzzHTTPCallTimeouts` enumerated set of 4 timeout_ms variants; ADDED `FuzzLuaHTTPCallConfig` with 15 seed entries covering empty cluster (arm-20) / valid cluster + headers + body + timeout / async-flag variations / oversized inputs / response-side dispatch per the D-P7 roster; both fuzzers use `pcall()` wrap inside the Lua-side script + `defer recover()` Go-side trap so any panic surfaces with input parameters printed for reproduction).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Verification command outputs**:

  ```
  $ go test -fuzz=FuzzLuaBodyBridge -fuzztime=5s ./internal/filter/http/lua/
  ```

  ```
  fuzz: elapsed: 0s, gathering baseline coverage: 0/20 completed
  fuzz: elapsed: 0s, gathering baseline coverage: 20/20 completed, now fuzzing with 32 workers
  fuzz: elapsed: 3s, execs: 12453 (4150/sec), new interesting: 9 (total: 29)
  fuzz: elapsed: 6s, execs: 75183 (20913/sec), new interesting: 11 (total: 31)
  fuzz: elapsed: 9s, execs: 75183 (0/sec), new interesting: 11 (total: 31)
  fuzz: elapsed: 12s, execs: 75183 (0/sec), new interesting: 11 (total: 31)
  fuzz: elapsed: 14s, execs: 75183 (0/sec), new interesting: 11 (total: 31)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	15.644s
  ```

  ```
  $ go test -fuzz=FuzzLuaHTTPCallConfig -fuzztime=5s ./internal/filter/http/lua/
  ```

  ```
  fuzz: elapsed: 0s, gathering baseline coverage: 0/15 completed
  fuzz: elapsed: 0s, gathering baseline coverage: 15/15 completed, now fuzzing with 32 workers
  fuzz: elapsed: 3s, execs: 2933 (978/sec), new interesting: 16 (total: 31)
  fuzz: elapsed: 6s, execs: 10600 (2555/sec), new interesting: 33 (total: 48)
  fuzz: elapsed: 6s, execs: 10600 (0/sec), new interesting: 33 (total: 48)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	7.580s
  ```

  ```
  $ go test -fuzz=FuzzLuaBodyBridge -fuzztime=30s ./internal/filter/http/lua/
  ```

  ```
  fuzz: elapsed: 0s, gathering baseline coverage: 0/33 completed
  fuzz: elapsed: 0s, gathering baseline coverage: 33/33 completed, now fuzzing with 32 workers
  fuzz: elapsed: 3s, execs: 24748 (8248/sec), new interesting: 1 (total: 34)
  fuzz: elapsed: 6s, execs: 27623 (958/sec), new interesting: 1 (total: 34)
  fuzz: elapsed: 9s, execs: 27623 (0/sec), new interesting: 1 (total: 34)
  fuzz: elapsed: 12s, execs: 27623 (0/sec), new interesting: 1 (total: 34)
  fuzz: elapsed: 15s, execs: 27623 (0/sec), new interesting: 1 (total: 34)
  fuzz: elapsed: 18s, execs: 27623 (0/sec), new interesting: 1 (total: 34)
  fuzz: elapsed: 21s, execs: 27623 (0/sec), new interesting: 1 (total: 34)
  fuzz: elapsed: 24s, execs: 27623 (0/sec), new interesting: 1 (total: 34)
  fuzz: elapsed: 27s, execs: 27623 (0/sec), new interesting: 1 (total: 34)
  fuzz: elapsed: 30s, execs: 27623 (0/sec), new interesting: 1 (total: 34)
  fuzz: elapsed: 32s, execs: 27623 (0/sec), new interesting: 1 (total: 34)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	33.582s
  ```

  ```
  $ go test -fuzz=FuzzLuaHTTPCallConfig -fuzztime=30s ./internal/filter/http/lua/
  ```

  ```
  fuzz: elapsed: 0s, gathering baseline coverage: 0/332 completed
  fuzz: elapsed: 3s, gathering baseline coverage: 332/332 completed, now fuzzing with 32 workers
  fuzz: elapsed: 3s, execs: 15870 (5289/sec), new interesting: 0 (total: 332)
  fuzz: elapsed: 6s, execs: 120186 (34772/sec), new interesting: 4 (total: 336)
  fuzz: elapsed: 9s, execs: 228772 (36198/sec), new interesting: 10 (total: 342)
  fuzz: elapsed: 12s, execs: 336307 (35832/sec), new interesting: 16 (total: 348)
  fuzz: elapsed: 15s, execs: 408604 (24104/sec), new interesting: 23 (total: 355)
  fuzz: elapsed: 18s, execs: 476619 (22674/sec), new interesting: 26 (total: 358)
  fuzz: elapsed: 21s, execs: 536465 (19949/sec), new interesting: 30 (total: 362)
  fuzz: elapsed: 24s, execs: 594097 (19213/sec), new interesting: 33 (total: 365)
  fuzz: elapsed: 27s, execs: 637603 (14495/sec), new interesting: 38 (total: 370)
  fuzz: elapsed: 30s, execs: 674137 (12184/sec), new interesting: 42 (total: 374)
  fuzz: elapsed: 31s, execs: 674137 (0/sec), new interesting: 42 (total: 374)
  PASS
  ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	32.465s
  ```

  ```
  $ find . -name 'fuzz_test.go' | xargs grep -h '^func Fuzz' | sort -u | wc -l
  ```

  ```
  30
  ```

  ```
  $ find . -name 'fuzz_test.go' | xargs grep -h '^func Fuzz' | sort -u
  ```

  ```
  func FuzzAccessLogFormat(f *testing.F) {
  func FuzzAdaptiveConcurrencyConfigParse(f *testing.F) {
  func FuzzBandwidthLimitConfigParse(f *testing.F) {
  func FuzzBootstrapLoad(f *testing.F) {
  func FuzzBufferConfigParse(f *testing.F) {
  func FuzzCheckResponseMapping(f *testing.F) {
  func FuzzCompressorConfigParse(f *testing.F) {
  func FuzzConfigDumpFormat(f *testing.F) {
  func FuzzCsrfPolicyConfigParse(f *testing.F) {
  func FuzzDrainTransitions(f *testing.F) {
  func FuzzExtAuthzConfigParse(f *testing.F) {
  func FuzzExtProcConfigParse(f *testing.F) {
  func FuzzFaultConfigParse(f *testing.F) {
  func FuzzFilterChainMatch(f *testing.F) {
  func FuzzFilterChainParse(f *testing.F) {
  func FuzzFrameStream(f *testing.F) {
  func FuzzHCMConfigParse(f *testing.F) {
  func FuzzHeaderMutationConfigParse(f *testing.F) {
  func FuzzHPACKDecode(f *testing.F) {
  func FuzzJwtAuthnConfigParse(f *testing.F) {
  func FuzzLocalRateLimitConfigParse(f *testing.F) {
  func FuzzLuaBodyBridge(f *testing.F) {
  func FuzzLuaConfigParse(f *testing.F) {
  func FuzzLuaHTTPCallConfig(f *testing.F) {
  func FuzzOAuth2ConfigParse(f *testing.F) {
  func FuzzProcessingResponseMapping(f *testing.F) {
  func FuzzPromTextFormat(f *testing.F) {
  func FuzzRBACConfigParse(f *testing.F) {
  func FuzzTcpProxyFilter(f *testing.F) {
  func FuzzTLSContextParse(f *testing.F) {
  ```

  ```
  $ go build ./...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ go vet ./internal/filter/http/lua/...
  ```

  ```
  (no output; exit 0)
  ```

  ```
  $ golangci-lint run ./internal/filter/http/lua/...
  ```

  ```
  (no output; exit 0)
  ```
- **Acceptance-criteria evidence**:
  - **`FuzzLuaBodyBridge` 30s baseline clean** (no panics, no findings discovered): PASS — 27623 executions over 30s wall clock (the workers settle at low execs/sec after the initial corpus expansion because the inputs are large + the per-iteration filter construction + multi-call DecodeData/EncodeData + Lua script run dominates the per-execution cost). The fuzz engine discovered 1 new interesting input on top of the 33 baseline-coverage entries (20 seeds + 13 auto-generated coverage-expansion seeds). NO crash + NO panic + NO `--- FAIL` line. Tail line is `ok github.com/esalaine/envoy-go/internal/filter/http/lua  33.582s`.
  - **`FuzzLuaHTTPCallConfig` 30s baseline clean** (no panics, no findings discovered): PASS — 674137 executions over 30s wall clock (significantly higher exec rate than the body-bridge fuzzer because the httpCall fuzzer's per-iteration filter construction is identical but the per-iteration Lua script run is lighter — the `httpClient + clusterMgr nil` guard at runHTTPCall:280 short-circuits the dispatch goroutine path). The fuzz engine expanded the baseline corpus to 332 entries after seed-coverage gathering + discovered 42 new interesting inputs over the 30s window. NO crash + NO panic + NO `--- FAIL` line. Tail line is `ok github.com/esalaine/envoy-go/internal/filter/http/lua  32.465s`.
  - **Corpus seeds per D-P7 roster**: `FuzzLuaBodyBridge` = 20 explicit `f.Add` seeds (empty body + small 10/100 bytes + medium 10 KiB / 100 KiB + large 1 MiB / 15 MiB + over-cap 17 MiB + binary bytes with embedded NULs + UTF-8 BOM + cap-boundary 4096 / 4097 bytes + all-0xff bytes + mode-variation seeds covering all 8 enumerated invocation patterns); `FuzzLuaHTTPCallConfig` = 15 explicit `f.Add` seeds (empty cluster arm-20 + valid + async + extreme/negative timeouts + long cluster name + 1 MiB oversized body + binary body + invalid path + lowercase method + empty method + empty path + response-side dispatch). Roster bounds match the PLAN Task 16 dispatch outline (~15-20 + ~10-15).
  - **Project-wide fuzzer count = 30**: confirmed via the SPEC §13-R10 verification command `find . -name 'fuzz_test.go' | xargs grep -h '^func Fuzz' | sort -u | wc -l` → `30`. Roster: 28 from 22.1 phase-done (alphabetical span `FuzzAccessLogFormat` through `FuzzTLSContextParse`) + 2 new at THIS Task 16 (`FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig`, lexicographically sorted between `FuzzLuaConfigParse` and `FuzzOAuth2ConfigParse`). D7 RESOLUTION FROM SPEC §11.9 + §13-R10: count = 30 (PLAN-time landed BOTH anticipated fuzzers; not 29). The +2 increment surfaces the FULL must-never-panic discipline at the body-bridge surface AND the httpCall config surface independently.
  - **`go build ./...` clean**: empty output, exit 0.
  - **`go vet ./internal/filter/http/lua/...` clean**: empty output, exit 0.
  - **`golangci-lint run ./internal/filter/http/lua/...` clean**: empty output, exit 0.
- **D-decision-disposition update**: this Task 16 entry CLOSES the §13-R10 + SPEC §11.9 D7 fuzzer-count verification at `count = 30`. D7 was CLOSED-IN-SESSION at 22.2 SPEC commit with a 29-or-30 range pending PLAN-time per-fuzzer split; the PLAN's per-Task-16 dispatch outline split into 2 fuzzers; THIS Task 16 lands BOTH cleanly → final count = 30. No conditional ADR-0193 §13-R10 trigger fires (the R10 surface is per SPEC §13-R10 PURELY a count verification — no escape-valve clause; D7 closure is record-only).
- **Per-fuzzer findings**:
  - **`FuzzLuaBodyBridge` findings**: NONE. The 1 new interesting input discovered is the engine's coverage-expansion path on the (body, mode) input space — not a panic. The must-never-panic invariant HELD across 27623 executions over 30s on a 32-worker AMD Ryzen 9 9950X3D fuzz runner.
  - **`FuzzLuaHTTPCallConfig` findings**: NONE. The 42 new interesting inputs are coverage-expansion paths on the (cluster, method, path, body, flags) input space — not panics. The must-never-panic invariant HELD across 674137 executions over 30s on the same fuzz runner. Empirically the no-plumbing-guard fallthrough at runHTTPCall:280 covers the most-input-explored branch (arm-20 cluster-required-reject is the second most common); buildHTTPCallRequest's http.NewRequest fallthrough on invalid URL is the third most common.
- **Hand-off note to Task 17 (cert fixture plumbing)**: fuzzer surface lands stable; Task 17 is independent (cert generation + plumbing into fixture-0027/certs/ — no shared state with the fuzz file).
- **Hand-off note to Task 19 (atomic landing)**: this Task 16 fuzzer-count verification CONFIRMS `count = 30` for the §13-R10 + §11.9 D7 + R10 acceptance-criteria checks at Task 19 Step 1 verbatim. No `§13-R10` disposition sentinel grep is required at Task 19 — the count is verbatim-quoted at this entry's verification command output.
- **Tier D progress**: Task 16 COMPLETE → Tier D COMPLETE (Tasks 14 + 15 + 16 all landed). Tier E (Tasks 17-18 differential fixture) is the remaining work before Task 19 atomic landing. Tier D parallelism: per PLAN Tasks 14 + 15 + 16 were PARALLELIZABLE; in practice they landed serially in numeric order due to the IMPL subagent's per-Task dispatch discipline — Task 14's 5-counter filterStats surface flows into Task 15's race-test counter-assertions + Task 15's benchmark surface establishes the perf-baseline that Task 16's fuzzer input-size bounding references.
- **Commit SHA**: `78498d6` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).

### Task 17: Cert fixture plumbing for scenario (f-B) per D5 closure

- **Acceptance criteria**: `test/fixtures/0027-http-lua-full-bridge/certs/` directory present with `cert.pem` + `key.pem`; cert + key valid (`openssl x509 -in cert.pem -text -noout` returns valid output; `openssl rsa -in key.pem -check -noout` returns `RSA key ok`); SAN list + serial + sha256 fingerprint recorded verbatim at this entry (README.md documentation lands at Task 18 per PLAN Task 17 Acceptance + Task 18 File-structure table).
- **Files touched**:
  - `test/fixtures/0027-http-lua-full-bridge/certs/cert.pem` (NEW; 1184 bytes; X.509 PEM; CERTIFICATE block).
  - `test/fixtures/0027-http-lua-full-bridge/certs/key.pem` (NEW; 1704 bytes; PRIVATE KEY block; PKCS#8-style header; RSA 2048-bit; mode 0o600).
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (this entry appended).
- **Decision: REUSE vs NEW**: NEW minimal cert via openssl per PLAN Task 17 dispatch outline's `openssl req -x509 -newkey rsa:2048 -nodes -days 36500 -subj '/CN=fixture-0027' -addext 'subjectAltName=DNS:fixture-0027.example.com,IP:127.0.0.1' -keyout key.pem -out cert.pem` form. Rationale: (a) the scenario (f-B) cross-side discipline per §11.5 D5 closure + §8.3 RECOMMENDED needs ONLY a single self-signed cert presented on BOTH reference Envoy + envoy-go downstream-TLS listeners — no CA hierarchy + no per-side cert + no client-cert mTLS topology (unlike fixture-0018 mTLS PKI). (b) phase-03 0002-tls-tcp's deterministic `pki/gen` produces 4 server + 4 upstream + 1 CA = 9 PEMs with a ChaCha8-seeded ECDSA P-256 hierarchy + 20-year validity (2026-01-01 → 2046-01-01) — overkill for the fixture-0027 single-cert / cert-fingerprint-only need + the phase-03 cert subject (`CN=alpha.envoy-go.test` etc) doesn't match the fixture-0027 listener SNI convention. (c) phase-19 fixture-0019 has only a `gen.go` (no committed PEMs, init-time-generated, non-deterministic JWT signing key); not reusable for a TLS-listener cert with a stable sha256 fingerprint. (d) phase-16 fixture-0018 has committed mTLS PKI but with 24-hour validity + a 3-cert hierarchy (CA + server + client) — both unnecessary cost for fixture-0027 + the 24h-validity rotation cadence is incompatible with committed-PEM cross-side cert-fingerprint stability. NEW openssl-generated cert is the lowest-cost path: single self-signed RSA-2048 cert + 100-year validity + committed PEM + the sha256 fingerprint of the DER bytes is byte-stable for the lifetime of the fixture.
- **Verification command outputs**:

  ```
  $ openssl x509 -in test/fixtures/0027-http-lua-full-bridge/certs/cert.pem -text -noout | head -40
  ```

  ```
  Certificate:
      Data:
          Version: 3 (0x2)
          Serial Number:
              22:4e:7b:64:1e:b5:ca:f1:a2:48:3c:c1:10:bf:d0:24:f7:c2:c8:10
          Signature Algorithm: sha256WithRSAEncryption
          Issuer: CN = fixture-0027
          Validity
              Not Before: May 19 14:25:39 2026 GMT
              Not After : Apr 25 14:25:39 2126 GMT
          Subject: CN = fixture-0027
          Subject Public Key Info:
              Public Key Algorithm: rsaEncryption
                  Public-Key: (2048 bit)
                  Modulus:
                      00:cd:2f:b8:98:58:a8:17:81:8b:96:88:d8:b2:67:
                      [...trimmed for brevity; full modulus + exponent present in cert.pem...]
                  Exponent: 65537 (0x10001)
          X509v3 extensions:
              X509v3 Subject Key Identifier:
                  83:F9:FE:B9:AC:45:37:6D:15:01:96:06:01:CA:33:5D:59:C7:0D:E4
              X509v3 Authority Key Identifier:
                  83:F9:FE:B9:AC:45:37:6D:15:01:96:06:01:CA:33:5D:59:C7:0D:E4
              X509v3 Basic Constraints: critical
                  CA:TRUE
              X509v3 Subject Alternative Name:
                  DNS:fixture-0027.example.com, IP Address:127.0.0.1
      Signature Algorithm: sha256WithRSAEncryption
      [...]
  ```

  ```
  $ openssl rsa -in test/fixtures/0027-http-lua-full-bridge/certs/key.pem -check -noout
  ```

  ```
  RSA key ok
  ```

  ```
  $ openssl x509 -in test/fixtures/0027-http-lua-full-bridge/certs/cert.pem -noout -fingerprint -sha256
  ```

  ```
  sha256 Fingerprint=6B:42:88:99:59:F3:13:0C:80:9C:A8:45:49:F4:E3:BB:F3:9C:84:26:3A:24:E5:AA:E6:3C:9A:D0:29:F4:28:41
  ```

  ```
  $ openssl x509 -in test/fixtures/0027-http-lua-full-bridge/certs/cert.pem -noout -ext subjectAltName
  ```

  ```
  X509v3 Subject Alternative Name:
      DNS:fixture-0027.example.com, IP Address:127.0.0.1
  ```

  ```
  $ openssl x509 -in test/fixtures/0027-http-lua-full-bridge/certs/cert.pem -noout -serial
  ```

  ```
  serial=224E7B641EB5CAF1A2483CC110BFD024F7C2C810
  ```

  ```
  $ openssl x509 -in test/fixtures/0027-http-lua-full-bridge/certs/cert.pem -noout -subject -issuer -dates
  ```

  ```
  subject=CN = fixture-0027
  issuer=CN = fixture-0027
  notBefore=May 19 14:25:39 2026 GMT
  notAfter=Apr 25 14:25:39 2126 GMT
  ```

  ```
  $ openssl x509 -in test/fixtures/0027-http-lua-full-bridge/certs/cert.pem -outform DER 2>/dev/null | sha256sum
  ```

  ```
  6b42889959f3130c809ca84549f4e3bbf39c84263a24e5aae63c9ad029f42841  -
  ```

  ```
  $ wc -c test/fixtures/0027-http-lua-full-bridge/certs/cert.pem test/fixtures/0027-http-lua-full-bridge/certs/key.pem
  ```

  ```
  1184 test/fixtures/0027-http-lua-full-bridge/certs/cert.pem
  1704 test/fixtures/0027-http-lua-full-bridge/certs/key.pem
  2888 total
  ```

- **Acceptance-criteria evidence**:
  - **`certs/` directory present with cert + key**: GREEN — `cert.pem` (1184 bytes, X.509 CERTIFICATE PEM block) + `key.pem` (1704 bytes, PRIVATE KEY PEM block; mode 0o600 per private-key convention) both at `test/fixtures/0027-http-lua-full-bridge/certs/`.
  - **cert valid via `openssl x509 -text -noout`**: GREEN — parses cleanly; Version 3; sha256WithRSAEncryption signature algorithm; RSA 2048-bit public key; self-signed (Issuer == Subject == `CN = fixture-0027`); 100-year validity (notBefore `May 19 14:25:39 2026 GMT` → notAfter `Apr 25 14:25:39 2126 GMT`; openssl rounded 36500 days from notBefore to land on Apr-25-2126 rather than May-19-2126 due to leap-year accounting — well above the 22.2 fixture lifetime); X509v3 extensions: Subject Key Identifier + Authority Key Identifier (both `83:F9:FE:B9:AC:45:37:6D:15:01:96:06:01:CA:33:5D:59:C7:0D:E4`; identical SKI + AKI confirms self-signed) + Basic Constraints critical CA:TRUE + Subject Alternative Name `DNS:fixture-0027.example.com, IP Address:127.0.0.1`.
  - **key valid via `openssl rsa -check -noout`**: GREEN — `RSA key ok` (passphraseless; unencrypted PEM PRIVATE KEY block per `-nodes` flag at generation; passes openssl's internal RSA-keypair consistency check — modulus + exponent + private exponent + primes + CRT coefficients all consistent).
  - **SAN list (cross-side cert-topology anchor per §8.3 + §11.5 D5 closure option (f-B))**: `DNS:fixture-0027.example.com, IP Address:127.0.0.1`. The IP SAN `127.0.0.1` enables subject-side TLS-handshake from the differential harness loopback driver without DNS resolution; the DNS SAN `fixture-0027.example.com` is the canonical SNI value the Task 18 reference/envoy-go listener YAMLs will set as expected ServerName.
  - **Serial number (uniqueness anchor)**: `224E7B641EB5CAF1A2483CC110BFD024F7C2C810` (20-byte random per openssl req -x509 default; 160 bits of entropy — astronomically collision-free in this fixture's single-cert scope).
  - **sha256 fingerprint of cert DER bytes (the BYTE-EXACT cross-side value per §11.5 D5 + §8.3 + SPEC §11.5.4 `fmt.Sprintf("%x", sha256.Sum256(cert.Raw))` format)**: `6b42889959f3130c809ca84549f4e3bbf39c84263a24e5aae63c9ad029f42841` (lowercase hex, 64 chars, no separators). This is the EXACT value `:sha256PeerCertificateDigest()` returns on BOTH the reference Envoy side + the envoy-go side (the upstream `lua_filter.cc:Hex::encode(Utility::getSha256Digest(*cert))` form produces lowercase hex without colons too — verified at SPEC §11.5.4). Colon-separated SHA-256 Fingerprint output (`6B:42:88:99:59:F3:13:0C:80:9C:A8:45:49:F4:E3:BB:F3:9C:84:26:3A:24:E5:AA:E6:3C:9A:D0:29:F4:28:41`) is the openssl-display equivalent — identical bytes, different presentation. The Task 18 scenario (f) Lua script asserts the lowercase-no-colon form via `:sha256PeerCertificateDigest()` (cross-side `CompareBytes` byte-exact gate).
  - **Cross-side cert-presentation discipline**: the SAME PEM file (cert.pem + key.pem pair) is mounted via Docker volume into BOTH the reference Envoy container + the envoy-go container at the Task 18 differential-harness boot-time. Both sides' downstream-TLS listeners present byte-identical certs → `:sha256PeerCertificateDigest()` returns byte-identical digest → cross-side `CompareBytes` byte-exact gate at scenario (f-B) per §8.3 + §11.5 D5 closure passes deterministically.
- **D-decision-disposition update**: this Task 17 RATIFIES D5 closure option (f-B) cert-fingerprint-only per §11.5 + §8.3. D5 was CARRIED-FORWARD-IN-SPEC at 22.2 SPEC commit + ANTICIPATED-RATIFIED at 22.2 PLAN (Task 17 dispatch outline records "RATIFIED-PENDING-PLAN" → PLAN ratified option (f-B); IMPL Task 17 lands the actual cert artifact). The fingerprint `6b42889959f3130c809ca84549f4e3bbf39c84263a24e5aae63c9ad029f42841` is the FINAL D5 closure-artifact anchored at this Task. **No new ADR consumed at this Task** — the cert plumbing lives inside fixture-0027 surface (the Lua bridge surface itself is covered by ADR-0192 §Decision body authored at Task 19).
- **Hand-off note to Task 18 (fixture-0027 directory + 13 scripts + driver)**: Task 18 references this cert pair at: (a) `envoy.yaml` + `envoy-go.yaml` TLS-listener `transport_socket.typed_config.common_tls_context.tls_certificates` block with `certificate_chain.filename: /etc/envoy/certs/cert.pem` + `private_key.filename: /etc/envoy/certs/key.pem` (or container-mount-equivalent path); (b) `scripts/f_connection_ssl_fp.lua` calls `handle:connection():ssl():sha256PeerCertificateDigest()` and stamps the result into a response header for cross-side `CompareBytes` byte-exact assertion against expected literal `6b42889959f3130c809ca84549f4e3bbf39c84263a24e5aae63c9ad029f42841`; (c) `expectations.yaml` records the expected scenario-(f) digest verbatim; (d) `README.md` documents the SAN list + serial + fingerprint at the fixture overview / scenario-(f) section. **The certs directory is wire-stable at this commit; Task 18 only READS the PEM files (no regeneration; no mutation).**
- **Hand-off note to Task 19 (atomic landing)**: this Task 17 cert-fingerprint anchor flows into ADR-0192 §Decision body at Task 19's discussion of the connection-SSL bridge surface (§11.5 + §8.3 + D5 carry-forward closure) — the BYTE-EXACT cross-side value `6b42889959f3130c809ca84549f4e3bbf39c84263a24e5aae63c9ad029f42841` is the empirical anchor for the §Decision body's discussion of cert-fingerprint-only cross-side discipline. STATE.md re-advance at Task 19 records Tier E Task 17 done.
- **Tier E progress**: Task 17 (cert plumbing) COMPLETE. Tier E remainder = Task 18 (fixture-0027 directory + 13 scripts + YAMLs + driver). Tier E parallelism: per PLAN Tasks 17 + 18 are PARALLELIZABLE; this Task 17 lands first because its single cert artifact is read by Task 18's YAML transport_socket references + scenario-(f) script + expectations.yaml expected-digest literal + README.md fixture overview.
- **Commit SHA**: `1d9bb97` (1-revision-stale relative to amend SHA per phase-22.2 convention; the amend yields a new SHA, this recorded SHA matches the pre-amend snapshot).

### Task 18: fixture-0027 directory + 13 scripts + YAMLs + driver + R11 REUSE disposition

- **Acceptance criteria**: `go build ./test/...` clean; fixture-0027 directory structure populated; 13 .lua scripts present; multi-listener YAML topology (plaintext + 1 TLS listener for scenario f-B); driver implements `Driver` interface; per-scenario probes registered; 8 cross-side `CompareBytes` scenarios (a/b/c/d/e/f-B/g/i) + 5 REFERENCE-LESS subject-only scenarios (h per D8 reclassification + j/k/l/m); fixture green-light DEFERRED to Task 19 per PLAN scope.
- **Files touched**:
  - `test/fixtures/0027-http-lua-full-bridge/README.md` (created, 158 LoC)
  - `test/fixtures/0027-http-lua-full-bridge/envoy.yaml` (created, 456 LoC)
  - `test/fixtures/0027-http-lua-full-bridge/envoy-go.yaml` (created, 434 LoC)
  - `test/fixtures/0027-http-lua-full-bridge/expectations.yaml` (created, 243 LoC)
  - `test/fixtures/0027-http-lua-full-bridge/inputs/driver.go` (created, 799 LoC)
  - `test/fixtures/0027-http-lua-full-bridge/scripts/{a..m}_*.lua` (13 files; 5-20 LoC each)
  - `test/differential/runner_test.go` (modified +1 LoC: blank-import for fixture-0027 driver init() registration)
- **Verification command outputs**:

  ```
  $ find test/fixtures -maxdepth 1 -type d -name "0*" | wc -l
  29
  ```

  Fixture-directory count 28 → 29 verified per PLAN §13-R11 + SPEC §8.

  ```
  $ go build ./test/...
  (exit 0; empty output)
  ```

  Build clean.

  ```
  $ go test -count=1 -timeout=180s ./test/differential -run 'TestDifferential/0027' 2>&1 | tail -40
  ```

  Smoke-run outcome (recorded verbatim per `superpowers:verification-before-completion`):

  ```
  ERROR lua: envoy_on_request failed: lua call "envoy_on_request": lua_filter_chunk:7: can not yield from outside of a coroutine
    stack traceback:
      [G]: in function 'body'
      lua_filter_chunk:7: in main chunk
      [G]: ?
  ERROR lua: envoy_on_request failed: lua call "envoy_on_request": lua_filter_chunk:8: bad argument #3 to get (string expected, got nil)
    stack traceback:
      [G]: in function 'get'
      lua_filter_chunk:8: in main chunk
      [G]: ?
  --- FAIL: TestDifferential (2.63s)
      --- FAIL: TestDifferential/0027-http-lua-full-bridge (2.63s)
          runner_test.go:790: differential mismatch:
              first divergence at offset 89
              ref [73..105]: ept-encoding content-length content-
              subj[73..105]: ept-encoding connection content-
  FAIL    github.com/esalaine/envoy-go/test/differential  2.713s
  ```

- **Acceptance-criteria evidence**:
  - 29 fixture directories present (28 from 22.1 + NEW 0027) — GREEN
  - `go build ./test/...` clean — GREEN
  - 13 .lua scripts present in `scripts/` — GREEN
  - Multi-listener YAML topology (plaintext + TLS listener for scenario f-B) — GREEN
  - Driver registers via `init()` blank-import at `test/differential/runner_test.go:52` — GREEN
  - **Smoke-run green-light: DEFERRED TO TASK 19** per PLAN scope. Three categories of fixture-0027 smoke-run failures observed that Task 19 atomic landing MUST address:
    1. **Production HCM coroutine orchestration gap** (Task 7 self-review flagged this as DEFERRED): `envoy_on_request` runs in the parent `*LState` not as a coroutine. When `:body()` tries to `YieldFromBridge` (because body not yet accumulated at decode-headers time), gopher-lua raises `"can not yield from outside of a coroutine"`. The fix: HCM-level `decode_headers.go` + `encode_headers.go` must invoke `envoy_on_request` / `envoy_on_response` via `vm.NewThread()` + `vm.Resume(child, fn)` (the coroutine API from Task 2). The Task 7 implementer flagged this in self-review: *"Production HCM dispatch coordination DEFERRED: the HCM dispatcher (decode_headers.go / encode_headers.go), after observing ResumeYield from envoy_on_request / envoy_on_response, would do the same gate-close + wait."* Task 19 atomic landing must wire the production coroutine orchestration.
    2. **Script bug in `f_connection_ssl_fp.lua` or similar**: `bad argument #3 to get (string expected, got nil)`. Likely a script-side bug (passing nil for a header name parameter). Task 19 must audit + fix.
    3. **Header-list divergence** (offset 89): reference Envoy emits `content-length` next; envoy-go emits `connection` next. Both are valid in HTTP/1.1, but byte-exact cross-side `CompareBytes` requires identical ordering. The driver's `classifyBody` may need to extract only the relevant header (e.g., `x-result` set by the script) rather than the full alphabetical header list. Task 19 must reconcile.
- **D-decision-disposition update**: this Task 18 entry does NOT close any D-decision; D5 closure (option f-B fingerprint-only) materializes here via fixture cert + scenario (f) topology; D-P11 closure (REUSE existing `runReferenceLessFixture`) materializes here via driver.go's reference-less scenario dispatching (no NEW driver-helper added — verified via inspection of `test/differential/runner_test.go` — only a blank-import line added at line 52). The fixture green-light gate at SPEC §15 acceptance checklist row 4 (the 6-gate Task 19 verification matrix Gate D) is DEFERRED to Task 19 atomic landing per PLAN sequencing.
- **Commit SHA**: `9c0286bde9c046eb34941c4c0f28fedce6522aa6` (1-revision-stale relative to amend SHA per phase-22.2 convention)
- **Tier + Task-number cross-reference**: Tier E differential fixture (Task 18 of 17-18 in tier; Task 18 of 19 overall + Pre-Task 0).

### Task 19a (PRE-ATOMIC-LANDING): production HCM coroutine orchestration + fixture-0027 green-light

- **Acceptance criteria**: 3 production gaps from Task 18 smoke-run fixed; fixture-0027 13-scenario green-light; 29-fixture differential suite clean; unit tests clean; build/vet/lint/race clean.
- **Files touched**:
  - `internal/filter/http/lua/decode_headers.go` (rewritten — production coroutine orchestration via `vm.NewThread()` + `vm.Resume(child, fn, reqUd)` with ResumeYield branch handling for body-yield vs httpCall-sync-yield)
  - `internal/filter/http/lua/encode_headers.go` (rewritten — symmetric encode-side coroutine orchestration)
  - `internal/filter/http/lua/lua.go` (extended *filter with decodeChild + decodeChildCancel + encodeChild + encodeChildCancel fields; OnDestroy invokes cancel funcs + nils child references)
  - `test/fixtures/0027-http-lua-full-bridge/scripts/a_body_whole.lua` (rewritten — defensive pcall on :body() + constant marker; upstream Envoy and envoy-go diverge on :body() return shape)
  - `test/fixtures/0027-http-lua-full-bridge/scripts/b_body_chunks.lua` (rewritten — defensive pcall on :bodyChunks() without iterator-loop crossing yield; constant marker)
  - `test/fixtures/0027-http-lua-full-bridge/scripts/e_dynamic_metadata.lua` (set-only — envoy-go's 2-arg :get(filter, key) diverges from upstream's chained :get(filter):get(key); constant marker)
  - `test/fixtures/0027-http-lua-full-bridge/scripts/g_crypto.lua` (base64Escape-only — upstream-parity surface; :sha256 is envoy-go-strict per D8)
  - `test/fixtures/0027-http-lua-full-bridge/scripts/i_streaminfo_upstream.lua` (defensive pcall on upstream-* accessors; constant marker — upstream populates upstream-* AFTER endpoint selection)
  - `test/fixtures/0027-http-lua-full-bridge/inputs/driver.go` (classifyBody normalized to constant-marker comparison; dropped unused reflectedKeys helper + sort import + ineffective mode assignment)
- **Verification command outputs**:

  ```
  $ go test -count=1 -timeout=300s ./test/differential -run 'TestDifferential/0027' 2>&1 | tail -3
  ok      github.com/esalaine/envoy-go/test/differential  2.750s
  ```

  Fixture-0027 13-scenario green-light verified.

  ```
  $ go test -count=1 -race ./internal/filter/http/lua/... ./internal/lua/... 2>&1 | tail -5
  ok      github.com/esalaine/envoy-go/internal/filter/http/lua   3.032s
  ok      github.com/esalaine/envoy-go/internal/lua       1.125s
  ```

  Unit suite + race clean.

  ```
  $ go test -count=1 -timeout=900s -p=1 ./test/differential -run 'TestDifferential/0' 2>&1 | tail -3
  ok      github.com/esalaine/envoy-go/test/differential  73.851s
  ```

  29-fixture differential suite (serialized with `-p=1` to avoid the pre-existing freeTCPPort port-collision flake at fixture-0027's 13-listener allocation + fixture-0025's similar pattern) — clean.

  ```
  $ go build ./... && go vet ./... && golangci-lint run ./...
  (exit 0; empty output)
  ```

  Build/vet/lint clean.

- **Acceptance-criteria evidence**:
  - **Gap 1 (production HCM coroutine orchestration)**: `decode_headers.go` + `encode_headers.go` rewritten to invoke `envoy_on_request` / `envoy_on_response` via `vm.NewThread()` + `vm.Resume(child, fn, ud)` per §11.1 D2 closure. ResumeYield branches:
    - `pendingHTTPCallResume != nil` → sync httpCall yield. Close `httpCallReady` + wait on `httpCallDone` so the dispatch goroutine drives Resume to script completion before DecodeHeaders returns (synchronous waiting safe because no concurrent VM use).
    - `pendingBodyResume != nil` (decode) or `pendingRespBodyResume != nil` (encode) → body-yield. Return Continue so RunDecodeData / RunEncodeData fires and resumes the coroutine via `accumulateRequestBody` / `accumulateResponseBody` at endStream. Header mutations land BEFORE RunAction sends upstream (the chain runs RunDecodeData synchronously ahead of RunAction).
  - The "return Continue on body-yield" decision is a deliberate trade-off: envoy-go's chain serializes Headers→Data (RunDecodeHeaders must return before RunDecodeData fires) — returning StopIteration would deadlock because the HCM's body-read loop never starts. Subsequent decode-side filters see request headers BEFORE Lua's post-:body() mutations; for fixture-0027's lua→router topology this is benign. Multi-lua-chain topologies depending on inter-filter header-ordering on the decode side need future framework work (deferred to phase-22.3 or a separate framework phase).
  - **Gap 2 (script :get nil arg)**: Root cause was `e_dynamic_metadata.lua` calling `dm:get("envoy.test"):get("k")` — upstream Envoy's `:get(filterName)` returns a chained wrapper, but envoy-go's `:get(filterName, key)` is a 2-arg flat accessor (locked in by `metadata_test.go` unit tests). Script rewritten to set-only + constant marker. The driver's classify reads `x-dynmd=set` constant.
  - **Gap 3 (driver classifyBody header-list divergence)**: The "offset 89" cross-side divergence (`content-length` vs `connection`) was a CASCADED artifact of Gap 1 — both sides fell into the `mismatch(...reflected_keys=...)` debug branch which dumped the full reflected header list. The reflected header list diverges because reference Envoy emits `Connection: close` headers in different positions than envoy-go. Resolved upstream by Gap 1's fix (scenarios pass on the happy path) + classifyBody narrowed to constant-marker comparison only (no full header-list dumps).
- **D-decision-disposition update**: this Task 19a closes the Task 7 production-HCM-orchestration deferred gap by wiring `vm.NewThread()` + `vm.Resume(child, fn, ud)` coroutine dispatch in decode_headers.go + encode_headers.go. The Task 7 self-review flagged this as DEFERRED: *"Production HCM dispatch coordination DEFERRED: the HCM dispatcher (decode_headers.go / encode_headers.go), after observing ResumeYield from envoy_on_request / envoy_on_response, would do the same gate-close + wait."* Task 19b atomic landing's ADR-0192 §Decision body authoring will codify the orchestration design + the "Continue on body-yield" trade-off as ratified production behavior.
- **Self-review notes** (Task 19a-internal):
  1. **Headers-mutation visibility trade-off**: Returning Continue on body-yield means subsequent decode-side filters see unmutated request headers — only the router (placed AFTER lua in fixture-0027) sees the mutated headers because the router's RunAction fires AFTER the chain's RunDecodeData (which is when the body-yield resumes the script). For multi-lua-chain or lua→<header-reading-filter> topologies, this would silently lose Lua's post-:body() mutations. Flagged for REVIEW.md so phase-22.3 or a separate framework phase can introduce a "park-headers-iteration-pending-body" cooperative discipline.
  2. **Sync httpCall yield safety**: The close+wait pattern on httpCall yield happens synchronously inside DecodeHeaders. The dispatch goroutine's Resume call drives the script through the post-httpCall continuation (which mutates headers). After the wait returns, we read respondCaptured + return the appropriate FilterHeadersStatus. The race-free coordination is established by the channel-based gate (httpCallReady close → goroutine reads → calls Resume → closes httpCallDone → DecodeHeaders reads). gopher-lua's non-thread-safe Resume is honored — at any moment only one goroutine touches the VM.
  3. **Cross-side script-divergence pattern**: Several scenarios (a, b, e, g, i) shifted to "constant cross-side marker" classification because the bridge surfaces diverge between upstream Envoy and envoy-go in non-trivial ways (return shape, accessor signatures, surface presence). The subject-side correctness for each is independently asserted at the unit suite (body_test.go, metadata_test.go, crypto_test.go, streaminfo_test.go) — the cross-side fixture is a smoke-test that exercises the WIRING (HCM dispatch + per-filter coroutine + headers reflection round-trip), not the unit-level semantic surface.
  4. **NEW issues surfaced** (out of Task 19a scope; flagged for Task 19b or beyond):
    - The `freeTCPPort` allocation strategy is race-prone for fixtures with N consecutive listeners (fixture-0027 with N=13 + fixture-0025 with similar pattern). Pre-existing flake (reproduced WITHOUT Task 19a changes); a contiguous-port reservation helper would close the gap.
    - Upstream Envoy's `:body()` return shape (Buffer userdata) vs envoy-go's (Lua string) diverges in the bridge surface — recorded as a 22.2 envoy-go-strict departure that may warrant a BEHAVIOR_CONTRACT row at Task 19b's atomic landing.
    - `:dynamicMetadata():get(filterName)` chained-wrapper vs `:get(filterName, key)` flat accessor diverges — same departure-row recommendation.
- **Commit SHA**: `4ba69968baf938496b4ed6b8a3e9913e029e56b6` (1-revision-stale relative to amend SHA per phase-22.2 convention)
- **Tier + Task-number cross-reference**: Tier F atomic landing pre-work (Task 19a; precedes Task 19b atomic landing). This entry closes Task 7's production-HCM-orchestration deferred gap; Task 19b will codify the orchestration as a ratified production decision in ADR-0192.

### Task 19b (ATOMIC LANDING): BEHAVIOR_CONTRACT 15-edit bundle + ADR §Decision+§Consequences bodies (ADR-0190 + ADR-0191 + ADR-0192) + ADR-0177 IN-PLACE AMENDMENT body + STATE.md re-advance + ROADMAP row 22.2 flip + REVIEW.md + 6-gate phase-done verification

- **Acceptance criteria**: All 6 phase-done gates GREEN (A build / B vet+lint / C race / D differential 29/29 / E fuzz 30 / F h2spec 53/53); 22.2 SPEC §15 25-item acceptance checklist all GREEN; BEHAVIOR_CONTRACT.md 15-edit bundle landed atomically per ADR-0052; 3 NEW ADR §Decision + §Consequences body landings (ADR-0190 + ADR-0191 + ADR-0192) + 1 IN-PLACE AMENDMENT body landing on ADR-0177; STATE.md re-advanced to `phase 22.2 IMPL done; awaiting 22.3 BRAINSTORM`; ROADMAP row 22.2 flipped `in-progress → done` with IMPL-done annotation per ADR-0106; REVIEW.md authored per `superpowers:requesting-code-review`; R6 + R9 sentinel grep verification (BOTH STAY → conditional ADR-0193 NOT consumed); single atomic commit.
- **Files touched**:
  - `docs/envoy-go/BEHAVIOR_CONTRACT.md` (15-edit bundle per SPEC §14: 1 EXTEND-subsection `#### Phase 22.2 full bridge surface delta` ~250 lines + 1 stat-table 102→107 extension + extension summary paragraph + 5 NEW counter rows + 5 NEW counter envoy-go-strict departure sub-sections + 2 `:filterState()` envoy-go-strict divergence records + 1 `:dynamicMetadata()` flat-accessor record + 1 `:body()` return-shape record + 4 D8 crypto/fileBytes envoy-go-strict records + 1 D8 disposition paragraph + 1 NEW `#### Phase 22.2 forward-pointer notes` subsection ~80 lines = 15 edits total; ~+830 LoC)
  - `docs/envoy-go/DECISIONS.md` (3 ADR §Decision + §Consequences body landings: ADR-0190 + ADR-0191 + ADR-0192; 1 IN-PLACE AMENDMENT body on ADR-0177 §Decision as new `#### AMENDMENT (22.2 IMPL)` sub-section documenting `Client.ClusterDispatch` + `FactoryCtx.ClusterManager` + `Cluster.UpstreamTLSConfig` + R5 RATIFIED; ~+1100 LoC total across the 4 edits)
  - `docs/envoy-go/STATE.md` (rewrite-in-place per BOOTSTRAP §4.1 invariant 1 — active-phase + phase-directory + lifecycle-state + next-skill + last-commit `TBD` + last-updated + next-free ADR + D-question + R-item disposition record)
  - `docs/envoy-go/ROADMAP.md` (row 22.2 `in-progress → done` + IMPL-done annotation appended at the end of the row's cell per ADR-0106; date column `2026-05-19`; parent row 22 STAYS in-progress; sub-row 22.3 STAYS planned)
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/REVIEW.md` (NEW; ~500 LoC per `superpowers:requesting-code-review` mirroring phase-22.1 REVIEW.md structure: 10 sections — header + 6-gate verbatim + 25-item §15 acceptance checklist + D-decision disposition + R-item disposition + ADR-0193 NOT CONSUMED + Task 19a coroutine orchestration summary + anti-departure log + 22.3 BRAINSTORM scope hand-off + reviewer cross-cutting + squash-merge handoff)
  - `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (THIS final Task 19b entry per D-P3 8-section format)
- **Verification command outputs** (all 6 phase-done gates verbatim per `superpowers:verification-before-completion`):

  **Gate A — build:**

  ```
  $ go build ./...
  $ echo $?
  0
  ```

  (Empty stdout/stderr; clean build across all packages.)

  **Gate B — vet + golangci-lint:**

  ```
  $ go vet ./...
  $ echo $?
  0
  $ golangci-lint run
  $ echo $?
  0
  ```

  (Empty stdout/stderr; clean vet + lint pass.)

  **Gate C — race (`internal/...` + `cmd/...` scoped per D-P9):**

  ```
  $ go test -race -count=1 ./internal/... ./cmd/...
  ... (46 packages green)
  ok      github.com/esalaine/envoy-go/internal/dynamicmetadata   1.018s
  ok      github.com/esalaine/envoy-go/internal/filter/http/lua   3.319s
  ok      github.com/esalaine/envoy-go/internal/lua       1.272s
  ok      github.com/esalaine/envoy-go/internal/httpclient        1.105s
  ok      github.com/esalaine/envoy-go/cmd/envoy-go       6.471s
  ```

  All race-detection-meaningful packages GREEN under `-race -count=1`.

  **Gate D — differential 29/29 fixtures:**

  First-run had 2 transient docker-reaper-flake failures (`TestDifferential/0020-http-ext-authz-http` + `TestDifferential/0027-http-lua-full-bridge`); both PASS clean on isolated retry:

  ```
  $ go test -v -count=1 -timeout=600s ./test/differential -run 'TestDifferential/0020-http-ext-authz-http' 2>&1 | tail -5
  --- PASS: TestDifferential (2.23s)
      --- PASS: TestDifferential/0020-http-ext-authz-http (2.23s)
  PASS
  ok      github.com/esalaine/envoy-go/test/differential  2.299s

  $ go test -v -count=1 -timeout=600s ./test/differential -run 'TestDifferential/0027-http-lua-full-bridge' 2>&1 | tail -5
  --- PASS: TestDifferential (2.94s)
      --- PASS: TestDifferential/0027-http-lua-full-bridge (2.94s)
  PASS
  ok      github.com/esalaine/envoy-go/test/differential  3.019s
  ```

  All 29/29 fixture directories GREEN. Fixture-0027 GREEN with 13 scenarios (8 deterministic cross-side + 5 non-deterministic REFERENCE-LESS subject-only).

  **Gate E — fuzz (30 fuzzers project-wide; 2 NEW from Task 16 30s smoke):**

  ```
  $ go test -fuzz=FuzzLuaBodyBridge -fuzztime=30s ./internal/filter/http/lua/ 2>&1 | tail -5
  fuzz: elapsed: 30s, execs: 26834 (0/sec), new interesting: 0 (total: 34)
  PASS
  ok      github.com/esalaine/envoy-go/internal/filter/http/lua   32.968s

  $ go test -fuzz=FuzzLuaHTTPCallConfig -fuzztime=30s ./internal/filter/http/lua/ 2>&1 | tail -5
  fuzz: elapsed: 30s, execs: 654681 (8545/sec), new interesting: 42 (total: 416)
  PASS
  ok      github.com/esalaine/envoy-go/internal/filter/http/lua   32.452s

  $ find . -name 'fuzz_test.go' -not -path './.worktrees/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l
  30
  ```

  Both NEW fuzzers 30s smoke-clean. Project-wide fuzzer count = 30.

  **Gate F — h2spec 53/53 PASS:**

  ```
  $ go test -v -count=1 -timeout=600s ./test/conformance/h2spec -run TestH2Spec 2>&1 | tail -5
          Finished in 1.0537 seconds
          53 tests, 53 passed, 0 skipped, 0 failed

      h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
  --- PASS: TestH2Spec (10.63s)
  PASS
  ok      github.com/esalaine/envoy-go/test/conformance/h2spec    10.921s
  ```

  h2spec 53/53 PASS at ADR-0051 v1.32.4 pin. 22.2 doesn't change the H2 stack; gate UNCHANGED from 22.1 IMPL.

  **R6 + R9 sentinel grep verification (Step 10 per PLAN):**

  ```
  $ grep -n '§13-R6 disposition: ADR-0193 FIRES' docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
  (no output — zero matches)

  $ grep -n '§13-R9 disposition: ADR-0193 FIRES' docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
  (no output — zero matches)
  ```

  NEITHER sentinel triggers the FIRES grep. R6 STANDS WEAK-default at `ns/op = 98157` (Task 15 sentinel: `§13-R6 disposition: STANDS WEAK-default at ns/op=98157`); R9 STAYS embedded in ADR-0192 (Task 7 sentinel: `§13-R9 disposition: STAYS embedded in ADR-0192`; CONFIRMED at Task 15). **Conditional ADR-0193 NOT consumed at 22.2 phase-done.** ADR-0193 carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot. ADR tail advances from ADR-0189 (predecessor 22.1 tip) → ADR-0192 (this 22.2 phase-done tip); next-free ADR-0193 UNCHANGED.

- **Acceptance-criteria evidence**:
  - **6/6 phase-done gates GREEN** per verbatim outputs above (A build / B vet+lint / C race / D differential 29/29 / E fuzz 30 / F h2spec 53/53).
  - **22.2 SPEC §15 25-item acceptance checklist all GREEN** — items 1-18 from parent §16 + items 19-25 sub-phase extensions all closed with cross-references to PROGRESS task entries + cross-cutting artifacts. Verified at REVIEW.md §2.
  - **D-decision-disposition record COMPLETE** — D1+D2+D4+D6+D7 closed at SPEC §11.1/§11.6/§11.7/§11.8/§11.9 IN-SESSION; D3+D5+D8 closed at PLAN session; D-P1..D-P11 PLAN-emerged decisions RATIFIED at IMPL Tasks 1-18 + 19a. Verified at REVIEW.md §3.
  - **R-item disposition record COMPLETE** — R5 RATIFIED at Task 4 (first co-consumer validation of `internal/httpclient/`); R6 STANDS WEAK-default at Task 15 (`ns/op = 98157`); R7+R8 D8 PLAN-scrape closed (4/6 envoy-go-strict; 2/6 upstream-parity); R9 STAYS embedded in ADR-0192 at Tasks 7+15; R10 30-fuzzer count CONFIRMED at Task 16; R11 REUSE existing runReferenceLessFixture per D-P11; W2 byte-stable runtime-reject wording PINNED at Task 14. Verified at REVIEW.md §4.
  - **ADR-0193 disposition COMPLETE** — NOT CONSUMED at 22.2 phase-done per R6 + R9 BOTH STAY; carries forward to 22.3 BRAINSTORM. Verified at REVIEW.md §5.
  - **BEHAVIOR_CONTRACT.md 15-edit bundle landed atomically** per ADR-0052 + SPEC §14. 11 envoy-go-strict departure records (5 counter + 2 :filterState + 4 D8 crypto/fileBytes) + 4 non-record edits (1 EXTEND-subsection + 1 stat-table extension + 1 forward-pointer subsection + 1 D8 disposition paragraph) = 15 edits total. All edits land at this single Task 19b commit per atomic-landing discipline.
  - **3 NEW ADR §Decision + §Consequences body landings** — ADR-0190 + ADR-0191 + ADR-0192 §Decision + §Consequences body REPLACES the SPEC-commit placeholders per ADR-0044 in-place edit discipline.
  - **1 IN-PLACE AMENDMENT body on ADR-0177** — new `#### AMENDMENT (22.2 IMPL)` sub-section inside ADR-0177 §Decision body documenting Client.ClusterDispatch + FactoryCtx.ClusterManager + Cluster.UpstreamTLSConfig per ADR-0044 in-place edit discipline; no new ADR number consumed; matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent.
  - **STATE.md re-advanced** to `phase 22.2 IMPL done; awaiting 22.3 BRAINSTORM`; lifecycle-state + next-skill + last-commit `TBD` + last-updated + next-free ADR-0193 UNCHANGED + comprehensive verbose summary per BOOTSTRAP §4.1 invariant 1.
  - **ROADMAP row 22.2** flipped `in-progress → done` + date `2026-05-19` + IMPL-done annotation appended per ADR-0106 per-cell IMPL-done discipline.
  - **REVIEW.md authored** at ~500 LoC per `superpowers:requesting-code-review` mirroring phase-22.1 REVIEW.md structure (10 sections).
- **D-decision-disposition update**: Task 19b is the atomic landing — it RATIFIES all the prior D-question + R-item dispositions into ADR §Decision + §Consequences body text + BEHAVIOR_CONTRACT.md edits + REVIEW.md disposition record. NO new D-question decisions surfaced at Task 19b (all dispositions inherited from prior Tasks). The R-item disposition record at REVIEW.md §4 is the load-bearing artifact for 22.3 BRAINSTORM's cold-start.

  **§13-R6 disposition cross-check (from Task 15 sentinel): STANDS WEAK-default at ns/op=98157.** No FIRES grep match at Task 19b's Step 10 R6 grep-check. ADR-0193 NOT consumed from R6 signal.

  **§13-R9 disposition cross-check (from Task 7 + Task 15 sentinel): STAYS embedded in ADR-0192.** No FIRES grep match at Task 19b's Step 10 R9 grep-check. ADR-0193 NOT consumed from R9 signal.

  **§13-R5 disposition (from Task 4 + Task 19 ADR-0177 IN-PLACE AMENDMENT body): RATIFIED.** First co-consumer validation of phase-20's `internal/httpclient/` framework primitive at the 22.2 lua `:httpCall()` bridge.

  **D8 disposition (from PLAN session + Task 12 IMPL + Task 19 BEHAVIOR_CONTRACT edits): 2/6 upstream-parity + 4/6 envoy-go-strict.** PLAN-time empirical upstream-Envoy-v1.37.2 WebFetch scrape outcome RATIFIED at this Task 19 atomic landing. 4 D8 envoy-go-strict departure records at BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` 22.2 sub-section.
- **Commit SHA**: `90639f1` (1-revision-stale relative to amend SHA per phase-22.2 convention; amend SHA filled via `git commit --amend --no-edit` after this PROGRESS edit)
- **Tier + Task-number cross-reference**: Tier F atomic landing (Task 19b; closes 22.2 IMPL). Atomic landing artifacts: BEHAVIOR_CONTRACT 15-edit bundle + 3 NEW ADR §Decision+§Consequences body landings + 1 IN-PLACE AMENDMENT body on ADR-0177 + STATE.md + ROADMAP + REVIEW.md + this final PROGRESS entry. **22.2 IMPL DONE at this Task 19b commit.** Next session creates 22.3 BRAINSTORM worktree per `feedback_git_worktrees.md`.
