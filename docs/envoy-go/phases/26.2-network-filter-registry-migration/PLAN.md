# Phase 26.2 Implementation Plan — `tcp_proxy` + HCM migration onto `internal/filter/network/` + hardcoded-registry retirement + dispatch unification

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended, per the `feedback_execution_style` project memory) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task is TDD-first (write-the-failing-test → run-it-fails → minimal-impl → run-it-passes → commit) per superpowers:test-driven-development. Subagents commit LOCAL-ONLY (they do NOT push — `feedback_subagents_no_push`); the controller pushes at stage-close.

**Goal:** Migrate the two terminal network filters — `tcp_proxy` (`internal/filter/tcpproxy/`) + HCM (`internal/filter/hcm/`) — onto the `internal/filter/network/` framework + the freeze-after-boot `*network.Registry` that 26.1 landed, RETIRE the hardcoded `internal/listener/manager.go` terminal-filter registry (`filterHandler`/`filterConstructor`/`filterRegistry`/`buildTerminalFilter`), and UNIFY the 26.1 dual-dispatch into one registry-driven per-connection path — at byte-exact parity (R3 back-compat is paramount).

**Architecture:** A NEW `network.TerminalFilter interface { Handle(ctx, downstream net.Conn) }` — byte-identical to the retired `manager.go` `filterHandler` — sits alongside the 26.1 `ReadFilter`. `*tcpproxy.Filter` + `*hcm.Filter` satisfy it with ZERO `Handle`-method changes (their connection-takeover loops are UNTOUCHED → byte-exact parity is intrinsic). A sealed `NetworkFilter` marker (`ReadFilter | TerminalFilter`) generalizes `FilterInstanceFactory`. `FactoryCtx` gains per-chain primitives (HasTLS/AllowH2C/ListenerPrincipal/NodeServiceCluster); the heavy boot singletons are closure-captured in `tcpproxy.NewNetworkFactory` / `hcm.NewNetworkFactory` registration adapters so `internal/filter/network` stays import-light. The `*network.Registry` becomes the SOLE network-filter registry (echo/direct_response/tcp_proxy/HCM); `buildNetworkChainFactory` becomes the sole chain builder (classify + validate `[read*, terminal?]`); `serveConnection` step-7 collapses to ONE `serveNetworkChain` branch. The mixed read→terminal chain restriction LIFTS (buffered-prefix `prefixConn` handover shaped + unit-tested; first production consumer = `rbac_network` 26.3). This is a MIGRATION/refactor: NO operator-visible feature; stat surface stays 132; fixtures stay 44; fuzzers stay 35.

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); golangci-lint 1.64.8 (ADR-0009). ZERO new third-party `go.mod` dependencies. REUSES the 26.1 `internal/filter/network/` framework + `internal/filter/tcpproxy/` + `internal/filter/hcm/` (no package move).

**Module path:** `github.com/esalaine/envoy-go`.

**Source of truth:** the phase-26.2 SPEC (`docs/envoy-go/phases/26.2-network-filter-registry-migration/SPEC.md`), especially §3.1 (the terminal-filter seam + rejected `OnData`-rewrite alternative), §3.2 (API extensions: `TerminalFilter`/sealed `NetworkFilter` marker/extended `FactoryCtx`/unified chain runner + buffered-prefix handover), §3.3 (`manager.go` retirement + unification surface), §3.4 (registration seam + boot-wiring), §4 (tcp_proxy/HCM adapters), §6 (PARSE-REJECT — unified unknown-type + new chain-shape rejects + mixed-chain LIFT), §10 (the ~12-task spine this plan decomposes), §11.1 (D-S1 baselines + manager.go line anchors = Task-1 gate), §12 (D-26.2-1..7), §13 (R3/R-T/R-M/R-U/R-S/R-A), §15 (acceptance).

---

## ADR-0045 split-gate check (PERFORMED at PLAN time)

Per SKILL_ROUTING state-2 GATE: split if PLAN > ~25 tasks OR > ~1500 estimated LoC. This plan is **12 tasks**; SPEC §10/§11.8-D8 estimates **~500–900 net-new LoC** (`tcp_proxy` adapt ~40-80 + HCM adapt ~60-120 + `manager.go` retirement/unification ~200-350 + terminal seam + `prefixConn` ~120-200 + test churn ~100-200), with ~0 moved LoC (the HCM-bridge closure MOVES from `manager.go:112-132` into the `hcm` adapter — mechanical). Both are comfortably within the gate (~25 tasks / ~1500 LoC). **NO split.** Proceed as a single 26.2 IMPL.

## PLAN-time D-question resolutions (SPEC §12)

The SPEC marks D-26.2-1/-2/-3/-5 for PLAN-time resolution; resolved here so IMPL has no open design choices. D-26.2-4/-6/-7 remain IMPL-time empirical pins (scoped into Tasks 7/8/5-6 below).

- **D-26.2-1 (sealed vs open `NetworkFilter` marker) → SEALED marker via an exported embeddable `network.Marker`.** A truly sealed interface (unexported `isNetworkFilter()` method) cannot be satisfied from OUTSIDE the `network` package — but Go's standard sealing idiom is an **exported empty struct carrying the unexported method**: out-of-package filters embed `network.Marker` and gain the promoted `isNetworkFilter()`, satisfying the sealed `NetworkFilter`. This is type-safe + exhaustive (the dispatch type-switch is `ReadFilter` / `TerminalFilter` / defensive-default). **Blast radius (Task 2; verified via `git grep`):** the ONLY `network.ReadFilter` implementers are `internal/filter/network/echo/echo.go` (`echoFilter`), `internal/filter/network/directresponse/directresponse.go` (`filter`), and the in-package test fakes in `internal/filter/network/chain_test.go` + `types_test.go` — exactly 4 files embed `network.Marker` in Task 2. `tcp_proxy`/HCM embed it in Tasks 5/6.
- **D-26.2-2 (FactoryCtx field set) → `BaseDir`(existing) + `HasTLS` + `AllowH2C` + `ListenerPrincipal` + `NodeServiceCluster`.** Confirmed against the HCM constructor: `hcm.NewFilterWithCtxAndSinksAndRegistry` consumes `hcm.ListenerCtx{HasTLS, AllowH2C, ListenerPrincipal, HTTPClient, NodeServiceCluster}` + `cm`/`registry`/`accessLogSinks`/`httpRegistry`/`dm`. `HTTPClient` is the shared `*httpclient.Client` singleton → closure-captured in the adapter (NOT a FactoryCtx field). `tcpproxy.NewFilter(tc, cm, dm)` consumes only closure-captured singletons → ignores all FactoryCtx fields. The four per-chain primitives go in FactoryCtx; the heavy singletons (cm/dm/stats/sinks/httpReg/httpClient) are closure-captured (§3.4).
- **D-26.2-3 (buffered-prefix handover: land at 26.2 vs defer) → LAND at 26.2, unit-tested.** A small `prefixConn net.Conn` adapter (replays undrained buffered prefix bytes on the first `Read`(s), then delegates to the live conn; all other methods promoted) lands in `chain.go` at 26.2 and is unit-tested via a synthetic always-`Continue` read filter (drains nothing) → a recording terminal that asserts it receives the buffered prefix THEN the live socket bytes (R-M). This makes the SPEC's "mixed chains become expressible" claim honest + proven; first PRODUCTION consumer is `rbac_network` (26.3). Mirrors how 26.1 shaped `DynamicMetadata`/`DownstreamPrincipals` ahead of their 26.3 consumer.
- **D-26.2-5 (registration seam location + test-caller wiring) → a new `internal/filter/network/builtins` package exposing `RegisterBuiltins(reg *network.Registry, deps Deps)`.** Because `netReg` becomes required by every manager constructor + test caller, a single shared helper registers all four built-ins (echo/direct_response/tcp_proxy/HCM) with their boot singletons captured, avoiding four duplicated `Register` calls. Placement is import-cycle-constrained: the helper imports `echo`+`directresponse`+`tcpproxy`+`hcm`+`network`, so it CANNOT live in `network` (those import `network`) nor in `listener` (which `builtins` consumers' tests import). `cmd/envoy-go/` is a `main` package — NOT importable by the internal test callers (`internal/admin`, `internal/listener`). Therefore a NEW importable package `internal/filter/network/builtins` is the placement (imported by `main.go` + `internal/listener/manager.go`'s thinner ctors + the admin/manager_test/main_test callers). No cycle forms: none of `network`/`echo`/`directresponse`/`tcpproxy`/`hcm`/`listener` import `builtins`. Verified at IMPL by `go build ./...` (D-26.2-7).
- **D-26.2-4 (`listenerCtx` full-retire vs thin-retain) → ANTICIPATED full-retire; CONFIRMED at IMPL Task 7.** All five `listenerCtx` fields migrate cleanly: `httpClient` → the HCM adapter closure; `hasTLS`/`allowH2C`/`listenerPrincipal`/`nodeServiceCluster` → `FactoryCtx`. `extractListenerPrincipal` (`manager.go:153`) is RETAINED (it populates `FactoryCtx.ListenerPrincipal` in `buildNetworkChainFactory`). The `listenerCtx` struct is DELETED with `buildTerminalFilter` in Task 7/10. If IMPL surfaces a residual manager-side use, retain as a thin per-chain carrier (documented in PROGRESS.md).
- **D-26.2-6 (PARSE-REJECT byte-stable wording) → IMPL Task 8.** Preserve the existing `unknown filter type_url %q` wording (`buildTerminalFilter:628`) byte-for-byte in the unified path (R-S); finalize the NEW `terminal-not-last`/`multiple-terminals` arm wording + the `TestParseRejectConstants_ByteStable`-style table.
- **D-26.2-7 (import-cycle audit) → IMPL Tasks 5/6/9.** `tcpproxy`/`hcm` import `network` (one-directional); `network` imports neither + no heavy packages (FactoryCtx is primitives-only); `builtins` imports all filters + `network`. Verified by `go build ./...`.

## Sealed-marker mechanics (Task 2 design pin — D-26.2-1)

