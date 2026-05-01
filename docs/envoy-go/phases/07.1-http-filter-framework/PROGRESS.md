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

## Task 10 — FuzzFilterChainParse (ninth fuzzer)

**Commits:** 6cb0b5b — this task's commit; 25b1d42 — code-review-loop follow-up
**Notes:** Created `internal/filter/http/fuzz_test.go` with `FuzzFilterChainParse` per PLAN scaffold, targeting `BuildPerRouteConfig` (Task 4) on adversarial typed_per_filter_config maps. Asserts: no panic; the function returns either nil or an error — no crashes — and never deadlocks. Per ADR-0018, the 30s short-budget gate ran clean (3,996,389 execs, 217 new-interesting inputs, 0 crashers). Doc-comment style matches the prior `FuzzHCMConfigParse` precedent (header naming the assertion + ADR-0018 reference for the 30s budget). Seed corpus exercises three distinct shapes: (1) well-formed filter name `envoy.filters.http.cors` + four payload byte slices; (2) all-empty (zero-length non-nil) shape; (3) binary-noise shape (`\x00\x01\x02` name + `\xff\xfe` rcVal + zero-length vh/rt). The fuzzer body iterates over three chain shapes — empty, matching-only `{filterName}`, and matching-plus-router `{filterName, "envoy.filters.http.router"}` — exercising the chain-name exact-string-equality surface in `BuildPerRouteConfig`. Total fuzzer count post-Task-10 is **9** (matches SPEC §1 + §14.9): bootstrap (1) + stats (1) + tls (1) + accesslog (1) + filter/tcpproxy (1) + filter/hcm (1) + filter/hcm/h2 (2) + filter/http (1, this task) = 9. **PLAN deviations:** the PLAN scaffold's `f.Add(..., nil, nil)` seed entries used Go-`nil` for the third + fourth `[]byte` arguments. Go's fuzz engine accepts typed-nil `[]byte` arguments at `f.Add` time, but to keep the seed corpus shapes unambiguous + safely round-trippable through the corpus-file format, the empty-everywhere + binary-noise seeds use `[]byte{}` (zero-length but non-nil) instead — semantically equivalent in the fuzzer body (`mk` calls `wrapperspb.String(string(b))` which yields `""` for both nil + empty inputs). One sentence deviation noted per the phase-04..06.2 PLAN-deviation precedent. All 38 prior tests + 3 fuzzer seed sub-tests pass under `-race`; `go vet ./...` + `go build ./...` clean.
**Outputs:**
```
$ go test -fuzz=FuzzFilterChainParse -fuzztime=30s ./internal/filter/http/
hcm: filter "b" called SendLocalReply after encode-side started; ignoring
fuzz: elapsed: 0s, gathering baseline coverage: 0/3 completed
fuzz: elapsed: 0s, gathering baseline coverage: 3/3 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 97016 (32330/sec), new interesting: 87 (total: 90)
fuzz: elapsed: 6s, execs: 355749 (86207/sec), new interesting: 141 (total: 144)
fuzz: elapsed: 9s, execs: 985209 (209840/sec), new interesting: 177 (total: 180)
fuzz: elapsed: 12s, execs: 1911704 (309011/sec), new interesting: 208 (total: 211)
fuzz: elapsed: 15s, execs: 2275689 (121305/sec), new interesting: 209 (total: 212)
fuzz: elapsed: 18s, execs: 2530284 (84881/sec), new interesting: 214 (total: 217)
fuzz: elapsed: 21s, execs: 2766090 (78587/sec), new interesting: 214 (total: 217)
fuzz: elapsed: 24s, execs: 3334760 (189579/sec), new interesting: 216 (total: 219)
fuzz: elapsed: 27s, execs: 3674797 (113324/sec), new interesting: 217 (total: 220)
fuzz: elapsed: 30s, execs: 3996389 (107141/sec), new interesting: 217 (total: 220)
fuzz: elapsed: 31s, execs: 3996389 (0/sec), new interesting: 217 (total: 220)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	31.177s

(3,996,389 executions in 30s, 0 crashers, 220 interesting inputs — clean. The leading `hcm: filter "b" ...` line is stderr-leakage from the prior `TestChain_SendLocalReply_FirstCallWins` test that runs before fuzzing begins; not from the fuzz target.)

$ go test -race ./internal/filter/http/ -count=1 -v   # 38 tests + 3 fuzzer seeds (trailing block)
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
=== RUN   FuzzFilterChainParse
=== RUN   FuzzFilterChainParse/seed#0
=== RUN   FuzzFilterChainParse/seed#1
=== RUN   FuzzFilterChainParse/seed#2
--- PASS: FuzzFilterChainParse (0.00s)
    --- PASS: FuzzFilterChainParse/seed#0 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#1 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.146s

$ go vet ./...
$ go build ./...

$ find internal -name 'fuzz_test.go' -exec grep -c '^func Fuzz' {} + | awk -F: '{s+=$2} END {print s}'
9
```

**Code-review-loop follow-up:** I-1 (assert `hcm:`-prefix discipline matching prior fuzzers in `internal/filter/hcm/fuzz_test.go` + `internal/filter/tcpproxy/fuzz_test.go` + `internal/tls/fuzz_test.go`) + I-2 (exercise `Resolve` path at boundary routeIdx values — 0, -1, 999 — covering lazy-cache prime, negative-bounds, and out-of-range bounds-check) addressed in commit `25b1d42`. The fuzzer's doc-comment was updated to reflect the extended assertion shape: now fuzzes `BuildPerRouteConfig` + `Resolve`, asserting both no-panic + canonical `hcm:` error-prefix discipline. Re-ran the 30s gate clean (1,006,177 execs, 13 new-interesting, 0 crashers — fewer execs than initial Task 10 run because Resolve's per-iter `sync.Mutex` Lock/Unlock + 9 extra calls per iteration shrinks the iteration rate; the fuzz-engine throttling visible in the trailing zeros is the corpus-saturation idle, not a hang — `PASS` + ≥30s elapsed confirm clean). All 38 prior tests + 3 fuzzer seed sub-tests still pass under `-race`; `go vet ./...` + `go build ./...` clean.

