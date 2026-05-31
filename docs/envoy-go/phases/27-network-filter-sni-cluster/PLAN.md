# Phase 27 PLAN — `sni_cluster` network filter + the connection-scoped upstream-cluster-override seam

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended, per `feedback_execution_style`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Subagents commit **LOCAL-ONLY** (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on the touched packages before its commit (`feedback_pertask_gofmt_lint`).

**Goal:** Land `sni_cluster` (`envoy.filters.network.sni_cluster`) — a config-less L4 read-filter that publishes the TLS SNI verbatim as a per-connection upstream-cluster-override — at full upstream parity, in one flat phase, with the terminal `tcp_proxy` made per-connection-cluster-resolving.

**Architecture:** A NARROW typed per-connection cluster-override string carried on the per-connection `chainRuntime` (NOT a general filter-state primitive — Q2/YAGNI; ADR-0219). `sni_cluster` writes it in `OnNewConnection` via a new `SetUpstreamCluster` setter ON the `ReadFilterCallbacks` interface; the framework threads it to the terminal `tcp_proxy` at `Handle` dispatch via the call `ctx` (`handleTerminal` wraps the ctx iff an override is set; `tcp_proxy` reads `network.UpstreamClusterOverride(ctx)`). `tcp_proxy` retains its `*cluster.Manager` + boot-resolved default cluster and resolves override-then-fallback per connection; unknown override cluster → downstream close / zero bytes; no-override path byte-exact with master tip. No new package; no new third-party dependency.

**Tech stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy v1.37.2 (ADR-0008); go-control-plane v1.32.4 (ADR-0008). Reuses `internal/filter/network/` (26.1/26.2/26.3 chain framework + `TerminalFilter` seam), `internal/filter/tcpproxy/`, `internal/cluster/`.

---

## ADR-0045 split-gate re-check (at PLAN time, per ADR-0045 §6)

The gate fires at `> ~25 tasks OR > ~1500 net-new LoC`. This PLAN decomposes to **9 tasks** / **~180–400 net-new LoC** (SPEC §3.0 envelope, unchanged at PLAN time):

| Unit | Net-new LoC |
|---|---|
| `snicluster` filter package (config-less parse + `OnNewConnection` + no-op `OnData`/`OnDestroy` + registration) | ~80–160 |
| Override seam (`chainRuntime` field + `SetUpstreamCluster` on `ReadFilterCallbacks` + concrete impl + the `UpstreamClusterOverride` ctx accessor + `handleTerminal` threading) | ~40–100 |
| `tcp_proxy` per-connection resolution (retain manager + default; override-then-fallback at `Handle`; unknown→close) | ~60–140 |

Both axes comfortably under the gate → **NO split. Single flat phase 27.** (Test artifacts + the new fixture are not counted against the LoC gate.)

## PLAN-time D-question dispositions (SPEC §12)

- **D27-S1** (the `0045` dir number + the fallback-arm shape) — Task 1 re-pins the next-free fixture dir against the IMPL-session tip (working = `0045`; tail at master tip = `0044-network-rbac-boot-reject`). The fallback arm is expressed as a **TLS connection with NO SNI** to a listener whose chain IS `[sni_cluster, tcp_proxy]` with a configured fallback cluster — this proves the `sni_cluster` empty-SNI no-op AND the `tcp_proxy` configured-cluster fallback in one arm (cleaner + wire-deterministic vs a no-`sni_cluster` chain, which would not exercise `sni_cluster` at all). Final wire-shape (one vs two listeners) is an IMPL Task-7 detail; the dir count stays +1 regardless.
- **D27-S2** (main.go parity) — **RESOLVED at PLAN: `cmd/envoy-go/main.go` delegates the network built-ins WHOLLY to `builtins.RegisterBuiltins`** (verified: `main.go:222` calls `builtins.RegisterBuiltins(netReg, …)` with no explicit per-filter list). The 6th registration is a SINGLE insertion point in `internal/filter/network/builtins/builtins.go` — **no parallel main.go edit**. Task 6 confirms via grep.
- **D27-S3** (unknown-override close) — RESOLVED at PLAN: the `tcp_proxy.Handle` early-`return` BEFORE the Dial lets the existing `defer downstream.Close()` (FIN) fire → zero application bytes on the wire → body-differential byte-exact vs Envoy's `NoFlush` (RST). **No `SO_LINGER` plumbing on the terminal `Handle` path** (the framework `NoFlush` plumbing is for the read-filter close site, not the terminal-owned conn).
- **D27-S4** (fuzzer) — RESOLVED at PLAN: **DEFER.** `sni_cluster`'s parse is byte-identical to `echo` (config-less, accept empty/any), and `echo` carries no dedicated fuzzer. Fuzzers stay **36**.

---

## File Structure

**Created:**
- `internal/filter/network/snicluster/snicluster.go` — the config-less `sni_cluster` filter (`TypeURL` via `proto.MessageName`; `New`; `filter` type; `OnNewConnection`/`OnData`/`SetReadFilterCallbacks`/`OnDestroy`).
- `internal/filter/network/snicluster/snicluster_test.go` — parse (empty/any/malformed) + `OnNewConnection` SNI→override (live-assert) + empty-SNI no-op + `OnData` pass-through.
- `internal/filter/network/upstreamcluster.go` — the unexported `upstreamClusterKey` + `withUpstreamClusterOverride` + the exported `UpstreamClusterOverride(ctx)` accessor.
- `internal/filter/network/upstreamcluster_test.go` — the ctx accessor round-trip (present/absent) + `handleTerminal` wraps-iff-set.
- `test/fixtures/0045-sni-cluster/driver/driver.go` + `README.md` (+ `pki/` if a fresh keypair set is needed; reuse `0002-tls-tcp/pki` shape) — the 3-arm cross-side TLS fixture.

**Modified:**
- `internal/filter/network/callbacks.go:16` — add `SetUpstreamCluster(name string)` to the `ReadFilterCallbacks` interface.
- `internal/filter/network/chain.go:127` — add the `upstreamClusterOverride string` field to `chainRuntime`; `chain.go:209` `handleTerminal` wraps the ctx iff override set; `chain.go:321` concrete `*callbacks` gains `SetUpstreamCluster`.
- `internal/filter/tcpproxy/filter.go:26` — `Filter` struct (`cluster` → `cm` + `defaultCluster`); `:47` `NewFilter` stores both; `:94` `Handle` resolves override-then-fallback.
- `internal/filter/network/builtins/builtins.go:42` — the 6th `reg.Register(snicluster.TypeURL, snicluster.New)`.
- `internal/filter/network/builtins/builtins_test.go` — extend the all-built-ins assertion (5 → 6).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` / `DECISIONS.md` / `STATE.md` / `ROADMAP.md` — the completion bundle (Task 9).

---

## Task 1: First-action baselines/anchors gate (no code change)

**Files:** none modified — this is a verification + re-pin gate run at the IMPL-session tip.

- [ ] **Step 1: Re-confirm the project counts at the IMPL-session tip**

Run (from repo root):
```bash
git log --oneline -1
# fixtures (canonical: count numbered dirs under test/fixtures/):
ls -d test/fixtures/[0-9]* | wc -l            # expect 46; tail dir:
ls -d test/fixtures/[0-9]* | tail -1          # expect test/fixtures/0044-network-rbac-boot-reject
# DECISIONS.md tail + next-free:
grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -2   # expect ADR-0219 ADR-0220 → tail ADR-0220, next-free ADR-0221
```
Expected: fixtures **46** (tail `0044-network-rbac-boot-reject`); DECISIONS.md tail **ADR-0220** (next-free **ADR-0221**). **NOTE:** the SPEC §10 Task-1 row text reads "DECISIONS.md tail ADR-0218 (this SPEC drafts 0219/0220)" — that line was written PRE-SPEC-commit; the SPEC is now committed, so the LIVE tail is **ADR-0220**. Re-pin to ADR-0220 here.

- [ ] **Step 2: Re-confirm the stat surface = 136 (+0 this phase)**

Run the project's canonical stat-surface recipe (the same grep/command STATE.md/BEHAVIOR_CONTRACT.md use to report 136 — do NOT invent a new count). Expected: **136**. `sni_cluster` adds none (§7.1); the `downstream_cx_*` family stays unmirrored (§7.2).

- [ ] **Step 3: Re-confirm the fuzzer count = 36 (+0 this phase)**

Use the project's canonical fuzzer-accounting (per STATE.md — NOT a raw `grep '^func Fuzz'`, which over-counts grouped fuzzers). Expected: **36**. Fuzzer DEFERRED (D27-S4).

- [ ] **Step 4: Re-confirm `proto.MessageName` + the as-built line anchors**

```bash
# proto @type (must carry the extensions. segment — reference_network_filter_typeurl_extensions):
cat > /tmp/tu.go <<'EOF'
package main
import ( "fmt"; "google.golang.org/protobuf/reflect/protoreflect"; "google.golang.org/protobuf/proto"
  sni "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3" )
func main() { var _ protoreflect.Message; fmt.Println(proto.MessageName(&sni.SniCluster{})) }
EOF
go run /tmp/tu.go   # expect: envoy.extensions.filters.network.sni_cluster.v3.SniCluster
```
Expected `proto.MessageName` = `envoy.extensions.filters.network.sni_cluster.v3.SniCluster` → `TypeURL` = `type.googleapis.com/` + that. Confirm the as-built anchors still hold (drift here re-points the later tasks): `chain.go:127` `chainRuntime` struct; `chain.go:209` `handleTerminal`; `chain.go:321` concrete `*callbacks`; `callbacks.go:16` `ReadFilterCallbacks`; `tcpproxy/filter.go:26` `Filter` struct, `:47` `NewFilter`, `:94` `Handle`; `builtins/builtins.go:42` `RegisterBuiltins`. (The package import path for the binding is `…/sni_cluster/v3`, package identifier `sni_clusterv3`.)

- [ ] **Step 5: Commit the gate record**

No code changed; record the re-pinned baselines in PROGRESS.md (created this task). Commit:
```bash
git add docs/envoy-go/phases/27-network-filter-sni-cluster/PROGRESS.md
git commit -m "phase 27 Task 1: baselines/anchors gate — fixtures 46 (tail 0044), stats 136, fuzzers 36, DECISIONS tail ADR-0220 (next-free 0221); proto.MessageName + line-anchors re-pinned"
```

---

## Task 2: The override field + `SetUpstreamCluster` on `ReadFilterCallbacks` + concrete impl

**Files:**
- Modify: `internal/filter/network/callbacks.go:16` (add to interface)
- Modify: `internal/filter/network/chain.go:127` (struct field), `chain.go:321` (concrete impl)
- Test: `internal/filter/network/chain_test.go` (white-box, `package network`)

- [ ] **Step 1: Write the failing test**

In `internal/filter/network/chain_test.go`, add a test proving a filter that calls `cb.SetUpstreamCluster("foo.example.com")` lands the value on the runtime's `upstreamClusterOverride` field:
```go
func TestSetUpstreamClusterStoresOverride(t *testing.T) {
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.cb.SetUpstreamCluster("foo.example.com")
	if got := rt.upstreamClusterOverride; got != "foo.example.com" {
		t.Fatalf("upstreamClusterOverride = %q, want %q", got, "foo.example.com")
	}
}
```
(Use the existing test conn helper in `chain_test.go` — `fakeConn`/equivalent. If none takes `nil` filters, pass `[]ReadFilter{}`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/network/ -run TestSetUpstreamClusterStoresOverride -v`
Expected: FAIL — `rt.cb.SetUpstreamCluster` undefined (method missing) AND `rt.upstreamClusterOverride` undefined (field missing) → compile error.

- [ ] **Step 3: Add the struct field**

In `chain.go`, inside the `chainRuntime` struct (after `rcd string` at `:135`), add:
```go
	// upstreamClusterOverride is the per-connection upstream-cluster-override a
	// read filter (sni_cluster, 27) publishes; "" = no override. It is the NARROW
	// typed stand-in for Envoy's PerConnectionCluster filter-state entry (key
	// "envoy.tcp_proxy.cluster"; ADR-0219) — NOT a general filter-state primitive
	// (Q2/YAGNI). handleTerminal threads it to the terminal filter via the call ctx.
	upstreamClusterOverride string
```

- [ ] **Step 4: Add the interface method**

In `callbacks.go`, add to the `ReadFilterCallbacks` interface (after `DynamicMetadata()` at `:29`):
```go
	// SetUpstreamCluster publishes a per-connection upstream-cluster-override that
	// the terminal filter (tcp_proxy) consumes to replace its configured cluster
	// (ADR-0219). The NARROW typed stand-in for Envoy's
	// connection().streamInfo().filterState()->setData("envoy.tcp_proxy.cluster", …)
	// (Q2 — no general filter-state primitive). Set by sni_cluster (27) in
	// OnNewConnection with the verbatim SNI; "" is a no-op (leaves the configured
	// cluster). Last writer wins (Envoy's PerConnectionCluster is Mutable).
	SetUpstreamCluster(name string)
```

- [ ] **Step 5: Add the concrete impl**

In `chain.go`, after `SetResponseCodeDetails` (`:355`), add:
```go
// SetUpstreamCluster records the per-connection upstream-cluster-override on the
// runtime (ADR-0219). sni_cluster (27) calls it from OnNewConnection with the
// verbatim SNI; handleTerminal threads it to the terminal filter via the ctx.
func (c *callbacks) SetUpstreamCluster(name string) { c.rt.upstreamClusterOverride = name }
```

- [ ] **Step 6: Run the test + the package suite**

Run: `go test ./internal/filter/network/ -run TestSetUpstreamClusterStoresOverride -v` → PASS.
Run: `go test ./internal/filter/network/ -race -short` → PASS (no other consumer breaks; on-interface addition forces all in-package doubles to compile — fix any test double in `chain_test.go`/`terminal_test.go`/`types_test.go` that implements `ReadFilterCallbacks` to add the no-op method if they are concrete structs rather than using `*callbacks`).

- [ ] **Step 7: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/callbacks.go internal/filter/network/chain.go internal/filter/network/chain_test.go
git commit -m "phase 27 Task 2: SetUpstreamCluster on ReadFilterCallbacks + chainRuntime.upstreamClusterOverride field (ADR-0219 writer seam)"
```

---

## Task 3: The `UpstreamClusterOverride` ctx accessor + `handleTerminal` threading

**Files:**
- Create: `internal/filter/network/upstreamcluster.go`
- Create: `internal/filter/network/upstreamcluster_test.go`
- Modify: `internal/filter/network/chain.go:209` (`handleTerminal`)

- [ ] **Step 1: Write the failing tests**

`internal/filter/network/upstreamcluster_test.go` (`package network`):
```go
func TestUpstreamClusterOverrideRoundTrip(t *testing.T) {
	ctx := withUpstreamClusterOverride(context.Background(), "foo.example.com")
	got, ok := UpstreamClusterOverride(ctx)
	if !ok || got != "foo.example.com" {
		t.Fatalf("UpstreamClusterOverride = (%q,%v), want (foo.example.com,true)", got, ok)
	}
}

func TestUpstreamClusterOverrideAbsent(t *testing.T) {
	got, ok := UpstreamClusterOverride(context.Background())
	if ok || got != "" {
		t.Fatalf("UpstreamClusterOverride = (%q,%v), want (\"\",false)", got, ok)
	}
}

// handleTerminal must wrap the ctx with the override iff one is set, and pass it
// through unchanged otherwise. A recording terminal captures the ctx it receives.
func TestHandleTerminalThreadsOverrideWhenSet(t *testing.T) {
	rec := &recordingTerminal{}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.upstreamClusterOverride = "foo.example.com"
	rt.handleTerminal(context.Background())
	if got, ok := UpstreamClusterOverride(rec.gotCtx); !ok || got != "foo.example.com" {
		t.Fatalf("terminal ctx override = (%q,%v), want (foo.example.com,true)", got, ok)
	}
}

func TestHandleTerminalNoOverrideLeavesCtxClean(t *testing.T) {
	rec := &recordingTerminal{}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.handleTerminal(context.Background())
	if got, ok := UpstreamClusterOverride(rec.gotCtx); ok || got != "" {
		t.Fatalf("terminal ctx override = (%q,%v), want (\"\",false) when no override set", got, ok)
	}
}
```
Add the recording terminal double (embed `Marker`):
```go
type recordingTerminal struct {
	Marker
	gotCtx context.Context
}

func (r *recordingTerminal) Handle(ctx context.Context, _ net.Conn) { r.gotCtx = ctx }
```
(If `terminal_test.go` already has a recording terminal double, reuse it instead of declaring a duplicate.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run 'UpstreamClusterOverride|HandleTerminal' -v`
Expected: FAIL — `withUpstreamClusterOverride` / `UpstreamClusterOverride` undefined; the threading assertion fails (handleTerminal does not yet wrap).

- [ ] **Step 3: Create the accessor file**

`internal/filter/network/upstreamcluster.go`:
```go
// internal/filter/network/upstreamcluster.go — the connection-scoped
// upstream-cluster-override ctx channel (ADR-0219). handleTerminal wraps the
// terminal call ctx with the override a read filter (sni_cluster) published;
// the terminal filter (tcp_proxy) reads it via UpstreamClusterOverride. This is
// the reader half of the narrow override seam (the writer half is
// ReadFilterCallbacks.SetUpstreamCluster → chainRuntime.upstreamClusterOverride).

package network

import "context"

type upstreamClusterKey struct{}

// withUpstreamClusterOverride returns ctx carrying override for the terminal
// filter to read via UpstreamClusterOverride. Internal to the framework's
// terminal handoff; not part of the public filter API.
func withUpstreamClusterOverride(ctx context.Context, override string) context.Context {
	return context.WithValue(ctx, upstreamClusterKey{}, override)
}

// UpstreamClusterOverride returns the per-connection upstream-cluster-override a
// read filter published (ADR-0219), or ("", false) if none. tcp_proxy reads it
// at Handle to override its configured cluster.
func UpstreamClusterOverride(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(upstreamClusterKey{}).(string)
	return v, ok
}
```

- [ ] **Step 4: Thread it in `handleTerminal`**

In `chain.go`, modify `handleTerminal` (`:209`) to wrap the ctx iff an override is set, before `rt.terminal.Handle`:
```go
func (rt *chainRuntime) handleTerminal(ctx context.Context) {
	conn := rt.conn
	if rt.buf.Len() > 0 {
		prefix := make([]byte, rt.buf.Len())
		copy(prefix, rt.buf.Bytes())
		rt.buf.Drain(rt.buf.Len())
		conn = newPrefixConn(rt.conn, prefix)
	}
	if rt.upstreamClusterOverride != "" {
		ctx = withUpstreamClusterOverride(ctx, rt.upstreamClusterOverride)
	}
	rt.terminal.Handle(ctx, conn)
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/filter/network/ -run 'UpstreamClusterOverride|HandleTerminal' -v` → PASS.
Run: `go test ./internal/filter/network/ -race -short` → PASS.

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/upstreamcluster.go internal/filter/network/upstreamcluster_test.go internal/filter/network/chain.go
git commit -m "phase 27 Task 3: UpstreamClusterOverride ctx accessor + handleTerminal threads override iff set (ADR-0219 reader seam)"
```

---

## Task 4: `tcp_proxy` per-connection cluster resolution (override-then-fallback; unknown→close; back-compat)

**Files:**
- Modify: `internal/filter/tcpproxy/filter.go:26` (struct), `:47` (`NewFilter`), `:94` (`Handle`)
- Test: `internal/filter/tcpproxy/filter_test.go`

- [ ] **Step 1: Write the failing test (the back-compat regression sentinel)**

**Test-home split (resolved here; do NOT add a production-exported setter).** The override-carrying ctx is produced ONLY by the unexported `network.withUpstreamClusterOverride` (keyed by an unexported type — §3.2 keeps the seam framework-internal). So a `tcpproxy`-package test CANNOT build an override ctx, and a `network`-package test CANNOT import `tcpproxy` (that cycles: `tcpproxy` imports `network`). Therefore:
  - **Task 4** (here) tests only the **no-override back-compat** path in `tcpproxy/filter_test.go` — a plain `context.Background()` carries no override → `eff = defaultCluster` → directly constructible, no seam access needed. This is the regression sentinel for the struct refactor.
  - The **override-routes** + **unknown-close** end-to-end assertions live in **Task 6's `builtins` package test** — `internal/filter/network/builtins/` already imports BOTH `network` and `tcpproxy` with no cycle, and drives the FULL production path (`network.NewChainRuntime([sniClusterFilter, tcpProxyFilter], …, ConnFacts{ServerName: …})` → `OnNewConnection` → `OnData` → `HandleTerminal`), so the override flows through the real seam. (`sniClusterFilter` only exists after Task 5, so these naturally belong in Task 6.)

In `filter_test.go`, add the back-compat test (build a 2-cluster manager so the refactored struct is exercised, but drive with NO override):
```go
// Empty/absent override → configured default cluster (byte-exact back-compat).
// This is the regression sentinel: it must stay green across the struct refactor.
func TestHandle_NoOverrideUsesDefaultCluster(t *testing.T) {
	// Configured cluster = "bar" (default); manager also knows "foo".
	// ctx = context.Background() (NO override) → bytes reach the "bar" backend.
}
```

- [ ] **Step 2: Run to verify failure / baseline**

Run: `go test ./internal/filter/tcpproxy/ -run TestHandle_NoOverrideUsesDefaultCluster -v`
Expected before the refactor: this exercises the existing default-cluster path; it should PASS against the current code (it is the back-compat anchor). Write it FIRST and confirm green pre-refactor, then keep it green through the refactor (deliberate-break discipline: it is the regression sentinel).

- [ ] **Step 3: Refactor the struct + `NewFilter`**

In `filter.go`, change the `Filter` struct (`:26`):
```go
type Filter struct {
	network.Marker
	cm             *cluster.Manager // retained for per-connection override resolution (ADR-0219)
	defaultCluster *cluster.Cluster // the boot-resolved configured cluster (no-override fallback)
	statPrefix     string
	dm             *drain.Manager
}
```
In `NewFilter` (`:65`), change the success return to store both:
```go
		return &Filter{cm: cm, defaultCluster: c, statPrefix: msg.GetStatPrefix(), dm: dm}, nil
```
(`cm` is already a `NewFilter` parameter. Grep `f.cluster` to find every reference — there are exactly two, both in `Handle` — and rewrite them to `eff` in Step 4. `NewNetworkFactory` is unchanged: it already passes `cm` to `NewFilter`.)

- [ ] **Step 4: Per-connection resolution in `Handle`**

Rewrite `Handle` (`:94`) to resolve the effective cluster after the ctx-cancel check, before the drain Inc:
```go
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	if err := ctx.Err(); err != nil {
		return
	}
	eff := f.defaultCluster
	if override, ok := network.UpstreamClusterOverride(ctx); ok && override != "" {
		c, found := f.cm.Get(override)
		if !found {
			// F-NOROUTE (D27-4): unknown override cluster → close downstream, zero
			// bytes. Envoy increments downstream_cx_no_route + NoFlush-closes; that
			// counter family is NOT mirrored (§7.2). The deferred downstream.Close()
			// (FIN) yields zero-byte body parity regardless of FIN-vs-RST (D27-S3).
			log.Printf("tcpproxy: per-connection override cluster %q not found", override)
			return
		}
		eff = c
	}
	if f.dm != nil {
		f.dm.Inc()
		defer f.dm.Dec()
	}
	upstream, _, err := eff.Dial(ctx)
	if err != nil {
		log.Printf("tcpproxy: dial cluster %q: %v", eff.Name(), err)
		return
	}
	defer func() { _ = upstream.Close() }()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(netConn{upstream}, netConn{downstream}); halfClose(upstream) }()
	go func() { defer wg.Done(); _, _ = io.Copy(netConn{downstream}, netConn{upstream}); halfClose(downstream) }()
	wg.Wait()
}
```
No-override path: `UpstreamClusterOverride` returns `("", false)` → `eff = f.defaultCluster` → byte-identical to today (only `f.cluster` renamed to `eff`/`defaultCluster`). The `override != ""` guard is defense-in-depth (redundant-by-design with `sni_cluster`'s `sni != ""` guard — §3.3; keep both deliberately).

- [ ] **Step 5: Run the tcp_proxy suite (back-compat regression gate)**

Run: `go test ./internal/filter/tcpproxy/ -race -short -v`
Expected: ALL existing tests PASS (`TestHandle_BidirectionalEcho`, `TestFilter_Handle_TLSUpstreamTransparent`, `TestHandle_DialFailure_ClosesDownstream`, `TestTCPProxy_DrainInflightBalance`, the `NewFilter_*` parse tests, `network_factory_test.go`) + the new `TestHandle_NoOverrideUsesDefaultCluster`. These are the strongest proof the per-connection-resolution change is non-regressive (§3.3 / §8.3).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/tcpproxy/ ; golangci-lint run ./internal/filter/tcpproxy/...
git add internal/filter/tcpproxy/filter.go internal/filter/tcpproxy/filter_test.go
git commit -m "phase 27 Task 4: tcp_proxy per-connection cluster resolution — retain cm + defaultCluster; Handle override-then-fallback + unknown→close; no-override byte-exact (ADR-0219)"
```

