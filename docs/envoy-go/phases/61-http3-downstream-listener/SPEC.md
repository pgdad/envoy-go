# SPEC 61 — a minimal downstream HTTP/3 listener (the HTTP/3 + QUIC family OPENER; the FIRST greenfield transport since the MVP trunk; introduces the FIRST new go.mod module `github.com/quic-go/quic-go`; a TLS-mandatory UDP/QUIC listen path parallel to the TCP one, serving an H3 GET into the EXISTING HCM → router → filter-chain → upstream path; a MULTI-LEG SPLIT 61.1/61.2/61.3, ADR-0279)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only; NO production `.go` changes at this stage. Fresh worktree `.worktrees/phase-61-spec`, branch `phase-61-http3-downstream-listener-spec`, per `feedback_git_worktrees`.
>
> **ANCHORS ADR-0279 §Context DRAFT** (§13). §Decision/§Consequences land at the phase-61 IMPL(s) per ADR-0044; a MULTI-leg split may anchor per-leg ADRs (§3.0). DECISIONS tail STAYS **ADR-0276** at this SPEC (ADR-0277 reserved phase-59 tracing; ADR-0278 reserved the parallel xDS opener; ADR-0279 reserved THIS family — given).
>
> **Baselines re-verified against master tip `cbda648b` (the phase-60+61 fan-out registration):** stat surface **1201** · fixtures **103** (tail `0101-stats-sink-graphite`) · fuzzers **54** · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0276** · new Go packages **0** · **new go.mod modules 0** (this family opener adds the FIRST — `quic-go`). Counts UNCHANGED at this SPEC (docs-only). Every `file:line` below was RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — the roster is §12.

---

## 1. Purpose / Mission

Phase 61 OPENS the HTTP/3 + QUIC family with the smallest slice that genuinely stands up the family's novel substrate: a **minimal TLS-mandatory downstream HTTP/3 listener**. An operator configuring a `Listener` with `udp_listener_config.quic_options`, a `filter_chain` whose transport socket is `envoy.transport_sockets.quic` (wrapping a `DownstreamTlsContext` with ALPN `h3`) and whose HCM has `codec_type: HTTP3`, gets a UDP-bound QUIC listener that accepts a QUIC connection, negotiates ALPN `h3`, decodes an HTTP/3 request, and dispatches it through the **EXISTING** HCM → router → filter-chain → upstream path, writing the response back over H3. The proven capability is a single `GET → 200 (or a routed backend response)` over HTTP/3, differentially provable cross-side against `envoyproxy/envoy:contrib-v1.37.2`.

This lifts the two greenfield-confirming rejects: the `transport_protocol "quic"` reject (`internal/listener/manager.go:685-689`) and the `codec_type HTTP3` reject (`internal/filter/hcm/config.go:231-240`). Everything downstream of the H3 codec (routing, filters, cluster dial, load balancing, upstream) is REUSED unchanged.

ADR-0279 §Context is DRAFTED here (§13). **All eleven BRAINSTORM D-H3-* questions are DISPOSED at this SPEC** — five of them PINNED by a LIVE probe against `envoyproxy/envoy:contrib-v1.37.2` this session (§11):

- **D-H3-SLICE / D-H3-SPLIT** — DECIDED (§3.0): a MULTI-LEG split **61.1** (QUIC/UDP listen substrate + the `quic-go` module) / **61.2** (H3 codec + HCM adapter) / **61.3** (differential fixture + harness UDP/H3-client surgery). Anticipated well over the ADR-0045 ~15-task gate — the escape-valve is CONSUMED.
- **D-H3-QUICLIB** — **PINNED** (§11 arm h3-get): `github.com/quic-go/quic-go` **v0.54.1** — its `http3.Transport` client completed an H3 GET against the reference's H3 listener (`proto=HTTP/3.0`, `status=200`, `body="h3-ok"`, `ALPN="h3"`, `TLS 1.3`). v0.54.1 is the LAST release keeping the project's `go 1.23` directive (§2.4). Interop with reference Envoy H3 is PROVEN.
- **D-H3-HCM-SEAM** *(the central uncertainty)* — **DERIVED** (§3.3): the codec-agnostic per-request dispatch seam ALREADY EXISTS (the H2 arm `WriteH2` takes NO `net.Conn`, feeds an `http.Header` → the shared filter-chain → `router.ActionResponse`). H3 is a THIRD PARALLEL dispatch arm modeled on `runH2`/`WriteH2` (~200 lines), NOT framework surgery; quic-go's `http3.Server` yields `(*http.Request, http.ResponseWriter)` — the request side reuses the H1 `Action` path (plausibly ZERO `router.go` changes), the response side is a small `http.ResponseWriter → writeH3Reply` adapter.
- **D-H3-LISTEN-SEAM** — **DERIVED** (§3.2): a discriminated `kind` field on `listenerRuntime` (≈80% field overlap; reuse the chain-build, `registerListenerMetrics`, and `netChainFactory`); `Start` branches on kind (`net.Listen("tcp",…)` vs a UDP `net.ListenUDP` + a quic-go listener); a sibling `quicAcceptLoop`/`serveQUICConnection` parallel to `acceptLoop`/`serveConnection`.
- **D-H3-BOOTSTRAP-WIRE** — **PINNED** (§11 arms h3-get / reject-B / reject-C): the exact minimal viable config; the reference boot-rejects HTTP3-on-non-QUIC-listener and QUIC-without-transport-socket (§6). The strict-reject posture (ADR-0080, distinct substrings) for unsupported `quic_options` sub-fields is DECIDED (§6).
- **D-H3-TLS** — **PINNED** (§11 arm h3-get): ALPN `h3`, TLS 1.3, cert via `DownstreamTlsContext.common_tls_context.tls_certificates` + `alpn_protocols`. REUSES the existing `internal/tls.NewDownstreamConfig` which ALREADY sets `NextProtos` from `alpn_protocols` (`internal/tls/config.go:208`) — quic-go consumes that `*stdtls.Config` directly.
- **D-H3-DIFF-HARNESS** *(a genuine uncertainty)* — **DERIVED** (§8): the harness is TCP-only in three container-starters; add `/udp` exposure + a `udpAddrs` map + `MappedPort(…/udp)` + a `ListenerUDPAddr` accessor; a new shared `test/helpers/h3.go` `H3RoundTrip` on quic-go's `http3.Transport` (modeled on `h2.go:33`). The cross-side fixture lands in leg 61.3.
- **D-H3-STATS** — **PINNED** (§11 arm h3-get /stats): the reference emits a small H3/QUIC downstream stat family (§7). envoy-go asserts a NAMED SUBSET (`reference_stats_sink_emits_used_only`); the exact NEW registered stat count is IMPL-pinned per leg (recommend deferring the granular `http3.*` robustness counters).
- **D-H3-FUZZSEED** — DECIDED (§6): anticipated **+0** — quic-go owns HTTP/3 framing + QPACK; the only hand-rolled parse is the bootstrap `udp_listener_config`/`quic_options`/transport-socket config, which takes a SEED on the existing listener/HCM config fuzzer if the IMPL adds one. Fuzzers stay **54**.

