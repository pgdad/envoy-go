# Phase 61 Brainstorm — a minimal downstream HTTP/3 listener (OPENS the HTTP/3 + QUIC family; the FIRST greenfield transport since the MVP trunk; introduces the FIRST new go.mod module `github.com/quic-go/quic-go`; a TLS-mandatory UDP/QUIC listen path parallel to the TCP one, serving an H3 GET into the EXISTING HCM/router/filter-chain; anticipated a MULTI-leg SPLIT — the ADR-0045 ~15-task gate is exceeded by a greenfield transport)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only; **NO** `.go` changes, **NO** Docker, **NO** tests at this stage. Fresh worktree `.worktrees/phase-61-brainstorm`, branch `phase-61-http3-brainstorm`, per `feedback_git_worktrees`.
>
> **Family-opening pick (AUTONOMOUS — no human pick):** per the STANDING DIRECTIVE (human, 2026-07-12) the loop runs autonomously and self-picks the smallest defensible candidate. This BRAINSTORM OPENS a NEW §9 family — **HTTP/3 + QUIC** (ROADMAP ~line 165: "quic-go transport, downstream H3 listener, upstream H3 cluster, `h3spec` gate") — chosen because it is a chartered never-opened family and its smallest defensible slice (a minimal downstream H3 listener) genuinely opens it. The ROADMAP row number is **reserved 61** (given). Note: this phase-61 family-opener is being brainstormed IN PARALLEL with a separate xDS family-opener (row/ADR reserved elsewhere); the controller registers the ROADMAP row / STATE / router serially after this returns — this BRAINSTORM edits ONLY this per-phase file (no shared-doc edits).
>
> **ADR anchor:** the eventual SPEC anticipates **ADR-0279** (ADR-0277 reserved for phase-59 tracing-custom-tags; ADR-0278 reserved for the parallel xDS family-opener). A BRAINSTORM allocates no ADR.
>
> **Baselines re-verified against master tip `958d0154` this session:** stat surface **1201** · differential fixtures **103** (tail dir `0101-stats-sink-graphite`) · fuzzers **54** · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0276** (next-free ADR-0277; ADR-0279 reserved for THIS family after 0277/0278) · new Go packages **0** · **new go.mod modules 0** (this family opener will be the FIRST to add one — `quic-go`). Counts are UNCHANGED at a BRAINSTORM (docs-only). All `file:line` citations below were RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — see §11.

---

## 1. Mission and scope confirmation (61 — the HTTP/3 + QUIC family opener; a minimal TLS-mandatory downstream H3 listener)

### 1.1 What phase 61 delivers as a self-contained whole (a downstream QUIC listener serving an H3 GET into the existing HCM)

Today envoy-go has **ZERO** QUIC / HTTP/3 / UDP-listen substrate. The two greenfield-confirming rejects that phase 61 will eventually LIFT are:

```go
// internal/listener/manager.go:685-689 (re-derived against master 958d0154)
switch tp := fm.GetTransportProtocol(); tp {
case "", "tls", "raw_buffer":
    spec.TransportProtocol = tp
default:
    return nil, fmt.Errorf("transport_protocol %q must be \"tls\" or \"raw_buffer\" or empty", tp)
}
```

```go
// internal/filter/hcm/config.go:231-241 (re-derived)
codecType := msg.GetCodecType()
switch codecType {
case hcmv3.HttpConnectionManager_HTTP1, hcmv3.HttpConnectionManager_AUTO:
    // ok — H1 or ALPN-driven
case hcmv3.HttpConnectionManager_HTTP2:
    if !lc.HasTLS && !lc.AllowH2C { ... }
default:
    return nil, fmt.Errorf("hcm: codec_type %s is not supported in phase 05.1", codecType)  // HTTP3 lands here today
}
```

The **smallest defensible slice that genuinely opens the family** is a **minimal downstream HTTP/3 listener**: a UDP-bound QUIC listener (mandatory TLS — HTTP/3 has no plaintext form) that accepts a QUIC connection, negotiates ALPN `h3`, decodes an HTTP/3 request, and dispatches it into the **existing** HCM → router → filter-chain → upstream path, returning the response over H3. The proven capability is a single **`GET → 200 (or a routed backend response)`** over HTTP/3 — the transport-opening happy path — differentially provable cross-side against `envoyproxy/envoy:contrib-v1.37.2`.

This is the family-opener because it stands up the whole novel substrate the rest of the family layers on: (a) the FIRST new go.mod module (`github.com/quic-go/quic-go`, §2.4/§3.3 — a MAJOR, tracked decision), (b) the FIRST UDP/packet listen path parallel to the TCP `net.Listen` path (`manager.go:862`, §2.3), and (c) the FIRST H3 codec adapting a non-`net.Conn`-stream request source into envoy-go's request model (§2.5 — the deepest integration question). Everything downstream of the H3 codec (routing, filters, upstream) is REUSED.

### 1.2 What phase 61 does NOT deliver (forward to §8)

NO **upstream** H3 (H3 to backends — a separate cluster-side transport + pool; §2.2/§8). NO **alt-svc** advertisement (the `Alt-Svc: h3=...` header on the H1/H2 listener steering clients to H3; §8). NO **0-RTT / early-data** (§8). NO **h3spec conformance gate** (§8 — a substantial conformance harness). NO QUIC connection migration, GREASE, datagrams, or WebTransport (§8). NO tuning of `QuicProtocolOptions` sub-fields beyond what the minimal boot requires (max concurrent streams / idle timeout / connection-id length are anticipated **accepted-and-ignored or strict-rejected**, ADR-0080 — SPEC pins, §2.6). The minimal slice is a happy-path GET; robustness surfaces (stream limits, flow control, RESET_STREAM semantics, GOAWAY) are quic-go-internal at this depth and are NOT independently asserted here.

### 1.3 Phase-done OPENS the HTTP/3 + QUIC family (family STAYS OPEN)

