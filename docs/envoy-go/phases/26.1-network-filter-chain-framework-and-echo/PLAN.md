# Phase 26.1 Implementation Plan — NEW `internal/filter/network/` read-filter chain framework + `echo` + `direct_response` (dual-dispatch wiring)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended, per the `feedback_execution_style` project memory) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task is TDD-first (write-the-failing-test → run-it-fails → minimal-impl → run-it-passes → commit) per superpowers:test-driven-development.

**Goal:** Land the L4 network read-filter chain framework (`internal/filter/network/`) and the family's first two trivial filters — `echo` + `direct_response` — wired into `internal/listener/manager.go` via an additive dual-dispatch path, boot-wired in `cmd/envoy-go/main.go`, at full upstream parity with reference Envoy v1.37.2.

**Architecture:** A NEW `internal/filter/network/` package supplies the read-filter iteration protocol (two-value `Status`; `ReadFilter` with `OnNewConnection`/`OnData`/`SetReadFilterCallbacks`/`OnDestroy`), `ReadFilterCallbacks` (Connection accessor + `ContinueReading()` + `DynamicMetadata()`), a per-connection drainable `*Buffer`, a per-connection chain runner + runtime context (owns a REUSED `*dynamicmetadata.Bucket`), and a freeze-after-boot `*Registry` (mirrors `internal/listener/listenerfilter/registry.go` + `internal/filter/http/registry.go`). `echo` + `direct_response` are the first two consumers. The chain dispatch is wired into `manager.go` as a NEW path ALONGSIDE the existing untouched terminal-filter path (`tcp_proxy`/HCM are migrated at 26.2, not here). Read-filter-ONLY scope (ADR-0213).

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); golangci-lint 1.64.8 (ADR-0009); REUSE `internal/dynamicmetadata/` at connection scope (parent AMEND-A5). ZERO new third-party `go.mod` dependencies.

**Module path:** `github.com/esalaine/envoy-go`.

**Source of truth:** the phase-26.1 SPEC (`docs/envoy-go/phases/26.1-network-filter-chain-framework-and-echo/SPEC.md`), especially §3.1 (production API signatures — copy verbatim), §3.2 (file split), §3.3 (chain runner), §3.4 (per-connection context), §3.5 (dual-dispatch), §3.6 (boot-wiring), §4 (echo + direct_response), §6 (PARSE-REJECT), §8 (fixtures), §10 (task spine this plan decomposes), §11.1 (D-S1 baselines = Task-1 gate), §12 (D-questions), §13 (R1–R7), §15 (acceptance).

---

## ADR-0045 split-gate check (PERFORMED at PLAN time)

Per SKILL_ROUTING state-2 GATE: split if PLAN > ~25 tasks OR > ~1500 estimated LoC. This plan is **17 tasks**, SPEC §10/§15 estimates **~850–1020 net-new LoC, ZERO moved LoC**. Both are comfortably within the gate (~25 tasks / ~1500 LoC). **NO split.** Proceed as a single 26.1 IMPL.

## PLAN-time D-question resolutions (SPEC §12)

These were left to the PLAN; resolved here so IMPL has no open design choices (the remaining D-P26.1-3/4/5 are byte-stable wording + boot-reject arm + close-detection/RCD-storage, which are IMPL-time empirical pins and are scoped into Tasks 8/9/11/16):

- **D-P26.1-1 (read-buffer type) → drainable `*Buffer`.** A small struct wrapping `[]byte` exposing `Bytes() []byte`, `Drain(n int)`, `Len() int`, `Append(p []byte)`. Faithfully models upstream's `Buffer::Instance` drain semantics that `echo.cc`'s `write()` relies on, and is unit-testable in isolation (Task 3). Simplify to a plain `[]byte`+consumed-count only if TDD surfaces friction.
- **D-P26.1-2 (DataSource resolution sharing) → INLINE in `directresponse`, modeled on `internal/tls/datasource.go`.** There is **no** shared `internal/datasource` package. The two precedents are (a) `internal/filter/http/wasm/datasource.go:resolveDataSource` — `os.ReadFile`-direct, no baseDir, `AsyncDataSource.Local`-shaped (WRONG shape); and (b) **`internal/tls/datasource.go:loadDataSource(ds *corev3.DataSource, baseDir string)`** — plain `DataSource`, baseDir-relative `Filename` (RIGHT shape). `directresponse` inlines a 4-arm switch mirroring (b). **Consequence (SPEC §3.1 refinement):** the SPEC declared `network.FactoryCtx struct{}` (empty), but direct_response's `Filename` arm needs the bootstrap base dir at **boot** time (a missing file must reject at config-load for byte-stable boot-reject parity, not at connection). Therefore this plan REFINES `network.FactoryCtx` to carry **`BaseDir string`** (threaded from `manager.go`, which already has `baseDir`). This is a deliberate ADR-0004 ambiguity-resolution; echo ignores `BaseDir`, direct_response consumes it. (Recorded in the `reference_network_factoryctx_basedir` project memory.)

## Registry fidelity note (SPEC §3.1 / R1)

