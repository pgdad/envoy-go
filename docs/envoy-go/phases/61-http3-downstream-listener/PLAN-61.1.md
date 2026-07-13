# Phase 61.1 Implementation Plan — `http3-quic-substrate`: the QUIC/UDP downstream listen substrate + the FIRST new go.mod module (`github.com/quic-go/quic-go` v0.54.1) — a UDP-bound QUIC accept path parallel to the TCP `net.Listen` path, completing the QUIC/`h3`-ALPN/TLS-1.3 handshake (NO HTTP serving yet — that is 61.2). The FIRST of the confirmed 61.1/61.2/61.3 split (SPEC-61 §3.0); ANCHORS **ADR-0279**. Row 61 STAYS `in-progress`.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). Subagents auto-commit per CLAUDE.md — the controller verifies each commit, cleans leak files, squashes at stage-close, re-runs the suite on the frozen HEAD (`feedback_subagent_autocommit_claudemd`).

**Goal:** Stand up envoy-go's FIRST UDP/QUIC downstream listen path. An operator configures a `Listener` with `udp_listener_config.quic_options` + a `filter_chain` whose transport socket is `envoy.transport_sockets.quic` (a `QuicDownstreamTransport` wrapping a `DownstreamTlsContext` with ALPN `h3` + a server cert). envoy-go binds a UDP socket (`net.ListenUDP`), stands a quic-go listener over it, and accepts a QUIC connection whose TLS handshake negotiates ALPN `h3` and TLS 1.3 — proven by a subject-side integration test in which a local quic-go client completes the handshake against the bound listener. NO HTTP request is decoded or served (that is leg 61.2); the leg's proven capability is "a QUIC listener that binds and completes the `h3`-ALPN handshake."

**Architecture:** A discriminated `kind` field on `listenerRuntime` (`kindTCP` default / `kindQUIC`) set from `udp_listener_config.quic_options` presence at build time. `buildListenerRuntimeWithCtx` decodes a QUIC listener's transport socket via a NEW `internal/tls.NewQUICDownstreamConfig` (unwraps `QuicDownstreamTransport` → the inner `DownstreamTlsContext` → the SHARED `commonTLSContextToConfig`, so ALPN + cert handling is reused byte-for-byte), and enforces the mandatory-TLS + strict-reject config-parity arms. `Start` branches on `kind`: TCP keeps `net.Listen("tcp", …)`; QUIC binds `net.ListenUDP` + `quic.Listen` and launches a sibling `quicAcceptLoop`/`serveQUICConnection` (in a NEW same-package file `internal/listener/quic.go`) reusing the SAME per-listener `chainInfo`/metric fields. The `transport_protocol "quic"` filter-chain-match reject is lifted. quic-go v0.54.1 is confined to `internal/listener/quic.go` (the ONLY new external import); `internal/tls` imports only the go-control-plane quic transport proto, never quic-go.

**Tech Stack:** Go; the NEW `github.com/quic-go/quic-go v0.54.1` module (`http3`/`quic`; the FIRST external module added since the MVP trunk); `crypto/tls` (`*stdtls.Config` with `NextProtos:["h3"]`, TLS 1.3); the go-control-plane `config/listener/v3` (`UdpListenerConfig`/`QuicProtocolOptions`) + `extensions/transport_sockets/quic/v3` (`QuicDownstreamTransport`) + `extensions/transport_sockets/tls/v3` (`DownstreamTlsContext`) protos (resolved at the PRESENT `go-control-plane/envoy v1.32.4` — NO new proto module); `internal/tls` (the QUIC transport-socket unwrap) + `internal/listener` (the kind discriminant + the UDP/QUIC accept path). ZERO new PRODUCTION Go packages (the accept path folds into the `listener` package); +1 new go.mod module.

---

## Global Constraints

