# Phase 05.2 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1 PROGRESS.md structure.

## Preamble — execution preconditions

none — all 12 preconditions per PLAN §"Execution preconditions" satisfied at cold-start. Branch `phase/05.2-upstream-h2-impl` cut from master `4c6b6bb` (the PLAN.md commit). Docker available (Engine 28.4.0 client / 28.1.1 server). Go 1.26.2 (≥ 1.23 floor). golangci-lint v1.64.8 (ADR-0009 pin). `go test ./...` green across all 26 reported packages (0 FAIL). go-control-plane envoy at v1.32.4 (ADR-0013). DECISIONS.md ADR tail at `## ADR-0054:` (next-free 0055, matches PLAN's 0055..0058 assignment). SPEC at `dacf4b7` (matches PLAN authorship pin). Phase-05.1 REVIEW close `d69446a` present in HEAD; CONFORMANCE_PINS.md `## Refresh procedure` section present at line 7 (I-4 follow-up close). golang.org/x/net at v0.34.0 (intact 05.1 direct pin). h2 sub-package contains the expected 18 files (no `client.go` — Task 7 deliverable). BEHAVIOR_CONTRACT.md `## HTTP/2` SCAFFOLD present (1 match).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** 9bda8f9 (SHA-fill: see next commit)
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions"; phase-05.1 close + follow-up batch confirmed present in HEAD; SPEC at dacf4b7; ADR tail at 0054 (next-free 0055); client.go absent (will land at Task 7).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/05.2-upstream-h2-impl

$ git log -1 --format=%H
4c6b6bb67aff12b93642ef70c24ee8f0d14d0d12

$ docker version
Client: Docker Engine - Community
 Version:           28.4.0
Server: Docker Desktop 4.41.2 (191736)
 Engine:
  Version:          28.1.1

$ go version
go version go1.26.2 linux/amd64

$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)

$ go test ./...    # last 30 lines (full output: 26 lines, 0 FAIL)
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	1.970s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	0.039s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.008s
ok  	github.com/esalaine/envoy-go/internal/cluster	0.009s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.011s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.261s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.009s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	0.011s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	0.017s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.233s
ok  	github.com/esalaine/envoy-go/test/differential	6.959s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	0.003s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/helpers	0.006s

$ go list -m github.com/envoyproxy/go-control-plane/envoy
github.com/envoyproxy/go-control-plane/envoy v1.32.4

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0054: ADR-0046 prose correction — root-http2 import file list

$ git log -1 --format=%H -- docs/envoy-go/phases/05.2-upstream-h2/SPEC.md
dacf4b726f02c1fb81b8fbfca6bc714d9eaad54b

$ git log --oneline -- docs/envoy-go/phases/05.1-downstream-h2/REVIEW.md | head -5
d69446a phase 05.1: REVIEW.md — APPROVED WITH FOLLOW-UPS

$ grep -nE '## Refresh procedure' docs/envoy-go/CONFORMANCE_PINS.md
7:## Refresh procedure

$ go list -m golang.org/x/net
golang.org/x/net v0.34.0

$ ls internal/filter/hcm/h2/client.go
ls: cannot access 'internal/filter/hcm/h2/client.go': No such file or directory

$ grep -cE "^## HTTP/2$" docs/envoy-go/BEHAVIOR_CONTRACT.md
1

$ ls internal/filter/hcm/h2/
conn.go
conn_test.go
doc.go
errors.go
errors_test.go
flow.go
flow_test.go
framer.go
framer_test.go
fuzz_test.go
hpack.go
hpack_test.go
preface.go
preface_test.go
settings.go
settings_test.go
stream.go
stream_test.go
```

## Task 2 — ADR-0055 prerequisites — `window.reserveBlocking` collapse (M-3) + `translateFramerErr` helper extraction (M-5)

**Commits:** 964df19
**Files changed:**
- `internal/filter/hcm/h2/flow.go` — replaced `reserve` + `waitFor` pair with single atomic `reserveBlocking(ctx, max) (int32, error)`; both deleted methods removed.
- `internal/filter/hcm/h2/flow_test.go` — added `TestWindow_ReserveBlocking_AtomicityUnderConcurrency` (20 consumers × 50 bytes vs window=100 + 10×100 replenisher; asserts `taken <= 1100`); rewrote 4 prior tests to call `reserveBlocking`.
- `internal/filter/hcm/h2/framer.go` — extracted `translateFramerErr(err) error` helper; `readFrameCtx` and `tryReadFrame` now both call it (was identical inline blocks before).
- `internal/filter/hcm/h2/framer_test.go` — added `TestTranslateFramerErr` covering nil / `http2.ConnectionError` / `http2.StreamError` / `http2.ErrFrameTooLarge` branches.
- `internal/filter/hcm/h2/conn.go` — `writeData` consumer site updated to call `reserveBlocking`; the dead `if taken <= 0` recovery branch (M-3 finding) is GONE.

**Notes:** TWO red→green TDD cycles, both observed:
1. `TestWindow_ReserveBlocking_AtomicityUnderConcurrency` failed with `w.reserveBlocking undefined` → implemented `reserveBlocking` per PLAN.md:388-406 → PASS under `-race`.
2. `TestTranslateFramerErr` failed with `undefined: translateFramerErr` (4 occurrences) → implemented per PLAN.md:480-496 → PASS.

Confirmed pre-extraction by grep that `readFrameCtx` and `tryReadFrame` had IDENTICAL translation blocks (3 branches × 2 sites = 6 cases collapsed to 1 helper). The `reserveBlocking` collapse (M-3) is a prerequisite for I-1/I-2 race-freedom in Task 3; the `translateFramerErr` extraction (M-5) is a prerequisite for the third call site in `client.go` at Task 7.

**Outputs:**
```
$ go test -race ./internal/filter/hcm/h2/ -run TestWindow_ReserveBlocking_AtomicityUnderConcurrency -v   # before flow.go change
# github.com/esalaine/envoy-go/internal/filter/hcm/h2 [github.com/esalaine/envoy-go/internal/filter/hcm/h2.test]
internal/filter/hcm/h2/flow_test.go:76:18: w.reserveBlocking undefined (type *window has no field or method reserveBlocking)
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm/h2 [build failed]
FAIL

