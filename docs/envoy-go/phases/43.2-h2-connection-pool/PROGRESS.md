# Phase 43.2a Progress — H2 multiplex connection pool substrate

**Branch:** `phase-43.2a-impl`
**Plan:** `docs/envoy-go/phases/43.2-h2-connection-pool/PLAN.md`
**Spec:** `docs/envoy-go/phases/43.2-h2-connection-pool/SPEC.md`
**Goal:** Replace the ADR-0056 per-request-fresh H2 dial with a per-endpoint pool of reusable multiplexed `*h2.ClientConn`s; local-cap-driven `ceil(K/C)` multi-conn growth; SEPARATE stream-aware pending queue; 2 new stats; cross-side EXACT differential `0079`. Supersedes ADR-0056 (ADR-0253).

---

## Baselines (captured at Task 1, commit `a527cb35`)

### Build / vet / fmt / lint

```
$ go build ./...
(no output)
EXIT: 0  — CLEAN

$ go vet ./...
(no output)
EXIT: 0  — CLEAN

$ gofmt -l internal/ test/
(no output)
EXIT: 0  — CLEAN

$ golangci-lint run ./internal/cluster/... ./internal/filter/... 2>&1 | tail -5
(no output)
EXIT: 0  — CLEAN
```

### Unit tests

```
$ go test ./internal/cluster/... ./internal/filter/hcm/h2/... 2>&1 | tail -20
ok  	github.com/esalaine/envoy-go/internal/cluster	3.459s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	2.471s
EXIT: 0  — PASS
```

### Differential suite

Differential: green at master tip (PLAN landed at commit `a527cb35`); not re-run at Task 1 (heavy — 80-dir suite).

---

## Count baselines

| Counter | Value | Verification command |
|---------|-------|----------------------|
| Stat surface | **1183** | recorded in `docs/envoy-go/STATE.md` active-phase note |
| Fixtures | **80** (tail `0078`) | `ls -d test/fixtures/00*/ \| wc -l` → 80; `ls -d test/fixtures/00*/` tail → `0078-connection-pool-max-connections` |
| Fuzzers (documented) | **42** | carried unchanged per `reference_fuzzer_count_docs_drift` |
| Fuzzers (actual `^func Fuzz`) | **43** | `grep -c '^func Fuzz' $(git grep -l '^func Fuzz' -- '*_test.go') \| awk -F: '{s+=$2} END{print "fuzzers:", s}'` → `fuzzers: 43` |
| BackendKind tail | **36** (`BlockingHoldResponder`) | `grep -n 'BackendKind = 3[0-9]' test/differential/fixture/fixture.go \| tail -1` → `588: BlockingHoldResponder BackendKind = 36` |
| DECISIONS tail | **ADR-0252** (next-free **ADR-0253**) | `grep -n '^## ADR-' docs/envoy-go/DECISIONS.md \| tail -1` → `15809: ## ADR-0252 — connection-pool budgets ...` |

**Anticipated at 43.2a IMPL completion:** stat **1185** (+2) / fixtures **81** (`0079`) / fuzzers **42** (documented, unchanged) / BackendKind **37** (`H2HoldResponder`) / DECISIONS **ADR-0253** (next-free **ADR-0254**).

---

## D-H2-SPLIT-FINAL — final ADR-0045 split re-check

**Decision:** 43.2a ships **flat** (ONE leg, ~12 tasks, ≤~400 prod LoC, one differential `0079`).

- GOAWAY rotation and reset stats remain **43.2b** (to be recorded as ADR-0254).
- No task-spine sub-split within 43.2a.
- The 12-task spine in the PLAN is the authoritative decomposition.

---

## ROADMAP posture

Row 43 (`connection-pooling`) **STAYS `in-progress`** at 43.2a completion:
- 43.1 done (ADR-0252, commit `c8014f48`)
- 43.2a in-progress (this leg)
- 43.2b pending (GOAWAY rotation + reset stats — ADR-0254)

Row 43 flips `done` only when ALL legs 43.1 + 43.2a + 43.2b land (ADR-0106 + `reference_roadmap_split_phase_row_done`). Do NOT edit ROADMAP until Task 12.

---

## Task log

| Task | Status | Description | Commit |
|------|--------|-------------|--------|
| T1 | **DONE** | PROGRESS scaffolding + green baselines + ADR-0045 split re-check (flat) | (this commit) |
| T2 | **DONE** | Parse `http2_protocol_options.max_concurrent_streams` → `Cluster.h2MaxConcurrentStreams` (local cap, default `1<<30`; extend `extractH2Mode`; `h2DefaultMaxConcurrentStreams` const) | (this commit) |
| T3 | **DONE** | `ClientConn.Closed()` predicate (exported; `ctx.Err() != nil`) | (this commit) |
| T4 | **DONE** | Stream-aware pending queue (`pooledH2Conn`/`h2Grant`/`h2Waiter` + `enqueueWaiterLocked`/`removeH2WaiterLocked`) + `tryAcquireConnSlot` (non-blocking permit; AMEND-CP1 parity) + `h2PromoteLocked` (D-H2-EVICTORDER: stream-slot→dial-grant→leave-queued, skips `Closed()` conns); single-mutator (D-H2-MUTEX); 8-case `-race -count=1` matrix green incl. 3000-iter dial-grant drain-give-back (clean=1475/drain=1525) + 200-goroutine stream-recycle (no lost wakeup/double-grant/leak); gofmt+lint clean | (this commit) |
| T5 | **DONE** | `AcquireH2Stream`/`makeRelease`/`evictH2Conn` lifecycle on the Task-4 primitive + `DialH2` permit-less-dial refactor (`dialNoPermit` mirrors `Dial` minus `acquireConnSlot`; `dialPooledH2`=`dialNoPermit`+shared `h2ConnFromDialed`; uniform contract — on ANY dialPooledH2 error the permit is ALREADY released, caller only promotes). 6-case `-race -count=1` matrix green: multiplex-HIT (1 conn/streams_active==4), ceil(K/C) growth (5 holds@C=2→3 conns), pend+wake-on-stream-free (same conn, no 2nd dial), overflow (`IsConnPoolOverflow`+`pending_overflow`++), ctx-cancel clean unwind (no permit leak), evict-Closed-conn-last-stream (permit+cx_active→0). Added `Cluster.upstreamCxHTTP2Total` field (Task 6 binds). gofmt+lint clean | (this commit) |
| T6 | **DONE** | 2 stats: `upstream_cx_http2_total` + `http2.streams_active` (useH2-gated; non-H2 byte-stable); `TestRegisterClusterMetrics_H2Stats` (H2 + non-H2 + absent _http2_active); 1183→1185 for H2 clusters | (this commit) |
| T7 | **DONE** | Rewire the LIVE `doH2ClusterAction` site onto `AcquireH2Stream`/`release` + `EvictH2ConnOnError` + bounded `errH2GrantRaced` retry (`IsH2GrantRaced` predicate). D-H2-LEGACY: legacy `doH2` was DEAD (no prod callers) → REMOVED with orphaned helpers. D-H2-BYTESTAB: full 80-dir differential GREEN. | (this commit) |
| T8 | **DONE** | `H2HoldResponder` BackendKind 37 (h2c hold-and-release backend; `MaxConcurrentStreams=1000`; re-armable + sticky release; compile-verified D-H2-BACKEND) | (this commit) |
| T9 | **DONE** | `0079-h2-multiplex-pool` cross-side EXACT differential (driver methods; ceil prong + overflow prong; H2 downstream listener per the SPEC §8.1 topology correction) — authored at T9; GREEN after the Task 9.5 connect-time-coalescing fix | (T9/T9.5 commits) |
| T10 | **DONE** | Deliberate breaks (`-count=1`) + 20/20 flake + `-race` (3 breaks, all bit; flake gate 19/20+20/20 transient-confirmed; race clean) | (this commit) |
| T11 | **DONE** | D-H2-CLOSEDSTREAMS (M-12): NO unbounded per-stream retention in `ClientConn`; docs-only resolution. | (this commit) |
| T12 | **DONE** | ADR-0253 §Decision/§Consequences body + ADR-0056 superseded edit + BEHAVIOR_CONTRACT subsection + stat-surface block + STATE/ROADMAP + PROGRESS + the full six-gate (all GREEN; 81/81 differential) | (this commit) |

