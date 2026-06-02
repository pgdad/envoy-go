# Phase 28.2 IMPL Progress

## Task 1: baselines/anchors gate

**Live tip SHA:** `d927a4f` (next-prompt.txt: repoint master-tip reference to c02b950)

---

### Step 1: Project counts at IMPL-session tip

```
$ git log --oneline -1
d927a4f next-prompt.txt: repoint master-tip reference to c02b950 (actual HEAD; trails 28.2-PLAN squash 6c60d36 +1)

$ ls -d test/fixtures/[0-9]* | wc -l
49

$ ls -d test/fixtures/[0-9]* | tail -1
test/fixtures/0047-zookeeper-boot-reject

$ grep "_ \"github.com/esalaine/envoy-go/test/fixtures/" test/differential/runner_test.go | wc -l
49

$ grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l
37

$ grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -3
ADR-0221
ADR-0222
ADR-0223
```

NOTE: The PLAN recipe `grep -c "test/fixtures/00.*/driver"` returns 25 (the regex `00.*` only matches
fixture numbers starting with `00`, missing e.g. `0015`-`0039`). The correct count of 49 is confirmed
by the broader `grep "_ \"github.com/esalaine/envoy-go/test/fixtures/"` recipe above.

| Metric | Expected | Actual | Status |
|--------|----------|--------|--------|
| Active fixture dirs | 49 | 49 | OK |
| Tail fixture dir | `0047-zookeeper-boot-reject` | `0047-zookeeper-boot-reject` | OK |
| Active blank-imports | 49 | 49 | OK |
| Fuzzers | 37 | 37 | OK |
| DECISIONS tail | ADR-0223 | ADR-0223 | OK |
| Next-free ADR | ADR-0224 | ADR-0224 | OK |

---

### Step 2: Stat surface

```
docs/envoy-go/BEHAVIOR_CONTRACT.md:464
"Phase 28.1 extension — 136 → 337 internal names: ..."
```

| Metric | Expected | Actual | Status |
|--------|----------|--------|--------|
| Stat surface | 337 | 337 | OK |

---

### Step 3: Port assignments for fixture 0048

```
$ grep -rn "150[0-9][0-9]" test/fixtures/*/driver/driver.go | grep -oE "150[0-9][0-9]" | sort -u | tail -6
15044
15045
15046
15047
15048
15049
```

Highest assigned reference port = **15049** (fixture `0047`).

Port pin for `0048`:
- `l_resp` = **15050**
- `l_fast` = **15051**
- `l_slow` = **15052**
- `l_rflags` = **15053**

Ports 15050–15053 are **free** (none taken by any existing fixture). No shift required.

---

### Step 4: As-built line anchors

All 31 anchors verified at IMPL tip `d927a4f`. Zero drift found — all anchors hold exactly at the PLAN-pinned lines.

| # | File:line | Construct | Status |
|---|-----------|-----------|--------|
| 1 | `internal/filter/network/zookeeperproxy/decoder.go:27-56` | `requestDecoder` struct (`requestsByXid` :50, `controlRequestsByXid` :55) | OK |
| 2 | `internal/filter/network/zookeeperproxy/decoder.go:58-65` | `newRequestDecoder` | OK |
| 3 | `internal/filter/network/zookeeperproxy/decoder.go:75-90` | `decodeOnData` | OK |
| 4 | `internal/filter/network/zookeeperproxy/decoder.go:96-112` | `nextFrame` | OK |
| 5 | `internal/filter/network/zookeeperproxy/decoder.go:116-141` | `decodeFrame` | OK |
| 6 | `internal/filter/network/zookeeperproxy/decoder.go:147-173` | `onConnect` (`recordControl(connectXid, opname, opConnect)` at :171) | OK |
| 7 | `internal/filter/network/zookeeperproxy/decoder.go:216-219` | `recordControl` | OK |
| 8 | `internal/filter/network/zookeeperproxy/decoder.go:224` | `wireFootprint` | OK |
| 9 | `internal/filter/network/zookeeperproxy/decoder.go:239-245` | `decoderError` | OK |
| 10 | `internal/filter/network/zookeeperproxy/decoder.go:308-334` | `onDataRequest` (`requestsByXid` write at :332) | OK |
| 11 | `internal/filter/network/zookeeperproxy/zookeeperproxy.go:70-76` | no-op `OnWrite`; :40 the `newRequestDecoder` call; :51 the `decoder *requestDecoder` field | OK |
| 12 | `internal/filter/network/zookeeperproxy/zookeeperproxy.go:88` | `OnDestroy` | OK |
| 13 | `internal/filter/network/zookeeperproxy/config.go:118-137` | `compiledConfig` (latency fields :128-130; three flags :133-136) | OK |
| 14 | `internal/filter/network/zookeeperproxy/stats.go:119-152` | `respOpNames` (28 names; NO `connect_readonly`) | OK |
| 15 | `internal/filter/network/zookeeperproxy/stats.go:204-220` | `inc`/`add` (panic-on-unknown-suffix) | OK |
| 16 | `internal/filter/network/zookeeperproxy/decoder_test.go:16-67` | test helpers (`be32`/`be64`/`zkFrame`/`connectFrame`/`dataFrame`/`newTestDecoder`/`counterValue`); `padTo` at :249 | OK |
| 17 | `internal/filter/network/zookeeperproxy/zookeeperproxy_test.go:135-150` | `TestFilterOnWritePureNoOp`; `newTestFilter` at :26 | OK |
| 18 | `internal/filter/network/zookeeperproxy/fuzz_test.go:28-73` | `FuzzZookeeperRequestDecode` (`newRequestDecoder` call at :49) | OK |
| 19 | `internal/filter/network/writeconn.go:34-48` | `writeChainConn.Write` (fresh per-Write Buffer; `endStream=false`) | OK |
| 20 | `internal/filter/tcpproxy/filter.go:134-138` | the two pumps + `wg.Wait()` | OK |
| 21 | `test/differential/fixture/fixture.go:493-502` | `TCPSink BackendKind = 28` + 0048-responder forward-pointer comment; :505-510 `BackendKindAware` | OK |
| 22 | `test/differential/runner_test.go:827-841` | the `TCPSink` backend arm | OK |
| 23 | `test/differential/runner_test.go:1258-1276` | `acceptSinkCounting` | OK |
| 24 | `test/differential/runner_test.go:70-72` | the `0045`/`0046`/`0047` blank-imports | OK |
| 25 | `test/fixtures/0046-zookeeper-requests/driver/driver.go` | 875 LoC; `driveFrames` :394; `AssertStats` :605; `renderBootstrap` :805 | OK |
| 26 | `docs/envoy-go/DECISIONS.md:14324-14343` | ADR-0223 heading + §AMEND + §Context (file ends at :14343) | OK |
| 27 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:464` | the 28.1 stat-mapping block | OK |
| 28 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:3627-3654` | the `### envoy.filters.network.zookeeper_proxy` subsection | OK |
| 29 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:3656-3686` | the conn-wrap-seam block | OK |
| 30 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:3688-3708` | `### Stat surface` / `### Applies to` / `### Does not yet apply to` (28.2 forward bullet at :3704) | OK |
| 31 | `docs/envoy-go/ROADMAP.md:82/:85` | parent row 28 / sub-row 28.2 | OK |

**Anchor drift: NONE.** All 31 anchors hold at their PLAN-pinned lines. No re-pointing needed.

---

### Step 5: Baseline build + test + race

```
$ go build ./... && go vet ./...
(no output — clean)

$ go test ./internal/filter/network/... -race -short -count=1
ok      github.com/esalaine/envoy-go/internal/filter/network            1.011s
ok      github.com/esalaine/envoy-go/internal/filter/network/builtins   1.025s
ok      github.com/esalaine/envoy-go/internal/filter/network/directresponse    1.013s
ok      github.com/esalaine/envoy-go/internal/filter/network/echo       1.011s
ok      github.com/esalaine/envoy-go/internal/filter/network/rbac       1.017s
ok      github.com/esalaine/envoy-go/internal/filter/network/snicluster 1.011s
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy    1.095s

$ go test ./internal/filter/network/zookeeperproxy/ -run Fuzz -count=1
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy    0.003s
```

All green. Build clean, vet clean, race tests pass, seed-corpus fuzz pass.

---

### Summary

Task 1 DONE. All counts match, ports 15050-15053 free, 31/31 anchors hold with zero drift, baseline green.

---

## Task 2: Decoder rename + writeBuf/mu fields + request-path locking

### Step 1: Mechanical rename

Occurrence counts (pre-verified match PLAN expectations):

| File | `requestDecoder` | `newRequestDecoder` | Total |
|------|-----------------|---------------------|-------|
| `decoder.go` | 13 | 1 | 14 |
| `decoder_test.go` | 2 | 5 | 7 |
| `zookeeperproxy.go` | 1 | 1 | 2 |
| `fuzz_test.go` | 0 | 1 | 1 |

All 24 substitutions applied via `sed -i 's/\brequestDecoder\b/decoder/g; s/\bnewRequestDecoder\b/newDecoder/g'`.

