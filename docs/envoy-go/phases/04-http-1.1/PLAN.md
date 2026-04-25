# Phase 04 — HTTP/1.1 (HCM + route match + router + direct_response) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants), §5 (state machine), §6 (splitting), §7 (differential contract); `docs/envoy-go/phases/04-http-1.1/SPEC.md` (authoritative scope — every PLAN decision below traces to a SPEC section); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0036 — especially **ADR-0003** branch convention, **ADR-0004** autonomous brainstorming adaptation, **ADR-0005** autonomous plan-review adaptation, **ADR-0008** Envoy v1.37.2 pin, **ADR-0010** V4_ONLY DNS rule, **ADR-0013** go-control-plane proto-types-only pin, **ADR-0014** `Server: envoy` admin header value, **ADR-0015** admin `/ready` Date allow-list, **ADR-0016** bootstrap loader unknown-field policy + blank-import amendment policy, **ADR-0018** fuzz CI budget, **ADR-0019** `expectations.yaml` prose form deferred, **ADR-0023** phase-00 pump lift, **ADR-0027** STATIC-vs-STRICT_DNS divergence, **ADR-0028** reference-side `--concurrency 1` pin, **ADR-0031** stdlib `crypto/tls` stack, **ADR-0032** `Cluster.Dial(ctx)` upstream dialer, **ADR-0033** phase-03 filter-chain subset, **ADR-0034** fixture-driver interface split, **ADR-0036** BEHAVIOR_CONTRACT TLS subsection); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (existing `## Equivalence Matrix`, `## Header allow-list`, `## Admin API — /ready`, `## Test harness host networking`, `## TCP proxy`, `## TLS` subsections — phase 04 adds a new `## HTTP/1.1` subsection and appends rows to `## Header allow-list`); `docs/envoy-go/phases/03-tls/PLAN.md` and `PROGRESS.md` (style reference for tasks, atomic per-task commits, PROGRESS conventions, ADR-with-first-use-commit discipline); `docs/envoy-go/phases/03-tls/REVIEW.md` (the 4 Important + 8 Minor findings — SPEC §12 triages the Minors; phase 04 resolves M-4 and M-5 opportunistically only if `internal/cluster/` is touched, which it is **not** per SPEC §4.2; all 8 Minors carry forward).

**Goal:** Land envoy-go's first HTTP-aware dataplane — a new `internal/filter/hcm/` package decomposing into six files (`doc.go`, `codec.go`, `route.go`, `actions.go`, `connection.go`, `config.go`, `filter.go`) that together parse the Envoy v3 `HttpConnectionManager` proto from a network-filter `typed_config` Any, validate a router-only HTTP-filter chain, resolve an inline `route_config`'s single virtual_host's routes into an in-memory route table supporting `match.prefix` and `match.path` predicates, dispatch matched requests through `direct_response` (synthesized local reply) or `route` (router action) — the latter dialing the named cluster via the phase-03 `Cluster.Dial(ctx)` per-request-fresh, writing the request via stdlib `http.Request.Write`, reading the response via `http.ReadResponse`, and streaming the response back through the downstream `bufio.Writer`; an extension to `internal/listener/manager.go`'s inline `filterRegistry` map registering the HCM type_url alongside the phase-02 `tcp_proxy` entry; an addition of three blank imports to `internal/bootstrap/bootstrap.go` (HCM, Router, route-config protos) per ADR-0016's amendment policy; an amendment to the phase-00 `internal/http/doc.go` placeholder pointing at the real implementation under `internal/filter/hcm/`; an additive optional-method extension to the `test/differential/fixture.Driver` interface (`HTTPExpectations() []HTTPRequestExpectation`) plus a per-request orchestration pass in `test/differential/runner_test.go` that opts in when the driver implements it; new `test/helpers/http.go` (`HTTPRoundTrip`) and `test/helpers/http_diff.go` (`HTTPHeaderDiff`) helpers mirroring the phase-03 `tls.go`/`tcp.go` style; a new HTTP/1.1 echo backend type alongside the existing TCP echo backend in the runner's per-fixture spawning code; a new differential fixture `0003-http11-routing` exercising three routes (`/health` → direct_response, `/api/*` → router → 3-endpoint cluster, catch-all → 404 direct_response) with 27 requests per side (9 per route × 3 routes) and a `[3,3,3]` per-cluster RR distribution assertion; a new `FuzzHCMConfigParse` short-budget fuzz target; a `cmd/envoy-go/main_test.go` HCM bootstrap variant verifying the binary serves an HCM-routed response on a localhost probe; a new `## HTTP/1.1` BEHAVIOR_CONTRACT subsection codifying the equivalence surface plus extended Header allow-list rows for `Content-Length`/`Transfer-Encoding`/`x-envoy-*`/`x-forwarded-*`/`x-request-id`; and eight ADRs (`ADR-0037`..`ADR-0044`) — satisfying every gate in `docs/envoy-go/phases/04-http-1.1/SPEC.md` §3.

