# Phase 43.2a Implementation Plan — the HTTP/2 upstream multiplex connection pool substrate

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The controller squashes + pushes at stage-close; subagents commit LOCALLY only (`feedback_subagents_no_push`).

**Goal:** Replace the ADR-0056 per-request-fresh upstream HTTP/2 dial with a per-endpoint pool of reusable, multiplexed `*h2.ClientConn`s — many concurrent streams ride a small number of connections; a new connection opens (consuming a 43.1 `max_connections` permit) only when the existing connections are stream-saturated at the cluster's own `http2_protocol_options.max_concurrent_streams`; when both stream and connection capacity are exhausted a request PENDS on a stream-aware bounded wait-queue (queue-full ⇒ fail-fast 503).

**Architecture:** A new `internal/cluster/h2pool.go` holds `c.h2Pool map[string][]*pooledH2Conn` (keyed by `ep.Addr()`, guarded by `c.h2PoolMu`) + a SEPARATE per-endpoint stream-aware pending queue, layered over the 43.1 connection-pool permit substrate (ADR-0252). The local cap (`http2_protocol_options.max_concurrent_streams`) drives `ceil(K/C)` multi-conn growth — the load-bearing finding of the SPEC (AMEND-H2-1, the live probe REVERSED the brainstorm's peer-driven choice). The router's two H2 call sites swap `DialH2`+`defer cc.Close()` for `AcquireH2Stream`+`releaseStream`. Adds 2 stats (`upstream_cx_http2_total` + `http2.streams_active`). Supersedes ADR-0056 (ADR-0253).

**Tech Stack:** Go; the in-tree `internal/filter/hcm/h2` codec (already multiplex-capable); the 43.1 `connPool` permit seam (`sync.Mutex` + FIFO buffered-cap-1 wake); the Docker-bridge differential harness (`reference_docker_probe_bridge_network`). ZERO new Go packages; ZERO new go.mod modules.

**Source SPEC:** `docs/envoy-go/phases/43.2-h2-connection-pool/SPEC.md` (commit `17b09d55`). Predecessor BRAINSTORM: `docs/envoy-go/phases/43.2-h2-connection-pool/BRAINSTORM.md` (`34de6d14`) — note AMEND-H2-1 reversed its peer-driven choice.

---

## Orientation — read before Task 1 (the zero-context brief)

envoy-go is a from-scratch Go reimplementation of Envoy, validated **cross-side** against reference `envoyproxy/envoy:contrib-v1.37.2` over a Docker bridge. Every change is proven by a differential fixture that drives BOTH proxies and compares observable behavior.

**The two admission dimensions (the mental model for this whole plan):**

1. **Connection-creation permit (43.1, `connPool`)** — gates opening a NEW upstream connection. `max_connections` is the cap; each new conn holds one permit; conn-Close releases it. Lives in `internal/cluster/connpool.go` + the `Cluster.acquireConnSlot`/`releaseConnSlot`/`connDec` seam in `cluster.go`.
2. **Stream slot (43.2a, NEW `h2Pool`)** — gates multiplexing a NEW stream onto an EXISTING connection. The per-conn budget is the cluster's `http2_protocol_options.max_concurrent_streams` (cap `C`).

A request opens a new conn (dim 1) only when every existing conn is stream-saturated (dim 2). It PENDS only when BOTH are exhausted: all conns saturated AND `max_connections` reached.

**The load-bearing finding (AMEND-H2-1, USER-CONFIRMED, live-probed 2026-06-23):** the reference's multi-conn growth keys off the cluster's OWN `max_concurrent_streams` (local cap `C`), NOT the peer's advertised SETTINGS. With `C` set and `K` fully-overlapping held streams, the reference opens EXACTLY `ceil(K/C)` connections — deterministic, zero errors. This makes the `0079` differential **cross-side EXACT** on conn/stream counts (contrast 43.1's soft `max_connections`, which forced a robust-only prong — `reference_max_connections_soft_breaker`).

**Why a SEPARATE pending queue (the crux the SPEC pins, §3.5 + BRAINSTORM §2.4):** the 43.1 `connPool.waiters` wakes ONLY on conn-close (permit release). In steady H2 load conns rarely close — capacity frees via stream-completion. A waiter that only woke on conn-close would STARVE. So the H2 pool owns its OWN per-endpoint FIFO, woken on BOTH (a) a stream completing on an existing conn (the freed slot is handed to the head waiter, which rides that conn — no permit, no dial) AND (b) a conn closing (the freed permit lets the head waiter dial). It REUSES the 43.1 wake PATTERN (`sync.Mutex` + FIFO + buffered-cap-1 channels) but is a DISTINCT structure — it must NOT overload `connPool.waiters`.

