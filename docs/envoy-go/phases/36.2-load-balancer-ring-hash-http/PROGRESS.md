# Phase 36.2 — HTTP route `hash_policy` producer (ring_hash) — IMPL progress ledger

Worktree: `/home/esa/git/envoy-go/.worktrees/phase-36.2-impl` (branch
`phase-36.2-load-balancer-ring-hash-http-impl`). IMPL session of the 8-task TDD
plan (`PLAN.md`). Commits are LOCAL-ONLY; the controller squash-merges + pushes at
stage-close (`feedback_subagents_no_push` / `feedback_push_to_origin`).

This leg lands the HTTP router route-level `hash_policy` PRODUCER plane (the 36.1
ring + the ADR-0235 ctx-carried-hash-key seam are already LANDED; 36.2 only STUFFS
a hash key into the ctx before the existing dial). The 36.1/36.2 by-plane split is
CONSUMED (parent SPEC §3.0; D-RH7).

## Task table

| # | Task | Status |
|---|------|--------|
| 1 | Baselines/anchors gate + PROGRESS.md | done |
| 2 | `cluster.HashHeaderValues` seed-chained `XXH64` fold (D-S362-1/4) | pending |
| 3 | `parseRouteHashPolicies` + DEPARTURE-reject + descriptor wiring (D-S362-2/5) | pending |
| 4 | `applyHashKey` fold + 4 dial-site ctx rebinds + uniform remote-addr carry (D-S362-3/4) | pending |
| 5 | `0062-lb-ring-hash-http` differential fixture (D-S362-6) | pending |
| 6 | deliberate-break liveness (`-count=1`) + flake check | pending |
| 7 | full differential re-verify + the six-gate (D-S362-7) | done |
| 8 | completion bundle (ADR-0052 atomic landing) | done |

## Step 1 — count anchors (re-confirmed against the IMPL-session tip)

Re-confirmed BEFORE touching any code (the established first-task discipline). Each
row records the EXACT recipe used + its output + whether it matched the plan's
expected value.

| Anchor | Recipe (run from the worktree root) | Output | Expected | Match |
|--------|-------------------------------------|--------|----------|-------|
| fixtures | `ls -d test/fixtures/[0-9]* \| wc -l` | `63` | 63 | YES |
| fixtures tail | `ls -d test/fixtures/[0-9]* \| tail -1` | `test/fixtures/0061-lb-ring-hash` | `0061-lb-ring-hash` | YES |
| fuzzers | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` | `42` | 42 | YES |
| stat surface | BEHAVIOR_CONTRACT doc count — `grep -n "→ 1119" docs/envoy-go/BEHAVIOR_CONTRACT.md` (line 4064: "Phase 36.1 extension — 1116 → 1119 internal names") | `1119` | 1119 | YES |
| BackendKind tail | `grep -n "BackendKind = " test/differential/fixture/fixture.go \| tail -1` | `TCPThriftResponder BackendKind = 33` (line 562) | 33 | YES |
| DECISIONS tail | `grep "^## ADR-" docs/envoy-go/DECISIONS.md \| tail -1` | `ADR-0236` (next-free ADR-0237) | ADR-0236 | YES |
| ADR count | `grep -c "^## ADR-" docs/envoy-go/DECISIONS.md` | `235` | informational | — |
| `go build ./...` | `go build ./... && echo BUILD_OK` | `BUILD_OK` | green | YES |

**All five count anchors MATCHED expected (fixtures 63 / fuzzers 42 / BackendKind
tail 33 / DECISIONS tail ADR-0236 / stat surface 1119).** Two RECIPE corrections
were needed (the COUNTS are correct; the PLAN's quoted recipe strings are stale) —
documented below.

### Recipe notes (canonical-recipe discovery + two PLAN-recipe corrections)

- **fuzzers (42) — PLAN recipe over-counts; canonical recipe used.** The PLAN's
  Task-1 recipe `grep -rho 'Fuzz[A-Za-z0-9_]*' --include=*_test.go | sort -u | wc -l`
  yields **54** — it sweeps EVERY `*_test.go` file (not just `fuzz_test.go`) and
  matches non-target identifiers (`Fuzz`, `FuzzSeed`, `FuzzBody`, `FuzzFilter`,
  `FuzzBytesOfLen`, `FuzzFixtures`, `FuzzFrameStream`, … — corpus/helper funcs).
  The **canonical recipe carried from phase-34/35/36.1 PROGRESS.md** is
  `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l` → **42**
  (counts ONLY actual `func Fuzz…` fuzz-target defs under `internal/**/fuzz_test.go`).
  Used the canonical recipe; the count (42) MATCHES expected.
- **fuzzers tail — PLAN annotation stale.** The PLAN says "tail `FuzzThriftDecode`";
  the ACTUAL sorted tail is now **`FuzzZookeeperResponseDecode`** (the zookeeper
  proxy added `FuzzZookeeperRequestDecode` + `FuzzZookeeperResponseDecode` since
  the 36.1 PROGRESS; thrift slipped from the tail). This is a benign tail-naming
  drift in the PLAN — the COUNT (42 unique targets) is unchanged and correct. No
  new fuzzer expected in 36.2 (the HTTP `hash_policy` producer decodes NO untrusted
  wire bytes — the hash is over a request-derived header value / source IP, not a
  decoded protocol frame; the fold digest is unit-tested).
- **DECISIONS tail (ADR-0236) — PLAN recipe uses the wrong heading depth.** The
  PLAN's `grep -c '^### ADR-'` (THREE hashes) yields **0** — DECISIONS.md headings
  are `## ADR-` (TWO hashes). The correct recipe is
  `grep "^## ADR-" docs/envoy-go/DECISIONS.md | tail -1` → **`ADR-0236`** (the
  36.1 policy ADR; next-free **ADR-0237**), and `grep -c "^## ADR-"` → **235**
  headings. Tail MATCHES expected. (36.2 may surface a small route-hash ADR or
  amend ADR-0236 — settled at the completion bundle Task 8; ADR-0237 is the
  candidate next-free slot.)
