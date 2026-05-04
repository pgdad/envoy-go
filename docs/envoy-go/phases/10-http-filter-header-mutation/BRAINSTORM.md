# Phase 10 Brainstorm — `envoy.filters.http.header_mutation`

**Status:** brainstorm complete. This document captures the design decisions reached during the lifecycle-state-0 → 2 brainstorm session for phase 10 (`http-filter-header-mutation`), the THIRD concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1 and `fault` at phase 09). The next session (lifecycle-state 2 → 3 for phase 10, skill `superpowers:writing-plans` per ADR-0005, but routed through the SPEC-authoring step first per the phase 09 precedent) authors `docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §9 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Brainstorm session:** worktree `.worktrees/phase-10-http-filter-header-mutation-brainstorm`, branch `phase-10-http-filter-header-mutation-brainstorm`, branched from master tip `3066c72` (the 09 phase-done REVIEW commit `phase 09: REVIEW — end-of-phase retrospective + N-1 carry-forward`). The 09 follow-up SHA-fill commit `595c4cb` is the prior tip; `3066c72` is the REVIEW-landing commit.

**Brainstorm mode:** interactive with a live human (the user picked filter selection + MVP scope envelope via two-question dialogue: filter = `header_mutation`, MVP scope envelope = the in-scope/deferred lists settled in §1.1 / §8 below). The §9 family selection is implicit — phase 09 set the precedent that subsequent §9 family-rows continue under the umbrella per ADR-0106. Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0107), and the just-shipped phase 09 + phase 07.1 artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §9 and deferred to SPEC-drafting time per the phase 09 + 08.2 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md` section-for-section, reframed for a single-filter scope and adapted for the smaller surface area (no async-resume; no concurrency cap; no stats; new wrinkle is multi-tier per-route evaluation). Sections §§1–11 are decision-bearing prose; §9 enumerates the empirical-pin obligations the 10 SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

---

## 1. Mission and scope confirmation (10 only)

ROADMAP row `10 | http-filter-header-mutation | 09 | planned | | …` (added by this brainstorm, see §10 below) is the row this brainstorm registers as `planned`. Phase 10 is the THIRD concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading (the family heading at `ROADMAP.md` line 56 — `### HTTP filters family` — is a conceptual umbrella, not a row, per ADR-0106). The phase 09 phase-done commit `c7de495` (with follow-up `595c4cb` for SHA fill, REVIEW at `3066c72`) is this row's `depends-on` anchor.

The HTTP filters family lists 16 candidate filters (`ROADMAP.md` line 58): header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. `cors` shipped in phase 07.1 (`internal/filter/http/cors/` per ADR-0074); `fault` shipped in phase 09 (`internal/filter/http/fault/` per ADR-0100). Phase 10 ships `header_mutation` as the THIRD real filter — the canonical Envoy-style "header-rewrite primitive" — and establishes the per-filter-phase pattern's third data point.

### 1.1 What 10 delivers as a self-contained whole

Phase 10 lands `envoy.filters.http.header_mutation` (the canonical Envoy header-rewrite filter) under the 07.1 framework. Eight in-scope items:

1. **New `internal/filter/http/header_mutation/` package** owning the filter implementation. The package mirrors the `internal/filter/http/fault/` shape: `header_mutation.go` (filter type + factory + decode/encode methods), `header_mutation_test.go` (unit tests), `doc.go` (package overview). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation"`) + `New` (the `HTTPFilterFactory`) per the cors/fault precedent.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering `router.New`, `cors.New`, `envoygotest.New`, `fault.New` at lines 112–115) gains a fifth `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` call before the `httpReg.Freeze()` invocation (line 116). Insertion alphabetical-after-router per the ADR-0100 §2.2 convention: `router → cors → envoy_go_test → fault → header_mutation → Freeze`.

3. **Proto-config parsing** of `envoy.extensions.filters.http.header_mutation.v3.HeaderMutation`, the canonical filter-level config message. Per `go-control-plane`'s v1.32.4 module (proto pin via ADR-0008 → Envoy v1.37.2 → proto v3), the message has 2 top-level fields (`mutations`, `most_specific_header_mutations_wins`); the nested `Mutations` triple has 3 fields (`request_mutations`, `response_mutations`, `query_parameter_mutations`). Phase 10 consumes 4 (`mutations.request_mutations[]`, `mutations.response_mutations[]`, `most_specific_header_mutations_wins`, `HeaderMutationPerRoute.mutations`) and explicitly silently-ignores 1 (`mutations.query_parameter_mutations[]` — see §8.1 for the deferral discipline).

4. **Request-mutation execution in `DecodeHeaders`.** The filter iterates the resolved request-mutations list (filter-level prepended; per-route tiers appended in flag-controlled order, see §5) and applies each `HeaderMutation` primitive (the Envoy `config/common/mutation_rules/v3` oneof of `Remove` | `Append`) to the request headers. Returns `Continue` after the loop. NO async-resume primitive is exercised — this filter is fully synchronous.

5. **Response-mutation execution in `EncodeHeaders`.** The filter iterates the resolved response-mutations list against response headers using the same algorithm. Returns `Continue`. The encode-side coverage closes a 07.1 framework gap: cors only short-circuits via SendLocalReply on preflight + injects response headers on non-preflight (no encode iteration); fault never reaches encode (delay is decode-side; abort is terminal). Phase 10 is the FIRST production filter to exercise the encode-side iteration on the non-error path with state mutation.

6. **AppendAction × 4 + `keep_empty_value` semantics** per the `config/common/mutation_rules/v3.HeaderMutation` proto. The four `AppendAction` enum values (`APPEND_IF_EXISTS_OR_ADD` (default), `ADD_IF_ABSENT`, `OVERWRITE_IF_EXISTS_OR_ADD`, `OVERWRITE_IF_EXISTS`) map to natural Go `http.Header` operations (`Add`, conditional `Add`, `Set`, conditional `Set`). The `keep_empty_value` flag (on `HeaderValueOption.keep_empty_value`) controls whether an empty-string value is materialized or silently skipped — empirical-pinned in §9.P2.

7. **Multi-tier per-route resolution — NOT most-specific override.** The proto field `HeaderMutation.most_specific_header_mutations_wins` (with the proto comment at `header_mutation.pb.go:149–154`: "If per route HeaderMutationPerRoute config is configured at multiple route levels, header mutations at all specified levels are evaluated") REQUIRES the filter to evaluate per-route configs at ALL THREE tiers (Route, VirtualHost, RouteConfiguration), not just the most-specific one. This is **structurally different** from the 07.1 framework's `PerRouteConfig.Resolve` (per `internal/filter/http/perroute.go:103–128`) which returns the MOST-SPECIFIC scope's config and discards the others (per ADR-0073: "No field-merge"). Phase 10 therefore requires a small framework extension to expose per-tier configs unmerged. See §5 for the design + ADR-0110 anchor. This is the phase's primary novel piece.

8. **`HeaderMutationPerRoute` + `most_specific_header_mutations_wins` ordering.** The filter applies (a) listener-level mutations FIRST (always, per the proto comment at `header_mutation.pb.go:141–142`: "The mutation rules in the filter configuration will always be applied first and then the per-route mutation rules"), then (b) per-route tier mutations in flag-controlled order: with `most_specific_header_mutations_wins=false` (default) the order is most-specific-first → least-specific-last (so least-specific "wins" overlap by virtue of being applied last); with `=true` the order is reversed (least-specific-first → most-specific-last; most-specific wins).

Plus three artifact-level deliverables:

9. **Differential fixture `0012-http-header-mutation`** under `test/fixtures/0012-http-header-mutation/`: `envoy.yaml` + `envoy-go.yaml` + a Go driver in `inputs/driver.go` exercising five scenarios (per §6.2 below). The fixture asserts response headers (under the BEHAVIOR_CONTRACT.md HV-allow-list discipline), request headers (verified via the upstream echo route surfacing request headers in the response body), and per-route-tier interaction across `most_specific=true` and `most_specific=false` configurations. NO timing assertions (synchronous filter).

10. **`BEHAVIOR_CONTRACT.md` extension** under the existing `## HTTP filter chain` umbrella (alongside the existing `### envoy.filters.http.fault` subsection landed in phase 09): a NEW `### envoy.filters.http.header_mutation` subsection covering AppendAction × 4 semantics, `keep_empty_value` behavior, multi-tier per-route evaluation, and the protected-header set (the §9.P1 empirical pin). Plus (if the §9.P3 stats pin lands a no-emit verdict) a `## Stat-name mapping` entry codifying `header_mutation.*` as the EMPTY namespace — analogous to how `cors` has no stats per ADR-0074.