The precedents `internal/listener/listenerfilter/registry.go` and `internal/filter/http/registry.go` both use `frozen atomic.Bool` (NOT a plain bool) and take `mu.RLock()` on `Lookup` (the SPEC's "lock-free post-Freeze" means the read path is uncontended after Freeze, not literally lock-free). The `*network.Registry` mirrors this EXACTLY: `mu sync.RWMutex; byTypeURL map[string]NetworkFilterFactory; frozen atomic.Bool`; `Lookup` RLocks; `Freeze` does `frozen.Store(true)`. `KnownTypeURLs()` mirrors `internal/filter/http/registry.go:66` (insertion-sort, deterministic). Register panic wording (mirroring listenerfilter): `"network: registry frozen: cannot register %q post-boot"` and `"network: duplicate factory for %q"`. NO package-global `init()` (ADR-0072).

---

## File Structure

### Created — framework package `internal/filter/network/`

| File | Responsibility |
|---|---|
| `doc.go` | package doc — L4 read-filter chain framework; ADR-0213/0214 cross-refs |
| `types.go` | `Status` enum + `ReadFilter` interface + `NetworkFilterFactory`/`FilterInstanceFactory` + `FactoryCtx{BaseDir string}` (SPEC §3.1, refined per D-P26.1-2) |
| `buffer.go` | drainable `*Buffer` (`Bytes`/`Drain`/`Len`/`Append`) |
| `callbacks.go` | `ReadFilterCallbacks` + `Connection` interface + `CloseType` enum (SPEC §3.1) |
| `registry.go` | freeze-after-boot `*Registry` (mirror listenerfilter + http `KnownTypeURLs`) |
| `chain.go` | per-connection chain runner (§3.3) + per-connection runtime context (§3.4, owns `*dynamicmetadata.Bucket` + `responseCodeDetails`) + concrete `connection`/`callbacks` impls over `net.Conn` |

Tests: `buffer_test.go`, `registry_test.go`, `chain_test.go`, `callbacks_test.go`.

### Created — filter packages

| Package | Files |
|---|---|
| `internal/filter/network/echo/` | `doc.go`, `echo.go` (`TypeURL` + `New` + `echoFilter` + `OnData`), `echo_test.go`, `fuzz_test.go` |
| `internal/filter/network/directresponse/` | `doc.go`, `directresponse.go` (`TypeURL` + `New` + `compiledConfig` + 4-arm DataSource + `OnNewConnection` + parse-reject consts), `directresponse_test.go`, `fuzz_test.go` |

### Created — differential fixtures

The differential harness uses **driver packages** that self-register via `init()` (`fixture.RegisterFixture(name, driver{})`, `test/differential/fixture/fixture.go:84`) and are **blank-imported** into `test/differential/runner_test.go` (the import block at ~lines 27-36). The runner iterates `fixture.DriverRegistry` and `t.Run(name, …)` where `name` is the fixture-dir name. A new fixture = a new `driver` package + a new blank-import line. Cross-side drivers live under `…/driver/`; boot-reject drivers under `…/inputs/` (both `package driver`). Templates: `test/fixtures/0000-tcp-echo/driver/driver.go` (cross-side raw TCP — the closest analogue for echo/direct_response) and `test/fixtures/0033-http-ratelimit-boot-reject/inputs/driver.go` (boot-reject).

| File | Responsibility |
|---|---|
| `test/fixtures/0040-network-echo/driver/driver.go` | fixture `0040` driver (cross-side; `package driver`; `init()`-registered; 8-method `fixture.Driver`) |
| `test/fixtures/0040-network-echo/README.md` | fixture rationale |
| `test/fixtures/0041-network-direct-response/driver/driver.go` | fixture `0041` driver (cross-side) |
| `test/fixtures/0041-network-direct-response/README.md` | fixture rationale |
| `test/fixtures/0042-network-direct-response-boot-reject/inputs/driver.go` | fixture `0042` driver (boot-reject; `fixture.Driver` + `harness.BootRejectFixture`) |
| `test/fixtures/0042-network-direct-response-boot-reject/README.md` | fixture rationale |
| `test/differential/runner_test.go` (MODIFY) | add 3 blank-import lines for the new driver packages |

### Modified

| File | Change |
|---|---|
| `internal/listener/manager.go` | `chainInfo.netChainFactory` field (4th); `netReg *network.Registry` 11th ctor param + `nil` in thinner variants; build-time dual-dispatch pre-check at the two `buildTerminalFilter` call sites (@444/@503); `network-filter-mixed-chain-unsupported` arm; `serveReadFilterChain` read loop; `serveConnection` step-7 dual-branch (@1004-1005) |
| `internal/listener/manager_test.go`, `internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go`, `cmd/envoy-go/main_test.go` | the OTHER direct callers of `NewManagerWithBaseDirAndAllowH2C` (verified via `git grep`) — each gains the new `netReg` arg (`nil` where no network filters) so the build gate stays green |
| `test/differential/runner_test.go` | 3 blank-import lines for the new driver packages |
| `cmd/envoy-go/main.go` | `netReg` construct+Register+Freeze block (after `lfReg` @198-200); append `netReg` to the `NewManagerWithBaseDirAndAllowH2C(...)` call @213 |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | 26.1 bundle (Task 17) |
| `docs/envoy-go/DECISIONS.md` | ADR-0213 + ADR-0214 §Decision/§Consequences bodies (Task 17) |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | phase-done advance (Task 17) |

---

## Task 1: First-action baselines + proto-roster re-confirm (HARD GATE)

The master tip may have advanced since the SPEC commit (`9429983`). Re-pin every baseline BEFORE asserting any delta (SPEC §11.1 D-S1; R6/R7). No production code in this task.

**Files:** none (verification only). Record results in `docs/envoy-go/phases/26.1-network-filter-chain-framework-and-echo/PROGRESS.md` (create it).

- [ ] **Step 1: Re-grep the four baselines — use git-tracked enumeration (deterministic).**

Use `git ls-files`, NOT `find .`: the repo root contains dozens of nested git worktrees under `.worktrees/` whose `fuzz_test.go` files inflate a naive `find .`/`grep -r` count. (The PLAN review flagged a spurious "35" from exactly this artifact; the git-tracked count is 34.)

```bash
cd "$(git rev-parse --show-toplevel)"
echo "fuzzers:";      git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l   # expect 34
echo "fixture dirs:"; ls test/fixtures/ | grep -E '^[0-9]' | wc -l                          # expect 41
echo "fixture tail:"; ls test/fixtures/ | grep -E '^[0-9]' | sort | tail -1                 # expect 0039-...
echo "ADR tail:";     grep -nE '^#+ +ADR-0[0-9]{3}' docs/envoy-go/DECISIONS.md | tail -1            # expect ADR-0214 (grep HEADINGS, not prose: a naive `grep -oE 'ADR-0[0-9]{3}' | sort -u | tail -1` matches PLANNED forward-references like 0215/0216/0217 in the §provisional-span text and falsely reports a higher tail)
```

Expected: `34`, `41`, `0039-…`, `ADR-0214`. **If any differ**, STOP and reconcile the deltas (the SPEC numbering 0040/0041/0042 + "35th fuzzer" + "stays 132" assume these baselines) before proceeding — adjust fixture numbers / counts to the new tip and note the drift in PROGRESS.md. Also re-pin the `manager.go` line numbers cited in Tasks 10/11 (@182/@261/@273/@302/@444/@503/@585/@1004-1005), since `manager.go` may have shifted since the SPEC's `9429983` pin.

- [ ] **Step 2: Re-confirm the stat surface = 132.** Use the project's stat-roster grep/golden (the same one phase-25.3 used; see `docs/envoy-go/STATE.md` history / the stats golden test). Expected: `132`. 26.1 adds 0.

- [ ] **Step 3: Re-confirm the echo/direct_response/DataSource proto rosters vs go-control-plane v1.32.4.**

```bash
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/direct_response/v3.Config
go doc github.com/envoyproxy/go-control-plane/envoy/config/core/v3.DataSource
```

Expected: `direct_response.v3.Config` has the single `Response *v3.DataSource` field (message is named `Config`, NOT `DirectResponse`); `echo.v3.Echo` is an empty message; `DataSource` has the 4-arm `specifier` oneof (`InlineBytes`/`InlineString`/`Filename`/`EnvironmentVariable`). Confirm the Go binding package names (`echov3` / `direct_responsev3`).

- [ ] **Step 4: Confirm six gates green at the tip** (baseline must be clean before new code):

```bash
go build ./... && go vet ./... && golangci-lint run && go test -race -short ./...
```

Expected: all pass.

- [ ] **Step 5: Commit** (PROGRESS.md only):

```bash
git add docs/envoy-go/phases/26.1-network-filter-chain-framework-and-echo/PROGRESS.md
git commit -m "phase 26.1 Task 1: re-pin D-S1 baselines (fuzzers 34, fixtures 41/tail 0039, stats 132, ADR-0214)"
```

---

## Task 2: `internal/filter/network/` skeleton — `doc.go` + `types.go`

**Files:**
- Create: `internal/filter/network/doc.go`
- Create: `internal/filter/network/types.go`
- Test: `internal/filter/network/types_test.go`

- [ ] **Step 1: Write the failing test.** A compile-level test that the protocol types exist with the right shape.

```go
package network

import "testing"

func TestStatusValues(t *testing.T) {
	if Continue != 0 || StopIteration != 1 {
		t.Fatalf("Status enum drift: Continue=%d StopIteration=%d", Continue, StopIteration)
	}
}

// Compile-time assertion that a minimal ReadFilter satisfies the interface.
type noopFilter struct{}

func (noopFilter) OnNewConnection() Status                  { return Continue }
func (noopFilter) OnData(_ *Buffer, _ bool) Status          { return Continue }
func (noopFilter) SetReadFilterCallbacks(_ ReadFilterCallbacks) {}
func (noopFilter) OnDestroy()                               {}

var _ ReadFilter = noopFilter{}
```

- [ ] **Step 2: Run test to verify it fails.**

Run: `go test ./internal/filter/network/ -run TestStatusValues -v`
Expected: FAIL — `undefined: Continue` / package does not compile.

- [ ] **Step 3: Write minimal implementation.** Copy the `types.go` block VERBATIM from SPEC §3.1, REFINED per D-P26.1-2 so `FactoryCtx` carries `BaseDir`:

```go
// internal/filter/network/types.go
package network

import "google.golang.org/protobuf/types/known/anypb"

type Status int

const (
	Continue Status = iota
	StopIteration
)

type ReadFilter interface {
	OnNewConnection() Status
	OnData(buf *Buffer, endStream bool) Status
	SetReadFilterCallbacks(cb ReadFilterCallbacks)
	OnDestroy()
}

//nolint:revive // ADR-0213 reserves the NetworkFilterFactory name for the boot-time factory surface.
type NetworkFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)

type FilterInstanceFactory func() ReadFilter

// FactoryCtx carries the parsed-config context a NetworkFilterFactory needs.
// BaseDir is the bootstrap config directory (for direct_response DataSource
// Filename resolution relative to the config file; D-P26.1-2). echo ignores it.
type FactoryCtx struct {
	BaseDir string
}
```

Include the full doc comments from SPEC §3.1 on `Status`/`ReadFilter` (and the `//nolint:revive` on `Status`). Add `doc.go` (package doc; cite ADR-0213/0214).

- [ ] **Step 4: Run test to verify it passes.**

Run: `go test ./internal/filter/network/ -run TestStatusValues -v && go vet ./internal/filter/network/`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/doc.go internal/filter/network/types.go internal/filter/network/types_test.go
git commit -m "phase 26.1 Task 2: network framework skeleton (Status, ReadFilter, factory types, FactoryCtx.BaseDir) [SPEC 3.1; D-P26.1-2]"
```

---

## Task 3: drainable `*Buffer` (D-P26.1-1)

**Files:**
- Create: `internal/filter/network/buffer.go`
- Test: `internal/filter/network/buffer_test.go`

- [ ] **Step 1: Write the failing test** (drain semantics — the load-bearing contract):

```go
func TestBufferDrainSemantics(t *testing.T) {
	b := &Buffer{}
	b.Append([]byte("hello"))
	b.Append([]byte("-world"))
	if b.Len() != 11 {
		t.Fatalf("Len=%d want 11", b.Len())
	}
	if string(b.Bytes()) != "hello-world" {
		t.Fatalf("Bytes=%q", b.Bytes())
	}
	b.Drain(6) // drop "hello-"
	if string(b.Bytes()) != "world" || b.Len() != 5 {
		t.Fatalf("after Drain(6): Bytes=%q Len=%d", b.Bytes(), b.Len())
	}
	b.Drain(b.Len())
	if b.Len() != 0 {
		t.Fatalf("after full drain: Len=%d want 0", b.Len())
	}
	b.Drain(100) // over-drain is clamped, not a panic
	if b.Len() != 0 {
		t.Fatalf("over-drain: Len=%d want 0", b.Len())
	}
}
```

- [ ] **Step 2: Run test → fails** (`undefined: Buffer`).
Run: `go test ./internal/filter/network/ -run TestBufferDrainSemantics -v`

- [ ] **Step 3: Minimal implementation:**

```go
// internal/filter/network/buffer.go
package network

// Buffer is the per-connection drainable read buffer. The chain owns ONE
// Buffer for all read filters (connection-level buffering per SPEC §3.3); a
// filter consumes bytes by Drain after copying them out (e.g. echo writes
// Bytes() back then Drain(Len())).
type Buffer struct {
	data []byte
}

func (b *Buffer) Append(p []byte) { b.data = append(b.data, p...) }
func (b *Buffer) Bytes() []byte   { return b.data }
func (b *Buffer) Len() int        { return len(b.data) }

// Drain drops the first n bytes (clamped to Len). It re-slices in place;
// callers must copy Bytes() before Drain if they need the bytes after.
func (b *Buffer) Drain(n int) {
	if n >= len(b.data) {
		b.data = b.data[:0]
		return
	}
	b.data = b.data[n:]
}
```

- [ ] **Step 4: Run test → passes.** Run: `go test ./internal/filter/network/ -run TestBufferDrainSemantics -v`

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/buffer.go internal/filter/network/buffer_test.go
git commit -m "phase 26.1 Task 3: drainable network.Buffer [SPEC 3.1; D-P26.1-1]"
```

---

## Task 4: `callbacks.go` — `ReadFilterCallbacks` + `Connection` + `CloseType`

**Files:**
- Create: `internal/filter/network/callbacks.go`
- Test: extended in `internal/filter/network/callbacks_test.go` (the live accessor test lands with the concrete impl in Task 6; here we only assert the interface + enum shape).

- [ ] **Step 1: Write the failing test** (enum + interface shape):

```go
func TestCloseTypeValues(t *testing.T) {
	if FlushWrite != 0 || NoFlush != 1 {
		t.Fatalf("CloseType drift: FlushWrite=%d NoFlush=%d", FlushWrite, NoFlush)
	}
}

// Compile-time: the interfaces are declared with the §3.1 method set.
var _ = func(cb ReadFilterCallbacks) {
	_ = cb.Connection()
	cb.ContinueReading()
	_ = cb.DynamicMetadata()
}
var _ = func(c Connection) {
	c.Write(nil, false)
	c.Close(FlushWrite)
	_, _ = c.LocalAddr(), c.RemoteAddr()
	_ = c.RequestedServerName()
	_ = c.DownstreamPrincipals()
}
```

- [ ] **Step 2: Run test → fails** (`undefined: FlushWrite` / `Connection`).

- [ ] **Step 3: Minimal implementation.** Copy the `callbacks.go` block VERBATIM from SPEC §3.1 (the `ReadFilterCallbacks` interface with `Connection()`/`ContinueReading()`/`DynamicMetadata() *dynamicmetadata.Bucket`; the `CloseType` enum `FlushWrite`/`NoFlush`; the `Connection` interface `Write([]byte, bool)`/`Close(CloseType)`/`LocalAddr()`/`RemoteAddr() net.Addr`/`RequestedServerName() string`/`DownstreamPrincipals() []string`), including the full doc comments and the `import "github.com/esalaine/envoy-go/internal/dynamicmetadata"`.

- [ ] **Step 4: Run test → passes.** Run: `go test ./internal/filter/network/ -run TestCloseTypeValues -v`

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/callbacks.go internal/filter/network/callbacks_test.go
git commit -m "phase 26.1 Task 4: ReadFilterCallbacks + Connection + CloseType [SPEC 3.1]"
```

---

## Task 5: freeze-after-boot `*Registry` (R1)

**Files:**
- Create: `internal/filter/network/registry.go`
- Test: `internal/filter/network/registry_test.go` (mirror `internal/listener/listenerfilter/registry_test.go`)

- [ ] **Step 1: Write the failing tests** (mirror the listenerfilter suite — plain `t`/`recover()`, no testify):

```go
func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	var f NetworkFilterFactory = func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) { return nil, nil }
	r.Register("type.url/A", f)
	if _, ok := r.Lookup("type.url/A"); !ok {
		t.Errorf("registered factory not found")
	}
	if _, ok := r.Lookup("missing"); ok {
		t.Errorf("missing key returned ok=true")
	}
}

