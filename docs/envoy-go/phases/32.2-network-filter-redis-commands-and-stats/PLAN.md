# Phase 32.2 PLAN — the full single-route command set + the per-command/splitter/cluster stat roster + the `redis.` Prometheus arm + the differential command matrix + the 41st fuzzer + the parent-row-32 ROLLUP

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended, per `feedback_execution_style`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Subagents commit **LOCAL-ONLY** (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on the touched packages before its commit (`feedback_pertask_gofmt_lint`). After a temporary deliberate break (Task 11 R6 liveness), use `go test -count=1` to defeat result caching (`reference_differential_break_protocol_count1`). Subagent worktree-detach hygiene per `feedback_subagent_worktree_detach` (a deliberate break can detach HEAD; the controller re-verifies the branch each task and a GIT HYGIENE block uses `git restore`, never `checkout-sha`/`amend`).

**Goal:** Complete `redis_proxy` to full single-route observability — wire the EAGER 180-command per-command stat roster (`command.<cmd>.{total,success,error}`) + the 2 `splitter.*` counters + the 3 `REDIS_CLUSTER_STATS` counters + the 2 connection-lifecycle gauges' (`downstream_cx_active`/`downstream_rq_active`) inc/dec + the `downstream_cx_protocol_error` increment into the `Handle` pump; add the `ECHO`/`TIME`/`QUIT`/`HELLO` local-reply commands; land the `redis.` LABEL-HOISTED Prometheus tag-extractor arm at `internal/stats/name.go`; extend `0055` with a differential command matrix (and the `TCPRedisResponder` reply table); add the 41st fuzzer `FuzzRESPDecode`; and roll the parent row 32 `in-progress → done` ATOMICALLY with sub-row 32.2 (ADR-0229 §Decision/§Consequences body completes PARTIAL → ACCEPTED).

**Architecture:** A `redisproxy`-package + `internal/stats/name.go` + test-surface change ONLY (the 29.2 "consumer sub-phase" shape — ZERO framework touch; the seam ADR-0230 is consumed unchanged). The 32.1 `Handle` pump's "else → proxy" branch gains a `classify(cmd, args)` dispatch verdict (local / proxy / unsupported / invalid-arity) that drives the table lookup + the per-command/splitter increments; the request lifecycle gains the `downstream_rq_active` gauge inc/dec; `Handle` entry/exit gains the `downstream_cx_active` gauge inc/dec; the decode-error path wires `downstream_cx_protocol_error`. `decodeRequest` is extended to also return the parsed argument slice (D-S32.2-4 option (a) — the cleaner no-double-parse path). The local-reply set extends from PING/AUTH to ECHO/TIME/QUIT/HELLO. `internal/stats/name.go` gains the `redis.<prefix>.<rest>` SINGLE-label hoist arm (the mongo `.rbac.` shape). ZERO changes to `internal/filter/network/` framework files, `upstreampool.go`, `manager.go`, `tcp_proxy`, or HCM.

**Tech Stack:** Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227); go-control-plane `/envoy` v1.32.4 (ADR-0008). Consumes the as-built `internal/filter/network/redisproxy/` package (32.1 — extended in place), `internal/filter/network/upstreampool.go` (the seam — UNCHANGED), `internal/stats/` (counters + gauges + `IsValidName` + `name.go` tag-extractor), the differential harness + `fixture.StatsAsserter` + the `/stats/prometheus` scrape. **ZERO new go.mod dependencies** (redis_proxy is CORE `/envoy v1.32.4`; the RESP codec is in-house — AMEND-R1).

---

## ADR-0045 split-gate FINAL re-check (at PLAN time, per SPEC §14.1 + parent §3.0)

The gate fires at `> ~25 tasks OR > ~1500 net-new production LoC`. This PLAN decomposes to **12 tasks** / **~325–500 production LoC** (the SPEC §14.1 envelope, re-confirmed at PLAN time on the 26.x–31 accounting basis — fixture drivers + unit/fuzz tests + the 180-name table literal EXCLUDED):

| Unit | Production LoC | Tasks |
|---|---|---|
| `commands.go` (the `classify` dispatch verdict + the extended local-reply set + arity/error helpers; the 180-name table literal EXCLUDED) | ~120–180 (+ ~30-line table literal, excluded) | 2, 3 |
| `stats.go` (the eager per-command roster build + 5 fixed counters + the inc accessors + the 2 gauge inc/dec + the protocol-error accessor) | ~90–140 | 4, 5 |
| `filter.go` (the `Handle` pump wiring — `classify` switch + splitter + per-command + gauges + protocol_error + QUIT close) | ~90–140 | 6 |
| `resp.go` (the `decodeRequest` argument-slice exposure — additive) | ~15–25 | 3 |
| `internal/stats/name.go` (the `redis.` arm) | ~15–25 | 7 |
| `FuzzRESPDecode` (test — excluded) | — | 8 |
| **Total (production basis)** | **~330–510** | **12** |

Both axes under the gate (12 ≤ ~25 tasks; ~510 ≤ ~1500 LoC) → **NO split. 32.2 proceeds as ONE sub-phase.** The pre-authorized 32.2a (Tasks 2–6: the command set + the per-command/splitter/cluster roster + the gauges) / 32.2b (Tasks 7–12: the `redis.` arm + the fuzzer + the differential matrix + the ROLLUP) escape-valve axis (SPEC §14.1) stays **UNCONSUMED**. The `0055` driver extension + the `TCPRedisResponder` reply-table extension + the unit/fuzz tests are excluded per the 26.x–31 accounting precedent.

---

## PLAN-time D-question dispositions (SPEC §13.2)

- **Task order = the SPEC §14 spine, green-compiling at every task.** Each task builds + tests green standalone (`go test ./internal/filter/network/redisproxy/...` builds the whole package every task). The order: `baselines (1) → commands.go table+lookup (2) → commands.go classify+local-reply + decodeRequest args (3) → stats.go per-command+splitter+cluster roster (4) → stats.go gauges+protocol_error (5) → filter.go pump wiring (6) → name.go redis. arm (7) → FuzzRESPDecode (8) → TCPRedisResponder reply table (9) → 0055 command matrix (10) → 0055 held-open gauge arm + R6 break (11) → completion bundle (12)`.
- **D-S32.2-1 (the table representation + case) — RESOLVED at PLAN: a sorted `[]string` golden list (lower-cased, the 180-name source of truth for both the roster build AND the `TestCommandRoster_MatchesUpstream` golden) + a derived `map[string]struct{}` for O(1) dispatch lookup.** The decoder uppercases (`decodeRequest` → `strings.ToUpper`); dispatch lower-cases ONCE (`lc := strings.ToLower(cmd)`) and uses `lc` for BOTH the membership check AND the `command.<lc>.*` stat segment (the upstream `AsciiStrToLower`; §3.2 / §12.1). One lower-case per request; the table member is the stat segment → IsValidName by construction (§5.2).
- **D-S32.2-4 (decodeRequest argument exposure) — RESOLVED at PLAN: extend `decodeRequest` to return the parsed argument slice (option (a), the cleaner no-double-parse path).** New signature `decodeRequest(r) (cmd string, args [][]byte, raw []byte, err error)` where `args[0]` is the AS-RECEIVED command token (original case — for the unknown-command error echo) and `args[1:]` are the arguments; `cmd = strings.ToUpper(string(args[0]))`. The arity count is `len(args)`; ECHO/HELLO read `args[1]`. This **refines the SPEC §3.1 "resp.go UNCHANGED" anticipation** — the SPEC explicitly flagged the arg-exposure split as the PLAN's via D-S32.2-4 (option (a) "cleaner — the arity check + ECHO arg + HELLO arg-count all want the parsed array"). `resp.go` is touched minimally (the array/inline loop already has the bulk payloads in hand; exposing them is ~15 LoC). The alternative (re-parse `raw` in `commands.go`) is rejected — it double-walks an already-validated frame.
- **D-S32.2-5 (the QUIT close signal) — RESOLVED at PLAN: a `commandVerdict` struct with a `closeAfter bool` field.** `classify(cmd, args)` returns the full dispatch decision `{action, reply, closeAfter, statCmd}`; QUIT → `{action: actLocal, reply: respOK, closeAfter: true}`. The pump writes `reply`, then `return`s the pump loop when `closeAfter` (after the deferred `downstream.Close()` flushes). This replaces the 32.1 `isLocalReply(cmd) bool` + `localReply(cmd) []byte` two-function shape with a single typed verdict (cleaner than a second `[]byte`-plus-`bool` return; testable in isolation).
- **D-S32.2-2 (the unknown-command error wording) — anticipated `-ERR unknown command '<name>', with args beginning with: \r\n`** (the upstream wording, parent §11.5; `<name>` = the AS-RECEIVED `args[0]`, original case; EMPTY args-suffix per SPEC §8.1). The exact byte-stable form is **confirmed LIVE at Task 10** (the `0055` UNKNOWN arm captures the reference bytes — `reference_wire_format_both_sides_see_same_bytes`). If the reference's suffix differs, Task 10 pins the captured bytes and Task 3's constant is corrected in lockstep.
- **D-S32.2-3 (the arity rule + `commandsWithoutMandatoryArgs` set) — RESOLVED at PLAN: the minimal rule `len(args) < 2 && !commandsWithoutMandatoryArgs[lc] → actInvalid`.** The exact `commandsWithoutMandatoryArgs` membership is transcribed from upstream `supported_commands.h` at Task 3 (the singletons PING/TIME/QUIT/AUTH/ECHO are handled BEFORE the table arity check, so they need no entry; the set matters only for table commands legitimately callable with zero args — e.g. `command`, `info`, `randomkey`). The conservative default (empty set → every table command needs ≥1 arg) does NOT break any `0055` arm (GET/INCR/DEL/SET are all sent with ≥1 arg). The ECHO-wrong-arity arm (§8.1) is the live `splitter.invalid_request` proof.
- **D-S32.2-6 (the prometheus vs flat-`/stats` scrape mix) — RESOLVED at PLAN: the NEW per-command + splitter + gauge assertions use `/stats/prometheus` (the `redis.` label-aware arm — the kafka `0053` `scrapeKafkaStats`/`canonicalize` mechanics); the 32.1 flat-`/stats` counters stay flat-scraped (the existing `scrapeStats`/`AssertStats` is KEPT and extended).** Both endpoints are available to the in-band driver.
- **D-S32.2-7 (the held-open gauge-arm mechanism) — RESOLVED at PLAN: two driver fields `refHeld, subjHeld net.Conn`** holding an idle PING'd connection per side, opened LAST in `driveProxy` (after all transient matrix arms close), kept alive across `AssertStats`, and closed in a `t.Cleanup` AFTER the gauge assertion. The mongo `op_query_active` 29.2 held-arm is the precedent (§8.2).
- **D-S32.2-8 (the fuzzer allocation cap) — RESOLVED at PLAN: keep `maxBulkLen` (512 MiB) / `maxArrayLen` (1 Mi) as the documented bound** (the upstream `proto_max_bulk_size` analog; `resp.go:14-17` UNCHANGED). The fuzzer documents the cap as the allocation bound; it does NOT tighten it (§9).
- **The `classify` verdict + the new helpers live in `commands.go`** (SPEC §3.1: commands.go holds "the unknown-command + bad-arity dispatch decision"); `filter.go`'s pump switches on `verdict.action`. This keeps the dispatch logic unit-testable in isolation (`commands_test.go`) and the pump (`filter.go`) thin.

---

## File Structure

**Created:**
- `internal/filter/network/redisproxy/fuzz_test.go` — `FuzzRESPDecode` (the 41st; §9). Created at Task 8.