The one genuine SPEC-BLOCKING unknown (does quic-go interop with reference Envoy H3, and at what version) is RESOLVED. The remaining depth (the exact code shape of the third dispatch arm and the QUIC-native SNI/ALPN handshake seam) is IMPL-investigation, DERIVED here from source with citations, NOT guessed.

### 1.1 Empirical-finding-driven scope note (per ADR-0044)

The BRAINSTORM (§2.5) flagged D-H3-HCM-SEAM as possibly needing a NEW codec-agnostic dispatch seam (framework surgery). The SPEC-61 source read (§3.3, DERIVED from the H2 codec) shows that seam ALREADY EXISTS: the H2 arm `WriteH2` (`internal/filter/hcm/h2dispatch.go:269`) takes NO `net.Conn` and drives the whole shared chain from a decoded request + a `StreamWriter` abstraction. This is a material DE-RISKING of the family's central uncertainty — H3 is an additive third arm, not a re-architecture. The one nuance the source overturns from a naive reading: there is NO single unified request struct both codecs hand to one entry function — each codec (H1 `dispatchRequest`, H2 `WriteH2`) re-implements the same ~10-step recipe (symmetric-by-duplication), so "thin adapter" means "a new ~200-line arm modeled on `WriteH2`," which the split (§3.0) sizes as leg 61.2.

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8)

NO **upstream** H3 (a cluster-side QUIC transport socket + an H3 upstream pool). NO **alt-svc** advertisement (`Alt-Svc: h3=…` on the H1/H2 listener). NO **0-RTT / early-data** (`enable_early_data`). NO **h3spec** conformance gate. NO QUIC connection migration, GREASE, datagrams, WebTransport. NO consumption of `QuicProtocolOptions` sub-fields beyond the minimal boot (max concurrent streams / idle timeout / connection-id length / flow-control windows are accepted-and-ignored or strict-rejected, §6). NO granular `http3.*` robustness counters (`rx_reset`/`tx_reset`/`tx_flush_timeout`, §7 — deferred to a QUIC-robustness follow-on). Filter-chain-match over QUIC beyond a single default chain (SNI-based selection at the QUIC handshake) is scoped to the minimal single-chain slice; full SNI-dispatch semantics carry forward.

### 2.1 The family STAYS OPEN after phase-done

Row 61 is the family-OPENING row. After phase 61 phase-done the §8 candidates (upstream H3, alt-svc, 0-RTT, h3spec, `QuicProtocolOptions` tuning, full transport-socket options, QUIC robustness) remain ⇒ the sentinel check-(2)/(3) still prints ⇒ the loop continues. The controller wrote the family's LIVE deferred-candidate sentence at row-registration (`reference_sentinel_deferred_sentence_live_vs_historical`).

---

## 3. The change — a NEW UDP/QUIC listen path + a NEW H3 codec/HCM-adapter arm + the `quic-go` module + the mandatory-TLS/ALPN wiring (ADR-0279)

### 3.0 Split disposition — a MULTI-LEG split; the ADR-0045 escape-valve CONSUMED

A greenfield transport with a new external module, a new UDP listen path, and a new codec is well over the ADR-0045 ~15-task gate. The **by-concern** legs (each independently verifiable; the parent row 61 flips `done` only when ALL legs land, `reference_roadmap_split_phase_row_done`):

- **61.1 — QUIC/UDP listen substrate + the `quic-go` module.** Introduce `github.com/quic-go/quic-go` v0.54.1 (§2.4); a UDP/packet listen path parallel to `manager.go:862`'s `net.Listen("tcp",…)` (bind `net.ListenUDP` + a quic-go listener over it); parse the bootstrap wire shape (`udp_listener_config` + `quic_options` + the `envoy.transport_sockets.quic` downstream transport socket wrapping a `DownstreamTlsContext`; §5); lift the `transport_protocol "quic"` reject (`manager.go:685-689`); build the `*stdtls.Config` (ALPN `h3`) and complete the QUIC/`h3`-ALPN handshake; register the reused per-listener `downstream_cx_total` stat. Accept a QUIC connection + handshake; the strict-reject arms (§6). Verified subject-side by a unit/integration test of the handshake (a local quic-go client) — NO HTTP serving yet. Anticipated ADR-0279 (the family opener + the listen-path seam + the module decision).
- **61.2 — the H3 codec + HCM integration.** Add a THIRD dispatch arm (`internal/filter/hcm/h3`, sibling of `h2`) adapting quic-go's `http3.Server` `(*http.Request, http.ResponseWriter)` into the EXISTING shared chain (§3.3); lift the `codec_type HTTP3` reject (`config.go:231-240`); add `emitAccessLogH3` (`accesslog_emit.go`, §3.3). Serve a GET → routed backend, verified subject-side by a local quic-go `http3.Transport` client (FREE once the module landed in 61.1). Anticipated a per-leg ADR (the codec-arm seam) OR folded into ADR-0279 — the PLAN/IMPL decide.
- **61.3 — the differential fixture + harness UDP/H3-client surgery.** The harness UDP-port surgery (three starters, §8) + a shared `test/helpers/h3.go` `H3RoundTrip` client; the one cross-side H3-GET fixture `0102-http3-downstream-get` (§8). May be FOLDED into 61.2 or landed as an infra prerequisite — the PLAN decides; recommended as its own leg because the harness has never driven a non-TCP transport.

The SPEC RE-SCOPES if 61.2 proves smaller than anticipated (e.g. if the H3 arm drops in under ~120 lines, 61.2 could fold the fixture leg). ADR-0045 escape-valve CONSUMED.

### 3.1 The bootstrap wire shape — all protos RESOLVE in the EXISTING go-control-plane module (NO new proto module)

Confirmed present in the ALREADY-resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module this session (`go list` resolves both `config/listener/v3` and `extensions/transport_sockets/quic/v3`; §11):

