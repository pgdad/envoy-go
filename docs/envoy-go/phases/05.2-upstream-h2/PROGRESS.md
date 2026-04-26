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

**Commits:** (SHA-fill: see next commit)
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
