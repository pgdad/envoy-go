# Phase 26.1 IMPL — PROGRESS

> Authoritative input: `docs/envoy-go/phases/26.1-network-filter-chain-framework-and-echo/PLAN.md` (1451-line PLAN; 17-task TDD plan). SPEC: `…/SPEC.md`. Parent SPEC: `docs/envoy-go/phases/26-network-filter-chain-and-rbac/SPEC.md`. Executed via `superpowers:subagent-driven-development` (fresh subagent per task, two-stage review; subagents commit local-only per `feedback_subagents_no_push`; controller pushes at stage-close). Worktree: `.worktrees/phase-26.1-network-filter-chain-impl` on branch `phase/26.1-network-filter-chain-impl` (off master `2c8ad75`).

---

## Task 1: First-action baselines + proto-roster re-confirm (HARD GATE) — DONE

**Worktree tip at start:** `2c8ad75` (master). The SPEC pinned baselines against `9429983`; the current tip differs only by docs/next-prompt commits (no Go code changed), so no code-surface drift is possible.

### Step 1 — four baselines re-pinned via git-tracked enumeration (deterministic; avoids the nested-`.worktrees/` artifact)

| Baseline | Command | Expected | Got | Status |
|---|---|---|---|---|
| fuzzers | `git ls-files '*fuzz_test.go' \| xargs grep -h "^func Fuzz" \| wc -l` | 34 | **34** | ✓ |
| fixture dirs | `ls test/fixtures/ \| grep -E '^[0-9]' \| wc -l` | 41 | **41** | ✓ |
| fixture tail | `ls test/fixtures/ \| grep -E '^[0-9]' \| sort \| tail -1` | 0039-… | **0039-http-wasm-perroute-boot-reject** | ✓ |
| ADR tail | `grep -nE '^#+ +ADR-0[0-9]{3}' docs/envoy-go/DECISIONS.md \| tail -1` | ADR-0214 | **ADR-0214** | ✓ |

The "41 fixture dirs" reconciles with "tail 0039" via the two letter-variant dirs `0007a-cors` + `0007b-iteration-probe` (0000–0039 numeric = 40, + 2 letter-variants − 1 collision in a naive `[0-9]{4}` sort = 41 distinct dirs). No drift.

### manager.go line anchors re-pinned (PLAN Tasks 10/11 cite these; verified against current tip)

| Anchor | PLAN pin | Actual | Status |
|---|---|---|---|
| `type chainInfo struct` | @182 | @182 | ✓ |
| `NewManager` | @261 | @261 | ✓ |
| `NewManagerWithBaseDir` | @273 | @273 | ✓ |
| `NewManagerWithBaseDirAndAllowH2C` | @302 | @302 | ✓ |
| `buildTerminalFilter` call site (per-chain) | @444 | @444 | ✓ |
| `buildTerminalFilter` call site (default_filter_chain) | @503 | @503 | ✓ |
| `buildTerminalFilter` def | @569 | @569 | ✓ |
| `serveConnection` | ~@945 (step-7 ~@1004) | @945 | ✓ |

**No manager.go drift** — all PLAN-cited anchors match exactly.

Other `NewManagerWithBaseDirAndAllowH2C` callers (build-fix blast radius for Tasks 10/12), via `git grep -l`:
`internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go`, `internal/listener/manager_test.go`, `cmd/envoy-go/main_test.go` (+ `cmd/envoy-go/main.go` itself).

### Step 2 — stat surface = 132

26.1 adds 0 stats (echo + direct_response have zero built-in counters; framework adds none — SPEC §2.8). The project stat surface is additive-per-filter and pinned at 132 in BEHAVIOR_CONTRACT.md (25.3 FAMILY-FINAL: 128 → 132). No Go code changed since the SPEC's `9429983` pin, so 132 holds; re-confirmed at Task 17's six-gate.

### Step 3 — proto rosters re-confirmed vs go-control-plane v1.32.4

- `direct_response.v3.Config` (package alias `direct_responsev3`; PLAN aliases it `drv3`) — single field `Response *corev3.DataSource`; message named **`Config`** (not `DirectResponse`). ✓
- `echo.v3.Echo` (`echov3`) — empty message. ✓
- `core.v3.DataSource` (`corev3`) — 4-arm `specifier` oneof: `DataSource_Filename` / `DataSource_InlineBytes` / `DataSource_InlineString` / `DataSource_EnvironmentVariable`. ✓

