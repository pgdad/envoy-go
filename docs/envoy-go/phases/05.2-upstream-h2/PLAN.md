# Phase 05.2 — Upstream HTTP/2 (client-side codec, `Cluster.DialH2`, `routerActionH2`, fixture 0004) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants), §5 (state machine), §6 (splitting), §7 (differential contract), §7.5 (phase-done gates with the conformance gate (c) clarification); `docs/envoy-go/phases/05.2-upstream-h2/SPEC.md` (commit `dacf4b7` — authoritative scope; every PLAN decision below traces to a SPEC section); `docs/envoy-go/phases/05-http-2/SPEC.md` (commit `612cdea` — master phase-05 design document; sub-phase SPECs carve coherent slices of its §4 deliverables per ADR-0045); `docs/envoy-go/phases/05.1-downstream-h2/{SPEC.md,PLAN.md,PROGRESS.md,REVIEW.md}` (closed read-only history; the 05.1 PLAN is the precedent for task shape and TDD discipline; the 05.1 REVIEW's I-1/I-2/I-3 + Minor findings are the carry-forward into ADR-0055); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0054 — especially **ADR-0003** branch convention, **ADR-0004** autonomous brainstorming adaptation, **ADR-0005** autonomous plan-review adaptation, **ADR-0008** Envoy v1.37.2 pin, **ADR-0010** V4_ONLY DNS rule, **ADR-0013** go-control-plane proto-types-only pin, **ADR-0014** `Server: envoy` HCM-locally-generated header value, **ADR-0016** bootstrap loader unknown-field policy + blank-import amendment policy, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0024** per-cluster RR scope, **ADR-0027** STATIC-vs-STRICT_DNS divergence, **ADR-0028** reference-side `--concurrency 1` pin, **ADR-0031** stdlib `crypto/tls` stack, **ADR-0032** `Cluster.Dial(ctx)` upstream dialer, **ADR-0035** fixture-0002 differential scope (carry-forward closed for H2 leg in 05.2 by ADR-0057), **ADR-0039** per-request fresh upstream H1 dial (ADR-0056 mirrors for H2), **ADR-0041** HCM `stat_prefix` + silently-ignored field set, **ADR-0042** HTTP-filter chain shape `[router]`, **ADR-0044** BEHAVIOR_CONTRACT HTTP/1.1 subsection, **ADR-0045** plan-time split of phase 05 into 05.1 + 05.2, **ADR-0046** HTTP/2 codec source — `golang.org/x/net/http2.Framer` + `hpack`, **ADR-0047** server settings defaults, **ADR-0048** server connection manager from scratch (the ADR explicitly reserves `client.go` for 05.2 — this PLAN delivers it), **ADR-0050** ALPN dispatch wiring, **ADR-0051** h2spec threshold + image pin, **ADR-0052** BEHAVIOR_CONTRACT `## HTTP/2` SCAFFOLD subsection (in-place edit authorised for 05.2 — this PLAN performs the edit), **ADR-0053** phase-04 REVIEW Minor carry-forward triage (the "phase-05.2-will-repeat-the-pattern" note for M-5 lands in this PLAN as the `defer cc.Close()` shape inside `routerActionH2.do`), **ADR-0054** ADR-0046 prose correction); `docs/envoy-go/BEHAVIOR_CONTRACT.md ## HTTP/2` (lines 267-315 — the SCAFFOLD subsection from 05.1; this PLAN edits it in place per ADR-0052 to flip the deferred items to active rules, add the upstream + fixture-0004 rules, and extend the threshold-language with non-default `MaxFrameSize` / tight-window prose per ADR-0055; it does NOT supersede ADR-0052 — the SCAFFOLD-pattern in-place-edit authorisation IS the supersession-free mechanism); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 05.2; the ADR-0051 pin and threshold list stay at the 05.1 baseline; D-3.7 reserves pin bumps for dedicated phases); `docs/envoy-go/phases/05.1-downstream-h2/PLAN.md` (the 16-task precedent — task shape, ADR-with-first-use-commit discipline, PROGRESS conventions, post-plan-handoff section).

**Goal:** Land envoy-go's first upstream HTTP/2 dataplane — a from-scratch `internal/filter/hcm/h2/client.go` (the ONE new file in the 05.1 sub-package; per ADR-0048's reservation) carrying `H2Request`/`H2Response` value types, a per-upstream-conn `ClientConn`, an odd-numbered client-initiated stream-id allocator (RFC 9113 §5.1.1), client-preface emit, client-side initial SETTINGS exchange (synchronous SETTINGS_ACK wait inside `NewClientConn` per SPEC §10 #5), `(*ClientConn).RoundTrip(ctx, req H2Request) (H2Response, error)` exercising one stream per conn (HEADERS+END_STREAM out for bodyless GETs; HEADERS+DATA+END_STREAM out otherwise; HEADERS+DATA+END_STREAM in; trailing HEADERS observed-and-discarded per ADR-0058) with ctx-cancel emitting RST_STREAM(CANCEL), and `(*ClientConn).Close()` emitting graceful GOAWAY(NO_ERROR) followed by TCP-level FIN; a new `internal/cluster/dial_h2.go` carrying `Cluster.DialH2(ctx) (*h2.ClientConn, error)` that delegates to `Cluster.Dial(ctx)` (phase-03 helper, unchanged), type-asserts to `*stdtls.Conn`, defensively calls `tlsConn.HandshakeContext(ctx)` (SPEC §11.3 mitigation; idempotent for already-handshaken conns), reads `ConnectionState().NegotiatedProtocol` (errors `cluster: dial h2: alpn negotiated %q, want "h2"` on mismatch), wraps via `h2.NewClientConn(ctx, raw)`, and returns; an extension to `internal/cluster/manager.go` reading `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]` (the standardised v3-proto cluster-side carrier) per SPEC §5.5's behaviour matrix — `useH2=true` only on the `explicit_http_config.http2_protocol_options{}` discriminator with build-time validation that the cluster's `transport_socket` is present, is type `envoy.transport_sockets.tls`, and the parsed TLS config's `alpn_protocols` includes `"h2"`; the H1 discriminator + `auto_config{}` + nil/empty branch silent-ignore (the 05.2 narrowing of master SPEC §5.8 per SPEC §5.5); a `Cluster.UseH2() bool` accessor in `internal/cluster/cluster.go` (existing `Cluster.Dial(ctx)` unchanged — `DialH2` is a SEPARATE helper, not a method swap); a blank import `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` in `internal/cluster/cluster.go` AND in `internal/bootstrap/bootstrap.go` (per ADR-0016's blank-import amendment policy — registry population, not a new ADR); a new `routerActionH2` action variant in `internal/filter/hcm/actions.go` alongside the phase-04 `routerAction` (unchanged) and the codec-neutral `directResponseAction` (unchanged from 05.1) — `routerActionH2.do(ctx, req H2Request, w h2.StreamWriter) error` calls `r.cluster.DialH2(ctx)` (502 local-reply on dial failure with byte-equal-to-H1 prose body `bad gateway\n` per SPEC §11.9), `defer cc.Close()` (per-request fresh conn per ADR-0056 — the analogous shape to phase-04's `defer upstream.Close()` from M-5; the cosmetic prose-vs-mechanism gap from ADR-0053 carries forward), `cc.RoundTrip(ctx, req)` (502 on protocol error; RST_STREAM(CANCEL) on ctx cancel; verbatim 5xx forward from upstream HTTP-status-level errors), writes `w.Headers(resp.Headers, false)` (pseudo-headers `:status` first per RFC 9113 §8.3) + `w.Data(resp.Body, true)` (END_STREAM via DATA-with-END per SPEC §5.3 step 5 + ADR-0058) — the `Date` header IS included in the 502 local-reply per SPEC §10 #4; an extension to `internal/filter/hcm/filter.go`'s `NewFilter` build path picking `routerActionH2` vs `routerAction` per `Cluster.UseH2()` (the ALPN dispatch in `Filter.Handle` UNCHANGED from 05.1 per ADR-0050) plus an extension to `internal/filter/hcm/h2dispatch.go` adding a `h2RouterActionAdapter` that wraps `routerActionH2` for the codec-neutral `h2.Action` interface (replacing 05.1's `h2RouterActionRejection` sentinel for clusters with `UseH2()==true`; the rejection sentinel stays for non-H2 clusters reachable on the H2 path); a flow-control discipline tightening of the 05.1 codec primitives per ADR-0055 — `ServerConn.writeData` (and the new client-side path) caps each outgoing DATA chunk at `min(conn-window-available, stream-window-available, peer.MaxFrameSize)` per I-1/I-2; `ServerConn.onData` (and the symmetric client-side path) decrements `recvW` and emits `WriteWindowUpdate(0,n)`/`WriteWindowUpdate(streamID,n)` once a half-window high-water threshold is crossed per I-3; `flow.go`'s `window.waitFor`+`window.reserve` pair collapses into a single mutex-guarded `window.reserveBlocking(ctx, max int32) (taken int32, err error)` per M-3; `serverStream.recvWindowUpdate` and `ServerConn.onWindowUpdate` add `2³¹ - 1` overflow bounds-checks per M-9; `serverStream.recvData` checks state BEFORE appending to `s.reqBody` per M-11; `framer.go`'s duplicated `http2.ConnectionError`/`StreamError`/`ErrFrameTooLarge` translation block is extracted into `translateFramerErr(err) error` per M-5 (cosmetic prerequisite so the new `ClientConn`'s framer wrapper consumes the same translation); a new test helper `test/helpers/h2.go` carrying `H2RoundTrip(ctx, addr, tlsConf, method, path, headers, body) (status, respHeaders, respBody, err)` for the fixture-0004 driver (driver-side use of `golang.org/x/net/http2.Transport` is permitted per D-3.2 — runtime not); the project's first full-stack HTTPS h2 differential fixture `test/fixtures/0004-h2-routing/` (the new gate-(a) deliverable; non-vacuous for the first time on the H2 surface) carrying `envoy-go.yaml` (subject — STATIC cluster `c_h2_backend` × 3 TLS endpoints + `HttpProtocolOptions.explicit_http_config.http2_protocol_options{}` + `transport_socket: tls` with `alpn_protocols: ["h2"]`) + `envoy.yaml` (reference — STRICT_DNS cluster pointing at `host.docker.internal` × 3 TLS ports per ADR-0010, `--concurrency 1` per ADR-0028) + prose `expectations.yaml` (the M-6 carry-forward from phase-04 — heredoc/prose-form pattern preserved per the 05.1 disposition) + `README.md` + `pki/gen/main.go` (deterministic CA + leaf generator emitting SANs `localhost`/`127.0.0.1`/`host.docker.internal` per fixture-0002 precedent; PEMs committed) + `pki/*.pem` (committed deterministic artefacts) + `driver/driver.go` + `driver/driver_test.go` + `backends/main.go` (small Go program: TLS listener with `NextProtos: ["h2"]` + `http2.ConfigureServer` driver-side use; echo handler returning `backend-<idx>:<path-suffix>` for `/api/v1/<n>` paths); driver issues 27 sequential H2 requests per side (9 × `GET /health` direct_response 200 / 9 × `GET /api/v1/<n>` router-action / 9 × `GET /missing/<n>` direct_response 404) with `Transport.DisableKeepAlives = true` AND a fresh `Transport` per request to keep RR distribution deterministic; `AssertDistribution([3,3,3])` per side over the 9 router-action requests (local-correctness; cross-side sequence is NOT asserted, mirroring phase-04's relaxation per ADR-K extended to H2); a one-line blank import update to `test/differential/runner_test.go` registering fixture 0004's driver (no `H2Expectations` interface extension — fixture asserts in-band per SPEC §4.3 recommendation); a NEW `TestServerConn_GOAWAYOnProtocolError_StreamIDReuse` integration test in `internal/filter/hcm/h2/conn_test.go` covering the previously-uncovered monotonic-id-reuse rejection branch at `internal/filter/hcm/h2/conn.go:308-319` (the 05.1 REVIEW carry-forward per SPEC §12.3); a small mechanical M-8 cleanup (`excludedSubsections []string{"http2/6/6"}` is currently `//nolint:unused`-suppressed in `test/conformance/h2spec/h2spec.go` per the 05.1 REVIEW M-8 finding — promoted to a doc comment in `CONFORMANCE_PINS.md` per the SPEC §12.2 "fold-into-PLAN-as-5-minute-task" recommendation); FOUR new ADRs (ADR-0055..ADR-0058 — re-verified at Task 1 step 1 against `DECISIONS.md` tail being ADR-0054); a flipped `BEHAVIOR_CONTRACT.md ## HTTP/2` SCAFFOLD subsection (in-place edit per ADR-0052 — the "Asserted equivalence" + "Not asserted" + "Header allow-list extensions" + "Applies to" + "Does not yet apply to" subsections reflect the 05.1 + 05.2 unified scope per SPEC §5.7; the header allow-list table at the top has its `:method`/`:path`/`:scheme`/`:authority` rows flipped from `applies-to: phase 05.2 (forward-looking)` to `applies-to: phase 05.2 routed-to-upstream H2 (active per ADR-0057)`); STATE.md / ROADMAP.md / PROGRESS.md updates with row 05.2 → `done` AND parent row 05 → `done` at the same phase-done commit (per SPEC §4.4 — 05.1 is already `done` at `bc4fca4`, so 05.2's phase-done commit closes both rows). After phase 05.2, envoy-go originates upstream HTTP/2 over TLS — it dials, confirms ALPN, runs an own client connection manager, opens streams under flow-control discipline, multiplexes (capability — not exercised by the fixture, which uses per-request fresh conns per ADR-0056), and produces a structurally-equivalent end-to-end framing surface plus per-stream behaviourally-equivalent responses to upstream Envoy on a deterministic full-stack HTTPS h2 workload. Phase 05's parent ROADMAP row flips to `done` at this commit; the project advances to phase 06 (observability-baseline) at lifecycle-state 1.

**Architecture:** The 05.2 surface is the symmetric mirror of 05.1's server-side codec on the client side, plus the cluster-builder's H2-mode discriminator and the action-variant choice at filter-build time. Concretely: `internal/filter/hcm/h2/client.go` (the ONE new file in the 05.1 sub-package per ADR-0048's reservation; ~400 LoC) defines `type H2Request struct { Method, Path, Scheme, Authority string; Headers []hpack.HeaderField; Body []byte }` and `type H2Response struct { Status int; Headers []hpack.HeaderField; Body []byte }` — small value types internal to the codec sub-package per SPEC §10 #1's recommended introduction; `type ClientConn struct { fr *framer; hp *hpackState; sendW *window; recvW *window; nextStreamID uint32 (atomic); streams map[uint32]*clientStream; ...; goawayCh chan struct{}; closedOnce sync.Once }` is the per-upstream-conn manager; `NewClientConn(ctx context.Context, upstream net.Conn) (*ClientConn, error)` writes the 24-byte client preface (`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n` — the `clientPrefaceBytes` constant from 05.1's `preface.go` is exported within the package to be consumed here), writes the client's initial SETTINGS via the new `writeClientInitialSettings(fr *framer, s ServerSettings) error` helper in `settings.go` (the same `DefaultServerSettings` constants are shared per SPEC §2.1 — ADR-0047 settings symmetrically apply to the client surface; `ServerSettings` is mis-named only in the 05.1 sense — it carries SETTINGS values regardless of conn role), reads the server's initial SETTINGS via the existing `readClientSettings(fr *framer, applyTo *clientSettings) error` (renamed conceptually but kept as the existing symbol — ADR-0048's reuse-not-rename discipline), writes SETTINGS_ACK in response, and synchronously waits for the server's SETTINGS_ACK for our SETTINGS before returning per SPEC §10 #5 (synchronous wait inside the constructor surfaces handshake errors as constructor errors); a frame-read goroutine spawned at `NewClientConn` return time runs the conn-level read loop (DATA/HEADERS/RST_STREAM/SETTINGS/PING/WINDOW_UPDATE/GOAWAY dispatch — same shape as 05.1's `ServerConn.Run` minus the new-stream-from-HEADERS path because clients don't accept new streams from servers per `SETTINGS_ENABLE_PUSH=0` per ADR-0047; SPEC §10 #7 keeps the loops separate from the server-side loop); `(*ClientConn).RoundTrip(ctx, req H2Request) (H2Response, error)` allocates a fresh stream id via `atomic.AddUint32(&cc.nextStreamID, 2)` (returning 1 on first call per RFC 9113 §5.1.1 — the allocator initialises to `^uint32(0)` so the first add returns `0xFFFFFFFF + 2 ≡ 1 mod 2³²`; equivalently `atomic.AddUint32(&cc.nextStreamID, 2) - 2 + 2 == 1` after init — the cleaner shape: initialise to 0, allocate as `next := atomic.AddUint32(&cc.nextStreamID, 2) - 1` so first call returns 1, second 3, third 5; the planner picks the cleaner form), encodes HEADERS via the conn's `hpackState` (pseudo `:method`/`:path`/`:scheme`/`:authority` first per RFC 9113 §8.3, then regular headers in deterministic order — same discipline as 05.1's `directResponseAction.writeH2`), writes HEADERS frame with END_HEADERS+END_STREAM (or END_HEADERS only if body present + final DATA carries END_STREAM) chunked per ADR-0055/I-1+I-2 discipline, waits on a per-`clientStream` response channel (the conn-level frame-read goroutine routes inbound frames addressed to this stream id into the channel), assembles the response from inbound HEADERS+DATA frames respecting END_STREAM, decrements `recvW` per ADR-0055/I-3 and emits inbound WINDOW_UPDATE on the half-window threshold, observes-and-discards trailing HEADERS per ADR-0058, returns the assembled `H2Response`; on ctx cancel mid-RoundTrip emits RST_STREAM(CANCEL) on the upstream stream and returns ctx.Err(); `(*ClientConn).Close() error` (called via `defer` in `routerActionH2.do`) emits `WriteGoAway(highestStreamID, NO_ERROR, []byte("client close"))` per RFC 9113 §6.8 and closes the underlying conn; `internal/cluster/dial_h2.go` carries the 25-line `Cluster.DialH2` per SPEC §4.1's snippet — `Cluster.Dial(ctx)` (existing, unchanged) → `*stdtls.Conn` type-assert (else `cluster: dial h2: not a TLS conn`) → defensive `tlsConn.HandshakeContext(ctx)` (idempotent; SPEC §11.3 mitigation) → `NegotiatedProtocol` read (else `cluster: dial h2: alpn negotiated %q, want "h2"`) → `h2.NewClientConn(ctx, raw)` (else `cluster: dial h2: client conn: %w`) → return — each error path closes the conn explicitly (no defer-rescue because the function returns the conn on success and the caller takes ownership); `internal/cluster/manager.go`'s `buildCluster` is extended with an `extractH2Mode` helper that peeks at `proto.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]` and, when present, unmarshals into `*upstreamshttpv3.HttpProtocolOptions` (the blank-imported registration carries the proto descriptor; the `Manager.buildCluster` switch on `UpstreamProtocolOptions.(type)` handles four cases per SPEC §5.5: `*HttpProtocolOptions_ExplicitHttpConfig` with `*ExplicitHttpConfig_Http2ProtocolOptions` → `useH2=true` + validation; `*ExplicitHttpConfig_HttpProtocolOptions` (the H1 discriminator) → `useH2=false` silent-ignore; `*HttpProtocolOptions_AutoConfig` → `useH2=false` silent-ignore (the 05.2 narrowing of master SPEC §5.8 — silent-ignore preserves forward-compat as more discriminators land in later phases); nil/empty → `useH2=false`); validation when `useH2==true` requires `transport_socket` present + type `envoy.transport_sockets.tls` + `alpn_protocols` includes `"h2"` (build-time errors with the diagnostics enumerated in SPEC §4.1); `internal/cluster/cluster.go` gains a `useH2 bool` field set at build time, exposes `Cluster.UseH2() bool`, and acquires the blank import `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"`; `internal/bootstrap/bootstrap.go` gains the same blank import so fixture-0004 bootstraps round-trip cleanly through `protojson` per ADR-0016's blank-import amendment policy (registry-population mechanism, not a new ADR); `internal/filter/hcm/actions.go` gains `routerActionH2` per SPEC §4.1's snippet — `routerActionH2.do(ctx, req, w)` per the goal section's enumeration; the codec-neutral `Action` interface from 05.1 is satisfied by `routerActionH2`'s method set (it adapts via the new `h2RouterActionAdapter` in `h2dispatch.go`); `internal/filter/hcm/filter.go`'s `NewFilter` build path picks `routerActionH2` vs the existing `routerAction` per `Cluster.UseH2()` at filter-build time (the ALPN dispatch in `Filter.Handle` UNCHANGED from 05.1 per ADR-0050 — codec selection on the downstream is per-conn, the upstream codec choice is per-route); `internal/filter/hcm/h2dispatch.go`'s `h2Dispatcher.Match` is extended: when the matched route's action is `*routerActionH2`, return a new `h2RouterActionAdapter{a *routerActionH2}` wrapping the action; the existing `h2RouterActionRejection` sentinel stays as the fall-through for any non-H2 cluster reached on the H2 path; the existing `h2DirectResponseAdapter` stays unchanged (codec-neutral 05.1 surface); the flow-control tightening per ADR-0055 lands in five surgical patches (M-5 helper extraction in `framer.go`; M-3 `reserveBlocking` collapse in `flow.go`; I-1/I-2 outbound chunking in `conn.go`; I-3 inbound flow control + WINDOW_UPDATE emission in `conn.go`/`stream.go`; M-9 overflow bounds-check in `conn.go`/`stream.go`; M-11 state-before-append reorder in `stream.go`) — the regression tests live alongside the existing `flow_test.go` / `conn_test.go` / `stream_test.go` files; the `framer.translateFramerErr` helper is consumed by the new `ClientConn`'s framer wrapper to share the translation; `test/helpers/h2.go` exports `H2RoundTrip` for the fixture-0004 driver (driver-side use of `golang.org/x/net/http2.Transport` is permitted per D-3.2; runtime is not — the boundary grep at Task 15 step 9 catches violations); fixture `0004-h2-routing/` is the project's first full-stack H2 differential per the goal section's enumeration; the `BEHAVIOR_CONTRACT.md ## HTTP/2` subsection is edited in place per ADR-0052 with the exact prose from SPEC §5.7 (the SCAFFOLD-pattern in-place-edit authorisation IS the supersession-free mechanism); the four ADRs land at first-use commit ordering per the phase-02/03/04/05.1 precedent.

**Tech Stack:**
- Go 1.23 (unchanged from 05.1; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `net/http` (`http.TimeFormat` for the H2 502 local-reply Date header; `http.ReadResponse` is NOT consumed on the H2 path — the codec is byte-level), `bufio`, `context`, `io`, `net`, `strings`, `time`, `errors`, `fmt`, `log`, `sync`, `sync/atomic`, `crypto/tls` (the `*tls.Conn.ConnectionState().NegotiatedProtocol` read site for ALPN-confirm in `DialH2`).
- **`golang.org/x/net/http2.Framer` and `golang.org/x/net/http2/hpack`** — used as low-level codec only per doctrine `D-3.2` and ADR-0046. Phase 05.2 extends the production-side allowed-import list from 05.1's three files (`framer.go`, `settings.go`, `conn.go` per ADR-0054 correction of ADR-0046's prose) by ONE: `client.go` joins the list. The boundary grep at Task 15 step 9 verifies: `! grep -nR '"golang.org/x/net/http2"' internal/ cmd/envoy-go/main.go --include='*.go'` excluding `_test.go` files must return zero hits OUTSIDE `framer.go`/`settings.go`/`conn.go`/`client.go`.
- **Forbidden runtime imports (D-3.2):** `golang.org/x/net/http2.Server`, `http2.Server.ServeConn`, `http2.ConfigureServer`, `http2.Transport`, `http2.Transport.NewClientConn`. Driver-side test use of `x/net/http2.Transport` (in `cmd/envoy-go/main_test.go`, `internal/filter/hcm/h2/*_test.go`, `test/fixtures/0004-h2-routing/...`, `test/conformance/h2spec/...`, the new `test/helpers/h2.go`, and the new `test/fixtures/0004-h2-routing/backends/main.go` — driver-side `http2.ConfigureServer` consumption is OK there) is permitted because that is fixture infrastructure, not envoy-go runtime — D-3.2 governs runtime, not test code. The discipline is grep-verifiable at Task 15 step 9.
- **NEW: `github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3`** — proto package for the cluster-side `HttpProtocolOptions`. Blank-imported in two places per ADR-0016's blank-import amendment policy: `internal/cluster/cluster.go` (so the `cluster.Manager` builder can read the typed extension via protojson) and `internal/bootstrap/bootstrap.go` (so fixture-0004 bootstraps round-trip cleanly through the loader). Per ADR-0013's pin (`go-control-plane/envoy@v1.32.4`), the proto package is already on disk; no new external dependency lands in 05.2.
- `github.com/envoyproxy/go-control-plane/envoy` at v1.32.4 (ADR-0013 pin, unchanged).
- `google.golang.org/protobuf/types/known/anypb` (Any unmarshal for typed_config — same pattern as phase-04 / 05.1).
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0004's reference (Envoy in a Docker container) — same harness as 05.1's conformance gate consumes for h2spec; phase 05.2 does not modify `test/differential/harness.go`.
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0004's reference image.
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 05.2 — D-3.7 reserves pin bumps for dedicated phases).
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- The `internal/cluster` package — phase 05.2 introduces the FIRST changes to this package since phase 03 (the 05.1 REVIEW verified `internal/cluster/` was byte-for-byte unchanged from `ddf41cd` through 05.1's close at `536f353`). The 05.2 changes are: `cluster.go` (`UseH2()` accessor + blank import), `manager.go` (`buildCluster` extension), `dial_h2.go` (NEW). The phase-03 `Cluster.Dial(ctx)` is consumed but UNCHANGED.
- The `internal/tls` package — UNCHANGED in 05.2 (the phase-03 `alpn_protocols` plumbing already covers what 05.2 needs on the cluster side; the cluster's `transport_socket` + `alpn_protocols` use the same parsing as the listener's, per the phase-03 design).
- The `internal/listener` package — UNCHANGED in 05.2 (the 05.1 `NewManagerWithBaseDirAndAllowH2C` + `listenerCtx{hasTLS, allowH2C}` plumbing is sufficient; 05.2 does not introduce listener-side H2 changes).

---

## Scope check — why phase 05.2 ships as one sub-phase

Net change estimate: **~1500 LoC** (~400 production code in `internal/filter/hcm/h2/client.go` + ~250 `client_test.go`; ~80 `internal/cluster/dial_h2.go` + ~150 `dial_h2_test.go`; ~120 `internal/cluster/manager.go` extension + ~180 `manager_test.go`/`cluster_test.go` extension; ~30 `internal/cluster/cluster.go` + 30 `internal/bootstrap/bootstrap.go` (mostly imports); ~150 `internal/filter/hcm/actions.go` `routerActionH2` + ~150 `actions_test.go` extension; ~30 `filter.go` variant pick + ~60 `h2dispatch.go` adapter + ~80 test extension; ~200 ADR-0055 codec patches across `flow.go`/`framer.go`/`conn.go`/`stream.go` + ~250 regression tests; ~50 `test/helpers/h2.go` + 80 `h2_test.go`; ~600 fixture 0004 (`pki/gen/main.go` ~120, `backends/main.go` ~80, `envoy.yaml` ~80, `envoy-go.yaml` ~80, `expectations.yaml` ~30, `README.md` ~40, `driver/driver.go` ~200, `driver/driver_test.go` ~50); ~300 across the four ADRs in DECISIONS.md; ~30 BEHAVIOR_CONTRACT in-place edit; ~30 monotonic-id-reuse integration test). The split-gate threshold is **~1500 LoC OR ~25 numbered tasks** (`BOOTSTRAP_PROMPT.md` §6.1). Task count is **15** — well below the 25-task gate and within SPEC §11.1's anticipated 12–15 range (the planner adds Task 1 preconditions + Task 15 closing sweep beyond the SPEC's pure TDD-task count, matching phase-04 / 05.1 precedent). LoC estimate is at the soft threshold; comparable in magnitude to phase-04 (~2400) and 05.1 (~2400) which both shipped as one phase per `ddf41cd` / `536f353`'s scope checks.

Phase 05.2 ships as **one** sub-phase (not split into 05.2.1 + 05.2.2) for four reasons:

1. **The split-by-ADR-scope axis enumerated in SPEC §11.1 is reserved as a mid-execution release valve, not a pre-execution split.** SPEC §11.1 enumerates three plausible split axes and explicitly recommends "if under threshold (likely), do not split." This PLAN's LoC estimate is at the soft threshold (~1500), task count is comfortably under the gate (15 < 25), and the ADR-0055 flow-control discipline is bundled coherently as the 05.1 → 05.2 surface-extension prerequisite (the seven fixes are interlinked per SPEC §11.7's bundling rationale; separating them creates cross-references harder to read than the bundled form). The split-by-ADR-scope axis (05.2.1 = ADR-0055 only; 05.2.2 = rest) is preserved as a mid-execution valve per BOOTSTRAP §6.1's secondary trigger.

2. **Task count is at the SPEC's recommended low end; LoC estimate is the OR-leg with precedent.** Per phase-04 / 05.1 precedent, task-count-under-25 is the primary signal one phase is the right shape. 05.2's 15 tasks matches SPEC §11.1's expected 12–15 plus one preconditions task plus one closing sweep. The LoC estimate is at the soft threshold; the OR-leg has the phase-04 / 05.1 precedent of accepted ~2400-LoC one-phase shipments.

3. **The fixture 0004 differential is the load-bearing claim that defines this sub-phase's atomic scope.** Per BOOTSTRAP §6.3, "do not ship incomplete stubs that conformance tests can't exercise." A 05.2.1 sub-phase carrying only the codec + cluster builder would have no differential fixture (gate (a) vacuous for 05.2.1; non-vacuous only at 05.2.2) — that's TWO consecutive sub-phases under phase 05 with vacuous gate (a), which the SPEC §11.1 recommendation explicitly identifies as a "process smell." The fixture-0004 deliverable is what makes 05.2 atomically claimable as "envoy-go originates upstream HTTP/2 over TLS" per the SPEC §1's central engineering claim.

4. **Mid-execution split valve is preserved.** `BOOTSTRAP_PROMPT.md` §6.1's secondary trigger (any single task's sub-steps blow up past ~10 items once contact with reality reveals complexity) stays active. The two tasks most likely to blow past 10 sub-steps are Task 8 (`client.go` RoundTrip + frame-read goroutine — the largest single-file change in this phase, with tests spanning every state transition + every error code) and Task 14 (fixture 0004 driver — orchestrates 27-request workload + RR distribution assertions + PKI bootstrap timing). If either exceeds 15 sub-steps at execution time, the executor splits per §6.2 with a new ADR — the natural axis is split-by-ADR-scope per SPEC §11.1 (05.2.1 = ADR-0055 + the codec primitives; 05.2.2 = upstream surface + fixture 0004).