- **stat surface (1119) — DOC count, no programmatic golden.** There is NO test
  asserting the number. The canonical source-of-truth is the BEHAVIOR_CONTRACT
  doc count: `grep -n "1116 → 1119" docs/envoy-go/BEHAVIOR_CONTRACT.md` → line 4064
  ("Phase 36.1 extension — 1116 → 1119 internal names"). 36.2 adds **0** new stat
  names (it REUSES the 3 `cluster.<name>.ring_hash_lb.*` gauges + the existing
  cluster roster — confirmed in the same BEHAVIOR_CONTRACT block: "The 36.2 HTTP
  `hash_policy` plane adds 0 new stat names"). Surface STAYS 1119.

## Step 2 — as-built line anchors (re-confirmed against IMPL-session tip)

All grepped from the worktree root. ACTUAL line numbers recorded (line numbers
drift — Tasks 2–4 re-grep these; do NOT trust them blindly later in the session).

### 2a — LANDED seam + digest surface (36.1; the 36.2 base — read-only)

| Symbol | File | Line | Notes |
|--------|------|------|-------|
| `func WithHashKey(ctx, key uint64) context.Context` | `internal/cluster/cluster.go` | 178 | the ADR-0235 ctx-carry seam — 36.2 STUFFS into this |
| `func hashKeyFrom(ctx) (uint64, bool)` | `internal/cluster/cluster.go` | 182 | the seam read side (consumed by `Pick`) |
| `func HashSourceIP(addr string) uint64` | `internal/cluster/hash.go` | 129 | bare-client-IP `xxHash64` (36.1; reused by the source_ip arm of the 36.2 fold) |
| `func xxHash64(b []byte) uint64` | `internal/cluster/hash.go` | 38 | hand-rolled seed-0 XXH64 (UNEXPORTED; `HashHeaderValues` Task-2 wraps it) |
| `func murmurHash2(b []byte, seed uint64) uint64` | `internal/cluster/hash.go` | 136 | hand-rolled `0xc70f6907` (ring-internal; not on the 36.2 producer path) |

### 2b — HCM config-build parse boundary (Task 3 edits here)

| Symbol | File | Line | Notes |
|--------|------|------|-------|
| `routev3 "…/envoy/config/route/v3"` import | `internal/filter/hcm/config.go` | 7 | `routev3` already imported → the proto-read stays here (D-S362-2; router stays proto-free) |
| `func buildRouterAction(r *routev3.RouteAction, clusters *cluster.Manager)` | `internal/filter/hcm/config.go` | 535 | returns `&clusterRouteAction{cluster: c}` at line **550** — CONFIRMED; the `hash_policy` parse hangs off this |
| `type clusterRouteAction struct` | `internal/filter/hcm/actions.go` | 201 | the descriptor carrier — grows a `hashPolicies` field at Task 3 |
| `func (a *clusterRouteAction) asRouterAction()` | `internal/filter/hcm/actions.go` | 234 | H1 hand-off → `router.H1ClusterAction(a.cluster)` (line 235) |
| `func (a *clusterRouteAction) asRouterActionH2()` | `internal/filter/hcm/actions.go` | 247 | H2 hand-off → `router.H2ClusterAction(a.cluster)` (line 248) |

### 2c — router H1/H2 action surface (Task 4 edits here)

| Symbol | File | Line | Notes |
|--------|------|------|-------|
| `func H1ClusterAction(c *cluster.Cluster) Action` | `internal/filter/http/router/router.go` | 474 | H1 ctor; signature grows the hash-policy carry at Task 3/4 |
| `func doH1ClusterAction(ctx, a *routerAction, req)` | `internal/filter/http/router/router.go` | 504 | H1 driver — `applyHashKey(ctx, …)` folds the key here |
| `type routerAction struct` | `internal/filter/http/router/router.go` | 623 | H1 action carrier |
| `func (a *routerAction) do(ctx, req, bw)` | `internal/filter/http/router/router.go` | 641 | H1 chain entry |
| `func H2ClusterAction(c *cluster.Cluster) H2Action` | `internal/filter/http/router/router_h2.go` | 39 | H2 ctor |
| `func doH2ClusterAction(ctx, a *routerActionH2, req)` | `internal/filter/http/router/router_h2.go` | 57 | H2 driver — `applyHashKey` fold |
| `type routerActionH2 struct` | `internal/filter/http/router/router_h2.go` | 189 | H2 action carrier |
| `func (r *routerActionH2) doH2(ctx, req, w)` | `internal/filter/http/router/router_h2.go` | 214 | H2 chain entry |

### 2d — the FOUR dial sites (Task 4 ctx-rebind targets — load-bearing)

| Dial site | File | Line | Notes |
|-----------|------|------|-------|
| `a.cluster.AcquireH1(ctx)` | `internal/filter/http/router/router.go` | 509 | H1 pooled dial #1 |
| `a.cluster.Dial(ctx)` | `internal/filter/http/router/router.go` | 662 | H1 unpooled dial #2 |
| `c.cluster.DialH2(ctx)` | `internal/filter/http/router/router_h2.go` | 62 | H2 dial #3 (`doH2ClusterAction`; `IncUpstreamRqTotal` at line 60) |
| `r.cluster.DialH2(ctx)` | `internal/filter/http/router/router_h2.go` | 233 | H2 dial #4 (`doH2`; `IncUpstreamRqTotal` at line 231) |

Each dial site must receive the `applyHashKey`-rebound `ctx` so the LANDED
`hashKeyFrom(ctx)` read inside `Pick` sees the producer's key. `IncUpstreamRqTotal`
sites (router_h2.go:60, :231) bracket the H2 dials — confirm the ctx rebind precedes
the `DialH2` but the counter ordering is unchanged.

### 2e — the TWO downstream-remote-addr dispatch seams (Task 4; source_ip arm)

| Seam | File | Line | Notes |
|------|------|------|-------|
| `chain.SetDownstreamRemoteAddr(downstream.RemoteAddr())` | `internal/filter/hcm/connection.go` | 384 | H1 dispatch — `downstream net.Conn` in scope (`runConnection`, line 151) |
| `chain.SetDownstreamRemoteAddr(c.downstreamRemoteAddr)` | `internal/filter/hcm/h2dispatch.go` | 337 | H2 dispatch — `c.downstreamRemoteAddr` in scope |
| `rf.RunAction(ctx)` | `internal/filter/hcm/h2dispatch.go` | 480 | H2 action invoke (the ctx that reaches `doH2ClusterAction`) |
| `resp, picked, err := c.action(ctx, h2req)` | `internal/filter/hcm/h2dispatch.go` | 289 | H2 chain-dispatch action call |

D-S362-3 finding (as-built): `*http.Request.RemoteAddr` is NOT populated on the HCM
H1 path — only the chain remote addr is set (connection.go:384). So source_ip MUST
flow through a NEW `router.WithDownstreamRemoteAddr(ctx, addr)` ctx key (set at both
dispatch sites), NOT `req.RemoteAddr`. The header arm is LIVE on every `0062`
request; the source_ip arm is UNIT-tested only (the fixture keys on header).

### 2f — `H1ClusterAction` / `H2ClusterAction` constructor blast radius (Task 3)

`grep -n 'H1ClusterAction(\|H2ClusterAction(' -r internal/`:

| Call site | File | Line | Notes |
|-----------|------|------|-------|
| `router.H1ClusterAction(a.cluster)(ctx, req)` | `internal/filter/hcm/actions.go` | 212 | a direct-invoke call (inside an `asRouterAction`-adjacent helper) |
| `return router.H1ClusterAction(a.cluster)` | `internal/filter/hcm/actions.go` | 235 | `asRouterAction()` hand-off |
| `return router.H2ClusterAction(a.cluster)` | `internal/filter/hcm/actions.go` | 248 | `asRouterActionH2()` hand-off |
| `func H1ClusterAction` (def) | `internal/filter/http/router/router.go` | 474 | the def |
| `func H2ClusterAction` (def) | `internal/filter/http/router/router_h2.go` | 39 | the def |

**Blast radius is exactly 3 call sites (all in `hcm/actions.go`: 212, 235, 248) +
the 2 defs.** When Task 3 extends the `H1/H2ClusterAction` signature to carry the
parsed hash policies, all three `hcm/actions.go` call sites must thread the new
argument from `clusterRouteAction.hashPolicies`.

## Step 3 — D-resolution summary (D-S362-1..7 + ADR-0045)

One-line résumés (full text in `PLAN.md`):

- **D-S362-1 (header-hash exported surface) → RESOLVED:** one additive
  `cluster.HashHeaderValues(values []string) uint64` (seed-chained XXH64 fold,
  collapses to `xxHash64([]byte(value))` single-value); keeps `xxHash64`/`ipOnly`
  unexported; the ONE additive exported `cluster` symbol; `Cluster` surface stays
  byte-stable. (Task 2.)
- **D-S362-2 (parse placement) → RESOLVED (refined):** the proto-free descriptor
  `router.HashPolicy` lives in the `router` package; the proto-read + fail-fast
  reject (`parseRouteHashPolicies`) lives in `hcm/config.go` (where `routev3` is
  already imported) — preserves router's ZERO-proto property + keeps the fail-fast
  at the config-build boundary. (Task 3.)
- **D-S362-3 (per-codec header accessor + remote-addr access) → RESOLVED:** a
  `headerVal func(name)(string,bool)` closure per codec + a UNIFORM ctx-carried
  downstream remote addr (`router.WithDownstreamRemoteAddr`/`downstreamRemoteAddrFrom`)
  for BOTH codecs (because `*http.Request.RemoteAddr` is NOT populated on the H1
  path); set at the H1 dispatch (connection.go) + H2 dispatch (h2dispatch.go). (Task 4.)
- **D-S362-4 (multi-value header fold) → RESOLVED:** implement the FULL seed-chained
  XXH64 fold over byte-sorted values in `cluster.HashHeaderValues` (~6 LoC over
  single-value); the producer feeds single-value (H1 `Get`/H2 first-match), the
  digest SUPPORTS multi-value (unit-tested). (Tasks 2 + 4.)
- **D-S362-5 (empty-`header_name` reject wording) → RESOLVED:** pin the go-binding
  PGV form `value length must be at least 1 runes`; unit-level discretion (the
  `0062` fixture is an ACCEPT path; no cross-side fixture pins it; ADR-0080). (Task 3.)
- **D-S362-6 (`0062` affinity attribution) → RESOLVED:** reuse the 36.1
  aggregate-count modular invariant (K repeats per distinct `X-Hash` → each
  backend's per-value count is a K-multiple → affinity provable from aggregate
  counts); applicable to BOTH sides (the header key is NAT-transparent); NO new
  BackendKind. (Task 5.)
- **D-S362-7 (conformance gate) → RESOLVED:** the final task RE-RUNS
  h2spec/proxy-wasm where Docker is present, ELSE asserts-unaffected with the
  rationale "the producer only STUFFS a ctx key before the existing dial; with no
  `hash_policy` configured (every conformance config), `applyHashKey` returns
  `ctx` unchanged and the wire path is byte-identical." 36.2 TOUCHES the H1/H2
  router path (NOT zero-touch like 36.1) → the obligation is STRONGER; the real
  guard is the full 63-dir differential re-verify. (Task 7.)
- **ADR-0045 split-gate re-check → NO SPLIT.** Envelope ≈ 150 production LoC across
  8 tasks (1 additive `cluster` helper + 1 hcm parse + 1 router descriptor/fold/
  4-dial-site wiring + 1 fixture), all against the LANDED ADR-0235 seam. Well under
  the gate; the 36.1/36.2 by-plane split is already CONSUMED (parent SPEC §3.0).

## Confirmations (PLAN Step-2 assertions, all VERIFIED at this tip)

- `WithHashKey` / `HashSourceIP` / `xxHash64` / `murmurHash2` — all LANDED (§2a).
- `buildRouterAction` returns `&clusterRouteAction{cluster: c}` — config.go:550. ✓
- The FOUR dial sites present — `AcquireH1` (router.go:509), `Dial` (router.go:662),
  two `DialH2` (router_h2.go:62, :233). ✓ (§2d)
- The downstream-remote-addr seam at connection.go (H1 dispatch, :384) +
  h2dispatch.go (H2 dispatch, :337). ✓ (§2e)
- `H1/H2ClusterAction` blast radius = 3 call sites in `hcm/actions.go` + 2 defs. ✓ (§2f)
- `go build ./...` → `BUILD_OK` (green baseline). ✓

## Notes

- NO new fuzzer (no wire decode — fuzzers STAY 42), NO new BackendKind (the `0062`
  fixture reuses existing HTTP backends — tail STAYS 33), NO new stat names (36.2
  reuses the 3 `ring_hash_lb.*` gauges — surface STAYS 1119), ZERO new packages,
  ZERO new go.mod deps anticipated. The only count that MOVES is fixtures 63 → 64
  (the `0062-lb-ring-hash-http` dir at Task 5).
- ADR disposition (route-hash ADR vs ADR-0236 amend) is settled at the completion
  bundle (Task 8); next-free ADR slot is ADR-0237.

## Task 4 — as-built (applyHashKey fold + 4 dial-site ctx rebinds + remote-addr carry)

- `applyHashKey(ctx, hps, headerVal, remoteAddr) (ctx, key, has)` LANDED in
  `router.go` — folds `rotl64(prev,1)^new`, first contributor verbatim, nullopt
  policies skipped, `terminal` short-circuits once the accumulator is non-empty
  (Envoy `HashPolicyImpl::generateHash`, SPEC §3.2). Testability triple-return;
  dial sites use only ctx.
- **Multi-value header boundary (D-S362-4):** the H1 (`req.Header.Get`) and H2
  (`h2HeaderVal` hpack scan) producer shims feed a SINGLE header value — the
  producer plane is single-value. The full multi-value byte-sorted fold lives in
  `cluster.HashHeaderValues([]string)` (Task 2, unit-tested) and is reachable but
  not driven by the current single-value shims. A future multi-value producer
  simply collects all matching header values into the slice; the digest already
  supports it.
- 4 dial-site ctx rebinds (re-grepped at this tip): `doH1ClusterAction` before
  `AcquireH1` (`router.go`), `routerAction.do` before `Dial` (`router.go`),
  `doH2ClusterAction` before `DialH2` (`router_h2.go`), `routerActionH2.doH2`
  before `DialH2` (`router_h2.go`). Each reassigns the local `ctx` (`ctx,_,_ =`)
  that flows into the dial.
- Uniform remote-addr carry: `WithDownstreamRemoteAddr`/`downstreamRemoteAddrFrom`
  (router.go ctx-key); set on the LIVE dispatch ctx at connection.go (H1, inside
  the `downstream != nil` guard) + h2dispatch.go (H2, top of `WriteH2`, guarded
  `c.downstreamRemoteAddr != nil`, covering BOTH the no-match `c.action(ctx,...)`
  and matched `rf.RunAction(ctx)`). `*http.Request.RemoteAddr` is unpopulated on
  the HCM codec path (D-S362-3) → source_ip reads the ctx-carried addr.
- Byte-stable when no hash_policy configured: `applyHashKey` returns ctx unchanged
  → no `WithHashKey` → LB no-hash fallback.

## Task 6 — as-built (0062 deliberate-break liveness — 4 prongs + 20-run flake check)

Each break applied ONE AT A TIME, run with `-count=1`
(`reference_differential_break_protocol_count1`), the named prong observed to
`--- FAIL`, then `git restore`d. Selector `-run 'TestDifferential/0062-lb-ring-hash-http'`
(`reference_differential_run_selector`). Production tree UNCHANGED — only this
PROGRESS + the fixture README are committed. Revert via `git restore` ONLY
(`feedback_subagent_worktree_detach`); branch re-confirmed clean on-branch after
each restore.

| # | break (file · what changed)                                                  | prong proven                     | quoted `--- FAIL` line |
|---|-------------------------------------------------------------------------------|----------------------------------|------------------------|
| 1 | `internal/filter/http/router/router.go` — `applyHashKey` → `return ctx, 0, false` (key never contributes) | **affinity** (`cᵢ ≡ 0 mod 16`)   | `distribution: subject affinity: backend[0]=90 not a multiple of 16 (key scattered? an X-Hash value split across backends)` |
| 2 | `internal/cluster/ringhash.go` — `ringHashLB.Pick` → `return rh.endpoints[0]` (collapse to backend 0) | **spread** (`>= 2` nonzero)      | `distribution: subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)` |
| 3 | `internal/filter/http/router/router.go` — drop `a.cluster.IncUpstreamRqTotal()` in `doH1ClusterAction` | **`upstream_rq_total` cross-eq**  | `cross-side mismatch cluster.c_echo.upstream_rq_total: ref=256 subj=0` · `subj cluster.c_echo.upstream_rq_total = 0, want 256` |
| 4 | `test/fixtures/0062-lb-ring-hash-http/driver/driver.go` — expected `ring_hash_lb.size` 1026 → 1025 | **`ring_hash_lb.*` gauge cross-eq** | `ref cluster.c_echo.ring_hash_lb.size = 1026, want 1025` · `subj cluster.c_echo.ring_hash_lb.size = 1026, want 1025` |

All four bit the SPECIFIC named assertion. The subject runs H1 (`codec_type: HTTP1`)
so prongs 1/3 land on the `doH1ClusterAction` path (`router.go`). Command (each
prong): `go test ./test/differential/ -run 'TestDifferential/0062-lb-ring-hash-http' -count=1`.
The `--- FAIL` lines above are at the post-fix **N=16** workload (prongs 1 & 2 were
re-confirmed to BITE at N=16 in the follow-up fix below — affinity rose
`backend[0]=25` → `backend[0]=90`; the rq-break counts rose `64` → `256` = N*K).

After every restore: `git status` clean, `git diff --stat -- internal/ test/.../driver/`
EMPTY, branch `phase-36.2-load-balancer-ring-hash-http-impl`.

### Flake check — was 18/20 at N=4, NOW 30/30 PASS at N=16 (spread flake FIXED)

At the original **N=4**: `for i in $(seq 1 20); …` → **18/20 PASS**. The 2 failures
(run 14 `reference spread: only 1 backend(s) nonzero`; run 15 `subject spread: only
1 backend(s) nonzero`) were BOTH the **spread** prong; affinity + conservation +
all stats prongs held 20/20.

**Root cause (NOT a producer regression):** the Ketama ring is keyed off the
endpoint ADDRESS strings (`<addr>:<port>_i`), and the harness allocates the 3
backend ports DYNAMICALLY per run → the ring layout varies run-to-run. With only
**N=4** distinct `X-Hash` values over 3 backends, the four values occasionally all
land on ONE backend → spread collapses to 1 (`P(all N on 1 of 3) ≈ 3·(1/3)^4 ≈
3.7%` per side). This contradicted the "4 values overwhelmingly cover ≥ 2"
assumption (which presumed a fixed ring).

**Follow-up fix (this commit):** raise **N=4 → 16** distinct `X-Hash` values
(`hashValues` constant; K unchanged at 16). `totalReqs = hashValues * repeatPerVal`
is DERIVED, so it auto-updated 64 → 256 and the `upstream_cx_total` /
`upstream_rq_total` AssertStats wants track it (no hardcoded literal was present —
they already referenced `totalReqs`). The per-side collapse probability drops to
`3·(1/3)^16 ≈ 7e-8` — past the 5σ-equivalent flake-free margin
(`reference_differential_band_sigma_margin`, applied to a spread threshold rather
than a σ-band). Verified:

`pass=0; for i in $(seq 1 30); do go test ./test/differential/ -run 'TestDifferential/0062-lb-ring-hash-http' -count=1 && pass=$((pass+1)); done; echo $pass/30`
→ **30/30 PASS**.

The affinity (modular-invariant) and stats correctness proof of Tasks 2-4 was
unaffected throughout — only the fixture's spread robustness changed. `git diff
--stat -- internal/` is EMPTY (the fix touches only the `0062` fixture dir + this
PROGRESS doc).

## Task 7 — as-built (full 64-dir differential + six-gate + h2spec/proxy-wasm disposition — D-S362-7)

Verification gate for the WHOLE change. HEAD at start `9440e82` (7 commits: Task 1-6
+ the Task-6 N=16 fix). The branch stays LOCAL-ONLY; no push, no branch switch, no
amend (`feedback_subagents_no_push`).

### Step 1 — the LANDED-at-36.1 seam is byte-unchanged

`git diff master --stat -- internal/cluster/loadbalancer.go internal/cluster/ringhash.go internal/cluster/manager.go internal/filter/tcpproxy/filter.go`
→ **EMPTY** (no diff). The producer touched NONE of the 36.1 LB/ring/manager/tcpproxy seam.

`git diff master -- internal/cluster/cluster.go internal/cluster/hash.go`:
- `cluster.go` → **UNCHANGED vs master** (`WithHashKey` landed at 36.1; 36.2 only CONSUMES it; no `WithHashKey`/`Dial`-body change).
- `hash.go` → **ONLY the Task-2 additions**: the additive exported `HashHeaderValues` (seed-chained XXH64 fold over byte-sorted header values) + the behavior-neutral `xxHash64Seed` seed-refactor (`xxHash64` now delegates to `xxHash64Seed(b, 0)` — byte-for-byte identical at seed 0). No other production-file diff.

### Step 2 — full 64-dir differential (Docker)

`go test ./test/differential/ -count=1 -v` → **`--- PASS: TestDifferential (216.83s)`**, **64/64 subtests PASS, 0 SKIP, 0 FAIL** (`0000-tcp-echo` … `0062-lb-ring-hash-http`; `0062` green at 2.18s). All 64 fixture dirs have a registered driver/inputs blank-import in `runner_test.go` (no skips). The 63 prior dirs are byte-exact (the `applyHashKey` closures are behavior-neutral when no `hash_policy` is configured — every prior fixture). Plus the 4 responder-backend unit tests (`TestZK/Mongo/Kafka/ThriftResponderBackend`) PASS. A prior non-`-v` run independently confirmed `ok …/test/differential 226.792s` with 0 FAIL.

### Step 3 — `-race -short` + build/vet/gofmt/lint/tidy

| gate | command | result |
|------|---------|--------|
| race + short | `go test -race -short ./... -count=1` | **PASS** (exit 0; Docker differential skipped via `-short` as intended — it ran in Step 2) |
| build | `go build ./...` | **clean** (exit 0) |
| vet | `go vet ./...` | **clean** (exit 0) |
| gofmt | `gofmt -l .` | **clean** (no files listed) |
| lint | `golangci-lint run ./...` | **clean** (exit 0) |
| tidy | `go mod tidy -diff` | **EMPTY** (zero new dep) |

**Stale-test bug found + fixed (test-only — the one production-tree-adjacent deviation from "PROGRESS-only").** The FIRST `-race -short` run surfaced a RED test:
`--- FAIL: TestAssertDistribution_BothSideAffinity … reference conservation: sum 64 != 256`
in `test/fixtures/0062-lb-ring-hash-http/driver/driver_test.go`. Root cause: the Task-6
N=4→16 fix (commit `9440e82`) raised `totalReqs` 64→256 in `driver.go` but did NOT update
`driver_test.go`'s hand-rolled count slices, which still summed to 64. It was masked at
Task 6 by go-test result caching (`reference_differential_break_protocol_count1` — the exact
footgun); the `-count=1` six-gate here defeated the cache and exposed it. Worse, three
negative tests (`ScatterBitesAffinity_Subject`, `CollapseBitesSpread`, `Conservation`) were
VACUOUS under N=16 — their "good" reference side summed to 64, so the `conservation` leg
(`sum != 256`) fired FIRST and returned err before the INTENDED leg (affinity/spread) was
reached. **Fix:** rewrote the 6 unit fixtures to sum to **256** (multiples of 16, ≥2 nonzero
for the good side — e.g. `{128,64,64}`) so each negative test bites its intended leg. PROVEN
live via a throwaway `TestLegProof` (asserted each error string names the intended leg —
`reference/subject affinity`, `subject spread`, `subject conservation`, wrong-length — then
removed; dir clean). Production code (`internal/`, `driver.go`) UNCHANGED — the only edit is
`driver_test.go`. Post-fix `go test -race -short ./... -count=1` → **PASS** (exit 0), `0062`
driver green.

### Step 4 — h2spec / proxy-wasm disposition (D-S362-7) — RE-RAN BOTH (not asserted-unaffected)

36.2 TOUCHES the H1/H2 router path (`applyHashKey` stuffs a ctx key before the dial), so the
conformance gate needs evidence. BOTH harnesses are runnable here, so I RE-RAN them (stronger
than assert-unaffected):
- **h2spec** (`go test ./test/conformance/h2spec/ -count=1` — Docker, `h2spec` container vs an
  envoy-go h2c subject) → **53/53 tests passed, 0 failed** (`--- PASS: TestH2Spec (2.74s)`).
  The single `COMPRESSION_ERROR: HPACK decode failed` log line is an EXPECTED negative-path
  conformance case (4.3 Header Compression 3/3), not a failure.
- **proxy-wasm** (`go test ./test/conformance/proxy-wasm/ -count=1` — pure in-process) →
  **PASS**, **10 families all green** (exports, security, runtime, wasm_vm, bytecode_util,
  logging, stop_iteration, shared_data, pairs_util, endianness).

Both match the expected 53/53 + 10/10. (Rationale that ALSO holds, had they been impractical:
with no `hash_policy` configured — every conformance config — `applyHashKey` returns ctx
unchanged and the request/response wire path is byte-identical; the full 64-dir differential
in Step 2, which includes all H1/H2 fixtures, is the real guard.)

### Hygiene

After all runs: `git diff --stat -- internal/ test/` shows ONLY
`test/fixtures/0062-lb-ring-hash-http/driver/driver_test.go` (the test-only stale-fixture fix
above; zero `internal/` production diff). The throwaway `TestLegProof` and the Docker
containers left no artifacts. Committed LOCAL-ONLY on branch
`phase-36.2-load-balancer-ring-hash-http-impl`; no push, no switch, no amend.

## Task 8 — as-built (completion bundle — ADR-0052 atomic doc landing)

DOC-ONLY (no production code). Lands the whole completion bundle in ONE atomic commit
(ADR-0052): the BEHAVIOR_CONTRACT 36.2 HTTP-plane addendum + the full ADR-0237 + the
STATE/ROADMAP row-36.2-done advance + this final six-gate evidence. `git diff --stat --
internal/ test/` confirmed EMPTY before staging (the commit touches only `docs/`).

### Doc edits

- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — extended the `### Load balancer — ring_hash
  (RING_HASH)` subsection (created at 36.1) with a new `#### The HTTP route hash_policy
  producer plane (phase 36.2 — ADR-0237)` block (SPEC §9): the supported specifiers
  (`header` `xxHash64` / single-value + the multi-value seed-chained `XXH64` fold via
  `cluster.HashHeaderValues`; `connection_properties.source_ip` reusing the tcp compute);
  the `rotl64(prev,1)^new` fold (first-verbatim, nullopt-skip, `terminal` short-circuit on a
  non-empty accumulator); the FIVE DEPARTURE/PARITY rejects (cookie/query_parameter/
  filter_state + regex_rewrite + source_ip-false + the empty-`header_name` parity reject
  `value length must be at least 1 runes`); the NAT-transparent TRUE cross-side affinity
  (`0062`, N=16 × K=16 = 256); the `upstream_rq_total` cross-equal on the HTTP plane; the
  stat-surface note STAYS **1119** (the FIRST LB producer reusing the prior plane's stat
  surface entirely); the NO-new-fuzzer/BackendKind family expectations; the conformance
  re-run (53/53 + 10/10).
- **`docs/envoy-go/DECISIONS.md`** — appended the full **ADR-0237 — the HTTP route
  `hash_policy` producer plane (phase 36.2)** (status ACCEPTED) AFTER ADR-0236, heading
  `## ADR-0237 — …` (matching ADR-0236's depth/structure: a dense-block heading paragraph +
  **Status** + `### Context` + `### Decision` + `### Consequences`). §Context is promoted
  VERBATIM from the SPEC §13 ADR-0237 §Context DRAFT; §Decision records the as-built producer
  wiring (the additive `cluster.HashHeaderValues`; the proto-free `router.HashPolicy`
  parsed at the hcm boundary; the uniform ctx-carried `router.WithDownstreamRemoteAddr`; the
  `applyHashKey` fold at the 4 dial sites; the unit-level reject roster); §Consequences
  records the byte-stable +1 additive symbol, the NO seam/manager/stat change, the
  conformance-path-touch caveat, the `0062` NAT-transparent proof, and the single-value-
  producer-vs-multi-value-digest boundary. DECISIONS tail **ADR-0236 → ADR-0237** (next-free
  ADR-0238).