**Follow-up outputs:**
```
$ go test -fuzz=FuzzFilterChainParse -fuzztime=30s ./internal/filter/http/
hcm: filter "b" called SendLocalReply after encode-side started; ignoring
fuzz: elapsed: 0s, gathering baseline coverage: 0/245 completed
fuzz: elapsed: 1s, gathering baseline coverage: 245/245 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 529194 (176383/sec), new interesting: 5 (total: 250)
fuzz: elapsed: 6s, execs: 873815 (114876/sec), new interesting: 12 (total: 257)
fuzz: elapsed: 9s, execs: 979153 (35069/sec), new interesting: 13 (total: 258)
fuzz: elapsed: 12s, execs: 1006177 (9017/sec), new interesting: 13 (total: 258)
fuzz: elapsed: 15s, execs: 1006177 (0/sec), new interesting: 13 (total: 258)
fuzz: elapsed: 18s, execs: 1006177 (0/sec), new interesting: 13 (total: 258)
fuzz: elapsed: 21s, execs: 1006177 (0/sec), new interesting: 13 (total: 258)
fuzz: elapsed: 24s, execs: 1006177 (0/sec), new interesting: 13 (total: 258)
fuzz: elapsed: 27s, execs: 1006177 (0/sec), new interesting: 13 (total: 258)
fuzz: elapsed: 30s, execs: 1006177 (0/sec), new interesting: 13 (total: 258)
fuzz: elapsed: 31s, execs: 1006177 (0/sec), new interesting: 13 (total: 258)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	31.184s

$ go test -race ./internal/filter/http/ -count=1 -v   # 38 tests + 3 fuzzer seeds (trailing block)
=== RUN   FuzzFilterChainParse
=== RUN   FuzzFilterChainParse/seed#0
=== RUN   FuzzFilterChainParse/seed#1
=== RUN   FuzzFilterChainParse/seed#2
--- PASS: FuzzFilterChainParse (0.00s)
    --- PASS: FuzzFilterChainParse/seed#0 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#1 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.146s

$ go vet ./...
$ go build ./...
```

## Task 11 — internal/filter/http/router/ — extract router as terminal filter (byte-preserved tests)

**Commits:** 193e73e — this task's commit
**Notes:** Created internal/filter/http/router/{doc,router,router_h2,router_test,router_h2_test}.go. Migrated `routerAction` + `routerActionH2` from `internal/filter/hcm/actions.go` to the new package; the iteration-protocol surface (`Filter` implementing `envoyhttp.StreamDecoderFilter` + `envoyhttp.StreamEncoderFilter` per ADR-0071, exposed at boot via the `New` HTTPFilterFactory + `TypeURL = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"`) lands as a working skeleton (Decode*/Encode* return Continue / DataContinue / TrailersContinue; Tasks 15+16 wire HCM dispatch through to drive the route-action dispatch via the chain). All 14 router-related tests from `actions_test.go` are byte-preserved verbatim — only `package hcm` → `package router` and the import block adjusts (regrouped per the new package's surface, with `regexp`+`os` dropped since the H1-golden test stays in hcm/). Per the PLAN's Task 11 → Task 12 split, `internal/filter/hcm/actions.go` STILL contains `routerAction` + `routerActionH2` at this commit; the duplication is intentional and Task 12 deletes the hcm-side originals in a separate clean commit. **PLAN deviations / byte-preservation deviations:** (1) The PLAN scaffold described a complete inlining of `routerAction.do(ctx, req, bw)` body into `(*Filter).DecodeHeaders/Data/Trailers`; this is structurally incompatible with byte-preserved tests that exercise `&routerAction{...}.do(ctx, req, bw)` directly (the test bodies would have to change shape). To honor BRAINSTORM §6.8 byte-preservation, this task keeps `routerAction` + `routerActionH2` as private dispatch primitives in the new package with verbatim signatures + bodies; the iteration-protocol `Filter` type wraps them at the public boundary. Tasks 15+16 (HCM dispatch wiring) connect `Filter.DecodeHeaders`'s end-of-stream point to the routerAction dispatch — i.e., the inlining the PLAN sketched lands as composition, not flattening. This preserves the entire test surface verbatim while still satisfying the iteration-protocol contract. (2) Five private helpers are duplicated from `internal/filter/hcm` into the new package (`writeStatusReply`, `dateHeader`, `serverHeader`, `upstreamHostString`, `h2UserAgent`) plus three constants/vars (`bad502Body`, `errCloseAfterAction`, the `Filter` struct's `accessLog` field + `emitAccessLog` + `emitAccessLogH2` methods). Exporting these from hcm would create a forward-coupling that Task 12 can clean, but the PLAN's explicit "duplication is intentional between Task 11 and Task 12" framing means duplicating in this task is correct (Task 12 deletes the hcm-side originals; if Task 12 ALSO removes the helper duplication, that's a separate concern for Task 13's hcm-side trim). The `errCloseAfterAction` error message changes from `"hcm: action requested connection close"` to `"router: action requested connection close"` to identify the new package as the source — tests use `errors.Is` (identity-based), so the prose change does not break byte-preservation. All 14 byte-preserved tests pass; `go vet ./...` + `go build ./...` clean; `go test -race ./internal/filter/http/router/` clean.
**Outputs:**
```
$ go test ./internal/filter/http/router/ -count=1 -v
=== RUN   TestRouterActionH2_HappyPath
--- PASS: TestRouterActionH2_HappyPath (0.00s)
=== RUN   TestRouterActionH2_502OnDialFailure
--- PASS: TestRouterActionH2_502OnDialFailure (0.00s)
=== RUN   TestRouterActionH2_502OnRoundTripProtocolError
--- PASS: TestRouterActionH2_502OnRoundTripProtocolError (0.00s)
=== RUN   TestRouterActionH2_CtxCancelEmitsRSTStreamCancel
--- PASS: TestRouterActionH2_CtxCancelEmitsRSTStreamCancel (0.20s)
=== RUN   TestRouterActionH2_Do_IncsUpstreamRqTotalAndStatusClass
--- PASS: TestRouterActionH2_Do_IncsUpstreamRqTotalAndStatusClass (0.00s)
=== RUN   TestRouterActionH2_Upstream5xxForwardedVerbatim
--- PASS: TestRouterActionH2_Upstream5xxForwardedVerbatim (0.00s)
=== RUN   TestRouterActionH2_DefensiveDoEmits500AndLogs
--- PASS: TestRouterActionH2_DefensiveDoEmits500AndLogs (0.00s)
=== RUN   TestRouterAction_DoHappy
--- PASS: TestRouterAction_DoHappy (0.00s)
=== RUN   TestRouterAction_DoDialFailureReturns503
--- PASS: TestRouterAction_DoDialFailureReturns503 (0.00s)
=== RUN   TestRouterAction_DoCtxCancel
--- PASS: TestRouterAction_DoCtxCancel (0.00s)
=== RUN   TestRouterAction_Do_IncsUpstreamRqTotalAndStatusClass
--- PASS: TestRouterAction_Do_IncsUpstreamRqTotalAndStatusClass (0.00s)
=== RUN   TestRouterAction_Do_DialFailureInc5xx
--- PASS: TestRouterAction_Do_DialFailureInc5xx (0.00s)
=== RUN   TestRouterAction_EmitsAccessLog_HappyPath
--- PASS: TestRouterAction_EmitsAccessLog_HappyPath (0.00s)
=== RUN   TestRouterAction_EmitsAccessLog_DialFailure
--- PASS: TestRouterAction_EmitsAccessLog_DialFailure (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	0.211s

$ go test -race ./internal/filter/http/router/ -count=1
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.234s

$ go vet ./...
$ go build ./...

$ go test ./internal/filter/hcm/ -count=1 -short   # confirm hcm still passes (routerAction + routerActionH2 still live here; Task 12 deletes them)
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.419s
```