**Triggering re-evaluation:** if at execution time the cumulative landed-LoC count exceeds **2500** by the end of Task 11 (i.e., before fixture 0004's PKI + driver tasks), invoke `superpowers:systematic-debugging` on the estimate-vs-reality gap and re-evaluate. A ~70% miss on a carefully-bounded sub-phase is a signal the plan's shape is wrong, not just that the work is large.

---

## File Structure

| Path | Created/Modified/Deleted | Purpose |
|---|---|---|
| `internal/filter/hcm/h2/client.go` | Create | NEW client-side connection manager. Exports `H2Request`/`H2Response` value types (per SPEC §10 #1's recommended introduction); `ClientConn` (per-upstream-conn manager); `NewClientConn(ctx, upstream net.Conn) (*ClientConn, error)` (writes preface, exchanges initial SETTINGS synchronously per SPEC §10 #5, returns ready-to-`RoundTrip` conn); `(*ClientConn).RoundTrip(ctx, req H2Request) (H2Response, error)` (one stream per call, monotonic odd-numbered ids per RFC 9113 §5.1.1, ctx-cancel emits RST_STREAM(CANCEL), peer GOAWAY/RST_STREAM/PROTOCOL_ERROR translated to `*Error`); `(*ClientConn).Close() error` (graceful GOAWAY(NO_ERROR) with last-stream-id + TCP FIN). Reuses 05.1 `framer`/`hpackState`/`window`/`ServerSettings`/`ErrCode` primitives unchanged at the API level (the ADR-0055 internal patches to `flow.go`/`conn.go`/`stream.go`/`framer.go` apply transparently). The frame-read goroutine runs the conn-level read loop (DATA/HEADERS/RST_STREAM/SETTINGS/PING/WINDOW_UPDATE/GOAWAY dispatch — same shape as `ServerConn.Run` minus the new-stream-from-HEADERS path because `ENABLE_PUSH=0` per ADR-0047) per SPEC §10 #7's "keep loops separate" choice. ~400 LoC. |
| `internal/filter/hcm/h2/client_test.go` | Create | Exhaustive client-side coverage per SPEC §4.1 + §8.1: client-preface emit + SETTINGS exchange happy path; `RoundTrip` happy path (HEADERS+END_STREAM out, HEADERS+DATA+END_STREAM in); `RoundTrip` with body (HEADERS+DATA+END_STREAM out); `RoundTrip` ctx cancel during write → RST_STREAM(CANCEL); `RoundTrip` ctx cancel during read → RST_STREAM(CANCEL); `RoundTrip` peer RST_STREAM mid-response → `*Error{Code: peer's code}`; `RoundTrip` peer GOAWAY mid-conn → `*Error{Code: NO_ERROR}` for streams above last-stream-id; `RoundTrip` peer DATA after END_STREAM → `*Error{Code: STREAM_CLOSED}`; `Close` graceful GOAWAY(NO_ERROR) + underlying conn close; `RoundTrip` after `Close` returns error; multi-`RoundTrip` capability check (stream-id monotonicity 1, 3, 5 — exercises the allocator under the per-conn fresh-conn discipline of ADR-0056 not exploiting the multiplexing capability). Test peer is `golang.org/x/net/http2.Server` driver-side use (D-3.2 governs runtime, not test code; SPEC §4.1 explicitly authorises). ~250 LoC. |
| `internal/filter/hcm/h2/flow.go` | Modify | Per ADR-0055 / M-3 + the new `reserveBlocking` API consumed by both `ServerConn.writeData` and `ClientConn.writeData`: the existing `(*window) waitFor(ctx, n) error` + `(*window) reserve(n) (int32, error)` pair collapses into a single `(*window) reserveBlocking(ctx context.Context, max int32) (taken int32, err error)`. The new method holds `mu` across the wait + take so wait-and-take is atomic; the dead `if taken <= 0` recovery branch in 05.1's `writeData` (per the 05.1 REVIEW M-3 finding) is no longer reachable and gets deleted in Task 3. The existing `waitFor`/`reserve` exports are deleted (no external consumers — both methods are package-internal per the 05.1 surface). |
| `internal/filter/hcm/h2/flow_test.go` | Modify | Add ADR-0055 regression tests: race-detector test for `reserveBlocking` under concurrent multi-stream writes against a window primed at boundary values (no over-reservation per the M-3 atomicity fix); existing `waitFor`/`reserve`-style cases re-cast through `reserveBlocking`'s API. ~60 LoC delta. |
| `internal/filter/hcm/h2/framer.go` | Modify | Per ADR-0055 / M-5: extract a `translateFramerErr(err error) error` helper carrying the duplicated `http2.ConnectionError`/`StreamError`/`ErrFrameTooLarge` translation block from `readFrameCtx` and `tryReadFrame`. Both call sites consume the helper after the extraction; the new `ClientConn`'s framer wrapper (Task 7) consumes the same helper. Cosmetic prerequisite for the 05.1 → 05.2 codec-symmetry surface; no behavioural change. |
| `internal/filter/hcm/h2/framer_test.go` | Modify | Add a small unit test for `translateFramerErr` covering each branch (ConnectionError / StreamError / ErrFrameTooLarge / passthrough). ~30 LoC. |
| `internal/filter/hcm/h2/conn.go` | Modify | Per ADR-0055 / I-1 + I-2 + I-3 + M-9: `ServerConn.writeData` caps each chunk at `min(connWindow, streamWindow, peer.MaxFrameSize)` per I-1/I-2 — the chunk size is computed from `min(s.sendW.reserveBlocking(ctx, want), ss.sendW.reserveBlocking(ctx, want), int32(s.clientS.MaxFrameSize))` where `ss` is the per-stream view (per-stream send-window held by `serverStream.sendW`). `ServerConn.onData` decrements `s.recvW`/`ss.recvW` and emits `WriteWindowUpdate(0,n)`/`WriteWindowUpdate(streamID,n)` once a half-window high-water threshold is crossed per I-3 (the half-window policy is conventional; 05.1's `recvW` allocations are no longer dead per the M-7 disposition). `ServerConn.onWindowUpdate` adds `2³¹ - 1` overflow bounds-check per M-9; on overflow emits `GOAWAY(FLOW_CONTROL_ERROR)`. The dead `if taken <= 0` branch in `writeData` is deleted per M-3. ~70 LoC delta in production code. |
| `internal/filter/hcm/h2/conn_test.go` | Modify | Add ADR-0055 regression tests + the 05.1 carry-forward `TestServerConn_GOAWAYOnProtocolError_StreamIDReuse` integration test. The flow-control regression coverage: >16384-byte body chunked correctly with peer `MaxFrameSize: 16384` (≥2 DATA frames on the wire; no peer-side `FRAME_SIZE_ERROR`); `INITIAL_WINDOW_SIZE: 16` + 100-byte response body produces ~7 DATA frames + no `FLOW_CONTROL_ERROR`; >65 KB inbound body completes (no deadlock) with WINDOW_UPDATE emission verified on the wire; WINDOW_UPDATE delta totalling > 2³¹ - 1 → `GOAWAY(FLOW_CONTROL_ERROR)`. The id-reuse integration test (`TestServerConn_GOAWAYOnProtocolError_StreamIDReuse`) opens stream id N, completes it, then sends another HEADERS on the same N — assert `GOAWAY(PROTOCOL_ERROR)`. ~150 LoC delta. |
| `internal/filter/hcm/h2/stream.go` | Modify | Per ADR-0055 / M-9 + M-11: `serverStream.recvWindowUpdate` adds `2³¹ - 1` overflow bounds-check on the addition (on overflow emits `RST_STREAM(FLOW_CONTROL_ERROR)`); `serverStream.recvData` checks `s.state` BEFORE appending to `s.reqBody` (one-line reorder; eliminates the memory-waste path on closed streams flagged by the 05.1 REVIEW M-11). ~10 LoC delta. |
| `internal/filter/hcm/h2/stream_test.go` | Modify | Add regression tests for M-9 (delta-overflow `RST_STREAM(FLOW_CONTROL_ERROR)`) and M-11 (DATA on closed stream does NOT grow `s.reqBody`). ~50 LoC delta. |
| `internal/filter/hcm/h2/settings.go` | Modify | Add a `writeClientInitialSettings(fr *framer, s ServerSettings) error` helper alongside the existing `writeServerInitialSettings`. The two helpers are byte-identical in 05.2 (the same `DefaultServerSettings` constants per ADR-0047 apply on both sides; `SETTINGS_ENABLE_PUSH=0` is correct for the client per RFC 9113 §6.5.2 because clients can't accept PUSH — advertising it disabled is symmetric correctness); the separate helpers exist for future divergence (when client-only settings tuning lands in a future phase). The existing `readClientSettings` reads "the peer's initial SETTINGS" regardless of conn role; the conn driver determines what role-specific behaviour to apply on receipt. ~25 LoC delta. |
| `internal/filter/hcm/h2/settings_test.go` | Modify | Add a small unit test verifying `writeClientInitialSettings` writes the expected SETTINGS values (round-tripped via a `net.Pipe` peer reading the SettingsFrame). ~30 LoC. |
| `internal/cluster/cluster.go` | Modify | Add `useH2 bool` field to `Cluster` struct; add `Cluster.UseH2() bool` accessor (one-liner returning `c.useH2`). Add blank import `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` per ADR-0016's blank-import amendment policy. The existing `Cluster.Dial(ctx) (net.Conn, error)` is UNCHANGED — `DialH2` is a SEPARATE helper, not a method swap. ~10 LoC delta. |
| `internal/cluster/cluster_test.go` | Modify | Add unit tests for `Cluster.UseH2()` accessor (zero-value cluster returns `false`; explicit `useH2: true` returns `true`); `protojson` round-trip of a fixture-0004-shaped bootstrap (with `HttpProtocolOptions` on the cluster) round-trips cleanly via the blank-imported registration. ~40 LoC delta. |
| `internal/cluster/manager.go` | Modify | Extend `buildCluster` with `extractH2Mode(c *clusterv3.Cluster) (useH2 bool, err error)` per SPEC §5.5's behaviour matrix: peek at `c.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]`; absent → `useH2=false` (phase-04 baseline; no regression). Present → unmarshal into `*upstreamshttpv3.HttpProtocolOptions`; switch on `UpstreamProtocolOptions.(type)`: `*HttpProtocolOptions_ExplicitHttpConfig` → switch on `ProtocolConfig.(type)`: `*ExplicitHttpConfig_Http2ProtocolOptions` → `useH2=true` + run validation; `*ExplicitHttpConfig_HttpProtocolOptions` → `useH2=false` (silent-ignore inner fields); other → `useH2=false`. `*HttpProtocolOptions_AutoConfig` → `useH2=false` (silent-ignore — the 05.2 narrowing of master SPEC §5.8). Nil/empty → `useH2=false`. Validation when `useH2==true`: `transport_socket` MUST be present (else `cluster %q: HttpProtocolOptions.http2_protocol_options requires transport_socket`); transport_socket MUST be type `envoy.transport_sockets.tls` (else `cluster %q: HttpProtocolOptions.http2_protocol_options requires transport_socket of type tls, got %q`); the parsed TLS config's `alpn_protocols` MUST include `"h2"` (else `cluster %q: HttpProtocolOptions.http2_protocol_options requires alpn_protocols to include "h2", got %v`). The error wrapping uses the existing diagnostics shape ("cluster: %q: ..."). ~120 LoC delta. |
| `internal/cluster/manager_test.go` | Modify | Add unit tests per SPEC §4.1 + §8.2: H2-cluster build positive (`http2_protocol_options{}` + TLS + ALPN h2 → builds successfully + `UseH2()==true`); H2-cluster without TLS → build error; H2-cluster with TLS but `alpn_protocols` lacks `h2` → build error; H2-cluster with TLS but no `alpn_protocols` field → build error; H1 discriminator (`http_protocol_options{}`) → silent-ignore + `UseH2()==false`; `auto_config{}` → silent-ignore + `UseH2()==false` (the 05.2 narrowing); cluster with no `typed_extension_protocol_options` map → `UseH2()==false` (phase-04 baseline). ~140 LoC delta. |
| `internal/cluster/dial_h2.go` | Create | NEW. `Cluster.DialH2(ctx context.Context) (*h2.ClientConn, error)` per SPEC §4.1's snippet — `Cluster.Dial(ctx)` → `*stdtls.Conn` type-assert (else `cluster: dial h2: not a TLS conn`) → defensive `tlsConn.HandshakeContext(ctx)` (idempotent for already-handshaken conns; SPEC §11.3 mitigation) → `NegotiatedProtocol` read (else `cluster: dial h2: alpn negotiated %q, want "h2"`) → `h2.NewClientConn(ctx, raw)` (else `cluster: dial h2: client conn: %w`) → return. Each error branch closes the conn explicitly (no defer-rescue because the function returns the conn on success and the caller takes ownership; mirrors the discipline 05.1's `Filter.Handle` uses for ALPN dispatch error paths). ~30 LoC. |
| `internal/cluster/dial_h2_test.go` | Create | NEW. `DialH2` happy path against an in-process h2-over-TLS backend (uses `crypto/tls` + `golang.org/x/net/http2.ConfigureServer` driver-side); `DialH2` over an h2-over-TLS backend negotiating `http/1.1` instead of `h2` (alpn-mismatch error); `DialH2` over a plaintext backend (not-TLS error); `DialH2` with a cancelled context (dial-timeout error path); `DialH2` over a TLS backend that fails the handshake (TLS error path bubbles through `Cluster.Dial`'s existing error handling — phase-03 surface). ~150 LoC. |
| `internal/bootstrap/bootstrap.go` | Modify | Add blank import `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` so fixture-0004 bootstraps round-trip cleanly through `protojson` per ADR-0016's blank-import amendment policy. The addition slots in among the existing 05.1 `extensions/filters/network/http_connection_manager/v3` blank import + the phase-03 `extensions/transport_sockets/tls/v3` import. Per ADR-0016 the addition is documented in PROGRESS, not a new ADR. ~3 LoC delta. |
| `internal/filter/hcm/actions.go` | Modify | Add `routerActionH2` action variant per SPEC §4.1's snippet alongside the existing phase-04 `routerAction` (UNCHANGED) and the codec-neutral `directResponseAction` (UNCHANGED from 05.1). `routerActionH2{cluster *cluster.Cluster}`; `routerActionH2.do(ctx, req h2.H2Request, w h2.StreamWriter) error`: calls `r.cluster.DialH2(ctx)` (502 local-reply on dial failure with byte-equal-to-H1 body `bad gateway\n` per SPEC §10 #4 + §11.9; the Date header IS included via the same `dateNowRFC7231` helper 05.1's `directResponseAction.writeH2` uses); `defer cc.Close()` (per-request fresh conn per ADR-0056); `cc.RoundTrip(ctx, req)` (502 on `*h2.Error` non-CANCEL/non-context errors; RST_STREAM(CANCEL) via `w.RST(h2.ErrCancel)` on `errors.Is(err, context.Canceled)` || `errors.Is(err, context.DeadlineExceeded)`; verbatim 5xx HTTP-status-level forward — only protocol errors translate to 502); writes `w.Headers(resp.Headers, false)` (pseudo `:status` first per RFC 9113 §8.3) + `w.Data(resp.Body, true)` (END_STREAM via DATA-with-END per SPEC §5.3 step 5 + ADR-0058). The 502 body string is extracted to a shared package-level constant `bad502Body = "bad gateway\n"` so the H1 path's 502 wording (currently inlined via `writeStatusReply(bw, 502, "")` per phase-04 — empty body; the H2 502 has a non-empty body) and the H2 path's 502 wording are grep-verifiable as a single source per SPEC §11.9's mitigation; the constant is defined where both action types can consume it. ~150 LoC delta. |
| `internal/filter/hcm/actions_test.go` | Modify | Extend per SPEC §4.1 + §8.3: `routerActionH2.do` happy path (in-process h2 backend; fake `h2.StreamWriter` captures `:status` + body; assert match); 502 on dial failure (cluster pointing at a closed port; assert `:status 502` + body `bad gateway\n` via the captured writer); 502 on RoundTrip protocol error (in-process backend emitting a malformed HEADERS frame); ctx cancel during RoundTrip → `w.RST(h2.ErrCancel)` observed on the captured writer; upstream returns 5xx → forwarded verbatim (NOT translated to 502). Plus a build-time variant-selection unit test that constructs a route pointing at a `UseH2()==true` cluster vs a `UseH2()==false` cluster and asserts the produced action types (`*routerActionH2` vs `*routerAction`). ~150 LoC delta. |
| `internal/filter/hcm/filter.go` | Modify | UNCHANGED at the `Filter.Handle` / `runH2` level — the ALPN dispatch in `Handle` is per ADR-0050 from 05.1 and 05.2 does not change it. The build-path change happens in `internal/filter/hcm/config.go` (or its called helpers), where `buildRouterAction` is extended to inspect `Cluster.UseH2()` and return `*routerActionH2` instead of `*routerAction` when true. Net delta in `filter.go`: 0 LoC. The actual code change is in `config.go` Step 3 of Task 11. |
| `internal/filter/hcm/config.go` | Modify | `buildRouterAction(r *routev3.RouteAction, clusters *cluster.Manager) (action, error)` (return type widened to the codec-neutral `action` interface — currently returns `*routerAction`) is extended to look up the resolved cluster's `UseH2()`; on `true` returns `&routerActionH2{cluster: c}`; on `false` returns the existing `&routerAction{cluster: c}`. The function signature widens its return type from `*routerAction` to the existing `action` interface (already present in `actions.go` per the 05.1 codec-neutral factoring). Callers (`buildAction`) consume the wider type without further change. ~15 LoC delta. |
| `internal/filter/hcm/config_test.go` | Modify | Add a unit test `TestBuildRouterAction_PicksH2VariantByClusterUseH2` that builds two clusters (one with H2 typed options, one without), constructs an HCM with two routes targeting each, and asserts the resulting `routeTable.entries[i].action` types (`*routerActionH2` vs `*routerAction`). ~50 LoC delta. |
| `internal/filter/hcm/h2dispatch.go` | Modify | Extend `h2Dispatcher.Match` per SPEC §4.1: when the matched route's action is `*routerActionH2` (a NEW concrete type), return a NEW `h2RouterActionAdapter{a *routerActionH2}` wrapping the action. The existing `h2RouterActionRejection` sentinel stays as the fall-through for any non-H2 cluster reached on the H2 path (a route whose target cluster has `UseH2()==false`, dispatched on an H2 listener — runtime per-stream INTERNAL_ERROR + RST_STREAM per SPEC §5.2 step 4c carried forward from 05.1; this is unreachable in well-formed bootstraps but defended for defence-in-depth). The existing `h2DirectResponseAdapter` stays unchanged. The new adapter's `WriteH2(sw h2.StreamWriter) error` method delegates to `a.a.do(<some-ctx>, <some-h2.H2Request>, sw)` — but the ctx and req must be threaded through the dispatch chain. **Concretely: the `h2.Action` interface gains a second method shape OR the existing `WriteH2(StreamWriter) error` shape is extended to also receive the request.** Per Task 11 step 1: extend `h2.Action` to `WriteH2(ctx context.Context, req H2Request, sw StreamWriter) error` (breaking change to the 05.1 surface; the existing two adapters are updated to match the new signature; `h2DirectResponseAdapter.WriteH2` ignores `ctx` and `req` because direct_response doesn't consume them). The signature extension is a cross-phase concern but small enough to absorb without an ADR — recorded in the SPEC §13 acceptance bullet "the codec-neutral interface is the same one `directResponseAction` already satisfies (per the 05.1 codec-neutral factoring)" via the natural extension path. ~80 LoC delta in `h2dispatch.go` + ~10 LoC delta in `stream.go`'s `dispatch` invocation site. |
| `internal/filter/hcm/h2/stream.go` | Modify (further) | The `serverStream.dispatch` invocation site of `action.WriteH2(sw)` is updated to pass `(ctx, req H2Request, sw)` per the interface widening above. The `H2Request` value is constructed from the inbound HEADERS pseudo-headers + decoded regular headers + the request body — the same data that `serverStream.dispatch` currently materialises into a `*http.Request` for the route-match step (see `buildRequest` at `stream.go:295`). The `H2Request` is a separate value path from the route-match's `*http.Request` (used for path/method matching only); the codec-neutral interface's `H2Request` carries the verbatim header set so the upstream-bound encoding preserves byte-equality on `:method`/`:path`/`:scheme`/`:authority` per ADR-0057's "request preservation" rule. ~25 LoC delta. |
| `internal/filter/hcm/h2/stream_test.go` | Modify (further) | Add coverage for the wider `WriteH2(ctx, req, sw)` interface. ~40 LoC delta. |
| `test/helpers/h2.go` | Create | NEW. `H2RoundTrip(ctx context.Context, addr string, tlsConf *tls.Config, method, path string, headers []hpack.HeaderField, body []byte) (status int, respHeaders []hpack.HeaderField, respBody []byte, err error)`. Dials TLS with `NextProtos: ["h2"]`; constructs a `*http2.ClientConn` via `golang.org/x/net/http2.Transport.NewClientConn` (driver-side use OK per D-3.2); issues one request; reads the response; returns. Used by the fixture-0004 driver. Returns the body separately from the response for byte-comparison convenience. ~50 LoC. |
| `test/helpers/h2_test.go` | Create | NEW. Test peer is an in-process h2 server (using `golang.org/x/net/http2.Server` driver-side); `H2RoundTrip` issues a `GET /test` and asserts the status + body. ~80 LoC. |
| `test/fixtures/0004-h2-routing/envoy-go.yaml` | Create | NEW. Subject bootstrap. 1 listener `l_h2` binding `127.0.0.1:0` with `transport_socket: tls` carrying the fixture's PKI + `alpn_protocols: ["h2", "http/1.1"]`. 1 filter chain with empty `filter_chain_match`. 1 HCM network filter with `codec_type: AUTO` so ALPN drives codec selection per-conn (the driver advertises only `h2`). 3 routes: `/health` direct_response 200 body `OK\n`; `/api` (prefix) → cluster `c_h2_backend`; `/missing` direct_response 404 (body relaxed). 1 STATIC cluster `c_h2_backend` with three TLS upstream endpoints carrying `transport_socket: tls` + `alpn_protocols: ["h2"]` + `validation_context` against the fixture-local CA. The cluster has `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"] = {explicit_http_config: {http2_protocol_options: {}}}` (empty inner — every inner field silently ignored at 05.2 per SPEC §1 #3). ~80 LoC. |
| `test/fixtures/0004-h2-routing/envoy.yaml` | Create | NEW. Reference bootstrap. Same listener shape, same HCM, same routes. 1 STRICT_DNS cluster `c_h2_backend` pointing at `host.docker.internal:<dyn>` × three TLS ports per ADR-0010 (`dns_lookup_family: V4_ONLY`); same `transport_socket` + `HttpProtocolOptions` shape as the subject. The HCM `route_config` is identical between the two (verbatim, modulo cluster.address differences). The reference is invoked with `--concurrency 1` per ADR-0028 (single-worker Envoy keeps RR distribution deterministic). ~80 LoC. |
| `test/fixtures/0004-h2-routing/expectations.yaml` | Create | NEW. Prose description of the 27-request workload (mirroring fixture 0003's prose form per the M-6 carry-forward decision from 05.1 ADR-0053). Allow-list lines for `Date`, `Server`, `Content-Type`, `Content-Length`, `x-envoy-*`, `x-forwarded-*`, `x-request-id`, plus the H2-pseudo-header presence rule (`:status` required + asserted; `:method`/`:path`/`:scheme`/`:authority` required + asserted on the upstream-side via in-process backend assertions on received pseudo-headers). ~30 LoC. |
| `test/fixtures/0004-h2-routing/README.md` | Create | NEW. Explains the fixture's purpose (HCM H2 dispatch + route match + router H2 + direct_response H2 + closes ADR-0035 H2 leg per ADR-0057), the STATIC-vs-STRICT_DNS divergence (per ADR-0027), the ALPN-h2 e2e shape, the upstream-TLS-now-exercised closure of ADR-0035 H2 leg, the `--concurrency 1` reference pin (ADR-0028), the per-side `[3,3,3]` RR distribution rule (local-correctness only — cross-side sequence is NOT asserted, mirroring phase-04's relaxation per ADR-K extended to H2). The PKI regeneration procedure (`go generate ./test/fixtures/0004-h2-routing/...`) is documented in this README per fixture-0002 precedent. ~40 LoC. |
| `test/fixtures/0004-h2-routing/pki/gen/main.go` | Create | NEW. Deterministic PKI generator emitting CA + 4 leafs (1 for the listener server cert; 3 for the H2 backend endpoints). Uses fixed seeds for the RSA keys per fixture-0002 precedent — committed PEMs are deterministic; CI does NOT run `go generate` automatically; the README documents regeneration. SANs include `localhost` + `127.0.0.1` + `host.docker.internal` so both subject (loopback) and reference (testcontainers Docker bridge → host) trust the same chain. Mirrors `test/fixtures/0002-tls-tcp/pki/gen/main.go`'s structure. ~120 LoC. |
| `test/fixtures/0004-h2-routing/pki/*.pem` | Create | Generated artefacts; committed (mirroring fixture 0002's discipline). 5 certs (1 CA + 1 listener + 3 backend leafs) + 5 keys = 10 PEM files. |
| `test/fixtures/0004-h2-routing/backends/main.go` | Create | NEW. Small Go program: TLS listener with `NextProtos: ["h2"]` + `http2.ConfigureServer` driver-side use (D-3.2 governs runtime, not test backends — a backend in `test/fixtures/.../backends/main.go` is fixture infrastructure, not envoy-go runtime). Reads `--port`, `--cert`, `--key`, `--ca` flags + `BACKEND_IDX` env var. Handler returns `backend-<idx>:<path-suffix>` for `/api/v1/<n>` paths (e.g. `backend-1:v1/3` for backend index 1 receiving `/api/v1/3`); returns `OK\n` for `/health`; returns `not found\n` with 404 for unknown paths (the driver tolerates this; the proxy's 404 covers `/missing/*`). ~80 LoC. |
| `test/fixtures/0004-h2-routing/driver/driver.go` | Create | NEW. `BackendCount() = 3`. `BackendKind() = fixture.HTTPSH2` (NEW kind — see Task 13 step 1). `SubjectListenerName() = "l_h2"`. `ReferenceListenerPort() = 15004`. `ReferenceBootstrap(backendPorts)` and `SubjectConfig` render the YAMLs with the three backend ports. `DriveReference(ctx, addr)` / `DriveSubject(ctx, addr)`: each issues 27 sequential H2 requests via `helpers.H2RoundTrip` (9 × `/health` → status 200, body `OK\n`; 9 × `/api/v1/<n>` → status 200, body backend-determined; 9 × `/missing/<n>` → status 404, body relaxed). `Transport.DisableKeepAlives = true` AND a fresh `Transport` per request to keep RR distribution deterministic. Returns the concatenated 9 `/health` body bytes as the diff-string (mirroring fixture-0003's `drive` shape). `AssertDistribution(refCounts, subjCounts []uint64) error` checks each side's `c_h2_backend` distribution is `[3,3,3]` over the 9 router-action requests. `ProbeAdmin` — same as phase 02/03/04 (admin remains H1 in 05.2). ~200 LoC. |
| `test/fixtures/0004-h2-routing/driver/driver_test.go` | Create | NEW. Distribution-assertion helper unit test (mirror of fixture 0003's). Exercises the `[3,3,3]`-vs-`[4,3,2]` discrimination edge case. ~50 LoC. |
| `test/differential/runner_test.go` | Modify | Add the blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver"` so the fixture registers via its `init()`. The runner does NOT require a new interface extension (no `H2Expectations`) — fixture asserts in-band per SPEC §4.3 recommendation. 1-line delta. |
| `test/differential/fixture/fixture.go` (or wherever `BackendKind` lives) | Modify | Add `HTTPSH2` constant alongside the existing `TCPEcho`, `HTTPEcho`, etc. The harness's backend-startup routine learns to spawn `test/fixtures/0004-h2-routing/backends/main.go` for `HTTPSH2` kind (discovers cert paths from a fixture-local convention; the harness already passes fixture-local file paths for fixture-0002's TLS PKI). ~30 LoC delta. |
| `test/conformance/h2spec/h2spec.go` | Modify | Per the 05.1 REVIEW M-8 cleanup: the `excludedSubsections []string{"http2/6/6"}` slice is currently `//nolint:unused`-suppressed. Promote to a `const excludedSubsection6_6 = "http2/6/6"` reference that is documented (but unused at the production-call-site level — the exclusion is enforced by the CONFORMANCE_PINS.md threshold list, not by a code path), then either remove the slice and rely on the doc comment OR keep the slice as an explicit reference for `assertThreshold`'s threshold-check audit trail. Recommendation: delete the slice (the `//nolint` is the smell — the field is documentation-only; documentation belongs in `CONFORMANCE_PINS.md`'s threshold-section list and h2spec.go's package doc). 5-LoC delta + a 2-line addition to `CONFORMANCE_PINS.md` documenting the section-6.6 exclusion rationale (push disabled per ADR-0047 / SPEC §2.1). |
| `docs/envoy-go/CONFORMANCE_PINS.md` | Modify | Add a 2-line note under the threshold-section enumeration documenting why section 6.6 is excluded (push disabled per ADR-0047 / SPEC §2.1). The 05.1 follow-up batch already landed the `## Refresh procedure` per the I-4 finding (commit `4ec0f7d` or similar; verified at Task 1 step 1). 5-LoC delta. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | Modify | In-place edit per ADR-0052's authorisation. The `## HTTP/2` SCAFFOLD subsection (lines 267-315) is rewritten per SPEC §5.7's exact prose; the header allow-list table at the top of the file has its `:method`/`:path`/`:scheme`/`:authority` rows flipped from `applies-to: phase 05.2 (forward-looking)` to `applies-to: phase 05.2 routed-to-upstream H2 (active per ADR-0057)`. The "Asserted equivalence" subsection's title flips from "(05.1 scope)" to "(05.1 + 05.2 scope)" with the new bullets; "Not asserted" gets new bullets per SPEC §5.7; "Header allow-list extensions" updates the prose; "h2spec threshold" gets the ADR-0055 prose-extension paragraph; "Applies to (05.1 + 05.2)" enumerates 05.2's NEW surfaces; "Does not yet apply to" REMOVES routed-to-upstream H2 + fixture 0004 (now active). The edit is in place — no supersession ADR for ADR-0052. ~100 LoC delta. |
| `docs/envoy-go/DECISIONS.md` | Modify | Append four ADRs at execution time per the first-use commit ordering: ADR-0055 (Task 5; bundles I-1/I-2/I-3 + M-3/M-5/M-7/M-9/M-11), ADR-0056 (Task 9; per-request fresh upstream H2 dial), ADR-0058 (Task 11; trailers observed but not forwarded — folds in M-4/M-10 carry-forward from the 05.1 REVIEW per SPEC §12.2 — the planner picks bundle vs separate at PLAN-write time; bundle into ADR-0058 as a Carry-forward subsection per the SPEC's per-finding-disposition guidance), ADR-0057 (Task 14; closes ADR-0035 H2 leg). The numbering is sequential by first-use commit order, NOT by topical grouping (the 05.1 PLAN's precedent: ADR-0046..ADR-0053 land in commit order). ~300 LoC across the four ADRs. |
| `docs/envoy-go/ROADMAP.md` | Modify (twice) | First modification: at the SPEC commit (already landed at `dacf4b7`, before this PLAN commit) — row 05.2 `planned → in-progress`. This was done by the SPEC-authoring session per BOOTSTRAP §4.1 invariant 3. Second modification: at the phase-done commit (Task 15) — row 05.2 `in-progress → done` AND row 05 (parent) `in-progress → done` AT THE SAME COMMIT (per SPEC §4.4; 05.1 is already `done` at `bc4fca4`). ~3 LoC delta at the phase-done commit. |
| `docs/envoy-go/STATE.md` | Modify (multiple times) | Updated at each lifecycle transition per BOOTSTRAP §5. The PLAN-authoring session (this session) advances STATE to lifecycle-state 3 + `next-skill: superpowers:subagent-driven-development` + `next-skill-scope: <execute PLAN.md>` + `last-commit: <PLAN.md commit SHA>`. The implementation session (next-fresh) re-enters at state 3 and advances through 4 (verification) → 5 (review) → 6 (phase-done). At state 6 the phase-done commit advances STATE to phase 06 (observability-baseline) at lifecycle-state 1. |
| `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` | Create (Task 1) + Modify (each subsequent task) | Append-only log per the phase-02/03/04/05.1 convention. Each task lands one entry quoting command outputs verbatim. The Task 15 closing entry carries the SPEC §13 acceptance-checklist verification block (re-runs every gate, records every command output). |
| `docs/envoy-go/phases/05.2-upstream-h2/REVIEW.md` | (NOT IN PLAN) | The REVIEW lands in a separate session per BOOTSTRAP §5 state 5 (`superpowers:requesting-code-review`). The PLAN does not pre-author REVIEW. |

---

## ADRs introduced by this plan

Four ADRs land at execution time. Each is the first-use task's responsibility and goes into the same commit as the code that consumes it. All entries in `DECISIONS.md` are append-only (D-3.5); no landed ADR is edited. ADR numbering continues from the tail verified at PLAN-write time (**ADR-0054** is the current tail, verified by `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` → `## ADR-0054:` at the `bc4fca4` baseline; re-verified at Task 1 step 1). Per SPEC §4.4 + §10 #9, phase 05.2's four ADRs land at ADR-0055..ADR-0058. The SPEC anticipated topics (flow-control discipline, per-request fresh upstream dial, ADR-0035 H2 leg closure, trailers observed-but-not-forwarded) are assigned to the four numbers in **first-use commit order** (phase-02/03/04/05.1 precedent).

The topic-to-ADR-number map:

- **SPEC §4.4 ADR-0055 anticipation** (Flow-control discipline for from-scratch H2 codec) → **ADR-0055** (lands Task 5, the closing task of the ADR-0055 fix sequence; first ADR landed of the four).
- **SPEC §4.4 ADR-0056 anticipation** (Per-request fresh upstream H2 dial) → **ADR-0056** (lands Task 9, first use of `Cluster.DialH2` in production).
- **SPEC §4.4 ADR-0058 anticipation** (Trailers observed but not forwarded — H2 router) → **ADR-0058** (lands Task 11, first use of `routerActionH2.do`'s observe-discard trailer rule). The ADR also folds in the 05.1 REVIEW Minor carry-forwards M-4 (`readClientPreface` not ctx-aware) and M-10 (`SETTINGS_TIMEOUT` absent) per SPEC §12.2's per-finding disposition (deferred-with-tag): the bundle keeps the carry-forward log auditable in one place; the planner chose bundle-into-ADR-0058 over a separate carry-forward ADR because the trailer rule and the carry-forward bookkeeping are both phase-bookkeeping concerns rather than discrete design choices.
- **SPEC §4.4 ADR-0057 anticipation** (Closes ADR-0035 H2 leg via fixture 0004's full-stack HTTPS h2) → **ADR-0057** (lands Task 14, first use of fixture-0004's full-stack HTTPS h2 surface — the closing ADR before the BEHAVIOR_CONTRACT in-place edit at Task 15).

Note: the FIRST-USE ORDERING is Tasks 5, 9, 11, 14 — i.e. ADR-0055 first, ADR-0056 second, ADR-0058 third, ADR-0057 fourth. This produces a non-monotonic ADR-number-vs-commit-order sequence (0055, 0056, 0058, 0057) which is intentional per the topical-vs-commit-order tradeoff: SPEC §4.4 + §10 #9 explicitly anticipate the four numbers as a CONTIGUOUS BLOCK (ADR-0055..ADR-0058) and the planner is required to honour the SPEC's anticipated topic mapping. The 05.1 PLAN precedent had monotonic mapping by accident (the SPEC's lettered topics aligned with the first-use task order); the 05.2 SPEC's anticipated topics map to slightly different first-use orderings, so monotonic ADR numbering would either re-order the SPEC's anticipated topics (violating SPEC §4.4) or re-order the first-use tasks (violating the topical-coherence-of-the-implementation-order). The non-monotonic mapping is the correct choice; the PLAN documents it explicitly so the executor doesn't "fix" the ordering at execution time.

Summaries:

- **ADR-0055 — Flow-control discipline for from-scratch H2 codec.** Status: Accepted. Date: 2026-04-26 (or task-execution date). Doctrine: D-3.6 (every phase is a green build — and the from-scratch codec must be RFC-correct under realistic peer settings, not just under the conformance-suite peer's defaults). Settles: the 05.1 REVIEW Important findings I-1 (`ServerConn.writeData` not respecting `SETTINGS_MAX_FRAME_SIZE`), I-2 (`ServerConn.writeData` not respecting per-stream send window), I-3 (receive-side flow control allocated but never enforced), and Minor findings M-3 (`writeData` dead `if taken <= 0` recovery branch + `waitFor`+`reserve` non-atomicity), M-5 (`framer.readFrameCtx`/`tryReadFrame` translation block duplication — cosmetic prerequisite so `ClientConn`'s framer wrapper consumes the same translation), M-7 (`recvW` fields kept-and-consumed under I-3), M-9 (WINDOW_UPDATE delta overflow not bounds-checked), M-11 (`recvData` writes to `s.reqBody` before checking state-transition validity). Context: the 05.1 codec primitives implemented the RFC 9113 §5.2 baseline but had three dormant gaps the 05.1 REVIEW flagged as Important — dormant because every shipped 05.1 path was a bodyless GET to a `direct_response` with a small body. Phase 05.2's routed-to-upstream H2 is the load-bearing surface that activates these gaps; ADR-0055 fixes them as the prerequisite. Decision: enumerates the seven specific code-level fixes (I-1 outbound `MaxFrameSize` chunking; I-2 per-stream send-window enforcement on outgoing DATA; I-3 inbound `recvW` decrement + half-window WINDOW_UPDATE emission; M-3 `reserveBlocking` collapse + dead-branch deletion; M-9 overflow bounds-check on `serverStream.recvWindowUpdate`/`ServerConn.onWindowUpdate`; M-11 `recvData` state-before-append reorder; M-5 `translateFramerErr` helper extraction). The seven fixes are interlinked: `reserveBlocking` is required for per-stream send-window enforcement to be race-free; the overflow bounds-check is required for the WINDOW_UPDATE emission to be safe under adversarial peers. Consequences: (a) the 05.1 codec primitives are now load-bearing for realistic upstream H2 workloads and bear regression tests for each fix (per SPEC §1 #6 + the ADR's Settles list); (b) `BEHAVIOR_CONTRACT.md ## HTTP/2`'s threshold-language paragraph is extended (in-place per ADR-0052) with non-default `MaxFrameSize` and tight-window prose, but per-section pass counts at the ADR-0051 pin remain at the 05.1 baseline (no new section requirements); (c) the bundled-vs-per-fix-ADR shape is intentional per the 05.1 REVIEW.md `Recommendation` Path A wording ("a single dedicated ADR documenting flow-control discipline for the from-scratch H2 codec end-to-end") — separating the seven fixes into per-fix ADRs would create cross-references harder to read than the bundle. Lands in Task 5 (the closing task of the ADR-0055 fix sequence at Tasks 2-5). Supersedes nothing.

- **ADR-0056 — Per-request fresh upstream H2 dial.** Status: Accepted. Date: task-execution date. Doctrine: D-3.5 (record cross-phase decisions). Settles: SPEC §10 #2.3 (newly out-of-scope at 05.2). Mirrors phase-04 ADR-0039 (per-request fresh upstream H1 dial). Context: H2's multiplexing benefit (one conn carrying many streams) is fundamentally NOT realised when the upstream dial happens once per router-action invocation. SPEC §2.1 enumerates upstream H2 stream pooling / multiplexing across requests as deferred to the upstream-robustness family; this ADR is the formal disposition record at phase 05.2. Decision: every `routerActionH2.do` invocation calls `r.cluster.DialH2(ctx)` to obtain a *fresh* `*h2.ClientConn`; within a single invocation, exactly one stream is opened on the new conn; the conn is closed immediately after the response is consumed via `defer cc.Close()`. Cross-invocation pooling is the upstream-robustness family's deliverable. The phase-05.2 differential surface does NOT assert pool/non-pool — Envoy pools, envoy-go does not, both produce the same per-request `:status`/body output, both produce per-side `[3,3,3]` distribution under the sequential-request workload, the cross-conn frame counts differ but those are not in the equivalence matrix. Consequences: (a) under load, latency increases linearly with request rate; tail latency suffers from TLS handshake variance — intentional in 05.2, mitigated by ADR-0056's "production guidance: pooling is required for production workloads" clause; (b) the upstream-robustness family, when it lands, brings H2 pooling and supersedes ADR-0056 with a pooling discipline ADR; (c) the carry-forward from phase-04 ADR-0053's "phase-05.2-will-repeat-the-pattern" note is now resolved — 05.2 introduces the analogous prose-vs-mechanism shape (the `defer cc.Close()` in `routerActionH2.do`), formally acknowledging the cosmetic gap that ADR-0053 deferred to a future SPEC-corrections ADR. Lands in Task 9 (first use of `Cluster.DialH2`). Supersedes nothing.

- **ADR-0058 — Trailers observed but not forwarded — H2 router.** Status: Accepted. Date: task-execution date. Doctrine: D-3.4 (record durable design rationale where context-isolation requires it). Settles: SPEC §2.1 (Trailer support — request and response — out-of-scope), SPEC §10 #1 (the trailer rule reaffirmed for the 05.2 client surface), and the 05.1 REVIEW Minor carry-forwards M-4 (`readClientPreface` not ctx-aware — bundled per SPEC §12.2's per-finding-disposition; deferred to phase 06 or 07 with the proper fix at the listener-manager level via uniform OS read deadlines) and M-10 (`SETTINGS_TIMEOUT` absent — bundled per SPEC §12.2's per-finding-disposition; deferred to phase 06 or 08 with the proper fix at the listener-manager's per-conn timeout policies). Context: the 05.1 H2 server-side codec correctly observes trailing HEADERS frames per RFC 9113 §8.1 (h2spec section 8 asserts this); the 05.2 H2 client-side codec also observes them on the upstream conn; but the `routerActionH2` action discards trailers in BOTH directions (downstream-from-client trailing HEADERS are observed by `ServerConn` and discarded by `serverStream`'s dispatch; upstream-from-server trailing HEADERS are observed by `ClientConn` and discarded by `RoundTrip`). The router emits END_STREAM on the response HEADERS or final DATA, never via a trailing HEADERS frame. The fixture-0004 driver does not exercise trailers (bodyless GETs only). Decision: trailers are observed-and-discarded in both directions at phase 05.2; the codec correctness (h2spec section 8 PASS at 53/53 — the trailer-ordering tests pass because observation-not-forwarding is correct framing) is unaffected. Forward-looking: phase 07's filter-chain framework + the gRPC family land trailer forwarding (where `grpc-status` is carried in trailers and forwarding is the load-bearing benefit). Consequences: (a) the differential surface is asymmetric in principle (envoy-go discards; Envoy forwards) — bounded in 05.2 because fixture 0004 doesn't exercise trailers; the divergence is unobservable on the differential gate; (b) `BEHAVIOR_CONTRACT.md ## HTTP/2`'s "Not asserted" subsection enumerates trailers per ADR-0058; (c) M-4 carry-forward: `readClientPreface`'s ctx-unaware shape stays in 05.2 (the proper fix at the listener-manager level via uniform OS read deadlines is a phase 06/07 concern); (d) M-10 carry-forward: `SETTINGS_TIMEOUT` stays absent in 05.2 (h2spec sends SETTINGS_ACK promptly per the 05.1 REVIEW; the proper fix lands with the listener-manager's per-conn timeout policies in phase 06 or 08 — the carry-forward is tagged "phase-06-or-08-must-consider"). Lands in Task 11 (first use of `routerActionH2.do`'s observe-discard trailer rule). Supersedes nothing.

- **ADR-0057 — Closes ADR-0035 H2 leg via fixture 0004's full-stack HTTPS h2.** Status: Accepted. Date: task-execution date. Doctrine: D-3.6 (every phase is a green build — and the H2 surface is now under differential coverage). Settles: SPEC §1 #9 + §2.3 + §11.6 (carry-forward of "fixture-0003 still does not differentially exercise upstream TLS" from phase-04 REVIEW, narrowed to the H2 leg specifically); SPEC §10 #1.7 ("BEHAVIOR_CONTRACT.md flips deferred-to-05.2 entries to active per ADR-0057"). Context: ADR-0035 (phase 03 era) recorded that fixture 0002's plaintext upstream backends left the upstream-TLS code path (phase-03's `Cluster.Dial` TLS branch) under unit-test coverage only, not differential. Phase 04's REVIEW carried this forward as the H1+TLS-upstream gap; phase 05's parent SPEC anticipated 05.2 closing the H2 leg via fixture 0004. Decision: fixture 0004 has full-stack HTTPS h2 between proxy and upstream backends (subject `127.0.0.1` × 3 TLS endpoints; reference `host.docker.internal` × 3 TLS ports per ADR-0010; both with `alpn_protocols: ["h2"]`). The upstream-TLS code path (phase-03's `Cluster.Dial` TLS branch + phase-05.2's `DialH2`) is NOW under differential coverage. The H1+TLS upstream gap remains open — a future HTTPS-H1 fixture (or an extension of fixture 0003 to TLS upstream) closes the H1 leg. ADR-0057 explicitly carries forward the H1+TLS upstream gap with a "phase-05.2-follow-up" tag pointing at later phases (likely between phase 05.2 and phase 06, or folded into phase 07's filter-chain framework, or staying open into HTTP-filter-family phases — 05.2 does not pre-decide). Consequences: (a) `BEHAVIOR_CONTRACT.md ## HTTP/2`'s "Header allow-list extensions" rows for `:method`/`:path`/`:scheme`/`:authority` flip from `applies-to: phase 05.2 (forward-looking)` to `applies-to: phase 05.2 routed-to-upstream H2 (active per ADR-0057)`; (b) the H1+TLS upstream gap is now the surviving carry-forward from ADR-0035; (c) the differential coverage of fixture 0004 is the FIRST non-vacuous gate (a) on the H2 surface — gate (a) was vacuous in 05.1 per ADR-0045, and 05.2's gate (a) is non-vacuous via fixture 0004. Lands in Task 14 (first use of fixture 0004's full-stack HTTPS h2 surface). Supersedes nothing — it CLOSES (settles) the H2 leg of ADR-0035 without superseding ADR-0035 itself; the H1 leg remains open under ADR-0035.

If an unforeseen decision surfaces during execution that has cross-phase impact (per D-3.5), the executor writes a new sequential ADR (ADR-0059+) in the same commit as the code it decides for. If such a decision would expand phase-05.2 scope beyond SPEC §1–§4, invoke `superpowers:systematic-debugging` and then either re-scope the task in place or split per `BOOTSTRAP_PROMPT.md` §6 — noting that 05.2 SPEC §11.1 recommends the split-by-ADR-scope axis (05.2.1 = ADR-0055 only; 05.2.2 = rest) over the split-by-surface axis.

---

## Settled SPEC §10 deferred decisions

SPEC §10 leaves ten 05.2-scoped implementation-detail choices to the planner. This PLAN settles them so the executor does not re-litigate. Only decisions with cross-phase impact are also captured as ADRs.

1. **`H2Request`/`H2Response` type-pair on the CLIENT side.** **Decision: introduce a small `H2Request`/`H2Response` struct pair internal to `internal/filter/hcm/h2/`.** Per SPEC §10 #1's recommendation. Rationale: the request passes through the codec twice (server-side decoded → action → client-side encoded), and the type-pair makes the codec-internal shape explicit instead of overloading stdlib `*http.Request`/`*http.Response` types. The type-pair lives in `client.go` (Task 7); the action layer translates from the route-table's `*http.Request` to the codec's `H2Request` at the action-invocation boundary (Task 11; the translation is small and lives in `routerActionH2.do`'s entry block). The server-side `*http.Request` shape from 05.1 is UNCHANGED — the 05.1 server-side decision to re-use stdlib `*http.Request` for route-match stays; the 05.2 client-side decision is independent. Codified in Task 7; not separately ADRd (codec-internal type design with no cross-phase impact beyond the next pooling phase).

2. **`routerActionH2` and `routerAction` interface vs concrete switch.** **Decision: keep the codec-neutral interface shape; both `routerAction.do` and `routerActionH2.do` satisfy a common `action` interface** (already present in `actions.go` per the 05.1 codec-neutral factoring of `directResponseAction`). Per SPEC §10 #2's recommendation. Rationale: the H2 stream dispatch's `dispatch` helper invokes `action.WriteH2(ctx, req, sw)` polymorphically (the `h2.Action` interface as widened in Task 11); the action variant carries the codec choice internally. The H1 path keeps its `action.do(ctx, req, bw)` shape (Task 11 does NOT widen the H1 action interface; only `h2.Action` widens). Codified in Task 11; not separately ADRd (implementation detail).

3. **Per-cluster RR counter scope: per-`Cluster` (no change to ADR-0024 scope).** Per SPEC §10 #3's decision. Rationale: fixture 0004's cluster is H2-only; the question is dormant in 05.2's scope. If a future phase introduces a mixed-codec cluster (single cluster used by both H1 and H2 listeners), an ADR will land at that time deciding per-codec scoping. Codified in this PLAN; not separately ADRd (the question is dormant; ADR-0024's scope is unchanged).

4. **`routerActionH2.do`'s 502 local-reply emits a `Date` header.** Per SPEC §10 #4's decision. Rationale: matches the 05.1 `directResponseAction.writeH2` discipline; uses the same `dateNowRFC7231` helper (existing in `internal/filter/hcm/actions.go` from 05.1). Codified in Task 11 step 3 (the `routerActionH2.write502` helper enumerates the headers including `Date`). Not separately ADRd (mechanical correctness rule).

5. **`ClientConn` waits synchronously for SETTINGS_ACK before allowing `RoundTrip`.** Per SPEC §10 #5's recommendation. Rationale: the per-request fresh-conn discipline (ADR-0056) means we'd otherwise `RoundTrip` immediately after `NewClientConn`; doing the SETTINGS_ACK wait inside the constructor surfaces SETTINGS-handshake errors as constructor errors instead of mid-request errors. Codified in Task 7 step 4 (`NewClientConn` blocks until both SETTINGS frames are exchanged AND the peer's SETTINGS_ACK for our SETTINGS is received). Not separately ADRd (codec-internal lifecycle decision with no cross-phase impact).

6. **Skip the optional fifth `cmd/envoy-go/main_test.go` bootstrap variant.** Per SPEC §10 #6's recommendation. Rationale: fixture 0004 covers the same surface differentially; the smoke test would be redundant. Codified in this PLAN's File Structure (`cmd/envoy-go/main_test.go` UNCHANGED); not separately ADRd (test-coverage decision with no cross-phase impact).

7. **`ClientConn`'s frame-read goroutine factored separately from `ServerConn.Run`'s loop.** Per SPEC §10 #7's recommendation. Rationale: the server/client asymmetries (PUSH_PROMISE handling — server accepts trailing HEADERS as request trailers; client rejects PUSH_PROMISE per `ENABLE_PUSH=0`; stream-id allocator direction — server sees odd-from-client; client allocates odd-self; settings-application differences — peer-advertised settings the role consumes) are subtle enough that a shared `runFrameLoop(direction)` helper introduces risk of hidden differential bugs. The two loops live as separate methods on `ServerConn` and `ClientConn` respectively. Codified in Task 8; not separately ADRd (codec-internal lifecycle with no cross-phase impact).

8. **`RoundTrip` response-body buffer is NOT bounded in 05.2.** Per SPEC §10 #8's recommendation. Rationale: matches 05.1's per-stream `reqBody` discipline (also unbounded; the future bounding is a hardening-phase concern, likely tied to per-cluster `max_response_headers_kb` / future `max_response_body_bytes` settings). Codified as a known limitation in this PLAN's `## Spec-review advisory responses` section (item iii); not separately ADRd (security-hardening item with no immediate cross-phase impact; phase 06's brainstorming or a dedicated hardening phase is the right scope for the bounding decision).

9. **Concrete ADR numbers for ADR-0055..ADR-0058.** Per SPEC §10 #9's deferred decision: the planner re-verifies next-free at write time. Verified at PLAN-write time: `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0054:` at the `bc4fca4` baseline. Phase 05.2's four ADRs land at ADR-0055..ADR-0058. The mapping is documented in `## ADRs introduced by this plan` above; the executor re-verifies at Task 1 step 1 in case a mid-PLAN-authoring or pre-implementation ADR has landed since this PLAN was written.

10. **Monotonic-id-reuse integration test alongside in `conn_test.go`.** Per SPEC §10 #10's recommendation. Rationale: easy to find; same test peer; same fixture pattern as `TestServerConn_GOAWAYOnProtocolError_EvenStreamID`. Codified in Task 6 step 2 (the test lands in `internal/filter/hcm/h2/conn_test.go` as `TestServerConn_GOAWAYOnProtocolError_StreamIDReuse`).

Three additional 05.2-internal implementation choices (not in SPEC §10 but settled here so the executor doesn't re-litigate):

11. **`h2.Action` interface widening from `WriteH2(StreamWriter) error` to `WriteH2(ctx, req H2Request, sw StreamWriter) error`.** Per the File Structure entry for `h2dispatch.go` and Task 11. **Decision: extend the existing `h2.Action` interface in place** (not introduce a parallel interface). The existing `h2DirectResponseAdapter.WriteH2` is updated to accept (and ignore) the new ctx + req parameters — direct_response doesn't consume them; the new `h2RouterActionAdapter.WriteH2` consumes both. The widening is a small breaking change to the 05.1 surface but does not warrant a separate ADR — the surface is internal to `internal/filter/hcm/`, not exported, and the codec-neutral interface shape is the same one the 05.1 PLAN's File Structure entry for `actions.go` already anticipated as forward-compatible. Recorded here, not ADRd.

12. **`h2RouterActionAdapter` lives in `h2dispatch.go` (NOT in `actions.go`).** The hcm-package's `routerActionH2` carries only the action's behaviour; the codec-neutral wrapper lives in `h2dispatch.go` per the one-way-import boundary the 05.1 PLAN's "Settled SPEC §10 deferred decisions" #10 established (hcm → h2 only; h2 does not import hcm). The adapter pattern lets `h2.Action` stay codec-internal. Recorded here, not ADRd.

13. **Fixture-0004 driver fresh `Transport` per request (not just `DisableKeepAlives`).** Per SPEC §5.8 step 3 — keeping RR distribution deterministic requires that the driver's HTTP/2 client NEVER reuses an `*http2.ClientConn` across requests (otherwise the driver's Transport pool can short-circuit subsequent requests over a single conn, which would still be 27 separate streams from envoy-go's perspective but could batch differently against Envoy reference's pool). The driver constructs a fresh `*http2.Transport{TLSClientConfig: ...}` for every call to `helpers.H2RoundTrip`; the helper does NOT cache. The 05.1 fixture-0003 driver pattern of `Transport.DisableKeepAlives = true` is augmented (not replaced) by the fresh-transport-per-call discipline. Recorded here, not ADRd; codified in Task 12 (`H2RoundTrip` helper signature) + Task 14 (driver consumption).

The master phase-05 SPEC §10 also has items #6 (per-cluster RR counter scope), #7 (per-cluster RR distribution dimension), #8 (`:status`-first vs `content-type`-first ordering), #14 (cluster-side `dial_h2.go` factoring) — items #6/#7 are settled by 05.2 SPEC §10 #3 (per-`Cluster` scope retained); item #8 is a CORRECTNESS rule (RFC 9113 §8.3) and is enforced in `client.go` Task 7; item #14 is the `internal/cluster/dial_h2.go` shape, settled in Task 9.

---

## Phase-05.1 REVIEW carry-forward resolution matrix

SPEC §12 + the four new ADRs (ADR-0055..ADR-0058) triage the 13 phase-05.1 carry-forwards (8 absorbed into ADR-0055, 4 deferred to later phases per per-finding disposition, 1 integration-test gap landed as a small task in PLAN, plus an FU-7 future-tightening confirmed out-of-scope, plus 8 already-closed-in-05.1's-follow-up-batch items recorded for audit). Triage table:

| Phase-05.1 finding | Triage | Landing task / rationale |
|---|---|---|
| I-1 (`ServerConn.writeData` does not respect `SETTINGS_MAX_FRAME_SIZE`) | RESOLVED-IN-05.2 (ADR-0055) | Task 3. Outbound DATA chunking caps at `min(connWindow, streamWindow, peer.MaxFrameSize)`. Regression test: >16384-byte body chunked correctly (≥2 DATA frames; no peer-side `FRAME_SIZE_ERROR`). |
| I-2 (`ServerConn.writeData` does not respect per-stream send window) | RESOLVED-IN-05.2 (ADR-0055) | Task 3. Per-stream send-window enforcement on outgoing DATA. Regression test: `INITIAL_WINDOW_SIZE: 16` + 100-byte response body produces ~7 DATA frames + no `FLOW_CONTROL_ERROR`. |
| I-3 (Receive-side flow control allocated but never enforced) | RESOLVED-IN-05.2 (ADR-0055) | Task 4. `recvW` decrement on every inbound DATA chunk + half-window WINDOW_UPDATE emission policy. Regression test: >65 KB inbound body completes (no deadlock) with WINDOW_UPDATE frames observed on the wire. |
| I-4 (`CONFORMANCE_PINS.md` missing `## Refresh procedure`) | RESOLVED-IN-05.1-FOLLOW-UP | Already landed in the 05.1 follow-up batch per REVIEW.md `Recommendation` Path A (verify at Task 1 step 1: `grep -nE '## Refresh procedure' docs/envoy-go/CONFORMANCE_PINS.md` returns at least one match; if absent, follow precondition guidance). NOT carried to 05.2 as a code change; 05.2 only adds 2 lines under the threshold-section enumeration documenting why section 6.6 is excluded (per Task 6 step 4). |
| M-1 (`hpackBlocked` dead code) | RESOLVED-IN-05.1-FOLLOW-UP | Already landed; NOT carried to 05.2. |
| M-2 (`validateClientStreamID` dead code) | RESOLVED-IN-05.1-FOLLOW-UP | Already landed; NOT carried to 05.2. |
| M-3 (`writeData` dead `if taken <= 0` branch + `waitFor`+`reserve` non-atomicity) | RESOLVED-IN-05.2 (ADR-0055) | Task 2. `waitFor`+`reserve` collapse into `reserveBlocking`; dead branch deleted. Regression test (race-detector): concurrent multi-stream writes against window primed at boundary values produce no over-reservation. |
| M-4 (`readClientPreface` not ctx-aware) | DEFERRED to phase 06 or 07 (ADR-0058 carry-forward) | Bundled into ADR-0058's carry-forward subsection per SPEC §12.2 per-finding-disposition. The proper fix is at the listener-manager level via uniform OS read deadlines, which is a phase 06/07 concern. Phase 05.2 does NOT touch `preface.go`. |
| M-5 (`framer.readFrameCtx`/`tryReadFrame` translation block duplication) | RESOLVED-IN-05.2 (ADR-0055) | Task 2. `translateFramerErr` helper extraction; both call sites consume the helper; the new `ClientConn`'s framer wrapper consumes the same helper. |
| M-6 (fuzzer `errors.Is`) | RESOLVED-IN-05.1-FOLLOW-UP | Already landed; NOT carried to 05.2. |
| M-7 (`recvW` fields dead) | RESOLVED-IN-05.2 (ADR-0055; kept-and-consumed under I-3) | Task 4. The `recvW` allocations are no longer dead per the I-3 fix. |
| M-8 (`excludedSubsections` `//nolint:unused`-suppressed) | RESOLVED-IN-05.2 (small mechanical task) | Task 6 step 4. Promoted to a doc comment in `CONFORMANCE_PINS.md` (the threshold-section enumeration); the `//nolint`-suppressed slice is deleted. SPEC §12.2's "fold-into-PLAN-as-5-minute-task" recommendation honoured. |
| M-9 (WINDOW_UPDATE delta overflow not bounds-checked) | RESOLVED-IN-05.2 (ADR-0055) | Task 4. `serverStream.recvWindowUpdate` and `ServerConn.onWindowUpdate` add `2³¹ - 1` overflow bounds-check. Regression test: WINDOW_UPDATE delta totalling > 2³¹ - 1 → `RST_STREAM(FLOW_CONTROL_ERROR)` (stream-scoped) or `GOAWAY(FLOW_CONTROL_ERROR)` (conn-scoped). |
| M-10 (`SETTINGS_TIMEOUT` absent) | DEFERRED to phase 06 or 08 (ADR-0058 carry-forward) | Bundled into ADR-0058's carry-forward subsection per SPEC §12.2 per-finding-disposition. The proper fix lands with the listener-manager's per-conn timeout policies in phase 06 or 08. Phase 05.2 does NOT introduce a SETTINGS_TIMEOUT timer. |
| M-11 (`recvData` writes to `s.reqBody` before checking state) | RESOLVED-IN-05.2 (ADR-0055) | Task 5. One-line reorder; eliminates memory-waste path on closed streams. Regression test: DATA on closed stream does NOT grow `s.reqBody`. |
| M-12 (`closedStreams` map unbounded) | DEFERRED to long-lived-conn phase | Bundled neither into ADR-0055 nor into ADR-0058 — kept as a free-standing carry-forward in PROGRESS.md only. Per SPEC §12.2's recommendation: fixture 0004 doesn't exercise long-lived conns; the cap is a hardening-phase item. |
| M-13 (BEHAVIOR_CONTRACT prose tightening) | RESOLVED-IN-05.1-FOLLOW-UP | Already landed; NOT carried to 05.2. |
| M-14 (no-match 404 body alignment) | RESOLVED-IN-05.1-FOLLOW-UP | Already landed; NOT carried to 05.2. |
| M-15 (ADR-0046 prose correction via ADR-0054 supersession) | RESOLVED-IN-05.1-FOLLOW-UP (ADR-0054) | Already landed; NOT carried to 05.2. |
| M-16 (smoke-only docstring) | RESOLVED-IN-05.1-FOLLOW-UP | Already landed; NOT carried to 05.2. |
| M-17 (connection.go fall-through doc comment) | RESOLVED-IN-05.1-FOLLOW-UP | Already landed; NOT carried to 05.2. |
| Integration-test gap for monotonic-id-reuse rejection branch | RESOLVED-IN-05.2 (Task 6) | Task 6 step 2. `TestServerConn_GOAWAYOnProtocolError_StreamIDReuse` lands in `conn_test.go`. |
| FU-7 (elide empty trailing DATA frame) | CONFIRMED OUT-OF-SCOPE for 05.2 | Per SPEC §12.4 + §5.4. Fixture 0004's `direct_response` bodies are non-empty (`OK\n`, `not found\n`); 05.2 has no upstream-H2 alignment motive. Carries forward to a future phase if a fixture with empty `body` arises. |

The 8 RESOLVED-IN-05.1-FOLLOW-UP items confirm the 05.1 close commit (`536f353` and the follow-up batch tail). 8 RESOLVED-IN-05.2 items land via ADR-0055 + Task 6 (the integration test) + Task 6 step 4 (M-8 cleanup). 3 DEFERRED items (M-4, M-10, M-12) carry forward with documented rationale via ADR-0058 (M-4, M-10) or PROGRESS.md (M-12). No 05.1 finding rises to a 05.2 blocker; no Critical findings exist; the 05.1 REVIEW verdict was `APPROVED WITH FOLLOW-UPS` and the follow-up landed cleanly.

Additionally, the 05.1 REVIEW surfaced (as the "single most important context to surface to the phase-05.2 planner") that **the three flow-control discipline gaps (I-1/I-2/I-3) form a coherent ADR-shaped unit.** This PLAN's ADR-0055 (per Task 5) delivers exactly that unit — the SPEC honoured the REVIEW's framing in §1 #6 + §5.6, and this PLAN honours it in the task structure (Tasks 2-5 form a coherent ADR-0055 sequence; Task 5 is the closing task that lands the ADR alongside the final code fix).

---

## Spec-review advisory responses

The SPEC's brainstorming session (per ADR-0004) ran the `spec-document-reviewer` subagent loop and reached APPROVED on iteration 2 per STATE.md `last-commit` field at `dacf4b7`. The SPEC at `dacf4b7` carries no outstanding advisory items at PLAN-write time.

Three planner-time advisory items, structurally akin to the 05.1 PLAN's "spec-review advisory responses" but originating from the planner's reading of the SPEC during PLAN authoring:

i. **The `h2.Action` interface widening from `WriteH2(StreamWriter) error` to `WriteH2(ctx, req H2Request, sw StreamWriter) error`** — a small breaking change to the 05.1 surface — is documented in `## Settled SPEC §10 deferred decisions` #11 above. The change is internal to `internal/filter/hcm/`, not exported, and applies cleanly to both existing adapters (the direct-response adapter ignores ctx + req; the new router adapter consumes both). Recorded here so the executor doesn't re-litigate the interface shape at Task 11 execution time.

ii. **The `bad502Body = "bad gateway\n"` shared constant** for SPEC §11.9's mitigation — the H1 path's 502 currently uses `writeStatusReply(bw, 502, "")` (empty body), so the H2 path's `bad gateway\n` body is NEW prose, not a pre-existing wording the H2 path needs to byte-equal. The mitigation's intent (a single source for the prose) is honoured by the constant; the H1 path's 502 wording is NOT changed by 05.2 (no regression to phase-04 byte-equivalence). The shared-constant pattern future-proofs the 05.2 → upstream-robustness-phase migration where the H1 502 might also gain a non-empty body.

iii. **The `RoundTrip` response-body buffer is NOT bounded in 05.2** per `## Settled SPEC §10 deferred decisions` #8. A malicious upstream could send an unbounded response body to OOM the proxy; this is a known security limitation. Phase 06's brainstorming or a dedicated hardening phase is the right scope for the bounding decision (likely tied to per-cluster `max_response_headers_kb` / future `max_response_body_bytes` settings). Recorded here so a reviewer reading the PLAN doesn't flag it as missing.

The planner re-verified at PLAN-write time that the SPEC at `dacf4b7` does not contain any iteration-1 contradiction the reviewer flagged. The `dacf4b7` SPEC is internally consistent: §1 #6's flow-control list aligns with §5.6's prose; §10 #1's `H2Request`/`H2Response` recommendation aligns with §4.1's snippet; §10 #5's synchronous SETTINGS_ACK wait aligns with §5.3's "exchange initial SETTINGS" prose; §11.9's 502 prose-divergence risk is mitigated by the shared-constant pattern in this PLAN.

---

## Execution preconditions

Before Task 1, the executing session must:

1. Be running in a **fresh worktree on a phase-implementation branch cut off `master`**, NOT `phase/05.2-upstream-h2-plan` (this plan's authoring branch) and NOT `phase/05.2-upstream-h2-spec` (the SPEC's authoring branch). Recommended: `.worktrees/phase-05.2-upstream-h2-impl` on branch `phase/05.2-upstream-h2-impl`. STATE.md's `last-commit` at cold-start must be the commit that landed this PLAN.md on master. Per ADR-0003: branch fast-forwards into `master` at session exit.
2. Have `docker` available (verify with `docker version`). Required for fixture 0004's reference (Envoy in a testcontainer) AND for the unchanged 05.1 conformance gate's re-run during Task 15's local sweep.
3. Have Go 1.23+ installed (verify with `go version`). Native fuzzing (`testing.F`) requires Go 1.18+; 1.23 is the module floor.
4. Have `golangci-lint` installed at the ADR-0009-pinned version v1.64.8 (verify with `golangci-lint version`).
5. `go test ./...` must be green on `master` at cold-start — this plan assumes a clean baseline (phase-05.1 gate (e) still holds at `bc4fca4`'s tail). If not, invoke `superpowers:systematic-debugging` on the regression *before* starting Task 1.
6. `go list -m github.com/envoyproxy/go-control-plane/envoy` resolves to `v1.32.4` (ADR-0013). If a different version is recorded, invoke `superpowers:systematic-debugging` — phase 05.2 must not silently re-pin.
7. `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0054:` (or later if a mid-phase ADR has landed since this PLAN was written). If the tail is `ADR-0054`, the phase-05.2 ADRs are assigned 0055..0058 as in this PLAN. If higher, re-number phase-05.2 ADRs sequentially from `tail + 1` and update every task's ADR reference *before* starting Task 1.
8. The phase-05.2 SPEC at `docs/envoy-go/phases/05.2-upstream-h2/SPEC.md` is at commit `dacf4b7` (verify with `git log -1 --format=%H -- docs/envoy-go/phases/05.2-upstream-h2/SPEC.md`). If the SPEC has been amended since `dacf4b7`, invoke `superpowers:systematic-debugging` on the divergence — the PLAN was authored against `dacf4b7` and silent SPEC drift voids the PLAN's traceability.
9. Phase-05.1 close + follow-up batch is present in HEAD: `git log --oneline -- docs/envoy-go/phases/05.1-downstream-h2/REVIEW.md` shows the close commit; `grep -nE '## Refresh procedure' docs/envoy-go/CONFORMANCE_PINS.md` returns ≥ 1 match (per the I-4 follow-up close). If either is missing, invoke `superpowers:systematic-debugging` on the gap.
10. `go list -m golang.org/x/net` resolves to a version at-or-above the one consumed by 05.1 (transitively via go-control-plane; promoted to direct in 05.1 Task 4). The 05.2 PLAN does not bump this — verify the existing pin is intact.
11. The 05.1 `internal/filter/hcm/h2/` sub-package is intact: `ls internal/filter/hcm/h2/` returns `conn.go`, `conn_test.go`, `doc.go`, `errors.go`, `errors_test.go`, `flow.go`, `flow_test.go`, `framer.go`, `framer_test.go`, `fuzz_test.go`, `hpack.go`, `hpack_test.go`, `preface.go`, `preface_test.go`, `settings.go`, `settings_test.go`, `stream.go`, `stream_test.go` (18 files; NO `client.go` per ADR-0048's reservation — Task 7 lands it). If `client.go` is already present, invoke `superpowers:systematic-debugging` — 05.2's surface is being introduced incrementally and a pre-existing `client.go` voids the PLAN's traceability.
12. The 05.1 `BEHAVIOR_CONTRACT.md ## HTTP/2` SCAFFOLD subsection is intact at lines ~267-315: `grep -nE "^## HTTP/2$|^### Asserted equivalence \(05\.1 scope\)$|^### Does not yet apply to$" docs/envoy-go/BEHAVIOR_CONTRACT.md` returns three line-number matches in the expected order. If the SCAFFOLD has been edited since 05.1 close (other than the I-4-related M-13 prose tightening already absorbed in the 05.1 follow-up batch), invoke `superpowers:systematic-debugging` — the in-place edit at Task 15 must start from the 05.1 SCAFFOLD shape.

If any precondition fails: invoke `superpowers:systematic-debugging` on the missing dependency. Do not improvise an install path or skip a failing test.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md`

No code change. This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase/05.2-upstream-h2-impl
git log -1 --format=%H                                                # expect: same SHA as docs/envoy-go/STATE.md last-commit field (the PLAN.md commit)
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: golangci-lint has version 1.64.8
go test ./...                                                         # expect: every package PASS (no FAIL, no compile error)
go list -m github.com/envoyproxy/go-control-plane/envoy               # expect: v1.32.4
grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1                  # expect: ## ADR-0054:
git log -1 --format=%H -- docs/envoy-go/phases/05.2-upstream-h2/SPEC.md
                                                                       # expect: dacf4b7 (or the documented SPEC commit; if newer, follow precondition 8 guidance)
git log --oneline -- docs/envoy-go/phases/05.1-downstream-h2/REVIEW.md | head -5
                                                                       # expect: at least one commit visible (the 05.1 REVIEW close)
grep -nE '## Refresh procedure' docs/envoy-go/CONFORMANCE_PINS.md     # expect: ≥ 1 match (the 05.1 follow-up close of I-4)
go list -m golang.org/x/net                                           # expect: a resolvable version (already promoted to direct in 05.1)
ls internal/filter/hcm/h2/client.go                                   # expect: ENOENT (client.go is THIS phase's deliverable, Task 7)
grep -cE "^## HTTP/2$" docs/envoy-go/BEHAVIOR_CONTRACT.md             # expect: 1 (the 05.1 SCAFFOLD subsection)
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md`**

```markdown
# Phase 05.2 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all preconditions were satisfied at cold-start>

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** <sha — this task's commit>
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions"; phase-05.1 close + follow-up batch confirmed present in HEAD; SPEC at dacf4b7; ADR tail at 0054 (next-free 0055); client.go absent (will land at Task 7).
**Outputs:**
\`\`\`
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
$ git log -1 --format=%H -- docs/envoy-go/phases/05.2-upstream-h2/SPEC.md
<verbatim>
$ grep -nE '## Refresh procedure' docs/envoy-go/CONFORMANCE_PINS.md
<verbatim>
$ ls internal/filter/hcm/h2/client.go
<verbatim — should report 'No such file or directory'>
\`\`\`
```

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: PROGRESS.md preamble + precondition verification"
```

After the commit, update the just-written PROGRESS.md entry's `**Commits:**` line with the short SHA of the commit (phase-02/03/04/05.1 precedent: a follow-up tiny commit `phase 05.2: PROGRESS SHA-fill for Task 1` lands the SHA).

---

## Task 2: ADR-0055 prerequisites — `window.reserveBlocking` collapse (M-3) + `translateFramerErr` helper extraction (M-5)

**Files:**
- Modify: `internal/filter/hcm/h2/flow.go`
- Modify: `internal/filter/hcm/h2/flow_test.go`
- Modify: `internal/filter/hcm/h2/framer.go`
- Modify: `internal/filter/hcm/h2/framer_test.go`
- Modify: `internal/filter/hcm/h2/conn.go` (only the `writeData` consumer site is updated to call `reserveBlocking`; the deeper conn.go changes for I-1/I-2 land in Task 3)
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 2 entry)

Mechanical prerequisites for the ADR-0055 sequence. The `reserveBlocking` collapse (M-3) is required for I-1/I-2 (Task 3) to be race-free. The `translateFramerErr` helper extraction (M-5) is required so `client.go` (Task 7) can consume the same translation block as the server-side `framer.readFrameCtx`/`tryReadFrame`. No ADR yet — ADR-0055 lands at Task 5 with the closing fix.

- [ ] **Step 1: Write the failing test for `reserveBlocking` (in `flow_test.go`)**

```go
// TestWindow_ReserveBlocking_AtomicityUnderConcurrency verifies that
// reserveBlocking holds the mutex across the wait+take so concurrent
// reservers never over-allocate the window.
func TestWindow_ReserveBlocking_AtomicityUnderConcurrency(t *testing.T) {
	w := newWindow(100)
	ctx := context.Background()
	var taken atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := w.reserveBlocking(ctx, 50)
			if err != nil {
				t.Errorf("reserveBlocking: %v", err)
				return
			}
			taken.Add(int64(n))
		}()
	}
	// Release more capacity gradually.
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(time.Millisecond)
			w.replenish(100)
		}
	}()
	wg.Wait()
	// All 20 goroutines together asked for 1000 (20 × 50);
	// initial 100 + replenish 1000 = 1100 capacity. Each reserveBlocking
	// call returns 1..50. Total taken must equal sum(returned values),
	// and must NOT exceed 1100.
	if taken.Load() > 1100 {
		t.Errorf("over-reservation: taken=%d, max=1100", taken.Load())
	}
}
```

Run: `go test -race ./internal/filter/hcm/h2/ -run TestWindow_ReserveBlocking_AtomicityUnderConcurrency -v`
Expected: FAIL — `reserveBlocking` not defined yet.

- [ ] **Step 2: Implement `reserveBlocking` in `flow.go`; deprecate the public `waitFor` + `reserve` pair**

```go
// reserveBlocking reserves up to max units from the window, blocking until
// at least 1 unit is available or ctx is cancelled. The mutex is held across
// the wait + take so concurrent reservers never over-allocate.
//
// Returns the actually-reserved amount (1..max) and nil on success;
// returns 0 + ctx.Err() if the context is cancelled before any reservation.
//
// Replaces the (waitFor + reserve) pair from 05.1 per ADR-0055 / M-3.
func (w *window) reserveBlocking(ctx context.Context, max int32) (int32, error) {
	w.mu.Lock()
	for w.n <= 0 {
		w.mu.Unlock()
		select {
		case <-w.ch:
			w.mu.Lock()
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	taken := max
	if taken > w.n {
		taken = w.n
	}
	w.n -= taken
	w.mu.Unlock()
	return taken, nil
}
```

Delete the existing `waitFor` and `reserve` exported methods on `*window`; the only consumer was `ServerConn.writeData` and Step 4 below replaces the call site.

Run: `go test -race ./internal/filter/hcm/h2/ -run TestWindow_ -v`
Expected: PASS for the new test; existing `flow_test.go` cases that called `waitFor`/`reserve` directly are updated in this same step to call `reserveBlocking` (the cases that asserted blocking semantics + ctx-cancel + multi-consumer remain valid; only the API surface they call changes).

- [ ] **Step 3: Update `ServerConn.writeData` call site in `conn.go`**

The existing `writeData` body has a `taken := s.sendW.reserve(int32(len(remaining)))` call followed by an `if taken <= 0` recovery branch (the dead branch per M-3). Replace with:

```go
taken, err := s.sendW.reserveBlocking(ctx, int32(len(remaining)))
if err != nil {
	return err  // ctx error
}
// (the dead branch is GONE)
```

The conn.go change is small (~5 LoC delta); the larger I-1/I-2 changes land in Task 3 and use this same `reserveBlocking` plumbing.

Run: `go test -race ./internal/filter/hcm/h2/ -v`
Expected: every existing test PASSES (no behavioural regression from the `reserveBlocking` collapse; the dead branch was unreachable, so deleting it doesn't change any test's expected output).

- [ ] **Step 4: Write the failing test for `translateFramerErr` (in `framer_test.go`)**

```go
// TestTranslateFramerErr verifies the framer-error translation helper covers
// each branch of the duplicated logic from 05.1's readFrameCtx/tryReadFrame.
func TestTranslateFramerErr(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want ErrCode
	}{
		{"nil", nil, 0},
		{"connection-error", http2.ConnectionError(http2.ErrCodeProtocol), ErrProtocolError},
		{"stream-error", http2.StreamError{StreamID: 5, Code: http2.ErrCodeFlowControl}, ErrFlowControlError},
		{"frame-too-large", http2.ErrFrameTooLarge, ErrFrameSizeError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := translateFramerErr(tc.in)
			if tc.in == nil {
				if got != nil {
					t.Errorf("translateFramerErr(nil) = %v, want nil", got)
				}
				return
			}
			var e *Error
			if !errors.As(got, &e) {
				t.Fatalf("translateFramerErr(%v) = %v (not an *Error)", tc.in, got)
			}
			if e.Code != tc.want {
				t.Errorf("translateFramerErr(%v) code = %v, want %v", tc.in, e.Code, tc.want)
			}
		})
	}
}
```

Run: `go test ./internal/filter/hcm/h2/ -run TestTranslateFramerErr -v`
Expected: FAIL — `translateFramerErr` not defined yet.

- [ ] **Step 5: Extract `translateFramerErr` in `framer.go`; consume from both call sites**

```go
// translateFramerErr maps http2 framer-layer errors to *Error per the M-5
// helper-extraction recommendation from the 05.1 REVIEW. Both readFrameCtx
// and tryReadFrame consume this helper; client.go's framer wrapper consumes
// it identically per ADR-0055.
//
// Returns nil for nil input; passes through non-translateable errors.
func translateFramerErr(err error) error {
	if err == nil {
		return nil
	}
	var connErr http2.ConnectionError
	if errors.As(err, &connErr) {
		return connError(ErrCode(connErr), fmt.Sprintf("framer: connection-error code=%d", connErr))
	}
	var streamErr http2.StreamError
	if errors.As(err, &streamErr) {
		return streamError(ErrCode(streamErr.Code), streamErr.StreamID, fmt.Sprintf("framer: stream-error code=%d", streamErr.Code))
	}
	if errors.Is(err, http2.ErrFrameTooLarge) {
		return connError(ErrFrameSizeError, "framer: frame too large")
	}
	return err
}
```

Update `readFrameCtx` and `tryReadFrame` to call `translateFramerErr(err)` at the call sites that previously inlined the translation.

Run: `go test ./internal/filter/hcm/h2/ -run TestFramer -v`
Expected: PASS — the existing framer tests cover the same translation paths that the helper now centralises.

- [ ] **Step 6: Run full package tests + race-detector**

```bash
go test -race ./internal/filter/hcm/h2/ -v
golangci-lint run ./internal/filter/hcm/h2/
```

Expected: green; no new lint warnings.

- [ ] **Step 7: Append Task 2 entry to PROGRESS.md, commit**

PROGRESS.md entry shape per phase-05.1 precedent: task title, commits SHA placeholder, files changed, output of `go test -race ./internal/filter/hcm/h2/` last-30-lines, output of `golangci-lint run ./internal/filter/hcm/h2/`.

```bash
git add internal/filter/hcm/h2/flow.go internal/filter/hcm/h2/flow_test.go internal/filter/hcm/h2/framer.go internal/filter/hcm/h2/framer_test.go internal/filter/hcm/h2/conn.go docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: ADR-0055 prereqs — window.reserveBlocking (M-3) + translateFramerErr (M-5)"
```

Follow up with a SHA-fill commit per the 05.1 precedent.

---

## Task 3: ADR-0055 — outbound DATA chunking (`MaxFrameSize` + per-stream send-window) [I-1, I-2]

**Files:**
- Modify: `internal/filter/hcm/h2/conn.go` (the `writeData` body)
- Modify: `internal/filter/hcm/h2/conn_test.go` (regression tests for I-1, I-2)
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 3 entry)

Bundle I-1 (peer `MaxFrameSize` chunking) and I-2 (per-stream send-window enforcement) into one task — they touch the same `writeData` inner loop and share the regression-test fixture pattern (an in-process h2 client peer that exercises the boundary). No ADR yet — ADR-0055 lands at Task 5.

- [ ] **Step 1: Write the failing test for I-1 (peer `MaxFrameSize` cap)**

```go
// TestServerConn_WriteData_RespectsMaxFrameSize verifies that outbound DATA
// chunks never exceed peer.MaxFrameSize even when the conn-level send
// window allows larger frames. Per ADR-0055 / I-1 from the 05.1 REVIEW.
func TestServerConn_WriteData_RespectsMaxFrameSize(t *testing.T) {
	// Set up: in-process peer advertising MaxFrameSize=16384 (the RFC 9113
	// default). Send a 32768-byte response body; expect ≥2 DATA frames.
	const bodyLen = 32768
	const peerMaxFrame = 16384
	// ... fixture setup mirroring the 05.1 conn_test.go pattern ...
	// Exchange preface + SETTINGS where peer advertises MaxFrameSize=16384.
	// Run a synthetic dispatch that calls (sc).writeData(streamID, body, true).
	// Read frames from the peer side; collect DATA frames; assert ≥2 frames
	// and each frame's payload length ≤ peerMaxFrame.
	// ...
}
```

Run: `go test ./internal/filter/hcm/h2/ -run TestServerConn_WriteData_RespectsMaxFrameSize -v`
Expected: FAIL — current `writeData` writes the body in a single 32768-byte DATA frame because the conn-level window (default 65535) allows it.

- [ ] **Step 2: Write the failing test for I-2 (per-stream send-window cap)**

```go
// TestServerConn_WriteData_RespectsPerStreamSendWindow verifies that
// outbound DATA never over-feeds a small per-stream window. Per ADR-0055 / I-2
// from the 05.1 REVIEW.
func TestServerConn_WriteData_RespectsPerStreamSendWindow(t *testing.T) {
	// Set up: in-process peer advertising INITIAL_WINDOW_SIZE=16; conn-level
	// window stays at 65535. Send a 100-byte response body; the peer drips
	// WINDOW_UPDATE(streamID, 16) frames every 5ms.
	// Expect ~7 DATA frames before completion (100 / 16 ≈ 6.25, so 7 frames),
	// no FLOW_CONTROL_ERROR from the peer side, no oversized frame.
	// ...
}
```

Run: `go test ./internal/filter/hcm/h2/ -run TestServerConn_WriteData_RespectsPerStreamSendWindow -v`
Expected: FAIL — current `writeData` uses only the conn-level window; the per-stream `serverStream.sendW` is replenished but never reserved against.

- [ ] **Step 3: Modify `ServerConn.writeData` to apply the I-1 + I-2 caps**

```go
// writeData chunks b into one or more DATA frames respecting:
//  - connection-level send window (s.sendW.reserveBlocking)
//  - per-stream send window (ss.sendW.reserveBlocking)
//  - peer-advertised MaxFrameSize (s.clientS.MaxFrameSize)
// Per ADR-0055 / I-1 + I-2.
func (s *ServerConn) writeData(streamID uint32, b []byte, endStream bool) error {
	ss, ok := s.lookupStream(streamID)
	if !ok {
		return streamError(ErrStreamClosed, streamID, "writeData: unknown stream")
	}
	remaining := b
	for len(remaining) > 0 {
		want := int32(len(remaining))
		// Cap by peer's MaxFrameSize (default 16384 per RFC 9113 §6.5.2).
		maxFrame := int32(s.clientS.MaxFrameSize)
		if maxFrame == 0 {
			maxFrame = 16384
		}
		if want > maxFrame {
			want = maxFrame
		}
		// Reserve against per-stream send window FIRST (smaller bound first
		// reduces head-of-line blocking against other streams on this conn).
		streamTaken, err := ss.sendW.reserveBlocking(s.ctx, want)
		if err != nil {
			return err
		}
		// Then reserve against conn-level send window.
		connTaken, err := s.sendW.reserveBlocking(s.ctx, streamTaken)
		if err != nil {
			// Roll back the stream-level reservation (replenish what we took
			// but won't use). This is correct because no DATA has been written
			// yet; the stream's flow window state is consistent.
			ss.sendW.replenish(streamTaken)
			return err
		}
		// connTaken is the actual chunk size; if connTaken < streamTaken,
		// roll back the stream-level over-reservation.
		if connTaken < streamTaken {
			ss.sendW.replenish(streamTaken - connTaken)
		}
		chunk := remaining[:connTaken]
		remaining = remaining[connTaken:]
		end := endStream && len(remaining) == 0
		s.mu.Lock()
		err = s.fr.WriteData(streamID, end, chunk)
		s.mu.Unlock()
		if err != nil {
			return translateFramerErr(err)
		}
	}
	return nil
}
```

Note: the snippet above pseudocode-references `s.lookupStream`, `s.ctx`, `s.clientS` — verify these symbols exist or rename to whatever the 05.1 conn.go currently uses (`s.streams[id]`, `s.ctx` is fine; `s.clientS` is the peer-settings struct from `settings.go`'s `clientSettings`). Adapt the snippet to match the on-disk symbols at execution time.

Run: `go test ./internal/filter/hcm/h2/ -run TestServerConn_WriteData_RespectsMaxFrameSize -v`
Expected: PASS.

Run: `go test ./internal/filter/hcm/h2/ -run TestServerConn_WriteData_RespectsPerStreamSendWindow -v`
Expected: PASS.

- [ ] **Step 4: Run the full h2 package tests + race-detector**

```bash
go test -race ./internal/filter/hcm/h2/ -v
```

Expected: every existing test PASSES + the two new I-1/I-2 tests PASS. The 05.1 tests (which used small bodies that fit in one frame regardless) are unaffected.

- [ ] **Step 5: Run h2spec gate to confirm no regression on the conformance surface**

```bash
go test ./test/conformance/h2spec/ -v
```

Expected: 53/53 PASS. The flow-control tightening must NOT regress the ADR-0051 baseline.

- [ ] **Step 6: Append Task 3 entry to PROGRESS.md, commit**

```bash
git add internal/filter/hcm/h2/conn.go internal/filter/hcm/h2/conn_test.go docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: ADR-0055 outbound chunking — MaxFrameSize cap (I-1) + per-stream send-window (I-2)"
```

---

## Task 4: ADR-0055 — receive-side flow control + WINDOW_UPDATE emission + delta-overflow bounds-check [I-3, M-9]

**Files:**
- Modify: `internal/filter/hcm/h2/conn.go` (`onData`, `onWindowUpdate`)
- Modify: `internal/filter/hcm/h2/stream.go` (`recvWindowUpdate`)
- Modify: `internal/filter/hcm/h2/conn_test.go`
- Modify: `internal/filter/hcm/h2/stream_test.go`
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 4 entry)

I-3 lands the receive-side enforcement (`recvW` decrement + half-window WINDOW_UPDATE emission). M-9 lands alongside because both touch `onWindowUpdate` and `recvWindowUpdate` and share the regression-test fixture pattern.

- [ ] **Step 1: Write the failing test for I-3 (>65 KB inbound body completes)**

```go
// TestServerConn_ReceiveSide_FlowControl_LargeInboundBody verifies that
// inbound DATA exceeding the initial window completes via WINDOW_UPDATE
// emission. Per ADR-0055 / I-3 from the 05.1 REVIEW.
func TestServerConn_ReceiveSide_FlowControl_LargeInboundBody(t *testing.T) {
	// Drive an in-process peer that sends a 100KB request body in
	// 8KB DATA frames. Without I-3, the server never replenishes the
	// receive window after 65535 cumulative bytes, deadlocking.
	// With I-3, the server emits WINDOW_UPDATE on a half-window threshold;
	// the peer can keep sending; the request body completes; assertion: the
	// dispatched action sees the full 100KB body bytes.
	// ...
}
```

Run: `go test ./internal/filter/hcm/h2/ -run TestServerConn_ReceiveSide_FlowControl_LargeInboundBody -timeout 10s -v`
Expected: FAIL (timeout or hung) — current code never replenishes recv window.

- [ ] **Step 2: Write the failing test for M-9 (delta overflow → FLOW_CONTROL_ERROR)**

```go
// TestServerConn_WindowUpdate_OverflowBoundsCheck verifies that a sequence
// of WINDOW_UPDATE frames totalling > 2³¹ - 1 triggers GOAWAY(FLOW_CONTROL_ERROR)
// at the conn level and RST_STREAM(FLOW_CONTROL_ERROR) at the stream level.
// Per ADR-0055 / M-9 from the 05.1 REVIEW.
func TestServerConn_WindowUpdate_OverflowBoundsCheck(t *testing.T) {
	// Open a stream; send WINDOW_UPDATE(streamID, math.MaxInt32) after the
	// stream's initial window of 65535 is at full. Expect RST_STREAM(FLOW_CONTROL_ERROR).
	// ...
	// Separately: send WINDOW_UPDATE(0, math.MaxInt32) at the conn level
	// after the conn's initial window of 65535 is at full. Expect GOAWAY(FLOW_CONTROL_ERROR).
	// ...
}
```

Run: `go test ./internal/filter/hcm/h2/ -run TestServerConn_WindowUpdate_OverflowBoundsCheck -v`
Expected: FAIL — current code wraps silently to a negative `int32`.

- [ ] **Step 3: Implement `onData` recv-window decrement + half-window WINDOW_UPDATE emission**

```go
// onData handles an inbound DATA frame. Per ADR-0055 / I-3:
//  - decrement s.recvW (conn-level) and ss.recvW (per-stream) by len(data)
//  - emit WriteWindowUpdate(0, n) and WriteWindowUpdate(streamID, n) once
//    cumulative debit on either window crosses the half-window high-water
//    threshold (default 32768 = 65535/2 rounded down)
func (s *ServerConn) onData(f *http2.DataFrame) error {
	streamID := f.StreamID
	ss, ok := s.lookupStream(streamID)
	if !ok {
		return streamError(ErrStreamClosed, streamID, "onData: unknown stream")
	}
	data := f.Data()
	if err := ss.recvData(data, f.StreamEnded()); err != nil {
		return err
	}
	// Debit recv windows.
	const halfWindowThreshold = 32768
	dataLen := int32(len(data))
	s.recvW.replenish(-dataLen)  // negative replenish == debit
	ss.recvW.replenish(-dataLen)
	if s.recvDebitSinceLastUpdate += dataLen; s.recvDebitSinceLastUpdate >= halfWindowThreshold {
		// Emit conn-level WINDOW_UPDATE.
		s.mu.Lock()
		err := s.fr.WriteWindowUpdate(0, uint32(s.recvDebitSinceLastUpdate))
		s.mu.Unlock()
		if err != nil {
			return translateFramerErr(err)
		}
		s.recvW.replenish(s.recvDebitSinceLastUpdate)
		s.recvDebitSinceLastUpdate = 0
	}
	// Same logic per-stream on ss.recvDebitSinceLastUpdate.
	if ss.recvDebitSinceLastUpdate += dataLen; ss.recvDebitSinceLastUpdate >= halfWindowThreshold {
		s.mu.Lock()
		err := s.fr.WriteWindowUpdate(streamID, uint32(ss.recvDebitSinceLastUpdate))
		s.mu.Unlock()
		if err != nil {
			return translateFramerErr(err)
		}
		ss.recvW.replenish(ss.recvDebitSinceLastUpdate)
		ss.recvDebitSinceLastUpdate = 0
	}
	return nil
}
```

Add `recvDebitSinceLastUpdate int32` field to `ServerConn` and `serverStream` (the running counter; reset on emit).

Run: `go test ./internal/filter/hcm/h2/ -run TestServerConn_ReceiveSide_FlowControl_LargeInboundBody -v`
Expected: PASS.

- [ ] **Step 4: Implement M-9 overflow bounds-check in `onWindowUpdate` and `recvWindowUpdate`**

```go
// onWindowUpdate handles an inbound WINDOW_UPDATE at the conn level (streamID==0)
// or stream level. Per ADR-0055 / M-9: the addition is bounds-checked against
// math.MaxInt32; on overflow, emit GOAWAY(FLOW_CONTROL_ERROR) at the conn level
// or RST_STREAM(FLOW_CONTROL_ERROR) at the stream level.
func (s *ServerConn) onWindowUpdate(f *http2.WindowUpdateFrame) error {
	delta := int32(f.Increment)
	if delta == 0 {
		return connError(ErrProtocolError, "WINDOW_UPDATE: zero increment")
	}
	streamID := f.StreamID
	if streamID == 0 {
		newVal, ok := safeAddInt32(s.sendW.available(), delta)
		if !ok {
			s.emitGoaway(ErrFlowControlError)
			return connError(ErrFlowControlError, "WINDOW_UPDATE: conn-level window overflow")
		}
		_ = newVal
		s.sendW.replenish(delta)
		return nil
	}
	ss, ok := s.lookupStream(streamID)
	if !ok {
		return nil  // stream gone; ignore
	}
	return ss.recvWindowUpdate(delta)
}

// safeAddInt32 returns (a+b, true) if a+b fits in int32; (_, false) on overflow.
func safeAddInt32(a, b int32) (int32, bool) {
	c := int64(a) + int64(b)
	if c > math.MaxInt32 || c < math.MinInt32 {
		return 0, false
	}
	return int32(c), true
}
```

Symmetric `serverStream.recvWindowUpdate` extension lands in `stream.go`:

```go
func (s *serverStream) recvWindowUpdate(delta int32) error {
	if delta == 0 {
		return streamError(ErrProtocolError, s.id, "WINDOW_UPDATE: zero increment")
	}
	if _, ok := safeAddInt32(s.sendW.available(), delta); !ok {
		return streamError(ErrFlowControlError, s.id, "WINDOW_UPDATE: stream-level window overflow")
	}
	s.sendW.replenish(delta)
	return nil
}
```

Run: `go test ./internal/filter/hcm/h2/ -run TestServerConn_WindowUpdate_OverflowBoundsCheck -v`
Expected: PASS.

- [ ] **Step 5: Run full h2 package tests + race-detector + h2spec**

```bash
go test -race ./internal/filter/hcm/h2/ -v
go test ./test/conformance/h2spec/ -v
```

Expected: green; h2spec 53/53 PASS (no regression on the conformance surface).

- [ ] **Step 6: Append Task 4 entry to PROGRESS.md, commit**

```bash
git add internal/filter/hcm/h2/conn.go internal/filter/hcm/h2/stream.go internal/filter/hcm/h2/conn_test.go internal/filter/hcm/h2/stream_test.go docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: ADR-0055 receive-side — recvW enforcement (I-3) + delta-overflow bounds (M-9)"
```

---

## Task 5: ADR-0055 — `recvData` state-before-append (M-11) + ADR-0055 lands in DECISIONS.md

**Files:**
- Modify: `internal/filter/hcm/h2/stream.go`
- Modify: `internal/filter/hcm/h2/stream_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0055)
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 5 entry)

The closing task of the ADR-0055 sequence. M-11 is a one-line reorder; ADR-0055 lands in DECISIONS.md alongside the final code fix per the first-use-commit-ordering discipline.

- [ ] **Step 1: Write the failing test for M-11**

```go
// TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream verifies that
// DATA arriving on a closed/half-closed stream does NOT append to s.reqBody.
// Per ADR-0055 / M-11 from the 05.1 REVIEW.
func TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream(t *testing.T) {
	ss := newServerStream(1, &fakeStreamConn{}, 65535, 65535)
	ss.transition(streamHalfClosedRemote)  // peer already sent END_STREAM
	preLen := ss.reqBody.Len()
	err := ss.recvData([]byte("late data"), false)
	if err == nil {
		t.Errorf("recvData on half-closed-remote stream should error")
	}
	if ss.reqBody.Len() != preLen {
		t.Errorf("reqBody grew on closed stream: pre=%d post=%d (M-11 regression)", preLen, ss.reqBody.Len())
	}
}
```

Run: `go test ./internal/filter/hcm/h2/ -run TestServerStream_RecvData_DoesNotGrowReqBodyOnClosedStream -v`
Expected: FAIL — current `recvData` appends BEFORE the state check.

- [ ] **Step 2: Reorder `recvData` to check state first**

In `stream.go`'s `recvData`, move the state check (`if s.state != streamOpen && s.state != streamHalfClosedLocal`) BEFORE the `s.reqBody.Write(b)` call. Return the stream error first; do not append on a closed stream.

Run: `go test ./internal/filter/hcm/h2/ -run TestServerStream_RecvData -v`
Expected: PASS for the new test + every existing recvData test.

- [ ] **Step 3: Run full h2 package tests + race-detector + h2spec**

```bash
go test -race ./internal/filter/hcm/h2/ -v
go test ./test/conformance/h2spec/ -v
```

Expected: green; h2spec 53/53 PASS.

- [ ] **Step 4: Append ADR-0055 to `docs/envoy-go/DECISIONS.md`**

ADR-0055 prose per `## ADRs introduced by this plan` summary above. Land at the file tail (append-only per D-3.5). Cross-reference the seven fixes by file:line where they landed (Task 2's `flow.go` for `reserveBlocking`; `framer.go` for `translateFramerErr`; Task 3's `conn.go writeData` for I-1/I-2; Task 4's `conn.go onData` for I-3 + `conn.go onWindowUpdate` / `stream.go recvWindowUpdate` for M-9; this task's `stream.go recvData` for M-11). The ADR enumerates the seven fixes individually so a future supersession can target precisely.

- [ ] **Step 5: Run lint + vet on the closing patch**

```bash
go vet ./internal/filter/hcm/h2/
golangci-lint run ./internal/filter/hcm/h2/
```

Expected: green.

- [ ] **Step 6: Append Task 5 entry to PROGRESS.md, commit**

```bash
git add internal/filter/hcm/h2/stream.go internal/filter/hcm/h2/stream_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: ADR-0055 lands — recvData state-before-append (M-11) + ADR enumerated"
```

---

## Task 6: 05.1 REVIEW carry-forward — monotonic-id-reuse integration test + M-8 cleanup

**Files:**
- Modify: `internal/filter/hcm/h2/conn_test.go` (add `TestServerConn_GOAWAYOnProtocolError_StreamIDReuse`)
- Modify: `test/conformance/h2spec/h2spec.go` (delete the `excludedSubsections` `//nolint:unused` slice)
- Modify: `docs/envoy-go/CONFORMANCE_PINS.md` (add 2-line note documenting 6.6 exclusion)
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 6 entry)

Closes the two 05.1-REVIEW carry-forwards that don't fit ADR-0055: the monotonic-id-reuse integration test gap (per SPEC §12.3 / §10 #10) and the M-8 cleanup (per SPEC §12.2's "fold-into-PLAN-as-5-minute-task" recommendation).

- [ ] **Step 1: Write the failing test for monotonic-id-reuse rejection**

```go
// TestServerConn_GOAWAYOnProtocolError_StreamIDReuse verifies that a peer
// re-using a previously-completed stream id triggers GOAWAY(PROTOCOL_ERROR).
// This is the integration coverage for the rejection branch at conn.go:308-319
// previously only unit-tested via the even-id branch.
// Per 05.1 REVIEW carry-forward + SPEC §12.3.
func TestServerConn_GOAWAYOnProtocolError_StreamIDReuse(t *testing.T) {
	// Drive an in-process peer that:
	//  1. Opens stream id 1, sends HEADERS+END_STREAM, reads response.
	//  2. After stream 1 completes, sends another HEADERS frame on stream id 1.
	// Assert: the server emits GOAWAY(PROTOCOL_ERROR) and closes the conn.
	// ...
}
```

Run: `go test ./internal/filter/hcm/h2/ -run TestServerConn_GOAWAYOnProtocolError_StreamIDReuse -v`
Expected: PASS — the rejection branch already exists in 05.1 production code; this task only adds the integration coverage that was missing per the 05.1 REVIEW.

(Verify the test actually exercises the production path: read the assertion order — if the test passes too easily, ensure the expected GOAWAY frame is observed via the framer, not just inferred from the conn close.)

- [ ] **Step 2: Delete the M-8 `excludedSubsections` slice**

In `test/conformance/h2spec/h2spec.go`, locate the `//nolint:unused`-suppressed `excludedSubsections []string{"http2/6/6"}` slice. Delete it. The exclusion rationale (push disabled per ADR-0047 / SPEC §2.1) is documented in `CONFORMANCE_PINS.md`'s threshold-section enumeration (Step 3 below).

- [ ] **Step 3: Add 2-line note to `docs/envoy-go/CONFORMANCE_PINS.md`**

Under the threshold-section enumeration, add:

```markdown
Section 6.6 (PUSH_PROMISE) is excluded because phase 05.1 disables server push per ADR-0047 / 05.1 SPEC §2.1; the section's tests are conformance-irrelevant for this surface. Per ADR-0055 (phase 05.2), the exclusion stays — the flow-control discipline tightening does not change the server-push posture.
```

- [ ] **Step 4: Run h2spec gate to confirm no regression**

```bash
go test ./test/conformance/h2spec/ -v
```

Expected: 53/53 PASS at the ADR-0051 pin.

- [ ] **Step 5: Run full h2 package tests + race-detector**

```bash
go test -race ./internal/filter/hcm/h2/ -v
golangci-lint run ./internal/filter/hcm/h2/ ./test/conformance/h2spec/
```

Expected: green; the M-8 `//nolint:unused` is gone, no other lint warnings.

- [ ] **Step 6: Append Task 6 entry to PROGRESS.md, commit**

```bash
git add internal/filter/hcm/h2/conn_test.go test/conformance/h2spec/h2spec.go docs/envoy-go/CONFORMANCE_PINS.md docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: 05.1-REVIEW carry-forwards — monotonic-id-reuse test + M-8 cleanup"
```

---

## Task 7: H2 client-side codec — `client.go` skeleton (`H2Request`/`H2Response`, `ClientConn`, preface + SETTINGS exchange, `Close`)

**Files:**
- Create: `internal/filter/hcm/h2/client.go`
- Create: `internal/filter/hcm/h2/client_test.go`
- Modify: `internal/filter/hcm/h2/settings.go` (add `writeClientInitialSettings`)
- Modify: `internal/filter/hcm/h2/settings_test.go`
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 7 entry)

Lands the new file `client.go` with the static surface: `H2Request`/`H2Response` value types, `ClientConn` struct, `NewClientConn` (preface emit + synchronous SETTINGS exchange + frame-read goroutine spawn), `Close` (graceful GOAWAY emit). The `RoundTrip` method is stubbed to return an error in this task; Task 8 lands the full `RoundTrip` body. This split keeps the test focus narrow (Task 7 covers conn lifecycle; Task 8 covers stream lifecycle).

- [ ] **Step 1: Write `internal/filter/hcm/h2/client_test.go` (the failing tests for Task 7's surface)**

```go
package h2

import (
	"context"
	"net"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

// TestNewClientConn_PrefaceAndSettingsExchange verifies NewClientConn:
//  1. Writes the 24-byte client preface
//  2. Writes initial SETTINGS
//  3. Reads the server's initial SETTINGS
//  4. Writes SETTINGS_ACK in response
//  5. Reads the server's SETTINGS_ACK for our SETTINGS
// Returns a ready-to-RoundTrip *ClientConn.
func TestNewClientConn_PrefaceAndSettingsExchange(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()
	// Spawn a fake server peer that runs the symmetric SETTINGS exchange
	// using x/net/http2.Framer driver-side (D-3.2 governs runtime, not test code).
	done := make(chan error, 1)
	go func() {
		done <- runFakeServerPeerForClientHandshake(serverSide)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cc, err := NewClientConn(ctx, clientSide)
	if err != nil {
		t.Fatalf("NewClientConn: %v", err)
	}
	defer cc.Close()
	if err := <-done; err != nil {
		t.Fatalf("fake peer: %v", err)
	}
	// cc is ready for RoundTrip (Task 8 will exercise this).
}

// TestClientConn_Close_EmitsGracefulGoaway verifies Close emits a GOAWAY
// with NO_ERROR before closing the underlying conn.
func TestClientConn_Close_EmitsGracefulGoaway(t *testing.T) {
	// ... fixture setup (same shape as above) ...
	// After NewClientConn returns, call cc.Close().
	// Read frames from the server side; expect a GOAWAY frame with code=NO_ERROR
	// and last-stream-id matching the highest stream id allocated (0 if none).
	// Then assert the underlying serverSide reads return io.EOF.
}

// TestNewClientConn_SettingsHandshakeFailureBubblesUp verifies that a peer
// sending malformed SETTINGS (e.g. ACK bit set on first frame, which is
// forbidden per RFC 9113 §6.5) causes NewClientConn to return a *Error.
func TestNewClientConn_SettingsHandshakeFailureBubblesUp(t *testing.T) {
	// ... peer writes SETTINGS_ACK as its first frame ...
	// Expect NewClientConn to return *Error{Code: ErrProtocolError}.
}
```

Plus a small helper:

```go
// runFakeServerPeerForClientHandshake reads the client preface + client SETTINGS,
// writes the server's initial SETTINGS + SETTINGS_ACK for the client's SETTINGS,
// then idles. Returns nil on the happy path.
func runFakeServerPeerForClientHandshake(conn net.Conn) error {
	// Read the 24-byte preface.
	prefaceBuf := make([]byte, 24)
	if _, err := io.ReadFull(conn, prefaceBuf); err != nil {
		return fmt.Errorf("preface: %w", err)
	}
	if string(prefaceBuf) != string(clientPrefaceBytes) {
		return fmt.Errorf("bad preface: %q", prefaceBuf)
	}
	fr := http2.NewFramer(conn, conn)
	// Read client SETTINGS.
	if _, err := fr.ReadFrame(); err != nil {
		return fmt.Errorf("read client SETTINGS: %w", err)
	}
	// Write server SETTINGS.
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16384}); err != nil {
		return fmt.Errorf("write server SETTINGS: %w", err)
	}
	// Write SETTINGS_ACK for client's SETTINGS.
	if err := fr.WriteSettingsAck(); err != nil {
		return fmt.Errorf("write SETTINGS_ACK: %w", err)
	}
	// Read client's SETTINGS_ACK for our SETTINGS.
	if _, err := fr.ReadFrame(); err != nil {
		return fmt.Errorf("read client SETTINGS_ACK: %w", err)
	}
	// Idle.
	return nil
}
```

Run: `go test ./internal/filter/hcm/h2/ -run TestNewClientConn -v`
Expected: FAIL — `NewClientConn`, `ClientConn`, `H2Request`, `H2Response` all undefined.

- [ ] **Step 2: Add `writeClientInitialSettings` to `settings.go`**

```go
// writeClientInitialSettings writes the client's initial SETTINGS frame.
// In phase 05.2, the client and server settings are byte-identical (the same
// DefaultServerSettings constants per ADR-0047 apply on both sides;
// SETTINGS_ENABLE_PUSH=0 is correct for the client per RFC 9113 §6.5.2 because
// clients can't accept PUSH — advertising it disabled is symmetric correctness).
// Separate helpers exist for future divergence (when client-only settings tuning
// lands in a future phase).
func writeClientInitialSettings(fr *framer, s ServerSettings) error {
	settings := []http2.Setting{
		{ID: http2.SettingMaxConcurrentStreams, Val: s.MaxConcurrentStreams},
		{ID: http2.SettingInitialWindowSize, Val: s.InitialWindowSize},
		{ID: http2.SettingMaxFrameSize, Val: s.MaxFrameSize},
		{ID: http2.SettingEnablePush, Val: s.EnablePush},
		{ID: http2.SettingHeaderTableSize, Val: s.HeaderTableSize},
		// SETTINGS_NO_RFC7540_PRIORITIES is RFC 9218; not in stdlib's enum.
		{ID: 0x9, Val: s.NoRFC7540Priorities},
	}
	return fr.WriteSettings(settings...)
}
```

Add a small unit test in `settings_test.go` that round-trips the SETTINGS through a `net.Pipe` peer reading the SettingsFrame and asserts each value.

Run: `go test ./internal/filter/hcm/h2/ -run TestClientInitialSettings -v`
Expected: PASS.

- [ ] **Step 3: Write `internal/filter/hcm/h2/client.go` — types + NewClientConn skeleton + Close**

```go
// Package-level doc lives in doc.go; this file adds the client-side surface.
//
// client.go is the ONE new file in the h2 sub-package for phase 05.2 per
// ADR-0048's reservation. It carries the from-scratch H2 client connection
// manager (ClientConn), per-call request/response value types
// (H2Request/H2Response), and the symmetric mirror of ServerConn's
// surface for upstream H2 origination per ADR-0056.

package h2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// H2Request is the codec-internal request shape passed from routerActionH2
// to ClientConn.RoundTrip. The pseudo-headers are split out so the codec
// can encode them in the RFC 9113 §8.3-required order (:method, :path,
// :scheme, :authority first, then regular headers).
type H2Request struct {
	Method    string
	Path      string
	Scheme    string
	Authority string
	Headers   []hpack.HeaderField  // regular headers; pseudo-headers excluded
	Body      []byte               // nil for bodyless GETs (the fixture-0004 case)
}

// H2Response is the codec-internal response shape returned by RoundTrip.
type H2Response struct {
	Status  int
	Headers []hpack.HeaderField  // includes :status as the first element
	Body    []byte
}

// ClientConn is the per-upstream-conn H2 connection manager. One ClientConn
// per upstream *stdtls.Conn after Cluster.DialH2 confirms ALPN h2 (see
// internal/cluster/dial_h2.go).
//
// Per ADR-0056, phase 05.2 uses ClientConn for exactly one RoundTrip per
// instance (per-request fresh dial). The conn supports multi-RT in principle
// (the stream-id allocator is monotonic, not reset per call) but the router
// does not exploit it.
type ClientConn struct {
	ctx          context.Context  // conn-lifetime ctx
	cancel       context.CancelFunc
	conn         net.Conn         // the underlying TLS conn
	fr           *framer
	hp           *hpackState
	sendW        *window  // conn-level send window
	recvW        *window  // conn-level recv window
	clientS      ServerSettings   // our settings (advertised)
	serverS      clientSettings   // peer-advertised settings
	nextStreamID uint32           // atomic; allocated odd from 1 per RFC 9113 §5.1.1
	streams      sync.Map         // map[uint32]*clientStream
	mu           sync.Mutex       // serialises framer writes
	closeOnce    sync.Once
	goawayCh     chan struct{}    // closed when peer GOAWAY observed
	settingsAckCh chan struct{}   // closed when peer ACKs our SETTINGS
	recvDebitSinceLastUpdate int32  // for half-window WINDOW_UPDATE emission
}

// NewClientConn writes the client preface + initial SETTINGS, exchanges
// SETTINGS_ACKs synchronously with the peer, and returns a ready-to-RoundTrip
// conn. Per SPEC §10 #5: the synchronous wait surfaces handshake errors as
// constructor errors instead of mid-request errors.
//
// NewClientConn does NOT take ownership of upstream's TLS handshake — the
// caller (Cluster.DialH2) is expected to have verified ALPN h2 already.
func NewClientConn(ctx context.Context, upstream net.Conn) (*ClientConn, error) {
	ctx, cancel := context.WithCancel(ctx)
	cc := &ClientConn{
		ctx:           ctx,
		cancel:        cancel,
		conn:          upstream,
		fr:            newFramer(upstream, DefaultServerSettings.MaxFrameSize),
		hp:            newHPACKState(DefaultServerSettings.HeaderTableSize),
		sendW:         newWindow(int32(DefaultServerSettings.InitialWindowSize)),
		recvW:         newWindow(int32(DefaultServerSettings.InitialWindowSize)),
		clientS:       DefaultServerSettings,
		nextStreamID:  0,  // atomic increments by 2; first stream allocates 1
		goawayCh:      make(chan struct{}),
		settingsAckCh: make(chan struct{}),
	}
	// Step 1: write the client preface.
	if _, err := upstream.Write(clientPrefaceBytes); err != nil {
		cancel()
		return nil, fmt.Errorf("h2: client: write preface: %w", err)
	}
	// Step 2: write client initial SETTINGS.
	if err := writeClientInitialSettings(cc.fr, cc.clientS); err != nil {
		cancel()
		return nil, fmt.Errorf("h2: client: write SETTINGS: %w", err)
	}
	// Step 3: read peer SETTINGS, apply, write SETTINGS_ACK.
	if err := cc.readPeerSettingsAndAck(ctx); err != nil {
		cancel()
		return nil, err
	}
	// Step 4: spawn the frame-read goroutine BEFORE waiting for SETTINGS_ACK
	// (the ACK arrives as a SETTINGS frame with the ACK flag set; the goroutine
	// closes settingsAckCh when it sees that frame).
	go cc.readLoop()
	// Step 5: wait synchronously for the peer's SETTINGS_ACK for our SETTINGS.
	select {
	case <-cc.settingsAckCh:
		return cc, nil
	case <-ctx.Done():
		cc.cancel()
		return nil, fmt.Errorf("h2: client: SETTINGS_ACK wait: %w", ctx.Err())
	}
}

// readPeerSettingsAndAck reads exactly one SETTINGS frame from the peer
// (which MUST NOT be an ACK; the peer's first frame is the peer's initial
// SETTINGS per RFC 9113 §6.5), applies the values to cc.serverS, and writes
// SETTINGS_ACK back.
func (cc *ClientConn) readPeerSettingsAndAck(ctx context.Context) error {
	if err := readClientSettings(cc.fr, &cc.serverS); err != nil {
		return fmt.Errorf("h2: client: read peer SETTINGS: %w", err)
	}
	cc.mu.Lock()
	err := cc.fr.WriteSettingsAck()
	cc.mu.Unlock()
	if err != nil {
		return fmt.Errorf("h2: client: write SETTINGS_ACK: %w", err)
	}
	return nil
}

// readLoop runs the conn-level frame-read goroutine. Per SPEC §10 #7, this
// is structured separately from ServerConn.Run because of role asymmetries
// (no PUSH_PROMISE acceptance; client allocates stream ids; settings
// application is peer's-not-ours).
func (cc *ClientConn) readLoop() {
	for {
		f, err := cc.fr.readFrameCtx(cc.ctx)
		if err != nil {
			cc.cancel()
			return
		}
		if err := cc.dispatchFrame(f); err != nil {
			cc.cancel()
			return
		}
	}
}

// dispatchFrame routes a single inbound frame. The full implementation
// lands in Task 8; this skeleton handles only the SETTINGS_ACK signal
// needed for NewClientConn's synchronous wait, plus GOAWAY observation.
func (cc *ClientConn) dispatchFrame(f http2.Frame) error {
	switch fr := f.(type) {
	case *http2.SettingsFrame:
		if fr.IsAck() {
			select {
			case <-cc.settingsAckCh:
				// already closed
			default:
				close(cc.settingsAckCh)
			}
			return nil
		}
		// Mid-stream SETTINGS update (peer changing window sizes etc.) —
		// apply and ACK. Task 8 expands this.
		return nil
	case *http2.GoAwayFrame:
		select {
		case <-cc.goawayCh:
		default:
			close(cc.goawayCh)
		}
		return nil
	default:
		// Stream-routed frames: handled by Task 8's per-stream channels.
		// Skeleton ignores them (will fail tests in Task 8; correct here).
		return nil
	}
}

// RoundTrip is stubbed in this task. Task 8 lands the full implementation.
func (cc *ClientConn) RoundTrip(ctx context.Context, req H2Request) (H2Response, error) {
	return H2Response{}, errors.New("h2: client: RoundTrip not implemented (Task 8)")
}

// Close emits a graceful GOAWAY(NO_ERROR) with the highest allocated stream
// id as last-stream-id and closes the underlying conn. Idempotent.
func (cc *ClientConn) Close() error {
	var closeErr error
	cc.closeOnce.Do(func() {
		lastID := atomic.LoadUint32(&cc.nextStreamID)
		cc.mu.Lock()
		_ = cc.fr.WriteGoAway(lastID, uint32(ErrNoError), []byte("client close"))
		cc.mu.Unlock()
		cc.cancel()
		closeErr = cc.conn.Close()
	})
	return closeErr
}
```

Run: `go test ./internal/filter/hcm/h2/ -run TestNewClientConn_PrefaceAndSettingsExchange -v`
Expected: PASS.

Run: `go test ./internal/filter/hcm/h2/ -run TestClientConn_Close_EmitsGracefulGoaway -v`
Expected: PASS.

Run: `go test ./internal/filter/hcm/h2/ -run TestNewClientConn_SettingsHandshakeFailureBubblesUp -v`
Expected: PASS.

Note: `clientPrefaceBytes` may need a small `package h2` export adjustment — it's defined in `preface.go` as a package-internal constant; `client.go` consumes it directly (same package). If 05.1 named it lower-case, this works without change.

- [ ] **Step 4: Run full h2 package tests + race-detector + lint**

```bash
go test -race ./internal/filter/hcm/h2/ -v
go vet ./internal/filter/hcm/h2/
golangci-lint run ./internal/filter/hcm/h2/
```

Expected: green; the new client.go + client_test.go pass; existing tests unaffected.

ADR-0046 boundary check: `client.go` imports `golang.org/x/net/http2` directly (it's in the allowed file list per the SPEC's tech-stack section). The boundary grep at Task 15 step 9 verifies this is the only addition.

- [ ] **Step 5: Append Task 7 entry to PROGRESS.md, commit**

```bash
git add internal/filter/hcm/h2/client.go internal/filter/hcm/h2/client_test.go internal/filter/hcm/h2/settings.go internal/filter/hcm/h2/settings_test.go docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: client.go skeleton — H2Request/H2Response + NewClientConn + Close"
```

---

## Task 8: H2 client-side codec — `(*ClientConn).RoundTrip` + `clientStream` + frame-read loop

**Files:**
- Modify: `internal/filter/hcm/h2/client.go`
- Modify: `internal/filter/hcm/h2/client_test.go`
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 8 entry)

The largest single-file change in 05.2. Lands the full `RoundTrip` body, the `clientStream` per-stream state, and the frame-read loop's stream-routed dispatch (DATA/HEADERS/RST_STREAM/WINDOW_UPDATE/PING for streams + the conn-level cases from Task 7's skeleton expanded).

- [ ] **Step 1: Write the failing tests for `RoundTrip`**

The full test set per SPEC §4.1 + §8.1:

```go
// TestClientConn_RoundTrip_HappyPath_BodylessGET — bodyless GET → server
// responds with HEADERS+DATA+END_STREAM; assert H2Response{Status, Headers, Body}.
// TestClientConn_RoundTrip_HappyPath_WithBody — POST with body → server
// responds with HEADERS+DATA+END_STREAM.
// TestClientConn_RoundTrip_CtxCancelDuringWrite — ctx cancels mid-HEADERS
// write; assert RoundTrip returns ctx.Err() AND emits RST_STREAM(CANCEL).
// TestClientConn_RoundTrip_CtxCancelDuringRead — ctx cancels mid-DATA read;
// same assertion shape.
// TestClientConn_RoundTrip_PeerRSTStream — peer sends RST_STREAM(CANCEL)
// mid-response; assert RoundTrip returns *Error{Code: CANCEL}.
// TestClientConn_RoundTrip_PeerGoaway — peer sends GOAWAY(NO_ERROR)
// before our request reaches it; assert RoundTrip returns *Error{Code: NO_ERROR}.
// TestClientConn_RoundTrip_PeerDataAfterEndStream — peer sends DATA after END_STREAM;
// assert RoundTrip returns *Error{Code: STREAM_CLOSED} on the SECOND DATA frame.
// TestClientConn_RoundTrip_AfterClose — call RoundTrip after Close;
// assert error.
// TestClientConn_RoundTrip_StreamIDMonotonicity — three RoundTrips on the
// same conn; assert stream ids 1, 3, 5.
```

Run each test: FAIL (RoundTrip is the stub from Task 7).

- [ ] **Step 2: Implement `clientStream` + `RoundTrip` + dispatchFrame extension**

```go
type clientStream struct {
	id           uint32
	cc           *ClientConn
	sendW        *window
	recvW        *window
	respHeaders  []hpack.HeaderField  // populated on inbound HEADERS
	respStatus   int                  // populated on inbound HEADERS (parsed from :status)
	respBody     bytes.Buffer
	doneCh       chan error           // closed (with nil) on END_STREAM; closed (with *Error) on RST_STREAM/peer-error
	closedOnce   sync.Once
	recvDebitSinceLastUpdate int32
}

func newClientStream(id uint32, cc *ClientConn) *clientStream {
	return &clientStream{
		id:    id,
		cc:    cc,
		sendW: newWindow(int32(cc.serverS.InitialWindowSize)),
		recvW: newWindow(int32(cc.clientS.InitialWindowSize)),
		doneCh: make(chan error, 1),
	}
}

func (cs *clientStream) finish(err error) {
	cs.closedOnce.Do(func() {
		cs.doneCh <- err
		close(cs.doneCh)
	})
}

// RoundTrip allocates a fresh stream id, encodes HEADERS+DATA+END_STREAM,
// waits on cs.doneCh for the response, returns the assembled H2Response.
//
// Per RFC 9113 §5.1.1, client-initiated streams use odd-numbered ids
// starting at 1. The atomic allocator stores nextID×2 (initialised to 0);
// each RoundTrip computes id = atomic.AddUint32(&cc.nextStreamID, 2) - 1,
// returning 1, 3, 5, ... per call.
func (cc *ClientConn) RoundTrip(ctx context.Context, req H2Request) (H2Response, error) {
	if err := cc.ctx.Err(); err != nil {
		return H2Response{}, fmt.Errorf("h2: client: RoundTrip on closed conn: %w", err)
	}
	id := atomic.AddUint32(&cc.nextStreamID, 2) - 1
	cs := newClientStream(id, cc)
	cc.streams.Store(id, cs)
	defer cc.streams.Delete(id)

	// Encode HEADERS: pseudo-headers first per RFC 9113 §8.3.
	var headers []hpack.HeaderField
	headers = append(headers, hpack.HeaderField{Name: ":method", Value: req.Method})
	headers = append(headers, hpack.HeaderField{Name: ":path", Value: req.Path})
	headers = append(headers, hpack.HeaderField{Name: ":scheme", Value: req.Scheme})
	headers = append(headers, hpack.HeaderField{Name: ":authority", Value: req.Authority})
	headers = append(headers, req.Headers...)
	encoded := cc.hp.encodeHeaders(headers)

	endStream := len(req.Body) == 0
	cc.mu.Lock()
	err := cc.fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      id,
		BlockFragment: encoded,
		EndStream:     endStream,
		EndHeaders:    true,
	})
	cc.mu.Unlock()
	if err != nil {
		cs.finish(streamError(ErrInternalError, id, fmt.Sprintf("h2: client: WriteHeaders: %v", err)))
		return H2Response{}, translateFramerErr(err)
	}

	// Write body if present, chunked per ADR-0055 / I-1 + I-2.
	if !endStream {
		if err := cc.writeData(ctx, cs, req.Body, true); err != nil {
			cs.finish(err)
			return H2Response{}, err
		}
	}

	// Wait for response or ctx cancel.
	select {
	case err := <-cs.doneCh:
		if err != nil {
			return H2Response{}, err
		}
		return H2Response{
			Status:  cs.respStatus,
			Headers: cs.respHeaders,
			Body:    cs.respBody.Bytes(),
		}, nil
	case <-ctx.Done():
		// Emit RST_STREAM(CANCEL) on the upstream stream; return ctx error.
		cc.mu.Lock()
		_ = cc.fr.WriteRSTStream(id, http2.ErrCodeCancel)
		cc.mu.Unlock()
		return H2Response{}, ctx.Err()
	case <-cc.ctx.Done():
		return H2Response{}, fmt.Errorf("h2: client: conn closed mid-RoundTrip: %w", cc.ctx.Err())
	}
}

// writeData chunks b into one or more DATA frames respecting the same
// flow-control discipline as ServerConn.writeData (per ADR-0055 / I-1 + I-2).
// Symmetric implementation; the only difference is the per-stream window
// belongs to clientStream rather than serverStream.
func (cc *ClientConn) writeData(ctx context.Context, cs *clientStream, b []byte, endStream bool) error {
	// (mirror of ServerConn.writeData per Task 3, with cs.sendW in place of ss.sendW)
	// ...
}
```

Extend `dispatchFrame` to route stream-bound frames:

```go
case *http2.HeadersFrame:
	cs, ok := cc.lookupStream(fr.StreamID)
	if !ok {
		// Server-initiated HEADERS (PUSH_PROMISE-class) — we advertise
		// ENABLE_PUSH=0 so this is a peer protocol violation.
		cc.emitGoaway(ErrProtocolError)
		return connError(ErrProtocolError, "client: server-initiated HEADERS with ENABLE_PUSH=0")
	}
	// Decode pseudo-headers + regular headers; extract :status.
	decoded, err := cc.hp.decodeBlock(fr.HeaderBlockFragment(), fr.HeadersEnded())
	if err != nil {
		cc.emitGoaway(ErrCompressionError)
		return err
	}
	cs.respHeaders = decoded
	for _, hf := range decoded {
		if hf.Name == ":status" {
			cs.respStatus, _ = strconv.Atoi(hf.Value)
			break
		}
	}
	if fr.StreamEnded() {
		cs.finish(nil)
	}
	return nil

case *http2.DataFrame:
	cs, ok := cc.lookupStream(fr.StreamID)
	if !ok {
		// Stream gone (we already finished it) — DATA after END_STREAM.
		return streamError(ErrStreamClosed, fr.StreamID, "client: DATA on closed stream")
	}
	cs.respBody.Write(fr.Data())
	// Recv-window debit + half-window WINDOW_UPDATE emission per ADR-0055 / I-3.
	// (mirror of ServerConn.onData; uses cs.recvW + cc.recvW)
	// ...
	if fr.StreamEnded() {
		cs.finish(nil)
	}
	return nil

case *http2.RSTStreamFrame:
	cs, ok := cc.lookupStream(fr.StreamID)
	if !ok {
		return nil  // stream gone; ignore
	}
	cs.finish(streamError(ErrCode(fr.ErrCode), fr.StreamID, "client: peer RST_STREAM"))
	return nil

case *http2.WindowUpdateFrame:
	// Mirror ServerConn.onWindowUpdate per ADR-0055 / M-9.
	// ...
	return nil

case *http2.PingFrame:
	if !fr.IsAck() {
		cc.mu.Lock()
		err := cc.fr.WritePing(true, fr.Data)
		cc.mu.Unlock()
		return translateFramerErr(err)
	}
	return nil
```

Plus a `(cc *ClientConn) lookupStream(id uint32) (*clientStream, bool)` helper, and a `(cc *ClientConn) emitGoaway(code ErrCode)` helper.

Run each test in Step 1: PASS.

- [ ] **Step 3: Run full h2 package tests + race-detector + h2spec**

```bash
go test -race ./internal/filter/hcm/h2/ -v
go test ./test/conformance/h2spec/ -v
```

Expected: green; h2spec 53/53 PASS (no regression — h2spec only exercises the server side; the client side is new code that adds tests but doesn't touch the server-side surface).

- [ ] **Step 4: Lint + ADR-0046 boundary partial check**

```bash
golangci-lint run ./internal/filter/hcm/h2/
grep -n '"golang.org/x/net/http2"' internal/filter/hcm/h2/client.go
```

Expected: lint clean; the import in client.go is the ONLY production-code addition to the boundary.

- [ ] **Step 5: Append Task 8 entry to PROGRESS.md, commit**

```bash
git add internal/filter/hcm/h2/client.go internal/filter/hcm/h2/client_test.go docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: client.go RoundTrip + clientStream + frame-read dispatch"
```

---

## Task 9: `Cluster.UseH2()` accessor + `internal/cluster/dial_h2.go` + ADR-0056

**Files:**
- Modify: `internal/cluster/cluster.go` (`useH2 bool` field + `UseH2()` accessor + blank import)
- Create: `internal/cluster/dial_h2.go`
- Create: `internal/cluster/dial_h2_test.go`
- Modify: `internal/cluster/cluster_test.go` (UseH2 accessor coverage)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0056)
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 9 entry)