func TestRegistryDuplicateRegisterPanics(t *testing.T) {
	r := NewRegistry()
	var f NetworkFilterFactory = func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) { return nil, nil }
	r.Register("dup", f)
	defer func() { if recover() == nil { t.Errorf("expected panic on duplicate register") } }()
	r.Register("dup", f)
}

func TestRegistryFreezeBlocksRegister(t *testing.T) {
	r := NewRegistry()
	r.Freeze()
	defer func() { if recover() == nil { t.Errorf("expected panic on post-freeze register") } }()
	r.Register("late", func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) { return nil, nil })
}

func TestRegistryKnownTypeURLsSorted(t *testing.T) {
	r := NewRegistry()
	noop := func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) { return nil, nil }
	r.Register("b", noop); r.Register("a", noop); r.Register("c", noop)
	got := r.KnownTypeURLs()
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("KnownTypeURLs not sorted: %v", got)
	}
}

func TestRegistryConcurrentLookup(t *testing.T) {
	r := NewRegistry()
	r.Register("x", func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) { return nil, nil })
	r.Freeze()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.Lookup("x") }()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run tests → fail** (`undefined: NewRegistry`).
Run: `go test ./internal/filter/network/ -run TestRegistry -v`

- [ ] **Step 3: Minimal implementation** (mirror listenerfilter + http `KnownTypeURLs`):

```go
// internal/filter/network/registry.go
package network

import (
	"fmt"
	"sync"
	"sync/atomic"
)

//nolint:revive // ADR-0214 reserves the network.Registry name for the boot-time network-filter registry.
type Registry struct {
	mu        sync.RWMutex
	byTypeURL map[string]NetworkFilterFactory
	frozen    atomic.Bool
}

func NewRegistry() *Registry {
	return &Registry{byTypeURL: make(map[string]NetworkFilterFactory)}
}

func (r *Registry) Register(typeURL string, f NetworkFilterFactory) {
	if r.frozen.Load() {
		panic(fmt.Sprintf("network: registry frozen: cannot register %q post-boot", typeURL))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byTypeURL[typeURL]; dup {
		panic(fmt.Sprintf("network: duplicate factory for %q", typeURL))
	}
	r.byTypeURL[typeURL] = f
}

func (r *Registry) Lookup(typeURL string) (NetworkFilterFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.byTypeURL[typeURL]
	return f, ok
}

func (r *Registry) Freeze() { r.frozen.Store(true) }

// KnownTypeURLs returns a sorted slice of registered type_urls for boot-reject
// error messages. Mirrors internal/filter/http/registry.go (insertion sort).
func (r *Registry) KnownTypeURLs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byTypeURL))
	for k := range r.byTypeURL {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests → pass; race-clean.**
Run: `go test -race ./internal/filter/network/ -run TestRegistry -v`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/registry.go internal/filter/network/registry_test.go
git commit -m "phase 26.1 Task 5: freeze-after-boot network.Registry (mirror listenerfilter + http KnownTypeURLs) [SPEC 3.1; R1]"
```

---

## Task 6: chain runner + per-connection runtime context (R3/R2/R5; D-P26.1-5)

This is the load-bearing iteration task. The runner drives sequential read-filter dispatch (SPEC §3.3); the runtime context owns the REUSED `*dynamicmetadata.Bucket` (§3.4) and a `responseCodeDetails string` (D-P26.1-5b sink). The concrete `connection` impl wraps `net.Conn`, records a `closeRequested` state on `Close` (D-P26.1-5a), and exposes the L4 accessors. The concrete `callbacks` impl threads the context.

**Files:**
- Create: `internal/filter/network/chain.go`
- Test: `internal/filter/network/chain_test.go`, plus the live accessor assertions in `internal/filter/network/callbacks_test.go` (R2).

- [ ] **Step 1: Write the failing tests** with two synthetic filters proving StopIteration + ContinueReading resume + connection-level buffering, plus the DynamicMetadata round-trip and the Connection accessor surface:

```go
// fakeConn implements net.Conn capturing writes + close.
type fakeConn struct {
	writes []byte
	closed bool
	addr   net.Addr
}
func (c *fakeConn) Read(b []byte) (int, error)  { return 0, io.EOF }
func (c *fakeConn) Write(b []byte) (int, error) { c.writes = append(c.writes, b...); return len(b), nil }
func (c *fakeConn) Close() error                { c.closed = true; return nil }
func (c *fakeConn) LocalAddr() net.Addr         { return c.addr }
func (c *fakeConn) RemoteAddr() net.Addr        { return c.addr }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

// filterA stops on first OnData, then ContinueReadings; filterB records what it sees.
type filterA struct{ cb ReadFilterCallbacks; stoppedOnce bool }
func (f *filterA) OnNewConnection() Status { return Continue }
func (f *filterA) OnData(b *Buffer, _ bool) Status {
	if !f.stoppedOnce {
		f.stoppedOnce = true
		return StopIteration // hold; resume below
	}
	f.cb.ContinueReading()
	return Continue
}
func (f *filterA) SetReadFilterCallbacks(cb ReadFilterCallbacks) { f.cb = cb }
func (f *filterA) OnDestroy() {}

type filterB struct{ saw string; newConnCalled bool }
func (f *filterB) OnNewConnection() Status { f.newConnCalled = true; return Continue }
func (f *filterB) OnData(b *Buffer, _ bool) Status { f.saw = string(b.Bytes()); return Continue }
func (f *filterB) SetReadFilterCallbacks(ReadFilterCallbacks) {}
func (f *filterB) OnDestroy() {}

func TestChainContinueReadingResumesAtNextFilter(t *testing.T) {
	fc := &fakeConn{addr: &net.TCPAddr{IP: net.IPv4(127,0,0,1), Port: 9}}
	a, b := &filterA{}, &filterB{}
	rt := newChainRuntime([]ReadFilter{a, b}, fc, connFacts{})
	rt.onNewConnection()
	rt.onData([]byte("payload"), false)
	if !b.newConnCalled { t.Errorf("filterB.OnNewConnection not called on resume") }
	if b.saw != "payload" { t.Errorf("filterB saw %q, want buffered bytes", b.saw) }
}

func TestChainDynamicMetadataRoundTrip(t *testing.T) {
	fc := &fakeConn{}
	rt := newChainRuntime([]ReadFilter{&filterB{}}, fc, connFacts{})
	bucket := rt.callbacks().DynamicMetadata()
	bucket.Set("f", "k", structpb.NewStringValue("v"))
	got, ok := rt.callbacks().DynamicMetadata().Get("f", "k")
	if !ok || got.GetStringValue() != "v" { t.Fatalf("metadata round-trip failed: %v %v", got, ok) }
}

func TestConnectionAccessorSurface(t *testing.T) { // R2 readiness — prove each accessor is live
	fc := &fakeConn{addr: &net.TCPAddr{IP: net.IPv4(10,0,0,1), Port: 443}}
	rt := newChainRuntime(nil, fc, connFacts{serverName: "sni.example", principals: []string{"spiffe://x"}})
	c := rt.callbacks().Connection()
	c.Write([]byte("hi"), true)
	if string(fc.writes) != "hi" { t.Errorf("Write not forwarded: %q", fc.writes) }
	if c.RequestedServerName() != "sni.example" { t.Errorf("SNI accessor dead") }
	if len(c.DownstreamPrincipals()) != 1 { t.Errorf("principals accessor dead") }
	if c.RemoteAddr().String() != "10.0.0.1:443" { t.Errorf("RemoteAddr dead") }
	c.Close(FlushWrite)
	if !rt.closeRequested() { t.Errorf("Close did not set closeRequested (D-P26.1-5a)") }
}

func TestResponseCodeDetailsSink(t *testing.T) { // D-P26.1-5b — prove the sink is live before direct_response uses it
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	setter, ok := rt.callbacks().(interface{ SetResponseCodeDetails(string) })
	if !ok { t.Fatalf("callbacks does not expose SetResponseCodeDetails — direct_response (Task 8) sink would be dead") }
	setter.SetResponseCodeDetails("DirectResponse")
	if rt.responseCodeDetails() != "DirectResponse" { t.Errorf("RCD not stored: %q", rt.responseCodeDetails()) }
}
```