```go
// internal/filter/network/terminal.go (NEW)
package network

import (
	"context"
	"net"
)

// NetworkFilter is the sealed common interface every chain filter satisfies —
// either a ReadFilter (OnData inspection model) or a TerminalFilter
// (connection-takeover model). The chain builder classifies each; the dispatch
// type-switches. Sealed via the unexported isNetworkFilter() method, granted
// ONLY by embedding the exported Marker — so a value cannot satisfy
// NetworkFilter without being a deliberate network filter.
type NetworkFilter interface {
	isNetworkFilter()
}

// Marker is the exported embeddable that grants the sealed isNetworkFilter()
// method. Out-of-package filters (echo, direct_response, tcp_proxy, HCM) embed
// network.Marker to satisfy ReadFilter / TerminalFilter (which embed
// NetworkFilter). Zero-size; no state.
type Marker struct{}

func (Marker) isNetworkFilter() {}

// TerminalFilter is a network filter that takes over the downstream connection
// at the END of the chain (tcp_proxy: L4 bidirectional pump to an upstream
// cluster member; HCM: the HTTP/1 driver or HTTP/2 codec). Unlike a ReadFilter
// (which inspects buffered bytes via OnData and writes through
// ReadFilterCallbacks.Connection), a TerminalFilter owns the raw net.Conn and
// runs a blocking serve loop to connection close. It mirrors upstream's
// terminal read filters (tcp_proxy/HCM), which return StopIteration and drive
// the connection directly. Handle's signature is byte-identical to the
// phase-02 manager.go filterHandler interface this seam retires, so the
// existing *tcpproxy.Filter + *hcm.Filter satisfy it with no method change.
//
//nolint:revive // ADR-0215 reserves the network.TerminalFilter name.
type TerminalFilter interface {
	NetworkFilter
	// Handle takes ownership of the downstream connection and runs to
	// completion. It owns the conn-close lifecycle (the unified dispatch does
	// NOT close the conn for a terminal filter; Handle's own defer conn.Close()
	// runs — byte-identical to the retired terminal path).
	Handle(ctx context.Context, downstream net.Conn)
}
```

`ReadFilter` (in `types.go`) gains the embedded marker: `type ReadFilter interface { NetworkFilter; OnNewConnection() Status; … }`.

## Terminal-handoff dispatch design (Task 4/8 design pin — §3.1/§3.2)

`ChainRuntime` classifies its `[]NetworkFilter` once at construction into a read-filter prefix (`[]ReadFilter`) + an optional trailing `TerminalFilter`. The 26.1 pure-read behavior is unchanged. New surface:

- `TerminalReady() bool` — true when control has reached the terminal: for a **pure-terminal** chain (0 read filters + terminal) it is true immediately; for a **mixed** chain it becomes true once every read filter has `Continue`d past (resumeIdx ≥ len(readFilters), not `connHalted`). For a **pure-read** chain (no terminal) it is always false.
- `HandleTerminal(ctx)` — builds a `prefixConn` wrapping the connection read buffer's undrained bytes + the live conn, then calls `terminal.Handle(ctx, prefixConn)`. For a pure-terminal chain the prefix is empty so `prefixConn` is a transparent passthrough → byte-identical to today's `selected.filter.Handle(ctx, dispatchConn)`.

`serveNetworkChain` (Task 8) drives all three chain kinds with the same code (pure-read read loop unchanged from 26.1; terminal handoff on `TerminalReady`; pure-terminal immediate handoff). At 26.2 NO shippable read filter `Continue`s to a terminal (echo halts; direct_response closes), so the mixed handoff is exercised by Task 4 unit tests only; production hits only pure-read + pure-terminal.

---

## File Structure

### Created — framework `internal/filter/network/`

| File | Responsibility |
|---|---|
| `terminal.go` | sealed `NetworkFilter` + exported `Marker` + `TerminalFilter` interface (D-26.2-1) |
| `prefixconn.go` | `prefixConn` net.Conn wrapper (buffered-prefix handover; D-26.2-3) |

Tests: `terminal_test.go`, `prefixconn_test.go`; extend `chain_test.go` (classification + terminal handoff + mixed-prefix R-M).

### Created — registration seam

| Package | Files | Responsibility |
|---|---|---|
| `internal/filter/network/builtins/` | `builtins.go`, `builtins_test.go` | `RegisterBuiltins(reg, Deps)` registers all four built-ins (D-26.2-5) |

### Modified — framework + filters

| File | Change |
|---|---|
| `internal/filter/network/types.go` | `ReadFilter` embeds `NetworkFilter`; `FilterInstanceFactory func() ReadFilter` → `func() NetworkFilter` (Task 4) |
| `internal/filter/network/chain.go` | classify `[]NetworkFilter` → read-prefix + terminal; `NewChainRuntime([]NetworkFilter,…)`; `TerminalReady()` / `HandleTerminal(ctx)` (Task 4) |
| `internal/filter/network/echo/echo.go` | `echoFilter` embeds `network.Marker`; `New` closure returns `network.NetworkFilter` |
| `internal/filter/network/directresponse/directresponse.go` | `filter` embeds `network.Marker`; `New` closure returns `network.NetworkFilter` |
| `internal/filter/network/{types_test.go,chain_test.go}` | test fakes embed `network.Marker` (Task 2); the existing `newChainRuntime([]ReadFilter{…})` call sites STAY `[]ReadFilter` — only the NEW Task-4 tests calling the exported `NewChainRuntime` use `[]NetworkFilter` |
| `internal/filter/network/types.go` (FactoryCtx) | add `HasTLS`/`AllowH2C`/`ListenerPrincipal`/`NodeServiceCluster` (Task 3) |
| `internal/filter/tcpproxy/filter.go` | `Filter` embeds `network.Marker`; `var _ network.TerminalFilter`; NEW `NewNetworkFactory(cm, dm)` adapter (Task 5) |
| `internal/filter/hcm/{filter.go,config.go}` | `Filter` embeds `network.Marker`; `var _ network.TerminalFilter`; NEW `NewNetworkFactory(cm, registry, sinks, httpReg, dm, httpClient)` adapter (Task 6) |

### Modified — `manager.go` retirement + unification

| File | Change |
|---|---|
| `internal/listener/manager.go` | DELETE `filterHandler`@46 / `listenerCtx`@61 / `filterConstructor`@97 / `filterRegistry`@104 / `buildTerminalFilter`@612; collapse `chainInfo`@184 (drop `filter`; `netChainFactory func() []network.NetworkFilter`); `buildNetChainFactory`@654 → unified `buildNetworkChainFactory` (classify + `[read*, terminal?]` shape validation + unified unknown-type reject); `serveConnection` step-7@1104-1109 → one `serveNetworkChain`; `serveReadFilterChain`@1126 → `serveNetworkChain`; `netReg` required |
| `internal/listener/manager_test.go` | `netReg` built via `builtins.RegisterBuiltins`; build-path + dispatch tests |
| `internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go`, `cmd/envoy-go/main_test.go` | ctor callers obtain a populated `netReg` via `builtins.RegisterBuiltins` |
| `cmd/envoy-go/main.go` | replace the echo/direct_response Register block with `builtins.RegisterBuiltins(netReg, builtins.Deps{…})`; move `httpClient :=` above the netReg block (Task 11) |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | 26.2 bundle (Task 12) |
| `docs/envoy-go/DECISIONS.md` | ADR-0215 §Decision/§Consequences bodies (tail STAYS ADR-0215; Task 12) |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | phase-done advance (Task 12) |

---

## Task 1: First-action baselines + proto/anchor re-confirm (HARD GATE)

The master tip may have advanced since the SPEC commit (`7c19957`; tip at PLAN authoring `4adf7eb`). Re-pin every baseline + every `manager.go` line anchor BEFORE asserting any delta or editing (SPEC §11.1 D-S1; R-S). No production code in this task.

**Files:** none (verification only). Record results in `docs/envoy-go/phases/26.2-network-filter-registry-migration/PROGRESS.md` (create it).

- [ ] **Step 1: Re-grep the baselines — git-tracked enumeration (deterministic).**

Use `git ls-files`, NOT `find .`: the repo root has dozens of nested worktrees under `.worktrees/` + `.claude/worktrees/` whose `fuzz_test.go` files inflate a naive `find`/`grep -r` count (the 26.1 PLAN review flagged exactly this artifact).

```bash
cd "$(git rev-parse --show-toplevel)"
echo "fuzzers:";      git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l   # expect 35
echo "fixture dirs:"; ls test/fixtures/ | grep -E '^[0-9]' | wc -l                          # expect 44
echo "fixture tail:"; ls test/fixtures/ | grep -E '^[0-9]' | sort | tail -1                 # expect 0042-...
echo "ADR tail:";     grep -nE '^#+ +ADR-0[0-9]{3}' docs/envoy-go/DECISIONS.md | tail -1   # expect ADR-0215 (the 26.2 SPEC drafted the §Context). Grep HEADINGS, not prose — a naive 'grep -oE ADR-0[0-9]{3}|sort -u|tail' matches PLANNED forward-refs (0216/0217/0218) in the provisional-span text and over-reports.
```