---

## Task 7 — Router rewire (both `router_h2.go` H2 sites) + evict-on-error

**Commit:** (this commit)
**Files changed:** `internal/filter/http/router/router_h2.go`, `internal/filter/http/router/router_h2_test.go`, `internal/cluster/h2pool.go`, `internal/cluster/dial_h2.go`, this PROGRESS.

### Step 1 — `doH2ClusterAction` rewired onto the pool

`DialH2(ctx)` + `defer cc.Close()` → `AcquireH2Stream(ctx)` + `defer release()`. The
`IsConnPoolOverflow → 503` branch is unchanged (`AcquireH2Stream` returns the SAME
`errConnPoolOverflow` sentinel). The ctx-cancel-vs-protocol-error discrimination after
`RoundTrip` is preserved verbatim. On a `RoundTrip` transport error the pooled conn is
evicted via the new `Cluster.EvictH2ConnOnError(cc, ep)` (so a broken conn is never
reused); the deferred `release()` still accounts THIS stream's slot.

**`errH2GrantRaced` disposition — bounded transparent retry (predicate route).**
Added `cluster.IsH2GrantRaced(err) bool` (sibling of `IsConnPoolOverflow`). The driver
loops `AcquireH2Stream` up to `h2GrantRaceMaxRetries` (3) on `IsH2GrantRaced`, then falls
back to a load-shed 503 (NOT a 502 — no upstream connect was attempted; and NOT counted as
overflow). Reasoning: `errH2GrantRaced` is a RETRYABLE lost-race (a granted conn evicted
before use); it must never leak to the user as a 502 nor be miscounted as `pending_overflow`.
The branch is *effectively unreachable* in 43.2a (a granted stream-conn has `inFlight>0`, so
it cannot be evicted out from under the grant), but it is handled cleanly + defensively. The
predicate route (vs. a single re-acquire) was chosen because it is the simplest CORRECT
handling that keeps the retry bounded and the stat semantics exact.

### Step 2 — D-H2-LEGACY: legacy `doH2` is DEAD → REMOVED

**Reachability evidence (grep call-graph):**
- `grep -rn '\.doH2(' internal/` → ONLY `router_h2_test.go` (6 call sites). NO production caller.
- The live H2 dispatch runs `chainDispatchAction.WriteH2` (h2dispatch.go) → the `H2Action`
  closure → `doH2ClusterAction`. It never invokes the `doH2` METHOD.
- `grep -rn 'h2RouterActionAdapter' internal/` → only stale doc references; the adapter no
  longer routes to `doH2`.

→ The ADR-0056 per-request-fresh supersession was INCOMPLETE in code while `doH2` lived.
**REMOVED:** the `doH2` method + its now-orphaned helpers `write502`, `write503`,
`emitAccessLogH2`, `h2UserAgent`, and the `routerActionH2.filter` field (each verified
unused after `doH2`'s removal — `grep` confirmed no other referent; the H2 access-log emit is
owned by `h2dispatch.go`, not the router action). `bad502Body`, `h2HeaderVal`, and the `do`
defensive stub are RETAINED (still used by `doH2ClusterAction` / the routeAction interface).
The 6 `doH2` unit tests were RETARGETED onto `doH2ClusterAction` (asserting the
`ActionResponse` instead of the removed `captureH2Writer` writer-inspection — the dead
`captureH2Writer` fake was removed).

**`DialH2` fate:** KEPT. It has NO production callers post-rewire, BUT it is exported and its
`dial_h2_test.go` tests exercise the shared `h2ConnFromDialed` finalizer (TLS/ALPN/h2c) that
the pool's `dialPooledH2` depends on — so it remains as the tested single-shot dial entry. Its
doc comment was updated to record the ADR-0056→ADR-0253 supersession + the no-prod-caller status.

### Step 3 — unit gates

```
$ go test ./internal/filter/http/router/... ./internal/filter/hcm/... -race -count=1
ok  internal/filter/http/router   2.7s
ok  internal/filter/hcm           1.1s
ok  internal/filter/hcm/h2        3.5s
$ go test ./internal/cluster/... -race -count=1   →  ok (5.3s)
$ gofmt -l internal/        → (clean)
$ golangci-lint run ./internal/filter/... ./internal/cluster/...  → (clean)
```

