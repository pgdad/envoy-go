# Phase 07.2 — Listener-chain completion (`internal/listener/listenerfilter/` package, `tls_inspector` filter, `FilterChainMatch` beyond SNI, `Listener.default_filter_chain`, fixture `0008`) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4 and per the user's persistent preference for subagent-driven execution recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants — particularly §4.1's ROADMAP-row-flips-at-SPEC-commit and at-phase-done discipline), §5 (state machine), §5.3 (commit-message-completeness — every ADR introduced or referenced is named in the phase-done commit message), §6.1 (split gate — ~1500 LoC AND <25 tasks), §7 (differential contract), §7.5 (phase-done six-gate checklist that SPEC §3 specialises for 07.2); `docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md` (the authoritative source — every PLAN task traces to one or more SPEC sections; ~1101 lines, 17 sections, read in full); `docs/envoy-go/phases/07-filter-chain-framework/SPEC.md` (parent master SPEC — cross-cutting context for the 07.1 + 07.2 split; §5 commits the parent-row-closure-at-07.2-phase-done discipline); `docs/envoy-go/phases/07-filter-chain-framework/BRAINSTORM.md` (the brainstorm-close artefact at master `da28039` that the 07.2 SPEC distils from); `docs/envoy-go/phases/07.1-http-filter-framework/{SPEC.md,PLAN.md,PROGRESS.md,REVIEW.md}` (closed read-only history; the 07.1 PLAN is the structural precedent — §-numbering, heredoc-style task headers, ADR-with-first-use-commit discipline, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections, TDD-step granularity, multi-listener fixture extension reasoning); `docs/envoy-go/phases/06.2-access-log/{SPEC.md,PLAN.md,PROGRESS.md,REVIEW.md}` (the structural template the 07.1 PLAN itself mirrors); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0076 — especially **ADR-0001** template, **ADR-0003** branch convention, **ADR-0004** autonomous-numbering rule, **ADR-0005** autonomous plan-review adaptation + subagent-driven preference, **ADR-0008** Envoy v1.37.2 pin, **ADR-0010** V4_ONLY DNS rule, **ADR-0017** small-mechanical-fixes do not require ADRs, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0028** reference-side `--concurrency 1` pin, **ADR-0033** phase-03 filter-chain subset (partially superseded by 07.2 — see SPEC §5.7), **ADR-0041** silent-ignore set + amendment policy (extended by 07.2 per SPEC §12), **ADR-0045** planner-time-split discipline, **ADR-0050** ALPN-driven codec selection inside `Filter.Handle` (NOT superseded by 07.2 — see SPEC §2.5 + ADR-0083), **ADR-0051** h2spec pin SHA, **ADR-0052** BEHAVIOR_CONTRACT in-place edit authorisation, **ADR-0059** internal stats Registry architecture (the architectural-shape sibling 07.2's ADR-0079 mirrors), **ADR-0061** Rule SN4 empirical-pin pattern (the SPEC §11 mirror), **ADR-0066** access-log architecture + empirical-pin pattern, **ADR-0070** parent phase-07 split ADR, **ADR-0072** `*HTTPRegistry` threaded constructor map (the architectural-shape sibling 07.2's `*ListenerFilterRegistry` mirrors), **ADR-0076** is the verified DECISIONS.md tail at master `424485b`; phase 07.2's seven anticipated ADRs land at ADR-0077..ADR-0083); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the in-place-edit target — adds a NEW `## Listener filters` top-level section + amends `## TCP proxy "Does not yet apply to"` and `## TLS "Scope boundaries"` for the 07.2 promotions; lands at the phase-done commit per ADR-0052); `docs/envoy-go/ENVOY_TARGET.md` (the v1.37.2 image pin SPEC §11 empirical pins cite); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 07.2 — D-3.7 reserves pin bumps for dedicated phases).

**Goal:** Land envoy-go's listener-chain-completion sub-phase — a real listener-filter dispatch pipeline that runs BEFORE HCM (and any other network filter) on every accepted downstream connection, the full 8-dimension `FilterChainMatch` algorithm with `most-specific-wins` priority ordering, and the `Listener.default_filter_chain` no-match-fallback semantics. One new differential fixture lands: `test/fixtures/0008-listener-chain-match/` is differentially green (gate (a) **non-vacuous**; per-connection backend-port routing byte-equal across envoy-go and reference Envoy v1.37.2 over a 5-connection workload spanning two listeners + four chains + one default chain). Concretely: a NEW `internal/listener/listenerfilter/` package (`ListenerFilter` + `ListenerFilterStatus` enum + `ChainMatchInputs` + `Peeker` + `peekerConn` in `types.go` + `callbacks.go`; `ListenerFilterRegistry` Register/Lookup/Freeze in `registry.go`; per-connection `Pipeline` with sequential dispatch + per-pipeline timeout in `pipeline.go`; 8-dimension `chainmatch.SelectChain` algorithm in `chainmatch.go`; per-package `doc.go`; ~600-800 LoC of new machinery per SPEC §1 #1 + §4.1); ONE new concrete listener filter under `internal/listener/listenerfilter/tls_inspector/` (~250 LoC + ~120 LoC parser + ~50 LoC proto + ~600 LoC tests; peeks the first ~512 bytes off the connection, parses the TLS ClientHello, extracts SNI + ALPN, populates `ChainMatchInputs.ServerName` + `.ApplicationProtocols` + `.TransportProtocol`); substantial refactor of `internal/listener/manager.go` (~+200 LoC net — `validateFilterChainMatch` accepts the seven new dimensions; `default_filter_chain` is parsed; `listener_filters[]` is parsed via the registry; the post-handshake `dispatch` shape is replaced by the unified pre/post-handshake dispatch path per SPEC §5.2; the SNI-only `chainSpecificityRank` is preserved as the tie-breaker WITHIN the `server_names` priority slot of the new 8-dimension algorithm); a constructor signature widening that threads `*listenerfilter.ListenerFilterRegistry` through `listener.NewManager*`; boot wiring in `cmd/envoy-go/main.go` (alloc + register `tls_inspector` + Freeze + thread); a single blank-import in `internal/bootstrap/bootstrap.go` for the upstream `tls_inspector v3` proto so `protojson` round-trips fixture 0008's bootstraps; a NEW `internal/listener/listenerfilter.FuzzFilterChainMatch` fuzzer at the 30s ADR-0018 budget — fuzzes adversarial `ChainMatchInputs` corners + adversarial chain-spec lists into `chainmatch.SelectChain` (10th fuzzer overall); two new optional Driver interfaces in `test/differential/fixture/fixture.go` (`MultiListenerDriver` for fixtures targeting >1 listeners + `AlternateConfigDriver` for fixtures with secondary bootstrap variants — fixture-0008 is the first consumer of both per SPEC §7.4 + §9.2); runner-side branches in `test/differential/runner_test.go` for the two new optional interfaces; `test/fixtures/0008-listener-chain-match/` directory carrying `envoy.yaml` + `envoy-go.yaml` + `envoy-c4.yaml` + `envoy-go-c4.yaml` + `expectations.yaml` + `README.md` + `driver/driver.go` + `driver/driver_test.go` + `backends/main.go` (small Go program returning the listener address as the response body); seven new ADRs ADR-0077..ADR-0083 (re-verified at Task 1 step 1 against `DECISIONS.md` tail being ADR-0076); the §11.4 carry-forward empirical pin (`tls_inspector`-populated ALPN feeds `application_protocols` chain-match) is resolved at Task 16 — the executor scrapes verbatim Envoy v1.37.2 stats output and pastes it both into SPEC §11.4 (replacing the carry-forward placeholder) and into `BEHAVIOR_CONTRACT.md ## Listener filters` per Decision K; a NEW `## Listener filters` top-level section is added to `BEHAVIOR_CONTRACT.md` between `## HTTP filter chain` and `## xDS wire state machine` carrying the four §11 empirical-pin blocks verbatim per ADR-0052 + SPEC §13.1; `## TCP proxy "Does not yet apply to"` and `## TLS "Scope boundaries"` are amended for the 07.2 promotions per SPEC §13.3; STATE.md / ROADMAP.md / PROGRESS.md updates with row 07.2 → `done` AND parent row 07 → `done` AT THE SAME phase-done commit (per parent SPEC §5 + the 05/05.1/05.2 + 06/06.1/06.2 closure pattern). After phase 07.2, the project has proven the second half of its eighth central engineering claim: *envoy-go runs a real listener-filter dispatch pipeline before HCM (or any other network filter) on each accepted connection — supporting filters that peek the ClientHello to extract SNI + ALPN — and matches a downstream filter chain against any FilterChainMatch dimension Envoy v1.37.2 documents (port, IP CIDR, source-IP CIDR, source port, source-type, SNI, transport protocol, ALPN), with `default_filter_chain` as the documented no-match fallback — making the proxy a programmable middlebox aligned with Envoy's documented chain-match extensibility model.* The parent ROADMAP row `07` flips to `done` at THIS phase's phase-done commit per the parent SPEC §5 closure pattern.

**Architecture:** The 07.2 surface is the additive introduction of one new package tree (`internal/listener/listenerfilter/` with one sub-package: `tls_inspector/`) plus substantial refactor of `internal/listener/manager.go` (constructor signatures widen + the dispatch path gains a listener-filter pipeline + the chain-match algorithm replaces the SNI-only specificity sort + `default_filter_chain` is honored + `listener_filters[]` is dispatched) plus the threading of a `*listenerfilter.ListenerFilterRegistry` parameter through one constructor chain (`listener.NewManager*`) plus boot-wiring in `cmd/envoy-go/main.go` plus a single blank-import addition in `internal/bootstrap/bootstrap.go` for the `tls_inspector v3` proto plus harness extensions in `test/differential/fixture/fixture.go` (two new optional Driver interfaces) and `test/differential/runner_test.go` (branches for those interfaces). The threading mirrors 07.1's `*filter_http.HTTPRegistry` parameter-threading discipline (codified in 07.1 ADR-0072) which itself mirrors 06.1's `*stats.Registry` pattern (ADR-0059); SPEC §4.2's file inventory enumerates each constructor change explicitly. Concretely: `internal/listener/listenerfilter/types.go` (NEW; ~120 LoC) defines the `ListenerFilter` interface (single method `Inspect(ctx, peeker, inputs) (Status, error)` + `OnDestroy()`), `ListenerFilterStatus` enum (`Continue`, `StopIteration`), `ChainMatchInputs` struct (eight fields: `DestinationIP net.IP`, `DestinationPort uint32`, `SourceIP net.IP`, `SourcePort uint32`, `ServerName string`, `TransportProtocol string`, `ApplicationProtocols []string`, plus the `IsLoopbackSource() bool` helper for `source_type: LOCAL`), `Peeker` interface (`Peek(n int) ([]byte, error)`), `ListenerFilterFactory` + `FilterInstanceFactory` two-step factory pattern, `FactoryCtx` carrying registry pointer + parsed proto-helpers; `internal/listener/listenerfilter/callbacks.go` (NEW; ~80 LoC) defines `peekerConn` concrete (`net.Conn` wrapper backed by a `bufio.Reader`; `Peek(n)` returns first n bytes without consuming; `Read(b)` drains from buffer first, then transitions to underlying conn — same trick `crypto/tls.Conn.Handshake()` uses internally for resumption-cookie peek-back); `internal/listener/listenerfilter/registry.go` (NEW; ~80 LoC) defines `ListenerFilterRegistry struct { mu sync.RWMutex; byTypeURL map[string]ListenerFilterFactory; frozen atomic.Bool }`, `NewListenerFilterRegistry()`, `Register(typeURL string, f ListenerFilterFactory)` (panics if frozen, panics on duplicate type_url), `Lookup(typeURL string) (ListenerFilterFactory, bool)`, `Freeze()` (idempotent); mirrors 07.1's `*filter_http.HTTPRegistry` LBP-1 (per ADR-0072) and 06.1's `*stats.Registry` (per ADR-0059); `internal/listener/listenerfilter/pipeline.go` (NEW; ~120 LoC) defines `Pipeline` per-connection sequential dispatch — allocated by the listener manager's accept-loop on each accepted connection; method `Run(ctx, filters []ListenerFilter, peeker Peeker, inputs *ChainMatchInputs, timeoutMs uint32) error` iterates `filters[]` sequentially calling each filter's `Inspect`; on `Continue` advances; on `StopIteration` halts; **a single per-pipeline `context.WithTimeout(ctx, timeoutMs * time.Millisecond)` is established once before the loop and shared across all filters** (per Decision N — NOT per-filter time-slicing); on context-deadline-exceeded returns timeout error which the listener manager handles per the proto's `continue_on_listener_filters_timeout` field; `internal/listener/listenerfilter/chainmatch.go` (NEW; ~250 LoC) defines the 8-dimension chain-match precedence algorithm — `ChainSpec` struct holding parsed match dimensions for one chain; `SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error)` runs the 2-pass eligibility-then-specificity algorithm per SPEC §5.5 (Pass 1: eligibility filter — every non-zero dimension on `c.spec` must match `inputs`; empty-match chains universally eligible. Pass 2: specificity scoring by priority-ordered 8-bit vector — bit i set iff dimension `priorityOrder[i]` is specified; chain with highest specificity integer wins; ties broken at finer grain — longer CIDR prefix on `prefix_ranges`/`source_prefix_ranges`, SNI-specificity on `server_names` per ADR-0033 clause 9 preserved as special case); `priorityOrder = [destination_port, prefix_ranges, server_names, transport_protocol, application_protocols, source_type, source_prefix_ranges, source_ports]`; `internal/listener/listenerfilter/doc.go` (NEW; ~50 LoC) — package overview pointing to the framework architecture, registry contract, per-connection `Pipeline` lifecycle, freeze-after-boot invariant, peek-buffer discipline; `internal/listener/listenerfilter/fuzz_test.go` (NEW; ~80 LoC) — `FuzzFilterChainMatch` fuzzes adversarial `ChainMatchInputs` corners + adversarial chain-spec lists into `chainmatch.SelectChain`; asserts (i) never panics; (ii) returned chain is one of the input chains OR `defaultChain` OR nil; (iii) returned chain's match dimensions all satisfied by `inputs`; (iv) deterministic on identical inputs; `internal/listener/listenerfilter/tls_inspector/tls_inspector.go` (NEW; ~250 LoC) — concrete `ListenerFilter` implementation `envoy.filters.listener.tls_inspector` (`type_url = "type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector"`); `Inspect`: peek up to N bytes (N = `initial_read_buffer_size` or 4096 default) from connection; check whether byte preamble is a TLS ClientHello (TLS record header `0x16 0x03 ...` followed by HandshakeType.ClientHello byte 1); if YES, parse via `parser.parseClientHello` to extract SNI + ALPN, populate `inputs.ServerName` + `inputs.ApplicationProtocols` + `inputs.TransportProtocol = "tls"`, return `Continue`; if NO (non-TLS preamble — plaintext), set `inputs.TransportProtocol = "raw_buffer"`, return `Continue`; `internal/listener/listenerfilter/tls_inspector/parser.go` (NEW; ~120 LoC) — hand-rolled minimal ClientHello parser `parseClientHello(buf []byte) (sni string, alpns []string, ok bool)`; pure function, no I/O; adapted from `crypto/tls/handshake_messages.go:unmarshal` for the ClientHello message type, narrowed to extract only `server_name` (extension type 0) + `application_layer_protocol_negotiation` (extension type 16); does NOT pull in the upstream Envoy C++ `tls_inspector` implementation (D-3.2 forbids cgo / C++ binding); `internal/listener/listenerfilter/tls_inspector/proto.go` (NEW; ~50 LoC) — proto-config marshaling for the upstream `envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector` proto; honors `initial_read_buffer_size` (clamped to [256, 65536]; defaulted to 4096 if unset); silently ignores `enable_ja3_fingerprinting` per SPEC §12; `internal/listener/listenerfilter/tls_inspector/doc.go` (NEW; ~30 LoC); `internal/listener/manager.go` (MODIFIED) — `validateFilterChainMatch` rewritten to ACCEPT `destination_port` / `prefix_ranges` / `source_prefix_ranges` / `source_type != ANY` / `source_ports` / `application_protocols` (with SPEC §12 carve-out: `direct_source_prefix_ranges` is silently ignored — does NOT error); the function is renamed to `parseChainSpec` returning a `*chainmatch.ChainSpec` instead of just validating; `default_filter_chain` is now parsed (the ADR-0033 clause 3 error at line 251 is removed; a chain-with-empty-match is constructed for `default_filter_chain` and stored on `listenerRuntime.defaultChain`); `listener_filters[]` is now parsed (each entry resolved via `*ListenerFilterRegistry.Lookup`; instances constructed via per-connection factories; stored on `listenerRuntime.listenerFilters[]` slice); the `chainSpecificityRank` function at line 352 is preserved as the SNI-internal tie-breaker reused by `chainmatch.SelectChain`'s `breakTie` for the `server_names` priority slot; the `sort.SliceStable` chain-sort at line 327 is REMOVED (chains stored in declaration order; `SelectChain` scores at dispatch time); the `dispatch` function at line 434 is replaced by the unified pre/post-handshake dispatch per SPEC §5.2 — accept conn → allocate `ChainMatchInputs` from `LocalAddr()`+`RemoteAddr()` → wrap in `peekerConn` → run `Pipeline.Run` → call `chainmatch.SelectChain` → if selected has TLS run handshake → dispatch to `selected.filter.Handle`; the `makeGetConfigForClient` callback at line 413 is simplified — it returns the pre-selected chain's TLS config (no chain-match logic); the `serveTLS` function at line 550 is also collapsed into the unified dispatch path; `internal/listener/manager.go` constructor signature — `NewManagerWithBaseDirAndAllowH2C` gains a new `lfRegistry *listenerfilter.ListenerFilterRegistry` parameter; existing constructors `NewManager` + `NewManagerWithBaseDir` delegate; tests update mechanically to thread an empty-or-`tls_inspector`-only frozen registry; new field `listenerRuntime.listenerFilters []ListenerFilter`, `listenerRuntime.defaultChain *chainInfo`, `listenerRuntime.lfTimeoutMs uint32`, `listenerRuntime.continueOnLfTimeout bool`; `cmd/envoy-go/main.go` (MODIFIED) — at boot: `lfReg := listenerfilter.NewListenerFilterRegistry(); lfReg.Register(tls_inspector.TypeURL, tls_inspector.New); lfReg.Freeze();` threaded into the listener-manager constructor chain; `internal/bootstrap/bootstrap.go` (MODIFIED) — adds blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"` (carries the `TlsInspector` proto message) so `protojson` can round-trip 07.2 fixture bootstraps; per ADR-0016's amendment policy this addition is documented in PROGRESS, not a new ADR; `test/differential/fixture/fixture.go` (MODIFIED) — adds two new optional Driver interfaces: `MultiListenerDriver` (`SubjectListenerNames() []string` + `ReferenceListenerPorts() []int` + `DriveReferenceMulti(ctx, addrs map[string]string) ([]byte, error)` + `DriveSubjectMulti(ctx, addrs map[string]string) ([]byte, error)`) and `AlternateConfigDriver` (`AlternateReferenceBootstrap(backendPorts []int) string` + `AlternateSubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string` + `AlternateSubjectListenerName() string` + `AlternateReferenceListenerPort() int` + `DriveAlternate(ctx, refAddr, subjAddr string) ([]byte, error)`); the existing `Driver` interface methods stay unchanged (a multi-listener driver returns the FIRST listener as the primary in `SubjectListenerName()` so the runner type-assertion path falls through cleanly when neither optional interface is implemented); `test/differential/runner_test.go` (MODIFIED) — runner branches: after the standard ref+subj startup + `DriveReference` + `DriveSubject` + `CompareBytes`, type-assert on `MultiListenerDriver` (if implemented, the standard `DriveReference` + `DriveSubject` are SKIPPED in favor of `DriveReferenceMulti` + `DriveSubjectMulti`; the runner allocates additional subject ports for the additional listeners and exposes the additional reference ports); after the diff, type-assert on `AlternateConfigDriver` (if implemented, spawn a SECOND ref+subj pair using `AlternateReferenceBootstrap` + `AlternateSubjectConfig`, run `DriveAlternate`, diff its bytes against ref-side); `test/fixtures/0008-listener-chain-match/` (NEW directory) carries `envoy-go.yaml` (subject — primary; dual listeners `l_test_a` + `l_test_b` each binding `127.0.0.1:0` plaintext; both carry the SAME 3 `filter_chains[]` entries + `default_filter_chain`; STATIC clusters; 5 backend pools — one per chain, one for default chain, shared across both listeners) + `envoy.yaml` (reference — primary; same dual-listener / 3-chain + default shape; STRICT_DNS `host.docker.internal:<backend-port>` per ADR-0010; `--concurrency 1` per ADR-0028) + `envoy-go-c4.yaml` (subject — c4 variant; identical to primary but `chain_other` REMOVED so connection 4 falls through to `chain_default`) + `envoy.yaml` (reference — c4 variant; same shape) + `expectations.yaml` (prose description of the 5-connection workload + per-connection expected backend port + chain-precedence demonstration narrative) + `README.md` (fixture purpose, dual-listener + c4-variant rationale, chain-match-precedence demonstration, cross-reference to BEHAVIOR_CONTRACT `## Listener filters` introduced at 07.2 phase-done) + `driver/driver.go` (`BackendCount() = 5`; `SubjectListenerName() = "l_test_a"` (primary, for `Driver` compat); `SubjectListenerNames() = ["l_test_a", "l_test_b"]` (for `MultiListenerDriver`); `ReferenceListenerPort() = 15008` (primary); `ReferenceListenerPorts() = [15008, 15009]`; `DriveSubjectMulti(ctx, addrs)` issues 4 sequential TCP connections (1, 2, 3, 5 per SPEC §7.4) routed across both listeners + `AlternateConfigDriver` interface drives connection 4 via the c4-variant ref+subj pair) + `driver/driver_test.go` (distribution-/expectation-assertion unit tests) + `backends/main.go` (small Go program — `net.Listen` on configurable port, accepts a TCP connection, returns the listener address `127.0.0.1:NNNN` as the response body, closes the connection); `test/differential/runner_test.go` (MODIFIED) blank-imports the new fixture-0008 driver package; `BEHAVIOR_CONTRACT.md` is edited in place at the closing-sweep commit per ADR-0052 — adds NEW `## Listener filters` top-level section between `## HTTP filter chain` (line 514) and `## xDS wire state machine` (line 250) populated with the four §11 empirical-pin blocks verbatim + listener-filter dispatch protocol rules + chain-match algorithm rules + `default_filter_chain` semantics + new `## Equivalence Matrix` row addition; amends `## TCP proxy "Does not yet apply to"` (line 360) to remove the "Filter chain matching (`filter_chain_match` non-empty) — phase 07" entry and rewrite "Multiple filters in a chain — phase 07" to clarify in-scope-vs-out-of-scope; amends `## TLS "Scope boundaries"` (line 405) to remove "ALPN-driven filter-chain selection", "non-SNI filter-chain match fields", "`Listener.default_filter_chain`", "`listener_filters` (still silently skipped)" — all now in scope; the seven ADRs ADR-0077..ADR-0083 land at first-use-task ordering per the phase-04/05.1/05.2/06.1/06.2/07.1 precedent.

