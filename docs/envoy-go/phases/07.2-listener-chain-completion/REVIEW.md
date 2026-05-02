# Phase 07.2 — Code review (REVIEW.md)

**Phase id:** `07.2`
**Slug:** `07.2-listener-chain-completion`
**Branch under review:** `phase/07.2-listener-chain-completion-impl`
**Range:** `9627855..10b0aba` (21 commits — 17 plan tasks + 2 review-driven follow-ups + 1 closing follow-up + 1 verification commit + the SHA-fill follow-up)
**Reviewer skill:** `superpowers:requesting-code-review` (final REVIEW per BOOTSTRAP §5 step 6, lifecycle-state 5 → 6)
**Six-gate state at HEAD:** all green per the verification commit `10b0aba` (gate (a) fixture 0008 PASS; gate (b) 0000–0007b PASS; gate (c) h2spec 53/53 at the ADR-0051 pin; gate (d) all 10 fuzzers clean; gate (e) `go build/vet/golangci-lint/go test -race ./...` clean; gate (f) is THIS document).

This review covers the full 6 500-LoC phase 07.2 surface — a new `internal/listener/listenerfilter/` package tree, a substantial refactor of `internal/listener/manager.go` (chain parsing + dispatch path), a new `tls_inspector` listener filter, a new differential fixture `0008-listener-chain-match`, two new optional driver interfaces in the harness, BEHAVIOR_CONTRACT.md amendments, and seven new ADRs (ADR-0077..ADR-0083).

---

## 1. Final assessment

**APPROVED.**

All six phase-done gates are GREEN at HEAD; the implementation faithfully realizes the SPEC across all 17 PLAN tasks; the two review-driven mid-flight self-corrections (Task 5 `breakTie` priority-order fix; Task 9 `chainSpecKey` slice-canonicalization fix) are correct and well-tested; the two ADR-anchored empirical pins (ADR-0080 §11.1+§11.2, ADR-0081 §11.3) plus the SPEC §11.4 carry-forward (resolved at Task 16 per Decision K) are paste-verbatim-synchronized into BEHAVIOR_CONTRACT.md `## Listener filters`; ADR discipline is exemplary with all seven anticipated ADRs landed at the right tasks and full Context/Decision/Consequences sections; supersession enumeration (ADR-0078) is precise and grep-discoverable; doctrine adherence (D-3.2 no-cgo for the ClientHello parser; D-3.3 differential correctness via fixture-0008; D-3.5 ADRs for ambiguities; D-3.6 green build) is uniform across the phase; and the boundary grep for third-party listener-filter dependencies returns zero matches per the SPEC §16 acceptance bullet.

The minor issues identified below are NOT blockers; they are documentation/test-coverage sharpenings that the parent session may choose to carry forward into a future hardening pass. The phase-done commit may proceed.

---

## 2. Strengths

### 2.0 Phase shape and scope discipline

Phase 07.2 closes a load-bearing parent ROADMAP row (07-filter-chain-framework) at the same phase-done commit per the parent SPEC §5 closure pattern that 06.2 + 05.2 established. The split between 07.1 (HTTP-filter framework) and 07.2 (listener-chain completion) was anchored at brainstorm time per ADR-0070; 07.2 is the second sub-phase under that split. Three dimensions show that the scope discipline was honored:

- **No 07.1 surface touched in 07.2.** The implementation lives entirely under `internal/listener/` and `cmd/envoy-go/main.go`; no production-code edit touches the 07.1 `internal/filter/http/` package. The two phases are architecturally independent per the parent BRAINSTORM §1 split-table — 07.2's only contact with 07.1 is the constructor-signature widening at `NewManagerWithBaseDirAndAllowH2C` to thread the `*ListenerFilterRegistry`, which is the same mechanical pattern 07.1 used for `*HTTPRegistry`.
- **Deferred items are well-enumerated**. `original_dst`, `proxy_protocol`, `http_inspector` listener filters; `direct_source_prefix_ranges` chain-match dimension; xDS LDS dynamic listener configuration; HTTP/3 + QUIC listener filters; per-listener-filter metrics; CEL-driven `filter_disabled` — every one of these is documented in SPEC §2 with its deferral rationale. ADR-0077 records the scope decision; the silent-ignore set extension under SPEC §12 mirrors the ADR-0041 amendment policy.
- **Single fixture, non-vacuous.** SPEC §9 + Decision G commit to a single fixture (`0008-listener-chain-match`) — the chain-match surface fits in one fixture (5 connections cover the priority-ordering corners). The 07.1 precedent of two fixtures (`0007a` + `0007b`) was driven by the differential-vs-structural split; 07.2 has no analogous split. This is the right discipline for a sub-phase whose central engineering claim ("envoy-go runs a real listener-filter dispatch pipeline before HCM and matches downstream filter chains across 8 dimensions") is provable with a single dual-listener fixture.

### 2.1 Algorithm correctness — `SelectChain` 8-dimension precedence

The heart of phase 07.2 is `internal/listener/listenerfilter/chainmatch.go`'s `SelectChain` function (334 LoC). The algorithm implements SPEC §5.5 + §7.3's 2-pass eligibility-then-specificity contract:

- **Pass 1** (`matches` helper, lines 115-147) admits any chain whose every non-zero dimension is satisfied; empty-match chains short-circuit to true. The eligibility predicate is tight — every priority dimension is checked exactly once with a small set of helper predicates (`ipInAny`, `portInAny`, `sniMatchAny`, `alpnMatchAny`).
- **Pass 2** (`specificityScore`, lines 153-183) computes an 8-bit specificity vector with bit `prioCount-1-i` set when priority slot `i` is specified. The bit-ordering puts the most-significant-bit on the highest-priority dimension so a numerical compare reflects the priority order — clean, fast, and obviously correct on inspection.
- **Tie-breaking** (`breakTie`, lines 201-238) cascades through the priority slots that have a meaningful sub-ordering (PrefixRanges → ServerNames → SourcePrefixRanges) and returns nil when chains are indistinguishable, which propagates as `ErrAmbiguousChainMatch` from `SelectChain`.

The algorithm is O(N × D) with D = 8 constant, so per-connection dispatch is microseconds-scale even for hundreds of chains; the SPEC §11.3 + ADR-0081 empirical pin against Envoy v1.37.2 (`destination_port: 10000` BEATS `source_prefix_ranges: 127.0.0.1/32` on a connection from 127.0.0.1 to port 10000) is realized correctly by `TestSelectChainDestinationPortBeatsSourcePrefix`. The SPEC §11.2 + ADR-0080 empirical pin (empty-match chain BEATS `default_filter_chain` when both coexist) is realized correctly by `TestSelectChainEmptyMatchBeatsDefault` — the universal-eligibility-of-empty-match invariant is encoded in line 116-118's `c.Empty` short-circuit and the no-default-fallback when ANY chain is eligible.

### 2.2 The Task 5 review-driven `breakTie` priority-order fix

The first mid-flight self-correction was load-bearing for SPEC compliance. The original Task 5 commit `b36f251` shipped `breakTie` with cascade order PrefixRanges → SourcePrefixRanges → ServerNames — PLAN-verbatim but contradicting SPEC §5.5 line 519 ("walk down the priority list with finer-grain tie-breakers") and §7.3 line 524's "more-specific value within the highest-priority dimension where they differ in specifics" rule. The reviewer caught it; the follow-up commit `12969ed` re-ordered to PrefixRanges (slot 1) → ServerNames (slot 2) → SourcePrefixRanges (slot 6) and added four new tests:
- `TestSelectChainBreakTieFollowsPriorityOrder` — the multi-dimension counter-example (a chain with `*` ServerNames + tighter SourcePrefixRanges loses to a chain with exact ServerNames + looser SourcePrefixRanges).
- `TestSelectChainAmbiguousReturnsError` — closes the ambiguous-selection acceptance criterion (originally missing from the unit test set).
- `TestSelectChainTransportProtocol` + `TestSelectChainSourcePorts` — exercise the two priority dimensions previously not covered.

The fix is correct: by visiting `ServerNames` BEFORE `SourcePrefixRanges`, the cascade respects the SPEC's priority ordering. Per `internal/listener/listenerfilter/chainmatch.go:213-222`, the implementation now reads "Slot 2 — ServerNames: SNI specificity" and gives chain `b` the win in the test scenario. The doc comment on `breakTie` (lines 188-200) explicitly cites both SPEC §5.5 line 519 and §7.3 line 524, which is the right discipline — future readers can grep to those specific SPEC lines.

### 2.3 The Task 9 review-driven `chainSpecKey` slice canonicalization fix

The second mid-flight self-correction was a subtler correctness issue. The original Task 9 commit `ee45fd35` shipped `chainSpecKey` (used by `findIdenticalChainSpecs` for boot-time ambiguous-selection detection) without sorting the multi-element slice fields. Because `matches()` is set-based (uses `ipInAny`/`alpnMatchAny`/`sniMatchAny`/`portInAny`), two chains differing ONLY in slice declaration order — e.g., `ServerNames: ["a","b"]` vs `["b","a"]` — are semantically identical at runtime, but the original `chainSpecKey` would emit different keys. The boot-time duplicate-check would miss them; at runtime the first matching connection would hit `breakTie` returning nil → `ErrAmbiguousChainMatch` was a worse failure mode than a clean boot-time error.

