# Phase 33 — `thrift_proxy` network-filter IMPL — PROGRESS

Running log for the 16-task TDD plan that lands `envoy.filters.network.thrift_proxy`.
Worktree: `.worktrees/phase-33-network-filter-thrift-proxy-impl` (branch
`phase-33-network-filter-thrift-proxy-impl`, based on master `bce2977`).

---

## Task 1 — baselines/anchors gate (NO production code)

Date (UTC): 2026-06-10T08:16:47Z
Branch HEAD at task start: `bce2977c328663fcb306fa9646e2297a6f6b498b` (master tip `bce2977`).

NO production code in this task. It (a) re-confirms the master-tip counts the
rest of the plan depends on, (b) re-derives the TypeURL / zero-new-dep pin,
(c) re-pins the as-built file/line anchors, (d) establishes a clean six-gate
baseline.

### Step 1 — master-tip counts (canonical recipes)

| # | Recipe | Output | Expected | Match |
|---|--------|--------|----------|-------|
| C1 | `ls -d test/fixtures/[0-9]* \| wc -l` | `58` | 58 | ✅ |
| C2 | `ls -d test/fixtures/[0-9]* \| tail -1` | `test/fixtures/0056-redis-boot-reject` | `0056-redis-boot-reject` | ✅ |
| C3 | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` | `41` | 41 | ✅ |
| C4 | `grep -n "BackendKind = 3" test/differential/fixture/fixture.go \| tail -1` | `551:	TCPRedisResponder BackendKind = 32` | `TCPRedisResponder BackendKind = 32` | ✅ |
| C5 (info) | `grep -c "request_time_ms" docs/envoy-go/SPEC.md` | (empty / 0 matches) | informational | n/a — `request_time_ms` HISTOGRAM is DEFERRED per ADR-0060, so absence from SPEC is consistent |
| C6 | `grep -n "^## ADR-023" docs/envoy-go/DECISIONS.md \| tail -1` | `14864:## ADR-0231: the thrift_proxy network filter (phase 33) …` | DECISIONS tail `ADR-0231` | ✅ |

All 5 substantive counts MATCH expected. The informational `request_time_ms`
grep returns no matches (the histogram is deferred per ADR-0060 — not a
mismatch; the PLAN flagged this recipe as informational only).

### Step 2 — TypeURL + ZERO-new-dep re-pin (re-derived, NOT trusted from SPEC string)

```
go doc …/network/thrift_proxy/v3 ThriftProxy | head -3
  → package thrift_proxyv3 // import ".../extensions/filters/network/thrift_proxy/v3"
  → type ThriftProxy struct {

go list -m github.com/envoyproxy/go-control-plane/envoy
  → github.com/envoyproxy/go-control-plane/envoy v1.32.4
```

`thrift_proxy/v3` resolves as a subpackage of the ALREADY-direct
`/envoy v1.32.4` module → **ZERO new go.mod dep** (the redis D32-1 / CORE-extension
posture, NOT the kafka `/contrib` posture).

TypeURL **re-derived programmatically** via `proto.MessageName(&tpv3.ThriftProxy{})`
(throwaway `go run`, NOT hand-typed):

```
type.googleapis.com/envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy
```

Matches the SPEC pin exactly. (Task 2 pins this by a `proto.MessageName` test in
the repo — this is the verification-only pre-derivation.)

### Step 3 — as-built anchors (still present at IMPL-session tip)

| Anchor | Recipe | Result |
|--------|--------|--------|
| A1 — UpstreamConn seam API | `sed -n '37,73p' internal/filter/network/upstreampool.go` | Present: `type UpstreamConn struct` (37), `NewUpstreamConn` (47), `Send` (54), `Reader` (72). `Close` lives just below line 73 — API spine confirmed. |
| A2 — registration site | `grep -n "redisproxy.TypeURL" …/builtins/builtins.go` | `90:	reg.Register(redisproxy.TypeURL, redisproxy.NewFactory(deps.ClusterManager, deps.StatsRegistry))` |
| A3 — blank-import block | `grep -n "redis_proxy/v3" internal/bootstrap/bootstrap.go` | `119:	_ ".../extensions/filters/network/redis_proxy/v3"` (~line 119 as expected) |
| A4 — redis stats arm | `grep -n 'CutPrefix(internal, "redis.")' internal/stats/name.go` | `315:		if rest, ok := strings.CutPrefix(internal, "redis."); ok {` (~line 315 as expected) |
| A5 — main.go uses builtins | `grep -n "RegisterBuiltins" cmd/envoy-go/main.go` | `222:	builtins.RegisterBuiltins(netReg, builtins.Deps{` |

All anchors confirmed at the expected locations.

### Step 4 — six-gate baseline

| Gate | Command | Result |
|------|---------|--------|
| 1 build | `go build ./...` | exit 0 — CLEAN |
| 2 vet | `go vet ./...` | exit 0 — CLEAN |
| 3 lint | `golangci-lint run ./...` (v1.64.8 / go1.26.2) | exit 0 — CLEAN |
| 4–6 test | `go test -race -short ./...` | exit 0 — PASS (82 `ok` packages, 0 FAIL, no `FAIL` substring anywhere in output) |

**Six-gate baseline: GREEN.** Clean baseline established before any production work.

### Task 1 outcome

Status: **DONE** — all 5 substantive counts match, TypeURL re-derived =
SPEC pin, zero-new-dep confirmed, all 5 anchors present, six-gate GREEN.
No mismatches, no concerns.

## Task 2 — package skeleton + `TypeURL` (proto.MessageName pin) + `/envoy` blank-import

Created `internal/filter/network/thriftproxy/{doc.go,thriftproxy.go,thriftproxy_test.go}`
and added the `thrift_proxy/v3` blank-import to `internal/bootstrap/bootstrap.go`
(immediately after the existing `redis_proxy/v3` blank-import, matching the
surrounding doc-commented `_ "..."` block style). Modeled on the
`internal/filter/network/redisproxy/` precedent.

### Step 1–2 — failing test (TDD red)

`thriftproxy_test.go` pins the SPEC §5.1 string via `TestTypeURL`. Before impl:

```
# .../internal/filter/network/thriftproxy [.../thriftproxy.test]
internal/filter/network/thriftproxy/thriftproxy_test.go:7:5: undefined: TypeURL
internal/filter/network/thriftproxy/thriftproxy_test.go:8:37: undefined: TypeURL
FAIL	.../internal/filter/network/thriftproxy [build failed]
```

Fails for the right reason (`TypeURL` undefined / package empty).

### Step 3 — minimal implementation

`thriftproxy.go` derives the type URL via `proto.MessageName(&thrift_proxyv3.ThriftProxy{})`
(NEVER hand-typed — `reference_network_filter_typeurl_extensions`). `doc.go` carries
the package doc (11th network-filter built-in / ADR-0231). `bootstrap.go` gains the
`_ ".../envoy/extensions/filters/network/thrift_proxy/v3"` blank-import with the
AMEND-T1 / ADR-0016 doc comment (the v3 subpackage registers both `thrift_proxy.pb.go`
and `route.pb.go` descriptors).

### Step 4 — test PASS + zero-new-dep

| Check | Command | Result |
|-------|---------|--------|
| test | `go test ./internal/filter/network/thriftproxy/ -run TestTypeURL` | `ok` — PASS |
| zero-dep | `go mod tidy && git diff --exit-code go.mod go.sum` | exit 0 — NO change (ZERO new go.mod dep, AMEND-T1 confirmed) |

### Step 5 — gofmt + lint

| Gate | Command | Result |
|------|---------|--------|
| gofmt | `gofmt -l internal/filter/network/thriftproxy/ internal/bootstrap/` | empty — CLEAN |
| lint | `golangci-lint run ./internal/filter/network/thriftproxy/... ./internal/bootstrap/...` | exit 0 — CLEAN |

### Task 2 outcome

Status: **DONE** — TDD red→green, `TypeURL` == SPEC pin, zero-new-dep confirmed,
gofmt + golangci-lint clean. Commit SHA recorded below.

## Task 3 — config parse (`stat_prefix` required + IsValidName guard + transport/protocol departure rejects + payload_passthrough)

Files: `config.go` (new), `config_test.go` (new), `route.go` (new — TEMPORARY Task-3
stub, Task 4 replaces wholesale).

### Step 0 — proto enum + `IsValidName` verification (re-derived, NOT trusted)

All 8 enum value names used by the test were confirmed present in the generated
`thrift_proxyv3` package via `go doc` / source grep:

| Enum | Confirmed values (name = number) |
|------|----------------------------------|
| `TransportType` | `AUTO_TRANSPORT`=0, `FRAMED`=1, `UNFRAMED`=2, `HEADER`=3 |
| `ProtocolType` | `AUTO_PROTOCOL`=0, `BINARY`=1, `COMPACT`=3, `TWITTER`=4 |

`stats.IsValidName(name string) bool` confirmed at `internal/stats/registry.go:55`.
`ThriftProxy.GetRouteConfig()` returns `*RouteConfiguration` (same `thrift_proxyv3`
package → stub param type matches exactly). **No proto-name deviations.**

### Step 1–2 — failing test (TDD red)

`config_test.go` table test over `parseConfig(*ThriftProxy) (*compiledConfig, error)`
(9 sub-tests). `go test ./internal/filter/network/thriftproxy/ -run TestParseConfig`
→ FAIL (build error): `undefined: parseConfig / errStatPrefixRequired /
errStatPrefixInvalid / errUnsupportedTransport / errUnsupportedProtocol`. Right reason.

### Step 3 — minimal implementation

`config.go` — byte-stable reject arms (ADR-0080; `thrift_proxy: ` prefix mirrors
`redis_proxy: `): only `stat_prefix` is a hard PGV gate (`""` → required;
`!IsValidName` → invalid); `transport`/`protocol` accept `{AUTO,FRAMED}×{AUTO,BINARY}`,
any other defined enum value → envoy-go-strict DEPARTURE reject (reference parse-accepts
those — AMEND-T7). `route_config` NOT required. `compiledConfig{statPrefix,
payloadPassthrough, routes}`. `route.go` — TEMPORARY stub (`routeTable struct{}` +
`parseRouteConfig(*RouteConfiguration) (*routeTable, error)`), replaced wholesale by Task 4.

### Step 4 — test PASS (9 sub-tests)

`go test ./internal/filter/network/thriftproxy/ -run TestParseConfig -v` → PASS:
`minimal-defaults-ok`, `framed-binary-ok`, `passthrough-ok`, `stat-prefix-missing`,
`stat-prefix-invalid`, `transport-unsupported`, `transport-header-unsupported`,
`protocol-unsupported`, `protocol-twitter-unsupported`.

### Step 5 — gofmt + lint

| Gate | Command | Result |
|------|---------|--------|
| gofmt | `gofmt -l internal/filter/network/thriftproxy/` | empty — CLEAN |
| lint | `golangci-lint run ./internal/filter/network/thriftproxy/...` | exit 0 — CLEAN |

Lint initially flagged `ProtocolType_TWITTER` as deprecated (SA1019/staticcheck). The
deprecated value is intentionally exercised to prove the departure-reject covers it, so a
targeted `//nolint:staticcheck` (with rationale) was added to that one test row rather
than dropping the assertion. Re-run → exit 0.

### Task 3 outcome

Status: **DONE** — TDD red→green (9 sub-tests), all 8 proto enum names + `IsValidName`
re-verified (no deviations), byte-stable reject arms per ADR-0080, gofmt +
golangci-lint clean. Commit SHA recorded below.

(Task 3 commit: `984f6a0`.)

---

## Task 4 — route_config method-routing table + match

Files: `route.go` (REPLACES the Task-3 stub wholesale), `route_test.go` (new).

### Step 0 — oneof type-name verification (re-derived, NOT trusted)

`go doc .../thrift_proxy/v3 RouteMatch` + `RouteAction` + `RouteConfiguration` —
all PLAN-quoted generated names confirmed exact (no deviations):

| Type | Confirmed generated names |
|------|---------------------------|
| `RouteMatch.MatchSpecifier` oneof | `*RouteMatch_MethodName` (field `MethodName string`), `*RouteMatch_ServiceName` (field `ServiceName string`) |
| `RouteAction.ClusterSpecifier` oneof | `*RouteAction_Cluster` (field `Cluster string`); deferred siblings `*RouteAction_WeightedClusters`, `*RouteAction_ClusterHeader` → fall into `errRouteClusterRequired` via the type-assertion `!ok` |
| `RouteConfiguration` | `Routes []*Route` |

Import alias `routev3` in `route_test.go` resolves to the SAME package path as
`thrift_proxyv3` in `route.go` (route.pb.go ships in the ThriftProxy package) — Go
permits two aliases for one path across files; both compile clean.

### Step 1–2 — failing test (TDD red)

`go test ./internal/filter/network/thriftproxy/ -run TestRoute` → build failed:
`rt.match undefined` (×4) on the Task-3 stub `type routeTable struct{}`, plus undefined
`errRouteMatchRequired`/`errRouteActionRequired`/`errRouteClusterRequired`/`errRouteMatchUnsupported`. Right reason.

### Step 3 — minimal implementation

`route.go` (stub → real): `entries []routeEntry` ordered table. `parseRouteConfig` —
nil/absent route_config → all-miss (no error, AMEND-T7); per route, byte-stable ADR-0080
reject arms (match required, only method_name matches supported, action required,
cluster required — also catches non-`Cluster` oneofs + empty `Cluster`). `match(method)` —
exact `method_name` first-match, then first match-all (empty `method_name`) fallback, else
`("",false)`.

### Step 4 — test PASS (+ full-package suite, no regression)

`go test ./internal/filter/network/thriftproxy/ -v` → TestRouteMatch, TestRouteMiss,
TestRouteParseRejects{no-match,no-action,empty-cluster,service-name-unsupported}, plus the
carried Task 2-3 tests (TestTypeURL, TestParseConfig +9) — all PASS, `ok 0.004s`.

### Step 5 — gofmt + lint

| Gate | Command | Result |
|------|---------|--------|
| gofmt | `gofmt -l internal/filter/network/thriftproxy/` | empty — CLEAN |
| lint | `golangci-lint run ./internal/filter/network/thriftproxy/...` | exit 0 — CLEAN |

### Task 4 outcome

Status: **DONE** — TDD red→green, oneof type-names re-verified (no deviations), byte-stable
reject arms per ADR-0080, gofmt + golangci-lint clean. Commit `4db2f6e` (route code) +
this controller fix-up commit (PROGRESS log relocated from the stale repo-root
`PROGRESS.md` to this canonical phase-33 file).

---

## Task 5 — framed-transport frame decode + binary message-begin (the codec core)

**Files:** `internal/filter/network/thriftproxy/thrift.go` (new),
`internal/filter/network/thriftproxy/thrift_test.go` (new).

### Step 1-2 — failing test (red, right reason)

Wrote `thrift_test.go` (TestDecodeFrame_Call + TestDecodeFrame_Errors with 7 sub-cases:
empty, truncated-length, truncated-payload, bad-magic, bad-msgtype, zero-length,
oversized-length). `go test -run TestDecodeFrame` → **build failed**: `undefined:
msgTypeCall`, `undefined: decodeFrame` (×N). Red for the right reason (symbols not yet
declared), not an assertion miscompile.

### Step 3 — minimal implementation

`thrift.go` per SPEC Appendix A: FRAMED = 4-byte BE signed int32 length prefix (`>0` and
`<= maxFrameSize` 100 MiB) + payload; BINARY strict message-begin (magic `0x8001` + zero +
msgtype `Call/Reply/Exception/Oneway` + i32 name-len + name + i32 seq_id,
`minMessageBeginLength=12`). `decodeFrame(r *bufio.Reader) (*thriftMessage, error)` returns
the decoded begin + the RAW full frame (length prefix + payload, forwarded VERBATIM) + the
opaque `body` (passthrough). Two error sentinels per D-S33-6: `errDecode` (all malformed
frames → `request_decoding_error`) and the DISTINCT `errInvalidType` (out-of-range msgtype →
`request_invalid_type`, Task 8 `errors.Is`). Frame-boundary `io.EOF` (no bytes) returned
verbatim = clean end between frames.

### Step 4 — test PASS (+ full-package suite, no regression)

`go test ./internal/filter/network/thriftproxy/ -run TestDecodeFrame -v` → TestDecodeFrame_Call
PASS + all 7 TestDecodeFrame_Errors sub-cases PASS. Full package suite
`go test ./internal/filter/network/thriftproxy/...` → `ok 0.003s` (carried Task 1-4 tests all
still green — no regression).

### Panic-safety reasoning (independent self-check; Task 14 fuzzer hardening)

No input can cause a slice-out-of-range or an unbounded allocation:

1. **Oversized/zero/negative length → no allocation.** The `frameLen <= 0 || int64(frameLen)
   > maxFrameSize` guard runs BEFORE `make([]byte, frameLen)`. The `oversized-length`
   case `0x7fffffff` (= 2147483647) is `> maxFrameSize` (104857600) so it returns `errDecode`
   without ever allocating ~2 GB. Negative int32 (high bit set) and zero are caught by `<= 0`.
   So `frameLen` reaching `make` is always in `[1, maxFrameSize]` — a bounded allocation.