**Architecture:** `internal/filter/hcm/` decomposes into seven source files with orthogonal responsibility — `doc.go` (package overview + import discipline), `codec.go` (small bufio-writer helpers `writeStatusReply`, `serverHeader`, `dateHeader` — the only path that locally generates a response body), `route.go` (`routeTable` + `routeEntry` + `routeMatch` interface with `matchPrefix(string)` and `matchPath(string)` implementations + `match(*http.Request) (*routeEntry, bool)` first-match-wins evaluator), `actions.go` (`routeAction` interface with `directResponseAction{status, body}` and `routerAction{cluster *cluster.Cluster}` implementations — direct_response calls `writeStatusReply`, router calls `cluster.Dial(ctx)` then `req.Write` then `http.ReadResponse` then `resp.Write`, with per-failure-class local-reply mapping `Cluster.Dial`→503, `req.Write`→502, `http.ReadResponse`→502), `connection.go` (`runConnection(ctx, downstream, *routeTable)` — the per-conn loop that reads requests via `http.ReadRequest` off a `bufio.Reader`, applies phase-04 out-of-scope guards [`Expect:`→417, `Upgrade:`/`Connection: Upgrade`→501], dispatches matched requests, drains `req.Body` between iterations, honours `Connection: close` on either request or response via a sentinel error returned by `routeAction.do`), `config.go` (typed_config decoder — validates HCM type_url, codec_type ∈ {HTTP1, AUTO}, mandatory `stat_prefix`, exactly-one router-named-and-typed entry in `http_filters[]`, exactly-one vhost with `domains: ["*"]`, errors on every disallowed-shape per SPEC §2, silently passes through every ignored-field per SPEC §9), and `filter.go` (`Filter` struct + `NewFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error)` orchestrating the config decode then `Filter.Handle(ctx context.Context, downstream net.Conn)` matching the existing `filterHandler` interface in `internal/listener/manager.go:33-38` — note `Handle` returns no error to match phase-02's `tcpproxy.Filter.Handle` shape exactly). `internal/listener/manager.go`'s `filterRegistry` map (lines 41–51) gains one new entry mapping `"type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"` to a constructor that calls `hcm.NewFilter` and returns a `filterHandler` — the listener manager's existing per-chain build-error wrap discipline (`listener: %q: filter_chains[%d]: %w`) automatically nests `hcm:` errors at the right depth. `internal/bootstrap/bootstrap.go` gains three blank-import lines under the existing comment block (lines 8–14) for `…/extensions/filters/network/http_connection_manager/v3`, `…/extensions/filters/http/router/v3`, `…/config/route/v3` — the additions are documented in PROGRESS per ADR-0016 (no new ADR for the blank-imports themselves; ADR-0040 records the broader HTTP-filter-chain shape decision that the proto packages support). The phase-00 `internal/http/doc.go` placeholder is amended (not deleted — settles SPEC §10 #10 via the conservative recommendation in master STATE.md) to read "Phase-04 implementation lives at `internal/filter/hcm/`. This package remains as a stable import target if future code needs HTTP utilities that are not HCM-specific." The fixture `Driver` interface gains an OPTIONAL method `HTTPExpectations() []HTTPRequestExpectation` (additive — the type-assertion-at-orchestration-site pattern already established by `DistributionAsserter` at `runner_test.go:140-145` is the model). The runner adds a per-request orchestration pass: when the driver implements `HTTPExpectations`, after `DriveReference` and `DriveSubject` complete and after the byte-comparison + distribution assertion run, the runner re-issues each `HTTPRequestExpectation`'s request once against ref and once against subject through `helpers.HTTPRoundTrip`, comparing status + decoded body + header set (under `helpers.HTTPHeaderDiff`'s phase-04 allow-list). Fixture `0003-http11-routing` registers via the phase-02 `fixture.RegisterFixture` mechanism (mirror of fixtures 0001/0002), with `BackendCount=3`, `SubjectListenerName="l_http"`, `ReferenceListenerPort=15003` (sequential after 0002's 15002), `DriveReference`/`DriveSubject` issuing 27 requests per side via fresh-per-request `net.Dial` (so per-request connections form the basis of the RR distribution count, deterministic via reference-side `--concurrency 1` from ADR-0028), and `AssertDistribution` checking `[3,3,3]` exactly across the 9 router-action requests. The HTTP echo backends spawn alongside the TCP echo backends in `runner_test.go`'s per-fixture setup; each HTTP backend reads one `http.ReadRequest`, writes one `HTTP/1.1 200 OK` response with `Content-Length`-framed body `backend-<idx>:<n>` where `<idx>` is the backend index 0..2 and `<n>` is the request URL path's last component, then closes — the `backend-<idx>:` prefix is what `AssertDistribution` counts on. The phase-04 BEHAVIOR_CONTRACT subsection codifies four asserted equivalence dimensions (response status, decoded body for routed-to-upstream and direct_response paths, route-match selection witness, upstream-side request preservation) and five non-asserted dimensions (response-header set is compared modulo the extended allow-list, local-reply body bytes, framing choice, upstream connection re-use, x-envoy-/x-forwarded-/x-request-id header injection). Eight ADRs land at execution time, mapped from SPEC §4.4's anticipated ADR-H..ADR-O letters to sequential numbers ADR-0037..ADR-0044 (see `## ADRs introduced by this plan` below — first-use-commit ordering per phase-02/03 precedent).

**Tech Stack:**
- Go 1.23 (unchanged from phase 03; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `net/http` (`ReadRequest`, `ReadResponse`, `Request.Write`, `Response.Write`, `StatusText`, `TimeFormat`, `Header`), `bufio`, `context`, `io`, `net`, `strings`, `time`, `errors`, `fmt`, `log` — phase-04 wire codec consumes only stdlib parsers/serializers; **no `net/http.Server` and no `http.Handler`** (D-3.2 boundary; SPEC §1 narrative).
- `github.com/envoyproxy/go-control-plane/envoy` at v1.32.4 (ADR-0013 pin, unchanged). Phase 04 takes typed imports of:
  - `…/extensions/filters/network/http_connection_manager/v3` (`HttpConnectionManager`, `HttpFilter`, `HttpConnectionManager_RouteConfig`)
  - `…/extensions/filters/http/router/v3` (`Router`)
  - `…/config/route/v3` (`RouteConfiguration`, `VirtualHost`, `Route`, `RouteMatch`, `RouteAction`, `DirectResponseAction`)
- `google.golang.org/protobuf/types/known/anypb` (Any unmarshal of HCM typed_config and Router typed_config — same pattern as phase-02 `tcpproxy.NewFilter`).
- `github.com/envoyproxy/go-control-plane/envoy/config/core/v3` (`DataSource` for `direct_response.body` — the `inline_string` specifier is the only one phase-04 honours; SPEC §2 errors on `inline_bytes` and `filename` for `direct_response.body`).
- Existing `internal/cluster` package (`Manager`, `Cluster`, `Cluster.Dial(ctx) (net.Conn, error)`, `Manager.Get(name)`) — unchanged consumer; the router action calls `cluster.Dial(ctx)` exactly as the phase-03 `tcpproxy` filter does.
- `github.com/testcontainers/testcontainers-go` for the differential harness (consumed via `test/differential/harness.go`; phase 04 does not modify the harness, only adds an HTTP echo backend type alongside the existing TCP echo backend in `runner_test.go`).
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- Upstream Envoy v1.37.2 @ `sha256:c5e8a68e…` (ADR-0008, consumed not modified); `--concurrency 1` inheritance from ADR-0028 (verified at Task 1's preconditions).

---

## Scope check — why phase 04 ships as one phase, not 04.1 + 04.2 (+ 04.x HTTPS)

Net change estimate: **~2400 LoC** (~750 new production code under `internal/filter/hcm/` including tests; ~50 listener-manager extension; ~15 bootstrap blank-imports; ~10 internal/http/doc.go amend; ~80 fixture/runner extension for HTTPExpectations; ~120 test/helpers HTTP round-trip + diff with tests; ~150 HTTP echo backend in runner; ~700 fixture 0003 including YAMLs/driver/test/README; ~150 BEHAVIOR_CONTRACT additions; ~400 across the eight ADRs in DECISIONS.md). The split-gate threshold is **~1500 LoC OR ~25 numbered tasks** (`BOOTSTRAP_PROMPT.md` §6.1); the estimate exceeds the LoC threshold. Task count is 17 — well below the 25 gate.

Phase 04 ships as **one** phase (not split into 04.1 foundation + 04.2 wiring, and not split off an 04.x HTTPS sub-phase), for the same three reasons phase 03 shipped as one phase, plus a fourth specific to HTTP:

1. **Atomic-claim cohesion (SPEC §1).** The phase's central claim is: *envoy-go speaks HTTP/1.1 — it parses requests, matches them against a configured route table, and either synthesizes a local reply or proxies to an upstream HTTP/1.1 cluster — on a deterministic workload that produces byte-equivalent decoded response bodies to upstream Envoy's.* A split where 04.1 ships `internal/filter/hcm/` (parse + route + actions in isolation) and 04.2 ships fixture + BEHAVIOR_CONTRACT weakens this claim — 04.1 would have only unit-test evidence for a package whose end-to-end path is unexercised by any differential fixture, and SPEC §3 gate (a) ("new/changed differential fixtures green") could not be satisfied in 04.1. `BOOTSTRAP_PROMPT.md` §6.3 explicitly warns against shipping incomplete stubs that differential tests can't exercise. A 04.1 / 04.2 split would ship exactly that.

2. **No clean half-fixture seam.** The alternative split — 04.1: `internal/filter/hcm/` + listener registration + bootstrap blank imports + helpers + runner extension; 04.2: fixture 0003 + driver + BEHAVIOR_CONTRACT — has a slightly better shape (04.1 ends with unit-test green + updated 0000/0001/0002 fixtures regression-free; 04.2 lights up 0003). But 04.1 would still ship a *dataplane whose new code paths are unexercised end-to-end by any differential fixture* — the HCM, the router, the route match, and the new helpers would all be unit-tested only. SPEC §3 gate (a) would partially satisfy (via 0000/0001/0002 regression-freeness), but the phase's *primary* new surface — HTTP/1.1 routing — would be unasserted end-to-end until 04.2. This is the same anti-pattern in a different wrapper — and is exactly the same shape phase 03 considered and rejected.

3. **Mid-execution split valve is preserved.** `BOOTSTRAP_PROMPT.md` §6.1's secondary trigger ("if any single task's sub-steps blow up past ~10 items once contact with reality reveals complexity") stays active. The two tasks most likely to blow past 10 sub-steps are Task 7 (`config.go` typed_config parse — large surface, three landing ADRs) and Task 15 (fixture 0003, which integrates every prior task end-to-end). If either exceeds 15 sub-steps at execution time, the executor splits per §6.2 with an ADR. That is a real release valve — the executor does not need permission to invoke it.

4. **HTTPS sub-phase deferral is not a current-phase split.** SPEC §2's HTTPS deferral target is "phase 04.x or 05.x or a dedicated HTTPS-fixture sub-phase, depending on phase-05's planning split." Pulling HTTPS into phase 04.1 + 04.2 simply because HTTPS is "obvious next step" would either (a) require a TLS-capable HTTP echo backend infrastructure (which is a non-trivial helper layer phase 04 does not need for plaintext fixture 0003) or (b) ship without that infrastructure and produce a half-tested HTTPS path. SPEC §2 explicitly leaves the HTTPS sub-phase choice to the future planner; phase 04 does not preempt that decision.

**Triggering re-evaluation:** if at execution time the cumulative landed-LoC count exceeds **3500** by the end of Task 14 (i.e., before the fixture 0003 + BEHAVIOR_CONTRACT + verification sweep), invoke `superpowers:systematic-debugging` on the estimate-vs-reality gap and re-evaluate the split decision. A 45% estimate miss on a carefully-bounded phase is a signal that the plan's shape is wrong, not just that the work is large.

---

## File Structure

| Path | Created/Modified/Deleted | Purpose |
|---|---|---|
| `internal/filter/hcm/doc.go` | Create | Package doc — phase-04 surface (HTTP connection manager + route match + router + direct_response over HTTP/1.1 only); references SPEC §4.1, ADR-0037 (wire codec source), ADR-0040 (HTTP-filter framework subset), ADR-0042 (HTTP-filter chain shape `[router]`), ADR-0044 (BEHAVIOR_CONTRACT). Documents that all errors begin with `hcm: ` and that the package does NOT use `net/http.Server` / `http.Handler` (D-3.2 boundary). |
| `internal/filter/hcm/codec.go` | Create | Wire-codec helpers: `serverHeader() string` returns `"envoy"` (ADR-0014 reaffirmed for HCM-locally-generated responses, settles SPEC §10 #12); `dateHeader() string` returns `time.Now().UTC().Format(http.TimeFormat)` (RFC 7231 IMF-fixdate; settles SPEC §10 #8 to (a) per-response, no caching); `writeStatusReply(w io.Writer, status int, body string) error` writes the locally-generated HTTP/1.1 response (status line, `Content-Type: text/plain`, `Content-Length`, `Server: envoy`, `Date: <imf-fixdate>`, blank line, body). The reason phrase comes from `http.StatusText(status)`; if zero/unknown, falls back to empty string. |
| `internal/filter/hcm/codec_test.go` | Create | Golden-string assertions for `writeStatusReply` outputs at status 200, 400, 404, 417, 500, 502, 503; verify Date is RFC 7231 IMF-fixdate parseable; verify Server is exactly `envoy`; verify Content-Length matches body length; verify body bytes appear after the blank line. Also: `serverHeader()` returns `"envoy"`; `dateHeader()` returns parseable IMF-fixdate. |
| `internal/filter/hcm/route.go` | Create | Route table + match engine. `type routeMatch interface { matches(string) bool }` with two implementations `matchPrefix string` (bytewise prefix; phase-04 documented divergence from Envoy's segment-aware semantics — codified in ADR-0038) and `matchPath string` (case-sensitive exact equality). `type routeAction interface { ... }` is forward-declared but defined in `actions.go` (Go's interface satisfaction is structural, no cyclic-import problem; the interface declaration lives next to the action types in actions.go and route.go uses an interface assertion). `type routeEntry struct { match routeMatch; action routeAction }`. `type routeTable struct { routes []routeEntry }`. `(t *routeTable) match(req *http.Request) (*routeEntry, bool)` walks routes in declaration order; first matching predicate wins; matches on `req.URL.Path` only (query string excluded; SPEC §5.4). All path comparisons are case-sensitive (Envoy default). Settles SPEC §10 #1 to interface-based representation (recommended option (a)). |
| `internal/filter/hcm/route_test.go` | Create | Exhaustive table-driven tests: `matchPath` exact match + non-match; `matchPrefix` boundary-aware (prefix `/api` matches `/api`, `/api/`, `/api/v1` and ALSO matches `/apifoo` per the phase-04 documented divergence); first-match-wins ordering (e.g., `[matchPath("/health"), matchPrefix("/")]` resolves `/health` to the path entry); query-string-excluded-from-match (`/api?q=1` matches `prefix: /api`); empty route table returns `false`; `matchPath` case-sensitive (rejects `/HEALTH` against `path: /health`). |
| `internal/filter/hcm/actions.go` | Create | Two action implementations + the action-interface contract. `type routeAction interface { do(ctx context.Context, req *http.Request, bw *bufio.Writer) error }`. Returning the package-level sentinel `errCloseAfterAction` from `do` signals the connection loop to close after this iteration (settles SPEC §10 #3 to recommended option (a) — sentinel error). Other errors propagate through the connection loop and trigger downstream close. `type directResponseAction struct { status int; body string }` — `do` calls `codec.writeStatusReply(bw, a.status, a.body)`; never returns `errCloseAfterAction` (direct_response participates in keep-alive). `type routerAction struct { cluster *cluster.Cluster }` — `do` opens `upstream, err := a.cluster.Dial(ctx)` (503 on err); `defer upstream.Close()`; `req.Write(upstream)` (502 on err); `resp, err := http.ReadResponse(bufio.NewReader(upstream), req)` (502 on err); `defer resp.Body.Close()`; `resp.Write(bw)` propagates any I/O error. The router does NOT inject `x-envoy-*`, `x-forwarded-*`, or `x-request-id` headers (SPEC §2). Per-request fresh dial is codified in ADR-0039 (per-request fresh upstream dial — connection pooling deferred to upstream-robustness family). |
| `internal/filter/hcm/actions_test.go` | Create | `directResponseAction.do` writes the expected status line + body (golden via `writeStatusReply` shape). `routerAction.do` against a loopback HTTP/1.1 echo server (a one-shot helper running an HTTP/1.1 backend in-test): sends `GET /x HTTP/1.1`, asserts the echo round-trips (status 200, body `backend-test:x`, etc.); ctx cancellation while waiting on upstream dial returns 503 to the bufio writer (the `cluster.Dial(ctx)` error is propagated as a 503 local reply); upstream that closes mid-request triggers 502; upstream that returns malformed bytes triggers 502; the routerAction does NOT inject `x-envoy-*`/`x-forwarded-*`/`x-request-id` (verified by reading what arrives at the upstream). |
| `internal/filter/hcm/connection.go` | Create | Per-conn driver. `func runConnection(ctx context.Context, downstream net.Conn, table *routeTable)`. Wraps downstream in `bufio.NewReader(downstream)` + `bufio.NewWriter(downstream)`. Loops: `req, err := http.ReadRequest(br)` — `io.EOF`-only causes clean exit; any other error → `writeStatusReply(bw, 400, "")` + flush + return. Out-of-scope guards: `req.Header.Get("Expect") != ""` → 417 + flush + return; `req.Header.Get("Upgrade") != ""` OR `strings.EqualFold(req.Header.Get("Connection"), "Upgrade")` → 501 + flush + return. Determine `closeAfterRequest := strings.EqualFold(req.Header.Get("Connection"), "close")`. Match: `entry, ok := table.match(req)` — on `!ok` write 404 (no body content beyond the local-reply body permitted by ADR-0044) + flush. Otherwise `actErr := entry.action.do(ctx, req, bw)`; flush. If `actErr == errCloseAfterAction` set `closeAfterAction := true`; if `actErr != nil` (and not the sentinel) the connection loop logs (`hcm: action: %v`) and returns (closes downstream). After flush: drain `req.Body` (`io.Copy(io.Discard, req.Body); req.Body.Close()`) — mandatory for HTTP/1.1 keep-alive correctness. If `closeAfterRequest || closeAfterAction` return. |
| `internal/filter/hcm/connection_test.go` | Create | Loopback TCP-pair tests (no docker): keep-alive happy path (two requests on one conn against an in-test routeTable that uses `directResponseAction{200, "OK\n"}` for `path:/health`); `Connection: close` on request closes the conn after one request; bad request line returns 400 + closes; `Expect: 100-continue` returns 417 + closes; `Upgrade: websocket` returns 501 + closes; `Connection: Upgrade` returns 501 + closes; route-not-found returns 404; chunked request body is consumed via `io.Copy(io.Discard, req.Body)` before next iteration; pipelined two-request stream where both succeed. The actions used in tests are plumbed via a small helper that builds an in-memory routeTable from a `[]routeEntry` literal so tests do not depend on the proto path. |
| `internal/filter/hcm/config.go` | Create | Typed_config parser. `func parseFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error)`: validates the type_url is exactly `type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager`; unmarshals to `*hcmv3.HttpConnectionManager`. Reads `codec_type` (must be `HTTP1` or `AUTO`; `HTTP2`/`HTTP3` error with `hcm: codec_type %q is not supported in phase 04 (HTTP/1.1 only)`). Reads `stat_prefix` (must be non-empty; mandatory). Validates `route_specifier`: must be `*hcmv3.HttpConnectionManager_RouteConfig` (inline); `*hcmv3.HttpConnectionManager_Rds` errors; `*hcmv3.HttpConnectionManager_ScopedRoutes` and `*hcmv3.HttpConnectionManager_ScopedRds` error. Validates `route_config.virtual_hosts[]` has exactly one entry whose `domains[]` is exactly `["*"]`. Validates `http_filters[]` has exactly one entry with `name == "envoy.filters.http.router"` AND its `typed_config.type_url == "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"` (the Router proto body is unmarshalled but every Router-proto field is silently ignored — phase-04 ignored-set codified in ADR-0041). For each `routes[i]`: decode `match.path_specifier` — `*routev3.RouteMatch_Path` → `matchPath(p.Path)`; `*routev3.RouteMatch_Prefix` → `matchPrefix(p.Prefix)`; any other variant → build error. Validate `match.case_sensitive` — if explicitly set to `false` → error; otherwise (unset = nil pointer, or set to `true`) accepted. Reject every other `match` field set (headers, query_parameters, runtime_fraction, dynamic_metadata, tls_context, connect_matcher) with explicit per-field errors. Decode `action`: `*routev3.Route_Route` (router action) → validate `route.cluster_specifier` is `*routev3.RouteAction_Cluster` and the named cluster exists in `clusters.Get(name)` (404-style "phase 04 supports only literal cluster names" error otherwise); `*routev3.Route_DirectResponse` → validate status ∈ [100, 599], `body.specifier` is `*corev3.DataSource_InlineString` (else error), `body.GetInlineString() != ""`. Any other `action` variant errors. The Filter holds `routeTable`, `clusters`, `statPrefix string`. Settles SPEC §10 #2 (action representation) to interface-based (matches §10 #1's choice). Settles SPEC §10 #5 to **explicit catch-all** in the fixture (so config.go does NOT special-case the "no match" path beyond what `connection.go` handles; the fixture supplies `prefix: "/"` → 404 direct_response). |
| `internal/filter/hcm/config_test.go` | Create | Happy: well-formed HCM Any with one direct_response + one router route + router-only http_filters list — successful parse. Build errors (one test per error class): wrong type_url; codec_type=HTTP2 → error; codec_type=HTTP3 → error; codec_type=AUTO → ok (alias for HTTP1); missing stat_prefix → error; non-`route_config` route_specifier (Rds) → error; route_config with 0 vhosts → error; route_config with 2 vhosts → error; vhost with `domains: []` → error; vhost with `domains: ["example.com"]` → error; vhost with `domains: ["*", "example.com"]` → error; http_filters empty → error; http_filters with 2 entries → error; http_filters[0].name != "envoy.filters.http.router" → error; http_filters[0].typed_config.type_url != Router → error; route with unknown action variant → error; route with `RouteMatch_SafeRegex` → error; route with `RouteMatch_PathSeparatedPrefix` → error; route with `RouteMatch_ConnectMatcher` → error; route with `case_sensitive: false` → error; route with `match.headers != nil` → error; route with `match.query_parameters != nil` → error; route with `match.runtime_fraction != nil` → error; direct_response with `status: 0` → error; direct_response with `status: 600` → error; direct_response with `body.inline_bytes` → error; direct_response with `body.filename` → error; direct_response with empty `inline_string` → error; route action with `cluster_specifier_plugin` → error; route action with `cluster_header` → error; route action with `weighted_clusters` → error; route action `route.cluster` referencing unknown cluster → error. Each error message verified to begin with `hcm:`. |
| `internal/filter/hcm/filter.go` | Create | Top-level `Filter` type + constructor. `const TypeURL = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"`. `type Filter struct { table *routeTable; clusters *cluster.Manager; statPrefix string }`. `func NewFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error)` calls `parseFilter` (from config.go) and returns. `func (f *Filter) Handle(ctx context.Context, downstream net.Conn)` matches the `internal/listener.filterHandler` interface (no error return — phase-02 `tcpproxy.Filter.Handle` precedent). Body: `defer downstream.Close()`; `if err := ctx.Err(); err != nil { return }`; `runConnection(ctx, downstream, f.table)`. The connection loop owns all error logging. The `statPrefix` field is stored unused (forward-look for phase 06 stats per ADR-0041; settles SPEC §10 #9 to recommended option (a) — `string` field on `Filter`). |
| `internal/filter/hcm/filter_test.go` | Create | `NewFilter` happy path (one direct_response + one router route, valid Router typed_config); `NewFilter` returns the same set of errors `parseFilter` does (one smoke test asserting the prefix-based error wrap is intact); `Filter.Handle` against a real loopback `net.Listener` exercising a one-request-then-EOF flow ending in clean close. The exhaustive build-error coverage lives in `config_test.go`; this file's job is the public-API contract surface. |
| `internal/filter/hcm/fuzz_test.go` | Create | `FuzzHCMConfigParse(f *testing.F)`. Seed corpus (3 entries, settles SPEC §4.1 corpus shape): (a) well-formed HCM Any with one direct_response (`/health` → 200 `OK\n`) + one router route (`prefix: /api` → cluster `c_test`) + router-only http_filters; (b) truncated Any bytes (an empty `[]byte`); (c) Any with wrong type_url (`type.googleapis.com/google.protobuf.StringValue`). Fuzz body: build a tiny single-cluster `*cluster.Manager` (one STATIC cluster `c_test` at `127.0.0.1:1`); call `NewFilter(any, cm)` against the mutated bytes; assert no panic; assert every returned error begins with `hcm:`. Short-budget `-fuzztime=30s` per ADR-0018 (precedent inherited from phase-01/02/03). |
| `internal/listener/manager.go` | Modify | The `filterRegistry` map (lines 41–51) gains one new entry mapping the HCM type_url to a constructor that calls `hcm.NewFilter` and adapts the `*hcm.Filter` return to a `filterHandler`. The phase-02 `tcpproxy.TypeURL` entry is unchanged. The plaintext-listener single-filter-chain rule (lines 196–200) is unchanged: phase 04 fixture 0003 uses one filter chain with empty `filter_chain_match`, which already passes. The all-TLS-or-all-plaintext rule (lines 189–195) is unchanged. The error-wrap discipline (`listener: %q: filter_chains[%d]: %w` at line 175) automatically nests `hcm:` errors at the right depth — no new wrap site needed. |
| `internal/listener/manager_test.go` | Modify | Extend: HCM type_url resolves to `hcm.NewFilter` (happy path with a minimal HCM bootstrap embedded as a proto literal); HCM with wrong-shape typed_config errors with `listener: %q: filter_chains[0]: hcm: ...` (verifying both the listener wrap and the inner hcm prefix); unknown filter type_url still errors with the existing `listener: ...: unknown filter type_url ...` message. |
| `internal/bootstrap/bootstrap.go` | Modify | Three blank imports added under the existing comment block (after line 14): `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"`, `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"`, `_ "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"`. Per ADR-0016 the addition is a registry-population mechanism documented in PROGRESS, not a new ADR. The existing comment block already explains the pattern; the new imports inherit that documentation by adjacency. |
| `internal/bootstrap/bootstrap_test.go` | Modify | Extend (or add): a phase-04 round-trip test loading a minimal HCM-only bootstrap YAML and confirming `protojson` round-trips it without `discardUnknown: false` complaints. The shape mirrors the existing phase-01 round-trip test for tcp_proxy. |
| `internal/http/doc.go` | Modify | Amend the package doc-string from "the real implementation lands in phase 04. See docs/envoy-go/ROADMAP.md and docs/envoy-go/phases/04-*/SPEC.md once that phase enters in-progress." to "Phase 04 landed the HTTP connection manager + route match + router action + direct_response under `internal/filter/hcm/`. This `internal/http` package is retained as a stable import target if future code needs HTTP utilities that are not HCM-specific (e.g., a shared HTTP/2 or HTTP/3 helper layer). It contains no symbols at phase 04." Settles SPEC §10 #10 to the conservative recommendation (keep the placeholder, amend the doc — preserves the import path for future use). |
| `internal/filter/tcpproxy/` | *Unchanged* | Phase-02 carryover. Verified at Task 17 gate sweep that `FuzzTcpProxyFilter` runs clean and all `internal/filter/tcpproxy/*_test.go` tests pass with no regression. |
| `internal/cluster/` | *Unchanged* | Per SPEC §4.2. The HCM router action consumes `Cluster.Dial(ctx)` and `Manager.Get(name)` exactly as the phase-03 `tcpproxy` filter does. Phase-03 REVIEW Minor 4 (stale "phase 02 SPEC §10 #2" comment at `cluster.go:13`) and Minor 5 (`manager.go` "phase 02" error texts) carry forward — phase 04 does not touch these files. |
| `internal/tls/` | *Unchanged* | Per SPEC §4.2. Fixture 0003 is plaintext. |
| `cmd/envoy-go/main.go` | *Unchanged* | The listener manager's HCM registration is transparent to `main`. |
| `cmd/envoy-go/main_test.go` | Modify | Add a third bootstrap variant: `TestEnvoyGoBinary_HCMSmoke` — a single plaintext listener with one HCM filter, one direct_response route (`path: /health` → 200 `OK\n`). Spawn the envoy-go binary; issue an HTTP/1.1 `GET /health` against the listener; assert status 200 and body `OK\n`. The phase-02 / phase-03 bootstrap variants remain. |
| `test/differential/fixture/fixture.go` | Modify | Add an additive optional interface: `type HTTPExpectations interface { HTTPExpectations() []HTTPRequestExpectation }` plus `type HTTPRequestExpectation struct { Method, Path string; ExpectStatus int; ExpectBodyEquivalent bool }`. Existing `Driver` interface is unchanged; the optional interface is type-asserted at the runner per phase-02's `DistributionAsserter` pattern (already at `runner_test.go:140-145`). Settles SPEC §10 #4 to ADR-0043 (typed extension over in-driver internalization) — picked because phase-05 HTTP/2 will reuse the same machinery. |
| `test/differential/runner_test.go` | Modify | Per-fixture orchestration loop gains a new opt-in pass: if `d.(fixture.HTTPExpectations)` succeeds (mirror of the existing `DistributionAsserter` type assertion), after the byte-comparison and distribution assertion, the runner iterates `d.HTTPExpectations()`. For each `HTTPRequestExpectation`, the runner: dials ref via `helpers.HTTPRoundTrip`, captures status+body+headers; dials subj similarly; asserts ref-status == subj-status == expectation.ExpectStatus; if `ExpectBodyEquivalent`, asserts ref-body bytes == subj-body bytes; computes `helpers.HTTPHeaderDiff(refHeaders, subjHeaders, helpers.PhaseFourHTTPAllowList)` and fails if any non-allowlisted header differs. Blank-import for `test/fixtures/0003-http11-routing/driver` added at the top alongside the three existing fixture imports. |
| `test/helpers/http.go` | Create | `HTTPRoundTrip(ctx context.Context, addr, method, path string, headers http.Header, body []byte) (*http.Response, []byte, error)`: dials TCP via `net.DialTimeout` honouring ctx, constructs an HTTP/1.1 request via `http.NewRequestWithContext`, writes via `req.Write(conn)`, reads response via `http.ReadResponse(bufio.NewReader(conn), req)`, fully consumes `Response.Body` via `io.ReadAll`, returns the response (with Body already consumed and replaced by an `io.NopCloser(bytes.NewReader(...))`) plus the body bytes plus error. Closes the conn on every exit path. Used by the 0003 driver and by `runner_test.go`'s per-request orchestration. |
| `test/helpers/http_diff.go` | Create | `HTTPHeaderDiff(refHeaders, subjHeaders http.Header, allowList []string) (refOnly, subjOnly []string)` returns the set-symmetric-difference of header names after lowercasing and applying the allow-list. Allow-list entries are compared case-insensitively; entries with a trailing `*` are prefix-allow (e.g., `x-envoy-*` permits any header whose lowercased name begins with `x-envoy-`). Phase-04 default allow-list constant: `var PhaseFourHTTPAllowList = []string{"date", "server", "content-length", "transfer-encoding", "x-envoy-*", "x-forwarded-*", "x-request-id"}`. Settles SPEC §10 #7 to recommended option (a) — fixed in-code list (one fixture in phase 04). |
| `test/helpers/http_test.go` | Create | Round-trip against a loopback `net.Listener`-backed in-memory HTTP/1.1 echo (a small `accept loop → http.ReadRequest → write canned response`); ctx-cancel before dial → context-deadline error; ctx-cancel during read → propagated error; conn close on error path verified via finalizer or explicit goroutine-leak check. |
| `test/helpers/http_diff_test.go` | Create | Table-driven: identical headers → empty diff; ref-only / subj-only header in diff; allow-listed `Date` header excluded; allow-listed `x-envoy-attempt-count` (prefix match) excluded; case-insensitivity of header names; case-insensitivity of allow-list entries. |
| `test/differential/runner_test.go` (HTTP echo backend) | Modify | The per-fixture backend-spawning code (currently spawns N TCP echo backends) gains a fixture-driver-controlled flag for HTTP echo backends. The fixture driver reports a backend kind (`BackendKind() backendKind` — new optional method, type-asserted at the runner; if absent, default is TCP echo). For HTTP echo: each backend's accept-loop reads one `http.ReadRequest`, writes one `HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: <n>\r\n\r\nbackend-<idx>:<last-segment-of-path>` per request, then closes the conn. Settles SPEC §10 #6 to recommended option (a) variant — handcrafted bufio-driven backend (NOT `http.Server`; D-3.2 is fine for fixtures, but a handcrafted backend keeps the symmetry with phase-02's TCP echo backends and avoids spinning up a stdlib server in test code). The backend kind is its own optional method to keep the existing TCP-echo fixtures (0000/0001/0002) untouched. |
| `test/fixtures/0003-http11-routing/envoy-go.yaml` | Create | Subject bootstrap. 1 listener `l_http` binding `127.0.0.1:0` (port-zero, harness-resolved like fixtures 0001/0002). 1 plaintext filter chain with empty `filter_chain_match` (phase-03 plaintext rule satisfied). 1 HCM network filter with: `codec_type: HTTP1`; `stat_prefix: ingress_http`; `route_config: { virtual_hosts: [{ name: vh_default, domains: ["*"], routes: [...] }] }`; `http_filters: [{name: envoy.filters.http.router, typed_config: {Router{}}}]`. Three routes in declaration order: `match.path: "/health"` → `direct_response: {status: 200, body: {inline_string: "OK\n"}}`; `match.prefix: "/api"` → `route: {cluster: c_backend}`; `match.prefix: "/"` → `direct_response: {status: 404, body: {inline_string: "not found\n"}}` (the explicit catch-all per SPEC §10 #5 settled choice). 1 STATIC cluster `c_backend` with three `lb_endpoints` at `127.0.0.1:<dyn0>`, `127.0.0.1:<dyn1>`, `127.0.0.1:<dyn2>` (templated by the driver's `SubjectConfig`) and `lb_policy: ROUND_ROBIN`. Admin listener at `127.0.0.1:0` (admin port templated alongside listener port). |
| `test/fixtures/0003-http11-routing/envoy.yaml` | Create | Reference bootstrap. Same listener shape, same HCM config (verbatim, modulo cluster.address differences), same three routes. 1 STRICT_DNS cluster `c_backend` with three `lb_endpoints` at `host.docker.internal:<dyn0..2>`, `dns_lookup_family: V4_ONLY` (ADR-0010 inherited from fixture 0001). Admin at `0.0.0.0:9901` per phase-01/02/03 convention. |
| `test/fixtures/0003-http11-routing/expectations.yaml` | Create | Prose-per-fixture-0001/0002 style (Minor 7 deferred per ADR-0019). Asserts byte-equivalent decoded HTTP/1.1 response body across 27 requests per side; status equivalence per request; route-match selection witnessed by per-cluster RR distribution `[3,3,3]` over the 9 router-action requests; framing-divergence-permitted (Content-Length vs Transfer-Encoding chunked is decoded transparently by `http.ReadResponse`); local-reply body bytes relaxed (404 body may differ between Envoy's local-reply JSON-shape and envoy-go's `not found\n`); `Date`, `Server`, `x-envoy-*`, `x-forwarded-*`, `x-request-id` headers allow-listed. |
| `test/fixtures/0003-http11-routing/README.md` | Create | Explains the fixture's purpose (HCM + route match + router + direct_response), the STATIC-vs-STRICT_DNS divergence (rationale per ADR-0027, inherited), the framing-divergence-permitted rule (per the new BEHAVIOR_CONTRACT subsection ADR-0044), the `--concurrency 1` reference pin inheritance (ADR-0028), the per-request fresh-dial subject-side behaviour (ADR-0039), the explicit catch-all 404 route choice (SPEC §10 #5 settled). |
| `test/fixtures/0003-http11-routing/driver/driver.go` | Create | Fixture driver. `init()` registers via `fixture.RegisterFixture(fixtureName, &httpDriver{})`. `(httpDriver) BackendCount() = 3`. `(httpDriver) BackendKind()` returns the new HTTP-echo enum value (Task 14 introduces). `(httpDriver) SubjectListenerName() = "l_http"`. `(httpDriver) ReferenceListenerPort() = 15003`. `(httpDriver) ReferenceBootstrap(backendPorts []int) string` and `(httpDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string` template the three backend ports + listener/admin ports into the respective YAML shells. `(httpDriver) DriveReference(ctx, addr) ([]byte, error)` and `(httpDriver) DriveSubject(ctx, addr) ([]byte, error)` each issue 27 HTTP/1.1 requests via fresh-per-request `helpers.HTTPRoundTrip` (`Connection: close` per request to keep the RR distribution deterministic): 9 × `GET /health` → expects 200 + body `OK\n`; 9 × `GET /api/v1/<n>` for n=0..8 → expects 200 + body matching `backend-<idx>:v1/<n>` (where `<idx>` is whichever backend served, count-asserted not request-asserted); 9 × `GET /missing/<n>` → expects 404 (body relaxed). Concatenates the 27 response bodies and returns. `(httpDriver) AssertDistribution(refCounts, subjCounts []uint64) error` checks that each side's 9-router-action subset distributes `[3,3,3]` over indices 0..2. `(httpDriver) HTTPExpectations() []fixture.HTTPRequestExpectation` returns the 27 expectations. `(httpDriver) ProbeAdmin` same as fixtures 0001/0002. |
| `test/fixtures/0003-http11-routing/driver/driver_test.go` | Create | Unit test for `AssertDistribution` only — Docker-free. Happy `[3,3,3]` passes; `[4,3,2]` fails with a clear message; `[3,3,3,0,0,0]` (counts longer than expected, e.g. if BackendCount changes) fails. Mirror of fixture 0001/0002's `driver_test.go` discipline. |
| `test/differential/harness.go` | *Unchanged* | The `--concurrency 1` reference container flag is verified at Task 1 to be unconditionally inherited by fixture 0003 (same precondition shape phase 03 used). No harness modification needed. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | Modify | Append two changes: (1) extend the **Header allow-list** table (lines 27–34) with five new rows: `Server` (HCM-locally-generated responses; presence-only; Phase 04; ADR-0044), `Content-Length` and `Transfer-Encoding` (HTTP/1.1 framing-divergence; Phase 04; ADR-0044), `x-envoy-*` (every header; presence-not-required-on-subject; Phase 04; ADR-0044), `x-forwarded-*` and `x-request-id` (same scope; Phase 04; ADR-0044). (2) Add a new top-level `## HTTP/1.1` section after `## TLS`, content per SPEC §5.7, justified by ADR-0044, with explicit Asserted / Not asserted / Header allow-list extensions / Applies to / Does not yet apply to subsections. |
| `docs/envoy-go/DECISIONS.md` | Modify | Append ADR-0037 through ADR-0044 (eight ADRs — listed in `## ADRs introduced by this plan` below). Each ADR lands in the same commit as the code that consumes it (phase-00/01/02/03 precedent). |
| `docs/envoy-go/ROADMAP.md` | *Not modified by this plan* | Row 04 is already `in-progress` (phase-04 SPEC commit's state-2 entry flipped it). Advances to `done` at state-machine step 6 in a later session per ADR-0005. |
| `docs/envoy-go/STATE.md` | Modify (at exit) | Advanced to `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development` at this plan-authoring session's exit commit — matching phases 02/03's exit discipline per ADR-0005 §1. |
| `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` | Create (during execution) | Append-only running log per BOOTSTRAP §5 step 3, matching phase-00/01/02/03 conventions. |

---

## ADRs introduced by this plan

Eight ADRs land at execution time. Each is the first-use task's responsibility and goes into the same commit as the code that consumes it. All entries in `DECISIONS.md` are append-only (D-3.5); no landed ADR is edited. ADR numbering continues from the tail verified at PLAN-write time (**ADR-0036** is the current tail, verified by `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` → `ADR-0036:` at `docs/envoy-go/DECISIONS.md:1146`). Per SPEC §4.4, the SPEC anticipated ADRs H–O; the assigned numbers below are sequenced so that **first-use commit order matches DECISIONS.md file order** (phase-02/03 precedent).

The SPEC-to-ADR-number map:

- **SPEC §4.4 ADR-H** (HTTP/1.1 wire codec source) → **ADR-0037** (lands Task 3, first use of `writeStatusReply` / wire-codec helpers).
- **SPEC §4.4 ADR-J** (route match subset — `prefix` + `path`) → **ADR-0038** (lands Task 4, first use of `routeTable.match`).
- **SPEC §4.4 ADR-L** (per-request fresh upstream dial in router) → **ADR-0039** (lands Task 5, first use of `routerAction.do`).
- **SPEC §4.4 ADR-I** (HTTP-filter framework subset — function-call dispatch, no iteration protocol) → **ADR-0040** (lands Task 7, first use of `parseFilter` http_filters validation).
- **SPEC §4.4 ADR-N** (HCM `stat_prefix` + silently-ignored field set) → **ADR-0041** (lands Task 7, alongside ADR-0040 — same first-use commit).
- **SPEC §4.4 ADR-O** (phase-04 HTTP-filter chain shape — exactly `[router]`) → **ADR-0042** (lands Task 7, alongside ADR-0040 + ADR-0041).
- **SPEC §4.4 ADR-M** (`HTTPExpectations` extension to `Driver`) → **ADR-0043** (lands Task 12, first use of the new optional interface).
- **SPEC §4.4 ADR-K** (BEHAVIOR_CONTRACT HTTP/1.1 subsection) → **ADR-0044** (lands Task 17, BEHAVIOR_CONTRACT update).

Summaries:

- **ADR-0037 (= SPEC ADR-H) — HTTP/1.1 wire codec source: stdlib `net/http`.** Options considered: (H1) handcrafted RFC 7230/9112 parser+writer; (H2) stdlib `net/http.ReadRequest` / `Response.Write` / `ReadResponse` (the SPEC's choice); (H3) build on `net/http.Server` + `http.Handler`. (H1) carries an unbounded RFC-corner-case tax (chunked encoding, header continuation, Host enforcement) that stdlib already pays. (H3) is forbidden by D-3.2 (`net/http.Server` injects Date / Content-Length / strips headers / enforces RedactHeaders — too much magic for a proxy). (H2) keeps the doctrine intent (no `httputil.ReverseProxy`, no third-party server core, no embedded fasthttp/Caddy/Traefik) while sidestepping (H1)'s tax. Documents the residual stdlib-driven divergences from upstream Envoy: header canonicalization (`textproto.CanonicalMIMEHeaderKey` on header names), Host validation, method whitelist (stdlib doesn't reject custom methods but may normalise certain method spellings). All such divergences are observable on the asserted request-forwarding path; ADR-0044's BEHAVIOR_CONTRACT subsection records each one. Lands in Task 3. Supersedes nothing.
- **ADR-0038 (= SPEC ADR-J) — Phase-04 route match subset: `match.prefix` (bytewise) + `match.path` (case-sensitive exact).** Permits `match.prefix` (bytewise prefix on `req.URL.Path`; `prefix: "/api"` matches `/api`, `/api/`, `/api/v1`, AND `/apifoo`) and `match.path` (case-sensitive exact equality on `req.URL.Path`). Documents the planned-divergence on `match.prefix` semantics: Envoy uses path-segment-aware prefix matching (so `prefix: "/api"` matches `/api`, `/api/`, `/api/x` but NOT `/apifoo`); phase 04 implements bytewise prefix. The fixture driver does not exercise `prefix` on a non-segment-boundary path (every router-action request uses `/api/v1/<n>`), so the divergence is not surfaced in the differential gate. A future phase that exercises non-segment paths must either (a) tighten `match.prefix` to be segment-aware or (b) extend BEHAVIOR_CONTRACT with the assertion. Other match fields error: `safe_regex`, `path_separated_prefix`, `connect_matcher`, `headers[]`, `query_parameters[]`, `dynamic_metadata[]`, `runtime_fraction`, `tls_context`, `case_sensitive=false`. `case_sensitive` unset (nil pointer) and `case_sensitive=true` both pass through (case-sensitive is the Envoy default). Lands in Task 4. Supersedes nothing.
- **ADR-0039 (= SPEC ADR-L) — Per-request fresh upstream dial in phase-04 router.** Documents that the router action does NOT pool upstream connections — every routed request opens a new TCP connection to the picked endpoint via `Cluster.Dial(ctx)`. Rationale: connection pooling is a load-bearing concern with timeouts, idle-eviction, max-streams, idle-stream-cleanup, etc., that belongs to the upstream-robustness family; phase 04 punts. Performance is suboptimal vs upstream Envoy (which pools); the differential gate does not assert pool/non-pool. The divergence is permitted because (a) BEHAVIOR_CONTRACT does not assert connection re-use, (b) the asserted surface (decoded body byte-equivalence per request) is preserved, and (c) the per-request `Cluster.Dial` call is what makes the round-robin distribution `[3,3,3]` deterministic on the subject side (non-pooled flow → endpoint pick on every request → mod-3 partition). Records the carry-forward to the upstream-robustness phase that introduces pooling. Lands in Task 5. Supersedes nothing.
- **ADR-0040 (= SPEC ADR-I) — Phase-04 HTTP-filter framework subset.** Permits exactly one HTTP filter, named `envoy.filters.http.router` with `typed_config.type_url == "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"`. The filter-iteration protocol (decode-headers, decode-data, decode-trailers, encode-headers, encode-data, encode-trailers, stop/continue/buffer iteration directives) is NOT introduced here. Instead, the router is invoked by direct function call inside the HCM connection loop (`entry.action.do(ctx, req, bw)` where the action is `routerAction`). The Router proto's body is unmarshalled but every Router-proto field is silently ignored at phase 04 (`dynamic_stats`, `start_child_span`, `upstream_log[]`, `suppress_envoy_headers`, `strict_check_headers`, `respect_expected_rq_timeout`, `suppress_grpc_request_failure_code_stats`, `upstream_http_filters`). Phase 07's filter-chain framework supersedes this with the actual iteration protocol. Lands in Task 7. Supersedes nothing (phase 04 is the first HCM phase).
- **ADR-0041 (= SPEC ADR-N) — HCM `stat_prefix` + silently-ignored field set.** Phase 04's `HttpConnectionManager` proto consumption: REQUIRED fields are `codec_type` (must be `HTTP1` or `AUTO`), `stat_prefix` (must be non-empty string — stored on `Filter` for forward use; phase 06 stats consumer; settles SPEC §10 #9 to `string` field), `route_specifier` (must be `route_config`), `http_filters` (must be exactly one `[router]` per ADR-0040). ERRORED fields (per SPEC §2): `Rds`, `ScopedRoutes`, `ScopedRds` route_specifiers; `codec_type=HTTP2`/`HTTP3`. Every other top-level HCM proto field is SILENTLY IGNORED — the phase-04 ignored-set: `tracing`, `access_log[]`, `http_protocol_options`, `common_http_protocol_options`, `server_header_transformation`, `local_reply_config`, `internal_redirect_policy`, `request_id_extension`, `path_with_escaped_slashes_action`, `merge_slashes`, `xff_num_trusted_hops`, `via`, `proxy_100_continue`, `stream_idle_timeout`, `request_timeout`, `request_headers_timeout`, `drain_timeout`, `delayed_close_timeout`, `forward_client_cert_details`, `original_ip_detection_extensions`, `idle_timeout`, `max_request_headers_kb`, `request_headers_kb_limit`, `add_user_agent`, `set_current_client_cert_details`, `mutex_tracing`, `proxy_status_config`, `early_header_mutation_extensions`, `header_validation_config`, `append_local_overload`, `pass_through_is_optional`, `request_block_size`, `strip_matching_host_port`, `strip_any_host_port`, `strip_trailing_host_dot`, `add_proxy_protocol_connection_state`. Route-level silently-ignored: `request_headers_to_add`, `request_headers_to_remove`, `response_headers_to_add`, `response_headers_to_remove`, `metadata`, `decorator`, `tracing`, `per_request_buffer_limit_bytes`. Phase 06+ may move members from "ignored" to "honoured" with a superseding ADR. Rationale for silent-ignore (vs error): phase-04 fixtures may inherit upstream-Envoy bootstraps that include these fields; matching upstream Envoy's forward-compatible posture on irrelevant-to-the-asserted-surface fields keeps fixture authors from having to scrub bootstraps. Lands in Task 7 alongside ADR-0040. Supersedes nothing.
- **ADR-0042 (= SPEC ADR-O) — Phase-04 HTTP-filter chain shape: exactly `[router]`.** Tightens phase-02 ADR-0033's network-filter chain rule to the HTTP-filter sub-domain. `http_filters[]` must be exactly one entry, named `envoy.filters.http.router` with the Router proto type_url. `http_filters` empty, `http_filters` with two entries (even if both router), or `http_filters[0]` named/typed differently — all error at build with `hcm: http_filters: ...`. Rationale: phase-04's filter sub-domain is degenerate by construction; the iteration protocol lands in phase 07. The router-only constraint is the smallest shape that makes "the router action runs" expressible. `typed_per_filter_config` on routes (per-route filter override) errors at build (SPEC §2). Phase 07's filter-chain framework supersedes this with the multi-filter shape. Lands in Task 7 alongside ADR-0040 + ADR-0041. **Supersedes:** none — ADR-0033 covers network-filter chains (phase 02); ADR-0042 covers HTTP-filter chains (phase 04). They share a "minimal chain shape" theme but address disjoint protocol layers.
- **ADR-0043 (= SPEC ADR-M) — Fixture-driver `HTTPExpectations` extension.** Adds an OPTIONAL interface `type HTTPExpectations interface { HTTPExpectations() []HTTPRequestExpectation }` and a new struct `type HTTPRequestExpectation struct { Method, Path string; ExpectStatus int; ExpectBodyEquivalent bool }`. The `Driver` interface itself is unchanged; the new optional interface is type-asserted at the runner per phase-02's `DistributionAsserter` precedent (`runner_test.go:140-145`). The runner's per-fixture orchestration loop gains a new pass: when `d.(fixture.HTTPExpectations)` succeeds, after byte-comparison and distribution assertion, the runner re-issues each expectation against ref and subject via `helpers.HTTPRoundTrip` and compares status + body (when `ExpectBodyEquivalent`) + header set (under `helpers.HTTPHeaderDiff`). Existing TCP-only fixtures (0000, 0001, 0002) do NOT implement this interface; the runner's code path is gated on the type assertion, so they are unaffected. Settles SPEC §10 #4 to the typed-extension over in-driver internalisation — picked because phase-05 HTTP/2 will reuse the same machinery (the alternative would re-introduce per-driver comparison code in 0004+ as well, doubling the divergence). Lands in Task 12. **Supersedes (informal):** the implicit "byte-comparison is the only assertion" contract of the runner; that contract was never ADR'd, so ADR-0043's supersession header is informal, mirroring ADR-0034's (informal) qualifier on the `Drive` split.
- **ADR-0044 (= SPEC ADR-K) — BEHAVIOR_CONTRACT HTTP/1.1 subsection.** New top-level section codifying the phase-04 HTTP/1.1 equivalence surface. **Asserted equivalence**: response status code per request; decoded response body bytes per routed-to-upstream request (the body the fixture driver reads after `http.ReadResponse`); decoded response body bytes for `direct_response` 200 paths; route-match selection (same method + path → same matched route on both proxies, witnessed via per-cluster RR distribution `[3,3,3]`); upstream-side request preservation (verbatim Host, method, path-with-query, body — except where stdlib HTTP/1.1 parsing on the subject side introduces a bounded, documented normalisation per ADR-0037). **Not asserted**: response-header set equality (only set-equality modulo allow-list); local-reply body bytes (Envoy and envoy-go differ in their default 4xx/5xx local-reply body content; status is asserted, body is relaxed); `Content-Length` vs `Transfer-Encoding: chunked` framing per response (the harness decodes both sides via `http.ReadResponse`); upstream connection re-use (envoy-go does not pool per ADR-0039; Envoy does); `x-envoy-*` / `x-forwarded-*` / `x-request-id` headers (envoy-go adds none; Envoy adds many — all in the allow-list). **Header allow-list extensions**: `Server` (HCM-locally-generated; presence-only; phase 04; ADR-0044), `Content-Length` and `Transfer-Encoding` (HTTP/1.1 framing-divergence-permitted), every `x-envoy-*` header (presence-not-required on subject), every `x-forwarded-*` header, `x-request-id`. **Applies to**: phase-04 envoy-go `internal/filter/hcm/` package, exercised via fixture `0003-http11-routing`. **Does not yet apply to**: HTTP/2 (phase 05), HTTP/3 (later), HCM filter chain beyond `[router]` (phase 07), upstream connection pooling (upstream-robustness family), HTTPS (phase 04.x or phase 05.x). Lands in Task 17. Supersedes nothing.

If an unforeseen decision surfaces during execution that has cross-phase impact (per D-3.5), the executor writes a new sequential ADR (ADR-0045+) in the same commit as the code it decides for. If such a decision would expand phase-04 scope beyond SPEC §1–§4, invoke `superpowers:systematic-debugging` and then either re-scope the task in place or split per `BOOTSTRAP_PROMPT.md` §6.

---

## Settled SPEC §10 deferred decisions

SPEC §10 leaves fourteen implementation-detail choices to the planner. This PLAN settles them here so the executor does not re-litigate. Only decisions with cross-phase impact (security tightening, new mechanism choice, interface shape) are also captured as ADRs.

1. **Route match representation.** **Interface `routeMatch` with two implementations `matchPrefix` and `matchPath`.** SPEC §10 #1 recommended option (a). Type-safe; matches the codebase's pattern of small interfaces with concrete implementations (e.g., phase-02 `loadBalancer`, phase-03 `validatorChain`). Not separately ADR'd — implementation-detail; ADR-0038 (route match subset) names the predicates.
2. **Action representation.** **Interface `routeAction` with two implementations `directResponseAction` and `routerAction`.** Same shape as #1 for symmetry. Not ADR'd — captured in ADR-0040 (filter framework subset) by reference.
3. **Close-after-action signal mechanism.** **Sentinel error `errCloseAfterAction` returned by `routeAction.do`.** SPEC §10 #3 recommended option (a). Rationale: a sentinel error is idiomatic Go (compare `io.EOF`, `http.ErrNoCookie`); a `*bool` callback flag (option (b)) couples action types to a parameter shape they should not own; option (c) (re-parsing the bufio buffer) was rejected by the SPEC. The connection loop checks `errors.Is(actErr, errCloseAfterAction)` after `do` returns; non-sentinel errors are logged and trigger downstream close. Not separately ADR'd — implementation-detail.
4. **`HTTPExpectations` extension to `Driver` vs in-driver comparison.** **Typed extension via the new optional interface.** SPEC §10 #4 recommended path; phase-05 HTTP/2 will reuse the same machinery, justifying the extension cost over per-driver duplication. Codified in ADR-0043.
5. **Explicit catch-all 404 route in fixture 0003 vs implicit no-match 404.** **Explicit catch-all `match.prefix: "/"` → `direct_response: {status: 404, body: {inline_string: "not found\n"}}`.** SPEC §10 #5 recommended option (a). Rationale: matches Envoy's typical config posture (configs usually carry an explicit terminal route); makes the fixture's BehaviorContract's "Asserted: 404 status equivalence" path explicit rather than reliant on the connection-loop's implicit 404; the fixture is more self-documenting. The connection-loop's implicit 404 path is still exercised by negative unit tests in `connection_test.go`. Not ADR'd.
6. **HTTP echo backend isolation.** **Handcrafted bufio-driven backend matching the existing TCP echo backends.** SPEC §10 #6 listed two options: (a) one in-process `http.Server` per backend behind `net.Listen`, (b) handcrafted bufio-driven backend. Settling on (b) — even though SPEC recommended (a) — for two reasons: (i) symmetry with phase-02's TCP echo backends in `runner_test.go`'s per-fixture spawning code, which use a handcrafted accept loop; (ii) `http.Server` brings a goroutine-management surface (`http.Server.Shutdown`, listener tracking) that the rest of the harness does not need and would have to plumb. The handcrafted backend is one accept-loop-with-`http.ReadRequest`-then-canned-response shaped exactly like the existing TCP-echo backend. D-3.2 permits stdlib in test code, so option (a) is not foreclosed by doctrine — this is a code-organisation choice. Not ADR'd.
7. **Header allow-list shape in `helpers.HTTPHeaderDiff`.** **Fixed in-code list.** SPEC §10 #7 recommended option (a) for phase 04 (one fixture); phase 05 may move to per-fixture configuration if it adds a second HTTP fixture with different allow-list needs. Phase-04 list: `["date", "server", "content-length", "transfer-encoding", "x-envoy-*", "x-forwarded-*", "x-request-id"]`. Not ADR'd — list content captured in ADR-0044.
8. **Date header source.** **`time.Now().UTC().Format(http.TimeFormat)` per response.** SPEC §10 #8 recommended option (a); option (b) (cached + refreshed once per second) is a phase-06+ optimisation if profiling reveals hot allocation. Not ADR'd.
9. **`stat_prefix` storage shape.** **`string` field on `Filter`.** SPEC §10 #9 recommended option (a) for minimum surface; phase 06 stats consumer can extend to a struct if needed. Codified in ADR-0041.
10. **Whether to introduce `internal/http/` as a package now or keep everything inside `internal/filter/hcm/`.** **Keep everything inside `internal/filter/hcm/` and amend the `internal/http/doc.go` placeholder to point at hcm.** SPEC §10 #10 recommended path; matches the master STATE.md's phase-04 scope guidance. The placeholder package's `doc.go` is amended (Task 2) to read "Phase 04 landed the HTTP connection manager + route match + router action + direct_response under `internal/filter/hcm/`. This package is retained as a stable import target if future code needs HTTP utilities that are not HCM-specific." Not ADR'd.
11. **ADR numbering.** **ADR-0037..ADR-0044 sequential at file tail.** Tail verified at PLAN-write time as ADR-0036 (`docs/envoy-go/DECISIONS.md:1146`). First-use commit order matches DECISIONS.md file order per phase-02/03 precedent. Mapping in `## ADRs introduced by this plan` above.
12. **Subject-side `Server: envoy` vs `Server: envoy-go`.** **`Server: envoy`.** Reaffirms ADR-0014 (admin `/ready` precedent). SPEC §10 #12 recommended this — picking `envoy-go` would require superseding ADR-0014 (more work) and would force the BEHAVIOR_CONTRACT allow-list to add a special-case rule for the subject-side `Server: envoy-go` value (the current rule is `Server: presence-only` and the existing fixtures already comply). Not ADR'd — codified in `codec.serverHeader()` and ADR-0044 (BEHAVIOR_CONTRACT subsection).
13. **`Content-Type: text/plain` on locally-generated bodies.** **`Content-Type: text/plain` for direct_response and for 4xx/5xx local replies.** SPEC §10 #13 recommended path; consistent with phase-01 admin `/ready`. Not ADR'd — captured in `codec.writeStatusReply` and the BEHAVIOR_CONTRACT subsection.
14. **HTTP echo backend body format.** **`backend-<idx>:<last-segment-of-path>` per response, where `<idx>` is the backend index 0..2 and `<last-segment-of-path>` is the part of `req.URL.Path` after the final `/`.** SPEC §5.10 / §10 #14 recommended path. The `backend-<idx>:` prefix is what `AssertDistribution` counts on; the path-segment suffix makes responses distinguishable per request (so the byte-stream returned by `DriveSubject` is non-degenerate and can be byte-compared on the routed-to-upstream subset). Not ADR'd.

---

## Phase-03 REVIEW carryover resolution matrix

SPEC §12 triages the eight phase-03 Minors. Phase 04 lands ZERO Minors as code fixes (the SPEC §12 "RESOLVED-OPPORTUNISTIC" status for M-4 and M-5 is conditional on touching `internal/cluster/`, which phase 04 does NOT do per SPEC §4.2). All 8 Minors carry forward. Triage table:

| Phase-03 Minor | Triage | Landing task |
|---|---|---|
| M-1 (ADR-0033 `Supersedes:` punctuation drift) | DEFERRED | Cosmetic; addressing requires editing a landed ADR (D-3.5 forbids). No phase-04 site. |
| M-2 (Listener `Stop()`/`Listeners()` race on `rt.netLn`) | DEFERRED | Phase 04 does not touch `internal/listener/manager.go` lock discipline (the change is a `filterRegistry` map extension, not a state-change). Race-detector run in Task 17 will flag if regression occurs. |
| M-3 (`chainSpecificityRank` initial `rank := 4` defensive sentinel) | NO-ACTION | Phase 04 does not touch the listener-manager body beyond `filterRegistry`. |
| M-4 (`internal/cluster/cluster.go:13` stale "phase 02 SPEC §10 #2") | DEFERRED | SPEC §4.2 declares `internal/cluster/` unchanged. Phase-04 router action consumes `Cluster.Dial` but does not edit `cluster.go`. Carries forward. |
| M-5 (`internal/cluster/manager.go` "phase 02" error texts) | DEFERRED | Same posture as M-4. |
| M-6 (phase-02 Minor 5 `readyListenerAddrs` goroutine leak) | DEFERRED-AGAIN | Phase 04 does not touch ready-sentinel path. |
| M-7 (prose `expectations.yaml`) | DEFERRED-AGAIN | Fixture 0003's `expectations.yaml` follows the prose convention per ADR-0019. |
| M-8 (`inlineString` indent helper in fixture-0002 driver) | NO-ACTION | Phase 04 fixture 0003 is plaintext — no PEM rendering. |

Zero RESOLVED items in this phase. Eight DEFERRED items documented with rationale. No Minor rises to a phase-04 blocker.

A separate D-3.4 finding is recorded by SPEC §12 final paragraph: master `STATE.md` at commit `230fef6` listed an M-1..M-8 description that did not match the actual phase-03 REVIEW.md content. The fabrication is a context-isolation slip, silently corrected at SPEC drafting time by SPEC §12 citing the actual REVIEW content. No phase-04 action needed.

---

## Spec-review advisory responses

The SPEC STATE block notes four non-blocking advisory items from the spec-document-reviewer loop. Each is addressed:

- **(i) Verify phase-03 REVIEW Important findings I-1..I-4 actually landed in commits `98cc35b` / `cbfe275`.** **Verified at PLAN-write time:** all four fixes are present in the worktree's HEAD code per `git log --oneline -- internal/tls/params.go internal/listener/manager.go test/fixtures/0002-tls-tcp/driver/driver.go` and direct inspection of the affected lines. I-1 (TLS_AUTO doc-comment-vs-code) is fixed in `internal/tls/params.go` head comment; I-2 (`listener: %q:` colon discipline) is fixed at `internal/listener/manager.go:302`; I-3 (SPEC §5.5 TLS_AUTO row) is fixed in phase-03 SPEC; I-4 (orphaned `prefix` variable in fixture 0002 driver) is removed. No Task 0 needed. Task 1's precondition check re-verifies via the same `git log` command for defense-in-depth.
- **(ii) Decide whether `internal/http/` placeholder stays / is amended / removed at PLAN time.** **Decision: kept and amended** (Settled SPEC §10 #10 above). Task 2 amends the placeholder.
- **(iii) Record whether a phase-04.x HTTPS sub-phase is in scope.** **Decision: not in phase-04 scope.** Phase 04 ships plaintext-only HTTP/1.1 (fixture 0003). The HTTPS sub-phase is deferred to phase 04.x or 05.x or a dedicated HTTPS-fixture sub-phase per SPEC §2 — that decision is the next planner's. Recorded in `## Scope check` reason 4.
- **(iv) The `HttpConnectionManager` proto field set in go-control-plane may differ slightly from what SPEC §9 enumerates; planner verifies against the proto at PLAN time.** **Verified at PLAN-write time:** the go-control-plane v1.32.4 `HttpConnectionManager` proto contains the field set SPEC §9 enumerates plus a small number of additional fields (newer extensions added in later upstream Envoy versions but compiled into v1.32.4's proto). Task 7 (`config.go`) handles unknown-to-phase-04 fields by silently ignoring them per ADR-0041 (the silent-ignore policy is forward-compatible by construction). The exact list of recognised-but-ignored field names is captured in ADR-0041's "ignored-set" enumeration, which the executor verifies by reading the proto descriptor at landing time and adjusting the ADR-0041 list if any field was added in v1.32.4 between SPEC writing and PLAN writing.

---

## Execution preconditions

Before Task 1, the executing session must:

1. Be running in a **fresh worktree on a phase-implementation branch cut off `master`**, NOT `phase/04-http-1.1-plan` (this plan's authoring branch) and NOT `phase/04-http-1.1-spec` (the SPEC's authoring branch). Recommended: `.worktrees/phase-04-http-1.1-impl` on branch `phase/04-http-1.1-impl`. STATE.md's `last-commit` at cold-start must be the commit that landed this PLAN.md on master. Per ADR-0003: branch fast-forwards into `master` at session exit.
2. Have `docker` available (verify with `docker version`). Required for Task 15's full differential gate (`go test ./test/differential/...`).
3. Have Go 1.23+ installed (verify with `go version`). Native fuzzing (`testing.F`) requires Go 1.18+; 1.23 is the module floor.
4. Have `golangci-lint` installed at the ADR-0009-pinned version v1.64.8 (verify with `golangci-lint version`); install via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8` if missing.
5. `go test ./...` must be green on `master` at cold-start — this plan assumes a clean baseline (phase-03 gate (e) still holds). If not, invoke `superpowers:systematic-debugging` on the regression *before* starting Task 1.
6. `go list -m github.com/envoyproxy/go-control-plane/envoy` resolves to `v1.32.4` (ADR-0013). If a different version is recorded, invoke `superpowers:systematic-debugging` — phase 04 must not silently re-pin.
7. `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0036:` (or later if a mid-phase ADR has landed since this PLAN was written). If the tail is `ADR-0036`, the phase-04 ADRs are assigned 0037..0044 as in this PLAN. If higher, re-number phase-04 ADRs sequentially from `tail + 1` and update every task's ADR reference *before* starting Task 1.
8. Phase-03 REVIEW Important findings I-1..I-4 are present in HEAD: `git log --oneline -- internal/tls/params.go internal/listener/manager.go test/fixtures/0002-tls-tcp/driver/driver.go` shows commits `98cc35b` and `cbfe275` in the history of those files. If any commit is missing from the file's history, invoke `superpowers:systematic-debugging` on the gap before starting Task 1.

If any precondition fails: invoke `superpowers:systematic-debugging` on the missing dependency. Do not improvise an install path or skip a failing test.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md`

No code change. This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                # expect: phase/04-http-1.1-impl (or equivalent impl branch)
git log -1 --format=%H                                         # expect: same SHA as docs/envoy-go/STATE.md last-commit field
docker version                                                 # expect: client + server reported
go version                                                     # expect: go1.23+
golangci-lint version                                          # expect: golangci-lint has version 1.64.8
go test ./...                                                  # expect: every package PASS (no FAIL, no compile error)
go list -m github.com/envoyproxy/go-control-plane/envoy        # expect: v1.32.4
grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1           # expect: ## ADR-0036:  (re-number phase-04 ADRs if higher — see precondition 7)
git log --oneline -- internal/tls/params.go internal/listener/manager.go test/fixtures/0002-tls-tcp/driver/driver.go | head -20
                                                                # expect: commits 98cc35b and cbfe275 visible in the output (phase-03 I-1..I-4 fixes)
grep -n -- '--concurrency' test/differential/harness.go        # expect: at least one hit; Task 15 assumes unconditional (not fixture-gated) inheritance per ADR-0028
```

If any line fails, stop and follow the precondition's "if fails" guidance (typically: invoke `superpowers:systematic-debugging` with the specific symptom).

**Note on the `--concurrency` grep:** if the call site is fixture-gated (e.g., wrapped in `if fixtureName == "0001-tcp-proxy-rr"`), Task 15's inheritance assumption is wrong — either update `harness.go` to make the flag unconditional (writing ADR-0045 for the change) or extend the gate to include `"0003-http11-routing"`. Catching this at Task 1 saves an hour at Task 15 differential-gate debugging.

- [ ] **Step 2: Create `docs/envoy-go/phases/04-http-1.1/PROGRESS.md`**

```markdown
# Phase 04 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all preconditions were satisfied at cold-start>

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** <sha — this task's commit>
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions"; phase-03 I-1..I-4 fixes confirmed present in HEAD.
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ docker version
<verbatim — first line of client + server sections>
$ go version
<verbatim>
$ golangci-lint version
<verbatim>
$ go test ./...
<verbatim — last 30 lines>
$ go list -m github.com/envoyproxy/go-control-plane/envoy
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
<verbatim>
$ git log --oneline -- internal/tls/params.go internal/listener/manager.go test/fixtures/0002-tls-tcp/driver/driver.go | head -20
<verbatim>
```
```

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/04-http-1.1/PROGRESS.md
git commit -m "phase 04: PROGRESS.md preamble + precondition verification"
```

After the commit, update the just-written PROGRESS.md entry's `**Commits:**` line with the short SHA of the commit (phase-02/03 precedent: a follow-up tiny commit `phase 04: PROGRESS SHA-fill for Task 1` lands the SHA).

---

## Task 2: Package skeleton — `internal/filter/hcm/doc.go` + amend `internal/http/doc.go`

**Files:**
- Create: `internal/filter/hcm/doc.go`
- Modify: `internal/http/doc.go`
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 2 entry)

This task lands the new package's doc-shell + the amendment to the phase-00 placeholder, settling SPEC §10 #10. No symbols beyond package documentation; subsequent tasks fill in the package contents.

- [ ] **Step 1: Write `internal/filter/hcm/doc.go`**

```go
// Package hcm parses Envoy v3 HttpConnectionManager protos from a network
// filter's typed_config Any, validates a router-only HTTP-filter chain,
// resolves an inline route_config's single virtual_host's routes into an
// in-memory route table supporting match.prefix and match.path predicates,
// and dispatches matched requests through direct_response (synthesized local
// reply) or route (router action — dialing the named cluster via
// Cluster.Dial(ctx) per request, fresh).
//
// Phase 04 surface: see docs/envoy-go/phases/04-http-1.1/SPEC.md §4.1. Doctrine:
// see docs/envoy-go/DECISIONS.md ADR-0037 (HTTP/1.1 wire codec source: stdlib
// net/http), ADR-0038 (route match subset: prefix + path), ADR-0039 (per-request
// fresh upstream dial), ADR-0040 (HTTP-filter framework subset),
// ADR-0041 (stat_prefix + ignored-set), ADR-0042 (HTTP-filter chain shape
// [router]), ADR-0044 (BEHAVIOR_CONTRACT HTTP/1.1 subsection).
//
// Error-prefix discipline: every error returned by this package begins with
// "hcm: ". Errors crossing the listener-manager boundary are further wrapped
// with "listener: %q: filter_chains[%d]: " by the caller (see
// internal/listener/manager.go).
//
// What this package does NOT do: it does NOT use net/http.Server, does NOT
// use the http.Handler interface, and does NOT call ServeHTTP. The connection
// loop is driven explicitly under Filter.Handle. See doctrine D-3.2.
package hcm
```

- [ ] **Step 2: Amend `internal/http/doc.go`**

Replace the existing placeholder body (the "the real implementation lands in phase 04" text per the C11 verifier report) with:

```go
// Package http is a phase-00 placeholder. Phase 04 landed the HTTP connection
// manager + route match + router action + direct_response under
// internal/filter/hcm/. This package is retained as a stable import target if
// future code needs HTTP utilities that are not HCM-specific (e.g., a shared
// HTTP/2 or HTTP/3 helper layer when those phases land). It contains no
// symbols at phase 04. See SPEC §10 #10 settled choice.
package http
```

- [ ] **Step 3: Build verification**

Run: `go build ./internal/filter/hcm/... ./internal/http/...`
Expected: clean (no symbols, no compile errors). The doc-only packages compile.

- [ ] **Step 4: Vet verification**

Run: `go vet ./internal/filter/hcm/... ./internal/http/...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/hcm/doc.go internal/http/doc.go
git commit -m "phase 04: internal/filter/hcm package skeleton + amend internal/http placeholder"
```

- [ ] **Step 6: Append PROGRESS.md entry for Task 2**

```markdown
## Task 2 — internal/filter/hcm — package skeleton + internal/http amendment

**Commits:** <short-sha>
**Notes:** Doc-only kickoff; no symbols. Settles SPEC §10 #10 (kept the placeholder, amended the doc).
**Outputs:**
```
$ go build ./internal/filter/hcm/... ./internal/http/...
<no output>
$ go vet ./internal/filter/hcm/... ./internal/http/...
<no output>
```
```

Commit the PROGRESS.md update with message `phase 04: PROGRESS SHA-fill for Task 2`.

---

## Task 3: `internal/filter/hcm/codec.go` + tests + ADR-0037

**Files:**
- Create: `internal/filter/hcm/codec.go`
- Create: `internal/filter/hcm/codec_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0037)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 3 entry)

The wire-codec helpers — the only path that locally generates HTTP/1.1 response bytes. TDD: tests first.

- [ ] **Step 1: Write `internal/filter/hcm/codec_test.go`**

```go
package hcm

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServerHeader(t *testing.T) {
	if got := serverHeader(); got != "envoy" {
		t.Errorf("serverHeader() = %q, want %q", got, "envoy")
	}
}

func TestDateHeader(t *testing.T) {
	got := dateHeader()
	if _, err := time.Parse(http.TimeFormat, got); err != nil {
		t.Errorf("dateHeader() = %q is not RFC 7231 IMF-fixdate parseable: %v", got, err)
	}
}

func TestWriteStatusReply(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantStatus  string
		wantCLen    string
	}{
		{"200 OK with body", 200, "OK\n", "HTTP/1.1 200 OK\r\n", "Content-Length: 3\r\n"},
		{"400 Bad Request empty body", 400, "", "HTTP/1.1 400 Bad Request\r\n", "Content-Length: 0\r\n"},
		{"404 Not Found", 404, "not found\n", "HTTP/1.1 404 Not Found\r\n", "Content-Length: 10\r\n"},
		{"417 Expectation Failed empty", 417, "", "HTTP/1.1 417 Expectation Failed\r\n", "Content-Length: 0\r\n"},
		{"500 Internal Server Error empty", 500, "", "HTTP/1.1 500 Internal Server Error\r\n", "Content-Length: 0\r\n"},
		{"502 Bad Gateway empty", 502, "", "HTTP/1.1 502 Bad Gateway\r\n", "Content-Length: 0\r\n"},
		{"503 Service Unavailable empty", 503, "", "HTTP/1.1 503 Service Unavailable\r\n", "Content-Length: 0\r\n"},
		{"501 Not Implemented empty", 501, "", "HTTP/1.1 501 Not Implemented\r\n", "Content-Length: 0\r\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeStatusReply(&buf, tc.status, tc.body); err != nil {
				t.Fatalf("writeStatusReply: %v", err)
			}
			out := buf.String()
			if !strings.HasPrefix(out, tc.wantStatus) {
				t.Errorf("status line:\n  got:  %q\n  want prefix: %q", out, tc.wantStatus)
			}
			if !strings.Contains(out, tc.wantCLen) {
				t.Errorf("missing %q in:\n%s", tc.wantCLen, out)
			}
			if !strings.Contains(out, "Server: envoy\r\n") {
				t.Errorf("missing Server header in:\n%s", out)
			}
			if !strings.Contains(out, "Content-Type: text/plain\r\n") {
				t.Errorf("missing Content-Type header in:\n%s", out)
			}
			if !strings.Contains(out, "Date: ") {
				t.Errorf("missing Date header in:\n%s", out)
			}
			// Body bytes appear after the blank line.
			if tc.body != "" {
				idx := strings.Index(out, "\r\n\r\n")
				if idx < 0 || out[idx+4:] != tc.body {
					t.Errorf("body mismatch: got %q, want %q", out[idx+4:], tc.body)
				}
			}
		})
	}
}

func TestWriteStatusReply_UnknownStatusFallsBackToEmptyReason(t *testing.T) {
	var buf bytes.Buffer
	// 999 has no canonical StatusText; expect "HTTP/1.1 999 \r\n" (empty reason)
	if err := writeStatusReply(&buf, 999, ""); err != nil {
		t.Fatalf("writeStatusReply: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "HTTP/1.1 999 \r\n") {
		t.Errorf("expected empty reason phrase for unknown status, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/filter/hcm && go test .`
Expected: compile error — `serverHeader`, `dateHeader`, `writeStatusReply` undefined.

- [ ] **Step 3: Write `internal/filter/hcm/codec.go`**

```go
package hcm

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// serverHeader returns the canonical Server header value for HCM-locally-
// generated responses. ADR-0014 (admin /ready precedent) reaffirmed for HCM
// per ADR-0044 / SPEC §10 #12 settled.
func serverHeader() string { return "envoy" }

// dateHeader returns the current Date header value formatted as RFC 7231
// IMF-fixdate (e.g. "Sun, 06 Nov 2024 08:49:37 GMT"). Per-response computation
// (no caching) per SPEC §10 #8 settled. Phase-06+ may add caching if profiling
// reveals hot allocation.
func dateHeader() string { return time.Now().UTC().Format(http.TimeFormat) }

// writeStatusReply writes a complete HTTP/1.1 local-reply response to w. The
// status line uses http.StatusText for the reason phrase; if the status is
// unknown to stdlib, the reason phrase is empty. Headers in fixed order:
// Content-Type, Content-Length, Server, Date. A CRLF blank line then the body.
//
// This is the ONLY path in package hcm that locally generates a response
// body. The router action goes through stdlib's Response.Write for proxied
// responses.
func writeStatusReply(w io.Writer, status int, body string) error {
	reason := http.StatusText(status)
	// Status line: "HTTP/1.1 <code> <reason>\r\n" (reason may be empty).
	if _, err := fmt.Fprintf(w, "HTTP/1.1 %d %s\r\n", status, reason); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w,
		"Content-Type: text/plain\r\nContent-Length: %d\r\nServer: %s\r\nDate: %s\r\n\r\n",
		len(body), serverHeader(), dateHeader()); err != nil {
		return err
	}
	if body != "" {
		if _, err := io.WriteString(w, body); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/filter/hcm && go test -run 'TestServerHeader|TestDateHeader|TestWriteStatusReply' -v .`
Expected: every subtest PASS.

- [ ] **Step 5: Append ADR-0037 to `docs/envoy-go/DECISIONS.md`**

Append (after the existing ADR-0036 block; verify the tail one more time before writing):

```markdown
---

## ADR-0037: HTTP/1.1 wire codec source — stdlib `net/http`

**Status:** Accepted
**Date:** <YYYY-MM-DD landing date>
**Doctrine:** D-3.2, D-3.5
**Settles:** SPEC ADR-H, phase-04 §4.1 codec.go

### Context

Phase 04 introduces envoy-go's first HTTP-aware dataplane. The wire codec for HTTP/1.1 request parsing, response readback (for the router action), and local-reply generation must be doctrine-compatible: D-3.2 forbids `net/http/httputil.ReverseProxy`, embedding 3rd-party server cores (Caddy, Traefik, fasthttp), and copying GPL-licensed code. It permits Go standard library use as foundation. Three candidate sources were considered:

- (H1) Handcrafted RFC 7230 / RFC 9112 parser+writer.
- (H2) Stdlib `net/http.ReadRequest`, `Request.Write`, `ReadResponse`, `Response.Write`.
- (H3) Build on `net/http.Server` + `http.Handler`.

### Decision

(H2) is selected. Phase 04's wire codec consumes only stdlib parsers/serializers, never `net/http.Server` and never `http.Handler`.

Rationale:

- (H1) carries an unbounded RFC-corner-case tax that stdlib already pays (chunked encoding, header continuation, Host enforcement, request-target form, header field validation). Hand-rolling these is one to several phases of work for no asserted-surface benefit at phase 04.
- (H3) is forbidden in spirit by D-3.2: `net/http.Server` injects `Date` and `Content-Length` automatically, strips per-RFC headers, enforces RedactHeaders, and assumes HTTP-server semantics that are wrong for a proxy (an Envoy proxy must preserve upstream headers verbatim and is not the canonical authority for response Date/Server values on routed responses). Using `http.Server` would silently introduce divergences from upstream Envoy that cannot be patched out without forking stdlib.
- (H2) keeps the doctrine intent (no `httputil.ReverseProxy`, no third-party server core) while sidestepping (H1)'s tax. The stdlib parsers/serializers are loose enough to use as primitives without inheriting `http.Server`'s magic.

### Consequences

Documented residual stdlib-driven divergences from upstream Envoy that ADR-0044's BEHAVIOR_CONTRACT subsection records:

- Header canonicalization (`textproto.CanonicalMIMEHeaderKey` capitalises header names — `Content-Type`, not `content-type`). Envoy emits lowercase. The phase-04 differential allow-list compares header names case-insensitively (already true in the runner per `helpers.HTTPHeaderDiff`).
- `Host` header validation: stdlib `http.ReadRequest` rejects malformed Host values; Envoy accepts a wider grammar. Phase-04 fixtures issue only well-formed Host values, so the divergence is not exercised.
- Method whitelist: stdlib does not reject custom methods but normalises certain method spellings (e.g., `GET`/`get` round-trip to `GET`). Envoy preserves wire-form. Phase-04 fixtures issue only canonical-spelling methods.
- `Connection` header handling: stdlib's `Request.Write` may add or remove `Connection`-related headers based on the request's `Close` field. Phase-04 router action passes the original request through `Request.Write` after `http.ReadRequest`, so any stdlib-driven change is documented as part of the upstream-side request preservation rule's "bounded normalisation" caveat.

`net/http.Server` and `http.Handler` are NEVER imported by `internal/filter/hcm/`. Code review enforces. Future phases that need HTTP-server-style handling for a non-proxy purpose (e.g., the admin API in phase 08) may use `http.Server` in their own packages — this ADR scopes only the HCM dataplane.

Lands in Task 3 (first use site of `writeStatusReply`).
```

- [ ] **Step 6: Run go vet + golangci-lint on the new files**

```bash
go vet ./internal/filter/hcm/...
golangci-lint run ./internal/filter/hcm/...
```
Expected: both clean.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/hcm/codec.go internal/filter/hcm/codec_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 04: internal/filter/hcm — wire-codec helpers (writeStatusReply, server/date) [ADR-0037]"
```

- [ ] **Step 8: Append PROGRESS.md entry for Task 3**

```markdown
## Task 3 — internal/filter/hcm — codec.go + ADR-0037

**Commits:** <short-sha>
**Notes:** Wire-codec helpers landed; ADR-0037 documents the H2 (stdlib net/http) choice and the residual divergences from upstream Envoy.
**Outputs:**
```
$ cd internal/filter/hcm && go test -run 'TestServerHeader|TestDateHeader|TestWriteStatusReply' -v .
<verbatim>
$ go vet ./internal/filter/hcm/...
<no output>
$ golangci-lint run ./internal/filter/hcm/...
<no output>
```
```

Commit the PROGRESS.md update with `phase 04: PROGRESS SHA-fill for Task 3`.

---

## Task 4: `internal/filter/hcm/route.go` + tests + ADR-0038

**Files:**
- Create: `internal/filter/hcm/route.go`
- Create: `internal/filter/hcm/route_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0038)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 4 entry)

Route table + match engine. Pure stdlib + http; no internal deps. TDD: tests first.

- [ ] **Step 1: Write `internal/filter/hcm/route_test.go`**

```go
package hcm

import (
	"net/http"
	"net/url"
	"testing"
)

func reqWithPath(p string) *http.Request {
	return &http.Request{URL: &url.URL{Path: p}}
}

func TestMatchPath(t *testing.T) {
	m := matchPath("/health")
	if !m.matches("/health") {
		t.Error("matchPath should match exact path")
	}
	if m.matches("/HEALTH") {
		t.Error("matchPath is case-sensitive (per Envoy default)")
	}
	if m.matches("/health/") {
		t.Error("matchPath should NOT match trailing slash")
	}
	if m.matches("/api") {
		t.Error("matchPath should NOT match a different path")
	}
}

func TestMatchPrefix(t *testing.T) {
	m := matchPrefix("/api")
	for _, p := range []string{"/api", "/api/", "/api/v1", "/api/v1/users"} {
		if !m.matches(p) {
			t.Errorf("matchPrefix(/api) should match %q", p)
		}
	}
	// Phase-04 documented divergence: bytewise prefix matches /apifoo (Envoy's
	// segment-aware semantics would not). ADR-0038 records this.
	if !m.matches("/apifoo") {
		t.Error("phase-04 matchPrefix is bytewise; expected to match /apifoo (documented divergence per ADR-0038)")
	}
	if m.matches("/v1/api") {
		t.Error("matchPrefix should not match a path that does not begin with the prefix")
	}
}

func TestRouteTableMatch_FirstMatchWins(t *testing.T) {
	t1 := &routeTable{routes: []routeEntry{
		{match: matchPath("/health")},
		{match: matchPrefix("/")},
	}}
	if e, ok := t1.match(reqWithPath("/health")); !ok || e != &t1.routes[0] {
		t.Error("first-match-wins should resolve /health to routes[0]")
	}
	if e, ok := t1.match(reqWithPath("/anything-else")); !ok || e != &t1.routes[1] {
		t.Error("first-match-wins should resolve /anything-else to routes[1] (catch-all)")
	}
}

func TestRouteTableMatch_QueryStringExcluded(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPrefix("/api")},
	}}
	r := &http.Request{URL: &url.URL{Path: "/api", RawQuery: "q=1"}}
	if _, ok := tt.match(r); !ok {
		t.Error("match should evaluate URL.Path only (query excluded)")
	}
}

func TestRouteTableMatch_NoMatch(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health")},
		{match: matchPrefix("/api")},
	}}
	if _, ok := tt.match(reqWithPath("/missing")); ok {
		t.Error("expected no-match for unrouted path")
	}
}

func TestRouteTableMatch_EmptyTable(t *testing.T) {
	tt := &routeTable{}
	if _, ok := tt.match(reqWithPath("/anything")); ok {
		t.Error("empty route table should never match")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/filter/hcm && go test -run 'TestMatch|TestRouteTable' .`
Expected: compile error — `matchPath`, `matchPrefix`, `routeTable`, `routeEntry` undefined.

- [ ] **Step 3: Write `internal/filter/hcm/route.go`**

```go
package hcm

import (
	"net/http"
	"strings"
)

// routeMatch is the predicate side of a routeEntry. Each route binds exactly
// one match implementation. Phase 04 supports two: matchPrefix (bytewise
// prefix on req.URL.Path) and matchPath (case-sensitive exact equality).
// Other Envoy match shapes (safe_regex, path_separated_prefix, headers, etc.)
// are rejected at config-parse time per ADR-0038.
type routeMatch interface {
	matches(path string) bool
}

// matchPath performs a case-sensitive exact comparison on the request URL's
// Path component. Envoy's default case_sensitive=true is the only mode
// supported in phase 04.
type matchPath string

func (m matchPath) matches(p string) bool { return string(m) == p }

// matchPrefix performs a bytewise prefix match on the request URL's Path
// component. Phase 04 documents a planned divergence from Envoy's
// segment-aware prefix semantics (ADR-0038): "/api" matches "/apifoo" under
// matchPrefix; under Envoy's segment-aware semantics it would not. The
// fixture driver issues only segment-boundary paths, so the divergence is
// not surfaced in the differential gate.
type matchPrefix string

func (m matchPrefix) matches(p string) bool { return strings.HasPrefix(p, string(m)) }

// routeEntry pairs a match predicate with the action to invoke on a hit. The
// action interface is defined in actions.go to keep route.go free of any
// dependency on the cluster manager and bufio.
type routeEntry struct {
	match  routeMatch
	action routeAction
}

// routeTable is the resolved route_config. Routes are evaluated in
// declaration order; first match wins.
type routeTable struct {
	routes []routeEntry
}

// match walks the routes in declaration order, returning the first entry
// whose match predicate accepts req.URL.Path. Returns (nil, false) on no
// match. Query-string is excluded (URL.RawQuery is not considered).
func (t *routeTable) match(req *http.Request) (*routeEntry, bool) {
	p := req.URL.Path
	for i := range t.routes {
		if t.routes[i].match.matches(p) {
			return &t.routes[i], true
		}
	}
	return nil, false
}
```

Note: `route.go` references `routeAction` (defined in `actions.go`). Go's same-package-different-file structure handles this; the `routeAction` interface is declared in `actions.go` (Task 5). To make `route_test.go` compile in this task, add a temporary forward declaration in `route.go` itself — better, use a build-time stub in this task: write `route.go` as shown WITH a placeholder `type routeAction = any` declaration in `route.go`'s body. Task 5 removes the `type routeAction = any` line and adds the real interface in `actions.go`.

Actually, the cleaner discipline is to define the `routeAction` interface in `route.go` itself (since the routeEntry struct is its first consumer) and have `actions.go` provide the implementations. Update `route.go` to:

```go
// routeAction is the action half of a routeEntry. Implementations live in
// actions.go: directResponseAction (synthesizes a local reply) and
// routerAction (proxies via Cluster.Dial). Returning errCloseAfterAction
// from do signals the connection loop to close after this iteration; other
// non-nil errors propagate and trigger downstream close.
type routeAction interface {
	do(ctx context.Context, req *http.Request, bw *bufio.Writer) error
}
```

…and add `import ("bufio"; "context")` to route.go's import block. The interface is empty-of-method-bodies so `route.go` does not depend on `cluster` or `codec`. Task 5 fills in the implementations in `actions.go`.

- [ ] **Step 4: Update tests to use a no-op action stub**

`route_test.go` does not exercise actions; the `routeEntry.action` field is nil in tests. The Go zero value for an interface is `nil`, which is fine because `route_test.go` only calls `routeTable.match`. No test edit needed.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd internal/filter/hcm && go test -run 'TestMatch|TestRouteTable' -v .`
Expected: every subtest PASS.

- [ ] **Step 6: Append ADR-0038 to `docs/envoy-go/DECISIONS.md`**

```markdown
---

## ADR-0038: Phase-04 route match subset — `match.prefix` (bytewise) + `match.path` (case-sensitive exact)

**Status:** Accepted
**Date:** <YYYY-MM-DD>
**Doctrine:** D-3.5
**Settles:** SPEC ADR-J, phase-04 §4.1 route.go

### Context

Envoy's `route.RouteMatch` proto carries a `path_specifier` oneof with seven variants (`prefix`, `path`, `safe_regex`, `path_separated_prefix`, `connect_matcher`, plus deprecated `regex`, plus the never-set `path_match_policy` extension point) and side fields (`headers[]`, `query_parameters[]`, `dynamic_metadata[]`, `runtime_fraction`, `case_sensitive`, `tls_context`, `grpc`). Phase 04's fixture exercises exactly two predicates: an exact-path route for `/health` and a prefix route for `/api`. Implementing the full match surface is at least one phase of work and pulls in a regex engine, segment parser, header-match grammar, and runtime-substitution machinery — all out of phase-04 scope per SPEC §2.

### Decision

Phase 04 supports exactly two match predicates:

- `match.path` (`*routev3.RouteMatch_Path`) — case-sensitive exact comparison on `req.URL.Path`.
- `match.prefix` (`*routev3.RouteMatch_Prefix`) — bytewise prefix match on `req.URL.Path`.

`match.case_sensitive` is honoured only as the Envoy default (`true` or unset/nil pointer); explicitly setting `case_sensitive: false` errors at parse with `hcm: route %d: match.case_sensitive=false is not supported in phase 04`.

Every other `path_specifier` variant errors at parse: `safe_regex`, `path_separated_prefix`, `connect_matcher`, the deprecated `regex`, and any future variant the proto adds. Side fields error: `headers[]` non-empty, `query_parameters[]` non-empty, `dynamic_metadata[]` non-empty, `runtime_fraction` set, `tls_context` set, `grpc` set.

### Documented divergence

Envoy's `match.prefix` is path-segment-aware: `prefix: "/api"` matches `/api`, `/api/`, `/api/x` but NOT `/apifoo`. Phase 04 implements bytewise prefix: `/apifoo` WOULD match. The phase-04 fixture driver does not exercise non-segment-boundary paths; every router-action request uses `/api/v1/<n>`. Therefore:

- The differential gate does not exercise the divergence.
- A future phase that fixes the divergence (by introducing segment-aware prefix matching) does not need to supersede this ADR — it simply tightens the implementation while keeping the proto-level surface (`match.prefix`) the same. ADR-0038's "permitted predicates" list does not change.
- A future fixture that DOES exercise non-segment-boundary paths must either rely on the segment-aware tightening or extend BEHAVIOR_CONTRACT with a fixture-specific assertion.

### Consequences

- The phase-04 ignored-set (ADR-0041) does NOT include `match.case_sensitive` because phase 04 explicitly errors on `case_sensitive: false`.
- BEHAVIOR_CONTRACT's HTTP/1.1 subsection (ADR-0044) records "route-match selection equivalence" as an asserted dimension; the divergence above is permitted because the asserted surface is path-equivalence on segment-boundary inputs.
- Phase 07's filter-chain framework + HTTP-filter family supersede this ADR's "what predicates are supported" list (not the underlying ADR; phase 07's ADR records the expanded predicate set + tightened semantics).

Lands in Task 4 (first use site of `routeTable.match`).
```

- [ ] **Step 7: Run go vet + golangci-lint**

```bash
go vet ./internal/filter/hcm/...
golangci-lint run ./internal/filter/hcm/...
```
Expected: both clean.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/hcm/route.go internal/filter/hcm/route_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 04: internal/filter/hcm — route table + match engine [ADR-0038]"
```

- [ ] **Step 9: Append PROGRESS.md entry for Task 4** (mirror the Task 3 shape; commit as `phase 04: PROGRESS SHA-fill for Task 4`).

---

## Task 5: `internal/filter/hcm/actions.go` + tests + ADR-0039

**Files:**
- Create: `internal/filter/hcm/actions.go`
- Create: `internal/filter/hcm/actions_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0039)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 5 entry)

The two action implementations + the `errCloseAfterAction` sentinel. Depends on `codec.go` (writeStatusReply) and `internal/cluster` (Cluster.Dial). TDD: tests first.

- [ ] **Step 1: Write `internal/filter/hcm/actions_test.go`**

```go
package hcm

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDirectResponseAction_Do(t *testing.T) {
	a := &directResponseAction{status: 200, body: "OK\n"}
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := a.do(context.Background(), &http.Request{}, bw); err != nil {
		t.Fatalf("do: %v", err)
	}
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("expected 200 OK status line, got: %q", out)
	}
	if !strings.HasSuffix(out, "OK\n") {
		t.Errorf("expected body 'OK\\n' suffix, got: %q", out)
	}
}

// loopbackHTTPEcho starts a tiny HTTP/1.1 echo server and returns its address
// + a stop function. The server reads one request, writes one response with
// body "echo:<URL.Path>", then closes. Used to exercise routerAction.
func loopbackHTTPEcho(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				body := "echo:" + req.URL.Path
				_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: " +
					stringLen(len(body)) + "\r\nConnection: close\r\n\r\n" + body))
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close(); <-done }
}

func stringLen(n int) string {
	// avoid pulling fmt into the test for one purpose
	return strings.TrimSpace((bytes.NewBufferString("")).String() + itoa(n))
}

func itoa(n int) string {
	// minimal-allocation int→string for the test
	if n == 0 { return "0" }
	var b [20]byte; i := len(b)
	for n > 0 { i--; b[i] = byte('0' + n%10); n /= 10 }
	return string(b[i:])
}

func TestRouterAction_DoHappy(t *testing.T) {
	addr, stop := loopbackHTTPEcho(t)
	defer stop()

	// Single-endpoint cluster pointing at the loopback echo.
	cm, c := singleEndpointCluster(t, addr)
	_ = cm
	a := &routerAction{cluster: c}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := a.do(req.Context(), req, bw); err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = bw.Flush()
	if !strings.Contains(buf.String(), "echo:/x") {
		t.Errorf("expected echo:/x in response, got: %q", buf.String())
	}
}

func TestRouterAction_DoDialFailureReturns503(t *testing.T) {
	// Cluster with an unreachable endpoint (port 1 is always rejected).
	cm, c := singleEndpointCluster(t, "127.0.0.1:1")
	_ = cm
	a := &routerAction{cluster: c}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := a.do(req.Context(), req, bw); err != nil {
		// dial-failure becomes a 503 LOCAL REPLY; do() should NOT error
		// (it writes the local reply and returns nil).
		if !errors.Is(err, errCloseAfterAction) {
			t.Errorf("dial failure should write 503 and return nil (or sentinel), got: %v", err)
		}
	}
	_ = bw.Flush()
	if !strings.HasPrefix(buf.String(), "HTTP/1.1 503 ") {
		t.Errorf("expected 503 local reply on dial failure, got: %q", buf.String())
	}
}

func TestRouterAction_DoCtxCancel(t *testing.T) {
	addr, stop := loopbackHTTPEcho(t)
	defer stop()

	cm, c := singleEndpointCluster(t, addr)
	_ = cm
	a := &routerAction{cluster: c}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before do — Cluster.Dial(ctx) should return ctx.Err()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	_ = a.do(ctx, req, bw)
	_ = bw.Flush()
	if !strings.HasPrefix(buf.String(), "HTTP/1.1 503 ") {
		t.Errorf("ctx cancel should produce 503 local reply, got: %q", buf.String())
	}
}

// helper: build a single-endpoint cluster manager pointing at addr; sourced
// from internal/cluster's existing test helpers if any, or constructed inline.
// See internal/cluster/cluster_test.go for the shape; this test mirrors it.
func singleEndpointCluster(t *testing.T, addr string) (interface{}, *clusterRef) {
	t.Helper()
	// Implementation note: build a minimal *cluster.Cluster directly via the
	// package's exported constructor or via an in-test bootstrap proto. The
	// exact code shape depends on the executor's read of internal/cluster's
	// API — pick whichever is least invasive for a one-endpoint loopback
	// cluster. The clusterRef alias here exists to keep this file
	// self-documenting; replace with the real *cluster.Cluster type at
	// implementation time.
	_ = time.Second // keep the time import warm for any deadline use
	t.Fatalf("TODO: instantiate *cluster.Cluster pointing at %s; see internal/cluster/cluster_test.go for the helper pattern", addr)
	return nil, nil
}
type clusterRef = struct{} // placeholder; replace with *cluster.Cluster at impl time
```

(The `singleEndpointCluster` test helper is a stub the executor fills in by reading `internal/cluster/cluster_test.go` for the existing pattern. This is intentional — the plan does not pre-commit the helper shape because the executor sees the live `internal/cluster` API at task-execution time. The `clusterRef = struct{}` placeholder must be replaced with `*cluster.Cluster`.)

- [ ] **Step 2: Run test to verify it fails (compile error expected)**

Run: `cd internal/filter/hcm && go test -run 'TestDirectResponse|TestRouterAction' .`
Expected: compile error — `directResponseAction`, `routerAction`, `errCloseAfterAction` undefined.

- [ ] **Step 3: Write `internal/filter/hcm/actions.go`**

```go
package hcm

import (
	"bufio"
	"context"
	"errors"
	"net/http"

	"github.com/esalaine/envoy-go/internal/cluster"
)

// errCloseAfterAction is returned by routeAction.do when the action's
// response carried Connection: close (or the equivalent semantic on the
// upstream-routed response). The connection loop checks for this sentinel
// via errors.Is and closes the downstream after the current iteration.
//
// SPEC §10 #3 settled to the sentinel-error mechanism (option (a)). Other
// non-nil errors from do trigger downstream close + log (the connection
// loop handles).
var errCloseAfterAction = errors.New("hcm: action requested connection close")

// directResponseAction synthesizes a local-reply HTTP/1.1 response. body is
// the inline_string contents. status must be in [100, 599] (validated at
// config-parse time per SPEC §2). direct_response participates in keep-alive;
// it never returns errCloseAfterAction.
type directResponseAction struct {
	status int
	body   string
}

func (a *directResponseAction) do(_ context.Context, _ *http.Request, bw *bufio.Writer) error {
	return writeStatusReply(bw, a.status, a.body)
}

// routerAction proxies the request to the named cluster's selected endpoint.
// Per ADR-0039, every routed request opens a fresh upstream connection via
// Cluster.Dial(ctx); no pooling at phase 04. Per-failure-class mapping:
//   Cluster.Dial error      → 503 local reply, do returns nil
//   Request.Write error     → 502 local reply, do returns nil
//   http.ReadResponse error → 502 local reply, do returns nil
//   resp.Write error        → propagated up (downstream is broken)
//
// The router does NOT inject x-envoy-*, x-forwarded-*, or x-request-id
// headers (SPEC §2). The upstream sees the unmodified downstream request
// (modulo stdlib's textproto canonicalization on header names).
type routerAction struct {
	cluster *cluster.Cluster
}

func (a *routerAction) do(ctx context.Context, req *http.Request, bw *bufio.Writer) error {
	upstream, err := a.cluster.Dial(ctx)
	if err != nil {
		return writeStatusReply(bw, 503, "")
	}
	defer func() { _ = upstream.Close() }()

	// Preserve the original request verbatim. stdlib req.Write handles
	// chunked vs Content-Length framing automatically based on what was read.
	if err := req.Write(upstream); err != nil {
		return writeStatusReply(bw, 502, "")
	}

	resp, err := http.ReadResponse(bufio.NewReader(upstream), req)
	if err != nil {
		return writeStatusReply(bw, 502, "")
	}
	defer func() { _ = resp.Body.Close() }()

	// resp.Write writes the full HTTP/1.1 response back to bw (status line,
	// headers, body). Any I/O failure here means the downstream is broken;
	// propagate.
	return resp.Write(bw)
}
```

- [ ] **Step 4: Run test to verify the direct-response cases pass; finish the cluster helper for the routerAction tests**

The executor reads `internal/cluster/cluster_test.go` to learn how to instantiate a single-endpoint plaintext cluster, replaces the `singleEndpointCluster` stub in `actions_test.go`, then re-runs:

```bash
cd internal/filter/hcm && go test -run 'TestDirectResponse|TestRouterAction' -v .
```

Expected: every subtest PASS.

- [ ] **Step 5: Append ADR-0039 to `docs/envoy-go/DECISIONS.md`**

```markdown
---

## ADR-0039: Per-request fresh upstream dial in phase-04 router

**Status:** Accepted
**Date:** <YYYY-MM-DD>
**Doctrine:** D-3.3, D-3.5
**Settles:** SPEC ADR-L, phase-04 §4.1 actions.go

### Context

The router action in phase 04 must move bytes from the downstream request to a selected upstream endpoint and back. Two upstream-connection strategies are possible: (a) per-request fresh dial — every routed request opens a new TCP connection via `Cluster.Dial(ctx)` and closes it after the response is written; (b) connection pooling — keep a per-endpoint pool of upstream connections, idle-evict, max-streams, etc. Upstream Envoy uses (b). Implementing (b) faithfully is at least one phase of upstream-robustness work (timeouts, idle eviction, max-streams, idle-stream-cleanup, max-concurrent-streams-per-connection, draining-on-shutdown).

### Decision

Phase 04 picks (a) — per-request fresh dial. The router action calls `cluster.Dial(ctx)` on every request, defers `upstream.Close()`, and lets the connection close after the response is fully written.

### Consequences

- Performance is suboptimal vs upstream Envoy (extra TCP handshake per request, no upstream-side keep-alive). The differential gate does not assert connection-reuse, so the divergence is permitted.
- Per-request `cluster.Dial` is what makes the round-robin distribution `[3,3,3]` deterministic on the subject side: every request takes one endpoint pick from the cluster's RR state, mod-3 partition over 9 requests. A pooled implementation would need different distribution-witness arithmetic.
- Pool semantics land in the upstream-robustness family. That phase's ADR supersedes this one for the dial-strategy choice.
- BEHAVIOR_CONTRACT's HTTP/1.1 subsection (ADR-0044) explicitly enumerates "upstream connection re-use" under "Not asserted" so future phases can change the strategy without breaking the contract.

Lands in Task 5 (first use site of `routerAction.do`).
```

- [ ] **Step 6: Commit**

```bash
git add internal/filter/hcm/actions.go internal/filter/hcm/actions_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 04: internal/filter/hcm — directResponseAction + routerAction [ADR-0039]"
```

- [ ] **Step 7: Append PROGRESS.md entry for Task 5** (Task-3-shaped; commit `phase 04: PROGRESS SHA-fill for Task 5`).

---

## Task 6: `internal/filter/hcm/connection.go` + tests

**Files:**
- Create: `internal/filter/hcm/connection.go`
- Create: `internal/filter/hcm/connection_test.go`
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 6 entry)

The per-conn loop. Depends on codec.go, route.go, actions.go. No new ADR (the connection-loop shape is implementation, not cross-phase decision). TDD: tests first.

- [ ] **Step 1: Write `internal/filter/hcm/connection_test.go`**

```go
package hcm

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// connPair returns a connected pair of net.Conn, both ends in-process.
func connPair(t *testing.T) (clientSide, serverSide net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	type result struct{ c net.Conn; err error }
	ch := make(chan result, 1)
	go func() {
		c, err := ln.Accept()
		ch <- result{c, err}
	}()
	c1, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	return c1, r.c
}

func writeRequest(t *testing.T, w io.Writer, method, path string, headers ...string) {
	t.Helper()
	hdr := "Host: example\r\n"
	for _, h := range headers {
		hdr += h + "\r\n"
	}
	_, err := io.WriteString(w, method+" "+path+" HTTP/1.1\r\n"+hdr+"Content-Length: 0\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
}

func readResponseStatus(t *testing.T, r io.Reader) int {
	t.Helper()
	resp, err := http.ReadResponse(bufio.NewReader(r), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestRunConnection_DirectResponseHappy(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, body: "OK\n"}},
	}}
	client, server := connPair(t)
	defer client.Close()

	go runConnection(context.Background(), server, tt)

	writeRequest(t, client, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Errorf("status: got %d, want 200", got)
	}
}

func TestRunConnection_KeepAliveTwoRequests(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, body: "OK\n"}},
	}}
	client, server := connPair(t)
	defer client.Close()
	go runConnection(context.Background(), server, tt)

	// Two pipelined requests; second one carries Connection: close.
	writeRequest(t, client, "GET", "/health")
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("first status: got %d, want 200", got)
	}
	writeRequest(t, client, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("second status: got %d, want 200", got)
	}
}

