# Phase 07.2 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2/06.1/06.2/07.1 PROGRESS.md structure.

## Preamble — execution preconditions

None. All 14 preconditions satisfied at cold-start: branch `phase/07.2-listener-chain-completion-impl` at HEAD `9627855` (master tip after the PLAN SHA-fill follow-up); `git status` clean; Docker client (28.4.0) + server (28.1.1) both reported; `go version go1.26.2 linux/amd64` (PLAN required go1.23+); `golangci-lint has version v1.64.8` per ADR-0009; all 9 pre-existing differential fixtures (0000–0007b) PASS via `TestDifferential` subtests (the precondition's `-run 'Test.*0000|...|Test.*0007b'` regex does not match Go subtests directly — same workaround the 07.1 preamble documented; verified substantive intent by running `TestDifferential` directly and observing all 9 subtests PASS); `github.com/envoyproxy/go-control-plane/envoy v1.32.4` present per ADR-0013; `grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1` returns `ADR-0076:` (next-free 0077); SPEC.md last-commit is `bb5f4378dcc7ece9deddc703023d23e7e642cdfd`; `internal/listener/listenerfilter/` absent; `internal/listener/manager.go` key extension points present at lines 352 (`chainSpecificityRank`), 378 (`validateFilterChainMatch`), 413 (`makeGetConfigForClient`), 434 (`dispatch`), 550 (`serveTLS`); `BEHAVIOR_CONTRACT.md` has all four anchor headings (`## TCP proxy` line 330, `## TLS` line 372, `## HTTP filter chain` line 514, `## xDS wire state machine` line 250); `HTTPRegistry` symbol present in `internal/filter/http/registry.go` (07.1 deliverable per ADR-0072); `go list github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3` returns the package path; reference Envoy image `envoyproxy/envoy:v1.37.2` pulled successfully; `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` is empty.

## Task 1 — Execution-precondition check + PROGRESS.md preamble [ADR-0077, ADR-0083]

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 14 preconditions per PLAN §"Execution preconditions"; phase-07.1 close confirmed present in HEAD; SPEC at bb5f4378dcc7ece9deddc703023d23e7e642cdfd; ADR tail at 0076 (next-free 0077); internal/listener/listenerfilter/ absent (the package implementation lands at Task 2+); manager.go line numbers verified at 327/352/378/413/434/550 (the chain-sort at 327 is verified by inspection; the other five via the precondition-9 grep). Landed ADR-0077 (phase-07.2 scope decision) + ADR-0083 (ADR-0050 disposition; coexistence not supersession).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/07.2-listener-chain-completion-impl
$ go version
go version go1.26.2 linux/amd64
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
ADR-0076:
$ git log -1 --format=%H -- docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md
bb5f4378dcc7ece9deddc703023d23e7e642cdfd
$ test ! -d internal/listener/listenerfilter && echo OK
OK
```

## Task 2 — internal/listener/listenerfilter/{doc,types,callbacks}.go [ADR-0079]

**Commits:** TBD — this task's commit
**Notes:** Created `internal/listener/listenerfilter/` package with `doc.go` (~36 LoC; package-level overview enumerating ListenerFilter / Status / ChainMatchInputs / Peeker / Registry / Pipeline / chainmatch.SelectChain), `types.go` (~95 LoC: `ListenerFilter` interface, `ListenerFilterStatus` enum with `Continue=0`/`StopIteration=1`, 8-field `ChainMatchInputs` struct + `IsLoopbackSource()` helper, `Peeker` interface, `ListenerFilterFactory` + `FilterInstanceFactory` 2-step factory pattern, empty `FactoryCtx` carrier), `callbacks.go` (~67 LoC: `peekerConn` concrete struct embedding `net.Conn` + `bufio.Reader`, `NewPeekerConn` default-size constructor + `NewPeekerConnSize` size-clamped variant `[256, 65536]`, `Peek`/`Read` discipline, `AsPeeker` helper). Tests in `types_test.go` (`TestChainMatchInputsZeroValueIsBenign`, `TestChainMatchInputsIsLoopbackSource` 7 cases, `TestStatusEnumValues`) cover zero-value semantics + IPv4 127.0.0.0/8 + IPv6 ::1 loopback / non-loopback / nil + status enum drift; tests in `callbacks_test.go` (`TestPeekerConnPeekDoesNotConsume`, `TestPeekerConnPeekBeyondBuffer`, `TestNewPeekerConnSizeClamps`) cover `peekerConn`'s bytes-not-consumed invariant under interleaved `Peek` / `ReadFull`, `Peek(257)` on a 256-byte buffer returning `bufio.ErrBufferFull`, and `NewPeekerConnSize(srv, 100)` clamping up to 256 (`Peek(256)` succeeds). TDD discipline observed: types_test.go was written first; tests confirmed failing (build error: undefined symbols ChainMatchInputs / Continue / StopIteration); then doc.go / types.go / callbacks.go / callbacks_test.go landed; tests confirmed passing. Project-precedent `//nolint:revive` annotations cite ADR-0079 for the `ListenerFilterStatus` and `ListenerFilterFactory` reserved type names (mirrors `internal/filter/http/types.go` ADR-0071/ADR-0072 precedent); `//nolint:unused` on the PLAN-verbatim `pipeConn` test type (reserved scaffolding); `defer func() { _ = c.Close() }()` pattern adopted in tests for errcheck cleanliness (mirrors `internal/filter/hcm/h2/framer_test.go` precedent). Landed ADR-0079 (listener-filter dispatch protocol shape; sync-only; freeze-after-boot registry; 2-step factory pattern; 4096-byte default peeker buffer clamped [256, 65536]).
**Outputs:**
```
$ go test ./internal/listener/listenerfilter/... -v
=== RUN   TestPeekerConnPeekDoesNotConsume
--- PASS: TestPeekerConnPeekDoesNotConsume (0.00s)
=== RUN   TestPeekerConnPeekBeyondBuffer
--- PASS: TestPeekerConnPeekBeyondBuffer (0.00s)
=== RUN   TestNewPeekerConnSizeClamps
--- PASS: TestNewPeekerConnSizeClamps (0.00s)
=== RUN   TestChainMatchInputsZeroValueIsBenign
--- PASS: TestChainMatchInputsZeroValueIsBenign (0.00s)
=== RUN   TestChainMatchInputsIsLoopbackSource
=== RUN   TestChainMatchInputsIsLoopbackSource/IPv4_127.0.0.1
=== RUN   TestChainMatchInputsIsLoopbackSource/IPv4_127.255.255.254
=== RUN   TestChainMatchInputsIsLoopbackSource/IPv6_::1
=== RUN   TestChainMatchInputsIsLoopbackSource/IPv4_192.168.1.1
=== RUN   TestChainMatchInputsIsLoopbackSource/IPv4_10.0.0.1
=== RUN   TestChainMatchInputsIsLoopbackSource/IPv6_2001:db8::1
=== RUN   TestChainMatchInputsIsLoopbackSource/nil
--- PASS: TestChainMatchInputsIsLoopbackSource (0.00s)
    --- PASS: TestChainMatchInputsIsLoopbackSource/IPv4_127.0.0.1 (0.00s)
    --- PASS: TestChainMatchInputsIsLoopbackSource/IPv4_127.255.255.254 (0.00s)
    --- PASS: TestChainMatchInputsIsLoopbackSource/IPv6_::1 (0.00s)
    --- PASS: TestChainMatchInputsIsLoopbackSource/IPv4_192.168.1.1 (0.00s)
    --- PASS: TestChainMatchInputsIsLoopbackSource/IPv4_10.0.0.1 (0.00s)
    --- PASS: TestChainMatchInputsIsLoopbackSource/IPv6_2001:db8::1 (0.00s)
    --- PASS: TestChainMatchInputsIsLoopbackSource/nil (0.00s)
=== RUN   TestStatusEnumValues
--- PASS: TestStatusEnumValues (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	0.002s
$ go vet ./internal/listener/listenerfilter/...
$ golangci-lint run ./internal/listener/listenerfilter/...
```

## Task 3 — internal/listener/listenerfilter/registry.go

**Commits:** TBD — this task's commit
**Notes:** Created `internal/listener/listenerfilter/registry.go` (~57 LoC: `ListenerFilterRegistry struct{ mu sync.RWMutex; byTypeURL map[string]ListenerFilterFactory; frozen atomic.Bool }`, `NewListenerFilterRegistry()` empty-allocator, `Register(typeURL, f)` with frozen-guard + duplicate-guard panics, `Lookup(typeURL)` RLock-protected, `Freeze()` idempotent atomic.Bool.Store(true)) and `internal/listener/listenerfilter/registry_test.go` (~75 LoC, 5 tests + `dummyFactory` helper). Tests cover register+lookup happy path + absent-key ok=false branch (`TestRegistryRegisterAndLookup`), duplicate-Register panic (`TestRegistryDuplicateRegisterPanics`), post-Freeze Register panic (`TestRegistryFreezeBlocksRegister`), Freeze idempotency over 3 calls (`TestRegistryFreezeIsIdempotent`), and 100-goroutine concurrent Lookup under `-race` (`TestRegistryConcurrentLookup`). No new ADR for Task 3 — follows ADR-0079's pre-landed registry shape and mirrors 07.1's `internal/filter/http/registry.go` HTTPRegistry (ADR-0072) + 06.1's `*stats.Registry` LBP-1 (ADR-0059) freeze-after-boot discipline. Panic messages match PLAN exactly: `listenerfilter: registry frozen: cannot register %q post-boot` and `listenerfilter: duplicate factory for %q`. TDD discipline observed: registry_test.go was written first; tests confirmed failing (build error: `undefined: NewListenerFilterRegistry` x5); then registry.go landed; tests confirmed passing under `-race`. Project-precedent `//nolint:revive` on the `ListenerFilterRegistry` stuttering type name cites ADR-0079 (mirrors `internal/filter/http/registry.go`'s `HTTPRegistry` ADR-0072 annotation).
**Outputs:**
```
$ go test ./internal/listener/listenerfilter/... 2>&1 | head -30
# github.com/esalaine/envoy-go/internal/listener/listenerfilter [github.com/esalaine/envoy-go/internal/listener/listenerfilter.test]
internal/listener/listenerfilter/registry_test.go:15:7: undefined: NewListenerFilterRegistry
internal/listener/listenerfilter/registry_test.go:31:7: undefined: NewListenerFilterRegistry
internal/listener/listenerfilter/registry_test.go:43:7: undefined: NewListenerFilterRegistry
internal/listener/listenerfilter/registry_test.go:56:7: undefined: NewListenerFilterRegistry
internal/listener/listenerfilter/registry_test.go:63:7: undefined: NewListenerFilterRegistry
FAIL	github.com/esalaine/envoy-go/internal/listener/listenerfilter [build failed]
FAIL
$ go test -race -run 'TestRegistry' ./internal/listener/listenerfilter/... -v
=== RUN   TestRegistryRegisterAndLookup
--- PASS: TestRegistryRegisterAndLookup (0.00s)
=== RUN   TestRegistryDuplicateRegisterPanics
--- PASS: TestRegistryDuplicateRegisterPanics (0.00s)
=== RUN   TestRegistryFreezeBlocksRegister
--- PASS: TestRegistryFreezeBlocksRegister (0.00s)
=== RUN   TestRegistryFreezeIsIdempotent
--- PASS: TestRegistryFreezeIsIdempotent (0.00s)
=== RUN   TestRegistryConcurrentLookup
--- PASS: TestRegistryConcurrentLookup (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.007s
$ go test -race ./internal/listener/listenerfilter/...
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.008s
$ go vet ./internal/listener/listenerfilter/...
$ golangci-lint run ./internal/listener/listenerfilter/...
```