Expected: `35`, `44`, `0042-…`, `ADR-0215`. **If any differ**, STOP and reconcile (the SPEC's "+0" deltas + "tail ADR-0215" assume these) — note the drift in PROGRESS.md before proceeding.

- [ ] **Step 2: Re-confirm the stat surface = 132.** Use the project's stat-roster grep/golden (the same one phase-25.3/26.1 used). Expected: `132`. 26.2 adds 0.

- [ ] **Step 3: Re-confirm the `tcp_proxy` + HCM type URLs vs go-control-plane v1.32.4 (R-S; §5).**

```bash
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3.TcpProxy | head -3
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3.HttpConnectionManager | head -3
```

Confirm `tcpproxy.TypeURL` = `type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy` and `hcm.TypeURL` = `…http_connection_manager.v3.HttpConnectionManager` are stable (the registrations in Task 11 depend on them). Note: unlike echo (whose 26.1 IMPL discovered an `extensions.` segment surprise — `network_filter_typeurl_extensions` memory), tcp_proxy + HCM type URLs are long-established (phase 02/04) + already carry `extensions.` — verify, don't assume.

- [ ] **Step 4: Re-pin the `manager.go` retirement line anchors against the IMPL-session tip** (the SPEC §11.1 pins were against an earlier tip; at PLAN authoring tip `4adf7eb` they are): `filterHandler`@46; `listenerCtx`@61; `filterConstructor`@97; `filterRegistry`@104 (tcp_proxy closure @105, HCM bridge @112-132); `extractListenerPrincipal`@153; `chainInfo`@184 (`filter`@187, `netChainFactory`@195); `NewManager`@271; `NewManagerWithBaseDir`@283; `NewManagerWithBaseDirAndAllowH2C`@319; per-chain build (`buildNetChainFactory` call@463, `buildTerminalFilter` call@474, `chainInfo{}`@483); default_filter_chain build (`buildNetChainFactory`@539, `buildTerminalFilter`@545, `chainInfo{}`@551); `buildTerminalFilter`@612; `buildNetChainFactory`@654; `serveConnection`@1045; step-7 dual-branch@1104-1109; `serveReadFilterChain`@1126. Record the re-pinned numbers in PROGRESS.md (subsequent tasks cite line ranges; drift will move them).

- [ ] **Step 5: Confirm the ctor-caller blast radius (Task 10 re-wiring).**

```bash
git grep -l 'NewManagerWithBaseDirAndAllowH2C' -- '*.go'
```

Expected (callers): `cmd/envoy-go/main.go`, `cmd/envoy-go/main_test.go`, `internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go`, `internal/listener/manager_test.go`. NOTE: `internal/drain/doc.go` also matches — it is a doc-COMMENT reference, NOT a caller; do NOT chase it.

- [ ] **Step 6: Confirm six gates green at the tip** (baseline must be clean before new code):

```bash
go build ./... && go vet ./... && golangci-lint run && go test -race -short ./...
```

Expected: all pass.

- [ ] **Step 7: Commit** (PROGRESS.md only):

```bash
git add docs/envoy-go/phases/26.2-network-filter-registry-migration/PROGRESS.md
git commit -m "phase 26.2 Task 1: re-pin D-S1 baselines (fuzzers 35, fixtures 44/tail 0042, stats 132, ADR-0215) + manager.go anchors"
```

---

## Task 2: `network.TerminalFilter` + sealed `NetworkFilter` marker (D-26.2-1)

Purely ADDITIVE: add the new interfaces + marker, embed `NetworkFilter` into `ReadFilter`, and add `network.Marker` to the 4 existing `ReadFilter` implementers. `FilterInstanceFactory` stays `func() ReadFilter` (generalized in Task 4) — so the manager + chain runner are untouched here.

**Files:**
- Create: `internal/filter/network/terminal.go`, `internal/filter/network/terminal_test.go`
- Modify: `internal/filter/network/types.go` (`ReadFilter` embeds `NetworkFilter`)
- Modify: `internal/filter/network/echo/echo.go`, `internal/filter/network/directresponse/directresponse.go` (embed `Marker`)
- Modify: `internal/filter/network/types_test.go`, `internal/filter/network/chain_test.go` (test fakes embed `Marker`)

- [ ] **Step 1: Write the failing test** (`terminal_test.go`): the seal + the terminal interface shape.

```go
package network

import (
	"context"
	"net"
	"testing"
)

// fakeTerminal satisfies TerminalFilter with zero extra methods beyond Handle.
type fakeTerminal struct {
	Marker
	handled bool
}

func (f *fakeTerminal) Handle(_ context.Context, _ net.Conn) { f.handled = true }

func TestTerminalFilterSatisfied(t *testing.T) {
	var _ TerminalFilter = (*fakeTerminal)(nil)
	var _ NetworkFilter = (*fakeTerminal)(nil)
}

// A ReadFilter is also a NetworkFilter (embeds the marker via Marker).
func TestReadFilterIsNetworkFilter(t *testing.T) {
	var _ NetworkFilter = noopFilter{} // noopFilter (types_test.go) embeds Marker
}

// Dispatch type-switch is exhaustive over the two kinds.
func TestNetworkFilterClassify(t *testing.T) {
	classify := func(nf NetworkFilter) string {
		switch nf.(type) {
		case TerminalFilter:
			return "terminal"
		case ReadFilter:
			return "read"
		default:
			return "neither"
		}
	}
	if got := classify(&fakeTerminal{}); got != "terminal" {
		t.Errorf("fakeTerminal classified %q", got)
	}
	if got := classify(noopFilter{}); got != "read" {
		t.Errorf("noopFilter classified %q", got)
	}
}
```

- [ ] **Step 2: Run test → fails** (`undefined: TerminalFilter` / `Marker`; `noopFilter` lacks the marker).
Run: `go test ./internal/filter/network/ -run 'TestTerminal|TestReadFilterIsNetwork|TestNetworkFilterClassify' -v`

- [ ] **Step 3: Minimal implementation.**
  - Create `terminal.go` with the block from "Sealed-marker mechanics" above (`NetworkFilter`, `Marker`, `TerminalFilter`).
  - In `types.go`, embed the marker into `ReadFilter`:
    ```go
    type ReadFilter interface {
        NetworkFilter
        OnNewConnection() Status
        OnData(buf *Buffer, endStream bool) Status
        SetReadFilterCallbacks(cb ReadFilterCallbacks)
        OnDestroy()
    }
    ```
  - Embed `network.Marker` into the out-of-package implementers:
    - `echo/echo.go`: `type echoFilter struct{ network.Marker; cb network.ReadFilterCallbacks }`
    - `directresponse/directresponse.go`: `type filter struct{ network.Marker; cfg *compiledConfig; cb network.ReadFilterCallbacks }`
  - Embed `Marker` into the in-package test fakes:
    - `types_test.go`: `type noopFilter struct{ Marker }`
    - `chain_test.go`: add `Marker` to ALL SIX fake `ReadFilter`s — `filterA`, `filterB`, `stopConnFilter`, `lazyConnFilter`, `echoStyleFilter`, `destroyFilter` (verified set; each `type fooFilter struct{ … }` gains an embedded `Marker` field). Re-confirm with `git grep -nE 'func \(.*\) OnNewConnection' internal/filter/network/chain_test.go` in case the suite grew since PLAN authoring.

- [ ] **Step 4: Run tests → pass; the whole framework + filter packages compile.**
Run: `go test ./internal/filter/network/... -v && go vet ./internal/filter/network/...`
Expected: PASS (echo/direct_response tests still green — adding an embedded zero-size marker changes no behavior).

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/terminal.go internal/filter/network/terminal_test.go internal/filter/network/types.go internal/filter/network/types_test.go internal/filter/network/chain_test.go internal/filter/network/echo/ internal/filter/network/directresponse/
git commit -m "phase 26.2 Task 2: network.TerminalFilter + sealed NetworkFilter marker (Marker embeddable) [SPEC 3.2; D-26.2-1]"
```

---

## Task 3: Extend `network.FactoryCtx` with per-chain build fields (D-26.2-2)

Purely ADDITIVE: adding fields to a struct breaks no existing `FactoryCtx{BaseDir: …}` literal. echo/direct_response ignore the new fields; the HCM adapter (Task 6) consumes them.

**Files:**
- Modify: `internal/filter/network/types.go` (extend `FactoryCtx`)
- Test: `internal/filter/network/types_test.go`

- [ ] **Step 1: Write the failing test.**

```go
func TestFactoryCtxPerChainFields(t *testing.T) {
	ctx := FactoryCtx{
		BaseDir:            "/cfg",
		HasTLS:             true,
		AllowH2C:           true,
		ListenerPrincipal:  "spiffe://x",
		NodeServiceCluster: "svc-a",
	}
	if !ctx.HasTLS || !ctx.AllowH2C || ctx.ListenerPrincipal != "spiffe://x" || ctx.NodeServiceCluster != "svc-a" || ctx.BaseDir != "/cfg" {
		t.Fatalf("FactoryCtx field round-trip failed: %+v", ctx)
	}
}
```

- [ ] **Step 2: Run test → fails** (`unknown field HasTLS`).
Run: `go test ./internal/filter/network/ -run TestFactoryCtxPerChainFields -v`

- [ ] **Step 3: Minimal implementation** (extend the struct; keep doc-comments faithful to §3.2):

```go
// FactoryCtx carries the PER-CHAIN build context a NetworkFilterFactory needs.
// Primitives + BaseDir only — the heavy boot singletons (cluster manager, stats
// registry, access-log sinks, HTTP-filter registry, drain manager, http client)
// are captured in the registration closures (internal/filter/network/builtins,
// §3.4), keeping this package free of cluster/stats/hcm imports.
type FactoryCtx struct {
	// BaseDir is the bootstrap config directory (direct_response DataSource
	// Filename resolution relative to the config file; D-P26.1-2). echo ignores it.
	BaseDir string
	// Per-chain terminal-filter build context (26.2; consumed by the HCM
	// adapter — mirrors the retired manager.go listenerCtx). echo/direct_response
	// ignore these; tcp_proxy ignores all but is handed them uniformly.
	HasTLS             bool   // chain has a *stdtls.Config (hcm.ListenerCtx.HasTLS)
	AllowH2C           bool   // --allow-h2c (hcm.ListenerCtx.AllowH2C)
	ListenerPrincipal  string // per-chain leaf-cert principal (hcm.ListenerCtx.ListenerPrincipal)
	NodeServiceCluster string // bootstrap node.cluster (hcm.ListenerCtx.NodeServiceCluster)
}
```

- [ ] **Step 4: Run test → passes; framework + filters compile.**
Run: `go test ./internal/filter/network/... && go build ./...`
Expected: PASS (manager.go still passes `network.FactoryCtx{BaseDir: baseDir}` — extra fields default to zero).

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/types.go internal/filter/network/types_test.go
git commit -m "phase 26.2 Task 3: extend network.FactoryCtx (HasTLS/AllowH2C/ListenerPrincipal/NodeServiceCluster) [SPEC 3.2; D-26.2-2]"
```

---

## Task 4: Generalize the chain to `[]NetworkFilter` + terminal handoff + `prefixConn` (D-26.2-3; R-M)

The load-bearing framework task. Generalize `FilterInstanceFactory` to return `NetworkFilter`; teach `ChainRuntime` to classify `[]NetworkFilter` into a read-prefix + optional terminal; add `TerminalReady()`/`HandleTerminal(ctx)` + the buffered-prefix `prefixConn`. The mechanical `[]ReadFilter`→`[]NetworkFilter` ripple reaches `echo`/`directresponse` (closure return type) + `manager.go` (`buildNetChainFactory` closure + `chainInfo.netChainFactory` + `serveReadFilterChain`). The 26.1 PURE-READ behavior is unchanged; production chains stay pure-read here (terminals are not registered in `netReg` until Task 11), so the terminal handoff is exercised by UNIT tests only.

**Files:**
- Create: `internal/filter/network/prefixconn.go`, `internal/filter/network/prefixconn_test.go`
- Modify: `internal/filter/network/types.go` (`FilterInstanceFactory func() NetworkFilter`)
- Modify: `internal/filter/network/chain.go` (classification + `TerminalReady`/`HandleTerminal`; `NewChainRuntime([]NetworkFilter,…)`)
- Modify: `internal/filter/network/chain_test.go` (NEW R-M handoff tests calling the exported `NewChainRuntime([]NetworkFilter{…})` + the `scriptedConn`/`recordTerminal`/`alwaysContinue` helpers; the existing `newChainRuntime([]ReadFilter{…})` call sites are UNCHANGED in type — Marker-embed only, done in Task 2)
- Modify: `internal/filter/network/echo/echo.go`, `internal/filter/network/directresponse/directresponse.go` (closure returns `network.NetworkFilter`)
- Modify: `internal/listener/manager.go` (mechanical `[]network.ReadFilter` → `[]network.NetworkFilter` in `buildNetChainFactory`@684-690 + `chainInfo.netChainFactory`@195 + `serveReadFilterChain`@1133)

- [ ] **Step 1: Write the failing tests.**

`prefixconn_test.go` — replay semantics:

```go
package network

import (
	"io"
	"testing"
)

func TestPrefixConnReplaysPrefixThenLive(t *testing.T) {
	// scriptedConn yields "LIVE" on the first underlying Read, then io.EOF — a
	// deterministic harness (no net.Pipe / no CloseWrite, which net.Pipe lacks).
	pc := newPrefixConn(scriptedConn([]byte("LIVE")), []byte("PREFIX"))
	got, _ := io.ReadAll(pc)
	if string(got) != "PREFIXLIVE" {
		t.Fatalf("prefixConn read %q, want PREFIXLIVE", got)
	}
}

func TestPrefixConnEmptyPrefixIsPassthrough(t *testing.T) {
	pc := newPrefixConn(scriptedConn([]byte("X")), nil)
	got, _ := io.ReadAll(pc)
	if string(got) != "X" {
		t.Fatalf("empty-prefix passthrough broken: %q", got)
	}
}
```

> NOTE: `scriptedConn(live []byte) net.Conn` is the shared Task-4 test helper (see the `chain_test.go` block below) — a `net.Conn` whose first `Read` returns `live` then `io.EOF`, capturing writes. It is used by BOTH `prefixconn_test.go` and `chain_test.go`. Do NOT use `net.Pipe` here: `net.Pipe` conns do NOT implement `CloseWrite`, and the existing `chain_test.go` `fakeConn` is read-EOF-only (won't exercise the live tail). The assertion (`PREFIX` then live bytes; empty-prefix passthrough) is the load-bearing contract — `prefixConn.Read` drains the prefix first then delegates, so a broken replay fails it.

`chain_test.go` — classification + terminal handoff (R-M):

```go
// recordTerminal captures the bytes Handle reads off the handed-over conn.
type recordTerminal struct {
	Marker
	got []byte
}

func (rt *recordTerminal) Handle(_ context.Context, c net.Conn) {
	rt.got, _ = io.ReadAll(c)
}

// alwaysContinue drains NOTHING and Continues — the synthetic read filter that
// hands the buffered prefix to the terminal (R-M; no shippable 26.2 filter does
// this — rbac_network is the first, at 26.3).
type alwaysContinue struct {
	Marker
	cb ReadFilterCallbacks
}

func (f *alwaysContinue) OnNewConnection() Status                       { return Continue }
func (f *alwaysContinue) OnData(_ *Buffer, _ bool) Status               { return Continue }
func (f *alwaysContinue) SetReadFilterCallbacks(cb ReadFilterCallbacks) { f.cb = cb }
func (f *alwaysContinue) OnDestroy()                                    {}

func TestPureTerminalImmediateHandoff(t *testing.T) {
	term := &recordTerminal{}
	// A conn that yields "RAW" then EOF.
	conn := scriptedConn([]byte("RAW"))
	rt := NewChainRuntime([]NetworkFilter{term}, conn, ConnFacts{})
	if !rt.TerminalReady() {
		t.Fatal("pure-terminal chain not TerminalReady at construction")
	}
	rt.HandleTerminal(context.Background())
	if string(term.got) != "RAW" {
		t.Fatalf("pure-terminal handoff: terminal saw %q, want RAW (byte-identical to Handle(conn))", term.got)
	}
}

func TestMixedChainBufferedPrefixHandoff(t *testing.T) { // R-M
	term := &recordTerminal{}
	rf := &alwaysContinue{}
	conn := scriptedConn([]byte("LIVE"))
	rt := NewChainRuntime([]NetworkFilter{rf, term}, conn, ConnFacts{})
	rt.OnNewConnection()
	rt.OnData([]byte("PREFIX"), false) // rf Continues without draining → prefix retained
	if !rt.TerminalReady() {
		t.Fatal("mixed chain not TerminalReady after read filter Continued")
	}
	rt.HandleTerminal(context.Background())
	if string(term.got) != "PREFIXLIVE" {
		t.Fatalf("buffered-prefix handoff: terminal saw %q, want PREFIXLIVE", term.got)
	}
}

func TestPureReadNeverTerminalReady(t *testing.T) {
	conn := scriptedConn(nil)
	rt := NewChainRuntime([]NetworkFilter{&filterB{}}, conn, ConnFacts{})
	rt.OnNewConnection()
	if rt.TerminalReady() {
		t.Fatal("pure-read chain reported TerminalReady")
	}
}
```

(Add a small `scriptedConn(live []byte) net.Conn` test helper that returns `live` on the first `Read` then `io.EOF`, capturing writes — or reuse/extend the existing `fakeConn`.)

- [ ] **Step 2: Run tests → fail** (`undefined: newPrefixConn` / `TerminalReady` / `NewChainRuntime` arity).
Run: `go test ./internal/filter/network/ -run 'PrefixConn|Terminal|MixedChain|PureRead' -v`

- [ ] **Step 3: Minimal implementation.**

`prefixconn.go`:

```go
// internal/filter/network/prefixconn.go — buffered-prefix handover wrapper.
package network

import "net"

// prefixConn replays an undrained buffered prefix to a terminal filter BEFORE
// delegating to the live downstream conn (the bytes a preceding read filter
// inspected but did not consume). All non-Read methods promote from the
// embedded net.Conn. For an empty prefix it is a transparent passthrough →
// byte-identical to handing the raw conn to Handle (the pure-terminal case).
type prefixConn struct {
	net.Conn
	prefix []byte
}

func newPrefixConn(c net.Conn, prefix []byte) *prefixConn {
	return &prefixConn{Conn: c, prefix: prefix}
}

func (p *prefixConn) Read(b []byte) (int, error) {
	if len(p.prefix) > 0 {
		n := copy(b, p.prefix)
		p.prefix = p.prefix[n:]
		return n, nil
	}
	return p.Conn.Read(b)
}
```

`types.go`: `type FilterInstanceFactory func() NetworkFilter` (update the doc-comment to "allocates a fresh ReadFilter — or returns the shared TerminalFilter — per accepted connection").

`echo/echo.go`: `return func() network.NetworkFilter { return &echoFilter{} }, nil`
`directresponse/directresponse.go`: `return func() network.NetworkFilter { return &filter{cfg: cc} }, nil`

`chain.go`:
  - `NewChainRuntime(filters []NetworkFilter, conn net.Conn, facts ConnFacts) *ChainRuntime` — classify into `read []ReadFilter` + `terminal TerminalFilter` (type-switch each; a value that is `case TerminalFilter` → terminal, `case ReadFilter` → append to read; `default` → this is a build-time-validated invariant, but defensively ignore). Pass `read` to `newChainRuntime`; store `terminal` on the `ChainRuntime`/`chainRuntime`.
    > Classification ASSUMES the shape is already validated (Task 8 `buildNetworkChainFactory` rejects terminal-not-last / multiple-terminals at boot). `NewChainRuntime` keeps the LAST terminal if (defensively) more than one appears; the boot validation prevents that reaching here.
  - Add `terminal TerminalFilter` to `chainRuntime`.
  - `TerminalReady() bool` on `ChainRuntime` → `rt.terminalReady()`:
    ```go
    func (rt *chainRuntime) terminalReady() bool {
        return rt.terminal != nil && !rt.connHalted && rt.resumeIdx >= len(rt.filters)
    }
    ```
    (`rt.filters` is the read prefix. Pure-terminal: `len==0`, `resumeIdx==0` → ready immediately. Mixed: ready once all read filters Continued. Pure-read: `terminal==nil` → never.)
  - `HandleTerminal(ctx context.Context)` on `ChainRuntime` → `rt.handleTerminal(ctx)`:
    ```go
    func (rt *chainRuntime) handleTerminal(ctx context.Context) {
        conn := rt.conn
        if rt.buf.Len() > 0 {
            // Replay the undrained buffered prefix before the live socket.
            prefix := make([]byte, rt.buf.Len())
            copy(prefix, rt.buf.Bytes())
            rt.buf.Drain(rt.buf.Len())
            conn = newPrefixConn(rt.conn, prefix)
        }
        rt.terminal.Handle(ctx, conn)
    }
    ```
    (For a pure-terminal chain `rt.buf` is empty → `conn == rt.conn` → byte-identical to today's `Handle(ctx, dispatchConn)`.)

`manager.go` (mechanical ripple — keep the 26.1 mixed-chain reject; it is REPLACED in Task 7):
  - `chainInfo.netChainFactory func() []network.NetworkFilter` (was `[]network.ReadFilter`).
  - `buildNetChainFactory` closure: `rf := make([]network.NetworkFilter, len(instFactories)); rf[i] = mk()` returning `[]network.NetworkFilter`.
  - `serveReadFilterChain`: `filters := selected.netChainFactory()` (now `[]network.NetworkFilter`) → `network.NewChainRuntime(filters, dispatchConn, facts)`.

- [ ] **Step 4: Run tests → pass; race-clean; full build green.**
Run: `go test -race ./internal/filter/network/... && go build ./... && go test -short ./internal/listener/...`
Expected: PASS (the manager's read-filter path still drives echo/direct_response through `NewChainRuntime`; terminals are not yet built into any chain).

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/prefixconn.go internal/filter/network/prefixconn_test.go internal/filter/network/types.go internal/filter/network/chain.go internal/filter/network/chain_test.go internal/filter/network/echo/ internal/filter/network/directresponse/ internal/listener/manager.go
git commit -m "phase 26.2 Task 4: []NetworkFilter chain + terminal handoff + prefixConn (FilterInstanceFactory→NetworkFilter) [SPEC 3.2; D-26.2-3; R-M]"
```

---

## Task 5: `tcpproxy.NewNetworkFactory` adapter (§4.1)

`tcp_proxy`'s `Handle` (`filter.go:69`) + `NewFilter` (`filter.go:40`) are UNTOUCHED. Add the `network.Marker` embed + the `TerminalFilter` compile-assertion + the `NewNetworkFactory` adapter that captures `cm`/`dm` and yields the SHARED `*Filter` per connection (terminal filters are conn-stateless — preserving today's `chainInfo.filter` shared-instance semantic).

**Files:**
- Modify: `internal/filter/tcpproxy/filter.go` (embed `network.Marker`; `var _`; `NewNetworkFactory`)
- Test: `internal/filter/tcpproxy/network_factory_test.go` (NEW)

- [ ] **Step 1: Write the failing test.**

```go
package tcpproxy

import (
	"testing"

	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/network"
)

var _ network.TerminalFilter = (*Filter)(nil)

func TestNewNetworkFactorySharedInstance(t *testing.T) {
	cm := /* build a cluster.Manager with cluster "c" — reuse the existing tcpproxy_test helper */ testClusterManager(t, "c")
	factory := NewNetworkFactory(cm, nil)
	tc, _ := anypb.New(&tcpproxyv3.TcpProxy{
		StatPrefix:       "p",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c"},
	})
	mk, err := factory(tc, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("NewNetworkFactory factory err: %v", err)
	}
	a, b := mk(), mk()
	if a != b {
		t.Errorf("tcp_proxy adapter must yield the SAME shared instance per call (conn-stateless terminal); got distinct")
	}
	if _, ok := a.(network.TerminalFilter); !ok {
		t.Errorf("yielded instance is not a network.TerminalFilter")
	}
}

func TestNewNetworkFactoryParseRejectPassthroughByteStable(t *testing.T) {
	cm := testClusterManager(t, "c")
	factory := NewNetworkFactory(cm, nil)
	// empty cluster reference → the existing NewFilter reject, surfaced verbatim.
	tc, _ := anypb.New(&tcpproxyv3.TcpProxy{ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: ""}})
	_, err := factory(tc, network.FactoryCtx{})
	if err == nil || err.Error() != "tcpproxy: cluster reference is empty" {
		t.Fatalf("parse-reject not surfaced byte-stable through adapter: %v", err)
	}
}
```

(Reuse the existing `tcpproxy` test cluster-manager helper — `git grep -n 'cluster.NewManager' internal/filter/tcpproxy/*_test.go` to find it; do NOT invent a new one.)

- [ ] **Step 2: Run test → fails** (`undefined: NewNetworkFactory`; `*Filter` does not satisfy `network.TerminalFilter` — missing `isNetworkFilter`).
Run: `go test ./internal/filter/tcpproxy/ -run 'NewNetworkFactory' -v`

- [ ] **Step 3: Minimal implementation** (`filter.go`):
  - Add the import `"github.com/esalaine/envoy-go/internal/filter/network"`.
  - Embed the marker: `type Filter struct { network.Marker; cluster *cluster.Cluster; statPrefix string; dm *drain.Manager }`.
  - Add `var _ network.TerminalFilter = (*Filter)(nil)` (compile-time R-T assertion).
  - Add the adapter:
    ```go
    // NewNetworkFactory returns a network.NetworkFilterFactory that builds the
    // tcp_proxy terminal filter once per chain at boot (capturing cm + dm) and
    // yields that SHARED *Filter per accepted connection. tcp_proxy is
    // conn-stateless (per-connection state lives on Handle's stack), so the
    // shared instance preserves the retired chainInfo.filter semantic. The
    // FactoryCtx per-chain fields are ignored (tcp_proxy has no listener-ctx
    // dependency). The existing NewFilter parse-reject errors surface verbatim
    // (byte-stable; R-S).
    func NewNetworkFactory(cm *cluster.Manager, dm *drain.Manager) network.NetworkFilterFactory {
        return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
            f, err := NewFilter(tc, cm, dm)
            if err != nil {
                return nil, err
            }
            return func() network.NetworkFilter { return f }, nil
        }
    }
    ```

- [ ] **Step 4: Run tests → pass; package green; existing tcpproxy tests unaffected.**
Run: `go test ./internal/filter/tcpproxy/ -v && go vet ./internal/filter/tcpproxy/`
Expected: PASS (R3: `Handle`/`NewFilter` untouched → existing tests green).

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/tcpproxy/
git commit -m "phase 26.2 Task 5: tcpproxy.NewNetworkFactory adapter + TerminalFilter assertion [SPEC 4.1; R-T]"
```

---

## Task 6: `hcm.NewNetworkFactory` adapter (§4.2)

HCM's `Handle` (`filter.go:66`) + `NewFilterWithCtxAndSinksAndRegistry` (`filter.go:45`) + `hcm.ListenerCtx` (`config.go:55`) are UNTOUCHED. Add the marker + assertion + the adapter that captures the boot singletons (`cm`/`registry`/`sinks`/`httpReg`/`dm`/`httpClient`) and bridges `network.FactoryCtx` → `hcm.ListenerCtx` — exactly the bridge the retired `filterRegistry` HCM closure did at `manager.go:112-132`, MOVED into the adapter (mechanical).

**Files:**
- Modify: `internal/filter/hcm/filter.go` (embed `network.Marker` on `Filter`; `var _`; `NewNetworkFactory`)
- Test: `internal/filter/hcm/network_factory_test.go` (NEW)

- [ ] **Step 1: Write the failing test** (FactoryCtx→ListenerCtx bridge; shared instance; stat-registration parity; parse-reject pass-through):

```go
package hcm

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

var _ network.TerminalFilter = (*Filter)(nil)

func TestHCMNewNetworkFactoryBridgesFactoryCtx(t *testing.T) {
	reg := stats.NewRegistry()
	httpReg := /* a frozen *filter_http.HTTPRegistry with router registered — reuse the existing hcm test helper */ testHTTPRegistry(t)
	factory := NewNetworkFactory(testClusterManager(t), reg, nil, httpReg, nil, nil)
	tc := validHCMTypedConfig(t) // reuse the existing hcm config-test fixture builder
	mk, err := factory(tc, network.FactoryCtx{HasTLS: true, AllowH2C: true, ListenerPrincipal: "spiffe://p", NodeServiceCluster: "svc"})
	if err != nil {
		t.Fatalf("NewNetworkFactory err: %v", err)
	}
	a, b := mk(), mk()
	if a != b {
		t.Errorf("HCM adapter must yield the SAME shared instance per call")
	}
	if _, ok := a.(network.TerminalFilter); !ok {
		t.Errorf("yielded instance is not a network.TerminalFilter")
	}
	// stat-registration parity: the same metrics the manager-path build registered
	// are present on reg (assert the HCM per-instance metric names via the existing
	// stat-roster assertion the hcm config tests already use; R-A).
}

func TestHCMNewNetworkFactoryParseRejectPassthrough(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewNetworkFactory(testClusterManager(t), reg, nil, testHTTPRegistry(t), nil, nil)
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff}}
	if _, err := factory(bad, network.FactoryCtx{}); err == nil {
		t.Fatalf("expected HCM parse-reject through adapter")
	}
}
```

(Reuse the existing `hcm` test helpers — `git grep -n 'func test\|NewFilterWithCtxAndSinksAndRegistry(' internal/filter/hcm/*_test.go` to find the config-test fixture builders + cluster/HTTP-registry helpers; do NOT invent new ones. The exact metric-parity assertion mirrors the existing HCM stat tests — R-A.)

- [ ] **Step 2: Run test → fails** (`undefined: NewNetworkFactory`; `*Filter` missing `isNetworkFilter`).
Run: `go test ./internal/filter/hcm/ -run 'NewNetworkFactory' -v`

- [ ] **Step 3: Minimal implementation** (`filter.go`):
  - Add the import `"github.com/esalaine/envoy-go/internal/filter/network"`.
  - Embed the marker on `Filter`: `type Filter struct { network.Marker; … }` (the existing fields unchanged — `git show HEAD:internal/filter/hcm/filter.go` shows `Filter` is defined in `config.go:87`; embed `network.Marker` as its first field there).
  - Add `var _ network.TerminalFilter = (*Filter)(nil)`.
  - Add the adapter (bridge MOVED from `manager.go:112-132`):
    ```go
    // NewNetworkFactory returns a network.NetworkFilterFactory that builds the
    // HCM terminal filter once per chain at boot, capturing the boot singletons
    // (cm, registry, accessLogSinks, httpRegistry, dm, httpClient) and bridging
    // the per-chain network.FactoryCtx into hcm.ListenerCtx. It yields the SHARED
    // *Filter per accepted connection (HCM is conn-stateless across its Handle
    // serve loop). This is the bridge the retired manager.go filterRegistry HCM
    // closure performed (manager.go:112-132), moved into the hcm package.
    func NewNetworkFactory(
        cm *cluster.Manager,
        registry *stats.Registry,
        accessLogSinks []accesslog.Sink,
        httpRegistry *filter_http.HTTPRegistry,
        dm *drain.Manager,
        httpClient *httpclient.Client,
    ) network.NetworkFilterFactory {
        return func(tc *anypb.Any, ctx network.FactoryCtx) (network.FilterInstanceFactory, error) {
            f, err := NewFilterWithCtxAndSinksAndRegistry(
                tc, cm,
                ListenerCtx{
                    HasTLS:             ctx.HasTLS,
                    AllowH2C:           ctx.AllowH2C,
                    ListenerPrincipal:  ctx.ListenerPrincipal,
                    HTTPClient:         httpClient,
                    NodeServiceCluster: ctx.NodeServiceCluster,
                },
                registry, accessLogSinks, httpRegistry, dm,
            )
            if err != nil {
                return nil, err
            }
            return func() network.NetworkFilter { return f }, nil
        }
    }
    ```
    (`httpClient` is closure-captured — a global singleton, NOT a FactoryCtx field per D-26.2-2. Add the `httpclient` import to `filter.go` if not present.)

- [ ] **Step 4: Run tests → pass; package green; existing HCM tests + h2 unaffected.**
Run: `go test ./internal/filter/hcm/... -v && go vet ./internal/filter/hcm/...`
Expected: PASS (R3/R-A: `Handle` + the stat registrations untouched).

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/hcm/
git commit -m "phase 26.2 Task 6: hcm.NewNetworkFactory adapter (FactoryCtx→ListenerCtx bridge moved from manager) + TerminalFilter assertion [SPEC 4.2; R-T; R-A]"
```

---

## Task 7: `manager.go` unified chain builder `buildNetworkChainFactory` — classify + `[read*, terminal?]` shape validation (§3.3 / §6)

Generalize `buildNetChainFactory` into the SOLE chain builder: resolve EVERY filter against `netReg`, classify read vs terminal (by allocating a one-shot sample chain + type-switching), validate the `[read*, terminal?]` shape (the NEW `terminal-not-last` + `multiple-terminals` rejects), DELETE the 26.1 `network-filter-mixed-chain-unsupported` reject, and preserve the unknown-type wording byte-stable. The OLD terminal path (`buildTerminalFilter`/`filterRegistry`) is still present (deleted in Task 10) but now only reached when `filters[0]` is NOT in `netReg`; tests register all four into a test `netReg` so terminals build through the new path.

**Files:**
- Modify: `internal/listener/manager.go` (`buildNetChainFactory` → `buildNetworkChainFactory`; the two call sites @463/@539; the `chainInfo{}` construction)
- Test: `internal/listener/manager_test.go` (build-path: classification, shape rejects, mixed-chain now valid)

- [ ] **Step 1: Write the failing build-path tests.** (Use a test `netReg` with all four registered via `builtins.RegisterBuiltins` — but `builtins` lands in Task 9. To avoid a forward dep, this task's tests register echo/direct_response/tcp_proxy/HCM into the test `netReg` INLINE via `netReg.Register(tcpproxy.TypeURL, tcpproxy.NewNetworkFactory(cm, nil))` etc.; Task 10 swaps the inline registration for `builtins.RegisterBuiltins`.)

```go
func TestBuildChainPureTerminalThroughNetReg(t *testing.T) {
	// netReg has tcp_proxy; a [tcp_proxy] chain builds a netChainFactory whose
	// instances classify as a terminal (no panic; chainInfo.netChainFactory != nil).
}

func TestBuildChainMixedReadTerminalNowValid(t *testing.T) {
	// [echo, tcp_proxy] — the 26.1 mixed-chain reject is LIFTED; this now BUILDS
	// (echo is read, tcp_proxy terminal-last → valid [read*, terminal?]).
}

func TestBuildChainTerminalNotLastRejected(t *testing.T) {
	// [tcp_proxy, echo] → boot-reject: "network-filter-terminal-not-last".
}

func TestBuildChainMultipleTerminalsRejected(t *testing.T) {
	// [tcp_proxy, hcm] → boot-reject: "network-filter-multiple-terminals".
}

// NOTE: the unknown-type-url reject is NOT tested here — through Task 7 a
// filters[0] netReg-miss still falls through to the OLD buildTerminalFilter
// path (manager.go:628), which still emits "unknown filter type_url %q". The
// unified unknown-type reject (the new builder owning that wording) lands in
// Task 10 when buildTerminalFilter is deleted — TestBuildChainUnknownTypeWordingPreserved
// lives there (R-S / D-26.2-6). (A LATER-index miss within an ALREADY-resolved
// net chain IS handled byte-stably by the new builder here via the same
// "unknown filter type_url %q" form — exercised as a sibling case of the
// shape-reject tests; only the index-0 miss defers to the old path through Task 7.)
```

(Match the existing `manager_test.go` bootstrap-construction style; thread the test `netReg`. The shape-reject + mixed-valid tests above register tcp_proxy/HCM into the test `netReg` so `filters[0]` resolves on the NEW path; a netReg-miss still uses the old path here.)

- [ ] **Step 2: Run tests → fail** (the shape rejects + classification not yet implemented; mixed-chain still rejected by the 26.1 arm).
Run: `go test ./internal/listener/ -run 'BuildChain' -v`

- [ ] **Step 3: Minimal implementation.** Generalize `buildNetChainFactory` IN PLACE — **keep its existing 3-value signature** `(func() []network.NetworkFilter, bool, error)` (the `bool` is the 26.1 `isNetChain` fall-through; it is collapsed to 2-value in Task 10 when the old path is deleted). Task 7 changes ONLY the in-net-chain body (classification + shape rejects + the mixed-reject LIFT); it does NOT yet own the unknown-type reject.
  - **Preserve the fall-through:** when `netReg == nil`, `filters` is empty, or `filters[0]`'s type_url is NOT in `netReg`, return `(nil, false, nil)` UNCHANGED — the caller takes the old `buildTerminalFilter` path (which still emits the `unknown filter type_url %q` reject at manager.go:628 for a genuinely-unknown type). Deleting that fall-through + unifying the unknown-type reject is Task 10. (Optionally rename `buildNetChainFactory` → `buildNetworkChainFactory` now for the final name; the rename is cosmetic and the call sites update in the same commit.)
  - When `filters[0]` IS in `netReg` (a network chain): invoke each `NetworkFilterFactory(tc, fctx)` once at boot (validates typed_config → boot-reject on error: `fmt.Errorf("%s: filters[%d]: %w", prefix, idx, err)`), collecting `[]network.FilterInstanceFactory`. A LATER filter that misses `netReg` is no longer the 26.1 mixed-reject — it is now an unknown-type within a net chain: keep returning `(nil, true, err)` with the `buildTerminalFilter:628` wording form `fmt.Errorf("%s: unknown filter type_url %q", prefix, tu)` (byte-stable) so the message is identical whether the miss is at index 0 or later.
  - **Classify + validate shape** by allocating a one-shot sample chain: `sample := make([]network.NetworkFilter, len(insts)); for i, mk := range insts { sample[i] = mk() }`; iterate with a `seenTerminal bool`:
    ```go
    for idx, nf := range sample {
        switch nf.(type) {
        case network.TerminalFilter:
            if seenTerminal {
                return nil, fmt.Errorf("%s: network-filter-multiple-terminals: filters[%d] is a second terminal filter (a chain may carry at most one tcp_proxy/HCM)", prefix, idx)
            }
            if idx != len(sample)-1 {
                return nil, fmt.Errorf("%s: network-filter-terminal-not-last: filters[%d] is a terminal filter but is not last in the chain (a terminal filter owns the connection; nothing may follow it)", prefix, idx)
            }
            seenTerminal = true
        case network.ReadFilter:
            if seenTerminal { // defensive — unreachable given terminal-not-last above
                return nil, fmt.Errorf("%s: network-filter-terminal-not-last: read filter follows a terminal filter (filters[%d])", prefix, idx)
            }
        default:
            return nil, fmt.Errorf("%s: filters[%d]: resolved filter is neither a read nor a terminal network filter", prefix, idx)
        }
    }
    ```
    (D-26.2-6 finalizes whether `multiple-terminals` folds into `terminal-not-last` — kept distinct here for a clearer operator message; confirm wording byte-stable at IMPL.)
  - DELETE the 26.1 `network-filter-mixed-chain-unsupported` arm (§6.3 LIFT) — a later-filter miss is now the unknown-type form above, not a "mixed chain" reject.
  - On the success path, return `func() []network.NetworkFilter { … re-run insts … }, true, nil` (the closure now yields `[]network.NetworkFilter`; the `bool` stays `true` for a resolved net chain, `false` only on the index-0 fall-through above). **The 3-value `(factory, isNetChain, error)` signature is RETAINED in Task 7** — the only changes vs 26.1 are the classification + shape rejects + the lifted mixed-reject. The 2-value collapse + the index-0 fall-through deletion + the unified-unknown-type ownership are Task 10.
  - Thread the per-chain `network.FactoryCtx` from the call sites: replace the 26.1 `network.FactoryCtx{BaseDir: baseDir}` (built inside `buildNetChainFactory`) with the caller passing `network.FactoryCtx{BaseDir: baseDir, HasTLS: chainTLS != nil, AllowH2C: allowH2C, ListenerPrincipal: extractListenerPrincipal(chainTLS), NodeServiceCluster: nodeServiceCluster}` (per-chain @463; the dfc form @539 uses `dfcTLS`). This is the FactoryCtx the HCM adapter consumes.

- [ ] **Step 4: Run tests → pass; full listener package green (R4 — existing tcp_proxy/HCM tests still pass via the OLD path when netReg lacks them, and via the NEW path when the test registers them).**
Run: `go test ./internal/listener/ -v`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go
git commit -m "phase 26.2 Task 7: unified buildNetworkChainFactory (classify + [read*,terminal?] shape rejects; mixed-chain LIFT) [SPEC 3.3/6; D-26.2-6]"
```

---

## Task 8: `manager.go` `serveConnection` step-7 → one `serveNetworkChain` (§3.3)

Generalize `serveReadFilterChain` → `serveNetworkChain` to drive all three chain kinds (pure-read read loop unchanged; pure-terminal immediate `Handle`; mixed handoff on `TerminalReady`). The old `selected.filter.Handle` branch at step-7 STAYS for nil-`netChainFactory` chains (deleted in Task 10). Test all four dispatch shapes through a test `netReg`.

**Files:**
- Modify: `internal/listener/manager.go` (`serveConnection` step-7@1104-1109; `serveReadFilterChain`@1126 → `serveNetworkChain`)
- Test: `internal/listener/manager_test.go` (dispatch shapes over real localhost conns)

- [ ] **Step 1: Write the failing tests** (drive real localhost connections; reuse the existing manager_test listener-start harness + ephemeral ports):

```go
func TestServeNetworkChainTCPProxy(t *testing.T) {
	// [tcp_proxy] new-path chain → byte-identical L4 pump to an upstream echo
	// backend (R3: matches the pre-migration terminal path).
}

func TestServeNetworkChainHCM(t *testing.T) {
	// [hcm] new-path chain → an HTTP/1 request round-trips (R3).
}

func TestServeNetworkChainEchoStillReadLoop(t *testing.T) {
	// [echo] → bytes echoed (the 26.1 read loop is unchanged).
}

func TestServeNetworkChainDirectResponse(t *testing.T) {
	// [direct_response] → static body then close (26.1 path unchanged).
}
```

- [ ] **Step 2: Run tests → fail** (`serveNetworkChain` undefined; tcp_proxy/HCM not yet dispatched through the new path).
Run: `go test ./internal/listener/ -run 'ServeNetworkChain' -v`

- [ ] **Step 3: Minimal implementation.**
  - `serveConnection` step-7 (@1104-1109): keep the dual-branch, but route the net-chain branch to the renamed unifier:
    ```go
    // (7) Dispatch: unified network-filter chain path OR (transitional) old
    // terminal path (deleted in Task 10 once every chain resolves via netReg).
    if selected.netChainFactory != nil {
        rt.serveNetworkChain(ctx, dispatchConn, *selected)
    } else {
        selected.filter.Handle(ctx, dispatchConn)
    }
    ```
  - Rename `serveReadFilterChain` → `serveNetworkChain` and generalize:
    ```go
    func (rt *listenerRuntime) serveNetworkChain(ctx context.Context, dispatchConn net.Conn, selected chainInfo) {
        facts := network.ConnFacts{
            ServerName: requestedServerName(dispatchConn, selected),
            Principals: downstreamPrincipals(dispatchConn),
            Local:      dispatchConn.LocalAddr(),
            Remote:     dispatchConn.RemoteAddr(),
        }
        filters := selected.netChainFactory()
        rtChain := network.NewChainRuntime(filters, dispatchConn, facts)
        defer rtChain.OnDestroy()

        // Pure-terminal chain: hand off immediately — byte-identical to the
        // retired selected.filter.Handle(ctx, dispatchConn). Handle owns close.
        if rtChain.TerminalReady() {
            rtChain.HandleTerminal(ctx)
            return
        }

        rtChain.OnNewConnection()
        if rtChain.TerminalReady() { // a read filter Continued in OnNewConnection
            rtChain.HandleTerminal(ctx)
            return
        }

        buf := make([]byte, 16*1024)
        for {
            if rtChain.CloseRequested() {
                break
            }
            n, err := dispatchConn.Read(buf)
            if n > 0 {
                rtChain.OnData(buf[:n], false)
            }
            if rtChain.TerminalReady() { // mixed read→terminal handoff (rbac_network 26.3)
                rtChain.HandleTerminal(ctx)
                return // Handle owns the conn-close lifecycle
            }
            if rtChain.CloseRequested() {
                break
            }
            if err != nil {
                if errors.Is(err, io.EOF) {
                    rtChain.OnData(nil, true)
                }
                break
            }
        }
        _ = dispatchConn.Close() // pure-read close (echo/direct_response); terminal path returned above
    }
    ```
    (Pure-read path: byte-identical to the 26.1 `serveReadFilterChain` loop + `dispatchConn.Close()`. Terminal paths `return` before the manager close so `Handle`'s own `defer conn.Close()` runs — byte-identical to today.)

- [ ] **Step 4: Run tests → pass; race-clean; full listener package green (R3).**
Run: `go test -race ./internal/listener/ -v`
Expected: PASS (tcp_proxy/HCM dispatch byte-exact through the new path; echo/direct_response unchanged).

- [ ] **Step 5: Commit.**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go
git commit -m "phase 26.2 Task 8: serveConnection step-7 → unified serveNetworkChain (pure-read/pure-terminal/mixed) [SPEC 3.3; R3/R-U]"
```

---

## Task 9: `RegisterBuiltins` seam — `internal/filter/network/builtins` (D-26.2-5)

A NEW importable package with a single `RegisterBuiltins(reg, Deps)` helper registering all four built-ins with their boot singletons captured. ADDITIVE (no manager change yet); consumed by main.go + the thinner ctors + the test callers in Tasks 10/11.

**Files:**
- Create: `internal/filter/network/builtins/builtins.go`, `internal/filter/network/builtins/builtins_test.go`

- [ ] **Step 1: Write the failing test.**

```go
package builtins

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/filter/network/echo"
	"github.com/esalaine/envoy-go/internal/filter/network/directresponse"
	"github.com/esalaine/envoy-go/internal/filter/hcm"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
)

func TestRegisterBuiltinsRegistersAllFour(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{ /* cm, stats, httpReg may be stubbed/nil where the registration itself does not build a filter */ })
	for _, tu := range []string{echo.TypeURL, directresponse.TypeURL, tcpproxy.TypeURL, hcm.TypeURL} {
		if _, ok := reg.Lookup(tu); !ok {
			t.Errorf("RegisterBuiltins did not register %q", tu)
		}
	}
}
```

(Registration only stores factory CLOSURES — it does NOT build any filter — so `Deps` may be zero-valued for this test; the closures are invoked later at chain-build time.)

- [ ] **Step 2: Run test → fails** (package does not exist).
Run: `go test ./internal/filter/network/builtins/ -v`

- [ ] **Step 3: Minimal implementation** (`builtins.go`):

```go
// Package builtins registers the four built-in network filters (echo,
// direct_response, tcp_proxy, HCM) into a *network.Registry with their boot
// singletons captured. It lives OUTSIDE internal/filter/network (which the
// filters import) and outside internal/listener (whose tests import this), so
// no import cycle forms (D-26.2-5 / D-26.2-7). Consumed by cmd/envoy-go/main.go
// + the listener manager's thinner constructors + the admin/manager/main test
// callers — the single place the boot-singleton wiring lives.
package builtins

