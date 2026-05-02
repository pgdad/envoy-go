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
