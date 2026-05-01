# Phase 07.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2/06.1/06.2 PROGRESS.md structure.

## Preamble — execution preconditions

None. All 14 preconditions satisfied at cold-start: branch `phase/07.1-http-filter-framework-impl` at HEAD `0bfaaf1` (master tip at branch creation); Docker client (28.4.0) + server (28.1.1) both reported; `go version go1.26.2 linux/amd64` (PLAN required go1.23+); `golangci-lint has version v1.64.8`; all 7 differential fixtures (0000–0006, including Docker-dependent 0004/0005/0006) PASS via `TestDifferential` subtests (precondition #6's regex did not match Go subtests directly; verified substantive intent by running `TestDifferential` directly); `github.com/envoyproxy/go-control-plane/envoy v1.32.4` present in `go.mod`; `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0069:` (next-free 0070); SPEC.md last-commit is `f2dd6593ceb2f74fff73120b053c32bb0c0b1486`; `internal/filter/http/` absent; the four refactor-target action-method signatures (`directResponseAction.do`, `routerAction.do`, `routerActionH2.doH2`, `h2DirectResponseAdapter.WriteH2`) present in `internal/filter/hcm/`; `BEHAVIOR_CONTRACT.md` has both `## HTTP/1.1` and `## HTTP/2` anchor headings present; HTTPRegistry symbol absent in `internal/` and `cmd/`; reference Envoy image `envoyproxy/envoy:v1.37.2` pulled successfully.

## Task 1 — Execution-precondition check + PROGRESS.md preamble [ADR-0070]

**Commits:** 9d59c6d — this task's commit
**Notes:** Created PROGRESS.md; verified all 14 preconditions per PLAN §"Execution preconditions"; phase-06.2 close confirmed present in HEAD; SPEC at f2dd6593ceb2f74fff73120b053c32bb0c0b1486; ADR tail at 0069 (next-free 0070); internal/filter/http/ absent (the package implementation lands at Task 2+); HTTPRegistry symbol absent. Landed ADR-0070 (phase-07 planner-time split per ADR-0045's pattern).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/07.1-http-filter-framework-impl
$ go version
go version go1.26.2 linux/amd64
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
ADR-0069:
$ git log -1 --format=%H -- docs/envoy-go/phases/07.1-http-filter-framework/SPEC.md
f2dd6593ceb2f74fff73120b053c32bb0c0b1486
$ test ! -d internal/filter/http && echo OK
OK
```

## Task 2 — internal/filter/http/types.go + callbacks.go [ADR-0071]

**Commits:** 0a6526b — this task's commit
**Notes:** Created internal/filter/http/{doc,types,callbacks}.go + test pairs. Defined StreamDecoderFilter + StreamEncoderFilter interfaces, three status enums (FilterHeadersStatus/FilterDataStatus/FilterTrailersStatus), DecoderFilterCallbacks + EncoderFilterCallbacks interfaces, two-step HTTPFilterFactory + FilterInstanceFactory pattern. Landed ADR-0071 (HTTP filter iteration protocol shape; supersedes ADR-0040 totally; partially supersedes ADR-0042). go test ./internal/filter/http/ green; package compiles standalone (registry.go + chain.go + perroute.go land in subsequent tasks).
**Outputs:**
```
$ go test ./internal/filter/http/ -count=1 -v
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	0.002s
$ grep -nE '^## ADR-0071:' docs/envoy-go/DECISIONS.md
2547:## ADR-0071: HTTP filter iteration protocol shape
```

## Task 3 — internal/filter/http/registry.go [ADR-0072]

**Commits:** bf83004 — this task's commit
**Notes:** Created internal/filter/http/{registry,registry_test}.go. Removed the Task-2 forward-declaration stub of HTTPRegistry from types.go now that registry.go defines the real type. HTTPRegistry exposes Register / Lookup / Freeze / KnownTypeURLs; freeze-after-boot invariant via atomic.Bool; duplicate Register and post-Freeze Register both panic with verbatim messages per SPEC §5.3 + ADR-0072. Six new tests (registry shape + duplicate-panic + post-Freeze-panic + Freeze-idempotent + Lookup-after-Freeze + concurrent-Lookup-race-clean); all pass under -race. Landed ADR-0072 (*HTTPRegistry threaded constructor map; mirrors *stats.Registry LBP-1 from ADR-0059).
**Outputs:**
```
$ go test -race ./internal/filter/http/ -count=1 -v
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestRegistry_RegisterLookup
--- PASS: TestRegistry_RegisterLookup (0.00s)
=== RUN   TestRegistry_DuplicateRegisterPanics
--- PASS: TestRegistry_DuplicateRegisterPanics (0.00s)
=== RUN   TestRegistry_PostFreezeRegisterPanics
--- PASS: TestRegistry_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_FreezeIdempotent
--- PASS: TestRegistry_FreezeIdempotent (0.00s)
=== RUN   TestRegistry_LookupAfterFreezeOK
--- PASS: TestRegistry_LookupAfterFreezeOK (0.00s)
=== RUN   TestRegistry_ConcurrentLookup_RaceClean
--- PASS: TestRegistry_ConcurrentLookup_RaceClean (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.009s
$ grep -nE '^## ADR-0072:' docs/envoy-go/DECISIONS.md
2595:## ADR-0072: HTTPRegistry threaded constructor map, no package-global
```

## Task 4 — internal/filter/http/perroute.go [ADR-0073]

**Commits:** 02c45b0 — this task's commit
**Notes:** Created internal/filter/http/{perroute,perroute_test}.go. Defines routeScope, scopeParsed, cacheKey, PerRouteConfig types; BuildPerRouteConfig parser+validator with chain-name allow-list (errors on unknown keys with verbatim message); Resolve method performs Route > VHost > RC most-specific-override lookup with lazy per-(filter,route) cache. Six new tests covering merge precedence (route-wins, vh-fallback, rc-fallback), nil-on-absent, unknown-filter-name rejection, and cache-pointer-stability. Landed ADR-0073 (typed_per_filter_config 3-tier merge; amends ADR-0041's silent-ignore set).
**Outputs:**
```
$ go test ./internal/filter/http/ -count=1 -v
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RouteWins
--- PASS: TestPerRoute_BuildAndResolve_RouteWins (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_VHostFallback
--- PASS: TestPerRoute_BuildAndResolve_VHostFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RCFallback
--- PASS: TestPerRoute_BuildAndResolve_RCFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_NilOnAbsent
--- PASS: TestPerRoute_BuildAndResolve_NilOnAbsent (0.00s)
=== RUN   TestPerRoute_BuildRejectsUnknownFilterName
--- PASS: TestPerRoute_BuildRejectsUnknownFilterName (0.00s)
=== RUN   TestPerRoute_LazyCacheHitMiss
--- PASS: TestPerRoute_LazyCacheHitMiss (0.00s)
=== RUN   TestRegistry_RegisterLookup
--- PASS: TestRegistry_RegisterLookup (0.00s)
=== RUN   TestRegistry_DuplicateRegisterPanics
--- PASS: TestRegistry_DuplicateRegisterPanics (0.00s)
=== RUN   TestRegistry_PostFreezeRegisterPanics
--- PASS: TestRegistry_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_FreezeIdempotent
--- PASS: TestRegistry_FreezeIdempotent (0.00s)
=== RUN   TestRegistry_LookupAfterFreezeOK
--- PASS: TestRegistry_LookupAfterFreezeOK (0.00s)
=== RUN   TestRegistry_ConcurrentLookup_RaceClean
--- PASS: TestRegistry_ConcurrentLookup_RaceClean (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	0.002s
$ grep -nE '^## ADR-0073:' docs/envoy-go/DECISIONS.md
2631:## ADR-0073: typed_per_filter_config 3-tier merge model
```

## Task 5 — internal/filter/http/chain.go decode-side iteration

**Commits:** 73b2783 — this task's commit
**Notes:** Created internal/filter/http/{chain,chain_test}.go (initial decode-side surface). Defines filterBufferLimitBytes constant (1<<20 = 1 MiB; honored at Task 9), FilterChain struct (filters slice, decodeIdx/encodeIdx int cursors per Decision §3.5, decodeResumeCh/encodeResumeCh capacity-1 buffered channels for async-resume per ADR-0071, decodeBuf decode buffer scaffolding for Task 9, localReplyOnce/localReplyDone/encodeStarted scaffolding for Task 7), NewFilterChain (per-stream allocation, per-filter callback wiring), RunDecodeHeaders (decode-side declaration-order iteration with Continue / StopIteration / unknown-status err handling), parkDecode (single-goroutine select on decodeResumeCh / ctx.Done), Destroy (idempotent OnDestroy fan-out), decoderCB + encoderCB concrete callback structs (idempotent non-blocking signal sends; SendLocalReply stubbed for Task 7). PLAN scaffold's `RequestRouteConfig() any` corrected to `RequestRouteConfig() proto.Message` to satisfy DecoderFilterCallbacks interface from callbacks.go (returning nil satisfies the interface; PLAN's "temporary divergence" framing was a planner-time error since Go's interface satisfaction is compile-time-checked). Three new tests for decode-side iteration: TestChain_Decode_AllContinue (Continue chain), TestChain_Decode_StopIteration_ResumeAdvances (async resume after 20ms), TestChain_Decode_StopIteration_CtxCancelAborts (ctx-cancel during park yields ctx.Err + OnDestroy fires on chain.Destroy). All 21 tests pass under -race.
**Outputs:**
```
$ go test -race ./internal/filter/http/ -run TestChain_Decode -count=1 -v
=== RUN   TestChain_Decode_AllContinue
--- PASS: TestChain_Decode_AllContinue (0.00s)
=== RUN   TestChain_Decode_StopIteration_ResumeAdvances
--- PASS: TestChain_Decode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Decode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Decode_StopIteration_CtxCancelAborts (0.01s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.037s
$ go test -race ./internal/filter/http/ -count=1 -v
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestChain_Decode_AllContinue
--- PASS: TestChain_Decode_AllContinue (0.00s)
=== RUN   TestChain_Decode_StopIteration_ResumeAdvances
--- PASS: TestChain_Decode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Decode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Decode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestPerRoute_BuildAndResolve_RouteWins
--- PASS: TestPerRoute_BuildAndResolve_RouteWins (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_VHostFallback
--- PASS: TestPerRoute_BuildAndResolve_VHostFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RCFallback
--- PASS: TestPerRoute_BuildAndResolve_RCFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_NilOnAbsent
--- PASS: TestPerRoute_BuildAndResolve_NilOnAbsent (0.00s)
=== RUN   TestPerRoute_BuildRejectsUnknownFilterName
--- PASS: TestPerRoute_BuildRejectsUnknownFilterName (0.00s)
=== RUN   TestPerRoute_LazyCacheHitMiss
--- PASS: TestPerRoute_LazyCacheHitMiss (0.00s)
=== RUN   TestRegistry_RegisterLookup
--- PASS: TestRegistry_RegisterLookup (0.00s)
=== RUN   TestRegistry_DuplicateRegisterPanics
--- PASS: TestRegistry_DuplicateRegisterPanics (0.00s)
=== RUN   TestRegistry_PostFreezeRegisterPanics
--- PASS: TestRegistry_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_FreezeIdempotent
--- PASS: TestRegistry_FreezeIdempotent (0.00s)
=== RUN   TestRegistry_LookupAfterFreezeOK
--- PASS: TestRegistry_LookupAfterFreezeOK (0.00s)
=== RUN   TestRegistry_ConcurrentLookup_RaceClean
--- PASS: TestRegistry_ConcurrentLookup_RaceClean (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.041s
```

## Task 6 — chain.go encode-side reverse iteration

**Commits:** a11df11 — this task's commit
**Notes:** Added RunEncodeHeaders / RunEncodeData / RunEncodeTrailers + parkEncode to internal/filter/http/chain.go. Encode iteration traverses `len(filters)-1 → 0` per SPEC §5.5 + §11.1 empirical pin (Envoy filter order is reverse-of-decode on the encode side). Continue advances the cursor; StopIteration / DataStopIterationAndBuffer / DataStopIterationNoBuffer / TrailersStopIteration park on encodeResumeCh until ContinueEncoding fires (non-blocking, capacity-1 coalesce); ctx.Done unparks with ctx.Err. RunEncodeHeaders also flips encodeStarted to true (Task 7's beginLocalReply consults this flag). Three new tests added under -race: TestChain_Encode_ReverseOrder (asserts c→b→a invocation order via encodeRecorder wrapper), TestChain_Encode_StopIteration_ResumeAdvances (parks at b, async ContinueEncoding 20ms later, both filters ran exactly once), and TestChain_Encode_StopIteration_CtxCancelAborts (encode-side analogue of the existing decode ctx-cancel test). Recording-filter struct gained three additional fields (encHeadersStatus / encDataStatus / encTrailersStatus) so encode-side returns can be configured independently from decode-side; existing tests are zero-value compatible (Continue/DataContinue/TrailersContinue). Out of scope and deferred per PLAN: SendLocalReply trigger (Task 7), buffer overflow / 413-on-encode (Task 9), HCM dispatch wiring (Tasks 15/16). No new ADR introduced (PLAN reserves ADR slots for tasks T1/T2/T3/T4/T7/T9/T18). All 24 tests pass under -race.

**Spec-review follow-up:** RunEncodeTrailers gained the `default:` clause that all other Run* methods have, plus TestChain_Encode_UnknownTrailersStatusErrs. Originated in spec-review of `a11df11` (PLAN.md:1631-1650 scaffold gap propagated into impl); fixed in this commit. Test count post-fix: 25 (was 24). Test was authored TDD-red first (hung 2s on missing default → t.Fatalf), then default clause added → green.
**Outputs:**
```
$ go test -race ./internal/filter/http/ -count=1 -v
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestChain_Decode_AllContinue
--- PASS: TestChain_Decode_AllContinue (0.00s)
=== RUN   TestChain_Decode_StopIteration_ResumeAdvances
--- PASS: TestChain_Decode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Decode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Decode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestChain_Encode_ReverseOrder
--- PASS: TestChain_Encode_ReverseOrder (0.00s)
=== RUN   TestChain_Encode_StopIteration_ResumeAdvances
--- PASS: TestChain_Encode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Encode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Encode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestPerRoute_BuildAndResolve_RouteWins
--- PASS: TestPerRoute_BuildAndResolve_RouteWins (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_VHostFallback
--- PASS: TestPerRoute_BuildAndResolve_VHostFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RCFallback
--- PASS: TestPerRoute_BuildAndResolve_RCFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_NilOnAbsent
--- PASS: TestPerRoute_BuildAndResolve_NilOnAbsent (0.00s)
=== RUN   TestPerRoute_BuildRejectsUnknownFilterName
--- PASS: TestPerRoute_BuildRejectsUnknownFilterName (0.00s)
=== RUN   TestPerRoute_LazyCacheHitMiss
--- PASS: TestPerRoute_LazyCacheHitMiss (0.00s)
=== RUN   TestRegistry_RegisterLookup
--- PASS: TestRegistry_RegisterLookup (0.00s)
=== RUN   TestRegistry_DuplicateRegisterPanics
--- PASS: TestRegistry_DuplicateRegisterPanics (0.00s)
=== RUN   TestRegistry_PostFreezeRegisterPanics
--- PASS: TestRegistry_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_FreezeIdempotent
--- PASS: TestRegistry_FreezeIdempotent (0.00s)
=== RUN   TestRegistry_LookupAfterFreezeOK
--- PASS: TestRegistry_LookupAfterFreezeOK (0.00s)
=== RUN   TestRegistry_ConcurrentLookup_RaceClean
--- PASS: TestRegistry_ConcurrentLookup_RaceClean (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.071s
```

## Task 7 — chain.go beginLocalReply + first-call-wins [ADR-0075]

**Commits:** a03a1d3 — this task's commit
**Notes:** Implemented `beginLocalReply` in `internal/filter/http/chain.go` per ADR-0075 + SPEC §11 #4 empirical pin: the synthesized response enters the encode chain at `filter[len-1]`, the FULL encode chain runs in reverse declaration order (INCLUDING the calling filter's own encode side), first-call-wins via `sync.Once`, and second-call-after-encode-started is a no-op + the diagnostic log line `hcm: filter %q called SendLocalReply after encode-side started; ignoring`. Wired `decoderCB.SendLocalReply` to call `beginLocalReply` (replacing the Task 5 stub) and wired `decoderCB.RequestRouteConfig` to consult `c.perRoute.Resolve(filterName, c.routeIdx)`. Added two struct fields to `FilterChain` — `routeIdx int` and `ambientCtx context.Context` — set by HCM dispatch via the new `SetRequestCtx(ctx, routeIdx)` method (HCM wire-up lands in Task 13). Framework-injected response headers: `Content-Length` (always set from `len(body)`) and `Content-Type` (defaults to `text/plain` if user did not supply); `Date` and `Server` are intentionally NOT set here per ADR-0075 (b) — they are filled by the HCM wire-write path. `RunDecodeHeaders` gained a post-filter-call short-circuit: after `f.DecodeHeaders` returns, if `localReplyDone` is set the loop returns `(false, nil)` immediately rather than parking — critical for the `StopIteration`-after-SendLocalReply pattern (the calling filter typically returns StopIteration; without this gate the dispatch goroutine would deadlock on `decodeResumeCh` since no `ContinueDecoding` will arrive after a SendLocalReply). Four new tests added (29 total in package): `TestChain_SendLocalReply_EntersAtLenMinus1` (the §11 #4 empirical-pin assertion in unit-test form — synthetic 4-filter chain `[a, b, c, router]` where `b`'s `DecodeHeaders` triggers SendLocalReply; observed encode order is `router → c → b → a` and decode side past b never ran), `TestChain_SendLocalReply_FirstCallWins` (two back-to-back SendLocalReply calls from the same DecodeHeaders → each encode-side filter runs exactly once), `TestChain_SendLocalReply_CallingFilterEncodeRuns` (asserts ADR-0075 (d) — the calling filter's own encode side runs), and `TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs` (a reentrant encoder fires a second SendLocalReply during the synthesized-reply encode pass; the diagnostic log line is emitted to the captured buffer and the original encode pass completes unaffected). The `TestChain_SendLocalReply_FirstCallWins` test does emit the "second call ignored" log line to stderr in the test output (visible in the verbatim dump above) — that's expected behavior since the test does not override the diag-log writer; the assertion is on encode-side counters, not log content. **PLAN deviation:** PLAN.md line 1717 framed `SetDiagLogWriter` as "test-only helper not in this task," but the Task 7 acceptance includes `TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs` which MUST be able to capture the log line — `SetDiagLogWriter` is therefore landed in this task. Mirrors the phase-04..06.2 PLAN-deviation precedent (codify in PROGRESS notes). All tests pass under `-race`.
**Outputs:**
```
$ go test -race ./internal/filter/http/ -count=1 -v
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestChain_Decode_AllContinue
--- PASS: TestChain_Decode_AllContinue (0.00s)
=== RUN   TestChain_Decode_StopIteration_ResumeAdvances
--- PASS: TestChain_Decode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Decode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Decode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestChain_Encode_ReverseOrder
--- PASS: TestChain_Encode_ReverseOrder (0.00s)
=== RUN   TestChain_Encode_StopIteration_ResumeAdvances
--- PASS: TestChain_Encode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Encode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Encode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestChain_Encode_UnknownTrailersStatusErrs
--- PASS: TestChain_Encode_UnknownTrailersStatusErrs (0.00s)
=== RUN   TestChain_SendLocalReply_EntersAtLenMinus1
--- PASS: TestChain_SendLocalReply_EntersAtLenMinus1 (0.00s)
=== RUN   TestChain_SendLocalReply_FirstCallWins
hcm: filter "b" called SendLocalReply after encode-side started; ignoring
--- PASS: TestChain_SendLocalReply_FirstCallWins (0.00s)
=== RUN   TestChain_SendLocalReply_CallingFilterEncodeRuns
--- PASS: TestChain_SendLocalReply_CallingFilterEncodeRuns (0.00s)
=== RUN   TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs
--- PASS: TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RouteWins
--- PASS: TestPerRoute_BuildAndResolve_RouteWins (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_VHostFallback
--- PASS: TestPerRoute_BuildAndResolve_VHostFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RCFallback
--- PASS: TestPerRoute_BuildAndResolve_RCFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_NilOnAbsent
--- PASS: TestPerRoute_BuildAndResolve_NilOnAbsent (0.00s)
=== RUN   TestPerRoute_BuildRejectsUnknownFilterName
--- PASS: TestPerRoute_BuildRejectsUnknownFilterName (0.00s)
=== RUN   TestPerRoute_LazyCacheHitMiss
--- PASS: TestPerRoute_LazyCacheHitMiss (0.00s)
=== RUN   TestRegistry_RegisterLookup
--- PASS: TestRegistry_RegisterLookup (0.00s)
=== RUN   TestRegistry_DuplicateRegisterPanics
--- PASS: TestRegistry_DuplicateRegisterPanics (0.00s)
=== RUN   TestRegistry_PostFreezeRegisterPanics
--- PASS: TestRegistry_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_FreezeIdempotent
--- PASS: TestRegistry_FreezeIdempotent (0.00s)
=== RUN   TestRegistry_LookupAfterFreezeOK
--- PASS: TestRegistry_LookupAfterFreezeOK (0.00s)
=== RUN   TestRegistry_ConcurrentLookup_RaceClean
--- PASS: TestRegistry_ConcurrentLookup_RaceClean (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.071s
```

**Code-quality-review follow-up:** Code-quality reviewer report on `a03a1d3` flagged two important + three minor issues, addressed in a single follow-up commit. **I-1 (Content-Type case-canonicalization on user-supplied headers)** — `beginLocalReply`'s merge step copied user-supplied keys verbatim via `merged[k] = v`, so a user header `http.Header{"content-type": []string{"application/json"}}` survived as a non-canonical key; the subsequent `merged.Get("Content-Type")` (which canonicalizes its argument) missed the user value and the framework injected the default `text/plain` under the canonical key, producing a duplicate `content-type` + `Content-Type` pair on the wire. Fix: replace the raw map copy with a per-value `merged.Add(k, v)` loop, which canonicalizes via `textproto.CanonicalMIMEHeaderKey` internally. Regression test `TestChain_SendLocalReply_UserContentTypeNonCanonicalKey` (TDD-red verified pre-fix; failing assertion `expected exactly one canonical Content-Type=application/json; got [text/plain]`) asserts (a) canonical `Content-Type` has exactly one value `application/json`, (b) no `content-type` key present, (c) total Content-Type values across all casings is exactly 1. **I-2 (`ambientCtx` nil-unsafe pre-`SetRequestCtx`)** — `decoderCB.SendLocalReply` propagates `c.ambientCtx` to `beginLocalReply` → `RunEncode*` → `parkEncode(nil)`, where `<-ctx.Done()` on a nil interface panics (or, depending on race ordering, blocks forever masking cancellation). Fix: `NewFilterChain` now default-initializes `ambientCtx = context.Background()`; `SetRequestCtx` overwrites in production. Regression test `TestChain_SendLocalReply_DefaultsAmbientCtxToBackground` (TDD-red verified pre-fix; failing assertion `expected a.EncodeHeaders called once; got 0` because the nil-ctx panic short-circuited the encode chain) constructs a chain WITHOUT `SetRequestCtx`, has the router's encode side return `StopIteration` to force a `parkEncode` reach, then asynchronously fires `ContinueEncoding` and asserts the chain completes within 2s with both filters' EncodeHeaders called exactly once. **M-1 (misleading first-call-wins comment)** — the test comment claimed `sync.Once` dedups the second call; in reality `c.encodeStarted.Load()` short-circuits at the top of `beginLocalReply` before `Once.Do` is reached on the second call. Comment updated to acknowledge the layered mechanism (encodeStarted gate fires first; `Once` is defense-in-depth for hypothetical pre-`RunEncodeHeaders` concurrent calls — ruled out in production by ADR-0071's single-driver invariant). **M-2 (`fmt.Fprintf` errcheck warning)** — wrapped the diagnostic-log `fmt.Fprintf` call in `_, _ = fmt.Fprintf(...)` to silence golangci-lint's errcheck warning; baseline lint diff confirms the `chain.go:344` errcheck warning is gone post-fix. **M-4 (stale `_ = status` discard + misleading comment)** — Go does not warn on unused function parameters, so the `_ = status` line and accompanying "Suppress unused-warning until Task 13 lands" comment were misleading (the parameter IS used by future call sites — it just is not consumed inside the framework body); removed the discard and replaced the comment with a brief note that the status int travels to the HCM wire-write layer (Task 13). One additional staticcheck SA1008 false positive on `got["content-type"]` in the I-1 regression test (probing for a non-canonical key absence is the negative assertion the test needs) is suppressed via `//nolint:staticcheck` with rationale on the immediately-preceding line. M-3 (ADR prose "cancel any pending resume" — vacuous parenthetical) and M-5 (Sprintf vs strconv.Itoa — stylistic; PLAN scaffold uses Sprintf) deferred per reviewer-marked out-of-scope. Test count post-fix: 31 (was 29). All 31 tests pass under `-race`. Net lint warnings introduced: zero (one removed via M-2; the new SA1008 is intentional + suppressed).

**Outputs:**
```
$ go test -race ./internal/filter/http/ -count=1 -v
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestChain_Decode_AllContinue
--- PASS: TestChain_Decode_AllContinue (0.00s)
=== RUN   TestChain_Decode_StopIteration_ResumeAdvances
--- PASS: TestChain_Decode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Decode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Decode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestChain_Encode_ReverseOrder
--- PASS: TestChain_Encode_ReverseOrder (0.00s)
=== RUN   TestChain_Encode_StopIteration_ResumeAdvances
--- PASS: TestChain_Encode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Encode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Encode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestChain_Encode_UnknownTrailersStatusErrs
--- PASS: TestChain_Encode_UnknownTrailersStatusErrs (0.00s)
=== RUN   TestChain_SendLocalReply_EntersAtLenMinus1
--- PASS: TestChain_SendLocalReply_EntersAtLenMinus1 (0.00s)
=== RUN   TestChain_SendLocalReply_FirstCallWins
hcm: filter "b" called SendLocalReply after encode-side started; ignoring
--- PASS: TestChain_SendLocalReply_FirstCallWins (0.00s)
=== RUN   TestChain_SendLocalReply_CallingFilterEncodeRuns
--- PASS: TestChain_SendLocalReply_CallingFilterEncodeRuns (0.00s)
=== RUN   TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs
--- PASS: TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs (0.00s)
=== RUN   TestChain_SendLocalReply_UserContentTypeNonCanonicalKey
--- PASS: TestChain_SendLocalReply_UserContentTypeNonCanonicalKey (0.00s)
=== RUN   TestChain_SendLocalReply_DefaultsAmbientCtxToBackground
--- PASS: TestChain_SendLocalReply_DefaultsAmbientCtxToBackground (0.02s)
=== RUN   TestPerRoute_BuildAndResolve_RouteWins
--- PASS: TestPerRoute_BuildAndResolve_RouteWins (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_VHostFallback
--- PASS: TestPerRoute_BuildAndResolve_VHostFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RCFallback
--- PASS: TestPerRoute_BuildAndResolve_RCFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_NilOnAbsent
--- PASS: TestPerRoute_BuildAndResolve_NilOnAbsent (0.00s)
=== RUN   TestPerRoute_BuildRejectsUnknownFilterName
--- PASS: TestPerRoute_BuildRejectsUnknownFilterName (0.00s)
=== RUN   TestPerRoute_LazyCacheHitMiss
--- PASS: TestPerRoute_LazyCacheHitMiss (0.00s)
=== RUN   TestRegistry_RegisterLookup
--- PASS: TestRegistry_RegisterLookup (0.00s)
=== RUN   TestRegistry_DuplicateRegisterPanics
--- PASS: TestRegistry_DuplicateRegisterPanics (0.00s)
=== RUN   TestRegistry_PostFreezeRegisterPanics
--- PASS: TestRegistry_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_FreezeIdempotent
--- PASS: TestRegistry_FreezeIdempotent (0.00s)
=== RUN   TestRegistry_LookupAfterFreezeOK
--- PASS: TestRegistry_LookupAfterFreezeOK (0.00s)
=== RUN   TestRegistry_ConcurrentLookup_RaceClean
--- PASS: TestRegistry_ConcurrentLookup_RaceClean (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.092s
```

## Task 8 — chain.go async-resume + concurrent-callback race tests

**Commits:** cca6e65 — this task's commit
**Notes:** Added three race-tested cases to `internal/filter/http/chain_test.go` to explicitly exercise the concurrent-callback discipline + per-stream-goroutine model that Tasks 5/6 wired into `chain.go`. **No production-code change** — the buffered-1 + non-blocking-send pattern in `decoderCB.ContinueDecoding` / `encoderCB.ContinueEncoding`, the `sync.Once` + `encodeStarted` first-call-wins guard in `beginLocalReply`, and the `destroyOnce`-guarded `Destroy()` were all already present from Tasks 5/6/7. Task 8 is the explicit race-test coverage for SPEC §5.7 + §14.10's four bullets + §15 acceptance bullet 2 (`go test -race -count=10 -v` passes) (this task lands three of the four; the `HTTPRegistry.Lookup` race-clean bullet is already covered by `TestRegistry_ConcurrentLookup_RaceClean` from Task 3). Per `superpowers:test-driven-development`'s "red" reasoning — without the production-code disciplines under test, each new test would catch a regression: (1) without the `default:` arm of `decoderCB.ContinueDecoding`'s select, 63 of the 64 senders in `TestChain_ConcurrentContinueDecoding_Coalesced` would block forever on a full channel and the WaitGroup's 2s timeout would fire `t.Fatalf("ContinueDecoding goroutines leaked")`; (2) without `localReplyOnce` + `encodeStarted` being concurrency-safe, `TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply`'s timer-goroutine + dispatch-goroutine concurrent access would surface a DATA RACE under `-race`; (3) if `Destroy()` closed `encodeResumeCh` (it doesn't, by design), `TestChain_DestroyVsInFlightContinueEncoding`'s post-Destroy `b.ecb.ContinueEncoding()` would panic with "send on closed channel" and the inner `recover()` would `t.Errorf` — the test confirms the production code's "channel send silently dropped" via the buffered-1 + non-blocking-send pattern (the channel is intentionally not closed so in-flight senders are safe). **PLAN deviation context for Step 3:** PLAN.md framed Step 3 as testing "channel send silently dropped" but the production `Destroy()` does NOT have a "kill the channel" mechanism (and shouldn't — closing would panic concurrent senders); the observable invariant in the production code is that `ContinueEncoding`'s non-blocking send into `encodeResumeCh` is a no-op once the dispatch goroutine has returned (the buffered-1 absorbs the first stale send, and subsequent senders hit the `default:` arm and drop). The test asserts this invariant by spawning 8 concurrent post-Destroy `ContinueEncoding` calls + verifying no panic + verifying `Destroy()` is idempotent (sync.Once). Test count: 31 → 34 (3 new). All 34 tests × 10 iterations = 340 PASS, zero FAIL, zero DATA RACE under `go test -race -count=10 -v`.

**Outputs:**
```
$ go test -race ./internal/filter/http/ -run 'TestChain_ConcurrentContinueDecoding_Coalesced|TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply|TestChain_DestroyVsInFlightContinueEncoding' -count=10 -v
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.294s

$ go test -race ./internal/filter/http/ -count=10 -v   # full suite (representative trailing block; 10× iterations all PASS)
... [9 prior iterations elided — each identical to the iteration shown below; 34 tests × 10 iterations = 340 PASS lines, 0 FAIL, 0 DATA RACE; verified via `... | grep -c -- '--- PASS'` → 340 and `... | grep -c -- '--- FAIL\|DATA RACE'` → 0]
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestChain_Decode_AllContinue
--- PASS: TestChain_Decode_AllContinue (0.00s)
=== RUN   TestChain_Decode_StopIteration_ResumeAdvances
--- PASS: TestChain_Decode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Decode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Decode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestChain_Encode_ReverseOrder
--- PASS: TestChain_Encode_ReverseOrder (0.00s)
=== RUN   TestChain_Encode_StopIteration_ResumeAdvances
--- PASS: TestChain_Encode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Encode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Encode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestChain_Encode_UnknownTrailersStatusErrs
--- PASS: TestChain_Encode_UnknownTrailersStatusErrs (0.00s)
=== RUN   TestChain_SendLocalReply_EntersAtLenMinus1
--- PASS: TestChain_SendLocalReply_EntersAtLenMinus1 (0.00s)
=== RUN   TestChain_SendLocalReply_FirstCallWins
hcm: filter "b" called SendLocalReply after encode-side started; ignoring
--- PASS: TestChain_SendLocalReply_FirstCallWins (0.00s)
=== RUN   TestChain_SendLocalReply_CallingFilterEncodeRuns
--- PASS: TestChain_SendLocalReply_CallingFilterEncodeRuns (0.00s)
=== RUN   TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs
--- PASS: TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs (0.00s)
=== RUN   TestChain_SendLocalReply_UserContentTypeNonCanonicalKey
--- PASS: TestChain_SendLocalReply_UserContentTypeNonCanonicalKey (0.00s)
=== RUN   TestChain_SendLocalReply_DefaultsAmbientCtxToBackground
--- PASS: TestChain_SendLocalReply_DefaultsAmbientCtxToBackground (0.02s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RouteWins
--- PASS: TestPerRoute_BuildAndResolve_RouteWins (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_VHostFallback
--- PASS: TestPerRoute_BuildAndResolve_VHostFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RCFallback
--- PASS: TestPerRoute_BuildAndResolve_RCFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_NilOnAbsent
--- PASS: TestPerRoute_BuildAndResolve_NilOnAbsent (0.00s)
=== RUN   TestPerRoute_BuildRejectsUnknownFilterName
--- PASS: TestPerRoute_BuildRejectsUnknownFilterName (0.00s)
=== RUN   TestPerRoute_LazyCacheHitMiss
--- PASS: TestPerRoute_LazyCacheHitMiss (0.00s)
=== RUN   TestRegistry_RegisterLookup
--- PASS: TestRegistry_RegisterLookup (0.00s)
=== RUN   TestRegistry_DuplicateRegisterPanics
--- PASS: TestRegistry_DuplicateRegisterPanics (0.00s)
=== RUN   TestRegistry_PostFreezeRegisterPanics
--- PASS: TestRegistry_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_FreezeIdempotent
--- PASS: TestRegistry_FreezeIdempotent (0.00s)
=== RUN   TestRegistry_LookupAfterFreezeOK
--- PASS: TestRegistry_LookupAfterFreezeOK (0.00s)
=== RUN   TestRegistry_ConcurrentLookup_RaceClean
--- PASS: TestRegistry_ConcurrentLookup_RaceClean (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	2.116s

$ go vet ./internal/filter/http/...
$ go build ./...
```

## Task 9 — chain.go buffer overflow [ADR-0076]

**Commits:** 45b49d9 — this task's commit
**Notes:** Implemented `RunDecodeData` + decode-side 413-on-overflow path + encode-side overflow sentinel in `internal/filter/http/chain.go` per ADR-0076 + SPEC §11 #3 empirical pin + SPEC §15 acceptance bullet 2. **`RunDecodeData`** iterates decode-side filters in declaration order with cursor-reset to 0 at the start (mirroring the encode-side cursor reset in `RunEncodeData` from Task 6 — the cursor sits at `len(filters)` after `RunDecodeHeaders` completes); on `DataStopIterationAndBuffer` the chain checks `len(c.decodeBuf)+len(data) > filterBufferLimitBytes` and either accumulates into `decodeBuf` + parks via `parkDecode` or — on overflow — synthesizes the verbatim 413 wire shape via `beginLocalReply(c.ambientCtx, c.decodeIdx, 413, "Payload Too Large", http.Header{"Connection": ["close"]})` and returns `(false, nil)` with the chain transitioned to encode mode. The 17-byte body literal is pinned in the package-private constant `localReply413BodyBytes = "Payload Too Large"` (no trailing newline per §11 #3). `Date` and `Server` headers are intentionally NOT framework-injected (per ADR-0075 (b) — those land on the HCM wire-write path at Task 15). The same `localReplyDone` early-out + post-filter-call short-circuit pattern from `RunDecodeHeaders` (Task 7) is honored, so a filter's `DecodeData` calling `SendLocalReply` directly is also handled correctly. **`RunEncodeData`** updated to track per-stream encode-side buffer accumulation in the new `encodeBuf []byte` field; the cap check `len(c.encodeBuf)+len(data) > filterBufferLimitBytes` runs at the TOP of the method (before any filter is iterated on the overflowing chunk) and returns the package-private sentinel `errEncodeBufferOverflow = errors.New("chain: encode-side buffer overflow; resetting connection")` with `(false, errEncodeBufferOverflow)`. The HCM dispatch path consumes this sentinel at Tasks 15 + 16 (H1: `connection: close` + conn close; H2: RST_STREAM with `INTERNAL_ERROR`). After successful iteration on a within-cap chunk, the bytes are appended to `encodeBuf` to update the accumulator. Two new struct fields landed for symmetry with the existing `decodeBuf`/`decodeBufOver`: `encodeBuf []byte` + `encodeBufOver bool`. **Tests:** four new tests added (38 total, was 34): `TestChain_DecodeData_OverflowSynthesizes413` (the §11 #3 verbatim-pin assertion in unit-test form — synthetic 2-filter chain `[buf, router]` where `buf.DecodeData` returns `DataStopIterationAndBuffer`; chunk `filterBufferLimitBytes+1` bytes triggers 413; `captureRecorder` snapshot asserts body == `"Payload Too Large"` (17 bytes, no trailing newline) + `Content-Length: 17` + `Content-Type: text/plain` + `Connection: close` on the encode-side captured headers + `chain.localReplyDone == true` proxies the unobservable status code), `TestChain_DecodeData_BelowCapDoesNotSynthesize` (a 1024-byte chunk on the same chain with async `ContinueDecoding` 20ms later — iteration completes, `localReplyDone=false`, `decodeBuf` accumulated 1024 bytes, router's decode side ran), `TestChain_EncodeData_OverflowReturnsSentinel` (encode-side: first `RunEncodeData` call with `filterBufferLimitBytes` bytes succeeds; second call with 1 byte triggers `errEncodeBufferOverflow` at exactly the cap boundary; verified via `errors.Is`), and `TestChain_EncodeData_BelowCapNoSentinel` (three 1024-byte chunks succeed without sentinel). The `bufferOnceFilter` + `captureRecorder` test helpers were added (extending the Task 7 `headerCaptureRecorder` pattern with body capture). **PLAN deviations:** (i) the PLAN's pseudo-code for `RunDecodeData` did not show an explicit `c.decodeIdx = 0` reset; without the reset the loop body never runs because `decodeIdx == len(filters)` after `RunDecodeHeaders` completes. The reset is necessary + matches `RunEncodeData`'s symmetric reset already present from Task 6; codified as a deviation here per the phase-04..06.2 PLAN-deviation precedent. (ii) **Encode-side cap-check at top-of-method.** PLAN scaffold framed the check inside the `DataStopIterationAndBuffer` case (gated on filter status). Landed as a top-of-method check instead: this is a deliberate broadening — encode-side overflow caps total wire output regardless of filter status (a chain that returns `DataContinue` for every chunk of a multi-MB response still warrants the connection reset). The sentinel returns BEFORE any filter sees the overflowing chunk, so no partially-emitted overflow data hits the wire. Codified per the phase-04..06.2 PLAN-deviation precedent. All 38 tests pass under `-race`; `go vet ./...` + `go build ./...` clean. ADR-0076 appended to `DECISIONS.md` (full Status / Date / Doctrine / Amends / Context / Decision / Inline-supersession / Alternatives / Consequences / Lands-in-task template per ADR-0001 + ADR-0075 stylistic precedent). **Code-review-loop follow-up:** I-1 (encodeBuf size-only — replaced `encodeBuf []byte` with `encodeBufLen int` to drop up-to-1-MiB-per-stream redundantly-held memory), I-2 (drop `encodeBufOver` set-but-never-read flag), I-3 (deviation (ii) reworded for honesty about the broadened semantic), and M-1 (rename `localReply413BodyBytes` → `localReply413Body` since the value is a Go `string`, not `[]byte`) addressed in commit `94e854c`.
**Outputs:**
```
$ go test -race ./internal/filter/http/ -count=1 -v
=== RUN   TestDecoderFilterCallbacks_Compile
--- PASS: TestDecoderFilterCallbacks_Compile (0.00s)
=== RUN   TestEncoderFilterCallbacks_Compile
--- PASS: TestEncoderFilterCallbacks_Compile (0.00s)
=== RUN   TestChain_Decode_AllContinue
--- PASS: TestChain_Decode_AllContinue (0.00s)
=== RUN   TestChain_Decode_StopIteration_ResumeAdvances
--- PASS: TestChain_Decode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Decode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Decode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestChain_Encode_ReverseOrder
--- PASS: TestChain_Encode_ReverseOrder (0.00s)
=== RUN   TestChain_Encode_StopIteration_ResumeAdvances
--- PASS: TestChain_Encode_StopIteration_ResumeAdvances (0.02s)
=== RUN   TestChain_Encode_StopIteration_CtxCancelAborts
--- PASS: TestChain_Encode_StopIteration_CtxCancelAborts (0.01s)
=== RUN   TestChain_Encode_UnknownTrailersStatusErrs
--- PASS: TestChain_Encode_UnknownTrailersStatusErrs (0.00s)
=== RUN   TestChain_SendLocalReply_EntersAtLenMinus1
--- PASS: TestChain_SendLocalReply_EntersAtLenMinus1 (0.00s)
=== RUN   TestChain_SendLocalReply_FirstCallWins
hcm: filter "b" called SendLocalReply after encode-side started; ignoring
--- PASS: TestChain_SendLocalReply_FirstCallWins (0.00s)
=== RUN   TestChain_SendLocalReply_CallingFilterEncodeRuns
--- PASS: TestChain_SendLocalReply_CallingFilterEncodeRuns (0.00s)
=== RUN   TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs
--- PASS: TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs (0.00s)
=== RUN   TestChain_SendLocalReply_UserContentTypeNonCanonicalKey
--- PASS: TestChain_SendLocalReply_UserContentTypeNonCanonicalKey (0.00s)
=== RUN   TestChain_SendLocalReply_DefaultsAmbientCtxToBackground
--- PASS: TestChain_SendLocalReply_DefaultsAmbientCtxToBackground (0.02s)
=== RUN   TestChain_ConcurrentContinueDecoding_Coalesced
--- PASS: TestChain_ConcurrentContinueDecoding_Coalesced (0.02s)
=== RUN   TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply
--- PASS: TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply (0.01s)
=== RUN   TestChain_DestroyVsInFlightContinueEncoding
--- PASS: TestChain_DestroyVsInFlightContinueEncoding (0.00s)
=== RUN   TestChain_DecodeData_OverflowSynthesizes413
--- PASS: TestChain_DecodeData_OverflowSynthesizes413 (0.01s)
=== RUN   TestChain_DecodeData_BelowCapDoesNotSynthesize
--- PASS: TestChain_DecodeData_BelowCapDoesNotSynthesize (0.02s)
=== RUN   TestChain_EncodeData_OverflowReturnsSentinel
--- PASS: TestChain_EncodeData_OverflowReturnsSentinel (0.00s)
=== RUN   TestChain_EncodeData_BelowCapNoSentinel
--- PASS: TestChain_EncodeData_BelowCapNoSentinel (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RouteWins
--- PASS: TestPerRoute_BuildAndResolve_RouteWins (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_VHostFallback
--- PASS: TestPerRoute_BuildAndResolve_VHostFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_RCFallback
--- PASS: TestPerRoute_BuildAndResolve_RCFallback (0.00s)
=== RUN   TestPerRoute_BuildAndResolve_NilOnAbsent
--- PASS: TestPerRoute_BuildAndResolve_NilOnAbsent (0.00s)
=== RUN   TestPerRoute_BuildRejectsUnknownFilterName
--- PASS: TestPerRoute_BuildRejectsUnknownFilterName (0.00s)
=== RUN   TestPerRoute_LazyCacheHitMiss
--- PASS: TestPerRoute_LazyCacheHitMiss (0.00s)
=== RUN   TestRegistry_RegisterLookup
--- PASS: TestRegistry_RegisterLookup (0.00s)
=== RUN   TestRegistry_DuplicateRegisterPanics
--- PASS: TestRegistry_DuplicateRegisterPanics (0.00s)
=== RUN   TestRegistry_PostFreezeRegisterPanics
--- PASS: TestRegistry_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_FreezeIdempotent
--- PASS: TestRegistry_FreezeIdempotent (0.00s)
=== RUN   TestRegistry_LookupAfterFreezeOK
--- PASS: TestRegistry_LookupAfterFreezeOK (0.00s)
=== RUN   TestRegistry_ConcurrentLookup_RaceClean
--- PASS: TestRegistry_ConcurrentLookup_RaceClean (0.00s)
=== RUN   TestFilterHeadersStatus_Values
--- PASS: TestFilterHeadersStatus_Values (0.00s)
=== RUN   TestFilterDataStatus_Values
--- PASS: TestFilterDataStatus_Values (0.00s)
=== RUN   TestFilterTrailersStatus_Values
--- PASS: TestFilterTrailersStatus_Values (0.00s)
=== RUN   TestFilterInterfaces_Compile
--- PASS: TestFilterInterfaces_Compile (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.148s

$ go vet ./...
$ go build ./...

$ grep -nE '^## ADR-0076:' docs/envoy-go/DECISIONS.md
2702:## ADR-0076: Body buffer cap; 413 on decode overflow; reset on encode overflow
```