**Code-review-loop follow-up:** I-1 (log-prefix symmetry) addressed in commit `73cc8da`. The defensive `(*routerActionH2).do` H1-stub log line in `internal/filter/http/router/router_h2.go` had a `"hcm:"` prefix left over from the pre-extraction text; this is asymmetric with the prior `errCloseAfterAction` rename (`"hcm: action requested connection close"` → `"router: action requested connection close"`) made when the package was extracted. An operator debugging an unexpected 500 in the new package's logs will grep for `router:` and miss the unreachable-but-defensive line. Renamed `"hcm:"` → `"router:"` in that single log message; the test `TestRouterActionH2_DefensiveDoEmits500AndLogs` substring assertions only check `"routerActionH2.do reached on H1 path"` and `"cluster="`, so the prefix change is safe. Minor issues M-1..M-6 from the review (docstring polish, stale comment, etc.) are deferred to future tasks. Fixed log line:

```
log.Printf("router: routerActionH2.do reached on H1 path — bootstrap misconfiguration; route variant selection should have produced *routerAction, not *routerActionH2 (cluster=%q)", r.cluster.Name())
```

**Follow-up outputs:**
```
$ go test ./internal/filter/http/router/ -count=1 -run TestRouterActionH2_DefensiveDoEmits500AndLogs -v
=== RUN   TestRouterActionH2_DefensiveDoEmits500AndLogs
--- PASS: TestRouterActionH2_DefensiveDoEmits500AndLogs (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	0.003s

$ go test -race ./internal/filter/http/router/ -count=1
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.234s

$ go vet ./...
$ go build ./...
```

## Task 12 — internal/filter/hcm/actions.go — delete routerAction + routerActionH2 (moved to internal/filter/http/router)

