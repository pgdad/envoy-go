# Phase 61.2 Implementation Plan — `http3-h3-codec-hcm`: the H3 codec + HCM adapter — a THIRD dispatch arm serving an HTTP/3 GET into the EXISTING HCM → router → filter-chain → upstream path, wiring the 61.1 handshake-only `serveQUICConnection` into a quic-go `http3.Server`. The SECOND of the confirmed 61.1/61.2/61.3 split (SPEC-61 §3.0); ANCHORS **ADR-0281**. Row 61 STAYS `in-progress`.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). Subagents auto-commit per CLAUDE.md — the controller verifies each commit, cleans leak files, squashes at stage-close, re-runs the suite on the frozen HEAD (`feedback_subagent_autocommit_claudemd`).

**Goal:** Make envoy-go's 61.1 QUIC listener SERVE HTTP/3. An operator's QUIC listener (`udp_listener_config.quic_options` + an `envoy.transport_sockets.quic` transport socket + an HCM with `codec_type: HTTP3` + a router) accepts a QUIC connection (61.1), decodes an H3 GET, and dispatches it through the EXISTING HCM → router → filter-chain → upstream path, writing the response back over H3. The proven capability is a single `GET → 200 (or a routed backend response)` over HTTP/3, verified SUBJECT-side by a local quic-go `http3.Transport` client. The cross-side differential (`0102`) + harness UDP surgery is leg 61.3.

**Architecture:** quic-go's `http3.Server` IS the H3 codec (framing + QPACK); it yields the stdlib `(*http.Request, http.ResponseWriter)` handler contract. So the HCM H3 arm is a **stdlib-typed dispatch method** — NOT a codec package and NOT a quic-go importer. A new `network.H3Terminal` interface (`TerminalFilter` + `ServeH3(w http.ResponseWriter, r *http.Request)`) bridges the two layers: `internal/listener/quic.go`'s `serveQUICConnection` (61.1 handshake-only) is extended to find the accepted connection's chain terminal filter, assert it to `network.H3Terminal`, and drive `http3.Server{Handler: http.HandlerFunc(h3t.ServeH3)}.ServeQUICConn(conn)`. quic-go stays confined to `internal/listener/quic.go` (the 61.1 import-hygiene pin holds unchanged — `internal/filter/hcm` gains ZERO quic-go import). The HCM `*Filter.ServeH3` → `runH3` dispatch arm is modeled on the H1 `dispatchRequest` (the request side is a native `*http.Request`, so it reuses the H1 `router.Action` path — plausibly ZERO `router.go` changes) with a new `writeH3Reply` response adapter (`ActionResponse → w.Header()/WriteHeader/Write`) and a new `emitAccessLogH3` arm (`Protocol="HTTP/3"`). The `codec_type HTTP3` reject is lifted, gated on a new `IsQUIC` context flag threaded `network.FactoryCtx → hcm.ListenerCtx` from the listener `kind`; `codec_type HTTP3` on a non-QUIC listener becomes a config-parity boot-reject.

**Tech Stack:** Go; the `github.com/quic-go/quic-go v0.54.1` module's `http3` package (`http3.Server`/`ServeQUICConn`, landed at 61.1 — confined to `internal/listener/quic.go`); `net/http` (`*http.Request`/`http.ResponseWriter`/`http.HandlerFunc` — the stdlib handler contract the H3 arm speaks); the EXISTING `internal/filter/hcm` dispatch machinery (`routeTable.match`, `entry.action.asRouterAction()`, `filter_http.NewFilterChain`, `router.Filter.SetAction/SetRequest/RunAction`, `ActionResponse`) reused byte-for-byte; `internal/filter/network` (the new `H3Terminal` interface). ZERO new production Go packages (the H3 arm folds into `hcm` as `h3dispatch.go`); +0 go.mod modules (quic-go landed at 61.1).

---

## Global Constraints