Lands the cluster-side dial helper. The `useH2` field is set to `false` for every existing cluster build path (Task 10 wires up the actual setter from `HttpProtocolOptions` parsing). ADR-0056 lands here per the first-use ordering — `Cluster.DialH2` is the per-request fresh-conn site.

- [ ] **Step 1: Write the failing tests for `Cluster.UseH2()` and `Cluster.DialH2`**

In `cluster_test.go`:

```go
func TestCluster_UseH2_DefaultsFalse(t *testing.T) {
	c := &Cluster{}
	if c.UseH2() {
		t.Errorf("zero-value Cluster.UseH2() = true, want false")
	}
}

func TestCluster_UseH2_True(t *testing.T) {
	c := &Cluster{useH2: true}
	if !c.UseH2() {
		t.Errorf("Cluster.UseH2() = false, want true (with useH2: true)")
	}
}
```

In `dial_h2_test.go`:

```go
package cluster

import (
	"context"
	"crypto/tls"
	"net"
	"testing"

	"golang.org/x/net/http2"
)

// TestCluster_DialH2_HappyPath verifies DialH2 against an in-process h2-over-TLS
// backend. The backend uses crypto/tls + http2.ConfigureServer driver-side
// (D-3.2 governs runtime, not test code).
func TestCluster_DialH2_HappyPath(t *testing.T) {
	// Stand up an in-process h2-over-TLS server with NextProtos: ["h2"].
	// Construct a *Cluster pointing at the backend.
	// Call c.DialH2(ctx); assert nil error and a non-nil *h2.ClientConn.
	// ... (full fixture setup mirroring fixture-0002 driver_test.go's TLS pattern)
}

func TestCluster_DialH2_ALPNMismatch(t *testing.T) {
	// In-process backend negotiates http/1.1 (NextProtos: ["http/1.1"]).
	// DialH2 should error with "alpn negotiated %q, want \"h2\"".
}

func TestCluster_DialH2_NotTLS(t *testing.T) {
	// Plain TCP backend (no TLS).
	// DialH2 should error with "not a TLS conn".
}

func TestCluster_DialH2_CtxCancel(t *testing.T) {
	// Cancel ctx before dial completes.
	// DialH2 should error with the propagated ctx error.
}

func TestCluster_DialH2_TLSHandshakeFailure(t *testing.T) {
	// In-process backend that closes the TLS handshake mid-flight.
	// DialH2's error should bubble through Cluster.Dial's existing TLS error handling.
}
```

