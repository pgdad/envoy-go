# Phase 36.1 — `ring_hash` load balancer — IMPL progress ledger

Worktree: `/home/esa/git/envoy-go/.worktrees/phase-36.1-impl` (branch `phase-36.1-impl`).
IMPL session of the 10-task TDD plan (`PLAN.md`). Commits are LOCAL-ONLY; the
controller squash-merges + pushes at stage-close (`feedback_subagents_no_push` /
`feedback_push_to_origin`).

## Task table

| # | Task | Status |
|---|------|--------|
| 1 | Baselines/anchors gate + PROGRESS.md | done |
| 2 | pure-Go `xxHash64` + `murmurHash2` (`hash.go`) | done |
| 3 | seam EXTENSION (ctx-carried hash key; ADR-0235) | done |
| 4 | `ringHashLB` Ketama ring (`ringhash.go`) | done |
| 5 | manager acceptance + gate + retarget + gauges | done |
| 6 | tcp_proxy source_ip hash plane | done |
| 7 | `0061-lb-ring-hash` differential fixture | done |
| 8 | deliberate-break liveness + flake check | done |
| 9 | full differential re-verify + race + conformance | done |
| 10 | completion bundle (ADR-0052 atomic landing) | done |

## Step 1 — count anchors (re-confirmed against the IMPL-session tip)

Re-confirmed BEFORE touching any code (the established first-task discipline). Each
row records the EXACT recipe used + its output + whether it matched the plan's
expected value.

