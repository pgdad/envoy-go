# Phase 32.1 PLAN — redis_proxy upstream-pool seam + RESP codec + round-trip foundation

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended, per `feedback_execution_style`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Subagents commit **LOCAL-ONLY** (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on the touched packages before its commit (`feedback_pertask_gofmt_lint`). After a temporary deliberate break (Task 14 R6 liveness), use `go test -count=1` to defeat result caching (`reference_differential_break_protocol_count1`). Subagent worktree-detach hygiene per `feedback_subagent_worktree_detach`.

**Goal:** Land a real single-command `redis_proxy` TERMINAL routing proxy — the project's FIRST connection-terminating proxy — built on (a) the NEW **upstream connection-pool / cluster-routing seam** (`internal/filter/network/upstreampool.go`; ADR-0230; the framework's SIXTH structural extension), (b) the NEW `internal/filter/network/redisproxy/` package (TypeURL + config parse + the in-house RESP codec + the `TerminalFilter.Handle` command→reply pump + the 10-name eager downstream roster + the PING/AUTH local-reply set + lazy catch_all resolution), (c) the 10th built-in + the `redis_proxy/v3` blank-import (ZERO new go.mod dep), (d) the `TCPRedisResponder` BackendKind (value 32), proven by fixtures `0055-redis-roundtrip` (cross-side `StatsAsserter` + the downstream-RESP byte-equivalence prong; PING + proxied SET/GET arms) + `0056-redis-boot-reject`.

**Architecture:** A NEW `internal/filter/network/redisproxy/` package implements `network.TerminalFilter` (NOT `ReadFilter`/`WriteFilter` — it TERMINATES the downstream connection via `Handle(ctx, conn)`; there is no `tcp_proxy` behind it; the `*tcpproxy.Filter` shape). Per accepted downstream connection, `Handle` owns the raw `net.Conn`, reads RESP request frames from a `bufio.Reader` over it (partial frames simply BLOCK — the terminal owns the conn, UNLIKE the mongo/kafka observer private-buffer model), and dispatches each request: PING/AUTH are answered LOCALLY (zero upstream); data commands round-trip through the **upstream-pool seam** — one upstream connection per downstream connection, lazily dialed on the first proxied command over the as-built `cluster.Cluster.Dial` path, with synchronous single-flight (depth-1 FIFO/positional) reply correlation. Reply bytes are forwarded VERBATIM downstream (`reference_wire_format_both_sides_see_same_bytes`, EXTENDED to the response the proxy generates). The seam consumes a DIAL CLOSURE → `internal/filter/network` gains NO `internal/cluster` import (the `upstreamcluster.go` decoupling discipline). The differential proof is TWO-pronged (a §9 FIRST): downstream-RESP byte-equivalence PLUS cross-side `StatsAsserter` over the flat admin `/stats` `redis.<stat_prefix>.` + `cluster.<name>.*` names.