func TestRunConnection_RouteNotFoundReturns404(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, body: "OK\n"}},
	}}
	client, server := connPair(t)
	defer client.Close()
	go runConnection(context.Background(), server, tt)

	writeRequest(t, client, "GET", "/missing", "Connection: close")
	if got := readResponseStatus(t, client); got != 404 {
		t.Errorf("status: got %d, want 404", got)
	}
}

func TestRunConnection_ExpectHeaderReturns417(t *testing.T) {
	tt := &routeTable{}
	client, server := connPair(t)
	defer client.Close()
	go runConnection(context.Background(), server, tt)

	writeRequest(t, client, "GET", "/x", "Expect: 100-continue", "Connection: close")
	if got := readResponseStatus(t, client); got != 417 {
		t.Errorf("status: got %d, want 417", got)
	}
}

func TestRunConnection_UpgradeReturns501(t *testing.T) {
	tt := &routeTable{}
	client, server := connPair(t)
	defer client.Close()
	go runConnection(context.Background(), server, tt)

	writeRequest(t, client, "GET", "/x", "Upgrade: websocket", "Connection: Upgrade")
	if got := readResponseStatus(t, client); got != 501 {
		t.Errorf("status: got %d, want 501", got)
	}
}

func TestRunConnection_BadRequestReturns400(t *testing.T) {
	tt := &routeTable{}
	client, server := connPair(t)
	defer client.Close()
	go runConnection(context.Background(), server, tt)

	// Malformed: no Host header, malformed request line.
	if _, err := io.WriteString(client, "GARBAGE\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if got := readResponseStatus(t, client); got != 400 {
		t.Errorf("status: got %d, want 400", got)
	}
}