- **One stage = leg 61.2 only (the H3 codec + HCM adapter).** This PLAN is the 61.2 IMPL decomposition. Row 61 STAYS `in-progress` at its six-gate — it flips `done` only when ALL THREE legs land (61.1 substrate + 61.2 codec/HCM + 61.3 fixture/harness), ADR-0106 / `reference_roadmap_split_phase_row_done`. The HTTP/3 FAMILY STAYS OPEN after phase-done.
- **SUBJECT-SIDE ONLY in 61.2.** The proof is a local quic-go `http3.Transport` client GET→200 against the subject's bound QUIC listener. The CROSS-SIDE differential fixture (`0102-http3-downstream-get`), the harness UDP-port surgery (three container-starters), and the shared `test/helpers/h3.go` `H3RoundTrip` are leg 61.3. Do NOT add a differential fixture or touch `test/differential/harness.go` in 61.2.
- **quic-go STAYS confined to `internal/listener/quic.go`.** The H3 arm in `internal/filter/hcm` speaks ONLY stdlib `net/http` types. Verify at Task 7 + Task 9: `go list -deps ./internal/filter/hcm | grep -i quic-go` prints NOTHING (echo `HCM-NO-QUICGO`); `internal/tls` stays quic-go-free (`TLS-NO-QUICGO`, the 61.1 gate). quic-go's `http3` import appears in exactly ONE production file: `internal/listener/quic.go`.
- **NO new production Go package.** The H3 arm folds into the `hcm` package as `h3dispatch.go` (methods on `*Filter` needing the private `chainConfig`/`table`/`perRouteConfig`/`downstreamStatusClassCounter`/`emitAccessLog*` internals). This REFINES SPEC-61 §4/§12's "+1 `internal/filter/hcm/h3` package" estimate: because quic-go OWNS the H3 codec (framing + QPACK), there is no hand-rolled codec to house in a sibling package — the 61.1 precedent (SPEC estimated an `internal/listener/quic` package; the IMPL folded it into `quic.go`, +0). New production packages: **+0**.
- **ZERO `router.go` changes (target).** The H3 request side reuses the H1 `router.Action` path (`func(ctx, req *http.Request) (ActionResponse, Endpoint, error)`, `router.go:116`), NOT `router.H2Action` — the router's `Action`/`H2Action` split is about the UPSTREAM protocol, not the downstream codec. `SetAction`/`SetRequest`/`RunAction` (`router.go:222/226/297`) are reused unchanged. If the IMPL discovers a genuine `router.go` need, record it in ADR-0281 as a departure from this target.
- **TDD (`superpowers:test-driven-development`):** every code task is failing-test → run-fail → minimal-impl → run-pass → commit. NO production code without a failing test first. The `writeH3Reply` adapter (Task 4) and the `runH3` arm (Task 5) are unit-tested with `httptest.NewRecorder()` + `httptest.NewRequest()` — NO quic-go needed; only the Task 8 subject-side integration test uses the real quic-go `http3.Transport`.
- **Per-task gates (`feedback_pertask_gofmt_lint`):** every code task ends with `gofmt -l` (expect empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. Do NOT skip gofmt.
- **Worktree hygiene (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`):** subagents write to the WORKTREE path (`.worktrees/phase-61.2-impl/…`); the controller verifies `git -C <main-checkout> status` stays clean after each task and the worktree branch is unchanged (no detached HEAD). Pin worktree-relative paths in every dispatch.
- **Commit locally only (`feedback_subagents_no_push`):** subagents NEVER push; the controller squashes + pushes at stage-close.
- **Distinct reject substrings (ADR-0080).** The two config-parity rejects lifted/added in Task 3 (`codec_type HTTP3 requires a QUIC (udp_listener_config) listener` for HTTP3-on-non-QUIC; the existing `codec_type HTTP2 requires TLS…` untouched) each carry a distinct substring. HTTP3-on-non-QUIC is a config-PARITY reject (both the reference and envoy-go reject — SPEC-61 §11 arm reject-B), NOT an envoy-go-strict departure.
- **`reference_fatalf_makes_assertions_unreachable`:** in tests asserting multiple independent properties (the GET→200 test asserts status + body + protocol), use `Errorf` per property; `Fatalf` only for a broken precondition (dial failed, Start failed, request errored).
- **`reference_ondestroy_fires_once_encoder_unreachable`:** the H3 arm builds a fresh `filter_http.FilterChain` per request and `defer chain.Destroy()` — identical to the H1/H2 arms. Do NOT introduce a second Destroy path; the per-request chain owns its own lifecycle. (This memory trap bites HTTP-FILTER OnDestroy install-in-both-fields; the H3 arm reuses the existing per-request chain machinery, so it inherits the correct semantics — DO NOT re-plumb it.)
- **RE-DERIVE every `file:line` against the master tip at IMPL start (`feedback_brief_citations_not_evidence`).** This PLAN's citations were RE-DERIVED against master tip `222dbee2` (the phase-61.1 IMPL) this PLAN session. The hcm package was UNTOUCHED by 61.1, so the SPEC-61 hcm citations (`h2dispatch.go:269`, `config.go:231-240`, `config_test.go:207-208`) are STILL ACCURATE; the listener package shifted (`serveQUICConnection` now lives at `internal/listener/quic.go:92`, the 61.1 landed file). Re-confirm before editing.
- **ADR body lands at THIS IMPL (ADR-0044):** ADR-0281 §Decision/§Consequences are authored at this 61.2 IMPL. §Context is drafted at this IMPL too (SPEC-61 §13 was ADR-0279's Context; ADR-0281 is a NEW per-leg ADR for the codec-arm seam — see the "ADR-0281 decision" pin). DECISIONS tail **ADR-0279 → ADR-0281** (next-free after this IMPL is **ADR-0282**).
- **Counts at 61.2 exit:** fixtures **105** (+0 — the cross-side H3-GET `0102` is leg 61.3) · fuzzers **55** (+0 — quic-go owns H3 framing/QPACK; the only hand-rolled parse is the bootstrap config, reachable from the existing listener/HCM parse — no new `func Fuzz`) · BackendKind **38** (+0 — the subject-side test reuses an existing responder or `direct_response`) · stat surface **1201** (+0 RECOMMENDED — the H3 arm reuses the codec-agnostic `downstream_rq_<Nxx>`/`downstream_rq_completed` counters via `downstreamStatusClassCounter`; the reference's `downstream_{cx,rq}_http3_total` are DEFERRED to 61.3/robustness, asserted as a NAMED SUBSET per `reference_stats_sink_emits_used_only`; IMPL may pin +2 if trivial) · new production Go packages **+0** · new go.mod modules **+0** (quic-go landed at 61.1) · DECISIONS tail **ADR-0281**.

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. Leg **61.1** (LANDED) stood up envoy-go's FIRST UDP/QUIC downstream listen path: a listener whose `udp_listener_config.quic_options` marks it `kindQUIC`, which binds `net.ListenUDP`, stands a quic-go listener over it, and completes the QUIC/TLS-1.3 handshake negotiating ALPN `h3` — but does NOT serve HTTP. The accept path (`internal/listener/quic.go`) accepts a `*quic.Conn` and immediately closes it (`serveQUICConnection` is handshake-only). Leg **61.2** (THIS plan) makes it SERVE: decode an H3 request off the accepted QUIC connection and dispatch it into the EXISTING HCM → router → filter-chain → upstream path.

**The KEY insight that shapes the whole leg:** quic-go's `http3.Server` IS the HTTP/3 codec. It parses HTTP/3 frames + QPACK and invokes an `http.Handler` with a native `(*http.Request, http.ResponseWriter)` — the exact stdlib contract. So the HCM's "H3 codec arm" is NOT a codec (there are no frames to parse — quic-go did that) and NOT a quic-go importer (it speaks stdlib `net/http`). It is a per-request DISPATCH method that runs the existing filter chain from an `*http.Request` and writes the `ActionResponse` to an `http.ResponseWriter`. The three existing arms and where each lives:

- **H1** — `runConnection` (`connection.go:152`) → `dispatchRequest(ctx, downstream net.Conn, req *http.Request, bw *bufio.Writer)` (`connection.go:312`). The request side is a NATIVE `*http.Request` off the H1 codec; the response is written to a `*bufio.Writer` via `writeH1Reply`/`writeStatusReply` (`codec.go:74`/`codec.go:33`). **This is the closest model for the H3 arm** — same `*http.Request` request side, same `router.Action` path, same chain build + encode-chain; only the wire-write differs.
- **H2** — `runH2` (`filter.go:153`) captures per-conn TLS/addr state onto an `h2Dispatcher`, then per stream `chainDispatchAction.WriteH2(ctx, h2req h2.H2Request, sw h2.StreamWriter)` (`h2dispatch.go:269`) — the `net.Conn`-free existence proof. The H2 request is an `h2.H2Request` (not `*http.Request`), so H2 needs the `internal/filter/hcm/h2` codec package. **H3 does NOT** — quic-go hands a real `*http.Request`.
- **The shared core** (`filter_http.FilterChain` over `http.Header`, `router.Filter`, `router.ActionResponse`) is codec-agnostic and untouched by codec choice.

**The dispatch machinery you REUSE (RE-DERIVED against master tip `222dbee2` — RE-CONFIRM at IMPL):**
- `Filter.Handle(ctx, downstream net.Conn)` (`filter.go:112`) — the top-level codec fork: `switch f.codecType` → `HTTP1 → runConnection` (`:121`), `HTTP2 → f.runH2` (`:124`), `AUTO → ALPN peek` (`:126`). **NOTE:** `Handle` takes a `net.Conn` and is the TCP-path entry. The H3 arm does NOT go through `Handle` — QUIC has no `net.Conn`. The H3 entry is a NEW `ServeH3(w, r)` method invoked by quic-go's `http3.Server`, NOT by `Handle`. (SPEC-61 §12's "`filter.go:112` ADD a `codecType == HTTP3` branch" is a RED HERRING re-derived here: `Handle` is never called on the QUIC path, so no branch is added there — the H3 entry is the `ServeH3` seam. Record this correction in PROGRESS.)
- `routeTable.match(req *http.Request) (*routeEntry, int, bool)` (`route.go:127`) — the route match, taking a native `*http.Request`. Reused verbatim by the H3 arm.
- `entry.action.asRouterAction() router.Action` (`actions.go:147`/`:211`/`:244`) — builds the H1-flavored `router.Action` closure from the matched route (`directResponseAction`/`clusterRouteAction`/`weightedClusterRouteAction`). The H3 arm calls `asRouterAction()` (the H1 flavor), NOT `asRouterActionH2()`.
- `filter_http.NewFilterChain(chainHF, f.perRouteConfig)` + the `chain.SetX` seeders (`SetRequestCtx`, `SetTLSPrincipals`, `SetTLSConnectionState`, `SetDownstreamRemoteAddr/LocalAddr`, `SetDownstreamTLSServerName/PeerCertDER`, `SetDownstreamProtocol`, `SetListenerPrincipal`, `SetRouteRateLimits`/`SetVirtualHostRateLimits`/`SetRouteMetadata`/`SetRouteIncludeVhRateLimits`) — the per-request chain build, IDENTICAL to `dispatchRequest`'s (`connection.go:346-383` region) and `WriteH2`'s (`h2dispatch.go:323-382`).
- `rf.SetAction(action)` + `rf.SetRequest(req)` (`router.go:222`/`:226`) → `chain.RunDecodeHeaders(ctx, req.Header, endStream)` → `rf.RunAction(ctx)` (`router.go:297`) → `rf.Response()`/`rf.Picked()`/`rf.ActionErr()`/`rf.ActionRan()` → the encode chain (`chain.RunEncodeHeaders`/`RunEncodeData`, `connection.go:731-757`) → the wire write.
- `router.ActionResponse{Status int, Headers envoyhttp.OrderedHeaders, Body []byte, Close bool}` (`router.go:79`) — the codec-agnostic response the wire-write consumes. `Close` is H1-only (ignored on H3, like H2).
- `f.downstreamStatusClassCounter(code int) *stats.Counter` (`config.go:188`) — Inc the `downstream_rq_<Nxx>` bucket. Codec-agnostic; the H3 arm Inc's it exactly as H1/H2 do (this is why +0 stat surface — the request-class counters already exist).
- `f.emitAccessLog(r *http.Request, …)` (`accesslog_emit.go:25`, H1, `Protocol: r.Proto`) / `f.emitAccessLogH2(req h2.H2Request, …)` (`accesslog_emit.go:85`, `Protocol: "HTTP/2.0"`) — the two existing access-log/span arms. The H3 arm adds a third, `emitAccessLogH3(r *http.Request, …, Protocol="HTTP/3")`.

**The response-projection recipe you MIRROR (`connection.go:731-763`, the H1 encode+write tail):**
```
status := resp.Status
if rf.ActionRan() && status > 0 && actionErr == nil {
    merged := resp.Headers.ToHTTPHeader()          // filter_http/types.go:128
    chain.SetEncodeResponseStatus(status)
    chain.RunEncodeHeaders(ctx, merged, len(resp.Body) == 0)
    resp.Headers = filter_http.ReconcileOrderedHeaders(resp.Headers, merged)  // types.go:206
    if len(resp.Body) > 0 { chain.RunEncodeData(ctx, resp.Body, true) }
    if override, ok := chain.EncodeBodyOverride(); ok { resp.Body = override }
}
// then wire-write: H1 uses writeH1Reply(bw, status, resp.Headers, resp.Body);
// H3 uses writeH3Reply(w, status, resp.Headers, resp.Body)  (Task 4)
```

**The listener seam you WIRE (RE-DERIVED against `222dbee2` — the 61.1 landed code):**
- `internal/listener/quic.go:92` — `serveQUICConnection(ctx context.Context, conn *quic.Conn)` — the 61.1 handshake-only terminal: `defer rt.downstreamCxActive.Dec(); _ = ctx; _ = conn.CloseWithError(0, "")`. Task 7 REPLACES the body with the `http3.Server` serve.
- `internal/listener/quic.go:52` — `quicTLSConfig() *stdtls.Config` — returns the single chain's `*stdtls.Config` (defaultChain-first, then first non-nil `chainByName`). Task 7 adds a SIBLING `quicChain() *chainInfo` returning the single chain (same selection logic) so `serveQUICConnection` can reach its `netChainFactory`.
- `internal/listener/manager.go` — `chainInfo{serverNames, tlsCfg, netChainFactory}` (`:107`); `listenerRuntime.defaultChain *chainInfo` (`:155`) + `chainByName map[string]*chainInfo`; `netChainFactory func() []network.NetworkFilter` (`:117`) — the per-chain factory that allocates the network-filter slice; the LAST filter is the terminal (validated `network.TerminalFilter`, `buildNetworkChainFactory` `:709`/`:724`). `serveQUICConnection` calls `ci.netChainFactory()`, takes `filters[len-1]`, and asserts `network.H3Terminal`.
- `network.FactoryCtx` (`internal/filter/network/types.go:142`) — carries `BaseDir`/`HasTLS`/`AllowH2C`/`ListenerPrincipal`; built at `manager.go:494` (filter_chains[]) + `:565` (default_filter_chain). Task 2 adds `IsQUIC bool`, set from `kind == kindQUIC`.
- `hcm.ListenerCtx` (`internal/filter/hcm/config.go:60`) — carries `HasTLS`/`AllowH2C`/`ListenerPrincipal`/`HTTPClient`/`NodeServiceCluster`; bridged from `FactoryCtx` in `NewNetworkFactory` (`filter.go:46-57`). Task 2 adds `IsQUIC bool`, bridged from `ctx.IsQUIC`.

**The quic-go `http3` API (v0.54.1, VERIFIED this PLAN session via `go doc` — RE-CONFIRM at IMPL):**
- `http3.Server{Handler http.Handler, TLSConfig *tls.Config, QUICConfig *quic.Config, EnableDatagrams bool, …}` — set `Handler` to `http.HandlerFunc(h3t.ServeH3)`. `TLSConfig`/`QUICConfig` are used by the `ListenAndServe`/`Serve` methods; `ServeQUICConn` serves over an already-accepted, already-handshaked conn, so those fields are advisory here — set `TLSConfig` from the chain for completeness (the IMPL verifies whether `ServeQUICConn` requires it non-nil).
- `(*http3.Server).ServeQUICConn(conn *quic.Conn) error` — "serves a single QUIC connection." BLOCKS serving requests on the conn until it closes; returns the terminal error. This is the analog of the TCP path's `terminal.Handle(ctx, conn)` blocking serve loop.
- The handler's `*http.Request`: `req.TLS *tls.ConnectionState` is populated (quic-go sets it — the source for the H3 arm's TLS-principal seeds, replacing H1's `downstream.(*stdtls.Conn)` assertion); `req.RemoteAddr`/`req.Host`/`req.Header`/`req.Method`/`req.URL`/`req.Body` are native; `req.Proto` is `"HTTP/3.0"`, `req.ProtoMajor==3`.

### Discipline (honor on EVERY task) — the memory traps that bite this row
- **`feedback_brief_citations_not_evidence`** — RE-DERIVE every `file:line` against the master tip; the numbers above are from `222dbee2`. The hcm citations are stable (61.1 did not touch hcm); the listener citations are the 61.1 landed code.
- **`feedback_pertask_gofmt_lint`** — `gofmt -l` + `golangci-lint run` on touched packages every task.
- **`reference_fatalf_makes_assertions_unreachable`** — `Errorf` per independent property in the GET→200 + `writeH3Reply` tests.
- **`reference_ondestroy_fires_once_encoder_unreachable`** — the H3 arm reuses the per-request chain lifecycle (`defer chain.Destroy()`); do NOT add a second Destroy path.
- **`reference_dynamic_stat_name_charset_guard`** — IF the IMPL adds an `http3.*` or `downstream_*_http3_total` counter (the +2 option), any wire-derived segment passes `stats.IsValidName` before `NewCounterIfAbsent` (which PANICS). The recommended +0 path (reuse existing counters) does not touch this.
- **`reference_roadmap_split_phase_row_done`** — row 61 STAYS `in-progress`; it flips `done` only at the 61.3 (final-leg) IMPL.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — after 61.2 the HTTP/3 deferred sentence STAYS exactly one live "candidates:" match (the family stays open).
- **`reference_docker_probe_bridge_network` / `reference_host_gateway_ip_docker_desktop`** — NOT relevant to 61.2 (no Docker/differential; the GET→200 test is an in-process local quic-go client). These bite 61.3.

---

## Design pins settled here (the 61.2 D-question resolutions over SPEC-61 + PLAN-61.1)

**H3 SEAM = a stdlib-typed dispatch arm, NOT a codec package.** quic-go's `http3.Server` owns HTTP/3 framing + QPACK and yields `(*http.Request, http.ResponseWriter)`. So the HCM arm is a per-request dispatch method (`ServeH3`→`runH3`) in a NEW same-package file `internal/filter/hcm/h3dispatch.go`, speaking ONLY stdlib `net/http`. NO `internal/filter/hcm/h3` package (+0 packages — refines SPEC §4/§12 exactly as 61.1 refined the `internal/listener/quic` package into `quic.go`). Rejected alternative: a sibling `internal/filter/hcm/h3` codec package — there is no hand-rolled codec to house (quic-go is the codec), so a package would hold one dispatch method that needs `*Filter`'s private internals, forcing exports for no gain.

**BRIDGE = a new `network.H3Terminal` interface.** `internal/filter/network/terminal.go` gains `type H3Terminal interface { TerminalFilter; ServeH3(w http.ResponseWriter, r *http.Request) }` (stdlib-only — the network package gains a `net/http` import). `*hcm.Filter` implements `ServeH3`. `internal/listener/quic.go`'s `serveQUICConnection` finds the chain terminal filter (`filters[len-1]`), asserts `network.H3Terminal`, and drives `http3.Server{Handler: http.HandlerFunc(h3t.ServeH3)}.ServeQUICConn(conn)`. Rejected alternatives: (a) `internal/listener` importing `internal/filter/hcm` directly — a layering violation (the listener works through the `network` abstraction; the HCM filter is injected as a `network.NetworkFilterFactory` at boot); (b) `*Filter` implementing `http.Handler` (`ServeHTTP`) directly — overloads the stdlib handler name with HCM-specific semantics; an explicit `ServeH3` is clearer and wrapped with `http.HandlerFunc` at the one call site. NO cycle: neither `hcm` nor `network` imports `listener` (VERIFIED this PLAN session).

**REQUEST SIDE reuses the H1 `router.Action` path.** `runH3` mirrors `dispatchRequest` (`connection.go:312`): `f.table.match(req)` → `entry.action.asRouterAction()` (the H1 `router.Action`, NOT `H2Action`) → build chain → seed → `rf.SetAction`/`SetRequest` → `RunDecodeHeaders(req.Header, endStream)` → `RunAction` → encode chain → `writeH3Reply`. Because `router.Action` is `func(ctx, *http.Request) (ActionResponse, Endpoint, error)` and the H3 request IS an `*http.Request`, ZERO `router.go` changes (target). TLS seeds come from `req.TLS` (quic-go populates it) rather than H1's `downstream.(*stdtls.Conn)`. `SetDownstreamProtocol("HTTP/3")`.

**RESPONSE SIDE = `writeH3Reply` adapter (NOT `writeH1Reply`).** `writeH1Reply(w io.Writer, …)` (`codec.go:74`) writes the HTTP/1.1 wire format (status line + headers + CRLF framing) — WRONG for H3 (quic-go's `http.ResponseWriter` owns H3 framing/QPACK). `writeH3Reply(w http.ResponseWriter, status int, headers filter_http.OrderedHeaders, body []byte) error` projects `ActionResponse` via the stdlib writer: set `w.Header()` from `headers` (skip pseudo-headers like `:status`), `w.WriteHeader(status)`, `w.Write(body)`. Mirrors `writeH2Reply` (`h2dispatch.go:671`) structurally but over `http.ResponseWriter`.

**reject-B (HTTP3-on-non-QUIC) via `lc.IsQUIC`.** Thread the listener `kind` into the HCM parse context: `network.FactoryCtx.IsQUIC` (set at `manager.go:494`/`:565` from `kind == kindQUIC`) → `hcm.ListenerCtx.IsQUIC` (bridged in `NewNetworkFactory`, `filter.go:46-57`). In `parseFilterWithCtx`, `codec_type HTTP3` is ACCEPTED iff `lc.IsQUIC`; on a non-QUIC listener → config-parity reject `hcm: codec_type HTTP3 requires a QUIC (udp_listener_config) listener` (SPEC-61 §11 arm reject-B — both sides reject). Lift the existing `config.go:240` blanket reject (`codec_type %s is not supported in phase 05.1`). Rejected alternative: accepting HTTP3 unconditionally + rejecting at listener-build — the parse-time reject with `IsQUIC` context matches the reference's boot-reject timing and keeps the reject in the HCM parser beside the HTTP2-requires-TLS sibling (`config.go:237`).

**emitAccessLogH3 — a thin third arm, `Protocol="HTTP/3"`.** A sibling of `emitAccessLogH2` (`accesslog_emit.go:85`) reading from `*http.Request` (like `emitAccessLog`, `:25`) but forcing `Protocol="HTTP/3"` (both the access-log `Record.Protocol` and the span `SpanInputs.Protocol`). Rejected alternative: reusing `emitAccessLog` (H1) with `r.Proto` — quic-go sets `r.Proto="HTTP/3.0"`, but the reference's `%PROTOCOL%` / Lua `:protocol()` emit `"HTTP/3"` (SPEC-61 §8); a dedicated arm controls the exact string. The 61.3 differential VERIFIES the exact access-log/span protocol string cross-side; 61.2 pins `"HTTP/3"` and flags it for 61.3 confirmation.

**STAT SURFACE +0 (recommended).** The H3 arm Inc's the EXISTING codec-agnostic `downstream_rq_<Nxx>`/`downstream_rq_completed` counters via `downstreamStatusClassCounter` (`config.go:188`) — no new surface. The reference's `downstream_cx_http3_total`/`downstream_rq_http3_total`/`http3.*` family is DEFERRED (asserted as a NAMED SUBSET at 61.3, `reference_stats_sink_emits_used_only`). The IMPL MAY register `downstream_{cx,rq}_http3_total` (+2 surface, `stats.IsValidName`-guarded) if trivial — pin the actual N at the IMPL. Recommended: +0 (the minimal GET→200 slice needs none), matching 61.1's +0 posture.

**serveQUICConnection ctx-honoring (M6-2 pickup).** The 61.1 review MINOR M6-2 (`serveQUICConnection` discards `ctx`) is NATURALLY picked up here: Task 7's `ServeQUICConn` serve path honors the connection lifecycle; thread `ctx` for shutdown (a `context`-canceled serve closes the conn). M6-1 (`quicAcceptLoop` no TCP-style backoff) is UNCHANGED by 61.2 — re-defer it explicitly to a QUIC-robustness row (record in PROGRESS). M-FB1/M-FB2 (multi-chain/`default_filter_chain` QUIC + `quicTLSConfig` map-nondeterminism) stay 61.3/robustness pickups.

**ADR-0281 decision — a NEW per-leg ADR for the codec-arm seam.** The 61.2 codec arm introduces a genuinely distinct architectural seam from 61.1's listen substrate: the `network.H3Terminal` bridge, the stdlib-typed dispatch arm (quic-go-free HCM), and the `http3.Server` wiring. This WARRANTS its own ADR (ADR-0281) rather than folding into ADR-0279 (the listen-substrate ADR) — per SPEC-61 §3.0's "a per-leg ADR (the codec-arm seam) OR fold into ADR-0279 — the PLAN/IMPL decide." §Context + §Decision + §Consequences all land at this IMPL (ADR-0044). Next-free after: ADR-0282.

---

## File structure (decomposition locked here)

**Production (created):**
- `internal/filter/hcm/h3dispatch.go` — CREATE: `(*Filter).ServeH3(w http.ResponseWriter, r *http.Request)` (the `http.Handler`-shaped entry) → `(*Filter).runH3(ctx, w, r)` (the dispatch arm modeled on `dispatchRequest`) + `writeH3Reply(w http.ResponseWriter, status int, headers filter_http.OrderedHeaders, body []byte) error` (the response adapter). Stdlib `net/http` only; ZERO quic-go import (Tasks 4/5/6).

**Production (modified):**
- `internal/filter/network/terminal.go` — ADD `type H3Terminal interface { TerminalFilter; ServeH3(w http.ResponseWriter, r *http.Request) }` + the `net/http` import (Task 6).
- `internal/filter/network/types.go` — ADD `IsQUIC bool` to `FactoryCtx` (`:142`) (Task 2).
- `internal/filter/hcm/config.go` — ADD `IsQUIC bool` to `ListenerCtx` (`:60`); LIFT the `codec_type HTTP3` reject + add the HTTP3-on-non-QUIC parity reject in `parseFilterWithCtx` (`:231-240`) (Tasks 2/3).
- `internal/filter/hcm/filter.go` — BRIDGE `IsQUIC: ctx.IsQUIC` in `NewNetworkFactory`'s `ListenerCtx` literal (`:49-55`) (Task 2). (NO `Handle` branch — the H3 entry is `ServeH3`, not `Handle`.)
- `internal/filter/hcm/accesslog_emit.go` — ADD `(*Filter).emitAccessLogH3(r *http.Request, …)` (sibling of `:85` `emitAccessLogH2`, `Protocol="HTTP/3"`) (Task 5).
- `internal/listener/manager.go` — SET `IsQUIC: kind == kindQUIC` in the two `network.FactoryCtx` literals (`:494`, `:565`) (Task 2).
- `internal/listener/quic.go` — ADD `quicChain() *chainInfo` (sibling of `quicTLSConfig` `:52`); REPLACE the handshake-only `serveQUICConnection` body (`:92`) with the `http3.Server{Handler}.ServeQUICConn` serve; ADD the `http3` import (the quic-go `http3` package — the ONLY new import site) (Task 7).

**Test (created / modified):**
- `internal/filter/network/terminal_test.go` (or the existing test file) — ADD a compile/assert test that `*hcm.Filter` (or a test double) satisfies `network.H3Terminal` (Task 6). *(NOTE: `network` must not import `hcm` — the assert test lives in the `hcm` package or uses a local test double implementing the interface. RE-DERIVE the cleanest placement at IMPL; a `var _ network.H3Terminal = (*Filter)(nil)` compile-assert in `internal/filter/hcm/h3dispatch.go` is the simplest and needs no test file.)*
- `internal/filter/hcm/h3dispatch_test.go` — CREATE: `TestWriteH3Reply_*` (Task 4, `httptest.NewRecorder`) + `TestRunH3_GET_DirectResponse` / `TestRunH3_GET_RoutedBackend` / `TestRunH3_NoMatch_404` (Task 5, `httptest.NewRequest` + `httptest.NewRecorder` — NO quic-go).
- `internal/filter/hcm/config_test.go` — MODIFY `TestParseFilter_CodecTypeHTTP3` (`:207-208`): was HTTP3-rejected-blanket; now → REJECTED on a non-QUIC listener (`IsQUIC=false`) with the parity substring + ACCEPTED on a QUIC listener (`IsQUIC=true`). ADD `TestParseFilter_CodecTypeHTTP3_AcceptsOnQUICListener` (Task 3).
- `internal/listener/quic_test.go` — ADD `TestQUICListener_ServesH3GET` (Task 8, the subject-side integration proof: a local quic-go `http3.Transport` client GET → 200 + body). RE-DERIVE the 61.1 `mkQUICListener` helper — extend it to carry a real HCM (codec_type HTTP3 + a `direct_response` or a routed cluster) rather than the 61.1 handshake-only `mkTcpProxyFilter`.

**Test fixtures/certs:** REUSE the 61.1 committed cert/key helper (`mkQUICListener` already loads one). NO new cert.

**Docs (this IMPL):**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — EXTEND the HTTP/3 section (61.1 wrote the substrate paragraph): H3 GET is now SERVED into the HCM/router/filter path; the HTTP3-on-non-QUIC config-parity reject (Task 9).
- `docs/envoy-go/DECISIONS.md` — ADR-0281 §Context/§Decision/§Consequences (Task 9).
- `docs/envoy-go/phases/61-http3-downstream-listener/PROGRESS-61.2.md` — the checklist (updated each task).
- `docs/envoy-go/STATE.md` — active-phase header (Task 9, controller).
- `docs/envoy-go/ROADMAP.md` — row 61 STAYS `in-progress` (NO flip in 61.2).
- `next-prompt.txt` — the router roll to the 61.2 IMPL → 61.3 PLAN (Task 9, folded into the squash; `reference_next_prompt_tracked_despite_gitignore`).

---

## Task 1: PROGRESS scaffold + baselines + design pins

**Files:**
- Create: `docs/envoy-go/phases/61-http3-downstream-listener/PROGRESS-61.2.md`

- [ ] **Step 1: Author `PROGRESS-61.2.md`** — the baseline-counts table (all +0 except DECISIONS tail → ADR-0281), the import-hygiene note (quic-go stays confined to `internal/listener/quic.go`; the H3 arm is quic-go-free), the 61.2 design pins (copied from "Design pins settled here"), the M6-1/M6-2 review-pickup disposition, and the Task checklist (Tasks 1–9 unchecked). Model it on `PROGRESS-61.1.md`. (This step is folded into the PLAN commit — no separate code.)

- [ ] **Step 2: Commit** (folded into the PLAN stage commit by the controller).

---

## Task 2: Thread `IsQUIC` through `network.FactoryCtx` → `hcm.ListenerCtx` (prep for the reject + the H3 protocol)

**Files:**
- Modify: `internal/filter/network/types.go:142` (add `IsQUIC bool` to `FactoryCtx`)
- Modify: `internal/filter/hcm/config.go:60` (add `IsQUIC bool` to `ListenerCtx`)
- Modify: `internal/filter/hcm/filter.go:49-55` (bridge `IsQUIC: ctx.IsQUIC` in `NewNetworkFactory`)
- Modify: `internal/listener/manager.go:494,565` (set `IsQUIC: kind == kindQUIC` in both `FactoryCtx` literals)
- Test: `internal/listener/manager_test.go` (assert a QUIC listener's HCM sees `IsQUIC`)

**Interfaces:**
- Produces: `network.FactoryCtx.IsQUIC` + `hcm.ListenerCtx.IsQUIC` (both `bool`); a QUIC listener's HCM filter is parsed with `lc.IsQUIC == true`.
- Consumes: the 61.1 `listenerKind`/`kindQUIC` (`manager.go`).

- [ ] **Step 1: Write the failing test.** The cleanest observable is that a QUIC listener carrying an HCM parses with `IsQUIC=true`. Because 61.2 Task 3 makes `codec_type HTTP3` accepted-on-QUIC / rejected-off-QUIC, the end-to-end proof is deferred to Task 3; for Task 2, a focused test that a QUIC listener builds an HCM chain (using `codec_type: AUTO` or `HTTP1`, which needs no HTTP3 accept) while a NON-QUIC HCM sees `IsQUIC=false` is sufficient. In `manager_test.go`, add:

```go
// TestBuildListenerRuntime_QUICThreadsIsQUIC verifies the listener kind is
// threaded into the HCM parse context: a QUIC listener's network.FactoryCtx
// (and thus hcm.ListenerCtx) carries IsQUIC=true. Observed here via a test
// hook: the HCM parser records lc.IsQUIC (RE-DERIVE the least-invasive probe —
// e.g. a package-level test-only capture in NewNetworkFactory, or assert the
// downstream effect once Task 3 lands). Prefer asserting via the Task-3 reject
// behavior if a direct probe is intrusive.
func TestBuildListenerRuntime_QUICThreadsIsQUIC(t *testing.T) {
	// Build a QUIC listener with an HCM whose codec_type is AUTO (no HTTP3
	// accept needed yet). Assert NewManager succeeds and the chain built.
	// The load-bearing assertion (IsQUIC actually reached the HCM) is proven
	// end-to-end by Task 3's accept-on-QUIC test; this task's test guards the
	// plumbing compiles + the QUIC listener still builds with an HCM chain.
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListenerHCM(t, cm, testCertPEM, testKeyPEM, hcmv3.HttpConnectionManager_AUTO)
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	if _, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry()); err != nil {
		t.Fatalf("NewManager(quic+hcm): %v", err)
	}
}
```

*(RE-DERIVE `mkQUICListenerHCM` — a 61.1 `mkQUICListener` variant whose filter chain carries a real HCM `typed_config` (codec_type param) instead of the 61.1 handshake-only tcp_proxy filter. The 61.1 `mkQUICListener` built a QUIC listener with a resolvable-but-never-dispatched filter; 61.2's HCM-carrying variant is the base for Tasks 3/8. Model the HCM typed_config on the existing `mkHCMFilter`/`mkTcpProxyFilter` test helpers.)*

- [ ] **Step 2: Run to verify RED.** `go test ./internal/listener/ -run 'TestBuildListenerRuntime_QUICThreadsIsQUIC' -count=1 -v` — FAIL to COMPILE (`IsQUIC` undefined on `FactoryCtx`) or FAIL if `mkQUICListenerHCM` cannot build (HTTP3 not yet accepted — use AUTO to avoid that until Task 3).

- [ ] **Step 3: Implement the thread.**
  (a) `internal/filter/network/types.go` — add to `FactoryCtx` (after `ListenerPrincipal`, `:151`):
```go
	IsQUIC bool // listener kind == kindQUIC (phase 61.2 — gates codec_type HTTP3 accept)
```
  (b) `internal/filter/hcm/config.go` — add to `ListenerCtx` (after `AllowH2C`, `:62`):
```go
	IsQUIC bool // enclosing listener is QUIC/H3 (phase 61.2 — gates codec_type HTTP3)
```
  (c) `internal/filter/hcm/filter.go` — bridge in `NewNetworkFactory`'s `ListenerCtx` literal (`:49-55`):
```go
			ListenerCtx{
				HasTLS:             ctx.HasTLS,
				AllowH2C:           ctx.AllowH2C,
				IsQUIC:             ctx.IsQUIC,
				ListenerPrincipal:  ctx.ListenerPrincipal,
				HTTPClient:         httpClient,
				NodeServiceCluster: ctx.NodeServiceCluster,
			},
```
  (d) `internal/listener/manager.go` — set in BOTH `network.FactoryCtx` literals (`:494` filter_chains[], `:565` default_filter_chain):
```go
		ncfCtx := network.FactoryCtx{
			// … existing fields …
			HasTLS:             chainTLS != nil,
			AllowH2C:           allowH2C,
			IsQUIC:             kind == kindQUIC,
			// …
		}
```
*(`kind` is the 61.1 `listenerKind` computed early in `buildListenerRuntimeWithCtx`. Confirm `kind` is in scope at both literal sites — if the `default_filter_chain` build is in a separate scope, thread `kind` in.)*

- [ ] **Step 4: Run to verify GREEN.** `go test ./internal/listener/ -run 'TestBuildListenerRuntime_QUICThreadsIsQUIC' -count=1 -v` — PASS. Then `go test ./internal/filter/network/ ./internal/filter/hcm/ ./internal/listener/ -count=1` (no regressions — `IsQUIC` defaults false, so every existing TCP path is unaffected).

- [ ] **Step 5: Per-task gates.** `gofmt -l internal/filter/network/ internal/filter/hcm/ internal/listener/` (empty) · `golangci-lint run ./internal/filter/network/... ./internal/filter/hcm/... ./internal/listener/...` · `go vet ./...` · `go build ./...`.

- [ ] **Step 6: Commit** — `phase 61.2: thread listener kind (IsQUIC) through network.FactoryCtx → hcm.ListenerCtx`.

---

## Task 3: Lift the `codec_type HTTP3` reject (gated on `IsQUIC`) + the HTTP3-on-non-QUIC config-parity reject

**Files:**
- Modify: `internal/filter/hcm/config.go:231-240` (the `parseFilterWithCtx` codec-type switch)
- Test: `internal/filter/hcm/config_test.go:207-208` (flip the reject test) + a positive-on-QUIC test

**Interfaces:**
- Consumes: `lc.IsQUIC` (Task 2).
- Produces: `codec_type HTTP3` ACCEPTED when `lc.IsQUIC` (sets `codecType = HTTP3`); REJECTED `hcm: codec_type HTTP3 requires a QUIC (udp_listener_config) listener` when `!lc.IsQUIC`.

- [ ] **Step 1: Flip the failing test.** In `config_test.go`, replace `TestParseFilter_CodecTypeHTTP3` (`:207-208`, which currently asserts the blanket reject) and add the positive-on-QUIC case. RE-DERIVE the `expectErr`/accept test helper — it currently builds an HCM with a default (non-QUIC) `ListenerCtx`; add a QUIC-`ListenerCtx` variant helper:

```go
// TestParseFilter_CodecTypeHTTP3_RejectsOnNonQUICListener verifies codec_type
// HTTP3 on a non-QUIC (TCP) listener is boot-rejected — config parity with the
// reference (SPEC-61 §11 arm reject-B: "HTTP/3 codec configured on non-QUIC
// listener.").
func TestParseFilter_CodecTypeHTTP3_RejectsOnNonQUICListener(t *testing.T) {
	expectErr(t, func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP3 },
		"codec_type HTTP3 requires a QUIC")
}