(Adjust the exact helper names — `newChainRuntime`, `connFacts`, `callbacks()`, `closeRequested()` — to whatever the impl settles on; the test names + assertions are the contract.)

- [ ] **Step 2: Run tests → fail** (`undefined: newChainRuntime`).
Run: `go test ./internal/filter/network/ -run 'TestChain|TestConnectionAccessor' -v`

- [ ] **Step 3: Minimal implementation** of `chain.go`:
  - `connFacts struct { serverName string; principals []string; local, remote net.Addr }` — the L4 facts the manager already extracted (SNI from tls_inspector, principals from TLS handshake).
  - `chainRuntime struct { filters []ReadFilter; conn net.Conn; facts connFacts; buf *Buffer; bucket *dynamicmetadata.Bucket; rcd string; resumeIdx int; halted bool; closeReq bool; newConnDone []bool }`.
  - `newChainRuntime(filters, conn, facts)` → constructs `bucket = dynamicmetadata.NewBucket()`, allocates `buf`, `newConnDone` per filter, builds the `callbacks`/`connection` impls.
  - concrete `connection` (implements the `Connection` interface): `Write(p, endStream)` → `conn.Write(p)` (synchronous; net.Conn write is already flushed); `Close(ct)` → set `closeReq = true` (record `ct`); `LocalAddr`/`RemoteAddr` from `conn`/`facts`; `RequestedServerName`→`facts.serverName`; `DownstreamPrincipals`→`facts.principals`.
  - concrete `callbacks` (implements `ReadFilterCallbacks`): `Connection()`→the connection impl; `ContinueReading()`→advance `resumeIdx`, clear `halted`, re-run data iteration on `buf`; `DynamicMetadata()`→`bucket`.
  - `onNewConnection()` — for each filter in order: if not `newConnDone[i]`, call `SetReadFilterCallbacks` (once, at construction is cleaner — do it in `newChainRuntime`) then `OnNewConnection()`; mark done; on `StopIteration` set `halted`, record `resumeIdx`, return.
  - `onData(p, endStream)` — `buf.Append(p)`; iterate from `resumeIdx`: lazily call `OnNewConnection` if not done (StopIteration → halt); call `OnData(buf, endStream)` when `buf.Len()>0 || endStream`; on `StopIteration` halt at current idx (undrained bytes stay); on `Continue` advance.
  - `ContinueReading()` — bump `resumeIdx` to next, clear `halted`, re-run the data iteration with currently-buffered bytes (SPEC §3.3).
  - `onDestroy()` — call each filter's `OnDestroy()` in order (defer-style), then `bucket.Reset()`.
  - `closeRequested() bool` → `closeReq`.
  - **RCD sink (D-P26.1-5b) — PINNED HERE, not left open across tasks.** The runtime context owns `rcd string`. The concrete `callbacks` impl exposes a method **`SetResponseCodeDetails(s string)`** (writes `rt.rcd = s`) DIRECTLY on the `ReadFilterCallbacks` concrete type — NOT via an optional-interface assertion. direct_response (Task 8) calls `f.cb.(interface{ SetResponseCodeDetails(string) }).SetResponseCodeDetails(...)` OR, cleaner, add `SetResponseCodeDetails(string)` to a small framework-internal interface the concrete callbacks satisfies. Decide the exact surface NOW (recommended: the concrete `*callbacks` has the method; direct_response type-asserts once and the Task-6 test proves the assertion succeeds + round-trips `rcd`). This makes the sink provably live before Task 8/11 execute (the three tasks run in separate subagents).
  - `SetReadFilterCallbacks(cb)` is called once per filter in `newChainRuntime`.

  Keep it single-goroutine (no locks beyond the registry's; ADR-0213 single-goroutine-per-connection).

- [ ] **Step 4: Run tests → pass; race-clean.**
Run: `go test -race ./internal/filter/network/ -v`
Expected: PASS (all framework tests).

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/chain.go internal/filter/network/chain_test.go internal/filter/network/callbacks_test.go
git commit -m "phase 26.1 Task 6: chain runner + per-conn context (Bucket reuse, closeRequested, RCD sink) [SPEC 3.3/3.4; R2/R3/R5; D-P26.1-5]"
```

---

## Task 7: `echo` filter (§4.1)

**Files:**
- Create: `internal/filter/network/echo/doc.go`, `internal/filter/network/echo/echo.go`
- Test: `internal/filter/network/echo/echo_test.go`

- [ ] **Step 1: Write the failing test** (parse empty config + write-back-drain-StopIteration on OnData):

```go
func TestEchoParseEmptyConfig(t *testing.T) {
	any, _ := anypb.New(&echov3.Echo{})
	fif, err := New(any, network.FactoryCtx{})
	if err != nil || fif == nil { t.Fatalf("New(empty Echo) = %v, %v", fif, err) }
	if rf := fif(); rf == nil { t.Fatalf("instance factory returned nil") }
}

func TestEchoOnDataWritesBackAndDrains(t *testing.T) {
	fif, _ := New(&anypb.Any{TypeUrl: TypeURL}, network.FactoryCtx{}) // empty body accepted
	rf := fif()
	cb := &fakeCallbacks{} // records Connection().Write; provides a *network.Buffer
	rf.SetReadFilterCallbacks(cb)
	buf := &network.Buffer{}
	buf.Append([]byte("ping"))
	st := rf.OnData(buf, false)
	if st != network.StopIteration { t.Errorf("OnData status=%v want StopIteration", st) }
	if string(cb.written) != "ping" { t.Errorf("echo wrote %q want ping", cb.written) }
	if buf.Len() != 0 { t.Errorf("echo did not drain buffer, Len=%d", buf.Len()) }
}
```

(Provide a small `fakeCallbacks` in the test implementing `network.ReadFilterCallbacks` + a `fakeConnection` capturing `Write`.)

- [ ] **Step 2: Run test → fails** (`undefined: New`).
Run: `go test ./internal/filter/network/echo/ -v`

- [ ] **Step 3: Minimal implementation** (mirror the buffer-filter template; SPEC §4.1):

```go
// internal/filter/network/echo/echo.go
package echo

import (
	"fmt"
	echov3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/echo/v3"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"google.golang.org/protobuf/types/known/anypb"
)

const TypeURL = "type.googleapis.com/envoy.filters.network.echo.v3.Echo"

const filterName = "envoy.filters.network.echo"

// New is the NetworkFilterFactory registered at boot. Echo's config is empty;
// an empty/absent typed_config body is accepted (parent AMEND-A2). No field-level reject.
func New(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
	cfg := &echov3.Echo{}
	if tc != nil && len(tc.GetValue()) > 0 {
		if err := tc.UnmarshalTo(cfg); err != nil {
			return nil, fmt.Errorf("echo: invalid typed_config: %w", err)
		}
	}
	return func() network.ReadFilter { return &echoFilter{} }, nil
}

type echoFilter struct{ cb network.ReadFilterCallbacks }

func (f *echoFilter) OnNewConnection() network.Status { return network.Continue }

func (f *echoFilter) OnData(buf *network.Buffer, endStream bool) network.Status {
	f.cb.Connection().Write(buf.Bytes(), endStream)
	buf.Drain(buf.Len())
	return network.StopIteration
}

func (f *echoFilter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks) { f.cb = cb }
func (f *echoFilter) OnDestroy()                                            {}
```

Verify the exact go-control-plane import path/package alias (`echov3`) from Task 1 Step 3.

- [ ] **Step 4: Run test → passes.** Run: `go test ./internal/filter/network/echo/ -v`

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/echo/
git commit -m "phase 26.1 Task 7: network echo filter (OnData write-back + drain + StopIteration) [SPEC 4.1]"
```

---

## Task 8: `direct_response` filter — config + DataSource 4-arm + OnNewConnection (§4.2; D-P26.1-2; D-P26.1-5b)

**Files:**
- Create: `internal/filter/network/directresponse/doc.go`, `internal/filter/network/directresponse/directresponse.go`
- Test: `internal/filter/network/directresponse/directresponse_test.go`

- [ ] **Step 1: Write the failing test** (parse inline_string + OnNewConnection write+endStream+RCD+FlushWrite-close+StopIteration; plus Filename baseDir resolution):

```go
func TestDirectResponseInlineStringWritesAndCloses(t *testing.T) {
	cfg := &drv3.Config{Response: &corev3.DataSource{
		Specifier: &corev3.DataSource_InlineString{InlineString: "BYE\n"}}}
	any, _ := anypb.New(cfg)
	fif, err := New(any, network.FactoryCtx{})
	if err != nil { t.Fatalf("New = %v", err) }
	rf := fif()
	cb := &fakeCallbacks{}
	rf.SetReadFilterCallbacks(cb)
	st := rf.OnNewConnection()
	if st != network.StopIteration { t.Errorf("status=%v want StopIteration", st) }
	if string(cb.written) != "BYE\n" { t.Errorf("wrote %q want BYE", cb.written) }
	if !cb.endStream { t.Errorf("write endStream not set") }
	if cb.closeType != network.FlushWrite { t.Errorf("close type=%v want FlushWrite", cb.closeType) }
}

func TestDirectResponseFilenameRelativeToBaseDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "body.txt"), []byte("FILE-BODY"), 0o600)
	cfg := &drv3.Config{Response: &corev3.DataSource{
		Specifier: &corev3.DataSource_Filename{Filename: "body.txt"}}}
	any, _ := anypb.New(cfg)
	fif, err := New(any, network.FactoryCtx{BaseDir: dir})
	if err != nil { t.Fatalf("New(Filename) = %v", err) }
	cb := &fakeCallbacks{}
	rf := fif(); rf.SetReadFilterCallbacks(cb); rf.OnNewConnection()
	if string(cb.written) != "FILE-BODY" { t.Errorf("wrote %q want FILE-BODY", cb.written) }
}
```

- [ ] **Step 2: Run test → fails.** Run: `go test ./internal/filter/network/directresponse/ -v`

- [ ] **Step 3: Minimal implementation** (SPEC §4.2; inline 4-arm modeled on `internal/tls/datasource.go`):

```go
// internal/filter/network/directresponse/directresponse.go
package directresponse

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	drv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/direct_response/v3"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"google.golang.org/protobuf/types/known/anypb"
)

const TypeURL = "type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config"

// responseCodeDetails is the internal RCD string set on write (parity with
// upstream; no operator-visible surface per SPEC §2.10 / D-P26.1-5b).
const responseCodeDetails = "DirectResponse"

// PARSE-REJECT wording finalized at IMPL (D-P26.1-3); these are the anticipated
// arms (§6.1). Keep byte-stable; pin in TestParseRejectConstants_ByteStable (Task 9).
const (
	parseRejectResponseSpecifierRequired = "direct_response: response.specifier is required"
	parseRejectFilenameRead              = "direct_response: response.filename: %s: %w"
	parseRejectEnvVarUnset               = "direct_response: response.environment_variable: %s: unset"
)

type compiledConfig struct{ body []byte }

func New(tc *anypb.Any, ctx network.FactoryCtx) (network.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, fmt.Errorf("direct_response: invalid typed_config: nil")
	}
	cfg := &drv3.Config{}
	if err := tc.UnmarshalTo(cfg); err != nil {
		return nil, fmt.Errorf("direct_response: invalid typed_config: %w", err)
	}
	body, err := resolveDataSource(cfg.GetResponse(), ctx.BaseDir)
	if err != nil {
		return nil, err
	}
	cc := &compiledConfig{body: body}
	return func() network.ReadFilter { return &filter{cfg: cc} }, nil
}

// resolveDataSource mirrors internal/tls/datasource.go:loadDataSource (baseDir-
// relative Filename), with the 4 specifier arms (D-P26.1-2). An absent/unset
// specifier is the boot-reject parity arm (§6.1; D-P26.1-4 finalizes vs upstream).
func resolveDataSource(ds *corev3.DataSource, baseDir string) ([]byte, error) {
	if ds == nil || ds.GetSpecifier() == nil {
		return nil, errors.New(parseRejectResponseSpecifierRequired)
	}
	switch s := ds.GetSpecifier().(type) {
	case *corev3.DataSource_InlineString:
		return []byte(s.InlineString), nil
	case *corev3.DataSource_InlineBytes:
		return s.InlineBytes, nil
	case *corev3.DataSource_Filename:
		p := s.Filename
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf(parseRejectFilenameRead, p, err)
		}
		return b, nil
	case *corev3.DataSource_EnvironmentVariable:
		v, ok := os.LookupEnv(s.EnvironmentVariable)
		if !ok {
			return nil, fmt.Errorf(parseRejectEnvVarUnset, s.EnvironmentVariable)
		}
		return []byte(v), nil
	default:
		return nil, errors.New(parseRejectResponseSpecifierRequired)
	}
}

type filter struct {
	cfg *compiledConfig
	cb  network.ReadFilterCallbacks
}

func (f *filter) OnNewConnection() network.Status {
	f.cb.Connection().Write(f.cfg.body, true)
	// RCD set on the per-connection context for forward-consumer readiness
	// (no operator surface, no fixture assertion at 26.1 — D-P26.1-5b).
	if s, ok := f.cb.(interface{ SetResponseCodeDetails(string) }); ok {
		s.SetResponseCodeDetails(responseCodeDetails)
	}
	f.cb.Connection().Close(network.FlushWrite)
	return network.StopIteration
}

func (f *filter) OnData(*network.Buffer, bool) network.Status { return network.StopIteration }
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks) { f.cb = cb }
func (f *filter) OnDestroy() {}
```

NOTE on the RCD sink: the storage mechanism is PINNED in Task 6 (the concrete `callbacks` exposes `SetResponseCodeDetails(string)`, proven live by `TestResponseCodeDetailsSink`). The optional-interface assertion above is safe BECAUSE Task 6 guarantees the concrete callbacks satisfies it — it is not left open. Do NOT introduce a divergent field-based mechanism in Task 11; the sink set-path is already live + tested here.

- [ ] **Step 4: Run test → passes.** Run: `go test ./internal/filter/network/directresponse/ -v`

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/directresponse/
git commit -m "phase 26.1 Task 8: direct_response filter (Config + DataSource 4-arm + OnNewConnection write/FlushWrite-close) [SPEC 4.2; D-P26.1-2]"
```

---

## Task 9: `direct_response` PARSE-REJECT arms + `TestParseRejectConstants_ByteStable` (§6.1; D-P26.1-3)

**Files:**
- Modify: `internal/filter/network/directresponse/directresponse.go` (finalize arm wording)
- Test: `internal/filter/network/directresponse/directresponse_test.go` (add the byte-stable table + the reject-path tests)

- [ ] **Step 1: Write the failing tests** (mirror `internal/filter/http/ratelimit/compiled_config_test.go:TestParseRejectConstants_ByteStable`):

```go
func TestParseRejectConstants_ByteStable(t *testing.T) {
	cases := []struct{ name, got, want string }{
		{"SpecifierRequired", parseRejectResponseSpecifierRequired, "direct_response: response.specifier is required"},
		// add Filename / EnvVar arms once finalized empirically (Task 16 / D-P26.1-4)
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("byte-stable drift: %s\n const: %q\n  want: %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestDirectResponseRejectsUnsetSpecifier(t *testing.T) {
	cfg := &drv3.Config{Response: &corev3.DataSource{}} // specifier nil
	any, _ := anypb.New(cfg)
	_, err := New(any, network.FactoryCtx{})
	if err == nil || err.Error() != parseRejectResponseSpecifierRequired {
		t.Fatalf("expected specifier-required reject, got %v", err)
	}
}

func TestDirectResponseRejectsAbsentResponse(t *testing.T) {
	any, _ := anypb.New(&drv3.Config{}) // Response nil
	_, err := New(any, network.FactoryCtx{})
	if err == nil { t.Fatalf("expected reject for absent response") }
}
```

- [ ] **Step 2: Run tests → fail** (constants/behavior not yet finalized).
Run: `go test ./internal/filter/network/directresponse/ -run 'ParseReject|Rejects' -v`

- [ ] **Step 3: Finalize the reject arms.** Confirm the wording constants are exactly as asserted; ensure `New` returns them for the unset-specifier and absent-response cases (already wired in Task 8). The exact `direct-response-*` constant *names* and the boot-stderr substring used by fixture 0042 are finalized in Task 16 against the real upstream binary (D-P26.1-4); keep the wording byte-stable here and update the 0042 substring to a fragment shared with upstream's stderr (the `0033` "omain" precedent).

- [ ] **Step 4: Run tests → pass.** Run: `go test ./internal/filter/network/directresponse/ -v`

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/directresponse/
git commit -m "phase 26.1 Task 9: direct_response PARSE-REJECT arms + byte-stable table [SPEC 6.1; D-P26.1-3]"
```

---

## Task 10: `manager.go` dual-dispatch — build-time pre-check + `chainInfo.netChainFactory` + mixed-chain reject (§3.5 / §6.2)

**Files:**
- Modify: `internal/listener/manager.go` (`chainInfo` struct @182-186; `netReg` ctor param @302 + thinner variants @261/@273; build-time pre-check at the two `buildTerminalFilter` call sites @444/@503)
- Test: `internal/listener/manager_test.go` (build-path unit tests)
- Modify (build-fix): `internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go` — the OTHER direct callers of `NewManagerWithBaseDirAndAllowH2C` (found via `git grep -l NewManagerWithBaseDirAndAllowH2C`). Add the new `netReg` arg (`nil` — admin tests register no network filters) so `go test ./...` compiles. Do this in the SAME task as the signature change to keep every commit green.

- [ ] **Step 1: Write the failing tests** (chain-build new-path vs old-path decision; mixed-chain reject; unknown-type reject preserved):

```go
func TestBuildNewPathChainWhenFilterInNetReg(t *testing.T) {
	netReg := network.NewRegistry()
	netReg.Register(echo.TypeURL, echo.New)
	netReg.Freeze()
	// Build a manager from a bootstrap whose chain filters[0] is echo;
	// assert the resulting chainInfo has netChainFactory != nil and filter == nil.
	// (Use the existing manager_test harness for constructing from a bootstrap.)
}

func TestMixedChainRejectedAt261(t *testing.T) {
	// chain filters = [echo, tcp_proxy] with echo registered in netReg →
	// expect a boot error mentioning "mixed" / network-filter-mixed-chain-unsupported.
}

func TestOldPathUnaffectedWhenNetRegNilOrMiss(t *testing.T) {
	// nil netReg OR filters[0]=tcp_proxy → chainInfo.filter != nil, netChainFactory == nil.
}
```

(Match the existing `manager_test.go` construction style — it already builds managers from bootstrap protos; thread the new `netReg` param.)

- [ ] **Step 2: Run tests → fail** (compile error: `netChainFactory` undefined / ctor arity).
Run: `go test ./internal/listener/ -run 'NewPath|MixedChain|OldPath' -v`

- [ ] **Step 3: Minimal implementation:**
  - Add field to `chainInfo` (after `filter`): `netChainFactory func() []network.ReadFilter` (nil for old-path; non-nil for new-path — a closure capturing the resolved `[]network.FilterInstanceFactory` that allocates fresh instances per connection). Exactly one of `filter` / `netChainFactory` is non-nil.
  - Add `netReg *network.Registry` as the 11th parameter to `NewManagerWithBaseDirAndAllowH2C` (@302); pass `nil` in `NewManager` (@261) and `NewManagerWithBaseDir` (@273).
  - At each `buildTerminalFilter` call site (@444 per-chain, @503 default_filter_chain): BEFORE calling `buildTerminalFilter`, if `netReg != nil` and `filters[0].GetTypedConfig().GetTypeUrl()` resolves in `netReg`, build a new-path factory: resolve EVERY filter in `fc.GetFilters()` against `netReg` (calling each `NetworkFilterFactory(tc, network.FactoryCtx{BaseDir: baseDir})` once at boot to validate + get the per-conn `FilterInstanceFactory`); if any does NOT resolve → boot-reject with the `network-filter-mixed-chain-unsupported` arm; store the resulting `netChainFactory` closure on `chainInfo`, leave `filter` nil, and SKIP `buildTerminalFilter`. Else (filters[0] not in netReg) → existing `buildTerminalFilter` path UNCHANGED.
  - The mixed-chain reject wording: `fmt.Errorf("%s: network-filter-mixed-chain-unsupported: filters[%d] type_url %q is not a network read filter (mixed read+terminal chains are not supported until 26.2)", prefix, idx, tu)`. (Finalize exact wording; keep byte-stable; it is unit-test-only, no fixture.)
  - The existing unknown-type reject (@585 `"%s: unknown filter type_url %q"`) is preserved for the old-path miss (a type_url in neither netReg nor filterRegistry fails both and reuses this wording).

- [ ] **Step 4: Run tests → pass; full package green.**
Run: `go test ./internal/listener/ -v`
Expected: PASS (incl. existing tcp_proxy/HCM tests — R4).

- [ ] **Step 5: Commit.**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go
git commit -m "phase 26.1 Task 10: manager dual-dispatch build-time pre-check + chainInfo.netChainFactory + mixed-chain reject [SPEC 3.5/6.2]"
```

---

## Task 11: `manager.go` `serveReadFilterChain` read loop + `serveConnection` step-7 dual-branch (§3.5; D-P26.1-5a)

**Files:**
- Modify: `internal/listener/manager.go` (`serveConnection` step-7 @1004-1005; add `serveReadFilterChain` method)
- Test: `internal/listener/manager_test.go` (read-loop behavior — echo loop + direct_response write-and-close)

- [ ] **Step 1: Write the failing tests** (drive a real localhost connection through a new-path chain):

```go
func TestServeReadFilterChainEcho(t *testing.T) {
	// Start a manager listener with an [echo] new-path chain; dial it, write
	// bytes, expect them echoed back; half-close → server closes.
}

func TestServeReadFilterChainDirectResponse(t *testing.T) {
	// Start a manager listener with a [direct_response inline_string] chain;
	// dial it, read until EOF, assert bytes == configured response + closed.
}
```

(Reuse the existing manager_test listener-start harness; allocate an ephemeral port.)

- [ ] **Step 2: Run tests → fail** (`serveReadFilterChain` undefined).
Run: `go test ./internal/listener/ -run ServeReadFilterChain -v`

- [ ] **Step 3: Minimal implementation:**
  - Replace `serveConnection` step 7 (@1004-1005) `selected.filter.Handle(ctx, dispatchConn)` with:
    ```go
    // (7) Dispatch: new read-filter chain path OR existing terminal path.
    if selected.netChainFactory != nil {
        rt.serveReadFilterChain(ctx, dispatchConn, selected)
    } else {
        selected.filter.Handle(ctx, dispatchConn)
    }
    ```
    (The old branch is byte-identical to today.)
  - Add `serveReadFilterChain(ctx, dispatchConn net.Conn, selected chainInfo)`: build `connFacts` from the chain's extracted SNI + principals + `dispatchConn` addrs; `filters := selected.netChainFactory()`; `rt := network.NewChainRuntime(filters, dispatchConn, facts)` (export the constructor/runtime needed by manager — add a small exported entry point in `chain.go`, e.g. `network.RunReadFilterChain(ctx, conn, filters, facts)` that wraps `newChainRuntime` + the loop, OR export `NewChainRuntime` + its `OnNewConnection/OnData/Destroy/CloseRequested`); `defer rt.OnDestroy()`; call `rt.OnNewConnection()`; then loop:
    ```go
    buf := make([]byte, 16*1024)
    for {
        if rt.CloseRequested() { break } // direct_response closed in OnNewConnection (D-P26.1-5a)
        n, err := dispatchConn.Read(buf)
        if n > 0 { rt.OnData(buf[:n], false) }
        if rt.CloseRequested() { break }
        if err != nil { // EOF/closed/error
            if errors.Is(err, io.EOF) { rt.OnData(nil, true) }
            break
        }
    }
    _ = dispatchConn.Close()
    ```
    (Echo loops until downstream EOF; direct_response exits on the first `CloseRequested` check after `OnNewConnection`.)
  - This task FINALIZES D-P26.1-5a (the `CloseRequested` flag drives loop exit; `FlushWrite` writes are synchronous net.Conn writes already flushed before `Close()`) and D-P26.1-5b (the RCD sink lives on the runtime context; set-but-unread).
  - Decide the exported surface in `chain.go` so `manager.go` does not reach into unexported runtime internals; keep the framework's public API minimal.

- [ ] **Step 4: Run tests → pass; full package green; race-clean.**
Run: `go test -race ./internal/listener/ -v`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go internal/filter/network/chain.go
git commit -m "phase 26.1 Task 11: serveReadFilterChain read loop + serveConnection step-7 dual-branch [SPEC 3.5; D-P26.1-5a/b]"
```

---

## Task 12: boot-wiring in `cmd/envoy-go/main.go` (§3.6)

**Files:**
- Modify: `cmd/envoy-go/main.go` (add `netReg` block after `lfReg` @198-200; append `netReg` to ctor call @213)
- Modify (build-fix): `cmd/envoy-go/main_test.go` — also a direct caller of `NewManagerWithBaseDirAndAllowH2C` (per `git grep`); add the new `netReg` arg (`nil` or a test registry) so the build gate stays green

- [ ] **Step 1: Write the failing test / gate.** main.go has no unit test; the gate is `go build ./...` + a boot smoke. Add (or extend) a boot smoke test that starts the binary with an `[echo]`-chain config and confirms it accepts (no boot reject). If a main-package test harness does not exist, rely on the differential fixtures (Task 14/15) as the live proof and gate this task on `go build ./...` + `go vet`.

- [ ] **Step 2: Confirm current build references the old 10-arg ctor.**
Run: `go build ./cmd/envoy-go/` — Expected: still builds (pre-edit).

- [ ] **Step 3: Implement the boot-wiring** (mirror the `lfReg` block @198-200):

```go
// Phase-26.1: build the *network.Registry and register the two network read
// filters envoy-go ships at 26.1 (echo + direct_response). Freeze BEFORE the
// listener manager is constructed (the per-listener parser resolves
// filter_chains[].filters[].type_urls against the frozen registry for the
// dual-dispatch decision). tcp_proxy/HCM are NOT registered here (26.2).
netReg := network.NewRegistry()
netReg.Register(echo.TypeURL, echo.New)
netReg.Register(directresponse.TypeURL, directresponse.New)
netReg.Freeze()
```

Append `netReg` to the constructor call @213:

```go
lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C, bs.Stats, sinks, httpReg, lfReg, drainMgr, httpClient, netReg)
```

Add imports: `network "github.com/esalaine/envoy-go/internal/filter/network"`, `"github.com/esalaine/envoy-go/internal/filter/network/echo"`, `"github.com/esalaine/envoy-go/internal/filter/network/directresponse"`.

- [ ] **Step 4: Verify build + vet + a manual boot smoke.**
Run: `go build ./... && go vet ./...`
Expected: PASS. Optionally run the binary against a tiny `[echo]` config and `nc` a payload (manual sanity; the live proof is Task 14).

- [ ] **Step 5: Commit.**

```bash
git add cmd/envoy-go/main.go
git commit -m "phase 26.1 Task 12: boot-wire *network.Registry (echo + direct_response) into manager [SPEC 3.6]"
```

---

## Task 13: 35th fuzzer `FuzzNetworkFilterConfigParse` (§11; R6)

**Files:**
- Create: `internal/filter/network/echo/fuzz_test.go`, `internal/filter/network/directresponse/fuzz_test.go` (per-filter fuzzers; the SPEC counts ONE new fuzzer `FuzzNetworkFilterConfigParse` — name the load-bearing one that, and a second mirror if both filters get one; confirm the count delta in Step 4).

Decision: land ONE fuzzer named `FuzzNetworkFilterConfigParse` covering BOTH filters (dispatch on a leading selector byte), to match the SPEC's "35th fuzzer" single-count. Place it in `internal/filter/network/` (a package-level `fuzz_test.go`) or in directresponse (the one with non-trivial parse). Recommended: `internal/filter/network/directresponse/fuzz_test.go` (echo's parse is vacuous), named `FuzzNetworkFilterConfigParse`, fuzzing direct_response's `New` (the parse with real surface). Re-confirm the count goes 34 → 35 in Step 4.

- [ ] **Step 1: Write the fuzzer** (mirror `internal/filter/http/buffer/fuzz_test.go`):

```go
func FuzzNetworkFilterConfigParse(f *testing.F) {
	seed := func(ds *corev3.DataSource) {
		b, _ := proto.Marshal(&drv3.Config{Response: ds})
		f.Add(b)
	}
	seed(&corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: "x"}})
	seed(&corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte{1}}})
	seed(nil)                  // absent response → reject
	f.Add([]byte{})           // empty
	f.Add([]byte{0xff})       // garbage
	f.Add([]byte("not-proto"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		any := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		fif, err := New(any, network.FactoryCtx{})
		if fif == nil && err == nil { t.Fatalf("New returned (nil, nil)") }
		if fif != nil && err != nil { t.Fatalf("New returned (factory, error): %v", err) }
	})
}
```

- [ ] **Step 2: Run it briefly → builds + no crash on seeds.**
Run: `go test ./internal/filter/network/directresponse/ -run FuzzNetworkFilterConfigParse -v`
Expected: PASS (seed corpus exercises parse).

- [ ] **Step 3: Short fuzz run** (smoke):
Run: `go test ./internal/filter/network/directresponse/ -run '^$' -fuzz FuzzNetworkFilterConfigParse -fuzztime 20s`
Expected: no new crashers.

- [ ] **Step 4: Re-confirm the count delta (deterministic, git-tracked — avoids the nested-worktree artifact).**
Run: `git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l`
Expected: `35` (34 → 35).

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/directresponse/fuzz_test.go
git commit -m "phase 26.1 Task 13: FuzzNetworkFilterConfigParse (35th fuzzer) [SPEC 11; R6]"
```

---

## Task 14: differential fixture `0040-network-echo` (cross-side) (§8.1)

The harness is **Docker-driven** (the real `fixture.Driver` interface, `test/differential/fixture/fixture.go:15-52`): a `driver` package self-registers via `init()`, is blank-imported into `runner_test.go`, and the runner runs each registered driver as the subtest `TestDifferential/<dir-name>` (skipped under `-short`, needs Docker via `ensureDocker`). Mirror `test/fixtures/0000-tcp-echo/driver/driver.go` (the raw-TCP cross-side template).

**Files:**
- Create: `test/fixtures/0040-network-echo/driver/driver.go` (`package driver`)
- Create: `test/fixtures/0040-network-echo/README.md`
- Modify: `test/differential/runner_test.go` (add the blank-import line)

- [ ] **Step 1: Write the driver + register it + blank-import it.** Implement the full 8-method `fixture.Driver`. echo has no upstream cluster, but the runner's `runFixture` fatals on `BackendCount() < 1` (`test/differential/runner_test.go` ~line 123) — so return `BackendCount() == 1` and simply ignore the spare TCP-echo backend the runner spawns (no zero-backend precedent exists; boot-reject fixture `0033` likewise returns `1` without round-tripping). Bootstraps are container-shaped (admin block; reference binds `0.0.0.0:15000`; subject binds `subjListenerPort` + admin `subjAdminPort`):

```go
// test/fixtures/0040-network-echo/driver/driver.go
package driver

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

func init() { fixture.RegisterFixture("0040-network-echo", driver{}) }

type driver struct{}

func (driver) BackendCount() int          { return 0 }
func (driver) SubjectListenerName() string { return "l_echo" }
func (driver) ReferenceListenerPort() int  { return 15000 }

func (driver) ReferenceBootstrap(_ []int) string { return bootstrap(15000, 0) }
func (driver) SubjectConfig(_ , subjListenerPort int, _ []int, subjAdminPort int) string {
	return bootstrap(subjListenerPort, subjAdminPort)
}

func bootstrap(listenerPort, adminPort int) string {
	return fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }
static_resources:
  listeners:
  - name: l_echo
    address:
      socket_address: { address: 0.0.0.0, port_value: %d }
    filter_chains:
    - filters:
      - name: envoy.filters.network.echo
        typed_config:
          "@type": type.googleapis.com/envoy.filters.network.echo.v3.Echo
`, adminPort, listenerPort)
}

func (driver) DriveReference(ctx context.Context, addr string) ([]byte, error) { return driveEcho(ctx, addr) }
func (driver) DriveSubject(ctx context.Context, addr string) ([]byte, error)   { return driveEcho(ctx, addr) }

func driveEcho(ctx context.Context, addr string) ([]byte, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil { return nil, fmt.Errorf("dial %s: %w", addr, err) }
	defer conn.Close()
	payload := []byte("alpha\nbeta-gamma\n") // exercises the OnData loop
	if _, err := conn.Write(payload); err != nil { return nil, fmt.Errorf("write: %w", err) }
	if err := conn.(*net.TCPConn).CloseWrite(); err != nil { return nil, fmt.Errorf("close-write: %w", err) }
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, buf); err != nil { return nil, fmt.Errorf("read: %w", err) }
	return buf, nil
}

func (driver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	return nil, nil, nil // echo fixture has no admin-probe assertion
}
```

Add the blank-import to `test/differential/runner_test.go`'s import block (alongside the existing `_ ".../0000-tcp-echo/driver"` lines):

```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0040-network-echo/driver"
```

Add `test/fixtures/0040-network-echo/README.md` (rationale; mirror `0000-tcp-echo/README.md`).

- [ ] **Step 2: Run → fails** (subject not yet echoing byte-identically, or driver not wired).
Run: `go test ./test/differential/ -run 'TestDifferential/0040-network-echo' -v`
Expected: FAIL (until the echo dispatch path from Tasks 7/10/11/12 is in place and byte-matches upstream). Note: needs Docker; NOT run under `-short`.

- [ ] **Step 3: Make it pass.** With echo (Task 7) + dual-dispatch (Tasks 10/11) + boot-wiring (Task 12) in place, the subject echoes byte-identically to reference Envoy v1.37.2.

- [ ] **Step 4: Run → passes byte-exact.**
Run: `go test ./test/differential/ -run 'TestDifferential/0040-network-echo' -v`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add test/fixtures/0040-network-echo/ test/differential/runner_test.go
git commit -m "phase 26.1 Task 14: differential fixture 0040-network-echo (cross-side) [SPEC 8.1]"
```

---

## Task 15: differential fixture `0041-network-direct-response` (cross-side) (§8.2)

**Files:**
- Create: `test/fixtures/0041-network-direct-response/driver/driver.go` (`package driver`)
- Create: `test/fixtures/0041-network-direct-response/README.md`
- Modify: `test/differential/runner_test.go` (add the blank-import line)

- [ ] **Step 1: Write the driver + register + blank-import.** Single `[direct_response]` chain with an `inline_string` response; `BackendCount()==0`; Drive connects, reads until EOF (server closes after `FlushWrite`):

```go
// test/fixtures/0041-network-direct-response/driver/driver.go
package driver

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

func init() { fixture.RegisterFixture("0041-network-direct-response", driver{}) }

type driver struct{}

func (driver) BackendCount() int           { return 0 }
func (driver) SubjectListenerName() string { return "l_dr" }
func (driver) ReferenceListenerPort() int  { return 15000 }
func (driver) ReferenceBootstrap(_ []int) string { return bootstrap(15000, 0) }
func (driver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return bootstrap(subjListenerPort, subjAdminPort)
}

func bootstrap(listenerPort, adminPort int) string {
	return fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }
static_resources:
  listeners:
  - name: l_dr
    address:
      socket_address: { address: 0.0.0.0, port_value: %d }
    filter_chains:
    - filters:
      - name: envoy.filters.network.direct_response
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config
          response: { inline_string: "envoy-go-direct-response\n" }
`, adminPort, listenerPort)
}

func (driver) DriveReference(ctx context.Context, addr string) ([]byte, error) { return driveRead(ctx, addr) }
func (driver) DriveSubject(ctx context.Context, addr string) ([]byte, error)   { return driveRead(ctx, addr) }

func driveRead(ctx context.Context, addr string) ([]byte, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil { return nil, fmt.Errorf("dial %s: %w", addr, err) }
	defer conn.Close()
	return io.ReadAll(conn) // static response, then server closes (FlushWrite)
}

func (driver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	return nil, nil, nil
}
```

Blank-import: `_ "github.com/esalaine/envoy-go/test/fixtures/0041-network-direct-response/driver"` into `runner_test.go`. Add `README.md`.

- [ ] **Step 2: Run → fails.** Run: `go test ./test/differential/ -run 'TestDifferential/0041-network-direct-response' -v` (Docker; not `-short`).

- [ ] **Step 3: Make it pass** (direct_response Task 8 + dual-dispatch Tasks 10/11 + boot-wiring Task 12 in place).

- [ ] **Step 4: Run → passes byte-exact.** Run: `go test ./test/differential/ -run 'TestDifferential/0041-network-direct-response' -v`

- [ ] **Step 5: Commit.**

```bash
git add test/fixtures/0041-network-direct-response/ test/differential/runner_test.go
git commit -m "phase 26.1 Task 15: differential fixture 0041-network-direct-response (cross-side) [SPEC 8.2]"
```

---

## Task 16: differential fixture `0042-network-direct-response-boot-reject` (boot-reject) (§8.3; D-P26.1-4)

The boot-reject driver implements `fixture.Driver` PLUS `harness.BootRejectFixture` (`test/differential/harness.go:340-352`), which requires **two** methods: `BootRejectScript() string` (return `""` — no Lua script here) and `ExpectedBootErrorSubstring() string`. The runner's boot-reject branch asserts both binaries exit non-zero at config-load and BOTH stderr buffers contain the substring (case-sensitive). Mirror `test/fixtures/0033-http-ratelimit-boot-reject/inputs/driver.go`. Per the `reference_differential_fixture_dispatch_constraint` memory, this is a SEPARATE dir from 0040/0041 (one dir = one runner branch).

**Files:**
- Create: `test/fixtures/0042-network-direct-response-boot-reject/inputs/driver.go` (`package driver`)
- Create: `test/fixtures/0042-network-direct-response-boot-reject/README.md`
- Modify: `test/differential/runner_test.go` (add the blank-import line)

- [ ] **Step 1: Empirically pin the boot-reject arm (D-P26.1-4).** Build both binaries; run each against a `direct_response` config whose `response.specifier` is unset (`response: {}`) and, separately, `response` absent; capture stderr:

```bash
go build -o /tmp/envoy-go ./cmd/envoy-go
/tmp/envoy-go -c /tmp/dr-unset.yaml 2>&1 | head -20      # subject
envoy -c /tmp/dr-unset.yaml 2>&1 | head -20              # reference v1.37.2 (per ADR-0008)
```

Identify a **case-sensitive** substring present in BOTH stderr streams (the `0033` "omain" precedent: upstream emits proto-camel-case, envoy-go emits wire-name; the shared fragment is the assertion). NOTE: the fixture runs the reference in Docker (`ensureDocker`), so the authoritative upstream substring is the **Dockerized v1.37.2** stderr — if you pin against a locally-installed `envoy`, re-confirm the fragment against the containerized binary the runner actually uses. If the natural envoy-go wording (Task 9 byte-stable const) shares no fragment with upstream's, adjust the Task-9 const so a common fragment exists, then re-pin. Record which arm (`specifier` unset vs `response` absent) gives the cleanest shared substring.

- [ ] **Step 2: Write the driver + register + blank-import, then run → fails.**

```go
// test/fixtures/0042-network-direct-response-boot-reject/inputs/driver.go
package driver

import (
	"context"
	"fmt"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

func init() { fixture.RegisterFixture("0042-network-direct-response-boot-reject", driver{}) }

type driver struct{}

func (driver) BackendCount() int           { return 0 }
func (driver) SubjectListenerName() string { return "l_dr" }
func (driver) ReferenceListenerPort() int  { return 15000 }
func (driver) ReferenceBootstrap(_ []int) string { return bootstrap(15000, 0) }
func (driver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return bootstrap(subjListenerPort, subjAdminPort)
}

func bootstrap(listenerPort, adminPort int) string {
	return fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }
static_resources:
  listeners:
  - name: l_dr
    address:
      socket_address: { address: 0.0.0.0, port_value: %d }
    filter_chains:
    - filters:
      - name: envoy.filters.network.direct_response
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config
          response: {}   # specifier unset → boot-reject on both sides (D-P26.1-4)
`, adminPort, listenerPort)
}