**Tech Stack:** Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227); go-control-plane `/envoy` v1.32.4 (ADR-0008). Reuses `internal/filter/network/` (26.1/26.2 `TerminalFilter`; 27 ADR-0219 override-seam decoupling — consumed, the existing files untouched), `internal/cluster/` (`Manager.Get` + `Cluster.Dial` + `Cluster.IncUpstreamRqTotal`), `internal/filter/tcpproxy/` (the terminal-dial precedent the seam parallels), `internal/stats/` (06.1 counters + gauges + `IsValidName`), the differential harness + `fixture.StatsAsserter`. **ZERO new third-party `go.mod` dependencies** (redis_proxy is a CORE `/envoy v1.32.4` extension — UNLIKE kafka_broker's `/contrib`; the RESP codec is in-house byte scanning, no Redis client library; AMEND-R1/R8).

---

## ADR-0045 split-gate FINAL re-check (at PLAN time, per SPEC §14.1 + parent §3.0)

The gate fires at `> ~25 tasks OR > ~1500 net-new production LoC`. This PLAN decomposes to **16 tasks** / **~940–1420 production LoC** (the SPEC §14.1 envelope, re-confirmed at PLAN time on the 26.x–31 accounting basis — fixture drivers + unit tests EXCLUDED):

| Unit | Production LoC | Tasks |
|---|---|---|
| `config.go` (PGV arms + runtime no-upstream check + unknown-cluster tolerance + IsValidName guard + parse-accept) | ~120–170 | 3 |
| PARSE-REJECT constants (`redis_proxy: ` arms) | (folded into config.go) | 4 |
| `resp.go` (value-type framing + request/reply decode + encode + streaming reassembly + overflow guards) | ~300–420 | 5–6 |
| `upstreampool.go` (the seam: dial closure + lazy dial + single-flight Send/Reader/Close + lifecycle) | ~120–200 | 7 |
| `commands.go` (PING/AUTH local-reply + dispatch) | ~60–110 | 8 |
| `stats.go` (the 10-name roster + inc accessors) | ~70–110 | 9 |
| `filter.go` + `redisproxy.go` + `doc.go` (NewFactory + the Handle pump + glue) | ~180–260 | 2, 10 |
| builtins + `bootstrap.go` (the 10th built-in + blank-import) | ~30–50 | 11 |
| `TCPRedisResponder` BackendKind (the canned RESP responder) | ~60–100 | 12 |
| **Total (production basis)** | **~940–1420** | **16** |

Both axes under the gate (16 ≤ ~25 tasks; ~1420 ≤ ~1500 LoC) → **NO split. 32.1 proceeds as ONE sub-phase.** The pre-authorized 32.1a (Tasks 1–11: the package + seam + codec + builtins/bootstrap — a bootable, unit-proven redis_proxy) / 32.1b (Tasks 12–16: the `TCPRedisResponder` + `0055` + `0056` + the completion bundle) escape-valve axis (SPEC §14.1) stays **UNCONSUMED**. The fixture drivers (`0055` + `0056`; the `0053`/`0054` kafka precedents ~600–900 LoC across both) are excluded per the 26.x–31 accounting precedent.

---

## PLAN-time D-question dispositions (SPEC §12.2)

- **Task order = the SPEC §14 spine, lightly resequenced for green-compiling dependency order.** Each task compiles + tests green standalone (`go test ./internal/filter/network/redisproxy/...` builds the whole package every task). The order is `baselines (1) → skeleton+TypeURL (2) → config (3) → reject-constants (4) → resp request (5) → resp reply+encode (6) → seam (7) → commands (8) → stats (9) → filter glue (10) → integration (11) → backend (12) → fixtures (13–15) → completion (16)`. The seam (Task 7, in `internal/filter/network/`) is dependency-free and could land anywhere after Task 1; it is placed before `filter.go` (Task 10) which consumes it.
- **D-S32.1-1 (the seam's exact exported signatures) — RESOLVED at PLAN: `UpstreamConn` + `UpstreamDialFunc` + `Send`/`Reader`/`Close` (the SPEC §4.2 shape, NOT a fused `RoundTrip`).** Keeping `Send` (write + lazy-dial + onRequest hook) and `Reader()` (the decode source) as separate methods lets the codec own the reply-frame boundary detection (`decodeReply` reads from `Reader()`) — the seam stays codec-agnostic (a connection-lifecycle + ordered-round-trip primitive thrift can reuse). Signatures pinned at Task 7.
- **D-S32.1-2 (the RESP decoder internal representation) — RESOLVED at PLAN: raw-bytes-verbatim, streaming-reader, no materialized value tree.** `decodeRequest` returns `(cmd string, raw []byte, err error)`; `decodeReply` returns `(raw []byte, err error)`. Both read incrementally from a `*bufio.Reader` (block on partial frames) and accumulate the consumed bytes into a `bytes.Buffer` for verbatim forwarding. Bulk-length / array-count bounds are checked against `maxBulkLen` (512 MiB; the upstream `proto_max_bulk_size` analogue) + `maxArrayLen` (1 Mi elements) BEFORE any allocation. No `respValue` AST is built (YAGNI; the 32.2 command matrix needs only the command name + raw bytes). Finalized at Tasks 5–6.
- **D-S32.1-3 (PARSE-REJECT byte-stable wording) — anticipated prefix `redis_proxy: `; the exact arm strings finalized at Task 4** (`TestParseRejectConstants_ByteStable`). The boot-reject differential matches a per-side stderr SUBSTRING (not cross-impl string equality — the kafka `0054`/mongo `0050` precedent; SPEC §6).
- **D-S32.1-4 (RESP request-byte builder helper shape) — anticipated `respArray(...)`, `respBulk(...)`, `inline(...)` builders in the `0055` driver package, shared with the 32.2 command-matrix arms. Finalized at Task 13.**
- **D-S32.1-5 (flat-`/stats` StatsAsserter mechanics) — RESOLVED at PLAN: the `0055` driver adds a small in-band `name → value` parser over the admin `/stats` text** (the harness exposes no flat-`/stats` scrape-and-diff helper; the `fixture.go:70-77` in-band discipline gives the driver full latitude). The assertion is keyed by `redis.<sp>.` + `cluster.<name>.` internal names. Finalized at Task 14.
- **D-S32.1-6 (the lazy-dial guard + the unknown-catch_all-cluster failure path) — RESOLVED at PLAN: `Handle` holds a nil `*network.UpstreamConn` and an `ensureUpstream()` closure that resolves `cm.Get(catchAllCluster)` ONCE on the first proxied command. A missing cluster → `ensureUpstream` returns an error → `Handle` returns (the deferred `downstream.Close()` runs); NO `-ERR` is synthesized at 32.1** (a coverage-boundary note in the 32.1 bundle). Finalized at Task 10.
- **The 10-name roster lives on the `filter` struct (a `*redisStats`), created at `NewFactory` — NOT on `compiledConfig` (refines the SPEC §3.7 anticipated "roster on cfg").** `parseConfig` (Task 3) stays PURE (no `*stats.Registry` argument), so `config.go` carries NO reference to the `redisStats` type that `stats.go` (Task 9) introduces — keeping Task 3 green-compiling before Task 9. `NewFactory` (Task 10) calls `parseConfig` then `newRedisStats(reg, cfg.statPrefix)` and stores both on the shared `*filter`. This is the 29.1 reorder discipline (`config → stats → … → filter`) applied so every intermediate task builds standalone.
- **`NewFactory` + the `filter` struct land at Task 10 (`filter.go`), NOT Task 2 (`redisproxy.go`).** Task 2 lands `doc.go` + `redisproxy.go` (TypeURL ONLY); `NewFactory` constructs the decoder/roster/seam consumption that Tasks 3–9 build, so it follows them (the 28.1/29.1 precedent — `mongoproxy.go` was TypeURL-only at 29.1 Task 2, `NewFactory` at Task 10).

---

## File Structure

**Created:**
- `internal/filter/network/upstreampool.go` — the ADR-0230 seam: `UpstreamDialFunc` + `UpstreamConn` + `NewUpstreamConn` + `Send`/`Reader`/`Close` (package `network`; NO `internal/cluster` import).
- `internal/filter/network/upstreampool_test.go` — lazy-dial / no-dial-on-PING-only / single-flight round-trip / `-race` / `Close` idempotence tests.
- `internal/filter/network/redisproxy/doc.go` — package doc (the terminal redis_proxy; ADR-0229/ADR-0230 cross-refs; the 32.2 forward-pointers).
- `internal/filter/network/redisproxy/redisproxy.go` — `TypeURL` (via `proto.MessageName`).
- `internal/filter/network/redisproxy/redisproxy_test.go` — TypeURL pinning test.
- `internal/filter/network/redisproxy/config.go` — `compiledConfig` + `parseConfig` (PGV-required arms + runtime no-upstream check + unknown-cluster tolerance + `IsValidName(stat_prefix)` guard + deferred-field parse-accept) + the PARSE-REJECT byte-stable constants.
- `internal/filter/network/redisproxy/config_test.go` — parse / defaults / every PGV-mirror reject-arm / runtime-no-upstream / unknown-cluster-tolerated / IsValidName-guard / byte-stable-constants tests.
- `internal/filter/network/redisproxy/resp.go` — the in-house RESP codec (`decodeRequest` + `decodeReply` + the local-reply encode constants; value-type framing + null sentinels + inline; streaming-reader reassembly + overflow guards).
- `internal/filter/network/redisproxy/resp_test.go` — each value type / null sentinels / inline / nested arrays / partial-frame block-and-resume / malformed/overflow/truncated → error-not-panic / raw-bytes-verbatim tests.
- `internal/filter/network/redisproxy/commands.go` — the 32.1 local-reply set (PING / AUTH) + the local-vs-proxy dispatch decision.
- `internal/filter/network/redisproxy/commands_test.go` — PING/AUTH dispatch + reply-byte / case-insensitivity / proxied-passthrough tests.
- `internal/filter/network/redisproxy/stats.go` — the 10-name eager roster (`newRedisStats`) + the suffix tables + the 32.1 inc accessors.
- `internal/filter/network/redisproxy/stats_test.go` — `TestStatRoster32_1_MatchesUpstream` (R2 golden) + eager/idempotent/inc-accessor tests.
- `internal/filter/network/redisproxy/filter.go` — `NewFactory(cm, reg)` + the `filter` struct + the `TerminalFilter.Handle` command→reply pump.
- `internal/filter/network/redisproxy/filter_test.go` — factory / TypeURL-reject / single-command round-trip / pipelined SET-then-GET / PING-local / unknown-cluster graceful-close / EOF clean-close / byte-count-increment tests.
- `test/fixtures/0055-redis-roundtrip/driver/driver.go` + `README.md` — the cross-side byte-equivalence + flat-`/stats` StatsAsserter fixture (PING + proxied arms).
- `test/fixtures/0056-redis-boot-reject/driver/driver.go` + `README.md` — the symmetric `stat_prefix`-required boot-reject fixture.

**Modified:**
- `internal/filter/network/builtins/builtins.go` — the 10th registration (after `kafkabroker` at `builtins.go:82`); package doc `nine` → `ten`; `RegisterBuiltins` doc adds `redis_proxy`. redisproxy passes BOTH `deps.ClusterManager` (lazy catch_all) + `deps.StatsRegistry` (the roster).
- `internal/filter/network/builtins/builtins_test.go` — the all-ten registration test + the redis registration test + boot smoke.
- `internal/bootstrap/bootstrap.go` — the `redis_proxy/v3` blank-import (after `kafka_broker/v3` at `bootstrap.go:111`).
- `test/differential/fixture/fixture.go` — the `TCPRedisResponder = 32` BackendKind constant + its backend wiring (after `TCPKafkaResponder = 31` at `fixture.go:542`).
- `test/differential/runner_test.go` — the `0055`/`0056` driver blank-imports + the `TCPRedisResponder` switch-case (mirroring the `TCPKafkaResponder` arm).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` / `DECISIONS.md` (ADR-0230 §Decision/§Consequences body + the ADR-0229 32.1 half) / `STATE.md` / `ROADMAP.md` / `next-prompt.txt` — the completion bundle (Task 16).

**Untouched (pinned — the seam is ADDITIVE; a regression gate; SPEC §13-R1):** the existing `internal/filter/network/` files (`chain.go` / `readconn.go` / `writeconn.go` / `types.go` / `callbacks.go` / `terminal.go` / `registry.go` / `upstreamcluster.go` — `upstreampool.go` is a NEW file, the existing files stay byte-identical), `internal/listener/manager.go`, `internal/filter/tcpproxy/`, `internal/filter/hcm/`, `internal/cluster/` (consumed via `Manager.Get`/`Cluster.Dial`/`IncUpstreamRqTotal`, NOT modified), `internal/stats/name.go` (the `redis.` Prometheus tag-extractor arm is 32.2 — the 32.1 differential compares flat `/stats` internal names), `internal/stats/registry.go` / `gauge.go` / `counter.go`.

---

## Task 1: First-action baselines/anchors gate (no code change)

**Files:** none modified — verification + re-pin gate at the IMPL-session tip; record in `PROGRESS.md` (created this task at the worktree root).

- [ ] **Step 1: Re-confirm the project counts at the IMPL-session tip**

Run (from the worktree root):
```bash
git log --oneline -1
# fixtures (canonical recipe):
ls -d test/fixtures/[0-9]* | wc -l            # expect 56; tail dir:
ls -d test/fixtures/[0-9]* | tail -1          # expect test/fixtures/0054-kafka-boot-reject
# fuzzers (canonical recipe — scoped to ./internal):
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 40 (tail FuzzKafkaDecode)
# BackendKind tail:
grep -nE "TCPKafkaResponder BackendKind = 31" test/differential/fixture/fixture.go   # expect a hit
# DECISIONS.md tail + next-free:
grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -3  # tail = ADR-0230 → next-free ADR-0231
```
Expected: fixtures **56** (tail `0054-kafka-boot-reject`); fuzzers **40**; BackendKind tail **31** (`TCPKafkaResponder`); DECISIONS.md tail **ADR-0230** (next-free **ADR-0231**; the ADR-0230 §Context draft landed at the 32.1 SPEC). 32.1 lands `0055`+`0056` → 58, NO new fuzzer (`FuzzRESPDecode` is 32.2), `TCPRedisResponder = 32`, and the ADR-0230 §Decision/§Consequences body IN PLACE (no new ADR number consumed).

- [ ] **Step 2: Re-confirm the stat surface = 536**

Run the project's canonical stat-surface recipe (the count STATE.md / BEHAVIOR_CONTRACT.md report as **536** — the BEHAVIOR_CONTRACT stat-table row count; do NOT invent a new recipe). Expected: **536**. 32.1 lands +10 (6 downstream counters + 4 gauges; D-P32-1 EAGER) → **546** at Task 16.

- [ ] **Step 3: Re-confirm `proto.MessageName` (the TypeURL pin) + the field roster + a clean `go mod tidy` (R8)**

```bash
cat > /tmp/redis_tu.go <<'EOF'
package main

import (
	"fmt"

	rpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	"google.golang.org/protobuf/proto"
)

func main() { fmt.Println(proto.MessageName(&rpv3.RedisProxy{})) }
EOF
go run /tmp/redis_tu.go   # expect: envoy.extensions.filters.network.redis_proxy.v3.RedisProxy
```
Expected `proto.MessageName` = `envoy.extensions.filters.network.redis_proxy.v3.RedisProxy` (the `extensions.` segment per `reference_network_filter_typeurl_extensions`) → `TypeURL` = `type.googleapis.com/` + that. Confirm the field accessor set against go-control-plane v1.32.4 in-tree:
```bash
RP=$(go env GOMODCACHE)/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/network/redis_proxy/v3/redis_proxy.pb.go
grep -nE "func \(x \*RedisProxy\) Get(StatPrefix|Settings|PrefixRoutes)\b" $RP
# expect: GetStatPrefix() string / GetSettings() *RedisProxy_ConnPoolSettings / GetPrefixRoutes() *RedisProxy_PrefixRoutes
grep -nE "func \(x \*RedisProxy_ConnPoolSettings\) GetOpTimeout" $RP
# expect: GetOpTimeout() *durationpb.Duration
grep -nE "func \(x \*RedisProxy_PrefixRoutes\) Get(CatchAllRoute|Routes)\b|func \(x \*RedisProxy_PrefixRoutes_Route\) GetCluster" $RP
# expect: GetCatchAllRoute() *RedisProxy_PrefixRoutes_Route / GetRoutes() []*... / GetCluster() string
```
Confirm a clean `go mod tidy` (R8 — redis_proxy/v3 is CORE `/envoy v1.32.4`, ZERO new module):
```bash
cp go.mod /tmp/go.mod.before && cp go.sum /tmp/go.sum.before
go mod tidy
diff /tmp/go.mod.before go.mod && diff /tmp/go.sum.before go.sum && echo "CLEAN: go.mod/go.sum unchanged"
```
Expected: `go mod tidy` leaves `go.mod`/`go.sum` byte-identical (the bindings are already present from the existing `/envoy v1.32.4` dep; the blank-import at Task 11 + the first `redis_proxyv3` consumer at Task 2 add NO module). Record the exact Go identifiers + the alias `redis_proxyv3` for Tasks 2–3.

- [ ] **Step 4: Re-verify the §11.1 as-built anchors compile-reachable**

```bash
grep -nE "func \(c \*Cluster\) Dial|func \(c \*Cluster\) IncUpstreamRqTotal" internal/cluster/cluster.go   # :198 / :134
grep -nE "func \(m \*Manager\) Get" internal/cluster/manager.go                                              # Manager.Get(name) (*Cluster, bool)
grep -nE "Handle\(ctx context.Context, downstream net.Conn\)" internal/filter/network/terminal.go            # the TerminalFilter contract
grep -nE "reg.Register\(kafkabroker.TypeURL" internal/filter/network/builtins/builtins.go                    # :82 — the registration site
```
Expected: all present (the seam + factory anchors are live).

- [ ] **Step 5: Create `PROGRESS.md`** at the worktree root with the count baselines above + a per-task log section. Commit:

```bash
git add PROGRESS.md
git commit -m "phase 32.1 IMPL Task 1: first-action baselines gate (fixtures 56, fuzzers 40, stats 536, BackendKind 31, ADR tail 0230; redis_proxy TypeURL + field roster + clean go mod tidy pinned)"
```

---

## Task 2: `redisproxy` package skeleton + TypeURL

**Files:**
- Create: `internal/filter/network/redisproxy/doc.go`
- Create: `internal/filter/network/redisproxy/redisproxy.go`
- Test: `internal/filter/network/redisproxy/redisproxy_test.go`

- [ ] **Step 1: Write the failing TypeURL pinning test**

`redisproxy_test.go`:
```go
package redisproxy

import "testing"

func TestTypeURL_PinnedToUpstreamExtensionsName(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.network.redis_proxy.v3.RedisProxy"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}
```

- [ ] **Step 2: Run it — expect a COMPILE failure** (`undefined: TypeURL`)

Run: `go test ./internal/filter/network/redisproxy/ -run TestTypeURL -v`
Expected: build error `undefined: TypeURL`.

- [ ] **Step 3: Write `doc.go` + `redisproxy.go`**

`doc.go`:
```go
// Package redisproxy implements the envoy.filters.network.redis_proxy network
// filter (ADR-0229) — the project's FIRST terminal routing proxy. UNLIKE the
// prior network-filter rows (echo/sni_cluster/zookeeper_proxy/mongo_proxy/
// kafka_broker, all passive sniffers on a [filter, tcp_proxy] chain), redis_proxy
// TERMINATES the downstream connection: it implements network.TerminalFilter
// (Handle owns the raw net.Conn), parses RESP request frames, answers PING/AUTH
// locally, and round-trips data commands to an upstream cluster member via the
// upstream connection-pool / cluster-routing seam (ADR-0230,
// internal/filter/network/upstreampool.go).
//
// At phase 32.1 it lands the seam + the RESP codec + the round-trip foundation:
// the config parse (stat_prefix + settings.op_timeout + catch_all_route.cluster,
// with unknown clusters tolerated at validate and resolved lazily at Handle), the
// in-house RESP codec (the +/-/:/$/* value types + null sentinels + inline
// commands; the streaming-reader partial-frame model — the terminal owns the conn
// and block-reads a bufio.Reader, NO private high-water buffer), the PING/AUTH
// local-reply set (zero upstream), the 10 fixed downstream cx/rq counters + the 4
// created-not-yet-incremented gauges under redis.<stat_prefix>., and the
// TerminalFilter.Handle command->reply pump (one upstream conn per downstream
// conn; synchronous single-flight FIFO/positional reply correlation).
//
// 32.2 completes the full command set + the per-command/splitter/REDIS_CLUSTER
// stat roster + the gauges' inc/dec + the redis. Prometheus tag-extractor arm +
// the differential command matrix + the 41st fuzzer FuzzRESPDecode + the
// parent-row-32 rollup.
package redisproxy
```

`redisproxy.go`:
```go
package redisproxy

import (
	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	"google.golang.org/protobuf/proto"
)

// TypeURL is the canonical Any type URL for redis_proxy's typed_config. Derived
// via proto.MessageName (NEVER a hand-typed docs string —
// reference_network_filter_typeurl_extensions; the kafkabroker.go/mongoproxy.go
// precedent). Resolves to the SPEC §5.1 string (the extensions. segment).
// redis_proxy/v3 is CORE /envoy v1.32.4 (AMEND-R1) — ZERO new go.mod dep.
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&redis_proxyv3.RedisProxy{}))
```

- [ ] **Step 4: Run the TypeURL test — expect PASS**

Run: `go test ./internal/filter/network/redisproxy/ -run TestTypeURL -v`
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
go test ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/ && git commit -m "phase 32.1 IMPL Task 2: redisproxy skeleton + TypeURL (proto.MessageName, extensions. segment)"
```
Expected: `gofmt -l` prints nothing; lint clean; tests PASS.

---

## Task 3: `config.go` — parse + PGV arms + runtime no-upstream check + unknown-cluster tolerance + IsValidName guard

**Files:**
- Create: `internal/filter/network/redisproxy/config.go`
- Test: `internal/filter/network/redisproxy/config_test.go`

> **PLAN disposition:** `parseConfig` is PURE (takes the proto, returns `*compiledConfig` or a PARSE-REJECT error). It does NOT take a `*stats.Registry` and stores NO roster pointer (the roster lives on the `filter` struct, created at `NewFactory` — Task 10). The PARSE-REJECT byte-stable CONSTANTS land here too; their dedicated `TestParseRejectConstants_ByteStable` table test is Task 4 (the constants must exist for Task 3's reject tests to reference them). `cm.Get` is NOT called at parse (that would reject unknown clusters, breaking AMEND-R2 arm C — the unknown-cluster tolerance); resolution is lazy at `Handle`.

- [ ] **Step 1: Write the failing parse + reject tests**

`config_test.go`:
```go
package redisproxy

import (
	"testing"

	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// settings builds a minimal valid ConnPoolSettings (op_timeout required).
func settings() *redis_proxyv3.RedisProxy_ConnPoolSettings {
	return &redis_proxyv3.RedisProxy_ConnPoolSettings{OpTimeout: durationpb.New(1_000_000_000)}
}

// catchAll builds a prefix_routes with a catch_all_route → cluster.
func catchAll(cluster string) *redis_proxyv3.RedisProxy_PrefixRoutes {
	return &redis_proxyv3.RedisProxy_PrefixRoutes{
		CatchAllRoute: &redis_proxyv3.RedisProxy_PrefixRoutes_Route{Cluster: cluster},
	}
}

func valid() *redis_proxyv3.RedisProxy {
	return &redis_proxyv3.RedisProxy{StatPrefix: "redis_a", Settings: settings(), PrefixRoutes: catchAll("redis_cluster")}
}

func TestParseConfig_StoresFields(t *testing.T) {
	cfg, err := parseConfig(valid())
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.statPrefix != "redis_a" {
		t.Errorf("statPrefix = %q, want redis_a", cfg.statPrefix)
	}
	if cfg.catchAllCluster != "redis_cluster" {
		t.Errorf("catchAllCluster = %q, want redis_cluster", cfg.catchAllCluster)
	}
	if cfg.opTimeout != 1_000_000_000 {
		t.Errorf("opTimeout = %v, want 1s", cfg.opTimeout)
	}
}

func TestParseConfig_StatPrefixRequired(t *testing.T) {
	m := valid()
	m.StatPrefix = ""
	_, err := parseConfig(m)
	if err == nil || err.Error() != errStatPrefixRequired {
		t.Fatalf("err = %v, want %q", err, errStatPrefixRequired)
	}
}

func TestParseConfig_StatPrefixInvalidName(t *testing.T) {
	m := valid()
	m.StatPrefix = "bad prefix!" // not IsValidName → reject at the user-input boundary
	_, err := parseConfig(m)
	if err == nil || err.Error() != errStatPrefixInvalid {
		t.Fatalf("err = %v, want %q", err, errStatPrefixInvalid)
	}
}

func TestParseConfig_SettingsRequired(t *testing.T) {
	m := valid()
	m.Settings = nil
	_, err := parseConfig(m)
	if err == nil || err.Error() != errSettingsRequired {
		t.Fatalf("err = %v, want %q", err, errSettingsRequired)
	}
}

func TestParseConfig_OpTimeoutRequired(t *testing.T) {
	m := valid()
	m.Settings = &redis_proxyv3.RedisProxy_ConnPoolSettings{} // settings present, op_timeout absent
	_, err := parseConfig(m)
	if err == nil || err.Error() != errOpTimeoutRequired {
		t.Fatalf("err = %v, want %q", err, errOpTimeoutRequired)
	}
}

func TestParseConfig_NoUpstream(t *testing.T) {
	// prefix_routes omitted AND prefix_routes: {} both → the runtime no-upstream reject.
	for _, pr := range []*redis_proxyv3.RedisProxy_PrefixRoutes{nil, {}} {
		m := valid()
		m.PrefixRoutes = pr
		_, err := parseConfig(m)
		if err == nil || err.Error() != errNoUpstream {
			t.Fatalf("prefix_routes=%v: err = %v, want %q", pr, err, errNoUpstream)
		}
	}
}

func TestParseConfig_CatchAllClusterRequired(t *testing.T) {
	m := valid()
	m.PrefixRoutes = &redis_proxyv3.RedisProxy_PrefixRoutes{
		CatchAllRoute: &redis_proxyv3.RedisProxy_PrefixRoutes_Route{Cluster: ""}, // present, empty cluster
	}
	_, err := parseConfig(m)
	if err == nil || err.Error() != errCatchAllClusterRequired {
		t.Fatalf("err = %v, want %q", err, errCatchAllClusterRequired)
	}
}

func TestParseConfig_UnknownClusterTolerated(t *testing.T) {
	// AMEND-R2 arm C: an unknown catch_all cluster name does NOT reject at parse —
	// it is resolved lazily at Handle. parseConfig stores the name verbatim.
	m := valid()
	m.PrefixRoutes = catchAll("nonexistent_cluster")
	cfg, err := parseConfig(m)
	if err != nil {
		t.Fatalf("parseConfig must tolerate an unknown cluster at parse: %v", err)
	}
	if cfg.catchAllCluster != "nonexistent_cluster" {
		t.Errorf("catchAllCluster = %q, want nonexistent_cluster", cfg.catchAllCluster)
	}
}
```

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: parseConfig` / the error constants)

Run: `go test ./internal/filter/network/redisproxy/ -run TestParseConfig -v`
Expected: build error.

- [ ] **Step 3: Write `config.go`**

`config.go`:
```go
package redisproxy

import (
	"errors"
	"time"

	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"

	"github.com/esalaine/envoy-go/internal/stats"
)

// PARSE-REJECT arms (ADR-0080 byte-stable; SPEC §6; D-S32.1-3). The error prefix
// for all redisproxy arms is "redis_proxy: " (mirrors kafka_broker: / mongo_proxy:
// / zookeeper_proxy:). These strings are byte-stable from this commit forward — DO
// NOT CHANGE. Only errStatPrefixRequired is fixture-proven (the 0056 arm, §6.1);
// the rest are unit-test-only at 32.1 (D-P32-5; the kafka 0054 / mongo 0050
// precedent). The boot-reject differential matches a per-side stderr SUBSTRING,
// not exact cross-impl equality (§6).
const (
	errStatPrefixRequired      = "redis_proxy: stat_prefix is required"
	errStatPrefixInvalid       = "redis_proxy: stat_prefix is not a valid metric name"
	errSettingsRequired        = "redis_proxy: settings: value is required"
	errOpTimeoutRequired       = "redis_proxy: settings.op_timeout: value is required"
	errNoUpstream              = "redis_proxy: cannot configure a redis-proxy without any upstream"
	errCatchAllClusterRequired = "redis_proxy: catch_all_route.cluster is required"
)

// compiledConfig is the boot-parsed, per-listener-shared redis_proxy config
// (ADR-0079 two-step factory). The roster (stats.go) is NOT here — it attaches to
// the filter struct at NewFactory (filter.go). The catch_all cluster is stored by
// NAME (resolved lazily at Handle, tolerant of an unknown cluster — §3.3).
type compiledConfig struct {
	statPrefix      string
	catchAllCluster string        // the catch_all_route.cluster name (lazy-resolved at Handle)
	opTimeout       time.Duration // parsed + stored; CONSUMPTION (bounding the round-trip) is 32.2
}

// parseConfig validates + compiles the proto (SPEC §5 roster, inherited). PGV
// mirrors: stat_prefix required + IsValidName-guarded; settings required;
// settings.op_timeout required; catch_all_route.cluster required when present. A
// runtime reject fires when NEITHER catch_all_route NOR routes[] supplies an
// upstream. An unknown catch_all cluster is TOLERATED (AMEND-R2 arm C). All
// deferred fields (faults, downstream_auth_*, routes[], read_policy, the rest of
// ConnPoolSettings) parse-accept standalone (SPEC §6.3). Pure — no registry.
func parseConfig(msg *redis_proxyv3.RedisProxy) (*compiledConfig, error) {
	sp := msg.GetStatPrefix()
	if sp == "" {
		return nil, errors.New(errStatPrefixRequired)
	}
	if !stats.IsValidName(sp) {
		// the cluster manager.go:205 / reference_dynamic_stat_name_charset_guard
		// precedent: a metric-name-invalid prefix → reject at the user-input
		// boundary (NewCounterIfAbsent would otherwise PANIC at NewFactory).
		return nil, errors.New(errStatPrefixInvalid)
	}
	s := msg.GetSettings()
	if s == nil {
		return nil, errors.New(errSettingsRequired)
	}
	ot := s.GetOpTimeout()
	if ot == nil {
		return nil, errors.New(errOpTimeoutRequired)
	}
	cluster, err := upstreamCluster(msg.GetPrefixRoutes())
	if err != nil {
		return nil, err
	}
	return &compiledConfig{statPrefix: sp, catchAllCluster: cluster, opTimeout: ot.AsDuration()}, nil
}

// upstreamCluster extracts the catch_all_route.cluster name. Returns errNoUpstream
// when prefix_routes is omitted/empty (neither catch_all_route nor routes[]
// supplies an upstream) and errCatchAllClusterRequired when a catch_all_route is
// present with an empty cluster. routes[]-only configs are 32.2 (SPEC §2.4);
// at 32.1 only catch_all_route supplies the upstream.
func upstreamCluster(pr *redis_proxyv3.RedisProxy_PrefixRoutes) (string, error) {
	if pr == nil || (pr.GetCatchAllRoute() == nil && len(pr.GetRoutes()) == 0) {
		return "", errors.New(errNoUpstream)
	}
	car := pr.GetCatchAllRoute()
	if car == nil {
		// routes[]-only: parse-accepted but unconsumed at 32.1; with no
		// catch_all there is no 32.1 upstream → treat as no-upstream.
		return "", errors.New(errNoUpstream)
	}
	if car.GetCluster() == "" {
		return "", errors.New(errCatchAllClusterRequired)
	}
	return car.GetCluster(), nil
}
```

- [ ] **Step 4: Run the config tests — expect PASS**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestParseConfig|TestTypeURL' -v`
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
go test ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/ && git commit -m "phase 32.1 IMPL Task 3: config.go parse (PGV arms + runtime no-upstream + unknown-cluster tolerance + IsValidName guard + byte-stable reject constants)"
```
Expected: clean; tests PASS.

---

## Task 4: PARSE-REJECT byte-stable table test

**Files:**
- Test: `internal/filter/network/redisproxy/config_test.go` (append)

> The reject CONSTANTS already landed at Task 3; this task adds the dedicated byte-stable guard test (ADR-0080; D-S32.1-3). No production change.

- [ ] **Step 1: Append the byte-stable table test**

Append to `config_test.go`:
```go
func TestParseRejectConstants_ByteStable(t *testing.T) {
	// ADR-0080 byte-stable wording guard (D-S32.1-3). DO NOT update these to match
	// a code change — a mismatch means the production wording regressed. Every arm
	// carries the "redis_proxy: " prefix (the kafka_broker:/mongo_proxy: precedent).
	want := map[string]string{
		"stat_prefix":         "redis_proxy: stat_prefix is required",
		"stat_prefix_invalid": "redis_proxy: stat_prefix is not a valid metric name",
		"settings":            "redis_proxy: settings: value is required",
		"op_timeout":          "redis_proxy: settings.op_timeout: value is required",
		"no_upstream":         "redis_proxy: cannot configure a redis-proxy without any upstream",
		"catch_all_cluster":   "redis_proxy: catch_all_route.cluster is required",
	}
	got := map[string]string{
		"stat_prefix": errStatPrefixRequired, "stat_prefix_invalid": errStatPrefixInvalid,
		"settings": errSettingsRequired, "op_timeout": errOpTimeoutRequired,
		"no_upstream": errNoUpstream, "catch_all_cluster": errCatchAllClusterRequired,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s arm = %q, want %q", k, got[k], w)
		}
	}
}
```

- [ ] **Step 2: Run it — expect PASS** (the constants match)

Run: `go test ./internal/filter/network/redisproxy/ -run TestParseRejectConstants -v`
Expected: PASS.

- [ ] **Step 3: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/ && git commit -m "phase 32.1 IMPL Task 4: TestParseRejectConstants_ByteStable (ADR-0080 byte-stable wording guard, redis_proxy: arms)"
```

---

## Task 5: `resp.go` part 1 — request decode (inline + array-of-bulk) + streaming reassembly + overflow guards

**Files:**
- Create: `internal/filter/network/redisproxy/resp.go`
- Test: `internal/filter/network/redisproxy/resp_test.go`

> Mirrors upstream `codec_impl.cc` framing exactly (SPEC §3.5 / parent §11.5 — `reference_wire_format_both_sides_see_same_bytes`). All reads are `*bufio.Reader`-based and BLOCK on partial frames (the terminal owns the conn — D-P32-4 streaming-reader model). `decodeRequest` returns the UPPERCASED command name (for dispatch) + the RAW frame bytes (forwarded VERBATIM upstream when proxied). The decoder NEVER panics on arbitrary bytes (proven by unit tests; the `FuzzRESPDecode` fuzzer is 32.2). Overflow guards: `maxBulkLen` / `maxArrayLen` checked BEFORE any allocation.

- [ ] **Step 1: Write the failing request-decode tests**

`resp_test.go`:
```go
package redisproxy

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func reqReader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

func TestDecodeRequest_InlinePing(t *testing.T) {
	r := reqReader("PING\r\n")
	cmd, raw, err := decodeRequest(r)
	if err != nil {
		t.Fatalf("decodeRequest: %v", err)
	}
	if cmd != "PING" {
		t.Errorf("cmd = %q, want PING", cmd)
	}
	if string(raw) != "PING\r\n" {
		t.Errorf("raw = %q, want %q", raw, "PING\r\n")
	}
}

func TestDecodeRequest_InlineBareLF(t *testing.T) {
	// inline accepts a bare \n terminator (no \r).
	cmd, raw, err := decodeRequest(reqReader("PING\n"))
	if err != nil || cmd != "PING" || string(raw) != "PING\n" {
		t.Fatalf("cmd=%q raw=%q err=%v", cmd, raw, err)
	}
}

func TestDecodeRequest_ArrayOfBulk(t *testing.T) {
	wire := "*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"
	cmd, raw, err := decodeRequest(reqReader(wire))
	if err != nil {
		t.Fatalf("decodeRequest: %v", err)
	}
	if cmd != "SET" {
		t.Errorf("cmd = %q, want SET", cmd)
	}
	if string(raw) != wire {
		t.Errorf("raw = %q, want verbatim %q", raw, wire)
	}
}

func TestDecodeRequest_CaseInsensitiveCommand(t *testing.T) {
	cmd, _, err := decodeRequest(reqReader("*1\r\n$4\r\nping\r\n"))
	if err != nil || cmd != "PING" {
		t.Fatalf("cmd = %q err=%v, want PING (uppercased)", cmd, err)
	}
}

func TestDecodeRequest_EOFAtFrameBoundary(t *testing.T) {
	// An empty reader → clean io.EOF (the connection ended between frames).
	_, _, err := decodeRequest(reqReader(""))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want io.EOF at the frame boundary", err)
	}
}

func TestDecodeRequest_PartialFrameBlocksThenResumes(t *testing.T) {
	// A pipe that delivers a request in two writes proves block-and-resume: the
	// decoder blocks mid-frame on the first short read and completes on the second.
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("*1\r\n$4\r\nPI"))
		_, _ = pw.Write([]byte("NG\r\n"))
		_ = pw.Close()
	}()
	cmd, _, err := decodeRequest(bufio.NewReader(pr))
	if err != nil || cmd != "PING" {
		t.Fatalf("cmd=%q err=%v, want PING across two reads", cmd, err)
	}
}

func TestDecodeRequest_MalformedNeverPanics(t *testing.T) {
	bad := []string{
		"*abc\r\n",              // non-numeric array count
		"*2\r\n$3\r\nSET\r\n",   // truncated mid-array (declares 2, supplies 1) → unexpected EOF
		"*1\r\n$-5\r\n",         // negative non(-1) bulk length
		"*1\r\n$99999999999\r\n", // overflow bulk length (> maxBulkLen)
		"$3\r\nfoo\r\n",         // a reply type byte where a request is expected
		"*1\r\n#bad\r\n",        // bad bulk type marker
	}
	for _, s := range bad {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("decodeRequest(%q) PANICKED: %v", s, p)
				}
			}()
			if _, _, err := decodeRequest(reqReader(s)); err == nil {
				t.Errorf("decodeRequest(%q) = nil error, want a decode error", s)
			}
		}()
	}
}
```

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: decodeRequest`)

Run: `go test ./internal/filter/network/redisproxy/ -run TestDecodeRequest -v`
Expected: build error.

- [ ] **Step 3: Write `resp.go` part 1**

`resp.go`:
```go
package redisproxy

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
)