func TestRunConnection_BodyDrainedBetweenRequests(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/post"), action: &directResponseAction{status: 200, body: "ok\n"}},
	}}
	client, server := connPair(t)
	defer client.Close()
	go runConnection(context.Background(), server, tt)

	// First request with a body.
	body := strings.Repeat("x", 64)
	if _, err := io.WriteString(client,
		"POST /post HTTP/1.1\r\nHost: example\r\nContent-Length: "+itoa(len(body))+"\r\n\r\n"+body); err != nil {
		t.Fatal(err)
	}
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("first status: got %d, want 200", got)
	}
	// Second request immediately, with Connection: close.
	writeRequest(t, client, "GET", "/post", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Errorf("second status (post-drain): got %d, want 200", got)
	}
	_ = time.Second // keep the time import warm if needed
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/filter/hcm && go test -run 'TestRunConnection' .`
Expected: compile error — `runConnection` undefined.

- [ ] **Step 3: Write `internal/filter/hcm/connection.go`**

```go
package hcm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

// runConnection drives one downstream HTTP/1.1 connection from acceptance to
// close. The loop reads requests via http.ReadRequest off a bufio.Reader
// over downstream, applies phase-04 out-of-scope guards (Expect:→417,
// Upgrade:→501), dispatches each request through the route table, and exits
// on:
//   - clean EOF from the downstream
//   - any non-EOF parse error (a 400 is sent before close)
//   - phase-04-out-of-scope guard trip (417 or 501 before close)
//   - Connection: close on the request OR the response signalling
//     errCloseAfterAction
//
// Per SPEC §5.3 / §5.6 / SPEC §10 #3 settled.
func runConnection(ctx context.Context, downstream net.Conn, table *routeTable) {
	defer func() { _ = downstream.Close() }()

	br := bufio.NewReader(downstream)
	bw := bufio.NewWriter(downstream)

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		req, err := http.ReadRequest(br)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_ = writeStatusReply(bw, 400, "")
				_ = bw.Flush()
			}
			return
		}

		// Phase-04 out-of-scope guards.
		if req.Header.Get("Expect") != "" {
			_ = writeStatusReply(bw, 417, "")
			_ = bw.Flush()
			drainAndClose(req)
			return
		}
		if req.Header.Get("Upgrade") != "" || strings.EqualFold(req.Header.Get("Connection"), "Upgrade") {
			_ = writeStatusReply(bw, 501, "")
			_ = bw.Flush()
			drainAndClose(req)
			return
		}

		closeAfterRequest := strings.EqualFold(req.Header.Get("Connection"), "close")
		closeAfterAction := false

		entry, ok := table.match(req)
		if !ok {
			_ = writeStatusReply(bw, 404, "")
		} else {
			actErr := entry.action.do(ctx, req, bw)
			if errors.Is(actErr, errCloseAfterAction) {
				closeAfterAction = true
			} else if actErr != nil {
				log.Printf("hcm: action: %v", actErr)
				_ = bw.Flush()
				drainAndClose(req)
				return
			}
		}

		if err := bw.Flush(); err != nil {
			drainAndClose(req)
			return
		}

		// Drain the request body; mandatory for HTTP/1.1 keep-alive correctness.
		drainAndClose(req)

		if closeAfterRequest || closeAfterAction {
			return
		}
	}
}