// boot-reject: Drive*/ProbeAdmin are never called (the runner asserts stderr).
func (driver) DriveReference(context.Context, string) ([]byte, error) { return nil, nil }
func (driver) DriveSubject(context.Context, string) ([]byte, error)   { return nil, nil }
func (driver) ProbeAdmin(context.Context, string, string) ([]byte, []byte, error) { return nil, nil, nil }

func (driver) BootRejectScript() string           { return "" } // no Lua script
func (driver) ExpectedBootErrorSubstring() string { return "<empirically-pinned-substring>" }
```

Blank-import: `_ "github.com/esalaine/envoy-go/test/fixtures/0042-network-direct-response-boot-reject/inputs"` into `runner_test.go`. Add `README.md` (mirror `0033` — document both stderr excerpts + the chosen substring).

Run: `go test ./test/differential/ -run 'TestDifferential/0042-network-direct-response-boot-reject' -v` → FAIL until the substring matches both binaries.

- [ ] **Step 3: Make it pass.** Set `ExpectedBootErrorSubstring()` to the pinned substring from Step 1; ensure Task-9 envoy-go wording shares it.

- [ ] **Step 4: Run → passes (both reject; substring present in both).**
Run: `go test ./test/differential/ -run 'TestDifferential/0042-network-direct-response-boot-reject' -v`

- [ ] **Step 5: Confirm fixture count + commit.**
Run: `ls test/fixtures/ | grep -E '^[0-9]' | wc -l` → expect `44`.

```bash
git add test/fixtures/0042-network-direct-response-boot-reject/ test/differential/runner_test.go
git commit -m "phase 26.1 Task 16: differential fixture 0042-direct-response-boot-reject [SPEC 8.3; D-P26.1-4]"
```

---

## Task 17: docs bundle (BEHAVIOR_CONTRACT + ADR-0213/0214 bodies) + STATE/ROADMAP advance + six-gate verification (§9 / §14 / §15)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the §9 4-edit bundle)
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0213 + ADR-0214 §Decision/§Consequences bodies — DECISIONS.md tail STAYS ADR-0214; no new number)
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`
- Modify: `docs/envoy-go/phases/26.1-network-filter-chain-framework-and-echo/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT.md** — add the 3 NEW subsections + forward-pointer note exactly as enumerated in SPEC §9 / §14 (framework subsection; `### envoy.filters.network.echo`; `### envoy.extensions.filters.network.direct_response`; the 26.2/26.3 forward-pointer). Fold in the envoy-go-strict departure records (write-filter absent; `network-filter-mixed-chain-unsupported` 26.1-transitional reject lifted at 26.2; the `DirectResponse` RCD string with no operator surface).