---

## Task 5: The `internal/filter/network/snicluster/` config-less filter

**Files:**
- Create: `internal/filter/network/snicluster/snicluster.go`
- Create: `internal/filter/network/snicluster/snicluster_test.go`

- [ ] **Step 1: Write the failing tests**

`snicluster_test.go` (`package snicluster`). Mirror `echo`'s test shape + a live `OnNewConnection` assertion. Use a fake `network.ReadFilterCallbacks` double that records `SetUpstreamCluster` calls and returns a configurable SNI from `Connection().RequestedServerName()`:
```go
func TestTypeURLHasExtensionsSegment(t *testing.T) {
	want := "type.googleapis.com/envoy.extensions.filters.network.sni_cluster.v3.SniCluster"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}

func TestNew_AcceptsEmptyAndAbsentConfig(t *testing.T) {
	for _, tc := range []*anypb.Any{nil, {}, mustAny(t, &sni_clusterv3.SniCluster{})} {
		if _, err := New(tc, network.FactoryCtx{}); err != nil {
			t.Fatalf("New(%v) error = %v, want nil", tc, err)
		}
	}
}

func TestNew_MalformedAnyRejected(t *testing.T) {
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	if _, err := New(bad, network.FactoryCtx{}); err == nil {
		t.Fatal("New(malformed) error = nil, want non-nil")
	} // error must begin "sni_cluster: invalid typed_config: "
}

// LIVE assertion: a non-empty SNI must produce a verbatim SetUpstreamCluster call.
func TestOnNewConnection_SetsOverrideFromSNI(t *testing.T) {
	cb := &fakeCB{sni: "foo.example.com"}
	f := newFilterForTest(t)           // builds via New(...)() and SetReadFilterCallbacks(cb)
	if got := f.OnNewConnection(); got != network.Continue {
		t.Fatalf("OnNewConnection status = %v, want Continue", got)
	}
	if cb.setCalls != 1 || cb.lastSet != "foo.example.com" {
		t.Fatalf("SetUpstreamCluster calls=%d last=%q, want 1 / foo.example.com", cb.setCalls, cb.lastSet)
	}
}

// Empty SNI → NO SetUpstreamCluster call (no-op), still Continue.
func TestOnNewConnection_EmptySNINoOp(t *testing.T) {
	cb := &fakeCB{sni: ""}
	f := newFilterForTest(t)
	if got := f.OnNewConnection(); got != network.Continue {
		t.Fatalf("status = %v, want Continue", got)
	}
	if cb.setCalls != 0 {
		t.Fatalf("SetUpstreamCluster called %d times on empty SNI, want 0", cb.setCalls)
	}
}

// OnData is a pass-through Continue (does not drain, does not halt).
func TestOnData_PassThroughContinue(t *testing.T) {
	f := newFilterForTest(t)
	buf := &network.Buffer{}
	buf.Append([]byte("hello"))
	if got := f.OnData(buf, false); got != network.Continue {
		t.Fatalf("OnData status = %v, want Continue", got)
	}
	if buf.Len() != 5 {
		t.Fatalf("OnData drained the buffer (len=%d), want pass-through (5)", buf.Len())
	}
}
```
The `fakeCB` implements `network.ReadFilterCallbacks` (`Connection()` returns a `fakeConn` whose `RequestedServerName()` returns `cb.sni`; `SetUpstreamCluster` records calls; `ContinueReading`/`DynamicMetadata` no-op). **The live SetUpstreamCluster assertion is the proof the call is non-vacuous (per `reference_differential_asserter_dispatch` discipline — prove the write is live).**

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/snicluster/ -v`
Expected: FAIL — package does not compile (`snicluster.go` not yet created).

- [ ] **Step 3: Write the filter**

`snicluster.go`:
```go
// Package snicluster implements envoy.filters.network.sni_cluster — a config-less
// L4 read filter that publishes the TLS SNI verbatim as the per-connection
// upstream-cluster-override (ADR-0220). It reads Connection().RequestedServerName()
// in OnNewConnection and, when non-empty, calls ReadFilterCallbacks.SetUpstreamCluster
// (the narrow override seam, ADR-0219); the terminal tcp_proxy consumes it to
// override its configured cluster. Mirrors echo's config-less parse shape.
package snicluster