Run: `go test ./internal/cluster/ -v -run "TestCluster_UseH2_|TestCluster_DialH2_"`
Expected: FAIL — accessor + dial_h2.go don't exist yet.

- [ ] **Step 2: Add `useH2 bool` + `UseH2()` to `cluster.go`; add the blank import**

```go
// (existing imports)
import (
	// ... existing ...

	// Phase 05.2 (ADR-0016 amendment, no separate ADR): the cluster-side
	// HttpProtocolOptions extension proto is registered with
	// protoregistry.GlobalTypes so protojson can round-trip the typed-extension
	// in Manager.buildCluster (per Task 10).
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
)

type Cluster struct {
	// ... existing fields ...
	useH2 bool  // set by Manager.buildCluster from typed_extension_protocol_options
}

// UseH2 reports whether this cluster's HttpProtocolOptions selects HTTP/2
// upstream origination. When true, the HCM filter-build path constructs
// routerActionH2 instead of routerAction (per ADR-0056; phase 05.2 SPEC §5.5).
func (c *Cluster) UseH2() bool { return c.useH2 }
```

- [ ] **Step 3: Write `internal/cluster/dial_h2.go`**

```go
package cluster

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"fmt"

	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// DialH2 dials an upstream endpoint, confirms the negotiated ALPN protocol
// is "h2", and wraps the conn in an h2.ClientConn ready for one RoundTrip.
//
// Per ADR-0056, the returned *h2.ClientConn is per-request fresh; the caller
// (routerActionH2.do) closes it via defer after the response is consumed.
//
// Each error branch closes the underlying conn explicitly because the function
// returns the conn on success (caller takes ownership); on error, no caller-owned
// conn exists to defer-close.
func (c *Cluster) DialH2(ctx context.Context) (*h2.ClientConn, error) {
	raw, err := c.Dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("cluster: dial h2: %w", err)
	}
	tlsConn, ok := raw.(*stdtls.Conn)
	if !ok {
		_ = raw.Close()
		return nil, errors.New("cluster: dial h2: not a TLS conn")
	}
	// Defensive: ensure handshake is complete so NegotiatedProtocol is
	// authoritative. Idempotent for already-handshaken conns; SPEC §11.3 mitigation.
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("cluster: dial h2: handshake: %w", err)
	}
	alpn := tlsConn.ConnectionState().NegotiatedProtocol
	if alpn != "h2" {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("cluster: dial h2: alpn negotiated %q, want %q", alpn, "h2")
	}
	cc, err := h2.NewClientConn(ctx, tlsConn)
	if err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("cluster: dial h2: client conn: %w", err)
	}
	return cc, nil
}
```

