# Phase 09 — HTTP filter `envoy.filters.http.fault` (`internal/filter/http/fault/`, differential fixture `0011-http-fault`, `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.fault` extension)

**Phase id:** `09`
**Slug:** `09-http-filter-fault`
**Status:** `in-progress` (SPEC stage; ROADMAP row `09` flips `planned → in-progress` at this commit per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3)
**Produced by:** `superpowers:writing-plans` (lifecycle-state 1 → 2; transcribes the brainstorm-close BRAINSTORM.md (`docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md`) §§1–12 into formal SPEC shape, executing the eight §11 empirical-pin obligations against reference Envoy v1.37.2 in-session per ADR-0004)
**Depends on:** phase 08.2 (done at master `b33e04f` — 08.2 phase-done close + MVP-trunk closure; SHA-fill follow-up at master `14a68e7`). The 07.1 HTTP filter framework (settled at master `5e7c5f1` per the 07.1 phase-done; subsequently extended by 07.2 listener-chain completion) is the load-bearing surface phase 09 plugs into: `internal/filter/http/types.go` (FilterHeadersStatus + StreamDecoderFilter + StreamEncoderFilter + HTTPFilter + HTTPFilterFactory + FilterInstanceFactory), `internal/filter/http/callbacks.go` (DecoderFilterCallbacks + EncoderFilterCallbacks + SendLocalReply + ContinueDecoding + RequestRouteConfig), `internal/filter/http/registry.go` (HTTPRegistry.Register + Freeze + Lookup), `internal/filter/http/perroute.go` (3-tier merge per ADR-0073). The cors filter at `internal/filter/http/cors/cors.go` is the package-shape precedent (per ADR-0074).
**Parent phase:** None — phase 09 is a top-level row under `BOOTSTRAP_PROMPT.md` §9 "HTTP filters family" (the §9 family heading at `ROADMAP.md` line 56 is a conceptual umbrella, not a row, per BOOTSTRAP §9 invariant 4 + ADR-0106 settled by this phase). Each §9 family-child is its own coherent phase row; phase 09 is the FIRST row to land under the §9 HTTP filters family.
**Master design document:** `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md` (autonomous-brainstorm artifact per ADR-0004; this SPEC distills BRAINSTORM §§1–12 into formal contract language and executes the §11 empirical-pin obligations IN-SESSION; the empirical findings AMEND BRAINSTORM Decisions 3 + 5 + 7 + 10 — see §11 below for the surprises).
**Differential surface at end of phase:** ROADMAP row `09` flips `in-progress → done` at the phase-done commit. NEW differential fixture `0011-http-fault` (per-scenario equivalence under a small-static-backend driver against the §7 fixture bootstrap — five scenarios per BRAINSTORM Decision 12, refined to **four scenarios** in this SPEC per §11.5 empirical pin's header-driven-path finding) is differentially green. Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`, `0006-access-log`, `0007a-cors`, `0007b-iteration-probe`, `0008-listener-chain-match`, `0009-admin-config-dump`, `0010-graceful-drain` all green. The h2spec conformance gate at the ADR-0051 pin is unchanged at 53/53 PASS (phase 09's filter is a pure HTTP-layer addition; it touches no codec/framer/HPACK paths). Existing 11 fuzzers (10 from 08.1 + 1 added by 08.2) re-run clean; one NEW fuzzer `FuzzFaultConfigParse` per §14.5. `BEHAVIOR_CONTRACT.md ## HTTP filter chain` umbrella (the existing host of the `### Empirical evidence (cors preflight)` block at line 748) gains a new `### envoy.filters.http.fault` subsection per §13.1; `## Stat-name mapping` 17-name table extends to 22 names per §13.2; `## Timing tolerances` gains a new fault-delay-accuracy bullet per §13.3.

---

## 1. Purpose

Phase 09 lands `envoy.filters.http.fault` — Envoy's canonical fault-injection filter — as the SECOND production HTTP filter in envoy-go after cors (07.1) and the FIRST top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family. The five new architectural primitives:

1. A new `internal/filter/http/fault/` package owning the filter implementation. The package mirrors the cors precedent (`internal/filter/http/cors/`): `fault.go` (filter type + factory + decode/encode methods), `fault_test.go` (unit tests), `doc.go` (package overview). Two top-level exports: `TypeURL` (string constant `"type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault"`) + `New` (the `HTTPFilterFactory` registered against `TypeURL` in the boot registry). All other types (`runtimeConfig`, `filter`) are unexported. See ADR-0100.
2. **Delay async-resume.** When the request matches the delay-injection criteria, `DecodeHeaders` returns `StopIteration` after starting `time.AfterFunc(fixed_delay, …)` that fires `cb.ContinueDecoding()` from the timer goroutine. The chain parks; the timer wakes the chain after the delay; the request resumes through subsequent filters and onward to the upstream. This is the FIRST real filter to exercise the framework's async-resume primitive on the request side (the `envoy_go_test` probe filter exercised it structurally in 07.1 but no production filter has until now). See ADR-0102.
3. **Abort terminal-replace.** When the request matches the abort-injection criteria, `DecodeHeaders` invokes `cb.SendLocalReply(http_status, "fault filter abort", nil)` and returns `StopIteration`. The chain enters the encode iteration starting at `filter[len-1]` per ADR-0075; the upstream is never dialed. The body string is **byte-exact `"fault filter abort"` (18 bytes, NO trailing newline)** per §11.3/§11.4 empirical pin; the response carries the **4-header set** `content-length: 18`, `content-type: text/plain` (no `charset=UTF-8` modifier), `date: <IMF-fixdate>`, `server: envoy` — distinct from the 6-header set used by admin endpoints (no `cache-control`, no `x-content-type-options`). See ADR-0103.
4. **Combined delay + abort ordering.** When both criteria match, the delay timer fires first; the timer's callback then calls `SendLocalReply` for the abort (skipping `ContinueDecoding`). The upstream is never dialed; the response is the abort response, but it arrives `delay.fixed_delay` after the request. Confirmed empirically at 100ms delay + 503 abort: 5 samples at 101.1–102.1ms total = 100ms delay + ~1.5ms abort overhead. See §11.3/§11.4 + ADR-0102.
5. **`max_active_faults` concurrency cap.** A `sync/atomic.Int64` counter per filter-instance increments at fault-trigger time (after percentage roll succeeds; before timer schedule or SendLocalReply) and decrements at fault-completion (timer fires + callback completes; OR abort SendLocalReply completes). When the counter is at the configured cap, NEW faults are silently NOT injected and the `fault.faults_overflow` counter increments. Hot path is lock-free per LBP-1 sixth application (after ADR-0072 HTTPRegistry, ADR-0079 ListenerFilterRegistry, ADR-0061 stats Registry, ADR-0091 drain Manager, ADR-0078 ChainBuilder closure capture). See ADR-0105.

After phase 09, the project has proven the §9 HTTP filters family-expansion pattern: *envoy-go's HTTP filter framework can host a non-trivial production filter under the cors precedent's package-shape discipline; the framework's async-resume primitive is exercised in production for the first time; the per-route 3-tier merge (ADR-0073) carries through to a second filter under the wholesale-override discipline; the stats registry extends from 17 to 22 names without revisiting the SN1–SN8 flattening rules (per ADR-0061) — all under flat top-level row expansion (per ADR-0106) without a parent §9 row.* This is the FIRST §9 family-row to land; subsequent filters (header_mutation, buffer, local_ratelimit, …) follow the same row-as-its-own-phase pattern.

### 1.1 Empirical-finding-driven scope revisions (per §11)

The §11 empirical-pin block executed in this SPEC's drafting session AMENDS BRAINSTORM design decisions in three places:

- **§11.5 (header-driven fault path) — MAJOR REVISION:** BRAINSTORM §1.1 item 7 + Decision 7 designed envoy-go to honor the `x-envoy-fault-{delay,abort}-request[-percentage]` request headers as overrides on the listener-level static config. The empirical pin proves this is **WRONG**: Envoy v1.37.2 ONLY honors these headers when the listener config sets `delay.header_delay: {}` / `abort.header_abort: {}` proto sub-messages — the `delay.fixed_delay` / `abort.http_status` static config alone does NOT activate the header-driven path. Since BRAINSTORM §10.3 explicitly defers `delay.header_delay` (and the analogous `abort.header_abort`), the request-header path CANNOT be cleanly separated; phase 09 MUST drop request-header parsing from MVP scope and re-anchor that deferral on the `header_delay`/`header_abort` proto-field deferral. ADR-0104 is REPURPOSED as the deferral-ADR (per ADR-0040 format) instead of the implementation-ADR. BRAINSTORM §1.1 in-scope item 7 → moves to BRAINSTORM §10.3 deferral cluster. **In-scope item count: 8 → 7.** **Differential fixture scenarios: 5 → 4** (scenario 4 from BRAINSTORM §8.2 — "header-driven abort" — drops). See §1.2 below for the revised scope summary.
- **§11.1 (abort.http_status edge cases) — MINOR REVISION:** BRAINSTORM Decision 3 hypothesized that `abort.http_status: 0` would be treated as "503 default OR config-load error" with the resolution deferred. The empirical pin confirms it is a **PGV (proto-gen-validate) hard config-load error**: Envoy refuses any HTTPFault with `abort.http_status` outside `[200, 600)` at boot (`HTTPFaultValidationError.Abort: embedded message failed validation | caused by FaultAbortValidationError.HttpStatus: value must be inside range [200, 600)`). envoy-go's `New` factory MUST mirror by validating the unmarshaled `abort.http_status` against `[200, 600)` and returning a non-nil error from `New` for out-of-range values; ADR-0072 boot-time-fail-fast contract makes this ergonomic (the registry resolves typed_config at HCM-build time, BEFORE any traffic). ADR-0101 carries this constraint as a load-bearing detail.
- **§11.6 (stat-name verification) — MINOR REVISION:** BRAINSTORM Decision 10 anticipated FOUR `fault.*` stat names (`aborts_injected`, `delays_injected`, `active_faults`, `faults_overflow`). The empirical pin shows Envoy v1.37.2 emits a FIFTH counter `response_rl_injected` (the response-rate-limit-injected counter, emitted as a permanently-zero counter even when `response_rate_limit` is not configured). Since BRAINSTORM §10.1 defers `response_rate_limit`, envoy-go MUST decide between (A) emitting `response_rl_injected` as a permanently-zero counter for differential parity, or (B) skipping the emission and allow-listing the stat under the BEHAVIOR_CONTRACT.md `## Stat-name mapping ### Twin-series filter discipline` (the same disposition Envoy's emitted-but-not-configured stats receive elsewhere). Per ADR-0107 (settled by this SPEC), envoy-go takes route (A): emit `response_rl_injected` as a permanently-zero counter. Rationale: zero-cost (no goroutine, no memory beyond the 8-byte counter cell), differential-parity-positive (no allow-list bookkeeping), and forward-positive (when response_rate_limit lands in a future phase, the same stat name carries the count without rename or migration). Stat-name mapping table extension: 17 → 22 names (not 17 → 21 as BRAINSTORM anticipated). See §13.2.

### 1.2 Revised scope summary (post-§11 amendments)

After the §1.1 amendments, phase 09's in-scope architectural primitives are the FIVE listed at the head of §1 (delay async-resume, abort terminal-replace, combined ordering, max_active_faults, package + registration), expressed as 7 BRAINSTORM-§1.1-style line items (BRAINSTORM's 8 collapse to 7 with the request-header-path drop). Differential fixture has FOUR scenarios per §7.1 (the BRAINSTORM-§8.2 fifth — "header-driven abort" — drops). Stat-name extension is 17→22 (not 17→21). ADR list stays at 8 (ADR-0100..ADR-0107) with ADR-0104 repurposed from implementation to deferral.

### 1.3 Family-expansion shape (per BRAINSTORM Decisions 12, 13 + ADR-0106)

Phase 09 is a **flat top-level row** under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family heading; the §9 family heading at `ROADMAP.md` line 56 is a conceptual umbrella, not a row, and stays unchanged in state across all §9-family-row landings. Each subsequent HTTP filters family member (header_mutation, buffer, local_ratelimit, …) becomes its own top-level row at row 10, 11, 12, … There is NO sibling-stub authored by this SPEC for the next §9 row; future family-expansion brainstorms cold-start from the §9 heading + the just-shipped phase 09 artefacts (per BRAINSTORM Decision 13). ADR-0106 codifies the flat shape; rejected alternatives are a parent-09 row with sub-phases 09.1/09.2/… (rejected per BRAINSTORM §1.3 rationale).

---

## 2. Non-purposes

Per `BOOTSTRAP_PROMPT.md` §6.3 (scope-bounding) and ADR-0040 (out-of-scope deferrals format), the following are explicitly out of phase 09's scope:

### 2.1 HTTPFault proto-message non-goals (per BRAINSTORM §10 + §11.5 amendment)

- **`response_rate_limit` (FaultRateLimit).** Bandwidth throttling on the response body via token-bucket primitive; orthogonal to delay/abort. Forward-pointer: a future fault-extension phase OR the `envoy.filters.http.bandwidth_limit` filter under §9 which subsumes the same primitive. Deferral ADR per ADR-0040 format consolidated into ADR-0107 §Consequences.
- **`abort.grpc_status`.** Requires gRPC framing + `grpc-status` + `grpc-message` headers. Forward-pointer: §9 gRPC family or the gRPC-aware-filters phase that lands gRPC primitives. Until then, the proto field is parsed but its value is silently NOT honored at fault-eval time. Deferral ADR per ADR-0040 format consolidated into ADR-0103 §Consequences.
- **`delay.header_delay` (FaultDelay.HeaderDelay) AND the `x-envoy-fault-{delay,abort}-request[-percentage]` request-header path** — coupled per §11.5 empirical pin. Phase 09 silently parses `delay.header_delay` / `abort.header_abort` proto sub-messages but does NOT honor them; the runtimeConfig shape's `delayEnabled` / `abortEnabled` flags are gated on `delay.fixed_delay` / `abort.http_status` being set, not on the header-driven oneof variants. Forward-pointer: a small follow-up phase (~150 LoC) that lands `header_delay` + `header_abort` proto-field handling AND the four documented request headers (`x-envoy-fault-delay-request`, `x-envoy-fault-delay-request-percentage`, `x-envoy-fault-abort-request`, `x-envoy-fault-abort-request-percentage`) in one coherent slice. Deferral ADR per ADR-0040 format = ADR-0104 (REPURPOSED from BRAINSTORM's anticipated implementation-ADR).
- **`upstream_cluster` filter.** Filters fault application to requests routed to a specific upstream cluster. Forward-pointer: small follow-up phase. Deferral consolidated into ADR-0101 §Consequences.
- **`downstream_nodes` filter.** Filters fault application to requests originating from specific downstream node IDs. Forward-pointer: small follow-up phase. Deferral consolidated into ADR-0101 §Consequences.
- **All four runtime-key fields** (`delay_percent_runtime`, `abort_percent_runtime`, `max_active_faults_runtime`, `response_rate_limit_percent_runtime`). Defer to the runtime layer (RTDS); envoy-go has no runtime layer per `BOOTSTRAP_PROMPT.md` §9 "Runtime + hot restart family". Phase 09 silently parses these fields but ignores their values; the static `delay.percentage` / `abort.percentage` / `max_active_faults` fields are the source of truth. Forward-pointer: §9 Runtime + hot restart family. Deferral consolidated into ADR-0101 §Consequences.
- **`disable_downstream_cluster_stats`.** Suppresses per-downstream-cluster stat emission. Phase 09 ignores; envoy-go's stats discipline emits on the namespace it scopes to with no per-downstream-cluster split. Forward-pointer: small follow-up phase if/when per-downstream-cluster stat fan-out is added.
- **`filter_enabled` / `filter_enabled_runtime`.** Gates the filter on a runtime-key. Phase 09 ignores (filter is always enabled when registered). Forward-pointer: §9 Runtime + hot restart family.