// TestParseFilter_CodecTypeHTTP3_AcceptsOnQUICListener verifies codec_type HTTP3
// is ACCEPTED when the enclosing listener is QUIC (lc.IsQUIC=true) — phase 61.2
// lifted the blanket reject.
func TestParseFilter_CodecTypeHTTP3_AcceptsOnQUICListener(t *testing.T) {
	f := parseFilterQUIC(t, func(h *hcmv3.HttpConnectionManager) { h.CodecType = hcmv3.HttpConnectionManager_HTTP3 })
	if f.codecType != hcmv3.HttpConnectionManager_HTTP3 {
		t.Errorf("codecType = %v, want HTTP3", f.codecType)
	}
}
```

*(RE-DERIVE `expectErr` — the existing helper builds with a default `ListenerCtx{}` (IsQUIC=false), so it already exercises the reject arm. `parseFilterQUIC` is a NEW helper that parses with `ListenerCtx{IsQUIC: true, HasTLS: true}` — model it on the existing `parseFilter`/`expectErr` helpers, passing the QUIC ListenerCtx. HasTLS=true because QUIC is mandatory-TLS.)*

- [ ] **Step 2: Run to verify RED.** `go test ./internal/filter/hcm/ -run 'TestParseFilter_CodecTypeHTTP3' -count=1 -v` — the non-QUIC test FAILS (current error is `codec_type HTTP3 is not supported in phase 05.1`, not `requires a QUIC`); the accept-on-QUIC test FAILS (still rejected).

- [ ] **Step 3: Lift + gate the reject.** In `config.go`, edit the codec-type switch (`:231-240`):
```go
	codecType := msg.GetCodecType()
	switch codecType {
	case hcmv3.HttpConnectionManager_HTTP2:
		if !lc.HasTLS && !allowH2C {
			return nil, fmt.Errorf("hcm: codec_type HTTP2 requires TLS transport_socket (or --allow-h2c for conformance testing)")
		}
	case hcmv3.HttpConnectionManager_HTTP3:
		// Phase 61.2 (ADR-0281): codec_type HTTP3 is served on a QUIC listener
		// (udp_listener_config.quic_options → kindQUIC → lc.IsQUIC). On a
		// non-QUIC listener it is a config-parity boot-reject (SPEC-61 §11 arm
		// reject-B — the reference rejects "HTTP/3 codec configured on non-QUIC
		// listener."). QUIC bakes TLS 1.3 into the transport, so no HasTLS check.
		if !lc.IsQUIC {
			return nil, fmt.Errorf("hcm: codec_type HTTP3 requires a QUIC (udp_listener_config) listener")
		}
	case hcmv3.HttpConnectionManager_HTTP1, hcmv3.HttpConnectionManager_AUTO:
		// accepted
	default:
		return nil, fmt.Errorf("hcm: codec_type %s is not supported in phase 05.1", codecType)
	}