- [ ] **Step 2: DECISIONS.md** — fill the ADR-0213 §Decision/§Consequences (two-value Status protocol; ReadFilterCallbacks; connection-level buffering on StopIteration; single-goroutine-per-connection; per-connection runtime context; drainable Buffer; read-filter-ONLY + write-filter deferral + API-revision allowance) and ADR-0214 §Decision/§Consequences (registry mirroring ADR-0072/0079; new ctor arg; build-time dual-dispatch decision; planned 26.2 hardcoded-registry retirement). DECISIONS.md tail STAYS ADR-0214; next-free ADR-0215.

- [ ] **Step 3: Run the full six-gate suite + record outputs in PROGRESS.md** (per verification-before-completion):

```bash
go build ./... && go vet ./... && golangci-lint run && go test -race -short ./...
# differential suite is Docker-driven + skipped under -short — run it explicitly (not -short):
go test ./test/differential/ -run 'TestDifferential' -v   # 0040/0041/0042 green + all existing green (R4: 0000-tcp-echo/0002-tls-tcp/HCM byte-exact)
git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l   # 35 (34 + FuzzNetworkFilterConfigParse)
ls test/fixtures/ | grep -E '^[0-9]' | wc -l                        # 44
# stat surface golden → 132 (unchanged)
```