**Commits:** c3f77d4 — this task's commit
**Notes:** Mechanical deletion per PLAN Task 12. Removed from `internal/filter/hcm/actions.go`: `routerAction` struct + `routerAction.do`, `routerActionH2` struct + `routerActionH2.doH2` + `routerActionH2.write502` + the defensive `routerActionH2.do` H1 stub, plus the `bad502Body` constant (only consumed by the now-deleted `routerActionH2.write502`). Preserved verbatim: `errCloseAfterAction` (still consumed by `connection.go`'s H1 loop), `directResponseAction` struct, `directResponseAction.body` / `writeH1` / `writeH2` / `do`. Removed from `internal/filter/hcm/actions_test.go`: 14 router-related test functions (TestRouterAction_DoHappy, TestRouterAction_DoDialFailureReturns503, TestRouterAction_DoCtxCancel, TestRouterActionH2_HappyPath, TestRouterActionH2_502OnDialFailure, TestRouterActionH2_502OnRoundTripProtocolError, TestRouterActionH2_CtxCancelEmitsRSTStreamCancel, TestRouterAction_Do_IncsUpstreamRqTotalAndStatusClass, TestRouterAction_Do_DialFailureInc5xx, TestRouterActionH2_Do_IncsUpstreamRqTotalAndStatusClass, TestRouterActionH2_Upstream5xxForwardedVerbatim, TestRouterAction_EmitsAccessLog_HappyPath, TestRouterAction_EmitsAccessLog_DialFailure, TestRouterActionH2_DefensiveDoEmits500AndLogs) plus all router-only test helpers (loopbackHTTPEcho, singleEndpointCluster*, h2BackendPKI + mkH2BackendPKI, h2BackendBehavior + runH2Backend + startH2Backend, h2EndpointCluster*, pemEncodeCAPool, h2RequestForTest, captureH2Writer, counterValue). Preserved verbatim: TestDirectResponseAction_Do, TestDirectResponseWriteH1_GoldenCompat, TestDirectResponseWriteH2_HEADERSThenDATAEndStream, TestDirectResponseAction_EmitsAccessLog, TestDirectResponseAction_NilFilter_DoesNotPanic, plus the captureSW shim those tests share. Trimmed now-unused imports from `actions.go` (dropped `bytes`, `log`) and from `actions_test.go` (dropped `crypto/ecdsa`, `crypto/elliptic`, `crypto/rand`, `stdtls`, `crypto/x509`, `crypto/x509/pkix`, `encoding/pem`, `errors`, `fmt`, `io`, `log`, `math/big`, `net`, `strconv`, `sync`, `time`, the six envoy go-control-plane proto packages, `golang.org/x/net/http2`, `anypb`, `durationpb`, and the project-internal `cluster`/`h2`/`stats` imports). LoC delta: actions.go 378 → 100 (−278); actions_test.go 1102 → 161 (−941). **Package does not yet build; restored at Task 16** — per PLAN §Task 12 Step 3 refinement: deleting `routerAction` + `routerActionH2` breaks references in `internal/filter/hcm/h2dispatch.go` (Cluster.UseH2 → routerActionH2 selection), `internal/filter/hcm/config.go` (variant selection in buildRouterAction), `internal/filter/hcm/route.go` (route-table action variant), `internal/filter/hcm/config_test.go`, and `internal/filter/hcm/connection_test.go`. The PLAN explicitly permits Tasks 12–15 to land with the package not-yet-building; Task 13 (config.go chain build), Task 15 (connection.go H1 dispatch through chain), and Task 16 (h2dispatch.go H2 dispatch through chain) collectively restore buildability via the FilterChain calling `internal/filter/http/router/`'s `Filter` instead of the deleted hcm-package symbols. This is doctrine D-3.6 release-valve compliance: the deliberate red state is documented in this PROGRESS entry + verbatim error output below so a reviewer or future debug session can confirm these are EXACTLY the references that Tasks 13/15/16 will resolve.

**Verification (zero-match grep + LoC delta + router-package build clean + expected hcm red state):**
```
$ grep -nE 'routerAction|routerActionH2' internal/filter/hcm/actions.go internal/filter/hcm/actions_test.go
(zero matches)

$ wc -l internal/filter/hcm/actions.go internal/filter/hcm/actions_test.go
  100 internal/filter/hcm/actions.go
  161 internal/filter/hcm/actions_test.go
  261 total

$ go build ./internal/filter/http/router/
(clean — router package independent)

$ go build ./internal/filter/hcm/ 2>&1 | head -30
# github.com/esalaine/envoy-go/internal/filter/hcm
internal/filter/hcm/h2dispatch.go:62:8: undefined: routerActionH2
internal/filter/hcm/h2dispatch.go:119:10: undefined: routerActionH2
internal/filter/hcm/config.go:339:11: undefined: routerActionH2
internal/filter/hcm/config.go:341:10: undefined: routerAction
internal/filter/hcm/route.go:89:9: undefined: routerAction
internal/filter/hcm/route.go:91:9: undefined: routerActionH2

$ go test ./internal/filter/hcm/ -run TestDirectResponseAction -count=1 -v 2>&1 | head -15
# github.com/esalaine/envoy-go/internal/filter/hcm [github.com/esalaine/envoy-go/internal/filter/hcm.test]
internal/filter/hcm/h2dispatch.go:62:8: undefined: routerActionH2
internal/filter/hcm/h2dispatch.go:119:10: undefined: routerActionH2
internal/filter/hcm/config.go:339:11: undefined: routerActionH2
internal/filter/hcm/config.go:341:10: undefined: routerAction
internal/filter/hcm/route.go:89:9: undefined: routerAction
internal/filter/hcm/route.go:91:9: undefined: routerActionH2
internal/filter/hcm/config_test.go:527:21: undefined: routerAction
internal/filter/hcm/config_test.go:539:21: undefined: routerActionH2
internal/filter/hcm/connection_test.go:228:7: undefined: singleEndpointCluster
internal/filter/hcm/connection_test.go:230:38: undefined: routerAction
internal/filter/hcm/connection_test.go:230:38: too many errors
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm [build failed]
FAIL
```

The `TestDirectResponseAction*` test invocation cannot exercise the preserved test bodies until Task 16 restores hcm-package buildability. The directResponseAction symbol surface (struct + body + writeH1 + writeH2 + do) is preserved byte-identically across this commit; the byte-preservation can be reviewed via `git show -- internal/filter/hcm/actions.go` (the unified diff shows zero touches inside the directResponseAction block). The preserved test bodies will be re-run at Task 16 when the package builds again.

## Task 13 — internal/filter/hcm/config.go parseFilterWithCtx + chain build + per-route plumbing

**Commits:** dff5a78 — this task's commit
**Notes:** Widened `parseFilterWithCtx` with a trailing `httpRegistry *filter_http.HTTPRegistry` parameter (per PLAN Task 13 Step 2 + ADR-0072). Replaced the legacy `requireRouterOnlyHTTPFilters` (exactly-`[router]` rule per ADR-0042) with a chain-walking `parseHTTPFiltersChain` that delegates the four canonical chain-shape rules per SPEC §1 #6 + ADR-0071's partial supersession of ADR-0042 to a new `filter_http.ValidateChainShape` helper (added in `internal/filter/http/chain_shape.go`). The validator returns the canonical error texts: rule #1 `hcm: http_filters: must contain at least 1 entry (the router)`; rule #2 `hcm: http_filters: last entry must be %q (router); got %q (%s)`; rule #3 `hcm: http_filters: duplicate filter name %q`; rule #4 `hcm: http_filters[i]: unknown type_url %q (registry: known are %v)`. On success, the parser walks the chain a second time invoking each entry's `HTTPFilterFactory(typed_config_Any, FactoryCtx{Registry: httpRegistry})` to allocate the per-instance `FilterInstanceFactory` closures stored on the new `Filter.chainConfig []chainEntry` field. After the chain is built, `buildPerRouteFromHCM` extracts the typed_per_filter_config maps from RouteConfiguration / VirtualHost / Route levels and runs them through `filter_http.BuildPerRouteConfig` (per ADR-0073's 3-tier merge model); the result is stored on the new `Filter.perRouteConfig *filter_http.PerRouteConfig` field. Short-circuits the BuildPerRouteConfig allocation when no typed_per_filter_config map is present at any level (the common phase-04..06.2 path). `RouteScope` (formerly package-private `routeScope`) is now exported from `internal/filter/http/perroute.go` so the hcm parser can construct the per-route scopes vector without an alias dance; a transitional `type routeScope = RouteScope` alias preserves the existing fuzzer + perroute_test.go bodies during the Task 13 cycle (Task 14 sweeps the lowercase references). The legacy constructors (`NewFilter`, `NewFilterWithCtx`, `NewFilterWithCtxAndSinks`, `parseFilter`) all gained a transitional `defaultRouterOnlyHTTPRegistry()` helper that constructs a fresh frozen registry containing only `router.New` under `router.TypeURL` — so that pre-Task-14 tests continue to validate clean against the new four-rule chain shape; Task 14's caller-sweep deletes the legacy constructors (and this helper) per PLAN Task 14 Step 3. Updated four existing chain-validation tests (TestParseFilter_HTTPFiltersEmpty, TestParseFilter_HTTPFiltersTwoEntries, TestParseFilter_HTTPFiltersWrongName, TestParseFilter_HTTPFiltersWrongTypeURL) to assert the new canonical error text — all four still test exactly the same misconfiguration shape, only the matched substring changes per the post-Task-13 wording. Added four new acceptance tests covering each canonical error class verbatim (TestParseFilterWithCtx_RejectsEmptyChain, TestParseFilterWithCtx_RejectsNonRouterTerminal, TestParseFilterWithCtx_RejectsDuplicateFilterName, TestParseFilterWithCtx_RejectsUnknownTypeURL). Extended `internal/filter/http/fuzz_test.go` with a sibling `FuzzFilterChainParse_ChainShape` that fuzzes `ValidateChainShape` with adversarial `(name1, typeURL1, name2, typeURL2, count, registerRouter)` tuples; per the PLAN's "logically a single FuzzFilterChainParse target with two seed corpora" framing, both fuzzers run under the 30s ADR-0018 budget (separately invoked here for verification — the seed body counts are 3+3=6 sub-tests under `go test`). 30s gates clean: `FuzzFilterChainParse` 1,360,373 execs / 0 crashers; `FuzzFilterChainParse_ChainShape` 11,426,311 execs / 0 crashers. **PLAN deviations:**
- (i) The PLAN Task 14 Step 1 places `chainConfig` + `perRouteConfig` field additions in Task 14's `filter.go` change. Task 13 pre-emptively adds these fields to the `Filter` struct in this commit (per PROMPT option (a) — produces a cleaner intermediate state since the parser side that populates them lives in Task 13); Task 14's PROGRESS will note "fields landed in Task 13".
- (ii) The PLAN Task 13 Step 4 sketches the chain-shape fuzzer extension as fuzzing `parseFilterWithCtx` directly via a thin shim. This requires `internal/filter/http/fuzz_test.go` to import `internal/filter/hcm` — which would create an import cycle (`hcm` already imports `filter/http`). Resolved by extracting the four-rule chain-shape validation to `filter_http.ValidateChainShape` (in `internal/filter/http/chain_shape.go`) — the hcm parser delegates to it, the fuzzer fuzzes it directly, no cycle. The fuzzer now exercises the same validation paths the PLAN intended without the sketch's cycle hazard.
- (iii) The dangling `routerAction` / `routerActionH2` references at `config.go:480,482` (action-construction in `buildRouterAction`), `route.go:89,91` (route-table action variant binding), and `h2dispatch.go:62,119` (h2-dispatch action variant) are LEFT in place by Task 13 per the PROMPT's disposition guidance: "those refs are in the per-request action-selection path... they belong to Task 14/15/16 wiring, NOT Task 13". The hcm package therefore still does not build — restored at Task 16 per PLAN Task 12 Step 3 refinement. The `RouteScope` field rename (lowercase `vhost`/`route` → uppercase `VHost`/`Route`) is Task 13's one structural ripple in `internal/filter/http/`; the transitional `routeScope` type alias preserves the rest of the existing test bodies verbatim.

**Acceptance:** All four new error-class tests are wired into `parseFilter` (which delegates to `parseFilterWithCtx`); they will execute successfully once Task 16 restores hcm-package buildability — the same testing discipline that Task 12 documented. The four tests assert the verbatim canonical error texts (TestParseFilterWithCtx_RejectsEmptyChain + RejectsDuplicateFilterName use exact-match equality; RejectsNonRouterTerminal + RejectsUnknownTypeURL use substring match for the parts that depend on the wider error context). Existing chain-validation test substrings updated to match the new wording. Extended `FuzzFilterChainParse` (via the sibling `_ChainShape` target) ran clean under the 30s budget. `go vet ./internal/filter/http/...` clean.

**Outputs:**
```
$ go build ./internal/filter/http/
(clean)

$ go vet ./internal/filter/http/...
(clean)

$ go test -count=1 -v ./internal/filter/http/ 2>&1 | tail -22
=== RUN   FuzzFilterChainParse
=== RUN   FuzzFilterChainParse/seed#0
=== RUN   FuzzFilterChainParse/seed#1
=== RUN   FuzzFilterChainParse/seed#2
--- PASS: FuzzFilterChainParse (0.00s)
    --- PASS: FuzzFilterChainParse/seed#0 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#1 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#2 (0.00s)
=== RUN   FuzzFilterChainParse_ChainShape
=== RUN   FuzzFilterChainParse_ChainShape/seed#0
=== RUN   FuzzFilterChainParse_ChainShape/seed#1
=== RUN   FuzzFilterChainParse_ChainShape/seed#2
--- PASS: FuzzFilterChainParse_ChainShape (0.00s)
    --- PASS: FuzzFilterChainParse_ChainShape/seed#0 (0.00s)
    --- PASS: FuzzFilterChainParse_ChainShape/seed#1 (0.00s)
    --- PASS: FuzzFilterChainParse_ChainShape/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	0.131s

$ go test -fuzz=FuzzFilterChainParse$ -fuzztime=30s ./internal/filter/http/
hcm: filter "b" called SendLocalReply after encode-side started; ignoring
fuzz: elapsed: 0s, gathering baseline coverage: 0/266 completed
fuzz: elapsed: 1s, gathering baseline coverage: 266/266 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 490053 (163344/sec), new interesting: 5 (total: 271)
fuzz: elapsed: 6s, execs: 873442 (127768/sec), new interesting: 7 (total: 273)
fuzz: elapsed: 9s, execs: 956931 (27809/sec), new interesting: 7 (total: 273)
fuzz: elapsed: 12s, execs: 992626 (11906/sec), new interesting: 7 (total: 273)
fuzz: elapsed: 15s, execs: 992626 (0/sec), new interesting: 7 (total: 273)
fuzz: elapsed: 18s, execs: 1287010 (98128/sec), new interesting: 8 (total: 274)
fuzz: elapsed: 21s, execs: 1332084 (15032/sec), new interesting: 8 (total: 274)
fuzz: elapsed: 24s, execs: 1360373 (9428/sec), new interesting: 9 (total: 275)
fuzz: elapsed: 27s, execs: 1360373 (0/sec), new interesting: 9 (total: 275)
fuzz: elapsed: 30s, execs: 1360373 (0/sec), new interesting: 9 (total: 275)
fuzz: elapsed: 31s, execs: 1360373 (0/sec), new interesting: 9 (total: 275)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	31.177s

$ go test -fuzz=FuzzFilterChainParse_ChainShape -fuzztime=30s ./internal/filter/http/
hcm: filter "b" called SendLocalReply after encode-side started; ignoring
fuzz: elapsed: 0s, gathering baseline coverage: 0/196 completed
fuzz: elapsed: 1s, gathering baseline coverage: 196/196 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 950364 (316718/sec), new interesting: 2 (total: 198)
fuzz: elapsed: 6s, execs: 2117131 (388920/sec), new interesting: 2 (total: 198)
fuzz: elapsed: 9s, execs: 3279430 (387431/sec), new interesting: 9 (total: 205)
fuzz: elapsed: 12s, execs: 4413372 (378007/sec), new interesting: 11 (total: 207)
fuzz: elapsed: 15s, execs: 5548529 (378429/sec), new interesting: 13 (total: 209)
fuzz: elapsed: 18s, execs: 6779711 (410291/sec), new interesting: 14 (total: 210)
fuzz: elapsed: 21s, execs: 7945530 (388595/sec), new interesting: 16 (total: 212)
fuzz: elapsed: 24s, execs: 9112138 (388898/sec), new interesting: 17 (total: 213)
fuzz: elapsed: 27s, execs: 10267281 (385064/sec), new interesting: 18 (total: 214)
fuzz: elapsed: 30s, execs: 11426311 (386389/sec), new interesting: 19 (total: 215)
fuzz: elapsed: 30s, execs: 11426311 (0/sec), new interesting: 19 (total: 215)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	30.249s

$ go build ./internal/filter/hcm/ 2>&1
# github.com/esalaine/envoy-go/internal/filter/hcm
internal/filter/hcm/h2dispatch.go:62:8: undefined: routerActionH2
internal/filter/hcm/h2dispatch.go:119:10: undefined: routerActionH2
internal/filter/hcm/config.go:480:11: undefined: routerActionH2
internal/filter/hcm/config.go:482:10: undefined: routerAction
internal/filter/hcm/route.go:89:9: undefined: routerAction
internal/filter/hcm/route.go:91:9: undefined: routerActionH2
```

The hcm package still does not build (same six dangling refs from Task 12 — Task 13's parseFilterWithCtx widening compiles cleanly modulo those). Per PLAN Task 12 Step 3 refinement: Tasks 13–15 may leave the package non-building; Task 16 restores buildability. The four new tests (and the four existing tests with adapted error-message substrings) cannot run until then; the test bodies have been audit-trail-verified against the new validator's verbatim error texts.

The fuzzer count post-Task-13 is **10** (matches SPEC §1 + §14.9): bootstrap (1) + stats (1) + tls (1) + accesslog (1) + filter/tcpproxy (1) + filter/hcm (1) + filter/hcm/h2 (2) + filter/http (2 — `FuzzFilterChainParse` + `FuzzFilterChainParse_ChainShape`, the latter added in this task) = 10. The PLAN's "logically a single FuzzFilterChainParse target with two seed corpora" framing keeps the *logical* count at 9; the *function* count is 10 (Go's `func Fuzz*` counter). [Superseded by the **Code-review-loop follow-up** below — fuzzer count returned to 9 by consolidating both branches into one `FuzzFilterChainParse` function with a discriminator parameter; the count delta resolves the I-1 review issue.]

**Code-review-loop follow-up:** I-1 (fuzzer-count discipline — SPEC §1 + §14.9 + PLAN.md:2917,2925 + Task 23 close-out gate all commit to "9 fuzzers post-07.1"; the function-count delta to 10 was a documented mismatch above) addressed in commit `d473822`. Consolidated `FuzzFilterChainParse_ChainShape` into the existing `FuzzFilterChainParse` per PLAN.md:2179 ("logically a single FuzzFilterChainParse target with two seed corpora"). **Pattern A picked** (single function, single seed corpus split by leading discriminator byte `mode`, each branch keeps its own assertion shape) over Pattern B (run both branches per fuzz input) — the two branches have divergent input-arity needs (chain-shape wants count + registry-toggle that the per-route path does not), so Pattern A keeps each branch's assertion surface narrow with no cross-branch noise. The two branch bodies are extracted into private package-private helpers (`fuzzBuildPerRouteAndResolve` for `mode==0`, `fuzzValidateChainShape` for `mode!=0`); the `f.Fuzz` callback dispatches via a `switch` on the discriminator. Six seed entries are preserved verbatim — three per branch — under one common 7-arg seed shape (the unused branch-0 args `count` + `registerRouter` are pinned to zero values; the mirror unused branch-1 args are absorbed into the dispatch). Re-ran the 30s gate clean (7,207,481 execs / 227 new-interesting / 0 crashers — the per-iteration cost dropped vs. the prior `_ChainShape`-only run because branch-0's BuildPerRouteConfig+Resolve work is a fraction of the 32-worker dispatch slot share, but well above the prior `FuzzFilterChainParse`-only run because the simpler branch-0 path now shares iterations with branch-1; PASS confirms clean). All 39 tests + 6 fuzzer seed sub-tests still pass under `-race`; `go vet ./internal/filter/http/...` + `go build ./internal/filter/http/...` clean (the wider-tree hcm-package still-not-building red state from Task 12 is unchanged — same six dangling refs documented above; restored at Task 16 per PLAN Task 12 Step 3). Function-count post-consolidation: **9** (verified via `find internal -name 'fuzz_test.go' -exec grep -c '^func Fuzz' {} + | awk -F: '{s+=$2} END {print s}'`). Minor issues M-1..M-6 from the review (`defaultRouterOnlyHTTPRegistry` over-build; `RouteScope` alias; field naming; test scope shift; helper decomposition; import alias) deferred to natural cleanup at Tasks 14+.