**Modified (production):**
- `internal/filter/network/redisproxy/commands.go` — the static 180-command `supportedCommands` table + `supportedCommandList` golden slice + `commandSupported`; the `commandVerdict` struct + `classify(cmd, args)` dispatch; the extended local-reply set (ECHO/TIME/QUIT/HELLO) + `encodeBulk`/`encodeTime`/`classifyHello`; the `commandsWithoutMandatoryArgs` set + `validArity`; the `unknownCommandError` builder + `respOK`/`respInvalidRequest`/`respHelloOptions`/`respNoProto` constants. (Tasks 2, 3.)
- `internal/filter/network/redisproxy/resp.go` — `decodeRequest` extended to return `args [][]byte` (additive; the local-reply constants `respPong`/`respAuthNoPassword` stay; the new local-reply byte constants land in `commands.go`). (Task 3.)
- `internal/filter/network/redisproxy/stats.go` — the EAGER 540 per-command counters (`command.<cmd>.{total,success,error}`) + the 2 `splitter.*` + 3 `REDIS_CLUSTER_STATS` counters + the per-command/splitter inc accessors + the 2 lifecycle gauges' inc/dec + the `downstream_cx_protocol_error` accessor. (Tasks 4, 5.)
- `internal/filter/network/redisproxy/filter.go` — the `Handle` pump wiring: the `classify` switch + the per-command total/success/error + the `splitter.*` increments + the `cx_active`/`rq_active` gauge inc/dec + the `cx_protocol_error` increment + the QUIT close (via a `serveRequest` helper for defer-safe `rq_active` accounting). (Task 6.)
- `internal/filter/network/redisproxy/doc.go` — the 32.2 forward-pointers resolved (the command set + roster landed). (Task 6.)
- `internal/stats/name.go` — the `redis.<prefix>.<rest>` SINGLE-label hoist arm in the default branch. (Task 7.)
- `test/differential/runner_test.go` — the `redisRespondLoop` reply-table extension (`$-1` GET-miss, `:1` INCR/DEL). (Task 9.)
- `test/fixtures/0055-redis-roundtrip/driver/driver.go` — the command-matrix arms + the `/stats/prometheus` label-aware per-command/splitter assertions + the `cx_active` held-open gauge arm + the `rq_active`/buffered quiesced-zero assertions. (Tasks 10, 11.)
- `test/fixtures/0055-redis-roundtrip/README.md` — the 32.2 arm table + the R6 break-liveness records. (Tasks 10, 11.)

**Modified (test — unit/fuzz):**
- `internal/filter/network/redisproxy/commands_test.go` — the table golden + IsValidName tests + the `classify` dispatch tests + the local-reply behaviors. (Tasks 2, 3, 6.)
- `internal/filter/network/redisproxy/stats_test.go` — the per-command/splitter/cluster roster size + golden + idempotency + inc-accessor tests + the gauge inc/dec + protocol-error tests. (Tasks 4, 5.)
- `internal/filter/network/redisproxy/filter_test.go` — the pump dispatch tests (unknown/bad-arity/success-on-error-reply/error-on-transport-failure/gauge inc-dec/protocol-error/QUIT-close) + the `decodeRequest`-signature caller updates. (Tasks 3, 6.)
- `internal/filter/network/redisproxy/resp_test.go` — the `decodeRequest` args-return assertions. (Task 3.)
- `internal/stats/name_test.go` — the `redis.` arm tests (fixed names + dynamic command names + dot-flatten + gauge `# TYPE`; contrast kafka/mongo). (Task 7.)

**Completion bundle (Task 12):**
- `docs/envoy-go/DECISIONS.md` (ADR-0229 §Decision/§Consequences 32.2 half PARTIAL → ACCEPTED, in-place) / `docs/envoy-go/BEHAVIOR_CONTRACT.md` / `docs/envoy-go/STATE.md` / `docs/envoy-go/ROADMAP.md` (parent row 32 + sub-row 32.2 ATOMIC ROLLUP) / `next-prompt.txt`.

**Untouched (pinned — a regression gate; SPEC §2.6/§6/§8.5):** `internal/filter/network/upstreampool.go` (the seam — consumed, not modified), all other `internal/filter/network/` framework files (`chain.go`/`readconn.go`/`writeconn.go`/`types.go`/`callbacks.go`/`terminal.go`/`registry.go`/`upstreamcluster.go`), `internal/listener/manager.go`, `internal/filter/tcpproxy/`, `internal/filter/hcm/`, `internal/cluster/`, `internal/stats/registry.go`/`gauge.go`/`counter.go`, `internal/filter/network/redisproxy/config.go` (no new parse arm — §6), `internal/filter/network/builtins/` + `internal/bootstrap/bootstrap.go` (the 10th built-in + blank-import landed at 32.1), `test/differential/fixture/fixture.go` (the `TCPRedisResponder = 32` constant unchanged — only the `runner_test.go` reply LOOP extends), the `0056-redis-boot-reject` fixture.

---

## Task 1: First-action baselines/anchors gate (no code change)

**Files:** none modified — verification + re-pin gate at the IMPL-session tip; record in `PROGRESS.md` (created this task at the worktree root).

- [ ] **Step 1: Re-confirm the project counts at the IMPL-session tip**

Run (from the worktree root):
```bash
git log --oneline -1
# fixtures (canonical recipe):
ls -d test/fixtures/[0-9]* | wc -l            # expect 58; tail dir:
ls -d test/fixtures/[0-9]* | tail -1          # expect test/fixtures/0056-redis-boot-reject
# fuzzers (canonical recipe — scoped to ./internal):
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 40 (tail FuzzKafkaDecode)
# BackendKind tail:
grep -nE "TCPRedisResponder BackendKind = 32" test/differential/fixture/fixture.go   # expect a hit
# DECISIONS.md tail + next-free:
grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -3  # tail = ADR-0230 → next-free ADR-0231
```
Expected: fixtures **58** (tail `0056-redis-boot-reject`); fuzzers **40** (tail `FuzzKafkaDecode`); BackendKind tail **32** (`TCPRedisResponder`); DECISIONS.md tail **ADR-0230** (next-free **ADR-0231**). 32.2 lands the 41st fuzzer `FuzzRESPDecode`, NO new fixture dir (`0055` extended), `TCPRedisResponder` reply-table extended (NOT re-numbered), and the ADR-0229 §Decision/§Consequences body completes IN PLACE (no new ADR number consumed).

- [ ] **Step 2: Re-confirm the stat surface = 546**

Run the project's canonical stat-surface recipe (the count STATE.md / BEHAVIOR_CONTRACT.md report as **546** — the BEHAVIOR_CONTRACT stat-table row count; do NOT invent a new recipe). Expected: **546**. 32.2 lands +545 (2 `splitter.*` + 3 `REDIS_CLUSTER_STATS` + 180×3=540 per-command; latency histograms + `*_fault` counters NOT counted per ADR-0060/faults) → **1091** at Task 12.

- [ ] **Step 3: Re-confirm the §12.1 180-command table source + a clean `go mod tidy` (ZERO new dep)**

Re-read SPEC §12.1 (the deduplicated 180-name list) — this is the golden the Task 2 table transcribes. Run:
```bash
go mod tidy && git diff --exit-code go.mod go.sum   # expect NO diff (redis_proxy is CORE /envoy v1.32.4 — ZERO new dep)
go build ./... && go vet ./...                       # expect clean
```
Expected: clean baseline. Record all counts + the `git log --oneline -1` tip SHA in `PROGRESS.md`.

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/phases/32.2-network-filter-redis-commands-and-stats/PROGRESS.md
git commit -m "phase 32.2 Task 1: baselines gate — fixtures 58 / fuzzers 40 / stat surface 546 / BackendKind 32 / ADR tail 0230; clean go mod tidy"
```

---

## Task 2: `commands.go` — the static 180-command table + roster golden tests

**Files:**
- Modify: `internal/filter/network/redisproxy/commands.go`
- Test: `internal/filter/network/redisproxy/commands_test.go`

- [ ] **Step 1: Write the failing tests (the golden 180 + IsValidName-by-construction)**

Append to `commands_test.go`:
```go
// TestCommandRoster_MatchesUpstream pins supportedCommandList against the SPEC
// §12.1 golden 180-name list (the 8 iterated SupportedCommands groups, deduped).
// A transcription error in the table literal fails here (the byte-stable guard).
func TestCommandRoster_MatchesUpstream(t *testing.T) {
	if got := len(supportedCommandList); got != 180 {
		t.Fatalf("supportedCommandList size = %d, want 180", got)
	}
	// Sorted + unique (the map is derived from the slice; a dup would shrink it).
	if got := len(supportedCommands); got != 180 {
		t.Fatalf("supportedCommands map size = %d, want 180 (a duplicate name?)", got)
	}
	for i := 1; i < len(supportedCommandList); i++ {
		if supportedCommandList[i-1] >= supportedCommandList[i] {
			t.Fatalf("supportedCommandList not strictly sorted/unique at %d: %q >= %q",
				i, supportedCommandList[i-1], supportedCommandList[i])
		}
	}
	// Spot-pin a representative member from each of the 8 groups + the dotted names.
	for _, name := range []string{
		"get", "set", "append", // simpleCommands
		"eval", "evalsha", // evalCommands
		"object",                      // objectCommands
		"del", "exists", "touch", "unlink", // hashMultipleSumResultCommands
		"mget", "mset", "scan", "info.shard", // dedicated handlers
		"cluster", "randomkey", // randomShardCommands
		"multi", "exec", "discard", "watch", "unwatch", // transactionCommands
		"script", "flushall", "config", "info", "keys", "select", "role", "hello", // ClusterScopeCommands
		"bf.add", "bf.scandump", // module dotted names
	} {
		if _, ok := supportedCommands[name]; !ok {
			t.Errorf("supportedCommands missing %q", name)
		}
	}
	// The singletons handled inline (NOT in the per-command table — §12.1).
	for _, name := range []string{"ping", "auth", "echo", "time", "quit"} {
		if _, ok := supportedCommands[name]; ok {
			t.Errorf("supportedCommands must NOT contain inline-singleton %q", name)
		}
	}
}

// TestCommandRoster_AllValidNames pins the IsValidName-by-construction property
// (D-P32-7 / §5.2): every table name flattens to command.<name>.{total,...} whose
// segment chars are [a-z._] — all valid; a wire command not in the table never
// reaches NewCounterIfAbsent (it routes to splitter.unsupported_command).
func TestCommandRoster_AllValidNames(t *testing.T) {
	for _, name := range supportedCommandList {
		for _, slot := range []string{"total", "success", "error"} {
			full := "redis.rp.command." + name + "." + slot
			if !stats.IsValidName(full) {
				t.Errorf("stat name %q is NOT IsValidName — table member %q breaks by-construction guarantee", full, name)
			}
		}
	}
}

func TestCommandSupported_LookupIsLowerCase(t *testing.T) {
	if lc, ok := commandSupported("GET"); !ok || lc != "get" {
		t.Errorf("commandSupported(GET) = (%q,%v), want (get,true)", lc, ok)
	}
	if lc, ok := commandSupported("INFO.SHARD"); !ok || lc != "info.shard" {
		t.Errorf("commandSupported(INFO.SHARD) = (%q,%v), want (info.shard,true)", lc, ok)
	}
	if _, ok := commandSupported("BOGUSCMD"); ok {
		t.Error("commandSupported(BOGUSCMD) = true, want false")
	}
}
```
Add the `stats` import to `commands_test.go` (`"github.com/esalaine/envoy-go/internal/stats"`).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestCommandRoster|TestCommandSupported' -count=1`
Expected: FAIL (compile error — `supportedCommandList`, `supportedCommands`, `commandSupported` undefined).

- [ ] **Step 3: Write the table + lookup in `commands.go`**