import (
	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/drain"
	"github.com/esalaine/envoy-go/internal/filter/hcm"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/filter/network/directresponse"
	"github.com/esalaine/envoy-go/internal/filter/network/echo"
	"github.com/esalaine/envoy-go/internal/filter/tcpproxy"
	"github.com/esalaine/envoy-go/internal/httpclient"
	"github.com/esalaine/envoy-go/internal/stats"
)

// Deps carries the boot singletons the terminal-filter adapters capture. The
// read filters (echo/direct_response) need none. Nil-tolerant where the
// underlying adapter/constructor is (dm, httpClient, accessLogSinks).
type Deps struct {
	ClusterManager *cluster.Manager
	StatsRegistry  *stats.Registry
	AccessLogSinks []accesslog.Sink
	HTTPRegistry   *filter_http.HTTPRegistry
	DrainManager   *drain.Manager
	HTTPClient     *httpclient.Client
}

// RegisterBuiltins registers echo, direct_response, tcp_proxy, and HCM into reg.
// It does NOT Freeze (the caller freezes after any additional registration).
func RegisterBuiltins(reg *network.Registry, deps Deps) {
	reg.Register(echo.TypeURL, echo.New)
	reg.Register(directresponse.TypeURL, directresponse.New)
	reg.Register(tcpproxy.TypeURL, tcpproxy.NewNetworkFactory(deps.ClusterManager, deps.DrainManager))
	reg.Register(hcm.TypeURL, hcm.NewNetworkFactory(deps.ClusterManager, deps.StatsRegistry, deps.AccessLogSinks, deps.HTTPRegistry, deps.DrainManager, deps.HTTPClient))
}
```

- [ ] **Step 4: Run test → passes; `go build ./...` confirms NO import cycle (D-26.2-7).**
Run: `go test ./internal/filter/network/builtins/ -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/builtins/
git commit -m "phase 26.2 Task 9: RegisterBuiltins seam (internal/filter/network/builtins) [SPEC 3.4; D-26.2-5/7]"
```

---

## Task 10: `netReg` required + caller re-wiring + DELETE old terminal path (§3.3)

Make `netReg` intrinsic, rewire every constructor + test caller to a populated `netReg` (via `builtins.RegisterBuiltins`), and DELETE the now-unreachable old terminal path (`filterHandler`/`filterConstructor`/`filterRegistry`/`buildTerminalFilter`/`listenerCtx`/`chainInfo.filter`). After this task every chain resolves through `netReg`; the old path is dead → its deletion is safe + green.

**Files:**
- Modify: `internal/listener/manager.go` (collapse `buildNetworkChainFactory` to 2-value; drop the `isNetChain` branch + `chainInfo.filter`; delete `filterHandler`/`filterConstructor`/`filterRegistry`/`buildTerminalFilter`/`listenerCtx`; `NewManager`/`NewManagerWithBaseDir` build a netReg via `builtins.RegisterBuiltins`)
- Modify: `internal/listener/manager_test.go` (swap the Task-7 inline registrations for `builtins.RegisterBuiltins`)
- Modify: `internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go`, `cmd/envoy-go/main_test.go` (populate `netReg` via `builtins.RegisterBuiltins` instead of `nil`)

- [ ] **Step 1: Write the failing test** (R-U: the old types are gone; nil netReg now rejects a filter chain):

```go
func TestOldTerminalRegistryRetired(t *testing.T) {
	// Compile-time/grep guard lives in Step 4; here assert behavior: a chain with
	// a tcp_proxy filter built through NewManagerWithBaseDirAndAllowH2C with a
	// builtins-populated netReg dispatches correctly, and a nil netReg rejects it.
}