// Overflow guards (checked BEFORE any allocation; the upstream proto_max_bulk_size
// analogue). A declared length beyond these → a decode error, never an allocation.
const (
	maxBulkLen  = 512 * 1024 * 1024 // 512 MiB
	maxArrayLen = 1024 * 1024       // 1 Mi elements
)

// errProtocol is the catch-all RESP framing error. Wording is internal-only
// (never byte-compared) — the differential proof is the downstream_cx_protocol_
// error COUNT (32.2), not the message.
var errProtocol = errors.New("redis_proxy: resp: protocol error")

// unexpected maps a mid-frame io.EOF to io.ErrUnexpectedEOF (a transport/protocol
// error — the connection ended INSIDE a frame). A frame-boundary io.EOF is
// returned verbatim by decodeRequest's first read (clean connection end).
func unexpected(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

// readLine reads through the next '\n', appends the raw bytes (incl. the
// terminator) to raw, and returns the line WITHOUT the trailing "\r\n" (or bare
// "\n"). Blocks until a full line arrives (partial-frame reassembly).
func readLine(r *bufio.Reader, raw *bytes.Buffer) (string, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return "", err
	}
	raw.Write(line)
	n := len(line) - 1 // drop '\n'
	if n > 0 && line[n-1] == '\r' {
		n-- // drop '\r'
	}
	return string(line[:n]), nil
}