Prepend to `commands.go` (after the package clause; add the `"strings"` import):
```go
import "strings"

// supportedCommandList is the EXACT 180-command roster (lower-cased, strictly
// sorted) transcribed from SPEC §12.1 — the 8 iterated SupportedCommands groups
// of upstream Envoy v1.37.2 supported_commands.h (simpleCommands 152 + eval 2 +
// object 1 + hashMultipleSumResult 4 + mget/mset/scan/info.shard 4 + randomShard
// 2 + transaction 5 + ClusterScope 10 = 180). It is the SINGLE SOURCE OF TRUTH
// for BOTH the eager per-command stat roster (stats.go) AND the dispatch lookup.
// The dotted names (bf.add…bf.scandump, info.shard) flatten to command_bf_add /
// command_info_shard in Prometheus (dot→underscore; §5) and pass IsValidName
// (§5.2 — TestCommandRoster_AllValidNames). The inline singletons ping/auth/echo/
// time/quit are NOT here (handled in classify — §12.1). The latency histogram +
// *_fault counters are NOT created (ADR-0060 / faults — §2.3).
//
// KEEP-IN-SYNC: SPEC §12.1 golden + TestCommandRoster_MatchesUpstream.
var supportedCommandList = []string{
	"append", "bf.add", "bf.card", "bf.exists", "bf.info", "bf.insert", "bf.loadchunk",
	"bf.madd", "bf.mexists", "bf.reserve", "bf.scandump", "bitcount", "bitfield", "bitop",
	"bitpos", "cluster", "config", "copy", "decr", "decrby", "del", "discard", "dump",
	"eval", "evalsha", "exec", "exists", "expire", "expireat", "flushall", "flushdb",
	"geoadd", "geodist", "geohash", "geopos", "georadius", "georadius_ro", "georadiusbymember",
	"georadiusbymember_ro", "geosearch", "geosearchstore", "get", "getbit", "getdel", "getex",
	"getrange", "getset", "hdel", "hello", "hexists", "hget", "hgetall", "hincrby",
	"hincrbyfloat", "hkeys", "hlen", "hmget", "hmset", "hrandfield", "hscan", "hset", "hsetnx",
	"hstrlen", "hvals", "incr", "incrby", "incrbyfloat", "info", "info.shard", "keys", "lindex",
	"linsert", "llen", "lmove", "lpop", "lpos", "lpush", "lpushx", "lrange", "lrem", "lset",
	"ltrim", "mget", "mset", "msetnx", "multi", "object", "persist", "pexpire", "pexpireat",
	"pfadd", "pfcount", "pfmerge", "psetex", "pttl", "publish", "randomkey", "rename", "renamenx",
	"restore", "role", "rpop", "rpoplpush", "rpush", "rpushx", "sadd", "scan", "scard", "script",
	"sdiff", "sdiffstore", "select", "set", "setbit", "setex", "setnx", "setrange", "sinter",
	"sinterstore", "sismember", "slowlog", "smembers", "smismember", "smove", "sort", "sort_ro",
	"spop", "srandmember", "srem", "sscan", "strlen", "substr", "sunion", "sunionstore", "touch",
	"ttl", "type", "unlink", "unwatch", "watch", "xack", "xadd", "xautoclaim", "xclaim", "xdel",
	"xlen", "xpending", "xrange", "xrevrange", "xtrim", "zadd", "zcard", "zcount", "zdiff",
	"zdiffstore", "zincrby", "zinter", "zinterstore", "zlexcount", "zmscore", "zpopmax", "zpopmin",
	"zrandmember", "zrange", "zrangebylex", "zrangebyscore", "zrangestore", "zrank", "zrem",
	"zremrangebylex", "zremrangebyrank", "zremrangebyscore", "zrevrange", "zrevrangebylex",
	"zrevrangebyscore", "zrevrank", "zscan", "zscore", "zunion", "zunionstore",
}

// supportedCommands is the O(1) dispatch-lookup set derived from supportedCommandList.
var supportedCommands = func() map[string]struct{} {
	m := make(map[string]struct{}, len(supportedCommandList))
	for _, c := range supportedCommandList {
		m[c] = struct{}{}
	}
	return m
}()

// commandSupported lower-cases cmd (the decoder uppercases) and reports whether it
// is a per-command-stat-bearing command. The returned lc is the command.<lc>.*
// stat segment (a table member → IsValidName by construction, §5.2).
func commandSupported(cmd string) (lc string, ok bool) {
	lc = strings.ToLower(cmd)
	_, ok = supportedCommands[lc]
	return lc, ok
}
```

NOTE: the literal above MUST total exactly 180 entries — Task 2's `TestCommandRoster_MatchesUpstream` is the guard. If the count is off, re-transcribe from SPEC §12.1 verbatim (do NOT hand-edit individual lines — re-paste the §12.1 block and re-flow).

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestCommandRoster|TestCommandSupported' -count=1 -v`
Expected: PASS (all four tests). If `TestCommandRoster_MatchesUpstream` fails on the count, the literal has a transcription drift — fix against SPEC §12.1.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/commands.go internal/filter/network/redisproxy/commands_test.go
git commit -m "phase 32.2 Task 2: the static 180-command supportedCommands table + commandSupported + roster golden/IsValidName tests"
```

---

## Task 3: `commands.go` + `resp.go` — the `classify` dispatch verdict + the extended local-reply set + `decodeRequest` arg exposure

**Files:**
- Modify: `internal/filter/network/redisproxy/resp.go` (the `decodeRequest` arg-slice return — additive)
- Modify: `internal/filter/network/redisproxy/commands.go` (the `commandVerdict` + `classify` + the local-reply set + helpers)
- Modify: `internal/filter/network/redisproxy/filter.go` (caller update — `decodeRequest` signature; functional pump wiring is Task 6)
- Modify: `internal/filter/network/redisproxy/commands_test.go` (delete/replace the stale `TestIsLocalReply`/`TestLocalReply_Bytes`; add `classify` tests)
- Test: `internal/filter/network/redisproxy/resp_test.go`, `internal/filter/network/redisproxy/filter_test.go` (caller updates)

- [ ] **Step 1: Write the failing `decodeRequest`-args test (`resp_test.go`)**

Append to `resp_test.go`:
```go
func TestDecodeRequest_ExposesArgs(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantCmd string
		wantArg []string // args[0]=command token (original case), args[1:]=arguments
	}{
		{"array SET", "*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n", "SET", []string{"SET", "foo", "bar"}},
		{"array ECHO", "*2\r\n$4\r\nECHO\r\n$2\r\nhi\r\n", "ECHO", []string{"ECHO", "hi"}},
		{"inline PING arg", "PING hello\r\n", "PING", []string{"PING", "hello"}},
		{"array HELLO 3", "*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n", "HELLO", []string{"HELLO", "3"}},
		{"lowercase echo preserved in args0", "*1\r\n$3\r\nget\r\n", "GET", []string{"get"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, args, _, err := decodeRequest(bufio.NewReader(strings.NewReader(tc.in)))
			if err != nil {
				t.Fatalf("decodeRequest(%q) err = %v", tc.in, err)
			}
			if cmd != tc.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tc.wantCmd)
			}
			if len(args) != len(tc.wantArg) {
				t.Fatalf("len(args) = %d, want %d (%q)", len(args), len(tc.wantArg), args)
			}
			for i := range tc.wantArg {
				if string(args[i]) != tc.wantArg[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tc.wantArg[i])
				}
			}
		})
	}
}
```
(`resp_test.go` already imports `bufio`/`strings`; confirm and add if missing.)

- [ ] **Step 2: Write the failing `classify` + local-reply tests (`commands_test.go`)**

DELETE the stale 32.1 `TestIsLocalReply` + `TestLocalReply_Bytes` (the `isLocalReply`/`localReply` two-function shape is replaced by `classify` — D-S32.2-5). Add:
```go
// asArgs builds the args slice classify expects: args[0]=command token, args[1:]=arguments.
func asArgs(parts ...string) [][]byte {
	out := make([][]byte, len(parts))
	for i, p := range parts {
		out[i] = []byte(p)
	}
	return out
}

func TestClassify_LocalReplies(t *testing.T) {
	cases := []struct {
		name       string
		cmd        string
		args       []string
		wantAction action
		wantReply  string
		wantClose  bool
	}{
		{"PING no echo", "PING", []string{"PING", "hello"}, actLocal, "+PONG\r\n", false},
		{"AUTH no password", "AUTH", []string{"AUTH", "x"}, actLocal, "-ERR Client sent AUTH, but no password is set\r\n", false},
		{"ECHO valid", "ECHO", []string{"ECHO", "hi"}, actLocal, "$2\r\nhi\r\n", false},
		{"ECHO wrong arity", "ECHO", []string{"ECHO"}, actInvalid, "-invalid request\r\n", false},
		{"QUIT closes", "QUIT", []string{"QUIT"}, actLocal, "+OK\r\n", true},
		{"HELLO 3 NOPROTO", "HELLO", []string{"HELLO", "3"}, actLocal, "-NOPROTO unsupported protocol version\r\n", false},
		{"HELLO options", "HELLO", []string{"HELLO", "2", "AUTH", "u", "p"}, actLocal, "-ERR HELLO options like AUTH and SETNAME are not supported\r\n", false},
		{"unknown command", "BOGUSCMD", []string{"BOGUSCMD", "x"}, actUnsupported, "-ERR unknown command 'BOGUSCMD', with args beginning with: \r\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := classify(tc.cmd, asArgs(tc.args...))
			if v.action != tc.wantAction {
				t.Errorf("action = %d, want %d", v.action, tc.wantAction)
			}
			if string(v.reply) != tc.wantReply {
				t.Errorf("reply = %q, want %q", v.reply, tc.wantReply)
			}
			if v.closeAfter != tc.wantClose {
				t.Errorf("closeAfter = %v, want %v", v.closeAfter, tc.wantClose)
			}
		})
	}
}

func TestClassify_Proxied(t *testing.T) {
	// HELLO 2 / HELLO no-arg → proxied (it IS in the table — ClusterScopeCommand).
	for _, args := range [][]string{{"HELLO", "2"}, {"HELLO"}} {
		v := classify("HELLO", asArgs(args...))
		if v.action != actProxy || v.statCmd != "hello" {
			t.Errorf("classify(HELLO %v) = {action:%d statCmd:%q}, want {actProxy hello}", args, v.action, v.statCmd)
		}
	}
	// A data command → proxied, lower-cased stat segment.
	v := classify("GET", asArgs("GET", "foo"))
	if v.action != actProxy || v.statCmd != "get" {
		t.Errorf("classify(GET foo) = {action:%d statCmd:%q}, want {actProxy get}", v.action, v.statCmd)
	}
}

func TestClassify_BadArity(t *testing.T) {
	// A table command needing args, sent with none → invalid_request.
	v := classify("GET", asArgs("GET"))
	if v.action != actInvalid || string(v.reply) != "-invalid request\r\n" {
		t.Errorf("classify(GET) = {action:%d reply:%q}, want {actInvalid -invalid request}", v.action, v.reply)
	}
}

func TestClassify_TimeShapeOnly(t *testing.T) {
	// TIME is local but wall-clock (NON-DETERMINISTIC) — assert SHAPE: a 2-element
	// array of bulk strings, both numeric. NOT a byte-equivalence arm (§12.4).
	v := classify("TIME", asArgs("TIME"))
	if v.action != actLocal {
		t.Fatalf("TIME action = %d, want actLocal", v.action)
	}
	cmd2, _, _, err := decodeReplyShape(t, v.reply) // parse the reply as RESP
	_ = cmd2
	if err != nil {
		t.Fatalf("TIME reply not a valid RESP frame: %v (%q)", err, v.reply)
	}
	if !strings.HasPrefix(string(v.reply), "*2\r\n$") {
		t.Errorf("TIME reply = %q, want a 2-element array of bulk strings", v.reply)
	}
}
```
(For `TestClassify_TimeShapeOnly`, assert the prefix shape directly — `decodeReplyShape` is illustrative; the minimal assertion is the `*2\r\n$` prefix + two numeric bulk bodies. Simplify to a direct `decodeReply(bufio.NewReader(bytes.NewReader(v.reply)))` round-trip + the prefix check, dropping the helper.)

- [ ] **Step 3: Run to verify they fail**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestDecodeRequest_ExposesArgs|TestClassify' -count=1`
Expected: FAIL (compile errors — `classify`, `action`, `actLocal`/`actProxy`/`actUnsupported`/`actInvalid` undefined; `decodeRequest` returns 3 values not 4).

- [ ] **Step 4: Extend `decodeRequest` in `resp.go` (additive — the args return)**

Change the signature and accumulate the parsed elements. Replace the `decodeRequest` function:
```go
// decodeRequest reads ONE request frame (inline OR array-of-bulk-strings) from r
// and returns the UPPERCASED command name (for dispatch), the parsed argument
// slice (args[0]=the AS-RECEIVED command token, args[1:]=arguments — for the
// local-reply ECHO/HELLO arg + arity + the unknown-command echo), and the RAW
// frame bytes (forwarded VERBATIM upstream when proxied). Blocks on partial
// frames. A frame-boundary io.EOF is returned verbatim (clean connection end).
func decodeRequest(r *bufio.Reader) (cmd string, args [][]byte, raw []byte, err error) {
	p, err := r.Peek(1)
	if err != nil {
		return "", nil, nil, err // io.EOF here = clean end between frames
	}
	var buf bytes.Buffer
	if p[0] != '*' {
		switch p[0] {
		case '+', '-', ':', '$', '@':
			return "", nil, nil, errProtocol
		}
		line, err := readLine(r, &buf)
		if err != nil {
			return "", nil, nil, unexpected(err)
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return "", nil, nil, errProtocol
		}
		args = make([][]byte, len(fields))
		for i, f := range fields {
			args[i] = []byte(f)
		}
		return strings.ToUpper(fields[0]), args, buf.Bytes(), nil
	}
	header, err := readLine(r, &buf)
	if err != nil {
		return "", nil, nil, unexpected(err)
	}
	n, err := strconv.Atoi(header[1:])
	if err != nil || n <= 0 || n > maxArrayLen {
		return "", nil, nil, errProtocol
	}
	args = make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		bulk, err := readBulk(r, &buf)
		if err != nil {
			return "", nil, nil, err
		}
		args = append(args, bulk)
		if i == 0 {
			cmd = strings.ToUpper(string(bulk))
		}
	}
	return cmd, args, buf.Bytes(), nil
}
```
NOTE: `readBulk` returns `body[:n]` (a slice into the per-call `body` allocation, not into the shared `raw` buffer) — `args` entries are stable. The inline-path `args[i]` copies the field string into a fresh `[]byte` (independent of `buf`).