```
*(RE-DERIVE the EXACT current switch shape at `:231-240` — the HTTP2 arm + the blanket `default` reject. The above splits HTTP3 out of the blanket reject into a gated arm. Confirm HTTP1/AUTO were already on the accept path (they fall through today); preserve that.)*

- [ ] **Step 4: Run to verify GREEN.** `go test ./internal/filter/hcm/ -run 'TestParseFilter_CodecType' -count=1 -v` — all PASS (HTTP1/HTTP2/AUTO unaffected; HTTP3 rejects off-QUIC, accepts on-QUIC). Then `go test ./internal/filter/hcm/ -count=1` (no regressions).

- [ ] **Step 5: Per-task gates.** `gofmt -l internal/filter/hcm/` · `golangci-lint run ./internal/filter/hcm/...` · `go vet ./...` · `go build ./...`.

- [ ] **Step 6: Commit** — `phase 61.2: lift codec_type HTTP3 reject (gated on IsQUIC) + HTTP3-on-non-QUIC config-parity reject`.

---

## Task 4: The `writeH3Reply` response adapter (`ActionResponse → http.ResponseWriter`)

**Files:**
- Create: `internal/filter/hcm/h3dispatch.go` (the `writeH3Reply` function + the file's package/imports)
- Test: `internal/filter/hcm/h3dispatch_test.go`

**Interfaces:**
- Produces: `func writeH3Reply(w http.ResponseWriter, status int, headers filter_http.OrderedHeaders, body []byte) error` — sets `w.Header()` from `headers` (skipping HTTP pseudo-headers), calls `w.WriteHeader(status)`, then `w.Write(body)`.

- [ ] **Step 1: Write the failing test.** In `h3dispatch_test.go`:

```go
package hcm