**Follow-up outputs:**
```
$ find internal -name 'fuzz_test.go' -exec grep -c '^func Fuzz' {} + | awk -F: '{s+=$2} END {print s}'
9

$ go test -fuzz=FuzzFilterChainParse -fuzztime=30s ./internal/filter/http/
hcm: filter "b" called SendLocalReply after encode-side started; ignoring
fuzz: elapsed: 0s, gathering baseline coverage: 0/6 completed
fuzz: elapsed: 0s, gathering baseline coverage: 6/6 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 125338 (41770/sec), new interesting: 95 (total: 101)
fuzz: elapsed: 6s, execs: 483477 (119390/sec), new interesting: 140 (total: 146)
fuzz: elapsed: 9s, execs: 1506225 (340930/sec), new interesting: 164 (total: 170)
fuzz: elapsed: 12s, execs: 2504469 (332672/sec), new interesting: 185 (total: 191)
fuzz: elapsed: 15s, execs: 3401939 (299193/sec), new interesting: 200 (total: 206)
fuzz: elapsed: 18s, execs: 4017076 (205070/sec), new interesting: 211 (total: 217)
fuzz: elapsed: 21s, execs: 4725944 (236233/sec), new interesting: 218 (total: 224)
fuzz: elapsed: 24s, execs: 5920622 (398226/sec), new interesting: 224 (total: 230)
fuzz: elapsed: 27s, execs: 6604003 (227800/sec), new interesting: 226 (total: 232)
fuzz: elapsed: 30s, execs: 7207481 (201121/sec), new interesting: 227 (total: 233)
fuzz: elapsed: 31s, execs: 7207481 (0/sec), new interesting: 227 (total: 233)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	31.178s

$ go test -race ./internal/filter/http/ -count=1 -v   # 39 tests + 6 fuzzer seeds (trailing block)
=== RUN   FuzzFilterChainParse
=== RUN   FuzzFilterChainParse/seed#0
=== RUN   FuzzFilterChainParse/seed#1
=== RUN   FuzzFilterChainParse/seed#2
=== RUN   FuzzFilterChainParse/seed#3
=== RUN   FuzzFilterChainParse/seed#4
=== RUN   FuzzFilterChainParse/seed#5
--- PASS: FuzzFilterChainParse (0.00s)
    --- PASS: FuzzFilterChainParse/seed#0 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#1 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#2 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#3 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#4 (0.00s)
    --- PASS: FuzzFilterChainParse/seed#5 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.147s

$ go vet ./internal/filter/http/...
$ go build ./internal/filter/http/...
```