// drainAndClose discards any unread request body bytes and closes the body.
// Without this, the next iteration's http.ReadRequest would read stale body
// bytes as the request line.
func drainAndClose(req *http.Request) {
	if req.Body != nil {
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/filter/hcm && go test -run 'TestRunConnection' -v .`
Expected: every subtest PASS.

- [ ] **Step 5: Race-detector run**

Run: `cd internal/filter/hcm && go test -race -run 'TestRunConnection' .`
Expected: PASS, no data races reported.

- [ ] **Step 6: Run go vet + golangci-lint**

```bash
go vet ./internal/filter/hcm/...
golangci-lint run ./internal/filter/hcm/...
```
Expected: both clean.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/hcm/connection.go internal/filter/hcm/connection_test.go
git commit -m "phase 04: internal/filter/hcm — per-conn loop (runConnection)"
```

- [ ] **Step 8: Append PROGRESS.md entry for Task 6** (Task-3-shaped; SHA-fill follow-up commit).

---

## Task 7: `internal/filter/hcm/config.go` + tests + ADR-0040 + ADR-0041 + ADR-0042

**Files:**
- Create: `internal/filter/hcm/config.go`
- Create: `internal/filter/hcm/config_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0040, ADR-0041, ADR-0042 — three sequential entries in a single commit)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 7 entry)

The HCM typed_config parser. The largest task in the plan and the natural site for three ADRs. TDD: tests first; expect this task to take longer than its peers.

- [ ] **Step 1: Sketch the test surface in `internal/filter/hcm/config_test.go`**

The config_test surface enumerates every error class from SPEC §2 + every happy path. Structure:

```go
package hcm

import (
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

// helpers — build a minimal valid HCM proto and an Any wrapping it; tests
// mutate one field at a time to provoke the targeted error class.

func mkRouter() *anypb.Any {
	a, _ := anypb.New(&routerv3.Router{})
	return a
}

func mkHCM(modify func(*hcmv3.HttpConnectionManager)) *anypb.Any {
	hcm := &hcmv3.HttpConnectionManager{
		CodecType:  hcmv3.HttpConnectionManager_HTTP1,
		StatPrefix: "ingress_http",
		RouteSpecifier: &hcmv3.HttpConnectionManager_RouteConfig{
			RouteConfig: &routev3.RouteConfiguration{
				VirtualHosts: []*routev3.VirtualHost{{
					Name:    "vh_default",
					Domains: []string{"*"},
					Routes: []*routev3.Route{{
						Match: &routev3.RouteMatch{PathSpecifier: &routev3.RouteMatch_Path{Path: "/health"}},
						Action: &routev3.Route_DirectResponse{DirectResponse: &routev3.DirectResponseAction{
							Status: 200,
							Body:   &corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: "OK\n"}},
						}},
					}},
				}},
			},
		},
		HttpFilters: []*hcmv3.HttpFilter{{
			Name:       "envoy.filters.http.router",
			ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: mkRouter()},
		}},
	}
	if modify != nil {
		modify(hcm)
	}
	any, _ := anypb.New(hcm)
	return any
}

// One-cluster manager for tests that exercise routerAction parsing.
func mkClusterManager(t *testing.T) *cluster.Manager { /* … see internal/cluster test helpers … */ return nil }

func TestParseFilter_Happy(t *testing.T) {
	cm := mkClusterManager(t)
	if _, err := NewFilter(mkHCM(nil), cm); err != nil {
		t.Fatalf("happy: %v", err)
	}
}

// — error-class tests, one t.Run per class. Each modifier mutates one field;
//   the test asserts err != nil and err message begins with "hcm:".

func TestParseFilter_WrongTypeURL(t *testing.T) { /* …Any with type_url of google.protobuf.StringValue… */ }
func TestParseFilter_CodecTypeHTTP2(t *testing.T) { /* hcm.CodecType = HTTP2 → error */ }
func TestParseFilter_CodecTypeHTTP3(t *testing.T) { /* HTTP3 → error */ }
func TestParseFilter_CodecTypeAUTO(t *testing.T)  { /* AUTO → ok (alias for HTTP1) */ }
func TestParseFilter_MissingStatPrefix(t *testing.T) { /* stat_prefix = "" → error */ }
func TestParseFilter_RDSRouteSpecifier(t *testing.T) { /* RDS → error */ }
func TestParseFilter_ScopedRoutes(t *testing.T) { /* ScopedRoutes → error */ }
func TestParseFilter_ZeroVirtualHosts(t *testing.T) { /* virtual_hosts: [] → error */ }
func TestParseFilter_TwoVirtualHosts(t *testing.T)  { /* virtual_hosts: [a, b] → error */ }
func TestParseFilter_VHostDomainsEmpty(t *testing.T) { /* vh.domains: [] → error */ }
func TestParseFilter_VHostDomainsNotStarOnly(t *testing.T) { /* vh.domains: ["example.com"] → error */ }
func TestParseFilter_HTTPFiltersEmpty(t *testing.T) { /* http_filters: [] → error */ }
func TestParseFilter_HTTPFiltersTwoEntries(t *testing.T) { /* two filters → error */ }
func TestParseFilter_HTTPFiltersWrongName(t *testing.T) { /* name != envoy.filters.http.router → error */ }
func TestParseFilter_HTTPFiltersWrongTypeURL(t *testing.T) { /* typed_config not Router → error */ }
func TestParseFilter_RouteUnknownAction(t *testing.T) { /* RouteAction with redirect → error */ }
func TestParseFilter_RouteSafeRegex(t *testing.T) { /* match.SafeRegex → error */ }
func TestParseFilter_RoutePathSeparatedPrefix(t *testing.T) { /* match.PathSeparatedPrefix → error */ }
func TestParseFilter_RouteCaseSensitiveFalse(t *testing.T) { /* match.case_sensitive: false → error */ }
func TestParseFilter_RouteHeadersSet(t *testing.T) { /* match.headers != nil → error */ }
func TestParseFilter_RouteQueryParamsSet(t *testing.T) { /* match.query_parameters != nil → error */ }
func TestParseFilter_RouteRuntimeFraction(t *testing.T) { /* match.runtime_fraction != nil → error */ }
func TestParseFilter_DirectResponseStatusZero(t *testing.T) { /* status: 0 → error */ }
func TestParseFilter_DirectResponseStatus600(t *testing.T) { /* status: 600 → error */ }
func TestParseFilter_DirectResponseInlineBytes(t *testing.T) { /* body.inline_bytes set → error */ }
func TestParseFilter_DirectResponseFilename(t *testing.T) { /* body.filename set → error */ }
func TestParseFilter_DirectResponseEmptyBody(t *testing.T) { /* body.inline_string = "" → error */ }
func TestParseFilter_RouterActionWeightedClusters(t *testing.T) { /* weighted_clusters → error */ }
func TestParseFilter_RouterActionClusterHeader(t *testing.T) { /* cluster_header → error */ }
func TestParseFilter_RouterActionUnknownCluster(t *testing.T) { /* cluster: "c_missing" → error */ }
func TestParseFilter_AllErrorsBeginWithHCMPrefix(t *testing.T) {
	// Roll-up: collect all error classes' messages, assert each begins with "hcm: "
}
```

The executor expands each `t.Run` body using a small modifier function passed to `mkHCM`. The shape stays consistent across all 28 error-class tests.

- [ ] **Step 2: Run test scaffolding to verify it fails (compile error)**

Run: `cd internal/filter/hcm && go test -run 'TestParseFilter' .`
Expected: compile error — `NewFilter` undefined (also any `mkClusterManager` helper that depends on the not-yet-existing config.go).

- [ ] **Step 3: Write `internal/filter/hcm/config.go`**

```go
package hcm

import (
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

const (
	// TypeURL is the proto descriptor URL for HttpConnectionManager. Registered
	// in internal/listener/manager.go's filterRegistry alongside tcpproxy.TypeURL.
	TypeURL = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"

	// routerFilterName is the canonical Envoy name for the router HTTP filter.
	// SPEC §4.1 and ADR-0040 require it as the only permitted http_filters entry.
	routerFilterName = "envoy.filters.http.router"

	// routerTypeURL is the proto descriptor URL for the Router HTTP filter.
	routerTypeURL = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
)

// parseFilter decodes the typed_config Any into a *Filter. All errors begin
// with "hcm: ". See ADR-0040 (HTTP-filter framework subset), ADR-0041
// (stat_prefix + ignored-set), ADR-0042 (HTTP-filter chain shape), ADR-0038
// (route match subset), and SPEC §2/§9.
func parseFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error) {
	if got := tc.GetTypeUrl(); got != TypeURL {
		return nil, fmt.Errorf("hcm: wrong type_url %q (want %q)", got, TypeURL)
	}
	msg := &hcmv3.HttpConnectionManager{}
	if err := tc.UnmarshalTo(msg); err != nil {
		return nil, fmt.Errorf("hcm: unmarshal: %w", err)
	}

	// codec_type: HTTP1 or AUTO only (AUTO is treated as HTTP1 per SPEC §2).
	switch msg.GetCodecType() {
	case hcmv3.HttpConnectionManager_HTTP1, hcmv3.HttpConnectionManager_AUTO:
		// ok
	default:
		return nil, fmt.Errorf("hcm: codec_type %s is not supported in phase 04 (HTTP/1.1 only)", msg.GetCodecType())
	}

	// stat_prefix: mandatory non-empty string.
	statPrefix := msg.GetStatPrefix()
	if statPrefix == "" {
		return nil, fmt.Errorf("hcm: stat_prefix is required")
	}

	// route_specifier: must be inline route_config.
	rc, err := requireInlineRouteConfig(msg)
	if err != nil {
		return nil, err
	}

	// virtual_hosts[]: exactly one with domains: ["*"].
	if got := len(rc.GetVirtualHosts()); got != 1 {
		return nil, fmt.Errorf("hcm: route_config: virtual_hosts: got %d, want exactly 1", got)
	}
	vh := rc.GetVirtualHosts()[0]
	if domains := vh.GetDomains(); len(domains) != 1 || domains[0] != "*" {
		return nil, fmt.Errorf("hcm: route_config: virtual_hosts[0]: domains: got %v, want [\"*\"]", domains)
	}

	// http_filters[]: exactly one router by name + type_url.
	if err := requireRouterOnlyHTTPFilters(msg.GetHttpFilters()); err != nil {
		return nil, err
	}

	// Routes: build the in-memory route table.
	table, err := buildRouteTable(vh.GetRoutes(), clusters)
	if err != nil {
		return nil, err
	}

	return &Filter{table: table, clusters: clusters, statPrefix: statPrefix}, nil
}