| Anchor | Recipe (run from the worktree root) | Output | Expected | Match |
|--------|-------------------------------------|--------|----------|-------|
| fixtures | `ls -d test/fixtures/[0-9]* \| wc -l` | `62` | 62 | YES |
| fixtures tail | `ls -d test/fixtures/[0-9]* \| tail -1` | `test/fixtures/0060-lb-random` | `0060-lb-random` | YES |
| fuzzers | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` | `42` | 42 | YES |
| stat surface | BEHAVIOR_CONTRACT doc count — `grep -n "1116" docs/envoy-go/BEHAVIOR_CONTRACT.md` (line 466: "Phase 33 extension — 1091 → 1116 internal names") | `1116` | 1116 | YES |
| BackendKind tail | `grep -n "BackendKind = " test/differential/fixture/fixture.go \| tail -1` | `TCPThriftResponder BackendKind = 33` (line 562) | 33 | YES |
| DECISIONS tail | `grep "^## ADR-0" docs/envoy-go/DECISIONS.md \| tail -1` | `ADR-0234` (next-free ADR-0235) | ADR-0234 | YES |
| ADR count | `grep -c "^## ADR-0" docs/envoy-go/DECISIONS.md` | `233` | informational | — |
| `go build ./...` | `go build ./... && echo BUILD_OK` | `BUILD_OK` | green | YES |

### Recipe notes (canonical-recipe discovery)

- **fuzzers (42):** the canonical recipe carried from phase-34/35 PROGRESS.md —
  `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`.
  Counts ONLY `fuzz_test.go` files (excludes seed/corpus helper funcs; does NOT
  over-count via a hand-rolled `grep "func Fuzz"`). No new fuzzer expected in
  phase 36.1 (RING_HASH: NO wire decode — the hash is over a request-derived key,
  not a decoded protocol frame). Tail stays at the 42nd `FuzzThriftDecode`.
- **stat surface (1116):** there is NO programmatic test asserting the number. The
  canonical source-of-truth is the BEHAVIOR_CONTRACT doc count at line 466
  (`grep -n "1116" docs/envoy-go/BEHAVIOR_CONTRACT.md` → line 466: "Phase 33
  extension — 1091 → 1116 internal names"). Phase 36.1 adds the **3
  `ring_hash_lb.*` gauges** (`size`, `min_hashes_per_host`, `max_hashes_per_host`)
  per the SPEC — but these live under the existing `cluster.<name>.*` roster and
  whether/how they move the 1116 doc count is settled at Task 10 (the
  completion-bundle BEHAVIOR_CONTRACT delta). At THIS Task-1 tip the surface is
  1116.
- **DECISIONS tail (ADR-0234):** `grep -c "^## ADR-0"` yields 233 headings
  (ADR-0002..ADR-0234; ADR-0000/0001 omitted from the running count). **ADR-0235
  (seam EXTENSION) + ADR-0236 §Context are DRAFTS in SPEC §13 — NOT yet in
  DECISIONS.md at this IMPL tip.** They land at Task 3 (ADR-0235 body) and Task 10
  (ADR-0236 body) per ADR-0044 in-place authoring.

## Step 2 — as-built line anchors (re-confirmed against IMPL-session tip)

All grepped from the worktree root. ACTUAL line numbers recorded.

| Symbol | File | Line | Notes |
|--------|------|------|-------|
| `Pick() (Endpoint, func(), error)` (interface) | `internal/cluster/loadbalancer.go` | 16 | the `loadBalancer` interface method — the seam Task 3 EXTENDS (ctx-carry) |
| `func (rr *roundRobin) Pick` | `internal/cluster/loadbalancer.go` | 34 | incumbent `Pick` impl #1 |
| `noopRelease = func` | `internal/cluster/loadbalancer.go` | 21 | shared release `ringHashLB` returns (non-keyed release) |
| `func (lr *leastRequest) Pick` | `internal/cluster/leastrequest.go` | 81 | incumbent `Pick` impl #2 |
| `func (r *randomLB) Pick` | `internal/cluster/random.go` | 37 | incumbent `Pick` impl #3 — `ringHashLB.Pick` becomes #4 |
| `func newPCGRNG` | `internal/cluster/leastrequest.go` | 63 | REUSED verbatim by `newRingHash` (D-S36-2) |
| `func (c *Cluster) PickEndpoint` | `internal/cluster/cluster.go` | 180 | one of the 3 `c.lb.Pick()` funnels |
| `c.lb.Pick()` call sites | `internal/cluster/cluster.go` | 181, 217, 270 | the THREE `Pick` call sites (PickEndpoint + the two ADR-0232 OPTION-C direct picks); all get the ctx-carry threading at Task 3 |
| `func registerClusterMetrics` | `internal/cluster/manager.go` | 99 | runs AFTER `buildCluster` → `c.lb` set; the gauge type-assert hook (D-S36-6) |
| `switch c.GetLbPolicy()` | `internal/cluster/manager.go` | 235 | the LB-policy switch to extend with `Cluster_RING_HASH` |
| `unsupported lb_policy` | `internal/cluster/manager.go` | 257 | the reject text (the ONE production string) |
| `func parseLeastRequestLbConfig` | `internal/cluster/manager.go` | 295 | the config-parse sibling `parseRingHashLbConfig` mirrors |
| `func TestManager_Error_UnsupportedLBPolicy` | `internal/cluster/manager_test.go` | 320 | the doubly-hit retarget (`Cluster_RING_HASH` → next-unsupported, e.g. `Cluster_MAGLEV`) |
| `type Filter struct` (tcpproxy) | `internal/filter/tcpproxy/filter.go` | 27 | the tcp_proxy filter — Task 6 source_ip hash plane |
| `func NewFilter` (tcpproxy) | `internal/filter/tcpproxy/filter.go` | 49 | tcp_proxy ctor |
| `func (f *Filter) Handle` (tcpproxy) | `internal/filter/tcpproxy/filter.go` | 101 | the per-conn handler that derives source_ip → ctx hash key |
| `eff.Dial(ctx)` (tcpproxy) | `internal/filter/tcpproxy/filter.go` | 127 | the funnel that threads the ctx-carried hash key into the cluster |

NOTE on the switch grep: the PLAN suggested the pattern might differ; `grep -n
"switch c.GetLbPolicy()"` matched directly at `manager.go:235` (no adaptation
needed).

### seqRNG / eps pre-check (load-bearing for the unit tests)

```
grep -n "func seqRNG\|func eps" internal/cluster/*_test.go
```
Result:
```
internal/cluster/leastrequest_test.go:10:func seqRNG(vals ...uint64) func() uint64 {
internal/cluster/leastrequest_test.go:19:func eps(n int) []Endpoint {
```

**Both `seqRNG` and `eps` already exist in `internal/cluster/leastrequest_test.go`
(lines 10 and 19).** The `ringhash_test.go` / `hash_test.go` unit tests MUST REUSE
them (same `package cluster`), NOT redeclare them (would be a compile error).

## Step 3 — reject-text blast radius (AMEND-RH5 / the byte-stable-reject discipline)

Expected: production string ONLY in `manager.go`; ZERO fixture hits; the doc hit in
`BEHAVIOR_CONTRACT.md`. The "3 sites" = the manager.go production string + the
manager_test.go pin + the BEHAVIOR_CONTRACT.md doc line.

| Site | File | Notes |
|------|------|-------|
| production string | `internal/cluster/manager.go` (line 257) | `"… unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM)"` — the ONE production string; Task 5 extends `…, RANDOM` → `…, RANDOM, RING_HASH` |
| test assertion | `internal/cluster/manager_test.go` | the `TestManager_Error_UnsupportedLBPolicy` substring check — unit-level only |
| doc pin | `docs/envoy-go/BEHAVIOR_CONTRACT.md` | the departure/supported-set record — updated at Task 10 |

**ZERO fixture hits.** `grep -rln "ROUND_ROBIN, LEAST_REQUEST, RANDOM" test/` →
EMPTY — confirms NO boot-reject fixture pins the text (NO boot-reject dir for
36.1; the proof is the subject-side affinity fixture `0061`).

Verification commands + outputs:
```
grep -rln "unsupported lb_policy" internal/ cmd/
  → internal/cluster/manager.go   (ONLY)
grep -rln "ROUND_ROBIN, LEAST_REQUEST, RANDOM" test/
  → (empty)
grep -rln "ROUND_ROBIN, LEAST_REQUEST, RANDOM" docs/envoy-go/BEHAVIOR_CONTRACT.md
  → docs/envoy-go/BEHAVIOR_CONTRACT.md
```
Full-radius confirm (`unsupported lb_policy` OR the supported-set string across
`internal/ test/ BEHAVIOR_CONTRACT.md`) → exactly the THREE files above
(`manager.go`, `manager_test.go`, `BEHAVIOR_CONTRACT.md`).

## Step 4 — seam-consumer survey

```
grep -rln "\.lb\.Pick(\|\.Dial(ctx\|\.AcquireH1(ctx\|\.PickEndpoint()" internal/
```
Result (12 files). The key finding:
- **Only `internal/cluster/cluster.go` calls `c.lb.Pick()` directly** (the 3
  funnels at lines 181/217/270). Every other hit consumes the seam via
  `Dial(ctx)` / `AcquireH1(ctx)` / `PickEndpoint()` — they thread `ctx` and do NOT
  touch `Pick` directly. Consumers in the list: `internal/filter/tcpproxy/filter.go`,
  `internal/filter/network/thriftproxy/filter.go`, `internal/filter/network/redisproxy/filter.go`,
  `internal/filter/http/router/router.go`, `internal/httpclient/httpclient.go`,
  `internal/cluster/dial_h2.go`, plus `*_test.go` + `doc.go` references.
- **The producers that CHURN this phase:** `tcp_proxy` (Task 6 — derives the
  source_ip hash key into ctx). The HTTP router's `hash_policy` plane is **36.2,
  NOT this PLAN**. All other consumers thread `ctx` UNCHANGED (the ctx-carry is
  additive — a nil/absent key falls back to the existing pick).

```
grep -rln "cluster.WithHashKey\|hashKeyFrom" internal/
  → (empty)
```
**EMPTY pre-impl** (as expected) — the `WithHashKey` / `hashKeyFrom` ctx helpers do
NOT yet exist; they are AUTHORED at Task 3 (the ADR-0219 ctx-carry precedent).

## D-S36-1..7 resolutions (summarized from SPEC §12 / PLAN.md)

| Decision | Resolution |
|----------|-----------|
| D-S36-1: file placement | `ringHashLB` + ring build + lookup → NEW `internal/cluster/ringhash.go`; `xxHash64` + `murmurHash2` → NEW `internal/cluster/hash.go`; seam widening → `loadbalancer.go` (interface + 3 incumbent `Pick` sigs) + `cluster.go` (ctx-carry helpers + the `Dial`/`AcquireH1`/`PickEndpoint` threading); manager `case` + `parseRingHashLbConfig` + reject text + conditional gauge registration → `manager.go`; doubly-hit retarget → `manager_test.go`. (The `random.go`/`leastrequest.go` sibling-file precedent.) |
| D-S36-2: no-hash-fallback RNG message | `newRingHash` calls `newPCGRNG()` DIRECTLY, accepting the shared seed-error message (`"cluster: least_request: seed rng"`) — NO wrapper. Same rationale as D-S35-2: reachable only on a `crypto/rand` read failure (effectively unreachable on Linux `getrandom`). The misattributed `"least_request"` prefix is a recorded cosmetic note (PROGRESS + a BEHAVIOR_CONTRACT departure). YAGNI. |
| D-S36-3: hash impl shape + MURMUR_HASH_2 timing | Implement BOTH `xxHash64` (XXH64 reference port, seed 0) AND `murmurHash2(key, 0xc70f6907)` at 36.1; parse arm accepts both, ring build dispatches on `hash_function`; the differential fixtures use the `XX_HASH` default (cross-side-reproducible), `MURMUR_HASH_2` is unit-level. `xxHash64` verified against published XXH64 vectors AND the live reference's observed key→host mapping (D-RH4b). |
| D-S36-4: multi-source-IP driver + affinity attribution | **NO new driver primitive, NO new BackendKind.** The harness provides per-backend AGGREGATE accept counts to `AssertDistribution(refCounts, subjCounts []uint64)`. TCPEcho carries NO backend identifier → per-source-IP→backend attribution is NOT observable. **BUT the affinity invariant manifests in the aggregate counts:** binding exactly `burstPerIP` (16) conns to each of 4 source IPs forces **every subject per-backend count to be a multiple of 16** (one source IP → one key → one ring point → one backend, all-16-or-0). SUBJECT affinity assertion: `count[i] % 16 == 0` (EXACT, not a band); SPREAD: `>= 2` distinct nonzero backends; conservation: `sum == 64`. SCATTER break (key ignored → random) yields ~21/21/22 (not multiples of 16) → affinity leg FAILS; COLLAPSE-ring break (all → endpoints[0]) → `< 2` nonzero → spread leg FAILS. Driver binds via `net.Dialer{LocalAddr}`. Reference (Docker-NAT'd → one source IP → all 64 on ONE backend) asserted on `sum == 64` only (AMEND-RH8); its real proof is cross-side byte-equivalence + the cross-side `StatsAsserter`. **Coverage boundary:** the mod-16 invariant is necessary-and-overwhelmingly-discriminating against the random-scatter break (random multinomial(64, 1/3) landing all-multiples-of-16 is < 1%), not a tight proof against an adversarial even-split. |
| D-S36-5: ring-build formula | Equal-weight `entriesPerHost = ceil(minRingSize / N)` (the running-sum `scale` formula collapses to this under equal weights; the weighted carry is DEFERRED). Task 4 CONFIRMS the build matches the live reference for the default 3-host case: `size = 3 * ceil(1024/3) = 3 * 342 = 1026`, `min = max = 342`. |
| D-S36-6: gauge registration scope | Register the 3 `ring_hash_lb.*` gauges ONLY on the RING_HASH path (reference parity). Mechanism: `registerClusterMetrics` (runs AFTER `buildCluster`, so `c.lb` is set) type-asserts `if rh, ok := c.lb.(*ringHashLB); ok { … }` and registers + `Set`s the 3 gauges from `rh.size`/`rh.minPerHost`/`rh.maxPerHost`. STATIC (immutable ring) → registered + Set once; NO held `*stats.Gauge` field on `Cluster` (unlike `upstreamCxActive`). |
| D-S36-7: 36.2 | OUT OF SCOPE for this PLAN (the HTTP route `hash_policy` plane lands at 36.2). |

## ADR-0045 split-gate FINAL re-check

**Verdict: NO FURTHER SPLIT for 36.1.** This PLAN decomposes into **10 tasks**
(≤ ~25) over **~365 production LoC** (seam ~55 + `ringHashLB` ~205 + hash funcs
~45 + tcp plane ~60; ≤ ~1500) — both ADR-0045 axes hold with a wide margin. (36.2
— the ~140-LoC HTTP plane — re-checks the gate at its own PLAN; anticipated NO
split.)

## ADRs in flight (SPEC §13 DRAFTS → DECISIONS.md at IMPL)

- **ADR-0235** (the seam EXTENSION — ctx-carried hash key; the ADR-0232 PICK-INPUT
  half) — body authored at **Task 3** (next-free at this tip).
- **ADR-0236** (the `ring_hash` policy) — §Context body authored at **Task 10**
  (the completion bundle, ADR-0052 atomic landing).

## Anticipated end-of-IMPL count moves

| Counter | Before | After | Delta |
|---------|--------|-------|-------|
| fixtures | 62 | 63 | +1 (`0061-lb-ring-hash`) |
| DECISIONS tail | ADR-0234 | ADR-0236 | +2 (ADR-0235 seam + ADR-0236 policy) |
| stat surface | 1116 | (Task-10 settled) | +3 `ring_hash_lb.*` gauges (per SPEC; the doc-count delta finalized at the completion bundle) |
| fuzzers | 42 | 42 | 0 (no wire decode) |
| BackendKind tail | 33 | 33 | 0 (`TCPEcho` reused; NO new BackendKind — D-S36-4) |

## Task 1 — verdict

All count anchors MATCHED expected (fixtures 62 / fuzzers 42 / stat surface 1116 /
BackendKind tail 33 / DECISIONS tail ADR-0234; `go build ./...` BUILD_OK). All
as-built line anchors re-pinned (actual line numbers above). The reject-text blast
radius is exactly the 3 sites (manager.go production string + manager_test.go pin +
BEHAVIOR_CONTRACT.md doc), ZERO fixture hits. The seam-consumer survey confirms the
single `c.lb.Pick()` funnel in `cluster.go` + the 2 churning producers (tcp_proxy
this PLAN; the HTTP router at 36.2); `WithHashKey`/`hashKeyFrom` are EMPTY
pre-impl. **No drift — the tip did NOT move; the PLAN's anchors hold.** No
production code touched. **Task 1 COMPLETE.**

## Task 8 — `0061` deliberate-break liveness + flake check

Every `0061` assertion prong proven LIVE (the `0030` dead-assertion lesson — no
vacuous green). Each break applied ONE AT A TIME, run with `-count=1`
(`reference_differential_break_protocol_count1` — go-test caching defeated; each
confirmed `--- FAIL`, never a cached PASS), `-run 'TestDifferential/0061'`
(`reference_differential_run_selector` — NOT bare `0061`), then `git restore`d.
Production code (`ringhash.go`, `manager.go`) + the fixture `driver.go` end
byte-identical to their committed state (verified: `git status --short` clean
after each revert; only README + PROGRESS in the final commit).

| # | Break | File / fn | Prong it bit | Observed `--- FAIL` |
|---|-------|-----------|--------------|----------------------|
| (i) | scatter the key (`Pick` draws random, ignores `hashKey`) | `ringhash.go` `Pick` | SUBJECT **affinity** (`% 16 == 0`) | `subject affinity: backend[0]=14 not a multiple of 16` |
| (ii) | collapse the ring (`m = 0`, all picks → `ring[0]`) | `ringhash.go` `Pick` | SUBJECT **spread** (`>= 2` nonzero) | `subject spread: only 1 backend(s) nonzero, want >= 2` |
| (iii) | corrupt cross-equal stat want (`upstream_cx_total` 64 → 99) | `driver.go` `AssertStats` | **stats** cross-equal | `ref … upstream_cx_total = 64, want 99` + `subj … = 64, want 99` |
| (iv) | corrupt the size gauge (`rh.size` → `rh.size + 1`) | `manager.go` `registerClusterMetrics` | **`ring_hash_lb.*` gauge** | `cross-side mismatch … ring_hash_lb.size: ref=1026 subj=1027` + `subj … = 1027, want 1026` |

Each break bit EXACTLY its expected prong (none mis-wired). Notes: break (ii)
keeps affinity intact (64 % 16 == 0) so the SPREAD leg bites independently —
proving spread is non-vacuous; break (iv) tripped BOTH the cross-side
`ref != subj` mismatch AND the `subj != want` value check — both halves of the
cross-equal stats loop are live.

**Flake check:** `-count=20` → **20/20 PASS** (66 s; 20 fresh reference
containers). Affinity is DETERMINISTIC (fixed ring + fixed source-IP keys →
never flakes); spread (`>= 2`) is overwhelmingly stable (4 keys / 3 backends).
NO assertion loosened. Post-revert single `-count=1` run GREEN; `git status`
clean. **Task 8 COMPLETE.**

## Task 9 — full differential re-verify + race + conformance (the REAL six-gate)

VERIFICATION ONLY — NO production or test code touched; the sole commit is this
PROGRESS.md evidence update. Worktree branch confirmed `phase-36.1-impl` before
AND after. Fixture dir count: `ls -d test/fixtures/[0-9]* | wc -l` → **63**.

### Gate 1 — full differential suite (63 dirs) — byte-exact through the seam

`go test ./test/differential/ -count=1` → **`ok … 217.221s`, EXIT=0, ZERO subtest
failures** (clean full-suite run). A separate `-v` run confirmed the per-fixture
roster: **62 prior dirs byte-exact through the seam widening + the new
`case Cluster_RING_HASH` manager arm, and `0061-lb-ring-hash` GREEN** (`--- PASS:
TestDifferential/0061-lb-ring-hash (3.27s)`). 63 `=== RUN TestDifferential/<NNNN>`
subtests total.

**Harness-flake note (NOT a regression):** an earlier two full runs each tripped a
single `--- FAIL: TestDifferential/0025-http-adaptive-concurrency` with
`listener start: … bind 0.0.0.0:33318: bind: address already in use` →
`subj start: subject ready: EOF`. This is an ephemeral-port-bind race in the
differential harness (a prior fixture's port not yet released under back-to-back
container churn) on an **HTTP** fixture — entirely outside phase 36.1's
`internal/cluster` + `tcp_proxy` blast radius. Proven transient: `0025` PASSES in
isolation every time (`-run 'TestDifferential/0025-http-adaptive-concurrency'` →
`ok … 5.2s`, twice), and the third full-suite run came back fully green with NO
subtest failures. The seam extension keeps all 62 prior fixtures byte-exact.

### Gate 2 — `-race -short` across the repo

`go test -race -short ./...` → **EXIT=0, ZERO `DATA RACE`, NO FAIL lines**;
`test/fixtures/0061-lb-ring-hash/driver ok 1.007s`. The ring is immutable
post-build, `Pick` is read-only, and the no-hash-fallback rng is the REUSED
mutex-guarded `newPCGRNG` — no new shared mutable state.

### Gate 3 — build + vet + gofmt + lint + tidy

| Sub-gate | Recipe | Result |
|----------|--------|--------|
| build | `go build ./...` | `BUILD_OK` |
| vet | `go vet ./...` | `VET_OK` |
| gofmt | `gofmt -l internal/ test/fixtures/0061-lb-ring-hash/` | (empty — no drift) |
| lint | `golangci-lint run ./...` | exit 0, no findings |
| tidy | `go mod tidy -diff` | `TIDY_CLEAN` — **ZERO new go.mod dep** (`xxHash64`/`murmurHash2` hand-rolled) |

### Gate 4 — h2spec / proxy-wasm conformance — ASSERTED-UNAFFECTED (with rationale)

**Disposition: asserted-unaffected.** No Makefile and no fast self-contained
recipe exists; the h2spec gate (`test/conformance/h2spec/h2spec.go`,
`go test -run TestH2Spec ./test/conformance/h2spec/`) requires pulling the
`summerwind/h2spec` Docker image (heavy external setup), so the plan's
assert-with-rationale arm applies. The rationale is grounded in the verified
change scope (`git diff --name-only` vs the merge-base): phase 36.1 modifies ONLY
`internal/cluster/*` (LB seam + `ring_hash` policy + the 3 `ring_hash_lb.*` gauges
+ the hand-rolled hash funcs) and `internal/filter/tcpproxy/*` (the source_ip hash
plane), plus the one-line `0061` registration in `test/differential/runner_test.go`
and the `0061` fixture + docs. It touches **NO HTTP/2, h2spec-gate, or proxy-wasm
code path** — so the **h2spec 53/53** and **proxy-wasm 10/10** baselines
(`docs/envoy-go/CONFORMANCE_PINS.md`) are **unaffected by construction**.

### Task 9 — verdict

All six gates GREEN: 63-dir differential byte-exact (62 prior unchanged through the
seam extension + the new manager case + `0061`), `-race -short` clean (no race),
build/vet/gofmt/lint clean, `go mod tidy -diff` empty (zero new dep), h2spec 53/53
+ proxy-wasm 10/10 asserted-unaffected by change-scope construction. NO production
or test code touched. **Task 9 COMPLETE.**

## Task 10 — completion bundle (ADR-0052 atomic landing)

DOCUMENTATION ONLY — NO production or test code touched (the sole change set is the
5 doc files below). The atomic-landing bundle promotes the phase-36.1 deltas into
the canonical docs in ONE commit (ADR-0052).

### The five doc edits

1. **`BEHAVIOR_CONTRACT.md`** — (a) NEW `### Load balancer — ring_hash (RING_HASH)`
   subsection (mirroring the random subsection's depth): the `RingHashLbConfig`
   two-layer gate (min/max defaults 1024/8388608; PGV `<= 8388608` per-field +
   runtime min>max; XX_HASH/MURMUR_HASH_2), the Ketama ring semantics
   (`hash("addr:port_i")` build, first-point-`>=`-key lookup with wrap, the no-hash
   random fallback), the seam EXTENSION (the ctx-carried hash key; the TWO new
   exported helpers `cluster.WithHashKey` + `cluster.HashSourceIP`; the byte-stable
   `Cluster` surface), the tcp `source_ip` plane (HTTP plane = 36.2), per-side
   affinity (deterministic; the `count % 16 == 0` modular invariant) vs cross-side
   host-identity non-reproducibility (AMEND-RH8), the 3 mirrored `ring_hash_lb.*`
   gauges (cross-side-exact 1026/342/342), the D-S36-2 shared-RNG cosmetic note, the
   D-S36-4 modular-invariant coverage note, the healthy-set boundary, the 4
   deliberate-break records; the FIRSTS (FIRST consistent-hash policy; FIRST
   non-zero LB-stat delta). (b) UPDATED the reject-text DEPARTURE bullet — RING_HASH
   now ACCEPTED (the FOURTH policy; supported list `ROUND_ROBIN, LEAST_REQUEST,
   RANDOM, RING_HASH`); MAGLEV the LONE departure. (c) UPDATED the deferred-LB-family
   "Does not yet apply to" bullet — `5 remaining candidates {maglev, subset LB,
   locality-weighted LB, priority load balancing, panic thresholds}`. (d) UPDATED
   the stat-surface DOC count `1116 → 1119` (a new "Phase 36.1 extension" block in
   `## Stat-name mapping` + 3 new gauge rows in the cluster-stats table after
   `membership_total`; flagged as a DOC count, not a programmatic golden).

2. **`DECISIONS.md`** — full **ADR-0235** ("the LB hash-key seam extension (phase
   36.1)") + **ADR-0236** ("the `ring_hash` load-balancing policy (phase 36.1)")
   entries authored at the tail (heading + status line + §Context [promoted PROPOSED
   → ACCEPTED from SPEC §13] + §Decision [the as-built seam widening / ringHashLB /
   manager gate / gauges / tcp plane] + §Consequences), matching the ADR-0234 format
   exactly. DECISIONS tail advances **ADR-0234 → ADR-0236**; next-free **ADR-0237**.
   (NOTE: ADR-0235's body was NOT pre-authored at Task 3 as the early Task-1 ledger
   anticipated — both ADR bodies land here at Task 10.)

3. **`STATE.md`** — active-phase → `phase 36.1 (load-balancer-ring-hash) IMPL done`;
   lifecycle-state/next-skill → the 36.2 SPEC routing (the HTTP route hash_policy
   plane); counts: fixtures 62 → **63**, stat surface 1116 → **1119**, fuzzers **42**
   (unchanged), BackendKind tail **33** (unchanged), DECISIONS tail → **ADR-0236**
   (next-free ADR-0237); last-commit/last-updated refreshed; conformance line
   re-scoped to phase 36.1's blast radius.

4. **`ROADMAP.md`** — row 36's `36.1` leg flipped `in-progress → done` (2026-06-12)
   in the split column + the row narrative + the Load-balancing-family summary
   paragraph; the `36.2` leg STAYS in-progress (a FLAT family row — NO parent rollup
   per ADR-0106); the family STAYS OPEN (5 candidates remain after 36.1).

5. **`PROGRESS.md`** — all 10 tasks marked `done` in the task table; this summary.

### D-S36-1..7 resolutions (as built)

D-S36-1 file placement (`ringhash.go` + `hash.go` siblings; seam in
`loadbalancer.go`+`cluster.go`; manager case+gate+gauges in `manager.go`); D-S36-2
`newRingHash` reuses `newPCGRNG` verbatim (the shared `"least_request: seed rng"`
cosmetic message); D-S36-3 BOTH `xxHash64` (XXH64 seed 0) + `murmurHash2(0xc70f6907)`
implemented, fixtures use XX_HASH; D-S36-4 the `count % 16 == 0` modular-invariant
affinity proof (NO new driver primitive / BackendKind); D-S36-5 equal-weight
`ceil(minRingSize/N)` → 1026/342/342 for the default 3-host case; D-S36-6 the 3
gauges registered ONLY on the RING_HASH path via the `*ringHashLB` type-assert in
`registerClusterMetrics`; D-S36-7 36.2 out of scope.

### The four deliberate-break records (from Task 8, recorded here)

(i) scatter the key → subject affinity (`% 16`) FAILS; (ii) collapse the ring →
subject spread (`>= 2`) FAILS; (iii) corrupt a `StatsAsserter` want → stats
cross-equal FAILS; (iv) corrupt the `ring_hash_lb.size` gauge → BOTH the cross-side
mismatch AND the `subj != want` value check FAIL. Each `-count=1`, `git restore`d,
diff-clean. Flake check 20/20.

### Count deltas at phase-36.1 phase-done

| Counter | Before | After | Delta |
|---------|--------|-------|-------|
| fixtures | 62 | 63 | +1 (`0061-lb-ring-hash`) |
| stat surface (DOC count) | 1116 | 1119 | +3 `ring_hash_lb.*` gauges (FIRST non-zero LB-stat delta) |
| DECISIONS tail | ADR-0234 | ADR-0236 | +2 (ADR-0235 seam + ADR-0236 policy) |
| fuzzers | 42 | 42 | 0 (no wire decode) |
| BackendKind tail | 33 | 33 | 0 (`TCPEcho` reused) |
| exported symbols (cluster) | — | +2 | `cluster.WithHashKey` + `cluster.HashSourceIP` (additive) |

### Task-10 gate (docs-only → fast gates) + verdict

Recorded in the controller's stage-close evidence; the docs-only change cannot break
the build/tests (the six gates were GREEN at Task 9). **Task 10 COMPLETE — the
phase-36.1 IMPL is DONE.**