- **One stage = leg 61.1 only (the QUIC/UDP listen substrate).** This PLAN is the 61.1 IMPL decomposition. Row 61 STAYS `in-progress` at its six-gate — it flips `done` only when ALL THREE legs land (61.1 substrate + 61.2 codec/HCM + 61.3 fixture/harness), ADR-0106 / `reference_roadmap_split_phase_row_done`. The HTTP/3 FAMILY STAYS OPEN after phase-done.
- **NO HTTP serving in 61.1.** The QUIC accept path completes the handshake and closes the connection. Decoding an H3 request and dispatching it into the HCM/router/filter chain is leg 61.2. Do NOT add an `http3.Server`, a codec arm, or an HCM dispatch here.
- **quic-go v0.54.1 EXACTLY** (D-H3-QUICLIB, SPEC §2.4 / §4.1 — PINNED). It is the LAST release keeping the project's `go 1.23.0` directive UNCHANGED (v0.55.0 requires `go 1.24`; v0.60.0 requires `go 1.25`). Re-confirmed this PLAN session: quic-go v0.54.1's own `go.mod` declares `go 1.23`. Do NOT bump the project's `go` directive. SPEC §11 PROVED v0.54.1's `http3.Transport` interoperates with reference Envoy `contrib-v1.37.2` (`HTTP/3.0`, 200, ALPN `h3`, TLS 1.3).
- **quic-go is confined to `internal/listener/quic.go`.** `internal/tls` imports ONLY the go-control-plane `quic/v3` transport proto (`quicv3`), NEVER quic-go. Verify at Task 6: `go list -deps ./internal/tls | grep quic-go` prints NOTHING.
- **`go mod tidy -diff` STAYS EMPTY after a proper tidy.** Adding the module MODIFIES `go.mod`/`go.sum` (the FIRST external module ever — the git diff is non-empty), but `go get …@v0.54.1` + `go mod tidy` leaves the tree TIDY, so the six-gate's `go mod tidy -diff` gate still prints nothing (exit 0). (SPEC §4's "go mod tidy -diff becomes non-empty" phrasing conflates the COMMIT diff with the tidiness check — the tidiness check PASSES; only `go.mod`/`go.sum` gain entries.)
- **TDD (`superpowers:test-driven-development`):** every code task is failing-test → run-fail → minimal-impl → run-pass → commit. NO production code without a failing test first. The module-add (Task 5) is a compile-gated smoke test.
- **Per-task gates (`feedback_pertask_gofmt_lint`):** every code task ends with `gofmt -l` (expect empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. Do NOT skip gofmt.
- **Worktree hygiene (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`):** subagents write to the WORKTREE path (`.worktrees/phase-61.1-impl/…`); the controller verifies `git -C <main-checkout> status` stays clean after each task and the worktree branch is unchanged (no detached HEAD). Pin worktree-relative paths in every dispatch.
- **Commit locally only (`feedback_subagents_no_push`):** subagents NEVER push; the controller squashes + pushes at stage-close.
- **Distinct reject substrings (ADR-0080).** Every strict-reject arm (Task 4 mandatory-TLS; Task 7 quic tuning sub-fields) carries an ADR-0080-distinct substring. These are documented envoy-go-strict DEPARTURES (the reference HONORS these sub-fields); the QUIC-without-transport-socket reject is a config-PARITY reject (both sides reject — SPEC §11 arm reject-C).
- **RE-DERIVE every `file:line` against the master tip at IMPL start (`feedback_brief_citations_not_evidence`).** This PLAN's citations were RE-DERIVED against master tip `728732a2` (SPEC-61 cited the older `cbda648b` — the phase-60 legs shifted the numbers: the `transport_protocol` reject moved `685-689`→`690-695`; the reject test `1187-1196`→`1175-1199`; `NewDownstreamConfig` gained the `provider` param at 60.2). Re-confirm before editing.
- **`reference_fatalf_makes_assertions_unreachable`:** in tests asserting multiple independent properties (the handshake test asserts ALPN + TLS-version + cx-counter), use `Errorf` per property; `Fatalf` only for a broken precondition (dial failed, Start failed).
- **ADR body lands at THIS IMPL (ADR-0044):** ADR-0279 §Decision/§Consequences are authored at this 61.1 IMPL (§Context re-uses SPEC-61 §13). DECISIONS tail **ADR-0280 → ADR-0279** (ADR-0279 was RESERVED for HTTP/3 and lands OUT of numeric order after ADR-0280 per the parallel-fan-out reservation ledger); next-free after this IMPL is **ADR-0281** (which the 61.2 codec-arm decision may consume).
- **Counts at 61.1 exit:** fixtures **105** (+0 — no fixture in 61.1; the cross-side H3-GET is 61.3) · fuzzers **55** (+0 — quic-go owns H3 framing/QPACK; the only hand-rolled parse is the bootstrap config, reachable from the existing listener parse — no new `func Fuzz`) · BackendKind **38** (+0) · stat surface **1201** (+0 — the QUIC listener REUSES `registerListenerMetrics`, whose `downstream_cx_total`/`downstream_cx_active` are per-bound-address DYNAMIC names, not new SURFACE entries) · new production Go packages **+0** (the accept path folds into `internal/listener`) · new go.mod modules **+1** (`quic-go v0.54.1` + transitive) · DECISIONS tail **ADR-0279**.

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. Through 60 phases the proxy has been TCP-only: every listener binds `net.Listen("tcp", …)`, accepts `net.Conn` streams, and dispatches them through a listener-filter → chain-match → TLS-handshake → terminal-filter pipeline. Phase 61 opens the HTTP/3 + QUIC family; leg **61.1** stands up the UDP/QUIC LISTEN substrate — a listener that binds a UDP socket, runs a quic-go listener over it, and completes the QUIC/TLS-1.3 handshake negotiating ALPN `h3`. It does NOT yet serve HTTP (61.2 adds the H3 codec + HCM adapter; 61.3 adds the cross-side differential). The whole point of 61.1 is to introduce the module + the listen-path seam + the mandatory-TLS/ALPN wiring with the smallest testable slice.

**The existing TCP listen path you parallel (RE-DERIVED against master tip `728732a2` — RE-CONFIRM at IMPL):**
- `internal/listener/manager.go` — `listenerRuntime` (`:130-168`) holds the per-listener state: `name`/`addr`, `netLn net.Listener` (`:133`), the transport-AGNOSTIC built state `chainSpecs`/`defaultSpec`/`defaultChain`/`chainByName` (`:141-144`), and the metric fields `downstreamCxTotal *stats.Counter` / `downstreamCxActive *stats.Gauge` (`:163-164`). `chainInfo` (`:106-118`) carries the per-chain `tlsCfg *stdtls.Config` (`:110`) + `netChainFactory` (`:117`).
- `buildListenerRuntimeWithCtx(l *listenerv3.Listener, idx int, …, sdsProvider xds.SecretProvider) (*listenerRuntime, error)` (`:341`, called from the manager loop `:269`) — parses name/addr (`:342-349`), the filter chains (`:367-423`, decoding `fc.GetTransportSocket()` → `internaltls.NewDownstreamConfig(ts, baseDir, sdsProvider)` at `:387`), `default_filter_chain` (`:459-489`, transport-socket decode at `:463`), listener_filters, and assembles the `rt` at `:544-556`. This is where the `kind` discriminant + the QUIC transport-socket branch are added.
- `parseChainSpec(name string, fm *listenerv3.FilterChainMatch)` (`:668`) — the `transport_protocol` enum-domain switch at `:691-696` currently accepts `""`/`"tls"`/`"raw_buffer"` and rejects everything else (incl. `"quic"`) with `transport_protocol %q must be "tls" or "raw_buffer" or empty`. Leg 61.1 lifts `"quic"` into the accepted set.
- `registerListenerMetrics(r *stats.Registry, rt *listenerRuntime)` (`:314`) — allocates `listener.<normalizeAddr(addr)>.downstream_cx_total` (counter) + `.downstream_cx_active` (gauge). Transport-AGNOSTIC; the QUIC path reuses it verbatim after the UDP bind resolves the address.
- `(*Manager).Start(ctx)` (`:861`) — the per-runtime loop `net.Listen("tcp", rt.addr)` at `:868`, captures the resolved addr `:878`, calls `registerListenerMetrics` `:887`, then launches `go rt.acceptLoop(ctx, ln)` `:889-899`. Leg 61.1 branches this loop on `rt.kind`.
- `(*listenerRuntime).acceptLoop(ctx, ln net.Listener)` (`:923`) — the TCP accept loop: `ln.Accept()` → Inc `downstreamCxTotal`/`downstreamCxActive` (`:968-969`) → `go rt.serveConnection(ctx, raw)`. The QUIC sibling `quicAcceptLoop` mirrors the Inc discipline.
- `(*Manager).Stop()` (`:1297`) — closes every `rt.netLn`. Leg 61.1 extends it to close the QUIC listener + UDP conn.
- `(*Manager).Listeners()` (`:1261`) — reports `Info{Name, Addr}` from `rt.netLn.Addr()`, SKIPPING runtimes whose `netLn` is nil. Leg 61.1 makes it kind-aware so a QUIC listener (nil `netLn`) reports its bound UDP addr.

**The TLS seam you reuse (RE-DERIVED against `728732a2`):**
- `internal/tls/config.go` — `NewDownstreamConfig(ts *corev3.TransportSocket, baseDir string, provider xds.SecretProvider) (*DownstreamConfig, error)` (`:39`) dispatches ONLY the `downstreamTLSContextTypeURL` (`:16`, `…tls.v3.DownstreamTlsContext`) and calls the SHARED `commonTLSContextToConfig(c *tlsv3.CommonTlsContext, baseDir, side string, provider xds.SecretProvider) (*stdtls.Config, error)` (`:158`), which builds `cfg := &stdtls.Config{}` (`:214`), appends `tls_certificates` (`:220-237`), errors if a downstream chain has zero certs (`:239-241`), and sets `cfg.NextProtos = append(cfg.NextProtos, c.GetAlpnProtocols()...)` (`:247`). The `DownstreamConfig` struct carries `TLSConfig *stdtls.Config` (`:30`). Leg 61.1 adds a SIBLING `NewQUICDownstreamConfig` that dispatches the QUIC transport-socket type-URL and reuses `commonTLSContextToConfig` — so ALPN `h3` + cert loading + the empty-cert mandatory-TLS error come for free.

**The proto surface (RE-DERIVED @ `go-control-plane/envoy v1.32.4` this PLAN session — `go doc` clean):**
- `listenerv3 "…/envoy/config/listener/v3"`: `Listener.GetUdpListenerConfig() *UdpListenerConfig`; `UdpListenerConfig.GetQuicOptions() *QuicProtocolOptions` (its NON-NIL presence marks a QUIC/H3 listener — the discriminant); `UdpListenerConfig.GetDownstreamSocketConfig()` (accept-ignore). `QuicProtocolOptions` getters (Task 7 strict-reject roster): `GetProofSourceConfig() *TypedExtensionConfig`, `GetConnectionIdGeneratorConfig() *TypedExtensionConfig`, `GetRejectNewConnections() bool`, `GetServerPreferredAddressConfig() *TypedExtensionConfig`, `GetEnabled() *core.RuntimeFeatureFlag`; accept-ignore: `GetIdleTimeout()`, `GetCryptoHandshakeTimeout()`, `GetQuicProtocolOptions() *core.QuicProtocolOptions`.
- `quicv3 "…/envoy/extensions/transport_sockets/quic/v3"`: `QuicDownstreamTransport.GetDownstreamTlsContext() *tlsv3.DownstreamTlsContext` (field 1 — the mandatory inner TLS context); `GetEnableEarlyData() *wrapperspb.BoolValue` (0-RTT — Task 7 reject). Type URL: `type.googleapis.com/envoy.extensions.transport_sockets.quic.v3.QuicDownstreamTransport` (derive via `proto.MessageName` — never hardcode blindly; `reference_network_filter_typeurl_extensions`).
- `corev3 "…/envoy/config/core/v3"`: `SocketAddress_Protocol` enum (`SocketAddress_TCP = 0`, `SocketAddress_UDP = 1`) — informational; the discriminant is `quic_options` presence, not the socket protocol.

**The quic-go v0.54.1 API (VERIFIED this PLAN session via an isolated scratch-module `go doc` — RE-CONFIRM against the fetched module at IMPL):**
- `quic.Listen(conn net.PacketConn, tlsConf *tls.Config, config *quic.Config) (*quic.Listener, error)` — a `*net.UDPConn` satisfies the packet-conn interface (ECN/packet-info auto-enabled). A single `net.PacketConn` may back only one `Listen`.
- `(*quic.Listener).Accept(ctx context.Context) (*quic.Conn, error)` — "returns connections once the handshake has completed" (i.e. Accept returning ⇒ the QUIC + TLS-1.3 handshake succeeded). `(*quic.Listener).Close() error`, `.Addr() net.Addr`. Closing the Listener does NOT close the underlying `net.PacketConn` — the caller owns it (close BOTH in Stop).
- `(*quic.Conn).ConnectionState() quic.ConnectionState` — `.TLS` is a `crypto/tls.ConnectionState` carrying `NegotiatedProtocol` (the ALPN) + `Version` (the TLS version). `(*quic.Conn).CloseWithError(code quic.ApplicationErrorCode, desc string) error`; `HandshakeComplete() <-chan struct{}`.
- Client (the test driver): `quic.DialAddr(ctx context.Context, addr string, tlsConf *tls.Config, conf *quic.Config) (*quic.Conn, error)`.
- `quic.Config` fields the minimal slice may set: `MaxIdleTimeout`, `HandshakeIdleTimeout` (both `time.Duration`). The minimal slice passes `&quic.Config{}` (defaults).
- (Leg 61.2 context, NOT this leg): `http3.Server{Handler http.Handler, TLSConfig, QUICConfig}` + `ServeQUICConn` — the codec arm.

### Discipline (honor on EVERY task) — the memory traps that bite this row
- **`feedback_brief_citations_not_evidence`** — RE-DERIVE every `file:line` against the master tip; the numbers above are from `728732a2`.
- **`feedback_pertask_gofmt_lint`** — `gofmt -l` + `golangci-lint run` on touched packages every task.
- **`reference_fatalf_makes_assertions_unreachable`** — `Errorf` per independent property in the handshake test.
- **`reference_docker_probe_bridge_network` / `reference_host_gateway_ip_docker_desktop`** — NOT relevant to 61.1 (no Docker/differential in 61.1; the handshake test is an in-process local quic-go client). These bite 61.3.
- **`reference_roadmap_split_phase_row_done`** — row 61 STAYS `in-progress`; it flips `done` only at the 61.3 (final-leg) IMPL.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — after 61.1 the HTTP/3 deferred sentence STAYS exactly one live "candidates:" match (the family stays open).

---

## Design pins settled here (the 61.1 D-question resolutions over SPEC-61)

**KIND DISCRIMINANT → `l.GetUdpListenerConfig().GetQuicOptions() != nil`.** A per-listener `kind listenerKind` field (`kindTCP` zero-value / `kindQUIC`) set once in `buildListenerRuntimeWithCtx`. Rejected alternative: keying off `socket_address.protocol == UDP` (the `quic_options` presence is the reference's own QUIC marker and is unambiguous; the socket protocol is informational).

**QUIC TLS DECODE → a NEW `internal/tls.NewQUICDownstreamConfig` reusing `commonTLSContextToConfig`.** Unwrap `QuicDownstreamTransport` → the inner `DownstreamTlsContext` → the SHARED builder (ALPN + cert + empty-cert error reused). Rejected alternative: re-wrapping the inner context as a synthetic `envoy.transport_sockets.tls` `TransportSocket` and calling `NewDownstreamConfig` (an extra marshal round-trip for no gain; the shared `commonTLSContextToConfig` is directly callable within the package). QUIC carries NO SDS in 61.1 → the provider arg is `nil`.

**ACCEPT PATH → a same-package `internal/listener/quic.go` (NOT a new `internal/listener/quic` package).** The QUIC accept methods (`startQUIC`/`quicAcceptLoop`/`serveQUICConnection`) are methods on `*listenerRuntime` and need the PRIVATE runtime fields (`chainByName`, `downstreamCxTotal`, the new `udpConn`/`quicCloser`). A separate package would force exporting the runtime internals. quic-go is imported ONLY here. (SPEC §4 estimated `internal/listener/quic` as +1 package; the 61.1 minimal slice folds it in — +0 packages. A 61.2 quic-stream→`net.Conn` adapter may still warrant extracting a sub-package.)

**QUIC LISTENER CLOSE → two new `*listenerRuntime` fields.** `udpConn *net.UDPConn` (the bound socket) + `quicCloser io.Closer` (holding the `*quic.Listener` as an `io.Closer` so `manager.go`'s struct definition stays quic-go-free — quic-go lives only in `quic.go`). `Start`-unwind + `Stop` close BOTH (quic-go's `Listener.Close` does not close the packet conn).

**HANDSHAKE-ONLY → `serveQUICConnection` closes the conn.** 61.1 completes the handshake (Accept returning proves it) + Inc's the cx counters, then `CloseWithError(0, "")`. NO H3 decode/dispatch (61.2). The subject-side proof is a local quic-go client asserting ALPN `h3` + TLS 1.3.

**MANDATORY TLS → config-parity reject.** A QUIC listener (`quic_options` present) whose filter chain has NO transport socket is boot-rejected `quic listener requires a transport_socket (mandatory TLS)` — PARITY with the reference (SPEC §11 arm reject-C). The minimal slice uses `filter_chains[]` (a QUIC `default_filter_chain` is deferred).

**STRICT-REJECT ROSTER (ADR-0080, Task 7) → distinct substrings for the SET tuning fields the minimal slice does not honor:** `enable_early_data` (0-RTT), `quic_options.proof_source_config`, `.connection_id_generator_config`, `.reject_new_connections`, `.server_preferred_address_config` (connection migration), and a runtime-disabled `.enabled`. ACCEPT-and-IGNORE (documented departure boundary): `idle_timeout`, `crypto_handshake_timeout`, `downstream_socket_config`, the nested `core.QuicProtocolOptions` knobs.

**NO HTTP3-on-non-QUIC parity reject in 61.1.** That reject (SPEC §11 arm reject-B) requires ACCEPTING `codec_type: HTTP3` on a QUIC listener, which is still rejected at `internal/filter/hcm/config.go:240` until leg 61.2 lifts it. The HTTP3-on-non-QUIC config-parity reject is therefore a 61.2 concern.

---

## File structure (decomposition locked here)

**Production (modified):**
- `internal/tls/config.go` — CREATE `NewQUICDownstreamConfig(ts *corev3.TransportSocket, baseDir string) (*DownstreamConfig, error)` + the `quicDownstreamTransportTypeURL` const + the `quicv3` import (Task 3); ADD the `enable_early_data` reject (Task 7).
- `internal/listener/manager.go` — ADD `type listenerKind int` + `kindTCP`/`kindQUIC` consts + the `kind`/`udpConn`/`quicCloser` fields on `listenerRuntime` (Task 4/6); the `kind` discriminant + the QUIC transport-socket branch + the mandatory-TLS reject in `buildListenerRuntimeWithCtx` (Task 4); the `validateQUICOptions` call + helper (Task 7); the `Start` kind-branch (Task 6); the `Stop` + `Listeners` kind-awareness (Task 6); the `transport_protocol "quic"` accept in `parseChainSpec` (Task 2).
- `internal/listener/quic.go` — CREATE: `startQUIC`/`quicAcceptLoop`/`serveQUICConnection`/`quicTLSConfig` (the UDP bind + quic-go listener + handshake-only accept path); the ONLY quic-go import (Task 6).
- `go.mod` / `go.sum` — ADD `github.com/quic-go/quic-go v0.54.1` (+ transitive `quic-go/qpack`, `golang.org/x/crypto`, `golang.org/x/mod`, `golang.org/x/tools`; bumps to `x/net`/`x/sys`/`x/sync`) (Task 5).

**Test (created / modified):**
- `internal/listener/manager_test.go` — MODIFY `TestParseChainSpecRejectsUnknownTransportProtocol` (flip: `"quic"` now ACCEPTED; a truly-unknown value still rejected) + ADD `TestParseChainSpec_QUICTransportProtocolAccepted` (Task 2); ADD `TestBuildListenerRuntime_QUICKind` + `TestBuildListenerRuntime_QUICMandatoryTLS` (Task 4); ADD the strict-reject arm tests (Task 7).
- `internal/tls/config_test.go` — ADD `TestNewQUICDownstreamConfig_ALPNh3` + `TestNewQUICDownstreamConfig_MandatoryTLS` + `TestNewQUICDownstreamConfig_WrongTypeURL` (Task 3); the `enable_early_data` reject test (Task 7).
- `internal/listener/quic_test.go` — CREATE: `TestQUICGoModuleWired` (Task 5, compile-gate) + `TestQUICListener_HandshakeALPNh3` (Task 6, the subject-side integration proof).

**Test fixtures/certs:** REUSE an existing committed self-signed leaf+key from `test/helpers` or `internal/tls/testdata` (a QUIC listener needs a cert; the handshake test client uses `InsecureSkipVerify: true`, so any valid cert works). RE-DERIVE the exact testdata path at IMPL — do NOT generate a new cert if one exists.

**Docs (this IMPL):**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the HTTP/3 substrate section (Task 8).
- `docs/envoy-go/DECISIONS.md` — ADR-0279 §Decision/§Consequences (Task 9).
- `docs/envoy-go/phases/61-http3-downstream-listener/PROGRESS-61.1.md` — the checklist (updated each task).
- `docs/envoy-go/STATE.md` — active-phase header (Task 9, controller).
- `docs/envoy-go/ROADMAP.md` — row 61 STAYS `in-progress` (NO flip in 61.1).
- `next-prompt.txt` — the router roll to the 61.1 IMPL → 61.2 PLAN (Task 9, folded into the squash; `reference_next_prompt_tracked_despite_gitignore`).

---

## Task 1: PROGRESS scaffold + baselines + design pins

**Files:**
- Create: `docs/envoy-go/phases/61-http3-downstream-listener/PROGRESS-61.1.md`

- [ ] **Step 1: Author `PROGRESS-61.1.md`** — the baseline-counts table, the cycle/import-hygiene note (quic-go confined to `quic.go`), the 61.1 design pins (copied from "Design pins settled here"), and the Task checklist (Tasks 1–9 unchecked). Model it on `phases/60-xds-sds-server-cert/PROGRESS-60.2.md`. (This step is folded into the PLAN commit — no separate code.)

- [ ] **Step 2: Commit** (folded into the PLAN stage commit by the controller).

---

## Task 2: Lift the `transport_protocol "quic"` filter-chain-match reject

**Files:**
- Modify: `internal/listener/manager.go:691-696` (the `parseChainSpec` `transport_protocol` switch)
- Test: `internal/listener/manager_test.go:1175-1199` (flip the existing reject test) + a new positive test

**Interfaces:**
- Produces: `parseChainSpec` now accepts `transport_protocol: "quic"` (sets `spec.TransportProtocol = "quic"`); a truly-unknown value still errors.

- [ ] **Step 1: Flip the failing test.** In `manager_test.go`, edit `TestParseChainSpecRejectsUnknownTransportProtocol` (`:1175`) to use a genuinely-unknown value and add a positive QUIC test:

```go
// TestParseChainSpecRejectsUnknownTransportProtocol verifies the
// transport_protocol enum domain is enforced at parse: only "tls",
// "raw_buffer", "quic", or "" are accepted (phase 61.1 lifted "quic").
func TestParseChainSpecRejectsUnknownTransportProtocol(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_tp",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			FilterChainMatch: &listenerv3.FilterChainMatch{TransportProtocol: "sctp"},
			Filters:          []*listenerv3.Filter{filter},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error for transport_protocol=sctp, got nil")
	}
	if !strings.Contains(err.Error(), `transport_protocol "sctp"`) {
		t.Errorf("error %q does not name the bad value", err.Error())
	}
}

// TestParseChainSpec_QUICTransportProtocolAccepted verifies phase 61.1 lifted
// the transport_protocol "quic" reject: the value now parses into the ChainSpec.
func TestParseChainSpec_QUICTransportProtocolAccepted(t *testing.T) {
	spec, err := parseChainSpec("l/fc[0]", &listenerv3.FilterChainMatch{TransportProtocol: "quic"})
	if err != nil {
		t.Fatalf("parseChainSpec(quic): unexpected error: %v", err)
	}
	if spec.TransportProtocol != "quic" {
		t.Errorf("TransportProtocol = %q, want %q", spec.TransportProtocol, "quic")
	}
}
```

- [ ] **Step 2: Run to verify RED.** `go test ./internal/listener/ -run 'TestParseChainSpec_QUICTransportProtocolAccepted|TestParseChainSpecRejectsUnknownTransportProtocol' -count=1 -v` — expect the QUIC test to FAIL (`transport_protocol "quic" must be …`) and the sctp test to PASS.

- [ ] **Step 3: Lift the reject.** In `manager.go`, edit the switch (`:691`):

```go
	// transport_protocol: validate against the v3 enum domain. "quic" is
	// accepted for QUIC/H3 listeners (phase 61.1); the runtime chain-match
	// semantics for a QUIC connection land at leg 61.2.
	switch tp := fm.GetTransportProtocol(); tp {
	case "", "tls", "raw_buffer", "quic":
		spec.TransportProtocol = tp
	default:
		return nil, fmt.Errorf("transport_protocol %q must be \"tls\", \"raw_buffer\", \"quic\", or empty", tp)
	}
```

- [ ] **Step 4: Run to verify GREEN.** `go test ./internal/listener/ -run 'TestParseChainSpec_QUICTransportProtocolAccepted|TestParseChainSpecRejectsUnknownTransportProtocol' -count=1 -v` — both PASS.

- [ ] **Step 5: Per-task gates.** `gofmt -l internal/listener/` (empty) · `golangci-lint run ./internal/listener/...` · `go vet ./internal/listener/...` · `go build ./...`.

- [ ] **Step 6: Commit** — `phase 61.1: lift transport_protocol "quic" filter-chain-match reject`.

---

## Task 3: `internal/tls.NewQUICDownstreamConfig` — unwrap the QUIC transport socket, reuse `commonTLSContextToConfig`

**Files:**
- Modify: `internal/tls/config.go` (add the const + import + function)
- Test: `internal/tls/config_test.go`

**Interfaces:**
- Consumes: the private `commonTLSContextToConfig(c, baseDir, side, provider)` (`:158`) + the `DownstreamConfig` struct (`:29`).
- Produces: `func NewQUICDownstreamConfig(ts *corev3.TransportSocket, baseDir string) (*DownstreamConfig, error)` — dispatches the QUIC transport-socket type URL, unwraps `QuicDownstreamTransport.downstream_tls_context`, and builds the `*stdtls.Config` (ALPN + cert). Errors begin with `tls: downstream: `.

- [ ] **Step 1: Write the failing test.** In `config_test.go` (RE-DERIVE the test helpers for building a `QuicDownstreamTransport` `TransportSocket` — reuse the committed cert/key the existing downstream tests use; see the existing `mkDownstreamTS`/`testdata` helpers):

```go
// TestNewQUICDownstreamConfig_ALPNh3 verifies the QUIC transport socket unwrap
// reuses commonTLSContextToConfig: the inner DownstreamTlsContext's cert loads
// and alpn_protocols:["h3"] lands in NextProtos.
func TestNewQUICDownstreamConfig_ALPNh3(t *testing.T) {
	ts := mkQUICDownstreamTS(t, testCertPEM, testKeyPEM, []string{"h3"})
	dc, err := NewQUICDownstreamConfig(ts, "")
	if err != nil {
		t.Fatalf("NewQUICDownstreamConfig: %v", err)
	}
	if len(dc.TLSConfig.Certificates) == 0 {
		t.Errorf("no certificates loaded from the inner DownstreamTlsContext")
	}
	if !containsStr(dc.TLSConfig.NextProtos, "h3") {
		t.Errorf("NextProtos = %v, want to contain \"h3\"", dc.TLSConfig.NextProtos)
	}
}

// TestNewQUICDownstreamConfig_MandatoryTLS verifies a QUIC transport socket with
// no cert (empty inner DownstreamTlsContext) errors — mandatory TLS.
func TestNewQUICDownstreamConfig_MandatoryTLS(t *testing.T) {
	ts := mkQUICDownstreamTS(t, nil, nil, []string{"h3"})
	_, err := NewQUICDownstreamConfig(ts, "")
	if err == nil {
		t.Fatal("expected error for a QUIC transport socket with no cert, got nil")
	}
	if !strings.Contains(err.Error(), "no tls_certificates configured") {
		t.Errorf("error %q is not the mandatory-TLS empty-cert error", err.Error())
	}
}

// TestNewQUICDownstreamConfig_WrongTypeURL verifies a non-QUIC transport socket
// type URL is rejected.
func TestNewQUICDownstreamConfig_WrongTypeURL(t *testing.T) {
	ts := &corev3.TransportSocket{
		Name:       "bogus",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: &anypb.Any{TypeUrl: "type.googleapis.com/bogus.Bogus"}},
	}
	_, err := NewQUICDownstreamConfig(ts, "")
	if err == nil || !strings.Contains(err.Error(), "unexpected quic transport_socket type_url") {
		t.Fatalf("expected wrong-type-URL error, got %v", err)
	}
}
```

*(The exact `mkQUICDownstreamTS` helper — marshal a `quicv3.QuicDownstreamTransport{DownstreamTlsContext: &tlsv3.DownstreamTlsContext{CommonTlsContext: &tlsv3.CommonTlsContext{TlsCertificates: […], AlpnProtocols: alpn}}}` into a `*corev3.TransportSocket` with the quic type URL — is authored at IMPL; model it on the existing downstream-TS test helper. `containsStr` may already exist; reuse or inline.)*

- [ ] **Step 2: Run to verify RED.** `go test ./internal/tls/ -run 'TestNewQUICDownstreamConfig' -count=1 -v` — FAIL (`NewQUICDownstreamConfig` undefined).

- [ ] **Step 3: Implement.** In `config.go`, add the import `quicv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/quic/v3"`, the const (near `:16`), and the function:

```go
	quicDownstreamTransportTypeURL = "type.googleapis.com/envoy.extensions.transport_sockets.quic.v3.QuicDownstreamTransport"
```

```go
// NewQUICDownstreamConfig parses a *corev3.TransportSocket whose typed_config is
// a QuicDownstreamTransport (envoy.transport_sockets.quic), unwraps the inner
// DownstreamTlsContext, and builds the *stdtls.Config via the SHARED
// commonTLSContextToConfig (so ALPN from alpn_protocols, cert loading, and the
// mandatory-TLS empty-cert error are reused). QUIC carries no SDS in phase 61.1,
// so the provider argument to commonTLSContextToConfig is nil. Errors begin with
// "tls: downstream: ". (Phase 61.1, ADR-0279.)
func NewQUICDownstreamConfig(ts *corev3.TransportSocket, baseDir string) (*DownstreamConfig, error) {
	if ts == nil {
		return nil, fmt.Errorf("tls: downstream: nil transport_socket")
	}
	if ts.GetTypedConfig() == nil || ts.GetTypedConfig().GetTypeUrl() != quicDownstreamTransportTypeURL {
		return nil, fmt.Errorf("tls: downstream: unexpected quic transport_socket type_url %q", ts.GetTypedConfig().GetTypeUrl())
	}
	qt := &quicv3.QuicDownstreamTransport{}
	if err := ts.GetTypedConfig().UnmarshalTo(qt); err != nil {
		return nil, fmt.Errorf("tls: downstream: quic transport_socket unmarshal: %w", err)
	}
	dtc := qt.GetDownstreamTlsContext()
	if dtc == nil {
		return nil, fmt.Errorf("tls: downstream: quic transport_socket has no downstream_tls_context")
	}
	cfg, err := commonTLSContextToConfig(dtc.GetCommonTlsContext(), baseDir, "downstream", nil)
	if err != nil {
		return nil, err
	}
	return &DownstreamConfig{TLSConfig: cfg}, nil
}
```