### Step 4 — six gates green at the tip (baseline clean before new code)

- `go build ./...` — **PASS**
- `go vet ./...` — **PASS**
- `golangci-lint run` — **PASS** (exit 0)
- `go test -race -short ./...` — **PASS** modulo a pre-existing flaky concurrency test in `internal/filter/http/wasm` (`dispatch_test.go` — counts goroutine completions under `-race` load; fails intermittently under full-suite parallelism, passes 5/5 in isolation and passed on the 2nd full-suite run). Pre-existing at clean master tip; unrelated to `internal/filter/network/`. Tracked, not introduced by 26.1.

**Gate verdict: GREEN.** All baselines hold; no fixture-number / fuzzer-count / stat-surface / ADR-tail / manager.go-line drift. Proceeding to Tasks 2–17 with the SPEC-asserted deltas (fixtures 41→44, fuzzers 34→35, stats 132→132, ADR tail stays 0214).

---

## Tasks 2–6: NEW `internal/filter/network/` framework package — DONE

Dispatched as ONE implementer unit (the five PLAN tasks build one mutually-referential package — `ReadFilter`→`ReadFilterCallbacks`→`Connection`→`CloseType`/`dynamicmetadata`, plus `Buffer` — so the package only compiles as a unit), committed as five commits in task order. Two-stage review (spec + code-quality) run on the whole package; both passed; 3 minor review nits fixed.

**Files created:** `internal/filter/network/{doc,types,buffer,callbacks,registry,chain}.go` + their `_test.go`.