Row 61 is the family-OPENING row. After phase 61 phase-done the family STAYS OPEN — the §8 candidates (upstream H3, alt-svc, 0-RTT, h3spec, QUIC option tuning) remain, so the sentinel check-(2)/(3) still prints ⇒ the loop continues. Per `reference_sentinel_deferred_sentence_live_vs_historical`, the controller writes the LIVE deferred-candidate sentence for this family at row-registration time (EXACTLY ONE live "candidates:" match); this BRAINSTORM only FRAMES the candidate list (§2.2/§8) for that sentence.

### 1.4 ADR-0045 split readiness — anticipated a MULTI-LEG SPLIT (a greenfield transport exceeds the ~15-task gate) *(framed; SPEC/PLAN decide the exact leg boundaries)*

A greenfield transport with a new external dependency, a new listen path, and a new codec is almost certainly **>15 tasks** — the ADR-0045 gate is exceeded, so a SPLIT is anticipated (NOT a single flat row). The anticipated **by-concern** split (SPEC/PLAN pin the exact boundaries — this is a FRAMING, `reference_roadmap_split_phase_row_done` governs the parent-row flip once ALL legs land):

- **61.1 — QUIC/UDP listen substrate + the `quic-go` module.** Introduce `github.com/quic-go/quic-go` (the new go.mod module, §2.4); a UDP/packet listen path parallel to `manager.go:862`'s `net.Listen("tcp", …)`; parse the bootstrap wire shape (`udp_listener_config` + `quic_options` + the `envoy.transport_sockets.quic` downstream transport socket wrapping a `DownstreamTlsContext`; §2.6); lift the `transport_protocol "quic"` reject (`manager.go:689`); accept a QUIC connection + complete the TLS/`h3`-ALPN handshake. Possibly NO request serving yet (accept + handshake + a listener-scope stat).
- **61.2 — the H3 codec + HCM integration.** Adapt a decoded H3 request into envoy-go's existing request model and dispatch it through the HCM → router → filter-chain → upstream path; write the response back over H3. Lift the `codec_type HTTP3` reject (`config.go:240`). Serve a GET → routed backend.
- **61.3 — the differential fixture + H3-client harness surgery.** The Docker harness is TCP-only today (`harness.go:108` exposes only `/tcp`); add UDP port exposure/mapping + an HTTP/3-capable client (`quic-go/http3.Transport`); the one cross-side H3-GET fixture (§2.7). This leg may be FOLDED into 61.2 or stood up first as an infra prerequisite (cf. the phase-30 reference-image pin-refresh precedent) — SPEC decides.

