# Phase 09 Brainstorm — `envoy.filters.http.fault`

**Status:** brainstorm complete. This document captures the design decisions reached during the lifecycle-state-0 → 1 brainstorm session for phase 09 (`http-filter-fault`), the first concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family. The next session (lifecycle-state 1 → 2 for phase 09, skill `superpowers:writing-plans`) authors `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §11 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Brainstorm session:** worktree `.worktrees/phase-09-http-filter-fault-brainstorm`, branch `phase-09-http-filter-fault-brainstorm`, branched from master tip `14a68e75ecb9b5ef16e4c1afd8237253207c469b` (the 08.2 phase-done SHA-fill commit `14a68e7`; that follow-up landed the `b33e04f` SHA into STATE.md + PROGRESS.md after the 08.2 phase-done commit, mirroring the 04..08.1 SHA-fill convention).

**Brainstorm mode:** interactive with a live human (the user picked the family + first-filter selections via three-question dialogue: §9 family = HTTP filters, first filter = `envoy.filters.http.fault`, MVP scope envelope = the in-scope/deferred lists settled in §1.1 / §10 below). Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0099), and the just-shipped 08.2 + 08.1 + 07.1 sub-phase artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §11 and deferred to SPEC-drafting time per the 08.2 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/08.2-graceful-drain/BRAINSTORM.md` section-for-section, reframed for a single-filter scope. Sections §§1–12 are decision-bearing prose; §11 enumerates the empirical-pin obligations the 09 SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

---

## 1. Mission and scope confirmation (09 only)

ROADMAP row `09 | http-filter-fault | 08 | planned | | …` (added by this brainstorm, see §12 below) is the row this brainstorm advances to `in-progress` at the SPEC commit. Phase 09 is the first concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading (the family heading at `ROADMAP.md` line 55 — `### HTTP filters family` — is a conceptual umbrella, not a row, per BOOTSTRAP §9 invariant 4). The MVP-trunk-close commit `b33e04f` (08.2 phase-done) is this row's `depends-on` anchor.

The HTTP filters family lists 16 candidate filters (`ROADMAP.md` line 57): header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. `cors` already shipped in phase 07.1 as the trivial real filter (per `internal/filter/http/cors/cors.go` + ADR-0074). This phase ships `fault` as the SECOND real filter — the canonical Envoy-style "production introductory filter" — and establishes the per-filter-phase pattern the rest of the family will follow (one filter per phase row, rare exceptions for tightly-paired filters that share infrastructure).

### 1.1 What 09 delivers as a self-contained whole

Phase 09 lands `envoy.filters.http.fault` (the canonical Envoy fault-injection filter) under the 07.1 framework. Eight in-scope items:

1. **New `internal/filter/http/fault/` package** owning the filter implementation. The package mirrors the `internal/filter/http/cors/` shape: `fault.go` (filter type + factory + decode/encode methods), `fault_test.go` (unit tests), `doc.go` (package overview). The package exposes `TypeURL` (the canonical type-URL constant) + `New` (the `HTTPFilterFactory`) per the cors precedent (`internal/filter/http/cors/cors.go:13` + `:22`).
2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering `router.New`, `cors.New`, `envoygotest.New` at lines 111–113 of the worktree HEAD `14a68e7`) gains a fourth `httpReg.Register(fault.TypeURL, fault.New)` call before the `httpReg.Freeze()` invocation (line 122). No other filter wiring changes.
3. **Proto-config parsing** of `envoy.extensions.filters.http.fault.v3.HTTPFault`, the canonical filter-level config message. Per `go-control-plane`'s `envoy/extensions/filters/http/fault/v3` package (proto pin via ADR-0008 → Envoy v1.37.2 → proto v3), the message has 11 fields; phase 09 consumes 7 (`delay`, `abort`, `headers`, `max_active_faults`, plus the four nested-message subfields it implies — `delay.percentage`, `delay.fixed_delay`, `abort.percentage`, `abort.http_status`) and explicitly silently-ignores 4 (`upstream_cluster`, `downstream_nodes`, `response_rate_limit`, `disable_downstream_cluster_stats` — see §10 for the deferral discipline).
4. **Delay implementation — async-resume semantics.** When the request matches the delay-injection criteria (per §2.4 below), `DecodeHeaders` returns `StopIteration` after starting a `time.AfterFunc(fixed_delay, …)` that fires `cb.ContinueDecoding()`. The chain parks; the timer wakes the chain after the delay; the request resumes through subsequent filters and onward to the upstream. This is the FIRST real filter to exercise the framework's async-resume primitive (per `internal/filter/http/callbacks.go:13–17` + ADR-0071 §async-resume); the `envoy_go_test` probe filter exercised it structurally in 07.1 but no production filter has until now.
5. **Abort implementation — terminal-replace semantics.** When the request matches the abort-injection criteria (per §2.5 below), `DecodeHeaders` invokes `cb.SendLocalReply(http_status, body, headers)` and returns `StopIteration`. The chain enters the encode iteration starting at `filter[len-1]` per ADR-0075; the upstream is never dialed. The body shape is empirical-pinned in §11 (Envoy uses the literal string `"fault filter abort"` per its source-of-truth observation; see §11 deferral).
6. **Combined delay+abort ordering.** When BOTH delay AND abort match for the same request, Envoy applies delay FIRST and then abort fires after the delay completes (per Envoy docs at `https://www.envoyproxy.io/docs/envoy/v1.37.2/configuration/http/http_filters/fault_filter#runtime`); envoy-go matches this ordering. The mechanism: `DecodeHeaders` sees both match, schedules the delay timer; on timer fire, `SendLocalReply` is called from inside the timer goroutine (which transitions to the chain's resume/encode path).
7. **Header-driven fault enable** via `x-envoy-fault-delay-request` (header value = milliseconds delay) and `x-envoy-fault-abort-request` (header value = HTTP status code). Per Envoy docs the header path requires the proto's `*_percent_runtime` runtime keys to evaluate to 100% AND the corresponding `*.percentage` field to be set; the filter's `headers` field acts as a request-match precondition. Envoy-go honors the request-header path with simplified semantics (see Decision 7 in §2): when `delay.percentage` is configured and the request carries `x-envoy-fault-delay-request`, the header value overrides `delay.fixed_delay`. Same pattern for abort.
8. **`max_active_faults` concurrency cap.** A `sync.atomic.Int64` per-filter-instance counter increments at fault-trigger time (delay timer scheduled OR abort about to fire) and decrements at fault-completion (timer fires / abort response written). When the counter is at the configured cap, NEW faults are silently NOT injected (the request passes through normally) and a stat (`fault.faults_overflow`) increments. This exercises a different framework primitive — concurrency-bounded fault injection — that complements async-resume.

Plus three artifact-level deliverables:

9. **Differential fixture `0011-http-fault`** under `test/fixtures/0011-http-fault/`: `envoy.yaml` + `envoy-go.yaml` + a Go driver in `inputs/driver.go` exercising five scenarios (per §8 below). The fixture asserts response status, body-shape (under the BEHAVIOR_CONTRACT.md tolerance for the abort body), and timing (under the §11 timing-tolerance pin).
10. **`BEHAVIOR_CONTRACT.md` extension** under the existing `## HTTP filter chain` umbrella (line 695): a NEW `### envoy.filters.http.fault` subsection added after the existing `### Empirical evidence (cors preflight)` block (line 748–831). The fault subsection covers async-resume timing tolerance, abort body shape, header-driven fault behavior, and `max_active_faults` overflow semantics. Plus a NEW twin-series `## Stat-name mapping` extension covering the four `fault.*` counters (`fault.aborts_injected`, `fault.delays_injected`, `fault.active_faults`, `fault.faults_overflow`).
11. **Anticipated 8 ADRs (ADR-0100 through ADR-0107)** per §9 below. ADR-0099 is the highest-numbered ADR landed in 08.2 (per `DECISIONS.md` line 4088 — last `## ADR-` heading); ADR-0100 is the next-free.

### 1.2 What 09 does NOT deliver (forward to §10)

The exhaustive deferral list lives in §10. The summary: `response_rate_limit`, `abort.grpc_status`, `delay.header_delay`, `upstream_cluster`, `downstream_nodes`, `disable_downstream_cluster_stats`, all four runtime-key fields (`delay_percent_runtime`, `abort_percent_runtime`, `max_active_faults_runtime`, `response_rate_limit_percent_runtime`), `filter_enabled` / `filter_enabled_runtime` are out-of-scope. None are blockers for closing the row 09 phase-done; each gets a deferral ADR per the ADR-0040 deferral-ADR format.

### 1.3 Phase-done as family-entry milestone

Phase 09's phase-done commit closes ROADMAP row `09` (single-row, no parent-child split anticipated; see §1.4). It does NOT close any §9 family heading (family headings are not rows per BOOTSTRAP §9 invariant 4) — the HTTP filters family stays "in-progress" implicitly until the last filter under the family ships, but no row tracks that aggregate. Each subsequent filter under the family becomes its own top-level row (10 = next filter, 11 = next, …) per BOOTSTRAP §9's "each family is brainstormed as its own phase when it enters in-progress" wording, interpreted by this brainstorm as "each filter is its own phase row." An alternative reading — phase 09 = parent row `http-filters` with sub-phases 09.1 (fault), 09.2 (next filter), … — is rejected because (a) the family has 16+ candidate filters which would create a 16-deep parent that violates the §6 splitting policy at the ~16-sub-phase scale; (b) BOOTSTRAP §9 invariant 4's wording "each family is brainstormed as its own phase" does not say "each family is one parent phase" — the more natural reading is "each phase under the family is its own brainstormed phase"; (c) no MVP-trunk precedent exists for a parent phase with more than 2 sub-phases (the 05/06/07/08 splits all stopped at 2). The flat top-level-row pattern (09 = fault, 10 = next, …) is therefore the project's chosen family-expansion shape, codified by Decision 12 in §2 below.

### 1.4 ADR-0045 split-by-surface readiness

Per `STATE.md` `next-skill-scope` ("scopes the first family as a parent phase (likely needing further splitting per ADR-0045)"), this brainstorm anticipates the SPEC/PLAN authors might find phase 09's surface big enough to trigger an ADR-0045 surface-split into 09.1 / 09.2. The brainstorm's POSITION is that phase 09 is **single-row at brainstorm time** — a cohesive ~600–900 LoC slice covering a single filter — but the planner-time release valve stays available. If the SPEC author finds the surface > 1500 LoC estimated or the PLAN > 25 tasks, the natural split is:

- **09.1 = `fault` delay + abort + headers + headers field** (the core-MVP slice).
- **09.2 = `fault` `max_active_faults` + per-route config + stats finalization** (the operational-readiness slice).

This split would mirror 08.1 (admin endpoints) + 08.2 (graceful drain) in shape. The brainstorm does NOT pre-commit to the split; that's the SPEC author's call.

### 1.5 Seed-stub alignment

Unlike 08.2 (which had a sibling SPEC stub `README.md` left by the 08.1 SPEC commit per the parent-08 brainstorm), phase 09 has NO sibling SPEC stub. The HTTP filters family entered fresh at this brainstorm. Decision 13 in §2 codifies that future family-entry brainstorms should NOT pre-author SPEC stubs for siblings (the BOOTSTRAP §9 invariant 4 forbids pre-populating per-phase rows; pre-populating SPEC stubs would be a parallel violation in spirit). Each next-filter phase brainstorms cold from the §9 heading.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

This section is the brainstorm's decision log. Each Decision states **what** is chosen, **why** that option vs. its alternatives, what **deferred-pin** obligations (if any) remain for SPEC-time empirical work, and what **ADR anchor** the SPEC author should expect. ADR numbering starts at **ADR-0100** (next-free; 08.2 closed at ADR-0099 per `DECISIONS.md` line 4088).

### 2.1 Filter package layout *(Decision 1 → ADR-0100)*

**Decision:** New package `internal/filter/http/fault/` with files mirroring the cors precedent: `fault.go`, `fault_test.go`, `doc.go`. The package exports two top-level symbols: `TypeURL` (string constant, `"type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault"`) and `New` (the `HTTPFilterFactory`). All other types (`filter`, `instance`, `runtimeConfig`) are unexported.

**Why this vs. alternatives:**
- *Why not a single `internal/filter/http/fault.go` flat file?* The cors filter is in its own subpackage (`internal/filter/http/cors/cors.go`) for namespace isolation — `cors.New` is the registry entry point, distinct from `router.New`, `envoygotest.New`. Same discipline for fault: subpackage isolation lets future filters (`fault2` etc.) import `internal/filter/http/fault` without import cycles. The 07.1 precedent is unanimous (cors, router, envoygotest each get their own subpackage).
- *Why not unify under a `internal/extensions/filters/http/fault/` Envoy-source-style path?* envoy-go is explicitly NOT mirroring Envoy's C++ source structure (`MISSION.md` §2.2 non-purpose). The `internal/filter/http/<name>/` pattern is the project's own convention.

**Deferred to SPEC:** package-internal type structure (whether to factor `runtimeConfig`, `filterInstance`, etc. as separate types vs. all-on-one `filter` struct) — the SPEC author chooses based on test readability. No ADR-class commitment from brainstorm.

**ADR anchor:** ADR-0100 — Filter package shape conformance with cors precedent.

### 2.2 Extension-registry registration *(Decision 2 → ADR-0100)*

**Decision:** `cmd/envoy-go/main.go` adds a single new line: `httpReg.Register(fault.TypeURL, fault.New)` between the existing `cors.New` registration (line 112 at the worktree HEAD `14a68e7`) and the existing `envoygotest.New` registration (line 113). The registration ordering is alphabetical by filter-name ('c' before 'e' before 'f' is wrong — the correct alphabetical order would put 'f' AFTER 'e'). Confirmed alphabetical insertion: cors → envoy_go_test → fault → router (router is already on line 111). Per ADR-0072, registration ordering does not affect runtime behavior; this is a stylistic discipline only. The brainstorm settles on **alphabetical-after-router** as the project convention going forward (router stays first as the framework's canonical terminal filter; everything else is alphabetized after it).

**Why this vs. alternatives:**
- *Why not registration-order = config-list-order?* Registration order is a global discipline; config-list order is per-listener / per-route. Decoupling avoids cross-cutting coupling.
- *Why router-first-then-alphabetical?* Router is the canonical terminal filter (the 07.1 chain validation per `internal/filter/http/chain_shape.go:33` requires router as the last entry). Stylistic asymmetry codifies that load-bearing role.

**Deferred to SPEC:** none — the line edit is mechanical.

**ADR anchor:** ADR-0100 — folded into the package-shape ADR (the registration line is a one-line consequence of the package shape).

### 2.3 Proto-config parsing *(Decision 3 → ADR-0101)*

**Decision:** The `New` factory unmarshals `tc *anypb.Any` into a `*envoyextensionsfiltershttpfaultv3.HTTPFault` value via `tc.UnmarshalTo(&cfg)`. Per the cors precedent (`internal/filter/http/cors/cors.go:24–27`), the unmarshal-and-discard pattern for "no fields envoy-go consumes" filters is wrong here — fault has 7 consumed fields. The `New` factory MUST parse the message into a long-lived `*runtimeConfig` value held by the factory closure, capturing:

```go
type runtimeConfig struct {
    delayEnabled       bool         // delay != nil
    delayPercentage    float64      // delay.percentage in [0, 100]
    delayFixedDelay    time.Duration // from delay.fixed_delay (proto duration)

    abortEnabled       bool         // abort != nil
    abortPercentage    float64      // abort.percentage in [0, 100]
    abortHTTPStatus    int          // abort.http_status (HTTP status code; 0 if unset, treat as 503)

    matchHeaders       []headerMatch // headers field; empty = match-all

    maxActiveFaults    int64        // 0 = no cap (nil pointer in proto); int64 for atomic counter ABI
}
```

Per-instance state (the `filter` struct returned by `FilterInstanceFactory()`) holds NO config pointer — config is resolved from the closure capture. Per-route config is RESOLVED PER-REQUEST from `cb.RequestRouteConfig()` (Decision 11 below) and merged on top of the listener-level `runtimeConfig` per ADR-0073's 3-tier merge.

**Why this vs. alternatives:**
- *Why not lazy-parse on first DecodeHeaders?* The `New` factory contract per ADR-0072 requires factories to validate `typed_config` shape at registration time so misconfiguration fails at boot, not under traffic. Parsing in DecodeHeaders defers errors to traffic time, which violates that contract.
- *Why not store the proto message directly?* The proto types carry serialization machinery (reflection paths, unknown-fields buffer) that are ~3× larger than the dehydrated `runtimeConfig`. Hot-path access to `runtimeConfig` fields is single-cache-line; hot-path access to the proto would chase pointer indirections. This is a 07.1-precedent micro-optimization (cors does the dehydration at per-route config time per `internal/filter/http/cors/perroute_test.go`).
- *Why `int` for `abortHTTPStatus` not `proto.Int32`?* The proto field is `uint32`; the Go-side representation is `int` for natural use with `http.StatusServiceUnavailable`-style constants and `cb.SendLocalReply(status int, …)` signature.

**Deferred to SPEC:** the exact behavior when `abort.http_status` is set to 0 or to an out-of-range value (negative, > 999). Empirical pin: scrape Envoy v1.37.2's behavior on `http_status: 0` (likely 503-default per Envoy docs) and `http_status: 9999` (likely a config-load error; Envoy refuses the config). See §11.1.

**ADR anchor:** ADR-0101 — `runtimeConfig` shape + 7-field consumed / 4-field silent-ignore decomposition + the unmarshal-at-New discipline.

### 2.4 Delay async-resume mechanics *(Decision 4 → ADR-0102)*

**Decision:** When `delayEnabled` AND the percentage roll succeeds AND the headers-field match succeeds (per §2.7), `DecodeHeaders` does:

```go
// Pseudo-Go; final shape is the SPEC author's call.
delay := f.cfg.delayFixedDelay
if hdrDelay, ok := parseHeaderDelay(headers); ok && f.cfg.delayEnabled {
    delay = hdrDelay
}
f.activeFaults.Add(1)  // counter increment for max_active_faults
f.delayTimer = time.AfterFunc(delay, func() {
    f.delayCompleted.Store(true)
    if f.shouldAbortAfterDelay() {
        f.cb.SendLocalReply(f.abortStatus, faultAbortBody, nil)
    } else {
        f.cb.ContinueDecoding()
    }
    f.activeFaults.Add(-1)
})
return envoyhttp.StopIteration
```

The `time.AfterFunc` semantics: `AfterFunc(d, fn)` returns a `*time.Timer`; if `Stop()` is called before fn fires, fn is not invoked. The filter's `OnDestroy()` calls `f.delayTimer.Stop()` to handle the request-cancellation path (downstream disconnect / timeout / SendLocalReply by another filter). Per ADR-0071 single-goroutine-per-stream invariant, the timer's callback runs on a TIMER goroutine, NOT the stream dispatch goroutine — so `cb.ContinueDecoding` is the framework's parking-resume primitive (its callback-coalescing per `internal/filter/http/callbacks.go:14–17`) which IS goroutine-safe by contract.

**Why this vs. alternatives:**
- *Why `time.AfterFunc` not a `time.Sleep` + goroutine?* `AfterFunc` is the stdlib idiomatic primitive and has clean cancel semantics via `Stop()`. A goroutine + `time.Sleep` cannot be cancelled cleanly without an extra channel.
- *Why not `cb.ContinueDecoding` directly without goroutine?* The chain is parked because `DecodeHeaders` returned `StopIteration`. The dispatch goroutine has unwound past the filter; the only way to wake it is via the callback-resume primitive from a separate goroutine (the timer's). Per ADR-0071 §async-resume.
- *Why not embed `cb.SendLocalReply` in the timer when delay+abort both apply (instead of resuming + having the resumed pass land an abort)?* The simpler shape — timer fires, decides delay-only OR delay-then-abort, calls the right primitive — avoids the resumed-then-aborted-mid-decode race. Phase 09 settles on the timer-decides-everything pattern.

**Deferred to SPEC:** the timing tolerance. Envoy's actual fault delay accuracy is empirically pinned in §11.2 (likely ±10ms based on the OS sleep granularity); the BEHAVIOR_CONTRACT.md `## Timing tolerances` section gets a new fault-filter-specific bullet.

**ADR anchor:** ADR-0102 — Delay async-resume mechanics (timer-driven; cancel on OnDestroy; combined delay+abort decided in timer callback).

### 2.5 Abort terminal-replace mechanics *(Decision 5 → ADR-0103)*

**Decision:** When `abortEnabled` AND the percentage roll succeeds AND the headers-field match succeeds (and delay does NOT apply OR has fired), `DecodeHeaders` (or the delay timer's callback) invokes:

```go
f.cb.SendLocalReply(f.cfg.abortHTTPStatus, "fault filter abort", nil)
return envoyhttp.StopIteration
```

The `SendLocalReply` per ADR-0075 enters the encode iteration at `filter[len-1]`, which is the router. Router's encode-side is the no-op pass-through that lets the synthesized response flow back to the chain's wire-write layer. The `nil` headers parameter is OrderedHeaders-empty per `internal/filter/http/callbacks.go:30` — no caller-injected headers; the wire response carries default content-type (text/plain charset=UTF-8 per Envoy's local-reply default; empirically pinned in §11.3).

The `fault filter abort` body string is Envoy's literal (per Envoy v1.37.2 source-of-truth observation; empirically scraped at SPEC time per §11.4). Per BEHAVIOR_CONTRACT.md `## HTTP filter chain` §`Asserted equivalence` (line 699-707 of the worktree HEAD), response body is "byte-exact for deterministic handlers" — fault's abort response is deterministic, so byte-exact equivalence applies.

**Why this vs. alternatives:**
- *Why not return a custom `FilterHeadersStatus` like `LocalReply`?* The 07.1 framework's status enum (`internal/filter/http/types.go:15–20`) settled at two values: `Continue` and `StopIteration`. ADR-0071 §iteration-protocol explicitly rejects expanding the enum; SendLocalReply is the framework's local-reply primitive, NOT a status enum value.
- *Why not parse the body from the proto (Envoy supports a `body` field on FaultAbort)?* The proto's FaultAbort message in v1.37.2 does NOT have a `body` field — only `http_status`, `grpc_status`, and `header_abort`. Body content is hardcoded by Envoy.

**Deferred to SPEC:** the empirical-pin obligation in §11.4 (scrape Envoy's actual abort body byte-exact + content-type + content-length).

**ADR anchor:** ADR-0103 — Abort terminal-replace mechanics + body byte-exact equivalence with Envoy v1.37.2.

### 2.6 Combined delay+abort ordering *(Decision 6 → ADR-0102 consequence)*

**Decision:** When the request matches BOTH delay AND abort criteria, the delay timer fires FIRST, and on fire the timer's callback calls `SendLocalReply` for the abort (skipping `ContinueDecoding`). The upstream is never dialed; the response is the abort response, but it arrives `delay.fixed_delay` after the request. This is an envoy.io-documented behavior (per `https://www.envoyproxy.io/docs/envoy/v1.37.2/configuration/http/http_filters/fault_filter#runtime` "If both 'delay' and 'abort' are specified, the delay is applied first.").

**Why this vs. alternatives:** This is empirically-fixed by Envoy. No alternative is conformant.

**Deferred to SPEC:** none.

**ADR anchor:** Folded into ADR-0102 (delay async-resume mechanics) — the ordering rule lives in the timer-callback decision branch.

### 2.7 Header-driven fault parsing *(Decision 7 → ADR-0104)*

**Decision:** Per Envoy's documented header-driven-fault path:

- `x-envoy-fault-delay-request: <ms>` — when present AND `delay.percentage > 0`, the value is parsed as integer milliseconds and OVERRIDES `delay.fixed_delay`. If parse fails, the header is silently ignored (envoy.io documents this; conform).
- `x-envoy-fault-delay-request-percentage: <0-100>` — when present, OVERRIDES `delay.percentage` for the percentage-roll evaluation.
- `x-envoy-fault-abort-request: <status>` — when present AND `abort.percentage > 0`, the value is parsed as HTTP status code (100-999) and OVERRIDES `abort.http_status`. Parse failure → silently ignore.
- `x-envoy-fault-abort-request-percentage: <0-100>` — same pattern as delay-percentage override.

Per Envoy v1.37.2 docs, the header-driven path requires the corresponding `*.percentage` field to be set (non-zero), not just the parent `delay`/`abort` message. This brainstorm settles `> 0` as the activation threshold: `delay.percentage = 0` means "no delays even with header", `delay.percentage = 100` means "header drives 100% of requests", intermediate values follow envoy.io's documented runtime-key semantics (which envoy-go silently ignores at filter boot — see §10).

**Why this vs. alternatives:**
- *Why these four header names?* They are the documented Envoy v1.37.2 fault-filter header set per `https://www.envoyproxy.io/docs/envoy/v1.37.2/configuration/http/http_filters/fault_filter`. Conformance dictates we honor them.
- *Why silent-ignore parse failures?* Envoy's behavior per docs ("the header is silently ignored if not parseable").
- *Why not honor the header-only flag (no `percentage` configured)?* Envoy requires the percentage-config precondition. Conform.

**Deferred to SPEC:** the empirical pin in §11.5 — confirm Envoy v1.37.2's behavior on a degenerate case (header set, percentage = 0; header set, percentage = 100). If Envoy diverges from this brainstorm's reading, the SPEC author updates the design + writes a corrective ADR.

**ADR anchor:** ADR-0104 — Header-driven fault enable; four documented headers; silent-ignore parse failures; percentage-precondition discipline.

### 2.8 `max_active_faults` concurrency cap *(Decision 8 → ADR-0105)*

**Decision:** A `sync/atomic.Int64` counter per filter-instance (not per-request — the counter is per-listener-filter-chain entry, since the `New` factory's closure holds it). The counter increments at fault-trigger time (after the percentage roll succeeds; before the timer schedule or the SendLocalReply). It decrements at fault-completion time (timer fires + callback completes; OR abort SendLocalReply completes). When the counter reaches `cfg.maxActiveFaults` at trigger time, the fault is silently SKIPPED (the request passes through normally) and the `fault.faults_overflow` counter increments.

Hot-path is lock-free: the counter is `atomic.Int64` per LBP-1 (per ADR-0091's drain-manager pattern; phase 06.1's stats Registry pattern; ADR-0072's HTTPRegistry pattern — fifth/sixth application of the lock-free-hot-path discipline depending on count). The counter exists ONLY when `maxActiveFaults > 0`; when 0 (proto default = no cap), the counter is not allocated and the cap-check is skipped (single non-zero-`maxActiveFaults` branch on the hot path).

**Why this vs. alternatives:**
- *Why per-instance, not global?* Envoy's docs scope `max_active_faults` to the filter-config instance. Multiple listeners with their own fault filter configs each get their own caps.
- *Why decrement on completion not on trigger?* Faults are TIME-extended (delay) or stream-extended (abort processing). The cap is on ACTIVE faults; activity ends at fault-completion.

**Deferred to SPEC:** none — the counter shape is mechanical.

**ADR anchor:** ADR-0105 — `max_active_faults` concurrency cap + LBP-1 fifth/sixth application + `fault.faults_overflow` stat semantics.

### 2.9 Percentage-roll determinism *(Decision 9 → ADR-0101 consequence)*

**Decision:** The percentage roll uses `crypto/rand`-seeded `math/rand/v2` (per Go 1.22+ idiomatic) with a per-instance `*rand.Rand` value. For 0% the roll is short-circuited to "miss" (no random draw); for 100% the roll is short-circuited to "hit" (no random draw); for intermediate percentages (1–99), the roll is a uniform `[0, 100)` draw compared to `cfg.delayPercentage`.

**Why this vs. alternatives:**
- *Why crypto-seeded math/rand?* Per-instance seed prevents inter-instance correlation. Crypto-seeding (vs `time.Now().UnixNano()`) prevents process-restart correlation across deployments.
- *Why short-circuit 0% and 100%?* Differential-test-time scenarios use 0 and 100 exclusively (per §8 fixture design). Short-circuit avoids the random-draw under test conditions, making test outcomes deterministic.

**Deferred to SPEC:** the differential-fixture reliance on 0%/100% short-circuit. SPEC author confirms fixture scenarios use only 0/100.

**ADR anchor:** Folded into ADR-0101 — the percentage-roll mechanism is a `runtimeConfig` consequence.

### 2.10 Stats emission *(Decision 10 → ADR-0107)*

**Decision:** Phase 09 introduces four new counters under the `fault.*` namespace, registered via the existing `internal/stats` registry (per phase 06.1's framework + ADR-0061):

- `fault.aborts_injected` — incremented each time an abort is fired (whether percentage-, header-, or runtime-driven).
- `fault.delays_injected` — incremented each time a delay is scheduled.
- `fault.active_faults` — gauge tracking the in-flight fault count (mirrors the `max_active_faults` counter).
- `fault.faults_overflow` — incremented each time a fault is SKIPPED due to `max_active_faults` cap.

These four names follow Envoy's documented `http.<stat-prefix>.fault.*` stat tree per `https://www.envoyproxy.io/docs/envoy/v1.37.2/configuration/http/http_filters/fault_filter#statistics`. Envoy's full prefix is `http.<stat-prefix>.fault.<counter>` where `<stat-prefix>` is the HCM's `stat_prefix` config field. Envoy-go's stats registry scopes via `http.<stat-prefix>.` already (per phase 06.1's HCM stats discipline), so the filter-level emit just adds `fault.<counter>` under that scope. The SPEC author confirms via an empirical pin (§11.6) that Envoy's exact stat names match this projection (specifically: that the `fault.` prefix is the right anchor and that no other intervening level — like `<filter-name>.` — exists).

Per ADR-0061 SN1–SN8 flattening rules (per `BEHAVIOR_CONTRACT.md ## Stat-name mapping`), the names project deterministically: `http.ingress_http.fault.aborts_injected` (with `stat_prefix = ingress_http` per the canonical fixture). The Twin-series filter discipline (per `BEHAVIOR_CONTRACT.md ## Stat-name mapping ### Twin-series filter discipline`) does not apply to fault — the `fault.*` prefix is a stable Envoy primitive, not a filter-name-derived prefix.

**Why this vs. alternatives:**
- *Why not skip stats in the MVP?* Envoy emits these counters; differential testing on stats is a pillar of phase 06.1's gate. Phase 09 must emit them for the fixture to be green on stats.
- *Why a gauge for `active_faults` not a counter?* Active-faults is by definition an instantaneous count, not a monotonic accumulation. Envoy emits it as a gauge; envoy-go conforms.

**Deferred to SPEC:** the empirical pin in §11.6 (confirm exact stat names + types via Prometheus scrape).

**ADR anchor:** ADR-0107 — `BEHAVIOR_CONTRACT.md ## Stat-name mapping` extension for the four `fault.*` counters + their Envoy-projection rules.

### 2.11 Per-route config 3-tier merge *(Decision 11 → ADR-0073 reuse)*

**Decision:** Phase 09 reuses the 07.1 framework's 3-tier `typed_per_filter_config` merge (Route > VirtualHost > RouteConfiguration; most-specific override; per ADR-0073). The fault filter's `RequestRouteConfig()` lookup returns a `*envoyextensionsfiltershttpfaultv3.HTTPFault` value (or nil for no per-route override). When non-nil, the per-route `runtimeConfig` overrides the listener-level config WHOLESALE per ADR-0073 (the proto defines per-field merge semantics but ADR-0073's wholesale-override discipline applies for now; per-field-merge is an explicit ADR-0073 deferred-pin).

The wholesale-override semantics map to:
- If per-route `delay` is set, it replaces the listener-level `delay` entirely (including percentage + fixed_delay).
- If per-route `delay` is not set (proto field absent), listener-level `delay` applies unchanged.
- Same pattern for `abort`, `headers`, `max_active_faults`.

**Why this vs. alternatives:** ADR-0073's wholesale-override is the project's settled discipline. Conform.

**Deferred to SPEC:** the SPEC author confirms via an empirical-pin obligation in §11.7 that Envoy v1.37.2's per-route fault config behaves per the wholesale-override discipline (confirm: a route-level fault that omits `delay` does NOT inherit the listener-level `delay`).

**ADR anchor:** Folded into ADR-0073's existing scope (no new ADR).

### 2.12 Family expansion shape *(Decision 12 → ADR-0106 partial)*

**Decision:** Each HTTP-filters-family phase becomes its own top-level row in ROADMAP.md (flat structure). Phase 09 = fault. Phase 10 = next-filter (TBD by next brainstorm). No parent-child split anticipated for the family-as-whole. The §9 `### HTTP filters family` heading at `ROADMAP.md` line 55 is a conceptual umbrella, not a row. Sub-phase splits per ADR-0045 are still permitted within any single filter's phase if its surface blows up — but the cross-filter family does not get a parent row.

**Why this vs. alternatives:**
- *Why flat top-level?* See §1.3 above: a 16-deep parent violates §6 splitting policy at the sub-phase scale; BOOTSTRAP §9 invariant 4's wording supports flat.
- *Why not bundle 2-3 filters per phase ("waves")?* Each filter is its own coherent surface; bundling adds cognitive load + cross-filter coupling without LoC savings. Each filter's tests + fixtures are independent.

**Deferred to SPEC:** none — this is a meta-shape decision.

**ADR anchor:** ADR-0106 — Family expansion shape (HTTP filters, network filters, and by analogy all §9 families) as flat top-level rows.

### 2.13 No sibling SPEC stub *(Decision 13 → ADR-0106 consequence)*

**Decision:** Phase 09 does NOT create a sibling-phase SPEC stub for the next-filter phase. Future family-expansion brainstorms cold-start from the §9 `### HTTP filters family` heading + the just-shipped phase-N filter's artefacts as their context. The 08.1-creates-08.2-stub precedent was a sub-phase-split-within-parent pattern (08.1 stub-authored 08.2 because 08.1's parent SPEC scoped them together); family-expansion has no parent SPEC, so no stub.

**Why this vs. alternatives:** stubs would risk pre-populating implicit per-phase rows (BOOTSTRAP §9 invariant 4 violation in spirit).

**Deferred to SPEC:** none.

**ADR anchor:** Folded into ADR-0106.

---

## 3. Surface inventory (09 only)

This section enumerates every code/doc surface phase 09 touches. The SPEC author uses this list as the cross-cutting concerns map.

### 3.1 New files (created in 09)

- `internal/filter/http/fault/fault.go` — filter implementation (~350–500 LoC est.).
- `internal/filter/http/fault/fault_test.go` — unit tests (~200–350 LoC est.).
- `internal/filter/http/fault/doc.go` — package overview comment (~30 LoC est.).
- `test/fixtures/0011-http-fault/envoy.yaml` — reference Envoy config.
- `test/fixtures/0011-http-fault/envoy-go.yaml` — envoy-go config (initially identical).
- `test/fixtures/0011-http-fault/inputs/driver.go` — Go driver running the 5 scenarios.
- `test/fixtures/0011-http-fault/expectations.yaml` — diff allow-lists + timing tolerances.
- `test/fixtures/0011-http-fault/README.md` — fixture-specific overview.
- `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` — to be authored next session per writing-plans.
- `docs/envoy-go/phases/09-http-filter-fault/PLAN.md` — same.
- `docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md` — same.
- `docs/envoy-go/phases/09-http-filter-fault/REVIEW.md` — at phase-done time.

### 3.2 Modified files (in 09)

- `cmd/envoy-go/main.go` — one new `httpReg.Register(fault.TypeURL, fault.New)` line (per Decision 2). Plus the import.
- `docs/envoy-go/ROADMAP.md` — add phase 09 row under MVP Trunk table.
- `docs/envoy-go/STATE.md` — flip to lifecycle-state-1 next-skill writing-plans.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — add `### envoy.filters.http.fault` subsection under `## HTTP filter chain` (line 695); add four-counter `fault.*` block under `## Stat-name mapping`.
- `docs/envoy-go/DECISIONS.md` — append ADR-0100..ADR-0107 (eight ADRs per §9 below).
- `internal/stats/registry.go` (or wherever) — add four new stat name registrations for the `fault.*` counters per the existing 17-name table in 06.1 — this becomes the 18th–21st entries.

### 3.3 Untouched files (load-bearing absence)

- `internal/filter/http/types.go` — no new iteration-protocol changes (fault uses the existing `StopIteration` + `ContinueDecoding` + `SendLocalReply` primitives).
- `internal/filter/http/callbacks.go` — same; no new callback methods.
- `internal/filter/http/registry.go` — same; the `Register` API is reused.
- `internal/filter/http/chain.go` — same; the chain dispatch is reused unchanged.
- `internal/filter/hcm/` — HCM-side hooks already exist for SendLocalReply / async resume per 07.1 + 07.2; no new HCM wiring.
- `internal/listener/`, `internal/cluster/` — fault is HTTP-only; no listener / cluster wiring.

This deliberate minimality is the brainstorm's core design discipline: phase 09 ships ONE NEW filter via the existing framework; no framework extensions.

---

## 4. Iteration-state coverage map

Phase 07.1 settled the HTTP filter iteration protocol enum (per `internal/filter/http/types.go:15–20`, `:27–34`, `:40–45`):

- `FilterHeadersStatus`: `Continue`, `StopIteration` (2 values).
- `FilterDataStatus`: `DataContinue`, `DataStopIterationAndBuffer`, `DataStopIterationNoBuffer` (3 values).
- `FilterTrailersStatus`: `TrailersContinue`, `TrailersStopIteration` (2 values).

The 07.1 cors filter exercised: `Continue` (DecodeHeaders pass-through and EncodeHeaders mutation) + `StopIteration` (DecodeHeaders for preflight short-circuit via SendLocalReply). Cors did NOT exercise: any data/trailers status, any async-resume (cors's StopIteration was followed immediately by SendLocalReply, no parking).

The 07.1 envoy_go_test probe filter exercised all 7 status enum values structurally.

Phase 09 fault adds:

- **`Continue` (DecodeHeaders)** — passthrough when fault doesn't apply. Already exercised by cors; reinforces.
- **`StopIteration` (DecodeHeaders) WITH async-resume** — delay path. Parks the chain, schedules the timer, resumes via `cb.ContinueDecoding()` from the timer goroutine. NEW exercise; only structurally covered by `envoy_go_test` before now.
- **`StopIteration` (DecodeHeaders) WITH SendLocalReply** — abort path. Same shape as cors's preflight-short-circuit; reinforces.

Phase 09 does NOT exercise:

- `FilterDataStatus` (any value) — fault operates on headers only; no body buffering or pass-through.
- `FilterTrailersStatus` (any value) — fault doesn't touch trailers.
- Encode-side `StopIteration` with parking — fault is decode-side only (the abort response goes through encode but fault doesn't park encode).

This iteration coverage map informs the §9 anticipated-ADR list — fault is the FIRST production exerciser of async-resume on the request side, and ADR-0102 codifies the timer-based mechanics as the project's reference pattern for future async-resume filters (e.g., ext_authz).

---

## 5. Async-resume mechanics in detail

This section deepens Decision 4 (§2.4) into the implementation-level concerns the SPEC author resolves.

### 5.1 Timer scheduling

`time.AfterFunc(delay, callback)` returns a `*time.Timer`. The callback runs on a goroutine spawned by the runtime (no goroutine-pool consumption from envoy-go's perspective). Per Go runtime docs, `AfterFunc` is the lowest-overhead one-shot timer primitive.

Phase 09's filter holds the timer pointer in the per-instance `filter` struct so `OnDestroy` can call `Stop()`. The `Stop()` return value indicates whether the timer was active (true) or already fired (false); envoy-go ignores the return — the activeFaults decrement happens unconditionally in `OnDestroy` if the delay was scheduled but neither completed nor cancelled.

### 5.2 Cancellation semantics

The downstream-disconnect path (downstream client closes the connection mid-fault-delay) triggers HCM stream-reset, which calls the filter chain's `OnDestroy` per filter, which calls `f.delayTimer.Stop()`. The timer either:

- Stopped before fire: fn never runs; activeFaults must be decremented in OnDestroy.
- Already fired (between Stop call and the runtime's check): fn is concurrent with OnDestroy; the activeFaults atomic.Int64 handles the race (the fn does -1; OnDestroy does -1; both are safe operations).

Per ADR-0091's drain-manager precedent (LBP-1 fifth application), the atomic.Int64 ABI is the project's idiomatic counter shape.

### 5.3 ContinueDecoding goroutine-safety

Per `internal/filter/http/callbacks.go:14–17`, `ContinueDecoding` is documented as "safe to call from any goroutine" with "duplicate calls coalesced via the chain's per-stream resume channel (capacity 1, non-blocking send)". The fault filter's timer-callback invocation of `ContinueDecoding` is therefore well-defined.

### 5.4 SendLocalReply from timer goroutine

Per `internal/filter/http/callbacks.go:20–30`, SendLocalReply is "first-call-wins via sync.Once on the chain". The fault filter's timer-callback calling SendLocalReply (when delay+abort both apply) IS valid; the `sync.Once` discipline guarantees no double-reply if a downstream-disconnect-induced parallel reply is in-flight.

The timer callback's SendLocalReply call MUST acquire no per-stream mutex — the chain's reply machinery is internally synchronized. Per ADR-0075's encode-chain semantics + the cors precedent, SendLocalReply is goroutine-safe by contract.

### 5.5 Timer-goroutine quiescence at shutdown

Phase 08.2's drain manager (per `internal/drain/`) waits for in-flight requests to complete before listener stop. Fault's delay timers are in-flight requests (HCM increments inflight when DecodeHeaders is invoked; decrements on terminal response). The drain wait-loop will therefore wait for fault delays to complete before allowing process exit. No new drain-manager wiring is needed — fault inherits the existing drain discipline.

If the drain timeout fires before all fault delays complete, the surviving timers' callbacks fire AFTER the drain manager's exit. Per the drain timeout discipline (per ADR-0095), this is the expected "best-effort" semantics; in-flight fault delays are sacrificed to honor the drain timeout. The fault filter's `OnDestroy` is called by the chain teardown path on drain-completion; the timers' Stop() prevents post-exit callbacks.

---

## 6. Stats specification

This section deepens Decision 10 (§2.10) into the registration-level concerns.

### 6.1 Counter names + types

| Counter name | Type | Increment site | Decrement site |
|---|---|---|---|
| `fault.aborts_injected` | Counter | abort fires (SendLocalReply call) | (counter only) |
| `fault.delays_injected` | Counter | delay scheduled (time.AfterFunc) | (counter only) |
| `fault.active_faults` | Gauge | fault triggered | fault completed |
| `fault.faults_overflow` | Counter | trigger denied at cap | (counter only) |

The HCM stat-prefix discipline (per phase 06.1) prepends `http.<stat-prefix>.` to the filter-emitted stat name. The four counters become e.g. `http.ingress_http.fault.aborts_injected` for the canonical fixture.

### 6.2 Registration surface

Phase 06.1's stats Registry (per `internal/stats/registry.go`, ADR-0061) currently registers 17 named stat call sites (per `BEHAVIOR_CONTRACT.md ## Stat-name mapping ### 17-name table` line 130). Phase 09 adds 4 new entries:

- 18: `fault.aborts_injected` (counter)
- 19: `fault.delays_injected` (counter)
- 20: `fault.active_faults` (gauge)
- 21: `fault.faults_overflow` (counter)

The SPEC author confirms the 17-name → 21-name extension via empirical pin §11.6.

### 6.3 Differential-fixture stat assertions

The fixture 0011's `expectations.yaml` asserts post-test stat values:

- Scenario 1 (delay): `delays_injected = 1`, `active_faults = 0` (final), `aborts_injected = 0`, `faults_overflow = 0`.
- Scenario 2 (abort): `aborts_injected = 1`, `delays_injected = 0`, `faults_overflow = 0`.
- Scenario 3 (header-match-only abort, with two requests, one with header, one without): `aborts_injected = 1` (only the matching one).
- Scenario 4 (header-driven abort): `aborts_injected = 1`.
- Scenario 5 (passthrough): all counters zero.

These assertions are exact-equality per the SN1–SN8 deterministic-flow rules.

---

## 7. Per-route config 3-tier merge — fault-filter specifics

This section deepens Decision 11 (§2.11). The 07.1 framework's 3-tier merge (per ADR-0073) applies wholesale-override at each tier. For fault, the wholesale-override semantics map to:

### 7.1 Route-level override

```yaml
route_config:
  virtual_hosts:
    - name: vh1
      routes:
        - match: { prefix: "/abort" }
          route: { cluster: backend }
          typed_per_filter_config:
            envoy.filters.http.fault:
              "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
              abort:
                percentage: { numerator: 100 }
                http_status: 418
```

When a request matches the `/abort` route, the per-route fault config WHOLESALE replaces the listener-level fault config. Even if the listener-level config had a `delay`, the per-route config (with no `delay` field set) means delay does NOT apply — wholesale-override discipline.

### 7.2 VirtualHost-level override

Same wholesale semantics; less specific than route-level.

### 7.3 RouteConfiguration-level override

Same wholesale semantics; least specific.

### 7.4 Listener-level (HTTP filter chain default)

The base case. If no `typed_per_filter_config` matches at any of the three tiers, the listener-level config from the http_filters[].typed_config applies.

### 7.5 Empirical pin

§11.7 defers to SPEC time the empirical-pin obligation: confirm Envoy v1.37.2's per-route fault config wholesale-override discipline matches phase 09's reading.

---

## 8. Differential fixture 0011-http-fault — scenarios + driver shape

This section deepens §1.1 item 9 into the fixture-level concerns.

### 8.1 Fixture topology

Single listener (`listener_0`); single backend cluster (`backend`); two routes:

- Route 1 (`prefix: /delay`): plain pass-through to backend; listener-level fault config has 100% delay 100ms, 0% abort.
- Route 2 (`prefix: /abort`): per-route fault override (100% abort 503).
- Default route: passthrough.

### 8.2 Scenarios

| # | Request | Listener fault | Per-route fault | Expected behavior |
|---|---|---|---|---|
| 1 | `GET /delay/foo` | delay 100ms 100% | none | 200 OK from backend; latency ≥ 100ms (within ±10ms tolerance). `delays_injected = 1`. |
| 2 | `GET /abort/bar` | delay 100ms 100% | abort 503 100% (wholesale override = no delay) | 503 with body `fault filter abort`; backend never sees request. `aborts_injected = 1`, `delays_injected = 0`. |
| 3 | `GET /` headers `x-test-fault: 1` (filter `headers` field requires this) | delay+abort match-headers config | none | abort fires (the headers field gates fault enable); 503 + body. |
| 3a | `GET /` no header (companion to 3) | same listener | none | passthrough; no fault. |
| 4 | `GET /` headers `x-envoy-fault-abort-request: 503` | listener config has abort.percentage=100 with http_status=200 (so the header overrides the configured status) | none | 503 from header override. |
| 5 | `GET /` (passthrough scenario) | no fault filter at all on this route | none | passthrough; counters all zero. |

(Scenario 3 + 3a are paired in a single test request volley.)

### 8.3 Driver shape

`inputs/driver.go` is a simple Go test harness running 6 HTTP requests against both the reference Envoy and envoy-go subjects via the differential harness's `RunFixture(t, "0011-http-fault")` entry point. Each scenario asserts:

- Response status (exact equality).
- Response body (exact equality for the abort case; tolerant for the delay case which has a backend-driven body).
- Response latency (within the timing-tolerance pin per §11.2).
- Stats snapshot delta (exact equality per the SN1–SN8 deterministic-flow rules).

### 8.4 Header-allow-list extensions

The differential harness's allow-list (per `BEHAVIOR_CONTRACT.md ## Header allow-list`) covers `server`, `date`, and timing/identity headers. Fault's abort response carries a `content-type: text/plain` header (empirically pinned in §11.3); this is a NEW header on the abort path and must be confirmed by the SPEC author as a deterministic emit (vs. allow-listed).

### 8.5 Timing tolerance

Per `BEHAVIOR_CONTRACT.md ## Timing tolerances` — currently scoped per ADR-0091's drain timeout. Phase 09 adds a fault-specific bullet: "Fault filter delay accuracy ± 10ms (OS timer-granularity)." Empirical pin §11.2 confirms.

---

## 9. Anticipated ADRs (ADR-0100 through ADR-0107)

Phase 09's ADR-numbering anchor is **ADR-0100** (next-free; 08.2 closed at ADR-0099 per `DECISIONS.md` line 4088). The expected eight ADRs:

| ADR | Title (anticipated) | Settles |
|---|---|---|
| ADR-0100 | `internal/filter/http/fault/` package shape + extension-registry registration line | Decisions 1, 2 |
| ADR-0101 | `runtimeConfig` shape + 7-field consumed / 4-field silent-ignore decomposition + percentage-roll determinism | Decisions 3, 9 |
| ADR-0102 | Delay async-resume mechanics — timer-based scheduling + cancel-on-OnDestroy + delay+abort timer-callback decision | Decisions 4, 6 |
| ADR-0103 | Abort terminal-replace mechanics + body byte-exact equivalence with Envoy v1.37.2 | Decision 5 |
| ADR-0104 | Header-driven fault enable — four documented headers + silent-ignore parse failures + percentage-precondition discipline | Decision 7 |
| ADR-0105 | `max_active_faults` concurrency cap + LBP-1 sixth application + `fault.faults_overflow` stat semantics | Decision 8 |
| ADR-0106 | Family expansion shape — flat top-level rows for §9 family-children + no-sibling-stub discipline | Decisions 12, 13 |
| ADR-0107 | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 17→21-name extension for the four `fault.*` counters | Decision 10 |

The SPEC author writes these ADRs in the SPEC drafting session (lifecycle-state 1 → 2). Each ADR cites this brainstorm's Decision-block as the source-of-record.

---

## 10. Out-of-scope deferrals (each gets a deferral ADR per ADR-0040 format)

This section enumerates the Envoy v1.37.2 fault-filter surface that phase 09 explicitly does NOT ship. Each deferral has a forward-pointer to the family / phase that should own it.

### 10.1 `response_rate_limit` (FaultRateLimit)

The proto's `response_rate_limit` field (type `envoy.extensions.filters.common.fault.v3.FaultRateLimit`) configures bandwidth-throttling on the response body. This is a separate subsystem (token-bucket + body-chunk delay scheduling); orthogonal to delay/abort.

**Forward pointer:** A future fault-filter-internal phase ("phase NN — http-filter-fault-rate-limit") OR the bandwidth-limit filter (`envoy.filters.http.bandwidth_limit`) which subsumes the same primitive. ADR at SPEC time documents the deferral + forward pointer.

### 10.2 `abort.grpc_status`

The proto's `abort.grpc_status` field configures gRPC-statuscode abort instead of HTTP-statuscode. Requires gRPC framing + grpc-status header + grpc-message header.

**Forward pointer:** §9 gRPC family — specifically the gRPC-bridge or gRPC-aware-filters phase that lands gRPC primitives in envoy-go. Until then, fault's gRPC abort is silently NOT honored (proto field parsed but value ignored at New). Deferral ADR per ADR-0040 format.

### 10.3 `delay.header_delay`

The proto's `delay.header_delay` (vs. `delay.fixed_delay`) is a complement to the `x-envoy-fault-delay-request` header path — it allows the listener config to declare "use a header for the delay value" without setting fixed_delay. Phase 09 honors the request-header path BUT NOT the proto-field-driven header path.

**Forward pointer:** A small phase NN follow-up (~50 LoC) to add `delay.header_delay` proto-field handling. Likely landed alongside other small fault-extension work. Deferral ADR per ADR-0040 format.

### 10.4 `upstream_cluster` filter

The proto's `upstream_cluster` (string) filters fault application to requests routed to a specific upstream cluster.

**Forward pointer:** A small phase NN follow-up. Deferral ADR per ADR-0040 format.

### 10.5 `downstream_nodes` filter

The proto's `downstream_nodes` (repeated string) filters fault application to requests originating from specific downstream node IDs.

**Forward pointer:** A small phase NN follow-up. Deferral ADR per ADR-0040 format.

### 10.6 Runtime-key fields

The proto's `delay_percent_runtime`, `abort_percent_runtime`, `max_active_faults_runtime`, `response_rate_limit_percent_runtime` fields all defer to the runtime layer (RTDS). Envoy-go has no runtime layer (per `BOOTSTRAP_PROMPT.md` §9 "Runtime + hot restart family"). Phase 09 silently parses these proto fields but ignores their values; the static `delay.percentage` etc. fields are the source of truth.

**Forward pointer:** §9 Runtime + hot restart family. Deferral ADR per ADR-0040 format covers all four runtime-key fields together.

### 10.7 `disable_downstream_cluster_stats`

The proto's `disable_downstream_cluster_stats` (bool) suppresses per-downstream-cluster stat emission. Phase 09 ignores; envoy-go's stats discipline emits on the namespace it scopes to, no per-downstream-cluster split.

**Forward pointer:** A small phase NN follow-up if/when per-downstream-cluster stat fan-out is added. Deferral ADR.

### 10.8 `filter_enabled` / `filter_enabled_runtime`

The proto's `filter_enabled` (RuntimeFractionalPercent) gates fault on a runtime-key. Phase 09 ignores (filter is always enabled when registered).

**Forward pointer:** §9 Runtime + hot restart family. Deferral ADR per the runtime-key block.

### 10.9 `headers` field — RBAC-style sub-matching

Phase 09 honors `headers` for SIMPLE name+value matching (and prefix-match if Envoy-faithful). Complex match expressions (regex, range, present-only) are limited per the §11.5 empirical pin's reading; full RBAC-grade match parsing is deferred.

**Forward pointer:** Whatever phase lands the full `envoy.config.route.v3.HeaderMatcher` parsing engine (used by RBAC + JWT + others). Deferral ADR.

### 10.10 `max_active_faults` runtime-key (`max_active_faults_runtime`)

Covered by §10.6. Listed separately for completeness.

---

## 11. Empirical-pin obligations (deferred to SPEC time)

Per ADR-0004 (autonomous brainstorm doctrine), brainstorm-time decisions are settled against on-disk artefacts; empirical evidence requiring scrape against reference Envoy is deferred to the SPEC author's session. This section enumerates the §11 obligations the 09 SPEC author MUST resolve.

### 11.1 `abort.http_status` edge cases

- What does Envoy v1.37.2 do when `abort.http_status: 0` (proto default)? Hypothesis: 503 default, OR config-load error.
- What does Envoy v1.37.2 do when `abort.http_status: 9999` (out of valid range)? Hypothesis: config-load error.

**Scrape command:** `docker run -v $(pwd)/test/fixtures/0011-http-fault/envoy.yaml:/etc/envoy.yaml envoyproxy/envoy:v1.37.2 -c /etc/envoy.yaml --component-log-level config:debug,upstream:debug` with each edge-case config; observe response or error.

### 11.2 Delay timing accuracy

What is the delay accuracy floor for Envoy v1.37.2's fault filter? Hypothesis: ±10ms based on standard-Linux timer granularity (TSC-driven ~1ms clock + scheduler quantum).

**Scrape command:** Run scenario 1 with delays of 50ms, 100ms, 200ms, 500ms; measure end-to-end latency from request-send to response-received; compute the delta from configured delay; the maximum delta is the tolerance.

**BEHAVIOR_CONTRACT.md target:** Add bullet to `## Timing tolerances` for fault delay accuracy.

### 11.3 Abort response shape

- Body: hypothesized `"fault filter abort"`. Confirm byte-exact.
- Content-Type header: hypothesized `text/plain` (Envoy local-reply default).
- Content-Length header: hypothesized = `len(body)`.
- Other headers: empirically captured.

**Scrape command:** `curl -i http://localhost:10000/abort/test` against a reference Envoy with abort 503 100% configured; capture full headers + body verbatim.

### 11.4 Abort body string verification

Same as §11.3, isolating the body string.

### 11.5 Header-driven fault edge cases

- `x-envoy-fault-delay-request: -1` — Envoy behavior?
- `x-envoy-fault-delay-request: abc` — silent-ignore confirmed?
- `x-envoy-fault-delay-request: 999999` (very large) — Envoy behavior?
- `x-envoy-fault-abort-request: 999` (out of HTTP-status range) — Envoy behavior?
- `x-envoy-fault-delay-request-percentage: 50` (with config delay.percentage=10) — does the header override or compose? Hypothesis: override.

**Scrape commands:** Run each edge case via curl; capture responses + log-output.

### 11.6 Stat-name verification

- Confirm Envoy v1.37.2 emits exactly `http.<stat-prefix>.fault.aborts_injected` (and three siblings) — no other intervening level.
- Confirm `active_faults` is gauge type (not counter).

**Scrape command:** `curl http://localhost:9901/stats?prefix=http.ingress_http.fault | grep fault` against a reference Envoy with the fault filter active.

### 11.7 Per-route config wholesale-override

Confirm: a route-level fault config that omits `delay` does NOT inherit the listener-level `delay`.

**Scrape command:** Configure listener-level fault with delay=100ms 100%. Configure route-level fault with abort 503 100% (no delay field). Send a request matching the route; observe: does it delay (would mean MERGE-style) or NOT delay (would mean WHOLESALE-OVERRIDE)? Hypothesis: NOT delay (wholesale override per ADR-0073).

### 11.8 `headers` field match semantics

Envoy's `headers` field uses `envoy.config.route.v3.HeaderMatcher` (the same matcher type used by RBAC routes). What subset does fault honor? Hypothesis: exact-match name+value via the simplest matcher path. Confirm.

**Scrape command:** Configure fault with `headers: [{name: x-test, exact_match: foo}]`; send request with header `x-test: foo` (should fault); send with `x-test: bar` (should not fault); confirm the binary outcome.

---

## 12. Phase-done definition + exit gate (09)

This section defines what "phase 09 is done" means, mirroring the BOOTSTRAP §7.5 phase-done gate.

### 12.1 Phase-done gate (per BOOTSTRAP §7.5)

(a) Differential fixture `0011-http-fault` is GREEN — all 5 scenarios pass.
(b) All pre-existing differential fixtures (`0001`..`0010`) remain green — no regressions.
(c) No new conformance suite required (fault is HTTP-1.1 + HTTP-2; the existing h2spec gate covers H2 framing per phase 05.1; no h3spec/grpc requirement).
(d) New fuzzer (if any) ran clean — phase 09 may add `FuzzFaultConfigParse` for proto-config parsing; SPEC author decides.
(e) `go vet`, `golangci-lint run`, `go test ./...` are all clean.
(f) `REVIEW.md` is approved.

### 12.2 Phase-done commit message format (per BOOTSTRAP §5.3)

```
phase 09: http-filter-fault [ADR-0100, ADR-0101, ADR-0102, ADR-0103, ADR-0104, ADR-0105, ADR-0106, ADR-0107]

New `internal/filter/http/fault/` package implementing `envoy.filters.http.fault` per Envoy v1.37.2; first concrete phase under §9 HTTP filters family; differential fixture `0011-http-fault` green; eight new ADRs ADR-0100..ADR-0107.

Differential surface: `0011-http-fault` newly green; all `0001`..`0010` still green.
Conformance: existing h2spec gate (53/53 PASS at the ADR-0051 pin) still green.
```

### 12.3 ROADMAP row at phase-done

Phase-done flips ROADMAP row 09's status `in-progress → done`. This is the FIRST row to land under the §9 HTTP filters family. The §9 family heading at `ROADMAP.md` line 55 stays unchanged (headings are not rows; their state is implicit).

### 12.4 STATE.md at phase-done

`active-phase: awaiting next planning` (per the 08.2 closure pattern); `next-skill: superpowers:brainstorming` for the next §9 HTTP-filters-family phase (or any other §9 family). The `next-skill-scope` field describes the next family-expansion brainstorm: pick the next filter from the §9 HTTP filters list (or pivot to another §9 family) and run the family-expansion brainstorm pattern.

### 12.5 Anticipated successor

The brainstorm leaves the next phase TBD. Reasonable candidates:

- Phase 10 = `envoy.filters.http.header_mutation` — second-most-stateless filter; trivial async-resume not exercised; reinforces the family-expansion pattern with minimal new ground.
- Phase 10 = `envoy.filters.http.buffer` — next filter exercising body iteration states; pushes the framework into `FilterDataStatus` territory.
- Phase 10 = `envoy.filters.http.local_ratelimit` — token-bucket primitive; first stateful filter (state shared across requests via the bucket).

The 10 brainstorm picks among these.

---

## End of phase 09 brainstorm.

This document is committed at the `phase-09-http-filter-fault-brainstorm` worktree branch's first commit, then squash-merged to master per the `superpowers:wt-merge` pattern. The next session reads this brainstorm + STATE.md and runs `superpowers:writing-plans` to author `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` per the §1.1 in-scope envelope, the §2 design decisions, the §9 anticipated-ADR list, and the §11 empirical-pin obligations.