// readBulk reads one RESP bulk string "$<len>\r\n<len bytes>\r\n", appends the
// raw bytes to raw, and returns the payload. Used by the request array path.
func readBulk(r *bufio.Reader, raw *bytes.Buffer) ([]byte, error) {
	header, err := readLine(r, raw)
	if err != nil {
		return nil, unexpected(err)
	}
	if len(header) == 0 || header[0] != '$' {
		return nil, errProtocol
	}
	n, err := strconv.Atoi(header[1:])
	if err != nil || n < 0 || n > maxBulkLen {
		return nil, errProtocol
	}
	body := make([]byte, n+2) // payload + trailing "\r\n"
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, unexpected(err)
	}
	raw.Write(body)
	if body[n] != '\r' || body[n+1] != '\n' {
		return nil, errProtocol
	}
	return body[:n], nil
}

// decodeRequest reads ONE request frame (inline OR array-of-bulk-strings) from r
// and returns the UPPERCASED command name (for dispatch) + the RAW frame bytes
// (forwarded VERBATIM upstream when proxied). Blocks on partial frames. A
// frame-boundary io.EOF is returned verbatim (clean connection end).
func decodeRequest(r *bufio.Reader) (cmd string, raw []byte, err error) {
	p, err := r.Peek(1)
	if err != nil {
		return "", nil, err // io.EOF here = clean end between frames
	}
	var buf bytes.Buffer
	if p[0] != '*' {
		// inline command: a space-separated token line.
		line, err := readLine(r, &buf)
		if err != nil {
			return "", nil, unexpected(err)
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return "", nil, errProtocol
		}
		return strings.ToUpper(fields[0]), buf.Bytes(), nil
	}
	// array-of-bulk: "*<n>\r\n" then n × "$<len>\r\n<bytes>\r\n".
	header, err := readLine(r, &buf)
	if err != nil {
		return "", nil, unexpected(err)
	}
	n, err := strconv.Atoi(header[1:])
	if err != nil || n <= 0 || n > maxArrayLen {
		return "", nil, errProtocol
	}
	for i := 0; i < n; i++ {
		bulk, err := readBulk(r, &buf)
		if err != nil {
			return "", nil, err
		}
		if i == 0 {
			cmd = strings.ToUpper(string(bulk))
		}
	}
	return cmd, buf.Bytes(), nil
}
```

- [ ] **Step 4: Run the request-decode tests — expect PASS**

Run: `go test ./internal/filter/network/redisproxy/ -run TestDecodeRequest -v`
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/ && git commit -m "phase 32.1 IMPL Task 5: resp.go part 1 — request decode (inline + array-of-bulk) + streaming reassembly + overflow guards + no-panic"
```

---

## Task 6: `resp.go` part 2 — reply decode (all value types + null sentinels + nested arrays) + local-reply encode constants

**Files:**
- Modify: `internal/filter/network/redisproxy/resp.go`
- Test: `internal/filter/network/redisproxy/resp_test.go` (append)

> `decodeReply` reads ONE complete reply frame of ANY value type (simple-string `+` / error `-` / integer `:` / bulk `$` incl. `$-1` null / array `*` incl. `*-1` null + nested arrays) and returns its RAW bytes (forwarded VERBATIM downstream — the byte-equivalence prong §8.1.1). It parses only enough to find the frame boundary; it does NOT re-encode. The encode constants are the byte-stable local replies (PING `+PONG`, the AUTH no-password error).

- [ ] **Step 1: Write the failing reply-decode + encode tests**

Append to `resp_test.go`:
```go
func replyRaw(t *testing.T, s string) string {
	t.Helper()
	raw, err := decodeReply(reqReader(s))
	if err != nil {
		t.Fatalf("decodeReply(%q): %v", s, err)
	}
	return string(raw)
}

func TestDecodeReply_ValueTypes(t *testing.T) {
	for _, wire := range []string{
		"+OK\r\n",                 // simple string
		"-ERR bad\r\n",            // error
		":42\r\n",                 // integer
		"$3\r\nbar\r\n",           // bulk string
		"$-1\r\n",                 // null bulk
		"*-1\r\n",                 // null array
		"*2\r\n$3\r\nfoo\r\n:7\r\n", // array with mixed elements
		"*2\r\n*1\r\n+a\r\n$1\r\nb\r\n", // NESTED array
	} {
		if got := replyRaw(t, wire); got != wire {
			t.Errorf("decodeReply(%q) raw = %q, want verbatim", wire, got)
		}
	}
}

func TestDecodeReply_MalformedNeverPanics(t *testing.T) {
	bad := []string{"%bad\r\n", "$\r\n", ":x\r\n", "*1\r\n", "$5\r\nab\r\n"}
	for _, s := range bad {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("decodeReply(%q) PANICKED: %v", s, p)
				}
			}()
			if _, err := decodeReply(reqReader(s)); err == nil {
				t.Errorf("decodeReply(%q) = nil error, want a decode error", s)
			}
		}()
	}
}

func TestLocalReplyConstants_ByteStable(t *testing.T) {
	if string(respPong) != "+PONG\r\n" {
		t.Errorf("respPong = %q, want +PONG\\r\\n", respPong)
	}
	if string(respAuthNoPassword) != "-ERR Client sent AUTH, but no password is set\r\n" {
		t.Errorf("respAuthNoPassword = %q", respAuthNoPassword)
	}
}
```

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: decodeReply` / `respPong`)

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestDecodeReply|TestLocalReply' -v`
Expected: build error.

- [ ] **Step 3: Add `decodeReply` + the encode constants to `resp.go`**

Append to `resp.go`:
```go
// Local-reply byte-stable constants (the +PONG / -ERR wording from parent
// §11.5/§11.6). DO NOT CHANGE — these bytes are asserted byte-identical
// cross-side (the byte-equivalence prong, §8.1.1).
var (
	respPong           = []byte("+PONG\r\n")
	respAuthNoPassword = []byte("-ERR Client sent AUTH, but no password is set\r\n")
)

// decodeReply reads ONE complete reply frame (any value type incl. null sentinels
// + nested arrays) from r and returns its RAW bytes (forwarded VERBATIM
// downstream — §8.1.1). It parses only enough to find the frame boundary; it does
// not re-encode. Blocks on partial frames; never panics on arbitrary bytes.
func decodeReply(r *bufio.Reader) (raw []byte, err error) {
	var buf bytes.Buffer
	if err := decodeReplyInto(r, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeReplyInto reads one reply frame, appending consumed bytes to raw. Arrays
// recurse (nested-array support).
func decodeReplyInto(r *bufio.Reader, raw *bytes.Buffer) error {
	t, err := r.ReadByte()
	if err != nil {
		return err
	}
	raw.WriteByte(t)
	switch t {
	case '+', '-', ':': // simple string / error / integer: one line
		_, err := readLine(r, raw)
		return unexpected(err)
	case '$': // bulk: "$<len>\r\n<bytes>\r\n" OR "$-1\r\n" (null)
		header, err := readLine(r, raw)
		if err != nil {
			return unexpected(err)
		}
		n, err := strconv.Atoi(header)
		if err != nil {
			return errProtocol
		}
		if n == -1 {
			return nil // null bulk
		}
		if n < 0 || n > maxBulkLen {
			return errProtocol
		}
		body := make([]byte, n+2)
		if _, err := io.ReadFull(r, body); err != nil {
			return unexpected(err)
		}
		raw.Write(body)
		if body[n] != '\r' || body[n+1] != '\n' {
			return errProtocol
		}
		return nil
	case '*': // array: "*<n>\r\n" + n elements OR "*-1\r\n" (null)
		header, err := readLine(r, raw)
		if err != nil {
			return unexpected(err)
		}
		n, err := strconv.Atoi(header)
		if err != nil {
			return errProtocol
		}
		if n == -1 {
			return nil // null array
		}
		if n < 0 || n > maxArrayLen {
			return errProtocol
		}
		for i := 0; i < n; i++ {
			if err := decodeReplyInto(r, raw); err != nil {
				return err
			}
		}
		return nil
	default:
		return errProtocol // unknown type byte
	}
}
```

- [ ] **Step 4: Run the reply + encode tests — expect PASS**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestDecodeReply|TestLocalReply' -v`
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
go test ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/ && git commit -m "phase 32.1 IMPL Task 6: resp.go part 2 — reply decode (all value types + null sentinels + nested arrays, raw verbatim) + byte-stable local-reply constants"
```

---

## Task 7: `upstreampool.go` — the upstream connection-pool / cluster-routing seam (ADR-0230)

**Files:**
- Create: `internal/filter/network/upstreampool.go`
- Test: `internal/filter/network/upstreampool_test.go`

> The framework's SIXTH structural extension, in the EXISTING `internal/filter/network/` package (NOT a new package). The seam consumes a DIAL CLOSURE (`UpstreamDialFunc`), NOT `*cluster.Cluster` — so `internal/filter/network` gains NO `internal/cluster` import (the `upstreamcluster.go` decoupling discipline §4.2). One-conn-per-downstream + synchronous single-flight (AMEND-R8 / D-P32-3). The seam knows NOTHING about RESP framing (the codec reads reply frames from `Reader()`).

- [ ] **Step 1: Write the failing seam tests**

`upstreampool_test.go`:
```go
package network

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
)