- **`Listener.udp_listener_config`** → `GetUdpListenerConfig() *UdpListenerConfig` (`config/listener/v3/listener.pb.go:606`). Sub-getters: `GetQuicOptions() *QuicProtocolOptions` (`udp_listener_config.pb.go:87`), `GetDownstreamSocketConfig()` (`:80`), `GetUdpPacketPacketWriterConfig()` (`:94`).
- **`UdpListenerConfig.quic_options`** → `*QuicProtocolOptions` (`config/listener/v3/quic_config.pb.go:31`) — enables QUIC/H3. Sub-getters include `GetQuicProtocolOptions() *core.v3.QuicProtocolOptions` (`:124`), `GetIdleTimeout()` (`:131`), `GetCryptoHandshakeTimeout()` (`:138`), `GetEnabled()` (`:145`), `GetProofSourceConfig()` (`:166`), `GetConnectionIdGeneratorConfig()` (`:173`), `GetRejectNewConnections()` (`:208`). The nested `core.v3.QuicProtocolOptions` carries `GetMaxConcurrentStreams()` (`config/core/v3/protocol.pb.go:283`), `GetInitialStreamWindowSize()` (`:290`), `GetIdleNetworkTimeout()` (`:332`), etc.
- **`envoy.transport_sockets.quic` — `QuicDownstreamTransport`** → `GetDownstreamTlsContext() *tls.v3.DownstreamTlsContext` (`extensions/transport_sockets/quic/v3/quic_transport.pb.go:71`) + `GetEnableEarlyData()` (`:78`). Field 1 is the mandatory TLS context (§3.4).
- **`HttpConnectionManager.http3_protocol_options`** → `GetHttp3ProtocolOptions() *core.v3.Http3ProtocolOptions` (`http_connection_manager.pb.go:942`, field 44).
- **`filter_chain_match.transport_protocol: "quic"`** — the value lifted at `manager.go:685-689`.

### 3.2 The listen path — a NEW UDP/QUIC accept path parallel to the TCP one (D-H3-LISTEN-SEAM, DERIVED)

The entire existing serve path is TCP-stream-shaped and never gated on the socket_address protocol enum: `Start` hardcodes `net.Listen("tcp", rt.addr)` (`manager.go:862`); `acceptLoop` calls `ln.Accept()` returning `net.Conn` (`manager.go:925`); `serveConnection` (`manager.go:996-1073`) runs the listener-filter → chain-match → `stdtls.Server` handshake → terminal `serveNetworkChain` (`manager.go:1101`) pipeline on that `net.Conn`. QUIC has no stream `net.Conn` to `Accept()` — it binds a UDP `net.PacketConn` and accepts QUIC *connections* multiplexing many streams.

**Recommended seam (DERIVED from source; IMPL re-decides):** a discriminated `kind` field on `listenerRuntime` (`manager.go:129-167`). The runtime cleanly separates transport-AGNOSTIC built state — `chainSpecs`/`defaultSpec`/`chainByName` (`:140-143`), `netChainFactory` (`chainInfo`, `:106-117`), the per-chain `tlsCfg` (`:108`), and the metric fields `downstreamCxTotal`/`downstreamCxActive` (`:162-163`) — from the two transport-SPECIFIC methods (the `Start` bind at `:862` and `acceptLoop`/`serveConnection` at `:917`/`:996`). The IMPL: (a) read the socket_address protocol in `buildListenerRuntimeWithCtx` (`manager.go:340`) into a new `kind` field; (b) branch in `Start` at `:862` (`net.Listen("tcp",…)` vs `net.ListenUDP` + a quic-go listener); (c) add a sibling `quicAcceptLoop`/`serveQUICConnection` reusing the same `chainByName`/`netChainFactory`/metric fields. `registerListenerMetrics` (`:313-317`) is fully reused. `parseChainSpec` (`:662`) and the transport_socket→`*tls.Config` decode (`:379-390`) are transport-agnostic and feed the QUIC path identically.

**Key coupling points to resolve at the IMPL (flagged, DERIVED):**
1. `acceptLoop`/`serveConnection`/`serveNetworkChain` thread `net.Conn` end-to-end (`:925`, `:996`, `:1101`) — QUIC needs its own accept/dispatch methods OR a quic-stream→`net.Conn` adapter for the terminal `network.ChainRuntime` handoff.
2. NO ClientHello peek for QUIC — the listener-filter/tls_inspector peek model (`:1007-1039`) has no QUIC analog; SNI/ALPN arrive from the integrated QUIC handshake, so chain-match inputs must be sourced from the QUIC handshake, not a peeked byte stream.
3. TLS handshake LOCATION differs — TCP does `stdtls.Server` post-chain-select (`:1059-1067`); QUIC handshakes at the transport layer before a stream exists, so per-chain `tlsCfg` selection needs a quic-native `GetConfigForClient`-style callback rather than the current select-then-handshake ordering. The minimal single-chain slice (§2) side-steps SNI dispatch by using the default chain's `tlsCfg`.
4. `Stop` closes the `net.Listener` field (`:1296`); the field is typed `net.Listener` (`:132`) — a UDP/QUIC listener needs an interface-widened field or a separate close path.

### 3.3 The H3 codec + HCM adapter — a THIRD dispatch arm modeled on the H2 arm (D-H3-HCM-SEAM, DERIVED)

The codec-agnostic per-request dispatch seam ALREADY EXISTS. Both existing codecs re-implement the same ~10-step recipe over a shared, `net.Conn`-free core:

- Top-level fork `Filter.Handle(ctx, downstream net.Conn)` switches on `f.codecType` (`internal/filter/hcm/filter.go:112`): `HTTP1 → runConnection` (`:121`), `HTTP2 → runH2` (`:124`).
- The H2 arm is the existence proof of a `net.Conn`-free seam: `runH2` (`filter.go:153`) pre-captures conn/TLS/addr state ONCE onto an `h2Dispatcher` (`filter.go:155-187`), then per stream `chainDispatchAction.WriteH2(ctx, h2req h2.H2Request, sw h2.StreamWriter)` (`h2dispatch.go:269`) — **takes NO `net.Conn`** — builds `filter_http.NewFilterChain` (`h2dispatch.go:327`), casts the terminal filter to `*router.Filter`, `SetAction`/`SetRequest`, `chain.RunDecodeHeaders(ctx, http.Header, endStream)` (`h2dispatch.go:508`), `RunAction` (`router.go:297`), reads the codec-agnostic `router.ActionResponse{Status, Headers, Body}` (`router.go:79`), and writes via the `h2.StreamWriter` abstraction (`h2/stream.go:31`).
- The shared core (`filter_http.FilterChain` over `http.Header`, `router.Filter`, `router.ActionResponse`) is codec-agnostic and untouched by codec choice.

**The H3 arm (leg 61.2):** quic-go's `http3.Server` yields `(*http.Request, http.ResponseWriter)` per request. The request side is the EASIEST of the three codecs — the H1 arm already runs the entire chain on a native `*http.Request` (`connection.go:312`+); an H3 handler calls `SetAction`/`SetRequest(req)` + `RunDecodeHeaders(req.Header,…)` exactly like H1. Because the router's `Action`/`H2Action` split is about the UPSTREAM protocol (`router.go:116` vs `:136`), an H3-DOWNSTREAM arm reuses the existing H1 `Action` path — plausibly ZERO `router.go` changes. The response side is a small `http.ResponseWriter → writeH3Reply` adapter mapping `ActionResponse{Status,Headers,Body}` to `w.Header()` set + `w.WriteHeader(status)` + `w.Write(body)`. Body streaming: quic-go's `req.Body io.ReadCloser` slots into the H1 read-into-chain loop (`connection.go:608-656`); no per-stream flow control is exposed to the HCM seam (QUIC handles it below `http.ResponseWriter`, exactly as the internal h2 codec hides HTTP/2 flow control below `StreamWriter`).

