# Phase 28.1 — network-filter write-seam + zookeeper_proxy requests — PROGRESS

This file accumulates task-completion records for each of the 18 IMPL tasks.
Commit tip at Task 1: `6dbc4c1` (branch `phase-28.1-network-filter-write-seam-and-zookeeper-requests-impl`).
Date: 2026-06-01.

---

## Task 1: First-action baselines/anchors gate (no code change)

**Status: PASS — all baselines confirmed at expected values, all 33 as-built anchors re-pinned with zero drift, proto.MessageName + all 27 enum identifiers recorded.**

---

### Step 1: Baseline counts at IMPL-session tip

Git tip: `6dbc4c1 next-prompt.txt: repoint master-tip reference to 4ee1ab5 (actual HEAD; trails 28.1-PLAN squash 29f9d38 +1)` (expected docs-only repoint — the substantive PLAN squash is at `29f9d38`).

| Baseline          | Expected                            | Actual                              | Result |
|-------------------|-------------------------------------|-------------------------------------|--------|
| Fixture dirs      | 47                                  | 47                                  | PASS   |
| Fixture tail      | `test/fixtures/0045-sni-cluster`    | `test/fixtures/0045-sni-cluster`    | PASS   |
| Fuzzers (./internal) | 36                               | 36                                  | PASS   |
| DECISIONS.md tail | ADR-0223 (next-free ADR-0224)       | ADR-0223 (grep → `ADR-0221 ADR-0222 ADR-0223`) | PASS |

Commands run (from repo root):
```
$ ls -d test/fixtures/[0-9]* | wc -l
47
$ ls -d test/fixtures/[0-9]* | tail -1
test/fixtures/0045-sni-cluster
$ grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l
36
$ grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -3
ADR-0221
ADR-0222
ADR-0223
```

The ADR-0221/0222/0223 §Context drafts landed at the parent SPEC (`DECISIONS.md:14226/:14245/:14264`). Phase 28.1 lands fixtures `0046`+`0047` → 49, the 37th fuzzer (`FuzzZookeeperRequestDecode`), and the ADR-0221/0222 BODIES in place (no new ADR number). DECISIONS tail STAYS at ADR-0223 (next-free ADR-0224).

No drift.

---

### Step 2: Stat surface = 136 (+0 this phase, this task)

Canonical recipe = the BEHAVIOR_CONTRACT.md cumulative "internal names" narrative accounting (the same count STATE.md reports as 136; the stat-table row count). The last delta landed at phase 26.3:

`docs/envoy-go/BEHAVIOR_CONTRACT.md:462` — "**Phase 26.3 extension — 132 → 136 internal names:** phase 26.3 adds the 4 `rbac_network` base counters … Phase 26.3 total: **132 → 136 internal names**."

Expected: **136**. Actual: **136** (BEHAVIOR_CONTRACT narrative tail; confirmed consistent in STATE.md). No new stat names land at Task 1.

Phase 28.1 lands the zookeeper_proxy +201 eager roster → **337** at Task 18 (per PLAN Task 8).

Result: **PASS**.

---

### Step 3: `proto.MessageName` (the TypeURL pin) + the 27 LatencyThresholdOverride_Opcode identifiers

#### proto.MessageName

Verified via a temp main package created in-worktree under `tmp_zk_tu/main.go`, run, then deleted (nothing temp committed):

```
$ go run ./tmp_zk_tu/
envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy
```

`proto.MessageName(&zkv3.ZooKeeperProxy{})` = `envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy` — carries the `extensions.` segment (memory note `reference_network_filter_typeurl_extensions` confirmed).

**TypeURL** = `type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy`

Package import path: `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3`. The build-resolved proto module is `github.com/envoyproxy/go-control-plane/envoy v1.32.4` (the `envoy` submodule; the umbrella `go-control-plane` is v0.13.4). Source file: `…/go-control-plane/envoy@v1.32.4/extensions/filters/network/zookeeper_proxy/v3/zookeeper_proxy.pb.go`.

#### The 27 `LatencyThresholdOverride_Opcode` Go identifiers (for Task 6's mapping table)

All 27 values, contiguous 0..26, digit-suffixed names intact (`Create2`=14, `GetChildren2`=11, `SetWatches2`=25). Source: `zookeeper_proxy.pb.go:30-56` (v1.32.4):

| Value | Go identifier |
|-------|---------------|
| 0  | `LatencyThresholdOverride_Connect` |
| 1  | `LatencyThresholdOverride_Create` |
| 2  | `LatencyThresholdOverride_Delete` |
| 3  | `LatencyThresholdOverride_Exists` |
| 4  | `LatencyThresholdOverride_GetData` |
| 5  | `LatencyThresholdOverride_SetData` |
| 6  | `LatencyThresholdOverride_GetAcl` |
| 7  | `LatencyThresholdOverride_SetAcl` |
| 8  | `LatencyThresholdOverride_GetChildren` |
| 9  | `LatencyThresholdOverride_Sync` |
| 10 | `LatencyThresholdOverride_Ping` |
| 11 | `LatencyThresholdOverride_GetChildren2` |
| 12 | `LatencyThresholdOverride_Check` |
| 13 | `LatencyThresholdOverride_Multi` |
| 14 | `LatencyThresholdOverride_Create2` |
| 15 | `LatencyThresholdOverride_Reconfig` |
| 16 | `LatencyThresholdOverride_CheckWatches` |
| 17 | `LatencyThresholdOverride_RemoveWatches` |
| 18 | `LatencyThresholdOverride_CreateContainer` |
| 19 | `LatencyThresholdOverride_CreateTtl` |
| 20 | `LatencyThresholdOverride_Close` |
| 21 | `LatencyThresholdOverride_SetAuth` |
| 22 | `LatencyThresholdOverride_SetWatches` |
| 23 | `LatencyThresholdOverride_GetEphemerals` |
| 24 | `LatencyThresholdOverride_GetAllChildrenNumber` |
| 25 | `LatencyThresholdOverride_SetWatches2` |
| 26 | `LatencyThresholdOverride_AddWatch` |

NOTE: the PLAN's illustrative grep `LatencyThresholdOverride_[A-Za-z0-9]+ LatencyThresholdOverride_Opcode = [0-9]+` matched only 1 line in v1.32.4 because the generated `const (...)` block tab-aligns the type+value with **variable** whitespace; reading the declaration block directly (`pb.go:30-56`) surfaced all 27. The digit-suffixed names are intact (CAUTION from MEMORY `reference_proto_roster_extraction_digits` observed — no truncation).

Result: **PASS**.

---

### Step 4: As-built line anchors (§3/§4 — drift here re-points later tasks)

Each anchor's cited construct verified at/near the cited line. **All 33 HOLD — zero drift.**

| # | File:line | Construct | Result |
|---|-----------|-----------|--------|
| 1  | `internal/filter/network/types.go:29-48` | `ReadFilter interface` | HOLDS |
| 2  | `internal/filter/network/types.go:61` | `FilterInstanceFactory func() NetworkFilter` | HOLDS |
| 3  | `internal/filter/network/terminal.go:18-28` | sealed `NetworkFilter` marker + `Marker` | HOLDS |
| 4  | `internal/filter/network/terminal.go:42-49` | `TerminalFilter.Handle` | HOLDS |
| 5  | `internal/filter/network/chain.go:57-83` | `NewChainRuntime` classification switch | HOLDS (`func NewChainRuntime` at :57) |
| 6  | `internal/filter/network/chain.go:127-168` | `chainRuntime` struct | HOLDS (`type chainRuntime struct` at :127) |
| 7  | `internal/filter/network/chain.go:174-189` | `newChainRuntime` + read-callbacks injection at `:185-187` | HOLDS (`f.SetReadFilterCallbacks(rt.cb)` loop at :185-187) |
| 8  | `internal/filter/network/chain.go:215-227` | `handleTerminal` | HOLDS (`func (rt *chainRuntime) handleTerminal` at :215) |
| 9  | `internal/filter/network/chain.go:321-326` | `onDestroy` | HOLDS (`func (rt *chainRuntime) onDestroy` at :321) |
| 10 | `internal/filter/network/chain.go:380-385` | `Connection.Write` (D-P3 bypass) | HOLDS (`func (c *connection) Write` at :380) |
| 11 | `internal/filter/network/prefixconn.go:12-28` | `prefixConn` + `newPrefixConn` + `Read` | HOLDS |
| 12 | `internal/filter/network/callbacks.go:16-38` | `ReadFilterCallbacks` interface | HOLDS |
| 13 | `internal/listener/manager.go:534-599` | `buildNetworkChainFactory` | HOLDS (`func buildNetworkChainFactory` at :534) |
| 14 | `internal/listener/manager.go:570-581` | boot classification switch | HOLDS (`for idx, nf := range sample` + type switch) |
| 15 | `internal/listener/manager.go:580` | write-only / unclassified default reject | HOLDS (`default:` arm returns "neither a read nor a terminal network filter") |
| 16 | `internal/stats/name.go:88-122` | wasm permissive arm | HOLDS (`case strings.HasPrefix(internal, "wasm."):` at :88) |
| 17 | `internal/stats/name.go:226-242` | rbac arm | HOLDS (`const rbacSegment = ".rbac."` at :226) |
| 18 | `internal/stats/name.go:243` | default error | HOLDS (`return "", nil, fmt.Errorf("stats: name %q has no recognized top-level segment …")`) |
| 19 | `internal/filter/network/builtins/builtins.go:44-63` | `RegisterBuiltins` | HOLDS (`func RegisterBuiltins` at :44) |
| 20 | `internal/filter/network/builtins/builtins.go:59` | rbac closure-capture register | HOLDS (`reg.Register(networkrbac.TypeURL, networkrbac.NewFactory(deps.StatsRegistry))`) |
| 21 | `internal/filter/network/builtins/builtins.go:62` | snicluster register | HOLDS (`reg.Register(snicluster.TypeURL, snicluster.New)`) |
| 22 | `internal/bootstrap/bootstrap.go:76-87` | network-filter blank-imports | HOLDS (direct_response/echo/sni_cluster blank-imports) |
| 23 | `internal/filter/network/rbac/rbac.go:38` | TypeURL via `proto.MessageName` | HOLDS (`var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&networkrbacv3.RBAC{}))`) |
| 24 | `internal/filter/network/rbac/rbac.go:187-198` | `newFilterStats` | HOLDS (`func newFilterStats` at :187) |
| 25 | `internal/stats/registry.go:157-171` | `NewCounterIfAbsent` | HOLDS (`func (r *Registry) NewCounterIfAbsent` at :157) |
| 26 | `internal/stats/counter.go:22/:27/:30` | `Inc`/`Add`/`Load` | HOLDS — accessor is **`Load()`** (NOT `Value()`); `Inc` at :22, `Add` at :27, `Load` at :30 |
| 27 | `test/differential/fixture/fixture.go:125/:129/:492/:495-499` | `BackendKind`(:125)/`TCPEcho=0`(:129)/`HTTPWasmPerRoute=27`(:492)/`BackendKindAware`(:495-499) | HOLDS |
| 28 | `test/differential/fixture/fixture.go:75-77` | `StatsAsserter` | HOLDS |
| 29 | `test/differential/fixture/fixture.go:584-589` | `MultiListenerDriver` | HOLDS |
| 30 | `test/differential/runner_test.go:98/:150/:1048-1050/:1219` | `TestDifferential`(:98)/backend-kind switch(:150)/`StatsAsserter` cross-side dispatch(:1048-1050)/`acceptEchoCounting`(:1219) | HOLDS |
| 31 | `test/differential/harness.go:340-352` | `BootRejectFixture` | HOLDS |
| 32 | `test/fixtures/0043-network-rbac/driver/driver.go:328-376/:388-461` | `AssertStats`(:328)/scrape-parse helpers (`scrapeRBACStats` at :388) | HOLDS |
| 33 | `test/fixtures/0044-network-rbac-boot-reject/driver/driver.go:159/:163` | `BootRejectScript`(:159)/`ExpectedBootErrorSubstring`(:163) | HOLDS |