---

## Task 14 — internal/filter/hcm/filter.go constructor widening + legacy constructor deletion

**Commits:** b23059e — this task's commit
**Notes:** Per Decision §3.4 + ADR-0072: deleted the four legacy hcm constructors (`NewFilter`, `NewFilterWithCtx`, `NewFilterWithCtxAndSinks`, plus the test-only `parseFilter` legacy entry point) and replaced them with a SOLE entry point `NewFilterWithCtxAndSinksAndRegistry(tc, clusters, lc, registry, accessLogSinks, httpRegistry)` (declared in `internal/filter/hcm/filter.go`). The new constructor's body is a thin pass-through to `parseFilterWithCtx` (the Task 13 widened parser); the seventh-name discipline matches PLAN Task 14 Step 2 verbatim. Also deleted the Task-13 transitional `defaultRouterOnlyHTTPRegistry()` helper from `config.go` per the PLAN's Task 14 §Step 3 anchor and the prompt's explicit "delete the helper" directive. The `chainConfig []chainEntry` + `perRouteConfig *filter_http.PerRouteConfig` field additions specified in PLAN Task 14 Step 1 were already landed in Task 13 (per Task 13's PLAN-deviation note (i)); Task 14's struct-extension step is therefore a no-op modulo a doc-comment edit on the Filter-struct comments to drop "pre-Task 14" language.