import (
	"fmt"

	sni_clusterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
)

// TypeURL is the canonical Any type URL for sni_cluster's typed_config. Derived
// from proto.MessageName (NOT hand-typed) so the extensions. segment is exact
// (reference_network_filter_typeurl_extensions). SniCluster is an EMPTY message.
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&sni_clusterv3.SniCluster{}))

// New is the NetworkFilterFactory for sni_cluster, registered at boot under
// TypeURL. The proto (sni_clusterv3.SniCluster) has no fields; an empty/absent
// typed_config is accepted (echo shape). A structurally-malformed Any body
// surfaces "sni_cluster: invalid typed_config: %w".
func New(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
	if tc != nil && len(tc.GetValue()) > 0 {
		if err := tc.UnmarshalTo(&sni_clusterv3.SniCluster{}); err != nil {
			return nil, fmt.Errorf("sni_cluster: invalid typed_config: %w", err)
		}
	}
	return func() network.NetworkFilter { return &filter{} }, nil
}

// filter publishes the SNI as the per-connection upstream-cluster-override.
type filter struct {
	network.Marker
	cb network.ReadFilterCallbacks
}

// OnNewConnection reads the SNI and, when non-empty, publishes it verbatim as the
// per-connection upstream-cluster-override (F-SNI). It MUST return Continue: an
// OnNewConnection StopIteration would set the chain's sticky connHalted flag and
// block all OnData (reference_network_read_filter_onnewconnection_halts). Envoy's
// filter also returns Continue (D27-6), so parity and the constraint coincide.
func (f *filter) OnNewConnection() network.Status {
	if sni := f.cb.Connection().RequestedServerName(); sni != "" {
		f.cb.SetUpstreamCluster(sni) // verbatim — no transform (F-SNI)
	}
	return network.Continue
}