- [ ] **Step 5: Update the existing `decodeRequest` callers (compile-fix only; pump wiring is Task 6)**

The 3→4 value signature change ripples to EVERY `decodeRequest` caller. Find them all first:
```bash
grep -rn "decodeRequest(" internal/filter/network/redisproxy/   # confirm the full caller set before editing
```
Update each — insert the new `args` return as `_` (the functional pump wiring is Task 6):
- **`filter.go`** (the pump call, currently `cmd, raw, err := decodeRequest(dr)`):
  ```go
  		cmd, _, raw, err := decodeRequest(dr) // args consumed by classify at Task 6
  ```
- **`filter_test.go`** — the two scripted-upstream goroutines call `_, _, _ = decodeRequest(br)` → `_, _, _, _ = decodeRequest(br)` (both occurrences).
- **`resp_test.go`** — **7 call sites** (currently 3-value): `cmd, raw, err := decodeRequest(...)` (lines ~15, 29, 37) → `cmd, _, raw, err := ...`; `cmd, _, err := decodeRequest(...)` (lines ~50, 73) → `cmd, _, _, err := ...`; `_, _, err := decodeRequest(...)` (line ~58) → `_, _, _, err := ...`; `if _, _, err := decodeRequest(...)` (line ~95) → `if _, _, _, err := ...`. These existing `resp_test.go` tests assert `cmd`/`raw`/`err` only (NOT `args`) — the `_` for `args` keeps them green; the new `args` behavior is asserted by Step 1's `TestDecodeRequest_ExposesArgs`.

Re-run `go build ./internal/filter/network/redisproxy/` after the caller edits to confirm the package compiles before Step 6.

- [ ] **Step 6: Write the `classify` verdict + local-reply set in `commands.go`**

Replace the 32.1 `isLocalReply`/`localReply` functions entirely with:
```go
import (
	"strconv"
	"strings"
	"time"
)

// action is the classify dispatch verdict kind.
type action uint8

const (
	actProxy       action = iota // forward upstream; statCmd set; command.<statCmd>.{total,success,error}
	actLocal                     // write reply in-filter (NO command.* increment); maybe closeAfter
	actUnsupported               // splitter.unsupported_command + write the -ERR unknown command reply
	actInvalid                   // splitter.invalid_request + write the -invalid request reply
)

// commandVerdict is the full dispatch decision for one decoded request (D-S32.2-5).
type commandVerdict struct {
	action     action
	reply      []byte // local/error reply bytes (actLocal/actUnsupported/actInvalid)
	closeAfter bool   // QUIT: close the downstream connection after writing reply
	statCmd    string // lower-cased command name for command.<statCmd>.* (actProxy)
}

// Local-reply byte-stable constants (the upstream wording — parent §11.5/§11.6 +
// §12.4; ResponseValues + makeError → "-<s>\r\n"). DO NOT CHANGE — asserted
// byte-identical cross-side (§8.1). (respPong/respAuthNoPassword stay in resp.go.)
var (
	respOK             = []byte("+OK\r\n")
	respInvalidRequest = []byte("-invalid request\r\n")
	respHelloOptions   = []byte("-ERR HELLO options like AUTH and SETNAME are not supported\r\n")
	respNoProto        = []byte("-NOPROTO unsupported protocol version\r\n")
)

// commandsWithoutMandatoryArgs is the upstream supported_commands.h set of table
// commands legitimately callable with zero arguments (so len(args)==1 is NOT a
// bad-arity reject). The inline singletons (ping/time/quit/auth/echo) are handled
// BEFORE the arity check (classify switch) and need no entry. D-S32.2-3: the exact
// membership is transcribed from upstream supported_commands.h — start from the
// empty set + add only commands the 0055 matrix or upstream confirms zero-arg-legal.
// KEEP-IN-SYNC: upstream supported_commands.h commandsWithoutMandatoryArgs().
var commandsWithoutMandatoryArgs = map[string]struct{}{
	// e.g. "info", "command", "randomkey", "role", "config", "cluster", "select" —
	// transcribe the exact upstream set at IMPL; conservative empty default is safe
	// for the 0055 arms (all send ≥1 arg).
}

// validArity applies the minimal upstream rule: a command with < 2 array elements
// (name only, no args) that is NOT in commandsWithoutMandatoryArgs is invalid.
func validArity(lc string, argc int) bool {
	if argc >= 2 {
		return true
	}
	_, ok := commandsWithoutMandatoryArgs[lc]
	return ok
}

// classify is the dispatch decision for a decoded (cmd UPPERCASED, args) request.
// args[0] is the AS-RECEIVED command token; args[1:] are the arguments.
func classify(cmd string, args [][]byte) commandVerdict {
	// Inline-singleton local-reply commands (NOT in the per-command table — §12.1).
	switch cmd {
	case "PING":
		return commandVerdict{action: actLocal, reply: respPong}
	case "AUTH":
		return commandVerdict{action: actLocal, reply: respAuthNoPassword}
	case "ECHO":
		if len(args) == 2 {
			return commandVerdict{action: actLocal, reply: encodeBulk(args[1])}
		}
		return commandVerdict{action: actInvalid, reply: respInvalidRequest}
	case "TIME":
		return commandVerdict{action: actLocal, reply: encodeTime()}
	case "QUIT":
		return commandVerdict{action: actLocal, reply: respOK, closeAfter: true}
	case "HELLO":
		if v, ok := classifyHello(args); ok {
			return v // error-form local reply; HELLO 2 / no-arg falls through to proxy
		}
	}
	lc, ok := commandSupported(cmd)
	if !ok {
		return commandVerdict{action: actUnsupported, reply: unknownCommandError(args)}
	}
	if !validArity(lc, len(args)) {
		return commandVerdict{action: actInvalid, reply: respInvalidRequest}
	}
	return commandVerdict{action: actProxy, statCmd: lc}
}

// classifyHello returns the local error verdict for a HELLO error-form (>2 args →
// options-unsupported; a non-"2" proto arg incl. "3"/non-numeric → NOPROTO), or
// (_, false) for HELLO 2 / no-arg (proxied — a ClusterScopeCommand; §3.3/§12.4).
func classifyHello(args [][]byte) (commandVerdict, bool) {
	if len(args) > 2 {
		return commandVerdict{action: actLocal, reply: respHelloOptions}, true
	}
	if len(args) == 2 && string(args[1]) != "2" {
		return commandVerdict{action: actLocal, reply: respNoProto}, true
	}
	return commandVerdict{}, false
}

// encodeBulk renders a RESP bulk string "$<len>\r\n<bytes>\r\n" (the ECHO reply).
func encodeBulk(b []byte) []byte {
	out := make([]byte, 0, len(b)+16)
	out = append(out, '$')
	out = strconv.AppendInt(out, int64(len(b)), 10)
	out = append(out, '\r', '\n')
	out = append(out, b...)
	out = append(out, '\r', '\n')
	return out
}

// encodeTime renders the TIME reply: a 2-element array of bulk strings carrying
// the local wall-clock unix seconds + microseconds (NON-DETERMINISTIC — §12.4;
// upstream's dispatcher.timeSource().systemTime()). Shape-tested only.
func encodeTime() []byte {
	now := time.Now()
	secs := strconv.FormatInt(now.Unix(), 10)
	micros := strconv.FormatInt(int64(now.Nanosecond())/1000, 10)
	var b []byte
	b = append(b, '*', '2', '\r', '\n')
	b = append(b, encodeBulk([]byte(secs))...)
	b = append(b, encodeBulk([]byte(micros))...)
	return b
}

// unknownCommandError builds the byte-stable "-ERR unknown command '<name>', with
// args beginning with: \r\n" reply (the upstream wording — parent §11.5; <name> is
// the AS-RECEIVED args[0], original case; EMPTY args-suffix — §8.1). D-S32.2-2: the
// exact form is confirmed LIVE at the 0055 UNKNOWN arm (Task 10).
func unknownCommandError(args [][]byte) []byte {
	var name string
	if len(args) > 0 {
		name = string(args[0])
	}
	return []byte("-ERR unknown command '" + name + "', with args beginning with: \r\n")
}
```
Remove the now-unused 32.1 `isLocalReply`/`localReply` functions and their doc comments.

- [ ] **Step 7: Run all redisproxy tests**

Run: `go test ./internal/filter/network/redisproxy/ -count=1 -v`
Expected: PASS. The 32.1 pump tests still pass (filter.go calls `decodeRequest` with the 4-value signature; the pump's functional behavior is unchanged until Task 6 — `cmd, _, raw, err` discards args and the existing `isLocalReply`/`localReply` references are GONE, so `filter.go`'s `Handle` must still compile). **IMPORTANT:** Task 3 removes `isLocalReply`/`localReply` — `filter.go:114-115` references them. To keep Task 3 green, replace the `filter.go` local-reply branch with a minimal `classify`-based shim NOW:
```go
		if v := classify(cmd, nil); v.action == actLocal { // args wired at Task 6
			// 32.1-equivalent shim: PING/AUTH only (nil args ⇒ ECHO/HELLO fall to proxy
			// or error harmlessly under the unit tests). Task 6 passes real args.
			if _, err := downstream.Write(v.reply); err != nil {
				return
			}
			f.st.addTxBytes(len(v.reply))
			continue
		}
```
This is a TRANSIENT shim — Task 6 replaces the whole pump body. (Alternative, cleaner: do Tasks 3+6 as one combined commit if the subagent prefers a single green step; the SPEC spine keeps them separate for review granularity. If combined, skip this shim and wire the real pump directly.) Note the shim passes `nil` args so only PING/AUTH (which ignore args) behave correctly; the 32.1 tests exercise only PING/SET/GET, all of which the shim handles. Re-run the four 32.1 `TestHandle_*` tests to confirm green.

- [ ] **Step 8: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/resp.go internal/filter/network/redisproxy/commands.go internal/filter/network/redisproxy/filter.go internal/filter/network/redisproxy/resp_test.go internal/filter/network/redisproxy/commands_test.go internal/filter/network/redisproxy/filter_test.go
git commit -m "phase 32.2 Task 3: classify dispatch verdict + ECHO/TIME/QUIT/HELLO local-reply set + decodeRequest arg exposure + QUIT close signal"
```

---

## Task 4: `stats.go` — the EAGER 540 per-command + 2 splitter + 3 cluster counters + inc accessors

**Files:**
- Modify: `internal/filter/network/redisproxy/stats.go`
- Test: `internal/filter/network/redisproxy/stats_test.go`

- [ ] **Step 1: Write the failing roster tests**

Append to `stats_test.go`:
```go
func TestStatRoster32_2_PerCommandAndFixed(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	// 540 per-command counters (180 × 3 slots).
	for _, name := range supportedCommandList {
		for _, slot := range []string{"total", "success", "error"} {
			if c := rs.command(name, slot); c == nil {
				t.Errorf("per-command counter command.%s.%s absent", name, slot)
			} else if c.Load() != 0 {
				t.Errorf("command.%s.%s = %d at creation, want 0", name, slot, c.Load())
			}
		}
	}
	// 2 splitter + 3 REDIS_CLUSTER_STATS fixed counters.
	for _, suf := range []string{
		"splitter.invalid_request", "splitter.unsupported_command",
		"upstream_cx_drained", "max_upstream_unknown_connections_reached", "connection_rate_limited",
	} {
		if c, ok := rs.counters[suf]; !ok {
			t.Errorf("fixed counter %q absent", suf)
		} else if c.Load() != 0 {
			t.Errorf("fixed counter %q = %d at creation, want 0", suf, c.Load())
		}
	}
}