### Step 2: Doc comment updates

- `decoder` struct: replaced old 3-line comment with 8-line both-directions comment (ADR-0222/ADR-0223, goroutine A/B, mu §3.6).
- Added `newDecoder` constructor doc comment (was absent).
- `zookeeperproxy.go` field: updated comment to `// per-connection (reassembly bufs + correlation structures + mu)` and fixed gofmt alignment (trailing spaces after `*decoder`).

### Step 3: Post-rename test run

```
$ go test ./internal/filter/network/... -race -short -count=1
ok      github.com/esalaine/envoy-go/internal/filter/network            1.012s
ok      github.com/esalaine/envoy-go/internal/filter/network/builtins   1.022s
ok      github.com/esalaine/envoy-go/internal/filter/network/directresponse    1.012s
ok      github.com/esalaine/envoy-go/internal/filter/network/echo       1.009s
ok      github.com/esalaine/envoy-go/internal/filter/network/rbac       1.019s
ok      github.com/esalaine/envoy-go/internal/filter/network/snicluster 1.009s
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy    1.093s
```

All green. Assertions unchanged (mechanical rename only).

### Step 4: writeBuf + mu fields added

Added after `readBuf`, before correlation maps:
- `writeBuf []byte` — write-side reassembly buffer (AMEND-A8 symmetry; accessed ONLY by goroutine B). Tagged `//nolint:unused` per established codebase pattern (field is intentionally unused until Task 3).
- `mu sync.Mutex` — guards the two correlation maps. Added `"sync"` import.

### Step 5: Request-path map writes locked

- `recordControl`: extracted `pendingRequest` construction before the lock; map append wrapped in `d.mu.Lock()` / `d.mu.Unlock()`. Updated doc comment.
- `onDataRequest` map write: extracted `entry` before the lock; `d.requestsByXid[xid] = entry` wrapped in `d.mu.Lock()` / `d.mu.Unlock()`.

### Step 6: Final test run (post-lock)

```
$ go test ./internal/filter/network/... -race -short -count=1
ok      github.com/esalaine/envoy-go/internal/filter/network            1.011s
ok      github.com/esalaine/envoy-go/internal/filter/network/builtins   1.024s
ok      github.com/esalaine/envoy-go/internal/filter/network/directresponse    1.012s
ok      github.com/esalaine/envoy-go/internal/filter/network/echo       1.010s
ok      github.com/esalaine/envoy-go/internal/filter/network/rbac       1.020s
ok      github.com/esalaine/envoy-go/internal/filter/network/snicluster 1.011s
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy    1.092s

$ go test ./internal/filter/network/zookeeperproxy/ -run Fuzz -count=1
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy    0.003s
```

### Step 7: gofmt + golangci-lint

Both clean (gofmt: no output; golangci-lint: no output after adding `//nolint:unused` on `writeBuf`).

### Self-review

- Rename hit exactly the expected 24 occurrences (14+7+2+1). Zero leftover `requestDecoder`/`newRequestDecoder` in the codebase.
- `decoder_test.go` diff: ONLY mechanical rename (`requestDecoder` → `decoder`, `newRequestDecoder` → `newDecoder`). Zero assertion changes.
- Diff confined to 4 allowed files + PROGRESS.md. `config.go`/`stats.go`/`config_test.go`/`stats_test.go`/`zookeeperproxy_test.go` untouched.
- gofmt + golangci-lint clean.

**Task 2 DONE.**

---

## Task 3: Write-side reassembly + response framing + UNCORRELATED dispatch rows

### Step 1: D-S28.2-1 upstream verification (parseWatchEvent min-length) — CORRECTED AT IMPL

Fetched `https://raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/source/extensions/filters/network/zookeeper_proxy/decoder.cc`.

Upstream `parseWatchEvent` at v1.37.2 (decoder.cc:1036 + :22):
```cpp
absl::Status DecoderImpl::parseWatchEvent(Buffer::Instance& data, uint64_t& offset,
                                          const uint32_t len, const int64_t zxid,
                                          const int32_t error) {
  absl::Status status = ensureMinLength(len, SERVER_HEADER_LENGTH + (3 * INT_LENGTH));
```

Where `SERVER_HEADER_LENGTH = 16` (xid(4)+zxid(8)+error(4), decoder.cc:22) and `INT_LENGTH = 4`.
So `ensureMinLength(len, 16 + 12) = ensureMinLength(len, 28)`. Upstream's minimum is **28**.

**The SPEC's 16-byte pin was WRONG. D-S28.2-1 says: "if upstream differs, use upstream's value."**

The watch-event wire format carries the full ReplyHeader (xid+zxid+error) like every non-connect
response (decoder.cc:321-346 — `decodeOnWrite` peeks xid, then zxid, then error, and only then
dispatches `XidCodes::WatchXid` → `parseWatchEvent(data, offset, len, zxid, error)`). The SPEC's
16-byte pin omitted zxid(8)+error(4) from the frame layout and was corrected at IMPL.

**Corrected wire format:** xid(4) + zxid(8) + error(4) + event_type(4) + client_state(4) +
path-len(4) + path — minimum **28 bytes** (upstream `parseWatchEvent` SERVER_HEADER_LENGTH + 3×INT_LENGTH).

**CONSEQUENCE FOR TASK 8:** The TCPZKResponder's watch-event push frame MUST be written in the
real format: `xid(-1) + zxid(8) + error(4) + event_type(4) + client_state(4) + path-len(4) + path`
(≥28 bytes). A frame without zxid+error would be shorter than 28 bytes and upstream would count
`decoder_error` instead of `watch_event`, causing fixture 0048 arm 2 to diverge cross-side.

**Pre-dispatch universal minimum note (recorded, NOT changed):** Upstream's `decodeOnWrite` has a
universal pre-dispatch minimum of 16 (xid+zxid+error) applied BEFORE correlation dispatch
(decoder.cc:321-346). Our decoder's universal minimum is 4 (the xid sniff) with row-specific
minimums applied after correlation. The difference is observable only for 4–15-byte degenerate
frames (which no real server and no fixture corpus produces): our decoder consumes a matching
correlation entry and fires the flag-gated per-opcode decoder error; upstream errors without
consuming. This is recorded as part of the documented response-side shallow-decode leniency
departure (SPEC §2.2) — NOT changed at IMPL because our complete-frame reassembly model makes
upstream's declared-vs-buffered length distinction structurally impossible.

**Fix applied at IMPL:** `onWatchEvent` minimum corrected from 16 → 28; `watchEventFrame` builder
corrected to include full ReplyHeader (zxid+error); boundary test case added to
`TestDecodeWatchEventTooShort` proving the 28-byte minimum is live (a 24-byte frame that would
have passed the old 16-byte check now correctly produces `decoder_error`).

### Step 2: Tests added

9 tests appended to `decoder_test.go` after the existing frame builders (later corrected in the D-S28.2-1 fix — see Step 1 above):
- Response frame builders: `stdRespFrame`, `connectRespFrame` (nolint:unused, Task 4), `watchEventFrame`, `feedRequest` (nolint:unused, Task 4)
- `TestDecodeWatchEvent` — watch_event + response_bytes, no decoder_error
- `TestDecodeWatchEventTooShort` — 8-byte payload < 28 min → decoder_error; PLUS boundary case proving the 28-byte minimum is live
- `TestDecodeResponseUnknownNegativeXid` — xid=-3 → decoder_error
- `TestDecodeResponseTooShortForXid` — 2-byte payload < 4 universal min → decoder_error
- `TestDecodeResponseOversized` — length prefix > maxPacketBytes → decoder_error + writeBuf nil
- `TestDecodeResponsePartialFrameReassembly` — 3-chunk split → decodes exactly once
- `TestDecodeResponseAbandonThenRecover` — oversized abandon + next write decodes normally
- `TestDecodeResponseMultipleFramesOneWrite` — two frames in one write → 2 watch_events
- `TestDecodeResponseCorrelatedRowsPendingTask4` — data xid → decoder_error (temporary Task-3 posture)

### Step 3: Failing run (compile error)

```
internal/filter/network/zookeeperproxy/decoder_test.go:610:4: d.decodeOnWrite undefined (type *decoder has no field or method decodeOnWrite)
... (12 errors total)
FAIL    github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy [build failed]
```

### Step 4: Implementation

Added to `decoder.go` after `onDataRequest`:
- `//nolint:unused` removed from `writeBuf` field (it is now used)
- `decodeOnWrite(p []byte)` — appends p to writeBuf, frame loop via nextWriteFrame + decodeResponseFrame
- `nextWriteFrame() ([]byte, bool)` — 4-byte BE length prefix scanner; oversized → responseError + nil
- `responseError(opname string)` — decoder_error + flag-gated per-opcode + writeBuf = nil
- `decodeResponseFrame(frame []byte) bool` — leading int32 dispatch: watchXid → onWatchEvent; correlated rows → responseError (Task-4 placeholder); default (unknown negative) → responseError
- `onWatchEvent(frame []byte) bool` — 28-byte min check (corrected from original 16-byte; see Step 1); watch_event + response_bytes

