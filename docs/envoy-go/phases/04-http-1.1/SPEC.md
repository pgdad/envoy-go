# Phase 04 — HTTP/1.1 (HCM + route match + router + direct_response)

**Phase id:** `04`
**Slug:** `04-http-1.1`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:brainstorming` (adapted autonomous mode — see `DECISIONS.md` ADR-0004)
**Depends on:** phase 03 (done)
**Differential surface at end of phase:** pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, and `0002-tls-tcp` remain green with no behavioural regression; new fixture `0003-http11-routing` green, exercising HTTP/1.1 request parsing, route match (`prefix` + `path`), the router HTTP filter dispatching to a STATIC cluster of HTTP/1.1 echo backends, and a `direct_response` route action — byte-equivalent decoded response body and set-equal response headers (modulo the existing `date`/`server` allow-list and the new HTTP/1.1 framing entries this phase adds) against upstream Envoy v1.37.2 on every request issued by the fixture driver.

---

## 1. Purpose

Phase 04 lands envoy-go's first HTTP-aware dataplane: the listener filter chain now dispatches an `HttpConnectionManager` (HCM) network filter that owns one downstream HTTP/1.1 connection at a time, parses each request, runs a degenerate HTTP-filter chain whose only entry is the `router` filter, matches the request against a single virtual host's route table, and either short-circuits to a `direct_response` body or proxies to an upstream HTTP/1.1 cluster via `Cluster.Dial(ctx)` (the phase-03 dialer). The phase ships the absolute minimum of the HCM proto surface that produces behaviourally-equivalent responses to upstream Envoy on the same configuration and request inputs.

Concretely, phase 04 produces:

1. An HTTP connection manager package under `internal/filter/hcm/` that parses the Envoy v3 proto `envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager` from a network filter's `typed_config` Any, resolves an inline route_config's single virtual_host's routes to a phase-04-internal route table, validates the HTTP-filters list is exactly one entry naming `envoy.filters.http.router` (`envoy.extensions.filters.http.router.v3.Router`), and registers a `Filter.Handle(ctx, downstream)` per the existing network-filter contract.
2. An HTTP/1.1 wire codec wrapping `net/http.ReadRequest` (request parse) and `net/http.Request.Write`, `net/http.ReadResponse`, `net/http.Response.Write` (request relay + response readback + response write) under `internal/filter/hcm/codec.go`. Stdlib parsing is doctrine-compatible (`D-3.2` forbids `net/http/httputil.ReverseProxy` and embedding 3rd-party server cores; stdlib parsers/serializers are permitted-foundation library use). The HCM does NOT use `net/http.Server` — there is no `http.Handler` glue and no `ServeHTTP`. The connection loop is driven explicitly under `Filter.Handle`.
3. A connection loop under `internal/filter/hcm/connection.go` that reads HTTP/1.1 requests one at a time off a `bufio.Reader` over the downstream conn, dispatches each request through the route table, performs the chosen action (router or direct_response), writes the response back through a `bufio.Writer`, and continues the loop iff persistent-connection semantics permit (HTTP/1.1 default keep-alive; `Connection: close` on either side terminates).
4. A route match engine under `internal/filter/hcm/route.go` implementing exactly two route match predicates: `match.prefix` and `match.path`. The first matching route wins (Envoy semantics). One virtual_host per route_config, with `domains: ["*"]` (catch-all) — multi-vhost matching is deferred to phase 07.
5. Two route actions implemented under `internal/filter/hcm/actions.go`: `route.direct_response` (synthesizes a status + inline body without dialing upstream) and `route.route` (the router action — dials the named cluster via `Cluster.Dial(ctx)`, writes the request through, reads the response back, and streams the response through to the downstream writer). Other actions (`redirect`, `cluster_specifier_plugin`, route-level `host_rewrite_*`, etc.) error at build time.
6. An inline filter-registry expansion in `internal/listener/manager.go`: the build-time filter-type-URL switch now also recognises `type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager` → `hcm.NewFilter`. The phase-02 `envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy` → `tcpproxy.NewFilter` mapping is unchanged.
7. A new differential fixture `test/fixtures/0003-http11-routing/`: one plaintext HTTP/1.1 listener with one HCM filter; one route_config with one `*` vhost holding three routes (`path: /health` → direct_response 200 `OK\n`; `prefix: /api` → cluster `c_backend`; otherwise no match → 404 local reply); one STATIC cluster (`c_backend`) of three HTTP/1.1 echo backends; driver issues N requests and asserts byte-equivalent decoded response bodies + status codes per request, plus per-cluster RR distribution `[k, k, k]` over the routed-to-upstream subset. Counts: see §5.10.
8. A new `BEHAVIOR_CONTRACT.md` subsection, **HTTP/1.1**, codifying the equivalence surface for HTTP/1.1 routing: response-status equivalence, decoded-body equivalence, header set-equivalence modulo the (extended) allow-list, route-match selection equivalence, framing-divergence rule (Content-Length vs Transfer-Encoding chunked is not asserted; the harness decodes both sides before comparing — this rule is inherited from the phase-01 admin `/ready` precedent and now codified for the general HTTP surface).
9. A small fuzz target `internal/filter/hcm.FuzzHCMConfigParse` covering the HCM typed_config Any → `Filter` build-time path. Seeds: one well-formed HCM Any with a router-only filter list and one direct_response + one router route; one malformed (truncated) Any; one Any with the wrong type_url. Short-budget `-fuzztime=30s` per ADR-0018. The HTTP/1.1 wire parsing surface is `net/http.ReadRequest` (stdlib, independently fuzzed) so phase 04 does not introduce a wire-parser fuzz target.

After phase 04, the project has proven its fifth central engineering claim: *envoy-go speaks HTTP/1.1 — it parses requests, matches them against a configured route table, and either synthesizes a local reply or proxies to an upstream HTTP/1.1 cluster — on a deterministic workload that produces byte-equivalent decoded response bodies to upstream Envoy's.* Every subsequent phase (HTTP/2 in phase 05, observability in phase 06) layers on top of this HTTP-aware dataplane.

## 2. Non-purposes

Phase 04 does **not** do any of the following. Each is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

- **HTTP/2 downstream or upstream.** The HCM proto carries `codec_type` (`HTTP1`, `HTTP2`, `HTTP3`, `AUTO`); phase 04 honours only `HTTP1` (and treats `AUTO` as `HTTP1` — the only codec compiled in). `HTTP2` and `HTTP3` error at build time with "phase 04 supports only HTTP/1.1 codec_type". → phase 05.
- **HTTPS (HTTP-over-TLS).** Phase 04's listener has no `transport_socket`. HCM works on a plaintext listener only. The phase-03 TLS plumbing exists in the listener manager but is not exercised by a phase-04 fixture. → phase 04.x or phase 05.x or a dedicated HTTPS-fixture sub-phase, depending on phase-05's planning split. The HCM filter itself is transport-agnostic (it sees a `net.Conn` that may already be a `*stdtls.Conn` from the phase-03 handshake) — no HCM-internal change is required to add HTTPS later.
- **Streaming-with-buffered-body or filter-side body modification.** Phase 04's HTTP filter chain is degenerate: exactly one filter (router), no per-filter body buffer. Bodies are streamed through with `io.Copy` semantics. No `BufferLimits`, no `MaxRequestBytes`, no `MaxResponseBytes`. → phase 07 framework + family-of-HTTP-filters phases.
- **`http_filters[]` other than `[router]`.** Phase 04 errors at build time if `http_filters` is empty, contains anything other than the router, or contains the router twice. → phase 07.
- **Per-route filter config (`typed_per_filter_config`).** Errors at build. → phase 07.
- **Route match predicates other than `prefix` and `path`.** `safe_regex`, `path_separated_prefix`, `connect_matcher`, `path_match_policy`, `headers[]`, `query_parameters[]`, `dynamic_metadata[]`, `runtime_fraction`, `grpc`, `tls_context`, `case_sensitive=false` (only `true` is allowed; the field defaults to true if absent), `path_specifier` oneof variants other than the two listed — all error at build. → phase 07 / HTTP-filters family.
- **Multi-vhost matching.** `route_config.virtual_hosts[]` must contain exactly one entry. That entry's `domains[]` must contain exactly one element, the literal `"*"`. Any other shape errors at build. → phase 07.
- **Route action types beyond `direct_response` and `route`.** `redirect`, `weighted_clusters`, `cluster_specifier_plugin`, `cluster_header`, `dynamic_route_actions`, `non_forwarding_action`, `filter_action`, route-level `host_rewrite_*`, `auto_host_rewrite`, route-level `timeout` / `retry_policy` / `hash_policy` / `request_headers_to_add` / `response_headers_to_add` / `prefix_rewrite` / `regex_rewrite` — all error. → phase 07 + HTTP-filters family + upstream-robustness family.
- **`direct_response.body` shapes beyond `inline_string`.** `inline_bytes` and `filename` error in phase 04 (consistent with phase-03's DataSource scope reduction for the comparable surface — and unlike phase-03's TLS scope, an HTTP body has no equivalent CA-chain pull-from-disk justification at phase 04). The `body` field must be present and non-empty. → phase 07.
- **`route.route.cluster` shapes beyond a string cluster name.** `cluster_specifier_plugin`, `cluster_header`, `weighted_clusters`, `inline_cluster_specifier_plugin` all error. → load-balancing family.
- **Persistent-connection upstream pooling.** Phase 04 dials a fresh upstream conn per HTTP request via `Cluster.Dial(ctx)`. No `Connection: keep-alive` honoured upstream-side; the upstream conn is closed after each request via `defer upstreamConn.Close()`. This is a known performance leak relative to upstream Envoy (which pools); the differential gate does not assert connection-reuse, so the divergence is permitted at phase 04. → upstream-robustness family.
- **HTTP request `Expect: 100-continue` semantics.** Phase 04 errors with status `417 Expectation Failed` on any request carrying an `Expect:` header (whether `100-continue` or any other expectation). Envoy supports `100-continue` natively; phase 04 does not. The fixture driver does not exercise `Expect`. → phase 07.
- **HTTP trailers (request or response).** Phase 04 sets `req.Trailer = nil` after `ReadRequest` (stdlib behaviour: `req.Trailer` is non-nil only with `Transfer-Encoding: chunked` and a `Trailer:` header naming trailers; phase-04 fixtures don't exercise this) and likewise `resp.Trailer = nil`. The fixture driver does not exercise trailers. → phase 05+ when HTTP/2 makes them load-bearing.
- **`HEAD`, `OPTIONS`, `CONNECT`, `TRACE`, `PATCH` method-specific semantics.** Phase 04 treats every method uniformly: parse, match, route. `CONNECT` does NOT install a tunnel (would error at upstream `Write` because the upstream isn't a CONNECT-aware peer); phase-04 fixtures don't issue `CONNECT`. `HEAD` is forwarded as-is; if the upstream responds with no body, phase 04 forwards no body. → phase 07 / family-of-HTTP-filters phases for method-specific filter behaviour.
- **`Connection: Upgrade` (WebSocket and HTTP/1.1 upgrades).** Errors at HCM with "phase 04 does not support upgrades". → upgrade-family phase. The fixture driver does not exercise upgrades.
- **HCM `tracing`, `access_log[]`, `http_protocol_options`, `common_http_protocol_options`, `server_header_transformation`, `local_reply_config`, `internal_redirect_policy`, `request_id_extension`, `path_with_escaped_slashes_action`, `merge_slashes`, `xff_num_trusted_hops`, `via`, `proxy_100_continue`, `stream_idle_timeout`, `request_timeout`, `request_headers_timeout`, `drain_timeout`, `delayed_close_timeout`, `forward_client_cert_details`, `original_ip_detection_extensions`.** All silently ignored at parse time (recorded as phase-04 ignored-set in §9). `stat_prefix` is read and stored on `Filter` for forward use (stats land in phase 06) but not emitted. `idle_timeout` is silently ignored (the connection loop exits when the downstream closes or `Connection: close` is sent; idle timeouts land later). Errors are not raised on any of these fields because phase-04 fixtures may inherit upstream-Envoy bootstraps that include them; matching upstream Envoy's forward-compatible posture on irrelevant-to-the-asserted-surface fields.
- **`server_header_transformation` and Server header.** Phase 04 sets `Server: envoy` on every locally-generated response (direct_response, 404, 400, 502, 503), matching the phase-01 admin `/ready` precedent (ADR-0014). Proxied responses (router action) preserve the upstream's `Server:` header value verbatim. The phase-04 BEHAVIOR_CONTRACT subsection codifies this rule.
- **`x-envoy-*` request and response headers.** Envoy adds `x-envoy-original-path`, `x-envoy-decorator-operation`, `x-envoy-internal`, `x-forwarded-proto`, `x-forwarded-for`, `x-request-id`, `x-envoy-expected-rq-timeout-ms`, etc. Phase 04 adds NONE of these. The BEHAVIOR_CONTRACT phase-04 header allow-list lists every `x-envoy-*` and `x-forwarded-*` header as "presence-permitted on upstream side, presence-not-required on subject side" — i.e., the subject is permitted to omit them and the differential diff filters them out before set-comparison. → observability family + HTTP-filters family.
- **`Date:` response header value comparison.** Already in the allow-list per ADR-0015; phase-04 reaffirms (presence required on locally-generated responses; value not byte-compared).
- **`Content-Length` vs `Transfer-Encoding: chunked` framing equivalence.** The phase-01 admin `/ready` precedent established that the harness decodes both sides before body comparison. Phase 04 generalises: for any HTTP/1.1 response (subject or upstream), the differential harness reads the response via `http.ReadResponse` (which decodes both framings transparently) and compares the resulting body bytes. The two proxies are permitted to differ on their choice of framing per response. → BEHAVIOR_CONTRACT phase-04 entry.
- **Stats, access logs, tracing, runtime overrides.** All deferred. → phases 06 / observability family.
- **Filter-chain matching beyond phase-03's SNI subset.** Phase-04 listeners are plaintext (no `transport_socket`) and have a single filter chain with empty `filter_chain_match`. Multi-chain on a phase-04 plaintext listener errors at listener build (carrying forward phase-03's "all-chains-must-be-TLS or all-plaintext-with-empty-match" rule). → phase 07.
- **Cluster types other than STATIC (subject side).** Unchanged from phase 02. → later phase.
- **LB policies other than ROUND_ROBIN.** Unchanged. → load-balancing family.
- **Upstream TLS exercised differentially.** ADR-0035's narrowed scope carries forward: fixture 0003 uses plaintext upstream backends. The phase-03 `Cluster.Dial` TLS branch remains unit-tested only. Closing the upstream-TLS differential gap is deferred to a future HTTPS-fixture sub-phase, which may land before or after phase 05 depending on planning. → phase 04.x or phase 05 follow-up.
- **Graceful drain of in-flight HTTP requests.** SIGINT behaviour unchanged from phase 02/03: listener sockets close, in-flight connections drop. → phase 08.

## 3. Phase-done gates (specialization of §7.5)

Per doctrine `D-3.6`, phase 04 lands only when every gate below is green. The generic `BOOTSTRAP_PROMPT.md` §7.5 gate set is narrowed:

| Gate | Specialization for phase 04 |
|---|---|
| (a) new/changed differential fixtures green | New fixture `test/fixtures/0003-http11-routing/` passes: byte-equivalent decoded response bodies on every request issued by the driver (status, body, headers set-equal modulo allow-list) across 27 requests per proxy (9 per route × 3 routes; see §5.10). Per-cluster RR distribution `[3, 3, 3]` on each proxy across the 9 router-action requests (3 per backend, mod-3 partition). `direct_response` requests return status 200 with body `OK\n` byte-exactly. No-match requests return status 404 from both proxies (body byte-comparison may be relaxed if upstream Envoy's local-reply body differs from envoy-go's; the phase-04 BEHAVIOR_CONTRACT subsection records the rule). |
| (b) all pre-existing differential fixtures still green | `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp` all pass without regression under their existing `expectations.yaml`. The TCP echo, RR distribution, and TLS plaintext-equivalence assertions still green. Admin `/ready` byte-exact still green on both. |
| (c) conformance suites pass | No conformance suite applies to phase 04 (h2spec is phase 05; h3spec later; grpc later; proxy-wasm later). HTTP/1.1 has no project-internal conformance suite at this phase; if a future phase introduces an `h1spec`-like suite, it would activate retroactively. This gate is vacuously green. |
| (d) new fuzzer runs clean for CI short-budget | New fuzz target `internal/filter/hcm.FuzzHCMConfigParse` runs clean for its CI short-budget run (30-second policy inherited from ADR-0018). Phase-01 `internal/bootstrap.FuzzBootstrapLoad`, phase-02 `internal/filter/tcpproxy.FuzzTcpProxyFilter`, and phase-03 `internal/tls.FuzzTLSContextParse` also run clean (no regression). |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | Standard. Unit tests for `internal/filter/hcm/` (HCM config parse, route table build, route match, direct_response synthesis, router dial+relay, codec read/write, connection loop persistence + close) plus extended tests for `internal/listener/` (HCM-as-network-filter registration) plus extended tests for `cmd/envoy-go/main_test.go` (HCM-bootstrap end-to-end smoke) all part of `go test ./...`. |
| (f) `REVIEW.md` approved | Per `SKILL_ROUTING.md` state 5. |

## 4. Deliverables (files and directories)

Grouped by lifecycle. Every path below is either new or materially changed.

### 4.1 New production code

- **`internal/filter/hcm/filter.go`** — exposes `Filter` (registered with the listener manager via `NewFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error)`), `Filter.Handle(ctx context.Context, downstream net.Conn) error`. Build-time errors begin with `hcm: `; runtime errors begin with `hcm: ` and are dropped by the connection loop (per phase-02 `_ = io.Copy` precedent). The filter holds: the resolved route table (§5.4), the resolved cluster manager handle, the configured `stat_prefix` (forward-looking; not used in phase 04), and the codec wiring.
- **`internal/filter/hcm/config.go`** — typed_config parser. Validates the type_url is `type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager`. Reads `codec_type` (must be `HTTP1` or `AUTO`; `HTTP2`/`HTTP3` error). Reads `stat_prefix` (string, mandatory — Envoy errors at config validation if absent). Reads `route_specifier`: must be `route_config` (inline); `rds` errors with "phase 04 does not support RDS"; `scoped_routes`/`scoped_rds` error similarly. Reads `route_config.virtual_hosts[]`: must have exactly one entry whose `domains: ["*"]`; otherwise errors. Reads `http_filters[]`: must be exactly one entry with `name == "envoy.filters.http.router"` (Envoy convention) AND `typed_config.type_url == "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"`; the Router proto body itself is read but no Router fields are honoured at phase 04 (every Router-proto field is silently ignored). All fields enumerated in §2 as "errors at build" produce a build-time error. All fields in §9 as "silently ignored" pass through without error or processing.
- **`internal/filter/hcm/route.go`** — route match engine. Type `routeTable struct{ routes []routeEntry }`. `routeEntry` carries the match predicate (one of `matchPrefix(string)` or `matchPath(string)` — implemented as a small interface, or a tagged-union struct, planner picks per §10) and the resolved action (`directResponseAction` or `routerAction`). `match(req *http.Request) (*routeEntry, bool)` walks the routes in order; first matching predicate wins (Envoy semantics). Match is on the request URI's path component (`req.URL.Path`); query string is excluded from the match input. `prefix` matches with strict prefix semantics: `prefix: "/api"` matches `/api`, `/api/`, `/api/v1` but not `/apiv1` (Envoy uses path-segment-aware semantics; phase 04 implements the simpler bytewise-prefix that DOES match `/apiv1` and documents the divergence in BEHAVIOR_CONTRACT — the fixture driver only sends paths that fall on segment boundaries so the divergence is not exercised). `path` matches exact-equal (case-sensitive). All path matches are case-sensitive (Envoy default; `case_sensitive=false` errors at build).
- **`internal/filter/hcm/actions.go`** — two action implementations. `directResponseAction{ status int, body string }` writes `HTTP/1.1 <status> <reasonPhrase>\r\n` followed by `Content-Length`, `Content-Type: text/plain`, `Server: envoy`, `Date: <RFC7231 IMF-fixdate>`, blank line, body. The reason phrase comes from `http.StatusText(status)`; if zero/unknown, falls back to empty string. `routerAction{ clusterName string }` resolves the cluster via the cluster manager (resolved at build-time, so the action carries `*cluster.Cluster` directly to avoid runtime lookups), then on each invocation: dials via `cluster.Dial(ctx)`; `defer upstreamConn.Close()`; writes the request via `req.Write(upstreamConn)`; reads response via `http.ReadResponse(bufio.NewReader(upstreamConn), req)`; writes response via `resp.Write(downstream)`; returns. Error from any step is returned and the connection loop closes the downstream.
- **`internal/filter/hcm/codec.go`** — codec helpers wrapping stdlib. `readRequest(br *bufio.Reader) (*http.Request, error)` calls `http.ReadRequest`. `writeStatusReply(w io.Writer, status int, body string)` writes a complete local-reply HTTP/1.1 response with the headers and body described under `directResponseAction` above. `serverHeader() string` returns `"envoy"` (matching ADR-0014). `dateHeader() string` returns `time.Now().UTC().Format(http.TimeFormat)` (RFC 7231 IMF-fixdate, matching phase-01). All locally-emitted responses go through `writeStatusReply`; the router action goes through stdlib's `resp.Write`.
- **`internal/filter/hcm/connection.go`** — per-connection driver. `runConnection(ctx, downstream net.Conn, table *routeTable)`: wraps the downstream in a `bufio.Reader` and a `bufio.Writer`; loops:
  - `req, err := readRequest(br)`. On `io.EOF`: clean exit (downstream closed). On any other parse error: `writeStatusReply(bw, 400, "")`; `bw.Flush()`; `return`.
  - Phase-04-out-of-scope guard: if `req.Header.Get("Expect") != ""`: `writeStatusReply(bw, 417, "")`; `bw.Flush()`; close downstream and `return`.
  - Phase-04-out-of-scope guard: if `req.Header.Get("Upgrade") != ""` OR `connection.HasUpgrade(req)`: `writeStatusReply(bw, 501, "")`; `bw.Flush()`; close downstream and `return`. (501 = Not Implemented; matches Envoy's behaviour for unsupported upgrades when the upgrade filter chain isn't configured.)
  - `entry, ok := table.match(req)`. If `!ok`: `writeStatusReply(bw, 404, "")`; `bw.Flush()`; check Connection-close behaviour; continue or close.
  - Else: invoke `entry.action.do(ctx, req, bw)`. The action is responsible for writing the full response (status line, headers, body) to `bw`. After the action returns, `bw.Flush()`.
  - Persistent-connection check: HTTP/1.1 default is keep-alive. Close the connection if the request OR the response carried `Connection: close`. Otherwise loop. On loop, drain `req.Body` to ensure it's fully consumed (`io.Copy(io.Discard, req.Body)`) before reading the next request — this is required to maintain the request-stream alignment.
- **`internal/filter/hcm/filter_test.go`** — unit tests for `NewFilter` (happy path with one direct_response + one router route; build errors for every §2 errors-at-build case; build error for missing `stat_prefix`; build error for non-router HTTP filter; build error for two HTTP filters; build error for empty route_config; build error for unknown route action).
- **`internal/filter/hcm/route_test.go`** — exhaustive route-match table: prefix matches at and beyond boundary; path exact match; path mismatch; first-match-wins ordering; query-string-excluded-from-match.
- **`internal/filter/hcm/actions_test.go`** — `directResponseAction.do` writes the expected wire bytes (golden-string assertion); `routerAction.do` dials a loopback echo server, sends `GET /x HTTP/1.1`, asserts the echo round-trips; ctx-cancellation while waiting on upstream returns a 502 local reply.
- **`internal/filter/hcm/codec_test.go`** — `readRequest` happy and error paths; `writeStatusReply` golden-string for status 200, 400, 404, 417, 501, 502; date/server header presence; `Content-Length` correctness.
- **`internal/filter/hcm/connection_test.go`** — keep-alive (two requests on one conn); `Connection: close` on request closes the conn; `Connection: close` on response closes; bad request line returns 400 + closes; `Expect:` returns 417; `Upgrade:` returns 501; route not found returns 404.
- **`internal/filter/hcm/fuzz_test.go`** — `FuzzHCMConfigParse`. Seed corpus: one well-formed HCM Any (one direct_response + one router route + router-only http_filters list); one malformed (truncated) Any; one Any with a wrong type_url. Fuzz body: call `NewFilter` against the mutated bytes; assert no panic and that every returned error begins with `hcm:`. Short-budget `-fuzztime=30s` per ADR-0018.

### 4.2 Changed production code

- **`internal/listener/manager.go`** — the inline filter-type-URL registry now also recognises `type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager` → `hcm.NewFilter`. The phase-02 `tcp_proxy` mapping is unchanged. No other listener-manager change. The plaintext-listener single-filter-chain rule from phase 03 is unchanged: phase 04 fixture `0003-http11-routing` uses one filter chain with empty `filter_chain_match`, which already passes the phase-03 plaintext predicate.
- **`internal/listener/manager_test.go`** — extended: HCM type_url resolves to `hcm.NewFilter`; HCM with wrong shape errors with `listener: ...: hcm: ...` wrapping (the listener-side wrap comes from the existing `listener: filter %d: ...` discipline, the inner `hcm:` from the new filter package).
- **`internal/bootstrap/bootstrap.go`** — adds a blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"`, `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"`, and the route-config proto package (`_ "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"`) so `protojson` can round-trip phase-04 fixture bootstraps. Per ADR-0016's amendment policy, this addition is documented in PROGRESS, not a new ADR.
- **`internal/cluster/`** — unchanged. The HCM consumes `Cluster.Dial(ctx)` exactly as the TCP proxy does. The plaintext-cluster `Dial` path (no `transport_socket`) is the one phase 04 exercises.
- **`internal/filter/tcpproxy/`** — unchanged.
- **`internal/tls/`** — unchanged.
- **`cmd/envoy-go/main.go`** — unchanged at the wiring level. The listener manager's HCM registration is transparent to `main`.
- **`cmd/envoy-go/main_test.go`** — extended with a third bootstrap variant that exercises the HCM smoke path: a single plaintext listener with one HCM filter, one direct_response route. Asserts the binary serves the configured response on a `localhost` HTTP/1.1 client probe. The phase-02 / phase-03 bootstrap variants remain.