The follow-up commit `70ff414` rewrote `chainSpecKey` (manager.go lines 657-711) to sort `ServerNames`, `ApplicationProtocols`, `SourcePorts`, `PrefixRanges`, and `SourcePrefixRanges` before serialization — operating on copies because `ChainSpec` is documented immutable post-build. The new key is canonical: any two chains semantically identical by `matches()` produce equal keys. Three new tests cover the I-1, I-2, I-3 surfaces. Per ADR-0017 (small-mechanical-fixes do not require ADRs) no new ADR was issued — appropriate, because the fix aligns code with the SPEC's already-documented set-based matching semantics.

### 2.4 The unified pre/post-handshake dispatch refactor (Task 10)

The dispatch refactor (manager.go `serveConnection`, lines 830-891) is the most architecturally significant change in the phase. Pre-Task-10 the dispatch was split: a `crypto/tls.GetConfigForClient` callback re-ran SNI-only chain match at handshake time and chose the per-chain `*tls.Config`; post-handshake `dispatch` re-ran the same SNI lookup. Task 10 unifies these into a single 7-step lifecycle:

1. ChainMatchInputs from connection-level facts (`localIP/localPort/remoteIP/remotePort` helpers, lines 898-924).
2. Wrap raw conn in `peekerConn`.
3. Construct per-connection ListenerFilter instances from per-listener factories.
4. Run pipeline with `continue_on_listener_filters_timeout` honored.
5. `SelectChain` over `chainSpecs/defaultSpec` to pick the chain BEFORE handshake.
6. If the selected chain has TLS, run `HandshakeContext` against the chain's per-chain `*stdtls.Config`.
7. Dispatch to the chain's terminal filter.

The doc comment on `serveConnection` (lines 804-829) is exemplary — it spells out the 7-step lifecycle and explicitly notes "Per ADR-0079 the listener-filter Pipeline.Run already calls OnDestroy on every constructed filter instance via its own deferred loop — the helper does NOT need to re-defer that here." That kind of cross-reference between manager.go and pipeline.go reduces future-reader confusion.

The deletions (the SPEC-anticipated removals of `dispatch` + `serveTLS` + `makeGetConfigForClient` + `chainSpecificityRank` + `orderLegacyChains` + `listenerRuntime.tlsCfg` + `listenerRuntime.chains []*chainInfo`) are clean — Task 10's `grep -nE '^func.*dispatch[^A-Za-z]|^func.*serveTLS|^func.*makeGetConfigForClient|^func orderLegacyChains' internal/listener/manager.go` returns empty per the PROGRESS Task 10 output, which is the right shape for the SPEC §5.7 supersession.

### 2.5 The Task 10 `tls_inspector.Inspect` deadlock fix

A subtle but important correctness fix landed in the Task 10 commit (per the PROGRESS "Follow-up fix included in same commit" entry): the original `tls_inspector.Inspect` peek of `bufferSize` bytes (default 4096) deadlocked on real TCP connections because `bufio.Reader.Peek(n)` blocks until n bytes are available, and a typical ClientHello is 250-350 bytes — so Peek(4096) waits forever for bytes the client will never send.

The fix in `tls_inspector.go` (lines 54-98) is the right shape: incremental peek, first 5 bytes for the TLS record header, then `5 + recordLen` (capped at bufferSize) for the actual ClientHello. The doc comment cites both the deadlock surface and the cap-by-bufferSize tolerance for pathological clients. The pre-existing parser tests use `net.Pipe()` with client-side `Close()` so they didn't surface the deadlock; the Task-10 dispatch test (real `net.Listen` + concurrent client) does. The fix imports `encoding/binary` and adds zero new public surface.

### 2.6 Test coverage

Test coverage across the phase is comprehensive:

- **`internal/listener/listenerfilter/`** carries 14 chainmatch_test cases (10 original + 4 review-driven), 7 pipeline_test cases, 5 registry_test cases, 3 callbacks_test cases, 3 types_test cases, plus the `FuzzFilterChainMatch` (10th repo-wide fuzzer).
- **`internal/listener/listenerfilter/tls_inspector/`** carries 7 parser tests (round-tripped through real `crypto/tls.Conn` ClientHello bytes via the `captureClientHello` helper — robust against future TLS spec drift), 6 tls_inspector unit tests (full filter behavior + race-clean concurrent inspection), 6 proto-config tests.
- **`internal/listener/manager.go`** test set extended with 13+ new test functions covering parseChainSpec acceptance of all 8 dimensions, default_filter_chain handling, listener_filters resolution + timeout envelope, ambiguous-selection detection, mixed-TLS preservation, and the unified dispatch path's plaintext + TLS subtests.
- **`internal/listener/integration_test.go`** (294 LoC) is a NEW end-to-end accept-loop test with 5 subtests covering the four chain-match decisions (D-only, S-only, both-D-wins, default-fallback) plus the listener-filter timeout abort.
- **Differential fixture 0008** is the load-bearing differential pin: 5-connection workload across the dual-listener construction (per SPEC §7.4) demonstrating destination_port BEATS source_prefix_ranges per §11.3 AND default_filter_chain fallback per §11.1, byte-equal subject vs reference Envoy v1.37.2. Per the PROGRESS Task 16 entry "PASS first try" — non-vacuous evidence that the algorithm + dispatch path align with reference Envoy.

The test coverage matches the SPEC §15 testing-strategy enumeration item-for-item — no skipped categories.

### 2.7 Empirical pin discipline

All four §11 empirical-pin blocks are paste-verbatim-synchronized between SPEC §11 and BEHAVIOR_CONTRACT.md `## Listener filters`. Three pins (§11.1 default_filter_chain fallback, §11.2 empty-match-vs-default, §11.3 destination_port-vs-source_prefix_ranges precedence) were captured at SPEC time by the planner against Envoy v1.37.2 server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`; the fourth (§11.4 tls_inspector-populated ALPN feeds application_protocols) was carry-forward per Decision K and resolved at Task 16 against the same image SHA. The PROGRESS Task 16 entry includes the verbatim Envoy probe outputs (probe (a) ALPN h2 → tcp_h2 ticked; probe (b) ALPN http/1.1 → tcp_h1 ticked; tls_inspector per-filter stats `tls_found:2 alpn_found:2 sni_not_found:2`) — durable evidence in the SPEC, the PROGRESS, and the BEHAVIOR_CONTRACT.

The boundary grep `grep -rE 'github.com/.*listener.*filter|github.com/.*chain.*match' . --include='*.go' --include='go.mod' --include='go.sum' | grep -v 'envoyproxy/go-control-plane' | grep -v 'envoy-go'` returns zero matches at HEAD, confirming the SPEC §16 acceptance bullet ("No third-party listener-filter or chain-match library is imported"). The hand-rolled minimal ClientHello parser per D-3.2 (no cgo) is the right discipline; it adapts `crypto/tls/handshake_messages.go:unmarshal` for the ClientHello case narrowed to two extension types (`0x0000` server_name + `0x0010` ALPN), which keeps the parser ~140 LoC and dependency-free.

### 2.8 ADR discipline

All seven anticipated ADRs (ADR-0077..ADR-0083) landed at the right tasks per the SPEC §10 mapping:

- **ADR-0077** (scope decision; Task 1, commit `66300ba`) — anchors the ROADMAP row-flip already landed at the SPEC commit; mirrors ADR-0070's 07.1 scope-confirmation pattern.
- **ADR-0083** (ADR-0050 disposition; Task 1, same commit) — explicit non-supersession of ADR-0050 with the rationale "chain-selection vs codec-selection" orthogonal mechanisms.
- **ADR-0079** (dispatch protocol shape; Task 2, commit `814ba67`) — the load-bearing decision for the listener-filter framework: sync-only Inspect, freeze-after-boot registry, two-step factory pattern, 4096-byte peeker default clamped [256, 65536].
- **ADR-0082** (listener_filters_timeout envelope; Task 4, commit `19bb90d`) — [1s, 60s] envelope, 15s default, continue_on_listener_filters_timeout honored, per-pipeline shared budget per Decision N.
- **ADR-0080** (default_filter_chain semantics; Task 5, commit `b36f251`) — supersedes ADR-0033 clause 3; pinned to SPEC §11.1 + §11.2 empirical evidence.
- **ADR-0081** (8-dim precedence algorithm; Task 5, same commit) — partially supersedes ADR-0033 clause 2; pinned to SPEC §11.3 empirical evidence.
- **ADR-0078** (ADR-0033 partial supersession enumeration; Task 9, commit `ee45f35`) — the canonical clause-by-clause table; consolidates the supersession relationship that ADR-0079/0080/0081 would otherwise leave scattered.

Each ADR has full Context/Decision/Consequences sections per ADR-0001's template; each names its `Lands-in-task` field; ADR-0078 is the kind of consolidating ADR that is grep-discoverable from EITHER side ("future reader of ADR-0033 sees the supersession relationship via grep on `Supersedes (partial): ADR-0033`"). Topical-vs-commit-order non-monotonicity is permitted and recorded — ADR-0080 + ADR-0081 land BEFORE ADR-0078 because they anchor production code first; ADR-0078 is then the consolidating note. This is the same pattern 06.2 + 07.1 used.

### 2.9 BEHAVIOR_CONTRACT.md discipline

The Task 17 in-place edit per ADR-0052 (commit `c4bcd02`) is well-executed:

- New `## Listener filters` section added between `## TLS` and `## HTTP filter chain` per the SPEC §13.1 layout. Sub-sections: `### Asserted equivalence`, `### Not asserted`, `### Listener-filter dispatch protocol`, `### Chain-match algorithm`, `### default_filter_chain semantics`, then four `### Empirical evidence (...)` blocks paste-verbatim from SPEC §11.1/§11.2/§11.3/§11.4.
- New `## Equivalence Matrix` row for "Listener filters" added.
- `## TCP proxy "Does not yet apply to"`: the "Filter chain matching (filter_chain_match non-empty) — phase 07" entry is REMOVED; the "Multiple filters in a chain" entry is rewritten to clarify network-filter-family vs listener-filter-pipeline scope.
- `## TLS "Scope boundaries"`: the four entries "ALPN-driven filter-chain selection", "non-SNI filter-chain match fields", "Listener.default_filter_chain", "listener_filters (still silently skipped)" are all REMOVED from the deferred list with a forward-pointer to `## Listener filters` and an explicit ADR-0050 / ADR-0083 coexistence-not-supersession paragraph.
- `## HTTP filter chain "Does not yet apply to"`: the three entries "Listener filters", "FilterChainMatch beyond SNI", "Listener.default_filter_chain" REMOVED.

