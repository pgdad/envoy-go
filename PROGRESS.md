# Phase 32.1 IMPL — PROGRESS

Worktree: `.worktrees/phase-32.1-impl`
Branch: `phase-32.1-network-filter-redis-upstream-pool-seam-and-codec-impl`
Base: master `309c72b` (PLAN squash `a28f593` is the substantive predecessor).
Execution: `superpowers:subagent-driven-development` — fresh subagent per task + two-stage review; subagents commit LOCAL-ONLY; controller squash-merges + pushes at stage-close.

## Task 1 — First-action baselines/anchors gate (DONE)

Re-pinned at the IMPL-session tip (`309c72b`):

| Baseline | Value | Verified |
|---|---|---|
| fixtures | **56** (tail `0054-kafka-boot-reject`) | yes |
| fuzzers | **40** | yes |
| stat surface | **536** (BEHAVIOR_CONTRACT doc count; STATE.md:21) | yes |
| BackendKind tail | **31** (`TCPKafkaResponder`, fixture.go:542) | yes |
| DECISIONS tail (heading) | **ADR-0230** (DECISIONS.md:14734; next-free ADR-0231 is only a "next-free" mention) | yes |

TypeURL pin (`proto.MessageName(&redis_proxyv3.RedisProxy{})`):
`envoy.extensions.filters.network.redis_proxy.v3.RedisProxy` → TypeURL = `type.googleapis.com/` + that (the `extensions.` segment, `reference_network_filter_typeurl_extensions`). Import alias: `redis_proxyv3`.

Field roster (go-control-plane `/envoy` v1.32.4 in-tree, `redis_proxy.pb.go`):
- `(*RedisProxy) GetStatPrefix() string` :292
- `(*RedisProxy) GetSettings() *RedisProxy_ConnPoolSettings` :299
- `(*RedisProxy) GetPrefixRoutes() *RedisProxy_PrefixRoutes` :313
- `(*RedisProxy_ConnPoolSettings) GetOpTimeout() *durationpb.Duration` :590
- `(*RedisProxy_PrefixRoutes) GetRoutes() []*RedisProxy_PrefixRoutes_Route` :706
- `(*RedisProxy_PrefixRoutes) GetCatchAllRoute() *RedisProxy_PrefixRoutes_Route` :720
- `(*RedisProxy_PrefixRoutes_Route) GetCluster() string` :922

`go mod tidy`: CLEAN — `go.mod`/`go.sum` byte-identical (redis_proxy/v3 is CORE `/envoy v1.32.4`; ZERO new module — AMEND-R1/R8).

§11.1 as-built anchors (compile-reachable):
- `(*Cluster) IncUpstreamRqTotal()` `internal/cluster/cluster.go:134`
- `(*Cluster) Dial(ctx) (net.Conn, Endpoint, error)` `internal/cluster/cluster.go:198`
- `(*Manager) Get(name) (*Cluster, bool)` `internal/cluster/manager.go:111`
- `TerminalFilter.Handle(ctx, downstream net.Conn)` `internal/filter/network/terminal.go:48`
- kafkabroker registration site `internal/filter/network/builtins/builtins.go:82`

Targets at Task 16: fixtures 56→58, fuzzers 40 (unchanged), stats 536→546 (+10 fixed: 6 counters + 4 gauges), BackendKind 31→32 (`TCPRedisResponder`), DECISIONS tail STAYS ADR-0230 (body fills in place).

## Per-task log