$ go test -race ./internal/filter/hcm/h2/ -run TestWindow_ -v   # after flow.go + conn.go change
=== RUN   TestWindow_ReserveAndReplenish
--- PASS: TestWindow_ReserveAndReplenish (0.00s)
=== RUN   TestWindow_BlockingWaitFor
--- PASS: TestWindow_BlockingWaitFor (0.02s)
=== RUN   TestWindow_CtxCancelDuringWait
--- PASS: TestWindow_CtxCancelDuringWait (0.02s)
=== RUN   TestWindow_ReserveBlocking_AtomicityUnderConcurrency
--- PASS: TestWindow_ReserveBlocking_AtomicityUnderConcurrency (2.00s)
=== RUN   TestWindow_TinyWindowStressDelivery
--- PASS: TestWindow_TinyWindowStressDelivery (0.10s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.154s

$ go test ./internal/filter/hcm/h2/ -run TestTranslateFramerErr -v   # before framer.go change
# github.com/esalaine/envoy-go/internal/filter/hcm/h2 [github.com/esalaine/envoy-go/internal/filter/hcm/h2.test]
internal/filter/hcm/h2/framer_test.go:271:12: undefined: translateFramerErr
internal/filter/hcm/h2/framer_test.go:277:10: undefined: translateFramerErr
internal/filter/hcm/h2/framer_test.go:292:10: undefined: translateFramerErr
internal/filter/hcm/h2/framer_test.go:306:10: undefined: translateFramerErr
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm/h2 [build failed]
FAIL

$ go test ./internal/filter/hcm/h2/ -run TestTranslateFramerErr -v   # after framer.go change
=== RUN   TestTranslateFramerErr
--- PASS: TestTranslateFramerErr (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.002s

$ go test -race ./internal/filter/hcm/h2/ -v   # last 30 lines
=== RUN   TestServerStream_StateTransitions_HeadersThenData
--- PASS: TestServerStream_StateTransitions_HeadersThenData (0.00s)
=== RUN   TestServerStream_StateTransitions_RSTStream
--- PASS: TestServerStream_StateTransitions_RSTStream (0.00s)
=== RUN   TestServerStream_RecvWindowUpdate_ReplenishesSendWindow
--- PASS: TestServerStream_RecvWindowUpdate_ReplenishesSendWindow (0.00s)
=== RUN   TestServerStream_RecvWindowUpdate_ZeroDeltaIsProtocolError
--- PASS: TestServerStream_RecvWindowUpdate_ZeroDeltaIsProtocolError (0.00s)
=== RUN   TestServerStream_Dispatch_DirectResponse_WritesHeadersAndData
--- PASS: TestServerStream_Dispatch_DirectResponse_WritesHeadersAndData (0.00s)
=== RUN   TestServerStream_Dispatch_RouterAction_EmitsRSTStreamInternalError
--- PASS: TestServerStream_Dispatch_RouterAction_EmitsRSTStreamInternalError (0.00s)
=== RUN   TestServerStream_Dispatch_404Adapter_WritesHeadersAndData
--- PASS: TestServerStream_Dispatch_404Adapter_WritesHeadersAndData (0.00s)
=== RUN   FuzzFrameStream
=== RUN   FuzzFrameStream/seed#0
=== RUN   FuzzFrameStream/seed#1
=== RUN   FuzzFrameStream/seed#2
--- PASS: FuzzFrameStream (0.00s)
    --- PASS: FuzzFrameStream/seed#0 (0.00s)
    --- PASS: FuzzFrameStream/seed#1 (0.00s)
    --- PASS: FuzzFrameStream/seed#2 (0.00s)
=== RUN   FuzzHPACKDecode
=== RUN   FuzzHPACKDecode/seed#0
=== RUN   FuzzHPACKDecode/seed#1
--- PASS: FuzzHPACKDecode (0.00s)
    --- PASS: FuzzHPACKDecode/seed#0 (0.00s)
    --- PASS: FuzzHPACKDecode/seed#1 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.270s

$ golangci-lint run ./internal/filter/hcm/h2/
$ echo $?
0

$ go test -race ./...   # broader sanity check, all packages green
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	(...)
ok  	github.com/esalaine/envoy-go/internal/admin	1.057s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.033s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.026s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.031s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.272s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.024s
ok  	github.com/esalaine/envoy-go/internal/listener	1.029s
ok  	github.com/esalaine/envoy-go/internal/tls	1.076s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.141s
ok  	github.com/esalaine/envoy-go/test/differential	7.607s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.009s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.020s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.008s
ok  	github.com/esalaine/envoy-go/test/helpers	1.025s
```

## Task 3 — ADR-0055 outbound DATA chunking — `MaxFrameSize` cap (I-1) + per-stream send-window (I-2)

**Commits:** d3de1f8
**Files changed:**
- `internal/filter/hcm/h2/conn.go` — rewrote `(*ServerConn).writeData` to apply the chunk = min(connSendWindow, streamSendWindow, peer.MaxFrameSize) algorithm: per-stream reservation first (smaller bound first reduces head-of-line blocking on the conn-level window), conn-level reservation second for the streamTaken amount, replenish per-stream over-reservation on conn-level under-reservation or ctx-cancel rollback. MaxFrameSize=0 default → 16384 per RFC 9113 §6.5.2. Use `translateFramerErr` (extracted by Task 2) on `framer.WriteData` errors. Also fixed `onHeaders` stream construction: per-stream **send** window initial size now reads `s.clientS.InitialWindowSize` (peer-announced — what governs how much WE can send) with a 65535 default; recv window initial size unchanged (still `s.settings.InitialWindowSize`, our own announced value).
- `internal/filter/hcm/h2/conn_test.go` — added `TestServerConn_WriteData_RespectsMaxFrameSize` (32768-byte body, peer MaxFrameSize=16384 → ≥ 2 DATA frames, none > 16384) and `TestServerConn_WriteData_RespectsPerStreamSendWindow` (100-byte body, peer InitialWindowSize=16, drip WINDOW_UPDATE(1, 16) every 5ms → ≥ 7 DATA frames, none > 16). LoC delta: +322/-14.

**Notes:** TWO red→green TDD cycles, both observed:
1. `TestServerConn_WriteData_RespectsMaxFrameSize` first failed: server emitted a single 32768-byte DATA frame (exceeded peer MaxFrameSize=16384). After implementing the MaxFrameSize cap in `writeData`, the test passes (≥ 2 DATA frames, each ≤ 16384).
2. `TestServerConn_WriteData_RespectsPerStreamSendWindow` first failed: server emitted a single 100-byte DATA frame (exceeded peer per-stream initial window=16). Root cause was twofold: (a) `writeData` only honored the conn-level window, never the per-stream window; (b) `onHeaders` initialized the per-stream send-window from `s.settings.InitialWindowSize` (the SERVER's own value, 65535) rather than `s.clientS.InitialWindowSize` (the PEER's announced value). Both fixes landed; the test passes.

The replenish discipline on conn-level under-reservation matches PLAN's pitfall #1: if `connTaken < streamTaken`, replenish exactly `streamTaken - connTaken`; if conn-level reservation errors, replenish exactly `streamTaken`. Verified manually by reading the new `writeData` against PLAN.md:585-632 reference snippet.

**Outputs:**
```
$ go test ./internal/filter/hcm/h2/ -run TestServerConn_WriteData_Respects -v   # before conn.go change
=== RUN   TestServerConn_WriteData_RespectsMaxFrameSize
    conn_test.go:925: DATA frame #1 payload = 32768 bytes, exceeds peer MaxFrameSize=16384
    conn_test.go:935: got 1 DATA frames, want >= 2 (32768 bytes / MaxFrameSize=16384)
--- FAIL: TestServerConn_WriteData_RespectsMaxFrameSize (0.00s)
=== RUN   TestServerConn_WriteData_RespectsPerStreamSendWindow
    conn_test.go:1055: DATA frame #1 payload = 100 bytes, exceeds peer per-stream initial window=16
    conn_test.go:1068: got 1 DATA frames, want >= 7 (ceil(100/16) = 7)
--- FAIL: TestServerConn_WriteData_RespectsPerStreamSendWindow (0.00s)
FAIL
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.005s
FAIL

$ go test ./internal/filter/hcm/h2/ -run TestServerConn_WriteData_Respects -v   # after conn.go change
=== RUN   TestServerConn_WriteData_RespectsMaxFrameSize
--- PASS: TestServerConn_WriteData_RespectsMaxFrameSize (0.00s)
=== RUN   TestServerConn_WriteData_RespectsPerStreamSendWindow
--- PASS: TestServerConn_WriteData_RespectsPerStreamSendWindow (0.03s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.035s

$ go test -race ./internal/filter/hcm/h2/ -v   # last 20 lines (full output: every existing test + 2 new tests = all PASS)
=== RUN   TestServerConn_WriteData_RespectsMaxFrameSize
--- PASS: TestServerConn_WriteData_RespectsMaxFrameSize (0.00s)
=== RUN   TestServerConn_WriteData_RespectsPerStreamSendWindow
--- PASS: TestServerConn_WriteData_RespectsPerStreamSendWindow (0.03s)
=== RUN   FuzzFrameStream
=== RUN   FuzzFrameStream/seed#0
=== RUN   FuzzFrameStream/seed#1
=== RUN   FuzzFrameStream/seed#2
--- PASS: FuzzFrameStream (0.00s)
    --- PASS: FuzzFrameStream/seed#0 (0.00s)
    --- PASS: FuzzFrameStream/seed#1 (0.00s)
    --- PASS: FuzzFrameStream/seed#2 (0.00s)
=== RUN   FuzzHPACKDecode
=== RUN   FuzzHPACKDecode/seed#0
=== RUN   FuzzHPACKDecode/seed#1
--- PASS: FuzzHPACKDecode (0.00s)
    --- PASS: FuzzHPACKDecode/seed#0 (0.00s)
    --- PASS: FuzzHPACKDecode/seed#1 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.316s

$ go test ./test/conformance/h2spec/ -v   # h2spec section roll-up (ADR-0051 pin)
        Finished in 0.5493 seconds
        53 tests, 53 passed, 0 skipped, 0 failed

    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
--- PASS: TestH2Spec (2.24s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.321s

$ golangci-lint run ./internal/filter/hcm/h2/
$ echo $?
0

$ go test -race ./...   # broader sanity check, all packages green
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	2.687s
ok  	github.com/esalaine/envoy-go/internal/admin	(cached)
ok  	github.com/esalaine/envoy-go/internal/bootstrap	(cached)
ok  	github.com/esalaine/envoy-go/internal/cluster	(cached)
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.031s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.317s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	(cached)
ok  	github.com/esalaine/envoy-go/internal/listener	1.025s
ok  	github.com/esalaine/envoy-go/internal/tls	(cached)
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	(cached)
ok  	github.com/esalaine/envoy-go/test/differential	(cached)
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	(cached)
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	(cached)
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	(cached)
ok  	github.com/esalaine/envoy-go/test/helpers	(cached)
```

## Task 4 — ADR-0055 receive-side flow control (I-3) + WINDOW_UPDATE delta-overflow bounds-check (M-9)

**Commits:** b951c38 (SHA-fill: see next commit)
**Files changed:**
- `internal/filter/hcm/h2/conn.go` — added `safeAddInt32(a, b int32) (int32, bool)` helper and `recvWindowUpdateThreshold = 32768` constant; added `recvDebitSinceLastUpdate int32` field to `ServerConn`; rewrote `onData` to debit conn-level + per-stream recv windows (`recvW.replenish(-dataLen)` — flow.go's `replenish` accepts negative deltas, no separate debit method needed) and emit `WriteWindowUpdate(0, n)` / `WriteWindowUpdate(streamID, n)` once the running counter crosses the half-window threshold (32768 = 65535/2). Per-stream WINDOW_UPDATE is suppressed when END_STREAM accompanies the DATA (no further DATA can arrive on that stream). `s.fr.WriteWindowUpdate` writes are serialised via `s.mu` against other framer writers (encodeAndWriteHeaders / writeData / RSTStream / GoAway). `onWindowUpdate` now bounds-checks the resulting send window against int32 overflow per RFC 9113 §6.9.1: conn-level overflow → `connError(ErrFlowControlError)` → GOAWAY; stream-level overflow handled inside `recvWindowUpdate`. The existing `delta == 0` PROTOCOL_ERROR path is preserved.
- `internal/filter/hcm/h2/stream.go` — added `recvDebitSinceLastUpdate int32` field to `serverStream`; updated `recvWindowUpdate` to bounds-check via `safeAddInt32(cur, delta)` → `streamError(ErrFlowControlError)` (RST_STREAM) on overflow.
- `internal/filter/hcm/h2/conn_test.go` — added `TestServerConn_ReceiveSide_FlowControl_LargeInboundBody` (in-process peer pushes a 100KB POST body in 8KB DATA frames, asserts the dispatcher sees the full 100KB — without I-3 the peer stalls at 65535) and `TestServerConn_WindowUpdate_OverflowBoundsCheck` (two subtests: stream-level overflow → RST_STREAM(FLOW_CONTROL_ERROR); conn-level overflow → GOAWAY(FLOW_CONTROL_ERROR)). Added `bodyCaptureDispatcher` / `bodyCaptureAction` helpers.
- `internal/filter/hcm/h2/stream_test.go` — added `TestServerStream_RecvWindowUpdate_OverflowIsFlowControlError` (stream-level overflow unit test) and `TestSafeAddInt32` (helper coverage: zero, normal, MaxInt32 boundary, MinInt32 boundary).

LoC delta: +517/-0 across the four files (`+79` conn.go impl, `+12` stream.go impl, `+372` conn_test.go, `+54` stream_test.go).

**Notes:** TWO red→green TDD cycles, both observed:

1. `TestServerConn_ReceiveSide_FlowControl_LargeInboundBody` first failed with `timed out waiting for WINDOW_UPDATE; sent=65535/102400 connAvail=0 streamAvail=0` — server never replenished the receive window, the in-process peer stalled at the 65535 initial-window boundary. After implementing the recv-side debit + half-window WINDOW_UPDATE emission in `onData`, the test passes (full 100KB body delivered to the dispatcher in ~50ms).
2. `TestServerConn_WindowUpdate_OverflowBoundsCheck` first failed both subtests: stream-level subtest reported `expected RST_STREAM(FLOW_CONTROL_ERROR) for stream 1 on send-window overflow`; conn-level subtest reported `expected GOAWAY(FLOW_CONTROL_ERROR) on conn-level send-window overflow`. After implementing the `safeAddInt32`-based bounds check in `onWindowUpdate` (conn) and `recvWindowUpdate` (stream), both subtests pass.

`flow.go`'s `replenish(delta int32)` accepts negative deltas (verified by reading `flow.go` lines 30-38 — no sign check, just `w.n += delta`); the debit path uses `s.recvW.replenish(-dataLen)` directly. The recv counters (`recvDebitSinceLastUpdate` on both `ServerConn` and `serverStream`) are mutated only from the frame-loop goroutine in `onData`, before the per-stream dispatch goroutine is launched, so no separate mutex is needed (the conn's frame-loop is single-threaded). The `s.fr.WriteWindowUpdate` writes are serialised via `s.mu` (same discipline as `encodeAndWriteHeaders` / `writeData`).

A test-fixture pre-fix surfaced during stabilisation: the new I-3 test originally started its frame-reader goroutine before completing the SETTINGS handshake, which raced with the DATA-send loop and produced an intermittent "short write". Restructured to drain the SETTINGS handshake synchronously before launching the reader; 5/5 fresh runs PASS.

**Outputs:**
```
$ go test ./internal/filter/hcm/h2/ -run TestServerConn_ReceiveSide_FlowControl_LargeInboundBody -timeout 15s -v   # before conn.go change
=== RUN   TestServerConn_ReceiveSide_FlowControl_LargeInboundBody
    conn_test.go:1315: timed out waiting for WINDOW_UPDATE; sent=65535/102400 connAvail=0 streamAvail=0
--- FAIL: TestServerConn_ReceiveSide_FlowControl_LargeInboundBody (3.00s)
FAIL
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.004s
FAIL

$ go test ./internal/filter/hcm/h2/ -run TestServerConn_WindowUpdate_OverflowBoundsCheck -v   # before conn.go/stream.go change
=== RUN   TestServerConn_WindowUpdate_OverflowBoundsCheck
=== RUN   TestServerConn_WindowUpdate_OverflowBoundsCheck/stream-level_overflow_→_RST_STREAM(FLOW_CONTROL_ERROR)
    conn_test.go:1452: expected RST_STREAM(FLOW_CONTROL_ERROR) for stream 1 on send-window overflow
=== RUN   TestServerConn_WindowUpdate_OverflowBoundsCheck/connection-level_overflow_→_GOAWAY(FLOW_CONTROL_ERROR)
    conn_test.go:1518: expected GOAWAY(FLOW_CONTROL_ERROR) on conn-level send-window overflow
--- FAIL: TestServerConn_WindowUpdate_OverflowBoundsCheck (6.00s)
    --- FAIL: TestServerConn_WindowUpdate_OverflowBoundsCheck/stream-level_overflow_→_RST_STREAM(FLOW_CONTROL_ERROR) (3.00s)
    --- FAIL: TestServerConn_WindowUpdate_OverflowBoundsCheck/connection-level_overflow_→_GOAWAY(FLOW_CONTROL_ERROR) (3.00s)
FAIL
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm/h2	6.005s
FAIL

$ go test ./internal/filter/hcm/h2/ -run "TestServerConn_(ReceiveSide_FlowControl_LargeInboundBody|WindowUpdate_OverflowBoundsCheck)|TestServerStream_RecvWindowUpdate_OverflowIsFlowControlError|TestSafeAddInt32" -v -count=1   # after conn.go/stream.go change
=== RUN   TestServerConn_ReceiveSide_FlowControl_LargeInboundBody
--- PASS: TestServerConn_ReceiveSide_FlowControl_LargeInboundBody (0.05s)
=== RUN   TestServerConn_WindowUpdate_OverflowBoundsCheck
=== RUN   TestServerConn_WindowUpdate_OverflowBoundsCheck/stream-level_overflow_→_RST_STREAM(FLOW_CONTROL_ERROR)
=== RUN   TestServerConn_WindowUpdate_OverflowBoundsCheck/connection-level_overflow_→_GOAWAY(FLOW_CONTROL_ERROR)
--- PASS: TestServerConn_WindowUpdate_OverflowBoundsCheck (0.00s)
    --- PASS: TestServerConn_WindowUpdate_OverflowBoundsCheck/stream-level_overflow_→_RST_STREAM(FLOW_CONTROL_ERROR) (0.00s)
    --- PASS: TestServerConn_WindowUpdate_OverflowBoundsCheck/connection-level_overflow_→_GOAWAY(FLOW_CONTROL_ERROR) (0.00s)
=== RUN   TestServerStream_RecvWindowUpdate_OverflowIsFlowControlError
--- PASS: TestServerStream_RecvWindowUpdate_OverflowIsFlowControlError (0.00s)
=== RUN   TestSafeAddInt32
--- PASS: TestSafeAddInt32 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.055s

$ go test -race ./internal/filter/hcm/h2/ -timeout 60s -v -count=1   # last 23 lines
--- PASS: TestServerStream_RecvWindowUpdate_OverflowIsFlowControlError (0.00s)
=== RUN   TestSafeAddInt32
--- PASS: TestSafeAddInt32 (0.00s)
=== RUN   TestServerStream_Dispatch_DirectResponse_WritesHeadersAndData
--- PASS: TestServerStream_Dispatch_DirectResponse_WritesHeadersAndData (0.00s)
=== RUN   TestServerStream_Dispatch_RouterAction_EmitsRSTStreamInternalError
--- PASS: TestServerStream_Dispatch_RouterAction_EmitsRSTStreamInternalError (0.00s)
=== RUN   TestServerStream_Dispatch_404Adapter_WritesHeadersAndData
--- PASS: TestServerStream_Dispatch_404Adapter_WritesHeadersAndData (0.00s)
=== RUN   FuzzFrameStream
=== RUN   FuzzFrameStream/seed#0
=== RUN   FuzzFrameStream/seed#1
=== RUN   FuzzFrameStream/seed#2
--- PASS: FuzzFrameStream (0.00s)
    --- PASS: FuzzFrameStream/seed#0 (0.00s)
    --- PASS: FuzzFrameStream/seed#1 (0.00s)
    --- PASS: FuzzFrameStream/seed#2 (0.00s)
=== RUN   FuzzHPACKDecode
=== RUN   FuzzHPACKDecode/seed#0
=== RUN   FuzzHPACKDecode/seed#1
--- PASS: FuzzHPACKDecode (0.00s)
    --- PASS: FuzzHPACKDecode/seed#0 (0.00s)
    --- PASS: FuzzHPACKDecode/seed#1 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.367s

$ go test ./test/conformance/h2spec/ -v -count=1   # h2spec section roll-up (ADR-0051 pin)
        53 tests, 53 passed, 0 skipped, 0 failed
    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
--- PASS: TestH2Spec (2.18s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.260s

$ golangci-lint run ./internal/filter/hcm/h2/
$ echo $?
0

$ go test -race ./... -count=1   # broader sanity check, all packages green
ok  	github.com/esalaine/envoy-go/internal/admin	1.060s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.041s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.033s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.041s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.379s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.033s
ok  	github.com/esalaine/envoy-go/internal/listener	1.041s
ok  	github.com/esalaine/envoy-go/internal/tls	1.087s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.221s
ok  	github.com/esalaine/envoy-go/test/differential	7.936s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.011s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.014s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.012s
ok  	github.com/esalaine/envoy-go/test/helpers	1.023s
```

## Task 5 — ADR-0055 `recvData` state-before-append (M-11) + ADR-0055 lands in DECISIONS.md

**Commits:** bef7a1e (SHA-fill: see next commit)
**Files changed:**
- `internal/filter/hcm/h2/stream.go` — `recvData` reordered to validate stream state BEFORE appending to `s.reqBody` per ADR-0055 M-11. The half-closed-remote and closed branches now return `streamError(ErrStreamClosed, ...)` BEFORE the body buffer is touched; the `streamOpen` and `streamHalfClosedLocal` branches fall through and append as before. The post-append state-transition switch is preserved (open → halfClosedRemote on END_STREAM; halfClosedLocal → closed on END_STREAM). The doc comment is updated to call out the state-first ordering.
- `internal/filter/hcm/h2/stream_test.go` — added `TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream`. Drives the stream to halfClosedRemote via `recvHeaders(minHeaders(), true)`, captures `reqBody.Len()` pre-call, calls `recvData([]byte("late data"), false)`, asserts (a) the returned error is `*Error` with `Code == ErrStreamClosed` AND (b) `reqBody.Len()` is unchanged from the pre-call snapshot.
- `docs/envoy-go/DECISIONS.md` — appended ADR-0055 at the file tail (next-free 0055; tail before append was ADR-0054, verified by `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1`). The ADR enumerates the seven fixes individually (I-1, I-2, I-3, M-3, M-5, M-9, M-11) with file:line + commit SHA cross-references so a future supersession can target precisely. Lands-in-task: this Task (Task 5).

LoC delta: +84/-11 across stream.go (+13 / -10 — the reorder is a small net add for the state-first switch + doc comment update) and stream_test.go (+44 / 0 — new test); plus +71 in DECISIONS.md (ADR-0055 prose).

**Notes:** TDD red→green observed:

1. New test failed first with `reqBody.Len() grew from 0 to 9 on a closed stream; want unchanged` — the pre-fix `recvData` appended `[]byte("late data")` (9 bytes) to `s.reqBody` BEFORE the state switch evaluated `streamHalfClosedRemote` and returned the error.
2. After the reorder (state switch evaluates first, append happens only on `streamOpen`/`streamHalfClosedLocal`), the test passes; `reqBody.Len()` stays at 0 and the returned error is `*Error{Code: ErrStreamClosed}`.

The reorder does NOT change observable wire behaviour for valid streams: open and halfClosedLocal streams still append + transition exactly as before. h2spec at the ADR-0051 pin remains 53/53 PASS. The full h2 race-detector run (60s timeout) is green; `go test -race ./...` across the repo is green.

ADR-0055 is the closing artifact of the seven-fix sequence (Tasks 2-5). It lands at the DECISIONS.md tail (D-3.5 append-only); next-free was 0055 before the append, and the new tail is 0055.

**Outputs:**
```
$ go test ./internal/filter/hcm/h2/ -run TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream -v -count=1   # before stream.go change
=== RUN   TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream
    stream_test.go:361: reqBody.Len() grew from 0 to 9 on a closed stream; want unchanged
--- FAIL: TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream (0.00s)
FAIL
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.002s
FAIL

$ go test ./internal/filter/hcm/h2/ -run TestServerStream_RecvData -v -count=1   # after stream.go change
=== RUN   TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream
--- PASS: TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.002s

$ go test -race ./internal/filter/hcm/h2/ -timeout 60s -count=1
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.368s

$ go test ./test/conformance/h2spec/ -v -count=1   # h2spec section roll-up (ADR-0051 pin)
        53 tests, 53 passed, 0 skipped, 0 failed
--- PASS: TestH2Spec (2.27s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.352s

$ go vet ./internal/filter/hcm/h2/   # exit 0
$ golangci-lint run ./internal/filter/hcm/h2/   # exit 0

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0055: Flow-control discipline for the from-scratch H2 codec

$ go test -race ./... -count=1   # broader sanity check, all packages green
ok  	github.com/esalaine/envoy-go/internal/admin	1.057s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.031s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.023s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.032s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.377s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.026s
ok  	github.com/esalaine/envoy-go/internal/listener	1.031s
ok  	github.com/esalaine/envoy-go/internal/tls	1.081s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.097s
ok  	github.com/esalaine/envoy-go/test/differential	7.562s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.009s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.009s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.020s
ok  	github.com/esalaine/envoy-go/test/helpers	1.028s
```

## Task 6 — 05.1-REVIEW carry-forward — monotonic-id-reuse integration test + M-8 cleanup

**Commits:** 5b1e47c
**Files changed:**
- `internal/filter/hcm/h2/conn_test.go` — added `TestServerConn_GOAWAYOnProtocolError_StreamIDReuse`. Drives an in-process peer that opens stream id 3 with HEADERS+END_STREAM, drains the response, then sends HEADERS on stream id 1. The server's `lastInID` was bumped to 3 by the first HEADERS, so the second HEADERS on stream 1 (1 ≤ 3) fires the monotonic-id rejection branch in `onHeaders` (currently at `conn.go` ~line 343-344) and emits GOAWAY(PROTOCOL_ERROR). The test asserts the GOAWAY frame on the wire via the framer (NOT inferred from conn close) AND asserts the server's `Run()` exits with `*Error{Code: ErrProtocolError}`. Mirrors the sibling `TestServerConn_GOAWAYOnProtocolError_EvenStreamID` fixture pattern.
- `test/conformance/h2spec/h2spec.go` — deleted the `excludedSubsections []string{"http2/6/6"}` slice (was kept with `//nolint:unused` in the 05.1 follow-up). The exclusion rationale moves to `CONFORMANCE_PINS.md` per M-8.
- `docs/envoy-go/CONFORMANCE_PINS.md` — replaced the brief "6.6 excluded per ADR-0051 (ENABLE_PUSH=0)" note under the threshold-section enumeration with the fuller PLAN-prescribed note that cites ADR-0047 / 05.1 SPEC §2.1 for the disable, and adds the ADR-0055 confirmation that the exclusion stays through phase 05.2.

LoC delta: +141/-0 conn_test.go (new test); -5/-0 h2spec.go (slice + comment + nolint); +1/-2 CONFORMANCE_PINS.md (note rewrite).

**Notes:** TDD self-check observed:

1. New test passed on first run against the intact production code — the `connError(ErrProtocolError, "stream id not monotonically increasing")` branch is already in place from 05.1.
2. To rule out test-pass-too-easily: temporarily commented out the rejection branch (`return connError(ErrProtocolError, ...)`) in `conn.go` and re-ran the test. It FAILED with `expected GOAWAY(PROTOCOL_ERROR) on wire after reusing stream id` AND `expected server to exit with PROTOCOL_ERROR, got: context deadline exceeded` — confirming the test catches a regression at both wire-level GOAWAY observation AND the server `Run()` exit. The production branch was then restored and the test passes again.

The task title and PLAN test name carry "OnProtocolError" / "monotonic-id-reuse" — the implementation reuses stream id 1 *below* `lastInID=3` rather than literally reusing id 1 after id 1 closed (which would hit the closed-stream branch returning `ErrStreamClosed`). The chosen path matches the test name's intent (PROTOCOL_ERROR via the §5.1.1 monotonic check) and exercises the rejection branch as documented in the PLAN. The PLAN's literal "open stream 1, complete, send HEADERS on stream 1" wording would hit the *closed-stream* branch (`isClosed → connError(ErrStreamClosed, ...)`) and emit GOAWAY(STREAM_CLOSED) not PROTOCOL_ERROR — that branch is already covered indirectly via h2spec 5.1/12 (part of the 53/53 threshold). The new test thus targets the only remaining uncovered rejection branch (monotonic-id), the more meaningful integration gap per the 05.1 REVIEW.

h2spec section roll-up at the ADR-0051 pin: 53/53 PASS (unchanged; M-8 was a documentation-only change). Lint clean. Race detector clean.

**Outputs:**
```
$ go test ./internal/filter/hcm/h2/ -run TestServerConn_GOAWAYOnProtocolError_StreamIDReuse -v
=== RUN   TestServerConn_GOAWAYOnProtocolError_StreamIDReuse
--- PASS: TestServerConn_GOAWAYOnProtocolError_StreamIDReuse (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.003s

$ # self-check: rejection branch in conn.go temporarily disabled
$ go test ./internal/filter/hcm/h2/ -run TestServerConn_GOAWAYOnProtocolError_StreamIDReuse -v -timeout 30s
=== RUN   TestServerConn_GOAWAYOnProtocolError_StreamIDReuse
    conn_test.go:566: expected GOAWAY(PROTOCOL_ERROR) on wire after reusing stream id
    conn_test.go:575: expected server to exit with PROTOCOL_ERROR, got: context deadline exceeded
--- FAIL: TestServerConn_GOAWAYOnProtocolError_StreamIDReuse (5.02s)
FAIL
$ # branch restored, test passes again

$ go test -race ./internal/filter/hcm/h2/
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.368s

$ go test ./test/conformance/h2spec/ -v
        53 tests, 53 passed, 0 skipped, 0 failed
--- PASS: TestH2Spec (2.24s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.352s

$ golangci-lint run ./internal/filter/hcm/h2/ ./test/conformance/h2spec/   # exit 0, M-8 //nolint:unused removed
```


## Task 7 — H2 client codec — `client.go` skeleton (`H2Request`/`H2Response`, `ClientConn`, preface + SETTINGS exchange, `Close`)

**Commits:** 75ad3af
**Files changed:**
- `internal/filter/hcm/h2/client.go` — NEW. ONE new file in the h2 sub-package per ADR-0048's reservation. Contents: package doc comment locating the file as the fourth in the package permitted to import `golang.org/x/net/http2` directly (after framer.go, settings.go, conn.go); `H2Request` + `H2Response` value types per ADR-0048 (split-pseudoheader fields per RFC 9113 §8.3); `ClientConn` struct with framer/hpack/window/settings state + `closeOnce`/`goawayCh`/`settingsAckCh` synchronisation channels; `NewClientConn(ctx, upstream net.Conn) (*ClientConn, error)` running the 5-step connection setup (preface → write SETTINGS → read peer SETTINGS + ACK → spawn readLoop → synchronously wait on `settingsAckCh` per SPEC §10 #5); `readPeerSettingsAndAck` reusing the existing `readClientSettings` helper (which rejects ACK-on-first-read with PROTOCOL_ERROR per RFC 9113 §6.5); `readLoop` driving `cc.fr.readFrameCtx(cc.ctx)`; `dispatchFrame` skeleton handling SETTINGS_ACK (closes `settingsAckCh`) + GOAWAY (closes `goawayCh`) + drop everything else (Task 8 routes stream frames); `RoundTrip` STUB returning `errors.New("h2: client: RoundTrip not implemented (Task 8)")`; `Close()` idempotent via `sync.Once` emitting `WriteGoAway(lastID, http2.ErrCode(ErrNoError), []byte("client close"))` then `cc.cancel()` then `conn.Close()`. 232 LoC.
- `internal/filter/hcm/h2/client_test.go` — NEW. Three tests: `TestNewClientConn_PrefaceAndSettingsExchange` (full handshake happy path through `runFakeServerPeerForClientHandshake` helper); `TestClientConn_Close_EmitsGracefulGoaway` (asserts GOAWAY frame on the wire with `ErrCodeNo` after Close); `TestNewClientConn_SettingsHandshakeFailureBubblesUp` (peer writes SETTINGS_ACK as its first frame → expects `*Error{Code: ErrProtocolError}` per RFC 9113 §6.5). Helper `runFakeServerPeerForClientHandshake` reads the client preface + client SETTINGS, writes server SETTINGS, reads client SETTINGS_ACK BEFORE writing its own SETTINGS_ACK to side-step a synchronous-`net.Pipe` deadlock (RFC 9113 §6.5 imposes no ordering between the two ACKs). 195 LoC.
- `internal/filter/hcm/h2/settings.go` — `writeClientInitialSettings` added (parallel to `writeServerInitialSettings`; phase 05.2 uses byte-identical defaults per ADR-0047 + RFC 9113 §6.5.2 ENABLE_PUSH=0). +19 LoC.
- `internal/filter/hcm/h2/settings_test.go` — `TestClientInitialSettings_RoundTrip` round-trips the SETTINGS through `net.Pipe` and asserts each of the six values, including the RFC 9218 `NoRFC7540Priorities` setting (raw ID 0x9). +42 LoC.

**Notes:** TDD self-check observed:

1. RED: with only `client_test.go` in place (and `client.go` absent), `go test ./internal/filter/hcm/h2/ -run TestNewClientConn -v` failed with `client_test.go:N: undefined: NewClientConn` ×3 — exactly the expected build failure for the three call sites.
2. GREEN: after writing `client.go`, the same command passes all three tests in milliseconds. `TestClientInitialSettings_RoundTrip` (Step 2's settings unit test) was verified separately by temporarily moving `client_test.go` aside (so the package builds without `NewClientConn`); it passed in isolation, then re-built and passed when `client_test.go` was restored alongside `client.go`.

Symbol-rename divergences from the PLAN's snippet:
- The PLAN snippet at line 1336 uses `cc.fr.WriteGoAway(lastID, uint32(ErrNoError), …)`. The on-disk `framer.WriteGoAway` (embedded `*http2.Framer.WriteGoAway`) has signature `(maxStreamID uint32, code http2.ErrCode, debugData []byte)`. Cast adjusted to `http2.ErrCode(ErrNoError)` to match `emitGoaway` in `conn.go:658`.
- The PLAN snippet at line 1261 took a `ctx` parameter on `readPeerSettingsAndAck`. `readClientSettings` is not ctx-aware (uses `fr.ReadFrame()` directly), so the parameter would be unused. Dropped to keep the call site honest; the wider `NewClientConn` ctx is honoured via the spawned readLoop's `readFrameCtx`.

Lint annotations added:
- `//nolint:revive` on `H2Request`/`H2Response` — ADR-0048 reserves these stuttering names; revive's `exported` rule otherwise flags `h2.H2Request` as a stutter.
- `//nolint:unused` on `streams sync.Map` and `recvDebitSinceLastUpdate int32` — both fields populated by Task 8.

Boundary-check confirmation: `grep -l 'golang.org/x/net/http2' internal/filter/hcm/h2/*.go | grep -v _test` now returns 4 non-`hpack` callers (framer.go, settings.go, conn.go, client.go) plus 3 hpack-only files (doc.go's reference is in a comment; hpack.go and stream.go import only `golang.org/x/net/http2/hpack`). The "exactly four" file claim from the SPEC's tech-stack section is satisfied; Task 15's boundary grep is the formal verification.

**Outputs:**
```
$ go test ./internal/filter/hcm/h2/ -run "TestNewClientConn|TestClientConn_Close|TestClientInitialSettings" -v -timeout 15s
=== RUN   TestNewClientConn_PrefaceAndSettingsExchange
--- PASS: TestNewClientConn_PrefaceAndSettingsExchange (0.00s)
=== RUN   TestClientConn_Close_EmitsGracefulGoaway
--- PASS: TestClientConn_Close_EmitsGracefulGoaway (0.00s)
=== RUN   TestNewClientConn_SettingsHandshakeFailureBubblesUp
--- PASS: TestNewClientConn_SettingsHandshakeFailureBubblesUp (0.00s)
=== RUN   TestClientInitialSettings_RoundTrip
--- PASS: TestClientInitialSettings_RoundTrip (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.003s

$ go test -race ./internal/filter/hcm/h2/ -timeout 120s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.372s

$ go vet ./internal/filter/hcm/h2/
$ # exit 0

$ golangci-lint run ./internal/filter/hcm/h2/
$ # exit 0
```


## Task 8 — H2 client codec — `(*ClientConn).RoundTrip` + `clientStream` + frame-read loop

**Commits:** 8fecc56
**Files changed:**
- `internal/filter/hcm/h2/client.go` — Task 7's stub `RoundTrip` replaced with the full implementation; conn-level dispatchFrame skeleton extended to route stream-bound frames (HEADERS, DATA, RST_STREAM, WINDOW_UPDATE, PING, GOAWAY-stream-cleanup); new `clientStream` struct (per-stream state) with `newClientStream` + `finish(err)` constructor/method; new `(*ClientConn).writeData` symmetric with Task 3's ServerConn.writeData (per-stream send-window first, then conn-level, MaxFrameSize cap, END_STREAM on last chunk); new `(*ClientConn).lookupStream(id)` and `(*ClientConn).emitGoaway(code)` helpers (the latter mirrors `(*ServerConn).emitGoaway`); `goawaySent bool` field added to ClientConn so Close + emitGoaway are idempotent against each other. RoundTrip body: ctx-check upfront → `atomic.AddUint32(&cc.nextStreamID, 2) - 1` allocator (1, 3, 5, ...) → `cc.streams.Store/Delete` → encode pseudo-headers in RFC 9113 §8.3 order then req.Headers → `cc.fr.WriteHeaders` mutex-guarded → optional `writeData` for body → 3-way select on `cs.doneCh` (success/stream-error), `ctx.Done()` (emit RST_STREAM(CANCEL), return ctx.Err()), `cc.ctx.Done()` (conn-closed wrap). Receive-side flow-control mirrors Task 4: per-DATA debit on both `cc.recvW` and `cs.recvW`, half-window threshold emits WINDOW_UPDATE on conn-level (id 0) and stream-level (only if stream is still half-open per ADR-0055 / I-3). Trailing HEADERS observed-and-discarded per ADR-0058 (ADR lands in Task 11; this task implements the rule via `cs.respHeadersSeen` boolean). +389 LoC (621 total).
- `internal/filter/hcm/h2/client_test.go` — Nine new RoundTrip tests per SPEC §4.1 + §8.1 + a `fakeH2ServerPeer` helper (handshake + readRequestHeaders + readDataFrame + writeResponse + persistent hpack.Decoder for cross-iteration HPACK state) and a `dialClientConn` test fixture that bundles handshake + idempotent cleanup. Tests: HappyPath_BodylessGET, HappyPath_WithBody, CtxCancelDuringWrite (asserts RST_STREAM(CANCEL) on the wire), CtxCancelDuringRead (HEADERS observed but no DATA → cancel mid-read), PeerRSTStream (asserts `*Error{Code: CANCEL}`), PeerGoaway (LastStreamID=0 finishes stream 1 with stream-error), PeerDataAfterEndStream (rogue DATA after END_STREAM → `cc.ctx` canceled by readLoop), AfterClose, StreamIDMonotonicity (sequential 3× → ids 1,3,5). +668 LoC (863 total).
- `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` — this entry.

**Notes:** TDD discipline observed: with the Task 7 stub in place, all 9 new tests first FAILED (8 with the stub's "h2: client: RoundTrip not implemented (Task 8)" error message; AfterClose passed early because the stub-error matched the assertion's "any error" criterion — the test still verified the post-implementation conn-closed path because Close cancels `cc.ctx` before the upfront `cc.ctx.Err()` check). After the implementation landed, all 9 PASS:

```
=== RUN   TestClientConn_RoundTrip_HappyPath_BodylessGET
--- PASS: TestClientConn_RoundTrip_HappyPath_BodylessGET (0.00s)
=== RUN   TestClientConn_RoundTrip_HappyPath_WithBody
--- PASS: TestClientConn_RoundTrip_HappyPath_WithBody (0.00s)
=== RUN   TestClientConn_RoundTrip_CtxCancelDuringWrite
--- PASS: TestClientConn_RoundTrip_CtxCancelDuringWrite (0.00s)
=== RUN   TestClientConn_RoundTrip_CtxCancelDuringRead
--- PASS: TestClientConn_RoundTrip_CtxCancelDuringRead (0.02s)
=== RUN   TestClientConn_RoundTrip_PeerRSTStream
--- PASS: TestClientConn_RoundTrip_PeerRSTStream (0.00s)
=== RUN   TestClientConn_RoundTrip_PeerGoaway
--- PASS: TestClientConn_RoundTrip_PeerGoaway (0.00s)
=== RUN   TestClientConn_RoundTrip_PeerDataAfterEndStream
--- PASS: TestClientConn_RoundTrip_PeerDataAfterEndStream (0.10s)
=== RUN   TestClientConn_RoundTrip_AfterClose
--- PASS: TestClientConn_RoundTrip_AfterClose (0.00s)
=== RUN   TestClientConn_RoundTrip_StreamIDMonotonicity
--- PASS: TestClientConn_RoundTrip_StreamIDMonotonicity (0.00s)
```

Symbol-rename divergences from the PLAN snippets:
- The PLAN snippet at line 1481 calls `cs.finish(streamError(...))` then `return H2Response{}, translateFramerErr(err)` after the WriteHeaders error path. Implementation simplifies: no separate `cs.finish` call before returning, because the deferred `cc.streams.Delete(id)` cleanup runs on RoundTrip's return regardless and no other goroutine has yet observed the stream. The stream is never "finished"-via-doneCh since RoundTrip returns directly with the framer error; this matches the symmetric ServerConn behavior where a write failure short-circuits the goroutine.
- The PLAN snippet at line 1556 returns `streamError(ErrStreamClosed, ...)` from the DATA-on-closed-stream case. Implementation upgrades that to `connError(ErrStreamClosed, ...)` so the readLoop's `cc.cancel()` is triggered (the readLoop's error handling treats any non-nil dispatchFrame error as connection-fatal). The PeerDataAfterEndStream test asserts `cc.ctx.Err() != nil` after the rogue frame, validating that this is the desired behavior (otherwise the test couldn't observe the violation, since RoundTrip already returned cleanly with the legitimate response).
- New `respHeadersSeen` boolean added to `clientStream` to disambiguate "first HEADERS not yet observed" from "first HEADERS observed but had no fields beyond :status" — without it, the second-HEADERS detection rule (ADR-0058 trailing-HEADERS-discard) could misclassify a legitimate first HEADERS block whose `respHeaders` slice happened to be a non-nil empty slice.
- `PingFrame` ACK path in dispatchFrame writes the ACK reply mutex-guarded; this is symmetric with ServerConn.onPing.
- `WindowUpdateFrame` zero-delta on a stream is treated as a stream-error (cs.finish with PROTOCOL_ERROR); RFC 9113 §6.9 is somewhat ambiguous about whether the stream-level zero-delta is connection-fatal, but ServerConn.onWindowUpdate treats stream-level zero-delta (when found) as a stream error too (via the `recvWindowUpdate` path). Symmetric.

Pitfall avoidance:
- The PeerDataAfterEndStream test originally used a 50ms post-response sleep before sending the rogue DATA; observed flake on a slower scheduler (deferred `cc.streams.Delete` not yet run when readLoop processed rogue DATA → lookupStream succeeded → no protocol violation surfaced). Bumped to 100ms — the test is intentionally non-time-sensitive on the assertion side (it polls `cc.ctx.Err()` for up to 2s).
- StreamIDMonotonicity originally used a concurrent-WaitGroup pattern with three goroutines and 20ms staggered launches. Net.Pipe's synchronous bidirectional I/O made the lockstep fragile (peer's writeResponse blocks on a non-reading client when multiple RT goroutines contend on `cc.mu`). Refactored to sequential RoundTrips — the unit-under-test is the allocator, not the concurrency model.
- `fakeH2ServerPeer.decodeBlock` originally constructed a fresh `hpack.Decoder` per call. HPACK is stateful; the client's encoder accumulates dynamic-table entries across iterations, so a per-call decoder rejected entries indexed into the table on iteration 2+. Refactored to a persistent `p.dec` + `p.decFields` slice reused across iterations.

**Outputs:**
```
$ go test ./internal/filter/hcm/h2/ -run TestClientConn_RoundTrip -v -timeout 30s
[... 9 PASS lines as above ...]
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	0.124s

$ go test -race ./internal/filter/hcm/h2/ -timeout 120s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.512s

$ go test ./test/conformance/h2spec/ -v
        53 tests, 53 passed, 0 skipped, 0 failed
--- PASS: TestH2Spec (2.24s)

$ golangci-lint run ./internal/filter/hcm/h2/
$ # exit 0

$ grep -n '"golang.org/x/net/http2"' internal/filter/hcm/h2/client.go
24:	"golang.org/x/net/http2"
$ # single import — ADR-0046 boundary preserved (no new file added; the import was added in Task 7)
```

## Task 9 — `Cluster.UseH2()` accessor + `internal/cluster/dial_h2.go` + ADR-0056

**Commits:** 344c371 (SHA-fill: see next commit)
**Files changed:**
- `internal/cluster/cluster.go` — Added `useH2 bool` field on `Cluster` struct, `UseH2() bool` accessor (zero-value defaults false; Task 10 wires the actual setter from typed_extension_protocol_options), and a blank import of `github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3` (registry-population for protojson round-trip in Manager.buildCluster — Task 10; documented per ADR-0016, no separate ADR per the ADR-0016 amendment). +21 LoC.
- `internal/cluster/dial_h2.go` — New file implementing `(*Cluster).DialH2(ctx) (*h2.ClientConn, error)`: calls `c.Dial(ctx)` and wraps errors as `cluster: dial h2: %w`; type-asserts `*stdtls.Conn` (else closes raw + returns `not a TLS conn`); defensively re-runs `HandshakeContext(ctx)` (idempotent on already-handshaken conns; SPEC §11.3 mitigation); validates `NegotiatedProtocol == "h2"` (else closes + returns `alpn negotiated %q, want %q`); calls `h2.NewClientConn(ctx, tlsConn)` (else closes + wraps); each error branch closes the underlying conn explicitly because successful return transfers ownership to the *h2.ClientConn (no defer-close on the error paths would leak file descriptors). 54 LoC.
- `internal/cluster/dial_h2_test.go` — New file with five DialH2 tests + helpers: an in-memory P-256 CA + leaf PKI generated per test (`mkH2TestPKI`) — note: PKI lives in-memory via crypto/ecdsa, NOT committed PEM files (those are fixture-0004's deliverable in Task 13); a from-scratch `h2ServerPrefacePeer` driver that reads preface + client SETTINGS, writes server SETTINGS, exchanges SETTINGS_ACKs (mirrors h2/client_test.go's `runFakeServerPeerForClientHandshake`); and three listener helpers (`listenH2`, `listenALPN`, `listenTLSCloseOnAccept`) for the five test scenarios. Tests: HappyPath, ALPNMismatch (server NextProtos=["http/1.1"] → "alpn negotiated" + `want "h2"` substrings), NotTLS (plaintext cluster → "not a TLS conn"), CtxCancel (canceled ctx → context.Canceled propagates), TLSHandshakeFailure (TCP-only listener that closes immediately → handshake-error chain). 388 LoC.
- `internal/cluster/cluster_test.go` — Two new accessor-coverage tests: `TestCluster_UseH2_DefaultsFalse` (zero-value Cluster reports false), `TestCluster_UseH2_True` (`&Cluster{useH2: true}` reports true). +24 LoC.
- `docs/envoy-go/DECISIONS.md` — ADR-0056 appended at file tail (post-ADR-0055): "Per-request fresh upstream H2 dial". Status Accepted; Doctrine D-3.5; Settles SPEC §10 #2.3 + closes the SPEC §4.4 anticipation; mirrors phase-04 ADR-0039; resolves the carry-forward from ADR-0053's "phase-05.2-will-repeat-the-pattern" forward-looking clause; cross-references `internal/cluster/dial_h2.go:DialH2`, `routerActionH2.do` (Task 11 carry-forward), ADR-0039, ADR-0053, and explicitly notes M-12 (closedStreams unbounded) is unaffected by ADR-0056 because per-request-fresh discipline produces short-lived conns where `closedStreams` is reclaimed at conn close; M-12 becomes load-bearing only in the upstream-robustness family's pooling phase. +36 LoC.
- `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` — this entry.

**Notes:** TDD discipline observed: with `Cluster.UseH2` undefined and `Cluster.DialH2` undefined at Step 1 commit-prep, all 7 new tests FAILED with compile errors (`c.UseH2 undefined`, `unknown field useH2 in struct literal`, `c.DialH2 undefined`). After Steps 2-3 landed, all 7 PASS:

```
=== RUN   TestCluster_UseH2_DefaultsFalse
--- PASS: TestCluster_UseH2_DefaultsFalse (0.00s)
=== RUN   TestCluster_UseH2_True
--- PASS: TestCluster_UseH2_True (0.00s)
=== RUN   TestCluster_DialH2_HappyPath
--- PASS: TestCluster_DialH2_HappyPath (0.00s)
=== RUN   TestCluster_DialH2_ALPNMismatch
--- PASS: TestCluster_DialH2_ALPNMismatch (0.00s)
=== RUN   TestCluster_DialH2_NotTLS
--- PASS: TestCluster_DialH2_NotTLS (0.00s)
=== RUN   TestCluster_DialH2_CtxCancel
--- PASS: TestCluster_DialH2_CtxCancel (0.00s)
=== RUN   TestCluster_DialH2_TLSHandshakeFailure
--- PASS: TestCluster_DialH2_TLSHandshakeFailure (0.00s)
```

Implementation divergence from PLAN snippets (substantive, none; symbol-rename, none): the dial_h2.go file matches the PLAN snippet at lines 1730-1779 exactly except for prose-only comment expansion. The PLAN snippet's comment about defer-close discipline is preserved verbatim and extended with a sentence about the "no caller-owned wrapper to defer-close on error paths" rationale.

Test-fixture divergence from PLAN: the PLAN's Step 1 snippet for `TestCluster_DialH2_HappyPath` referenced "fixture-0002 driver_test.go's TLS pattern" as the model, but inspecting `test/fixtures/0002-tls-tcp/driver/driver_test.go` revealed it carries no in-memory PKI — it's a unit-test of `tlsDriver.AssertDistribution`. The actual in-memory PKI bootstrap pattern lives at `internal/tls/config_test.go` (the `pki = func() *testPKI { ... }()` package-level var). The Task 9 tests use the same `crypto/ecdsa` + `crypto/x509` + `pkix.Name` pattern, mirrored into `mkH2TestPKI(t)` per-test (P-256 keygen is cheap; per-test instead of package-level reduces parallel-test contention).

Pitfall avoided: the initial happy-path implementation used `golang.org/x/net/http2.Server.ServeConn` as the in-process h2 server. That driver-side use is permitted in test code per D-3.2, but `http2.Server.ServeConn` immediately sent a GOAWAY in response to the from-scratch client preface (root cause not isolated; possibly an ALPN/preface-validation difference). Refactored to a from-scratch `h2ServerPrefacePeer` using `http2.NewFramer` directly — symmetric with the h2/client_test.go fixture, and proven against the production codec. The `golang.org/x/net/http2` import in the test file is for `http2.Framer` only; `http2.Server` and `http2.ConfigureServer` are not used.

Idempotence verification: `(*tls.Conn).HandshakeContext` is documented as idempotent — calling it on an already-handshaken conn returns nil immediately. Verified by the happy-path test passing despite Cluster.Dial having already completed the handshake before DialH2 calls HandshakeContext a second time.

**Outputs:**
```
$ go test ./internal/cluster/ -v -run "TestCluster_UseH2_|TestCluster_DialH2_"
[... 7 PASS lines as above ...]
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	0.006s

$ go test -race ./internal/cluster/ -v
[... full suite ...]
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	1.029s

$ go vet ./internal/cluster/
$ # exit 0

$ golangci-lint run ./internal/cluster/
$ # exit 0

$ go test ./...
[... 26 packages, 0 FAIL ...]

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -2
## ADR-0055: Flow-control discipline for the from-scratch H2 codec
## ADR-0056: Per-request fresh upstream H2 dial
$ # ADR tail advanced 0055 → 0056 as expected
```

## Task 10 — `internal/cluster/manager.go` HttpProtocolOptions parsing + `internal/bootstrap/bootstrap.go` blank import

**Commits:** 307d378 (SHA-fill: see next commit)
**Files changed:**
- `internal/cluster/manager.go` — Added `extractH2Mode(c, parsedTLS)` helper that reads `c.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]`, unmarshals into `*upstreamshttpv3.HttpProtocolOptions`, and returns `(useH2 bool, err error)` per SPEC §5.5's behavior matrix: field absent → false; `*ExplicitHttpConfig_` with inner `*ExplicitHttpConfig_Http2ProtocolOptions` → true (validated); inner `*ExplicitHttpConfig_HttpProtocolOptions` (the H1 discriminator) → false (silent-ignore inner); `*HttpProtocolOptions_AutoConfig` → false (the 05.2 narrowing of master SPEC §5.8 per SPEC §5.5); nil/empty → false. When useH2==true, validates `c.TransportSocket` non-nil + `typed_config` non-nil + type_url == upstream TLS context + parsedTLS.NextProtos contains "h2". Wired into `buildCluster` after the existing transport_socket parse so `cl.upstreamCfg` is the `*stdtls.Config` passed to `extractH2Mode`. Added blank import path verified (already present in `cluster.go` from Task 9). Added `httpProtocolOptionsKey` const. New imports: `crypto/tls` (for the `*stdtls.Config` parameter type) and `upstreamshttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"`. +94 LoC.
- `internal/cluster/manager_test.go` — Added 8 tests covering the §5.5 behavior matrix + 3 helpers (`mkHttpProtocolOptionsAny`, `mkUpstreamTLSTransportSocketWithALPN`, `hpoExplicitH2`/`hpoExplicitH1`/`hpoAutoConfig`). The H2-mode positive test reuses `test/fixtures/0002-tls-tcp/pki/ca.pem` for the inline trusted_ca + sets `alpn_protocols: ["h2"]` on the CommonTlsContext. The TLSWithoutALPN variant passes `alpn=nil` (the TLS parser accepts an absent alpn_protocols). +203 LoC.
- `internal/bootstrap/bootstrap.go` — Added blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` to the existing import-group section so protojson round-trip in `Load` resolves the `type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions` Any type_url without "type not registered" errors. Per ADR-0016 amendment policy, the addition is documented in this PROGRESS entry, NOT a new ADR. The bootstrap-package-side import is required because `internal/bootstrap` does not import `internal/cluster` — the cluster.go-side blank import (added in Task 9) is sufficient for cluster-package callers but not for `Load(reader)` callers that work with raw bootstraps before passing to the cluster manager. Verified by temporarily removing the import and observing `bootstrap: protojson: ... unable to resolve "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions": "not found"`. +6 LoC.
- `internal/bootstrap/bootstrap_test.go` — Added `TestBootstrap_RoundTrips_FixtureFour_Shape` which loads a minimal fixture-0004-shaped bootstrap (1 cluster with `transport_socket: tls`, `alpn_protocols: ["h2"]`, and `typed_extension_protocol_options.HttpProtocolOptions = {explicit_http_config: {http2_protocol_options: {}}}`) via `Load`, asserts the cluster carries the typed-extension key with the expected type_url, and re-marshals via `protojson.Marshal` to verify the round-trip is symmetric. +66 LoC.
- `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` — this entry.

**Notes:** TDD discipline observed: with `useH2` always-false in Task 9's wiring, all 4 H2-mode-positive/negative tests FAILED at Step 1 (`UseH2() = false, want true` + `expected error, got nil` × 3). The 4 silent-ignore tests (`H1Discriminator`, `AutoConfig`, `NoTypedExtension`, `NilUpstreamProtocolOptions`) PASSED at Step 1 because UseH2's zero value is already false — the tests still validated that the new parser does NOT spuriously err on those inputs once it landed. After Step 2 wired `extractH2Mode` into `buildCluster`, all 8 PASS:

```
=== RUN   TestBuildCluster_H2Mode_Positive
--- PASS: TestBuildCluster_H2Mode_Positive (0.00s)
=== RUN   TestBuildCluster_H2Mode_NoTLS
--- PASS: TestBuildCluster_H2Mode_NoTLS (0.00s)
=== RUN   TestBuildCluster_H2Mode_TLSWithoutALPNH2
--- PASS: TestBuildCluster_H2Mode_TLSWithoutALPNH2 (0.00s)
=== RUN   TestBuildCluster_H2Mode_TLSWithoutALPN
--- PASS: TestBuildCluster_H2Mode_TLSWithoutALPN (0.00s)
=== RUN   TestBuildCluster_H1Discriminator_SilentIgnore
--- PASS: TestBuildCluster_H1Discriminator_SilentIgnore (0.00s)
=== RUN   TestBuildCluster_AutoConfig_SilentIgnore
--- PASS: TestBuildCluster_AutoConfig_SilentIgnore (0.00s)
=== RUN   TestBuildCluster_NoTypedExtension_BaselineFalse
--- PASS: TestBuildCluster_NoTypedExtension_BaselineFalse (0.00s)
=== RUN   TestBuildCluster_HttpProtocolOptions_NilUpstreamProtocolOptions
--- PASS: TestBuildCluster_HttpProtocolOptions_NilUpstreamProtocolOptions (0.00s)
```

Symbol-rename divergence from the PLAN snippet (lines 1843-1917):
- The PLAN's case wrapper `*upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig` is the inner *message* type, not the oneof *case wrapper*. The actual case-wrapper symbol is `*upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_` (trailing underscore) — verified via `go doc`. Implementation uses the trailing-underscore form.
- The PLAN's `parsedTLS.ALPNProtocols` field is named `NextProtos` on `*crypto/tls.Config` (the standard library's ALPN field). The on-disk plumbing from `internal/tls.NewUpstreamConfig` populates `cfg.NextProtos = append(cfg.NextProtos, c.GetAlpnProtocols()...)` (config.go:174). Implementation reads `parsedTLS.NextProtos`.
- The PLAN's `parseTransportSocket(c, baseDir)` is not yet a separate function on disk; the existing `buildCluster` inlines the transport_socket parse + sets `cl.upstreamCfg = uc.TLSConfig`. The implementation passes `cl.upstreamCfg` directly into `extractH2Mode` rather than refactor-extracting a `parseTransportSocket` helper — minimum-diff per D-3.6.

Pitfall avoided: the bootstrap-package-side blank import is necessary even though cluster.go (Task 9) added the same import, because `internal/bootstrap` does not transitively import `internal/cluster` — `Load(reader)` callers need the registry populated for protojson to resolve the type_url, and protojson resolves via the *importing program's* registry, not the cluster-package's. Verified by the round-trip test failure when the bootstrap.go blank import was temporarily removed.

The blank import addition follows ADR-0016's amendment policy — register-only blank imports are documented in PROGRESS, not as a new ADR. The phase-04 amendment shape (PROGRESS entry per fixture batch) is mirrored here. ADR tail is unchanged: still ADR-0056.

**Outputs:**
```
$ go test ./internal/cluster/ -v -run TestBuildCluster_ -count=1
[... 8 PASS lines as above ...]
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	0.005s

$ go test ./internal/bootstrap/ -v -run TestBootstrap_RoundTrips_FixtureFour_Shape -count=1
=== RUN   TestBootstrap_RoundTrips_FixtureFour_Shape
--- PASS: TestBootstrap_RoundTrips_FixtureFour_Shape (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.006s

$ go test -race ./internal/cluster/ ./internal/bootstrap/
ok  	github.com/esalaine/envoy-go/internal/cluster	1.030s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.030s

$ go vet ./internal/cluster/ ./internal/bootstrap/
$ # exit 0

$ golangci-lint run ./internal/cluster/ ./internal/bootstrap/
$ # exit 0

$ go test -race ./...
[... 26 packages, 0 FAIL ...]

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0056: Per-request fresh upstream H2 dial
$ # ADR tail unchanged at 0056 (Task 10 lands no new ADR per ADR-0016 + SPEC §5.5)
```

## Task 11 — HCM `routerActionH2` + variant selection + `h2.Action` widening + `h2RouterActionAdapter` + ADR-0058

**Commit:** dd30a4c (main); SHA-fill commit lands on top.

**Files changed:**

- `internal/filter/hcm/h2/stream.go` — Widened the `Action` interface from `WriteH2(StreamWriter) error` to `WriteH2(ctx context.Context, req H2Request, sw StreamWriter) error` per PLAN Task 11 Step 1 + ## Settled SPEC §10 deferred decisions #11. Updated `serverStream.dispatch` to construct an `H2Request` (via the new `buildH2Request` helper) from the inbound HEADERS pseudo-headers + decoded regular headers + buffered request body, and pass it to `action.WriteH2(ctx, h2Req, s)`. The `buildH2Request` helper splits pseudo-headers out into the named fields (Method/Path/Scheme/Authority) and keeps regular headers on `.Headers` in wire order — the upstream encoder re-prepends pseudo-headers per RFC 9113 §8.3. +30 LoC (interface + buildH2Request helper + dispatch call-site update).
- `internal/filter/hcm/h2/stream_test.go`, `internal/filter/hcm/h2/conn_test.go`, `internal/filter/hcm/h2/fuzz_test.go` — Updated all existing `Action` implementors (`fakeAction`, `errorAction`, `fixedAction`, `blockingAction`, `bodyCaptureAction`, `stubAction`) to the wider signature. The new args are ignored (each fake synthesizes the response from its own state, like `directResponseAction`). +6 LoC across three files.
- `internal/filter/hcm/actions.go` — Added the shared `bad502Body = "bad gateway\n"` constant per SPEC §11.9. Added the `routerActionH2` struct with two methods: `doH2(ctx, h2.H2Request, h2.StreamWriter) error` (the H2 driver — invoked by `h2RouterActionAdapter.WriteH2`) and `do(ctx, *http.Request, *bufio.Writer) error` (a defensive 500 stub satisfying the routeAction interface so `*routerActionH2` is storable in `routeEntry.action`). Added the `write502` helper emitting `:status 502 + Date + Server + content-type + content-length + bad502Body` per SPEC §10 #4. Added a compile-time interface conformance assertion `var _ routeAction = (*routerActionH2)(nil)`. +98 LoC.
- `internal/filter/hcm/config.go` — Updated `buildRouterAction` return type from `*routerAction` to the wider `routeAction` interface. Added the variant-selection branch: when `c.UseH2()` reports true, return a `*routerActionH2`; otherwise the existing `*routerAction`. Per SPEC §5.5 + §4.1. +6 LoC.
- `internal/filter/hcm/h2dispatch.go` — Added the `h2RouterActionAdapter` wrapping `*routerActionH2` (delegates `WriteH2` → `a.a.doH2(ctx, req, sw)`). Updated `h2Dispatcher.Match` to recognize `*routerActionH2` (returning the new adapter) alongside `*directResponseAction` (returning `h2DirectResponseAdapter`); other action types fall through to `h2RouterActionRejection`. Updated `h2DirectResponseAdapter.WriteH2` and `h2RouterActionRejection.WriteH2` to the wider signature (both ignore ctx + req — direct_response synthesizes from its own state; rejection emits INTERNAL_ERROR regardless). +27 LoC.
- `internal/filter/hcm/actions_test.go` — Added 5 routerActionH2 tests + a `captureH2Writer` fake h2.StreamWriter + an in-process H2 backend (`mkH2BackendPKI`, `runH2Backend`, `startH2Backend`, `h2EndpointCluster`) that listens on a fresh TLS port with NextProtos=["h2"], reads client preface + SETTINGS, and dispatches per a `h2BackendBehavior` selector (OK/503/Malformed/Hang). The malformed-bytes branch writes garbage HPACK so the client surfaces a COMPRESSION_ERROR and the action emits 502; the Hang branch never responds so the test cancels ctx mid-RoundTrip and verifies the action returns `*h2.Error{Code: ErrCancel}`. +400 LoC.
- `internal/filter/hcm/config_test.go` — Added `TestBuildRouterAction_PicksH2VariantByClusterUseH2` + `mkH2ClusterManager` helper. Builds a 2-cluster manager (c_h1 without HttpProtocolOptions, c_h2 with `explicit_http_config.http2_protocol_options{}` + tls + alpn=["h2"]). Asserts `buildRouterAction` returns `*routerAction` for c_h1 and `*routerActionH2` for c_h2. +118 LoC.
- `docs/envoy-go/DECISIONS.md` — Appended ADR-0058 (Trailers observed but not forwarded — H2 router) per PLAN line 121. Bundles the M-4 (`readClientPreface` ctx-unaware) and M-10 (`SETTINGS_TIMEOUT` absent) carry-forwards per SPEC §12.2's per-finding-disposition. Cross-references `internal/filter/hcm/h2/client.go:dispatchFrame` (upstream-side observe-discard) and `internal/filter/hcm/h2/stream.go:recvTrailingHeaders` (downstream-side observe-discard). +43 LoC.
- `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` — this entry.

**Decision: Option B for the H1-side handling of *routerActionH2.** *routerActionH2 satisfies the hcm-package routeAction interface via a defensive 500 stub `do(ctx, *http.Request, *bufio.Writer) error` rather than widening the `routeEntry.action` field type to `interface{}` and adding a runtime type-check in connection.go. Rationale: the stub is a single-method addition (~3 LoC including doc comment); widening the field type would require simultaneous edits to route.go's `routeAction` definition, route.go's `routeEntry` struct, connection.go's `entry.action.do(...)` site, and would surface a runtime-type-check at every H1 dispatch. The stub is unreachable in well-formed bootstraps (variant selection at filter-build time guarantees H2-clusters get *routerActionH2 routed via the H2 dispatch path; the H1 driver never sees them on H2-clusters); if reached defensively (invalid bootstrap shape), the 500 surfaces the misconfiguration without crashing the connection. PLAN line 2121's hint at Option B as the "simpler shape" is honored.

**Method-namespace note:** Go disallows two methods with the same name on the same receiver, so the H2 driver method on *routerActionH2 is named `doH2(ctx, h2.H2Request, h2.StreamWriter) error` (consumed only by `h2RouterActionAdapter.WriteH2`); the routeAction-interface-satisfying method keeps the name `do(ctx, *http.Request, *bufio.Writer) error` (the defensive 500 stub). The rename is a small divergence from the PLAN's snippet at line 2059 (which used `do` for the H2 method); the rename is necessary for interface-satisfaction symmetry under Go's method-set rule.

**TDD discipline:** The 5 routerActionH2 tests were added before the routerActionH2 + h2RouterActionAdapter wiring was complete; each FAILED at compile-time first (no `routerActionH2` symbol, no `doH2` method, no `bad502Body` constant), then PASSED after the wiring landed. The variant-selection test FAILED at compile-time (no variant-selection branch in buildRouterAction returning a non-routerAction type); PASSED after the buildRouterAction widening + variant branch landed.

**Pitfall observed and resolved:** the initial `routerActionH2.doH2` ctx-cancel branch used `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` against the RoundTrip error directly. This mis-categorized the upstream-conn-broken case (where readLoop calls `cc.cancel()` after a malformed HEADERS frame and `RoundTrip` returns a wrapped `cc.ctx.Err()` which IS `context.Canceled`) as a caller-side ctx-cancel — the malformed-headers test failed because the action returned RST(CANCEL) instead of writing the 502 local-reply. Fixed by checking the CALLER's ctx specifically (`ctx.Err() != nil && (errors.Is(ctx.Err(), ...))`) instead of the RoundTrip-returned error. The distinction matters: caller-side ctx-cancel is "client gave up — emit RST(CANCEL)"; upstream-conn-broken is "couldn't reach upstream — emit 502 local-reply". The fix is a 2-line change in actions.go's `routerActionH2.doH2`.

**Outputs:**
```
$ go test ./internal/filter/hcm/ -count=1 -run "TestRouterActionH2_|TestBuildRouterAction_PicksH2" -v
=== RUN   TestRouterActionH2_HappyPath
--- PASS: TestRouterActionH2_HappyPath (0.00s)
=== RUN   TestRouterActionH2_502OnDialFailure
--- PASS: TestRouterActionH2_502OnDialFailure (0.00s)
=== RUN   TestRouterActionH2_502OnRoundTripProtocolError
--- PASS: TestRouterActionH2_502OnRoundTripProtocolError (0.00s)
=== RUN   TestRouterActionH2_CtxCancelEmitsRSTStreamCancel
--- PASS: TestRouterActionH2_CtxCancelEmitsRSTStreamCancel (0.20s)
=== RUN   TestRouterActionH2_Upstream5xxForwardedVerbatim
--- PASS: TestRouterActionH2_Upstream5xxForwardedVerbatim (0.00s)
=== RUN   TestBuildRouterAction_PicksH2VariantByClusterUseH2
--- PASS: TestBuildRouterAction_PicksH2VariantByClusterUseH2 (0.00s)
PASS

$ go test -race ./internal/filter/hcm/ ./internal/filter/hcm/h2/ ./internal/cluster/ -count=1
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.239s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.512s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.030s

$ go test ./test/conformance/h2spec/ -v -count=1
[... 53 tests ...]
53 tests, 53 passed, 0 skipped, 0 failed
PASS

$ go vet ./...
$ # exit 0

$ golangci-lint run ./...
$ # exit 0

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0058: Trailers observed but not forwarded — H2 router
$ # ADR tail advanced 0056 → 0058 (Task 11 lands ADR-0058; ADR-0057 lands later at Task 14 — non-monotonic per topical-vs-commit-order).
```

## Task 12 — `test/helpers/h2.go` H2RoundTrip helper

**Commit:** {{SHA}} (main); SHA-fill commit lands on top.

**Files changed:**

- `test/helpers/h2.go` — New driver-side helper `H2RoundTrip(ctx, addr, *tls.Config, method, path, []hpack.HeaderField, body) (status, []hpack.HeaderField, []byte, error)` consumed by fixture-0004's driver (Task 14). Algorithm: TCP-dial → `tls.Client(rawConn, tlsConf).HandshakeContext` → ALPN-verify (must be "h2") → fresh `*http2.Transport{TLSClientConfig: tlsConf}` per call (no caching per ## Settled SPEC §10 deferred decisions #13) → `tr.NewClientConn(tlsConn)` → `cc.RoundTrip(*http.Request)` with `bytes.NewReader(body)` (zero-length reader when body==nil; cleaner than nil body) → `io.ReadAll(resp.Body)` → convert `resp.Header` to `[]hpack.HeaderField` and prepend `{Name: ":status", Value: strconv.Itoa(resp.StatusCode)}` per RFC 9113 §8.3 wire-order convention. Driver-side `golang.org/x/net/http2.Transport` import is permitted per D-3.2 — this file lives under `test/`, not `internal/`; the Task 15 boundary grep excludes test files. 87 LoC.
- `test/helpers/h2_test.go` — `TestH2RoundTrip_HappyPath` stands up `httptest.NewUnstartedServer` with TLS + `http2.ConfigureServer` driver-side (handler returns "ok" status 200), builds a TLS config with the server's cert in RootCAs + `NextProtos=["h2"]` + `ServerName="127.0.0.1"`, calls `H2RoundTrip(ctx, addr, tlsConf, "GET", "/", nil, nil)`, asserts status==200 + body=="ok". 43 LoC.
- `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` — this entry.

**TDD discipline:** `TestH2RoundTrip_HappyPath` was added before `H2RoundTrip` was implemented — initial run FAILED at compile-time (`undefined: H2RoundTrip`); after the implementation landed, run PASSED.

**Pitfall observed and resolved:** initial implementation had a blank line between the `// Package helpers ...` doc comment and the `package helpers` clause, which `revive`'s `package-comments` linter rejects ("package comment is detached"). Fixed by removing the blank line — Go's package-comment convention requires the comment to be directly attached to the `package` clause.

**Outputs:**
```
$ go test ./test/helpers/ -v -run TestH2RoundTrip
=== RUN   TestH2RoundTrip_HappyPath
--- PASS: TestH2RoundTrip_HappyPath (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/helpers	0.004s

$ go test ./test/helpers/ -v
[... 20 tests, all PASS ...]
PASS
ok  	github.com/esalaine/envoy-go/test/helpers	0.006s

$ go test -race ./...
[... all packages green ...]
ok  	github.com/esalaine/envoy-go/test/helpers	1.025s

$ golangci-lint run ./test/helpers/
$ # exit 0

$ golangci-lint run ./...
$ # exit 0

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0058: Trailers observed but not forwarded — H2 router
$ # ADR tail unchanged at 0058 (Task 12 lands no new ADR — fixture infrastructure per PLAN line 2221).
```