No signature adaptations required — all as-built fields/functions matched PLAN exactly.

### Step 5: Passing run

```
=== RUN   TestDecodeWatchEvent
--- PASS: TestDecodeWatchEvent (0.00s)
=== RUN   TestDecodeWatchEventTooShort
--- PASS: TestDecodeWatchEventTooShort (0.00s)
=== RUN   TestDecodeResponseUnknownNegativeXid
--- PASS: TestDecodeResponseUnknownNegativeXid (0.00s)
=== RUN   TestDecodeResponseTooShortForXid
--- PASS: TestDecodeResponseTooShortForXid (0.00s)
=== RUN   TestDecodeResponseOversized
--- PASS: TestDecodeResponseOversized (0.00s)
=== RUN   TestDecodeResponsePartialFrameReassembly
--- PASS: TestDecodeResponsePartialFrameReassembly (0.00s)
=== RUN   TestDecodeResponseAbandonThenRecover
--- PASS: TestDecodeResponseAbandonThenRecover (0.00s)
=== RUN   TestDecodeResponseMultipleFramesOneWrite
--- PASS: TestDecodeResponseMultipleFramesOneWrite (0.00s)
=== RUN   TestDecodeResponseCorrelatedRowsPendingTask4
--- PASS: TestDecodeResponseCorrelatedRowsPendingTask4 (0.00s)
PASS
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy     0.003s
```

Full suite (race):
```
ok      github.com/esalaine/envoy-go/internal/filter/network            1.010s
ok      github.com/esalaine/envoy-go/internal/filter/network/builtins   1.024s
ok      github.com/esalaine/envoy-go/internal/filter/network/directresponse    1.011s
ok      github.com/esalaine/envoy-go/internal/filter/network/echo       1.009s
ok      github.com/esalaine/envoy-go/internal/filter/network/rbac       1.019s
ok      github.com/esalaine/envoy-go/internal/filter/network/snicluster 1.009s
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy    1.107s
```

### Step 6: gofmt + golangci-lint

Initial lint run flagged 3 issues (before fixes):
1. `connectRespFrame` unused → added `//nolint:unused // retained for Task 4` comment
2. `feedRequest` unused → added `//nolint:unused // retained for Task 4` comment
3. `analogue` misspelling → corrected to `analog`

After fixes: gofmt clean (no output), golangci-lint clean (no output).

### Self-review

- All 9 tests exercise real production paths (no tautologies verified).
- `//nolint:unused` annotation removed from `writeBuf` field — now used by `decodeOnWrite`.
- D-S28.2-1 upstream verification completed; original IMPL conclusion was WRONG (16-byte rationalization), corrected at post-IMPL fix: minimum corrected to 28, wire format corrected to include full ReplyHeader, boundary test added. See Step 1 for full record.
- Diff confined to `decoder.go` + `decoder_test.go` + `PROGRESS.md`. Nothing else touched.
- gofmt + golangci-lint clean. Full suite green.

**Task 3 DONE (with D-S28.2-1 correction applied as separate commit).**

---

## Task 4: Correlated dispatch + correlation consumption + byte accounting (SPEC §3.3 rows 1/3/4 + §3.4)

### Step 1: Upstream verification + D-S28.2-5 filter.cc check

#### Previously recorded upstream facts (verified against /tmp/upstream_decoder.cc)

1. **Connect response min length = 20** (proto_version(4) + timeout(4) + session_id(8) + password_len(4) = 20 — NO zxid, NO error). Decoder.cc:309-315: "Connect responses are special, they have no full reply header but just an XID with no zxid nor error fields." SPEC's 20-byte pin is CORRECT.

2. **Correlate-then-validate confirmed**: upstream pops the pending request (fetchControlRequestData / fetchDataRequestData, decoder.cc:297-307) BEFORE parsing the response body. A malformed connect response still consumes the entry and fires the per-opcode decoder error.

3. **Data correlation = erase-on-lookup** (requests_by_xid_.find + erase, decoder.cc:1086-1105). **Control correlation = per-xid FIFO queue front+pop** (decoder.cc:1063-1084). Missing xid / empty queue → InvalidArgumentError → decoder_error WITHOUT per-opcode attribution.

#### D-S28.2-5: filter.cc onResponse verification

Fetched `https://raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/source/extensions/filters/network/zookeeper_proxy/filter.cc`.

**Finding:** `onResponse` (lines 354–385) does NOT increment any counter keyed on the `error` field value. The error is:
1. Used to compute latency metadata (passed to `onResponse` for histogram/fast/slow bucketing).
2. Written to dynamic metadata: `"error", std::to_string(error)` — informational only.
3. NOT used as a counter key or conditional dispatch trigger.

**Consequence for envoy-go Task 4:** The response decoders (`onDataResponse`, `onControlResponse`, `onConnectResponse`) do NOT need to read or dispatch on the error field. Shallow decode stops at `len(frame) < 16` (xid+zxid+error min-length validation); the error int32 is validated for presence but not extracted. No counter is keyed on it. Dynamic metadata is deferred per AMEND-A9.

**D-S28.2-5: CLOSED. Expected finding confirmed.**

### Step 2: Tests written (failing)

Added to `decoder_test.go` (after `TestDecodeResponseCorrelatedRowsPendingTask4`, which was DELETED):
- Removed `//nolint:unused` from `connectRespFrame` and `feedRequest`
- Deleted `TestDecodeResponseCorrelatedRowsPendingTask4` (Task-3 temporary-posture test — superseded)
- Added 11 Task-4 tests: TestDecodeDataResponseCorrelates, TestDecodeDataResponseDoubleResponse,
  TestDecodeDataResponseMissingXid, TestDecodeControlResponseFIFOAndUnderflow,
  TestDecodeConnectResponse, TestDecodeConnectReadonlyResponseMapsToConnect,
  TestDecodeConnectResponseEmptyQueue, TestDecodeDataResponseTruncatedAfterCorrelation,
  TestDecodeResponseBytesFlagGating, TestDecodeControlResponseAuthAndSetwatches,
  TestDecodeResponsesDrainCorrelationStructures

**Adaptations from PLAN snippets:**
- Custom-config tests (`TestDecodeDataResponseTruncatedAfterCorrelation`, `TestDecodeResponseBytesFlagGating`): used as-built pattern `rs.counters["..."].Load()` (not `counterValue()` which requires `*testing.T`) — actually counterValue also works (it just calls `rs.counters[suffix].Load()`); used the `rs.counters["..."].Load()` direct pattern for the custom-config sub-tests to match the `TestDecodeMinLengthViolation` / `TestDecodeFlagGatedRequestBytes` precedent.
- `connectFrame(*bool)` signature confirmed: `connectFrame(nil)` for plain, `ro := true; connectFrame(&ro)` for readonly.
- `be32`/`be64`/`opGetData`/`opSetData`/`opPing`/`opSetAuth`/`opSetWatches` all confirmed as-built names.
- `padTo(opcode)` confirmed as-built name.
- `zookeeper_proxyv3` import alias confirmed.

### Step 3: Failing run

```
=== RUN   TestDecodeDataResponseCorrelates
    decoder_test.go:741: getdata_resp = 0, want 1
--- FAIL: TestDecodeDataResponseCorrelates (0.00s)
=== RUN   TestDecodeDataResponseDoubleResponse
    decoder_test.go:762: getdata_resp = 0, want 1
--- FAIL: TestDecodeDataResponseDoubleResponse (0.00s)
=== RUN   TestDecodeDataResponseMissingXid
--- PASS: TestDecodeDataResponseMissingXid (0.00s)
=== RUN   TestDecodeControlResponseFIFOAndUnderflow
    decoder_test.go:788: ping_resp = 0, want 2
--- FAIL: TestDecodeControlResponseFIFOAndUnderflow (0.00s)
=== RUN   TestDecodeConnectResponse
    decoder_test.go:806: connect_resp = 0, want 1
--- FAIL: TestDecodeConnectResponse (0.00s)
...
FAIL    github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy 0.006s
```

(MissingXid and EmptyQueue accidentally passed because the placeholder responseError("") already fires decoder_error on the unmatched path — they are correct-behavior tests that the placeholder happened to satisfy.)

### Step 4: Implementation

Added to `decoder.go`:
- `respOpname(entryOpname string) string` — free function; maps `connect_readonly` → `connect`; everything else passes through
- `popControl(xid int32) (pendingRequest, bool)` — FIFO-pops control queue under `d.mu`; copies entry out before returning (counters happen OUTSIDE the lock — §3.6)
- `takeData(xid int32) (pendingRequest, bool)` — erase-on-lookup for `requestsByXid` under `d.mu`
- `countResponse(respOpname string, frame []byte)` — `<opname>_resp` (always) + `response_bytes` (always) + flag-gated `<opname>_resp_bytes`
- `onConnectResponse(frame []byte) bool` — connect special framing (20-byte min; popControl connectXid; counters use "connect" regardless of entry opname)
- `onControlResponse(xid int32, frame []byte) bool` — ping/auth/setwatches FIFO pop; 16-byte min
- `onDataResponse(xid int32, frame []byte) bool` — erase-on-lookup; 16-byte min