// OnData is a pass-through Continue — sni_cluster does not inspect payload; it
// passes bytes through so the chain reaches the terminal (unlike echo, which
// halts/drains). Makes [sni_cluster, tcp_proxy] a mixed read→terminal chain.
func (f *filter) OnData(*network.Buffer, bool) network.Status { return network.Continue }

// SetReadFilterCallbacks stores the per-connection callbacks handle.
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks) { f.cb = cb }

// OnDestroy is a no-op; sni_cluster holds no per-connection resources.
func (f *filter) OnDestroy() {}
```
(Confirm against the Task-1 `proto.MessageName` output that `TypeURL` resolves to the expected string. If the project prefers a `const TypeURL` over a `var`, the IMPL may hard-code the verified string with a test pinning it to `proto.MessageName` — Task-1/Step-1-test `TestTypeURLHasExtensionsSegment` guards either way. `echo`/`tcpproxy` use `const`; mirror that with the value verified live, and keep the pinning test.)

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/filter/network/snicluster/ -race -short -v` → all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/snicluster/ ; golangci-lint run ./internal/filter/network/snicluster/...
git add internal/filter/network/snicluster/
git commit -m "phase 27 Task 5: sni_cluster config-less filter — TypeURL via proto.MessageName; OnNewConnection SNI→SetUpstreamCluster+Continue; OnData pass-through (ADR-0220)"
```

---

## Task 6: Register as the 6th built-in + boot smoke + the end-to-end override integration tests

**Files:**
- Modify: `internal/filter/network/builtins/builtins.go:42` (+ package doc 5→6)
- Modify: `internal/filter/network/builtins/builtins_test.go`

- [ ] **Step 1: Write the failing tests**

In `builtins_test.go`, extend the all-built-ins assertion to include `snicluster.TypeURL` (5 → 6) and add a dedicated registration test:
```go
func TestRegisterBuiltins_RegistersSniCluster(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{})
	reg.Freeze()
	if _, ok := reg.Lookup(snicluster.TypeURL); !ok {
		t.Fatal("sni_cluster not registered as the 6th built-in")
	}
}
```
Add the two **end-to-end override integration tests** here (the `builtins` package imports both `network` and `tcpproxy` with NO cycle — the home deferred from Task 4). Each builds a real `[*snicluster.filter-equivalent, *tcpproxy.Filter]` chain via `network.NewChainRuntime`, then drives the chain and asserts routing. Since `snicluster.filter` is unexported, drive it through `snicluster.New(nil, network.FactoryCtx{})()` to get a `network.NetworkFilter`; build a `*tcpproxy.Filter` via `tcpproxy.NewFilter(...)`; supply the SNI via `network.ConnFacts{ServerName: "foo.example.com"}` to `NewChainRuntime`:
```go
// SNI naming an existing cluster → tcp_proxy routes to that cluster's backend.
func TestSniClusterOverrideRoutesEndToEnd(t *testing.T) { /* foo→foo backend sentinel */ }