- **`docs/envoy-go/STATE.md`** — active-phase → `phase 36.2 (load-balancer-ring-hash-http)
  done`; lifecycle-state/next-skill/last-commit/last-updated/next-free-ADR advanced; the
  counts block updated (fixtures 63 → 64, surface 1119 unchanged, fuzzers 42, BackendKind 33,
  DECISIONS tail ADR-0237 / next-free ADR-0238; conformance re-ran green).
- **`docs/envoy-go/ROADMAP.md`** — row 36 status `in-progress → done`; leg column
  `36.1 done, 36.2 done` (the CONSUMED 36.1/36.2 by-plane split CLOSED); a `36.2 IMPL DONE`
  parenthetical prepended; the trailing "Next →" advanced to the next Load-balancing
  candidate / next ROADMAP family. The Load-balancing FAMILY STAYS OPEN (5 candidates remain:
  maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds) — a
  flat family row, NO parent rollup per ADR-0106.

### Final six-gate count confirmations (run from the worktree root, this commit)

| Gate | Recipe | Output | Expected | Match |
|------|--------|--------|----------|-------|
| fixtures | `ls -d test/fixtures/[0-9]* \| wc -l` | `64` | 64 | YES |
| fixtures tail | `ls -d test/fixtures/[0-9]* \| tail -1` | `test/fixtures/0062-lb-ring-hash-http` | `0062-…` | YES |
| DECISIONS tail | `grep '^## ADR-' docs/envoy-go/DECISIONS.md \| tail -1` | `ADR-0237` | ADR-0237 | YES |
| ADR count | `grep -c '^## ADR-' docs/envoy-go/DECISIONS.md` | `236` | +1 vs 235 | YES |
| fuzzers | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` | `42` | 42 | YES |
| stat surface | BEHAVIOR_CONTRACT doc count (`1116 → 1119` block) | `1119` | 1119 | YES |
| BackendKind tail | (unchanged from Task 1) | `TCPThriftResponder = 33` | 33 | YES |
| doc-only | `git diff --stat -- internal/ test/` | **EMPTY** | EMPTY | YES |

As-built final state (carried from Tasks 1-7): surface **1119** (unchanged — ZERO new stat
names), fixtures **64**, fuzzers **42**, BackendKind tail **33**, DECISIONS tail **ADR-0237**,
ONE additive exported symbol (`cluster.HashHeaderValues`), ZERO new packages + ZERO new go.mod
deps (`go mod tidy -diff` EMPTY at Task 7); the full **64/64** differential GREEN, **tidy-diff
empty**, **h2spec 53/53 + proxy-wasm 10/10** re-ran green; the `0062` fixture as-built workload
is **N=16 distinct `X-Hash` values × K=16 = 256 routed requests** (the Task-6 N=4→16
flake-margin fix; the modular-invariant affinity asserted on BOTH sides since the header key is
NAT-transparent).

### Hygiene (Task 8)

`git diff --stat -- internal/ test/` is **EMPTY** (DOC-ONLY — the commit touches only
`docs/`). The 5 modified docs: `BEHAVIOR_CONTRACT.md`, `DECISIONS.md`, `STATE.md`,
`ROADMAP.md`, and this `PROGRESS.md`. Committed LOCAL-ONLY on branch
`phase-36.2-load-balancer-ring-hash-http-impl` (ADR-0052 atomic landing — the whole bundle in
ONE commit); no push, no switch, no amend (`feedback_subagents_no_push`). The controller
squash-merges the 8 task commits to master + pushes at stage-close.
