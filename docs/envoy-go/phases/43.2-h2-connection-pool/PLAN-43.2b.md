# Phase 43.2b Implementation Plan — graceful GOAWAY-driven connection rotation: the H2-multiplex-pool drain lifecycle + the behavior-driven `http2.*` reset stats

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The controller squashes + pushes at stage-close; subagents commit LOCALLY only (`feedback_subagents_no_push`).

**Goal:** Complete the HTTP/2 upstream multiplex pool's lifecycle — when a pooled upstream conn receives a peer GOAWAY, mark it draining (take NO new streams), drain its in-flight streams to completion, close it (promptly when idle, on the last stream-release when in-flight), and open a replacement lazily on the next demand — plus the first codec→cluster stat wiring for the two behavior-driven reset counters `http2.rx_reset` + `http2.tx_reset`.

**Architecture:** The codec already observes a peer GOAWAY (`goawayCh` closed in `dispatchFrame`'s `GoAwayFrame` case) but does NOT cancel the conn ctx, so a GOAWAY'd-but-alive conn is invisible to the 43.2a admission scan (`Closed()` stays false) and keeps taking streams — the gap 43.2b closes. We expose the signal read-only (`GoneAway()`/`GoneAwayCh()`/`Done()`), add the admission-skip + eviction generalization, a per-conn drain-watcher goroutine (so an IDLE draining conn closes promptly), and inject two reset-counter hooks into the codec at dial. NO new pool state (`draining` is DERIVED from `cc.GoneAway()`); NO new proto field; NO new fuzzer.

**Tech Stack:** Go; the in-tree `internal/filter/hcm/h2` codec; the 43.2a `h2Pool` (`internal/cluster/h2pool.go`); the 43.1 permit substrate; the Docker-bridge differential harness (`reference_docker_probe_bridge_network`). ZERO new Go packages; ZERO new go.mod modules.

**Source SPEC:** `docs/envoy-go/phases/43.2-h2-connection-pool/SPEC-43.2b.md` (commit `f823aa7e`). Predecessor BRAINSTORM: `BRAINSTORM-43.2b.md` (`b8ec7b4a`). The §11 D-H2B-* empirical pins were EXECUTED live (2026-06-24, contrib-v1.37.2) — do NOT re-litigate them.

---

## Orientation — read before Task 1 (the zero-context brief)

envoy-go is a from-scratch Go reimplementation of Envoy, validated **cross-side** against reference `envoyproxy/envoy:contrib-v1.37.2` over a Docker bridge. Every behavior change is proven by a differential fixture driving BOTH proxies and comparing observable stats.

**What 43.2a delivered (the substrate this leg extends — ADR-0253):** a per-endpoint H2 multiplex `ClientConn` pool (`internal/cluster/h2pool.go`). `AcquireH2Stream(ctx)` rides a four-tier ladder (stream-HIT → connecting-HIT/attach → MISS-dial → PEND) and returns a call-once `release()`. The per-conn stream budget is the cluster's OWN `http2_protocol_options.max_concurrent_streams` (the local cap `C`). 43.2a shipped MINIMAL liveness only: `Closed()`-skip in the admission scan + evict-on-RoundTrip-error. It DEFERRED the graceful drain lifecycle + reset stats to THIS leg.

**The headline empirical finding (AMEND-H2B-1..4, the live probe REVERSED the brainstorm DOWN):** a peer GOAWAY is observed by the reference ENTIRELY via `upstream_cx_*` lifecycle counters (`cx_close_notify` then `cx_destroy_local`; the conn is held until the last in-flight stream completes — graceful), with **NO `http2.*` counter moving during rotation** and **NO `goaway_received` counter existing**. Of the brainstorm's four candidate reset/goaway stats, only **`http2.rx_reset` + `http2.tx_reset`** are LIVE and cross-side-assertable. So 43.2b registers exactly those two (**1185 → 1187, +2**); `goaway_sent` (reference-0-on-drain ⇒ would diverge) + `stream_refused_errors` (reference-dormant) are DEFERRED.

**The mental model for the drain lifecycle:**

- A pooled conn becomes *draining* the instant the codec observes a peer GOAWAY (`goawayCh` closed). `draining` is NOT a stored field — it is DERIVED via `cc.GoneAway()`, exactly as `Closed()` derives from `cc.ctx.Err()`.
- A draining conn takes **NO new streams** (admission-skip) but **completes its in-flight streams** (the GOAWAY does NOT cancel the conn ctx; streams below `LastStreamID` finish normally).
- Close timing has two paths: **IN-FLIGHT** → the last `release()` evicts+Closes it (the generalized eager-close check); **IDLE** → the per-conn drain-watcher closes it promptly (an idle conn has no `release()` to ever fire and its ctx is never cancelled, so without the watcher it would leak).
- Replacement is **LAZY**: a skipped draining conn → the next acquire MISSes → dials a fresh conn (`upstream_cx_http2_total` ++). No background dial.

### Key source seams (verified at PLAN time against the tree at `f823aa7e`; re-confirm line numbers before editing — the file evolves)

| File | Seam | This plan |
|------|------|-----------|
| `internal/filter/hcm/h2/client.go` | `goawayCh` field (`:78`); closed in `dispatchFrame`'s `GoAwayFrame` case (`:444-449`) WITHOUT `cc.cancel()`; `Closed()` (`:614`); `Close()` emits GOAWAY(NO_ERROR) (`:626`); `NewClientConn(ctx, upstream net.Conn)` (`:153`, spawns `go cc.readLoop()` at `:186`); the `RSTStreamFrame` `cs.finish(...)` site (`:394`); the `RoundTrip` `ctx.Done` → `WriteRSTStream(id, ErrCodeCancel)` site (`:537`) | ADD `GoneAway()`/`GoneAwayCh()`/`Done()` accessors (Task 2); ADD `onRxReset`/`onTxReset func()` hook fields + the `WithResetHooks` option + nil-guarded calls at `:394`/`:537` (Task 5) |
| `internal/cluster/h2pool.go` | `findStreamHitLocked` (`:444`, cond `!pc.cc.Closed() && pc.inFlight < C`); `h2PromoteLocked`'s stream-slot branch (`:131`, same cond); `findConnectingHitLocked` (`:456`, `cn.cc == nil` here — STAYS unguarded); `makeRelease` eager-close (`:623`, `pc.cc.Closed() && pc.inFlight == 0`); `evictH2ConnLocked` (`:638`); `EvictH2ConnOnError` (`:665`); `findPooledLocked` (`:679`); `dialAndPool` pool-insert (`:601`, `c.h2Pool[addr] = append(...)`) | ADD `&& !pc.cc.GoneAway()` at `:446`+`:131`; generalize `:623` to `(Closed()||GoneAway()) && inFlight==0`; spawn the watcher after `:601` (Task 3+4); ADD `h2RxResetInc`/`h2TxResetInc` nil-guarded helpers (Task 5) |
| `internal/cluster/dial_h2.go` | `dialPooledH2To` (`:73`) → `h2ConnFromDialed` (`:96`, shared with the dead `DialH2` `:48`) → `h2.NewClientConn(ctx, wrapped)` (`:133`) | thread `h2.ClientConnOption` through `h2ConnFromDialed`; `dialPooledH2To` passes `WithResetHooks(...)`, `DialH2` passes nothing (Task 6) |
| `internal/cluster/cluster.go` | the H2 stat-handle fields (`http2StreamsActive` `:145`, `upstreamCxHTTP2Total` `:160`) | ADD `http2RxReset`/`http2TxReset *stats.Counter` next to them (Task 6) |
| `internal/cluster/manager.go` | `registerClusterMetrics` (`:112`); the `if c.useH2 { ... }` H2-stats block (`:191-197`); `prefix := "cluster." + c.name + "."` (`:113`) | register the two reset counters in the `useH2` block (Task 6) |
| `internal/cluster/manager_test.go` | `TestRegisterClusterMetrics_H2Stats` (`:1633-1692`, presence/absence via `hasMetric`, NOT an exact count) | extend to assert the two new names present (H2) / absent (non-H2) (Task 6) |
| `test/differential/fixture/fixture.go` | `BackendKind` enum (tail `H2HoldResponder = 37`) | ADD `H2GoawayResponder = 38` (Task 7) |
| `test/differential/runner_test.go` | the `H2HoldResponder` dispatch case (`:997`) + `acceptH2Hold` (`:3054`, uses `h2c.NewHandler`+`http.Server` — CANNOT emit GOAWAY/RST frames) | ADD a `H2GoawayResponder` case + a raw-framer `acceptH2Goaway` (Task 7) |
| `test/fixtures/0079-h2-multiplex-pool/driver/driver.go` | the H2 (TLS+ALPN-h2) DOWNSTREAM listener template `h2ListenerFilterChain()` (`:282`); the config-as-Go-string `ReferenceBootstrap`/`SubjectConfig`; `pollStats`/`scrapeStats`/`release` barrier helpers | the `0080` driver template (Task 8) |
| `test/fixtures/0004-h2-routing/pki/` | `listener.pem` / `listener.key.pem` — the downstream TLS+ALPN-h2 PKI material 0079 already reuses | `0080` reuses the same shape (Task 8) |

### Discipline (honor on EVERY task)

- `feedback_pertask_gofmt_lint` — each task runs `gofmt -l` (must print nothing) + `golangci-lint run` on touched packages, NOT just `go vet`.
- `feedback_subagents_no_push` — commit LOCALLY only; the controller squashes + pushes at stage-close.
- `reference_differential_break_protocol_count1` — go-test caches results; every deliberate-break check AND every `-race` run uses `-count=1`, or a stale PASS masks a dead assertion.
- `reference_differential_run_selector` — a single fixture is `-run 'TestDifferential/0080'` (subtests are `TestDifferential/<fixture>`; a bare `-run '0080'` matches ZERO subtests → vacuous green).
- `reference_concurrency_differential_release_barrier` + `reference_concurrent_attempt_downstream_class_assertion` — the `0080` drive uses release-barrier + poll-the-gauge + sequential-per-side, NEVER a `time.Sleep`; assert the DOWNSTREAM response class on the rx_reset prong, not the over-counting upstream class.
- `reference_h2_pool_downstream_codec_gate` — the H2 pool engages ONLY for an H2 (TLS+ALPN-h2) DOWNSTREAM listener. `0080` MUST use the 0004/0079 H2-downstream shape; an H1-downstream variant silently never exercises the pool.
- `reference_h2_pool_connect_coalescing` — the connecting-tier coalescing (`findConnectingHitLocked`) MUST be PRESERVED; that site stays GoneAway-guard-free (a `connectingH2Conn` has `cc == nil` there — a guard would nil-deref AND is vacuous).
- `reference_h2_goaway_rotation_stats` — the rotation is observed via `upstream_cx_*` lifecycle counters, NOT `http2.*`; `goaway_received` does not exist; only `rx_reset` + `tx_reset` are live. Do NOT assert `cx_close_notify`/`cx_destroy_local` cross-side (envoy-go does not emit them).
- `reference_docker_probe_bridge_network` — any live reference probe uses a shared bridge + a backend hostname reachable from BOTH containers; verify decode actually ran.
- `reference_differential_fullsuite_startup_flake` — a `subject ready: EOF` is a transient startup race, NOT a regression; isolate-re-run to tell them apart.
- The stat surface is **1185** at start and **1187** at end (H2 clusters only — the 2 new names register useH2-gated so non-H2 fixtures stay byte-identical at 1183). The registration test is presence/absence-based (no magic-number assertion to update); STATE.md/PROGRESS carry the figure.

---

## D-question resolutions (the §12 PLAN pins — settled here)

- **D-H2B-SPLIT-FINAL** → 43.2b ships as ONE flat leg (~10 tasks, ≈150–220 prod LoC, one differential) — under the ADR-0045 soft gate (the 43.1 D-S431-7 / 43.2a precedent). No task-spine sub-split. **Row 43 flips `done` at this leg's IMPL** ⇒ the Upstream-robustness family (phases 39–43) CLOSES.
- **D-H2B-WIRING** → the codec→cluster reset handles are injected via a **variadic functional-option** `h2.WithResetHooks(onRx, onTx func())` on `NewClientConn`, applied BEFORE `go cc.readLoop()` (race-free; a post-construction setter would race the already-running readLoop that reads the hook in `dispatchFrame`). The codec stores two `func()` hook fields and stays stats-agnostic (no `internal/stats` import). The option is threaded through `h2ConnFromDialed`; `dialPooledH2To` passes `WithResetHooks(c.h2RxResetInc, c.h2TxResetInc)`, the dead `DialH2` passes nothing (→ nil hooks). The 6 other existing `NewClientConn` callers (4 in `client_test.go` + 2 in the cluster pool tests — every call site except the production dial path being rewired) compile unchanged (variadic).
- **D-H2B-BACKEND** → a **new BackendKind 38** (`H2GoawayResponder`), a raw-`golang.org/x/net/http2`-framer h2c server (modeled on the §11 live-probe backend). The kind-37 `H2HoldResponder` uses `h2c.NewHandler`+`http.Server`, which abstracts away the framer and CANNOT emit on-demand GOAWAY / targeted RST_STREAM frames — extending it would mean a full rewrite to a raw framer anyway. BackendKind tail **37 → 38**.
- **D-H2B-WATCHER-LIFECYCLE** → act-then-exit single-shot watcher: `select { <-cc.GoneAwayCh() → re-lock, re-check `findPooledLocked != nil && inFlight == 0`, then `evictH2ConnLocked` + `h2PromoteLocked` (idle path; in-flight path is a no-op here and the last `release()` evicts) ; <-cc.Done() → exit (evicted by another path) }`. The `findPooledLocked != nil` re-check is the double-evict guard vs `EvictH2ConnOnError`/`makeRelease`. No pool-owned done channel; no goroutine leak. The `Done()` accessor (Task 2) is REQUIRED — the SPEC §3.3 pseudocode reads `cc.ctx.Done()` but `cc.ctx` is unexported.
- **D-H2B-CXSTATS** → the `0080` cross-side EXACT asserts are `upstream_cx_http2_total`, `upstream_cx_active`, `http2.streams_active`, `http2.rx_reset`, `http2.tx_reset` — all emitted by BOTH sides. Do NOT assert `cx_close_notify`/`cx_destroy_local` (envoy-go does not emit them; they GUIDE the reference's observed rotation but are not cross-side targets). Confirm envoy-go's emitted `upstream_cx_*` set against a `/stats` scrape during Task 8.
- **D-H2B-FUZZER-RECONCILE** → reconcile the documented-42-vs-actual-43 `^func Fuzz` drift to **43** at this family-closing leg (Task 10) per `reference_fuzzer_count_docs_drift`; update STATE/PROGRESS + the memory.
- **D-H2B-BYTESTAB** → `0004` + `0079` stay GREEN under the drain extensions (their EXACT prongs never trigger GOAWAY; assertions are GOAWAY-agnostic). Proven by the full-suite gate (Task 10).

---

## File structure (decomposition locked here)

- **`internal/filter/hcm/h2/client.go` (MODIFY)** — `GoneAway()`/`GoneAwayCh()`/`Done()` read-only accessors; the `onRxReset`/`onTxReset func()` hook fields; the `ClientConnOption` type + `WithResetHooks`; nil-guarded hook calls at the two RST sites.
- **`internal/filter/hcm/h2/client_test.go` (MODIFY)** — accessor unit tests (`-race`); reset-hook fire tests.
- **`internal/cluster/h2pool.go` (MODIFY)** — the `&& !GoneAway()` admission skips; the `(Closed()||GoneAway())` eviction generalization; the per-conn drain-watcher (`watchDrain`); `h2RxResetInc`/`h2TxResetInc` nil-guarded helpers.
- **`internal/cluster/h2pool_test.go` (MODIFY)** — admission-skip + drain-close + watcher tests (driving a real GOAWAY over the test pipe); the double-evict `-race` test.
- **`internal/cluster/dial_h2.go` (MODIFY)** — thread `...h2.ClientConnOption` through `h2ConnFromDialed`; `dialPooledH2To` wires the hooks.
- **`internal/cluster/cluster.go` (MODIFY)** — add `http2RxReset`/`http2TxReset *stats.Counter` fields.
- **`internal/cluster/manager.go` (MODIFY)** — register the two reset counters useH2-gated.
- **`internal/cluster/manager_test.go` (MODIFY)** — extend the H2-stats registration test.
- **`test/differential/fixture/fixture.go` + `runner_test.go` (MODIFY)** — `H2GoawayResponder = 38` + the raw-framer dispatch/accept.
- **`test/fixtures/0080-h2-goaway-rotation/` (NEW)** — `driver/driver.go` (config-as-Go-string; the 0079/0004 H2-downstream shape), `driver/driver_test.go`, `expectations.yaml`, `README.md`.
- **Docs (MODIFY at the completion task):** `DECISIONS.md` (ADR-0254 body), `BEHAVIOR_CONTRACT.md`, `STATE.md`, `ROADMAP.md` (row 43 → done), `phases/43.2-h2-connection-pool/PROGRESS.md` (NEW for 43.2b).

---

## Task 1: Phase scaffolding — PROGRESS.md + baselines + the final ADR-0045 split re-check (D-H2B-SPLIT-FINAL)

**Files:**
- Create: `docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS-43.2b.md`

- [ ] **Step 1: Capture the green baseline.** Run from the worktree root; record outputs in PROGRESS:

```bash
go build ./...
go vet ./...
gofmt -l internal/ test/        # expect: no output
golangci-lint run ./internal/cluster/... ./internal/filter/... 2>&1 | tail -5
go test ./internal/cluster/... ./internal/filter/hcm/h2/... -count=1 2>&1 | tail -20
```

Expected: build/vet clean; `gofmt -l` prints nothing; lint clean; unit tests PASS.

- [ ] **Step 2: Record the count baselines** in PROGRESS: stat surface **1185** (H2 cluster; non-H2 **1183**); fixtures **81** (tail `0079-h2-multiplex-pool`); fuzzers documented **42** (actual `^func Fuzz` = **43** — `reference_fuzzer_count_docs_drift`); BackendKind tail **37** (`H2HoldResponder`); DECISIONS tail **ADR-0253** (next-free **ADR-0254**). Verify:

```bash
grep -c '^func Fuzz' $(git grep -l '^func Fuzz' -- '*_test.go') | awk -F: '{s+=$2} END{print "fuzzers:", s}'
grep -n 'BackendKind = 3[0-9]' test/differential/fixture/fixture.go | tail -1
ls -d test/fixtures/00* | wc -l
```

- [ ] **Step 3: D-H2B-SPLIT-FINAL — the final ADR-0045 re-check.** Confirm 43.2b ships as ONE flat leg (~10 tasks, ≤~220 prod LoC, one differential `0080`). Record in PROGRESS: "43.2b ships flat; no remaining deferral except the recorded hardening items (`goaway_sent`, `stream_refused_errors`, `min(local,peer)`, `nextStreamID` 2^31 retirement)."

- [ ] **Step 4: Confirm ROADMAP posture.** Record: row 43 (`connection-pooling`) flips `done` at THIS leg's IMPL (43.1 + 43.2a + 43.2b ALL landed) ⇒ the Upstream-robustness family (39–43) CLOSES (`reference_roadmap_split_phase_row_done`). Do NOT edit ROADMAP yet (Task 10).

- [ ] **Step 5: Commit.**

```bash
git add docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS-43.2b.md
git commit -m "phase 43.2b Task 1: PROGRESS scaffolding + green baselines + ADR-0045 split re-check (flat)"
```

---

## Task 2: Codec-visible GOAWAY signal — `GoneAway()` / `GoneAwayCh()` / `Done()` accessors

**Files:**
- Modify: `internal/filter/hcm/h2/client.go`
- Test: `internal/filter/hcm/h2/client_test.go`

**Context:** The codec already closes `goawayCh` on a peer GOAWAY (`dispatchFrame` `:444-449`) WITHOUT cancelling the conn ctx, so `Closed()` stays false on a GOAWAY'd-but-alive conn. Expose the signal read-only for the pool. **Three accessors** (the SPEC §3.1 names two; the watcher needs the third): `GoneAway()` (non-blocking closed-state read of `goawayCh`), `GoneAwayCh()` (the channel, for the watcher to `select` on), and `Done()` (`= cc.ctx.Done()`, the watcher's "evicted by another path" exit signal — REQUIRED because `cc.ctx` is unexported and the SPEC §3.3 pseudocode's `cc.ctx.Done()` is not reachable from package `cluster`). NO new codec state — all three are read-only over existing fields.

- [ ] **Step 1: Write the failing test** in `client_test.go` (reuse the in-memory-pipe `NewClientConn` scaffolding at `client_test.go:84` — a `net.Pipe` with a test-controlled server side that completes the preface/SETTINGS handshake). Cases:
  1. A fresh conn reports `GoneAway() == false` and `GoneAwayCh()` is NOT closed (a `select`/`default` reads not-ready).
  2. After the test's server side writes a `GOAWAY` frame (via a `golang.org/x/net/http2.Framer.WriteGoAway`), poll until `GoneAway() == true` AND `GoneAwayCh()` is closed AND **`Closed() == false`** (the load-bearing invariant: GOAWAY does not cancel the ctx).
  3. `Done()` is NOT closed on a fresh conn; after `cc.Close()`, `Done()` IS closed (and `Closed() == true`).

- [ ] **Step 2: Run — expect FAIL** (accessors undefined):

```bash
go test ./internal/filter/hcm/h2/ -run 'TestClientConnGoneAway|TestClientConnDone' -count=1 2>&1 | tail
```

- [ ] **Step 3: Implement** near `Closed()` in `client.go`:

```go
// GoneAway reports whether this connection has observed a peer GOAWAY (its
// goawayCh has been closed by dispatchFrame's GoAwayFrame case). It is a
// non-blocking read of the closed state — distinct from Closed(): a GOAWAY'd
// conn keeps serving its in-flight streams (the ctx is NOT cancelled) until
// they drain, so Closed() stays false while GoneAway() is true. The h2 pool's
// admission scan skips a GoneAway conn (takes no new streams) and evicts+Closes
// it once its last in-flight stream drains. (phase 43.2b, ADR-0254)
func (cc *ClientConn) GoneAway() bool {
	select {
	case <-cc.goawayCh:
		return true
	default:
		return false
	}
}

// GoneAwayCh returns the channel closed when a peer GOAWAY is observed, for the
// pool's per-conn drain-watcher to select on. (phase 43.2b, ADR-0254)
func (cc *ClientConn) GoneAwayCh() <-chan struct{} { return cc.goawayCh }

// Done returns the conn-lifetime ctx's Done channel, closed when the conn is
// torn down (Close or a transport error cancels cc.ctx). The drain-watcher
// selects on it as the "evicted by another path → exit" signal (so the watcher
// never leaks when a conn is evicted before any GOAWAY arrives). (phase 43.2b)
func (cc *ClientConn) Done() <-chan struct{} { return cc.ctx.Done() }
```

- [ ] **Step 4: Run — expect PASS** + `-race` + lint:

```bash
go test ./internal/filter/hcm/h2/ -race -count=1 2>&1 | tail
gofmt -l internal/filter/hcm/h2/ && golangci-lint run ./internal/filter/hcm/h2/...
```

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/hcm/h2/client.go internal/filter/hcm/h2/client_test.go
git commit -m "phase 43.2b Task 2: GoneAway()/GoneAwayCh()/Done() read-only codec accessors over goawayCh + conn ctx (GOAWAY != Closed)"
```

---

## Task 3: Admission-skip (draining) + eviction generalization in `h2pool.go`

**Files:**
- Modify: `internal/cluster/h2pool.go` (`findStreamHitLocked` `:446`, `h2PromoteLocked` stream-slot branch `:131`, `makeRelease` `:623`)
- Test: `internal/cluster/h2pool_test.go`

**Context (SPEC §3.2):** a draining conn takes NO new streams (admission-skip on both ESTABLISHED-conn sites) but closes on the last in-flight stream-release (eviction generalization). `draining` is DERIVED from `cc.GoneAway()` — NO new `pooledH2Conn` field. The connecting tier (`findConnectingHitLocked`) STAYS unguarded: a `connectingH2Conn` there has `cc == nil`, so a guard would nil-deref and is vacuous (`reference_h2_pool_connect_coalescing` — the coalescing MUST be preserved).

- [ ] **Step 1: Write the failing tests** in `h2pool_test.go`. The pool tests build a REAL `*h2.ClientConn` over an in-process h2c backend (the `h2pool_test.go:84` pattern). To make a pooled conn report `GoneAway() == true`, drive a real GOAWAY from the test's backend side, then poll `cc.GoneAway()`. Add a small test helper that, given the backend's accepted raw conn (or a backend gate), writes a `GOAWAY(NO_ERROR)` frame; reuse the Task-7 backend if sequencing is easier, otherwise a minimal `golang.org/x/net/http2.Framer` over the server side. Cases:
  1. **Admission-skip:** a single pooled conn with a free stream slot; drive its GOAWAY; poll `GoneAway()`; then assert `findStreamHitLocked(addr) == nil` (the draining conn is skipped) and a fresh `AcquireH2Stream` MISSes → dials a SECOND conn (`len(c.h2Pool[addr]) == 2`, `upstream_cx_http2_total == 2`).
  2. **Promote-skip:** a queued waiter + the only free-slot conn is draining → `h2PromoteLocked` does NOT hand that conn (falls through to the permit/dial path).
  3. **In-flight drain-close:** a draining conn with `inFlight == 1`; call its `release()`; assert the conn is evicted+Closed (the generalized eager-close fired) and its permit freed (`upstream_cx_active` decremented).

- [ ] **Step 2: Run — expect FAIL:**

```bash
go test ./internal/cluster/ -run 'TestH2.*Drain|TestH2.*GoneAway' -race -count=1 2>&1 | tail
```

- [ ] **Step 3: Implement.** Add `&& !pc.cc.GoneAway()` to BOTH established-conn admission conditions:

`findStreamHitLocked` (`:446`):
```go
if !pc.cc.Closed() && !pc.cc.GoneAway() && pc.inFlight < c.h2MaxConcurrentStreams {
```
`h2PromoteLocked`'s stream-slot branch (`:131`):
```go
if !pc.cc.Closed() && !pc.cc.GoneAway() && pc.inFlight < c.h2MaxConcurrentStreams {
```
Generalize `makeRelease`'s eager-close (`:623`):
```go
if (pc.cc.Closed() || pc.cc.GoneAway()) && pc.inFlight == 0 {
```
Do NOT touch `findConnectingHitLocked` (`:456`).

- [ ] **Step 4: Run — expect PASS under `-race`** + lint:

```bash
go test ./internal/cluster/ -race -count=1 2>&1 | tail -20
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
```

- [ ] **Step 5: Commit.**

```bash
git add internal/cluster/h2pool.go internal/cluster/h2pool_test.go
git commit -m "phase 43.2b Task 3: admission-skip draining conns (!GoneAway on both established sites) + (Closed||GoneAway)&&inFlight==0 eviction; connecting tier stays unguarded"
```

---

## Task 4: The per-conn drain-watcher goroutine (LOAD-BEARING, `-race -count=1`)

**Files:**
- Modify: `internal/cluster/h2pool.go` (add `watchDrain`; spawn it in `dialAndPool` after the pool-insert `:601`)
- Test: `internal/cluster/h2pool_test.go`

**Context (SPEC §3.3 + D-H2B-WATCHER-LIFECYCLE):** an IDLE conn that receives GOAWAY has no `release()` to ever fire and its ctx is never cancelled — without a watcher it would be skipped on admission forever but never closed (a slow conn leak). One single-shot goroutine per pooled conn closes it promptly. The watcher MUST re-check `findPooledLocked != nil` under `h2PoolMu` before evicting (the double-evict guard vs `EvictH2ConnOnError`/`makeRelease`). `cc.Done()` (Task 2) is the "evicted by another path → exit" signal (no pool-owned done channel; no goroutine leak).

- [ ] **Step 1: Write the failing tests** in `h2pool_test.go`. Cases:
  1. **Idle prompt-close:** pool a conn (idle, `inFlight == 0`); drive its GOAWAY; poll (no sleep) until the conn is evicted (`findPooledLocked(addr, cc) == nil`, `upstream_cx_active` decremented) — proving the watcher closed it (no `release()` drove it).
  2. **In-flight → watcher no-op:** pool a conn with `inFlight == 1`; drive its GOAWAY; assert the conn STAYS pooled (the watcher's `inFlight == 0` re-check is false); then `release()` → evicted (Task-3 path). Proves the watcher does not double-close an in-flight draining conn.
  3. **Evicted-by-another-path → no leak:** pool a conn; `EvictH2ConnOnError(cc, ep)` it (cancels ctx → `Done()` closes); assert the watcher exits without panicking / double-evicting. Run under `-race -count=1` with a tight loop (≥1000 iters of pool→race(evict, GOAWAY)→drain) to surface a double-evict or a lost wakeup.

- [ ] **Step 2: Run — expect FAIL:**

```bash
go test ./internal/cluster/ -run 'TestH2.*Watcher' -race -count=1 2>&1 | tail
```

- [ ] **Step 3: Implement `watchDrain`** in `h2pool.go`:

```go
// watchDrain is the per-conn drain-watcher (phase 43.2b, ADR-0254). One
// goroutine per pooled conn, spawned at pool insertion (dialAndPool). It closes
// an IDLE draining conn promptly: such a conn has no in-flight stream whose
// release() would evict it, and its ctx is never cancelled, so without this
// watcher it would be admission-skipped forever but never closed (a conn leak).
//
// Single-shot: it acts once then exits.
//   - GoneAwayCh fires (peer GOAWAY): re-lock h2PoolMu, re-check the conn is
//     still pooled AND idle (the findPooledLocked != nil guard dodges the
//     double-evict race with EvictH2ConnOnError / makeRelease); if so evict+Close
//     it and promote a waiter onto the freed permit. If inFlight > 0 the last
//     release()'s generalized eager-close (Task 3) does the eviction instead, so
//     this is a no-op and the watcher just exits.
//   - Done fires (conn evicted/closed by another path): exit, no action.
func (c *Cluster) watchDrain(addr string, cc *h2.ClientConn) {
	select {
	case <-cc.GoneAwayCh():
		c.h2PoolMu.Lock()
		if pc := c.findPooledLocked(addr, cc); pc != nil && pc.inFlight == 0 {
			c.evictH2ConnLocked(addr, pc)
			c.h2PromoteLocked(addr)
		}
		c.h2PoolMu.Unlock()
	case <-cc.Done():
		// evicted by another path → nothing to do.
	}
}
```

Spawn it in `dialAndPool` immediately after the successful pool-insert (`:601`, `c.h2Pool[addr] = append(c.h2Pool[addr], pc)`):
```go
c.h2Pool[addr] = append(c.h2Pool[addr], pc)
go c.watchDrain(addr, cc)
```
(This is the SINGLE site where a new conn enters `h2Pool` — `grep` confirms `c.h2Pool[addr] = append` appears only here. Every pooled conn gets exactly one watcher.)

- [ ] **Step 4: Run — expect PASS under `-race -count=1`** + lint:

```bash
go test ./internal/cluster/ -run 'TestH2' -race -count=1 2>&1 | tail -20
go test ./internal/cluster/ -count=1 2>&1 | tail
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
```

- [ ] **Step 5: Commit.**

```bash
git add internal/cluster/h2pool.go internal/cluster/h2pool_test.go
git commit -m "phase 43.2b Task 4: per-conn drain-watcher goroutine (idle prompt-close; findPooledLocked double-evict guard; Done()-exit no leak); -race -count=1 clean"
```

---

## Task 5: Codec reset-hooks — `WithResetHooks` option + nil-guarded increments at the RST sites

**Files:**
- Modify: `internal/filter/hcm/h2/client.go` (hook fields + `ClientConnOption` + `WithResetHooks` + the two increment sites)
- Test: `internal/filter/hcm/h2/client_test.go`

**Context (SPEC §3.4 + D-H2B-WIRING):** the reset EVENTS happen in the codec but the COUNTERS live on the cluster. The codec stays stats-agnostic: it stores two `func()` hooks fired nil-guarded at the existing RST sites. The option is applied BEFORE `go cc.readLoop()` so the readLoop never races the hook fields. `rx_reset` ++ at `dispatchFrame`'s `RSTStreamFrame` `cs.finish(...)` site (`:394`, the in-flight RST the differential exercises). `tx_reset` ++ at `RoundTrip`'s `ctx.Done` `WriteRSTStream(id, CANCEL)` site (`:537`).

- [ ] **Step 1: Write the failing tests** in `client_test.go` (in-memory-pipe scaffolding). Cases:
  1. **rx_reset hook:** construct a conn with `WithResetHooks` incrementing two test counters; start a RoundTrip held by the backend; the test's server side writes `RST_STREAM(INTERNAL_ERROR)` on that stream id; assert RoundTrip returns a stream error AND the rx counter == 1 AND the tx counter == 0 AND `Closed() == false` (conn survives a stream RST).
  2. **tx_reset hook:** start a RoundTrip with a per-call ctx; cancel the ctx while the backend holds the stream; assert RoundTrip returns `ctx.Err()` AND the tx counter == 1 AND the rx counter == 0 (the codec sent CANCEL).
  3. **nil hooks:** a conn built with NO option fires no hook and does not panic on either RST path.

- [ ] **Step 2: Run — expect FAIL** (`WithResetHooks` undefined):

```bash
go test ./internal/filter/hcm/h2/ -run 'TestClientConnResetHooks' -count=1 2>&1 | tail
```

- [ ] **Step 3: Implement.** Add the hook fields to `ClientConn` (near `goawayCh`):
```go
	// onRxReset / onTxReset are nil-guarded behavioral hooks fired when this conn
	// RECEIVES (dispatchFrame RSTStreamFrame) / SENDS (RoundTrip ctx.Done CANCEL)
	// a RST_STREAM. The cluster pool wires them to its http2.rx_reset / tx_reset
	// counters at dial (WithResetHooks); nil for non-pooled conns. The codec stays
	// stats-agnostic (no internal/stats import). (phase 43.2b, ADR-0254)
	onRxReset func()
	onTxReset func()
```
Add the option type + constructor option:
```go
// ClientConnOption configures a ClientConn at construction, applied BEFORE the
// readLoop goroutine starts (so hook fields are race-free). (phase 43.2b)
type ClientConnOption func(*ClientConn)

// WithResetHooks installs the RST_STREAM rx/tx behavioral hooks. Either may be
// nil. The pool passes cluster-counter increments; the codec fires them
// nil-guarded at its existing RST sites. (phase 43.2b, ADR-0254)
func WithResetHooks(onRx, onTx func()) ClientConnOption {
	return func(cc *ClientConn) { cc.onRxReset = onRx; cc.onTxReset = onTx }
}
```
Change the constructor signature to variadic and apply options before `go cc.readLoop()` (`:186`):
```go
func NewClientConn(ctx context.Context, upstream net.Conn, opts ...ClientConnOption) (*ClientConn, error) {
	// ... build cc ...
	for _, o := range opts {
		o(cc)
	}
	// ... preface / SETTINGS exchange (Steps 1-3) ...
	go cc.readLoop() // Step 4 — hooks already installed, no race
	// ...
}
```
(Place the `opts` application after the struct literal but before any framer write, certainly before `go cc.readLoop()`.) Fire the hooks nil-guarded:

At the `RSTStreamFrame` `cs.finish(...)` site (`:394`):
```go
cs.finish(streamError(ErrCode(fr.ErrCode), fr.StreamID, "client: peer RST_STREAM"))
if cc.onRxReset != nil {
	cc.onRxReset()
}
return nil
```
At the `RoundTrip` `ctx.Done` CANCEL site (`:537`):
```go
_ = cc.fr.WriteRSTStream(id, http2.ErrCodeCancel)
cc.mu.Unlock()
if cc.onTxReset != nil {
	cc.onTxReset()
}
return H2Response{}, ctx.Err()
```

- [ ] **Step 4: Run — expect PASS** + `-race` + lint. The 6 other `NewClientConn` callers (all but the production dial path) compile unchanged (variadic):

```bash
go test ./internal/filter/hcm/h2/ -race -count=1 2>&1 | tail
gofmt -l internal/filter/hcm/h2/ && golangci-lint run ./internal/filter/hcm/h2/...
```

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/hcm/h2/client.go internal/filter/hcm/h2/client_test.go
git commit -m "phase 43.2b Task 5: codec reset-hooks (WithResetHooks variadic option, applied pre-readLoop) firing at the rx RST_STREAM + tx CANCEL sites; codec stays stats-agnostic"
```

---

## Task 6: Register `http2.rx_reset` + `http2.tx_reset` (useH2-gated) + wire the codec hooks at dial

**Files:**
- Modify: `internal/cluster/cluster.go` (2 handle fields)
- Modify: `internal/cluster/h2pool.go` (`h2RxResetInc`/`h2TxResetInc` helpers)
- Modify: `internal/cluster/manager.go:191-197` (register useH2-gated)
- Modify: `internal/cluster/dial_h2.go` (thread the option; `dialPooledH2To` wires the hooks)
- Test: `internal/cluster/manager_test.go`

**Context (SPEC §7 + AMEND-H2B-2):** add EXACTLY two shapes — `cluster.<name>.http2.rx_reset` + `cluster.<name>.http2.tx_reset`, both counters, both useH2-gated (non-H2 stays byte-stable at 1183; an H2 cluster goes 1185 → 1187). The pool wires the codec hooks (Task 5) to these handles at dial via `h2.WithResetHooks`; the dead `DialH2` passes nothing (→ nil hooks). NO `goaway_sent` (reference-0-on-drain ⇒ divergence — AMEND-H2B-3) and NO `stream_refused_errors` (reference-dormant).

- [ ] **Step 1: Write the failing test** — extend `TestRegisterClusterMetrics_H2Stats` (`manager_test.go:1633`): in the H2 subtest assert `c.http2RxReset != nil` + `c.http2TxReset != nil` and `hasMetric(reg, "cluster.c_h2s.http2.rx_reset")` + `... .http2.tx_reset`; in the non-H2 subtest assert NEITHER name present. (Presence/absence — the test has no exact-count assertion to bump.)

- [ ] **Step 2: Run — expect FAIL:**

```bash
go test ./internal/cluster/ -run 'TestRegisterClusterMetrics_H2Stats' -count=1 2>&1 | tail
```

- [ ] **Step 3: Add the handle fields** to `Cluster` (`cluster.go`, next to `upstreamCxHTTP2Total` `:160`):
```go
	// http2RxReset / http2TxReset are the cluster.<name>.http2.rx_reset /
	// .http2.tx_reset counters — ++ per RST_STREAM the upstream codec RECEIVES /
	// SENDS (the FIRST codec→cluster stat wiring; the pool injects these via
	// h2.WithResetHooks at dial). Registered useH2-gated; nil for non-H2 clusters
	// (the h2pool helpers nil-guard). NO http2.goaway_sent / stream_refused_errors
	// (AMEND-H2B-3). (phase 43.2b, ADR-0254)
	http2RxReset *stats.Counter
	http2TxReset *stats.Counter
```
Add the nil-guarded helpers to `h2pool.go` (next to `h2CxHTTP2TotalInc` `:196`):
```go
// h2RxResetInc Inc's the cluster's http2.rx_reset counter (a received RST_STREAM
// observed by the codec). nil-guarded; useH2-gated handle. (phase 43.2b)
func (c *Cluster) h2RxResetInc() { incCounter(c.http2RxReset) }

// h2TxResetInc Inc's the cluster's http2.tx_reset counter (a sent RST_STREAM —
// the codec's downstream-cancel CANCEL). nil-guarded. (phase 43.2b)
func (c *Cluster) h2TxResetInc() { incCounter(c.http2TxReset) }
```

- [ ] **Step 4: Register useH2-gated** — extend the `if c.useH2 { ... }` block in `registerClusterMetrics` (`manager.go:191-197`):
```go
	if c.useH2 {
		c.upstreamCxHTTP2Total = r.NewCounter(prefix + "upstream_cx_http2_total")
		c.http2StreamsActive = r.NewGauge(prefix + "http2.streams_active")
		// Phase 43.2b (ADR-0254): the 2 behavior-driven reset counters, wired
		// from the codec via h2.WithResetHooks at dial. NO goaway_sent /
		// stream_refused_errors (AMEND-H2B-3).
		c.http2RxReset = r.NewCounter(prefix + "http2.rx_reset")
		c.http2TxReset = r.NewCounter(prefix + "http2.tx_reset")
	}
```

- [ ] **Step 5: Wire the hooks at dial.** Thread `...h2.ClientConnOption` through `h2ConnFromDialed` and pass it to `NewClientConn`:
```go
func (c *Cluster) h2ConnFromDialed(ctx context.Context, raw net.Conn, ep Endpoint, opts ...h2.ClientConnOption) (*h2.ClientConn, Endpoint, error) {
	// ... unchanged through the TLS/ALPN-or-h2c branch ...
	cc, err := h2.NewClientConn(ctx, wrapped, opts...)
	// ...
}
```
In `dialPooledH2To`, pass the cluster's reset hooks (DialH2 keeps calling `h2ConnFromDialed` with no opts → nil hooks):
```go
cc, _, ferr := c.h2ConnFromDialed(ctx, raw, ep,
	h2.WithResetHooks(c.h2RxResetInc, c.h2TxResetInc))
```
(`c.h2RxResetInc`/`c.h2TxResetInc` are method values closing over `c`; safe even when the handles are nil.)

- [ ] **Step 6: Run — expect PASS** + the full cluster suite + lint:

```bash
go test ./internal/cluster/ -count=1 2>&1 | tail
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
```

- [ ] **Step 7: Commit.**

```bash
git add internal/cluster/cluster.go internal/cluster/h2pool.go internal/cluster/manager.go internal/cluster/dial_h2.go internal/cluster/manager_test.go
git commit -m "phase 43.2b Task 6: register http2.rx_reset + http2.tx_reset (useH2-gated; 1185->1187) + wire codec hooks at dialPooledH2To via WithResetHooks (DialH2 nil); AMEND-H2B-2"
```

---

## Task 7: The GOAWAY/RST-capable h2c backend — `H2GoawayResponder` BackendKind 38 (D-H2B-BACKEND)

**Files:**
- Modify: `test/differential/fixture/fixture.go` (add `H2GoawayResponder = 38`)
- Modify: `test/differential/runner_test.go` (dispatch case + `acceptH2Goaway`)

**Context (D-H2B-BACKEND):** the rotation differential needs an h2c backend that (a) advertises `SETTINGS_MAX_CONCURRENT_STREAMS` high so the LOCAL cap binds, (b) holds each stream open on a release-gate (the `/__release` precedent), (c) emits a **GOAWAY(NO_ERROR)** on `/__goaway` (the drain trigger — a TCP close would instead cancel the ctx and hit the 43.2a `Closed()` path, NOT the GOAWAY drain path), and (d) emits a **RST_STREAM(INTERNAL_ERROR)** on a held in-flight stream on `/__rst` (the rx_reset prong). The kind-37 `H2HoldResponder` uses `h2c.NewHandler`+`http.Server` and CANNOT emit frames on demand — so this is a NEW BackendKind built on a raw `golang.org/x/net/http2.Framer` (the §11 live-probe backend is the template). Reuse the gate concept; the framer gives the frame-level control.

- [ ] **Step 1: Add the enum value** in `fixture.go` after `H2HoldResponder = 37`:
```go
	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
	// (phase 43.2b, the GOAWAY drain lifecycle). Unlike H2HoldResponder (kind 37,
	// http.Server/h2c — no frame control) it drives the framer directly so it can,
	// on control requests, emit a peer GOAWAY(NO_ERROR) (/__goaway — the drain
	// trigger) and a targeted RST_STREAM(INTERNAL_ERROR) on a held stream (/__rst
	// — the rx_reset prong), alongside the /__release hold gate. Advertises a high
	// SETTINGS_MAX_CONCURRENT_STREAMS so the cluster's LOCAL cap binds. NEW
	// BackendKind per reference_differential_fixture_dispatch_constraint.
	H2GoawayResponder BackendKind = 38
```

- [ ] **Step 2: Implement `acceptH2Goaway`** in `runner_test.go` (model the frame loop on the §11 live-probe backend / the existing raw-framer usages in the harness). It accepts a conn, reads the client preface, exchanges SETTINGS (advertising a high `MAX_CONCURRENT_STREAMS`), then runs a frame loop: decode HEADERS → if a control path (`/__release` / `/__goaway` / `/__rst`) act on the conn's framer; otherwise register the stream in a per-conn held set and block its response until a release. Control semantics:
  - `/__release` — respond 200 to all currently-held streams (re-armable, the kind-37 precedent).
  - `/__goaway` — `fr.WriteGoAway(lastStreamID, http2.ErrCodeNo, nil)` on this conn; keep serving already-open streams to completion (graceful — do NOT close the TCP conn).
  - `/__rst` — `fr.WriteRSTStream(heldStreamID, http2.ErrCodeInternal)` on the currently-held request stream (the rx_reset prong); the control request itself answers 200.
  Serialize framer writes with a per-conn mutex (mirroring the codec's `cc.mu`). Add the dispatch case in the backend `switch` (model on the `H2HoldResponder` case at `:997`).

- [ ] **Step 3: Sanity-compile + smoke** the harness (test-only):
```bash
go vet ./test/differential/...
go build ./test/differential/...
gofmt -l test/differential/ && golangci-lint run ./test/differential/...
```

- [ ] **Step 4: Commit.**
```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go
git commit -m "phase 43.2b Task 7: H2GoawayResponder BackendKind 38 — raw-framer h2c backend w/ /__release + /__goaway(NO_ERROR) + /__rst(INTERNAL_ERROR) controls (D-H2B-BACKEND)"
```

---

## Task 8: The `0080-h2-goaway-rotation` differential fixture (cross-side EXACT, sequenced barriers)

**Files:**
- Create: `test/fixtures/0080-h2-goaway-rotation/driver/driver.go` (config-as-Go-string — the 0079 mechanism)
- Create: `test/fixtures/0080-h2-goaway-rotation/driver/driver_test.go`
- Create: `test/fixtures/0080-h2-goaway-rotation/expectations.yaml`
- Create: `test/fixtures/0080-h2-goaway-rotation/README.md`

**Context (SPEC §8.1 + `reference_h2_pool_downstream_codec_gate`):** an H2 (TLS+ALPN-h2) DOWNSTREAM listener (the 0079/0004 PKI shape — MANDATORY; an H1-downstream variant never exercises the pool) → an h2c upstream cluster `c_h2gw` with `http2_protocol_options{ max_concurrent_streams: C }`, on BOTH subject and reference. Backend = `H2GoawayResponder` (Task 7). SLEEPLESS: release-barrier + poll-to-converge + sequential-per-side. Model the driver structure (incl. `h2ListenerFilterChain()` + `ReferenceBootstrap`/`SubjectConfig` Go-string templates + `pollStats`/`scrapeStats`/`release` helpers) on `0079-h2-multiplex-pool/driver/driver.go`; reuse the `0004-h2-routing/pki/` cert material.

The staged drive (SPEC §8.1):
1. **Establish** — fire a held request ⇒ poll `upstream_cx_http2_total == 1` AND `http2.streams_active == 1`.
2. **Drain (in-flight)** — trigger the backend GOAWAY honoring the in-flight stream ⇒ poll: the conn STAYS active while its stream is in flight, takes NO new stream (a concurrent request MISSes → a REPLACEMENT, `upstream_cx_http2_total → 2`), then on release the drained conn closes (`upstream_cx_active` settles).
3. **Drain (idle)** — establish a conn, let it go idle, trigger its GOAWAY ⇒ poll it closed PROMPTLY (the watcher; `upstream_cx_active` drops without a release driving it).
4. **rx_reset prong** — fire a held request, trigger a backend RST_STREAM on it ⇒ poll `http2.rx_reset == 1` + a downstream 503 (assert the DOWNSTREAM class — `reference_concurrent_attempt_downstream_class_assertion`); the conn survives.
5. **tx_reset prong** — fire a held request, cancel it DOWNSTREAM ⇒ poll `http2.tx_reset == 1`.
6. **Quiesce** — release barrier ⇒ all held drain to 200 ⇒ `http2.streams_active` + `upstream_cx_active` poll back to steady state.

- [ ] **Step 1: Write the two config-returning driver methods** — `ReferenceBootstrap(backendPorts) string` + `SubjectConfig(refListenerPort, subjListenerPort, backendPorts, subjAdminPort) string`, mirroring 0079. Reuse 0079's `h2ListenerFilterChain()` (TLS+ALPN-h2 downstream, 0004 PKI). The cluster `c_h2gw`: h2c upstream (no `transport_socket`, ADR-0166) + `typed_extension_protocol_options` H2 block with `max_concurrent_streams: C` (pick a small concrete `C`, e.g. 100 high enough that one held stream + one replacement is the whole story — the rotation is about conn identity, not stream saturation; the GOAWAY drives the multi-conn count, not the local cap). Confirm both YAMLs carry the same `c_h2gw` shape on both sides.

- [ ] **Step 2: Write the rest of `driver.go`** — the `Driver` interface methods + `BackendKind() → fixture.H2GoawayResponder` + the staged `DriveSubject`/`DriveReference` (sequential-per-side) + `AssertStats` with the poll-to-converge barriers above. Reuse 0079's `pollStats`/`scrapeStats`/`release` and add `goaway(side, backendPort)` + `rst(side, backendPort)` control helpers (hit the backend's `/__goaway` / `/__rst` over h2c, like `release` hits `/__release`). **D-H2B-CXSTATS:** scrape and assert only the counters BOTH sides emit — `upstream_cx_http2_total`, `upstream_cx_active`, `http2.streams_active`, `http2.rx_reset`, `http2.tx_reset` — and confirm during this task (via a `/stats` scrape of the subject) which `upstream_cx_*` names envoy-go actually exposes before pinning them; do NOT assert `cx_close_notify`/`cx_destroy_local`.

- [ ] **Step 3: Write `expectations.yaml`** pinning the cross-side EXACT values: `upstream_cx_http2_total == 2` after one rotation, `http2.streams_active` back to 0 at quiesce, `http2.rx_reset == 1`, `http2.tx_reset == 1`, downstream all-200 except the rx_reset prong's single 503.

- [ ] **Step 4: Run `0080` — expect GREEN** (correct selector):
```bash
go test ./test/differential/ -run 'TestDifferential/0080' -count=1 2>&1 | tail -30
```
If a count is off, re-probe with `reference_docker_probe_bridge_network` discipline (verify the backend saw `proto=HTTP/2.0` and the GOAWAY/RST landed on the intended stream on BOTH sides). A `subject ready: EOF` is a startup flake — isolate-re-run (`reference_differential_fullsuite_startup_flake`).

- [ ] **Step 5: Write `README.md`** (purpose + the cross-side EXACT rationale + the six-stage barrier drive + the D-H2B-CXSTATS assertion-scope note).

- [ ] **Step 6: Commit.**
```bash
git add test/fixtures/0080-h2-goaway-rotation/
git commit -m "phase 43.2b Task 8: 0080-h2-goaway-rotation differential — cross-side EXACT GOAWAY drain (in-flight + idle) + rx_reset/tx_reset prongs; H2-downstream PKI shape; sequenced barriers, sleepless"
```

---

## Task 9: `0080` deliberate-break proofs + flake gate + `-race`

**Files:**
- (no production changes; this task PROVES the Task-8 assertions are live)

**Context:** `reference_differential_break_protocol_count1` — go-test caches a stale PASS after a temporary break, so EVERY break check uses `-count=1`. Revert each break with `git restore` (NOT checkout-sha/amend — `feedback_subagent_worktree_detach`). Prove each load-bearing `0080` assertion bites.

- [ ] **Step 1: Break (a) — disable the admission-skip.** Temporarily drop `&& !pc.cc.GoneAway()` from `findStreamHitLocked`. Re-run `go test ./test/differential/ -run 'TestDifferential/0080' -count=1` → expect FAIL: the draining conn keeps taking streams, no replacement opens, `upstream_cx_http2_total` stays 1 (≠ 2). `git restore`.

- [ ] **Step 2: Break (b) — drop the watcher's idle-close.** Temporarily make `watchDrain`'s GOAWAY case a no-op (or skip the spawn). Re-run `-count=1` → expect FAIL on the idle-drain stage (the idle draining conn is never closed; `upstream_cx_active` does not drop). `git restore`.

- [ ] **Step 3: Break (c) — drop the rx_reset hook increment.** Temporarily comment the `cc.onRxReset()` call. Re-run `-count=1` → expect FAIL: `http2.rx_reset` stays 0 (≠ 1). `git restore`.

- [ ] **Step 4: Break (d) — drop the tx_reset hook increment.** Temporarily comment the `cc.onTxReset()` call. Re-run `-count=1` → expect FAIL: `http2.tx_reset` stays 0 (≠ 1). `git restore`.

- [ ] **Step 5: Flake gate — 20 consecutive clean runs:**
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0080' -count=1 2>&1 | tail -1; done
```
Expected: 20/20 PASS. A `subject ready: EOF` is a startup flake, not a real failure — isolate-re-run to confirm.

- [ ] **Step 6: `-race` the fixture once:**
```bash
go test ./test/differential/ -run 'TestDifferential/0080' -race -count=1 2>&1 | tail
```

- [ ] **Step 7: Record the break evidence** in PROGRESS (each break → the assertion it bit) and commit.
```bash
git add docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS-43.2b.md
git commit -m "phase 43.2b Task 9: 0080 deliberate-break proofs (4 breaks, all bit, -count=1) + 20/20 flake gate + -race clean"
```

---

## Task 10: ADR-0254 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + fuzzer reconcile + completion six-gate (row 43 → done; family CLOSES)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0254 §Decision + §Consequences — §Context drafted in SPEC §13)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (extend the H2-pool subsection + stat-surface 1185→1187)
- Modify: `docs/envoy-go/STATE.md` (active-phase + counts + fuzzer reconcile)
- Modify: `docs/envoy-go/ROADMAP.md` (leg 43.2b done; **row 43 → done**; family 39–43 CLOSES)
- Modify: `docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS-43.2b.md` (final state)

**Context:** the atomic landing of the 43.2b bundle (ADR-0044: §Decision/§Consequences land at IMPL; §Context already in SPEC §13). Row 43 flips `done` — ALL legs (43.1 + 43.2a + 43.2b) landed (`reference_roadmap_split_phase_row_done`, ADR-0106) ⇒ the Upstream-robustness family (39–43) CLOSES.

- [ ] **Step 1: Write ADR-0254 §Decision + §Consequences** in `DECISIONS.md` (copy/refine the SPEC §13 §Context). Record: the codec-visible GOAWAY signal (`GoneAway`/`GoneAwayCh`/`Done`); the admission-skip + `(Closed()||GoneAway())` eviction; the per-conn drain-watcher (idle prompt-close); lazy replacement via the MISS path; the first codec→cluster stat wiring (`http2.rx_reset`/`http2.tx_reset` via `WithResetHooks`); the rotation observed via `upstream_cx_*` (NO new GOAWAY stat — AMEND-H2B-1); the DEFERRED hardening items (`goaway_sent`, `stream_refused_errors`, `min(local,peer)`, `nextStreamID` 2^31 retirement). Confirm next-free becomes **ADR-0255**.

- [ ] **Step 2: Write the BEHAVIOR_CONTRACT delta** — extend `### Cluster — HTTP/2 upstream multiplex connection pool` (SPEC §9) with the drain lifecycle + the two reset counters; advance the stat-surface block 1185→1187.

- [ ] **Step 3: Update STATE.md** — active-phase `phase 43.2b (h2-connection-pool) IMPL done`; counts: stat **1187** (H2; non-H2 **1183**) / fixtures **82** (`0080`) / fuzzers **43** (RECONCILED from documented-42 — D-H2B-FUZZER-RECONCILE) / BackendKind **38** (`H2GoawayResponder`) / DECISIONS **ADR-0254** (next-free **ADR-0255**). Demote the prior 43.2b-SPEC active-phase line.

- [ ] **Step 4: Update ROADMAP.md** — mark leg 43.2b done AND flip **row 43 (`connection-pooling`) → `done`**; note the Upstream-robustness family (phases 39–43) CLOSES.

- [ ] **Step 5: Fuzzer reconcile** — update any running-total doc that says "fuzzers: 42" to **43** (the actual `^func Fuzz` count; `reference_fuzzer_count_docs_drift`). 43.2b adds NO fuzzer; this only corrects the documented figure. Note it in PROGRESS.

- [ ] **Step 6: The full six-gate:**
```bash
go build ./...
go vet ./...
gofmt -l internal/ test/                                  # no output
golangci-lint run ./...
go test ./... -count=1 2>&1 | tail -30                     # unit + integration
go test ./test/differential/ -count=1 2>&1 | tail -40      # full 82-dir differential GREEN
```
Expected: all green; differential 82/82 (D-H2B-BYTESTAB: `0004` + `0079` unaffected). Re-confirm 1187 for an H2 cluster + 1183 for non-H2. A `subject ready: EOF` is a startup flake — isolate-re-run (`reference_differential_fullsuite_startup_flake`).

- [ ] **Step 7: Commit the completion bundle.**
```bash
git add docs/envoy-go/
git commit -m "phase 43.2b (h2-connection-pool) IMPL: graceful GOAWAY-driven connection rotation — drain lifecycle (admission-skip + watcher + lazy replacement) + http2.rx_reset/tx_reset (1185->1187) + 0080 cross-side EXACT; ANCHORS ADR-0254; row 43 DONE, Upstream-robustness family (39-43) CLOSES"
```

---

## Final review + handoff

After Task 10, dispatch a final `superpowers:code-reviewer` over the whole 43.2b diff (the entire branch vs `master`), focused on: the single-mutator invariant (no `inFlight`/pool access outside `h2PoolMu`); the watcher's double-evict guard (`findPooledLocked != nil` re-check) + no goroutine leak (the `Done()` exit); the `findConnectingHitLocked` site STAYS GoneAway-guard-free (coalescing preserved — `reference_h2_pool_connect_coalescing`); the reset-hook race-freedom (options applied before `go cc.readLoop()`); byte-stability (non-H2 + no-GOAWAY paths unchanged); permit + LB-release conservation unchanged from 43.2a. Then `superpowers:finishing-a-development-branch` → the controller squash-merges to `master` + pushes (`feedback_push_to_origin`).

**Counts at 43.2b IMPL exit:** stat **1187** (H2; non-H2 1183) / fixtures **82** (`0080-h2-goaway-rotation`) / fuzzers **43** (reconciled) / BackendKind **38** (`H2GoawayResponder`) / DECISIONS **ADR-0254** (next-free **ADR-0255**). ROADMAP row 43 **`done`** ⇒ the Upstream-robustness family (phases 39–43) CLOSES.