The paste-verbatim discipline between SPEC §11 and BEHAVIOR_CONTRACT.md is the right shape — the PROGRESS Task 17 entry confirms the four `grep -A5 '^### Empirical evidence'` checks find paste-verbatim blocks. Future image bumps per ENVOY_TARGET refresh procedure that alter any of the four shapes will require updating both locations in the same commit (mirrors 06.1/06.2/07.1 paste-verbatim discipline).

### 2.10 Differential fixture 0008 design

The fixture-0008 design (per SPEC §7.4 + Decision G) is technically subtle but gets it right:

- **Dual-listener construction** (`l_test_a` + `l_test_b` on distinct ports) — required to exercise the `destination_port` priority dimension, because a single-port listener cannot make destination_port a discriminator. The SPEC commits to the dual-listener shape; the planner refines the YAML at PLAN time.
- **Connection-4-only c4 variant** — alternate bootstrap that omits `chain_other` so that connection 4 (`l_test_b` from non-loopback source) falls through to `default_filter_chain`. The variant-driven approach keeps the fixture clean (one `expectations.yaml`; two `envoy*.yaml` pairs).
- **5-connection per-connection assertion**: response body is the backend's listener address as a string (deterministic per port → distinct per backend → distinct per chain). Per-connection (subjectResponse, referenceResponse) byte-equality is the differential claim.
- **Source-IP cross-side compatibility** (per the PROGRESS Task 16 surface adjustment): the static envoy.yaml + envoy-c4.yaml use `source_prefix_ranges: [127.0.0.1/32]` for documentation; the driver-embedded const templates use `0.0.0.0/0` because Docker bridge networking changes the source IP visibility. The discriminator becomes `source_ports: [known_driver_port]`. The chain still has slots 6+7 specified, so the §11.3 precedence demonstration (connection 5: chain_dstport_alpha at slot 0 BEATS chain_srcprefix_loopback at slots {6,7}) is unchanged. This is documented in the driver doc-comment.

The fixture PASS-first-try at Task 16 is non-vacuous evidence that envoy-go's chain-match algorithm matches Envoy v1.37.2 across a 5-connection workload spanning four chain-match decisions. This is the right shape for gate (a).

### 2.11 The Task 10/11 boundary effect resolution

The Task 10/11 boundary effect — fixture-0002-tls-tcp transient regression — is well-documented and correctly resolved:

- **Cause**: Task 10's accept-loop refactor deleted `crypto/tls.GetConfigForClient` so the unified pre-handshake dispatch path requires an explicit `tls_inspector` listener filter for SNI extraction. But the bootstrap parser at Task-10 HEAD did not yet have the tls_inspector v3 proto blank-imported (Task 11's domain), AND the 0002 driver did not yet declare `listener_filters: [tls_inspector]`.
- **Documentation**: The Task 10 PROGRESS entry has an explicit "Differential note (Task-10 → Task-11 boundary)" block that names the regression, the cause, and the Task-11 resolution path. The Task-10 outputs include a verbatim `[FAIL — handshake EOF; expected boundary effect, resolved at Task 11 per PLAN line 2272]` — the right shape for an in-PROGRESS-flight transient.
- **Resolution at Task 11**: commit `85e8a74` updates `test/fixtures/0002-tls-tcp/driver/driver.go`'s `buildBootstrap` helper to emit the `listener_filters: [tls_inspector]` block for both reference AND subject (previously only reference). Stale "envoy-go reads SNI natively via crypto/tls GetConfigForClient" comment removed; replaced with an ADR-0079 reference. The Task-11 PROGRESS entry explicitly invokes the ADR-0017 small-mechanical-fixes carve-out + PLAN line 2272 ("pre-existing fixtures must be re-runnable from THIS commit").
- **Verification**: the Task 11 outputs include the full TestDifferential run (22.00s; all 9 fixtures PASS); the verification commit re-runs at HEAD with all 10 fixtures (0008 included) PASS in 25.17s.

This is the right discipline — the boundary effect was anticipated, surfaced clearly, and resolved at the next task with full traceability. A future reader auditing the phase can grep "boundary effect" or "PLAN line 2272" and find the full chain.

### 2.12 The two new optional driver interfaces

The Task 13 + Task 14 harness extensions (`MultiListenerDriver` + `AlternateConfigDriver`) are clean and additive:

- Both interfaces are PURELY ADDITIVE — the existing `Driver` interface is unchanged, so pre-existing fixtures (0000-0007b) pass `ok=false` at the runner's type-assertion and fall through to the standard path. The Task 13 `TestOptionalInterfaces_NotImplemented` test pins this contract.
- Both interfaces follow the existing optional-interface idiom (mirror `DistributionAsserter`, `StatsAsserter`, `HTTPExpectations`, `BackendKindAware`, `ReferenceLogMounter`, `AccessLogAsserter`, `ReferenceLessFixture`, `SubjectAsserter` per the fixture.go shape).
- The MultiListenerDriver doc-comment correctly notes that drivers MUST still implement single-addr `Driver` methods so the runner's pre-multi-branch path works (admin-probe, fixture-discovery). This is enforced by the fixture-0008 driver's stub Drive methods.
- Task 14's runner-side branches are surgical — `mld, isMulti := d.(fixture.MultiListenerDriver)` is hoisted to the top of the reference-spawn block so the same flag drives both the exposed-ports list AND the Drive dispatch. The alternate-config branch is appended as a step-12 block AFTER all primary-pair assertions. Pre-existing fixtures are unaffected (`isMulti=false`, `acd-ok=false`).

### 2.13 Doctrine adherence

Each of the four load-bearing doctrines per `BOOTSTRAP_PROMPT.md` §3 is honored uniformly across the phase:

- **D-3.2 (no cgo / pure Go)**: the ClientHello parser at `internal/listener/listenerfilter/tls_inspector/parser.go` (139 LoC) is hand-rolled per the doctrine, adapting `crypto/tls/handshake_messages.go:unmarshal` for the ClientHello case and narrowing to two extension types (`0x0000` server_name, `0x0010` ALPN). The `parseClientHello` function uses `encoding/binary.BigEndian.Uint16` for length-prefix decoding; defensive bounds-checking on every length-bounded read; no upstream Envoy C++ binding. The boundary grep confirms zero third-party listener-filter imports. The `tls_inspector` package's external dependencies are limited to `bufio`, `context`, `encoding/binary`, `errors`, `net`, `sync`, `sync/atomic`, `time` from the Go stdlib plus `google.golang.org/protobuf/types/known/anypb` and `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3` for the TlsInspector proto type only — exactly the surface SPEC §16 acceptance bullet enumerates.

- **D-3.3 (differential correctness)**: fixture-0008 demonstrates differential per-connection chain-selection equivalence across envoy-go and reference Envoy v1.37.2. The 5-connection workload covers four chain-match decisions (destination_port wins, source_prefix_ranges wins when alone, empty-match catches the catch-all, no-match falls to default) plus the SPEC §11.3 precedence demonstration (connection 5: both chains satisfy their dimensions; destination_port wins). PASS-first-try at Task 16 is non-vacuous evidence.

- **D-3.5 (ADRs for ambiguities)**: 7 ADRs cover the seven ambiguity-bearing decision points (scope, dispatch protocol, default chain semantics, 8-dim algorithm, timeout envelope, ADR-0050 disposition, ADR-0033 supersession enumeration). Each ADR has full Context/Decision/Consequences sections; each names its `Lands-in-task`; explicit empirical-pin citations on ADR-0080 + ADR-0081.

- **D-3.6 (green build)**: every task's PROGRESS entry confirms `go vet` + `golangci-lint run` + `go test -race` clean. The verification commit re-runs all six gates fresh against HEAD. Zero races detected. Zero lint warnings. The `//nolint:revive` annotations cite ADR-0079 specifically (for the reserved type names `ListenerFilterStatus`, `ListenerFilterFactory`, `ListenerFilterRegistry`) — the right discipline for stuttering-name suppression.

### 2.14 Single-goroutine-per-connection contract

The concurrency model is exemplary:

- **Boot path**: `ListenerFilterRegistry.Register` + `Lookup` are atomic.Bool-guarded + sync.RWMutex-protected; `Freeze()` is idempotent. Mirrors 06.1's `*stats.Registry` LBP-1 + 07.1's `*HTTPRegistry` ADR-0072 — same discipline.
- **Per-connection path**: the accept goroutine (`acceptLoop`, lines 783-802) launches one goroutine per accepted conn (`go rt.serveConnection(ctx, raw)`); `serveConnection` owns the entire lifecycle through dispatch + return. The `peekerConn` is constructed and consumed within that one goroutine; no concurrent Read/Peek is possible. The `Pipeline.Run` driver (pipeline.go lines 32-59) iterates filters sequentially within the same goroutine.
- **Chain-list immutability**: `[]*ChainSpec` and `*ChainSpec.defaultChain` constructed once at NewManager-build; never mutated. Concurrent `SelectChain` calls are read-only; safe by construction.
- **Race-clean tests**: `go test -race -count=1 ./...` clean per the verification gate (e); `TestRegistryConcurrentLookup` (100 goroutines under -race) and `TestInspectConcurrentIndependentConnections` (10 goroutines) cover the registry-lookup and tls_inspector-inspection race surfaces.

The SPEC §5.6 concurrency-model table is realized faithfully.

---

### 2.15 Code-level structural quality

A few structural observations that exemplify the phase's quality:

**Constructor signature widening propagates cleanly.** `NewManagerWithBaseDirAndAllowH2C` (manager.go:207) gains a trailing `lfRegistry *listenerfilter.ListenerFilterRegistry` parameter. The two delegating constructors `NewManager` (line 166) and `NewManagerWithBaseDir` (line 178) thread `nil` for backwards compatibility — appropriate because they're the legacy entry points used by tests that don't need listener filters. `cmd/envoy-go/main.go` constructs the registry, registers `tls_inspector`, calls `Freeze()`, and threads the frozen registry into the widened constructor. The `lfRegistry == nil` guard at manager.go:410-412 surfaces a clear error message when a bootstrap declares `listener_filters[]` but no registry was threaded — defensive and well-tested by `TestParseListenerFiltersResolvesViaRegistry` + the I-3 review-driven test added at Task 9 follow-up.

**Helper functions are appropriately scoped.** The four IP/port helpers (`localIP`, `localPort`, `remoteIP`, `remotePort` at manager.go:898-924) are unexported and idiomatic — a TCP-only type-assertion that returns the zero value (nil IP, port 0) on non-TCP addresses. The chain-match algorithm correctly treats those as unmatched on every IP/port dimension. This is the right discipline for a listener path that currently only sees TCP but may grow to accept other address kinds in the future.

**The `errUnwrapFilterChain` helper** (manager.go:495-505) is a small but thoughtful piece of error-message polish: when the `default_filter_chain` caller wraps a `buildTerminalFilter` error with `default_filter_chain: %w`, the inner prefix would otherwise double. The helper peels the inner prefix; the surfaced error remains single-prefixed. The Task 9 follow-up added a unit test pinning this contract (per the I-2 finding).

**The `parseListenerFiltersTimeout` envelope check** (manager.go:510-520) is precise: nil/zero defaults to 15 000ms; values outside [1 000, 60 000]ms error with the exact message ADR-0082 specifies. The conversion `total / time.Millisecond` returns a `time.Duration` (not a uint32 directly), and the comparison is against the int constants — this is correct Go time arithmetic. The two new tests `TestParseListenerFiltersTimeoutBelowFloorErrors` + `TestParseListenerFiltersTimeoutAboveCapErrors` cover the envelope corners.

**The `findIdenticalChainSpecs` + `chainSpecKey` pair** is well-factored. The duplicate detection is two concerns: (1) canonicalization of a ChainSpec into a comparable key (`chainSpecKey`); (2) pairwise comparison of keys (`findIdenticalChainSpecs`). Splitting into two helpers makes each independently testable. The Task 9 follow-up's I-1 fix to `chainSpecKey` (slice canonicalization) is then a localized one-function change rather than a refactor of the comparison logic.

**The `serveConnection` lifecycle is self-contained.** All 7 steps of the SPEC §5.2 lifecycle live in one ~60-line function (manager.go:830-891). No control flow leaks across goroutine boundaries; no shared mutable state with the accept-loop except the read-only `*listenerRuntime`. The deferred `downstreamCxActive.Dec()` at the top mirrors the Inc in `acceptLoop` — symmetric and easy to reason about.

### 2.16 Documentation quality

The package-level doc comments are uniformly strong:

- **`internal/listener/listenerfilter/doc.go`** (36 LoC) enumerates the public surface (ListenerFilter, Status, ChainMatchInputs, Peeker, Registry, Pipeline, chainmatch.SelectChain) and the contract relationships between them.
- **`internal/listener/listenerfilter/types.go`** has per-field doc comments on `ChainMatchInputs` explaining each field's source (connection-level fact vs listener-filter contributed). The `IsLoopbackSource` helper is doc-commented with the IPv4 127.0.0.0/8 + IPv6 ::1 semantics.
- **`internal/listener/listenerfilter/chainmatch.go`** has full SPEC §5.5 + §7.3 cross-references on `breakTie`'s cascade order; the priority constants (`prioDestinationPort` = 0 ... `prioSourcePorts` = 7) carry the SPEC §5.5 line citation.
- **`internal/listener/listenerfilter/pipeline.go`** doc-comments `Pipeline.Run` with all six behavior cases enumerated.
- **`internal/listener/listenerfilter/tls_inspector/tls_inspector.go`** has the deadlock-fix rationale embedded in `Inspect`'s doc comment (lines 41-53) — future readers don't need to dig through git log to understand why the peek is incremental.
- **`internal/listener/manager.go`** doc-comments cite ADRs liberally: ADR-0078 for the supersession boundary; ADR-0079 for the registry threading; ADR-0080 for the default-chain semantics; ADR-0081 for the 8-dim algorithm; ADR-0082 for the timeout envelope. A future reader can grep ADR-XXX in manager.go and find the relevant code path.

This is the kind of doc-comment density that pays off in onboarding and in 6-month-later debugging.

## 3. Issues

### 3.1 Critical issues

**None.**

The phase has no critical issues blocking phase-done. All six gates are GREEN; the algorithm is correct; the dispatch refactor is sound; the empirical pins are paste-verbatim; the ADR set is complete and well-formed; the boundary grep returns zero.

### 3.2 Important issues

**None.**

No design or correctness issues at the Important tier. The two mid-flight self-corrections (Task 5 breakTie priority order; Task 9 chainSpecKey canonicalization) caught what would have been Important issues; the remaining surface is at the Minor tier or below.

### 3.3 Minor issues (carry-forward, non-blocking)

#### M-1 — `breakTie` skips slots 3-5 + 7 (no sub-ordering) but the cascade comment could enumerate the omissions

`breakTie` at `internal/listener/listenerfilter/chainmatch.go:201-238` correctly visits only PrefixRanges (slot 1), ServerNames (slot 2), and SourcePrefixRanges (slot 6) — the three dimensions with meaningful sub-ordering. The doc comment at lines 188-200 names this ("Only dimensions with a meaningful finer-grain sub-ordering are listed: PrefixRanges (slot 1, longest CIDR), ServerNames (slot 2, SNI rank), SourcePrefixRanges (slot 6, longest CIDR). Dimensions that are exact-value match (DestinationPort, TransportProtocol, ApplicationProtocols, SourceType, SourcePorts) have no sub-ordering and are skipped").

This is correct, but a future reader unfamiliar with the priority slots may wonder "what about ApplicationProtocols (slot 4)? Couldn't the most-specific ALPN match win?" The Envoy upstream `filter_chain_match.proto` doesn't define such a sub-ordering, so the omission is correct. Recommendation: in a future hardening pass, the doc comment could enumerate slots 0/3/4/5/7 with explicit "no sub-ordering per Envoy upstream" annotations to head off the question. Non-blocking.

#### M-2 — `tls_inspector.Inspect` swallows context.Canceled gracefully; the convention is clear from the code but could be doc-commented

At `tls_inspector.go:57-63`, when `peeker.Peek(5)` returns an error AND `len(hdr) == 0`, the code checks `errors.Is(err, context.Canceled)` and returns `(Continue, ctx.Err())` — but otherwise sets `inputs.TransportProtocol = "raw_buffer"` and returns `(Continue, nil)`. The same shape repeats at lines 78-83 for the second peek.

This is correct (a closed-mid-handshake conn correctly degrades to raw_buffer; a context-canceled call propagates the cancel). But the doc comment on `Inspect` (lines 41-53) doesn't explicitly call out the context.Canceled propagation. A future reader of the function may wonder why the Peek error path branches on context.Canceled specifically. Recommendation: in a future hardening pass, add a one-line note to the doc comment explaining the context.Canceled propagation. Non-blocking.

#### M-3 — `Pipeline.Run` returns `nil` even after `continue_on_listener_filters_timeout=true` semantics

The `continue_on_listener_filters_timeout` semantics are NOT directly handled in `Pipeline.Run` — the pipeline always returns `(timeout error)` on context-deadline-exceeded. Then in `serveConnection` at manager.go:852-859, the listener manager checks `!rt.continueOnLfTimeout` and decides whether to abort or proceed. This is correct (the SPEC §6.5 puts the policy in the listener manager, not the pipeline) and the test `listener_filters_timeout_abort` covers it.

But it means the Pipeline's contract is "return error on timeout; let the caller decide whether to honor or ignore" — which is fine but worth a doc-comment note. The doc comment at pipeline.go:16-31 enumerates the behaviors but doesn't mention the continue_on_lf_timeout caller policy. Recommendation: add a one-line forward-pointer "the caller (listener manager) honors `Listener.continue_on_listener_filters_timeout` per ADR-0082 — see internal/listener/manager.go:serveConnection". Non-blocking.