import (
	"net/http"
	"net/http/httptest"
	"testing"

	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
)

// TestWriteH3Reply_StatusHeadersBody verifies the ActionResponse → ResponseWriter
// projection: status code, response headers, and body are written; HTTP pseudo-
// headers (":status" etc.) are NOT leaked into the response header map.
func TestWriteH3Reply_StatusHeadersBody(t *testing.T) {
	rec := httptest.NewRecorder()
	hdrs := filter_http.OrderedHeaders{
		{Name: "content-type", Value: "text/plain"},
		{Name: "x-custom", Value: "v1"},
		{Name: ":status", Value: "200"}, // pseudo-header — must be dropped
	}
	if err := writeH3Reply(rec, 200, hdrs, []byte("h3-ok")); err != nil {
		t.Fatalf("writeH3Reply: %v", err)
	}
	res := rec.Result()
	if res.StatusCode != 200 {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("content-type"); got != "text/plain" {
		t.Errorf("content-type = %q, want text/plain", got)
	}
	if got := res.Header.Get("x-custom"); got != "v1" {
		t.Errorf("x-custom = %q, want v1", got)
	}
	if _, leaked := res.Header[":status"]; leaked {
		t.Errorf(":status pseudo-header leaked into the response header map")
	}
	if body := rec.Body.String(); body != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", body)
	}
}

// TestWriteH3Reply_EmptyBody verifies a headers-only response (no body) writes
// the status with no panic and an empty body.
func TestWriteH3Reply_EmptyBody(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := writeH3Reply(rec, 204, nil, nil); err != nil {
		t.Fatalf("writeH3Reply: %v", err)
	}
	if rec.Result().StatusCode != 204 {
		t.Errorf("status = %d, want 204", rec.Result().StatusCode)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body len = %d, want 0", rec.Body.Len())
	}
}
```

- [ ] **Step 2: Run to verify RED.** `go test ./internal/filter/hcm/ -run 'TestWriteH3Reply' -count=1 -v` — FAIL (`writeH3Reply` undefined).

- [ ] **Step 3: Implement.** Create `internal/filter/hcm/h3dispatch.go` (the arm functions land across Tasks 4/5; this task adds the file + `writeH3Reply`):
```go
package hcm

import (
	"net/http"
	"strings"

	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
)

// writeH3Reply projects a router.ActionResponse (status + ordered headers +
// body) onto quic-go's http.ResponseWriter. quic-go's http3.Server owns HTTP/3
// framing + QPACK below this seam (exactly as the internal h2 codec hides HTTP/2
// framing below h2.StreamWriter), so this adapter is a pure stdlib projection —
// NOT the HTTP/1.1 wire writer writeH1Reply (which emits a status line + CRLF
// framing wrong for H3). HTTP pseudo-headers (":status", ":path", …) are dropped
// — they are not real response headers. Phase 61.2, ADR-0281.
func writeH3Reply(w http.ResponseWriter, status int, headers filter_http.OrderedHeaders, body []byte) error {
	h := w.Header()
	for _, hf := range headers {
		if strings.HasPrefix(hf.Name, ":") {
			continue // pseudo-header — not a wire response header
		}
		h.Add(hf.Name, hf.Value)
	}
	w.WriteHeader(status)
	if len(body) > 0 {
		if _, err := w.Write(body); err != nil {
			return err
		}
	}
	return nil
}
```
*(RE-DERIVE the `filter_http.OrderedHeaders` field names (`Name`/`Value`) against `internal/filter/http/types.go` — confirm the struct shape used by `writeH2Reply` at `h2dispatch.go:671`. Confirm whether Add vs Set semantics matter for multi-value headers — mirror `writeH2Reply`. Confirm the pseudo-header skip matches how `writeH2Reply` handles `:status`.)*

- [ ] **Step 4: Run to verify GREEN.** `go test ./internal/filter/hcm/ -run 'TestWriteH3Reply' -count=1 -v` — PASS.

- [ ] **Step 5: Per-task gates.** `gofmt -l internal/filter/hcm/` · `golangci-lint run ./internal/filter/hcm/...` · `go vet ./...` · `go build ./...`.

- [ ] **Step 6: Commit** — `phase 61.2: writeH3Reply (ActionResponse → http.ResponseWriter adapter)`.

---

## Task 5: The `runH3` dispatch arm + `emitAccessLogH3` (modeled on H1 `dispatchRequest`)

**Files:**
- Modify: `internal/filter/hcm/h3dispatch.go` (add `ServeH3` + `runH3`)
- Modify: `internal/filter/hcm/accesslog_emit.go` (add `emitAccessLogH3`)
- Test: `internal/filter/hcm/h3dispatch_test.go` (add the dispatch tests)

**Interfaces:**
- Consumes: `f.table.match` (`route.go:127`), `entry.action.asRouterAction()` (`actions.go:147`), `filter_http.NewFilterChain` + the `chain.SetX` seeders, `router.Filter.SetAction/SetRequest/RunAction`, `f.downstreamStatusClassCounter`, `writeH3Reply` (Task 4).
- Produces: `(*Filter).ServeH3(w http.ResponseWriter, r *http.Request)` (the `http.HandlerFunc`-compatible entry) → `(*Filter).runH3(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error)`; `(*Filter).emitAccessLogH3(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders filter_http.OrderedHeaders, traceDecision *tracing.Decision)`.

- [ ] **Step 1: Write the failing tests.** In `h3dispatch_test.go` — use `httptest.NewRequest` + `httptest.NewRecorder` (NO quic-go). RE-DERIVE the test-`*Filter` builder from the existing `chain_dispatch_test.go`/`h2dispatch_test.go` helpers (they construct a `*Filter` with a route table + a `direct_response` or routed action + a router-only chain):

```go
// TestRunH3_GET_DirectResponse verifies a routed GET is dispatched through the
// chain and the direct_response body + status are written to the ResponseWriter.
func TestRunH3_GET_DirectResponse(t *testing.T) {
	f := mkDirectResponseFilter(t, "/probe", 200, "h3-ok") // RE-DERIVE from existing helpers
	req := httptest.NewRequest(http.MethodGet, "https://example.test/probe", nil)
	req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, NegotiatedProtocol: "h3"} // quic-go populates this
	rec := httptest.NewRecorder()
	status, err := f.runH3(req.Context(), rec, req)
	if err != nil {
		t.Fatalf("runH3: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	if rec.Body.String() != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", rec.Body.String())
	}
}

