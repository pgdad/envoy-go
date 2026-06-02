# Phase 28.1 PLAN — the `network.WriteFilter` seam + `zookeeper_proxy` request side

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended, per `feedback_execution_style`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Subagents commit **LOCAL-ONLY** (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on the touched packages before its commit (`feedback_pertask_gofmt_lint`).

**Goal:** Land the `network.WriteFilter` seam (the framework's write-direction dispatch half — ADR-0221, consuming the ADR-0213 API-revision allowance) and the `internal/filter/network/zookeeperproxy/` package's REQUEST side (config parse + the 201-counter eager roster + the shallow request decoder + the correlation structures — ADR-0222), wired as the 7th built-in with the `.zookeeper.` Prometheus inline-prefix arm, proven by fixtures `0046-zookeeper-requests` (cross-side `StatsAsserter`) + `0047-zookeeper-boot-reject` and the 37th fuzzer.

**Architecture:** The existing `internal/filter/network/` package gains `WriteFilter`/`WriteFilterCallbacks` interfaces, classification restructured into independent type-asserts (a both-direction filter lands in BOTH the read and write sets — same instance), and a `writeChainConn` (NEW `writeconn.go`) that `handleTerminal` wraps OUTSIDE `prefixConn` iff the chain has ≥1 write filter — zero-write-filter chains stay UNWRAPPED (R1 byte-identical back-compat; `manager.go`/`tcp_proxy`/HCM untouched). A NEW `internal/filter/network/zookeeperproxy/` package implements BOTH `ReadFilter` and `WriteFilter` (one instance, both directions): 9-field config parse + proto→wire opcode mapping, the 201-counter EAGER roster under `<stat_prefix>.zookeeper.` (creation parity, D-P5), the shallow request decoder (decoder-internal reassembly + chain-buffer high-water mark + xid sniffing + opcode dispatch), the two correlation structures (written at 28.1, consumed at 28.2), and a PURE no-op `OnWrite` stub. Cross-side `StatsAsserter` per-opcode counter parity over a NEW silent `TCPSink` backend is the load-bearing differential proof.

**Tech stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy v1.37.2 (ADR-0008); go-control-plane v1.32.4 (ADR-0008). Reuses `internal/filter/network/` (26.1/26.2/27), `internal/stats/` (06.1; `NewCounterIfAbsent`), `internal/filter/tcpproxy/` (untouched terminal), the differential harness + `fixture.StatsAsserter`. ZERO new third-party `go.mod` dependencies (jute decode = `encoding/binary` big-endian reads).

---

## ADR-0045 split-gate FINAL re-check (D-P1; at PLAN time, per SPEC §10.1 + parent §3.0)

The gate fires at `> ~25 tasks OR > ~1500 net-new production LoC`. This PLAN decomposes to **18 tasks** / **~990–1410 production LoC** (the SPEC §10.1 envelope, re-confirmed at PLAN time on the 26.x accounting basis — fixture drivers + unit tests excluded):

| Unit | Production LoC | Tasks |
|---|---|---|
| WriteFilter seam (interfaces + classification + `writeconn.go` + `handleTerminal` wrap) | ~150–250 | 2–5 |
| zookeeperproxy `config.go` (parse + PGV-mirror rejects + proto→wire mapping) | ~150–200 | 6–7 |
| zookeeperproxy `stats.go` (201-suffix roster + eager creation + dynamic auth) | ~150–200 | 8 |
| zookeeperproxy `decoder.go` (framing + reassembly + dispatch + counters + correlation) | ~300–450 | 9–10 |
| zookeeperproxy filter glue + `doc.go` + TypeURL + factory | ~80–100 | 11 |
| builtins + `bootstrap.go` + `name.go` arm + `TCPSink` runner plumbing | ~100–150 | 12, 13, 15 |
| The 37th fuzzer | ~60 | 14 |
| **Total (production basis)** | **~990–1410** | **18** |

Both axes under the gate (18 ≤ ~25 tasks; ~1410 ≤ ~1500 LoC) → **NO split. 28.1 proceeds as ONE sub-phase.** The pre-authorized 28.1a/28.1b axis (SPEC §10.1) stays UNCONSUMED. The fixture drivers (`0046` ~600–800 LoC + `0047` ~200 LoC, the 637/220-LoC `0043`/`0044` precedents) are excluded per the 26.x/27 accounting precedent (the 27 PLAN excluded the 522-LoC `0045` driver).

## PLAN-time D-question dispositions (SPEC §12.2)

- **D-S28.1-3 (chain-buffer high-water-mark home) — RESOLVED at PLAN: the mark lives ON THE DECODER.** `requestDecoder.chainConsumed int`; `decodeOnData(chainBytes []byte)` receives the **FULL** chain-buffer contents (`buf.Bytes()`) on every call and internally processes only `chainBytes[chainConsumed:]`. Rationale: (i) ALL decode state lives in one type → the multi-read no-double-count invariant is unit-testable on the decoder alone (feed cumulative slices — no filter, no chain runtime needed); (ii) the filter glue (Task 11) stays stateless w.r.t. decoding (`OnData` just forwards `buf.Bytes()`); (iii) the fuzzer (Task 14) exercises the exact production entry point including the high-water-mark arithmetic. The dedicated multi-read no-double-count unit test lands at Task 9.
- **Task ordering** — the SPEC §10 spine order is kept verbatim: seam (Tasks 2–5) → zookeeperproxy core (6–11) → integration (12–13) → fuzzer (14) → fixtures (15–17) → completion (18). The seam lands FIRST so Task 11's both-directions filter glue and Task 12's boot smoke exercise the full read+write dispatch path, and so the R1 back-compat property (zero-write-filter chains unwrapped) is provable in isolation before any consumer exists.
- **TypeURL/skeleton placement** — the SPEC §10 spine's Task-6 "package skeleton + TypeURL + NewFactory + config parse" is SPLIT (the SPEC §10 lead-in permits merge/split): Task 6 lands `doc.go` + `config.go` (pure parse, no factory); Task 7 lands the PARSE-REJECT arms; Task 11 lands `zookeeperproxy.go` (TypeURL + `NewFactory` + the `filter` glue). Each intermediate task compiles + tests green standalone.
- **IMPL-owned D-questions left to their tasks** (per SPEC §12.2): D-S28.1-1 (per-opcode min-length table values — Task 10, transcribed from upstream `decoder.cc`), D-S28.1-2 (PARSE-REJECT byte-stable wording — Task 7), D-S28.1-4 (frame-builder shape — Task 15; anticipated: small builder helpers), D-S28.1-5 (TCPSink close semantics — Task 15; anticipated: read-until-EOF then close).

---

## File Structure

**Created:**
- `internal/filter/network/writeconn.go` — the `writeChainConn` (embeds `net.Conn`, overrides `Write` only; mirrors `prefixconn.go:12-28`).
- `internal/filter/network/writeconn_test.go` — forward / stop-no-forward / dispatch-order / mutating-filter / error-propagation / endStream-false unit tests.
- `internal/filter/network/zookeeperproxy/doc.go` — package doc (request side; ADR-0222; 28.2 forward-pointer).
- `internal/filter/network/zookeeperproxy/config.go` — `compiledConfig` + `parseConfig` (9 fields + PGV mirrors + the proto→wire opcode mapping + the wire-opcode→opname table + the latency-override map).
- `internal/filter/network/zookeeperproxy/config_test.go` — parse + defaults + mapping + reject-arm unit tests.
- `internal/filter/network/zookeeperproxy/stats.go` — `rosterSuffixes()` (the 201-suffix table) + `rosterStats` eager creation + the dynamic auth-scheme counter helper.
- `internal/filter/network/zookeeperproxy/stats_test.go` — `TestCounterRoster_MatchesUpstreamMacro` (R2 golden list) + eager/idempotent/dynamic-auth tests.
- `internal/filter/network/zookeeperproxy/decoder.go` — the shallow request decoder + the two correlation structures.
- `internal/filter/network/zookeeperproxy/decoder_test.go` — special-xid / data-request / reassembly / high-water-mark / decoder-error / flag-gating / correlation tests.
- `internal/filter/network/zookeeperproxy/zookeeperproxy.go` — `TypeURL` + `NewFactory(reg *stats.Registry)` + the both-directions `filter` glue.
- `internal/filter/network/zookeeperproxy/zookeeperproxy_test.go` — TypeURL pinning + factory + filter-glue + R3 chain-buffer-never-drained tests.
- `internal/filter/network/zookeeperproxy/fuzz_test.go` — `FuzzZookeeperRequestDecode` (the 37th fuzzer).
- `test/fixtures/0046-zookeeper-requests/driver/driver.go` + `README.md` — the cross-side 7-arm StatsAsserter fixture.
- `test/fixtures/0047-zookeeper-boot-reject/driver/driver.go` + `README.md` — the symmetric boot-reject fixture.

**Modified:**
- `internal/filter/network/types.go` — + `WriteFilter` + `WriteFilterCallbacks` interfaces (after `ReadFilter`, `types.go:29-48`).
- `internal/filter/network/chain.go` — `NewChainRuntime` classification restructure (`chain.go:57-83`); `chainRuntime` struct + `writeFilters` field (`chain.go:127-168`); the concrete `writeCallbacks` impl; `handleTerminal` wrap insertion (`chain.go:215-227`); `onDestroy` once-per-instance dedupe (`chain.go:321-326`).
- `internal/filter/network/chain_test.go` — classification / dual-injection / destroy-dedupe / wrap-composition / back-compat tests.
- `internal/stats/name.go` — the `.zookeeper.` INLINE-PREFIX arm (after the `.rbac.` arm at `name.go:226-242`, before the default error at `name.go:243`).
- `internal/stats/name_test.go` — the zookeeper flattening tests.
- `internal/filter/network/builtins/builtins.go` — the 7th registration (after `snicluster` at `builtins.go:62`); package doc 6 → 7.
- `internal/filter/network/builtins/builtins_test.go` — `TestRegisterBuiltinsRegistersAllSix` → AllSeven + the zookeeper registration test + boot smoke.
- `internal/bootstrap/bootstrap.go` — the `zookeeper_proxy/v3` blank-import (after `sni_cluster/v3` at `bootstrap.go:87`).
- `test/differential/fixture/fixture.go` — `TCPSink BackendKind = 28` (after `HTTPWasmPerRoute = 27` at `fixture.go:492`).
- `test/differential/runner_test.go` — the `case fixture.TCPSink:` backend arm + `acceptSinkCounting` + the `0046`/`0047` driver blank-imports.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` / `DECISIONS.md` / `STATE.md` / `ROADMAP.md` / `next-prompt.txt` — the completion bundle (Task 18).

**Untouched (pinned):** `internal/listener/manager.go` (§3.6 — its `case network.ReadFilter` boot arm accepts a both-direction filter with zero delta; write-only filters stay boot-rejected); `internal/filter/tcpproxy/`; `internal/filter/hcm/`; `terminal.go`; `callbacks.go`; `buffer.go`; `registry.go`; `prefixconn.go`; `upstreamcluster.go`.

---

## Task 1: First-action baselines/anchors gate (no code change)

**Files:** none modified — verification + re-pin gate run at the IMPL-session tip; record in PROGRESS.md (created this task).

- [ ] **Step 1: Re-confirm the project counts at the IMPL-session tip**

Run (from repo root):
```bash
git log --oneline -1
# fixtures (canonical recipe):
ls -d test/fixtures/[0-9]* | wc -l            # expect 47; tail dir:
ls -d test/fixtures/[0-9]* | tail -1          # expect test/fixtures/0045-sni-cluster
# fuzzers (canonical recipe — scoped to ./internal per parent §11.10 advisory):
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 36
# DECISIONS.md tail + next-free:
grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -3  # tail = ADR-0223 → next-free ADR-0224
```
Expected: fixtures **47** (tail `0045-sni-cluster`); fuzzers **36**; DECISIONS.md tail **ADR-0223** (next-free **ADR-0224**; the ADR-0221/0222/0223 §Context drafts landed at the parent SPEC — `DECISIONS.md:14226/:14245/:14264`). 28.1 lands `0046`+`0047` → 49, the 37th fuzzer, and the ADR-0221/0222 bodies IN PLACE (no new ADR number).

- [ ] **Step 2: Re-confirm the stat surface = 136**

Run the project's canonical stat-surface recipe (the same count STATE.md/BEHAVIOR_CONTRACT.md report as 136 — the BEHAVIOR_CONTRACT stat table row count; do NOT invent a new recipe). Expected: **136**. 28.1 lands +201 → **337** at Task 18.

- [ ] **Step 3: Re-confirm `proto.MessageName` (the TypeURL pin)**

```bash
cat > /tmp/zk_tu.go <<'EOF'
package main

import (
	"fmt"

	zkv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	"google.golang.org/protobuf/proto"
)

func main() { fmt.Println(proto.MessageName(&zkv3.ZooKeeperProxy{})) }
EOF
go run /tmp/zk_tu.go   # expect: envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy
```
Expected `proto.MessageName` = `envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy` (the `extensions.` segment per `reference_network_filter_typeurl_extensions`) → `TypeURL` = `type.googleapis.com/` + that. Also confirm the Go enum identifier spelling of the proto `LatencyThresholdOverride_Opcode` values (e.g. `zookeeper_proxyv3.LatencyThresholdOverride_Connect`) by listing them:
```bash
grep -oE "LatencyThresholdOverride_[A-Za-z0-9]+ LatencyThresholdOverride_Opcode = [0-9]+" \
  $(go env GOMODCACHE)/github.com/envoyproxy/go-control-plane/envoy@*/extensions/filters/network/zookeeper_proxy/v3/zookeeper_proxy.pb.go
```
Expected: 27 contiguous values 0..26 with digit-suffixed names intact (`Create2`/`GetChildren2`/`SetWatches2` — the `reference_proto_roster_extraction_digits` discipline). Record the exact Go identifiers for Task 6's mapping table.

- [ ] **Step 4: Re-confirm the as-built line anchors (drift here re-points later tasks)**

Confirm each §3/§4 anchor still holds (all verified at SPEC tip `2a525ff`; only docs-only commits land between sessions, but the gate catches drift): `types.go:29-48` (ReadFilter), `types.go:61` (FilterInstanceFactory → NetworkFilter), `terminal.go:18-28` (sealed marker + Marker), `terminal.go:42-49` (TerminalFilter.Handle), `chain.go:57-83` (NewChainRuntime classification switch), `chain.go:127-168` (chainRuntime struct), `chain.go:174-189` (newChainRuntime + read-callbacks injection at `:185-187`), `chain.go:215-227` (handleTerminal), `chain.go:321-326` (onDestroy), `chain.go:380-385` (Connection.Write — the D-P3 bypass), `prefixconn.go:12-28`, `callbacks.go:16-38` (ReadFilterCallbacks), `manager.go:534-599` (buildNetworkChainFactory) + `:570-581` (boot classification) + `:580` (write-only default reject), `name.go:88-122` (wasm permissive arm) + `:226-242` (rbac arm) + `:243` (default error), `builtins/builtins.go:44-63` (RegisterBuiltins; rbac closure-capture at `:59`; snicluster at `:62`), `bootstrap.go:76-87` (network-filter blank-imports), `rbac/rbac.go:38` (TypeURL via proto.MessageName) + `:187-198` (newFilterStats), `registry.go:157-171` (NewCounterIfAbsent), `fixture.go:125/:129/:492/:495-499` (BackendKind/TCPEcho=0/HTTPWasmPerRoute=27/BackendKindAware), `fixture.go:75-77` (StatsAsserter), `fixture.go:584-589` (MultiListenerDriver), `runner_test.go:98` (TestDifferential), `:150` (the backend-kind switch), `:1048-1050` (StatsAsserter cross-side dispatch), `:1219` (acceptEchoCounting), `harness.go:340-352` (BootRejectFixture), `0043/driver/driver.go:328-376` (AssertStats) + `:388-461` (scrape/parse helpers), `0044/driver/driver.go:159/:163` (BootRejectScript/ExpectedBootErrorSubstring).

- [ ] **Step 5: Commit the gate record**

```bash
git add docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md
git commit -m "phase 28.1 Task 1: baselines/anchors gate — fixtures 47 (tail 0045), fuzzers 36, stats 136, DECISIONS tail ADR-0223 (next-free 0224); proto.MessageName + line anchors re-pinned"
```

---

## Task 2: `WriteFilter` + `WriteFilterCallbacks` interfaces + the concrete `writeCallbacks`

**Files:**
- Modify: `internal/filter/network/types.go` (additions after `ReadFilter`, `:29-48`)
- Modify: `internal/filter/network/chain.go` (the concrete `writeCallbacks` near `callbacks`/`connection` impls)
- Test: `internal/filter/network/chain_test.go` (white-box, `package network`)

- [ ] **Step 1: Write the failing tests**

In `chain_test.go`, add the synthetic write-filter double (reused by Tasks 3–5) and the writeCallbacks accessor test:
```go
// fakeWriteFilter is a synthetic WriteFilter recording OnWrite calls + injections.
// status controls the per-call return; calls records the buffer contents seen.
type fakeWriteFilter struct {
	Marker
	name      string
	status    Status
	calls     []string
	wcb       WriteFilterCallbacks
	wcbCalls  int
	destroyed int
}

func (f *fakeWriteFilter) OnWrite(buf *Buffer, _ bool) Status {
	f.calls = append(f.calls, f.name+":"+string(buf.Bytes()))
	return f.status
}
func (f *fakeWriteFilter) SetWriteFilterCallbacks(cb WriteFilterCallbacks) { f.wcb = cb; f.wcbCalls++ }
func (f *fakeWriteFilter) OnDestroy()                                     { f.destroyed++ }

// The concrete writeCallbacks Connection() must return the SAME per-connection
// accessor the read callbacks return (SPEC §3.1 — one connection, two views).
func TestWriteCallbacksConnectionAccessor(t *testing.T) {
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	wcb := &writeCallbacks{rt: rt}
	if wcb.Connection() != Connection(rt.cxn) {
		t.Fatal("writeCallbacks.Connection() != rt.cxn (must be the same accessor as read callbacks)")
	}
}
```
(Reuse the existing `fakeConn` at `chain_test.go:161`. If `newChainRuntime(nil, …)` panics on nil filters, pass `[]ReadFilter{}` — match the existing tests' construction.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run TestWriteCallbacksConnectionAccessor -v`
Expected: FAIL — compile error: `WriteFilterCallbacks`, `writeCallbacks` undefined.

- [ ] **Step 3: Add the interfaces to `types.go`**

Append after the `ReadFilter` interface (`types.go:48`), the SPEC §3.1 production signatures VERBATIM:
```go
// WriteFilter is the per-connection write-filter interface — the write-direction
// (upstream→downstream) half of the chain. Mirrors upstream Network::WriteFilter
// (onWrite + initializeWriteFilterCallbacks) + the envoy-go OnDestroy convention.
// A filter may implement ReadFilter, WriteFilter, or BOTH (one instance seeing
// both directions — upstream Network::Filter / addFilter parity, AMEND-A11).
//
//nolint:revive // ADR-0221 reserves the network.WriteFilter name for the write-direction dispatch surface.
type WriteFilter interface {
	NetworkFilter
	// OnWrite is called with the bytes a terminal filter writes downstream,
	// BEFORE they reach the downstream socket, in REVERSE chain order
	// (AMEND-A11 LIFO parity). Return Continue to forward; StopIteration to
	// stop the write (the bytes are NOT forwarded — upstream no-forward parity;
	// documented-unsupported-by-consumers at 28.x).
	OnWrite(buf *Buffer, endStream bool) Status
	// SetWriteFilterCallbacks injects the per-connection write callbacks. Called
	// once by the chain runtime at construction, before any OnWrite.
	SetWriteFilterCallbacks(cb WriteFilterCallbacks)
	// OnDestroy releases per-connection resources. For a filter implementing
	// BOTH ReadFilter and WriteFilter this is the SAME method (one instance);
	// the chain runtime calls OnDestroy exactly ONCE per filter instance.
	OnDestroy()
}

// WriteFilterCallbacks is the per-connection callback surface handed to each
// WriteFilter via SetWriteFilterCallbacks. Minimal at 28.1 (D-P2): zookeeper
// needs only the connection accessor. Upstream's WriteFilterCallbacks carries
// injectWriteDataToFilterChain + disableClose — both deferred under the
// ADR-0213/0221 API-revision allowance until a consumer needs them.
//
//nolint:revive // ADR-0221 reserves the network.WriteFilterCallbacks name.
type WriteFilterCallbacks interface {
	// Connection returns the per-connection accessor (the same concrete impl
	// the ReadFilterCallbacks Connection() returns).
	Connection() Connection
}
```

- [ ] **Step 4: Add the concrete `writeCallbacks` to `chain.go`**

After the existing `*callbacks` methods (the read-callbacks concrete impl):
```go
// writeCallbacks is the concrete WriteFilterCallbacks injected into every
// WriteFilter at chain construction (ADR-0221; D-P2 — a both-directions filter
// receives BOTH a *callbacks and a *writeCallbacks injection). Connection()
// returns the SAME per-connection accessor the read callbacks expose.
type writeCallbacks struct {
	rt *chainRuntime
}

func (w *writeCallbacks) Connection() Connection { return w.rt.cxn }
```

- [ ] **Step 5: Run the test + the package suite**

Run: `go test ./internal/filter/network/ -run TestWriteCallbacksConnectionAccessor -v` → PASS.
Run: `go test ./internal/filter/network/ -race -short` → PASS (additive interfaces; no existing consumer breaks — `WriteFilter` is satisfied by NO existing type, so no double needs updating).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/types.go internal/filter/network/chain.go internal/filter/network/chain_test.go
git commit -m "phase 28.1 Task 2: WriteFilter + WriteFilterCallbacks interfaces + concrete writeCallbacks (ADR-0221 §3.1)"
```

---

## Task 3: Chain classification restructure (read/write/both/terminal) + dual injection + OnDestroy dedupe

**Files:**
- Modify: `internal/filter/network/chain.go:57-83` (`NewChainRuntime`), `:127-168` (`chainRuntime` struct), `:321-326` (`onDestroy`)
- Test: `internal/filter/network/chain_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// fakeBothFilter implements BOTH ReadFilter and WriteFilter (the zookeeperproxy
// shape — one instance, both directions; upstream addFilter parity).
type fakeBothFilter struct {
	Marker
	rcb       ReadFilterCallbacks
	wcb       WriteFilterCallbacks
	rcbCalls  int
	wcbCalls  int
	destroyed int
}

func (f *fakeBothFilter) OnNewConnection() Status                        { return Continue }
func (f *fakeBothFilter) OnData(*Buffer, bool) Status                    { return Continue }
func (f *fakeBothFilter) SetReadFilterCallbacks(cb ReadFilterCallbacks)  { f.rcb = cb; f.rcbCalls++ }
func (f *fakeBothFilter) OnWrite(*Buffer, bool) Status                   { return Continue }
func (f *fakeBothFilter) SetWriteFilterCallbacks(cb WriteFilterCallbacks) { f.wcb = cb; f.wcbCalls++ }
func (f *fakeBothFilter) OnDestroy()                                     { f.destroyed++ }

// A both-directions filter lands in BOTH the read and write sets — SAME instance.
func TestClassificationBothDirectionsFilter(t *testing.T) {
	both := &fakeBothFilter{}
	term := &recordingTerminal{}
	crt := NewChainRuntime([]NetworkFilter{both, term}, &fakeConn{}, ConnFacts{})
	rt := crt.rt
	if len(rt.filters) != 1 || rt.filters[0].(*fakeBothFilter) != both {
		t.Fatalf("read set = %v, want [both]", rt.filters)
	}
	if len(rt.writeFilters) != 1 || rt.writeFilters[0].(*fakeBothFilter) != both {
		t.Fatalf("write set = %v, want [both]", rt.writeFilters)
	}
}

// A write-only filter lands ONLY in the write set (framework-level; boot still
// rejects it — manager.go untouched, SPEC §3.6).
func TestClassificationWriteOnlyFilter(t *testing.T) {
	wf := &fakeWriteFilter{name: "w", status: Continue}
	crt := NewChainRuntime([]NetworkFilter{wf}, &fakeConn{}, ConnFacts{})
	rt := crt.rt
	if len(rt.filters) != 0 {
		t.Fatalf("read set = %v, want empty", rt.filters)
	}
	if len(rt.writeFilters) != 1 {
		t.Fatalf("write set len = %d, want 1", len(rt.writeFilters))
	}
}

// Both-directions filter receives BOTH callback injections, each exactly once (D-P2).
func TestBothFilterDualCallbackInjection(t *testing.T) {
	both := &fakeBothFilter{}
	NewChainRuntime([]NetworkFilter{both}, &fakeConn{}, ConnFacts{})
	if both.rcbCalls != 1 || both.wcbCalls != 1 {
		t.Fatalf("injections (read=%d, write=%d), want (1, 1)", both.rcbCalls, both.wcbCalls)
	}
	if both.rcb == nil || both.wcb == nil {
		t.Fatal("callbacks not stored")
	}
	if both.wcb.Connection() != both.rcb.Connection() {
		t.Fatal("write and read callbacks must expose the SAME Connection accessor")
	}
}

// OnDestroy runs exactly ONCE per instance for a both-directions filter (D-P2 dedupe);
// read-only and write-only filters each get exactly one call too.
func TestOnDestroyOncePerInstance(t *testing.T) {
	both := &fakeBothFilter{}
	ro := &filterA{}              // existing read-only double (chain_test.go:182)
	wo := &fakeWriteFilter{name: "w", status: Continue}
	crt := NewChainRuntime([]NetworkFilter{ro, both, wo}, &fakeConn{}, ConnFacts{})
	crt.rt.onDestroy()
	if both.destroyed != 1 {
		t.Fatalf("both-directions filter destroyed %d times, want exactly 1", both.destroyed)
	}
	if wo.destroyed != 1 {
		t.Fatalf("write-only filter destroyed %d times, want 1", wo.destroyed)
	}
}
```
(Adapt the read-only double + destroy-count plumbing to the existing `filterA`/`destroyFilter` doubles at `chain_test.go:182/:458` — reuse whichever already counts `OnDestroy` calls; the existing `TestChainOnDestroyCallsAllFilters` must STAY GREEN.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run 'Classification|DualCallback|OnDestroyOnce' -v`
Expected: FAIL — compile error: `rt.writeFilters` undefined.

- [ ] **Step 3: Restructure `NewChainRuntime` + add the `writeFilters` field**

In `chain.go`, replace the `NewChainRuntime` type-switch (`:62-74`) with independent type-asserts (SPEC §3.3 verbatim) and add the write-set wiring:
```go
func NewChainRuntime(filters []NetworkFilter, conn net.Conn, facts ConnFacts) *ChainRuntime {
	var (
		read     []ReadFilter
		write    []WriteFilter // CHAIN order; dispatch reverses (handleTerminal — AMEND-A11)
		terminal TerminalFilter
	)
	for _, f := range filters {
		// Independent type-asserts (NOT a type-switch): a filter implementing
		// BOTH ReadFilter and WriteFilter must land in BOTH sets — the SAME
		// instance (upstream addFilter parity; D-P2). zookeeperproxy (28.1) is
		// the first such filter. A type-switch's first-match-wins cannot express this.
		if t, ok := f.(TerminalFilter); ok {
			terminal = t // boot validation (manager.go) guarantees uniqueness + last position
			continue
		}
		if rf, ok := f.(ReadFilter); ok {
			read = append(read, rf)
		}
		if wf, ok := f.(WriteFilter); ok {
			write = append(write, wf)
		}
		// A NetworkFilter that is neither (sealed-marker-only) is defensively
		// ignored, exactly as today.
	}
	rt := newChainRuntime(read, conn, connFacts{
		serverName: facts.ServerName,
		principals: facts.Principals,
		local:      facts.Local,
		remote:     facts.Remote,
	})
	rt.terminal = terminal
	rt.writeFilters = write
	// Write-callbacks injection (mirrors the read-callbacks loop in
	// newChainRuntime, chain.go:185-187): every WriteFilter receives
	// SetWriteFilterCallbacks exactly once at construction. A both-directions
	// filter therefore receives BOTH injections (D-P2).
	for _, wf := range write {
		wf.SetWriteFilterCallbacks(&writeCallbacks{rt: rt})
	}
	return &ChainRuntime{rt: rt}
}
```
Add to the `chainRuntime` struct (after the `terminal` field, `:129`):
```go
	// writeFilters is the write-direction half of the chain in CHAIN order
	// (ADR-0221). handleTerminal hands a REVERSED copy (dispatch order) to the
	// writeChainConn (AMEND-A11 LIFO parity). A both-directions filter appears
	// here AND in filters — the same instance.
	writeFilters []WriteFilter
```
(Keep the inner `newChainRuntime` signature unchanged — read filters only; the write set is attached by `NewChainRuntime` after construction, exactly as `terminal` is today.)

- [ ] **Step 4: `onDestroy` once-per-instance dedupe**

Replace `onDestroy` (`chain.go:321-326`):
```go
func (rt *chainRuntime) onDestroy() {
	// Once-per-instance dedupe (D-P2): a both-directions filter appears in BOTH
	// rt.filters and rt.writeFilters as the SAME instance; its OnDestroy must run
	// exactly once. Filter instances are pointers (interface identity comparison
	// is well-defined), hence usable as map keys.
	destroyed := make(map[NetworkFilter]bool, len(rt.filters)+len(rt.writeFilters))
	for _, f := range rt.filters {
		if !destroyed[f] {
			destroyed[f] = true
			f.OnDestroy()
		}
	}
	for _, f := range rt.writeFilters {
		if !destroyed[f] {
			destroyed[f] = true
			f.OnDestroy()
		}
	}
	rt.bucket.Reset()
}
```

- [ ] **Step 5: Run the tests + the package suite**

Run: `go test ./internal/filter/network/ -run 'Classification|DualCallback|OnDestroyOnce' -v` → PASS.
Run: `go test ./internal/filter/network/ -race -short` → ALL existing tests PASS (the classification restructure is behavior-preserving for read/terminal-only chains: `TestChainOnDestroyCallsAllFilters`, `TestPureTerminalImmediateHandoff`, `TestMixedChainBufferedPrefixHandoff`, the sticky-halt tests, etc. all stay green).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/chain.go internal/filter/network/chain_test.go
git commit -m "phase 28.1 Task 3: chain classification restructure (read/write/both/terminal) + writeFilters field + dual injection + OnDestroy once-per-instance (ADR-0221 §3.3)"
```

---

## Task 4: `writeconn.go` — the `writeChainConn`

**Files:**
- Create: `internal/filter/network/writeconn.go`
- Create: `internal/filter/network/writeconn_test.go`

- [ ] **Step 1: Write the failing tests**

`writeconn_test.go` (`package network`):
```go
// recordingConn is a net.Conn double recording Write payloads; failErr forces
// Write to fail (error-propagation test).
type recordingConn struct {
	net.Conn // nil; only Write is exercised
	writes   [][]byte
	failErr  error
}

func (r *recordingConn) Write(p []byte) (int, error) {
	if r.failErr != nil {
		return 0, r.failErr
	}
	cp := make([]byte, len(p))
	copy(cp, p)
	r.writes = append(r.writes, cp)
	return len(p), nil
}

// All-Continue chain: bytes forward to the inner conn; Write reports (len(p), nil).
func TestWriteChainConnForwards(t *testing.T) {
	inner := &recordingConn{}
	a := &fakeWriteFilter{name: "a", status: Continue}
	w := newWriteChainConn(inner, []WriteFilter{a})
	n, err := w.Write([]byte("hello"))
	if n != 5 || err != nil {
		t.Fatalf("Write = (%d, %v), want (5, nil)", n, err)
	}
	if len(inner.writes) != 1 || string(inner.writes[0]) != "hello" {
		t.Fatalf("inner writes = %q, want [hello]", inner.writes)
	}
	if len(a.calls) != 1 {
		t.Fatalf("OnWrite calls = %d, want 1", len(a.calls))
	}
}

// StopIteration: nothing reaches the inner conn; Write STILL reports (len(p), nil) — D-P7.
func TestWriteChainConnStopIterationNoForward(t *testing.T) {
	inner := &recordingConn{}
	stop := &fakeWriteFilter{name: "stop", status: StopIteration}
	after := &fakeWriteFilter{name: "after", status: Continue}
	w := newWriteChainConn(inner, []WriteFilter{stop, after})
	n, err := w.Write([]byte("dropme"))
	if n != 6 || err != nil {
		t.Fatalf("Write = (%d, %v), want (6, nil) — D-P7 chain-stop reports success", n, err)
	}
	if len(inner.writes) != 0 {
		t.Fatalf("inner received %q, want nothing (no-forward parity)", inner.writes)
	}
	if len(after.calls) != 0 {
		t.Fatal("filters after the stopping filter must NOT be invoked")
	}
}

// Dispatch order: writeChainConn iterates its slice front-to-back (the slice IS
// dispatch order — handleTerminal reverses chain order before constructing).
func TestWriteChainConnDispatchOrder(t *testing.T) {
	var order []string
	mk := func(name string) *fakeWriteFilter { return &fakeWriteFilter{name: name, status: Continue} }
	a, b := mk("a"), mk("b")
	w := newWriteChainConn(&recordingConn{}, []WriteFilter{a, b})
	_, _ = w.Write([]byte("x"))
	order = append(order, a.calls...)
	order = append(order, b.calls...)
	if len(a.calls) != 1 || len(b.calls) != 1 {
		t.Fatalf("calls a=%d b=%d, want 1 each (order asserted via call recording)", len(a.calls), len(b.calls))
	}
	_ = order // a's call must precede b's: assert via a shared recorder if the doubles support it
}

// Post-chain bytes: a mutating write filter's output is what reaches the inner
// conn (upstream parity: the transport sees the filtered buffer); the Write
// return value still reports len(p) of the ORIGINAL payload.
func TestWriteChainConnForwardsPostChainBytes(t *testing.T) {
	inner := &recordingConn{}
	mut := &mutatingWriteFilter{}
	w := newWriteChainConn(inner, []WriteFilter{mut})
	n, err := w.Write([]byte("abc"))
	if n != 3 || err != nil {
		t.Fatalf("Write = (%d, %v), want (3, nil)", n, err)
	}
	if len(inner.writes) != 1 || string(inner.writes[0]) != "abcXYZ" {
		t.Fatalf("inner writes = %q, want [abcXYZ] (post-chain buffer)", inner.writes)
	}
}

// Underlying-write errors propagate as (0, err).
func TestWriteChainConnUnderlyingErrorPropagates(t *testing.T) {
	wantErr := errors.New("boom")
	inner := &recordingConn{failErr: wantErr}
	w := newWriteChainConn(inner, []WriteFilter{&fakeWriteFilter{name: "a", status: Continue}})
	n, err := w.Write([]byte("x"))
	if n != 0 || !errors.Is(err, wantErr) {
		t.Fatalf("Write = (%d, %v), want (0, boom)", n, err)
	}
}

// endStream is always false at 28.1 (net.Conn.Write carries no half-close signal).
func TestWriteChainConnEndStreamAlwaysFalse(t *testing.T) {
	got := make([]bool, 0, 1)
	rec := &endStreamRecorder{sink: &got}
	w := newWriteChainConn(&recordingConn{}, []WriteFilter{rec})
	_, _ = w.Write([]byte("x"))
	if len(got) != 1 || got[0] != false {
		t.Fatalf("endStream values = %v, want [false]", got)
	}
}
```
Add the two extra synthetic doubles (`mutatingWriteFilter` appends "XYZ" to the buffer in OnWrite via `buf.Append([]byte("XYZ"))`; `endStreamRecorder` records the endStream argument). Use a shared call-order recorder (e.g. a `*[]string` field on `fakeWriteFilter` appended as `name`) to make `TestWriteChainConnDispatchOrder` assert strict ordering — extend `fakeWriteFilter` from Task 2 with an optional `order *[]string` field rather than introducing a new double.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run TestWriteChainConn -v`
Expected: FAIL — compile error: `newWriteChainConn` undefined.

- [ ] **Step 3: Create `writeconn.go`**

The SPEC §3.5 code verbatim:
```go
// internal/filter/network/writeconn.go — write-chain conn for terminal handoff
// (ADR-0221). Mirrors prefixconn.go's embed-and-override-one-method shape.

package network

import "net"

// writeChainConn runs the write-filter chain over every terminal-originated
// downstream write BEFORE forwarding to the wrapped conn. Mirrors upstream
// ConnectionImpl::write → filter_manager_.onWrite() → transport (AMEND-A11).
// All non-Write methods promote from the embedded net.Conn (so Read still
// reaches a wrapped prefixConn's buffered-prefix replay).
type writeChainConn struct {
	net.Conn               // the wrapped conn (prefixConn or the raw downstream conn)
	filters []WriteFilter  // DISPATCH order (already reversed by handleTerminal)
}

func newWriteChainConn(c net.Conn, dispatch []WriteFilter) *writeChainConn {
	return &writeChainConn{Conn: c, filters: dispatch}
}

// Write runs the write chain (dispatch order) over p, then forwards the
// POST-CHAIN buffer to the wrapped conn (upstream parity: write filters may
// mutate; the transport sees the filtered buffer — at 28.x no production filter
// mutates, so the bytes are identical). Return semantics (D-P7):
//   - chain-stopped (StopIteration): (len(p), nil) — the terminal cannot
//     distinguish a stopped write from a delivered one, exactly as upstream
//     where ConnectionImpl::write returns void. A dropped write surfaces as
//     downstream silence.
//   - underlying-write error: (0, err) — the terminal's existing downstream
//     write-error handling sees it exactly as it would from the raw conn.
//     (Byte-count fidelity under mutation is moot at 28.x — no filter mutates.)
//   - success: (len(p), nil).
func (w *writeChainConn) Write(p []byte) (int, error) {
	buf := &Buffer{}
	buf.Append(p)
	for _, f := range w.filters {
		if f.OnWrite(buf, false) == StopIteration {
			// Upstream parity: the write does not proceed; nothing reaches the
			// transport. The terminal cannot distinguish (D-P7): report success.
			return len(p), nil
		}
	}
	if _, err := w.Conn.Write(buf.Bytes()); err != nil {
		return 0, err
	}
	return len(p), nil
}
```
(Per-Write allocation posture: one `*Buffer` + one `Append` copy per Write — accepted at 28.1, SPEC §3.5 item 6; pooling is a non-goal.)

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/filter/network/ -run TestWriteChainConn -v` → all PASS.
Run: `go test ./internal/filter/network/ -race -short` → PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/writeconn.go internal/filter/network/writeconn_test.go internal/filter/network/chain_test.go
git commit -m "phase 28.1 Task 4: writeChainConn — forward / StopIteration-no-forward / post-chain-bytes / error-propagation / endStream-false (ADR-0221 §3.5, D-P7)"
```

---

## Task 5: `handleTerminal` wrap insertion + back-compat

**Files:**
- Modify: `internal/filter/network/chain.go:215-227` (`handleTerminal`)
- Test: `internal/filter/network/chain_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// Zero-write-filter chains get NO writeChainConn wrap (R1 back-compat): the
// terminal receives the raw conn (or prefixConn) — never a writeChainConn.
func TestHandleTerminalZeroWriteFiltersUnwrapped(t *testing.T) {
	rec := &recordingTerminal{} // existing double (terminal ctx/conn recorder)
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.handleTerminal(context.Background())
	if _, isWrap := rec.gotConn.(*writeChainConn); isWrap {
		t.Fatal("zero-write-filter chain must NOT wrap the terminal conn (R1 back-compat)")
	}
}

// ≥1 write filter: the terminal receives a writeChainConn; composition is
// writeChainConn(prefixConn(conn)) when a buffered prefix exists — prefixConn
// INNER so reads still replay the prefix.
func TestHandleTerminalWrapComposition(t *testing.T) {
	rec := &recordingTerminal{}
	wf := &fakeWriteFilter{name: "w", status: Continue}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{wf}
	rt.buf.Append([]byte("prefix")) // simulate undrained buffered prefix
	rt.handleTerminal(context.Background())
	wrap, ok := rec.gotConn.(*writeChainConn)
	if !ok {
		t.Fatalf("terminal conn = %T, want *writeChainConn", rec.gotConn)
	}
	if _, ok := wrap.Conn.(*prefixConn); !ok {
		t.Fatalf("writeChainConn wraps %T, want *prefixConn (prefix INNER)", wrap.Conn)
	}
	// Reads promote through writeChainConn to the prefix replay:
	got := make([]byte, 6)
	n, _ := wrap.Read(got)
	if string(got[:n]) != "prefix" {
		t.Fatalf("Read through writeChainConn = %q, want prefix replay", got[:n])
	}
}

// Write dispatch through handleTerminal is REVERSE chain order (AMEND-A11):
// chain [A, B] ⇒ terminal write dispatch B → A.
func TestHandleTerminalReverseWriteDispatch(t *testing.T) {
	var order []string
	mk := func(name string) *fakeWriteFilter {
		return &fakeWriteFilter{name: name, status: Continue, order: &order}
	}
	a, b := mk("A"), mk("B")
	rec := &recordingTerminal{}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{a, b} // CHAIN order
	rt.handleTerminal(context.Background())
	_, _ = rec.gotConn.Write([]byte("x"))
	if len(order) != 2 || order[0] != "B" || order[1] != "A" {
		t.Fatalf("write dispatch order = %v, want [B A] (reverse chain order)", order)
	}
}

// The chain-order slice on the runtime is NOT mutated by the reversal (the
// dispatch slice is a copy).
func TestHandleTerminalDoesNotMutateChainOrder(t *testing.T) {
	a := &fakeWriteFilter{name: "A", status: Continue}
	b := &fakeWriteFilter{name: "B", status: Continue}
	rec := &recordingTerminal{}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{a, b}
	rt.handleTerminal(context.Background())
	if rt.writeFilters[0].(*fakeWriteFilter).name != "A" {
		t.Fatal("handleTerminal mutated chainRuntime.writeFilters (must reverse a COPY)")
	}
}
```
(Extend the existing `recordingTerminal` double — 27 PLAN Task 3 added it with `gotCtx`; add a `gotConn net.Conn` field, or add it if the as-built double lacks one. Extend `fakeWriteFilter` with the shared `order *[]string` recorder from Task 4.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run TestHandleTerminal -v`
Expected: the new tests FAIL (no wrap exists yet — `rec.gotConn` is never a `*writeChainConn`; the reverse-dispatch test gets no OnWrite calls). The EXISTING `TestHandleTerminalThreadsOverrideWhenSet`/`TestHandleTerminalNoOverrideLeavesCtxClean` (27) must still PASS.

- [ ] **Step 3: Insert the wrap in `handleTerminal`**

Modify `handleTerminal` (`chain.go:215-227`) — insert the writeChainConn wrap AFTER the prefixConn wrap, BEFORE the override ctx-threading:
```go
func (rt *chainRuntime) handleTerminal(ctx context.Context) {
	conn := rt.conn
	if rt.buf.Len() > 0 {
		prefix := make([]byte, rt.buf.Len())
		copy(prefix, rt.buf.Bytes())
		rt.buf.Drain(rt.buf.Len())
		conn = newPrefixConn(rt.conn, prefix)
	}
	// WriteFilter seam (ADR-0221): wrap the conn handed to the terminal in a
	// writeChainConn IFF the chain has ≥1 write filter, so terminal-originated
	// downstream writes run the write chain BEFORE reaching the socket.
	// Composition: writeChainConn OUTER, prefixConn INNER (reads promote through
	// to the buffered-prefix replay; writes run the chain then hit the inner
	// conn). Zero-write-filter chains get NO wrap → byte-identical to the
	// pre-28.1 path (R1 back-compat over all 47 existing fixtures).
	// The dispatch slice is a REVERSED COPY of the chain-order writeFilters
	// (AMEND-A11 LIFO parity: config [A, B, C] ⇒ write dispatch C → B → A).
	if len(rt.writeFilters) > 0 {
		dispatch := make([]WriteFilter, len(rt.writeFilters))
		for i, wf := range rt.writeFilters {
			dispatch[len(rt.writeFilters)-1-i] = wf
		}
		conn = newWriteChainConn(conn, dispatch)
	}
	if rt.upstreamClusterOverride != "" {
		ctx = withUpstreamClusterOverride(ctx, rt.upstreamClusterOverride)
	}
	rt.terminal.Handle(ctx, conn)
}
```

- [ ] **Step 4: Run the tests + the full package suite**

Run: `go test ./internal/filter/network/ -run TestHandleTerminal -v` → all PASS (new + the existing 27 override-threading tests).
Run: `go test ./internal/filter/network/ -race -short` → ALL PASS — the zero-write-filter back-compat property means every existing chain test (pure-read, pure-terminal, mixed, override) is unaffected.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/chain.go internal/filter/network/chain_test.go
git commit -m "phase 28.1 Task 5: handleTerminal writeChainConn wrap (prefixConn-inner composition; reverse dispatch; zero-write-filter unwrapped — R1) (ADR-0221 §3.4/§3.5)"
```

---
## Task 6: `zookeeperproxy` package skeleton + config parse (9 fields + the proto→wire opcode mapping)

**Files:**
- Create: `internal/filter/network/zookeeperproxy/doc.go`
- Create: `internal/filter/network/zookeeperproxy/config.go`
- Create: `internal/filter/network/zookeeperproxy/config_test.go`

- [ ] **Step 1: Write the failing tests**

`config_test.go` (`package zookeeperproxy`). Cover: the happy-path 9-field parse, the defaults, and the proto→wire mapping (the reject arms are Task 7):
```go
func TestParseConfig_AllFieldsAndDefaults(t *testing.T) {
	// Minimal config: only stat_prefix → all defaults.
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk"})
	if err != nil {
		t.Fatalf("parseConfig minimal: %v", err)
	}
	if cfg.statPrefix != "zk" {
		t.Errorf("statPrefix = %q, want zk", cfg.statPrefix)
	}
	if cfg.maxPacketBytes != 1024*1024 {
		t.Errorf("maxPacketBytes = %d, want 1 MiB default (parent §5.2)", cfg.maxPacketBytes)
	}
	if cfg.defaultLatencyThreshold != 100*time.Millisecond {
		t.Errorf("defaultLatencyThreshold = %v, want 100ms default", cfg.defaultLatencyThreshold)
	}
	if cfg.enableLatencyThresholdMetrics || cfg.enablePerOpcodeRequestBytesMetrics ||
		cfg.enablePerOpcodeResponseBytesMetrics || cfg.enablePerOpcodeDecoderErrorMetrics {
		t.Error("all enable_* flags must default false")
	}

	// Full config: every field set.
	full, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
		StatPrefix:                          "zk2",
		AccessLog:                           "/dev/null", // parse-accept-IGNORE (upstream parity)
		MaxPacketBytes:                      wrapperspb.UInt32(512),
		EnableLatencyThresholdMetrics:       true,
		DefaultLatencyThreshold:             durationpb.New(250 * time.Millisecond),
		EnablePerOpcodeRequestBytesMetrics:  true,
		EnablePerOpcodeResponseBytesMetrics: true,
		EnablePerOpcodeDecoderErrorMetrics:  true,
		LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
			{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping, Threshold: durationpb.New(5 * time.Millisecond)},
		},
	})
	if err != nil {
		t.Fatalf("parseConfig full: %v", err)
	}
	if full.maxPacketBytes != 512 || full.defaultLatencyThreshold != 250*time.Millisecond {
		t.Error("explicit max_packet_bytes / default_latency_threshold not honored")
	}
	// The override map is keyed by WIRE opcode (proto Ping=10 → wire 11; AMEND-A6).
	if got, ok := full.latencyThresholdOverrides[opPing]; !ok || got != 5*time.Millisecond {
		t.Errorf("override[wire opPing=11] = (%v, %v), want (5ms, true)", got, ok)
	}
}

// max_packet_bytes accepts ANY uint32 including 0 (no PGV bound; 0 → every
// packet oversized → decoder_error at decode time; upstream parity).
func TestParseConfig_MaxPacketBytesZeroAccepted(t *testing.T) {
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk", MaxPacketBytes: wrapperspb.UInt32(0)})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.maxPacketBytes != 0 {
		t.Errorf("maxPacketBytes = %d, want 0 (explicitly-set zero is honored, not defaulted)", cfg.maxPacketBytes)
	}
}

// The proto→wire opcode mapping — byte-stable against the parent §5.3 + §11.4
// pinned values. Spot-pins every divergent value (the gaps + negatives + >100s).
func TestProtoToWireOpcodeMapping(t *testing.T) {
	want := map[zookeeper_proxyv3.LatencyThresholdOverride_Opcode]int32{
		zookeeper_proxyv3.LatencyThresholdOverride_Connect:              0,
		zookeeper_proxyv3.LatencyThresholdOverride_Create:               1,
		zookeeper_proxyv3.LatencyThresholdOverride_Delete:               2,
		zookeeper_proxyv3.LatencyThresholdOverride_Exists:               3,
		zookeeper_proxyv3.LatencyThresholdOverride_GetData:              4,
		zookeeper_proxyv3.LatencyThresholdOverride_SetData:              5,
		zookeeper_proxyv3.LatencyThresholdOverride_GetAcl:               6,
		zookeeper_proxyv3.LatencyThresholdOverride_SetAcl:               7,
		zookeeper_proxyv3.LatencyThresholdOverride_GetChildren:          8,
		zookeeper_proxyv3.LatencyThresholdOverride_Sync:                 9,
		zookeeper_proxyv3.LatencyThresholdOverride_Ping:                 11,  // proto 10 → wire 11 (the gap)
		zookeeper_proxyv3.LatencyThresholdOverride_GetChildren2:         12,
		zookeeper_proxyv3.LatencyThresholdOverride_Check:                13,
		zookeeper_proxyv3.LatencyThresholdOverride_Multi:                14,
		zookeeper_proxyv3.LatencyThresholdOverride_Create2:              15,
		zookeeper_proxyv3.LatencyThresholdOverride_Reconfig:             16,
		zookeeper_proxyv3.LatencyThresholdOverride_CheckWatches:         17,
		zookeeper_proxyv3.LatencyThresholdOverride_RemoveWatches:        18,
		zookeeper_proxyv3.LatencyThresholdOverride_CreateContainer:      19,
		zookeeper_proxyv3.LatencyThresholdOverride_CreateTtl:            21,  // proto 19 → wire 21 (the gap at 20)
		zookeeper_proxyv3.LatencyThresholdOverride_Close:                -11, // proto 20 → wire −11
		zookeeper_proxyv3.LatencyThresholdOverride_SetAuth:              100, // proto 21 → wire 100
		zookeeper_proxyv3.LatencyThresholdOverride_SetWatches:           101,
		zookeeper_proxyv3.LatencyThresholdOverride_GetEphemerals:        103,
		zookeeper_proxyv3.LatencyThresholdOverride_GetAllChildrenNumber: 104,
		zookeeper_proxyv3.LatencyThresholdOverride_SetWatches2:          105,
		zookeeper_proxyv3.LatencyThresholdOverride_AddWatch:             106,
	}
	if len(protoToWireOpcode) != 27 {
		t.Fatalf("protoToWireOpcode has %d entries, want 27 (the proto enum is 27 contiguous values)", len(protoToWireOpcode))
	}
	for proto, wire := range want {
		if got := protoToWireOpcode[proto]; got != wire {
			t.Errorf("protoToWireOpcode[%v] = %d, want %d", proto, got, wire)
		}
	}
}

// The wire-opcode→opname table: digit-suffixed names intact + the SetAuth→auth
// aliasing (there are no setauth_* counters; AMEND-A3).
func TestWireOpcodeToOpname(t *testing.T) {
	cases := map[int32]string{
		opGetData:              "getdata",
		opCreate2:              "create2",
		opGetChildren2:         "getchildren2",
		opSetWatches2:          "setwatches2",
		opGetAllChildrenNumber: "getallchildrennumber",
		opClose:                "close",
		opSetAuth:              "auth", // SetAuth's opname is auth (no setauth_* counters)
		opPing:                 "ping",
	}
	for opcode, want := range cases {
		if got := wireOpcodeToOpname[opcode]; got != want {
			t.Errorf("wireOpcodeToOpname[%d] = %q, want %q", opcode, got, want)
		}
	}
}
```
**IMPL note:** the exact Go identifiers of the proto enum values (`LatencyThresholdOverride_Connect` vs another casing) come from Task 1 Step 3's grep of the generated `.pb.go` — fix up the test to the verified spelling before running.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -v`
Expected: FAIL — package does not compile (no production files yet).

- [ ] **Step 3: Create `doc.go` + `config.go`**

`doc.go`:
```go
// Package zookeeperproxy implements envoy.filters.network.zookeeper_proxy —
// a passive both-direction ZooKeeper-protocol observability sniffer (ADR-0222).
//
// Phase 28.1 lands the REQUEST side: the 9-field config parse, the 201-counter
// EAGER roster under <stat_prefix>.zookeeper. (creation parity — response-side
// counters exist at zero), the shallow request decoder (framing + xid sniffing
// + opcode dispatch + min-length validation + decoder-internal reassembly),
// the two xid correlation structures (written here, consumed at 28.2), and the
// dynamic per-scheme auth.<scheme>_rq counters. The filter implements BOTH
// network.ReadFilter and network.WriteFilter (one instance, both directions —
// the first consumer of the ADR-0221 WriteFilter seam); its 28.1 OnWrite is a
// PURE no-op Continue stub. Phase 28.2 lands the response decoder + xid
// correlation consumption + latency-threshold counters (ADR-0223).
package zookeeperproxy
```

`config.go` — the `compiledConfig` + `parseConfig` (happy path; the reject arms are Task 7) + the wire-opcode constants + both tables:
```go
package zookeeperproxy

import (
	"time"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
)

// Wire OpCodes (upstream decoder.h:30-58; AMEND-A6 — gaps, a negative value,
// and >100 values; NOT the proto's contiguous 0..26 enum).
const (
	opConnect              int32 = 0
	opCreate               int32 = 1
	opDelete               int32 = 2
	opExists               int32 = 3
	opGetData              int32 = 4
	opSetData              int32 = 5
	opGetACL               int32 = 6
	opSetACL               int32 = 7
	opGetChildren          int32 = 8
	opSync                 int32 = 9
	opPing                 int32 = 11
	opGetChildren2         int32 = 12
	opCheck                int32 = 13
	opMulti                int32 = 14
	opCreate2              int32 = 15
	opReconfig             int32 = 16
	opCheckWatches         int32 = 17
	opRemoveWatches        int32 = 18
	opCreateContainer      int32 = 19
	opCreateTTL            int32 = 21
	opClose                int32 = -11
	opSetAuth              int32 = 100
	opSetWatches           int32 = 101
	opGetEphemerals        int32 = 103
	opGetAllChildrenNumber int32 = 104
	opSetWatches2          int32 = 105
	opAddWatch             int32 = 106
)

// protoToWireOpcode maps the proto LatencyThresholdOverride_Opcode enum
// (27 contiguous values 0..26) to the wire opcode (parent SPEC §5.3 + AMEND-A6;
// upstream filter.h:311-339 opcodeMap). Used to key the latency-override map
// (consumed at 28.2) and to validate override opcodes at parse.
var protoToWireOpcode = map[zookeeper_proxyv3.LatencyThresholdOverride_Opcode]int32{
	// ... all 27 entries, transcribed per the Task-6 test table ...
}

// wireOpcodeToOpname maps wire opcodes to the upstream stats-macro opname
// (parent §7.2). NOTE: SetAuth's opname is "auth" — there are no setauth_*
// counters (AMEND-A3). connect is NOT here (it has no wire opcode — the
// decoder dispatches it by xid sniffing, AMEND-A5).
var wireOpcodeToOpname = map[int32]string{
	opCreate:               "create",
	opDelete:               "delete",
	opExists:               "exists",
	opGetData:              "getdata",
	opSetData:              "setdata",
	opGetACL:               "getacl",
	opSetACL:               "setacl",
	opGetChildren:          "getchildren",
	opSync:                 "sync",
	opPing:                 "ping",
	opGetChildren2:         "getchildren2",
	opCheck:                "check",
	opMulti:                "multi",
	opCreate2:              "create2",
	opReconfig:             "reconfig",
	opCheckWatches:         "checkwatches",
	opRemoveWatches:        "removewatches",
	opCreateContainer:      "createcontainer",
	opCreateTTL:            "createttl",
	opClose:                "close",
	opSetAuth:              "auth",
	opSetWatches:           "setwatches",
	opGetEphemerals:        "getephemerals",
	opGetAllChildrenNumber: "getallchildrennumber",
	opSetWatches2:          "setwatches2",
	opAddWatch:             "addwatch",
}

// compiledConfig is the boot-parsed, per-listener-shared zookeeper_proxy config
// (ADR-0079 two-step factory). The roster counters (stats.go) attach at factory
// time (Task 11); per-connection state lives on the decoder, never here.
type compiledConfig struct {
	statPrefix     string
	maxPacketBytes uint32

	// Latency fields: parsed + validated at 28.1; CONSUMED at 28.2 (SPEC §4.3).
	enableLatencyThresholdMetrics bool
	defaultLatencyThreshold       time.Duration
	latencyThresholdOverrides     map[int32]time.Duration // keyed by WIRE opcode

	// Flags consumed at 28.1 (gate increments, never creation — AMEND-A2).
	enablePerOpcodeRequestBytesMetrics bool
	enablePerOpcodeDecoderErrorMetrics bool
	// Parsed at 28.1; consumed at 28.2.
	enablePerOpcodeResponseBytesMetrics bool

	// stats is the eager 201-counter roster (attached by NewFactory, Task 11).
	stats *rosterStats
}

const (
	defaultMaxPacketBytes          = 1 << 20 // 1 MiB (parent §5.2)
	defaultLatencyThresholdMillis  = 100     // 100 ms (parent §5.2)
)

// parseConfig validates + compiles the 9-field proto (parent §5.2 roster).
// access_log is parse-accept-IGNORED (upstream parity — completely unread
// upstream; parent §11.2). Returns the Task-7 PARSE-REJECT errors on PGV-mirror
// violations.
func parseConfig(msg *zookeeper_proxyv3.ZooKeeperProxy) (*compiledConfig, error) {
	cfg := &compiledConfig{
		statPrefix:                          msg.GetStatPrefix(),
		maxPacketBytes:                      defaultMaxPacketBytes,
		defaultLatencyThreshold:             defaultLatencyThresholdMillis * time.Millisecond,
		latencyThresholdOverrides:           map[int32]time.Duration{},
		enableLatencyThresholdMetrics:       msg.GetEnableLatencyThresholdMetrics(),
		enablePerOpcodeRequestBytesMetrics:  msg.GetEnablePerOpcodeRequestBytesMetrics(),
		enablePerOpcodeResponseBytesMetrics: msg.GetEnablePerOpcodeResponseBytesMetrics(),
		enablePerOpcodeDecoderErrorMetrics:  msg.GetEnablePerOpcodeDecoderErrorMetrics(),
	}
	if msg.GetMaxPacketBytes() != nil {
		cfg.maxPacketBytes = msg.GetMaxPacketBytes().GetValue()
	}
	if msg.GetDefaultLatencyThreshold() != nil {
		cfg.defaultLatencyThreshold = msg.GetDefaultLatencyThreshold().AsDuration()
	}
	for _, o := range msg.GetLatencyThresholdOverrides() {
		wire := protoToWireOpcode[o.GetOpcode()]
		cfg.latencyThresholdOverrides[wire] = o.GetThreshold().AsDuration()
	}
	// Validation arms (stat_prefix required; latency PGV mirrors; duplicate
	// override) land at Task 7.
	return cfg, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/filter/network/zookeeperproxy/ -race -v` → all Task-6 tests PASS (the reject-arm tests do not exist yet).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/zookeeperproxy/...
git add internal/filter/network/zookeeperproxy/
git commit -m "phase 28.1 Task 6: zookeeperproxy package skeleton + 9-field config parse + proto→wire opcode mapping + opname table (ADR-0222 §4.3)"
```

---

## Task 7: PARSE-REJECT arms + `TestParseRejectConstants_ByteStable` (D-S28.1-2)

**Files:**
- Modify: `internal/filter/network/zookeeperproxy/config.go` (validation arms in `parseConfig`)
- Modify: `internal/filter/network/zookeeperproxy/config_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// ADR-0080 byte-stable PARSE-REJECT discipline: every arm's wording is a named
// constant pinned by this table test. Prefix: "zookeeper_proxy: " (SPEC §6;
// D-S28.1-2 finalized HERE — these strings are byte-stable from this commit on).
func TestParseRejectConstants_ByteStable(t *testing.T) {
	cases := []struct{ name, constant, want string }{
		{"stat-prefix-required", errStatPrefixRequired, "zookeeper_proxy: stat_prefix is required"},
		{"latency-override-threshold-required", errLatencyOverrideThresholdRequired, "zookeeper_proxy: latency_threshold_overrides: threshold is required"},
		{"latency-override-threshold-too-small", errLatencyOverrideThresholdTooSmall, "zookeeper_proxy: latency_threshold_overrides: threshold must be at least 1ms"},
		{"latency-override-opcode-undefined", errLatencyOverrideOpcodeUndefined, "zookeeper_proxy: latency_threshold_overrides: opcode is not a defined opcode"},
		{"default-latency-threshold-too-small", errDefaultLatencyThresholdTooSmall, "zookeeper_proxy: default_latency_threshold must be at least 1ms"},
		{"latency-override-duplicate-opcode", errLatencyOverrideDuplicateOpcode, "zookeeper_proxy: latency_threshold_overrides: duplicate opcode"},
	}
	for _, tc := range cases {
		if tc.constant != tc.want {
			t.Errorf("%s = %q, want %q (byte-stable; ADR-0080)", tc.name, tc.constant, tc.want)
		}
	}
}

// Each PGV-mirror arm fires (SPEC §6.1 + §6.2; the parse code lands at 28.1,
// the latency arms' FIXTURE disposition is parent D-P4 = 28.2's).
func TestParseConfig_RejectArms(t *testing.T) {
	ms := func(d time.Duration) *durationpb.Duration { return durationpb.New(d) }
	cases := []struct {
		name    string
		msg     *zookeeper_proxyv3.ZooKeeperProxy
		wantErr string
	}{
		{"missing stat_prefix", &zookeeper_proxyv3.ZooKeeperProxy{}, errStatPrefixRequired},
		{"empty stat_prefix", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: ""}, errStatPrefixRequired},
		{"override threshold missing", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
				{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping}}}, errLatencyOverrideThresholdRequired},
		{"override threshold below 1ms", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
				{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping, Threshold: ms(500 * time.Microsecond)}}}, errLatencyOverrideThresholdTooSmall},
		{"override opcode undefined", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
				{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Opcode(999), Threshold: ms(5 * time.Millisecond)}}}, errLatencyOverrideOpcodeUndefined},
		{"default threshold below 1ms", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			DefaultLatencyThreshold: ms(500 * time.Microsecond)}, errDefaultLatencyThresholdTooSmall},
		{"duplicate override opcode", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
				{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping, Threshold: ms(5 * time.Millisecond)},
				{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping, Threshold: ms(7 * time.Millisecond)},
			}}, errLatencyOverrideDuplicateOpcode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfig(tc.msg)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("parseConfig() err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// 1ms exactly is ACCEPTED (PGV gte = inclusive).
func TestParseConfig_OneMillisecondAccepted(t *testing.T) {
	_, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
		DefaultLatencyThreshold: durationpb.New(time.Millisecond),
		LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
			{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping, Threshold: durationpb.New(time.Millisecond)}}})
	if err != nil {
		t.Fatalf("parseConfig(1ms thresholds) = %v, want nil (gte is inclusive)", err)
	}
}
```
**IMPL note (D-S28.1-2 finalization):** the table above is the PLAN-anticipated wording. The IMPL may adjust phrasing ONCE at this task (e.g. to embed the offending opcode value via `fmt.Errorf` wrapping a constant prefix) — whatever lands here is byte-stable from this commit forward. The error CONSTANTS hold the stable prefix; dynamic detail (index/value) may be appended via `%w`-wrapped formatting, mirroring the rbac/tcpproxy arm shapes.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run 'ParseReject|RejectArms|OneMillisecond' -v`
Expected: FAIL — the `err*` constants are undefined; `parseConfig` performs no validation yet.