(The PLAN's anchor list groups constructs with "+" into single items; this table splits or merges them where natural, yielding 33 rows total; all sub-constructs were verified at their cited offsets.) **Every anchor HOLDS at byte-exact / on-the-nose line offsets.**

---

### Summary

All baselines confirmed at expected values: fixtures **47** (tail `0045-sni-cluster`), fuzzers **36**, stat surface **136**, DECISIONS.md tail **ADR-0223** (next-free **ADR-0224**). `proto.MessageName` = `envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy` (carries `extensions.`) → TypeURL `type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy`. All 27 `LatencyThresholdOverride_Opcode` Go identifiers recorded with their 0..26 values (digit-suffixed names intact). All 33 as-built anchor rows HOLD with zero drift. Gate GREEN. Ready to proceed to Task 2 (WriteFilter / WriteFilterCallbacks interfaces).

---

## Task 2: WriteFilter + WriteFilterCallbacks interfaces + concrete writeCallbacks

**Status: PASS — TDD green, gofmt clean, golangci-lint clean.**

### What landed

- `internal/filter/network/types.go`: Added `WriteFilter` interface (embeds `NetworkFilter`, declares `OnWrite`, `SetWriteFilterCallbacks`, `OnDestroy`) and `WriteFilterCallbacks` interface (declares `Connection() Connection`). Inserted after `ReadFilter` at `:48`, before `NetworkFilterFactory`. Both carry `//nolint:revive` per ADR-0221.
- `internal/filter/network/chain.go`: Added `writeCallbacks` struct (fields: `rt *chainRuntime`) + `Connection() Connection` method returning `w.rt.cxn` — the same concrete `*connection` the read callbacks expose. Inserted before the existing `connection` type.
- `internal/filter/network/chain_test.go`: Added `fakeWriteFilter` synthetic double (records `OnWrite` calls, captures `WriteFilterCallbacks` injection, counts `OnDestroy` — reused by Tasks 3–5) + compile-time assertion `var _ WriteFilter = (*fakeWriteFilter)(nil)` (silences `unused` linter) + `TestWriteCallbacksConnectionAccessor` test.

### TDD steps

1. Test written first (no implementation yet) → compile failure: `undefined: WriteFilterCallbacks`, `undefined: writeCallbacks` — exactly the right failure mode.
2. Implementation added (`WriteFilter`/`WriteFilterCallbacks` in types.go + `writeCallbacks` in chain.go).
3. `TestWriteCallbacksConnectionAccessor` → **PASS**.
4. Full package suite `go test ./internal/filter/network/ -race -short` → **PASS** (no existing test broken — `WriteFilter` is satisfied by NO existing type, so no double needed updating).

### Deviation from PLAN snippets

- Added `var _ WriteFilter = (*fakeWriteFilter)(nil)` after the `fakeWriteFilter` method set. Not in the PLAN snippet, but required: `golangci-lint unused` fires on the type+methods when the double has no direct usage in this task's scope. The compile-time assertion follows the established project pattern (`types_test.go:32`, `rbac/rbac_test.go:241`, etc.) and will survive Tasks 3–5 (which add usage anyway).

### Files touched

- `internal/filter/network/types.go`
- `internal/filter/network/chain.go`
- `internal/filter/network/chain_test.go`
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 3: Chain classification restructure (read/write/both/terminal) + dual injection + OnDestroy dedupe

**Status: PASS — TDD green, gofmt clean, golangci-lint clean.**

### What landed

- `internal/filter/network/chain.go`:
  - `NewChainRuntime`: replaced the type-switch with independent type-asserts (SPEC §3.3) — TerminalFilter first-and-continue, then independent `ReadFilter` + `WriteFilter` checks so a both-directions filter lands in BOTH sets (same instance). Added `rt.writeFilters = write` attachment and a post-construction write-callbacks injection loop (`wf.SetWriteFilterCallbacks(&writeCallbacks{rt: rt})`) mirroring the read-callbacks loop in `newChainRuntime`.
  - `chainRuntime` struct: added `writeFilters []WriteFilter` field (with ADR-0221 comment) after `terminal`, before `conn`.
  - `onDestroy`: replaced the simple `for _, f := range rt.filters` loop with a once-per-instance dedupe map (`destroyed map[NetworkFilter]bool`) iterating both `rt.filters` and `rt.writeFilters`. `rt.bucket.Reset()` preserved exactly as before.

- `internal/filter/network/chain_test.go`:
  - Added `fakeBothFilter` double (implements both `ReadFilter` and `WriteFilter`; counts `destroyed int`, captures both `rcb`/`wcb`; includes two compile-time assertions).
  - Added four new tests: `TestClassificationBothDirectionsFilter`, `TestClassificationWriteOnlyFilter`, `TestBothFilterDualCallbackInjection`, `TestOnDestroyOncePerInstance`.

### TDD steps

1. Tests written first → compile failure: `rt.writeFilters undefined (type *chainRuntime has no field or method writeFilters)` — exactly the right failure mode.
2. Implementation added (independent type-asserts + `writeFilters` field + write-callbacks injection + `onDestroy` dedupe).
3. Four new tests → **PASS**.
4. Full package suite `go test ./internal/filter/network/ -race -short` → **PASS** — all existing tests green (`TestChainOnDestroyCallsAllFilters`, `TestPureTerminalImmediateHandoff`, `TestMixedChainBufferedPrefixHandoff`, etc.).

### Adaptations from PLAN snippets

- `recordingTerminal` in PLAN → used existing `recordTerminal` (same type, as-built name).
- `filterA` for `ro` in `TestOnDestroyOncePerInstance`: PLAN cites `filterA` (chain_test.go:182) which has `OnDestroy(){}` with no counter — acceptable since the test only checks `both.destroyed` and `wo.destroyed`, not `ro.destroyed`.
- `destroyFilter.destroyed` is `bool` (not `int`) as-built — `TestChainOnDestroyCallsAllFilters` uses it correctly; no change needed.
- Added two compile-time assertions (`var _ ReadFilter = (*fakeBothFilter)(nil)` and `var _ WriteFilter = (*fakeBothFilter)(nil)`) following project pattern to silence linter.
- No dispatch logic added (Tasks 4–5 scope).

### Files touched

- `internal/filter/network/chain.go`
- `internal/filter/network/chain_test.go`
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 4: writeconn.go — the writeChainConn

**Status: PASS — TDD green, gofmt clean, golangci-lint clean.**

### What landed

- `internal/filter/network/writeconn.go`: `writeChainConn` struct (embeds `net.Conn`, `filters []WriteFilter`) + `newWriteChainConn` constructor + `Write` method. Mirrors `prefixconn.go`'s embed-and-override-one-method shape. Write semantics: run filters front-to-back over a per-call `&Buffer{}` pre-loaded with `p`; on `StopIteration` return `(len(p), nil)` (D-P7 no-forward-parity); on underlying write error return `(0, err)`; on success return `(len(p), nil)`. `endStream` is always `false` (net.Conn.Write carries no half-close signal at 28.1).
- `internal/filter/network/writeconn_test.go`: Six new tests covering: forwarding (all-Continue), stop-iteration-no-forward (D-P7), dispatch order (front-to-back strict via shared `order *[]string` recorder), post-chain-bytes mutation (mutatingWriteFilter appends "XYZ"), underlying-error propagation, and endStream-always-false. Adds two new test doubles: `recordingConn` (records Write payloads, optionally fails) and `mutatingWriteFilter` + `endStreamRecorder`.
- `internal/filter/network/chain_test.go`: Extended `fakeWriteFilter` with an optional `order *[]string` field; `OnWrite` appends `f.name` to `*order` when non-nil. Existing users (Tasks 2–3 tests) unaffected (field defaults to nil).

### TDD steps

1. Tests + new doubles written; `chain_test.go` extended — compile failure: `undefined: newWriteChainConn` (6 sites). Correct failure mode.
2. `writeconn.go` created with SPEC §3.5 implementation, adapted to as-built `&Buffer{}` + `Append` API.
3. `go test ./internal/filter/network/ -run TestWriteChainConn -v` → all 6 PASS.
4. `go test ./internal/filter/network/ -race -short` → PASS (all existing tests green).

### As-built adaptations

- Buffer construction: `&Buffer{}` (zero-value literal) is correct — `buffer.go` has no `NewBuffer()` constructor. `Append(p []byte)` is the correct method name. Both match the SPEC §3.5 snippet verbatim.
- gofmt reformat: the SPEC snippet used extra-space alignment (`filters []WriteFilter  // ...`); gofmt normalised it to single-space + tab-alignment (`filters  []WriteFilter // ...`). No semantic change.

### Files touched

- `internal/filter/network/writeconn.go` (new)
- `internal/filter/network/writeconn_test.go` (new)
- `internal/filter/network/chain_test.go` (extend `fakeWriteFilter` with `order *[]string`)
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 5: `handleTerminal` writeChainConn wrap insertion + back-compat

**Status: PASS — TDD green (4 new tests), gofmt clean, golangci-lint clean. WriteFilter seam COMPLETE.**

### What landed

- `internal/filter/network/chain.go`: Inserted `if len(rt.writeFilters) > 0 { ... }` block in `handleTerminal` AFTER the prefixConn wrap and BEFORE the ctx-override threading. The block builds a REVERSED COPY of `rt.writeFilters` (dispatch order = LIFO parity; AMEND-A11) and wraps `conn` in a `newWriteChainConn(conn, dispatch)`. Zero-write-filter chains get NO wrap — pure insertion, byte-identical to the pre-28.1 path (R1 back-compat).
- `internal/filter/network/upstreamcluster_test.go`: Extended `recordingTerminal` with `gotConn net.Conn` field; `Handle` now records both `ctx` and `conn`. Existing override tests (`TestHandleTerminalThreadsOverrideWhenSet`, `TestHandleTerminalNoOverrideLeavesCtxClean`) stay green — they still use `gotCtx`.
- `internal/filter/network/chain_test.go`: Added 4 new tests:
  - `TestHandleTerminalZeroWriteFiltersUnwrapped` — back-compat: zero-filter chain → no `*writeChainConn` wrap.
  - `TestHandleTerminalWrapComposition` — `writeChainConn` outer, `prefixConn` inner; prefix replay works through the outer wrap.
  - `TestHandleTerminalReverseWriteDispatch` — chain `[A, B]` → dispatch `[B, A]` (AMEND-A11 LIFO).
  - `TestHandleTerminalDoesNotMutateChainOrder` — `rt.writeFilters` slice unchanged after `handleTerminal`.

### TDD steps

1. Tests written (with `recordingTerminal` extended for `gotConn`); `handleTerminal` unchanged → fail:
   - `TestHandleTerminalWrapComposition`: `terminal conn = *network.prefixConn, want *writeChainConn`
   - `TestHandleTerminalReverseWriteDispatch`: `write dispatch order = [], want [B A]`
   - Two trivially-pass cases: `TestHandleTerminalZeroWriteFiltersUnwrapped` (no wrap → correct already) and `TestHandleTerminalDoesNotMutateChainOrder` (no mutation → also vacuously correct pre-impl). All existing tests green.
2. Wrap block inserted. `go test ./internal/filter/network/ -run TestHandleTerminal -v` → all 6 PASS (4 new + 2 existing override tests).
3. `go test ./internal/filter/network/ -race -short` → **PASS** (full package suite, all 47+ existing chain tests green — R1 back-compat confirmed).

### As-built adaptations

- `recordingTerminal` (in `upstreamcluster_test.go`) had only `gotCtx context.Context` — extended with `gotConn net.Conn` rather than creating a duplicate double. Existing callers (`TestHandleTerminalThreadsOverrideWhenSet` / `TestHandleTerminalNoOverrideLeavesCtxClean`) use `_` for conn — updated to named `c` in `Handle` signature; functionally identical.
- `TestHandleTerminalZeroWriteFiltersUnwrapped` and `TestHandleTerminalDoesNotMutateChainOrder` pass both before and after the implementation (property was already vacuously true; post-impl it is actively proven by the contract).
- As-built `handleTerminal` matched the PLAN snippet exactly — pure insertion, no restructuring required.

### Files touched

- `internal/filter/network/chain.go`
- `internal/filter/network/chain_test.go`
- `internal/filter/network/upstreamcluster_test.go`
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 6: `zookeeperproxy` package skeleton + config parse

**Status: PASS — TDD green (4 tests), gofmt clean, golangci-lint clean.**

### What landed

- `internal/filter/network/zookeeperproxy/doc.go`: Package doc explaining phase 28.1 scope (request decoder, 201-counter roster, xid correlation, both-directions filter with stub OnWrite, ADR-0222 reference).
- `internal/filter/network/zookeeperproxy/config.go`:
  - 27 wire opcode constants (`opConnect`..`opAddWatch`) with the upstream decoder.h non-contiguous values (gaps at wire 10 and 20, negative wire -11 for Close, and >100 values for auth/watches/ephemerals; AMEND-A6).
  - `protoToWireOpcode` map: all 27 proto enum→wire opcode entries.
  - `wireOpcodeToOpname` map: 26 wire opcode→stats opname entries (connect absent; SetAuth→"auth" per AMEND-A3).
  - `compiledConfig` struct: 9-field compiled config (sans `stats *rosterStats` — deferred to Task 8/11).
  - `parseConfig`: happy-path only; Task-7 PARSE-REJECT arms not yet present.
- `internal/filter/network/zookeeperproxy/config_test.go`: 4 tests (4 PASS):
  - `TestParseConfig_AllFieldsAndDefaults` — minimal (defaults) + full (all 9 fields) parse.
  - `TestParseConfig_MaxPacketBytesZeroAccepted` — explicit zero max_packet_bytes honored (no defaulting).
  - `TestProtoToWireOpcodeMapping` — byte-stable pin of all 27 proto→wire entries including the 3 divergent groups (gap at Ping, gap at CreateTtl, negative Close, >100 set).
  - `TestWireOpcodeToOpname` — 8 spot-pins (digit-suffixed names intact, SetAuth→"auth" alias).

### As-built adaptations

- **Proto enum identifier spellings**: `GetAcl`/`SetAcl` (not `GetACL`/`SetACL`) and `CreateTtl` (not `CreateTTL`) — taken from Task-1 PROGRESS.md record (pb.go:30-56, v1.32.4). The PLAN template used uppercase forms that would not compile.
- **`stats *rosterStats` field DELIBERATELY OMITTED** from `compiledConfig`: `rosterStats` does not exist until Task 8; including it here would block compilation. Task 11 adds the field when the type exists.
- **gofmt**: one double-space before inline comment on the `Ping` line in the test (`// proto 10 → wire 11`) — corrected to single space.
- **Lint**: zero findings. All symbol usages (opcode constants, both maps, all `compiledConfig` fields) are exercised via the test file's direct references — no `//nolint` annotations needed.
- **Verified proto field names** against pb.go v1.32.4: `StatPrefix string`, `AccessLog string`, `MaxPacketBytes *wrapperspb.UInt32Value`, `EnableLatencyThresholdMetrics bool`, `DefaultLatencyThreshold *durationpb.Duration`, `LatencyThresholdOverrides []*LatencyThresholdOverride`, `EnablePerOpcodeRequestBytesMetrics bool`, `EnablePerOpcodeResponseBytesMetrics bool`, `EnablePerOpcodeDecoderErrorMetrics bool`. All struct-literal field names in the test match exactly.

### Files touched

- `internal/filter/network/zookeeperproxy/doc.go` (new)
- `internal/filter/network/zookeeperproxy/config.go` (new)
- `internal/filter/network/zookeeperproxy/config_test.go` (new)
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 7: PARSE-REJECT arms + byte-stable wording test (ADR-0080; D-S28.1-2)

**Status: PASS — TDD green (3 new tests / 7 new sub-tests), gofmt clean, golangci-lint clean. D-S28.1-2 FINALIZED at this commit.**

### What landed

- `internal/filter/network/zookeeperproxy/config.go`:
  - Added `errors` and `fmt` imports (needed by validation arms).
  - Added the 6 byte-stable PARSE-REJECT error constants (ADR-0080; **DO NOT CHANGE after this commit**):
    - `errStatPrefixRequired` = `"zookeeper_proxy: stat_prefix is required"`
    - `errLatencyOverrideThresholdRequired` = `"zookeeper_proxy: latency_threshold_overrides: threshold is required"`
    - `errLatencyOverrideThresholdTooSmall` = `"zookeeper_proxy: latency_threshold_overrides: threshold must be at least 1ms"`
    - `errLatencyOverrideOpcodeUndefined` = `"zookeeper_proxy: latency_threshold_overrides: opcode is not a defined opcode"`
    - `errDefaultLatencyThresholdTooSmall` = `"zookeeper_proxy: default_latency_threshold must be at least 1ms"`
    - `errLatencyOverrideDuplicateOpcode` = `"zookeeper_proxy: latency_threshold_overrides: duplicate opcode"`
  - Replaced the unvalidated Task-6 override loop in `parseConfig` with the fully-validated loop. Validation order: `stat_prefix` → `default_latency_threshold` → per-override (opcode-defined → threshold-required → threshold-too-small → duplicate). Dynamic detail (opcode value) is appended via `fmt.Errorf` wrapping the constant prefix, following the project shape (rbac.go / tcpproxy pattern).

- `internal/filter/network/zookeeperproxy/config_test.go`:
  - Added `"strings"` import.
  - Added `TestParseRejectConstants_ByteStable`: 6-row table test pinning each constant against its exact wording (D-S28.1-2 record).
  - Added `TestParseConfig_RejectArms`: 7-case table test; each arm fires the corresponding reject and is checked with `strings.Contains`.
  - Added `TestParseConfig_OneMillisecondAccepted`: boundary test confirming 1ms is accepted (PGV `gte` is inclusive) for both `default_latency_threshold` and an override threshold.

### TDD steps

1. Tests added to `config_test.go` (constants not yet defined) → compile failure: 12 `undefined: err*` errors — correct RED failure mode.
2. Constants + validated `parseConfig` added — replaced the unvalidated override loop (two latent traps from Task 6's code-review flags: unknown opcodes mapped to 0/opConnect silently; duplicates wrote last-wins silently).
3. `go test ./internal/filter/network/zookeeperproxy/ -race -v` → **8/8 PASS** (4 Task-6 tests + 3 new Task-7 tests). All sub-tests pass.
4. `gofmt -l` → no output (zero drift). `golangci-lint run` → no findings.

### D-S28.1-2 final wording table (byte-stable from this commit)

| Constant | Exact string |
|----------|-------------|
| `errStatPrefixRequired` | `zookeeper_proxy: stat_prefix is required` |
| `errLatencyOverrideThresholdRequired` | `zookeeper_proxy: latency_threshold_overrides: threshold is required` |
| `errLatencyOverrideThresholdTooSmall` | `zookeeper_proxy: latency_threshold_overrides: threshold must be at least 1ms` |
| `errLatencyOverrideOpcodeUndefined` | `zookeeper_proxy: latency_threshold_overrides: opcode is not a defined opcode` |
| `errDefaultLatencyThresholdTooSmall` | `zookeeper_proxy: default_latency_threshold must be at least 1ms` |
| `errLatencyOverrideDuplicateOpcode` | `zookeeper_proxy: latency_threshold_overrides: duplicate opcode` |

The `errLatencyOverrideOpcodeUndefined` and `errLatencyOverrideDuplicateOpcode` arms use `fmt.Errorf` to append the offending opcode value as dynamic detail (e.g. `"…: 999"` or `"…: Ping"`); the stable prefix is the pinned constant and `strings.Contains` checks pass.

### As-built adaptations

- Task-6's unvalidated loop (`wire := protoToWireOpcode[o.GetOpcode()]` with no comma-ok) silently mapped unknown opcodes to 0 (opConnect) and silently overwrote duplicate entries (last-wins). Both were latent correctness traps flagged by a code-reviewer at Task 6. The Task-7 validated loop fixes both with fail-closed semantics.
- The opcode-defined check uses `protoToWireOpcode` membership (single lookup for both defined-check and wire-value retrieval) rather than the proto-generated `_name` map. Both approaches cover exactly the 27 defined values. Documented in a code comment.
- Validation order matches the PLAN: `stat_prefix` first (load-bearing 0047 fixture arm), then `default_latency_threshold`, then per-override checks in the single loop.

### Files touched

- `internal/filter/network/zookeeperproxy/config.go`
- `internal/filter/network/zookeeperproxy/config_test.go`
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 8: `stats.go` — 201-suffix roster + eager creation + dynamic auth counters (R2; ADR-0222 §4.4; D-P5)

**Status: PASS — TDD green (5 new tests), gofmt clean, golangci-lint clean. Live-dump golden list locked.**

### Step 1: Roster derivation — live dump (authoritative)

Method: live probe of `envoyproxy/envoy:v1.37.2`. Container `zk-roster-probe-281` booted on host port 19901 (admin). Bootstrap: minimal static YAML with `[zookeeper_proxy (stat_prefix: zkprobe), tcp_proxy]` listener + one cluster. Pre-existing containers (`happy_feistel`, `funny_northcutt`) untouched. Container removed after dump.

**Dump command:**
```
curl -s http://127.0.0.1:19901/stats | grep '^zkprobe\.zookeeper\.' | cut -d: -f1 | sed 's/^zkprobe\.zookeeper\.//' | sort > /tmp/zk_roster.txt
```

**Family arithmetic verification (actual command outputs):**

```
$ wc -l /tmp/zk_roster.txt
201 /tmp/zk_roster.txt

$ grep -c '_rq$' /tmp/zk_roster.txt
28

$ grep -c '_rq_bytes$' /tmp/zk_roster.txt
29

$ grep -c '_decoder_error$' /tmp/zk_roster.txt
28

$ grep '^decoder_error$' /tmp/zk_roster.txt
decoder_error

$ grep -c '_resp$' /tmp/zk_roster.txt
28

$ grep -c '_resp_bytes$' /tmp/zk_roster.txt
28

$ grep -c '_resp_fast$' /tmp/zk_roster.txt
28

$ grep -c '_resp_slow$' /tmp/zk_roster.txt
28

$ grep -E '^(decoder_error|request_bytes|response_bytes|watch_event)$' /tmp/zk_roster.txt
decoder_error
request_bytes
response_bytes
watch_event
```

All arithmetic: 4 + 28 + 29 + 28 + 28 + 28 + 28 + 28 = 201. PASS.

**Asymmetries confirmed by live dump:**

| Asymmetry | Expected | Live dump |
|-----------|----------|-----------|
| `auth_rq` absent | YES (AMEND-A3) | Confirmed absent |
| `auth_rq_bytes` present | YES (AMEND-A3) | Confirmed present |
| `connect_readonly_rq` present | YES (AMEND-A3) | Confirmed present |
| `connect_readonly_rq_bytes` present | YES | Confirmed present |
| `connect_readonly_resp` absent | YES | Confirmed absent |
| `setauth_rq` present | **PLAN said absent** | **CONFIRMED PRESENT — PLAN CORRECTION** |
| `setauth_*` counters | **PLAN said no setauth_* counters** | **All 7 setauth_* exist: rq, rq_bytes, resp, resp_bytes, resp_fast, resp_slow, decoder_error** |

**AMEND NOTE (D28-3 prose correction):** The parent SPEC §7.2 and config.go comment state "SetAuth's opname is 'auth' — there are no setauth_* counters". The live dump REFUTES this: `setauth_rq`, `setauth_rq_bytes`, `setauth_resp`, `setauth_resp_bytes`, `setauth_resp_fast`, `setauth_resp_slow`, `setauth_decoder_error` ARE in the upstream macro. The `auth_*` counters (auth_rq_bytes, auth_resp, auth_resp_bytes, auth_resp_fast, auth_resp_slow, auth_decoder_error) are ALSO present. `setauth` and `auth` are DISTINCT opnames in the macro. The `wireOpcodeToOpname[opSetAuth] = "auth"` mapping in config.go (used for dynamic per-scheme auth counter routing) is a decoder-level alias for request counting — the MACRO itself has separate `setauth_*` and `auth_*` roster entries. The `mustNot` test assertion for `setauth_rq` has been REMOVED from the test; the golden list includes all 7 `setauth_*` entries verbatim.

**Sort note:** Shell `sort` (locale-aware) places `createcontainer_*` before `create_*` (locale treats `_` > `c`). Go's `sort.Strings` (byte-lexicographic) places `create_*` before `createcontainer_*` (`_`=95 < `c`=99). The golden list in the test uses Go sort order (the comparison is `sort.Strings(sorted)` vs `golden`).

Docker cleanup: `docker rm -f zk-roster-probe-281` executed; `/tmp/zkprobe.yaml` removed. Pre-existing containers verified still running.

### What landed

- `internal/filter/network/zookeeperproxy/stats.go` (new):
  - Four family tables (`rqOpNames` 28, `rqBytesOpNames` 29, `decoderErrorOpNames` 28, `respOpNames` 28) transcribed verbatim from the live dump.
  - `rosterSuffixes()` — returns the exact 201 suffixes from the four families + 4 plain names.
  - `rosterStats` struct (`prefix`, `reg`, `counters map[string]*stats.Counter`).
  - `newRosterStats(reg, statPrefix)` — eagerly creates all 201 counters via `NewCounterIfAbsent` (idempotent; post-Freeze-permitted; the rbac `newFilterStats` precedent).
  - `inc(suffix)` / `add(suffix, delta)` — look up counter by suffix, panic on unknown (programming-error guard; roster is closed).
  - `authSchemeCounter(scheme)` — lazily creates `<stat_prefix>.zookeeper.auth.<scheme>_rq` via `NewCounterIfAbsent`.

- `internal/filter/network/zookeeperproxy/stats_test.go` (new):
  - `firstDiff(a, b []string)` test helper.
  - `TestCounterRoster_MatchesUpstreamMacro` — 201-count check, family arithmetic check, digit-suffix guard, asymmetry mustNot checks (`auth_rq`, `connect_readonly_resp`), full 201-name Go-sorted golden list comparison.
  - `TestRosterStats_EagerCreation` — 201 counters created, spot-check 4 response-side counters at 0, exercises `add` to prevent lint dead-code.
  - `TestRosterStats_IdempotentSharedPrefix` — two `newRosterStats` on same `reg`+prefix share pointer-identical counters.
  - `TestRosterStats_DynamicAuthSchemeCounters` — `authSchemeCounter("digest")` idempotent, `Inc` + `Load` verified.
  - `TestRosterStats_UnknownSuffixPanics` — `inc("not_a_counter")` panics (deferred recover).

### TDD steps

1. `stats_test.go` written first (no implementation) → compile failure: `undefined: rosterSuffixes`, `undefined: newRosterStats` (6 sites). Correct RED failure mode.
2. `stats.go` created.
3. `go test ./internal/filter/network/zookeeperproxy/ -race -v` → first run: `TestCounterRoster_MatchesUpstreamMacro` FAIL (golden list used shell sort order; corrected to Go sort order). All other 4 new tests + 8 existing tests PASS.
4. Golden list corrected (Go byte-lexicographic order: `create_*` before `createcontainer_*`).
5. `go test ./internal/filter/network/zookeeperproxy/ -race -v` → **13/13 PASS** (5 new Task-8 tests + 8 existing Tasks 6-7 tests).
6. `gofmt -l` → initial finding on `stats.go` (table formatting); `gofmt -w` applied. Clean.
7. `golangci-lint run ./internal/filter/network/zookeeperproxy/...` → zero findings.

### Stat surface delta

136 → **337** at this task's roster creation (all 201 counter objects created eagerly, though increment paths complete at 28.2 for response-side). The `+add` exerciser in `TestRosterStats_EagerCreation` ensures no lint dead-code on the `add` method.

### Files touched

- `internal/filter/network/zookeeperproxy/stats.go` (new)
- `internal/filter/network/zookeeperproxy/stats_test.go` (new)
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 9: `decoder.go` part 1 — framing + reassembly + chain-buffer high-water mark + special-xid dispatch

**Status: PASS — TDD green (9 new tests + 1 xid-constants test), gofmt clean, golangci-lint clean.**

### What landed

- `internal/filter/network/zookeeperproxy/decoder.go` (new):
  - Special xid constants: `connectXid=0`, `watchXid=-1` (response-side, 28.2), `pingXid=-2`, `authXid=-4`, `setWatchesXid=-8`.
  - `pendingRequest` struct: correlation entry (opname, wireOpcode, start time.Time); written at 28.1, consumed at 28.2 (R5 — never read at 28.1).
  - `requestDecoder` struct: `cfg`, `stats`, `chainConsumed int` (high-water mark), `readBuf []byte` (internal reassembly buffer), `requestsByXid map[int32]pendingRequest` (data requests), `controlRequestsByXid map[int32][]pendingRequest` (FIFO queues for control xids).
  - `newRequestDecoder(cfg, rs)` constructor.
  - `decodeOnData(chainBytes []byte)`: appends only new tail bytes past the high-water mark into `readBuf` (D-S28.1-3 no-double-count), then loops `nextFrame` + `decodeFrame`.
  - `nextFrame()`: extracts one 4-byte-prefixed frame from `readBuf`; oversized or negative length → `decoderError("")` + abandon; partial frame → ok=false.
  - `decodeFrame(frame)`: universal 8-byte min-length check; xid-switch to `onConnect`/`onPing`/`onAuth`/`onSetWatches`/`onDataRequest`.
  - `onConnect(frame)`: parses connect framing (28-byte fixed header + password + optional readonly bool); readonly=true → `connect_readonly_rq`, else → `connect_rq`; both paths count `request_bytes` + correlation.
  - `onAuth(frame)`: extracts scheme string; increments `authSchemeCounter(scheme)`; counts `auth_rq_bytes` path via `countRequestBytes("auth", ...)`. **(Task-14 amend: the wire-offset parse bug and the builtin-set scheme gating were both corrected — see the POST-TASK-14 CORRECTION note above and Task 14 §C below. The original `scheme="" → "unknown_scheme"` guard is subsumed by the builtin-set lookup in `authSchemeCounter`.)**
  - `recordControl(xid, opname, wireOpcode)`: appends FIFO entry to `controlRequestsByXid`.
  - `wireFootprint(frame)`: returns `4 + len(frame)` (the full wire wire size including the stripped length prefix).
  - `countRequestBytes(opname, wireBytes)`: always increments `request_bytes`; flag-gated `<opname>_rq_bytes`.
  - `decoderError(opname)`: increments `decoder_error` (always) + flag-gated `<opname>_decoder_error`; abandons `readBuf` (no resync).
  - `onDataRequest(xid, frame)`: minimal version — opcode lookup in `wireOpcodeToOpname`, `decoderError` on unknown; SetAuth guard (no `auth_rq` increment — AMEND-A3); `inc(<opname>_rq)` + `countRequestBytes` + `requestsByXid` write.

- `internal/filter/network/zookeeperproxy/decoder_test.go` (new):
  - Frame builders: `be32`, `be64`, `zkFrame`, `connectFrame`, `dataFrame`.
  - `newTestDecoder(t)` helper: `newRequestDecoder(cfg, rs)` — no `cfg.stats` field assignment (compiledConfig has no stats field; the constructor takes `rs` directly).
  - `counterValue(t, rs, suffix)` helper.
  - `TestSpecialXidConstants`: exercises all 5 constants including `watchXid` (prevents lint "unused constant" — see Lint handling below).
  - 9 `TestDecode*` tests: Connect, ConnectReadonly, Ping, AuthScheme, SetWatches, PartialFrameReassembly, HighWaterMarkNoDoubleCount, TwoFramesOneRead, DoesNotMutateInput.

### TDD steps

1. `decoder_test.go` written first — compile failure: `undefined: requestDecoder`, `undefined: newRequestDecoder`, `undefined: connectXid` (and 9 more) — correct RED failure mode.
2. `decoder.go` created.
3. `go test ./internal/filter/network/zookeeperproxy/ -race -v` → **27/27 PASS** (9 new decoder tests + `TestSpecialXidConstants` + 13 existing Tasks 6-8 tests). First run — no RED/GREEN cycles needed.
4. `gofmt -l internal/filter/network/zookeeperproxy/` → no output (zero drift).
5. `golangci-lint run ./internal/filter/network/zookeeperproxy/...` → zero findings.

### As-built adaptations

- **`cfg.stats` removal**: The PLAN snippet's `newTestDecoder` had `cfg.stats = rs` which would not compile — `compiledConfig` has no `stats` field (deferred to Task 11). The helper correctly constructs `newRequestDecoder(cfg, rs)` directly without the assignment.
- **SetAuth guard**: Inserted after the `known` check and before `inc(opname+"_rq")` as directed. The guard path counts bytes and writes correlation but does NOT call `inc("auth_rq")` (which would panic — that suffix is absent from the roster per AMEND-A3). Task 10 replaces this with the full SetAuth-as-data-request handling.
- **`watchXid` lint handling**: `watchXid=-1` is declared as a response-side constant (28.2) but unused at 28.1. Rather than `//nolint`, `TestSpecialXidConstants` exercises all 5 xid constants (including `watchXid`) with equality assertions. This is the project-preferred pattern (no nolint annotations in production code) and documents the XidCodes contract.
- **Connect frame parsing**: `fixedLen = 4+8+4+8+4 = 28` (protocol_version + last_zxid + timeout + session_id + password_length_field). Password bytes start at offset 28; readonly (if present) follows the password bytes. The `connectFrame` test builder uses `be32(0)` as protocol_version = the sniffed xid (xid=0 = connectXid per AMEND-A5).
- **`decodeFrame` min-length check**: The universal 8-byte check (`xid(4) + opcode(4)`) fires before xid dispatch. The connect frame (xid=0) does NOT reach this check via the switch dispatch — `onConnect` does its own deeper validation. The 8-byte check only applies in the xid-switch default path and before xid extraction; since `len(frame) < 8` is checked before the switch, this is fine for all cases.

### Files touched

- `internal/filter/network/zookeeperproxy/decoder.go` (new)
- `internal/filter/network/zookeeperproxy/decoder_test.go` (new)
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 10: `decoder.go` part 2 — data-request dispatch + min-length table + flag gating + correlation (D-S28.1-1; AMEND-A2/A7/A8)

**Status: PASS — TDD green (7 new tests, 3 Task-9 tests updated), gofmt clean, golangci-lint clean. D-S28.1-1 RESOLVED. AMEND: SetAuth-as-data-request mirrors upstream decode-error (decoder.cc:244 default branch, onDecodeError(nullopt)); the only auth-request path is AuthXid (-4).**

### Step 1: D-S28.1-1 — per-opcode min-length table (upstream decoder.cc v1.37.2)

#### Constants (decoder.cc:12-20)
```
BOOL_LENGTH = 1, INT_LENGTH = 4, LONG_LENGTH = 8
XID_LENGTH = 4, OPCODE_LENGTH = 4
MULTI_HEADER_LENGTH = 9
```
Universal minimum: `XID_LENGTH + INT_LENGTH = 8`.

#### Transcribed min-length table (decoder.cc line refs)

| Opcode(s) | Value | decoder.cc line | Formula |
|-----------|-------|-----------------|---------|
| SetAuth (100) | 20 | 398 | XID+OPCODE+3*INT |
| GetData (4) | 13 | 418 | XID+OPCODE+INT+BOOL |
| Create/Create2/CreateContainer/CreateTTL (1/15/19/21) | 24 | 457 | XID+OPCODE+4*INT |
| SetData (5) | 20 | 490 | XID+OPCODE+3*INT |
| GetChildren/GetChildren2 (8/12) | 13 | 513 | XID+OPCODE+INT+BOOL |
| Delete (2) | 16 | 535 | XID+OPCODE+2*INT |
| Exists (3) | 13 | 553 | XID+OPCODE+INT+BOOL |
| GetAcl (6) | 12 | 571 | XID+OPCODE+INT (pathOnlyRequest) |
| SetAcl (7) | 20 | 585 | XID+OPCODE+3*INT |
| Sync/GetEphemerals/GetAllChildrenNumber (9/103/104) | 12 | 606 | XID+OPCODE+INT (pathOnlyRequest) |
| Check (13) | 16 | 615 | XID+OPCODE+2*INT |
| Multi (14) | 17 | 634 | XID+OPCODE+MULTI_HEADER(9) |
| Reconfig (16) | 28 | 702 | XID+OPCODE+3*INT+LONG |
| SetWatches (101) | 28 | 729 | XID+OPCODE+LONG+3*INT |
| SetWatches2 (105) | 36 | 757 | XID+OPCODE+LONG+5*INT |
| AddWatch (106) | 16 | 792 | XID+OPCODE+2*INT |
| CheckWatches/RemoveWatches (17/18) | 16 | 810 | XID+OPCODE+2*INT |
| Close (-11), Ping (11) | (universal 8) | — | no ensureMinLength call |

Opcodes absent from the `dataRequestMinLength` map use the universal 8-byte minimum.

#### Verified upstream SetAuth dispatch behavior

The upstream data-request opcode switch (`decoder.cc:134-244`) contains NO `OpCodes::SetAuth` case. A data-xid SetAuth (xid > 0, opcode=100) falls to the `default` branch at `decoder.cc:243-247`:
```cpp
default:
    ENVOY_LOG(debug, "zookeeper_proxy: decodeOnData failed: unknown opcode {}",
              enumToSignedInt(opcode));
    callbacks_.onDecodeError(absl::nullopt);
    return absl::nullopt;
```

The `parseAuthRequest` function (decoder.cc:396-413) is ONLY called from the `XidCodes::AuthXid` (-4) control path. **envoy-go mirrors upstream exactly**: a data-xid SetAuth is a `decoder_error` (plain, no per-opcode counter, no correlation write) — the only sanctioned auth-request path is `onAuth` (AuthXid=-4). This decision is load-bearing for the 0046 cross-side differential: if a SetAuth data frame were driven, reference Envoy would show `decoder_error=1` while any extension behavior would diverge.

The `parseAuthRequest` wire layout (decoder.cc:396-413):
- `ensureMinLength(len, XID+OPCODE+INT+INT+INT)` = 20 bytes minimum (decoder.cc:398)
- `offset += OPCODE_LENGTH + INT_LENGTH` skips opcode(4) + type(4) (decoder.cc:401)
- `peekString` reads scheme_len(4) + scheme_bytes (decoder.cc:403)
- `skipString` skips cred (decoder.cc:408)

In the Go frame (xid+opcode+type+scheme_len+scheme+cred_len+cred, length-prefix stripped):
- frame[0:4]=xid, frame[4:8]=opcode(100), frame[8:12]=type, frame[12:16]=scheme_len

> **POST-TASK-14 CORRECTION (frame-layout parse bug — see Task 14 §C below).** The Task-9 `onAuth` CODE diverged from this (correct) prose: the shipped implementation read `schemeLen` at `frame[8:12]` and `scheme` at `frame[12:...]`, treating the opcode position as "type" and the type position as schemeLen. It also used a `min length < 12` (only XID+INT+INT). Against a REAL auth frame (which DOES carry the type field) this read `type` (usually 0) as schemeLen → empty scheme → wrong counter. The Task-9 unit test masked it by omitting the type field from its test frame. Live empirical probing against `envoyproxy/envoy:v1.37.2` proved the real layout (a frame WITHOUT the type field fails upstream's `parseAuthRequest` with "peekString: read beyond buffer size"; WITH it succeeds → `zkauth.zookeeper.auth.digest_rq: 1`). Fixed in the Task-14 amend: min length 20 (XID+OPCODE+INT+INT+INT, decoder.cc:397-398), schemeLen at `frame[12:16]`, scheme at `frame[16:]` — matching the prose above.

#### Correlation map note

`controlRequestsByXid` doc comment extended: **KNOWN 28.1 boundary**: control queues grow unbounded for the connection's lifetime at 28.1 (nothing drains them until the 28.2 response decoder consumes entries); accepted hand-off, documented in PROGRESS (Task 9 reviewer follow-up).

### What landed

- `internal/filter/network/zookeeperproxy/decoder.go`:
  - `controlRequestsByXid` doc comment: added 28.1-boundary/unbounded-growth note.
  - `dataRequestMinLength map[int32]int`: 23-entry table covering all opcodes with >8-byte minimums (D-S28.1-1). **SetAuth is NOT in this table**: the 20-byte ensureMinLength at decoder.cc:398 only applies on the AuthXid (-4) control path; the data-request switch has no SetAuth case and never reaches ensureMinLength for SetAuth.
  - `onDataRequest` (FULL form): opcode lookup → unknown → `decoderError("")`; **SetAuth → `decoderError("")` + return false (upstream parity: decoder.cc:244 default branch, onDecodeError(nullopt), no per-opcode counter, no correlation)**; min-length check from table → short → `decoderError(opname)` (flag-gated per-opcode counter); else `inc(opname+"_rq")` + `countRequestBytes` + `requestsByXid` write.
  - `onSetAuthDataRequest` **DELETED**: the old "documented extension" routing data-xid SetAuth through scheme extraction diverged from upstream. Removed entirely; no references remain.

- `internal/filter/network/zookeeperproxy/decoder_test.go`:
  - Updated 3 Task-9 tests (`TestDecodeHighWaterMarkNoDoubleCount`, `TestDecodeTwoFramesOneRead`, `TestDecodeDoesNotMutateInput`) to use frames meeting the new min-length requirements.
  - Added `padTo(opcode)` helper: returns `make([]byte, minLen-8)` for opcodes in the table, `nil` for universal-8 opcodes.
  - 7 new tests: `TestDecodeDataRequestAllOpcodes` (10 subtests), `TestDecodeSetAuthDataRequest`, `TestDecodeUnknownOpcode`, `TestDecodeOversizedThenRecovers`, `TestDecodeMinLengthViolation`, `TestDecodeFlagGatedRequestBytes`, `TestDecodeCorrelationStructuresPopulated`.
  - Added `google.golang.org/protobuf/types/known/wrapperspb` import (for `TestDecodeOversizedThenRecovers`).

### TDD steps

1. Tests written (Step 2); `padTo` defined; existing tests NOT yet updated → compile success but Task-9 tests fail (getdata/create/exists with nil payload now violate min-length) → identified 3 tests to update.
2. Updated 3 Task-9 tests with explicit padded payloads.
3. `go test ./internal/filter/network/zookeeperproxy/ -race -v` → **35/35 PASS** (7 new + 3 updated + 25 prior).
4. `gofmt -l` → drift in decoder.go + decoder_test.go (tab-aligned comment block, test comment); `gofmt -w` applied.
5. `golangci-lint run` → one finding: `behaviour` misspelling → corrected to `behavior`. Clean.

### As-built adaptations

- **3 Task-9 test updates required**: `TestDecodeHighWaterMarkNoDoubleCount` used `dataFrame(1, opGetData, nil)` (8 bytes) and `dataFrame(2, opCreate, nil)` (8 bytes); now need `make([]byte, 5)` (13 bytes) and `make([]byte, 16)` (24 bytes) respectively. `TestDecodeTwoFramesOneRead` same. `TestDecodeDoesNotMutateInput` switched from `opGetData` to `opClose` (no min-length entry → universal 8).
- **SetAuth data-xid upstream behavior**: verified to be plain `decoder_error` in upstream (decoder.cc:244 default branch, `onDecodeError(nullopt)` — no per-opcode counter, no correlation write). envoy-go mirrors upstream exactly; `onSetAuthDataRequest` removed; `TestDecodeSetAuthDataRequest` rewritten to assert `decoder_error=1`, `auth.digest_rq=0`, `setauth_rq=0`, `requestsByXid[5]` absent. The 20-byte ensureMinLength at decoder.cc:398 is only reachable via the AuthXid (-4) `onAuth` path, so `opSetAuth` is NOT in `dataRequestMinLength`.
- **`padTo` placement**: defined in the Task-10 test block (after `TestDecodeDoesNotMutateInput`); Go packages allow forward references within the same file/package so existing tests with `make([]byte, N)` inline padding needed no `padTo` calls.

### Files touched

- `internal/filter/network/zookeeperproxy/decoder.go`
- `internal/filter/network/zookeeperproxy/decoder_test.go`
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 11: Filter glue — `zookeeperproxy.go` (TypeURL + NewFactory + both-directions filter)

**Status: PASS — TDD green (7 new tests: TypeURL pin + factory reject/accept/malformed + 5 filter behavior tests), gofmt clean, golangci-lint clean. zookeeperproxy package production surface COMPLETE.**

### What landed

- `internal/filter/network/zookeeperproxy/zookeeperproxy.go` (new):
  - `TypeURL` — derived via `proto.MessageName(&zookeeper_proxyv3.ZooKeeperProxy{})` (never hand-typed; `reference_network_filter_typeurl_extensions`; `rbac.go:38` precedent).
  - `NewFactory(reg *stats.Registry) network.NetworkFilterFactory` — closure-captures `reg`; the factory parses + validates once at boot (ADR-0079); creates the 201-counter roster eagerly on `parseConfig` success; returns a `FilterInstanceFactory` that allocates a fresh `*filter` per connection with a per-connection `*requestDecoder`. Both factory and instance-factory match the `network.NetworkFilterFactory` / `network.FilterInstanceFactory` as-built signatures exactly.
  - `filter` struct — embeds `network.Marker`, holds `cfg *compiledConfig` (shared), `decoder *requestDecoder` (per-connection), `cb network.ReadFilterCallbacks`, `wcb network.WriteFilterCallbacks`.
  - `OnNewConnection() → Continue` (sticky-halt-safe; `reference_network_read_filter_onnewconnection_halts`).
  - `OnData(buf, endStream) → Continue` — feeds `buf.Bytes()` to `decoder.decodeOnData` (FULL buffer; high-water mark on decoder skips already-consumed bytes; D-S28.1-3); NEVER drains the chain buffer (R3 unconditional passthrough; AMEND-A8).
  - `OnWrite(_ *network.Buffer, _ bool) → Continue` — PURE no-op at 28.1 (SPEC §4.7 pin); no buffering, no counter increments.
  - `SetReadFilterCallbacks` / `SetWriteFilterCallbacks` — store both injections verbatim (both-directions dual injection; D-P2/§3.3).
  - `OnDestroy()` — drops the per-connection decoder (`f.decoder = nil`); the chain runtime calls this exactly once per instance (the §3.3 dedupe).

- `internal/filter/network/zookeeperproxy/config.go` (modified):
  - Added `stats *rosterStats` field to `compiledConfig` with boot-attachment comment (the Task-6 deferral lands here; the "DEFERRED" note replaced with the actual attachment description).

- `internal/filter/network/zookeeperproxy/zookeeperproxy_test.go` (new):
  - `mustAny(t, msg proto.Message)` helper — mirrors `rbac_test.go:41` shape; accepts `proto.Message` (generic, not type-specific) since the test only needs this package's proto.
  - `newTestFilter(t)` helper — `NewFactory` + good typed_config + `instFactory().(*filter)`.
  - `TestTypeURLViaProtoMessageName` — pins the literal against the proto-derived value.
  - `TestNewFactoryParseAndReject` — reject (missing stat_prefix) + accept (201 counters pre-created at 0) + shared-cfg/independent-decoder invariants.
  - `TestNewFactoryMalformedAny` — `0xff 0xff` value bytes → `"zookeeper_proxy: invalid typed_config: …"` prefix check.
  - Compile-time interface assertions (`var _ network.ReadFilter = (*filter)(nil)` + `var _ network.WriteFilter = (*filter)(nil)`).
  - `TestFilterOnDataPassthroughNeverDrains` — `OnData` → Continue + `buf.Len()` unchanged.
  - `TestFilterMultiReadNoDoubleCount` — two-read accumulation; `getdata_rq=1`, `exists_rq=1` (no double-count via high-water mark).
  - `TestFilterOnWritePureNoOp` — `OnWrite` → Continue + buffer untouched + `decoder_error=0`, `response_bytes=0`.
  - `TestFilterOnNewConnectionContinue` — returns Continue.
  - `TestFilterCallbacksAndDestroy` — nil injections stored; `OnDestroy` → `f.decoder == nil`.

### TDD steps

1. `zookeeperproxy_test.go` written; `TypeURL`, `NewFactory`, `filter` undefined → 9 compile errors — correct RED failure mode.
2. `zookeeperproxy.go` created + `stats *rosterStats` field added to `config.go`.
3. `go test ./internal/filter/network/zookeeperproxy/ -race -v` → **43/43 PASS** (7 new + 36 existing). First run — no RED/GREEN cycles needed.
4. `gofmt -l` → drift in `zookeeperproxy_test.go` (double-space before inline comment on `SetReadFilterCallbacks` line); `gofmt -w` applied. Clean.
5. `golangci-lint run ./internal/filter/network/zookeeperproxy/...` → zero findings.

### As-built adaptations

- **`mustAny` signature**: PLAN cited `mustAny(t, msg)` for `*zookeeper_proxyv3.ZooKeeperProxy`. The as-built version accepts `proto.Message` (the `anypb.New` parameter type), matching the `rbac_test.go:41` spirit while being usable for the malformed-Any test (where we construct `*anypb.Any` directly — no `mustAny` needed there).
- **`malformedAny` construction**: `tc.Value = []byte{0xff, 0xff}` with a non-empty `GetValue()` triggers the `tc.UnmarshalTo(&msg)` branch (the `len(tc.GetValue()) > 0` guard in `NewFactory` is required — an empty-value Any with no proto-set fields is valid for the `ZooKeeperProxy{}` case; the guard avoids an unnecessary unmarshal when the typed_config is omitted entirely, but allows the error to surface on genuinely malformed bytes).
- **No `FactoryCtx` adaptation needed**: `network.FactoryCtx{}` zero-literal is valid as-built (the struct has no required fields; the factory ignores it entirely).
- **`cfg.stats` field addition**: the decoder-test `newTestDecoder` helper was confirmed to construct `newRequestDecoder(cfg, rs)` directly (no `cfg.stats` assignment) — adding the field to `compiledConfig` does NOT break any existing test.

### Files touched

- `internal/filter/network/zookeeperproxy/zookeeperproxy.go` (new)
- `internal/filter/network/zookeeperproxy/zookeeperproxy_test.go` (new)
- `internal/filter/network/zookeeperproxy/config.go` (add `stats *rosterStats` field)
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 12: 7th built-in + `bootstrap.go` blank-import + boot smoke

**Status: PASS — TDD green (3 new tests: AllSeven + RegistersZookeeperProxy + ZookeeperProxyBootSmoke), gofmt clean, golangci-lint clean. `go build ./...` clean. `./internal/bootstrap/` PASS.**

### What landed

- `internal/filter/network/builtins/builtins.go` (modified):
  - Package doc updated: "six built-in network filters (echo, direct_response, tcp_proxy, HCM, rbac_network, sni_cluster)" → "seven built-in network filters (…, sni_cluster, zookeeper_proxy)".
  - `RegisterBuiltins` doc comment updated to list `zookeeper_proxy`.
  - Import added: `"github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy"`.
  - Registration after `snicluster.New`: `reg.Register(zookeeperproxy.TypeURL, zookeeperproxy.NewFactory(deps.StatsRegistry))` with ADR-0221/ADR-0222 comment (both-directions precedent; stats-PRIMARY closure-capture; rbac_network/D-26.3-3 precedent).

- `internal/filter/network/builtins/builtins_test.go` (modified):
  - Imports added: `zookeeper_proxyv3`, `proto`, `zookeeperproxy`.
  - `mustAny(t, msg proto.Message)` helper added (mirrors `zookeeperproxy_test.go:16` / `rbac_test.go:41` shape; accepts `proto.Message` for generality).
  - `TestRegisterBuiltinsRegistersAllSix` renamed to `TestRegisterBuiltinsRegistersAllSeven`; `zookeeperproxy.TypeURL` added to the asserted set; doc comment updated to mention 7 filters.
  - `TestRegisterBuiltins_RegistersZookeeperProxy` — proves lookup of `zookeeperproxy.TypeURL` post-Freeze; non-nil `StatsRegistry` supplied (mirrors `TestRegisterBuiltins_RegistersRBACNetwork`).
  - `TestZookeeperProxyBootSmoke` — factory lookup → `mustAny(ZooKeeperProxy{StatPrefix:"zkboot"})` → `factory(tc, FactoryCtx{})` → `instFactory()` → ReadFilter+WriteFilter interface assertions → 4-counter spot-check at 0 (connect_rq, getdata_resp, watch_event, decoder_error).

- `internal/bootstrap/bootstrap.go` (modified):
  - Blank-import added after `sni_cluster/v3`: `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"` with comment matching the sni_cluster/echo/rbac style (Phase-28.1; round-trip guarantee; differential harness; ADR-0016 amendment policy / PROGRESS not a new ADR).

### main.go single-insertion-point confirmation

`grep -n "reg.Register\|RegisterBuiltins" cmd/envoy-go/main.go` → line 222 only: `builtins.RegisterBuiltins(netReg, builtins.Deps{…})`. Single insertion point confirmed; no parallel edit to `main.go` required.

### TDD steps

1. Tests written (`TestRegisterBuiltinsRegistersAllSeven`, `TestRegisterBuiltins_RegistersZookeeperProxy`, `TestZookeeperProxyBootSmoke`) with `zookeeperproxy` import undefined.
2. `go test ./internal/filter/network/builtins/ -run 'AllSeven|Zookeeper' -v` → **3 FAIL** (TypeURL not registered — correct RED).
3. `builtins.go`: import + `reg.Register(zookeeperproxy.TypeURL, ...)` + doc update. `bootstrap.go`: blank-import added.
4. `go test ./internal/filter/network/builtins/ -run 'AllSeven|Zookeeper' -v` → **3 PASS** (GREEN).
5. `go test ./internal/filter/network/builtins/ -race -short -v` → **7/7 PASS** (3 new + 4 prior).
6. `go build ./...` → clean. `go test ./internal/bootstrap/ -race -short` → PASS.
7. `gofmt -l` → no drift. `golangci-lint run` → zero findings.

### As-built adaptations

- **`mustAny` already needed in the test file**: the boot-smoke test calls `mustAny(t, &zookeeper_proxyv3.ZooKeeperProxy{...})`. The existing `builtins_test.go` had no `mustAny` helper (unlike `zookeeperproxy_test.go` and `rbac_test.go`). Added `mustAny(t, proto.Message)` mirroring the `zookeeperproxy_test.go:16` generic shape. The `mkTcpProxyAny` helper already in the file uses `anypb.New` inline; `mustAny` is the cleaner form for the new tests.
- **PLAN's `TestZookeeperProxyBootSmoke` used `mustAny(t, &zookeeper_proxyv3.ZooKeeperProxy{...})` directly**: no adaptation needed; the new `mustAny` helper matches that call site exactly.
- **Existing `TestRegisterBuiltinsRegistersAllSix` → AllSeven**: renamed and extended in-place (not duplicated). The doc comment was updated to list 7 filters and explain nil-StatsRegistry tolerance for closure-only registration.

### Files touched

- `internal/filter/network/builtins/builtins.go`
- `internal/filter/network/builtins/builtins_test.go`
- `internal/bootstrap/bootstrap.go`
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 13: `.zookeeper.` `name.go` INLINE-PREFIX arm (D-P8)

**Status: PASS — TDD green (5 new tests), gofmt clean, golangci-lint clean.**

### What landed

- `internal/stats/name.go`: Added the `.zookeeper.` inline-prefix arm in the `default` branch, after the `.rbac.` arm and before the final error return. Detection: `strings.Index(internal, ".zookeeper.")` + `idx > 0` guard (non-empty, non-leading) + `!strings.ContainsRune(prefix, '.')` guard (dot-free head = single stat_prefix segment). On match: `base = "envoy_" + strings.ReplaceAll(internal, ".", "_")` — full dot→underscore, NO label promotion. Returns `(base, nil, nil)` immediately (skip SN4 status-class collapse; zookeeper names carry no `_Nxx` suffixes).

- `internal/stats/name_test.go`: 5 new tests appended after the NetworkRBAC group:
  - `TestFlattenToProm_Zookeeper_Basic` — `zk.zookeeper.getdata_rq` → `envoy_zk_zookeeper_getdata_rq`, empty labels.
  - `TestFlattenToProm_Zookeeper_DottedDynamicAuth` — `zk.zookeeper.auth.digest_rq` → `envoy_zk_zookeeper_auth_digest_rq` (dotted counter name flattened via full-string replacement).
  - `TestFlattenToProm_Zookeeper_DigitSuffixed` — `create2_rq` and `getallchildrennumber_rq` flatten intact (digit guard).
  - `TestFlattenToProm_Zookeeper_DottedPrefixRejected` — `a.b.zookeeper.getdata_rq` errors (dot-free-prefix guard; dotted stat_prefix rejected).
  - `TestFlattenToProm_Zookeeper_UnderscorePrefix` — `zk_flags.zookeeper.getdata_rq_bytes` → `envoy_zk_flags_zookeeper_getdata_rq_bytes` (underscores in prefix pass through).

### TDD steps

1. 5 tests appended to `name_test.go` (arm not yet present) → `go test ./internal/stats/ -run TestFlattenToProm_Zookeeper -v`: 4 FAIL (Basic, DottedDynamicAuth, DigitSuffixed, UnderscorePrefix), 1 vacuously PASS (DottedPrefixRejected — falls to default error before the arm exists). Correct RED failure mode.
2. Arm inserted into `name.go` (after rbacSegment block, before final `fmt.Errorf`).
3. `go test ./internal/stats/ -race -v` → **PASS** (all tests including all prior tests + 5 new zookeeper tests + `TestFlattenToProm_Invalid_NoMatchingRule` unchanged/green).
4. `gofmt -l internal/stats/` → no output (zero drift).
5. `golangci-lint run ./internal/stats/...` → no output (zero findings).

### As-built adaptations

- The arm uses the as-built local variable name `internal` (matches the `flattenToProm` parameter) and `base` (the pre-declared `var base string`). Both are consistent with the `.rbac.` arm immediately above — no deviation needed.
- The return is `return base, nil, nil` (no label slice; nil is the zero value for `[]Label`), consistent with the wasm arm's `return base, nil, nil` pattern.
- `TestFlattenToProm_Zookeeper_DottedPrefixRejected` uses `strings.Index` semantics: `"a.b.zookeeper.getdata_rq"` has `.zookeeper.` at index 3; `prefix = "a.b"` which ContainsRune('.') → rejected. The dotted-prefix guard fires correctly.

### Files touched

- `internal/stats/name.go`
- `internal/stats/name_test.go`
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 14: `FuzzZookeeperRequestDecode` — the 37th fuzzer + control-frame decoder_error coverage gap closure

**Status: DONE_WITH_CONCERNS (fuzz-found real bug triaged and fixed; no remaining crashers after fix; fuzzer count 37 confirmed).**

### Part A: The fuzzer

#### Step 1: fuzzer written

`internal/filter/network/zookeeperproxy/fuzz_test.go` created (new file). The fuzzer is `FuzzZookeeperRequestDecode` — the 37th fuzzer. It asserts three invariants:
1. No panic (implicit — any panic fails the fuzz run).
2. The input slice is never mutated (R3; chain buffer immutability).
3. The decoder-internal reassembly buffer is bounded by `maxPkt+8` bytes after both `decodeOnData` calls.

Six seeds: connectFrame(nil), ping, data/GetData, garbage, oversized length prefix (1<<20 > maxPkt=1024), partial frame (first 6 bytes of a create frame).

Invariant-3 bound reasoning: after `decodeOnData` returns, the loop has processed all complete frames. `readBuf` holds at most one partial frame = 4-byte prefix + up to `maxPkt-1` body bytes = `maxPkt+3` bytes. The `+8` slack accommodates any frame-header growth. This holds for both single and cumulative calls because (a) oversized frames trigger `decoderError` → `readBuf=nil`, and (b) the high-water mark prevents double-accumulation.

#### Step 2: fuzz runs

**Seed corpus run (before bug fix, failed on seed#0–5, PASS):**
```
$ go test ./internal/filter/network/zookeeperproxy/ -run FuzzZookeeperRequestDecode -v
=== RUN   FuzzZookeeperRequestDecode/seed#0 ... PASS
=== RUN   FuzzZookeeperRequestDecode/seed#1 ... PASS
[... seed#2–5 PASS ...]
PASS  ok ... 0.002s
```

**30-second fuzz run (first attempt — CRASHER FOUND):**
```
fuzz: elapsed: 0s, gathering baseline coverage: 6/6 completed
fuzz: elapsed: 3s, minimizing
--- FAIL: FuzzZookeeperRequestDecode (3.05s)
    testing.go:1927: panic: stats: invalid metric name:
        "fuzz.zookeeper.auth.00\x00\x00\x00 \xff\xff\xff\xfc0000\x00\x00\x00\x1400_rq"
        (must match ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$)
    ... (stats.(*Registry).checkName → authSchemeCounter → onAuth)
Failing input written to testdata/fuzz/FuzzZookeeperRequestDecode/43597239f63b8e0d
```

**Triage and root cause:**

The crasher input (minimized by the fuzzer): `\x00\x00\x00 \xff\xff\xff\xfc0000\x00\x00\x00\x1400` (18 bytes).

Decode: 4-byte length prefix = 0x20 = 32. Frame payload needs 32 bytes. First `decodeOnData(data)`: 14 bytes of payload < 32 → partial frame, `readBuf = data`. Second `decodeOnData(data+data)`: `chainConsumed=18`, appends data[18:36] = second copy. `readBuf = 32 bytes`. `nextFrame` extracts frame (32 bytes):
- frame[0:4] = `\xff\xff\xff\xfc` → xid = -4 (authXid)
- frame[4:8] = `0000` → type
- frame[8:12] = `\x00\x00\x00\x14` → schemeLen = 20
- frame[12:32] = 20 bytes: `\x30\x30` + 18 NUL bytes

`onAuth` passes the 20-byte scheme string (containing `\x00` bytes) to `authSchemeCounter`, which constructs `"fuzz.zookeeper.auth.<scheme>_rq"` and calls `NewCounterIfAbsent`. The stats registry's `checkName` validates the name against `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` — NUL bytes fail this → PANIC.

**Root cause:** `onAuth` accepted any UTF-8 byte sequence as a scheme name without validating it was a valid metric-name component. ZooKeeper auth schemes are simple ASCII identifiers (`digest`, `sasl`, `ip`, `x509`); arbitrary wire bytes must be sanitized.

**Fix applied to `decoder.go`:**

Added `isValidSchemeName(scheme string) bool` helper: returns true only for non-empty strings containing exclusively `[a-zA-Z0-9_]` (the ASCII characters safe in any metric-name segment). The existing `scheme == "" → "unknown_scheme"` guard was replaced with `!isValidSchemeName(scheme) → "unknown_scheme"`. This handles: empty scheme, NUL bytes, non-ASCII bytes, punctuation — all fall back to `"unknown_scheme"` without a panic.

The crasher was left in `testdata/fuzz/FuzzZookeeperRequestDecode/43597239f63b8e0d` (it is automatically run as a regression seed on normal `go test`).

**30-second fuzz run (after fix — PASS):**
```
fuzz: elapsed: 0s, gathering baseline coverage: 30/30 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s,  execs: 171564 (57168/sec), new interesting: 24 (total: 54)
fuzz: elapsed: 6s,  execs: 369607 (66030/sec), new interesting: 42 (total: 72)
fuzz: elapsed: 9s,  execs: 565812 (65402/sec), new interesting: 48 (total: 78)
fuzz: elapsed: 12s, execs: 764603 (66250/sec), new interesting: 52 (total: 82)
fuzz: elapsed: 15s, execs: 956326 (63876/sec), new interesting: 53 (total: 83)
fuzz: elapsed: 18s, execs: 1154310 (66027/sec), new interesting: 53 (total: 83)
fuzz: elapsed: 21s, execs: 1350107 (65281/sec), new interesting: 54 (total: 84)
fuzz: elapsed: 24s, execs: 1547975 (65953/sec), new interesting: 54 (total: 84)
fuzz: elapsed: 27s, execs: 1747310 (66438/sec), new interesting: 55 (total: 85)
fuzz: elapsed: 30s, execs: 1939563 (64082/sec), new interesting: 55 (total: 85)
PASS  ok  github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy  30.146s
```

1,939,563 executions. Zero crashers after fix.

#### Step 3: Fuzzer count verified

```
$ grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l
37
```

Confirmed: 37. PASS.

### Part B: Control-frame decoder_error coverage gap closure

Added `TestDecodeControlFrameErrors` to `decoder_test.go` with 6 sub-tests:

| Sub-test | Frame | Expected | Result |
|----------|-------|----------|--------|
| short connect frame | 12-byte xid=0 frame | decoder_error=1, connect_decoder_error=1 | PASS |
| negative connect pwLen | 28-byte xid=0 frame, pwLen=-1 | connect_decoder_error=1 | PASS |
| short auth frame | 8-byte xid=-4 frame | auth_decoder_error=1 | PASS |
| negative auth schemeLen | 12-byte xid=-4 frame, schemeLen=-1 | auth_decoder_error=1 | PASS |
| sub-8-byte frame | 4-byte xid=1 frame (< universal 8 min) | decoder_error=1 | PASS |
| flag off gates per-opcode | flag=false, short connect | decoder_error=1, connect_decoder_error=0 | PASS |

Frame-shape analysis: the "short connect frame" uses `zkFrame(be32(connectXid), be32(0), be32(0))` → 12-byte payload. `decodeFrame` universal 8-byte check passes (12 ≥ 8); `onConnect` first check `len(frame) < 28` → true → `decoderError("connect")`. The "sub-8-byte frame" uses `zkFrame(be32(1))` → 4-byte payload; `decodeFrame` fires `decoderError("")` before any xid dispatch.

### Full suite

`go test ./internal/filter/network/zookeeperproxy/ -race -short -count=1` → **PASS** (50 tests + 7 fuzzer seeds including the regression entry).

### gofmt + lint

`gofmt -l` → one finding on `fuzz_test.go` (trailing comment alignment); `gofmt -w` applied. Zero drift after.

`golangci-lint run ./internal/filter/network/zookeeperproxy/...` → zero findings.

### Files touched

- `internal/filter/network/zookeeperproxy/fuzz_test.go` (new — the 37th fuzzer)
- `internal/filter/network/zookeeperproxy/decoder_test.go` (Part B: `TestDecodeControlFrameErrors` + section header)
- `internal/filter/network/zookeeperproxy/decoder.go` (bug fix: `isValidSchemeName` + `onAuth` guard — **subsequently superseded by Part C below**)
- `internal/filter/network/zookeeperproxy/testdata/fuzz/FuzzZookeeperRequestDecode/43597239f63b8e0d` (new — regression seed for the fuzz-found bug)
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

### Part C: Two upstream-parity bugs found via live empirical verification (amended into this commit)

Live probing of `envoyproxy/envoy:v1.37.2` (admin `/stats`, against a running ZooKeeper-proxy listener with `stat_prefix: zkauth`) plus a re-read of upstream source v1.37.2 surfaced two divergences in the Task-9 `onAuth` / Task-8 `authSchemeCounter` work. Both fixed here; no API change, no new files.

**Empirical pins (live /stats lines):**
- scheme `digest` (a builtin) → `zkauth.zookeeper.auth.digest_rq: 1`
- scheme `foobar` (valid charset, NOT a builtin) → `zkauth.zookeeper.auth.unknown_scheme_rq: 1` (NO `auth.foobar_rq` is ever created)
- The dynamic-counter shape `<stat_prefix>.zookeeper.auth.<scheme>_rq` is confirmed correct (the `.zookeeper.` segment is the scope); envoy-go's `stats.go` prefix was already right — NO change there.

**Bug 1 — auth frame wire-layout parse bug (Fix A, `decoder.go`).** The real auth frame is `[len] xid | opcode(100) | type | schemeLen | scheme | credLen | cred`. Upstream `parseAuthRequest` (decoder.cc:396-413) does `ensureMinLength(XID+OPCODE+INT+INT+INT = 20)` (decoder.cc:397-398: xid + opcode + type + scheme-len + cred-len), then "Skip opcode + type" (`offset += 8` after xid), then `peekString` reads schemeLen+scheme — so schemeLen is at frame offset 12, scheme at 16. The shipped `onAuth` read schemeLen at `frame[8:12]` and scheme at `frame[12:]` with a 12-byte minimum — treating the opcode position as "type" and the type position as schemeLen. Against a real auth frame this reads `type` (usually 0) as schemeLen → empty scheme → wrong counter. The Task-9 unit test masked it by omitting the type field. **Fix:** min length 20; schemeLen at `frame[12:16]`; scheme at `frame[16:16+schemeLen]`; doc comment cites the layout + decoder.cc:397-398 + the "Skip opcode + type" step. (See also the POST-TASK-14 CORRECTION note in the Task 9 section.)

**Bug 2 — builtin auth-scheme set (Fix B, `stats.go`).** Upstream registers a fixed builtin set in `filter.cc:45-46` (`rememberBuiltins {"auth_rq","digest_rq","host_rq","ip_rq","ping_response_rq","world_rq","x509_rq"}`) and `onAuthRequest` calls `getBuiltin(scheme+"_rq", fallback=unknown_scheme_rq)` (filter.cc:306-310). The shipped `authSchemeCounter` created a counter for ANY valid-charset scheme → divergence (a non-builtin scheme like `foobar` got its own counter instead of `unknown_scheme`) + unbounded cardinality. **Fix:** `authSchemeBuiltins` map gating `authSchemeCounter` — a scheme outside the set takes the `unknown_scheme` fallback (`ping_response` IS in the set because upstream shares one StatNameSet; mirrored for exactness).

**`isValidSchemeName` removal rationale (Fix A/B).** The Task-14 fuzz-found NUL-byte panic was originally fixed with an `isValidSchemeName` charset gate. That helper is now REMOVED entirely: the builtin-set lookup subsumes it — arbitrary wire bytes are never used in a counter name (any non-builtin, including garbage bytes, collapses to `unknown_scheme`), so charset sanitization is unnecessary. The fuzz regression seed `43597239f63b8e0d` still passes (garbage scheme → not in builtins → `unknown_scheme`, no panic).

**Tests (Fix C).**
- `TestDecodeAuthScheme`: rebuilt on the REAL layout via a new `authFrame(scheme)` helper (`xid | opcode(100) | type(0) | schemeLen | scheme | credLen`); asserts `auth.digest_rq == 1`.
- `TestDecodeAuthSchemeNonBuiltin` (new): real-layout frame, scheme `foobar` → asserts `zk.zookeeper.auth.unknown_scheme_rq == 1` AND scans the registry (`reg.Walk`) to prove `auth.foobar_rq` was NEVER created (deliberately NOT probed via `NewCounterIfAbsent`, which would create it at 0 and mask the divergence).
- `TestDecodeControlFrameErrors`: "short auth frame" sub-test updated to use a 16-byte frame (< 20-byte floor); "negative auth schemeLen" sub-test uses a 20-byte frame (passes floor, hits negative check) with 4 pad bytes so the branch is independently exercised.
- `TestRosterStats_DynamicAuthSchemeCounters`: `digest` still gets its own (builtin) counter; added a non-builtin assertion (`kerberos`/`sasl` both return the SAME `unknown_scheme` counter instance, name `zk.zookeeper.auth.unknown_scheme_rq`).

**Gates (all green):** package `-race -count=1 -v` all pass (incl. the two new/updated auth tests); `-run FuzzZookeeperRequestDecode` seed + regression-seed pass; `-fuzz ... -fuzztime 30s` → 1,960,548 execs, zero crashers; `gofmt -l` empty; `golangci-lint` clean; `go build ./...` clean; cross-package `./internal/... -race -short` clean (a one-off `wasm` RootVM cross-stream flake under parallel-load contention reproduced once and cleared on re-run; unrelated — only `zookeeperproxy` + this doc were touched).

**Files touched (Part C):** `decoder.go` (onAuth rewrite + `isValidSchemeName` removal), `stats.go` (`authSchemeBuiltins` + `authSchemeCounter` gating), `decoder_test.go` (`authFrame` helper + the two auth tests + control-frame error sub-tests), `stats_test.go` (`TestRosterStats_DynamicAuthSchemeCounters`), this PROGRESS.md.

---

## Task 15: TCPSink BackendKind=28 runner plumbing + 0046 driver part 1 (bootstraps + frame builders + wiring)

**Status: DONE_WITH_CONCERNS — discovered-but-red (expected at Task 15). `go vet` + `go build` + `gofmt` + `golangci-lint` clean; 0046 CompareBytes PASS with placeholder; existing fixtures 0001/0043/0045 no regression.**

### What landed

- `test/differential/fixture/fixture.go`: Added `TCPSink BackendKind = 28` after `HTTPWasmPerRoute = 27`. The constant carries the full rationale comment (silent backend for 0046; echoing backend would cause cross-side *_resp/decoder_error divergence due to 28.1 OnWrite no-op stub; D-S28.1-5).

- `test/differential/runner_test.go`:
  - Blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0046-zookeeper-requests/driver"` added after the 0045 line.
  - `case fixture.TCPSink` arm added to the backend-kind switch, using the as-built shape (`net.Listen("tcp", "0.0.0.0:0")` + `defer` close + `bo.ln`/`bo.port`/`bo.accepts` fields + `go acceptSinkCounting`). No subprocess (no `bo.proc`).
  - `acceptSinkCounting(ln, counter)` added after the existing `acceptEchoCounting` function: accepts connections, counts them, drains via `io.Copy(io.Discard, c)`, never writes.

- `test/fixtures/0046-zookeeper-requests/driver/driver.go` (new):
  - Package doc: fixture taxonomy (7 arms), TCPSink rationale, StatsAsserter-as-load-bearing-proof note, cross-references.
  - Constants: `fixtureName`, `refAdminPort=9901`, `refLPlainPort=15047`, `refLFlagsPort=15048` (15046 was already taken by 0044-network-rbac-boot-reject).
  - Local opcode constants (driver packages cannot import internal/): `drvOpConnect=0`, `drvOpCreate=1`, `drvOpExists=3`, `drvOpGetData=4`, `drvOpGetChildren2=12`, `drvOpCreate2=15`, `drvOpSetWatches2=105`, `drvOpClose=-11`, `drvOpPing=11`.
  - Frame-crafting helpers: `be32`, `be64`, `zkFrame`, `connectFrame(readonly)`, `dataFrame(xid, opcode, payload)`, `pingFrame()`.
  - Payload builders (meeting the per-opcode min-length table from decoder.go): `getdataPayload(path)` (min 13), `createPayload(path)` (min 24), `getchildren2Payload(path)` (min 13), `setwatches2Payload()` (min 36, exactly met), `existsPayload(path)` (min 13), `closePayload()` (universal 8-byte min, xid+opcode only).
  - Two bootstraps: `ReferenceBootstrap` (STRICT_DNS, `host.docker.internal`, no `nodeLine`) + `SubjectConfig` (STATIC, `127.0.0.1`, `node:` fragment). Two listeners each: `l_plain` (stat_prefix=zk_plain, flags off) + `l_flags` (stat_prefix=zk_flags, per_opcode_request_bytes + per_opcode_decoder_error). One cluster `c_sink`.
  - Driver interface wiring: `BackendCount()=1`, `SubjectListenerName()="l_plain"`, `ReferenceListenerPort()=refLPlainPort`, `BackendKind()=fixture.TCPSink`, `ProbeAdmin` (helpers.HTTPGetReadyRaw shape), `DriveReference`/`DriveSubject` delegating to Multi variants.
  - `MultiListenerDriver`: `SubjectListenerNames()=["l_plain","l_flags"]`, `ReferenceListenerPorts()=[15047,15048]`, `DriveReferenceMulti`/`DriveSubjectMulti` delegating to `driveProxy`.
  - `driveProxy` stub: emits `"0046-placeholder: arms pending Task 16\n"` (deterministic, side-label-free → CompareBytes PASS).
  - Stub `scrapeZKStats` (Task 16 expands the parse loop).
  - Interface assertions: `Driver`, `MultiListenerDriver`, `BackendKindAware`. StatsAsserter deliberately absent (asserter-dispatch memory: no vacuous asserter).
  - Blank var references for all payload helpers + frame builders to suppress `unused` linter until Task 16 wires them from `driveProxy`.

### Port allocation

| Fixture | Port | Notes |
|---------|------|-------|
| 0043-network-rbac | 15043/15044/15045 | l_allow/l_deny/l_shadow |
| 0044-network-rbac-boot-reject | 15046 | l_rbac |
| 0045-sni-cluster | 15045 | l_sni (collision with 0043's l_shadow — both use separate containers so no conflict) |
| 0046-zookeeper-requests | **15047/15048** | l_plain/l_flags (new, no collision) |

### Discovery-run output

```
=== RUN   TestDifferential
=== RUN   TestDifferential/0046-zookeeper-requests
[docker startup logs omitted]
--- PASS: TestDifferential (2.06s)
    --- PASS: TestDifferential/0046-zookeeper-requests (2.06s)
PASS
ok  github.com/esalaine/envoy-go/test/differential  2.152s
```

CompareBytes PASS: both sides emit `"0046-placeholder: arms pending Task 16\n"` → identical bytes. No StatsAsserter registered yet → no counter checks at Task 15 (expected).

### No-regression spot check (0001/0043/0045)

```
--- PASS: TestDifferential/0001-tcp-proxy-rr (2.11s)
--- PASS: TestDifferential/0043-network-rbac (5.91s)
--- PASS: TestDifferential/0045-sni-cluster (1.73s)
PASS
```

All three existing fixtures PASS. No regression introduced.

### Status notes (discovered-but-red)

The fixture is correctly discovered-but-red:
- **Green aspect**: `go vet`, `go build`, `gofmt`, `golangci-lint` all clean. `driveProxy` placeholder produces identical bytes on both sides → CompareBytes PASS. The runner discovers the fixture, boots both sides, and completes cleanly.
- **Red aspect**: No StatsAsserter is registered. The 7 arms are stubs. The actual counter-parity assertions land at Task 16 alongside the real `driveProxy` implementation and `AssertStats`. Task 16 also replaces the blank var references with real call sites.

### Task 16 needs to know

1. Port allocation: l_plain=15047 (ref), l_flags=15048 (ref); subject ports are runner-allocated (subjListenerPort, subjListenerPort+1).
2. Frame builders are all correct against the decoder.go min-length table. `setwatches2Payload` is the tightest (exactly 36 bytes including xid+opcode).
3. The `scrapeZKStats` stub returns an empty map — Task 16 must fill the parse loop (use `_zookeeper_` infix to filter Prometheus lines, strip `envoy_` prefix, handle the absence of a tag-extractor label for zookeeper counters — the `.zookeeper.` arm in `name.go` uses full dot→underscore, no label promotion, so the metric name IS the full stat name with underscores).
4. The StatsAsserter compile-time assertion (`_ fixture.StatsAsserter = (*zkRequestsDriver)(nil)`) must be added to the var block at Task 16 alongside a real `AssertStats` method.
5. The blank var references at the bottom of the file (`_ = getdataPayload`, etc.) should be removed at Task 16 once `driveProxy` calls those helpers.

### Files touched

- `test/differential/fixture/fixture.go`
- `test/differential/runner_test.go`
- `test/fixtures/0046-zookeeper-requests/driver/driver.go` (new)
- `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (this file)

---

## Task 16: 0046 driver part 2 — 7 arms + AssertStats + deliberate-break

**Status: BLOCKED — Task-16 deliverables (driver `driveProxy` 7-arm workload + `AssertStats` cross-side StatsAsserter) are fully and correctly implemented, but the fixture CANNOT go green because of a production-code gap in the mixed read→terminal chain runtime that is OUT OF Task-16's file scope. The fixture is doing its job: it surfaces a genuine cross-side divergence.**

### What was implemented (complete, correct, lint/vet/gofmt clean)

- `driveProxy`: the 7 arms exactly per SPEC §8.1.3 / PLAN arm table — side-independent per-arm verdict lines (`arm <name> sent=<n> verdict=<v>`); arm 1 connect (l_plain); arm 2 multi-opcode (connect+ping+getdata+create+close, paced writes, l_plain); arm 3 digit-suffixed (create2 xid4 + getchildren2 xid5 + setwatches2 xid6/op105, l_plain); arm 4 garbage+survival (2 MiB oversized prefix → 250 ms pause → recovery getdata on the SAME conn, l_plain); arm 5 flag-gated getdata (l_flags); arm 6 exists-at-zero (assertion-only); arm 7 deliberate-break (recorded).
- `AssertStats` (real StatsAsserter, runner invokes ONCE with both admin addrs — runner_test.go:1063-1064): scrapes `/stats/prometheus` from both sides; fixed-value cumulative expectations + cross-side equality (`request_bytes`, `getdata_rq_bytes`) + ABSENT-vs-wrong distinction (name-shape / creation-parity proof, R7).
- `scrapeZKStats` + `parseZKPromBody` + `lookupZKCounter`: real Prometheus parse (`_zookeeper_` infix, flat names, no label promotion per AMEND-A4), dotted→flattened lookup.
- Compile-time `_ fixture.StatsAsserter = (*zkRequestsDriver)(nil)` added; blank-var suppressions removed; `existsPayload`/`drvOpExists`/`drvOpConnect` deleted (unused); doc comments corrected (arm 1 = l_plain only; arm 3 = xid 4/wire op 105).

### The blocking divergence (l_plain multi-frame connections)

Reference Envoy v1.37.2 counts EVERY frame on l_plain; envoy-go counts ONLY THE FIRST frame of each connection:

| counter (zk_plain.zookeeper.*) | reference | envoy-go (subject) |
|---|---|---|
| connect_rq | 2 | 2 |
| ping_rq | 1 | **0** |
| getdata_rq | 2 | **0** |
| create_rq | 1 | **0** |
| close_rq | 1 | **0** |
| create2_rq | 1 | 1 |
| getchildren2_rq | 1 | **0** |
| setwatches2_rq | 1 | **0** |
| decoder_error | 1 | 1 |
| request_bytes | 307 | **132** |

l_flags (arm 5, a SINGLE-frame connection) is PERFECT cross-side parity (`getdata_rq=1`, `request_bytes=25`, `getdata_rq_bytes=25` on both sides) — the divergence is specific to connections whose frames arrive across MULTIPLE socket reads.

### Root cause (production-code gap, out of Task-16 scope)

envoy-go's mixed read→terminal chain (`internal/listener/manager.go` `serveNetworkChain` + `internal/filter/network/chain.go` `handleTerminal`) hands the RAW downstream socket to the terminal (`tcp_proxy`) as soon as the read filter (`zookeeper_proxy`) Continues past itself on the FIRST `OnData`. After `HandleTerminal` the read loop `return`s — the terminal reads directly from the socket and the read filter's `OnData` is NEVER called again. So `zookeeper_proxy` only ever observes the bytes buffered before the first Continue (the first socket read).

Reference Envoy's `FilterManagerImpl::onRead` re-iterates the read-filter chain on EVERY socket read for the connection's lifetime, so `zookeeper_proxy` (a read filter terminating at `tcp_proxy`, also a read filter) sees every frame. The SPEC §4.5 decoder design (chain buffer "accumulates undrained bytes across reads" + a high-water mark) explicitly ASSUMES repeated `OnData` per connection — an assumption the implemented terminal-takeover runtime defeats. There is a `writeChainConn` (write seam, ADR-0221) but NO symmetric read-side seam.

### Proof-of-cause (temporary, ran green, then fully reverted)

To confirm the diagnosis and that the fix is viable + small, a temporary `readChainConn` (symmetric to `writeChainConn`) was added: it re-feeds every terminal-side socket read through the read filters before returning to the terminal. WITH it, the subject matched the reference EXACTLY on all 10 l_plain counters and the fixture went GREEN:

```
=== subj zookeeper stats (with proof readChainConn) ===
  envoy_zk_plain_zookeeper_connect_rq = 2
  envoy_zk_plain_zookeeper_ping_rq = 1
  envoy_zk_plain_zookeeper_getdata_rq = 2
  envoy_zk_plain_zookeeper_create_rq = 1
  envoy_zk_plain_zookeeper_close_rq = 1
  envoy_zk_plain_zookeeper_create2_rq = 1
  envoy_zk_plain_zookeeper_getchildren2_rq = 1
  envoy_zk_plain_zookeeper_setwatches2_rq = 1
  envoy_zk_plain_zookeeper_decoder_error = 1
  envoy_zk_plain_zookeeper_request_bytes = 307
--- PASS: TestDifferential/0046-zookeeper-requests
```

The proof files (`internal/filter/network/readconn_proof.go` + the `chain.go` `replayReadFilters`/`replayBuf`/`handleTerminal` wrap) were FULLY REVERTED — `git diff internal/` is empty. They are NOT part of any commit; the real fix belongs in a chain-runtime task (a revision to Task 3/5 or a new read-side-seam task), with the design decision owned by the controller/architect.

### Deliberate-break (R4) — NOT run

The deliberate-break proof presupposes a green baseline. The fixture is already failing LIVE on the real cross-side divergence above, which itself proves the AssertStats assertions are non-vacuous (the connect_rq=2/ping_rq=0/... mismatches are real, live `t.Errorf` failures). Re-running the break protocol on a red baseline would be meaningless; it is deferred until the read-side seam lands and the fixture is green.

### Disposition / what the controller must decide

The Task-16 driver is complete and correct and is COMMITTED (durable handoff). **The 0046 fixture is RED in the committed tree until the read-side seam lands** — the controller MUST schedule that production work (a revision to Task 3/5 or a new task) BEFORE Task 17's full-suite run / the six-gate, or temporarily skip 0046. The phase's central goal — request-side cross-side observability for `zookeeper_proxy` — requires the seam. Recommended fix: add a `readChainConn` (symmetric to `writeChainConn`) in the chain runtime + `handleTerminal` so a request-inspecting read filter remains engaged after terminal handoff (proven viable + small above); then re-run 0046 (expected green) + the deliberate-break R4 protocol; then author the README. This is production work outside Task 16's declared three-file scope.

### Files touched (this task)

- `test/fixtures/0046-zookeeper-requests/driver/driver.go` (7 arms + AssertStats; COMMITTED — fixture RED pending the read-side seam)
- `docs/.../PROGRESS.md` (this entry)
- (`README.md` DEFERRED until the fixture is green so it documents the as-shipped green result, not a red intermediate)

---

## 28.1a closure — ADR-0045 split invoked (user-approved 2026-06-02)

**Status: 28.1a CLOSED. All gates green on the 28.1a scope. The Task-16 0046
fixture is committed-but-DISABLED pending the 28.1b read-side seam.**

### The gap evidence (Task 16 BLOCKED analysis)

Task 16's 0046 fixture surfaced a genuine SPEC design gap. envoy-go's network
chain runtime exits its read loop **permanently** at terminal handoff
(`internal/listener/manager.go` `serveNetworkChain`:
`TerminalReady()` → `HandleTerminal()` → `return`), so a
`[zookeeper_proxy, tcp_proxy]` chain delivers only the FIRST socket read's bytes
to `zookeeper_proxy`'s `OnData`. Reference Envoy re-iterates its read filters on
every read for the connection's lifetime. The cross-side divergence table is in
the Task-16 entry above (PROGRESS.md "The blocking divergence (l_plain
multi-frame connections)", ~line 1049): e.g. `zk_plain.zookeeper.ping_rq` ref=1
vs subject=0; `getdata_rq` 2 vs 0; `request_bytes` 307 vs 132. l_flags (the
single-frame arm 5) is PERFECT cross-side parity — the divergence is specific to
multi-socket-read connections. A proof-of-concept `readChainConn` (reverted, not
committed) achieved EXACT cross-side parity, proving the gap is fixable with a
small framework addition (the symmetric read-side seam the 28.1 SPEC never
designed — it designed only the WriteFilter seam / `writeChainConn`, ADR-0221).

### The user's decision + the three options presented

Three options were presented to the user (2026-06-02):

1. **Land the read seam now** — extend 28.1 with a new chain-runtime task
   (`readChainConn` + `handleTerminal` wrap) to green 0046 in this sub-phase.
2. **Descope the multi-frame arms** — keep only the single-frame parity arms
   (arm 1 / arm 5) so 0046 greens within the existing runtime.
3. **Invoke the ADR-0045 pre-authorized 28.1a/28.1b split** — land the seam +
   package + unit layer + the 0046 driver code now (28.1a); defer the read seam
   + 0046-green + 0047 + the completion bundle to 28.1b.

**The user chose option 3** — the ADR-0045 pre-authorized 28.1a/28.1b split.

### 28.1a scope statement

**Lands at 28.1a (this merge):**
- PLAN Tasks 1–15 in full: the WriteFilter seam (ADR-0221: `WriteFilter` /
  `WriteFilterCallbacks` interfaces, chain classification restructure, dual
  callback injection, `writeChainConn`, `handleTerminal` wrap), the complete
  `internal/filter/network/zookeeperproxy` package (config parse + the 201-suffix
  eager stats roster + the request decoder + xid maps + filter glue), the 7th
  built-in + bootstrap blank-import, the `.zookeeper.` name.go arm, the 37th
  fuzzer (`FuzzZookeeperRequestDecode`).
- The Task-16 driver work: the `TCPSink` BackendKind + `acceptSinkCounting`
  framework plumbing + the 0046 driver (7 arms + AssertStats) — **correct but
  RED**, and therefore **committed-but-DISABLED** (blank-import commented out in
  `runner_test.go`).

**Defers to 28.1b (next sub-phase, SPEC first):**
- The read-side seam design (the symmetric `readChainConn`).
- 0046 going green (re-enable the blank-import) + the deliberate-break R4 proof.
- The `0047` boot-reject fixture.
- The completion bundle: ADR-0221 / ADR-0222 bodies, BEHAVIOR_CONTRACT stat-table
  rollup (136 → 337), ROADMAP rollup.

### Pre-authorized vs actual split axis

The parent SPEC §3.0 / §11.9 / §15 (D-P1) **pre-authorized** a split axis of:
- **28.1a** = WriteFilter seam + config parse + eager counter roster + `0047`
  boot-reject fixture
- **28.1b** = request decoder + xid maps + `0046` cross-side fixture + fuzzer

The **actual** split invoked is:
- **28.1a** = WriteFilter seam + the FULL `zookeeperproxy` package (config +
  stats roster + **request decoder + xid maps** + glue) + builtin/bootstrap/
  name.go + **the 37th fuzzer** + the `0046` driver code (DISABLED)
- **28.1b** = the **read-side seam** + `0046` green + `0047` boot-reject + the
  completion bundle (ADR bodies / BEHAVIOR_CONTRACT / ROADMAP)

**These DIFFER.** The pre-authorized axis split on the *decoder/fixture* boundary
(seam+roster vs decoder+fixtures+fuzzer); the actual axis splits on the
*read-seam / fixtures-and-completion* boundary (the whole package + unit layer +
0046 driver land at 28.1a; the read seam + both fixtures-green + completion bundle
land at 28.1b). The realized 28.1a is a LARGER landing than the pre-authorized
28.1a (it carries the decoder + fuzzer + 0046 driver, which the pre-authorized
axis had placed in 28.1b), because the work was already implemented and correct
by Task 16 and the only blocker is the read seam. **The user's explicit approval
(2026-06-02) is the governing authority for this deviation from the pre-authorized
axis** (the ADR-0045 split is pre-authorized; the precise axis was a PLAN/IMPL-time
judgement settled by the user's in-session decision).

### Full gate outputs (Step 2 — quoted honestly)

**Gate 1 — `go build ./...`:** clean. `BUILD_EXIT=0`.

**Gate 2 — `go vet ./...`:** clean. `VET_EXIT=0`.

**Gate 3 — `golangci-lint run` (whole repo):** clean. `LINT_EXIT=0`. (One
goimports finding appeared on the first run — the commented-out 0046 blank-import
left an import-group gap; resolved with `gofmt -w` + `goimports -w`, which inserted
the canonical blank line separating the trailing `helpers` import. Re-run clean.
NOTE: `acceptSinkCounting` was NOT flagged unused — the `case fixture.TCPSink:`
switch arm in `runner_test.go` keeps the function source-referenced even though no
enabled driver returns `TCPSink` at runtime, so no `//nolint` was needed; the
TCPSink BackendKind + `acceptSinkCounting` stay as framework plumbing 28.1b uses.)

**Gate 4 — `go test ./... -race -short`:** all green. `TEST_EXIT=0`. 79 `ok`
packages, 0 FAIL, 0 panic. The known wasm-dispatch flake did NOT manifest this run
(no retry needed).

**Gate 5 — FULL differential suite (`go test ./test/differential/ -run
TestDifferential -v`):** with 0046 DISABLED, the suite ran the 47 active fixtures.
First full run: **44 PASS, 1 SKIP, 3 FAIL** (`DIFF_EXIT=1`). The SKIP is
`0046-zookeeper-requests` (no driver registered → correctly skipped). The 3 FAILs
were ALL the documented `freeTCPPort` TOCTOU port-collision flake (NOT 28.1a
regressions — none touch the network-filter seam or zookeeper code):
- `0015-http-buffer`: `bind: 0.0.0.0:41501: address already in use` (backend port)
- `0018-http-rbac`: `l_test_b bind 0.0.0.0:39252: address already in use`
- `0036-http-wasm-body-and-advanced`: `l_test_b bind 0.0.0.0:42884: address already in use`

All three are the multi-listener second-port (`l_test_b`/`l_test_d`) bind race in
the window between `freeTCPPort`'s free-check and the listener's bind. Re-run in
isolation: `0015` PASS, `0018` PASS, `0036` hit the same flake once more (different
port `l_test_d:41776`) then PASSed on its own isolated re-run (34.32s).
**Net: all 47 active fixtures green; 0046 correctly skipped (disabled).**

**Gate 6a — h2spec conformance (re-run LIVE):** `53 total tests, 0 failures`.
```
$ go test ./test/conformance/h2spec/ -run TestH2Spec -v
    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.702s
```
**PASS — 53/53.**

**Gate 6b — proxy-wasm conformance (the "10/10" suite — re-run LIVE):** all
families PASS.
```
$ go test ./test/conformance/proxy-wasm/ -run TestProxyWasmConformance -v
--- PASS: TestProxyWasmConformance (0.26s)
    ... (exports / security / runtime / wasm_vm / bytecode_util / logging /
         stop_iteration / shared_data / pairs_util / endianness all PASS)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/proxy-wasm	0.258s
```
**PASS — all families (10/10).** 28.1 touches no HTTP/h2/proxy-wasm path (the
seam is network-chain only; HCM has no write filters), so 6a/6b are
asserted-unaffected — re-run LIVE since the harness was available, both green.

### Counts at 28.1a

- **Fixtures:** 47 ACTIVE (tail `0045-sni-cluster`) **+ 1 committed-but-DISABLED**
  (`0046-zookeeper-requests` — code present, blank-import commented out; 28.1b
  re-enables once the read seam greens it).
- **Fuzzers:** 37 (`FuzzZookeeperRequestDecode` is the +1 over the 36 baseline
  STATE.md records; a raw `grep '^func Fuzz'` across the repo yields 38 functions
  — the canonical 36-baseline roster excludes one helper-adjacent fuzzer; the
  canonical-roster count is 37 at 28.1a).
- **Stats:** the 201 zookeeper counters EXIST in code (the `stats.go` eager roster
  + dynamic auth counters, Task 8) and are LIVE-asserted by the unit layer. The
  BEHAVIOR_CONTRACT stat-table count **STAYS at 136** at 28.1a — it is NOT rolled
  to 337 here. The roll to 337 is deferred to 28.1b, gated on the 0046
  differential cross-side proof (the read seam) + the completion bundle. This is
  deliberate: the BEHAVIOR_CONTRACT records cross-side-PROVEN surface, and the
  zookeeper counters are not yet cross-side-proven (0046 is RED/disabled at
  28.1a).

### Files touched (28.1a closure)

- `test/differential/runner_test.go` — commented out the `0046` driver
  blank-import (with the split-rationale comment); `gofmt`/`goimports` normalized
  the import group.
- `test/fixtures/0046-zookeeper-requests/driver/driver.go` — prepended the
  "DISABLED at 28.1a — re-enabled at 28.1b" banner to the package doc comment.
- `docs/.../PROGRESS.md` — this closure entry.