- [x] Task 1 — baselines/anchors gate
- [x] Task 2 — redisproxy skeleton + TypeURL (commit 22744b1; spec+quality APPROVED)
- [x] Task 3 — config.go parse (commit 7914295; spec+quality APPROVED)
- [x] Task 4 — byte-stable reject table test (commit ba350c4; APPROVED, liveness confirmed non-vacuous)
- [x] Task 5 — resp.go part 1 (request decode) (commit 71b6d46; APPROVED). NOTE 3 reviewed+validated deviations from PLAN verbatim: (1) dropped unused `bytes` import in resp_test.go (compile), (2) inline path guards reply-type first bytes `+ - : $ @` → errProtocol (PLAN verbatim would parse `$3\r\nfoo\r\n` as cmd and fail its own MalformedNeverPanics test; guard matches PLAN intent, over-rejects no valid inline cmd), (3) comment misspell analogue→analog.
- [x] Task 6 — resp.go part 2 (reply decode + encode constants) (commit 917e8e5; APPROVED). DEVIATION (controller-approved): PLAN verbatim grouped `:` with `+`/`-` framing-only, but PLAN's own MalformedNeverPanics requires `:x\r\n` to error → split `:` out + `strconv.Atoi` validate (errProtocol on non-numeric); `+`/`-` stay framing-only. Other 4 malformed cases pass under PLAN verbatim. Verbatim-bytes + no-panic + nested-array recursion all verified.
- [x] Task 7 — upstreampool.go seam (commit 55d651a; APPROVED). 5 invariants verified: no internal/cluster import leak (go list -deps CLEAN), ADDITIVE (only 2 new files, existing package byte-identical), lazy dial, single-flight reuse, no mutex/pool (YAGNI). Minor deviation: test-only `defer func(){_=u.Close()}()` for errcheck (package convention).
- [x] Task 8 — commands.go PING/AUTH (commit b6db4ba; APPROVED, no deviations; ECHO/TIME/QUIT/HELLO correctly NOT local at 32.1)
- [x] Task 9 — stats.go 10-name roster (commit f8a2164; APPROVED, no deviations). 10 names byte-exact vs ALL_REDIS_PROXY_STATS (6 counters + 4 gauges), eager+idempotent, only 4 inc accessors (deferred names not over-built).
- [x] Task 10 — filter.go NewFactory + Handle pump (commit a95ca3d; APPROVED — capstone). Production Handle/NewFactory/resolveCatchAll = PLAN verbatim (zero logic drift). 2 TEST-ONLY deviations: (1) interleaved SET→read+OK→GET→read-bar choreography (PLAN's consecutive writes deadlock synchronous net.Pipe; 32.1 pump is depth-1 single-flight, pipelining deferred), (2) added TestNewFactory_ValidConfig (PLAN's validAny helper was unused → lint). Full pkg -race clean.
- [x] Task 11 — 10th built-in + bootstrap blank-import (commit 7a60720; APPROVED). redisproxy registered with BOTH deps.ClusterManager + deps.StatsRegistry; all-nine test extended to all-ten (no prior assertion dropped); bootstrap blank-import added; go mod tidy CLEAN (zero new dep). gofmt reordered import alphabetically (cosmetic); new test uses nil cm (safe — NewFactory closure-captures, derefs only at Handle).
- [x] Task 12 — TCPRedisResponder BackendKind (32) (commit b9884b5; APPROVED). Constant in fixture.go + serve loop in runner_test.go (where kafka/mongo responders are dispatched — anticipated by PLAN File Structure). RESP array-of-bulk parser validated no-desync (consumes all n bulks incl trailing \r\n via io.ReadFull(len+2)); SET→+OK, GET→$3 bar, FIFO no correlation id; no production redisproxy import. Inert until Task 13.
- [x] Task 13 — 0055 driver part 1 (PING arm) (commit cacfea3; APPROVED). Cross-side [redis_proxy] TERMINAL bootstraps (no tcp_proxy), RESP builders, PING local-reply arm. Test BOOTS real contrib reference Envoy + subject; +PONG byte-identical cross-side; CompareBytes is LIVE (reads from live proxy conns, no fabrication). ref=STRICT_DNS+host.docker.internal, subj=STATIC+127.0.0.1 (kafka/mongo precedent).
- [x] Task 14 — 0055 driver part 2 (SET/GET + StatsAsserter + R6) (commits 3e63565 + follow-up 2ba177a; APPROVED). SET/GET cross-side byte-equivalence (+OK / $3 bar) + AssertStats cross-side EQUALITY on 6 names (redis.redis_r.downstream_{cx_total,rq_total=4,cx_rx_bytes_total,cx_tx_bytes_total} + cluster.redis_cluster.upstream_cx_total=1, upstream_rq_total=2). R6: 5 deliberate breaks each FAIL with -count=1 then revert→PASS; committed code is the PASS version (no leftover perturbation). NO cross-side divergence.
  - **UNPLANNED PRODUCTION ADDITION (justified):** added `internal/admin/stats.go` — a flat `GET /stats` admin endpoint (`name: value\n`, sorted). envoy-go had only `/stats/prometheus`, which omits redis.* (the `redis.` prom tag-extractor is 32.2); PLAN D-S32.1-5 explicitly required a flat-`/stats` scrape → endpoint had to exist. Purely additive (new route only; existing handlers untouched → 56-fixture back-compat safe). Follow-up 2ba177a routed it through the shared `writeAdminHeaders` helper (uniform 4-header set) + added a header-assertion test.
  - **TASK 16 MUST DOCUMENT:** BEHAVIOR_CONTRACT Admin API section says "seven endpoints/routes" (lines ~614/616/618/729) → now EIGHT; add `GET /stats` (flat internal-name text; phase 32.1; for the 0055 AssertStats scrape) to the endpoint list + the applies-to header list. The admin.go inline doc was already bumped to "eight routes".