- [ ] **Step 3: Add the constants + validation to `config.go`**

```go
// PARSE-REJECT arms (ADR-0080 byte-stable; SPEC §6; D-S28.1-2). The
// stat-prefix arm is the load-bearing 0047 fixture arm; the latency PGV-mirror
// arms are unit-test-only at 28.1 (their fixture disposition is parent D-P4 = 28.2's).
const (
	errStatPrefixRequired               = "zookeeper_proxy: stat_prefix is required"
	errLatencyOverrideThresholdRequired = "zookeeper_proxy: latency_threshold_overrides: threshold is required"
	errLatencyOverrideThresholdTooSmall = "zookeeper_proxy: latency_threshold_overrides: threshold must be at least 1ms"
	errLatencyOverrideOpcodeUndefined   = "zookeeper_proxy: latency_threshold_overrides: opcode is not a defined opcode"
	errDefaultLatencyThresholdTooSmall  = "zookeeper_proxy: default_latency_threshold must be at least 1ms"
	errLatencyOverrideDuplicateOpcode   = "zookeeper_proxy: latency_threshold_overrides: duplicate opcode"
)
```
In `parseConfig`, add the validation (in field order — stat_prefix first, then default threshold, then per-override checks):
```go
	if msg.GetStatPrefix() == "" {
		return nil, errors.New(errStatPrefixRequired)
	}
	...
	if msg.GetDefaultLatencyThreshold() != nil && msg.GetDefaultLatencyThreshold().AsDuration() < time.Millisecond {
		return nil, errors.New(errDefaultLatencyThresholdTooSmall)
	}
	for _, o := range msg.GetLatencyThresholdOverrides() {
		if _, defined := protoToWireOpcode[o.GetOpcode()]; !defined {
			return nil, fmt.Errorf("%s: %d", errLatencyOverrideOpcodeUndefined, o.GetOpcode())
		}
		if o.GetThreshold() == nil {
			return nil, errors.New(errLatencyOverrideThresholdRequired)
		}
		if o.GetThreshold().AsDuration() < time.Millisecond {
			return nil, errors.New(errLatencyOverrideThresholdTooSmall)
		}
		wire := protoToWireOpcode[o.GetOpcode()]
		if _, dup := cfg.latencyThresholdOverrides[wire]; dup {
			return nil, fmt.Errorf("%s: %v", errLatencyOverrideDuplicateOpcode, o.GetOpcode())
		}
		cfg.latencyThresholdOverrides[wire] = o.GetThreshold().AsDuration()
	}
```
(`defined_only` check note: the proto enum's generated `_name` map can also drive the defined-check — `zookeeper_proxyv3.LatencyThresholdOverride_Opcode_name[int32(o.GetOpcode())]`; using `protoToWireOpcode` membership is equivalent because the mapping covers exactly the 27 defined values; either is acceptable, document the choice.)

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/filter/network/zookeeperproxy/ -race -v` → ALL pass (Task 6 happy-path tests stay green; the new reject tests pass).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/zookeeperproxy/...
git add internal/filter/network/zookeeperproxy/
git commit -m "phase 28.1 Task 7: PARSE-REJECT arms (stat_prefix + latency PGV mirrors + duplicate-override) + byte-stable wording table test (ADR-0080; D-S28.1-2)"
```

---

## Task 8: `stats.go` — the 201-suffix roster + eager creation + dynamic auth counters (R2)

**Files:**
- Create: `internal/filter/network/zookeeperproxy/stats.go`
- Create: `internal/filter/network/zookeeperproxy/stats_test.go`

- [ ] **Step 1: Derive the authoritative 201-suffix golden list (no code yet)**

The roster is the upstream `ALL_ZOOKEEPER_PROXY_STATS` macro (upstream `filter.h:30-231` at v1.37.2) — but the AUTHORITATIVE empirical source is the live reference image's eager roster (parent §11.1: the booted image exposes all 201 at 0). Derive the golden list from the live image (the same probe the parent SPEC ran):
```bash
# Boot the reference image with a [zookeeper_proxy, tcp_proxy] listener (stat_prefix: zkprobe)
# — reuse/adapt the parent SPEC §11.1 probe bootstrap — then:
curl -s http://127.0.0.1:9901/stats | grep '^zkprobe\.zookeeper\.' | cut -d: -f1 | sed 's/^zkprobe\.zookeeper\.//' | sort > /tmp/zk_roster.txt
wc -l /tmp/zk_roster.txt    # expect: 201
# family arithmetic check:
grep -c '_rq$' /tmp/zk_roster.txt              # expect 28
grep -c '_rq_bytes$' /tmp/zk_roster.txt        # expect 29
grep -c '_decoder_error$' /tmp/zk_roster.txt   # expect 28
grep -c '_resp$' /tmp/zk_roster.txt            # expect 28
grep -c '_resp_bytes$' /tmp/zk_roster.txt      # expect 28
grep -c '_resp_fast$' /tmp/zk_roster.txt       # expect 28
grep -c '_resp_slow$' /tmp/zk_roster.txt       # expect 28
# remaining 4 plain: decoder_error, request_bytes, response_bytes, watch_event
```
The dump IS the golden list transcribed into `rosterSuffixes()` + the test. (If docker is unavailable in the IMPL session, fall back to transcribing the upstream macro from `https://raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/source/extensions/filters/network/zookeeper_proxy/filter.h` — but the live dump is preferred: it is what the `0046` arm-6 exists-at-zero assertions will compare against.) **Note the per-family membership asymmetries (AMEND-A3) the dump will confirm:** `connect_readonly` appears ONLY in `_rq`/`_rq_bytes`; there is NO `auth_rq` but `auth_rq_bytes`/`auth_resp`/`auth_resp_*`/`auth_decoder_error` exist; the dump resolves any prose ambiguity in the parent §7.2 opname list authoritatively.

- [ ] **Step 2: Write the failing tests**

`stats_test.go`:
```go
// R2: the roster matches the upstream macro / live reference dump exactly.
func TestCounterRoster_MatchesUpstreamMacro(t *testing.T) {
	suffixes := rosterSuffixes()
	if len(suffixes) != 201 {
		t.Fatalf("rosterSuffixes() = %d names, want 201 (AMEND-A2)", len(suffixes))
	}
	// Family arithmetic (AMEND-A2): 4 plain + 28 _rq + 29 _rq_bytes +
	// 28 _decoder_error + 28 _resp + 28 _resp_bytes + 28 _resp_fast + 28 _resp_slow.
	families := map[string]int{}
	for _, s := range suffixes {
		switch {
		case strings.HasSuffix(s, "_rq_bytes"):
			families["_rq_bytes"]++
		case strings.HasSuffix(s, "_rq"):
			families["_rq"]++
		case strings.HasSuffix(s, "_decoder_error") && s != "decoder_error":
			families["_decoder_error"]++
		case strings.HasSuffix(s, "_resp_bytes"):
			families["_resp_bytes"]++
		case strings.HasSuffix(s, "_resp_fast"):
			families["_resp_fast"]++
		case strings.HasSuffix(s, "_resp_slow"):
			families["_resp_slow"]++
		case strings.HasSuffix(s, "_resp"):
			families["_resp"]++
		default:
			families["plain"]++
		}
	}
	want := map[string]int{"plain": 4, "_rq": 28, "_rq_bytes": 29, "_decoder_error": 28,
		"_resp": 28, "_resp_bytes": 28, "_resp_fast": 28, "_resp_slow": 28}
	for fam, n := range want {
		if families[fam] != n {
			t.Errorf("family %s has %d names, want %d", fam, families[fam], n)
		}
	}
	// Digit-suffix regression guard (reference_proto_roster_extraction_digits).
	set := map[string]bool{}
	for _, s := range suffixes {
		set[s] = true
	}
	for _, must := range []string{"create2_rq", "getchildren2_rq", "setwatches2_rq",
		"getallchildrennumber_rq", "connect_readonly_rq", "auth_rq_bytes", "auth_resp",
		"decoder_error", "request_bytes", "response_bytes", "watch_event"} {
		if !set[must] {
			t.Errorf("roster missing %q", must)
		}
	}
	// Asymmetries (AMEND-A3): NO auth_rq; NO setauth_*; NO connect_readonly_resp.
	for _, mustNot := range []string{"auth_rq", "setauth_rq", "setauth_resp", "connect_readonly_resp"} {
		if set[mustNot] {
			t.Errorf("roster must NOT contain %q (AMEND-A3 asymmetry)", mustNot)
		}
	}
	// The full sorted golden list (transcribed from the Step-1 live dump).
	golden := []string{ /* the 201 sorted names from /tmp/zk_roster.txt */ }
	sorted := append([]string(nil), suffixes...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(sorted, golden) {
		t.Errorf("roster diverges from the golden list (len got %d / want %d); diff: %v",
			len(sorted), len(golden), firstDiff(sorted, golden))
	}
}

// Eager creation (D-P5): newRosterStats creates exactly 201 counters in the registry.
func TestRosterStats_EagerCreation(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRosterStats(reg, "zk")
	if len(rs.counters) != 201 {
		t.Fatalf("rosterStats created %d counters, want 201", len(rs.counters))
	}
	// Spot-check internal names + zero values: response-side counters exist at 0.
	for _, suffix := range []string{"getdata_resp", "getdata_resp_fast", "watch_event", "response_bytes"} {
		c := rs.counters[suffix]
		if c == nil {
			t.Fatalf("counter %q not created", suffix)
		}
		if c.Load() != 0 {
			t.Errorf("counter %q = %d at creation, want 0", suffix, c.Load())
		}
	}
}

// Idempotent shared-prefix creation (NewCounterIfAbsent): two listeners sharing a
// stat_prefix share counters, no panic (the rbac newFilterStats precedent).
func TestRosterStats_IdempotentSharedPrefix(t *testing.T) {
	reg := stats.NewRegistry()
	a := newRosterStats(reg, "zk")
	b := newRosterStats(reg, "zk")
	if a.counters["getdata_rq"] != b.counters["getdata_rq"] {
		t.Fatal("shared stat_prefix must share the same counter instances")
	}
}

// Dynamic per-scheme auth counters: lazily created; unknown schemes get their
// own counter; repeated calls return the same counter.
func TestRosterStats_DynamicAuthSchemeCounters(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRosterStats(reg, "zk")
	c1 := rs.authSchemeCounter("digest")
	c2 := rs.authSchemeCounter("digest")
	if c1 != c2 {
		t.Fatal("repeated authSchemeCounter(digest) must return the same counter")
	}
	c1.Inc()
	if c1.Load() != 1 {
		t.Fatal("auth counter increment lost")
	}
	// The dynamic name shape: <stat_prefix>.zookeeper.auth.<scheme>_rq
	// (verified via the registry — flatten through internal/stats Prometheus
	// path at Task 13).
}

// inc/add panics on an unknown suffix (programming-error guard).
func TestRosterStats_UnknownSuffixPanics(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRosterStats(reg, "zk")
	defer func() {
		if recover() == nil {
			t.Fatal("inc(unknown) must panic")
		}
	}()
	rs.inc("not_a_counter")
}
```
(The as-built `*stats.Counter` accessor is **`Load() uint64`** — `internal/stats/counter.go:30`; `Inc()`/`Add(delta)` at `:22`/`:27`. The rbac tests use `Load()` too — `rbac_test.go:340`. There is NO `Value()` method; every counter-read in this PLAN's test snippets uses `Load()`.)

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run 'Roster' -v`
Expected: FAIL — `rosterSuffixes`/`newRosterStats` undefined.

- [ ] **Step 4: Create `stats.go`**

```go
package zookeeperproxy

import (
	"fmt"

	"github.com/esalaine/envoy-go/internal/stats"
)

// opNames is the per-opcode counter-name table (the upstream stats-macro
// spelling, parent §7.2 + the Step-1 live-dump transcription). Family
// membership is NOT uniform (AMEND-A3) — the per-family lists below encode the
// asymmetries exactly as the upstream macro declares them.
//
// KEEP-IN-SYNC: internal/stats/name.go (the .zookeeper. inline-prefix arm) and
// the 0046 fixture's expected-counter tables key off these suffixes.

// rqOpNames: the 28 <op>_rq counters (incl. connect_readonly; NO auth — AMEND-A3).
var rqOpNames = []string{ /* transcribed from the Step-1 dump */ }

// rqBytesOpNames: the 29 <op>_rq_bytes counters (incl. both connect variants + auth).
var rqBytesOpNames = []string{ /* transcribed */ }

// decoderErrorOpNames: the 28 <op>_decoder_error counters.
var decoderErrorOpNames = []string{ /* transcribed */ }

// respOpNames: the 28 <op>_resp counters (incl. auth; NO connect_readonly) —
// shared by the _resp/_resp_bytes/_resp_fast/_resp_slow families.
var respOpNames = []string{ /* transcribed */ }

// rosterSuffixes returns the exact 201 upstream-macro counter suffixes
// (4 plain + 28 _rq + 29 _rq_bytes + 28 _decoder_error + 28 _resp +
// 28 _resp_bytes + 28 _resp_fast + 28 _resp_slow — AMEND-A2; R2).
func rosterSuffixes() []string {
	out := make([]string, 0, 201)
	out = append(out, "decoder_error", "request_bytes", "response_bytes", "watch_event")
	for _, op := range rqOpNames {
		out = append(out, op+"_rq")
	}
	for _, op := range rqBytesOpNames {
		out = append(out, op+"_rq_bytes")
	}
	for _, op := range decoderErrorOpNames {
		out = append(out, op+"_decoder_error")
	}
	for _, op := range respOpNames {
		out = append(out, op+"_resp", op+"_resp_bytes", op+"_resp_fast", op+"_resp_slow")
	}
	return out
}

// rosterStats holds the 201 EAGERLY-created macro counters (D-P5 creation
// parity; created once per distinct stat_prefix at config parse) + the lazily
// created dynamic per-scheme auth counters.
type rosterStats struct {
	prefix   string // "<stat_prefix>.zookeeper."
	reg      *stats.Registry
	counters map[string]*stats.Counter // keyed by suffix; all 201 created eagerly
}

// newRosterStats eagerly creates all 201 macro counters under
// <statPrefix>.zookeeper. via NewCounterIfAbsent — post-Freeze-permitted
// (registry.go:157-171) + idempotent across listeners sharing a stat_prefix
// (the rbac newFilterStats precedent, rbac.go:187-198).
func newRosterStats(reg *stats.Registry, statPrefix string) *rosterStats {
	rs := &rosterStats{
		prefix:   statPrefix + ".zookeeper.",
		reg:      reg,
		counters: make(map[string]*stats.Counter, 201),
	}
	for _, suffix := range rosterSuffixes() {
		rs.counters[suffix] = reg.NewCounterIfAbsent(rs.prefix + suffix)
	}
	return rs
}

// inc increments the macro counter for suffix. Unknown suffix = programming
// error → panic (the roster is closed; dynamic names go through authSchemeCounter).
func (rs *rosterStats) inc(suffix string) {
	c, ok := rs.counters[suffix]
	if !ok {
		panic(fmt.Sprintf("zookeeperproxy: unknown roster suffix %q", suffix))
	}
	c.Inc()
}

// add adds delta to the macro counter for suffix (the *_bytes accounting).
func (rs *rosterStats) add(suffix string, delta uint64) {
	c, ok := rs.counters[suffix]
	if !ok {
		panic(fmt.Sprintf("zookeeperproxy: unknown roster suffix %q", suffix))
	}
	c.Add(delta)
}

// authSchemeCounter returns (lazily creating) the dynamic per-scheme auth
// request counter <stat_prefix>.zookeeper.auth.<scheme>_rq (AMEND-A3; the rbac
// per-policy dynamic-name precedent). NewCounterIfAbsent is REQUIRED here:
// decode time is post-Freeze runtime.
func (rs *rosterStats) authSchemeCounter(scheme string) *stats.Counter {
	return rs.reg.NewCounterIfAbsent(rs.prefix + "auth." + scheme + "_rq")
}
```
Fill the four family tables from the Step-1 dump. The IMPL transcribes them VERBATIM (all-lowercase macro spelling; digit-suffixed names intact).

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/filter/network/zookeeperproxy/ -race -v` → all PASS (incl. the golden-list comparison).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/zookeeperproxy/...
git add internal/filter/network/zookeeperproxy/
git commit -m "phase 28.1 Task 8: the 201-suffix eager roster + TestCounterRoster_MatchesUpstreamMacro golden list + dynamic auth-scheme counters (ADR-0222 §4.4; D-P5; R2)"
```

---

## Task 9: `decoder.go` part 1 — framing + reassembly + the high-water mark + special-xid dispatch

**Files:**
- Create: `internal/filter/network/zookeeperproxy/decoder.go`
- Create: `internal/filter/network/zookeeperproxy/decoder_test.go`

- [ ] **Step 1: Write the failing tests**

`decoder_test.go`. Frame-building test helpers (these double as the model for the Task-15 fixture builders):
```go
// --- test frame builders (big-endian; 4-byte length prefix EXCLUDES itself) ---

func be32(v int32) []byte  { b := make([]byte, 4); binary.BigEndian.PutUint32(b, uint32(v)); return b }
func be64(v int64) []byte  { b := make([]byte, 8); binary.BigEndian.PutUint64(b, uint64(v)); return b }
func zkFrame(parts ...[]byte) []byte {
	payload := bytes.Join(parts, nil)
	return append(be32(int32(len(payload))), payload...)
}

// connectFrame: protocol_version(4=0) + last_zxid(8=0) + timeout(4) + session(8=0)
// + password(4-byte len + 16 bytes) [+ optional readonly bool(1)].
// The leading protocol_version=0 doubles as the sniffed ConnectXid (AMEND-A5).
func connectFrame(readonly *bool) []byte {
	parts := [][]byte{be32(0), be64(0), be32(30000), be64(0), be32(16), make([]byte, 16)}
	if readonly != nil {
		b := byte(0)
		if *readonly {
			b = 1
		}
		parts = append(parts, []byte{b})
	}
	return zkFrame(parts...)
}

// dataFrame: xid(4) + opcode(4) + payload.
func dataFrame(xid, opcode int32, payload []byte) []byte {
	return zkFrame(be32(xid), be32(opcode), payload)
}

func newTestDecoder(t *testing.T) (*requestDecoder, *rosterStats, *compiledConfig) {
	t.Helper()
	reg := stats.NewRegistry()
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk"})
	if err != nil {
		t.Fatal(err)
	}
	rs := newRosterStats(reg, "zk")
	cfg.stats = rs
	return newRequestDecoder(cfg, rs), rs, cfg
}

func counterValue(t *testing.T, rs *rosterStats, suffix string) uint64 {
	t.Helper()
	return rs.counters[suffix].Load()
}

// --- special-xid dispatch (AMEND-A5) ---

func TestDecodeConnect(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(connectFrame(nil))
	if got := counterValue(t, rs, "connect_rq"); got != 1 {
		t.Fatalf("connect_rq = %d, want 1", got)
	}
	if got := counterValue(t, rs, "connect_readonly_rq"); got != 0 {
		t.Fatalf("connect_readonly_rq = %d, want 0", got)
	}
}

func TestDecodeConnectReadonly(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	ro := true
	d.decodeOnData(connectFrame(&ro))
	if got := counterValue(t, rs, "connect_readonly_rq"); got != 1 {
		t.Fatalf("connect_readonly_rq = %d, want 1", got)
	}
	if got := counterValue(t, rs, "connect_rq"); got != 0 {
		t.Fatalf("connect_rq = %d, want 0 (readonly connect counts ONLY connect_readonly_rq)", got)
	}
}

func TestDecodePing(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(zkFrame(be32(pingXid), be32(opPing)))
	if got := counterValue(t, rs, "ping_rq"); got != 1 {
		t.Fatalf("ping_rq = %d, want 1", got)
	}
}

// auth: xid −4 → skip type int(4) → scheme string (4-byte len + bytes) →
// dynamic auth.<scheme>_rq counter (AMEND-A3).
func TestDecodeAuthScheme(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	scheme := []byte("digest")
	d.decodeOnData(zkFrame(be32(authXid), be32(100) /* type */, be32(int32(len(scheme))), scheme, be32(0) /* cred len */))
	got := rs.reg.NewCounterIfAbsent("zk.zookeeper.auth.digest_rq").Load()
	if got != 1 {
		t.Fatalf("auth.digest_rq = %d, want 1", got)
	}
	// NO static auth_rq counter exists (AMEND-A3) — nothing else incremented.
}

func TestDecodeSetWatches(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(zkFrame(be32(setWatchesXid), be32(opSetWatches)))
	if got := counterValue(t, rs, "setwatches_rq"); got != 1 {
		t.Fatalf("setwatches_rq = %d, want 1", got)
	}
}

// --- reassembly + the high-water mark (D-S28.1-3; AMEND-A8) ---

// Partial frames: a frame split across two decodeOnData calls (cumulative chain
// buffer — the chain Buffer is never drained, so call 2 sees call 1's bytes too).
func TestDecodePartialFrameReassembly(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	frame := dataFrame(1, opGetData, []byte("/path"))
	cut := len(frame) / 2
	// First read: chain buffer holds the first half.
	d.decodeOnData(frame[:cut])
	if got := counterValue(t, rs, "getdata_rq"); got != 0 {
		t.Fatalf("getdata_rq = %d after partial frame, want 0", got)
	}
	// Second read: chain buffer now holds the WHOLE accumulating buffer
	// (the chain Buffer accumulates: zookeeperproxy never drains it).
	d.decodeOnData(frame)
	if got := counterValue(t, rs, "getdata_rq"); got != 1 {
		t.Fatalf("getdata_rq = %d after reassembly, want 1", got)
	}
}

// The high-water mark: re-delivered (undrained) chain-buffer bytes are NOT
// double-counted (D-S28.1-3 — the multi-read no-double-count proof).
func TestDecodeHighWaterMarkNoDoubleCount(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	f1 := dataFrame(1, opGetData, nil)
	f2 := dataFrame(2, opCreate, nil)
	// Read 1: chain buffer = f1.
	d.decodeOnData(f1)
	// Read 2: chain buffer = f1 + f2 (accumulated — f1 is RE-DELIVERED).
	d.decodeOnData(append(append([]byte{}, f1...), f2...))
	if got := counterValue(t, rs, "getdata_rq"); got != 1 {
		t.Fatalf("getdata_rq = %d, want 1 (re-delivered bytes must not double-count)", got)
	}
	if got := counterValue(t, rs, "create_rq"); got != 1 {
		t.Fatalf("create_rq = %d, want 1", got)
	}
}

// Two complete frames in one read decode in sequence.
func TestDecodeTwoFramesOneRead(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	buf := append(dataFrame(1, opGetData, nil), dataFrame(2, opExists, nil)...)
	d.decodeOnData(buf)
	if counterValue(t, rs, "getdata_rq") != 1 || counterValue(t, rs, "exists_rq") != 1 {
		t.Fatal("both frames in a single read must decode")
	}
}

// The decoder NEVER mutates the chain bytes it is fed (R3 precondition; the
// fuzzer re-asserts this).
func TestDecodeDoesNotMutateInput(t *testing.T) {
	d, _, _ := newTestDecoder(t)
	frame := dataFrame(1, opGetData, []byte("/p"))
	orig := append([]byte(nil), frame...)
	d.decodeOnData(frame)
	if !bytes.Equal(frame, orig) {
		t.Fatal("decodeOnData mutated its input slice")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run TestDecode -v`
Expected: FAIL — `requestDecoder`/`newRequestDecoder` undefined.

- [ ] **Step 3: Create `decoder.go` (part 1 — structure + reassembly + special xids)**

```go
package zookeeperproxy

import (
	"encoding/binary"
	"time"
)

// Special xids (upstream XidCodes; AMEND-A5). The decoder dispatches per-packet
// by xid sniffing — there is no first-packet state machine.
const (
	connectXid    int32 = 0
	watchXid      int32 = -1 // response-side only (28.2); listed for completeness
	pingXid       int32 = -2
	authXid       int32 = -4
	setWatchesXid int32 = -8
)

// pendingRequest is a correlation-structure entry (AMEND-A7): written at 28.1
// on every successful request decode; consumed by the 28.2 response decoder
// (R5 — never read at 28.1).
type pendingRequest struct {
	opname     string
	wireOpcode int32
	start      time.Time
}

// requestDecoder is the per-connection shallow request decoder (ADR-0222;
// AMEND-A5/A8; D-P2 shallow). It owns its OWN reassembly buffer; the chain
// Buffer is read, NEVER drained (R3).
type requestDecoder struct {
	cfg   *compiledConfig
	stats *rosterStats

	// chainConsumed is the high-water mark of chain-buffer bytes already fed
	// into readBuf (D-S28.1-3, PLAN-resolved: the mark lives on the decoder).
	// The chain buffer accumulates undrained bytes across reads, so
	// decodeOnData receives the FULL buffer each call and feeds only the new
	// tail — re-delivered bytes are never double-counted.
	chainConsumed int

	// readBuf is the decoder-internal reassembly buffer (AMEND-A8): complete
	// frames are decoded + consumed; a trailing partial frame survives until
	// the next read; a decode failure ABANDONS it (no resync).
	readBuf []byte

	// Correlation structures (AMEND-A7; written 28.1, consumed 28.2 — R5).
	requestsByXid        map[int32]pendingRequest   // data requests (xid > 0); insert overwrites
	controlRequestsByXid map[int32][]pendingRequest // control requests; FIFO queue per xid
}

func newRequestDecoder(cfg *compiledConfig, rs *rosterStats) *requestDecoder {
	return &requestDecoder{
		cfg:                  cfg,
		stats:                rs,
		requestsByXid:        map[int32]pendingRequest{},
		controlRequestsByXid: map[int32][]pendingRequest{},
	}
}

// decodeOnData feeds the FULL current chain-buffer contents into the decoder.
// Only bytes past the high-water mark are appended to readBuf (a COPY — the
// chain buffer is never aliased or mutated); then complete frames are decoded
// in a loop. Decode failures abandon readBuf (AMEND-A8 no-resync) but never
// affect the chain buffer or the connection.
func (d *requestDecoder) decodeOnData(chainBytes []byte) {
	if len(chainBytes) > d.chainConsumed {
		d.readBuf = append(d.readBuf, chainBytes[d.chainConsumed:]...)
		d.chainConsumed = len(chainBytes)
	}
	for {
		frame, ok := d.nextFrame()
		if !ok {
			return // no complete frame buffered (or buffer abandoned)
		}
		if !d.decodeFrame(frame) {
			// decoder_error path already counted + readBuf abandoned.
			return
		}
	}
}

// nextFrame extracts one complete frame from readBuf (the 4-byte BE length
// prefix EXCLUDES itself and is stripped from the returned frame). Returns
// ok=false when no complete frame is buffered. Oversized frames
// (len > max_packet_bytes) take the decoder_error path and abandon the buffer.
func (d *requestDecoder) nextFrame() ([]byte, bool) {
	if len(d.readBuf) < 4 {
		return nil, false
	}
	frameLen := int32(binary.BigEndian.Uint32(d.readBuf[0:4]))
	if frameLen < 0 || uint32(frameLen) > d.cfg.maxPacketBytes {
		// "packet is too big" (parent §11.5) → decoder_error + abandon.
		d.decoderError("")
		return nil, false
	}
	if len(d.readBuf) < 4+int(frameLen) {
		return nil, false // partial frame — wait for more bytes
	}
	frame := d.readBuf[4 : 4+frameLen]
	d.readBuf = d.readBuf[4+frameLen:]
	return frame, true
}

// decodeFrame dispatches one frame by xid sniffing (AMEND-A5). Returns false on
// a decode failure (the decoder_error path has already run).
func (d *requestDecoder) decodeFrame(frame []byte) bool {
	if len(frame) < 8 {
		// universal min: xid(4) + opcode(4) ("packet is too small").
		d.decoderError("")
		return false
	}
	xid := int32(binary.BigEndian.Uint32(frame[0:4]))
	switch xid {
	case connectXid:
		return d.onConnect(frame)
	case pingXid:
		d.stats.inc("ping_rq")
		d.countRequestBytes("ping", wireFootprint(frame))
		d.recordControl(pingXid, "ping", opPing)
		return true
	case authXid:
		return d.onAuth(frame)
	case setWatchesXid:
		d.stats.inc("setwatches_rq")
		d.countRequestBytes("setwatches", wireFootprint(frame))
		d.recordControl(setWatchesXid, "setwatches", opSetWatches)
		return true
	default:
		return d.onDataRequest(xid, frame) // Task 10
	}
}

// onConnect parses the connect special framing: protocol_version(4) +
// last_zxid(8) + timeout(4) + session_id(8) + password(4-byte len + bytes) +
// OPTIONAL trailing readonly bool(1). Readonly present AND true →
// connect_readonly_rq; else connect_rq (AMEND-A3/A5).
func (d *requestDecoder) onConnect(frame []byte) bool {
	// Shallow validation: the fixed header is 28 bytes + password + optional readonly.
	const fixedLen = 4 + 8 + 4 + 8 + 4 // up to and including the password length
	if len(frame) < fixedLen {
		d.decoderError("connect")
		return false
	}
	pwLen := int32(binary.BigEndian.Uint32(frame[24:28]))
	if pwLen < 0 || len(frame) < fixedLen+int(pwLen) {
		d.decoderError("connect")
		return false
	}
	readonly := false
	if rest := frame[fixedLen+int(pwLen):]; len(rest) >= 1 && rest[0] == 1 {
		readonly = true
	}
	opname := "connect"
	if readonly {
		opname = "connect_readonly"
		d.stats.inc("connect_readonly_rq")
	} else {
		d.stats.inc("connect_rq")
	}
	d.countRequestBytes(opname, wireFootprint(frame))
	d.recordControl(connectXid, opname, opConnect)
	return true
}

// onAuth parses the auth special framing: xid(4) + type(4) + scheme string
// (4-byte len + bytes) + credential (skipped — shallow). The scheme is the
// only payload value the shallow decoder extracts (SPEC §4.4) → the dynamic
// auth.<scheme>_rq counter (AMEND-A3). There is NO static auth_rq.
func (d *requestDecoder) onAuth(frame []byte) bool {
	if len(frame) < 12 {
		d.decoderError("auth")
		return false
	}
	schemeLen := int32(binary.BigEndian.Uint32(frame[8:12]))
	if schemeLen < 0 || len(frame) < 12+int(schemeLen) {
		d.decoderError("auth")
		return false
	}
	scheme := string(frame[12 : 12+schemeLen])
	if scheme == "" {
		scheme = "unknown_scheme"
	}
	d.stats.authSchemeCounter(scheme).Inc()
	d.countRequestBytes("auth", wireFootprint(frame))
	d.recordControl(authXid, "auth", opSetAuth)
	return true
}

// recordControl appends to the per-xid FIFO control queue (AMEND-A7).
func (d *requestDecoder) recordControl(xid int32, opname string, wireOpcode int32) {
	d.controlRequestsByXid[xid] = append(d.controlRequestsByXid[xid],
		pendingRequest{opname: opname, wireOpcode: wireOpcode, start: time.Now()})
}

// wireFootprint is the request_bytes accounting basis: the 4-byte length
// prefix + the frame payload (SPEC §4.5 item 4 — the 0046 arm-2 cross-side
// equality proof).
func wireFootprint(frame []byte) int { return 4 + len(frame) }

// countRequestBytes increments request_bytes (always) + the flag-gated
// per-opcode <opname>_rq_bytes (AMEND-A2: flags gate increments, never creation).
func (d *requestDecoder) countRequestBytes(opname string, wireBytes int) {
	d.stats.add("request_bytes", uint64(wireBytes))
	if d.cfg.enablePerOpcodeRequestBytesMetrics {
		d.stats.add(opname+"_rq_bytes", uint64(wireBytes))
	}
}

// decoderError runs the decoder_error path (AMEND-A8): increment decoder_error
// (always) + the flag-gated per-opcode counter (when the failing frame's opcode
// is known), then ABANDON the current readBuf (no resync). The connection is
// never closed; the correlation structures persist; later reads decode normally.
func (d *requestDecoder) decoderError(opname string) {
	d.stats.inc("decoder_error")
	if opname != "" && d.cfg.enablePerOpcodeDecoderErrorMetrics {
		d.stats.inc(opname + "_decoder_error")
	}
	d.readBuf = nil
}

// onDataRequest lands at Task 10; a stub keeps this task compiling:
func (d *requestDecoder) onDataRequest(xid int32, frame []byte) bool {
	_ = xid
	_ = frame
	d.decoderError("")
	return false
}
```
**IMPL note:** the `onDataRequest` stub makes Task-9 data-request tests (e.g. `TestDecodePartialFrameReassembly`, which uses `dataFrame(1, opGetData, …)`) fail — those tests' counter assertions need the Task-10 dispatch. EITHER write only the special-xid + reassembly tests with control frames at Task 9 and move data-frame-based tests to Task 10, OR (preferred — keeps the reassembly/high-water tests in Task 9 where they belong) land a MINIMAL `onDataRequest` at Task 9 (opcode lookup + `<opname>_rq` increment + correlation write, NO min-length table / flag gating) and EXTEND it at Task 10. The minimal Task-9 version:
```go
func (d *requestDecoder) onDataRequest(xid int32, frame []byte) bool {
	opcode := int32(binary.BigEndian.Uint32(frame[4:8]))
	opname, known := wireOpcodeToOpname[opcode]
	if !known {
		d.decoderError("")
		return false
	}
	d.stats.inc(opname + "_rq")
	d.countRequestBytes(opname, wireFootprint(frame))
	d.requestsByXid[xid] = pendingRequest{opname: opname, wireOpcode: opcode, start: time.Now()}
	return true
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/filter/network/zookeeperproxy/ -race -v` → all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/zookeeperproxy/...
git add internal/filter/network/zookeeperproxy/
git commit -m "phase 28.1 Task 9: shallow request decoder part 1 — framing + reassembly + chain-buffer high-water mark + special-xid dispatch (ADR-0222 §4.5; D-S28.1-3; AMEND-A5/A8)"
```

---

## Task 10: `decoder.go` part 2 — data-request dispatch + min-length table + decoder_error + flag gating + correlation (D-S28.1-1)

**Files:**
- Modify: `internal/filter/network/zookeeperproxy/decoder.go`
- Modify: `internal/filter/network/zookeeperproxy/decoder_test.go`

- [ ] **Step 1: Transcribe the per-opcode min-length table (D-S28.1-1)**

Fetch upstream `decoder.cc` at v1.37.2 and extract the per-opcode minimum-length checks the shallow decode mirrors:
```bash
curl -s https://raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/source/extensions/filters/network/zookeeper_proxy/decoder.cc > /tmp/zk_decoder.cc
grep -n "ensureMinLength\|OpCodes::" /tmp/zk_decoder.cc | head -80
```
Per parent §11.4 the universal minimum is `XID_LENGTH + INT_LENGTH` (8 bytes); opcodes whose upstream parse reads payload fields have larger minimums (e.g. getdata reads a path string + watch bool). **D-S28.1-1 resolution:** transcribe each `ensureMinLength` value into a `dataRequestMinLength map[int32]int` table (opcodes absent from the table use the universal 8). The shallow decoder validates length only (no payload extraction) — a frame shorter than its opcode's minimum takes the decoder_error path with the opcode KNOWN (so the flag-gated `<opname>_decoder_error` fires).

- [ ] **Step 2: Write the failing tests**

```go
// --- data-request dispatch (Task 10) ---

// Every wire opcode dispatches to its <opname>_rq counter — incl. the
// digit-suffixed ones (reference_proto_roster_extraction_digits guard).
func TestDecodeDataRequestAllOpcodes(t *testing.T) {
	cases := []struct {
		opcode int32
		suffix string
	}{
		{opGetData, "getdata_rq"}, {opCreate, "create_rq"}, {opCreate2, "create2_rq"},
		{opGetChildren2, "getchildren2_rq"}, {opSetWatches2, "setwatches2_rq"},
		{opGetAllChildrenNumber, "getallchildrennumber_rq"}, {opClose, "close_rq"},
		{opMulti, "multi_rq"}, {opDelete, "delete_rq"}, {opCheckWatches, "checkwatches_rq"},
	}
	for _, tc := range cases {
		t.Run(tc.suffix, func(t *testing.T) {
			d, rs, _ := newTestDecoder(t)
			d.decodeOnData(dataFrame(1, tc.opcode, padTo(tc.opcode)))
			if got := counterValue(t, rs, tc.suffix); got != 1 {
				t.Fatalf("%s = %d, want 1", tc.suffix, got)
			}
		})
	}
}
// padTo returns filler payload bytes meeting dataRequestMinLength for opcode
// (so min-length validation passes); empty when the universal 8-byte min suffices.

// SetAuth as a DATA request (xid > 0, opcode 100) counts via the dynamic auth
// scheme path or auth family — upstream maps SetAuth's opname to "auth"
// (AMEND-A3): assert NO setauth_rq counter exists and the decode does not panic.
func TestDecodeSetAuthDataRequest(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(dataFrame(5, opSetAuth, padTo(opSetAuth)))
	// SetAuth's opname is "auth"; there is no auth_rq macro counter (AMEND-A3).
	// The IMPL mirrors upstream: a SetAuth data request increments the dynamic
	// auth.<scheme>_rq path ONLY when the scheme is parseable; the shallow
	// decoder counts request_bytes + correlation. Pin: decoder_error stays 0.
	if got := counterValue(t, rs, "decoder_error"); got != 0 {
		t.Fatalf("decoder_error = %d, want 0 (SetAuth data request is not an error)", got)
	}
}

// Unknown opcode → decoder_error (no per-opcode counter — opcode unknown).
func TestDecodeUnknownOpcode(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(dataFrame(1, 9999, nil))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
}

// Oversized frame (len > max_packet_bytes) → decoder_error + buffer abandoned;
// later reads decode normally (the 0046 arm-4 unit mirror).
func TestDecodeOversizedThenRecovers(t *testing.T) {
	reg := stats.NewRegistry()
	cfg, _ := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
		MaxPacketBytes: wrapperspb.UInt32(64)})
	rs := newRosterStats(reg, "zk")
	cfg.stats = rs
	d := newRequestDecoder(cfg, rs)
	// Oversized: length prefix says 1000 > 64.
	d.decodeOnData(append(be32(1000), make([]byte, 10)...))
	if got := rs.counters["decoder_error"].Load(); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	// Later read (fresh bytes appended after the abandoned buffer): decodes fine.
	prior := d.chainConsumed
	good := dataFrame(1, opGetData, nil)
	d.decodeOnData(append(make([]byte, prior), good...)) // chain buffer grew by `good`
	if got := rs.counters["getdata_rq"].Load(); got != 1 {
		t.Fatalf("getdata_rq = %d, want 1 (decoder must recover after abandon)", got)
	}
}

// Min-length validation (D-S28.1-1): a known opcode with a too-short frame →
// decoder_error + (flag-gated) <opname>_decoder_error.
func TestDecodeMinLengthViolation(t *testing.T) {
	reg := stats.NewRegistry()
	cfg, _ := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
		EnablePerOpcodeDecoderErrorMetrics: true})
	rs := newRosterStats(reg, "zk")
	cfg.stats = rs
	d := newRequestDecoder(cfg, rs)
	// Pick an opcode with a >8-byte minimum from the transcribed table (e.g.
	// getdata, whose upstream parse needs path-len + watch); send only xid+opcode.
	d.decodeOnData(dataFrame(1, opGetData, nil)) // 8 bytes < getdata's minimum
	if got := rs.counters["decoder_error"].Load(); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if got := rs.counters["getdata_decoder_error"].Load(); got != 1 {
		t.Fatalf("getdata_decoder_error = %d, want 1 (flag enabled + opcode known)", got)
	}
}
// NOTE: if the transcribed table gives getdata an 8-byte minimum, pick another
// opcode with a larger minimum, or drop this test in favor of a synthetic-table
// unit test — the IMPL adapts to the actual upstream values (D-S28.1-1).

// Flag gating (AMEND-A2): _rq_bytes increments ONLY when the flag is true;
// request_bytes increments ALWAYS; the wire footprint includes the 4-byte prefix.
func TestDecodeFlagGatedRequestBytes(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		reg := stats.NewRegistry()
		msg := &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			EnablePerOpcodeRequestBytesMetrics: enabled}
		cfg, _ := parseConfig(msg)
		rs := newRosterStats(reg, "zk")
		cfg.stats = rs
		d := newRequestDecoder(cfg, rs)
		frame := dataFrame(1, opGetData, padTo(opGetData))
		d.decodeOnData(frame)
		wantWire := uint64(len(frame)) // = 4-byte prefix + payload
		if got := rs.counters["request_bytes"].Load(); got != wantWire {
			t.Fatalf("request_bytes = %d, want %d (always counted)", got, wantWire)
		}
		gotGated := rs.counters["getdata_rq_bytes"].Load()
		if enabled && gotGated != wantWire {
			t.Fatalf("getdata_rq_bytes = %d, want %d (flag on)", gotGated, wantWire)
		}
		if !enabled && gotGated != 0 {
			t.Fatalf("getdata_rq_bytes = %d, want 0 (flag off — gates increments not creation)", gotGated)
		}
	}
}

// Correlation structures (R5): data requests land in requestsByXid (insert
// overwrites); control requests append to the per-xid FIFO queue.
func TestDecodeCorrelationStructuresPopulated(t *testing.T) {
	d, _, _ := newTestDecoder(t)
	d.decodeOnData(dataFrame(7, opGetData, padTo(opGetData)))
	pr, ok := d.requestsByXid[7]
	if !ok || pr.opname != "getdata" || pr.wireOpcode != opGetData {
		t.Fatalf("requestsByXid[7] = (%+v, %v), want getdata entry", pr, ok)
	}
	// Insert overwrites (AMEND-A7): same xid again with a different opcode.
	d.decodeOnData(append(dataFrame(7, opGetData, padTo(opGetData)), dataFrame(7, opExists, padTo(opExists))...))
	if d.requestsByXid[7].opname != "exists" {
		t.Fatalf("requestsByXid[7].opname = %q, want exists (insert overwrites)", d.requestsByXid[7].opname)
	}
	// Control FIFO: two pings queue in order.
	d2, _, _ := newTestDecoder(t)
	ping := zkFrame(be32(pingXid), be32(opPing))
	d2.decodeOnData(append(append([]byte{}, ping...), ping...))
	if got := len(d2.controlRequestsByXid[pingXid]); got != 2 {
		t.Fatalf("control queue len = %d, want 2 (FIFO per control xid)", got)
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run 'DataRequest|Unknown|Oversized|MinLength|FlagGated|Correlation' -v`
Expected: the min-length + flag-gated `_decoder_error` tests FAIL (Task-9's minimal `onDataRequest` has no min-length table); others may pass — extend until all listed behaviors are real.

- [ ] **Step 4: Extend `onDataRequest` to the full Task-10 form**

```go
// dataRequestMinLength: per-opcode minimum frame lengths (xid+opcode+required
// payload header), transcribed from upstream decoder.cc ensureMinLength calls
// at v1.37.2 (D-S28.1-1). Opcodes absent here use the universal 8-byte minimum.
var dataRequestMinLength = map[int32]int{
	// ... transcribed values (Step 1) ...
}

func (d *requestDecoder) onDataRequest(xid int32, frame []byte) bool {
	opcode := int32(binary.BigEndian.Uint32(frame[4:8]))
	opname, known := wireOpcodeToOpname[opcode]
	if !known {
		d.decoderError("") // unknown opcode: no per-opcode counter
		return false
	}
	if minLen, ok := dataRequestMinLength[opcode]; ok && len(frame) < minLen {
		d.decoderError(opname) // known opcode: the flag-gated per-opcode counter fires
		return false
	}
	// SetAuth-as-data-request: opname is "auth" (AMEND-A3) — there is no
	// auth_rq macro counter; mirror upstream by counting the dynamic
	// auth.<scheme>_rq when the scheme parses, else only bytes + correlation.
	if opcode == opSetAuth {
		d.onSetAuthDataRequest(xid, frame)
		return true
	}
	d.stats.inc(opname + "_rq")
	d.countRequestBytes(opname, wireFootprint(frame))
	d.requestsByXid[xid] = pendingRequest{opname: opname, wireOpcode: opcode, start: time.Now()}
	return true
}
```
**IMPL note (SetAuth-as-data-request):** upstream's decoder routes SetAuth (opcode 100, positive xid) through `parseAuthRequest` exactly like the AuthXid path, counting the dynamic per-scheme counter. Mirror that: `onSetAuthDataRequest` parses the scheme at offset 8 (after xid+opcode) and increments `auth.<scheme>_rq` + bytes + the DATA correlation map. Verify against the fetched `decoder.cc` (the Step-1 grep shows the dispatch); record the verified behavior in the test.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/filter/network/zookeeperproxy/ -race -v` → ALL pass (Tasks 6–10 cumulative).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/zookeeperproxy/...
git add internal/filter/network/zookeeperproxy/
git commit -m "phase 28.1 Task 10: data-request dispatch + min-length table + decoder_error path + flag gating + correlation writes (ADR-0222 §4.5/§4.6; D-S28.1-1; R5)"
```

---

## Task 11: Filter glue — `zookeeperproxy.go` (TypeURL + `NewFactory` + the both-directions `filter`)

**Files:**
- Create: `internal/filter/network/zookeeperproxy/zookeeperproxy.go`
- Create: `internal/filter/network/zookeeperproxy/zookeeperproxy_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// TypeURL pinning: derived via proto.MessageName (rbac.go:38 precedent), must
// equal the parent §5.1 empirically-pinned literal.
func TestTypeURLViaProtoMessageName(t *testing.T) {
	want := "type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}

// The factory: parses ONCE at boot (ADR-0079); rejects propagate; the instance
// factory allocates fresh per-connection filters sharing the compiled config
// (incl. the eagerly-created roster).
func TestNewFactoryParseAndReject(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)

	// Reject: missing stat_prefix.
	bad := mustAny(t, &zookeeper_proxyv3.ZooKeeperProxy{})
	if _, err := factory(bad, network.FactoryCtx{}); err == nil || !strings.Contains(err.Error(), errStatPrefixRequired) {
		t.Fatalf("factory(no stat_prefix) err = %v, want %q", err, errStatPrefixRequired)
	}

	// Accept: the 201 counters exist at 0 right after parse (eager creation — D-P5).
	good := mustAny(t, &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk"})
	instFactory, err := factory(good, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if got := reg.NewCounterIfAbsent("zk.zookeeper.getdata_resp_fast").Load(); got != 0 {
		t.Fatalf("response-side counter not pre-created at 0 (eager roster)")
	}

	// Two instances share config but have independent decoders.
	f1 := instFactory().(*filter)
	f2 := instFactory().(*filter)
	if f1.cfg != f2.cfg {
		t.Fatal("instances must share the boot-parsed compiledConfig")
	}
	if f1.decoder == f2.decoder {
		t.Fatal("instances must have per-connection decoders")
	}
}

// Malformed Any → "zookeeper_proxy: invalid typed_config: …".
func TestNewFactoryMalformedAny(t *testing.T) {
	factory := NewFactory(stats.NewRegistry())
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff}}
	if _, err := factory(bad, network.FactoryCtx{}); err == nil ||
		!strings.HasPrefix(err.Error(), "zookeeper_proxy: invalid typed_config: ") {
		t.Fatalf("factory(malformed) err = %v, want invalid-typed_config prefix", err)
	}
}

// The filter implements BOTH interfaces (the first both-directions production filter).
var (
	_ network.ReadFilter  = (*filter)(nil)
	_ network.WriteFilter = (*filter)(nil)
)

// OnData feeds the decoder + ALWAYS Continue + NEVER drains the chain buffer (R3).
func TestFilterOnDataPassthroughNeverDrains(t *testing.T) {
	f := newTestFilter(t) // helper: builds via NewFactory + instance factory
	buf := &network.Buffer{}
	buf.Append(dataFrame(1, opGetData, padTo(opGetData)))
	before := buf.Len()
	if got := f.OnData(buf, false); got != network.Continue {
		t.Fatalf("OnData = %v, want Continue (R3 unconditional passthrough)", got)
	}
	if buf.Len() != before {
		t.Fatalf("OnData drained the chain buffer (len %d → %d) — FORBIDDEN (R3)", before, buf.Len())
	}
}

// Multi-read: undrained chain-buffer accumulation does not double-count
// (the filter feeds buf.Bytes() — the FULL buffer — each call; D-S28.1-3).
func TestFilterMultiReadNoDoubleCount(t *testing.T) {
	f := newTestFilter(t)
	buf := &network.Buffer{}
	buf.Append(dataFrame(1, opGetData, padTo(opGetData)))
	f.OnData(buf, false)
	buf.Append(dataFrame(2, opExists, padTo(opExists))) // chain buffer accumulates
	f.OnData(buf, false)
	rs := f.cfg.stats
	if rs.counters["getdata_rq"].Load() != 1 || rs.counters["exists_rq"].Load() != 1 {
		t.Fatalf("counters getdata=%d exists=%d, want 1/1 (no double-count across reads)",
			rs.counters["getdata_rq"].Load(), rs.counters["exists_rq"].Load())
	}
}

// OnWrite is a PURE no-op Continue at 28.1 (SPEC §4.7 pin): no buffering, no
// counting, no mutation.
func TestFilterOnWritePureNoOp(t *testing.T) {
	f := newTestFilter(t)
	buf := &network.Buffer{}
	buf.Append([]byte("response-direction bytes"))
	if got := f.OnWrite(buf, false); got != network.Continue {
		t.Fatalf("OnWrite = %v, want Continue", got)
	}
	if buf.Len() != 24 {
		t.Fatal("OnWrite must not touch the buffer")
	}
	// No counter moved — spot-check decoder_error + response_bytes stay 0.
	rs := f.cfg.stats
	if rs.counters["decoder_error"].Load() != 0 || rs.counters["response_bytes"].Load() != 0 {
		t.Fatal("OnWrite must not increment any counter at 28.1")
	}
}

// OnNewConnection is a no-op Continue (the sticky-halt constraint —
// reference_network_read_filter_onnewconnection_halts).
func TestFilterOnNewConnectionContinue(t *testing.T) {
	f := newTestFilter(t)
	if got := f.OnNewConnection(); got != network.Continue {
		t.Fatalf("OnNewConnection = %v, want Continue (sticky-halt constraint)", got)
	}
}

// Both callback injections are stored; OnDestroy drops the decoder.
func TestFilterCallbacksAndDestroy(t *testing.T) {
	f := newTestFilter(t)
	f.SetReadFilterCallbacks(nil)  // the values are stored verbatim; nil ok in unit
	f.SetWriteFilterCallbacks(nil)
	f.OnDestroy()
	if f.decoder != nil {
		t.Fatal("OnDestroy must drop the per-connection decoder")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run 'TypeURL|NewFactory|Filter' -v`
Expected: FAIL — `TypeURL`/`NewFactory`/`filter` undefined.

- [ ] **Step 3: Create `zookeeperproxy.go`**

```go
package zookeeperproxy

import (
	"errors"
	"fmt"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical Any type URL for zookeeper_proxy's typed_config.
// Derived via proto.MessageName (NEVER a hand-typed docs string —
// reference_network_filter_typeurl_extensions; the rbac.go:38 precedent).
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&zookeeper_proxyv3.ZooKeeperProxy{}))

// NewFactory returns the zookeeperproxy NetworkFilterFactory with the stats
// registry closure-captured (the rbac NewFactory(reg) / D-26.3-3 precedent —
// network.FactoryCtx carries no stats registry). The factory parses + validates
// ONCE at boot (ADR-0079 two-step factory) and EAGERLY creates the 201-counter
// roster per distinct stat_prefix (D-P5 creation parity). The returned
// FilterInstanceFactory allocates a fresh per-connection *filter; all instances
// share the boot-parsed *compiledConfig (counter increments are atomic).
func NewFactory(reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		var msg zookeeper_proxyv3.ZooKeeperProxy
		if tc != nil && len(tc.GetValue()) > 0 {
			if err := tc.UnmarshalTo(&msg); err != nil {
				return nil, fmt.Errorf("zookeeper_proxy: invalid typed_config: %w", err)
			}
		}
		cfg, err := parseConfig(&msg)
		if err != nil {
			return nil, err
		}
		cfg.stats = newRosterStats(reg, cfg.statPrefix)
		return func() network.NetworkFilter {
			return &filter{cfg: cfg, decoder: newRequestDecoder(cfg, cfg.stats)}
		}, nil
	}
}

// filter is the per-connection zookeeper_proxy filter. It implements BOTH
// network.ReadFilter and network.WriteFilter (one instance, both directions —
// the first consumer of the ADR-0221 seam; upstream addFilter parity).
type filter struct {
	network.Marker
	cfg     *compiledConfig // shared, boot-parsed (incl. the roster counters)
	decoder *requestDecoder // per-connection (reassembly buf + correlation structures)
	cb      network.ReadFilterCallbacks
	wcb     network.WriteFilterCallbacks
}

// OnNewConnection is a no-op Continue: an OnNewConnection StopIteration would
// set the chain's sticky connHalted flag and block all OnData
// (reference_network_read_filter_onnewconnection_halts).
func (f *filter) OnNewConnection() network.Status { return network.Continue }

// OnData feeds the decoder with the FULL chain-buffer contents (the decoder's
// high-water mark skips already-consumed bytes — D-S28.1-3) and ALWAYS returns
// Continue. It NEVER drains the chain buffer, never closes, never halts
// (AMEND-A8 unconditional passthrough; R3).
func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.decoder.decodeOnData(buf.Bytes())
	return network.Continue
}

// OnWrite is a PURE no-op Continue at 28.1 (SPEC §4.7 pin). It does NOT buffer
// write-direction bytes: with no response decoder to drain it, a write-side
// reassembly buffer would grow unboundedly on long-lived connections. The
// write-side buffer is created WITH the 28.2 response decoder (ADR-0223). The
// method exists so the filter satisfies WriteFilter and the 0046 fixture's
// traffic exercises the writeChainConn → OnWrite seam end-to-end.
func (f *filter) OnWrite(_ *network.Buffer, _ bool) network.Status { return network.Continue }

// SetReadFilterCallbacks / SetWriteFilterCallbacks store both injections (the
// both-directions dual injection — D-P2/§3.3).
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks)    { f.cb = cb }
func (f *filter) SetWriteFilterCallbacks(cb network.WriteFilterCallbacks) { f.wcb = cb }

// OnDestroy drops the per-connection decoder (the correlation structures +
// reassembly buffer die with the connection). The chain runtime calls this
// exactly once per instance (the §3.3 dedupe).
func (f *filter) OnDestroy() { f.decoder = nil }
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/filter/network/zookeeperproxy/ -race -v` → ALL pass (the package's full unit surface: config + rejects + roster + decoder + glue).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/zookeeperproxy/...
git add internal/filter/network/zookeeperproxy/
git commit -m "phase 28.1 Task 11: zookeeperproxy filter glue — TypeURL via proto.MessageName + NewFactory(reg) + both-directions filter (pure no-op OnWrite stub) (ADR-0222 §4.2/§4.7)"
```

---
## Task 12: The 7th built-in + `bootstrap.go` blank-import + boot smoke

**Files:**
- Modify: `internal/filter/network/builtins/builtins.go` (registration after `snicluster` at `:62`; package doc 6 → 7)
- Modify: `internal/filter/network/builtins/builtins_test.go`
- Modify: `internal/bootstrap/bootstrap.go` (blank-import after `sni_cluster/v3` at `:87`)

- [ ] **Step 1: Write the failing tests**

In `builtins_test.go`:
```go
// Rename + extend the all-built-ins assertion: TestRegisterBuiltinsRegistersAllSix
// (builtins_test.go:37) → TestRegisterBuiltinsRegistersAllSeven, adding
// zookeeperproxy.TypeURL to the asserted set.

func TestRegisterBuiltins_RegistersZookeeperProxy(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: stats.NewRegistry()})
	reg.Freeze()
	if _, ok := reg.Lookup(zookeeperproxy.TypeURL); !ok {
		t.Fatal("zookeeper_proxy not registered as the 7th built-in")
	}
}

// Boot smoke: a [zookeeper_proxy, tcp_proxy] chain resolves through the
// registry; parsing the zookeeper config eagerly creates the 201 counters at 0
// (mirrors the 26.3 Task-12 [rbac_network, tcp_proxy] boot smoke shape).
func TestZookeeperProxyBootSmoke(t *testing.T) {
	sreg := stats.NewRegistry()
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: sreg})
	reg.Freeze()

	factory, ok := reg.Lookup(zookeeperproxy.TypeURL)
	if !ok {
		t.Fatal("zookeeper_proxy factory not found")
	}
	tc := mustAny(t, &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zkboot"})
	instFactory, err := factory(tc, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	inst := instFactory()
	// The both-directions classification: the instance satisfies BOTH interfaces.
	if _, isRead := inst.(network.ReadFilter); !isRead {
		t.Fatal("zookeeper_proxy instance must be a ReadFilter")
	}
	if _, isWrite := inst.(network.WriteFilter); !isWrite {
		t.Fatal("zookeeper_proxy instance must be a WriteFilter")
	}
	// Eager roster: 201 counters exist at 0 (spot-check response-side names).
	for _, name := range []string{"zkboot.zookeeper.getdata_resp", "zkboot.zookeeper.watch_event",
		"zkboot.zookeeper.connect_rq", "zkboot.zookeeper.decoder_error"} {
		if got := sreg.NewCounterIfAbsent(name).Load(); got != 0 {
			t.Errorf("counter %s = %d at boot, want 0", name, got)
		}
	}
}
```
(Reuse the existing `mustAny` helper if `builtins_test.go` has one; add it if not. Use `reg.Lookup` per the existing `TestRegisterBuiltins_RegistersSniCluster` shape at `builtins_test.go:66`.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/builtins/ -run 'AllSeven|Zookeeper' -v`
Expected: FAIL — `zookeeperproxy` import undefined / not registered.

- [ ] **Step 3: Register the 7th built-in + the blank-import**

In `builtins.go`, add the import + the registration after the snicluster line (`:62`):
```go
	// zookeeper_proxy: the 7th built-in (28.1; ADR-0222). Stats-PRIMARY filter:
	// the registry is closure-captured (the rbac_network/D-26.3-3 precedent —
	// FactoryCtx carries no stats registry). The first both-directions
	// (ReadFilter + WriteFilter) production filter (ADR-0221 consumer #1).
	reg.Register(zookeeperproxy.TypeURL, zookeeperproxy.NewFactory(deps.StatsRegistry))
```
Update the package doc (`:1-8`): "six built-in network filters (echo, direct_response, tcp_proxy, HCM, rbac_network, sni_cluster)" → "seven built-in network filters (…, sni_cluster, zookeeper_proxy)".

In `internal/bootstrap/bootstrap.go`, add after the sni_cluster blank-import (`:87`):
```go
	// Phase-28.1 registers the zookeeper_proxy network-filter extension proto so
	// protojson round-trips bootstraps carrying
	// filter_chains[].filters[].typed_config of that type. Registered
	// transitively by the zookeeperproxy filter package too; the explicit
	// blank-import here guarantees resolution in any bootstrap-parsing context
	// (e.g. the differential harness). Per ADR-0016 amendment policy, documented
	// in PROGRESS, not a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
```

- [ ] **Step 4: Confirm main.go needs no parallel edit + run the tests**

Run: `grep -n "reg.Register\|RegisterBuiltins" cmd/envoy-go/main.go`
Expected: only the `builtins.RegisterBuiltins(netReg, …)` call (the D27-S2 confirmation carries forward — single insertion point).
Run: `go test ./internal/filter/network/builtins/ -race -short -v` → all PASS.
Run: `go build ./...` → clean (zookeeper_proxy wired into the boot path).
Run: `go test ./internal/bootstrap/ -race -short` → PASS (the blank-import compiles; bootstrap parse round-trips).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/builtins/ internal/bootstrap/ ; golangci-lint run ./internal/filter/network/builtins/... ./internal/bootstrap/...
git add internal/filter/network/builtins/ internal/bootstrap/bootstrap.go
git commit -m "phase 28.1 Task 12: register zookeeper_proxy as the 7th built-in + bootstrap.go blank-import + boot smoke (201 counters at 0) (ADR-0222 §4.8)"
```

---

## Task 13: The `.zookeeper.` `name.go` INLINE-PREFIX arm (D-P8)

**Files:**
- Modify: `internal/stats/name.go` (the new arm after the `.rbac.` arm at `:226-242`, before the default error at `:243`)
- Modify: `internal/stats/name_test.go`

- [ ] **Step 1: Write the failing tests**

In `name_test.go` (mirror the existing `TestFlattenToProm_*` shapes):
```go
// Phase-28.1 .zookeeper. INLINE-PREFIX arm (ADR-0222; AMEND-A4; D-P8 shape-based).
// Internal <stat_prefix>.zookeeper.<counter> → envoy_<flat>, NO labels (the
// reference's Prometheus exposition is flat with an empty label set).

func TestFlattenToProm_Zookeeper_Basic(t *testing.T) {
	base, labels, err := flattenToProm("zk.zookeeper.getdata_rq")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_zk_zookeeper_getdata_rq" {
		t.Errorf("base = %q, want envoy_zk_zookeeper_getdata_rq", base)
	}
	if len(labels) != 0 {
		t.Errorf("labels = %v, want EMPTY (no label promotion — AMEND-A4)", labels)
	}
}

// The dotted dynamic auth.<scheme>_rq family flattens via full-string
// dot→underscore (counter names MAY contain dots — D-P8).
func TestFlattenToProm_Zookeeper_DottedDynamicAuth(t *testing.T) {
	base, labels, err := flattenToProm("zk.zookeeper.auth.digest_rq")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_zk_zookeeper_auth_digest_rq" || len(labels) != 0 {
		t.Errorf("(%q, %v), want (envoy_zk_zookeeper_auth_digest_rq, [])", base, labels)
	}
}

// Digit-suffixed counter names flatten intact (the digits guard).
func TestFlattenToProm_Zookeeper_DigitSuffixed(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"zk.zookeeper.create2_rq", "envoy_zk_zookeeper_create2_rq"},
		{"zk.zookeeper.getallchildrennumber_rq", "envoy_zk_zookeeper_getallchildrennumber_rq"},
	} {
		base, _, err := flattenToProm(tc.in)
		if err != nil || base != tc.want {
			t.Errorf("flattenToProm(%q) = (%q, %v), want %q", tc.in, base, err, tc.want)
		}
	}
}

// The dot-free-prefix guard: a dotted head before .zookeeper. does NOT match
// the arm — falls through to the default unrecognized-prefix error.
func TestFlattenToProm_Zookeeper_DottedPrefixRejected(t *testing.T) {
	if _, _, err := flattenToProm("a.b.zookeeper.getdata_rq"); err == nil {
		t.Fatal("dotted prefix must NOT match the .zookeeper. arm (D-P8 shape guard)")
	}
}

// Underscore-bearing stat_prefixes are fine (the prefix lands in the metric name).
func TestFlattenToProm_Zookeeper_UnderscorePrefix(t *testing.T) {
	base, _, err := flattenToProm("zk_flags.zookeeper.getdata_rq_bytes")
	if err != nil || base != "envoy_zk_flags_zookeeper_getdata_rq_bytes" {
		t.Errorf("got (%q, %v)", base, err)
	}
}

// Non-zookeeper unrecognized names STILL error (the default branch is preserved).
// The existing TestFlattenToProm_Invalid_NoMatchingRule (name_test.go:140) must stay green.
```
(Match the as-built `flattenToProm` signature — `(string) (string, []Label, error)` per the existing tests; adjust if the in-tree signature differs.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/stats/ -run TestFlattenToProm_Zookeeper -v`
Expected: FAIL — the names hit the default unrecognized-prefix error (`name.go:243`).

- [ ] **Step 3: Add the arm to `name.go`**

Insert AFTER the `.rbac.` arm (`name.go:242`), BEFORE the default error (`:243`) — the SPEC §7.4 code verbatim:
```go
	// Phase-28.1 zookeeper_proxy INLINE-PREFIX detection (ADR-0222; parent AMEND-A4;
	// the ADR-0138 bandwidth_limit + wasm.* permissive-shape precedents). Internal
	// name <stat_prefix>.zookeeper.<counter> (counter MAY contain dots — the dynamic
	// auth.<scheme>_rq family) flattens to envoy_<stat_prefix>_zookeeper_<counter>
	// with NO label promotion (upstream applies no tag extraction to this filter;
	// its Prometheus exposition is flat with an empty label set).
	// Validation is the SHAPE (D-P8): a .zookeeper. segment with a dot-free head —
	// no per-counter allowlist (201 static names + an open-ended dynamic family
	// make an allowlist unmaintainable; the wasm. arm at :88-122 is the
	// established permissive precedent). Documented acceptance: any future stat
	// named <x>.zookeeper.<y> from another subsystem matches this arm.
	// KEEP-IN-SYNC: internal/filter/network/zookeeperproxy/stats.go (the roster).
	const zkSegment = ".zookeeper."
	if idx := strings.Index(internal, zkSegment); idx > 0 {
		prefix := internal[:idx]
		if !strings.ContainsRune(prefix, '.') {
			base = "envoy_" + strings.ReplaceAll(internal, ".", "_")
			return base, nil, nil
		}
	}
```
(Adapt the local-variable names — `internal`, `base` — to the as-built `flattenToProm` body; the `.rbac.` arm at `:226-242` shows the exact local conventions.)

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/stats/ -race -v` → ALL pass (the new zookeeper tests + every existing flattening test, incl. `TestFlattenToProm_Invalid_NoMatchingRule` — the default error is preserved for non-matching names).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/stats/ ; golangci-lint run ./internal/stats/...
git add internal/stats/name.go internal/stats/name_test.go
git commit -m "phase 28.1 Task 13: the .zookeeper. Prometheus inline-prefix arm (shape-based, no allowlist; dotted dynamic auth names; dot-free-prefix guard) (ADR-0222 §7.4; D-P8)"
```

---

## Task 14: The 37th fuzzer — `FuzzZookeeperRequestDecode`

**Files:**
- Create: `internal/filter/network/zookeeperproxy/fuzz_test.go`

- [ ] **Step 1: Write the fuzzer**

Mirror the rbac fuzzer shape (`internal/filter/network/rbac/fuzz_test.go` — seed corpus + invariant assertions):
```go
package zookeeperproxy

import (
	"bytes"
	"testing"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/stats"
)

// FuzzZookeeperRequestDecode is the 37th fuzzer (parent §11.10 / SPEC §15.1
// Layer C). It feeds arbitrary bytes through the production decodeOnData entry
// point and asserts the three safety invariants:
//  1. no panic;
//  2. the input slice (the chain buffer stand-in) is NEVER mutated (R3);
//  3. the decoder-internal reassembly buffer stays bounded by
//     max_packet_bytes + the frame-header overhead (no unbounded growth).
func FuzzZookeeperRequestDecode(f *testing.F) {
	// Seed corpus: a valid connect frame, a ping, a data request, garbage, an
	// oversized length prefix, a partial frame.
	seedRO := false
	f.Add(connectSeed(&seedRO))
	f.Add(zkFrameSeed(be32(pingXid), be32(opPing)))
	f.Add(dataFrameSeed(1, opGetData, []byte("/path")))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0x00})
	f.Add(append(be32(1<<30), make([]byte, 16)...))
	f.Add(dataFrameSeed(1, opCreate, nil)[:6])

	f.Fuzz(func(t *testing.T, data []byte) {
		const maxPkt = 1024 // small bound so the invariant is exercised by short inputs
		reg := stats.NewRegistry()
		cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
			StatPrefix:     "fuzz",
			MaxPacketBytes: wrapperspb.UInt32(maxPkt),
		})
		if err != nil {
			t.Fatal(err)
		}
		rs := newRosterStats(reg, "fuzz")
		cfg.stats = rs
		d := newRequestDecoder(cfg, rs)

		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit — a panic fails the fuzz run).
		d.decodeOnData(data)
		// Feed cumulatively a second time (the chain buffer accumulates) — the
		// high-water mark must not re-process or panic.
		d.decodeOnData(append(append([]byte(nil), data...), data...))

		// Invariant 2: the input was never mutated (R3).
		if !bytes.Equal(data, orig) {
			t.Fatal("decodeOnData mutated the chain bytes")
		}

		// Invariant 3: the internal reassembly buffer is bounded.
		if len(d.readBuf) > maxPkt+8 {
			t.Fatalf("readBuf grew to %d bytes, want <= max_packet_bytes(%d)+8", len(d.readBuf), maxPkt)
		}
	})
}
```
(The seed helpers `connectSeed`/`zkFrameSeed`/`dataFrameSeed` re-export the decoder_test.go builders for fuzz seeds — or call the test builders directly if Go's fuzzer accepts them in the same package; they are in the same `zookeeperproxy` test package, so direct reuse works: `f.Add(connectFrame(nil))` etc. Drop the wrapper names if direct reuse compiles.)

- [ ] **Step 2: Run the fuzzer (short corpus + a bounded fuzz run)**

```bash
go test ./internal/filter/network/zookeeperproxy/ -run FuzzZookeeperRequestDecode -v          # seed corpus only
go test ./internal/filter/network/zookeeperproxy/ -fuzz FuzzZookeeperRequestDecode -fuzztime 30s
```
Expected: seed corpus PASS; the 30s fuzz run finds no crashers. (If a crasher is found: fix the decoder, add the crasher as a regression seed, re-run — the standard fuzz triage loop.)

- [ ] **Step 3: Verify the fuzzer count 36 → 37**

```bash
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 37
```

- [ ] **Step 4: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/zookeeperproxy/...
git add internal/filter/network/zookeeperproxy/fuzz_test.go
git commit -m "phase 28.1 Task 14: FuzzZookeeperRequestDecode — the 37th fuzzer (no-panic + no-chain-mutation + bounded internal buffer)"
```

---

## Task 15: `TCPSink` BackendKind runner plumbing + `0046` driver part 1 (bootstraps + frame builders + wiring)

**Files:**
- Modify: `test/differential/fixture/fixture.go` (TCPSink = 28 after `HTTPWasmPerRoute = 27` at `:492`)
- Modify: `test/differential/runner_test.go` (the `case fixture.TCPSink:` arm + `acceptSinkCounting` + the `0046` driver blank-import)
- Create: `test/fixtures/0046-zookeeper-requests/driver/driver.go` (part 1)

- [ ] **Step 1: Add the `TCPSink` BackendKind**

In `fixture.go`, after `HTTPWasmPerRoute BackendKind = 27` (`:492`):
```go
	// TCPSink is a SILENT TCP backend: it accepts connections, drains all reads
	// (io.Copy(io.Discard, conn)), NEVER writes, and closes when the client
	// closes (read-until-EOF; D-S28.1-5). Added at 28.1 for
	// 0046-zookeeper-requests (SPEC §8.1.1): an echoing backend would push the
	// echoed ZK request bytes back through reference Envoy's onWrite response
	// decoder — counting *_resp/decoder_error increments that envoy-go's 28.1
	// OnWrite no-op stub never mirrors → cross-side stat divergence. The 0046
	// backend MUST be silent. (28.2's 0048 uses a driver-controlled responder —
	// a separate kind; TCPSink stays request-side-only.)
	TCPSink BackendKind = 28
```

- [ ] **Step 2: Add the runner backend arm + accept loop**

In `runner_test.go`, add a case to the backend-kind switch (after the `case fixture.HTTPWasm:` arm, mirroring the `case fixture.TCPEcho` shape at `:151-163`):
```go
		case fixture.TCPSink:
			// Silent sink backend (28.1 §8.1.1): accept + drain + never write.
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptSinkCounting(ln, bo.accepts)
```
And the accept loop next to `acceptEchoCounting` (`:1219`):
```go
// acceptSinkCounting accepts connections, counts them, drains all reads, and
// NEVER writes (the TCPSink backend — 28.1 §8.1.1; D-S28.1-5 read-until-EOF).
func acceptSinkCounting(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			_, _ = io.Copy(io.Discard, c)
		}(c)
	}
}
```
(Match the TCPEcho arm's exact field assignments — `bo.ln`/`bo.port` — against the as-built switch; the explore shows `ln`-then-counter-goroutine. Add the `0046` driver blank-import to the runner's import block after the `0045` line: `_ "github.com/esalaine/envoy-go/test/fixtures/0046-zookeeper-requests/driver"`.)

- [ ] **Step 3: Author `0046` driver part 1**

`test/fixtures/0046-zookeeper-requests/driver/driver.go` — mirror the `0043` driver structure (`test/fixtures/0043-network-rbac/driver/driver.go`, the closest cross-side StatsAsserter + MultiListenerDriver template). Part 1 lands:

1. **Package doc** — the fixture taxonomy (the 7 arms, §8.1.3), the TCPSink rationale, the StatsAsserter-as-load-bearing-proof note, the cross-references (SPEC §8.1, the `0043` precedent, `reference_differential_asserter_dispatch`).
2. **Constants** — `fixtureName = "0046-zookeeper-requests"`; `refAdminPort = 9901`; reference listener ports `refLPlainPort = 15046` / `refLFlagsPort = 15047` (the "150NN" convention).
3. **Frame-crafting helpers (D-S28.1-4 resolution: small builder funcs, readable + reusable by the 28.2 `0048` driver):**
```go
func be32(v int32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, uint32(v)); return b }
func be64(v int64) []byte { b := make([]byte, 8); binary.BigEndian.PutUint64(b, uint64(v)); return b }

// zkFrame prepends the 4-byte BE length prefix (excluding itself) to the
// concatenated parts.
func zkFrame(parts ...[]byte) []byte {
	payload := bytes.Join(parts, nil)
	return append(be32(int32(len(payload))), payload...)
}

// connectFrame: protocol_version(0) + last_zxid(0) + timeout(30000) + session(0)
// + 16-byte password [+ readonly]. The 48-byte connect frame of SPEC §8.1.3 arm 1.
func connectFrame(readonly bool) []byte { … }

// dataFrame: xid + opcode + payload.
func dataFrame(xid, opcode int32, payload []byte) []byte { … }

// pingFrame: xid −2 + opcode 11.
func pingFrame() []byte { … }
```
   Wire opcodes are duplicated as local constants in the driver (the driver package cannot import `internal/` packages — the fixture drivers are standalone; mirror the values from the SPEC §5.3 wire enum: getdata=4, create=1, create2=15, getchildren2=12, setwatches2=105, close=−11).
4. **The two bootstraps** — `ReferenceBootstrap(backendPorts []int) string` + `SubjectConfig(...)` (mirror the `0043` YAML shapes at `:160-199`): TWO listeners each with chain `[zookeeper_proxy, tcp_proxy]` → one cluster `c_sink` → the runner's TCPSink backend. Listener configs:
   - `l_plain`: `stat_prefix: zk_plain`, all flags default (false).
   - `l_flags`: `stat_prefix: zk_flags`, `enable_per_opcode_request_bytes_metrics: true`, `enable_per_opcode_decoder_error_metrics: true`.
   The zookeeper `@type` carries the `extensions.` segment: `type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy`.
5. **Driver interface wiring** — `BackendCount() 1`; `SubjectListenerName()/SubjectListenerNames()` (`l_plain`, `l_flags`); `ReferenceListenerPort()/ReferenceListenerPorts()`; `BackendKind() fixture.TCPSink` (the `BackendKindAware` impl); `ProbeAdmin` (the `0043` shape); `DriveReference`/`DriveSubject` delegating to `DriveReferenceMulti`/`DriveSubjectMulti` → a `driveProxy` stub that part 2 (Task 16) fills with the 7 arms; `init()` calling `fixture.RegisterFixture(fixtureName, &zkRequestsDriver{})`.
6. **Interface assertions** (the `0043:633-637` shape):
```go
var (
	_ fixture.Driver              = (*zkRequestsDriver)(nil)
	_ fixture.MultiListenerDriver = (*zkRequestsDriver)(nil)
	_ fixture.StatsAsserter       = (*zkRequestsDriver)(nil)
	_ fixture.BackendKindAware    = (*zkRequestsDriver)(nil)
)
```
(The `StatsAsserter` assertion forces Task 16's `AssertStats` to exist — declare a stub `AssertStats` in part 1 that calls `t.Fatal("AssertStats lands at Task 16")`, OR defer the interface assertion line to Task 16. Prefer deferring the assertion line: part 1 must compile + the fixture must NOT register a vacuously-passing asserter.)

- [ ] **Step 4: Compile + verify the runner discovers the fixture (still red — driver incomplete)**

```bash
go vet ./test/... && go build ./test/...
go test ./test/differential/ -run 'TestDifferential/0046-zookeeper-requests' -v 2>&1 | head -30
```
Expected: compiles; the differential subtest runs and FAILS or produces empty-drive output (the driveProxy arms land at Task 16). Honest record: this task ends with the fixture discovered-but-red; Task 16 turns it green. (If running the incomplete fixture would fail the suite for other developers, guard `driveProxy` to return a deterministic placeholder verdict and note that the fixture is not yet asserting — the dispatch-constraint memory: one dir = one runner branch, and this dir's branch is cross-side.)

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l test/ ; golangci-lint run ./test/...
git add test/differential/fixture/fixture.go test/differential/runner_test.go test/fixtures/0046-zookeeper-requests/
git commit -m "phase 28.1 Task 15: TCPSink BackendKind=28 runner plumbing + 0046 driver part 1 (bootstraps + frame builders + wiring) (SPEC §8.1.1; D-S28.1-4/5)"
```

---

## Task 16: `0046` driver part 2 — the 7 arms + `AssertStats` + deliberate-break (the fixture goes green)

**Files:**
- Modify: `test/fixtures/0046-zookeeper-requests/driver/driver.go`
- Create: `test/fixtures/0046-zookeeper-requests/README.md`

- [ ] **Step 1: Implement `driveProxy` — the 7 arms (SPEC §8.1.3)**

Each arm drives BOTH sides identically (via `DriveReferenceMulti`/`DriveSubjectMulti` → `driveProxy(ctx, addrs, side)`), emitting side-independent per-arm verdict lines (`arm <name> sent=<n> verdict=<v>\n` — the `0043` verdict-line precedent; the side label EXCLUDED so equivalent behavior produces identical bytes):

| # | Arm | Listener | Drive | Expected counters (asserted in Step 2) |
|---|---|---|---|---|
| 1 | connect | `l_plain` | one `connectFrame(false)` on a fresh conn | `zk_plain.zookeeper.connect_rq` == 1 |
| 2 | multi-opcode | `l_plain` | one conn: connect + ping + getdata(xid 1) + create(xid 2) + close(xid 3), separate writes with ~50ms inter-write delays | `connect_rq`==2 (cumulative w/ arm 1), `ping_rq`/`getdata_rq`/`create_rq`/`close_rq` each == 1; `request_bytes` EQUAL cross-side |
| 3 | digit-suffixed | `l_plain` | one conn: create2(15) + getchildren2(12) + setwatches2 — sent as a SetWatches2 DATA request (xid 4, wire op 105) | `create2_rq`/`getchildren2_rq`/`setwatches2_rq` each == 1 |
| 4 | garbage + survival | `l_plain` | one conn: a frame whose length prefix exceeds 1 MiB → pause ≥200ms → a valid getdata frame | `decoder_error` == 1 both sides; `getdata_rq` == 2 (cumulative); connection NOT closed |
| 5 | flag-gated | `l_flags` | one conn: a getdata frame | `zk_flags.zookeeper.getdata_rq_bytes` > 0 AND equal cross-side; `zk_plain.zookeeper.getdata_rq_bytes` == 0 both sides |
| 6 | exists-at-zero | both | no traffic (assertion-only) | `getdata_resp`, `getdata_resp_fast`, `watch_event`, `response_bytes` PRESENT and == 0 on both sides for both prefixes |
| 7 | deliberate-break | — | recorded procedure (Step 4) | — |

**Cumulative-counter discipline:** the arms run in order over shared listeners, so later arms assert CUMULATIVE values (e.g. arm 4's getdata is the second getdata on `l_plain`). The expected-value table in `AssertStats` is written once, post-workload — exactly the `0043` model. **Arm-3 note:** setwatches2 (wire op 105) is a data request (positive xid); the special-xid setwatches (−8) is arm-2 adjacent but NOT driven here — keep arm 3 purely data-request digit-suffixed opcodes.

- [ ] **Step 2: Implement `AssertStats` (the StatsAsserter — the load-bearing proof)**

Mirror `0043`'s `AssertStats`/`scrapeRBACStats`/`parseRBACPromBody` (`driver.go:328-461`) with the `_zookeeper_` infix:
```go
func (d *zkRequestsDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	refStats, err := scrapeZkStats(refAdminAddr)   // GET /stats/prometheus; keep _zookeeper_ lines
	if err != nil { t.Fatalf("scrape ref: %v", err) }
	subjStats, err := scrapeZkStats(subjAdminAddr)
	if err != nil { t.Fatalf("scrape subj: %v", err) }

	// Per SPEC §8.1.2: BOTH sides scraped via /stats/prometheus; expected
	// counters looked up via the FLATTENED form envoy_<prefix>_zookeeper_<suffix>
	// (no labels — AMEND-A4). A name-shape mismatch on either side makes the
	// lookup miss → the assertion fails → R7 Prometheus parity is intrinsic.
	type expect struct {
		metric string // dotted internal form: <prefix>.zookeeper.<suffix>
		want   int64
	}
	expectations := []expect{
		{"zk_plain.zookeeper.connect_rq", 2},
		{"zk_plain.zookeeper.ping_rq", 1},
		{"zk_plain.zookeeper.getdata_rq", 2},
		{"zk_plain.zookeeper.create_rq", 1},
		{"zk_plain.zookeeper.close_rq", 1},
		{"zk_plain.zookeeper.create2_rq", 1},
		{"zk_plain.zookeeper.getchildren2_rq", 1},
		{"zk_plain.zookeeper.setwatches2_rq", 1},
		{"zk_plain.zookeeper.decoder_error", 1},
		// exists-at-zero (arm 6; creation parity D-P5/R2):
		{"zk_plain.zookeeper.getdata_resp", 0},
		{"zk_plain.zookeeper.getdata_resp_fast", 0},
		{"zk_plain.zookeeper.watch_event", 0},
		{"zk_plain.zookeeper.response_bytes", 0},
		// flag-gating (arm 5):
		{"zk_plain.zookeeper.getdata_rq_bytes", 0},  // flag OFF on l_plain
		{"zk_flags.zookeeper.getdata_rq", 1},
	}
	for _, side := range []struct {
		label string
		stats map[string]int64
	}{{"ref", refStats}, {"subj", subjStats}} {
		for _, exp := range expectations {
			got, present := lookupZkCounter(side.stats, exp.metric)
			if !present {
				t.Errorf("%s: counter %s ABSENT (creation parity / name-shape failure)", side.label, exp.metric)
				continue
			}
			if got != exp.want {
				t.Errorf("%s %s = %d, want %d", side.label, exp.metric, got, exp.want)
			}
		}
	}
	// Cross-side EQUALITY assertions (no fixed expected value — the value must
	// simply agree): request_bytes (arm 2's byte-accounting proof) and the
	// flag-gated zk_flags getdata_rq_bytes (arm 5).
	for _, metric := range []string{"zk_plain.zookeeper.request_bytes", "zk_flags.zookeeper.getdata_rq_bytes"} {
		refV, refOK := lookupZkCounter(refStats, metric)
		subjV, subjOK := lookupZkCounter(subjStats, metric)
		if !refOK || !subjOK || refV != subjV || refV == 0 {
			t.Errorf("cross-side %s: ref=(%d,%v) subj=(%d,%v), want equal and > 0", metric, refV, refOK, subjV, subjOK)
		}
	}
}
```
Add the `fixture.StatsAsserter` interface assertion (deferred from Task 15).

- [ ] **Step 3: Author the README**

Document: the 7-arm taxonomy; the TCPSink-not-echo rationale (SPEC §8.1.1); the both-sides-`/stats/prometheus` scrape mechanics + R7 intrinsic parity; the cumulative-counter discipline; the cross-side request_bytes equality proof; the body-differential-is-vacuous note (the stat comparison IS the proof); the deliberate-break procedure.

- [ ] **Step 4: Run the fixture cross-side + the deliberate-break proof (R4)**

```bash
go test ./test/differential/ -run 'TestDifferential/0046-zookeeper-requests' -v
```
Expected: PASS — drive verdict lines byte-identical across sides AND `AssertStats` green on both sides.

Deliberate-break (R4; the `0030` dead-assertion lesson — record in PROGRESS.md + README):
1. Temporarily change the `connect_rq` expectation to 3 → the fixture MUST FAIL on both runner paths → revert.
2. Temporarily comment out the `name.go` `.zookeeper.` arm → arm-6/all subject-side lookups MUST report ABSENT → the fixture FAILS → revert.
Record both break/revert outputs honestly.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l test/fixtures/0046-zookeeper-requests/ ; golangci-lint run ./test/fixtures/0046-zookeeper-requests/...
git add test/fixtures/0046-zookeeper-requests/ docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md
git commit -m "phase 28.1 Task 16: 0046-zookeeper-requests cross-side StatsAsserter fixture — 7 arms green + deliberate-break recorded (SPEC §8.1; R2/R3/R4/R7)"
```

---

## Task 17: `0047-zookeeper-boot-reject` fixture

**Files:**
- Create: `test/fixtures/0047-zookeeper-boot-reject/driver/driver.go`
- Create: `test/fixtures/0047-zookeeper-boot-reject/README.md`
- Modify: `test/differential/runner_test.go` (the `0047` driver blank-import)

- [ ] **Step 1: Author the driver**

Mirror `0044-network-rbac-boot-reject/driver/driver.go` (220 LoC; the symmetric BootRejectFixture precedent). The bootstrap: a `[zookeeper_proxy, tcp_proxy]` chain whose zookeeper `typed_config` has **NO `stat_prefix`** + a minimal unused cluster (the zero-cluster boot reject + `reference_network_filter_typeurl_extensions`):

- `ReferenceBootstrap` / `SubjectConfig` — the same chain shape as `0046`'s `l_plain` minus the `stat_prefix` field.
- `BootRejectScript() string` → `""` (inline config; the `0044:159` shape).
- `ExpectedBootErrorSubstring() string` → `"stat_prefix"` — case-sensitive, present in BOTH the reference's PGV violation text (`StatPrefix: value length must be at least 1`-style — the substring `stat_prefix` appears in the field path of the PGV error) AND in envoy-go's `zookeeper_proxy: stat_prefix is required` (Task 7's byte-stable arm). **IMPL note:** verify the reference's actual stderr contains the literal lowercase `stat_prefix` during the first run; if the PGV text renders the field differently (e.g. `StatPrefix`), choose the longest common case-sensitive substring present in both (the `0044` precedent settled on `stat_prefix` for the rbac PGV mirror — the same PGV violation class, so the same substring is expected to hold).
- `DriveReference`/`DriveSubject` → never called on the boot-reject path (the `0044:137-141` no-op shape).
- Symmetric mode: implement `differential.BootRejectFixture` (`harness.go:340-352`), NOT `SubjectOnlyBootRejectFixture` — BOTH sides must reject.
- `init()` → `fixture.RegisterFixture("0047-zookeeper-boot-reject", &zkBootRejectDriver{})`; runner blank-import.
- Per `reference_differential_fixture_dispatch_constraint`: this dir is the BOOT-REJECT branch (one dir = one runner branch) — no cross-side arms in this dir.

- [ ] **Step 2: Author the README**

Document: the missing-`stat_prefix` PGV-mirror arm (SPEC §6.1; the load-bearing `0047` arm); the symmetric-reject discipline; the latency PGV-mirror arms' disposition (unit-test-only at 28.1; their fixture disposition is parent D-P4 = the 28.2 SPEC's decision); the common stderr substring choice.

- [ ] **Step 3: Run the fixture**

```bash
go test ./test/differential/ -run 'TestDifferential/0047-zookeeper-boot-reject' -v
```
Expected: PASS — both sides reject at boot; both stderrs contain `stat_prefix`.

- [ ] **Step 4: Run the FULL differential suite (back-compat gate R1 + the new dirs)**

```bash
go test ./test/differential/ -run TestDifferential -v 2>&1 | tail -60
```
Expected: **49/49 PASS** — the 47 existing dirs (the seam's R1 back-compat regression gate: zero-write-filter chains byte-identical) + `0046` + `0047`. Quote the output into PROGRESS.md honestly (the KNOWN environmental ephemeral-port-bind flake passes on retry — record if it occurs, not a regression).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l test/fixtures/0047-zookeeper-boot-reject/ ; golangci-lint run ./test/fixtures/0047-zookeeper-boot-reject/...
git add test/fixtures/0047-zookeeper-boot-reject/ test/differential/runner_test.go docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md
git commit -m "phase 28.1 Task 17: 0047-zookeeper-boot-reject symmetric fixture + full 49-dir differential suite green (SPEC §8.2; R1)"
```

---

## Task 18: Completion bundle — BEHAVIOR_CONTRACT + ADR bodies + STATE/ROADMAP + six-gate

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0221 + ADR-0222 §Decision/§Consequences bodies — IN PLACE per ADR-0044)
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `next-prompt.txt`
- Modify: `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT.md 28.1 bundle (SPEC §9/§14 — ONE atomic edit)**

1. NEW `### Network filter chain framework — WriteFilter seam (28.1 amendment)` block: the write-direction dispatch + REVERSE order (AMEND-A11) + StopIteration-no-forward (documented-unsupported-by-consumers) + the terminal-originated-writes-only boundary (§3.7 / D-P3) + the write-only-filter boot boundary (§3.6) + the `writeChainConn` composition (`writeChainConn(prefixConn(conn))`) + zero-write-filter back-compat (R1).
2. NEW `### envoy.filters.network.zookeeper_proxy` subsection: request-side semantics (§4.5); the 201-counter roster + creation parity (§4.4 / D-P5); the `<stat_prefix>.zookeeper.` scope (AMEND-A1); the Prometheus inline-prefix flattening (§7.4 / D-P8); the dynamic per-scheme auth counters (AMEND-A3); the shallow-decode leniency departure (payload-malformed → `<op>_rq` not `decoder_error`); the dynamic-metadata coverage boundary (AMEND-A9); the `access_log` parse-accept-ignore note; the parsed-not-consumed latency-field note (28.2 forward-pointer).
3. Stat table: 136 → **337** (the 201 new rows).
4. The 28.2 forward-pointer note (response decoder + correlation consumption + latency counters + the latency-histogram coverage boundary + the parent-row-28 ROLLUP).

- [ ] **Step 2: ADR-0221 + ADR-0222 §Decision/§Consequences bodies (ADR-0044 in-place)**

Fill IN PLACE (DECISIONS.md tail STAYS **ADR-0223**; next-free STAYS **ADR-0224**; NO new ADR number consumed):
- **ADR-0221** (`DECISIONS.md:14226`) — the `network.WriteFilter` seam: the §3.1 interfaces (`OnDestroy` ON `WriteFilter`; separate `SetWriteFilterCallbacks`; both-directions dual injection); the §3.3 classification restructure (independent type-asserts; once-per-instance destroy dedupe); the §3.4 reverse dispatch; the §3.5 `writeChainConn` + D-P7 return semantics; the §3.6 write-only boot boundary (manager.go untouched); the §3.7 terminal-originated-writes-only boundary (D-P3); CONSUMES the ADR-0213 §Decision item 8 API-revision allowance (consumer #1 zookeeper_proxy; anticipated #2 mongo_proxy).
- **ADR-0222** (`DECISIONS.md:14245`) — the `zookeeper_proxy` request side: TypeURL via proto.MessageName; `NewFactory(reg)` closure-capture; the 9-field parse + proto→wire mapping; the 201-counter eager roster + D-P5; the shallow decoder + D-P2 + the high-water mark (D-S28.1-3); the correlation structures (R5); the dynamic auth counters; the name.go arm + D-P8; the fixtures + the TCPSink pin; the 37th fuzzer; the AMEND-A9 metadata deferral.

- [ ] **Step 3: STATE.md + ROADMAP.md + next-prompt.txt**

ROADMAP: sub-row 28.1 `in-progress → done` (append the IMPL-DONE note: fixtures 47→49, stats 136→337, fuzzers 36→37, ADR-0221/0222 bodies landed); **parent row 28 STAYS `in-progress`** (the ROLLUP is 28.2's per the parent precedent); 28.2 STAYS `planned`. STATE.md: `active-phase` → `phase 28.1 IMPL done`; `next-skill` → `superpowers:brainstorming`-equivalent per SKILL_ROUTING for the **28.2 SPEC** (the per-sub-phase precedent: the next session authors the 28.2 sub-phase SPEC); counts updated (fixtures **49** [tail `0047`], stats **337**, fuzzers **37**, DECISIONS tail **ADR-0223**, next-free **ADR-0224**); `last-commit` filled at squash. next-prompt.txt rewritten for the 28.2-SPEC cold-start.

- [ ] **Step 4: The six-gate (SPEC §15.2) — run LIVE, quote into PROGRESS.md**

```bash
go build ./...                       # clean
go vet ./...                         # clean
golangci-lint run                    # clean
go test ./... -race -short           # green (all packages)
# FULL differential suite: 49/49 byte-exact green (incl. the 47-dir R1 back-compat gate)
go test ./test/differential/ -run TestDifferential -v
# h2spec 53/53 + proxy-wasm conformance 10/10 re-run LIVE (asserted-unaffected — phase 28
#   touches no HTTP path; HCM's chain has zero write filters so even the seam is inert there —
#   but re-confirmed since the harness is available)
```
Confirm + quote: stat surface **337** (+201), fixtures **49**, fuzzers **37**, DECISIONS tail **ADR-0223**, next-free **ADR-0224**. All outputs quoted honestly into PROGRESS.md (per `superpowers:verification-before-completion`).

- [ ] **Step 5: Commit the bundle**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md next-prompt.txt docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md
git commit -m "phase 28.1 Task 18: completion bundle — BEHAVIOR_CONTRACT WriteFilter-seam block + zookeeper_proxy subsection (136→337); ADR-0221/0222 bodies in-place; ROADMAP sub-row 28.1 done; six gates GREEN LIVE [ADR-0221,ADR-0222]"
```

---

## Test surface summary (SPEC §15.1)

- **Layer A — framework unit** (`internal/filter/network/`): writeCallbacks accessor (Task 2); classification read/write/both/terminal + dual injection + OnDestroy-once (Task 3); writeChainConn forward / stop-no-forward / dispatch-order / post-chain-bytes / error-propagation / endStream-false (Task 4); handleTerminal wrap composition (4 conn shapes) + reverse dispatch + zero-write-filter back-compat + chain-order-not-mutated (Task 5).
- **Layer A — zookeeperproxy unit**: config parse 9 fields + defaults + proto→wire mapping + opname table (Task 6); all PARSE-REJECT arms + byte-stable wording (Task 7); the 201-suffix roster golden list + eager/idempotent creation + dynamic auth (Task 8); special-xid decode + reassembly + high-water mark + input-not-mutated (Task 9); data-request dispatch + min-length + decoder_error/recovery + flag gating + correlation population (Task 10); TypeURL pin + factory parse/reject + filter glue (OnData-never-drains R3 / OnWrite-pure-no-op / multi-read no-double-count) (Task 11).
- **Layer A — stats unit** (`internal/stats/`): the `.zookeeper.` arm — basic / dotted-dynamic-auth / digit-suffixed / dotted-prefix-rejected / underscore-prefix / default-error-preserved (Task 13).
- **Layer A — builtins**: all-seven registration + zookeeper registration + the both-interfaces boot smoke + the 201-counters-at-0 assertion (Task 12).
- **Layer C — fuzz**: `FuzzZookeeperRequestDecode` (no-panic + no-chain-mutation + bounded internal buffer) — the 37th fuzzer (Task 14).
- **Layer D — differential**: `0046` cross-side StatsAsserter 7 arms + deliberate-break (Tasks 15–16); `0047` symmetric boot-reject (Task 17); the FULL 47-dir back-compat gate (R1) → 49/49 (Tasks 17–18).
- **Layer E — race**: `go test -race -short` across all touched packages, per task + at the six-gate.

## Acceptance checklist (SPEC §15.3 — verified at Task 18)

- [ ] The `WriteFilter` seam lands per SPEC §3 (interfaces + classification + reverse dispatch + `writeChainConn` + the §3.6/§3.7 boundaries); `manager.go`/`tcp_proxy`/HCM untouched; all 47 existing fixtures byte-exact green (R1).
- [ ] The `zookeeperproxy` package lands per SPEC §4 (config parse + the 201-counter eager roster + the shallow request decoder + the correlation structures + the dynamic auth counters + the pure no-op OnWrite stub).
- [ ] The 7th built-in + `bootstrap.go` blank-import + the `.zookeeper.` name.go arm land (SPEC §4.8/§7.4).
- [ ] Fixtures `0046` + `0047` green (incl. the `TCPSink` BackendKind + the recorded deliberate-breaks); the 37th fuzzer lands; counts: fixtures 47→49, fuzzers 36→37, stats 136→337 (R6).
- [ ] ADR-0221 + ADR-0222 §Decision/§Consequences bodies land in place (DECISIONS.md tail STAYS ADR-0223; no new number consumed); the BEHAVIOR_CONTRACT 28.1 bundle lands (SPEC §14).
- [ ] Six gates green LIVE + quoted into PROGRESS.md; STATE.md advanced; ROADMAP sub-row 28.1 `in-progress → done`; parent row 28 STAYS `in-progress`; next-prompt.txt rewritten for the 28.2-SPEC cold-start.