func TestStatRoster32_2_IncAccessors(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	rs.incCommandTotal("get")
	rs.incCommandSuccess("get")
	rs.incCommandError("set")
	rs.incSplitterInvalid()
	rs.incSplitterUnsupported()
	if rs.command("get", "total").Load() != 1 || rs.command("get", "success").Load() != 1 {
		t.Error("incCommandTotal/Success(get)")
	}
	if rs.command("set", "error").Load() != 1 {
		t.Error("incCommandError(set)")
	}
	if rs.counters["splitter.invalid_request"].Load() != 1 || rs.counters["splitter.unsupported_command"].Load() != 1 {
		t.Error("splitter inc accessors")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestStatRoster32_2' -count=1`
Expected: FAIL (compile — `rs.command`, `incCommandTotal`, etc. undefined; the 5 fixed names absent).

- [ ] **Step 3: Extend `stats.go`**

Add the 5 fixed counters to `counterSuffixes` and the per-command roster build. Replace `counterSuffixes` + extend `redisStats`/`newRedisStats`:
```go
// counterSuffixes is the fixed counter roster under redis.<stat_prefix>.: the 6
// downstream counters (32.1) + the 2 splitter.* + 3 REDIS_CLUSTER_STATS (32.2).
var counterSuffixes = []string{
	"downstream_cx_total",
	"downstream_cx_drain_close",
	"downstream_cx_protocol_error",
	"downstream_cx_rx_bytes_total",
	"downstream_cx_tx_bytes_total",
	"downstream_rq_total",
	// 32.2 fixed adds (§4.3):
	"splitter.invalid_request",
	"splitter.unsupported_command",
	"upstream_cx_drained",                      // REDIS_CLUSTER_STATS — exist-at-0 (§12.6)
	"max_upstream_unknown_connections_reached", // REDIS_CLUSTER_STATS — exist-at-0
	"connection_rate_limited",                  // REDIS_CLUSTER_STATS — exist-at-0
}

// commandStat is the eager 3-counter block for one supported command (§4.1).
type commandStat struct {
	total   *stats.Counter
	success *stats.Counter
	errc    *stats.Counter
}

type redisStats struct {
	prefix   string
	counters map[string]*stats.Counter
	gauges   map[string]*stats.Gauge
	commands map[string]*commandStat // keyed by the lower-cased command name (§12.1)
}
```
In `newRedisStats`, after the fixed-counter/gauge loops, build the per-command roster:
```go
	rs.commands = make(map[string]*commandStat, len(supportedCommandList))
	for _, name := range supportedCommandList {
		base := rs.prefix + "command." + name + "."
		rs.commands[name] = &commandStat{
			total:   reg.NewCounterIfAbsent(base + "total"),
			success: reg.NewCounterIfAbsent(base + "success"),
			errc:    reg.NewCounterIfAbsent(base + "error"),
		}
	}
```
Add the accessors:
```go
// command returns the per-command counter for (lower-cased name, slot∈{total,
// success,error}), or nil if the name is not a supported command (a classify
// invariant — the pump only calls these for actProxy verdicts with a table member).
func (rs *redisStats) command(name, slot string) *stats.Counter {
	cs, ok := rs.commands[name]
	if !ok {
		return nil
	}
	switch slot {
	case "total":
		return cs.total
	case "success":
		return cs.success
	case "error":
		return cs.errc
	}
	return nil
}

func (rs *redisStats) incCommandTotal(name string)   { rs.commands[name].total.Inc() }
func (rs *redisStats) incCommandSuccess(name string) { rs.commands[name].success.Inc() }
func (rs *redisStats) incCommandError(name string)   { rs.commands[name].errc.Inc() }

func (rs *redisStats) incSplitterInvalid()     { rs.counters["splitter.invalid_request"].Inc() }
func (rs *redisStats) incSplitterUnsupported() { rs.counters["splitter.unsupported_command"].Inc() }
```
NOTE: `command.<name>.error` uses field `errc` (Go does not allow a field named `error`); the STAT name segment is still `error`.

- [ ] **Step 4: Update the 32.1 roster-size test**

`TestStatRoster32_1_MatchesUpstream` asserts `len(rs.counters) == 6`. The fixed counter roster is now 11 — update that assertion to `11` (and keep the gauge assertion at `4`). Re-pin the 6 downstream names it loops over (unchanged) + note the 5 new fixed names are covered by `TestStatRoster32_2_PerCommandAndFixed`.

- [ ] **Step 5: Run to verify all stats tests pass**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestStatRoster' -count=1 -v`
Expected: PASS (all roster tests). Sanity: `len(rs.commands) == 180`; `len(rs.counters) == 11`.

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/stats.go internal/filter/network/redisproxy/stats_test.go
git commit -m "phase 32.2 Task 4: EAGER 540 per-command counters + 2 splitter + 3 REDIS_CLUSTER_STATS fixed counters + inc accessors"
```

---

## Task 5: `stats.go` — the 2 lifecycle gauges' inc/dec + the `downstream_cx_protocol_error` accessor

**Files:**
- Modify: `internal/filter/network/redisproxy/stats.go`
- Test: `internal/filter/network/redisproxy/stats_test.go`

- [ ] **Step 1: Write the failing gauge/protocol-error tests**

Append to `stats_test.go`:
```go
func TestStatRoster32_2_GaugeAndProtocolError(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	rs.incCxActive()
	rs.incCxActive()
	rs.decCxActive()
	if got := rs.gauges["downstream_cx_active"].Load(); got != 1 {
		t.Errorf("downstream_cx_active = %d, want 1 (2 inc - 1 dec)", got)
	}
	rs.incRqActive()
	rs.decRqActive()
	if got := rs.gauges["downstream_rq_active"].Load(); got != 0 {
		t.Errorf("downstream_rq_active = %d, want 0 (balanced)", got)
	}
	rs.incProtocolError()
	if got := rs.counters["downstream_cx_protocol_error"].Load(); got != 1 {
		t.Errorf("downstream_cx_protocol_error = %d, want 1", got)
	}
	// The 2 buffered gauges stay exist-at-0 (coverage boundary — no inc/dec accessor).
	for _, suf := range []string{"downstream_cx_rx_bytes_buffered", "downstream_cx_tx_bytes_buffered"} {
		if got := rs.gauges[suf].Load(); got != 0 {
			t.Errorf("buffered gauge %q = %d, want 0 (framework-managed coverage boundary)", suf, got)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestStatRoster32_2_GaugeAndProtocolError' -count=1`
Expected: FAIL (compile — `incCxActive`/`decCxActive`/`incRqActive`/`decRqActive`/`incProtocolError` undefined).

- [ ] **Step 3: Add the gauge + protocol-error accessors to `stats.go`**

```go
// The 2 filter-driven lifecycle gauges (§4.4 / §12.3). cx_active: Handle entry/exit
// (LIVE-mirrored — the held-open arm §8.2). rq_active: request-received/reply-sent
// (quiesced-zero post-workload). The 2 *_bytes_buffered gauges get NO accessor —
// they are framework-buffer-managed upstream and stay exist-at-0 (coverage boundary).
func (rs *redisStats) incCxActive() { rs.gauges["downstream_cx_active"].Inc() }
func (rs *redisStats) decCxActive() { rs.gauges["downstream_cx_active"].Dec() }
func (rs *redisStats) incRqActive() { rs.gauges["downstream_rq_active"].Inc() }
func (rs *redisStats) decRqActive() { rs.gauges["downstream_rq_active"].Dec() }

// incProtocolError increments downstream_cx_protocol_error (§4.5) — wired at 32.2
// on the decode-error path (a malformed frame; the 32.1 pump returned silently).
func (rs *redisStats) incProtocolError() { rs.counters["downstream_cx_protocol_error"].Inc() }
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestStatRoster' -count=1 -v`
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/stats.go internal/filter/network/redisproxy/stats_test.go
git commit -m "phase 32.2 Task 5: the 2 lifecycle gauges' inc/dec accessors (cx_active/rq_active) + the downstream_cx_protocol_error accessor"
```

---

## Task 6: `filter.go` — the `Handle` pump wiring (the classify switch + per-command/splitter/gauge/protocol-error + QUIT close)

**Files:**
- Modify: `internal/filter/network/redisproxy/filter.go`
- Modify: `internal/filter/network/redisproxy/doc.go` (32.2 forward-pointers resolved)
- Test: `internal/filter/network/redisproxy/filter_test.go`

- [ ] **Step 1: Write the failing pump tests**

Append to `filter_test.go` (these drive `Handle` over `net.Pipe` with a scripted upstream; the 32.1 `newTestFilter` helper is reused):
```go
// TestHandle_UnknownCommand: BOGUSCMD → splitter.unsupported_command +1, -ERR
// unknown command reply, zero upstream.
func TestHandle_UnknownCommand(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	dialed := false
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { dialed = true; return nil, io.EOF })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("*2\r\n$8\r\nBOGUSCMD\r\n$1\r\nx\r\n"))
		buf := make([]byte, 256)
		n, _ := client.Read(buf)
		if !strings.HasPrefix(string(buf[:n]), "-ERR unknown command 'BOGUSCMD'") {
			t.Errorf("reply = %q, want -ERR unknown command 'BOGUSCMD'…", buf[:n])
		}
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	if dialed {
		t.Error("unknown command dialed upstream; want zero upstream")
	}
	if rs.counters["splitter.unsupported_command"].Load() != 1 {
		t.Errorf("splitter.unsupported_command = %d, want 1", rs.counters["splitter.unsupported_command"].Load())
	}
}

// TestHandle_EchoWrongArity: ECHO (arity 1) → splitter.invalid_request +1.
func TestHandle_EchoWrongArity(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.EOF })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("*1\r\n$4\r\nECHO\r\n"))
		buf := make([]byte, 64)
		n, _ := client.Read(buf)
		if string(buf[:n]) != "-invalid request\r\n" {
			t.Errorf("reply = %q, want -invalid request", buf[:n])
		}
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	if rs.counters["splitter.invalid_request"].Load() != 1 {
		t.Errorf("splitter.invalid_request = %d, want 1", rs.counters["splitter.invalid_request"].Load())
	}
}

// TestHandle_SuccessOnErrorReply: a backend -ERR reply counts command.get.success
// (the round-trip completed — §4.2), NOT error.
func TestHandle_SuccessOnErrorReply(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	upSrv, upClient := net.Pipe()
	go func() {
		br := bufio.NewReader(upSrv)
		_, _, _, _ = decodeRequest(br)
		_, _ = upSrv.Write([]byte("-ERR backend boom\r\n"))
	}()
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return upClient, nil })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
		buf := make([]byte, 64)
		_, _ = client.Read(buf)
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	if rs.command("get", "success").Load() != 1 || rs.command("get", "error").Load() != 0 {
		t.Errorf("GET success/error = %d/%d, want 1/0 (a -ERR reply is success — round-trip completed)",
			rs.command("get", "success").Load(), rs.command("get", "error").Load())
	}
	if rs.command("get", "total").Load() != 1 {
		t.Errorf("GET total = %d, want 1", rs.command("get", "total").Load())
	}
}

// TestHandle_ErrorOnTransportFailure: a dial/Send failure counts command.get.error.
func TestHandle_ErrorOnTransportFailure(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.ErrClosedPipe })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	// total Incs at dispatch (before the failed dial); error Incs on the transport failure.
	if rs.command("get", "total").Load() != 1 || rs.command("get", "error").Load() != 1 {
		t.Errorf("GET total/error = %d/%d, want 1/1 (dispatch then transport failure)",
			rs.command("get", "total").Load(), rs.command("get", "error").Load())
	}
}

// TestHandle_QuitCloses: QUIT → +OK then the pump returns (connection closes).
func TestHandle_QuitCloses(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.EOF })
	down, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		_, _ = client.Write([]byte("*1\r\n$4\r\nQUIT\r\n"))
		buf := make([]byte, len("+OK\r\n"))
		_, _ = io.ReadFull(client, buf)
		if string(buf) != "+OK\r\n" {
			t.Errorf("QUIT reply = %q, want +OK", buf)
		}
		// Do NOT client.Close() — QUIT itself must end the pump (downstream.Close()).
		close(done)
	}()
	go f.Handle(context.Background(), down)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("QUIT did not close the connection")
	}
}

// TestHandle_ProtocolErrorIncrements: a malformed frame → downstream_cx_protocol_error.
func TestHandle_ProtocolErrorIncrements(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.EOF })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("?bad\r\n")) // unknown type byte → errProtocol
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	if rs.counters["downstream_cx_protocol_error"].Load() != 1 {
		t.Errorf("downstream_cx_protocol_error = %d, want 1", rs.counters["downstream_cx_protocol_error"].Load())
	}
}