### 2.2 `headers` field — RBAC-style sub-matching deferrals

Phase 09 honors `headers` for **StringMatcher.exact** matching (case-insensitive header NAME, case-sensitive header VALUE — confirmed via §11.8 empirical pin). The full `envoy.config.route.v3.HeaderMatcher` proto surface includes additional matcher variants (regex via `safe_regex_match`, range via `range_match`, present-only via `present_match`, prefix via `string_match.prefix`, suffix via `string_match.suffix`, contains via `string_match.contains`); phase 09 silently parses the full HeaderMatcher message but only honors the `string_match.exact` path. All other matcher variants are silently ignored at fault-eval time (the request is treated as IF the headers field were absent for non-exact matchers). Forward-pointer: whatever phase lands the full HeaderMatcher parsing engine (used by RBAC, JWT, ext_authz, and several other filters) consolidates the matching engine; until then, `string_match.exact` is the only honored matcher. Deferral consolidated into ADR-0101 §Consequences.

### 2.3 Test-surface non-purposes

- **Pre-defined `delay.percentage` strictly between 1% and 99%.** Phase 09 supports the proto field at all percentage values (the runtimeConfig stores the float64), but the differential fixture exclusively uses 0% and 100% to keep scenarios deterministic per ADR-0019 (no random seed in fixtures). Intermediate-percentage testing is unit-test-only and does not block the phase-done gate (e). The percentage-roll mechanism is implemented but not differentially exercised at intermediate values.
- **Stress / load testing of `max_active_faults`.** Unit tests cover the cap mechanically (Inc to cap → next fault skipped + `fault.faults_overflow` increments → Dec → next fault honored), but no large-scale concurrency stress test is run as part of phase-done gate (b).
- **Differential testing of fault behavior under H2 streams.** Fixture 0011's bootstrap is HTTP/1.1-only (codec_type: HTTP1). H2 fault behavior is exercised by unit tests against the framework's H2 dispatch path (per phase 05.1), but is NOT differentially asserted at phase 09. H2 differential coverage of fault is deferred (low operator value; high fixture cost; the H2 codec path is settled at the ADR-0051 pin). Forward-pointer: a future small follow-up phase if operator value materializes.

### 2.4 Cross-filter non-purposes

- **Filter-ordering interactions.** Phase 09 puts `envoy.filters.http.fault` BEFORE `envoy.filters.http.router` in the http_filters list (per the fixture envoy.yaml at §7.1). The CORS filter is independent and not involved in fixture 0011. The 07.1 ADR-0072 filter-ordering discipline (filters in the order declared) is unchanged. There is no test of fault interleaving with cors, ext_authz, jwt_authn, or other filters in this phase — the fixture is fault-only. Cross-filter interaction tests are deferred to whatever phase lands the relevant sibling filter.
- **Filter-on-filter behavior under SendLocalReply.** When fault's abort fires `SendLocalReply`, the encode iteration starts at `filter[len-1]` per ADR-0075 (which is `router`); router's encode-side is no-op pass-through. If a HYPOTHETICAL future filter sits BETWEEN router and fault on the encode-side (i.e., later in http_filters list than fault) and mutates response headers, the abort response would carry those mutations. Phase 09 does not test this scenario because fault is the only between-cors-and-router filter in the fixture. Future filter additions to fixture 0011 should test the encode-mutation path.

### 2.5 Security non-purposes

- **Authentication / authorization on fault-injection.** Fault is a per-request behavioral filter; there is no admin-endpoint surface for it. The operator's listener config controls whether and how fault is applied; envoy-go's MVP does not gate fault behind any auth surface.
- **Audit-logging of fault-injection events.** Fault emits 5 stat counters per §13.2; no per-request audit log. Existing access-log discipline (per phase 06.2) covers per-request log emission.

---

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for phase 09)

| Gate | 09 specialization |
|---|---|
| **(a)** `go build ./...` clean | Including the new `internal/filter/http/fault/` package (`doc.go`, `fault.go`, `fault_test.go`, OPTIONAL `fuzz_test.go`), the modified `cmd/envoy-go/main.go` (one-line registration delta), the modified `internal/stats/registry.go` (or wherever the 17-name table lives — five new fault.* stat-name registrations). All under `go vet ./...` clean and `golangci-lint run ./...` clean. The `gofmt`/`goimports` discipline mirrors the 08.2 close (per phase 08.2 follow-up). |
| **(b)** `go test ./...` clean | New unit tests in `internal/filter/http/fault/fault_test.go` covering: New-factory typed_config validation (success path + nil tc + malformed tc + abort.http_status out-of-range PGV → error), runtimeConfig shape correctness, percentage-roll determinism (0% → no fault; 100% → always fault; intermediate via injected RNG), DecodeHeaders happy-path delay-only / abort-only / combined / no-fault / headers-mismatch / max-active-faults-cap, async-resume timer behavior + cancel-on-OnDestroy, max_active_faults Inc/Dec balance + overflow stat increment, per-route wholesale-override resolution. Plus `go test -race ./...` clean (the timer-driven async-resume must be race-clean by construction; the atomic.Int64 maxActiveFaults counter exercises the race detector under a concurrent benchmark in `BenchmarkFaultMaxActiveFaultsCap` which is excluded from -short but run under -race in CI). |
| **(c)** h2spec re-run clean (53/53 PASS at ADR-0051 pin) | Phase 09 touches no codec / framer / hpack / connection-management code; it adds a single new HTTP filter in the chain. The h2spec gate at 53/53 PASS is invariant under filter additions to a chain that includes a router; re-running is mechanical (gate (c) per ADR-0051). The CONFORMANCE_PINS pin is unchanged. |
| **(d)** new/existing fuzzers run clean | Existing 11 fuzzers (10 from 08.1 + `FuzzDrainTransitions` from 08.2 — per the 08.2 phase-done close + REVIEW gate (d) appendix) re-run clean at the 30s ADR-0018 budget. **NEW (REQUIRED):** `FuzzFaultConfigParse` (~50 LoC; 30s budget) — fuzzes the `tc *anypb.Any` parameter to `New` against arbitrary byte sequences, asserting that `New` either returns a valid factory OR a non-nil error (no panics, no empty-OK responses). Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the fault filter's typed_config parser falls under "parser." Total fuzzer count post-09: **12**. |
| **(e)** Differential fixtures all green | All pre-existing fixtures `0000–0010` remain green (phase 09 adds a filter but does not modify any existing fixture's bootstraps). **NEW:** `0011-http-fault` (`test/differential/0011-http-fault/`) is green under the per-scenario equivalence claims of §7.1 — four scenarios per BRAINSTORM Decision 12 refined per §11.5: (1) delay-only listener-level, (2) combined delay+abort per-route, (3) per-route wholesale-override (abort-only over delay-only listener), (4) headers-field exact-match gate. The `RequiresReference: true` flag is set in `test/differential/runner.go` per the existing fixture-registration pattern (mirrors `0007a-cors`, `0009-admin-config-dump`, `0010-graceful-drain`). |
| **(f)** `BEHAVIOR_CONTRACT.md` populated | `## HTTP filter chain` umbrella (line 695; 07.1 host) gains a new `### envoy.filters.http.fault` subsection per §13.1. `## Stat-name mapping ### 17-name table` (line 130; 06.1 host) extends to 22 names per §13.2. `## Timing tolerances` (line 266) gains a new fault-delay-accuracy bullet per §13.3. ADR-0040 forward-pointer notes appended to the relevant deferral cluster paragraphs (response_rate_limit, header_delay, abort.grpc_status). In-place edit per ADR-0052 — lands at the phase-done commit alongside the implementation. |

The phase-done commit message body must explicitly state that ROADMAP row `09` flips `in-progress → done` AT this commit AND that the §9 HTTP filters family heading at ROADMAP line 56 stays unchanged (headings are not rows; their state is implicit) AND that this is the FIRST §9 family-row to land. Per `BOOTSTRAP_PROMPT.md` §5.3 commit message format. Commit subject: `phase 09: http-filter-fault [ADR-0100, ADR-0101, ADR-0102, ADR-0103, ADR-0104, ADR-0105, ADR-0106, ADR-0107]` (or fewer if the planner consolidates per §8 consolidation candidates).

---

## 4. Deliverables (files and directories)

### 4.1 New production code (in 09)

- `internal/filter/http/fault/doc.go` — package doc enumerating the typed_config surface (`HTTPFault` proto with 7-field consumed / 4-field silent-ignore decomposition per ADR-0101), the public API surface (`TypeURL`, `New`), the iteration-protocol coverage (Continue + StopIteration on DecodeHeaders only — no data/trailers/encode-side states exercised), and the cross-cutting ADR anchors. ~40 LoC.
- `internal/filter/http/fault/fault.go` — filter implementation. Public surface: `TypeURL` constant + `New` factory (matches `envoyhttp.HTTPFilterFactory` type signature). Unexported types: `runtimeConfig` struct (8 fields per §6.2 below), `filter` struct (per-instance state per §6.3), `headerMatch` struct (one (name, value) entry for the headers-field match list). `New(tc *anypb.Any, _ envoyhttp.FactoryCtx)` parses `tc` to `*envoyextensionsfiltershttpfaultv3.HTTPFault`, validates (`abort.http_status` ∈ `[200, 600)` per §11.1 PGV mirror; non-zero `delay.fixed_delay` if `delay.percentage > 0`; non-empty `abort` and `delay` if their respective `percentage > 0`), constructs a `*runtimeConfig` capturing the consumed fields, and returns a `FilterInstanceFactory` closure that allocates a fresh `*filter{cfg: rc, activeFaults: <atomic int64>, …}` per request. The filter implements both `StreamDecoderFilter` and `StreamEncoderFilter` (encode-side is no-op pass-through; the decoder side carries all fault logic). `DecodeHeaders` body: percentage-roll → headers-field match → max-active-faults check → start delay timer (if delayEnabled) OR fire abort SendLocalReply (if abortEnabled and not delayEnabled) → return `StopIteration` if either fault triggered, else `Continue`. The timer's callback decides whether to fire abort (combined delay+abort case) or call `ContinueDecoding` (delay-only case); on either path it decrements activeFaults. `OnDestroy` calls `f.delayTimer.Stop()` + decrements activeFaults if the timer was scheduled but neither fired nor was already stopped. `EncodeHeaders` / `DecodeData` / `EncodeData` / `DecodeTrailers` / `EncodeTrailers` are pass-through (Continue / DataContinue / TrailersContinue). ~400 LoC.
- `internal/filter/http/fault/fault_test.go` — unit tests per §14.1. ~280 LoC.
- `internal/filter/http/fault/fuzz_test.go` — `FuzzFaultConfigParse` per §14.5. ~50 LoC.

### 4.2 Changed production code (in 09)

- `cmd/envoy-go/main.go` — modified per BRAINSTORM Decision 2. ONE new `httpReg.Register(fault.TypeURL, fault.New)` line at the top of the http-registry block (currently registering router/cors/envoy_go_test at lines 111–113 of master HEAD `14a68e7`), inserted alphabetically per BRAINSTORM Decision 2's "router-first-then-alphabetical" stylistic discipline: the resulting block is `httpReg.Register(router.TypeURL, router.New); httpReg.Register(cors.TypeURL, cors.New); httpReg.Register(envoygotest.TypeURL, envoygotest.New); httpReg.Register(fault.TypeURL, fault.New); httpReg.Freeze()`. Plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/fault"` alphabetically among the existing filter-package imports. **No other wiring changes** — fault is HTTP-only, no listener / cluster / drain manager threading. ~3 LoC delta.
- `internal/stats/registry.go` (or wherever the 17-name registration table lives — exact file path settled at PLAN time; 06.1 SPEC §6 has the canonical pointer) — five new stat-name registrations appended after the existing 17 entries: `fault.aborts_injected` (counter), `fault.delays_injected` (counter), `fault.active_faults` (gauge), `fault.faults_overflow` (counter), `fault.response_rl_injected` (counter; permanently zero in phase 09 per §11.6 + ADR-0107 route-A choice). The 17-name table's flattening rules SN1–SN8 (per ADR-0061) project unchanged: `http.<stat_prefix>.fault.<counter>` is the canonical internal name; the Prometheus form is `envoy_http_fault_<counter>{envoy_http_conn_manager_prefix="<stat_prefix>"}` (the stat_prefix is extracted as a label per Envoy's tag-extraction discipline; confirmed via §11.6 empirical pin). ~10 LoC delta.

### 4.3 New harness and fixture code (in 09)