#### M-4 — `chainSpecKey` is O(N²) duplicate-detection (manager.go:630-643)

`findIdenticalChainSpecs` at manager.go:630-643 does a naive O(N²) pairwise comparison. For the realistic deployment profile (N < 100 chains) this is fine; for pathological N (1000s of chains) it would dominate boot time. The `keys[]` slice is already canonicalized by `chainSpecKey`, so a `map[string]int` keyed by canonicalized chainSpecKey would yield O(N) duplicate detection.

Recommendation: in a future hardening pass, swap to a map-based duplicate scan. The current implementation is correct + tested; this is a performance polish, not a correctness issue. Non-blocking.

#### M-5 — `Pipeline` struct is empty + Run is a method — pure-function refactor would simplify

`Pipeline` at pipeline.go:14 is an empty struct (`type Pipeline struct{}`); the only method `Run` is a pointer-receiver method on it. The struct exists "for future per-pipeline state (e.g., metrics counters) without breaking the API" per the doc comment. This is fine — the empty-struct-now-non-empty-later pattern is idiomatic Go.

But a future reader may wonder why the method isn't a package-level `func Run(...)`. The reservation is reasonable; if the parent session prefers, a package-level `Run` plus a comment "future per-pipeline state will introduce a Pipeline struct" would be slightly cleaner. Non-blocking — reasonable people disagree on this convention.

#### M-6 — Some test files mix table-driven and non-table-driven styles

`integration_test.go` uses a table-driven shape (`subtest` struct with `configure` + `slowLF` fields) for 5 subtests; `chainmatch_test.go` uses 14 separate `TestSelectChainXxx` functions. Both styles are valid; the consistency could be tightened in a future pass. Non-blocking.

#### M-7 — `Pipeline.Run` defer-OnDestroy ordering

`Pipeline.Run` at pipeline.go:33-37 defers `OnDestroy` on every constructed filter, called in declaration order at function exit (regardless of how the loop exited). LIFO defer order means the LAST filter is destroyed FIRST, which is correct symmetric to the construction order from the caller's perspective — but a future filter that depends on cross-filter resource ordering (e.g., an `original_dst` that opens a socket option that a later filter consumes) would need to know this. The current MVP only registers `tls_inspector` so this is moot.

Recommendation: in a future hardening pass when a second concrete listener filter ships, the doc-comment on `Pipeline.Run` could enumerate the LIFO-destroy order explicitly. Non-blocking.

#### M-8 — Test naming convention mild drift

The test naming convention is mostly consistent (`TestSelectChainXxx` for chainmatch tests; `TestPipelineRunXxx` for pipeline tests; `TestRegistryXxx` for registry tests; `TestInspectXxx` for tls_inspector tests). One mild drift: `TestNewRoundtripsThroughRegistry` (tls_inspector_test.go) does not follow the package-prefixed pattern that the other tls_inspector tests use. Could be `TestTLSInspectorNewRoundtripsThroughRegistry` for consistency. Non-blocking.

#### M-9 — `chainSpecKey` could elide unspecified fields for shorter keys

`chainSpecKey` at manager.go:657-711 always emits all 9 dimension prefixes (`dp=...|pr=...|sn=...|...`) even when a dimension is unspecified, producing keys like `dp=0|pr=|sn=|tp=|ap=|stl=false|ste=false|spr=|sp=` for an empty-ish chain. This is functionally correct (the `Empty: true` short-circuit at line 658-660 already handles the canonical empty case) but slightly verbose. A future hardening pass could elide unspecified-dimension prefixes. Non-blocking — the keys are not user-visible.

#### M-10 — Fixture-0008 `BackendCount = 5` with 5th port unused

The PROGRESS Task 16 entry notes "5 backends total: 4 chain-specific + 1 placeholder-for-symmetry; only ports[0..3] are wired into clusters, ports[4] is allocated but unused — driver_test.go's TestReferenceBootstrapRenders pins this invariant." This is a 1-port over-allocation that maintains the SPEC §7.4's "5 backends total" symmetry but consumes one extra OS port per fixture run.

Recommendation: in a future fixture-cleanup pass, drop the placeholder port. The symmetry argument is documentary; the actual chain count is 4 (3 specific + 1 default). Non-blocking.

---

## 4. Process observations

### 4.1 What worked well

- **Subagent-driven execution per the user's preference**: per the PROGRESS, each task was a discrete subagent invocation with a single-commit lifecycle; the parent session's role was orchestration + review. This kept each task's scope tight and grep-traceable.
- **TDD discipline observed at every task**: each task's PROGRESS entry confirms the test was written first, the build error was observed (e.g., `undefined: ChainSpec`, `undefined: Pipeline`), then the implementation landed and tests passed. The fail-then-pass cycle is documented verbatim in each task's "Outputs" block. This is exemplary discipline for a doctrine D-3.6 ("green build") project.
- **Two review-driven mid-flight self-corrections caught real issues** before phase-done: the Task 5 breakTie priority-order fix (a SPEC §5.5 violation) and the Task 9 chainSpecKey slice-canonicalization fix (a runtime ErrAmbiguousChainMatch latency bomb). Both fixes landed as follow-up commits with new tests; both are correct. This is the kind of in-flight iterative refinement that a less rigorous review process would miss.
- **The Task 10/11 boundary effect was anticipated, documented, and resolved cleanly**: the Task 10 PROGRESS entry's "Differential note" block named the regression in advance; the Task 11 commit resolved it; the verification commit re-ran all 10 fixtures green. This is the right shape for a multi-task refactor that touches a load-bearing path.
- **Empirical pin discipline is uniform**: three pins (§11.1/§11.2/§11.3) at SPEC time + one pin (§11.4) at impl time per Decision K — all four against the same Envoy v1.37.2 server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`, all four paste-verbatim into BEHAVIOR_CONTRACT.md. The carry-forward resolution at Task 16 (per Decision K) is well-traced.
- **ADR discipline is exemplary**: 7 ADRs landed at the right tasks; ADR-0078 consolidates the supersession enumeration so a future reader can grep from either side; non-monotonic commit-order is permitted and recorded; full Context/Decision/Consequences sections; explicit `Lands-in-task` fields. ADR-0083 is the explicit-non-supersession ADR (ADR-0050 stays in force) — the right discipline for a phase that touches a related surface.
- **The differential fixture passed first try at Task 16** — non-vacuous evidence that the algorithm + dispatch refactor produced an Envoy-equivalent chain-selection across 5 connections.
- **The verification commit's six-gate sweep is FRESH (not cached)** per the verification entry: gate (a) re-ran TestDifferential against HEAD; gate (c) re-ran h2spec at the ADR-0051 pin; gate (d) re-ran all 10 fuzzers at -fuzztime=30s each. This is the right discipline for lifecycle-state 4 → 5 per ADR-0049.

### 4.2 Process improvements (for future phases)

- **Task 15's "ad-hoc test harness scoped to this commit" pattern** for fixture YAML round-trip validation is a one-off — the test ran but wasn't committed. A future phase could either (a) commit a generic `TestBootstrapLoadAllFixtures` auto-discovery helper to the bootstrap test set, or (b) document the ad-hoc convention in a new ADR. The current state is fine; it's just slightly opaque to a future reader who tries to re-run the verification.
- **The driver-embedded const templates vs static envoy*.yaml dichotomy** is well-explained in the fixture-0008 driver doc-comment but is non-obvious to a first-time reader. A future hardening pass could either (a) add a top-level fixture-pattern doc to docs/envoy-go/ explaining the dichotomy, or (b) make the static YAMLs strictly illustrative (with a clear "informational only — driver embeds the load-bearing definitions" header). The current state is fine; it's a polish.
- **The Task 10 + Task 11 multi-commit boundary** was handled well but relied on the implementer's discipline to document the transient regression. A more mechanical approach would be a per-task pre-flight `go test ./test/differential/...` hook that surfaces transient regressions before the commit lands; this would prevent the boundary effect from being silently introduced. Non-blocking.

---

## 5. Recommendations for follow-up

### 5.1 Suggested follow-up (no urgency)

1. **`breakTie` doc-comment enrichment** — enumerate slots 0/3/4/5/7 with explicit "no sub-ordering per Envoy upstream" annotations, per M-1.
2. **`tls_inspector.Inspect` doc-comment** — add a note on context.Canceled propagation, per M-2.
3. **`Pipeline.Run` doc-comment** — add a forward-pointer to the listener manager's `continue_on_listener_filters_timeout` policy, per M-3.
4. **`findIdenticalChainSpecs` map-based scan** — swap O(N²) to O(N) for pathological N, per M-4.
5. **Generic `TestBootstrapLoadAllFixtures`** — add a fixture-auto-discovery test to `internal/bootstrap/` so future fixture additions don't need ad-hoc scoped-to-commit harnesses (per the Task 15 PROGRESS note).

None of the above is required for phase-done. They're documented here so a future hardening phase has a starting list.

### 5.2 Items the parent session may carry forward

The phase has no architectural debt. The seven ADRs are complete; the supersession enumeration is precise; the dispatch path is unified; the empirical pins are durable; the differential fixture is non-vacuous. The parent session may proceed to the phase-done commit.

The follow-up items above are SUGGESTIONS for a future hardening phase and need not be addressed before phase-done.

### 5.3 Future-phase pointers (already documented in SPEC §2)

The deferred items the SPEC §2 already records — `original_dst`, `proxy_protocol`, `http_inspector` listener filters; `direct_source_prefix_ranges` chain-match dimension; xDS LDS dynamic listener configuration; HTTP/3 + QUIC listener filters; per-listener-filter metrics; CEL-driven `filter_disabled` — are all out of scope for 07.2 and well-documented. No action needed.

---

## 5A. ADR-by-ADR review

Each of the seven 07.2 ADRs is reviewed below for content correctness, supersession discipline, and `Lands-in-task` accuracy.

### ADR-0077 — Phase-07.2 scope decision

Lands in PROGRESS Task 1 (commit `66300ba`). Anchors the ROADMAP row-flip already landed at the SPEC commit. Scope decision is precise (the three deliverables: listener_filters[] framework with tls_inspector; full 8-dim FilterChainMatch; default_filter_chain honored). Explicit deferrals enumerate all six items per SPEC §2.1-§2.6. Mirrors ADR-0070's 07.1 scope-confirmation pattern. **Verdict: well-formed.**

### ADR-0078 — ADR-0033 partial supersession enumeration

Lands at Task 9 (commit `ee45f35`). The clause-by-clause table consolidates supersession enumeration that ADR-0079/0080/0081 would otherwise leave scattered. Each of the 9 ADR-0033 clauses receives a precise disposition (3 fully preserved, 3 preserved with caveats, 3 superseded). The "Net effect" summary is grep-friendly. The Consequences enumerate concrete code changes (the deletion of `chainSpecificityRank` from manager.go; the rewrite of `validateFilterChainMatch` → `parseChainSpec`; the listenerRuntime field additions). **Verdict: exemplary; this is the kind of consolidating ADR that pays off in 6-month-later code archaeology.**

### ADR-0079 — Listener-filter dispatch protocol shape

Lands at Task 2 (commit `814ba67`). The most architecturally load-bearing ADR of the phase. Specifies: (a) sync-only Inspect (no async-resume); (b) 2-state Status enum (Continue/StopIteration); (c) freeze-after-boot Registry mirroring 07.1's ADR-0072 + 06.1's LBP-1; (d) two-step factory pattern (config-time + per-connection); (e) 4096-byte default peeker buffer clamped [256, 65536]; (f) per-connection sequential dispatch supporting multi-filter pipelines. The Rationale section justifies sync-only at MVP (listener filters have a much narrower surface than HTTP filters). **Verdict: well-formed; the consequence enumeration matches the production-code surface that landed.**

### ADR-0080 — `default_filter_chain` semantics

Lands at Task 5 (commit `b36f251`). Three load-bearing decisions: (a) default_filter_chain consulted ONLY when no filter_chains[] entry is eligible; (b) empty-match chain in filter_chains[] BEATS default_filter_chain when both coexist; (c) TLS posture independent. Empirical-pin citation: SPEC §11.1 + §11.2 against Envoy v1.37.2 server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`. Supersedes ADR-0033 clause 3 totally. The empirical evidence is paste-verbatim into BEHAVIOR_CONTRACT.md. **Verdict: well-formed; the empirical pin is the durable evidence base.**