Run: `go test ./internal/cluster/ -v -run "TestCluster_UseH2_|TestCluster_DialH2_"`
Expected: PASS.

- [ ] **Step 4: Append ADR-0056 to `docs/envoy-go/DECISIONS.md`**

ADR-0056 prose per `## ADRs introduced by this plan` summary. Cross-reference:
- `internal/cluster/dial_h2.go:DialH2` (the per-request fresh dial site)
- `routerActionH2.do` at `internal/filter/hcm/actions.go` (Task 11; the call site that consumes ADR-0056's discipline; the `defer cc.Close()` is the per-request closure)
- ADR-0039 (the H1 mirror)
- ADR-0053's "phase-05.2-will-repeat-the-pattern" forward-looking note (now resolved by ADR-0056's `defer` mechanism acknowledgement)

- [ ] **Step 5: Run full cluster tests + race-detector + lint**

```bash
go test -race ./internal/cluster/ -v
go vet ./internal/cluster/
golangci-lint run ./internal/cluster/
```

Expected: green.

- [ ] **Step 6: Append Task 9 entry to PROGRESS.md, commit**

```bash
git add internal/cluster/cluster.go internal/cluster/dial_h2.go internal/cluster/dial_h2_test.go internal/cluster/cluster_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: Cluster.DialH2 + UseH2 accessor + ADR-0056"
```