`decodeResponseFrame` middle case replaced: `connectXid → onConnectResponse`, `pingXid/authXid/setWatchesXid → onControlResponse`, `> 0 → onDataResponse`.

**No-adaptation notes:**
- `enablePerOpcodeResponseBytesMetrics` field name confirmed exact.
- `wireFootprint(frame)` returns `4 + len(frame)` — passes the prefixed-frame length correctly since `frame` is the payload-only slice (the 4-byte prefix is stripped by `nextWriteFrame`). So `wireFootprint(frame) = 4 + len(frame)` = total on-wire bytes. CORRECT.
- All counter names (`connect_resp`, `ping_resp`, `auth_resp`, `setwatches_resp`) confirmed in `respOpNames`.
- `connect_decoder_error` confirmed in `decoderErrorOpNames` (no `connect_readonly_decoder_error` — consistent with the mapping).

### Step 5: Passing run

```
$ go test ./internal/filter/network/zookeeperproxy/ -v -count=1
[...all 80+ tests PASS including all 11 Task-4 tests...]
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy     0.010s

$ go test ./internal/filter/network/... -race -short -count=1
ok      github.com/esalaine/envoy-go/internal/filter/network            1.011s
ok      github.com/esalaine/envoy-go/internal/filter/network/builtins   1.024s
ok      github.com/esalaine/envoy-go/internal/filter/network/directresponse    1.012s
ok      github.com/esalaine/envoy-go/internal/filter/network/echo       1.011s
ok      github.com/esalaine/envoy-go/internal/filter/network/rbac       1.020s
ok      github.com/esalaine/envoy-go/internal/filter/network/snicluster 1.010s
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy    1.117s
```

### Step 6: gofmt + lint + commit

Initial gofmt run flagged one issue: the `_ = entry` line in `onConnectResponse` had a tab-indentation drift after copy. Fixed via `gofmt -w`. Re-run: both gofmt and golangci-lint clean (no output).

---

## Task 5: Latency-threshold counters (recordLatency)

### Adaptations from PLAN