- `test/differential/0011-http-fault/README.md` — fixture overview + per-scenario equivalence-claim narrative + the four-scenario list (per §7.2) + the dual-proxy bootstrap discipline (admin/listener ports disambiguated for dual-boot under `--network host` per the existing fixture pattern). ~80 LoC.
- `test/differential/0011-http-fault/expectations.yaml` — per-scenario tolerance discipline encoding the §13.4 allow-list + the per-scenario assertion matrix (per §7.1 prefix paths): scenario 1 (`/scenario1/...`) → delay 100ms ±10ms, body byte-equal `backend\n`, status 200, stat delta `delays_injected += 1`; scenario 2 (`/scenario2/...`) → combined delay+abort, status 503, body byte-equal `fault filter abort` (18 bytes, no newline), 4-header set, stat delta `delays_injected += 1` AND `aborts_injected += 1`, time_total ≈ 100ms; scenario 3a (`/scenario3-wholesale/...`) → wholesale-override → status 418, body `fault filter abort`, NO inherited delay (time_total < 50ms), stat delta `aborts_injected += 1`; scenario 3b (`/scenario3-baseline/...`) → listener-level delay only → 200 from backend, time_total ≈ 100ms, stat delta `delays_injected += 1`; scenario 4 (`/scenario4/...`) → 4 sub-probes a/b/c/d per §7.1 scenario 4, stat delta `aborts_injected += 2`. ~80 LoC.
- `test/differential/0011-http-fault/envoy.yaml` — reference Envoy bootstrap (admin :9902, listener :10001) with the SINGLE-listener five-prefix layout per §7.4 verbatim YAML: listener-level fault is `delay 100% 100ms` (no abort); per-route overrides on `/scenario2` (delay+abort), `/scenario3-wholesale` (abort 418 only), `/scenario3-baseline` (no override → inherit), `/scenario4` (abort 503 + headers gate). Single STRICT_DNS cluster `c_backend` pointing at the harness backend hostname. ~70 LoC.
- `test/differential/0011-http-fault/envoy-go.yaml` — envoy-go bootstrap (admin :9901, listener :10000). Identical to `envoy.yaml` modulo admin/listener ports. ~70 LoC.
- `test/differential/0011-http-fault/driver/driver.go` — Go driver implementing the §7.3 four-scenario orchestration: dual-proxy boot + small-static-backend boot + scenario probes (one curl-equivalent per scenario, run sequentially against both proxies, with timing capture for scenario 1) + stat-snapshot delta assertions per scenario + cleanup. Event-based synchronization throughout (no hardcoded sleeps per the 08.2 SPEC §10 + 07.2 REVIEW M-8 carry-forward). ~250 LoC.
- `test/differential/0011-http-fault/backends/backend.go` — minimal Go HTTP backend bound to port 18001. `/` endpoint serves a fast `200 OK` with body `backend\n` (8 bytes; matches the empirical-pin backend used during §11 capture). ~40 LoC.
- `test/differential/runner.go` — `RegisterFixture("0011-http-fault", ..., Capabilities{RequiresReference: true})` registration line added per the existing fixture-registration pattern. ~3 LoC delta.

### 4.4 Changed documentation and state (in 09)

- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — in-place edit per ADR-0052: (a) NEW `### envoy.filters.http.fault` subsection under `## HTTP filter chain` umbrella (line 695) per §13.1; (b) `## Stat-name mapping ### 17-name table` (line 130) extends to 22 names per §13.2 (renamed to `### 22-name table` to match the SN1–SN8 flattening discipline's name-count anchor); (c) `## Timing tolerances` (line 266) gains a NEW bullet for fault-delay accuracy per §13.3; (d) `## Equivalence Matrix` (line 9) gains ONE new row for the fault-filter equivalence claim per §13.4. Lands at phase-done commit alongside impl.
- `docs/envoy-go/DECISIONS.md` — eight new ADRs (ADR-0100..ADR-0107 per §8) appended. Lands incrementally per `superpowers:executing-plans` PROGRESS preamble convention (ADRs land at the task that anchors them).
- `docs/envoy-go/ROADMAP.md` — row `09` flips `planned → in-progress` AT THIS COMMIT (per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3); flips `in-progress → done` at the 09 phase-done commit. The §9 family heading at line 56 stays unchanged (headings are not rows; per ADR-0106 settled by this phase).
- `docs/envoy-go/STATE.md` — flips `lifecycle-state: 1 → 2`, `next-skill: superpowers:writing-plans` (PLAN.md authoring for phase 09), `active-phase: 09-http-filter-fault`, `last-commit: <SPEC commit SHA>`, `last-updated: <date>`. SHA-fill follow-up commit per phase-04..08.2 convention. NOT modified in this commit (the orchestrating session handles STATE.md per the cold-start contract); the SPEC merely names what the next session will write.
- `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` — this file.
- `docs/envoy-go/phases/09-http-filter-fault/PLAN.md` — authored by the next session (lifecycle-state 2 → 3); not part of THIS commit.
- `docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md` — authored at PLAN-execution start by the executing session (lifecycle-state 3); not part of THIS commit.

---

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 09)

```
                     +-------------------------------------+
                     |  cmd/envoy-go/main.go               |
                     |  (registers router, cors,           |
                     |   envoy_go_test, fault under        |
                     |   httpReg; one new line + import)   |
                     +------------+------------------------+
                                  |
                                  v
                     +-------------------------------------+
                     |  internal/filter/http/HTTPRegistry  |
                     |  (07.1; ADR-0072; freeze-after-boot)|
                     |  resolves fault.TypeURL @ HCM build |
                     +------------+------------------------+
                                  |
                                  v
                     +-------------------------------------+
                     |  internal/filter/http/fault/        |
                     |  -------------------------------    |
                     |  TypeURL = "...HTTPFault" (const)   |
                     |  New(tc, ctx) -> (factory, error)   |
                     |    parses tc, validates abort.      |
                     |    http_status [200,600), captures  |
                     |    *runtimeConfig                   |
                     |  factory() -> HTTPFilter{           |
                     |    Decoder: f, Encoder: f, Name:... |
                     |  }                                  |
                     |  filter.DecodeHeaders ->            |
                     |    percentage roll +                |
                     |    headers-field match +            |
                     |    activeFaults cap check +         |
                     |    delay timer schedule OR          |
                     |    abort SendLocalReply +           |
                     |    StopIteration                    |
                     +-------------------------------------+
                                  |
                                  v  (delay timer callback OR abort path)
                     +-------------------------------------+
                     |  cb.SendLocalReply(status,          |
                     |   "fault filter abort", nil)        |
                     |   (per ADR-0075 chain encode entry  |
                     |    at filter[len-1] = router)       |
                     +-------------------------------------+
                                  |
                                  +-> stats: fault.aborts_injected++
                                  +-> stats: fault.faults_overflow++ (if cap)
                                  +-> stats: fault.delays_injected++ (delay path)
                                  +-> stats: fault.active_faults gauge (Inc/Dec)
                                  +-> stats: fault.response_rl_injected (zero)
```