**Test-backend fix (pooled-conn reuse).** The in-process H2 test backends
(`runH2Backend` OK/503, `h2RecordingBackend.serve`) assumed ONE stream per conn (the ADR-0056
shape) — with pooling, a retry now RIDES the same pooled conn and sends a SECOND stream, which
the single-shot backends never served → the retry `RoundTrip` hung to the 10s ctx deadline
(CANCEL). Fixed both to serve MULTIPLE sequential streams per conn (connection-scoped HPACK
encoder reused across streams; `h2RecordingBackend.conns` is now a per-STREAM counter so
`statusFor(idx)` still keys by retry-attempt index). This is the unit-test analog of the
differential conn-count adjustment the task anticipated.

### Step 4 — D-H2-BYTESTAB: full differential GREEN

```
$ go test ./test/differential/ -count=1 2>&1 | tail
ok  github.com/esalaine/envoy-go/test/differential  243.338s   (all 80 fixtures)

$ go test ./test/differential/ -run 'TestDifferential/0004' -count=1
--- PASS: TestDifferential/0004-h2-routing (2.43s)
```

No fixture needed a conn-count expectation update: the only H2 fixture `0004-h2-routing` probes
`/ready` (not `/stats`), so the conn-reuse reduction in `upstream_cx_total` is unobserved by it,
exactly as the PLAN anticipated. No `subject ready: EOF` startup flakes seen.

### CONCERN (out of Task-7 scope; flagged for follow-up)

`AcquireH2Stream` selects its endpoint via `Cluster.PickEndpoint()` (per the approved PLAN Task-5
spec), which calls `c.lb.Pick(0, false, SubsetMatch{}, false)` — it does NOT read the
ctx-carried ring_hash key (`WithHashKey`) nor the subset match (`WithSubsetMatch`) that
`doH2ClusterAction` still threads onto ctx. So **H2 ring_hash / source_ip / subset affinity is
not honored on the pooled path** (the pre-rewire `DialH2(ctx)`→`c.Dial`→`c.lb.Pick(hk,…)` DID
read them). This is a property of the merged Tasks 4–6 design, NOT a Task-7 regression, and is
UNDETECTED by the differential suite (no H2 hash/subset fixture exists — `reference_differential
_hash_key_cross_side_infeasible` notes hash-key cross-side is per-side-only anyway). The router
comments + this note record it; a hash/subset-aware H2 acquire is a follow-up (43.2b or later).

## Task 7.5 — endpoint-selection-correctness fix (RESOLVES the Task-7 CONCERN above)

**The defect.** `AcquireH2Stream` picked the endpoint TWICE: a ctx-BLIND `PickEndpoint()` (=
`c.lb.Pick(0,false,SubsetMatch{},false)`) used to compute `addr` + key/scan the `h2Pool[addr]`
bucket + return `ep` to the router, AND a SECOND ctx-AWARE re-pick buried in
`dialNoPermit` (mirroring `Dial`) that actually chose the dialed endpoint. On any MULTI-endpoint
H2 cluster the two picks could disagree, so: (1) a conn dialed to epB was stored under epA's
bucket (future epA stream-HITs ride a conn to epB); (2) the router got the wrong `ep`
(access-log/stat misattribution); (3) the LB cursor advanced twice per MISS (double RR /
double least_request accounting); (4) hash/subset/source_ip affinity was lost. Single-endpoint
clusters (the only H2 fixture, `0004`) are unaffected — hence the green differential.