### 4.3 New harness and fixture code

- **`test/differential/fixture/fixture.go`** — extended with a small typed-extension for HTTP/1.1 differential expectations. The `Driver` interface gains an OPTIONAL `HTTPExpectations() []HTTPRequestExpectation` method (where present, the runner uses it; where absent — fixtures 0000/0001/0002 — the runner falls back to byte-comparison of the raw `[]byte` returned by `DriveReference`/`DriveSubject` per the phase-03 contract). `HTTPRequestExpectation` is a struct: `{Method, Path string; ExpectStatus int; ExpectBodyEquivalent bool}`. The runner orchestrates per-fixture HTTP request issuance separately on reference and subject, parses responses via `http.ReadResponse`, and compares status+body. Headers are diffed via the existing helpers under `test/helpers/` augmented with a phase-04 `HTTPHeaderDiff` that applies the phase-04 BEHAVIOR_CONTRACT allow-list.
   - Alternative considered: keep `Driver` interface unchanged and have the 0003 driver itself parse responses + assert. Picked the typed-extension approach because (a) the next HTTP fixture (0004 in phase 05 for HTTP/2) will want the same wire-format-aware comparison, and (b) parking the comparison in the runner keeps the per-fixture driver thin. The planner may invert this if it adds non-trivial machinery; both are SPEC-compatible.