// SNI naming an unknown cluster → downstream closed, zero application bytes.
func TestSniClusterUnknownOverrideClosesEndToEnd(t *testing.T) { /* ghost→EOF, 0 bytes */ }
```
(These exercise the FULL production path — `sni_cluster` write → `handleTerminal` ctx-thread → `tcp_proxy` resolve — through the unexported seam, proving the override is live cross-package. This is the home for the override-routes + unknown-close assertions deferred from Task 4.)

**Two IMPL notes for these integration tests (dispatch fidelity):**
  1. **Reach `TerminalReady()` via an `OnData` pass — do NOT call `HandleTerminal` straight after `OnNewConnection`.** For a mixed `[sni_cluster, tcp_proxy]` chain, `onNewConnection()` resets `resumeIdx` to 0 at completion (`chain.go:240`), so `TerminalReady()` (`resumeIdx >= len(filters)`) is FALSE until a subsequent `OnData` pass advances `resumeIdx` past the read filter — exactly the production flow (`serveNetworkChain`'s in-loop `TerminalReady()` check at `manager.go:1066`, after the first `OnData`). So the test must: `OnNewConnection()` (sets the override) → `OnData([]byte("…"), false)` (sni_cluster passes through, `resumeIdx` advances) → assert `TerminalReady()` true → `HandleTerminal(ctx)` (replays the buffered bytes via `prefixConn` to `tcp_proxy`, which resolves the override and pumps). Driving it this way mirrors SPEC §4.3 faithfully; calling `HandleTerminal` directly after `OnNewConnection` would work mechanically (the override write already landed) but would not exercise the real mixed-chain handoff.
  2. **Build a fresh 2-cluster manager** — `tcpproxy`'s `mkClusterMgr` test helper is unexported, single-cluster, and in the `tcpproxy_test` package (not reusable cross-package). Build a 2-cluster manager + two backends in the `builtins` test directly via `cluster.NewManager(bs, stats.NewRegistry())` (the API used by `tcpproxy/filter_test.go:58`), with clusters `foo` (sentinel `FOO`) + `bar` (the `tcp_proxy` configured default), each pointing at a local echo backend.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/builtins/ -v`