// echoPipe returns a dial closure yielding one end of a net.Pipe; the other end
// is served by serve. dialed counts dials; reqs counts onRequest hook calls.
func TestUpstreamConn_LazyDial_RoundTrip(t *testing.T) {
	var dials, reqs int
	dial := func(ctx context.Context) (net.Conn, error) {
		dials++
		c, s := net.Pipe()
		go func() { // a trivial server: read a line, reply "+OK\r\n"
			br := bufio.NewReader(s)
			for {
				if _, err := br.ReadBytes('\n'); err != nil {
					return
				}
				_, _ = s.Write([]byte("+OK\r\n"))
			}
		}()
		return c, nil
	}
	u := NewUpstreamConn(dial, func() { reqs++ })
	defer u.Close()

	if dials != 0 {
		t.Fatalf("dials = %d before first Send, want 0 (lazy)", dials)
	}
	if err := u.Send(context.Background(), []byte("PING\r\n")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if dials != 1 || reqs != 1 {
		t.Fatalf("after Send: dials=%d reqs=%d, want 1/1", dials, reqs)
	}
	line, err := u.Reader().ReadBytes('\n')
	if err != nil || string(line) != "+OK\r\n" {
		t.Fatalf("reply = %q err=%v, want +OK", line, err)
	}
	// a SECOND Send reuses the SAME conn (no re-dial) and fires the hook again.
	if err := u.Send(context.Background(), []byte("GET x\r\n")); err != nil {
		t.Fatalf("Send 2: %v", err)
	}
	if dials != 1 || reqs != 2 {
		t.Fatalf("after 2nd Send: dials=%d reqs=%d, want 1/2", dials, reqs)
	}
}

func TestUpstreamConn_DialError(t *testing.T) {
	want := errors.New("dial boom")
	u := NewUpstreamConn(func(context.Context) (net.Conn, error) { return nil, want }, nil)
	if err := u.Send(context.Background(), []byte("X\r\n")); !errors.Is(err, want) {
		t.Fatalf("Send err = %v, want %v", err, want)
	}
}

func TestUpstreamConn_CloseIdempotent_NoDial(t *testing.T) {
	// Close before any Send (a PING-only downstream that never proxied) is a no-op.
	u := NewUpstreamConn(func(context.Context) (net.Conn, error) {
		t.Fatal("dial must NOT happen on a Send-less connection")
		return nil, nil
	}, nil)
	if err := u.Close(); err != nil {
		t.Fatalf("Close (no dial) = %v, want nil", err)
	}
	if err := u.Close(); err != nil {
		t.Fatalf("second Close = %v, want nil (idempotent)", err)
	}
}

func TestUpstreamConn_Race(t *testing.T) {
	// One Handle-style goroutine drives Send→read in a loop (single-flight); the
	// -race detector proves no data race on the seam's per-connection state.
	dial := func(context.Context) (net.Conn, error) {
		c, s := net.Pipe()
		go func() { _, _ = io.Copy(s, s) }() // echo
		return c, nil
	}
	u := NewUpstreamConn(dial, nil)
	defer u.Close()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if err := u.Send(context.Background(), []byte("+x\r\n")); err != nil {
				return
			}
			if _, err := u.Reader().ReadBytes('\n'); err != nil {
				return
			}
		}
	}()
	wg.Wait()
}
```

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: NewUpstreamConn`)

Run: `go test ./internal/filter/network/ -run TestUpstreamConn -v`
Expected: build error.

- [ ] **Step 3: Write `upstreampool.go`**

`upstreampool.go`:
```go
// internal/filter/network/upstreampool.go (ADR-0230)
//
// The upstream connection-pool / cluster-routing seam — the framework's SIXTH
// structural extension. It lets a network.TerminalFilter ACTIVELY manage an
// upstream connection (dial + per-downstream lifecycle + FIFO/positional reply
// correlation) rather than observe a tcp_proxy-terminated one. Redis-scoped (no
// thrift generalization; thrift reuses/extends it at its phase). It builds ON
// TerminalFilter.Handle (UNCHANGED) and reuses cluster.Cluster.Dial via a DIAL
// CLOSURE — keeping internal/filter/network free of an internal/cluster import
// (the upstreamcluster.go decoupling discipline).
//
// 32.1 MVP: one upstream conn per downstream conn (AMEND-R8); synchronous
// single-flight (a degenerate depth-1 FIFO/positional pending queue — RESP
// carries no correlation id). The shared per-host pool + a >1-depth pending queue
// + the two-goroutine pipelined model + the ADR-0223 per-conn mutex are DEFERRED
// (consume-at-second-consumer — thrift / a latency-or-fan-out sub-phase extends).

package network

import (
	"bufio"
	"context"
	"net"
)

// UpstreamDialFunc resolves + dials one upstream connection. redisproxy supplies
// a closure over the boot-resolved cluster's Cluster.Dial (Endpoint discarded) —
// keeping internal/filter/network free of an internal/cluster import.
type UpstreamDialFunc func(ctx context.Context) (net.Conn, error)

// UpstreamConn is one downstream connection's dedicated upstream connection, with
// FIFO/positional reply correlation. The MVP is one-conn-per-downstream +
// synchronous single-flight; the shared pool + deep queue + pipelined model are
// DEFERRED (see the file header). Not safe for concurrent Send (the single-flight
// pump is single-goroutine — the 32.1 contract; the ADR-0223 mutex arrives with
// the deferred pipelined model).
type UpstreamConn struct {
	dial      UpstreamDialFunc
	onRequest func() // fired per Send (the cluster.IncUpstreamRqTotal hook); nil-tolerant
	conn      net.Conn
	r         *bufio.Reader // bufio.Reader over conn; the codec decodes reply frames from it
}

// NewUpstreamConn binds a dial closure + a per-proxied-request hook (the filter
// passes cluster.IncUpstreamRqTotal — the AMEND-R6 upstream_rq_total Inc; pass
// nil for none). No dial happens until the first Send.
func NewUpstreamConn(dial UpstreamDialFunc, onRequest func()) *UpstreamConn {
	return &UpstreamConn{dial: dial, onRequest: onRequest}
}

// Send forwards one request's raw bytes upstream. On the FIRST call it lazily
// dials (that first Dial Incs cluster.upstream_cx_total/active); every call fires
// the onRequest hook (→ cluster.upstream_rq_total) then writes reqBytes.
func (u *UpstreamConn) Send(ctx context.Context, reqBytes []byte) error {
	if u.conn == nil {
		c, err := u.dial(ctx)
		if err != nil {
			return err
		}
		u.conn = c
		u.r = bufio.NewReader(c)
	}
	if u.onRequest != nil {
		u.onRequest()
	}
	_, err := u.conn.Write(reqBytes)
	return err
}

// Reader returns the buffered reader the codec decodes the reply frame from
// (stable across Sends; valid after the first Send). Positional correlation: the
// synchronous pump reads exactly one reply per Send (depth-1 FIFO).
func (u *UpstreamConn) Reader() *bufio.Reader { return u.r }

// Close closes the upstream conn (idempotent; a no-op if never dialed — a
// PING/AUTH-only downstream never dials). The Cluster.Dial connWithGauge Decs
// upstream_cx_active once. Called from the filter's Handle defer.
func (u *UpstreamConn) Close() error {
	if u.conn == nil {
		return nil
	}
	c := u.conn
	u.conn = nil
	return c.Close()
}
```

- [ ] **Step 4: Run the seam tests — expect PASS (incl. `-race`)**

Run: `go test ./internal/filter/network/ -run TestUpstreamConn -race -v`
Expected: all PASS, no race.

- [ ] **Step 5: Assert NO `internal/cluster` import leaked into the core package**

Run:
```bash
go list -deps ./internal/filter/network/ | grep -q "envoy-go/internal/cluster" && echo "LEAK: cluster imported" || echo "CLEAN: no cluster import"
```
Expected: `CLEAN: no cluster import` (the seam consumes a closure; the decoupling holds). Add a guard comment in `upstreampool_test.go` recording this invariant.

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/
golangci-lint run ./internal/filter/network/...
git add internal/filter/network/upstreampool.go internal/filter/network/upstreampool_test.go && git commit -m "phase 32.1 IMPL Task 7: upstreampool.go — the ADR-0230 upstream-pool seam (dial closure + lazy dial + single-flight Send/Reader/Close; no internal/cluster import)"
```

---

## Task 8: `commands.go` — the PING/AUTH local-reply set + the local-vs-proxy dispatch

**Files:**
- Create: `internal/filter/network/redisproxy/commands.go`
- Test: `internal/filter/network/redisproxy/commands_test.go`

> AMEND-R5 / D-P32-6 (32.1 subset). PING → `+PONG\r\n` (the argument, if any, is IGNORED — does NOT echo); AUTH (no password configured) → `-ERR Client sent AUTH, but no password is set\r\n`; any other command → proxied. ECHO/TIME/QUIT/HELLO are 32.2 follow-ons. Command matching is case-insensitive (the decoded name is already uppercased by `decodeRequest`).

- [ ] **Step 1: Write the failing dispatch tests**

`commands_test.go`:
```go
package redisproxy

import "testing"

func TestIsLocalReply(t *testing.T) {
	for _, c := range []string{"PING", "AUTH"} {
		if !isLocalReply(c) {
			t.Errorf("isLocalReply(%q) = false, want true", c)
		}
	}
	for _, c := range []string{"SET", "GET", "ECHO", "TIME", "QUIT", "HELLO", ""} {
		if isLocalReply(c) {
			t.Errorf("isLocalReply(%q) = true, want false (proxied or 32.2)", c)
		}
	}
}

func TestLocalReply_Bytes(t *testing.T) {
	if got := localReply("PING"); string(got) != "+PONG\r\n" {
		t.Errorf("localReply(PING) = %q", got)
	}
	if got := localReply("AUTH"); string(got) != "-ERR Client sent AUTH, but no password is set\r\n" {
		t.Errorf("localReply(AUTH) = %q", got)
	}
	if got := localReply("SET"); got != nil {
		t.Errorf("localReply(SET) = %q, want nil (proxied)", got)
	}
}
```

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: isLocalReply`)

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestIsLocalReply|TestLocalReply_Bytes' -v`
Expected: build error.

- [ ] **Step 3: Write `commands.go`**

`commands.go`:
```go
package redisproxy

// isLocalReply reports whether cmd (UPPERCASED by decodeRequest) is answered
// in-filter with zero upstream traffic. The 32.1 set is PING + AUTH (AMEND-R5 /
// D-P32-6 32.1 subset); ECHO/TIME/QUIT/HELLO are 32.2 follow-ons.
func isLocalReply(cmd string) bool {
	switch cmd {
	case "PING", "AUTH":
		return true
	default:
		return false
	}
}

// localReply returns the byte-stable local reply for a local-reply command, or
// nil for a proxied command. PING ignores its argument (does NOT echo — the
// reference behavior, parent §11.6). AUTH answers the no-password-set error (the
// 32.1 posture: no downstream_auth_password is consumed — SPEC §2.4).
func localReply(cmd string) []byte {
	switch cmd {
	case "PING":
		return respPong
	case "AUTH":
		return respAuthNoPassword
	default:
		return nil
	}
}
```

- [ ] **Step 4: Run the dispatch tests — expect PASS**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestIsLocalReply|TestLocalReply_Bytes' -v`
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/ && git commit -m "phase 32.1 IMPL Task 8: commands.go — PING/AUTH local-reply set + local-vs-proxy dispatch (AMEND-R5)"
```

---

## Task 9: `stats.go` — the 10-name eager downstream roster + inc accessors

**Files:**
- Create: `internal/filter/network/redisproxy/stats.go`
- Test: `internal/filter/network/redisproxy/stats_test.go`

> D-P32-1 EAGER: the 10 fixed names (6 downstream counters + 4 gauges) are created via `NewCounterIfAbsent`/`NewGaugeIfAbsent` at `NewFactory` — idempotent across listeners sharing a `stat_prefix` (the kafka/mongo precedent). At 32.1 only the 4 cx/rq counters increment; `drain_close`/`protocol_error` + all 4 gauges' inc/dec are 32.2 (created-not-incremented; §7.2). The `redisStats` map-keyed shape mirrors `kafkabroker/stats.go`.

- [ ] **Step 1: Write the failing roster tests**

`stats_test.go`:
```go
package redisproxy

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestStatRoster32_1_MatchesUpstream(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	if got := len(rs.counters); got != 6 {
		t.Fatalf("counter roster size = %d, want 6", got)
	}
	if got := len(rs.gauges); got != 4 {
		t.Fatalf("gauge roster size = %d, want 4", got)
	}
	// The 10 names match upstream ALL_REDIS_PROXY_STATS (the 32.1 subset, R2).
	for _, suf := range []string{
		"downstream_cx_total", "downstream_cx_drain_close", "downstream_cx_protocol_error",
		"downstream_cx_rx_bytes_total", "downstream_cx_tx_bytes_total", "downstream_rq_total",
	} {
		c, ok := rs.counters[suf]
		if !ok {
			t.Errorf("counter %q absent from eager roster", suf)
			continue
		}
		if c.Load() != 0 {
			t.Errorf("counter %q = %d at creation, want 0", suf, c.Load())
		}
	}
	for _, suf := range []string{
		"downstream_cx_active", "downstream_cx_rx_bytes_buffered",
		"downstream_cx_tx_bytes_buffered", "downstream_rq_active",
	} {
		if _, ok := rs.gauges[suf]; !ok {
			t.Errorf("gauge %q absent from eager roster", suf)
		}
	}
}

func TestStatRoster32_1_Idempotent(t *testing.T) {
	reg := stats.NewRegistry()
	a := newRedisStats(reg, "rp")
	b := newRedisStats(reg, "rp") // a second listener sharing the prefix — no panic, SAME instances
	if a.counters["downstream_cx_total"] != b.counters["downstream_cx_total"] {
		t.Fatal("shared stat_prefix must share the same counter instances")
	}
}

func TestStatRoster32_1_IncAccessors(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	rs.incCxTotal()
	rs.incRqTotal()
	rs.addRxBytes(7)
	rs.addTxBytes(3)
	if rs.counters["downstream_cx_total"].Load() != 1 {
		t.Error("incCxTotal")
	}
	if rs.counters["downstream_rq_total"].Load() != 1 {
		t.Error("incRqTotal")
	}
	if rs.counters["downstream_cx_rx_bytes_total"].Load() != 7 {
		t.Error("addRxBytes")
	}
	if rs.counters["downstream_cx_tx_bytes_total"].Load() != 3 {
		t.Error("addTxBytes")
	}
}
```

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: newRedisStats`)

Run: `go test ./internal/filter/network/redisproxy/ -run TestStatRoster -v`
Expected: build error.

