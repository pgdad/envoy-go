# Repository-wide code review — 2026-07-07

A comprehensive maintenance review of the whole codebase (13 parallel review
passes over every package, plus a cross-package duplication scan), followed by
implementation of the findings that could be landed safely. This work is
orthogonal to the phase-driven workflow (phase 53 PLAN was the tip at the
time); nothing under `docs/envoy-go/` or `test/fixtures/` was modified.

**Ground rule for what was implemented:** only changes that are strictly
behavior-preserving on working paths (consolidation, dead code, efficiency,
lock discipline) or that fix paths that were demonstrably broken today
(deadlocks, hangs, races, leaks, panics). Everything contractual to the
differential harness — wire bytes, stat names, admin output, boot/config error
strings, access-log formats — was preserved byte-for-byte. Anything that would
change observable behavior on a working path (even toward reference-Envoy
parity) was **deferred** and is listed at the bottom for future phases.

Verification: `gofmt` clean, `go build ./...`, `go vet ./...`,
`golangci-lint run`, and `go test ./... -race -count=1` all green. The full
Docker differential suite was NOT run as part of this pass.

## Implemented — bug fixes

- **cluster**: `health_check.interval`/`timeout` of `0s` now boot-reject
  (PGV parity wording) instead of panicking `time.NewTicker` at start;
  `Endpoint.Addr()` uses `net.JoinHostPort` (IPv6 endpoints were undialable;
  IPv4 output byte-identical).
- **wasm**: `SetTickPeriod` self-deadlock when a guest re-arms the tick from
  inside `proxy_on_tick` (generation-checked tick goroutine replacement);
  nil-instance panic window when `CallProxyOn*`/`NewStreamContext` raced
  `RootVM.Close` (instance read + closed re-check moved under `dispatchMu`);
  `DispatchHttpCall` after `Close` could leak a dispatch goroutine;
  wasm filter body/trailer paths used the listener config instead of the
  per-route override for body caps and counter scopes.
- **h2**: requests ending in trailing HEADERS were never dispatched (hung
  forever and leaked a MAX_CONCURRENT_STREAMS slot); a response racing a
  local RST_STREAM tore down the whole pooled client conn (now tolerated per
  RFC 9113 §5.1); lost-wakeup in the flow-control window with 2+ blocked
  writers.
- **lua**: `respond()` after a `:body()` yield was silently dropped (now sends
  the local reply and stops iteration); script runtime errors after an inner
  resume (`:body()`, sync `:httpCall()`) were swallowed — they now increment
  `lua.errors` and log like every other hook error.
- **extproc**: `completeStage` released `f.mu` before touching the chain,
  racing OnDestroy (now holds the lock across the disposition, matching
  extauthz); per-message deadline watchdog could kill a healthy bidi stream
  when Recv completed at the same instant the deadline expired.
- **adaptive_concurrency**: unsynchronized `sampleResetTimer` write after
  unlock (write-write race; could double the tick cadence).
- **localratelimit / compressor**: latent nil-`Stats` panics on the request
  path (both packages document nil tolerance).
- **listener/sds**: `Manager.Listeners()` read listener state without the
  lock (raced Stop from admin handlers); `sdsfile.Watcher.started` was an
  unsynchronized bool; accept-loop now backs off (5→100 ms) on persistent
  `Accept` errors instead of hot-looping; `tls_inspector`'s
  `initial_read_buffer_size` is now actually plumbed to the peek buffer
  (configs > 4096 were silently ineffective, misclassifying large
  ClientHellos).
- **bootstrap/cmd**: access-log parsing/validation now covers HCM inside
  `default_filter_chain` (was silently skipped); statsd/dogstatsd/admin
  IPv6 socket addresses now formatted with `net.JoinHostPort`.
- **statssink**: delta-mode counters were latched before the enqueue, so a
  dropped batch permanently lost increments (apply now happens in the writer
  goroutine; drops self-heal).
- **tracing**: `ExporterFor` memoized by cluster name only — a second tracing
  config on the same cluster with a different provider silently got the wrong
  exporter (key is now provider+cluster with conflict detection).
- **jwks**: `Get` had a nondeterministic select that could return `ctx.Err()`
  despite cached keys.
- **httpclient**: `ClusterDispatch` permanently rewrote the caller's
  `request.URL` (breaking request reuse).
- **network filters**: mongoproxy had no frame-length cap (2 GiB memory
  exhaustion from one declared length; now 100 MiB like kafka); BSON and RESP
  decoders had no recursion depth limit (stack-exhaustion panic from crafted
  input; now 256/128); `chain.runData` left `resumeIdx` sticky on
  terminal-less all-Continue chains (latent unbounded buffering).