**The fix (ONE ctx-aware pick threaded through the dial seam — no re-pick).** `AcquireH2Stream`
now does a single `c.lb.Pick(hashKeyFrom(ctx), …, subsetMatchFrom(ctx), …)` (mirroring `Dial`),
holds the pick's `lbRelease`, and threads it through EVERY exit path so it fires EXACTLY ONCE —
immediately on a non-dialing path (stream-HIT / stream-grant / overflow / clean-cancel /
raced-give-back / grant-race) or TRANSFERRED to the dialed conn's `connWithGauge` dec (MISS-dial
/ dial-grant), where it fires at conn-Close via `connDec` (ADR-0232 OPTION C "hold until final
Close"). The new permit-less `dialPooledH2To(ctx, ep, release)` = `dialPicked(ctx, ep, release,
true)` + `h2ConnFromDialed` dials the GIVEN ep (no pick); `dialAndPool(ctx, addr, ep, lbRelease)`
keys the conn under `addr == ep.Addr()` and attributes that `ep` to the router. The now-unused
ctx-blind re-pick wrappers `dialNoPermit` + `dialPooledH2` were REMOVED (`PickEndpoint` is kept —
still used by httpclient/thriftproxy/manager_test). LB-release conservation is now a second
traced property alongside permit conservation (see the `AcquireH2Stream` doc block).

**Regression tests** (`h2pool_test.go`, two in-process h2c backends):
- `TestAcquireH2Stream_MultiEndpoint_CorrectKeying` — round-robin over 2 backends, C=1: two
  overlapping holds land one conn per endpoint, each in its OWN bucket
  (`len(h2Pool[A])==len(h2Pool[B])==1`), the bucketed conn is the one returned for that addr, and
  the returned `ep` matches. (Verified live: a deliberate wrong-key break → both buckets empty.)
- `TestAcquireH2Stream_MultiEndpoint_HashKeyAffinity` — a `recordingLB` records every Pick's
  (hashKey, hasHash): two same-key holds route to the SAME endpoint AND multiplex onto the SAME
  conn; a different key → the other endpoint/conn; exactly 3 picks total (one ctx-aware pick per
  acquire, no re-pick) all with `hasHash==true` + the exact ctx key; LB-release drains to 0 after
  conn Close. (Verified live: a deliberate ctx-blind-pick break → same key splits across
  endpoints.)

```
$ go test ./internal/cluster/ -race -count=1                                  → ok (5.3s)
$ go test ./internal/filter/http/router/... ./internal/filter/hcm/... -race -count=1  → ok
$ gofmt -l internal/        → (clean)
$ golangci-lint run ./internal/cluster/... ./internal/filter/...  → (clean)
$ go test ./test/differential/ -count=1   → ok (all fixtures; 0004 single-endpoint unaffected)
```

## Task 8 — `H2HoldResponder` BackendKind 37 (h2c hold-and-release backend, D-H2-BACKEND)

**Files changed:** `test/differential/fixture/fixture.go`, `test/differential/runner_test.go`, this PROGRESS.

### Backend structure

- **SETTINGS advertised:** `MaxConcurrentStreams: 1000` via `http2.Server{MaxConcurrentStreams: 1000}` in the `h2c.NewHandler` call. 1000 >> any cluster cap `C`, so the LOCAL cluster cap is always the binding constraint (AMEND-H2-1/H2-5 — prevents `REFUSED_STREAM` from the peer in the EXACT prong of 0079).
- **Gate mechanism:** identical to `acceptBlockingHold` — a mutex + a swappable `chan struct{}` (closed by release; re-armed by creating a new channel). The snapshot (`gate`, `sticky`) is read under the mutex, then the `<-g` wait happens OUTSIDE the mutex so many concurrent H2 streams can block without contending on the lock (safe for multiplexed concurrent invocation).
- **Body format:** `backend-<idx>:<seg>` (the `acceptBlockingHold` convention; host attribution via body, no accept counter).
- **Two release paths:** `/__release` (re-armable, for batch hold/release in the 0079 hold phase) + `/__release_sticky` (permanent open, for the drain phase — same pattern as 43.1's `acceptBlockingHold` D-S431-5 addition).
- **H2 multiplexing:** `http.Server.Serve` + `h2c.NewHandler` dispatch one goroutine per stream invoking the handler — multiplexing is automatic; the handler is stateless per-stream (only reads the gate snapshot).

### Sanity-compile gates

```
$ go vet ./test/differential/...        → (clean)
$ go build ./test/differential/...     → (clean)
$ gofmt -l test/differential/          → (clean)
$ go test ./test/differential/ -run 'TestNothingXYZ' -count=1
ok   github.com/esalaine/envoy-go/test/differential  0.083s [no tests to run]
```

All clean. BackendKind tail is now **37** (`H2HoldResponder`).

## Task 9 — `0079-h2-multiplex-pool` differential fixture (cross-side EXACT) — BLOCKED on two findings

**Files added:** `test/fixtures/0079-h2-multiplex-pool/{driver/driver.go,driver/driver_test.go,expectations.yaml,README.md}`, `test/fixtures/0079-h2-multiplex-pool/pki/{ca.pem,listener.pem,listener.key.pem}` (copied from 0004), `test/helpers/h2.go` (added `H2CRoundTrip`), `test/differential/runner_test.go` (blank-import the 0079 driver). Fixtures count becomes **81** (note for Task 12).

### Constants (single-sourced in driver.go, pinned by driver_test.go::TestConstants)

- EXACT ceil prong (cluster `c_h2mp`): `C (streamCapMP) = 2`, `K (heldK) = 6` ⇒ `ceil(6/2) = 3` conns; `max_connections = 16`, `max_pending_requests = 16` (non-binding — only C drives growth).
- Overflow prong (cluster `c_h2of`): `C = 1`, `max_connections = 1`, `max_pending_requests = 1` (1 held fills the conn, a 2nd PENDS, a 3rd OVERFLOWS → 503).
- Topology: TWO h2c clusters, ONE backend; routes `/mp → c_h2mp`, `/of → c_h2of`. Reference container listener port **19168** (next-free).

### FINDING 1 (SPEC topology error — RESOLVED in the fixture): H1-downstream does NOT engage the H2 upstream pool

SPEC §8.1 specified an "HTTP/1.1 downstream listener → H2-upstream cluster". That does **not** exercise envoy-go's H2 multiplex pool. The HCM selects the H1 vs H2 router action by the **DOWNSTREAM listener codec** (`internal/filter/hcm/filter.go:120 HttpConnectionManager_HTTP2`; `config.go:535-541` — "the H2 variant ... is exercised only on H1-listener bootstraps where UseH2() is false"). An H1-downstream listener ALWAYS drives the H1 upstream dial (`Cluster.AcquireH1/Dial`), NEVER `Cluster.AcquireH2Stream`, even for a `UseH2()==true` cluster. There is no H1-down→H2-up bridge in the router. Verified live: an H1-downstream version of 0079 showed `upstream_cx_http2_total=0`, 6 H1 conns. **Resolution:** 0079 uses an HTTP/2 (TLS+ALPN-h2) downstream listener on BOTH sides (the 0004 PKI/TLS shape) so both engage their H2 upstream pool. This is documented in the driver doc comment + README.

### FINDING 2 (subject pool DEFECT — BLOCKS the green run; needs a Task 4/5 fix): concurrent-burst over-dial (no dial coalescing)

With the H2-downstream listener, the pool IS engaged and `http2.streams_active` is correct on both sides. But the ceil-prong conn count DIVERGES cross-side under a CONCURRENT held-stream burst:

```
DIAG reference: cx_total=3 cx_http2_total=3 streams_active=6   ← EXACTLY ceil(6/2)=3 (SPEC D-H2-EXACT confirmed)
DIAG subject:   cx_total=6 cx_http2_total=6 streams_active=6   ← opens 6 conns (one per stream)
```

Root cause: `AcquireH2Stream`'s MISS path (`internal/cluster/h2pool.go:255-260`) reserves a permit (`tryAcquireConnSlot`) then dials OUT OF LOCK without coalescing concurrent demand. Under a 6-way simultaneous burst with `max_connections=16`, all 6 acquires MISS the stream-HIT scan (the pool is empty / conns not yet appended), all get a permit, all dial → **6 conns**. The reference coalesces concurrent demand onto establishing conns and opens exactly `ceil(K/C)`. The pool lacks **in-flight-dial coalescing** (pend-behind-an-establishing-conn, then ride its spare slots). The Task-4/5 unit tests (`TestAcquireH2Stream_ConnGrowthCeil`) only exercised SEQUENTIAL acquires (a `for` loop), so this gap was never covered.

This is a real subject-side pool defect in the load-bearing concurrency primitive (Tasks 4-5), surfaced — correctly — by the Task-9 fixture. The fix (a per-endpoint dialing counter + pend-while-dialing + post-dial spare-slot multi-promote, preserving the permit + lbRelease conservation invariants and a NEW `-race` concurrent-burst unit matrix) is a Task-4/5-scale production change beyond "author the 0079 fixture" and is deferred to the controller's decision. The overflow prong (`c_h2of`, max_connections=1) is UNAFFECTED by this (only one dial can ever happen) — only the multi-conn ceil prong needs the coalescing fix.

### Status: fixture authored + correct; gofmt/build/lint/driver-unit GREEN; the cross-side run is RED pending the FINDING-2 pool fix.

```
$ gofmt -l test/fixtures/0079-h2-multiplex-pool/ test/helpers/h2.go   → (clean)
$ go build ./test/...                                                  → (clean)
$ go test ./test/fixtures/0079-h2-multiplex-pool/... ./test/helpers/... -count=1  → ok
$ golangci-lint run ./test/fixtures/0079-h2-multiplex-pool/... ./test/helpers/... → (clean)
$ go test ./test/differential/ -run 'TestDifferential/0079' -count=1
  FAIL: subject ceil prong opens 6 conns, want ceil(6/2)=3 (FINDING 2 — concurrent-burst over-dial)
