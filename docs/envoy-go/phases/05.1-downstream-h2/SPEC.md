# Phase 05.1 — Downstream HTTP/2 (server-side codec, ALPN dispatch, h2spec conformance)

**Phase id:** `05.1`
**Slug:** `05.1-downstream-h2`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:brainstorming` (adapted autonomous mode — see `DECISIONS.md` ADR-0004)
**Depends on:** phase 04 (done)
**Parent phase:** `05-http-2` (in-progress; split into `05.1` + `05.2` per ADR-0045)
**Master design document:** `docs/envoy-go/phases/05-http-2/SPEC.md` (commit `612cdea`) — phase 05.1 carves a coherent slice of its §4 deliverables; the master SPEC is authoritative for cross-cutting design (codec choice rationale, RFC compliance, equivalence shape).
**Differential surface at end of sub-phase:** No new differential fixture lands in 05.1 (gate (a) is *vacuously* green per `BOOTSTRAP_PROMPT.md` §7.5 — fixture `0004-h2-routing` is 05.2's deliverable). Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing` remain green with no behavioural regression (gate (b)). The project's first non-vacuous **conformance** gate (c) lands here: `summerwind/h2spec` runs against a `--allow-h2c` h2c listener and reports `failed == 0` over the threshold sections declared in `BEHAVIOR_CONTRACT.md`'s new `## HTTP/2` subsection (sections 3, 4, 5, 6 ex-6.6, 7, 8 — see ADR-U). New short-budget fuzz targets `FuzzFrameStream` and `FuzzHPACKDecode` run clean (gate (d)). Standard build/vet/lint/test cleanliness (gate (e)). REVIEW.md approved (gate (f)).

---

## 1. Purpose

Phase 05.1 delivers envoy-go's downstream HTTP/2 dataplane: the listener accepts TLS connections that negotiate ALPN `h2` (a capability already wired by phase 03's `alpn_protocols` plumbing), the HCM filter dispatches the post-handshake conn into a from-scratch HTTP/2 server-side codec under `internal/filter/hcm/h2/`, the codec drives `golang.org/x/net/http2.Framer` + `golang.org/x/net/http2/hpack` as low-level codec surfaces (per doctrine `D-3.2`), the per-connection state machine demuxes HEADERS/DATA/RST_STREAM/SETTINGS/PING/WINDOW_UPDATE/GOAWAY into per-stream request/response scopes, each stream runs the same degenerate `[router]` HTTP-filter chain that phase 04 introduced (per ADR-0042), and the only route action exercised on the H2 path in 05.1 is `direct_response` — codec-neutralised so the same `directResponseAction` value backs both the H1 and H2 wire writers (§5.5).

Concretely, phase 05.1 produces:

1. An HTTP/2 server-side codec under `internal/filter/hcm/h2/` (sub-package of the phase-04 HCM package) that owns one downstream H2 connection at a time. It performs the connection preface check (`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n` per RFC 9113 §3.4), reads/writes frames via `http2.Framer`, runs SETTINGS handshake (server-initial SETTINGS + client-initial SETTINGS ack), maintains per-connection send/recv flow-control windows, demuxes incoming frames into per-stream state machines, runs HEADERS encode/decode through `hpack.Encoder`/`hpack.Decoder`, and emits GOAWAY on graceful shutdown or unrecoverable connection-level errors. Files: `errors.go`, `preface.go`, `framer.go`, `hpack.go`, `settings.go`, `flow.go`, `stream.go`, `conn.go`. **`client.go` is explicitly NOT in 05.1** — `ClientConn` is 05.2's deliverable.
2. A from-scratch per-stream **server-side** state machine (`stream.go`) implementing the RFC 9113 §5.1 Idle → Open → Half-closed (remote/local) → Closed lifecycle. Each stream's request scope (request headers, request body reader, response writer, request trailers reader) is constructed on receipt of HEADERS, dispatched through the route table on END_STREAM (phase 05.1 keeps the simpler "wait for END_STREAM before dispatching" form per master SPEC §10.1; full streaming-body filter dispatch lands with the phase-07 framework), and torn down on `closed`. Server-side stream IDs are odd-numbered client-initiated per RFC 9113 §5.1.1; the codec rejects even-numbered client-allocated IDs as PROTOCOL_ERROR.
3. An ALPN-driven codec dispatcher under `internal/filter/hcm/filter.go`: `Filter.Handle(ctx, downstream)` inspects `downstream.(*stdtls.Conn).ConnectionState().NegotiatedProtocol` after the TLS handshake completes (the listener's pre-existing TLS handshake from phase 03 is unchanged); if `"h2"`, dispatches to the new H2 connection driver; if `"http/1.1"` or empty, dispatches to the phase-04 H1 connection driver unchanged. Plaintext (non-TLS) listeners continue to dispatch to the H1 driver. The `--allow-h2c` test-only flag (ADR-Z, 05.1) bypasses the TLS-required check at `Filter.Handle` invocation time so that h2spec can drive h2c traffic through the codec; the flag is present only when `cmd/envoy-go` is invoked with it.
4. A small extension to `internal/filter/hcm/config.go`: `codec_type: HTTP2` is now permitted alongside phase-04's `HTTP1` and `AUTO`. `AUTO` continues to mean "the listener picks via ALPN" — i.e. dispatches per-conn at runtime (on TLS conns it reads `NegotiatedProtocol`; on plaintext it falls through to H1 unchanged). `HTTP2` means "this listener accepts only h2"; build-time validation requires the listener carry a `transport_socket: tls` UNLESS `--allow-h2c` is set in the runtime context (the build-time validator consults a `listenerCtx{hasTLS bool, allowH2C bool}` value plumbed in by the listener manager). `HTTP3` continues to error at build (→ phase 09+). The `http2_protocol_options` field on the HCM is silently ignored at phase 05.1 (added to the phase-04 ignored-set per master SPEC §9 and ADR-N's amendment).
5. A codec-neutral factoring of `directResponseAction` (`internal/filter/hcm/actions.go`): the action gains a method `body() (status int, headers http.Header, body []byte)` returning the synthesised reply; an H1 adapter `writeH1(io.Writer)` retains phase-04's exact wire bytes; a NEW H2 adapter `writeH2(streamWriter)` writes HEADERS (`:status` + `Content-Length` + `Content-Type` + `Server` + `Date` per phase-04 ADR-0044) + DATA (body bytes) + END_STREAM. The factoring is required in 05.1 because h2spec's threshold section 8 (HTTP Message Exchanges) exercises basic request-response shapes and `direct_response` is the simplest such shape (per ADR-0045).
6. The project's first conformance suite under `test/conformance/h2spec/`: a Go test driver that boots a phase-05.1 envoy-go subject with a single h2c listener (TLS-less, dedicated to conformance, opt-in via `--allow-h2c`), runs the upstream `summerwind/h2spec` Docker image against it via `testcontainers-go`, parses the structured JUnit-XML output, and asserts `failed == 0` over the SECTION SET declared in `BEHAVIOR_CONTRACT.md`'s new `## HTTP/2` subsection (initial sections: 3 HTTP Frame Format, 4 HPACK, 5 Streams and Multiplexing, 6 Frame Definitions excluding 6.6 PUSH_PROMISE, 7 Error Codes, 8 HTTP Message Exchanges; explicit exclusions documented per ADR-U). The conformance binary is pinned by Docker tag + SHA in a NEW pin file `docs/envoy-go/CONFORMANCE_PINS.md`, sibling to `ENVOY_TARGET.md`, same refresh discipline per D-3.7. Per `BOOTSTRAP_PROMPT.md` §7.5 (c) this gate is **non-vacuous for the first time in the project**.
7. A new `BEHAVIOR_CONTRACT.md` subsection, **HTTP/2** — drafted as a SCAFFOLD in 05.1: it codifies the phase-05.1 codec/conformance scope (asserted: `:status` and decoded-body equivalence on `direct_response` 2xx via h2spec; the structurally-equivalent framing rule from §7.2 of `BOOTSTRAP_PROMPT.md` materialised; not asserted: wire-byte framing) and the h2spec threshold list with exclusions. **Upstream-H2 + fixture-0004 differential rules are NOT added in 05.1** — those land in 05.2 as an extension (per ADR-0045 consequences). The 05.1 subsection explicitly enumerates "Does not yet apply to: routed-to-upstream H2 (deferred to 05.2 + fixture 0004)" so the boundary is auditable.
8. New fuzz targets: `internal/filter/hcm/h2.FuzzFrameStream` over the connection-manager ingest (mutates a sequence of well-formed frames into malformed ones, asserts no panic and that all returned errors begin with `h2:`), and `internal/filter/hcm/h2.FuzzHPACKDecode` over the hpack decoder integration (asserts no panic on adversarial header blocks; the underlying `x/net/http2/hpack` package has its own fuzzer upstream, but a wrapper-level fuzz target catches integration regressions in our usage). Short-budget `-fuzztime=30s` per ADR-0018.
9. Phase-04 REVIEW Minor carry-forward triage (M-2/M-4/M-5/M-6/M-7) lands in 05.1 per ADR-0045 because the dispositions are textual / cosmetic + a forward-looking "phase-06-must-consume" tag and none touches upstream-H2 surface. ADR-X (under 05.1) is the formal landing.