- **compressor**: 4 GiB `uint32` length wrap in the min-content-length gate;
  Content-Length parse widened to 64-bit.
- **filter/http per-route framework**: boot-error location strings used the
  scope index for both vhost and route coordinates — real
  `virtual_hosts[i].routes[j]` indices now reported.

## Implemented — consolidation (behavior-preserving)

- cluster: single `dialPicked` path for `AcquireH1` (was a verbatim fork);
  shared `panicGate`/`nextAvailable` health-gate helpers across the five leaf
  LBs (pick sequences pinned bit-identical by tests); one `bumpStreak`
  consecutive-outlier detector (was triplicated); `checkerSpec` embedded.
- wasm: ten `CallProxyOn*` methods → one `dispatchGuest` template (error
  strings byte-identical); registry factory now runs outside the process-wide
  lock (per-key pending sentry).
- lua: decode/encode body paths unified behind a `bodySide` selector (six
  near-identical function pairs → three shared implementations); shared
  list-shape detection for table→Struct/Value marshaling; response-side log
  stubs and streamInfo accessors deduplicated; pairs shim precompiled once
  per process instead of parsed per request.
- hcm/router: deleted the entire production-unreachable legacy `do()`
  direct-write path (tests ported to the live Action-closure path);
  `clusterRouteAction` memoizes its per-request closures; one TLS
  `ConnectionState()` snapshot per H1 request (was three).
- httpclient: one shared retry loop + transport-clone helper for
  `Do`/`ClusterDispatch`.
- grpcclient: seven client wrappers share an embedded `connHolder` and
  `dialConn` (all error strings byte-identical).
- network: shared `NewBytes` chain-buffer helper and a `statroster` package
  for the five protocol proxies' counter rosters; mongo/zk framing pairs
  collapsed to kafka's pointer-parameter shape; write-dispatch reversal built
  once per connection.
- observability: statsd/dogstatsd share a `udpWriter` + line-emit helper;
  stats registry constructor pairs factored; `stats.NamePattern` is the single
  name-regex definition; `orDash`/`orEmptyDash` merged; access-log escaper
  built once.
- ratelimit family: generic `PerRouteCache` in `internal/filter/http` adopted
  by localratelimit, bandwidthlimit, and buffer; localratelimit's duplicated
  listener/per-route validator merged; paired stat constructors factored in
  two packages; admission_control adopted `internal/clock` (deleted its local
  clock + fake).
- oauth2: HMAC composition lives in one function; the triplicated 302
  auth-challenge wire emission extracted to one helper.
- tls/listener: shared trusted-CA pool loader; shared cert-principal walk.
- bootstrap: statsd/dogstatsd sink parsers share the address/prefix helper;
  hard-coded TypeURLs now descriptor-derived (equality with the old literals
  pinned by a test).

## Implemented — efficiency (no observable change)

- cluster: endpoint addr string cached at extraction; single map lookup per
  availability check (was two lookups + two `fmt.Sprintf` per endpoint per
  Pick).
- rbac: SafeRegex and CIDR matchers compiled once at build time (was
  `regexp.Compile` + `net.ParseCIDR` per request); ratelimit SafeRegex
  matchers cached (`sync.Map`); matcher `contains`/`ignore_case` needle
  lowered once at compile.
- compressor: pooled `gzip.Writer` per config level (output byte-identity
  pinned by tests); ETag regexes → direct string checks.
- fault: per-request RNG allocated lazily (0%/100% rolls never allocate) and
  seeded uniquely for same-nanosecond requests.
- bandwidthlimit: full body copies replaced with length counters.
- extauthz: HTTP-mode allow path drains the response body (keep-alive reuse).
- adaptive_concurrency: in-place quantile for controller sampling;
  admission_control: one lock/purge per request (was two).
- thrift: one allocation per frame (was two + full copy); accesslog/tracing
  sinks skip `proto.Size` when no size trigger is configured.

## Implemented — dead code removed

Legacy `do()` router path, `byteCounterWriter`, `Filter.Status()`,
`chainDispatchAction.status`, lua `BodyBuffer` wrappers + `Chunk.hash`,
wasm `pendingHttpCall.deadline` + import guards + 19 stale `//nolint:unused`,
extproc `processorClient` interface + `forwardRules` placeholder +
`lowercaseHeaderMap`, extauthz `buildTargetURL`, grpcclient `target` fields,
cluster `hostHealth.ejectCount`, tracing `IncSent`/`IncDropped` (×4),
jwtauthn extraction tuples + `sourceKind`, ratelimit `callCtx`, unused `ecb`
fields (fault, header_mutation, envoygotest, localratelimit),
`tls.MatchServerName`, `drain.StateDrained`, `internal/tcp` placeholder
package, unreachable `context.Canceled` branches in tls_inspector,
oauth2/redis/zookeeper/kafka/mongo write-only callback fields.