2. **`minMessageBeginLength=12` gate.** After `io.ReadFull(r, payload)` succeeds, `len(payload)
   == frameLen`. The `len(payload) < 12` check guarantees indices `payload[0:2]`, `payload[3]`,
   `payload[4:8]` are all in range (max index 7 < 12) BEFORE any of them are read.

3. **`nameLen` bound proves the name + seqid slices.** `nameLen := int32(...payload[4:8])` then
   `nameLen < 0 || 8+int64(nameLen)+4 > int64(len(payload))` rejects via `errDecode` BEFORE
   slicing. The arithmetic is done in `int64` so `8+nameLen+4` cannot overflow (nameLen is an
   int32, widened). After the check: `8+nameLen+4 <= len(payload)`, therefore
   `payload[8:8+nameLen]` is in range (`8+nameLen <= len(payload)`) AND
   `payload[off:off+4]` with `off=8+nameLen` is in range (`off+4 <= len(payload)`). The
   `nameLen < 0` arm also rejects a name-len with the int32 high bit set so the `string(...)`
   slice low bound `8` never exceeds the high bound.

4. **`body = payload[off:]`** with `off = 8+nameLen+4 <= len(payload)` is always a valid
   (possibly empty) slice.

Conclusion: every read of `payload` is dominated by a prior length check, and the single
`make` is bounded by `maxFrameSize`. The decoder is panic-safe on arbitrary bytes.

### Step 5 — gofmt + lint

| Gate | Command | Result |
|------|---------|--------|
| gofmt | `gofmt -l internal/filter/network/thriftproxy/` | empty — CLEAN |
| lint | `golangci-lint run ./internal/filter/network/thriftproxy/...` | exit 0 — CLEAN |

### Task 5 outcome

Status: **DONE** — TDD red→green, codec core (`decodeFrame`/`thriftMessage`/msgType
constants/`errDecode`+`errInvalidType` sentinels) for Tasks 6/8/14 to build on. Wire format
SPEC-Appendix-A-verbatim (no invented framing). Panic-safety independently proven above.
gofmt + golangci-lint clean. Commit local-only.

---

## Task 6 — reply classifier + UnknownMethod AppException encoder

Status: **DONE** — TDD red→green. Appended `classifyReply` (D-S33-4 first-field-header
peek) + `encodeUnknownMethod` (Appendix A live-captured AppException) + the `appendI32`
helper to `thrift.go`; appended `framedBinaryReply`/`mustDecode` helpers +
`TestClassifyReply`/`TestEncodeUnknownMethodException` to `thrift_test.go`.

### Step 1 — failing test (red)

Compile-red on undefined `classifyReply`, `replySuccess`, `replyError`,
`encodeUnknownMethod` (build failed), exactly as TDD expects before the impl lands.

### Step 2 — frame-len recomputation (the critical deliverable)

The encoder is the layout authority; the frame-len prefix MUST equal the true assembled
payload byte-length. Byte-by-byte derivation of the EXCEPTION payload for method
`"somethingelse"`, seq 1, message `no route for method 'somethingelse'`:

| Section | Bytes |
|---------|-------|
| magic(2) + zero(1) + msgtype EXCEPTION(1) | 4 |
| name-len i32 | 4 |
| name `somethingelse` | 13 |
| seq_id i32 | 4 |
| field-1 header: type STRING 0x0b + id 0x0001 | 3 |
| string msg-len i32 | 4 |
| msg `no route for method 'somethingelse'` | 35 (0x23) |
| field-2 header: type I32 0x08 + id 0x0002 | 3 |
| i32 value (1 = UnknownMethod) | 4 |
| STOP 0x00 | 1 |
| **total payload** | **75 (0x4b)** |

Message-string recount: `no route for method '` = 21 + `somethingelse` = 13 + `'` = 1 =
**35 (0x23)**. Verified against the encoder's actual emitted bytes (`len(p) == 75`,
prefix `0x0000004b`).

**Frame-len byte used: `0x4b` (75).** The PLAN's `0x4b`=75 literal was CORRECT. The PLAN
prose's alternate hand-sum of "71/0x47" was an arithmetic slip (it dropped/miscounted in the
4+4+13+4+3+4+35+3+4+1 sum, which equals **75**, not 71). I initially set the test literal to
the prose's 0x47, the encoder produced the true 0x4b, and I corrected the test literal back
to `0x4b` to match the encoder (the layout authority). No change to the encoder was needed.

### Step 3/4 — impl + green

`go test -run 'TestClassifyReply|TestEncodeUnknownMethod' -v` → both PASS. Full package
`go test ./internal/filter/network/thriftproxy/` → ok (no regression).

`classifyReply` is panic-safe: length-guarded (`len(m.body)==0` / `<3`) before reading the
1 type byte + 2 id bytes; STOP(0x00) or field-id 0 → success, field-id ≥1 → error. Reads
only the leading field header of the opaque passthrough body.

### Step 5 — gofmt + lint

| Gate | Command | Result |
|------|---------|--------|
| gofmt | `gofmt -l internal/filter/network/thriftproxy/` | empty — CLEAN |
| lint | `golangci-lint run ./internal/filter/network/thriftproxy/...` | exit 0 — CLEAN |

### Task 6 outcome

`encodeUnknownMethod` output is load-bearing: later asserted byte-for-byte against the LIVE
reference Envoy miss-path reply in the 0057 differential fixture (Task 12). Wire format is
SPEC-Appendix-A-verbatim. Commit local-only.

---

## Task 7 — EAGER 25-name stat roster (24 counters + request_active gauge)

**Files:** `internal/filter/network/thriftproxy/stats.go` (new), `internal/filter/network/thriftproxy/stats_test.go` (new).

### `internal/stats` Registry API confirmation

Read `internal/stats/registry.go` + `counter.go` + `gauge.go`, and mirrored
`internal/filter/network/redisproxy/stats.go` + its `stats_test.go` for the established pattern.

| Concern | Confirmed real API |
|---------|--------------------|
| Constructor | `stats.NewRegistry() *Registry` |
| Eager-create counter | `reg.NewCounterIfAbsent(name) *stats.Counter` (idempotent, post-Freeze-permitted) |
| Eager-create gauge | `reg.NewGaugeIfAbsent(name) *stats.Gauge` |
| Counter type/methods | `*stats.Counter`: `.Inc()`, `.Add(uint64)`, `.Load() uint64`, `.Name()` |
| Gauge type/methods | `*stats.Gauge`: `.Inc()`, `.Dec()`, `.Load() int64`, `.Name()` |
| Lookup accessors | **NONE** — no `LookupCounter`/`LookupGauge`/`LookupHistogram` exists |
| Histogram type | **NONE** — ADR-0060 reserves it; the registry has no Histogram type at all |

### Test adjustment vs PLAN's assumed API

The PLAN's draft test assumed `LookupCounter`/`LookupGauge`/`LookupHistogram` registry accessors —
**these do not exist**. Mirrored the redisproxy precedent instead: presence asserted via the
`thriftStats.counters` map + `.active` field (and `.Name()` checks the registered `thrift.tp.` prefix),
present-at-0 via `.Load()==0`, idempotency via SAME-pointer comparison across a second prefix-sharing
`newThriftStats(reg,"tp")`. The "histogram NOT created" check became a roster-membership assertion
(`request_time_ms` absent from `counters`), which is the correct shape given no Histogram type exists.
Added `TestStatAccessors` to prove `inc`/`incActive`/`decActive` are live (also keeps lint happy).

### TDD steps

| Step | Command | Result |
|------|---------|--------|
| 2 — fail | `go test -run 'TestStatRoster\|TestStatAccessors'` | FAIL — `undefined: newThriftStats`, `undefined: counterSuffixes` |
| 4 — pass | `go test -run 'TestStatRoster\|TestStatAccessors' -v` | both PASS — 24 counters + request_active gauge present-at-0, no histogram, idempotent |
| full pkg | `go test ./internal/filter/network/thriftproxy/` | ok — no regression |

### Roster

`counterSuffixes` = exactly **24** counters (test asserts `len==24`) + the `request_active` gauge = **25** names,
all under `thrift.<stat_prefix>.`, created EAGER via `NewCounterIfAbsent`/`NewGaugeIfAbsent`. Pinned
name-for-name against `ALL_THRIFT_FILTER_STATS` + the 5 router counters (SPEC §7.2 / §11.3). The
`request_time_ms` histogram is DEFERRED (ADR-0060). The 2 close-direction counters
(`cx_destroy_{local,remote}_with_active_rq`) + `downstream_response_drain_close` are created but NEVER
incremented (AMEND-T6); `downstream_cx_max_requests` exist-at-0 (max_requests_per_connection deferred).