// TestRunH3_NoMatch_404 verifies an unmatched path returns a 404 (mirrors the
// H1 dispatchRequest no-match branch).
func TestRunH3_NoMatch_404(t *testing.T) {
	f := mkDirectResponseFilter(t, "/probe", 200, "h3-ok")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/nope", nil)
	rec := httptest.NewRecorder()
	status, _ := f.runH3(req.Context(), rec, req)
	if status != 404 {
		t.Errorf("status = %d, want 404", status)
	}
}
```

- [ ] **Step 2: Run to verify RED.** `go test ./internal/filter/hcm/ -run 'TestRunH3' -count=1 -v` — FAIL (`runH3` undefined).

- [ ] **Step 3: Implement `runH3` + `ServeH3`.** In `h3dispatch.go`, add the arm modeled on `dispatchRequest` (`connection.go:312-763`). The body is a duplication of the H1 recipe with `http.ResponseWriter` output + `req.TLS`-sourced TLS seeds + `Protocol="HTTP/3"` + `writeH3Reply`. Structure (RE-DERIVE each `chain.SetX` call + the encode-chain tail against `connection.go` at IMPL — the exact seeder set may have shifted; mirror it EXACTLY so the H3 path has identical filter-chain semantics to H1):

```go
// ServeH3 is the quic-go http3.Server handler entry (satisfies network.H3Terminal
// via http.HandlerFunc(f.ServeH3)). It dispatches one HTTP/3 request through the
// shared HCM → router → filter-chain path. Phase 61.2, ADR-0281.
func (f *Filter) ServeH3(w http.ResponseWriter, r *http.Request) {
	_, _ = f.runH3(r.Context(), w, r)
}

// runH3 dispatches one HTTP/3 request. It mirrors the H1 dispatchRequest
// (connection.go) — same *http.Request request side, same router.Action path,
// same per-request chain build + encode chain — but writes the response via
// writeH3Reply (http.ResponseWriter) and sources TLS state from r.TLS (quic-go
// populates it) rather than a net.Conn. Returns the response status + any
// wire-write error. Phase 61.2, ADR-0281.
func (f *Filter) runH3(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	var traceDecision *tracing.Decision
	start := time.Now()

	entry, routeIdx, ok := f.table.match(r)
	if !ok {
		err := writeH3Reply(w, 404, nil, nil)
		f.emitAccessLogH3(r, 404, 0, cluster.Endpoint{}, start, nil, traceDecision)
		if cnt := f.downstreamStatusClassCounter(404); cnt != nil {
			cnt.Inc()
		}
		return 404, err
	}

	action := entry.action.asRouterAction() // H1-flavored Action — NOT asRouterActionH2

	chainHF := make([]filter_http.HTTPFilter, len(f.chainConfig))
	for i, e := range f.chainConfig {
		chainHF[i] = e.factory()
	}
	chain := filter_http.NewFilterChain(chainHF, f.perRouteConfig)
	chain.SetRequestCtx(ctx, routeIdx)
	// TLS seeds from r.TLS (quic-go populates it) — the H3 analog of the H1
	// downstream.(*stdtls.Conn) snapshot in dispatchRequest. nil for the
	// unit-test path that leaves r.TLS unset.
	chain.SetTLSConnectionState(r.TLS)
	chain.SetTLSPrincipals(tlsPrincipalsFromState(r.TLS)) // RE-DERIVE the extractor used by H1/H2
	// … the full chain.SetX seeder set, mirroring dispatchRequest EXACTLY:
	//   SetDownstreamRemoteAddr / SetDownstreamLocalAddr (from r.RemoteAddr /
	//   the listener local addr), SetDownstreamTLSServerName (r.TLS.ServerName),
	//   SetDownstreamTLSPeerCertDER, SetListenerPrincipal(f.listenerPrincipal),
	//   SetRouteRateLimits / SetVirtualHostRateLimits / SetRouteMetadata /
	//   SetRouteIncludeVhRateLimits …
	chain.SetDownstreamProtocol("HTTP/3")
	defer chain.Destroy()

	rf, ok := chainHF[len(chainHF)-1].Decoder.(*router.Filter)
	if !ok {
		_ = writeH3Reply(w, 500, nil, nil)
		f.emitAccessLogH3(r, 500, 0, cluster.Endpoint{}, start, nil, traceDecision)
		if cnt := f.downstreamStatusClassCounter(500); cnt != nil {
			cnt.Inc()
		}
		return 500, nil
	}
	rf.SetAction(action)
	rf.SetRequest(r)

	endStreamOnHeaders := r.Body == nil || r.Body == http.NoBody // RE-DERIVE the H1 endStream rule
	if _, err := chain.RunDecodeHeaders(ctx, r.Header, endStreamOnHeaders); err != nil {
		// mirror the H1 decode-error handling
		return 0, err
	}
	// … H1 body-read-into-chain loop (connection.go RunDecodeData) if the
	//     request has a body; the minimal GET slice has none …

	rf.RunAction(ctx)
	resp := rf.Response()
	picked := rf.Picked()
	actionErr := rf.ActionErr()

	status := resp.Status
	if rf.ActionRan() && status > 0 && actionErr == nil {
		merged := resp.Headers.ToHTTPHeader()
		chain.SetEncodeResponseStatus(status)
		if _, err := chain.RunEncodeHeaders(ctx, merged, len(resp.Body) == 0); err != nil {
			return status, err
		}
		resp.Headers = filter_http.ReconcileOrderedHeaders(resp.Headers, merged)
		if len(resp.Body) > 0 {
			if _, err := chain.RunEncodeData(ctx, resp.Body, true); err != nil {
				return status, err
			}
		}
		if override, ok := chain.EncodeBodyOverride(); ok {
			resp.Body = override
		}
	}

	werr := writeH3Reply(w, status, resp.Headers, resp.Body)
	f.emitAccessLogH3(r, status, int64(len(resp.Body)), picked, start, resp.Headers, traceDecision)
	if cnt := f.downstreamStatusClassCounter(status); cnt != nil {
		cnt.Inc()
	}
	return status, werr
}
```

*(This is a DUPLICATION of `dispatchRequest` — the symmetric-by-duplication pattern of the H1/H2 arms, per SPEC-61 §3.3. RE-DERIVE the EXACT `chain.SetX` seeder set, the `tlsPrincipals` extractor, the body-read loop, the decode-error handling, and the drain Inc/Dec (`f.dm`) discipline against `connection.go:312-770` + `h2dispatch.go:269-520` at IMPL — do NOT ship a partial seeder set (a missing `SetDownstreamRemoteAddr` silently breaks source_ip hash policies; a missing rate-limit seed silently breaks ratelimit filters). The two existing arms are the ground truth; the H3 arm must seed IDENTICALLY. Consider extracting a shared seeder helper if the duplication is large — but the H1/H2 precedent is duplication, so match it unless the IMPL judges extraction lower-risk.)*

- [ ] **Step 4: Implement `emitAccessLogH3`.** In `accesslog_emit.go`, add after `emitAccessLogH2` (`:137`) — a sibling reading from `*http.Request` (like `emitAccessLog`, `:25`) with `Protocol="HTTP/3"` in BOTH the span `SpanInputs.Protocol` and the `Record.Protocol`:
```go
// emitAccessLogH3 is the HTTP/3 access-log + span arm (sibling of emitAccessLog
// (H1) / emitAccessLogH2). It reads from the native *http.Request (like H1) but
// forces Protocol="HTTP/3" (the reference's %PROTOCOL% / :protocol() emit
// "HTTP/3", not r.Proto's "HTTP/3.0"; the 61.3 differential verifies the exact
// string cross-side). Phase 61.2, ADR-0281.
func (f *Filter) emitAccessLogH3(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders filter_http.OrderedHeaders, traceDecision *tracing.Decision) {
	// … structurally identical to emitAccessLog (:25) but Protocol="HTTP/3" …
}
```
*(RE-DERIVE `emitAccessLog` (`:25`) as the model — it reads Method/Path/Authority/UserAgent from `*http.Request`; copy that body and force `Protocol="HTTP/3"`. Confirm the span-block early-return ordering (AMEND-TRACE-SPANEND-SEAM) matches emitAccessLogH2.)*

- [ ] **Step 5: Run to verify GREEN.** `go test ./internal/filter/hcm/ -run 'TestRunH3|TestWriteH3Reply' -count=1 -v` — PASS. Then `go test ./internal/filter/hcm/ -count=1` (no regressions).

- [ ] **Step 6: Per-task gates.** `gofmt -l internal/filter/hcm/` · `golangci-lint run ./internal/filter/hcm/...` · `go vet ./...` · `go build ./...` · `go list -deps ./internal/filter/hcm | grep -i quic-go || echo HCM-NO-QUICGO` (expect `HCM-NO-QUICGO` — the arm is stdlib-only).

- [ ] **Step 7: Commit** — `phase 61.2: runH3 dispatch arm + emitAccessLogH3 (H3 GET into the shared HCM/router/chain)`.

---

## Task 6: The `network.H3Terminal` interface + `*Filter` satisfies it

**Files:**
- Modify: `internal/filter/network/terminal.go` (add the interface + `net/http` import)
- Modify: `internal/filter/hcm/h3dispatch.go` (add the compile-assert `var _ network.H3Terminal = (*Filter)(nil)`)
- Test: `internal/filter/network/terminal_test.go` (a local-double assert) OR rely on the hcm compile-assert

**Interfaces:**
- Produces: `type network.H3Terminal interface { TerminalFilter; ServeH3(w http.ResponseWriter, r *http.Request) }`; `*hcm.Filter` satisfies it.

- [ ] **Step 1: Write the failing assert.** In `internal/filter/network/terminal_test.go`, prove the interface shape with a LOCAL double (the `network` package must NOT import `hcm`):
```go
// h3Double is a local test double proving the H3Terminal interface shape is
// satisfiable by a TerminalFilter that also serves H3.
type h3Double struct{ Marker }