func TestNilNetRegRejectsFilterChain(t *testing.T) {
	// netReg == nil + a [tcp_proxy] chain → boot error (no old path to fall back to).
}

func TestBuildChainUnknownTypeWordingPreserved(t *testing.T) { // R-S / D-26.2-6 — MOVED here from Task 7
	// filters[0] type_url in NEITHER built-in → the new builder (now the SOLE
	// path; buildTerminalFilter deleted) emits the EXISTING wording byte-for-byte:
	// "...: unknown filter type_url %q" (the manager.go:628 string preserved).
}
```

- [ ] **Step 2: Run test → fails** (nil-netReg still falls to the old path; old types still present; the unknown-type reject is still emitted by the old `buildTerminalFilter` until it is deleted in Step 3).
Run: `go test ./internal/listener/ -run 'OldTerminalRegistry|NilNetReg|BuildChainUnknownType' -v`

- [ ] **Step 3: Minimal implementation.**
  - **Collapse `buildNetworkChainFactory` to 2-value** `(func() []network.NetworkFilter, error)`: drop the `(nil, false, nil)` fall-through; `netReg == nil` OR `len(filters) == 0` → return a clear boot error (e.g. `fmt.Errorf("%s: filter chain has no filters", prefix)` for empty; for `netReg == nil` the constructors now always pass one, so a nil here is a programming error — return `fmt.Errorf("%s: no network-filter registry configured", prefix)`). An unresolved `filters[0]` is now the unified `unknown filter type_url` reject (it was the only reason to fall through before).
  - **Call sites** (@463/@539): drop the `isNetChain` bool + the `if !isNetChain { buildTerminalFilter(...) }` block + the `var fh filterHandler`. `chainInfo{serverNames, tlsCfg, netChainFactory}` (no `filter`). The dfc error-wrap (`errUnwrapFilterChain`) — re-evaluate: the new builder emits the full prefix already, so the dfc arm wraps the same way the 26.1 net-chain arm did (no double-prefix); keep/drop `errUnwrapFilterChain` per whether any path still double-wraps (it was only the old `buildTerminalFilter` dfc arm — likely DELETE `errUnwrapFilterChain` too; confirm via build + the dfc boot-reject tests).
  - **`chainInfo`**: delete the `filter filterHandler` field; keep `netChainFactory func() []network.NetworkFilter` (now always non-nil).
  - **`serveConnection` step-7**: collapse to the single call `rt.serveNetworkChain(ctx, dispatchConn, *selected)` (delete the `else { selected.filter.Handle }` branch).
  - **DELETE**: `filterHandler` (@46), `filterConstructor` (@97), `filterRegistry` (@104), `buildTerminalFilter` (@612), `listenerCtx` (@61) — confirm `extractListenerPrincipal` (@153) is RETAINED (it feeds `FactoryCtx.ListenerPrincipal`). Remove now-unused imports surfaced by `go build` (e.g. if `accesslog`/`stats` were only used by the old closures — but they remain used elsewhere; let the compiler guide).
  - **Thinner ctors** `NewManager`/`NewManagerWithBaseDir`: build + populate a `netReg` internally so they stay self-sufficient:
    ```go
    func NewManager(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, registry *stats.Registry, httpRegistry *filter_http.HTTPRegistry) (*Manager, error) {
        netReg := network.NewRegistry()
        builtins.RegisterBuiltins(netReg, builtins.Deps{ClusterManager: cm, StatsRegistry: registry, HTTPRegistry: httpRegistry})
        netReg.Freeze()
        return NewManagerWithBaseDirAndAllowH2C(bs, cm, "", false, registry, nil, httpRegistry, nil, nil, nil, netReg)
    }
    ```
    (and the analogous body for `NewManagerWithBaseDir`). Add the `builtins` import to `manager.go` — no cycle (builtins does not import listener; D-26.2-7).
  - **Test callers** (admin/manager_test/main_test): replace the `nil` netReg arg (or the Task-7 inline registration) with a `builtins.RegisterBuiltins`-populated, frozen `netReg`. A shared test helper (e.g. in `manager_test.go`) `func testNetReg(t, cm, stats, httpReg) *network.Registry` keeps the four registrations DRY across the test files.

- [ ] **Step 4: Run tests → pass; ZERO post-retirement references (R-U).**
```bash
go test -race ./internal/listener/... ./internal/admin/... ./cmd/envoy-go/... -v
git grep -nE 'filterHandler|filterConstructor|filterRegistry|buildTerminalFilter|\blistenerCtx\b' -- 'internal/listener/*.go'   # expect: no matches (only doc/history references elsewhere)
```
Expected: tests PASS; the grep returns nothing in `internal/listener/*.go` (R-U).

- [ ] **Step 5: Commit.**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go internal/admin/admin_helpers_test.go internal/admin/listeners_test.go cmd/envoy-go/main_test.go
git commit -m "phase 26.2 Task 10: netReg intrinsic + caller re-wiring (builtins) + retire filterRegistry/buildTerminalFilter/listenerCtx [SPEC 3.3; D-26.2-4/5; R-U]"
```

---

## Task 11: Boot-wiring at `cmd/envoy-go/main.go` (§3.4)

Replace the 26.1 echo/direct_response Register block with `builtins.RegisterBuiltins` capturing all the boot singletons. The `httpClient :=` declaration (@227) currently lands AFTER the netReg block (@213-216) — it must move ABOVE so `RegisterBuiltins` can capture it.

**Files:**
- Modify: `cmd/envoy-go/main.go` (move `httpClient :=` above netReg; swap the Register block for `builtins.RegisterBuiltins`)

- [ ] **Step 1: Gate** — main.go has no unit test; gate on `go build ./...` + `go vet` + a boot smoke (Step 4). Confirm the pre-edit build:
Run: `go build ./cmd/envoy-go/`
Expected: builds (pre-edit).

- [ ] **Step 2: Re-confirm the singleton variable names** the adapter closures need (from Task 1 re-pin): `cm` (@98), `bs.Stats`, `sinks` (@108), `httpReg` (@132), `drainMgr` (@90), `httpClient` (@227). Note the ORDERING fix required.

- [ ] **Step 3: Implement the boot-wiring.**
  - MOVE the `httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})` declaration (@227) to ABOVE the netReg block (before line 213) so it is in scope for `RegisterBuiltins`.
  - Replace the 26.1 netReg block (@213-216):
    ```go
    // Phase-26.2 (§3.4): register all four built-in network filters (echo +
    // direct_response read filters; tcp_proxy + HCM terminal filters) via the
    // shared seam, capturing the boot singletons in the terminal adapters. Freeze
    // BEFORE the listener manager is constructed (the per-listener parser resolves
    // filter_chains[].filters[].type_urls against the frozen registry).
    netReg := network.NewRegistry()
    builtins.RegisterBuiltins(netReg, builtins.Deps{
        ClusterManager: cm,
        StatsRegistry:  bs.Stats,
        AccessLogSinks: sinks,
        HTTPRegistry:   httpReg,
        DrainManager:   drainMgr,
        HTTPClient:     httpClient,
    })
    netReg.Freeze()
    ```
  - Update imports: drop the now-unused `echo`/`directresponse` direct imports if no longer referenced; add `"github.com/esalaine/envoy-go/internal/filter/network/builtins"` (keep `network` for `NewRegistry`).
  - The ctor call (@229) is unchanged (it already passes `netReg`).

- [ ] **Step 4: Verify build + vet + a live boot smoke (tcp_proxy + HCM through the unified dispatch).**
```bash
go build ./... && go vet ./...
# boot smoke: start the binary with a [tcp_proxy] config + a [hcm] config and confirm no boot reject + a round-trip
go build -o /tmp/envoy-go ./cmd/envoy-go
/tmp/envoy-go -c <a minimal tcp_proxy bootstrap> &  # then nc a payload; expect proxied bytes
```
Expected: build/vet PASS; boot smoke accepts + round-trips (the live proof is the Task-12 differential suite).

- [ ] **Step 5: Commit.**

```bash
git add cmd/envoy-go/main.go
git commit -m "phase 26.2 Task 11: boot-wire all four network filters via builtins.RegisterBuiltins (httpClient ordering fix) [SPEC 3.4]"
```

---

## Task 12: docs bundle (BEHAVIOR_CONTRACT + ADR-0215 body) + STATE/ROADMAP advance + six-gate verification (§9 / §14 / §15)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the §9 26.2 bundle)
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0215 §Decision/§Consequences bodies — tail STAYS ADR-0215; NO new number)
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`
- Modify: `docs/envoy-go/phases/26.2-network-filter-registry-migration/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT.md 26.2 bundle** (§9 / §14): UPDATE the `## Network filters` → `### Network filter chain framework` subsection — document the NEW `TerminalFilter` seam (connection-takeover model alongside the `ReadFilter` OnData model); the `[read-filter*, terminal-filter?]` chain shape; the unified single-dispatch path (dual-dispatch retired); the `tcp_proxy`/HCM migration onto the registry (no behavior change). REMOVE the `network-filter-mixed-chain-unsupported` 26.1-transitional departure record (the restriction is LIFTED) — replace with a note that mixed read+terminal chains are now expressible (first consumer `rbac_network` 26.3) + the NEW chain-shape rejects (`network-filter-terminal-not-last`, `network-filter-multiple-terminals`). Add the structural note (hardcoded `manager.go` `filterRegistry`/`filterConstructor`/`filterHandler`/`buildTerminalFilter` retired; all four filters resolve through `*network.Registry`; `tcp_proxy`/HCM `Handle` UNCHANGED). Confirm stat surface 132 + no new fixtures/fuzzers. Add the 26.3 forward-pointer (`rbac_network` + `internal/rbac/` engine + connection-metadata writes).

- [ ] **Step 2: DECISIONS.md ADR-0215 §Decision/§Consequences bodies** (the §Context was drafted at the 26.2 SPEC; fill the bodies IN PLACE per ADR-0044 — tail STAYS ADR-0215; next-free STAYS ADR-0216). §Decision: the `network.TerminalFilter` connection-takeover seam (parent §3.2 terminal-fit resolved in favor of the seam over an OnData rewrite — byte-exact parity over re-architecture); the sealed `NetworkFilter` marker (exported `Marker` embeddable) + generalized `FilterInstanceFactory`; the extended `FactoryCtx` (per-chain primitives; heavy singletons closure-captured in `internal/filter/network/builtins`); the unified `[read*, terminal?]` dispatch + the buffered-prefix `prefixConn` handover; the `filterRegistry`/`filterConstructor`/`filterHandler`/`buildTerminalFilter`/`listenerCtx` retirement; the `network-filter-mixed-chain-unsupported` LIFT + the NEW chain-shape rejects; the `netReg`-intrinsic registration seam. §Consequences: back-compat-via-existing-fixtures discipline (R3); stat/fixture/fuzzer-neutral; readies `rbac_network` (26.3) as the first mixed read→terminal consumer.

- [ ] **Step 3: Run the full six-gate suite + record outputs in PROGRESS.md** (per superpowers:verification-before-completion — quote every command's output):

```bash
go build ./... && go vet ./... && golangci-lint run && go test -race -short ./...
# differential suite is Docker-driven + skipped under -short — run it explicitly (R3 back-compat, run LIVE):
go test ./test/differential/ -run 'TestDifferential' -v   # ALL 44 byte-exact green, ESPECIALLY 0000-tcp-echo + 0002-tls-tcp (tcp_proxy) + the HCM/h2/wasm/lua fixtures (HCM through the migrated dispatch) + 0040/0041/0042
git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l   # 35 (+0)
ls test/fixtures/ | grep -E '^[0-9]' | wc -l                        # 44 (+0)
# stat surface golden → 132 (+0)
# conformance 10/10 + h2spec 53/53 — re-run LIVE (HCM dispatches through the migrated path; NOT asserted-unaffected)
```

Expected: build/vet/lint/race-test PASS; the FULL differential suite byte-exact green (the load-bearing R3 migration proof); fuzzers 35; fixtures 44; stats 132; conformance 10/10; h2spec 53/53. **If any tcp_proxy/HCM fixture fails byte-exactness, the migration is wrong — STOP and debug (superpowers:systematic-debugging); do NOT adjust the fixture.**

- [ ] **Step 4: STATE.md + ROADMAP.md advance** (per BOOTSTRAP §5; SKILL_ROUTING state 3 → phase-done for sub-row 26.2): STATE.md → "phase 26.2 phase-done; awaiting 26.3 SPEC"; next-skill `superpowers:brainstorming`/`writing-plans` for the 26.3 SPEC; next-free ADR-0216; last-commit TBD-26.2-IMPL-SQUASH (SHA-filled at stage-close). ROADMAP sub-row 26.2 `in-progress → done`; parent row 26 STAYS `in-progress` (flips `in-progress → done` at 26.3 phase-done per the 18/19/22/24/25 ROLLUP precedent).

- [ ] **Step 5: Commit.**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md docs/envoy-go/phases/26.2-network-filter-registry-migration/PROGRESS.md
git commit -m "phase 26.2 Task 12: BEHAVIOR_CONTRACT + ADR-0215 bodies + STATE/ROADMAP advance + six-gate verification [SPEC 9/14/15]"
```

---

## Acceptance (SPEC §15.3) — all must hold before requesting review

1. `tcp_proxy` + HCM migrated onto `network.TerminalFilter` (`Handle` UNCHANGED; compile-time `var _ network.TerminalFilter` assertions) + registered in `*network.Registry` via the `NewNetworkFactory` adapters (Tasks 5/6); R-T verified.
2. The hardcoded `manager.go` `filterHandler`/`filterConstructor`/`filterRegistry`/`buildTerminalFilter`/`listenerCtx` RETIRED (zero post-retirement references — R-U, Task 10); `chainInfo` collapsed to one filter field.
3. The dual-dispatch UNIFIED: `serveConnection` step-7 single branch (Task 8); `buildNetworkChainFactory` the sole chain builder (Task 7); the `*network.Registry` the sole network-filter registry.
4. The `network-filter-mixed-chain-unsupported` 26.1-transitional reject LIFTED; mixed `[read*, terminal]` chains expressible (R-M unit-tested via the synthetic always-`Continue` filter + buffered-prefix `prefixConn` handover, Task 4); the NEW chain-shape rejects (`terminal-not-last`/`multiple-terminals`) land (Task 7).
5. `netReg` intrinsic (nil-tolerance dropped); the `RegisterBuiltins` seam (`internal/filter/network/builtins`, Task 9) consumed by main.go + the thinner ctors + the admin/manager_test/main_test callers (Tasks 10/11); boot smoke green.
6. R3 back-compat: the FULL differential suite (44) + conformance (10/10) + h2spec (53/53) byte-exact green LIVE (Task 12). Stat surface 132; fuzzers 35; fixtures 44 (all +0).
7. ADR-0215 §Decision/§Consequences bodies land (tail STAYS ADR-0215; no new number); BEHAVIOR_CONTRACT 26.2 bundle (Task 12).
8. Six gates green; STATE advanced; ROADMAP 26.2 `→ done`; parent 26 stays `in-progress` (Task 12).

## Execution handoff

After this PLAN is reviewed (plan-document-reviewer-APPROVED) + committed + squash-merged + pushed, the 26.2 IMPL runs via **superpowers:subagent-driven-development** (per `feedback_execution_style`): a fresh subagent per task, two-stage review between tasks, commits LOCAL-ONLY (subagents do NOT push — `feedback_subagents_no_push`), controller pushes at stage-close.