### Step 5 — gofmt + lint

| Gate | Command | Result |
|------|---------|--------|
| gofmt | `gofmt -l internal/filter/network/thriftproxy/` | empty — CLEAN |
| lint | `golangci-lint run ./internal/filter/network/thriftproxy/...` | exit 0 — CLEAN (no unused-method flag) |

### Task 7 outcome

The roster is consumed by Task 8 (`inc`/`incActive`/`decActive` in the pump), flattened by Task 10's prom
arm, and counted by Task 16's BEHAVIOR_CONTRACT (surface 1091→1116 = +25). EAGER creation makes all 25
present-at-0 at NewFactory so the differential `/stats` scrape sees them even when never incremented.
Commit local-only.

---

## Task 8 — `TerminalFilter.Handle` request→reply pump (filter.go)

The heart of the filter: the framed-binary message→reply pump wired to the framework's
`network.TerminalFilter` seam, `cluster.Manager`, and the REUSED ADR-0230 `UpstreamConn` seam.
TDD: failing test (compile-red) → implement → green; 8 arms; `request_active` balanced to 0.

### Real interface / API names adapted to (vs the PLAN sketch)

| PLAN sketch name | Real name used |
|------------------|----------------|
| `network.TerminalFilter.Handle(ctx, downstream net.Conn)` | EXACT match (`internal/filter/network/terminal.go:42-49`); embed `network.Marker` for the sealed `isNetworkFilter()` |
| `f.st.inc(suf)` / `incActive` / `decActive` | EXACT (`stats.go`); counters via `ts.counters[suf]`, gauge `ts.active` |
| `network.UpstreamConn` / `NewUpstreamConn(dial, hook)` / `Send(ctx, raw)` / `Reader()` / `Close()` | EXACT (`upstreampool.go:37-85`); `dial` is `network.UpstreamDialFunc func(ctx) (net.Conn, error)`, `hook` is `func()` (cluster.IncUpstreamRqTotal) |
| `f.cm.Get(cluster)` | `cluster.Manager.Get(name) (*Cluster, bool)` (`manager.go:111`) |
| `f.dialSource = f.resolveCluster` (redis names it `dialSource`/`resolveCatchAll`) | renamed `resolve resolveFunc` / `f.resolveCluster` — cluster-KEYED (redis's is catch-all-fixed) |
| (redis folds upstream-miss into one transport error) | thrift SPLITS into a `resolveStatus` tri-state: `resolveOK` / `resolveUnknownCluster` (→`unknown_cluster`) / `resolveNoHealthyUpstream` (→`no_healthy_upstream`), per the router roster (§3.3) |

`resolveCluster` (production): `cm.Get`-miss → `resolveUnknownCluster`; `PickEndpoint()` error → `resolveNoHealthyUpstream`;
else a dial closure over `Cluster.Dial(ctx)` (Endpoint discarded, §8) + `IncUpstreamRqTotal`. `NewFactory(cm, reg)` mirrors
redisproxy's two-dep shape: type_url reject → `UnmarshalTo` → `parseConfig` → `newThriftStats` → wire `f.resolve = f.resolveCluster`.

`ensureUpstream` is cluster-KEYED (reuse if the cluster name is unchanged, re-dial defensively if a later frame names a
different cluster). The MVP fixture pins ONE route/cluster, so a single `UpstreamConn` per downstream conn suffices.

### Oneway handling decision

The MVP single-flight pump round-trips a route HIT regardless of CALL vs ONEWAY: `request_oneway` is counted for
`msgTypeOneway` (vs `request_call` for everything else), but the HIT path still `Send`s + reads ONE reply (SPEC §3.3
step 3 "Round-trip a HIT" is msgtype-agnostic; the fixture pins CALL "Ping"). True fire-and-forget oneway semantics
(no reply expected; the framed single-flight model would block on `Reader()`) are NOT special-cased in the MVP — a
concern only if a future fixture drives a oneway HIT, which the §3.3 MVP does not. NOTED as a deferred concern.

A REPLY whose decoded msgtype is semantically not a reply/exception (CALL/ONEWAY in reply position) → `response_invalid_type`
(the `default` branch). `decodeFrame` itself rejects out-of-range msgtypes with `errInvalidType` before this switch.

### The 8 test arms (all PASS; request_active==0 in hit AND miss)

| Arm | Asserts |
|-----|---------|
| `TestNewFactory_TypeURLReject` | wrong type_url → error |
| `TestNewFactory_ValidConfig` | valid config → non-nil FilterInstanceFactory → non-nil NetworkFilter |
| `TestHandle_RouteHit` | CALL "ping" round-trips; REPLY forwarded byte-equivalent; request/request_call/request_passthrough/response/response_reply/response_success/response_passthrough each +1; request_oneway==0; **request_active==0**; dialed |
| `TestHandle_RouteMiss` | CALL "other" → local UnknownMethod exception bytes downstream; route_missing/response_exception +1; request/request_call +1; NO dial; cx_destroy_*/downstream_response_drain_close stay 0 (AMEND-T6); **request_active==0** |
| `TestHandle_DecodingError` | bad-magic frame → request_decoding_error +1; request_invalid_type==0; request_active==0 |
| `TestHandle_InvalidType` | msgtype 0x09 → request_invalid_type +1; request_decoding_error==0 |
| `TestHandle_UnknownCluster` | resolveUnknownCluster → unknown_cluster +1; graceful close (no hang); no_healthy_upstream==0 |
| `TestHandle_NoHealthyUpstream` | resolveNoHealthyUpstream → no_healthy_upstream +1; graceful close |
| `TestHandle_CrossClusterReuse` | Task 8 followup (code-review minor #2): two routes "Ping"→c1 / "Pong"→c2 over ONE downstream conn; both replies byte-equivalent to their distinct backend; c1 dialed 1, c2 dialed 1 (cross-cluster re-dial fired); OLD c1 upstream observed closed on re-dial (EOF on c1 backend); request==2/response_success==2/route_missing==0; request_active==0. Makes the defensive `upCluster != clusterName` re-dial branch in `ensureUpstream` live-tested. Break-verified: removing the close-on-re-dial fails the c1-closed assertion. |

### Gates

| Gate | Command | Result |
|------|---------|--------|
| package (race) | `go test -count=1 -race ./internal/filter/network/thriftproxy/...` | ok |
| gofmt | `gofmt -l internal/filter/network/thriftproxy/` | empty — CLEAN |
| vet | `go vet ./internal/filter/network/thriftproxy/...` | clean |
| lint | `golangci-lint run ./internal/filter/network/thriftproxy/...` | exit 0 — CLEAN |
| build | `go build ./...` | OK (no regression) |

Counter-assertion correctness is load-bearing for Task 12's differential fixture (same counters cross-side).
Commit local-only.

## Task 9 — register thrift_proxy as the 11th network-filter built-in

TDD. Wires the thrift_proxy filter into the framework's built-in registry so a bootstrap carrying a `thrift_proxy`
`typed_config` resolves to `thriftproxy.NewFactory`. The parity step that makes the filter reachable at runtime (and at
the Task 12 differential fixture). `cmd/envoy-go/main.go` registers network filters SOLELY via
`builtins.RegisterBuiltins`, so NO main.go change is needed.

### Real APIs (vs the PLAN's illustrative snippet)

- `Deps` fields: `ClusterManager *cluster.Manager`, `StatsRegistry *stats.Registry` (+ AccessLogSinks / HTTPRegistry /
  DrainManager / HTTPClient). The PLAN's `deps.ClusterManager` / `deps.StatsRegistry` field names are EXACT.
- Registry API: `network.NewRegistry()` → `reg.Register(typeURL, factory)` (void; panics on frozen/dup) →
  `reg.Freeze()` → `reg.Lookup(typeURL) (factory, ok)`. Matches the PLAN's `reg.Lookup` assumption.
- `thriftproxy.NewFactory(cm *cluster.Manager, reg *stats.Registry) network.NetworkFilterFactory` — the SAME two-dep
  shape as `redisproxy.NewFactory` (UNLIKE the stats-only kafka/mongo/zookeeper factories). The PLAN's two-dep
  registration line is correct verbatim.

### Test shape

The existing `builtins_test.go` carries BOTH an enumerate-all table (`TestRegisterBuiltinsRegistersAllTen`) AND
per-filter focused tests + boot-smokes (mongo/zookeeper). I matched all three patterns:

- Renamed/extended the table test → `TestRegisterBuiltinsRegistersAllEleven`, adding `thriftproxy.TypeURL` to the
  enumerated set (this is the "count assertion" — the loop now covers 11 TypeURLs). The all-ten test was the count
  assertion that had to bump.
- Added `TestRegisterBuiltins_RegistersThriftProxy` (focused: passes BOTH ClusterManager:nil + a real StatsRegistry,
  mirroring `TestRegisterBuiltins_RegistersRedisProxy`).
- Added `TestThriftProxyBootSmoke` (mirrors mongo/zookeeper boot-smokes, but asserts the instance is a
  `network.TerminalFilter` — thrift_proxy OWNS the downstream conn via `Handle`, UNLIKE the both-directions
  ReadFilter+WriteFilter mongo/zookeeper filters — and spot-checks the eager `thrift.<sp>` roster present-at-0:
  request/response/route_missing/request_decoding_error counters + the request_active gauge).

### Implementation (builtins.go)

Added the import + the 11th `reg.Register(thriftproxy.TypeURL, thriftproxy.NewFactory(deps.ClusterManager,
deps.StatsRegistry))` after the redis_proxy line. Bumped the package doc + the `RegisterBuiltins` doc comment from
"ten"→"eleven" built-ins.

### Red → green

| Step | Command | Result |
|------|---------|--------|
| RED | `go test ./internal/filter/network/builtins/ -run 'TestRegisterBuiltins_RegistersThriftProxy\|TestThriftProxyBootSmoke\|TestRegisterBuiltinsRegistersAllEleven'` | FAIL — "did not register …ThriftProxy" / "thrift_proxy factory not found" |
| GREEN | same | PASS (all 3) |

### Gates

| Gate | Command | Result |
|------|---------|--------|
| focused | `go test ./internal/filter/network/builtins/ -run Thrift\|Eleven -v` | PASS |
| package | `go test ./internal/filter/network/builtins/` | ok |
| build | `go build ./...` | OK (new import compiles repo-wide) |
| boot smoke | `go test ./internal/bootstrap/... -short` | ok (bootstrap parses a thrift_proxy listener) |
| gofmt | `gofmt -l internal/filter/network/builtins/` | empty — CLEAN |
| lint | `golangci-lint run ./internal/filter/network/builtins/...` | exit 0 — CLEAN |

Commit local-only. No main.go change.

## Task 10 — `thrift.` SINGLE-label-hoist Prometheus arm (`internal/stats/name.go`)

The redis-32 ADR-0229 single-label tag-extractor shape generalized to a `thrift.` ROOT prefix. Internal
name `thrift.<stat_prefix>.<rest>` → Prometheus base `envoy_thrift_<rest flattened>` + hoisted label
`envoy_thrift_prefix="<stat_prefix>"`. Per SPEC §7.4 / ADR-0231 (AMEND-T3).

### Real APIs (vs the PLAN's illustrative names)

The PLAN's `promName(in)` / `hasLabel(...)` / `Label{Key,Value}` were illustrative. The REAL names:

- Entry point: **`flattenToProm(internal string) (string, []Label, error)`** (NOT `promName`).
- `Label` struct fields ARE `Key` / `Value` — the PLAN's `Label{Key:..., Value:...}` matched verbatim.
- There is NO `hasLabel` helper. The redis-arm test (`TestFlattenToProm_RedisArm`) asserts labels
  inline: `len(labels) != 1 || labels[0].Key != "..." || labels[0].Value != "..."`. The new
  `TestFlattenToProm_ThriftArm` mirrors that exact shape (NOT the PLAN's `hasLabel`).

### Implementation

Inserted the `thrift.` arm IMMEDIATELY after the `redis.` arm (`strings.CutPrefix(internal, "thrift.")`),
BEFORE the final fall-through error return. Body is the redis arm verbatim with `redis`→`thrift`
substituted — `strings.CutPrefix` + `strings.IndexByte('.')>0` + dot-free `<prefix>` guard + label append +
`base = "envoy_thrift_" + strings.ReplaceAll(tail, ".", "_")` + early return.

### Roster cleanliness

All 24 `counterSuffixes` + the `request_active` gauge in `internal/filter/network/thriftproxy/stats.go`
are dot-free leaf names → they flatten to `envoy_thrift_<leaf>` with NO internal-dot substitution and NO
charset risk (the roster is FIXED; the Thrift method drives ROUTING, not a counter name).

### Red → green

| Step | Command | Result |
|------|---------|--------|
| RED | `go test ./internal/stats/ -run TestFlattenToProm_ThriftArm -count=1` | FAIL — all 4 cases "has no recognized top-level segment" (fall-through) |
| GREEN | same `-v` | PASS — all 4 cases (`request`, `response_reply`, `route_missing`, `request_active` gauge) |

### Gates

| Gate | Command | Result |
|------|---------|--------|
| focused | `go test ./internal/stats/ -run TestFlattenToProm_ThriftArm -v -count=1` | PASS (4 cases) |
| full suite | `go test ./internal/stats/ -count=1` | ok — no regression to redis/mongo/kafka/wasm/SN1-SN9 arms |
| gofmt | `gofmt -l internal/stats/` | empty — CLEAN |
| lint | `golangci-lint run ./internal/stats/...` | exit 0 — CLEAN |

Commit local-only.

## Task 11 — `TCPThriftResponder` BackendKind (value 33) in the differential fixture harness

### Where the responder lives (the REAL pattern, vs the PLAN's "modify fixture.go")

The PLAN's "Files: Modify fixture.go (the BackendKind enum + the responder loop)" is an over-simplification.
In the ACTUAL codebase the responder machinery is split:

- `test/differential/fixture/fixture.go` holds ONLY the `BackendKind` enum constant (the shared discriminator).
- `test/differential/runner_test.go` (package `differential`) holds the `accept*Responder` + `*RespondLoop`
  functions, the per-kind dispatch `case` in the runner's backend-setup `switch`, AND the responder unit
  tests (`TestKafkaResponderBackend`, `TestMongoResponderBackend`, `TestZKResponderBackend`). There is NO
  responder loop in `fixture.go`, and the `fixture_test.go` file is interface smoke-tests only.

So Task 11 mirrors the REAL precedent: enum in `fixture.go`; loop + accept + dispatch case + unit test in
`runner_test.go`. (PLAN line 1290 anticipated this: "If fixture.go responders are exercised only through the
runner, add the focused test in the fixture package; else the 0057 driver is the live proof — but a unit
test here de-risks Task 12." The responders ARE runner-exercised, so the test lands beside the loop in
`runner_test.go`, matching the Kafka/Mongo/ZK responder-test precedent.)

### How TCPRedisResponder is defined + dispatched (mirrored verbatim)

- Enum: `TCPRedisResponder BackendKind = 32` in `fixture.go`. I appended `TCPThriftResponder BackendKind = 33`.
- Dispatch: a `case fixture.TCPRedisResponder:` in the runner's backend `switch` that `net.Listen`s a
  loopback TCP socket, records the port, and `go acceptRedisResponder(ln, bo.accepts)`. I added the parallel
  `case fixture.TCPThriftResponder:` → `go acceptThriftResponder(ln, bo.accepts)`.
- Loop: `acceptRedisResponder` accept-loops + counts + `go redisRespondLoop(c)` per conn; `redisRespondLoop`
  duplicates a tiny RESP codec (no filter-package import). I mirrored this with `acceptThriftResponder` +
  `thriftRespondLoop`, duplicating the framed-binary wire format (4-byte BE frame-len + binary message-begin)
  WITHOUT importing `internal/filter/network/thriftproxy` (the self-contained precedent).

### void-success vs exception mode

Keyed by an IN-BAND request marker — the `kafkaMarkerUncorrelated`/`kafkaMarkerNoReply` request-keyed
precedent (the responder stays keyed by `BackendKind`; per-request behavior is selected from the wire, so NO
new harness parameterization mechanism was needed). The marker is the request METHOD name:
`thriftMarkerException = "boom"`. A CALL with any other method → void-success REPLY (msgtype 2, body = single
STOP `0x00`); a CALL with method `"boom"` → EXCEPTION (msgtype 3) carrying an AppException TStruct body
`{1: string "backend exception", 2: i32 6}` (the `encodeUnknownMethod` field layout: field-1 STRING id 1,
field-2 I32 id 2, STOP). Both echo the SAME method + RECEIVED seq_id (seq_id-AGNOSTIC — AMEND-T5).

Exception mode IS wired and IS unit-tested (the test's second arm). The Task 12 0057 reply-EXCEPTION arm dials
method `"boom"` to exercise `response_exception` from a BACKEND reply (distinct from the local route-miss
exception). The marker method choice is the responder's contract; Task 12 must dial it.

### Red → green

| Step | Command | Result |
|------|---------|--------|
| RED | `go test ./test/differential/ -run TestThriftResponder -count=1` | FAIL — `undefined: acceptThriftResponder` (compile) |
| GREEN | `go test ./test/differential/ -run TestThriftResponderBackend -count=1 -v` | PASS — REPLY decoded msgtype 2, method "ping", seq 7, body `00`; marker "boom" → EXCEPTION msgtype 3, seq 9 |

### Gates

| Gate | Command | Result |
|------|---------|--------|
| focused | `go test ./test/differential/ -run TestThriftResponderBackend -v -count=1` | PASS |
| siblings | `go test ./test/differential/ -run 'TestThriftResponderBackend\|TestKafkaResponderBackend\|TestMongoResponderBackend\|TestZKResponderBackend' -count=1` | ok — no regression |
| fixture pkg | `go test ./test/differential/fixture/... -count=1` | ok |
| build | `go build ./...` | clean |
| vet | `go vet ./test/differential/...` | clean |
| gofmt | `gofmt -l test/differential/fixture/ test/differential/runner_test.go` | empty — CLEAN |
| lint | `golangci-lint run ./test/differential/fixture/... ./test/differential/...` | exit 0 — CLEAN |

### Files changed

- `test/differential/fixture/fixture.go` — `+TCPThriftResponder BackendKind = 33`.
- `test/differential/runner_test.go` — dispatch `case`, `acceptThriftResponder`, `thriftRespondLoop`,
  `thriftFrame`/`thriftMsgBegin`/`thriftReplyFrame`/`thriftExceptionFrame` builders, `thriftMarkerException`
  const, `thriftReqFrame` test helper, `TestThriftResponderBackend`.

Commit local-only.

---

## Task 12 — `0057-thrift-roundtrip` cross-side fixture

Status: DONE. The largest task — Docker (28.1.1) + the reference image
`envoyproxy/envoy:contrib-v1.37.2` available; the cross-side differential was run
for real (3 arms green, byte-equivalence + `StatsAsserter`).

### Files changed

- `test/fixtures/0057-thrift-roundtrip/driver/driver.go` (NEW) — the fixture
  driver: both bootstraps (thrift_proxy TERMINAL; STRICT_DNS ref / STATIC subj;
  `thrift_cluster` → `TCPThriftResponder`), `BackendKind() → fixture.TCPThriftResponder`,
  the self-contained `thriftCallFrame` builder (NOT imported from the filter pkg —
  the 0055 precedent), the 3-arm `driveProxy`, and the `StatsAsserter` `AssertStats`.
- `test/fixtures/0057-thrift-roundtrip/README.md` (NEW) — arm taxonomy, the
  two-pronged proof, the per-side boundaries, the deliberate-break record.
- `test/differential/runner_test.go` — `+_ ".../0057-thrift-roundtrip/driver"` blank-import.

### Route-table design (method-keyed, NO match-all)

`Ping`→`thrift_cluster` (HIT void), `boom`→`thrift_cluster` (HIT exception),
`Pong` misses (→ local `UnknownMethod`). `boom` MUST be a HIT route so the
reply-EXCEPTION arm reaches the backend (the Task-11 `thriftMarkerException`); a
match-all route would make nothing miss.

### Arms (each a fresh conn, one CALL, one framed reply frame)

1. HIT `Ping` seq 1 → REPLY void-success forwarded downstream.
2. MISS `Pong` seq 2 → local `UnknownMethod` EXCEPTION, NO dial.
3. reply-EXCEPTION `boom` seq 3 → backend EXCEPTION (D-S33-2).

### Cross-side run — observed stat table (FLAT /stats, both sides)

| Counter | ref | subj | Disposition |
|---|---|---|---|
| `thrift.thrift_r.request` / `_call` / `_passthrough` | 2 | 3 | **per-side** — ref does NOT count `request*` on a MISS (SPEC §7.3); subj counts-before-match (PLAN Task 8 pump) |
| `thrift.thrift_r.response` | 2 | 2 | cross-equal (Ping REPLY + boom EXCEPTION) |
| `thrift.thrift_r.response_reply` | 1 | 1 | cross-equal |
| `thrift.thrift_r.response_success` | 1 | 1 | cross-equal |
| `thrift.thrift_r.response_passthrough` | 2 | 2 | cross-equal |
| `thrift.thrift_r.response_exception` | 2 | 2 | cross-equal (Pong local + boom backend — D-S33-2) |
| `thrift.thrift_r.route_missing` | 1 | 1 | cross-equal |
| `thrift.thrift_r.request_active` | 0 | 0 | cross-equal (quiesced, D-S33-3) |
| `thrift.thrift_r.cx_destroy_local_with_active_rq` | 1 | 0 | **per-side** boundary (subj==0; AMEND-T6/D-T8) |
| `thrift.thrift_r.downstream_response_drain_close` | 1 | 0 | **per-side** boundary (subj==0) |
| `cluster.thrift_cluster.upstream_cx_total` | 1 | 2 | **per-side** pooling (D-T9b / redis D-P32-9) |
| `cluster.thrift_cluster.upstream_rq_total` | 2 | 2 | cross-equal (pooling-independent) |
| `cluster.thrift_cluster.upstream_cx_rx_bytes_total` | 73 | absent | ref-only witness (subj cluster doesn't emit it — SPEC §7.5) |

Byte-equivalence (CompareBytes) PASSED for all 3 arms with NO encoder
reconciliation (the SPEC §11.2/Appendix A wire layout matched the reference
verbatim).

### Decode-ran witness (SPEC §8 caveat (i))

`upstream_cx_rx_bytes_total = 73 > 0` (reference; subj cluster doesn't emit it)
AND `request_call > 0` (both sides). thrift_proxy emits no listener
`downstream_cx_rx_bytes_total` (HCM stat) — `request*`/cluster bytes are the witness.

### Deliberate-break liveness (`-count=1`)

| Break | Perturbation | FAIL output | Restore → PASS |
|---|---|---|---|
| PRODUCTION — `response_success` | commented out `f.st.inc("response_success")` in `filter.go` (HIT void-success inc) | `cross-side mismatch thrift.thrift_r.response_success: ref=1 subj=0` + `subj … = 0, want 1` | ✓ |

A PRODUCTION break (not a driver echo) — proves the `StatsAsserter` is wired to
the real subject counter. After restore: `git diff internal/…/filter.go` EMPTY,
`0057` re-runs PASS.

### Real subject bug? NO.

All divergences are SPEC-documented per-side boundaries (miss-not-counted,
one-conn-per-downstream pooling, the close-direction framework gap). Byte
behavior is identical cross-side.

### Gates

| Gate | Command | Result |
|------|---------|--------|
| full fixture | `go test ./test/differential/ -run 'TestDifferential/0057' -count=1 -v` | PASS (3 arms, byte-equivalence + StatsAsserter) |
| build | `go build ./test/...` | clean |
| vet | `go vet ./test/fixtures/0057-thrift-roundtrip/...` | clean |
| gofmt | `gofmt -l test/fixtures/0057-thrift-roundtrip/driver/` | empty — CLEAN |
| lint | `golangci-lint run ./test/fixtures/0057-thrift-roundtrip/...` | exit 0 — CLEAN |

Commit local-only.

---

## Task 13 — `0058-thrift-boot-reject` differential fixture (missing `stat_prefix`)

Authored `test/fixtures/0058-thrift-boot-reject/driver/driver.go` +
`README.md` and blank-imported the driver in `test/differential/runner_test.go`.
SYMMETRIC `differential.BootRejectFixture`: a `thrift_proxy` listener OMITTING
`stat_prefix` → BOTH the contrib reference Envoy AND envoy-go reject at boot
(SPEC §8.2 / §6.2). Modeled verbatim on `0056-redis-boot-reject` (terminal
network filter, no tcp_proxy; the `c_unused` zero-cluster-boot sidestep).

### Per-side stderr (captured empirically against `envoyproxy/envoy:contrib-v1.37.2`)

| Side | Genuine reject stderr | Contains `stat_prefix`? | Contains `thrift_proxy`? |
|---|---|---|---|
| subject (envoy-go) | `listener manager: listener: "l_thrift": filter_chains[0]: filters[0]: thrift_proxy: stat_prefix is required` | YES (snake_case) | YES (error line) |
| reference (C++) | `Proto constraint validation failed (ThriftProxyValidationError.StatPrefix: value length must be at least 1 characters)` + echoed bootstrap | NO (CamelCase `StatPrefix`; field omitted → never echoed) | YES (×2 in the `--config-yaml` echo: filter `name:` + @type) |

The reference DOES boot-reject a no-`stat_prefix` thrift_proxy config (verified
`refErr != nil` + the captured stderr above). The two impls word the violation
differently (CamelCase vs snake_case), so lowercase `stat_prefix` is NOT a shared
substring. The committed substring is `thrift_proxy` — the strongest token
genuinely present in BOTH from a non-circular source (the 0056 `redis_proxy`
precedent): the subject error line + the reference's echoed real filter
name/@type. NOT cross-impl string equality (per-side per §6.2).

### Deliberate-break liveness (`-count=1`)

| Break | Perturbation | FAIL output | Restore → PASS |
|---|---|---|---|
| substring | `expectedBootErrorSubstr` → `"zzz_not_present"` | `BootRejectFixture: reference stderr does NOT contain "zzz_not_present"` | ✓ (restored to `thrift_proxy`, re-run PASS) |

Proves the SECONDARY substring assertion is live. The PRIMARY claim (both sides
fail to boot) is the runner's separate `refErr != nil && subjErr != nil` gate.

### Other reject arms (unit-tested, NOT fixtures)

The route / route-action / thrift-filter-name PGV arms + the un-chosen
transport/protocol DEPARTURE arms (envoy-go-strict rejects enum values the
reference parse-ACCEPTS — AMEND-T7) are covered UNIT-TEST-ONLY by
`config_test.go` (`TestParseConfig`) + `route_test.go` (`TestRouteParseRejects`),
both green.

### Gates

| Gate | Command | Result |
|------|---------|--------|
| fixture | `go test ./test/differential/ -run 'TestDifferential/0058' -count=1 -v` | PASS (both sides boot-reject) |
| unit reject arms | `go test ./internal/filter/network/thriftproxy/ -run 'TestParseConfig\|TestRouteParseRejects' -v` | PASS |
| fixture count | `ls -d test/fixtures/[0-9]* \| wc -l` | 60 (58 + 0057 + 0058) |
| vet | `go vet ./test/differential/` | clean |
| gofmt | `gofmt -l test/fixtures/0058-thrift-boot-reject/driver/` | empty — CLEAN |
| lint | `golangci-lint run ./test/fixtures/0058-thrift-boot-reject/...` | exit 0 — CLEAN |

Commit local-only.

## Task 14 — the 42nd fuzzer `FuzzThriftDecode` (no-panic / no-mutation / bounded-allocation)

SPEC §14 + D-S33-5. Created `internal/filter/network/thriftproxy/fuzz_test.go`
mirroring `redisproxy/fuzz_test.go` (the established 41st fuzzer house style:
seed corpus → `f.Fuzz` closure → snapshot-and-compare no-mutation invariant; the
bounded-allocation invariant is enforced implicitly by the codec's pre-`make()`
length guard, no explicit assertion — same as redis).

The fuzzer drives `decodeFrame` over a `bufio.NewReader(bytes.NewReader(data))`
and calls `classifyReply` on a non-nil success result. Three invariants:
(1) no panic (implicit); (2) no mutation of the input slice (snapshot `orig` vs
`data` post-decode); (3) bounded allocation — the `frameLen <= 0 || > maxFrameSize`
guard (thrift.go:52) rejects a crafted 4-byte length prefix BEFORE `make([]byte,
frameLen)`. Per `reference_dynamic_stat_name_charset_guard` the codec touches NO
stat registry (the roster is fixed/EAGER in stats.go) — no charset-panic risk, so
the fuzzer scope is the codec only.

### Seed corpus (6)

| Seed | Shape |
|---|---|
| `framedBinaryCall(msgTypeCall, "ping", 1)` | valid CALL (helper from Task 5) |
| `framedBinaryReply("ping", 1, {0x00})` | valid void REPLY (helper from Task 6) |
| `{00 00 00 11 80 01 00}` | truncated payload (length says 0x11, 3 bytes follow) |
| `{00 00 00 0c 00…00 01}` | bad magic (version uint16 != 0x8001) |
| `{7f ff ff ff}` | oversized length prefix (> maxFrameSize → guard rejects pre-make) |
| `framedBinaryCall(0x09, "x", 1)` | invalid msgtype (> msgTypeOneway → errInvalidType) |

### Gates

| Gate | Command | Result |
|------|---------|--------|
| seed corpus | `go test ./internal/filter/network/thriftproxy/ -run FuzzThriftDecode -v` | PASS (6/6 seeds) |
| campaign | `go test ./internal/filter/network/thriftproxy/ -fuzz FuzzThriftDecode -fuzztime 20s` | PASS — 347,128 execs, no panic, no new crashers |
| no crashers | `ls internal/filter/network/thriftproxy/testdata/fuzz/` | no such dir — zero crashers written |
| fuzzer count | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` | **42** |
| gofmt | `gofmt -l internal/filter/network/thriftproxy/` | empty — CLEAN (ran `gofmt -w` to align seed comments) |
| lint | `golangci-lint run ./internal/filter/network/thriftproxy/...` | exit 0 — CLEAN |

Commit local-only.

---

## Task 15 — full differential re-verify (60 dirs) + deliberate-break liveness proofs + six gates

Date (UTC): 2026-06-10T09:49:21Z
Branch HEAD at task start: `2adee7973aded3b70ecf72bdb66eef159909a521` (Task 14 tip).

VERIFICATION task — NO production code changes (only temporary deliberate-breaks,
each restored via `git checkout -- <file>` with `git diff internal/` re-confirmed
empty). The comprehensive gate before the Task 16 completion bundle.
Docker (28.1.1) + reference image `envoyproxy/envoy:contrib-v1.37.2`
(`@sha256:7edd5b0fd763…`) confirmed present.

### Step 1 — full differential (all dirs byte-exact, `-count=1`)

`go test ./test/differential/ -count=1` → `ok … 203.958s`, exit 0.

Reconciliation of the "60 dirs":

| Recipe | Output |
|--------|--------|
| `ls -d test/fixtures/00* \| wc -l` | **60** fixture dirs (0000…0058, incl. the two letter-suffixed `0007a-cors`/`0007b-iteration-probe`) |
| differential driver-registered subtests (`grep -oE 'TestDifferential/[0-9]{4}-…' \| sort -u`) | **58** — every numbered fixture EXCEPT `0007a-cors`/`0007b-iteration-probe` (non-differential: unit/probe-only, never registered a Driver) |
| `--- FAIL` markers in verbose run | **0** (the only `FAIL` substrings are proxy-wasm fixture log lines `FAIL_CLOSED 503`, not test results) |
| new thrift dirs present | `TestDifferential/0057-thrift-roundtrip` ✅ + `TestDifferential/0058-thrift-boot-reject` ✅ |

All 58 driver-registered dirs PASS (the 56 prior byte-exact back-compat + the 2
new thrift dirs). NO regression in any prior dir. The two non-differential
fixtures (0007a/0007b) are out of the differential runner's scope by design and
are exercised by their own unit suites.

### Step 2 — deliberate-break liveness proofs (`-count=1`, the 0030-vacuity guard)

Per `reference_differential_break_protocol_count1` EVERY run used `-count=1` (the
test cache serves a stale PASS otherwise). After EACH restore, `git diff --stat
internal/` was empty before proceeding.

Baseline: `go test ./test/differential/ -run 'TestDifferential/0057' -count=1` → `ok` (3.452s).

| # | Break | Result FAIL evidence | Restore → re-PASS | `git diff internal/` |
|---|-------|----------------------|-------------------|----------------------|
| (a) | `response_success` mis-count — replaced `f.st.inc("response_success")` (filter.go:209) with a no-op | `cross-side mismatch thrift.thrift_r.response_success: ref=1 subj=0` → FAIL | `git checkout -- filter.go` → 0057 `ok` (3.634s) | empty ✅ |
| (b) | MISS-arm downstream byte-equivalence — flipped `"no route…"`→`"No route…"` in `encodeUnknownMethod` (thrift.go:113) | `differential mismatch: first divergence at offset 48` — ref byte `6e`('n') vs subj `4e`('N') in the EXCEPTION TStruct string → FAIL | `git checkout -- thrift.go` → 0057 `ok` (3.686s) | empty ✅ |
| (c) | cross-side counter parity — removed `f.st.inc("route_missing")` (filter.go:178) | `cross-side mismatch thrift.thrift_r.route_missing: ref=1 subj=0` → FAIL | `git checkout -- filter.go` → 0057 `ok` (3.487s) | empty ✅ |

All three load-bearing 0057 assertions proven LIVE (none vacuous): the
StatsAsserter cross-side counter parity (a,c) and the MISS-arm downstream
byte-equivalence diff (b) each fail crisply on a one-line/one-byte break and
recover on restore. (Recorded here for the 0057 README per the PLAN.)

### Step 3 — the six gates

| Gate | Command | Result |
|------|---------|--------|
| build | `go build ./...` | exit 0 |
| vet | `go vet ./...` | exit 0 |
| gofmt | `gofmt -l internal/ test/` | empty — CLEAN |
| lint | `golangci-lint run ./...` (v1.64.8) | exit 0 — CLEAN |
| race-short | `go test -race -short -count=1 ./...` | exit 0 — 83 pkgs `ok`, 0 `FAIL`/`DATA RACE`; `thriftproxy` `ok` (see flake note) |
| differential | `go test ./test/differential/ -count=1` | `ok … 203.958s` exit 0 (Step 1) |

**Race-short flake note:** a FIRST full `-race ./...` run surfaced one isolated
failure in `internal/filter/http/wasm` (`dispatch_test.go:753 created = 56; want
100` — a 100-goroutine concurrency test starved under `-race` load). The wasm
package is **untouched by phase-33** (`git diff master --name-only | grep wasm`
→ none) and re-ran 3/3 PASS in isolation; the full `-race -short -count=1 ./...`
re-run was exit-0 with zero FAILs. Classified pre-existing flake, NOT a phase-33
regression.

**Conformance gates (h2spec 53/53 + proxy-wasm 10/10) asserted UNAFFECTED**
(SPEC §8.4), NOT re-run live: phase 33 adds only the `thrift_proxy` network
filter (a new leaf package + a blank-import in bootstrap). It touches NO HTTP /
HTTP-2 / proxy-wasm code path — `git diff master --name-only` lists only
`internal/filter/network/thriftproxy/*`, the bootstrap blank-import, the two new
`test/fixtures/0057*`/`0058*` dirs, the TCP responder, and docs. The h2spec /
proxy-wasm surfaces are therefore image-independent and structurally unreachable
by this change.

### Step 4 — live stat-surface delta spot-check (1091 → 1116, +25)

| Evidence | Result |
|----------|--------|
| static roster (`stats.go` `counterSuffixes` slice) | **24** counter names + `request_active` **gauge** = **25** |
| `TestStatRoster` unit | PASS — pins `len(counterSuffixes)==24` & `len(st.counters)==24`, all 24 present-at-0 under `thrift.<sp>.`, `request_active` gauge present-at-0; shared-prefix gauge-instance share |
| live roster (running subject) | the 0057 differential PASS scrapes the subject's `/stats` and cross-side-asserts the `thrift.thrift_r.*` roster (StatsAsserter) — proves the 25 names are LIVE on a booted `thrift_proxy` listener, not merely unit-constructed |

The +25 delta (24 counters + 1 gauge) is thus both statically pinned and live-proven;
the BEHAVIOR_CONTRACT count advances 1091 → 1116 at Task 16.

### Outcome

Status: **DONE**. Full 60-driver differential byte-exact (incl. 0057/0058), all
three 0057 liveness breaks FAIL→restore→PASS with `internal/` clean after each,
six gates green, conformance asserted-unaffected, roster spot-checked live. No
production code changed. Commit local-only.

## Task 16 — the completion bundle (the atomic landing per ADR-0052)

A DOCS-ONLY task (no production code changed). The contract/ADR/STATE/ROADMAP
edits land together as the final phase-33 commit (ADR-0052 atomic landing).

### Files modified

- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — (1) the NEW `### envoy.filters.network.thrift_proxy`
  subsection (the single-pair single-route terminal envelope; the `route_config` method-routing;
  the REUSED ADR-0230 round-trip; the local `UnknownMethod` exception; the EAGER 25-name roster
  under `thrift.<stat_prefix>.`; the 11th built-in; the `thrift.` SINGLE-label-hoist prom arm; the
  full coverage-boundary/departure list incl. the two Task-12 per-side findings); (2) the
  `## Stat-name mapping` **1091 → 1116** extension block (+25: 24 counters + 1 gauge `request_active`);
  (3) the §9-family narrative roll-up sentence extended (1 candidate → 0; family CLOSES).
- `docs/envoy-go/DECISIONS.md` — the **ADR-0231 §Decision + §Consequences body IN-PLACE**
  (PROPOSED → ACCEPTED per ADR-0044 — NO new ADR number; DECISIONS tail STAYS **ADR-0231**,
  next-free → ADR-0232). §Decision: the as-built package layout + the REUSED-seam consumption +
  the local-reply semantics + the EAGER roster + the prom arm + the 11th built-in. §Consequences:
  the §9 family CLOSES; the ADR-0230 seam's redis-scoped YAGNI sizing VALIDATED at its FIRST reuse
  (zero seam churn; the cross-cluster re-dial branch made live-tested); the deferred multiplexing
  surface stays deferred; the two Task-12 per-side findings + the close-direction boundary recorded.
- `docs/envoy-go/STATE.md` — active-phase → `phase 33 (network-filter-thrift-proxy) done`;
  lifecycle-state → phase 33 CLOSED + the §9 family CLOSES; next-skill → `superpowers:brainstorming`
  (a NEW family/row, TBD); counts block → **1116 / 60 / 42 / BackendKind 33 / DECISIONS tail ADR-0231**;
  next-free ADR-0232 (ADR-0231 now ACCEPTED).
- `docs/envoy-go/ROADMAP.md` — row 33 `in-progress → done (2026-06-10)` (a flat §9 row — NO parent
  rollup per ADR-0106) + the IMPL-DONE note; the §9 Network-filters family candidate-roster paragraph
  records the family CLOSURE (0 candidates remain after thrift; the family that opened at phase 26 closes).
- `docs/envoy-go/phases/33-network-filter-thrift-proxy/PROGRESS.md` — this final record.

### The two empirical per-side findings (Task-12 `0057`, recorded in the contract + ADR)

1. **The subject counts `request`/`request_call`/`request_passthrough` on a routing MISS**
   (count-before-match in the pump → subj=3 across the 3-arm workload: Ping+Pong+boom), while the
   C++ reference does NOT count `request*` on a miss (ref=2 — HIT-only: Ping+boom; the miss accounted
   ONLY via `route_missing`+`response_exception`). Asserted PER-SIDE in `0057` (ref=2/subj=3, NOT
   cross-equal) — a documented behavioral divergence, NOT a subject bug.
2. **The subject's cluster package does NOT emit `upstream_cx_rx_bytes_total`** (it reuses only
   `upstream_cx_total`/`upstream_rq_total`). The decode-ran byte-witness is reference-side only
   (`cluster.<name>.upstream_cx_rx_bytes_total > 0`, ref=73); the subject witness is `request_call > 0`.
   `upstream_cx_total` is per-side (ref=1 pooled / subj=2 one-conn-per-downstream); `upstream_rq_total == 2`
   cross-equal.

### Step 4 — the six-gate evidence + final counts

| Gate | Command | Result |
|------|---------|--------|
| build | `go build ./...` | exit 0 |
| vet | `go vet ./...` | exit 0 |
| lint | `golangci-lint run ./...` | exit 0 — CLEAN |
| race-short | `go test -race -short ./...` | exit 0 — 0 FAIL / 0 DATA RACE |
| differential | `go test ./test/differential/ -run 'TestDifferential/0057\|TestDifferential/0058' -count=1` | `ok … 4.861s` exit 0 (the docs-only change compiles all tests; Task 15's full 60-dir run is the authoritative byte-exact gate) |
| fixtures | `ls -d test/fixtures/[0-9]* \| wc -l` | **60** |
| fuzzers | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` | **42** |

Final counts: stat surface **1116** (+25), fixtures **60**, fuzzers **42**, BackendKind tail **33**,
DECISIONS tail **ADR-0231** (next-free ADR-0232). The BEHAVIOR_CONTRACT count edit reads **1116**
consistently (the `## Stat-name mapping` 1091→1116 block + the narrative roll-up + the subsection).

### Outcome

Status: **DONE**. The atomic landing commit (ADR-0052) — BEHAVIOR_CONTRACT subsection +
1091→1116 + the two Task-12 findings; ADR-0231 §Decision/§Consequences body in-place (tail STAYS
ADR-0231); STATE/ROADMAP row 33 `in-progress → done` + the §9 Network-filters family CLOSURE; six
gates green; counts 1116 / 60 / 42 / BackendKind 33. No production code changed. Commit local-only.
**Phase 33 CLOSED; the §9 Network-filters family CLOSES (0 candidates remain).**