func (h3Double) Handle(ctx context.Context, downstream net.Conn)  {}
func (h3Double) ServeH3(w http.ResponseWriter, r *http.Request)   {}

func TestH3TerminalInterfaceShape(t *testing.T) {
	var _ H3Terminal = h3Double{}
}
```

- [ ] **Step 2: Run to verify RED.** `go test ./internal/filter/network/ -run 'TestH3TerminalInterfaceShape' -count=1` — FAIL (`H3Terminal` undefined).

- [ ] **Step 3: Implement the interface.** In `internal/filter/network/terminal.go`, add the `net/http` import and:
```go
// H3Terminal is a TerminalFilter that can additionally serve a single HTTP/3
// request via quic-go's http3.Server handler contract. The QUIC listen path
// (internal/listener/quic.go) type-asserts an accepted connection's chain
// terminal filter to this interface and drives it via
// http3.Server{Handler: http.HandlerFunc(t.ServeH3)}.ServeQUICConn(conn).
// Stdlib-typed (*http.Request / http.ResponseWriter) so quic-go stays confined
// to internal/listener — the hcm terminal (which implements ServeH3) never
// imports quic-go. Phase 61.2, ADR-0281.
//
//nolint:revive // ADR-0215 reserves the network.* filter interface names.
type H3Terminal interface {
	TerminalFilter
	ServeH3(w http.ResponseWriter, r *http.Request)
}
```

- [ ] **Step 4: Add the hcm compile-assert.** In `internal/filter/hcm/h3dispatch.go`, near the top:
```go
// *Filter satisfies network.H3Terminal (phase 61.2) — the QUIC listen path
// drives f.ServeH3 via quic-go's http3.Server. This assert fails to compile if
// ServeH3's signature drifts from the interface.
var _ network.H3Terminal = (*Filter)(nil)
```
*(`network` is already imported in the hcm package — filter.go imports it.)*

- [ ] **Step 5: Run to verify GREEN.** `go test ./internal/filter/network/ -run 'TestH3TerminalInterfaceShape' -count=1` — PASS. `go build ./...` (the hcm compile-assert compiles).

- [ ] **Step 6: Per-task gates.** `gofmt -l internal/filter/network/ internal/filter/hcm/` · `golangci-lint run ./internal/filter/network/... ./internal/filter/hcm/...` · `go vet ./...` · `go build ./...`.

- [ ] **Step 7: Commit** — `phase 61.2: network.H3Terminal interface + *hcm.Filter satisfies it`.

---

## Task 7: Wire `serveQUICConnection` → `http3.Server{Handler}.ServeQUICConn`

**Files:**
- Modify: `internal/listener/quic.go` (add `quicChain()`; replace the `serveQUICConnection` body; add the `http3` import)
- Test: covered by Task 8's integration test (the wiring has no isolated unit test — the observable is a served H3 request)

**Interfaces:**
- Consumes: `rt.quicChain()` (the single chain's `*chainInfo`), `ci.netChainFactory()` (the network-filter slice), `network.H3Terminal` (Task 6), `rt.quicTLSConfig()` (61.1).
- Produces: an accepted QUIC connection whose H3 requests are served into the chain terminal's `ServeH3`.

- [ ] **Step 1: Add `quicChain()`.** In `quic.go`, mirror `quicTLSConfig()` (`:52`):
```go
// quicChain returns the single chain's *chainInfo for the minimal single-chain
// QUIC slice (defaultChain first, then the first chain in chainByName). SNI-
// dispatched multi-chain QUIC is deferred (M-FB1/M-FB2). Phase 61.2.
func (rt *listenerRuntime) quicChain() *chainInfo {
	if rt.defaultChain != nil {
		return rt.defaultChain
	}
	for _, ci := range rt.chainByName {
		return ci
	}
	return nil
}
```
*(NOTE the M-FB2 map-nondeterminism caveat from the 61.1 review: `chainByName` range order is nondeterministic; harmless for the single-chain slice but the wording must not overclaim determinism. Keep the deferral note.)*

- [ ] **Step 2: Replace the `serveQUICConnection` body** (`:92`). Replace the 61.1 handshake-only body:
```go
// serveQUICConnection serves HTTP/3 requests on an accepted QUIC connection.
// The QUIC/TLS-1.3 handshake is complete (Accept returned). quic-go's
// http3.Server decodes H3 frames + QPACK and invokes the chain terminal
// filter's ServeH3 per request (via the network.H3Terminal seam), dispatching
// each request into the shared HCM → router → filter-chain path. Phase 61.2
// (ADR-0281) replaces the 61.1 handshake-only close. Honors ctx: a canceled
// ctx closes the connection (M6-2 pickup).
func (rt *listenerRuntime) serveQUICConnection(ctx context.Context, conn *quic.Conn) {
	defer rt.downstreamCxActive.Dec()

	ci := rt.quicChain()
	if ci == nil {
		_ = conn.CloseWithError(0, "")
		return
	}
	filters := ci.netChainFactory()
	if len(filters) == 0 {
		_ = conn.CloseWithError(0, "")
		return
	}
	term, ok := filters[len(filters)-1].(network.H3Terminal)
	if !ok {
		// The chain terminal does not serve H3 (e.g. tcp_proxy on a QUIC
		// listener — out of the minimal slice). Close cleanly.
		log.Printf("listener %q: quic: chain terminal is not H3-capable (%T)", rt.name, filters[len(filters)-1])
		_ = conn.CloseWithError(0, "")
		return
	}
	srv := &http3.Server{
		Handler:   http.HandlerFunc(term.ServeH3),
		TLSConfig: rt.quicTLSConfig(),
		QUICConfig: &quic.Config{},
	}
	// ServeQUICConn blocks serving requests until the conn closes. Honor ctx:
	// close the conn when ctx is canceled so Stop unblocks the serve.
	go func() {
		<-ctx.Done()
		_ = conn.CloseWithError(0, "")
	}()
	if err := srv.ServeQUICConn(conn); err != nil && ctx.Err() == nil {
		log.Printf("listener %q: quic: serve: %v", rt.name, err)
	}
}
```
*(RE-DERIVE at IMPL: (a) whether `http3.Server.ServeQUICConn` requires `TLSConfig`/`QUICConfig` non-nil (the conn is already handshaked — they may be advisory; set them for safety). (b) The `<-ctx.Done()` goroutine leaks if `ServeQUICConn` returns first — use a `select`/`done` channel to stop it, mirroring the TCP path's shutdown discipline (RE-DERIVE `serveConnection`'s ctx handling). (c) Confirm `http` (`net/http`) + `http3` (`github.com/quic-go/quic-go/http3`) imports — `http3` is the ONLY new import; quic-go stays confined to this file.)*

- [ ] **Step 3: Add the imports.** In `quic.go`, add `"net/http"` and `http3 "github.com/quic-go/quic-go/http3"` + `"github.com/pgdad/envoy-go/internal/filter/network"`.

- [ ] **Step 4: Run to verify build + no isolated test.** `go build ./...` (green). The behavioral proof is Task 8. Run `go test ./internal/listener/ -count=1` to confirm the 61.1 handshake test still passes (the handshake-only path is now a serve path — the 61.1 `TestQUICListener_HandshakeALPNh3` used a bare quic-go client that does NOT send an H3 request; confirm it still completes the handshake and does not hang. If it now hangs waiting for the server to serve, adjust it or supersede it with Task 8's H3 test — RE-DERIVE and record which).

- [ ] **Step 5: Per-task gates.** `gofmt -l internal/listener/` · `golangci-lint run ./internal/listener/...` · `go vet ./...` · `go build ./...` · `go list -deps ./internal/listener | grep -i quic-go` (expect the quic-go module — confined here) · `go list -deps ./internal/filter/hcm | grep -i quic-go || echo HCM-NO-QUICGO`.

- [ ] **Step 6: Commit** — `phase 61.2: wire serveQUICConnection → http3.Server.ServeQUICConn (serve H3 into the chain terminal)`.

---

## Task 8: The subject-side H3 GET→200 integration test

**Files:**
- Modify: `internal/listener/quic_test.go` (add `TestQUICListener_ServesH3GET` + the HCM-carrying `mkQUICListenerHCM` helper if not landed at Task 2)
- Test: `internal/listener/quic_test.go`

**Interfaces:**
- Consumes: the full 61.1 + 61.2 stack (a QUIC listener with an HCM `codec_type HTTP3` + a `direct_response` or routed action).
- Produces: the leg-61.2 subject-side proof — a local quic-go `http3.Transport` client GET returns 200 + the expected body over HTTP/3.

- [ ] **Step 1: Write the failing integration test.** In `quic_test.go`:
```go
// TestQUICListener_ServesH3GET is the leg-61.2 subject-side proof: a QUIC
// listener with an HCM (codec_type HTTP3 + a direct_response) serves an H3 GET
// to a local quic-go http3.Transport client, returning 200 + the body over
// HTTP/3. NO differential (that is 61.3).
func TestQUICListener_ServesH3GET(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	// A QUIC listener whose HCM codec_type=HTTP3 routes /probe → direct_response
	// 200 "h3-ok" (RE-DERIVE the HCM+route typed_config helper).
	l := mkQUICListenerH3DirectResponse(t, cm, testCertPEM, testKeyPEM, "/probe", 200, "h3-ok")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	addr := mgr.Listeners()[0].Addr

	rt := &http3.Transport{
		TLSClientConfig: &stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true}, //nolint:gosec // local test
		QUICConfig:      &quic.Config{},
	}
	defer rt.Close()
	client := &http.Client{Transport: rt}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+"/probe", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("H3 GET %s: %v", addr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.ProtoMajor != 3 {
		t.Errorf("proto major = %d, want 3 (HTTP/3)", resp.ProtoMajor)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", string(body))
	}
}
```
*(RE-DERIVE `mkQUICListenerH3DirectResponse` — a `mkQUICListener` variant whose filter chain carries an HCM `typed_config` with `codec_type: HTTP3` + an `http3_protocol_options:{}` + a route `/probe → direct_response{status:200, body:"h3-ok"}` + the router filter. Model the HCM typed_config on the existing HCM test helpers (`config_test.go`'s HCM builders) + the QUIC transport socket from 61.1's `mkQUICListener`. The `http3.Transport` client is the quic-go client — pin the dialed addr, IGNORE Alt-Svc (mirrors the 61.3 `H3RoundTrip` discipline).)*

- [ ] **Step 2: Run to verify RED.** `go test ./internal/listener/ -run 'TestQUICListener_ServesH3GET' -count=1 -v` — FAIL before Task 7's wiring is complete (dial times out / 404). *(If Tasks 2–7 all landed, this may already pass — that is the integration confirming the stack. Still run RED first by temporarily reverting the Task-7 serve body to the handshake-only close, to prove the test BITES the serve path, per `reference_differential_break_protocol_count1` discipline; then restore.)*

- [ ] **Step 3: Confirm GREEN.** `go test ./internal/listener/ -run 'TestQUICListener_ServesH3GET' -count=1 -v` — PASS (200, ProtoMajor 3, body `h3-ok`).

- [ ] **Step 4: Run the package under -race.** `go test ./internal/listener/ -race -count=1` — race-clean (the QUIC accept goroutine + the `http3.Server` serve goroutine + the ctx-cancel goroutine + `Stop` nil-writes). Address any race (esp. the Task-7 ctx-cancel goroutine's `conn` access vs `ServeQUICConn`'s internal close).

- [ ] **Step 5: Record the break evidence.** Confirm WHICH assertion fired under the temporary Task-7 revert (status≠200 or dial error) — record it in PROGRESS per `reference_deliberate_break_wrong_assertion`.

- [ ] **Step 6: Per-task gates.** `gofmt -l internal/listener/` · `golangci-lint run ./internal/listener/...` · `go vet ./...` · `go build ./...`.

- [ ] **Step 7: Commit** — `phase 61.2: subject-side H3 GET→200 integration test (local quic-go http3.Transport client)`.

---

## Task 9: Docs + verify + ADR-0281 + router roll

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (extend the HTTP/3 section — H3 GET now SERVED + the HTTP3-on-non-QUIC reject)
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0281 §Context/§Decision/§Consequences)
- Modify: `docs/envoy-go/phases/61-http3-downstream-listener/PROGRESS-61.2.md` (final checklist + six-gate record + break evidence)
- Modify: `docs/envoy-go/STATE.md` (active-phase header → phase 61.2 IMPL done; NEXT = the phase-61.3 PLAN) — controller
- Modify: `docs/envoy-go/ROADMAP.md` (row 61 STAYS `in-progress` — NO flip; the LIVE HTTP/3 deferred sentence UNCHANGED)
- Modify: `next-prompt.txt` (the router roll — folded into the squash; `reference_next_prompt_tracked_despite_gitignore`)

- [ ] **Step 1: Extend BEHAVIOR_CONTRACT HTTP/3 section.** RE-DERIVE the 61.1 HTTP/3 substrate paragraph and extend it: a downstream QUIC/H3 listener now SERVES a GET into the existing HCM → router → filter-chain → upstream path (mandatory TLS 1.3, ALPN h3); `codec_type HTTP3` on a non-QUIC listener is a config-parity boot-reject; upstream H3 / alt-svc / 0-RTT / h3spec / QUIC robustness / SNI-multi-chain still DEFERRED.

- [ ] **Step 2: Author ADR-0281.** §Context (the codec-arm seam over the 61.1 listen substrate: quic-go's http3.Server as the codec, the stdlib-typed dispatch arm keeping hcm quic-go-free, the `network.H3Terminal` bridge, the H1-Action reuse); §Decision (the `ServeH3`→`runH3` arm modeled on `dispatchRequest`; `writeH3Reply`; the `IsQUIC`-gated codec reject; `emitAccessLogH3`; the `http3.Server.ServeQUICConn` wiring; +0 packages / +0 stat surface recommended; the M6-2 ctx pickup); §Consequences (what 61.3 must do: the cross-side `0102` fixture, the harness UDP surgery, `test/helpers/h3.go` `H3RoundTrip`, the named-stat-subset assertion, the exact access-log/span protocol string verification, row 61 → `done`). DECISIONS tail **ADR-0279 → ADR-0281** (next-free **ADR-0282**).

- [ ] **Step 3: Run the six-gate** (in the worktree `.worktrees/phase-61.2-impl`):
```
gofmt -l .                                                    # GOFMT_CLEAN
golangci-lint run ./...                                       # clean, exit 0
go vet ./...                                                  # clean
go build ./... && echo BUILD_OK                               # BUILD_OK
go mod tidy -diff && echo MODTIDY_CLEAN                       # MODTIDY_CLEAN (no module change in 61.2)
go list -deps ./internal/filter/hcm | grep -i quic-go || echo HCM-NO-QUICGO  # HCM-NO-QUICGO
go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO         # TLS-NO-QUICGO
grep -rh '^func Fuzz' --include='*.go' . | wc -l             # 55 (+0)
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l            # 105 (+0)
go test ./internal/filter/hcm/ ./internal/filter/network/ ./internal/listener/ -count=1   # ok
go test ./internal/filter/hcm/ ./internal/filter/network/ ./internal/listener/ -race -count=1  # race-clean
```
Confirm quic-go appears in `./internal/listener` deps (confined) but NOT in `./internal/filter/hcm` or `./internal/tls`.

- [ ] **Step 4: FULL non-differential suite + the full 105-dir differential** — DELEGATED to the controller on the frozen squash HEAD (the 105-dir differential must stay byte-stable — 61.2 adds no fixture and changes no TCP path; the QUIC serve path is only reached by a QUIC listener, which no existing fixture configures).

- [ ] **Step 5: Sentinel re-check + ROADMAP/STATE.** Run the three sentinel checks MECHANICALLY (they must all print — row 61 `in-progress`, three live "candidates:" sentences, three never-opened families). Confirm row 61 STAYS `in-progress`; the HTTP/3 deferred sentence STAYS exactly one live match. Update STATE (controller). Update `next-prompt.txt` to roll to the phase-61.2 IMPL → the phase-61.3 PLAN.

- [ ] **Step 6: Commit** — `phase 61.2: BEHAVIOR_CONTRACT + ADR-0281 + STATE + router roll (row 61 stays in-progress)`.

---

## Self-review (run against SPEC-61 with fresh eyes)

**Spec coverage (SPEC-61 §3.3 / §10 leg-61.2 / §11 reject-B / §12):**
- §3.3 the H3 codec arm modeled on WriteH2 → Tasks 4/5 (`writeH3Reply` + `runH3`; refined: modeled on H1 `dispatchRequest` since the request side is `*http.Request`, closer than WriteH2). ✓
- §3.3 the `http.ResponseWriter → writeH3Reply` adapter → Task 4. ✓
- §10 leg-61.2 (1) the `internal/filter/hcm/h3` codec arm → Tasks 4/5 (refined: `h3dispatch.go` in the hcm package, +0 packages). ✓
- §10 leg-61.2 (2) lift the `codec_type HTTP3` reject + UPDATE config_test → Task 3. ✓
- §10 leg-61.2 (3) `emitAccessLogH3` → Task 5. ✓
- §10 leg-61.2 (4) the subject-side H3 GET→200 test → Task 8. ✓
- §11 reject-B (HTTP3-on-non-QUIC) → Task 3 (gated on `IsQUIC`, threaded in Task 2). ✓
- SPEC §12 "wire `serveQUICConnection` into an `http3.Server`" → Task 7. ✓
- The `network.H3Terminal` bridge (NOT in the SPEC — DERIVED here as the layering-clean seam) → Task 6. ✓ *(This is the PLAN's material design addition over the SPEC: the SPEC assumed a `codecType==HTTP3` branch in `Handle`, but `Handle` takes a `net.Conn` never present on the QUIC path; the H3 entry is a distinct `ServeH3` seam reached via the interface. Recorded in PROGRESS as a SPEC correction.)*

**Placeholder scan:** the `runH3` body (Task 5 Step 3) shows the FULL recipe skeleton with explicit "RE-DERIVE the exact `chain.SetX` seeder set against `connection.go`" callouts — this is a deliberate, flagged IMPL-investigation (the seeder set is long and shifts across phases; shipping a hardcoded partial set would be a worse failure than the flagged re-derive). Every OTHER step has complete code. No `TBD`/`handle edge cases`/`similar to Task N` placeholders.

**Type consistency:** `writeH3Reply(w http.ResponseWriter, status int, headers filter_http.OrderedHeaders, body []byte) error` — consistent across Tasks 4/5/7. `ServeH3(w http.ResponseWriter, r *http.Request)` — consistent across the interface (Task 6), the `*Filter` method (Task 5), and the `http.HandlerFunc(term.ServeH3)` call site (Task 7). `runH3(ctx, w, r) (int, error)` — consistent Tasks 5/8. `IsQUIC bool` — consistent across `FactoryCtx` (Task 2) / `ListenerCtx` (Task 2) / the reject gate (Task 3). `emitAccessLogH3` signature matches `emitAccessLog`/`emitAccessLogH2` (Task 5).

**Gaps found + fixed:** the SPEC's "`filter.go:112` ADD a `codecType == HTTP3` branch in `Handle`" is a re-derivation error (Handle is never called on the QUIC path) — the PLAN replaces it with the `ServeH3`/`H3Terminal` seam and records the correction (design pin + PROGRESS). The `IsQUIC` threading (Task 2) is a prerequisite the SPEC did not decompose (it lumped the reject into leg 61.2 without the context-plumbing) — added as Task 2.
