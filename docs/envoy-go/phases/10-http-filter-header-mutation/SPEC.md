# Phase 10 — HTTP filter `envoy.filters.http.header_mutation` (`internal/filter/http/header_mutation/`, differential fixture `0012-http-header-mutation`, `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.header_mutation` extension)

**Phase id:** `10`
**Slug:** `10-http-filter-header-mutation`
**Status:** `in-progress` (SPEC stage; ROADMAP row `10` flips `planned → in-progress` at this commit per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3)
**Produced by:** `superpowers:writing-plans` (lifecycle-state 2 → 3; transcribes the brainstorm-close BRAINSTORM.md (`docs/envoy-go/phases/10-http-filter-header-mutation/BRAINSTORM.md`) §§1–12 into formal SPEC shape, executing the five §11 empirical-pin obligations against reference Envoy v1.37.2 in-session per ADR-0004)
**Depends on:** phase 09 (done at master `c7de495` — 09 phase-done close + N-1 carry-forward retrospective; SHA-fill follow-up at master `595c4cb`; REVIEW at master `3066c72`). The 07.1 HTTP filter framework is the load-bearing surface phase 10 plugs into: `internal/filter/http/types.go` (FilterHeadersStatus + StreamDecoderFilter + StreamEncoderFilter + HTTPFilter + HTTPFilterFactory + FilterInstanceFactory + FactoryCtx widened to 3 fields per ADR-0100), `internal/filter/http/callbacks.go` (DecoderFilterCallbacks + EncoderFilterCallbacks + RequestRouteConfig), `internal/filter/http/registry.go` (HTTPRegistry.Register + Freeze + Lookup), `internal/filter/http/perroute.go` (3-tier merge per ADR-0073; phase 10 ADDS a sibling `ResolveAllTiers` method per ADR-0110). The fault filter at `internal/filter/http/fault/fault.go` (per ADR-0100 + ADR-0101) is the immediate package-shape precedent (4-field consumed / 11-field silent-ignore pattern; runtimeConfig closure capture; FactoryCtx use); the cors filter at `internal/filter/http/cors/cors.go` (per ADR-0074) is the secondary precedent (encode-side header mutation pattern).
**Parent phase:** None — phase 10 is a top-level row under `BOOTSTRAP_PROMPT.md` §9 "HTTP filters family" (the §9 family heading at `ROADMAP.md` line 56 is a conceptual umbrella, not a row, per ADR-0106 settled by phase 09). Each §9 family-child is its own coherent phase row; phase 10 is the THIRD §9 family-row to land (after cors @ 07.1 and fault @ 09).
**Master design document:** `docs/envoy-go/phases/10-http-filter-header-mutation/BRAINSTORM.md` (autonomous-brainstorm artifact per ADR-0004; this SPEC distills BRAINSTORM §§1–12 into formal contract language and executes the §9 empirical-pin obligations IN-SESSION; the empirical findings AMEND BRAINSTORM Decision 11 — see §11.1 below for the major surprise).
**Differential surface at end of phase:** ROADMAP row `10` flips `in-progress → done` at the phase-done commit. NEW differential fixture `0012-http-header-mutation` (per-scenario equivalence under a single-listener prefix-routed driver against the §7 fixture bootstrap — five scenarios per BRAINSTORM Decision 9 / §6.2, refined to **four scenarios** in this SPEC per §11.1 empirical pin's protected-header config-load discipline) is differentially green. Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log`, `0007a-cors`, `0007b-iteration-probe`, `0008-listener-chain-match`, `0009-admin-config-dump`, `0010-graceful-drain`, `0011-http-fault` all green. The h2spec conformance gate at the ADR-0051 pin is unchanged at 53/53 PASS (phase 10's filter is a pure HTTP-layer addition; it touches no codec/framer/HPACK paths). Existing 12 fuzzers (10 from 08.1 + 1 from 08.2 + 1 from 09 — `FuzzFaultConfigParse`) re-run clean; one NEW fuzzer `FuzzHeaderMutationConfigParse` per §14.5. `BEHAVIOR_CONTRACT.md ## HTTP filter chain` umbrella gains a new `### envoy.filters.http.header_mutation` subsection per §13.1; `## Stat-name mapping` 22-name table is UNCHANGED (per §11.3 empirical pin: header_mutation emits zero stats).

---

## 1. Purpose

Phase 10 lands `envoy.filters.http.header_mutation` — Envoy's canonical programmable header-rewrite filter — as the THIRD production HTTP filter in envoy-go after cors (07.1) and fault (09), and the SECOND top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family after fault. The five new architectural primitives:

1. A new `internal/filter/http/header_mutation/` package owning the filter implementation. The package mirrors the fault precedent (`internal/filter/http/fault/`): `header_mutation.go` (filter type + factory + decode/encode methods + runtimeConfig parser + compiledMutationOp shape + applyOps loop), `header_mutation_test.go` (unit tests), `doc.go` (package overview), `fuzz_test.go` (`FuzzHeaderMutationConfigParse` per §14.5). Two top-level exports: `TypeURL` (string constant `"type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation"`) + `New` (the `HTTPFilterFactory` registered against `TypeURL` in the boot registry). All other types (`runtimeConfig`, `compiledMutationOp`, `mutationOpKind`, `filter`) are unexported. See ADR-0108.

2. **Programmable request-mutation execution in `DecodeHeaders`.** The filter iterates the resolved request-mutations list (filter-level prepended; per-route tiers appended in flag-controlled order, see primitive 5) and applies each `HeaderMutation` primitive (the `config/common/mutation_rules/v3.HeaderMutation` oneof of `Remove` | `Append`) to the request headers. Returns `Continue` after the loop. NO async-resume primitive is exercised — this filter is fully synchronous on both decode and encode paths. See ADR-0109.

3. **Programmable response-mutation execution in `EncodeHeaders`.** The filter iterates the resolved response-mutations list against response headers using the symmetric algorithm. Returns `Continue`. This is the FIRST production filter to perform PROGRAMMABLE state-mutation in the encode path on the non-error path: cors injects a fixed three-header set (per ADR-0074); fault never reaches encode on the normal path (delay is decode-side; abort is terminal-replace via SendLocalReply). Phase 10 stress-tests the framework's `EncodeHeaders` mutation API across the full AppendAction × 4 + Remove surface. See ADR-0109.

4. **Multi-tier per-route resolution — NOT most-specific override.** The proto field `HeaderMutation.most_specific_header_mutations_wins` (per `header_mutation.pb.go:149–154`) REQUIRES the filter to evaluate per-route configs at ALL THREE tiers (Route, VirtualHost, RouteConfiguration), not just the most-specific one. This is **structurally different** from the 07.1 framework's `PerRouteConfig.Resolve` (per `internal/filter/http/perroute.go:103–128`) which returns the MOST-SPECIFIC scope's config and discards the others (per ADR-0073: "no field-merge"). Phase 10 therefore adds a small framework extension `PerRouteConfig.ResolveAllTiers` (~80 LoC sibling to `Resolve`) exposing per-tier configs unmerged. See ADR-0110.

5. **`most_specific_header_mutations_wins` cross-tier ordering.** The filter applies (a) listener-level mutations FIRST (always, per the proto comment at `header_mutation.pb.go:141–142`: "The mutation rules in the filter configuration will always be applied first and then the per-route mutation rules"), then (b) per-route tier mutations in flag-controlled order: with `most_specific_header_mutations_wins=false` (DEFAULT) the order is most-specific-first → least-specific-last (so least-specific RouteConfiguration "wins" overlap by virtue of being applied last); with `=true` the order is reversed (least-specific-first → most-specific-last; most-specific Route wins). Confirmed empirically at §11.5: with a 4-tier overlapping `x-test` write (listener=`listener`, RouteConfiguration=`rc`, VirtualHost=`vh`, Route=`route`), flag=false yields final `x-test: rc`; flag=true yields final `x-test: route`. See ADR-0110.

After phase 10, the project has proven the §9 HTTP filters family-expansion pattern carries through a SECOND filter under the cors precedent's package-shape discipline, the fault precedent's `runtimeConfig` parser pattern, and a NEW per-route accessor model (multi-tier vs. most-specific): *envoy-go's HTTP filter framework can host a programmable header-rewrite primitive that exercises both decode-side and encode-side state mutation under traffic; the framework's per-route accessor surface extends from a single `Resolve` (most-specific) to a sibling `ResolveAllTiers` (multi-tier) with no impact on existing cors/fault per-route discipline; per-filter accessor choice becomes the load-bearing model for filters whose proto semantics demand multi-tier vs. most-specific evaluation; the protected-header set is enforced at config-load time (boot-fail-fast per ADR-0072); zero new stats are emitted (analogous to cors per ADR-0074); all under flat top-level row expansion (per ADR-0106).* This is the THIRD §9 family-row to land; subsequent filters (compression, local_ratelimit, jwt_authn, …) follow the same row-as-its-own-phase pattern.

### 1.1 Empirical-finding-driven scope revisions (per §11)

The §11 empirical-pin block executed in this SPEC's drafting session AMENDS BRAINSTORM design decisions in two places:

- **§11.1 (protected-header set) — MAJOR REVISION:** BRAINSTORM Decision 11 hypothesized that mutation attempts on protected headers would be **silent runtime no-ops** ("no error, no log, no stat"). The empirical pin proves this is **WRONG**: Envoy v1.37.2 enforces protection at **CONFIG-LOAD TIME** with a hard error (`:-prefixed or host headers may not be modified`). The protected set is exactly the five HTTP/2 pseudo-headers (`:method`, `:path`, `:authority`, `:scheme`, `:status`) plus `host` (case-insensitive: `host`, `Host`, `HOST` all rejected). Both listener-level AND per-route configs are validated; both `request_mutations` and `response_mutations` are validated. envoy-go's `New` factory MUST mirror by validating each `compiledMutationOp.headerName` against the protected set at parse time and returning a non-nil error from `New` for protected headers; ADR-0072's boot-time-fail-fast contract makes this ergonomic. Per-route protected-header rejection happens at HCM-build time (when `BuildPerRouteConfig` parses the per-route any-blob into `*HeaderMutationPerRoute`) — see §6.7 for the per-route validation thread. ADR-0111 carries this constraint as a load-bearing detail. **Differential fixture scenario count: 5 → 4** (BRAINSTORM §6.2 scenario 5 — "attempts to mutate protected headers; expects silent no-op verified differentially" — drops, replaced by a UNIT TEST asserting that `New` rejects each protected-header config-load attempt with a non-nil error; the boot-fail-fast surface is unit-test territory, not differential territory).
- **§11.3 (stats verification) — CONFIRMATION (no revision):** BRAINSTORM Decision 12 hypothesized zero `header_mutation.*` stats. The empirical pin confirms this: zero stats emitted by the dedicated filter in Envoy v1.37.2 across both `/stats` and `/stats/prometheus` formats (after driving 5 requests through the configured filter). Phase 10 emits zero stats; ADR-0114 codifies the no-emit discipline analogous to cors per ADR-0074. The 22-name `## Stat-name mapping` table (extended by phase 09) is UNCHANGED in phase 10. **No `FactoryCtx.Stats` consumption needed** — header_mutation's `New` factory does NOT consume `ctx.Stats` or `ctx.StatPrefix` (the 3-field `FactoryCtx` per ADR-0100 stays as-is; phase 10 simply does not exercise the stat-bearing fields).

### 1.2 Revised scope summary (post-§11 amendments)