- [ ] **Step 3: Write `stats.go`**

`stats.go`:
```go
package redisproxy

import "github.com/esalaine/envoy-go/internal/stats"

// counterSuffixes / gaugeSuffixes are the 32.1 subset of the parent §7.2 fixed-15
// roster: 6 downstream counters + 4 downstream gauges = 10 names under
// redis.<stat_prefix>. (the 2 splitter.* + 3 REDIS_CLUSTER_STATS + the EAGER
// per-command table are 32.2). Pinned name-for-name against ALL_REDIS_PROXY_STATS.
var counterSuffixes = []string{
	"downstream_cx_total",
	"downstream_cx_drain_close",
	"downstream_cx_protocol_error",
	"downstream_cx_rx_bytes_total",
	"downstream_cx_tx_bytes_total",
	"downstream_rq_total",
}

var gaugeSuffixes = []string{
	"downstream_cx_active",
	"downstream_cx_rx_bytes_buffered",
	"downstream_cx_tx_bytes_buffered",
	"downstream_rq_active",
}

// redisStats holds the EAGER 10-name 32.1 roster, created under redis.<prefix>.
// via NewCounterIfAbsent / NewGaugeIfAbsent — post-Freeze-permitted and
// idempotent across listeners sharing a stat_prefix (the kafka/mongo precedent).
type redisStats struct {
	prefix   string
	counters map[string]*stats.Counter
	gauges   map[string]*stats.Gauge
}

// newRedisStats eagerly creates the 10 fixed names under redis.<statPrefix>.
// (D-P32-1). The 4 gauges are created but NOT incremented at 32.1 (inc/dec is
// 32.2); the cx/rq counters increment in the Handle pump (filter.go).
func newRedisStats(reg *stats.Registry, statPrefix string) *redisStats {
	rs := &redisStats{
		prefix:   "redis." + statPrefix + ".",
		counters: make(map[string]*stats.Counter, len(counterSuffixes)),
		gauges:   make(map[string]*stats.Gauge, len(gaugeSuffixes)),
	}
	for _, suf := range counterSuffixes {
		rs.counters[suf] = reg.NewCounterIfAbsent(rs.prefix + suf)
	}
	for _, suf := range gaugeSuffixes {
		rs.gauges[suf] = reg.NewGaugeIfAbsent(rs.prefix + suf)
	}
	return rs
}

// The 32.1-incremented subset (§7.2). drain_close / protocol_error + the 4
// gauges' inc/dec are 32.2.
func (rs *redisStats) incCxTotal()     { rs.counters["downstream_cx_total"].Inc() }
func (rs *redisStats) incRqTotal()     { rs.counters["downstream_rq_total"].Inc() }
func (rs *redisStats) addRxBytes(n int) { rs.counters["downstream_cx_rx_bytes_total"].Add(uint64(n)) }
func (rs *redisStats) addTxBytes(n int) { rs.counters["downstream_cx_tx_bytes_total"].Add(uint64(n)) }
```

- [ ] **Step 4: Run the roster tests — expect PASS**

Run: `go test ./internal/filter/network/redisproxy/ -run TestStatRoster -v`
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/ && git commit -m "phase 32.1 IMPL Task 9: stats.go — the 10-name eager downstream roster (6 counters + 4 gauges) + TestStatRoster32_1_MatchesUpstream + the 32.1 inc accessors"
```

---

## Task 10: `filter.go` — `NewFactory` + the `TerminalFilter.Handle` command→reply pump

**Files:**
- Create: `internal/filter/network/redisproxy/filter.go`
- Test: `internal/filter/network/redisproxy/filter_test.go`

> The capstone: ties config + resp + commands + stats + the seam. `NewFactory(cm, reg)` parses/validates ONCE at boot (ADR-0079), creates the roster, and yields the SHARED `*filter` per accepted connection (redis_proxy is conn-stateless at the struct level — per-connection state is `Handle`'s `bufio.Reader` + `*network.UpstreamConn`). The pump is the SPEC §3.7 shape: decode → PING/AUTH local OR lazy-resolve catch_all + seam round-trip → write reply. NO halt path, NO chain Buffer, NO `OnData` (terminal). Single-goroutine per `Handle`; NO per-connection mutex at 32.1.

- [ ] **Step 1: Write the failing factory + pump tests**

`filter_test.go`:
```go
package redisproxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"testing"
	"time"

	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

func validAny(t *testing.T) *anypb.Any {
	t.Helper()
	msg := &redis_proxyv3.RedisProxy{
		StatPrefix:   "rp",
		Settings:     &redis_proxyv3.RedisProxy_ConnPoolSettings{OpTimeout: durationpb.New(time.Second)},
		PrefixRoutes: &redis_proxyv3.RedisProxy_PrefixRoutes{CatchAllRoute: &redis_proxyv3.RedisProxy_PrefixRoutes_Route{Cluster: "rc"}},
	}
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func TestNewFactory_TypeURLReject(t *testing.T) {
	reg := stats.NewRegistry()
	f := NewFactory(nil, reg)
	bad := &anypb.Any{TypeUrl: "type.googleapis.com/wrong.Type"}
	if _, err := f(bad, network.FactoryCtx{}); err == nil {
		t.Fatal("NewFactory accepted a wrong type_url; want a reject")
	}
}

// newFilterForTest builds a *filter directly with an injected dial closure (the
// cluster.Manager path is exercised in the differential; the unit test injects a
// fake upstream so the pump logic is tested in isolation). The IMPL may expose a
// small package-internal seam (e.g. f.dialOverride) for this; the SPEC §3.7
// production path resolves cm.Get → Cluster.Dial.
//
// This test drives Handle over a net.Pipe downstream + a scripted upstream.

func TestHandle_PingLocalReply_NoUpstream(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	dialed := false
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) {
		dialed = true
		return nil, io.EOF
	})
	down, client := net.Pipe()
	go func() {
		// net.Pipe is unbuffered + synchronous: the client MUST drain the +PONG
		// reply (else Handle's downstream.Write blocks), then Close to deliver the
		// EOF that ends the pump. (Do NOT CloseWrite — net.Pipe conns don't
		// implement it; the assertion would panic.)
		_, _ = client.Write([]byte("PING\r\n"))
		buf := make([]byte, len("+PONG\r\n"))
		_, _ = io.ReadFull(client, buf)
		if string(buf) != "+PONG\r\n" {
			t.Errorf("PING reply = %q, want +PONG\\r\\n", buf)
		}
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	// PING is local → never dials.
	if dialed {
		t.Error("PING dialed upstream; want zero upstream (AMEND-R5)")
	}
	if rs.counters["downstream_cx_total"].Load() != 1 || rs.counters["downstream_rq_total"].Load() != 1 {
		t.Errorf("cx/rq totals = %d/%d, want 1/1", rs.counters["downstream_cx_total"].Load(), rs.counters["downstream_rq_total"].Load())
	}
}

func TestHandle_ProxiedRoundTrip_SetThenGet(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	// scripted upstream: SET → +OK, GET → $3\r\nbar\r\n (positional).
	upSrv, upClient := net.Pipe()
	go func() {
		br := bufio.NewReader(upSrv)
		// read SET (3-bulk array), reply +OK
		_, _, _ = decodeRequest(br)
		_, _ = upSrv.Write([]byte("+OK\r\n"))
		// read GET (2-bulk array), reply bulk bar
		_, _, _ = decodeRequest(br)
		_, _ = upSrv.Write([]byte("$3\r\nbar\r\n"))
	}()
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return upClient, nil })
	down, client := net.Pipe()
	got := make(chan []byte, 1)
	go func() {
		_, _ = client.Write([]byte("*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"))
		_, _ = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
		buf := make([]byte, 64)
		var acc []byte
		for len(acc) < len("+OK\r\n$3\r\nbar\r\n") {
			n, err := client.Read(buf)
			acc = append(acc, buf[:n]...)
			if err != nil {
				break
			}
		}
		_ = client.Close()
		got <- acc
	}()
	f.Handle(context.Background(), down)
	if g := string(<-got); g != "+OK\r\n$3\r\nbar\r\n" {
		t.Errorf("downstream replies = %q, want +OK then $3 bar", g)
	}
	if rs.counters["downstream_rq_total"].Load() != 2 {
		t.Errorf("downstream_rq_total = %d, want 2", rs.counters["downstream_rq_total"].Load())
	}
}

func TestHandle_UnknownClusterGracefulClose(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	// a dial closure that always errors models cm.Get-miss → graceful close, no panic.
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.ErrClosedPipe })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("*1\r\n$3\r\nGET\r\n")) // a proxied command
		_ = client.Close()
	}()
	done := make(chan struct{})
	go func() { f.Handle(context.Background(), down); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Handle hung on an unresolvable upstream; want graceful close")
	}
}

func TestHandle_EOFCleanClose(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.EOF })
	down, client := net.Pipe()
	_ = client.Close() // immediate EOF, no request
	f.Handle(context.Background(), down)
	if rs.counters["downstream_cx_total"].Load() != 1 {
		t.Errorf("downstream_cx_total = %d, want 1 (connection counted even with no request)", rs.counters["downstream_cx_total"].Load())
	}
}
```

> **PLAN note (test seam):** `newTestFilter(rs *redisStats, dial network.UpstreamDialFunc) *filter` is a package-internal test helper that builds a `*filter` whose `Handle` uses the injected dial closure instead of resolving `cm.Get → Cluster.Dial`. The IMPL realizes this by factoring `Handle`'s upstream construction through a `dialFor(ctx) (network.UpstreamDialFunc, func(), error)` method that the production path implements via `cm.Get`/`Cluster.Dial`/`IncUpstreamRqTotal` and the test overrides via an unexported field. Keep the production `Handle` body identical; only the dial source is injectable (the tcpproxy `Handle` is the structural precedent). The cm.Get → Cluster.Dial production path is exercised end-to-end in the `0055` differential (Task 13–14).

- [ ] **Step 2: Run them — expect COMPILE failure** (`undefined: NewFactory` / `newTestFilter`)

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestNewFactory|TestHandle' -v`
Expected: build error.

- [ ] **Step 3: Write `filter.go`**

`filter.go`:
```go
package redisproxy

import (
	"bufio"
	"context"
	"fmt"
	"net"

	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/network"
)

// filter is the boot-parsed, per-listener-shared redis_proxy terminal filter. It
// is conn-stateless at the struct level — per-connection state (the bufio.Reader
// + the *network.UpstreamConn) lives on Handle's stack. The shared instance is
// read-only after boot; the roster counters/gauges are atomic.
type filter struct {
	network.Marker
	cfg *compiledConfig
	st  *redisStats
	cm  *cluster.Manager

	// dialSource resolves the upstream dial closure + the per-request hook for a
	// proxied command. Production wires it to cm.Get(catch_all) → Cluster.Dial /
	// IncUpstreamRqTotal; unit tests inject a fake (newTestFilter). Returns an
	// error on an unresolvable cluster (→ Handle graceful-closes; D-S32.1-6).
	dialSource func(ctx context.Context) (network.UpstreamDialFunc, func(), error)
}

var _ network.TerminalFilter = (*filter)(nil)

// NewFactory returns the redisproxy NetworkFilterFactory. UNLIKE the stats-only
// zookeeper/mongo/kafka factories, redisproxy needs BOTH the cluster Manager (to
// resolve catch_all → *cluster.Cluster at Handle time — the tcp_proxy precedent)
// AND the stats registry (the redis.<sp> roster). Both are closure-captured from
// builtins.Deps (the network FactoryCtx carries neither). Parses + validates ONCE
// at boot (ADR-0079) and creates the 10 fixed stats once per distinct stat_prefix.
func NewFactory(cm *cluster.Manager, reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		if got := tc.GetTypeUrl(); got != TypeURL {
			return nil, fmt.Errorf("redis_proxy: wrong type_url %q (want %q)", got, TypeURL)
		}
		msg := &redis_proxyv3.RedisProxy{}
		if err := tc.UnmarshalTo(msg); err != nil {
			return nil, fmt.Errorf("redis_proxy: unmarshal: %w", err)
		}
		cfg, err := parseConfig(msg)
		if err != nil {
			return nil, err
		}
		st := newRedisStats(reg, cfg.statPrefix)
		f := &filter{cfg: cfg, st: st, cm: cm}
		f.dialSource = f.resolveCatchAll // production dial source
		return func() network.NetworkFilter { return f }, nil
	}
}

// resolveCatchAll resolves catch_all → *cluster.Cluster LAZILY (tolerant of an
// unknown cluster at config time — §3.3). On a miss it returns an error → Handle
// graceful-closes (no -ERR synthesized at 32.1; D-S32.1-6).
func (f *filter) resolveCatchAll(_ context.Context) (network.UpstreamDialFunc, func(), error) {
	cl, ok := f.cm.Get(f.cfg.catchAllCluster)
	if !ok {
		return nil, nil, fmt.Errorf("redis_proxy: catch_all cluster %q not found", f.cfg.catchAllCluster)
	}
	dial := func(ctx context.Context) (net.Conn, error) {
		c, _, err := cl.Dial(ctx) // Endpoint discarded (§4.2)
		return c, err
	}
	return dial, cl.IncUpstreamRqTotal, nil
}

// Handle takes ownership of the downstream connection and runs the RESP
// command→reply pump to connection close (the tcp_proxy.Handle shape). PING/AUTH
// are answered locally (zero upstream); data commands round-trip through a lazily
// dialed one-conn-per-downstream upstream seam with synchronous single-flight
// FIFO/positional reply correlation.
func (f *filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	f.st.incCxTotal()
	dr := bufio.NewReader(downstream)

	var up *network.UpstreamConn
	defer func() {
		if up != nil {
			_ = up.Close()
		}
	}()
	// ensureUpstream lazily prepares the seam on the first proxied command.
	ensureUpstream := func() (*network.UpstreamConn, error) {
		if up != nil {
			return up, nil
		}
		dial, hook, err := f.dialSource(ctx)
		if err != nil {
			return nil, err
		}
		up = network.NewUpstreamConn(dial, hook)
		return up, nil
	}

	for {
		cmd, raw, err := decodeRequest(dr)
		if err != nil {
			return // io.EOF clean close / a decode error → close (protocol_error is 32.2)
		}
		f.st.incRqTotal()
		f.st.addRxBytes(len(raw))

		if isLocalReply(cmd) {
			reply := localReply(cmd)
			if _, err := downstream.Write(reply); err != nil {
				return
			}
			f.st.addTxBytes(len(reply))
			continue
		}
		u, err := ensureUpstream()
		if err != nil {
			return // unresolvable cluster → graceful close (D-S32.1-6)
		}
		if err := u.Send(ctx, raw); err != nil {
			return
		}
		reply, err := decodeReply(u.Reader())
		if err != nil {
			return
		}
		if _, err := downstream.Write(reply); err != nil {
			return
		}
		f.st.addTxBytes(len(reply))
	}
}
```