**Tech Stack:**
- Go 1.23 (unchanged from 07.1; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `bufio`, `context`, `crypto/tls` (read-only — for ClientHello parser reference + TLS Server Conn type), `errors`, `fmt`, `net`, `sort`, `sync`, `sync/atomic`, `time` — the exhaustive set the `internal/listener/listenerfilter/` package consumes.
- `google.golang.org/protobuf` (proto runtime) — the `*anypb.Any` type the two-step factory parses + the `*listenerv3.FilterChainMatch` messages parsed from bootstraps; transitive from existing imports.
- `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3` — proto types only (`TlsInspector`); blank-imported in `internal/bootstrap/bootstrap.go` so `protojson` can round-trip 07.2 fixture bootstraps containing `listener_filters[]` entries with `typed_config` of this type. **No new go-control-plane runtime helpers** (D-3.2 forbids).
- **NEW: no third-party listener-filter / chain-match library.** `go.mod` MUST NOT contain any listener-filter-engine library import. The acceptance check at Task 17 step 8 grep-verifies the absence (per ADR-0079 + SPEC §16 acceptance bullet "No third-party listener-filter or chain-match library is imported").
- `internal/cluster` (existing) — UNCHANGED in 07.2.
- `internal/stats` (06.1's deliverable) — UNCHANGED in 07.2; framework allocates no new stats counters per SPEC §2.6.
- `internal/accesslog` (06.2's deliverable) — UNCHANGED in 07.2.
- `internal/filter/http` (07.1's deliverable) — UNCHANGED in 07.2; the HCM HTTP-filter framework continues to dispatch downstream of the listener-filter pipeline + chain-match algorithm. ADR-0083 (anticipated) records the no-supersession of ADR-0050 (HCM-internal ALPN dispatch coexists with 07.2's listener-side `application_protocols` chain-match).
- `internal/filter/hcm`, `internal/filter/tcpproxy` (existing) — UNCHANGED in 07.2 (terminal-filter dispatch path is reached AFTER chain selection completes; the dispatch is filter-agnostic).
- `internal/tls` (existing) — UNCHANGED in 07.2 (TLS handshake path is reached AFTER chain selection; `internaltls.NewDownstreamConfig` continues to parse `transport_socket` per chain).
- `github.com/envoyproxy/go-control-plane/envoy` at v1.32.4 (ADR-0013 pin, unchanged). Phase 07.2 reads `envoy.config.listener.v3.{Listener, FilterChainMatch, ListenerFilter}` + `envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector` typed-config; no proto version bump.
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0008's reference (Envoy in a Docker container) — same harness as 06.2/07.1's fixtures consume; phase 07.2 extends `test/differential/fixture/fixture.go` with two new optional interfaces but does NOT modify `test/differential/harness.go` itself (only adds branches in `runner_test.go`).
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0008's reference image AND the source of the four §11 empirical pins. Three of four already executed at SPEC time and pinned verbatim in SPEC §11.1–§11.3; the fourth (§11.4 — `tls_inspector`-populated ALPN) is **resolved at Task 16** per Decision K.
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 07.2 — D-3.7 reserves pin bumps for dedicated phases). The conformance gate (c) re-runs at the same pin and reports unchanged 53/53 PASS; phase 07.2's surface is pre-HCM (the listener-filter pipeline runs BEFORE HCM dispatches; nothing about chain-match changes how H2 frames flow on the wire after the chain is selected).
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- **Forbidden runtime imports (D-3.2 + ADR-0079):** any listener-filter-engine / chain-match library; cgo / C++ binding to upstream Envoy's `tls_inspector` implementation. Test-side use is also forbidden. The boundary grep at Task 17 step 8 enforces.
- `internal/listener/listenerfilter/` is a NEW package tree introduced in 07.2; no pre-existing imports of it exist.
- `internal/listener/` extends in place; the only new external import is `github.com/esalaine/envoy-go/internal/listener/listenerfilter` (and via it, the `tls_inspector` sub-package needed only by `cmd/envoy-go/main.go`'s registry-population). The package-import-graph stays acyclic.

---

## Scope check — why phase 07.2 ships as one sub-phase

Net change estimate (mirroring the 06.2 / 07.1 PLAN's component-table convention):

- `internal/listener/listenerfilter/types.go` ~120 + `types_test.go` ~150 = ~270
- `internal/listener/listenerfilter/callbacks.go` ~80 + `callbacks_test.go` ~150 = ~230
- `internal/listener/listenerfilter/registry.go` ~80 + `registry_test.go` ~150 = ~230
- `internal/listener/listenerfilter/pipeline.go` ~120 + `pipeline_test.go` ~250 = ~370
- `internal/listener/listenerfilter/chainmatch.go` ~250 + `chainmatch_test.go` ~400 = ~650
- `internal/listener/listenerfilter/doc.go` ~50
- `internal/listener/listenerfilter/fuzz_test.go` ~80
- `internal/listener/listenerfilter/tls_inspector/tls_inspector.go` ~250 + `tls_inspector_test.go` ~300 + `doc.go` ~30 = ~580
- `internal/listener/listenerfilter/tls_inspector/parser.go` ~120 + `parser_test.go` ~200 = ~320
- `internal/listener/listenerfilter/tls_inspector/proto.go` ~50 + `proto_test.go` ~80 = ~130
- `internal/listener/manager.go` extension (validateFilterChainMatch rewrite + default_filter_chain parse + listener_filters[] parse + dispatch refactor + chainSpec building) ~+250 / ~-50 = ~+200 net + `manager_test.go` extension ~+250 = ~450
- `internal/listener/listener_test.go` extension (per-listener filter-set tests) ~+50 = ~50
- `internal/listener/integration_test.go` (NEW) ~250
- `internal/bootstrap/bootstrap.go` extension (tls_inspector v3 blank import) ~3 + `bootstrap_test.go` extension ~10 = ~13
- `cmd/envoy-go/main.go` extension (alloc + register + Freeze + thread) ~20 + `main_test.go` extension ~40 = ~60
- `test/differential/fixture/fixture.go` extension (MultiListenerDriver + AlternateConfigDriver interfaces) ~50
- `test/differential/runner_test.go` extension (multi-listener + alternate-config branches) ~200
- `test/fixtures/0008-listener-chain-match/envoy-go.yaml` ~120 + `envoy.yaml` ~120 + `envoy-go-c4.yaml` ~100 + `envoy-c4.yaml` ~100 + `expectations.yaml` ~80 + `README.md` ~100 + `driver/driver.go` ~400 + `driver/driver_test.go` ~120 + `backends/main.go` ~60 = ~1200
- `docs/envoy-go/DECISIONS.md` (seven ADRs) ~700
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (in-place edit + amendments) ~250
- `docs/envoy-go/ROADMAP.md` (row updates) ~3
- `docs/envoy-go/STATE.md` (lifecycle transitions) ~5
- `docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md` ~250
- `docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md` (§11.4 carry-forward fill at Task 16) ~+30 = ~30

Total estimate: **~6500 LoC**. Test-LoC is ~3200 of the total; production-LoC is ~2200; doc/yaml/ADR/PROGRESS/SPEC-edit LoC is ~1100. Task count is **17** — well below the 25-task gate (`BOOTSTRAP_PROMPT.md` §6.1's primary signal). LoC estimate is at the OR-leg, comparable to 07.1 (~5500 effective) and 06.1 (~3300 actual) and 06.2 (~4000 actual) — all of which shipped as one (sub-)phase. The phase-04 / 05.1 / 05.2 / 06.1 / 06.2 / 07.1 precedent is that task-count-under-25 is the load-bearing signal; the LoC OR-leg has been exceeded in five of six prior phases without splitting, when the surface is structurally atomic.

Phase 07.2 ships as **one** sub-phase (NOT split into 07.2.1 + 07.2.2 — even though the natural surface axis exists: 07.2.1 = framework + tls_inspector + manager refactor + boot wiring; 07.2.2 = harness extensions + fixture-0008) for three reasons:

1. **The surface-axis split (07.2.1 framework + manager refactor + boot wiring; 07.2.2 fixture-0008 + harness extensions) creates vacuous gate (a) on 07.2.1.** Per BOOTSTRAP §6.3 ("do not ship incomplete stubs that conformance tests can't exercise"), a 07.2.1 carrying only the listener-filter framework + tls_inspector + manager.go refactor + boot wiring would have NO new differential fixture exercising the framework's chain-match claim — fixture 0008 is the SOLE 07.2 differential surface, and it requires the dual-listener + 5-connection workload + harness extensions to land together. The pre-existing fixtures `0000`-`0007b` would still be green on 07.2.1 (the SNI-only path is preserved as a special case), but they only exercise the SNI dimension of the 8-dimension chain-match algorithm. A 07.2.1 with the framework but no fixture exercising it is exactly the "incomplete stub" anti-pattern §6.3 targets. Splitting also leaves the `*ListenerFilterRegistry` allocated-but-only-`tls_inspector`-registered in 07.2.1's `cmd/envoy-go/main.go` until 07.2.2 — which is fine — but the chain-match algorithm's 8-dimension-priority-ordering claim is unverified differentially in 07.2.1.

2. **Task count is below the 25-task gate; LoC estimate is at the OR-leg with established phase-04 / 05.1 / 05.2 / 06.1 / 06.2 / 07.1 precedent.** Per phase-04 / 05.1 / 05.2 / 06.1 / 06.2 / 07.1 precedent, task-count-under-25 is the primary signal that one phase is the right shape. 07.2's 17 tasks fits with margin; the SPEC's estimated 1100-1500 LoC range was for the production-code surface only, and the +6 tasks for fixtures + ADRs + closing sweep are within the gate. The ~6500-LoC effective estimate is comparable to 07.1's ~5500 actual landed; the OR-leg has been exceeded in five of six prior phases without splitting, and the structural-atomicity argument (#1 above) precludes splitting on the surface axis.

3. **The framework + tls_inspector + manager refactor + boot wiring + harness extensions + fixture-0008 form one atomic load-bearing claim.** Per BOOTSTRAP §6.3 + SPEC §1, the central engineering claim of 07.2 is "envoy-go runs a real listener-filter dispatch pipeline before HCM (or any other network filter) on each accepted connection — supporting filters that peek the ClientHello to extract SNI + ALPN — and matches a downstream filter chain against any FilterChainMatch dimension Envoy v1.37.2 documents (port, IP CIDR, source-IP CIDR, source port, source-type, SNI, transport protocol, ALPN), with `default_filter_chain` as the documented no-match fallback". Removing fixture 0008 reduces 07.2 to "framework exists but is not differentially equivalence-claimed" — fails the D-3.3 differential-correctness doctrine. Removing tls_inspector leaves the `application_protocols` + `transport_protocol` chain-match dimensions un-exercisable (those dimensions are populated only by listener filters; without a concrete listener filter, they're dead code). Removing the manager refactor leaves the framework with nothing to dispatch from. Removing the harness extensions makes fixture 0008's dual-listener + c4-variant shape infeasible to drive. The six components form a coherent atomic unit.

**Triggering re-evaluation:** if at execution time the cumulative landed-LoC count exceeds **9000** by the end of Task 12 (i.e., before fixture 0008's harness + driver tasks), invoke `superpowers:systematic-debugging` on the estimate-vs-reality gap and re-evaluate. A ~38% miss on a carefully-bounded sub-phase is a signal the plan's shape is wrong, not just that the work is large. Mid-execution split valve: `BOOTSTRAP_PROMPT.md` §6.1's secondary trigger (any single task's sub-steps blow up past ~10 items) stays active. The two tasks most likely to blow past 10 sub-steps are Task 5 (`chainmatch.go` 8-dimension algorithm — the largest single-file logic surface) and Task 16 (fixture 0008 driver — orchestrates dual-listener + c4-variant + 5-connection workload + §11.4 empirical pin). If either exceeds 15 sub-steps at execution time, the executor splits per §6.2 with a new ADR — the natural axis is 07.2.1 (framework + tls_inspector + manager refactor + boot wiring; Tasks 1–12) and 07.2.2 (harness extensions + fixture-0008 + closing sweep; Tasks 13–17), with the caveat from #1 above that 07.2.1 has vacuous gate (a) and would need a placeholder fixture probe.

---

## ADRs introduced by this plan

Seven ADRs land at execution time. Each is the first-use task's responsibility and goes into the same commit as the code that consumes it. All entries in `DECISIONS.md` are append-only (D-3.5); no landed ADR is edited. ADR numbering continues from the tail verified at PLAN-write time (**ADR-0076** is the current tail, verified by `grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1` → `ADR-0076:` at the master-`424485b`-then-SPEC-commit baseline; the planner re-verified at PLAN-write time that ADR-0070..ADR-0076 all landed in the 07.1 phase-done + sha-fill commits per SPEC §10 anticipation; if a mid-PLAN-authoring ADR landed since the SPEC commit, re-number 07.2 ADRs sequentially from `tail + 1` and update every task's ADR reference *before* starting Task 2 — the executor checks at Task 1 step 1). Per SPEC §10, phase 07.2's seven ADRs land at ADR-0077..ADR-0083 in topical order. The topic-to-ADR-number map:

- **SPEC §10 ADR-0077 anticipation** (Phase-07.2 scope decision) → **ADR-0077** (lands Task 1, the PROGRESS preamble; first ADR of the seven; the scope decision is a process decision that documents the SPEC-drafting session's already-landed ROADMAP edit at master `bb5f437`, so anchoring it at T1 — the implementation session's first commit — is the natural fit since this is the first opportunity to land the ADR after the SPEC commit's ROADMAP edit). Mirrors ADR-0070's 07.1 pattern.
- **SPEC §10 ADR-0079 anticipation** (Listener-filter dispatch protocol shape — sync-only, narrow `Inspect(peeker, inputs)` surface, freeze-after-boot registry, two-step factory pattern) → **ADR-0079** (lands Task 2, the `internal/listener/listenerfilter/types.go` + `callbacks.go` introduction; first use of the dispatch-protocol shape in production code; the architectural shape applies to every subsequent task in the package).
- **SPEC §10 ADR-0082 anticipation** (`listener_filters_timeout` honored in [1s, 60s]; default 15s; per-pipeline shared budget) → **ADR-0082** (lands Task 4, the `internal/listener/listenerfilter/pipeline.go` introduction; first use of the timeout-enforcement mechanism in production code; the bootstrap parser's [1s, 60s] envelope check at Task 9 cross-references this ADR).
- **SPEC §10 ADR-0080 anticipation** (`default_filter_chain` semantics — no-match fallback; empty-match chain BEATS `default_filter_chain`; TLS posture independent; supersedes ADR-0033 clause 3) → **ADR-0080** (lands Task 5, the `chainmatch.SelectChain` default-fallback path; first use of the default-chain semantics in production code).
- **SPEC §10 ADR-0081 anticipation** (8-dimension `FilterChainMatch` precedence algorithm — priority-ordered specificity scoring; ties broken at finer grain; supersedes ADR-0033 clause 2 partial) → **ADR-0081** (lands Task 5, the `chainmatch.SelectChain` 2-pass algorithm; first use of the priority-ordering claim in production code; ADR-0080 + ADR-0081 land in the SAME commit since both anchor in `chainmatch.go`).
- **SPEC §10 ADR-0078 anticipation** (ADR-0033 partial supersession enumeration — clauses 1, 4, 7 fully preserved; 5, 6, 9 preserved with caveats; 2, 3, 8 superseded) → **ADR-0078** (lands Task 9, the `internal/listener/manager.go` rewrite of `validateFilterChainMatch` + `default_filter_chain` parse + `listener_filters[]` parse; first use of the supersession enumeration in production code at the listener-manager boundary).
- **SPEC §10 ADR-0083 anticipation** (ADR-0050 disposition — no supersession; `application_protocols` chain-match and HCM-internal ALPN dispatch coexist) → **ADR-0083** (lands Task 1, the PROGRESS preamble alongside ADR-0077; this ADR is mainly explanatory and doesn't anchor a code change — per SPEC §10 "Lands-in-task: 07.2 PLAN Task wherever the integration is documented (likely the PROGRESS preamble)"; pairing it with ADR-0077 at T1 keeps the PROGRESS preamble's ADR list cohesive).

Note: the FIRST-USE ORDERING is Tasks 1, 2, 4, 5, 5, 9, 1 — i.e. ADR-0077 + ADR-0083 at T1 (paired); ADR-0079 at T2; ADR-0082 at T4; ADR-0080 + ADR-0081 at T5 (paired); ADR-0078 at T9. This produces an ADR-number-vs-commit-order sequence (0077, 0083, 0079, 0082, 0080, 0081, 0078) — non-monotonic in three places. Per SPEC §10's explicit permission ("the planner may permute commit-time landings if that reads more naturally in PLAN.md") and per the 05.2 ADR-0055..ADR-0058 + 06.1 ADR-0059..ADR-0064 + 06.2 ADR-0066..ADR-0069 + 07.1 ADR-0070..ADR-0076 precedents (all four used non-monotonic commit-time orderings), the non-monotonic mapping is correct here. The contiguous-block discipline (ADR-0077..ADR-0083 inclusive, no gaps) is preserved; topical coherence drives the in-task pairing. The PLAN documents the mapping explicitly so the executor doesn't "fix" the ordering at execution time.

Summaries:

- **ADR-0077 — Phase-07.2 scope decision (split confirmation + listener-filter MVP boundary).** Status: Accepted. Date: task-execution date. Doctrine: D-3.5 + D-3.6. Decision: 07.2 covers (a) `listener_filters[]` framework with `tls_inspector` as the first concrete filter; (b) full 8-dimension `FilterChainMatch` algorithm; (c) `Listener.default_filter_chain` honored. Explicit deferrals: `original_dst`, `proxy_protocol`, `http_inspector` listener filters; `direct_source_prefix_ranges` chain-match dimension; xDS LDS; listener-level access logging on chain-match-miss; per-listener-filter metrics. Rationale: the MVP dispatch pipeline is exercised by `tls_inspector` alone — the filter's contribution to `ChainMatchInputs.ServerName` + `.ApplicationProtocols` exercises the same dispatch surface `original_dst`'s contribution to `.DestinationPort` would; additional surfaces are purely additive (new packages + new Register calls). Mirrors ADR-0070's 07.1 scope-confirmation pattern. **Anchors the ROADMAP edit landed at the SPEC commit (row 07.2 → in-progress)** — which is the 07.1 REVIEW I-3 corrected pattern continued. Lands in Task 1 (the PROGRESS preamble — first commit of the implementation session; the ROADMAP edit anchored by this ADR already landed at master `bb5f437` per SPEC drafting). Supersedes nothing.

- **ADR-0078 — ADR-0033 partial supersession enumeration.** Status: Accepted. Date: task-execution date. Doctrine: D-3.5. **Supersedes (partial):** ADR-0033 (Phase-03 filter-chain subset). Decision: clauses 1, 4, 7 fully preserved; clauses 5, 6, 9 preserved with caveats (clause 5: `default_filter_chain` may have an independent TLS posture; clause 6: plaintext multi-chain is now allowed except when any chain populates `server_names`; clause 9: SNI-internal sub-ordering becomes the tie-breaker WITHIN the new 8-dimension priority list's `server_names` slot, with no-match falling through to `default_filter_chain` if set); clauses 2, 3, 8 superseded (clause 2: the `filter_chain_match` whitelist is partially superseded — only `direct_source_prefix_ranges` stays silent-skipped post-07.2, all other dimensions are honored; clause 3: the parse-time error on `default_filter_chain` is totally superseded — 07.2 honors the field; clause 8: silent-skip on `listener_filters[]` is totally superseded — 07.2 honors the field). Full clause-by-clause table in SPEC §5.7. Rationale: the 07.2 deliverables explicitly settle each clause; recording the supersession enumeration in a dedicated ADR makes it grep-verifiable from `DECISIONS.md` (a future reader of ADR-0033 sees "supersession-of-ADR-0033 enumerated in ADR-0078"). Lands in Task 9 (the `internal/listener/manager.go` rewrite of `validateFilterChainMatch` — the first task that materially realizes the supersession in production code).

- **ADR-0079 — Listener-filter dispatch protocol shape (sync-only; narrow `Inspect(peeker, inputs)` surface; freeze-after-boot registry; two-step factory pattern).** Status: Accepted. Date: task-execution date. Doctrine: D-3.2 (write from scratch) + D-3.5. Decision: `ListenerFilter` interface single method `Inspect(ctx, peeker, inputs) (Status, error)` + `OnDestroy()`; `Status` enum 2-state (`Continue`, `StopIteration`); synchronous-only (no async-resume — Decision A from SPEC §6.1); `*ListenerFilterRegistry` threaded constructor (mirrors 07.1 ADR-0072 + 06.1 LBP-1 from ADR-0059); two-step factory pattern (`ListenerFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)` parses + validates `typed_config` once at HCM-build time; `FilterInstanceFactory func() ListenerFilter` allocates per-connection); per-connection sequential dispatch (Decision D from SPEC §1 #1 supports multi-filter pipelines but MVP registers only `tls_inspector`); 4096-byte default peeker buffer (Decision C from SPEC §5.3; clamped [256, 65536] via `initial_read_buffer_size` proto). Rationale: listener filters have a much narrower surface than HTTP filters (peek + populate inputs + return); async-resume + watermark events + body-buffering would all be unjustified machinery at MVP — the inputs are a peek-buffer + a known-shape connection-level facts struct; the output is a chain-match-inputs mutation; no realistic listener filter needs async-resume. Adding it later is straightforward (mirror 07.1's channel-based mechanic) but unjustified at MVP. Alternatives considered: (A) Envoy's full listener-filter API (with `SetCallbacks`-style callback registration + watermark-aware buffered I/O) — rejected for YAGNI; the methods we drop have no in-scope callers in 07.2's filter set, and any future listener filter that needs them would re-litigate via its own ADR; (B) per-filter goroutine — rejected because spawning a goroutine per filter per connection is goroutine-bloat (each connection already has a goroutine for the accept-loop dispatch). Consequences: (a) the framework's external dependencies are limited to the Go stdlib + `google.golang.org/protobuf` + `tls_inspector v3 proto` — no third-party listener-filter library; (b) the dispatch-protocol shape documented in `internal/listener/listenerfilter/doc.go`; (c) future family phases that introduce additional listener filters (e.g., `original_dst`, `proxy_protocol`) extend this package by adding to the `ListenerFilter` interface — each such addition lands its own ADR in the family phase that needs it. Lands in Task 2 (the `types.go` + `callbacks.go` introduction). Supersedes nothing.

- **ADR-0080 — `default_filter_chain` semantics (no-match fallback; empty-match chain BEATS default_filter_chain; TLS posture independent).** Status: Accepted. Date: task-execution date. Doctrine: D-3.3 (differential correctness beats internal fidelity) + D-3.5. **Supersedes:** ADR-0033 clause 3. Decision: `default_filter_chain` is consulted ONLY when no `filter_chains[]` entry's `filter_chain_match` matches the per-connection `ChainMatchInputs`; an empty-match chain in `filter_chains[]` BEATS `default_filter_chain` when both coexist (the empty-match chain is universally eligible at Pass 1 of the chain-match algorithm); `default_filter_chain` may carry an independent `transport_socket` (TLS or plaintext) regardless of the `filter_chains[]` entries' TLS posture (the mixed-TLS-and-plaintext rule from ADR-0033 clause 5 applies WITHIN `filter_chains[]` only). Empirical evidence: SPEC §11.1 (verbatim Envoy stats showing `default_filter_chain` honored as no-match fallback) + §11.2 (verbatim Envoy stats showing empty-match chain beats `default_filter_chain` — `tcp.tcp_empty.downstream_cx_total: 1`, `tcp.tcp_default.downstream_cx_total: 0`). Both pinned at server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` per ENVOY_TARGET. Rationale: the SPEC-time empirical pin is authoritative — envoy-go matches Envoy's documented semantics. Alternatives considered: (A) `default_filter_chain` ALWAYS preferred (i.e., bypass `filter_chains[]` if `default_filter_chain` is set) — rejected per §11.1 pin (Envoy honors `filter_chains[]` first); (B) error if both empty-match chain AND `default_filter_chain` exist — rejected per §11.2 pin (Envoy boots cleanly on the combined config). Consequences: (a) the `chainmatch.SelectChain` algorithm consults `defaultChain` ONLY when `len(eligibleChains) == 0`; (b) the `catchAllCount > 1` validation at `internal/listener/manager.go:308` is preserved for `filter_chains[]` empty-match entries; the `default_filter_chain` is a SEPARATE structural slot and does NOT count toward `catchAllCount` (so 0/0, 1/0, 0/1, 1/1 are all valid combinations of empty-match-chain-count + default-chain-presence); (c) the mixed-TLS rule applies WITHIN `filter_chains[]` only; `default_filter_chain` is independent. Lands in Task 5 (the `chainmatch.SelectChain` default-fallback path).

- **ADR-0081 — `FilterChainMatch` 8-dimension precedence algorithm.** Status: Accepted. Date: task-execution date. Doctrine: D-3.5 + D-3.6. **Supersedes (partial):** ADR-0033 clause 2. Decision: priority order (highest priority first) `[destination_port, prefix_ranges, server_names, transport_protocol, application_protocols, source_type, source_prefix_ranges, source_ports]`; eligibility-then-specificity 2-pass algorithm — Pass 1 eliminates chains whose specified dimensions don't match the inputs (chains with all-zero match are universally eligible); Pass 2 scores each surviving chain by the priority-ordered specificity vector (8-bit integer where bit `i` is set iff dimension `priorityOrder[i]` is specified; chain with highest specificity integer wins); ties broken at finer grain — longer CIDR prefix on `prefix_ranges`/`source_prefix_ranges`, SNI-specificity on `server_names` per ADR-0033 clause 9 preserved as special case (the existing `chainSpecificityRank` function at `internal/listener/manager.go:352` is reused verbatim by `chainmatch.SelectChain`'s `breakTie`); final ties (chains identical on all 8 dimensions) error at `NewManager`-build time with `listener: %q: filter_chains[i] and filter_chains[j] have identical filter_chain_match — ambiguous selection`. Empirical evidence: SPEC §11.3 (verbatim Envoy stats showing `destination_port` BEATS `source_prefix_ranges` — `tcp.tcp_dstport.downstream_cx_total: 1`, `tcp.tcp_srcprefix.downstream_cx_total: 0`) at server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`. Rationale: the priority order matches the upstream `filter_chain_match.proto` documented order; the empirical pin confirms the priority is realized in dispatch. Alternatives considered: (A) per-priority-level eligibility (eliminate chains as soon as ANY higher-priority dimension's value differs) — rejected because it introduces O(N²) worst-case lookup and doesn't respect Envoy's documented priority semantics; (B) string-based pattern matching (regex) — rejected as out-of-scope. Consequences: (a) the algorithm is O(N × D) where N = number of chains, D = 8 (constant); (b) chain-list immutability after `NewManager`-build means concurrent `SelectChain` calls are read-only and lock-free; (c) the algorithm's worst-case latency per connection is microseconds — well below the `listener_filters_timeout` budget. Lands in Task 5 (the `chainmatch.SelectChain` algorithm; same task as ADR-0080).

- **ADR-0082 — `listener_filters_timeout` honored in [1s, 60s]; default 15s; `continue_on_listener_filters_timeout` honored.** Status: Accepted. Date: task-execution date. Doctrine: D-3.5 + D-3.6. Decision: `Listener.listener_filters_timeout` proto field honored, with values clamped/validated to the [1s, 60s] envelope; default 15s if unset; values outside envelope error at parse with `listener: %q: listener_filters_timeout %s is outside the supported [1s, 60s] envelope`. `continue_on_listener_filters_timeout` honored as-is per the proto's documented semantics: `false` (default) → timeout aborts the connection; `true` → timeout treats the listener-filter pipeline as having returned `Continue` and proceeds to chain match with partial inputs. Per-pipeline shared budget — a single `context.WithTimeout(ctx, timeoutMs)` is established once before the pipeline loop and shared across all filters' `Inspect` calls (per Decision N from SPEC §6.5 — NOT per-filter time-slicing; a slow first filter eats subsequent filters' budget; that's correct because the user's budget is the budget for all filters combined). Rationale: the [1s, 60s] envelope reflects realistic deployment values; values < 1s risk false-positive timeouts under CI scheduler jitter; values > 60s indicate misconfiguration (the listener-filter pipeline should NEVER take longer). Alternatives considered: (A) per-filter timeout (each filter gets the full budget independently) — rejected per Decision N (forces `len(filters)` context-derivations per connection AND penalizes multi-filter pipelines unfairly; the proto's documented semantics are per-pipeline); (B) hardcoded 15s ignoring the proto field — rejected because the proto field is widely used in real Envoy deployments; honoring it is doctrine D-3.5. Consequences: (a) the bootstrap parser's envelope-check error message is consistent with the rest of `internal/listener/manager.go`'s error-message conventions; (b) `Pipeline.Run` takes a `timeoutMs uint32` parameter (0 = no-op; the listener manager passes `lfTimeoutMs` populated from the parsed proto); (c) a future hardening phase may revisit the envelope (e.g., relax the upper bound for slow-network deployments) with its own ADR. Lands in Task 4 (the `Pipeline.Run` timeout enforcement; the bootstrap parser's envelope check at Task 9 cross-references this ADR).

- **ADR-0083 — ADR-0050 disposition (no supersession; `application_protocols` chain-match and HCM-internal ALPN dispatch coexist).** Status: Accepted. Date: task-execution date. Doctrine: D-3.5. **Settles:** ADR-0050 (ALPN-driven codec selection inside `Filter.Handle`). Decision: ADR-0050 stays in force; 07.2's `application_protocols` chain-match field and ADR-0050's HCM-internal ALPN dispatch are orthogonal mechanisms (chain-selection vs codec-selection). They coexist by construction: a single-HCM-with-AUTO-codec listener uses ADR-0050's mechanic (HCM type-asserts on `*tls.Conn` and reads `NegotiatedProtocol`); a multi-chain listener with per-chain `application_protocols` + per-chain forced `codec_type` uses 07.2's mechanic (chain-match selects between HCM-h2 and HCM-h1 chains; AUTO branch in HCM is a no-op when `codec_type` is forced). Rationale: ADR-0050 governs codec-selection (which Go-level codec implementation runs the request); 07.2's `application_protocols` governs chain-selection (which `filter_chain` entry runs at all). The two are orthogonal axes of the listener-side dispatch pipeline. Empirical evidence: SPEC §11.4 (carry-forward — resolved at Task 16 per Decision K) documents the dispatch interaction. Alternatives considered: (A) supersede ADR-0050 (move ALPN dispatch entirely into the chain-match algorithm) — rejected because ADR-0050 covers the AUTO-codec case which is independent of chain-match (a single-chain listener with no `application_protocols` still needs ADR-0050's HCM-internal mechanic); (B) supersede ADR-0050 partially (chain-match preferred when both could fire) — rejected as confusing; the orthogonality is cleaner. Consequences: (a) ADR-0050 is preserved verbatim; (b) `BEHAVIOR_CONTRACT.md ## TLS "Scope boundaries"` removes "ALPN-driven filter-chain selection" from the out-of-scope enumeration but does NOT remove "ALPN-driven codec selection inside Filter.Handle" (which is ADR-0050's purview, still in scope and still asserted); (c) future xDS phases that revisit ALPN dispatch consult both ADRs. Lands in Task 1 (the PROGRESS preamble alongside ADR-0077; explanatory ADR with no production-code anchor). Supersedes nothing.

---

## Settled SPEC §14 deferred decisions

The SPEC §14 enumerates 15 deferred decisions (A–O); 13 are already settled in the SPEC body, 2 are explicitly recommended for the planner. The PLAN-time settlements:

- **Decision A — Listener-filter dispatch protocol: sync-only at MVP.** Settled in SPEC §6.1; recorded in ADR-0079.
- **Decision B — Listener-filter pipeline timeout: 15s default, [1s, 60s] envelope.** Settled in SPEC §6.5; recorded in ADR-0082. Planner-time refinement: the envelope-check error message format `listener: %q: listener_filters_timeout %s is outside the supported [1s, 60s] envelope` matches the rest of `internal/listener/manager.go`'s error-message conventions (verified at PLAN time by inspection of existing errors at lines 247, 252, 257, 286, 290, 294 — all begin with `listener: %q:`). Task 9 step 4 implements this validation.
- **Decision C — Peeker buffer size: 4096 default; clamped [256, 65536].** Settled in SPEC §5.3; recorded in ADR-0079. Planner-time refinement: lower bound 256 verified at PLAN time — minimum ClientHello is ~50 bytes (TLS record header 5 + Handshake header 4 + ClientHello body ~40+ + extensions varies); 256 is safely above this floor. The clamp is implemented in `tls_inspector/proto.go` at Task 8 step 5.
- **Decision D — Multi-listener-filter pipelines: supported at MVP via sequential dispatch; only `tls_inspector` registered.** Settled in SPEC §1 #1 + §6.1 + §6.4; recorded in ADR-0079. Planner-time choice: the `pipeline_test.go` 2-filter test at Task 4 uses two `tls_inspector` instances with different `initial_read_buffer_size` values (256 and 4096) rather than introducing test-only filters. This avoids test-only Go code in production packages and exercises the same code paths.
- **Decision E — `default_filter_chain` semantics: no-match fallback; empty-match wins; TLS independent.** Settled in SPEC §5.7 + §8; recorded in ADR-0080.
- **Decision F — `original_dst` listener filter: deferred.** Settled in SPEC §1 #3 + §2.1; recorded in ADR-0077. Future-phase pointer: a dedicated transparent-proxy phase OR the network-filters family — neither on the BOOTSTRAP MVP trunk; PLAN records as a pending future-phase item.
- **Decision G — Fixture id + shape: single fixture `0008-listener-chain-match`; differential; backend-port routing as the differential dimension; dual-listener (`l_test_a` + `l_test_b`) construction with a connection-4-only configuration variant (`chain_other` omitted) for the no-match → `default_filter_chain` path.** Settled in SPEC §7.4 + §9; recorded in ADR-0077. Planner-time refinements: backend-port allocations are dynamic (the runner's `freeTCPPort` + `BackendCount = 5` allocates 5 backends with OS-picked ports per fixture run; the driver templates them into both bootstrap pairs); the `<known_driver_port>` value is also OS-picked at driver run time via the driver pre-allocating one source port via `net.Listen("tcp", "127.0.0.1:0")` then closing the listener and using the captured port number for the connection-2 + connection-5 forced-source-bind; the source-bind for connections 2 and 5 uses `net.Dialer.LocalAddr` set to `&net.TCPAddr{IP: 127.0.0.1, Port: <known_driver_port>}` per Go stdlib idiom; the c4-variant subj+ref is spawned by the runner via the new `AlternateConfigDriver` interface added at Task 13. Recorded in `expectations.yaml` at Task 15. The dual-listener-with-c4-variant shape is preserved per Decision G's "cannot revisit without ADR" clause.
- **Decision H — ADR-0050 disposition: no supersession; `application_protocols` chain-match and HCM-internal ALPN dispatch coexist.** Settled in SPEC §2.5; recorded in ADR-0083.
- **Decision I — ADR-0033 supersession enumeration.** Settled in SPEC §5.7; recorded in ADR-0078.
- **Decision J — Listener-filter API mirrors 07.1's HTTP-filter API at the surface level (registry + 2-step factory + freeze-after-boot) but is narrower (1 method `Inspect`; 2-state status; no callbacks).** Settled in SPEC §6; recorded in ADR-0079.
- **Decision K — `tls_inspector`-populated ALPN empirical pin: carry-forward to impl time.** Settled in SPEC §11.4. **Resolved at Task 16** (the fixture-0008 driver task; the executor produces the verbatim Envoy evidence by spawning a real Envoy v1.37.2 container with a TLS bootstrap + `tls_inspector` listener filter + multi-chain `application_protocols` matching, then issues an HTTPS-h2 connection from a Go driver with `NextProtos = ["h2"]`, captures the `/stats?filter=tcp_(h2|h1).downstream_cx_total` output, and pastes it verbatim into both SPEC §11.4 (replacing the carry-forward placeholder) and `BEHAVIOR_CONTRACT.md ## Listener filters` at Task 17 step 1). The 06.1 SN4 + 06.2 default-format empirical-pin patterns are the precedents.
- **Decision L — `chainmatch.SelectChain` chain ordering: declaration order (input-list order) preserved; chains NOT pre-sorted; specificity-scored at dispatch time.** Settled in SPEC §5.5; PLAN records: Task 5 step 1 implements `SelectChain` to operate on the unsorted slice; the `sort.SliceStable` chain-sort at `internal/listener/manager.go:327` is REMOVED at Task 9 (the `chainSpecificityRank`-based sort is replaced by per-call scoring in `SelectChain`).
- **Decision M — Concrete ADR numbers ADR-0077..ADR-0083.** Per `DECISIONS.md` tail at master `bb5f437` being ADR-0076, the next-free is ADR-0077; 07.2's seven ADRs land at ADR-0077..ADR-0083. The planner re-verified at PLAN-write time. Topical ordering (scope / ADR-0033-supersession / dispatch-protocol / default-chain / chain-match-algorithm / timeout / ADR-0050-disposition) is the documented authoring order; non-monotonic commit-time mapping (per `## ADRs introduced by this plan` above) is permitted per SPEC §10's explicit clause and the four prior precedents.
- **Decision N — `pipeline.go` per-filter timeout split: per-pipeline (Decision B) NOT per-filter.** Settled in SPEC §6.5; recorded in ADR-0082.
- **Decision O — `chainmatch.go` builds the `[]*ChainSpec` from the bootstrap's `filter_chains[]` at `NewManager` time; the `*ChainSpec` is immutable thereafter.** Settled in SPEC §5.6; PLAN records: Task 9 step 5 builds the slice on `listenerRuntime.chains` at `NewManager` time; concurrent reads are safe by construction (no `sync.Map` overhead per the SPEC's recommendation (a)).

---

## Carry-forward triage

Carry-forward items from prior phases that the 07.2 PLAN must NOT re-litigate:

- **07.1 REVIEW Minors / Carry-forwards.** None applicable to 07.2 (07.1's surface is HCM-internal; 07.2's surface is listener-side; zero overlap).
- **06.2 REVIEW Minors / Carry-forwards.** None applicable to 07.2 (06.2's surface is access-log; 07.2 doesn't touch access-log).
- **04 / 05.x deferred items.** None re-litigated by 07.2; the H2 ctx-cancel zero-status sentinel is preserved as 06.2 inheritance and unchanged.
- **05.2 REVIEW M-9 (h2RouterActionAdapter log line).** Already resolved in 06.1; 07.2 doesn't touch it.
- **SPEC §11.4 carry-forward** (`tls_inspector`-populated ALPN empirical pin). **Resolved at Task 16** per Decision K; the executor produces the verbatim Envoy evidence and pastes it both in SPEC §11.4 (replacing placeholder) and in `BEHAVIOR_CONTRACT.md ## Listener filters` (initial population at Task 17 step 1).

---

## Execution preconditions

Before starting Task 1, the executor verifies:

1. **Worktree.** Operate on `phase/07.2-listener-chain-completion-impl` worktree (recommended: `.worktrees/phase-07.2-listener-chain-completion-impl/` per ADR-0003). Branch base is master tip after the PLAN.md SHA-fill follow-up commit.
2. **Branch state.** `git status` shows clean tree on the impl branch; no uncommitted changes.
3. **Toolchain.** `go version` reports go1.23+; `golangci-lint version` reports 1.64.8 per ADR-0009; `docker version` reports both client + server.
4. **Pre-existing fixtures green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b' -v` passes for every fixture. If any pre-existing fixture is RED at Task 1, stop and invoke `superpowers:systematic-debugging` — fixing must precede 07.2's surface.
5. **go-control-plane pin.** `go list -m github.com/envoyproxy/go-control-plane/envoy` reports `v1.32.4` per ADR-0013. If different, stop — D-3.7 reserves pin bumps for dedicated phases.
6. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1` returns `ADR-0076:`. If different (a mid-PLAN-authoring ADR landed since the SPEC commit), every task's ADR reference shifts uniformly — re-number 07.2 ADRs sequentially from `tail + 1` BEFORE starting Task 2.
7. **SPEC commit present.** `git log -1 --format=%H -- docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md` returns `bb5f437` or descendant.
8. **`internal/listener/listenerfilter/` absent.** `test ! -d internal/listener/listenerfilter && echo OK` reports `OK` (the package does not yet exist at impl session entry; first creation is at Task 2).
9. **`internal/listener/manager.go` line numbers.** The pre-07.2 file is 585 LoC; key extension points: `validateFilterChainMatch` at line 378; `chainSpecificityRank` at line 352; chain-sort at line 327; `default_filter_chain` error at line 251; `dispatch` at line 434; `makeGetConfigForClient` at line 413; `serveTLS` at line 550. Verified by `grep -nE '^func validateFilterChainMatch|^func chainSpecificityRank|^func \(rt \*listenerRuntime\) dispatch|^func makeGetConfigForClient|^func \(rt \*listenerRuntime\) serveTLS' internal/listener/manager.go`. If line numbers drift (e.g., from an unrelated mechanical-fix per ADR-0017), update Task 9 + Task 10 step references at execution time.
10. **BEHAVIOR_CONTRACT.md sections present.** `grep -nE '^## TCP proxy$|^## TLS$|^## HTTP filter chain$|^## xDS wire state machine$' docs/envoy-go/BEHAVIOR_CONTRACT.md` reports four matches (lines 330, 372, 514, 250). If any is missing, stop and invoke `superpowers:systematic-debugging` — the closing-sweep edit at Task 17 depends on these anchors.
11. **HTTPRegistry symbol present.** `grep -nE 'HTTPRegistry' internal/filter/http/registry.go` reports the 07.1 deliverable. The 07.2 `*ListenerFilterRegistry` mirrors its discipline.
12. **`tls_inspector v3` proto type pullable.** `go list github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3 2>&1 | head -1` returns the package path (no error). The blank-import at Task 11 step 5 depends on this.
13. **Reference Envoy image pull.** `docker pull envoyproxy/envoy:v1.37.2` succeeds. The image is needed for fixture 0008's reference container at Task 16. If the pull fails (network / Docker auth), Tasks 1–15 still proceed; Task 16 blocks on this.
14. **CONFORMANCE_PINS.md UNCHANGED.** `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` reports zero changes (D-3.7).

If all 14 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble [ADR-0077, ADR-0083]

**Files:**
- Create: `docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0077 + ADR-0083)

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. ADR-0077 (Phase-07.2 scope decision) lands here as the first ADR of the seven, anchored at the PROGRESS preamble — the implementation session's first commit — since the ROADMAP edit that the scope decision formalizes already landed at master `bb5f437` (the SPEC commit), and T1 is the first opportunity to land the ADR after the SPEC commit's ROADMAP edit. ADR-0083 (ADR-0050 disposition) lands alongside ADR-0077 at the PROGRESS preamble per SPEC §10's "lands wherever the integration is documented (likely the PROGRESS preamble; this ADR is mainly explanatory)" — pairing it with ADR-0077 keeps the PROGRESS preamble's ADR list cohesive.

**Precondition:** worktree exists at `phase/07.2-listener-chain-completion-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up.
**Artifact:** `docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md` (new file); `docs/envoy-go/DECISIONS.md` (ADR-0077 + ADR-0083 appended).
**Acceptance:** all 14 preconditions report green; PROGRESS.md preamble entry committed; ADR-0077 + ADR-0083 appear in `DECISIONS.md` with full Context/Decision/Consequences sections per the ADR-0001 template.
**Verification command:** `git log -1 --format=%H -- docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md` returns the Task 1 commit's SHA; `grep -nE '^## ADR-007[78]' docs/envoy-go/DECISIONS.md | wc -l` returns 1; `grep -nE '^## ADR-0083:' docs/envoy-go/DECISIONS.md | wc -l` returns 1.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase/07.2-listener-chain-completion-impl
git log --oneline master | head -3                                    # expect: PLAN SHA-fill, PLAN commit, SPEC SHA-fill or SPEC commit
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: golangci-lint has version 1.64.8
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b' -v
                                                                       # expect: every fixture PASS
go list -m github.com/envoyproxy/go-control-plane/envoy               # expect: v1.32.4
grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
                                                                       # expect: ADR-0076:
git log -1 --format=%H -- docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md
                                                                       # expect: bb5f437... or descendant
test ! -d internal/listener/listenerfilter && echo OK                  # expect: OK
grep -nE '^func validateFilterChainMatch|^func chainSpecificityRank|^func \(rt \*listenerRuntime\) dispatch|^func makeGetConfigForClient|^func \(rt \*listenerRuntime\) serveTLS' internal/listener/manager.go
                                                                       # expect: 5 matches
grep -nE '^## TCP proxy$|^## TLS$|^## HTTP filter chain$|^## xDS wire state machine$' docs/envoy-go/BEHAVIOR_CONTRACT.md
                                                                       # expect: 4 matches
go list github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3 2>&1 | head -1
                                                                       # expect: package path returned
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
git diff master -- docs/envoy-go/CONFORMANCE_PINS.md                  # expect: empty
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md`**

```markdown
# Phase 07.2 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2/06.1/06.2/07.1 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all 14 preconditions were satisfied at cold-start>

## Task 1 — Execution-precondition check + PROGRESS.md preamble [ADR-0077, ADR-0083]

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 14 preconditions per PLAN §"Execution preconditions"; phase-07.1 close confirmed present in HEAD; SPEC at <SPEC SHA>; ADR tail at 0076 (next-free 0077); internal/listener/listenerfilter/ absent (the package implementation lands at Task 2+); manager.go line numbers verified at 251/327/352/378/413/434/550. Landed ADR-0077 (phase-07.2 scope decision) + ADR-0083 (ADR-0050 disposition; coexistence not supersession).
**Outputs:**
\`\`\`
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ go version
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
<verbatim>
$ git log -1 --format=%H -- docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md
<verbatim>
$ test ! -d internal/listener/listenerfilter && echo OK
OK
\`\`\`
```

- [ ] **Step 3: Append ADR-0077 to `docs/envoy-go/DECISIONS.md`**

Append to the file's tail (after ADR-0076; preserve existing content verbatim). Use the ADR-0001 template structure (Status / Doctrine / Lands-in-task / Context / Decision / Consequences / Supersedes). Body content is the ADR-0077 summary above; flesh out Context (the SPEC §1 + parent BRAINSTORM §1 design history); Decision (the three deliverables enumerated in SPEC §1); Consequences (a–d enumerated in the summary).

- [ ] **Step 4: Append ADR-0083 to `docs/envoy-go/DECISIONS.md`**

Append to the file's tail (after ADR-0077; preserve existing content verbatim). Use the same ADR-0001 template. Body content is the ADR-0083 summary above; flesh out Context (the SPEC §2.5 question — does 07.2's `application_protocols` supersede ADR-0050?); Decision (the orthogonality argument: chain-selection vs codec-selection); Consequences (a–c enumerated in the summary).

- [ ] **Step 5: Run preconditions verbatim and confirm pristine state**

```bash
go vet ./...                                                  # expect: clean
golangci-lint run ./...                                       # expect: clean
go test -race -count=1 -short ./...                           # expect: all PASS (short mode skips differential)
```

- [ ] **Step 6: Commit**

```bash
git add docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md docs/envoy-go/DECISIONS.md
git commit -m "phase 07.2: PROGRESS preamble + ADR-0077 (scope decision) + ADR-0083 (ADR-0050 disposition) [ADR-0077, ADR-0083]"
```

SHA-fill follow-up.

*Anchored: SPEC §1, §10 (ADR-0077 + ADR-0083 anticipations), §16 (acceptance bullet for ADRs in DECISIONS.md), and BOOTSTRAP §5.3 (commit-message-completeness).*

---

## Task 2: `internal/listener/listenerfilter/{doc,types,callbacks}.go` — interfaces, status enum, ChainMatchInputs, Peeker, two-step factory pattern, peekerConn [ADR-0079]

**Files:**
- Create: `internal/listener/listenerfilter/doc.go`
- Create: `internal/listener/listenerfilter/types.go`
- Create: `internal/listener/listenerfilter/callbacks.go`
- Create: `internal/listener/listenerfilter/types_test.go`
- Create: `internal/listener/listenerfilter/callbacks_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0079)

This task introduces the `internal/listener/listenerfilter/` package's foundational types: the `ListenerFilter` interface, the `ListenerFilterStatus` enum, the `ChainMatchInputs` struct holding the eight chain-match dimensions, the `Peeker` interface, the `ListenerFilterFactory` + `FilterInstanceFactory` two-step factory pattern, the `FactoryCtx` carrier, and the `peekerConn` concrete implementation. Tests in `types_test.go` cover the value semantics of `ChainMatchInputs` (zero values, IsLoopbackSource() against IPv4/IPv6 loopback / non-loopback / nil); tests in `callbacks_test.go` cover the `peekerConn` bytes-not-consumed invariant under interleaved Peek/Read scenarios. ADR-0079 (Listener-filter dispatch protocol shape) lands in this commit per SPEC §10's "first use of the dispatch-protocol shape in production code".

**Precondition:** Task 1 done; `internal/listener/listenerfilter/` directory does not yet exist.
**Artifact:** the new package's `doc.go` + `types.go` + `callbacks.go` + their tests; ADR-0079 in DECISIONS.md.
**Acceptance:** `go vet ./internal/listener/listenerfilter/...` clean; `go test ./internal/listener/listenerfilter/...` passes (only `types_test.go` + `callbacks_test.go` exist at this task; subsequent tasks add registry / pipeline / chainmatch tests); the `peekerConn.Peek(n)` returns first n bytes without consuming, subsequent `Read` returns the same bytes; `ListenerFilter` interface is `interface{ Inspect(ctx, peeker, inputs) (Status, error); OnDestroy() }`; `Status` enum has exactly two values `Continue = 0` and `StopIteration = 1`.

- [ ] **Step 1: Write `types_test.go` failing tests**

```go
package listenerfilter

import (
	"net"
	"testing"
)

func TestChainMatchInputsZeroValueIsBenign(t *testing.T) {
	var c ChainMatchInputs
	if c.DestinationIP != nil { t.Errorf("zero ChainMatchInputs DestinationIP should be nil; got %v", c.DestinationIP) }
	if c.DestinationPort != 0 { t.Errorf("zero ChainMatchInputs DestinationPort should be 0; got %d", c.DestinationPort) }
	if c.IsLoopbackSource() { t.Errorf("zero ChainMatchInputs IsLoopbackSource should be false") }
}

func TestChainMatchInputsIsLoopbackSource(t *testing.T) {
	cases := []struct {
		name   string
		ip     net.IP
		want   bool
	}{
		{"IPv4 127.0.0.1", net.ParseIP("127.0.0.1"), true},
		{"IPv4 127.255.255.254", net.ParseIP("127.255.255.254"), true},
		{"IPv6 ::1", net.ParseIP("::1"), true},
		{"IPv4 192.168.1.1", net.ParseIP("192.168.1.1"), false},
		{"IPv4 10.0.0.1", net.ParseIP("10.0.0.1"), false},
		{"IPv6 2001:db8::1", net.ParseIP("2001:db8::1"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := ChainMatchInputs{SourceIP: tc.ip}
			if got := c.IsLoopbackSource(); got != tc.want {
				t.Errorf("IsLoopbackSource()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestStatusEnumValues(t *testing.T) {
	if Continue != 0 || StopIteration != 1 {
		t.Errorf("Status enum drift: Continue=%d StopIteration=%d", Continue, StopIteration)
	}
}
```

- [ ] **Step 2: Run tests; confirm they fail (file does not exist)**

```bash
go test ./internal/listener/listenerfilter/... 2>&1 | head -20
```

Expected: build error (`no Go files`).

- [ ] **Step 3: Write `doc.go`**

```go
// Package listenerfilter implements envoy-go's listener-side filter dispatch
// pipeline (the listener-filter framework introduced by phase 07.2). The
// package defines:
//
//   - ListenerFilter: the per-connection filter interface (Inspect + OnDestroy).
//   - ListenerFilterStatus: 2-state enum (Continue, StopIteration); no
//     async-resume per ADR-0079.
//   - ChainMatchInputs: 8-field struct holding the chain-match dimensions
//     populated from connection-level facts (DestinationIP/Port,
//     SourceIP/Port) plus listener-filter-contributed fields (ServerName,
//     TransportProtocol, ApplicationProtocols).
//   - Peeker: peek-without-consume interface backed by peekerConn (a
//     bufio.Reader-wrapped net.Conn).
//   - ListenerFilterRegistry: boot-populated, frozen-after-boot registry
//     mapping type_url → factory; mirrors *filter_http.HTTPRegistry from
//     07.1 (ADR-0072) and *stats.Registry from 06.1 (ADR-0059).
//   - Pipeline: per-connection sequential dispatch with shared per-pipeline
//     timeout (ADR-0082).
//   - chainmatch.SelectChain: 8-dimension precedence algorithm (ADR-0081)
//     consulting default_filter_chain (ADR-0080) on no-match.
//
// Architecture: the listener manager's accept-loop allocates a Pipeline per
// accepted connection, wraps the raw conn in a peekerConn, runs the
// listener-filter pipeline (which populates ChainMatchInputs), then calls
// chainmatch.SelectChain to pick the filter chain. The selected chain's TLS
// handshake (if any) runs next, then dispatch falls through to the chain's
// terminal filter unchanged. See SPEC §5.2 for the per-connection lifecycle
// and SPEC §5.5 for the chain-match algorithm.
//
// Concurrency: single-goroutine-per-connection drives the pipeline +
// chain-match + dispatch; no shared mutable state on the hot path.
// ListenerFilterRegistry locks only at boot (Register/Lookup); post-Freeze
// reads are lock-free. See SPEC §5.6.
//
// Introduced by phase 07.2.
package listenerfilter
```

- [ ] **Step 4: Write `types.go`**

```go
package listenerfilter

import (
	"context"
	"net"

	"google.golang.org/protobuf/types/known/anypb"
)

// ListenerFilterStatus is the result of a ListenerFilter.Inspect call.
type ListenerFilterStatus int

const (
	// Continue allows the pipeline to advance to the next filter (or finish
	// if this is the last filter).
	Continue ListenerFilterStatus = 0
	// StopIteration halts the pipeline; remaining filters are skipped. The
	// chain-match algorithm runs on whatever inputs were populated so far.
	StopIteration ListenerFilterStatus = 1
)

// ChainMatchInputs holds the eight chain-match dimensions that
// chainmatch.SelectChain consults. Some fields are populated from
// connection-level facts at Pipeline construction (LocalAddr/RemoteAddr);
// others are populated by listener filters (e.g., tls_inspector contributes
// ServerName, TransportProtocol, ApplicationProtocols).
//
// The struct is mutated in place by listener filters during pipeline
// dispatch. Callers that need an immutable snapshot must copy.
type ChainMatchInputs struct {
	// DestinationIP is the listener's bound IP address (conn.LocalAddr().IP).
	DestinationIP net.IP
	// DestinationPort is the listener's bound port (conn.LocalAddr().Port).
	DestinationPort uint32
	// SourceIP is the accepted connection's peer IP (conn.RemoteAddr().IP).
	SourceIP net.IP
	// SourcePort is the accepted connection's peer port (conn.RemoteAddr().Port).
	SourcePort uint32
	// ServerName is the SNI extracted from the ClientHello by tls_inspector.
	// Empty when no TLS inspection happened or the ClientHello had no SNI.
	ServerName string
	// TransportProtocol is "tls" if a TLS ClientHello was detected,
	// "raw_buffer" if the byte preamble was non-TLS, or "" if no listener
	// filter inspected the connection.
	TransportProtocol string
	// ApplicationProtocols is the ALPN offer list extracted from the
	// ClientHello by tls_inspector. Empty when no TLS inspection happened
	// or the ClientHello had no ALPN extension.
	ApplicationProtocols []string
}

// IsLoopbackSource reports whether SourceIP is in the IPv4 127.0.0.0/8 range
// or is the IPv6 ::1 address. Used by the source_type: LOCAL match rule.
func (c *ChainMatchInputs) IsLoopbackSource() bool {
	return c.SourceIP != nil && c.SourceIP.IsLoopback()
}

// Peeker is the peek-without-consume interface listener filters call to
// inspect the connection's byte preamble. Returns up to n bytes without
// advancing the read position. Subsequent Read calls drain the same bytes.
type Peeker interface {
	Peek(n int) ([]byte, error)
}

// ListenerFilter is the per-connection filter interface. Implementations
// inspect the peeker (read without consuming) and write to inputs (mutate
// chain-match inputs in place). Return Continue to allow the pipeline to
// advance, StopIteration to halt the pipeline (whatever inputs were
// populated stand). On non-nil error the pipeline aborts with the error.
type ListenerFilter interface {
	Inspect(ctx context.Context, peeker Peeker, inputs *ChainMatchInputs) (ListenerFilterStatus, error)
	// OnDestroy is called when the per-connection pipeline ends (either
	// after dispatch completes or on connection close before dispatch).
	// Implementations release any per-connection resources here.
	OnDestroy()
}

// FactoryCtx carries the parsed-config context a ListenerFilterFactory
// needs to resolve the typed_config Any. Currently empty; reserved for
// future extensions (e.g., a Registry pointer for filters that compose).
type FactoryCtx struct{}

// ListenerFilterFactory parses + validates a listener filter's typed_config
// Any once at NewManager-build time and returns a per-connection
// FilterInstanceFactory closure. Mirrors 07.1's HTTPFilterFactory pattern
// (ADR-0072).
type ListenerFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)

// FilterInstanceFactory allocates a fresh ListenerFilter instance once per
// accepted connection. Per-config validation cost is paid once at boot;
// per-connection cost is one allocation.
type FilterInstanceFactory func() ListenerFilter
```

- [ ] **Step 5: Write `callbacks.go`**

```go
package listenerfilter

import (
	"bufio"
	"net"
)

// peekerBufSize is the default peek buffer size — matches Envoy's
// tls_inspector.initial_read_buffer_size default (Decision C, ADR-0079).
// The tls_inspector proto config can override per-listener; the override
// value is clamped to [256, 65536] (Decision C).
const peekerBufSize = 4096

// peekerConn wraps a net.Conn with a bufio.Reader so listener filters can
// Peek bytes without consuming them. Subsequent Read calls drain the same
// buffer first, then transition to the underlying conn once the buffer is
// exhausted. This is the same trick crypto/tls.Conn.Handshake() uses
// internally for resumption-cookie peek-back. See SPEC §5.3.
type peekerConn struct {
	net.Conn
	br *bufio.Reader
}

// NewPeekerConn allocates a peekerConn over c with a default-size peek
// buffer. The returned value satisfies both Peeker (via Peek) and net.Conn
// (via the embedded conn + the Read override).
func NewPeekerConn(c net.Conn) net.Conn {
	return &peekerConn{Conn: c, br: bufio.NewReaderSize(c, peekerBufSize)}
}

// NewPeekerConnSize is the size-configurable variant. size is clamped to
// [256, 65536] per ADR-0079 Decision C; values outside the range are
// silently clamped (the proto-config parser at tls_inspector/proto.go
// errors at parse time on out-of-range values).
func NewPeekerConnSize(c net.Conn, size int) net.Conn {
	if size < 256 {
		size = 256
	}
	if size > 65536 {
		size = 65536
	}
	return &peekerConn{Conn: c, br: bufio.NewReaderSize(c, size)}
}

// Peek implements Peeker. Returns up to n bytes without advancing the read
// position. Returns bufio.ErrBufferFull if n > the buffer's capacity.
func (p *peekerConn) Peek(n int) ([]byte, error) {
	return p.br.Peek(n)
}

// Read drains from the buffered reader first; once the buffer is exhausted
// transitions to the underlying conn. This is the discipline that makes
// the post-listener-filter dispatch path see the same bytes the inspector
// peeked.
func (p *peekerConn) Read(b []byte) (int, error) {
	return p.br.Read(b)
}

// AsPeeker returns the Peeker view of conn if conn was constructed via
// NewPeekerConn; returns nil otherwise. Used by listener filters that need
// to access the peek buffer.
func AsPeeker(conn net.Conn) Peeker {
	if pc, ok := conn.(*peekerConn); ok {
		return pc
	}
	return nil
}
```

- [ ] **Step 6: Write `callbacks_test.go`**

```go
package listenerfilter

import (
	"bytes"
	"io"
	"net"
	"testing"
)

// pipeConn is a net.Conn backed by an in-memory pipe. Useful for unit-
// testing peekerConn without binding sockets.
type pipeConn struct{ net.Conn }

func newPipePair() (client, server net.Conn) {
	client, server = net.Pipe()
	return
}

func TestPeekerConnPeekDoesNotConsume(t *testing.T) {
	cli, srv := newPipePair()
	defer cli.Close()
	defer srv.Close()
	go func() { cli.Write([]byte("HELLO_WORLD")); cli.Close() }()
	pc := NewPeekerConn(srv)
	peeker := AsPeeker(pc)
	if peeker == nil { t.Fatal("AsPeeker returned nil; expected non-nil") }
	first5, err := peeker.Peek(5)
	if err != nil { t.Fatalf("Peek(5): %v", err) }
	if !bytes.Equal(first5, []byte("HELLO")) { t.Errorf("Peek(5)=%q, want %q", first5, "HELLO") }
	// Subsequent Read returns the same bytes (peek didn't consume).
	buf := make([]byte, 11)
	n, err := io.ReadFull(pc, buf)
	if err != nil { t.Fatalf("ReadFull: %v", err) }
	if n != 11 || !bytes.Equal(buf, []byte("HELLO_WORLD")) {
		t.Errorf("ReadFull: got %d bytes %q, want 11 %q", n, buf, "HELLO_WORLD")
	}
}

func TestPeekerConnPeekBeyondBuffer(t *testing.T) {
	cli, srv := newPipePair()
	defer cli.Close()
	defer srv.Close()
	go func() { cli.Write(make([]byte, 5000)); cli.Close() }()
	pc := NewPeekerConnSize(srv, 256)
	peeker := AsPeeker(pc)
	_, err := peeker.Peek(257)
	if err == nil { t.Errorf("Peek(257) on 256-byte buffer; want bufio.ErrBufferFull, got nil") }
}

func TestNewPeekerConnSizeClamps(t *testing.T) {
	cli, srv := newPipePair()
	defer cli.Close()
	defer srv.Close()
	// size=100 clamps to 256.
	pc := NewPeekerConnSize(srv, 100)
	go func() { cli.Write(make([]byte, 256)); cli.Close() }()
	peeker := AsPeeker(pc)
	_, err := peeker.Peek(256)
	if err != nil { t.Errorf("Peek(256) after clamp-to-256; got %v, want nil", err) }
}
```

- [ ] **Step 7: Run tests; confirm they pass**

```bash
go test ./internal/listener/listenerfilter/... -v 2>&1 | head -60
```

Expected: all subtests PASS. Quote the last 30 lines into PROGRESS.

- [ ] **Step 8: Append ADR-0079 to `docs/envoy-go/DECISIONS.md`**

Use the ADR-0079 summary above (full Context / Decision / Consequences). Lands-in-task: 07.2 PLAN Task 2.

- [ ] **Step 9: Append PROGRESS Task 2 entry**

```markdown
## Task 2 — internal/listener/listenerfilter/{doc,types,callbacks}.go [ADR-0079]

**Commits:** TBD — this task's commit
**Notes:** Created internal/listener/listenerfilter/ package with doc.go (~50 LoC), types.go (~120 LoC: ListenerFilter interface, ListenerFilterStatus enum, ChainMatchInputs struct, Peeker interface, ListenerFilterFactory + FilterInstanceFactory factory pattern, FactoryCtx), callbacks.go (~80 LoC: peekerConn concrete + NewPeekerConn + NewPeekerConnSize + AsPeeker), types_test.go + callbacks_test.go covering zero-value semantics, IsLoopbackSource against IPv4 127.0.0.0/8 + IPv6 ::1 + non-loopback / nil, status enum drift guard, peekerConn bytes-not-consumed invariant under interleaved Peek/Read, Peek-beyond-buffer returning bufio.ErrBufferFull, NewPeekerConnSize clamping below 256. Landed ADR-0079 (listener-filter dispatch protocol shape; sync-only; freeze-after-boot registry; 2-step factory pattern; 4096-byte default peeker buffer).
**Outputs:**
\`\`\`
$ go test ./internal/listener/listenerfilter/... -v
<verbatim>
$ go vet ./internal/listener/listenerfilter/...
<verbatim — clean>
\`\`\`
```

- [ ] **Step 10: Commit**

```bash
git add internal/listener/listenerfilter/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: internal/listener/listenerfilter/ types + callbacks [ADR-0079]"
```

SHA-fill follow-up.

*Anchored: SPEC §1 #1, §4.1 (file inventory), §6.1 (filter interface), §6.2 (status enum), §6.3 (Peeker interface), §6.4 (two-step factory), §10 (ADR-0079 anticipation), §15.1 (registry/pipeline/chainmatch/callbacks unit tests).*

---

## Task 3: `internal/listener/listenerfilter/registry.go` — ListenerFilterRegistry Register / Lookup / Freeze

**Files:**
- Create: `internal/listener/listenerfilter/registry.go`
- Create: `internal/listener/listenerfilter/registry_test.go`

This task implements the boot-populated, frozen-after-boot listener-filter registry. Mirrors 07.1's `*filter_http.HTTPRegistry` (ADR-0072) and 06.1's `*stats.Registry` LBP-1 (ADR-0059) — same discipline, narrower API surface (the registry only stores `type_url → factory`, no per-instance metric allocation).

**Precondition:** Task 2 done; `ListenerFilterFactory` type is defined in `types.go`.
**Artifact:** `registry.go` + `registry_test.go`.
**Acceptance:** `go test ./internal/listener/listenerfilter/...` passes; concurrent `Lookup` calls are race-clean under `-race`; post-`Freeze` `Register("X", ...)` panics with `listenerfilter: registry frozen: cannot register %q post-boot`; duplicate `Register("X", ...)` calls panic with `listenerfilter: duplicate factory for %q`; `Freeze()` is idempotent.

- [ ] **Step 1: Write `registry_test.go` failing tests**

```go
package listenerfilter

import (
	"sync"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
)

func dummyFactory(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error) {
	return func() ListenerFilter { return nil }, nil
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewListenerFilterRegistry()
	r.Register("type.googleapis.com/foo", dummyFactory)
	got, ok := r.Lookup("type.googleapis.com/foo")
	if !ok { t.Errorf("Lookup(\"foo\") returned ok=false; want true") }
	if got == nil { t.Errorf("Lookup(\"foo\") returned nil factory") }
	_, missing := r.Lookup("type.googleapis.com/bar")
	if missing { t.Errorf("Lookup(\"bar\") returned ok=true on absent registration") }
}

func TestRegistryDuplicateRegisterPanics(t *testing.T) {
	r := NewListenerFilterRegistry()
	r.Register("type.googleapis.com/foo", dummyFactory)
	defer func() {
		recv := recover()
		if recv == nil { t.Errorf("expected panic on duplicate register; got none") }
	}()
	r.Register("type.googleapis.com/foo", dummyFactory)
}

func TestRegistryFreezeBlocksRegister(t *testing.T) {
	r := NewListenerFilterRegistry()
	r.Register("type.googleapis.com/foo", dummyFactory)
	r.Freeze()
	defer func() {
		recv := recover()
		if recv == nil { t.Errorf("expected panic on post-freeze register; got none") }
	}()
	r.Register("type.googleapis.com/bar", dummyFactory)
}

func TestRegistryFreezeIsIdempotent(t *testing.T) {
	r := NewListenerFilterRegistry()
	r.Freeze()
	r.Freeze() // must not panic
	r.Freeze()
}

func TestRegistryConcurrentLookup(t *testing.T) {
	r := NewListenerFilterRegistry()
	r.Register("type.googleapis.com/foo", dummyFactory)
	r.Freeze()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.Lookup("type.googleapis.com/foo")
		}()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run tests; confirm they fail (Registry symbol does not exist)**

```bash
go test ./internal/listener/listenerfilter/... 2>&1 | head -30
```

Expected: build error.

- [ ] **Step 3: Write `registry.go`**

```go
package listenerfilter

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// ListenerFilterRegistry maps type_url → ListenerFilterFactory. The registry
// is populated once at boot from cmd/envoy-go/main.go (per ADR-0079 +
// ADR-0072 threading discipline mirrored from 07.1), Freeze()'d before the
// listener manager starts accepting, and read concurrently from the
// listener-manager's per-listener constructor at boot.
//
// Post-Freeze Register panics; Lookup remains lock-free (atomic.Bool guard
// + sync.RWMutex RLock for the map read).
type ListenerFilterRegistry struct {
	mu        sync.RWMutex
	byTypeURL map[string]ListenerFilterFactory
	frozen    atomic.Bool
}

// NewListenerFilterRegistry allocates an empty, unfrozen registry.
func NewListenerFilterRegistry() *ListenerFilterRegistry {
	return &ListenerFilterRegistry{byTypeURL: make(map[string]ListenerFilterFactory)}
}

// Register adds f under typeURL. Panics if the registry is frozen, or if
// typeURL is already registered. Boot-time only.
func (r *ListenerFilterRegistry) Register(typeURL string, f ListenerFilterFactory) {
	if r.frozen.Load() {
		panic(fmt.Sprintf("listenerfilter: registry frozen: cannot register %q post-boot", typeURL))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byTypeURL[typeURL]; dup {
		panic(fmt.Sprintf("listenerfilter: duplicate factory for %q", typeURL))
	}
	r.byTypeURL[typeURL] = f
}

// Lookup returns the factory for typeURL plus an ok flag. Safe for
// concurrent use post-Freeze (and pre-Freeze, though typically not called
// pre-Freeze in production).
func (r *ListenerFilterRegistry) Lookup(typeURL string) (ListenerFilterFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.byTypeURL[typeURL]
	return f, ok
}

// Freeze marks the registry as immutable. Subsequent Register calls panic;
// Lookup calls remain valid. Idempotent.
func (r *ListenerFilterRegistry) Freeze() {
	r.frozen.Store(true)
}
```

- [ ] **Step 4: Run tests; confirm they pass**

```bash
go test -race ./internal/listener/listenerfilter/... -v 2>&1 | tail -30
```

Expected: all subtests PASS under `-race`.

- [ ] **Step 5: Append PROGRESS Task 3 entry + Commit**

```bash
git add internal/listener/listenerfilter/registry.go internal/listener/listenerfilter/registry_test.go docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: internal/listener/listenerfilter/registry.go — Register/Lookup/Freeze"
```

SHA-fill follow-up.

*Anchored: SPEC §1 #1, §4.1 (registry.go), §5.6 (concurrency model — Registry.mu Lock pre-Freeze; RLock for Lookup), §6.4 (factory pattern), §15.1 (registry_test.go).*

---

## Task 4: `internal/listener/listenerfilter/pipeline.go` — Per-connection Pipeline with per-pipeline timeout [ADR-0082]

**Files:**
- Create: `internal/listener/listenerfilter/pipeline.go`
- Create: `internal/listener/listenerfilter/pipeline_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0082)

This task implements the per-connection sequential dispatch pipeline. The `Pipeline.Run` method iterates a slice of `ListenerFilter` instances, calling each filter's `Inspect` synchronously; on `Continue` advances to the next filter; on `StopIteration` halts the loop; on non-nil error aborts. A single per-pipeline `context.WithTimeout(ctx, timeoutMs * time.Millisecond)` is established once before the loop and shared across all filters' `Inspect` calls (per Decision N + ADR-0082 — NOT per-filter time-slicing). On context-deadline-exceeded the function returns a wrapped error; the listener manager handles the error per the proto's `continue_on_listener_filters_timeout` field (Task 9 step 6 wires that).

**Precondition:** Tasks 2 + 3 done; `ListenerFilter`, `ChainMatchInputs`, `Peeker` are defined.
**Artifact:** `pipeline.go` + `pipeline_test.go`; ADR-0082 in DECISIONS.md.
**Acceptance:** `go test -race ./internal/listener/listenerfilter/...` passes; the multi-filter test exercises both `Continue` and `StopIteration` paths; the timeout test demonstrates a slow filter can eat subsequent filters' budget (per-pipeline shared deadline); the zero-filters case returns nil immediately; a 0-`timeoutMs` argument is treated as "no timeout enforcement" (no `context.WithTimeout` wrapping); `OnDestroy()` is called on every constructed filter instance after pipeline completion (whether via Continue, StopIteration, or error).

- [ ] **Step 1: Write `pipeline_test.go` failing tests**

```go
package listenerfilter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

// stubFilter is a test-only implementation of ListenerFilter whose
// behavior is fully programmable per test case.
type stubFilter struct {
	onInspect func(ctx context.Context, peeker Peeker, inputs *ChainMatchInputs) (ListenerFilterStatus, error)
	destroyed bool
}

func (s *stubFilter) Inspect(ctx context.Context, peeker Peeker, inputs *ChainMatchInputs) (ListenerFilterStatus, error) {
	return s.onInspect(ctx, peeker, inputs)
}
func (s *stubFilter) OnDestroy() { s.destroyed = true }

func TestPipelineRunZeroFilters(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	p := &Pipeline{}
	if err := p.Run(context.Background(), nil, pc, inputs, 1000); err != nil {
		t.Errorf("Run(nil filters): got %v, want nil", err)
	}
}

func TestPipelineRunContinuePath(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	f1 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, in *ChainMatchInputs) (ListenerFilterStatus, error) {
		in.ServerName = "f1"
		return Continue, nil
	}}
	f2 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, in *ChainMatchInputs) (ListenerFilterStatus, error) {
		in.TransportProtocol = "f2"
		return Continue, nil
	}}
	p := &Pipeline{}
	err := p.Run(context.Background(), []ListenerFilter{f1, f2}, pc, inputs, 1000)
	if err != nil { t.Fatalf("Run: %v", err) }
	if inputs.ServerName != "f1" || inputs.TransportProtocol != "f2" {
		t.Errorf("inputs not populated by both filters; got %+v", inputs)
	}
	if !f1.destroyed || !f2.destroyed { t.Errorf("OnDestroy not called: f1=%v f2=%v", f1.destroyed, f2.destroyed) }
}

func TestPipelineRunStopIterationPath(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	f1 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, in *ChainMatchInputs) (ListenerFilterStatus, error) {
		in.ServerName = "f1"
		return StopIteration, nil
	}}
	f2Fired := false
	f2 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, in *ChainMatchInputs) (ListenerFilterStatus, error) {
		f2Fired = true
		return Continue, nil
	}}
	p := &Pipeline{}
	if err := p.Run(context.Background(), []ListenerFilter{f1, f2}, pc, inputs, 1000); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f2Fired { t.Errorf("f2 fired despite f1 returning StopIteration") }
	if inputs.ServerName != "f1" { t.Errorf("ServerName=%q, want \"f1\"", inputs.ServerName) }
	if !f1.destroyed || !f2.destroyed { t.Errorf("OnDestroy not called: f1=%v f2=%v", f1.destroyed, f2.destroyed) }
}

func TestPipelineRunFilterError(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	want := errors.New("inspect failure")
	f := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
		return Continue, want
	}}
	p := &Pipeline{}
	err := p.Run(context.Background(), []ListenerFilter{f}, pc, inputs, 1000)
	if err == nil || !errors.Is(err, want) {
		t.Errorf("Run: got %v, want errors.Is %v", err, want)
	}
}

func TestPipelineRunTimeoutSharedAcrossFilters(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	// f1 sleeps for 50ms; f2 will see ctx already expired since the per-
	// pipeline budget is 30ms.
	f1 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
		select { case <-time.After(50 * time.Millisecond): case <-ctx.Done(): }
		return Continue, nil
	}}
	f2Fired := false
	f2 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
		f2Fired = true
		return Continue, nil
	}}
	p := &Pipeline{}
	err := p.Run(context.Background(), []ListenerFilter{f1, f2}, pc, inputs, 30) // 30ms budget
	if err == nil { t.Errorf("Run: got nil, want timeout error") }
	if f2Fired { t.Errorf("f2 fired despite per-pipeline timeout exhausted by f1") }
}

func TestPipelineRunZeroTimeoutDisablesEnforcement(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	f := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
		// Sleeps for longer than any reasonable budget would allow; with
		// timeoutMs=0 the pipeline does not enforce.
		time.Sleep(10 * time.Millisecond)
		return Continue, nil
	}}
	p := &Pipeline{}
	if err := p.Run(context.Background(), []ListenerFilter{f}, pc, inputs, 0); err != nil {
		t.Errorf("Run with timeoutMs=0: %v, want nil", err)
	}
}

func TestPipelineRunPropagatesError(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	f := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
		return Continue, fmt.Errorf("filter-specific")
	}}
	p := &Pipeline{}
	err := p.Run(context.Background(), []ListenerFilter{f}, pc, inputs, 1000)
	if err == nil { t.Errorf("Run: got nil, want filter-specific error") }
}
```

- [ ] **Step 2: Run tests; confirm they fail (Pipeline symbol does not exist)**

```bash
go test ./internal/listener/listenerfilter/... 2>&1 | head -30
```

Expected: build error.

- [ ] **Step 3: Write `pipeline.go`**

```go
package listenerfilter

import (
	"context"
	"fmt"
	"time"
)

// Pipeline drives the per-connection listener-filter sequential dispatch.
// Allocated by the listener manager's accept-loop on each accepted
// connection. Owns nothing per-instance — Run is a pure function over
// (filters, peeker, inputs, timeoutMs); the struct exists for future
// per-pipeline state (e.g., metrics counters) without breaking the API.
type Pipeline struct{}

// Run iterates filters sequentially, calling each filter's Inspect with the
// shared (ctx, peeker, inputs) trio. Behavior:
//   - 0 filters: returns nil immediately.
//   - timeoutMs == 0: no per-pipeline deadline established (the caller's
//     ctx is passed through as-is).
//   - timeoutMs > 0: a single context.WithTimeout(ctx, timeoutMs *
//     time.Millisecond) wraps the loop; the per-filter Inspect calls share
//     the deadline (ADR-0082 + Decision N — per-pipeline NOT per-filter).
//   - On Continue: advances to the next filter (or finishes if last).
//   - On StopIteration: halts the loop; remaining filters are skipped.
//   - On non-nil error: aborts; the error is wrapped with the filter index.
//   - On context-deadline-exceeded after a filter's Inspect returns: the
//     pipeline returns a wrapped timeout error.
//   - OnDestroy is called on every filter (in declaration order) after the
//     loop ends, regardless of how the loop exited (Continue/StopIteration/
//     error/timeout).
func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Peeker, inputs *ChainMatchInputs, timeoutMs uint32) (retErr error) {
	defer func() {
		for _, f := range filters {
			f.OnDestroy()
		}
	}()
	if len(filters) == 0 {
		return nil
	}
	if timeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}
	for i, f := range filters {
		status, err := f.Inspect(ctx, peeker, inputs)
		if err != nil {
			return fmt.Errorf("listener-filter[%d]: %w", i, err)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("listener-filter[%d]: pipeline timeout: %w", i, ctx.Err())
		}
		if status == StopIteration {
			return nil
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests; confirm they pass under `-race`**

```bash
go test -race ./internal/listener/listenerfilter/... -v 2>&1 | tail -30
```

Expected: all PASS. Quote the last 30 lines into PROGRESS.

- [ ] **Step 5: Append ADR-0082 to `docs/envoy-go/DECISIONS.md`**

Use the ADR-0082 summary above (full Context/Decision/Consequences). Lands-in-task: 07.2 PLAN Task 4.

- [ ] **Step 6: Append PROGRESS Task 4 entry + Commit**

```bash
git add internal/listener/listenerfilter/pipeline.go internal/listener/listenerfilter/pipeline_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: internal/listener/listenerfilter/pipeline.go — sequential dispatch + per-pipeline timeout [ADR-0082]"
```

SHA-fill follow-up.

*Anchored: SPEC §1 #7, §4.1 (pipeline.go), §5.4 (Pipeline-deadline mechanism), §5.6 (concurrency — single-goroutine-per-connection drives the pipeline; lock-free), §6.5 (Pipeline deadline default + envelope), §10 (ADR-0082 anticipation), §15.1 (pipeline_test.go).*

---

## Task 5: `internal/listener/listenerfilter/chainmatch.go` — 8-dimension SelectChain algorithm [ADR-0080, ADR-0081]

**Files:**
- Create: `internal/listener/listenerfilter/chainmatch.go`
- Create: `internal/listener/listenerfilter/chainmatch_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0080 + ADR-0081)

This task implements the heart of phase 07.2: the 8-dimension chain-match precedence algorithm. `SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error)` runs the 2-pass eligibility-then-specificity algorithm per SPEC §5.5 + §7.3. ADR-0080 (default_filter_chain semantics) + ADR-0081 (8-dimension precedence algorithm) both land in this commit per SPEC §10's anchoring.

**Precondition:** Tasks 2 + 3 + 4 done; the package's `ChainMatchInputs` is defined.
**Artifact:** `chainmatch.go` + `chainmatch_test.go`; ADR-0080 + ADR-0081 in DECISIONS.md.
**Acceptance:** `go test -race ./internal/listener/listenerfilter/...` passes; the unit-test matrix covers each priority dimension in isolation (SPEC §11.3 pin: `destination_port` BEATS `source_prefix_ranges`); empty-match chain in `filter_chains[]` BEATS `default_filter_chain` (§11.2 pin); `default_filter_chain` consulted only when no chain eligible (§11.1 pin); `prefix_ranges` longer-prefix tie-breaker (`192.168.1.0/24` BEATS `192.168.0.0/16`); `server_names` SNI-specificity tie-breaker (exact > suffix > universal > catch-all per ADR-0033 clause 9 preserved as special case); identical chains (same on all 8 dimensions) → returns `ErrAmbiguousChainMatch` so the manager can detect at NewManager-build time and fail loudly; no eligible chain + no default → returns `ErrNoChainMatched`.

- [ ] **Step 1: Write `chainmatch_test.go` failing tests**

```go
package listenerfilter

import (
	"errors"
	"net"
	"testing"
)

func cidr(s string) *net.IPNet { _, n, _ := net.ParseCIDR(s); return n }

func TestSelectChainEmptyMatchUniversallyEligible(t *testing.T) {
	cs := &ChainSpec{Name: "catchall", Empty: true}
	inputs := ChainMatchInputs{DestinationPort: 8080, SourceIP: net.ParseIP("10.0.0.1")}
	got, err := SelectChain(inputs, []*ChainSpec{cs}, nil)
	if err != nil { t.Fatalf("SelectChain: %v", err) }
	if got != cs { t.Errorf("got %v, want %v", got, cs) }
}

func TestSelectChainDestinationPortBeatsSourcePrefix(t *testing.T) {
	dstport := &ChainSpec{Name: "dstport", DestinationPort: 8080}
	srcprefix := &ChainSpec{Name: "srcprefix", SourcePrefixRanges: []*net.IPNet{cidr("127.0.0.0/8")}}
	inputs := ChainMatchInputs{DestinationPort: 8080, SourceIP: net.ParseIP("127.0.0.1")}
	got, err := SelectChain(inputs, []*ChainSpec{dstport, srcprefix}, nil)
	if err != nil { t.Fatalf("SelectChain: %v", err) }
	if got != dstport { t.Errorf("expected dstport (priority slot 0); got %v", got) }
}

func TestSelectChainDefaultChainOnNoMatch(t *testing.T) {
	specific := &ChainSpec{Name: "loopback", SourcePrefixRanges: []*net.IPNet{cidr("127.0.0.0/8")}}
	def := &ChainSpec{Name: "default"}
	inputs := ChainMatchInputs{SourceIP: net.ParseIP("10.0.0.1")} // not in 127.0.0.0/8
	got, err := SelectChain(inputs, []*ChainSpec{specific}, def)
	if err != nil { t.Fatalf("SelectChain: %v", err) }
	if got != def { t.Errorf("expected default chain; got %v", got) }
}

func TestSelectChainEmptyMatchBeatsDefault(t *testing.T) {
	emptyMatch := &ChainSpec{Name: "empty", Empty: true}
	def := &ChainSpec{Name: "default"}
	inputs := ChainMatchInputs{SourceIP: net.ParseIP("10.0.0.1")}
	got, err := SelectChain(inputs, []*ChainSpec{emptyMatch}, def)
	if err != nil { t.Fatalf("SelectChain: %v", err) }
	if got != emptyMatch { t.Errorf("empty-match chain should beat default; got %v", got) }
}

func TestSelectChainNoEligibleNoDefault(t *testing.T) {
	specific := &ChainSpec{Name: "loopback", SourcePrefixRanges: []*net.IPNet{cidr("127.0.0.0/8")}}
	inputs := ChainMatchInputs{SourceIP: net.ParseIP("10.0.0.1")}
	got, err := SelectChain(inputs, []*ChainSpec{specific}, nil)
	if !errors.Is(err, ErrNoChainMatched) {
		t.Errorf("got (%v, %v), want (nil, ErrNoChainMatched)", got, err)
	}
	if got != nil { t.Errorf("got chain=%v on no-match, want nil", got) }
}

func TestSelectChainPrefixRangesLongerWins(t *testing.T) {
	a := &ChainSpec{Name: "a", PrefixRanges: []*net.IPNet{cidr("192.168.0.0/16")}}
	b := &ChainSpec{Name: "b", PrefixRanges: []*net.IPNet{cidr("192.168.1.0/24")}}
	inputs := ChainMatchInputs{DestinationIP: net.ParseIP("192.168.1.50")}
	got, err := SelectChain(inputs, []*ChainSpec{a, b}, nil)
	if err != nil { t.Fatalf("SelectChain: %v", err) }
	if got != b { t.Errorf("expected longer-prefix chain b (/24); got %v", got) }
}

func TestSelectChainServerNamesSpecificity(t *testing.T) {
	exact := &ChainSpec{Name: "exact", ServerNames: []string{"foo.example.test"}}
	suffix := &ChainSpec{Name: "suffix", ServerNames: []string{"*.example.test"}}
	universal := &ChainSpec{Name: "universal", ServerNames: []string{"*"}}
	inputs := ChainMatchInputs{ServerName: "foo.example.test"}
	// All three eligible; exact wins.
	got, err := SelectChain(inputs, []*ChainSpec{universal, suffix, exact}, nil)
	if err != nil { t.Fatalf("SelectChain: %v", err) }
	if got != exact { t.Errorf("expected exact-match SNI chain; got %v", got) }
}

func TestSelectChainSourceTypeLocal(t *testing.T) {
	local := &ChainSpec{Name: "local", SourceTypeLocal: true}
	universal := &ChainSpec{Name: "u", Empty: true}
	inputs := ChainMatchInputs{SourceIP: net.ParseIP("127.0.0.1")}
	got, err := SelectChain(inputs, []*ChainSpec{local, universal}, nil)
	if err != nil { t.Fatalf("SelectChain: %v", err) }
	if got != local { t.Errorf("expected source_type:LOCAL chain; got %v", got) }
}

func TestSelectChainSourceTypeExternalSkipsLoopback(t *testing.T) {
	ext := &ChainSpec{Name: "ext", SourceTypeExternal: true}
	universal := &ChainSpec{Name: "u", Empty: true}
	inputs := ChainMatchInputs{SourceIP: net.ParseIP("127.0.0.1")} // loopback
	got, err := SelectChain(inputs, []*ChainSpec{ext, universal}, nil)
	if err != nil { t.Fatalf("SelectChain: %v", err) }
	// ext is eliminated (loopback ≠ external); universal wins.
	if got != universal { t.Errorf("expected universal chain (ext eliminated); got %v", got) }
}

func TestSelectChainApplicationProtocolsTieBreaker(t *testing.T) {
	h2 := &ChainSpec{Name: "h2", ApplicationProtocols: []string{"h2"}}
	h1 := &ChainSpec{Name: "h1", ApplicationProtocols: []string{"http/1.1"}}
	inputs := ChainMatchInputs{ApplicationProtocols: []string{"h2"}}
	got, err := SelectChain(inputs, []*ChainSpec{h1, h2}, nil)
	if err != nil { t.Fatalf("SelectChain: %v", err) }
	if got != h2 { t.Errorf("expected h2 chain (ALPN match); got %v", got) }
}
```

- [ ] **Step 2: Run tests; confirm they fail (ChainSpec / SelectChain do not exist)**

- [ ] **Step 3: Write `chainmatch.go`**

```go
package listenerfilter

import (
	"errors"
	"net"
	"strings"
)

// ChainSpec is the parsed match-dimension shape for one filter_chain (or
// the default_filter_chain). Constructed at NewManager-build time from the
// proto's FilterChainMatch message; immutable thereafter.
type ChainSpec struct {
	Name string
	// Empty: true means filter_chain_match is absent or all-zero — the
	// chain is universally eligible (catch-all). When true, every other
	// field is ignored.
	Empty bool
	// DestinationPort: 0 means unspecified; non-zero means the chain
	// requires the connection's local port to equal this value.
	DestinationPort uint32
	// PrefixRanges: empty means unspecified; non-empty means the chain
	// requires conn.LocalAddr().IP to fall in at least one CIDR.
	PrefixRanges []*net.IPNet
	// ServerNames: empty means unspecified; non-empty means SNI must match
	// at least one entry per chainSpecificityRank semantics (exact > suffix
	// > universal > catch-all). Re-uses the existing
	// internal/listener/manager.go:chainSpecificityRank logic at the
	// tie-breaker level.
	ServerNames []string
	// TransportProtocol: "" means unspecified; "tls" or "raw_buffer" means
	// the chain requires the listener-filter pipeline to have set
	// inputs.TransportProtocol to the matching value.
	TransportProtocol string
	// ApplicationProtocols: empty means unspecified; non-empty means
	// inputs.ApplicationProtocols must contain at least one matching entry.
	ApplicationProtocols []string
	// SourceTypeLocal / SourceTypeExternal: at most one true; true means
	// the chain requires conn.RemoteAddr().IP to be loopback / non-loopback
	// respectively. Both false means source_type unspecified (ANY).
	SourceTypeLocal    bool
	SourceTypeExternal bool
	// SourcePrefixRanges: empty means unspecified; non-empty means
	// conn.RemoteAddr().IP must fall in at least one CIDR.
	SourcePrefixRanges []*net.IPNet
	// SourcePorts: empty means unspecified; non-empty means
	// conn.RemoteAddr().Port must equal at least one entry.
	SourcePorts []uint32
}

// ErrNoChainMatched is returned by SelectChain when no chain in chains is
// eligible AND defaultChain is nil.
var ErrNoChainMatched = errors.New("no filter_chain matches connection")

// ErrAmbiguousChainMatch is returned by SelectChain when two chains have
// identical specificity vectors AND identical sub-ordering values for the
// highest-priority specific dimension. The listener manager detects this at
// NewManager-build time (it pre-runs SelectChain on a sample input or
// duplicate-matches the chain specs structurally) and rejects the bootstrap.
var ErrAmbiguousChainMatch = errors.New("ambiguous filter_chain selection")

// priorityOrder is the 8-dimension specificity priority vector per SPEC
// §5.5 and the upstream filter_chain_match.proto comments. Index 0 = most
// specific dimension.
const (
	prioDestinationPort     = 0
	prioPrefixRanges        = 1
	prioServerNames         = 2
	prioTransportProtocol   = 3
	prioApplicationProtocols = 4
	prioSourceType          = 5
	prioSourcePrefixRanges  = 6
	prioSourcePorts         = 7
	prioCount               = 8
)

// SelectChain runs the 2-pass eligibility-then-specificity algorithm per
// SPEC §5.5 + §7.3. Returns the winning chain, or defaultChain if no chain
// in chains is eligible, or (nil, ErrNoChainMatched) if neither path yields
// a result.
func SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error) {
	// Pass 1: eligibility.
	eligible := make([]*ChainSpec, 0, len(chains))
	for _, c := range chains {
		if matches(c, &inputs) {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		if defaultChain != nil {
			return defaultChain, nil
		}
		return nil, ErrNoChainMatched
	}
	// Pass 2: specificity scoring.
	best := eligible[0]
	bestScore := specificityScore(best)
	for _, c := range eligible[1:] {
		cScore := specificityScore(c)
		switch {
		case cScore > bestScore:
			best = c
			bestScore = cScore
		case cScore == bestScore:
			best = breakTie(best, c, &inputs)
			if best == nil {
				return nil, ErrAmbiguousChainMatch
			}
		}
	}
	return best, nil
}

// matches reports whether every non-zero dimension of c is satisfied by
// inputs. Empty-match chains return true unconditionally.
func matches(c *ChainSpec, inputs *ChainMatchInputs) bool {
	if c.Empty {
		return true
	}
	if c.DestinationPort != 0 && c.DestinationPort != inputs.DestinationPort {
		return false
	}
	if len(c.PrefixRanges) > 0 && !ipInAny(inputs.DestinationIP, c.PrefixRanges) {
		return false
	}
	if len(c.ServerNames) > 0 && !sniMatchAny(c.ServerNames, inputs.ServerName) {
		return false
	}
	if c.TransportProtocol != "" && c.TransportProtocol != inputs.TransportProtocol {
		return false
	}
	if len(c.ApplicationProtocols) > 0 && !alpnMatchAny(c.ApplicationProtocols, inputs.ApplicationProtocols) {
		return false
	}
	if c.SourceTypeLocal && !inputs.IsLoopbackSource() {
		return false
	}
	if c.SourceTypeExternal && inputs.IsLoopbackSource() {
		return false
	}
	if len(c.SourcePrefixRanges) > 0 && !ipInAny(inputs.SourceIP, c.SourcePrefixRanges) {
		return false
	}
	if len(c.SourcePorts) > 0 && !portInAny(inputs.SourcePort, c.SourcePorts) {
		return false
	}
	return true
}

// specificityScore returns an 8-bit integer where bit prioCount-1-i is set
// iff priority slot i is specified (specific) on c. Higher score = more
// specific. Bit ordering puts the most-significant-bit on the highest-
// priority dimension so a numerical compare reflects the priority order.
func specificityScore(c *ChainSpec) uint8 {
	if c.Empty {
		return 0
	}
	var s uint8
	if c.DestinationPort != 0 {
		s |= 1 << (prioCount - 1 - prioDestinationPort)
	}
	if len(c.PrefixRanges) > 0 {
		s |= 1 << (prioCount - 1 - prioPrefixRanges)
	}
	if len(c.ServerNames) > 0 {
		s |= 1 << (prioCount - 1 - prioServerNames)
	}
	if c.TransportProtocol != "" {
		s |= 1 << (prioCount - 1 - prioTransportProtocol)
	}
	if len(c.ApplicationProtocols) > 0 {
		s |= 1 << (prioCount - 1 - prioApplicationProtocols)
	}
	if c.SourceTypeLocal || c.SourceTypeExternal {
		s |= 1 << (prioCount - 1 - prioSourceType)
	}
	if len(c.SourcePrefixRanges) > 0 {
		s |= 1 << (prioCount - 1 - prioSourcePrefixRanges)
	}
	if len(c.SourcePorts) > 0 {
		s |= 1 << (prioCount - 1 - prioSourcePorts)
	}
	return s
}

// breakTie compares a vs b on the per-dimension finer-grain criteria when
// their specificity vectors are identical. Returns the winner; returns nil
// if a and b are entirely indistinguishable (a NewManager-time config error
// the listener manager surfaces as ErrAmbiguousChainMatch).
func breakTie(a, b *ChainSpec, inputs *ChainMatchInputs) *ChainSpec {
	// PrefixRanges: longer prefix wins (smaller IPNet).
	if len(a.PrefixRanges) > 0 && len(b.PrefixRanges) > 0 {
		la := longestPrefix(inputs.DestinationIP, a.PrefixRanges)
		lb := longestPrefix(inputs.DestinationIP, b.PrefixRanges)
		if la > lb { return a }
		if lb > la { return b }
	}
	// SourcePrefixRanges: longer prefix wins.
	if len(a.SourcePrefixRanges) > 0 && len(b.SourcePrefixRanges) > 0 {
		la := longestPrefix(inputs.SourceIP, a.SourcePrefixRanges)
		lb := longestPrefix(inputs.SourceIP, b.SourcePrefixRanges)
		if la > lb { return a }
		if lb > la { return b }
	}
	// ServerNames: SNI specificity (exact > suffix > universal > catch-all).
	if len(a.ServerNames) > 0 && len(b.ServerNames) > 0 {
		ra := sniSpecificityRank(a.ServerNames)
		rb := sniSpecificityRank(b.ServerNames)
		if ra < rb { return a } // lower rank = more specific
		if rb < ra { return b }
	}
	// All other dimensions are exact-value match — no sub-ordering. If we
	// reach here, the two chains are indistinguishable on this input.
	return nil
}

// ipInAny reports whether ip falls in at least one of the CIDRs.
func ipInAny(ip net.IP, cidrs []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}

// portInAny reports whether p equals at least one of the ports.
func portInAny(p uint32, ports []uint32) bool {
	for _, q := range ports {
		if p == q {
			return true
		}
	}
	return false
}

// longestPrefix returns the longest prefix-length CIDR in cidrs that
// contains ip; 0 if none.
func longestPrefix(ip net.IP, cidrs []*net.IPNet) int {
	best := 0
	for _, c := range cidrs {
		if c.Contains(ip) {
			ones, _ := c.Mask.Size()
			if ones > best {
				best = ones
			}
		}
	}
	return best
}

// sniMatchAny reports whether sni matches any of the patterns (exact OR
// suffix-wildcard OR universal-wildcard OR catch-all).
func sniMatchAny(patterns []string, sni string) bool {
	for _, p := range patterns {
		if p == "*" || p == sni {
			return true
		}
		if strings.HasPrefix(p, "*.") && strings.HasSuffix(sni, p[1:]) {
			return true
		}
	}
	return false
}

// alpnMatchAny reports whether at least one element of want is in offered.
func alpnMatchAny(want, offered []string) bool {
	for _, w := range want {
		for _, o := range offered {
			if w == o {
				return true
			}
		}
	}
	return false
}

// sniSpecificityRank mirrors internal/listener/manager.go:chainSpecificityRank
// (preserved from phase 03 per ADR-0033 clause 9 → ADR-0078). Lower rank =
// more specific. Used as the SNI sub-ordering tie-breaker WITHIN the
// server_names priority slot per SPEC §5.5.
//
//	0: any non-wildcard pattern
//	1: any suffix-wildcard ("*.foo.test")
//	2: universal-wildcard ("*")
//	3: catch-all (empty patterns slice — unused here since
//	   matches() rejects this case before breakTie sees it)
func sniSpecificityRank(patterns []string) int {
	if len(patterns) == 0 {
		return 3
	}
	rank := 4
	for _, p := range patterns {
		switch {
		case p == "*":
			if 2 < rank { rank = 2 }
		case strings.HasPrefix(p, "*."):
			if 1 < rank { rank = 1 }
		default:
			return 0
		}
	}
	return rank
}
```

- [ ] **Step 4: Run tests; confirm they pass under `-race`**

```bash
go test -race ./internal/listener/listenerfilter/... -v 2>&1 | tail -40
```

Expected: all subtests PASS. Quote the last 30 lines into PROGRESS.

- [ ] **Step 5: Append ADR-0080 + ADR-0081 to `docs/envoy-go/DECISIONS.md`**

Use the ADR-0080 + ADR-0081 summaries above (full Context/Decision/Consequences). Both ADRs lands-in-task: 07.2 PLAN Task 5.

- [ ] **Step 6: Append PROGRESS Task 5 entry + Commit**

```bash
git add internal/listener/listenerfilter/chainmatch.go internal/listener/listenerfilter/chainmatch_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: internal/listener/listenerfilter/chainmatch.go — 8-dim SelectChain [ADR-0080, ADR-0081]"
```

SHA-fill follow-up.

*Anchored: SPEC §1 #5, §1 #6, §4.1 (chainmatch.go), §5.5 (precedence algorithm), §5.7 (ADR-0033 supersession enumeration — clause 9 preserved as SNI tie-breaker), §7.1–§7.3 (eight dimensions + algorithm pseudocode), §8 (default_filter_chain semantics), §10 (ADR-0080 + ADR-0081 anticipations), §11.2 + §11.3 (empirical pins driving the algorithm), §15.1 (chainmatch_test.go).*

---

## Task 6: `internal/listener/listenerfilter/fuzz_test.go` — `FuzzFilterChainMatch` (10th fuzzer)

**Files:**
- Create: `internal/listener/listenerfilter/fuzz_test.go`

This task introduces the new fuzzer per SPEC §15.6. Asserts: (i) `SelectChain` never panics; (ii) returned chain is one of the input chains OR `defaultChain` OR nil with `ErrNoChainMatched` / `ErrAmbiguousChainMatch`; (iii) returned chain's match dimensions are all satisfied by the inputs (re-runs `matches`); (iv) deterministic on identical inputs (running twice yields the same result).

**Precondition:** Task 5 done; `SelectChain` + `ChainSpec` + `ChainMatchInputs` are defined.
**Artifact:** `fuzz_test.go`.
**Acceptance:** `go test -fuzz=FuzzFilterChainMatch -fuzztime=30s ./internal/listener/listenerfilter/` runs clean (no panics, no assertion failures) at the ADR-0018 short-budget. `go test -count=1 ./internal/listener/listenerfilter/...` passes the seed corpus.

- [ ] **Step 1: Write `fuzz_test.go`**

```go
package listenerfilter

import (
	"errors"
	"net"
	"testing"
)

// FuzzFilterChainMatch fuzzes adversarial ChainMatchInputs corners +
// adversarial chain-spec lists into SelectChain. Per SPEC §15.6.
func FuzzFilterChainMatch(f *testing.F) {
	// Seed corpus: SPEC §11.1, §11.2, §11.3 pin shapes.
	f.Add(uint32(8080), uint32(0), "127.0.0.1", "")           // §11.3-shape inputs
	f.Add(uint32(0), uint32(54321), "10.0.0.1", "foo.test") // non-loopback inputs
	f.Add(uint32(443), uint32(0), "::1", "")                  // IPv6 loopback
	f.Add(uint32(80), uint32(12345), "192.168.1.1", "*")    // wildcard SNI
	f.Fuzz(func(t *testing.T, dstPort, srcPort uint32, srcIPStr, sni string) {
		ip := net.ParseIP(srcIPStr)
		inputs := ChainMatchInputs{
			DestinationIP:   net.ParseIP("0.0.0.0"),
			DestinationPort: dstPort,
			SourceIP:        ip,
			SourcePort:      srcPort,
			ServerName:      sni,
		}
		// Build a varied chain set covering the 8 priority dimensions.
		chains := []*ChainSpec{
			{Name: "a", DestinationPort: 8080},
			{Name: "b", SourcePrefixRanges: []*net.IPNet{mustCIDR("127.0.0.0/8")}},
			{Name: "c", ServerNames: []string{"foo.test", "*.bar.test"}},
			{Name: "d", Empty: true},
		}
		def := &ChainSpec{Name: "default"}

		// Assertion (i): never panic.
		got, err := SelectChain(inputs, chains, def)
		// Assertion (ii): result is one of input chains OR default OR
		// (nil, ErrNoChainMatched / ErrAmbiguousChainMatch).
		if err != nil {
			if !errors.Is(err, ErrNoChainMatched) && !errors.Is(err, ErrAmbiguousChainMatch) {
				t.Errorf("unexpected error type: %v", err)
			}
			if got != nil {
				t.Errorf("err non-nil but chain non-nil: %v / %v", err, got)
			}
			return
		}
		valid := got == def
		for _, c := range chains {
			if got == c { valid = true }
		}
		if !valid {
			t.Errorf("returned chain not in input set or default: %v", got)
		}
		// Assertion (iii): returned chain's match dimensions all satisfied.
		if got != def && !matches(got, &inputs) {
			t.Errorf("returned chain %v does not match inputs %+v", got, inputs)
		}
		// Assertion (iv): deterministic.
		got2, err2 := SelectChain(inputs, chains, def)
		if got != got2 || (err == nil) != (err2 == nil) {
			t.Errorf("non-deterministic: first=(%v,%v) second=(%v,%v)", got, err, got2, err2)
		}
	})
}

func mustCIDR(s string) *net.IPNet { _, n, err := net.ParseCIDR(s); if err != nil { panic(err) }; return n }
```

- [ ] **Step 2: Run the fuzzer at the 30s ADR-0018 budget**

```bash
go test -fuzz=FuzzFilterChainMatch -fuzztime=30s ./internal/listener/listenerfilter/ 2>&1 | tail -10
```

Expected: clean (no panics, no assertion failures). If the fuzzer surfaces a counterexample, fix the bug + add the regression case to the seed corpus.

- [ ] **Step 3: Append PROGRESS Task 6 entry + Commit**

```bash
git add internal/listener/listenerfilter/fuzz_test.go docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: internal/listener/listenerfilter/fuzz_test.go — FuzzFilterChainMatch (10th fuzzer)"
```

SHA-fill follow-up.

*Anchored: SPEC §15.6 (fuzzer assertions + input space), §3 (gate (d) — total 10 fuzzers post-07.2).*

---

## Task 7: `internal/listener/listenerfilter/tls_inspector/parser.go` — minimal ClientHello parser

**Files:**
- Create: `internal/listener/listenerfilter/tls_inspector/parser.go`
- Create: `internal/listener/listenerfilter/tls_inspector/parser_test.go`

This task introduces the hand-rolled minimal ClientHello parser. Pure function; no I/O. Adapted from `crypto/tls/handshake_messages.go:unmarshal` for the ClientHello message type; narrowed to extract only `server_name` (extension type 0) + `application_layer_protocol_negotiation` (extension type 16). Does NOT pull in the upstream Envoy C++ `tls_inspector` implementation (D-3.2 forbids cgo binding). Returns `(sni string, alpns []string, ok bool)` where `ok=false` indicates the buffer is not a valid ClientHello (the caller in `tls_inspector.go` treats this as "non-TLS preamble" and returns `Continue` with `inputs.TransportProtocol = "raw_buffer"`).

**Precondition:** Tasks 2-6 done.
**Artifact:** `parser.go` + `parser_test.go`.
**Acceptance:** `go test -race ./internal/listener/listenerfilter/tls_inspector/...` passes; the parser handles full ClientHello with both extensions; SNI-only; ALPN-only; no extensions; truncated ClientHello (returns `ok=false`); malformed length prefix (returns `ok=false` without panic); ALPN with multiple protocols; SNI with multiple hostnames per the upstream `ServerNameList` proto (uses only the first per Envoy convention).

- [ ] **Step 1: Write `parser_test.go` failing tests**

```go
package tls_inspector

import (
	"crypto/tls"
	"net"
	"testing"
)

// captureClientHello uses a real TLS handshake against a pipe to capture
// the ClientHello bytes. The resulting buffer is the verbatim handshake
// record envoy-go's parser must accept.
func captureClientHello(t *testing.T, sni string, alpns []string) []byte {
	t.Helper()
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	// Read the first record on the server side.
	recv := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := srv.Read(buf)
		recv <- buf[:n]
	}()
	go func() {
		c := tls.Client(cli, &tls.Config{ServerName: sni, NextProtos: alpns, InsecureSkipVerify: true})
		_ = c.Handshake() // expected to fail (server doesn't respond); we only need ClientHello bytes
	}()
	return <-recv
}

func TestParseClientHelloWithSNIAndALPN(t *testing.T) {
	buf := captureClientHello(t, "foo.example.test", []string{"h2", "http/1.1"})
	sni, alpns, ok := parseClientHello(buf)
	if !ok { t.Fatalf("parseClientHello: ok=false on real ClientHello") }
	if sni != "foo.example.test" { t.Errorf("SNI: got %q, want \"foo.example.test\"", sni) }
	if len(alpns) != 2 || alpns[0] != "h2" || alpns[1] != "http/1.1" {
		t.Errorf("ALPN: got %v, want [h2, http/1.1]", alpns)
	}
}

func TestParseClientHelloSNIOnly(t *testing.T) {
	buf := captureClientHello(t, "foo.example.test", nil)
	sni, alpns, ok := parseClientHello(buf)
	if !ok { t.Fatalf("parseClientHello: ok=false on SNI-only ClientHello") }
	if sni != "foo.example.test" { t.Errorf("SNI: got %q", sni) }
	if len(alpns) != 0 { t.Errorf("ALPN: got %v, want empty", alpns) }
}

func TestParseClientHelloALPNOnly(t *testing.T) {
	buf := captureClientHello(t, "", []string{"h2"})
	sni, alpns, ok := parseClientHello(buf)
	if !ok { t.Fatalf("parseClientHello: ok=false on ALPN-only ClientHello") }
	if sni != "" { t.Errorf("SNI: got %q, want \"\"", sni) }
	if len(alpns) != 1 || alpns[0] != "h2" { t.Errorf("ALPN: got %v, want [h2]", alpns) }
}

func TestParseClientHelloEmpty(t *testing.T) {
	_, _, ok := parseClientHello(nil)
	if ok { t.Errorf("parseClientHello(nil): ok=true; want false") }
}

func TestParseClientHelloNonTLSPreamble(t *testing.T) {
	buf := []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")
	_, _, ok := parseClientHello(buf)
	if ok { t.Errorf("parseClientHello(non-TLS): ok=true; want false") }
}

func TestParseClientHelloTruncated(t *testing.T) {
	buf := captureClientHello(t, "foo.example.test", nil)
	for cut := 1; cut < len(buf) && cut < 50; cut++ {
		_, _, ok := parseClientHello(buf[:cut])
		if ok { t.Errorf("parseClientHello(truncated to %d): ok=true; want false", cut) }
	}
}

func TestParseClientHelloMalformedLengthPrefix(t *testing.T) {
	// TLS record header: 0x16 (handshake) 0x03 0x03 (TLS 1.2) 0xFF 0xFF (length)
	// followed by no body. Should return ok=false without panic.
	buf := []byte{0x16, 0x03, 0x03, 0xFF, 0xFF, 0x01, 0x00}
	defer func() {
		if r := recover(); r != nil { t.Errorf("parseClientHello panicked: %v", r) }
	}()
	_, _, _ = parseClientHello(buf)
}
```

- [ ] **Step 2: Run tests; confirm they fail (parser does not exist)**

- [ ] **Step 3: Write `parser.go`**

The parser walks the TLS record header (5 bytes), the Handshake header (4 bytes), the ClientHello body (legacy_version 2 + random 32 + legacy_session_id_length 1 + session_id_bytes + cipher_suites_length 2 + cipher_suites_bytes + legacy_compression_methods_length 1 + compression_methods_bytes + extensions_length 2 + extensions_bytes), then iterates extensions looking for type 0 (server_name) and type 16 (application_layer_protocol_negotiation). The parser is defensive: every length-bounded read checks remaining-buffer-size before advancing; malformed inputs return `ok=false`.

```go
package tls_inspector

import "encoding/binary"

// parseClientHello extracts the SNI server_name + ALPN application_protocols
// from a TLS ClientHello byte buffer. Returns ok=false on any malformed
// input (truncated, wrong record type, length-prefix mismatch). Pure
// function; no I/O. Adapted from crypto/tls/handshake_messages.go:unmarshal
// for the ClientHello case, narrowed to the two extensions of interest.
func parseClientHello(buf []byte) (sni string, alpns []string, ok bool) {
	// TLS record header: 5 bytes.
	if len(buf) < 5 { return "", nil, false }
	if buf[0] != 0x16 { return "", nil, false } // not a Handshake record
	// buf[1:3] = legacy_version (TLS 1.0–1.2 marker; ClientHello is allowed
	// to have any value here per RFC 8446 §4.1.2).
	recordLen := int(binary.BigEndian.Uint16(buf[3:5]))
	if 5+recordLen > len(buf) { return "", nil, false } // truncated record
	body := buf[5 : 5+recordLen]
	// Handshake header: 4 bytes.
	if len(body) < 4 { return "", nil, false }
	if body[0] != 0x01 { return "", nil, false } // not a ClientHello
	hsLen := int(body[1])<<16 | int(body[2])<<8 | int(body[3])
	if 4+hsLen > len(body) { return "", nil, false }
	ch := body[4 : 4+hsLen]
	// ClientHello body: legacy_version (2) + random (32) + session_id_length (1) + ...
	off := 0
	if off+2+32+1 > len(ch) { return "", nil, false }
	off += 2 + 32 // skip legacy_version + random
	sidLen := int(ch[off])
	off++
	if off+sidLen+2 > len(ch) { return "", nil, false }
	off += sidLen
	csLen := int(binary.BigEndian.Uint16(ch[off : off+2]))
	off += 2
	if off+csLen+1 > len(ch) { return "", nil, false }
	off += csLen
	cmLen := int(ch[off])
	off++
	if off+cmLen+2 > len(ch) { return "", nil, false }
	off += cmLen
	if off+2 > len(ch) { return "", nil, true } // no extensions block — valid
	extLen := int(binary.BigEndian.Uint16(ch[off : off+2]))
	off += 2
	if off+extLen > len(ch) { return "", nil, false }
	exts := ch[off : off+extLen]
	// Iterate extensions.
	for len(exts) >= 4 {
		typ := binary.BigEndian.Uint16(exts[:2])
		ln := int(binary.BigEndian.Uint16(exts[2:4]))
		if 4+ln > len(exts) { return "", nil, false }
		body := exts[4 : 4+ln]
		switch typ {
		case 0x0000: // server_name
			if name, ok := parseServerName(body); ok {
				sni = name
			}
		case 0x0010: // application_layer_protocol_negotiation
			if al, ok := parseALPN(body); ok {
				alpns = al
			}
		}
		exts = exts[4+ln:]
	}
	return sni, alpns, true
}

// parseServerName walks the ServerNameList per RFC 6066 §3. Returns the
// first host_name (NameType 0) — Envoy convention.
func parseServerName(buf []byte) (string, bool) {
	if len(buf) < 2 { return "", false }
	listLen := int(binary.BigEndian.Uint16(buf[:2]))
	if 2+listLen > len(buf) { return "", false }
	list := buf[2 : 2+listLen]
	for len(list) >= 3 {
		nameType := list[0]
		nameLen := int(binary.BigEndian.Uint16(list[1:3]))
		if 3+nameLen > len(list) { return "", false }
		if nameType == 0x00 { // host_name
			return string(list[3 : 3+nameLen]), true
		}
		list = list[3+nameLen:]
	}
	return "", false
}

// parseALPN walks the ProtocolNameList per RFC 7301 §3.1. Returns every
// protocol_name in declaration order.
func parseALPN(buf []byte) ([]string, bool) {
	if len(buf) < 2 { return nil, false }
	listLen := int(binary.BigEndian.Uint16(buf[:2]))
	if 2+listLen > len(buf) { return nil, false }
	list := buf[2 : 2+listLen]
	var out []string
	for len(list) >= 1 {
		nameLen := int(list[0])
		if 1+nameLen > len(list) { return nil, false }
		out = append(out, string(list[1:1+nameLen]))
		list = list[1+nameLen:]
	}
	return out, true
}
```

- [ ] **Step 4: Run tests; confirm they pass**

```bash
go test -race ./internal/listener/listenerfilter/tls_inspector/... -v 2>&1 | tail -30
```

Expected: all PASS. Quote the last 30 lines into PROGRESS.

- [ ] **Step 5: Append PROGRESS Task 7 entry + Commit**

```bash
git add internal/listener/listenerfilter/tls_inspector/parser.go internal/listener/listenerfilter/tls_inspector/parser_test.go docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: internal/listener/listenerfilter/tls_inspector/parser.go — minimal ClientHello extractor"
```

SHA-fill follow-up.

*Anchored: SPEC §1 #2, §4.1 (parser.go), §15.2 (parser_test.go), D-3.2 (no cgo / C++ binding to upstream Envoy's tls_inspector).*

---

## Task 8: `internal/listener/listenerfilter/tls_inspector/{doc,tls_inspector,proto}.go` — full ListenerFilter implementation

**Files:**
- Create: `internal/listener/listenerfilter/tls_inspector/doc.go`
- Create: `internal/listener/listenerfilter/tls_inspector/tls_inspector.go`
- Create: `internal/listener/listenerfilter/tls_inspector/proto.go`
- Create: `internal/listener/listenerfilter/tls_inspector/tls_inspector_test.go`
- Create: `internal/listener/listenerfilter/tls_inspector/proto_test.go`

This task implements the concrete `tls_inspector` listener filter: peeks the connection's byte preamble, runs `parseClientHello`, populates `inputs.ServerName` + `inputs.ApplicationProtocols` + `inputs.TransportProtocol`. The proto config (`envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector`) is parsed by `proto.go`: `initial_read_buffer_size` is honored (clamped [256, 65536]; defaulted 4096); `enable_ja3_fingerprinting` is silently ignored per SPEC §12.

**Precondition:** Tasks 2-7 done.
**Artifact:** `doc.go` + `tls_inspector.go` + `proto.go` + tests.
**Acceptance:** `go test -race ./internal/listener/listenerfilter/tls_inspector/...` passes; full real-ClientHello + non-TLS preamble + partial ClientHello scenarios work; the `tls_inspector.New` factory round-trips through the registry; concurrent inspection on independent connections is race-clean; `initial_read_buffer_size` clamps work.

- [ ] **Step 1: Write `tls_inspector_test.go` + `proto_test.go` failing tests** covering: `Inspect` with real ClientHello populates `inputs.ServerName` + `inputs.ApplicationProtocols` + `inputs.TransportProtocol="tls"`; `Inspect` with non-TLS preamble (e.g., `GET / HTTP/1.1\r\n`) sets `inputs.TransportProtocol="raw_buffer"`, leaves SNI + ALPN at zero; concurrent inspection on independent connections is race-clean; the type_url + factory round-trip through the registry; proto-config parsing honors `initial_read_buffer_size` (clamped [256, 65536]; default 4096; values < 256 error at parse with `tls_inspector: initial_read_buffer_size %d below floor 256`); `enable_ja3_fingerprinting=true` is silently ignored (no error).

- [ ] **Step 2: Run tests; confirm they fail**

- [ ] **Step 3: Write `doc.go`** — package overview pointing to the concrete filter's role (peek ClientHello → populate ChainMatchInputs).

- [ ] **Step 4: Write `tls_inspector.go`**

```go
package tls_inspector

import (
	"context"
	"errors"

	"github.com/esalaine/envoy-go/internal/listener/listenerfilter"
	"google.golang.org/protobuf/types/known/anypb"
)

// TypeURL is the proto type_url for the tls_inspector listener filter, per
// upstream go-control-plane.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector"

// New is the ListenerFilterFactory entry point. Parses the typed_config
// (TlsInspector proto), validates initial_read_buffer_size against the
// [256, 65536] envelope (defaults 4096 if unset), and returns a
// per-connection FilterInstanceFactory closure capturing the parsed config.
func New(tc *anypb.Any, ctx listenerfilter.FactoryCtx) (listenerfilter.FilterInstanceFactory, error) {
	cfg, err := parseConfig(tc)
	if err != nil {
		return nil, err
	}
	return func() listenerfilter.ListenerFilter {
		return &filter{cfg: cfg}
	}, nil
}

// config is the parsed tls_inspector configuration.
type config struct {
	bufferSize int
}

// filter is the per-connection ListenerFilter instance.
type filter struct {
	cfg *config
}

// Inspect peeks the connection preamble; if a TLS ClientHello is detected,
// populates inputs with extracted SNI + ALPN. Else sets
// inputs.TransportProtocol = "raw_buffer". Always returns Continue (the
// pipeline advances regardless of inspection outcome).
func (f *filter) Inspect(ctx context.Context, peeker listenerfilter.Peeker, inputs *listenerfilter.ChainMatchInputs) (listenerfilter.ListenerFilterStatus, error) {
	buf, err := peeker.Peek(f.cfg.bufferSize)
	if err != nil && len(buf) == 0 {
		// Connection closed before any bytes arrived; non-fatal — let the
		// chain-match algorithm operate on the un-inspected facts.
		if errors.Is(err, context.Canceled) {
			return listenerfilter.Continue, ctx.Err()
		}
		inputs.TransportProtocol = "raw_buffer"
		return listenerfilter.Continue, nil
	}
	sni, alpns, ok := parseClientHello(buf)
	if !ok {
		inputs.TransportProtocol = "raw_buffer"
		return listenerfilter.Continue, nil
	}
	inputs.TransportProtocol = "tls"
	if sni != "" { inputs.ServerName = sni }
	if len(alpns) > 0 { inputs.ApplicationProtocols = alpns }
	return listenerfilter.Continue, nil
}

// OnDestroy releases per-connection resources. tls_inspector holds none.
func (f *filter) OnDestroy() {}
```

- [ ] **Step 5: Write `proto.go`**

```go
package tls_inspector

import (
	"fmt"

	tls_inspectorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	defaultBufferSize = 4096
	minBufferSize     = 256
	maxBufferSize     = 65536
)

func parseConfig(tc *anypb.Any) (*config, error) {
	if tc == nil {
		return &config{bufferSize: defaultBufferSize}, nil
	}
	var pb tls_inspectorv3.TlsInspector
	if err := tc.UnmarshalTo(&pb); err != nil {
		return nil, fmt.Errorf("tls_inspector: typed_config unmarshal: %w", err)
	}
	cfg := &config{bufferSize: defaultBufferSize}
	if pb.GetInitialReadBufferSize() != nil {
		v := int(pb.GetInitialReadBufferSize().GetValue())
		if v < minBufferSize {
			return nil, fmt.Errorf("tls_inspector: initial_read_buffer_size %d below floor %d", v, minBufferSize)
		}
		if v > maxBufferSize {
			v = maxBufferSize // clamp without error per ADR-0079 Decision C
		}
		cfg.bufferSize = v
	}
	// pb.EnableJa3Fingerprinting is silently ignored per SPEC §12.
	return cfg, nil
}
```

- [ ] **Step 6: Run tests; confirm they pass under `-race`**

```bash
go test -race ./internal/listener/listenerfilter/tls_inspector/... -v 2>&1 | tail -30
```

- [ ] **Step 7: Append PROGRESS Task 8 entry + Commit**

```bash
git add internal/listener/listenerfilter/tls_inspector/ docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: internal/listener/listenerfilter/tls_inspector/ — concrete ListenerFilter (ClientHello peek → SNI/ALPN/transport_protocol)"
```

SHA-fill follow-up.

*Anchored: SPEC §1 #2, §4.1 (tls_inspector files), §6.4 (factory pattern), §11.4 (carry-forward — resolved at Task 16; tls_inspector contributes the ALPN that fixture-0008 exercises), §12 (silent-ignore set including `enable_ja3_fingerprinting`), §15.2 (tls_inspector_test.go).*

---

## Task 9: `internal/listener/manager.go` — `validateFilterChainMatch` rewrite + parse `default_filter_chain` + parse `listener_filters[]`; constructor signature widening; `[]*ChainSpec` building [ADR-0078]

**Files:**
- Modify: `internal/listener/manager.go`
- Modify: `internal/listener/manager_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0078)

This task is the largest single-file refactor of the phase. It rewrites `validateFilterChainMatch` (currently rejecting most dimensions per ADR-0033) into `parseChainSpec` (returning a `*chainmatch.ChainSpec` accepting all 8 dimensions per ADR-0078); removes the `default_filter_chain` parse-time error at line 251 (instead constructs a default chain stored on `listenerRuntime.defaultChain`); adds `listener_filters[]` parsing (each entry resolved via `*ListenerFilterRegistry.Lookup`; per-connection factories stored on `listenerRuntime.listenerFilters[]`); preserves `chainSpecificityRank` at line 352 as the SNI-internal tie-breaker reused by `chainmatch.SelectChain`; widens constructor signatures to thread `*listenerfilter.ListenerFilterRegistry`. Does NOT yet touch the dispatch path (Task 10 owns that).

**Precondition:** Tasks 2-8 done; the `*ListenerFilterRegistry` + `ChainSpec` are ready to consume.
**Artifact:** `manager.go` rewrite of validation + parsing + struct fields; ADR-0078 in DECISIONS.md.
**Acceptance:** `go vet ./internal/listener/... && go test ./internal/listener/...` passes; `validateFilterChainMatch`'s 8-dimension acceptance is unit-tested in `manager_test.go` (each dimension parses without error; `direct_source_prefix_ranges` silently ignored — no error); `default_filter_chain` parses without error; `listener_filters[]` resolves through the registry; mixed-TLS rule preserved within `filter_chains[]`; SNI-only-on-plaintext-with-multi-chain rule preserved (per SPEC §5.7 clause 6); identical-chains error surfaces with `ambiguous selection` message.

- [ ] **Step 1: Write failing tests in `manager_test.go`** for the 8-dimension acceptance: `destination_port` parses; `prefix_ranges` parses; `source_prefix_ranges` parses; `source_type: LOCAL` parses; `source_ports` parses; `application_protocols` parses; `transport_protocol="raw_buffer"` parses; `direct_source_prefix_ranges` set → silently ignored (no error); `default_filter_chain` set → parses (no longer errors); `listener_filters[{name: tls_inspector, typed_config: {...}}]` parses; `listener_filters_timeout: 5s` parses; `listener_filters_timeout: 0.5s` errors with `[1s, 60s] envelope` message; `listener_filters_timeout: 90s` errors with same message; mixed-TLS rule preserved within `filter_chains[]`; identical filter_chains[i] and filter_chains[j] error with `ambiguous selection` (the test crafts two chains with identical match dimensions).

- [ ] **Step 2: Rewrite `validateFilterChainMatch` (line 378) → `parseChainSpec`** returning `*chainmatch.ChainSpec`. Drop the seven "is not supported (phase 07)" errors at lines 382-398; instead populate ChainSpec fields from `fm.GetDestinationPort()`, `fm.GetPrefixRanges()`, etc. `fm.GetDirectSourcePrefixRanges()` is silently ignored per SPEC §12 (no field on ChainSpec). The `transport_protocol == "tls"` line at 400-402 widens to permit any value (the proto's documented enum is "tls"/"raw_buffer"/empty; `parseChainSpec` accepts those + emits a parse-error on unknown values). Build the `*ChainSpec` and return it.

- [ ] **Step 3: Remove the `default_filter_chain` error at line 251.** Replace with: parse `l.GetDefaultFilterChain()` if non-nil — call `parseChainSpec` (with `Empty: true` since `default_filter_chain` has no `filter_chain_match`), build the per-connection factory for its single terminal filter, store on `listenerRuntime.defaultChain *chainInfo`. The mixed-TLS rule from line 313 is kept WITHIN `filter_chains[]` only (per SPEC §5.7 clause 5 + ADR-0080: `default_filter_chain`'s TLS posture is independent).

- [ ] **Step 4: Parse `listener_filters[]`** at the per-listener loop. For each entry: `Lookup(typeURL)` on the threaded registry; if not found, error with `listener: %q: listener_filters[%d]: unknown filter type_url %q`; if found, call the factory to get the per-connection `FilterInstanceFactory`; store the factory slice on `listenerRuntime.listenerFilterFactories []FilterInstanceFactory`. Also parse `l.GetListenerFiltersTimeout()` → `lfTimeoutMs uint32` (default 15000; values outside [1000, 60000] error with `listener: %q: listener_filters_timeout %s is outside the supported [1s, 60s] envelope` per ADR-0082); parse `l.GetContinueOnListenerFiltersTimeout()` → `continueOnLfTimeout bool`. Store both on `listenerRuntime`.

- [ ] **Step 5: Build `[]*chainmatch.ChainSpec`** alongside the existing `[]*chainInfo` per chain (Decision O: `[]*ChainSpec` immutable after build; concurrent reads safe). Detect identical chains (chains with identical specificity vectors AND identical sub-ordering) at build time per ADR-0081's "final ties at NewManager-build time error" — call `SelectChain` against a synthetic input that hits both chains and check the result; OR (cheaper) sort the ChainSpec slice and compare adjacent entries; on a positive detection, return `listener: %q: filter_chains[i] and filter_chains[j] have identical filter_chain_match — ambiguous selection`.

- [ ] **Step 6: Widen `NewManagerWithBaseDirAndAllowH2C` signature** to add `lfRegistry *listenerfilter.ListenerFilterRegistry` parameter. Update `NewManager` + `NewManagerWithBaseDir` to delegate. Update existing test bootstraps to thread an empty registry (or a `tls_inspector`-only frozen one) — manager_test.go + listener_test.go bootstrap helpers update mechanically.

- [ ] **Step 7: DELETE `chainSpecificityRank` from `internal/listener/manager.go` (line 352).** With the line-327 `sort.SliceStable` chain-sort removed (per Decision L) and the `dispatch` function deleted at Task 10, `chainSpecificityRank` has no remaining callers in `internal/listener/`. The SNI-internal tie-breaker logic now lives ONLY as `sniSpecificityRank` in `internal/listener/listenerfilter/chainmatch.go` (introduced at Task 5 step 3 — a verbatim adaptation of the deleted function). Keeping `chainSpecificityRank` in manager.go would surface as `unused` warnings under `go vet` / `golangci-lint` at step 8. The Architecture text's "preserved as the SNI-internal tie-breaker" phrasing refers to the LOGIC's preservation in chainmatch.go's copy, not the in-manager.go preservation of the symbol. Manager.go imports `internal/listener/listenerfilter` but the listenerfilter package does NOT import manager (the reverse import would be cyclic), so the chainmatch.go internal copy is the right place for the live function.

- [ ] **Step 8: Run tests; confirm they pass**

```bash
go vet ./internal/listener/...
go test -race ./internal/listener/...
```

- [ ] **Step 9: Append ADR-0078 + PROGRESS Task 9 entry + Commit**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: internal/listener/manager.go — parseChainSpec rewrite + default_filter_chain + listener_filters[] [ADR-0078]"
```

SHA-fill follow-up.

*Anchored: SPEC §1 #4, §1 #6, §4.2 (manager.go MODIFIED — ~+200 LoC net), §5.7 (ADR-0033 supersession), §7.1 (eight dimensions), §10 (ADR-0078 anticipation), §15.3 (manager_test.go extensions).*

---

## Task 10: `internal/listener/manager.go` — accept-loop dispatch refactor (unified pre/post-handshake path per SPEC §5.2)

**Files:**
- Modify: `internal/listener/manager.go`
- Modify: `internal/listener/manager_test.go`
- Modify: `internal/listener/listener_test.go`

This task replaces the SNI-only post-handshake `dispatch` function (line 434) with the unified pre/post-handshake dispatch path per SPEC §5.2: (1) accept conn → (2) allocate `ChainMatchInputs` from `LocalAddr()` + `RemoteAddr()` → (3) wrap raw conn in `peekerConn` → (4) construct per-connection ListenerFilter instances from the listener's factory slice → (5) `Pipeline.Run` → (6) `chainmatch.SelectChain` → (7) if selected has TLS, run handshake using selected chain's `tls.Config` directly (no `GetConfigForClient` callback for chain-match) → (8) dispatch to `selected.filter.Handle(ctx, dispatchConn)`. The `makeGetConfigForClient` callback at line 413 is simplified — it no longer runs chain-match (chain is pre-selected). The `serveTLS` function at line 550 collapses into the unified dispatch path. The `acceptLoop` (line 513) gains the per-conn pipeline + chain-match logic; the existing `downstreamCxTotal.Inc` + `downstreamCxActive.Inc/Dec` discipline at lines 528-529 is preserved.

**Precondition:** Task 9 done; `listenerRuntime` has `listenerFilterFactories`, `defaultChain`, `chainSpecs []*chainmatch.ChainSpec`, `lfTimeoutMs`, `continueOnLfTimeout` fields.
**Artifact:** `manager.go` dispatch refactor.
**Acceptance:** `go test -race ./internal/listener/...` passes; pre-existing fixtures `0000`-`0007b` are still differentially green when re-run with `go test ./test/differential/... -run 'Test.*0000|0001|0002|0003|0004|0005|0006|0007a|0007b' -v`; the unified dispatch path correctly handles plaintext + TLS + listener-filter pipeline + chain-match + default_filter_chain fallback in unit tests.

- [ ] **Step 1: Write failing tests in `manager_test.go`** asserting the unified dispatch path: a plaintext listener with no listener_filters[] dispatches into the matching chain by `destination_port` / `prefix_ranges` / `source_*`; a TLS listener with `tls_inspector` populates `inputs.ServerName` from the ClientHello, then chain-match selects on `server_names`, then handshake uses the selected chain's TLS config; a listener with `default_filter_chain` (no `filter_chains[]` matching) dispatches into the default; a listener-filter timeout aborts when `continue_on_listener_filters_timeout=false`, treats as Continue when `=true`.

- [ ] **Step 2: Refactor `acceptLoop` (line 513)** to call a new `serveConnection` helper that owns the unified dispatch:

```go
func (rt *listenerRuntime) serveConnection(ctx context.Context, raw net.Conn) {
	defer rt.downstreamCxActive.Dec()

	// (1) ChainMatchInputs from connection-level facts.
	inputs := listenerfilter.ChainMatchInputs{
		DestinationIP:   localIP(raw),
		DestinationPort: localPort(raw),
		SourceIP:        remoteIP(raw),
		SourcePort:      remotePort(raw),
	}

	// (2) Wrap in peekerConn so listener filters can read without consuming.
	pkConn := listenerfilter.NewPeekerConn(raw)

	// (3) Construct per-connection ListenerFilter instances from factories.
	filters := make([]listenerfilter.ListenerFilter, len(rt.listenerFilterFactories))
	for i, fac := range rt.listenerFilterFactories {
		filters[i] = fac()
	}

	// (4) Run listener-filter pipeline.
	var p listenerfilter.Pipeline
	if err := p.Run(ctx, filters, listenerfilter.AsPeeker(pkConn), &inputs, rt.lfTimeoutMs); err != nil {
		if !rt.continueOnLfTimeout {
			log.Printf("listener %q: listener-filter pipeline aborted: %v", rt.name, err)
			_ = pkConn.Close()
			return
		}
		// continue_on_listener_filters_timeout=true: fall through with partial inputs.
	}

	// (5) Run chain-match algorithm.
	selectedSpec, err := listenerfilter.SelectChain(inputs, rt.chainSpecs, rt.defaultSpec)
	if err != nil {
		log.Printf("listener %q: chain-match: %v", rt.name, err)
		_ = pkConn.Close()
		return
	}
	selected := rt.chainByName[selectedSpec.Name]

	// (6) If selected chain has TLS, run handshake.
	dispatchConn := pkConn
	if selected.tlsCfg != nil {
		tlsConn := stdtls.Server(pkConn, selected.tlsCfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			log.Printf("listener %q: handshake: %v", rt.name, err)
			_ = pkConn.Close()
			return
		}
		dispatchConn = tlsConn
	}

	// (7) Dispatch to terminal filter.
	selected.filter.Handle(ctx, dispatchConn)
}
```

The helper accessors `localIP`, `localPort`, `remoteIP`, `remotePort` extract from `net.TCPAddr`; helper `chainByName` is a `map[string]*chainInfo` populated at NewManager-build time alongside `chainSpecs` (Task 9 step 5 builds this in parallel).

- [ ] **Step 3: Replace `acceptLoop` body** with a wrapper that just preserves the `Inc/Dec` discipline + spawns `go rt.serveConnection(ctx, raw)`:

```go
func (rt *listenerRuntime) acceptLoop(ctx context.Context, ln net.Listener) {
	for {
		raw, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) { return }
			if ctx.Err() != nil { return }
			log.Printf("listener %q: accept: %v", rt.name, err)
			continue
		}
		rt.downstreamCxTotal.Inc()
		rt.downstreamCxActive.Inc()
		go rt.serveConnection(ctx, raw)
	}
}
```

- [ ] **Step 4: Simplify `makeGetConfigForClient` (line 413)**. With chain selection happening BEFORE the TLS handshake (the chain's `tls.Config` is passed directly to `stdtls.Server`), the `GetConfigForClient` callback is no longer needed for chain-match — it's only useful when a single `tls.Config` must dynamically select certs based on SNI within ONE chain. Phase-03's per-chain `tls.Config` allocation already includes the right cert for the chain's server_names; pass it in directly. Delete `makeGetConfigForClient` + remove the `rt.tlsCfg = &stdtls.Config{GetConfigForClient: ...}` allocation at line 338-340.

- [ ] **Step 5: Delete `dispatch` function (line 434) and `serveTLS` function (line 550)**. Their behavior is folded into `serveConnection`.

- [ ] **Step 6: Update `chainInfo` struct** if needed: confirm `serverNames`, `tlsCfg`, `filter` are still the only fields (Task 9 added `defaultSpec` separately on `listenerRuntime`; chainInfo itself doesn't change).

- [ ] **Step 7: Run tests; confirm they pass**

```bash
go vet ./internal/listener/...
go test -race ./internal/listener/...
go test ./test/differential/ -run 'Test.*0000|0001|0002|0003|0004|0005|0006|0007a|0007b' -v 2>&1 | tail -30
```

Expected: pre-existing fixtures still PASS (0000 through 0007b).

- [ ] **Step 8: Append PROGRESS Task 10 entry + Commit**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go internal/listener/listener_test.go docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: internal/listener/manager.go — unified pre/post-handshake dispatch (acceptLoop + serveConnection)"
```

SHA-fill follow-up.

*Anchored: SPEC §1 #7, §4.2 (manager.go MODIFIED), §5.2 (per-connection lifecycle), §5.6 (concurrency model — single-goroutine-per-connection drives the pipeline + dispatch), §15.3 (manager_test.go + listener_test.go extensions).*

---

## Task 11: `cmd/envoy-go/main.go` + `internal/bootstrap/bootstrap.go` — boot wiring (Registry alloc + Register tls_inspector + Freeze; tls_inspector v3 blank import)

**Files:**
- Modify: `cmd/envoy-go/main.go`
- Modify: `cmd/envoy-go/main_test.go`
- Modify: `internal/bootstrap/bootstrap.go`
- Modify: `internal/bootstrap/bootstrap_test.go`

This task wires the boot-time `*ListenerFilterRegistry` allocation + `tls_inspector` registration + Freeze + threading into `listener.NewManagerWithBaseDirAndAllowH2C`. Adds the blank-import for `tls_inspector v3` proto so `protojson` round-trips fixture-0008 bootstraps.

**Precondition:** Tasks 2-10 done.
**Artifact:** `main.go` boot-wiring diff; `bootstrap.go` blank-import diff; tests.
**Acceptance:** `go vet ./...` clean; `go test ./...` passes; the new boot sequence allocates `lfReg`, registers `tls_inspector.New` under `tls_inspector.TypeURL`, calls `Freeze()`, and threads `lfReg` into the listener-manager constructor. Manual smoke test: a bootstrap containing `listener_filters: [{name: envoy.filters.listener.tls_inspector, typed_config: ...}]` parses without error; `protojson` round-trips the typed_config.

- [ ] **Step 1: Write failing tests in `main_test.go` + `bootstrap_test.go`** — bootstrap parser must round-trip a fixture containing `listener_filters[]` with `tls_inspector v3` typed_config; main.go boot must Freeze the registry before listener manager Start.

- [ ] **Step 2: Modify `cmd/envoy-go/main.go`** — at boot, after stats Registry allocation, before `listenerManager.New(...)`:

```go
import (
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter"
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector"
)
// ...
lfReg := listenerfilter.NewListenerFilterRegistry()
lfReg.Register(tls_inspector.TypeURL, tls_inspector.New)
lfReg.Freeze()
// ...
mgr, err := listener.NewManagerWithBaseDirAndAllowH2C(bs, cm, baseDir, *allowH2C, statsRegistry, accessLogSinks, httpRegistry, lfReg)
```

- [ ] **Step 3: Modify `internal/bootstrap/bootstrap.go`** — add blank import:

```go
import (
	// ...
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
)
```

- [ ] **Step 4: Update `bootstrap_test.go`** to round-trip a sample fixture-0008-shape bootstrap containing `listener_filters[]` with `tls_inspector` typed_config.

- [ ] **Step 5: Run tests; confirm they pass**

```bash
go vet ./...
golangci-lint run ./...
go test -race ./... -short
```

- [ ] **Step 6: Append PROGRESS Task 11 entry + Commit**

```bash
git add cmd/envoy-go/main.go cmd/envoy-go/main_test.go internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: cmd/envoy-go/main.go + internal/bootstrap/bootstrap.go — Registry boot wiring + tls_inspector v3 blank import"
```

SHA-fill follow-up. Note: this commit completes the production-code surface; pre-existing fixtures (0000-0007b) must be re-runnable from THIS commit.

*Anchored: SPEC §4.2 (cmd/envoy-go/main.go + internal/bootstrap/bootstrap.go MODIFIED), §5.1 (module graph), §5.6 (registry freeze invariant).*

---

## Task 12: `internal/listener/integration_test.go` — end-to-end accept-loop unit test

**Files:**
- Create: `internal/listener/integration_test.go`

This task introduces an in-process end-to-end test of the new unified dispatch path: a TLS connection with SNI + ALPN dispatches to the matching chain; a plaintext connection dispatches to the matching `destination_port` chain; a non-matching connection falls to `default_filter_chain`. Pure Go; no Docker. Exercises the full §5.2 dispatch path under unit-test scope before fixture-0008 lands at Task 16.

**Precondition:** Tasks 2-11 done.
**Artifact:** `integration_test.go` (~250 LoC).
**Acceptance:** `go test -race ./internal/listener/... -run TestIntegration`. The test runs the full `Manager.Start` + `acceptLoop` + `serveConnection` against an in-process bootstrap with two filter_chains (matching `destination_port: <subj_port>` and `source_prefix_ranges: 127.0.0.1/32`) plus a `default_filter_chain`; opens 5 client connections covering: (i) match `chain_dstport_only`; (ii) match `chain_srcprefix_only`; (iii) match both, precedence selects `chain_dstport_only`; (iv) match neither (when only specific chains exist), falls to default; (v) listener_filters timeout abort.

- [ ] **Step 1: Write `integration_test.go`** with table-driven sub-tests. Use `net.Listen("tcp", "127.0.0.1:0")` for the listener bind; OS-pick the port; populate the bootstrap programmatically (or via a YAML fixture loaded inline); start the `Manager`; dial the bound port; assert the response body matches the expected backend.

- [ ] **Step 2: Run the test under `-race`**

```bash
go test -race ./internal/listener/... -run TestIntegration -v 2>&1 | tail -30
```

- [ ] **Step 3: Append PROGRESS Task 12 entry + Commit**

```bash
git add internal/listener/integration_test.go docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: internal/listener/integration_test.go — end-to-end accept-loop unit test"
```

SHA-fill follow-up.

*Anchored: SPEC §15.3 (integration_test.go).*

---

## Task 13: `test/differential/fixture/fixture.go` — `MultiListenerDriver` + `AlternateConfigDriver` optional interfaces

**Files:**
- Modify: `test/differential/fixture/fixture.go`
- Modify: `test/differential/fixture/fixture_test.go`

This task extends the fixture Driver interface set with two new optional interfaces required by fixture-0008's dual-listener + c4-variant shape (per SPEC §7.4 + Decision G). The existing `Driver` interface is UNCHANGED (multi-listener drivers return the FIRST listener as the primary in `SubjectListenerName()` for compat). The runner branches on the optional interfaces at Task 14.

**Precondition:** Tasks 2-12 done.
**Artifact:** `fixture.go` extension; tests.
**Acceptance:** `go vet ./test/differential/...` clean; `go test ./test/differential/fixture/...` passes (the type-assertion happy path is exercised by a stub driver in the test).

- [ ] **Step 1: Append to `fixture.go`**:

```go
// MultiListenerDriver is an OPTIONAL driver-side interface for fixtures
// that target >1 listener simultaneously. Drivers that implement it return
// >=2 listener names (and matching reference ports); the runner allocates
// additional subject ports and exposes additional reference ports, then
// dispatches DriveReferenceMulti / DriveSubjectMulti instead of the
// single-addr Drive variants. The single-addr Driver methods
// (SubjectListenerName / ReferenceListenerPort / DriveReference /
// DriveSubject) MUST still be implemented (returning the first listener as
// the primary) so the runner's pre-multi-branch path still works for the
// fixture-discovery / admin-probe steps.
//
// Introduced by phase 07.2 / fixture-0008 per SPEC §7.4.
type MultiListenerDriver interface {
	SubjectListenerNames() []string
	ReferenceListenerPorts() []int
	DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error)
	DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error)
}

// AlternateConfigDriver is an OPTIONAL driver-side interface for fixtures
// that need to spawn a SECOND ref+subj pair with an alternate bootstrap
// (e.g., to exercise a code path the primary bootstrap cannot reach
// without removing one of its chains). The runner spawns the alternate
// pair AFTER the primary diff completes, runs DriveAlternate against the
// alternate addrs, and diffs the resulting bytes. fixture-0008 uses this
// for the c4 variant (chain_other removed) which exercises the
// default_filter_chain fallback.
//
// Introduced by phase 07.2 / fixture-0008 per SPEC §7.4 + Decision G.
type AlternateConfigDriver interface {
	AlternateReferenceBootstrap(backendPorts []int) string
	AlternateSubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string
	AlternateSubjectListenerName() string
	AlternateReferenceListenerPort() int
	DriveAlternate(ctx context.Context, refAddr, subjAddr string) ([]byte, error)
}
```

- [ ] **Step 2: Add a test in `fixture_test.go`** that constructs a stub Driver implementing both optional interfaces and verifies the type-assertions succeed.

- [ ] **Step 3: Run tests**

```bash
go test ./test/differential/fixture/... -v 2>&1 | tail -20
```

- [ ] **Step 4: Append PROGRESS Task 13 entry + Commit**

```bash
git add test/differential/fixture/fixture.go test/differential/fixture/fixture_test.go docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: test/differential/fixture — MultiListenerDriver + AlternateConfigDriver optional interfaces"
```

SHA-fill follow-up.

*Anchored: SPEC §7.4 (dual-listener construction + c4 variant), §9.2 (driver outline), Decision G.*

---

## Task 14: `test/differential/runner_test.go` — runner branches for MultiListenerDriver + AlternateConfigDriver

**Files:**
- Modify: `test/differential/runner_test.go`

This task adds the runner-side branches that consume the two new optional interfaces from Task 13. After the standard ref+subj startup + `DriveReference` + `DriveSubject` + `CompareBytes`:

- Type-assert on `MultiListenerDriver`; if implemented, after the standard startup the runner allocates additional `freeTCPPort` for each extra listener name in `SubjectListenerNames()[1:]` and additional reference port via `StartReferenceProxyWithMounts(...)` exposing `ReferenceListenerPorts()` instead of just `ReferenceListenerPort()`. The runner then constructs `addrs map[string]string` mapping each subject listener name → bound subject addr (and same for reference). Calls `DriveSubjectMulti(ctx, addrs)` / `DriveReferenceMulti(ctx, addrs)` instead of the single-addr Drives.
- Type-assert on `AlternateConfigDriver`; if implemented, after the primary diff the runner spawns a SECOND ref+subj pair using `AlternateReferenceBootstrap` + `AlternateSubjectConfig` (spawned via the same `StartReferenceProxy*` + `StartSubjectProxy` helpers); calls `DriveAlternate`; diffs its bytes via `CompareBytes`.

**Precondition:** Task 13 done.
**Artifact:** `runner_test.go` extensions (~200 LoC).
**Acceptance:** `go vet ./test/differential/...` clean; pre-existing fixtures (0000-0007b) — none of which implement the new interfaces — still PASS unchanged.

- [ ] **Step 1: Add the multi-listener branch** in `runFixture` (around line 290 where `DriveSubject` is called). Pseudo-code:

```go
var subjBytes []byte
if mld, ok := d.(fixture.MultiListenerDriver); ok {
	subjAddrs := map[string]string{}
	for _, name := range mld.SubjectListenerNames() {
		subjAddrs[name] = subj.ListenerAddr(name)
	}
	subjBytes, err = mld.DriveSubjectMulti(ctx, subjAddrs)
} else {
	subjBytes, err = d.DriveSubject(ctx, subj.ListenerAddr(d.SubjectListenerName()))
}
```

Same shape on the reference side. Note: the reference-container exposed-ports list must include `ReferenceListenerPorts()` (call `StartReferenceProxy*` with the slice instead of single port).

- [ ] **Step 2: Add the alternate-config branch** AFTER the standard diff (after `CompareBytes` returns). Pseudo-code:

```go
if acd, ok := d.(fixture.AlternateConfigDriver); ok {
	altRefBootstrap := acd.AlternateReferenceBootstrap(backendPorts)
	altRef, err := StartReferenceProxy(ctx, pin, altRefBootstrap, acd.AlternateReferenceListenerPort())
	defer altRef.Stop(ctx)
	altSubjPort := freeTCPPort(t)
	altSubjAdminPort := freeTCPPort(t)
	altSubjCfg := acd.AlternateSubjectConfig(acd.AlternateReferenceListenerPort(), altSubjPort, backendPorts, altSubjAdminPort)
	altSubj, err := StartSubjectProxy(ctx, root, altSubjCfg, fmt.Sprintf("127.0.0.1:%d", altSubjAdminPort))
	defer altSubj.Stop()
	altRefAddr := altRef.ListenerAddr(acd.AlternateReferenceListenerPort())
	altSubjAddr := altSubj.ListenerAddr(acd.AlternateSubjectListenerName())
	altBytes, err := acd.DriveAlternate(ctx, altRefAddr, altSubjAddr)
	if err != nil {
		t.Fatalf("DriveAlternate: %v", err)
	}
	// Drive returns concatenated (ref, subj) bytes; diff is intrinsic.
	// The driver may also implement DriveAlternateCompareInBand(t) — but
	// for MVP we expect DriveAlternate to drive both sides and return ONE
	// concatenated byte stream that the runner doesn't diff (driver does
	// in-band per the SubjectAsserter precedent).
	_ = altBytes // driver-internal assertion
}
```

(The exact API surface decision — whether `DriveAlternate` returns concatenated bytes or each side separately — is settled at PLAN time: per fixture-0008's needs, `DriveAlternate` returns ONE byte slice that the driver itself produced after driving both ref+subj sides; the diff is intrinsic to the driver's logic. This mirrors `SubjectAsserter`'s "driver does in-band" pattern.)

- [ ] **Step 3: Run pre-existing fixtures to confirm zero regression**

```bash
go test ./test/differential/ -v 2>&1 | tail -50
```

Expected: 0000-0007b all PASS (none implement the new interfaces; the runner falls through to the standard path).

- [ ] **Step 4: Append PROGRESS Task 14 entry + Commit**

```bash
git add test/differential/runner_test.go docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: test/differential/runner_test.go — multi-listener + alternate-config branches"
```

SHA-fill follow-up.

*Anchored: SPEC §7.4, §9.2, Decision G.*

---

## Task 15: `test/fixtures/0008-listener-chain-match/` — fixture static files (configs + expectations + README + backends)

**Files:**
- Create: `test/fixtures/0008-listener-chain-match/envoy-go.yaml` (subject — primary)
- Create: `test/fixtures/0008-listener-chain-match/envoy.yaml` (reference — primary)
- Create: `test/fixtures/0008-listener-chain-match/envoy-go-c4.yaml` (subject — c4 variant)
- Create: `test/fixtures/0008-listener-chain-match/envoy-c4.yaml` (reference — c4 variant)
- Create: `test/fixtures/0008-listener-chain-match/expectations.yaml`
- Create: `test/fixtures/0008-listener-chain-match/README.md`
- Create: `test/fixtures/0008-listener-chain-match/backends/main.go`

This task lands the fixture's static surface — the four bootstrap variants (primary subject + primary reference + c4 subject + c4 reference), the expectations description, the README, and the backend program. The driver lands at Task 16 (separated to keep this task focused on YAML/text + the small backend program).

**Precondition:** Task 14 done.
**Artifact:** the seven files above (~700 LoC of YAML/text/Go).
**Acceptance:** `go build ./test/fixtures/0008-listener-chain-match/backends/...` succeeds; `cat envoy*.yaml | python3 -c "import yaml, sys; yaml.safe_load_all(sys.stdin)"` parses; `protojson` round-trip via `internal/bootstrap/bootstrap_test.go`'s `TestBootstrapLoadAllFixtures` passes; the four YAMLs differ ONLY in (a) STATIC vs STRICT_DNS clusters per ADR-0010 and (b) primary-vs-c4 chain_other presence.

- [ ] **Step 1: Author `envoy-go.yaml`** (subject — primary). Two listeners `l_test_a` + `l_test_b` each binding `127.0.0.1:0` plaintext (port 0 → OS-pick); both carry the SAME 3 `filter_chains[]` entries:

  ```yaml
  filter_chains:
  - name: chain_dstport_alpha
    filter_chain_match:
      destination_port: <l_test_a port — templated by driver>
    filters:
    - name: envoy.filters.network.tcp_proxy
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
        stat_prefix: tcp_dstport
        cluster: c_dstport
  - name: chain_srcprefix_loopback
    filter_chain_match:
      source_prefix_ranges:
      - { address_prefix: 127.0.0.1, prefix_len: 32 }
      source_ports: [<known_driver_port — templated>]
    filters:
    - name: envoy.filters.network.tcp_proxy
      typed_config: { "@type": ..., stat_prefix: tcp_srcprefix, cluster: c_srcprefix }
  - name: chain_other
    filter_chain_match: {}
    filters:
    - name: envoy.filters.network.tcp_proxy
      typed_config: { "@type": ..., stat_prefix: tcp_other, cluster: c_other }
  default_filter_chain:
    name: chain_default
    filters:
    - name: envoy.filters.network.tcp_proxy
      typed_config: { "@type": ..., stat_prefix: tcp_default, cluster: c_default }
  ```

  Plus 4 STATIC clusters (`c_dstport`, `c_srcprefix`, `c_other`, `c_default`) each with one endpoint pointing at a `127.0.0.1:<backend_port>` (templated by driver). NO TLS / NO SNI / NO listener_filters[] (this fixture exercises chain-match without listener filters; tls_inspector integration is unit-tested separately AND empirically pinned at Task 16 step 6 via §11.4 carry-forward).

- [ ] **Step 2: Author `envoy.yaml`** (reference — primary). Same dual-listener / 3-chain + default shape as the subject. STRICT_DNS clusters per ADR-0010 (`host.docker.internal:<backend_port>` with `dns_lookup_family: V4_ONLY`); `--concurrency 1` per ADR-0028.

- [ ] **Step 3: Author `envoy-go-c4.yaml`** (subject — c4 variant). Identical to primary but `chain_other` REMOVED so connection 4 (non-loopback to `l_test_b`) falls through to `chain_default`. Only `l_test_b` is needed in this variant (connection 4 hits `l_test_b` only).

- [ ] **Step 4: Author `envoy-c4.yaml`** (reference — c4 variant). Same shape as `envoy-go-c4.yaml` with STRICT_DNS clusters.

- [ ] **Step 5: Author `expectations.yaml`** — prose description of the 5-connection workload + the per-connection expectation table (which backend port hits, which chain selected, which precedence dimension demonstrated). Mirror SPEC §7.4 Table.

- [ ] **Step 6: Author `README.md`** — fixture purpose (differential per-connection chain-selection equivalence), the STATIC-vs-STRICT_DNS divergence, the dual-listener + c4-variant rationale, the chain-match-precedence demonstration (connection 5 satisfies two chains' dimensions; `destination_port` wins over `source_prefix_ranges` per §11.3 empirical pin), the cross-reference to BEHAVIOR_CONTRACT `## Listener filters` (introduced at 07.2 phase-done).

- [ ] **Step 7: Author `backends/main.go`** — small Go program (~60 LoC) that:
  - Reads `BACKEND_PORT` env var.
  - `net.Listen("tcp", "0.0.0.0:"+port)`.
  - Accept loop: read until EOF, write back the listener address (`fmt.Sprintf("%s\n", ln.Addr().String())`), close conn.
  - Used by the runner to spawn 5 backends per fixture run; each backend's response body is its own listener address — distinct per backend → distinct per chain.

- [ ] **Step 8: Run `protojson` round-trip + build the backend**

```bash
go build ./test/fixtures/0008-listener-chain-match/backends/...
go test ./internal/bootstrap/... -run TestBootstrapLoadAllFixtures -v 2>&1 | tail -10
```

Expected: backend builds; bootstrap round-trip passes for all four YAMLs.

- [ ] **Step 9: Append PROGRESS Task 15 entry + Commit**

```bash
git add test/fixtures/0008-listener-chain-match/envoy*.yaml test/fixtures/0008-listener-chain-match/expectations.yaml test/fixtures/0008-listener-chain-match/README.md test/fixtures/0008-listener-chain-match/backends/ docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: test/fixtures/0008-listener-chain-match — primary + c4 bootstraps + backends"
```

SHA-fill follow-up.

*Anchored: SPEC §4.3, §7.4 (dual-listener + 5-connection workload + c4 variant), §9 (differential fixture), Decision G.*

---

## Task 16: `test/fixtures/0008-listener-chain-match/driver/` — Driver implementing MultiListenerDriver + AlternateConfigDriver; resolves §11.4 carry-forward [Decision K]

**Files:**
- Create: `test/fixtures/0008-listener-chain-match/driver/driver.go`
- Create: `test/fixtures/0008-listener-chain-match/driver/driver_test.go`
- Modify: `test/differential/runner_test.go` (blank-import the driver package)
- Modify: `docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md` (§11.4 carry-forward fill)

This task implements fixture-0008's driver, exercising both `MultiListenerDriver` (for the dual-listener primary path) and `AlternateConfigDriver` (for the c4-variant connection 4). Also resolves SPEC §11.4 carry-forward per Decision K — the executor scrapes verbatim Envoy v1.37.2 stats output for the `tls_inspector`-populated ALPN → `application_protocols` chain-match interaction, and pastes it into SPEC §11.4 (replacing the carry-forward placeholder). The pinned shape is added to PROGRESS for cross-checking at Task 17 step 1 when the BEHAVIOR_CONTRACT `## Listener filters` initial population happens.

**Precondition:** Tasks 2-15 done.
**Artifact:** `driver/driver.go` (~400 LoC) + `driver/driver_test.go` (~120 LoC); SPEC §11.4 update.
**Acceptance:** `go test ./test/differential/ -run 'Test.*0008' -v` passes; per-connection backend-port routing byte-equal across envoy-go and reference Envoy v1.37.2; SPEC §11.4 carries the verbatim Envoy output (replacing the placeholder); no test-only Go code in production packages.

- [ ] **Step 1: Author `driver/driver.go`**. Implements `fixture.Driver` (single-listener compat: `SubjectListenerName() = "l_test_a"`, `ReferenceListenerPort() = 15008`); implements `fixture.MultiListenerDriver` (`SubjectListenerNames() = ["l_test_a", "l_test_b"]`; `ReferenceListenerPorts() = [15008, 15009]`); implements `fixture.AlternateConfigDriver` for the c4 variant; implements `fixture.BackendKindAware` returning a custom `BackendKind` for the listener-address-echo backend (or extends `BackendKind` enum if needed).

  `BackendCount() = 5`. `ReferenceBootstrap`/`SubjectConfig` template the YAML loaded from the fixture root with the OS-allocated backend ports + the OS-allocated `<known_driver_port>` (driver pre-allocates one source port via `net.Listen("tcp", "127.0.0.1:0")` then closes the listener, captures the port number).

  `DriveSubjectMulti(ctx, addrs)` issues 4 sequential TCP connections (1, 2, 3, 5 per SPEC §7.4) routed across both listeners — connection 1 to `l_test_a` from non-loopback; connection 2 to `l_test_b` from `127.0.0.1` with the pre-allocated source port; connection 3 to `l_test_b` from non-loopback; connection 5 to `l_test_a` from `127.0.0.1` with the pre-allocated source port. Captures each response (the backend address echo); returns the concatenated bytes.

  `DriveReferenceMulti(ctx, addrs)` issues the same 4 connections against the reference proxy (the addrs map keys "l_test_a" / "l_test_b" → reference addrs).

  `AlternateReferenceBootstrap` / `AlternateSubjectConfig` template the c4 YAML; `DriveAlternate(ctx, refAddr, subjAddr)` issues connection 4 (l_test_b non-loopback) against both ref and subj, captures both responses, asserts byte-equality in-band, returns concatenated bytes.

  Source-bind for connections 2 and 5 uses `net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: <known_driver_port>}}`.

- [ ] **Step 2: Author `driver/driver_test.go`** — distribution-/expectation-assertion unit tests for the driver's deterministic per-connection routing.

- [ ] **Step 3: Modify `test/differential/runner_test.go`** — append blank-import:

```go
_ "github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver"
```

- [ ] **Step 4: Run fixture 0008**

```bash
go test ./test/differential/ -run 'Test.*0008' -v 2>&1 | tail -50
```

Expected: PASS. Each connection's response body is byte-equal across subject and reference. If a connection's body differs, debug per `superpowers:systematic-debugging` (likely cause: chain-match algorithm bug, or chain-spec build error, or the driver's source-bind not taking effect).

- [ ] **Step 5: Re-run pre-existing fixtures to confirm zero regression**

```bash
go test ./test/differential/ -v 2>&1 | tail -30
```

Expected: 0000-0007b PASS unchanged; 0008 PASS new.

- [ ] **Step 6: Resolve SPEC §11.4 carry-forward (Decision K)**. Spawn a real Envoy v1.37.2 container with a TLS bootstrap that includes `tls_inspector` listener filter + multi-chain `application_protocols: [h2]` / `application_protocols: [http/1.1]` matching against per-chain HCM filters with forced `codec_type`. Issue an HTTPS-h2 connection from a Go probe with `NextProtos = ["h2"]`. Capture `/stats?filter=tcp_(h2|h1).downstream_cx_total` output verbatim. Paste into SPEC §11.4 replacing the carry-forward placeholder block; the resolved block is structured identically to §11.1-§11.3 (verbatim envoy output + conclusions). The probe scaffolding can live in `/tmp/envoy-07.2-impl-empirical/` (NOT committed) per the SPEC's empirical-pin convention.

- [ ] **Step 7: Append PROGRESS Task 16 entry + Commit**

```bash
git add test/fixtures/0008-listener-chain-match/driver/ test/differential/runner_test.go docs/envoy-go/phases/07.2-listener-chain-completion/SPEC.md docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: test/fixtures/0008-listener-chain-match/driver — multi-listener + alternate-config; SPEC §11.4 carry-forward resolved [Decision K]"
```

SHA-fill follow-up.

*Anchored: SPEC §4.3, §7.4 (5-connection workload), §9.1 (equivalence claim), §11.4 (carry-forward — resolved at this task per Decision K), §15.4 (differential fixture).*

---

## Task 17: BEHAVIOR_CONTRACT in-place edit + closing six-gate sweep [ADR-0077, ADR-0078, ADR-0079, ADR-0080, ADR-0081, ADR-0082, ADR-0083]

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (insert NEW `## Listener filters` top-level section between `## HTTP filter chain` and `## xDS wire state machine`; amend `## TCP proxy "Does not yet apply to"`; amend `## TLS "Scope boundaries"`; add new row to `## Equivalence Matrix`)
- Modify: `docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md` (closing entry quoting all six gates' command outputs verbatim)
- Modify: `docs/envoy-go/STATE.md` (NOT touched at this commit — see Refinement; advanced by the verification session)
- Modify: `docs/envoy-go/ROADMAP.md` (NOT touched at this commit — see Refinement; flipped by the REVIEW session's phase-done commit)

The BEHAVIOR_CONTRACT in-place edit lands at THIS commit (the implementation session's last commit) per the 06.1 / 06.2 / 07.1 in-place-edit-at-impl-session-last-commit timing convention. The four §11 empirical-pin blocks land verbatim in `## Listener filters`'s `### Empirical evidence (default_filter_chain fallback)` / `### Empirical evidence (empty-match-vs-default)` / `### Empirical evidence (precedence-ordering)` / `### Empirical evidence (tls_inspector ALPN)` subsections per SPEC §13.1. The §11 block + the §13 block are paste-verbatim-synchronized (the §11.4 block was filled at Task 16; the rest at SPEC time).

**Precondition:** Tasks 1–16 done; pre-existing fixtures still green; new fixture-0008 green; SPEC §11.4 resolved.
**Artifact:** BEHAVIOR_CONTRACT extended in place; PROGRESS quotes all six gates.
**Acceptance:** all six phase-done gates (a–f) green per SPEC §3 (gate (f) defers to REVIEW session); the boundary grep at step 6 surfaces no third-party listener-filter library; the four-empirical-pin grep at step 7 confirms paste-verbatim-synchronization with SPEC §11.

- [ ] **Step 1: Insert NEW `## Listener filters` top-level section into BEHAVIOR_CONTRACT.md** (between `## HTTP filter chain` ending around line 730 and `## xDS wire state machine` starting at line 250 — the file's section ordering may not be linear; verify the insertion point at execution time. Per SPEC §13.1's parenthetical: "the new `## Listener filters` section is placed after `## HTTP filter chain` and before `## xDS wire state machine` per the planner's discretion").

Use the SPEC §13.1 verbatim block (the one starting with `## Listener filters` heading and ending with the `### Does not yet apply to` block). Paste the four §11 empirical-pin blocks verbatim into the four `### Empirical evidence (...)` subsections (§11.1, §11.2, §11.3 from SPEC time; §11.4 filled at Task 16).

- [ ] **Step 2: Amend `## TCP proxy "Does not yet apply to"` (line 360)**:
- Remove "Filter chain matching (`filter_chain_match` non-empty) — phase 07."
- Replace "Multiple filters in a chain — phase 07." with "Multiple network filters in a single filter_chain (e.g., chained `redis_proxy + tcp_proxy`) — Network filters family. Multiple listener filters in a `listener_filters[]` pipeline IS supported as of 07.2 — see `## Listener filters`."

- [ ] **Step 3: Amend `## TLS "Scope boundaries"` (line 405)**:
- Remove "ALPN-driven filter-chain selection" (now in scope per ADR-0081's `application_protocols` priority slot).
- Remove "non-SNI filter-chain match fields" (now in scope per ADR-0081).
- Remove "`Listener.default_filter_chain`" (now in scope per ADR-0080).
- Remove "`listener_filters` (still silently skipped)" (now in scope per ADR-0079).
- Add forward-pointer: "See `## Listener filters` for the listener-side filter primitives."
- Preserve "ALPN-driven codec selection inside `Filter.Handle`" if currently absent — ADR-0050 is NOT superseded; clarify that ALPN-codec-dispatch (ADR-0050) and ALPN-chain-match (ADR-0083 + ADR-0081) coexist orthogonally.

- [ ] **Step 4: Add new row to `## Equivalence Matrix` (per SPEC §13.2)**:

```
| Listener filters       | Per-connection chain-selection equivalence: which       | Differential covers chain-selection   |
|                        | filter_chain is dispatched is byte-equal across         | only (which backend each connection   |
|                        | envoy-go and reference Envoy. Verified via per-         | is routed to). Listener-filter        |
|                        | connection backend-port routing in fixture 0008.        | internal byte-level behavior          |
|                        | Chain-match precedence ordering, default_filter_chain   | (e.g., tls_inspector parser output)   |
|                        | fallback semantics, and empty-match-vs-default          | is unit-tested only.                  |
|                        | resolution are verbatim-pinned at the ENVOY_TARGET SHA. |                                       |
```

- [ ] **Step 5: Run gate (a) sweep — all differential fixtures green**

```bash
go test -count=1 ./test/differential/... -v 2>&1 | tee /tmp/07.2-gate-a.log
```

Expected: all 10 fixtures (`0000` through `0008`) PASS. Quote the last 30 lines into PROGRESS.

- [ ] **Step 6: Run gate (c) sweep — h2spec at 53/53 PASS at the ADR-0051 pin**

```bash
docker run --rm summerwind/h2spec@sha256:<pin from CONFORMANCE_PINS.md> -h 127.0.0.1 -p <subject port> -t -s 1.1 -e <sections per CONFORMANCE_PINS.md>
```

Expected: 53/53 PASS, 0 fail. Quote into PROGRESS.

- [ ] **Step 7: Run gate (d) sweep — all 10 fuzzers clean for 30s each**

```bash
go test -fuzz=FuzzBootstrapLoad -fuzztime=30s ./internal/bootstrap/...
go test -fuzz=FuzzTcpProxyFilter -fuzztime=30s ./internal/filter/tcpproxy/...
go test -fuzz=FuzzTLSContextParse -fuzztime=30s ./internal/tls/...
go test -fuzz=FuzzHCMConfigParse -fuzztime=30s ./internal/filter/hcm/...
go test -fuzz=FuzzFrameStream -fuzztime=30s ./internal/filter/hcm/h2/...
go test -fuzz=FuzzHPACKDecode -fuzztime=30s ./internal/filter/hcm/h2/...
go test -fuzz=FuzzPromTextFormat -fuzztime=30s ./internal/stats/...
go test -fuzz=FuzzAccessLogFormat -fuzztime=30s ./internal/accesslog/...
go test -fuzz=FuzzFilterChainParse -fuzztime=30s ./internal/filter/http/...
go test -fuzz=FuzzFilterChainMatch -fuzztime=30s ./internal/listener/listenerfilter/...
```

Expected: 10 fuzzers (9 from prior phases + new `FuzzFilterChainMatch`) clean.

- [ ] **Step 8: Run gate (e) sweep — vet + lint + test -race**

```bash
go vet ./...                                                   2>&1 | tee /tmp/07.2-vet.log
golangci-lint run ./...                                        2>&1 | tee /tmp/07.2-lint.log
go test -race -count=1 ./...                                   2>&1 | tee /tmp/07.2-race.log
```

Expected: all clean. Quote each.

- [ ] **Step 9: Boundary grep — no third-party listener-filter / chain-match library**

```bash
grep -rE 'github.com/.*listener.*filter|github.com/.*chain.*match' . --include='*.go' --include='go.mod' --include='go.sum' | grep -v 'envoyproxy/go-control-plane' | grep -v 'envoy-go'
```

Expected: zero matches (per SPEC §16 final acceptance bullet "No third-party listener-filter or chain-match library is imported").

- [ ] **Step 10: Empirical-pin grep — confirm BEHAVIOR_CONTRACT carries verbatim §11 blocks**

```bash
grep -A5 '^### Empirical evidence (default_filter_chain fallback)' docs/envoy-go/BEHAVIOR_CONTRACT.md
grep -A5 '^### Empirical evidence (empty-match-vs-default)' docs/envoy-go/BEHAVIOR_CONTRACT.md
grep -A5 '^### Empirical evidence (precedence-ordering)' docs/envoy-go/BEHAVIOR_CONTRACT.md
grep -A5 '^### Empirical evidence (tls_inspector ALPN)' docs/envoy-go/BEHAVIOR_CONTRACT.md
```

Expected: each returns the corresponding §11 block's first 5 content lines verbatim (the `tcp.tcp_default.downstream_cx_total: 1` / `tcp.tcp_loopback.downstream_cx_total: 1` / `tcp.tcp_empty.downstream_cx_total: 1` / `tcp.tcp_dstport.downstream_cx_total: 1` stats output lines + the post-Task-16 §11.4 fill).

- [ ] **Step 11: Append closing PROGRESS entry**

```markdown
## Task 17 — BEHAVIOR_CONTRACT in-place edit + closing six-gate sweep [ADR-0077, ADR-0078, ADR-0079, ADR-0080, ADR-0081, ADR-0082, ADR-0083]

**Commits:** TBD — this task's commit
**Notes:** Landed BEHAVIOR_CONTRACT.md ## Listener filters section + four §11 empirical-pin blocks verbatim + ## TCP proxy + ## TLS amendments + Equivalence Matrix row addition. Six-gate sweep all green (gate (f) defers to REVIEW session). Total fuzzer count post-07.2 is 10. No third-party listener-filter library imported. SPEC §11.4 carry-forward filled at Task 16; the §11 block in SPEC and the §13 block in BEHAVIOR_CONTRACT are paste-verbatim-synchronized.
**Outputs (verbatim — gate (a) tail; gate (c) summary; gate (d) summary; gate (e) all clean; gate boundary grep zero):**
\`\`\`
$ go test -count=1 ./test/differential/... -v | tail -30
<verbatim>
$ docker run ... summerwind/h2spec ... | tail -10
<verbatim — 53/53 PASS>
$ go test -fuzz=FuzzFilterChainMatch -fuzztime=30s ./internal/listener/listenerfilter/ | tail -5
<verbatim — clean>
$ go vet ./...
<verbatim — empty (clean)>
$ golangci-lint run ./...
<verbatim — empty (clean)>
$ go test -race -count=1 ./... | tail -20
<verbatim — all PASS>
$ grep -rE 'github.com/.*listener.*filter|...' . --include='*.go' --include='go.mod' --include='go.sum' | grep -v envoyproxy/go-control-plane | grep -v envoy-go
<verbatim — empty (zero matches)>
\`\`\`
```

- [ ] **Step 12: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/phases/07.2-listener-chain-completion/PROGRESS.md
git commit -m "phase 07.2: BEHAVIOR_CONTRACT.md ## Listener filters + closing sweep [ADR-0077, ADR-0078, ADR-0079, ADR-0080, ADR-0081, ADR-0082, ADR-0083]"
```

SHA-fill follow-up.

- [ ] **Step 13: Confirm phase-07.2 readiness for state-5 transition (do NOT advance STATE — that's the verification session per BOOTSTRAP §5)**

The implementation session ends with Task 17 committed on `phase/07.2-listener-chain-completion-impl`. STATE advancement through 4 → 5 → 6 is per-session work, not this task's responsibility. The Refinement section names what the verification + REVIEW sessions land on top of this commit.

*Anchored: SPEC §1 #11, §3 (six-gate phase-done), §4.4 (ROADMAP/STATE/PROGRESS lifecycle), §13 (BEHAVIOR_CONTRACT additions), §16 (full acceptance checklist), and BOOTSTRAP §5.3 (commit-message-completeness), §7.5 (six-gate sweep).*

---

## Refinement

This section absorbs the conventions that the 06.2 / 07.1 PLAN's Refinement sections codified for execution-time consistency. Every item below applies to phase 07.2 unless explicitly noted.

**SHA-fill follow-up convention (per phase-02 / 03 / 04 / 05.1 / 05.2 / 06.1 / 06.2 / 07.1 precedent).** Every task's commit lands the task's main change; immediately after, a follow-up tiny commit `phase 07.2: PROGRESS SHA-fill for Task N` updates that task's PROGRESS.md `**Commits:**` line with the just-landed short SHA. The follow-up commit's body is empty; its title is the only line. Two commits per task; the executor MUST NOT skip the follow-up.

**BEHAVIOR_CONTRACT in-place edit lands at the Task 17 commit (per ADR-0052).** The `## Listener filters` section addition + the `## TCP proxy` + `## TLS` amendments + the `## Equivalence Matrix` row addition land at Task 17's commit, NOT at any earlier task's commit. Per ADR-0052 the in-place edit is authorised; per SPEC §4.4 the timing is "at the phase-done commit" — but per BOOTSTRAP §5 step 6 the phase-done commit is the REVIEW session's, NOT the implementation session's; the BEHAVIOR_CONTRACT edit anticipates the REVIEW session by landing at Task 17 (the implementation session's last commit) so the verification session can grep-check the edit before REVIEW runs. Mirrors the 06.1 / 06.2 / 07.1 PLAN's identical convention.

**Empirical-pin blocks land verbatim at Task 17 (NOT scraped at Task 17; §11.1–§11.3 already pinned at SPEC time; §11.4 was scraped at Task 16).** Per SPEC §11 + §13.1, three empirical-pin blocks were already executed at SPEC time (against reference Envoy v1.37.2 SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`) and pinned verbatim in SPEC §11.1–§11.3. The fourth (§11.4) was carry-forward to impl time per Decision K and resolved at Task 16. Task 17's job is to PASTE the §11 blocks (all four) into BEHAVIOR_CONTRACT.md `## Listener filters` verbatim — NOT to re-scrape. The §11 block + the §13 block are paste-verbatim-synchronized; future image bumps per `ENVOY_TARGET.md`'s refresh procedure that alter any of the four shapes require updating BOTH locations in the same commit, mirroring the 06.1 / 06.2 / 07.1 paste-verbatim discipline.

**Multi-file refactor handling for Tasks 9–10 (parser-then-dispatch).** Tasks 9 (validateFilterChainMatch rewrite + default_filter_chain + listener_filters[]) + 10 (acceptLoop + serveConnection refactor) form a parser-then-dispatch sequence. The package may be temporarily not-buildable between Tasks 9 and 10 (inclusive); buildability is restored at Task 10 (the dispatch refactor finishes the surface) and the differential gates (b) re-runnable from Task 11 onward. Two execution patterns are permitted:
- **Pattern A (recommended for review-granularity):** two separate commits (one per task); inter-task `go build` failures are documented in each PROGRESS entry's "Notes" field as "package does not yet build; restored at Task 10"; the executor confirms buildability is restored at the targeted task before SHA-fill follow-up.
- **Pattern B (recommended for atomic-refactor preference):** Tasks 9 + 10 batched as one commit with two PROGRESS entries against that one commit; the executor still SHA-fills against the single bundled commit per the 06.2 / 07.1 PLAN's allowance for related multi-file refactors when each component is independently undebuggable.

The PLAN does not prescribe which pattern; the executor picks based on review preference at execution time.

**ROADMAP row 07.2 → in-progress at the SPEC commit (already landed); → done at the phase-done commit. Parent row 07 → done AT THE SAME phase-done commit.** Per BOOTSTRAP §4.1 invariant 3: at the SPEC commit (already landed at master `bb5f437`, before this PLAN commit), row 07.2 flipped `planned → in-progress` — the SPEC-authoring session did this. Per SPEC §4.4 + parent SPEC §5: at the phase-done commit (the REVIEW session's lifecycle-state-6 commit, NOT Task 17), row 07.2 flips `in-progress → done` AND parent row 07 flips `in-progress → done` AT THE SAME commit (the 05/05.1/05.2 + 06/06.1/06.2 closure pattern). Task 17's commit deliberately does NOT touch ROADMAP.md; the anticipated text is recorded in the PROGRESS Task 17 entry but lands at the REVIEW session's phase-done commit. The phase-done commit-message body explicitly names BOTH row transitions per SPEC §3's commit-subject template.

**ADR-numbering monotonicity discipline (ADR-0077..ADR-0083 contiguous).** Per ADR-0004's autonomous-numbering rule, the planner verified at PLAN-write time that the DECISIONS.md tail is `ADR-0076`; phase 07.2's seven ADRs land at ADR-0077..ADR-0083 (contiguous block). Per `## ADRs introduced by this plan` above, the commit-time ordering (Task 1 / Task 2 / Task 4 / Task 5 / Task 5 / Task 9 / Task 1) produces non-monotonic ADR-number-vs-commit-order at three places (0077, 0083, 0079, 0082, 0080, 0081, 0078), permitted per SPEC §10 and the 05.2 + 06.1 + 06.2 + 07.1 precedents. The contiguous-block discipline is preserved (no gaps); each ADR's `Lands-in-task` field records the in-task anchoring. The Task 1 step 1 precondition re-verifies the tail; if ADR-0076 has been superseded by a mid-PLAN-authoring ADR, every task's ADR reference shifts uniformly.

**Commit-message-completeness check (per BOOTSTRAP §5.3).** Each task's commit message names the ADR(s) introduced in that task (in `[ADR-NNNN]` square-bracket form per the phase-04/05.1/05.2/06.1/06.2/07.1 convention). The Task 17 closing commit (per Step 12) names ALL SEVEN ADRs in the bracketed list — `[ADR-0077, ADR-0078, ADR-0079, ADR-0080, ADR-0081, ADR-0082, ADR-0083]` — so a `git log --grep='ADR-007[7-9]\\|ADR-008[0-3]'` query surfaces every authoring task plus the closing task. The phase-done commit (REVIEW session's) carries the same bracketed list per SPEC §3.

**Six-gate local sweep at Task 17 (per BOOTSTRAP §7.5; SPEC §3).** Gates (a) / (b) / (c) / (d) / (e) all run at Task 17; gate (f) defers to REVIEW. The PROGRESS Task 17 entry quotes each gate's last-30-lines output verbatim. The Task 17 step 9 boundary grep + the step 10 four-empirical-pin grep are SPEC §16-anchored acceptance bullets that the verification session re-runs.

**No-third-party-listener-filter-library acceptance (per ADR-0079 + SPEC §16).** Task 17 step 9's grep is the gate; the executor CONFIRMS no third-party listener-filter / chain-match library import lands in any production-code path. Test-side use is also forbidden. The grep applies uniformly across `_test.go` and production code.

**Mid-execution split valve.** Per `## Scope check` triggering re-evaluation: if cumulative landed-LoC by Task 12 exceeds 9000, invoke `superpowers:systematic-debugging`. Per `BOOTSTRAP_PROMPT.md` §6.1's secondary trigger, if any single task's sub-steps blow past 15 (vs the recommended 10 trigger; 15 reflects the framework's structural complexity), the executor splits per §6.2 with a new ADR. The natural axis is 07.2.1 (framework + tls_inspector + manager refactor + boot wiring; Tasks 1–12) and 07.2.2 (harness extensions + fixture-0008 + closing sweep; Tasks 13–17) — with the caveat from the Scope check argument #1 that 07.2.1 has vacuous gate (a) and would need a placeholder fixture probe.

**Empirical-pin discipline (ENVOY_TARGET image bumps).** The four §11 empirical-pin blocks are verbatim-paste against reference Envoy v1.37.2 (server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`). If a future phase bumps `ENVOY_TARGET.md`'s pin, ALL FOUR pin blocks in BOTH SPEC §11 AND `BEHAVIOR_CONTRACT.md ## Listener filters` must be re-scraped + updated in the SAME commit (the pin-bump phase's commit). The §11 block + the §13 block are synchronized; no drift permitted.

**Decision K resolution at Task 16 (not Task 17).** Per Decision K: SPEC §11.4 carry-forward (`tls_inspector`-populated ALPN feeds `application_protocols` chain-match) is resolved at the FIRST listener-filter integration task that has a real probe driver — i.e., Task 16 (the fixture-0008 driver). The executor scrapes Envoy stats verbatim, updates SPEC §11.4 (replacing the placeholder), and the resolved block is paste-verbatim into BEHAVIOR_CONTRACT.md at Task 17 step 1. The 06.1 SN4 + 06.2 default-format empirical-pin patterns are the precedents.

**Listener-filter pipeline determinism (per SPEC §15.7).** `go test -race ./...` at Task 17 step 8 stresses (per SPEC §5.6): N goroutines accepting connections on the same listener, each running its own pipeline (no shared mutable state); concurrent `ListenerFilterRegistry.Lookup` calls from N listener-manager constructors at boot; `peekerConn.Peek` + `peekerConn.Read` interleaved on the same connection (single-goroutine invariant means we test sequential ON the same goroutine — but Task 12's `integration_test.go` exercises concurrent connections + pipelines); the registry's `Freeze()` invariant: post-Freeze `Register` panics; concurrent `Freeze` calls are idempotent. Unit tests in `pipeline_test.go` + `registry_test.go` + `chainmatch_test.go` exercise each. Differential fixture `0008-listener-chain-match` indirectly stresses end-to-end concurrency under load (5 connections × 2 listeners × 2 sides = 20 concurrent connection-dispatches per fixture run).

---

## Post-plan handoff: state advancement + worktree cleanup (session-exit duties)

This section is the plan-authoring session's exit contract, not an executable task.

After the executing session commits Task 17 on `phase/07.2-listener-chain-completion-impl`:

1. **Fast-forward merge to master.** Per ADR-0003:
   ```bash
   cd /home/esa/git/envoy-go   # master worktree
   git merge --ff-only phase/07.2-listener-chain-completion-impl
   ```
2. **The verification session** (next-fresh from the implementation session) re-runs all six gates per BOOTSTRAP §7.5 and advances STATE to lifecycle-state 5 with `next-skill: superpowers:requesting-code-review`. Verification commits `phase 07.2: STATE.md → lifecycle-state 5` on master.
3. **The REVIEW session** (next-fresh from verification) writes `docs/envoy-go/phases/07.2-listener-chain-completion/REVIEW.md` per BOOTSTRAP §5 state 5 → 6. The REVIEW session's phase-done commit (per SPEC §3 commit-subject template):
   - Flips ROADMAP row 07.2 → `done`.
   - Flips ROADMAP parent row 07 → `done` AT THE SAME COMMIT (per parent SPEC §5 closure pattern; mirrors 05/05.1/05.2 + 06/06.1/06.2).
   - Lands the BEHAVIOR_CONTRACT verification block (a re-grep that the Task 17 edit landed correctly).
   - Advances STATE to phase 08 (`active-phase: 08-admin-api-and-drain`; `lifecycle-state: 0` per the §5 state machine; `next-skill: superpowers:brainstorming`) at the SAME phase-done commit.
   - Commit message: `phase 07.2: phase-done — listener-chain-completion lands; ROADMAP rows 07.2 + 07 → done [ADR-0077, ADR-0078, ADR-0079, ADR-0080, ADR-0081, ADR-0082, ADR-0083]`.

**No part of this section is done by Task 17.** It lives here so the plan-authoring session knows where to leave STATE after its own commit, and so the executing session has clear context for its exit.

This plan-authoring session's own exit contract:

1. After plan-document-reviewer approves (`## Plan review loop` below), commit `PLAN.md` on `phase/07.2-listener-chain-completion-plan`.
2. Update `docs/envoy-go/STATE.md` on the same branch: `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development` (per ADR-0005 and per the user's persistent preference for subagent-driven execution recorded in MEMORY.md), `next-skill-scope: <execute PLAN.md per the 17-task sequence; create worktree .worktrees/phase-07.2-listener-chain-completion-impl per ADR-0003>`, `last-commit: TBD` (the SHA-fill follow-up commit lands the actual SHA per the phase-02..07.1 SHA-fill precedent).
3. Fast-forward `master` to `phase/07.2-listener-chain-completion-plan` per ADR-0003 (parent session's responsibility).
4. Worktree for the next session: `.worktrees/phase-07.2-listener-chain-completion-impl` on branch `phase/07.2-listener-chain-completion-impl` (recommended per `## Execution preconditions` #1).
5. Exit clean.

---

## Plan review loop (invoked at end of plan-authoring session)

Per `superpowers:writing-plans` and ADR-0005: after this PLAN.md is written, dispatch the `plan-document-reviewer` subagent with the PLAN.md path + the SPEC.md path. If the reviewer returns approved → commit PLAN.md + STATE advancement (state 2 → state 3 on `phase/07.2-listener-chain-completion-plan`). If the reviewer returns changes-requested → address feedback in place, re-dispatch (max 3 iterations per ADR-0005 + skill guidance); on iteration 3 without approval, exit blocked per `BOOTSTRAP_PROMPT.md` §5 deviations.

The reviewer's scope:

- Does the PLAN cover every SPEC §4 deliverable? (`internal/listener/listenerfilter/{doc,types,callbacks,registry,pipeline,chainmatch,fuzz_test}.go` and the test pairs; `internal/listener/listenerfilter/tls_inspector/{doc,tls_inspector,parser,proto}.go` + tests; `internal/listener/manager.go` + `manager_test.go` + `listener_test.go` + new `integration_test.go` modifications; `cmd/envoy-go/main.go` + `internal/bootstrap/bootstrap.go` boot wiring; `test/differential/fixture/fixture.go` + `test/differential/runner_test.go` extensions; differential fixture 0008 in full; runner registration; seven ADRs ADR-0077..ADR-0083; `BEHAVIOR_CONTRACT.md ## Listener filters` in-place edit + amendments to `## TCP proxy` and `## TLS` + Equivalence Matrix row.)
- Does the PLAN settle every SPEC §14 deferred decision (15 items A–O)? See `## Settled SPEC §14 deferred decisions` above.
- Does the PLAN mitigate every SPEC §15 testing-strategy item with a task-level step? (15.1 unit tests for `internal/listener/listenerfilter/` → Tasks 2–6; 15.2 `tls_inspector/{parser,tls_inspector,proto}_test.go` → Tasks 7–8; 15.3 `internal/listener/{manager,listener,integration}_test.go` extensions → Tasks 9, 10, 12; 15.4 differential fixture 0008 → Tasks 15–16; 15.5 h2spec re-run → Task 17 step 6; 15.6 fuzzers → Task 6 + Task 17 step 7; 15.7 race detector + lint → Task 17 step 8.)
- Does the PLAN preserve the empirical-pin discipline? (SPEC §11.1–§11.3 blocks land verbatim at Task 17 step 1; §11.4 carry-forward resolved at Task 16 step 6 then paste-verbatim into BEHAVIOR_CONTRACT at Task 17 step 1; the §11 block + the §13 block are synchronized — no drift.)
- Does the PLAN preserve the carry-forward triage from SPEC §10? (None except §11.4, which Task 16 resolves; 07.1 REVIEW Minors stay separate; 04 + 05.x deferred items unchanged in disposition.)
- Are tasks atomic (one logical commit each, 2–5 minutes per step except the well-annotated longer ones — Task 5 chainmatch.go algorithm, Task 9 manager.go validateFilterChainMatch + default_filter_chain + listener_filters[] parsing, Task 10 dispatch refactor, Task 16 fixture 0008 driver + §11.4 empirical pin, Task 17 closing sweep)?
- Does the ADR number sequence match the verified DECISIONS.md tail? (ADR-0076 → ADR-0077..0083; non-monotonic mapping by topic-vs-first-use-order documented above.)
- Is the LoC estimate honest and does the scope-check argument hold? (Per `## Scope check`: ~6500 LoC effective, 17 tasks, no further coherent split axis exists without vacuous-gate-(a) anti-pattern; per phase-04 / 05.1 / 05.2 / 06.1 / 06.2 / 07.1 precedent, one-sub-phase shipment is correct.)
- Does the import topology stay clean? (`internal/listener/listenerfilter/` is a near-leaf importing only stdlib + `google.golang.org/protobuf` + `tls_inspector/v3` proto; `internal/listener/manager.go` imports `internal/listener/listenerfilter/`; `cmd/envoy-go/main.go` + `internal/bootstrap/bootstrap.go` import `internal/listener/listenerfilter/tls_inspector/`; no third-party listener-filter library; the boundary grep at Task 17 step 9 enforces.)
- Are the seven ADRs internally consistent? (ADR-0077's scope decision matches the SPEC commit's ROADMAP edit; ADR-0078's enumeration matches SPEC §5.7's clause-by-clause table; ADR-0079's interface shape matches Task 2's types.go; ADR-0080's default-chain semantics match Task 5's SelectChain; ADR-0081's algorithm matches Task 5's chainmatch.go; ADR-0082's [1s, 60s] envelope matches Task 9 step 4 + Task 4's Pipeline.Run; ADR-0083's coexistence claim is non-superseding — ADR-0050 stays in force.)
- Are the empirical pins faithfully transcribed? (Task 17 step 10 grep-verifies; SPEC §11.1's tcp.tcp_default + tcp.tcp_loopback stats, §11.2's tcp.tcp_empty + tcp.tcp_default stats, §11.3's tcp.tcp_dstport + tcp.tcp_srcprefix stats, §11.4's Task-16-resolved tcp.tcp_h2 + tcp.tcp_h1 stats all paste-verbatim into BEHAVIOR_CONTRACT.)
- Is the harness extension (Task 13's MultiListenerDriver + AlternateConfigDriver) minimal and orthogonal? (Yes — the existing Driver interface is UNCHANGED; both new interfaces are optional; pre-existing fixtures (0000-0007b) don't implement either and fall through to the standard runner path; the alternate-config "driver does in-band diff" pattern mirrors SubjectAsserter from 0007b.)
- Does the fixture-0008 design honor SPEC Decision G's "cannot revisit dual-listener-with-c4-variant shape without ADR" clause? (Yes — Task 15 implements the dual-listener primary + c4-variant; Task 16 drives 4 connections via MultiListenerDriver and 1 connection via AlternateConfigDriver; the shape is preserved.)

