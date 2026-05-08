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