> Add the `"github.com/esalaine/envoy-go/internal/stats"` import for the `*stats.Registry` parameter. Add the `newTestFilter` helper to `filter_test.go` (NOT production):
> ```go
> func newTestFilter(rs *redisStats, dial network.UpstreamDialFunc) *filter {
> 	return &filter{
> 		cfg: &compiledConfig{statPrefix: "rp", catchAllCluster: "rc"},
> 		st:  rs,
> 		dialSource: func(context.Context) (network.UpstreamDialFunc, func(), error) {
> 			return dial, nil, nil
> 		},
> 	}
> }
> ```

- [ ] **Step 4: Run the factory + pump tests — expect PASS**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestNewFactory|TestHandle' -race -v`
Expected: all PASS, no race.

- [ ] **Step 5: Full-package green + gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
go test ./internal/filter/network/redisproxy/... -race
git add internal/filter/network/redisproxy/ && git commit -m "phase 32.1 IMPL Task 10: filter.go — NewFactory + the TerminalFilter.Handle command->reply pump (PING/AUTH local, lazy catch_all + seam round-trip, downstream cx/rq + byte accounting)"
```
Expected: clean; tests PASS.

---

## Task 11: The 10th built-in registration + bootstrap blank-import + boot smoke

**Files:**
- Modify: `internal/filter/network/builtins/builtins.go`
- Modify: `internal/filter/network/builtins/builtins_test.go`
- Modify: `internal/bootstrap/bootstrap.go`

- [ ] **Step 1: Write the failing registration + boot-smoke test**

Append to `builtins_test.go` (mirror the existing all-N registration test):
```go
func TestRegisterBuiltins_RegistersRedisProxy(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{ClusterManager: testClusterManager(t), StatsRegistry: stats.NewRegistry()})
	if _, ok := reg.Lookup(redisproxy.TypeURL); !ok { // network.Registry exposes Lookup(typeURL) (NetworkFilterFactory, bool)
		t.Fatalf("redis_proxy not registered at %q", redisproxy.TypeURL)
	}
}
```
> Adapt `testClusterManager(t)` to the existing `builtins_test.go` helpers. ALSO edit the existing `TestRegisterBuiltinsRegistersAllNine` test (`builtins_test.go:56`) — rename/extend it to cover the tenth filter and bump its ordinal/count assertions (`nine → ten`); it asserts the per-filter "Nth built-in" positions, so the new `redis_proxy` registration must be added to its expectations, not just covered by a new standalone test.

- [ ] **Step 2: Run it — expect FAIL** (`redisproxy` not imported / not registered)

Run: `go test ./internal/filter/network/builtins/ -run TestRegisterBuiltins -v`
Expected: build error / FAIL.

- [ ] **Step 3: Add the 10th registration to `builtins.go`**

Add the import (after the `kafkabroker` import at line 21):
```go
	"github.com/esalaine/envoy-go/internal/filter/network/redisproxy"
```
Add the registration (after the `kafkabroker` registration at line 82):
```go
	// redis_proxy: the 10th built-in (32.1; ADR-0229/ADR-0230). UNLIKE the
	// stats-only kafka/mongo/zookeeper registrations, redisproxy passes BOTH
	// deps.ClusterManager (lazy catch_all resolution → Cluster.Dial via the
	// upstream-pool seam, ADR-0230) AND deps.StatsRegistry (the redis.<sp>
	// roster) — the tcpproxy.NewNetworkFactory cluster-capture + the stats-capture
	// precedents combined. The project's FIRST terminal routing proxy.
	reg.Register(redisproxy.TypeURL, redisproxy.NewFactory(deps.ClusterManager, deps.StatsRegistry))
```
Update the package doc (line 1) `nine built-in network filters (… kafka_broker)` → `ten built-in network filters (… kafka_broker, redis_proxy)` and the `RegisterBuiltins` doc (line 43) likewise.

- [ ] **Step 4: Add the bootstrap blank-import**

In `internal/bootstrap/bootstrap.go`, after the `kafka_broker/v3` blank-import (line 111):
```go
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
```
(Required for `@type` Any resolution at config load; ZERO new go.mod dep, AMEND-R1.)

- [ ] **Step 5: Run the registration test + a boot smoke + clean `go mod tidy`**

Run:
```bash
go test ./internal/filter/network/builtins/ -run TestRegisterBuiltins -v   # expect PASS
go build ./...                                                              # whole tree builds
cp go.mod /tmp/go.mod.b && go mod tidy && diff /tmp/go.mod.b go.mod && echo "CLEAN: zero new module (R8)"
```
Expected: PASS; build clean; `go mod tidy` no-op (R8).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/builtins/ internal/bootstrap/
golangci-lint run ./internal/filter/network/builtins/... ./internal/bootstrap/...
git add internal/filter/network/builtins/ internal/bootstrap/ && git commit -m "phase 32.1 IMPL Task 11: redis_proxy 10th built-in registration (cluster + stats deps) + bootstrap blank-import (zero new go.mod dep)"
```

---

## Task 12: `TCPRedisResponder` BackendKind (value 32) + the canned-reply RESP responder

**Files:**
- Modify: `test/differential/fixture/fixture.go`

