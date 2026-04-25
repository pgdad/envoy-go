# Phase 05 — HTTP/2 (downstream + upstream, low-level framer, own conn mgr)

**Phase id:** `05`
**Slug:** `05-http-2`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:brainstorming` (adapted autonomous mode — see `DECISIONS.md` ADR-0004)
**Depends on:** phase 04 (done)
**Differential surface at end of phase:** pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, and `0003-http11-routing` remain green with no behavioural regression; new fixture `0004-h2-routing` green, exercising downstream HTTP/2 over TLS (ALPN `h2`) into the existing HCM, route match (`prefix` + `path`, unchanged from phase 04), the router HTTP filter dispatching to a STATIC cluster of upstream HTTP/2-over-TLS backends, and `direct_response` — status equivalence per request, decoded body byte-equivalence on `direct_response` 2xx paths, set-equal response headers (modulo the existing phase-04 allow-list extended for HTTP/2), per-cluster RR distribution `[3, 3, 3]` over the routed-to-upstream subset, against upstream Envoy v1.37.2. Additionally, the project's first conformance suite — `h2spec` — runs against the subject's HTTP/2 listener and passes at the threshold declared in `BEHAVIOR_CONTRACT.md`'s new `## HTTP/2` subsection.

---

## 1. Purpose

Phase 05 lands envoy-go's first HTTP/2 dataplane on both ends of the proxy: the listener accepts TLS connections that negotiate ALPN `h2`, the HCM dispatches to a from-scratch HTTP/2 codec driving `golang.org/x/net/http2.Framer` + `golang.org/x/net/http2/hpack` as low-level codec surfaces (per doctrine `D-3.2`), the per-connection state machine demuxes HEADERS/DATA/RST_STREAM/SETTINGS/PING/WINDOW_UPDATE/GOAWAY into per-stream request/response scopes, each stream runs the same degenerate HTTP-filter chain that phase 04 introduced (exactly `[router]`, per ADR-0042), and the router action either short-circuits with a `direct_response` or dials a fresh upstream H2-over-TLS connection per request via a new `Cluster.DialH2(ctx)` helper that mirrors phase-04's per-request fresh-dial discipline (per ADR-0039) extended with ALPN-driven codec confirmation.

Concretely, phase 05 produces:

1. An HTTP/2 codec under `internal/filter/hcm/h2/` (sub-package of the phase-04 HCM package) that owns one downstream H2 connection at a time. It performs the connection preface check (`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n` per RFC 9113 §3.4), reads/writes frames via `http2.Framer`, runs SETTINGS handshake (server-initial SETTINGS + client-initial SETTINGS ack), maintains per-connection send/recv flow-control windows, demuxes incoming frames into per-stream state machines, runs HEADERS encode/decode through `hpack.Encoder`/`hpack.Decoder`, and emits GOAWAY on graceful shutdown or unrecoverable connection-level errors.
2. A from-scratch per-stream state machine (`stream.go`) implementing the RFC 9113 §5.1 Idle → Open → Half-closed (remote/local) → Closed lifecycle on the server side. Each stream's request scope (request headers, request body reader, response writer, request trailers reader, response trailers writer) is constructed on receipt of HEADERS, dispatched through the route table on END_STREAM (or before, on streamed bodies — phase 05 keeps the simpler "wait for END_STREAM before dispatching" form per §10.1; full streaming-body filter dispatch lands with the phase-07 framework), and torn down on `closed`.
3. An ALPN-driven codec dispatcher under `internal/filter/hcm/filter.go`: `Filter.Handle(ctx, downstream)` inspects `downstream.(*stdtls.Conn).ConnectionState().NegotiatedProtocol` after the TLS handshake completes (the listener's pre-existing TLS handshake from phase 03 is unchanged); if `"h2"`, dispatches to the new H2 connection driver; if `"http/1.1"` or empty, dispatches to the phase-04 H1 connection driver unchanged. Plaintext (non-TLS) listeners continue to dispatch to the H1 driver — phase 05 does NOT introduce h2c (HTTP/2 over plaintext, a.k.a. "prior knowledge") on the differential gate; h2spec is exercised over h2c on a dedicated test-only listener but no fixture asserts h2c equivalence (per §2 / ADR-W).
4. An upstream H2 dial helper under `internal/cluster/dial_h2.go`: `Cluster.DialH2(ctx) (*h2.ClientConn, error)`. Builds on the existing phase-03 `Cluster.Dial` (which returns a `*stdtls.Conn` post-handshake when `transport_socket: tls` is configured); confirms `NegotiatedProtocol == "h2"` on the returned conn (errors with `cluster: dial h2: alpn negotiated %q, want "h2"` otherwise); wraps the conn in a from-scratch `h2.ClientConn` (phase-05's own client-side H2 connection manager — NOT `http2.Transport`, NOT `http2.NewClientConn`, both of which are server/transport runtimes forbidden by D-3.2). The router action consumes this via a new `routerActionH2` variant.
5. A small extension to `internal/filter/hcm/config.go`: `codec_type: HTTP2` is now permitted alongside phase-04's `HTTP1` and `AUTO`. `AUTO` continues to mean "the listener picks via ALPN" — i.e. dispatches per-conn at runtime. `HTTP2` means "this listener accepts only h2"; if a downstream conn arrives with ALPN `http/1.1` (or empty / plaintext), HCM closes it via `writeStatusReply` for plaintext-fallback isn't possible, so the H1 driver writes a 400 line and closes (justification: a `codec_type: HTTP2` listener that's reached by a non-h2 client is a misconfiguration). `HTTP3` continues to error at build (→ later phase). The `http2_protocol_options` field on the HCM is silently ignored at phase 05 (added to the phase-04 ignored-set per §9 and ADR-N's amendment).
6. A small extension to `internal/cluster/manager.go` (or its config builder): the cluster's `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]` is parsed for the `explicit_http_config.http2_protocol_options` discriminator. Presence of `http2_protocol_options` switches the cluster's outbound codec to h2 (and validates that the cluster's `transport_socket` is TLS with `alpn_protocols` containing `"h2"`); absence keeps phase-04 H1 semantics. `auto_config`, `connection_pool_per_downstream_connection`, all `common_http_protocol_options` fields, and the rest of `HttpProtocolOptions` are silently ignored at phase 05.
7. A new differential fixture `test/fixtures/0004-h2-routing/`: one TLS listener with ALPN `h2, http/1.1` and one HCM filter with `codec_type: AUTO` (so ALPN drives the per-conn codec choice on both proxies); one route_config with one `*` vhost holding three routes mirroring fixture 0003 (`path: /health` → `direct_response 200 OK\n`; `prefix: /api` → cluster `c_h2_backend`; otherwise → `direct_response 404 not found\n`); one STATIC cluster `c_h2_backend` with three TLS upstream endpoints, each an h2-capable test backend serving a per-backend echo body; driver issues 27 requests via `golang.org/x/net/http2.Transport` (driver-side use of the h2 transport is permitted because the driver is fixture infrastructure, not envoy-go runtime — D-3.2 governs runtime, not test code); asserts status equivalence per request, decoded body equivalence on the 9 `/health` direct-response requests, per-cluster RR distribution `[3, 3, 3]` on each side over the 9 `/api` requests, and 404 status equivalence on the 9 `/missing` requests. Mirrors fixture-0003's request shape so the regression is mechanical to compare.
8. The first conformance suite under `test/conformance/h2spec/`: a Go test driver that boots a phase-05 envoy-go subject with a single h2c listener (TLS-less, dedicated to conformance), runs the upstream `summerwind/h2spec` Docker image against it via `testcontainers-go`, parses the structured output, and asserts `failed == 0` over the SECTION SET declared in `BEHAVIOR_CONTRACT.md`'s new `## HTTP/2` subsection (initial sections: 3 HTTP Frame Format, 4 HPACK, 5 Streams and Multiplexing, 6 Frame Definitions excluding 6.6 PUSH_PROMISE, 7 Error Codes, 8 HTTP Message Exchanges; explicit exclusions documented per ADR-U). The conformance binary is pinned by Docker tag + SHA in `ENVOY_TARGET.md`'s sibling pin file (`docs/envoy-go/CONFORMANCE_PINS.md` — new in phase 05; sibling to `ENVOY_TARGET.md`, same refresh discipline per D-3.7). Per `BOOTSTRAP_PROMPT.md` §7.5 (c) this gate is now non-vacuous for the first time.
9. A new `BEHAVIOR_CONTRACT.md` subsection, **HTTP/2**, codifying: the structurally-equivalent (NOT byte-equivalent) framing rule from §7.2 of `BOOTSTRAP_PROMPT.md`; the per-stream request/response equivalence inheriting from the phase-04 HTTP/1.1 subsection (status, decoded body for direct_response, header set-equality modulo allow-list, route-match selection equivalence); the new HTTP/2-specific allow-list rules (`x-envoy-*`, `x-forwarded-*`, `x-request-id` continue to be presence-not-required on the subject side; HTTP/2 pseudo-headers `:status`, `:method`, `:path`, `:scheme`, `:authority` are required and asserted by name + value on both sides); h2spec's threshold list and exclusions; the upstream-h2-TLS scope which closes ADR-0035's gap.
10. New fuzz targets: `internal/filter/hcm/h2.FuzzFrameStream` over the connection-manager ingest (mutates a sequence of well-formed frames into malformed ones, asserts no panic and that all returned errors begin with `h2:`), and `internal/filter/hcm/h2.FuzzHPACKDecode` over the hpack decoder integration (asserts no panic on adversarial header blocks; the underlying `x/net/http2/hpack` package has its own fuzzer upstream, but a wrapper-level fuzz target catches integration regressions in our usage). Short-budget `-fuzztime=30s` per ADR-0018.

After phase 05, the project has proven its sixth central engineering claim: *envoy-go speaks HTTP/2 on both downstream and upstream surfaces — it negotiates ALPN, drives an own framer over TLS, demuxes streams through an own connection-manager state machine, and produces structurally-equivalent framing and per-stream behaviourally-equivalent responses to upstream Envoy on a deterministic workload, while passing the declared subset of `h2spec` conformance.* Every subsequent phase (observability in phase 06, filter-chain framework in phase 07, admin/drain in phase 08) builds on a proxy that now speaks two HTTP versions through one HCM.

## 2. Non-purposes

Phase 05 does **not** do any of the following. Each is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

- **HTTP/3 / QUIC.** Both HCM `codec_type: HTTP3` and any QUIC transport socket continue to error at build, unchanged from phase 04. → HTTP/3 + QUIC family.
- **HTTP/2 server push (`PUSH_PROMISE`).** Phase 05's H2 server NEVER emits `PUSH_PROMISE`; on receipt of a `PUSH_PROMISE` (clients can't legally send these — only servers — so this is a protocol-error case), the connection emits GOAWAY with `PROTOCOL_ERROR`. The server's SETTINGS handshake advertises `SETTINGS_ENABLE_PUSH = 0` to disable push from the client side as well. h2spec section 6.6 (PUSH_PROMISE) is excluded from the conformance threshold per ADR-U. → HTTP/2 family extensions, if/when ever (Envoy itself doesn't push; this is unlikely to ever land).
- **HTTP/2 PRIORITY-driven scheduling.** RFC 9113 (the H2 update from RFC 7540) deprecated PRIORITY frames and the priority dependency tree. Phase 05 reads PRIORITY frames (must, per RFC 9113 §6.3) and silently discards them — i.e., does not adjust stream scheduling. The advertised `SETTINGS_NO_RFC7540_PRIORITIES = 1` informs clients of this. → out of scope permanently unless a workload demands it.
- **Adaptive flow-control / BDP estimation.** Phase 05's flow-control implements the RFC 9113 §5.2 baseline only: hard-coded initial windows, WINDOW_UPDATE on consumption, no dynamic resizing of `SETTINGS_INITIAL_WINDOW_SIZE` after the handshake. Initial windows: connection-level 65535 (the protocol-default), per-stream 65535 (the protocol-default; phase 05 does NOT advertise a larger window — adaptive sizing is a tail-latency optimisation that belongs to a perf-tuning phase). → upstream-robustness family or a perf phase.
- **Upstream H2 stream pooling / multiplexing across requests.** Phase-05's `Cluster.DialH2` returns a *fresh* `h2.ClientConn` per upstream request — analogous to phase-04 ADR-0039's per-request fresh H1 conn. Within a single `routerActionH2.do`, exactly one stream is opened on the new conn and the conn is closed immediately after the response is consumed. The multiplexing benefit of H2 is intentionally unrealised on the upstream side at phase 05; pooling lands with the upstream-robustness family (which also covers H1 pooling). The phase-05 differential surface does not assert pool/non-pool — see ADR-R. → upstream-robustness family.
- **Trailer support (request and response).** Phase 04 set `req.Trailer = nil` after `http.ReadRequest` (stdlib H1 limitation). Phase 05's H2 codec, in contrast, *can* observe trailers (HEADERS frames after DATA with END_STREAM set) but the phase-05 router action does NOT forward them: trailers received from the downstream are discarded; trailers received from the upstream are discarded. The router emits END_STREAM on the response HEADERS or final DATA, never via a trailing HEADERS frame. The fixture driver does not exercise trailers. ADR-X carries the rationale and the deferral. → phase 07 framework + gRPC-family phases (where trailers carry `grpc-status`).
- **gRPC-specific behaviour.** No `grpc-status` translation, no gRPC-Web bridging, no `grpc-timeout` honouring. The phase-05 differential surface is plain H2 routing only. → gRPC family.
- **0-RTT (TLS 1.3 early data).** crypto/tls supports 0-RTT only via TLS sessions and explicit opt-in; phase-05 does not opt in. Server emits `Server: envoy` on locally-generated responses (per ADR-0014); 0-RTT replay-protection logic is out of scope. → later phase if ever.
- **HTTP/1.1 → HTTP/2 upgrade ("h2c upgrade", RFC 7540 §3.2 / RFC 9113 deprecated this — clients use prior knowledge or ALPN now).** Phase 04's HCM rejects `Upgrade: h2c` request headers with 501 (per phase-04 SPEC §4.1's connection-loop guard). Phase 05 does NOT change that. The h2c-prior-knowledge surface (no Upgrade, the client just sends the preface bytes after a plaintext TCP connect) IS used by `h2spec` against a dedicated conformance listener (see §4.3) but is NOT exposed via any production fixture. → out of scope permanently unless a workload demands it; the h2c-conformance listener is a test-only construct.
- **HCM `tracing`, `access_log[]`, `http_protocol_options` (the deprecated direct field — distinct from the typed-config `HttpProtocolOptions` extension), `common_http_protocol_options`, `server_header_transformation`, `local_reply_config`, `internal_redirect_policy`, `request_id_extension`, `path_with_escaped_slashes_action`, `merge_slashes`, `xff_num_trusted_hops`, `via`, `proxy_100_continue`, `stream_idle_timeout`, `request_timeout`, `request_headers_timeout`, `drain_timeout`, `delayed_close_timeout`, `forward_client_cert_details`, `original_ip_detection_extensions`.** All silently ignored, unchanged from phase 04 (per ADR-N). The new HCM field newly silently-ignored in phase 05: `http2_protocol_options` (the directly-on-HCM field, not the cluster-side typed-extension). Recorded in §9 and ADR-N's phase-05 amendment.
- **Upstream cluster `HttpProtocolOptions` fields beyond the H2 codec switch.** `common_http_protocol_options` (timeouts, headers-with-underscores action, max-headers-count, max-stream-duration), `auto_config`, `upstream_http_filters`, `connection_pool_per_downstream_connection`: all silently ignored. Only `explicit_http_config.http2_protocol_options` is *read*, and only its presence/absence is honoured (the inner `http2_protocol_options.{initial_stream_window_size, initial_connection_window_size, max_concurrent_streams, hpack_table_size, allow_metadata, ...}` fields are silently ignored — phase 05 advertises hardcoded defaults and does not adjust per-cluster). → upstream-robustness family.
- **Stats, access logs, tracing, runtime overrides.** All deferred. → phase 06 / observability family.
- **HTTP filters other than `[router]`.** Unchanged from phase 04 (per ADR-0042). → phase 07.
- **Per-route filter config.** Unchanged from phase 04. → phase 07.
- **Route match predicates beyond `prefix` and `path`.** Unchanged from phase 04 (per ADR-0038). → phase 07.
- **Multi-vhost matching.** Unchanged from phase 04. → phase 07.
- **Route action types beyond `direct_response` and `route`.** Unchanged from phase 04. → phase 07 + load-balancing family.
- **`direct_response.body` shapes beyond `inline_string`.** Unchanged. → phase 07.
- **`route.route.cluster` shapes beyond a string cluster name.** Unchanged. → load-balancing family.
- **Filter-chain matching beyond ALPN-derived codec selection.** Phase 03's SNI-keyed filter-chain match plus phase 04's empty-match plaintext rule continue to apply. ALPN is NOT a `filter_chain_match` field at phase 05 — codec selection happens *inside* the HCM filter, not at the listener-side filter-chain match step. The `filter_chain_match.application_protocols[]` field is silently ignored if present (extending the phase-04 ignored set). → phase 07's filter-chain framework, which may upgrade `application_protocols` to a chain-match dimension.
- **Cluster types other than STATIC (subject side).** Unchanged from phase 02. → later phase.
- **LB policies other than ROUND_ROBIN.** Unchanged. → load-balancing family.
- **Graceful drain of in-flight HTTP/2 requests.** SIGINT behaviour unchanged from phase 04: listener sockets close, in-flight conns drop. The H2 connection manager does NOT emit a graceful GOAWAY with `last-stream-id` followed by an idle drain on shutdown; it just closes. → phase 08 (admin-api-and-drain).
- **HTTP/2 stream priority observed in any way.** RFC 9113 deprecated; phase 05 advertises `SETTINGS_NO_RFC7540_PRIORITIES = 1`, accepts incoming PRIORITY frames per RFC, and does nothing with them. → out of scope.
- **HTTP/2 max-concurrent-streams enforcement.** Phase 05 advertises `SETTINGS_MAX_CONCURRENT_STREAMS = 100` (Envoy's documented default) on the server side. If a client opens more than 100 streams concurrently, the server emits `RST_STREAM` with `REFUSED_STREAM` on the excess streams (RFC 9113 §5.1.2 mandate). The fixture driver issues sequential streams (one at a time) so this code path is exercised only by the unit tests, not by the differential fixture or h2spec.

## 3. Phase-done gates (specialization of §7.5)

Per doctrine `D-3.6`, phase 05 lands only when every gate below is green. The generic `BOOTSTRAP_PROMPT.md` §7.5 gate set is narrowed:

| Gate | Specialization for phase 05 |
|---|---|
| (a) new/changed differential fixtures green | New fixture `test/fixtures/0004-h2-routing/` passes: 27 HTTP/2 requests per proxy (9 `/health` + 9 `/api/v1/<n>` + 9 `/missing/<n>`); `:status` equivalence per request on both sides; decoded body equivalence on the 9 `/health` direct-response requests (200 + body `OK\n`); 404 status equivalence on the 9 `/missing` requests (body relaxed under the phase-04 framing-divergence rule extended for H2 local-reply prose); per-cluster RR distribution `[3, 3, 3]` on EACH SIDE over the 9 `/api` requests (9 sequential streams; 3 per backend per side; same local-correctness property as fixtures 0001/0002/0003 per ADR-0024). Driver uses `golang.org/x/net/http2.Transport` over TLS with `InsecureSkipVerify: false` and the fixture's pinned PKI (carry-forward of the fixture-0002 PKI generation pattern). |
| (b) all pre-existing differential fixtures still green | `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing` all pass without regression under their existing `expectations.yaml`. The phase-04 HCM H1 path is NOT touched by phase 05's changes (H1 driver remains intact; phase-05 changes are additive — the `Filter.Handle` ALPN dispatch falls through to the H1 driver on non-h2 connections). |
| (c) conformance suites pass | `test/conformance/h2spec/` runs the upstream `summerwind/h2spec` image (pinned in `docs/envoy-go/CONFORMANCE_PINS.md`, new in phase 05) against a dedicated phase-05 h2c listener and reports `failed == 0` over the threshold section list (3, 4, 5, 6 ex-6.6, 7, 8); see ADR-U for the exact section list and exclusions. THIS GATE IS NEWLY NON-VACUOUS — it was vacuously green in phases 00–04. |
| (d) new fuzzer runs clean for CI short-budget | New fuzz targets `internal/filter/hcm/h2.FuzzFrameStream` and `internal/filter/hcm/h2.FuzzHPACKDecode` run clean for their CI short-budget runs (30-second policy inherited from ADR-0018). Phase-01 `internal/bootstrap.FuzzBootstrapLoad`, phase-02 `internal/filter/tcpproxy.FuzzTcpProxyFilter`, phase-03 `internal/tls.FuzzTLSContextParse`, and phase-04 `internal/filter/hcm.FuzzHCMConfigParse` also run clean (no regression). |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | Standard. Unit tests for `internal/filter/hcm/h2/` (codec, framer, conn manager, stream state machine, hpack roundtrip, settings handshake, flow control, error codes, GOAWAY, RST_STREAM, PING) plus extended tests for `internal/filter/hcm/` (ALPN dispatch, codec_type=HTTP2 build) plus extended tests for `internal/cluster/` (DialH2, ALPN confirmation, h2 conn factory) plus extended tests for `cmd/envoy-go/main_test.go` (H2 bootstrap end-to-end smoke) all part of `go test ./...`. |
| (f) `REVIEW.md` approved | Per `SKILL_ROUTING.md` state 5. |

## 4. Deliverables (files and directories)

Grouped by lifecycle. Every path below is either new or materially changed.

### 4.1 New production code

- **`internal/filter/hcm/h2/conn.go`** — exposes `ServerConn` (per-downstream-conn server-side H2 connection manager; one per downstream `*stdtls.Conn` after ALPN selects `h2`). `NewServerConn(ctx, downstream net.Conn, table *hcm.RouteTable, settings ServerSettings) *ServerConn`. `(*ServerConn).Run() error` — the connection loop: writes server preface (server-initial SETTINGS), reads client preface (`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n` then client-initial SETTINGS), processes incoming frames, dispatches HEADERS-on-new-stream into a fresh per-stream goroutine that runs the route table → action sequence. Returns `nil` on clean shutdown (downstream closed gracefully or GOAWAY exchanged), an `h2:`-prefixed error on protocol violations (the caller — `Filter.Handle` — drops the error per the phase-02 `_ = io.Copy` precedent). The `ServerSettings` struct carries the hardcoded defaults (`MaxConcurrentStreams=100`, `InitialWindowSize=65535`, `MaxFrameSize=16384`, `EnablePush=0`, `NoRFC7540Priorities=1`, `HeaderTableSize=4096`); phase 05 does not vary these per-config, but the struct exists so phase 06+ tests can.
- **`internal/filter/hcm/h2/stream.go`** — per-stream state machine. `serverStream` carries: stream ID, current state (idle/open/halfClosedRemote/halfClosedLocal/closed), per-stream send/recv windows, decoded request headers, request body pipe (an `io.Pipe` writer fed by DATA frames; the route-table action consumes the reader end), END_STREAM flags. Methods: `recvHeaders([]hpack.HeaderField, endStream bool) error`, `recvData([]byte, endStream bool) error`, `recvRSTStream(errCode) error`, `recvWindowUpdate(increment uint32) error`, `dispatch(ctx context.Context, table *hcm.RouteTable) error` (called once on END_STREAM-on-headers OR END_STREAM-on-data; runs the route match + action; the action's response is written back via `sendHeaders`/`sendData`/`sendRSTStream`).
- **`internal/filter/hcm/h2/framer.go`** — thin wrapper over `http2.Framer` adding context-cancellation on the read side (`http2.Framer.ReadFrame` is blocking and not ctx-aware; the wrapper sets a deadline derived from the ctx and translates `os.IsTimeout`+`ctx.Err() != nil` into a clean cancellation). All write methods are passthrough. Phase 05 does NOT use `http2.Framer`'s `WriteRawFrame`; only the high-level write methods (`WriteSettings`, `WriteSettingsAck`, `WriteHeaders`, `WriteData`, `WriteRSTStream`, `WriteWindowUpdate`, `WritePing`, `WriteGoAway`).
- **`internal/filter/hcm/h2/hpack.go`** — pair of `hpack.Encoder` and `hpack.Decoder` with the conn-level state. The encoder writes into a per-frame buffer; the decoder feeds emitted fields into a slice that the stream state machine consumes. Header table size is fixed at 4096 octets (RFC 9113 default + Envoy default; the `SETTINGS_HEADER_TABLE_SIZE` setting we receive from the client is honoured by passing it to the encoder side so our outgoing header tables shrink/grow as the client requests; the decoder side is fixed at 4096 because we don't advertise a different value).
- **`internal/filter/hcm/h2/flow.go`** — flow-control helpers. Connection-level send window (decrements on DATA send, blocks when ≤ 0 until WINDOW_UPDATE arrives), per-stream send window (likewise), connection-level recv window (decrements on DATA recv, increments + WINDOW_UPDATE emit on consumption), per-stream recv window (likewise). Implemented with channels + a small mutex; phase 05 keeps it minimal and correct, not optimised.
- **`internal/filter/hcm/h2/preface.go`** — server-side preface check: read 24 bytes, compare against `[]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")`. Mismatch → connection-level error.
- **`internal/filter/hcm/h2/settings.go`** — settings handshake helpers. `writeServerInitialSettings(fr *http2.Framer, s ServerSettings) error` writes a SETTINGS frame with the phase-05 default settings; `readClientSettings(fr *http2.Framer, applyTo *clientSettings) error` reads one SETTINGS frame from the client and applies the values; the conn driver issues SETTINGS_ACK on receipt of client SETTINGS and expects a SETTINGS_ACK in response to its own.
- **`internal/filter/hcm/h2/errors.go`** — small enum of H2 error codes (`NO_ERROR`, `PROTOCOL_ERROR`, `INTERNAL_ERROR`, `FLOW_CONTROL_ERROR`, `STREAM_CLOSED`, `FRAME_SIZE_ERROR`, `REFUSED_STREAM`, `CANCEL`, `COMPRESSION_ERROR`, `CONNECT_ERROR`, `ENHANCE_YOUR_CALM`, `INADEQUATE_SECURITY`, `HTTP_1_1_REQUIRED`) mapped to RFC 9113 §7 numeric codes. All errors returned by H2 internals are `*h2.Error{Code, Stream, Underlying}` so the top-level connection driver can dispatch RST_STREAM (stream-scoped) vs GOAWAY (conn-scoped).
- **`internal/filter/hcm/h2/client.go`** — server-side, but used by the upstream router action: `ClientConn` is the from-scratch H2 client conn manager. `NewClientConn(ctx, upstream net.Conn) (*ClientConn, error)` writes the client preface, exchanges initial SETTINGS, returns a usable conn. `(*ClientConn).RoundTrip(ctx, req H2Request) (H2Response, error)` opens a single stream, writes HEADERS+DATA+END_STREAM, reads the response HEADERS+DATA+END_STREAM, returns the response. Phase 05 uses this for ONE round-trip per `ClientConn` instance (per-request fresh conn — see ADR-R); the conn supports multiple round-trips in principle but the router does not exploit it.
- **`internal/filter/hcm/h2/h2_test.go`** (or split into `conn_test.go`, `stream_test.go`, …) — exhaustive unit tests covering: server preface check (good + bad); SETTINGS handshake (server initial + ack + client initial + ack ordering); HEADERS encode + decode roundtrip (request and response); DATA framing (single + multi + END_STREAM); RST_STREAM (server-side emit on stream-scoped error; client-side recv); WINDOW_UPDATE (recv increments send window; recv DATA decrements recv window; emit WINDOW_UPDATE on consumption); per-stream state machine transitions (idle→open→half-closed-remote→closed and idle→open→half-closed-local→closed); GOAWAY emit on graceful shutdown and on protocol error; PING + PING-ACK; MAX_CONCURRENT_STREAMS enforcement (101st concurrent stream → REFUSED_STREAM); flow-control correctness (small windows + multiple DATA frames + WINDOW_UPDATE → eventual full delivery); HPACK dynamic table size update propagation; PUSH_PROMISE on the server-receive path → GOAWAY/PROTOCOL_ERROR; PRIORITY frame received → silently discarded (no state change). Stdlib `golang.org/x/net/http2.Transport` is used as the test peer for client-side scenarios (driver-side use OK; runtime use forbidden).
- **`internal/filter/hcm/h2/fuzz_test.go`** — `FuzzFrameStream` (mutates a corpus of well-formed frame sequences and asserts no panic + all errors begin with `h2:`); `FuzzHPACKDecode` (adversarial header-block bytes through the conn-level decoder).
- **`internal/cluster/dial_h2.go`** — `Cluster.DialH2(ctx) (*h2.ClientConn, error)`. Calls `Cluster.Dial(ctx)`; type-asserts the returned conn to `*stdtls.Conn` (errors `cluster: dial h2: not a TLS conn` if not — H2 over plaintext is out of scope per §2); inspects `ConnectionState().NegotiatedProtocol` (errors `cluster: dial h2: alpn negotiated %q, want "h2"` if not h2); wraps via `h2.NewClientConn(ctx, raw)`; returns. The cluster's per-cluster H2 enablement (driven by `explicit_http_config.http2_protocol_options` per §1 #6) is a *build-time* flag on the cluster; runtime callers select `Dial` (H1) vs `DialH2` (H2) based on which router action variant invokes them.
- **`internal/cluster/manager.go`** (extended) — config builder learns to read `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]` (the standardised cluster-side H2-config carrier on the v3 proto), peeks at the `explicit_http_config` discriminator, sets a `useH2 bool` on the resulting `*Cluster`. Validation: if `useH2 == true`, the cluster's `transport_socket` MUST be present, MUST be type `envoy.transport_sockets.tls`, and the parsed TLS config's `alpn_protocols` MUST include `"h2"`. Otherwise build-time error. The manager's `Cluster.UseH2() bool` accessor is consumed by the HCM at filter-build time to construct the right router-action variant.
- **`internal/filter/hcm/actions.go`** (extended) — new `routerActionH2` variant alongside the phase-04 `routerAction`. Build-time choice: at `NewFilter` time, the route's resolved cluster's `UseH2()` is checked; if true, a `routerActionH2{cluster *cluster.Cluster}` is constructed; if false, the existing `routerAction{cluster *cluster.Cluster}` is constructed. `routerActionH2.do(ctx, req H2Request, w H2ResponseWriter) error` calls `cluster.DialH2(ctx)`, defers `clientConn.Close()`, calls `clientConn.RoundTrip(ctx, req)`, writes the response back via `w`. `direct_response` action is unchanged from phase 04 but a thin H2 adapter writes the local-reply via H2 frames (HEADERS + DATA + END_STREAM) instead of HTTP/1.1 wire bytes — see §5.5.
- **`internal/filter/hcm/filter.go`** (extended) — `Filter.Handle(ctx, downstream)` learns ALPN dispatch. The new dispatch logic: type-assert downstream to `*stdtls.Conn`; if assertion succeeds, read `ConnectionState().NegotiatedProtocol`; if `"h2"`, dispatch to `h2.ServerConn`; else (including `"http/1.1"` or `""`), dispatch to the existing phase-04 H1 `runConnection`. If the assertion fails (plaintext listener), dispatch directly to the H1 driver (unchanged). Build-time: `codec_type: HTTP2` rejects plaintext listeners (the listener manager already knows whether the listener has a `transport_socket`; the HCM is constructed *after* listener-build, but the `manager` builder passes a `listenerCtx{hasTLS bool}` into `hcm.NewFilter` so HCM can validate at build time).
- **`internal/filter/hcm/config.go`** (extended) — `codec_type: HTTP2` is now permitted; `codec_type: AUTO` continues to be permitted (phase-04 maps AUTO→HTTP1; phase-05 redefines AUTO→ALPN-driven, which on a non-TLS listener still resolves to HTTP1, mirroring upstream Envoy's documented default). HTTP3 still errors. Additional silent-ignore: `http2_protocol_options` (the directly-on-HCM field; distinct from cluster-side `HttpProtocolOptions`).

### 4.2 Changed production code

- **`internal/listener/manager.go`** — minimal extension: when constructing an HCM filter, the listener-side build now passes a `listenerCtx{hasTLS: tc != nil}` value into `hcm.NewFilter` so HCM can validate codec_type vs transport semantics at build time. No filter-type-URL registry change — HCM is already registered (phase 04). The listener's `transport_socket` plumbing for ALPN was completed in phase 03; phase 05 adds NO listener-side ALPN handling beyond what phase 03 already does (the negotiated ALPN is observed by HCM after the handshake, not by the listener).
- **`internal/listener/manager_test.go`** — extended: HCM with `codec_type: HTTP2` on a plaintext listener errors at build with `listener: ...: hcm: codec_type HTTP2 requires TLS transport_socket`; HCM with `codec_type: AUTO` on a TLS listener with `alpn_protocols: ["h2", "http/1.1"]` builds successfully.
- **`internal/cluster/cluster.go`** — `Cluster.UseH2() bool` accessor added; `Dial(ctx)` is unchanged.
- **`internal/cluster/cluster_test.go`** — extended: H2-cluster build (positive); H2-cluster build without TLS (build error); H2-cluster build with TLS but `alpn_protocols` lacks `h2` (build error); `DialH2` over a TLS h2-capable backend (positive); `DialH2` over a TLS h1-only backend (errors with the alpn-mismatch diagnostic); `DialH2` over a plaintext backend (errors with the not-a-TLS-conn diagnostic).
- **`internal/bootstrap/bootstrap.go`** — adds blank imports for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` (carries the `HttpProtocolOptions` proto) so `protojson` can round-trip phase-05 fixture bootstraps. Per ADR-0016's amendment policy, this addition is documented in PROGRESS, not a new ADR.
- **`internal/cluster/`** — the `_ "...upstreams/http/v3"` import is also blank-imported in `internal/cluster/cluster.go` so the `typed_extension_protocol_options` round-trip works at unit-test time.
- **`internal/filter/tcpproxy/`** — unchanged.
- **`internal/tls/`** — unchanged. The phase-03 `alpn_protocols` plumbing already covers what phase 05 needs on the listener side and the cluster side.
- **`cmd/envoy-go/main.go`** — unchanged. The HCM's H2 dispatch is internal to the filter; `main` does not need to know about codec selection.
- **`cmd/envoy-go/main_test.go`** — extended with a fourth bootstrap variant: a TLS listener with `alpn_protocols: ["h2"]` and HCM `codec_type: HTTP2`, one direct_response route. Asserts the binary serves the configured response on a `localhost` HTTP/2-over-TLS client probe (the test uses a self-signed cert and `golang.org/x/net/http2.Transport` with `InsecureSkipVerify: true` — driver-side use of x/net/http2.Transport is permitted; runtime is not).

### 4.3 New harness and fixture code

- **`test/differential/fixture/fixture.go`** — possibly extended with an optional `H2Expectations() []H2RequestExpectation` method on `Driver` (parallel to phase-04's `HTTPExpectations`). The structure is the same shape: per-request expected status + per-request body equivalence flag; the runner orchestrates the H2 round-trips and compares. Alternative: have the 0004 driver issue + assert in-band, mirroring fixture 0003 if the planner picks that style. Both are SPEC-compatible; the planner records the choice.
- **`test/differential/runner_test.go`** — call sites updated for the optional `H2Expectations` extension if added; blank-import for `test/fixtures/0004-h2-routing/driver` added.
- **`test/fixtures/0004-h2-routing/`** — new fixture directory. Contents:
  - `envoy-go.yaml` — subject bootstrap. 1 listener (`l_h2`) binding `127.0.0.1:0` with a `transport_socket: tls` carrying a self-signed cert pair (regenerated at fixture build time, mirroring fixture-0002's `pki/gen/` pattern) and `alpn_protocols: ["h2", "http/1.1"]`. 1 filter chain with empty `filter_chain_match`. 1 HCM network filter with `codec_type: AUTO` so ALPN drives codec selection per-connection. Same routes as fixture 0003 (`/health` direct_response 200, `/api` → cluster, `/missing` direct_response 404). 1 STATIC cluster `c_h2_backend` with three TLS upstream endpoints, each carrying a `transport_socket: tls` with `alpn_protocols: ["h2"]` and a `validation_context` pointing at the same fixture-local CA. The cluster has `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"] = {explicit_http_config: {http2_protocol_options: {}}}`.
  - `envoy.yaml` — reference bootstrap. Same listener shape, same HCM, same routes. 1 STRICT_DNS cluster `c_h2_backend` pointing at `host.docker.internal` × three TLS ports with `dns_lookup_family: V4_ONLY` per ADR-0010; same `transport_socket` and `HttpProtocolOptions` shape as the subject. The HCM `route_config` is identical between the two bootstraps (verbatim, modulo cluster.address differences).
  - `expectations.yaml` — prose description of the 27-request workload (mirroring fixture 0003's prose form per the M-6 carry-forward decision in §12). Allow-list lines enumerated for `Date`, `Server`, `x-envoy-*`, `x-forwarded-*`, `x-request-id`, plus the H2-pseudo-header presence rule (`:status`, etc.).
  - `README.md` — explains the fixture's purpose (HCM H2 dispatch + route match + router H2 + direct_response H2), the STATIC-vs-STRICT_DNS divergence (same as 0001/0002/0003), the ALPN-h2 e2e shape, the upstream-TLS-now-exercised closure of ADR-0035, the `--concurrency 1` reference pin inherited from ADR-0028.
  - `pki/gen/main.go` — port of fixture-0002's `pki/gen/` PKI generator emitting CA + leaf for the H2 backend's three endpoints + the listener's server cert. Run-time: `go generate ./test/fixtures/0004-h2-routing/...`.
  - `pki/*.pem` — generated artefacts; committed (mirroring fixture 0002).
  - `driver/driver.go` — `BackendCount() = 3`. `SubjectListenerName() = "l_h2"`. `ReferenceListenerPort() = 15004`. `ReferenceBootstrap(backendPorts)` and `SubjectConfig` render the YAMLs with the three backend ports. `DriveReference(ctx, addr)` / `DriveSubject(ctx, addr)`: each issues 27 H2 requests against the proxy (the proxy listener is HTTPS h2; client uses `x/net/http2.Transport` with the fixture CA in `RootCAs` and `NextProtos: ["h2"]`):
     - 9 × `GET /health` → expects `:status 200`, body `OK\n`.
     - 9 × `GET /api/v1/<n>` n=0..8 → expects `:status 200`, body `backend-<idx>:v1/<n>` for the picked backend (RR index per side; per-request body equivalence is NOT asserted across sides, mirroring phase-04's relaxation per ADR-K — only `:status` and per-side distribution).
     - 9 × `GET /missing/<n>` → expects `:status 404`, body relaxed.
     The 27 requests are issued sequentially; each request opens a fresh H2 connection (new `*http2.ClientConn` per request, mirroring the runtime's per-request fresh upstream conn — this also keeps the fixture's RR distribution deterministic). `AssertDistribution(refCounts, subjCounts [3]uint64) error` checks each side's `c_h2_backend` distribution is exactly `[3, 3, 3]` over the 9 router-action requests. Returns the concatenated status-code bytes + decoded-body bytes from all 27 requests.
     `H2Expectations()` (if added) returns the 27 expectations enumerated above.
     `ProbeAdmin` — same as phase 02 / 03 / 04. Admin remains H1 in phase 05.
  - `driver/driver_test.go` — distribution-assertion helper unit test (mirror of fixture 0003's).
  - `backends/main.go` — small Go program that starts an HTTPS h2 echo server on a configurable port, reading cert paths from flags. Used by the harness-side per-fixture backend orchestration. Implementation: `net.Listen("tcp", ":port")`; `tls.NewListener` with the loaded cert + `NextProtos: []string{"h2"}`; `&http.Server{Handler: echoHandler, TLSConfig: tlsConfig}`; the server uses Go's `http2.ConfigureServer` (driver-side use OK) to enable H2 on the stdlib `http.Server` — this is *test backend*, not envoy-go runtime. The handler writes `backend-<idx>:<path-suffix>` as the response body where `<idx>` is the backend's instance id (env var) and `<path-suffix>` is the path component after the last `/`.
- **`test/conformance/h2spec/h2spec_test.go`** — Go test driver. Spins up a `summerwind/h2spec` container via `testcontainers-go` pinned by tag + SHA from `docs/envoy-go/CONFORMANCE_PINS.md`. Boots a phase-05 envoy-go subject with a single h2c listener bound to a host port (the test-only h2c listener has `codec_type: HTTP2` and a NEW build-time exemption: HTTP2 is permitted on a plaintext listener IFF a `subject_only_h2c_test: true` synthetic flag is set on the bootstrap — flag is present only in conformance test bootstraps, never in fixtures or production configs; flag presence is not a doctrine concern because the bootstrap loader already accepts proto-extension fields, but we'll implement this as a hardcoded test-only entry-point on `cmd/envoy-go/main.go` that bypasses the codec_type-vs-transport check ONLY when invoked via a hidden CLI flag like `--allow-h2c`; the planner picks the exact mechanism). Runs h2spec against the listener, parses the JSON output (h2spec emits structured TAP-or-JSON via `--strict --junit-report`), asserts `failed == 0` over the threshold-section subset declared in `BEHAVIOR_CONTRACT.md`'s `## HTTP/2` subsection. Runtime budget: ~30s for the full h2spec run; the CI gate is `go test ./test/conformance/h2spec/...`.
- **`test/conformance/h2spec/h2spec.go`** (small helper, no `_test.go`) — defines the threshold section list as a Go slice of strings. The slice is the canonical reference; `BEHAVIOR_CONTRACT.md`'s `## HTTP/2` subsection is the human-readable narrative form of this slice.
- **`test/helpers/h2.go`** — `H2RoundTrip(ctx context.Context, addr string, tlsConf *tls.Config, method, path string, headers []hpack.HeaderField, body []byte) (status int, respHeaders []hpack.HeaderField, respBody []byte, err error)`: dials TLS, opens an h2 client conn (using x/net/http2.Transport on the driver side), issues one request, returns the response. Used by the 0004 driver. Returns the body separately from the response object because callers want raw bytes for byte-compare.
- **`test/helpers/h2_test.go`** — round-trip tests against an in-process h2 server.

### 4.4 Changed documentation and state

- **`docs/envoy-go/ROADMAP.md`** — phase 05 row: `status: planned → in-progress` already flipped at SPEC-stage entry (commit `8d18320` on this branch — the corrected pattern per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3, deviating from phase-04's bd9c13f pattern where the flip happened at state-2 entry). Transitions to `done` at the §5.3 phase-done commit.
- **`docs/envoy-go/STATE.md`** — updated at each lifecycle transition (SPEC drafted = state 2 candidate; PLAN written = state 3; …).
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — add new subsection **HTTP/2** covering: (a) per-stream `:status` equivalence per request; (b) decoded-body equivalence on `direct_response` 2xx paths; (c) header set-equality modulo allow-list inherited from phase-04 plus H2-specific entries (`:status` required; `:method`/`:path`/`:scheme`/`:authority` upstream-side preservation rule); (d) structurally-equivalent framing rule (frame *types* and *order on equivalent events* match; frame byte-equivalence NOT asserted; the harness compares decoded request/response semantics, not wire bytes — this rule is the §7.2 row materialised); (e) per-stream-not-per-conn flow control NOT asserted (the harness compares decoded payload, not WINDOW_UPDATE byte counts); (f) ALPN-driven codec selection equivalence (downstream ALPN h2 → both proxies dispatch h2; downstream ALPN http/1.1 → both proxies dispatch h1 — phase 05 covers only the h2 path since fixture 0004 advertises `["h2", "http/1.1"]` but exercises only h2 from the driver; the h1 fall-through path is the phase-04 surface and remains green via fixture 0003); (g) upstream-h2-TLS exercised — closes ADR-0035's gap; the upstream side of the diff is now full-stack TLS h2 not just plaintext H1; (h) h2spec threshold list and exclusions; (i) does-not-yet-apply-to enumeration (HTTP/3, server push, trailers, gRPC).
- **`docs/envoy-go/CONFORMANCE_PINS.md`** — NEW file. Pins the `summerwind/h2spec` Docker image by tag + SHA256, mirroring `ENVOY_TARGET.md`'s discipline. Refresh procedure: run h2spec at the candidate version against the subject; investigate any new failures; either fix envoy-go or extend `BEHAVIOR_CONTRACT.md`'s threshold list (with an ADR superseding ADR-U); update the pin; commit. Per D-3.7 the pin is changed only via a dedicated phase or sub-phase.
- **`docs/envoy-go/DECISIONS.md`** — new ADRs introduced by phase 05 (numbers assigned at planning/implementation time; expected starting number ADR-0045 based on phase-04's ADR-0044 tail; the planner verifies at write time). Anticipated:
  - **ADR-P:** HTTP/2 codec source — `golang.org/x/net/http2.Framer` + `golang.org/x/net/http2/hpack`. Options considered: (P1) handcrafted RFC 9113 framer + handcrafted HPACK (highest control, highest cost — HPACK alone is a non-trivial dynamic-table state machine, and getting it wrong is a security issue per CVE history); (P2) x/net/http2 sub-packages used as low-level codec only (the SPEC's choice); (P3) build on `http2.Server` / `http2.Transport` (FORBIDDEN by D-3.2 — these are server/transport runtimes, not codecs). (P2) keeps the doctrine intent (own connection manager, own state machine, own dispatch) while sidestepping the (P1) cost-of-correctness tax on HPACK. Documents the residual surface that x/net/http2 owns vs what envoy-go owns: x/net owns frame byte-layout serialisation and HPACK table state; envoy-go owns the entire connection lifecycle, settings handshake, stream demux, flow control, error dispatch, GOAWAY/RST_STREAM/PING semantics, and the bridge to HCM's filter chain. Supersedes nothing.
  - **ADR-Q:** HCM H2 connection manager from scratch — the per-conn state machine and per-stream state machine. Documents the explicit decision NOT to use the Server-or-Transport runtime constructs that `golang.org/x/net/http2` exposes (concretely: `http2.Server`, `http2.Server.ServeConn`, `http2.ConfigureServer`, `http2.Transport`, `http2.Transport.NewClientConn`) even though they ostensibly fit the "low-level" framing. Rationale: those types carry their own request-routing, header-canonicalization, response-header injection, and error policies that diverge from Envoy's; we'd have to fight or unwind those to match Envoy semantics. Building directly on `http2.Framer` + `hpack` is cheaper. Records the architectural shape of `ServerConn`/`serverStream`/`ClientConn`.
  - **ADR-R:** Per-request fresh upstream H2 dial. Mirrors ADR-0039 (per-request fresh upstream H1 dial). Documents that upstream H2 multiplexing is intentionally unrealised at phase 05; pooling lands with the upstream-robustness family. The phase-05 differential surface does not assert pool/non-pool — Envoy pools, envoy-go does not, both produce the same per-request `:status`/body output, both produce per-side `[3,3,3]` distribution under sequential-request workload, the cross-conn frame counts differ but those are not in the equivalence matrix. Carry-forward to upstream-robustness.
  - **ADR-S:** Phase-05 H2 server settings — hardcoded defaults: `MAX_CONCURRENT_STREAMS=100`, `INITIAL_WINDOW_SIZE=65535`, `MAX_FRAME_SIZE=16384`, `ENABLE_PUSH=0`, `NO_RFC7540_PRIORITIES=1`, `HEADER_TABLE_SIZE=4096`. Rationale: matches Envoy's documented defaults except for header-table-size (Envoy's default is 4096 too — RFC default — so this matches). The differential gate does not assert SETTINGS values byte-for-byte (those are inside the structurally-equivalent framing rule). Cluster-side `http2_protocol_options` field-level fidelity is silently-ignored (advertise hardcoded defaults regardless of config); recorded as part of the silently-ignored set in §9.
  - **ADR-T:** BEHAVIOR_CONTRACT HTTP/2 subsection. Codifies the phase-05 equivalence surface (see §1 #9 and §5.7 below). Includes the H2-pseudo-header rules and the structurally-equivalent framing rule. Supersedes nothing.
  - **ADR-U:** h2spec conformance scope and threshold. Pins `summerwind/h2spec` by tag + SHA. Declares the section list under threshold: 3 (HTTP Frame Format), 4 (HPACK), 5 (Streams and Multiplexing), 6 (Frame Definitions) MINUS 6.6 (PUSH_PROMISE), 7 (Error Codes), 8 (HTTP Message Exchanges). Excludes 6.6 because phase 05 disables push (`ENABLE_PUSH=0`); the section is conformance-irrelevant. Records the per-section pass-count expected at phase-done. The pin's refresh procedure is documented in `docs/envoy-go/CONFORMANCE_PINS.md`. Supersedes nothing (this is the project's first conformance ADR).
  - **ADR-V:** ALPN-driven codec selection wiring. Records the architectural choice that codec selection happens *inside* `Filter.Handle` after the TLS handshake completes (by inspecting `*tls.Conn.ConnectionState().NegotiatedProtocol`), NOT at the listener-side filter-chain match step. Rationale: keeps phase 03's filter-chain-match SNI-only surface unchanged (per ADR-0033), keeps phase 07's filter-chain framework as the natural home for `application_protocols` chain matching when it lands, and minimises blast radius (HCM gains a small dispatch helper; listener manager doesn't change). Documents the alternative considered (treat ALPN as a chain-match dimension) and why it was deferred.
  - **ADR-W:** Closes ADR-0035 — fixture-0004 differentially exercises upstream TLS h2. Documents that ADR-0035's narrowed scope (fixture-0002 plaintext upstream backends) is now superseded for the H2 surface specifically: fixture-0004 has full-stack HTTPS h2 between proxy and upstream backends, so the upstream-TLS code path (phase-03's `Cluster.Dial` TLS branch + phase-05's `DialH2`) is now under differential coverage. The H1+TLS upstream gap remains open from ADR-0035 — a future HTTPS-H1 fixture (or an extension of fixture 0003 to TLS upstream) closes the H1 leg; phase 05 does not. ADR-W explicitly carries forward the H1+TLS upstream gap with a "phase-05+follow-up" tag.
  - **ADR-X:** Phase-04 REVIEW Minor carry-forward triage. Records the phase-05 disposition of M-2/M-4/M-5/M-6/M-7 from `docs/envoy-go/phases/04-http-1.1/REVIEW.md` (commit 04527eb). Disposition decided in §12 below; this ADR is the formal landing spot.
  - **ADR-Y:** Trailer scope at phase 05 — observed but not forwarded. Documents that the H2 codec correctly observes trailing HEADERS frames (RFC 9113 compliance — h2spec section 8 asserts this) but the router action discards trailers in both directions. Forward-looking note: phase 07's filter-chain framework + the gRPC family land trailer forwarding.
  - **ADR-Z:** Test-only `--allow-h2c` flag (or equivalent) on `cmd/envoy-go` to permit `codec_type: HTTP2` on a plaintext listener for h2spec conformance only. Documents the security posture (flag is not advertised; default OFF; production builds may strip the flag); the flag exists solely so `test/conformance/h2spec/` can run h2c against the subject without a TLS handshake stealing test cycles. Alternative considered: run h2spec over TLS — rejected because h2spec's TLS support requires a custom CA and complicates the conformance pin; running h2c is the documented h2spec convention.

(The planner re-numbers these to ADR-0045..ADR-0054 (or wherever the tail lands) at write time. The lettered placeholders here exist for SPEC clarity; they have no on-disk meaning.)

## 5. Architecture and components

### 5.1 Module graph (new / changed shape)

Phase 05 introduces one new sub-package and modifies several existing ones:

```
cmd/envoy-go/main.go                 (unchanged at wiring; --allow-h2c hidden flag added per ADR-Z)
cmd/envoy-go/main_test.go            (extended: h2-bootstrap variant)
internal/listener/manager.go         (MODIFIED: passes listenerCtx to hcm.NewFilter)
internal/listener/manager_test.go    (extended: h2 build cases)
internal/cluster/cluster.go          (MODIFIED: UseH2() accessor)
internal/cluster/manager.go          (MODIFIED: parses HttpProtocolOptions extension)
internal/cluster/dial_h2.go          (NEW: Cluster.DialH2)
internal/cluster/cluster_test.go     (extended: h2-cluster build + DialH2)
internal/filter/hcm/filter.go        (MODIFIED: ALPN dispatch in Handle; listenerCtx in NewFilter)
internal/filter/hcm/config.go        (MODIFIED: codec_type=HTTP2 permitted; new ignored field)
internal/filter/hcm/actions.go       (MODIFIED: routerActionH2; H2-aware direct_response writer)
internal/filter/hcm/connection.go    (UNCHANGED — the H1 driver is the existing connection.go)
internal/filter/hcm/h2/              (NEW sub-package)
   conn.go         — ServerConn + connection state machine
   stream.go       — serverStream + per-stream state machine
   framer.go       — http2.Framer wrapper with ctx cancellation
   hpack.go        — encoder/decoder integration
   flow.go         — connection + per-stream flow control
   preface.go      — server preface check
   settings.go     — SETTINGS handshake helpers
   errors.go       — error code enum + h2.Error type
   client.go       — ClientConn + RoundTrip (used by routerActionH2)
   <test files>    — exhaustive unit tests; FuzzFrameStream + FuzzHPACKDecode
internal/bootstrap/bootstrap.go      (MODIFIED: blank import for upstreams/http/v3)

test/conformance/h2spec/             (NEW)
   h2spec_test.go  — testcontainers-driven h2spec runner
   h2spec.go       — threshold section list

test/fixtures/0004-h2-routing/       (NEW fixture)
   envoy.yaml, envoy-go.yaml, expectations.yaml, README.md
   pki/gen/main.go, pki/*.pem
   driver/driver.go, driver/driver_test.go
   backends/main.go

test/helpers/h2.go + h2_test.go      (NEW: H2RoundTrip helper)

docs/envoy-go/BEHAVIOR_CONTRACT.md   (MODIFIED: ## HTTP/2 subsection)
docs/envoy-go/CONFORMANCE_PINS.md    (NEW: h2spec image pin)
docs/envoy-go/DECISIONS.md           (APPENDED: ADR-0045..ADR-0054 — final numbering at planner's write time)
docs/envoy-go/ROADMAP.md             (already flipped to in-progress at 8d18320; flips to done at phase-end)
docs/envoy-go/STATE.md               (updated at each lifecycle transition)
docs/envoy-go/phases/05-http-2/SPEC.md / PLAN.md / PROGRESS.md / REVIEW.md
```

### 5.2 Downstream HTTP/2 connection lifecycle

A downstream conn that arrives at `Filter.Handle` (after the listener has accepted, the filter chain has dispatched, and the TLS handshake has completed) flows like this on the h2 path:

1. `Filter.Handle(ctx, downstream)` type-asserts `downstream` to `*stdtls.Conn`. Reads `ConnectionState().NegotiatedProtocol`. If `"h2"`, jumps to step 2. Otherwise dispatches to the H1 driver (phase-04 unchanged).
2. Constructs `serverConn := h2.NewServerConn(ctx, downstream, table, defaultServerSettings)`. Calls `serverConn.Run()`.
3. `Run()`:
   a. Reads 24 bytes; verifies the connection preface bytes (RFC 9113 §3.4).
   b. Writes the server's initial SETTINGS frame.
   c. Reads the client's initial SETTINGS frame; applies the values; writes SETTINGS_ACK.
   d. Reads the SETTINGS_ACK for our own initial SETTINGS.
   e. Enters the frame loop: `frame := framer.ReadFrame()`; dispatch by frame type to the connection-level handler or to the per-stream handler. PING → emit PING_ACK. WINDOW_UPDATE (stream 0) → adjust connection send window. WINDOW_UPDATE (stream N) → adjust stream N's send window. SETTINGS → apply + ack. PING_ACK → discard. GOAWAY → mark conn for close, finish in-flight streams, exit. RST_STREAM → mark stream as closed. HEADERS (new stream) → construct serverStream, start dispatch goroutine. HEADERS (existing stream — for trailers) → observe + discard per ADR-Y. DATA → feed stream's body pipe; on END_STREAM, signal dispatch.
   f. On any connection-level error, emit GOAWAY with the appropriate error code, drain pending writes, close.
4. Each new stream (HEADERS dispatch in step 3.e) spawns a per-stream dispatch goroutine that:
   a. Builds an `H2Request` from the decoded HEADERS + the body pipe reader.
   b. Calls `table.Match(req)` (the route table from phase 04 — re-used unchanged; the route match operates on `req.URL.Path` derived from the `:path` pseudo-header).
   c. Invokes the matched action (`directResponseAction.do` or `routerActionH2.do`) which writes the response back via the stream's send-side helpers. The action is responsible for emitting END_STREAM on the response.
   d. On action error, the goroutine emits RST_STREAM with INTERNAL_ERROR (or whatever `*h2.Error` was returned) and closes the stream.
5. `Run()` exits when: (a) downstream EOF + no streams open; (b) GOAWAY exchanged; (c) ctx cancelled; (d) unrecoverable connection-level error (in which case GOAWAY is emitted before exit).

### 5.3 Upstream HTTP/2 connection (router action)

`routerActionH2.do(ctx, req H2Request, w H2ResponseWriter) error`:

1. `clientConn, err := r.cluster.DialH2(ctx)`. On error → write 502 local reply via H2 (HEADERS `{:status: 502}` + DATA `bad gateway\n` + END_STREAM) on the downstream stream; return nil. (Local-reply prose body is relaxed under the phase-04 framing-divergence rule extended for H2 per BEHAVIOR_CONTRACT phase-05 subsection.)
2. `defer clientConn.Close()`. Per-request fresh conn — no pooling (ADR-R).
3. `resp, err := clientConn.RoundTrip(ctx, req)`. The `RoundTrip` method opens stream id 1 (the only stream the conn ever uses) on the upstream conn, writes the request HEADERS+DATA+END_STREAM, reads the response HEADERS+DATA+END_STREAM, returns the assembled `H2Response`.
4. Write the response back through `w`: `w.Headers(resp.Headers, false)`; copy DATA frames; `w.Headers(emptyTrailers, true)` OR a final DATA with END_STREAM (the planner picks whether to use trailing-headers or DATA-with-END for response close — both are valid per RFC 9113 §8.1; phase-05 picks DATA-with-END for the simpler shape, mirroring x/net/http2.Server's default response close).
5. Return.

### 5.4 ALPN dispatch decision tree

```
Filter.Handle(ctx, downstream):
  if codec_type == HTTP1:
    runH1Connection(ctx, downstream, table)
    return
  if codec_type == HTTP2:
    if downstream is not *stdtls.Conn AND not --allow-h2c:
      log.Printf("hcm: codec_type HTTP2 requires TLS"); return  // build-time prevents reaching this
    // For codec_type=HTTP2 we run the H2 driver unconditionally:
    // - --allow-h2c (test-only) path: plaintext h2c, no TLS conn, no ALPN
    // - TLS path with ALPN h2: production
    // - TLS path with ALPN http/1.1 (or empty): misconfigured client; H2 driver's
    //   preface check fails on the first 24 bytes and the conn is dropped
    runH2Connection(ctx, downstream, table)
    return
  if codec_type == AUTO:
    if downstream is *stdtls.Conn:
      alpn := downstream.(*stdtls.Conn).ConnectionState().NegotiatedProtocol
      if alpn == "h2":
        runH2Connection(ctx, downstream, table)
        return
    runH1Connection(ctx, downstream, table)
    return
```

The `codec_type: HTTP2` over TLS with ALPN h2 is the production path; over TLS with ALPN http/1.1 (a misconfigured client speaking h1 against an h2-only listener) the H2 driver runs and the client's first request bytes (which look like an H1 request line) fail the preface check, returning a connection-level error and closing — symmetrical to upstream Envoy's posture. The `--allow-h2c` test-only path bypasses the TLS-required check; production builds may compile that flag out per ADR-Z.

### 5.5 Direct-response on H2

The phase-04 `directResponseAction` writes HTTP/1.1 wire bytes (`writeStatusReply`). For H2, we need a parallel path that writes H2 frames. The cleanest factoring (and the one the planner is encouraged to take) is:

- `directResponseAction` gains a method `body() (status int, headers http.Header, body []byte)` returning the synthesised reply in a codec-neutral form.
- An H1 adapter (`writeH1`) writes the HTTP/1.1 wire bytes (current phase-04 behaviour).
- An H2 adapter (`writeH2`) writes HEADERS (`:status`, plus the configured headers) + DATA (body bytes) + END_STREAM via the per-stream helpers.

The H1 connection driver invokes `writeH1`; the H2 stream dispatch invokes `writeH2`. The action carries no codec awareness; the caller picks the writer.

### 5.6 Route table re-use

Phase-04's `routeTable` operates on `req.URL.Path` and matches via `prefix` or `path`. Phase 05 re-uses this *verbatim*: the H2 stream dispatch builds an `*http.Request` (or a phase-05-internal equivalent) whose `URL.Path` is the `:path` pseudo-header decoded value, then calls `table.match(req)`. No change to `route.go`; no change to `actions.go`'s `directResponseAction`; only the new `routerActionH2` is added.

### 5.7 BEHAVIOR_CONTRACT phase-05 subsection (HTTP/2)

The new subsection codifies these rules. The wording below is the intended SPEC-binding form for the planner to lift into `BEHAVIOR_CONTRACT.md`:

- **Asserted equivalence:**
  - `:status` per request, exact integer match.
  - Decoded body byte-equivalence on `direct_response` 2xx paths (the 9 × `/health` requests).
  - Per-stream response header set-equality modulo the phase-04 allow-list (`Date`, `Server`, `Content-Length`, `Transfer-Encoding`, every `x-envoy-*`, every `x-forwarded-*`, `x-request-id`) extended for H2 with: `:status` always required and asserted; the four request pseudo-headers `:method`/`:path`/`:scheme`/`:authority` asserted on the upstream request preservation surface (the router must forward them verbatim from the downstream request modulo path normalisation per ADR-0037-on-H2 — H2 doesn't have stdlib net/http parsing so the path normalisation is *empty* here, the path is the bytes of the `:path` pseudo-header).
  - Route-match selection equivalence: same method + path → same matched route on both proxies (witnessed indirectly by per-side `[3,3,3]` distribution + `:status` per request).
  - Per-cluster RR distribution `[3, 3, 3]` per side over the 9 router-action requests (local-correctness; cross-side sequence is NOT asserted, mirroring phase-04).
  - ALPN selection equivalence: a downstream client that advertises only `h2` reaches the H2 driver on both proxies.
- **Not asserted:**
  - Wire-byte H2 framing (frame headers, frame ordering at byte level, padding bytes, HPACK encoded-bytes representation). Frame *types* and *types-on-equivalent-events* are required to match; frame *byte-equivalence* is not.
  - Response body bytes for routed-to-upstream requests (mirror of phase-04's relaxation per ADR-K).
  - SETTINGS values byte-for-byte.
  - WINDOW_UPDATE timing or count (different windows + different consumption pacing yield different WINDOW_UPDATE frame counts; that's structurally divergent but semantically equivalent).
  - Stream id allocation pattern (Envoy's stream ids may differ from envoy-go's; both must be odd-numbered server-allocated client streams per RFC 9113 §5.1.1, but the specific ids assigned aren't compared).
  - Trailers (per ADR-Y, both proxies discard trailers; this is asymmetric to upstream Envoy which forwards them — but the fixture driver doesn't send trailers, so the divergence isn't exercised).
  - 0-RTT TLS early-data behaviour.
  - Connection re-use upstream (per ADR-R).
- **Header allow-list extensions** (rows added to the table at the top of `BEHAVIOR_CONTRACT.md`):
  - `:status` (HCM-locally-generated H2 responses) — presence required on both sides; value asserted.
  - `:scheme`, `:authority`, `:path`, `:method` (routed-to-upstream H2 requests) — presence required; value asserted (verbatim forwarding).
- **h2spec threshold:** sections 3, 4, 5, 6 (excluding 6.6 PUSH_PROMISE), 7, 8 — all `failed == 0`. Pin: `summerwind/h2spec` v2.x at the SHA recorded in `CONFORMANCE_PINS.md`.
- **Applies to:** phase-05 `internal/filter/hcm/h2/` package; phase-05 `internal/cluster/dial_h2.go`; fixture 0004; the conformance suite under `test/conformance/h2spec/`.
- **Does not yet apply to:** HTTP/3, server push, gRPC framing, trailer forwarding, upstream H2 stream pooling, h2c production fixtures, mTLS over h2.

### 5.8 Cluster H2 build-time validation

Phase 05's cluster builder accepts `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]` only with the `explicit_http_config.http2_protocol_options` discriminator. Inside that, every field is silently ignored at phase 05 (advertise hardcoded defaults regardless). Validation:

- Cluster has `HttpProtocolOptions` with `http2_protocol_options` → `useH2 = true`.
  - Cluster MUST have `transport_socket: tls` (build error if not).
  - The TLS config's `alpn_protocols` MUST include `"h2"` (build error if not).
- Cluster has `HttpProtocolOptions` with `http_protocol_options` (the H1 variant of the discriminator) → silently ignored at phase 05; the cluster behaves as H1 (phase-04 path), which is the default anyway. Phase-05 does NOT introduce per-cluster H1 typed-options support beyond this; phase-04's per-request fresh-dial behaviour is unchanged.
- Cluster has `HttpProtocolOptions` with `auto_config` → build error: "phase 05 does not support http_protocol_options.auto_config".
- Cluster has neither → unchanged (H1 path).

### 5.9 h2spec integration shape

`test/conformance/h2spec/h2spec_test.go`:

```
TestH2Spec(t):
  1. Read pinned image tag + SHA from docs/envoy-go/CONFORMANCE_PINS.md (or a sibling .go file with the const; the planner picks).
  2. Build envoy-go binary into a temp dir.
  3. Start envoy-go with a synthetic h2c bootstrap on a host port (--allow-h2c flag) listening on 127.0.0.1:<dyn>.
  4. Start the h2spec container via testcontainers-go pinned by SHA.
  5. Exec h2spec inside the container with --host=host.docker.internal --port=<dyn> --strict --junit-report=/tmp/h2spec.xml. Wait for completion.
  6. Read /tmp/h2spec.xml, parse it into a section/test tree.
  7. For each section in the threshold list (3, 4, 5, 6, 7, 8) excluding 6.6, assert all child tests passed.
  8. Stop subject. Stop container.
```

Runtime budget: the full h2spec run is ~30s wall-clock. Per the project's conformance-suite policy the gate is `go test ./test/conformance/h2spec/...` and the test is excluded from short-budget CI (`go test -short` skips it). The planner explicitly wires the `-short` skip.

### 5.10 Request workload counts on fixture 0004

Mirrors fixture 0003's 27-request workload exactly:

- 9 × `GET /health HTTP/2.0` → `direct_response 200 OK\n` (9 ID-disjoint streams).
- 9 × `GET /api/v1/<n>` for n=0..8 → routes to `c_h2_backend`; per-side RR distribution `[3, 3, 3]` over these 9 requests; per-request body is `backend-<idx>:v1/<n>` where `<idx>` is the picked backend.
- 9 × `GET /missing/<n>` for n=0..8 → `direct_response 404 not found\n`.

Total: 27 streams per side. The driver opens a fresh `*http2.ClientConn` per request (no transport-side pooling on the driver), so each request maps to exactly one downstream H2 conn → 27 conns per side. Combined with `--concurrency 1` on the reference (per ADR-0028), this yields the deterministic per-side RR distribution.

## 6. Data flow

### 6.1 Downstream request → upstream → response

Plain-text-after-decryption view of one router-action request on phase 05:

```
[client] -- TLS handshake (ALPN: h2) --> [listener]
[client] -- preface bytes --> [serverConn]
[client] -- SETTINGS --> [serverConn]
[serverConn] -- SETTINGS_ACK --> [client]
[serverConn] -- SETTINGS --> [client]
[client] -- SETTINGS_ACK --> [serverConn]
[client] -- HEADERS{:method GET, :path /api/v1/3, ...; END_HEADERS, END_STREAM} --> [serverConn]
[serverConn] -- demux frame to streamN --> [serverStream(N)]
[serverStream] -- build H2Request --> [routeTable.match(req)]
[routeTable] -- returns matching routeEntry{action: routerActionH2{cluster: c_h2_backend}} --> [serverStream]
[serverStream] -- routerActionH2.do(ctx, req, streamWriter) --> [routerActionH2]
[routerActionH2] -- cluster.DialH2(ctx) --> [Cluster][TLS dial][ALPN "h2"]
[Cluster] -- *h2.ClientConn --> [routerActionH2]
[routerActionH2] -- clientConn.RoundTrip(ctx, req) --> [ClientConn]
[ClientConn] -- preface + SETTINGS exchange (mirror of server side) --> [upstream]
[ClientConn] -- HEADERS+END_STREAM (no body) --> [upstream]
[upstream] -- HEADERS{:status 200, content-type text/plain, ...} + DATA{"backend-1:v1/3"} + END_STREAM --> [ClientConn]
[ClientConn] -- *H2Response{headers, body} --> [routerActionH2]
[routerActionH2] -- streamWriter.Headers(resp.Headers, false) --> [serverStream]
[routerActionH2] -- streamWriter.Data(resp.Body, true) --> [serverStream]
[serverStream] -- HEADERS{:status 200, ...; END_HEADERS} --> [client]
[serverStream] -- DATA{"backend-1:v1/3"; END_STREAM} --> [client]
[serverStream] -- transition to closed --> [serverConn]
[serverStream] -- defer clientConn.Close() --> [upstream] (FIN)
```

### 6.2 Direct-response 200 path

Same up to `routeTable.match(req)`. The matched action is `directResponseAction{status: 200, body: "OK\n"}`. The H2 adapter writes `HEADERS{:status 200, content-type text/plain, content-length 3, server envoy, date <now>; END_HEADERS}` + `DATA{"OK\n"; END_STREAM}` and closes the stream.

### 6.3 No-match 404 path

`routeTable.match(req)` returns the third route's `directResponseAction{status: 404, body: "not found\n"}` (per the explicit catch-all on the route table from §5.10), or — if the planner picks the implicit-404 form — the dispatch helper synthesises a 404 in-band (mirroring phase-04's connection-loop fallback). Either way: `HEADERS{:status 404, ...}` + `DATA{"not found\n"; END_STREAM}`.

## 7. Error handling and failure modes

The phase-05 H2 codec follows RFC 9113's two-tier error model:

- **Connection-level errors** trigger GOAWAY + close. Examples: bad preface, malformed SETTINGS, HPACK COMPRESSION_ERROR, FRAME_SIZE_ERROR on a non-DATA frame, PUSH_PROMISE received from client (PROTOCOL_ERROR), stream id reuse, even-numbered stream id from client (PROTOCOL_ERROR), connection-level WINDOW_UPDATE with increment=0, GOAWAY received with NO_ERROR (graceful — finish in-flight, don't accept new streams).
- **Stream-level errors** trigger RST_STREAM + per-stream cleanup, conn keeps running. Examples: stream-level FRAME_SIZE_ERROR on DATA, request that fails the action (`routerActionH2.do` returned an error → RST_STREAM with INTERNAL_ERROR + per-stream goroutine exit), client sends DATA on a half-closed-remote stream (STREAM_CLOSED).

The router action's failure modes:

- **Upstream dial fails (TCP, TLS, or ALPN-mismatch)** → write a 502 local reply via H2 (HEADERS `{:status: 502, content-type: text/plain, server: envoy}` + DATA `bad gateway\n` + END_STREAM) on the downstream stream. Return nil (the connection-level error handler doesn't run; this is a per-stream-level recovery).
- **Upstream H2 protocol error (RoundTrip returns h2-error)** → similar: write 502 local reply + END_STREAM, close the upstream conn, return nil.
- **ctx cancellation mid-request** → emit RST_STREAM with CANCEL on both sides, return nil. The connection driver propagates ctx cancellation but doesn't kill the conn over a single cancelled stream.

The per-stream `defer` in the action closes the upstream `clientConn` regardless of error path. This satisfies M-5 cleanup (the phase-04 prose-vs-mechanism gap is repeated here in the H2 form, ADR-X carries the resolution forward).

Listener-bind error semantics carried forward unchanged from phase-02 (`log.Fatalf` on bind failure; admin `/ready` never reaches Ready).

## 8. Testing scope for phase 05

### 8.1 Unit tests (under `internal/filter/hcm/h2/`)

Exhaustive coverage of every state-machine transition, every error code, every flow-control corner case, every settings-handshake variant. Specific test names listed in §4.1's `h2_test.go` bullet. Test peer is `golang.org/x/net/http2.Transport` for client-side scenarios (driver-side use OK).

### 8.2 Unit tests (under `internal/cluster/`)

`DialH2` happy path, ALPN-mismatch error, not-TLS error, dial-timeout error.

### 8.3 Unit tests (under `internal/filter/hcm/`)

ALPN dispatch in `Filter.Handle` (TLS+h2 → H2 driver; TLS+h1 → H1 driver; plaintext → H1 driver). `codec_type: HTTP2` build-time error on plaintext listeners. `codec_type: HTTP2` build success on TLS listener with ALPN h2.

### 8.4 End-to-end smoke (under `cmd/envoy-go/main_test.go`)

H2-bootstrap variant: TLS listener + HCM HTTP2 + direct_response → asserted via x/net/http2.Transport client probe.

### 8.5 Differential (under `test/differential/` + `test/fixtures/0004-h2-routing/`)

The 27-request workload of §5.10 against both proxies. Status equivalence + decoded-body equivalence on direct-response paths + per-side RR distribution + 404 status equivalence.

### 8.6 Conformance (under `test/conformance/h2spec/`)

h2spec at the pinned SHA + threshold sections 3/4/5/6\6.6/7/8 → `failed == 0`. Excluded from `-short`.

### 8.7 Fuzz (under `internal/filter/hcm/h2/`)

`FuzzFrameStream` (mutates frame sequences; asserts no panic + h2:-prefixed errors). `FuzzHPACKDecode` (adversarial header blocks; asserts no panic). Short-budget 30s CI.

## 9. Out-of-scope (explicitly deferred)

Beyond §2's non-purposes, phase 05 silently ignores the following at parse time (no error, no honoured behaviour):

- HCM `http2_protocol_options` (the directly-on-HCM proto field, distinct from cluster-side `HttpProtocolOptions`).
- Cluster `HttpProtocolOptions.common_http_protocol_options` (every field).
- Cluster `HttpProtocolOptions.upstream_http_filters[]`.
- Cluster `HttpProtocolOptions.connection_pool_per_downstream_connection`.
- Cluster `HttpProtocolOptions.explicit_http_config.http2_protocol_options.{initial_stream_window_size, initial_connection_window_size, max_concurrent_streams, hpack_table_size, allow_metadata, allow_connect, max_outbound_frames, max_outbound_control_frames, max_consecutive_inbound_frames_with_empty_payload, max_inbound_priority_frames_per_stream, max_inbound_window_update_frames_per_data_frame_sent, stream_error_on_invalid_http_messaging, override_stream_error_on_invalid_http_message}` (every inner field).
- Cluster `HttpProtocolOptions.explicit_http_config.http_protocol_options.*` (the H1 discriminator's inner fields).
- Listener `filter_chain_match.application_protocols[]` (extending the phase-04 ignored set; ALPN-driven chain matching is a phase-07 concern).
- HCM `internal_address_config`, `path_with_escaped_slashes_action`, `add_user_agent`, `proxy_status_config`, `typed_header_validation_config`, `original_ip_detection_extensions`, `early_header_mutation_extensions`, `header_validation_config`. (Some of these are phase-04-already-ignored; this list calls out the ones that are H2-relevant to enumerate the phase-05 ignored-set explicitly.)

The full silently-ignored set is the union of phase-04's (per ADR-N) and phase-05's amendment above. ADR-N is amended (not superseded) to record the additions.

## 10. Deferred decisions (the planner / implementer settles these)

1. **Streaming-body dispatch vs wait-for-END_STREAM.** Phase 05 prescribes "wait for END_STREAM before invoking the action" (§1 #2) for simplicity. The planner may instead pass a streaming body reader to the action and have the router stream the body upstream — saves memory at the cost of more state. Picked here: wait-for-END_STREAM. Planner may revisit if fixture-0004 grows large bodies (it doesn't).
2. **Whether to factor `directResponseAction.body()` codec-neutral now or keep two writers.** §5.5 prescribes the codec-neutral factoring; the planner may instead keep duplicate H1-and-H2 implementations of `directResponseAction.do` if the factoring proves messier than expected. Both are SPEC-compatible.
3. **Whether to thread an `H2Request`/`H2Response` type-pair or re-use stdlib `*http.Request`/`*http.Response` shapes.** Stdlib `http.Request` is H1-shaped (it has a `Trailer http.Header` and a `Body io.ReadCloser`, both of which work for H2 conceptually). Re-use is cheaper but might leak H1-isms into H2 code. The planner picks; both are SPEC-compatible. Recommendation: re-use stdlib types for the action surface (so route-table machinery stays single-shape) but use phase-05-internal types for the H2 codec surface (frame/stream level).
4. **Whether the conformance pin lives in `docs/envoy-go/CONFORMANCE_PINS.md` (new file) or as a Go const in `test/conformance/h2spec/h2spec.go` (no doc file).** The doc file is the more durable choice (mirroring `ENVOY_TARGET.md`); the const is shorter. SPEC prescribes the doc file (§4.4); the planner may downgrade.
5. **Whether `--allow-h2c` is a CLI flag or an environment variable or a build tag (`//go:build h2c_test`).** All three work; the planner picks the lowest-friction option for the testcontainers driver.
6. **Whether `routerActionH2` and `routerAction` share an interface or are concrete types invoked via a switch in the route-table builder.** Phase 04's `actions.go` uses a small interface (`action.do(...)`); phase 05 keeps it. The planner may flatten if it makes the H2 path's writer-injection cleaner.
7. **Whether the per-cluster RR counter is per-`Cluster` (current phase-02 ADR-0024 scope) or per-`Cluster`-per-codec.** Phase-05 keeps it per-`Cluster`: H2 and H1 dials on the same cluster share a counter. Fixture 0004's cluster is H2-only so the question doesn't surface; the planner records the choice in PLAN.md if a mixed-codec cluster ever arises (it doesn't in phase 05's scope).
8. **Whether the H2 server emits `:status` first or `content-type` first in HEADERS.** RFC 9113 §8.3 requires pseudo-headers before regular headers; both x/net/http2 and Envoy comply. The planner ensures phase-05's HEADERS encoding puts pseudo-headers first; this is a correctness rule (the server-h2 conformance gate would catch a violation). Not a deferred decision so much as a note.
9. **Concrete ADR numbers for ADR-P..ADR-Z.** The planner verifies the next-free ADR number is 0045 (based on phase-04's ADR-0044 tail) and assigns 0045..0054 to ADR-P..ADR-Z respectively. The mapping is recorded in PLAN.md.
10. **Whether to introduce a structured `expectations.yaml` for fixture 0004 now or carry forward M-6's heredoc-prose pattern.** §12 below picks "carry forward heredoc per ADR-0019's deferral to observability"; the planner may overrule this choice if it's cheap to switch (the structured form is a 1-day exercise, not a phase-blocker), but the SPEC prescribes the conservative path.

## 11. Risks and mitigations

### 11.1 Phase-splitting risk (most-likely outcome)

**Risk:** Phase 05 is the largest phase to date — the H2 codec alone is ~1500 LoC of state-machine plumbing, plus the cluster-side dial helper, plus the fixture, plus h2spec integration. The phase-2 split gate (~25 tasks OR ~1500 LoC) is likely to trip when the planner writes `PLAN.md`.

**Mitigation:** the SPEC is *split-friendly*. Three plausible split axes, in declining-recommendation order:

- **Split by surface.** 05.1 = downstream H2 (codec + ServerConn + ALPN dispatch + h2spec conformance against a downstream-only listener). 05.2 = upstream H2 (DialH2 + ClientConn + routerActionH2 + fixture-0004). 05.1 is testable in isolation via h2spec and the unit test of `Filter.Handle` ALPN dispatch with an H2-only-no-router-action listener (the bootstrap is artificial but unit-testable). 05.2 builds on 05.1 and adds the differential.
- **Split by transport.** 05.1 = h2c plaintext H2 (no TLS at all — uses `--allow-h2c` everywhere; codec proven via h2spec). 05.2 = HTTPS h2 (adds TLS + ALPN dispatch + fixture 0004). This split has the disadvantage of leaving 05.1 with no differential fixture (h2c is conformance-only) — phase-done gate (a) becomes vacuously green for 05.1.
- **Split by ends.** 05.1 = downstream H2 + h2spec + fixture 0004 with H1 upstream backends (h2 → h1 router). 05.2 = upstream H2 + fixture-0004 upgrade to h2-end-to-end + DialH2. Disadvantage: 05.1 ships a degenerate fixture that doesn't close ADR-0035.

**Recommendation to planner:** if `PLAN.md` exceeds the gate, split by surface (option 1). The 05.1 sub-phase ships h2 downstream + h2spec; 05.2 ships h2 upstream + fixture 0004. ADR-0035's gap closes at 05.2.

### 11.2 h2spec image availability / pin freshness

**Risk:** `summerwind/h2spec` is community-maintained; tags can move or disappear. CI runs against a moving image break gate (c).

**Mitigation:** pin by tag + SHA256 in `CONFORMANCE_PINS.md`. The image refresh is a dedicated phase per D-3.7. CI uses the SHA digest, not the tag.

### 11.3 x/net/http2 version drift

**Risk:** `golang.org/x/net/http2` evolves; Framer or hpack APIs may shift, breaking phase-05 code. Phase-04 already pinned go.mod entries — new pins for x/net/http2 are inherited.

**Mitigation:** `go.sum` pins the SHA; module updates are explicit phase work per D-3.7.

### 11.4 HPACK compression-table-size negotiation correctness

**Risk:** SETTINGS_HEADER_TABLE_SIZE is dynamic; the receiver must respect the sender's max even mid-stream after a SETTINGS update. Subtle bugs here yield COMPRESSION_ERROR connection-aborts under heavy header churn (gRPC-style). h2spec section 4 covers this, but if our test peer (driver-side x/net/http2) doesn't drive aggressive size-update sequences, our impl might pass h2spec yet fail on real workloads.

**Mitigation:** unit test that explicitly drives a SETTINGS_HEADER_TABLE_SIZE shrink mid-conn and asserts the next outgoing HEADERS frame respects the new size. h2spec's section 4 tests cover the on-wire correctness; the unit test covers our internal table-state propagation.

### 11.5 Flow-control deadlock under tiny windows

**Risk:** A degenerate test (or a malicious client) advertises `INITIAL_WINDOW_SIZE = 1` and sends a many-byte body; our send path must block-and-wake correctly on WINDOW_UPDATE. Bugs yield deadlock.

**Mitigation:** unit test with `INITIAL_WINDOW_SIZE = 1` and a 1024-byte response body; assert delivery completes via WINDOW_UPDATE-driven progress. h2spec section 5 covers the on-wire correctness.

### 11.6 ALPN dispatch race on slow handshakes

**Risk:** `Filter.Handle` reads `NegotiatedProtocol` immediately on entry. If the listener's TLS handshake hasn't completed (it should have, but a buggy refactor could change that), `NegotiatedProtocol` returns `""` and dispatch falls through to H1 — silently mis-routing h2 traffic to the H1 driver, which produces a 400 line on the first request bytes.

**Mitigation:** the listener-side handshake-completion contract is asserted by phase-03's tests. Phase-05's `Filter.Handle` adds a defensive `downstream.(*stdtls.Conn).HandshakeContext(ctx)` no-op call before reading `NegotiatedProtocol` — the call is idempotent for already-completed handshakes; if a future refactor removes the listener-side handshake, the HCM still gets correct data. Recorded in PLAN.md.

### 11.7 Phase-04 carryover Minor M-7 (`Filter.statPrefix` unused) becomes load-bearing for phase 06 and slips

**Risk:** Phase 06 introduces stats; if phase 05 doesn't either consume `Filter.statPrefix` or explicitly mark its consumer-shape, phase 06's planner inherits an unclear handoff.

**Mitigation:** §12's M-7 disposition explicitly defers but with a SPEC-noted "phase-06-must-consume" tag in `PLAN.md`. Phase 06's brainstorm reads this tag and either honours `Filter.statPrefix` or supersedes ADR-N.

## 12. Phase-04 REVIEW carryover triage

Phase-04 closed with `REVIEW.md` (commit `04527eb`) verdict APPROVED WITH FOLLOW-UPS; all four Important findings (I-1..I-4) plus the cleanest Minor (M-1) landed in `671a059`. Five Minor findings (M-2, M-4, M-5, M-6, M-7) were deferred to "phase 05+" per `STATE.md`. M-3 was naturally resolved by the I-1 fix. Phase-05 disposition:

- **M-2 — ADR-0043's `Doctrine: D-3.4, D-3.5` mismatched against the informal supersession qualifier.** *Defer.* Phase 05 does not touch ADR-0043 (HTTPExpectations driver extension); the inconsistency is cosmetic. ADR-X carries the explicit deferral; a future doctrine-cleanup ADR (likely under the observability or admin-API phases when multiple ADRs are amended together) supersedes ADR-0043 with a corrected doctrine attribution.
- **M-4 — listener-manager `Stop()`/`Listeners()` race.** *Defer.* Phase 05 does not touch `internal/listener/manager.go`'s lock surface (the only listener-manager change in phase 05 is passing `listenerCtx` into `hcm.NewFilter`, which is a build-time path; runtime Stop/Listeners is unchanged). The race is inherited from phase 03's M-2 carry-forward and remains unresolved. ADR-X carries the deferral; phase 08's admin-api-and-drain phase is the natural place to close this (drain semantics require a correct Listeners() snapshot).
- **M-5 — phase-04 SPEC §7 failure-mode "close upstream" prose vs `defer upstreamConn.Close()` mechanism.** *Defer (cosmetic).* Phase 05 introduces a parallel mechanism on the H2 path (`defer clientConn.Close()` in `routerActionH2.do`) and reuses the same prose-vs-mechanism shape. The cosmetic gap remains; ADR-X carries forward. Documentation cleanup is bundled into a future SPEC-corrections ADR.
- **M-6 — fixture-0003 driver's heredoc YAML pattern.** *Defer.* Phase 05's fixture 0004 follows the same heredoc pattern (per §4.3). The structured-`expectations.yaml` plan from ADR-0019 still belongs to the observability / phase-06 sweep; phase 05 holds the line. ADR-X carries forward.
- **M-7 — `Filter.statPrefix` stored but never consumed.** *Defer with phase-06-must-consume tag.* Phase 05 does not consume `Filter.statPrefix` either (no stats subsystem yet). The phase-04 `stat_prefix` storage shape is unchanged. ADR-X carries the forward-looking note; phase 06's brainstorm is required to either honour `Filter.statPrefix` (lifting M-7 to resolved) or supersede ADR-N with a stat-naming policy that obviates the field. Phase 05 plans `PLAN.md` will include a SPEC-noted "phase-06-must-consume" tag in the carryover-list section.

ADR-X (Phase-04 REVIEW Minor carry-forward triage) is the formal landing for these dispositions.

Additionally, REVIEW.md surfaced (as the "single most important context to surface to the phase-05 planner") that **fixture-0003 still does not differentially exercise upstream TLS** (per ADR-0035 carry-forward). Phase 05 closes this gap *for the H2 leg* via fixture 0004's full-stack HTTPS h2 (ADR-W). The H1+TLS leg remains open after phase 05; ADR-W carries forward an explicit "phase-05-follow-up" tag pointing at a future HTTPS-H1 fixture (or an extension of fixture 0003 to TLS upstream). The follow-up may land between phase 05 and phase 06, or be folded into phase 07's filter-chain framework, or stay open into HTTP-filter-family phases — the planner does not pre-decide here.

## 13. Acceptance checklist (for the reviewer of this phase's final state)

A reviewer (phase 05's `superpowers:requesting-code-review` subagent) signs off when every item below is verifiable from the on-disk state:

- [ ] All six phase-done gates (a–f) green per §3.
- [ ] `internal/filter/hcm/h2/` package contains the from-scratch ServerConn + serverStream + ClientConn implementations; no `http2.Server` or `http2.Transport` runtime is used by envoy-go runtime code (driver-side use in `cmd/envoy-go/main_test.go`, `test/fixtures/0004-h2-routing/driver/`, `test/conformance/h2spec/`, and `test/helpers/h2.go` is permitted and grep-verifiable).
- [ ] `internal/cluster/dial_h2.go` exists; `Cluster.DialH2` is the only API for upstream H2 dial; the manager validates the H2-cluster invariants (TLS + ALPN h2) at build time.
- [ ] `Filter.Handle` ALPN dispatch is grep-verifiable (one `*stdtls.Conn` type-assert + one `NegotiatedProtocol` read + one branch on `"h2"`).
- [ ] `BEHAVIOR_CONTRACT.md` carries a new `## HTTP/2` subsection with all five subheadings (Asserted equivalence; Not asserted; Header allow-list extensions; h2spec threshold; Applies to / Does not yet apply to). Header allow-list table at the top of the file has rows for `:status`/`:method`/`:path`/`:scheme`/`:authority` with phase-05 + ADR-T provenance.
- [ ] `CONFORMANCE_PINS.md` exists and pins `summerwind/h2spec` by tag + SHA256 with a refresh procedure.
- [ ] All ADRs referenced in §4.4 (ADR-P..ADR-Z, materialised at planner-assigned numbers) appear in `DECISIONS.md` with full Context/Decision/Consequences sections per ADR-0001's template.
- [ ] Fixture 0004's PKI (CA + leafs) is committed under `test/fixtures/0004-h2-routing/pki/`; the PKI generator is committed under `pki/gen/`.
- [ ] `test/conformance/h2spec/h2spec_test.go` exists, is excluded from `go test -short`, and reports `failed == 0` over the threshold sections when run unrestricted.
- [ ] No phase-04 fixture (`0000`/`0001`/`0002`/`0003`) regressed under the unrestricted `go test ./test/differential/...` run.
- [ ] `cmd/envoy-go/main_test.go` carries an h2-bootstrap variant exercising the binary's H2 listener path.
- [ ] `STATE.md` is at lifecycle-state 6 (or appropriate end state); `ROADMAP.md` row 05 is `done`; the §5.3 phase-done commit's message names every ADR introduced or referenced.
- [ ] `PROGRESS.md` quotes the command outputs of all six gates per the §5.3 verification protocol; SHA-fill for each task entry per the phase-04 convention.
- [ ] The phase-04 REVIEW Minor disposition (§12) is faithfully recorded in ADR-X with no silent re-disposition.

When all boxes above are checked, phase 05 is `done` and the project advances to phase 06 (observability-baseline) at lifecycle-state 1.