After the §1.1 amendments, phase 10's in-scope architectural primitives are the FIVE listed at the head of §1 (programmable request-mutation, programmable response-mutation, multi-tier per-route resolution, cross-tier ordering flag, package + registration), expressed as 8 BRAINSTORM-§1.1-style line items (BRAINSTORM's 8 in-scope items stay at 8; the per-route 3-tier evaluation is a new framework method, not a separate primitive). Differential fixture has FOUR scenarios per §7.1 (BRAINSTORM §6.2's fifth scenario — protected-header attempts — drops since the protection is config-load-time and is therefore unit-test territory, not differential territory). Stat-name extension is ZERO names. ADR list is REDUCED from the brainstorm's 7 anticipated (ADR-0108..ADR-0114) to **6** (ADR-0108..ADR-0113); ADR-0114 is dropped (no-stats is documented inline in §13.1's BEHAVIOR_CONTRACT extension, not promoted to its own ADR — see §8.1 consolidation).

### 1.3 Family-expansion shape (per BRAINSTORM Decisions 12, 13 + ADR-0106)

Phase 10 is a **flat top-level row** under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family heading; the §9 family heading at `ROADMAP.md` line 56 is a conceptual umbrella, not a row, and stays unchanged in state across phase 10's landings. Phase 10 is the THIRD §9 family-row to land (after cors @ 07.1 and fault @ 09). Each subsequent HTTP filters family member (compression, local_ratelimit, jwt_authn, …) becomes its own top-level row at row 11, 12, 13, … There is NO sibling-stub authored by this SPEC for the next §9 row; future family-expansion brainstorms cold-start from the §9 heading + the just-shipped artefacts (per ADR-0106(b) + (e)). The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing per ADR-0106(c).

---

## 2. Non-purposes

Per `BOOTSTRAP_PROMPT.md` §6.3 (scope-bounding) and ADR-0040 (out-of-scope deferrals format), the following are explicitly out of phase 10's scope:

### 2.1 HeaderMutation proto-message non-goals (per BRAINSTORM §8 + §11.1 amendment)

- **`mutations.query_parameter_mutations[]` (KeyValueMutation triple).** Path-query rewriting via the `envoy.config.common.mutation_rules.v3.KeyValueMutation` triple (a query-parameter analogue of `HeaderMutation`). The Mutations message has 3 fields total (`request_mutations`, `query_parameter_mutations`, `response_mutations` per `header_mutation.pb.go:35`); phase 10 honors the 1st and 3rd, silently parses the 2nd. Forward-pointer: a future `query_parameter_mutations` extension (numbered `10.x` per ADR-0045 internal split, or a new top-level row at family-expansion time). Deferral ADR per ADR-0040 format = ADR-0112.
- **Header-value formatter substitution syntax** (`%REQ(:path)%`, `%DOWNSTREAM_REMOTE_ADDRESS%`, `%RESPONSE_CODE%`, `%START_TIME(...)%`, etc.). Envoy's `HeaderValue.value` field accepts formatter-substitution tokens evaluated against per-request context. Phase 10's runtimeConfig stores `headerValue` as a STATIC string verbatim — formatter syntax is materialized as a literal value (i.e., a configured value of `"%REQ(:path)%"` produces the literal 11-byte string on the wire, not the substituted path). Forward-pointer: a future formatter-substitution extension that lifts the access-log subset of the formatter (per phase 06.2) into a header-value evaluator. Deferral ADR per ADR-0040 format = ADR-0113.

### 2.2 `most_specific_header_mutations_wins` non-goals

- **Field-level merge across tiers** (e.g., per-tier `request_mutations` lists merged into a single deduplicated list). Phase 10's algorithm is APPLY-IN-ORDER: each tier's full list is applied independently in proto-declared order WITHIN the tier, with cross-tier ordering controlled by the flag (per §2.10 algorithm). NO deduplication of overlapping op targets across tiers; if two tiers both `OVERWRITE_IF_EXISTS_OR_ADD` `x-test`, both ops execute and the later wins. This matches Envoy's empirically-pinned behavior at §11.5.
- **Per-route override of the `most_specific_header_mutations_wins` flag.** The flag is on the listener-level `HeaderMutation` proto message (per `header_mutation.pb.go:155`); the per-route `HeaderMutationPerRoute` proto message has only a `mutations` field (per `header_mutation.pb.go:55–62`). Per-route configs cannot override the flag; the flag is a listener-level invariant.

### 2.3 Test-surface non-purposes

- **Differential testing under H2 streams.** Fixture 0012's bootstrap is HTTP/1.1-only (codec_type: HTTP1), mirroring 0011's discipline. Phase 05.1 / 05.2 H2 codec paths are unchanged by phase 10; H2 differential testing of header_mutation is deferred (low operator value; high fixture cost). Forward-pointer: a future small follow-up phase if operator value materializes.
- **Stress / load testing.** Phase 10's filter is fully synchronous + lock-free (no shared per-instance state beyond the closure-captured `*runtimeConfig`); no stress test is run as part of phase-done gate (b). Unit tests cover the algorithmic surface mechanically.
- **Differential testing of the header_mutation × cors interaction.** Fixture 0012 is header_mutation + router only. The cors filter is independent and not involved in fixture 0012. Cross-filter interaction tests are deferred to whatever phase lands the relevant sibling filter. Note that header_mutation interacts cleanly with router (header_mutation runs BEFORE router on decode; router's encode-side is no-op pass-through; header_mutation's encode-side mutates the upstream response before the wire-write fires) — fixture 0012 exercises this without explicit cross-filter assertions.
- **Differential testing of header_mutation × fault interaction.** When fault aborts via SendLocalReply (per ADR-0103), the encode chain enters at `filter[len-1]` (router) per ADR-0075, iterates in reverse, and runs header_mutation's EncodeHeaders against the synthesized abort response. This means header_mutation's response_mutations WOULD apply to fault's abort body's headers (4-header set per phase 09 §11.3). Fixture 0012 does NOT test this scenario (no fault filter in the chain); the cross-filter interaction is deferred. Future fixtures that combine header_mutation + fault should test the encode-mutation-on-abort path.

### 2.4 Cross-filter non-purposes

- **Filter-ordering interactions.** Phase 10 puts `envoy.filters.http.header_mutation` BEFORE `envoy.filters.http.router` in the http_filters list (per the fixture envoy.yaml at §7.4). The 07.1 ADR-0072 filter-ordering discipline (filters in the order declared) is unchanged. There is no test of header_mutation interleaving with other §9 filters in this phase. Cross-filter interaction tests are deferred.
- **Encode-side ordering with sibling encode-mutating filters.** Phase 10 is the FIRST production filter to perform programmable encode-side mutation; cors's encode-side injects a fixed 3-header set (not programmable). When a future encode-mutating filter lands (e.g., an analytics tag injector), the encode-iteration order will matter: encode runs in REVERSE filter-list order (per ADR-0075), so a filter listed AFTER header_mutation in http_filters will run BEFORE header_mutation on encode. Phase 10 does not test sibling encode-mutating filters; deferred to whatever phase lands the second encode-mutating filter.

### 2.5 Security non-purposes

- **Authentication / authorization on header-mutation behavior.** header_mutation is a per-request behavioral filter; there is no admin-endpoint surface for it. The operator's listener config controls whether and how header_mutation is applied; envoy-go's MVP does not gate header_mutation behind any auth surface.
- **Audit-logging of header-mutation events.** header_mutation emits zero stat counters per §1.1 + ADR-0114-folded discipline; no per-request audit log of mutations applied. Existing access-log discipline (per phase 06.2) covers per-request log emission of the post-mutation headers.
- **Protection against malicious config that mutates security-sensitive non-protected headers** (e.g., `authorization`, `cookie`, `set-cookie`). Envoy v1.37.2 does NOT protect these; they fall outside the `:`-prefixed + `host` set per §11.1. Phase 10 mirrors: any non-protected header is mutable. Operators are responsible for not configuring mutations that would degrade security (e.g., stripping `Authorization` headers downstream-bound). Forward-pointer: future ext_authz / rbac filter phases that gate sensitive-header rewrites.

---

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for phase 10)

| Gate | 10 specialization |
|---|---|
| **(a)** `go build ./...` clean | Including the new `internal/filter/http/header_mutation/` package (`doc.go`, `header_mutation.go`, `header_mutation_test.go`, `fuzz_test.go`), the modified `cmd/envoy-go/main.go` (one-line registration delta + import line), the modified `internal/filter/http/perroute.go` (new `ResolveAllTiers` method ~60 LoC) and `internal/filter/http/perroute_test.go` (new tests ~100 LoC). All under `go vet ./...` clean and `golangci-lint run ./...` clean. The `gofmt`/`goimports` discipline mirrors the 09 close. |
| **(b)** `go test ./...` clean | New unit tests in `internal/filter/http/header_mutation/header_mutation_test.go` covering: New-factory typed_config validation (success path + nil tc + malformed tc + protected-header at filter-level → error per §11.1); runtimeConfig shape correctness; `compiledMutationOp` parsing for all 4 AppendAction variants + Remove; AppendAction × 4 semantics (each variant against EXISTS / ABSENT targets); `keep_empty_value=false` skip + `=true` materialize semantics per §11.2; multi-valued header behavior per §11.4 (OVERWRITE collapse + APPEND preserve); protected-header rejection symmetric across listener-level and per-route configs; multi-tier ordering with both flag values per §11.5; route-config-level + virtual-host-level + route-level config combinations (all 7 non-empty subsets). Plus `go test -race ./...` clean (no concurrency primitives, but the race detector run validates by construction). New `ResolveAllTiers` tests in `internal/filter/http/perroute_test.go` covering all-three-tiers-set, two-of-three set in 3 combinations, one-tier-set, no-tier-set. |
| **(c)** h2spec re-run clean (53/53 PASS at ADR-0051 pin) | Phase 10 touches no codec / framer / hpack / connection-management code; it adds a single new HTTP filter to the chain and one new framework method to perroute.go. The h2spec gate at 53/53 PASS is invariant under these additions; re-running is mechanical. The CONFORMANCE_PINS pin is unchanged. |
| **(d)** new/existing fuzzers run clean | Existing 12 fuzzers (10 from 08.1 + `FuzzDrainTransitions` from 08.2 + `FuzzFaultConfigParse` from 09) re-run clean at the 30s ADR-0018 budget. **NEW (REQUIRED):** `FuzzHeaderMutationConfigParse` (~50 LoC; 30s budget) — fuzzes the `tc *anypb.Any` parameter to `New` against arbitrary byte sequences, asserting that `New` either returns a valid factory OR a non-nil error (no panics, no empty-OK responses). Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the header_mutation filter's typed_config parser falls under "parser." Total fuzzer count post-10: **13**. |
| **(e)** Differential fixtures all green | All pre-existing fixtures `0000–0011` remain green (phase 10 adds a filter but does not modify any existing fixture's bootstrap). **NEW:** `0012-http-header-mutation` (`test/differential/0012-http-header-mutation/`) is green under the per-scenario equivalence claims of §7.1 — four scenarios per §7.1 refined per §11.1: (1) listener-only request_mutations + response_mutations covering all 4 AppendActions + Remove + keep_empty_value boundary; (2) per-route override + listener interaction; (3) multi-tier evaluation with `most_specific_header_mutations_wins=false` (least-specific wins); (4) multi-tier evaluation with `most_specific_header_mutations_wins=true` (most-specific wins). The `RequiresReference: true` flag is set in `test/differential/runner.go` per the existing fixture-registration pattern (mirrors `0011-http-fault`). |
| **(f)** `BEHAVIOR_CONTRACT.md` populated | `## HTTP filter chain` umbrella gains a new `### envoy.filters.http.header_mutation` subsection per §13.1. `## Stat-name mapping ### 22-name table` is UNCHANGED (per §1.1 + §11.3 — header_mutation emits zero stats). `## Timing tolerances` is UNCHANGED (synchronous filter; no time-bounded assertions). ADR-0040 forward-pointer notes appended to the relevant deferral cluster paragraphs (query_parameter_mutations, formatter substitution). In-place edit per ADR-0052 — lands at the phase-done commit alongside the implementation. |

The phase-done commit message body must explicitly state that ROADMAP row `10` flips `in-progress → done` AT this commit AND that the §9 HTTP filters family heading at ROADMAP line 56 stays unchanged (headings are not rows; their state is implicit) AND that this is the THIRD §9 family-row to land (after cors @ 07.1 and fault @ 09). Per `BOOTSTRAP_PROMPT.md` §5.3 commit message format. Commit subject: `phase 10: http-filter-header-mutation [ADR-0108, ADR-0109, ADR-0110, ADR-0111, ADR-0112, ADR-0113]` (or fewer if the planner consolidates per §8.1 consolidation candidates).

---

## 4. Deliverables (files and directories)

### 4.1 New production code (in 10)

- `internal/filter/http/header_mutation/doc.go` — package doc enumerating the typed_config surface (`HeaderMutation` proto with 4-field consumed / 1-field silent-ignore decomposition per ADR-0109; `HeaderMutationPerRoute` proto with 1-field-consumed decomposition), the public API surface (`TypeURL`, `New`), the iteration-protocol coverage (Continue on DecodeHeaders + EncodeHeaders only — no StopIteration; no SendLocalReply; no async-resume; no body / trailers states exercised), the multi-tier per-route discipline (per ADR-0110), the protected-header set (per ADR-0111 + §11.1 verbatim), and the cross-cutting ADR anchors. ~40 LoC.
- `internal/filter/http/header_mutation/header_mutation.go` — filter implementation. Public surface: `TypeURL` constant + `New` factory (matches `envoyhttp.HTTPFilterFactory` type signature). Unexported types: `runtimeConfig` struct (3 fields per §6.2 below), `compiledMutationOp` struct (5 fields per §6.4 below), `mutationOpKind` uint8 type with two values (`kindRemove`, `kindAppend`), `filter` struct (per-instance state per §6.3). `New(tc *anypb.Any, _ envoyhttp.FactoryCtx)` parses `tc` to `*envoyextensionsfiltershttpheadermutationv3.HeaderMutation`, validates each mutation's `headerName` against the protected-header set per §11.1 (returning an error mirroring Envoy's verbatim message `header_mutation: %q is :-prefixed or host; may not be modified`), constructs a `*runtimeConfig` capturing the consumed fields, and returns a `FilterInstanceFactory` closure that allocates a fresh `*filter{cfg: rc}` per request. The filter implements both `StreamDecoderFilter` and `StreamEncoderFilter` (decoder and encoder both carry mutation logic). `DecodeHeaders` body: `applyOps(headers, cfg.requestOps)`; resolve all per-route tiers via `dcb.RequestRouteConfigsAllTiers(filterName)`; iterate tiers in flag-controlled order applying each tier's `requestOps`; return `Continue`. `EncodeHeaders` body: symmetric on response side. `OnDestroy` is no-op (no timers, no async state). `DecodeData` / `EncodeData` / `DecodeTrailers` / `EncodeTrailers` are pass-through (DataContinue / TrailersContinue). ~280 LoC.
- `internal/filter/http/header_mutation/header_mutation_test.go` — unit tests per §14.1. ~320 LoC.
- `internal/filter/http/header_mutation/fuzz_test.go` — `FuzzHeaderMutationConfigParse` per §14.5. ~50 LoC.

### 4.2 Changed production code (in 10)

- `cmd/envoy-go/main.go` — modified per BRAINSTORM Decision 2. ONE new `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` line inserted alphabetically-after-fault per the ADR-0100 §2.2 convention codified at phase-09 brainstorm time: the resulting block is `httpReg.Register(router.TypeURL, router.New); httpReg.Register(cors.TypeURL, cors.New); httpReg.Register(envoygotest.TypeURL, envoygotest.New); httpReg.Register(fault.TypeURL, fault.New); httpReg.Register(header_mutation.TypeURL, header_mutation.New); httpReg.Freeze()`. Plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/header_mutation"` alphabetically among the existing filter-package imports (between `fault` and `router`). **No other wiring changes** — header_mutation is HTTP-only, no listener / cluster / drain manager threading. ~3 LoC delta.
- `internal/filter/http/perroute.go` — NEW method `ResolveAllTiers(filterName string, routeIdx int) (route, vhost, rc proto.Message)` per ADR-0110. Implementation reads directly from the existing fields (`p.scopes[routeIdx].route[filterName]`, `p.scopes[routeIdx].vhost[filterName]`, `p.rc[filterName]`) without most-specific selection. Returns the unmerged 3-tuple; nil entries for tiers without configs. The cache (`p.cache`) is NOT consulted by `ResolveAllTiers` (the existing cache-key is `(filterName, routeIdx)` returning a single proto.Message; multi-tier returns 3 messages). The SPEC author's choice (per §6.7): leave the cache as-is and re-read from the maps on each `ResolveAllTiers` call (the maps are read-only post-`BuildPerRouteConfig`; lookup is sub-microsecond). ~60 LoC delta.
- `internal/filter/http/perroute_test.go` — NEW tests for `ResolveAllTiers` covering: (a) all-three-tiers-set returns the 3-tuple in correct positions; (b) two-of-three set (3 combinations: route+vhost / route+rc / vhost+rc); (c) one-tier-set (3 combinations); (d) no-tier-set returns 3-nil; (e) routeIdx out-of-range returns 3-nil; (f) filterName not present at any tier returns 3-nil; (g) `ResolveAllTiers` does NOT pollute or read from the existing `Resolve` cache (call `ResolveAllTiers` then `Resolve` and verify both return correct values independently). ~100 LoC delta.
- `internal/filter/http/callbacks.go` — NEW callback method `RequestRouteConfigsAllTiers(filterName string) (route, vhost, rc proto.Message)` on `DecoderFilterCallbacks` (and `EncoderFilterCallbacks` if EncodeHeaders needs to re-resolve — see §6.6 deferred decision). The callback delegates to the chain's per-route resolver: `c.chain.perRoute.ResolveAllTiers(filterName, c.chain.routeIdx)`. Existing `RequestRouteConfig(filterName)` callback unchanged (cors and fault continue to use it). ~30 LoC delta on callbacks.go + ~50 LoC delta on chain.go (the chain wires the new callback into the per-stream state machine; the routeIdx resolution is identical to the existing `RequestRouteConfig` path). Final integration shape (whether to add the method on both Decoder and Encoder callback interfaces, or to cache the 3-tuple at first decode-side resolution and re-read from a per-instance field at encode time) is the planner's call per §12 deferred decision 1; default position: add to BOTH callback interfaces symmetrically (the existing `RequestRouteConfig` is on the Decoder interface only — phase 10's encode-side re-resolution is novel and the SPEC author leaves the symmetry choice to the planner).

### 4.3 New harness and fixture code (in 10)

- `test/differential/0012-http-header-mutation/README.md` — fixture overview + per-scenario equivalence-claim narrative + the four-scenario list (per §7.1) + the dual-proxy bootstrap discipline (admin/listener ports disambiguated for dual-boot under `--network host` per the existing fixture pattern). ~80 LoC.
- `test/differential/0012-http-header-mutation/expectations.yaml` — per-scenario assertion matrix (per §7.1 prefix paths): scenario 1 (`/listener-only`) → request body reflects mutated headers (echo backend); response carries mutated response headers; status 200; scenario 2 (`/route-override`) → per-route layered with listener; status 200; scenario 3 (`/multi-tier-lws`) → flag=false → least-specific (RouteConfiguration) wins overlap; scenario 4 (`/multi-tier-mws`) → flag=true → most-specific (Route) wins overlap. NO timing assertions (synchronous filter). NO stat assertions (zero stats per §11.3). Allow-list extensions: NONE (no Envoy-emitted headers added by header_mutation; user-configured mutations are byte-equivalent across reference vs. envoy-go). ~60 LoC.
- `test/differential/0012-http-header-mutation/envoy.yaml` — reference Envoy bootstrap (admin :9912, listener :10012). The single-listener prefix-routed shape per §7.4 verbatim YAML: listener-level header_mutation sets one request_mutation + one response_mutation; per-route tiers configured under `typed_per_filter_config` on Route + VirtualHost + RouteConfiguration per scenarios 2/3/4. Two listeners (`l_lws`, `l_mws`) for the two flag values; both share the same per-route tier configs (only the flag differs). Single STRICT_DNS cluster `c_backend` pointing at the harness echo backend hostname. ~110 LoC.
- `test/differential/0012-http-header-mutation/envoy-go.yaml` — envoy-go bootstrap (admin :9911, listener :10011 + :10012). Identical to `envoy.yaml` modulo admin/listener port disambiguation. ~110 LoC.
- `test/differential/0012-http-header-mutation/driver/driver.go` — Go driver implementing the §7.3 four-scenario orchestration: dual-proxy boot + echo-backend boot + scenario probes (one curl-equivalent per scenario, run sequentially against both proxies, with response-body byte-equality + response-header set assertions) + cleanup. Event-based synchronization throughout (no hardcoded sleeps per the 08.2 SPEC §10 + 07.2 REVIEW M-8 carry-forward). ~220 LoC.
- `test/differential/0012-http-header-mutation/backends/backend.go` — minimal Go HTTP backend bound to port 18012. `/` endpoint serves a fast `200 OK` with body listing every received request header (one per line: `"Name: value\n"`); response carries one single-value header (`X-Resp-Test: backend-original`) and one multi-value header (`X-Multi: alpha`, `X-Multi: beta`) for OVERWRITE / APPEND multi-value testing. Mirrors the §11.4 echo backend exactly. ~50 LoC.
- `test/differential/runner.go` — `RegisterFixture("0012-http-header-mutation", ..., Capabilities{RequiresReference: true})` registration line added per the existing fixture-registration pattern. ~3 LoC delta.

### 4.4 Changed documentation and state (in 10)

- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — in-place edit per ADR-0052: NEW `### envoy.filters.http.header_mutation` subsection under `## HTTP filter chain` umbrella per §13.1. The `## Stat-name mapping ### 22-name table` is UNCHANGED (zero new stats). The `## Timing tolerances` section is UNCHANGED (synchronous filter). The `## Equivalence Matrix` gains ONE new row for the header_mutation-filter equivalence claim per §13.4. Lands at phase-done commit alongside impl.
- `docs/envoy-go/DECISIONS.md` — six new ADRs (ADR-0108..ADR-0113 per §8) appended. Lands incrementally per `superpowers:executing-plans` PROGRESS preamble convention (ADRs land at the task that anchors them).
- `docs/envoy-go/ROADMAP.md` — row `10` flips `planned → in-progress` AT THIS COMMIT (per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3); flips `in-progress → done` at the 10 phase-done commit. The §9 family heading at line 56 stays unchanged (per ADR-0106).
- `docs/envoy-go/STATE.md` — flips `lifecycle-state: 2 → 3`, `next-skill: superpowers:writing-plans` (PLAN.md authoring for phase 10), `active-phase: 10-http-filter-header-mutation`, `last-commit: <SPEC commit SHA>`, `last-updated: <date>`. SHA-fill follow-up commit per phase-04..09 convention. Updated in this commit (the SPEC-authoring session updates STATE.md per the cold-start exit-contract); the SPEC names what the next session will write.
- `docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md` — this file.
- `docs/envoy-go/phases/10-http-filter-header-mutation/PLAN.md` — authored by the next session (lifecycle-state 3 → 4); not part of THIS commit.
- `docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md` — authored at PLAN-execution start by the executing session (lifecycle-state 4); not part of THIS commit.

---

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 10)

```
                     +-------------------------------------+
                     |  cmd/envoy-go/main.go               |
                     |  (registers router, cors,           |
                     |   envoy_go_test, fault,             |
                     |   header_mutation under httpReg;    |
                     |   one new line + import)            |
                     +------------+------------------------+
                                  |
                                  v
                     +-------------------------------------+
                     |  internal/filter/http/HTTPRegistry  |
                     |  (07.1; ADR-0072; freeze-after-boot)|
                     |  resolves header_mutation.TypeURL   |
                     |  @ HCM build                        |
                     +------------+------------------------+
                                  |
                                  v
                     +-------------------------------------+
                     |  internal/filter/http/              |
                     |  header_mutation/                   |
                     |  -------------------------------    |
                     |  TypeURL = "...HeaderMutation"      |
                     |  New(tc, ctx) -> (factory, error)   |
                     |    parses tc, validates protected   |
                     |    headers, captures *runtimeConfig |
                     |  factory() -> HTTPFilter{           |
                     |    Decoder: f, Encoder: f, Name:... |
                     |  }                                  |
                     |  filter.DecodeHeaders ->            |
                     |    applyOps(req, cfg.requestOps);   |
                     |    routeOps, vhOps, rcOps =         |
                     |      cb.RequestRouteConfigsAllTiers |
                     |    apply tiers in flag-order;       |
                     |    return Continue                  |
                     |  filter.EncodeHeaders ->            |
                     |    symmetric on response side       |
                     +-------------------------------------+
                                  |
                                  v
                     +-------------------------------------+
                     |  internal/filter/http/perroute.go   |
                     |  PerRouteConfig.ResolveAllTiers     |
                     |  (NEW per ADR-0110; sibling to      |
                     |   Resolve; reads p.scopes[*].route, |
                     |   p.scopes[*].vhost, p.rc unmerged) |
                     +-------------------------------------+
```

The registry → factory → filter-instance flow mirrors cors/fault verbatim (per ADR-0072's HTTPFilterFactory two-step contract). The NEW dimension is the multi-tier per-route accessor: `RequestRouteConfigsAllTiers` (NEW callback) → `PerRouteConfig.ResolveAllTiers` (NEW framework method) returns the 3-tuple of unmerged per-tier configs. cors and fault continue to use `RequestRouteConfig` → `PerRouteConfig.Resolve` (most-specific override per ADR-0073) — the framework now hosts BOTH accessor models, with per-filter accessor choice (per ADR-0110). NO async-resume primitive is exercised. NO `SendLocalReply`. NO `StopIteration`.

### 5.2 Per-request flow — listener-only mutations (canonical scenario)

```
HCM dispatch
  -> filter[0..N-1].DecodeHeaders run in order
  -> header_mutation.DecodeHeaders fires (filter[i] for some i ∈ [0, len-2])
       1. applyOps(reqHeaders, cfg.requestOps) — listener-level mutations
          For each compiledMutationOp:
            if op.kind == kindRemove: reqHeaders.Del(op.headerName)
            if op.kind == kindAppend:
              if op.headerValue == "" && !op.keepEmptyValue: skip
              switch op.appendAction:
                APPEND_IF_EXISTS_OR_ADD:    reqHeaders.Add(op.headerName, op.headerValue)
                ADD_IF_ABSENT:              if reqHeaders.Get(op.headerName) == "":
                                              reqHeaders.Add(op.headerName, op.headerValue)
                OVERWRITE_IF_EXISTS_OR_ADD: reqHeaders.Set(op.headerName, op.headerValue)
                OVERWRITE_IF_EXISTS:        if reqHeaders.Get(op.headerName) != "":
                                              reqHeaders.Set(op.headerName, op.headerValue)
       2. perRoute resolution: routeOps, vhOps, rcOps := dcb.RequestRouteConfigsAllTiers("envoy.filters.http.header_mutation")
       3. (no per-route configs in scenario 1) — all 3 tiers nil; skip applyOps for all 3
       4. return Continue

  -> chain advances to filter[i+1..len-1]; router dials upstream

(upstream returns)

  -> filter[len-1..0].EncodeHeaders run in REVERSE order (per ADR-0075)
  -> header_mutation.EncodeHeaders fires
       1. applyOps(respHeaders, cfg.responseOps) — listener-level response mutations
       2. perRoute re-resolution (or cached from decode-side; SPEC §6.6 deferred);
          per-tier responseOps applied in flag-controlled order
       3. return Continue

  -> wire-write fires on the post-mutation response
```

### 5.3 Per-request flow — per-route override (scenario 2)

```
HCM dispatch
  -> header_mutation.DecodeHeaders fires
       1. applyOps(reqHeaders, cfg.requestOps) — listener-level FIRST
       2. routeOps, vhOps, rcOps := dcb.RequestRouteConfigsAllTiers(filterName)
       3. routeOps != nil; vhOps == nil; rcOps == nil (scenario 2: only Route tier set)
       4. Default flag (false): apply Route → VirtualHost → RouteConfiguration order
            applyOps(reqHeaders, routeOps.requestOps)  // applied second (after listener)
            // vhOps, rcOps nil — no-op
       5. return Continue
```

### 5.4 Per-request flow — multi-tier with `most_specific=false` (scenario 3, lws)

```
HCM dispatch
  -> header_mutation.DecodeHeaders fires
       1. applyOps(reqHeaders, cfg.requestOps) — listener writes x-test=listener
       2. routeOps, vhOps, rcOps := cb.RequestRouteConfigsAllTiers(filterName)
       3. All 3 non-nil; flag=false → apply most-specific FIRST, least-specific LAST:
            applyOps(reqHeaders, routeOps.requestOps)   // x-test=route
            applyOps(reqHeaders, vhOps.requestOps)      // x-test=vh    (overwrites route)
            applyOps(reqHeaders, rcOps.requestOps)      // x-test=rc    (overwrites vh)
       4. return Continue → upstream sees x-test=rc

  Confirmed empirically at §11.5: final x-test value is "rc" with flag=false.
```

### 5.5 Per-request flow — multi-tier with `most_specific=true` (scenario 4, mws)

```
HCM dispatch
  -> header_mutation.DecodeHeaders fires
       1. applyOps(reqHeaders, cfg.requestOps) — listener writes x-test=listener
       2. routeOps, vhOps, rcOps := cb.RequestRouteConfigsAllTiers(filterName)
       3. All 3 non-nil; flag=true → apply least-specific FIRST, most-specific LAST:
            applyOps(reqHeaders, rcOps.requestOps)      // x-test=rc
            applyOps(reqHeaders, vhOps.requestOps)      // x-test=vh    (overwrites rc)
            applyOps(reqHeaders, routeOps.requestOps)   // x-test=route (overwrites vh)
       4. return Continue → upstream sees x-test=route

  Confirmed empirically at §11.5: final x-test value is "route" with flag=true.
```

### 5.6 Per-request flow — encode-side response_mutations

```
(upstream returns response with X-Multi: alpha, X-Multi: beta)

  -> filter[len-1..0].EncodeHeaders run in REVERSE filter-list order
  -> header_mutation.EncodeHeaders fires
       1. applyOps(respHeaders, cfg.responseOps)
          For scenario 1's response_mutations:
            OVERWRITE_IF_EXISTS_OR_ADD (X-Multi → "OVERWRITTEN"):
              respHeaders.Set("X-Multi", "OVERWRITTEN")  // collapses to single value
            APPEND_IF_EXISTS_OR_ADD (X-Multi → "APPENDED"):
              respHeaders.Add("X-Multi", "APPENDED")     // adds one more value
          Net effect: X-Multi: OVERWRITTEN, X-Multi: APPENDED (2 values)
       2. (no per-route response ops in scenario 1)
       3. return Continue

  -> response writes to wire with the post-mutation headers

  Confirmed empirically at §11.4: OVERWRITE collapses multi-value to single; APPEND
  preserves prior + adds one more.
```

### 5.7 Concurrency model

- **Per-request filter instance.** Each `HCM dispatch` call allocates a fresh `*filter` per request via the FilterInstanceFactory closure. Per-instance state is just `cfg *runtimeConfig` (a closure-captured pointer to a read-only listener-level config) + per-stream callbacks. No shared mutable state across requests; the single-goroutine-per-stream invariant per ADR-0071 makes per-instance state race-free WITHOUT synchronization.
- **`runtimeConfig` is read-only after `New`.** All fields of `*runtimeConfig` are populated at `New` time and never mutated. Multiple per-request `*filter` instances share the same `*runtimeConfig` via closure capture — read-only sharing is race-free.
- **No timer goroutines.** Unlike fault, header_mutation has no async-resume primitive; no timer goroutines spawned; no `OnDestroy` cleanup logic needed.
- **No `activeFaults`-style atomic counter.** Unlike fault, header_mutation has no concurrency cap; no shared atomic state.
- **No SendLocalReply.** Unlike cors / fault, header_mutation never short-circuits the chain; no encode-iteration entry-at-`filter[len-1]` discipline triggered.

The concurrency model is **maximally simple** — phase 10's filter is the project's most concurrency-trivial production filter (after router). The race-detector run under gate (b) validates by construction; no race-condition surface to test specifically.

### 5.8 Filter ordering in fixture 0012

The fixture's http_filters list is `[envoy.filters.http.header_mutation, envoy.filters.http.router]` (header_mutation BEFORE router). Per the ADR-0072 declaration-order discipline, header_mutation's DecodeHeaders runs at index 0; router's runs at index 1 (terminal). Encode iteration runs reverse: router's EncodeHeaders at index 1 (no-op pass-through), header_mutation's at index 0 (response_mutations applied). The fixture has no other intervening filters; cors, envoygotest, and fault are NOT included.

---

## 6. Per-component contract summary

### 6.1 Constructor signatures (fault precedent verbatim, modulo ctx unused)

`internal/filter/http/header_mutation/header_mutation.go` exports:

```go
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation"

func New(tc *anypb.Any, _ envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)
```

Note the `_ envoyhttp.FactoryCtx` discard: header_mutation does NOT consume `ctx.Stats` / `ctx.StatPrefix` / `ctx.Registry` (no stats per §11.3; no cross-filter lookups). The 3-field `FactoryCtx` per ADR-0100 stays as-is; phase 10 simply does not exercise the fields.

`New` body discipline (per ADR-0109):

1. If `tc == nil`: return error `errors.New("header_mutation: typed_config required")`. Mirrors the fault discipline (per ADR-0101 step 1) — a header_mutation filter with NO typed_config has no behavioral effect; surface configuration mistakes at boot.
2. Unmarshal: `var c envoyextensionsfiltershttpheadermutationv3.HeaderMutation; if err := tc.UnmarshalTo(&c); err != nil { return nil, err }`.
3. Parse the listener-level `mutations.request_mutations` and `mutations.response_mutations` into `[]compiledMutationOp` slices via `compileOps` helper (per §6.4). For each op, validate `headerName` against the protected-header set per §11.1; if protected, return error `fmt.Errorf("header_mutation: %q is :-prefixed or host; may not be modified", op.headerName)` (mirrors Envoy's verbatim message at §11.1).
4. Capture `most_specific_header_mutations_wins` from `c.MostSpecificHeaderMutationsWins`.
5. Construct `*runtimeConfig` per §6.2.
6. Return `func() envoyhttp.HTTPFilter { f := &filter{cfg: rc}; return envoyhttp.HTTPFilter{Name: "envoy.filters.http.header_mutation", Decoder: f, Encoder: f} }, nil`.

### 6.2 `runtimeConfig` shape (per ADR-0109)

```go
type runtimeConfig struct {
    requestOps                       []compiledMutationOp // listener-level request mutations (proto-declared order)
    responseOps                      []compiledMutationOp // listener-level response mutations (proto-declared order)
    mostSpecificHeaderMutationsWins  bool                  // precedence-order flag (default false; per §6.5 algorithm)
}
```

The 4 consumed fields from BRAINSTORM §1.1 item 3 are: `mutations.request_mutations[]`, `mutations.response_mutations[]`, `most_specific_header_mutations_wins`, `HeaderMutationPerRoute.mutations`. The first 3 land in this listener-level `runtimeConfig`. The 4th (per-route `HeaderMutationPerRoute.mutations`) is stored in the framework's per-route map (parsed at `BuildPerRouteConfig` time, surfaced via `ResolveAllTiers`) and re-compiled on demand at request-time per §6.7.

The 1 silently-ignored field from BRAINSTORM §1.1 item 3 is `mutations.query_parameter_mutations[]` (the `KeyValueMutation` triple acting on path-query) — silent at parse time per the cors/fault precedent of unmarshal-and-discard for unconsumed fields. Deferral ADR per ADR-0040 format = ADR-0112.

### 6.3 Per-instance `filter` struct

```go
type filter struct {
    cfg *runtimeConfig // pointer to listener-level config; per-route configs resolved per-request via cb

    dcb envoyhttp.DecoderFilterCallbacks
    ecb envoyhttp.EncoderFilterCallbacks

    // perRouteCache (planner-time decision per §12 deferred decision 2):
    // optionally cache the resolved per-tier compiledMutationOp slices at first
    // decode-side resolution to avoid re-resolving + re-compiling at encode-side.
    // Default position: do NOT cache; re-resolve at encode time (the maps are
    // read-only and lookup is sub-microsecond). Planner may cache if profiling
    // shows cost.
}
```

Per the cors / fault precedent, the filter implements both StreamDecoderFilter and StreamEncoderFilter so the chain can drive it as a both-sides filter. Both decode-side and encode-side carry mutation logic. `OnDestroy` is the single teardown hook; for header_mutation it is a no-op (no timers, no async state to release).

### 6.4 `compiledMutationOp` representation (per ADR-0109)

The repeated `HeaderMutation` (the primitive in `config/common/mutation_rules/v3` per `mutation_rules.pb.go`) is parsed at `New` time into a flat per-mutation struct:

```go
type mutationOpKind uint8

const (
    kindRemove mutationOpKind = iota
    kindAppend
)

type compiledMutationOp struct {
    kind           mutationOpKind                                       // kindRemove or kindAppend
    headerName     string                                                // canonicalized via http.CanonicalHeaderKey at parse time
    headerValue    string                                                // for kindAppend only ("" for kindRemove)
    appendAction   commonv3core.HeaderValueOption_HeaderAppendAction    // 4 variants; for kindAppend only
    keepEmptyValue bool                                                  // for kindAppend only
}
```

The `HeaderValueOption_HeaderAppendAction` enum is the canonical 4-valued type from `core/v3.HeaderValueOption` (NOT redefined locally — reuse the proto enum directly to avoid drift). Enum values:
- `APPEND_IF_EXISTS_OR_ADD` (0; default)
- `ADD_IF_ABSENT` (1)
- `OVERWRITE_IF_EXISTS_OR_ADD` (2)
- `OVERWRITE_IF_EXISTS` (3)

`compileOps` helper (called by `New` and by per-route on-demand compilation per §6.7):

```go
// compileOps projects []*HeaderMutation (the proto primitive) into []compiledMutationOp.
// Each input op must EITHER set `Action.Remove` (kindRemove) OR set `Action.Append`
// (kindAppend). Validates each headerName against the protected-header set per §11.1.
// Returns error on the first protected-header violation.
func compileOps(in []*commonmutationrulesv3.HeaderMutation) ([]compiledMutationOp, error)
```

### 6.5 `most_specific_header_mutations_wins` cross-tier algorithm (per ADR-0110 + §11.5 confirmation)

Per the proto comment at `header_mutation.pb.go:149–154` (verbatim quoted in the proto Go-control-plane module): "If per route HeaderMutationPerRoute config is configured at multiple route levels, header mutations at all specified levels are evaluated. By default, the order is from most specific (i.e. route entry level) to least specific (i.e. route configuration level). Later header mutations may override earlier mutations. This order can be reversed by setting this field to true. In other words, most specific level mutation is evaluated last."

The application algorithm (after listener-level mutations have been applied per §5.2 step 1):

**With `most_specific_header_mutations_wins = false` (DEFAULT):**
1. Apply Route-tier mutations (most specific, applied FIRST)
2. Apply VirtualHost-tier mutations (intermediate)
3. Apply RouteConfiguration-tier mutations (least specific, applied LAST → wins overlap)

**With `most_specific_header_mutations_wins = true`:**
1. Apply RouteConfiguration-tier mutations (least specific, applied FIRST)
2. Apply VirtualHost-tier mutations (intermediate)
3. Apply Route-tier mutations (most specific, applied LAST → wins overlap)

The `compiledMutationOp` slices for each tier are evaluated in proto-declared order WITHIN the tier (the cross-tier ordering is what the flag controls; within-tier order is fixed by the proto config).

**Empirical confirmation (per §11.5):** With listener-level `x-test=listener`, RouteConfiguration `x-test=rc`, VirtualHost `x-test=vh`, Route `x-test=route`, all OVERWRITE_IF_EXISTS_OR_ADD: flag=false yields final `x-test: rc` (RouteConfiguration applied last); flag=true yields final `x-test: route` (Route applied last). The proto comment matches Envoy's actual behavior verbatim.

### 6.6 `DecodeHeaders` body discipline (per ADR-0109 + ADR-0110)

```go
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    applyOps(headers, f.cfg.requestOps) // listener-level FIRST per §5.2 step 1

    if f.dcb == nil {
        return envoyhttp.Continue
    }
    routeMsg, vhMsg, rcMsg := f.dcb.RequestRouteConfigsAllTiers("envoy.filters.http.header_mutation")
    routeOps := f.compileForRequest(routeMsg)
    vhOps := f.compileForRequest(vhMsg)
    rcOps := f.compileForRequest(rcMsg)

    if !f.cfg.mostSpecificHeaderMutationsWins {
        applyOps(headers, routeOps)
        applyOps(headers, vhOps)
        applyOps(headers, rcOps)
    } else {
        applyOps(headers, rcOps)
        applyOps(headers, vhOps)
        applyOps(headers, routeOps)
    }
    return envoyhttp.Continue
}

func applyOps(headers http.Header, ops []compiledMutationOp) {
    for _, op := range ops {
        switch op.kind {
        case kindRemove:
            headers.Del(op.headerName)
        case kindAppend:
            applyAppendAction(headers, op)
        }
    }
}

func applyAppendAction(headers http.Header, op compiledMutationOp) {
    if op.headerValue == "" && !op.keepEmptyValue {
        return // silent skip per §11.2
    }
    switch op.appendAction {
    case core.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD:
        headers.Add(op.headerName, op.headerValue)
    case core.HeaderValueOption_ADD_IF_ABSENT:
        if headers.Get(op.headerName) == "" {
            headers.Add(op.headerName, op.headerValue)
        }
    case core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD:
        headers.Set(op.headerName, op.headerValue)
    case core.HeaderValueOption_OVERWRITE_IF_EXISTS:
        if headers.Get(op.headerName) != "" {
            headers.Set(op.headerName, op.headerValue)
        }
    }
}
```

(Pseudo-Go; final shape is the PLAN-implementer's call. The above captures the contract anchors per §11 empirical pins.)

`compileForRequest(msg proto.Message) []compiledMutationOp` is a per-request helper that type-asserts `msg` to `*HeaderMutationPerRoute` and re-compiles the request-mutations slice into `[]compiledMutationOp`. The compilation is sub-microsecond on small per-route configs (typical: < 5 ops per tier); the per-request cost is acceptable per §12 deferred decision 2. Planner may cache if profiling shows cost.

### 6.7 Per-route 3-tier multi-evaluation framework extension (per ADR-0110)

The framework's existing `internal/filter/http/perroute.go:103–128 Resolve(filterName, routeIdx)` returns one `proto.Message` (the most-specific tier's config). Phase 10 adds a sibling method `ResolveAllTiers`:

```go
// ResolveAllTiers returns the parsed per-route config at each tier, unmerged.
// Tiers are returned in canonical proto order: Route (most specific),
// VirtualHost (intermediate), RouteConfiguration (least specific). A tier
// with no config for filterName at the matched route is nil.
//
// Used by filters whose semantics require multi-tier evaluation rather than
// most-specific override (e.g., envoy.filters.http.header_mutation per its
// most_specific_header_mutations_wins flag). The default Resolve method
// (per ADR-0073) remains the canonical accessor for filters that use
// most-specific override (cors, fault).
func (p *PerRouteConfig) ResolveAllTiers(filterName string, routeIdx int) (route, vhost, rc proto.Message)
```

Implementation:

```go
func (p *PerRouteConfig) ResolveAllTiers(filterName string, routeIdx int) (route, vhost, rc proto.Message) {
    if p == nil {
        return nil, nil, nil
    }
    if routeIdx >= 0 && routeIdx < len(p.scopes) {
        if m, ok := p.scopes[routeIdx].route[filterName]; ok {
            route = m
        }
        if m, ok := p.scopes[routeIdx].vhost[filterName]; ok {
            vhost = m
        }
    }
    if m, ok := p.rc[filterName]; ok {
        rc = m
    }
    return route, vhost, rc
}
```

NOTE: `ResolveAllTiers` does NOT consult or pollute the existing `p.cache` (which is keyed by `(filterName, routeIdx)` returning a single proto.Message). The map reads (`p.scopes[routeIdx].route`, `p.scopes[routeIdx].vhost`, `p.rc`) are sub-microsecond; per-request re-reads are acceptable. Planner may add a per-tuple cache later if profiling shows cost.

The callback surface: `DecoderFilterCallbacks.RequestRouteConfigsAllTiers(filterName) (proto.Message, proto.Message, proto.Message)` (and symmetric `EncoderFilterCallbacks.ResponseRouteConfigsAllTiers` if §6.6 / §12 deferred decision 1 settles symmetric). The callback delegates to `chain.perRoute.ResolveAllTiers(filterName, chain.routeIdx)`.

**Per-route protected-header validation:** Per-route `HeaderMutationPerRoute` configs are unmarshalled by the framework's `BuildPerRouteConfig` (per `perroute.go:57–98`) into `proto.Message` values; the framework's `parseMap` only does proto unmarshalling and carries no per-filter validation hook today. **Empirical confirmation (per §11.1 per-route subprobe):** Envoy v1.37.2 DOES validate per-route configs at boot time — the same `:-prefixed or host headers may not be modified` error fires when a per-route `HeaderMutationPerRoute` attempts to mutate `:path`. envoy-go's discipline must therefore validate per-route configs at HCM-build time (boot), NOT at request-time, to mirror Envoy's boot-fail-fast surface. **Default position per §12 deferred decision 3 (EAGER validation):** the planner adds a small post-`BuildPerRouteConfig` validation hook (~40 LoC framework delta) that allows each filter to register a per-route-validator callback; at HCM-build time after `BuildPerRouteConfig`, the framework invokes each filter's per-route-validator on the parsed proto.Message values for that filter name. header_mutation's per-route-validator runs `compileOps` against each tier's `HeaderMutationPerRoute.mutations.{request,response}_mutations` and returns the first protected-header error. The hook surfaces any per-route protected-header violation as a boot-time error, identical-in-effect to the listener-level validation in `New`. (Lazy alternative — validate at first per-route resolution and panic — is REJECTED per §12 deferred decision 3 rationale: surfaces errors at first request rather than at boot, which violates the ADR-0072 boot-time-fail-fast contract.)

### 6.8 `EncodeHeaders` body discipline (symmetric to DecodeHeaders)

```go
func (f *filter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    applyOps(headers, f.cfg.responseOps) // listener-level FIRST

    if f.ecb == nil {
        return envoyhttp.Continue
    }
    routeMsg, vhMsg, rcMsg := f.ecb.ResponseRouteConfigsAllTiers("envoy.filters.http.header_mutation")
    routeOps := f.compileForResponse(routeMsg)
    vhOps := f.compileForResponse(vhMsg)
    rcOps := f.compileForResponse(rcMsg)

    if !f.cfg.mostSpecificHeaderMutationsWins {
        applyOps(headers, routeOps)
        applyOps(headers, vhOps)
        applyOps(headers, rcOps)
    } else {
        applyOps(headers, rcOps)
        applyOps(headers, vhOps)
        applyOps(headers, routeOps)
    }
    return envoyhttp.Continue
}
```

`compileForResponse(msg proto.Message)` is symmetric to `compileForRequest` but reads `mutations.response_mutations` instead of `mutations.request_mutations`.

**Encode-side framework gap check:** `applyOps` calls `headers.Add` / `headers.Del` / `headers.Set` / `headers.Get` — all on `net/http.Header` (the framework's existing `EncodeHeaders` API per `internal/filter/http/types.go:64`). The cors filter's encode-side already exercises `headers.Set` via `cors.go:138–144`. Phase 10 stress-tests `headers.Add` (multi-value APPEND), `headers.Del` (Remove), `headers.Set` (OVERWRITE), and `headers.Get` (conditional gates) on the encode-side. No new framework primitives needed. If the SPEC author finds (during impl) that the chain's encode-side `headers` map is presented in an opaque form that loses insertion order on multi-valued slots, the `OrderedHeaders` reconciliation discipline per `internal/filter/http/types.go:204–236` already handles it (the chain's `RunEncodeHeaders` projects `OrderedHeaders` to `http.Header`, runs the filter, then reconciles back to `OrderedHeaders` for the wire-write). No new gap.

---

## 7. Differential fixture `0012-http-header-mutation`

### 7.1 Equivalence claims (per BRAINSTORM §6 refined per §11)

Per §11.1 empirical-pin findings, the BRAINSTORM's 5-scenario list collapses to 4 scenarios (the protected-headers scenario drops per §11.1 — protected-header rejection is config-load-time, hence unit-test territory rather than differential-fixture territory). The per-scenario equivalence claims:

The fixture has TWO listeners (per §11.5 precedent): `l_lws` (`most_specific_header_mutations_wins=false`, port :10012) and `l_mws` (`most_specific_header_mutations_wins=true`, port :10013) sharing the same per-route tier configurations. This is the project's preferred fixture shape for testing flag-controlled cross-tier ordering: TWO listeners with identical per-route tiers and the flag as the distinguishing variable.

1. **Scenario 1 (listener-only mutations).** Probe: `GET /listener-only/anything` against `l_lws:10012` → no per-route override → only listener-level mutations apply. Listener config: `request_mutations` exercising APPEND_IF_EXISTS_OR_ADD + ADD_IF_ABSENT + OVERWRITE_IF_EXISTS_OR_ADD + OVERWRITE_IF_EXISTS + Remove against various target headers + `keep_empty_value` boundary; `response_mutations` exercising the same 5 op variants against the backend's response headers. Expects 200 from echo backend; response body byte-equivalent across reference vs. envoy-go (the body lists post-mutation request headers); response headers match the post-mutation set; no allow-list extensions needed.
2. **Scenario 2 (per-route override + listener interaction).** Probe: `GET /route-override/anything` against `l_lws:10012` → per-route `HeaderMutationPerRoute` at the Route tier; tests per-route + listener interaction (listener applied first, then route). Expects 200; response carries the layered-mutation result; differential equivalence on the layered output.
3. **Scenario 3 (multi-tier evaluation, flag=false, least-specific wins).** Probe: `GET /multi-tier/anything` against `l_lws:10012` → per-route `HeaderMutationPerRoute` at Route + VirtualHost + RouteConfiguration tiers, all OVERWRITE_IF_EXISTS_OR_ADD on `x-test`. Expects 200; final `x-test` value at upstream = `rc` (RouteConfiguration tier wins per default flag=false). Response_mutations symmetric: `x-resp-test` final value = `rc-resp`.
4. **Scenario 4 (multi-tier evaluation, flag=true, most-specific wins).** Probe: `GET /multi-tier/anything` against `l_mws:10013` → SAME per-route configs as scenario 3, but listener-level `most_specific_header_mutations_wins=true`. Expects 200; final `x-test` value at upstream = `route` (Route tier wins per flag=true). Response_mutations symmetric: `x-resp-test` final value = `route-resp`.

### 7.2 Dropped scenario (per §11.1)

BRAINSTORM §6.2's scenario 5 ("attempts to mutate `:method`, `:path`, `:authority`, `host`, `content-length`; expects silent no-op verified via differential equivalence") is **DROPPED** per §11.1 empirical pin. The protected-header rejection is config-load-time (boot fails with a hard error), not silent runtime no-op. The differential fixture's bootstrap CANNOT include a config attempting to mutate protected headers — both reference Envoy AND envoy-go would refuse to boot. The protected-header discipline is therefore covered by UNIT TESTS (per §14.1) that assert `New` returns a non-nil error mirroring Envoy's verbatim message for each protected-header attempt. The unit-test coverage is more rigorous than the BRAINSTORM's anticipated differential coverage (per-header attempt vs. a few sampled ones); the surface migrates from differential to unit without loss.

### 7.3 Driver outline

`test/differential/0012-http-header-mutation/driver/driver.go` orchestrates the four scenarios per the existing differential harness pattern (mirroring 0007a-cors and 0011-http-fault):

```
1. Boot echo backend (port 18012) per §7.5.
2. Boot reference Envoy and envoy-go subjects (admin :9912/listeners :10012+:10013 and
   :9911/:10011+:10012) per §7.4 dual-bootstrap.
3. Per scenario 1..4:
   3a. Issue probe against both proxies (one curl-equivalent per scenario).
   3b. Capture full response status + body + headers.
   3c. Diff vs. expectations.yaml allow-list + per-scenario assertion matrix; report failures.
4. Cleanup: kill subjects + backend; remove temp dirs.
```

Synchronous probes via the harness's `httptest`-style client; no goroutines beyond the dual-proxy boot path. Total probe count: **4** per proxy (one per scenario). Total wall-clock per proxy: <0.1s (synchronous filter; no delay, no abort). Both proxies probed sequentially.

### 7.4 Fixture bootstrap (per BRAINSTORM §7; port-disambiguated)

`test/differential/0012-http-header-mutation/envoy.yaml` (reference Envoy; admin :9912, listeners :10012 + :10013 — TWO listeners for the two `most_specific_header_mutations_wins` flag values per §11.5 precedent):

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9912 }
static_resources:
  listeners:
    - name: l_lws
      address:
        socket_address: { address: 0.0.0.0, port_value: 10012 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http_lws
                route_config:
                  name: rc_lws
                  typed_per_filter_config:
                    envoy.filters.http.header_mutation:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                      mutations:
                        request_mutations:
                          - append:
                              header: { key: "x-test", value: "rc" }
                              append_action: OVERWRITE_IF_EXISTS_OR_ADD
                        response_mutations:
                          - append:
                              header: { key: "x-resp-test", value: "rc-resp" }
                              append_action: OVERWRITE_IF_EXISTS_OR_ADD
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      typed_per_filter_config:
                        envoy.filters.http.header_mutation:
                          "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                          mutations:
                            request_mutations:
                              - append:
                                  header: { key: "x-test", value: "vh" }
                                  append_action: OVERWRITE_IF_EXISTS_OR_ADD
                            response_mutations:
                              - append:
                                  header: { key: "x-resp-test", value: "vh-resp" }
                                  append_action: OVERWRITE_IF_EXISTS_OR_ADD
                      routes:
                        - match: { prefix: "/listener-only" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/route-override" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.header_mutation:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                              mutations:
                                request_mutations:
                                  - append:
                                      header: { key: "x-route-only", value: "yes" }
                                      append_action: OVERWRITE_IF_EXISTS_OR_ADD
                        - match: { prefix: "/multi-tier" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.header_mutation:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                              mutations:
                                request_mutations:
                                  - append:
                                      header: { key: "x-test", value: "route" }
                                      append_action: OVERWRITE_IF_EXISTS_OR_ADD
                                response_mutations:
                                  - append:
                                      header: { key: "x-resp-test", value: "route-resp" }
                                      append_action: OVERWRITE_IF_EXISTS_OR_ADD
                http_filters:
                  - name: envoy.filters.http.header_mutation
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation
                      mutations:
                        request_mutations:
                          # Listener-level: exercise all 4 AppendActions + Remove + keep_empty_value
                          - append:
                              header: { key: "x-test", value: "listener" }
                              append_action: OVERWRITE_IF_EXISTS_OR_ADD
                          - append:
                              header: { key: "x-listener-add", value: "added" }
                              append_action: APPEND_IF_EXISTS_OR_ADD
                          - append:
                              header: { key: "x-add-if-absent-test", value: "added-if-absent" }
                              append_action: ADD_IF_ABSENT
                          - append:
                              header: { key: "x-overwrite-if-exists-test", value: "" }
                              append_action: OVERWRITE_IF_EXISTS    # absent target → no-op
                          - append:
                              header: { key: "x-empty-skip", value: "" }
                              append_action: APPEND_IF_EXISTS_OR_ADD    # keep_empty_value=false default → skip
                          - append:
                              header: { key: "x-empty-keep", value: "" }
                              append_action: APPEND_IF_EXISTS_OR_ADD
                              keep_empty_value: true                    # materialize empty value
                          - remove: "user-agent"                        # demonstrate Remove
                        response_mutations:
                          - append:
                              header: { key: "x-resp-test", value: "listener-resp" }
                              append_action: OVERWRITE_IF_EXISTS_OR_ADD
                          - append:
                              header: { key: "x-resp-multi", value: "appended" }
                              append_action: APPEND_IF_EXISTS_OR_ADD
                      most_specific_header_mutations_wins: false
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
    - name: l_mws
      address:
        socket_address: { address: 0.0.0.0, port_value: 10013 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http_mws
                # IDENTICAL route_config to l_lws above (same rc, vh, route tiers; same configs).
                # Only the listener-level most_specific_header_mutations_wins flag differs.
                route_config:
                  name: rc_mws
                  # ... (route_config body identical to rc_lws above) ...
                http_filters:
                  - name: envoy.filters.http.header_mutation
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation
                      mutations:
                        request_mutations:
                          - append:
                              header: { key: "x-test", value: "listener" }
                              append_action: OVERWRITE_IF_EXISTS_OR_ADD
                        response_mutations:
                          - append:
                              header: { key: "x-resp-test", value: "listener-resp" }
                              append_action: OVERWRITE_IF_EXISTS_OR_ADD
                      most_specific_header_mutations_wins: true
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STRICT_DNS
      connect_timeout: 0.25s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: <backend-host>, port_value: 18012 }
```

`envoy-go.yaml` is byte-identical modulo `port_value: 9911` (admin) and `port_value: 10011 + 10012` (listeners). The `route_config` bodies of `l_lws` and `l_mws` are identical (same rc-tier, vh-tier, and per-route configs); only the listener-level `most_specific_header_mutations_wins` flag differs. (The full YAML expansion of `l_mws.route_config` mirrors `l_lws.route_config` line-for-line; the SPEC summarizes with `... (route_config body identical to rc_lws above) ...` for brevity. The fixture file's actual bytes contain the full expansion.)

### 7.5 Backend shape

`test/differential/0012-http-header-mutation/backends/backend.go` — minimal Go HTTP backend bound to port 18012:

```go
package main

import (
    "fmt"
    "net/http"
    "sort"
    "strings"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Reflect received request headers into the response body.
        // Sort for determinism (Go map iteration is non-deterministic).
        names := make([]string, 0, len(r.Header))
        for n := range r.Header {
            names = append(names, n)
        }
        sort.Strings(names)
        var b strings.Builder
        for _, n := range names {
            for _, v := range r.Header[n] {
                fmt.Fprintf(&b, "%s: %s\n", n, v)
            }
        }
        body := b.String()
        // Single-value response header for OVERWRITE_IF_EXISTS variants.
        w.Header().Set("X-Resp-Test", "backend-original")
        // Multi-value response header for APPEND/OVERWRITE multi-value testing.
        w.Header().Add("X-Multi", "alpha")
        w.Header().Add("X-Multi", "beta")
        w.Header().Set("Content-Type", "text/plain")
        w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte(body))
    })
    if err := http.ListenAndServe(":18012", nil); err != nil {
        panic(err)
    }
}
```

The backend echoes received request headers into the response body so the differential harness can verify request-header mutations via the response body alone (no out-of-band logging needed). The `X-Resp-Test` + `X-Multi` response headers seed the response_mutations test cases.

### 7.6 Differential gate scope clarification

Phase 10's gate (e) requires fixture 0012 to be GREEN against both reference Envoy v1.37.2 and envoy-go. The 4 scenarios cover listener-only, per-route override, multi-tier flag=false, multi-tier flag=true. NO stat assertions (zero new stats per §11.3). NO timing assertions (synchronous filter). The differential is byte-equality on response status + body + post-mutation header set, allow-listed for the standard `Date` / `Server` / `Content-Length` differential ignore-list per the existing fixture pattern.

---

## 8. ADRs anticipated (per BRAINSTORM §7; refined per §11)

Phase 10's ADR-numbering anchor is **ADR-0108** (next-free; 09 closed at ADR-0107 per `DECISIONS.md` line 4243). The expected six ADRs (one fewer than the brainstorm's anticipated 7 per §1.1's ADR-0114-folded discipline):

| ADR | Title (settled by this SPEC) | Settles | Anchor |
|---|---|---|---|
| ADR-0108 | `internal/filter/http/header_mutation/` package shape + extension-registry registration line + boot-time `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` | BRAINSTORM Decisions 1, 2 | §6.1 + §4.2 |
| ADR-0109 | `runtimeConfig` shape + 4-field-consumed / 1-field-silent-ignore decomposition + `compiledMutationOp` flat-struct + AppendAction × 4 mapping table + `keep_empty_value` semantics | BRAINSTORM Decisions 3, 4, 7, 8 + §11.2 + §11.4 confirmations | §6.2 + §6.4 + §6.6 + §11.2 + §11.4 |
| ADR-0110 | Multi-tier per-route evaluation: framework extension `PerRouteConfig.ResolveAllTiers` + `DecoderFilterCallbacks.RequestRouteConfigsAllTiers` callback + per-filter accessor-choice discipline + `most_specific_header_mutations_wins` cross-tier algorithm; **amends (does not supersede) ADR-0073** | BRAINSTORM Decisions 9, 10 + §11.5 confirmation | §5.1 + §6.5 + §6.7 + §6.8 + §11.5 |
| ADR-0111 | Protected-header set per §11.1 (`{:method, :path, :authority, :scheme, :status, host}` case-insensitive on `host`) + **CONFIG-LOAD-TIME** rejection (NOT runtime silent-no-op as BRAINSTORM hypothesized) + verbatim error message format `"header_mutation: %q is :-prefixed or host; may not be modified"` mirroring Envoy's `:-prefixed or host headers may not be modified` | BRAINSTORM Decision 11 (REPURPOSED from "silent runtime no-op" to "config-load-time rejection") + §11.1 amendment | §1.1 + §6.1 step 3 + §6.7 + §11.1 |
| ADR-0112 | `mutations.query_parameter_mutations[]` deferred — coupled to `KeyValueMutation` triple + path/query rewriting subsystem (deferral ADR per ADR-0040 format) | Decision (deferral; §2.1 / §8.1 BRAINSTORM) | §2.1 |
| ADR-0113 | Header-value formatter substitution (`%REQ(:path)%` etc) deferred — full Envoy command-string subsystem is its own multi-phase project (deferral ADR per ADR-0040 format) | Decision (deferral; §2.1 / §8.2 BRAINSTORM) | §2.1 |

Each ADR follows the project's standard 7-section format (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). ADRs land incrementally per `superpowers:executing-plans` PROGRESS preamble convention; the SPEC author is NOT responsible for writing the ADRs in this commit (they are written by the PLAN executor at the task that anchors each ADR).

### 8.1 Consolidation candidates

- **ADR-0114 (BRAINSTORM-anticipated stats absence ADR) → DROPPED.** Per §1.1 + §11.3 confirmation, header_mutation emits zero stats. A standalone ADR for "no stats" carries no design content beyond the BEHAVIOR_CONTRACT.md `## Stat-name mapping` table being unchanged. The discipline is documented inline in §13.1's BEHAVIOR_CONTRACT subsection (one paragraph); promoting it to an ADR adds bookkeeping without value. The cors precedent (per ADR-0074) similarly does NOT have a standalone "no stats" ADR. Final consolidation choice is the planner's; SPEC author RECOMMENDS DROP.
- **ADR-0108 + ADR-0109 consolidation candidate.** Per the 09 SPEC §8.1 precedent, the planner MAY consolidate ADR-0108 (package shape) into ADR-0109 (runtimeConfig + parser) §Consequences if the ADR-0108 text is short. SPEC author RECOMMENDS keep separate per the ADR-0100 + ADR-0101 phase-09 precedent (separate ADRs for package shape vs. parser keep grep-ability per the project's discipline).
- **ADR-0112 + ADR-0113 consolidation candidate.** Both are deferral ADRs per ADR-0040 format. The planner MAY consolidate into a single "phase 10 deferrals" ADR. SPEC author RECOMMENDS keep separate per ADR-0040 precedent: one deferral per ADR for grep-ability when future phases land the deferred surface.

---

## 9. Sibling-stub discipline (per BRAINSTORM §1.5 + Decision 13 + ADR-0106)

Phase 10 does NOT create a sibling-phase SPEC stub for the next §9 HTTP-filters-family phase. Future family-expansion brainstorms cold-start from the §9 `### HTTP filters family` heading + the just-shipped artefacts (cors @ 07.1, fault @ 09, header_mutation @ 10) as their context. Per BRAINSTORM Decision 13 + ADR-0106(b) + (e): stubs would risk pre-populating implicit per-phase rows. The 08.1-creates-08.2-stub precedent was a sub-phase-split-within-parent pattern; family-expansion has no parent SPEC, so no stub.

---

## 10. Acceptance review claims (the items the §5 reviewer must confirm)

### 10.1 Lifecycle correctness

- ROADMAP row `10` exists with status `in-progress` AT this commit (per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3); flips to `done` at the phase-done commit.
- `docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md` exists with §§1–16 populated.
- `docs/envoy-go/phases/10-http-filter-header-mutation/PLAN.md` does NOT exist at this commit (PLAN.md is the next session's deliverable per the SKILL_ROUTING state-machine state 3 → 4).
- `docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md` does NOT exist at this commit.
- `docs/envoy-go/STATE.md` updated to `lifecycle-state: 3`, `next-skill: superpowers:writing-plans` (PLAN.md drafting), `active-phase: 10-http-filter-header-mutation`, `last-commit: <THIS commit SHA>` (SHA-fill follow-up commit). Updates land at this SPEC-authoring session per the cold-start exit-contract (NOTE: phase 09's SPEC §10.1 placed the STATE.md update at the orchestrating session; phase 10 places it in this SPEC commit per the project's convergent convention).

### 10.2 Empirical-pin discipline

- §11 contains all five pins per BRAINSTORM §9 with verbatim Envoy v1.37.2 scrape evidence.
- Each pin's "Conclusions (pinned)" block names the BRAINSTORM Decision it settles + the ADR it anchors + the BEHAVIOR_CONTRACT section it lands in.
- The two pins that AMEND BRAINSTORM hypotheses (§11.1 protected-header rejection is config-load-time not runtime; §11.5 cross-tier ordering matches proto comment exactly — strictly-speaking a CONFIRMATION not an AMENDMENT, but enumerated under §11 for completeness) are explicitly marked as `**SURPRISE — empirical evidence diverges from BRAINSTORM hypothesis**` per the 08.2 / 09 precedent. §11.1 is the MAJOR surprise (BRAINSTORM said silent runtime no-op; reality is hard config-load error).

### 10.3 Scope envelope

- §1.1 lists 5 in-scope architectural primitives (programmable request-mutation, programmable response-mutation, multi-tier per-route resolution, cross-tier ordering flag, package + registration). The BRAINSTORM §1.1's 8 in-scope items collapse to 5 architectural primitives (BRAINSTORM lists them as 8 line-items per the cors/fault precedent's enumeration style; the architectural-primitives count is 5).
- §2 enumerates ALL non-purposes per BRAINSTORM §8 + the §11.1 amendment's protected-header config-load coupling (which is now an in-scope BOOT-time check rather than an out-of-scope runtime no-op).
- §4 deliverables match BRAINSTORM §3.1 + §3.2 with NO stats-registration delta (zero new stats per §11.3).
- §7 fixture has 4 scenarios (not 5 per BRAINSTORM §6.2; the protected-headers scenario drops per §11.1 — migrates to unit-test territory per §14.1).
- §8 has 6 ADRs (not 7 per BRAINSTORM §7; ADR-0114 stats-absence is folded into §13.1 inline per §8.1 consolidation).

### 10.4 No 09-introduced regressions

- All pre-existing fixtures `0000–0011` remain green at phase-done time. The phase-10 implementation TOUCHES no existing fixture's bootstrap or driver — header_mutation is plugged into a new fixture only.
- No 09 contract claim is invalidated by phase 10. Specifically: the fault filter (09 ADR-0100..ADR-0107) is unaffected; the cors filter (07.1 ADR-0074) is unaffected; the http-filter framework (07.1 ADR-0071/72/73/74/75) is unaffected by ADR-0110's `ResolveAllTiers` addition (purely additive; cors and fault continue to use `Resolve`); the per-route 3-tier merge (ADR-0073) is amended (not superseded) — the most-specific-override discipline remains the default for filters that don't opt into multi-tier evaluation.

---

## 11. Empirical-pin block (per BRAINSTORM §9 — all five pins resolved IN-SESSION)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline (autonomous-brainstorm requires empirical evidence for design decisions that are not derivable from documentation alone). Mirrors phase 09 SPEC §11's structure precisely.

**Reference image:** `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per `ENVOY_TARGET.md` + 08.1/08.2/09 SPEC §11 confirmation).

**Probe configuration:** A Docker bridge network `envoy-10-net` hosts: (a) a Python `http.server` echo backend (image `python:3.12-slim`; container alias `envoy10-backend`; bound to port 18001 within the bridge; serves a fast 200 OK reflecting received request headers into the response body, plus a single-value `X-Resp-Test` and a multi-value `X-Multi` response header) — script source at `/tmp/envoy-10-pins/echo_backend.py` of the SPEC drafting session machine; (b) an `envoyproxy/envoy:v1.37.2` reference proxy (container alias `envoy10-proxy`) booted under various per-pin bootstrap YAMLs — admin :9901, listener :10000 (and :10001 for the multi-listener P5 probe). Probe curl invocations issued from a `curlimages/curl` container on the same bridge network. The capture transcripts at `/tmp/envoy-10-pins/p*-results.txt` of the SPEC-drafting session machine are transient artifacts not committed; the verbatim outputs below are the durable evidence per the 09 SPEC §11 line 691 discipline.

Probe date: 2026-05-04.

### 11.1 Empirical pin #1 — Protected-header set + config-load enforcement

**Probe configuration:** envoy.yaml with `header_mutation` filter at the listener level attempting `OVERWRITE_IF_EXISTS_OR_ADD` against EACH candidate header individually (one config per candidate, iterated in a shell loop). Candidates: `:method`, `:path`, `:authority`, `:scheme`, `:status`, `host`, `Host`, `HOST`, `content-length`, `transfer-encoding`, `connection`, `keep-alive`, `te`, `upgrade`, `proxy-connection`, `x-envoy-internal`, `x-envoy-decorator-operation`, `x-envoy-attempt-count`, `x-forwarded-for`, `x-request-id`, `x-forwarded-proto`, `x-envoy-original-path`, `user-agent`. The container is allowed up to 6 seconds to start; boot-failure exits within ~1s with the protected-header error.

**Verbatim Envoy boot-failure tail (sample for `:method`):**

```
[2026-05-04 14:39:21.804][1][critical][main] [source/server/server.cc:453] error `:-prefixed or host headers may not be modified` initializing config 'goo.gle/debugproto
  /envoy.yaml'
:-prefixed or host headers may not be modified
```

**Iteration result table (pass/fail per candidate):**

```
REJECTED: :method
REJECTED: :path
REJECTED: :authority
REJECTED: :scheme
REJECTED: :status
REJECTED: host
REJECTED: Host
REJECTED: HOST
ALLOWED:  content-length
ALLOWED:  transfer-encoding
ALLOWED:  connection
ALLOWED:  keep-alive
ALLOWED:  te
ALLOWED:  upgrade
ALLOWED:  proxy-connection
ALLOWED:  x-envoy-internal
ALLOWED:  x-envoy-decorator-operation
ALLOWED:  x-envoy-attempt-count
ALLOWED:  x-forwarded-for
ALLOWED:  x-request-id
ALLOWED:  x-forwarded-proto
ALLOWED:  x-envoy-original-path
ALLOWED:  user-agent
```

**Per-route subprobe (confirms protection symmetric across listener-level and per-route configs):** envoy.yaml with `:path` mutation in a per-route `HeaderMutationPerRoute` typed_per_filter_config (NOT in the listener-level filter config) → SAME boot failure with the SAME `:-prefixed or host headers may not be modified` error. Per-route protected-header rejection is also config-load-time.

**Response-side subprobe (confirms protection applies to `response_mutations` too):** envoy.yaml with `:status` mutation in `mutations.response_mutations` (instead of request_mutations) → SAME boot failure. The protected set applies symmetrically to request and response mutations.

**Conclusions (pinned):**
- (a) Envoy v1.37.2 enforces protected-header rejection at **CONFIG-LOAD TIME** with a hard error, NOT silent runtime no-op.
- (b) Protected set is exactly: the five `:`-prefixed pseudo-headers (`:method`, `:path`, `:authority`, `:scheme`, `:status`) plus `host` (case-insensitive: `host`, `Host`, `HOST` all rejected). Verbatim error message: `:-prefixed or host headers may not be modified`. The error fires from `source/server/server.cc:453` (Envoy's main config-load path).
- (c) The protection scope spans (i) listener-level filter configs, (ii) per-route `HeaderMutationPerRoute` configs, (iii) `request_mutations`, (iv) `response_mutations` — all four combinations rejected at boot.
- (d) ALL non-protected headers are mutable, including hop-by-hop framing headers (`content-length`, `transfer-encoding`, `connection`, `keep-alive`, `te`, `upgrade`, `proxy-connection`), Envoy-internal coordination headers (`x-envoy-internal`, `x-envoy-decorator-operation`, `x-envoy-attempt-count`, `x-envoy-original-path`), tracing headers (`x-request-id`), forwarding headers (`x-forwarded-for`, `x-forwarded-proto`), and arbitrary headers (`user-agent`). Operators are responsible for not mutating headers that would degrade behavior (e.g., `content-length` corrupting the message body).
- (e) **MAJOR SURPRISE — empirical evidence diverges from BRAINSTORM hypothesis:** BRAINSTORM Decision 11 said "mutation attempts on protected headers are SILENT NO-OPS (no error, no log, no stat)". The reality is a hard CONFIG-LOAD ERROR at boot — far stricter than the brainstorm anticipated. envoy-go's `New` factory MUST mirror by validating each `compiledMutationOp.headerName` against the protected set (`{":method", ":path", ":authority", ":scheme", ":status", "host"}` matched case-insensitively on `host` and prefix-matched on `:`-prefix) at parse time and returning a non-nil error from `New` for protected headers. Per-route validation happens at HCM-build time (when `BuildPerRouteConfig` parses the per-route any-blob) OR lazily at first per-route resolution (per §6.7 + §12 deferred decision 3); either way, the error surface to the operator MUST be at boot, not at first request.
- (f) ADR-0072's boot-time-fail-fast contract makes this ergonomic (the registry resolves typed_config at HCM-build time, BEFORE any traffic). envoy-go's verbatim error format: `"header_mutation: %q is :-prefixed or host; may not be modified"` (mirrors Envoy's wording with the offending header name in quotes).
- (g) Settles BRAINSTORM Decision 11 / §9.P1 deferred-pin question — protection is hard-coded set of 6 names + config-load enforcement, NOT silent runtime no-op.
- (h) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.header_mutation` protected-header paragraph (§13.1) and ADR-0111 §Decision section.

### 11.2 Empirical pin #2 — `keep_empty_value` boundary

**Probe configuration:** envoy.yaml with `header_mutation` listener-level filter applying mutations with empty values across the 4 AppendAction variants, both with `keep_empty_value=false` (default) and `=true`. Mutations include OVERWRITE_IF_EXISTS variants against (a) absent target headers and (b) pre-existing target headers (seeded by sending request with `X-Pre-Existing` and `X-Pre-Existing-Keep`). Probe: `curl -sS http://envoy10-proxy:10000/echo -H 'X-Pre-Existing: original-value' -H 'X-Pre-Existing-Keep: original-value'`.

**Verbatim Envoy probe — request headers as seen by upstream backend (response body reflects them):**

```
host: envoy10-proxy:10000
user-agent: curl/8.20.0
accept: */*
x-pre-existing: original-value
x-forwarded-proto: http
x-request-id: 027a10d1-262f-4f91-b7de-80ebceee812c
x-empty-append-keep: 
x-empty-add-if-absent-keep: 
x-empty-overwrite-or-add-keep: 
x-pre-existing-keep: 
x-mutation-applied: yes
x-envoy-expected-rq-timeout-ms: 15000
```

**Conclusions (pinned):**
- (a) `keep_empty_value=false` (default) + empty value: **silent skip** — the op is no-op regardless of AppendAction. None of the `x-empty-*-default` headers appear in the upstream request (e.g. `x-empty-append-default`, `x-empty-add-if-absent-default`, `x-empty-overwrite-or-add-default`, `x-empty-overwrite-default`).
- (b) `keep_empty_value=true` + empty value: **materialized** — `x-empty-*-keep` headers appear with empty values, subject to the AppendAction conditional gate. Specifically:
    - `x-empty-append-keep: ` (APPEND_IF_EXISTS_OR_ADD with empty value + keep): materialized as empty-value header.
    - `x-empty-add-if-absent-keep: ` (ADD_IF_ABSENT with empty value + keep, target absent): materialized as empty-value header.
    - `x-empty-overwrite-or-add-keep: ` (OVERWRITE_IF_EXISTS_OR_ADD with empty value + keep, target absent): materialized as empty-value header.
    - `x-pre-existing-keep: ` (OVERWRITE_IF_EXISTS with empty value + keep, target present `original-value`): replaced original-value with empty value.
    - `x-empty-overwrite-keep` is NOT present (OVERWRITE_IF_EXISTS with empty value + keep, target ABSENT): the EXISTS gate fires regardless of keep_empty_value; absent target → no-op.
    - `x-pre-existing` is UNCHANGED (still `original-value`): OVERWRITE_IF_EXISTS with empty value + keep_empty_value=false → silent skip per (a) BEFORE the EXISTS gate is evaluated. The keep_empty_value check fires FIRST.
- (c) The algorithm is therefore: `if value == "" && !keep_empty_value: return` BEFORE any AppendAction switch; otherwise run the AppendAction switch normally (which may further conditionally apply per `ADD_IF_ABSENT` / `OVERWRITE_IF_EXISTS` gates).
- (d) Settles BRAINSTORM Decision 8 / §9.P2 deferred-pin question — confirmation of the natural reading; codified in §6.6 algorithm.
- (e) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.header_mutation` keep-empty-value paragraph (§13.1) and ADR-0109 §Decision section.

### 11.3 Empirical pin #3 — Filter stats verification

**Probe configuration:** Same envoy.yaml as §11.2 (active header_mutation filter with multiple mutations); drove 5 sequential requests through the filter, then scraped admin /stats and /stats?format=prometheus and grep'd for `header_mutation` and `header.mutation`.

**Verbatim Envoy `/stats?filter=header_mutation` — empty (no matching stats):**

```
(no output)
```

**Verbatim Envoy `/stats` — only header-related stats (none from header_mutation filter):**

```
cluster.c_backend.http1.dropped_headers_with_underscores: 0
cluster.c_backend.http1.requests_rejected_with_underscores_in_headers: 0
http.admin.downstream_rq_header_timeout: 0
http.ingress_http.downstream_rq_header_timeout: 0
http1.dropped_headers_with_underscores: 0
http1.requests_rejected_with_underscores_in_headers: 0
```

**Verbatim Envoy `/stats?format=prometheus` — only header-related stats (none from header_mutation filter):**

```
# TYPE envoy_cluster_http1_dropped_headers_with_underscores counter
envoy_cluster_http1_dropped_headers_with_underscores{envoy_cluster_name="c_backend"} 0
# TYPE envoy_cluster_http1_requests_rejected_with_underscores_in_headers counter
envoy_cluster_http1_requests_rejected_with_underscores_in_headers{envoy_cluster_name="c_backend"} 0
# TYPE envoy_http_downstream_rq_header_timeout counter
envoy_http_downstream_rq_header_timeout{envoy_http_conn_manager_prefix="admin"} 0
envoy_http_downstream_rq_header_timeout{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http1_dropped_headers_with_underscores counter
envoy_http1_dropped_headers_with_underscores{} 0
# TYPE envoy_http1_requests_rejected_with_underscores_in_headers counter
envoy_http1_requests_rejected_with_underscores_in_headers{} 0
```

(All `header*` matches are framework-internal stats from the HCM and the codec — `dropped_headers_with_underscores`, `requests_rejected_with_underscores_in_headers`, `downstream_rq_header_timeout`. NONE are emitted by the `envoy.filters.http.header_mutation` filter itself.)

**Conclusions (pinned):**
- (a) Envoy v1.37.2's `envoy.filters.http.header_mutation` filter emits **ZERO** stats. The filter has no stat_prefix field in its proto; no `header_mutation.*` namespace exists in the admin /stats output. CONFIRMS the brainstorm's POSITION.
- (b) Phase 10 emits zero stats. The 22-name `## Stat-name mapping` table (extended by phase 09) is UNCHANGED in phase 10. ADR-0114 (BRAINSTORM-anticipated stats-absence ADR) is folded into §13.1's BEHAVIOR_CONTRACT subsection inline per §8.1 consolidation; no standalone ADR.
- (c) header_mutation's `New` factory does NOT consume `ctx.Stats` or `ctx.StatPrefix` (the 3-field `FactoryCtx` per ADR-0100 stays as-is; phase 10 simply does not exercise the stat-bearing fields). The cors precedent (per ADR-0074) similarly has no stats; the pattern is established.
- (d) Settles BRAINSTORM Decision 12 / §9.P3 deferred-pin question — zero stats confirmed; route A applies (no-emit discipline, analogous to cors per ADR-0074).
- (e) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.header_mutation` no-stats paragraph (§13.1).

### 11.4 Empirical pin #4 — AppendAction × 4 multi-valued-header behavior

**Probe configuration:** envoy.yaml with `header_mutation` listener-level filter applying:
- `request_mutations`: `OVERWRITE_IF_EXISTS_OR_ADD` (`accept-encoding` → `OVERWRITTEN`); `APPEND_IF_EXISTS_OR_ADD` (`x-list` → `appended`).
- `response_mutations`: `OVERWRITE_IF_EXISTS_OR_ADD` (`x-multi` → `OVERWRITTEN`); `APPEND_IF_EXISTS_OR_ADD` (`x-multi` → `APPENDED`); `OVERWRITE_IF_EXISTS_OR_ADD` (`set-cookie` → `OVERWRITTEN`); `OVERWRITE_IF_EXISTS` (`x-absent` → `shouldnotappear`); `ADD_IF_ABSENT` (`x-resp-test` → `shouldbeskipped`); `ADD_IF_ABSENT` (`x-resp-added` → `added-via-add-if-absent`).

Probe: `curl -isS http://envoy10-proxy:10000/echo -H 'Accept-Encoding: gzip' -H 'Accept-Encoding: deflate' -H 'Accept-Encoding: br' -H 'X-List: one' -H 'X-List: two'` (multi-value source headers via repeated header lines).

**Verbatim Envoy probe — full -i output:**

```
HTTP/1.1 200 OK
server: envoy
date: Mon, 04 May 2026 14:37:52 GMT
content-type: text/plain
content-length: 245
x-resp-test: backend-original
x-envoy-upstream-service-time: 0
x-multi: OVERWRITTEN
x-multi: APPENDED
set-cookie: OVERWRITTEN
x-resp-added: added-via-add-if-absent

host: envoy10-proxy:10000
user-agent: curl/8.20.0
accept: */*
x-list: one
x-list: two
x-forwarded-proto: http
x-request-id: 545b577c-3084-4483-8975-396f4439cfdc
accept-encoding: OVERWRITTEN
x-list: appended
x-envoy-expected-rq-timeout-ms: 15000
```

**Conclusions (pinned):**

Request-side (mutations applied to request headers; backend echoes them in body):
- (a) `accept-encoding` original was 3 values (`gzip`, `deflate`, `br` as 3 separate header lines). After `OVERWRITE_IF_EXISTS_OR_ADD`: the upstream sees ONE `accept-encoding: OVERWRITTEN` line. **OVERWRITE_IF_EXISTS_OR_ADD collapses multi-value to single value** (consistent with Go's `http.Header.Set(name, value)` semantics).
- (b) `x-list` original was 2 values (`one`, `two`). After `APPEND_IF_EXISTS_OR_ADD`: upstream sees THREE entries (`x-list: one`, `x-list: two`, `x-list: appended`). **APPEND_IF_EXISTS_OR_ADD preserves prior values + adds one more** (consistent with Go's `http.Header.Add(name, value)` semantics).

Response-side (mutations applied to response headers; visible to downstream client directly):
- (c) `x-multi` from backend was 2 values (`alpha`, `beta`). After `OVERWRITE_IF_EXISTS_OR_ADD` (collapses to single `OVERWRITTEN`), then `APPEND_IF_EXISTS_OR_ADD` (adds `APPENDED`): downstream sees TWO values (`x-multi: OVERWRITTEN`, `x-multi: APPENDED`). Confirms order-of-application matters: OVERWRITE first → 1 value, then APPEND → 2 values.
- (d) `set-cookie` from backend was 2 cookies (`a=1`, `b=2`). After `OVERWRITE_IF_EXISTS_OR_ADD`: downstream sees ONE `set-cookie: OVERWRITTEN`. **Multi-value collapse applies to `set-cookie` too** (which Go's `net/http` normally treats specially — but Envoy's behavior is uniform: Set-Cookie is collapsed like any other multi-value header).
- (e) `x-absent` (target absent) with `OVERWRITE_IF_EXISTS`: NOT present in response (silent no-op). Confirms EXISTS conditional gate.
- (f) `x-resp-test` (present from backend as `backend-original`) with `ADD_IF_ABSENT` of `shouldbeskipped`: STILL `backend-original` in response (silent no-op). Confirms ABSENT conditional gate.
- (g) `x-resp-added` (target absent) with `ADD_IF_ABSENT` of `added-via-add-if-absent`: present in response. Confirms ABSENT-then-add behavior.

- (h) Settles BRAINSTORM Decision 7 / §9.P4 deferred-pin question — confirmation of the natural reading; the AppendAction × 4 mapping table in §6.6 / ADR-0109 matches Envoy's behavior.
- (i) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.header_mutation` AppendAction × 4 paragraph (§13.1) and ADR-0109 §Decision section.

### 11.5 Empirical pin #5 — `most_specific_header_mutations_wins` evaluation order

**Probe configuration:** envoy.yaml with TWO listeners (`l_lws` on :10000 with flag=false; `l_mws` on :10001 with flag=true) sharing identical per-route tier configurations. The route_config (rc), virtual_host (vh), and route tiers each carry a `HeaderMutationPerRoute` with one `OVERWRITE_IF_EXISTS_OR_ADD` of `x-test` to a tier-distinguishing value: rc=`rc`, vh=`vh`, route=`route`. The listener-level filter sets `x-test=listener` (always-first per the proto comment).

**Verbatim probes:**

```
===== flag=false (LWS — least-specific wins) =====
x-test: rc

===== flag=true  (MWS — most-specific wins) =====
x-test: route
```

(Showing only the relevant `x-test` lines from the upstream-echoed request headers; all other request headers are normal forwarding artifacts.)

**Conclusions (pinned):**
- (a) `most_specific_header_mutations_wins=false` (DEFAULT): final upstream `x-test: rc`. The cross-tier evaluation order is Route (most-specific) FIRST → VirtualHost → RouteConfiguration (least-specific) LAST. RouteConfiguration's value sticks because it was applied last in the chain of OVERWRITEs.
- (b) `most_specific_header_mutations_wins=true`: final upstream `x-test: route`. The cross-tier evaluation order is reversed: RouteConfiguration FIRST → VirtualHost → Route (most-specific) LAST. Route's value sticks.
- (c) Listener-level mutation (`x-test: listener`) is never the final value — overwritten by per-route tiers in both flag modes. Confirms the proto comment at `header_mutation.pb.go:141–142`: "filter configuration will always be applied first" (listener-level always FIRST, then per-route tiers in flag-controlled order).
- (d) The proto comment at `header_mutation.pb.go:149–154` matches Envoy's actual behavior verbatim (no idiosyncratic divergence). The §6.5 algorithm is correct as-stated.
- (e) Settles BRAINSTORM Decision 10 / §9.P5 deferred-pin question — proto-comment-vs-actual match confirmed; algorithm in §6.5 is the durable form.
- (f) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.header_mutation` cross-tier-ordering paragraph (§13.1) and ADR-0110 §Decision section.

### 11.6 Synchronization with BEHAVIOR_CONTRACT.md

The §11 pins above land in `BEHAVIOR_CONTRACT.md` per §13.1 (asserted-equivalence prose for the new `### envoy.filters.http.header_mutation` subsection). The 22-name stat-name mapping table is UNCHANGED (per §11.3). The timing-tolerances section is UNCHANGED (synchronous filter). §13.4 (equivalence-matrix new row) is the only other §13 patch. The PLAN executor follows the prose in §13 verbatim (per ADR-0052 in-place edit discipline) at the phase-done commit.

---

## 12. Deferred decisions (the planner / implementer settles these)

The following design-leaf decisions are deferred to PLAN authoring (lifecycle-state 3 → 4) or PLAN execution (lifecycle-state 4 → 5):

1. **`RequestRouteConfigsAllTiers` callback symmetry — Decoder vs. Decoder+Encoder.** §6.6 / §6.8 reference both `dcb.RequestRouteConfigsAllTiers` (decode-side) and `ecb.ResponseRouteConfigsAllTiers` (encode-side). The framework's existing `RequestRouteConfig` is on the Decoder interface only (per `internal/filter/http/callbacks.go`); the encode-side equivalent currently does NOT exist. Phase 10 introduces the multi-tier accessor; the planner chooses whether to add it on BOTH callback interfaces (symmetric) or only on Decoder + cache the per-tier configs in the per-instance `*filter` from decode-side for re-use at encode-side. Recommendation: add to BOTH callback interfaces symmetrically (the chain's `routeIdx` is per-stream and stable across decode and encode; the per-instance cache approach adds two pointer fields to `*filter` and a sync.Once-style first-resolve guard, which is more code than the symmetric callback addition). The cors precedent (which uses `routePolicy()` helper at both decode and encode time per `cors.go:163`) suggests the symmetric callback is natural.
2. **Per-request per-tier cache.** §6.6 / §6.8 leave open whether to cache the resolved `routeOps` / `vhOps` / `rcOps` in the per-instance `*filter` after first decode-side resolution to avoid re-resolving + re-compiling at encode-side. Recommendation: SKIP caching; the per-tier proto.Message lookup is sub-microsecond, and the `compiledMutationOp` slice rebuild is also sub-microsecond on small per-route configs (typical: < 5 ops per tier). Add caching only if profiling under fixture 0012 shows measurable cost.
3. **Per-route protected-header validation lifecycle.** §6.7 leaves open whether to validate per-route configs at HCM-build time (eager) or at first per-route resolution per filter-instance (lazy). Eager validation surfaces operator errors at boot (the same boot-fail-fast as listener-level configs); lazy validation surfaces them at first request through the offending route. Recommendation: EAGER. Per the 09 SPEC §12 deferred decision precedent, the planner adds a post-`BuildPerRouteConfig` validation hook that the header_mutation filter's `New` factory triggers. Implementation sketch: header_mutation's `New` registers a per-route-validator function with the framework; at HCM-build time after `BuildPerRouteConfig`, the framework iterates `chainNames` and calls each filter's per-route-validator on the parsed proto.Message values for that filter name. Returns an error if validation fails. ~40 LoC framework delta on top of ADR-0110's ~80 LoC.
4. **`compiledMutationOp` slice element type — value vs. pointer.** §6.4 leaves open whether `runtimeConfig.requestOps []compiledMutationOp` is value-typed or pointer-typed. Recommendation: VALUE-TYPED. The struct is small (~5 fields, ~40 bytes); value semantics improve cache locality during the apply-loop iteration. Pointer semantics only win if the slice is mutated post-construction, which it is not (read-only after `New`).
5. **Where to define the protected-header set constants.** §6.1 step 3 references the set `{":method", ":path", ":authority", ":scheme", ":status", "host"}`. The planner chooses whether to declare these as a `var protectedHeaders = map[string]bool{...}` in `header_mutation.go`, as a `const` set with a helper `isProtectedHeader(name string) bool`, or via prefix-check on `:` plus equality-check on canonical-`Host`. Recommendation: prefix-check + equality, since the `:`-prefixed set is open-ended at the spec level (Envoy may add `:protocol` or `:upgrade` in future) and a prefix check future-proofs against new pseudo-headers.
6. **Fuzzer mandatory or optional?** The SPEC author RECOMMENDS shipping `FuzzHeaderMutationConfigParse` per §14.5 (header_mutation's `New` factory is a parser; ADR-0018's "every parser/codec/filter ships a fuzzer" applies). Final shipping decision is the planner's; recommendation: SHIP (~50 LoC, low cost).
7. **Race-detector test for the multi-tier evaluation.** Recommendation: ADD `TestHeaderMutation_MultiTierConcurrentRequests` under `-race` that fires DecodeHeaders concurrently with shared `*runtimeConfig` (multiple per-instance `*filter` instances spawned in parallel). The framework's per-instance discipline makes the race trivially safe, but the race detector run validates by construction. ~30 LoC.
8. **Whether to expose `applyOps` as a package-level helper for testing.** Recommendation: KEEP unexported. Unit tests access via the public `New` + `DecodeHeaders` / `EncodeHeaders` surface, which is the canonical contract. Exposing `applyOps` would tempt drive-by testing of internals.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

### 13.1 `## HTTP filter chain ### envoy.filters.http.header_mutation` NEW subsection (verbatim Markdown patch)

The patch INSERTS a new `### envoy.filters.http.header_mutation` subsection AFTER the existing `### envoy.filters.http.fault` subsection landed by phase 09 (line ~832 + the phase-09 fault subsection extent). Insertion point: at the end of the `## HTTP filter chain` umbrella section, after fault.

```markdown
### envoy.filters.http.header_mutation

#### Asserted equivalence (per phase 10 SPEC §11)

When `envoy.filters.http.header_mutation` is present in `http_filters`, envoy-go MUST emit the same post-mutation request headers (visible at the upstream backend) and post-mutation response headers (visible at the downstream client) as reference Envoy v1.37.2 for the canonical mutation scenarios.

- **Request-side mutations** (per `mutations.request_mutations[]`): applied in proto-declared order in `DecodeHeaders` BEFORE the request reaches the upstream router. Each mutation is one of `Remove` (deletes the named header) or `Append` with one of 4 `AppendAction` variants:
    - `APPEND_IF_EXISTS_OR_ADD` (default; enum 0): `headers.Add(name, value)` — multi-valued header gets one more value; absent target gets first value.
    - `ADD_IF_ABSENT` (1): conditional add if and only if the target is absent (`headers.Get(name) == ""`).
    - `OVERWRITE_IF_EXISTS_OR_ADD` (2): `headers.Set(name, value)` — collapses any multi-valued slot to a single value; absent target gets first value.
    - `OVERWRITE_IF_EXISTS` (3): conditional set if and only if the target is present.
- **Response-side mutations** (per `mutations.response_mutations[]`): applied in proto-declared order in `EncodeHeaders` BEFORE the response writes to the wire. Same 4 AppendAction variants + Remove. envoy-go's `EncodeHeaders` runs in REVERSE filter-list order per ADR-0075; header_mutation's response_mutations apply AFTER any later-in-list filter's encode-side mutations.
- **`keep_empty_value` semantics**: when an Append op has `value == ""` and `keep_empty_value=false` (default), the op is a SILENT NO-OP regardless of AppendAction. When `keep_empty_value=true`, the empty value is materialized subject to the AppendAction's conditional gate (e.g., `OVERWRITE_IF_EXISTS` with empty value + keep + absent target = no-op; with present target = replace value with empty).
- **Multi-valued header behavior** (per phase 10 SPEC §11.4): `OVERWRITE_*` collapses multi-valued slots to a single value (the new one). `APPEND_IF_EXISTS_OR_ADD` preserves prior values + adds one more (resulting in N+1 values). Applies to all multi-valued headers including `Set-Cookie`, `Vary`, `Cache-Control`.

#### Multi-tier per-route evaluation (per ADR-0110 + phase 10 SPEC §11.5)

Per-route `typed_per_filter_config` for `envoy.filters.http.header_mutation` is evaluated at ALL THREE tiers (Route, VirtualHost, RouteConfiguration), NOT merged most-specific-only. This is structurally different from cors / fault per-route handling (which use most-specific override per ADR-0073). The cross-tier ordering is controlled by the listener-level `most_specific_header_mutations_wins` flag:

- **`most_specific_header_mutations_wins=false`** (DEFAULT): Listener-level mutations applied FIRST, then per-route tiers in order Route → VirtualHost → RouteConfiguration. RouteConfiguration's mutations are applied LAST → least-specific-wins overlap.
- **`most_specific_header_mutations_wins=true`**: Listener-level mutations applied FIRST, then per-route tiers in REVERSED order RouteConfiguration → VirtualHost → Route. Route's mutations are applied LAST → most-specific-wins overlap.

Each tier's mutations are applied in proto-declared order WITHIN the tier (the cross-tier flag controls only the inter-tier sequence). Listener-level mutations are ALWAYS applied first regardless of the flag.

Empirically confirmed: with listener `x-test=listener`, RouteConfiguration `x-test=rc`, VirtualHost `x-test=vh`, Route `x-test=route` (all OVERWRITE_IF_EXISTS_OR_ADD): flag=false → final `x-test: rc`; flag=true → final `x-test: route`.

#### Protected-header set (per ADR-0111 + phase 10 SPEC §11.1)

Envoy v1.37.2 enforces a hard-coded protected-header set at CONFIG-LOAD TIME: a `header_mutation` config attempting to mutate any protected header causes Envoy to refuse to boot with a verbatim error `:-prefixed or host headers may not be modified`. The protected set is exactly:

- All five `:`-prefixed pseudo-headers: `:method`, `:path`, `:authority`, `:scheme`, `:status`.
- The HTTP/1.1 `host` header (case-insensitive: `host`, `Host`, `HOST` all rejected).

Protection scope spans listener-level filter configs, per-route `HeaderMutationPerRoute` configs, `request_mutations`, and `response_mutations` — all four combinations rejected at boot.

envoy-go MIRRORS this discipline by validating each `compiledMutationOp.headerName` against the protected set at `New` time (listener-level) and at HCM-build time (per-route, per §6.7 / §12 deferred decision 3); the verbatim error format is `header_mutation: %q is :-prefixed or host; may not be modified`. Boot-time-fail-fast per ADR-0072 — a misconfigured protected-header mutation surfaces as a non-zero exit before the listener accepts traffic.

#### Stats — none emitted (per phase 10 SPEC §11.3)

`envoy.filters.http.header_mutation` emits ZERO stats. The proto has no `stat_prefix` field; no `header_mutation.*` namespace exists in `/stats` or `/stats?format=prometheus`. envoy-go matches: zero stats. The `## Stat-name mapping ### 22-name table` (extended by phase 09) is UNCHANGED in phase 10.

(Cors @ 07.1 also emits zero stats per ADR-0074. The pattern is established: not every HTTP filter is stat-bearing.)

#### Does not yet apply to (per phase 10 deferrals — ADRs 0112, 0113)

- **`mutations.query_parameter_mutations[]`** (KeyValueMutation triple): path-query rewriting deferred per ADR-0112. envoy-go silently parses the field but does not honor it; configured query-parameter mutations are no-ops.
- **Header-value formatter substitution** (`%REQ(:path)%`, `%DOWNSTREAM_REMOTE_ADDRESS%`, `%RESPONSE_CODE%`, etc.): formatter syntax deferred per ADR-0113. envoy-go materializes header values as STATIC strings verbatim; a configured value of `"%REQ(:path)%"` produces the literal 11-byte string on the wire, not the substituted path.
- **Differential testing under H2 streams**: fixture 0012 is HTTP/1.1-only; H2 differential testing of header_mutation is deferred.
- **Cross-filter interaction tests** (header_mutation × cors, header_mutation × fault): fixture 0012 is header_mutation + router only; cross-filter encode-side ordering with sibling encode-mutating filters is deferred to whatever phase lands the second encode-mutating filter.

#### Empirical evidence (verbatim curl excerpts from phase 10 SPEC §11)

```
$ curl -isS http://127.0.0.1:10000/echo  # listener: OVERWRITE x-multi to OVERWRITTEN, then APPEND APPENDED

HTTP/1.1 200 OK
server: envoy
date: Mon, 04 May 2026 14:37:52 GMT
content-type: text/plain
content-length: 245
x-resp-test: backend-original
x-multi: OVERWRITTEN
x-multi: APPENDED
set-cookie: OVERWRITTEN
x-resp-added: added-via-add-if-absent
```

(Multi-value `x-multi`: OVERWRITE collapsed `alpha`/`beta` to single `OVERWRITTEN`, then APPEND added `APPENDED`. Final response carries 2 `x-multi` values per phase 10 §11.4.)
```

### 13.2 `## Stat-name mapping ### 22-name table` extension

**No changes.** Per §11.3, header_mutation emits zero stats. The 22-name table (extended by phase 09 to include 5 fault.* entries) is UNCHANGED in phase 10. Future stat-bearing §9 family-row phases (e.g., local_ratelimit) will extend the table.

### 13.3 `## Timing tolerances` extension

**No changes.** Per the synchronous-filter discipline (no async-resume, no time-bounded operations), header_mutation introduces no timing-tolerance bullet. The existing fault-delay-accuracy bullet from phase 09 is unchanged.

### 13.4 `## Equivalence Matrix` new row (verbatim table-row patch)

The patch APPENDS one new row to the equivalence matrix table.

```markdown
| HTTP filter `envoy.filters.http.header_mutation` | Per-request equivalence on post-mutation request headers (visible at upstream backend) and post-mutation response headers (visible at downstream client) under listener-level + per-route 3-tier configurations, including AppendAction × 4 + Remove + `keep_empty_value` boundary + multi-valued header collapse / preserve semantics + `most_specific_header_mutations_wins` cross-tier ordering (both flag values). Boot-time enforcement of the 6-name protected-header set per ADR-0111 + phase 10 §11.1. Differential gate fixture 0012-http-header-mutation. NOT asserted: header-value formatter substitution (deferred — ADR-0113), `query_parameter_mutations` (deferred — ADR-0112), H2 differential coverage. |
```

### 13.5 Forward-pointer notes (per BRAINSTORM §11 inline supersessions/amendments)

Two small forward-pointer notes are appended to existing BEHAVIOR_CONTRACT sections:

- After `## HTTP filter chain ### typed_per_filter_config 3-tier merge` discussion (the section that codifies ADR-0073's most-specific-override discipline): "Phase 10 (`envoy.filters.http.header_mutation`) introduces the **multi-tier evaluation** model (per ADR-0110 amending ADR-0073). The default model remains most-specific-override per ADR-0073 (used by cors, fault); filters whose proto semantics demand multi-tier evaluation use the framework's `RequestRouteConfigsAllTiers` callback + `PerRouteConfig.ResolveAllTiers` accessor, opting into the multi-tier model per ADR-0110."
- After `## HTTP filter chain ### envoy.filters.http.cors ### Asserted equivalence` (phase 07.1's cors block): "Phase 10 (`envoy.filters.http.header_mutation`) is the SECOND production filter to mutate response headers in `EncodeHeaders` — see `### envoy.filters.http.header_mutation` for the programmable-mutation discipline. Cors injects a fixed 3-header set; header_mutation runs a programmable AppendAction × 4 + Remove pipeline."

---

## 14. Testing strategy (per BRAINSTORM §6 + §11 amendment)

### 14.1 Unit tests (`internal/filter/http/header_mutation/`)

`header_mutation_test.go` covers:

- `TestNew_NilTC` — nil tc returns non-nil error per §6.1 step 1.
- `TestNew_MalformedTC` — malformed Any returns unmarshal error.
- `TestNew_ProtectedHeader_Method` — request_mutations attempting `:method` returns non-nil error mirroring §11.1 verbatim message.
- `TestNew_ProtectedHeader_Path` — `:path` rejected.
- `TestNew_ProtectedHeader_Authority` — `:authority` rejected.
- `TestNew_ProtectedHeader_Scheme` — `:scheme` rejected.
- `TestNew_ProtectedHeader_Status` — `:status` rejected (response_mutations side too).
- `TestNew_ProtectedHeader_Host_LowerCase` — `host` rejected.
- `TestNew_ProtectedHeader_Host_TitleCase` — `Host` rejected (case-insensitive on host).
- `TestNew_ProtectedHeader_Host_UpperCase` — `HOST` rejected.
- `TestNew_ProtectedHeader_ResponseSide` — protected header in `response_mutations` rejected (symmetric).
- `TestNew_HappyPath_ListenerLevelOnly` — valid listener-level config with all 5 op variants returns non-nil factory + nil error.
- `TestRuntimeConfig_FieldExtraction` — verifies the 4-field extraction maps proto values correctly (request_mutations, response_mutations, most_specific flag).
- `TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored` — non-empty `mutations.query_parameter_mutations[]` does NOT error; the field is silently parsed and discarded per §2.1 / ADR-0112.
- `TestCompiledMutationOp_AllAppendActionsParse` — each of the 4 AppendAction enum values (including default 0) maps to the correct `compiledMutationOp.appendAction` value.
- `TestCompiledMutationOp_RemoveAndAppend` — both oneof variants of `HeaderMutation.action` parse correctly.
- `TestApplyOps_AppendIfExistsOrAdd_AbsentTarget` — adds new header value.
- `TestApplyOps_AppendIfExistsOrAdd_PresentTarget` — adds another value (multi-value preservation per §11.4).
- `TestApplyOps_AddIfAbsent_AbsentTarget` — adds.
- `TestApplyOps_AddIfAbsent_PresentTarget` — no-op.
- `TestApplyOps_OverwriteIfExistsOrAdd_AbsentTarget` — adds.
- `TestApplyOps_OverwriteIfExistsOrAdd_PresentMultiValue` — collapses multi-value to single per §11.4.
- `TestApplyOps_OverwriteIfExists_AbsentTarget` — no-op.
- `TestApplyOps_OverwriteIfExists_PresentTarget` — replaces.
- `TestApplyOps_Remove_PresentTarget` — deletes.
- `TestApplyOps_Remove_AbsentTarget` — no-op (idempotent delete).
- `TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions` — silent skip across all 4 variants per §11.2.
- `TestApplyOps_KeepEmptyValueTrue_EmptyValue_AppendIfExistsOrAdd` — materializes empty value per §11.2.
- `TestApplyOps_KeepEmptyValueTrue_EmptyValue_OverwriteIfExists_AbsentTarget` — silent no-op (EXISTS gate fires) per §11.2.
- `TestApplyOps_KeepEmptyValueTrue_EmptyValue_OverwriteIfExists_PresentTarget` — replaces with empty per §11.2.
- `TestDecodeHeaders_ListenerLevel_NoPerRoute` — listener-level mutations apply; no per-route tier resolution attempted (or resolution returns 3-nil and applies no per-tier ops).
- `TestDecodeHeaders_PerRoute_RouteOnly` — Route tier set; Route ops apply after listener.
- `TestDecodeHeaders_MultiTier_FlagFalse` — all 3 tiers set, flag=false → least-specific (rc) wins per §11.5.
- `TestDecodeHeaders_MultiTier_FlagTrue` — all 3 tiers set, flag=true → most-specific (route) wins per §11.5.
- `TestDecodeHeaders_MultiTier_TwoOfThree_RouteAndVHost` — Route + VirtualHost set, RC nil → Route applied first/last per flag.
- `TestDecodeHeaders_MultiTier_TwoOfThree_RouteAndRC` — Route + RC set, VHost nil → Route applied first/last per flag.
- `TestDecodeHeaders_MultiTier_TwoOfThree_VHostAndRC` — VHost + RC set, Route nil → VHost applied first/last per flag.
- `TestEncodeHeaders_Symmetric` — encode-side mutations apply with same algorithm.
- `TestPerRouteProtectedHeader_BootError` — per §6.7 EAGER validation default: a per-route `HeaderMutationPerRoute` containing a protected-header mutation surfaces as a boot-time error (via the post-`BuildPerRouteConfig` validation hook). Asserts the verbatim error format mirrors the listener-level case.
- `TestPerRouteWholesaleNotApplicable` — header_mutation explicitly does NOT use wholesale-override (vs. fault per ADR-0073); tier values combine per the multi-tier algorithm.

`perroute_test.go` (existing file, extended) covers:

- `TestResolveAllTiers_AllThreeSet` — returns the 3-tuple in correct positions.
- `TestResolveAllTiers_RouteAndVHostOnly` — returns (route, vhost, nil).
- `TestResolveAllTiers_RouteAndRCOnly` — returns (route, nil, rc).
- `TestResolveAllTiers_VHostAndRCOnly` — returns (nil, vhost, rc).
- `TestResolveAllTiers_RouteOnly` — returns (route, nil, nil).
- `TestResolveAllTiers_VHostOnly` — returns (nil, vhost, nil).
- `TestResolveAllTiers_RCOnly` — returns (nil, nil, rc).
- `TestResolveAllTiers_NoneSet` — returns (nil, nil, nil).
- `TestResolveAllTiers_FilterNameNotPresent` — returns (nil, nil, nil) when filterName is not in any tier's map.
- `TestResolveAllTiers_RouteIdxOutOfRange` — returns (nil, nil, nil) for invalid routeIdx; rc-tier still consulted.
- `TestResolveAllTiers_DoesNotPolluteResolveCache` — call ResolveAllTiers, then Resolve, assert both return correct values independently.
- `TestResolveAllTiers_NilReceiver` — returns (nil, nil, nil) when *PerRouteConfig is nil (defensive).

### 14.2 Race detector + lint

- `go test -race ./internal/filter/http/header_mutation/...` — clean. No timer goroutines, no shared atomic state, no concurrency primitives; the race detector validates the per-instance read-only sharing of `*runtimeConfig` by construction.
- `go vet ./...` — clean.
- `golangci-lint run ./...` — clean. `gofmt`/`goimports` discipline per the 09 close.

### 14.3 Fuzzers

- `FuzzHeaderMutationConfigParse` (~50 LoC) — fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either (factory, nil) OR (nil, error); never panics; never returns (nil, nil). Per ADR-0018's "every parser/codec/filter ships a fuzzer" — header_mutation's New is the parser. 30s budget for CI short-mode.

### 14.4 Existing fuzzers re-run

The 12 existing fuzzers (10 from 08.1 + `FuzzDrainTransitions` from 08.2 + `FuzzFaultConfigParse` from 09) re-run clean at the 30s budget. Phase 10 touches none of their target packages; the re-run is mechanical.

### 14.5 h2spec re-run

Phase 10 touches no codec / framer / hpack / connection-management code. The h2spec gate at 53/53 PASS (per CONFORMANCE_PINS at the ADR-0051 pin) is invariant under filter additions. Re-run mechanical.

### 14.6 Differential 0000–0011 + 0012

All pre-existing fixtures (0000-tcp-echo through 0011-http-fault) re-run clean. NEW fixture 0012-http-header-mutation green under the 4 scenarios per §7.1.

### 14.7 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

The six gates (a)–(f) per §3 are all green at phase-done commit time. Verification commands quoted into PROGRESS.md per the executing-plans discipline.

---

## 15. Acceptance checklist (for the reviewer of this phase's final state)

- [ ] `internal/filter/http/header_mutation/{doc.go,header_mutation.go,header_mutation_test.go,fuzz_test.go}` exist with the contract per §4.1 + §6 + §14.
- [ ] `cmd/envoy-go/main.go` registers `header_mutation.New` against `header_mutation.TypeURL` in the http registry, alphabetically between `fault.New` and `httpReg.Freeze()` per §4.2.
- [ ] `internal/filter/http/perroute.go` has the new `ResolveAllTiers` method per §4.2 + §6.7.
- [ ] `internal/filter/http/perroute_test.go` carries the ResolveAllTiers tests per §4.2 + §14.1.
- [ ] `internal/filter/http/callbacks.go` has the new `RequestRouteConfigsAllTiers` (and symmetric `ResponseRouteConfigsAllTiers` per §12 deferred decision 1) callback method per §4.2.
- [ ] `internal/filter/http/chain.go` (or equivalent) wires the new callback into the per-stream state machine per §4.2 (the route-idx resolution is identical to the existing `RequestRouteConfig` path).
- [ ] `test/differential/0012-http-header-mutation/` exists with `README.md`, `expectations.yaml`, `envoy.yaml`, `envoy-go.yaml`, `driver/driver.go`, `backends/backend.go` per §4.3 + §7.
- [ ] `test/differential/runner.go` registers `0012-http-header-mutation` with `RequiresReference: true`.
- [ ] `docs/envoy-go/BEHAVIOR_CONTRACT.md` carries the §13.1 + §13.4 + §13.5 patches at phase-done commit. The `## Stat-name mapping ### 22-name table` is UNCHANGED. The `## Timing tolerances` section is UNCHANGED.
- [ ] `docs/envoy-go/DECISIONS.md` carries ADR-0108 through ADR-0113 at phase-done commit (per the planner's consolidation choices per §8.1).
- [ ] `docs/envoy-go/ROADMAP.md` row 10 status flips `in-progress → done` at the phase-done commit.
- [ ] `docs/envoy-go/STATE.md` advanced through lifecycle-states 3 (PLAN drafting), 4 (PLAN execution), 5 (verification), 6 (phase-done) per the SKILL_ROUTING state-machine discipline.
- [ ] All six phase-done gates (a)–(f) per §3 are green.
- [ ] Phase-done commit message subject: `phase 10: http-filter-header-mutation [ADR-0108, ADR-0109, ADR-0110, ADR-0111, ADR-0112, ADR-0113]` (or fewer per §8.1 consolidation).
- [ ] Phase-done commit message body explicitly states: (1) ROADMAP row 10 flips in-progress → done; (2) the §9 family heading at ROADMAP line 56 stays unchanged; (3) phase 10 is the THIRD §9 family-row to land (after cors @ 07.1 and fault @ 09); (4) anticipated 6 ADRs (or planner-consolidated count) listed in the subject.
- [ ] No 09 contract claim is invalidated by phase 10's changes (specifically: fault filter unaffected; cors filter unaffected; ADR-0073 amended-not-superseded; ADR-0072 + ADR-0074 boot-registration discipline unchanged modulo the one-line addition of header_mutation.New).
- [ ] No pre-existing fixture (0000–0011) regressed at phase-done time.

---

## 16. References

- **BRAINSTORM:** `docs/envoy-go/phases/10-http-filter-header-mutation/BRAINSTORM.md` (this SPEC's input).
- **BOOTSTRAP_PROMPT.md** §§4–9 (lifecycle, gating, family expansion).
- **MISSION.md** §2 (project non-purposes; informs §2 above).
- **ENVOY_TARGET.md** (reference image pin: `envoyproxy/envoy:v1.37.2 @ sha256:c5e8...`).
- **BEHAVIOR_CONTRACT.md** `## HTTP filter chain` (host of the new `### envoy.filters.http.header_mutation` subsection per §13.1; located after the phase-09 fault subsection); `## Stat-name mapping ### 22-name table` (UNCHANGED in phase 10); `## Timing tolerances` (UNCHANGED in phase 10); `## Equivalence Matrix` (new row per §13.4).
- **ROADMAP.md** row 10 (status `planned → in-progress` AT this SPEC commit; `in-progress → done` at phase-done).
- **DECISIONS.md** ADR-0107 (last landed before phase 10); ADR-0108..ADR-0113 land in phase 10.
- **Fault filter precedent:** `internal/filter/http/fault/fault.go` (per ADR-0100 + ADR-0101 — the package-shape + runtimeConfig parser pattern header_mutation mirrors).
- **Cors filter precedent:** `internal/filter/http/cors/cors.go` (per ADR-0074 — the encode-side header injection pattern; header_mutation extends from fixed-3-header set to programmable-mutation pipeline).
- **Framework surface:** `internal/filter/http/types.go` (HTTPFilter + FilterHeadersStatus + StreamDecoderFilter + StreamEncoderFilter + HTTPFilterFactory + FilterInstanceFactory + FactoryCtx); `internal/filter/http/callbacks.go` (DecoderFilterCallbacks + EncoderFilterCallbacks + RequestRouteConfig; phase 10 ADDS `RequestRouteConfigsAllTiers` and symmetric `ResponseRouteConfigsAllTiers`); `internal/filter/http/registry.go` (HTTPRegistry); `internal/filter/http/perroute.go` (3-tier merge per ADR-0073; phase 10 ADDS `ResolveAllTiers` per ADR-0110).
- **Phase 09 SPEC** `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` (structural precedent for this SPEC; mirrored §-for-§).
- **Envoy v1.37.2 header_mutation filter docs** (https://www.envoyproxy.io/docs/envoy/v1.37.2/configuration/http/http_filters/header_mutation_filter; https://www.envoyproxy.io/docs/envoy/v1.37.2/api-v3/extensions/filters/http/header_mutation/v3/header_mutation.proto.html) — the canonical proto reference.
- **Envoy v1.37.2 mutation_rules proto** (https://www.envoyproxy.io/docs/envoy/v1.37.2/api-v3/config/common/mutation_rules/v3/mutation_rules.proto.html) — the canonical `HeaderMutation` (Remove | Append) primitive.

---

## End of phase 10 SPEC.