After phase 05.1, the project has proven a half of its sixth central engineering claim: *envoy-go terminates downstream HTTP/2 on a TLS listener — it negotiates ALPN, drives an own framer, demuxes streams through an own connection-manager state machine, and produces structurally-equivalent framing and per-stream behaviourally-equivalent responses to upstream Envoy on the `direct_response` surface, while passing the declared subset of `h2spec` conformance.* The remaining half (upstream H2 origination + a full-stack h2 differential fixture closing ADR-0035's H2 leg) is delivered by phase 05.2.

## 2. Non-purposes

Phase 05.1 does **not** do any of the following. Most are inherited verbatim from the master phase-05 SPEC §2; a few are scope-narrowings introduced by the 05.1/05.2 split per ADR-0045. Each is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

### 2.1 Inherited from master phase-05 SPEC §2 (no change)

- **HTTP/3 / QUIC.** Both HCM `codec_type: HTTP3` and any QUIC transport socket continue to error at build, unchanged from phase 04. → HTTP/3 + QUIC family.
- **HTTP/2 server push (`PUSH_PROMISE`).** Phase 05.1's H2 server NEVER emits `PUSH_PROMISE`; on receipt of one (clients can't legally send these — only servers — so this is a protocol-error case), the connection emits GOAWAY with `PROTOCOL_ERROR`. The server's SETTINGS handshake advertises `SETTINGS_ENABLE_PUSH = 0` to disable push from the client side as well. h2spec section 6.6 is excluded from the conformance threshold per ADR-U.
- **HTTP/2 PRIORITY-driven scheduling.** RFC 9113 deprecated PRIORITY. Phase 05.1 reads PRIORITY frames per RFC 9113 §6.3 and silently discards them. The advertised `SETTINGS_NO_RFC7540_PRIORITIES = 1` informs clients of this.
- **Adaptive flow-control / BDP estimation.** Phase 05.1's flow-control implements the RFC 9113 §5.2 baseline only: hard-coded initial windows (connection-level 65535, per-stream 65535 — both protocol defaults), WINDOW_UPDATE on consumption, no dynamic resizing of `SETTINGS_INITIAL_WINDOW_SIZE` after the handshake.
- **Trailer support (request and response).** Phase 04 set `req.Trailer = nil` after `http.ReadRequest` (stdlib H1 limitation). Phase 05.1's H2 codec *can* observe trailers (HEADERS frames after DATA with END_STREAM set) but the phase-05.1 server-side dispatch DOES NOT forward them: trailers received from the downstream are discarded; the H2 adapter for `direct_response` emits END_STREAM on the response HEADERS or final DATA, never via a trailing HEADERS frame. The fixture driver (05.2's deliverable) does not exercise trailers either. ADR-Y carries the rationale and the deferral; ADR-Y lands in 05.2 because the upstream-side trailer behaviour (what `routerActionH2` does with trailing HEADERS observed on the upstream conn) is part of 05.2's surface — 05.1 only contributes the server-side discard rule. → phase 07 framework + gRPC family.
- **gRPC-specific behaviour.** No `grpc-status` translation, no gRPC-Web bridging, no `grpc-timeout` honouring. The phase-05.1 conformance surface is plain H2 only (h2spec is protocol-level, not gRPC). → gRPC family.
- **0-RTT (TLS 1.3 early data).** crypto/tls supports 0-RTT only via TLS sessions and explicit opt-in; phase-05.1 does not opt in. → later phase if ever.
- **HTTP/1.1 → HTTP/2 upgrade ("h2c upgrade", RFC 7540 §3.2 / RFC 9113 deprecated).** Phase-04's HCM rejects `Upgrade: h2c` request headers with 501 (per phase-04 SPEC §4.1's connection-loop guard). Phase 05.1 does NOT change that. The h2c-prior-knowledge surface (no Upgrade, the client just sends the preface bytes after a plaintext TCP connect) IS used by `h2spec` against the `--allow-h2c` conformance listener but is NOT exposed via any production fixture or production-build code path. → out of scope permanently unless a workload demands it.
- **HCM `tracing`, `access_log[]`, `http_protocol_options` (deprecated direct field), `common_http_protocol_options`, `server_header_transformation`, `local_reply_config`, `internal_redirect_policy`, `request_id_extension`, `path_with_escaped_slashes_action`, `merge_slashes`, `xff_num_trusted_hops`, `via`, `proxy_100_continue`, `stream_idle_timeout`, `request_timeout`, `request_headers_timeout`, `drain_timeout`, `delayed_close_timeout`, `forward_client_cert_details`, `original_ip_detection_extensions`.** All silently ignored, unchanged from phase 04 (per ADR-N). The newly-ignored field in 05.1: `http2_protocol_options` (the directly-on-HCM field, distinct from the cluster-side typed-extension which is 05.2's surface). Recorded in §9 below.
- **Stats, access logs, tracing, runtime overrides.** All deferred. → phase 06.
- **HTTP filters other than `[router]`.** Unchanged from phase 04 (per ADR-0042). → phase 07.
- **Per-route filter config.** Unchanged from phase 04. → phase 07.
- **Route match predicates beyond `prefix` and `path`.** Unchanged from phase 04 (per ADR-0038). → phase 07.
- **Multi-vhost matching.** Unchanged from phase 04. → phase 07.
- **Filter-chain matching beyond ALPN-derived codec selection.** Phase 03's SNI-keyed filter-chain match plus phase 04's empty-match plaintext rule continue to apply. ALPN is NOT a `filter_chain_match` field at phase 05.1 — codec selection happens *inside* the HCM filter, not at the listener-side filter-chain match step (per ADR-V). The `filter_chain_match.application_protocols[]` field is silently ignored if present (extending the phase-04 ignored set). → phase 07.
- **Cluster types other than STATIC (subject side).** Unchanged from phase 02. → later phase.
- **LB policies other than ROUND_ROBIN.** Unchanged. → load-balancing family.
- **Graceful drain of in-flight HTTP/2 requests.** SIGINT behaviour unchanged from phase 04: listener sockets close, in-flight conns drop. The H2 connection manager does NOT emit a graceful GOAWAY with `last-stream-id` followed by an idle drain on shutdown; it just closes. → phase 08.
- **HTTP/2 max-concurrent-streams enforcement.** Phase 05.1 advertises `SETTINGS_MAX_CONCURRENT_STREAMS = 100` (Envoy's documented default) on the server side. If a client opens more than 100 streams concurrently, the server emits `RST_STREAM` with `REFUSED_STREAM` on the excess streams (RFC 9113 §5.1.2 mandate). h2spec section 5 exercises this code path; the unit tests cover it directly. There is no fixture exercise in 05.1 (no fixture lands in 05.1).

### 2.2 Narrowings introduced by the 05.1 / 05.2 split (per ADR-0045)

These deliverables ARE part of the master phase-05 SPEC but are **explicitly deferred to 05.2**. They are listed here so the 05.1/05.2 boundary is auditable and the planner does not implement out-of-scope work:

- **Upstream HTTP/2 origination (`internal/filter/hcm/h2/client.go`).** The from-scratch `ClientConn` + `RoundTrip` lives in 05.2. 05.1's `h2/` sub-package contains NO `client.go` file; the package compiles and is unit-tested in 05.1 with server-side surfaces only.
- **Upstream H2 dial helper (`internal/cluster/dial_h2.go`) and `Cluster.UseH2()` accessor.** Both 05.2 deliverables. 05.1's `internal/cluster/` is unchanged from phase 04 except for the blank import noted below in §4.2.
- **Cluster `HttpProtocolOptions` parsing in `internal/cluster/manager.go`.** 05.2 deliverable. 05.1 does NOT introduce the `typed_extension_protocol_options` reader. Configs that set `HttpProtocolOptions` continue to be silently ignored (the field falls into the phase-04 silent-ignore set per ADR-N) — which is the correct behaviour for 05.1, where no cluster needs to be H2-capable on the upstream side.
- **`routerActionH2` variant in `internal/filter/hcm/actions.go`.** 05.2 deliverable. 05.1's `actions.go` introduces only the codec-neutral `directResponseAction` factoring (§5.5) and leaves the existing phase-04 `routerAction` unchanged. **In 05.1 the `routerAction` action type is functionally unreachable from production H2 listeners** — see §4.2's actions.go bullet and §5.2 step 4c for the exact mechanics: there is **no build-time guard** in 05.1 (the route-table builder cannot check H2 capability because `Cluster.UseH2()` does not exist yet — that accessor lands in 05.2 with `HttpProtocolOptions` parsing); instead, an H2 stream that resolves to a `routerAction` produces a runtime per-stream INTERNAL_ERROR + RST_STREAM (the protective shape per §5.2 step 4c). The unreachability in production is structural rather than enforced: the only 05.1 bootstraps that select the H2 codec path are the h2spec conformance bootstrap and the `main_test.go` h2 smoke bootstrap, both `direct_response`-only by construction.
- **Differential fixture `0004-h2-routing/`.** 05.2 deliverable. 05.1's gate (a) is vacuously green per `BOOTSTRAP_PROMPT.md` §7.5.
- **`test/helpers/h2.go` H2RoundTrip helper.** 05.2 deliverable. 05.1's conformance suite uses h2spec's own client (a Docker container); no first-party H2 client helper is needed in 05.1.
- **Blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` in `internal/cluster/cluster.go`.** 05.2 deliverable (the proto's `HttpProtocolOptions` is consumed only when 05.2's manager reader lands). 05.1 adds NO blank import to `internal/cluster/`.
- **`BEHAVIOR_CONTRACT.md ## HTTP/2` upstream + fixture-0004 rules.** 05.2 extends 05.1's scaffold with: (a) routed-to-upstream H2 request preservation (verbatim `:method`/`:path`/`:scheme`/`:authority` forwarding), (b) per-cluster RR distribution rules on H2, (c) closes ADR-0035's H2 leg via fixture 0004's full-stack HTTPS h2. 05.1's scaffold explicitly enumerates these as "does not yet apply".
- **ADRs deferred to 05.2:** ADR-R (per-request fresh upstream H2 dial; mirrors ADR-0039), ADR-W (closes ADR-0035 H2 leg), ADR-Y (trailers — see §2.1 above for why trailer scope partially lives in 05.1 but the formal ADR lands with the upstream surface in 05.2).

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for 05.1)

Per doctrine `D-3.6`, phase 05.1 lands only when every gate below is green. The generic six-gate set is narrowed:

| Gate | Specialization for phase 05.1 |
|---|---|
| (a) new/changed differential fixtures green | **Vacuously green.** No new differential fixture lands in 05.1; fixture `0004-h2-routing` is 05.2's deliverable. The verifier (`superpowers:verification-before-completion`) records this as "vacuous — no new fixture in this sub-phase per ADR-0045". |
| (b) all pre-existing differential fixtures still green | `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing` all pass without regression under their existing `expectations.yaml`. Phase-04's HCM H1 path is NOT touched by 05.1's changes (the H1 driver remains intact; 05.1's changes are additive — `Filter.Handle`'s ALPN dispatch falls through to the H1 driver on non-h2 connections). The `directResponseAction` codec-neutral factoring (§5.5) preserves H1 wire bytes byte-for-byte; the H1 adapter is the same `writeStatusReply`/equivalent code paths from phase 04 (refactored, not rewritten). Verified by re-running fixture 0003 + the unit-test set under `internal/filter/hcm/`. |
| (c) conformance suites pass | `test/conformance/h2spec/` runs the upstream `summerwind/h2spec` image (pinned in `docs/envoy-go/CONFORMANCE_PINS.md`, NEW in 05.1) against a dedicated phase-05.1 h2c listener (subject is started with `--allow-h2c`) and reports `failed == 0` over the threshold section list (3, 4, 5, 6 ex-6.6, 7, 8); see ADR-U for the exact section list and exclusions. **THIS GATE IS NEWLY NON-VACUOUS** — it was vacuously green in phases 00–04. |
| (d) new fuzzer runs clean for CI short-budget | New fuzz targets `internal/filter/hcm/h2.FuzzFrameStream` and `internal/filter/hcm/h2.FuzzHPACKDecode` run clean for their CI short-budget runs (30-second policy inherited from ADR-0018). Phase-01 `internal/bootstrap.FuzzBootstrapLoad`, phase-02 `internal/filter/tcpproxy.FuzzTcpProxyFilter`, phase-03 `internal/tls.FuzzTLSContextParse`, and phase-04 `internal/filter/hcm.FuzzHCMConfigParse` also run clean (no regression). |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | Standard. Unit tests for `internal/filter/hcm/h2/` (codec, framer, conn manager, server-side stream state machine, hpack roundtrip, settings handshake, flow control, error codes, GOAWAY, RST_STREAM, PING) plus extended tests for `internal/filter/hcm/` (ALPN dispatch, `codec_type=HTTP2` build, codec-neutral `direct_response` factoring) plus extended tests for `cmd/envoy-go/main_test.go` (h2-bootstrap variant smoke test) all part of `go test ./...`. **`internal/cluster/` test surface is unchanged from phase 04** in 05.1 (no DialH2, no UseH2). |
| (f) `REVIEW.md` approved | Per `SKILL_ROUTING.md` state 5. |

## 4. Deliverables (files and directories)

Grouped by lifecycle. Every path below is either new or materially changed in 05.1. Paths explicitly marked **NOT in 05.1** are 05.2 deliverables that the master phase-05 SPEC §4 lists; they are repeated here only to make the 05.1/05.2 boundary unambiguous to the reviewer.

### 4.1 New production code (in 05.1)

- **`internal/filter/hcm/h2/conn.go`** — exposes `ServerConn` (per-downstream-conn server-side H2 connection manager; one per downstream `*stdtls.Conn` after ALPN selects `h2`, or per `net.Conn` when invoked through the `--allow-h2c` h2c path). `NewServerConn(ctx, downstream net.Conn, table *hcm.RouteTable, settings ServerSettings) *ServerConn`. `(*ServerConn).Run() error` — the connection loop: writes server preface (server-initial SETTINGS), reads client preface (`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n` then client-initial SETTINGS), processes incoming frames, dispatches HEADERS-on-new-stream into a fresh per-stream goroutine that runs the route table → action sequence. Returns `nil` on clean shutdown (downstream closed gracefully or GOAWAY exchanged), an `h2:`-prefixed error on protocol violations (the caller — `Filter.Handle` — drops the error per the phase-02 `_ = io.Copy` precedent). The `ServerSettings` struct carries the hardcoded defaults (`MaxConcurrentStreams=100`, `InitialWindowSize=65535`, `MaxFrameSize=16384`, `EnablePush=0`, `NoRFC7540Priorities=1`, `HeaderTableSize=4096`); 05.1 does not vary these per-config, but the struct exists so phase 06+ tests can.
- **`internal/filter/hcm/h2/stream.go`** — per-stream state machine (server-side only in 05.1). `serverStream` carries: stream ID, current state (idle/open/halfClosedRemote/halfClosedLocal/closed), per-stream send/recv windows, decoded request headers, request body pipe (an `io.Pipe` writer fed by DATA frames; the route-table action consumes the reader end), END_STREAM flags. Methods: `recvHeaders([]hpack.HeaderField, endStream bool) error`, `recvData([]byte, endStream bool) error`, `recvRSTStream(errCode) error`, `recvWindowUpdate(increment uint32) error`, `dispatch(ctx context.Context, table *hcm.RouteTable) error` (called once on END_STREAM-on-headers OR END_STREAM-on-data; runs the route match + action; the action's response is written back via `sendHeaders`/`sendData`/`sendRSTStream`). The dispatch helper waits for END_STREAM before invoking the action per master SPEC §10 #1 (the planner records the choice in PLAN.md per ADR-0045 consequence "Streaming-body-vs-wait-for-END_STREAM dispatch decided in 05.1").
- **`internal/filter/hcm/h2/framer.go`** — thin wrapper over `http2.Framer` adding context-cancellation on the read side (`http2.Framer.ReadFrame` is blocking and not ctx-aware; the wrapper sets a deadline derived from the ctx and translates `os.IsTimeout`+`ctx.Err() != nil` into a clean cancellation). All write methods are passthrough. Phase 05.1 does NOT use `http2.Framer`'s `WriteRawFrame`; only the high-level write methods (`WriteSettings`, `WriteSettingsAck`, `WriteHeaders`, `WriteData`, `WriteRSTStream`, `WriteWindowUpdate`, `WritePing`, `WriteGoAway`).
- **`internal/filter/hcm/h2/hpack.go`** — pair of `hpack.Encoder` and `hpack.Decoder` with the conn-level state. The encoder writes into a per-frame buffer; the decoder feeds emitted fields into a slice that the stream state machine consumes. Header table size: the decoder side is fixed at 4096 octets (RFC 9113 default + Envoy default; we don't advertise a different value in our SETTINGS, so the encoder side at the peer is also 4096). `SETTINGS_HEADER_TABLE_SIZE` received from the client is honoured by passing it to the encoder side so our outgoing header tables shrink/grow as the client requests.
- **`internal/filter/hcm/h2/flow.go`** — flow-control helpers. Connection-level send window (decrements on DATA send, blocks when ≤ 0 until WINDOW_UPDATE arrives), per-stream send window (likewise), connection-level recv window (decrements on DATA recv, increments + WINDOW_UPDATE emit on consumption), per-stream recv window (likewise). Implemented with channels + a small mutex; 05.1 keeps it minimal and correct, not optimised.
- **`internal/filter/hcm/h2/preface.go`** — server-side preface check: read 24 bytes, compare against `[]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")`. Mismatch → connection-level error (`h2: bad preface`). No client-side preface emitter in 05.1 (that's `client.go`, deferred to 05.2).
- **`internal/filter/hcm/h2/settings.go`** — settings handshake helpers. `writeServerInitialSettings(fr *http2.Framer, s ServerSettings) error` writes a SETTINGS frame with the phase-05.1 default settings; `readClientSettings(fr *http2.Framer, applyTo *clientSettings) error` reads one SETTINGS frame from the client and applies the values; the conn driver issues SETTINGS_ACK on receipt of client SETTINGS and expects a SETTINGS_ACK in response to its own.
- **`internal/filter/hcm/h2/errors.go`** — small enum of H2 error codes (`NO_ERROR`, `PROTOCOL_ERROR`, `INTERNAL_ERROR`, `FLOW_CONTROL_ERROR`, `STREAM_CLOSED`, `FRAME_SIZE_ERROR`, `REFUSED_STREAM`, `CANCEL`, `COMPRESSION_ERROR`, `CONNECT_ERROR`, `ENHANCE_YOUR_CALM`, `INADEQUATE_SECURITY`, `HTTP_1_1_REQUIRED`) mapped to RFC 9113 §7 numeric codes. All errors returned by H2 internals are `*h2.Error{Code, Stream, Underlying}` so the top-level connection driver can dispatch RST_STREAM (stream-scoped) vs GOAWAY (conn-scoped). Error message convention: every `Error()` string starts with `h2:` so fuzzers and unit tests can grep for it.
- **`internal/filter/hcm/h2/<test files>`** — exhaustive unit tests covering, at minimum: server preface check (good + bad); SETTINGS handshake (server initial + ack + client initial + ack ordering); HEADERS encode + decode roundtrip (request and response — server-side only in 05.1); DATA framing (single + multi + END_STREAM); RST_STREAM (server-side emit on stream-scoped error; client-side recv); WINDOW_UPDATE (recv increments send window; recv DATA decrements recv window; emit WINDOW_UPDATE on consumption); per-stream server-side state machine transitions (idle→open→half-closed-remote→closed and idle→open→half-closed-local→closed); GOAWAY emit on graceful shutdown and on protocol error; PING + PING-ACK; MAX_CONCURRENT_STREAMS enforcement (101st concurrent stream → REFUSED_STREAM); flow-control correctness (small windows + multiple DATA frames + WINDOW_UPDATE → eventual full delivery); HPACK dynamic table size update propagation (SETTINGS_HEADER_TABLE_SIZE shrink mid-conn → next outgoing HEADERS respects new size); PUSH_PROMISE on the server-receive path → GOAWAY/PROTOCOL_ERROR; PRIORITY frame received → silently discarded (no state change); even-numbered client-initiated stream id → PROTOCOL_ERROR; stream id reuse → PROTOCOL_ERROR; bad preface bytes → connection error. Stdlib `golang.org/x/net/http2.Transport` is used as the test peer for client-side scenarios (driver-side use of x/net/http2.Transport is permitted because the test is fixture infrastructure, not envoy-go runtime — D-3.2 governs runtime, not test code). Unit-test layout (single file vs split per topic) is a planner choice; the SPEC does not prescribe.
- **`internal/filter/hcm/h2/fuzz_test.go`** — `FuzzFrameStream` (mutates a corpus of well-formed frame sequences and asserts no panic + all errors begin with `h2:`); `FuzzHPACKDecode` (adversarial header-block bytes through the conn-level decoder).
- **`test/conformance/h2spec/h2spec_test.go`** — Go test driver. Spins up a `summerwind/h2spec` container via `testcontainers-go` pinned by tag + SHA from `docs/envoy-go/CONFORMANCE_PINS.md`. Boots a phase-05.1 envoy-go subject with a synthetic h2c-listener bootstrap on a host port (subject invoked with `--allow-h2c`). Runs h2spec against the listener, parses the JUnit-XML output (`--junit-report=/tmp/h2spec.xml`), asserts `failed == 0` over the threshold-section subset declared in `BEHAVIOR_CONTRACT.md`'s `## HTTP/2` subsection. Runtime budget: ~30s wall-clock for the full h2spec run; the CI gate is `go test ./test/conformance/h2spec/...`. The test is excluded from `-short` (the planner wires `t.Skip("h2spec is not -short")` on `testing.Short()`).
- **`test/conformance/h2spec/h2spec.go`** — small helper, no `_test.go` suffix. Defines the threshold section list as a Go slice of strings (`[]string{"3", "4", "5", "6 (ex 6.6)", "7", "8"}` or equivalent — the exact representation matches the JUnit-XML section identifiers h2spec emits; the planner picks). The slice is the canonical reference; `BEHAVIOR_CONTRACT.md`'s `## HTTP/2` subsection is the human-readable narrative form of this slice.

### 4.2 Changed production code (in 05.1)

- **`internal/listener/manager.go`** — minimal extension: when constructing an HCM filter, the listener-side build now passes a `listenerCtx{hasTLS: tc != nil, allowH2C bool}` value into `hcm.NewFilter` so HCM can validate `codec_type` vs transport semantics at build time. The `allowH2C` field reflects the runtime CLI flag plumbed in from `cmd/envoy-go/main.go` via the listener-manager constructor. No filter-type-URL registry change — HCM is already registered (phase 04). The listener's `transport_socket` plumbing for ALPN was completed in phase 03; 05.1 adds NO listener-side ALPN handling beyond what phase 03 already does (the negotiated ALPN is observed by HCM after the handshake, not by the listener).
- **`internal/listener/manager_test.go`** — extended: HCM with `codec_type: HTTP2` on a plaintext listener WITHOUT `--allow-h2c` errors at build with `listener: ...: hcm: codec_type HTTP2 requires TLS transport_socket (or --allow-h2c)`; HCM with `codec_type: HTTP2` on a plaintext listener WITH `--allow-h2c` builds successfully (the conformance test exercises this); HCM with `codec_type: AUTO` on a TLS listener with `alpn_protocols: ["h2", "http/1.1"]` builds successfully.
- **`internal/filter/hcm/filter.go`** — `Filter.Handle(ctx, downstream)` learns ALPN dispatch (per ADR-V). The new dispatch logic: type-assert downstream to `*stdtls.Conn`; if assertion succeeds, read `ConnectionState().NegotiatedProtocol`; if `"h2"`, dispatch to `h2.ServerConn`; else (including `"http/1.1"` or `""`), dispatch to the existing phase-04 H1 `runConnection`. If the assertion fails (plaintext listener), dispatch to the H1 driver UNLESS the build-time `codec_type` was `HTTP2` (which only succeeds at build time when `allowH2C=true`), in which case dispatch directly to `h2.ServerConn` — this is the h2c conformance path. Build-time validation in `hcm.NewFilter`: `codec_type: HTTP2` rejects plaintext listeners unless `listenerCtx.allowH2C` is set; `codec_type: AUTO` accepts either; `codec_type: HTTP1` accepts either (unchanged).
- **`internal/filter/hcm/config.go`** — `codec_type: HTTP2` is now permitted; `codec_type: AUTO` continues to be permitted (phase-04 maps AUTO→HTTP1; phase-05.1 redefines AUTO→ALPN-driven, which on a non-TLS listener still resolves to HTTP1, mirroring upstream Envoy's documented default). HTTP3 still errors. Additional silent-ignore: `http2_protocol_options` (the directly-on-HCM field; distinct from cluster-side `HttpProtocolOptions` which remains in the phase-04 silent-ignore set since 05.1 does not parse it).
- **`internal/filter/hcm/actions.go`** — codec-neutral factoring of `directResponseAction` per master SPEC §5.5 and ADR-0045. The action gains `body() (status int, headers http.Header, body []byte)` returning the synthesised reply. `writeH1(w io.Writer)` is the existing phase-04 wire-bytes writer (extracted from `directResponseAction.do`). `writeH2(streamWriter)` is NEW: writes a single HEADERS frame carrying the pseudo-header `:status` followed by the same regular headers as `writeH1` (`Content-Length`, `Content-Type`, `Server`, `Date` — pseudo-headers MUST appear before regular headers per RFC 9113 §8.3) plus a single DATA frame with the body bytes and END_STREAM. The phase-04 `routerAction` is **unchanged** in 05.1 (no H2 variant lands here). The route-table builder gains a guard: a `route` action whose target cluster does not advertise H2 capability is still permitted (existing phase-04 behaviour) for H1 listeners; on an `HTTP2` listener (or AUTO listener that selected H2 at runtime), a stream that resolves to a `routerAction` produces a per-stream INTERNAL_ERROR + RST_STREAM in 05.1 because no H2-capable cluster yet exists. **In practice this code path is unreachable in 05.1**: the only production fixture configurations with `codec_type: HTTP2` or `codec_type: AUTO` carrying h2 ALPN don't land until 05.2's fixture 0004; the only 05.1 H2 listener bootstraps are the conformance-suite synthetic h2c bootstrap (which uses `direct_response` only) and `cmd/envoy-go/main_test.go`'s h2-bootstrap variant (which also uses `direct_response` only). The unreachability is asserted by the unit test suite's coverage of `Filter.Handle` paths (no H2-route-action test case is written; the missing-coverage is itself the boundary witness, documented in PLAN.md).
- **`internal/filter/hcm/connection.go`** (the H1 driver) — UNCHANGED in 05.1. Phase-04's H1 connection loop is the H1 dispatch target of `Filter.Handle`'s ALPN switch; the H2 dispatch target is the new `h2.ServerConn`. The codec-neutral `directResponseAction` factoring keeps this file's call into `directResponseAction.writeH1` byte-for-byte equivalent to its phase-04 call into `directResponseAction.do` (the rename is mechanical).
- **`cmd/envoy-go/main.go`** — adds the `--allow-h2c` CLI flag (per ADR-Z). Default OFF. When ON, plumbed into the listener manager constructor so build-time validation of `codec_type: HTTP2` on plaintext listeners succeeds. The flag is documented in `--help` output but flagged as "test-only; not for production". 05.1 does not strip the flag in production builds — the phase-08 admin/drain phase or a future doctrine-cleanup phase may add a `//go:build !production` guard if needed; 05.1 considers the flag's runtime cost (one `if !flag` branch in `Filter.Handle`) negligible.
- **`cmd/envoy-go/main_test.go`** — extended with a new bootstrap variant: a TLS listener with `alpn_protocols: ["h2"]` and HCM `codec_type: HTTP2`, one direct_response route. Asserts the binary serves the configured response on a `localhost` HTTP/2-over-TLS client probe (the test uses a self-signed cert and `golang.org/x/net/http2.Transport` with `InsecureSkipVerify: true` — driver-side use of x/net/http2.Transport is permitted; runtime is not). The h2c variant (subject started with `--allow-h2c`) is exercised separately by the conformance suite under `test/conformance/h2spec/`; a duplicate h2c smoke test in `main_test.go` is not warranted.
- **`internal/cluster/cluster.go`** — UNCHANGED in 05.1 (no `UseH2()` accessor, no `DialH2`, no blank import). All cluster-side H2 changes are deferred to 05.2 per ADR-0045.
- **`internal/bootstrap/bootstrap.go`** — UNCHANGED in 05.1 (no blank import for `upstreams/http/v3`; that import lands with 05.2's `HttpProtocolOptions` reader).
- **`internal/filter/tcpproxy/`** — unchanged.
- **`internal/tls/`** — unchanged. The phase-03 `alpn_protocols` plumbing already covers what 05.1 needs on the listener side.

### 4.3 New harness and conformance code (in 05.1)

- **`test/conformance/h2spec/h2spec_test.go`** + **`test/conformance/h2spec/h2spec.go`** — see §4.1.
- **`test/differential/`** — UNCHANGED in 05.1. No new fixture, no driver-runner change. (The `runner_test.go` blank-import for fixture 0004 is 05.2's deliverable.)
- **`test/helpers/h2.go`** — NOT in 05.1 (05.2 deliverable).

### 4.4 Changed documentation and state (in 05.1)

- **`docs/envoy-go/ROADMAP.md`** — row 05.1: `status: planned → in-progress`. Per the corrected pattern from phase 05's commit `8d18320` (`BOOTSTRAP_PROMPT.md` §4.1 invariant 3, also recorded in STATE.md `c940928`'s lifecycle-state-1 line), the flip to `in-progress` happens at the SAME COMMIT as this SPEC lands — i.e., at the directory-already-exists time, not at state-2 entry. Row 05.1 transitions to `done` at the 05.1 phase-done commit (after PLAN, IMPL, VERIFY, REVIEW). Row 05 (the parent) stays `in-progress`; it transitions to `done` only when both 05.1 AND 05.2 are `done`. Row 05.2 stays `planned` until 05.1 lands `done`.
- **`docs/envoy-go/STATE.md`** — updated at each lifecycle transition (SPEC drafted = state 2 candidate; PLAN written = state 3; impl complete = state 4; verified = state 5; reviewed = state 6 → 05.2 entry).
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — add new subsection **HTTP/2** (SCAFFOLD form per master SPEC §5.7 and ADR-0045) covering: (a) per-stream `:status` equivalence per request — **scoped in 05.1 to `direct_response` paths exercised via h2spec section 8** (no differential fixture in 05.1; the equivalence shape is asserted indirectly through h2spec's own conformance assertions); (b) decoded-body equivalence on `direct_response` 2xx paths — same scope as (a); (c) header set-equality modulo allow-list — **05.1 inherits the phase-04 allow-list and adds the H2 pseudo-header rows** (`:status` always required + asserted on locally-generated H2 responses; the four request pseudo-headers `:method`/`:path`/`:scheme`/`:authority` are NOT yet asserted because the routed-to-upstream path is 05.2's surface; the row is added with phase-05.1 + ADR-T provenance and a "applies-to: 05.2 routed-to-upstream" forward-looking note); (d) structurally-equivalent framing rule (frame *types* and *order on equivalent events* match; frame *byte-equivalence* NOT asserted; the §7.2 row materialised — but the harness rule that materialises this is the h2spec runner, not a differential fixture in 05.1); (e) per-stream-not-per-conn flow control NOT asserted; (f) ALPN-driven codec selection equivalence — DEFERRED TO 05.2 (no fixture in 05.1 exercises the ALPN path differentially; the h2spec runner uses h2c which has no ALPN); (g) h2spec threshold list and exclusions; (h) does-not-yet-apply-to enumeration — `## HTTP/2` 05.1 scaffold's "does-not-yet-apply" includes: routed-to-upstream H2 (05.2), HTTP/3, server push, trailers, gRPC, h2c production fixtures, mTLS over h2, fixture-0004 (05.2). The 05.2 brainstorming session edits this subsection in place to flip the deferred items to "applies-to" as fixture 0004 lands.
- **`docs/envoy-go/CONFORMANCE_PINS.md`** — NEW file. Pins `summerwind/h2spec` by tag + SHA256, mirroring `ENVOY_TARGET.md`'s discipline. Refresh procedure: run h2spec at the candidate version against the subject; investigate any new failures; either fix envoy-go or extend `BEHAVIOR_CONTRACT.md`'s threshold list (with an ADR superseding ADR-U); update the pin; commit. Per D-3.7 the pin is changed only via a dedicated phase or sub-phase. The file format mirrors `ENVOY_TARGET.md`: a header explaining the file's purpose, a "Pins" section with tag + SHA256 + the date observed, a "Refresh procedure" section, and a "Cross-references" section (links to ADR-U, BEHAVIOR_CONTRACT.md `## HTTP/2`, and `test/conformance/h2spec/`).
- **`docs/envoy-go/DECISIONS.md`** — new ADRs introduced by phase 05.1 (numbers assigned at planner write time per ADR-0004's autonomous-numbering rule). Per ADR-0045 consequence "ADR numbering shift": after ADR-0045's split-decision ADR, the next-free is ADR-0046; 05.1's eight ADRs (P, Q, S, T, U, V, Z, X) → ADR-0046..ADR-0053; 05.2's three ADRs (R, W, Y) → ADR-0054..ADR-0056 when its planner writes them. The planner re-verifies next-free at write time.

  Anticipated 05.1 ADRs:

  - **ADR-P** (codec source): HTTP/2 codec source — `golang.org/x/net/http2.Framer` + `golang.org/x/net/http2/hpack`. Options considered: (P1) handcrafted RFC 9113 framer + handcrafted HPACK (highest control, highest cost — HPACK alone is a non-trivial dynamic-table state machine, and getting it wrong is a security issue per CVE history); (P2) x/net/http2 sub-packages used as low-level codec only (this SPEC's choice); (P3) build on `http2.Server` / `http2.Transport` (FORBIDDEN by D-3.2 — these are server/transport runtimes, not codecs). (P2) keeps the doctrine intent (own connection manager, own state machine, own dispatch) while sidestepping the (P1) cost-of-correctness tax on HPACK. Documents the residual surface that x/net/http2 owns vs what envoy-go owns: x/net owns frame byte-layout serialisation and HPACK table state; envoy-go owns the entire connection lifecycle, settings handshake, stream demux, flow control, error dispatch, GOAWAY/RST_STREAM/PING semantics, and the bridge to HCM's filter chain. Supersedes nothing.
  - **ADR-Q** (server connection manager from scratch): HCM H2 server-side connection manager from scratch — the per-conn state machine and per-stream state machine. Documents the explicit decision NOT to use the Server-or-Transport runtime constructs that `golang.org/x/net/http2` exposes (concretely: `http2.Server`, `http2.Server.ServeConn`, `http2.ConfigureServer`, `http2.Transport`, `http2.Transport.NewClientConn`) even though they ostensibly fit the "low-level" framing. Rationale: those types carry their own request-routing, header-canonicalization, response-header injection, and error policies that diverge from Envoy's; we'd have to fight or unwind those to match Envoy semantics. Building directly on `http2.Framer` + `hpack` is cheaper. Records the architectural shape of `ServerConn`/`serverStream` (client-side `ClientConn` lands in 05.2 under a follow-up ADR).
  - **ADR-S** (server settings defaults): Phase-05.1 H2 server settings — hardcoded defaults: `MAX_CONCURRENT_STREAMS=100`, `INITIAL_WINDOW_SIZE=65535`, `MAX_FRAME_SIZE=16384`, `ENABLE_PUSH=0`, `NO_RFC7540_PRIORITIES=1`, `HEADER_TABLE_SIZE=4096`. Rationale: matches Envoy's documented defaults; matches RFC 9113 protocol defaults where Envoy doesn't override. The differential gate does not assert SETTINGS values byte-for-byte (those are inside the structurally-equivalent framing rule). Per-listener `http2_protocol_options` field-level fidelity is silently-ignored in 05.1 (the directly-on-HCM proto field; the cluster-side typed-extension is 05.2's surface); recorded as part of the silently-ignored set in §9.
  - **ADR-T** (BEHAVIOR_CONTRACT scaffold): BEHAVIOR_CONTRACT HTTP/2 subsection — 05.1 SCAFFOLD shape. Codifies the phase-05.1 codec/conformance equivalence surface (see §1 #7 and §5.7 below). Includes the H2-pseudo-header allow-list rows (as forward-looking entries with "applies-to: 05.2 routed-to-upstream"), the structurally-equivalent framing rule, the h2spec threshold list. The subsection's "does-not-yet-apply-to" explicitly defers fixture 0004 + routed-to-upstream rules to 05.2's brainstorming. ADR-T explicitly records that 05.2's brainstorming will EDIT this subsection in place (not replace via supersession) to flip deferred items to active rules.
  - **ADR-U** (h2spec threshold + pin): h2spec conformance scope and threshold. Pins `summerwind/h2spec` by tag + SHA in `CONFORMANCE_PINS.md`. Declares the section list under threshold: 3 (HTTP Frame Format), 4 (HPACK), 5 (Streams and Multiplexing), 6 (Frame Definitions) MINUS 6.6 (PUSH_PROMISE), 7 (Error Codes), 8 (HTTP Message Exchanges). Excludes 6.6 because phase 05.1 disables push (`ENABLE_PUSH=0`); the section is conformance-irrelevant. Records the per-section pass-count expected at phase-done. The pin's refresh procedure is documented in `docs/envoy-go/CONFORMANCE_PINS.md`. Supersedes nothing (this is the project's first conformance ADR).
  - **ADR-V** (ALPN dispatch wiring): ALPN-driven codec selection wiring. Records the architectural choice that codec selection happens *inside* `Filter.Handle` after the TLS handshake completes (by inspecting `*tls.Conn.ConnectionState().NegotiatedProtocol`), NOT at the listener-side filter-chain match step. Rationale: keeps phase 03's filter-chain-match SNI-only surface unchanged (per ADR-0033), keeps phase 07's filter-chain framework as the natural home for `application_protocols` chain matching when it lands, and minimises blast radius (HCM gains a small dispatch helper; listener manager doesn't change). Documents the alternative considered (treat ALPN as a chain-match dimension) and why it was deferred.
  - **ADR-Z** (test-only `--allow-h2c` flag): Test-only `--allow-h2c` flag on `cmd/envoy-go` to permit `codec_type: HTTP2` on a plaintext listener for h2spec conformance only. Documents the security posture (flag is not advertised in production-marketing surface; default OFF; production builds may strip the flag via a future build tag); the flag exists solely so `test/conformance/h2spec/` can run h2c against the subject without a TLS handshake stealing test cycles. Alternative considered: run h2spec over TLS — rejected because h2spec's TLS support requires a custom CA and complicates the conformance pin; running h2c is the documented h2spec convention. Form decision: CLI flag (vs env var or build tag) — chosen because the testcontainers driver already constructs the subject via `os/exec` and a CLI flag is the lowest-friction option for that driver. The flag accepts no value (boolean `--allow-h2c` / no flag = off); a value-bearing form was considered and rejected as over-engineered for a single use site.
  - **ADR-X** (phase-04 REVIEW carry-forward triage): Phase-04 REVIEW Minor carry-forward triage. Records the phase-05.1 disposition of M-2/M-4/M-5/M-6/M-7 from `docs/envoy-go/phases/04-http-1.1/REVIEW.md` (commit `04527eb`). Disposition decided in §12 below; this ADR is the formal landing spot. ADR-X also explicitly records that 05.1 introduces a *new* prose-vs-mechanism shape on the H2 path (the `defer` cleanup in `serverStream.dispatch`'s action invocation; analogous to phase-04 M-5's H1 prose-vs-mechanism gap) — the cosmetic gap is acknowledged and deferred to the same future SPEC-corrections ADR.

  (The planner re-numbers ADR-P/Q/S/T/U/V/Z/X to concrete ADR numbers at PLAN write time. The lettered placeholders here exist for SPEC clarity; they have no on-disk meaning.)

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 05.1)

Phase 05.1 introduces one new sub-package and modifies several existing ones. **05.2 deliverables are NOT shown** to keep the 05.1 boundary unambiguous:

```
cmd/envoy-go/main.go                 (MODIFIED: --allow-h2c CLI flag per ADR-Z)
cmd/envoy-go/main_test.go            (MODIFIED: h2-over-TLS bootstrap variant)
internal/listener/manager.go         (MODIFIED: passes listenerCtx{hasTLS, allowH2C} to hcm.NewFilter)
internal/listener/manager_test.go    (extended: h2 build cases — TLS + h2c)
internal/filter/hcm/filter.go        (MODIFIED: ALPN dispatch in Handle; listenerCtx in NewFilter)
internal/filter/hcm/config.go        (MODIFIED: codec_type=HTTP2 permitted; new ignored field)
internal/filter/hcm/actions.go       (MODIFIED: directResponseAction codec-neutral factoring;
                                       writeH1 + writeH2 adapters; routerAction unchanged)
internal/filter/hcm/connection.go    (UNCHANGED: H1 driver invokes writeH1 instead of writeStatusReply
                                       — mechanical rename, byte-identical wire output)
internal/filter/hcm/h2/              (NEW sub-package, server-side only in 05.1)
   conn.go         — ServerConn + connection state machine
   stream.go       — serverStream + per-stream state machine
   framer.go       — http2.Framer wrapper with ctx cancellation
   hpack.go        — encoder/decoder integration
   flow.go         — connection + per-stream flow control
   preface.go      — server preface check
   settings.go     — SETTINGS handshake helpers
   errors.go       — error code enum + h2.Error type
   <test files>    — exhaustive unit tests; FuzzFrameStream + FuzzHPACKDecode
                     (NO client.go in 05.1 — that lands in 05.2)

test/conformance/h2spec/             (NEW)
   h2spec_test.go  — testcontainers-driven h2spec runner
   h2spec.go       — threshold section list

docs/envoy-go/BEHAVIOR_CONTRACT.md   (MODIFIED: ## HTTP/2 SCAFFOLD subsection)
docs/envoy-go/CONFORMANCE_PINS.md    (NEW: h2spec image pin)
docs/envoy-go/DECISIONS.md           (APPENDED: ADR-0046..ADR-0053 — 05.1's eight ADRs;
                                       planner verifies next-free at write time)
docs/envoy-go/ROADMAP.md             (row 05.1: planned → in-progress at SPEC commit; → done at phase-done)
docs/envoy-go/STATE.md               (updated at each lifecycle transition)
docs/envoy-go/phases/05.1-downstream-h2/SPEC.md / PLAN.md / PROGRESS.md / REVIEW.md
docs/envoy-go/phases/05-http-2/SPEC.md  (UNCHANGED — master design doc, read-only at parent)

internal/bootstrap/bootstrap.go      (UNCHANGED in 05.1 — blank import for upstreams/http/v3 is 05.2)
internal/cluster/                    (UNCHANGED in 05.1 — DialH2, UseH2(), HttpProtocolOptions parsing all 05.2)
test/differential/                   (UNCHANGED in 05.1 — fixture 0004 + helpers/h2.go all 05.2)
```

### 5.2 Downstream HTTP/2 connection lifecycle

A downstream conn that arrives at `Filter.Handle` (after the listener has accepted, the filter chain has dispatched, and the TLS handshake has completed where applicable) flows like this on the h2 path:

1. `Filter.Handle(ctx, downstream)` checks the build-time-resolved `codecType` and the `listenerCtx`.
   - If `codecType == HTTP1`: dispatch to H1 driver (phase-04 unchanged). Done.
   - If `codecType == HTTP2`:
     - If `downstream` is `*stdtls.Conn`: TLS path. Read `ConnectionState().NegotiatedProtocol`. If `"h2"`, dispatch to H2 driver. If `"http/1.1"` or empty (misconfigured client speaking h1 against h2-only listener), dispatch to H2 driver anyway — the H2 driver's preface check reads 24 bytes and immediately fails (the client's h1 request line doesn't match `PRI * HTTP/2.0...`), producing a connection error and close. This mirrors upstream Envoy's posture.
     - If `downstream` is plaintext (`net.Conn` not `*stdtls.Conn`): h2c path (only reachable when `--allow-h2c` was set at start time, which is the only way the build-time validator allowed `HTTP2` on a plaintext listener). Dispatch to H2 driver directly.
   - If `codecType == AUTO`:
     - If `downstream` is `*stdtls.Conn` and `NegotiatedProtocol == "h2"`: dispatch to H2 driver.
     - Otherwise (plaintext, or TLS with h1/empty ALPN): dispatch to H1 driver (phase-04 unchanged).
2. H2 dispatch constructs `serverConn := h2.NewServerConn(ctx, downstream, table, defaultServerSettings)`. Calls `serverConn.Run()`.
3. `Run()`:
   a. Reads 24 bytes; verifies the connection preface bytes (RFC 9113 §3.4). Mismatch → connection-level error, exit.
   b. Writes the server's initial SETTINGS frame.
   c. Reads the client's initial SETTINGS frame; applies the values; writes SETTINGS_ACK.
   d. Reads the SETTINGS_ACK for our own initial SETTINGS.
   e. Enters the frame loop: `frame := framer.ReadFrame()`; dispatch by frame type to the connection-level handler or to the per-stream handler. PING → emit PING_ACK. WINDOW_UPDATE (stream 0) → adjust connection send window. WINDOW_UPDATE (stream N) → adjust stream N's send window. SETTINGS → apply + ack. PING_ACK → discard. GOAWAY (received from client) → mark conn for close, finish in-flight streams, exit. RST_STREAM → mark stream as closed. HEADERS (new stream) → construct serverStream, start dispatch goroutine. HEADERS (existing stream — for trailers per RFC 9113) → observe + discard per phase-05.1 trailer rule (§2.1). DATA → feed stream's body pipe; on END_STREAM, signal dispatch.
   f. On any connection-level error, emit GOAWAY with the appropriate error code, drain pending writes, close.
4. Each new stream (HEADERS dispatch in step 3.e) spawns a per-stream dispatch goroutine that:
   a. Builds an H2-flavoured request from the decoded HEADERS + the body pipe reader. The exact request type is a planner choice per master SPEC §10 #3 + ADR-0045 ("server-side request/response types live in 05.1") — two SPEC-compatible options: stdlib `*http.Request` reused (cheaper; same shape as the route-table consumes today) or a phase-05.1-internal `H2Request` struct (cleaner separation; insulates HCM-internal types from H1-isms). Recommendation: **reuse stdlib `*http.Request`** so the route-table machinery and the action interface stay single-shape; the H2 codec's `serverStream` builds the `*http.Request` from `:method`/`:path`/`:scheme`/`:authority` pseudo-headers + decoded HEADERS + the body pipe reader. The planner records the choice in PLAN.md.
   b. Calls `table.Match(req)` (the route table from phase 04 — re-used unchanged; the route match operates on `req.URL.Path` derived from the `:path` pseudo-header).
   c. Invokes the matched action. In 05.1 the only reachable action is `directResponseAction`; the action's `writeH2(streamWriter)` adapter writes HEADERS (`:status` pseudo + regular headers) + DATA (body) + END_STREAM via the per-stream send-side helpers. (A `routerAction` match on an H2 stream is theoretically reachable in 05.1 via misconfiguration but is intentionally out-of-spec — see §4.2's `actions.go` note; the route-table builder doesn't yet refuse the configuration in 05.1 because ADR-0045's split with 05.2 means the H2-cluster validation lives in 05.2; instead, an H2 stream that resolves to `routerAction` produces a per-stream INTERNAL_ERROR + RST_STREAM at runtime, with a unit test asserting this protective shape.)
   d. On action error, the goroutine emits RST_STREAM with INTERNAL_ERROR (or whatever `*h2.Error` was returned) and closes the stream.
5. `Run()` exits when: (a) downstream EOF + no streams open; (b) GOAWAY exchanged; (c) ctx cancelled; (d) unrecoverable connection-level error (in which case GOAWAY is emitted before exit).

### 5.3 Upstream HTTP/2 connection — DEFERRED TO 05.2

Master SPEC §5.3 describes `routerActionH2.do(ctx, req H2Request, w H2ResponseWriter)`, `Cluster.DialH2`, and the upstream-side per-stream lifecycle. **None of this lands in 05.1.** 05.1's H2 dispatch path (§5.2) terminates at `directResponseAction.writeH2`; routed-to-upstream H2 is 05.2's deliverable.

This omission is the structural reason 05.1's gate (a) is vacuously green: there is no full-stack H2 fixture to run because 05.1 cannot dial an H2 upstream. The h2spec conformance suite (gate (c)) is the substitute integration coverage for 05.1; it exercises the server-side codec end-to-end against a third-party client without requiring an upstream backend at all (h2spec sends synthetic frames and reads server responses; the only response shape exercised is `direct_response`).

### 5.4 ALPN dispatch decision tree

```
Filter.Handle(ctx, downstream):
  switch f.codecType:
  case HTTP1:
    runH1Connection(ctx, downstream, table)
  case HTTP2:
    // Build-time validator already ensured: TLS listener OR --allow-h2c set.
    runH2Connection(ctx, downstream, table)
  case AUTO:
    if tlsConn, ok := downstream.(*stdtls.Conn); ok {
      if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
        runH2Connection(ctx, downstream, table)
        return
      }
    }
    runH1Connection(ctx, downstream, table)
  }
```

`codec_type: HTTP2` over TLS with ALPN h2 is the production path. Over TLS with ALPN http/1.1 (a misconfigured client speaking h1 against an h2-only listener), the H2 driver runs and the client's first request bytes (which look like an H1 request line) fail the preface check, returning a connection-level error and closing — symmetrical to upstream Envoy's posture. The `--allow-h2c` test-only path is the only way `codec_type: HTTP2` on a plaintext listener passes build-time validation; production builds default the flag to OFF and may strip it entirely in a future doctrine-cleanup phase per ADR-Z.

### 5.5 Direct-response codec-neutral factoring

The phase-04 `directResponseAction.do` writes HTTP/1.1 wire bytes via `writeStatusReply` (or equivalent helper). For 05.1, the action is refactored:

- `directResponseAction` retains its phase-04 fields (`status int`, `body string`, optional `headers map[string]string`).
- It gains a method `body() (status int, headers http.Header, body []byte)` returning the synthesised reply in a codec-neutral form. The body bytes are the configured `body` string interpreted per ADR-0019 / phase-04 rules (`inline_string` is the only supported `body` shape, per ADR-0044's `direct_response.body` row in the silently-ignored set's exclusions); headers contain `Content-Length`, `Content-Type` (`text/plain` per phase-04 default), `Server` (`envoy` per ADR-0014), `Date` (RFC 7231 IMF-fixdate per ADR-0015's allow-list discipline).
- An H1 adapter `writeH1(w io.Writer) error` writes the HTTP/1.1 wire bytes — phase-04 byte-for-byte preserved.
- An H2 adapter `writeH2(sw streamWriter) error` writes HEADERS (`:status` pseudo first, then regular headers in deterministic order — `Date`, `Server`, `Content-Type`, `Content-Length`) + DATA (body bytes) + END_STREAM. The streamWriter interface is internal to `internal/filter/hcm/h2/` and exposes `WriteHeaders(headers []hpack.HeaderField, endStream bool)` + `WriteData(b []byte, endStream bool)`.

The H1 connection driver (`connection.go`) invokes `writeH1`; the H2 stream dispatch (`internal/filter/hcm/h2/stream.go`) invokes `writeH2`. The action carries no codec awareness; the caller picks the writer.

**Backwards-compat invariant:** fixture `0003-http11-routing` MUST remain green after the 05.1 refactor. The refactor is a pure extraction — `writeH1` is the phase-04 `writeStatusReply` body, copied verbatim. Verified by running fixture 0003's existing differential tests after the refactor.

### 5.6 Route table re-use

Phase-04's `routeTable` operates on `req.URL.Path` and matches via `prefix` or `path`. Phase 05.1 re-uses this *verbatim*: the H2 `serverStream.dispatch` helper builds an `*http.Request` whose `URL.Path` is the `:path` pseudo-header decoded value, then calls `table.match(req)`. No change to `route.go`; no change to `actions.go`'s `directResponseAction` semantics (only the codec-neutral factoring). The `routerAction` action type is unchanged in 05.1.

### 5.7 BEHAVIOR_CONTRACT phase-05.1 subsection (HTTP/2) — SCAFFOLD

The new subsection codifies these rules. The wording below is the intended SPEC-binding form for the planner to lift into `BEHAVIOR_CONTRACT.md`. The 05.2 brainstorming session edits this subsection in place to add the routed-to-upstream rules; ADR-T explicitly authorises that in-place edit.

- **Asserted equivalence (05.1 scope):**
  - **Conformance, not differential.** The 05.1 H2 surface is asserted via `h2spec` against the subject standalone, not via a side-by-side proxy-vs-proxy fixture. The differential equivalence of the `direct_response` H2 path (status, decoded body, header set-equality, framing structure) is exercised indirectly through h2spec section 8 (HTTP Message Exchanges), which validates the server's response shape against RFC 9113.
  - **`:status` per request:** required + asserted by h2spec section 8 on every `direct_response` invocation.
  - **Decoded body bytes** on `direct_response` 2xx paths: byte-equal to the configured `body` string (h2spec validates this indirectly via response-length and END_STREAM checks; envoy-go's unit tests assert byte equality directly).
  - **Per-stream response header set-equality modulo allow-list:** locally-generated H2 responses carry `:status` (required + asserted), `Server` (required, value `envoy` per ADR-0014, matched verbatim with upstream's value also `envoy`), `Content-Type`, `Content-Length`, `Date` (presence required; value not byte-compared — same as phase-01 admin/ready discipline). Routed-to-upstream H2 surface: NOT YET ASSERTED IN 05.1 (deferred to 05.2 + fixture 0004).
- **Not asserted (05.1 scope):**
  - Wire-byte H2 framing (frame headers, frame ordering at byte level, padding bytes, HPACK encoded-bytes representation). Frame *types* and *types-on-equivalent-events* are required to match (verified via h2spec section 6); frame *byte-equivalence* is not.
  - SETTINGS values byte-for-byte (h2spec section 6.5 only validates RFC 9113 compliance, not Envoy-specific values).
  - WINDOW_UPDATE timing or count (different windows + different consumption pacing yield different WINDOW_UPDATE frame counts; that's structurally divergent but semantically equivalent).
  - Stream id allocation pattern.
  - Trailers (per phase-05.1 trailer rule + 05.2's ADR-Y).
  - 0-RTT TLS early-data behaviour.
  - **Routed-to-upstream H2 request preservation, decoded body equivalence on routed-to-upstream paths, per-cluster RR distribution on H2, ALPN selection equivalence at the differential level** — ALL DEFERRED TO 05.2.
- **Header allow-list extensions** (rows added to the table at the top of `BEHAVIOR_CONTRACT.md`):
  - `:status` (HCM-locally-generated H2 responses) — presence required on both sides; value asserted. Introduced by phase 05.1, justified by ADR-T.
  - `:scheme`, `:authority`, `:path`, `:method` (routed-to-upstream H2 requests) — **rows added in 05.1 as forward-looking**; the "applies-to" cell reads `phase 05.2 routed-to-upstream H2`. The 05.1 scaffold inserts the rows so the 05.2 brainstorming has nothing to add to the table itself; only the "applies-to" cell flips.
- **h2spec threshold:** sections 3, 4, 5, 6 (excluding 6.6 PUSH_PROMISE), 7, 8 — all `failed == 0`. Pin: `summerwind/h2spec` v2.x at the SHA recorded in `CONFORMANCE_PINS.md`.
- **Applies to (05.1):** phase-05.1 `internal/filter/hcm/h2/` package (server-side only); the codec-neutral `directResponseAction` factoring in `internal/filter/hcm/actions.go`; the conformance suite under `test/conformance/h2spec/`.
- **Does not yet apply to:** routed-to-upstream H2 (05.2 + fixture 0004); HTTP/3; server push; gRPC framing; trailer forwarding; upstream H2 stream pooling; h2c production fixtures; mTLS over h2.

### 5.8 Cluster H2 build-time validation — DEFERRED TO 05.2

Master SPEC §5.8 describes the cluster builder's `typed_extension_protocol_options["...HttpProtocolOptions"]` reader and its TLS+ALPN validation. **None of this lands in 05.1.** 05.1's `internal/cluster/manager.go` is unchanged; clusters carrying `HttpProtocolOptions` continue to be silently ignored per the phase-04 silent-ignore set (ADR-N).

### 5.9 h2spec integration shape

`test/conformance/h2spec/h2spec_test.go`:

```
TestH2Spec(t):
  if testing.Short() { t.Skip("h2spec is not -short") }
  1. Read pinned image tag + SHA from a constant in test/conformance/h2spec/h2spec.go
     (which mirrors the pin documented in docs/envoy-go/CONFORMANCE_PINS.md).
  2. Build envoy-go binary into a temp dir (using go build).
  3. Start envoy-go with a synthetic h2c bootstrap on a host port:
       --allow-h2c
       --config <tempdir>/h2c-bootstrap.yaml
     Bootstrap config: 1 listener on 127.0.0.1:<dyn> (plaintext), 1 filter chain (empty match),
     1 HCM filter with codec_type=HTTP2, 1 route_config with 1 vhost serving 1 catch-all
     direct_response (status 200, body "OK\n"). The h2c-only listener also serves as the target
     for h2spec's request shapes — h2spec sends a wide variety of headers to / and we serve
     direct_response 200 OK\n on every path (h2spec asserts protocol behaviour, not application
     semantics).
  4. Wait for envoy-go to print its admin /ready sentinel.
  5. Start the h2spec container via testcontainers-go pinned by SHA. Mount /tmp as bind mount
     for the JUnit report.
  6. Exec h2spec inside the container with --host=host.docker.internal --port=<dyn> --strict
     --junit-report=/tmp/h2spec.xml. Wait for completion.
  7. Read /tmp/h2spec.xml on host, parse it into a section/test tree.
  8. For each section in the threshold list (3, 4, 5, 6 ex-6.6, 7, 8), assert all child tests
     passed. Report any failure with the section number + test name + h2spec-emitted detail.
  9. Stop subject. Stop container.
```

Runtime budget: the full h2spec run is ~30s wall-clock. Per the project's conformance-suite policy the gate is `go test ./test/conformance/h2spec/...` and the test is excluded from short-budget CI (`go test -short` skips it). The planner explicitly wires the `-short` skip.

The synthetic h2c bootstrap YAML is a planner-time artifact; it lives in `test/conformance/h2spec/testdata/` (or generated inline by the test driver — planner choice). The bootstrap is not committed under `test/fixtures/` because it is not a differential fixture; it is conformance-only.

### 5.10 Streams workload on h2spec

h2spec drives the workload itself. envoy-go does not control which frame sequences h2spec sends; we serve `direct_response 200 OK\n` on every path. h2spec exercises:

- Section 3: Frame header parsing — under-sized, over-sized, with-padding, without-padding, with-various flags, with-various stream-IDs.
- Section 4: HPACK encoding and decoding — dynamic table size updates, indexed name + literal value, never-indexed flags, malformed compression input (SHOULD trigger COMPRESSION_ERROR connection-close).
- Section 5: Stream multiplexing — concurrent streams up to advertised MAX_CONCURRENT_STREAMS, stream ID monotonicity, even-numbered client streams (PROTOCOL_ERROR), idle → open → half-closed → closed transitions.
- Section 6 (excluding 6.6): All frame types except PUSH_PROMISE — DATA, HEADERS, PRIORITY, RST_STREAM, SETTINGS, PING, GOAWAY, WINDOW_UPDATE, CONTINUATION.
- Section 7: All RFC 9113 §7 error codes — verifies the server emits the correct code on each protocol violation.
- Section 8: HTTP message exchanges — one request → one response shape, request method/path/scheme/authority pseudo-headers, response :status pseudo-header.

## 6. Data flow

### 6.1 Direct-response 200 path on H2 (the only routed shape exercised in 05.1)

Plain-text-after-decryption view of one direct-response request on phase 05.1:

```
[client] -- TLS handshake (ALPN: h2) --> [listener]      [TLS path]
[client] -- (no TLS handshake, just TCP) --> [listener]  [h2c path; --allow-h2c]
[client] -- preface bytes ("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n") --> [serverConn]
[client] -- SETTINGS --> [serverConn]
[serverConn] -- SETTINGS_ACK --> [client]
[serverConn] -- SETTINGS --> [client]
[client] -- SETTINGS_ACK --> [serverConn]
[client] -- HEADERS{:method GET, :path /health, :scheme https|http, :authority example.com,
                    user-agent ..., ...; END_HEADERS, END_STREAM} --> [serverConn]
[serverConn] -- demux frame to streamN --> [serverStream(N)]
[serverStream] -- build *http.Request from pseudo-headers + decoded HEADERS + body pipe --> [routeTable.match(req)]
[routeTable] -- returns matching routeEntry{action: directResponseAction{status:200,body:"OK\n"}} --> [serverStream]
[serverStream] -- directResponseAction.writeH2(streamWriter) --> [streamWriter]
[streamWriter] -- HEADERS{:status 200, content-type text/plain, content-length 3, server envoy,
                          date <now>; END_HEADERS} --> [client]
[streamWriter] -- DATA{"OK\n"; END_STREAM} --> [client]
[serverStream] -- transition to closed --> [serverConn]
```

### 6.2 No-match 404 path (only exercised by unit tests in 05.1)

Same up to `routeTable.match(req)`. The matched action is the catch-all `directResponseAction{status: 404, body: "not found\n"}` (or, if the planner picks the implicit-404 form, the dispatch helper synthesises a 404 in-band — mirroring phase-04's connection-loop fallback). Either way: `HEADERS{:status 404, ...}` + `DATA{"not found\n"; END_STREAM}`. h2spec's request shapes don't typically exercise this directly (h2spec sends `/` mostly), but the unit-test suite has explicit coverage.

## 7. Error handling and failure modes

The phase-05.1 H2 codec follows RFC 9113's two-tier error model:

- **Connection-level errors** trigger GOAWAY + close. Examples: bad preface, malformed SETTINGS, HPACK COMPRESSION_ERROR, FRAME_SIZE_ERROR on a non-DATA frame, PUSH_PROMISE received from client (PROTOCOL_ERROR), stream id reuse, even-numbered stream id from client (PROTOCOL_ERROR), connection-level WINDOW_UPDATE with increment=0, GOAWAY received with NO_ERROR (graceful — finish in-flight, don't accept new streams).
- **Stream-level errors** trigger RST_STREAM + per-stream cleanup, conn keeps running. Examples: stream-level FRAME_SIZE_ERROR on DATA, action returned an error (`directResponseAction.writeH2` failed mid-write — exceptional, the body is already buffered in memory; only a downstream-write error can fail it → in that case the conn is half-broken anyway and the next write will fail the conn-level handler), client sends DATA on a half-closed-remote stream (STREAM_CLOSED), `routerAction` reached on an H2 stream (INTERNAL_ERROR — the protective shape per §5.2 step 4c).

The `directResponseAction.writeH2` failure modes:

- **Downstream-write error mid-HEADERS or mid-DATA** → connection-level error (the conn is broken; the conn-level handler emits GOAWAY with INTERNAL_ERROR and closes).
- **`hpack.Encoder` runtime failure** → connection-level error (hpack panics are wrapped to errors per `internal/filter/hcm/h2/errors.go`'s discipline). Fuzz target FuzzHPACKDecode covers the symmetric decoder side; the encoder side is not fuzzed in 05.1 because the inputs are server-controlled (no untrusted bytes reach the encoder), but the unit tests cover boundary cases (empty header value, max-length header value, large dynamic table evictions).

Listener-bind error semantics carried forward unchanged from phase-02 (`log.Fatalf` on bind failure; admin `/ready` never reaches Ready).

## 8. Testing scope for phase 05.1

### 8.1 Unit tests (under `internal/filter/hcm/h2/`)

Exhaustive coverage of every server-side state-machine transition, every error code, every flow-control corner case, every settings-handshake variant. Specific test names enumerated in §4.1's `<test files>` bullet. Test peer is `golang.org/x/net/http2.Transport` for client-side scenarios (driver-side use OK; runtime use forbidden). **No client-side codec tests in 05.1** — `client.go`/`ClientConn` lands in 05.2.

### 8.2 Unit tests (under `internal/filter/hcm/`)

ALPN dispatch in `Filter.Handle` (TLS+h2 → H2 driver; TLS+h1 → H1 driver; TLS+empty-ALPN → H1 driver; plaintext + `--allow-h2c` + `codec_type:HTTP2` → H2 driver; plaintext + no `--allow-h2c` + `codec_type:HTTP2` → build error tested at manager level). `codec_type: HTTP2` build-time error on plaintext listeners without `--allow-h2c`. `codec_type: HTTP2` build success on TLS listener with ALPN h2. `codec_type: AUTO` on TLS listener with `["h2","http/1.1"]` builds; runtime dispatch picks h2 vs h1 based on `NegotiatedProtocol`. `directResponseAction.writeH2` covers HEADERS+DATA+END_STREAM emission on 200 + 404; covers ordering rule (`:status` first, regular headers after); covers `writeH1` + `writeH2` byte-equivalence at the body-bytes level (the fixture-0003 surface is the integration check).

### 8.3 Unit tests (under `internal/listener/`)

Build-time validation: TLS listener + HCM HTTP2 → success. Plaintext listener + HCM HTTP2 + `allowH2C=true` → success. Plaintext listener + HCM HTTP2 + `allowH2C=false` → build error. TLS listener + HCM AUTO → success. Plaintext listener + HCM AUTO → success (resolves to H1 dispatch).

### 8.4 End-to-end smoke (under `cmd/envoy-go/main_test.go`)

H2-over-TLS bootstrap variant: TLS listener + HCM `codec_type: HTTP2` + ALPN h2 + direct_response → asserted via x/net/http2.Transport client probe (driver-side use OK).

### 8.5 Differential — VACUOUS in 05.1

No new differential fixture in 05.1. The pre-existing fixtures `0000`/`0001`/`0002`/`0003` continue to run unchanged. The test/differential/runner is not modified. Gate (a) is vacuously green.

### 8.6 Conformance (under `test/conformance/h2spec/`)

h2spec at the pinned SHA + threshold sections 3/4/5/6\6.6/7/8 → `failed == 0`. Excluded from `-short`. **THIS IS THE 05.1 NON-VACUOUS GATE** that substitutes for a differential fixture; it exercises the full server-side codec end-to-end against a third-party client.

### 8.7 Fuzz (under `internal/filter/hcm/h2/`)

`FuzzFrameStream` (mutates frame sequences; asserts no panic + h2:-prefixed errors). `FuzzHPACKDecode` (adversarial header blocks; asserts no panic). Short-budget 30s CI per ADR-0018.

## 9. Out-of-scope (explicitly deferred)

Beyond §2's non-purposes, phase 05.1 silently ignores the following at parse time (no error, no honoured behaviour):

- HCM `http2_protocol_options` (the directly-on-HCM proto field). New silent-ignore in 05.1; the cluster-side `HttpProtocolOptions` stays in the phase-04 silent-ignore set since 05.1 doesn't read it.
- Cluster `HttpProtocolOptions` (every field) — still silently ignored from phase 04 (per ADR-N); 05.2 introduces selective parsing of the `http2_protocol_options` discriminator.
- Listener `filter_chain_match.application_protocols[]` (extending the phase-04 ignored set; ALPN-driven chain matching is a phase-07 concern).
- HCM `internal_address_config`, `path_with_escaped_slashes_action`, `add_user_agent`, `proxy_status_config`, `typed_header_validation_config`, `original_ip_detection_extensions`, `early_header_mutation_extensions`, `header_validation_config`. (Some of these are phase-04-already-ignored; this list calls out the ones that are H2-relevant to enumerate the phase-05.1 ignored-set explicitly.)

The full silently-ignored set is the union of phase-04's (per ADR-N) and phase-05.1's amendment above. ADR-N is amended (not superseded) to record the addition of `http2_protocol_options`.

## 10. Deferred decisions (the planner / implementer settles these)

This list is narrowed from master phase-05 SPEC §10 to the items that are scoped to 05.1. Items #3, #6, #7, #10 from the master SPEC §10 are 05.2's planner concerns and are not repeated here.

1. **Streaming-body dispatch vs wait-for-END_STREAM.** Master SPEC §10 #1 + ADR-0045 prescribe "wait for END_STREAM before invoking the action" for 05.1. The planner records the choice in PLAN.md. (Decided.)
2. **Whether to factor `directResponseAction.body()` codec-neutral now or keep two writers.** Master SPEC §10 #2 + ADR-0045 prescribe the codec-neutral factoring (it lands in 05.1 because h2spec section 8 exercises `direct_response`). The planner records the choice in PLAN.md. (Decided.)
3. **Whether to thread an `H2Request`/`H2Response` type-pair or re-use stdlib `*http.Request`/`*http.Response` shapes.** Master SPEC §10 #3 + ADR-0045 split this per-direction: the SERVER-side request type lives in 05.1; recommendation is to **reuse stdlib `*http.Request`** so the route-table machinery and action interface stay single-shape (per master SPEC §10 #3 + §5.2 step 4a). The planner records the choice in PLAN.md.
4. **Whether the conformance pin lives in `docs/envoy-go/CONFORMANCE_PINS.md` (new file) or as a Go const in `test/conformance/h2spec/h2spec.go` only (no doc file).** Master SPEC §10 #4 prescribes the doc file; this 05.1 SPEC ratifies (§4.1 lists `CONFORMANCE_PINS.md` as a new file; the const in `h2spec.go` is a mirror of the doc-file pin, with an `// authoritative pin: docs/envoy-go/CONFORMANCE_PINS.md` comment to make the doc file the single source of truth).
5. **Whether `--allow-h2c` is a CLI flag or an environment variable or a build tag.** Master SPEC §10 #5 left this open; 05.1's ADR-Z decides **CLI flag** (lowest-friction for the testcontainers driver per ADR-Z's rationale). The planner wires the flag in `cmd/envoy-go/main.go`.
8. **Whether the H2 server emits `:status` first or `content-type` first in HEADERS.** Master SPEC §10 #8 — RFC 9113 §8.3 requires pseudo-headers before regular headers; both x/net/http2 and Envoy comply. The planner ensures phase-05.1's HEADERS encoding puts `:status` first in `directResponseAction.writeH2`; this is a correctness rule (h2spec section 8 catches violations). Not a deferred decision so much as a note.
9. **Concrete ADR numbers for ADR-P/Q/S/T/U/V/Z/X (eight in 05.1).** Per ADR-0045's "ADR numbering shift" consequence: after ADR-0045, the next-free is ADR-0046; 05.1's eight ADRs land at ADR-0046..ADR-0053. The planner re-verifies next-free at write time and assigns the eight letters to the eight numbers in the order they're authored in PLAN.md.

## 11. Risks and mitigations

### 11.1 Phase-splitting risk — RESOLVED

Master SPEC §11.1 anticipated the ~25-task / ~1500-LoC gate trip; ADR-0045 resolved it by splitting phase 05 into 05.1 + 05.2 (the split-by-surface axis). 05.1's planner re-runs the size estimate at PLAN.md write time; the expected size is ~12–15 TDD tasks + ~1700 LoC (codec sub-package ~1500 LoC + HCM + conformance + ADRs + carry-forward triage), which is comfortably under the gate per axis (the LoC is at the threshold; the task count is well under). If 05.1's PLAN trips the gate again, the split-by-codec-direction axis is exhausted and the planner exits blocked per `BOOTSTRAP_PROMPT.md` §6.

### 11.2 h2spec image availability / pin freshness

**Risk:** `summerwind/h2spec` is community-maintained; tags can move or disappear. CI runs against a moving image break gate (c).

**Mitigation:** pin by tag + SHA256 in `CONFORMANCE_PINS.md`. The image refresh is a dedicated phase per D-3.7. CI uses the SHA digest, not the tag.

### 11.3 x/net/http2 version drift

**Risk:** `golang.org/x/net/http2` evolves; Framer or hpack APIs may shift, breaking 05.1 code. Phase-04 already pinned go.mod entries — new pins for x/net/http2 are inherited.

**Mitigation:** `go.sum` pins the SHA; module updates are explicit phase work per D-3.7. The `internal/filter/hcm/h2/` package tests are run on every CI build; an x/net/http2 API drift surfaces immediately.

### 11.4 HPACK compression-table-size negotiation correctness

**Risk:** SETTINGS_HEADER_TABLE_SIZE is dynamic; the receiver must respect the sender's max even mid-stream after a SETTINGS update. Subtle bugs here yield COMPRESSION_ERROR connection-aborts under heavy header churn (gRPC-style). h2spec section 4 covers this, but if our test peer (driver-side x/net/http2) doesn't drive aggressive size-update sequences, our impl might pass h2spec yet fail on real workloads.

**Mitigation:** unit test that explicitly drives a SETTINGS_HEADER_TABLE_SIZE shrink mid-conn and asserts the next outgoing HEADERS frame respects the new size. h2spec's section 4 tests cover the on-wire correctness; the unit test covers our internal table-state propagation. Recorded in PLAN.md.

### 11.5 Flow-control deadlock under tiny windows

**Risk:** A degenerate test (or a malicious client) advertises `INITIAL_WINDOW_SIZE = 1` and sends a many-byte body; our send path must block-and-wake correctly on WINDOW_UPDATE. Bugs yield deadlock.

**Mitigation:** unit test with `INITIAL_WINDOW_SIZE = 1` and a 1024-byte response body; assert delivery completes via WINDOW_UPDATE-driven progress. h2spec section 5 covers the on-wire correctness.

### 11.6 ALPN dispatch race on slow handshakes

**Risk:** `Filter.Handle` reads `NegotiatedProtocol` immediately on entry. If the listener's TLS handshake hasn't completed (it should have, but a buggy refactor could change that), `NegotiatedProtocol` returns `""` and dispatch falls through to H1 — silently mis-routing h2 traffic to the H1 driver, which produces a 400 line on the first request bytes.

**Mitigation:** the listener-side handshake-completion contract is asserted by phase-03's tests. Phase-05.1's `Filter.Handle` adds a defensive `downstream.(*stdtls.Conn).HandshakeContext(ctx)` no-op call before reading `NegotiatedProtocol` — the call is idempotent for already-completed handshakes; if a future refactor removes the listener-side handshake, the HCM still gets correct data. Recorded in PLAN.md.

### 11.7 Phase-04 carryover Minor M-7 (`Filter.statPrefix` unused) becomes load-bearing for phase 06

Same risk + mitigation as master SPEC §11.7, repeated here because the carry-forward triage lands in 05.1 (per ADR-0045). Mitigation: §12's M-7 disposition explicitly defers but with a SPEC-noted "phase-06-must-consume" tag in `PLAN.md`. Phase 06's brainstorm reads this tag and either honours `Filter.statPrefix` or supersedes ADR-N.

### 11.8 `directResponseAction` refactor regresses fixture 0003

**Risk:** the codec-neutral factoring (§5.5) is a non-trivial refactor of phase-04's `directResponseAction.do`. A subtle bug (header ordering, byte-for-byte deviation in the H1 wire output) regresses fixture 0003.

**Mitigation:** the H1 adapter `writeH1` is a verbatim extraction of phase-04's `writeStatusReply` body — no logic change. The unit-test suite under `internal/filter/hcm/` has a dedicated `TestDirectResponseWriteH1Compat` that compares the new `writeH1` output against a golden byte-string captured from the phase-04 `writeStatusReply` (the golden bytes are committed as a small test fixture under `internal/filter/hcm/testdata/direct_response_h1.golden`). Fixture 0003's differential test is the integration check; the golden test is the unit check.

### 11.9 h2spec reads JUnit-XML format that differs across h2spec versions

**Risk:** h2spec's `--junit-report` output schema might change between versions. The pin in `CONFORMANCE_PINS.md` plus the test parser is tightly coupled to one schema.

**Mitigation:** the pin discipline (D-3.7) means the schema is frozen at the pinned tag/SHA. The test parser is small (~50 LoC) and lives in `test/conformance/h2spec/h2spec_test.go`; if the schema changes when the pin is refreshed, the parser is updated in the same PR as the pin (per D-3.7 the pin refresh is its own phase, and the schema-update is a single-task chunk inside it).

## 12. Phase-04 REVIEW carryover triage

Phase-04 closed with `REVIEW.md` (commit `04527eb`) verdict APPROVED WITH FOLLOW-UPS; all four Important findings (I-1..I-4) plus the cleanest Minor (M-1) landed in `671a059`. Five Minor findings (M-2, M-4, M-5, M-6, M-7) were deferred to "phase 05+" per `STATE.md`. M-3 was naturally resolved by the I-1 fix. Per ADR-0045 the carry-forward triage lands in 05.1 (none of the Minors touches upstream-H2 surface):

- **M-2 — ADR-0043's `Doctrine: D-3.4, D-3.5` mismatched against the informal supersession qualifier.** *Defer.* Phase 05.1 does not touch ADR-0043 (HTTPExpectations driver extension); the inconsistency is cosmetic. ADR-X carries the explicit deferral; a future doctrine-cleanup ADR (likely under the observability or admin-API phases when multiple ADRs are amended together) supersedes ADR-0043 with a corrected doctrine attribution.
- **M-4 — listener-manager `Stop()`/`Listeners()` race.** *Defer.* Phase 05.1 does not touch `internal/listener/manager.go`'s lock surface (the only listener-manager change in 05.1 is passing `listenerCtx` into `hcm.NewFilter`, which is a build-time path; runtime Stop/Listeners is unchanged). The race is inherited from phase 03's M-2 carry-forward and remains unresolved. ADR-X carries the deferral; phase 08's admin-api-and-drain phase is the natural place to close this (drain semantics require a correct Listeners() snapshot).
- **M-5 — phase-04 SPEC §7 failure-mode "close upstream" prose vs `defer upstreamConn.Close()` mechanism.** *Defer (cosmetic).* Phase 05.1 does NOT introduce a parallel mechanism on the H2 path because 05.1 does not have the upstream-H2 surface (that's 05.2's `routerActionH2.do` with `defer clientConn.Close()`). The phase-04 H1 prose-vs-mechanism gap remains unchanged in 05.1. ADR-X carries forward. Documentation cleanup is bundled into a future SPEC-corrections ADR.
  - **05.1 addendum:** the same prose-vs-mechanism shape will reappear in 05.2 (`routerActionH2.do`'s `defer clientConn.Close()`). ADR-X explicitly carries this forward as a "phase-05.2-will-repeat-the-pattern" note so 05.2's brainstorming inherits the disposition rather than re-litigating.
- **M-6 — fixture-0003 driver's heredoc YAML pattern.** *Defer.* Phase 05.1 does NOT introduce a new fixture (no fixture lands in 05.1; fixture 0004 is 05.2's deliverable). The structured-`expectations.yaml` plan from ADR-0019 still belongs to the observability / phase-06 sweep; 05.1 holds the line. ADR-X carries forward.
- **M-7 — `Filter.statPrefix` stored but never consumed.** *Defer with phase-06-must-consume tag.* Phase 05.1 does not consume `Filter.statPrefix` either (no stats subsystem yet). The phase-04 `stat_prefix` storage shape is unchanged. ADR-X carries the forward-looking note; phase 06's brainstorm is required to either honour `Filter.statPrefix` (lifting M-7 to resolved) or supersede ADR-N with a stat-naming policy that obviates the field. 05.1's `PLAN.md` includes a SPEC-noted "phase-06-must-consume" tag in the carryover-list section.

ADR-X (Phase-04 REVIEW Minor carry-forward triage) is the formal landing for these dispositions.

Additionally, REVIEW.md surfaced (as the "single most important context to surface to the phase-05 planner") that **fixture-0003 still does not differentially exercise upstream TLS** (per ADR-0035 carry-forward). Phase 05 closes this gap *for the H2 leg* via fixture 0004's full-stack HTTPS h2 — but **fixture 0004 lands in 05.2, not 05.1**. ADR-W (which closes ADR-0035's H2 leg) is therefore 05.2's deliverable per ADR-0045. 05.1 records the deferral here so the 05.2 brainstorming inherits the surfacing.

## 13. Acceptance checklist (for the reviewer of this sub-phase's final state)

A reviewer (phase 05.1's `superpowers:requesting-code-review` subagent) signs off when every item below is verifiable from the on-disk state:

- [ ] All six phase-done gates (a–f) green per §3, with gate (a) recorded as "vacuous — no new fixture in 05.1" by the verifier.
- [ ] `internal/filter/hcm/h2/` package contains the from-scratch ServerConn + serverStream implementations; **NO `client.go` file in the package** (clientConn is 05.2). No `http2.Server` or `http2.Transport` runtime is used by envoy-go runtime code (driver-side use in `cmd/envoy-go/main_test.go` and `test/conformance/h2spec/h2spec_test.go` is permitted and grep-verifiable).
- [ ] `internal/cluster/` is unchanged from phase 04 (no `dial_h2.go`; no `UseH2()`; no blank import; no `HttpProtocolOptions` reader). `git diff phase-04..05.1 -- internal/cluster/` shows zero changes.
- [ ] `Filter.Handle` ALPN dispatch is grep-verifiable (one `*stdtls.Conn` type-assert + one `NegotiatedProtocol` read + one branch on `"h2"`). Build-time validation of `codec_type: HTTP2` against `listenerCtx.allowH2C` is grep-verifiable.
- [ ] `directResponseAction` factoring is grep-verifiable: a `body()` method exists; `writeH1(io.Writer)` and `writeH2(streamWriter)` methods exist; the H1 connection driver invokes `writeH1`; the H2 stream dispatch invokes `writeH2`. Fixture `0003-http11-routing` runs green after the refactor.
- [ ] `BEHAVIOR_CONTRACT.md` carries a new `## HTTP/2` SCAFFOLD subsection with the seven subheadings prescribed by §5.7 (Asserted equivalence (05.1 scope); Not asserted (05.1 scope); Header allow-list extensions; h2spec threshold; Applies to (05.1); Does not yet apply to). Header allow-list table at the top of the file has rows for `:status` (active in 05.1) and `:method`/`:path`/`:scheme`/`:authority` (forward-looking, applies-to: 05.2) with phase-05.1 + ADR-T provenance.
- [ ] `CONFORMANCE_PINS.md` exists and pins `summerwind/h2spec` by tag + SHA256 with a refresh procedure.
- [ ] All eight 05.1 ADRs (the planner-assigned ADR-0046..ADR-0053 mapping to ADR-P/Q/S/T/U/V/Z/X) appear in `DECISIONS.md` with full Context/Decision/Consequences sections per ADR-0001's template. The ADR-numbering-shift discipline from ADR-0045 is honoured (the planner verified next-free at write time and the eight numbers are contiguous).
- [ ] `test/conformance/h2spec/h2spec_test.go` exists, is excluded from `go test -short`, and reports `failed == 0` over the threshold sections when run unrestricted. The h2c bootstrap consumed by the test uses `--allow-h2c`.
- [ ] No phase-04 fixture (`0000`/`0001`/`0002`/`0003`) regressed under the unrestricted `go test ./test/differential/...` run.
- [ ] `cmd/envoy-go/main_test.go` carries an h2-over-TLS bootstrap variant exercising the binary's H2 listener path. The `--allow-h2c` h2c smoke is provided by the conformance suite, not duplicated here.
- [ ] `STATE.md` is at lifecycle-state 6 (or appropriate end state for the 05.1 sub-phase); `ROADMAP.md` row 05.1 is `done`; row 05.2 remains `planned`; row 05 (parent) remains `in-progress`. The §5.3 phase-done commit's message names every ADR introduced or referenced.
- [ ] `PROGRESS.md` quotes the command outputs of all six gates per the §5.3 verification protocol (gate (a) explicitly noted as vacuous); SHA-fill for each task entry per the phase-04 convention.
- [ ] The phase-04 REVIEW Minor disposition (§12) is faithfully recorded in ADR-X with no silent re-disposition. The "05.2-will-repeat-the-pattern" note for M-5 is present.
- [ ] The 05.1/05.2 boundary is auditable: `internal/filter/hcm/h2/` has no `client.go`; `internal/cluster/` has no `dial_h2.go` or `HttpProtocolOptions` reader; `test/fixtures/` has no `0004-h2-routing/` directory; `BEHAVIOR_CONTRACT.md ## HTTP/2` explicitly enumerates the deferred-to-05.2 rules in its "Does not yet apply to" subsection.

When all boxes above are checked, phase 05.1 is `done` and the project advances to phase 05.2 (upstream-h2) at lifecycle-state 1. Phase 05 (the parent) remains `in-progress` until 05.2 also reaches `done`.