Expected: build/vet/lint/race-test PASS; differential `TestDifferential/0040…`/`0041…`/`0042…` + all existing PASS; fuzzers 35; fixtures 44; stats 132; conformance 10/10 + h2spec 53/53 unaffected. Quote every command's output into PROGRESS.md.

- [ ] **Step 4: STATE.md + ROADMAP.md advance.** STATE.md → "phase 26.1 phase-done; awaiting 26.2 SPEC"; next-skill `superpowers:brainstorming`/`writing-plans` for 26.2 SPEC; next-free ADR-0215; last-commit TBD-26.1-IMPL-SQUASH (SHA-filled at stage-close). ROADMAP sub-row 26.1 `in-progress → done`; parent row 26 STAYS `in-progress` (flips at 26.3 per the ROLLUP precedent).

- [ ] **Step 5: Commit.**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md docs/envoy-go/phases/26.1-network-filter-chain-framework-and-echo/PROGRESS.md
git commit -m "phase 26.1 Task 17: BEHAVIOR_CONTRACT + ADR-0213/0214 bodies + STATE/ROADMAP advance + six-gate verification [SPEC 9/14/15]"
```

---

## Acceptance (SPEC §15.3) — all must hold before requesting review

1. NEW `internal/filter/network/` lands with the §3.1 API + §3.2 file split (Tasks 2–6).
2. `echo` + `direct_response` land at upstream parity (Tasks 7–9).
3. Dual-dispatch in `manager.go` (Tasks 10–11); `tcp_proxy`/HCM fixtures stay byte-exact green (R4).
4. Boot-wiring in `main.go` (Task 12).
5. `ReadFilterCallbacks.DynamicMetadata()` + full L4 `Connection` accessor surface shaped + verified (R2); per-connection `*dynamicmetadata.Bucket` reused, no write at 26.1 (R5).
6. +3 differential fixtures (0040/0041/0042) byte-exact green; 35th fuzzer (R6); stat surface 132 (R7); fixtures 41 → 44 (Tasks 13–16).
7. ADR-0213 + ADR-0214 §Decision/§Consequences bodies land (tail STAYS ADR-0214); BEHAVIOR_CONTRACT 26.1 bundle (Task 17).
8. Six gates green; STATE advanced; ROADMAP 26.1 `→ done`; parent 26 stays `in-progress` (Task 17).

## Execution handoff

After this PLAN is reviewed + committed + squash-merged + pushed, the 26.1 IMPL runs via **superpowers:subagent-driven-development** (per the `feedback_execution_style` project memory): a fresh subagent per task, two-stage review between tasks, commits local-only (subagents do NOT push — `feedback_subagents_no_push`), controller pushes at stage-close.