## Deferred — needs differential/reference verification before landing

These are real findings, deliberately NOT implemented because they change
observable behavior on working paths (wire bytes, stat values, config
acceptance) and must be pinned against reference Envoy with new differential
fixtures first. Grouped by area; see git history of this file for details.

- **extproc**: mid-stream transport errors bypass `failure_mode_allow=false`
  (unconditional fail-open, no 500/counter); `HeaderMap` sent to the
  processor in nondeterministic order.
- **extauthz**: gRPC CheckRequest always reports method POST; pseudo-headers
  stripped so `Host`/`Scheme` are empty (contradicts the §11.P4 contract);
  `x-envoy-auth-partial-body` never injected.
- **oauth2** (flow is non-functional end-to-end today): issued-cookie HMAC
  computed with empty domain so post-callback validation always fails
  (infinite 302 loop); refresh-success leg ships placeholder empty cookies
  and never emits them (`Encoder: nil`); auth challenge omits all RFC 6749
  query params (client_id, redirect_uri, state…); state cookie is a bare
  guessable timestamp compared non-constant-time (login-CSRF weakness);
  state cookie name collides with `OauthExpires`.
- **cluster**: `maximum_ring_size` parsed but never applied (ring can exceed
  it; `ring_hash_lb.size` diverges); `lb_healthy_panic` double-incremented
  under locality-weighted panic; `membership_healthy` ignores outlier
  ejections; sequential health probing stretches effective interval for
  large clusters; ring/maglev gauges lost when the LB is wrapped
  (locality/priority wraps).
- **listener**: `listener_filters_timeout` never enforced (silent client
  hangs a goroutine+fd forever); `continue_on_listener_filters_timeout`
  wrongly gates non-timeout errors; SNI chain match is case-sensitive
  (reference is case-insensitive).
- **hcm/h2**: `closedStreams` map grows unboundedly on long-lived downstream
  conns (needs a watermark that keeps h2spec-verified §5.1 discrimination);
  H1 encode-error path emits no access log (H2 does); H1 mid-body client
  abort proxies a truncated body upstream with the original Content-Length;
  idle H2 conns wake 20×/s from the 50 ms readFrame poll.
- **matcher/string-matching**: SafeRegex is unanchored substring match where
  Envoy uses full-match — coordinate one fix across matcher, rbac, oauth2,
  extauthz, ratelimit (plus ASCII-only case folding vs `EqualFold`); a shared
  `stringmatcher` package is the natural vehicle; cors supports only 3 of 5
  StringMatcher arms.
- **fault/header_mutation**: exact header match compares only the first value
  of repeated headers (reference joins with `,`); ADD_IF_ABSENT /
  OVERWRITE_IF_EXISTS conflate absent with present-but-empty.
- **wasm**: present-but-empty header returns NotFound to the guest; outbound
  `proxy_http_call` bodies go chunked with no Content-Length (`byteReader`
  defeats net/http's sizing); pairs codec duplicated across wasm/abi
  packages; full wazero compile paid before the VM-registry probe.
- **network**: redis `op_timeout` parsed but never enforced (hung upstream
  wedges the pump); thrift oneway messages block awaiting a reply that never
  comes; mongo post-handoff drain-close is a no-op; tcpproxy close-direction
  attribution keyed off which pump returns first.
- **validate/bootstrap**: `--mode validate` skips admin-socket and
  gRPC-sink cluster checks that boot enforces (ADR-0268 divergence);
  `stats_flush_interval` accepted outside reference PGV bounds.
- **jwtauthn**: `jwks_fetch_success` counted per configured provider, not per
  successful initial fetch (async fast_listener).
- **misc consolidations parked**: generic batch-exporter engine (4 verbatim
  copies), generic frozen typeURL registry (3 copies), DataSource resolver
  package (5 variants with contractual per-package error wording), factory
  typed_config preamble helper, extproc/extauthz grpc_service validation
  dedupe, ratelimit OVER_LIMIT double header build.
- **listener peekerConn wrap is load-bearing — do not skip it**: an attempted
  "skip the per-connection peekerConn when the listener has zero
  listener_filters" optimization was implemented and then reverted during this
  pass: the wrapper's *absence* of `SetLinger` is what keeps the
  `serveNetworkChain` NoFlush SO_LINGER-0 (RST) branch dormant, and fixture
  0043-network-rbac pins the resulting plain-FIN deny close
  (`verdict=closed_no_bytes`). Skipping the wrap let a raw `*net.TCPConn`
  reach that branch and turned deny closes into RSTs. Any future retry must
  also neutralize the SetLinger branch (and re-verify 0043 plus reference
  behavior).