```

## Task 9.5 — connect-time dial coalescing (the FINDING-2 fix) — GREEN

**The defect (Task-9 FINDING 2):** `AcquireH2Stream`'s MISS path reserved a permit and dialed OUT OF LOCK with NO in-flight-dial coalescing. K concurrent acquires (permits available) all missed the stream-HIT scan simultaneously (no conn pooled yet) and each dialed its own conn → K conns. The reference coalesces concurrent demand onto establishing conns → exactly `ceil(K/C)`. Confirmed live by 0079: reference `upstream_cx_total=3`, subject `=6`.

**The design — a CONNECTING tier between stream-HIT and dial** (`internal/cluster/h2pool.go`):

- New `connectingH2Conn{ready chan struct{}, cc *h2.ClientConn, err error, promised int64}` + a per-endpoint `Cluster.h2Connecting map[string][]*connectingH2Conn` (guarded SOLELY by `h2PoolMu`).
- `AcquireH2Stream` MISS path is now three-tiered:
  1. **stream-HIT** (unchanged): ride a live conn with `inFlight < C`.
  2. **connecting-HIT (NEW):** scan `h2Connecting[addr]` for `promised < C`; `promised++` + `streams_active++` (commit our slot), fire our OWN `lbRelease` immediately (we ride the initiator's conn — like a stream-HIT, NO permit, NO dial), then WAIT on `ready`.
  3. **dial (INITIATOR):** only when no live AND no connecting spare. `tryAcquireConnSlot()` → register a connecting entry `promised=1` + `streams_active++` (our slot) → dial OUT OF LOCK → on success CONVERT the entry into `pooledH2Conn{cc, inFlight: promised}` (promised = initiator + everyone who attached while connecting), close `ready`, promote; on failure remove the entry, `streams_active -= promised`, set `err`, close `ready` (attachers surface the SAME error), promote.
  4. **PEND** (unchanged): a dial-grant becomes a connecting initiator.
- The PEND→dial-grant branch also creates a connecting entry, so a woken dial-grant coalesces concurrent attachers too.

**streams_active accounting (single owner per slot):** the COMMITTER increments (`initiator` at registration; each `attacher` at attach); the DE-committer decrements its OWN slot (dial-FAILURE conversion decrements `promised` slots once for everyone; ctx-cancel-while-connecting does `promised-- + streams_active--`; ctx-cancel-racing-ready rides-then-releases or treats failure as already-freed). On dial SUCCESS no further `streams_active` change — `inFlight = promised` carries the already-counted slots; each `release()` later decrements one.

**Conservation re-trace (both #1 properties; doc blocks in h2pool.go updated):**
- **Permit:** a connecting entry holds EXACTLY ONE permit (the initiator's). Attachers consume NO permit (ride the initiator's conn). On dial SUCCESS the permit transfers to the conn's `connWithGauge` dec (freed at Close). On dial FAILURE it is ALREADY released by `dialPooledH2To` (its contract) — `dialAndPool` never `releaseConnSlot`s. Attacher cancel/failure: no permit to release.
- **LB-release:** an ATTACHER fires its own `lbRelease` IMMEDIATELY at commit (like a stream-HIT) — covers its dial-success ride, dial-failure error, and cancel exits. The INITIATOR transfers `lbRelease` to the conn (success) or `dialPooledH2To` fires it (failure). Exactly one per acquire on every path.

**Attacher-on-dial-FAILURE:** surfaces the SAME dial error (symmetric with the initiator + the pre-9.5 MISS-dial path; the router owns retry) — NO in-pool retry storm. The defensive vanished-conn (`attachAfterReady` pc==nil) returns the retryable `errH2GrantRaced`.

**Tests (`internal/cluster/h2pool_coalesce_test.go`, `h2pool_overflow_integ_test.go`):**
- A gated-h2c listener parks each dial's handshake so K concurrent acquires deterministically overlap the connecting window.
- `TestAcquireH2Stream_ConcurrentBurstCeil_{6_2,5_2,7_4,8_1,4_4}`: EXACTLY `ceil(K/C)` conns + `streams_active==K` + `upstream_cx_http2_total==ceil(K/C)` + dials-started==`ceil(K/C)`. FAILS against pre-9.5 (6 conns for 6/2).
- `TestAcquireH2Stream_ConcurrentBurst_DialFailureWithAttachers`: K=8/C=16 all coalesce onto ONE failing dial → every acquirer surfaces the error cleanly; `streams_active==0`, `activeConns==0`, LB active==0 (zero leak).
- `TestAcquireH2Stream_ConcurrentBurst_CtxCancelWhileAttaching`: initiator pinned to Background; half the attachers canceled while waiting on `ready` → survivors ride, canceled release their slot; `streams_active==0`, `activeConns==1`, no leak.
- `TestAcquireH2Stream_OverflowProng_WokenPendingRoundTrips`: real RoundTrips through the C=1/maxConns=1/maxPending=1 pend→wake→ride-same-conn path (proved the pool mechanics; the 0079 overflow EOF was a FIXTURE drain-timing issue, not a pool bug).
- Race tests stable at `-race -count=20`.

**0079 fixture drain fix (`test/fixtures/0079-h2-multiplex-pool/driver/driver.go`):** the overflow prong's step-5 drain used ONE re-armable `/__release`, which freed only the held filler — the woken-pending request is sent upstream AFTER the gate re-armed and re-blocks on the fresh gate. Changed step 5 to a RE-ARM LOOP (release repeatedly until both requests return). A sticky release is not usable (the backend is SHARED with the not-yet-run side). This is a drain-timing correctness fix exposed once the ceil prong started passing; the pool mechanics were already correct (proven by the overflow integ test).

**Verification:**
```
$ go test ./internal/cluster/ -race -count=1                                   → ok
$ go test ./internal/cluster/ -run 'ConcurrentBurst|OverflowProng' -race -count=20 → ok (stable)
$ go test ./internal/filter/http/router/... ./internal/filter/hcm/... -race -count=1 → ok
$ go test ./test/differential/ -run 'TestDifferential/0079' -count=1            → ok (2.4s; both prongs, both sides)
$ go test ./test/differential/ -run 'TestDifferential/0004' -count=1            → ok
$ gofmt -l internal/ test/ && golangci-lint run ./internal/cluster/...          → (clean)
```

### Status: GREEN. FINDING-2 resolved; 0079 cross-side EXACT now passes.

## Task 10 — Deliberate-break proofs + 20/20 flake gate + `-race` (0079)

**Discipline:** every break-run and restore-run uses `-count=1` (no stale cache).
After each restore: `git restore internal/cluster/h2pool.go` + `git status` confirmed clean.
All breaks executed from: `/home/esa/git/envoy-go/.worktrees/phase-43.2a-impl`

### Baseline GREEN (pre-break)

```
$ go test ./test/differential/ -run 'TestDifferential/0079' -count=1
ok  github.com/esalaine/envoy-go/test/differential  2.583s
```

---

### Break (a) — conn growth / coalescing (`findConnectingHitLocked` neutered)

**Break:** Added `return nil` at the top of `findConnectingHitLocked` in
`internal/cluster/h2pool.go` so the connecting-tier scan always returns nil.
Without coalescing, all K=6 concurrent acquires miss both stream-HIT and
connecting-HIT, reserve 6 permits, and each dial their own conn → 6 conns
instead of `ceil(6/2)=3`.

**FAIL message (exact assertion line):**
```
runner_test.go:1257: subject: ceil prong: subject: stats did not converge to
  map[cluster.c_h2mp.http2.streams_active:6 cluster.c_h2mp.upstream_cx_http2_total:3
  cluster.c_h2mp.upstream_cx_total:3] within 15s
  (last seen map[cluster.c_h2mp.http2.streams_active:6
  cluster.c_h2mp.upstream_cx_http2_total:6 cluster.c_h2mp.upstream_cx_total:6])
  (the 6 held streams did not converge to ceil(6/2)=3 conns + 6 active streams —
  is the LOCAL cap C=2 driving multi-conn growth? is the backend holding all streams?)