- **Proto/field names confirmed exact**: `zookeeper_proxyv3.LatencyThresholdOverride`, `LatencyThresholdOverride_GetData`, `DefaultLatencyThreshold`, `LatencyThresholdOverrides`, `EnableLatencyThresholdMetrics` — all verified against as-built `config.go` + `config_test.go`.
- **`durationpb` import**: added `durationpb "google.golang.org/protobuf/types/known/durationpb"` to `decoder_test.go` (was only present in `config_test.go`). Also added `"time"` import.
- **`latencyTestDecoder` returns 2 values** (`*decoder, *rosterStats`) — no `*compiledConfig` third return (tests don't need it). Consistent with PLAN.
- **`compiledConfig` field names confirmed exact**: `enableLatencyThresholdMetrics bool`, `defaultLatencyThreshold time.Duration`, `latencyThresholdOverrides map[int32]time.Duration` (keyed by wire opcode `int32`). PLAN code matched as-built exactly.
- **`_ = entry` placeholder removed** from `onConnectResponse` as directed; `recordLatency("connect", entry.wireOpcode, time.Since(entry.start))` added after `countResponse`.

### Step 2: Failing run (compile error)

```
internal/filter/network/zookeeperproxy/decoder_test.go:978:4: d.recordLatency undefined (type *decoder has no field or method recordLatency)
... (5 errors total)
FAIL    github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy [build failed]
```

### Step 3: Implementation

- `recordLatency(respOpname string, wireOpcode int32, latency time.Duration)` appended after `onDataResponse` in `decoder.go`.
- Three wiring call sites (each AFTER `countResponse`):
  - `onConnectResponse`: deleted `_ = entry` placeholder; added `d.recordLatency("connect", entry.wireOpcode, time.Since(entry.start))`
  - `onControlResponse`: added `d.recordLatency(op, entry.wireOpcode, time.Since(entry.start))`
  - `onDataResponse`: added `d.recordLatency(op, entry.wireOpcode, time.Since(entry.start))`

### Step 4: Passing run

```
=== RUN   TestRecordLatencyInclusiveEdge
--- PASS: TestRecordLatencyInclusiveEdge (0.00s)
=== RUN   TestRecordLatencyOverrideBeatsDefault
--- PASS: TestRecordLatencyOverrideBeatsDefault (0.00s)
=== RUN   TestRecordLatencyFlagOff
--- PASS: TestRecordLatencyFlagOff (0.00s)
=== RUN   TestLatencyEndToEndInjectedStart
--- PASS: TestLatencyEndToEndInjectedStart (0.00s)
=== RUN   TestLatencyConnectResponse
--- PASS: TestLatencyConnectResponse (0.00s)
=== RUN   TestLatencyNeverForWatchEventsOrErrors
--- PASS: TestLatencyNeverForWatchEventsOrErrors (0.00s)
PASS
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy 0.003s

$ go test ./internal/filter/network/... -race -short -count=1
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy 1.136s
```

### Step 5: gofmt + golangci-lint

Both clean (no output).

### Self-review

- `<=` edge is INCLUSIVE (`latency <= threshold → _resp_fast`): confirmed correct.
- `recordLatency` called with roster-MAPPED opname in all three handlers (never `connect_readonly` — that's mapped via `respOpname()` before `countResponse`/`recordLatency`).
- `time.Since(entry.start)` computed OUTSIDE the lock (entry was COPIED OUT under the lock by `popControl`/`takeData` — §3.6 invariant holds).
- Fast/slow counter names exist in the stats roster for all opnames used in tests (`getdata`, `setdata`, `connect`, `exists`, `ping`, `auth`, `setwatches`) — confirmed via `respOpNames` in `stats.go`.
- Diff confined to `decoder.go` + `decoder_test.go` + `PROGRESS.md`. `config.go` and `stats.go` UNTOUCHED.
- gofmt + golangci-lint clean. Full suite green.

**Task 5 DONE.**

**Task 5 post-commit fix (code-review findings):**
- (Important) `recordLatency` parameter renamed from `respOpname` to `op` — eliminated shadowing of the package-level `respOpname()` function; doc comment updated accordingly. `countResponse` already used `op` — no change needed there.
- (Minor) `TestLatencyConnectResponse` comment corrected: removed false claim about overrides; now says "here via the default threshold; override-vs-default precedence is pinned by TestRecordLatencyOverrideBeatsDefault".
- (Minor) Added `TestLatencyControlResponse` end-to-end test pinning the `onControlResponse → recordLatency` wiring (ping path; 1 h threshold → `ping_resp_fast = 1`).
- gofmt + golangci-lint clean. Full suite + race detector green.

---

## Task 6: OnWrite glue + the §3.6 concurrent request/response race test (R9)

### Step 1: Tests written (failing filter-level tests)

Deleted `TestFilterOnWritePureNoOp` (pins the 28.1 no-op posture — exactly what this task removes).

Added to `zookeeperproxy_test.go`:
- `TestFilterOnWriteFeedsDecoder` — OnWrite feeds the decoder's write side; asserts `getdata_resp=1` + `response_bytes=len(resp)` + buffer Len unchanged (R3 immutability) + return Continue.
- `TestFilterOnWritePartialFramesAcrossCalls` — two half-frame OnWrite calls each with a FRESH `*network.Buffer` (as writeChainConn.Write does); asserts `getdata_resp=1` (reassembled across calls).

Added `"sync"` import to `decoder_test.go` and appended:
- `TestDecoderConcurrentRequestResponseRace` — goroutine A (200 feedRequest calls via the delta-feed helper) vs goroutine B (200 decodeOnWrite calls) concurrently over one decoder; conservation check: `getdata_resp + decoder_error == 200`. Includes an inline comment explaining the conservation soundness (abandon sets writeBuf=nil but `append(nil, p...)` is valid so each decodeOnWrite call is self-contained).

**Adaptations from PLAN:**
- Goroutine A uses `feedRequest(d, frame)` (the existing delta-feed helper at decoder_test.go:607) rather than a manual `consumed` accumulator — `feedRequest` already increments `d.chainConsumed` correctly and is single-goroutine safe (chainConsumed is owned exclusively by goroutine A per §3.6).
- Stats access: `f.cfg.stats.counters["..."].Load()` — confirmed as-built pattern.
- All test helpers (`dataFrame`, `stdRespFrame`, `padTo`, `opGetData`) confirmed in `decoder_test.go`.

### Step 2: Verify race test passes pre-implementation

The race test passed 5/5 pre-implementation (as expected — `decodeOnWrite` + `mu` already in place from Tasks 2–4):

```
$ go test ./internal/filter/network/zookeeperproxy/ -run TestDecoderConcurrentRequestResponseRace -race -count=5 -v
=== RUN   TestDecoderConcurrentRequestResponseRace
--- PASS: TestDecoderConcurrentRequestResponseRace (0.00s)
[x5]
PASS
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy     1.021s
```

This is expected: the race test's purpose is the PERMANENT R9 regression gate — it pinned the §3.6 mutex design from the moment `mu` was added. The pre-impl pass is a correctness signal, not a problem.

### Step 3: Failing run (filter tests)

```
$ go test ./internal/filter/network/zookeeperproxy/ -run 'TestFilterOnWrite' -v -count=1
=== RUN   TestFilterOnWriteFeedsDecoder
    zookeeperproxy_test.go:155: getdata_resp = 0, want 1 (OnWrite must feed the response decoder)
--- FAIL: TestFilterOnWriteFeedsDecoder (0.00s)
=== RUN   TestFilterOnWritePartialFramesAcrossCalls
    zookeeperproxy_test.go:182: getdata_resp = 0, want 1 (reassembled across OnWrite calls)
--- FAIL: TestFilterOnWritePartialFramesAcrossCalls (0.00s)
FAIL
FAIL    github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy     0.002s
```

Both fail as expected: `getdata_resp = 0, want 1` (OnWrite is still the 28.1 no-op).

### Step 4: Implementation

In `zookeeperproxy.go`, replaced the no-op `OnWrite` with the decoder feed:

```go
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
    f.decoder.decodeOnWrite(buf.Bytes())
    return network.Continue
}
```

Updated the doc comment (13-line comment per PLAN — AMEND-A8 unconditional passthrough, R3, fresh per-Write Buffer per writeChainConn.Write, §3.2 item 1 / ADR-0223, goroutine B, §3.6 mutex).

No signature adaptation needed: the as-built no-op already used `buf *network.Buffer` (not `_ *network.Buffer`) in the method body-replacement form.

### Step 5: Race gate (5/5 post-implementation)

```
$ go test ./internal/filter/network/zookeeperproxy/ -run TestDecoderConcurrentRequestResponseRace -race -count=5 -v
=== RUN   TestDecoderConcurrentRequestResponseRace
--- PASS: TestDecoderConcurrentRequestResponseRace (0.00s)
[x5]
PASS
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy     1.022s
```

Zero race reports. 5/5 clean.

### Full suite

```
$ go test ./internal/filter/network/... -race -short -count=1
ok      github.com/esalaine/envoy-go/internal/filter/network            1.011s
ok      github.com/esalaine/envoy-go/internal/filter/network/builtins   1.025s
ok      github.com/esalaine/envoy-go/internal/filter/network/directresponse    1.013s
ok      github.com/esalaine/envoy-go/internal/filter/network/echo       1.010s
ok      github.com/esalaine/envoy-go/internal/filter/network/rbac       1.021s
ok      github.com/esalaine/envoy-go/internal/filter/network/snicluster 1.010s
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy    1.138s
```

### 28.1b framework-level concurrent pumps (SPEC §3.6 item 4)

```
$ go test ./internal/filter/network/ -run TestWrappedChainConcurrentPumpsRace -race -count=5 -v
=== RUN   TestWrappedChainConcurrentPumpsRace
--- PASS: TestWrappedChainConcurrentPumpsRace (0.00s)
[x5]
PASS
ok      github.com/esalaine/envoy-go/internal/filter/network    1.013s
```

### Step 6: gofmt + lint

Both clean (no output from either).

### Self-review

- OnWrite returns Continue unconditionally (no error path). ✓
- Chain Buffer stays unmutated: test asserts `respBuf.Len() == before` before and after the call. ✓
- Race test ran 5/5 clean under -race (pre-impl AND post-impl). ✓
- 28.1b framework race test stayed green (5/5). ✓
- `TestFilterOnWritePureNoOp` deleted. ✓
- Diff confined to the 4 allowed files + PROGRESS.md: `zookeeperproxy.go`, `zookeeperproxy_test.go`, `decoder_test.go`, `PROGRESS.md`. `decoder.go` untouched. ✓
- gofmt + golangci-lint clean. ✓

**Task 6 DONE.**

---

## Task 7: FuzzZookeeperResponseDecode — the 38th fuzzer (SPEC §6 / R10)

### writeBuf bound derivation

`nextWriteFrame` semantics:
- `len(writeBuf) < 4` → return false (stay partial, no change to writeBuf)
- `frameLen < 0 || frameLen > maxPkt` → `responseError` → `writeBuf = nil` (abandoned)
- `len(writeBuf) < 4 + frameLen` → return false (partial frame, no change)
- Otherwise → consume frame: `writeBuf = writeBuf[4+frameLen:]`

**Post-call invariant:** After `decodeOnWrite` returns, `writeBuf` holds AT MOST one incomplete
frame: a valid length prefix (declared `frameLen ≤ maxPkt`) with fewer than `4+frameLen` bytes
present. Therefore `len(writeBuf) ≤ 4 + maxPkt - 1 = maxPkt + 3`. An oversized frame
(`frameLen > maxPkt`) abandons writeBuf to `nil` immediately.

This post-call invariant holds after EACH call to `decodeOnWrite` independently — the second
call in the fuzzer appends data to the residual partial-frame bytes and then runs the same
frame loop until only a partial (or nil) remains. So after two calls, writeBuf is still
bounded by `maxPkt + 3`.

**Bound used: `maxPkt + 8`** (matches the request-side fuzzer style; +5 slack above the
tight `maxPkt + 3` bound accommodates future framing changes without invalidating the test).
The PLAN's `maxPkt + 8` is therefore correct with no spurious failures possible.

### Pre-load map verification

- `feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))` → `requestsByXid[1]` (1 entry)
- `ro := true; feedRequest(d, connectFrame(&ro))` → `controlRequestsByXid[connectXid]` len=1 with opname `"connect_readonly"` (the §3.4-item-4 trap entry)
- `feedRequest(d, zkFrame(be32(pingXid), be32(opPing)))` → `controlRequestsByXid[pingXid]` len=1
- `correlationSize(d)` before fuzz = 3 entries total

### Fuzz outputs

**Seed corpus (7 seeds, all PASS):**
```
$ go test ./internal/filter/network/zookeeperproxy/ -run FuzzZookeeperResponseDecode -count=1 -v
=== RUN   FuzzZookeeperResponseDecode
=== RUN   FuzzZookeeperResponseDecode/seed#0
=== RUN   FuzzZookeeperResponseDecode/seed#1
=== RUN   FuzzZookeeperResponseDecode/seed#2
=== RUN   FuzzZookeeperResponseDecode/seed#3
=== RUN   FuzzZookeeperResponseDecode/seed#4
=== RUN   FuzzZookeeperResponseDecode/seed#5
=== RUN   FuzzZookeeperResponseDecode/seed#6
--- PASS: FuzzZookeeperResponseDecode (0.00s)
PASS
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy     0.003s
```

**30s live fuzz (zero crashers):**
```
$ go test ./internal/filter/network/zookeeperproxy/ -fuzz FuzzZookeeperResponseDecode -fuzztime 30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/7 completed
fuzz: elapsed: 0s, gathering baseline coverage: 7/7 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 162523 (54166/sec), new interesting: 20 (total: 27)
fuzz: elapsed: 6s, execs: 343124 (60199/sec), new interesting: 21 (total: 28)
fuzz: elapsed: 9s, execs: 525177 (60692/sec), new interesting: 21 (total: 28)
fuzz: elapsed: 12s, execs: 711147 (61986/sec), new interesting: 22 (total: 29)
fuzz: elapsed: 15s, execs: 894421 (61094/sec), new interesting: 22 (total: 29)
fuzz: elapsed: 18s, execs: 1076101 (60557/sec), new interesting: 22 (total: 29)
fuzz: elapsed: 21s, execs: 1260055 (61315/sec), new interesting: 22 (total: 29)
fuzz: elapsed: 24s, execs: 1443044 (61006/sec), new interesting: 22 (total: 29)
fuzz: elapsed: 27s, execs: 1627032 (61328/sec), new interesting: 23 (total: 30)
fuzz: elapsed: 30s, execs: 1809632 (60862/sec), new interesting: 23 (total: 30)
fuzz: elapsed: 30s, execs: 1809632 (0/sec), new interesting: 23 (total: 30)
PASS
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy     30.151s
```

**Both fuzzers seed corpora:**
```
$ go test ./internal/filter/network/zookeeperproxy/ -run Fuzz -count=1
ok      github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy     0.004s
```

### Fuzzer count

```
$ grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l
38
```

### gofmt + golangci-lint

Both clean (no output).

### Self-review

- All three invariants genuinely asserted: no-panic (implicit), maps-never-grow (explicit Fatalf), writeBuf bounded (explicit Fatalf).
- writeBuf bound `maxPkt+8` is provably correct: tight bound is `maxPkt+3`; +5 slack, no spurious failures possible.
- 30s fuzz: 1,809,632 execs, zero crashers. Validated.
- Fuzzer count = 38. ✓
- Diff confined to `fuzz_test.go` + `PROGRESS.md`. ✓
- gofmt + golangci-lint clean. ✓

**Task 7 DONE.**

## Task 8: TCPZKResponder BackendKind=29 + runner arm + unit test

### What was implemented

- **`fixture.go`**: Added `TCPZKResponder BackendKind = 29` const after `TCPSink = 28`, with full doc comment describing the fixed-delay deterministic-threshold construction (D-P9), trigger behaviors (D-S28.2-2), and the D-S28.2-1 full-ReplyHeader watch-push format. Also updated `TCPSink`'s trailing sentence to reference "That responder is TCPZKResponder = 29 (landed at 28.2)."

- **`runner_test.go`**:
  - Added `encoding/binary` import.
  - Added `case fixture.TCPZKResponder:` dispatch arm (after `TCPSink`) following the exact as-built `TCPSink` arm structure: `net.Listen("tcp", "0.0.0.0:0")`, `defer ln.Close()`, `bo.ln`, `bo.port`, `go acceptZKResponder(ln, bo.accepts)`.
  - Added constants: `zkResponderDelay = 10 * time.Millisecond`, `zkTriggerWrongXid int32 = 6`, `zkTriggerWatchPush int32 = 3`.
  - Added `acceptZKResponder(ln net.Listener, counter *atomic.Uint64)` — accept loop counting connections.
  - Added `zkRespondLoop(c net.Conn)` — the per-connection canned-response loop: parses 4-byte length prefix + frame, sleeps `zkResponderDelay`, then responds. Connect (xid==0) → 20-byte connect response. Data → 16-byte echo with monotonic zxid. getacl (op 6) → xid+1000 wrong-xid trigger. exists (op 3) → normal response + unsolicited watch-event push.
  - **Watch-push frame (D-S28.2-1 corrected format)**: `xid(−1,4) + zxid(8) + error(0,4) + event_type(1,4) + client_state(3,4) + path-len(4) + path` = 37 bytes, well above the 28-byte minimum required by upstream `parseWatchEvent`.
  - Added `TestZKResponderBackend` unit test verifying all four behaviors: connect response ≥ fixed delay + 20-byte format; getdata xid echo (16 bytes); getacl wrong-xid (xid+1000); exists watch-push (first response xid=10, then push xid=−1, len≥28, bytes[16:20]=event_type 1).

### Test run

```
$ go test ./test/differential/ -run TestZKResponderBackend -v -count=1
=== RUN   TestZKResponderBackend
--- PASS: TestZKResponderBackend (0.04s)
PASS
ok      github.com/esalaine/envoy-go/test/differential  0.121s
```

### go vet + gofmt + golangci-lint

All clean (no output).

### Self-review

- Watch-push frame includes `zxid(8) + error(4)` between xid and event_type — the D-S28.2-1 corrected format. Total frame = 37 bytes ≥ 28. ✓
- Unit test asserts `len(push) >= 28` AND `push[16:20]` = event_type 1 (verifying the correct offset after xid+zxid+error = 16 bytes). ✓
- Backend-dispatch arm follows the as-built `TCPSink` arm structure exactly (same `bo` fields, same `defer` pattern). ✓
- Diff confined to `fixture/fixture.go`, `runner_test.go`, and `PROGRESS.md`. ✓
- gofmt/lint/vet clean. ✓

**Task 8 DONE.**

## Task 9: 0048-zookeeper-responses fixture + cross-side GREEN + R4 + README

### Files

- **`test/fixtures/0048-zookeeper-responses/driver/driver.go`** (NEW) — mirrors
  the 0046 driver: four listeners (`l_resp`/`l_fast`/`l_slow`/`l_rflags`,
  ref ports 15050-15053, subject ports `subjListenerPort`+0..3), `BackendKind()`
  = `fixture.TCPZKResponder` (cluster `c_zk`), `MultiListenerDriver` +
  `StatsAsserter`. Round-trip driving (`driveRoundTrips`/`readZKFrame`): each arm
  writes a request frame then reads the expected number of response frames before
  proceeding. Copied 0046's frame builders + scrape/lookup helpers; added
  `existsPayload` / `getaclPayload` / `syncPayload` / `setdataPayload` /
  `deletePayload` (each meeting its request-side min-length from
  `decoder.go:dataRequestMinLength`: exists/getdata 13, getacl/sync 12, setdata
  20, delete 16). Wire opcodes redeclared locally, verified against `config.go`.
- **`test/fixtures/0048-zookeeper-responses/README.md`** (NEW) — topology,
  four-listener/stat-prefix table, deterministic-threshold construction,
  trigger-opcode encoding (incl. D-S28.2-1 full-ReplyHeader watch-push), 8-arm
  taxonomy with per-arm counters, round-trip discipline, cross-side equality,
  the R4 record, R5 ratification.
- **`test/differential/runner_test.go`** — added the `0048-zookeeper-responses/driver`
  blank-import after the `0047` line (ONLY change).

### Proto enum spelling

The `latency_threshold_overrides` opcode enum name `GetData` (value 4 in
`LatencyThresholdOverride_Opcode_name`) is accepted by reference Envoy v1.37.2 on
its first boot — NO spelling correction was needed.

### Step 3: fixture run — GREEN on first run (no debugging)

```
$ go test ./test/differential/ -run 'TestDifferential/0048-zookeeper-responses' -v -count=1 -timeout 600s
--- PASS: TestDifferential (3.87s)
    --- PASS: TestDifferential/0048-zookeeper-responses (3.87s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	3.952s
```

No debugging was required: the fixture passed cross-side on the first run.

### Step 4: R4 deliberate-break protocol (both `-count=1`)

**Break (a) — wrong expected value.** Edited `{"zk_resp.zookeeper.getdata_resp",
1}` → `2`. FAILED on BOTH sides:

```
runner_test.go:1080: ref zk_resp.zookeeper.getdata_resp = 1, want 2
runner_test.go:1080: subj zk_resp.zookeeper.getdata_resp = 1, want 2
--- FAIL: TestDifferential/0048-zookeeper-responses (3.96s)
```

Reverted (value back to 1).

**Break (b) — production-side liveness.** Commented out
`d.countResponse("connect", frame)` in `onConnectResponse`
(`internal/filter/network/zookeeperproxy/decoder.go`). FAILED with SUBJECT-only
divergence (no `ref` errors — the reference still counted connect_resp):

```
runner_test.go:1080: subj zk_resp.zookeeper.connect_resp = 0, want 1
runner_test.go:1080: subj zk_fast.zookeeper.connect_resp = 0, want 1
--- FAIL: TestDifferential/0048-zookeeper-responses (4.09s)
```

Reverted. Revert verification:

```
$ git diff --stat internal/ test/
 test/differential/runner_test.go | 1 +
$ git diff internal/   # (empty — both breaks fully reverted)
```

Post-revert baseline re-confirmed GREEN (`-count=1`):

```
--- PASS: TestDifferential/0048-zookeeper-responses (4.22s)
```

### Step 6: no-regression spot check (`-count=1`)

```
$ go test ./test/differential/ -run 'TestDifferential/(0001-tcp-proxy-rr|0046-zookeeper-requests|0047-zookeeper-boot-reject)' -v -count=1 -timeout 900s
    --- PASS: TestDifferential/0001-tcp-proxy-rr (2.05s)
    --- PASS: TestDifferential/0046-zookeeper-requests (4.42s)
    --- PASS: TestDifferential/0047-zookeeper-boot-reject (1.46s)
PASS
```

3/3 PASS.

### Step 7: gofmt + lint

`gofmt -l test/` and `golangci-lint run ./test/...` both clean (no output).

**Task 9 DONE.**

### Task 9 review fix: expectation coverage gaps closed

Code-review finding: three listeners had sparse expectation coverage.

**l_slow — missing TOTAL `_resp` counters.** Added
`connect_resp`=1, `setdata_resp`=1, `delete_resp`=1, `getdata_resp`=1 to the
fixed-value expectations. Without these a bug incrementing a latency bucket but
not the total (or vice versa) would have been undetected.

**l_rflags — missing `connect_resp` row.** Arm 7 sends connect + getdata; only
`getdata_resp` was previously asserted. Added `connect_resp`=1.

**l_rflags — `connect_resp_bytes` not asserted.** Arm 7's connect frame receives a
20-byte response body + 4-byte prefix = 24 wire bytes, so the flag-ON
`connect_resp_bytes` counter is exercised. Added it to the cross-side equality set
(present + equal + > 0) alongside `getdata_resp_bytes`.

**l_fast — `request_bytes` / `response_bytes` not asserted.** Arm 4 exercises these
counters on `l_fast` but they were absent from both the fixed-value and
cross-side-equality tables. Added both to the cross-side equality set.

After adding all rows: `go test ./test/differential/ -run
'TestDifferential/0048-zookeeper-responses' -v -count=1 -timeout 600s` →
`PASS`. `gofmt -l test/` and `golangci-lint run ./test/...` both clean.

Committed as: phase 28.2 Task 9 fix: close expectation coverage gaps — l_slow
total _resp counters, l_rflags connect row + connect_resp_bytes equality, l_fast
byte-counter equality (review finding)

---

## Task 10: Completion bundle — ADR-0223 body + BEHAVIOR_CONTRACT 28.2 bundle

### Step 1: ADR-0223 §Decision + §Consequences body

The §Decision + §Consequences body was appended in-place to ADR-0223 in
`docs/envoy-go/DECISIONS.md` (after the existing §AMEND + §Context). Style
matched ADR-0221 and ADR-0222 (numbered decision items with bold headers,
cross-reference block at the end, §Consequences as separate section).

**Sections written:**

1. §Decision item 1 — Unified decoder + write-side reassembly (the rename;
   NO write-side TotalAppended; structural asymmetry vs read side explained).
2. §Decision item 2 — Response dispatch table (5-row table; D-S28.2-1
   corrected 28-byte watch minimum; universal pre-dispatch min asymmetry;
   D-S28.2-5 shallow-decode confirmed).
3. §Decision item 3 — Correlation consumption semantics (erase-on-lookup;
   FIFO pop; correlate-then-validate order; connect_readonly→connect mapping;
   correlation structures now drained by responses).
4. §Decision item 4 — THE PER-CONNECTION MUTEX (ADR-0221 forward-pointer
   explicitly stated as DISCHARGED; exact mu scope; rationale; R9 proof).
5. §Decision item 5 — Latency-threshold counters (recordLatency; inclusive
   <=; wire-opcode override; deterministic-threshold discipline; proto vs
   wire enum AMEND-A6; proof surface summary).
6. §Consequences — phase-28 both-direction close; ADR-0221 forward-pointer
   discharge explicit; histogram deferral (ADR-0060); leniency departure;
   counts at close; THIRD §9 row closes; 4 candidates remain; cross-references.

**D-S28.2-1 handling:** the SPEC's 16-byte watch-event minimum is explicitly
called out as WRONG, the corrected upstream value (28 bytes) is stated, the
correction rationale is given (full ReplyHeader on the wire), and both the
ADR body and BEHAVIOR_CONTRACT record the corrected fact.

### Step 2: BEHAVIOR_CONTRACT 28.2 bundle

Six edits applied:

1. **`### envoy.filters.network.zookeeper_proxy` response-side extension:**
   Added after the existing Differential bullet — response dispatch table
   (5-row with D-S28.2-1 corrected 28-byte watch minimum), correlation-
   consumption semantics, latency-threshold semantics, proto/wire enum note
   (AMEND-A6), response-side leniency departure, latency-HISTOGRAM boundary
   (ADR-0060), 0048 differential record + 38th fuzzer.

2. **Conn-wrap-seam forward-pointer RESOLVED:** The `### Network filter chain
   framework` block's "FORWARD-POINTER (28.2 / ADR-0223)" sentence replaced
   with "FORWARD-POINTER DISCHARGED (ADR-0223 §Decision item 4)" recording
   the landed mutex, its exact scope, and R9 ratification.

3. **Latency-HISTOGRAM coverage boundary** recorded in the response-side
   extension block (edit 1) AND in the `### Does not yet apply to` section.

4. **"Control queues grow unbounded" boundary REWRITTEN** as upstream-parity
   behavior in the correlation-consumption block (responses now drain both
   structures; residual unanswered-request growth is upstream's behavior too).

5. **Proto/wire enum note** (AMEND-A6) included in the latency-threshold
   semantics block.

6. **`### Stat surface`** — 28.2 zero-creation-delta sentence added.
   **`## Stat-name mapping`** — "Phase 28.2 extension — 337 → 337" paragraph
   added above the 28.1 block.
   **`### Applies to`** — 28.2 bullet added (response side + mutex + 0048 +
   38th fuzzer + parent-row-28 close).
   **`### Does not yet apply to`** — old "Response-side decode + latency
   counters — 28.2 / ADR-0223" forward-pointer bullet REMOVED; latency-
   HISTOGRAM deferral bullet ADDED; Network-family-filters bullet UPDATED to
   reflect phase-28 closed + 4 candidates remain.

### Step 3: Tail verification

```
$ grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -1
ADR-0223
```

DECISIONS.md tail is still ADR-0223. No new ADR number minted.

---

## Task 11: Six-gate (50-dir differential -count=1; h2spec; proxy-wasm) + ROADMAP ATOMIC rollup + STATE + next-prompt

### Final counts (R6)

```
$ ls -d test/fixtures/[0-9]* | wc -l
50

$ ls -d test/fixtures/[0-9]* | tail -1
test/fixtures/0048-zookeeper-responses

$ grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l
38

$ grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -1
ADR-0223
```

All counts match expected: 50 fixtures / tail 0048-zookeeper-responses / 38 fuzzers / ADR-0223 tail.

---

### Gate 1: `go build ./...`

```
$ go build ./...
(no output — clean)
```

**Gate 1: PASS (clean)**

---

### Gate 2: `go vet ./...`

```
$ go vet ./...
(no output — clean)
```

**Gate 2: PASS (clean)**

---

### Gate 3: `golangci-lint run`

```
$ golangci-lint run
(no output — clean)
```

**Gate 3: PASS (clean)**

---

### Gate 4: `go test ./... -race -short -count=1`

```
$ go test ./... -race -short -count=1
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	7.447s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.038s
ok  	github.com/esalaine/envoy-go/internal/admin	1.634s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.114s
ok  	github.com/esalaine/envoy-go/internal/clock	1.029s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.120s
ok  	github.com/esalaine/envoy-go/internal/drain	1.129s
ok  	github.com/esalaine/envoy-go/internal/dynamicmetadata	1.018s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.124s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	8.419s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.308s
ok  	github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency	1.081s
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	1.117s
ok  	github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit	1.459s
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.047s
ok  	github.com/esalaine/envoy-go/internal/filter/http/compressor	1.068s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.025s
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.032s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.062s
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.421s
ok  	github.com/esalaine/envoy-go/internal/filter/http/extproc	1.248s
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.284s
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.016s
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.113s
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.024s
ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	3.457s
ok  	github.com/esalaine/envoy-go/internal/filter/http/oauth2	1.049s
ok  	github.com/esalaine/envoy-go/internal/filter/http/ratelimit	1.048s
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.026s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.243s
ok  	github.com/esalaine/envoy-go/internal/filter/http/wasm	1.086s
ok  	github.com/esalaine/envoy-go/internal/filter/network	1.010s
ok  	github.com/esalaine/envoy-go/internal/filter/network/builtins	1.018s
ok  	github.com/esalaine/envoy-go/internal/filter/network/directresponse	1.009s
ok  	github.com/esalaine/envoy-go/internal/filter/network/echo	1.007s
ok  	github.com/esalaine/envoy-go/internal/filter/network/rbac	1.021s
ok  	github.com/esalaine/envoy-go/internal/filter/network/snicluster	1.009s
ok  	github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy	1.173s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.189s
ok  	github.com/esalaine/envoy-go/internal/filterstate	1.053s
ok  	github.com/esalaine/envoy-go/internal/grpcclient	1.149s
ok  	github.com/esalaine/envoy-go/internal/httpclient	1.093s
ok  	github.com/esalaine/envoy-go/internal/jwks	2.677s
ok  	github.com/esalaine/envoy-go/internal/jwt	1.145s
ok  	github.com/esalaine/envoy-go/internal/listener	4.107s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.053s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.015s
ok  	github.com/esalaine/envoy-go/internal/lua	1.076s
ok  	github.com/esalaine/envoy-go/internal/matcher	1.018s
ok  	github.com/esalaine/envoy-go/internal/rbac	1.015s
ok  	github.com/esalaine/envoy-go/internal/sdsfile	1.767s
ok  	github.com/esalaine/envoy-go/internal/stats	1.028s
ok  	github.com/esalaine/envoy-go/internal/stats/dynamic	1.194s
ok  	github.com/esalaine/envoy-go/internal/tls	1.126s
ok  	github.com/esalaine/envoy-go/internal/wasm	1.631s
ok  	github.com/esalaine/envoy-go/internal/wasm/abi	1.029s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	1.150s
ok  	github.com/esalaine/envoy-go/test/conformance/proxy-wasm	3.748s
ok  	github.com/esalaine/envoy-go/test/differential	1.206s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.018s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.019s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.018s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.017s
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.021s
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.017s
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.021s
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	1.019s
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	1.020s
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	1.020s
ok  	github.com/esalaine/envoy-go/test/fixtures/0016-http-compressor/inputs	1.022s
ok  	github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/pki	1.013s
ok  	github.com/esalaine/envoy-go/test/helpers	1.025s
ok  	github.com/esalaine/envoy-go/test/helpers/echobackend	1.018s
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzgrpc	1.041s
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzhttp	1.019s
ok  	github.com/esalaine/envoy-go/test/helpers/extprocgrpc	1.042s
ok  	github.com/esalaine/envoy-go/test/helpers/jwksbackend	1.014s
ok  	github.com/esalaine/envoy-go/test/helpers/oauthbackend	1.013s
ok  	github.com/esalaine/envoy-go/test/helpers/ratelimitgrpc	1.038s
```

Zero FAILs across all packages.

**Gate 4: PASS (all packages green, race detector clean)**

---

### Gate 5: Full 50-dir differential suite (`-count=1`)

First run (full suite together):

```
$ go test ./test/differential/ -run TestDifferential -v -count=1 -timeout 3600s 2>&1 | tail -60
    --- PASS: TestDifferential/0000-tcp-echo (2.07s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.74s)
    --- PASS: TestDifferential/0002-tls-tcp (1.64s)
    --- PASS: TestDifferential/0003-http11-routing (1.65s)
    --- PASS: TestDifferential/0004-h2-routing (2.27s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.46s)
    --- PASS: TestDifferential/0006-access-log (11.25s)
    --- PASS: TestDifferential/0007a-cors (1.91s)
    --- PASS: TestDifferential/0007b-iteration-probe (1.30s)
    --- PASS: TestDifferential/0008-listener-chain-match (3.43s)
    --- PASS: TestDifferential/0009-admin-config-dump (2.40s)
    --- PASS: TestDifferential/0010-graceful-drain (9.90s)
    --- PASS: TestDifferential/0011-http-fault (2.45s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.85s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.55s)
    --- PASS: TestDifferential/0014-http-csrf (1.84s)
    --- PASS: TestDifferential/0015-http-buffer (1.90s)
    --- PASS: TestDifferential/0016-http-compressor (1.79s)
    --- PASS: TestDifferential/0017-http-bandwidth-limit (6.41s)
    --- PASS: TestDifferential/0018-http-rbac (1.80s)
    --- PASS: TestDifferential/0019-http-jwt-authn (1.89s)
    --- PASS: TestDifferential/0020-http-ext-authz-http (1.74s)
    --- PASS: TestDifferential/0021-http-ext-authz-grpc (1.87s)
    --- PASS: TestDifferential/0022-http-ext-proc-grpc (1.86s)
    --- PASS: TestDifferential/0023-http-ext-proc-body (1.83s)
    --- PASS: TestDifferential/0024-http-oauth2 (1.00s)
    --- FAIL: TestDifferential/0025-http-adaptive-concurrency (1.02s)
    --- PASS: TestDifferential/0026-http-lua-headers-bridge (1.53s)
    --- PASS: TestDifferential/0027-http-lua-full-bridge (2.46s)
    --- FAIL: TestDifferential/0028-http-lua-multi-script-and-per-route (2.10s)
    --- PASS: TestDifferential/0029-http-lua-source-codes-boot-reject (1.59s)
    --- PASS: TestDifferential/0030-http-admission-control (1.73s)
    --- PASS: TestDifferential/0031-http-admission-control-boot-reject (1.57s)
    --- PASS: TestDifferential/0032-http-ratelimit (1.74s)
    --- PASS: TestDifferential/0033-http-ratelimit-boot-reject (1.60s)
    --- PASS: TestDifferential/0034-http-wasm-headers-bridge (2.38s)
    --- PASS: TestDifferential/0035-http-wasm-boot-reject (1.63s)
    --- PASS: TestDifferential/0036-http-wasm-body-and-advanced (34.07s)
    --- PASS: TestDifferential/0037-http-wasm-body-and-advanced-boot-reject (1.73s)
    --- PASS: TestDifferential/0038-http-wasm-perroute-and-multi-plugin (3.64s)
    --- PASS: TestDifferential/0039-http-wasm-perroute-boot-reject (1.81s)
    --- PASS: TestDifferential/0040-network-echo (3.76s)
    --- PASS: TestDifferential/0041-network-direct-response (1.65s)
    --- PASS: TestDifferential/0042-network-direct-response-boot-reject (1.50s)
    --- PASS: TestDifferential/0043-network-rbac (5.77s)
    --- PASS: TestDifferential/0044-network-rbac-boot-reject (1.60s)
    --- PASS: TestDifferential/0045-sni-cluster (1.71s)
    --- PASS: TestDifferential/0046-zookeeper-requests (4.27s)
    --- PASS: TestDifferential/0047-zookeeper-boot-reject (1.57s)
    --- PASS: TestDifferential/0048-zookeeper-responses (3.60s)
FAIL
FAIL	github.com/esalaine/envoy-go/test/differential	158.953s
```

0025 and 0028 showed FAIL in the combined run. Both are TOCTOU port-bind flakes (unrelated to phase 28.2 — the 0025/0028 HTTP fixtures have no overlap with the zookeeper network filter changes). Isolated re-runs per the flake protocol:

**0025 re-run in isolation (`-count=1`):**
```
$ go test ./test/differential/ -run TestDifferential/0025-http-adaptive-concurrency -v -count=1 -timeout 600s | tail -5
=== RUN   TestDifferential/0025-http-adaptive-concurrency
--- PASS: TestDifferential (5.09s)
    --- PASS: TestDifferential/0025-http-adaptive-concurrency (5.09s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	5.177s
```

**0028 re-run in isolation (`-count=1`):**
```
$ go test ./test/differential/ -run TestDifferential/0028-http-lua-multi-script-and-per-route -v -count=1 -timeout 600s | tail -5
=== RUN   TestDifferential/0028-http-lua-multi-script-and-per-route
--- PASS: TestDifferential (2.45s)
    --- PASS: TestDifferential/0028-http-lua-multi-script-and-per-route (2.45s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.542s
```

Both GREEN in isolation. TOCTOU flakes recorded per protocol — not regressions.

Summary: **50/50 PASS** (48 green in combined run + 0025 + 0028 each green in isolation re-run).

**Gate 5: PASS (50/50; two TOCTOU flakes isolated-GREEN, recorded)**

---

### Gate 6a: h2spec conformance

```
$ go test ./test/conformance/h2spec/ -run TestH2Spec -v -count=1 -timeout 900s 2>&1 | tail -15
    h2spec_test.go:187:   [PASS] 5.4.1. Connection Error Handling: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.5. Extending HTTP/2: 2/2 passed
    h2spec_test.go:187:   [PASS] 7. Error Codes: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1. HTTP Request/Response Exchange: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2. HTTP Header Fields: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2.1. Pseudo-Header Fields: 4/4 passed
    h2spec_test.go:187:   [PASS] 8.1.2.2. Connection-Specific Header Fields: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
    h2spec_test.go:187:   [PASS] 8.1.2.6. Malformed Requests and Responses: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.2. Server Push: 1/1 passed
2026/06/02 19:03:59 🐳 Terminating container: 0a257fbf5df2
2026/06/02 19:03:59 🚫 Container terminated: 0a257fbf5df2
--- PASS: TestH2Spec (2.59s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.677s
```

**Gate 6a: PASS (h2spec 53/53)**

---

### Gate 6b: proxy-wasm conformance

```
$ go test ./test/conformance/proxy-wasm/ -run TestProxyWasmConformance -v -count=1 -timeout 900s 2>&1 | tail -20
--- PASS: TestProxyWasmConformance (0.25s)
    --- PASS: TestProxyWasmConformance/exports (0.03s)
    --- PASS: TestProxyWasmConformance/security (0.05s)
        --- PASS: TestProxyWasmConformance/security/allowed (0.02s)
        --- PASS: TestProxyWasmConformance/security/denied (0.03s)
    --- PASS: TestProxyWasmConformance/runtime (0.02s)
    --- PASS: TestProxyWasmConformance/wasm_vm (0.02s)
    --- PASS: TestProxyWasmConformance/bytecode_util (0.00s)
        --- PASS: TestProxyWasmConformance/bytecode_util/v0_2_1_compiles (0.00s)
        --- PASS: TestProxyWasmConformance/bytecode_util/wrong_abi_rejected (0.02s)
        --- PASS: TestProxyWasmConformance/bytecode_util/missing_abi_rejected (0.00s)
    --- PASS: TestProxyWasmConformance/logging (0.02s)
    --- PASS: TestProxyWasmConformance/stop_iteration (0.05s)
        --- PASS: TestProxyWasmConformance/stop_iteration/pause (0.02s)
        --- PASS: TestProxyWasmConformance/stop_iteration/continue (0.02s)
    --- PASS: TestProxyWasmConformance/shared_data (0.02s)
    --- PASS: TestProxyWasmConformance/pairs_util (0.02s)
    --- PASS: TestProxyWasmConformance/endianness (0.02s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/proxy-wasm	0.255s
```

All 10 families PASS.

**Gate 6b: PASS (proxy-wasm 10/10)**

---

### Six-gate summary

| Gate | Command | Result |
|------|---------|--------|
| 1 | `go build ./...` | PASS (clean) |
| 2 | `go vet ./...` | PASS (clean) |
| 3 | `golangci-lint run` | PASS (clean) |
| 4 | `go test ./... -race -short -count=1` | PASS (all packages green) |
| 5 | differential 50-dir `-count=1` | PASS (50/50; two TOCTOU flakes isolated-GREEN, recorded) |
| 6a | h2spec | PASS (53/53) |
| 6b | proxy-wasm | PASS (10/10) |

**Task 11 DONE. ROADMAP ATOMIC rollup + STATE.md + next-prompt.txt updates follow in the same commit.**