- **`test/differential/runner_test.go`** — call sites updated for the optional `HTTPExpectations` extension. Blank-import for `test/fixtures/0003-http11-routing/driver` added.
- **`test/fixtures/0003-http11-routing/`** — new fixture directory. Contents:
  - `envoy-go.yaml` — subject bootstrap. 1 listener (`l_http`) binding `127.0.0.1:0`, 1 plaintext filter chain, 1 HCM network filter with: `codec_type: HTTP1`, `stat_prefix: ingress_http`, `route_config: { virtual_hosts: [{ name: vh_default, domains: ["*"], routes: [...] }] }`, `http_filters: [{name: envoy.filters.http.router, typed_config: {Router{}}}]`. Three routes: `match: {path: "/health"}` → `direct_response: {status: 200, body: {inline_string: "OK\n"}}`; `match: {prefix: "/api"}` → `route: {cluster: c_backend}`; `match: {prefix: "/"}` → `direct_response: {status: 404, body: {inline_string: "not found\n"}}`. (The third entry is a phase-04 explicit catch-all so the SPEC-required no-match-→-404 path is exercised symmetrically; planner may instead leave the catch-all implicit and rely on the connection loop's no-match 404 — both produce status 404; the planner records the choice in PLAN.md.) One STATIC cluster `c_backend` with three `lb_endpoints` (ports allocated by the runner — `127.0.0.1:<dyn>` × 3) and ROUND_ROBIN policy.
  - `envoy.yaml` — reference bootstrap. Same listener shape, same HCM config, same routes (with the same explicit-catch-all-or-not choice). One STRICT_DNS cluster `c_backend` pointing at `host.docker.internal` × three ports, with `dns_lookup_family: V4_ONLY` per ADR-0010. The HCM `route_config` is identical between the two bootstraps (verbatim, modulo cluster.address differences).
  - `expectations.yaml` — prose description (same convention as fixtures 0000/0001/0002): three routes, byte-equivalent decoded bodies, 9 requests per route per side (3 per backend on the routed-prefix path). Allow-list lines for `Date`, `Server`, `x-envoy-*`, and `x-forwarded-*` headers. ADR-0019 still defers structured form.
  - `README.md` — explains the fixture's purpose (HCM + route match + router + direct_response), the STATIC-vs-STRICT_DNS divergence (same as 0001/0002 + ADR-0027), the framing-divergence-permitted rule (per the new BEHAVIOR_CONTRACT subsection), the `--concurrency 1` reference pin inherited from ADR-0028.
  - `driver/driver.go` — fixture driver. `BackendCount() = 3`. `SubjectListenerName() = "l_http"`. `ReferenceListenerPort() = 15003` (next sequential after fixtures 0001's 15001 / 0002's 15002). `ReferenceBootstrap(backendPorts)` renders the reference YAML with `backendPorts[0..2]` as the three c_backend endpoints. `SubjectConfig` does the same for STATIC `127.0.0.1`. `DriveReference(ctx, addr)` / `DriveSubject(ctx, addr)`: each issues 27 HTTP/1.1 requests against the proxy's listener address, breakdown:
     - 9 × `GET /health HTTP/1.1` → expects 200, body `OK\n`.
     - 9 × `GET /api/v1/<n> HTTP/1.1` for n=0..8 → expects 200, body equal to the configured echo backend's response (HTTP/1.1 echo backends defined under §5.10 — each backend writes back `HTTP/1.1 200 OK\r\nContent-Length: <len>\r\n\r\nbackend-<idx>:<n>` where `<idx>` is the backend index 0..2).
     - 9 × `GET /missing/<n> HTTP/1.1` → expects 404 (body relaxed by the framing-divergence rule).
   Each request uses a fresh `net.Dial` (not a persistent client) so per-request connections form the basis of the RR distribution count. The driver returns the concatenated response-body bytes from all 27 requests (length-stable for assertion purposes; 9 × `OK\n` + 9 × `backend-<i>:<n>` of variable length + 9 × ANY-404-body).
   `AssertDistribution(refCounts, subjCounts [3]uint64) error` checks that each side's `c_backend` distribution is exactly `[3, 3, 3]` over the 9 router-action requests.
   `HTTPExpectations()` returns the 27 expectations enumerated above.
   `ProbeAdmin` same as phase 02 / 01.
  - `driver/driver_test.go` — a small unit test covering the distribution-assertion helper without the harness startup cost (mirror of fixture 0001's test).
- **`test/helpers/http.go`** — `HTTPRoundTrip(ctx context.Context, addr, method, path string, headers http.Header, body []byte) (*http.Response, []byte, error)`: dials TCP, writes a constructed request, reads a response via `http.ReadResponse`, returns the response and the body bytes. Used by the 0003 driver. Returns the body separately from `Response.Body` because callers want the raw bytes for byte-compare; `Response.Body` is fully read before the return.
- **`test/helpers/http_diff.go`** — `HTTPHeaderDiff(refHeaders, subjHeaders http.Header, allowList []string) []string`: returns the list of header-name differences after applying the allow-list rule. The allow-list entries are: `Date` (presence-only), `Server` (presence-only), `Content-Length` and `Transfer-Encoding` (framing-divergence-permitted), every `x-envoy-*` header, every `x-forwarded-*` header, `x-request-id`. Used by the runner.
- **`test/helpers/http_test.go` / `http_diff_test.go`** — round-trip + diff tests against loopback HTTP/1.1 servers.

### 4.4 Changed documentation and state

- **`docs/envoy-go/ROADMAP.md`** — phase 04 row: `status: planned → in-progress` at SPEC commit (per the project's actual phase-03 pattern, the row flips at SPEC review approval / state-2 entry, not at SPEC drafting; the SPEC commit itself does not flip ROADMAP). Transitions to `done` at the §5.3 phase-done commit.
- **`docs/envoy-go/STATE.md`** — updated at each lifecycle transition (SPEC drafted = state 1 candidate; SPEC reviewer-approved + ROADMAP flipped = state 2; PLAN written = state 3; …).
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — add new subsection **HTTP/1.1** covering: (a) decoded-body byte-equivalence per request; (b) status-code equivalence per request; (c) header set-equality modulo a phase-04 allow-list (`Date`, `Server`, `Content-Length` + `Transfer-Encoding` framing-divergence, every `x-envoy-*`, every `x-forwarded-*`, `x-request-id`); (d) framing-divergence-permitted rule (Content-Length vs Transfer-Encoding chunked is not asserted; the harness decodes both sides through `http.ReadResponse`); (e) route-match selection equivalence (same request method + path → same matched route on both proxies); (f) `direct_response` body byte-exactness on the asserted path; (g) router-action upstream-side request preservation (subject must forward Host, method, path-with-query, body verbatim except for the documented header-set divergence). Plus, in the adjacent **Header allow-list** table, append rows for the new allow-list entries with phase 04 / ADR-K provenance.
- **`docs/envoy-go/DECISIONS.md`** — new ADRs introduced by phase 04 (numbers assigned at planning/implementation time; the planner may adjust; expected starting number ADR-0037 based on phase-03's ADR-0036 tail, the planner verifies at write time). Anticipated:
  - **ADR-H:** HTTP/1.1 wire codec source — stdlib `net/http.ReadRequest` / `Response.Write`. Options considered: (H1) handcrafted RFC 7230/9112 parser+writer; (H2) stdlib parsers/serializers (the SPEC's choice); (H3) build on `net/http.Server` + `http.Handler`. (H1) is the most-control-most-code option; (H3) is forbidden by D-3.2 (`net/http.Server` injects Date / Content-Length / strips headers / enforces RedactHeaders — too much magic for a proxy). (H2) keeps the doctrine intent (no `httputil.ReverseProxy`, no third-party server core, no embedded fasthttp/Caddy/Traefik) while sidestepping the (H1) cost-of-correctness tax on RFC corner cases that stdlib already covers (chunked encoding, header continuation, Host enforcement). Documents the residual stdlib-driven divergences from upstream Envoy: header canonicalization, Host validation, method whitelist (stdlib doesn't reject custom methods but may lowercase the method on certain paths). All such divergences are observable on the asserted request-forwarding path; the BEHAVIOR_CONTRACT phase-04 subsection records each one. Supersedes nothing.
  - **ADR-I:** Phase-04 HTTP-filter framework subset. Permits exactly one HTTP filter, named `envoy.filters.http.router` with type_url `envoy.extensions.filters.http.router.v3.Router`. The filter-iteration protocol (decode-headers, decode-data, decode-trailers, encode-headers, encode-data, encode-trailers, stop/continue/buffer/etc.) is NOT introduced here; instead, the router is invoked by direct function call inside the HCM connection loop. The Router proto's body is read but every Router-proto field is silently ignored at phase 04. Reads `dynamic_stats`, `start_child_span`, `upstream_log[]`, `suppress_envoy_headers`, `strict_check_headers`, `respect_expected_rq_timeout`, `suppress_grpc_request_failure_code_stats`, etc. All silently ignored. Phase 07's filter-chain framework supersedes this with the actual iteration protocol. Supersedes nothing (phase 04 is the first HCM phase).
  - **ADR-J:** Phase-04 route match subset. Permits `match.prefix` (bytewise prefix on `req.URL.Path`) and `match.path` (case-sensitive exact equality on `req.URL.Path`). Documents the planned-divergence on `match.prefix` semantics: Envoy uses path-segment-aware prefix matching (so `prefix: "/api"` matches `/api`, `/api/`, `/api/x` but NOT `/apifoo`); phase 04 implements bytewise prefix (so `/apifoo` WOULD match). The fixture driver does not exercise `prefix` on a non-segment-boundary path, so the divergence is not surfaced in the differential gate. A future phase that exercises non-segment paths must either fix `match.prefix` to be segment-aware or extend BEHAVIOR_CONTRACT with the assertion. Other match fields error.
  - **ADR-K:** BEHAVIOR_CONTRACT HTTP/1.1 subsection. Codifies the phase-04 equivalence surface (see §5.7 below). Includes the new header allow-list entries (`Content-Length`, `Transfer-Encoding`, every `x-envoy-*`, every `x-forwarded-*`, `x-request-id`) with phase 04 provenance. The framing-divergence rule generalises the phase-01 admin-`/ready`-specific rule. Supersedes nothing.
  - **ADR-L:** Per-request fresh upstream dial in phase-04 router. Documents that the router action does NOT pool upstream connections — every routed request opens a new TCP connection to the picked endpoint via `Cluster.Dial(ctx)`. Rationale: connection pooling is a load-bearing concern with timeouts, idle-eviction, max-streams, etc., that belongs to the upstream-robustness family; phase 04 punts. Performance is suboptimal vs upstream Envoy (which pools); the differential gate does not assert pool/non-pool, so the divergence is permitted. Records the carry-forward to the upstream-robustness phase that introduces pooling.
  - **ADR-M:** Fixture-driver `HTTPExpectations` extension to the `Driver` interface. Documents the additive (optional method) shape so existing drivers (0000, 0001, 0002) need no change. Supersedes nothing (additive evolution of the ADR-0034 driver split). The planner may instead embed comparison in the driver itself (the alternative noted in §4.3 above) and skip ADR-M; the choice is recorded at PLAN.md write time.
  - **ADR-N:** HCM `stat_prefix` and silently-ignored field set. Documents the exact `HttpConnectionManager` proto fields phase 04 reads vs silently ignores vs errors-on. Stats land in phase 06; this ADR pre-commits the stat_prefix's storage shape so phase 06 has a clean handoff. The set of silently-ignored fields is the phase-04 ignored-set; phase 06+ may move members from "ignored" to "honoured" with a superseding ADR.
  - **ADR-O:** Phase-04 HTTP-filter chain shape — exactly `[router]`. Tightens phase-02's filter-chain rule to the HTTP-filter sub-domain. Supersedes nothing (phase 02's ADR-0033 covers network filter chains; phase 04 introduces the HTTP-filter sub-domain).
  - If additional decisions emerge at plan or implementation time (e.g., a request-body-streaming back-pressure policy, a router-action error-wrapping policy, a phase-04 idle-timeout default), they are ADR'd at that point. Expected starting ADR number is ADR-0037 based on phase-03's ADR-0036 tail; the planner verifies at write time.

## 5. Architecture and components

### 5.1 Module graph (new / changed shape)

Phase 04 introduces one new package and modifies one existing one:

```
cmd/envoy-go/main.go        (unchanged at wiring; bootstrap variant added in main_test.go)
internal/listener/manager.go (MODIFIED: HCM type_url registered)
internal/filter/hcm/         (NEW package)
  filter.go        — Filter + NewFilter
  config.go        — typed_config decode
  route.go         — match engine
  actions.go       — directResponseAction + routerAction
  codec.go         — read/write helpers
  connection.go    — per-conn loop
  fuzz_test.go     — FuzzHCMConfigParse
  *_test.go        — unit tests
internal/filter/tcpproxy/    (UNCHANGED)
internal/cluster/            (UNCHANGED — Cluster.Dial is consumed)
internal/tls/                (UNCHANGED — phase 04 fixture is plaintext)
internal/bootstrap/          (MODIFIED: HCM/router/route blank imports added)
test/differential/fixture/   (MODIFIED: HTTPExpectations extension)
test/fixtures/0003-http11-routing/ (NEW fixture)
test/helpers/http.go, http_diff.go  (NEW helpers)
docs/envoy-go/BEHAVIOR_CONTRACT.md  (MODIFIED: new HTTP/1.1 subsection)
docs/envoy-go/DECISIONS.md           (APPENDED: ADR-H..ADR-O ≈ ADR-0037..ADR-0044)
```

Imports (new): `internal/filter/hcm` depends on `github.com/envoyproxy/go-control-plane/envoy/{config/route,extensions/filters/network/http_connection_manager,extensions/filters/http/router}/v3`, on `google.golang.org/protobuf/types/known/anypb`, on `internal/cluster`, on `net/http`, `bufio`, `context`, `io`, `net`, `strings`, `time`. The HCM does NOT depend on `internal/tls` (the listener has already done the handshake by the time `Filter.Handle` is called — HCM sees a `net.Conn` regardless of whether it's wrapped TLS).

### 5.2 HCM as a network filter — registration

`internal/listener/manager.go` carries an inline `filterTypeURLs` switch (phase 02 introduced this; phase 04 extends). Phase-04 entries:

```go
case "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy":
    return tcpproxy.NewFilter(typedConfig, clusters)
case "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager":
    return hcm.NewFilter(typedConfig, clusters)
default:
    return nil, fmt.Errorf("listener %q: unknown filter type_url %q", lname, typedURL)
```

The exact code-shape detail (whether the switch lives in `manager.go` or moves to a small `filter_registry.go` for tidiness) is a planner / implementer choice. SPEC §5.1 records the resulting topology either way.

### 5.3 Connection driver — per-downstream-conn loop

Pseudocode for `runConnection(ctx, downstream net.Conn, table *routeTable)` (full version under §4.1's `connection.go`):

```
br := bufio.NewReader(downstream)
bw := bufio.NewWriter(downstream)
for {
    req, err := http.ReadRequest(br)
    if err == io.EOF { return }                     // clean downstream close
    if err != nil { writeStatusReply(bw, 400, ""); bw.Flush(); return }

    // phase-04-out-of-scope guards
    if req.Header.Get("Expect") != "" { writeStatusReply(bw, 417, ""); bw.Flush(); return }
    if req.Header.Get("Upgrade") != "" || req.Header.Get("Connection") == "Upgrade" { writeStatusReply(bw, 501, ""); bw.Flush(); return }

    closeAfter := strings.EqualFold(req.Header.Get("Connection"), "close")

    entry, ok := table.match(req)
    if !ok { writeStatusReply(bw, 404, ""); bw.Flush() }
    else { entry.action.do(ctx, req, bw) }   // action writes its own response into bw
    bw.Flush()

    // drain request body before next iteration (mandatory for pipelining correctness)
    _, _ = io.Copy(io.Discard, req.Body)
    req.Body.Close()

    if closeAfter { return }
    // also break if the action's response carried "Connection: close" — the action layer
    // signals this via a sentinel error or a callback flag; planner picks the mechanism.
}
```

The planner picks the close-after-action signal mechanism (sentinel error vs return-value flag). Either is SPEC-compatible.

### 5.4 Route table — build-time resolution

`NewFilter` resolves the inline `route_config.virtual_hosts[0].routes[]` into an in-memory `*routeTable`:

```go
type routeTable struct{ routes []routeEntry }
type routeEntry struct {
    match  routeMatch    // matchPrefix(string) | matchPath(string)  (interface or tagged-union; planner picks)
    action routeAction   // *directResponseAction | *routerAction    (interface)
}
```

For each `routes[i]`:
- Decode `match.path_specifier`. If it's `path: "X"` → store as `matchPath("X")`. If it's `prefix: "X"` → store as `matchPrefix("X")`. Any other `path_specifier` variant → build error.
- Decode `action`. If `route.direct_response`: validate `status` is in [100, 599], validate `body.specifier` is `inline_string`, store as `directResponseAction{status, body}`. If `route.route`: validate `route.cluster` is a non-empty string, look it up in the cluster manager, store as `routerAction{cluster}`. Any other `action` variant → build error.
- Build error wraps with `hcm: route %d: ...`.

The match function:

```go
func (t *routeTable) match(req *http.Request) (*routeEntry, bool) {
    p := req.URL.Path
    for i := range t.routes {
        switch m := t.routes[i].match.(type) {
        case matchPath:    if p == string(m)               { return &t.routes[i], true }
        case matchPrefix:  if strings.HasPrefix(p, string(m)) { return &t.routes[i], true }
        }
    }
    return nil, false
}
```

First-match-wins is the project-wide rule mirroring the phase-03 SNI match's "exact > suffix-wildcard > catch-all" semantic but flatter (phase 04 has no specificity scoring; routes are evaluated in declaration order). The fixture driver issues paths designed so order-dependent ambiguity is not exercised.

### 5.5 Route actions

`directResponseAction.do(ctx, req, bw)`:

```
writeStatusReply(bw, status, body)
return nil   // never errors
```

`routerAction.do(ctx, req, bw)`:

```
upstream, err := r.cluster.Dial(ctx)
if err != nil { writeStatusReply(bw, 503, ""); return nil }
defer upstream.Close()

// preserve the original request headers + body verbatim; stdlib req.Write handles
// chunked vs content-length framing automatically based on what was read.
if err := req.Write(upstream); err != nil { writeStatusReply(bw, 502, ""); return nil }

resp, err := http.ReadResponse(bufio.NewReader(upstream), req)
if err != nil { writeStatusReply(bw, 502, ""); return nil }
defer resp.Body.Close()

// resp.Write writes the full HTTP/1.1 response back to bw (status line + headers + body).
return resp.Write(bw)
```

Notes:
- Phase-04 errors on the upstream side become 502/503 local replies. `Cluster.Dial` failure → 503 (matches Envoy's "no healthy upstream" mapping at the coarse level — phase 04 isn't health-checking, so any dial failure is "no upstream"). Upstream parse failure or `req.Write` failure → 502.
- The router does NOT add any `x-envoy-*` or `x-forwarded-*` request headers. The upstream sees the unmodified downstream request (with the exception of stdlib's normalisations on the request line and headers during `http.ReadRequest`).
- `req.URL.Host` may be empty after `http.ReadRequest` (it's stored under `req.Host`). `req.Write` handles this.

### 5.6 Codec wrappers

`writeStatusReply(w, status, body)` writes:

```
HTTP/1.1 <status> <reasonPhrase>\r\n
Content-Type: text/plain\r\n
Content-Length: <len(body)>\r\n
Server: envoy\r\n
Date: <RFC7231 IMF-fixdate now()>\r\n
\r\n
<body>
```

`writeStatusReply` is the ONLY path that locally generates a response body. Router-action responses go through stdlib's `Response.Write`, which writes the upstream response verbatim (including its status line and its own Server/Date if present). The phase-04 BEHAVIOR_CONTRACT subsection records that locally-generated responses on subject and reference may differ in body and headers (Envoy's local-reply has a `text/plain` body with a JSON-shaped error message; envoy-go's local-reply has empty body or a plain string). The harness's status comparison is what's asserted; the local-reply body byte-comparison is relaxed by an explicit allow-list entry in `expectations.yaml` and BEHAVIOR_CONTRACT.

### 5.7 BEHAVIOR_CONTRACT — new HTTP/1.1 subsection (content preview)

Heading: `## HTTP/1.1`. Justification: ADR-K. Body covers:

- **Asserted equivalence**: response status code per request; decoded response body bytes per routed-to-upstream request; route-match selection (same method + path → same matched route on both proxies); upstream-side request preservation (verbatim Host, method, path-with-query, body — except where stdlib HTTP/1.1 parsing on the subject side introduces a bounded, documented normalisation).
- **Not asserted**: response-header set equality (only set-equality modulo allow-list); local-reply body bytes (Envoy and envoy-go differ in their default local-reply body content); `Content-Length` vs `Transfer-Encoding: chunked` framing per response (the harness decodes both sides via `http.ReadResponse`); upstream connection re-use (envoy-go does not pool; Envoy does); `x-envoy-*` / `x-forwarded-*` / `x-request-id` headers (envoy-go adds none; Envoy adds many — all in the allow-list).
- **Header allow-list extensions** (per ADR-K): `Date` (presence-only; reaffirms ADR-0015 in this scope), `Server` (presence-only; phase-01 precedent), `Content-Length` and `Transfer-Encoding` (framing-divergence-permitted), every `x-envoy-*` header (presence-not-required on subject), every `x-forwarded-*` header (same), `x-request-id` (same).
- **Applies to**: phase-04 envoy-go `internal/filter/hcm/` package, exercised via fixture `0003-http11-routing`.
- **Does not yet apply to**: HTTP/2 (phase 05); HTTP/3 (later); HCM filter chain beyond `[router]` (phase 07); upstream connection pooling (upstream-robustness family); HTTPS (phase 04.x or phase 05.x).

### 5.8 Fixture `0003-http11-routing` — three-route HTTP/1.1 round-trip

The fixture's job is to exercise every phase-04 deliverable in one bootstrap:

- HCM as the network filter (the only other phase-04 codepath, the listener's HCM-type-URL switch, is exercised by config parse).
- Route match — both `path` (`/health` → direct_response) and `prefix` (`/api` → router) — and the no-match path.
- Both action types — `direct_response` (with inline body) and `route` (proxy via `Cluster.Dial`).
- The `[router]`-only HTTP filters list.
- `codec_type: HTTP1`, `stat_prefix: ingress_http`.

The fixture is plaintext (no `transport_socket`). The listener has one filter chain with empty `filter_chain_match` — the phase-03 plaintext-listener single-chain-with-empty-match rule is satisfied.

The fixture's HTTP backends are HTTP/1.1 echo servers spawned inline by the runner (next to the existing TCP echo backends used by 0000/0001/0002 — the runner already has the multi-backend abstraction from phase 02; phase 04 adds an HTTP echo backend type alongside it). Each backend serves `HTTP/1.1 200 OK\r\nContent-Length: <len>\r\nContent-Type: text/plain\r\n\r\nbackend-<idx>:<body>`, where `<idx>` is the backend index 0..2 and `<body>` is the request URL path's last component (so the same request to backend 0 vs backend 1 produces different responses; this lets `AssertDistribution` distinguish RR-balanced backends). The backends are TCP listeners that read one HTTP/1.1 request via `http.ReadRequest`, write one HTTP/1.1 response via the bufio path, then close.

### 5.9 Differential expectation expression

The fixture uses the `HTTPExpectations()` interface extension (§4.3) to express the 27 per-side request expectations. The runner orchestrates the per-request comparison:

1. Driver returns a list of 27 `HTTPRequestExpectation` records.
2. For each expectation, the runner issues the request via `helpers.HTTPRoundTrip` against both reference and subject, captures status+body+headers, applies `helpers.HTTPHeaderDiff` with the phase-04 allow-list, compares.
3. Distribution is captured separately by counting which backend's body returned (the body-prefix `backend-<idx>:` is the natural distinguisher) for the 9 router-action requests, then asserted via `AssertDistribution` per the runner's existing per-side counts mechanism.

The planner may inline this orchestration into the driver itself (skipping the `HTTPExpectations` extension and ADR-M altogether) if the runner-side change is judged too invasive. Either path is SPEC-compatible.

### 5.10 Request count and distribution arithmetic

- 9 health requests per side → `direct_response` 200 `OK\n`. Trivially equivalent.
- 9 api requests per side → router action through `c_backend` (3 endpoints, RR). `[3, 3, 3]` per side per ADR-0028's `--concurrency 1` reference pin. Reference-side distribution and subject-side distribution may differ in *sequence* (Envoy's per-worker RR start-offset random is suppressed by `--concurrency 1` but the per-cluster RR tail is still unmoored across worker restarts; phase-02 ADR-0028 + the BEHAVIOR_CONTRACT TCP-proxy LB-sequence-not-asserted rule both apply). The runner checks distribution counts, not sequence.
- 9 missing requests per side → 404. Status equivalent on both sides; body relaxed.

Total: 27 requests per side, 54 across both proxies, 9 per route × 3 routes. The single `--concurrency 1` reference-side pin is enough to make `[3, 3, 3]` deterministic; phase 04 adds no harness-runtime changes.

## 6. Data flow

### 6.1 Startup

1. `cmd/envoy-go/main.go` calls `bootstrap.Load(path)` which yields a `*bootstrapv3.Bootstrap` proto.
2. `cluster.NewManager(bs)` builds the cluster manager (unchanged from phase 03).
3. `listener.NewManager(bs, clusters)` builds the listener manager. For each listener: for each filter chain: for each filter: dispatch on the filter's `typed_config.type_url`. Phase 04: `tcp_proxy` → `tcpproxy.NewFilter`; `HttpConnectionManager` → `hcm.NewFilter` (NEW). Each `hcm.NewFilter` resolves its route_config and validates its http_filters list at build time; any violation aborts startup.
4. `listener.Manager.Start(ctx)` opens the listener sockets and launches Accept loops (unchanged).
5. `admin.MarkReady()` (unchanged).

### 6.2 Connection (HCM listener)

1. Accept loop returns a `net.Conn`.
2. The single (phase-04) filter chain dispatches the conn to its terminal filter — for the HCM listener, that's `hcm.Filter.Handle(ctx, conn)`.
3. `Filter.Handle` calls `runConnection(ctx, conn, f.routeTable)` (§5.3).
4. `runConnection` loops reading requests until clean EOF, parse error, `Connection: close`, or out-of-scope guard trip.
5. Per request: route match → action → response write → drain body → check close-after.
6. On loop exit: `conn.Close()` (deferred).

### 6.3 Connection (existing TCP listener types)

Unchanged from phase 02/03.

### 6.4 Shutdown

Unchanged from phase 02/03.

## 7. Error handling and failure modes

Single rule (preserved): every error crossing a package boundary begins with `<package>: ` (`hcm:`, `listener:`, `cluster:`, `tcpproxy:`, `tls:`).

| Failure site | Class | Handling |
|---|---|---|
| `hcm.NewFilter`: wrong type_url; non-HTTP1 codec_type; missing stat_prefix; non-`route_config` route_specifier; route_config with !=1 vhost or vhost domains != ["*"]; route with unknown action; route with disallowed match field; route with invalid status (≥600 / <100); direct_response body shape other than inline_string; route.route.cluster missing or unknown; http_filters not [router]; Router proto with disallowed shape (any shape works at phase 04) | build-time | Return error; surfaced via listener manager with `listener: filter %d: hcm: ...` wrapping. |
| `runConnection`: `http.ReadRequest` returns non-EOF error | runtime | `writeStatusReply(bw, 400, "")`, flush, close downstream. |
| `runConnection`: `Expect:` header present | runtime | `writeStatusReply(bw, 417, "")`, flush, close. |
| `runConnection`: `Upgrade:` header or `Connection: Upgrade` | runtime | `writeStatusReply(bw, 501, "")`, flush, close. |
| `runConnection`: route table no-match | runtime | `writeStatusReply(bw, 404, "")`, flush, continue or close per `Connection: close`. |
| `routerAction.do`: `Cluster.Dial` failure | runtime | `writeStatusReply(bw, 503, "")`, flush; do not close (loop continues). |
| `routerAction.do`: `req.Write` to upstream fails | runtime | `writeStatusReply(bw, 502, "")`, flush; close upstream; do not close downstream. |
| `routerAction.do`: `http.ReadResponse` fails | runtime | `writeStatusReply(bw, 502, "")`, flush; close upstream. |
| `routerAction.do`: `resp.Write` to downstream fails | runtime | propagate; the connection loop handles. |
| Listener bind error | startup | Same as phase 02/03. `log.Fatalf`. |
| SIGINT | shutdown | Unchanged from phase 02/03. |
| Bootstrap loader on `dynamic_resources` / `layered_runtime` | build-time | Unchanged. |

## 8. Testing scope for phase 04

Three layers.

### 8.1 Unit tests

- `internal/filter/hcm/{filter,config,route,actions,codec,connection}_test.go` — coverage enumerated in §4.1.
- `internal/listener/manager_test.go` — extended for HCM type_url registration; HCM build error wrapping.
- `cmd/envoy-go/main_test.go` — bootstrap variant added (HCM smoke).

### 8.2 Fixture-level (differential)

- `test/fixtures/0000-tcp-echo/`, `test/fixtures/0001-tcp-proxy-rr/`, `test/fixtures/0002-tls-tcp/` — unchanged behaviour; if `Driver` interface gains the optional `HTTPExpectations()` method (§4.3 alternative), no driver-side change for these three.
- `test/fixtures/0003-http11-routing/` — new; three-route HTTP/1.1 round-trip with byte-equivalent decoded body, status-code equivalent, distribution-asserted.

### 8.3 Conformance

None for phase 04.

## 9. Out-of-scope (explicitly deferred)

All items in §2 remain deferred. Additionally, the following HCM proto fields are silently ignored at build time (phase-04 ignored-set, frozen by ADR-N):

- `tracing`, `access_log[]`, `http_protocol_options`, `common_http_protocol_options`, `server_header_transformation` (phase-04 forces `Server: envoy` regardless), `local_reply_config`, `internal_redirect_policy`, `request_id_extension`, `path_with_escaped_slashes_action`, `merge_slashes`, `xff_num_trusted_hops`, `via`, `proxy_100_continue`, `stream_idle_timeout`, `request_timeout`, `request_headers_timeout`, `drain_timeout`, `delayed_close_timeout`, `forward_client_cert_details`, `original_ip_detection_extensions`, `idle_timeout`, `max_request_headers_kb`, `request_headers_kb_limit`, `add_user_agent`, `set_current_client_cert_details`, `mutex_tracing`, `proxy_status_config`, `early_header_mutation_extensions`, `header_validation_config`, `append_local_overload`, `pass_through_is_optional`, `request_block_size`, `strip_matching_host_port`, `strip_any_host_port`, `strip_trailing_host_dot`, `add_proxy_protocol_connection_state`.

The route-level silently-ignored fields:

- `request_headers_to_add`, `request_headers_to_remove`, `response_headers_to_add`, `response_headers_to_remove`, `metadata`, `decorator`, `tracing`, `per_request_buffer_limit_bytes`, `match.case_sensitive` (only `true` is honoured; `false` errors), `match.runtime_fraction`, `match.headers`, `match.query_parameters`, `match.dynamic_metadata`, `match.tls_context`, `match.connect_matcher`.

The router-filter (Router proto) silently-ignored fields:

- `dynamic_stats`, `start_child_span`, `upstream_log[]`, `suppress_envoy_headers`, `strict_check_headers`, `respect_expected_rq_timeout`, `suppress_grpc_request_failure_code_stats`, `upstream_http_filters`.

## 10. Deferred decisions (the planner / implementer settles these)

These are intentionally left open for the planning or implementation session to decide. None change the shape of the SPEC; all are implementation-detail choices whose outcome is recorded in PLAN.md or as PROGRESS/ADR notes.

1. **Route match representation.** Either (a) interface `routeMatch` with two implementations `matchPrefix` and `matchPath`, or (b) tagged-union struct `routeMatch{kind matchKind; pattern string}`. Recommend (a) for type-safety; planner picks.
2. **Action representation.** Same choice as #1: interface vs tagged-union. Recommend (a).
3. **Close-after-action signal mechanism.** Either (a) a sentinel error returned by `action.do` indicating "response carried Connection: close", or (b) a `*bool` callback flag passed to `action.do`, or (c) the connection loop re-parses the response headers from `bw`'s underlying buffer (rejected — invasive). Recommend (a).
4. **`HTTPExpectations` extension to `Driver` vs in-driver comparison.** Either land ADR-M and the optional interface method (per §4.3 / §5.9), or skip ADR-M and have the 0003 driver internalise the comparison. Recommend the extension because phase-05 HTTP/2 will want the same shape; planner confirms.
5. **Explicit catch-all 404 route in fixture 0003 vs implicit no-match 404.** Either land the route `match.prefix: "/"` with `direct_response: 404` body `not found\n`, or skip it and let the connection loop's no-match path emit the local 404. Recommend explicit (matches Envoy's typical config posture); planner confirms.
6. **HTTP echo backend isolation.** Either (a) one in-process `http.Server` per backend behind a `net.Listen`, or (b) handcrafted bufio-driven backend matching the rest of the helper code. Recommend (a) — the backends are test fixtures, not differential subjects, so stdlib usage is fine.
7. **Header allow-list shape in `helpers.HTTPHeaderDiff`.** Either (a) a fixed in-code list, or (b) a configurable list per fixture. Recommend (a) for phase 04 (one fixture); phase 05 may move to (b).
8. **Date header source.** Either (a) `time.Now().UTC().Format(http.TimeFormat)` per response (current SPEC), or (b) cached + refreshed once per second (Envoy does this for performance). Recommend (a) for simplicity; (b) is a phase-06+ optimisation if profiling shows hot allocation.
9. **`stat_prefix` storage shape.** Either (a) a `string` field on `Filter`, or (b) a `*statPrefix` struct preallocated to forward-look at phase 06's stats extension. Recommend (a) for minimum surface; phase 06 can extend.
10. **Whether to introduce `internal/http/` as a package now or keep everything inside `internal/filter/hcm/`.** The repo has an `internal/http/doc.go` placeholder from phase 00 saying "the real implementation lands in phase 04." Recommend keeping everything inside `internal/filter/hcm/` for phase 04 and either deleting the placeholder or letting it sit (the placeholder package's `doc.go` can be amended to point at the hcm package). Planner picks.
11. **ADR numbering.** The SPEC lists anticipated ADRs H–O with explicit purposes. At landing time, the planner assigns sequential numbers starting from the current highest ADR + 1 (expected ADR-0037..ADR-0044 based on phase-03's ADR-0036 tail).
12. **Subject-side `Server: envoy` vs `Server: envoy-go`.** Phase 01 chose `envoy` (ADR-0014) for the admin `/ready` response. Phase 04 reaffirms `envoy` for HCM-locally-generated responses (proxied responses preserve upstream's `Server`). The planner may instead pick `envoy-go` and supersede ADR-0014; recommend not — `envoy` matches what BEHAVIOR_CONTRACT already documents and lets the differential header allow-list keep its `Server: presence-only` rule unchanged.
13. **`Content-Type: text/plain` on locally-generated bodies vs no `Content-Type`.** Recommend `text/plain` for direct_response (consistent with the configured body format) and `text/plain` for 4xx/5xx local replies (consistent with phase-01 admin). Planner confirms.
14. **HTTP echo backend body format.** Recommend `backend-<idx>:<n>` per §5.10. Planner picks the exact format (the only constraint is that backends 0/1/2 produce distinguishable bodies so `AssertDistribution` can count).

## 11. Risks and mitigations

| Risk | Mitigation |
|---|---|
| `net/http.ReadRequest` rejecting an input that upstream Envoy accepts (or vice versa). | The differential gate's body-byte-equality assertion is on *successful* round-trips; a request rejected by stdlib parsing produces a 400 on the subject and (likely) some other status on the reference. The fixture driver only issues well-formed requests, so this divergence is not exercised. A future phase may add a "parse-divergence" fuzz that issues malformed requests and compares status codes; not phase-04 scope. |
| `req.Write` to upstream losing or reordering headers vs stdlib's write of what was just read. | `http.Request.Write` is the symmetric writer for `http.ReadRequest`; stdlib documents round-trip preservation for HTTP/1.1 (modulo CanonicalMIMEHeaderKey on header names — which is already a documented Envoy header allow-list rule — and modulo Host being stored separately). A fuzz target for the parse path catches catastrophic regressions; per-request body byte comparison catches subtle ones. |
| `http.ReadResponse` interpreting the upstream echo backend's `Content-Length` header strictly and erroring on a body shorter than declared. | The HTTP echo backend writes the exact `Content-Length` it declares. Unit-tested. |
| `bufio.Writer` flush ordering on close — unflushed bytes lost when the loop exits via guarded path. | Every guarded path explicitly calls `bw.Flush()` before `return`. Coverage in `connection_test.go`. |
| `req.Body` not fully drained between requests on a keep-alive connection, causing the next `http.ReadRequest` to read body bytes as the request line. | The connection loop calls `io.Copy(io.Discard, req.Body); req.Body.Close()` between iterations. Unit-tested. |
| `Connection: close` on the response not being honoured (loop continues, then next read fails). | The action layer signals close-after via a sentinel error / flag (planner choice in §10 #3). Connection loop checks both directions on every iteration. |
| `routerAction` upstream connection leak on partial-response paths (write succeeds, read fails, defer never fires). | `defer upstream.Close()` is the first statement after `Cluster.Dial` returns successfully. Coverage. |
| Stdlib `http.Server`-style behaviours leaking in via accidental import (e.g., someone wires `http.Handler` later). | The package interface is `Filter.Handle(ctx, net.Conn)`. There is no `http.Handler` glue. Code review enforces. |
| Phase-03 REVIEW Minor 2 (Listener `Stop`/`Listeners` race) re-surfacing under HCM connections. | HCM is a network filter; it doesn't change the listener-manager's lock discipline. Phase 04 does not mitigate Minor 2; it carries forward to the next phase that touches the listener-manager lock surface (or to a dedicated cleanup commit). |
| Phase-03 REVIEW Minor 5 (`cluster/manager.go` "phase 02" error texts) lingering. | Phase 04 does not touch the cluster manager. Carries forward; planner may opportunistically fix in a touch-adjacent commit. |
| Fixture 0003 distribution flaking because of HTTP/1.1 request-line variation introducing keep-alive/non-keep-alive variance. | Driver uses `Connection: close` on every request (per-request fresh dial). RR deterministic via `--concurrency 1`. |
| `http.ReadRequest` returning a `req.URL` whose `Path` is not what was on the wire (escaping, normalization). | Stdlib does not unescape `Path`; `req.URL.Path` is the wire-form path. Unit-tested. |

## 12. Phase-03 REVIEW carryover triage

Phase-03 REVIEW (`docs/envoy-go/phases/03-tls/REVIEW.md` commit `d45c467`, "Phase 03 — TLS Review", §Findings) lists 0 Critical, 4 Important, and 8 Minor findings. Important items I-1..I-4 already landed in commit `98cc35b` and were re-verified in `cbfe275` — they are NOT carry-forwards; they are closed at phase-03 close. The 8 Minor items are the carry-forwards. Phase-04 triage follows.

1. **M-1 — ADR-0033 `Supersedes:` header punctuation drift.** DEFERRED. Cosmetic; harmless to readers; addressing it requires editing a landed ADR (D-3.5 forbids editing landed ADRs; the fix would be a one-line "supersession-of-formatting" ADR which is itself worse than the drift). Carries forward as a phase-N+ doctrine note candidate; phase 04 does not address.
2. **M-2 — Listener `Stop()`/`Listeners()` race on `rt.netLn`.** DEFERRED. Phase 04 does not touch `internal/listener/manager.go` lock discipline (the phase-04 change is a switch-arm extension, not a state-change). The fix is one line (extend `m.startedMu` scope to `Listeners()`); a phase that exercises `go test -race` on the listener manager (likely phase 06 when stats are added with their own readback paths) is the natural site.
3. **M-3 — `chainSpecificityRank` initial `rank := 4` defensive sentinel.** NO-ACTION / DEFERRED. Defensive coding that compiles correctly; tightening to `var rank int = 2` is a pure-style change. Phase 04 does not touch the listener manager body. Carries forward for opportunistic cleanup.
4. **M-4 — `internal/cluster/cluster.go:14` stale comment cites "phase 02 SPEC §10 #2".** RESOLVED-OPPORTUNISTIC. Phase 04 consumes `Cluster.Dial(ctx)` from the router action and may touch `cluster.go` headers if `Cluster.Dial` documentation is updated to mention the HTTP-router consumer (the exact edit set is implementation-time judgment; the planner records). If `cluster.go` is not touched at all by phase 04, M-4 carries forward.
5. **M-5 — `internal/cluster/manager.go` error texts say "phase 02".** RESOLVED-OPPORTUNISTIC. Same posture as M-4: if phase 04 touches `manager.go` (it doesn't, per §4.2), the error texts get refreshed in the same commit; if not, M-5 carries forward.
6. **M-6 — phase-02 Minor 5 (`readyListenerAddrs` goroutine leak) deferred (confirmed).** DEFERRED-AGAIN. Phase 04 does not touch the ready-sentinel path. Carries forward.
7. **M-7 — phase-02 Minor 7 (prose `expectations.yaml`) deferred (confirmed).** DEFERRED-AGAIN. Fixture `0003-http11-routing`'s `expectations.yaml` follows the prose convention per ADR-0019. The structured-entries conversion remains a phase-06 / phase-08 candidate.
8. **M-8 — `inlineString` indent helper (fixture 0002 driver) hard-codes 22-space indent literal.** NO-ACTION FOR PHASE 04. Phase 04 fixture 0003 does not need an inline-PEM rendering at all (the fixture is plaintext). The helper lives only in fixture 0002's driver and is not a shared-helper concern. Carries forward to whatever future phase reorganises 0002 (likely the upstream-TLS-fixture follow-up).

Two of eight resolved-opportunistically (subject to phase-04 implementation-site adjacency), six explicitly deferred with rationale. No Minor rises to a phase-04 blocker.

A separate D-3.4 finding from the phase-04 brainstorming session: master `STATE.md` at commit `230fef6` listed an M-1..M-8 description that did not match the actual REVIEW.md content. The fabrication is a context-isolation slip. Phase-04 SPEC §12 (this section) cites the actual REVIEW.md `d45c467` Minor list. The next STATE.md update silently corrects.

## 13. Acceptance checklist (for the reviewer of this phase's final state)

- [ ] `internal/filter/hcm/{filter,config,route,actions,codec,connection,fuzz_test}.go` exist, build, and pass unit tests. Errors begin with `hcm: `.
- [ ] `internal/listener/manager.go` recognises the HCM type_url and dispatches to `hcm.NewFilter`. Build-time errors wrap as `listener: filter %d: hcm: ...`. Unit tests pass.
- [ ] `internal/bootstrap/bootstrap.go` blank-imports the HCM, Router, and route-config proto packages. PROGRESS records the addition (per ADR-0016 amendment policy, no new ADR required for the blank-import addition).
- [ ] `internal/filter/hcm/fuzz_test.go` `FuzzHCMConfigParse` runs clean on CI short budget. Phase-01 `FuzzBootstrapLoad`, phase-02 `FuzzTcpProxyFilter`, and phase-03 `FuzzTLSContextParse` also clean.
- [ ] `test/differential/fixture/fixture.go` carries the optional `HTTPExpectations()` extension (or, alternatively, the fixture-0003 driver internalises the comparison per §10 #4). Either way, the runner ↔ driver contract is documented in the file's package comment.
- [ ] `test/fixtures/0003-http11-routing/` contains `envoy-go.yaml`, `envoy.yaml`, `expectations.yaml`, `README.md`, `driver/driver.go`, `driver/driver_test.go`. Differential gate green.
- [ ] `test/helpers/http.go` `HTTPRoundTrip`, `test/helpers/http_diff.go` `HTTPHeaderDiff` exist and are used by the 0003 driver / runner.
- [ ] HTTP echo backend infrastructure lands either as a new helper next to the existing TCP-echo backends or as a backend-type extension to the runner. Either is SPEC-compatible.
- [ ] `BEHAVIOR_CONTRACT.md` contains a new **HTTP/1.1** subsection (ADR-K). Header allow-list table extended with the new entries (`Date` reaffirmed, `Server` reaffirmed for HCM-locally-generated, `Content-Length` + `Transfer-Encoding`, every `x-envoy-*`, every `x-forwarded-*`, `x-request-id`).
- [ ] `DECISIONS.md` contains ADRs H–O (with actual sequential numbers assigned at landing). ADR-J names the planned-divergence on `match.prefix` semantics. ADR-O does NOT name a `**Supersedes:**` (phase 02's filter-chain ADR-0033 is for network filters, not HTTP filters; phase-04 is the first HTTP-filter-chain ADR). Each ADR carries Status/Date/Doctrine plus Context/Decision/Rationale/Consequences.
- [ ] `ROADMAP.md` row for phase 04 is `status: in-progress` after the state-2 entry commit (post-spec-review), then `status: done` at the §5.3 phase-done commit.
- [ ] `STATE.md` advances per the lifecycle: drafting SPEC → state-1 candidate; SPEC reviewer-approved → state 2; PLAN drafted → state 3; …; phase done → next phase state 1.
- [ ] `go build ./...`, `go vet ./...`, `golangci-lint run`, `go test ./...` all clean (captured in PROGRESS.md per §7.5(e)).
- [ ] Commit messages follow `BOOTSTRAP_PROMPT.md` §5.3 format and reference the ADRs introduced or referenced.
- [ ] Phase-03 REVIEW carryovers triaged per §12; each Resolve-opportunistic item is either landed in code (with a PROGRESS pointer) or explicitly carried forward; each Defer item called out in PROGRESS or this SPEC.