- [ ] **Step 4: Run to verify GREEN.** `go test ./internal/tls/ -run 'TestNewQUICDownstreamConfig' -count=1 -v` — PASS. Then `go test ./internal/tls/ -count=1` (no regressions).

- [ ] **Step 5: Per-task gates + import-hygiene check.** `gofmt -l internal/tls/` · `golangci-lint run ./internal/tls/...` · `go vet ./internal/tls/...` · `go build ./...` · `go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO` (expect `TLS-NO-QUICGO` — `internal/tls` imports the go-control-plane quic PROTO, never quic-go).

- [ ] **Step 6: Commit** — `phase 61.1: internal/tls.NewQUICDownstreamConfig (unwrap QUIC transport socket, reuse commonTLSContextToConfig)`.

---

## Task 4: The `kind` discriminant + the QUIC transport-socket branch + the mandatory-TLS reject in `buildListenerRuntimeWithCtx`

**Files:**
- Modify: `internal/listener/manager.go` (add `listenerKind` type + consts near `:118`; the `kind` field on `listenerRuntime` `:130`; the discriminant + QUIC branch + mandatory-TLS reject in `buildListenerRuntimeWithCtx` `:341`; set `rt.kind` in the `rt` assembly `:544`)
- Test: `internal/listener/manager_test.go`