### ADR-0081 — `FilterChainMatch` 8-dimension precedence algorithm

Lands at Task 5 (same commit as ADR-0080). The heart of phase 07.2. Specifies: (a) priority order [destination_port, prefix_ranges, server_names, transport_protocol, application_protocols, source_type, source_prefix_ranges, source_ports]; (b) 2-pass eligibility-then-specificity algorithm; (c) tie-breaking finer-grain rules (longest CIDR; SNI specificity); (d) final ties at NewManager-build time error. Empirical-pin citation: SPEC §11.3 against the same Envoy v1.37.2 server SHA. Supersedes ADR-0033 clause 2 partially. The Alternatives Considered section explicitly rejects (A) per-priority-level eligibility and (B) string-based pattern matching — both reasonable rejections with the right rationale (algorithmic complexity / out-of-scope respectively). **Verdict: well-formed; the priority ordering matches the upstream `filter_chain_match.proto` documentation, and the empirical pin confirms it on a real Envoy v1.37.2 dispatch path.**

### ADR-0082 — `listener_filters_timeout` envelope

Lands at Task 4 (commit `19bb90d`). Specifies: (a) honored in [1s, 60s] envelope; (b) default 15s if unset; (c) values outside envelope error at parse with the exact message `listener: %q: listener_filters_timeout %s is outside the supported [1s, 60s] envelope`; (d) `continue_on_listener_filters_timeout` honored as proto-documented; (e) per-pipeline shared budget per Decision N (NOT per-filter time-slicing). The Rationale enumerates why [1s, 60s] is the right envelope (values < 1s risk false-positive timeouts under CI scheduler jitter; values > 60s indicate misconfiguration). **Verdict: well-formed; the envelope choice has clear empirical justification.**

### ADR-0083 — ADR-0050 disposition (no supersession)

Lands at Task 1 (commit `66300ba`, same commit as ADR-0077). Settles the question "does 07.2's `application_protocols` chain-match dimension supersede ADR-0050's HCM-internal ALPN dispatch?". The answer is NO: ADR-0050 is the codec-selection mechanism (which Go-level codec runs); 07.2's `application_protocols` is the chain-selection mechanism (which `filter_chain` entry runs at all). The two are orthogonal and complementary. The decision is empirically verified by the SPEC §11.4 carry-forward (resolved at Task 16 per Decision K). **Verdict: well-formed; explicit non-supersession ADRs are valuable when a future reader might wonder.**

### Cross-ADR coherence

The seven ADRs reference each other coherently:

- ADR-0078 (supersession enumeration) cross-references ADR-0079, ADR-0080, ADR-0081 as the per-clause supersession ADRs.
- ADR-0080 (default-chain) cross-references ADR-0081 (algorithm) for the empty-match-vs-default eligibility rule.
- ADR-0081 (algorithm) cross-references ADR-0080 for the no-eligible-chain fallback path.
- ADR-0082 (timeout) cross-references ADR-0079 (dispatch protocol) for the per-pipeline budget shape.
- ADR-0083 (ADR-0050 disposition) cross-references SPEC §2.5 + Decision H for the non-supersession rationale.
- ADR-0078 explicitly notes that "Future xDS / listener phases that revisit any ADR-0033 clause can cite ADR-0078 directly" — the right discipline for downstream phases.

The ADR set forms a coherent reference network; a future reader can navigate from any one to the related set via the explicit cross-references.

---

## 5B. Test-coverage analysis (per SPEC §15)

A side-by-side enumeration of SPEC §15's testing-strategy items vs the test files that landed:

### SPEC §15.1 — Unit tests (`internal/listener/listenerfilter/`)

| SPEC item | File | Status |
|---|---|---|
| `registry_test.go`: Register/Lookup/duplicate-name panic/Freeze/post-Freeze panic; concurrent Lookup race-clean | `registry_test.go` (75 LoC, 5 tests) | ✓ |
| `pipeline_test.go`: Continue/StopIteration; per-pipeline timeout; 2-filter pipeline; ctx cancel mid-pipeline | `pipeline_test.go` (172 LoC, 7 tests) | ✓ |
| `chainmatch_test.go`: 8-dim correctness; multi-dim chains; empty-match-beats-default; default consulted on no-match; CIDR + SNI tie-breakers; identical → ErrAmbiguousChainMatch; no-chain+no-default → ErrNoChainMatched | `chainmatch_test.go` (212 LoC, 14 tests — 10 original + 4 review-driven) | ✓ |
| `callbacks_test.go`: Peek-then-Read invariant; Peek-beyond-buffer ErrBufferFull; size clamp | `callbacks_test.go` (74 LoC, 3 tests) | ✓ |

### SPEC §15.2 — Unit tests (`internal/listener/listenerfilter/tls_inspector/`)

| SPEC item | File | Status |
|---|---|---|
| `parser_test.go`: full ClientHello with both SNI+ALPN; SNI-only; ALPN-only; no extensions; truncated; malformed length prefix; ALPN with multi/single protocols; SNI with multi-name | `parser_test.go` (108 LoC, 7 tests) | ✓ |
| `tls_inspector_test.go`: real ClientHello populates inputs; non-TLS preamble sets raw_buffer; concurrent inspection race-clean; type_url + factory round-trip through registry | `tls_inspector_test.go` (194 LoC, 6 tests) | ✓ |

### SPEC §15.3 — Unit tests (`internal/listener/`)

| SPEC item | File | Status |
|---|---|---|
| `manager_test.go` extended: `validateFilterChainMatch` accepts new dimensions; rejects `direct_source_prefix_ranges` (silently ignored); chain-precedence sorting via 8-dim algorithm | `manager_test.go` (extended) | ✓ — 13+ new test functions per Task 9 PROGRESS |
| `listener_test.go` extended: per-listener filter-set tests | `listener_test.go` (extended) | ✓ |
| `integration_test.go` (NEW): end-to-end accept-loop test with TLS+SNI dispatch, plaintext destination_port chain, default fallback | `integration_test.go` (294 LoC, 5 subtests) | ✓ |

### SPEC §15.4 — Differential fixture `0008-listener-chain-match`

5-connection workload with per-connection backend-port-equivalence assertion; PASS-first-try at Task 16. ✓

### SPEC §15.5 — h2spec re-run (gate (c))

53/53 PASS at the ADR-0051 SHA pin per the verification output. UNCHANGED relative to phase 07.1. ✓