Expected: FAIL — `snicluster` import undefined / `RegisterBuiltins` does not register it / the integration tests fail to route.

- [ ] **Step 3: Register the 6th built-in**

In `builtins.go`, add the import (`"github.com/esalaine/envoy-go/internal/filter/network/snicluster"`) and the registration line in `RegisterBuiltins` (after the rbac line at `:57`):
```go
	// sni_cluster: the 6th built-in (config-less, no boot singletons — like
	// echo/direct_response; ADR-0220). No Deps needed.
	reg.Register(snicluster.TypeURL, snicluster.New)
```
Update the package doc (`:1`) "five built-in network filters (echo, direct_response, tcp_proxy, HCM, rbac_network)" → "six … , sni_cluster)" and the `RegisterBuiltins` doc comment (`:37`). No `cmd/envoy-go/main.go` edit (D27-S2: main.go delegates wholly to `RegisterBuiltins`).

- [ ] **Step 4: Confirm main.go needs no parallel edit**

Run: `grep -n "reg.Register\|RegisterBuiltins" cmd/envoy-go/main.go`
Expected: only the `builtins.RegisterBuiltins(netReg, …)` call (no explicit per-filter list) → no main.go change required (D27-S2 confirmed).

- [ ] **Step 5: Run the tests + a boot smoke**