// TestHandle_CxActiveGauge: cx_active is 1 during Handle, 0 after.
func TestHandle_CxActiveGauge(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.EOF })
	down, client := net.Pipe()
	mid := make(chan int64, 1)
	go func() {
		_, _ = client.Write([]byte("PING\r\n"))
		buf := make([]byte, len("+PONG\r\n"))
		_, _ = io.ReadFull(client, buf)
		mid <- rs.gauges["downstream_cx_active"].Load() // sampled while Handle is live
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	if got := <-mid; got != 1 {
		t.Errorf("downstream_cx_active during Handle = %d, want 1", got)
	}
	if got := rs.gauges["downstream_cx_active"].Load(); got != 0 {
		t.Errorf("downstream_cx_active after Handle = %d, want 0", got)
	}
}
```
Add `"strings"` to `filter_test.go` imports if not present.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/filter/network/redisproxy/ -run 'TestHandle_' -count=1`
Expected: FAIL (the transient Task 3 shim handles only PING/AUTH; unknown/arity/per-command/gauge/protocol-error behaviors are unimplemented).

- [ ] **Step 3: Rewrite the `Handle` pump + add `serveRequest`**

Replace the `Handle` for-loop body + add the `serveRequest` helper. Add `"errors"` to the imports. The new `Handle`:
```go
func (f *filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	f.st.incCxTotal()
	f.st.incCxActive()
	defer f.st.decCxActive()
	dr := bufio.NewReader(downstream)

	var up *network.UpstreamConn
	defer func() {
		if up != nil {
			_ = up.Close()
		}
	}()
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
		cmd, args, raw, err := decodeRequest(dr)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				f.st.incProtocolError() // a malformed frame (§4.5); a clean EOF does not
			}
			return
		}
		f.st.incRqTotal()
		f.st.addRxBytes(len(raw))
		if !f.serveRequest(ctx, downstream, cmd, args, raw, ensureUpstream) {
			return
		}
	}
}

// serveRequest dispatches one decoded request and reports whether the pump should
// continue. It owns the downstream_rq_active gauge inc/dec (defer-balanced across
// every exit path — §4.4).
func (f *filter) serveRequest(ctx context.Context, downstream net.Conn, cmd string, args [][]byte, raw []byte,
	ensureUpstream func() (*network.UpstreamConn, error)) (cont bool) {
	f.st.incRqActive()
	defer f.st.decRqActive()

	v := classify(cmd, args)
	switch v.action {
	case actLocal:
		if _, err := downstream.Write(v.reply); err != nil {
			return false
		}
		f.st.addTxBytes(len(v.reply))
		return !v.closeAfter // QUIT ends the pump
	case actUnsupported:
		f.st.incSplitterUnsupported()
		if _, err := downstream.Write(v.reply); err != nil {
			return false
		}
		f.st.addTxBytes(len(v.reply))
		return true
	case actInvalid:
		f.st.incSplitterInvalid()
		if _, err := downstream.Write(v.reply); err != nil {
			return false
		}
		f.st.addTxBytes(len(v.reply))
		return true
	default: // actProxy
		f.st.incCommandTotal(v.statCmd)
		u, err := ensureUpstream()
		if err != nil {
			f.st.incCommandError(v.statCmd) // unresolvable upstream is a transport failure
			return false
		}
		if err := u.Send(ctx, raw); err != nil {
			f.st.incCommandError(v.statCmd)
			return false
		}
		reply, err := decodeReply(u.Reader())
		if err != nil {
			f.st.incCommandError(v.statCmd)
			return false
		}
		f.st.incCommandSuccess(v.statCmd)
		if _, err := downstream.Write(reply); err != nil {
			return false
		}
		f.st.addTxBytes(len(reply))
		return true
	}
}
```
NOTE on `actProxy` error accounting: the 32.1 unresolvable-cluster path returned WITHOUT a per-command increment (there was no per-command stat). 32.2 counts `command.<cmd>.error` on ANY transport/pool failure (dial/ensureUpstream miss, Send error, decodeReply error) — faithful to the single-server `onFailure` path (§4.2). The `total` Incs once at dispatch (before `ensureUpstream`), so a failed dial yields total=1, error=1 (TestHandle_ErrorOnTransportFailure).

Remove the transient Task 3 shim. Delete the now-unused `isLocalReply`/`localReply` references (already removed in Task 3).

- [ ] **Step 4: Resolve the `doc.go` 32.2 forward-pointers**

Update the `doc.go` 32.2 paragraph (lines 22-25) to past-tense "as-landed" wording: the full command set + the per-command/splitter/REDIS_CLUSTER roster + the gauges' inc/dec + the `downstream_cx_protocol_error` wiring are NOW landed; the `redis.` Prometheus arm lands at Task 7; the 41st fuzzer at Task 8; the differential matrix at Tasks 10-11; the parent-row-32 rollup at Task 12.

- [ ] **Step 5: Run the full redisproxy suite (incl. the 32.1 regression tests)**

Run: `go test ./internal/filter/network/redisproxy/ -count=1 -v`
Expected: PASS (all new `TestHandle_*` + the 32.1 `TestHandle_PingLocalReply_NoUpstream`/`ProxiedRoundTrip_SetThenGet`/`UnknownClusterGracefulClose`/`EOFCleanClose`). The 32.1 SET/GET test now also exercises `command.set.*`/`command.get.*` — confirm it still passes (the reply bytes are unchanged).

- [ ] **Step 6: Race + gofmt + lint + commit**

```bash
go test ./internal/filter/network/redisproxy/ -race -short -count=1
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/filter.go internal/filter/network/redisproxy/doc.go internal/filter/network/redisproxy/filter_test.go
git commit -m "phase 32.2 Task 6: Handle pump wiring — classify switch + per-command total/success/error + splitter increments + cx_active/rq_active gauges + cx_protocol_error + QUIT close"
```

---

## Task 7: `internal/stats/name.go` — the `redis.` LABEL-HOISTED Prometheus arm

**Files:**
- Modify: `internal/stats/name.go`
- Test: `internal/stats/name_test.go`

- [ ] **Step 1: Write the failing arm tests**

Append to `name_test.go` (match the existing table-test shape in that file):
```go
func TestFlattenToProm_RedisArm(t *testing.T) {
	cases := []struct {
		in        string
		wantBase  string
		wantLabel string // the single envoy_redis_prefix value
	}{
		{"redis.redis_r.downstream_cx_total", "envoy_redis_downstream_cx_total", "redis_r"},
		{"redis.redis_r.command.get.total", "envoy_redis_command_get_total", "redis_r"},
		{"redis.redis_r.command.bf.add.total", "envoy_redis_command_bf_add_total", "redis_r"}, // dotted name flatten
		{"redis.redis_r.command.info.shard.success", "envoy_redis_command_info_shard_success", "redis_r"},
		{"redis.redis_r.splitter.unsupported_command", "envoy_redis_splitter_unsupported_command", "redis_r"},
		{"redis.redis_r.downstream_cx_active", "envoy_redis_downstream_cx_active", "redis_r"}, // gauge
	}
	for _, tc := range cases {
		base, labels, err := flattenToProm(tc.in)
		if err != nil {
			t.Errorf("flattenToProm(%q) err = %v", tc.in, err)
			continue
		}
		if base != tc.wantBase {
			t.Errorf("flattenToProm(%q) base = %q, want %q", tc.in, base, tc.wantBase)
		}
		if len(labels) != 1 || labels[0].Key != "envoy_redis_prefix" || labels[0].Value != tc.wantLabel {
			t.Errorf("flattenToProm(%q) labels = %+v, want [{envoy_redis_prefix %q}]", tc.in, labels, tc.wantLabel)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/stats/ -run 'TestFlattenToProm_RedisArm' -count=1`
Expected: FAIL — `redis.*` falls to the final `default` error (`no recognized top-level segment`).

- [ ] **Step 3: Add the `redis.` arm to the default branch of `flattenToProm`**

Insert AFTER the kafka arm (`name.go:298-301`) and BEFORE the final `return "", nil, fmt.Errorf(...)`:
```go
		// Phase-32.2 redis_proxy SINGLE-label HOIST (ADR-0229; AMEND-R4; the mongo
		// .rbac. ADR-0218 single-label promotion generalized to a redis. ROOT prefix).
		// Internal name redis.<stat_prefix>.<rest> → envoy_redis_<rest flattened>
		// {envoy_redis_prefix="<stat_prefix>"}. Only the stat_prefix is hoisted to a
		// LABEL; the command name STAYS in the metric name (NOT a label — contrast the
		// mongo cmd/collection VALUE hoist; the dynamic command.<cmd>.* names flatten
		// identically, command.get.total → envoy_redis_command_get_total). The dotted
		// module/info.shard names (command.bf.add.total) flatten dot→underscore. SHAPE
		// validation (dot-free <prefix>) — an allowlist is impossible given the 180
		// dynamic command names (the wasm/zookeeper/kafka permissive precedent); a wire
		// command not in the static table never produces a name (routes to splitter.
		// unsupported_command — IsValidName by construction, §5.2). KEEP-IN-SYNC:
		// internal/filter/network/redisproxy/stats.go (the roster name builders).
		if rest, ok := strings.CutPrefix(internal, "redis."); ok {
			if idx := strings.IndexByte(rest, '.'); idx > 0 {
				prefix, tail := rest[:idx], rest[idx+1:]
				if !strings.ContainsRune(prefix, '.') {
					labels = append(labels, Label{Key: "envoy_redis_prefix", Value: prefix})
					base = "envoy_redis_" + strings.ReplaceAll(tail, ".", "_")
					return base, labels, nil
				}
			}
		}
```

- [ ] **Step 4: Run to verify it passes (+ the full stats suite — no regressions)**

Run: `go test ./internal/stats/ -count=1`
Expected: PASS (the new arm + every existing name-mapping test — the `redis.` arm is a new branch, the kafka/mongo/zookeeper/rbac arms are untouched).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/stats/
golangci-lint run ./internal/stats/...
git add internal/stats/name.go internal/stats/name_test.go
git commit -m "phase 32.2 Task 7: the redis. SINGLE-label HOIST Prometheus tag-extractor arm (envoy_redis_<leaf>{envoy_redis_prefix})"
```

---

## Task 8: `FuzzRESPDecode` — the 41st fuzzer (no-panic / no-mutation / bounded-allocation)

**Files:**
- Create: `internal/filter/network/redisproxy/fuzz_test.go`

- [ ] **Step 1: Write the fuzzer (the kafka `FuzzKafkaDecode` shape, adapted to the reader-based RESP codec)**

```go
package redisproxy

import (
	"bufio"
	"bytes"
	"testing"
)

// FuzzRESPDecode is the 41st fuzzer (SPEC §9). It feeds arbitrary bytes through
// BOTH resp.go decode entry points (decodeRequest + decodeReply) via a bufio.Reader
// over a bytes.Reader and asserts: (1) no panic; (2) no mutation of the input slice
// (the decoder reads, never writes back); (3) bounded allocation — a crafted length
// header never allocates beyond maxBulkLen (512 MiB) / maxArrayLen (1 Mi) before the
// overflow guards reject it (resp.go:14-17). Per reference_dynamic_stat_name_charset
// _guard the codec touches NO registry (the per-command stat lookup is table-bounded
// in filter.go, not resp.go) — the fuzzer scope is the codec only.
func FuzzRESPDecode(f *testing.F) {
	seeds := [][]byte{
		[]byte("PING\r\n"),                                            // inline
		[]byte("*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"),        // valid array request
		[]byte("+OK\r\n"),                                             // reply: simple string
		[]byte("-ERR x\r\n"),                                          // reply: error
		[]byte(":1\r\n"),                                              // reply: integer
		[]byte("$3\r\nbar\r\n"),                                       // reply: bulk
		[]byte("$-1\r\n"),                                             // reply: null bulk
		[]byte("*2\r\n$1\r\na\r\n$1\r\nb\r\n"),                        // reply: array
		[]byte("*-1\r\n"),                                             // reply: null array
		[]byte("$10\r\nshort"),                                        // partial frame
		[]byte("$999999999999\r\n"),                                  // overflow length
		[]byte("?xyz\r\n"),                                           // bad type byte
		[]byte(":abc\r\n"),                                           // non-numeric integer
		[]byte("*1\r\n$3\r\nbf.\r\n"),                                // dotted-name request
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit). Drive both entry points over fresh readers.
		_, _, _, _ = decodeRequest(bufio.NewReader(bytes.NewReader(data)))
		_, _ = decodeReply(bufio.NewReader(bytes.NewReader(data)))

		// Invariant 2: the input was never mutated.
		if !bytes.Equal(data, orig) {
			t.Fatal("decode mutated the input bytes")
		}
		// Invariant 3 (bounded allocation) is enforced by the resp.go overflow guards
		// (maxBulkLen/maxArrayLen checked BEFORE make()); a panic or OOM here would
		// fail the run. No explicit assertion — the guards are the bound (D-S32.2-8).
	})
}
```

- [ ] **Step 2: Run the fuzzer over its seed corpus + a short fuzz burst**

```bash
go test ./internal/filter/network/redisproxy/ -run 'FuzzRESPDecode' -count=1   # seed corpus, fast
go test ./internal/filter/network/redisproxy/ -run '^$' -fuzz 'FuzzRESPDecode' -fuzztime 20s
```
Expected: PASS (no panic, no mutation, no OOM/hang). If a crafted `$<huge>\r\n`-no-body input is slow, confirm the `maxBulkLen` guard rejects it (it returns `errProtocol` before `make()` for over-cap lengths; an in-cap declared length errors at `io.ReadFull`).

- [ ] **Step 3: Confirm the fuzzer count is 41**

```bash
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 41
```

- [ ] **Step 4: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/redisproxy/
golangci-lint run ./internal/filter/network/redisproxy/...
git add internal/filter/network/redisproxy/fuzz_test.go
git commit -m "phase 32.2 Task 8: the 41st fuzzer FuzzRESPDecode — no-panic / no-mutation / bounded-allocation over decodeRequest+decodeReply"
```