The bounded work items (a new arm, NOT surgery): (a) a `codecType == HTTP3` branch in `Handle` (`filter.go:119`) or a `runH3` sibling; (b) capture conn/TLS state once (quic-go exposes `req.TLS`) and seed the chain via the existing `chain.SetX` setters; (c) the `http.ResponseWriter → writeH3Reply` adapter; (d) a third `emitAccessLogH3` arm (`accesslog_emit.go`, symmetric to `emitAccessLog` `:25` / `emitAccessLogH2` `:85`, reading from `*http.Request`, `Protocol="HTTP/3"`).

**Honest uncertainty (DERIVED, not PINNED):** no H3/quic scaffolding exists in `internal/filter/hcm/` today (only `h2/`); the above is a design judgment from the H1/H2 structure. The one structural asymmetry a reviewer must weigh: each codec re-implements the full recipe, so "thin adapter" = "a new ~200-line arm," not "wire quic-go into one pre-existing entry function" (that single shared entry function does not exist). Whether quic-go's `http3.Server` handler model gives sufficient control (exact header casing/ordering, 1xx, trailers) at the depth envoy-go's HCM needs is the leg-61.2 IMPL's own investigation — the minimal GET-→-200 slice needs none of those advanced surfaces.

### 3.4 Mandatory TLS — reuse the existing cert substrate; quic-go consumes the `*stdtls.Config` (D-H3-TLS, PINNED)

QUIC bakes TLS 1.3 into the transport; there is no plaintext H3 (the reference boot-rejects a QUIC listener with no transport socket, §6 arm reject-C). The `QuicDownstreamTransport` wraps a `DownstreamTlsContext`, so the listener MUST carry a cert + key and advertise ALPN `h3`. This REUSES `internal/tls.NewDownstreamConfig(ts, baseDir)` (`internal/tls/config.go:34`) — which already parses `tls_certificates` and sets `cfg.NextProtos = append(cfg.NextProtos, c.GetAlpnProtocols()...)` (`config.go:208`) — producing a `*stdtls.Config` that quic-go's listener consumes directly (the SPEC-61 probe drove a `*tls.Config{NextProtos:["h3"]}` client against exactly this; §11). The IMPL threads the parsed `chainInfo.tlsCfg` (`manager.go:108`) into the quic-go listener rather than into `stdtls.Server`. The reference negotiated ALPN `h3` + TLS 1.3 (§11 arm h3-get). NOTE: the reference's transport socket wraps the TLS context UNDER `envoy.transport_sockets.quic` (not the bare `envoy.transport_sockets.tls`), so leg 61.1 must dispatch the quic transport-socket type-URL to unwrap the inner `DownstreamTlsContext` before calling `NewDownstreamConfig`.

### 3.5 Byte-stability — no behavior change on any existing TCP path

No existing fixture configures a UDP/QUIC listener; the new listener KIND is only reached by a `udp_listener_config` + `codec HTTP3` config. The 103-dir differential stays byte-stable — the new fixture `0102` (leg 61.3) is the only dir exercising the H3 path. The two lifted rejects (`transport_protocol "quic"`, `codec_type HTTP3`) were forward-looking guards; lifting them changes NO accepted-config behavior (they only ever produced errors).

---

## 4. Framework primitives — +1 or +2 packages; +1 new go.mod module (the FIRST ever)

- **New packages (best estimate +2):** `internal/listener/quic` (the UDP/QUIC accept path, leg 61.1) + `internal/filter/hcm/h3` (the codec/adapter, sibling of `internal/filter/hcm/h2`, leg 61.2). The IMPL pins the boundary (the listen path could fold into `manager.go`; the codec could fold into an existing package) — best estimate +2, floor +1.
- **New go.mod module (+1 direct, the FIRST ever):** `github.com/quic-go/quic-go v0.54.1` (§2.4). Transitive additions: `github.com/quic-go/qpack v0.5.1`, `golang.org/x/crypto` (CONFIRMED absent from `go.mod` today — grep count 0; a NEW transitive module), and version bumps to `golang.org/x/net`/`golang.org/x/sys`/`golang.org/x/sync` (already present). `go mod tidy -diff` becomes NON-EMPTY for the FIRST time in the project's history — the six-gate's `go mod tidy -diff` gate expectation flips for this phase's IMPL legs.
- **NO new proto module** — the QUIC/H3 protos all resolve in the EXISTING `go-control-plane/envoy v1.32.4` (§3.1, §11).

### 4.1 The `quic-go` version decision (D-H3-QUICLIB, PINNED)

The project's `go.mod` pins `go 1.23.0`. quic-go release/toolchain matrix (fetched from the module proxy this session): v0.54.1 requires `go 1.23`; **v0.55.0+ requires `go 1.24`; v0.60.0 (latest) requires `go 1.25`**. **Recommended: v0.54.1** — the NEWEST release that keeps the project's `go 1.23` directive UNCHANGED, avoiding a toolchain-directive bump as a side effect of the family opener. The SPEC-61 probe PROVED v0.54.1's `http3.Transport` interoperates with reference Envoy contrib-v1.37.2's H3 listener (§11). If the IMPL prefers a newer quic-go, it MUST also bump the `go.mod` `go` directive (a separate, larger decision to record in the ADR). quic-go is the de-facto production Go QUIC/HTTP-3 stack (caddy et al.), actively maintained — acceptable supply-chain posture, recorded in ADR-0279 §Context.

---

## 5. Proto-field roster — the H3/QUIC wire surface (RE-DERIVED @ go-control-plane/envoy v1.32.4)

| Field | Getter (file:line) | Phase-61 disposition |
|---|---|---|
| `Listener.udp_listener_config` | `config/listener/v3/listener.pb.go:606` | REQUIRED to mark the listener UDP/QUIC |
| `UdpListenerConfig.quic_options` | `config/listener/v3/udp_listener_config.pb.go:87` | REQUIRED (its presence = QUIC/H3); empty `{}` accepted (§11) |
| `UdpListenerConfig.downstream_socket_config` | `udp_listener_config.pb.go:80` | accepted-and-ignored (empty `{}` in the probe) |
| `QuicProtocolOptions.*` (idle_timeout, max_concurrent_streams, proof_source_config, connection_id_generator_config, reject_new_connections, …) | `config/listener/v3/quic_config.pb.go:124-208`; `config/core/v3/protocol.pb.go:283-339` | accepted-and-ignored OR strict-rejected at the minimal slice (§6, ADR-0080) |
| `QuicDownstreamTransport.downstream_tls_context` | `extensions/transport_sockets/quic/v3/quic_transport.pb.go:71` | REQUIRED (mandatory TLS, §3.4) |
| `QuicDownstreamTransport.enable_early_data` | `quic_transport.pb.go:78` | 0-RTT — DEFERRED (§2); strict-reject if set true |
| `HttpConnectionManager.http3_protocol_options` | `http_connection_manager.pb.go:942` (field 44) | accepted; empty `{}` in the probe |
| `filter_chain_match.transport_protocol == "quic"` | `manager.go:685-689` (reject lifted) | ACCEPT `"quic"` for a QUIC chain |
| `HttpConnectionManager_HTTP3` (codec enum) | `hcm config.go:231-240` (reject lifted) | ACCEPT on a QUIC listener (reject on a non-QUIC listener, §6) |