11. **Anticipated ~7 ADRs (ADR-0108 through ADR-0114)** per §7 below. ADR-0107 is the highest-numbered ADR landed in phase 09 (per `DECISIONS.md` last `## ADR-` heading at line 4243); ADR-0108 is the next-free.

### 1.2 What 10 does NOT deliver (forward to §8)

The exhaustive deferral list lives in §8. The summary: `mutations.query_parameter_mutations[]` (the `KeyValueMutation` triple acting on path-query) is out-of-scope; formatter-substitution syntax in header values (`%REQ(:path)%`, `%DOWNSTREAM_REMOTE_ADDRESS%`, `%RESPONSE_CODE%`, etc — Envoy's full command-string subsystem) is out-of-scope. None are blockers for closing row 10 phase-done; each gets a deferral ADR per the ADR-0040 deferral-ADR format.

### 1.3 Phase-done as a §9 family-row landing

Phase 10's phase-done commit closes ROADMAP row `10` (single-row, no parent-child split anticipated; see §1.4). It does NOT close any §9 family heading (family headings are not rows per ADR-0106) — the HTTP filters family stays "in-progress" implicitly until the last filter under the family ships, but no row tracks that aggregate. Phase 10 is the THIRD §9 family-row to land (after 07.1-cors and 09-fault). The next §9 family-row will be numbered `11` per the flat-row discipline of ADR-0106. The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing.

### 1.4 ADR-0045 split-by-surface readiness

The brainstorm's POSITION is that phase 10 is **single-row at brainstorm time** — a cohesive ~400–600 LoC slice covering a single filter — but the planner-time release valve stays available. If the SPEC author finds the surface > 1500 LoC estimated or the PLAN > 25 tasks, the natural split is:

- **10.1 = framework extension (`PerRouteConfig.ResolveAllTiers`) + listener-only filter MVP**: the framework piece (~150 LoC) + listener-level request/response mutations + AppendAction × 4 + `keep_empty_value` + protected-header set. Differential fixture covers listener-only scenarios.
- **10.2 = per-route + `most_specific_header_mutations_wins`**: per-route 3-tier handling + the precedence-flag ordering + 3 additional fixture scenarios.

This split mirrors 09's anticipated-but-unused split and the 08.1 (admin-endpoints) + 08.2 (graceful-drain) shape. The brainstorm does NOT pre-commit to the split; that's the SPEC author's call. The single-row position is supported by the modest LoC estimate (~400–600 impl + ~150 framework = ~550–750 total) and modest task count estimate (~12–15 tasks).

### 1.5 Seed-stub alignment

Like phase 09, phase 10 has NO sibling SPEC stub — phase 10 enters fresh after the phase 09 close. The §9 family-children list at ROADMAP line 58 enumerates the conceptual surface; the ROADMAP rows enumerate only filters currently in-progress or done. Per ADR-0106(b) (no-sibling-stub discipline), this brainstorm does NOT pre-author SPEC stubs for siblings (`compression`, `local_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `csrf`, `buffer`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

This section is the brainstorm's decision log. Each Decision states **what** is chosen, **why** that option vs. its alternatives, what **deferred-pin** obligations (if any) remain for SPEC-time empirical work, and what **ADR anchor** the SPEC author should expect. ADR numbering starts at **ADR-0108** (next-free; phase 09 closed at ADR-0107 per `DECISIONS.md` line 4243).

### 2.1 Filter package layout *(Decision 1 → ADR-0108)*

**Decision:** New package `internal/filter/http/header_mutation/` with files mirroring the cors + fault precedent: `header_mutation.go`, `header_mutation_test.go`, `doc.go`. The package exports two top-level symbols: `TypeURL` (string constant, `"type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation"`) and `New` (the `HTTPFilterFactory`). All other types (`filter`, `runtimeConfig`, `compiledMutationOp`, `mutationOpKind`) are unexported.

**Why this vs. alternatives:**
- *Why not a single `internal/filter/http/header_mutation.go` flat file?* The existing per-filter discipline is unanimous (cors, fault, router, envoygotest each get their own subpackage per `internal/filter/http/`). Subpackage isolation prevents future name collisions and is the project's convention.
- *Why not the Envoy-source-style path `internal/extensions/filters/http/header_mutation/`?* envoy-go is explicitly NOT mirroring Envoy's C++ source structure (`MISSION.md` §2.2 non-purpose). The `internal/filter/http/<name>/` pattern is the project's own convention.

**Deferred to SPEC:** the exact file split between `header_mutation.go` and any helper files (e.g. whether to factor `compiledMutationOp` into its own file `op.go`) — the SPEC author chooses based on test readability. No ADR-class commitment from brainstorm.

**ADR anchor:** ADR-0108 — Filter package shape conformance with cors + fault precedent.

### 2.2 Extension-registry registration *(Decision 2 → ADR-0108 consequence)*

**Decision:** `cmd/envoy-go/main.go` adds a single new line `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` between the existing `fault.New` registration (currently line 115) and the `httpReg.Freeze()` call (currently line 116). The registration ordering is alphabetical-after-router per the ADR-0100 §2.2 convention codified at phase-09 brainstorm time: `router (first) → cors → envoy_go_test → fault → header_mutation`. Per ADR-0072, registration ordering does not affect runtime behavior; this is a stylistic discipline only.

**Why this vs. alternatives:**
- *Why not registration-order = config-list-order?* Registration order is a global discipline; config-list order is per-listener / per-route. Decoupling avoids cross-cutting coupling (already settled at phase-09 brainstorm time).
- *Why router-first-then-alphabetical?* Router is the canonical terminal filter (the 07.1 chain validation per `internal/filter/http/chain_shape.go:33` requires router as the last entry); stylistic asymmetry codifies that load-bearing role.

**Deferred to SPEC:** none — the line edit is mechanical.

**ADR anchor:** ADR-0108 — folded into the package-shape ADR (the registration line is a one-line consequence of the package shape).

### 2.3 Proto-config parsing + `runtimeConfig` shape *(Decision 3 → ADR-0109)*

**Decision:** The `New` factory unmarshals `tc *anypb.Any` into a `*envoyextensionsfiltershttpheadermutationv3.HeaderMutation` value via `tc.UnmarshalTo(&cfg)`. Per the cors/fault precedent (`internal/filter/http/cors/cors.go:24–27`, `internal/filter/http/fault/fault.go:New`), the unmarshal-and-discard pattern is wrong here — header_mutation has 4 consumed fields. The `New` factory MUST parse the message into a long-lived `*runtimeConfig` value held by the factory closure, capturing:

```go
type runtimeConfig struct {
    requestOps                       []compiledMutationOp // listener-level request mutations (proto-declared order)
    responseOps                      []compiledMutationOp // listener-level response mutations (proto-declared order)
    mostSpecificHeaderMutationsWins  bool                  // precedence-order flag (default false; see §2.10)
}
```

Per-instance state (the `filter` struct returned by `FilterInstanceFactory()`) holds NO config pointer — config is resolved from the closure capture. Per-route config is RESOLVED PER-REQUEST from `cb.RequestRouteConfig()` (or its multi-tier successor `cb.RequestRouteConfigsAllTiers()` per Decision 9 below) and applied AFTER the listener-level pass per the algorithm in §2.10.

**Why this vs. alternatives:**
- *Why not lazy-parse on first DecodeHeaders?* The `New` factory contract per ADR-0072 requires factories to validate `typed_config` shape at registration time so misconfiguration fails at boot, not under traffic. Parsing in DecodeHeaders defers errors to traffic time, which violates that contract.
- *Why not store the proto message directly?* The proto types carry serialization machinery (reflection paths, unknown-fields buffer) that are ~3× larger than the dehydrated `runtimeConfig`. Hot-path access to `runtimeConfig` fields is single-cache-line; hot-path access to the proto would chase pointer indirections. This is the cors/fault-precedent micro-optimization.

**Deferred to SPEC:**
- Whether `runtimeConfig.requestOps` is `[]compiledMutationOp` (value-typed) or `[]*compiledMutationOp` (pointer-typed). The SPEC author chooses based on the iteration pattern; value-typed is preferred for cache locality if the struct is small.
- Whether to flatten the listener + per-route ops into a single resolved list at request time, or iterate them as separate slices in the application loop. The SPEC author chooses based on the `most_specific_header_mutations_wins` algorithm shape.

**ADR anchor:** ADR-0109 — `runtimeConfig` shape + 4-field consumed / 1-field silent-ignore decomposition + the unmarshal-at-New discipline + AppendAction × 4 mapping table + `keep_empty_value` semantics. Largest ADR of the cluster.

### 2.4 `compiledMutationOp` representation *(Decision 4 → ADR-0109 consequence)*

**Decision:** The repeated `HeaderMutation` (the primitive in `config/common/mutation_rules/v3` per `mutation_rules.pb.go:178–254`) is parsed at boot into a flat per-mutation struct:

```go
type mutationOpKind uint8

const (
    kindRemove mutationOpKind = iota
    kindAppend
)

type compiledMutationOp struct {
    kind         mutationOpKind        // kindRemove or kindAppend (proto's HeaderMutation.action oneof)
    headerName   string                 // for both kinds; preserved as configured (Envoy normalizes to lowercase per HTTP/2)
    headerValue  string                 // for kindAppend only ("" for kindRemove)
    appendAction commonv3core.HeaderValueOption_HeaderAppendAction  // 4 variants; for kindAppend only
    keepEmptyValue bool                 // for kindAppend only (proto: HeaderValueOption.keep_empty_value)
}
```

The `HeaderValueOption_HeaderAppendAction` enum is the canonical 4-valued type from `core/v3.HeaderValueOption` (NOT redefined locally — reuse the proto enum directly to avoid drift).

**Why this vs. alternatives:**
- *Why a flat struct vs. a sealed-trait `interface { apply(http.Header) }`?* The flat-struct form is cache-friendly (single allocation per op; tight memory layout); the interface form requires per-op heap allocation + virtual-dispatch on the hot path. For a filter that may apply 5–20 mutations per request, the flat form is measurably faster (microbenchmark exercise deferred to SPEC if relevant).
- *Why preserve the 4 `AppendAction` enum values directly vs. flattening to a 2-bit pair?* Preserving the proto enum keeps the apply-loop's switch statement self-documenting. No measurable footprint difference at < 10 ops per filter instance.
- *Why does `kindRemove` carry `headerValue=""`?* Cleaner than a discriminated union; the value-field is unused for remove ops but its zero cost is negligible. Alternative (interface or split structs) loses the cache locality benefit.

**Deferred to SPEC:** none — the shape is mechanical.

**ADR anchor:** ADR-0109 — folded into the proto-parsing ADR.

### 2.5 Request-mutation iteration in `DecodeHeaders` *(Decision 5 → ADR-0109 consequence)*

**Decision:** `DecodeHeaders` (per the framework's signature at `internal/filter/http/types.go`) executes:

```go
// Pseudo-Go; final shape is the SPEC author's call.
func (f *filter) DecodeHeaders(headers RequestHeaders, endStream bool) DecoderStatus {
    // 1. Listener-level mutations (always first, per proto comment at header_mutation.pb.go:141–142)
    applyOps(headers, f.cfg.requestOps)

    // 2. Per-route mutations in flag-controlled tier order (see §2.10 + §5.3 algorithm)
    if perRouteCfg := f.routePerTierCfg(); perRouteCfg != nil {
        for _, tierOps := range perRouteCfg.requestOpsInOrder {
            applyOps(headers, tierOps)
        }
    }

    return DecoderStatusContinue
}

// applyOps iterates a single ops slice in proto-declared order; each op mutates headers.
// Protected headers (§2.11) silently ignore mutation attempts.
func applyOps(headers RequestHeaders, ops []compiledMutationOp) {
    for _, op := range ops {
        if isProtectedHeader(op.headerName) {
            continue  // silent ignore per §2.11
        }
        switch op.kind {
        case kindRemove:
            headers.Del(op.headerName)
        case kindAppend:
            applyAppendAction(headers, op)
        }
    }
}
```

`DecoderStatusContinue` is returned unconditionally — no async-resume, no `SendLocalReply`, no `StopIteration`.

**Why this vs. alternatives:**
- *Why apply listener-level FIRST, then per-route?* Per the proto comment at `header_mutation.pb.go:141–142`. Inverting this order would silently break differential equivalence on configurations that exercise both levels.
- *Why not consolidate listener + per-route into a single resolved slice at request time?* See Decision 4 deferred-to-SPEC item 2; the consolidation is an optimization the SPEC author may choose, but the brainstorm-level model is "two passes" for clarity.

**Deferred to SPEC:** the exact `RequestHeaders` API surface used (`Del`, `Set`, `Add`, conditional variants) — the SPEC author looks up the framework's existing `RequestHeaders` interface and chooses the natural primitive for each AppendAction variant.

**ADR anchor:** ADR-0109 — folded.

### 2.6 Response-mutation iteration in `EncodeHeaders` *(Decision 6 → ADR-0109 consequence)*

**Decision:** `EncodeHeaders` executes the symmetric algorithm against `ResponseHeaders`. Returns `EncoderStatusContinue`. NOTE: this is the FIRST production filter to perform state-mutation in the encode path on the non-error path. Phase 09's `fault` exercises encode iteration only via terminal-replace (SendLocalReply causes encode-side filters to run starting at `filter[len-1]` per ADR-0075), but does not mutate response headers. Phase 07.1's `cors` injects CORS response headers on non-preflight, but does so via `DecodeHeaders` (capturing the request `Origin`) + `EncodeHeaders` injection — the encode-side path is on the response path of the matched-CORS-route only, and is logically a single-injection action rather than a programmable mutation pipeline.

Phase 10's encode path therefore validates two framework properties not previously validated under traffic:
- **EncodeHeaders is invoked on the normal upstream-response path** (cors's encode path is also normal, but only injects fixed values; phase 10 mutates programmable values across the full AppendAction × 4 + Remove surface).
- **EncodeHeaders' `ResponseHeaders` mutation methods are wired correctly** (no production filter has stress-tested the `Add`/`Set`/`Del` symmetry across decode and encode).

If the SPEC author finds a framework gap during empirical pin verification (e.g., `ResponseHeaders.Set` not implemented), that gap MUST be closed in this phase (not deferred), because a partial response-mutation implementation would silently fail on encode-side mutations under differential test.

**Why this vs. alternatives:**
- *Why not skip encode-side mutations in MVP?* The proto's `Mutations.response_mutations` is one of three explicit mutation surfaces; skipping it would degrade the filter to half-functionality and make `0012-http-header-mutation` differential trivially passable in a way that hides framework bugs.

**Deferred to SPEC:** validation that the framework's `ResponseHeaders` interface supports the same primitives as `RequestHeaders` for all 5 op variants (Remove + 4 AppendActions). If a primitive is missing, SPEC anchors a §11 framework-extension empirical pin.

**ADR anchor:** ADR-0109 — folded.

### 2.7 AppendAction × 4 semantics *(Decision 7 → ADR-0109)*

**Decision:** The four `AppendAction` enum values map to header-mutation semantics as follows. **`keep_empty_value=false` (default) skips the entire op when `op.headerValue == ""`; `keep_empty_value=true` materializes the empty value (subject to empirical pin §9.P2 verification).**

| AppendAction | When header EXISTS | When header ABSENT | Notes |
|---|---|---|---|
| `APPEND_IF_EXISTS_OR_ADD` (default; enum value 0) | Append a new value (multi-valued header gets one more value) | Add the header with this value | Maps to Go's `http.Header.Add(name, value)` |
| `ADD_IF_ABSENT` (1) | No-op | Add the header with this value | Conditional Add: `if header.Get(name) == "" { header.Add(name, value) }` |
| `OVERWRITE_IF_EXISTS_OR_ADD` (2) | Replace ALL existing values with this single value | Add the header with this value | Maps to Go's `http.Header.Set(name, value)` |
| `OVERWRITE_IF_EXISTS` (3) | Replace ALL existing values with this single value | No-op | Conditional Set: `if header.Get(name) != "" { header.Set(name, value) }` |

The "EXISTS" check operates on header-name equivalence (case-insensitive per HTTP/1.1 ABNF); multi-valued headers count as EXISTS regardless of value count. The "Replace ALL" semantics for OVERWRITE variants apply to the entire multi-valued slot — Envoy collapses the slot to a single value (the new one) per the empirical pin §9.P4.

**Why this vs. alternatives:**
- *Why not collapse `APPEND_IF_EXISTS_OR_ADD` and `OVERWRITE_IF_EXISTS_OR_ADD` to a single "always set" action?* Their semantics differ on multi-valued headers: APPEND preserves prior values + adds; OVERWRITE replaces all. The proto's distinction is load-bearing.
- *Why not error on `OVERWRITE_IF_EXISTS` against an absent header?* Envoy treats it as a silent no-op (consistent with `ADD_IF_ABSENT` against an existing header). Differential equivalence requires matching this silent-no-op behavior.

**Deferred to SPEC §11 empirical pins:**
- §9.P2: confirm `keep_empty_value=false` skips empty-value ops as the natural reading suggests.
- §9.P4: confirm OVERWRITE variants collapse multi-valued slots to single value (vs. e.g. delete-then-add-keeping-others).

**ADR anchor:** ADR-0109 — the AppendAction × 4 mapping table is part of the proto-parsing ADR.

### 2.8 `keep_empty_value` semantics *(Decision 8 → ADR-0109 consequence)*

**Decision:** When a `compiledMutationOp` has `kind = kindAppend` and `headerValue = ""`:
- If `keepEmptyValue = false` (default): the op is a SILENT NO-OP. The header is not mutated.
- If `keepEmptyValue = true`: the op proceeds with the AppendAction semantics from Decision 7, materializing an empty-string value into the header.

This reading is consistent with the proto field's documented purpose ("opt-in to writing empty values"). The empirical pin §9.P2 confirms the boundary condition.

**Why this vs. alternatives:**
- *Why not always materialize empty values?* The default (false) skips them, by Envoy convention. Always materializing diverges from default Envoy behavior.
- *Why not error on empty-value-with-default-false?* The proto allows it; Envoy treats it as silent skip; differential equivalence requires matching.

**Deferred to SPEC §11 empirical pin §9.P2.**

**ADR anchor:** ADR-0109.

### 2.9 Per-route 3-tier multi-evaluation *(Decision 9 → ADR-0110 + framework extension)*

**Decision:** Phase 10 requires the 07.1 framework to expose per-route configs UNMERGED at all three tiers (Route, VirtualHost, RouteConfiguration), in addition to the existing most-specific `Resolve` method. The framework's existing `internal/filter/http/perroute.go:103–128 Resolve(filterName, routeIdx)` returns one `proto.Message` (the most-specific tier's config); phase 10 adds a sibling method `ResolveAllTiers(filterName, routeIdx)` returning all three (or fewer if some tiers are unset).

**Proposed signature (final shape SPEC author's call):**

```go
// ResolveAllTiers returns the parsed per-route config at each tier, unmerged.
// Tiers are returned in the canonical proto order: Route (most specific),
// VirtualHost (intermediate), RouteConfiguration (least specific). A tier
// with no config for filterName at the matched route is nil. All-nil result
// means no per-route config exists for this filter at this route.
//
// Used by filters whose semantics require multi-tier evaluation rather than
// most-specific override (e.g., envoy.filters.http.header_mutation per its
// most_specific_header_mutations_wins flag). The default Resolve method
// (per ADR-0073) remains the canonical accessor for filters that use
// most-specific override (cors, fault).
func (p *PerRouteConfig) ResolveAllTiers(filterName string, routeIdx int) (route, vhost, rc proto.Message)
```

This is a NEW framework method (not a replacement). cors/fault keep using `Resolve` per ADR-0073's most-specific-override semantics; header_mutation uses `ResolveAllTiers`. The choice of accessor is per-filter and reflects the filter's own semantic model.

**Why this vs. alternatives:**

(A) **Continue using the existing `Resolve` and pretend `most_specific_header_mutations_wins` only flips between two single-tier behaviors** (e.g., always-most-specific or always-listener-level). REJECTED: this loses configuration fidelity. Customers who configure mutations at both Route and RouteConfiguration tiers expect both to apply (per the proto comment); a degraded MVP that only applies one would silently break differential equivalence on multi-tier configurations.

(B) **Generalize the framework merge function (per-filter merger interface)** so each filter declares its merge strategy. REJECTED for MVP: this is a larger framework refactor (~300+ LoC across cors + fault perroute discipline) and forces a behavioral reverification of cors + fault's per-route tests. Out of scope for phase 10.

(C) **Store the unmerged tiers in `runtimeConfig` at config time** (push the resolution into HCM-build time). REJECTED: per-route config is per-request (the matched route is known only at request time); the resolution MUST be at request time. The runtimeConfig closure-capture is for listener-level config only.

(D) **Add `ResolveAllTiers` as proposed.** ACCEPTED: smallest framework surface change (~80 LoC: new method + 2 new tests in `perroute_test.go`); zero impact on cors / fault; explicit per-filter opt-in via accessor choice.

**Deferred to SPEC:**
- Final method name (`ResolveAllTiers` vs `ResolveTiered` vs `ResolveTriple` — naming bikeshed).
- Whether to expose a fourth `nil` slot (e.g., for a listener-level synthetic) — proto only has three per-route tiers, but the SPEC author may want a 4-tuple for cleaner upstream consumption. Default position: 3-tuple matching proto's three per-route tiers.

**ADR anchor:** ADR-0110 — Multi-tier per-route evaluation: framework extension `ResolveAllTiers` + per-filter accessor-choice discipline. NOTE: ADR-0110 is the phase's PRIMARY novel-design ADR; it amends ADR-0073 (most-specific override) to clarify that ADR-0073 is the DEFAULT model, not the only model — filters whose proto semantics demand multi-tier evaluation use `ResolveAllTiers` and document the choice.

### 2.10 `most_specific_header_mutations_wins` order-of-evaluation *(Decision 10 → ADR-0110)*

**Decision:** Per the proto comment at `header_mutation.pb.go:149–154`: "If per route HeaderMutationPerRoute config is configured at multiple route levels, header mutations at all specified levels are evaluated. By default, the order is from most specific (i.e. route entry level) to least specific (i.e. route configuration level). Later header mutations may override earlier mutations. This order can be reversed by setting this field to true. In other words, most specific level mutation is evaluated last."

The application algorithm (after listener-level mutations have been applied per Decision 5):

**With `most_specific_header_mutations_wins = false` (DEFAULT):**
1. Apply Route-tier mutations (most specific, applied FIRST)
2. Apply VirtualHost-tier mutations (intermediate)
3. Apply RouteConfiguration-tier mutations (least specific, applied LAST → wins overlap)

**With `most_specific_header_mutations_wins = true`:**
1. Apply RouteConfiguration-tier mutations (least specific, applied FIRST)
2. Apply VirtualHost-tier mutations (intermediate)
3. Apply Route-tier mutations (most specific, applied LAST → wins overlap)

The `compiledMutationOp` slices for each tier are evaluated in proto-declared order WITHIN the tier (the cross-tier ordering is what the flag controls; within-tier order is fixed by the proto config).

The flag's name "most_specific_header_mutations_wins" is descriptive of the flag-true case: when true, most-specific is evaluated LAST and therefore "wins" any overlap. The flag-false case has the opposite behavior (least-specific wins). The naming convention reflects which level "wins" overlap, where "wins" = "is applied last in the evaluation chain."

**Why this vs. alternatives:**
- *Why match the proto's exact ordering vs. a simpler "route always last"?* Differential equivalence with Envoy v1.37.2 requires matching the proto's documented behavior verbatim. The flag is a load-bearing knob.
- *Why apply listener-level FIRST regardless of the flag?* The proto comment at line 141–142 ("filter configuration will always be applied first") is unambiguous; the flag only controls per-route tier ordering, not listener-vs-per-route ordering.

**Deferred to SPEC §11 empirical pin §9.P5:** confirm via reference Envoy that the algorithm applies cross-tier as documented (vs. e.g. some idiosyncratic behavior the proto comment doesn't describe).

**ADR anchor:** ADR-0110 — folded into the multi-tier per-route evaluation ADR.

### 2.11 Protected-header set *(Decision 11 → ADR-0111, empirical pin §9.P1)*

**Decision:** Envoy v1.37.2 has a hard-coded set of headers that the `header_mutation` filter silently refuses to mutate. The proto message itself does NOT have a `HeaderMutationRules` field (unlike `ext_proc`'s mutation_rules — see `mutation_rules.pb.go:54` which is for ext_proc, not for the dedicated header_mutation filter); the protected set is therefore not configurable. Envoy's hard-coded protection at minimum includes the four pseudo-headers (`:method`, `:path`, `:authority`, `:scheme`) — verifying via empirical pin §9.P1.

**Anticipated protected set (subject to §9.P1 confirmation):**
- `:method`, `:path`, `:authority`, `:scheme` (HTTP/2 pseudo-headers; Envoy MUST NOT allow rewrites that would disrupt routing semantics)
- `host` (HTTP/1.1 equivalent of `:authority`; symmetric protection expected)
- (Possibly) `content-length`, `transfer-encoding` (body-framing headers; rewriting them could corrupt the message)
- (Possibly) `x-envoy-internal` and other `x-envoy-*` headers (Envoy internal coordination headers)

The mutation attempts on protected headers are SILENT NO-OPS (no error, no log, no stat). The empirical pin §9.P1 confirms the exact set + the silent-no-op semantic.

**Why this vs. alternatives:**
- *Why hard-code vs. ADR-0040-defer?* The protected set is a load-bearing correctness invariant of the filter — without it, a misconfigured rule could break HTTP/2 routing. Deferring it to a future phase would ship a known-broken filter.
- *Why silent vs. log-warning vs. stat?* Differential equivalence: Envoy is silent; envoy-go must be silent.

**Deferred to SPEC §11 empirical pin §9.P1:** scrape reference Envoy's behavior on attempts to mutate each candidate protected header. Resolve the exact set + silent-no-op semantic.

**ADR anchor:** ADR-0111 — Protected-header set, anchored to §9.P1 pin scrape. The ADR records the exact set as resolved by the SPEC author.

### 2.12 Stats absence *(Decision 12 → ADR-0114, empirical pin §9.P3)*

**Decision:** Envoy v1.37.2 does not appear to emit any `header_mutation.*` stats for the dedicated `envoy.filters.http.header_mutation` filter. (Envoy's ext_proc filter emits a `rejected_header_mutations` counter per `mutation_rules.pb.go:51–52`, but that's ext_proc — different filter.) Phase 10 emits NO stats, matching the empirical-pinned absence (§9.P3 confirms).

If the §9.P3 scrape reveals that Envoy DOES emit `header_mutation.*` stats (against the brainstorm's expectation), the SPEC author MUST extend the BEHAVIOR_CONTRACT.md `## Stat-name mapping` section AND the filter implementation AND the `0012-http-header-mutation` fixture's stat-equality assertions. This is a "discovery contingency" — the brainstorm's POSITION is no-stats, with the empirical pin as the validation.

**Why this vs. alternatives:**
- *Why default-position no-stats vs. always-emit-something?* Differential equivalence with Envoy is the project's discipline; emitting stats Envoy doesn't would ship a divergence.
- *Why empirical-pin rather than just declaring no-stats?* The proto doesn't document filter stats one way or the other; only the reference scrape is authoritative.

**Deferred to SPEC §11 empirical pin §9.P3:** scrape `/stats` against an envoy.yaml configured with `header_mutation`; enumerate any `header_mutation.*` names; resolve the no-stats vs. some-stats verdict.

**ADR anchor:** ADR-0114 — Stats absence (or stats inventory if §9.P3 reveals stats), anchored to §9.P3 pin scrape.

### 2.13 Family-expansion shape *(Decision 13 → settled by ADR-0106)*

**Decision:** Phase 10 lands as a NEW top-level ROADMAP row `10`, NOT as a sub-phase of any parent (§9 is an umbrella heading, not a row). This is settled by ADR-0106 (BRAINSTORM Decision 12 of phase 09); no new ADR is needed in phase 10.

The flat-row family-expansion shape from ADR-0106 dictates:
- Phase 10's row in ROADMAP.md is a flat top-level row (between row 09 and any §9 family heading at line 56).
- No sibling stub rows are pre-populated (per ADR-0106(b) no-sibling-stub discipline).
- The §9 heading at ROADMAP line 56 stays unchanged (per ADR-0106(c)).

**Why this vs. alternatives:** all addressed in ADR-0106's "Alternatives considered" section. No new analysis needed.

**Deferred to SPEC:** none.

**ADR anchor:** none new — ADR-0106 anchors this decision.

---

## 3. Surface inventory (10 only)

### 3.1 New files (created in 10)

| Path | Purpose | LoC est |
|---|---|---|
| `internal/filter/http/header_mutation/header_mutation.go` | Filter implementation (factory, runtimeConfig, decode/encode methods, applyOps) | ~200 |
| `internal/filter/http/header_mutation/header_mutation_test.go` | Unit tests (proto-parse, AppendAction × 4, keep_empty_value, protected headers, multi-tier ordering) | ~280 |
| `internal/filter/http/header_mutation/doc.go` | Package overview (per cors/fault precedent) | ~20 |
| `test/fixtures/0012-http-header-mutation/envoy.yaml` | Reference Envoy config | ~80 |
| `test/fixtures/0012-http-header-mutation/envoy-go.yaml` | envoy-go config (initially identical to envoy.yaml) | ~80 |
| `test/fixtures/0012-http-header-mutation/inputs/driver.go` | Go driver exercising 5 scenarios (per §6.2) | ~150 |
| `test/fixtures/0012-http-header-mutation/expectations.yaml` | Allow-lists, ignore-lists, scenario assertions | ~40 |
| `test/fixtures/0012-http-header-mutation/README.md` | Fixture overview + scenario list | ~30 |
| `docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md` | Phase SPEC (next session, lifecycle 2 → 3) | (next session) |
| `docs/envoy-go/phases/10-http-filter-header-mutation/PLAN.md` | Phase PLAN (subsequent session, per state-3 routing) | (subsequent) |
| `docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md` | Implementation log | (subsequent) |
| `docs/envoy-go/phases/10-http-filter-header-mutation/REVIEW.md` | Phase review | (subsequent) |

### 3.2 Modified files (in 10)

| Path | Change | LoC est |
|---|---|---|
| `cmd/envoy-go/main.go` | One new `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` line | +1 |
| `internal/filter/http/perroute.go` | New `ResolveAllTiers` method + supporting types (per Decision 9) | +60 |
| `internal/filter/http/perroute_test.go` | New tests for `ResolveAllTiers` covering 3-tier configurations + nil-tier handling | +100 |
| `docs/envoy-go/ROADMAP.md` | New row `10 | http-filter-header-mutation | 09 | planned | | …` (this brainstorm); status flips at SPEC + PLAN + impl + phase-done commits | +1 row |
| `docs/envoy-go/STATE.md` | Active phase pointer + lifecycle state + next-skill | (per session) |
| `docs/envoy-go/DECISIONS.md` | New ADRs ADR-0108..ADR-0114 (~7 ADRs; ~600–900 LoC of ADR text) | +600–900 |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | New `### envoy.filters.http.header_mutation` subsection under `## HTTP filter chain` (AppendAction × 4 semantics, keep_empty_value, multi-tier ordering, protected-header set) + possible `## Stat-name mapping` extension if §9.P3 reveals stats | +80–150 |

### 3.3 Untouched files (load-bearing absence)

- `internal/filter/http/types.go` — the framework's iteration-state enums + callbacks interfaces; phase 10 does NOT extend this. The 07.1 framework supports header_mutation's needs as-is (modulo Decision 9's `ResolveAllTiers` extension to `perroute.go`, which is a separate file).
- `internal/filter/http/registry.go` — the extension registry; phase 10 uses the existing `httpReg.Register` API (no API change).
- `internal/filter/http/cors/`, `internal/filter/http/fault/`, `internal/filter/http/router/`, `internal/filter/http/envoygotest/` — sibling filter packages; phase 10 does NOT modify any of them.
- `internal/filter/hcm/` — the HTTP connection manager; phase 10 does NOT modify it (the per-route resolver `BuildPerRouteConfig` invocation is unchanged; only `Resolve` callers gain a sibling `ResolveAllTiers` accessor).
- `cmd/envoy-go/main.go` registry-Freeze line (currently line 116) — unchanged; only the Register-call list grows by one line.

The load-bearing absence: phase 10 does NOT touch the framework's iteration protocol, registry, HCM, stats subsystem, access-log subsystem, or any other §9 family-row's package. The change footprint is one new package + one framework method extension + one fixture + ROADMAP/STATE/ADR/BEHAVIOR_CONTRACT documentation.

---

## 4. Iteration-state coverage map

### 4.1 Continue-only synchronous filter

`header_mutation` is a CONTINUE-ONLY filter on both decode and encode paths:

| Path | Status returned | Notes |
|---|---|---|
| `DecodeHeaders` | `Continue` | Mutates request headers in place; never `StopIteration` |
| `DecodeData` | `Continue` (passthrough) | Filter does not consume body; trivial passthrough |
| `DecodeTrailers` | `Continue` (passthrough) | Filter does not modify trailers; trivial passthrough |
| `EncodeHeaders` | `Continue` | Mutates response headers in place; never `StopIteration` |
| `EncodeData` | `Continue` (passthrough) | Filter does not consume body |
| `EncodeTrailers` | `Continue` (passthrough) | Trivial passthrough |
| `OnDestroy` | (no-op) | No timers, no async state to clean up |

NO async-resume primitive (vs. fault). NO concurrency cap (vs. fault's `max_active_faults`). NO terminal-replace (vs. fault's abort, cors's preflight short-circuit). NO body buffering (vs. anticipated `buffer` filter).

### 4.2 Coverage relative to fault + cors

| Surface | cors @ 07.1 | fault @ 09 | header_mutation @ 10 |
|---|---|---|---|
| `DecodeHeaders` mutation in place | ❌ (only inspects Origin) | ❌ (timer-schedule only) | ✅ NEW |
| `EncodeHeaders` mutation in place | ✅ (CORS response headers) | ❌ (no encode reach) | ✅ NEW (programmable, vs cors's fixed set) |
| Async-resume | ❌ | ✅ | ❌ |
| Terminal-replace (`SendLocalReply`) | ✅ (preflight only) | ✅ (abort) | ❌ |
| Concurrency cap | ❌ | ✅ | ❌ |
| Stats emission | ❌ | ✅ (5 stats) | ❌ (per §2.12) |
| Per-route config | ✅ (most-specific via `Resolve`) | ✅ (most-specific via `Resolve`) | ✅ NEW (multi-tier via `ResolveAllTiers`) |
| Headers-field gate | ❌ | ✅ (StringMatcher.exact) | ❌ |
| Body operations | ❌ | ❌ | ❌ |

Phase 10's NEW coverage closes two framework gaps: (a) production filter exercising `EncodeHeaders` with programmable state mutation; (b) production filter requiring multi-tier per-route evaluation (vs. ADR-0073 most-specific override).

---

## 5. Per-route 3-tier multi-evaluation — header_mutation specifics

This section expands Decision 9 + Decision 10 into the algorithmic detail the SPEC author needs.

### 5.1 The semantic mismatch with ADR-0073 (most-specific override)

ADR-0073's "no field-merge; most-specific override" model: given typed_per_filter_config maps at three tiers (Route, VirtualHost, RouteConfiguration), `Resolve(filterName, routeIdx)` returns the proto.Message at the MOST-SPECIFIC tier that has an entry for filterName, discarding the others. cors and fault use this model — when a Route has its own per-route config, the VirtualHost / RouteConfiguration entries (if any) are silently ignored.

`header_mutation`'s proto requires the OPPOSITE: ALL THREE tiers' configs are evaluated (per the proto comment at `header_mutation.pb.go:149–151`). Discarding any tier's config silently breaks differential equivalence on a multi-tier configuration.

### 5.2 Framework extension: `Resolve` → `ResolveAllTiers`

Per Decision 9, the framework gains a new sibling method `ResolveAllTiers`. The proposed signature:

```go
// ResolveAllTiers returns the parsed per-route config at each tier, unmerged.
// Tiers are returned in canonical proto order: Route (most specific),
// VirtualHost (intermediate), RouteConfiguration (least specific). A tier
// with no config for filterName at the matched route is nil.
func (p *PerRouteConfig) ResolveAllTiers(filterName string, routeIdx int) (route, vhost, rc proto.Message)
```

Implementation sketch: read directly from `p.scopes[routeIdx].route[filterName]`, `p.scopes[routeIdx].vhost[filterName]`, and `p.rc[filterName]` — no most-specific selection logic. The cache (`p.cache`) is NOT used for `ResolveAllTiers` (or uses a separate cache key shape like `cacheKey{filterName, routeIdx, tier}`); SPEC author chooses based on hot-path measurement.

Test extension: `perroute_test.go` gains 2–3 new test functions covering (a) all-three-tiers-set, (b) two-of-three set with various combinations, (c) one-tier-set (matches existing Resolve behavior except returns the tier's tuple-position instead of just the value).

### 5.3 Application algorithm

```go
// Pseudo-Go; final shape is the SPEC author's call.
func (f *filter) DecodeHeaders(headers RequestHeaders, endStream bool) DecoderStatus {
    // Always-first: listener-level mutations
    applyOps(headers, f.cfg.requestOps)

    // Per-route: resolve all three tiers
    routeMsg, vhostMsg, rcMsg := f.cb.RequestRouteConfigsAllTiers(filterName)
    routeOps := compileFromMsg(routeMsg, requestSide)
    vhostOps := compileFromMsg(vhostMsg, requestSide)
    rcOps := compileFromMsg(rcMsg, requestSide)

    if !f.cfg.mostSpecificHeaderMutationsWins {
        // Default: most-specific FIRST, least-specific LAST → least-specific wins
        applyOps(headers, routeOps)
        applyOps(headers, vhostOps)
        applyOps(headers, rcOps)
    } else {
        // Reversed: least-specific FIRST, most-specific LAST → most-specific wins
        applyOps(headers, rcOps)
        applyOps(headers, vhostOps)
        applyOps(headers, routeOps)
    }

    return DecoderStatusContinue
}
```

`compileFromMsg` is a helper that re-parses the per-tier `proto.Message` (an `*envoyextensionsfiltershttpheadermutationv3.HeaderMutationPerRoute`) into a `[]compiledMutationOp` slice on demand. The SPEC author MAY choose to pre-compile per-route ops at HCM-build time (in a parallel cache to `runtimeConfig`) to avoid hot-path re-parsing; this is an optimization choice deferred to SPEC.

### 5.4 Empirical pin

§9.P5 confirms via reference Envoy that the cross-tier ordering is exactly as the proto comment describes. The pin is a sanity check; the proto comment is unambiguous.

---

## 6. Differential fixture 0012-http-header-mutation — scenarios + driver shape

### 6.1 Fixture topology

Single listener `l_test` (HTTP/1.1 plaintext, `0.0.0.0:10000`); single route_config `rc_main` with three routes covering tier configurations:

- `route_a` (path prefix `/listener-only`): no per-route header_mutation config; exercises listener-level mutations only.
- `route_b` (path prefix `/route-override`): per-route `HeaderMutationPerRoute` at the Route tier; tests per-route + listener-level interaction.
- `route_c` (path prefix `/multi-tier`): per-route `HeaderMutationPerRoute` at the Route tier AND VirtualHost tier AND RouteConfiguration tier; tests `most_specific_header_mutations_wins` flag in BOTH directions across two listener configurations (`l_test_a` with flag=false, `l_test_b` with flag=true).

Single upstream cluster `c_echo` pointing at a containerized echo backend that returns received headers in the response body (so the fixture can verify request-header mutations differentially without out-of-band logging).

### 6.2 Scenarios

| # | Path | Listener flag | Per-route tiers | Verifies |
|---|---|---|---|---|
| 1 | `/listener-only` | (any) | none | Listener-level request_mutations + response_mutations covering all 4 AppendActions + Remove |
| 2 | `/route-override` | false (default) | Route only | Per-route + listener interaction (listener applied first, then route; route wins overlap) |
| 3 | `/multi-tier?which=lws` | false (default) | Route + VirtualHost + RouteConfiguration | Default ordering: Route → VirtualHost → RouteConfiguration; RouteConfiguration wins overlap |
| 4 | `/multi-tier?which=mws` | true | Route + VirtualHost + RouteConfiguration | Reversed ordering: RouteConfiguration → VirtualHost → Route; Route wins overlap |
| 5 | `/listener-only?protected=1` | (any) | none | Attempts to mutate `:method`, `:path`, `:authority`, `host`, `content-length`; expects silent no-op verified via differential equivalence |

Scenario 5's exact protected-header set is calibrated to §9.P1 empirical pin output. If the §9.P1 pin reveals additional protected headers, scenario 5 expands to cover them.

### 6.3 Driver shape

```go
// Pseudo-Go; final driver in inputs/driver.go.
//
// Each scenario sends a single HTTP/1.1 GET with a configured set of request
// headers; the upstream echo backend reflects request headers into the
// response body. The driver collects (status, response-headers, body-as-headers)
// and asserts equivalence between the two proxies.
func runScenarios(env *fixtureenv.Env) error {
    for _, sc := range scenarios {
        respE := env.SendToReference(sc.path, sc.requestHeaders)
        respG := env.SendToSubject(sc.path, sc.requestHeaders)

        if err := assertHeaderEquivalence(respE, respG, sc.expected); err != nil {
            return fmt.Errorf("scenario %s: %w", sc.name, err)
        }
    }
    return nil
}
```

The driver does NOT exercise concurrency (single sequential request per scenario; `header_mutation` has no concurrency surface). The driver does NOT exercise timing assertions (synchronous filter; no time.AfterFunc). The driver SHOULD exercise multi-valued response headers (e.g., multiple `Set-Cookie` or `Vary` headers) to validate AppendAction multi-value semantics under §9.P4.

### 6.4 Header-allow-list extensions

Anticipated NONE. `header_mutation` does not introduce new "Envoy-emitted" headers (unlike e.g. `cors` adding `access-control-*` or `fault` setting `x-envoy-fault-injected`). The fixture's mutations are user-configured; the BEHAVIOR_CONTRACT.md HV-allow-list is NOT extended.

If §9.P1 reveals that Envoy's protection set includes `x-envoy-*` headers AND the protection is leaky (i.e., Envoy partially mutates and then reverts), the SPEC author may need to add a narrow allow-list entry. Default position: no allow-list extension.

### 6.5 Timing tolerance

NONE. `header_mutation` is synchronous; no time-bounded assertions in scenarios.

---

## 7. Anticipated ADRs (ADR-0108 through ADR-0114)

| ADR | Title (working) | Lands-in-task (anticipated) | Decision anchor |
|---|---|---|---|
| ADR-0108 | `internal/filter/http/header_mutation/` package shape + boot registration (mirrors ADR-0100) | impl Task 2–3 | Decisions 1, 2 |
| ADR-0109 | `runtimeConfig` shape + 4-field consumed / 1-field silent-ignore + `compiledMutationOp` flat-struct + AppendAction × 4 mapping table + `keep_empty_value` semantics | impl Task 3–4 | Decisions 3, 4, 7, 8 |
| ADR-0110 | Multi-tier per-route evaluation: framework extension `ResolveAllTiers` + per-filter accessor-choice discipline + `most_specific_header_mutations_wins` order-of-evaluation algorithm; amends (does not supersede) ADR-0073 | framework Task 1 + impl Task 5 | Decisions 9, 10 |
| ADR-0111 | Protected-header set (resolved against Envoy v1.37.2 per §9.P1) + silent-no-op semantic | impl Task 4 | Decision 11 |
| ADR-0112 | `mutations.query_parameter_mutations[]` deferred — coupled to `KeyValueMutation` triple + path/query rewriting subsystem (deferral ADR per ADR-0040 format) | doc Task | Decision (deferral; §8.1) |
| ADR-0113 | Header-value formatter substitution (`%REQ(:path)%` etc) deferred — full Envoy command-string subsystem is its own multi-phase project (deferral ADR per ADR-0040 format) | doc Task | Decision (deferral; §8.2) |
| ADR-0114 | Stats absence (or stats inventory if §9.P3 reveals stats); BEHAVIOR_CONTRACT no-emit discipline analogous to cors per ADR-0074 | impl Task 4 | Decision 12 |

ADR-0110 is the phase's PRIMARY novel-design ADR (the multi-tier per-route framework extension). ADR-0108 + ADR-0109 are the package-shape + parser ADRs (mechanical, mirror ADR-0100 + ADR-0101). ADR-0111 + ADR-0114 are empirical-pin-anchored. ADR-0112 + ADR-0113 are deferral ADRs.

The exact ADR list firms up at SPEC time. The brainstorm's list is anticipatory; the SPEC author may consolidate (e.g., ADR-0108 + ADR-0109 into one), split (e.g., ADR-0109 by primitive vs. AppendAction table), or extend (if §9 empirical pins reveal new design knobs).

---

## 8. Out-of-scope deferrals (each gets a deferral ADR per ADR-0040 format)

### 8.1 `mutations.query_parameter_mutations[]` (KeyValueMutation triple)

The proto's `Mutations.query_parameter_mutations` field (per `header_mutation.pb.go:35`) is a `[]*KeyValueMutation` (the type from `envoy/config/common/mutation_rules/v3` — a query-parameter analogue of `HeaderMutation`). It rewrites path-query (`?key=value&...`) entries.

**Deferral rationale:**
- Path-query rewriting is a different surface than header rewriting; the project has not built a path-query mutation pipeline (the closest existing surface is router's path rewriting, which is a single-target transform, not a programmable mutation list).
- The `KeyValueMutation` type has its own AppendAction-equivalent semantics (oneof of remove/append/overwrite), distinct from `HeaderMutation`'s, requiring a parallel `compiledQueryOp` shape.
- Including it in phase 10 would inflate the surface by ~150 LoC + 2 fixture scenarios + 1 ADR for the parallel primitive — net ~+15% surface, low marginal value for a Tight MVP.
- Future phase candidate: a dedicated `query_parameter_mutations` extension to phase 10 (numbered `10.1` per ADR-0045 internal split, or a new top-level row at family-expansion time).

ADR-0112 records this deferral per the ADR-0040 format.

### 8.2 Header-value formatter substitution (`%REQ(:path)%`, `%DOWNSTREAM_REMOTE_ADDRESS%`, etc)

Envoy's `HeaderValue.value` field accepts formatter-substitution syntax: tokens like `%REQ(:path)%`, `%RESPONSE_CODE%`, `%DOWNSTREAM_REMOTE_ADDRESS_WITHOUT_PORT%`, `%START_TIME(%Y-%m-%dT%H:%M:%S)%` are evaluated against per-request context. Phase 10's brainstorm position: header values are STATIC strings only; formatter syntax is silently treated as a literal value (i.e., a header value of `"%REQ(:path)%"` is materialized verbatim, not substituted).

**Deferral rationale:**
- Envoy's formatter-substitution is a substantive subsystem (~30+ tokens + parameterized variants + escape rules + nested expressions). It is shared across multiple Envoy filters (access-log, header_mutation, request_id, lua) and warrants its own multi-phase landing.
- The 06.2 access-log phase (per ADR-0090–0094 anticipated, actual ADRs in phase 06.2 PROGRESS) implemented a NARROW subset of formatter syntax for access-log specifically — that subset is reusable for header_mutation, but the BEHAVIOR_CONTRACT.md ## Access log field mapping section codified it for access-log only. Reusing the access-log formatter for header_mutation would require either (a) extending the formatter to header-value scope (needs request/response header context), or (b) a parallel implementation. Either is its own surface.
- Phase 10's MVP scope (per Decision settled with the user) explicitly excludes formatter syntax.

ADR-0113 records this deferral per the ADR-0040 format.

### 8.3 Header-mutation filter stats absence (§9.P3 contingent)

Per §2.12 / §9.P3, the brainstorm's POSITION is that Envoy v1.37.2 emits NO `header_mutation.*` stats for the dedicated filter. If the empirical pin §9.P3 confirms this, ADR-0114 codifies the no-emit discipline (analogous to cors's no-stats per ADR-0074). If the §9.P3 scrape reveals stats, the deferral discussion is moot (phase 10 emits whatever Envoy emits; ADR-0114 records the inventory + emission).

This is NOT a "deferral" in the same sense as §8.1 / §8.2 — the absence is documented as a fact, not deferred to a future phase. The contingent framing exists in case the empirical pin overturns the brainstorm's position.

---

## 9. Empirical-pin obligations for SPEC author (resolved against Envoy v1.37.2)

This section enumerates what the phase 10 SPEC author MUST scrape from reference Envoy v1.37.2 BEFORE finalizing SPEC.md. Each pin produces a load-bearing fact that gets codified in either an ADR or BEHAVIOR_CONTRACT.md. The empirical-pin discipline is per ADR-0004 (autonomous-brainstorming adaptation): empirical questions are deferred to SPEC drafting time so the brainstorm can complete without scrape evidence.

### 9.1 P1 — Protected-header set

**Question:** Which request and response headers does Envoy v1.37.2's `envoy.filters.http.header_mutation` filter silently refuse to mutate?

**Method:** Configure an envoy.yaml with a `header_mutation` filter attempting to (a) Remove and (b) OVERWRITE_IF_EXISTS_OR_ADD each of the candidate headers: `:method`, `:path`, `:authority`, `:scheme`, `host`, `content-length`, `transfer-encoding`, `connection`, `keep-alive`, `te`, `upgrade`, `proxy-connection`, `x-envoy-internal`, `x-envoy-decorator-operation`, `x-forwarded-for` (this last one is informative — Envoy may allow it). Send a request that exercises each. Compare reference vs. configured-for-mutation headers seen at the upstream backend.

**Expected output:** Concrete protected set (e.g., `{:method, :path, :authority, :scheme, host, content-length, transfer-encoding, connection}`). ADR-0111 records the exact set.

**Anchor:** §2.11; ADR-0111.

### 9.2 P2 — `keep_empty_value` boundary

**Question:** When `HeaderValueOption.keep_empty_value=false` (default) AND `HeaderValueOption.header.value=""`, does Envoy silently skip the mutation? When `keep_empty_value=true`, does Envoy materialize an empty-value header?

**Method:** Configure an envoy.yaml with two listeners (or two routes), one per `keep_empty_value` setting. Configure mutations with empty values across all 4 AppendAction variants. Verify response headers at the upstream backend reflect (a) silent-skip on `keep_empty_value=false`, (b) materialized empty values on `keep_empty_value=true`.

**Expected output:** Confirmation of the natural reading, codified in §2.8.

**Anchor:** §2.8; ADR-0109 reference.

### 9.3 P3 — Filter stats verification

**Question:** Does Envoy v1.37.2 emit any `header_mutation.*` stats for the `envoy.filters.http.header_mutation` filter?

**Method:** Configure an envoy.yaml with an active `header_mutation` filter; send a defined load (e.g., 5 requests across 2 routes); scrape `/stats` and `/stats/prometheus`; grep for `header_mutation` and the filter's stat_prefix (if configurable — check the proto for a stat_prefix field; per the proto inspection, no stat_prefix field exists, so any stats would use a fixed namespace).

**Expected output:** Either (a) confirmation that Envoy emits no `header_mutation.*` stats (the brainstorm's position; ADR-0114 codifies no-emit discipline), or (b) an inventory of names + types (counters/gauges) that ADR-0114 enumerates.

**Anchor:** §2.12; ADR-0114; BEHAVIOR_CONTRACT.md `## Stat-name mapping` extension if needed.

### 9.4 P4 — AppendAction × 4 multi-valued-header behavior

**Question:** When applying `OVERWRITE_IF_EXISTS_OR_ADD` against a multi-valued header (e.g., a request with `Accept-Encoding: gzip, deflate` becoming a target of OVERWRITE with value `br`), does Envoy collapse the multi-valued slot to a single value (`br`), or does it produce some other behavior (e.g., preserve some values, error, etc)?

**Method:** Configure an envoy.yaml with mutations against multi-valued headers (`Accept`, `Vary`, `Set-Cookie`, `Cache-Control`); send requests with multi-valued source headers; observe the upstream backend's view of mutated request headers AND the downstream client's view of mutated response headers.

**Expected output:** Confirmation of the natural reading (collapse-to-single), codified in §2.7.

**Anchor:** §2.7; ADR-0109 reference.

### 9.5 P5 — `most_specific_header_mutations_wins` evaluation order

**Question:** Does Envoy's `most_specific_header_mutations_wins` flag produce the cross-tier evaluation order documented in the proto comment at `header_mutation.pb.go:149–154`? (i.e., flag=false: Route → VirtualHost → RouteConfiguration; flag=true: RouteConfiguration → VirtualHost → Route)

**Method:** Configure an envoy.yaml with a route_config exercising all three per-route tiers AND a `header_mutation` filter at the listener level. Configure overlapping mutations (e.g., listener sets `X-Test: listener`, RouteConfiguration sets `X-Test: rc`, VirtualHost sets `X-Test: vh`, Route sets `X-Test: route`). Send a request matching the route. Observe the final `X-Test` value at the upstream. Repeat with `most_specific_header_mutations_wins` toggled.

**Expected output:** Confirmation that the algorithm in §2.10 matches Envoy's behavior. If divergence, the algorithm is corrected and ADR-0110 records the corrected version.

**Anchor:** §2.10; ADR-0110.

---

## 10. ROADMAP row addition

This brainstorm appends a single new row to `docs/envoy-go/ROADMAP.md`, immediately after the existing row 09:

```
| 10 | http-filter-header-mutation | 09 | planned |  | New `internal/filter/http/header_mutation/` package implementing `envoy.filters.http.header_mutation` (Envoy v1.37.2 canonical header-mutation filter) under the 07.1 framework. THIRD §9 family-row (after cors @ 07.1, fault @ 09). MVP envelope per BRAINSTORM §1.1: `mutations.request_mutations` + `mutations.response_mutations` (both directions; AppendAction × 4 + Remove + `keep_empty_value`); `HeaderMutationPerRoute` + `most_specific_header_mutations_wins` (both true/false; multi-tier evaluation across Route/VirtualHost/RouteConfiguration tiers); protected-header set per §9.P1 empirical pin. Differential fixture `0012-http-header-mutation` (5 scenarios per BRAINSTORM §6.2). NEW framework method `PerRouteConfig.ResolveAllTiers` (~80 LoC sibling to existing `Resolve` per ADR-0073; amends ADR-0073). Anticipated ADRs ADR-0108..ADR-0114 (~7 ADRs per BRAINSTORM §7). Per ADR-0106 (BRAINSTORM Decision 12 of phase 09), §9 family-rows are flat top-level rows; phase 10 lands as row `10`, NOT as a sub-phase of any §9 parent. ADR-0045 surface-split release valve stays available if SPEC/PLAN find > ~1500 LoC / > ~25 tasks; brainstorm's position is single-row. |
```

The row's `status` flips:
- `planned` → `in-progress` at the SPEC commit (lifecycle 2 → 3).
- `in-progress` → `done` at the phase-done commit (lifecycle 6).

The row's `summary` cell may receive minor edits at SPEC + impl + phase-done commits (per the existing project precedent at row 09's evolution: brainstorm placeholder → SPEC fill-out → phase-done final).

The §9 heading at ROADMAP line 56 is NOT modified (per ADR-0106(c)).

---

## 11. Open questions / risks

### 11.1 Framework extension scope creep

Decision 9's `ResolveAllTiers` method is small (~80 LoC). However, if SPEC writing reveals that the existing `Resolve` cache logic needs to be reworked to support `ResolveAllTiers` cleanly (e.g., if the cache key becomes `(filterName, routeIdx, tier)` and existing cors/fault tests need to be reverified), the framework footprint could grow to ~150–250 LoC. This is still well under the ADR-0045 split-gate, but the SPEC author should validate during framework Task 1 that no ADR-0073 reverification is required.

**Mitigation:** SPEC author runs cors + fault unit tests against the framework after `ResolveAllTiers` lands but before header_mutation impl; any regression triggers an immediate split-by-surface (10.1 = framework alone; 10.2 = filter atop the proven-stable framework).

### 11.2 Empirical pin §9.P1 set inflation

The protected-header set could be larger than the ~5–8 candidates anticipated in §2.11. Envoy could protect, e.g., 20+ headers including all `x-envoy-*`, all `:`-prefixed pseudo-headers, hop-by-hop headers, etc. A larger protected set inflates the SPEC + ADR-0111 + fixture scenario 5 surface.

**Mitigation:** the protected-header check is a single `isProtectedHeader(name)` call returning bool; the set's cardinality does not affect the algorithm or LoC materially. ADR-0111 simply enumerates whatever set §9.P1 reveals.

### 11.3 Encode-side framework gap discovery

§2.6 anticipates that the framework's `ResponseHeaders` interface supports the same primitives as `RequestHeaders` (Add, Set, Del, plus the conditional variants needed for `ADD_IF_ABSENT` and `OVERWRITE_IF_EXISTS`). If a primitive is missing (e.g., `ResponseHeaders` lacks a `Get`-then-`Add` conditional), the SPEC author MUST extend the framework before implementing the filter — or the encode-side mutations silently fail.

**Mitigation:** SPEC author explicitly includes a §11 or §13 framework-coverage check as Empirical Pin §9.P6 (added at SPEC time if needed). If gap found, framework extension is part of phase 10 (not split out — small surface).

### 11.4 `most_specific_header_mutations_wins` proto-comment ambiguity

The proto comment at `header_mutation.pb.go:149–154` is clear but could be subtly different from Envoy's actual behavior (proto comments are documentation, not specification). §9.P5 verifies the proto-comment-vs-actual match.

**Mitigation:** §9.P5 empirical pin is explicitly a sanity check. If divergence, ADR-0110 records the corrected algorithm.

### 11.5 Header-name normalization

Envoy normalizes header names to lowercase per HTTP/2 conventions; HTTP/1.1 has case-insensitive matching but preserves case-on-the-wire. The filter's behavior on `X-Test` vs `x-test` source vs target names should match Envoy: lowercase comparison for matching, lowercase storage for HTTP/2, configured case for HTTP/1.1 wire emission. The framework's `RequestHeaders.Get/Set/Del` interfaces presumably handle this normalization internally; phase 10 does not need to re-implement normalization.

**Mitigation:** SPEC author validates the framework's case-handling by including a "mixed-case mutation" sub-scenario in fixture scenario 1; differential equivalence over HTTP/1.1 + HTTP/2 (the latter via h2 routing in fixture 0004) confirms.

---

## 12. Handoff to SPEC author

The next session, per the state machine §5 step 2 (SPEC.md exists, PLAN.md does not — adjusted for the project's BRAINSTORM-then-SPEC pattern: BRAINSTORM.md exists, SPEC.md does not), is the SPEC-authoring session. Skill: `superpowers:writing-plans` per ADR-0005, but routed through SPEC-authoring first per the phase 09 precedent (the project's lifecycle has BRAINSTORM → SPEC → PLAN → impl → review, not the BOOTSTRAP_PROMPT.md §5 SPEC-only-output assumption).

The SPEC author MUST:

1. Read this `BRAINSTORM.md` in full.
2. Resolve all five §9 empirical pins against reference Envoy v1.37.2 IN-SESSION (per ADR-0004) — NOT defer to later sessions.
3. Author `docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md` covering: §1 mission, §§2–N implementation surface (per the §3 surface inventory in this brainstorm), §11 empirical-pin block (§9 here), §15 acceptance checklist (per the phase 09 SPEC pattern).
4. NOT begin implementation. NOT author PLAN.md. NOT modify any Go file.
5. Run `spec-document-reviewer` subagent loop per ADR-0004 (max 3 iterations; if the loop exceeds, set STATE.md `lifecycle-state` = `blocked` + `block-reason` and exit).
6. On reviewer-approved SPEC, update STATE.md to point at the next session's `superpowers:writing-plans` invocation; commit the SPEC + STATE.md update.

ADR numbering: SPEC author uses ADR-0108 onward; the brainstorm's anticipated list (§7) is anticipatory — SPEC author may consolidate, split, or extend.

ROADMAP row 10 status flips `planned → in-progress` at the SPEC commit (per the standard lifecycle).

---

*End of phase 10 BRAINSTORM.md.*