---

## Task 9: `TCPRedisResponder` reply-table extension (`$-1` GET-miss, `:1` INCR/DEL)

**Files:**
- Modify: `test/differential/runner_test.go` (the `redisRespondLoop` switch + first-arg capture)

- [ ] **Step 1: Extend `redisRespondLoop` to capture the first argument + the new replies**

In `runner_test.go`'s `redisRespondLoop` (≈line 2433): capture the first discarded bulk argument (the key) so GET can distinguish hit/miss, and extend the reply switch. In the `for i := 1; i < n; i++` discard loop, capture `i==1`:
```go
		var firstArg string
		for i := 1; i < n; i++ {
			hdr, err := r.ReadString('\n')
			if err != nil {
				return
			}
			hdr = strings.TrimRight(hdr, "\r\n")
			if len(hdr) < 2 || hdr[0] != '$' {
				return
			}
			var argLen int
			if _, err := fmt.Sscanf(hdr[1:], "%d", &argLen); err != nil || argLen < 0 {
				return
			}
			argBytes := make([]byte, argLen+2) // +2 for \r\n
			if _, err := io.ReadFull(r, argBytes); err != nil {
				return
			}
			if i == 1 {
				firstArg = strings.TrimRight(string(argBytes), "\r\n")
			}
		}
```
Then the reply switch:
```go
		var reply string
		switch cmd {
		case "SET":
			reply = "+OK\r\n"
		case "GET":
			if firstArg == "nope" {
				reply = "$-1\r\n" // GET-miss (null bulk — §8.4; the driver's miss key is "nope")
			} else {
				reply = "$3\r\nbar\r\n" // GET-hit
			}
		case "INCR", "DEL":
			reply = ":1\r\n"
		default:
			reply = "-ERR unsupported\r\n"
		}
```
Update the `redisRespondLoop` doc comment: the new reply table (`$-1` GET-miss keyed on the first arg "nope", `:1` INCR/DEL; the 32.2 command-matrix extension).

- [ ] **Step 2: Confirm the differential package still builds + the existing `0055`/`0056` pass (the responder change is additive)**

```bash
go build ./test/...
go test ./test/differential/ -run 'TestDifferential/0055|TestDifferential/0056' -count=1 -v
```
Expected: PASS (the 32.1 SET/GET arm — `GET foo` (firstArg "foo" ≠ "nope") → `$3\r\nbar\r\n`, unchanged). The new replies are exercised by the Task 10 arms.

> **NOTE (Docker):** the `0055` differential boots the contrib reference Envoy in Docker (`reference_docker_probe_bridge_network`). If Docker is unavailable in the IMPL environment, run the redisproxy unit/fuzz tests + `go build ./test/...` here and DEFER the full `0055` run to the six-gate (Task 12) where the differential suite runs in the CI-capable environment. Record the deferral honestly in `PROGRESS.md`.

- [ ] **Step 3: gofmt + commit**

```bash
gofmt -l test/differential/
git add test/differential/runner_test.go
git commit -m "phase 32.2 Task 9: TCPRedisResponder reply-table extension — \$-1 GET-miss (key 'nope'), :1 INCR/DEL"
```

---

## Task 10: `0055` driver — the command-matrix arms + the `/stats/prometheus` per-command/splitter assertions

**Files:**
- Modify: `test/fixtures/0055-redis-roundtrip/driver/driver.go`
- Modify: `test/fixtures/0055-redis-roundtrip/README.md`

This task extends the `driveProxy` workload with the command matrix (§8.1) and adds the `/stats/prometheus` label-aware assertions for the per-command + splitter counters. The held-open gauge arm + the quiesced-zero + the R6 break recording are Task 11.

- [ ] **Step 1: Add the command-matrix arms to `driveProxy`**

After the existing PING (arm 1) + SET/GET (arm 2) arms, add the §8.1 arms. Each uses a FRESH connection (the per-arm precedent — sequential, so `upstream_cx_total` equality holds per arm, D-P32-9) and writes the reply bytes into the verdict stream via `emitArmBytes` (the byte-equivalence prong). Use the existing `respArray`/`inline` builders + `readReply`/`readGetReply`. The arms (request → expected reply):

| Arm | Request | Expected reply (byte-equivalence) |
|---|---|---|
| GET-miss | `respArray("GET","nope")` | `$-1\r\n` |
| INCR | `respArray("INCR","ctr")` | `:1\r\n` |
| DEL | `respArray("DEL","foo")` | `:1\r\n` |
| ECHO | `respArray("ECHO","hi")` | `$2\r\nhi\r\n` |
| ECHO wrong-arity | `respArray("ECHO")` | `-invalid request\r\n` |
| QUIT | `respArray("QUIT")` | `+OK\r\n` + conn closes |
| HELLO-3 | `respArray("HELLO","3")` | `-NOPROTO unsupported protocol version\r\n` |
| HELLO-options | `respArray("HELLO","2","AUTH","u","p")` | `-ERR HELLO options like AUTH and SETNAME are not supported\r\n` |
| UNKNOWN | `respArray("BOGUSCMD","x")` | `-ERR unknown command 'BOGUSCMD', with args beginning with: \r\n` |
| PING-with-arg | `respArray("PING","hello")` | `+PONG\r\n` |

Implement each as a small `driveXArm(ctx, addr) ([]byte, error)` that opens a fresh conn, writes the request, reads the reply (use a generic `readReplyN` that reads up to N bytes — the existing `readReply` reads one chunk; for multi-line replies a single `Read` suffices since these are small single-frame replies), and returns the reply bytes. Append each via `emitArmBytes(&b, side, "<name>", reply, err)`. The QUIT arm reads `+OK\r\n` then confirms the conn closes (a follow-up `Read` returns EOF) — emit the `+OK\r\n` bytes.

**D-S32.2-2 byte-stable confirmation (the UNKNOWN arm):** the reference's exact `-ERR unknown command …` bytes are captured by the runner's `CompareBytes` — if the subject's `unknownCommandError` (Task 3) wording differs from the reference, the differential FAILS with a byte-divergence. On first run, set `FIXTURE_0055_DUMP_STATS`-style diagnostic (or inspect the divergence offset) to read the reference bytes, then correct Task 3's `unknownCommandError` constant + this arm's expected bytes in lockstep. Record the confirmed bytes in the README + `PROGRESS.md`.

- [ ] **Step 2: Upgrade `AssertStats` with the `/stats/prometheus` per-command + splitter assertions**