---

## Task 10: `internal/cluster/manager.go` HttpProtocolOptions parsing + `internal/bootstrap/bootstrap.go` blank import

**Files:**
- Modify: `internal/cluster/manager.go`
- Modify: `internal/cluster/manager_test.go`
- Modify: `internal/bootstrap/bootstrap.go`
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 10 entry)

Wires up the actual `useH2` setter from the cluster's typed extension. The behaviour matrix per SPEC §5.5 lands here. No ADR — the silent-ignore narrowing of master SPEC §5.8 is documented in 05.2 SPEC §5.5; the SPEC document itself is the durable D-3.4 record per SPEC §11.6.

- [ ] **Step 1: Write the failing tests for the parser per SPEC §4.1 + §8.2**

```go
// All tests live in manager_test.go.

// TestBuildCluster_H2Mode_Positive — http2_protocol_options{} + TLS + ALPN h2 → builds + UseH2()==true.
// TestBuildCluster_H2Mode_NoTLS — http2_protocol_options{} + no transport_socket → build error.
// TestBuildCluster_H2Mode_TLSWithoutALPNH2 — http2_protocol_options{} + TLS + alpn_protocols=["http/1.1"] → build error.
// TestBuildCluster_H2Mode_TLSWithoutALPN — http2_protocol_options{} + TLS without alpn_protocols → build error.
// TestBuildCluster_H1Discriminator_SilentIgnore — http_protocol_options{} → builds + UseH2()==false.
// TestBuildCluster_AutoConfig_SilentIgnore — auto_config{} → builds + UseH2()==false (the 05.2 narrowing).
// TestBuildCluster_NoTypedExtension_BaselineFalse — no typed_extension_protocol_options → builds + UseH2()==false.
// TestBuildCluster_HttpProtocolOptions_NilUpstreamProtocolOptions — empty HttpProtocolOptions{} → builds + UseH2()==false.
```

Each test constructs a `*clusterv3.Cluster` proto with the appropriate `typed_extension_protocol_options` map, calls `buildCluster`, and asserts the diagnostic / `UseH2()` value.

Run: `go test ./internal/cluster/ -v -run TestBuildCluster_H2Mode_ -count=1`
Expected: FAIL — `extractH2Mode` doesn't exist; `useH2` is always `false`.

- [ ] **Step 2: Implement `extractH2Mode` in `manager.go`**

```go
import (
	// ... existing ...
	upstreamshttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
)

const httpProtocolOptionsTypeURL = "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions"

// extractH2Mode reads the cluster's typed_extension_protocol_options and
// returns whether to enable H2 upstream origination. Per SPEC §5.5's behaviour
// matrix:
//   - field absent → false (phase-04 baseline; no regression)
//   - explicit_http_config.http2_protocol_options{} → true (validated; build-time TLS+ALPN check)
//   - explicit_http_config.http_protocol_options{} → false (silent-ignore inner)
//   - auto_config{} → false (the 05.2 narrowing of master SPEC §5.8)
//   - nil/empty UpstreamProtocolOptions → false (defensive)
//
// When useH2==true: the cluster's transport_socket MUST be present, MUST be
// type tls, and the parsed TLS config's alpn_protocols MUST include "h2".
// Validation errors carry the diagnostics enumerated in SPEC §4.1.
func extractH2Mode(c *clusterv3.Cluster, parsedTLS *internalTLSContext) (useH2 bool, err error) {
	tepo := c.GetTypedExtensionProtocolOptions()
	if tepo == nil {
		return false, nil
	}
	any, ok := tepo["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
	if !ok {
		return false, nil
	}
	var hpo upstreamshttpv3.HttpProtocolOptions
	if err := any.UnmarshalTo(&hpo); err != nil {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions: unmarshal: %w", c.Name, err)
	}
	switch up := hpo.UpstreamProtocolOptions.(type) {
	case *upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig:
		switch up.ExplicitHttpConfig.ProtocolConfig.(type) {
		case *upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions:
			useH2 = true
		default:
			useH2 = false
		}
	case *upstreamshttpv3.HttpProtocolOptions_AutoConfig:
		useH2 = false  // the 05.2 narrowing of master SPEC §5.8
	default:
		useH2 = false
	}
	if !useH2 {
		return false, nil
	}
	// Validate transport socket + ALPN.
	if c.TransportSocket == nil {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires transport_socket", c.Name)
	}
	if c.TransportSocket.GetTypedConfig() == nil {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires transport_socket of type tls, got transport_socket without typed_config", c.Name)
	}
	if c.TransportSocket.GetTypedConfig().GetTypeUrl() != upstreamTLSContextTypeURL {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires transport_socket of type tls, got %q", c.Name, c.TransportSocket.GetTypedConfig().GetTypeUrl())
	}
	if parsedTLS == nil {
		// transport_socket present but TLS parsing returned nil — this is an internal invariant.
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options: TLS parse returned nil", c.Name)
	}
	hasH2 := false
	for _, alpn := range parsedTLS.ALPNProtocols {
		if alpn == "h2" {
			hasH2 = true
			break
		}
	}
	if !hasH2 {
		return false, fmt.Errorf("cluster: %q: HttpProtocolOptions.http2_protocol_options requires alpn_protocols to include %q, got %v", c.Name, "h2", parsedTLS.ALPNProtocols)
	}
	return true, nil
}
```