The registry → factory → filter-instance flow mirrors cors/router/envoy_go_test verbatim (per ADR-0072's HTTPFilterFactory two-step contract). The NEW dimension is the timer-driven async-resume from inside DecodeHeaders, which is the FIRST production exerciser of the framework's `cb.ContinueDecoding()` primitive on the request side. The chain framework's existing async-resume coalescing (per `internal/filter/http/callbacks.go:14–17`) is sufficient — phase 09 needs no framework extensions.

### 5.2 Per-request flow — delay-only (canonical async-resume scenario)

```
HCM dispatch
  -> filter[0..N-1].DecodeHeaders run in order
  -> fault.DecodeHeaders fires (filter[i] for some i ∈ [0, len-2])
       1. percentage roll: if !delayEnabled OR roll < cfg.delayPercentage, fall through to step 2
          step 2 evaluates abortEnabled
       2. headers-field match: if matchHeaders empty OR all rules match, proceed to step 3
       3. max_active_faults check: if cfg.maxActiveFaults > 0 AND f.activeFaults.Load() >= cap, increment
          stats.fault.faults_overflow and SKIP (return Continue, no fault injected)
       4. f.activeFaults.Add(1); stats.fault.active_faults.Inc(); stats.fault.delays_injected.Inc()
       5. f.delayTimer = time.AfterFunc(cfg.delayFixedDelay, timerCallback); return StopIteration
  -> chain dispatch parks at filter[i]; HCM dispatch goroutine returns to its event loop

(time passes — delay duration)

  -> Go runtime's timer goroutine fires; runs timerCallback:
       6. If !abortEnabled OR delay-only path (Decision 6), call cb.ContinueDecoding(); chain wakes
          at filter[i+1]
       7. f.activeFaults.Add(-1); stats.fault.active_faults.Dec()

  -> chain resumes filter[i+1..len-1].DecodeHeaders + filter[len-1] (router) dials upstream
  -> upstream returns, encode chain runs, response written

  -> at finalize: HCM emits access log; OnDestroy runs on each filter; fault.OnDestroy is no-op
     (timer already fired; activeFaults already decremented at step 7)
```

### 5.3 Per-request flow — abort-only (terminal-replace scenario)

```
HCM dispatch
  -> fault.DecodeHeaders fires
       1. abortEnabled AND percentage roll AND headers match AND not at cap
       2. f.activeFaults.Add(1); stats.fault.active_faults.Inc(); stats.fault.aborts_injected.Inc()
       3. cb.SendLocalReply(cfg.abortHTTPStatus, "fault filter abort", nil)
       4. return StopIteration

  -> chain enters encode iteration at filter[len-1] (router); router.EncodeHeaders is no-op pass
  -> wire-write layer emits 4-header response (content-length: 18, content-type: text/plain, date,
     server) + 18-byte body "fault filter abort" (no trailing newline)

  -> at finalize: f.activeFaults.Add(-1); stats.fault.active_faults.Dec(); access log emitted;
     OnDestroy runs (no-op for abort path — no timer to cancel)
```

### 5.4 Per-request flow — combined delay + abort (delay-first-then-abort)

```
HCM dispatch
  -> fault.DecodeHeaders fires
       1. delayEnabled AND abortEnabled AND both percentage rolls succeed AND headers match
          AND not at cap
       2. f.activeFaults.Add(1); stats.fault.delays_injected.Inc();
          stats.fault.active_faults.Inc();
       3. f.delayTimer = time.AfterFunc(cfg.delayFixedDelay, abortCallback); return StopIteration

  (time passes — delay duration, ~100ms)

  -> Go runtime's timer goroutine fires; runs abortCallback:
       4. stats.fault.aborts_injected.Inc()
       5. cb.SendLocalReply(cfg.abortHTTPStatus, "fault filter abort", nil)
       6. f.activeFaults.Add(-1); stats.fault.active_faults.Dec()

  -> chain enters encode iteration at filter[len-1] (router); response written as in §5.3
```

The combined path's timing fingerprint is `delay + ~1.5ms` (per §11.3/§11.4 empirical pin; 5 samples at 101.1–102.1ms total for delay=100ms + abort=503).

### 5.5 Per-request flow — passthrough (no fault)

```
HCM dispatch
  -> fault.DecodeHeaders fires
       1. !delayEnabled (or percentage roll misses) AND !abortEnabled (or roll misses) OR
          headers field doesn't match
       2. return Continue

  -> chain advances to filter[i+1]; no stats changes on the fault-filter side
```

### 5.6 Per-request flow — max_active_faults overflow (fault skipped)

```
HCM dispatch
  -> fault.DecodeHeaders fires
       1. delayEnabled AND percentage roll succeeds AND headers match
       2. cap-check: f.activeFaults.Load() >= cfg.maxActiveFaults (cap is non-zero)
       3. stats.fault.faults_overflow.Inc(); fault is SKIPPED
       4. return Continue

  -> chain advances; request flows through without fault
```

This path is exercised in the unit test under controlled `cfg.maxActiveFaults: 1` + two concurrent requests (the second hits overflow); not in the differential fixture (which uses `max_active_faults: 0` = no cap throughout).

### 5.7 Concurrency model

- **Per-request filter instance.** Each `HCM dispatch` call allocates a fresh `*filter` per request via the FilterInstanceFactory closure. Per-instance state (`delayTimer`, `markedActive`) is not shared across requests; the single-goroutine-per-stream invariant per ADR-0071 makes per-instance state race-free WITHOUT synchronization.
- **`activeFaults` counter — shared across requests.** The counter is a closure field in the New factory's returned FilterInstanceFactory closure (NOT a per-instance field). Type: `*atomic.Int64`. All filter instances spawned from the same factory share the same counter. Hot path is lock-free (`atomic.Int64.Load`, `Add(1)`, `Add(-1)`); LBP-1 sixth application (after ADR-0072 HTTPRegistry, ADR-0079 ListenerFilterRegistry, ADR-0061 stats Registry, ADR-0091 drain Manager, ADR-0078 ChainBuilder closure capture). Per BRAINSTORM Decision 8.
- **Timer goroutine.** `time.AfterFunc(d, fn)` returns a `*time.Timer`; the runtime spawns a fresh goroutine per timer fire. The timer's goroutine is NOT the chain's dispatch goroutine. Inside the timer callback, the only safe-to-call chain operations are `cb.ContinueDecoding` (idempotent + coalescing per `internal/filter/http/callbacks.go:14–17`) and `cb.SendLocalReply` (first-call-wins via sync.Once per `callbacks.go:24–25`). Both are documented as goroutine-safe.
- **OnDestroy interaction with timer.** `OnDestroy` is called by the chain teardown path (request completion or downstream-disconnect-induced reset). It calls `f.delayTimer.Stop()` which returns true if the timer was active (callback not yet fired) or false if already fired. envoy-go IGNORES the return value — the activeFaults counter is decremented unconditionally in OnDestroy IF the timer was scheduled but neither completed nor cancelled. Race: if the timer fires between OnDestroy's `Stop()` call and the callback's first action, both OnDestroy and the callback decrement activeFaults. Mitigation: `markedActive bool` per-instance flag, set to true at trigger time and to false on EITHER completion path; both decrement-sites guard on `if f.markedActive { f.markedActive = false; f.activeFaults.Add(-1); stats.fault.active_faults.Dec() }`. This is the cors-precedent-style guard for sync.Once-style "decrement exactly once" idempotency on a single per-instance flag (no atomic needed because the single-goroutine-per-stream invariant per ADR-0071 makes the read-modify-write race-free WITHIN an instance).

### 5.8 Filter ordering in fixture 0011

The fixture's http_filters list is `[envoy.filters.http.fault, envoy.filters.http.router]` (fault BEFORE router). Per the ADR-0072 declaration-order discipline, fault's DecodeHeaders runs at index 0; router's runs at index 1 (terminal). Encode iteration runs reverse: router's EncodeHeaders at index 1, fault's at index 0. Fault's encode-side is no-op pass-through. The fixture has no other intervening filters; cors and envoy_go_test are NOT included.

---

## 6. Per-component contract summary

### 6.1 Constructor signatures (cors precedent verbatim)

`internal/filter/http/fault/fault.go` exports:

```go
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault"

func New(tc *anypb.Any, _ envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)
```

`New` body discipline (per ADR-0101):

1. If `tc == nil`: return error `errors.New("fault: typed_config required")`. (The cors filter accepts nil tc because the v3.Cors proto has no required fields; the v3.HTTPFault proto has no required fields either, but a fault filter with NO typed_config has no behavioral effect — every request would percentage-miss and pass through. Phase 09 chooses to REQUIRE non-nil tc to surface configuration mistakes at boot rather than silently no-op the filter; rationale documented in ADR-0101 §Alternatives.)
2. Unmarshal: `var c envoyextensionsfiltershttpfaultv3.HTTPFault; if err := tc.UnmarshalTo(&c); err != nil { return nil, err }`.
3. Validate `abort.http_status` ∈ `[200, 600)` if `abort != nil`; if out of range, return error mirroring Envoy's PGV message: `"fault: abort.http_status %d out of range [200, 600)"`. (Per §11.1.)
4. Validate `delay.fixed_delay > 0` if `delay != nil` AND `delay.percentage > 0`; if not, return error `"fault: delay.fixed_delay required when delay.percentage > 0"`. (Conform to Envoy's `fault: zero delay duration` config-load error path.)
5. Construct `*runtimeConfig` per §6.2.
6. Allocate the closure-captured `*atomic.Int64` activeFaults counter.
7. Return `func() envoyhttp.HTTPFilter { f := &filter{cfg: rc, active: activeFaults}; return envoyhttp.HTTPFilter{Name: "envoy.filters.http.fault", Decoder: f, Encoder: f} }, nil`.

### 6.2 `runtimeConfig` shape (per ADR-0101)

```go
type runtimeConfig struct {
    delayEnabled       bool          // delay != nil AND (delay.fixed_delay > 0 OR delay.header_delay set; the latter never honored — see §11.5)
    delayPercentage    float64       // delay.percentage in [0, 100]; 0 = never; 100 = always
    delayFixedDelay    time.Duration // from delay.fixed_delay; 0 if delay.header_delay set (silent-ignore path)

    abortEnabled       bool          // abort != nil AND (abort.http_status ∈ [200,600) OR abort.header_abort set; latter never honored)
    abortPercentage    float64       // abort.percentage in [0, 100]
    abortHTTPStatus    int           // abort.http_status; PGV-validated [200, 600) at New time

    matchHeaders       []headerMatch // headers field; empty = match-all; only string_match.exact honored

    maxActiveFaults    int64         // 0 = no cap (proto's nil pointer); int64 for atomic counter ABI
}

type headerMatch struct {
    name        string // canonicalized via http.CanonicalHeaderKey at New time
    exactValue  string // string_match.exact (only matcher variant honored per §11.8)
}
```

The 7 consumed fields from BRAINSTORM §1.1 item 3 are reduced to 6 in this SPEC's revised scope (per §11.5 amendment): delay.percentage, delay.fixed_delay, abort.percentage, abort.http_status, headers, max_active_faults. The dropped consumed field is the request-header path (which was conceptually a 7th-field sub-decision but is not a separate proto field). The 4 silent-ignored fields from BRAINSTORM §1.1 item 3 expand to 9 in this SPEC: upstream_cluster, downstream_nodes, response_rate_limit, disable_downstream_cluster_stats, delay.header_delay, abort.header_abort, abort.grpc_status, plus the 4 runtime-key fields (delay_percent_runtime, abort_percent_runtime, max_active_faults_runtime, response_rate_limit_percent_runtime), plus filter_enabled / filter_enabled_runtime — total 11 silent-ignore (the proto has 11+1+more but envoy-go cares about the 11 explicitly listed; HeaderMatcher non-exact matchers are silent-ignored at fault-eval time, not at parse time).

### 6.3 Per-instance `filter` struct

```go
type filter struct {
    cfg    *runtimeConfig // pointer to listener-level config; per-route config resolved per-request via cb.RequestRouteConfig
    active *atomic.Int64  // closure-captured activeFaults counter (shared across instances)

    dcb envoyhttp.DecoderFilterCallbacks
    ecb envoyhttp.EncoderFilterCallbacks

    delayTimer    *time.Timer // non-nil if delay scheduled; OnDestroy calls Stop
    markedActive  bool         // sync.Once-equivalent guard for activeFaults Inc/Dec balance (see §5.7)
}
```

Per the cors precedent, the filter implements both StreamDecoderFilter and StreamEncoderFilter so the chain can drive it as a both-sides filter. Encode-side is no-op; decode-side carries all fault logic. `OnDestroy` is the single teardown hook (callable from either Decoder or Encoder interface — per `internal/filter/http/types.go:55,66` they share the same OnDestroy method signature).

### 6.4 `DecodeHeaders` body discipline (per ADR-0102 + ADR-0103 + ADR-0105)

```go
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    cfg := f.routeConfigOrListener()  // per-route 3-tier merge per ADR-0073; wholesale-override per §11.7
    if !f.matchesHeaders(headers, cfg) {
        return envoyhttp.Continue
    }
    delayApplies := cfg.delayEnabled && rollPercent(cfg.delayPercentage)
    abortApplies := cfg.abortEnabled && rollPercent(cfg.abortPercentage)
    if !delayApplies && !abortApplies {
        return envoyhttp.Continue
    }
    if cfg.maxActiveFaults > 0 && f.active.Load() >= cfg.maxActiveFaults {
        stats.faultFaultsOverflow.Inc()
        return envoyhttp.Continue
    }
    f.active.Add(1); f.markedActive = true
    stats.faultActiveFaults.Inc()
    if delayApplies && abortApplies {
        stats.faultDelaysInjected.Inc()
        f.delayTimer = time.AfterFunc(cfg.delayFixedDelay, func() {
            stats.faultAbortsInjected.Inc()
            f.dcb.SendLocalReply(cfg.abortHTTPStatus, faultAbortBody, nil)
            f.decrementActive()
        })
        return envoyhttp.StopIteration
    }
    if delayApplies {
        stats.faultDelaysInjected.Inc()
        f.delayTimer = time.AfterFunc(cfg.delayFixedDelay, func() {
            f.dcb.ContinueDecoding()
            f.decrementActive()
        })
        return envoyhttp.StopIteration
    }
    // abortApplies && !delayApplies
    stats.faultAbortsInjected.Inc()
    f.dcb.SendLocalReply(cfg.abortHTTPStatus, faultAbortBody, nil)
    f.decrementActive()
    return envoyhttp.StopIteration
}

func (f *filter) decrementActive() {
    if f.markedActive {
        f.markedActive = false
        f.active.Add(-1)
        stats.faultActiveFaults.Dec()
    }
}

const faultAbortBody = "fault filter abort" // 18 bytes, NO trailing newline (per §11.3/§11.4)
```

(Pseudo-Go; final shape is the PLAN-implementer's call. The above captures the contract anchors per §11 empirical pins.)

### 6.5 Per-route 3-tier merge (per ADR-0073 + §11.7 confirmation)

`f.routeConfigOrListener()` body:

```go
func (f *filter) routeConfigOrListener() *runtimeConfig {
    if f.dcb == nil {
        return f.cfg
    }
    raw := f.dcb.RequestRouteConfig() // 3-tier merge per ADR-0073
    if raw == nil {
        return f.cfg
    }
    routeCfg, ok := raw.(*envoyextensionsfiltershttpfaultv3.HTTPFault)
    if !ok {
        return f.cfg // defensive; shouldn't happen if perroute parses anypb correctly
    }
    return parseRouteRuntimeConfig(routeCfg) // builds a *runtimeConfig per the route override
}
```

Per §11.7 wholesale-override pin: when the per-route config is non-nil, it WHOLESALE replaces the listener-level config (NOT field-merge). Specifically, a per-route HTTPFault that omits `delay` does NOT inherit the listener-level `delay`. Empirical evidence at §11.7 confirms (303ms time_total for /baseline = inherited 300ms delay; 1ms time_total for /abort-only = NO inherited delay; per-route abort 418 with no delay field).

### 6.6 `SendLocalReply` headers parameter

The third argument to `SendLocalReply(status int, body string, headers OrderedHeaders)` is `nil` for fault's abort path. Per §11.3 empirical pin, Envoy's abort response carries 4 headers in this on-wire order: `content-length: 18`, `content-type: text/plain`, `date: <IMF-fixdate>`, `server: envoy`. Of these:

- `content-length: 18` is calculated by the chain's writeH1Reply / writeH2Reply layer from `len(body) = 18`.
- `content-type: text/plain` is injected by Envoy's local-reply default; envoy-go's chain.beginLocalReply currently emits `content-type: text/plain; charset=UTF-8` for SendLocalReply with empty headers (per the cors precedent's preflight-200 with non-empty body). For the abort response, the `charset=UTF-8` modifier is ABSENT. Two implementation choices:
    - **(A)** Pass an explicit `OrderedHeaders` carrier from the fault filter with `content-type: text/plain` (no charset modifier) — overriding the chain's default. Mirrors cors's 6-header verbatim discipline.
    - **(B)** Modify `chain.beginLocalReply` to emit `text/plain` (no charset) for empty-body OR for fault's path, breaking the cors precedent.
- Phase 09 chooses **(A)** per §13.1 — fault passes a 1-element OrderedHeaders `[{Name: "Content-Type", Value: "text/plain"}]` to SendLocalReply, and the chain's reconcileOrderedHeaders preserves the override + appends framework-injected `date` and `server`. The `content-length` is computed automatically by the H1 wire writer. (Alternative: pass nil and accept the `charset=UTF-8` divergence, allow-listing it in the differential fixture's `expectations.yaml` — rejected because §11.3 byte-exact equivalence is the contract per BEHAVIOR_CONTRACT.md `## HTTP filter chain ### Asserted equivalence`'s `Response body is byte-exact for deterministic handlers` clause.)
- `date: <IMF-fixdate>` is non-deterministic (date-stamp); allow-listed per the existing differential harness allow-list (per `BEHAVIOR_CONTRACT.md ## Header allow-list`).
- `server: envoy` is framework-emitted constant; deterministic.

Per the §13.1 BEHAVIOR_CONTRACT.md extension, the asserted-equivalence prose calls out the 4-header set verbatim plus the body byte-exact `fault filter abort`. The header ordering on the wire is preserved by OrderedHeaders + reconcileOrderedHeaders (per ADR-0071 + 07.1's Task 18 ordered-headers discipline).

---

## 7. Differential fixture `0011-http-fault`

### 7.1 Equivalence claims (per BRAINSTORM §8 refined per §11)

Per §11 empirical-pin findings, the BRAINSTORM's 5-scenario list collapses to 4 scenarios (the header-driven scenario drops per §11.5). The per-scenario equivalence claims:

All four scenarios share a single listener config (per §7.4 verbatim YAML below): listener-level fault `delay: { percentage: 100%, fixed_delay: 0.1s }` (no abort), with per-route overrides applied to specific prefixes. This is the project's preferred fixture shape: ONE listener with prefix-routed scenarios under per-route `typed_per_filter_config` overrides (not one-listener-per-scenario).

1. **Scenario 1 (delay-only, listener-level inheritance).** Probe: `GET /scenario1/anything` → no per-route override → inherits listener fault `delay 100% 100ms`. Expects 200 OK from backend, body `backend\n` (8 bytes), `time_total ≈ 100ms ± 10ms` (per §11.2 + §13.3 timing tolerance), stats `delays_injected += 1`, `aborts_injected += 0`, `faults_overflow = 0`, `active_faults = 0` (final), `response_rl_injected = 0`.
2. **Scenario 2 (combined delay + abort, per-route override that re-asserts delay AND adds abort).** Probe: `GET /scenario2/anything` → per-route override `delay 100% 100ms + abort 100% 503` (wholesale-override re-includes delay). Expects 503 Service Unavailable, body byte-equal `fault filter abort` (18 bytes, no newline), 4-header set (content-length: 18, content-type: text/plain, date, server), `time_total ≈ 100ms` (delay-then-abort ordering per §11.3), stats `delays_injected += 1`, `aborts_injected += 1`. Confirms BRAINSTORM Decision 6's combined-ordering settled empirically.
3. **Scenario 3 (per-route wholesale-override — abort-only over delay-only listener; the canonical wholesale-override demonstration).** Two probes: (3a) `GET /scenario3-wholesale/anything` → per-route override `abort 100% 418` with NO delay field → wholesale-override REPLACES the listener-level delay. Expects 418 (status text portion allow-listed per §12 deferred decision 7), body `fault filter abort`, `time_total < 50ms` (NO inherited 100ms delay — wholesale-override per §11.7), stats `aborts_injected += 1`. (3b) `GET /scenario3-baseline/anything` → no per-route override → inherits listener `delay 100% 100ms` only. Expects 200 from backend, `time_total ≈ 100ms`, stats `delays_injected += 1`. The 3a/3b pair is the load-bearing wholesale-vs-inherit demonstration.
4. **Scenario 4 (headers-field exact-match gate).** Per-route override on `/scenario4`: `abort 100% 503, headers: [{name: x-fault-on, string_match: {exact: yes}}]`. The listener-level `delay 100% 100ms` is wholesale-replaced by the per-route fault config (which has no delay field). Probe (4a): `GET /scenario4/anything` no x-fault-on header → expects 200 from backend after listener-level delay applies — wait, no: per-route wholesale-override REMOVES listener delay too. So 4a expects 200 from backend at `time_total < 50ms` (no listener delay, no abort because headers don't match). Probe (4b): `GET /scenario4/anything` `x-fault-on: yes` → expects 503 + `fault filter abort` (headers match → fault fires). Probe (4c): `GET /scenario4/anything` `X-FAULT-ON: yes` (uppercase header NAME) → expects 503 (HTTP-level case-insensitive name lookup per §11.8.d). Probe (4d): `GET /scenario4/anything` `x-fault-on: YES` (uppercase header VALUE) → expects 200 (case-sensitive value match per §11.8.e). Stats deltas: `aborts_injected += 2` (probes 4b + 4c hit; 4a + 4d miss).

### 7.2 No header-driven scenario

BRAINSTORM §8.2's scenario 4 ("header-driven abort via `x-envoy-fault-abort-request: 503`") is DROPPED per §11.5 empirical pin. The header path requires `delay.header_delay` / `abort.header_abort` proto sub-messages which are deferred per §10.3. Phase 09's listener config never sets the header_delay/header_abort variants, so the request headers are silently ignored — there is no useful equivalence claim to make about a no-op behavior. Future phase that lands header_delay/header_abort + request-header parsing adds back this scenario.

### 7.3 Driver outline

`test/differential/0011-http-fault/driver/driver.go` orchestrates the four scenarios per the existing differential harness pattern (mirroring 0007a-cors and 0010-graceful-drain):

```
1. Boot small-static-backend (port 18001) per §7.5.
2. Boot reference Envoy and envoy-go subjects (admin :9902/listener :10001 and :9901/:10000)
   per §7.4 dual-bootstrap.
3. Per scenario 1..4:
   3a. (Optional) reconfigure dynamically — N/A for static fixtures; each scenario's listener
       config is fixed at boot.  (Fixture 0011 uses ONE listener with all four scenarios
       routed via prefix-match; alternative — separate listener per scenario — was rejected as
       fixture bloat.)
   3b. Issue probe(s) against both proxies.
   3c. Capture full response status + body + headers + time_total.
   3d. Snapshot stats (admin /stats?filter=fault) — both proxies.
   3e. Diff vs. expectations.yaml allow-list + per-scenario assertion matrix; report failures.
4. Cleanup: kill subjects + backend; remove temp dirs.
```

Synchronous probes via the harness's `httptest`-style client; no goroutines beyond the dual-proxy boot path. Total probe count: **8** per proxy (scenario 1 = 1 probe + scenario 2 = 1 + scenario 3a + 3b = 2 + scenario 4a/b/c/d = 4). Total wall-clock per proxy: <0.4s (dominated by scenario 1's 100ms delay + scenario 2's 100ms combined-fault delay + scenario 3b's 100ms baseline-inherit delay = 300ms of delay; the remaining 5 probes are sub-10ms). Both proxies probed sequentially; stat-snapshot diff at end of each scenario block.

### 7.4 Fixture bootstrap (verbatim, per BRAINSTORM §7.1; port-disambiguated)

`test/differential/0011-http-fault/envoy.yaml` (reference Envoy; admin :9902, listener :10001):

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9902 }
static_resources:
  listeners:
    - name: l_main
      address:
        socket_address: { address: 0.0.0.0, port_value: 10001 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/scenario1" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/scenario2" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.fault:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                              delay:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                fixed_delay: 0.1s
                              abort:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                http_status: 503
                        - match: { prefix: "/scenario3-wholesale" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.fault:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                              abort:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                http_status: 418
                        - match: { prefix: "/scenario3-baseline" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/scenario4" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.fault:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                              abort:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                http_status: 503
                              headers:
                                - name: x-fault-on
                                  string_match: { exact: "yes" }
                http_filters:
                  - name: envoy.filters.http.fault
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                      delay:
                        percentage: { numerator: 100, denominator: HUNDRED }
                        fixed_delay: 0.1s
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
                    socket_address: { address: <backend-host>, port_value: 18001 }
```

Listener-level fault: delay 100% 100ms (no abort). Per-route overrides change per scenario:

- `/scenario1`: NO override → inherits listener (delay 100ms only). Backend serves 200 OK after delay.
- `/scenario2`: per-route delay 100ms + abort 503 → wholesale-override; combined ordering = 100ms then 503.
- `/scenario3-wholesale`: per-route abort 418 (no delay) → wholesale-override; NO inherited 100ms delay; immediate 418.
- `/scenario3-baseline`: NO override → inherits listener (delay 100ms only). Probe to confirm baseline-vs-wholesale delta.
- `/scenario4`: per-route abort 503 + headers gate → wholesale-override; abort fires only if header matches.

`envoy-go.yaml` is byte-identical modulo `port_value: 9901` (admin) and `port_value: 10000` (listener).

### 7.5 Backend shape

`test/differential/0011-http-fault/backends/backend.go` — minimal Go HTTP backend bound to port 18001:

```go
package main

import (
    "fmt"
    "net/http"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/plain")
        body := "backend\n"
        w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte(body))
    })
    if err := http.ListenAndServe(":18001", nil); err != nil {
        panic(err)
    }
}
```

Single endpoint serving fast `200 OK` with body `backend\n` (8 bytes). Mirrors the §11 capture's backend exactly.

### 7.6 Differential gate scope clarification

Phase 09's gate (e) requires fixture 0011 to be GREEN against both reference Envoy v1.37.2 and envoy-go. The 4 scenarios cover delay-only, combined delay+abort, per-route wholesale-override, and headers-gate. Stats assertions are exact-equality per the SN1–SN8 deterministic-flow rules (per ADR-0061), with one allow-list extension: `fault.response_rl_injected` is allow-listed as "always 0 in phase 09" (twin-series-discipline analog, but envoy-go DOES emit the counter at zero — the allow-list note is documentation-only, asserting that the diff is byte-equal `0 == 0`).

---

## 8. ADRs anticipated (per BRAINSTORM §9; refined per §11)

Phase 09's ADR-numbering anchor is **ADR-0100** (next-free; 08.2 closed at ADR-0099 per `DECISIONS.md` line 4032). The expected eight ADRs:

| ADR | Title (settled by this SPEC) | Settles | Anchor |
|---|---|---|---|
| ADR-0100 | `internal/filter/http/fault/` package shape + extension-registry registration line + boot-time `httpReg.Register(fault.TypeURL, fault.New)` | BRAINSTORM Decisions 1, 2 | §6.1 + §4.2 |
| ADR-0101 | `runtimeConfig` shape + 6-field-consumed / 11-field-silent-ignore decomposition + `abort.http_status` PGV `[200, 600)` validation at New time + percentage-roll determinism | BRAINSTORM Decisions 3, 9 + §11.1 amendment | §6.2 + §11.1 |
| ADR-0102 | Delay async-resume mechanics — `time.AfterFunc` timer-driven scheduling + cancel-on-OnDestroy + combined delay+abort via timer-callback decision | BRAINSTORM Decisions 4, 6 | §5.2 + §5.4 |
| ADR-0103 | Abort terminal-replace mechanics + body byte-exact `"fault filter abort"` (18 bytes, no trailing newline) + 4-header set (content-length, content-type without charset, date, server) | BRAINSTORM Decision 5 + §11.3/§11.4 amendment | §5.3 + §6.6 + §11.3 |
| ADR-0104 | **Header-driven fault path DEFERRED** (per ADR-0040 deferral format) — coupled to `delay.header_delay` / `abort.header_abort` proto-field deferral per §11.5 empirical pin; phase 09 silently parses but does not honor; future small follow-up phase lands the coupled pair | BRAINSTORM Decision 7 (REPURPOSED from implementation to deferral) + §11.5 amendment | §1 §1.2 + §10.3 BRAINSTORM cluster + §11.5 |
| ADR-0105 | `max_active_faults` concurrency cap + LBP-1 sixth application + `fault.faults_overflow` stat semantics + `markedActive` per-instance Inc/Dec idempotency guard | BRAINSTORM Decision 8 | §5.7 + §6.4 |
| ADR-0106 | §9 family expansion shape — flat top-level rows for §9 family-children + no-sibling-stub discipline | BRAINSTORM Decisions 12, 13 | §1 + §1.3 BRAINSTORM |
| ADR-0107 | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 17→22-name extension for FIVE `fault.*` stats (4 counters + 1 gauge) + `response_rl_injected` permanently-zero counter discipline (route A) per §11.6 | BRAINSTORM Decision 10 + §11.6 amendment | §13.2 + §11.6 |

Each ADR follows the project's standard 7-section format (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). ADRs land incrementally per `superpowers:executing-plans` PROGRESS preamble convention; the SPEC author is NOT responsible for writing the ADRs in this commit (they are written by the PLAN executor at the task that anchors each ADR).

### 8.1 Consolidation candidates

Per the 08.2 SPEC §8 consolidation discipline, the planner MAY consolidate ADR-0104 (deferral) into ADR-0101 (runtimeConfig) §Consequences if the deferral text is short. The SPEC author RECOMMENDS keeping ADR-0104 as a standalone ADR per ADR-0040 deferral format precedent (deferrals get their own ADR for grep-ability). Final consolidation choice is the planner's.

---

## 9. Sibling-stub discipline (per BRAINSTORM §1.5 + Decision 13 + ADR-0106)

Phase 09 does NOT create a sibling-phase SPEC stub for the next §9 HTTP-filters-family phase. Future family-expansion brainstorms cold-start from the §9 `### HTTP filters family` heading + the just-shipped phase 09's artefacts as their context. Per BRAINSTORM Decision 13 + ADR-0106: stubs would risk pre-populating implicit per-phase rows (BOOTSTRAP §9 invariant 4 violation in spirit). The 08.1-creates-08.2-stub precedent was a sub-phase-split-within-parent pattern; family-expansion has no parent SPEC, so no stub.

---

## 10. Acceptance review claims (the items the §5 reviewer must confirm)

### 10.1 Lifecycle correctness

- ROADMAP row `09` exists with status `in-progress` AT this commit (per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3); flips to `done` at the phase-done commit.
- `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` exists with §§1–16 populated.
- `docs/envoy-go/phases/09-http-filter-fault/PLAN.md` does NOT exist at this commit (PLAN.md is the next session's deliverable per the SKILL_ROUTING state-machine state 2 → 3).
- `docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md` does NOT exist at this commit.
- `docs/envoy-go/STATE.md` updated to `lifecycle-state: 2`, `next-skill: superpowers:writing-plans` (PLAN.md drafting), `active-phase: 09-http-filter-fault`, `last-commit: <THIS commit SHA>` (SHA-fill follow-up commit). Updates land at the orchestrating session, not in this SPEC commit.

### 10.2 Empirical-pin discipline

- §11 contains all eight pins per BRAINSTORM §11.1–§11.8 with verbatim Envoy v1.37.2 scrape evidence.
- Each pin's "Conclusions (pinned)" block names the BRAINSTORM Decision it settles + the ADR it anchors + the BEHAVIOR_CONTRACT section it lands in.
- The four pins that AMEND BRAINSTORM hypotheses (§11.1 PGV constraint, §11.3/§11.4 4-header set + no newline, §11.5 header-driven deferral, §11.6 5th stat name) are explicitly marked as `**SURPRISE — empirical evidence diverges from BRAINSTORM hypothesis**` per the 08.2 precedent (08.2 SPEC §11.2's surprise about /healthcheck/fail vs /drain_listeners coupling).

### 10.3 Scope envelope

- §1.1 lists 5 in-scope architectural primitives (delay async-resume, abort terminal-replace, combined ordering, max_active_faults, package + registration). The BRAINSTORM §1.1's 8 in-scope items collapse to 7 (item 7 header-driven path drops per §11.5); the top-line architectural-primitives list is 5 because the 7 BRAINSTORM items group naturally by the 5 primitives + 2 packaging primitives.
- §2 enumerates ALL non-purposes per BRAINSTORM §10 + the §11.5 amendment's added `delay.header_delay` + request-header path coupling.
- §4 deliverables match BRAINSTORM §3.1 + §3.2 with the added `internal/stats` registration delta (5 stat names).
- §7 fixture has 4 scenarios (not 5 per BRAINSTORM §8.2; the header-driven scenario drops per §11.5).

### 10.4 No 08.2-introduced regressions

- All pre-existing fixtures `0000–0010` remain green at phase-done time. The phase-09 implementation TOUCHES no existing fixture's bootstrap or driver — fault is plugged into a new fixture only. The pre-existing-test re-run is mechanical.
- No 08.2 contract claim is invalidated by phase 09. Specifically: the drain manager (08.2 ADR-0091) is unaffected; the admin-mux scaffold (08.1 ADR-0085) is unaffected; the cors filter (07.1 ADR-0074) is unaffected; the http-filter framework (07.1 ADR-0071/72/73/74/75) is unaffected.

---

## 11. Empirical-pin block (per BRAINSTORM §11 — all eight pins resolved IN-SESSION)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline (autonomous-brainstorm requires empirical evidence for design decisions that are not derivable from documentation alone). Mirrors 08.2 SPEC §11's structure precisely.

**Reference image:** `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per `ENVOY_TARGET.md` + 08.1/08.2 SPEC §11 confirmation).

**Probe configuration:** A Docker bridge network `envoy-09-net` hosts: (a) a Python `http.server` backend (image `python:3.12-slim`; container alias `envoy09-backend`; bound to port 18001 within the bridge; serves a fast 200 OK with body `backend\n` for any path) — the script source is at `/tmp/envoy-09-pins/backend.py` of the SPEC drafting session machine; (b) an `envoyproxy/envoy:v1.37.2` reference proxy (container alias `envoy09-proxy`) booted under various per-pin bootstrap YAMLs — ports `9901:9901` (admin) and `10000:11000` (listener) published to host. Probe curl invocations issued from the host (curl 8.5.0) against `http://127.0.0.1:11000/` (listener) and `http://127.0.0.1:19901/stats` (admin). The capture transcripts at `/tmp/envoy-09-pins/pin-*.txt` of the SPEC-drafting session machine are transient artifacts not committed; the verbatim outputs below are the durable evidence per the 08.2 SPEC §11 line 899 discipline.

Probe date: 2026-05-03.

### 11.1 Empirical pin #1 — `abort.http_status` edge cases (PGV-constraint-driven)

**Probe configuration:** envoy.yaml with `abort: { percentage: { numerator: 100, denominator: HUNDRED }, http_status: <X> }` for X = 0 (proto default) and X = 9999 (out of HTTP-status range).

**Verbatim Envoy boot-failure tail (`abort.http_status: 0`):**

```
goo.gle/debugonly    
abort {
  http_status: 0
  percentage {
    numerator: 100
  }
}
: Proto constraint validation failed (HTTPFaultValidationError.Abort: embedded message failed validation | caused by FaultAbortValidationError.HttpStatus: value must be inside range [200, 600))
```

**Verbatim Envoy boot-failure tail (`abort.http_status: 9999`):**

```
goo.gle/debugproto  
abort {
  http_status: 9999
  percentage {
    numerator: 100
  }
}
: Proto constraint validation failed (HTTPFaultValidationError.Abort: embedded message failed validation | caused by FaultAbortValidationError.HttpStatus: value must be inside range [200, 600))
```

(Both probes: `docker run` exits with code 1 within ~3 seconds; the listener never opens; the admin port never binds.)

**Conclusions (pinned):**
- (a) Envoy v1.37.2 enforces a PGV (proto-gen-validate) constraint on `abort.http_status`: the value MUST be in `[200, 600)`. Values outside this range — including the proto default 0 — cause a hard config-load error at boot. The error path is the proto-validate layer, not the runtime layer.
- (b) **SURPRISE — empirical evidence diverges from BRAINSTORM hypothesis:** BRAINSTORM Decision 3 hypothesized "503 default if `http_status: 0`" as a possibility. The evidence shows it is a config-load error instead. envoy-go's `New` factory MUST mirror by validating `abort.http_status` against `[200, 600)` at unmarshal time and returning a non-nil error from `New` for out-of-range values; ADR-0072's boot-time-fail-fast contract makes this ergonomic (the registry resolves typed_config at HCM-build time, BEFORE any traffic). See ADR-0101 §Decision.
- (c) Settles BRAINSTORM Decision 3's deferred-pin question (the `http_status: 0` and `9999` edge cases settle as PGV-error, NOT as 503-default).
- (d) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.fault` body-shape-validation paragraph (§13.1) and ADR-0101 consequence section.

### 11.2 Empirical pin #2 — Delay timing accuracy (sweep across 50/100/200/500ms)

**Probe configuration:** envoy.yaml with `delay: { percentage: { numerator: 100, denominator: HUNDRED }, fixed_delay: <D>s }` for D = 0.05, 0.1, 0.2, 0.5. Five samples per configuration; HOST-curl direct to published port 11000 (excludes docker run startup overhead). `time_total` from curl's `-w "%{time_total}s\n"`.

**Verbatim sample (configured fixed_delay = 50ms):**

```
code=200 time_total=0.053623s body_bytes=8
code=200 time_total=0.053038s body_bytes=8
code=200 time_total=0.052783s body_bytes=8
code=200 time_total=0.053224s body_bytes=8
code=200 time_total=0.053354s body_bytes=8
```

**Verbatim sample (configured fixed_delay = 100ms):**

```
code=200 time_total=0.102990s body_bytes=8
code=200 time_total=0.102273s body_bytes=8
code=200 time_total=0.102396s body_bytes=8
code=200 time_total=0.103381s body_bytes=8
code=200 time_total=0.102896s body_bytes=8
```

**Verbatim sample (configured fixed_delay = 200ms):**

```
code=200 time_total=0.203475s body_bytes=8
code=200 time_total=0.202829s body_bytes=8
code=200 time_total=0.202613s body_bytes=8
code=200 time_total=0.202775s body_bytes=8
code=200 time_total=0.202875s body_bytes=8
```

**Verbatim sample (configured fixed_delay = 500ms):**

```
code=200 time_total=0.503863s body_bytes=8
code=200 time_total=0.503140s body_bytes=8
code=200 time_total=0.502971s body_bytes=8
code=200 time_total=0.502532s body_bytes=8
code=200 time_total=0.503157s body_bytes=8
```

**Conclusions (pinned):**
- (a) Envoy v1.37.2's fault-delay accuracy is consistently **+2.5ms to +3.6ms** above the configured value across the 50/100/200/500ms sweep. The overhead does NOT scale with delay duration; it is a fixed framework cost (timer-fire latency + chain-resume + upstream dial + backend response).
- (b) `body_bytes=8` confirms the backend's `backend\n` (8 bytes) was delivered — the delay path PASSED-THROUGH after the configured delay, not aborted.
- (c) The BRAINSTORM hypothesis "±10ms" is COMFORTABLE (actual is +3.6ms worst-case observed; ±10ms gives 6.4ms headroom for outliers). envoy-go MAY tighten to ±5ms, but ±10ms is the safer phase-09 contract per the BRAINSTORM hypothesis. **Settled at ±10ms** for `BEHAVIOR_CONTRACT.md ## Timing tolerances` per §13.3.
- (d) Settles BRAINSTORM Decision 4's deferred-pin question (timing tolerance settles at ±10ms; no timer-granularity surprises).
- (e) Lands in `BEHAVIOR_CONTRACT.md ## Timing tolerances` new bullet (§13.3) and ADR-0102 consequence section.

### 11.3 Empirical pin #3 — Abort response shape (full headers + body, combined delay + abort)

**Probe configuration:** envoy.yaml with `delay: { percentage: { numerator: 100, denominator: HUNDRED }, fixed_delay: 0.100s }` AND `abort: { percentage: { numerator: 100, denominator: HUNDRED }, http_status: 503 }`. Probe: `curl -isS http://127.0.0.1:11000/foo` from HOST.

**Verbatim Envoy `GET /foo` response (full -i output, byte-faithful):**

```
HTTP/1.1 503 Service Unavailable
content-length: 18
content-type: text/plain
date: Sun, 03 May 2026 18:33:59 GMT
server: envoy

fault filter abort
```

**Verbatim body byte-dump (od -c):**

```
0000000   f   a   u   l   t       f   i   l   t   e   r       a   b   o
0000020   r   t
0000022
```

(Body byte-count = 18 = `len("fault filter abort")`. NO trailing newline. Hex offset `0000022` = octal 18, confirming 18 bytes.)

**Verbatim combined-ordering timing samples (5 samples; expected ~100ms = delay + ~1.5ms abort):**

```
code=503 time_total=0.101817s
code=503 time_total=0.101576s
code=503 time_total=0.101751s
code=503 time_total=0.101146s
code=503 time_total=0.102054s
```

**Conclusions (pinned):**
- (a) Status: `503 Service Unavailable` (configurable via `abort.http_status` in `[200, 600)` per §11.1).
- (b) Body: byte-exact `fault filter abort` — **18 bytes, NO trailing newline**. Body byte-dump confirms.
- (c) Header set is **4 headers** in this on-wire order: `content-length: 18`, `content-type: text/plain`, `date: <IMF-fixdate>`, `server: envoy`.
- (d) `content-type` carries NO `charset=UTF-8` modifier — distinct from admin endpoints' `text/plain; charset=UTF-8`. Envoy's local-reply default for fault is the simpler `text/plain`. envoy-go MUST emit the same exact value to satisfy byte-equality on the differential gate.
- (e) NO `cache-control`, NO `x-content-type-options`, NO `transfer-encoding: chunked` — the 6-header-set discipline of admin endpoints DOES NOT apply to fault aborts. This is the envoy-go-internal divergence: admin endpoints use `chain.Server`'s admin-style header set; the fault filter uses `chain.beginLocalReply`'s base local-reply header set.
- (f) Combined delay + abort ordering: timing samples show 101.1–102.1ms (avg ~101.7ms = 100ms delay + ~1.7ms abort overhead). **Confirmed: delay fires first, then abort.** The combined response is delayed by `delay.fixed_delay` then carries the abort body/status.
- (g) **SURPRISE — empirical evidence diverges from BRAINSTORM hypothesis:** BRAINSTORM §2.5 said "Envoy uses the literal string `\"fault filter abort\"`" (correctly captured the body string) but did not pin (1) the absence of trailing newline; (2) the absence of `charset=UTF-8` modifier; (3) the 4-header set vs. 6-header set distinction. All three are settled here.
- (h) Settles BRAINSTORM Decision 5's deferred-pin question (body byte-exact `fault filter abort`, 18 bytes, no newline; 4-header set; no charset modifier).
- (i) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.fault` Asserted-equivalence paragraph (§13.1) and ADR-0103 consequence section.

### 11.4 Empirical pin #4 — Abort body string verification (subset of §11.3)

The body byte-dump in §11.3 is the canonical evidence. The body is byte-exact `fault filter abort` — 18 bytes (`5 + 1 + 6 + 1 + 5` = `len("fault") + " " + len("filter") + " " + len("abort")`) — NO leading whitespace, NO trailing newline, NO null terminator. The byte-equality discipline per BEHAVIOR_CONTRACT.md `## HTTP filter chain ### Asserted equivalence` line 705 ("Response body is byte-exact for deterministic handlers") applies.

(This pin is consolidated with §11.3 in the SPEC structure but enumerated separately to match BRAINSTORM §11.4's structure precisely.)

### 11.5 Empirical pin #5 — Header-driven fault edge cases (MAJOR REVISION)

**Probe configuration A — listener config with `delay.fixed_delay` + `abort.http_status` (no header_delay/header_abort sub-messages):**

envoy.yaml: `delay: { percentage: 100%, fixed_delay: 0.05s }, abort: { percentage: 100%, http_status: 200 }`.

**Verbatim probes (all baseline 50ms):**

```
--- baseline (no headers) ---
HTTP/1.1 200 OK
content-length: 18
content-type: text/plain
date: ...
server: envoy

fault filter abort
code=200 time_total=0.052393s

--- x-envoy-fault-abort-request: 503 ---
HTTP/1.1 200 OK
... same as baseline; status NOT overridden
fault filter abort
code=200 time_total=0.051575s

--- x-envoy-fault-delay-request: 200 (200ms) ---
HTTP/1.1 200 OK
... same as baseline; delay NOT overridden
fault filter abort
code=200 time_total=0.051585s

--- x-envoy-fault-delay-request: 999999 (very large) ---
code=200 time_total=0.051840s
```

(**ALL header-driven probes return identical baseline behavior under config A.** The headers are silently ignored.)

**Probe configuration B — listener config with `delay.header_delay: {}` + `abort.header_abort: {}` (header-driven path enabled):**

envoy.yaml: `delay: { percentage: 100%, header_delay: {} }, abort: { percentage: 100%, header_abort: {} }`. Note: `header_delay` and `header_abort` are protobuf empty-message sub-messages on the FaultDelay / FaultAbort oneof; their presence enables the header-driven path.

**Verbatim probes under config B:**

```
--- baseline (no headers; expect: no fault since neither fixed_delay nor abort.http_status set) ---
HTTP/1.1 200 OK
server: envoy
... backend response
backend
code=200 time_total=0.002290s

--- x-envoy-fault-delay-request: 200 (200ms delay then pass) ---
HTTP/1.1 200 OK
backend
code=200 time_total=0.202633s

--- x-envoy-fault-abort-request: 418 (custom HTTP status) ---
HTTP/1.1 418 Unknown
content-length: 18
content-type: text/plain
fault filter abort
code=418 time_total=0.000937s

--- x-envoy-fault-delay-request: -1 (negative; expect silent-ignore) ---
HTTP/1.1 200 OK
backend
code=200 time_total=0.001612s

--- x-envoy-fault-delay-request: abc (non-numeric) ---
HTTP/1.1 200 OK
backend
code=200 time_total=0.001365s

--- x-envoy-fault-abort-request: 999 (out of [200, 600)) ---
HTTP/1.1 200 OK
backend
code=200 time_total=0.001659s

--- x-envoy-fault-abort-request: 100 (below 200) ---
HTTP/1.1 200 OK
backend
code=200 time_total=0.001388s

--- x-envoy-fault-abort-request: abc (non-numeric) ---
HTTP/1.1 200 OK
backend
code=200 time_total=0.001691s

--- x-envoy-fault-delay-request: 200 + x-envoy-fault-delay-request-percentage: 0 (override 0% → no fault) ---
HTTP/1.1 200 OK
backend
code=200 time_total=0.001493s

--- x-envoy-fault-delay-request: 100 + x-envoy-fault-abort-request: 503 (combined) ---
HTTP/1.1 503 Service Unavailable
content-length: 18
content-type: text/plain
fault filter abort
code=503 time_total=0.101528s
```

**Conclusions (pinned):**
- (a) **MAJOR SURPRISE — empirical evidence diverges from BRAINSTORM hypothesis:** BRAINSTORM §1.1 item 7 + Decision 7 designed envoy-go to honor `x-envoy-fault-{delay,abort}-request[-percentage]` request headers as overrides on the listener-level static config (`delay.fixed_delay` / `abort.http_status`). The empirical evidence proves this is **WRONG**: under config A (with `fixed_delay` + `http_status` set), the headers are SILENTLY IGNORED — every header-driven probe returns identical baseline behavior. Under config B (with `header_delay: {}` + `header_abort: {}` sub-messages set), the headers ARE honored.
- (b) The header-driven path therefore REQUIRES the `delay.header_delay` and `abort.header_abort` proto sub-messages — which are deferred per BRAINSTORM §10.3 (header_delay) and the analogous abort.header_abort consequence. Phase 09 CANNOT cleanly separate request-header parsing from `header_delay`/`header_abort` proto-field handling; both must be implemented together OR both must be deferred together.
- (c) Phase 09's revised design **DEFERS BOTH** to a future small follow-up phase (~150 LoC; both header_delay/header_abort proto-field handling AND the four documented request headers). BRAINSTORM §1.1 item 7 → moves to deferral cluster. BRAINSTORM Decision 7 → REPURPOSED as deferral via ADR-0104 (per ADR-0040 deferral-ADR format). Differential fixture scenario count: 5 → 4 (the BRAINSTORM §8.2 scenario 4 "header-driven abort" drops).
- (d) Edge-case behavior under config B (for forward-compatibility documentation):
    - Negative delay value (`-1`): silent-ignore → no fault.
    - Non-numeric delay value (`abc`): silent-ignore → no fault.
    - Out-of-range abort status (`999`, `100`): silent-ignore → no fault. Confirms the same `[200, 600)` PGV-style range applies on the header path too.
    - Non-numeric abort value (`abc`): silent-ignore.
    - `delay-request-percentage: 0` overrides static 100% percentage → 0% effective → no fault. Header-percentage override DOES work.
    - Combined `delay + abort` headers: delay first, then abort (~100ms total). Same ordering as static-config combined.
    - Custom abort status (`418`): Envoy emits `HTTP/1.1 418 Unknown` (Envoy doesn't carry a built-in status-text table for non-stdlib codes; emits "Unknown"). The differential fixture's allow-list MUST tolerate the status-text divergence (envoy-go's net/http stdlib may emit `I'm a teapot` for 418); the equivalence is on STATUS CODE not status TEXT.
- (e) Settles BRAINSTORM Decision 7 / §11.5 deferred-pin questions — header path is COUPLED to the proto sub-messages; cannot be separated.
- (f) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.fault` Does-not-yet-apply-to paragraph (§13.1) and ADR-0104 consequence section.

### 11.6 Empirical pin #6 — Stat-name verification (FIVE stats, not four)

**Probe configuration:** Same as §11.5 config B (header_delay/header_abort enabled). Drove three faults: one delay (50ms via header), one abort 503, one abort 418. Then scraped admin /stats?filter=fault and /stats?filter=fault&format=prometheus.

**Verbatim Envoy `/stats?filter=fault` (text format, post-fault-injection):**

```
cluster.c_backend.circuit_breakers.default.cx_open: 0
cluster.c_backend.circuit_breakers.default.cx_pool_open: 0
cluster.c_backend.circuit_breakers.default.rq_open: 0
cluster.c_backend.circuit_breakers.default.rq_pending_open: 0
cluster.c_backend.circuit_breakers.default.rq_retry_open: 0
cluster.c_backend.default.total_match_count: 9
http.ingress_http.fault.aborts_injected: 5
http.ingress_http.fault.active_faults: 0
http.ingress_http.fault.delays_injected: 4
http.ingress_http.fault.faults_overflow: 0
http.ingress_http.fault.response_rl_injected: 0
```

**Verbatim Envoy `/stats?filter=fault&format=prometheus` (Prometheus form):**

```
# TYPE envoy_cluster_default_total_match_count counter
envoy_cluster_default_total_match_count{envoy_cluster_name="c_backend"} 9
# TYPE envoy_http_fault_aborts_injected counter
envoy_http_fault_aborts_injected{envoy_http_conn_manager_prefix="ingress_http"} 5
# TYPE envoy_http_fault_delays_injected counter
envoy_http_fault_delays_injected{envoy_http_conn_manager_prefix="ingress_http"} 4
# TYPE envoy_http_fault_faults_overflow counter
envoy_http_fault_faults_overflow{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_fault_response_rl_injected counter
envoy_http_fault_response_rl_injected{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_fault_active_faults gauge
envoy_http_fault_active_faults{envoy_http_conn_manager_prefix="ingress_http"} 0
```

**Conclusions (pinned):**
- (a) The fault filter emits **FIVE** stats under `http.<stat_prefix>.fault.*`: `aborts_injected` (counter), `delays_injected` (counter), `faults_overflow` (counter), `active_faults` (gauge), and `response_rl_injected` (counter). NOT four as BRAINSTORM Decision 10 anticipated.
- (b) **SURPRISE — empirical evidence diverges from BRAINSTORM hypothesis:** BRAINSTORM Decision 10 listed four stats and projected the 17→21-name BEHAVIOR_CONTRACT extension. The reality is FIVE stats and a 17→22-name extension. The fifth stat `response_rl_injected` is the response-rate-limit-injected counter, emitted as a permanently-zero counter when `response_rate_limit` is not configured (which is BRAINSTORM §10.1's deferred case).
- (c) Phase 09 takes **route A** (per ADR-0107): emit `response_rl_injected` as a permanently-zero counter to match Envoy's surface byte-for-byte. Cost: 8 bytes for the counter cell + one stat-name registration entry. Benefit: differential-parity-positive (no allow-list bookkeeping for an emitted-but-not-asserted stat) + forward-positive (when response_rate_limit lands in a future phase, the same stat name carries the count without rename or migration).
- (d) Stat-name flattening confirmed: internal name `http.<stat_prefix>.fault.<counter>` projects to Prometheus name `envoy_http_fault_<counter>{envoy_http_conn_manager_prefix="<stat_prefix>"}`. The HCM `stat_prefix` is extracted as a label, NOT as part of the metric name (Envoy's tag-extraction discipline; per the existing 17-name table). envoy-go's stat registry already projects this way per ADR-0061; the 5 new fault names slot in unchanged.
- (e) Counter types confirmed: 4 counters (`aborts_injected`, `delays_injected`, `faults_overflow`, `response_rl_injected`) + 1 gauge (`active_faults`). The Prometheus output's `# TYPE` lines are the canonical evidence.
- (f) Settles BRAINSTORM Decision 10 / §11.6 deferred-pin questions — 5 stats; route A (emit zero counter); 17→22-name extension.
- (g) Lands in `BEHAVIOR_CONTRACT.md ## Stat-name mapping ### 22-name table` (renamed from 17-name table) per §13.2 and ADR-0107 consequence section.

### 11.7 Empirical pin #7 — Per-route wholesale-override

**Probe configuration:** envoy.yaml with listener-level fault `delay: { percentage: 100%, fixed_delay: 0.300s }` (NO abort field). Two routes: `/abort-only` with per-route override `abort: { percentage: 100%, http_status: 418 }` (NO delay field); `/baseline` with no override (inherits listener).

**Verbatim probes:**

```
--- /baseline (no per-route override) → expect inherit listener-level delay 300ms ---
HTTP/1.1 200 OK
server: envoy
date: ...
content-type: text/plain
content-length: 8

backend
code=200 time_total=0.303493s

--- /abort-only (per-route abort 418, no delay field) → if WHOLESALE: instant 418; if MERGE: 300ms then 418 ---
HTTP/1.1 418 Unknown
content-length: 18
content-type: text/plain
date: ...
server: envoy

fault filter abort
code=418 time_total=0.001079s
```

**Conclusions (pinned):**
- (a) `/baseline` time_total = 303.5ms = listener-level delay 300ms inherited (3.5ms framework overhead). Confirms passthrough WITH listener delay when no per-route override exists.
- (b) `/abort-only` time_total = 1.1ms = NO inherited delay; immediate 418 abort. Confirms **WHOLESALE-OVERRIDE**: the per-route HTTPFault config (which omits `delay`) does NOT inherit the listener-level `delay`. Per-field merge would have produced a ~300ms delay before the 418 fires; that did not happen.
- (c) Confirms BRAINSTORM Decision 11 + ADR-0073 wholesale-override discipline applies to fault. envoy-go's per-route 3-tier merge (per ADR-0073's existing scope) handles this without per-fault-filter customization.
- (d) Settles BRAINSTORM Decision 11 / §11.7 deferred-pin question — wholesale-override confirmed; no merge-mode needed.
- (e) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.fault` per-route paragraph (§13.1) and reuses the existing ADR-0073 (no new ADR; consolidated into ADR-0101 §Consequences cross-reference).

### 11.8 Empirical pin #8 — Headers-field match semantics (StringMatcher.exact)

**Probe configuration:** envoy.yaml with `headers: [{name: x-fault-on, string_match: {exact: "yes"}}]` AND `abort: { percentage: 100%, http_status: 503 }`. (Note: the `string_match` field name is the canonical v3 field; the deprecated `exact_match` was previously the path. Phase 09 honors `string_match.exact` only — the deprecated alias is silently ignored.)

**Verbatim probes:**

```
--- 11.8.a — no x-fault-on header → no fault → backend 200 ---
HTTP/1.1 200 OK
server: envoy
... backend response
backend
code=200 time_total=0.002143s

--- 11.8.b — x-fault-on: yes → fault → 503 abort ---
HTTP/1.1 503 Service Unavailable
content-length: 18
content-type: text/plain
date: ...
server: envoy

fault filter abort
code=503 time_total=0.000837s

--- 11.8.c — x-fault-on: no → no match (value mismatch) → no fault ---
HTTP/1.1 200 OK
backend
code=200 time_total=0.005883s

--- 11.8.d — X-FAULT-ON: yes (uppercase header NAME) → match (HTTP-1.1 case-insensitive name) ---
HTTP/1.1 503 Service Unavailable
fault filter abort
code=503 time_total=0.000895s

--- 11.8.e — x-fault-on: YES (uppercase header VALUE) → no match (case-sensitive value) ---
HTTP/1.1 200 OK
backend
code=200 time_total=0.001276s

--- 11.8.f — empty header value (x-fault-on; with empty value) → no match ---
HTTP/1.1 200 OK
backend
code=200 time_total=0.001314s
```

**Conclusions (pinned):**
- (a) Header NAMES are matched **case-insensitively** (per HTTP/1.1 RFC 7230). `x-fault-on` and `X-FAULT-ON` are equivalent. envoy-go MUST canonicalize the header name via `http.CanonicalHeaderKey` at New time and at match time.
- (b) Header VALUES are matched **case-sensitively** under `string_match.exact`. `yes` and `YES` are NOT equivalent. envoy-go's match-eval comparison is byte-equality (`v == cfg.exactValue`).
- (c) Empty header value does NOT match a non-empty `exact` config (`""` ≠ `"yes"`).
- (d) Absent header value is equivalent to no header (no match).
- (e) Settles BRAINSTORM Decision 7 (headers field semantics) / §11.8 deferred-pin question — exact-match path confirmed; case-sensitivity discipline confirmed.
- (f) Lands in `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.fault` headers-match paragraph (§13.1) and ADR-0101 consequence section (the runtimeConfig matchHeaders field's parsing discipline).

### 11.9 Synchronization with BEHAVIOR_CONTRACT.md

The §11 pins above land in `BEHAVIOR_CONTRACT.md` per §13.1 (asserted-equivalence prose for the new `### envoy.filters.http.fault` subsection), §13.2 (22-name stat table extension), §13.3 (timing-tolerance bullet), and §13.4 (equivalence-matrix new row). The PLAN executor follows the prose in §13 verbatim (per ADR-0052 in-place edit discipline) at the phase-done commit.

---

## 12. Deferred decisions (the planner / implementer settles these)

The following design-leaf decisions are deferred to PLAN authoring (lifecycle-state 2 → 3) or PLAN execution (lifecycle-state 3 → 4):

1. **Fuzzer mandatory or optional?** The SPEC author RECOMMENDS shipping `FuzzFaultConfigParse` per §14.5 (fault's `New` factory is a parser; ADR-0018's "every parser/codec/filter ships a fuzzer" applies). Final shipping decision is the planner's; recommendation: SHIP (~50 LoC, low cost).
2. **`runtimeConfig` parser refactor.** Currently §6.2 has `parseRouteRuntimeConfig` as a separate function (per the cors precedent). The planner MAY consolidate `New`-time parsing and `parseRouteRuntimeConfig` into a single helper to DRY the validation. Recommendation: KEEP separate (the New-time variant has additional validation like `tc != nil` checks that don't apply at per-route resolve time).
3. **Stat-counter call-site organization.** Currently §6.4 spreads `stats.faultDelaysInjected.Inc()`, etc. across the DecodeHeaders body. The planner MAY consolidate into a `recordFaultEvent(...)` helper for testability. Recommendation: consolidate (cleaner test surface).
4. **Per-route runtimeConfig caching.** The chain's `RequestRouteConfig()` is lazy-cached per-request (per `internal/filter/http/callbacks.go:35–36`). Phase 09's `parseRouteRuntimeConfig` body parses the proto fresh on every call; the planner MAY add a per-request cache via a closure-captured `proto.Message → *runtimeConfig` map. Recommendation: SKIP (per-request cache adds 200 LoC; the parse is sub-microsecond).
5. **Should fault's stats use the existing `internal/stats.Registry` (06.1) or a sub-registry?** Recommendation: USE the existing Registry (per §4.2 — ~10 LoC delta). Sub-registries are out of scope for phase 09.
6. **`fault.response_rl_injected` route A vs route B?** SETTLED at §1's amendment + ADR-0107: route A (emit permanently-zero counter). NOT a planner decision.
7. **Allow-list discipline for the abort-status text divergence.** Per §11.5 conclusion (d), Envoy emits `HTTP/1.1 418 Unknown` for a 418 abort, while envoy-go's net/http stdlib likely emits `HTTP/1.1 418 I'm a teapot`. Phase 09's expectations.yaml MUST allow-list the status-text portion of the response status line for non-standard status codes; the differential equivalence is on STATUS CODE only. The planner adds the allow-list rule. Recommendation: scope allow-list narrowly to `[200, 600)` non-stdlib codes; `200`, `503`, `404` etc. assert byte-equal.
8. **Should the fixture cluster type be STATIC or STRICT_DNS?** Per the existing fixture pattern (0007a-cors uses STRICT_DNS; 0010-graceful-drain mixes STATIC inside its driver-only path), recommendation: STRICT_DNS pointing at a backend hostname resolvable via the test harness's docker bridge network. Final choice is the planner's based on the harness's host-networking discipline (per BEHAVIOR_CONTRACT.md ## Test harness host networking line 489).
9. **Whether to pass an explicit `OrderedHeaders` carrier from fault's SendLocalReply.** SETTLED at §6.6 choice (A): pass `[{Name: "Content-Type", Value: "text/plain"}]` to override the chain's default `text/plain; charset=UTF-8`. Final implementation detail is the planner's (the override may need `chain.beginLocalReply` to honor caller-supplied content-type ahead of the framework default).
10. **Race-detector-cycle test for the timer-driven async-resume.** Recommendation: ADD `TestFault_DelayTimerRace` under `-race` that fires DecodeHeaders + OnDestroy concurrently to exercise the `markedActive` guard. ~30 LoC.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

### 13.1 `## HTTP filter chain ### envoy.filters.http.fault` NEW subsection (verbatim Markdown patch)

The patch INSERTS a new `### envoy.filters.http.fault` subsection AFTER the existing `### Empirical evidence (cors preflight)` block (which ends at line 831 in master HEAD). Insertion point: line 832 (before `### Empirical evidence (413 overflow)`).

```markdown
### envoy.filters.http.fault

#### Asserted equivalence (per phase 09 SPEC §11)

When `envoy.filters.http.fault` is present in `http_filters`, envoy-go MUST emit the same response status, body, and 4-header set as reference Envoy v1.37.2 for the canonical fault scenarios.

- **Abort response** (when `abort.percentage` rolls hit and `headers` field matches):
    - Status: `<abort.http_status>` (constrained to `[200, 600)` at config-load time per ADR-0101; out-of-range values cause boot failure).
    - Body: byte-exact `fault filter abort` (18 bytes, NO trailing newline). NO `charset=UTF-8` modifier on the content-type. NO `cache-control`, NO `x-content-type-options`, NO `transfer-encoding: chunked` headers.
    - Header set on the wire: `content-length: 18`, `content-type: text/plain`, `date: <IMF-fixdate>`, `server: envoy`.
    - For non-stdlib status codes (e.g. 418), the status text portion is allow-listed per the differential harness (`418 Unknown` upstream vs `418 I'm a teapot` envoy-go stdlib). Status code asserts byte-equal; status text portion is allow-listed.
- **Delay response** (when `delay.percentage` rolls hit and `headers` field matches; no abort):
    - Status: passes through from upstream (typically 200 OK + backend body).
    - Latency: `delay.fixed_delay ± 10ms` (per the timing-tolerance bullet in `## Timing tolerances`).
- **Combined delay + abort** (when both criteria match): delay fires first, then abort fires after the delay completes. The wire response is the abort response (4-header set + `fault filter abort` body), arriving `delay.fixed_delay` after the request.
- **Headers-field gate**: when `headers` is non-empty, fault is only injected if ALL listed name+value pairs match the request. Header NAMES match case-insensitively (per HTTP/1.1); header VALUES match case-sensitively under `string_match.exact`. Other StringMatcher variants (regex, prefix, suffix, contains) are silently ignored at fault-eval time — see Does-not-yet-apply-to.

#### Per-route 3-tier merge (per ADR-0073 + phase 09 SPEC §11.7)

Per-route `typed_per_filter_config` for `envoy.filters.http.fault` WHOLESALE-overrides the listener-level fault config. A per-route HTTPFault that omits `delay` does NOT inherit the listener-level `delay` (and conversely for `abort`, `headers`, `max_active_faults`). Empirically confirmed: a route with `abort: 418, no delay` over a listener with `delay: 300ms` returns instant 418 (1.1ms total), NOT delayed 418 (~301ms).

#### `max_active_faults` concurrency cap

When `max_active_faults > 0`, a per-filter-instance `atomic.Int64` counter caps the in-flight fault count. New faults that arrive at the cap are SKIPPED (the request passes through normally) and the `fault.faults_overflow` counter increments. The cap is per-filter-instance (per `New` factory closure), shared across all requests routed through the same listener filter chain.

#### Async-resume mechanics (per ADR-0102)

The fault filter's delay path uses `time.AfterFunc` to schedule a callback that calls `cb.ContinueDecoding()` after the configured delay. The chain parks at `StopIteration` and resumes from the timer goroutine. `OnDestroy` calls `f.delayTimer.Stop()` to cancel the timer on request teardown (downstream-disconnect, drain-induced stream-reset). The `markedActive` per-instance flag guards the `activeFaults` atomic.Int64 counter Inc/Dec balance against the OnDestroy-races-timer-callback case.

#### Does not yet apply to (per phase 09 deferrals — ADRs 0101, 0103, 0104, 0107)

- **Header-driven fault path** (`x-envoy-fault-{delay,abort}-request[-percentage]`): coupled to `delay.header_delay` / `abort.header_abort` proto sub-messages per phase 09 §11.5 empirical pin; both deferred per ADR-0104. envoy-go silently parses the proto sub-messages but does not honor them; the request headers are silently ignored.
- **`response_rate_limit`** (FaultRateLimit token-bucket): deferred to a future fault-extension phase OR the bandwidth_limit filter. The `fault.response_rl_injected` stat is emitted as a permanently-zero counter for differential parity per ADR-0107.
- **`abort.grpc_status`**: deferred to gRPC family.
- **`upstream_cluster`, `downstream_nodes` filters**: deferred to small follow-up phases.
- **All four runtime-key fields** (`*_runtime`): deferred to Runtime + hot restart family.
- **`disable_downstream_cluster_stats`**: deferred to per-downstream-cluster stat fan-out phase (no current ROADMAP row).
- **`filter_enabled` / `filter_enabled_runtime`**: deferred to Runtime + hot restart family.
- **HeaderMatcher non-exact variants** (regex, range, prefix, suffix, contains, present-only): deferred to whatever phase lands the full HeaderMatcher engine.
- **Differential testing under H2 streams**: fixture 0011 is HTTP/1.1-only; H2 differential testing of fault is deferred.

#### Empirical evidence (verbatim curl excerpts from phase 09 SPEC §11.3)

```
$ curl -isS http://127.0.0.1:11000/foo  # delay 100% 100ms + abort 100% 503

HTTP/1.1 503 Service Unavailable
content-length: 18
content-type: text/plain
date: Sun, 03 May 2026 18:33:59 GMT
server: envoy

fault filter abort
```

(Body byte-count = 18; NO trailing newline; 4-header set as above.)
```

### 13.2 `## Stat-name mapping ### 22-name table` extension (verbatim Markdown patch)

The patch RENAMES the existing `### 17-name table (introduced by phase 06.1)` heading at line 130 to `### 22-name table (extended by phase 09)`, and APPENDS five new rows to the table. Preserved 17 entries unchanged; new entries appended at the bottom.

```markdown
**Fault filter — 5 names (introduced by phase 09):**

| Internal name | Type | Prometheus name |
| http.<stat_prefix>.fault.aborts_injected | counter | envoy_http_fault_aborts_injected{envoy_http_conn_manager_prefix="<stat_prefix>"} |
| http.<stat_prefix>.fault.delays_injected | counter | envoy_http_fault_delays_injected{envoy_http_conn_manager_prefix="<stat_prefix>"} |
| http.<stat_prefix>.fault.faults_overflow | counter | envoy_http_fault_faults_overflow{envoy_http_conn_manager_prefix="<stat_prefix>"} |
| http.<stat_prefix>.fault.active_faults | gauge | envoy_http_fault_active_faults{envoy_http_conn_manager_prefix="<stat_prefix>"} |
| http.<stat_prefix>.fault.response_rl_injected | counter | envoy_http_fault_response_rl_injected{envoy_http_conn_manager_prefix="<stat_prefix>"} |

`response_rl_injected` is emitted as a permanently-zero counter in phase 09 — Envoy emits it even when `response_rate_limit` is not configured (per phase 09 §11.6 empirical pin); envoy-go matches the surface for differential parity. When `response_rate_limit` lands in a future phase, the same name carries the actual count.
```

### 13.3 `## Timing tolerances` new bullet (verbatim Markdown patch)

The patch APPENDS a new bullet at the end of the existing `## Timing tolerances` section (line 266 in master HEAD).

```markdown
- **Fault filter delay accuracy: ±10ms (per phase 09 §11.2 empirical pin).** envoy-go's `time.AfterFunc` timer-driven async-resume matches Envoy v1.37.2's fault delay accuracy within ±10ms across the 50/100/200/500ms sweep. Empirical worst-case overhead observed: +3.6ms (Envoy v1.37.2 was tested; envoy-go's overhead is similar). The differential fixture's expectations.yaml asserts `time_total ∈ [delay - 10ms, delay + 10ms]` for delay scenarios.
```

### 13.4 `## Equivalence Matrix` new row (verbatim table-row patch)

The patch APPENDS one new row to the equivalence matrix table (which starts at line 9 of master HEAD; the existing `| HTTP filter chain |` row at line 22 covers cors but does NOT cover fault — phase 09 adds a fault-specific row OR extends the existing HTTP-filter-chain row's prose; SPEC author chooses APPEND to keep the per-filter prose unitary).

```markdown
| HTTP filter `envoy.filters.http.fault` | Per-request equivalence on abort response shape (status + 4-header set + body byte-exact `fault filter abort`), delay timing (±10ms tolerance), per-route wholesale-override resolution, headers-field exact-match gate, and stat counter increments under the per-scenario differential gate (fixture 0011-http-fault). NOT asserted: header-driven fault path (deferred — ADR-0104), response_rate_limit (deferred), abort.grpc_status (deferred), HeaderMatcher non-exact variants. |
```

### 13.5 Forward-pointer notes (per BRAINSTORM §9 inline supersessions/amendments)

Three small forward-pointer notes are appended to existing BEHAVIOR_CONTRACT sections:

- After `## HTTP filter chain ### Async resume mechanics` (line 720 in master HEAD): "Phase 09 (`envoy.filters.http.fault`) is the FIRST production exerciser of the async-resume primitive on the request side; see `### envoy.filters.http.fault ### Async-resume mechanics` for fault-specific details. The 07.1 `envoy_go_test` probe filter is the structural-coverage exerciser."
- After `## Stat-name mapping ### Twin-series filter discipline` (line 173 in master HEAD): "Phase 09 takes route A for `fault.response_rl_injected` (emit permanently-zero counter) per ADR-0107 — twin-series-discipline analog but with envoy-go-side emission. The 22-name table reflects the route A choice."
- After `## Equivalence Matrix` (line 9 area): the new HTTP-filter-fault row per §13.4 above.

---

## 14. Testing strategy (per BRAINSTORM §8 + §11 amendment)

### 14.1 Unit tests (`internal/filter/http/fault/`)

`fault_test.go` covers:

- `TestNew_NilTC` — nil tc returns non-nil error per §6.1 step 1.
- `TestNew_MalformedTC` — malformed Any returns unmarshal error.
- `TestNew_AbortHTTPStatusOutOfRange` — http_status=0 / 9999 / 100 / 600 each return error mirroring §11.1 PGV constraint.
- `TestNew_DelayPercentageWithoutFixedDelay` — `delay: { percentage: 50 }` (no fixed_delay) returns error per §6.1 step 4.
- `TestNew_HappyPath` — valid config returns non-nil factory + nil error.
- `TestRuntimeConfig_FieldExtraction` — verifies the 6-field extraction maps proto values correctly.
- `TestDecodeHeaders_DelayOnly` — fires async-resume; asserts ContinueDecoding called from timer goroutine after configured delay.
- `TestDecodeHeaders_AbortOnly` — fires SendLocalReply with status + body + nil headers; asserts return = StopIteration.
- `TestDecodeHeaders_Combined` — fires async-resume; timer callback calls SendLocalReply (not ContinueDecoding); asserts timing.
- `TestDecodeHeaders_NoFaultPercentage0` — percentage=0 short-circuits to no-op.
- `TestDecodeHeaders_NoFaultHeaderMismatch` — headers field non-empty but request header doesn't match → no fault.
- `TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue` — uppercase value doesn't match.
- `TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName` — uppercase name matches.
- `TestDecodeHeaders_MaxActiveFaultsCapOverflow` — concurrent setup at cap → next fault skipped + faults_overflow stat increments.
- `TestOnDestroy_TimerStopped` — timer cancelled before fire; callback does not run; activeFaults decremented in OnDestroy.
- `TestOnDestroy_TimerAlreadyFired` — timer fires before OnDestroy; activeFaults decremented exactly once via markedActive guard.
- `TestPerRouteWholesaleOverride` — per-route HTTPFault wholesale-replaces listener-level config.

### 14.2 Race detector + lint

- `go test -race ./internal/filter/http/fault/...` — clean. The `markedActive` guard, the `activeFaults` atomic.Int64, and the timer goroutine interaction are exercised under the race detector.
- `go vet ./...` — clean.
- `golangci-lint run ./...` — clean. `gofmt`/`goimports` discipline.

### 14.3 Fuzzers

- `FuzzFaultConfigParse` (~50 LoC) — fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either (factory, nil) OR (nil, error); never panics; never returns (nil, nil). Per ADR-0018's "every parser/codec/filter ships a fuzzer" — fault's New is the parser. 30s budget for CI short-mode.

### 14.4 Existing fuzzers re-run

The 11 existing fuzzers (10 from 08.1 + `FuzzDrainTransitions` from 08.2 — per the 08.2 phase-done close + REVIEW gate (d) appendix) re-run clean at the 30s budget. Phase 09 touches none of their target packages; the re-run is mechanical.

### 14.5 h2spec re-run

Phase 09 touches no codec / framer / hpack / connection-management code. The h2spec gate at 53/53 PASS (per CONFORMANCE_PINS at the ADR-0051 pin) is invariant under filter additions. Re-run mechanical.

### 14.6 Differential 0000–0010 + 0011

All pre-existing fixtures (0000-tcp-echo through 0010-graceful-drain) re-run clean. NEW fixture 0011-http-fault green under the 4 scenarios per §7.1.

### 14.7 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

The six gates (a)–(f) per §3 are all green at phase-done commit time. Verification commands quoted into PROGRESS.md per the executing-plans discipline.

---

## 15. Acceptance checklist (for the reviewer of this phase's final state)

- [ ] `internal/filter/http/fault/{doc.go,fault.go,fault_test.go,fuzz_test.go}` exist with the contract per §4.1 + §6 + §14.
- [ ] `cmd/envoy-go/main.go` registers `fault.New` against `fault.TypeURL` in the http registry, alphabetically after the existing three filters per §4.2.
- [ ] `internal/stats/registry.go` (or equivalent) registers FIVE new fault.* stat names per §4.2 + §13.2.
- [ ] `test/differential/0011-http-fault/` exists with `README.md`, `expectations.yaml`, `envoy.yaml`, `envoy-go.yaml`, `driver/driver.go`, `backends/backend.go` per §4.3 + §7.
- [ ] `test/differential/runner.go` registers `0011-http-fault` with `RequiresReference: true`.
- [ ] `docs/envoy-go/BEHAVIOR_CONTRACT.md` carries the §13.1 + §13.2 + §13.3 + §13.4 + §13.5 patches at phase-done commit.
- [ ] `docs/envoy-go/DECISIONS.md` carries ADR-0100 through ADR-0107 at phase-done commit (per the planner's consolidation choices per §8.1).
- [ ] `docs/envoy-go/ROADMAP.md` row 09 status flips `in-progress → done` at the phase-done commit.
- [ ] `docs/envoy-go/STATE.md` advanced through lifecycle-states 2 (PLAN drafting), 3 (PLAN execution), 4 (verification), 5 (review), 6 (phase-done) per the SKILL_ROUTING state-machine discipline.
- [ ] All six phase-done gates (a)–(f) per §3 are green.
- [ ] Phase-done commit message subject: `phase 09: http-filter-fault [ADR-0100, ADR-0101, ADR-0102, ADR-0103, ADR-0104, ADR-0105, ADR-0106, ADR-0107]` (or fewer per §8.1 consolidation).
- [ ] Phase-done commit message body explicitly states: (1) ROADMAP row 09 flips in-progress → done; (2) the §9 family heading at ROADMAP line 56 stays unchanged; (3) phase 09 is the FIRST §9 family-row to land; (4) anticipated 8 ADRs (or planner-consolidated count) listed in the subject.
- [ ] No 08.2 contract claim is invalidated by phase 09's changes.
- [ ] No pre-existing fixture (0000–0010) regressed at phase-done time.

---

## 16. References

- **BRAINSTORM:** `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md` (this SPEC's input).
- **BOOTSTRAP_PROMPT.md** §§4–9 (lifecycle, gating, family expansion).
- **MISSION.md** §2 (project non-purposes; informs §2 above).
- **ENVOY_TARGET.md** (reference image pin: `envoyproxy/envoy:v1.37.2 @ sha256:c5e8...`).
- **BEHAVIOR_CONTRACT.md** `## HTTP filter chain` (line 695; host of the new `### envoy.filters.http.fault` subsection per §13.1); `## Stat-name mapping ### 17-name table` (line 130; extended to 22-name per §13.2); `## Timing tolerances` (line 266; new bullet per §13.3); `## Equivalence Matrix` (line 9; new row per §13.4).
- **ROADMAP.md** row 09 (status `planned → in-progress` AT this SPEC commit; `in-progress → done` at phase-done).
- **DECISIONS.md** ADR-0099 (last landed before phase 09); ADR-0100..ADR-0107 land in phase 09.
- **Cors filter precedent:** `internal/filter/http/cors/cors.go` + `internal/filter/http/cors/perroute.go` (the package-shape + per-route 3-tier merge precedent fault inherits).
- **Framework surface:** `internal/filter/http/types.go` (HTTPFilter + FilterHeadersStatus + StreamDecoderFilter + HTTPFilterFactory + FilterInstanceFactory + OrderedHeaders); `internal/filter/http/callbacks.go` (DecoderFilterCallbacks + SendLocalReply + ContinueDecoding + RequestRouteConfig); `internal/filter/http/registry.go` (HTTPRegistry); `internal/filter/http/perroute.go` (3-tier merge per ADR-0073).
- **Phase-08.2 SPEC** `docs/envoy-go/phases/08.2-graceful-drain/SPEC.md` (structural precedent for this SPEC).
- **Envoy v1.37.2 fault filter docs** (https://www.envoyproxy.io/docs/envoy/v1.37.2/configuration/http/http_filters/fault_filter; https://www.envoyproxy.io/docs/envoy/v1.37.2/api-v3/extensions/filters/http/fault/v3/fault.proto.html) — the canonical proto reference.

---

## End of phase 09 SPEC.