func requireInlineRouteConfig(msg *hcmv3.HttpConnectionManager) (*routev3.RouteConfiguration, error) {
	switch rs := msg.GetRouteSpecifier().(type) {
	case *hcmv3.HttpConnectionManager_RouteConfig:
		if rs.RouteConfig == nil {
			return nil, fmt.Errorf("hcm: route_config is nil")
		}
		return rs.RouteConfig, nil
	case *hcmv3.HttpConnectionManager_Rds:
		return nil, fmt.Errorf("hcm: route_specifier=rds is not supported in phase 04")
	case *hcmv3.HttpConnectionManager_ScopedRoutes:
		return nil, fmt.Errorf("hcm: route_specifier=scoped_routes is not supported in phase 04")
	default:
		return nil, fmt.Errorf("hcm: route_specifier missing or of unsupported type %T", rs)
	}
}

func requireRouterOnlyHTTPFilters(filters []*hcmv3.HttpFilter) error {
	if len(filters) != 1 {
		return fmt.Errorf("hcm: http_filters: got %d entries, want exactly 1 (router only) per ADR-0042", len(filters))
	}
	f := filters[0]
	if f.GetName() != routerFilterName {
		return fmt.Errorf("hcm: http_filters[0]: name %q, want %q", f.GetName(), routerFilterName)
	}
	tc, ok := f.GetConfigType().(*hcmv3.HttpFilter_TypedConfig)
	if !ok || tc.TypedConfig == nil {
		return fmt.Errorf("hcm: http_filters[0]: typed_config is missing")
	}
	if got := tc.TypedConfig.GetTypeUrl(); got != routerTypeURL {
		return fmt.Errorf("hcm: http_filters[0]: typed_config type_url %q, want %q", got, routerTypeURL)
	}
	// The Router proto body is unmarshalled but every Router-proto field is
	// silently ignored at phase 04 per ADR-0041.
	if err := tc.TypedConfig.UnmarshalTo(&routerv3.Router{}); err != nil {
		return fmt.Errorf("hcm: http_filters[0]: typed_config unmarshal: %w", err)
	}
	return nil
}

func buildRouteTable(routes []*routev3.Route, clusters *cluster.Manager) (*routeTable, error) {
	t := &routeTable{routes: make([]routeEntry, 0, len(routes))}
	for i, r := range routes {
		match, err := buildMatch(r.GetMatch())
		if err != nil {
			return nil, fmt.Errorf("hcm: route %d: %w", i, err)
		}
		action, err := buildAction(r.GetAction(), clusters)
		if err != nil {
			return nil, fmt.Errorf("hcm: route %d: %w", i, err)
		}
		t.routes = append(t.routes, routeEntry{match: match, action: action})
	}
	return t, nil
}

func buildMatch(m *routev3.RouteMatch) (routeMatch, error) {
	if m == nil {
		return nil, fmt.Errorf("match is missing")
	}
	if m.GetCaseSensitive() != nil && !m.GetCaseSensitive().GetValue() {
		return nil, fmt.Errorf("match.case_sensitive=false is not supported in phase 04")
	}
	if len(m.GetHeaders()) > 0 {
		return nil, fmt.Errorf("match.headers is not supported in phase 04")
	}
	if len(m.GetQueryParameters()) > 0 {
		return nil, fmt.Errorf("match.query_parameters is not supported in phase 04")
	}
	if m.GetRuntimeFraction() != nil {
		return nil, fmt.Errorf("match.runtime_fraction is not supported in phase 04")
	}
	if len(m.GetDynamicMetadata()) > 0 {
		return nil, fmt.Errorf("match.dynamic_metadata is not supported in phase 04")
	}
	if m.GetTlsContext() != nil {
		return nil, fmt.Errorf("match.tls_context is not supported in phase 04")
	}
	switch ps := m.GetPathSpecifier().(type) {
	case *routev3.RouteMatch_Path:
		if ps.Path == "" {
			return nil, fmt.Errorf("match.path is empty")
		}
		return matchPath(ps.Path), nil
	case *routev3.RouteMatch_Prefix:
		if ps.Prefix == "" {
			return nil, fmt.Errorf("match.prefix is empty")
		}
		return matchPrefix(ps.Prefix), nil
	case *routev3.RouteMatch_SafeRegex:
		return nil, fmt.Errorf("match.safe_regex is not supported in phase 04")
	case *routev3.RouteMatch_PathSeparatedPrefix:
		return nil, fmt.Errorf("match.path_separated_prefix is not supported in phase 04")
	case *routev3.RouteMatch_ConnectMatcher_:
		return nil, fmt.Errorf("match.connect_matcher is not supported in phase 04")
	default:
		return nil, fmt.Errorf("match.path_specifier is missing or of unsupported type %T", ps)
	}
}

func buildAction(a interface{}, clusters *cluster.Manager) (routeAction, error) {
	switch act := a.(type) {
	case *routev3.Route_Route:
		return buildRouterAction(act.Route, clusters)
	case *routev3.Route_DirectResponse:
		return buildDirectResponseAction(act.DirectResponse)
	case nil:
		return nil, fmt.Errorf("action is missing")
	default:
		return nil, fmt.Errorf("action %T is not supported in phase 04", act)
	}
}

func buildRouterAction(r *routev3.RouteAction, clusters *cluster.Manager) (*routerAction, error) {
	if r == nil {
		return nil, fmt.Errorf("route action is nil")
	}
	cs, ok := r.GetClusterSpecifier().(*routev3.RouteAction_Cluster)
	if !ok {
		return nil, fmt.Errorf("route action: cluster_specifier %T is not supported in phase 04 (literal cluster name only)", r.GetClusterSpecifier())
	}
	if cs.Cluster == "" {
		return nil, fmt.Errorf("route action: cluster name is empty")
	}
	c, ok := clusters.Get(cs.Cluster)
	if !ok {
		return nil, fmt.Errorf("route action: cluster %q not found", cs.Cluster)
	}
	return &routerAction{cluster: c}, nil
}

func buildDirectResponseAction(d *routev3.DirectResponseAction) (*directResponseAction, error) {
	if d == nil {
		return nil, fmt.Errorf("direct_response is nil")
	}
	if d.Status < 100 || d.Status >= 600 {
		return nil, fmt.Errorf("direct_response.status %d out of range [100, 599]", d.Status)
	}
	body := d.GetBody()
	if body == nil {
		return nil, fmt.Errorf("direct_response.body is required")
	}
	is, ok := body.GetSpecifier().(*corev3.DataSource_InlineString)
	if !ok {
		return nil, fmt.Errorf("direct_response.body: only inline_string is supported in phase 04 (got %T)", body.GetSpecifier())
	}
	if is.InlineString == "" {
		return nil, fmt.Errorf("direct_response.body.inline_string is empty")
	}
	return &directResponseAction{status: int(d.Status), body: is.InlineString}, nil
}
```

- [ ] **Step 4: Fill in test bodies + run tests**

The executor expands each `t.Run` body and runs `cd internal/filter/hcm && go test -run 'TestParseFilter' -v .` until every error class is covered green.

- [ ] **Step 5: Append ADR-0040, ADR-0041, ADR-0042 to `docs/envoy-go/DECISIONS.md`**

(Three sequential ADR blocks; content per the summaries in `## ADRs introduced by this plan`. Each carries Status: Accepted, Date, Doctrine, Settles, Context, Decision, Consequences.)

- [ ] **Step 6: Run go vet + golangci-lint**

```bash
go vet ./internal/filter/hcm/...
golangci-lint run ./internal/filter/hcm/...
```
Expected: both clean.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/hcm/config.go internal/filter/hcm/config_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 04: internal/filter/hcm — typed_config parser [ADR-0040, ADR-0041, ADR-0042]"
```

- [ ] **Step 8: Append PROGRESS.md entry for Task 7** (Task-3-shaped; SHA-fill follow-up commit). The Notes line records that this task lands three ADRs in one commit per phase-02 precedent (Task 7 of phase-02 PLAN landed ADR-0024 + ADR-0025 in one commit).

---

## Task 8: `internal/filter/hcm/filter.go` + `internal/listener/manager.go` extension + tests

**Files:**
- Create: `internal/filter/hcm/filter.go`
- Create: `internal/filter/hcm/filter_test.go`
- Modify: `internal/listener/manager.go` (extend `filterRegistry` map at lines 41–51 with HCM type_url)
- Modify: `internal/listener/manager_test.go` (extend with HCM-registration tests)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 8 entry)

This task wires the HCM filter into the listener manager and adds the public-API contract surface (`NewFilter` + `Filter.Handle`). No new ADR (the registry-extension is implementation-time work; the filter's contract lives under ADR-0040 already). TDD: tests first.

- [ ] **Step 1: Write `internal/filter/hcm/filter_test.go`**

```go
package hcm

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestNewFilter_HappyPath(t *testing.T) {
	cm := mkClusterManager(t) // from config_test.go test helpers
	f, err := NewFilter(mkHCM(nil), cm)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	if f.statPrefix != "ingress_http" {
		t.Errorf("statPrefix: got %q, want %q", f.statPrefix, "ingress_http")
	}
	if len(f.table.routes) != 1 {
		t.Errorf("table.routes: got %d, want 1", len(f.table.routes))
	}
}

func TestNewFilter_PreservesParseErrorPrefix(t *testing.T) {
	cm := mkClusterManager(t)
	// Trigger one well-known error class (CodecType=HTTP2) and verify it
	// is surfaced through NewFilter unchanged (still hcm:-prefixed).
	any := mkHCM(func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP2 })
	if _, err := NewFilter(any, cm); err == nil || !strings.HasPrefix(err.Error(), "hcm:") {
		t.Errorf("expected hcm:-prefixed error, got: %v", err)
	}
}

func TestFilter_Handle_OneRequestThenEOF(t *testing.T) {
	cm := mkClusterManager(t)
	f, err := NewFilter(mkHCM(nil), cm)
	if err != nil {
		t.Fatal(err)
	}
	client, server := connPair(t)
	defer client.Close()
	go f.Handle(context.Background(), server)

	writeRequest(t, client, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Errorf("status: got %d, want 200", got)
	}
	// After Connection: close, the server side closes; client sees EOF on next read.
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Error("expected EOF/read-error after Connection: close, got bytes")
	}
}

func TestFilter_Handle_CtxAlreadyCancelledShortCircuits(t *testing.T) {
	cm := mkClusterManager(t)
	f, err := NewFilter(mkHCM(nil), cm)
	if err != nil {
		t.Fatal(err)
	}

	client, server := connPair(t)
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() { defer close(done); f.Handle(ctx, server) }()

	select {
	case <-done: // ok — Handle returned promptly
	case <-time.After(2 * time.Second):
		t.Error("Handle did not return promptly on cancelled ctx")
	}
}

// Ensure Filter satisfies the listener filterHandler interface shape (no error
// return). The compile-time check below would fail if Filter.Handle drifted.
var _ filterHandlerShape = (*Filter)(nil)

type filterHandlerShape interface {
	Handle(ctx context.Context, downstream net.Conn)
}
```

- [ ] **Step 2: Run test to verify it fails (compile error)**

Run: `cd internal/filter/hcm && go test -run 'TestNewFilter|TestFilter_Handle' .`
Expected: compile error — `Filter` has no field `statPrefix`/`table`/`Handle`.

- [ ] **Step 3: Write `internal/filter/hcm/filter.go`**

```go
package hcm

import (
	"context"
	"net"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

// Filter is the per-listener HTTP connection manager. It owns the resolved
// route table, the cluster manager handle, and the configured stat_prefix
// (forward-look for phase 06 stats per ADR-0041).
//
// Filter implements internal/listener.filterHandler:
//   Handle(ctx context.Context, downstream net.Conn)
//
// (No error return — matches the phase-02 tcpproxy.Filter.Handle precedent.)
type Filter struct {
	table      *routeTable
	clusters   *cluster.Manager
	statPrefix string
}

// NewFilter parses the typed_config Any into a *Filter. Errors begin with
// "hcm: "; the listener manager wraps them with "listener: %q: filter_chains[%d]: ".
func NewFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error) {
	return parseFilter(tc, clusters)
}

// Handle drives one downstream HTTP/1.1 connection from acceptance to close.
// On a cancelled ctx, Handle returns promptly without reading any bytes.
// All errors are owned by runConnection (logging + downstream close).
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	if err := ctx.Err(); err != nil {
		return
	}
	runConnection(ctx, downstream, f.table)
}
```

Note: `runConnection`'s defer-close in `connection.go` and `Filter.Handle`'s defer-close above both close the downstream — this is double-close. Either move the defer out of `runConnection` (preferred — Filter owns the conn) OR remove it from `Filter.Handle`. The plan picks the latter to keep `runConnection` self-contained for unit-testing without a Filter wrapper. Fix: change `Filter.Handle` to:

```go
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	if err := ctx.Err(); err != nil {
		_ = downstream.Close()
		return
	}
	runConnection(ctx, downstream, f.table) // owns the close
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/filter/hcm && go test -run 'TestNewFilter|TestFilter_Handle' -v .`
Expected: every subtest PASS.

- [ ] **Step 5: Extend `internal/listener/manager.go` `filterRegistry`**

The current map (lines 41–51) registers only `tcpproxy.TypeURL`. Add an HCM entry:

```go
// (existing block at lines 41–51)
var filterRegistry = map[string]filterConstructor{
	tcpproxy.TypeURL: func(tc *anypb.Any, cm *cluster.Manager) (filterHandler, error) {
		f, err := tcpproxy.NewFilter(tc, cm)
		if err != nil {
			return nil, err
		}
		return f, nil
	},
	hcm.TypeURL: func(tc *anypb.Any, cm *cluster.Manager) (filterHandler, error) {
		f, err := hcm.NewFilter(tc, cm)
		if err != nil {
			return nil, err
		}
		return f, nil
	},
}
```

Add the import: `"github.com/esalaine/envoy-go/internal/filter/hcm"` to `internal/listener/manager.go`'s import block.

- [ ] **Step 6: Extend `internal/listener/manager_test.go` with HCM-registration tests**

```go
// Add to the existing test file:

func TestNewManager_HCMRegistration(t *testing.T) {
	// Build a minimal bootstrap whose listener carries one HCM filter.
	// Reuse mkHCM from internal/filter/hcm if exposed via test helpers; or
	// construct the bootstrap proto inline.
	// Assert: NewManager succeeds.
}

func TestNewManager_HCMBuildErrorWrapsAsListenerFilter(t *testing.T) {
	// Bootstrap with an HCM whose codec_type is HTTP2.
	// Expected error message: "listener: \"l_http\": filter_chains[0]: hcm: codec_type HTTP2 is not supported in phase 04 (HTTP/1.1 only)"
	// Verify the wrap shape end-to-end.
}
```

- [ ] **Step 7: Run listener tests + race detector**

```bash
go test -race ./internal/listener/...
go test -race ./internal/filter/hcm/...
```
Expected: PASS, no data races.

- [ ] **Step 8: Run go vet + golangci-lint on touched packages**

```bash
go vet ./internal/listener/... ./internal/filter/hcm/...
golangci-lint run ./internal/listener/... ./internal/filter/hcm/...
```
Expected: both clean.

- [ ] **Step 9: Commit**

```bash
git add internal/filter/hcm/filter.go internal/filter/hcm/filter_test.go \
        internal/listener/manager.go internal/listener/manager_test.go
git commit -m "phase 04: internal/filter/hcm Filter + listener manager registers HCM type_url"
```

- [ ] **Step 10: Append PROGRESS.md entry for Task 8** (Task-3-shaped; SHA-fill follow-up commit).

---

## Task 9: `internal/bootstrap/bootstrap.go` blank imports + `internal/bootstrap/bootstrap_test.go`

**Files:**
- Modify: `internal/bootstrap/bootstrap.go` (three new blank imports)
- Modify: `internal/bootstrap/bootstrap_test.go` (HCM round-trip test)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 9 entry)

The blank-imports register the HCM, Router, and route-config proto descriptors with `protoregistry.GlobalTypes` so `protojson` can round-trip the typed_config Any without envoy-go interpreting its contents (ADR-0016). Per the existing comment block (lines 8–14 of `bootstrap.go`), the addition is documented in PROGRESS, not a new ADR. TDD: round-trip test first.

- [ ] **Step 1: Write the failing test in `internal/bootstrap/bootstrap_test.go`**

```go
// Add alongside the existing tests:
func TestLoad_HCMRoundTrip(t *testing.T) {
	// Minimal HCM bootstrap YAML — one listener, one filter chain, one HCM
	// network filter with one direct_response route. Verifies that protojson
	// round-trips the HCM typed_config (ADR-0016) without unknown-field errors.
	yamlSrc := `
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
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
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.GetStaticResources().GetListeners()); got != 1 {
		t.Fatalf("listeners: got %d, want 1", got)
	}
	// Sanity: re-serialize via protojson to confirm round-trippability without
	// discardUnknown — this is what would fail if any of the three new blank
	// imports were missing (the proto registry would not resolve the type_url).
	if _, err := protojson.Marshal(bs); err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/bootstrap && go test -run TestLoad_HCMRoundTrip -v .`
Expected: FAIL — `protojson` cannot resolve `…/HttpConnectionManager` type_url because the descriptor is not registered.

- [ ] **Step 3: Add the three blank imports to `internal/bootstrap/bootstrap.go`**

Append three lines under the existing tcp_proxy blank import block (after the `_ "...tcp_proxy/v3"` line near line 14):

```go
	// Phase 04 (HTTP/1.1) registers the HCM network filter, the router HTTP
	// filter, and the route-config proto so protojson round-trips fixtures
	// 0003-* and any future HTTP fixtures without interpreting typed_config.
	// Per ADR-0016 the addition is a registry-population mechanism, not a
	// new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/bootstrap && go test -run TestLoad_HCMRoundTrip -v .`
Expected: PASS.

- [ ] **Step 5: Run the full bootstrap test suite (regression check)**

Run: `go test ./internal/bootstrap/...`
Expected: every test PASS, including phase-01 `TestLoad_TcpProxyRoundTrip` and phase-01 fuzz seed corpus regression.

- [ ] **Step 6: Run go vet + golangci-lint**

```bash
go vet ./internal/bootstrap/...
golangci-lint run ./internal/bootstrap/...
```
Expected: both clean.

- [ ] **Step 7: Commit**

```bash
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go
git commit -m "phase 04: internal/bootstrap — register HCM + router + route-config protos for protojson round-trip"
```

- [ ] **Step 8: Append PROGRESS.md entry for Task 9** (Task-3-shaped; SHA-fill follow-up commit). The Notes line records that no new ADR is introduced; the addition is the per-ADR-0016 registry-population pattern.

---

## Task 10: `cmd/envoy-go/main_test.go` HCM bootstrap smoke variant

**Files:**
- Modify: `cmd/envoy-go/main_test.go` (add `TestEnvoyGoBinary_HCMSmoke`)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 10 entry)

End-to-end smoke test for the binary serving an HCM-routed response. Mirrors the existing `TestEnvoyGoBinary_TwoListenerCutover` shape (build the binary in `t.TempDir()`, write a bootstrap YAML, spawn the process with `--config-path`, wait for the per-listener ready sentinel, dial, assert response, signal shutdown). One direct_response route is enough — this task asserts the HCM is wired through `cmd/envoy-go/main.go` end-to-end without depending on fixture-0003 infrastructure (which lands in Task 15).

- [ ] **Step 1: Write the failing test**

Append to `cmd/envoy-go/main_test.go`:

```go
func TestEnvoyGoBinary_HCMSmoke(t *testing.T) {
	listenerPort := freeTCPPort(t)
	adminPort := freeTCPPort(t)

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cfgPath := filepath.Join(tmp, "envoy-go.yaml")
	cfg := fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
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
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`, adminPort, listenerPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "--config-path", cfgPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_, _ = cmd.Process.Wait()
	}()

	// Stream stdout/stderr for diagnostic logging.
	go io.Copy(io.Discard, stderr)

	// Wait for the per-listener ready sentinel.
	scanner := bufio.NewScanner(stdout)
	wantReady := regexp.MustCompile(`^envoy-go listener l_http ready on `)
	wantTerminal := regexp.MustCompile(`^envoy-go ready$`)
	sawListener := false
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && scanner.Scan() {
		line := scanner.Text()
		if wantReady.MatchString(line) {
			sawListener = true
		}
		if wantTerminal.MatchString(line) {
			break
		}
	}
	if !sawListener {
		t.Fatalf("did not see listener-ready sentinel within 15s")
	}

	// Issue an HTTP/1.1 GET /health and assert status 200 + body "OK\n".
	listenerAddr := fmt.Sprintf("127.0.0.1:%d", listenerPort)
	conn, err := net.DialTimeout("tcp", listenerAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("GET /health HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))
	body, _ := io.ReadAll(conn)
	got := string(body)
	if !strings.Contains(got, "HTTP/1.1 200 OK") {
		t.Errorf("status line missing from response:\n%s", got)
	}
	if !strings.Contains(got, "OK\n") {
		t.Errorf("expected body 'OK\\n' in response:\n%s", got)
	}
	if !strings.Contains(got, "Server: envoy") {
		t.Errorf("expected 'Server: envoy' header in response:\n%s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test -run TestEnvoyGoBinary_HCMSmoke -v ./cmd/envoy-go/`
Expected: PASS within 30s. The test reuses the existing `freeTCPPort` helper from `main_test.go`.

- [ ] **Step 3: Run the full cmd/envoy-go test suite (regression check)**

Run: `go test ./cmd/envoy-go/...`
Expected: every test PASS, including the phase-02 `TestEnvoyGoBinary_TwoListenerCutover`.

- [ ] **Step 4: Run go vet + golangci-lint**

```bash
go vet ./cmd/envoy-go/...
golangci-lint run ./cmd/envoy-go/...
```
Expected: both clean.

- [ ] **Step 5: Commit**

```bash
git add cmd/envoy-go/main_test.go
git commit -m "phase 04: cmd/envoy-go — HCM smoke variant (binary serves direct_response over HTTP/1.1)"
```

- [ ] **Step 6: Append PROGRESS.md entry for Task 10**.

---

## Task 11: `test/helpers/http.go` (`HTTPRoundTrip`) + `test/helpers/http_test.go`

**Files:**
- Create: `test/helpers/http.go`
- Create: `test/helpers/http_test.go`
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 11 entry)

The HTTP/1.1 round-trip helper consumed by fixture 0003's driver and by the runner's per-request orchestration pass (Task 13). Mirrors phase-03's `tls.go`/`tcp.go` style — a leaf helper with no dependency on `internal/`. TDD: tests first.

- [ ] **Step 1: Write the failing test in `test/helpers/http_test.go`**

```go
package helpers

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// startEcho starts an in-process HTTP/1.1 echo server that reads one request
// per accepted connection, writes one canned response, and closes. Returns the
// listener address and a cleanup function.
func startEcho(t *testing.T) (addr string, cleanup func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// read & discard the request
				buf := make([]byte, 4096)
				_, _ = c.Read(buf)
				_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\nContent-Type: text/plain\r\n\r\nhello"))
			}(c)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func TestHTTPRoundTrip_Happy(t *testing.T) {
	addr, cleanup := startEcho(t)
	defer cleanup()
	resp, body, err := HTTPRoundTrip(context.Background(), addr, "GET", "/x", nil, nil)
	if err != nil {
		t.Fatalf("HTTPRoundTrip: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if string(body) != "hello" {
		t.Errorf("body: got %q, want %q", string(body), "hello")
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain" {
		t.Errorf("Content-Type: got %q, want %q", got, "text/plain")
	}
}

func TestHTTPRoundTrip_CtxCancelledBeforeDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A non-routable address; dial would block, but cancelled ctx returns immediately.
	_, _, err := HTTPRoundTrip(ctx, "10.255.255.1:9", "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected ctx-cancelled error, got nil")
	}
}

func TestHTTPRoundTrip_ConnectionRefused(t *testing.T) {
	// Bind a port and immediately close to get a refused address.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()
	_, _, err := HTTPRoundTrip(context.Background(), addr, "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected connection-refused error, got nil")
	}
}

func TestHTTPRoundTrip_BodyClosedAfterReturn(t *testing.T) {
	addr, cleanup := startEcho(t)
	defer cleanup()
	resp, _, err := HTTPRoundTrip(context.Background(), addr, "GET", "/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// resp.Body should already be drained and replaced with an in-memory reader,
	// so a second read returns io.EOF without error.
	more, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ReadAll on returned body: %v", err)
	}
	if len(more) != 0 {
		t.Errorf("expected drained body, got %d bytes", len(more))
	}
}

func TestHTTPRoundTrip_SetHeaders(t *testing.T) {
	// A capture-the-request echo server: starts, reads the full request bytes,
	// then writes them back as the response body, prefixed by status/headers.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		defer c.Close()
		// Read until \r\n\r\n.
		buf := make([]byte, 4096)
		n, _ := c.Read(buf)
		req := string(buf[:n])
		body := req
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: " +
			itoa(len(body)) + "\r\n\r\n" + body))
	}()
	hdr := http.Header{"X-Test": []string{"yes"}}
	_, body, err := HTTPRoundTrip(context.Background(), ln.Addr().String(), "POST", "/p", hdr, []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "X-Test: yes") {
		t.Errorf("X-Test header missing from request: %s", got)
	}
	if !strings.Contains(got, "payload") {
		t.Errorf("payload missing from request: %s", got)
	}
	_ = time.Second
}

// small itoa to avoid importing strconv in the test helper for a single use.
func itoa(n int) string { return strings.TrimSuffix(strings.TrimPrefix(stdItoa(n), ""), "") }
func stdItoa(n int) string {
	if n == 0 { return "0" }
	digits := []byte{}
	for n > 0 { digits = append([]byte{byte('0'+n%10)}, digits...); n /= 10 }
	return string(digits)
}
```

(The `itoa`/`stdItoa` helpers above keep the test file dependency-light — the executor may instead `import "strconv"`. The shape is what matters; one of the two forms must compile.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/helpers && go test -run TestHTTPRoundTrip -v .`
Expected: compile error — `HTTPRoundTrip` undefined.

- [ ] **Step 3: Write `test/helpers/http.go`**

```go
package helpers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// HTTPRoundTrip dials addr, writes one HTTP/1.1 request, reads one response,
// drains the response body into memory, and returns the response (with Body
// replaced by an in-memory reader) plus the body bytes. The connection is
// closed on every exit path.
//
// Used by:
//   - test/fixtures/0003-http11-routing/driver to drive the differential gate.
//   - test/differential/runner_test.go's per-request HTTPExpectations
//     orchestration pass (ADR-0043).
//
// ctx governs the dial deadline (via net.Dialer.DialContext) and is propagated
// to the request via http.NewRequestWithContext.
func HTTPRoundTrip(ctx context.Context, addr, method, path string, headers http.Header, body []byte) (*http.Response, []byte, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+addr+path, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("new request: %w", err)
	}
	if headers != nil {
		for k, vv := range headers {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
	}
	// HTTP/1.1, default Connection: close per request to keep RR distribution
	// deterministic on the fixture-0003 subject side (per-request fresh dial,
	// ADR-0039). Callers that want keep-alive can override.
	if req.Header.Get("Connection") == "" {
		req.Header.Set("Connection", "close")
	}

	if err := req.Write(conn); err != nil {
		return nil, nil, fmt.Errorf("write request: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read body: %w", err)
	}
	resp.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	return resp, bodyBytes, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd test/helpers && go test -run TestHTTPRoundTrip -v .`
Expected: every TestHTTPRoundTrip_* PASS.

- [ ] **Step 5: Run go vet + golangci-lint**

```bash
go vet ./test/helpers/...
golangci-lint run ./test/helpers/...
```
Expected: both clean.

- [ ] **Step 6: Commit**

```bash
git add test/helpers/http.go test/helpers/http_test.go
git commit -m "phase 04: test/helpers — HTTPRoundTrip (HTTP/1.1 single round-trip helper)"
```

- [ ] **Step 7: Append PROGRESS.md entry for Task 11**.

---

## Task 12: `test/helpers/http_diff.go` (`HTTPHeaderDiff`) + `test/helpers/http_diff_test.go`

**Files:**
- Create: `test/helpers/http_diff.go`
- Create: `test/helpers/http_diff_test.go`
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 12 entry)

Header set-difference helper for the runner's per-request orchestration pass (Task 13) and for fixture 0003's `expectations.yaml` evaluation (Task 15). Settles SPEC §10 #7 to a fixed in-code allow-list (one fixture in phase 04). TDD: tests first.

- [ ] **Step 1: Write the failing test in `test/helpers/http_diff_test.go`**

```go
package helpers

import (
	"net/http"
	"reflect"
	"sort"
	"testing"
)

func TestHTTPHeaderDiff_Identical(t *testing.T) {
	a := http.Header{"X-A": {"1"}, "X-B": {"2"}}
	b := http.Header{"X-A": {"1"}, "X-B": {"2"}}
	refOnly, subjOnly := HTTPHeaderDiff(a, b, nil)
	if len(refOnly) != 0 || len(subjOnly) != 0 {
		t.Errorf("identical: got refOnly=%v subjOnly=%v, want both empty", refOnly, subjOnly)
	}
}

func TestHTTPHeaderDiff_RefOnlyAndSubjOnly(t *testing.T) {
	a := http.Header{"X-A": {"1"}, "X-Ref": {"r"}}
	b := http.Header{"X-A": {"1"}, "X-Subj": {"s"}}
	refOnly, subjOnly := HTTPHeaderDiff(a, b, nil)
	sort.Strings(refOnly); sort.Strings(subjOnly)
	if !reflect.DeepEqual(refOnly, []string{"x-ref"}) {
		t.Errorf("refOnly: got %v, want [x-ref]", refOnly)
	}
	if !reflect.DeepEqual(subjOnly, []string{"x-subj"}) {
		t.Errorf("subjOnly: got %v, want [x-subj]", subjOnly)
	}
}

func TestHTTPHeaderDiff_AllowListExact(t *testing.T) {
	a := http.Header{"Date": {"Tue, 01 Apr 2026 12:00:00 GMT"}, "X-A": {"1"}}
	b := http.Header{"Date": {"Tue, 01 Apr 2026 12:00:01 GMT"}, "X-A": {"1"}}
	refOnly, subjOnly := HTTPHeaderDiff(a, b, []string{"date"})
	if len(refOnly) != 0 || len(subjOnly) != 0 {
		t.Errorf("with date allow-listed: got refOnly=%v subjOnly=%v", refOnly, subjOnly)
	}
}

func TestHTTPHeaderDiff_AllowListPrefix(t *testing.T) {
	a := http.Header{"X-Envoy-Attempt-Count": {"1"}, "X-Envoy-Expected-Rq-Timeout-Ms": {"15000"}}
	b := http.Header{}
	refOnly, subjOnly := HTTPHeaderDiff(a, b, []string{"x-envoy-*"})
	if len(refOnly) != 0 || len(subjOnly) != 0 {
		t.Errorf("with x-envoy-* allow-listed: got refOnly=%v subjOnly=%v", refOnly, subjOnly)
	}
}

func TestHTTPHeaderDiff_CaseInsensitive(t *testing.T) {
	a := http.Header{"X-FOO": {"1"}}
	b := http.Header{"x-foo": {"1"}}
	refOnly, subjOnly := HTTPHeaderDiff(a, b, nil)
	if len(refOnly) != 0 || len(subjOnly) != 0 {
		t.Errorf("case-insensitive: got refOnly=%v subjOnly=%v", refOnly, subjOnly)
	}
}

func TestHTTPHeaderDiff_AllowListCaseInsensitive(t *testing.T) {
	a := http.Header{"DATE": {"x"}}
	b := http.Header{}
	refOnly, _ := HTTPHeaderDiff(a, b, []string{"DATE"})
	if len(refOnly) != 0 {
		t.Errorf("allow-list case-insensitive: got %v", refOnly)
	}
}

func TestPhaseFourHTTPAllowList_DefaultEntries(t *testing.T) {
	// Sanity: the default allow-list contains the seven entries codified in
	// ADR-0044 — date, server, content-length, transfer-encoding, x-envoy-*,
	// x-forwarded-*, x-request-id.
	want := map[string]bool{
		"date": true, "server": true, "content-length": true, "transfer-encoding": true,
		"x-envoy-*": true, "x-forwarded-*": true, "x-request-id": true,
	}
	got := map[string]bool{}
	for _, e := range PhaseFourHTTPAllowList {
		got[e] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PhaseFourHTTPAllowList:\n got: %v\n want: %v", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd test/helpers && go test -run 'TestHTTPHeaderDiff|TestPhaseFourHTTPAllowList' -v .`
Expected: compile error — `HTTPHeaderDiff` and `PhaseFourHTTPAllowList` undefined.

- [ ] **Step 3: Write `test/helpers/http_diff.go`**

```go
package helpers

import (
	"net/http"
	"strings"
)

// PhaseFourHTTPAllowList is the default allow-list for the phase-04 HTTP/1.1
// fixture (0003). Each entry is matched case-insensitively against the
// header name. An entry ending in '*' is a prefix-allow (e.g., "x-envoy-*"
// matches "x-envoy-attempt-count" and "x-envoy-expected-rq-timeout-ms").
// Codified in ADR-0044.
var PhaseFourHTTPAllowList = []string{
	"date",
	"server",
	"content-length",
	"transfer-encoding",
	"x-envoy-*",
	"x-forwarded-*",
	"x-request-id",
}

// HTTPHeaderDiff returns the symmetric difference of header names between
// refHeaders and subjHeaders, after lowercasing and applying allowList.
// refOnly contains lowercased names present in ref but not in subj (and not
// allow-listed). subjOnly is the mirror.
//
// allowList entries are matched case-insensitively; an entry ending in '*'
// is a prefix-allow.
func HTTPHeaderDiff(refHeaders, subjHeaders http.Header, allowList []string) (refOnly, subjOnly []string) {
	allowed := func(name string) bool {
		lname := strings.ToLower(name)
		for _, e := range allowList {
			le := strings.ToLower(e)
			if strings.HasSuffix(le, "*") {
				if strings.HasPrefix(lname, strings.TrimSuffix(le, "*")) {
					return true
				}
				continue
			}
			if lname == le {
				return true
			}
		}
		return false
	}
	subjSet := map[string]struct{}{}
	for k := range subjHeaders {
		subjSet[strings.ToLower(k)] = struct{}{}
	}
	refSet := map[string]struct{}{}
	for k := range refHeaders {
		refSet[strings.ToLower(k)] = struct{}{}
	}
	for k := range refSet {
		if _, ok := subjSet[k]; ok {
			continue
		}
		if allowed(k) {
			continue
		}
		refOnly = append(refOnly, k)
	}
	for k := range subjSet {
		if _, ok := refSet[k]; ok {
			continue
		}
		if allowed(k) {
			continue
		}
		subjOnly = append(subjOnly, k)
	}
	return refOnly, subjOnly
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd test/helpers && go test -run 'TestHTTPHeaderDiff|TestPhaseFourHTTPAllowList' -v .`
Expected: every test PASS.

- [ ] **Step 5: Run go vet + golangci-lint**

```bash
go vet ./test/helpers/...
golangci-lint run ./test/helpers/...
```
Expected: both clean.

- [ ] **Step 6: Commit**

```bash
git add test/helpers/http_diff.go test/helpers/http_diff_test.go
git commit -m "phase 04: test/helpers — HTTPHeaderDiff + PhaseFourHTTPAllowList"
```

- [ ] **Step 7: Append PROGRESS.md entry for Task 12**.

---

## Task 13: `test/differential/fixture/fixture.go` HTTPExpectations + runner orchestration pass + ADR-0043

**Files:**
- Modify: `test/differential/fixture/fixture.go` (add `HTTPRequestExpectation` struct + `HTTPExpectations` optional interface)
- Modify: `test/differential/runner_test.go` (per-request orchestration pass — type-asserted on the new optional interface)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0043)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 13 entry)

Lands ADR-0043 (the typed-extension decision) in the same commit as the first use of the interface (the runner's orchestration pass that consumes it). The `Driver` interface itself is unchanged — additivity preserves all phase-02/03 fixtures (0000, 0001, 0002).

- [ ] **Step 1: Sketch the test surface**

The orchestration pass in `runner_test.go` is exercised end-to-end by Task 15's fixture-0003 differential run; it does NOT need a unit test of its own (the assertion code is one branch of the existing per-fixture `TestDifferentialFixture` table). What this task does need is a regression check: after the change, fixtures 0000/0001/0002 (which do NOT implement `HTTPExpectations`) still pass.

There is no failing-test step at the head of this task because the change is additive-and-gated: the new branch only fires when the type assertion succeeds. A unit test of the type-assertion branch in isolation would fabricate a driver-shaped fake; the more honest test is Task 15's fixture 0003 actually running.

- [ ] **Step 2: Modify `test/differential/fixture/fixture.go`**

Append to the file (after the existing `DistributionAsserter` block):

```go
// HTTPRequestExpectation describes one HTTP/1.1 request the runner re-issues
// against ref and subject after Drive completes, to assert per-request status
// and body equivalence on top of the byte-stream comparison done by Drive.
//
// Phase 04 introduces this for fixture 0003-http11-routing. Phase 05 (HTTP/2)
// will reuse the shape — the path's protocol-version dimension is ignored
// here because phase 05's helpers will issue HTTP/2 round-trips via a
// different helper while populating the same struct.
//
// See ADR-0043.
type HTTPRequestExpectation struct {
	Method               string
	Path                 string
	ExpectStatus         int
	ExpectBodyEquivalent bool
}

// HTTPExpectations is an OPTIONAL fixture-driver interface. Drivers that
// implement it cause the runner to issue per-request HTTP round-trips against
// ref and subject after Drive completes, asserting status equivalence and
// (when ExpectBodyEquivalent is set) body byte-equivalence. Header set
// equality is checked via helpers.HTTPHeaderDiff under the phase-04 allow-list.
//
// Drivers that do NOT implement HTTPExpectations are unaffected (the runner's
// type assertion fails-silently and the new branch does not fire).
//
// See ADR-0043.
type HTTPExpectations interface {
	HTTPExpectations() []HTTPRequestExpectation
}
```

- [ ] **Step 3: Modify `test/differential/runner_test.go`**

After the existing `DistributionAsserter` branch (around line 145, after the `if da, ok := d.(fixture.DistributionAsserter); ok { … }` block), insert:

```go
	// 8b. Optional per-request HTTP round-trip + status/body/header
	// orchestration (phase 04, ADR-0043). Only fires when the driver
	// implements fixture.HTTPExpectations; phase-02/phase-03 fixtures
	// do not, so this is a no-op for 0000, 0001, 0002.
	if he, ok := d.(fixture.HTTPExpectations); ok {
		for i, exp := range he.HTTPExpectations() {
			refResp, refBody, err := helpers.HTTPRoundTrip(ctx, ref.ListenerAddr(d.SubjectListenerName()), exp.Method, exp.Path, nil, nil)
			if err != nil {
				t.Errorf("expectation[%d]: ref round-trip: %v", i, err)
				continue
			}
			subjResp, subjBody, err := helpers.HTTPRoundTrip(ctx, subj.ListenerAddr(d.SubjectListenerName()), exp.Method, exp.Path, nil, nil)
			if err != nil {
				t.Errorf("expectation[%d]: subj round-trip: %v", i, err)
				continue
			}
			if refResp.StatusCode != exp.ExpectStatus {
				t.Errorf("expectation[%d]: ref status: got %d, want %d", i, refResp.StatusCode, exp.ExpectStatus)
			}
			if subjResp.StatusCode != exp.ExpectStatus {
				t.Errorf("expectation[%d]: subj status: got %d, want %d", i, subjResp.StatusCode, exp.ExpectStatus)
			}
			if exp.ExpectBodyEquivalent && !bytes.Equal(refBody, subjBody) {
				t.Errorf("expectation[%d]: body mismatch:\n ref:  %q\n subj: %q", i, string(refBody), string(subjBody))
			}
			refOnly, subjOnly := helpers.HTTPHeaderDiff(refResp.Header, subjResp.Header, helpers.PhaseFourHTTPAllowList)
			if len(refOnly) > 0 || len(subjOnly) > 0 {
				t.Errorf("expectation[%d]: header diff outside allow-list:\n  ref-only: %v\n  subj-only: %v", i, refOnly, subjOnly)
			}
		}
	}
```

Also: ensure `bytes` is imported and `helpers` is imported at `test/differential/runner_test.go`'s top. The Reference container's listener address and the subject's listener address must both resolve to the **subject-side listener name** here — for fixture 0003 both proxies expose a listener named `l_http`; the `ref.ListenerAddr(...)` lookup uses the same key per ADR-0026's per-listener sentinel format. (If `ref.ListenerAddr` does not exist by that name on the ref side, this is a fixture-config bug — Task 15's first run will surface it.)

- [ ] **Step 4: Append ADR-0043 to `docs/envoy-go/DECISIONS.md`**

Per the summary in `## ADRs introduced by this plan` above (the ADR-0043 entry). Status: Accepted; Date: <session date>; Doctrine: D-3.4 / D-3.5 (both invoked: forward-look documentation, append-only).

- [ ] **Step 5: Run regression sweep — fixtures 0000/0001/0002 still green**

```bash
go test ./test/differential/... -timeout=8m
```
(Requires Docker — phase 02/03 differential gate equivalence.)
Expected: every existing fixture's `TestDifferentialFixture` row PASS; no failure introduced by the optional interface.

- [ ] **Step 6: Run go vet + golangci-lint**

```bash
go vet ./test/differential/...
golangci-lint run ./test/differential/...
```
Expected: both clean.

- [ ] **Step 7: Commit**

```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 04: test/differential — HTTPExpectations interface + runner orchestration pass [ADR-0043]"
```

- [ ] **Step 8: Append PROGRESS.md entry for Task 13**.

---

## Task 14: HTTP echo backend in `test/differential/runner_test.go` (`BackendKind`)

**Files:**
- Modify: `test/differential/fixture/fixture.go` (add `BackendKind` + `BackendKinds` optional interface)
- Modify: `test/differential/runner_test.go` (per-fixture backend-spawning code reads `BackendKind` and chooses TCP-echo vs HTTP-echo)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 14 entry)

Adds the HTTP/1.1 echo backend type alongside the existing TCP echo backend. Settles SPEC §10 #6 to the handcrafted-bufio variant (rationale in `## Settled SPEC §10 deferred decisions` entry #6). The optional `BackendKind` interface keeps existing TCP-echo fixtures (0000, 0001, 0002) untouched. No new ADR (implementation-detail; not cross-phase).

- [ ] **Step 1: Modify `test/differential/fixture/fixture.go`**

Append (after the `HTTPExpectations` block):

```go
// BackendKind discriminates between TCP-echo and HTTP-echo backends for the
// runner's per-fixture spawning code. Default (when a driver does NOT
// implement BackendKindAware) is TCPEcho — matches phase-02/03 fixtures.
type BackendKind int

const (
	TCPEcho  BackendKind = 0 // accept-loop reads-until-FIN, echoes bytes back; phase-02/03 default.
	HTTPEcho BackendKind = 1 // accept-loop reads one http.Request, writes "backend-<idx>:<lastSeg>" canned body, closes.
)

// BackendKindAware is an OPTIONAL driver-side method. Drivers that implement
// it select an alternative backend kind (e.g., HTTP-echo for phase-04 fixture
// 0003). Drivers that do NOT implement it default to TCPEcho.
type BackendKindAware interface {
	BackendKind() BackendKind
}
```

- [ ] **Step 2: Modify `test/differential/runner_test.go`'s backend-spawning code**

Around the per-fixture `for i := 0; i < n; i++ { … }` block that allocates backends (currently spawns TCP echo only), insert a kind switch:

```go
	kind := fixture.TCPEcho
	if bk, ok := d.(fixture.BackendKindAware); ok {
		kind = bk.BackendKind()
	}
	// existing TCP-echo accept loop (phase-02 shape) stays for kind == TCPEcho.
	// New branch for HTTPEcho:
	go func(b *backend) {
		for {
			c, err := b.ln.Accept()
			if err != nil {
				return
			}
			b.accepts.Add(1)
			go func(c net.Conn) {
				defer c.Close()
				switch kind {
				case fixture.TCPEcho:
					// phase-02 echo: io.Copy(c, c)
					_, _ = io.Copy(c, c)
				case fixture.HTTPEcho:
					// Read one request, write one canned response shaped
					// "backend-<idx>:<lastSegmentOfPath>", close.
					br := bufio.NewReader(c)
					req, err := http.ReadRequest(br)
					if err != nil {
						return
					}
					_, _ = io.Copy(io.Discard, req.Body)
					_ = req.Body.Close()
					seg := req.URL.Path
					if i := strings.LastIndex(seg, "/"); i >= 0 && i+1 < len(seg) {
						seg = seg[i+1:]
					}
					body := fmt.Sprintf("backend-%d:%s", b.idx, seg)
					fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
						len(body), body)
				}
			}(c)
		}
	}(b)
```

The `backend` struct gains an `idx int` field set to `i` at allocation time so the HTTP-echo branch can prefix `backend-<idx>:` (the prefix `AssertDistribution` counts on, settles SPEC §10 #14).

- [ ] **Step 3: Run regression sweep — TCPEcho fixtures still green**

```bash
go test ./test/differential/... -run TestDifferentialFixture -timeout=8m
```
Expected: PASS. Fixtures 0000, 0001, 0002 do not implement `BackendKindAware`, so the kind defaults to `TCPEcho` and the existing accept loop fires — no regression.

- [ ] **Step 4: Run go vet + golangci-lint**

```bash
go vet ./test/differential/...
golangci-lint run ./test/differential/...
```
Expected: both clean.

- [ ] **Step 5: Commit**

```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go
git commit -m "phase 04: test/differential — HTTP-echo backend kind alongside TCP-echo (handcrafted bufio per SPEC §10 #6)"
```

- [ ] **Step 6: Append PROGRESS.md entry for Task 14**.

---

## Task 15: `test/fixtures/0003-http11-routing/` — bootstraps + driver + driver_test + README + expectations

**Files:**
- Create: `test/fixtures/0003-http11-routing/envoy-go.yaml`
- Create: `test/fixtures/0003-http11-routing/envoy.yaml`
- Create: `test/fixtures/0003-http11-routing/expectations.yaml`
- Create: `test/fixtures/0003-http11-routing/README.md`
- Create: `test/fixtures/0003-http11-routing/driver/driver.go`
- Create: `test/fixtures/0003-http11-routing/driver/driver_test.go`
- Modify: `test/differential/runner_test.go` (add blank-import for the new driver)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 15 entry)

The differential fixture exercising HCM + route match + router + direct_response end-to-end. Three routes (`/health` → direct_response 200, `/api/*` → router → 3-endpoint cluster, `/` catch-all → direct_response 404), 27 requests per side (9 per route × 3 routes), `[3,3,3]` per-cluster RR distribution asserted. STATIC-vs-STRICT_DNS divergence per ADR-0027 (inherited).

This is the largest task in the plan. The executor should expect ~10 sub-step iterations and budget accordingly. If the body grows past 15 sub-steps once contact with reality reveals complexity, split per `BOOTSTRAP_PROMPT.md` §6.2 with a deviation ADR.

- [ ] **Step 1: Write `test/fixtures/0003-http11-routing/envoy.yaml`**

Reference (upstream Envoy) bootstrap. Key shape: 1 listener `l_http` on `0.0.0.0:15003` (in-container; the harness exposes it on a host port via testcontainers); 1 plaintext filter chain with empty `filter_chain_match`; 1 HCM filter; 3 routes; 1 STRICT_DNS cluster `c_backend` with `dns_lookup_family: V4_ONLY` (ADR-0010 inherited from fixtures 0001/0002) and three `lb_endpoints` at `host.docker.internal:<dyn0..2>`; admin at `0.0.0.0:9901`. Templated parameters: backend ports 0..2.

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 0.0.0.0, port_value: 15003 }
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
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                        - match: { prefix: "/api" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/" }
                          direct_response:
                            status: 404
                            body: { inline_string: "not found\n" }
                http_filters:
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
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
```

(The driver renders `%d` placeholders via `fmt.Sprintf`; the harness inherits the phase-02 `--concurrency 1` per ADR-0028.)

- [ ] **Step 2: Write `test/fixtures/0003-http11-routing/envoy-go.yaml`**

Subject (envoy-go) bootstrap. Same listener shape, same HCM config (verbatim modulo cluster.address), STATIC cluster (per ADR-0027) with 3 endpoints at `127.0.0.1:<dyn0..2>`. Templated parameters: subject listener port, subject admin port, backend ports 0..2.

```yaml
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
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
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                        - match: { prefix: "/api" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/" }
                          direct_response:
                            status: 404
                            body: { inline_string: "not found\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
```

- [ ] **Step 3: Write `test/fixtures/0003-http11-routing/expectations.yaml`** (prose form per ADR-0019)

```yaml
# Phase 04 fixture 0003-http11-routing — expectations
#
# This file is prose, not machine-evaluated (per ADR-0019). The runner
# enforces the assertions described below via test/differential/runner_test.go's
# byte-comparison + DistributionAsserter + HTTPExpectations passes.
#
# Asserted equivalence (per ADR-0044):
#   - response status code per request, across all 27 requests/side
#   - decoded response body bytes per routed-to-upstream request (the body the
#     fixture driver reads after http.ReadResponse over each of the 9 /api/*
#     requests)
#   - decoded response body bytes for /health direct_response (200, "OK\n")
#   - route-match selection witnessed by per-cluster RR distribution [3,3,3]
#     over the 9 router-action requests
#   - upstream-side request preservation (HTTP/1.1 method + path + Host)
#
# Not asserted:
#   - response-header set equality (only set-equality modulo allow-list)
#   - 404 local-reply body bytes (envoy-go: "not found\n"; Envoy: HTML/JSON
#     local reply — both produce 404 status, body relaxed)
#   - Content-Length vs Transfer-Encoding: chunked framing per response
#     (http.ReadResponse decodes both transparently)
#   - upstream connection re-use (envoy-go does not pool — ADR-0039; Envoy does)
#   - x-envoy-* / x-forwarded-* / x-request-id (envoy-go adds none; Envoy adds
#     many — all in the phase-04 allow-list)
#
# Header allow-list (set-equality modulo this list — values not compared):
#   date, server, content-length, transfer-encoding,
#   x-envoy-*, x-forwarded-*, x-request-id
#
# Route table (declaration order; first-match-wins):
#   1. match.path: "/health"   → direct_response 200 "OK\n"
#   2. match.prefix: "/api"    → router → cluster c_backend (3 endpoints, RR)
#   3. match.prefix: "/"       → direct_response 404 "not found\n" (catch-all)
#
# Driver request schedule (27 requests/side, deterministic ordering):
#   9 × GET /health                  → expect 200, body "OK\n"
#   9 × GET /api/v1/<n>  n=0..8      → expect 200, body "backend-<idx>:v1/<n>"
#                                       (idx is whichever backend served; count-asserted
#                                        not request-asserted — RR distribution [3,3,3])
#   9 × GET /missing/<n> n=0..8      → expect 404 (body relaxed)
#
# Container concurrency:
#   --concurrency 1 inherited from phase-02 ADR-0028 via test/differential/harness.go
#   (verified at PLAN Task 1 step 1 — the unconditional flag).
#
# Backend body format:
#   "backend-<idx>:<last-segment-of-path>" per response (ADR-0044, settled
#   SPEC §10 #14). The "backend-<idx>:" prefix is what AssertDistribution
#   counts on; the path-segment suffix makes responses distinguishable per
#   request so DriveSubject's concatenated byte stream is non-degenerate.
```

- [ ] **Step 4: Write `test/fixtures/0003-http11-routing/README.md`**

Mirrors phase-02/03 fixture README structure. Sections: Purpose, Topology, Routes, Driver request schedule, STATIC-vs-STRICT_DNS divergence (ADR-0027 inherited), `--concurrency 1` reference pin (ADR-0028 inherited), Per-request fresh dial (ADR-0039), Explicit catch-all 404 (SPEC §10 #5 settled), Header allow-list extensions (ADR-0044), Header values relaxed.

- [ ] **Step 5: Write `test/fixtures/0003-http11-routing/driver/driver.go`**

```go
// Package driver registers the 0003-http11-routing fixture with the
// differential runner. See ../README.md for the fixture's purpose; ADR-0027
// for the STATIC-vs-STRICT_DNS divergence; ADR-0044 for the BEHAVIOR_CONTRACT.
package driver

import (
	"context"
	"fmt"
	"strings"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const fixtureName = "0003-http11-routing"

func init() {
	fixture.RegisterFixture(fixtureName, &httpDriver{})
}

type httpDriver struct{}

func (httpDriver) BackendCount() int            { return 3 }
func (httpDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (httpDriver) SubjectListenerName() string  { return "l_http" }
func (httpDriver) ReferenceListenerPort() int   { return 15003 }

func (httpDriver) ReferenceBootstrap(backendPorts []int) string {
	return fmt.Sprintf(referenceTmpl, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (httpDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return fmt.Sprintf(subjectTmpl, subjAdminPort, subjListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

// 27 requests per side, deterministic ordering: 9 × /health, 9 × /api/v1/<n>,
// 9 × /missing/<n>. Per-request fresh dial (ADR-0039) — Connection: close
// is set inside HTTPRoundTrip's default header path.
func (httpDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}
func (httpDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

func drive(ctx context.Context, addr string) ([]byte, error) {
	var out strings.Builder
	for n := 0; n < 9; n++ {
		_, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/health", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/health[%d]: %w", n, err)
		}
		out.Write(body)
	}
	for n := 0; n < 9; n++ {
		_, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", fmt.Sprintf("/api/v1/%d", n), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/api/v1/%d: %w", n, err)
		}
		out.Write(body)
	}
	for n := 0; n < 9; n++ {
		// 404 body is relaxed across ref/subj — discard for the byte stream
		// but issue the request to drive cluster-RR distribution and to
		// observe status equivalence (asserted via HTTPExpectations).
		_, _, err := helpers.HTTPRoundTrip(ctx, addr, "GET", fmt.Sprintf("/missing/%d", n), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/missing/%d: %w", n, err)
		}
		// intentionally do not append the 404 body to the byte stream:
		// envoy-go and Envoy local-reply bodies differ (ADR-0044), so the
		// byte-equivalence assertion would fail. Status is checked via
		// HTTPExpectations.
	}
	return []byte(out.String()), nil
}

func (httpDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	// 27 requests/side; 9 of them are router-action (the /api/v1/<n> set).
	// Each side's per-backend count must be exactly [3,3,3] to witness RR.
	want := []uint64{3, 3, 3}
	if len(subjCounts) != 3 {
		return fmt.Errorf("subj backend count: got %d, want 3", len(subjCounts))
	}
	for i, c := range subjCounts {
		if c != want[i] {
			return fmt.Errorf("subj backend %d: got %d, want %d (RR [3,3,3] expected)", i, c, want[i])
		}
	}
	// Reference counts are not asserted to be [3,3,3] verbatim because
	// host.docker.internal DNS may rotate endpoints differently than STATIC.
	// (ADR-0027 documents the STATIC-vs-STRICT_DNS divergence.) The runner
	// uses subj-only RR as the gate.
	_ = refCounts
	return nil
}

func (httpDriver) HTTPExpectations() []fixture.HTTPRequestExpectation {
	exp := make([]fixture.HTTPRequestExpectation, 0, 27)
	for n := 0; n < 9; n++ {
		exp = append(exp, fixture.HTTPRequestExpectation{Method: "GET", Path: "/health", ExpectStatus: 200, ExpectBodyEquivalent: true})
	}
	for n := 0; n < 9; n++ {
		exp = append(exp, fixture.HTTPRequestExpectation{Method: "GET", Path: fmt.Sprintf("/api/v1/%d", n), ExpectStatus: 200, ExpectBodyEquivalent: true})
	}
	for n := 0; n < 9; n++ {
		exp = append(exp, fixture.HTTPRequestExpectation{Method: "GET", Path: fmt.Sprintf("/missing/%d", n), ExpectStatus: 404, ExpectBodyEquivalent: false})
	}
	return exp
}

func (httpDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	// /ready against both admin endpoints (phase-01 pattern, inherited).
	_, refBytes, err = helpers.HTTPRoundTrip(ctx, refAdminAddr, "GET", "/ready", nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	_, subjBytes, err = helpers.HTTPRoundTrip(ctx, subjAdminAddr, "GET", "/ready", nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// Templates — Sprintf'd in ReferenceBootstrap / SubjectConfig.
const referenceTmpl = `<contents of envoy.yaml above, with backend ports as %d format slots>`
const subjectTmpl = `<contents of envoy-go.yaml above, with admin/listener/backend ports as %d format slots>`
```

(The actual template strings are pasted from Steps 1 and 2; the executor copies the YAML body verbatim into Go raw-string-literals.)

- [ ] **Step 6: Write `test/fixtures/0003-http11-routing/driver/driver_test.go`**

Mirror of fixture-0001/0002 driver_test.go discipline — Docker-free; tests `AssertDistribution` only.

```go
package driver

import "testing"

func TestAssertDistribution_Happy(t *testing.T) {
	d := httpDriver{}
	if err := d.AssertDistribution([]uint64{3, 3, 3}, []uint64{3, 3, 3}); err != nil {
		t.Errorf("happy: %v", err)
	}
}

func TestAssertDistribution_SkewFails(t *testing.T) {
	d := httpDriver{}
	if err := d.AssertDistribution([]uint64{3, 3, 3}, []uint64{4, 3, 2}); err == nil {
		t.Error("expected error on [4,3,2], got nil")
	}
}

func TestAssertDistribution_WrongLengthFails(t *testing.T) {
	d := httpDriver{}
	if err := d.AssertDistribution([]uint64{3, 3, 3}, []uint64{3, 3, 3, 0, 0, 0}); err == nil {
		t.Error("expected error on length mismatch, got nil")
	}
}
```

- [ ] **Step 7: Add the blank-import to `test/differential/runner_test.go`**

Append to the import block:
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver"
```

- [ ] **Step 8: Run the driver's unit tests (Docker-free)**

```bash
go test ./test/fixtures/0003-http11-routing/...
```
Expected: `TestAssertDistribution_*` PASS.

- [ ] **Step 9: Run the differential gate against fixture 0003**

```bash
go test ./test/differential/... -run 'TestDifferentialFixture/0003-http11-routing' -v -timeout=10m
```
(Requires Docker.)
Expected: PASS — including byte-comparison of the concatenated `/health` + `/api/*` body stream, the [3,3,3] RR distribution assertion, the per-request HTTPExpectations status + body + header-allow-list pass, and the admin /ready differential.

If the run fails, the executor invokes `superpowers:systematic-debugging` on the specific assertion that failed (the runner output names the failing pass — byte-comparison vs distribution vs HTTPExpectations vs admin probe).

- [ ] **Step 10: Run go vet + golangci-lint**

```bash
go vet ./test/fixtures/0003-http11-routing/... ./test/differential/...
golangci-lint run ./test/fixtures/0003-http11-routing/... ./test/differential/...
```
Expected: both clean.

- [ ] **Step 11: Commit**

```bash
git add test/fixtures/0003-http11-routing/ test/differential/runner_test.go
git commit -m "phase 04: fixture 0003-http11-routing — HCM + route match + router + direct_response"
```

- [ ] **Step 12: Append PROGRESS.md entry for Task 15**.

---

## Task 16: `internal/filter/hcm/fuzz_test.go` — `FuzzHCMConfigParse`

**Files:**
- Create: `internal/filter/hcm/fuzz_test.go`
- Create: `internal/filter/hcm/testdata/fuzz/FuzzHCMConfigParse/` (seed corpus, three entries)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 16 entry)

Short-budget fuzz target for `parseFilter` per ADR-0018 (CI 30s budget, precedent inherited from phase-01/02/03). No new ADR.

- [ ] **Step 1: Write `internal/filter/hcm/fuzz_test.go`**

```go
package hcm

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"

	"github.com/esalaine/envoy-go/internal/cluster"
)

// FuzzHCMConfigParse exercises NewFilter against arbitrary Any byte streams.
// Asserts: no panic; every error message is hcm:-prefixed.
//
// Per ADR-0018: short-budget (30s in CI; arbitrary local time). Seed corpus
// gives the fuzzer three starting points: one well-formed Any, one truncated
// byte stream, one wrong-type-url Any.
func FuzzHCMConfigParse(f *testing.F) {
	// Seed corpus.
	wellFormed, _ := anypb.New(mkRawHCMForFuzz())
	f.Add(wellFormed.GetTypeUrl(), wellFormed.GetValue())            // (a) well-formed
	f.Add("type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager", []byte{}) // (b) truncated
	f.Add("type.googleapis.com/google.protobuf.StringValue", []byte("hello")) // (c) wrong type_url

	// One-cluster manager — wrap a STATIC c_test in a real cluster.Manager
	// so the router-action build path is reachable.
	cm := mkOneClusterManager(f)

	f.Fuzz(func(t *testing.T, typeURL string, value []byte) {
		any := &anypb.Any{TypeUrl: typeURL, Value: value}
		_, err := NewFilter(any, cm)
		if err != nil && !strings.HasPrefix(err.Error(), "hcm:") {
			t.Errorf("error not hcm:-prefixed: %v", err)
		}
		// Panic protection is automatic via testing.F's recover wrapper.
	})
}

// mkOneClusterManager builds a tiny cluster.Manager with one STATIC cluster
// "c_test" at 127.0.0.1:1. Used only by the fuzz target.
func mkOneClusterManager(t testing.TB) *cluster.Manager {
	t.Helper()
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name: "c_test", Type: clusterv3.Cluster_STATIC,
				LbPolicy: clusterv3.Cluster_ROUND_ROBIN,
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: "c_test",
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{SocketAddress: &corev3.SocketAddress{
									Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 1},
								}}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs)
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

// mkRawHCMForFuzz returns a minimal *anypb.Any-wrapped HCM body for the (a)
// well-formed seed corpus. Reuses mkHCM from config_test.go via the test
// package boundary.
func mkRawHCMForFuzz() *anypb.Any { return mkHCM(nil) }
```

- [ ] **Step 2: Run a short fuzz session locally to confirm no immediate crashers**

```bash
cd internal/filter/hcm && go test -run=^$ -fuzz=FuzzHCMConfigParse -fuzztime=30s .
```
Expected: completes in ~30s with no crashers added to `testdata/fuzz/FuzzHCMConfigParse/`. If a crasher appears, invoke `superpowers:systematic-debugging` on the input.

- [ ] **Step 3: Run go vet + golangci-lint**

```bash
go vet ./internal/filter/hcm/...
golangci-lint run ./internal/filter/hcm/...
```
Expected: both clean.

- [ ] **Step 4: Commit**

```bash
git add internal/filter/hcm/fuzz_test.go
# include testdata/fuzz/ ONLY if a crasher was discovered and triaged in step 2
git commit -m "phase 04: internal/filter/hcm — FuzzHCMConfigParse short-budget fuzz target"
```

- [ ] **Step 5: Append PROGRESS.md entry for Task 16**.

---

## Task 17: BEHAVIOR_CONTRACT update + ADR-0044 + all-gates green final sweep

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (add `## HTTP/1.1` section after `## TLS`; extend `## Header allow-list` table)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0044)
- Modify: `docs/envoy-go/phases/04-http-1.1/PROGRESS.md` (append Task 17 entry)

The phase-04 closing task: codify the new equivalence surface in BEHAVIOR_CONTRACT, land ADR-0044 in the same commit, then run the all-gates green local sweep mirroring phase-03's Task 15 shape (gates a/b/d/e per SPEC §3; gate c N/A — this phase introduces no new sentinel format; gate f deferred per BOOTSTRAP §5 step 6).

- [ ] **Step 1: Extend the `## Header allow-list` table in `docs/envoy-go/BEHAVIOR_CONTRACT.md`**

Add five rows after the existing rows (the existing block is at lines 27–34):

```
| Server | HCM-locally-generated responses; presence-only | Phase 04; ADR-0044 |
| Content-Length | HTTP/1.1 framing-divergence-permitted | Phase 04; ADR-0044 |
| Transfer-Encoding | HTTP/1.1 framing-divergence-permitted | Phase 04; ADR-0044 |
| x-envoy-* | every header with this prefix; presence-not-required on subject | Phase 04; ADR-0044 |
| x-forwarded-* | every header with this prefix; presence-not-required on subject | Phase 04; ADR-0044 |
| x-request-id | presence-not-required on subject | Phase 04; ADR-0044 |
```

- [ ] **Step 2: Add the new `## HTTP/1.1` section after `## TLS`**

Append (mirroring the `## TLS` shape — `### Asserted equivalence`, `### Not asserted`, `### Header allow-list extensions`, `### Applies to`, `### Does not yet apply to`):

```markdown
## HTTP/1.1

Phase 04 introduces HTTP/1.1 routing — the HCM network filter parses request
lines off the downstream connection, matches against an inline route table
(`match.prefix` bytewise OR `match.path` case-sensitive exact), dispatches
through a `direct_response` (HCM-locally-generated reply) or a `route` (router
action over a per-request fresh upstream dial). See ADR-0044.

### Asserted equivalence

- Response status code per request.
- Decoded response body bytes per routed-to-upstream request (the body the
  fixture driver reads after `http.ReadResponse`).
- Decoded response body bytes for `direct_response` paths whose status is in
  the 200–299 family (4xx and 5xx local replies have body relaxed — see Not
  asserted).
- Route-match selection: same method + path → same matched route on both
  proxies, witnessed by per-cluster RR distribution `[3,3,3]` over the
  router-action subset.
- Upstream-side request preservation: verbatim Host, method, path-with-query,
  body — except where stdlib HTTP/1.1 parsing on the subject side introduces
  a bounded, documented normalisation (per ADR-0037).

### Not asserted

- Response-header **value** equality (only set-equality modulo allow-list).
- Local-reply body bytes for 4xx/5xx (Envoy and envoy-go differ in their
  default local-reply body content; status is asserted, body is relaxed).
- `Content-Length` vs `Transfer-Encoding: chunked` framing per response (the
  harness decodes both via `http.ReadResponse`).
- Upstream connection re-use (envoy-go does not pool — ADR-0039; Envoy does).
- `x-envoy-*` / `x-forwarded-*` / `x-request-id` headers (envoy-go injects
  none; Envoy injects many — all in the allow-list).

### Header allow-list extensions

See the `## Header allow-list` table above, rows added by ADR-0044:
`Server`, `Content-Length`, `Transfer-Encoding`, `x-envoy-*`, `x-forwarded-*`,
`x-request-id`.

### Applies to

- Phase-04 envoy-go `internal/filter/hcm/` package, exercised via fixture
  `0003-http11-routing`.
- The phase-04 HCM-filter chain shape `[router]` (ADR-0042).
- `match.prefix` (bytewise) and `match.path` (case-sensitive exact) only.

### Does not yet apply to

- HTTP/2 (phase 05).
- HTTP/3 (later).
- HCM filter chain beyond `[router]` (phase 07's filter-chain framework).
- Upstream connection pooling (upstream-robustness family).
- HTTPS (phase 04.x or 05.x or a dedicated HTTPS-fixture sub-phase).
- `match.regex` / `match.path_separated_prefix` / `match.connect_matcher` /
  header-aware match / query-parameter-aware match (subset enforcement —
  ADR-0038).
- HTTP-filter iteration protocol (decode-headers, decode-data,
  encode-headers, etc. — phase 07).
```

- [ ] **Step 3: Append ADR-0044 to `docs/envoy-go/DECISIONS.md`**

Per the summary in `## ADRs introduced by this plan` above (the ADR-0044 entry). Status: Accepted; Date: <session date>; Doctrine: D-3.4 (forward-look) + D-3.5 (append-only).

- [ ] **Step 4: Commit the BEHAVIOR_CONTRACT + ADR-0044 update**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md
git commit -m "phase 04: BEHAVIOR_CONTRACT HTTP/1.1 subsection + Header allow-list extension [ADR-0044]"
```

- [ ] **Step 5: All-gates green local sweep — gate (a): differential fixtures**

```bash
go test ./test/differential/... -timeout=12m
```
Expected: every fixture (0000, 0001, 0002, 0003) PASS. Quote last 30 lines of output verbatim into the PROGRESS entry.

- [ ] **Step 6: All-gates green local sweep — gate (b): every package's unit tests**

```bash
go test -race ./...
```
Expected: every package PASS, no data races. Quote last 30 lines of output verbatim into the PROGRESS entry.

- [ ] **Step 7: All-gates green local sweep — gate (d): fuzz targets short budget (30s each)**

```bash
go test ./internal/bootstrap -run=FuzzBootstrapLoad -fuzz=FuzzBootstrapLoad -fuzztime=30s
go test ./internal/filter/tcpproxy -run=FuzzTcpProxyFilter -fuzz=FuzzTcpProxyFilter -fuzztime=30s
go test ./internal/tls -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s
go test ./internal/filter/hcm -run=FuzzHCMConfigParse -fuzz=FuzzHCMConfigParse -fuzztime=30s
```
Expected for each: completes in ~30s with no crashers added to `testdata/fuzz/`. Quote each summary line into PROGRESS.

- [ ] **Step 8: All-gates green local sweep — gate (e): vet + golangci-lint**

```bash
go vet ./...
golangci-lint run ./...
```
Expected: both clean across the whole module.

- [ ] **Step 9: Gate (c) explicit N/A note**

Phase 04 does not introduce any new sentinel format (no per-listener-name additions; no admin protocol additions; the cmd/envoy-go/main_test.go HCM smoke variant in Task 10 reads the existing `envoy-go listener <name> ready on <addr>` sentinel — already in the BEHAVIOR_CONTRACT). Record as `c: N/A — phase 04 introduces no new ready-sentinel format`.

- [ ] **Step 10: Gate (f) deferral note**

Per `BOOTSTRAP_PROMPT.md` §5 step 6, gate (f) (the cross-phase regression sweep) is owned by the verification-and-review session that follows the executor's commit. Record as `f: deferred to verification-before-completion session per BOOTSTRAP §5 step 6`.

- [ ] **Step 11: Append a Task 17 PROGRESS entry with every command output verbatim**

This PROGRESS entry is the session's "verification proof" — it is the content `superpowers:verification-before-completion` will read when phase 04 moves to lifecycle-state 4. Keep every last-30-lines-of-output block verbatim (mirror phase-03's Task 15 PROGRESS shape — commits `d9f29a9`, `e3a4f20`, `8d262cb`).

- [ ] **Step 12: Commit**

```bash
git add docs/envoy-go/phases/04-http-1.1/PROGRESS.md
git commit -m "phase 04: Task 17 — all-gates green local sweep (a/b/d/e; c N/A; f deferred)"
```

- [ ] **Step 13: Confirm phase-04 readiness for state-4 transition (do NOT advance STATE — that's a later session per ADR-0005)**

This plan-authored phase ends with Task 17 committed on `phase/04-http-1.1-impl`. Task 17's sweep pre-flights what the subsequent verification/review sessions will confirm. STATE advancement through 4 → 5 → 6 is per-session work, not this plan's responsibility.

---

## Post-plan handoff: state advancement + worktree cleanup (session-exit duties)

This section is the plan-authoring session's exit contract, not an executable task.

After the executing session commits Task 17 on `phase/04-http-1.1-impl`:

1. **Fast-forward merge to master.** Per ADR-0003:
   ```bash
   cd /home/esa/git/envoy-go   # master worktree
   git merge --ff-only phase/04-http-1.1-impl
   ```
2. **Advance `docs/envoy-go/STATE.md` on master** to `lifecycle-state: 4` + `next-skill: superpowers:verification-before-completion`, reflecting that the next fresh session runs verification before REVIEW. Commit with `phase 04: STATE.md → lifecycle-state 4`.
3. **The verification session** (next-next from the current plan-authoring session) then advances STATE through 5 and 6 per the state machine. Phase-04 ROADMAP row 04 advances to `done` at state 6. Phase 05's STATE handoff (`active-phase: 05-http-2`, `lifecycle-state: 1`, `next-skill: superpowers:brainstorming`) lands with the final phase-04 commit.

No part of this section is done by Task 17. It lives here so the plan-authoring session (i.e., the current one) knows where to leave STATE after its own commit, and so the executing session has clear context for its exit.

---

## Plan review loop (invoked at end of plan-authoring session)

Per `superpowers:writing-plans` and ADR-0005: after this PLAN.md is written, dispatch the `plan-document-reviewer` subagent with the PLAN.md path + the SPEC.md path. If the reviewer returns approved → commit PLAN.md + STATE advancement (state 2 → state 3 on master). If the reviewer returns changes-requested → address feedback in place, re-dispatch (max 3 iterations per ADR-0005 + skill guidance); on iteration 3 without approval, exit blocked per `BOOTSTRAP_PROMPT.md` §5 deviations.

The reviewer's scope:
- Does the PLAN cover every SPEC §4 deliverable? (7 items — `internal/filter/hcm/`, listener-manager extension, bootstrap blank imports, helper additions, fixture 0003, BEHAVIOR_CONTRACT subsection, eight ADRs.)
- Does the PLAN settle every SPEC §10 deferred decision? (14 items — see `## Settled SPEC §10 deferred decisions`.)
- Does the PLAN mitigate every SPEC §11 risk with a task-level step or an ADR?
- Does the PLAN resolve phase-03 REVIEW Minors triaged in SPEC §12? (8 items, all DEFERRED — see `## Phase-03 REVIEW carryover resolution matrix`.)
- Are tasks atomic (one logical commit each, 2–5 minutes per step except the well-annotated longer ones — Task 7, Task 15)?
- Does the ADR number sequence match verified DECISIONS.md tail? (ADR-0036 → ADR-0037..0044 — re-verified at Task 1 step 1.)
- Is the LoC estimate honest and does the scope-check argument hold?
- Are the four spec-review advisory items (i–iv) addressed? (See `## Spec-review advisory responses`.)