The escape valve is documented ARMED (a greenfield transport is the canonical multi-leg case). The SPEC re-scopes if the minimal slice proves smaller than anticipated (e.g. if quic-go's `http3.Server` drops in with a thin HCM adapter, §2.5, collapsing 61.1+61.2).

### 1.5 Package placement + module introduction — NEW packages AND the FIRST new go.mod module

- New QUIC listen substrate: anticipated a NEW package `internal/listener/quic` (or `internal/quic`) — the UDP-bound QUIC accept loop, parallel to the TCP `acceptLoop`/`serveConnection` in `manager.go`. SPEC pins the exact package boundary.
- New H3 codec: anticipated a NEW package `internal/http/h3` (a sibling of `internal/http` and `internal/filter/hcm/h2`), or folded into the quic package — SPEC pins.
- **NEW go.mod module:** `github.com/quic-go/quic-go` (+ its transitive deps: `quic-go/qpack`, `golang.org/x/crypto|net|sys`, etc.). This is the FIRST new module the project adds and a LOAD-BEARING, separately-tracked decision (§2.4/§3.3). `go mod tidy -diff` will be NON-empty for the first time in the project's history.
- Existing REUSED files: `internal/listener/manager.go` (the listen/serve orchestration — the UDP path threads in here), `internal/filter/hcm/config.go` (the `codec_type HTTP3` lift), the whole HCM/router/filter-chain path (unchanged behind the codec).

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for HTTP/3 (unlike the phase-11 `local_ratelimit` notes, `reference_phase_11_local_ratelimit_prebrainstorm`). This family is chartered only as the ROADMAP §9 stub. Grep of `internal/`/`cmd/` for `quic`/`http3`/UDP-listen confirms ZERO substrate (§11) — the only hits are the two rejects (§1.1), the accesslog HTTP/3 protocol-enum placeholder (`internal/accesslog/mapping.go:18`), and the Lua `:protocol()` "HTTP/3" string literal (`internal/filter/http/lua/bridge.go:156` — a pass-through string, no H3 dispatch).

### 1.7 Phase 61's relationship to the existing seams (a NEW listen path + a NEW codec in FRONT of the reused HCM)

Unlike prior rows (which added a filter/sink/arm on an existing seam), the family-opener adds an entirely NEW front-end transport. The reuse boundary is the HCM request-processing path: once an H3 request is adapted into envoy-go's request model, routing/filters/upstream are unchanged. The novel code is (a) the UDP/QUIC listen+accept, (b) the H3 request/response codec, and (c) the adapter from the H3 request source into the HCM. **The depth of (c) is the family's central uncertainty** (§2.5, D-H3-HCM-SEAM) and is explicitly deferred to the SPEC rather than guessed here.

---

## 2. Design decisions

### 2.1 Family + subject confirmation: OPEN the HTTP/3 + QUIC family with a minimal downstream H3 listener *(SELF-PICKED per the standing directive → phase 61 row reserved)*

The FIRST decision, made AUTONOMOUSLY per the 2026-07-12 standing directive, INVESTIGATING candidate slices against source this session (§11). Row 61 is reserved (given) for the family-opener.

**Why a minimal downstream H3 listener is smallest-defensible:** it is the smallest slice that genuinely stands up the family's novel substrate (the new module + the UDP listen path + the H3 codec) while REUSING the entire existing HCM/router/filter/upstream path behind the codec. A downstream listener is self-contained (no cluster-side transport changes), and its capability (serve an H3 GET) is deterministic and differentially provable cross-side.

**Rejected alternatives (recorded per the standing directive; each SIZED against source this session):**
- **Upstream H3 cluster (H3 to backends)** — needs a QUIC transport socket on the cluster side + an H3 upstream connection pool (echoing the H2-pool leg's shape, `reference_h2_pool_local_cap_driven`/`reference_h2_pool_connect_coalescing`) + upstream ALPN negotiation. It does NOT open the family more cheaply than downstream, and a pool is strictly larger than an accept loop. It ALSO needs a downstream H3 or H2 path to drive it in a differential. Deferred (§8).
- **Both downstream + upstream in one row** — clearly exceeds the split gate twice over; two independent transports in one row is not defensible. Deferred/split.
- **A pure "QUIC listener + handshake, no request serving" slice** — TOO small to be defensible as a family-opener: it stands up the module + listen path but proves NO HTTP/3 behavior (no differential HTTP surface), so it does not "genuinely open" the family in a way the differential harness can bite. It IS, however, the anticipated **61.1 leg** of the split (§1.4) — a stepping-stone WITHIN the opener, not the whole opener.
- **alt-svc advertisement first** (advertise `Alt-Svc: h3=…` on the existing H1/H2 listener) — a header-only change with NO actual H3 serving; it advertises a capability envoy-go would not yet have. Backwards. Deferred (§8).
- **h3spec conformance gate first** — a conformance harness with no implementation to test is vacuous. Deferred (§8).
- **Opening a DIFFERENT family** (gRPC / Runtime / WASM-host / Deprecated-edge) — the standing directive is smallest-defensible-first among chartered work; HTTP/3's downstream-listener opener is a coherent, bounded slice, and this row is RESERVED for it (given). The parallel xDS opener is a SEPARATE reserved row (ADR-0278). Not in scope here.

### 2.2 Family candidate list (for the ROADMAP deferred-candidate sentence) *(framed; controller writes the live sentence)*

The HTTP/3 + QUIC family candidates, for the controller's LIVE deferred sentence (this opener consumes the first; the rest carry forward):
1. **downstream H3 listener** — THIS opener (minimal GET happy path).
2. **upstream H3 cluster** — H3 to backends (QUIC transport socket + H3 upstream pool).
3. **alt-svc advertisement** — `Alt-Svc: h3=…` on the H1/H2 listener steering clients to H3.
4. **0-RTT / early data** — QUIC 0-RTT resumption.
5. **`h3spec` conformance gate** — the HTTP/3 conformance suite as a gate.
6. **`QuicProtocolOptions` tuning** — max concurrent streams / idle timeout / connection-id length / flow-control windows consumed rather than ignored.
7. **QUIC transport-socket full options** — proof source, cert selection, downstream/upstream socket sub-fields.
8. **QUIC robustness** — connection migration, RESET_STREAM/STOP_SENDING parity, GOAWAY/graceful-drain, datagrams, GREASE (far-future; WebTransport beyond).

### 2.3 The listen path: a NEW UDP/QUIC accept path PARALLEL to the TCP `net.Listen` path — NOT a mutation of `acceptLoop` *(self-answered shape; SPEC pins the seam — D-H3-LISTEN-SEAM)*

The entire existing serve path is TCP-stream-shaped: `Start` binds via `net.Listen("tcp", rt.addr)` (`manager.go:862`); `acceptLoop` calls `ln.Accept()` returning a `net.Conn` (`manager.go:917-925`); `serveConnection` runs the listener-filter → chain-match → TLS-handshake → terminal-dispatch pipeline on that `net.Conn` (`manager.go:996+`). QUIC has NO stream `net.Conn` to `Accept()` — it binds a UDP `net.PacketConn` and accepts QUIC *connections* (each multiplexing many streams) off it. So the H3 path is a NEW parallel listen+accept path (quic-go's `(*quic.Transport).Listen` / `quic.ListenEarly` over a `net.ListenUDP` socket), NOT a mutation of `acceptLoop`. The manager's listener-runtime model (`listenerRuntime`, the per-listener metric registration at `manager.go:313`/`855-884`) is REUSED/extended to carry a UDP-bound QUIC listener. The exact seam — a `udpRuntime` sibling of `listenerRuntime`, vs a discriminated `listenerRuntime.kind`, vs a separate manager — is **D-H3-LISTEN-SEAM** (SPEC pins). The chain-match model (`parseChainSpec`, `manager.go:662`) partially applies (a `transport_protocol: "quic"` chain), but filter-chain-match semantics over QUIC (SNI-based selection at the QUIC handshake) is a sub-question the SPEC scopes.

### 2.4 The `quic-go` module: the FIRST new go.mod dependency — a LOAD-BEARING, tracked decision *(self-answered direction; SPEC pins the exact version — D-H3-QUICLIB)*

`github.com/quic-go/quic-go` is the de-facto Go QUIC/HTTP-3 stack and the only production-grade option; it bundles an `http3` subpackage providing both `http3.Server` (downstream) and `http3.Transport`/`RoundTripper` (client — relevant to the harness, §2.7). This is the FIRST new go.mod module the project has ever added — the project has held **new modules = 0** through 60 phases — so it is a SEPARATELY-TRACKED, load-bearing decision, NOT an incidental import. Considerations the SPEC must dispose (**D-H3-QUICLIB**):
- **Exact version pin** — the current quic-go line (v0.4x/v0.5x era) requires a recent Go toolchain; the SPEC pins a specific version and confirms it builds against the project's Go version. (Do NOT guess the exact minor here — SPEC pins; `feedback_brief_citations_not_evidence`.)
- **Transitive deps** — quic-go pulls `quic-go/qpack`, `golang.org/x/crypto|net|sys`, and (test-only) `onsi/ginkgo`, `uber-go/mock`. `go mod tidy -diff` becomes NON-empty for the first time; the count-arithmetic (§12) must reflect the new DIRECT module (+1) and note the transitive additions.
- **Integration depth (the key risk):** does quic-go's `http3.Server` drop in as the H3 codec (feeding an `http.Handler` we adapt to the HCM, §2.5), or must we use the raw `quic` layer + our own HTTP/3 framing/QPACK? `http3.Server`'s `http.Handler`/`http.Request` surface is high-level and may not expose the low-level control envoy-go's HCM needs (per-stream trailers, 1xx, early hints, exact header casing/ordering). This is the deepest uncertainty — deferred to the SPEC as **D-H3-QUICLIB** + **D-H3-HCM-SEAM** (§2.5), NOT guessed.
- **Maintenance/security posture** — quic-go is actively maintained and widely deployed (caddy, etc.); acceptable supply-chain risk, recorded in the ADR-0279 §Context.

### 2.5 The HCM-integration seam: adapt the H3 request source into the EXISTING request model — the family's CENTRAL uncertainty *(explicitly DEFERRED to the SPEC — D-H3-HCM-SEAM)*

Everything downstream of the codec (routing, filters, upstream, stats) is REUSED — the value of the opener is that it does NOT re-implement the HTTP pipeline. The open question is the ADAPTER: envoy-go's HCM is driven today by H1 (`internal/http`) and H2 (`internal/filter/hcm/h2`) codecs over a `net.Conn` stream, producing envoy-go's internal request representation dispatched through the filter chain (`internal/filter/hcm/`). HTTP/3 via quic-go's `http3.Server` produces `(*http.Request, http.ResponseWriter)` — a DIFFERENT shape. The SPEC must dispose **D-H3-HCM-SEAM**: how much of the HCM request path is codec-agnostic vs H1/H2-`net.Conn`-coupled, and whether the H3 adapter (a) reuses the HCM's existing per-request entry point by synthesizing whatever internal request object the H1/H2 codecs produce, or (b) requires a new codec-agnostic dispatch seam (framework surgery). This is UNCERTAIN and load-bearing — it directly sizes the split (§1.4). Anticipated posture: a thin adapter feeding the existing HCM request path, BUT this is a HYPOTHESIS the SPEC must confirm by reading the HCM dispatch boundary (`internal/filter/hcm/connection.go`, the H1/H2 codec entry points) — flagged as the primary SPEC investigation.

### 2.6 The bootstrap wire shape: `udp_listener_config` + `quic_options` + the `envoy.transport_sockets.quic` socket + `http3_protocol_options` — all protos RESOLVE in the EXISTING go-control-plane module *(self-answered availability; SPEC pins REQUIRED-vs-ignored + strict-reject — D-H3-BOOTSTRAP-WIRE)*

The reference configures a downstream H3 listener with (confirmed present in the resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module this session, §11 — **NO new proto module needed**):
- **`Listener.udp_listener_config`** (field 18, `*UdpListenerConfig`; `config/listener/v3/listener.pb.go:342`, getter `:606`) — marks the listener UDP-based.
- **`UdpListenerConfig.quic_options`** (`*QuicProtocolOptions`; `config/listener/v3/quic_config.pb.go:31`) — enables QUIC/H3 on the UDP listener; sub-fields (max concurrent streams, idle timeout, connection-id length, etc.) anticipated accepted-and-ignored or strict-rejected (ADR-0080) at the minimal slice.
- **`envoy.transport_sockets.quic` — `QuicDownstreamTransport`** (`extensions/transport_sockets/quic/v3/quic_transport.pb.go:28`) wrapping a `DownstreamTlsContext` — the MANDATORY TLS (§2.8). ALPN must include `h3`.
- **`HttpConnectionManager.http3_protocol_options`** (field 44, `*core.v3.Http3ProtocolOptions`; `http_connection_manager.pb.go:396`, getter `:942`) on the HCM.
- **`filter_chain_match.transport_protocol: "quic"`** — the reject at `manager.go:689` is lifted for exactly this value.

**D-H3-BOOTSTRAP-WIRE:** the SPEC pins (via live reference probes) WHICH of these fields the reference REQUIRES vs accepts-and-ignores, the exact minimal viable config, and the strict-reject posture (ADR-0080, distinct substrings) for unsupported `quic_options`/transport-socket sub-fields — mirroring the project's incremental-arm posture (e.g. the OTel-transport rejects). A reference probe requires an H3 client to observe behavior (§2.7).

### 2.7 The differential harness gap: TCP-only ports + NO H3 client — a HARNESS-SURGERY question *(flagged; SPEC pins — D-H3-DIFF-HARNESS)*

The differential proof of a downstream H3 listener is an H3 GET driven cross-side (subject envoy-go vs `envoyproxy/envoy:contrib-v1.37.2`). TWO harness gaps, both re-derived this session (§11):
- **UDP port mapping:** the Docker harness exposes/maps ONLY `/tcp` ports (`test/differential/harness.go:108` `exposed := []string{"9901/tcp"}`, `:110`/`:145`/`:173`/`:182`/`:423` all `%d/tcp`). An H3 listener binds UDP; the harness needs `/udp` exposure + `MappedPort(..., "<p>/udp")`. Plus quic-go/HTTP-3 clients probe over UDP to a mapped port — reachability under Docker Desktop needs care (`reference_docker_probe_bridge_network`, `reference_host_gateway_ip_docker_desktop`).
- **No H3 client:** the harness drives H1 via `http.Client` (`harness_test.go:266`) and H2 via `golang.org/x/net/http2` (`runner_test.go:153`); there is **no HTTP/3 client anywhere** (grep of `test/` for `quic-go`/`http3` = ZERO, §11). The fixture needs a `quic-go/http3.Transport` client — which is available FREE once the `quic-go` module lands (§2.4). Note the runner's HTTP client is used for BOTH sides, so ONE http3 client serves both.

**D-H3-DIFF-HARNESS:** the SPEC scopes this surgery and decides whether the fixture lands in the opener (the 61.3 leg, §1.4) or whether harness surgery is large enough to be its own leg/prerequisite (cf. the phase-30 image-pin-refresh precedent). This is a genuine UNCERTAINTY — the harness has never driven a non-TCP transport — flagged, not guessed. `reference_differential_backendcount_min_one` (a downstream-only H3 fixture still returns a throwaway BackendKind ≥1) and `reference_differential_fixture_dispatch_constraint` (one dir = one runner branch) apply.

### 2.8 Mandatory TLS: HTTP/3 has NO plaintext form — reuse the existing cert-loading substrate *(self-answered direction; SPEC pins ALPN/cert requirements — D-H3-TLS)*

QUIC bakes TLS 1.3 into the transport; there is no `raw_buffer` H3. The `QuicDownstreamTransport` (§2.6) wraps a `DownstreamTlsContext`, so the listener MUST carry a cert + key and advertise ALPN `h3`. This REUSES the existing TLS cert/key-loading substrate the TCP-TLS path already uses (`internal/listener/manager.go` TLS-chain build + `stdtls.Config`), but quic-go consumes a `*tls.Config` on its OWN accept path (NOT `stdtls.Server` over a `net.Conn`). **D-H3-TLS** pins: the exact ALPN string(s) the reference negotiates (`h3`, and legacy `h3-29`?), whether the reference requires a specific cert posture, and how the cert/`tls.Config` threads into quic-go's listener. Anticipated: reuse the parsed cert, hand quic-go a `*tls.Config` with `NextProtos: []string{"h3"}`.

### 2.9 Stat-surface hypothesis: NEW downstream H3 listener stats (best-estimate several) *(SPEC pins exact set — D-H3-STATS)*

A new downstream listener emits downstream connection/request stats. Anticipated the H3 listener echoes the existing per-listener downstream counters (`downstream_cx_total`/`downstream_cx_active`, registered at `manager.go:313`/`registerListenerMetrics`) and the HCM `downstream_rq_*` family, plus possibly QUIC/H3-specific counters (the reference exposes `http3.*` and `quic.*` stats — e.g. `http3.rx_reset`, `quic.*` connection counters). The exact NEW stat set is **D-H3-STATS** (SPEC pins against the reference; `reference_stats_sink_emits_used_only` — the reference omits never-incremented counters, so assert a NAMED SUBSET). Best estimate: stat surface **1201 → 1201 + N** where N is small-to-moderate (SPEC-pinned); the count-arithmetic (§12) carries N as SPEC-TBD. Any wire-derived stat segment (e.g. a per-listener address in a name) must pass `stats.IsValidName` before registration (`reference_dynamic_stat_name_charset_guard`).

### 2.10 Fuzz posture: anticipated +0 (quic-go owns the framing) — SPEC confirms *(D-H3-FUZZSEED)*

HTTP/3 frame parsing + QPACK are quic-go-internal (not hand-rolled), so no new frame-parser fuzzer is anticipated — UNLIKE the network-filter rows that hand-rolled decoders. The bootstrap parse of `udp_listener_config`/`quic_options`/the transport socket is config-parsing that COULD take a fuzz SEED on an existing config fuzzer (if the H3 config parse joins a bootstrap/listener config fuzzer). Anticipated fuzzers **54 → 54 (+0)**, or +1 only if the SPEC hand-rolls any H3-adjacent parsing. `reference_fuzzer_count_docs_drift` — reconcile the running total against actual `^func Fuzz` before AND after. SPEC confirms **D-H3-FUZZSEED**.

---

## 3. Framework-survey result — a NEW transport front-end (new module + new packages) in FRONT of the reused HCM (61 anticipated)

### 3.1 Framework: a genuine NEW listen path + codec (NOT a small seam)

Unlike prior rows (a field + an arm on an existing seam), the opener adds a new UDP/QUIC listen path (§2.3), a new H3 codec (§2.4/§2.5), and an adapter into the HCM (§2.5). The manager's listener-runtime + per-listener-metric model is EXTENDED; the HCM request path is REUSED behind the adapter. The depth of the adapter (D-H3-HCM-SEAM) determines whether framework surgery is needed — the SPEC's primary investigation.

### 3.2 NEW packages: anticipated 1–2

Anticipated `internal/listener/quic` (the UDP/QUIC accept path) and possibly `internal/http/h3` (the codec/adapter) — SPEC pins the boundary (§1.5). Best estimate **+1 or +2 packages**.

### 3.3 go.mod modules: **+1 (the FIRST ever)** — `github.com/quic-go/quic-go`

The load-bearing decision (§2.4). The project has held modules = 0 for 60 phases; this opener adds the first DIRECT module (`quic-go`) plus transitive deps. `go mod tidy -diff` becomes NON-empty. SPEC pins the exact version (D-H3-QUICLIB) and records the supply-chain posture in ADR-0279 §Context. The Envoy QUIC/H3 protos are ALREADY in the resolved go-control-plane module (§2.6) — **NO new proto module** (contrast a hypothetical contrib-proto need; here the protos are in the standard `envoy` module already a dep).

### 3.4 REUSES

- **the whole HCM/router/filter-chain/upstream path** behind the H3 codec (routing, filters, cluster dial, load balancing — unchanged).
- **the listener manager's runtime + per-listener-metric model** (`manager.go` `listenerRuntime`, `registerListenerMetrics`) — extended for a UDP-bound QUIC listener.
- **the TLS cert/key-loading substrate** (§2.8) — reused to build quic-go's `*tls.Config`.
- **the differential harness** (`test/differential`) — extended (NOT replaced) for UDP ports + an http3 client (§2.7).
- **the incremental-reject / ADR-0080 posture** — the template for strict-rejecting unsupported `quic_options` sub-fields (§2.6).

---

## 4. Bootstrap-level applicability — a NEW listener KIND (udp_listener_config) + an HCM codec_type + a QUIC transport socket

Unlike the stats-sink rows (bootstrap `stats_sinks[]`), this is a LISTENER-level opener: a `Listener` with `udp_listener_config.quic_options`, a `filter_chain` whose transport socket is `envoy.transport_sockets.quic` (wrapping a `DownstreamTlsContext`) and whose HCM has `codec_type: HTTP3` (or `http3_protocol_options`). Parsing lands in `internal/listener/manager.go` (the UDP-listener + transport-socket dispatch) and `internal/filter/hcm/config.go` (lifting the `codec_type HTTP3` reject, `config.go:240`). No `stats_sinks[]` change.

---

## 5. Stat surface hypothesis — +N (SPEC-pinned), NOT +0

### 5.1 Stat names (SPEC confirms — D-H3-STATS)

Anticipated the new H3 listener's downstream connection/request counters + possibly `http3.*`/`quic.*` counters (§2.9). Exact set SPEC-pinned; assert a NAMED SUBSET (`reference_stats_sink_emits_used_only`).

### 5.2 envoy-go-strict departure flags

Unsupported `quic_options`/transport-socket sub-fields reject loudly with distinct substrings (ADR-0080) — the same incremental-arm posture as prior families (§2.6). Recorded in BEHAVIOR_CONTRACT at the IMPL.

### 5.3 Anticipated surface arithmetic

Stat surface **1201 → 1201 + N** (N SPEC-TBD, best estimate small-to-moderate). Carried as SPEC-TBD in §12.

---

## 6. Edit-site enumeration — RE-DERIVED this session (SPEC re-derives + pins the seams)

Each `file:line` RE-DERIVED against master `958d0154` this session (`feedback_brief_citations_not_evidence`); the SPEC re-derives again and the exact roster depends on the split-leg boundaries (§1.4) and D-H3-HCM-SEAM (§2.5).

**Production — listen path (anticipated NEW `internal/listener/quic` + edits to `internal/listener/manager.go`):**
1. **Lift the `transport_protocol "quic"` reject** (`manager.go:685-689`) — accept `"quic"` for a UDP/QUIC chain. [EDIT]
2. **A UDP/QUIC listen+accept path** parallel to `Start`'s `net.Listen("tcp", …)` (`manager.go:862`) and `acceptLoop` (`manager.go:917`) — bind `net.ListenUDP` + quic-go `Listen`; accept QUIC connections; per-listener metric registration reused (`manager.go:313`). [ADD — new file/package]
3. **`udp_listener_config` + `quic_options` + `envoy.transport_sockets.quic` parse** in `buildListenerRuntimeWithCtx` (`manager.go:340+`) — dispatch a UDP listener KIND. [EDIT/ADD]

**Production — H3 codec + HCM adapter (anticipated NEW `internal/http/h3`; DEPTH per D-H3-HCM-SEAM):**
4. **The H3 codec/adapter** — decode an H3 request (via quic-go `http3`), adapt into the HCM request model, write the H3 response. [ADD — new package/file]
5. **Lift the `codec_type HTTP3` reject** (`internal/filter/hcm/config.go:231-241`, the `default` arm at `:240`) — accept HTTP3 on a QUIC listener. [EDIT]

**Production — TLS:**
6. **Build quic-go's `*tls.Config`** from the parsed cert (ALPN `h3`) — reuse the existing cert-loading substrate (§2.8). [ADD]

**Module + build:**
7. **`go.mod`/`go.sum`** — add `github.com/quic-go/quic-go` (+ transitive). [EDIT — the FIRST new module]

**Test:**
8. **Unit tests** for the UDP-listener parse, the codec adapter, the reject-lifts, and the strict-reject arms (distinct substrings). [ADD]

**Harness + fixture:**
9. **`test/differential/harness.go`** — UDP port exposure/mapping (`:108` etc., §2.7). [EDIT]
10. **`test/differential` client** — an `http3.Transport` client for the H3 GET (§2.7). [ADD]
11. **`test/fixtures/NNNN-http3-downstream-get`** (new) — a cross-side H3 GET; assert the response + a named downstream-stat subset. [ADD — `reference_fatalf_makes_assertions_unreachable`: `Errorf` per independent property; a `-count=1` deliberate break confirming WHICH fires per `reference_deliberate_break_wrong_assertion`.]

**BEHAVIOR_CONTRACT / ROADMAP / STATE / DECISIONS (controller + IMPL, NOT this BRAINSTORM):**
12. **BEHAVIOR_CONTRACT.md** — a new HTTP/3 section (downstream H3 listener supported; unsupported sub-options reject loudly). [IMPL]
13. **ROADMAP row 61** `in-progress` + the family deferred-candidate sentence (controller); **STATE.md** active-phase (controller); **DECISIONS.md** ADR-0279 (SPEC §Context / IMPL §Decision). [controller/SPEC/IMPL — NOT this BRAINSTORM]

### 6.1 The parallel-family scheduling caveat (given)

This BRAINSTORM edits ONLY this per-phase file. The ROADMAP row / STATE / router are registered by the controller SERIALLY after this returns (the parallel xDS opener would conflict on those shared docs). No shared-doc edits here.

---

## 7. Anticipated ADRs — 1 at the phase-61 SPEC/IMPL: ADR-0279 (the HTTP/3 + QUIC family opener)

ADR-0279 (opening the HTTP/3 + QUIC family via a minimal downstream H3 listener — the new `quic-go` module decision + the UDP/QUIC listen path + the H3 codec/HCM-adapter seam + the mandatory-TLS posture + the strict-reject of unsupported quic sub-options). §Context drafted at the SPEC (the gap's provenance: the ROADMAP §9 stub + the two lifted rejects), §Decision/§Consequences at the IMPL per ADR-0044. Because this is a greenfield transport with genuine seam decisions (the listen-path seam D-H3-LISTEN-SEAM + the HCM-adapter seam D-H3-HCM-SEAM + the new-module decision), a SEPARATE seam sub-ADR is PLAUSIBLE (unlike the recent single-arm rows that folded seams into the family ADR) — the SPEC re-decides. A multi-leg split (§1.4) may anchor per-leg ADRs (SPEC/PLAN decide). Next-free after: **ADR-0280**. (ADR-0277 reserved phase-59; ADR-0278 reserved the parallel xDS opener; ADR-0279 reserved THIS family — given.)

---

## 8. Deferred items

- **upstream H3 cluster** — H3 to backends (QUIC transport socket + H3 upstream pool, echoing the H2-pool leg). The immediate next H3 slice after downstream. Carries forward.
- **alt-svc advertisement** — `Alt-Svc: h3=…` on the H1/H2 listener. Carries forward.
- **0-RTT / early data** — QUIC 0-RTT resumption. Carries forward.
- **`h3spec` conformance gate** — the HTTP/3 conformance suite as a gate (the ROADMAP stub names it). Carries forward.
- **`QuicProtocolOptions` tuning** — max concurrent streams / idle timeout / connection-id length / flow-control consumed rather than ignored. Carries forward.
- **QUIC transport-socket full options** — proof source, cert selection. Carries forward.
- **QUIC robustness** — connection migration, RESET_STREAM/STOP_SENDING parity, GOAWAY/graceful-drain, datagrams, GREASE, WebTransport. Carries forward.
- **filter-chain-match over QUIC** — SNI-based chain selection at the QUIC handshake (partial in the opener; full semantics deferred if the minimal slice uses a single chain). Carries forward.

After row 61 the family STAYS OPEN (candidates 2–8 of §2.2 remain) ⇒ the sentinel check-(2)/(3) still prints ⇒ the loop continues.

---

## 9. Cross-references against prior phases' deferred-items lists — a NEW family, no pickup

Phase 61 OPENS a never-opened family — it does NOT pick up a prior deferred candidate (unlike within-family rows). The two rejects it lifts (`transport_protocol "quic"`, `codec_type HTTP3`) were placed as forward-looking guards at phase 05.1/07.x, not deferred candidates. The Network-filters family is CLOSED (2026-06-10); the Observability family is the roller's recent home (phases 47–59); this opener PIVOTS to a new family per the standing directive's smallest-defensible-among-chartered-work rule (the parallel xDS opener pivots to another). **Sentinel maintenance:** the controller writes the family's LIVE deferred sentence at row registration (EXACTLY ONE live "candidates:" match, `reference_sentinel_deferred_sentence_live_vs_historical`).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D-H3-SLICE** — confirm the minimal downstream H3 listener (GET happy path) as the opener; the 61.1/61.2/61.3 split (§1.4). §2.1.
- **D-H3-QUICLIB** — `github.com/quic-go/quic-go` as the new module; the EXACT version pin (build against the project's Go); transitive deps + `go mod tidy -diff` non-empty; supply-chain posture. §2.4.
- **D-H3-HCM-SEAM** *(the central uncertainty)* — how much of the HCM request path is codec-agnostic vs H1/H2-`net.Conn`-coupled; whether a thin adapter feeds the existing HCM entry point or a new codec-agnostic dispatch seam is needed (framework surgery). RE-DERIVE the HCM dispatch boundary (`internal/filter/hcm/connection.go`, the H1/H2 codec entry points). §2.5.
- **D-H3-LISTEN-SEAM** — the UDP/QUIC listen-path seam (a `udpRuntime` sibling vs a discriminated `listenerRuntime.kind` vs a separate manager); how filter-chain-match applies over QUIC. §2.3.
- **D-H3-BOOTSTRAP-WIRE** — the exact minimal viable config (`udp_listener_config` + `quic_options` + `envoy.transport_sockets.quic` + `http3_protocol_options` + `transport_protocol: "quic"`); WHICH fields REQUIRED vs accepted-ignored; the strict-reject posture (ADR-0080) for unsupported quic sub-fields. One or more fresh-container probes against `envoyproxy/envoy:contrib-v1.37.2` (`reference_probe_fresh_container_per_arm`, `reference_envoy_contrib_image_tagging`). §2.6.
- **D-H3-TLS** — mandatory TLS: the ALPN string(s) (`h3`, legacy `h3-29`?); cert posture; threading the parsed cert into quic-go's `*tls.Config`. §2.8.
- **D-H3-DIFF-HARNESS** *(a genuine uncertainty)* — the harness is TCP-only (`harness.go:108`) with NO H3 client; the UDP-port-mapping + `http3.Transport`-client surgery; whether the fixture lands in the opener or a prerequisite leg; Docker-Desktop UDP reachability (`reference_docker_probe_bridge_network`, `reference_host_gateway_ip_docker_desktop`). §2.7.
- **D-H3-STATS** — the exact NEW downstream/`http3.*`/`quic.*` stat set (assert a NAMED SUBSET, `reference_stats_sink_emits_used_only`; `stats.IsValidName` guard on wire-derived segments). §2.9.
- **D-H3-FUZZSEED** — anticipated +0 (quic-go owns framing); a config-parse SEED only if the H3 config joins a bootstrap/listener fuzzer; fuzzer count reconciled before AND after (`reference_fuzzer_count_docs_drift`). §2.10.
- **D-H3-SPLIT** — the ADR-0045 disposition (anticipated MULTI-leg split; the 61.1/61.2/61.3 by-concern boundaries; the parent-row flip once ALL legs land per `reference_roadmap_split_phase_row_done`). §1.4.

---

## 11. Prior-phase lessons applied

- **`feedback_brief_citations_not_evidence`** — EVERY `file:line` here (`manager.go:685-689/862/917/340/313`, `config.go:231-241`, the go-control-plane proto lines `listener.pb.go:342`/`quic_config.pb.go:31`/`quic_transport.pb.go:28`/`http_connection_manager.pb.go:396`, the harness `harness.go:108`) was RE-DERIVED from source live this session against master `958d0154`; the SPEC re-derives again. The prose control-flow claims (the TCP-only serve path, the no-H3-client harness) were verified by reading the source, not asserted from memory.
- **greenfield confirmation** — grep of `internal/`/`cmd/` for `quic`/`http3`/UDP-listen returned ZERO substrate (only the two rejects + the accesslog HTTP/3 enum placeholder `mapping.go:18` + the Lua `:protocol()` string `bridge.go:156`); `go.mod` has NO `quic-go`; `test/` has NO `http3` client. §1.1/§1.6.
- **`feedback_git_worktrees` / `feedback_subagents_no_push`** — this BRAINSTORM runs in the pinned worktree `.worktrees/phase-61-brainstorm`; committed LOCALLY only, no push.
- **`reference_h2_pool_local_cap_driven` / `reference_h2_pool_connect_coalescing` / `reference_h2_pool_downstream_codec_gate`** — the H3 legs (especially upstream H3, deferred §8) will ECHO the H2-pool's shape (local-cap-driven, connect-coalescing, downstream-codec-gated); recorded so the upstream-H3 follow-on reuses those hard-won lessons.
- **`reference_probe_fresh_container_per_arm` + `reference_envoy_contrib_image_tagging`** — each SPEC probe arm (D-H3-BOOTSTRAP-WIRE / D-H3-TLS / D-H3-STATS) runs on a FRESH container against `envoyproxy/envoy:contrib-v1.37.2`. §10.
- **`reference_docker_probe_bridge_network` + `reference_host_gateway_ip_docker_desktop`** — the H3 probe/fixture drives UDP into a mapped port under Docker Desktop; verify the H3 request ACTUALLY completed (not a vacuous empty capture). §2.7.
- **`reference_differential_backendcount_min_one` + `reference_differential_fixture_dispatch_constraint`** — a downstream-only H3 fixture returns a throwaway BackendKind ≥1; one fixture dir = one runner branch. §2.7.
- **`reference_fatalf_makes_assertions_unreachable` + `reference_deliberate_break_wrong_assertion` + `reference_differential_break_protocol_count1`** — the H3 fixture asserts each property with `Errorf`, proves each assertion live with a `-count=1` deliberate break, and confirms WHICH break fires. §6.
- **`reference_stats_sink_emits_used_only` + `reference_dynamic_stat_name_charset_guard`** — assert a NAMED stat SUBSET (the reference omits unincremented counters); guard any wire-derived stat segment with `stats.IsValidName` before registration. §2.9.
- **`reference_fuzzer_count_docs_drift`** — reconcile the fuzzer running total (54) against actual `^func Fuzz` before AND after; anticipated +0. §2.10.
- **`reference_roadmap_split_phase_row_done`** — the parent row 61 flips `done` only once ALL split legs land (ADR-0106 governs family-expansion rollup, NOT split-phase completion). §1.4.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — the controller writes the family's LIVE deferred-candidate sentence (EXACTLY ONE live "candidates:" match). §1.3/§9.

---

## 12. Section closeout

**Settled (framing):** family (OPEN the HTTP/3 + QUIC family, SELF-PICKED per the standing directive, §2.1); slice (a minimal TLS-mandatory downstream H3 listener serving a GET into the EXISTING HCM/router/filter path, over five declined alternatives, §2.1); the listen path (a NEW UDP/QUIC accept path parallel to the TCP one, NOT a mutation, §2.3); the module (the FIRST new go.mod module `github.com/quic-go/quic-go`, load-bearing, §2.4/§3.3); the protos (all RESOLVE in the existing go-control-plane module — NO new proto module, §2.6); mandatory TLS reusing the cert substrate (§2.8); envelope (a MULTI-leg SPLIT anticipated — a greenfield transport exceeds the ADR-0045 gate; by-concern 61.1 listen / 61.2 codec+HCM / 61.3 fixture+harness, §1.4). The novel production code is the UDP/QUIC listen+accept, the H3 codec, and the HCM adapter; everything downstream is reused.

**Deferred to the SPEC (genuine uncertainties, NOT guessed):** D-H3-HCM-SEAM (the adapter depth — the family's central question, §2.5); D-H3-QUICLIB (the exact quic-go version + integration depth — `http3.Server` drop-in vs raw QUIC, §2.4); D-H3-DIFF-HARNESS (the TCP-only harness + no-H3-client surgery, §2.7). These three are the load-bearing unknowns the SPEC must dispose before the split legs can be planned.

**Anticipated moves at the phase-61 SPEC/IMPL (docs-only now):** the quic-go module + the UDP/QUIC listen path + the H3 codec/adapter + the two reject-lifts + the mandatory-TLS `tls.Config` + the strict-reject arms + unit tests + the harness UDP+http3-client surgery + the one cross-side H3-GET fixture + a BEHAVIOR_CONTRACT HTTP/3 section + ADR-0279 (+ possibly per-leg ADRs) + the ROADMAP row-61 registration (controller). Counts (best estimates; SPEC pins): stat surface **1201 → 1201 + N** (N SPEC-TBD, small-to-moderate) · differential fixtures **103 → 104** (the one H3-GET fixture; possibly landing in the 61.3 leg) · fuzzers **54 → 54** (+0 anticipated) · BackendKind **38 → 39** (a throwaway/echo backend for the downstream-only fixture) OR **38 (+0)** if an existing HTTP echo kind is reused (SPEC decides) · new Go packages **+1 or +2** (`internal/listener/quic` [+ `internal/http/h3`]) · **new go.mod modules +1** (`github.com/quic-go/quic-go`, the FIRST — plus transitive; `go mod tidy -diff` NON-empty for the first time) · DECISIONS tail advances to **ADR-0279** (next-free ADR-0280).

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `958d0154`):** stat surface **1201** · differential fixtures **103** · fuzzers **54** · BackendKind **38** · DECISIONS tail **ADR-0276** · new Go packages **0** · new go.mod modules **0**. Row 61 is registered `in-progress` by the CONTROLLER after this BRAINSTORM returns (not here — the parallel-family serialization caveat, §6.1).

**Next → the phase-61 SPEC** (dispose D-H3-HCM-SEAM by reading the HCM dispatch boundary; D-H3-QUICLIB by pinning the quic-go version + probing `http3.Server` fit; D-H3-BOOTSTRAP-WIRE / D-H3-TLS / D-H3-STATS via fresh-container probes against `envoyproxy/envoy:contrib-v1.37.2`; D-H3-DIFF-HARNESS by scoping the UDP+http3-client surgery; finalize the 61.1/61.2/61.3 split; draft ADR-0279 §Context).
