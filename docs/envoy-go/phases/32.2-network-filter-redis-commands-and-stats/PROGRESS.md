# Phase 32.2 IMPL — PROGRESS

Subagent-driven execution of the 12-task PLAN (`PLAN.md`). Controller tracks; subagents commit LOCAL-ONLY (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Worktree: `.worktrees/phase-32.2-network-filter-redis-commands-and-stats-impl` on branch `phase-32.2-network-filter-redis-commands-and-stats` (off master `9eba8f6`).

---

## Task 1 — baselines/anchors gate (DONE)

IMPL-session tip: `9eba8f6` (`next-prompt.txt: SHA-fill the phase-32.2 PLAN squash (dd340ff) as the live master tip`).

| Anchor | Recipe | Result | Expected |
|---|---|---|---|
| differential fixtures | `ls -d test/fixtures/[0-9]* \| wc -l` | **58** | 58 ✓ |
| fixtures tail | `ls -d test/fixtures/[0-9]* \| tail -1` | `0056-redis-boot-reject` | ✓ |
| fuzzers | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` | **40** | 40 ✓ |
| BackendKind tail | `grep TCPRedisResponder fixture.go` | `= 32` (line 551) | 32 ✓ |
| DECISIONS heading tail | `grep -nE "^#{2,4} ADR-0[0-9]+"` tail | **ADR-0230** (ADR-0231 only a "next-free" forward reference, NOT a defined heading) | tail ADR-0230, next-free ADR-0231 ✓ |
| stat surface | STATE.md / BEHAVIOR_CONTRACT.md report | **546** | 546 ✓ |
| `go mod tidy` diff | `git diff --exit-code go.mod go.sum` | **zero diff** (GOMOD_CLEAN) | clean (ZERO new dep) ✓ |
| `go build ./...` | — | BUILD_OK | clean ✓ |
| `go vet ./...` | — | VET_OK | clean ✓ |

32.2 IMPL targets: fuzzers 40 → **41** (`FuzzRESPDecode`); stat surface 546 → **1091** (+2 splitter + 3 REDIS_CLUSTER_STATS + 180×3=540 per-command); fixtures **58** (`0055` extended — NO new dir); `TCPRedisResponder` reply-table extended (NOT re-numbered); ADR-0229 body completes IN-PLACE (NO new ADR — tail STAYS ADR-0230).

Baseline gate GREEN.

---

## Tasks 2–11 — execution record (subagent-driven; each commit LOCAL-ONLY)

| Task | Commit | Summary | Tests |
|---|---|---|---|
| 2 | `62d747a` | static 180-command `supportedCommandList` + `supportedCommands` map + `commandSupported` | `TestCommandRoster_MatchesUpstream` (180, strict-sorted), `_AllValidNames`, `TestCommandSupported_LookupIsLowerCase` PASS |
| 3 | `5e55a30` | `commandVerdict`+`classify` dispatch + ECHO/TIME/QUIT/HELLO local-reply set + `decodeRequest` 4-value (args) signature (10 callers updated) | `TestDecodeRequest_ExposesArgs`, `TestClassify_*` PASS |
| 4 | `c911c6d` | EAGER 540 per-command counters + 2 splitter + 3 REDIS_CLUSTER_STATS; 32.1 roster test 6→11 | `TestStatRoster32_2_*` PASS |
| 5 | `a695a29` | `incCxActive`/`decCxActive`/`incRqActive`/`decRqActive` + `incProtocolError` | `TestStatRoster32_2_GaugeAndProtocolError` PASS |
| 6 | `09bbc81` | `Handle` pump rewire + `serveRequest` (classify switch, per-command total/success/error, splitter, cx_active/rq_active gauges, cx_protocol_error, QUIT close) + 3 hardened tests | 7 new `TestHandle_*` + 32.1 regression PASS; `-race` clean |
| 7 | `e2746c5` | `redis.` SINGLE-label hoist arm in `internal/stats/name.go` | `TestFlattenToProm_RedisArm` + full stats suite PASS |
| 8 | `11e44ca` | `FuzzRESPDecode` (41st fuzzer) | seed corpus + 20s burst (756K execs) NO crashers |
| 9 | `a4df306` | `TCPRedisResponder` reply-table: `$-1` GET-miss (key "nope"), `:1` INCR/DEL | `0055`/`0056` PASS (Docker live) |
| 10 | `43c1656` | `0055` 10 command-matrix arms + `/stats/prometheus` per-command/splitter cross-side assertions | `0055` differential PASS (live) |
| 11 | `a2c6779` | `0055` `cx_active` held-open gauge arm (==1 both sides) + `rq_active`/buffered quiesced-zero + R6 breaks A–E | `0055` differential PASS; 5 R6 breaks proven live (`-count=1`) |

## As-built deviations from the PLAN (all faithful; reference is the source of truth)

1. **Task 3 — `commandsWithoutMandatoryArgs` includes `hello`.** HELLO with no args is a valid ClusterScopeCommand (proxied), so it must NOT be a bad-arity reject; required for `TestClassify_Proxied(HELLO no-arg → actProxy)`. The PLAN's "empty default" prose was wrong for HELLO specifically.
2. **Task 6 — `TestHandle_ProtocolErrorIncrements` uses `"+bad\r\n"` not the PLAN's `"?bad\r\n"`.** `?` is a valid inline-command first byte (decodeRequest only rejects the reply-type bytes `+ - : $ @`), so `?bad` decodes fine and would NOT drive the protocol-error path; `+bad` is rejected as `errProtocol`. Plus 3 test-hardening fixes (QUIT-close read-EOF assertion, EOF clean-close `cx_protocol_error==0` assertion, pump-level `rq_active==0` assertion) — each proven live by deliberate-break.
3. **Task 10 — D-S32.2-2 UNKNOWN-command wording: the reference APPENDS args, NOT an empty suffix.** The SPEC anticipated `…beginning with: \r\n` (empty). The contrib reference Envoy v1.37.2 emits, for `BOGUSCMD x`, byte-exact: `-ERR unknown command 'BOGUSCMD', with args beginning with: x\r\n` (the request args appended after `beginning with: `). `unknownCommandError` corrected to append `strings.Join(args[1:], ", ")`. Confirmed byte-equivalent cross-side (`reference_wire_format_both_sides_see_same_bytes`). NO `name.go` reconciliation was needed — the reference's `redis.` prometheus shape matched the SPEC §5 design verbatim.
4. **Task 10 — `cluster.redis_cluster.upstream_cx_total` is a PER-SIDE PIN (ref=1 / subj=4), not cross-side equal.** The new per-connection arms expose the documented one-conn-per-downstream seam divergence (ADR-0230 AMEND-R8 / D-P32-9): the reference POOLS the upstream (1), the subject lazy-dials one upstream per proxied-command downstream conn (SET/GET + GET-miss + INCR + DEL = 4). The pooling-independent `upstream_rq_total`==5 stays cross-side equal. Per-side pin is exact + live (R6 Break E).
5. **Task 11 — the 2 `*_bytes_buffered` gauges are a SUBJECT-SIDE coverage boundary.** The reference legitimately tracks buffered bytes (`downstream_cx_rx_bytes_buffered == 14` == the held PING request frame parked unconsumed); the subject never wires them (`filter.go` inc/decs only `cx_active`+`rq_active`). Asserted SUBJECT==0 only (non-vacuous: proves rendered + not spuriously incremented); the reference is not asserted (the framework coverage boundary, `reference_close_direction_framework_gap`-style).

---

## Task 12 — six-gate evidence (run at the IMPL-session tip, pre-completion-bundle)

Run from the worktree root on branch `phase-32.2-network-filter-redis-commands-and-stats`:

| Gate | Command | Result |
|---|---|---|
| 1 build | `go build ./...` | **BUILD_OK** (clean) |
| 2 vet | `go vet ./...` | **VET_OK** (clean) |
| 3 lint | `golangci-lint run` | **clean** (no output) |
| 4 race | `go test ./... -race -short -count=1` | **exit 0** (no failures) |
| 5 differential | `go test ./test/differential/ -count=1` | **ok 192.6s** — all 58 fixtures byte-exact (incl. `0055` extended + `0056` boot-reject + the full back-compat gate) |
| 6 conformance | h2spec 53/53 + proxy-wasm 10/10 | **asserted UNAFFECTED** — phase 32.2 touches no HTTP/h2/proxy-wasm path (image-independent; the `redis.`/redisproxy changes are additive, active only for a redis_proxy terminal / `redis.` stat names) |

**Gate-5 flake note (recorded honestly):** the full differential suite is Docker-heavy (58 fixtures each booting a contrib reference container sequentially, ~3 min). On two earlier invocations during this session a NON-DETERMINISTIC set of UNRELATED HTTP fixtures (`0013-http-local-ratelimit`; then `0018-http-rbac` + `0036-http-wasm-body-and-advanced`) failed with `subj start: subject ready: EOF` — a transient subject-startup race under cumulative Docker churn. Each failing fixture PASSED in isolation, the failing set differed every run, NO redis fixture and NO byte-mismatch ever occurred. A clean re-run with settled Docker state passed all 58. The phase-32.2-touched fixtures (`0055`/`0056`) pass reliably and repeatedly. Conclusion: environmental flake, not a regression (the change is additive + scoped to the redisproxy package + the additive `redis.` name.go branch).

## Counts at phase-32.2 IMPL phase-done

- stat surface **546 → 1091** (+545: 2 `splitter.*` + 3 REDIS_CLUSTER_STATS + 180×3=540 per-command; latency histograms + `*_fault` counters NOT counted per ADR-0060/faults).
- fuzzers **40 → 41** (`FuzzRESPDecode`).
- differential fixtures **58** (`0055` EXTENDED in place — NO new dir; tail `0056-redis-boot-reject`).
- BackendKind tail **32** (`TCPRedisResponder` reply-table extended, NOT re-numbered).
- DECISIONS.md tail STAYS **ADR-0230** (ADR-0229 body completes IN-PLACE per ADR-0044 — NO new ADR; next-free ADR-0231).
- ZERO new go.mod dep (redis_proxy is CORE `/envoy v1.32.4`).