**Interfaces:**
- Produces: `listenerRuntime.kind listenerKind` (`kindTCP`/`kindQUIC`); a QUIC listener's `chainInfo.tlsCfg` built via `internaltls.NewQUICDownstreamConfig`; the `quic listener requires a transport_socket (mandatory TLS)` config-parity reject.
- Consumes: `internaltls.NewQUICDownstreamConfig` (Task 3).

- [ ] **Step 1: Write the failing tests.** In `manager_test.go` (RE-DERIVE a `mkQUICListener` helper building a `Listener` with `UdpListenerConfig{QuicOptions:{}}` + a `QuicDownstreamTransport` TS + a benign resolvable filter — use `mkTcpProxyFilter`, which is NEVER dispatched in 61.1; the listener only needs a resolvable chain to build):

```go
// TestBuildListenerRuntime_QUICKind verifies a udp_listener_config.quic_options
// listener builds with kind=kindQUIC and its chain TLS carries ALPN h3.
func TestBuildListenerRuntime_QUICKind(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListener(t, cm, testCertPEM, testKeyPEM, []string{"h3"})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager(quic listener): %v", err)
	}
	rt := mgr.runtimes[0]
	if rt.kind != kindQUIC {
		t.Errorf("kind = %d, want kindQUIC", rt.kind)
	}
	// The single chain's TLS carries ALPN h3.
	var ci *chainInfo
	for _, c := range rt.chainByName {
		ci = c
	}
	if ci == nil || ci.tlsCfg == nil || !containsStr(ci.tlsCfg.NextProtos, "h3") {
		t.Errorf("QUIC chain TLS did not carry ALPN h3: %+v", ci)
	}
}

// TestBuildListenerRuntime_QUICMandatoryTLS verifies a QUIC listener whose chain
// has no transport socket is boot-rejected (mandatory TLS, config parity).
func TestBuildListenerRuntime_QUICMandatoryTLS(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListener(t, cm, nil, nil, nil) // no transport socket
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected mandatory-TLS reject for a QUIC listener with no transport_socket, got nil")
	}
	if !strings.Contains(err.Error(), "quic listener requires a transport_socket") {
		t.Errorf("error %q is not the mandatory-TLS reject", err.Error())
	}
}
```

- [ ] **Step 2: Run to verify RED.** `go test ./internal/listener/ -run 'TestBuildListenerRuntime_QUIC' -count=1 -v` — FAIL (`kindQUIC` undefined / `NewDownstreamConfig` rejects the quic type URL).

- [ ] **Step 3: Implement.** In `manager.go`:

(a) Add the kind type near `chainInfo` (`:118`):
```go
// listenerKind discriminates the transport a listenerRuntime binds. kindTCP
// (the zero value) uses net.Listen("tcp", …) + the acceptLoop/serveConnection
// stream path; kindQUIC (phase 61.1) binds net.ListenUDP + a quic-go listener
// and uses the quicAcceptLoop/serveQUICConnection path (internal/listener/quic.go).
type listenerKind int

const (
	kindTCP  listenerKind = iota
	kindQUIC
)
```

(b) Add the field to `listenerRuntime` (near `:134`):
```go
	kind listenerKind // 61.1: kindTCP (default) or kindQUIC
```

(c) Compute the discriminant early in `buildListenerRuntimeWithCtx` (after the addr parse `:349`):
```go
	// Phase 61.1: udp_listener_config.quic_options presence marks a QUIC/H3
	// listener (the reference's own QUIC marker). A QUIC listener binds UDP and
	// stands a quic-go listener (Start branches on kind); its transport socket is
	// envoy.transport_sockets.quic (decoded below), and mandatory TLS applies.
	kind := kindTCP
	if l.GetUdpListenerConfig().GetQuicOptions() != nil {
		kind = kindQUIC
	}
```

(d) Branch the filter_chains[] transport-socket decode (`:382-395`):
```go
		var chainTLS *stdtls.Config
		if ts := fc.GetTransportSocket(); ts != nil {
			var dc *internaltls.DownstreamConfig
			var derr error
			if kind == kindQUIC {
				dc, derr = internaltls.NewQUICDownstreamConfig(ts, baseDir)
			} else {
				dc, derr = internaltls.NewDownstreamConfig(ts, baseDir, sdsProvider)
			}
			if derr != nil {
				return nil, fmt.Errorf("listener: %q: filter_chains[%d]: %w", name, i, derr)
			}
			chainTLS = dc.TLSConfig
			anyTLS = true
		} else if kind == kindQUIC {
			// Config-parity reject (SPEC-61 §11 arm reject-C): a QUIC listener
			// has no plaintext form; TLS is baked into the transport.
			return nil, fmt.Errorf("listener: %q: filter_chains[%d]: quic listener requires a transport_socket (mandatory TLS)", name, i)
		} else {
			anyPlaintext = true
		}
```

(e) Set `kind: kind` in the `rt := &listenerRuntime{…}` assembly (`:544`).

*(A QUIC `default_filter_chain` is out of the minimal slice — the QUIC listener uses `filter_chains[]`. Leaving the `default_filter_chain` decode (`:463`) TCP-only is acceptable; a QUIC listener that ONLY configures `default_filter_chain` would mis-decode its quic TS as a tls TS and error — a benign, documented boundary. IMPL may add a QUIC branch there symmetrically if trivial.)*

- [ ] **Step 4: Run to verify GREEN.** `go test ./internal/listener/ -run 'TestBuildListenerRuntime_QUIC' -count=1 -v` — PASS. Then `go test ./internal/listener/ -count=1` (no regressions on the TCP path — kindTCP is the zero value, so every existing test is unaffected).

- [ ] **Step 5: Per-task gates.** `gofmt -l` · `golangci-lint run ./internal/listener/...` · `go vet` · `go build ./...`.

- [ ] **Step 6: Commit** — `phase 61.1: listenerRuntime kind discriminant + QUIC transport-socket decode + mandatory-TLS reject`.

---

## Task 5: Add the `github.com/quic-go/quic-go v0.54.1` module (the FIRST external module)

**Files:**
- Modify: `go.mod`, `go.sum`
- Test: `internal/listener/quic_test.go` (a compile-gate smoke test)

**Interfaces:**
- Produces: the `github.com/quic-go/quic-go` + `…/http3` packages resolvable; `go build ./...` green; `go mod tidy -diff` EMPTY.

- [ ] **Step 1: Write the compile-gate smoke test.** Create `internal/listener/quic_test.go`:

```go
package listener

import (
	stdtls "crypto/tls"
	"net"
	"testing"
	"time"

	quic "github.com/quic-go/quic-go"
)

// TestQUICGoModuleWired is a compile-time proof the quic-go v0.54.1 module is
// wired and the API leg 61.1 depends on exists. Not behavioral — Task 6
// exercises the real bind + handshake.
func TestQUICGoModuleWired(t *testing.T) {
	_ = &quic.Config{MaxIdleTimeout: 30 * time.Second, HandshakeIdleTimeout: 5 * time.Second}
	var _ func(net.PacketConn, *stdtls.Config, *quic.Config) (*quic.Listener, error) = quic.Listen
	var _ func() error // placeholder to keep imports used if quic.Listen is refactored
}
```

- [ ] **Step 2: Run to verify RED.** `go test ./internal/listener/ -run 'TestQUICGoModuleWired' -count=1` — FAIL to COMPILE (`no required module provides package github.com/quic-go/quic-go`).

