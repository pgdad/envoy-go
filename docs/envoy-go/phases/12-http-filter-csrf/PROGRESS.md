# Phase 12 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..11 PROGRESS.md structure.

## Preamble — execution preconditions

All 16 preconditions verified green at cold-start without deviation. Worktree branch `phase-12-http-filter-csrf-impl`; master tail shows PLAN SHA-fill at `1c1c804`, PLAN at `b903ab2`, SPEC SHA-fill at `fb4d582`, SPEC at `a305b86`, BRAINSTORM commits at `bb29bb0`/`c2e7559`/`399532c`/`ba58c7e`/`7fd9213`, phase-11 REVIEW at `0f3a710`. Go 1.26.2, golangci-lint 1.64.8, Docker client+server present. ADR tail at 0119 (next-free 0120). SPEC at a305b86. `internal/filter/http/csrf/` absent. `fixture.HTTPCsrf` absent. CONFORMANCE_PINS.md unchanged. 6 `httpReg.Register` calls in main.go. `### envoy.filters.http.local_ratelimit` at line 1008. `CsrfPolicy` + `StringMatcher` protos present. Envoy image v1.37.2 SHA confirmed. Working tree pristine (untracked `.wt-parent` is worktree-machinery file, not uncommitted work). All 14 differential fixtures (0000–0013) PASS.

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The five ADRs anticipated by SPEC §8 (ADR-0120..ADR-0124). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0120** `internal/filter/http/csrf/` package shape (single-token directory matching cors precedent + extension-registry registration ordering + decoder-only `HTTPFilter` value with `Encoder: nil`) — Task 2
- **ADR-0121** runtimeConfig shape + 1-consumed/1-PGV-validated-not-honored/1-deferred field decomposition (`additional_origins[].StringMatcher.exact` non-empty values consumed; non-exact variants dropped at PARSE per ADR-0101 §3; `filter_enabled` PGV-validated at parse-time per §11.11 amendment but silent-ignored at runtime; `shadow_enabled` optional at parse + silent-ignored at runtime) + PGV-mirror filter-internal validation discipline at `New` time (envoy-go own-wording errors per planner-time decision 4 — phase 11 ADR-0115 precedent) + parse-time-drop discipline for non-exact StringMatcher variants per ADR-0101 §3 verbatim discipline (NOT match-time-keep-and-fail) — Task 2
- **ADR-0122** Origin extraction trichotomy (Origin: `null` literal → empty; Origin empty/absent → Referer fallback; Origin non-empty unparseable → verbatim string) + comparison algorithm host:port-only equality + method gate canonical 4-method set `{POST, PUT, DELETE, PATCH}` + `additional_origins[].exact` matched against `host[:port]` form + scheme-strip discipline — Task 3
- **ADR-0123** Rejection path wire shape + body byte-exact `Invalid origin` + 4-header set lowercase wire-form + 403 hardcoded status + `SendLocalReply` reuse from phase 09 — Task 3
- **ADR-0124** Stat-table 26→29-name extension + 3 csrf counters + namespace anchor at HCM stat_prefix reusing existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor (NO new SN flattening rule) + drop `shadow_request_invalid` from MVP + per-route stats SHARED with listener-level (diverges from phase 11 ADR-0117 precedent) — Task 4

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The nine planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — Filter-callback wiring hook = `SetDecoderCallbacks(cb)`; encode side ABSENT** (decoder-only filter; HTTPFilter struct sets Decoder: f, Encoder: nil — first §9 production filter to express decoder-only structurally).
2. **PLAN-emerging — `HTTPFilter` value shape = `Decoder: f, Encoder: nil`** (saves implementing StreamEncoderFilter method set; makes decoder-only nature self-documenting).
3. **D2 — URL-parse semantics for `hostAndPort()` = `net/url.Parse` + verbatim-string-on-parse-failure** (mirrors Envoy's `Http::Utility::Url::initialize` for common cases; verified at unit-test group 5 + §11 empirical pins as regression baseline).
4. **D3 — Filter-internal validation error message wording = envoy-go's own clear-text wording** (option (b); `csrf: filter_enabled is required` + `csrf: filter_enabled.default_value is required`; phase 11 ADR-0115 precedent for envoy-go-own-wording).
5. **D4 — Per-route stats wiring mechanism = OPTION (b) per-route runtime built via `buildPerRouteRuntime(perRoute, listenerStats)` helper** (called from DecodeHeaders at request time; per-route runtimeConfig SHARES the listener-level *filterStats pointer; no NewCounterIfAbsent re-registration; no caching for MVP).
6. **PLAN-emerging — File-split decision = SINGLE-FILE `csrf.go`** (no `origin.go` split; ~280 LoC stays under mental-model threshold; no future filter anticipated to reuse the host:port-only equality helpers).
7. **PLAN-emerging — Fixture topology = SINGLE LISTENER `l_main` with TWO ROUTES** (`/` default + `/route-only` per-route TPFC; fits existing `fixture.Driver` contract; saves driver complexity vs phase 11's 4-listener topology).
8. **PLAN-emerging — `:scheme` synthesis for `targetOriginValue` = USE A SYNTHETIC `http://` PREFIX** (no framework extension; no `:scheme` injection; no `DownstreamTLS()` callback; the synthetic prefix is stripped via `hostAndPort()` per §11.3 amendment so byte-equivalence with reference Envoy is preserved).
9. **PLAN-emerging — BackendKind enum value = `HTTPCsrf BackendKind = 11`** (continues existing naming convention).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** `3f34717` — `phase 12: PROGRESS preamble + planner-time decision resolution`
**Notes:** Created PROGRESS.md; verified all 16 preconditions per PLAN §"Execution preconditions"; phase-12 SPEC + PLAN confirmed present in HEAD; SPEC at a305b86; ADR tail at 0119 (next-free 0120); internal/filter/http/csrf/ absent (Task 2 lands); fixture.HTTPCsrf absent (Task 9 lands). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase-12-http-filter-csrf-impl
$ go version
go version go1.26.2 linux/amd64
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
119
$ git log -1 --format=%H -- docs/envoy-go/phases/12-http-filter-csrf/SPEC.md
a305b8662b43c8321f0d091415bb6b283ae8211b
$ go test -count=1 ./test/differential/ -run 'TestDifferential' -v
--- PASS: TestDifferential (41.42s)
    --- PASS: TestDifferential/0000-tcp-echo (1.54s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.24s)
    --- PASS: TestDifferential/0002-tls-tcp (1.29s)
    --- PASS: TestDifferential/0003-http11-routing (1.29s)
    --- PASS: TestDifferential/0004-h2-routing (1.74s)
    --- PASS: TestDifferential/0005-prometheus-stats (1.97s)
    --- PASS: TestDifferential/0006-access-log (10.93s)
    --- PASS: TestDifferential/0007a-cors (1.35s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.66s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.65s)
    --- PASS: TestDifferential/0009-admin-config-dump (1.98s)
    --- PASS: TestDifferential/0010-graceful-drain (9.35s)
    --- PASS: TestDifferential/0011-http-fault (1.94s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.33s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.17s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	42.575s
```

## Task 2 — `internal/filter/http/csrf/` package skeleton + New factory PGV-mirror + parse-time StringMatcher drop

**Commits:** `d127af4` — `phase 12: csrf package skeleton + New factory PGV-mirror + parse-time StringMatcher drop [ADR-0120, ADR-0121]`
**Notes:** Created `internal/filter/http/csrf/{doc.go, csrf.go, csrf_test.go}` per PLAN Task 2. TDD discipline: wrote failing tests (Group 1 PGV + Group 2 StringMatcher parse-time-drop) first; verified compile failure; then landed `doc.go` + `csrf.go` skeleton. Two minor PLAN-text adjustments at impl time: (a) `filterStats` field type is `*stats.Counter` (not `*atomic.Int64` as the PLAN/SPEC §6.2 documented conceptually) since `stats.Registry.NewCounter` returns `*stats.Counter` — the SPEC §6.2 documents `*atomic.Int64` as the conceptual semantics, and `*stats.Counter` itself wraps `atomic.Uint64` so the lock-free-Inc semantic is preserved (matches phase 11 ADR-0115's `filterStats` shape); (b) the PLAN's stub helpers (`sourceOriginValue`, `targetOriginValue`, `hostAndPort`, `evaluate`, `buildPerRouteRuntime`) + the `_ = url.Parse` import-keepalive were OMITTED — golangci-lint's `unused` linter flagged them; Task 3 lands them with their bodies + the `net/url` import in lockstep. Captured an ADR cross-reference adjustment in ADR-0121: planner-time decision 4's "envoy-go-own-wording" choice differs structurally from phase 11 ADR-0115's verbatim-mirror choice (numeric-bound check there vs proto-shape check here); ADR-0121 §Decision (ii) records the rationale.

ADR-0120 + ADR-0121 land at this commit per the ADR-0044 ADR-on-impl convention. Both follow the ADR-0001 7-section template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). ADR-0120 anchors the package shape (single-token directory matching cors precedent; decoder-only `HTTPFilter` value with `Encoder: nil`; boot-registration ordering between `cors` and `envoygotest`). ADR-0121 anchors the 1+1+1 field decomposition + the PGV-mirror filter-internal validation discipline (envoy-go-own-wording errors per planner-time decision 4) + the StringMatcher non-exact parse-time-drop discipline per ADR-0101 §3.

**Outputs:**
```
$ go build ./internal/filter/http/csrf/...
$ go vet ./internal/filter/http/csrf/...
$ golangci-lint run ./internal/filter/http/csrf/...
$ go test -race -count=1 -v ./internal/filter/http/csrf/
=== RUN   TestNew_NilTC
--- PASS: TestNew_NilTC (0.00s)
=== RUN   TestNew_MalformedTC
--- PASS: TestNew_MalformedTC (0.00s)
=== RUN   TestNew_FilterEnabledNil_RejectAtParseTime
--- PASS: TestNew_FilterEnabledNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime
--- PASS: TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored
--- PASS: TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored (0.00s)
=== RUN   TestNew_FilterEnabledHundredPercent_Accepted
--- PASS: TestNew_FilterEnabledHundredPercent_Accepted (0.00s)
=== RUN   TestNew_ShadowEnabledAbsent_Accepted
--- PASS: TestNew_ShadowEnabledAbsent_Accepted (0.00s)
=== RUN   TestNew_ShadowEnabledPresent_SilentIgnored
--- PASS: TestNew_ShadowEnabledPresent_SilentIgnored (0.00s)
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/prefix
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/suffix
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/contains
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/safe_regex
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/ignore_case_with_exact
--- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/prefix (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/suffix (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/contains (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/safe_regex (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/ignore_case_with_exact (0.00s)
=== RUN   TestNew_AdditionalOrigins_EmptyExactValue_Dropped
--- PASS: TestNew_AdditionalOrigins_EmptyExactValue_Dropped (0.00s)
=== RUN   TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm
--- PASS: TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.010s
$ grep -nE '^## ADR-0120|^## ADR-0121' docs/envoy-go/DECISIONS.md
5502:## ADR-0120: `internal/filter/http/csrf/` package shape — single-token directory matching cors precedent + extension-registry registration ordering + decoder-only `HTTPFilter` value
5550:## ADR-0121: `runtimeConfig` shape + 1-consumed / 1-PGV-validated-not-honored / 1-deferred field decomposition + PGV-mirror filter-internal validation discipline + StringMatcher non-exact parse-time-drop discipline
```

### Task 2 follow-up — code-review issue fix-ups (I-1, I-2, M-1, M-3)

**Commit:** `6bf381e` — `phase 12 Task 2 follow-up: code-review issue fix-ups (I-1 ADR-0121 prose; I-2 newFilterStats nil-guard; M-1, M-3)`
**Notes:** Code-quality reviewer returned Approved-with-comments on the Task 2 commit (`d127af4`); this follow-up addresses the two Important + two cheap Minor issues without scope creep:

- **I-1 (DECISIONS.md ADR-0121 §Decision (ii)):** Rewrote the self-correcting "— wait, ADR-0115 chose option (a) verbatim mirroring; the csrf precedent is the 50ms case INVERTED — phase 11 chose (a) for the boot-log byte-equivalence claim; phase 12 chooses (b) because…" mid-clause as polished ADR prose. New §Decision (ii) states the inversion crisply: phase 11 chose (a) verbatim Envoy-mirror wording for `fill_interval`'s numeric-bound check (canonical `server.cc:76` byte-equivalence target); phase 12 chooses (b) envoy-go-own-wording for csrf's proto-shape PGV check (no canonical Envoy-mirror equivalent — Envoy's PGV-template-generated messages are not hand-written byte-equivalence targets). §Decision and §Consequences are now mutually consistent (the §Consequences bullet on numeric-bound vs proto-shape was already crisp; §Decision (ii) now restates the same distinction without stream-of-consciousness).
- **I-2 (csrf.go newFilterStats nil-guard relocation):** Moved nil-guard from inside `newFilterStats` to the call site in `New` (around csrf.go:36-42), mirroring phase 11 local_ratelimit's pattern at `local_ratelimit.go:204-207` (`var fs *filterStats; if ctx.Stats != nil { fs = newFilterStats(...) }`). Updated `newFilterStats` doc comment: "Caller MUST guarantee `reg != nil`" — references the call-site guard contract and points at the phase 11 precedent line range.
- **M-1 (csrf.go:113-115 inaccurate comment):** Replaced "we read it for documentation but do not capture it into runtimeConfig" with the accurate phrasing: "filter_enabled.default_value's percentage value is silent-ignored at runtime per §1.1 amendment 3 — we do NOT inspect numerator/denominator." (The code does not call `.GetNumerator()`/`.GetDenominator()` anywhere; only checks wrapper presence.)
- **M-3 (doc.go forward-references):** Added one-line forward disclaimer above the Cross-cutting ADR anchors block — `Cross-cutting ADR anchors (ADR-0122/0123/0124 land in phase 12 Tasks 3-4):` — preserves the architectural roadmap visible from `doc.go` without requiring per-task `doc.go` amendments when Tasks 3-4 land. Chose option (a) over option (b)-trim per the brief's instruction.

Skipped (out of scope per the brief): I-3 (defensive `unset_oneof` test row — defer to Task 3 when DecodeHeaders body lands and the test surface broadens), M-2/M-4/M-5/M-6/M-7 (stylistic; defer indefinitely).

Verified all Group 1 + Group 2 tests still pass; build + vet + lint stay clean.

**Outputs:**
```
$ go build ./internal/filter/http/csrf/...
$ go vet ./internal/filter/http/csrf/...
$ golangci-lint run ./internal/filter/http/csrf/...
$ go test -race -count=1 -v ./internal/filter/http/csrf/
=== RUN   TestNew_NilTC
--- PASS: TestNew_NilTC (0.00s)
=== RUN   TestNew_MalformedTC
--- PASS: TestNew_MalformedTC (0.00s)
=== RUN   TestNew_FilterEnabledNil_RejectAtParseTime
--- PASS: TestNew_FilterEnabledNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime
--- PASS: TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored
--- PASS: TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored (0.00s)
=== RUN   TestNew_FilterEnabledHundredPercent_Accepted
--- PASS: TestNew_FilterEnabledHundredPercent_Accepted (0.00s)
=== RUN   TestNew_ShadowEnabledAbsent_Accepted
--- PASS: TestNew_ShadowEnabledAbsent_Accepted (0.00s)
=== RUN   TestNew_ShadowEnabledPresent_SilentIgnored
--- PASS: TestNew_ShadowEnabledPresent_SilentIgnored (0.00s)
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/prefix
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/suffix
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/contains
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/safe_regex
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/ignore_case_with_exact
--- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/prefix (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/suffix (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/contains (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/safe_regex (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/ignore_case_with_exact (0.00s)
=== RUN   TestNew_AdditionalOrigins_EmptyExactValue_Dropped
--- PASS: TestNew_AdditionalOrigins_EmptyExactValue_Dropped (0.00s)
=== RUN   TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm
--- PASS: TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.010s
```

## Task 3 — `DecodeHeaders` body + origin trichotomy + host:port-only equality + reject path

**Commits:** `b9f946b` — `phase 12: csrf DecodeHeaders body + origin trichotomy + host:port equality + reject path [ADR-0122, ADR-0123]`
**Notes:** Filled in `DecodeHeaders` body + `modifyingMethods` map + 5 helpers (`sourceOriginValue`, `targetOriginValue`, `hostAndPort`, `evaluate`, `buildPerRouteRuntime`) per PLAN Task 3. TDD discipline: appended Group 3 (5-method NonModifyingMethods subtests) + Group 4 (5 origin-trichotomy tests) + Group 5 (6 host:port-only-equality tests) + test helpers (`newPostHeaders`, `mustNewListenerFactory`, `freshFilter`, `localReplyArgs`, `fakeCallbacks`) to `csrf_test.go` first; verified Groups 4 + 5 fail as expected against the skeleton (Group 1 + 2 + 3 PASS — Group 3 is the non-modifying-methods short-circuit which the skeleton's blanket `return Continue` happens to satisfy). Then landed the impl in `csrf.go`; all 24 test leaves PASS under `-race -count=1`.

**One PLAN-text deviation noted:** the PLAN test code at lines 845-847 + 866 + 884 + 907 + 922-924 uses `.Add(1)` on counters; the PLAN impl code at lines 1118 + 1121 + 1123 also uses `.Add(1)`. The actual `*stats.Counter` API exposes both `.Inc()` (no-arg, +1) and `.Add(delta uint64)` so `.Add(1)` would compile via untyped-constant conversion. Implementation chose `.Inc()` to match the precedent established in phase 11's `local_ratelimit.go:363-369` which uses `Inc()` for the +1 case (idiomatic Counter usage; `Add(delta)` is reserved for non-unit increments). Tests use `.Load()` (returns `uint64`) directly per the PLAN.

**ADR-0122 + ADR-0123 land at this commit per the ADR-0044 ADR-on-impl convention.** Both follow the ADR-0001 7-section template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). ADR-0122 captures the four interlocked algorithm decisions: (1) 4-method gate `{POST, PUT, DELETE, PATCH}` per §11.1 empirical pin; (2) origin extraction trichotomy per §11.2 — `null` literal → empty NO Referer fallback / Origin empty/absent → Referer fallback / Origin non-empty unparseable → verbatim NO Referer fallback; (3) host:port-only equality per §11.3 + §11.7 — scheme stripped both sides via `hostAndPort()`, NO normalization (case preserved A2/A3, default ports preserved A4, trailing slash stripped via URL parser A7); (4) `additional_origins[].exact` matched against host[:port] form per §11.7 + §11.8 (operator footgun deferred to BEHAVIOR_CONTRACT §13.4); plus (5) synthetic `http://` prefix per planner-time decision 8 + §11.3 amendment for target-origin URL parser acceptance. ADR-0123 captures the rejection wire shape: `SendLocalReply(403, "Invalid origin", OrderedHeaders{Content-Type: text/plain})` — body byte-exact `Invalid origin` (14 bytes ASCII, no LF, MD5 `7433f3a046afcebee10e455dd26b0eb6`), 4-header lowercase wire-form (framework auto-injects 3 of 4 — content-length, date, server), 403 hardcoded status, `StopIteration` from DecodeHeaders, `SendLocalReply` reuse from phase 09 fault precedent (NO new framework primitive). Body literal kept inline at the single call site (NOT promoted to package-level `const`); structurally consistent with phase 11 `rateLimitedBody` which is `const` because of multi-call-site reference + `runtimeConfig.body` indirection — csrf has neither.

**Outputs:**
```
$ go vet ./internal/filter/http/csrf/...
$ golangci-lint run ./internal/filter/http/csrf/...
$ go test -race -count=1 -v ./internal/filter/http/csrf/
=== RUN   TestNew_NilTC
--- PASS: TestNew_NilTC (0.00s)
=== RUN   TestNew_MalformedTC
--- PASS: TestNew_MalformedTC (0.00s)
=== RUN   TestNew_FilterEnabledNil_RejectAtParseTime
--- PASS: TestNew_FilterEnabledNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime
--- PASS: TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored
--- PASS: TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored (0.00s)
=== RUN   TestNew_FilterEnabledHundredPercent_Accepted
--- PASS: TestNew_FilterEnabledHundredPercent_Accepted (0.00s)
=== RUN   TestNew_ShadowEnabledAbsent_Accepted
--- PASS: TestNew_ShadowEnabledAbsent_Accepted (0.00s)
=== RUN   TestNew_ShadowEnabledPresent_SilentIgnored
--- PASS: TestNew_ShadowEnabledPresent_SilentIgnored (0.00s)
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/prefix
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/suffix
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/contains
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/safe_regex
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/ignore_case_with_exact
--- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/prefix (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/suffix (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/contains (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/safe_regex (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/ignore_case_with_exact (0.00s)
=== RUN   TestNew_AdditionalOrigins_EmptyExactValue_Dropped
--- PASS: TestNew_AdditionalOrigins_EmptyExactValue_Dropped (0.00s)
=== RUN   TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm
--- PASS: TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm (0.00s)
=== RUN   TestDecodeHeaders_NonModifyingMethods
=== RUN   TestDecodeHeaders_NonModifyingMethods/GET
=== RUN   TestDecodeHeaders_NonModifyingMethods/HEAD
=== RUN   TestDecodeHeaders_NonModifyingMethods/OPTIONS
=== RUN   TestDecodeHeaders_NonModifyingMethods/TRACE
=== RUN   TestDecodeHeaders_NonModifyingMethods/PROPFIND
--- PASS: TestDecodeHeaders_NonModifyingMethods (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/GET (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/HEAD (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/OPTIONS (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/TRACE (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/PROPFIND (0.00s)
=== RUN   TestDecodeHeaders_OriginNullLiteral_MissingSourceOrigin_NoRefererFallback
--- PASS: TestDecodeHeaders_OriginNullLiteral_MissingSourceOrigin_NoRefererFallback (0.00s)
=== RUN   TestDecodeHeaders_OriginEmpty_RefererFallback
--- PASS: TestDecodeHeaders_OriginEmpty_RefererFallback (0.00s)
=== RUN   TestDecodeHeaders_OriginAbsent_RefererFallback
--- PASS: TestDecodeHeaders_OriginAbsent_RefererFallback (0.00s)
=== RUN   TestDecodeHeaders_OriginAbsent_RefererAbsent_MissingSourceOrigin
--- PASS: TestDecodeHeaders_OriginAbsent_RefererAbsent_MissingSourceOrigin (0.00s)
=== RUN   TestDecodeHeaders_OriginUnparseable_VerbatimUsed
--- PASS: TestDecodeHeaders_OriginUnparseable_VerbatimUsed (0.00s)
=== RUN   TestDecodeHeaders_SameOrigin_HostPortMatch
--- PASS: TestDecodeHeaders_SameOrigin_HostPortMatch (0.00s)
=== RUN   TestDecodeHeaders_CrossOrigin_HostMismatch
--- PASS: TestDecodeHeaders_CrossOrigin_HostMismatch (0.00s)
=== RUN   TestDecodeHeaders_AdditionalOriginsExactMatch
--- PASS: TestDecodeHeaders_AdditionalOriginsExactMatch (0.00s)
=== RUN   TestDecodeHeaders_NoCaseFolding_UppercaseRejected
--- PASS: TestDecodeHeaders_NoCaseFolding_UppercaseRejected (0.00s)
=== RUN   TestDecodeHeaders_NoDefaultPortStripping_PortMismatch
--- PASS: TestDecodeHeaders_NoDefaultPortStripping_PortMismatch (0.00s)
=== RUN   TestDecodeHeaders_TrailingSlashStripped_Allow
--- PASS: TestDecodeHeaders_TrailingSlashStripped_Allow (0.00s)
=== RUN   TestDecodeHeaders_OperatorFootgun_FullURLEntry_NeverMatches
--- PASS: TestDecodeHeaders_OperatorFootgun_FullURLEntry_NeverMatches (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.011s
$ grep -nE '^## ADR-0122|^## ADR-0123' docs/envoy-go/DECISIONS.md
5627:## ADR-0122: Origin extraction trichotomy + host:port-only equality + canonical 4-method gate + `additional_origins[].exact` matched against host[:port] form + scheme-strip discipline via synthetic `http://` prefix
5698:## ADR-0123: Rejection-path wire shape — `SendLocalReply(403, "Invalid origin", {Content-Type: text/plain})` + body byte-exact `Invalid origin` (14 bytes ASCII, no LF) + 4-header lowercase wire-form + 403 hardcoded status + `SendLocalReply` reuse from phase 09 fault precedent
```

## Task 4 — `filterStats` wiring + 3-counter Inc-discipline + per-route shared-stats build + Group 6 unit tests

**Commits:** `5b1b70e` — `phase 12: csrf per-route shared-stats unit tests + 3-counter stat-name discipline [ADR-0124]`
**Notes:** Appended Group 6 (3 tests: `TestDecodeHeaders_PerRouteOverride_DataReplaced`, `TestDecodeHeaders_PerRouteStatsShared_AggregatesAcrossListenerAndPerRoute`, `TestStats_ThreeCountersUnderHCMStatPrefix`) to `csrf_test.go` per PLAN Task 4 lines 1267-1357. **NO production code changes** — Task 3 already landed the per-route shared-stats wiring via `buildPerRouteRuntime` per planner-time decision 5; Task 4 is unit-test confirmation + ADR landing only. All 27 test leaves PASS under `-race -count=1` (Groups 1-5 = 24 leaves; Group 6 adds 3).

**Two PLAN-text deviations noted (both are PLAN verbatim test code that does not compile against the actual `*stats.Counter` / `*stats.Registry` API; impl adapted; no semantic change):**

(a) **`int64` → `uint64` in counter assertions.** PLAN lines 1325-1330 use `int64(2)` / `int64(1)` for the AGGREGATE assertions, but `*stats.Counter.Load()` returns `uint64` (per `internal/stats/counter.go:30`). The phase 11 local_ratelimit precedent at `local_ratelimit_test.go` also uses `uint64` for Load() comparisons. Adapted: `uint64(2)` + `uint64(1)`. The `f.rc.stats.requestValid.Load() != 1` form at PLAN line 1287 + the `f.rc.stats.missingSourceOrigin.Load(); got != 0` form at line 1331 already compile via untyped-constant conversion (no change there). NO semantic change — the assertion compares the same numeric quantities.

(b) **`reg.Counter(name)` → `reg.Walk(...)` set-membership check.** PLAN line 1353 calls `reg.Counter(n)` to look up a counter by name, but `*stats.Registry` exposes NO `Counter(name)` method (the Registry primitives are `NewCounter`, `NewCounterIfAbsent`, `Walk`, and `Freeze` per `internal/stats/registry.go`). Phase 11's `TestStatNames_FourCountersUnderStatPrefix` at `local_ratelimit_test.go:417-440` uses `reg.Walk(func(m stats.Metric) { ... })` to enumerate registered names; Task 4 mirrors this by collecting names into a `map[string]bool` set and asserting set-membership for the 3 expected counter names. NO semantic change — the assertion still verifies that the 3 expected counters were registered (per ADR-0124 §Decision (i) anchor `http.<HCM stat_prefix>.csrf.<name>`).

**ADR-0124 lands at this commit per the ADR-0044 ADR-on-impl convention.** Follows the ADR-0001 7-section template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). §Decision is split into 5 sub-decisions: (i) stat-name discipline + Rule SN2 reuse with NO new SN flattening rule (CONTRAST ADR-0118's SN9 addition for local_ratelimit); (ii) BEHAVIOR_CONTRACT.md 26→29-name extension landing at Task 12; (iii) `shadow_request_invalid` MVP scope-out aligned with §11.6 conclusion (e); (iv) per-route stats SHARED with listener-level — DIVERGENCE FROM PHASE 11 ADR-0117 INDEPENDENT-stats precedent — with the explicit phase-11 contrast paragraph per PLAN line 1371; (v) ADR-0073 wholesale-override applies AS-IS with NO amendment paragraph (phase 11's ADR-0117 amendment + phase 10's ADR-0110 amendment both stay landed and unused by phase 12). 5 alternatives considered (a-e); §Consequences 8 bullets including the `buildPerRouteRuntime` helper as canonical reference for "data-only per-route override + shared stats" implementations.

ADR-0124 is the LAST ADR for phase 12 (per SPEC §8 anticipated ADRs ADR-0120..0124). All 5 anticipated ADRs now landed: ADR-0120 + ADR-0121 (Task 2), ADR-0122 + ADR-0123 (Task 3), ADR-0124 (Task 4).

**Outputs:**
```
$ go vet ./internal/filter/http/csrf/...
$ golangci-lint run ./internal/filter/http/csrf/...
$ go test -race -count=1 -v ./internal/filter/http/csrf/
=== RUN   TestNew_NilTC
--- PASS: TestNew_NilTC (0.00s)
=== RUN   TestNew_MalformedTC
--- PASS: TestNew_MalformedTC (0.00s)
=== RUN   TestNew_FilterEnabledNil_RejectAtParseTime
--- PASS: TestNew_FilterEnabledNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime
--- PASS: TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored
--- PASS: TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored (0.00s)
=== RUN   TestNew_FilterEnabledHundredPercent_Accepted
--- PASS: TestNew_FilterEnabledHundredPercent_Accepted (0.00s)
=== RUN   TestNew_ShadowEnabledAbsent_Accepted
--- PASS: TestNew_ShadowEnabledAbsent_Accepted (0.00s)
=== RUN   TestNew_ShadowEnabledPresent_SilentIgnored
--- PASS: TestNew_ShadowEnabledPresent_SilentIgnored (0.00s)
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/prefix
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/suffix
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/contains
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/safe_regex
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/ignore_case_with_exact
--- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/prefix (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/suffix (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/contains (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/safe_regex (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/ignore_case_with_exact (0.00s)
=== RUN   TestNew_AdditionalOrigins_EmptyExactValue_Dropped
--- PASS: TestNew_AdditionalOrigins_EmptyExactValue_Dropped (0.00s)
=== RUN   TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm
--- PASS: TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm (0.00s)
=== RUN   TestDecodeHeaders_NonModifyingMethods
=== RUN   TestDecodeHeaders_NonModifyingMethods/GET
=== RUN   TestDecodeHeaders_NonModifyingMethods/HEAD
=== RUN   TestDecodeHeaders_NonModifyingMethods/OPTIONS
=== RUN   TestDecodeHeaders_NonModifyingMethods/TRACE
=== RUN   TestDecodeHeaders_NonModifyingMethods/PROPFIND
--- PASS: TestDecodeHeaders_NonModifyingMethods (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/GET (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/HEAD (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/OPTIONS (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/TRACE (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/PROPFIND (0.00s)
=== RUN   TestDecodeHeaders_OriginNullLiteral_MissingSourceOrigin_NoRefererFallback
--- PASS: TestDecodeHeaders_OriginNullLiteral_MissingSourceOrigin_NoRefererFallback (0.00s)
=== RUN   TestDecodeHeaders_OriginEmpty_RefererFallback
--- PASS: TestDecodeHeaders_OriginEmpty_RefererFallback (0.00s)
=== RUN   TestDecodeHeaders_OriginAbsent_RefererFallback
--- PASS: TestDecodeHeaders_OriginAbsent_RefererFallback (0.00s)
=== RUN   TestDecodeHeaders_OriginAbsent_RefererAbsent_MissingSourceOrigin
--- PASS: TestDecodeHeaders_OriginAbsent_RefererAbsent_MissingSourceOrigin (0.00s)
=== RUN   TestDecodeHeaders_OriginUnparseable_VerbatimUsed
--- PASS: TestDecodeHeaders_OriginUnparseable_VerbatimUsed (0.00s)
=== RUN   TestDecodeHeaders_SameOrigin_HostPortMatch
--- PASS: TestDecodeHeaders_SameOrigin_HostPortMatch (0.00s)
=== RUN   TestDecodeHeaders_CrossOrigin_HostMismatch
--- PASS: TestDecodeHeaders_CrossOrigin_HostMismatch (0.00s)
=== RUN   TestDecodeHeaders_AdditionalOriginsExactMatch
--- PASS: TestDecodeHeaders_AdditionalOriginsExactMatch (0.00s)
=== RUN   TestDecodeHeaders_NoCaseFolding_UppercaseRejected
--- PASS: TestDecodeHeaders_NoCaseFolding_UppercaseRejected (0.00s)
=== RUN   TestDecodeHeaders_NoDefaultPortStripping_PortMismatch
--- PASS: TestDecodeHeaders_NoDefaultPortStripping_PortMismatch (0.00s)
=== RUN   TestDecodeHeaders_TrailingSlashStripped_Allow
--- PASS: TestDecodeHeaders_TrailingSlashStripped_Allow (0.00s)
=== RUN   TestDecodeHeaders_OperatorFootgun_FullURLEntry_NeverMatches
--- PASS: TestDecodeHeaders_OperatorFootgun_FullURLEntry_NeverMatches (0.00s)
=== RUN   TestDecodeHeaders_PerRouteOverride_DataReplaced
--- PASS: TestDecodeHeaders_PerRouteOverride_DataReplaced (0.00s)
=== RUN   TestDecodeHeaders_PerRouteStatsShared_AggregatesAcrossListenerAndPerRoute
--- PASS: TestDecodeHeaders_PerRouteStatsShared_AggregatesAcrossListenerAndPerRoute (0.00s)
=== RUN   TestStats_ThreeCountersUnderHCMStatPrefix
--- PASS: TestStats_ThreeCountersUnderHCMStatPrefix (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.011s
$ grep -nE '^## ADR-0124' docs/envoy-go/DECISIONS.md
5757:## ADR-0124: `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 26→29-name extension + 3 csrf counters anchored at HCM stat_prefix (NO new SN flattening rule) + drop `shadow_request_invalid` from MVP + per-route stats SHARED with listener-level (DIVERGES from phase 11 ADR-0117 INDEPENDENT-stats precedent)
```

### Task 4 follow-up — code-review fix-ups (exact-count assertion + wording)

**Commit:** `c3cdd2c` (filled after commit lands).

**Reviewer findings addressed (Important + Minor from Task 4 review):**

1. **Important — `TestStats_ThreeCountersUnderHCMStatPrefix` was MISSING-only-asymmetric:** the
   Walk-based set-membership loop catches MISSING expected counters but would
   silently pass if a 4th unexpected counter (e.g., a future `shadow_request_invalid`
   re-introduction or a debug counter) sneaked into `newFilterStats`. Phase 11's
   analogous `TestStatNames_FourCountersUnderStatPrefix`
   (`internal/filter/http/localratelimit/local_ratelimit_test.go:432-434`)
   asserts BOTH `len(got) != len(want)` AND per-name presence — closing this gap.
   Added `if len(registered) != 3 { t.Errorf(...) }` ahead of the per-name
   presence loop so this test now rejects regressions in EITHER direction
   (extras AND missing).

2. **Minor — misleading "AGGREGATE" wording in single-request test:**
   `TestDecodeHeaders_PerRouteOverride_DataReplaced` (csrf_test.go:413-416)
   issues exactly ONE per-route request and asserts `requestValid == 1`. The
   prior error message said "listener-level stats.requestValid should AGGREGATE
   per-route increments" — but with only a single increment, "AGGREGATE" mis-
   describes the WHY. The actual property under test is that the per-route
   `runtimeConfig` SHARES the listener's `*filterStats` pointer per ADR-0124,
   so a per-route increment lands on the SAME counter the listener-level test
   would see. (True multi-source aggregation is covered by the next test,
   `TestDecodeHeaders_PerRouteStatsShared_AggregatesAcrossListenerAndPerRoute`.)
   Reworded to: `"per-route increment SHARES the listener-level counter; got %d"`.

**Production code untouched** — these are test-only follow-ups. The Important fix
verifies the ALREADY-CORRECT production-side 3-counter discipline more strictly;
the Minor is a pure error-message improvement.

**Outputs:**
```
$ go vet ./internal/filter/http/csrf/
$ golangci-lint run ./internal/filter/http/csrf/...
$ go test -race -count=1 -v ./internal/filter/http/csrf/
=== RUN   TestNew_NilTC
--- PASS: TestNew_NilTC (0.00s)
=== RUN   TestNew_MalformedTC
--- PASS: TestNew_MalformedTC (0.00s)
=== RUN   TestNew_FilterEnabledNil_RejectAtParseTime
--- PASS: TestNew_FilterEnabledNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime
--- PASS: TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored
--- PASS: TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored (0.00s)
=== RUN   TestNew_FilterEnabledHundredPercent_Accepted
--- PASS: TestNew_FilterEnabledHundredPercent_Accepted (0.00s)
=== RUN   TestNew_ShadowEnabledAbsent_Accepted
--- PASS: TestNew_ShadowEnabledAbsent_Accepted (0.00s)
=== RUN   TestNew_ShadowEnabledPresent_SilentIgnored
--- PASS: TestNew_ShadowEnabledPresent_SilentIgnored (0.00s)
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/prefix
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/suffix
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/contains
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/safe_regex
=== RUN   TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/ignore_case_with_exact
--- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/prefix (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/suffix (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/contains (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/safe_regex (0.00s)
    --- PASS: TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/ignore_case_with_exact (0.00s)
=== RUN   TestNew_AdditionalOrigins_EmptyExactValue_Dropped
--- PASS: TestNew_AdditionalOrigins_EmptyExactValue_Dropped (0.00s)
=== RUN   TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm
--- PASS: TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm (0.00s)
=== RUN   TestDecodeHeaders_NonModifyingMethods
=== RUN   TestDecodeHeaders_NonModifyingMethods/GET
=== RUN   TestDecodeHeaders_NonModifyingMethods/HEAD
=== RUN   TestDecodeHeaders_NonModifyingMethods/OPTIONS
=== RUN   TestDecodeHeaders_NonModifyingMethods/TRACE
=== RUN   TestDecodeHeaders_NonModifyingMethods/PROPFIND
--- PASS: TestDecodeHeaders_NonModifyingMethods (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/GET (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/HEAD (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/OPTIONS (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/TRACE (0.00s)
    --- PASS: TestDecodeHeaders_NonModifyingMethods/PROPFIND (0.00s)
=== RUN   TestDecodeHeaders_OriginNullLiteral_MissingSourceOrigin_NoRefererFallback
--- PASS: TestDecodeHeaders_OriginNullLiteral_MissingSourceOrigin_NoRefererFallback (0.00s)
=== RUN   TestDecodeHeaders_OriginEmpty_RefererFallback
--- PASS: TestDecodeHeaders_OriginEmpty_RefererFallback (0.00s)
=== RUN   TestDecodeHeaders_OriginAbsent_RefererFallback
--- PASS: TestDecodeHeaders_OriginAbsent_RefererFallback (0.00s)
=== RUN   TestDecodeHeaders_OriginAbsent_RefererAbsent_MissingSourceOrigin
--- PASS: TestDecodeHeaders_OriginAbsent_RefererAbsent_MissingSourceOrigin (0.00s)
=== RUN   TestDecodeHeaders_OriginUnparseable_VerbatimUsed
--- PASS: TestDecodeHeaders_OriginUnparseable_VerbatimUsed (0.00s)
=== RUN   TestDecodeHeaders_SameOrigin_HostPortMatch
--- PASS: TestDecodeHeaders_SameOrigin_HostPortMatch (0.00s)
=== RUN   TestDecodeHeaders_CrossOrigin_HostMismatch
--- PASS: TestDecodeHeaders_CrossOrigin_HostMismatch (0.00s)
=== RUN   TestDecodeHeaders_AdditionalOriginsExactMatch
--- PASS: TestDecodeHeaders_AdditionalOriginsExactMatch (0.00s)
=== RUN   TestDecodeHeaders_NoCaseFolding_UppercaseRejected
--- PASS: TestDecodeHeaders_NoCaseFolding_UppercaseRejected (0.00s)
=== RUN   TestDecodeHeaders_NoDefaultPortStripping_PortMismatch
--- PASS: TestDecodeHeaders_NoDefaultPortStripping_PortMismatch (0.00s)
=== RUN   TestDecodeHeaders_TrailingSlashStripped_Allow
--- PASS: TestDecodeHeaders_TrailingSlashStripped_Allow (0.00s)
=== RUN   TestDecodeHeaders_OperatorFootgun_FullURLEntry_NeverMatches
--- PASS: TestDecodeHeaders_OperatorFootgun_FullURLEntry_NeverMatches (0.00s)
=== RUN   TestDecodeHeaders_PerRouteOverride_DataReplaced
--- PASS: TestDecodeHeaders_PerRouteOverride_DataReplaced (0.00s)
=== RUN   TestDecodeHeaders_PerRouteStatsShared_AggregatesAcrossListenerAndPerRoute
--- PASS: TestDecodeHeaders_PerRouteStatsShared_AggregatesAcrossListenerAndPerRoute (0.00s)
=== RUN   TestStats_ThreeCountersUnderHCMStatPrefix
--- PASS: TestStats_ThreeCountersUnderHCMStatPrefix (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.012s
```

## Task 5 — `FuzzCsrfPolicyConfigParse` fuzzer

**Commits:** `d87afe4` — `phase 12: FuzzCsrfPolicyConfigParse (16th fuzzer; 30s budget green)`
**Notes:** Lands the SIXTEENTH fuzzer overall (post phase-11's fifteenth `FuzzLocalRateLimitConfigParse`) per SPEC §14.3 + ADR-0018's "every parser/codec/filter ships a fuzzer" discipline. ~70 LoC. Fuzzes arbitrary `[]byte` sequences as the `Value` of a `*anypb.Any` (with fixed canonical `TypeURL`) passed to `New`. Asserts: never panic; never return `(nil, nil)`; on factory-success the factory invokes without panic AND `hf.Decoder` is non-nil. Seed corpus: 3 well-formed `*csrfv3.CsrfPolicy` proto-marshalled entries (a) minimal `filter_enabled` 100/HUNDRED; (b) same with mixed `StringMatcher` variants exercising the parse-time-drop path (Exact + Prefix + empty-Exact); (c) empty `CsrfPolicy` (missing `filter_enabled` — must reject cleanly). 30s budget per ADR-0018 short-mode CI policy. **Deviation from PLAN-verbatim code** (lines 1444-1458): renamed local variable `any` → `tc` to avoid shadowing Go 1.18+'s predeclared `any` (alias for `interface{}`); linters (`predeclared`) commonly flag this. NO semantic change — same `*anypb.Any` value passed to `New`. No new ADR (ADR-0018 already covers fuzzer discipline). Reviews: skipped subagent dispatch — fuzzer parameters mechanical; the 30s execution + the seed-corpus regression run verify correctness.
**Outputs:**
```
$ go test -fuzz=FuzzCsrfPolicyConfigParse -fuzztime=1s ./internal/filter/http/csrf/
fuzz: elapsed: 0s, gathering baseline coverage: 0/3 completed
fuzz: elapsed: 0s, gathering baseline coverage: 3/3 completed, now fuzzing with 32 workers
fuzz: elapsed: 1s, execs: 15288 (14812/sec), new interesting: 37 (total: 40)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.042s
$ go test -fuzz=FuzzCsrfPolicyConfigParse -fuzztime=30s ./internal/filter/http/csrf/
fuzz: elapsed: 0s, gathering baseline coverage: 0/40 completed
fuzz: elapsed: 0s, gathering baseline coverage: 40/40 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 294741 (98247/sec), new interesting: 108 (total: 148)
fuzz: elapsed: 6s, execs: 859200 (188107/sec), new interesting: 132 (total: 172)
fuzz: elapsed: 9s, execs: 1326404 (155730/sec), new interesting: 145 (total: 185)
fuzz: elapsed: 12s, execs: 1616074 (96578/sec), new interesting: 148 (total: 188)
fuzz: elapsed: 15s, execs: 1699130 (27678/sec), new interesting: 150 (total: 190)
fuzz: elapsed: 18s, execs: 2143213 (148060/sec), new interesting: 154 (total: 194)
fuzz: elapsed: 21s, execs: 2729056 (195156/sec), new interesting: 161 (total: 201)
fuzz: elapsed: 24s, execs: 3262535 (177928/sec), new interesting: 163 (total: 203)
fuzz: elapsed: 27s, execs: 4080049 (272492/sec), new interesting: 169 (total: 209)
fuzz: elapsed: 30s, execs: 4770588 (230163/sec), new interesting: 175 (total: 215)
fuzz: elapsed: 31s, execs: 4770588 (0/sec), new interesting: 175 (total: 215)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	31.062s
$ go vet ./internal/filter/http/csrf/...
$ golangci-lint run ./internal/filter/http/csrf/...
$ go test ./internal/filter/http/csrf/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	0.003s
$ grep -rE '^func Fuzz' --include='*_test.go' internal/ test/ | wc -l
16
```