### SPEC §15.6 — Fuzzers (gate (d))

| Fuzzer | Status | Notes |
|---|---|---|
| `FuzzBootstrapLoad` | ✓ | 31s, 639 992 execs, 6 new interesting (per verification) |
| `FuzzTcpProxyFilter` | ✓ | 31s, 3 974 603 execs |
| `FuzzTLSContextParse` | ✓ | 31s, 4 491 833 execs |
| `FuzzHCMConfigParse` | ✓ | 31s, 3 524 079 execs |
| `FuzzFrameStream` | ✓ | 30s, 13 632 200 execs |
| `FuzzHPACKDecode` | ✓ | 31s, 1 946 601 execs |
| `FuzzPromTextFormat` | ✓ | 30s, 25 640 681 execs |
| `FuzzAccessLogFormat` | ✓ | 31s, 25 136 854 execs |
| `FuzzFilterChainParse` | ✓ | 31s, 4 687 424 execs |
| `FuzzFilterChainMatch` (NEW, 10th) | ✓ | 30s, 16 447 352 execs, 5 new interesting |

All 10 fuzzers clean for 30 s each; zero crashers; zero persisted corpus failures.

The new `FuzzFilterChainMatch` covers all four SPEC §15.6 assertions: (i) never panics; (ii) returned chain is one of input chains, default, or nil; (iii) returned chain's dimensions all satisfied by inputs; (iv) deterministic on identical inputs.

### SPEC §15.7 — Race detector + lint (gate (e))

`go vet ./... && golangci-lint run ./... && go test -race ./...` clean across all 30 packages per the verification commit. Zero data races. ✓

### Coverage summary

Every SPEC §15 testing-strategy item has a corresponding test landing. The fixture-0008 differential is non-vacuous. The fuzzer coverage matches the SPEC's enumeration. The integration_test.go end-to-end test exercises the full §5.2 dispatch path. Test coverage is comprehensive.

---

## 5C. SPEC-to-implementation cross-reference table

Acceptance bullets from SPEC §16 mapped to landed artifacts:

| SPEC §16 acceptance bullet | Landed at | Verified by |
|---|---|---|
| All six gates green; gate (a) non-vacuous | All tasks | Verification commit `10b0aba` |
| `internal/listener/listenerfilter/` package exists with full API | Tasks 2-5 | Files present; tests pass |
| `internal/listener/listenerfilter/tls_inspector/` package | Tasks 7-8 | Files present; tests pass |
| `internal/listener/manager.go` rewritten | Tasks 9-10 | Per PROGRESS; tests pass |
| Constructor signature widening | Task 9 | manager.go:207 |
| `cmd/envoy-go/main.go` boot wiring | Task 11 | main.go:118-122 |
| `ListenerFilterRegistry` freeze-after-boot enforced | Task 3 | `TestRegistryFreezeBlocksRegister` |
| All three §11 in-session pins verbatim in SPEC | Tasks 1+ | SPEC §11.1-§11.3 |
| §11.4 carry-forward documented + routed to PLAN | Task 1 (route) + Task 16 (resolve) | SPEC §11.4 + PROGRESS Task 16 |
| BEHAVIOR_CONTRACT `## Listener filters` at phase-done | Task 17 | BEHAVIOR_CONTRACT.md:733 |
| BEHAVIOR_CONTRACT Equivalence Matrix new row | Task 17 | BEHAVIOR_CONTRACT.md:9-28 (per Task 17 PROGRESS) |
| BEHAVIOR_CONTRACT TCP proxy "Does not yet apply to" updated | Task 17 | BEHAVIOR_CONTRACT.md:331+ |
| BEHAVIOR_CONTRACT TLS "Scope boundaries" updated | Task 17 | BEHAVIOR_CONTRACT.md:372+ |
| All seven ADRs in DECISIONS.md with full sections | Tasks 1-9 | DECISIONS.md:2804-3162 |
| Fixture `0008-listener-chain-match/` committed in full | Tasks 15-16 | test/fixtures/0008-... |
| h2spec UNCHANGED at ADR-0051 pin; 53/53 PASS | All tasks | Verification commit |
| No fixture regression | All tasks | Verification commit (10/10 PASS) |
| STATE.md at lifecycle-state 6 (post-REVIEW); ROADMAP rows done | Phase-done commit | (pending phase-done commit) |
| PROGRESS.md SHA-fill convention | Tasks 1-17 + verification | PROGRESS.md (commit `46e416c` SHA-fill) |
| `FuzzFilterChainMatch` committed; total 10 fuzzers | Task 6 | fuzz_test.go:11 |
| No third-party listener-filter / chain-match library imported | All tasks | Boundary grep zero |

Every acceptance bullet is satisfied at HEAD `10b0aba` except the two phase-done-commit-time items (STATE.md → 6 and ROADMAP rows → done) which land at the next commit.

---

## 5D. Comparison to prior sub-phase patterns

Phase 07.2 is the eighth sub-phase to follow the BOOTSTRAP_PROMPT-defined lifecycle pattern; the prior sub-phases (05.1, 05.2, 06.1, 06.2, 07.1) establish a mature precedent. 07.2's adherence to the precedent is uniform:

- **PROGRESS-style**: append-only log; per-task SHA-fill; verbatim command outputs; mirrors 06.1, 06.2, 07.1. ✓
- **ADR-numbering shift discipline**: planner re-verifies next-free at PLAN write time per ADR-0004; topical-vs-commit-order non-monotonicity recorded in `Lands-in-task` per 06.2 + 07.1 precedent. ✓
- **BEHAVIOR_CONTRACT in-place edit at phase-done**: per ADR-0052; mirrors 06.1, 06.2, 07.1 timing. ✓
- **Empirical-pin block in SPEC + paste-verbatim in BEHAVIOR_CONTRACT**: mirrors 06.1's Rule SN4, 06.2's verbatim access-log, 07.1's four-pin block. ✓
- **Boundary grep at REVIEW time**: mirrors 06.2 + 07.1 acceptance discipline. ✓
- **Six-gate verification commit before REVIEW**: per BOOTSTRAP §5 step 4; mirrors 07.1 verification commit pattern. ✓

The only minor deviation: the closing-sweep + BEHAVIOR_CONTRACT in-place edit landed in a single commit (`c4bcd02`) rather than two separate commits. This is consistent with 07.1's pattern (which also bundled the closing sweep + in-place edit) but a future reader unfamiliar with the precedent might wonder. The Task 17 PROGRESS entry documents the bundling explicitly, so this is non-blocking.

---

## 5E. Risk assessment for downstream phases

Phase 08 (admin-api-and-drain), the next planned phase per ROADMAP, will inherit the listener-filter framework from 07.2. The risk surface 07.2 introduces for 08:

- **Drain semantics**: phase 08's drain logic must respect the `Pipeline.Run` per-pipeline timeout — a connection that has entered the listener-filter pipeline but not yet returned cannot be force-closed mid-Pipeline without truncating the inputs population. The current listener-filter pipeline design honors `ctx.Done()` per ADR-0082 + the `context.WithTimeout` plumbing, so phase 08's drain ctx-cancel will propagate naturally. **Low risk.**
- **Admin API metrics**: phase 08 may want listener-filter pipeline metrics (e.g., `listener_filter.tls_inspector.invocations`, `.failures`, `.timeouts`). SPEC §2.6 explicitly defers these per ADR-0041 silent-ignore set; phase 08 may revisit. Adding metrics is purely additive (3 LoC per call site). **Low risk.**
- **Hot-reload via xDS LDS**: not in scope for 08 either; deferred to xDS family. The chain-list immutability invariant per ADR-0081 means hot-reload would require a copy-on-write or version-stamped chain set — out of scope for phase 08. **No risk; deferred.**
- **`original_dst` filter**: deferred per ADR-0077 + Decision F. Phase 08 does not need it for drain semantics. **No risk; deferred.**

Phase 08's surface area is admin/drain, not listener; 07.2 introduces no architectural debt that phase 08 would need to clean up. The listener-filter framework + chain-match algorithm are stable.

---

## 5F. Detailed algorithm walkthrough — `SelectChain`

For a future reviewer who needs to understand the chain-match algorithm at code-level, this walkthrough traces SelectChain against the SPEC §11.3 empirical-pin shape (the load-bearing precedence test).

**Setup**: 2 chains:
- chain `dstport`: `DestinationPort: 10000`
- chain `srcprefix`: `SourcePrefixRanges: [127.0.0.1/32]`

**Inputs** (connection from 127.0.0.1 to port 10000):
- `DestinationIP: 127.0.0.1`, `DestinationPort: 10000`
- `SourceIP: 127.0.0.1`, `SourcePort: <ephemeral>`

**Pass 1 (eligibility)** — chainmatch.go:82-87:
- chain `dstport`: `matches()` checks `c.DestinationPort != 0 && c.DestinationPort != inputs.DestinationPort` — 10000 == 10000 → satisfied. No other dimensions specified. **eligible.**
- chain `srcprefix`: `matches()` checks `len(c.SourcePrefixRanges) > 0 && !ipInAny(inputs.SourceIP, c.SourcePrefixRanges)` — `127.0.0.1` is in `127.0.0.1/32` → satisfied. **eligible.**

Both eligible. Proceed to Pass 2.