**Commits (local-only):**
- `dd2ff95` Task 2 — skeleton: `Status` enum, `ReadFilter`, factory types, `FactoryCtx{BaseDir string}` (D-P26.1-2 override of SPEC's empty struct).
- `ccf3889` Task 3 — drainable `*Buffer` (Append/Bytes/Len/Drain, over-drain clamped).
- `f9182ef` Task 4 — `ReadFilterCallbacks` + `Connection` + `CloseType` (FlushWrite/NoFlush); all six L4 `Connection` accessors live + unit-tested (R2 readiness for 26.3).
- `b479c49` Task 5 — freeze-after-boot `*Registry` (mirrors `listenerfilter/registry.go` core + `http/registry.go` `KnownTypeURLs` insertion-sort; panic wording `network: registry frozen…`/`network: duplicate factory…`; no package-global `init()`).
- `721435f` Task 6 — chain runner + per-connection runtime context: reused `*dynamicmetadata.Bucket` (NewBucket/Reset), `closeRequested` flag (D-P26.1-5a), `SetResponseCodeDetails` RCD sink on concrete `*callbacks` (D-P26.1-5b, proven live by `TestResponseCodeDetailsSink`), single-goroutine-per-connection (ADR-0213). Chain iteration §3.3 (eager OnNewConnection, lazy-on-resume, StopIteration halt with connection-level buffering, ContinueReading→next filter).
- `0244136` Task 6 review-fix — tightened `TestChainContinueReadingResumesAtNextFilter` to be discriminating (mutation-proven: fails on plain-advance, passes on real halting body); clarified RCD-sink + Write-error-drop doc comments.

**Exported surface for Task 11 (manager.go read loop):** runtime kept unexported (`chainRuntime`/`newChainRuntime`/`onNewConnection`/`onData`/`onDestroy`/`closeRequested`); Task 11 promotes a thin exported wrapper (`ChainRuntime` + `OnNewConnection`/`OnData`/`OnDestroy`/`CloseRequested`) without reaching into internals.

**Verification:** `go build/vet ./internal/filter/network/...` clean; `go test -race ./internal/filter/network/` PASS (15 tests, race-clean); `golangci-lint run` exit 0; full-module `go build ./...` green.

**Reviews:** Spec-compliance ✅ (FactoryCtx.BaseDir + RCD sink + closeRequested all confirmed live; `filterA` test deviation justified — PLAN's literal body was impossible in a single `onData` call). Code-quality ✅ Approve (registry mirrors precedent faithfully; state machine correct; concurrency sound). 3 minor nits fixed in `0244136`.

---

## Task 7: `echo` filter — DONE

`internal/filter/network/echo/{doc,echo,echo_test}.go`. `OnData` writes `buf.Bytes()` back, drains, returns `StopIteration`; `OnNewConnection`→Continue; empty/absent typed_config accepted (AMEND-A2). Commits: `b5f4fa5` (Task 7) + `3eca0d6` (review-fix). Combined spec+quality review (echo is ~30 LoC): NEEDS-CHANGES → fixed — removed a dead `filterName` const (masked by `//nolint:unused`; echo registers by TypeURL, has no name/stats surface), added the `New` error-path test, asserted `endStream` propagation + `OnNewConnection`→Continue, renamed an `any`-shadowing var.

## Task 8–9: `direct_response` filter — DONE

`internal/filter/network/directresponse/{doc,directresponse,directresponse_test}.go`. `TypeURL` ends `.Config` (not `DirectResponse`). `New` resolves the DataSource 4-arm (InlineString/InlineBytes/Filename-baseDir-relative/EnvironmentVariable) at **boot** (so bad file / unset specifier rejects at config-load — the basis for fixture 0042), sets RCD `"DirectResponse"` via the live `SetResponseCodeDetails` optional-interface assertion, `OnNewConnection` writes body+endStream → FlushWrite-close → StopIteration. Byte-stable reject const `direct_response: response.specifier is required` pinned via `TestParseRejectConstants_ByteStable`. Commits: `6f11c55` (Task 8) + `ba72af1` (Task 9) + `291bd6e` (review-fix: commented the forward-guard `default` arm, `errors.New` for the verb-less error, asserted `conn.closed`). Review ✅ Approve (boot-time resolution confirmed; RCD sink + Filename baseDir arms exercised live).

## Task 10–11: `manager.go` dual-dispatch — DONE (one CRITICAL fix)

`internal/listener/manager.go` + exported chain surface in `internal/filter/network/chain.go`. `chainInfo.netChainFactory func() []network.ReadFilter` (fresh instances per connection); `netReg *network.Registry` as 11th ctor param (nil in thinner variants + all admin/manager_test callers + `cmd/envoy-go/main.go` temporarily `// netReg wired in Task 12`); build-time pre-check at both `buildTerminalFilter` call sites (new-path iff `netReg!=nil` && `filters[0]∈netReg`, every filter resolved at boot, else old path UNCHANGED); `network-filter-mixed-chain-unsupported` boot-reject; `serveConnection` step-7 dual-branch; `serveReadFilterChain` read loop (CloseRequested-driven exit, D-P26.1-5a). Exported `ChainRuntime`/`NewChainRuntime`/`OnNewConnection`/`OnData`/`OnDestroy`/`CloseRequested`/`ConnFacts` (thin facade). Commits: `19cc6c7` (Task 10) + `f5120b5` (Task 11) + `f8668e8` (CRITICAL review-fix).

**R4 back-compat:** old terminal path (tcp_proxy/HCM) byte-identical (when `netReg==nil` the pre-check is fully skipped); differential fixtures 0012/0013/0027/0028 PASS in isolation on the branch (the earlier `go test ./...` failures were Docker container-readiness flakiness under parallel load, not regressions — netReg=nil makes dual-dispatch inert in the binary until Task 12).

**CRITICAL bug found + fixed (`f8668e8`):** the Task-6 chain runtime's single `halted` flag was sticky across socket reads — echo (drain+StopIteration, no ContinueReading) set it on the first read and `onData` then early-returned forever, so echo echoed only the FIRST read (would fail fixture 0040's multi-write probe). Fix: split into `connHalted` (the ONLY cross-read-persistent halt — set solely on `OnNewConnection` StopIteration, cleared by ContinueReading) vs a non-sticky per-pass OnData stop (leaves `resumeIdx` at the stopping filter; the next socket read re-runs `runData` from there, re-delivering the accumulated buffer — upstream `onRead` re-iteration semantics). Added `TestChainEchoStyleMultipleReads` + made `TestServeReadFilterChainEcho` write 3 separate payloads (both red pre-fix, green post-fix). Two-stage + re-review with mutation testing: CRITICAL resolved, ContinueReading/connHalted semantics proven sound, no busy-loop, R4 back-compat green, race-clean.

---

## Task 12: boot-wiring in `cmd/envoy-go/main.go` — DONE (+ SPEC empirical correction)

`netReg := network.NewRegistry(); netReg.Register(echo.TypeURL, echo.New); netReg.Register(directresponse.TypeURL, directresponse.New); netReg.Freeze()` after the `lfReg` block; the temporary `nil` at the ctor call replaced with `netReg`. Commits: `5507ff6` (Task 12) + `fea5d2d` (boot-fix).

**SPEC EMPIRICAL CORRECTION (boot smoke surfaced it; ⚠️ Task-17 docs must reflect):** the SPEC/PLAN pinned echo's TypeURL as `type.googleapis.com/envoy.filters.network.echo.v3.Echo` — but the actual go-control-plane v1.32.4 proto full-name (verified via `proto.MessageName`) is **`envoy.extensions.filters.network.echo.v3.Echo`** (WITH `extensions.`). Without `extensions.` the binary cannot boot an echo config (`protojson: ... "not found"`). Corrected `echo.TypeURL` to the `extensions.` form. CONFIRMED correct against real upstream Envoy v1.37.2 by fixture 0040 (the reference accepted it + echoed byte-exact). direct_response's URL was already correct. Also registered the echo + direct_response extension protos in `internal/bootstrap/bootstrap.go`'s blank-import block (per the cors precedent — guarantees `@type` resolution in any bootstrap-parsing context). **→ Task 17 must correct SPEC §3.6/§4.1 + BEHAVIOR_CONTRACT echo type-URL to the `extensions.` form.**

**Boot smoke (live binary, the real Task-12 acceptance):** echo multi-read (2 separate writes echoed byte-exact) + direct_response (`smoke-dr\n` body + server-close) both PASS. (Bootstraps needed a dummy static cluster — envoy-go's cluster manager rejects zero-cluster boot.)

## Task 13: 35th fuzzer `FuzzNetworkFilterConfigParse` — DONE

`internal/filter/network/directresponse/fuzz_test.go` — fuzzes direct_response's `New` (echo's parse is vacuous), invariant: never `(nil,nil)`, never `(factory,err)`. Seed corpus + 20s fuzz (3.7M execs, no crashers). Fuzzer count **34 → 35** (git-tracked). Commit: `1be52bb`.

## Tasks 14–16: differential fixtures (Docker, cross-side byte-exact) — DONE

All three pass byte-exact against reference Envoy v1.37.2. Fixture count **41 → 44**. Per `reference_differential_fixture_dispatch_constraint` (one dir = one runner branch): 0040/0041 cross-side, 0042 boot-reject = separate dirs. Bootstraps declare a dummy/`c_unused` cluster (envoy-go's cluster manager boots before the listener manager). All three reviewed ✅ Approve.

- **0040-network-echo** (`e64489a` + `f5b5e6c` hardening): echo filter reflects a 70-byte multi-line payload byte-exact. Debugged a reference-Envoy **FIN-co-arrival teardown race** (reference returns 0 bytes when the client's FIN arrives with the payload; full echo otherwise) — added `helpers.TCPRoundTripNoHalfClose` to drive WITHOUT immediate half-close (drain on 1s idle), steering around the nondeterministic race (the subject echoes fully regardless; the deterministic behavior matches). Existing `TCPRoundTrip` untouched (0000 unaffected). Confirms the corrected echo `@type` works on real upstream.
- **0041-network-direct-response** (`199b677` + `f5b5e6c`): static `inline_string` body `envoy-go-direct-response\n` byte-exact, server closes (FlushWrite). Both fixtures hardened with a non-empty-bytes guard (so a future both-sides-empty regression fails loudly rather than passing as `empty==empty`).
- **0042-network-direct-response-boot-reject** (`620da72`): `response: {}` (specifier oneof UNSET) → BOTH binaries reject at config-load. Empirically pinned (Dockerized v1.37.2): reference `ConfigValidationError.Response: ... field: "specifier", reason: is required` vs envoy-go `direct_response: response.specifier is required` → shared case-sensitive substring **`specifier`** (injection-proof — only in stderr, not echoed from config). The `response`-ABSENT arm does NOT reject upstream (boots) → `response:{}` is the only symmetric arm. `c_unused` cluster declared so the reject is attributable to direct_response, not zero-clusters. Used `driver/` (not the boot-reject `inputs/` convention) for phase-26.1 sibling-consistency with 0040/0041 + the PLAN's `package driver` stub — accepted deviation.

---

## Task 17: docs bundle (BEHAVIOR_CONTRACT + ADR-0213/0214 bodies + echo TypeURL correction) + STATE/ROADMAP advance + six-gate verification — DONE

### Doc edits

- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — NEW `## Network filters` top-level section inserted after `## Listener filters` (mirroring the §9 family-subsection format): the `### Network filter chain framework` subsection (two-value `Status` protocol; `ReadFilter`/`ReadFilterCallbacks`/`Connection`/`CloseType`; connection-level buffering with the sticky-`OnNewConnection` vs non-sticky-`OnData` re-iteration distinction; `ContinueReading`; single-goroutine-per-connection; per-connection runtime context + reused `*dynamicmetadata.Bucket`; drainable `Buffer`; freeze-after-boot `Registry`; `manager.go` dual-dispatch; the 3 envoy-go-strict departure records — write-filter absent / `network-filter-mixed-chain-unsupported` transitional reject lifted at 26.2 / read-filter-only scope); `### envoy.filters.network.echo` (empty config; OnData write-back+drain+StopIteration; corrected `@type`; 0 stats; fixture 0040); `### envoy.extensions.filters.network.direct_response` (`Config` not `DirectResponse`; single `response` DataSource 4-arm; OnNewConnection write+FlushWrite-close+StopIteration; no delay; the set-but-unread `DirectResponse` RCD string with no operator surface; `specifier`-required boot-reject parity via fixture 0042; 0 stats); `### Type-URL correction (echo @type)`; `### Stat surface` (stays 132); `### Forward-pointer note (26.2 / 26.3)`; Applies-to / Does-not-yet-apply-to.
- **`docs/envoy-go/DECISIONS.md`** — ADR-0213 §Decision + §Consequences bodies filled in place (two-value `Status` no-HTTP-buffer-variant; `ReadFilter`/`ReadFilterCallbacks`; connection-level buffering + the sticky/non-sticky re-iteration split; single-goroutine-per-connection; per-connection runtime context owning the reused Bucket; drainable Buffer; read-filter-only + write-filter API-revision allowance + set-but-unread RCD; FactoryCtx.BaseDir consequence). ADR-0214 §Decision + §Consequences bodies filled in place (registry mirroring ADR-0072/0079; `netReg` ctor arg; build-time dual-dispatch + mixed-chain reject; planned 26.2 retirement; FactoryCtx.BaseDir + echo-TypeURL-`extensions.` correction consequences). Both ADRs' Status/Date lines updated to ACCEPTED / bodies-landed-at-26.1-IMPL-Task-17. **DECISIONS.md tail STAYS ADR-0214** (no new number consumed; next-free ADR-0215).
- **SPEC §3.6 + §4.1** — echo TypeURL corrected from `…envoy.filters.network.echo.v3.Echo` to `…envoy.extensions.filters.network.echo.v3.Echo` with the inline correction note (go-control-plane v1.32.4 full-name; verified vs upstream v1.37.2 by fixture 0040).
- **STATE.md** — active-phase → `phase 26.1 phase-done; awaiting 26.2 SPEC`; lifecycle-state → SKILL_ROUTING state 4 → done for sub-row 26.1; next-skill → `superpowers:writing-plans` (scoped to the 26.2 per-sub-phase SPEC, per the 22.1/25.1 precedent); next-free ADR `ADR-0215` UNCHANGED; last-commit `TBD-26.1-IMPL-SQUASH` (controller fills at stage-close); echo-TypeURL correction + the sticky-halt CRITICAL bug recorded in the active-phase narrative.
- **ROADMAP.md** — sub-row 26.1 `in-progress → done`; parent row 26 STAYS `in-progress` (flips at 26.3 per the 18/19/22/24/25 ROLLUP precedent).

### Task 17 six-gate verification (verbatim)

```
$ go build ./... && echo BUILD_OK
BUILD_OK
$ go vet ./... && echo VET_OK
VET_OK
$ golangci-lint run && echo LINT_OK
LINT_OK
$ go test -race -short ./... 2>&1 | grep -vE '^ok |no test files' | tail
(no output — every package PASS; no FAIL lines after filtering ok/no-test-files. The pre-existing internal/filter/http/wasm dispatch_test.go flake did NOT fire this run.)
```

Differential suite (Docker; NOT -short):

```
$ go test ./test/differential/ -run TestDifferential -v 2>&1 | grep -E '^(--- PASS|--- FAIL|PASS|FAIL|ok)' | tail
--- FAIL: TestDifferential (145.15s)
    --- FAIL: TestDifferential/0034-http-wasm-headers-bridge (2.36s)
```

The SOLE failure was `0034-http-wasm-headers-bridge` — an UNRELATED wasm fixture (not 26.1), a Docker container-readiness flake under parallel load (the same class noted at Tasks 10/11 for 0012/0013/0027/0028). Re-run in isolation + within a targeted re-run, 0034 + all 26.1 fixtures + the R4 back-compat fixtures PASS:

```
$ go test ./test/differential/ -run TestDifferential -v 2>&1 | grep -E '0040|0041|0042|0000-tcp|0002-tls|0034' | grep -E 'PASS|FAIL'
    --- PASS: TestDifferential/0000-tcp-echo (2.12s)
    --- PASS: TestDifferential/0002-tls-tcp (1.91s)
    --- PASS: TestDifferential/0034-http-wasm-headers-bridge (2.24s)
    --- PASS: TestDifferential/0040-network-echo (3.71s)
    --- PASS: TestDifferential/0041-network-direct-response (1.65s)
    --- PASS: TestDifferential/0042-network-direct-response-boot-reject (1.55s)

$ go test ./test/differential/ -run 'TestDifferential/(0040|0041|0042|0000-tcp-echo|0002-tls-tcp|0034)' -v 2>&1 | grep -E '^(--- PASS|--- FAIL|PASS|FAIL|ok)' | tail
--- PASS: TestDifferential (13.17s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	13.260s
```

Count + stat gates:

```
$ git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l
35
$ ls test/fixtures/ | grep -E '^[0-9]' | wc -l
44
$ grep -rn "Counter\|RegisterStat\|stats\." internal/filter/network/
(no output — internal/filter/network/ registers NO counter / stat; 26.1 adds 0 stats)
```

- **Stat surface = 132** (26.1 adds 0; the framework + echo + direct_response register no built-in counter — confirmed by the empty grep above; BEHAVIOR_CONTRACT 25.3 FAMILY-FINAL pinned 132).
- **Conformance 10/10 + h2spec 53/53: ASSERTED-UNAFFECTED** (SPEC §2.9). 26.1 touches NO HTTP / proxy-wasm / HTTP-2 code path — it adds the NEW `internal/filter/network/` package + echo/directresponse + a dual-dispatch that is inert (`netReg==nil`) on every non-network-filter chain, leaving the HCM/h2/wasm paths byte-identical. The heavy conformance + h2spec suites were NOT re-run (no code path they exercise changed).

**Gate verdict: GREEN.** Build/vet/lint/`-race -short` clean; differential 0040/0041/0042 + R4 back-compat byte-exact (the lone 0034 failure was a confirmed Docker flake); fuzzers 35; fixtures 44; stats 132; conformance/h2spec asserted-unaffected. ADR-0213/0214 bodies + BEHAVIOR_CONTRACT bundle + SPEC TypeURL correction + STATE/ROADMAP advance all landed.

---

## Controller final verification (stage-close) — ALL GREEN

Independent six-gate re-run at branch tip after Task 17 + final whole-impl review:
- `go build ./...` / `go vet ./...` / `golangci-lint run` — clean (exit 0).
- `go test -race -short ./...` — ALL GREEN (the pre-existing wasm dispatch flake did not fire).
- **Full differential suite (Docker, R4 gate):** `go test ./test/differential/ -run TestDifferential -v` → `--- PASS: TestDifferential (144.99s)`, **exit 0, zero FAIL subtests** — 0040/0041/0042 byte-exact + ALL existing fixtures (R4 back-compat: 0000-tcp-echo / 0002-tls-tcp / HCM / wasm / lua) green in a single clean run.
- fuzzers **35** (canonical `git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l`); fixtures **44**; stat surface **132** (zero stat registrations under `internal/filter/network/`); DECISIONS.md tail **ADR-0214**.
- Conformance 10/10 + h2spec 53/53 ASSERTED-UNAFFECTED (SPEC §2.9 — 26.1 touches no HTTP/h2/proxy-wasm path; dual-dispatch is inert on non-network-filter chains).

**Final whole-implementation review:** 8/8 SPEC §15.3 acceptance criteria hold; Task-17 docs (ADR-0213/0214 bodies + BEHAVIOR_CONTRACT bundle) substantive + accurate; echo-TypeURL `extensions.` correction consistent across all live surfaces. One Minor (SPEC §2 summary straggler echo URL) fixed at stage-close.

**Phase 26.1 IMPL COMPLETE.** Ready for squash-merge to master + push.