Run: `go test ./internal/filter/network/builtins/ -race -short -v` → all PASS (registration + the two e2e integration tests).
Run: `go build ./...` → clean (sni_cluster wired into the boot path).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/builtins/ ; golangci-lint run ./internal/filter/network/builtins/...
git add internal/filter/network/builtins/builtins.go internal/filter/network/builtins/builtins_test.go
git commit -m "phase 27 Task 6: register sni_cluster as the 6th built-in + end-to-end override routing/unknown-close integration tests (no main.go edit — D27-S2)"
```

---

## Task 7: The `0045-sni-cluster` cross-side TLS fixture (3 arms)

**Files:**
- Create: `test/fixtures/0045-sni-cluster/driver/driver.go`
- Create: `test/fixtures/0045-sni-cluster/README.md`
- Create (if a fresh keypair set is needed): `test/fixtures/0045-sni-cluster/pki/…` (reuse the `0002-tls-tcp/pki` or `0043-network-rbac` shape; the differential runner builds the bootstrap programmatically — mirror `0043-network-rbac/driver/driver.go`, the closest cross-side template).

- [ ] **Step 1: Re-pin the dir number**

Run: `ls -d test/fixtures/[0-9]* | tail -1` → expect `0044-…` → next-free dir = **`0045-sni-cluster`** (D27-S1). If the IMPL-session tip has advanced the tail, re-number accordingly.

- [ ] **Step 2: Author the driver (3 arms, cross-side)**

Mirror `0043-network-rbac/driver/driver.go`'s registration + multi-listener structure. The bootstrap (both `envoy.yaml`-equivalent for ref + `envoy-go.yaml`-equivalent for subj, constructed programmatically per the 0043 pattern) configures listener chain(s) `[sni_cluster, tcp_proxy]` over TLS, with clusters named **verbatim** after the SNI values + distinct per-backend sentinels:
  - **Route arm** — TLS client sends SNI `foo.example.com` → `sni_cluster` sets override `foo.example.com` → `tcp_proxy` routes to cluster `foo.example.com` (sentinel `FOO`). Proves SNI→override→route.
  - **Fallback arm** — TLS client sends NO SNI (empty `ServerName`) → `sni_cluster` sets no override → `tcp_proxy` uses its configured fallback cluster (sentinel `FALLBACK`). Proves empty-SNI no-op + configured fallback (F-RESOLVE). (D27-S1 chosen shape: same chain `[sni_cluster, tcp_proxy]`, connection with no SNI — proves `sni_cluster`'s empty-SNI branch, not merely a chain without it.)
  - **Unknown-cluster-close arm** — TLS client sends SNI `unknown.example.com` (no such cluster) → override miss → downstream close, **zero application bytes** (F-NOROUTE). Both ref + subj close with zero bytes → byte-exact body comparison.

The driver issues one TLS round-trip per arm and emits a deterministic per-arm verdict line; the "side" label (ref vs subj) is EXCLUDED so both sides produce identical bytes when behavior is equivalent (the `0043` pattern). Per `reference_differential_fixture_dispatch_constraint`: all three arms are cross-side (NO boot-reject arm — config-less), so a SINGLE cross-side dir holds all three (multiple listeners/SNIs within the one dir). All three arms are wire-byte-exact via the standard body comparison → **no subject-only `StatsAsserter` arm is required** for the core proof (per `reference_differential_asserter_dispatch`; if the IMPL adds a `StatsAsserter` arm it MUST be proven non-vacuous — but none is needed here).

- [ ] **Step 3: Author the README**

Document the 3-arm taxonomy, the cluster-named-after-SNI convention, the TLS/SNI requirement, the zero-byte unknown-close parity, and the cross-side-byte-exact rationale (mirror `0043-network-rbac/README.md`).

- [ ] **Step 4: Run the new fixture cross-side**

Run the differential runner scoped to `0045-sni-cluster` (the project's standard differential invocation — see how `0043` is run; reference Envoy v1.37.2 dockerized + envoy-go).
Expected: `0045-sni-cluster` PASS (3 arms byte-exact, ref vs subj).

- [ ] **Step 5: Deliberate-break proof (per the differential discipline)**

Temporarily flip one arm's expectation (e.g. swap the route-arm sentinel) and confirm the runner FAILS — proving the assertion is live — then revert. Record the break/revert in PROGRESS.md.

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l test/fixtures/0045-sni-cluster/ ; golangci-lint run ./test/fixtures/0045-sni-cluster/...
git add test/fixtures/0045-sni-cluster/
git commit -m "phase 27 Task 7: 0045-sni-cluster 3-arm cross-side TLS fixture (route / empty-SNI-fallback / unknown-cluster-close) byte-exact vs Envoy v1.37.2"
```

---

## Task 8: Back-compat differential re-verify + full-suite green

**Files:** none (verification gate).

- [ ] **Step 1: Re-verify the existing `tcp_proxy` fixtures stay byte-exact**

Run the differential runner scoped to the back-compat tcp_proxy dirs: `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp` (+ the 26.x network fixtures `0040`/`0041`/`0042`/`0043`).
Expected: ALL byte-exact green — the strongest proof the per-connection-resolution change is non-regressive (no `sni_cluster` in chain → no override → `defaultCluster` → byte-exact with master tip; §3.3 / §8.3).

- [ ] **Step 2: Run the FULL differential suite**

Run the full suite (46 → **47** dirs incl. `0045`).
Expected: 47/47 byte-exact green (allow for the KNOWN environmental ephemeral-port-bind flake that PASSES on retry — not a regression; quote honestly in PROGRESS.md if it occurs).

- [ ] **Step 3: Record + commit the gate**