---

## 6. Bootstrap-wire REQUIRED/ignored/strict-reject roster + fuzzer (D-H3-BOOTSTRAP-WIRE)

**Required-vs-ignored (PINNED via §11 arm h3-get — the minimal accept config used `quic_options:{}`, `downstream_socket_config:{}`, `http3_protocol_options:{}`, all empty and accepted):** the minimal viable config is `udp_listener_config{quic_options:{}}` + a `filter_chain` with `transport_protocol: "quic"` (optional match) + a `QuicDownstreamTransport{downstream_tls_context{common_tls_context{tls_certificates[…], alpn_protocols:["h3"]}}}` transport socket + an HCM with `codec_type: HTTP3` + `http3_protocol_options:{}` + a router. REQUIRED: `udp_listener_config` (marks UDP), a QUIC transport socket (mandatory TLS), a cert, `codec_type HTTP3`. IGNORED-at-minimal-slice: all `quic_options`/`downstream_socket_config`/`http3_protocol_options` sub-fields.

**Config-parity boot-rejects (BOTH the reference and envoy-go reject — NOT a departure; PINNED §11 arms reject-B / reject-C):**
- `codec_type: HTTP3` on a non-QUIC (TCP) listener → the reference boot-rejects `HTTP/3 codec configured on non-QUIC listener.` envoy-go mirrors: `hcm: codec_type HTTP3 requires a QUIC (udp_listener_config) listener` (ADR-0080-distinct).
- a `udp_listener_config` (QUIC) listener with NO transport socket → the reference boot-rejects `no transport socket specified for connection oriented UDP listener`. envoy-go mirrors: `listener: quic listener requires a transport_socket (mandatory TLS)` (ADR-0080-distinct).