```

Subject opened 6 conns, wanted 3. Assertion is LIVE.

**Restore:** `git restore internal/cluster/h2pool.go` → `git status` clean.
**Restore re-run:** `ok  github.com/esalaine/envoy-go/test/differential  2.467s`

---

### Break (b) — pending-queue overflow bound neutered

**Break:** Changed the overflow check in `AcquireH2Stream` path (D) from
`int64(len(c.h2Waiters[addr])) >= maxPending` to `false && ...` so the bound
never fires. With c_h2of (C=1, max_connections=1, max_pending_requests=1):
the 3rd request is queued instead of returning `errConnPoolOverflow`, so the
overflow prong never gets its 503.

**FAIL message (exact assertion line):**
```
runner_test.go:1257: subject: overflow oversub: transport error: RoundTrip: unexpected EOF
  (should be a 503 local reply, not a transport failure)
```

The 3rd request was queued, blocked the subject for 90s until the test timer fired,
then got an EOF on subject shutdown — NOT the expected immediate 503. Assertion is LIVE.

**Restore:** `git restore internal/cluster/h2pool.go` → `git status` clean.
**Restore re-run:** `ok  github.com/esalaine/envoy-go/test/differential  2.484s`

---

### Break (c) — `h2StreamsActiveDec()` in `makeRelease` commented out

**Break:** Commented out `c.h2StreamsActiveDec()` in `makeRelease` in
`internal/cluster/h2pool.go`. The `http2.streams_active` gauge is never
decremented on release → stays at 6 after the K=6 held streams complete.
The drain-poll (Step 3) waits 15s for `http2.streams_active==0` and times out.

**FAIL message (exact assertion line):**
```
runner_test.go:1257: subject: ceil prong did not drain: subject: stats did not converge to
  map[cluster.c_h2mp.http2.streams_active:0] within 15s
  (last seen map[cluster.c_h2mp.http2.streams_active:6])
  (http2.streams_active should return to 0 after release)