**The composition rule that keeps the two queues from fighting (this plan's key design decision):** for an H2 cluster, requests NEVER enqueue in `connPool.waiters`. They acquire a permit via a NEW **non-blocking** `tryAcquireConnSlot()` (reserve-or-fail, never enqueue) and, on failure, pend in the H2 queue instead. Therefore `connPool.waiters` is always empty for H2 clusters, so `releaseConn`'s FIFO-handoff branch never fires for them — the permit simply frees (`activeConns--`). All H2 waiter wakeups are driven by the pool itself via a single `h2Promote` primitive, called after every capacity-freeing event (stream release OR conn eviction). Conn-Close is ALWAYS pool-driven (the codec's `readLoop` cancels `cc.ctx` on error but never closes the underlying conn — `client.go:222-224`), so there is no spontaneous close that bypasses the pool.

### Key source files (read these regions; do NOT re-read whole files)

| File | What's there | This plan |
|------|--------------|-----------|
| `internal/cluster/connpool.go` | the 43.1 `connPool` (`acquireConnOrPend`/`releaseConn`/`removeWaiterLocked`/`hasWaiter`), `errConnPoolOverflow`/`IsConnPoolOverflow`, the nil-guarded `setGauge`/`incGauge`/`decGauge`/`incCounter` helpers | ADD `tryAcquireConnSlot` (Task 4) |
| `internal/cluster/cluster.go` | `Cluster` struct (`:85`), `h1Pool`/`h1PoolMu` (`:96`), the permit seam `acquireConnSlot`/`releaseConnSlot`/`connDec` (`:232-261`), `Dial`/`AcquireH1`/`PutIdleH1`, `connWithGauge` (`:614`), `Endpoint.Addr()` (`:43`), `UseH2()` (`:349`) | ADD `h2Pool`/`h2PoolMu`/`h2MaxConcurrentStreams`/stat-handle fields + the `tryAcquireConnSlot` wrapper |
| `internal/cluster/circuitbreaker.go` | `parseCircuitBreakers` (`:47`, builds `out.pool`), `registerStats` (`:111`, the +16 CB stats incl. the pending-queue handles) | the H2 queue drives the connPool's pending stat handles + `maxPendingRequests` bound |
| `internal/cluster/manager.go` | `registerClusterMetrics` (`:112`), `extractH2Mode` (`:682`, the H2-mode parse site), `buildCluster` (calls `extractH2Mode` at `:508`) | extend `extractH2Mode` to return the cap; register the 2 H2 stats (useH2-gated) |
| `internal/cluster/dial_h2.go` | `DialH2` (`:42`) — `Cluster.Dial` → `connWithGauge` assert → TLS/ALPN-or-h2c branch → `h2.NewClientConn` | factor the dial internals into a pool-callable helper (Task 5) |
| `internal/filter/hcm/h2/client.go` | `ClientConn` struct (`:64`), `ctx` field (`:65`), `RoundTrip` (`:479`, checks `cc.ctx.Err()` at `:480`), `nextStreamID`/`goawayCh`/`serverS` | ADD `Closed()` predicate (Task 3) |
| `internal/filter/http/router/router_h2.go` | `doH2ClusterAction` (`:72`, the LIVE site; `DialH2` at `:97`, `IsConnPoolOverflow→503` at `:99`, `defer cc.Close()` at `:117`) + the legacy `doH2` (`:290`; `DialH2` at `:314`, `defer cc.Close()` at `:330` w/ the `ADR-0056` comment) | rewire BOTH sites (Task 7) |
| `test/differential/fixture/fixture.go` | `BackendKind` enum (tail `BlockingHoldResponder = 36` at `:588`) | ADD `H2HoldResponder = 37` (Task 8) |
| `test/differential/runner_test.go` | the `switch uniformKind` backend dispatch (`BlockingHoldResponder` case at `:981`; `acceptBlockingHold`) | ADD the `H2HoldResponder` case + `acceptH2Hold` (Task 8) |
| `test/fixtures/0078-connection-pool-max-connections/` | the 43.1 fixture — the closest model. **Configs are GENERATED by driver methods, NOT yaml files**: `cpDriver.ReferenceBootstrap(backendPorts) string` (`:189`) + `cpDriver.SubjectConfig(refListenerPort, subjListenerPort, backendPorts, subjAdminPort) string` (`:235`). Tree is only `driver/`, `expectations.yaml`, `README.md`. Release-barrier + poll-the-gauge + sequential-per-side; `AssertStats` (`:636`) scrapes `/stats`. | the `0079` driver template (Task 9) |
| `test/differential/fixture/fixture.go:27-34` | the `Driver` interface: `ReferenceBootstrap` + `SubjectConfig` return config YAML as Go STRINGS (no standalone yaml files for differential fixtures) | the `0079` driver implements these |

### Discipline (honor on EVERY task)

- `feedback_pertask_gofmt_lint` — each task runs `gofmt -l` (must print nothing) + `golangci-lint run` on touched packages, NOT just `go vet`.
- `feedback_subagents_no_push` — commit LOCALLY only; the controller squashes + pushes at stage-close.
- `reference_differential_break_protocol_count1` — go-test caches results; every deliberate-break check AND every `-race` run uses `-count=1`, or a stale PASS masks a dead assertion.
- `reference_differential_run_selector` — a single fixture is `-run 'TestDifferential/0079'` (the subtests are `TestDifferential/<fixture>`; a bare `-run '0079'` matches ZERO subtests → vacuous green).
- `reference_concurrency_differential_release_barrier` + `reference_concurrent_attempt_downstream_class_assertion` — the `0079` hold backend uses a release-barrier + poll-the-gauge + sequential-per-side, NEVER a `time.Sleep`; assert the DOWNSTREAM response class (`downstream_rq_2xx`/`5xx`), not the over-counting upstream class.
- `reference_docker_probe_bridge_network` — any live reference probe uses a shared bridge + a backend hostname reachable from BOTH containers; verify decode actually ran (`upstream_cx_total > 0`).
- The stat surface count is **1183** at the start and **1185** at the end (H2 clusters only — the 2 new names register useH2-gated so non-H2 fixtures stay byte-identical). Fuzzers STAY documented-**42** (actual `^func Fuzz` = 43; `reference_fuzzer_count_docs_drift` — 43.2a adds none; carry the figure unchanged).

---

## File structure (decomposition locked here)

- **`internal/cluster/h2pool.go` (NEW, ~200 LoC)** — the pool: `pooledH2Conn`, the per-endpoint stream-aware pending queue (`h2Waiter`/`h2Grant`), `AcquireH2Stream`, `releaseStream`, the `h2Promote` primitive, the `evictH2Conn` error-eviction helper, the `dialPooledH2` MISS-path dial. ALL admission accounting (`inFlight`, the waiter FIFO, the `streams_active` gauge) is guarded SOLELY by `c.h2PoolMu` (D-H2-MUTEX).
- **`internal/cluster/h2pool_test.go` (NEW)** — the `-race -count=1` unit matrix for the queue primitive + the promotion ordering (the 43.1 Task-3 precedent).
- **`internal/cluster/connpool.go` (MODIFY)** — add `tryAcquireConnSlot()` (non-blocking permit reserve-or-fail with the AMEND-CP1 soft-signal parity).
- **`internal/cluster/cluster.go` (MODIFY)** — add the `h2Pool`/`h2PoolMu`/`h2MaxConcurrentStreams` + the 2 stat-handle fields to `Cluster`; add the `Cluster.tryAcquireConnSlot` wrapper (nil-pool guard).
- **`internal/cluster/manager.go` (MODIFY)** — `extractH2Mode` returns the parsed cap; `registerClusterMetrics` registers the 2 H2 stats useH2-gated.
- **`internal/cluster/dial_h2.go` (MODIFY)** — factor `DialH2`'s dial internals into `dialPooledH2` (pool-callable); keep/remove the legacy single-shot per D-H2-LEGACY.
- **`internal/filter/hcm/h2/client.go` (MODIFY)** — add `Closed()`.
- **`internal/filter/http/router/router_h2.go` (MODIFY)** — rewire both H2 sites.
- **`test/differential/fixture/fixture.go` + `runner_test.go` (MODIFY)** — `H2HoldResponder = 37` + dispatch.
- **`test/fixtures/0079-h2-multiplex-pool/` (NEW)** — `driver/driver.go` (implements the `Driver` interface incl. `ReferenceBootstrap`/`SubjectConfig` which RETURN the config YAML as Go strings — NOT standalone yaml files; the 0078 mechanism), `driver/driver_test.go`, `expectations.yaml`, `README.md`.
- **Docs (MODIFY at the completion task):** `docs/envoy-go/DECISIONS.md` (ADR-0253 body), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS.md` (NEW).

---

## Task 1: Phase scaffolding — PROGRESS.md + baselines + the final ADR-0045 split re-check (D-H2-SPLIT-FINAL)

**Files:**
- Create: `docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS.md`

- [ ] **Step 1: Capture the green baseline.** Run from the worktree root and record the outputs in PROGRESS.md:

```bash
go build ./...
go vet ./...
gofmt -l internal/ test/        # expect: no output
golangci-lint run ./internal/cluster/... ./internal/filter/... 2>&1 | tail -5
go test ./internal/cluster/... ./internal/filter/hcm/h2/... 2>&1 | tail -20
```

Expected: build/vet clean; `gofmt -l` prints nothing; lint clean; unit tests PASS.

- [ ] **Step 2: Record the count baselines** in PROGRESS.md: stat surface **1183**; fixtures **80** (tail `0078`); fuzzers documented **42** (actual `^func Fuzz` = 43 — `reference_fuzzer_count_docs_drift`); BackendKind tail **36** (`BlockingHoldResponder`); DECISIONS tail **ADR-0252** (next-free **ADR-0253**). Verify each:

```bash
grep -c '^func Fuzz' $(git grep -l '^func Fuzz' -- '*_test.go') | awk -F: '{s+=$2} END{print "fuzzers:", s}'
grep -n 'BackendKind = 3[0-9]' test/differential/fixture/fixture.go | tail -1
ls -d test/fixtures/00* | wc -l
```

- [ ] **Step 3: D-H2-SPLIT-FINAL — the final ADR-0045 re-check.** Confirm 43.2a ships as ONE flat leg (the 43.1 D-S431-7 precedent: ~12 tasks, ≤~400 prod LoC, one differential). Record in PROGRESS.md: "43.2a ships flat; GOAWAY rotation + reset stats remain 43.2b (ADR-0254)." No task-spine sub-split.

- [ ] **Step 4: Confirm ROADMAP posture.** Record: row 43 (`connection-pooling`) STAYS `in-progress` at 43.2a completion — 43.1 done + 43.2a done, 43.2b pending. The row flips `done` only when ALL legs land (`reference_roadmap_split_phase_row_done`). Do NOT edit ROADMAP yet (that is Task 12).

- [ ] **Step 5: Commit.**

```bash
git add docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS.md
git commit -m "phase 43.2a Task 1: PROGRESS scaffolding + green baselines + ADR-0045 split re-check (flat)"
```

---

## Task 2: Parse `http2_protocol_options.max_concurrent_streams` → `Cluster.h2MaxConcurrentStreams`

**Files:**
- Modify: `internal/cluster/cluster.go` (add the field + the const)
- Modify: `internal/cluster/manager.go:682` (`extractH2Mode` returns the cap) + `:508` (`buildCluster` wires it)
- Test: `internal/cluster/manager_test.go`

**Context:** `extractH2Mode` already unmarshals `HttpProtocolOptions` and switches on `Http2ProtocolOptions`. We extend it to ALSO read `max_concurrent_streams` (a `UInt32Value` on `http2_protocol_options`). Per AMEND-H2-3 the default (absent / `0`) is a HIGH cap ⇒ effectively single-conn multiplex. Per D-H2-REJECT: NO new reject arm (the reference accepts any value; no documented bound envoy-go must reject) — confirm in the test, do not add validation.

- [ ] **Step 1: Write the failing test.** In `manager_test.go`, add a test that builds a cluster with `typed_extension_protocol_options` carrying `http2_protocol_options{ max_concurrent_streams: 2 }` and asserts the built `*Cluster` has `h2MaxConcurrentStreams == 2`; a second sub-case with `http2_protocol_options{}` (no cap) asserts `== h2DefaultMaxConcurrentStreams`. Follow the existing `extractH2Mode` / `buildCluster` test patterns already in `manager_test.go` (search for `extractH2Mode` or `useH2`).

- [ ] **Step 2: Run — expect FAIL** (field does not exist / value not threaded):

```bash
go test ./internal/cluster/ -run 'TestExtractH2Mode|TestBuildCluster' -count=1 2>&1 | tail
```

- [ ] **Step 3: Add the field + const** to `cluster.go`. Near the `useH2 bool` field (`:104`):

```go
	// h2MaxConcurrentStreams is the per-connection outbound stream budget for an
	// H2-upstream cluster — the cluster's OWN http2_protocol_options.max_concurrent_streams
	// (the LOCAL cap; AMEND-H2-1). It drives multi-conn growth in the h2Pool: a new
	// pooled ClientConn opens only when every existing conn holds this many in-flight
	// streams. Absent/0 ⇒ h2DefaultMaxConcurrentStreams (effectively single-conn
	// multiplex; AMEND-H2-3). Set by buildCluster from extractH2Mode; 0 for non-H2
	// clusters (unread there). (ADR-0253)
	h2MaxConcurrentStreams int64
```

Near `h1PoolMaxPerEndpoint` (`:79`):

```go
// h2DefaultMaxConcurrentStreams is the per-conn stream cap applied to an H2-upstream
// cluster when http2_protocol_options.max_concurrent_streams is absent or 0. Effectively
// unbounded (the reference's default is ≈2^31-1 — D-H2-DEFAULT/AMEND-H2-3) so the pool
// multiplexes onto a single connection unless the cap is configured small.
const h2DefaultMaxConcurrentStreams int64 = 1 << 30
```

- [ ] **Step 4: Extend `extractH2Mode`.** Change its signature to `(useH2 bool, h2MaxStreams int64, err error)`. On the `Http2ProtocolOptions` branch, read the cap:

```go
		case *upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions:
			useH2 = true
			h2MaxStreams = h2DefaultMaxConcurrentStreams
			if h2 := up.ExplicitHttpConfig.GetHttp2ProtocolOptions(); h2 != nil {
				if v := h2.GetMaxConcurrentStreams(); v != nil && v.GetValue() > 0 {
					h2MaxStreams = int64(v.GetValue())
				}
			}
```

Update every `return` in `extractH2Mode` to the 3-value form (the non-H2 returns use `0`). Update the caller in `buildCluster` (`:508`) to `useH2, h2MaxStreams, err := extractH2Mode(...)` and set `cl.h2MaxConcurrentStreams = h2MaxStreams` after the H2 branch. (Confirm the exact `GetHttp2ProtocolOptions`/`GetMaxConcurrentStreams` accessor names against the `upstreamshttpv3` + `corev3` generated protos; the field is `core.v3.Http2ProtocolOptions.max_concurrent_streams`, a `UInt32Value`.)

- [ ] **Step 5: Run — expect PASS.** Also run the full cluster suite to confirm no regression:

```bash
go test ./internal/cluster/ -count=1 2>&1 | tail
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
```

- [ ] **Step 6: Commit.**

```bash
git add internal/cluster/cluster.go internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 43.2a Task 2: parse http2_protocol_options.max_concurrent_streams onto Cluster.h2MaxConcurrentStreams (local cap, default high; AMEND-H2-1/H2-3)"
```

---

## Task 3: `ClientConn.Closed()` liveness predicate

**Files:**
- Modify: `internal/filter/hcm/h2/client.go`
- Test: `internal/filter/hcm/h2/client_test.go` (or the existing client test file)

**Context:** The pool's admission scan must skip a torn-down conn. `RoundTrip` already gates on `cc.ctx.Err()` (`:480`). We expose that as a predicate. **DEVIATION FROM SPEC:** the SPEC §3.2 names it `closed()` (lowercase); the pool lives in package `cluster` and calls it on `*h2.ClientConn` across the package boundary, so it MUST be exported — `Closed()`. (`goneAway()` / GOAWAY handling is 43.2b — do NOT add it here.)

- [ ] **Step 1: Write the failing test.** A fresh `ClientConn` (from a test harness or a `NewClientConn` over an in-memory pipe — reuse the existing client-test scaffolding) reports `Closed() == false`; after the conn's ctx is canceled (or the conn is `Close()`d) it reports `true`.

- [ ] **Step 2: Run — expect FAIL** (`Closed` undefined):

```bash
go test ./internal/filter/hcm/h2/ -run 'TestClientConnClosed' -count=1 2>&1 | tail
```

- [ ] **Step 3: Implement.** Near `RoundTrip` in `client.go`:

```go
// Closed reports whether this connection's frame loop has torn down (the
// conn-lifetime ctx is canceled, e.g. after Close or a transport error). The
// h2 pool's admission scan skips a Closed conn; the pool evicts+Closes a
// Closed conn once its last in-flight stream drains. (phase 43.2a, ADR-0253)
func (cc *ClientConn) Closed() bool { return cc.ctx.Err() != nil }
```

- [ ] **Step 4: Run — expect PASS** + lint:

```bash
go test ./internal/filter/hcm/h2/ -count=1 2>&1 | tail
gofmt -l internal/filter/hcm/h2/ && golangci-lint run ./internal/filter/hcm/h2/...
```

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/hcm/h2/client.go internal/filter/hcm/h2/client_test.go
git commit -m "phase 43.2a Task 3: add ClientConn.Closed() liveness predicate (exported for the cluster-pkg pool)"
```

---

## Task 4: The stream-aware pending queue + `tryAcquireConnSlot` + `h2Promote` (the LOAD-BEARING concurrency)

**Files:**
- Create: `internal/cluster/h2pool.go`
- Create: `internal/cluster/h2pool_test.go`
- Modify: `internal/cluster/connpool.go` (add `tryAcquireConnSlot`)
- Modify: `internal/cluster/cluster.go` (add `h2Pool`/`h2PoolMu` fields + the `Cluster.tryAcquireConnSlot` wrapper)

**Context (the concurrency contract — D-H2-MUTEX + D-H2-EVICTORDER):** This task builds the queue primitive + the promotion routine in ISOLATION, with a `-race -count=1` unit matrix, BEFORE wiring the acquire/release lifecycle (Task 5) on top. ALL of `inFlight`, the waiter FIFO, and the `streams_active` count are guarded SOLELY by `c.h2PoolMu` (single-mutator discipline; the 43.1 Task-3 precedent) — there is NO conn-level stream counter. The only out-of-lock operation is the buffered cap-1 wake send (never blocks).

The promotion primitive `h2Promote(addr)` is the single point that wakes a waiter, called after EVERY capacity-freeing event. Its ordering resolves **D-H2-EVICTORDER**: a waiter is handed a STREAM slot only on a LIVE (`!Closed()`) conn; it is NEVER handed a dead conn. If no live conn has a free slot, it tries a permit (dial-grant). Eager-close of a `Closed() && inFlight==0` conn happens in `releaseStream`/`evictH2Conn`, not here.

- [ ] **Step 1: Write the failing tests** (`h2pool_test.go`) — these tests construct an `h2Pool` state directly (no real conns; use `pooledH2Conn` with a `nil` or stub `cc` where the test only exercises counting/queueing, and a tiny fake for `Closed()` where liveness matters). Cover:
  1. `tryAcquireConnSlot` returns `true` and increments `activeConns` while under `maxConnections`; returns `false` (NO enqueue, `len(waiters)` unchanged on the connPool) at the cap; sets `cx_open` + Inc's `upstream_cx_overflow` on the at-cap failure (AMEND-CP1 parity).
  2. `tryAcquireConnSlot` on a nil-pool cluster returns `true` (no gating).
  3. `h2Promote` with a queued waiter + a live conn with a free stream slot hands a `h2Grant{cc: <that conn>}` and increments that conn's `inFlight` (stream handoff; no permit movement).
  4. `h2Promote` with a queued waiter + all conns saturated + a free permit hands a `h2Grant{cc: nil}` (dial-grant) and reserves the permit (`activeConns++`).
  5. `h2Promote` with a queued waiter + all conns saturated/Closed + NO permit leaves the waiter queued (no grant sent).
  6. **D-H2-EVICTORDER:** `h2Promote` with a queued waiter where the only free-slot conn is `Closed()` does NOT hand that conn (falls through to the permit path).
  7. **Race matrix (`-race`):** N goroutines concurrently enqueue-and-wait while M goroutines call `releaseStream`/`h2Promote`; assert no lost wakeups, no double-grant, `streams_active`/`inFlight`/`activeConns` all return to 0 at quiescence. Model on the 43.1 `connpool_test.go` drain-give-back race (search it for the pattern, ≥3000 iters).
  8. ctx-cancel-while-pending: a waiter whose ctx fires before a grant arrives is removed from the FIFO (the `pending_active` gauge decrements); if a grant RACED the cancel (already dequeued), the grant is drained and given back (the 43.1 drain-give-back discipline — for a stream-grant: `releaseStream` the slot; for a dial-grant: `releaseConnSlot` the permit).

- [ ] **Step 2: Run — expect FAIL** (types/functions undefined):

```bash
go test ./internal/cluster/ -run 'TestH2Pool' -race -count=1 2>&1 | tail
```

- [ ] **Step 3: Implement `tryAcquireConnSlot` in `connpool.go`** — the non-blocking sibling of `acquireConnOrPend`:

```go
// tryAcquireConnSlot reserves a connection-creation permit WITHOUT blocking or
// enqueuing. Returns true with a permit held (the caller MUST pair it with
// exactly one releaseConn) when under max_connections; false (no permit, no
// waiter appended) when at the cap. The H2 pool uses this instead of
// acquireConnOrPend so an H2 request never lands in connPool.waiters (whose
// wake fires only on conn-close) — H2 waiters live in the stream-aware queue
// in h2pool.go and are woken by h2Promote. Mirrors acquireConnOrPend's
// cap-crossing soft-signal parity (cx_open + upstream_cx_overflow; AMEND-CP1).
// (phase 43.2a, ADR-0253)
func (p *connPool) tryAcquireConnSlot() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.activeConns < p.maxConnections {
		p.activeConns++
		if p.activeConns >= p.maxConnections {
			setGauge(p.cxOpen, 1)
		}
		return true
	}
	setGauge(p.cxOpen, 1)
	incCounter(p.upstreamCxOverflow)
	return false
}
```

Add the `Cluster.tryAcquireConnSlot` wrapper in `cluster.go` next to `acquireConnSlot` (`:235`), with the nil-pool guard returning `true`:

```go
// tryAcquireConnSlot is the non-blocking permit reserve used by the H2 pool.
// true ⇒ permit held (pair with releaseConnSlot/connDec); false ⇒ at cap.
// No pool (no circuit_breakers) ⇒ always true (unbounded conn growth). (ADR-0253)
func (c *Cluster) tryAcquireConnSlot() bool {
	if c.circuitBreaker == nil || c.circuitBreaker.pool == nil {
		return true
	}
	return c.circuitBreaker.pool.tryAcquireConnSlot()
}
```

- [ ] **Step 4: Implement the pool structures + the queue + `h2Promote` in `h2pool.go`.** The shape (adapt field/method bodies as the tests require):

```go
package cluster

import (
	"context"

	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// pooledH2Conn is one multiplexed upstream H2 connection plus its in-flight
// stream count. inFlight is guarded SOLELY by Cluster.h2PoolMu (D-H2-MUTEX);
// there is no conn-level stream counter. (phase 43.2a, ADR-0253)
type pooledH2Conn struct {
	cc       *h2.ClientConn
	inFlight int64
}

// h2Grant is the wake token handed to a pending H2 waiter. cc != nil ⇒ ride
// this existing conn (a stream slot was reserved on it; no permit, no dial).
// cc == nil ⇒ a permit was reserved; the waiter dials a fresh conn. (ADR-0253)
type h2Grant struct {
	cc *h2.ClientConn
}

// h2Waiter is a pending request in the stream-aware queue. ch is buffered cap-1
// so the grant send under h2PoolMu never blocks. (ADR-0253)
type h2Waiter struct {
	ch chan h2Grant
}

// h2Promote wakes at most one head waiter for addr after a capacity-freeing
// event (stream release or conn eviction). Caller holds c.h2PoolMu. Ordering
// (D-H2-EVICTORDER): (1) hand a stream slot on a LIVE conn (never a Closed
// one); else (2) reserve a permit and hand a dial-grant; else (3) leave the
// waiter queued. The buffered send happens after the state mutation, still
// under the lock (cap-1 → never blocks).
func (c *Cluster) h2PromoteLocked(addr string) {
	q := c.h2Waiters[addr]
	if len(q) == 0 {
		return
	}
	// (1) live conn with a free stream slot
	for _, pc := range c.h2Pool[addr] {
		if !pc.cc.Closed() && pc.inFlight < c.h2MaxConcurrentStreams {
			pc.inFlight++
			c.h2StreamsActiveInc() // streams_active gauge ++ (nil-guarded)
			w := q[0]
			c.h2Waiters[addr] = q[1:]
			c.h2PendingDec() // upstream_rq_pending_active --
			w.ch <- h2Grant{cc: pc.cc}
			return
		}
	}
	// (2) a permit ⇒ dial-grant
	if c.tryAcquireConnSlot() {
		w := q[0]
		c.h2Waiters[addr] = q[1:]
		c.h2PendingDec()
		w.ch <- h2Grant{cc: nil}
		return
	}
	// (3) leave queued
}
```

(`h2StreamsActiveInc`/`h2PendingDec` etc. are thin nil-guarded helpers over the stat handles — Task 6 binds the handles; here they no-op on nil so the unit tests run registry-free, exactly like the 43.1 `setGauge` pattern. Define them in `h2pool.go` now as nil-guarded no-ops. **CRITICAL — the pending-queue helpers REUSE the 43.1 connPool handles, they do NOT register new gauges:** `h2PendingInc`/`h2PendingDec` drive `c.circuitBreaker.pool.upstreamRqPendingActive` (+`...Total` on enqueue); the overflow helper drives `c.circuitBreaker.pool.upstreamRqPendingOverflow`; the H2 queue bound is `c.circuitBreaker.pool.maxPendingRequests`. Only `h2StreamsActiveInc`/`Dec` (→ `c.http2StreamsActive`) and the per-dial counter (→ `c.upstreamCxHTTP2Total`) are NEW handles — Task 6. Guard the nil `c.circuitBreaker`/`.pool` so a no-CB H2 cluster — which never pends — is safe.) Add the `Cluster` fields in `cluster.go`:

```go
	h2PoolMu  sync.Mutex
	h2Pool    map[string][]*pooledH2Conn
	h2Waiters map[string][]*h2Waiter
```

- [ ] **Step 5: Run the race matrix — expect PASS, clean under `-race`:**

```bash
go test ./internal/cluster/ -run 'TestH2Pool' -race -count=1 2>&1 | tail -20
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
```

- [ ] **Step 6: Commit.**

```bash
git add internal/cluster/h2pool.go internal/cluster/h2pool_test.go internal/cluster/connpool.go internal/cluster/cluster.go
git commit -m "phase 43.2a Task 4: the stream-aware pending queue + tryAcquireConnSlot + h2Promote (single-mutator; -race matrix; D-H2-MUTEX/EVICTORDER)"
```

---

## Task 5: `AcquireH2Stream` / `releaseStream` admission lifecycle + the `DialH2` refactor

**Files:**
- Modify: `internal/cluster/h2pool.go` (add `AcquireH2Stream`, `releaseStream`, `evictH2Conn`, `dialPooledH2`)
- Modify: `internal/cluster/dial_h2.go` (factor the dial internals into `dialPooledH2`)
- Test: `internal/cluster/h2pool_test.go` (+ lifecycle tests; an in-process h2c backend via the existing `dial_h2_test.go` scaffolding)

**Context:** Wire the full lifecycle on top of Task 4's primitive. Signature per SPEC §3.3:
`(c *Cluster) AcquireH2Stream(ctx context.Context) (cc *h2.ClientConn, release func(), ep Endpoint, err error)`.

- [ ] **Step 1: Write failing lifecycle tests** against a real in-process h2c backend (reuse the `dial_h2_test.go` harness — search it for how it spins an h2c server + builds a plaintext-h2 `*Cluster`). Cases:
  1. **Stream HIT / multiplex:** with `h2MaxConcurrentStreams = 4` and `max_connections` high, fire 4 overlapping `AcquireH2Stream`; assert exactly ONE conn in `h2Pool` (all 4 multiplexed), `streams_active == 4`, `upstream_cx_total` (the existing gauge) shows 1 dial.
  2. **Conn growth (`ceil(K/C)`):** `C = 2`, 5 overlapping holds ⇒ exactly 3 conns (`ceil(5/2)`).
  3. **Pend + wake-on-stream-free:** `C = 1`, `max_connections = 1`, 1 held stream + a 2nd `AcquireH2Stream` ⇒ the 2nd PENDS (`upstream_rq_pending_active == 1`); release the 1st ⇒ the 2nd is woken onto the SAME conn (no new dial), `pending_active` back to 0.
  4. **Overflow:** `C = 1`, `max_connections = 1`, `max_pending_requests = 1`, 1 held + 1 pending + a 3rd ⇒ the 3rd gets `errConnPoolOverflow` (`IsConnPoolOverflow == true`) + `upstream_rq_pending_overflow` Inc.
  5. **ctx-cancel while pending** unwinds cleanly (gauge to 0; no leaked permit) — reuse the Task-4 give-back logic via the real path.
  6. **release of a `Closed()` conn's last stream** evicts+Closes it (the permit frees; `upstream_cx_active` decrements via `connWithGauge`).

- [ ] **Step 2: Run — expect FAIL:**

```bash
go test ./internal/cluster/ -run 'TestAcquireH2Stream' -race -count=1 2>&1 | tail
```

- [ ] **Step 3: Refactor `DialH2`.** Extract the body of `DialH2` (the `c.Dial` → `*connWithGauge` assert → TLS/ALPN-or-h2c branch → `h2.NewClientConn`) into `func (c *Cluster) dialPooledH2(ctx context.Context) (*h2.ClientConn, Endpoint, error)`. IMPORTANT: `c.Dial` itself calls `acquireConnSlot` (the BLOCKING permit). The pool's MISS path must NOT double-acquire — so `dialPooledH2` must dial WITHOUT re-acquiring a permit (the pool already reserved one via `tryAcquireConnSlot`). Resolve this by factoring the dial so the permit is owned by the pool:
  - Option A (preferred): `dialPooledH2` calls a permit-LESS dial. Add `Cluster.dialNoPermit(ctx)` that mirrors `Dial` but skips `acquireConnSlot` (the pool already holds the permit) and still wraps in `connWithGauge` with the `connDec` release closure (so conn-Close still frees the pool's permit via `releaseConn`). Then `dialPooledH2` = `dialNoPermit` + the TLS/ALPN/h2c branch + `h2.NewClientConn`.
  - The legacy single-shot `DialH2` either stays (delegating to the permit-acquiring path) for non-pooled callers OR is removed if Task 7 proves it dead (D-H2-LEGACY — decided in Task 7). Keep it for now; Task 7 makes the call.

  Study `cluster.go:432-475` (`Dial`) carefully — replicate the `connWithGauge{Conn: final, dec: c.connDec(release)}` wrapping EXACTLY so the gauge + LB-release + permit-release semantics stay aligned. The ONLY change is skipping `acquireConnSlot` (since the pool reserved the permit).

- [ ] **Step 4: Implement `AcquireH2Stream` / `releaseStream` / `evictH2Conn`** in `h2pool.go`:
  - `AcquireH2Stream`: lock `h2PoolMu`; lazy-init the maps; pick the endpoint (`c.PickEndpoint()`); **stream-HIT** scan (`!Closed() && inFlight < h2MaxConcurrentStreams` → `inFlight++`, `streams_active++`, unlock, return with a `release` closure); **MISS** → `tryAcquireConnSlot()`: granted ⇒ unlock, `dialPooledH2` (slow, out of lock), on success re-lock + append `pooledH2Conn{cc, inFlight:1}` + `streams_active++` + Inc `upstream_cx_http2_total`, return; on dial error ⇒ `releaseConnSlot()` + `h2Promote` + return the error; **no permit** ⇒ enqueue an `h2Waiter` (bounded by `maxPendingRequests` → else `errConnPoolOverflow` + Inc overflow), `select{ <-w.ch (a grant) ; <-ctx.Done() }`; on a `grant.cc != nil` ride it; on `grant.cc == nil` dial (permit reserved); on ctx-cancel run the give-back.
  - `release` closure (returned to the caller): under `h2PoolMu`, find the `pooledH2Conn`, `inFlight--`, `streams_active--`; then `h2PromoteLocked(addr)`; if after promotion the conn is `Closed() && inFlight == 0` → `evictH2Conn` (remove from pool + `cc.Close()` which frees the permit via `connDec`→`releaseConn`) then `h2PromoteLocked` again (the freed permit may now satisfy a dial-grant).
  - `evictH2Conn(addr, pc)`: remove `pc` from `c.h2Pool[addr]` + `pc.cc.Close()`. Used by `release` (closed-conn case) and by Task 7's RoundTrip-error path.
  - **Endpoint identity note:** the `release` closure must capture the SAME `addr` (`ep.Addr()`) used at acquire so it mutates the right per-endpoint slice.

- [ ] **Step 5: Run — expect PASS under `-race`** + lint:

```bash
go test ./internal/cluster/ -race -count=1 2>&1 | tail -20
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
```

- [ ] **Step 6: Commit.**

```bash
git add internal/cluster/h2pool.go internal/cluster/dial_h2.go internal/cluster/h2pool_test.go
git commit -m "phase 43.2a Task 5: AcquireH2Stream/releaseStream lifecycle + DialH2 permit-less dial refactor (HIT/MISS/PEND/overflow + evict)"
```

---

## Task 6: The 2 stats — `upstream_cx_http2_total` + `http2.streams_active` (useH2-gated)

**Files:**
- Modify: `internal/cluster/cluster.go` (add the 2 stat-handle fields + bind the nil-guarded helpers used in Tasks 4/5)
- Modify: `internal/cluster/manager.go:112` (`registerClusterMetrics` — register useH2-gated)
- Test: `internal/cluster/manager_test.go` (registration/count test)

**Context (AMEND-H2-2):** add EXACTLY two shapes — `cluster.<name>.upstream_cx_http2_total` (counter; ++ per new pooled H2 conn) and `cluster.<name>.http2.streams_active` (gauge; live multiplexed-stream count across the cluster's H2 conns). There is NO `upstream_cx_http2_active`. Register useH2-gated so non-H2 fixtures stay byte-identical (surface 1183); an H2 cluster's surface is 1185 (+2). The `http2.*` reset/goaway subtree stays unregistered (43.2b).

- [ ] **Step 1: Write the failing test.** Build an H2 cluster (useH2 = true) through `registerClusterMetrics` against a fresh `stats.Registry`; assert the registry contains `cluster.<name>.upstream_cx_http2_total` AND `cluster.<name>.http2.streams_active`, and does NOT contain `upstream_cx_http2_active`. Build a NON-H2 cluster; assert NEITHER new name is present (byte-stability). (Model on the existing registration test that pins the per-cluster stat set — search `manager_test.go` for `registerClusterMetrics` / a stat-name assertion.)

- [ ] **Step 2: Run — expect FAIL:**

```bash
go test ./internal/cluster/ -run 'TestRegisterClusterMetrics|TestStatSurface' -count=1 2>&1 | tail
```

- [ ] **Step 3: Add the handle fields** to `Cluster` (`cluster.go`, near the other stat fields `:114`):

```go
	// H2 multiplex-pool stats (phase 43.2a, ADR-0253) — registered useH2-gated in
	// registerClusterMetrics; nil for non-H2 clusters (the h2pool helpers nil-guard).
	upstreamCxHTTP2Total *stats.Counter // <cluster>.upstream_cx_http2_total
	http2StreamsActive   *stats.Gauge   // <cluster>.http2.streams_active
```

Bind the Task-4/5 nil-guarded helpers to these (`h2StreamsActiveInc`/`Dec` → `incGauge(c.http2StreamsActive)`/`decGauge(...)`; the per-conn-dial Inc → `incCounter(c.upstreamCxHTTP2Total)`). The pending-queue helpers (`h2PendingDec` etc.) drive `c.circuitBreaker.pool.upstreamRqPendingActive`/`...Total`/`...Overflow` — reuse the SAME 43.1 handles (do NOT register new pending names; the H2 queue shares the 43.1 pending budget + stats per SPEC §3.1).

- [ ] **Step 4: Register useH2-gated** in `registerClusterMetrics` (after the circuit_breakers block, `:190`):

```go
	// Phase 43.2a (ADR-0253): the 2 H2 multiplex-pool stats, on H2-upstream
	// clusters only (useH2-gated → non-H2 clusters stay byte-stable). NO
	// upstream_cx_http2_active (it does not exist in the reference; AMEND-H2-2).
	if c.useH2 {
		c.upstreamCxHTTP2Total = r.NewCounter(prefix + "upstream_cx_http2_total")
		c.http2StreamsActive = r.NewGauge(prefix + "http2.streams_active")
	}
```

- [ ] **Step 5: Run — expect PASS** + the full cluster suite + lint:

```bash
go test ./internal/cluster/ -count=1 2>&1 | tail
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
```

- [ ] **Step 6: Commit.**

```bash
git add internal/cluster/cluster.go internal/cluster/manager.go internal/cluster/manager_test.go internal/cluster/h2pool.go
git commit -m "phase 43.2a Task 6: register upstream_cx_http2_total + http2.streams_active (useH2-gated; 1183->1185 for H2 clusters; no _http2_active; AMEND-H2-2)"
```

---

## Task 7: Router rewire — both `router_h2.go` sites (D-H2-LEGACY) + byte-stability gate (D-H2-BYTESTAB)

**Files:**
- Modify: `internal/filter/http/router/router_h2.go` (`doH2ClusterAction` `:72` + the legacy `doH2` `:290`)
- Test: existing router H2 tests + the FULL differential

**Context:** Replace `DialH2(ctx)` + `defer cc.Close()` with `AcquireH2Stream(ctx)` + `release()` at BOTH sites. The `IsConnPoolOverflow → 503` branches (`:99`, `:316`) stay — `AcquireH2Stream` returns the SAME `errConnPoolOverflow` sentinel, so those branches keep working unchanged. A `RoundTrip` transport error must EVICT the conn (so a broken conn is not reused) — call `c.evictH2Conn` (or have `release` detect a poisoned conn) on the RoundTrip-error path, then still `release()` the stream slot. **D-H2-LEGACY:** determine whether the legacy `doH2` (`:290`, still carrying the `ADR-0056: per-request fresh conn close` comment) is still reachable post-rewire; if dead, REMOVE it (+ its `write502`/`write503` if orphaned) — the ADR-0056 supersession is incomplete in code until both per-request-fresh sites are gone. Prove reachability with a call-graph grep + the test suite (does any test/route construct exercise `doH2` vs `doH2ClusterAction`?).

- [ ] **Step 1: Rewire `doH2ClusterAction`.** Replace:

```go
	cc, ep, err := a.cluster.DialH2(ctx)
	if err != nil {
		if cluster.IsConnPoolOverflow(err) { /* 503 ... */ }
		/* 502 ... */
	}
	defer func() { _ = cc.Close() }()
	resp, err := cc.RoundTrip(ctx, req)
```

with:

```go
	cc, release, ep, err := a.cluster.AcquireH2Stream(ctx)
	if err != nil {
		if cluster.IsConnPoolOverflow(err) { /* 503 ... (unchanged) */ }
		/* 502 ... (unchanged) */
	}
	defer release()
	resp, err := cc.RoundTrip(ctx, req)
	if err != nil {
		a.cluster.EvictH2ConnOnError(cc, ep) // remove the poisoned conn from the pool (exported wrapper over evictH2Conn)
	}
```

(Keep the existing ctx-cancel-vs-protocol-error discrimination after `RoundTrip` exactly as-is — the rewire changes only acquire/release, not the response handling. Add an exported `Cluster.EvictH2ConnOnError(cc, ep)` thin wrapper if the router needs to trigger eviction; or fold eviction into `release` by having `release` check `cc.Closed()`.)

- [ ] **Step 2: Rewire the legacy `doH2`** identically (or remove it per the D-H2-LEGACY reachability check). Document the decision (kept-as-delegating vs removed-as-dead) in PROGRESS.md with the grep evidence.

- [ ] **Step 3: Run the router + hcm unit tests:**

```bash
go test ./internal/filter/http/router/... ./internal/filter/hcm/... -race -count=1 2>&1 | tail
gofmt -l internal/ && golangci-lint run ./internal/filter/...
```

- [ ] **Step 4: D-H2-BYTESTAB — the full differential gate.** (Count note: the suite is still **80 dirs** here — `0079` is added in Task 9.) The rewire changes upstream conn behavior for H2-ROUTE fixtures (the only callers of these sites — `0004-h2-routing` + any downstream-H2→upstream-H2 route). Conn-reuse now reduces `upstream_cx_total`. Run the FULL suite; any failure is either a real regression OR a fixture that pins a now-changed conn count:

```bash
go test ./test/differential/ -count=1 2>&1 | tail -40
```

Expected: GREEN. The `0004` driver only probes `/ready` (not `/stats`), so it should be unaffected. If an H2 fixture breaks on a conn-count assertion, update its expectation to the conn-reuse value AND verify the reference emits the same (the reference also pools H2). Record any updated fixture in PROGRESS.md. Distinguish a real regression from a `subject ready: EOF` startup flake (`reference_differential_fullsuite_startup_flake`) by isolate-re-running the single fixture.

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/http/router/router_h2.go internal/cluster/h2pool.go docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS.md
git commit -m "phase 43.2a Task 7: rewire both router_h2.go H2 sites onto AcquireH2Stream/releaseStream + evict-on-error; D-H2-LEGACY resolved; full differential green (D-H2-BYTESTAB)"
```

---

## Task 8: The `0079` H2 hold-and-release backend — `H2HoldResponder` BackendKind 37 (D-H2-BACKEND)

**Files:**
- Modify: `test/differential/fixture/fixture.go` (add `H2HoldResponder = 37`)
- Modify: `test/differential/runner_test.go` (add the dispatch case + `acceptH2Hold`)

**Context (D-H2-BACKEND):** the multiplex proof needs an H2 (h2c) backend that (a) advertises a controllable `SETTINGS_MAX_CONCURRENT_STREAMS` ≥ `C` (so the LOCAL cap binds, not the peer — AMEND-H2-1/H2-5; this keeps `REFUSED_STREAM` from firing in the EXACT prong), and (b) holds each stream open on a release-gate, answering 200 only after a `GET /__release` control request. The 43.1 `BlockingHoldResponder` (36) is H1-only — a NEW BackendKind is required (`reference_differential_fixture_dispatch_constraint`). Implement the h2c server with `golang.org/x/net/http2` (`h2c`) or the project's in-tree h2 server codec — whichever the existing H2 backends use (check how the `HTTPSH2 = 2` backend / 0004 spins its h2c server in `runner_test.go`).

- [ ] **Step 1: Add the enum value** in `fixture.go` after `BlockingHoldResponder = 36`:

```go
	// H2HoldResponder is an in-process HTTP/2 (h2c prior-knowledge) responder
	// (phase 43.2a, the H2 multiplex pool). It advertises a configurable
	// SETTINGS_MAX_CONCURRENT_STREAMS (set >= the cluster's max_concurrent_streams
	// so the LOCAL cap binds, not the peer — AMEND-H2-1/H2-5) and holds each
	// "GET /<seg>" stream open until a "GET /__release" control request frees the
	// current batch, then answers HTTP 200. Used by 0079 to fill ceil(K/C) conns
	// deterministically before probing stream counts + the pending queue. NEW
	// BackendKind per reference_differential_fixture_dispatch_constraint.
	H2HoldResponder BackendKind = 37
```

- [ ] **Step 2: Add the dispatch case + `acceptH2Hold`** in `runner_test.go` (model the structure on the `BlockingHoldResponder` case at `:981` + `acceptBlockingHold`, but over an h2c server with a settable max-concurrent-streams + per-stream gate). The release-gate semantics mirror `acceptBlockingHold`'s re-armable `/__release`. **Simplest h2c route:** `golang.org/x/net/http2` `http2.Server{MaxConcurrentStreams: <N ≥ C>}` + `h2c.NewHandler` (or `http2.Server.ServeConn` over the raw accepted conn) advertises the configurable SETTINGS directly via the field — check how the existing `HTTPSH2 = 2` backend / `http2.ConfigureServer` (referenced near `fixture.go:137`) spins its h2c server and reuse that idiom.

- [ ] **Step 3: Sanity-compile the harness** (the backend code is test-only):

```bash
go vet ./test/differential/...
go build ./test/differential/...
gofmt -l test/differential/
```

- [ ] **Step 4: Commit.**

```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go
git commit -m "phase 43.2a Task 8: H2HoldResponder BackendKind 37 — h2c hold-and-release backend w/ configurable SETTINGS_MAX_CONCURRENT_STREAMS (D-H2-BACKEND)"
```

---

## Task 9: The `0079-h2-multiplex-pool` differential fixture (cross-side EXACT)

**Files:**
- Create: `test/fixtures/0079-h2-multiplex-pool/driver/driver.go` (implements the `Driver` interface; the configs are RETURNED as Go strings from `ReferenceBootstrap`/`SubjectConfig` — there are NO standalone `envoy.yaml`/`envoy-go.yaml` files for a differential fixture; the 0078 mechanism)
- Create: `test/fixtures/0079-h2-multiplex-pool/driver/driver_test.go`
- Create: `test/fixtures/0079-h2-multiplex-pool/expectations.yaml`
- Create: `test/fixtures/0079-h2-multiplex-pool/README.md`

**Context (SPEC §8.1):** an HTTP/1.1 downstream listener → an H2-upstream cluster `c_h2mp` with `http2_protocol_options{ max_concurrent_streams: C }` + `circuit_breakers{ max_connections: K_max, max_pending_requests: M }`, on BOTH sides. The `H2HoldResponder` (Task 8) advertises `SETTINGS_MAX_CONCURRENT_STREAMS ≥ C`. SLEEPLESS: release-barrier + poll-to-converge + sequential-per-side (`reference_concurrency_differential_release_barrier`). Model the driver structure on `0078-connection-pool-max-connections/driver/driver.go` — including its config-as-Go-string mechanism: implement `ReferenceBootstrap(backendPorts) string` (the reference Envoy YAML) + `SubjectConfig(refListenerPort, subjListenerPort, backendPorts, subjAdminPort) string` (the envoy-go YAML), `BackendKind() → fixture.H2HoldResponder`, `BackendCount`, `DriveSubject`/`DriveReference`, `AssertStats`, and `ProbeAdmin` if needed. Do NOT author standalone yaml files.

The staged drive (per SPEC §8.1):
1. Fire `K` fully-overlapping held `GET /` → poll `/stats` until the conn count converges → assert (cross-side EXACT): `upstream_cx_total == ceil(K/C)` AND `http2.streams_active == K` AND `upstream_cx_http2_total == ceil(K/C)`.
2. Multiplex proof: `upstream_cx_total ≪ K`.
3. Pending/overflow prong (when `K_max < ceil(K/C)`): further held requests PEND (poll `upstream_rq_pending_active`); J oversubscribers ⇒ queue-full ⇒ DOWNSTREAM `503` + `upstream_rq_pending_overflow` delta. Assert the DOWNSTREAM class (`reference_concurrent_attempt_downstream_class_assertion`), not the upstream class.
4. Release barrier ⇒ all held drain to 200 ⇒ `http2.streams_active` + `upstream_rq_pending_active` poll back to 0.

- [ ] **Step 1: Write the two config-returning driver methods.** `ReferenceBootstrap(backendPorts) string` (reference Envoy YAML) + `SubjectConfig(refListenerPort, subjListenerPort, backendPorts, subjAdminPort) string` (envoy-go YAML) — both as Go string templates, mirroring `0078`'s `cpDriver.ReferenceBootstrap` (`:189`) / `SubjectConfig` (`:235`). Pick concrete constants, e.g. `C = 2`, `K = 6` ⇒ `ceil(6/2) = 3` conns; for the overflow prong set `K_max = 2` (< 3) + `M = 1`. The backend `SETTINGS_MAX_CONCURRENT_STREAMS` ≥ `C` (e.g. 100). Both YAMLs carry: the HTTP/1.1 downstream listener, the route to `c_h2mp`, and `c_h2mp` with `circuit_breakers{ max_connections, max_pending_requests }` + the `typed_extension_protocol_options` H2 block with `max_concurrent_streams: C` (h2c upstream — no transport_socket, per ADR-0166).

- [ ] **Step 2: Write the rest of `driver.go`** — the `Driver` interface methods + `BackendKindAware` (`BackendKind() → fixture.H2HoldResponder`) + the staged `DriveSubject`/`DriveReference` + `AssertStats` with poll-the-gauge helpers (reuse `0078`'s `/stats` scrape via `ProbeAdmin`/`AssertStats` + the release-control request pattern). Sequential-per-side (drive subject fully, then reference). `ProbeAdmin`/`AssertStats` scrapes the EXACT stat names asserted (`upstream_cx_total`, `http2.streams_active`, `upstream_cx_http2_total`, `upstream_rq_pending_active`, `upstream_rq_pending_overflow`, `downstream_rq_2xx`/`5xx`).

- [ ] **Step 3: Write `expectations.yaml`** pinning the cross-side EXACT values (`ceil(K/C)` conns, `streams_active == K`, the pending/overflow deltas, all-200 after release).

- [ ] **Step 4: Run `0079` — expect GREEN** (use the correct selector):

```bash
go test ./test/differential/ -run 'TestDifferential/0079' -count=1 2>&1 | tail -30
```

If the conn count is off, re-probe with `reference_docker_probe_bridge_network` discipline (verify the backend saw `proto=HTTP/2.0` and `upstream_cx_total > 0` on BOTH sides). Confirm the LOCAL cap binds (no `REFUSED_STREAM`) — backend SETTINGS ≥ `C`.

- [ ] **Step 5: Write `README.md`** (the fixture's purpose + the cross-side EXACT rationale + the staged drive).

- [ ] **Step 6: Commit.**

```bash
git add test/fixtures/0079-h2-multiplex-pool/
git commit -m "phase 43.2a Task 9: 0079-h2-multiplex-pool differential — cross-side EXACT ceil(K/C) conns + streams_active==K + pending/overflow prong (release-barrier, sleepless)"
```

---

## Task 10: `0079` deliberate-break proofs + flake gate + `-race`

**Files:**
- (no production changes; this task PROVES the Task-9 assertions are live)

**Context:** `reference_differential_break_protocol_count1` — go-test caches a stale PASS after you temporarily break production code, so EVERY break check uses `-count=1`. Prove each load-bearing `0079` assertion bites.

- [ ] **Step 1: Break (a) — disable conn growth.** Temporarily force the stream-HIT scan to ignore `inFlight < cap` (always multiplex onto conn 0). Re-run `go test ./test/differential/ -run 'TestDifferential/0079' -count=1` → expect FAIL on the `ceil(K/C)` conn-count assertion (subject opens 1 conn, not 3). REVERT (`git checkout` the file — `reference_subagent_worktree_detach`: use `git restore`, do NOT checkout-sha/amend).

- [ ] **Step 2: Break (b) — disable the pending queue bound.** Temporarily make the overflow check never fire. Re-run `-count=1` → expect FAIL on the `upstream_rq_pending_overflow` / downstream-503 assertion. REVERT with `git restore`.

- [ ] **Step 3: Break (c) — drop the `streams_active` decrement** in `release`. Re-run `-count=1` → expect FAIL on the "gauges return to 0 after release" assertion. REVERT.

- [ ] **Step 4: Flake gate — 20 consecutive clean runs** (the 0078 precedent):

```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0079' -count=1 2>&1 | tail -1; done
```

Expected: 20/20 PASS. A `subject ready: EOF` startup flake (`reference_differential_fullsuite_startup_flake`) is NOT a real failure — isolate-re-run to confirm.

- [ ] **Step 5: `-race` the fixture once:**

```bash
go test ./test/differential/ -run 'TestDifferential/0079' -race -count=1 2>&1 | tail
```

- [ ] **Step 6: Record the break evidence** in PROGRESS.md (each break → the assertion it bit) and commit.

```bash
git add docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS.md
git commit -m "phase 43.2a Task 10: 0079 deliberate-break proofs (3 breaks, all bit, -count=1) + 20/20 flake gate + -race clean"
```

---

## Task 11: M-12 `closedStreams` bounding under long-lived pooled conns (D-H2-CLOSEDSTREAMS)

**Files:**
- Investigate: `internal/filter/hcm/h2/client.go` (search for `closedStreams` / the recently-closed-stream tracking)
- Modify (if a 43.2a fix is warranted): `client.go` + a unit test
- Else: document the deferral in PROGRESS.md with evidence

**Context (M-12, from the 05.1 REVIEW):** the H2 codec may keep a `closedStreams` map (recently-closed stream IDs, for late-frame handling). Pre-43.2a, conns were per-request-fresh (one stream, then Close) so the map never grew. Post-43.2a, conns are LONG-LIVED across many request lifetimes — an unbounded `closedStreams` would leak. Determine whether this is real for THIS codec.

- [ ] **Step 1: Investigate.** Grep `client.go` for `closedStreams` (or any per-stream state retained after `finish`). Determine: does anything accumulate per-stream on a `ClientConn` without being released when the stream ends? (`cc.streams` is a `sync.Map` with `defer cc.streams.Delete(id)` in `RoundTrip` — that one is bounded. Look for any OTHER retained-forever-per-stream structure.)

- [ ] **Step 2a: If bounded already** → record in PROGRESS.md: "M-12 N/A for the client codec — `cc.streams` is Delete'd per RoundTrip; no unbounded per-stream retention. D-H2-CLOSEDSTREAMS resolved: no 43.2a change." Skip to Step 3.

- [ ] **Step 2b: If a leak exists** → add a bound (e.g. cap the map / ring-buffer of recent IDs) with a unit test that drives many sequential streams on one `ClientConn` and asserts the retained set stays bounded. Run `-race -count=1`.

- [ ] **Step 3: Commit** (docs-only if 2a; code+test if 2b).

```bash
git add -A
git commit -m "phase 43.2a Task 11: D-H2-CLOSEDSTREAMS — <resolved: no leak | bounded closedStreams under long-lived pooled conns>"
```

---

## Task 12: ADR-0253 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + completion six-gate

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0253 §Decision + §Consequences — the §Context drafted in SPEC §13)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the new H2-pool subsection + the stat-surface block 1183→1185)
- Modify: `docs/envoy-go/STATE.md` (active-phase + counts)
- Modify: `docs/envoy-go/ROADMAP.md` (leg 43.2a → done; row 43 STAYS in-progress)
- Modify: `docs/envoy-go/phases/43.2-h2-connection-pool/PROGRESS.md` (final state)

**Context:** the atomic landing of the 43.2a bundle (ADR-0044: §Decision/§Consequences land at IMPL; the §Context is already in SPEC §13). Row 43 STAYS `in-progress` (43.1 + 43.2a done, 43.2b pending — `reference_roadmap_split_phase_row_done`).

- [ ] **Step 1: Write ADR-0253 §Decision + §Consequences** in `DECISIONS.md` (the §Context is in SPEC §13 — copy/refine it). Record: the per-endpoint multiplex `ClientConn` pool over the 43.1 permit substrate; local-cap-driven `ceil(K/C)` growth (AMEND-H2-1); the SEPARATE stream-aware pending queue (woken on stream-free OR conn-close; `tryAcquireConnSlot` keeps H2 out of `connPool.waiters`); minimal liveness (`Closed()`-skip + evict-on-error); the 2 stats; the SUPERSESSION of ADR-0056; GOAWAY rotation deferred to 43.2b (ADR-0254). Mark ADR-0056 as superseded-by-0253. Confirm next-free is now **ADR-0254**.

- [ ] **Step 2: Write the BEHAVIOR_CONTRACT delta** — a new `### Cluster — HTTP/2 upstream multiplex connection pool` subsection (per SPEC §9) + advance the stat-surface block 1183→1185.

- [ ] **Step 3: Update STATE.md** — active-phase `phase 43.2a (h2-connection-pool) IMPL done`; counts: stat **1185** / fixtures **81** / fuzzers **42** / BackendKind **37** / DECISIONS **ADR-0253** (next-free **ADR-0254**). Demote the prior 43.2a-SPEC active-phase line to prior.

- [ ] **Step 4: Update ROADMAP.md** — mark leg 43.2a done; row 43 STAYS `in-progress` (43.2b pending). Do NOT flip row 43 to done.

- [ ] **Step 5: The full six-gate** (the project's release gate):

```bash
go build ./...
go vet ./...
gofmt -l internal/ test/                                  # no output
golangci-lint run ./...
go test ./... -count=1 2>&1 | tail -30                     # unit + integration
go test ./test/differential/ -count=1 2>&1 | tail -40      # full 81-dir differential GREEN
```

Expected: all green; differential 81/81. Re-confirm the stat count is 1185 for an H2 cluster + 1183 for non-H2 (a representative fixture each).

- [ ] **Step 6: Commit the completion bundle.**

```bash
git add docs/envoy-go/
git commit -m "phase 43.2a (h2-connection-pool) IMPL: per-endpoint H2 multiplex pool over the 43.1 permit substrate — local-cap-driven ceil(K/C) growth + stream-aware pending queue + 2 stats (1183->1185) + 0079 cross-side EXACT; SUPERSEDES ADR-0056 (ADR-0253); row 43 stays in-progress (43.2b pending)"
```

---

## Final review + handoff

After Task 12, dispatch a final `superpowers:code-reviewer` over the whole 43.2a diff (the entire branch vs `master`), focused on: the single-mutator invariant (D-H2-MUTEX — no `inFlight`/waiter access outside `h2PoolMu`); the evict-order (D-H2-EVICTORDER — never hand a waiter a dead conn); no `connPool.waiters` enqueue from the H2 path; the permit conservation (every `tryAcquireConnSlot`-true paired with exactly one release on every path incl. dial-error + ctx-cancel); byte-stability (non-H2 fixtures unchanged). Then `superpowers:finishing-a-development-branch` → the controller squash-merges to `master` + pushes (`feedback_push_to_origin`).

**Counts at 43.2a IMPL exit:** stat **1185** / fixtures **81** (`0079`) / fuzzers **42** (documented; actual 43) / BackendKind **37** (`H2HoldResponder`) / DECISIONS **ADR-0253** (next-free **ADR-0254**). ROADMAP row 43 `in-progress` (43.1 + 43.2a done; 43.2b — GOAWAY rotation + reset stats, ADR-0254 — pending).
