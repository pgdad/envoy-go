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