Add a `scrapeProm(adminAddr) (map[string]int64, error)` helper (the kafka `0053` `scrapeKafkaStats`/`canonicalize` shape — copy `canonicalize` + `httpGet` into the driver; filter lines beginning `envoy_redis_`). KEEP the existing flat-`/stats` `scrapeStats`/counter assertions (the 32.1 6 counters — D-S32.2-6). Add the per-command + splitter assertions keyed by the canonicalized prom name:
```go
	refP, err := scrapeProm(refAdminAddr)
	// … subjP …
	// Per-command + splitter expectations (cross-side EQUAL). Values from the arm
	// accounting: GET total = SET/GET arm(1) + GET-miss(1) = 2; INCR/DEL each 1; etc.
	promEqual := []string{
		`envoy_redis_command_get_total{envoy_redis_prefix="redis_r"}`,
		`envoy_redis_command_get_success{envoy_redis_prefix="redis_r"}`,
		`envoy_redis_command_incr_total{envoy_redis_prefix="redis_r"}`,
		`envoy_redis_command_del_total{envoy_redis_prefix="redis_r"}`,
		`envoy_redis_splitter_invalid_request{envoy_redis_prefix="redis_r"}`,
		`envoy_redis_splitter_unsupported_command{envoy_redis_prefix="redis_r"}`,
	}
	for _, raw := range promEqual {
		key := canonicalize(raw)
		rv, rok := refP[key]
		sv, sok := subjP[key]
		if !rok { t.Errorf("ref: %s ABSENT in /stats/prometheus", raw); continue }
		if !sok { t.Errorf("subj: %s ABSENT in /stats/prometheus", raw); continue }
		if rv != sv { t.Errorf("cross-side mismatch %s: ref=%d subj=%d", raw, rv, sv) }
	}
```
Confirm the exact per-command counts against the arm set live (the GET total = 2 because BOTH the SET/GET arm's GET and the GET-miss arm increment `command.get.total`; GET success = 2 as well). Re-derive each expected count from the arm table and record in the README accounting block. (`command.get.error` should be 0 — both GETs got replies.)

- [ ] **Step 3: Run the `0055` differential (cross-side)**

```bash
go test ./test/differential/ -run 'TestDifferential/0055' -count=1 -v
```
Expected: PASS (byte-equivalence over all arms + the per-command/splitter prometheus equality). If Docker is unavailable, DEFER to the six-gate (Task 12) per the Task 9 note; record honestly.

- [ ] **Step 4: Update the README arm table + commit**

```bash
gofmt -l test/fixtures/0055-redis-roundtrip/driver/
git add test/fixtures/0055-redis-roundtrip/driver/driver.go test/fixtures/0055-redis-roundtrip/README.md
git commit -m "phase 32.2 Task 10: 0055 command-matrix arms (GET-miss/INCR/DEL/ECHO/QUIT/HELLO-err/UNKNOWN/PING-arg) + per-command/splitter /stats/prometheus assertions"
```

---

## Task 11: `0055` driver — the `cx_active` held-open gauge arm + the quiesced-zero assertions + the R6 break recording

**Files:**
- Modify: `test/fixtures/0055-redis-roundtrip/driver/driver.go`
- Modify: `test/fixtures/0055-redis-roundtrip/README.md`

- [ ] **Step 1: Add the held-open connection fields + the held-open arm (D-S32.2-7)**

Add two fields to `redisRoundtripDriver` (it was `struct{}`; the held conns are the only mutable cross-arm state — the mongo `op_query_active` 29.2 precedent):
```go
type redisRoundtripDriver struct {
	refHeld  net.Conn // idle PING'd connection held open across AssertStats (ref side)
	subjHeld net.Conn // …subject side; closed in AssertStats after the gauge assertion
}
```
At the END of `driveProxy` (AFTER all transient matrix arms have opened+closed their fresh conns, so the held conn is the ONLY live downstream connection at scrape time), open a held connection, send `PING`, read `+PONG`, and DO NOT close it — store it in the side's field:
```go
	// Held-open gauge arm (§8.2): keep one idle PING'd connection alive across the
	// AssertStats prometheus scrape so downstream_cx_active == 1 on BOTH sides. Closed
	// in AssertStats after the gauge assertion. Opened LAST so it is the only live conn.
	held, err := openHeld(ctx, addr) // dial + write PING + read +PONG; return the open conn
	if err != nil {
		return nil, fmt.Errorf("held-open arm: %w", err)
	}
	if side == "ref" {
		d.refHeld = held
	} else {
		d.subjHeld = held
	}
```
NOTE: `driveProxy` is a pointer-receiver method (`d *redisRoundtripDriver`) — it already is. `DriveReference`/`DriveSubject` run on the same `d` instance; the ref/subj fields are distinct so concurrent writes are race-free (different fields). The held conn's `PING` keeps it parked in `Handle`'s `decodeRequest` block (`cx_active`==1).

- [ ] **Step 2: Assert the gauge + quiesced-zero in `AssertStats`, then close the held conns**

In `AssertStats`, after the per-command/splitter assertions, assert the gauge held arm + the quiesced-zero gauges, then close both held conns:
```go
	// downstream_cx_active == 1 on BOTH sides (the held-open arm — §8.2). The gauge
	// renders with a # TYPE gauge line; scrapeProm reads the value identically.
	for _, sd := range []struct{ label string; p map[string]int64 }{{"ref", refP}, {"subj", subjP}} {
		key := canonicalize(`envoy_redis_downstream_cx_active{envoy_redis_prefix="redis_r"}`)
		if got := sd.p[key]; got != 1 {
			t.Errorf("%s: downstream_cx_active = %d, want 1 (held-open arm)", sd.label, got)
		}
		// rq_active quiesces to 0 post-workload (§4.4); assert PRESENT (created eager)
		// AND == 0 (a non-present check would pass vacuously).
		rqk := canonicalize(`envoy_redis_downstream_rq_active{envoy_redis_prefix="redis_r"}`)
		if got, ok := sd.p[rqk]; !ok {
			t.Errorf("%s: downstream_rq_active ABSENT (created eager — should render)", sd.label)
		} else if got != 0 {
			t.Errorf("%s: downstream_rq_active = %d, want 0 (quiesced)", sd.label, got)
		}
		// The 2 buffered gauges quiesce to 0 (framework-managed coverage boundary).
		for _, q := range []string{"downstream_cx_rx_bytes_buffered", "downstream_cx_tx_bytes_buffered"} {
			qk := canonicalize(`envoy_redis_` + q + `{envoy_redis_prefix="redis_r"}`)
			if got, ok := sd.p[qk]; ok && got != 0 {
				t.Errorf("%s: %s = %d, want 0 (coverage-boundary)", sd.label, q, got)
			}
		}
	}
	// Close the held conns (cx_active → 0); a cleanup also guards a mid-assertion fatal.
	if d.refHeld != nil { _ = d.refHeld.Close() }
	if d.subjHeld != nil { _ = d.subjHeld.Close() }
```
Add a `t.Cleanup(func(){ if d.refHeld != nil { _ = d.refHeld.Close() }; if d.subjHeld != nil { _ = d.subjHeld.Close() } })` at the top of `AssertStats` as a belt-and-suspenders guard against a held conn leaking on a `t.Fatalf`.

NOTE on the scrape timing: the held conn is opened in `driveProxy` and `driveProxy` already sleeps `settleDelay` before returning; `AssertStats` scrapes after both Drive calls return. So at scrape time only the two held conns are alive (one per side) → each side's `cx_active`==1. Confirm `scrapeProm` reads gauge values (Prometheus emits `# TYPE … gauge` then `name{labels} value` — the `scrapeProm` parser skips `#` lines and reads the value line identically to a counter).

- [ ] **Step 3: Run the `0055` differential**

```bash
go test ./test/differential/ -run 'TestDifferential/0055' -count=1 -v
```
Expected: PASS (the gauge held arm + quiesced-zero + all Task 10 arms). Defer to the six-gate if Docker is unavailable (record honestly).

- [ ] **Step 4: R6 deliberate-break liveness proof (record in the driver comment + README + PROGRESS.md)**

For EACH new assertion category, prove it LIVE by temporarily perturbing the driver/production and running with `-count=1` (per `reference_differential_break_protocol_count1` — go-test caching serves a stale PASS otherwise; after a production break, restore via `git restore`, NOT `checkout`/`amend`, per `feedback_subagent_worktree_detach`). Minimum break set + the expected FAIL signature:
  - **Break A — a per-command counter (`command.incr.total`):** perturb the expected value (e.g. assert `!= 99`) → FAIL "cross-side mismatch … ref=1 subj=1" vs the wrong expectation; confirms the per-command prom assertion is live.
  - **Break B — `splitter.unsupported_command`:** same → confirms the splitter assertion is live (value 1 from the UNKNOWN arm).
  - **Break C — `downstream_cx_active` held arm:** temporarily close the held conn BEFORE the scrape (or assert `!= 1`) → FAIL "downstream_cx_active = 0, want 1"; confirms the gauge held arm is live.
  - **Break D — a new arm's reply bytes (e.g. GET-miss `$-1`):** append a stray byte to the subject's reply in `driveProxy` (`if side == "subj" { reply = append(reply, '!') }`) → FAIL "differential mismatch: first divergence at offset N"; confirms the CompareBytes prong is live for the new arms.
  - **Break E — the UNKNOWN-command reply (D-S32.2-2):** confirms the byte-stable `-ERR unknown command` wording matches the reference (Task 10 Step 1 already captures this; record the confirmed bytes).
Revert each break → PASS. Record each break's exact FAIL line in the driver's `AssertStats` doc comment (the 32.1 R6 block precedent) + the README.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l test/fixtures/0055-redis-roundtrip/driver/
git add test/fixtures/0055-redis-roundtrip/driver/driver.go test/fixtures/0055-redis-roundtrip/README.md
git commit -m "phase 32.2 Task 11: 0055 cx_active held-open gauge arm (==1 both sides) + rq_active/buffered quiesced-zero + R6 break-liveness records"
```

---

## Task 12: Completion bundle — ADR-0229 body + BEHAVIOR_CONTRACT + STATE + ROADMAP rollup + next-prompt + the six-gate

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0229 §Decision/§Consequences 32.2 half — PARTIAL → ACCEPTED, in-place)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the 32.2 bundle + the stat-table 546 → 1091)
- Modify: `docs/envoy-go/STATE.md`
- Modify: `docs/envoy-go/ROADMAP.md` (parent row 32 + sub-row 32.2 ATOMIC ROLLUP)
- Modify: `next-prompt.txt` (the next-phase cold-start)

- [ ] **Step 1: Complete the ADR-0229 §Decision/§Consequences body (PARTIAL → ACCEPTED, in-place)**

In `DECISIONS.md`, the ADR-0229 body has a PARTIAL status (the 32.1 half landed). Complete the 32.2 half IN-PLACE (NO new ADR number — DECISIONS.md tail STAYS ADR-0230; ADR-0044 §Decision/§Consequences-at-IMPL): the full command set + dispatch (§3), the per-command/splitter/cluster roster + the single-server success/error semantics (§4), the `redis.` arm (§5), the 2 lifecycle gauges (§4.4), the differential command matrix (§8), the 41st fuzzer (§9), the parent-row-32 ROLLUP (§10). The §Consequences gains: stat surface 546 → **1091**; fuzzers 40 → **41**; fixtures **58** (extended, no new dir); BackendKind tail **32** (unchanged); the §9 family candidate count 2 → **1** ({thrift}). Flip the ADR-0229 status header PARTIAL → **ACCEPTED**.

- [ ] **Step 2: Extend the BEHAVIOR_CONTRACT 32.2 bundle (§10)**

Extend the `### envoy.filters.network.redis_proxy` subsection (the full command set + the unknown→splitter / bad-arity→splitter dispatch; the per-command `command.<cmd>.{total,success,error}` roster + the success-on-any-reply / error-on-transport-failure semantics; the ECHO/TIME/QUIT/HELLO local-reply set [TIME non-deterministic; HELLO split]; the 2 lifecycle gauges' inc/dec [cx_active mirrored; rq_active quiesced] + the 2 buffered-gauge coverage boundary; the `downstream_cx_protocol_error` wiring). Add the NEW `### Prometheus exposition — the redis. tag-extractor arm` note (the `envoy_redis_<leaf>{envoy_redis_prefix="<sp>"}` single-label hoist; the kafka-INLINE / mongo-MULTI contrast). Add the coverage-boundary/departure records (§10: latency histograms unmirrored [ADR-0060]; `*_fault` deferred; the 2 `*_bytes_buffered` exist-at-0; the 3 `REDIS_CLUSTER_STATS` exist-at-0; TIME unit-test-only; HELLO 2 proxied; multi-key fragmentation deferred; one-conn-per-downstream per-side pooling divergence [D-P32-9]; `op_timeout` parsed-not-consumed; `enable_command_stats` no-op; runtime-keys-at-defaults; close-direction-zero-touch [AMEND-R9]). Add the stat-table 546 → **1091** block (+545 rows: 2 splitter + 3 cluster + 540 per-command). Add the parent-row-32 family ROLLUP note (ADR-0106(d)).

- [ ] **Step 3: Run the six-gate (per SPEC §15.2; quote every output into PROGRESS.md)**

```bash
go build ./...
go vet ./...
golangci-lint run
go test ./... -race -short -count=1
go test ./test/differential/ -count=1                 # the FULL 58-dir suite byte-exact (incl. 0055 extended + the back-compat gate)
# conformance (image-independent — asserted UNAFFECTED; re-run if the harness is available):
#   h2spec 53/53 + proxy-wasm 10/10
```
Expected: all green. The full 58-dir differential re-runs byte-exact (the redisproxy + name.go changes are ADDITIVE — they activate only for a redis_proxy terminal / `redis.` stat names). h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected (phase 32.2 touches no HTTP/h2/proxy-wasm path). Quote each output verbatim into `PROGRESS.md` (run honestly — if Docker is unavailable for the differential, state which gates ran and which were deferred).

- [ ] **Step 4: Update STATE.md + the ROADMAP ATOMIC ROLLUP**

- `STATE.md`: active-phase → `phase 32.2 IMPL done`; lifecycle-state → the next-phase cold-start; counts (stat surface **1091** / fixtures **58** / fuzzers **41** / BackendKind tail **32** / DECISIONS tail **ADR-0230**, next-free **ADR-0231**); the §9 family stays OPEN ({thrift} remains).
- `ROADMAP.md`: **parent row 32 `in-progress → done` ATOMICALLY with sub-row 32.2 `in-progress → done`** (the §9 in-row ROLLUP per ADR-0106(d) — the 18/19/22/24/25/26/28/29 precedent). Confirm sub-row 32.1 stays `done`.

- [ ] **Step 5: Rewrite `next-prompt.txt` for the next-phase cold-start**

The §9 Network-filters family stays OPEN with ONE candidate ({thrift}) remaining (it reuses the 32.1 upstream-pool seam ADR-0230). Write the next-prompt for the {thrift} BRAINSTORM (or the next roadmap item per the ROADMAP ordering), carrying the 32.2 as-built outputs + the relevant memories.

- [ ] **Step 6: Final commit (the atomic completion bundle — ADR-0052)**

```bash
gofmt -l .
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md next-prompt.txt docs/envoy-go/phases/32.2-network-filter-redis-commands-and-stats/PROGRESS.md
git commit -m "phase 32.2 Task 12: completion bundle — ADR-0229 ACCEPTED (32.2 half) + BEHAVIOR_CONTRACT 546→1091 + STATE + ROADMAP parent-row-32 ROLLUP + next-prompt + six-gate green"
```

At stage-close the controller squash-merges `phase-32.2-network-filter-redis-commands-and-stats` → master + pushes to origin (`feedback_push_to_origin`; subagents committed LOCAL-ONLY per `feedback_subagents_no_push`), cleans up the worktree, and deletes the branch.

---

## Acceptance checklist (SPEC §15.3 — verify before the stage-close)

1. The full command set lands per §3 (the 180-table dispatch + unknown/bad-arity splitter handling + the ECHO/TIME/QUIT/HELLO local-reply set); the per-command/splitter/cluster roster + the 2 lifecycle gauges' inc/dec + the `cx_protocol_error` wiring land per §4. *(Tasks 2–6.)*
2. The `redis.` LABEL-HOISTED Prometheus arm lands per §5; `TestCommandRoster_AllValidNames` proves IsValidName-by-construction (no per-wire-byte guard). *(Tasks 2, 7.)*
3. `FuzzRESPDecode` (the 41st) lands per §9; counts: fuzzers 40 → **41**, stat surface 546 → **1091**, fixtures **58** (extended, no new dir), BackendKind tail **32** (unchanged). *(Task 8.)*
4. `0055` extended with the command matrix + the gauge held-open arm + the `/stats/prometheus` assertions; the `TCPRedisResponder` reply table extended; every new assertion proven live (R6 `-count=1`). *(Tasks 9–11.)*
5. The ADR-0229 §Decision/§Consequences 32.2 half completes in-place (PARTIAL → ACCEPTED; DECISIONS.md tail STAYS ADR-0230; no new number); the BEHAVIOR_CONTRACT 32.2 bundle lands (§10). *(Task 12.)*
6. Six gates green (§15.2); STATE.md advanced; **ROADMAP parent row 32 `in-progress → done` ATOMICALLY with sub-row 32.2 `in-progress → done`** (the §9 in-row ROLLUP per ADR-0106(d)); next-prompt.txt rewritten for the next-phase cold-start (the §9 family stays OPEN — {thrift} remains). *(Task 12.)*