**Pass 2 (specificity)** — chainmatch.go:94-110:
- chain `dstport`: `specificityScore` sets bit `prioCount-1-prioDestinationPort = 8-1-0 = 7` → `1 << 7 = 128 = 0x80`.
- chain `srcprefix`: `specificityScore` sets bit `prioCount-1-prioSourcePrefixRanges = 8-1-6 = 1` → `1 << 1 = 2 = 0x02`.

`128 > 2` → chain `dstport` wins. **No tie-breaker needed.**

This matches the SPEC §11.3 empirical pin: `tcp.tcp_dstport.downstream_cx_total: 1`, `tcp.tcp_srcprefix.downstream_cx_total: 0`.

**Tie-breaker example** — `TestSelectChainBreakTieFollowsPriorityOrder`:

Setup: 2 chains both with `PrefixRanges + ServerNames + SourcePrefixRanges` specified:
- chain `a`: PrefixRanges=`10.0.0.0/8`, ServerNames=`["*"]`, SourcePrefixRanges=`192.168.1.0/24`
- chain `b`: PrefixRanges=`10.0.0.0/8`, ServerNames=`["foo.example"]`, SourcePrefixRanges=`192.168.0.0/16`

Inputs: DestinationIP=10.0.0.5, ServerName="foo.example", SourceIP=192.168.1.5.

**Pass 1**: both eligible (every specified dimension satisfied).

**Pass 2**: both have specificity `bit_1 | bit_2 | bit_6 = 1<<6 | 1<<5 | 1<<1 = 64+32+2 = 98 = 0x62`. **Tied.**

**`breakTie` cascade**:
- Slot 1 (PrefixRanges): `longestPrefix(10.0.0.5, [10.0.0.0/8])` = 8 for both. **Tied.**
- Slot 2 (ServerNames): `sniSpecificityRank(["*"]) = 2` (universal-wildcard); `sniSpecificityRank(["foo.example"]) = 0` (exact). `0 < 2` → chain `b` wins. **Decided.**
- Slot 6 (SourcePrefixRanges): NOT consulted; cascade returned at slot 2.

This matches the SPEC §5.5 line 519 "walk down the priority list" rule. **Critical**: if the original PLAN-verbatim cascade order (PrefixRanges → SourcePrefixRanges → ServerNames) had remained, slot 6 would have decided FIRST: chain `a`'s SourcePrefixRanges=192.168.1.0/24 (longest=24) > chain `b`'s SourcePrefixRanges=192.168.0.0/16 (longest=16) → chain `a` would have won. The Task 5 follow-up fix `12969ed` reorders the cascade to honor SPEC priority, picking chain `b`. **The fix is correct.**

**Ambiguous example** — `TestSelectChainAmbiguousReturnsError`:

Setup: 2 chains both with `TransportProtocol: "tls"` only.

Inputs: TransportProtocol="tls".

**Pass 1**: both eligible.

**Pass 2**: both have specificity `bit_3 = 1<<4 = 16 = 0x10`. **Tied.**

**`breakTie` cascade**: TransportProtocol is at slot 3 (no sub-ordering — exact-value match only). PrefixRanges, ServerNames, SourcePrefixRanges all unspecified on both chains; cascade falls through every slot. Returns nil. `SelectChain` returns `(nil, ErrAmbiguousChainMatch)`.

This is the right runtime behavior; the boot-time `findIdenticalChainSpecs` would catch this case at NewManager-build (both chains have identical chainSpecKey="EMPTY-non-empty-with-tp=tls"-flavored key after canonicalization). The `ErrAmbiguousChainMatch` runtime path is the safety net for the boot-time check.

**Empty-match-vs-default example** — `TestSelectChainEmptyMatchBeatsDefault`:

Setup: 1 chain `empty` (Empty=true) + 1 default chain `def`.

Inputs: arbitrary.

**Pass 1**: `matches(empty, ...)` returns true via the `c.Empty` short-circuit. eligible=[empty]. **len(eligible) > 0** → defaultChain branch NOT taken.

**Pass 2**: empty's specificity is 0 (Empty short-circuit). best=empty. Loop over eligible[1:] is empty. Return empty.

This matches the SPEC §11.2 empirical pin: `tcp.tcp_empty.downstream_cx_total: 1`, `tcp.tcp_default.downstream_cx_total: 0`. **Correct.**

The algorithm walkthrough confirms that the production code at chainmatch.go:80-238 realizes the SPEC §5.5 + §7.3 algorithm faithfully across all the corners the test set exercises.

---

## 5G. Verification evidence catalog

The verification commit `10b0aba` (re-run independently of the Task 17 closing sweep per `superpowers:verification-before-completion` discipline) captures fresh evidence for all six gates. A reviewer can grep PROGRESS.md `## Verification` for the verbatim outputs:

- **Gate (a)** fixture 0008: `--- PASS: TestDifferential/0008-listener-chain-match (2.37s)` — 5 connections; per-connection backend-port routing byte-equal across envoy-go and Envoy v1.37.2.
- **Gate (b)** pre-existing fixtures: 9/9 PASS in 25.17s aggregate; zero regressions.
- **Gate (c)** h2spec: `53 tests, 53 passed, 0 skipped, 0 failed` at the ADR-0051 pin — UNCHANGED.
- **Gate (d)** all 10 fuzzers PASS at -fuzztime=30s each; verbatim execs/sec + new-interesting counts in the verification entry.
- **Gate (e)** `go build ./...` clean (empty output); `go vet ./...` clean; `golangci-lint run ./...` clean; `go test -race -count=1 ./...` PASS across all 30 packages.
- **Gate (f)** is THIS REVIEW.

All evidence is fresh against HEAD `46e416cfe9d41e63109fc3bca6997c43d02feee6` (the SHA-fill commit; the verification commit's HEAD pointer at the time the gates were re-run). The verification session does NOT modify production code.

This is the right discipline for lifecycle-state 4 → 5 per ADR-0049 + BOOTSTRAP §5 step 4 — the verification subagent is independent of the implementation subagent + the closing-sweep subagent.

---

## 6. Summary

Phase 07.2 is a substantial sub-phase that lands the second half of the parent ROADMAP row 07 (filter-chain framework) — and closes the parent row at the same phase-done commit per the parent SPEC §5 closure pattern. The implementation realizes every SPEC requirement faithfully:

- New `internal/listener/listenerfilter/` package tree (~1 100 LoC production + tests) implementing the listener-filter framework + 8-dimension chain-match algorithm + the 10th repo-wide fuzzer.
- New `internal/listener/listenerfilter/tls_inspector/` sub-package (~600 LoC) implementing the first concrete listener filter with a hand-rolled minimal ClientHello parser per D-3.2.
- Substantial refactor of `internal/listener/manager.go` (~+200 LoC net) replacing the SNI-only post-handshake dispatch with the unified pre/post-handshake path; widening the constructor signature; honoring `default_filter_chain` and `listener_filters[]` per ADR-0078's supersession of ADR-0033 clauses 2/3/8.
- Boot wiring in `cmd/envoy-go/main.go` + a blank-import in `internal/bootstrap/bootstrap.go`.
- New differential fixture `0008-listener-chain-match` with the dual-listener + connection-4-only c4-variant construction, a 5-connection workload, and PASS-first-try evidence.
- Two new optional driver interfaces in the harness (`MultiListenerDriver`, `AlternateConfigDriver`) — purely additive.
- BEHAVIOR_CONTRACT.md `## Listener filters` section + amendments to `## TCP proxy` / `## TLS` / `## HTTP filter chain` per ADR-0052.
- 7 new ADRs (ADR-0077..ADR-0083) with full Context/Decision/Consequences sections, supersession enumeration, and `Lands-in-task` fields.

Two review-driven mid-flight self-corrections (Task 5 `breakTie` priority-order fix; Task 9 `chainSpecKey` slice-canonicalization fix) caught issues that would have been Important-tier in this review; both are correct and well-tested. The Task 10/11 boundary effect (transient fixture-0002 regression) was anticipated, documented, and resolved cleanly. The Task 16 SPEC §11.4 carry-forward resolution (per Decision K) lands the verbatim Envoy v1.37.2 ALPN-feeds-application_protocols evidence in BEHAVIOR_CONTRACT.md.

All six gates are GREEN at HEAD. The boundary grep for third-party listener-filter dependencies returns zero. The phase is APPROVED for phase-done.

---

## 7. Final assessment (re-stated for clarity)

**APPROVED.**

No critical or important issues block the phase-done commit. The minor issues above (M-1..M-6) are documentation/test-coverage sharpenings appropriate for a future hardening pass and need not be addressed before phase-done.

The parent session may proceed with the phase-done commit per BOOTSTRAP §5.3. The commit message is per SPEC §3:

```
phase 07.2: phase-done — listener-chain-completion lands; ROADMAP rows 07.2 + 07 → done [ADR-0077, ADR-0078, ADR-0079, ADR-0080, ADR-0081, ADR-0082, ADR-0083]
```

The body should explicitly name both ROADMAP-row transitions per parent SPEC §5 closure pattern (the parent row `07` flips `in-progress → done` AT THE SAME COMMIT as `07.2 → done`).

After the phase-done commit lands, the project advances to phase 08 (admin-api-and-drain) at lifecycle-state 0 per BOOTSTRAP §5.

---

**Reviewer:** code-reviewer subagent invoked by the parent session per BOOTSTRAP §5 step 6.
**Date of review:** 2026-05-02.
**Worktree HEAD at review time:** `10b0aba` (verification commit; all six gates green).
**Branch:** `phase/07.2-listener-chain-completion-impl`.