(`internalTLSContext` is the in-memory TLS-config shape `buildCluster` already uses for phase-03 transport-socket parsing; rename appropriately to match the on-disk symbol.)

Wire `extractH2Mode` into `buildCluster`:

```go
func buildCluster(c *clusterv3.Cluster, idx int, baseDir string) (*Cluster, error) {
	// ... existing prelude (name, lb policy, endpoints) ...

	// Parse transport_socket (phase-03 surface; UNCHANGED).
	parsedTLS, err := parseTransportSocket(c, baseDir)
	if err != nil {
		return nil, err
	}

	// Phase 05.2: extract H2 mode from typed_extension_protocol_options.
	useH2, err := extractH2Mode(c, parsedTLS)
	if err != nil {
		return nil, err
	}

	cl := &Cluster{
		// ... existing fields ...
		useH2: useH2,
	}
	return cl, nil
}
```

Run: `go test ./internal/cluster/ -v -run TestBuildCluster_H2Mode_`
Expected: PASS for the eight new cases.

- [ ] **Step 3: Add the blank import to `internal/bootstrap/bootstrap.go`**

```go
import (
	// ... existing ...

	// Phase 05.2 (ADR-0016 amendment, no separate ADR): registers the cluster-side
	// HttpProtocolOptions extension proto so protojson round-trips fixture 0004's
	// bootstraps without interpreting typed_config.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
)
```

Add a small bootstrap_test.go round-trip case verifying that a fixture-0004-shaped bootstrap (with `HttpProtocolOptions` on the cluster) loads cleanly:

```go
func TestBootstrap_RoundTrips_FixtureFour_Shape(t *testing.T) {
	yamlBytes := []byte(...)  // a minimal bootstrap with HttpProtocolOptions
	bs, err := Load(bytes.NewReader(yamlBytes))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// ... assert the cluster has the typed extension preserved through the round trip ...
}
```

Run: `go test ./internal/bootstrap/ -v`
Expected: PASS.

- [ ] **Step 4: Run full cluster + bootstrap tests + race-detector + lint**

```bash
go test -race ./internal/cluster/ ./internal/bootstrap/ -v
go vet ./internal/cluster/ ./internal/bootstrap/
golangci-lint run ./internal/cluster/ ./internal/bootstrap/
```

Expected: green.

- [ ] **Step 5: Append Task 10 entry to PROGRESS.md, commit**

```bash
git add internal/cluster/manager.go internal/cluster/manager_test.go internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: Manager.buildCluster reads HttpProtocolOptions; bootstrap blank import"
```

---

## Task 11: HCM `routerActionH2` action + filter-build variant selection + `h2dispatch.go` adapter + `h2.Action` widening (NOT the hcm-package `action`) + ADR-0058

**Files:**
- Modify: `internal/filter/hcm/h2/stream.go` (widen `Action` interface)
- Modify: `internal/filter/hcm/h2/stream_test.go`
- Modify: `internal/filter/hcm/actions.go` (add `routerActionH2` + shared `bad502Body` constant)
- Modify: `internal/filter/hcm/actions_test.go`
- Modify: `internal/filter/hcm/config.go` (`buildRouterAction` picks variant)
- Modify: `internal/filter/hcm/config_test.go`
- Modify: `internal/filter/hcm/h2dispatch.go` (add `h2RouterActionAdapter`)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0058)
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 11 entry)

Lands the action variant + the build-time variant selection + the dispatch adapter + the `h2.Action` interface widening. ADR-0058 lands here per the first-use ordering — `routerActionH2.do` is the first site that observes-and-discards trailers.