**envoy-go-strict DEPARTURES (ADR-0080 — unsupported quic sub-options reject loudly with distinct substrings):** the minimal slice consumes none of the `quic_options`/`QuicDownstreamTransport` tuning sub-fields; each that the reference honors but envoy-go does not yet implement STRICT-REJECTS rather than silently ignoring (the project's incremental-arm posture). The exact per-field reject arms are DECIDED at the leg-61.1 IMPL (best estimate: `enable_early_data`, `proof_source_config`, `connection_id_generator_config`, `reject_new_connections`, and the flow-control/idle-timeout knobs → distinct-substring rejects). Absent/empty sub-fields take the accept path (mirroring the probe).

**Fuzzer (D-H3-FUZZSEED).** HTTP/3 frame parsing + QPACK are quic-go-INTERNAL (not hand-rolled) — no new frame-parser fuzzer. The only hand-rolled parse is the bootstrap `udp_listener_config`/`quic_options`/transport-socket config, which is reachable from the existing listener/HCM config parse. If the leg-61.1 IMPL adds a config-parse SEED it joins an EXISTING fuzzer (no new `func Fuzz`). Anticipated fuzzers **54 → 54 (+0)** (`reference_fuzzer_count_docs_drift` — reconcile actual `^func Fuzz` = 54 before AND after).

---

## 7. Stat surface — +N (SPEC-TBD, IMPL-pinned per leg; recommend deferring granular `http3.*` counters)

**PINNED reference surface (§11 arm h3-get /stats, after ONE H3 GET):** the reference emitted, for the H3 listener (`stat_prefix: h3`, listener `0.0.0.0:10000`):
- `http.h3.downstream_cx_total: 1`, `http.h3.downstream_cx_http3_total: 1`
- `http.h3.downstream_rq_total: 1`, `http.h3.downstream_rq_2xx: 1`, `http.h3.downstream_rq_completed: 1`, `http.h3.downstream_rq_http3_total: 1`
- `listener.0.0.0.0_10000.downstream_cx_total: 1`
- `http3.quic_version_rfc_v1: 1` (+ the zero-valued `http3.*` family: `rx_reset`, `tx_reset`, `tx_flush_timeout`, `dropped_headers_with_underscores`, `metadata_not_supported_error`, `requests_rejected_with_underscores_in_headers`, `quic_version_h3_29`).

**envoy-go posture:** the per-listener `downstream_cx_total` shape and the HCM `downstream_rq_2xx`/`downstream_rq_completed`/`downstream_rq_total` shapes ALREADY exist in envoy-go's registry (registered per-listener / per-stat_prefix) — a new listener KIND that reuses `registerListenerMetrics` (`manager.go:313`) adds NO new stat-SURFACE entries for those. The genuinely NEW shapes would be the H3-specific `downstream_cx_http3_total`/`downstream_rq_http3_total` (+ optionally the `http3.*` family). **Recommendation:** the minimal slice registers the reused per-listener `downstream_cx_total` (+0 surface) and, at most, the two `*_http3_total` counters (+2); it DEFERS the granular `http3.*` robustness counters (`rx_reset`/`tx_reset`/etc.) to a QUIC-robustness follow-on. The cross-side fixture asserts a NAMED SUBSET (`reference_stats_sink_emits_used_only` — the reference omits unincremented counters). Any wire-derived stat segment (a per-listener address) passes `stats.IsValidName` before registration (`reference_dynamic_stat_name_charset_guard`). **Anticipated stat surface 1201 → 1201 + N, N ∈ [0, ~6], IMPL-pinned per leg** (61.1 the listener/handshake stats; 61.2 the HCM request stats).

---

## 8. Differential fixture + harness surgery (D-H3-DIFF-HARNESS, DERIVED) — +1 fixture; leg 61.3

**The harness is TCP-ONLY** (RE-DERIVED this session). `test/differential/harness.go` hardcodes `/tcp` in every exposure/mapping across THREE container-starters: `StartReferenceProxy` (`harness.go:108` `exposed := []string{"9901/tcp"}`, `:110` `%d/tcp`, `:138`/`:145`/`:150` MappedPort `.../tcp`, state field `tcpAddrs` `:100`), `StartReferenceProxyWithMounts` (`:171-219`), and `tryStartReferenceProxy` (`:423-445`). There is ZERO HTTP/3 client anywhere in `test/` (grep confirmed — the only `http3`/`quic` hits are the `h3spec` conformance stub `test/conformance/doc.go:7`, the `http2-http3-framing` stat token, the Lua `:protocol()` `"HTTP/3"` string, the two reject-assertions, and `go.sum` `quicktest` — all unrelated).

**The surgery (leg 61.3):**
1. **UDP port exposure/mapping** — in all three starters: append `fmt.Sprintf("%d/udp", p)` to `exposed`; add a `udpAddrs map[int]string` field beside `tcpAddrs` (`harness.go:100`); add a UDP `MappedPort(ctx, nat.Port(fmt.Sprintf("%d/udp", p)))` loop; a `ListenerUDPAddr(containerPort int)` accessor beside `:227`. Admin/readiness stays `9901/tcp` (`:121`). `nat.Port` (already imported `:19`) and testcontainers-go both accept the `/udp` form — no new hook needed. Thread UDP ports through the runner's `refPorts` allocation (`runner_test.go:1140-1145`) with an H3 sibling of `ReferenceListenerPort()`.
2. **The shared H3 client** — a NEW `test/helpers/h3.go` `H3RoundTrip(ctx, addr, tlsConf, method, path, headers, body)` on quic-go's `http3.Transport` (FREE once the module landed in 61.1), modeled on `test/helpers/h2.go:33` `H2RoundTrip` (single-shot, `addr`-parameterized, side-agnostic, fresh transport per call for RR determinism). ONE client drives BOTH sides — reference via `ref.ListenerUDPAddr(...)`, subject via its UDP `ListenerAddr(...)`. The client MUST pin the dialed `host:port` and IGNORE Alt-Svc/server-preferred-address (the container advertises its INTERNAL port, not the host-mapped one) — exactly as `H2RoundTrip` pins its dialed addr.
3. **The fixture** `test/fixtures/0102-http3-downstream-get` (fixtures **103 → 104**): a cross-side H3 GET; assert the response (status 200 + body) + a NAMED downstream-stat subset (§7) via `StatsAsserter`. `BackendCount() ≥ 1` (`reference_differential_backendcount_min_one`); reuse an existing HTTP echo/responder BackendKind for a routed GET (BackendKind stays **38**) or use `direct_response` (no backend, throwaway BackendKind). Assert each independent property with `Errorf`, NOT `Fatalf` (`reference_fatalf_makes_assertions_unreachable`); prove each assertion LIVE with a `-count=1` deliberate break, confirming WHICH fires (`reference_deliberate_break_wrong_assertion`, `reference_differential_break_protocol_count1`); use the `TestDifferential/0102-<slug>` `-run` selector (`reference_differential_run_selector`).

**Docker UDP-reachability RISK (flagged, `reference_docker_probe_bridge_network` / `reference_host_gateway_ip_docker_desktop`):** the codebase documents container→host UDP as verified (phase-48, `harness.go:498-502`), but host→container UDP publishing for QUIC is the UNTESTED direction. On Linux bridge (the CI substrate) the kernel does the NAT and QUIC over a published `-p host:container/udp` port works (PROVEN this session — the SPEC-61 probe drove H3 over exactly a published UDP port). On Docker Desktop the userland UDP proxy is a residual risk; the leg-61.3 IMPL verifies non-vacuously (the H3 client actually completes a request, `reference_docker_probe_bridge_network`).

**Subject side needs NO Docker change** — it binds the UDP socket directly and reports the addr via the ready-sentinel (`harness.go:69` regex accepts any `\S+` token). Uncertainty flagged: no UDP free-port helper exists in `test/` today (only TCP `net.Listen`); the subject's UDP ready-sentinel emission is unverified (it has no H3 listener today) — both are leg-61.3 IMPL work.

---

## 9. Behavior-contract delta (`docs/envoy-go/BEHAVIOR_CONTRACT.md`; atomic landing at each leg's IMPL)

A NEW HTTP/3 section: a downstream QUIC/H3 listener (`udp_listener_config.quic_options` + an `envoy.transport_sockets.quic` transport socket wrapping a `DownstreamTlsContext` with ALPN `h3` + `codec_type HTTP3`) is SUPPORTED for a GET into the existing HCM/router/filter path; mandatory TLS 1.3; the two config-parity boot-rejects (HTTP3-on-non-QUIC, QUIC-without-transport-socket) mirror the reference; unsupported `quic_options`/transport-socket tuning sub-fields STRICT-REJECT loudly (envoy-go-strict DEPARTURE, ADR-0080); upstream H3 / alt-svc / 0-RTT / h3spec / QUIC robustness DEFERRED. Exact wording RE-DERIVED and written at each leg's IMPL (the SPEC does NOT edit shared docs).

---

## 10. Test plan + per-leg task structure (multi-leg; the PLAN(s) decompose)

TDD (`superpowers:test-driven-development`); each task a red→green with a `-count=1` liveness break where an assertion is load-bearing.

**Leg 61.1 (QUIC/UDP listen + module):** (1) add `quic-go v0.54.1` to `go.mod`; `go mod tidy` (the FIRST non-empty `go mod tidy -diff`). (2) parse `udp_listener_config`/`quic_options`/the quic transport socket in `buildListenerRuntimeWithCtx`; the `kind` discriminant. (3) lift the `transport_protocol "quic"` reject (`manager.go:685-689`); UPDATE the existing reject-assertion test (`manager_test.go:1187-1196`). (4) the UDP `net.ListenUDP` + quic-go listener bind branch in `Start`; `quicAcceptLoop`; the reused `downstream_cx_total` registration. (5) build the `*stdtls.Config` (unwrap the quic transport socket → `NewDownstreamConfig`, ALPN `h3`). (6) the config-parity + strict-reject arms (§6, distinct substrings). (7) a subject-side handshake integration test (a local quic-go client completes the QUIC/`h3`-ALPN handshake). (8) verify (six-gate; `go mod tidy -diff` now EXPECTED non-empty). ADR-0279 body.

**Leg 61.2 (H3 codec + HCM adapter):** (1) the `internal/filter/hcm/h3` codec arm (a `runH3`/`WriteH3` modeled on `runH2`/`WriteH2`, §3.3); the `http.ResponseWriter → writeH3Reply` adapter. (2) lift the `codec_type HTTP3` reject (`config.go:231-240`); UPDATE `config_test.go:207-208`. (3) `emitAccessLogH3` (`accesslog_emit.go`, `Protocol="HTTP/3"`). (4) a subject-side H3 GET→200 test (a local quic-go `http3.Transport` client → routed backend). (5) verify. A per-leg ADR OR fold into ADR-0279.

**Leg 61.3 (fixture + harness surgery):** (1) harness UDP exposure/mapping (three starters, §8). (2) `test/helpers/h3.go` `H3RoundTrip`. (3) the `0102-http3-downstream-get` cross-side fixture; the span/stat assertions proven live. (4) verify (the full 104-dir differential, byte-stable except `0102`). (5) BEHAVIOR_CONTRACT HTTP/3 section; ROADMAP row 61 → `done` (ALL legs landed, `reference_roadmap_split_phase_row_done`); the deferred sentence; STATE; router roll.

(The PLAN(s) may re-partition — e.g. fold 61.3 into 61.2 if the harness surgery is small, or land 61.3's harness surgery first as an infra prerequisite so 61.2 proves cross-side.)

---

## 11. SPEC-time empirical-pin block (D-H3-* live probes — executed IN-SESSION 2026-07-12, `envoyproxy/envoy:contrib-v1.37.2`, FRESH container per arm)

Each arm ran a fresh `envoyproxy/envoy:contrib-v1.37.2` container (unique `probe61-*` names, ephemeral host ports; QUIC published as `-p 127.0.0.1:<port>:10000/udp`). The H3 client was a THROWAWAY Go program (`probe61/`, its OWN isolated go.mod — NOT the repo's) using `github.com/quic-go/quic-go v0.54.1`'s `http3.Transport`. Verified NON-vacuous (the client completed a real H3 request; the /stats showed `downstream_rq_2xx: 1`). Deleted after — this SPEC is docs-only.

**Arm h3-get (D-H3-QUICLIB + D-H3-BOOTSTRAP-WIRE accept + D-H3-TLS + D-H3-STATS).** Config: a UDP listener `udp_listener_config{quic_options:{}, downstream_socket_config:{}}` + a `QuicDownstreamTransport{downstream_tls_context{common_tls_context{tls_certificates:[{cert.pem,key.pem}], alpn_protocols:["h3"]}}}` transport socket + an HCM `codec_type:HTTP3`, `http3_protocol_options:{}`, a `direct_response{status:200, body:"h3-ok"}` route + the router. The reference BOOTED (`/ready ⇒ LIVE`, `loading 1 listener(s)`, workers started, NO config error). The quic-go client GET `https://127.0.0.1:<udp>/probe/path` returned:
```
proto=HTTP/3.0  status=200  body="h3-ok"  NEGOTIATED-ALPN="h3"  TLS-version=0x304 (TLS 1.3)
```
⇒ quic-go v0.54.1 interoperates with reference Envoy H3; the minimal empty-`{}`-sub-fields config is ACCEPTED; ALPN `h3` + TLS 1.3. The /stats surface is §7.

**Arm reject-B (D-H3-BOOTSTRAP-WIRE — HTTP3-on-non-QUIC).** Config: the same HCM `codec_type:HTTP3` but on a plain TCP `socket_address` (NO `udp_listener_config`). The reference BOOT-REJECTED:
```
error `HTTP/3 codec configured on non-QUIC listener.` initializing config
```
⇒ `codec_type HTTP3` REQUIRES a QUIC listener; envoy-go mirrors as a config-parity reject (§6).

**Arm reject-C (D-H3-BOOTSTRAP-WIRE — QUIC-without-TLS).** Config: the QUIC `udp_listener_config{quic_options:{}}` listener with the `transport_socket` block REMOVED (no quic transport socket). The reference BOOT-REJECTED:
```
error `error adding listener '0.0.0.0:10000': no transport socket specified for connection oriented UDP listener` initializing config
```
⇒ a QUIC listener REQUIRES a transport socket (mandatory TLS); envoy-go mirrors as a config-parity reject (§6).

**quic-go version/toolchain matrix (RE-DERIVED from the Go module proxy this session).** `v0.54.1` go.mod: `go 1.23` (+ `quic-go/qpack v0.5.1`, `golang.org/x/crypto v0.26.0`, `x/net v0.28.0`, `x/sys v0.23.0`). `v0.55.0`: `go 1.24`. `v0.60.0` (latest, 2026-06-04): `go 1.25.0`. ⇒ v0.54.1 is the LAST release keeping the project's `go 1.23.0` directive unchanged (§4.1). The probe built + interoperated with v0.54.1.

**Source cross-checks (RE-DERIVED @ master `cbda648b`).** The `transport_protocol "quic"` reject at `manager.go:685-689`; the `codec_type HTTP3` reject at `config.go:231-240` (`hcm: codec_type %s is not supported in phase 05.1`); the TLS `NextProtos`-from-`alpn_protocols` at `internal/tls/config.go:208`; the H2 `net.Conn`-free dispatch seam `WriteH2` at `h2dispatch.go:269`; the QUIC/H3 protos all resolve in `go-control-plane/envoy v1.32.4` (`go list` clean).

---

## 12. Edit-site roster (RE-DERIVED against master `cbda648b`; the exact set depends on the split-leg boundaries §3.0)

**Leg 61.1 — listen path (`internal/listener/manager.go` + NEW `internal/listener/quic`):**
- `manager.go:685-689` — LIFT the `transport_protocol "quic"` reject (accept `"quic"`). [EDIT]
- `manager.go:340` (`buildListenerRuntimeWithCtx`) — parse `udp_listener_config`/`quic_options`/the quic transport socket; a `kind` discriminant. [EDIT]
- `manager.go:129-167` (`listenerRuntime`) — ADD a `kind` field (+ possibly widen the `netLn` field `:132`). [EDIT]
- `manager.go:862` (`Start`) — branch on `kind` (`net.Listen("tcp",…)` vs `net.ListenUDP` + a quic-go listener). [EDIT]
- `manager.go` — ADD `quicAcceptLoop`/`serveQUICConnection` (sibling of `:917`/`:996`); reuse `registerListenerMetrics` (`:313`). [ADD]
- NEW `internal/listener/quic/*` — the UDP/QUIC accept + per-stream handoff. [ADD — new package]
- `internal/listener/manager_test.go:1187-1196` — UPDATE the `transport_protocol "quic"` reject-assertion (now accept, or a strict-reject on a sub-option). [EDIT]

**Leg 61.2 — H3 codec + HCM adapter (NEW `internal/filter/hcm/h3` + edits):**
- `internal/filter/hcm/config.go:231-240` — LIFT the `codec_type HTTP3` reject; add the non-QUIC-listener config-parity reject (§6). [EDIT]
- `internal/filter/hcm/filter.go:112` (`Handle`) — ADD a `codecType == HTTP3` branch / `runH3`. [EDIT]
- NEW `internal/filter/hcm/h3/*` — the codec/adapter (`runH3`/`WriteH3` + `writeH3Reply`, §3.3). [ADD — new package]
- `internal/filter/hcm/accesslog_emit.go` — ADD `emitAccessLogH3` (sibling of `:25`/`:85`). [ADD]
- `internal/filter/hcm/config_test.go:207-208` (`TestParseFilter_CodecTypeHTTP3`) — UPDATE (was: HTTP3 rejected; now: accepted on a QUIC listener / rejected on a non-QUIC listener). [EDIT]

**TLS (leg 61.1):**
- REUSE `internal/tls.NewDownstreamConfig` (`internal/tls/config.go:34`, `NextProtos` at `:208`) — unwrap the quic transport socket's inner `DownstreamTlsContext`. [REUSE + a quic transport-socket type-URL dispatch]

**Module + build (leg 61.1):**
- `go.mod`/`go.sum` — ADD `github.com/quic-go/quic-go v0.54.1` (+ transitive `quic-go/qpack`, `golang.org/x/crypto`; bumps to `x/net`/`x/sys`/`x/sync`). [EDIT — the FIRST new module]

**Harness + fixture (leg 61.3):**
- `test/differential/harness.go:100,108,110,138,145,150,155` (+ `171-219`, `423-445`) — UDP exposure/mapping (three starters) + `udpAddrs` + `ListenerUDPAddr`. [EDIT]
- `test/differential/runner_test.go:1140-1145` — thread UDP ref ports. [EDIT]
- NEW `test/helpers/h3.go` — `H3RoundTrip` on quic-go `http3` (modeled on `test/helpers/h2.go:33`). [ADD]
- NEW `test/fixtures/0102-http3-downstream-get/` — envoy.yaml, envoy-go.yaml, expectations.yaml, driver, README. [ADD]

**Docs (per-leg IMPL — NOT this SPEC):**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the HTTP/3 section (§9). [EDIT — IMPL]
- `docs/envoy-go/ROADMAP.md` — row 61 → `done` once ALL legs land; the deferred sentence. [EDIT — final IMPL]
- `docs/envoy-go/STATE.md` — active-phase header. [EDIT — each stage, controller]
- `docs/envoy-go/DECISIONS.md` — ADR-0279 §Context here (§13); §Decision/§Consequences + any per-leg ADRs at the IMPL. [ADD — IMPL]

---

## 13. ADR continuity — the ADR-0279 §Context DRAFT (anchored here; full entry at the phase-61 IMPL legs)

**ADR-0279 §Context (draft).** envoy-go had ZERO QUIC/HTTP-3/UDP-listen substrate through 60 phases: a `Listener`'s `filter_chain_match.transport_protocol: "quic"` was rejected (`internal/listener/manager.go:685-689`) and an HCM `codec_type: HTTP3` was rejected (`internal/filter/hcm/config.go:231-240`) as forward-looking guards. The ROADMAP §HTTP/3-family stub charters "quic-go transport, downstream H3 listener, upstream H3 cluster, `h3spec` gate." Phase 61 OPENS the family with the smallest defensible slice — a minimal TLS-mandatory downstream HTTP/3 listener serving a GET into the EXISTING HCM → router → filter-chain → upstream path. It stands up the family's novel substrate: (a) the FIRST new go.mod module `github.com/quic-go/quic-go` (v0.54.1 — the last release keeping the project's `go 1.23` directive; SPEC-61 §11 PROVED its `http3.Transport` interoperates with reference Envoy contrib-v1.37.2's H3 listener — `HTTP/3.0`, 200, ALPN `h3`, TLS 1.3 — de-risking the family's core interop unknown); `go mod tidy -diff` becomes NON-EMPTY for the first time; (b) the FIRST UDP/packet listen path parallel to the TCP `net.Listen` path — a discriminated-`kind` `listenerRuntime` (DERIVED §3.2) reusing the transport-agnostic chain-build + `registerListenerMetrics` + `netChainFactory`, with a sibling `quicAcceptLoop`/`serveQUICConnection`; the QUIC-native handshake sources SNI/ALPN from the integrated handshake (no ClientHello peek) and selects the per-chain `*stdtls.Config` (built by the reused `internal/tls.NewDownstreamConfig`, ALPN `h3`) at the transport layer; (c) the FIRST H3 codec — a THIRD dispatch arm (`internal/filter/hcm/h3`) modeled on the H2 arm, whose seam ALREADY EXISTS (the H2 `WriteH2` takes NO `net.Conn`, feeding the shared `filter_http` chain from a decoded request + a response-writer abstraction), so H3 adapts quic-go's `http3.Server` `(*http.Request, http.ResponseWriter)` via the H1 `Action` path (plausibly ZERO `router.go` changes) + a small `writeH3Reply` adapter + an `emitAccessLogH3` arm — an ADDITIVE arm, NOT framework surgery (the BRAINSTORM's central-uncertainty DE-RISKING, §1.1). Mandatory TLS 1.3 (SPEC-61 §11 PINNED the reference boot-rejects a QUIC listener with no transport socket). All QUIC/H3 protos resolve in the EXISTING go-control-plane v1.32.4 (NO new proto module). Two config-parity boot-rejects (HTTP3-on-non-QUIC-listener, QUIC-without-transport-socket — both PINNED §11) mirror the reference; unsupported `quic_options`/transport-socket tuning sub-fields STRICT-REJECT loudly (ADR-0080, distinct substrings). A MULTI-LEG SPLIT (61.1 listen+module / 61.2 codec+HCM / 61.3 fixture+harness — the ADR-0045 escape-valve CONSUMED); the cross-side proof is one H3-GET fixture (`0102`) over a harness extended for UDP publishing + a shared quic-go `http3` client (the harness's FIRST non-TCP transport). §Decision/§Consequences land at the phase-61 IMPL leg(s) per ADR-0044; per-leg ADRs are plausible (the greenfield transport's listen-path + codec-arm seams). ANCHORS ADR-0279. Next-free after: ADR-0280.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

**Counts UNCHANGED at this SPEC (docs-only; re-verified against master tip `cbda648b`):** stat surface **1201** · fixtures **103** · fuzzers **54** · BackendKind **38** · DECISIONS tail **ADR-0276** · new Go packages **0** · new go.mod modules **0**.

**Anticipated at the phase-61 IMPL leg(s) (best estimates; the IMPL pins):** stat surface **1201 → 1201 + N** (N ∈ [0, ~6], IMPL-pinned per leg; recommend deferring granular `http3.*` counters) · fixtures **103 → 104** (`0102-http3-downstream-get`, leg 61.3) · fuzzers **54 → 54 (+0)** · BackendKind **38 → 38 (+0)** (reuse an HTTP responder / `direct_response`) · new Go packages **+1 or +2** (`internal/listener/quic` [+ `internal/filter/hcm/h3`]) · **new go.mod modules +1** (`github.com/quic-go/quic-go v0.54.1`, the FIRST — plus transitive `quic-go/qpack` + `golang.org/x/crypto`; `go mod tidy -diff` NON-EMPTY for the first time) · DECISIONS tail advances to **ADR-0279** (next-free **ADR-0280**; per-leg ADRs plausible).

**ROADMAP/STATE at SPEC-DONE:** row 61 STAYS `in-progress` (a row/leg flips `done` only at its IMPL six-gate, ADR-0106 + `reference_roadmap_split_phase_row_done`). The LIVE deferred sentence is UNCHANGED at this SPEC. STATE active-phase header flips to `phase 61 SPEC done` (NEXT = the phase-61.1 PLAN). No shared-doc edits here (the controller updates STATE serially; the parallel xDS SPEC agent would conflict).

**Next → the phase-61.1 PLAN** (the TDD decomposition of §10 leg-61.1 over this SPEC; every `file:line` RE-DERIVED against the master tip; the multi-leg split §3.0; PROGRESS scaffolded).