```

Gauge stuck at 6. Assertion is LIVE.

**Restore:** `git restore internal/cluster/h2pool.go` → `git status` clean.
**Restore re-run:** `ok  github.com/esalaine/envoy-go/test/differential  2.309s` (×2 confirms)

---

### Flake gate — 20 consecutive runs

Two 20-run batches executed (`-count=1`):

**Batch 1 (runs 1–20):** 19/20 PASS. Run 5 printed bare `FAIL` (no assertion
message, no runner_test.go line) — the `reference_differential_fullsuite_startup_flake`
transient startup-race pattern. Isolated re-run of the same fixture immediately
after: PASS. Two additional standalone re-runs: PASS both. Diagnosis: transient
`subject ready: EOF` — subprocess startup race under system load, NOT an
assertion mismatch.

**Batch 2 (runs 1–20):** 20/20 PASS.

```
Run 1–20: ok  github.com/esalaine/envoy-go/test/differential  ~2.3–2.5s each
```

Flake gate: PASS (1 transient startup flake confirmed transient; assertions stable).

---

### `-race` run

```
$ go test ./test/differential/ -run 'TestDifferential/0079' -race -count=1 2>&1 | tail -5
ok  github.com/esalaine/envoy-go/test/differential  3.641s
```

Race-clean.

---

### Working-tree clean confirmation

```
$ git status
On branch phase-43.2a-impl
nothing to commit, working tree clean
```

Only PROGRESS.md change in this commit (verified via `git diff HEAD~1 HEAD --stat` after commit).

## Task 11 — D-H2-CLOSEDSTREAMS (M-12: unbounded per-stream retention audit)

**Decision: 2a — NO unbounded per-stream retention. Docs-only commit.**

### Background

M-12 was a phase-05.1 REVIEW concern: an H2 codec may keep a `closedStreams`
(recently-closed stream IDs) map for late-frame handling that grows unbounded
under a long-lived conn. Pre-43.2a, every `ClientConn` served exactly ONE
`RoundTrip` then was `Close()`d (ADR-0056 per-request-fresh shape), so no
long-lived per-stream accumulation was possible. Post-43.2a, a pooled
`*h2.ClientConn` serves MANY sequential and concurrent streams over its
lifetime, making any unreleased per-stream structure a genuine memory leak.

### Investigation findings

#### 1. `cc.streams` — the primary per-stream registry (`client.go:75`, `sync.Map`)

`RoundTrip` (`client.go:479-543`) does:

```go
id := atomic.AddUint32(&cc.nextStreamID, 2) - 1   // line 483
cs := newClientStream(id, cc)                       // line 484
cc.streams.Store(id, cs)                            // line 485
defer cc.streams.Delete(id)                         // line 486
```

The `defer cc.streams.Delete(id)` fires on EVERY `RoundTrip` exit path —
normal completion, ctx-cancel, conn-close. The `cc.streams` map contains
exactly the in-flight streams (those whose `RoundTrip` has not yet returned).
When `RoundTrip` returns, the entry is removed unconditionally. **BOUNDED:
at most `MaxConcurrentStreams` entries, all Delete'd on completion.**

#### 2. No `closedStreams` / `recentlyClosed` map in `ClientConn`

Grepping the entire `h2/` package:

```
grep -rn "closedStream\|recentlyClosed\|map\[uint32\]" internal/filter/hcm/h2/ | grep -v _test.go
```

Results: `cc.streams sync.Map` in `client.go:75` and `s.closedStreams
map[uint32]struct{}` in `conn.go:51`. The `closedStreams` field exists ONLY on
`ServerConn` (`conn.go:51,91,302,411,588,644`). **`ClientConn` has NO
`closedStreams` or `recentlyClosed` map.** It does not need one: the RFC rule
that requires tracking recently-closed streams for late-frame disambiguation
(`DATA on closed stream` → stream error vs. connection error) is a
SERVER-side concern; a CLIENT already knows which streams it has outstanding
(they are in `cc.streams`). Frames for unknown stream IDs on the client path
simply return a connection error or are ignored (`client.go:338-344`,
`client.go:390-394`) — no tombstone map required.

#### 3. HPACK dynamic table (`client.go:69`, `hpack.go`)

`cc.hp` is a `*hpackState` (one per `ClientConn`). The HPACK encoder uses
`hpack.Encoder.SetMaxDynamicTableSize(maxTableSize)` (`hpack.go:27,31`) where
`maxTableSize` is seeded from `DefaultServerSettings.HeaderTableSize` at
construction (`client.go:159`) and updated on mid-life peer `SETTINGS_HEADER_TABLE_SIZE`
messages (`client.go:295-298`, `hpack.go:68`). The HPACK standard (`RFC 7541
§4`) mandates that the dynamic table is BOUNDED by this size limit — the
encoder evicts old entries when the table would overflow the cap. The cap is
applied by `x/net/http2/hpack.Encoder` (the underlying library) per the RFC.

**BOUNDED: the HPACK dynamic table is size-capped, not per-stream-growing.**
It is a conn-level structure that grows/evicts under a fixed ceiling; it does
NOT accumulate one entry per completed stream.

#### 4. `nextStreamID` — the monotonic stream-ID allocator (`client.go:74`, `atomic uint32`)

`atomic.AddUint32(&cc.nextStreamID, 2)` allocates 1, 3, 5, … indefinitely.
This is an `uint32` INTEGER COUNTER, not a map or slice — it consumes 4 bytes
total for the lifetime of the conn regardless of how many streams have been
served. **NOT a memory leak.** RFC 9113 §5.1.1 requires exactly this
monotonic allocation and provides the natural bound: a client MUST NOT initiate
a new stream after the 31-bit stream-ID space is exhausted (i.e., after
stream ID 2^31-1 ≈ 2.1B). A long-lived pool conn will eventually exhaust the
ID space. The correct handling is to retire (GOAWAY) the conn before it
wraps. **This is a 43.2b concern (lazy GOAWAY rotation, ADR-0254) — NOT a
43.2a memory leak.** The counter does not grow memory.

#### 5. `clientStream` lifecycle (`client.go:97-112`)

Each `clientStream` is allocated on the heap by `newClientStream` (`line 484`)
and stored in `cc.streams` for the duration of one `RoundTrip`. Its fields:
- `id uint32`, `cc *ClientConn` — fixed-size scalar references
- `sendW *window`, `recvW *window` — two small fixed-size structs (mutex + int32 + buffered channel)
- `respHeaders []hpack.HeaderField` — populated from the decoded HEADERS block; size bounded by the peer's header block (which is bounded by HPACK table size + frame-size limits)
- `respBody bytes.Buffer` — accumulates response DATA; freed when `RoundTrip` returns (the `cs` pointer goes out of scope after `defer Delete`)
- `doneCh chan error` — buffered-1 channel; closed by `finish`

ALL fields are released when `defer cc.streams.Delete(id)` removes the
`clientStream` from the map and the caller discards the `cs` local. The GC
reclaims the `clientStream` and all its fields. **No per-stream state
outlives `RoundTrip`.**

#### 6. `dispatchFrame` DATA case: unknown stream → connection error, NOT accumulation

`client.go:338-344`:

```go
case *http2.DataFrame:
    cs, ok := cc.lookupStream(fr.StreamID)
    if !ok {
        // Stream gone (we already finished it) — DATA after END_STREAM.
        return connError(ErrStreamClosed, "client: DATA on closed stream")
    }