**Two interfaces, two separate decisions** — to avoid confusion at execution time:
- The **`h2.Action`** interface (in package `internal/filter/hcm/h2`, exported, consumed by `serverStream.dispatch`) **IS WIDENED** in this task to take `(ctx, req H2Request, sw StreamWriter)`.
- The **`hcm.action`** interface (in package `internal/filter/hcm`, lower-case package-internal, consumed by the H1 driver `connection.go`'s `entry.action.do(ctx, req, bw)`) **IS NOT WIDENED** — its `do(ctx, *http.Request, *bufio.Writer) error` method-set stays as-is from 05.1.

`routerActionH2` satisfies neither interface directly: its `do(ctx, h2.H2Request, h2.StreamWriter) error` method-set is consumed by `h2RouterActionAdapter.WriteH2` only (the new adapter in `h2dispatch.go`). On the H1 path, an H2-cluster route is unreachable in well-formed bootstraps because variant selection at filter-build time guarantees H2-clusters get `*routerActionH2` and H1-clusters get `*routerAction`; the H1 driver's `entry.action.do(...)` call site never sees `*routerActionH2`.

- [ ] **Step 1: Widen the `h2.Action` interface (in package `h2`)**

In `internal/filter/hcm/h2/stream.go`:

```go
// Action is the codec-neutral interface satisfied by HCM action variants
// (directResponseAction's adapter, routerActionH2's adapter). The H2 stream
// dispatch (serverStream.dispatch) invokes Action.WriteH2 polymorphically.
//
// Phase 05.2: the interface is widened from the 05.1 single-arg shape to
// (ctx, req, sw) so routerActionH2 can consume the upstream-bound request.
// directResponseAction's adapter ignores ctx + req (synthesises the reply
// from the action's own state).
type Action interface {
	WriteH2(ctx context.Context, req H2Request, sw StreamWriter) error
}
```

Update `serverStream.dispatch` to construct an `H2Request` from the inbound HEADERS pseudo-headers + decoded regular headers + buffered request body, and pass it to `action.WriteH2(ctx, req, sw)`.

Run: `go test ./internal/filter/hcm/h2/ -v`
Expected: PASS for existing tests once the call site is updated; the existing 05.1 cases use `directResponseAction` which ignores `req`.

- [ ] **Step 2: Add `routerActionH2` + shared `bad502Body` constant to `actions.go`**

```go
// Shared 502 local-reply body used by both the H1 router (when extended in a
// future phase to land a non-empty 502 body) and the H2 router (which uses
// it now per SPEC §11.9). The single source eliminates the divergence-risk
// of two separately-maintained string literals.
const bad502Body = "bad gateway\n"

type routerActionH2 struct {
	cluster *cluster.Cluster
}

// do drives an upstream H2 round-trip via Cluster.DialH2 + ClientConn.RoundTrip
// per ADR-0056 (per-request fresh dial), writing the response back through
// the H2 stream writer. Per ADR-0058: trailers are observed but not forwarded.
func (r *routerActionH2) do(ctx context.Context, req h2.H2Request, w h2.StreamWriter) error {
	cc, err := r.cluster.DialH2(ctx)
	if err != nil {
		return r.write502(w, fmt.Sprintf("upstream dial: %v", err))
	}
	defer cc.Close()  // ADR-0056: per-request fresh conn close (the analogue of phase-04 H1's defer upstream.Close())
	resp, err := cc.RoundTrip(ctx, req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return w.RST(h2.ErrCancel)
		}
		return r.write502(w, fmt.Sprintf("upstream roundtrip: %v", err))
	}
	// Forward response: pseudo-headers first (already first in resp.Headers
	// because the codec preserves the on-the-wire order from the upstream).
	if err := w.Headers(resp.Headers, false); err != nil {
		return err  // surfaced to serverStream.dispatch which emits RST_STREAM(INTERNAL_ERROR)
	}
	if err := w.Data(resp.Body, true); err != nil {
		return err
	}
	return nil
}

// write502 emits a 502 Bad Gateway local-reply via the H2 stream writer.
// The body is the shared bad502Body constant per SPEC §11.9. The Date
// header is included per SPEC §10 #4.
func (r *routerActionH2) write502(w h2.StreamWriter, _detail string) error {
	hdrs := []hpack.HeaderField{
		{Name: ":status", Value: "502"},
		{Name: "content-type", Value: "text/plain"},
		{Name: "server", Value: "envoy"},  // ADR-0014
		{Name: "content-length", Value: fmt.Sprintf("%d", len(bad502Body))},
		{Name: "date", Value: dateNowRFC7231()},
	}
	_ = w.Headers(hdrs, false)
	_ = w.Data([]byte(bad502Body), true)
	return nil
}
```

The `dateNowRFC7231` helper already exists in 05.1's actions.go (used by `directResponseAction.writeH2`).

- [ ] **Step 3: Wire variant selection in `buildRouterAction` (config.go) — the hcm-package `action` interface is NOT widened**

```go
// buildRouterAction returns an action interface satisfying the codec-neutral
// action shape. Phase 05.2: when the resolved cluster's UseH2() is true, the
// action variant is routerActionH2; otherwise the existing routerAction (H1).
// Per phase 05.2 SPEC §5.5 + §4.1.
func buildRouterAction(r *routev3.RouteAction, clusters *cluster.Manager) (action, error) {
	c, ok := clusters.Get(r.GetCluster())
	if !ok {
		return nil, fmt.Errorf("cluster %q: not found", r.GetCluster())
	}
	if c.UseH2() {
		return &routerActionH2{cluster: c}, nil
	}
	return &routerAction{cluster: c}, nil
}
```

**The hcm-package `action` interface stays unwidened** (this is THE different decision from Step 1's `h2.Action` widening). `h2dispatch.go`'s `Match` type-asserts to `*routerActionH2` (or `*directResponseAction`) directly and returns the appropriate adapter; the H1 driver's `entry.action.do(...)` call site is unchanged; it never sees `*routerActionH2` because variant selection guarantees an H2-cluster route gets `*routerActionH2` and is dispatched on the H2 path only. On the H1 path, an H2-cluster route is unreachable in well-formed bootstraps; if reached defensively (invalid bootstrap shape), `routerActionH2` does NOT implement `do(ctx, *http.Request, *bufio.Writer) error` and the type-assert in `connection.go`'s H1 dispatch fails — the conn returns 500 + closes. (Verify this assertion at execution time; if `connection.go` doesn't currently type-assert, the planner alternative is to add a runtime-check `if _, ok := entry.action.(*routerActionH2); ok { return 500-internal-error-or-similar }` at the H1 dispatch boundary — but this is a defensive shape only.)

- [ ] **Step 4: Add `h2RouterActionAdapter` to `h2dispatch.go`**

```go
// h2RouterActionAdapter wraps a *routerActionH2 as an h2.Action. WriteH2
// delegates to a.a.do(ctx, req, sw).
type h2RouterActionAdapter struct {
	a *routerActionH2
}

func (a *h2RouterActionAdapter) WriteH2(ctx context.Context, req h2.H2Request, sw h2.StreamWriter) error {
	return a.a.do(ctx, req, sw)
}
```

Update `h2Dispatcher.Match` to recognise `*routerActionH2` and return the new adapter:

```go
func (d *h2Dispatcher) Match(req *http.Request) (h2.Action, bool) {
	entry, ok := d.table.match(req)
	if !ok {
		return &h2DirectResponseAdapter{a: &directResponseAction{status: 404, bodyText: ""}}, true
	}
	switch a := entry.action.(type) {
	case *directResponseAction:
		return &h2DirectResponseAdapter{a: a}, true
	case *routerActionH2:
		return &h2RouterActionAdapter{a: a}, true
	default:
		// Non-H2 router action on H2 path (defensive — variant selection should prevent this).
		return &h2RouterActionRejection{}, true
	}
}
```

Update `h2DirectResponseAdapter.WriteH2` to accept the wider signature:

```go
func (a *h2DirectResponseAdapter) WriteH2(_ context.Context, _ h2.H2Request, sw h2.StreamWriter) error {
	return a.a.writeH2(sw)
}
```

Update `h2RouterActionRejection.WriteH2` similarly.

- [ ] **Step 5: Write the failing tests for `routerActionH2.do` per SPEC §4.1 + §8.3**

In `actions_test.go`:

```go
// TestRouterActionH2_HappyPath — in-process h2 backend; assert :status + body forwarded.
// TestRouterActionH2_502OnDialFailure — cluster pointing at closed port; assert :status 502 + body bad gateway\n.
// TestRouterActionH2_502OnRoundTripProtocolError — backend emits malformed HEADERS; assert :status 502.
// TestRouterActionH2_CtxCancelEmitsRSTStreamCancel — cancel ctx mid-RoundTrip; assert RST(CANCEL) on the captured writer.
// TestRouterActionH2_Upstream5xxForwardedVerbatim — backend returns 503; assert downstream :status 503 + body forwarded (NOT 502).

// TestBuildRouterAction_PicksH2VariantByClusterUseH2 — build a route pointing at a UseH2()==true cluster; assert *routerActionH2.
//                                                       Build the same route pointing at UseH2()==false; assert *routerAction.
```

Each test uses a fake `h2.StreamWriter` that captures `:status` + Headers + Data + RST calls.

Run: `go test ./internal/filter/hcm/ -v -run "TestRouterActionH2_|TestBuildRouterAction_PicksH2"`
Expected: PASS once all the wiring lands.

- [ ] **Step 6: Append ADR-0058 to `docs/envoy-go/DECISIONS.md`**

ADR-0058 prose per `## ADRs introduced by this plan` summary. Cross-reference:
- `internal/filter/hcm/h2/client.go:RoundTrip` (the upstream-side observe-discard site)
- `internal/filter/hcm/h2/stream.go:dispatch` / `recvTrailingHeaders` (the downstream-side observe-discard site, unchanged from 05.1)
- The carry-forward subsection lists M-4 (`readClientPreface` ctx-unaware) and M-10 (`SETTINGS_TIMEOUT` absent) per SPEC §12.2 + the carry-forward resolution matrix above.

- [ ] **Step 7: Run full HCM + h2 + cluster tests + race-detector + lint**

```bash
go test -race ./internal/filter/hcm/ ./internal/filter/hcm/h2/ ./internal/cluster/ -v
go test ./test/conformance/h2spec/ -v
go vet ./...
golangci-lint run ./...
```

Expected: green; h2spec 53/53 PASS; no lint warnings.

- [ ] **Step 8: Append Task 11 entry to PROGRESS.md, commit**

```bash
git add internal/filter/hcm/h2/stream.go internal/filter/hcm/h2/stream_test.go internal/filter/hcm/actions.go internal/filter/hcm/actions_test.go internal/filter/hcm/config.go internal/filter/hcm/config_test.go internal/filter/hcm/h2dispatch.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: routerActionH2 + h2.Action widening + h2RouterActionAdapter + ADR-0058"
```

---

## Task 12: `test/helpers/h2.go` H2RoundTrip helper

**Files:**
- Create: `test/helpers/h2.go`
- Create: `test/helpers/h2_test.go`
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 12 entry)

The driver-side helper consumed by fixture 0004's driver (Task 14). Driver-side use of `golang.org/x/net/http2.Transport` is permitted per D-3.2 (test code, not envoy-go runtime). Per `## Settled SPEC §10 deferred decisions` #13: a fresh `*http2.Transport` per call (no caching).

- [ ] **Step 1: Write the failing test**

```go
package helpers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// TestH2RoundTrip_HappyPath verifies the helper against an in-process h2-over-TLS server.
func TestH2RoundTrip_HappyPath(t *testing.T) {
	// Stand up an httptest.NewUnstartedServer with TLS + http2.ConfigureServer
	// driver-side (test code, not runtime). Handler returns "ok" with status 200.
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	if err := http2.ConfigureServer(srv.Config, &http2.Server{}); err != nil {
		t.Fatalf("ConfigureServer: %v", err)
	}
	srv.TLS = &tls.Config{NextProtos: []string{"h2"}}
	srv.StartTLS()
	defer srv.Close()

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	tlsConf := &tls.Config{RootCAs: pool, NextProtos: []string{"h2"}, ServerName: "127.0.0.1"}

	addr := srv.Listener.Addr().String()
	status, _, body, err := H2RoundTrip(context.Background(), addr, tlsConf, "GET", "/", nil, nil)
	if err != nil {
		t.Fatalf("H2RoundTrip: %v", err)
	}
	if status != 200 {
		t.Errorf("status: got %d, want 200", status)
	}
	if string(body) != "ok" {
		t.Errorf("body: got %q, want %q", body, "ok")
	}
}
```

Run: `go test ./test/helpers/ -v -run TestH2RoundTrip`
Expected: FAIL — `H2RoundTrip` undefined.

- [ ] **Step 2: Implement `test/helpers/h2.go`**

```go
// Package helpers H2 driver — driver-side H2 round-tripper for fixture-0004.
//
// Per D-3.2: this file lives under test/, NOT under internal/. The forbidden
// runtime imports rule (no http2.Server/Transport in production code) does
// NOT apply here — this is fixture infrastructure, not envoy-go runtime.

package helpers

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// H2RoundTrip dials addr over TLS (with the given config + NextProtos h2),
// constructs a fresh *http2.ClientConn, issues one HTTP/2 request, returns
// (status, response-headers, response-body, error).
//
// A fresh *http2.Transport is constructed per call (no caching) so consecutive
// invocations from the same goroutine never reuse a *http2.ClientConn — required
// for fixture 0004's RR distribution determinism per ADR-0028 + the 05.2 PLAN
// "Settled SPEC §10 deferred decisions" #13.
//
// The returned respHeaders include the :status pseudo-header as the first element
// (the codec preserves wire order; pseudo-headers are first per RFC 9113 §8.3).
func H2RoundTrip(
	ctx context.Context,
	addr string,
	tlsConf *tls.Config,
	method, path string,
	headers []hpack.HeaderField,
	body []byte,
) (status int, respHeaders []hpack.HeaderField, respBody []byte, err error) {
	d := &net.Dialer{}
	rawConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("dial: %w", err)
	}
	tlsConn := tls.Client(rawConn, tlsConf)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return 0, nil, nil, fmt.Errorf("handshake: %w", err)
	}
	if alpn := tlsConn.ConnectionState().NegotiatedProtocol; alpn != "h2" {
		_ = tlsConn.Close()
		return 0, nil, nil, fmt.Errorf("alpn negotiated %q, want %q", alpn, "h2")
	}
	tr := &http2.Transport{TLSClientConfig: tlsConf}
	cc, err := tr.NewClientConn(tlsConn)
	if err != nil {
		_ = tlsConn.Close()
		return 0, nil, nil, fmt.Errorf("NewClientConn: %w", err)
	}
	defer func() { _ = cc.Close() }()
	// Construct *http.Request and round-trip via the standard library Transport surface.
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("https://%s%s", addr, path), bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("NewRequestWithContext: %w", err)
	}
	for _, hf := range headers {
		req.Header.Add(hf.Name, hf.Value)
	}
	resp, err := cc.RoundTrip(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("RoundTrip: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("read body: %w", err)
	}
	for k, vv := range resp.Header {
		for _, v := range vv {
			respHeaders = append(respHeaders, hpack.HeaderField{Name: k, Value: v})
		}
	}
	// Prepend :status pseudo-header for caller convenience.
	respHeaders = append([]hpack.HeaderField{{Name: ":status", Value: fmt.Sprintf("%d", resp.StatusCode)}}, respHeaders...)
	return resp.StatusCode, respHeaders, respBody, nil
}
```

(Note: `import "net/http"` is needed; the function constructs `*http.Request` for `cc.RoundTrip` — that's the `*http2.ClientConn.RoundTrip(*http.Request) (*http.Response, error)` shape, which is fine driver-side per D-3.2.)

Run: `go test ./test/helpers/ -v -run TestH2RoundTrip`
Expected: PASS.

- [ ] **Step 3: Run full helpers tests + lint**

```bash
go test ./test/helpers/ -v
golangci-lint run ./test/helpers/
```

Expected: green.

- [ ] **Step 4: Append Task 12 entry to PROGRESS.md, commit**

```bash
git add test/helpers/h2.go test/helpers/h2_test.go docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: test/helpers/h2.go H2RoundTrip helper"
```

---

## Task 13: Fixture 0004 — PKI + backends + bootstraps + expectations + README

**Files:**
- Create: `test/fixtures/0004-h2-routing/pki/gen/main.go`
- Create: `test/fixtures/0004-h2-routing/pki/*.pem` (9 generated artefacts)
- Create: `test/fixtures/0004-h2-routing/backends/main.go`
- Create: `test/fixtures/0004-h2-routing/envoy-go.yaml`
- Create: `test/fixtures/0004-h2-routing/envoy.yaml`
- Create: `test/fixtures/0004-h2-routing/expectations.yaml`
- Create: `test/fixtures/0004-h2-routing/README.md`
- Modify: `test/differential/fixture/fixture.go` (add `HTTPSH2` BackendKind)
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 13 entry)

Lands the static fixture content. Task 14 lands the driver + ADR-0057 + the runner blank-import. Splitting Task 13 from Task 14 keeps each task focused and testable; running the fixture (the differential gate) is gated on the driver landing.

- [ ] **Step 1: Write `test/fixtures/0004-h2-routing/pki/gen/main.go`**

Mirror `test/fixtures/0002-tls-tcp/pki/gen/main.go`'s structure. Generate:
- 1 self-signed CA cert + key (deterministic via fixed RSA seed, e.g. `rand.Reader` replaced by a fixture-local PRNG seeded from a constant).
- 1 listener server cert + key (SAN: `localhost`, `127.0.0.1`, `host.docker.internal`).
- 3 backend leaf certs + keys (each carrying SAN `localhost`, `127.0.0.1`, `host.docker.internal` so both subject and reference can validate against the same CA).

Total: 10 PEM files emitted under `pki/` (5 certs + 5 keys):

```
ca.pem
ca.key.pem
listener.pem
listener.key.pem
backend-0.pem
backend-0.key.pem
backend-1.pem
backend-1.key.pem
backend-2.pem
backend-2.key.pem
```

The generator's `go:generate` directive lives in `test/fixtures/0004-h2-routing/doc.go` (a tiny stub file that is not strictly required but provides a stable `go generate` target):

```go
// Package fixtures_0004 holds fixture data; PKI is generated via go:generate.
//go:generate go run ./pki/gen
package fixtures_0004
```

Run: `cd test/fixtures/0004-h2-routing && go run ./pki/gen` → produces the 10 PEMs.

- [ ] **Step 2: Commit the generated PEMs**

The 10 PEMs are committed (mirroring fixture-0002 discipline). They are deterministic (same generator + same constants → same bytes); CI does NOT run `go generate` automatically.

- [ ] **Step 3: Write `test/fixtures/0004-h2-routing/backends/main.go`**

```go
// Backend for fixture 0004. Listens on a TLS port advertising NextProtos=["h2"]
// via http2.ConfigureServer (driver-side; D-3.2 governs runtime, not test backends).
// Returns "OK\n" for /health, "backend-<idx>:<path-suffix>" for /api/v1/<n>,
// "not found\n" with status 404 for unknown paths.

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/http2"
)

func main() {
	port := flag.String("port", "0", "listen port")
	cert := flag.String("cert", "", "server cert PEM path")
	key := flag.String("key", "", "server key PEM path")
	flag.Parse()
	idx := os.Getenv("BACKEND_IDX")
	if idx == "" {
		idx = "?"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "OK\n")
	})
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/")
		_, _ = fmt.Fprintf(w, "backend-%s:v1/%s", idx, suffix)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found\n"))
	})

	tlsCert, err := tls.LoadX509KeyPair(*cert, *key)
	if err != nil {
		log.Fatalf("load cert: %v", err)
	}
	srv := &http.Server{
		Addr:    ":" + *port,
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			NextProtos:   []string{"h2"},
		},
	}
	if err := http2.ConfigureServer(srv, &http2.Server{}); err != nil {
		log.Fatalf("ConfigureServer: %v", err)
	}
	log.Printf("backend %s listening on %s", idx, srv.Addr)
	if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ListenAndServeTLS: %v", err)
	}
}
```

- [ ] **Step 4: Write `envoy-go.yaml` (subject)**

Mirror `test/fixtures/0003-http11-routing/envoy-go.yaml`'s shape with these additions:
- Listener: `transport_socket: tls` carrying the fixture PKI + `alpn_protocols: ["h2", "http/1.1"]`.
- HCM: `codec_type: AUTO` (so ALPN drives selection per-conn; the driver advertises `h2` only).
- 3 routes: `/health` direct_response 200 body `OK\n`; `/api` (prefix) → cluster `c_h2_backend`; `/missing` direct_response 404 body `not found\n`.
- Cluster `c_h2_backend`: STATIC, three TLS endpoints carrying `transport_socket: tls` + `alpn_protocols: ["h2"]` + `validation_context: { trusted_ca: { filename: "../pki/ca.pem" } }` (relative to the bootstrap file's location). The cluster has `typed_extension_protocol_options` carrying:

```yaml
typed_extension_protocol_options:
  envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
    "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
    explicit_http_config:
      http2_protocol_options: {}
```

The cluster's `lb_policy` is `ROUND_ROBIN` (per ADR-0024).

- [ ] **Step 5: Write `envoy.yaml` (reference)**

Same listener + HCM + route shape. Cluster `c_h2_backend`: STRICT_DNS, three endpoints `host.docker.internal:<port>` per ADR-0010 (`dns_lookup_family: V4_ONLY`); same `transport_socket` shape; same `typed_extension_protocol_options.HttpProtocolOptions` shape.

- [ ] **Step 6: Write `expectations.yaml` (prose form per the M-6 carry-forward)**

Per fixture-0003 prose-form precedent: list the 27 sequential requests, the per-side `[3,3,3]` distribution rule, the allow-listed headers (`Date`, `Server`, `Content-Type`, `Content-Length`, `x-envoy-*`, `x-forwarded-*`, `x-request-id`), the H2-pseudo-header presence rule.

- [ ] **Step 7: Write `README.md`**

Per the File Structure entry above. Document the fixture's purpose, the STATIC-vs-STRICT_DNS divergence (ADR-0027), the ALPN-h2 e2e shape, the upstream-TLS closure of ADR-0035 H2 leg via ADR-0057, the `--concurrency 1` reference pin (ADR-0028), the per-side `[3,3,3]` RR rule, the PKI regeneration procedure (`cd test/fixtures/0004-h2-routing && go run ./pki/gen`).

- [ ] **Step 8: Add `HTTPSH2` BackendKind to the harness**

In `test/differential/fixture/fixture.go` (or wherever `BackendKind` is defined — verify at execution time):

```go
const (
	// ... existing kinds ...
	HTTPSH2 BackendKind = "https_h2"
)
```

Update the harness's backend-startup routine (`test/differential/harness.go` or similar) to spawn `test/fixtures/0004-h2-routing/backends/main.go` when the fixture's `BackendKind() == HTTPSH2`. Pass `--cert` / `--key` paths from the fixture-local `pki/` directory + `BACKEND_IDX` env var.

Run: `go test -count=1 -run TestDifferential -v ./test/differential/...`  (without the driver registered — the runner discovers no driver for fixture 0004 yet; that's Task 14)
Expected: 0000/0001/0002/0003 fixtures still green; fixture 0004 absent from the suite (no driver registered yet).

- [ ] **Step 9: Append Task 13 entry to PROGRESS.md, commit**

```bash
git add test/fixtures/0004-h2-routing/pki/ test/fixtures/0004-h2-routing/backends/ test/fixtures/0004-h2-routing/envoy-go.yaml test/fixtures/0004-h2-routing/envoy.yaml test/fixtures/0004-h2-routing/expectations.yaml test/fixtures/0004-h2-routing/README.md test/fixtures/0004-h2-routing/doc.go test/differential/fixture/fixture.go test/differential/harness.go docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: fixture 0004 PKI + backends + bootstraps + expectations"
```

---

## Task 14: Fixture 0004 driver + runner blank-import + ADR-0057 (closes ADR-0035 H2 leg)

**Files:**
- Create: `test/fixtures/0004-h2-routing/driver/driver.go`
- Create: `test/fixtures/0004-h2-routing/driver/driver_test.go`
- Modify: `test/differential/runner_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0057)
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (append Task 14 entry)

Lands the driver, registers the fixture, and runs the differential gate end-to-end. ADR-0057 closes ADR-0035's H2 leg as the driver's first invocation observes the proxy → backend HTTPS h2 surface.

- [ ] **Step 1: Write `test/fixtures/0004-h2-routing/driver/driver.go`**

```go
// Package driver registers the 0004-h2-routing fixture. Mirrors fixture-0003's
// driver shape with HTTPS h2 (vs HTTP/1.1 plaintext) and per-side [3,3,3]
// distribution assertion per ADR-0024 + the 05.2 PLAN "Settled SPEC §10
// deferred decisions" #3 (per-Cluster RR scope retained).
package driver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const fixtureName = "0004-h2-routing"

func init() {
	fixture.RegisterFixture(fixtureName, &h2Driver{})
}

type h2Driver struct{}

func (h2Driver) BackendCount() int                { return 3 }
func (h2Driver) BackendKind() fixture.BackendKind { return fixture.HTTPSH2 }
func (h2Driver) SubjectListenerName() string      { return "l_h2" }
func (h2Driver) ReferenceListenerPort() int       { return 15004 }

func (h2Driver) ReferenceBootstrap(backendPorts []int) string {
	return fmt.Sprintf(referenceTmpl, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (h2Driver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return fmt.Sprintf(subjectTmpl, subjAdminPort, subjListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

// drive issues 27 sequential H2 requests against addr (the proxy listener)
// and returns the concatenated 9 /health response bodies. Per ADR-0028 +
// ADR-0056: each request opens a fresh *http2.ClientConn (the helper's
// fresh-Transport-per-call discipline). The 9 /api response bodies are NOT
// concatenated (per-side RR offset may differ; routing correctness is
// covered by AssertDistribution). The 9 /missing bodies are NOT concatenated
// (404 body relaxed).
func (h2Driver) drive(ctx context.Context, addr string, tlsConf *tls.Config) ([]byte, error) {
	var out strings.Builder
	for n := 0; n < 9; n++ {
		status, _, body, err := helpers.H2RoundTrip(ctx, addr, tlsConf, "GET", "/health", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/health[%d]: %w", n, err)
		}
		if status != 200 {
			return nil, fmt.Errorf("/health[%d]: status=%d, want 200", n, status)
		}
		out.Write(body)
	}
	for n := 0; n < 9; n++ {
		status, _, _, err := helpers.H2RoundTrip(ctx, addr, tlsConf, "GET", fmt.Sprintf("/api/v1/%d", n), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/api/v1/%d: %w", n, err)
		}
		if status != 200 {
			return nil, fmt.Errorf("/api/v1/%d: status=%d, want 200", n, status)
		}
	}
	for n := 0; n < 9; n++ {
		status, _, _, err := helpers.H2RoundTrip(ctx, addr, tlsConf, "GET", fmt.Sprintf("/missing/%d", n), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/missing/%d: %w", n, err)
		}
		if status != 404 {
			return nil, fmt.Errorf("/missing/%d: status=%d, want 404", n, status)
		}
	}
	return []byte(out.String()), nil
}

// loadTLSConfig builds a TLS config that trusts the fixture-local CA.
func (h2Driver) loadTLSConfig(_ context.Context, _ string) (*tls.Config, error) {
	caPEM, err := os.ReadFile(fixturePath("pki/ca.pem"))
	if err != nil {
		return nil, fmt.Errorf("read CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("parse CA")
	}
	return &tls.Config{RootCAs: pool, NextProtos: []string{"h2"}, ServerName: "localhost"}, nil
}

func (d h2Driver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	tlsConf, err := d.loadTLSConfig(ctx, addr)
	if err != nil {
		return nil, err
	}
	return d.drive(ctx, addr, tlsConf)
}

func (d h2Driver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	tlsConf, err := d.loadTLSConfig(ctx, addr)
	if err != nil {
		return nil, err
	}
	return d.drive(ctx, addr, tlsConf)
}

func (h2Driver) AssertDistribution(refCounts, subjCounts []uint64) error {
	want := []uint64{3, 3, 3}
	if len(subjCounts) != 3 {
		return fmt.Errorf("subj backend count: got %d, want 3", len(subjCounts))
	}
	for i, c := range subjCounts {
		if c != want[i] {
			return fmt.Errorf("subj backend %d: got %d, want %d (RR [3,3,3] expected)", i, c, want[i])
		}
	}
	if len(refCounts) != 3 {
		return fmt.Errorf("ref backend count: got %d, want 3", len(refCounts))
	}
	for i, c := range refCounts {
		if c != want[i] {
			return fmt.Errorf("ref backend %d: got %d, want %d (RR [3,3,3] expected)", i, c, want[i])
		}
	}
	return nil
}

// fixturePath resolves a path relative to the fixture's directory.
// (The harness already provides this; reuse the existing helper if available;
// otherwise add it as a small driver-local helper.)
func fixturePath(rel string) string {
	// ... resolve relative to the fixture directory; mirror fixture-0002/0003 pattern ...
	return rel
}

// referenceTmpl + subjectTmpl are the raw YAML strings, %d-format-arg-shaped
// for the three backend ports + admin port. They are loaded from
// envoy.yaml / envoy-go.yaml at build time via a //go:embed directive.
var (
	referenceTmpl = `...`
	subjectTmpl   = `...`
)
```

(The `referenceTmpl` and `subjectTmpl` strings are loaded via `//go:embed envoy.yaml` and `//go:embed envoy-go.yaml`; the format strings have positional `%d` for the three backend ports + the admin port, mirroring fixture-0003's pattern.)

- [ ] **Step 2: Write `test/fixtures/0004-h2-routing/driver/driver_test.go`**

Mirror fixture-0003's `driver_test.go`:

```go
func TestH2Driver_AssertDistribution(t *testing.T) {
	d := h2Driver{}
	cases := []struct {
		name      string
		refCounts []uint64
		subjCounts []uint64
		wantErr   bool
	}{
		{"both [3,3,3]", []uint64{3, 3, 3}, []uint64{3, 3, 3}, false},
		{"subj [4,3,2]", []uint64{3, 3, 3}, []uint64{4, 3, 2}, true},
		{"ref [4,3,2]", []uint64{4, 3, 2}, []uint64{3, 3, 3}, true},
		{"subj count mismatch", []uint64{3, 3, 3}, []uint64{3, 3}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := d.AssertDistribution(tc.refCounts, tc.subjCounts)
			if (err != nil) != tc.wantErr {
				t.Errorf("AssertDistribution: err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
```

Run: `go test ./test/fixtures/0004-h2-routing/driver/ -v -short`
Expected: PASS for the unit test (the differential gate runs in Step 5 below; not via `-short`).

- [ ] **Step 3: Add the blank import to `test/differential/runner_test.go`**

```go
import (
	// ... existing ...
	_ "github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver"
)
```

- [ ] **Step 4: Append ADR-0057 to `docs/envoy-go/DECISIONS.md`**

ADR-0057 prose per `## ADRs introduced by this plan` summary. Cross-reference:
- `test/fixtures/0004-h2-routing/` (the new fixture closing ADR-0035 H2 leg)
- ADR-0035 (predecessor; the H1 leg remains open under ADR-0035)
- The "phase-05.2-follow-up" tag pointing at the H1+TLS upstream gap

- [ ] **Step 5: Run the differential gate against fixture 0004**

```bash
go test -count=1 -run TestDifferential/0004-h2-routing -v ./test/differential/
```

Expected: PASS — the 27-request workload completes; per-side `[3,3,3]` distribution holds; `:status` per request matches between subject and reference; decoded body equivalence on `/health`.

If the test fails on the first run, common causes:
- PKI SAN mismatch — backend cert doesn't include `host.docker.internal` (regenerate via Step 1 of Task 13).
- Subject's HCM `codec_type: AUTO` not picking H2 — verify `alpn_protocols: ["h2", "http/1.1"]` is set on the listener, and the driver advertises only `h2`.
- Cluster build error — `manager_test.go` cases (Task 10) should have caught this, but if a YAML drift slipped, fix the bootstrap.
- Backend not reachable — check `--port` flag plumbing in the harness's backend-startup.

- [ ] **Step 6: Re-run all gates as a sanity check**

```bash
go test -count=1 -run TestDifferential -v ./test/differential/
go test ./test/conformance/h2spec/ -v
go test -race ./internal/filter/hcm/h2/ -v
```

Expected: all four pre-existing fixtures + fixture 0004 GREEN; h2spec 53/53 PASS; h2 unit tests + race-detector clean.

- [ ] **Step 7: Append Task 14 entry to PROGRESS.md, commit**

```bash
git add test/fixtures/0004-h2-routing/driver/ test/differential/runner_test.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: fixture 0004 driver + runner registration + ADR-0057 (closes ADR-0035 H2 leg)"
```

---

## Task 15: BEHAVIOR_CONTRACT in-place edit (per ADR-0052) + all-gates green local sweep + STATE/ROADMAP advancement

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (in-place per ADR-0052)
- Modify: `docs/envoy-go/STATE.md` (advance to lifecycle-state 4 — verification-pending)
- Modify: `docs/envoy-go/ROADMAP.md` (row 05.2 → done; row 05 → done)
- Modify: `docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md` (Task 15 entry + verification block)

The closing task. Edits the SCAFFOLD subsection in place per ADR-0052 (NOT a supersession), runs the six-gate local sweep, advances STATE for the next-fresh verification session.

- [ ] **Step 1: Edit `BEHAVIOR_CONTRACT.md` ## HTTP/2 in place**

Per the SPEC §5.7 prose. The subsection at lines ~267-315 is rewritten:

```markdown
## HTTP/2

*Introduced by phase 05.1. Justified by ADR-0046 (codec source: x/net/http2.Framer + hpack), ADR-0047 (server settings defaults), ADR-0048 (server connection manager from scratch), ADR-0050 (ALPN dispatch wiring), ADR-0051 (h2spec threshold + pin), ADR-0052 (this subsection — SCAFFOLD form for 05.1, in-place edited for 05.2).*

*Extended by phase 05.2. Justified by ADR-0055 (flow-control discipline), ADR-0056 (per-request fresh upstream H2 dial), ADR-0057 (closes ADR-0035 H2 leg via fixture 0004), ADR-0058 (trailers observed but not forwarded; carry-forwards M-4 + M-10).*

Phase 05.1 introduced envoy-go's downstream HTTP/2 dataplane; phase 05.2 closes the dataplane on the upstream side: cluster-side HttpProtocolOptions parsing, Cluster.DialH2 + ClientConn + RoundTrip, routerActionH2 action variant, and the project's first full-stack HTTPS h2 differential fixture (0004-h2-routing) closing ADR-0035 H2 leg. The flow-control discipline tightening per ADR-0055 makes the codec primitives load-bearing for realistic H2 workloads.

### Asserted equivalence (05.1 + 05.2 scope)

- `:status` per request: required + asserted on every fixture-0004 request (h2spec section 8 covers indirect coverage on the codec; fixture-0004 covers the full proxy+upstream surface).
- Decoded body bytes on `direct_response` 2xx paths: byte-equal to the configured body string. Witnessed by fixture 0004's 9 `/health` requests on both sides + envoy-go's hcm-package unit tests.
- Per-stream response header set-equality modulo allow-list: `:status` (required + asserted), `Server` (matched verbatim with upstream's `envoy` per ADR-0014), `Content-Type`, `Content-Length`, `Date` (presence required; value not byte-compared). NEW (05.2): routed-to-upstream H2 responses now in scope — `:status` required + asserted; `Server`/`Content-Type`/`Content-Length`/`Date` headers from the upstream backend forwarded verbatim (no router-injected headers); the per-stream response header set-equality between sides asserted modulo the same allow-list.
- NEW (05.2): routed-to-upstream H2 request preservation — `:method`/`:path`/`:scheme`/`:authority` forwarded verbatim from downstream to upstream (witnessed by the in-process backend in fixture 0004's tests asserting on received pseudo-headers). The path normalisation discussed in master phase-05 SPEC §5.7 is empty on the H2 side (the path is the bytes of the `:path` pseudo-header — there's no stdlib net/http parsing to inject normalisations).
- NEW (05.2): route-match selection equivalence on H2 — same method + path → same matched route on both proxies (witnessed indirectly by per-side `[3,3,3]` distribution + `:status` per request).
- NEW (05.2): per-cluster RR distribution `[3, 3, 3]` per side over the 9 router-action requests (local-correctness; cross-side sequence is NOT asserted, mirroring phase-04's relaxation per ADR-K extended to H2).
- NEW (05.2): ALPN selection equivalence at the differential level — a downstream client advertising only `h2` reaches the H2 driver on both proxies (witnessed by fixture 0004's `:status 200` on every routed response).

### Not asserted (05.1 + 05.2 scope)

- Wire-byte H2 framing — unchanged from 05.1.
- SETTINGS values byte-for-byte — unchanged from 05.1.
- WINDOW_UPDATE timing or count — unchanged from 05.1; ADR-0055's tightening adds frame counts that depend on body size and peer window behaviour, which are inherently non-deterministic across the two proxies.
- Stream id allocation pattern — unchanged from 05.1.
- Trailers — observed but not forwarded per ADR-0058 (formalises the upstream-side discard rule; the 05.1 server-side rule was already trailers-not-forwarded).
- 0-RTT TLS early-data behaviour — unchanged from 05.1.
- NEW (05.2): connection re-use upstream (per ADR-0056) — Envoy pools, envoy-go does not; both produce the same per-request `:status`/body output; cross-conn frame counts differ.
- NEW (05.2): cross-side request body bytes for routed-to-upstream requests (mirror of phase-04's ADR-K relaxation extended to H2) — fixture 0004's 9 `/api` request bodies are bodyless GETs, so this rule is unexercised in 05.2; carried forward as the rule for any future POST/PUT-bearing fixture.

### Header allow-list extensions

See the `## Header allow-list` table above. Rows added by ADR-0052: `:status` (active in 05.1; locally-generated H2 responses), `:method`/`:path`/`:scheme`/`:authority` — phase 05.2 flips the latter four rows from "applies-to: phase 05.2 (forward-looking)" to **"applies-to: phase 05.2 routed-to-upstream H2 (active per ADR-0057)"**.

### h2spec threshold

Sections 3, 4, 5, 6 (excluding 6.6 PUSH_PROMISE), 7, 8 — all `failed == 0`. Pin: `summerwind/h2spec` at the SHA recorded in `CONFORMANCE_PINS.md` per ADR-0051.

**ADR-0055 prose extension (phase 05.2):** the from-scratch H2 codec respects `MaxFrameSize` chunking on outbound DATA, per-stream send-window enforcement, inbound WINDOW_UPDATE emission on a half-window high-water threshold, and overflow bounds-checks on WINDOW_UPDATE deltas. These properties are validated by the regression unit tests (per phase-05.2 PLAN Tasks 2-5) and by the existing h2spec section 5/6 coverage at the pinned SHA; no new section requirements are added.

### Applies to (05.1 + 05.2)

- Phase-05.1: `internal/filter/hcm/h2/` server-side codec (unchanged); the codec-neutral `directResponseAction` factoring; the `--allow-h2c` test-only flag; the conformance suite under `test/conformance/h2spec/`.
- Phase-05.2: `internal/filter/hcm/h2/client.go` (`ClientConn` + `RoundTrip` + `Close`); `internal/cluster/dial_h2.go`; `internal/cluster/manager.go HttpProtocolOptions` reader + validation; `Cluster.UseH2()` accessor; `routerActionH2` action variant in `internal/filter/hcm/actions.go`; fixture `0004-h2-routing` (full-stack HTTPS h2); `test/helpers/h2.go H2RoundTrip` helper.

### Does not yet apply to

- HTTP/3 (later).
- Server push (out of scope permanently in 05.1; potentially out of scope project-wide).
- gRPC framing.
- Trailer forwarding (deferred to phase 07 framework + gRPC family per ADR-0058).
- Upstream H2 stream pooling (upstream-robustness family per ADR-0056).
- h2c production fixtures (test-only path).
- mTLS over h2 (deferred).
- Mixed-codec clusters (a single cluster used by both H1 and H2 listeners — load-balancing family or a future phase explicitly adding mixed-codec clusters).

(REMOVED from this list — now active: routed-to-upstream H2 → active per ADR-0057; fixture 0004 → active per phase-05.2 Task 14.)
```

Also flip the `## Header allow-list` table rows for `:method`/`:path`/`:scheme`/`:authority` (lines ~40-44) — change the "applies-to" cell from "phase 05.2 routed-to-upstream H2 (forward-looking)" to "phase 05.2 routed-to-upstream H2 (active per ADR-0057)".

- [ ] **Step 2: Run all six gates locally**

```bash
# Gate (a): new differential fixture green (NON-VACUOUS for the first time on H2).
go test -count=1 -run TestDifferential/0004-h2-routing -v ./test/differential/

# Gate (b): all pre-existing fixtures still green.
go test -count=1 -run TestDifferential -v ./test/differential/

# Gate (c): h2spec conformance.
go test ./test/conformance/h2spec/ -v

# Gate (d): fuzzers.
go test -fuzz=FuzzFrameStream -fuzztime=30s ./internal/filter/hcm/h2/
go test -fuzz=FuzzHPACKDecode -fuzztime=30s ./internal/filter/hcm/h2/
go test -fuzz=FuzzBootstrapLoad -fuzztime=30s ./internal/bootstrap/
go test -fuzz=FuzzTcpProxyFilter -fuzztime=30s ./internal/filter/tcpproxy/
go test -fuzz=FuzzTLSContextParse -fuzztime=30s ./internal/tls/
go test -fuzz=FuzzHCMConfigParse -fuzztime=30s ./internal/filter/hcm/

# Gate (e): vet/lint/test.
go vet ./...
golangci-lint run ./...
go test -race ./...
```

Expected: every gate green.

- [ ] **Step 3: ADR-0046 boundary grep**

```bash
grep -nR '"golang.org/x/net/http2"' internal/ cmd/envoy-go/main.go --include='*.go' | grep -v '_test.go' | grep -v 'internal/filter/hcm/h2/framer.go\|internal/filter/hcm/h2/hpack.go\|internal/filter/hcm/h2/settings.go\|internal/filter/hcm/h2/conn.go\|internal/filter/hcm/h2/client.go'
```

Expected: empty output (no production-code imports of `golang.org/x/net/http2` outside the five allowed files: framer.go / hpack.go / settings.go / conn.go / client.go — the latter NEW in 05.2; the prior four per ADR-0054's correction of ADR-0046).

- [ ] **Step 4: ADR-0048 boundary check (client.go now exists)**

```bash
ls internal/filter/hcm/h2/client.go
```

Expected: file exists. (Previously absent in 05.1 per ADR-0048's reservation; 05.2 lands it per the same ADR's forward-looking note.)

- [ ] **Step 5: Forbidden-runtime-imports check**

```bash
grep -nR 'http2.Server\|http2.Transport\|http2.ConfigureServer\|http2.Server.ServeConn\|http2.Transport.NewClientConn' internal/ cmd/envoy-go/main.go --include='*.go' | grep -v '_test.go'
```

Expected: no production-code hits (the three textual mentions in `internal/filter/hcm/h2/doc.go` are inside the package's prohibition statement; tolerable per the 05.1 REVIEW's existing acceptance).

- [ ] **Step 6: Update STATE.md to lifecycle-state 4 (verification-pending)**

```yaml
- active-phase: 05.2-upstream-h2
- phase-directory: docs/envoy-go/phases/05.2-upstream-h2/
- lifecycle-state: 4   # implementation complete; verification not yet run
- next-skill: superpowers:verification-before-completion
- next-skill-scope: <verify all six gates per BOOTSTRAP §7.5 / SPEC §3>
- last-commit: <Task 15 commit SHA>
- last-updated: <date>
```

- [ ] **Step 7: Update ROADMAP.md row 05.2 to `done` AND row 05 to `done`**

Per SPEC §4.4: at the phase-done commit, both rows flip. 05.1 is already `done` at `bc4fca4`; 05.2's phase-done commit closes both rows (parent 05 + sub-phase 05.2).

```markdown
| 05 | http-2 | 04 | done |  | HTTP/2 downstream + upstream (low-level framer, own conn mgr). HTTP/2 fixture green; `h2spec` above threshold. Split planner-time per ADR-0045. |
| 05.1 | downstream-h2 | 04 | done |  | Downstream HTTP/2 termination (own framer, own ServerConn, ALPN dispatch); first non-vacuous conformance gate `h2spec` (53/53 PASS at the ADR-0051 pin); `--allow-h2c` test-only flag; `CONFORMANCE_PINS.md`; `BEHAVIOR_CONTRACT.md ## HTTP/2` scaffold. No new differential fixture. |
| 05.2 | upstream-h2 | 05.1 | done |  | Upstream HTTP/2 origination (`Cluster.DialH2`, own `ClientConn`, `routerActionH2`, cluster `HttpProtocolOptions` parsing); fixture `0004-h2-routing` (full HTTPS h2 end-to-end); closes ADR-0035 H2 leg; extends `BEHAVIOR_CONTRACT.md ## HTTP/2` with upstream + fixture-0004 rules; ADR-0055 flow-control discipline tightening (carries 05.1 REVIEW Importants). |
```

NOTE: per BOOTSTRAP §5 step 6, the phase-done commit (lifecycle-state 6) is owned by the REVIEW session, NOT by Task 15. Task 15 advances STATE to lifecycle-state 4 (implementation complete; verification pending). The verification session re-runs the gates (lifecycle-state 5) and the REVIEW session writes REVIEW.md (lifecycle-state 6 — phase-done). The ROADMAP row updates above are PRE-VERIFIED at Task 15 only IF the REVIEW also approves — re-state in Task 15's PROGRESS entry that the ROADMAP `done` flips are anticipatory and would land at the REVIEW session's phase-done commit.

**Refinement: Task 15 advances STATE to lifecycle-state 4 only.** ROADMAP row updates are deferred to the phase-done commit per BOOTSTRAP §5 step 6. The PROGRESS Task 15 entry records "ROADMAP rows 05.2 + 05 still `in-progress` at this commit; the phase-done commit at lifecycle-state 6 will flip both."

- [ ] **Step 8: Append Task 15 entry to PROGRESS.md (with verification block)**

The PROGRESS Task 15 entry is the session's "verification proof" — `superpowers:verification-before-completion` reads it when phase 05.2 moves to lifecycle-state 5. Keep every last-30-lines block verbatim. Mirror phase-04's Task 17 / phase-05.1's Task 16 PROGRESS shape.

The entry includes:
- Each gate's command + last-30-lines output verbatim.
- ADR-0046 boundary grep result.
- ADR-0048 client.go absence-or-presence (now present).
- Forbidden-runtime-imports grep result.
- The carry-forward triage log (which 05.1 REVIEW findings are RESOLVED-IN-05.2 vs DEFERRED, per the matrix in the PLAN).

- [ ] **Step 9: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/STATE.md docs/envoy-go/phases/05.2-upstream-h2/PROGRESS.md
git commit -m "phase 05.2: BEHAVIOR_CONTRACT in-place edit + all-gates green local sweep (a/b/c/d/e green; f deferred to REVIEW)"
```

- [ ] **Step 10: Confirm phase-05.2 readiness for state-5 transition (do NOT advance STATE — that's the verification session per BOOTSTRAP §5)**

The implementation session ends with Task 15 committed on `phase/05.2-upstream-h2-impl`. STATE advancement through 5 → 6 is per-session work, not this task's responsibility.

---

## Post-plan handoff: state advancement + worktree cleanup (session-exit duties)

This section is the plan-authoring session's exit contract, not an executable task.

After the executing session commits Task 15 on `phase/05.2-upstream-h2-impl`:

1. **Fast-forward merge to master.** Per ADR-0003:
   ```bash
   cd /home/esa/git/envoy-go   # master worktree
   git merge --ff-only phase/05.2-upstream-h2-impl
   ```
2. **The verification session** (next-fresh from the implementation session) re-runs all six gates per BOOTSTRAP §7.5 and advances STATE to lifecycle-state 5 with `next-skill: superpowers:requesting-code-review`. Verification commits `phase 05.2: STATE.md → lifecycle-state 5` on master.
3. **The REVIEW session** (next-fresh from verification) writes `docs/envoy-go/phases/05.2-upstream-h2/REVIEW.md` per BOOTSTRAP §5 state 5 → 6. The REVIEW session's phase-done commit flips ROADMAP row 05.2 → `done` AND row 05 (parent) → `done` AT THE SAME COMMIT (per SPEC §4.4; 05.1 is already `done` at `bc4fca4`). Phase-05.2 ROADMAP advancement, parent 05 closure, and STATE handoff to phase 06 (active-phase: 06-observability-baseline; lifecycle-state: 1; next-skill: superpowers:brainstorming) all land in the phase-done commit.

**No part of this section is done by Task 15.** It lives here so the plan-authoring session knows where to leave STATE after its own commit, and so the executing session has clear context for its exit.

This plan-authoring session's own exit contract:

1. After plan-document-reviewer approves (`## Plan review loop` below), commit `PLAN.md` on `phase/05.2-upstream-h2-plan`.
2. Update `docs/envoy-go/STATE.md` on the same branch: `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development`, `next-skill-scope: <execute PLAN.md>`, `last-commit: <PLAN.md commit SHA>`.
3. Fast-forward `master` to `phase/05.2-upstream-h2-plan` per ADR-0003.
4. Exit clean.

---

## Plan review loop (invoked at end of plan-authoring session)

Per `superpowers:writing-plans` and ADR-0005: after this PLAN.md is written, dispatch the `plan-document-reviewer` subagent with the PLAN.md path + the SPEC.md path. If the reviewer returns approved → commit PLAN.md + STATE advancement (state 2 → state 3 on master). If the reviewer returns changes-requested → address feedback in place, re-dispatch (max 3 iterations per ADR-0005 + skill guidance); on iteration 3 without approval, exit blocked per `BOOTSTRAP_PROMPT.md` §5 deviations.

The reviewer's scope:

- Does the PLAN cover every SPEC §4 deliverable? (`internal/filter/hcm/h2/client.go`; `internal/cluster/dial_h2.go`; `Cluster.UseH2()`; `internal/cluster/manager.go HttpProtocolOptions` reader + validation; blank imports in `cluster.go` + `bootstrap.go`; `routerActionH2`; `filter.go` build-time variant selection; `h2dispatch.go` adapter; `h2.Action` widening; the seven ADR-0055 fixes; `test/helpers/h2.go`; fixture 0004 in full; runner registration; monotonic-id-reuse integration test; M-8 cleanup; `BEHAVIOR_CONTRACT.md ## HTTP/2` in-place edit; four ADRs ADR-0055..ADR-0058; phase-05.1 REVIEW carry-forward.)
- Does the PLAN settle every 05.2-scoped SPEC §10 deferred decision? (10 items — see `## Settled SPEC §10 deferred decisions`.)
- Does the PLAN mitigate every SPEC §11 risk with a task-level step or an ADR? (11.1 phase-splitting → `## Scope check`; 11.2 ADR-0055 regression → Tasks 2-5 regression tests + h2spec re-run on every task; 11.3 ALPN-confirm race → `tlsConn.HandshakeContext(ctx)` defensive call in Task 9; 11.4 per-request-fresh-dial latency → ADR-0056 explicit; 11.5 PKI generation determinism → `pki/gen/main.go` deterministic seeds + committed PEMs (Task 13); 11.6 `auto_config` silent-ignore narrowing → SPEC §5.5 documented + Task 10 unit-test coverage; 11.7 ADR-0055 bundle vs per-fix ADR → Tasks 2-5 atomic fixes + bundle ADR; 11.8 trailer asymmetry vs Envoy → ADR-0058 + fixture-0004 doesn't exercise; 11.9 502 prose divergence H1 vs H2 → `bad502Body` shared constant in Task 11.)
- Does the PLAN resolve phase-05.1 REVIEW Important + Minor findings triaged in SPEC §12? (8 absorbed into ADR-0055 + Task 6 + Task 6's M-8 cleanup; 3 deferred via ADR-0058 carry-forward + PROGRESS-only; 1 integration test gap landed at Task 6.)
- Are tasks atomic (one logical commit each, 2–5 minutes per step except the well-annotated longer ones — Task 8 RoundTrip body, Task 13 fixture infrastructure, Task 14 driver + ADR-0057, Task 15 final sweep)?
- Does the ADR number sequence match the verified DECISIONS.md tail? (ADR-0054 → ADR-0055..0058; non-monotonic mapping by topic-vs-first-use-order documented above.)
- Is the LoC estimate honest and does the scope-check argument hold? (Per `## Scope check`: ~1500 LoC, 15 tasks, no further coherent split axis exists; per phase-04 / 05.1 precedent, one-sub-phase shipment is correct.)
- Are spec-review advisory items addressed? (Three planner-time items in `## Spec-review advisory responses`.)
- Does the import topology stay clean? (The 05.1 boundary `hcm → h2 only` is preserved; the new `routerActionH2` lives in `hcm` package; `h2` does NOT gain any imports of `hcm`. `h2.Action` is widened in place; no parallel interface.)
- Are the ADR-0046 boundary grep + ADR-0048 client.go presence checks codified in Task 15's gate sweep? (Yes — Steps 3 + 4.)
- Does the PLAN preserve the 05.1 BEHAVIOR_CONTRACT shape during the in-place edit, OR is the rewritten subsection a clean superset? (The Task 15 prose flips deferred items to active items + adds new bullets; no 05.1 prose is removed except the deferred-to-05.2 bullets that are now active. The "Does not yet apply to" enumeration removes ONLY the items that 05.2 closes.)
- Are the four ADRs internally consistent? (ADR-0055's bundling rationale matches SPEC §11.7; ADR-0056 mirrors ADR-0039 + resolves ADR-0053's "phase-05.2-will-repeat-the-pattern" note; ADR-0057's H1+TLS carry-forward tag is explicit; ADR-0058's carry-forward subsection enumerates M-4 + M-10 dispositions.)