- [x] Task 15 — 0056-redis-boot-reject (commits fbffd6e + follow-up 96d7681; APPROVED). Symmetric boot-reject: both sides fail to boot on omitted stat_prefix (reference PGV `RedisProxyValidationError.StatPrefix`, subject `redis_proxy: stat_prefix is required`). LIVENESS FIX: original substring `stat_prefix` matched the reference ONLY via a circular driver comment (reference's real error is CamelCase `StatPrefix`, no lowercase token). Changed common substring to `redis_proxy` (genuinely in both real stderrs: subject error prefix + reference filter name/@type); removed circular comment; R6 bogus→FAIL, revert→PASS proves liveness. PRIMARY assertion (both fail to boot) is load-bearing; substring is secondary sanity. Separate dir from 0055 (cross-side XOR boot-reject constraint).
- [x] Task 16 — completion bundle + six-gate (no commit SHA yet — commit is this task's output; see below).

## Task 16 — Completion bundle detail

### Six-gate evidence (all GREEN)

**Gate 1: `go build ./...`**
```
(no output — clean build)
```

**Gate 2: `go vet ./...`**
```
(no output — clean)
```

**Gate 3: `golangci-lint run`**
```
(no output — clean)
```

**Gate 4: `go test ./... -race -short`**
```
ok  	github.com/esalaine/envoy-go/internal/admin	(cached)
ok  	github.com/esalaine/envoy-go/internal/bootstrap	(cached)
ok  	github.com/esalaine/envoy-go/internal/cluster	(cached)
ok  	github.com/esalaine/envoy-go/internal/filter/network	(cached)
ok  	github.com/esalaine/envoy-go/internal/filter/network/redisproxy	(cached)
ok  	github.com/esalaine/envoy-go/internal/filter/network/kafkabroker	(cached)
ok  	github.com/esalaine/envoy-go/internal/filter/network/mongoproxy	(cached)
... (all 81+ packages ok, -race clean)
```

**Gate 5: `go test ./test/differential/ -count=1` (full 58-dir suite)**

Run 1 — 57 PASS, 1 FAIL (transient Docker timing flake):
- FAIL: `0046-zookeeper-requests` — Docker container startup saturation under load (58 containers starting in rapid succession); the container started late, not a real regression.
- All others (0000–0045, 0047–0056) PASS including the NEW 0055 and 0056.
- Confirmed flake: ran `0046` in ISOLATION → PASS at 4.52s immediately. Ran the full suite a SECOND time:

Run 2 — ALL 58 PASS:
```
ok  	github.com/esalaine/envoy-go/test/differential	190.390s
```
Exit code 0. h2spec 53/53 + proxy-wasm 10/10 UNAFFECTED (phase 32.1 touches no HTTP/h2/proxy-wasm path).

**Gate 6: fixture/fuzzer/stat/BackendKind/DECISIONS counts at Task 16**

| Baseline (Task 1) | Target (Task 16) | Actual |
|---|---|---|
| fixtures 56 | 58 | **58** (tail `0056-redis-boot-reject`) |
| fuzzers 40 | 40 (unchanged) | **40** (`FuzzRESPDecode` defers to 32.2) |
| stat surface 536 | 546 (+10 fixed) | **546** |
| BackendKind tail 31 | 32 (`TCPRedisResponder`) | **32** |
| DECISIONS tail ADR-0230 | STAYS ADR-0230 (body fills in-place) | **ADR-0230** (ACCEPTED) |

All six targets HIT. Six-gate is **GREEN**.

### Completion bundle — what was done

- **ADR-0230 §Decision/§Consequences** body filled IN-PLACE in `docs/envoy-go/DECISIONS.md` (status DRAFT → ACCEPTED per ADR-0044). Covers: 6 exported identifiers (`UpstreamDialFunc`, `UpstreamConn`, `NewUpstreamConn`, `Send`, `Reader`, `Close`); dial-closure decoupling (NO `internal/cluster` import in `internal/filter/network`); one-conn-per-downstream MVP; lazy dial; depth-1 FIFO/positional pop-front; deferred: shared per-host pool + deep queue + two-goroutine pipelined model + ADR-0223 mutex.
- **ADR-0229 §Decision/§Consequences 32.1 half** filled IN-PLACE (status DRAFT → PARTIAL — 32.2 half deferred). 9 numbered items: package layout, config parse, RESP codec, PING/AUTH local-reply, 10-name roster, Handle pump, 10th built-in, TCPRedisResponder, 0055+0056 fixtures.
- **BEHAVIOR_CONTRACT.md 32.1 bundle:**
  - Admin endpoint count: "seven" → "eight" (4 occurrences on lines ~614/616/618/729).
  - NEW `GET /stats` subsection (flat internal-name text; sorted; phase 32.1; for the 0055 `StatsAsserter` scrape).
  - NEW `Phase 32.1 extension — 536 → 546 internal names` block (10 fixed: 6 counters + 4 gauges; 32.2 anticipation note).
  - NEW `### envoy.filters.network.redis_proxy` subsection (10th built-in; TERMINAL routing proxy; 10 names; PING/AUTH local; 0055/0056 fixtures; 32.2 forward-pointer).
  - NEW `## Network filters — upstream connection-pool / cluster-routing seam (32.1)` section (`UpstreamConn` seam; SIXTH structural extension; dial-closure decoupling; no-mutex MVP; deferred boundary).
- **STATE.md:** active-phase → IMPL done; lifecycle-state → 32.2 SPEC; next-skill → `superpowers:writing-plans`; counts advanced (fixtures 58, stat surface 546, BackendKind 32); DECISIONS tail → body LANDED (ACCEPTED); last-commit → IMPL completion; next-free ADR → ADR-0231 (unchanged).
- **ROADMAP.md:** sub-row 32.1 `in-progress → done` + IMPL-DONE annotation (parent row 32 STAYS `in-progress`; 32.2 STAYS `planned`).
- **next-prompt.txt:** full REWRITE for the 32.2-SPEC cold-start (full command set + per-command/splitter/REDIS_CLUSTER_STATS roster + `redis.` LABEL-HOISTED Prometheus arm + 4 gauge inc/dec + differential command matrix + 41st fuzzer `FuzzRESPDecode` + parent-row-32 ROLLUP; reads-first list).

## Controller stage-close verification (independent)

The controller independently re-ran the six-gate at the branch tip (NOT trusting the implementer report):

- GATE 1 `go build ./...` → OK
- GATE 2 `go vet ./...` → OK
- GATE 3 `golangci-lint run` → exit 0 (clean)
- GATE 4 `go test ./... -race -short` → exit 0 (clean)
- GATE 5 `go test ./test/differential/ -count=1` → **`ok 192.686s`, exit 0** (all 58 dirs; the 56 pre-existing byte-identical — the seam is ADDITIVE — + 0055/0056)
- GATE 6 h2spec 53/53 + proxy-wasm 10/10 → asserted-unaffected (32.1 touches no HTTP/h2/proxy-wasm path)

Two-stage review for every task (spec-compliance + code-quality), independent reviewers reading the actual code/running gates. Notable review catches in the loop:
- Task 6: PLAN self-contradiction (`:x\r\n`) — `:` integer validation added (controller-approved).
- Task 14: unplanned production `GET /stats` endpoint (justified — PLAN D-S32.1-5 required a flat-`/stats` scrape that didn't exist) + follow-up routing it through `writeAdminHeaders` for the uniform admin-header invariant.
- Task 15: boot-reject substring was circular (matched only the driver's own echoed comment; reference's real error is CamelCase `StatPrefix`) → changed to `redis_proxy` (genuinely in both real stderrs) + R6 bogus→FAIL/revert→PASS liveness proof.
- Task 16: STATE.md active-phase/lifecycle-state/next-skill prose still carried stale "Next → the 32.1 IMPL" PLAN-era narrative → rewritten to the 32.2-SPEC-forward reality (commit 23ba550).

- [x] Task 16 — completion bundle + six-gate (commits 15956a4 + STATE prose fix 23ba550; controller six-gate GREEN)

Branch ready for controller squash-merge to master + push to origin (feedback_push_to_origin / feedback_subagents_no_push).