Quote the suite output into PROGRESS.md. (No code change; the commit is the PROGRESS.md update.)
```bash
git add docs/envoy-go/phases/27-network-filter-sni-cluster/PROGRESS.md
git commit -m "phase 27 Task 8: back-compat tcp_proxy fixtures byte-exact + full differential suite 47/47 green"
```

---

## Task 9: Completion bundle (BEHAVIOR_CONTRACT + ADR bodies + STATE/ROADMAP + six-gate)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0219 + ADR-0220 §Decision/§Consequences bodies — in-place per ADR-0044)
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT.md 27 bundle (§9 / §14)**

Add a NEW `### envoy.filters.network.sni_cluster` subsection — proto `…sni_cluster.v3.SniCluster` (EMPTY); `OnNewConnection` reads SNI → publishes verbatim as the per-connection upstream-cluster-override → `Continue`; `OnData` pass-through; empty/absent SNI → no override; the 6th built-in; no stats; no per-route surface. Add a `tcp_proxy` per-connection-resolution amendment to the existing network-filter section — `tcp_proxy` resolves `override (PerConnectionCluster-equivalent) → cm.Get(override)` (unknown → downstream close, zero bytes) `else → configured cluster`; back-compat byte-exact when no override. Add the coverage-boundary record — `tcp.<stat_prefix>.downstream_cx_no_route` (and the wider `downstream_cx_*` family) is a known-unmirrored upstream counter (§7.2); the narrow typed override is the envoy-go stand-in for Envoy's `envoy.tcp_proxy.cluster` filter-state key (no general filter-state primitive — Q2).

- [ ] **Step 2: ADR-0219 + ADR-0220 §Decision/§Consequences bodies (ADR-0044 in-place)**

Fill the §Decision + §Consequences of **ADR-0219** (the connection-scoped upstream-cluster-override seam — the narrow typed `chainRuntime.upstreamClusterOverride`; the `SetUpstreamCluster` on-interface writer + the rejected `rcd`-type-assert alternative; the `UpstreamClusterOverride` ctx-threaded reader + the rejected signature-change/terminal-accessor alternatives; the `tcp_proxy` per-connection resolution; the `downstream_cx_no_route`-unmirrored decision; the back-compat-via-existing-fixtures discipline; the no-general-primitive / API-revision-allowance clause) and **ADR-0220** (the `sni_cluster` filter — config-less parse; `OnNewConnection` SNI-verbatim→override + `Continue`; no-op `OnData`/`OnDestroy`; 6th built-in; empty-SNI→fallback + unknown→close parity; the second production mixed read→terminal chain). **NO new ADR number consumed** (DECISIONS.md tail stays ADR-0220; next-free stays ADR-0221).

- [ ] **Step 3: STATE.md + ROADMAP.md phase-done advance**

ROADMAP row 27 `in-progress → done` (append the IMPL-DONE note: fixtures 46→47, stat surface 136 +0, fuzzers 36 +0, ADR-0219/0220 bodies landed). STATE.md: `active-phase` → `phase 27 IMPL done`; `lifecycle-state` → phase-done; `next-skill` → the next phase's brainstorm (the §9 family stays OPEN — 5 candidates remain: redis/mongo/kafka_broker/thrift/zookeeper); `last-commit` filled at squash; counts updated (fixtures **47**, stats **136**, fuzzers **36**, DECISIONS tail **ADR-0220**, next-free **ADR-0221**).

- [ ] **Step 4: The six-gate (§15.2) — run LIVE, quote into PROGRESS.md**

Run + quote each, honestly:
```bash
go build ./...                       # clean
go vet ./...                         # clean
golangci-lint run                    # clean
go test ./... -race -short           # green
# full differential suite 47/47 byte-exact (incl. back-compat tcp_proxy + 0045)
# h2spec 53/53 + proxy-wasm conformance 10/10 re-run LIVE (asserted-unaffected — phase 27
#   touches no HTTP/h2/proxy-wasm path — but re-confirmed since the harness is available)
```
Confirm: stat surface **136** (+0), fixtures **47**, fuzzers **36**, DECISIONS tail **ADR-0220**, next-free **ADR-0221**.

- [ ] **Step 5: Commit the bundle**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md docs/envoy-go/phases/27-network-filter-sni-cluster/PROGRESS.md
git commit -m "phase 27 Task 9: completion bundle — BEHAVIOR_CONTRACT sni_cluster subsection + tcp_proxy amendment; ADR-0219/0220 bodies (in-place); STATE/ROADMAP row 27 in-progress→done; six gates GREEN LIVE"
```

---

## Test surface summary (SPEC §15.1)

- **Unit:** the override field + `SetUpstreamCluster` round-trip (Task 2); the `UpstreamClusterOverride` ctx accessor present/absent + `handleTerminal` wraps-iff-set (Task 3); `tcp_proxy` Handle no-override→default back-compat (Task 4); `sni_cluster` parse empty/any/malformed + `OnNewConnection` SNI→override live-assert + empty-SNI no-op + `OnData` pass-through (Task 5); registration smoke + end-to-end override-routes + unknown-close integration (Task 6).
- **Differential:** `0045-sni-cluster` 3 arms byte-exact (Task 7); the existing `tcp_proxy` fixtures byte-exact back-compat + full suite 46→47 (Task 8).
- **Byte-stable:** the `sni_cluster:` invalid-config wording + the existing `tcpproxy: cluster %q not found` boot-reject stays byte-stable (no NEW reject const).

## Acceptance checklist (SPEC §15.3)

- [ ] The override seam (field + on-interface `SetUpstreamCluster` + ctx accessor + `handleTerminal` threading) lands; `tcp_proxy` resolves per-connection (override/fallback/unknown-close); no-override byte-exact.
- [ ] `sni_cluster` lands config-less, `OnNewConnection` SNI→override→`Continue`, 6th built-in; `OnData` pass-through; empty-SNI no-op.
- [ ] `0045-sni-cluster` 3-arm cross-side fixture green; back-compat `tcp_proxy` fixtures green; suite 46 → 47.
- [ ] Stat surface 136 (+0); fuzzers 36 (+0); ADRs +2 bodies (0219/0220 in place; tail ADR-0220; next-free ADR-0221).
- [ ] BEHAVIOR_CONTRACT 27 bundle; STATE/ROADMAP row 27 `in-progress → done`; six gates GREEN LIVE quoted into PROGRESS.md.