```

When a DATA frame arrives for an already-finished stream (e.g., a stray DATA
after END_STREAM due to a race), the client promotes this to a connection
error and cancels `cc.ctx`. There is no "retain the stream ID in a
recently-closed tombstone so we can distinguish stream vs. connection error"
path — the client simply treats any unknown-stream DATA as a connection error.
This is a deliberate simplification (vs. the ServerConn's `closedStreams`
map) because the client controls which streams exist and does not need to
distinguish "was this a real stream I just finished" from "was this a
server-initiated stream I never opened". **No accumulation here.**

### Verdict

The `ClientConn` retains **NO unbounded per-stream state** under long-lived
pooled connections:

| Structure | Location | Bounded how |
|-----------|----------|-------------|
| `cc.streams` | `client.go:75,485-486` | `sync.Map.Delete(id)` in `defer` of `RoundTrip`; at most in-flight concurrency |
| `closedStreams` | — (ABSENT in `ClientConn`) | Does not exist; server-side-only concern |
| HPACK dynamic table | `client.go:69`, `hpack.go` | Size-capped by `SetMaxDynamicTableSize`; RFC 7541 §4 eviction |
| `nextStreamID` | `client.go:74` | Integer counter (4 bytes); not memory-proportional to stream count |
| `clientStream` | `client.go:97-112,484-486` | Fully released when `RoundTrip` returns (defer Delete + GC) |

**No 43.2a code change.** The 43.2b stream-ID exhaustion / lazy GOAWAY
rotation concern is noted and deferred to ADR-0254 (row 43.2b).

## Task 12 — completion bundle (ADR-0253 + BEHAVIOR_CONTRACT + STATE/ROADMAP + the full six-gate)

**Status: DONE — all 12 tasks complete; the six-gate GREEN; the full 81-dir differential GREEN.**

### As-built architecture (the ADR-0253 record)

A per-endpoint HTTP/2 multiplex `ClientConn` pool over the 43.1 permit substrate
(ADR-0252), **SUPERSEDING ADR-0056** (per-request-fresh H2 dial). Key mechanisms:

- `Cluster.h2Pool map[string][]*pooledH2Conn` (keyed by `ep.Addr()`, guarded by
  `h2PoolMu`) — a new conn opens only when every existing conn is saturated at the
  cluster's OWN `http2_protocol_options.max_concurrent_streams` (the LOCAL cap C —
  AMEND-H2-1; default `1<<30`, single-conn multiplex).
- A SEPARATE stream-aware pending queue (woken on stream-free via `h2PromoteLocked`
  OR conn-close; bounded by `max_pending_requests`; queue-full → `errConnPoolOverflow`
  → 503 + `upstream_rq_pending_overflow`); `tryAcquireConnSlot` (NON-blocking) keeps
  H2 OUT of the 43.1 `connPool.waiters`.
- **(Task 9.5) connect-time dial coalescing** — the `connectingH2Conn` "connecting"
  tier so K concurrent acquires ATTACH to establishing conns ⇒ exactly `ceil(K/C)`
  conns under burst (without it the pool over-dialed to K).
- **(Task 7.5) a single ctx-aware LB pick** threaded through the dial seam (fires
  exactly once per acquire) ⇒ conns keyed+attributed to the dialed endpoint +
  hash/subset/source_ip affinity honored (fixed an earlier ctx-blind double-pick).
- Router rewire: BOTH `router_h2.go` H2 sites off `DialH2`+`defer cc.Close()` onto
  `AcquireH2Stream`+`release()`+evict-on-error; the legacy `doH2` proven DEAD +
  REMOVED (completing the ADR-0056 supersession IN CODE).
- 2 new stats (useH2-gated): `upstream_cx_http2_total` + `http2.streams_active`; NO
  `upstream_cx_http2_active`. An H2 cluster emits **1185**, a non-H2 cluster stays at
  **1183**.
- Minimal liveness only (`Closed()`-skip + evict-on-RoundTrip-error); GOAWAY rotation
  + the `http2.*` reset/goaway stats + `min(local,peer)` peer-min → 43.2b (ADR-0254).
- **SPEC §8.1 topology correction:** the H2 pool engages ONLY on an H2 (TLS+ALPN-h2)
  DOWNSTREAM listener (no H1-down→H2-up bridge); `0079` uses an H2 downstream.

### The two mid-IMPL fixes (recorded above; carried into the ADR)

- **Task 7.5** — endpoint-selection correctness (the single ctx-aware pick replacing
  a ctx-blind double-pick; multi-endpoint keying + hash/subset affinity).
- **Task 9.5** — connect-time dial coalescing (the connecting tier; the FINDING-2
  concurrent-burst over-dial fix, surfaced by the `0079` fixture).

### Docs landed at Task 12

1. **DECISIONS.md** — ADR-0253 §Decision + §Consequences bodies (§Context promoted
   from SPEC §13, PROPOSED → ACCEPTED); ADR-0056 status header edited to
   `SUPERSEDED by ADR-0253`. DECISIONS tail ADR-0252 → ADR-0253 (next-free ADR-0254).
2. **BEHAVIOR_CONTRACT.md** — a new `### Cluster — HTTP/2 upstream multiplex connection
   pool` subsection + the stat-surface tracking block advanced 1183 → 1185.
3. **STATE.md** — the active-phase line replaced (`43.2a IMPL done`); the prior
   PLAN-done line demoted to prior active-phase.
4. **ROADMAP.md** — leg 43.2a marked done; row 43 STAYS `in-progress` (43.2b pending —
   ADR-0254; NOT flipped to done).
5. **PROGRESS.md** — this completion summary.

### The full six-gate (Task 12)

```
$ go build ./...                                  → (no output)  EXIT 0  CLEAN
$ go vet ./...                                    → (no output)  EXIT 0  CLEAN
$ gofmt -l internal/ test/                        → (no output)  EXIT 0  CLEAN
$ golangci-lint run ./...                         → (no output)          CLEAN
$ go test $(go list ./... | grep -v /test/differential) -count=1   → ALL ok (0 FAIL)
$ go test ./test/differential/ -count=1           → 81/81 GREEN (see note)
```

**Differential note (`reference_differential_fullsuite_startup_flake`).** The first
full-suite run surfaced 4 transient `subject ready: EOF` failures (`0038`, `0043`,
`0046`, `0048` — all UNRELATED to the 43.2a fixtures; the subprocess-startup-race
pattern, not assertion mismatches). Isolate-re-running those 4 + `0079`/`0078`/`0004`
together: ALL PASS (exit 0). A full-suite re-run confirms 81/81 GREEN. The flakes are
transient startup races under full-suite load, NOT regressions.

**Stat-count confirmation.** `TestRegisterClusterMetrics_H2Stats` (the Task-6
registration test) confirms: an H2 cluster gains exactly `upstream_cx_http2_total` +
`http2.streams_active` (1183 + 2 = **1185**) with `upstream_cx_http2_active` ABSENT;
a non-H2 cluster gains NEITHER (stays at **1183**).

### Final counts (as built at 43.2a IMPL)

| Counter | At PLAN | At 43.2a IMPL |
|---------|---------|---------------|
| Stat surface (H2 cluster) | 1183 | **1185** (+2) |
| Fixtures | 80 | **81** (`0079`) |
| Fuzzers (documented) | 42 | **42** (UNCHANGED; actual `^func Fuzz` 43 — `reference_fuzzer_count_docs_drift`) |
| BackendKind tail | 36 | **37** (`H2HoldResponder`) |
| DECISIONS tail | ADR-0252 | **ADR-0253** (next-free **ADR-0254**) |

ZERO new packages + ZERO new go.mod modules. **Row 43 STAYS `in-progress`** (flips
`done` only when ALL legs 43.1 + 43.2a + 43.2b land — ADR-0106 +
`reference_roadmap_split_phase_row_done`). **NEXT → the 43.2b leg** (GOAWAY rotation +
the `http2.*` reset/goaway stats + peer-min; ADR-0254); at the 43.2b IMPL row 43 flips
`done` + the Upstream-robustness family CLOSES.