- [ ] **Step 3: Add the module.** From the worktree root:
```bash
go get github.com/quic-go/quic-go@v0.54.1
go mod tidy
```
Confirm `go.mod` gained `github.com/quic-go/quic-go v0.54.1` (direct) + `github.com/quic-go/qpack` and `golang.org/x/crypto`/`x/mod`/`x/tools` (transitive), with bumps to `golang.org/x/net`/`x/sys`/`x/sync`. The exact transitive set is whatever `go mod tidy` writes — do NOT hand-edit.

- [ ] **Step 4: Run to verify GREEN + the module-gate.**
```bash
go test ./internal/listener/ -run 'TestQUICGoModuleWired' -count=1 -v   # PASS
go build ./...                                                          # green
go mod tidy -diff && echo MODTIDY_CLEAN                                 # MODTIDY_CLEAN (tidy)
grep -c 'quic-go/quic-go v0.54.1' go.mod                                # 1
```
The project's `go` directive stays `go 1.23.0` (do NOT let `go mod tidy` bump it — v0.54.1 is `go 1.23`-compatible; if the directive changed, the wrong quic-go version was pulled).

- [ ] **Step 5: Per-task gates.** `gofmt -l .` · `golangci-lint run ./...` (the new module's lint surface is external — expect clean on our code) · `go vet ./...`.

- [ ] **Step 6: Commit** — `phase 61.1: add github.com/quic-go/quic-go v0.54.1 (the first external module)` (stage `go.mod go.sum internal/listener/quic_test.go`).

---

## Task 6: The UDP/QUIC listen path — `Start` kind-branch + `quicAcceptLoop`/`serveQUICConnection` + the handshake integration test

**Files:**
- Create: `internal/listener/quic.go` (`startQUIC`/`quicAcceptLoop`/`serveQUICConnection`/`quicTLSConfig`; the sole quic-go import)
- Modify: `internal/listener/manager.go` — the `udpConn`/`quicCloser` fields on `listenerRuntime`; the `Start` kind-branch (`:867`); `Stop` (`:1300`) + `Listeners` (`:1265`) kind-awareness
- Test: `internal/listener/quic_test.go` — `TestQUICListener_HandshakeALPNh3`

**Interfaces:**
- Consumes: `rt.chainByName`/`rt.defaultChain` (the per-chain `tlsCfg`), `registerListenerMetrics`, `rt.downstreamCxTotal`/`downstreamCxActive`, `internaltls` (indirectly, via the already-built `tlsCfg`).
- Produces: a bound QUIC listener whose handshake negotiates ALPN `h3` + TLS 1.3; `downstream_cx_total` Inc per accepted conn.

- [ ] **Step 1: Write the failing integration test.** In `quic_test.go`:

```go
// TestQUICListener_HandshakeALPNh3 is the leg-61.1 subject-side proof: a QUIC
// listener binds UDP, and a local quic-go client completes the QUIC/TLS-1.3
// handshake negotiating ALPN h3. NO HTTP is served (leg 61.2).
func TestQUICListener_HandshakeALPNh3(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListener(t, cm, testCertPEM, testKeyPEM, []string{"h3"})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManager(boot, cm, reg, testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	infos := mgr.Listeners()
	if len(infos) != 1 {
		t.Fatalf("Listeners() = %d, want 1 (QUIC listener must report its bound UDP addr)", len(infos))
	}
	addr := infos[0].Addr

	clientTLS := &stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true} //nolint:gosec // local handshake test
	conn, err := quic.DialAddr(ctx, addr, clientTLS, &quic.Config{})
	if err != nil {
		t.Fatalf("quic.DialAddr(%s): %v", addr, err)
	}
	defer conn.CloseWithError(0, "")

	tlsState := conn.ConnectionState().TLS
	if tlsState.NegotiatedProtocol != "h3" {
		t.Errorf("negotiated ALPN = %q, want %q", tlsState.NegotiatedProtocol, "h3")
	}
	if tlsState.Version != stdtls.VersionTLS13 {
		t.Errorf("TLS version = %#x, want TLS 1.3 (%#x)", tlsState.Version, stdtls.VersionTLS13)
	}
	// The accept path Inc'd downstream_cx_total for the completed handshake.
	// (RE-DERIVE the registry read helper; poll briefly — Accept runs in a
	// goroutine so the Inc may lag the client's handshake completion.)
	if got := pollCounter(t, reg, "listener."+normalizeAddr(addr)+".downstream_cx_total", 1, 2*time.Second); got < 1 {
		t.Errorf("downstream_cx_total = %d, want >= 1", got)
	}
}
```

*(`pollCounter` reads the registry counter by name, retrying until >= want or timeout — RE-DERIVE the registry accessor at IMPL from the existing 06.1 listener-metric tests, e.g. `TestAllocatesTwoMetricsPerListener`. The cx-counter assertion has an inherent accept-goroutine race; poll, do not read once.)*

- [ ] **Step 2: Run to verify RED.** `go test ./internal/listener/ -run 'TestQUICListener_HandshakeALPNh3' -count=1 -v` — FAIL (Start binds `net.Listen("tcp",…)` on a UDP listener / `Listeners()` reports nothing / dial times out).

- [ ] **Step 3: Add the runtime close fields.** In `manager.go`, add to `listenerRuntime` (near `:133`):
```go
	// 61.1 QUIC close path: udpConn is the bound UDP socket; quicCloser holds
	// the quic-go *Listener (as an io.Closer so this struct stays quic-go-free —
	// quic.go owns the concrete type). Both are closed by Start-unwind + Stop
	// (quic-go's Listener.Close does not close the underlying packet conn).
	udpConn    *net.UDPConn
	quicCloser io.Closer
```
*(`io` is already imported at `:9`.)*

- [ ] **Step 4: Branch `Start` on kind.** In `Start` (`:867`), at the top of the per-runtime bind loop:
```go
	for i, rt := range m.runtimes {
		if rt.kind == kindQUIC {
			if err := rt.startQUIC(ctx, m.registry); err != nil {
				for j := 0; j < i; j++ {
					m.runtimes[j].closeBind()
				}
				return fmt.Errorf("listener: %q: bind %s: %w", rt.name, rt.addr, err)
			}
			continue
		}
		ln, err := net.Listen("tcp", rt.addr)
		// … existing TCP body unchanged …
	}
```
*(Add a small `closeBind()` helper on `*listenerRuntime` that closes whichever of `netLn`/`quicCloser`/`udpConn` are set, and use it in BOTH the TCP unwind and this QUIC unwind to keep the unwind uniform. Alternatively inline the three nil-checked closes — IMPL's call. The QUIC accept-loop goroutine is launched INSIDE `startQUIC` after the bind, mirroring the TCP path.)*

- [ ] **Step 5: Author `internal/listener/quic.go`:**
```go
package listener

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"log"
	"net"

	quic "github.com/quic-go/quic-go"

	"github.com/pgdad/envoy-go/internal/stats"
)

// startQUIC binds the listener's UDP socket, stands a quic-go listener over it
// (with the single chain's *stdtls.Config, ALPN h3), registers the reused
// per-listener metrics on the resolved address, and launches the accept loop.
// Phase 61.1: handshake substrate only — no HTTP is served.
func (rt *listenerRuntime) startQUIC(ctx context.Context, reg *stats.Registry) error {
	udpAddr, err := net.ResolveUDPAddr("udp", rt.addr)
	if err != nil {
		return err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	tlsCfg := rt.quicTLSConfig()
	if tlsCfg == nil {
		_ = udpConn.Close()
		return errors.New("quic listener has no TLS config (mandatory TLS not built)")
	}
	ql, err := quic.Listen(udpConn, tlsCfg, &quic.Config{})
	if err != nil {
		_ = udpConn.Close()
		return err
	}
	rt.udpConn = udpConn
	rt.quicCloser = ql
	rt.addr = udpConn.LocalAddr().String() // resolved (port 0 → OS pick)
	registerListenerMetrics(reg, rt)
	go rt.quicAcceptLoop(ctx, ql)
	return nil
}

// quicTLSConfig returns the single chain's *stdtls.Config for the minimal
// single-chain QUIC slice. SNI-dispatched multi-chain QUIC is deferred; if
// multiple chains exist, the first non-nil TLS config is used.
func (rt *listenerRuntime) quicTLSConfig() *stdtls.Config {
	if rt.defaultChain != nil && rt.defaultChain.tlsCfg != nil {
		return rt.defaultChain.tlsCfg
	}
	for _, ci := range rt.chainByName {
		if ci.tlsCfg != nil {
			return ci.tlsCfg
		}
	}
	return nil
}

// quicAcceptLoop accepts QUIC connections whose handshake has already completed
// (quic-go's Accept returns post-handshake). It mirrors acceptLoop's cx-metric
// discipline. Phase 61.1 does not serve HTTP — serveQUICConnection closes the
// connection after counting it.
func (rt *listenerRuntime) quicAcceptLoop(ctx context.Context, ql *quic.Listener) {
	for {
		conn, err := ql.Accept(ctx)
		if err != nil {
			// Listener closed (Stop) or ctx canceled — the normal shutdown path.
			if ctx.Err() != nil {
				return
			}
			log.Printf("listener %q: quic accept: %v", rt.name, err)
			return
		}
		rt.downstreamCxTotal.Inc()
		rt.downstreamCxActive.Inc()
		go rt.serveQUICConnection(ctx, conn)
	}
}

// serveQUICConnection is the phase-61.1 handshake-only terminal: the QUIC/TLS
// handshake is complete (Accept returned), so the leg's capability is proven.
// Leg 61.2 decodes an H3 request here and dispatches it into the HCM/router
// chain. For now, count the conn (deferred Dec) and close cleanly.
func (rt *listenerRuntime) serveQUICConnection(ctx context.Context, conn *quic.Conn) {
	defer rt.downstreamCxActive.Dec()
	_ = ctx
	_ = conn.CloseWithError(0, "")
}
```
*(RE-CONFIRM the quic-go v0.54.1 symbol names against the fetched module: `quic.Listen`, `(*quic.Listener).Accept` returning `*quic.Conn`, `(*quic.Conn).CloseWithError`, `(*quic.Conn).ConnectionState().TLS`. The listener-closed sentinel — if quic-go exports one, e.g. `quic.ErrServerClosed` — MAY be matched explicitly instead of the `ctx.Err()` guard; the minimal slice returns on any Accept error after Stop, which is correct because Stop closes the listener + cancels via ctx.)*

- [ ] **Step 6: Make `Stop` + `Listeners` kind-aware.** In `Stop` (`:1300`), before/alongside the `netLn` close:
```go
	for _, rt := range m.runtimes {
		if rt.quicCloser != nil {
			_ = rt.quicCloser.Close()
			rt.quicCloser = nil
		}
		if rt.udpConn != nil {
			_ = rt.udpConn.Close()
			rt.udpConn = nil
		}
		if rt.netLn != nil {
			_ = rt.netLn.Close()
			rt.netLn = nil
		}
	}
```
In `Listeners` (`:1265`), report the QUIC addr:
```go
	for _, rt := range m.runtimes {
		switch {
		case rt.kind == kindQUIC && rt.udpConn != nil:
			out = append(out, Info{Name: rt.name, Addr: rt.udpConn.LocalAddr().String()})
		case rt.netLn != nil:
			out = append(out, Info{Name: rt.name, Addr: rt.netLn.Addr().String()})
		}
	}
```

- [ ] **Step 7: Run to verify GREEN.** `go test ./internal/listener/ -run 'TestQUICListener_HandshakeALPNh3' -count=1 -v` — PASS (ALPN h3, TLS 1.3, cx counter >= 1). Then `go test ./internal/listener/ -count=1` (full package, TCP paths unaffected). Then `go test ./internal/listener/ -race -count=1` (the accept-goroutine + Stop nil-writes must be race-clean — mirror the TCP `ln` capture discipline; `feedback_full_suite_race_after_background_mutator`).

- [ ] **Step 8: Per-task gates + import-hygiene.** `gofmt -l` · `golangci-lint run ./internal/listener/...` · `go vet` · `go build ./...` · `go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO` (quic-go stays confined to `internal/listener`).

- [ ] **Step 9: Commit** — `phase 61.1: UDP/QUIC listen path (Start kind-branch + quicAcceptLoop handshake substrate) + handshake integration test`.

---

## Task 7: The QUIC strict-reject roster (ADR-0080 distinct substrings)

**Files:**
- Modify: `internal/tls/config.go` (`NewQUICDownstreamConfig`: the `enable_early_data` reject)
- Modify: `internal/listener/manager.go` (`validateQUICOptions` helper + its call in `buildListenerRuntimeWithCtx`)
- Test: `internal/tls/config_test.go` + `internal/listener/manager_test.go`

**Interfaces:**
- Produces: distinct-substring rejects for `enable_early_data`, `proof_source_config`, `connection_id_generator_config`, `reject_new_connections`, `server_preferred_address_config`, runtime-disabled `enabled`. Empty/absent sub-fields + `idle_timeout`/`crypto_handshake_timeout`/`downstream_socket_config` take the accept path.

- [ ] **Step 1: Write the failing tests.** Add per-arm tests. In `config_test.go`:
```go
func TestNewQUICDownstreamConfig_Rejects0RTT(t *testing.T) {
	ts := mkQUICDownstreamTSEarlyData(t, testCertPEM, testKeyPEM, []string{"h3"}, true)
	_, err := NewQUICDownstreamConfig(ts, "")
	if err == nil || !strings.Contains(err.Error(), "enable_early_data") {
		t.Fatalf("expected enable_early_data reject, got %v", err)
	}
}
```
In `manager_test.go` (a table over the quic_options sub-fields, each set on a `mkQUICListener` variant):
```go
func TestBuildListenerRuntime_QUICStrictRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*listenerv3.QuicProtocolOptions)
		substr string
	}{
		{"proof_source_config", func(q *listenerv3.QuicProtocolOptions) { q.ProofSourceConfig = &corev3.TypedExtensionConfig{Name: "x"} }, "proof_source_config"},
		{"connection_id_generator", func(q *listenerv3.QuicProtocolOptions) { q.ConnectionIdGeneratorConfig = &corev3.TypedExtensionConfig{Name: "x"} }, "connection_id_generator_config"},
		{"reject_new_connections", func(q *listenerv3.QuicProtocolOptions) { q.RejectNewConnections = true }, "reject_new_connections"},
		{"server_preferred_address", func(q *listenerv3.QuicProtocolOptions) { q.ServerPreferredAddressConfig = &corev3.TypedExtensionConfig{Name: "x"} }, "server_preferred_address_config"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
			l := mkQUICListenerWithOptions(t, cm, testCertPEM, testKeyPEM, []string{"h3"}, tc.mutate)
			boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
			_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
			if err == nil || !strings.Contains(err.Error(), tc.substr) {
				t.Fatalf("case %s: expected reject naming %q, got %v", tc.name, tc.substr, err)
			}
		})
	}
}

func TestBuildListenerRuntime_QUICAcceptsIgnoredOptions(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListenerWithOptions(t, cm, testCertPEM, testKeyPEM, []string{"h3"}, func(q *listenerv3.QuicProtocolOptions) {
		q.IdleTimeout = durationpb.New(30 * time.Second) // accept-and-ignore
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	if _, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry()); err != nil {
		t.Fatalf("idle_timeout should be accepted-and-ignored, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify RED.** `go test ./internal/tls/ ./internal/listener/ -run 'QUICStrictRejects|QUICAcceptsIgnored|Rejects0RTT' -count=1 -v` — the reject cases FAIL (currently accepted).

- [ ] **Step 3: Implement.**
(a) In `NewQUICDownstreamConfig` (`config.go`), after unmarshalling `qt`, before unwrapping the TLS context:
```go
	if qt.GetEnableEarlyData().GetValue() {
		return nil, fmt.Errorf("tls: downstream: quic enable_early_data (0-RTT) is not supported in phase 61.1")
	}
```
(b) In `manager.go`, add the helper + call it in `buildListenerRuntimeWithCtx` right after computing `kind == kindQUIC`:
```go
	if kind == kindQUIC {
		if err := validateQUICOptions(name, l.GetUdpListenerConfig().GetQuicOptions()); err != nil {
			return nil, err
		}
	}
```
```go
// validateQUICOptions strict-rejects the udp_listener_config.quic_options tuning
// sub-fields the phase-61.1 minimal slice does not honor (ADR-0080 distinct
// substrings; the reference SUPPORTS these — a documented envoy-go-strict
// DEPARTURE). idle_timeout / crypto_handshake_timeout / downstream_socket_config
// and the nested core QuicProtocolOptions knobs are accepted-and-ignored.
func validateQUICOptions(name string, q *listenerv3.QuicProtocolOptions) error {
	if q == nil {
		return nil
	}
	if q.GetProofSourceConfig() != nil {
		return fmt.Errorf("listener: %q: quic_options.proof_source_config is not supported in phase 61.1", name)
	}
	if q.GetConnectionIdGeneratorConfig() != nil {
		return fmt.Errorf("listener: %q: quic_options.connection_id_generator_config is not supported in phase 61.1", name)
	}
	if q.GetRejectNewConnections() {
		return fmt.Errorf("listener: %q: quic_options.reject_new_connections is not supported in phase 61.1", name)
	}
	if q.GetServerPreferredAddressConfig() != nil {
		return fmt.Errorf("listener: %q: quic_options.server_preferred_address_config is not supported in phase 61.1", name)
	}
	if ff := q.GetEnabled(); ff != nil && ff.GetDefaultValue() != nil && !ff.GetDefaultValue().GetValue() {
		return fmt.Errorf("listener: %q: quic_options.enabled=false (runtime-disabled QUIC) is not supported in phase 61.1", name)
	}
	return nil
}
```

- [ ] **Step 4: Run to verify GREEN.** `go test ./internal/tls/ ./internal/listener/ -run 'QUICStrictRejects|QUICAcceptsIgnored|Rejects0RTT' -count=1 -v` — all PASS. Then the full packages `go test ./internal/tls/ ./internal/listener/ -count=1`.

- [ ] **Step 5: Per-task gates.** `gofmt -l` · `golangci-lint run ./internal/tls/... ./internal/listener/...` · `go vet` · `go build ./...`.

- [ ] **Step 6: Commit** — `phase 61.1: QUIC strict-reject roster (0-RTT + quic_options tuning sub-fields, ADR-0080)`.

---

## Task 8: BEHAVIOR_CONTRACT — the HTTP/3 substrate section

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: Append a new HTTP/3 section** (RE-DERIVE the exact heading style + placement from the existing sections; this is the 61.1 SUBSTRATE portion — 61.2 extends it to HTTP serving, 61.3 to the cross-side proof):

> **HTTP/3 downstream listener (QUIC listen substrate — phase 61.1, ADR-0279).** A `Listener` with `udp_listener_config.quic_options` present is a QUIC/H3 listener: it binds a UDP socket (`net.ListenUDP`) and stands a quic-go (`github.com/quic-go/quic-go` v0.54.1) listener over it, parallel to the TCP `net.Listen` path. TLS is MANDATORY — the filter chain's transport socket MUST be `envoy.transport_sockets.quic` (a `QuicDownstreamTransport` wrapping a `DownstreamTlsContext` with a server cert + `alpn_protocols` including `h3`); a QUIC listener with no transport socket is boot-rejected (`quic listener requires a transport_socket (mandatory TLS)` — config parity with the reference). The QUIC/TLS-1.3 handshake negotiates ALPN `h3`. The `transport_protocol: "quic"` filter-chain-match value is accepted. **Phase 61.1 completes the handshake only — it does NOT yet decode or serve an HTTP/3 request (leg 61.2 adds the H3 codec + HCM adapter; leg 61.3 the cross-side differential).** Unsupported `quic_options`/transport-socket tuning sub-fields (`enable_early_data`/0-RTT, `proof_source_config`, `connection_id_generator_config`, `reject_new_connections`, `server_preferred_address_config`, runtime-disabled `enabled`) STRICT-REJECT loudly (envoy-go-strict DEPARTURE, ADR-0080); `idle_timeout`/`crypto_handshake_timeout`/`downstream_socket_config` are accepted-and-ignored. Deferred (HTTP/3 family stays open): upstream H3, alt-svc, 0-RTT, h3spec, QUIC robustness, SNI-dispatched multi-chain QUIC.

- [ ] **Step 2: Commit** — `phase 61.1: BEHAVIOR_CONTRACT HTTP/3 QUIC-substrate section`.

---

## Task 9: ADR-0279 + STATE + sentinel re-check + six-gate verify + router roll

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0279 §Decision/§Consequences), `docs/envoy-go/STATE.md` (active-phase header), `docs/envoy-go/ROADMAP.md` (row 61 STAYS `in-progress` — verify, no flip), `next-prompt.txt` (router roll), `docs/envoy-go/phases/61-http3-downstream-listener/PROGRESS-61.1.md` (final check-off)

- [ ] **Step 1: Author ADR-0279 §Decision/§Consequences** (§Context re-uses SPEC-61 §13). §Decision records: the `quic-go v0.54.1` module choice (the last `go 1.23`-compatible release; interop PROVEN vs reference contrib-v1.37.2, SPEC §11); the `listenerRuntime.kind` discriminant + the UDP/QUIC listen path parallel to the TCP one (reusing the transport-agnostic chain-build + `registerListenerMetrics`, a sibling `quicAcceptLoop`/`serveQUICConnection` in `internal/listener/quic.go`, quic-go confined there); the mandatory-TLS via the shared `commonTLSContextToConfig` (a new `internal/tls.NewQUICDownstreamConfig` unwrap); the config-parity + ADR-0080 strict-reject roster; the handshake-only 61.1 slice (HTTP serving deferred to 61.2). §Consequences: `go.mod`/`go.sum` gain the first external module (`go mod tidy -diff` stays clean); the codec-arm decision is deferred to leg 61.2 (which MAY anchor ADR-0281); row 61 stays `in-progress` until 61.3. DECISIONS tail **ADR-0280 → ADR-0279** (out of numeric order per the reservation ledger); next-free **ADR-0281**.

- [ ] **Step 2: Update STATE.md** — active-phase header → `phase 61.1 IMPL done` (NEXT = the phase-61.2 PLAN, or the roller's self-pick; the sentinel governs).

- [ ] **Step 3: Verify ROADMAP row 61 STAYS `in-progress`** — NO flip (61.2 + 61.3 still pending; `reference_roadmap_split_phase_row_done`). Confirm the HTTP/3 deferred sentence is unchanged (the family stays open).

- [ ] **Step 4: Sentinel re-check (MECHANICAL, per next-prompt.txt).** Run the three checks; confirm check-(1) STILL prints `NOT DONE: row 61` (in-progress), check-(2) STILL prints the three live "candidates:" sentences, check-(3) STILL prints the never-opened families ⇒ the sentinel does NOT fire.

- [ ] **Step 5: Six-gate verify** (in the worktree):
```bash
gofmt -l .                                                              # empty
golangci-lint run ./...                                                 # exit 0
go vet ./...                                                            # clean
go build ./... && echo BUILD_OK                                         # BUILD_OK
go mod tidy -diff && echo MODTIDY_CLEAN                                 # MODTIDY_CLEAN (tidy despite the new module)
go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO    # TLS-NO-QUICGO
grep -rh '^func Fuzz' --include='*.go' . | wc -l                        # 55 (+0)
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                       # 105 (+0)
grep -c 'quic-go/quic-go v0.54.1' go.mod                                # 1 (the first external module)
go test ./internal/tls/ ./internal/listener/ -count=1                   # ok, ok
go test ./internal/tls/ ./internal/listener/ -race -count=1             # race-clean (QUIC accept goroutine)
```
The FULL non-differential suite (`go test $(go list ./... | grep -v '/test/differential') -count=1`) + the full 105-dir differential (byte-stable — no new fixture, no TCP-path change) are run by the CONTROLLER on the frozen squash HEAD.

- [ ] **Step 6: Router roll.** Edit `next-prompt.txt` (in the worktree — `reference_next_prompt_tracked_despite_gitignore`) to point at the phase-61.2 PLAN (the H3 codec + HCM adapter leg): update the STATUS block, the sentinel re-check note, the "What THIS session does" section, and the ADR ledger (ADR-0279 LANDED; next-to-land ADR-0281 for the 61.2 codec arm OR folded). Fold into the stage squash.

- [ ] **Step 7: Commit** — `phase 61.1: ADR-0279 + STATE + sentinel re-check + six-gate + router roll to 61.2 PLAN`.

---

## Self-Review (run after drafting; fix inline)

**1. Spec coverage (SPEC-61 §10 leg-61.1 8-item list):** (1) add quic-go v0.54.1 → Task 5 ✓. (2) parse `udp_listener_config`/`quic_options` + the `kind` discriminant → Task 4 ✓. (3) lift the `transport_protocol "quic"` reject + UPDATE the reject test → Task 2 ✓. (4) UDP `net.ListenUDP` + quic-go listener bind + `quicAcceptLoop` + reused `downstream_cx_total` → Task 6 ✓. (5) build the `*stdtls.Config` (unwrap the quic TS → `NewDownstreamConfig`, ALPN h3) → Task 3 ✓. (6) config-parity + strict-reject arms → Task 4 (mandatory-TLS) + Task 7 (tuning sub-fields) ✓. (7) subject-side handshake integration test (local quic-go client) → Task 6 ✓. (8) verify six-gate (`go mod tidy -diff`) + ADR-0279 body → Task 9 ✓.

**2. Placeholder scan:** every code step carries complete code or an explicit RE-DERIVE flag for a test HELPER (`mkQUICListener`/`mkQUICDownstreamTS`/`pollCounter`) that mirrors an existing helper — no `TODO`/`implement later` in production code steps.

**3. Type consistency:** `NewQUICDownstreamConfig(ts, baseDir) (*DownstreamConfig, error)` (Task 3) is consumed with that exact signature in Task 4. `listenerKind`/`kindTCP`/`kindQUIC` (Task 4) are used identically in Tasks 6/7. `startQUIC`/`quicAcceptLoop`/`serveQUICConnection`/`quicTLSConfig`/`validateQUICOptions` names are consistent across Tasks 6/7. `udpConn`/`quicCloser` fields (Task 6) match their close sites.

**4. Ordering:** Task 2 (no module) → Task 3 (no module) → Task 4 (needs Task 3) → Task 5 (module) → Task 6 (needs Tasks 3/4/5) → Task 7 (needs Tasks 3/4) → Tasks 8/9 (docs). Each task builds green independently; kindTCP is the zero value so every existing TCP test stays unaffected before Task 6.

**5. Discipline:** quic-go confined to `internal/listener/quic.go` (import-hygiene check in Tasks 3/6/9); TDD red→green every code task; `Errorf`-per-property in the handshake test; the `-race` gate for the accept goroutine; row 61 stays `in-progress`; ADR-0279 §Decision at the IMPL (ADR-0044).