## Task 4 — internal/listener/listenerfilter/pipeline.go [ADR-0082]

**Commits:** TBD — this task's commit
**Notes:** Created `internal/listener/listenerfilter/pipeline.go` (~60 LoC: empty `Pipeline struct{}` reserved for future per-pipeline state; `Run(ctx, filters, peeker, inputs, timeoutMs uint32) (retErr error)` driving sequential dispatch with `defer` calling `OnDestroy()` on every constructed filter regardless of exit path; zero-filters short-circuit returning nil without timeout setup; `timeoutMs > 0` establishes a single shared `context.WithTimeout` once before the loop — per-pipeline NOT per-filter time-slicing per ADR-0082 + Decision N; `timeoutMs == 0` passes the caller's ctx through unmodified; per-iteration `Inspect` followed by error-wrap `listener-filter[%d]: %w`, post-Inspect `ctx.Err()` check wrapped `listener-filter[%d]: pipeline timeout: %w`, and `StopIteration` early-return) and `internal/listener/listenerfilter/pipeline_test.go` (~170 LoC, 7 tests + `stubFilter` test-only impl). Tests cover zero-filters (`TestPipelineRunZeroFilters`), Continue-then-finish populating both filters' inputs and verifying OnDestroy on both (`TestPipelineRunContinuePath`), StopIteration short-circuit with f2-not-fired assertion + OnDestroy on both (`TestPipelineRunStopIterationPath`), filter error propagation via `errors.Is` (`TestPipelineRunFilterError`), per-pipeline shared-budget timing (`TestPipelineRunTimeoutSharedAcrossFilters` — 30ms budget, f1 sleeps 50ms; f2 must not fire), zero-timeout no-op (`TestPipelineRunZeroTimeoutDisablesEnforcement`), and `fmt.Errorf` wrapping (`TestPipelineRunPropagatesError`). TDD discipline observed: pipeline_test.go was written first; tests confirmed failing (build error: `undefined: Pipeline` x7); then pipeline.go landed; tests confirmed passing under `-race`. Project-precedent `defer func() { _ = c.Close() }()` pattern adopted in tests for errcheck cleanliness (mirrors `callbacks_test.go` Task 2 precedent). No `nolint:revive` needed for the `Pipeline` type — name does not stutter against the package name. The 30ms budget on the timing-sensitive test is intentional (real `context.WithTimeout` mechanic; on slow CI it could flake — increase only if observed). Landed ADR-0082 (`listener_filters_timeout` [1s, 60s] envelope, 15s default, `continue_on_listener_filters_timeout` honored, per-pipeline shared budget per Decision N).
**Outputs:**
```
$ go test ./internal/listener/listenerfilter/... 2>&1 | head -30
# github.com/esalaine/envoy-go/internal/listener/listenerfilter [github.com/esalaine/envoy-go/internal/listener/listenerfilter.test]
internal/listener/listenerfilter/pipeline_test.go:30:8: undefined: Pipeline
internal/listener/listenerfilter/pipeline_test.go:50:8: undefined: Pipeline
internal/listener/listenerfilter/pipeline_test.go:78:8: undefined: Pipeline
internal/listener/listenerfilter/pipeline_test.go:103:8: undefined: Pipeline
internal/listener/listenerfilter/pipeline_test.go:130:8: undefined: Pipeline
internal/listener/listenerfilter/pipeline_test.go:152:8: undefined: Pipeline
internal/listener/listenerfilter/pipeline_test.go:167:8: undefined: Pipeline
FAIL	github.com/esalaine/envoy-go/internal/listener/listenerfilter [build failed]
FAIL
$ go test -race -run 'TestPipeline' ./internal/listener/listenerfilter/... -v
=== RUN   TestPipelineRunZeroFilters
--- PASS: TestPipelineRunZeroFilters (0.00s)
=== RUN   TestPipelineRunContinuePath
--- PASS: TestPipelineRunContinuePath (0.00s)
=== RUN   TestPipelineRunStopIterationPath
--- PASS: TestPipelineRunStopIterationPath (0.00s)
=== RUN   TestPipelineRunFilterError
--- PASS: TestPipelineRunFilterError (0.00s)
=== RUN   TestPipelineRunTimeoutSharedAcrossFilters
--- PASS: TestPipelineRunTimeoutSharedAcrossFilters (0.03s)
=== RUN   TestPipelineRunZeroTimeoutDisablesEnforcement
--- PASS: TestPipelineRunZeroTimeoutDisablesEnforcement (0.01s)
=== RUN   TestPipelineRunPropagatesError
--- PASS: TestPipelineRunPropagatesError (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.047s
$ go test -race ./internal/listener/listenerfilter/...
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.047s
$ go vet ./internal/listener/listenerfilter/...
$ golangci-lint run ./internal/listener/listenerfilter/...
```

## Task 5 — internal/listener/listenerfilter/chainmatch.go [ADR-0080, ADR-0081]

**Commits:** TBD — this task's commit
**Notes:** Created `internal/listener/listenerfilter/chainmatch.go` (~320 LoC) implementing the 8-dimension `SelectChain` precedence algorithm — the heart of phase 07.2. Defines `ChainSpec` struct with 11 fields (`Name`, `Empty`, `DestinationPort`, `PrefixRanges`, `ServerNames`, `TransportProtocol`, `ApplicationProtocols`, `SourceTypeLocal`, `SourceTypeExternal`, `SourcePrefixRanges`, `SourcePorts`); 8 priority constants (`prioDestinationPort` = 0 through `prioSourcePorts` = 7) + `prioCount` = 8; `ErrNoChainMatched` and `ErrAmbiguousChainMatch` sentinel errors. The `SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error)` function runs the 2-pass eligibility-then-specificity algorithm per SPEC §5.5 + §7.3: Pass 1 builds the eligibility set via `matches(c, &inputs)`; if empty, returns `defaultChain` if non-nil OR `ErrNoChainMatched` if nil (per ADR-0080's no-match-fallback semantics); Pass 2 scores each eligible chain via `specificityScore(c)` returning a `uint8` where bit `prioCount-1-i` is set iff priority slot `i` is specified, and chain with the highest score wins; ties broken via `breakTie(a, b, &inputs)` on per-dimension finer-grain criteria (longest CIDR prefix on `prefix_ranges`/`source_prefix_ranges`; SNI specificity on `server_names` per `sniSpecificityRank` — the new copy of `manager.go:chainSpecificityRank` per ADR-0033 clause 9 preserved as ADR-0078 sub-ordering; the duplication is intentional and temporary — Task 9 deletes the manager.go original); chains entirely indistinguishable on this input return `nil` from `breakTie` and `SelectChain` returns `ErrAmbiguousChainMatch`. Helpers: `ipInAny`, `portInAny`, `longestPrefix`, `sniMatchAny`, `alpnMatchAny`, `sniSpecificityRank`. The `matches` helper short-circuits on `c.Empty` (universally eligible) and otherwise checks each non-zero / non-empty dimension against the corresponding `ChainMatchInputs` field. Created `internal/listener/listenerfilter/chainmatch_test.go` (~140 LoC, 10 tests + `cidr(s)` helper at the top): `TestSelectChainEmptyMatchUniversallyEligible` (catch-all chain with `DestinationPort: 8080` + `SourceIP: 10.0.0.1` inputs); `TestSelectChainDestinationPortBeatsSourcePrefix` (the SPEC §11.3 empirical-pin assertion — `dstport` wins over `srcprefix` on a connection from `127.0.0.1` to port `8080`); `TestSelectChainDefaultChainOnNoMatch` (specific `loopback` chain not eligible against `10.0.0.1` source → `default` chain wins); `TestSelectChainEmptyMatchBeatsDefault` (the SPEC §11.2 empirical-pin assertion — empty-match chain in `filter_chains[]` BEATS `default_filter_chain` per ADR-0080); `TestSelectChainNoEligibleNoDefault` (uses `errors.Is(err, ErrNoChainMatched)`; verifies returned chain is nil); `TestSelectChainPrefixRangesLongerWins` (192.168.1.0/24 beats 192.168.0.0/16 on input 192.168.1.50 — longest-prefix tie-breaker); `TestSelectChainServerNamesSpecificity` (exact `foo.example.test` > suffix `*.example.test` > universal `*` on input `foo.example.test`); `TestSelectChainSourceTypeLocal` (loopback source → `source_type:LOCAL` chain wins over universal); `TestSelectChainSourceTypeExternalSkipsLoopback` (loopback source → `source_type:EXTERNAL` chain eliminated, universal wins); `TestSelectChainApplicationProtocolsTieBreaker` (ALPN `h2` offer → h2 chain wins, h1 chain eliminated). TDD discipline observed: chainmatch_test.go was written first; tests confirmed failing (build error: `undefined: ChainSpec`, `undefined: SelectChain`); then chainmatch.go landed; tests confirmed passing under `-race`. All 10 tests PASS; vet clean; golangci-lint clean. Landed ADR-0080 (`default_filter_chain` no-match-fallback semantics; empty-match BEATS default; TLS posture independent — supersedes ADR-0033 clause 3) and ADR-0081 (8-dim precedence algorithm; eligibility-then-specificity 2-pass; tie-break finer-grain; `ErrAmbiguousChainMatch` at NewManager-build time — partially supersedes ADR-0033 clause 2). The `sniSpecificityRank` function is a NEW copy of the existing `internal/listener/manager.go:chainSpecificityRank` (line 352) semantics; the duplication is intentional and temporary — Task 9 will refactor manager.go to delete the original (per ADR-0078 caveat).
**Outputs:**
```
$ go test ./internal/listener/listenerfilter/... 2>&1 | head -30
# github.com/esalaine/envoy-go/internal/listener/listenerfilter [github.com/esalaine/envoy-go/internal/listener/listenerfilter.test]
internal/listener/listenerfilter/chainmatch_test.go:12:9: undefined: ChainSpec
internal/listener/listenerfilter/chainmatch_test.go:14:14: undefined: SelectChain
internal/listener/listenerfilter/chainmatch_test.go:14:37: undefined: ChainSpec
internal/listener/listenerfilter/chainmatch_test.go:24:14: undefined: ChainSpec
internal/listener/listenerfilter/chainmatch_test.go:25:16: undefined: ChainSpec
internal/listener/listenerfilter/chainmatch_test.go:27:14: undefined: SelectChain
internal/listener/listenerfilter/chainmatch_test.go:27:37: undefined: ChainSpec
internal/listener/listenerfilter/chainmatch_test.go:37:15: undefined: ChainSpec
internal/listener/listenerfilter/chainmatch_test.go:38:10: undefined: ChainSpec
internal/listener/listenerfilter/chainmatch_test.go:40:14: undefined: SelectChain
internal/listener/listenerfilter/chainmatch_test.go:40:14: too many errors
FAIL	github.com/esalaine/envoy-go/internal/listener/listenerfilter [build failed]
FAIL
$ go test -race -run 'TestSelectChain' ./internal/listener/listenerfilter/... -v
=== RUN   TestSelectChainEmptyMatchUniversallyEligible
--- PASS: TestSelectChainEmptyMatchUniversallyEligible (0.00s)
=== RUN   TestSelectChainDestinationPortBeatsSourcePrefix
--- PASS: TestSelectChainDestinationPortBeatsSourcePrefix (0.00s)
=== RUN   TestSelectChainDefaultChainOnNoMatch
--- PASS: TestSelectChainDefaultChainOnNoMatch (0.00s)
=== RUN   TestSelectChainEmptyMatchBeatsDefault
--- PASS: TestSelectChainEmptyMatchBeatsDefault (0.00s)
=== RUN   TestSelectChainNoEligibleNoDefault
--- PASS: TestSelectChainNoEligibleNoDefault (0.00s)
=== RUN   TestSelectChainPrefixRangesLongerWins
--- PASS: TestSelectChainPrefixRangesLongerWins (0.00s)
=== RUN   TestSelectChainServerNamesSpecificity
--- PASS: TestSelectChainServerNamesSpecificity (0.00s)
=== RUN   TestSelectChainSourceTypeLocal
--- PASS: TestSelectChainSourceTypeLocal (0.00s)
=== RUN   TestSelectChainSourceTypeExternalSkipsLoopback
--- PASS: TestSelectChainSourceTypeExternalSkipsLoopback (0.00s)
=== RUN   TestSelectChainApplicationProtocolsTieBreaker
--- PASS: TestSelectChainApplicationProtocolsTieBreaker (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.007s
$ go test -race ./internal/listener/listenerfilter/...
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.049s
$ go vet ./internal/listener/listenerfilter/...
$ golangci-lint run ./internal/listener/listenerfilter/...
```

**Follow-up commit (review-driven fix):** Code-quality reviewer flagged a SPEC §5.5/§7.3 violation in the original `breakTie` cascade order: PrefixRanges → SourcePrefixRanges → ServerNames was PLAN-verbatim but contradicted the SPEC's "walk down the priority list" rule (§5.5 line 519, §7.3 line 524). The cascade now follows priority order: PrefixRanges (slot 1) → ServerNames (slot 2) → SourcePrefixRanges (slot 6). Added 4 new tests: TestSelectChainBreakTieFollowsPriorityOrder (the multi-dim counter-example), TestSelectChainAmbiguousReturnsError (covers ErrAmbiguousChainMatch — an acceptance criterion missing from the original test set), TestSelectChainTransportProtocol + TestSelectChainSourcePorts (cover the two priority dimensions previously not exercised). Per ADR-0017 (small-mechanical-fixes do not require ADRs), no new ADR — the fix aligns code with SPEC §5.5/§7.3 and ADR-0081's documented priority-ordered tie-break rule.

## Task 6 — internal/listener/listenerfilter/fuzz_test.go (10th fuzzer)

**Commits:** TBD — this task's commit
**Notes:** Created `internal/listener/listenerfilter/fuzz_test.go` (~75 LoC) introducing `FuzzFilterChainMatch` — the 10th fuzzer overall in the repo (prior 9: `FuzzAccessLogFormat`, `FuzzBootstrapLoad`, `FuzzFilterChainParse`, `FuzzHCMConfigParse`, `FuzzFrameStream`, `FuzzHPACKDecode`, `FuzzTcpProxyFilter`, `FuzzPromTextFormat`, `FuzzTLSContextParse`). Verbatim PLAN Step 1 source: 4 seed-corpus entries (§11.3-shape inputs `(8080, 0, "127.0.0.1", "")`; non-loopback `(0, 54321, "10.0.0.1", "foo.test")`; IPv6 loopback `(443, 0, "::1", "")`; wildcard SNI `(80, 12345, "192.168.1.1", "*")`); 4 assertions per SPEC §15.6 — (i) `SelectChain` never panics; (ii) returned chain is one of input chains OR `defaultChain` OR `(nil, ErrNoChainMatched)` / `(nil, ErrAmbiguousChainMatch)`; (iii) returned chain's match dimensions are all satisfied by the inputs (re-runs `matches`); (iv) deterministic on identical inputs (running twice yields the same result). The fuzz body builds a varied chain set covering the 8 priority dimensions: chain `a` with `DestinationPort: 8080`, chain `b` with `SourcePrefixRanges: 127.0.0.0/8`, chain `c` with `ServerNames: ["foo.test", "*.bar.test"]`, chain `d` with `Empty: true`, plus a `def` default chain. Local helper `mustCIDR(s)` panics on parse error; this is intentionally distinct from `chainmatch_test.go`'s `cidr(s)` helper which returns nil on error — both coexist in the same package because `mustCIDR` is fuzz-test seed-construction (must surface parse errors loudly) while `cidr` is unit-test inline literal (must be tolerant of typos at write time). Ran the 30s ADR-0018 short-budget locally: 17,175,377 executions at ~576k execs/sec, 76 interesting inputs discovered, 0 counterexamples — clean. Seed corpus runs as normal `go test` cases (the 4 `f.Add` entries become test inputs under `go test -run=FuzzFilterChainMatch`); all PASS. CI will replay the seed corpus per ADR-0018 (short-budget = 30s in CI, deterministic seed-corpus replay always). Pristine state confirmed: `go vet` clean, `golangci-lint run` clean, `go test -race -count=1 ./internal/listener/listenerfilter/...` PASS.
**Outputs:**
```
$ go test -fuzz=FuzzFilterChainMatch -fuzztime=30s ./internal/listener/listenerfilter/ 2>&1 | tail -10
fuzz: elapsed: 9s, execs: 5075793 (587034/sec), new interesting: 71 (total: 75)
fuzz: elapsed: 12s, execs: 6795521 (573201/sec), new interesting: 71 (total: 75)
fuzz: elapsed: 15s, execs: 8541202 (581917/sec), new interesting: 71 (total: 75)
fuzz: elapsed: 18s, execs: 10283306 (580511/sec), new interesting: 72 (total: 76)
fuzz: elapsed: 21s, execs: 12013519 (576707/sec), new interesting: 72 (total: 76)
fuzz: elapsed: 24s, execs: 13706085 (564403/sec), new interesting: 72 (total: 76)
fuzz: elapsed: 27s, execs: 15446634 (580175/sec), new interesting: 72 (total: 76)
fuzz: elapsed: 30s, execs: 17175377 (576264/sec), new interesting: 72 (total: 76)
fuzz: elapsed: 30s, execs: 17175377 (0/sec), new interesting: 72 (total: 76)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	30.176s
$ go test -count=1 ./internal/listener/listenerfilter/...
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	0.042s
$ go test -race -count=1 ./internal/listener/listenerfilter/...
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.049s
$ go vet ./internal/listener/listenerfilter/...
$ golangci-lint run ./internal/listener/listenerfilter/...
```

## Task 7 — internal/listener/listenerfilter/tls_inspector/parser.go

**Commits:** TBD — this task's commit
**Notes:** Created the `tls_inspector/` sub-package — the FIRST file in this directory — with `parser.go` (~150 LoC: `parseClientHello` + `parseServerName` + `parseALPN`; pure functions; no I/O; only `encoding/binary` import) and `parser_test.go` (~108 LoC: 7 tests using a `captureClientHello(t, sni, alpns)` helper that runs `crypto/tls.Client` against `net.Pipe()` to capture verbatim ClientHello bytes from the standard library — TestParseClientHelloWithSNIAndALPN, TestParseClientHelloSNIOnly, TestParseClientHelloALPNOnly, TestParseClientHelloEmpty, TestParseClientHelloNonTLSPreamble, TestParseClientHelloTruncated [loops cuts 1..50 asserting ok=false], TestParseClientHelloMalformedLengthPrefix [recovers from any panic]). PLAN-verbatim Go source from PLAN lines 1672-1756 (test) + 1765-1866 (impl). The parser is hand-rolled per D-3.2 (no cgo / C++ binding to upstream Envoy's `tls_inspector`); adapted from `crypto/tls/handshake_messages.go:unmarshal` for the ClientHello case, narrowed to two extension types of interest — `0x0000` (server_name, RFC 6066 §3) and `0x0010` (application_layer_protocol_negotiation, RFC 7301 §3.1). Defensive parsing: every length-bounded read checks remaining buffer size before advancing; malformed inputs return `ok=false` without panicking. The `case 0x0000`/`case 0x0010` handlers use `if name, ok := parseServerName(body); ok { sni = name }` — the inner `ok` shadows the outer return value but is scoped to the `if` block (per Go scoping rules), so a malformed extension body silently leaves `sni`/`alpns` empty rather than aborting the whole parse. TDD discipline observed: parser_test.go was written first; `go test ./internal/listener/listenerfilter/tls_inspector/... 2>&1 | head -30` confirmed failing (build error: undefined symbol `parseClientHello`); then parser.go landed; tests passed under `-race`. Two adaptations from PLAN-verbatim source for lint cleanliness: (a) `defer cli.Close()` / `defer srv.Close()` rewritten to `defer func() { _ = cli.Close() }()` / `defer func() { _ = srv.Close() }()` to satisfy errcheck (mirrors the `internal/listener/listenerfilter/callbacks_test.go` precedent established in Task 2); (b) added a package doc comment to `parser.go` to satisfy revive's `package-comments` rule (the only revive linter that fired against the PLAN-verbatim source) — the comment cites D-3.2 (no cgo) + ADR-0079 (the `tls_inspector` underscore name follows the `envoy.filters.listener.tls_inspector` type_url convention; an explicit `//nolint:revive` directive on the package declaration documents the convention even though revive did not flag the underscore itself). No new ADR — these are mechanical lint adaptations within the small-mechanical-fixes umbrella (ADR-0017). Pristine state confirmed: `go vet` clean, `golangci-lint run` clean, `go test -race` PASS for both the new sub-package AND the parent `listenerfilter` package.
**Outputs:**
```
$ go test ./internal/listener/listenerfilter/tls_inspector/... 2>&1 | head -30
# github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector [github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector.test]
internal/listener/listenerfilter/tls_inspector/parser_test.go:33:20: undefined: parseClientHello
internal/listener/listenerfilter/tls_inspector/parser_test.go:47:20: undefined: parseClientHello
internal/listener/listenerfilter/tls_inspector/parser_test.go:61:20: undefined: parseClientHello
internal/listener/listenerfilter/tls_inspector/parser_test.go:74:14: undefined: parseClientHello
internal/listener/listenerfilter/tls_inspector/parser_test.go:82:14: undefined: parseClientHello
internal/listener/listenerfilter/tls_inspector/parser_test.go:91:15: undefined: parseClientHello
internal/listener/listenerfilter/tls_inspector/parser_test.go:107:12: undefined: parseClientHello
FAIL	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector [build failed]
FAIL
$ go test -race ./internal/listener/listenerfilter/tls_inspector/... -v 2>&1 | tail -30
=== RUN   TestParseClientHelloWithSNIAndALPN
--- PASS: TestParseClientHelloWithSNIAndALPN (0.00s)
=== RUN   TestParseClientHelloSNIOnly
--- PASS: TestParseClientHelloSNIOnly (0.00s)
=== RUN   TestParseClientHelloALPNOnly
--- PASS: TestParseClientHelloALPNOnly (0.00s)
=== RUN   TestParseClientHelloEmpty
--- PASS: TestParseClientHelloEmpty (0.00s)
=== RUN   TestParseClientHelloNonTLSPreamble
--- PASS: TestParseClientHelloNonTLSPreamble (0.00s)
=== RUN   TestParseClientHelloTruncated
--- PASS: TestParseClientHelloTruncated (0.00s)
=== RUN   TestParseClientHelloMalformedLengthPrefix
--- PASS: TestParseClientHelloMalformedLengthPrefix (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.008s
$ go test -race ./internal/listener/listenerfilter/...
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.049s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.008s
$ go vet ./internal/listener/listenerfilter/tls_inspector/...
$ golangci-lint run ./internal/listener/listenerfilter/tls_inspector/...
```

## Task 8 — internal/listener/listenerfilter/tls_inspector/{doc,tls_inspector,proto}.go

**Commits:** TBD — this task's commit
**Notes:** Completed the `tls_inspector/` sub-package with the full ListenerFilter implementation: `doc.go` (~22 LoC; package-level overview citing ADR-0079 two-step factory pattern + D-3.2 no-cgo discipline + Introduced-by 07.2 marker), `tls_inspector.go` (~70 LoC: PLAN-verbatim from PLAN lines 1913-1979 — `TypeURL` constant matching upstream go-control-plane's `type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector`; `New(tc, ctx) (FilterInstanceFactory, error)` factory; private `config{bufferSize int}` + `filter{cfg *config}` types; `Inspect(ctx, peeker, inputs)` peeks `cfg.bufferSize` bytes, sets `inputs.TransportProtocol="raw_buffer"` on parse failure or empty preamble, sets `"tls"` + populates `ServerName`/`ApplicationProtocols` on ClientHello detection — always returns `Continue`; `OnDestroy()` no-op), `proto.go` (~38 LoC: PLAN-verbatim from PLAN lines 1985-2021 — `defaultBufferSize=4096`, `minBufferSize=256`, `maxBufferSize=65536`; `parseConfig(*anypb.Any)` returns default config on nil tc; unmarshals `tls_inspectorv3.TlsInspector`; honors `InitialReadBufferSize` with floor-error `tls_inspector: initial_read_buffer_size %d below floor 256` and silent-clamp at 65536; `EnableJa3Fingerprinting` silently ignored per SPEC §12). Tests in `tls_inspector_test.go` (~190 LoC; 6 tests) cover: `TestInspectWithClientHelloPopulatesInputs` (drives Inspect over a peekConn fed real ClientHello bytes via `feedBytesAsPeeker(captureClientHelloBytes(...))` — asserts TransportProtocol="tls", ServerName="foo.example.test", ApplicationProtocols=[h2 http/1.1]), `TestInspectWithNonTLSPreambleSetsRawBuffer` (HTTP/1.1 GET preamble → TransportProtocol="raw_buffer"; SNI + ALPN remain empty), `TestInspectWithEmptyConnectionDoesNotPanic` (cli + srv both pre-closed; recover() guard + Continue assertion), `TestNewRoundtripsThroughRegistry` (Register `New` under `TypeURL` in a `NewListenerFilterRegistry`, Freeze, Lookup, instantiate via FilterInstanceFactory, type-assert `*filter`, verify cfg.bufferSize=2048 honors UInt32(2048) override), `TestInspectConcurrentIndependentConnections` (10 goroutines each running Inspect on independent feedBytesAsPeeker pipes — race-clean), `TestOnDestroyIsNoOp` (3 calls, no panic). Tests in `proto_test.go` (~95 LoC; 6 tests) cover: nil-tc default, empty-proto default, custom-in-range (1024), below-floor error (128 → exact match `tls_inspector: initial_read_buffer_size 128 below floor 256`), above-cap clamp (999999 → 65536, no error), JA3 silently-ignored (parseConfig succeeds, bufferSize=4096). Helper file `helpers_test.go` (~30 LoC) factors `captureClientHelloBytes(t, sni, alpns)` — adapted from parser_test.go's helper but renamed to avoid same-package-test collision with `captureClientHello`. Removed parser.go's package-doc comment (relocated to doc.go per Go convention; doc.go is now the canonical package-doc location).

TDD discipline observed: tls_inspector_test.go + proto_test.go were written first; `go test ./internal/listener/listenerfilter/tls_inspector/... 2>&1 | head -40` confirmed failing (build error: `undefined: parseConfig`, `undefined: defaultBufferSize`, etc.); then doc.go / tls_inspector.go / proto.go landed; tests confirmed passing under `-race`. PLAN-verbatim Go source from PLAN lines 1913-1979 (tls_inspector.go) + 1985-2021 (proto.go); test file content was described (not verbatim) per PLAN Step 1.

Two PLAN-verbatim adaptations for lint cleanliness (within ADR-0017 small-mechanical-fixes umbrella): (a) the if-statements in `Inspect` were reformatted to multi-line bodies (`if sni != ""` and `if len(alpns) > 0`) per gofmt's preference vs. the PLAN's single-line `{ inputs.X = Y }` shorthand; (b) project-precedent goimports 3-group import ordering (stdlib + third-party + project-local with blank-line separators) applied to both tls_inspector.go and tls_inspector_test.go (mirrors `internal/filter/tcpproxy/filter.go` convention).

A subtle fixture-shape decision arose during TDD: the first test draft attempted to drive Inspect over a live `crypto/tls.Client` handshake against a `net.Pipe()` with the srv-side wrapped in a peekConn. That construction deadlocked at 10 minutes — `bufio.Reader.Peek(4096)` on net.Pipe blocks until the buffer fills OR the underlying reader returns an error; the tls.Client's Handshake() call meanwhile blocks waiting for a ServerHello that never comes; the result is a circular-block where neither side advances. Fix: pre-capture the ClientHello bytes via the parser_test.go pattern (run tls.Client against an ephemeral net.Pipe, srv.Read gives a verbatim copy of the ClientHello), then write those bytes onto a fresh net.Pipe and close the cli end immediately — Peek(4096) then returns the buffered ~500 bytes plus io.EOF, and Inspect's `if err != nil && len(buf) == 0` clause correctly proceeds to parseClientHello with the available bytes. This matches the production discipline where Peek(n) returns whatever is available when the underlying conn signals EOF (the listener manager's accept-loop sees the same shape on a connection-closed-during-handshake scenario).

No new ADR — Task 8's surface is fully covered by ADR-0079 (two-step factory pattern + 4096-byte default + [256, 65536] clamp + freeze-after-boot registry) + SPEC §12 (silent-ignore set including `enable_ja3_fingerprinting`) + D-3.2 (no cgo). Pristine state confirmed: `go vet` clean, `golangci-lint run` clean, `go test -race` PASS for the new sub-package (6 new tls_inspector_test.go tests + 6 new proto_test.go tests + 7 pre-existing parser_test.go tests = 19 total) AND the parent `listenerfilter` package (cached green from Task 6's last invocation).
**Outputs:**
```
$ go test ./internal/listener/listenerfilter/tls_inspector/... 2>&1 | head -40
# github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector [github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector.test]
internal/listener/listenerfilter/tls_inspector/proto_test.go:13:14: undefined: parseConfig
internal/listener/listenerfilter/tls_inspector/proto_test.go:17:23: undefined: defaultBufferSize
internal/listener/listenerfilter/tls_inspector/proto_test.go:18:59: undefined: defaultBufferSize
internal/listener/listenerfilter/tls_inspector/proto_test.go:27:14: undefined: parseConfig
internal/listener/listenerfilter/tls_inspector/proto_test.go:31:23: undefined: defaultBufferSize
internal/listener/listenerfilter/tls_inspector/proto_test.go:32:59: undefined: defaultBufferSize
internal/listener/listenerfilter/tls_inspector/proto_test.go:43:14: undefined: parseConfig
internal/listener/listenerfilter/tls_inspector/proto_test.go:59:11: undefined: parseConfig
internal/listener/listenerfilter/tls_inspector/proto_test.go:76:14: undefined: parseConfig
internal/listener/listenerfilter/tls_inspector/proto_test.go:80:23: undefined: maxBufferSize
internal/listener/listenerfilter/tls_inspector/proto_test.go:80:23: too many errors
FAIL	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector [build failed]
FAIL
$ go test -race ./internal/listener/listenerfilter/tls_inspector/... -v 2>&1 | tail -50
=== RUN   TestParseClientHelloWithSNIAndALPN
--- PASS: TestParseClientHelloWithSNIAndALPN (0.00s)
=== RUN   TestParseClientHelloSNIOnly
--- PASS: TestParseClientHelloSNIOnly (0.00s)
=== RUN   TestParseClientHelloALPNOnly
--- PASS: TestParseClientHelloALPNOnly (0.00s)
=== RUN   TestParseClientHelloEmpty
--- PASS: TestParseClientHelloEmpty (0.00s)
=== RUN   TestParseClientHelloNonTLSPreamble
--- PASS: TestParseClientHelloNonTLSPreamble (0.00s)
=== RUN   TestParseClientHelloTruncated
--- PASS: TestParseClientHelloTruncated (0.00s)
=== RUN   TestParseClientHelloMalformedLengthPrefix
--- PASS: TestParseClientHelloMalformedLengthPrefix (0.00s)
=== RUN   TestParseConfigNilReturnsDefault
--- PASS: TestParseConfigNilReturnsDefault (0.00s)
=== RUN   TestParseConfigDefaultBuffer
--- PASS: TestParseConfigDefaultBuffer (0.00s)
=== RUN   TestParseConfigCustomBufferInRange
--- PASS: TestParseConfigCustomBufferInRange (0.00s)
=== RUN   TestParseConfigBufferBelowFloorErrors
--- PASS: TestParseConfigBufferBelowFloorErrors (0.00s)
=== RUN   TestParseConfigBufferAboveCapClamps
--- PASS: TestParseConfigBufferAboveCapClamps (0.00s)
=== RUN   TestParseConfigEnableJA3SilentlyIgnored
--- PASS: TestParseConfigEnableJA3SilentlyIgnored (0.00s)
=== RUN   TestInspectWithClientHelloPopulatesInputs
--- PASS: TestInspectWithClientHelloPopulatesInputs (0.00s)
=== RUN   TestInspectWithNonTLSPreambleSetsRawBuffer
--- PASS: TestInspectWithNonTLSPreambleSetsRawBuffer (0.00s)
=== RUN   TestInspectWithEmptyConnectionDoesNotPanic
--- PASS: TestInspectWithEmptyConnectionDoesNotPanic (0.00s)
=== RUN   TestNewRoundtripsThroughRegistry
--- PASS: TestNewRoundtripsThroughRegistry (0.00s)
=== RUN   TestInspectConcurrentIndependentConnections
--- PASS: TestInspectConcurrentIndependentConnections (0.00s)
=== RUN   TestOnDestroyIsNoOp
--- PASS: TestOnDestroyIsNoOp (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.010s
$ go test -race ./internal/listener/listenerfilter/...
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.049s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.010s
$ go vet ./internal/listener/listenerfilter/tls_inspector/...
$ golangci-lint run ./internal/listener/listenerfilter/tls_inspector/...
```

## Task 9 — internal/listener/manager.go [ADR-0078]

**Commits:** TBD — this task's commit
**Notes:** Largest single-file refactor of the phase. Rewrote `validateFilterChainMatch` (the phase-03 narrow whitelist that errored on every dimension beyond `server_names` + `transport_protocol == "tls"`) into `parseChainSpec` (the 8-dimension parser per ADR-0081 returning `*listenerfilter.ChainSpec`); removed the parse-time error on `Listener.default_filter_chain` (ADR-0033 clause 3 superseded by ADR-0078) and added the default-chain construction path (independent TLS posture per ADR-0080); added `listener_filters[]` parsing with type_url resolution through the threaded `*ListenerFilterRegistry` (ADR-0079 + ADR-0078 clause 8 supersession); parsed `listener_filters_timeout` per ADR-0082's [1s, 60s] envelope (default 15s); built `[]*ChainSpec` alongside the legacy `[]*chainInfo` per chain with ambiguous-selection detection at NewManager-build time per ADR-0081; widened `NewManagerWithBaseDirAndAllowH2C` signature with a trailing `lfRegistry *listenerfilter.ListenerFilterRegistry` parameter (delegating constructors `NewManager` + `NewManagerWithBaseDir` thread `nil`; `cmd/envoy-go/main.go` likewise threads `nil` until Task 11 wires the boot-populated registry); deleted `chainSpecificityRank` from `manager.go` (no remaining callers — the SNI-internal sub-ordering logic lives now ONLY as `sniSpecificityRank` in `chainmatch.go`, introduced verbatim at Task 5); preserved the legacy `chains` slice ordering (catch-all chains moved to end, no SNI-specificity sort) and the `makeGetConfigForClient` GetConfigForClient path so the Task-9 commit doesn't yet touch the dispatch path (Task 10 owns the acceptLoop/dispatch/serveTLS refactor); `listenerRuntime` widened with 7 new fields (`chainSpecs []*ChainSpec`, `defaultSpec *ChainSpec`, `defaultChain *chainInfo`, `chainByName map[string]*chainInfo`, `listenerFilterFactories []FilterInstanceFactory`, `lfTimeoutMs uint32`, `continueOnLfTimeout bool`) populated at build time but not yet consulted by dispatch; mixed-TLS rule preserved within `filter_chains[]` (ADR-0033 clause 5); plaintext-multi-chain rule narrowed to "only when at least one chain populates `server_names[]`" (ADR-0033 clause 6 partial supersession). TDD discipline: existing `TestManager_Error_NonEmptyFilterChainMatch` / `TestNewManager_MultiChain_NonSNIMatchField_Errors` / `TestNewManager_MultiChain_ApplicationProtocols_Errors` / `TestNewManager_MultiChain_DefaultFilterChain_Errors` / `TestNewManager_PlaintextMultiChain_Errors` rewritten to verify ACCEPT semantics (the dimension parses + populates the ChainSpec field) since the phase-03 errors are no longer surfaced; new tests `TestParseChainSpecAcceptsAllEightDimensions` (7 sub-tests, one per dimension), `TestParseChainSpecSilentlyIgnoresDirectSourcePrefixRanges`, `TestParseChainSpecRejectsUnknownTransportProtocol`, `TestParseDefaultFilterChainNoLongerErrors`, `TestParseListenerFiltersResolvesViaRegistry`, `TestParseListenerFiltersUnknownTypeURLErrors`, `TestParseListenerFiltersTimeoutInRange`, `TestParseListenerFiltersTimeoutDefault`, `TestParseListenerFiltersTimeoutBelowFloorErrors`, `TestParseListenerFiltersTimeoutAboveCapErrors`, `TestParseChainSpecMixedTLSPreserved`, `TestIdenticalFilterChainsErrorWithAmbiguousSelection`, `TestParseDefaultFilterChain_Plaintext_WithTLSFilterChain` cover the ADR-0078 supersession surface, the ADR-0079 listener_filters[] resolution, the ADR-0082 timeout envelope, and the ADR-0081 ambiguous-selection detection. Test bootstrap helper `testLFRegistry()` mirrors the `testHTTPRegistry()` pattern (registers tls_inspector via TypeURL into a frozen `*ListenerFilterRegistry`) — Task 11 will replicate this in main.go. Landed ADR-0078 (ADR-0033 partial supersession enumeration; full clause-by-clause table in §5.7 of SPEC.md mirrored verbatim in the ADR body).
**Outputs:**
```
$ go vet ./internal/listener/...
$ golangci-lint run ./internal/listener/...
$ go test -race ./internal/listener/...
ok  	github.com/esalaine/envoy-go/internal/listener	1.024s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	(cached)
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	(cached)
$ go test -race -run 'TestParseChainSpec|TestParseDefaultFilterChain|TestParseListenerFilters|TestIdenticalFilterChains|TestNewManager_PlaintextMultiChain|TestNewManager_MultiChain_ApplicationProtocols|TestManager_NonEmptyFilterChainMatch_DestinationPort' ./internal/listener/ -v
=== RUN   TestManager_NonEmptyFilterChainMatch_DestinationPort_Accepted
--- PASS: TestManager_NonEmptyFilterChainMatch_DestinationPort_Accepted (0.00s)
=== RUN   TestParseDefaultFilterChainNoLongerErrors
--- PASS: TestParseDefaultFilterChainNoLongerErrors (0.00s)
=== RUN   TestParseChainSpecAcceptsAllEightDimensions
    --- PASS: TestParseChainSpecAcceptsAllEightDimensions/destination_port (0.00s)
    --- PASS: TestParseChainSpecAcceptsAllEightDimensions/prefix_ranges (0.00s)
    --- PASS: TestParseChainSpecAcceptsAllEightDimensions/source_prefix_ranges (0.00s)
    --- PASS: TestParseChainSpecAcceptsAllEightDimensions/source_type_LOCAL (0.00s)
    --- PASS: TestParseChainSpecAcceptsAllEightDimensions/source_ports (0.00s)
    --- PASS: TestParseChainSpecAcceptsAllEightDimensions/application_protocols (0.00s)
    --- PASS: TestParseChainSpecAcceptsAllEightDimensions/transport_protocol_raw_buffer (0.00s)
--- PASS: TestParseChainSpecAcceptsAllEightDimensions (0.00s)
=== RUN   TestParseChainSpecSilentlyIgnoresDirectSourcePrefixRanges
--- PASS: TestParseChainSpecSilentlyIgnoresDirectSourcePrefixRanges (0.00s)
=== RUN   TestParseChainSpecRejectsUnknownTransportProtocol
--- PASS: TestParseChainSpecRejectsUnknownTransportProtocol (0.00s)
=== RUN   TestNewManager_MultiChain_ApplicationProtocols_Accepted
--- PASS: TestNewManager_MultiChain_ApplicationProtocols_Accepted (0.00s)
=== RUN   TestNewManager_PlaintextMultiChain_WithSNI_Errors
--- PASS: TestNewManager_PlaintextMultiChain_WithSNI_Errors (0.00s)
=== RUN   TestNewManager_PlaintextMultiChain_NonSNIDimensions_Accepted
--- PASS: TestNewManager_PlaintextMultiChain_NonSNIDimensions_Accepted (0.00s)
=== RUN   TestParseListenerFiltersResolvesViaRegistry
--- PASS: TestParseListenerFiltersResolvesViaRegistry (0.00s)
=== RUN   TestParseListenerFiltersUnknownTypeURLErrors
--- PASS: TestParseListenerFiltersUnknownTypeURLErrors (0.00s)
=== RUN   TestParseListenerFiltersTimeoutInRange
--- PASS: TestParseListenerFiltersTimeoutInRange (0.00s)
=== RUN   TestParseListenerFiltersTimeoutDefault
--- PASS: TestParseListenerFiltersTimeoutDefault (0.00s)
=== RUN   TestParseListenerFiltersTimeoutBelowFloorErrors
--- PASS: TestParseListenerFiltersTimeoutBelowFloorErrors (0.00s)
=== RUN   TestParseListenerFiltersTimeoutAboveCapErrors
--- PASS: TestParseListenerFiltersTimeoutAboveCapErrors (0.00s)
=== RUN   TestParseChainSpecMixedTLSPreserved
--- PASS: TestParseChainSpecMixedTLSPreserved (0.00s)
=== RUN   TestIdenticalFilterChainsErrorWithAmbiguousSelection
--- PASS: TestIdenticalFilterChainsErrorWithAmbiguousSelection (0.00s)
=== RUN   TestParseDefaultFilterChain_Plaintext_WithTLSFilterChain
--- PASS: TestParseDefaultFilterChain_Plaintext_WithTLSFilterChain (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener	1.024s
$ grep -nE 'chainSpecificityRank|^func validateFilterChainMatch' internal/listener/manager.go
278:// ADR-0078 supersession (Task 9): the previous phase-03 `validateFilterChainMatch`
$ go test -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	2.326s
ok  	github.com/esalaine/envoy-go/internal/listener	0.023s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	0.044s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	0.003s
[every other package PASS — full output omitted]
```

**Follow-up commit (review-driven fix):** Code-quality reviewer flagged I-1 (chainSpecKey did not canonicalize slice order, causing duplicate-detection to miss semantically-identical chains differing only by slice permutation; matches() is set-based so this could surface as runtime ErrAmbiguousChainMatch on the first matching connection — worse failure mode than a boot-time error) and two test-coverage gaps (I-2 errUnwrapFilterChain unwrap, I-3 nil-lfRegistry-with-non-empty-listener_filters). Fix: chainSpecKey now sorts ServerNames / ApplicationProtocols / SourcePorts / PrefixRanges / SourcePrefixRanges (via copies — ChainSpec is immutable post-build) before serializing. Added three new tests covering the I-1, I-2, I-3 surfaces. Per ADR-0017 (small-mechanical-fixes do not require ADRs), no new ADR.

## Task 10 — internal/listener/manager.go (acceptLoop + serveConnection refactor)

**Commits:** TBD — this task's commit
**Notes:** Replaced the SNI-only post-handshake `dispatch` function (and its helpers `serveTLS` + `makeGetConfigForClient`) with the unified pre/post-handshake dispatch path per SPEC §5.2: a new `serveConnection` helper owns the per-connection lifecycle — (1) ChainMatchInputs from `LocalAddr/RemoteAddr` via four small helpers `localIP`/`localPort`/`remoteIP`/`remotePort`, (2) wrap raw conn in `peekerConn`, (3) construct per-connection ListenerFilter instances from `rt.listenerFilterFactories`, (4) `Pipeline.Run` with the listener-filter pipeline (continue-on-timeout policy honored per ADR-0082), (5) `listenerfilter.SelectChain` (8-dimension chain-match per ADR-0081), (6) handshake against the SELECTED chain's per-chain `*stdtls.Config` (no listener-level GetConfigForClient callback — chain selection now happens BEFORE the handshake, eliminating the SNI-callback shortcut), (7) terminal-filter dispatch on the post-handshake `*tls.Conn` for TLS chains or the raw peekerConn for plaintext. `acceptLoop` reduced to the Inc/Dec discipline + `go rt.serveConnection(ctx, raw)`. Deletions: `dispatch` + `serveTLS` + `makeGetConfigForClient` (3 functions, ~50 LoC; folded into `serveConnection`); `orderLegacyChains` (Task-9 stand-in for the legacy SNI-only chain ordering, no remaining callers); `listenerRuntime.tlsCfg` + `listenerRuntime.chains []*chainInfo` fields (the listener-level shared `*stdtls.Config` is gone — each chainInfo carries its own per-chain config; the legacy chain-ordered slice is replaced by `chainSpecs` + `chainByName` populated at Task 9 build time). Per-connection OnDestroy on every constructed listener-filter instance is handled by `Pipeline.Run` itself (its deferred loop, pipeline.go lines 33-37) per ADR-0079; `serveConnection` does not need to re-defer that. TDD discipline: five new tests added (`TestUnifiedDispatchPlaintextChainSelectByDestPort`, `TestUnifiedDispatchTLSWithSNI`, `TestUnifiedDispatchDefaultFilterChainFallback`, `TestUnifiedDispatchListenerFilterTimeoutAbortsConnection`, `TestUnifiedDispatchListenerFilterTimeoutContinue`); existing SNI-via-GetConfigForClient tests rewritten to consult `listenerfilter.SelectChain` directly through a new `selectByServerNameFromMgr` helper (the pre-Task-10 `getConfigForClientFromMgr` helper is gone with the callback). `TestNewManager_ChainSelectionPropagation` updated to declare `listener_filters: [tls_inspector]` (the unified path requires explicit tls_inspector for SNI extraction per ADR-0079; the legacy crypto/tls.GetConfigForClient SNI shortcut is no longer wired). All listener-package tests PASS under `-race`.

**Follow-up fix included in same commit (tls_inspector deadlock):** Surfaced by Task 10's switch to listener-filter-pipeline-driven SNI extraction on real network connections: the pre-existing `tls_inspector.Inspect` called `peeker.Peek(bufferSize)` with `bufferSize` defaulting to 4096, but `bufio.Reader.Peek(n)` blocks until n bytes are available — a typical ClientHello is ~250-350 bytes, so Peek(4096) on a real TCP connection waits forever for bytes the client will never send (the client is waiting for ServerHello). This deadlocked `TestNewManager_ChainSelectionPropagation` and the new TLS dispatch tests with `EOF` / handshake-timeout. Fix: `tls_inspector.Inspect` now does an incremental peek — first 5 bytes for the TLS record header (which `bufio.Reader.Peek(5)` returns as soon as 5 bytes have arrived), then computes `5 + recordLen` (capped at bufferSize), then peeks exactly that count. The pre-existing tls_inspector unit tests use `net.Pipe()` + client-side `Close()` to flush EOF so they didn't surface the deadlock; the dispatch test uses real `net.Listen` + concurrent client which does. Adds one `encoding/binary` import to `tls_inspector.go`. Per ADR-0017 (small-mechanical-fixes do not require ADRs), no new ADR — this is a correctness fix to the Task-7/8-introduced filter exposed by Task-10's hot path.

**Differential note (Task-10 → Task-11 boundary):** Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log`, `0007a-cors`, `0007b-iteration-probe` all PASS at Task 10. Fixture `0002-tls-tcp` regresses at Task 10 because the unified dispatch requires explicit `tls_inspector` listener-filter declaration for SNI extraction — but the bootstrap parser does not yet have the `tls_inspector v3` proto blank-imported (Task 11 owns that wiring per PLAN line 2246-2253), AND the `0002-tls-tcp/driver/driver.go:SubjectConfig` does not yet declare `listener_filters: [tls_inspector]` in the subject yaml (the original yaml comment "envoy-go reads SNI natively via crypto/tls GetConfigForClient" is now stale). Task 11's commit completes the production-code surface AND, per PLAN line 2272 ("pre-existing fixtures must be re-runnable from THIS commit"), restores fixture-0002 — either by Task-11 itself updating the 0002 driver to declare tls_inspector, or by a Task-11 follow-up. The 8 non-0002 fixtures' green state at Task 10 is direct evidence that the unified dispatch is differentially equivalent for the chains-with-no-SNI-match cases per SPEC §6.1 gate (b) preservation claim.
**Outputs:**
```
$ go vet ./internal/listener/...
$ golangci-lint run ./internal/listener/... ./internal/listener/listenerfilter/...
$ go test -race ./internal/listener/...
ok  	github.com/esalaine/envoy-go/internal/listener	3.055s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	(cached)
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.012s
$ grep -nE '^func.*serveConnection|^func.*acceptLoop' internal/listener/manager.go
783:func (rt *listenerRuntime) acceptLoop(ctx context.Context, ln net.Listener) {
830:func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {
$ grep -nE '^func.*dispatch[^A-Za-z]|^func.*serveTLS|^func.*makeGetConfigForClient|^func orderLegacyChains' internal/listener/manager.go
[empty — all 4 deleted]
$ go test -count=1 -short ./...
[every package PASS — full output omitted]
$ for fx in 0000 0001 0003 0004 0005 0006 0007a 0007b; do go test -count=1 -v ./test/differential/ -run "TestDifferential/$fx"; done
[all 8 fixtures PASS]
$ go test -count=1 -v ./test/differential/ -run "TestDifferential/0002"
[FAIL — handshake EOF; expected boundary effect, resolved at Task 11 per PLAN line 2272]
```

## Task 11 — cmd/envoy-go/main.go + internal/bootstrap/bootstrap.go (boot wiring)

**Commits:** TBD — this task's commit
**Notes:** Wired the boot-time `*listenerfilter.ListenerFilterRegistry` per ADR-0079: `main()` now allocates the registry, registers `tls_inspector.New` under `tls_inspector.TypeURL`, calls `Freeze()`, and threads the frozen registry into `listener.NewManagerWithBaseDirAndAllowH2C` (replacing the Task-9 `nil` placeholder). Two new imports landed in `cmd/envoy-go/main.go`: `internal/listener/listenerfilter` and `internal/listener/listenerfilter/tls_inspector`. Added one new blank-import to `internal/bootstrap/bootstrap.go`: `envoy/extensions/filters/listener/tls_inspector/v3` — without it `protojson` would error "type not registered" when parsing a bootstrap that declares `listener_filters: [{name: envoy.filters.listener.tls_inspector, typed_config: {"@type": ...TlsInspector}}]`. Per ADR-0016 amendment policy this addition is documented in PROGRESS, not as a new ADR (mirrors phase 04 / 05.2 / 06.2 / 07.1 blank-import precedent). New tests: `TestBootstrap_RoundTrips_TLSInspectorListenerFilter` (round-trips a minimal bootstrap carrying a `tls_inspector v3` typed_config through `Load` + `protojson.Marshal`; would error pre-blank-import) and `TestEnvoyGoBinary_TLSInspectorBootWiring` (boots the binary on a bootstrap declaring `listener_filters: [tls_inspector]` and asserts the `l_tls` ready sentinel emits — exercises the full Task-11 wiring chain end-to-end: blank-import + Registry alloc + Register + Freeze + threading + listener-build-time Lookup).

**Task-10 boundary-effect resolution (authorized PLAN deviation per ADR-0017 small-mechanical-fixes):** Task 10's accept-loop refactor deleted `crypto/tls.GetConfigForClient` so the unified pre-handshake dispatch path requires an explicit `tls_inspector` listener filter for SNI extraction. This caused fixture-0002-tls-tcp to regress (RED at Task-10 HEAD: TLS handshake EOF — the subject yaml had no `listener_filters[]` and SNI was never populated). Updated `test/fixtures/0002-tls-tcp/driver/driver.go`: the `buildBootstrap` helper now emits the same `listener_filters: [tls_inspector]` block for both reference and subject (previously emitted only for the reference path); the subject's `SubjectConfig` doc comment + the documentation-only `test/fixtures/0002-tls-tcp/envoy-go.yaml` were updated to remove the stale "envoy-go reads SNI natively via crypto/tls GetConfigForClient (no tls_inspector needed or parsed)" comment and replace it with a phase-07.2 ADR-0079 reference. PLAN.md (lines 2215-2274) does not explicitly mention this fixture-0002 update; it is the resolution of the boundary effect documented in the Task-10 PROGRESS entry's "Differential note" and authorized by PLAN line 2272 ("pre-existing fixtures must be re-runnable from THIS commit") plus ADR-0017's small-mechanical-fixes carve-out.
**Outputs:**
```
$ go vet ./...
$ golangci-lint run ./...
$ go test -race -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	3.895s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.019s
ok  	github.com/esalaine/envoy-go/internal/admin	1.080s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.067s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.068s
[every other package PASS — full output omitted]
$ go test -count=1 -v ./test/differential/ -run 'TestDifferential/(0000|0001|0002|0003|0004|0005|0006|0007)'
--- PASS: TestDifferential (22.00s)
    --- PASS: TestDifferential/0000-tcp-echo (1.54s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.29s)
    --- PASS: TestDifferential/0002-tls-tcp (1.26s)
    --- PASS: TestDifferential/0003-http11-routing (1.19s)
    --- PASS: TestDifferential/0004-h2-routing (1.76s)
    --- PASS: TestDifferential/0005-prometheus-stats (1.93s)
    --- PASS: TestDifferential/0006-access-log (10.91s)
    --- PASS: TestDifferential/0007a-cors (1.35s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.77s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	22.082s
$ grep -nE 'lfReg|lfRegistry' cmd/envoy-go/main.go
118:	lfReg := listenerfilter.NewListenerFilterRegistry()
119:	lfReg.Register(tls_inspector.TypeURL, tls_inspector.New)
120:	lfReg.Freeze()
122:	lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C, bs.Stats, sinks, httpReg, lfReg)
$ grep -n tls_inspector internal/bootstrap/bootstrap.go
55:	// Phase 07.2 (Task 11) registers the tls_inspector listener-filter
57:	// `listener_filters: [{name: envoy.filters.listener.tls_inspector,
63:	// for SNI-indexed filter chains MUST declare tls_inspector explicitly
67:	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
```

## Task 12 — internal/listener/integration_test.go (end-to-end unit test)

**Commits:** TBD — this task's commit
**Notes:** Added a single `TestIntegration` parent with 5 table-driven subtests exercising the full Task-10 unified pre/post-handshake dispatch path (Manager.Start + acceptLoop + serveConnection) against a real `net.Listen` bound on 127.0.0.1:0 + real `net.DialTimeout` from the test goroutine. Subtests cover the four chain-match decisions called out by PLAN.md Task 12 plus the listener-filter pipeline timeout abort: (i) `match_dstport_only` — `chain_dstport_only` matches the bound port, `chain_srcprefix_only` is `10.0.0.0/8` (loopback dialer no-match) → tag 'D'; (ii) `match_srcprefix_only` — `chain_dstport_only` is `bound_port+1` (no-match), `chain_srcprefix_only` is `127.0.0.1/32` → tag 'S'; (iii) `match_both_dstport_wins` — both chains match; per ADR-0081 priority vector destination_port (slot 0) outranks source_prefix_ranges (slot 6) → tag 'D'; (iv) `match_neither_falls_to_default` — neither specific chain matches, `default_filter_chain` wins → tag 'X'; (v) `listener_filters_timeout_abort` — single chain + 1s lfTimeout + slow listener filter that blocks 2s + continue=false → conn closed by the listener (Read returns EOF/non-nil err, no tag delivered). Each subtest probe-listens on `127.0.0.1:0` to learn the OS-assigned port, then re-binds the Manager on the same port (race-free because the kernel won't recycle the 4-tuple in the µs gap). Three new helpers local to `integration_test.go`: `mkChainsListener` (2-chain + default-chain listener constructor), `threeClusterMgr` (3-cluster cousin of manager_test.go's `twoClusterMgr`), `mkStaticCluster` (factored from threeClusterMgr to keep it small). Reuses existing manager_test.go helpers `startTaggedBackend`, `readByteWithTimeout`, `mkTcpProxyFilter`, `mkBoot`, `testHTTPRegistry`, `installSlowListenerFilter` (all in same package, no exports needed). The TLS+SNI dispatch dimension is already covered by `TestUnifiedDispatchTLSWithSNI` (Task 10) so this file focuses on the plaintext + listener-filter-timeout matrix per the PLAN line 2287 enumeration. All 5 subtests PASS under `-race`; aggregate runtime ~1s.

**Outputs:**
```
$ go vet ./internal/listener/...
$ golangci-lint run ./internal/listener/...
$ go test -race ./internal/listener/... -run TestIntegration -v
=== RUN   TestIntegration
=== RUN   TestIntegration/match_dstport_only
=== RUN   TestIntegration/match_srcprefix_only
=== RUN   TestIntegration/match_both_dstport_wins
=== RUN   TestIntegration/match_neither_falls_to_default
=== RUN   TestIntegration/listener_filters_timeout_abort
--- PASS: TestIntegration (1.01s)
    --- PASS: TestIntegration/match_dstport_only (0.00s)
    --- PASS: TestIntegration/match_srcprefix_only (0.00s)
    --- PASS: TestIntegration/match_both_dstport_wins (0.00s)
    --- PASS: TestIntegration/match_neither_falls_to_default (0.00s)
    --- PASS: TestIntegration/listener_filters_timeout_abort (1.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener	2.021s
$ go test -race -short -count=1 ./...
[every package PASS — full output omitted]
$ wc -l internal/listener/integration_test.go
294 internal/listener/integration_test.go
```