> A NEW synthesized BackendKind (next-free value 32, after `TCPKafkaResponder = 31` at `fixture.go:542`) speaking minimal RESP: it reads RESP request frames and returns canned replies for the exercised data commands (`+OK\r\n` for SET; `$3\r\nbar\r\n` bulk for GET-hit). FIFO/positional — NO correlation id (contrast `TCPKafkaResponder`'s correlation-id echo). PING/AUTH NEVER reach the backend (local-reply — AMEND-R5). The exact canned-reply table is finalized here (the 32.2 matrix adds `$-1\r\n` GET-miss + `:<n>\r\n` INCR).

- [ ] **Step 1: Add the `TCPRedisResponder` constant**

In `fixture.go`, after the `TCPKafkaResponder BackendKind = 31` block (line 542), add (mirroring the `TCPKafkaResponder` comment style):
```go
	// TCPRedisResponder is a RESP-aware canned-response TCP backend (32; 32.1 SPEC
	// §8.3). It reads RESP request frames (array-of-bulk) and returns positional
	// canned replies for the exercised data commands: SET → +OK\r\n; GET → a bulk
	// string ($3\r\nbar\r\n for the 0055 round-trip). FIFO/positional — NO
	// correlation id (contrast TCPKafkaResponder's correlation-id echo). PING/AUTH
	// never reach the backend (redis_proxy answers them locally — AMEND-R5). NEW
	// BackendKind per reference_differential_fixture_dispatch_constraint; the
	// 32.2 command matrix extends the reply table ($-1 GET-miss, :<n> INCR).
	TCPRedisResponder BackendKind = 32
```

- [ ] **Step 2: Wire the backend's listen-and-serve loop**

In the backend dispatch where `TCPKafkaResponder` is served (the same switch the runner uses to start the canned backend), add a `TCPRedisResponder` arm that:
1. Accepts a TCP connection.
2. Loops: read one RESP request frame (reuse a small in-driver decode, or read the array-of-bulk envelope), extract the UPPERCASED command name.
3. `SET` → write `+OK\r\n`; `GET` → write `$3\r\nbar\r\n`; any other → write `-ERR unsupported\r\n` (defensive; the `0055` arms only drive SET/GET).
4. On EOF → close.

Follow the `TCPKafkaResponder`/`TCPMongoResponder` serve-loop structure already in `fixture.go`. Verify the round-trip ran (the differential asserts `cluster.<name>.upstream_rq_total > 0` — §8 caveat (i)).

- [ ] **Step 3: Compile the harness**

Run: `go build ./test/... && go vet ./test/differential/...`
Expected: clean (the new constant + arm compile; no driver references it yet).

- [ ] **Step 4: gofmt + lint + commit**

```bash
gofmt -l test/differential/fixture/
golangci-lint run ./test/differential/...
git add test/differential/fixture/fixture.go && git commit -m "phase 32.1 IMPL Task 12: TCPRedisResponder BackendKind (32) — minimal canned-reply RESP backend (SET +OK / GET bulk; positional, no correlation id)"
```

---

## Task 13: `0055-redis-roundtrip` driver part 1 — bootstraps + RESP request builders + the PING arm

**Files:**
- Create: `test/fixtures/0055-redis-roundtrip/driver/driver.go`
- Create: `test/fixtures/0055-redis-roundtrip/README.md`
- Modify: `test/differential/runner_test.go` (blank-import the `0055` driver)

> Cross-side. Chain `[redis_proxy]` as the TERMINAL on BOTH sides (the contrib reference Envoy + the envoy-go subprocess; NO `tcp_proxy`), `catch_all_route.cluster` → ONE cluster → the `TCPRedisResponder` backend (§8.3); the driver acts as a RESP client. Per the cross-side-XOR-boot-reject constraint (`reference_differential_fixture_dispatch_constraint`), ALL cross-side arms share this ONE dir; 32.1 lands arms 1–2 (32.2 extends with the command matrix + splitter arms). The driver implements `fixture.Driver` + `fixture.StatsAsserter` + `fixture.BackendKindAware` (returns `TCPRedisResponder`).

- [ ] **Step 1: Author the bootstraps + the RESP request builders + the PING arm**

`driver.go` scaffolding (D-S32.1-4 — the builders shared with the 32.2 command-matrix arms):
```go
// respArray builds a RESP array-of-bulk-strings request frame.
func respArray(parts ...string) []byte { /* "*<n>\r\n" + respBulk per part */ }
// respBulk builds one "$<len>\r\n<bytes>\r\n" bulk string.
func respBulk(s string) []byte { /* ... */ }
// inline builds an inline command line "<text>\r\n".
func inline(s string) []byte { return []byte(s + "\r\n") }
```
The driver:
- Bootstraps (both sides): a `[redis_proxy]` terminal listener with `stat_prefix`, `settings.op_timeout`, `prefix_routes.catch_all_route.cluster → redis_cluster`; one STRICT_DNS/STATIC cluster → the `TCPRedisResponder` backend addr (the runner injects it). ≥1 cluster satisfies the zero-cluster boot reject (`reference_network_filter_typeurl_extensions`).
- `BackendKind()` → `fixture.TCPRedisResponder`.
- **PING arm (local-reply; 32.1):** `DriveSubject`/`DriveReference` send `inline("PING")` AND `respArray("PING")` on one connection, capture each `+PONG\r\n` reply, and return the concatenated reply bytes (the runner's `CompareBytes` proves cross-side byte-equivalence of the response the proxy GENERATED — §8.1.1).

- [ ] **Step 2: Register the driver + add the runner blank-import**

`driver.go` `init()`: `fixture.RegisterFixture("0055-redis-roundtrip", &driver{})`. In `runner_test.go`, add the blank-import for the `0055` driver package (mirroring the existing `0053`/`0054` kafka blank-imports). Ensure the `TCPRedisResponder` switch-case from Task 12 routes the backend.

- [ ] **Step 3: Run the `0055` PING arm cross-side**

Run: `go test ./test/differential/ -run '0055|Redis' -v` (the runner discovers the registered fixture).
Expected: the PING arm passes — `+PONG\r\n` byte-identical cross-side; the reference + subject both boot the `[redis_proxy]` terminal.

> **Docker note:** the differential runner boots the contrib reference image directly (`envoyproxy/envoy:contrib-v1.37.2`); the `TCPRedisResponder` is the in-process backend the runner wires (no separate redis container — the parent §11 probes already confirmed the reference round-trips a real `redis:7`; the hermetic responder is the differential backend, the `TCPKafkaResponder`/`TCPMongoResponder` precedent).

- [ ] **Step 4: gofmt + lint + commit**

```bash
gofmt -l test/fixtures/0055-redis-roundtrip/ test/differential/
golangci-lint run ./test/...
git add test/fixtures/0055-redis-roundtrip/ test/differential/runner_test.go && git commit -m "phase 32.1 IMPL Task 13: 0055 driver part 1 — [redis_proxy]-terminal bootstraps + RESP request builders + the PING local-reply arm (+PONG byte-equivalence, zero upstream)"
```

---

## Task 14: `0055` driver part 2 — the proxied SET/GET round-trip arm + flat-`/stats` StatsAsserter + the deliberate-break liveness proof (R6)

**Files:**
- Modify: `test/fixtures/0055-redis-roundtrip/driver/driver.go`
- Modify: `test/fixtures/0055-redis-roundtrip/README.md`

- [ ] **Step 1: Add the proxied SET/GET round-trip arm**

Extend `DriveSubject`/`DriveReference`: on one connection send `respArray("SET", "foo", "bar")` then `respArray("GET", "foo")`, capture `+OK\r\n` then `$3\r\nbar\r\n`, and include them in the returned reply bytes (the `CompareBytes` byte-equivalence prong — §8.1.1).

- [ ] **Step 2: Implement `fixture.StatsAsserter` with an in-band flat-`/stats` scrape (D-S32.1-5)**

```go
func (d *driver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	ref := scrapeStats(t, refAdminAddr)   // GET http://<addr>/stats → map[name]value over "name: value" lines
	subj := scrapeStats(t, subjAdminAddr)
	for _, name := range []string{
		"redis.<sp>.downstream_cx_total",
		"redis.<sp>.downstream_rq_total",
		"redis.<sp>.downstream_cx_rx_bytes_total",
		"redis.<sp>.downstream_cx_tx_bytes_total",
		"cluster.redis_cluster.upstream_cx_total",  // == 1 (one lazy dial for the SET/GET arm)
		"cluster.redis_cluster.upstream_rq_total",  // == 2 (SET + GET)
	} {
		assertEqual(t, name, ref[name], subj[name])
	}
}
```
- `scrapeStats` GETs `/stats` (FLAT admin text, NOT `/stats/prometheus`) and parses `name: value` lines into a map (the `redis.` Prometheus tag-extractor arm is 32.2 — §8.1.2). The exact `<sp>`/cluster names match the bootstraps from Task 13.
- The PING-only assertion path (if split by arm) asserts `cluster.redis_cluster.upstream_cx_total == 0` (the seam never dials on a PING-only connection — AMEND-R5).

- [ ] **Step 3: Run the full `0055` (both arms + StatsAsserter) cross-side**

Run: `go test ./test/differential/ -run '0055|Redis' -count=1 -v`
Expected: PASS — downstream-RESP byte-equivalence (PING `+PONG`, SET `+OK`, GET `$3 bar`) + the flat-`/stats` `redis.<sp>.` + `cluster.<name>.*` parity (`upstream_cx_total == 1`, `upstream_rq_total == 2`).

- [ ] **Step 4: Deliberate-break liveness proof (R6) — prove each assertion is LIVE**

Per `reference_differential_asserter_dispatch` + `reference_differential_break_protocol_count1`: temporarily perturb the driver to assert a wrong value (e.g. `downstream_rq_total == 99`, or a wrong expected reply byte) and confirm the test FAILS on BOTH runner paths with `-count=1` (defeating go test caching):
```bash
# 1. Temporarily edit the driver to assert a deliberately-wrong value.
go test ./test/differential/ -run '0055|Redis' -count=1 -v   # expect FAIL (assertion is live)
# 2. Revert the perturbation.
go test ./test/differential/ -run '0055|Redis' -count=1 -v   # expect PASS
```
Record the break-and-revert in the driver comments + `README.md` + `PROGRESS.md` (R6). Prove EACH asserted counter + EACH response-byte assertion live.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l test/fixtures/0055-redis-roundtrip/
golangci-lint run ./test/...
git add test/fixtures/0055-redis-roundtrip/ && git commit -m "phase 32.1 IMPL Task 14: 0055 driver part 2 — proxied SET/GET round-trip arm + flat-/stats StatsAsserter (redis.<sp>. + cluster.* parity) + R6 deliberate-break liveness proof"
```

---

## Task 15: `0056-redis-boot-reject` fixture (the `stat_prefix` arm)

**Files:**
- Create: `test/fixtures/0056-redis-boot-reject/driver/driver.go`
- Create: `test/fixtures/0056-redis-boot-reject/README.md`
- Modify: `test/differential/runner_test.go` (blank-import the `0056` driver)

> Boot-reject; SEPARATE dir (cross-side XOR boot-reject — `reference_differential_fixture_dispatch_constraint`). Missing `stat_prefix` → BOTH sides reject at boot (the §6.1 `redis-proxy-stat-prefix-required` arm; boot-stderr-substring parity — substring `stat_prefix`). The driver implements `fixture.Driver` + `differential.BootRejectFixture`. The `settings`/`op_timeout`/`no-upstream`/`catch-all-cluster` arms are unit-tested at 32.1 (Task 3/4), NOT fixture arms (D-P32-5 — the kafka `0054`/mongo `0050` precedent).

- [ ] **Step 1: Author the boot-reject driver**

`driver.go`:
- Bootstraps (symmetric — same for both sides): a `[redis_proxy]` terminal listener with `settings.op_timeout` + a valid `catch_all_route.cluster` but `stat_prefix` OMITTED → both sides must fail to boot. A minimal unused cluster satisfies the zero-cluster boot reject (`reference_network_filter_typeurl_extensions`).
- Implement `differential.BootRejectFixture`:
  - `BootRejectScript() string { return "" }` (no driver script — the boot failure IS the assertion).
  - `ExpectedBootErrorSubstring() string { return "stat_prefix" }`.
- `init()`: `fixture.RegisterFixture("0056-redis-boot-reject", &driver{})`. Add the `runner_test.go` blank-import.

- [ ] **Step 2: Run the boot-reject fixture**

Run: `go test ./test/differential/ -run '0056' -count=1 -v`
Expected: PASS — BOTH the reference + the subject exit non-zero at boot; both stderr buffers contain `stat_prefix`.

- [ ] **Step 3: gofmt + lint + commit**

```bash
gofmt -l test/fixtures/0056-redis-boot-reject/
golangci-lint run ./test/...
git add test/fixtures/0056-redis-boot-reject/ test/differential/runner_test.go && git commit -m "phase 32.1 IMPL Task 15: 0056-redis-boot-reject fixture (stat_prefix-required; symmetric boot-reject, stderr substring parity)"
```

---

## Task 16: Completion bundle — ADRs + BEHAVIOR_CONTRACT + STATE/ROADMAP + next-prompt + the six-gate

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0230 §Decision/§Consequences body in-place; the ADR-0229 §Decision/§Consequences 32.1 half)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the 32.1 bundle)
- Modify: `docs/envoy-go/STATE.md` / `docs/envoy-go/ROADMAP.md` / `next-prompt.txt`

> ONE atomic bundle at the final task (ADR-0052 atomic landing).

- [ ] **Step 1: Fill the ADR-0230 §Decision/§Consequences body IN PLACE (ADR-0044)**

In `DECISIONS.md`, replace the ADR-0230 §Decision/§Consequences placeholders (the §Context anchored at the 32.1 SPEC) with the as-built body per SPEC §4 + Appendix B: the API (`UpstreamDialFunc`/`UpstreamConn`/`Send`/`Reader`/`Close`); the one-conn-per-downstream multiplexing model + synchronous single-flight FIFO/positional correlation; the dial-closure decoupling (no `internal/cluster` import in `internal/filter/network`); the in-filter routing (catch_all → `cm.Get`, lazy + unknown-tolerant); the deferred shared-pool/deep-queue boundary (§4.4). DECISIONS.md tail STAYS ADR-0230 (no new number consumed). Also fill the ADR-0229 §Decision/§Consequences 32.1 half (the filter + codec + seam consumption + the 10 fixed counters + PING/AUTH; the 32.2 half lands at the 32.2 IMPL).

- [ ] **Step 2: Write the BEHAVIOR_CONTRACT 32.1 bundle (SPEC §9)**

Add to `BEHAVIOR_CONTRACT.md`:
- NEW `### envoy.filters.network.redis_proxy` subsection (after kafka_broker): the terminal-routing-proxy semantics; the RESP codec value-type framing + inline + null sentinels; the streaming-reader partial-frame model (contrasted with the observer model); the `catch_all` single-cluster routing + the unknown-cluster tolerance (vs tcp_proxy's reject); the PING/AUTH local-reply set; the config parse + the PGV/runtime reject arms; the 10 fixed counters + the 4 created-not-incremented gauges; the upstream traffic stats via the reused `cluster.<name>.*` roster.
- NEW `## Network filters — upstream connection-pool / cluster-routing seam (32.1)` subsection (the ADR-0230 seam): the API + one-conn-per-downstream + synchronous single-flight FIFO + the redis-scoped boundary + the deferred shared-pool/deep-queue.
- Departure / coverage-boundary records (§7.5): boot-window eager-creation difference (unobservable); `op_timeout` parsed-not-consumed; gauges created-not-incremented (closed at 32.2); runtime-keys-at-defaults; close-direction-zero-touch (AMEND-R9); the one-conn-per-downstream pooling divergence forward-pointer (D-P32-9, pinned at 32.2); the 32.2-bundle forward-pointers; the unknown-cluster-graceful-close-no-`-ERR` note (D-S32.1-6).
- Stat table: 536 → **546** (the 10 new rows).

- [ ] **Step 3: Advance STATE.md + ROADMAP + next-prompt**

- `STATE.md`: active-phase → `phase 32.1 IMPL done`; lifecycle-state → the 32.2 SPEC; counts (546 stats / 58 fixtures / 40 fuzzers / BackendKind tail 32 / DECISIONS tail ADR-0230, next-free ADR-0231).
- `ROADMAP.md`: sub-row 32.1 `in-progress → done`; parent row 32 STAYS `in-progress` (the ROLLUP is 32.2's); sub-row 32.2 STAYS `planned`.
- `next-prompt.txt`: rewrite for the 32.2-SPEC cold-start (the full command set + the per-command/splitter/REDIS_CLUSTER_STATS roster + the `redis.` Prometheus arm + the gauges' inc/dec + the differential command matrix + the 41st fuzzer `FuzzRESPDecode` + the parent-row-32 rollup).

- [ ] **Step 4: The six-gate (SPEC §16.2) — run honestly, quote every output into `PROGRESS.md`**

```bash
go build ./...
go vet ./...
golangci-lint run
go test ./... -race -short
# the FULL differential suite byte-exact (58 dirs incl. the 56-dir back-compat gate):
go test ./test/differential/ -count=1
# h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — phase 32.1 touches no HTTP/h2/proxy-wasm path)
```
Expected: all green; 58/58 differential dirs (the 56 existing dirs byte-identical — the seam is ADDITIVE, SPEC §13-R1); h2spec 53/53; proxy-wasm 10/10. Re-pin the counts (fixtures 56→58, fuzzers 40 unchanged, stats 536→546, BackendKind 31→32; R9).

- [ ] **Step 5: Commit the completion bundle**

```bash
git add docs/envoy-go/ next-prompt.txt PROGRESS.md
git commit -m "phase 32.1 IMPL Task 16: completion bundle — ADR-0230 §Decision/§Consequences body + ADR-0229 32.1 half + BEHAVIOR_CONTRACT 32.1 bundle (stats 536→546) + STATE/ROADMAP/next-prompt advance + six-gate green"
```

> **Stage-close (controller, post-IMPL):** per `feedback_push_to_origin` + `feedback_subagents_no_push`, the controller squash-merges the IMPL branch to master + pushes to origin (subagents committed local-only throughout). The 32.1 IMPL runs `superpowers:subagent-driven-development` (`feedback_execution_style`).

---

## Test surface summary (SPEC §16.1)

- **Layer A — redisproxy unit tests** (Tasks 3–10): config parse (all PGV arms + runtime no-upstream + unknown-cluster TOLERANCE + IsValidName guard + byte-stable rejects); the RESP codec (each value type + null sentinels + inline + nested arrays; partial-frame block-and-resume; malformed/overflow/truncated → error-not-panic; raw-bytes-verbatim); the `Handle` pump (PING/AUTH local; single proxied command; pipelined SET-then-GET; unknown-cluster graceful close; EOF clean close; cx/rq + byte-count increments).
- **Layer A — seam unit tests** (`internal/filter/network/`, Task 7): `UpstreamConn` lazy-dial (no dial on a PING-only path) + single-flight `Send`/`Reader`/`Close` round-trip + a `-race` test + the build-level no-`internal/cluster`-import assertion.
- **Layer D — differential** (Tasks 13–15): `0055` (cross-side byte-equivalence + flat-`/stats` StatsAsserter; PING + proxied arms) + `0056` (boot-reject) + the FULL 56-dir back-compat suite → 58/58 green.
- **Layer E — race**: `go test ./... -race -short` across `internal/filter/network/...` + `internal/filter/network/redisproxy/...`.
- Per-task `gofmt -l` + `golangci-lint run` on touched packages (`feedback_pertask_gofmt_lint`).

(No fuzzer at 32.1 — `FuzzRESPDecode` is the 41st, 32.2; SPEC §2.10. The 32.1 no-panic guarantee is unit-test-proven at Tasks 5–6.)

---

## Execution handoff

Per `feedback_execution_style`, the 32.1 IMPL runs **`superpowers:subagent-driven-development`** (fresh subagent per task + two-stage review between tasks). Subagents commit LOCAL-ONLY (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on touched packages before its commit (`feedback_pertask_gofmt_lint`); deliberate-break checks use `go test -count=1` (`reference_differential_break_protocol_count1`); the GIT HYGIENE re-verify-branch discipline applies (`feedback_subagent_worktree_detach`).