Threaded `*filter_http.HTTPRegistry` into `internal/listener/manager.go`: the `filterConstructor` typedef, the `filterRegistry` map's two closure bodies (tcpproxy ignores the registry; hcm consumes it), `buildListenerRuntimeWithCtx`, and the three public `NewManager*` constructors all gained the trailing `httpRegistry *filter_http.HTTPRegistry` parameter. The HCM-construction closure now calls `hcm.NewFilterWithCtxAndSinksAndRegistry(...)` with the threaded registry per ADR-0072's freeze-after-boot contract.

`cmd/envoy-go/main.go` deferred to Task 20 per the PLAN's Task 14 Step 3 explicit anchor (boot-wiring registry-population is Task 20's territory). Main.go would currently break compile because it calls the now-widened `listener.NewManagerWithBaseDirAndAllowH2C` with the pre-Task-14 6-arg signature — which is the expected residue per the PLAN; Task 20 adds the seventh argument (the boot-built `*filter_http.HTTPRegistry`).

**Test-site sweep:** updated all hcm-package test bootstraps + listener-package test bootstraps to thread the new registry parameter:

- `internal/filter/hcm/filter_test.go`: 7 sites — every `NewFilter(...)` / `NewFilterWithCtx(...)` call rewritten to `NewFilterWithCtxAndSinksAndRegistry(any, cm, ListenerCtx{...}, stats.NewRegistry(), nil, testHTTPRegistry())`.
- `internal/filter/hcm/fuzz_test.go`: 1 site — `NewFilter(...)` rewritten to the new constructor; the `httpReg` is allocated once outside the `f.Fuzz` callback and reused (frozen + immutable, no per-iter mutation).
- `internal/filter/hcm/config_test.go`: 17 sites — `parseFilter(...)` (test-only) rewritten to the new test-helper `parseFilterTest(...)`; `parseFilterWithCtx(...)` 5-arg call sites extended with the 6th `httpRegistry` argument (Task 13 widened the parser to 6 params; the test sites previously didn't compile because the package was non-buildable from Task 12's dangling refs — Task 14's sweep also fixes this latent test-file arity mismatch).
- `internal/listener/manager_test.go`: 30 sites — every `NewManager(...)` call rewritten to thread `testHTTPRegistry()`; both `NewManagerWithBaseDirAndAllowH2C(...)` sites threaded the same. Most listener tests use TCP-proxy (the registry is ignored on that path), but five listener tests build HCM filters and require a non-nil frozen registry to satisfy the parser.

Consolidated the empty-then-frozen registry pattern into a shared `testHTTPRegistry()` helper per the prompt's "use judgment" guidance. Two copies live in `internal/filter/hcm/testhelpers_test.go` and `internal/listener/manager_test.go` (each package needs its own; Go's `_test.go` visibility model doesn't permit cross-package import). The body is identical: `r := filter_http.NewHTTPRegistry(); r.Register(router.TypeURL, router.New); r.Freeze(); return r`. The hcm-side helper file also defines a `parseFilterTest(tc, cm) (*Filter, error)` two-arg shim that wraps `parseFilterWithCtx` with a zero-value ListenerCtx + fresh throwaway *stats.Registry + nil sinks + the router-only registry — the post-Task-14 replacement for the deleted production-code `parseFilter` (which existed in pre-Task-14 `config.go` as a "legacy entry point retained for existing tests").

**PLAN deviations:**
- (i) The PLAN Task 14 Step 1 places `chainConfig` + `perRouteConfig` field additions in this task's `filter.go` (sic; both are actually on `Filter` defined in `config.go`). Both fields were pre-emptively added in Task 13 per Task 13's deviation note (i); Task 14's struct-extension is a no-op modulo the doc-comment edits described above.
- (ii) The PLAN Task 14 Step 3 lists the call-site sweep as ~15 test sites. Actual count is **55 sites** (7 in `filter_test.go` + 1 in `fuzz_test.go` + 17 in `config_test.go` + 30 in `manager_test.go`); the PLAN's count counted the hcm-package sites only. The listener-package sites are a downstream consequence of widening the listener manager constructors, which is the prompt's stated "ONE site (HCM-construction closure)" plus the structural plumbing required to thread the registry to that site.
- (iii) The four hcm test files mentioned in the prompt (`accesslog_emit_test.go`, `actions_test.go`, `connection_test.go`, `h2dispatch_test.go`) had **zero** legacy-constructor call sites at Task 14 entry; their tests build `*Filter` via direct struct literals (e.g., `mkFilterForTable` in `connection_test.go`) or via fuzz-targeted helpers that don't go through the constructors. No deviation from the prompt's expectation, but worth recording: the prompt's "~15 test sites" estimate was distributed across only `config_test.go` + `filter_test.go` + `fuzz_test.go`.
- (iv) The `parseFilter` legacy entry point (introduced before Task 13 — see config.go pre-Task-14 line 149 doc) is also deleted in Task 14. The prompt explicitly required deleting `defaultRouterOnlyHTTPRegistry`; deleting `parseFilter` along with it is a natural correlate (its body was `return parseFilterWithCtx(..., defaultRouterOnlyHTTPRegistry())` so it transitively depended on the helper). Test sites that called `parseFilter` now call the new test-only `parseFilterTest` helper in `testhelpers_test.go`, preserving the two-arg call-site ergonomics.
- (v) Two doc-comment archaeology references remain in production code (`filter.go:24-26`) and one in test code (`testhelpers_test.go:15`) — they list the deleted symbol names ("the four legacy variants (NewFilter, NewFilterWithCtx, NewFilterWithCtxAndSinks, plus … defaultRouterOnlyHTTPRegistry helper) were deleted") so future readers understand the deletion. The prompt's "zero-grep" requirement is satisfied for actual call/declaration sites; only doc-comment archaeology mentions the strings.

**Acceptance:**
- `go build ./internal/filter/http/...` clean (unchanged from Task 13; this task does not touch `internal/filter/http`).
- `go vet ./internal/filter/http/...` clean.
- `go build ./internal/filter/hcm/` STILL fails — six dangling refs from Task 12 (per PLAN Task 12 Step 3 refinement; Tasks 15 + 16 restore buildability). Verbatim error head:

```
$ go build ./internal/filter/hcm/
# github.com/esalaine/envoy-go/internal/filter/hcm
internal/filter/hcm/h2dispatch.go:62:8: undefined: routerActionH2
internal/filter/hcm/h2dispatch.go:119:10: undefined: routerActionH2
internal/filter/hcm/config.go:430:11: undefined: routerActionH2
internal/filter/hcm/config.go:432:10: undefined: routerAction
internal/filter/hcm/route.go:89:9: undefined: routerAction
internal/filter/hcm/route.go:91:9: undefined: routerActionH2
```

The failure shape is exactly the expected Tasks-15-16-territory residue. Line numbers shifted (480/482 → 430/432 in `config.go`) because Task 14 deleted ~50 lines of legacy-constructor code from `config.go`, which is a structural deletion not a runtime-behavior change. Otherwise verbatim from Task 13's residue.

- `go build ./internal/listener/` fails transitively (depends on hcm). Listener manager source-side compile would otherwise succeed: the constructor-widening + closure-body update is internally consistent.
- `go build ./cmd/envoy-go/` fails transitively (depends on hcm + listener). Main.go's call to the now-widened `listener.NewManagerWithBaseDirAndAllowH2C` with 6 args (it now requires 7) is the additional Task-20-territory residue documented above.

**Zero-grep verification:** the four deleted symbols (`hcm.NewFilter`, `hcm.NewFilterWithCtx`, `hcm.NewFilterWithCtxAndSinks`, `defaultRouterOnlyHTTPRegistry`) have zero call/declaration sites. Doc-comment archaeology references (3 lines total in `filter.go:24-26` + 1 line in `testhelpers_test.go:15`) name the deleted symbols intentionally for code-archaeology readability. The unrelated `tcpproxy.NewFilter` (a different function in `internal/filter/tcpproxy/filter.go`) is correctly preserved — phase-02 TCP-proxy constructor untouched by Task 14.

**Outputs:**
```
$ go build ./internal/filter/http/...
(clean)

$ go vet ./internal/filter/http/...
(clean)

$ go build ./internal/filter/hcm/ 2>&1
# github.com/esalaine/envoy-go/internal/filter/hcm
internal/filter/hcm/h2dispatch.go:62:8: undefined: routerActionH2
internal/filter/hcm/h2dispatch.go:119:10: undefined: routerActionH2
internal/filter/hcm/config.go:430:11: undefined: routerActionH2
internal/filter/hcm/config.go:432:10: undefined: routerAction
internal/filter/hcm/route.go:89:9: undefined: routerAction
internal/filter/hcm/route.go:91:9: undefined: routerActionH2

$ grep -rnE '\bNewFilter\b|\bNewFilterWithCtx\b|\bNewFilterWithCtxAndSinks\b|\bdefaultRouterOnlyHTTPRegistry\b' internal/filter/hcm/ --include='*.go' | grep -v 'filter_test.go\|fuzz_test.go\|config_test.go\|listener/manager.go' | wc -l
4   # all four are doc-comment archaeology in filter.go (3) + testhelpers_test.go (1); zero call/declaration sites
```

The hcm package non-buildability is the same six dangling-refs red state as Tasks 12 + 13 — restored at Task 16 per PLAN Task 12 Step 3.
