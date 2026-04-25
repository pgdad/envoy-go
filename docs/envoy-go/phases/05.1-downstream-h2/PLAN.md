# Phase 05.1 — Downstream HTTP/2 (server-side codec, ALPN dispatch, h2spec conformance) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants), §5 (state machine), §6 (splitting), §7 (differential contract), §7.5 (phase-done gates with the conformance gate (c) clarification); `docs/envoy-go/phases/05.1-downstream-h2/SPEC.md` (authoritative scope — every PLAN decision below traces to a SPEC section); `docs/envoy-go/phases/05-http-2/SPEC.md` (master phase-05 design document; sub-phase SPECs carve coherent slices of its §4 deliverables per ADR-0045); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0045 — especially **ADR-0003** branch convention, **ADR-0004** autonomous brainstorming adaptation, **ADR-0005** autonomous plan-review adaptation, **ADR-0008** Envoy v1.37.2 pin, **ADR-0010** V4_ONLY DNS rule, **ADR-0013** go-control-plane proto-types-only pin, **ADR-0014** `Server: envoy` HCM-locally-generated header value, **ADR-0015** `/ready` Date allow-list, **ADR-0016** bootstrap loader unknown-field policy + blank-import amendment policy, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0023** phase-00 byte pump lift, **ADR-0024** per-cluster RR scope, **ADR-0027** STATIC-vs-STRICT_DNS divergence, **ADR-0028** reference-side `--concurrency 1` pin, **ADR-0031** stdlib `crypto/tls` stack, **ADR-0032** `Cluster.Dial(ctx)` upstream dialer, **ADR-0033** filter-chain subset, **ADR-0035** fixture-0002 differential scope, **ADR-0037** stdlib `net/http` HTTP/1.1 wire codec, **ADR-0038** route match subset, **ADR-0039** per-request fresh upstream dial, **ADR-0040** HTTP-filter framework subset, **ADR-0041** HCM `stat_prefix` + silently-ignored field set, **ADR-0042** HTTP-filter chain shape `[router]`, **ADR-0043** fixture-driver `HTTPExpectations` extension, **ADR-0044** BEHAVIOR_CONTRACT HTTP/1.1 subsection, **ADR-0045** plan-time split of phase 05 into 05.1 + 05.2); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (existing `## Equivalence Matrix`, `## Header allow-list`, `## Admin API — /ready`, `## Test harness host networking`, `## TCP proxy`, `## TLS`, `## HTTP/1.1` subsections — phase 05.1 adds a new `## HTTP/2` subsection in SCAFFOLD form per master SPEC §5.7 + 05.1 SPEC §5.7, and appends rows to `## Header allow-list` for `:status` (active in 05.1) and `:method`/`:path`/`:scheme`/`:authority` (forward-looking, applies-to: 05.2)); `docs/envoy-go/phases/04-http-1.1/PLAN.md` and `PROGRESS.md` (style reference for tasks, atomic per-task commits, PROGRESS conventions, ADR-with-first-use-commit discipline); `docs/envoy-go/phases/04-http-1.1/REVIEW.md` (commit `04527eb`; the 4 Important + 7 Minor findings — the four Importants and M-1/M-3 already landed in `671a059` / `1542102` / `bbe298f`; M-2/M-4/M-5/M-6/M-7 carry forward to 05.1 per ADR-0045 + SPEC §12 — disposition recorded in `## Phase-04 REVIEW carryover resolution matrix` below).

**Goal:** Land envoy-go's first downstream HTTP/2 dataplane — a from-scratch `internal/filter/hcm/h2/` server-side codec sub-package decomposing into nine production source files (`doc.go`, `errors.go`, `preface.go`, `framer.go`, `hpack.go`, `flow.go`, `settings.go`, `stream.go`, `conn.go` — explicitly NO `client.go` per ADR-0045) that drive `golang.org/x/net/http2.Framer` + `golang.org/x/net/http2/hpack` as low-level codec surfaces (per doctrine `D-3.2`); a per-stream server-side state machine implementing the RFC 9113 §5.1 idle/open/half-closed/closed lifecycle; an ALPN-driven codec dispatcher in `internal/filter/hcm/filter.go` that inspects `*tls.Conn.ConnectionState().NegotiatedProtocol` after the TLS handshake (the listener-side `alpn_protocols` plumbing is unchanged from phase 03) and dispatches to the new H2 connection driver on `"h2"` or to the phase-04 H1 driver on `"http/1.1"`/empty/plaintext; a small extension to `internal/filter/hcm/config.go` permitting `codec_type: HTTP2` (and re-defining `AUTO` from "alias for HTTP1" to "ALPN-driven" — on a non-TLS listener `AUTO` continues to resolve to HTTP1 unchanged) plus adding `http2_protocol_options` to the silent-ignore set; a codec-neutral factoring of `directResponseAction` in `internal/filter/hcm/actions.go` (`body() (status, headers, body)` + `writeH1(io.Writer)` H1 adapter retaining phase-04's exact wire bytes + new `writeH2(streamWriter)` H2 adapter writing HEADERS+DATA+END_STREAM) — required in 05.1 because h2spec section 8 exercises `direct_response`; a test-only `--allow-h2c` CLI flag on `cmd/envoy-go` plumbed through a new `listener.NewManagerWithBaseDirAndAllowH2C` constructor variant into a per-listener `listenerCtx{hasTLS bool, allowH2C bool}` value passed into `hcm.NewFilter` for build-time validation of `codec_type: HTTP2` vs transport semantics; the project's first non-vacuous **conformance** gate under `test/conformance/h2spec/` — a Go test driver that boots a phase-05.1 envoy-go subject with a synthetic h2c bootstrap (`--allow-h2c`), runs the upstream `summerwind/h2spec` Docker image via `testcontainers-go`, parses the JUnit-XML output, and asserts `failed == 0` over h2spec sections 3, 4, 5, 6 (excluding 6.6 PUSH_PROMISE), 7, 8 — the threshold codified in `BEHAVIOR_CONTRACT.md`'s new `## HTTP/2` SCAFFOLD subsection (per master SPEC §5.7 + 05.1 SPEC §5.7) and the conformance image pinned by tag + SHA256 in a NEW `docs/envoy-go/CONFORMANCE_PINS.md` file (sibling to `ENVOY_TARGET.md`, same refresh discipline per D-3.7); two new fuzz targets `internal/filter/hcm/h2.FuzzFrameStream` + `internal/filter/hcm/h2.FuzzHPACKDecode` running clean for the 30-second CI budget per ADR-0018; an h2-over-TLS bootstrap variant in `cmd/envoy-go/main_test.go` exercising the binary's H2 listener path via `golang.org/x/net/http2.Transport` (driver-side use permitted; runtime not); the new `## HTTP/2` BEHAVIOR_CONTRACT subsection in SCAFFOLD form (codifies 05.1's codec/conformance scope and explicitly defers fixture-0004 + routed-to-upstream rules to 05.2 in its "Does not yet apply to" enumeration); eight new ADRs (ADR-0046..ADR-0053 corresponding to SPEC ADR-P/Q/S/T/U/V/Z/X) — and the formal phase-04 REVIEW Minor carry-forward triage (M-2/M-4/M-5/M-6/M-7) per SPEC §12 + ADR-X. After phase 05.1, envoy-go terminates downstream HTTP/2 on a TLS listener — it negotiates ALPN, drives an own framer, demuxes streams through an own connection-manager state machine, and produces structurally-equivalent framing and per-stream behaviourally-equivalent responses to upstream Envoy on the `direct_response` surface, while passing the declared subset of `h2spec` conformance. The remaining half (upstream-H2 origination + a full-stack h2 differential fixture closing ADR-0035's H2 leg) is delivered by phase 05.2 per ADR-0045.

**Architecture:** `internal/filter/hcm/h2/` is a NEW server-side-only codec sub-package under the phase-04 HCM package, decomposing into nine production source files and ~five test files. `errors.go` defines an enum of RFC 9113 §7 error codes (`NO_ERROR`, `PROTOCOL_ERROR`, `INTERNAL_ERROR`, `FLOW_CONTROL_ERROR`, `STREAM_CLOSED`, `FRAME_SIZE_ERROR`, `REFUSED_STREAM`, `CANCEL`, `COMPRESSION_ERROR`, `CONNECT_ERROR`, `ENHANCE_YOUR_CALM`, `INADEQUATE_SECURITY`, `HTTP_1_1_REQUIRED`) and an `*Error{Code, Stream, Underlying}` type whose `Error()` strings begin with `h2:` (so fuzzers and unit tests can grep for it). `preface.go` checks the 24-byte client preface (`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n`); mismatch → connection-level error. `framer.go` is a thin wrapper over `http2.Framer` adding context-cancellation on the read side via deadline-translation (`http2.Framer.ReadFrame` is blocking and not ctx-aware); write methods are passthrough. `hpack.go` carries the per-conn `hpack.Encoder`/`hpack.Decoder` pair with the conn-level dynamic-table state; `SETTINGS_HEADER_TABLE_SIZE` from the client is honoured by passing it to the encoder side. `flow.go` implements connection-level + per-stream send/recv flow-control windows (channels + small mutex; minimal-and-correct, not optimised). `settings.go` writes the server's initial SETTINGS frame and reads the client's initial SETTINGS frame; the `ServerSettings` struct carries hardcoded defaults per ADR-S (`MaxConcurrentStreams=100`, `InitialWindowSize=65535`, `MaxFrameSize=16384`, `EnablePush=0`, `NoRFC7540Priorities=1`, `HeaderTableSize=4096`). `stream.go` defines the per-stream server-side state machine: `serverStream` carries stream ID, current state (idle/open/halfClosedRemote/halfClosedLocal/closed), per-stream send/recv windows, decoded request headers, request body pipe, END_STREAM flags; methods `recvHeaders`, `recvData`, `recvRSTStream`, `recvWindowUpdate`, `dispatch` (called once on END_STREAM-on-headers OR END_STREAM-on-data, runs the route match + action). `conn.go` exposes `ServerConn` (per-downstream-conn server-side connection manager): `NewServerConn(ctx, downstream net.Conn, table *hcm.RouteTable, settings ServerSettings) *ServerConn` and `(*ServerConn).Run() error` that performs preface check + SETTINGS handshake + frame loop + GOAWAY emission. `internal/filter/hcm/filter.go`'s `Filter.Handle(ctx, downstream)` learns ALPN dispatch (per ADR-V): type-assert to `*stdtls.Conn`; on TLS, read `ConnectionState().NegotiatedProtocol`; on `"h2"`, dispatch to `h2.NewServerConn(...).Run()`; otherwise fall through to phase-04's `runConnection` H1 driver (unchanged). The `--allow-h2c` plaintext-h2 path (only reachable when the test-only flag is set) bypasses the type-assert and dispatches directly to `h2.ServerConn`. `internal/filter/hcm/config.go` permits `codec_type: HTTP2` and adds build-time validation requiring TLS transport_socket UNLESS `listenerCtx.allowH2C` is set; `codec_type: AUTO` is re-defined from "alias for HTTP1" to "ALPN-driven" (on a non-TLS listener AUTO still resolves to HTTP1 — phase-04 behaviour preserved); `http2_protocol_options` (the directly-on-HCM proto field) joins the phase-04 silent-ignore set per ADR-0041's amendment. `internal/filter/hcm/actions.go` factors `directResponseAction` codec-neutral: a `body() (status int, headers http.Header, body []byte)` method returns the synthesised reply; `writeH1(io.Writer) error` writes the HTTP/1.1 wire bytes (extracted from phase-04's `do`+`writeStatusReply` path, byte-for-byte preserved); `writeH2(sw streamWriter) error` writes HEADERS frame (`:status` pseudo first, then `Date`/`Server`/`Content-Type`/`Content-Length` regular headers per RFC 9113 §8.3) + DATA frame (body bytes) + END_STREAM. The streamWriter interface is internal to `internal/filter/hcm/h2/` and exposes `WriteHeaders(headers []hpack.HeaderField, endStream bool)` + `WriteData(b []byte, endStream bool)`. The phase-04 `routerAction` is unchanged in 05.1 (no H2 variant lands here); on the H2 path a stream that resolves to a `routerAction` produces a runtime per-stream INTERNAL_ERROR + RST_STREAM (the protective shape per SPEC §5.2 step 4c — unreachable in production 05.1 bootstraps but unit-tested for defence-in-depth). `internal/filter/hcm/connection.go` (the H1 driver) is UNCHANGED in 05.1; the H1 call site `entry.action.do(ctx, req, bw)` continues to invoke `directResponseAction.do` whose body now is a single-line `return a.writeH1(bw)` shim (mechanical rename, byte-identical wire output). `internal/listener/manager.go` adds a new constructor variant `NewManagerWithBaseDirAndAllowH2C(bs, cm, baseDir, allowH2C)` that threads `listenerCtx{hasTLS, allowH2C}` through to `hcm.NewFilter`; the existing two constructors (`NewManager`, `NewManagerWithBaseDir`) call the new one with `allowH2C=false`. `cmd/envoy-go/main.go` adds the `--allow-h2c` CLI flag (per ADR-Z; default OFF; documented in `--help` as "test-only; not for production") and invokes `listener.NewManagerWithBaseDirAndAllowH2C(bs, cm, baseDir, *allowH2C)`. `cmd/envoy-go/main_test.go` gains an h2-over-TLS bootstrap variant (`TestEnvoyGoBinary_H2Smoke`) — a TLS listener with `alpn_protocols: ["h2"]` and HCM `codec_type: HTTP2`, one `direct_response` route, asserted via `golang.org/x/net/http2.Transport` client probe with `InsecureSkipVerify: true` (driver-side use permitted; runtime is not). `test/conformance/h2spec/h2spec_test.go` is the project's first conformance-suite Go test driver; `h2spec.go` (sibling helper, no `_test.go` suffix) holds the threshold-section-list constant and the pinned image reference. `docs/envoy-go/CONFORMANCE_PINS.md` is a NEW pin file mirroring `ENVOY_TARGET.md`'s discipline — pins `summerwind/h2spec` by tag + SHA256 with refresh procedure. `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains a new `## HTTP/2` SCAFFOLD subsection per ADR-T; the existing `## Header allow-list` table is extended with one row for `:status` (active in 05.1) and four forward-looking rows for `:method`/`:path`/`:scheme`/`:authority` whose "applies-to" reads `phase 05.2 routed-to-upstream H2` (per SPEC §5.7). Eight ADRs land at execution time, mapped from SPEC §4.4's anticipated lettered ADRs P/Q/S/T/U/V/Z/X to sequential numbers ADR-0046..ADR-0053 — first-use commit ordering per phase-02/03/04 precedent (re-verified at Task 1 step 1 by `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returning `ADR-0045`).

**Tech Stack:**
- Go 1.23 (unchanged from phase 04; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `net/http` (`http.TimeFormat` for the H2 Date header; otherwise unchanged HTTP/1.1 surfaces from phase 04), `bufio`, `context`, `io`, `net`, `strings`, `time`, `errors`, `fmt`, `log`, `sync`, `crypto/tls` (the `*tls.Conn.ConnectionState().NegotiatedProtocol` read site for ALPN dispatch).
- **NEW: `golang.org/x/net/http2.Framer` and `golang.org/x/net/http2/hpack`** — used as low-level codec only per doctrine `D-3.2` and ADR-P; the package is pinned via `go.mod` / `go.sum` (the existing `golang.org/x/net` indirect dependency carried by `go-control-plane` is promoted to a direct dependency at Task 2).
- **Forbidden runtime imports (D-3.2):** `golang.org/x/net/http2.Server`, `http2.Server.ServeConn`, `http2.ConfigureServer`, `http2.Transport`, `http2.Transport.NewClientConn`. Driver-side test use of `x/net/http2.Transport` (in `cmd/envoy-go/main_test.go` and `internal/filter/hcm/h2/*_test.go`) is permitted because that is fixture infrastructure, not envoy-go runtime — D-3.2 governs runtime, not test code. The discipline is grep-verifiable at Task 16: `! grep -nR '"golang.org/x/net/http2"' internal/ cmd/envoy-go/main.go` excluding `_test.go` files must return zero results outside the `h2/` sub-package's `framer.go`/`hpack.go`/`settings.go`.
- `github.com/envoyproxy/go-control-plane/envoy` at v1.32.4 (ADR-0013 pin, unchanged).
- `google.golang.org/protobuf/types/known/anypb` (Any unmarshal for HCM typed_config — same pattern as phase-04).
- Existing `internal/cluster` package — UNCHANGED in 05.1 (no `Cluster.UseH2()` accessor, no `DialH2`, no `HttpProtocolOptions` reader, no blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"`; all 05.2 deliverables per ADR-0045).
- Existing `internal/tls` package — UNCHANGED in 05.1 (the phase-03 `alpn_protocols` plumbing already covers what 05.1 needs on the listener side).
- `github.com/testcontainers/testcontainers-go` for the new conformance harness (consumed via `test/conformance/h2spec/h2spec_test.go`; phase 05.1 does not modify `test/differential/harness.go`).
- **NEW: `summerwind/h2spec` Docker image** at the tag + SHA256 pinned in `docs/envoy-go/CONFORMANCE_PINS.md` (per ADR-U); same refresh discipline as ENVOY_TARGET.md per D-3.7.
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- Upstream Envoy v1.37.2 @ `sha256:c5e8a68e…` (ADR-0008, consumed not modified by 05.1; the differential harness is not exercised by 05.1's gate (a) which is vacuously green per ADR-0045).

---

## Scope check — why phase 05.1 ships as one sub-phase, not 05.1.1 + 05.1.2

Net change estimate: **~2400 LoC** (~1000 new production code in `internal/filter/hcm/h2/`; ~880 codec sub-package tests; ~50 actions.go codec-neutral refactor + 60 test extension; ~30 filter.go ALPN dispatch + 80 test extension; ~20 config.go HTTP2 + 50 test extension; ~30 listener/manager.go listenerCtx + 60 test extension; ~10 cmd/envoy-go --allow-h2c + 120 main_test.go h2-over-TLS smoke; ~30 test/conformance/h2spec/h2spec.go + 200 h2spec_test.go; ~80 BEHAVIOR_CONTRACT scaffold; ~50 CONFORMANCE_PINS.md; ~500 across the eight new ADRs in DECISIONS.md; ~80 fuzz targets). The split-gate threshold is **~1500 LoC OR ~25 numbered tasks** (`BOOTSTRAP_PROMPT.md` §6.1); the LoC estimate exceeds the LoC threshold (over by ~60%, comparable in magnitude to phase-04's ~2400-LoC scope which shipped as one phase per `ddf41cd`'s scope check). Task count is 16 — well below the 25-task gate and within SPEC §11.1's anticipated 12–15 range (the planner adds one task — Task 1 preconditions — per phase-02/03/04 precedent, beyond the SPEC's pure TDD-task count).

Phase 05.1 ships as **one** sub-phase (not split into 05.1.1 codec-foundation + 05.1.2 conformance-and-dispatch, and not split off the BEHAVIOR_CONTRACT or REVIEW carry-forward into a separate sub-phase) for four reasons:

1. **No further coherent split axis exists.** ADR-0045 already split phase 05 into 05.1 (downstream H2 + h2spec) + 05.2 (upstream H2 + fixture 0004) on the split-by-surface axis recommended by phase-05 SPEC §11.1. SPEC §11.1 explicitly enumerated the alternative axes (split-by-transport, split-by-ends) and rejected each as "strictly worse than split-by-surface." 05.1 SPEC §11.1 carries this forward: "If 05.1's PLAN trips the gate again, the split-by-codec-direction axis is exhausted and the planner exits blocked per `BOOTSTRAP_PROMPT.md` §6 rather than re-split." A 05.1.1 / 05.1.2 split along any axis (codec primitives vs HCM dispatch, or codec+dispatch vs conformance, or implementation vs BEHAVIOR_CONTRACT) ships at least one sub-phase whose phase-done gate set is incomplete — gate (c) (h2spec) requires both the codec sub-package and the ALPN dispatch path live; an attempted 05.1.1 codec-only sub-phase would have neither gate (a) (vacuous in 05.1 anyway), gate (c) (the codec is not driveable without `Filter.Handle` ALPN dispatch and `--allow-h2c`), nor a coherent atomic claim. `BOOTSTRAP_PROMPT.md` §6.3 explicitly forbids shipping incomplete stubs that conformance tests can't exercise.

2. **Task count is well under the GATE threshold; LoC is the OR-leg with a precedent.** Per phase-04 precedent (`ddf41cd`'s scope check + `bbe298f`'s green close), task-count under 25 is the primary signal that one phase is the right shape. 05.1's 16 tasks is comfortably under, and matches SPEC §11.1's expected 12–15 plus one preconditions task. The LoC threshold trips because the codec sub-package is dense (RFC 9113 §3-§7 frame primitives + §5.1 stream state machine + §5.2 flow control + §6 frame definitions all land in this sub-phase), but task-count and atomic-claim coherence are the operative signals when the LoC overshoot is moderate. 05.1's ~2400 LoC is not "way over" the 1500 threshold (e.g., 4000+); it is at the same magnitude as phase-04's accepted one-phase shipment.

3. **Mid-execution split valve is preserved.** `BOOTSTRAP_PROMPT.md` §6.1's secondary trigger ("if any single task's sub-steps blow up past ~10 items once contact with reality reveals complexity") stays active. The two tasks most likely to blow past 10 sub-steps are Task 8 (`stream.go` per-stream state machine — the largest piece of stateful code in this phase, with tests spanning every state transition + every error code) and Task 9 (`conn.go` connection manager — orchestrates preface + settings + frame loop + GOAWAY emission). If either exceeds 15 sub-steps at execution time, the executor splits per §6.2 with a new ADR. That is a real release valve — the executor does not need permission to invoke it. Per 05.1 SPEC §11.1, if such a split *is* required, the planner exits blocked rather than re-splitting because the codec-direction axis is already exhausted by ADR-0045.

4. **BEHAVIOR_CONTRACT + REVIEW carry-forward are textual / bookkeeping.** Per ADR-0045 consequence: "phase-04 REVIEW carry-forward triage (M-2/M-4/M-5/M-6/M-7) lands in 05.1 because the dispositions are textual / cosmetic + a forward-looking 'phase-06-must-consume' tag and none touches upstream-H2 surface." The BEHAVIOR_CONTRACT `## HTTP/2` SCAFFOLD subsection is similarly textual. Splitting either out into a separate sub-phase would ship a sub-phase consisting entirely of documentation work — a worse atomic-claim shape than bundling them into the closing task of 05.1, which is the phase-04 precedent (Task 17 closed phase 04 with BEHAVIOR_CONTRACT + ADR-0044 + all-gates green local sweep).

**Triggering re-evaluation:** if at execution time the cumulative landed-LoC count exceeds **3500** by the end of Task 9 (i.e., before the HCM ALPN dispatch + conformance suite + BEHAVIOR_CONTRACT closing tasks), invoke `superpowers:systematic-debugging` on the estimate-vs-reality gap and re-evaluate. A 45% miss on a carefully-bounded sub-phase is a signal that the plan's shape is wrong, not just that the work is large. Per 05.1 SPEC §11.1, if a split *is* required at execution time, the planner exits blocked per `BOOTSTRAP_PROMPT.md` §6 because the codec-direction axis is already exhausted by ADR-0045.

---

## File Structure

| Path | Created/Modified/Deleted | Purpose |
|---|---|---|
| `internal/filter/hcm/h2/doc.go` | Create | Package doc — phase-05.1 server-side H2 codec; references SPEC §4.1, ADR-0046 (codec source: `golang.org/x/net/http2.Framer` + `hpack`), ADR-0048 (server connection manager from scratch), ADR-0047 (server settings defaults), ADR-0052 (BEHAVIOR_CONTRACT scaffold). Documents that all errors begin with `h2: `, that this package does NOT use `http2.Server` / `http2.ConfigureServer` / `http2.Transport` runtime constructs (D-3.2), and that **no `client.go` exists in 05.1** (`ClientConn` is 05.2's deliverable per ADR-0045). The doc also enumerates the package's public surface (`ServerConn`, `NewServerConn`, `ServerSettings`) and reserves `client.go` / `clientConn` for 05.2. |
| `internal/filter/hcm/h2/errors.go` | Create | RFC 9113 §7 error code enum (`NO_ERROR`=0x0 through `HTTP_1_1_REQUIRED`=0xd); `*Error{Code uint32, Stream uint32, Underlying error}` type with `Error() string` returning `"h2: <code-name>"` (or `"h2: <code-name> stream=<id>"` when `Stream != 0`) and `Unwrap()` returning `Underlying`. Helpers: `connError(code, msg)` builds a connection-scoped error (Stream=0); `streamError(code, streamID, msg)` builds a stream-scoped error. The convention `h2:` prefix on every error message is the discriminator the fuzz targets (Task 14) grep for. |
| `internal/filter/hcm/h2/errors_test.go` | Create | Table-driven: every error code's stringification matches `"h2: <code-name>"` (case + spelling per RFC 9113 §7 — "PROTOCOL_ERROR" not "Protocol_Error"); `connError`/`streamError` constructors set the right Stream field; `Unwrap` returns the wrapped error; `errors.Is(streamError(...), targetErr)` follows wrapping chains. |
| `internal/filter/hcm/h2/preface.go` | Create | `readClientPreface(r io.Reader) error` — reads exactly 24 bytes from r and compares against `[]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")` (RFC 9113 §3.4). Mismatch → `connError(PROTOCOL_ERROR, "bad preface")`. EOF mid-preface → `connError(PROTOCOL_ERROR, "short preface")`. The constant `clientPrefaceBytes` is exported within the package for tests. |
| `internal/filter/hcm/h2/preface_test.go` | Create | Good preface bytes → nil; truncated preface → `h2: PROTOCOL_ERROR` (short); single-byte tampering at each of 24 positions → `h2: PROTOCOL_ERROR` (bad); reader EOF before any byte → `h2: PROTOCOL_ERROR`; reader returns non-EOF error mid-preface → wrapped error. |
| `internal/filter/hcm/h2/framer.go` | Create | Thin context-aware wrapper over `http2.Framer`. `type framer struct { *http2.Framer; conn net.Conn }`. `newFramer(conn net.Conn) *framer` constructs the wrapped framer with `http2.NewFramer(conn, conn)`. `(*framer) readFrameCtx(ctx context.Context) (http2.Frame, error)` sets `conn.SetReadDeadline(deadline)` from `ctx.Deadline()` (or short-poll deadline if ctx has no deadline) and translates `os.IsTimeout(err) && ctx.Err() != nil` into `ctx.Err()` for clean cancellation; otherwise returns `framer.ReadFrame()`'s error verbatim. Write methods (`WriteSettings`, `WriteSettingsAck`, `WriteHeaders`, `WriteData`, `WriteRSTStream`, `WriteWindowUpdate`, `WritePing`, `WriteGoAway`) are passthrough — exposed unchanged by struct embedding. NO use of `http2.Framer.WriteRawFrame` per SPEC §4.1. Justified by ADR-0046 (codec source decision). |
| `internal/filter/hcm/h2/framer_test.go` | Create | Roundtrip via `net.Pipe` between two framers: SETTINGS → SETTINGS_ACK; HEADERS write → read decodes the same HeaderField slice (with hpack Decoder applied at the read side); DATA roundtrip; PING/PING_ACK; WINDOW_UPDATE; RST_STREAM; GOAWAY. Ctx-cancellation: `readFrameCtx` with a ctx that cancels mid-block returns `ctx.Err()` (a context.Canceled), not a deadline-exceeded error. Bad frame at the wire (write a malformed frame via raw bytes) → `framer.ReadFrame` returns an `http2.ConnectionError` or similar — the test asserts the wrapper does not panic and propagates the error. |
| `internal/filter/hcm/h2/hpack.go` | Create | Per-conn HPACK state. `type hpackState struct { enc *hpack.Encoder; encBuf bytes.Buffer; dec *hpack.Decoder; decFields []hpack.HeaderField }`. `newHPACKState(maxTableSize uint32) *hpackState` initializes encoder + decoder both with `maxTableSize` (defaults to 4096 per ADR-0047). `(*hpackState) encodeHeaders(headers []hpack.HeaderField) []byte` resets `encBuf`, writes each header via `enc.WriteField`, returns the encoded bytes (caller must copy if storing). `(*hpackState) decodeBlock(headerBlock []byte, endBlock bool) ([]hpack.HeaderField, error)` feeds bytes into `dec.Write` (and on `endBlock`, calls `dec.Close()`); on decoder error returns `connError(COMPRESSION_ERROR, ...)`. `(*hpackState) updateMaxTableSize(size uint32)` propagates a SETTINGS_HEADER_TABLE_SIZE change from the peer to `enc.SetMaxDynamicTableSize` so our outgoing tables shrink/grow on demand. |
| `internal/filter/hcm/h2/hpack_test.go` | Create | Encode then decode roundtrip preserves a 6-field header set (with `:status`/`:method` pseudo-headers + regular headers). Adversarial header block: arbitrary bytes through `decodeBlock` → `h2: COMPRESSION_ERROR` (no panic). `updateMaxTableSize(64)` shrinks the encoder side; subsequent encoded output respects the new size (the test compares encoded length before vs after the shrink with the same headers — within a small tolerance). Empty block → empty field slice. |
| `internal/filter/hcm/h2/flow.go` | Create | Connection-level + per-stream flow-control windows. `type window struct { mu sync.Mutex; n int32; ch chan struct{} }`. `newWindow(initial int32) *window` (initial = 65535 per ADR-0047). `(*window) reserve(n int32) (taken int32, err error)` decrements up to n and returns the actually-decremented amount; if the window is ≤ 0, blocks on `ch` and re-tries. `(*window) replenish(delta int32)` increments and signals `ch`. `(*window) waitFor(ctx context.Context, n int32) error` blocks until ≥ n is available or ctx cancels. The connection-level send window is one shared instance owned by the conn; each per-stream send window is its own instance owned by the serverStream; recv windows mirror this with the additional concern of consuming-then-WINDOW_UPDATE-emitting. |
| `internal/filter/hcm/h2/flow_test.go` | Create | Single producer/single consumer: `reserve(100)` on a window of 65535 returns 100 and decrements to 65435; subsequent `reserve(100000)` blocks until `replenish(100000)` is called from another goroutine; ctx-cancel during a blocking `waitFor` returns `ctx.Err()`. Multi-consumer: two concurrent reservers, partial allocations, eventual full delivery via `replenish`. Tiny-window stress: initial 1, send 100 bytes, replenish 1 at a time → eventual delivery (this is SPEC §11.5 mitigation). |
| `internal/filter/hcm/h2/settings.go` | Create | `type ServerSettings struct { MaxConcurrentStreams, InitialWindowSize, MaxFrameSize, EnablePush, NoRFC7540Priorities, HeaderTableSize uint32 }`. `var defaultServerSettings = ServerSettings{MaxConcurrentStreams: 100, InitialWindowSize: 65535, MaxFrameSize: 16384, EnablePush: 0, NoRFC7540Priorities: 1, HeaderTableSize: 4096}` (per ADR-0047). `writeServerInitialSettings(fr *framer, s ServerSettings) error` writes a single SETTINGS frame carrying the six values via `http2.SettingsFrame{...}` shape. `readClientSettings(fr *framer, applyTo *clientSettings) error` reads one SETTINGS frame; if the frame has `ACK` set on the very first read, it's a protocol error (RFC 9113 §6.5: server's initial SETTINGS must be ACK'd, but the server reads its own ACK *after* reading the client's initial SETTINGS). The conn driver issues SETTINGS_ACK on receipt of client SETTINGS and waits for SETTINGS_ACK in response to its own (orchestrated in `conn.go`). |
| `internal/filter/hcm/h2/settings_test.go` | Create | Roundtrip via `net.Pipe` framer pair: server writes initial SETTINGS, peer reads them and sees the six values; peer writes SETTINGS_ACK, server reads it. Client SETTINGS roundtrip: peer writes a SETTINGS frame with `INITIAL_WINDOW_SIZE=128`; server reads it and `clientSettings.initialWindowSize == 128`. ACK-on-first-read → protocol error. |
| `internal/filter/hcm/h2/stream.go` | Create | Per-stream server-side state machine. `type streamState int` with const block `streamIdle`/`streamOpen`/`streamHalfClosedRemote`/`streamHalfClosedLocal`/`streamClosed`. `type serverStream struct { id uint32; state streamState; sendW, recvW *window; reqHeaders []hpack.HeaderField; reqBodyR *io.PipeReader; reqBodyW *io.PipeWriter; endStream bool; mu sync.Mutex; conn *ServerConn }`. Methods: `recvHeaders(headers, endStream) error` (idle→open or open→halfClosedRemote); `recvData(b, endStream) error` (writes to `reqBodyW`; on endStream closes it and transitions to halfClosedRemote); `recvRSTStream(code) error` (transitions to closed; closes `reqBodyW`); `recvWindowUpdate(delta) error` (calls `sendW.replenish(delta)`); `dispatch(ctx, table)` (called once on END_STREAM-on-headers OR END_STREAM-on-data; builds `*http.Request` from `:method`/`:path`/`:scheme`/`:authority` + `reqHeaders` + `reqBodyR`; calls `table.Match(req)`; on `directResponseAction` → `action.writeH2(s.streamWriter())`; on `routerAction` → `s.sendRSTStream(INTERNAL_ERROR)` per SPEC §5.2 step 4c; on no match → 404 `directResponseAction` synthesized in-band). `(*serverStream) streamWriter()` exposes `WriteHeaders`/`WriteData` against the parent ServerConn's framer + hpack state. The server-side stream-id allocation rule: even-numbered client-initiated stream IDs → `connError(PROTOCOL_ERROR, "even client stream id")`. Stream-id reuse → `connError(PROTOCOL_ERROR, "stream id reuse")`. |
| `internal/filter/hcm/h2/stream_test.go` | Create | State-machine transitions: idle → open → halfClosedRemote → closed via HEADERS-no-END_STREAM + DATA-END_STREAM; idle → open → halfClosedLocal via HEADERS-with-END_STREAM (server end-stream after sending response); even-client-stream-id rejected; stream-id reuse rejected; RST_STREAM closes; WINDOW_UPDATE replenishes send window. Dispatch: `directResponseAction` produces HEADERS+DATA via the streamWriter; `routerAction` produces RST_STREAM(INTERNAL_ERROR); no-match produces a 404 direct_response. Tests use a fake ServerConn with a captured framer to validate the wire output. |
| `internal/filter/hcm/h2/conn.go` | Create | `type ServerConn struct { ctx context.Context; downstream net.Conn; table *hcm.RouteTable; settings ServerSettings; fr *framer; hpack *hpackState; sendW, recvW *window; streams map[uint32]*serverStream; nextRecvID uint32; goawayCode uint32 }`. `NewServerConn(ctx, downstream, table, settings) *ServerConn` constructs the value. `(*ServerConn) Run() error` — the connection loop: (1) `readClientPreface(downstream)` (2) `writeServerInitialSettings(fr, settings)` (3) `readClientSettings(fr, &clientSettings)` and write SETTINGS_ACK (4) read SETTINGS_ACK for our own (5) enter the frame-dispatch loop. Frame dispatch by type: HEADERS-on-new-stream → construct serverStream, store in `streams`, spawn dispatch goroutine on END_STREAM; HEADERS-on-existing-stream (trailers) → discard per SPEC §2.1 trailer rule; DATA → route to stream's recvData; SETTINGS → apply + ack; SETTINGS_ACK (in response to our initial SETTINGS) → discard; PING → emit PING_ACK; PING_ACK → discard; WINDOW_UPDATE (stream 0) → adjust connection send window; WINDOW_UPDATE (stream N) → route to stream's recvWindowUpdate; RST_STREAM → route to stream's recvRSTStream; GOAWAY (received) → mark conn for graceful close; PUSH_PROMISE (received from client; clients can't legally send these) → `connError(PROTOCOL_ERROR, ...)` + GOAWAY; PRIORITY → silently discard per SPEC §2.1; CONTINUATION → handled by the framer's HEADERS reassembly transparently. On any connection-scoped `*Error`, emit GOAWAY with the error code, drain pending writes, return the error. On `ctx.Done()`, emit GOAWAY(NO_ERROR), close. Returns `nil` on clean shutdown; `*Error` on protocol violations. The `Filter.Handle` caller drops the error per the phase-02 `_ = io.Copy` precedent. |
| `internal/filter/hcm/h2/conn_test.go` | Create | End-to-end via `net.Pipe`: drive an `http2.Transport` (test peer; driver-side use OK per D-3.2) against a `ServerConn` running over the pipe. Tests: simple GET / direct_response 200 OK roundtrip; HEADERS-with-END_STREAM-no-DATA path; HEADERS+DATA+END_STREAM path; multiple concurrent streams (open 3, complete in arbitrary order); MAX_CONCURRENT_STREAMS enforcement (101st concurrent stream → REFUSED_STREAM); SETTINGS handshake completes; GOAWAY emit on protocol error (e.g., even client stream id); PING + PING_ACK roundtrip; HPACK dynamic table size update from peer SETTINGS — next outgoing HEADERS encoded under the new size; flow-control with `INITIAL_WINDOW_SIZE=1` and a 1024-byte response — eventual full delivery via WINDOW_UPDATE-driven progress (SPEC §11.5 mitigation); bad preface bytes → `h2: PROTOCOL_ERROR` and conn close; PRIORITY frame received → silently discarded (no state change in `streams`); PUSH_PROMISE from client → GOAWAY(PROTOCOL_ERROR). The tests collectively exercise SPEC §4.1's enumerated unit-test set. |
| `internal/filter/hcm/h2/fuzz_test.go` | Create | `FuzzFrameStream(f *testing.F)`: seed corpus of 3 well-formed frame sequences (preface only; preface + SETTINGS + SETTINGS_ACK; preface + SETTINGS + HEADERS + DATA + END_STREAM). Fuzz body mutates the byte sequence and feeds it through a `ServerConn.Run()` driven over a `net.Pipe`; asserts no panic and that any returned error begins with `h2:`. `FuzzHPACKDecode(f *testing.F)`: seed corpus of 3 well-formed header blocks; fuzz body feeds adversarial bytes through `hpackState.decodeBlock` and asserts no panic. Short-budget `-fuzztime=30s` per ADR-0018. |
| `internal/filter/hcm/actions.go` | Modify | Codec-neutral factoring of `directResponseAction` per SPEC §5.5 and ADR-0045. The struct gains: `body() (status int, headers http.Header, body []byte)` returning the synthesised reply (status from the configured value; headers populated with `Date: <imf-fixdate>`, `Server: envoy`, `Content-Type: text/plain`, `Content-Length: <n>`; body bytes from the configured `inline_string`). `writeH1(w io.Writer) error`: extracts the existing phase-04 `do` body via the new helper — writes the same wire bytes as `writeStatusReply(w, a.status, a.body)` (preserved byte-for-byte; the H1 adapter calls into the SAME `writeStatusReply` from `codec.go` so there is zero behavioural delta). `writeH2(sw streamWriter) error` is NEW: encodes a HEADERS frame carrying `:status` (pseudo first per RFC 9113 §8.3) followed by `Date`/`Server`/`Content-Type`/`Content-Length` as regular headers, then a single DATA frame with the body bytes and END_STREAM. The existing `do(ctx, req, bw)` becomes a one-line shim: `return a.writeH1(bw)` — preserves the `routeAction` interface contract. The `routerAction` type is **unchanged** in 05.1 (no H2 variant). The `streamWriter` interface lives in `internal/filter/hcm/h2/stream.go` and is satisfied by `(*serverStream).streamWriter()`'s return value; `actions.go` imports `internal/filter/hcm/h2` to reference the interface — but ONLY the interface; no concrete h2 types leak into `actions.go`'s public surface. |
| `internal/filter/hcm/actions_test.go` | Modify | Extend: `directResponseAction.body()` returns the expected status/headers/body triple for status 200, 404, 500 (golden assertions); `writeH1` output is byte-identical to phase-04's `do` for the same input (golden test against a captured byte string in `internal/filter/hcm/testdata/direct_response_h1.golden` — see Task 10 for the golden capture procedure); `writeH2` writes HEADERS-then-DATA-with-END_STREAM via a fake streamWriter that captures the calls; HEADERS pseudo-header ordering rule (`:status` first) is verified; `Content-Length` matches `len(body)`. The phase-04 `TestDirectResponseDo` is replaced by `TestDirectResponseWriteH1Compat` (golden-byte equivalence) + `TestDirectResponseWriteH2Compat` (sequence assertion). |
| `internal/filter/hcm/codec.go` | *Unchanged* | Phase-04's `writeStatusReply`, `serverHeader`, `dateHeader` are still consumed by `directResponseAction.writeH1` (under the hood). No code change in this file. |
| `internal/filter/hcm/route.go` | *Unchanged* | Phase-04's `routeTable`, `routeEntry`, `routeMatch` interface are reused verbatim by the H2 dispatch path. NO type alias / capitalization is added to this file — see `## Settled SPEC §10 deferred decisions` #10 (import-cycle resolution): the H2 sub-package gets a typed handle on the route table via the new adapter file `internal/filter/hcm/h2dispatch.go` (Task 9), not via a `route.go` export. |
| `internal/filter/hcm/connection.go` | *Unchanged* | Phase-04's `runConnection(ctx, downstream, table)` is the H1 dispatch target of `Filter.Handle`'s ALPN switch. No code change beyond the type-alias visibility lift in `route.go` (which doesn't change call sites). The H1 driver continues to invoke `entry.action.do(ctx, req, bw)` which now (via the actions.go shim) calls `directResponseAction.writeH1(bw)` — byte-identical wire output. Verified by Task 10's golden test + the existing fixture-0003 differential gate. |
| `internal/filter/hcm/filter.go` | Modify | `Filter.Handle(ctx, downstream)` learns ALPN dispatch (per ADR-0050). The new dispatch logic: switch on `f.codecType` (a new field — either added to `Filter` struct in config.go or encoded as a method derived from the parsed proto). For `HTTP1` → call `runConnection` unchanged. For `HTTP2` → if `downstream.(*stdtls.Conn)` succeeds, call `tlsConn.HandshakeContext(ctx)` (idempotent for already-completed handshakes; defensive per SPEC §11.6); dispatch to `h2.NewServerConn(ctx, downstream, f.table, h2.DefaultServerSettings).Run()`; if downstream is plaintext (`net.Conn` not `*stdtls.Conn`), this is the h2c path — only reachable when build-time validation accepted `HTTP2` on plaintext via `--allow-h2c`; dispatch directly to `h2.ServerConn`. For `AUTO` → if downstream is `*stdtls.Conn` and `NegotiatedProtocol == "h2"`, dispatch to `h2.ServerConn`; otherwise (plaintext or TLS-h1 or TLS-empty-ALPN) dispatch to `runConnection`. Any error returned by `h2.ServerConn.Run()` is logged as `hcm: h2: %v` and the downstream is closed (mirrors phase-02 `tcpproxy.Filter.Handle`'s log+close discipline). The new codec-type field on `Filter` is added in this task's config.go change (Task 12). |
| `internal/filter/hcm/filter_test.go` | Modify | Extend with ALPN dispatch tests using a real `*stdtls.Conn` pair (loopback `net.Listen`-backed) with `alpn_protocols: ["h2"]`: H2 path dispatches to the H2 driver (assert via a sentinel — e.g., the H2 driver's preface-read times out in a reasonable interval since no preface arrives); H1 path dispatches to `runConnection` (assert by writing an HTTP/1.1 request and reading a 200 back). Plaintext + `codec_type: HTTP2` + `listenerCtx{allowH2C: true}` → dispatches directly to H2 driver. The test uses fixtures.go-style helpers from existing `tls_test.go` patterns. |
| `internal/filter/hcm/config.go` | Modify | Two changes: (1) accept `codec_type: HTTP2` (was `HTTP1`/`AUTO` only); (2) add `http2_protocol_options` to the silent-ignore set per ADR-0041's amendment (the directly-on-HCM proto field, distinct from cluster-side `HttpProtocolOptions` which remains 05.2's surface). The `Filter` struct gains a `codecType hcmv3.HttpConnectionManager_CodecType` field (or a small enum type internal to hcm — implementation choice). `parseFilter` accepts a new parameter `lc listenerCtx` (or, equivalently, `parseFilterWithCtx`); the existing `parseFilter` becomes a shim calling the new variant with `listenerCtx{}` (zero value: `hasTLS=false, allowH2C=false`). Build-time validation: `codec_type=HTTP2` AND `!lc.hasTLS` AND `!lc.allowH2C` → error `hcm: codec_type HTTP2 requires TLS transport_socket (or --allow-h2c for conformance testing)`. `codec_type=AUTO` is permitted on either TLS or plaintext (on plaintext the runtime fall-through resolves AUTO → H1 unchanged). `codec_type=HTTP3` continues to error. `NewFilter` gains a new variant `NewFilterWithCtx(tc, cm, lc)`; the existing `NewFilter` calls it with the zero-value `listenerCtx`. The listener manager (Task 11) calls the `WithCtx` variant. |
| `internal/filter/hcm/config_test.go` | Modify | Extend: codec_type=HTTP2 on a plaintext listener (lc.hasTLS=false, lc.allowH2C=false) → error matching `codec_type HTTP2 requires TLS`; codec_type=HTTP2 on a TLS listener (lc.hasTLS=true) → success; codec_type=HTTP2 on a plaintext listener WITH lc.allowH2C=true → success; codec_type=AUTO on a TLS listener with alpn_protocols=["h2","http/1.1"] → success; codec_type=AUTO on a plaintext listener → success (resolves to H1 at runtime, build-time accepts); `http2_protocol_options` field (anywhere on HCM) → silently ignored, parse succeeds. |
| `internal/listener/manager.go` | Modify | New constructor variant `NewManagerWithBaseDirAndAllowH2C(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, baseDir string, allowH2C bool) (*Manager, error)` — same body as `NewManagerWithBaseDir` but threads `allowH2C` into a per-listener `listenerCtx{hasTLS, allowH2C}` value passed into the HCM filter constructor via the `filterRegistry` map. The existing `NewManager(bs, cm)` and `NewManagerWithBaseDir(bs, cm, baseDir)` delegate to the new variant with `allowH2C=false`. The `filterRegistry` constructor signature changes from `func(tc *anypb.Any, cm *cluster.Manager) (filterHandler, error)` to `func(tc *anypb.Any, cm *cluster.Manager, lc listenerCtx) (filterHandler, error)`; the tcpproxy entry's lambda discards `lc` (its filter doesn't use it); the HCM entry's lambda passes `lc` through to `hcm.NewFilterWithCtx`. The `hasTLS` field is computed per-chain from whether `tlsCfg != nil` (chainInfo at line 64). |
| `internal/listener/manager_test.go` | Modify | Extend: `NewManagerWithBaseDirAndAllowH2C` with allowH2C=true and a plaintext listener carrying HCM `codec_type: HTTP2` → builds successfully; same call with allowH2C=false → builds with the HCM's `codec_type HTTP2 requires TLS` error wrapped at the listener-manager boundary (`listener: %q: filter_chains[0]: hcm: ...`); a TLS listener with `alpn_protocols: ["h2", "http/1.1"]` and HCM `codec_type: AUTO` → builds successfully (default `allowH2C=false` is fine because the TLS path is allowed). |
| `internal/bootstrap/bootstrap.go` | *Unchanged* | No new blank import in 05.1. The cluster-side `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` import is 05.2's deliverable per ADR-0045. The HCM proto blank import landed in phase 04 (per `bbe298f`'s blank-import block) and continues to cover the `http2_protocol_options` field's resolution at protojson time. |
| `internal/cluster/cluster.go` | *Unchanged* | Per SPEC §4.2 + ADR-0045. No `UseH2()` accessor in 05.1. |
| `internal/cluster/manager.go` | *Unchanged* | Per SPEC §4.2 + ADR-0045. No `HttpProtocolOptions` reader in 05.1. |
| `internal/tls/` | *Unchanged* | Per SPEC §4.2. The phase-03 `alpn_protocols` plumbing already covers the listener-side ALPN need. |
| `internal/filter/tcpproxy/` | *Unchanged* | Phase-02 carryover. Verified at Task 16 gate sweep that `FuzzTcpProxyFilter` runs clean with no regression. |
| `cmd/envoy-go/main.go` | Modify | Add `--allow-h2c` CLI flag (per ADR-0049). Default OFF. Documented in `--help` as "test-only; not for production — permits HCM codec_type:HTTP2 on plaintext listeners for h2spec conformance only". Plumbed through the `listener.NewManagerWithBaseDirAndAllowH2C` call in place of the existing `NewManagerWithBaseDir` call. The flag is a `*bool` from `flag.Bool("allow-h2c", false, ...)`; no environment-variable form, no build-tag form (per ADR-0049 which decides on the CLI form). |
| `cmd/envoy-go/main_test.go` | Modify | Add `TestEnvoyGoBinary_H2Smoke`: a TLS listener with `alpn_protocols: ["h2"]` and HCM `codec_type: HTTP2`, one direct_response route (`path: /health` → 200 `OK\n`). Uses a self-signed cert generated in-test (mirror `test/fixtures/0002-tls-tcp/pki/gen` patterns or the simpler in-test gen at `test/helpers/tls.go`). Spawns the envoy-go binary; issues an HTTP/2-over-TLS request via `golang.org/x/net/http2.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}}` (driver-side use OK); asserts status 200 and body `OK\n`. The phase-04 H1 + phase-02 TCP smoke variants remain unchanged. The h2c smoke variant is intentionally NOT duplicated here — `test/conformance/h2spec/` exercises the h2c path comprehensively. |
| `test/conformance/h2spec/h2spec.go` | Create | NEW conformance-suite helper (no `_test.go` suffix). Contains: `const h2specImage = "summerwind/h2spec@sha256:..."` (the pinned reference, sourced from `docs/envoy-go/CONFORMANCE_PINS.md`); `var thresholdSections = []string{"3", "4", "5", "6", "7", "8"}` and `var excludedSubsections = []string{"6.6"}` (per ADR-0051); `type junitReport struct { ... }` defining the structured XML decode shape h2spec emits via `--junit-report`; `parseJUnit(b []byte) ([]testCase, error)` parses the XML; `checkThreshold(cases []testCase) error` returns nil if all cases under threshold sections (excluding 6.6) passed, else a structured error naming the first failure. The threshold-section-list constant lives here (rather than inlined in the test file) so that `BEHAVIOR_CONTRACT.md ## HTTP/2`'s narrative form references the same source-of-truth. |
| `test/conformance/h2spec/h2spec_test.go` | Create | `TestH2Spec(t *testing.T)`: skip on `-short`; build envoy-go binary into a temp dir (`go build -o $tmp/envoy-go ./cmd/envoy-go`); generate a synthetic h2c bootstrap YAML (1 listener on `127.0.0.1:0` plaintext, 1 filter chain with empty match, 1 HCM with `codec_type: HTTP2`, 1 route_config with 1 vhost serving 1 catch-all `direct_response: 200 "OK\n"`); start envoy-go with `--allow-h2c -c <bootstrap>`; wait for the `envoy-go ready` sentinel (mirror of phase-04 / phase-02 sentinel polling); start `summerwind/h2spec` container via `testcontainers-go.GenericContainer` pinned by the `h2specImage` constant + `--add-host=host.docker.internal:host-gateway` for container→host reachability; exec h2spec inside the container with `--host=host.docker.internal --port=<dyn> --strict --junit-report=/tmp/h2spec.xml`; wait for completion; `docker cp` the JUnit report to the host (or use a `BindMount` per testcontainers-go pattern); read + parse via `parseJUnit`; assert `checkThreshold(cases) == nil` reporting any failure with the section number + test name + h2spec detail; stop subject + container. Runtime budget: ~30s wall-clock; CI gate is `go test ./test/conformance/h2spec/...`. |
| `test/conformance/h2spec/testdata/h2c-bootstrap.yaml` (or generated inline) | Create | The synthetic h2c bootstrap consumed by `h2spec_test.go`. Planner choice: file-vs-inline. **Decision: inline-templated in the test driver** — same shape as fixture-0001's `subjectTmpl` heredoc; the bootstrap is conformance-only and not a differential fixture, so a `testdata/` file would imply a stability contract it doesn't have. The planner records the choice in PLAN at write time; not ADRd (implementation detail). |
| `test/differential/` | *Unchanged* | No new fixture, no driver-runner change. The `runner_test.go` blank-import for fixture 0004 is 05.2's deliverable (the import line for `_ "github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver"` is added by 05.2). |
| `test/helpers/h2.go` | *Not in 05.1* | 05.2 deliverable (per ADR-0045). The H2RoundTrip helper and the h2-over-TLS round-trip primitive live in 05.2; 05.1's conformance suite uses h2spec's own client (a Docker container), and 05.1's `cmd/envoy-go/main_test.go` h2 smoke variant uses `x/net/http2.Transport` inline (driver-side use OK). |
| `test/helpers/tls.go` | *Possibly extended* | If `cmd/envoy-go/main_test.go`'s `TestEnvoyGoBinary_H2Smoke` needs an in-test self-signed cert with ALPN `h2` advertised, `test/helpers/tls.go` may grow a small `H2TLSConfig()` helper (~30 LoC). Planner records the decision: **introduce `test/helpers/tls.go` (NEW file, ~30 LoC) only if `cmd/envoy-go/main_test.go`'s h2 smoke variant cannot reasonably reuse `test/fixtures/0002-tls-tcp/pki/`'s committed PKI**. Default expectation: use fixture-0002 PKI (it already has an `h2`-capable cert chain via the standard TLS config; advertising `h2` in `alpn_protocols` is the only addition needed). Recorded in PLAN at write time; not ADRd. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | Modify | Two changes per ADR-0052: (1) extend the **Header allow-list** table with five rows: `:status` (HCM-locally-generated H2 responses; required + value-asserted; Phase 05.1; ADR-0052), `:method` (forward-looking; routed-to-upstream H2 requests; applies-to: 05.2; Phase 05.1; ADR-0052), `:path` (same), `:scheme` (same), `:authority` (same). (2) Add a new top-level `## HTTP/2` section after `## HTTP/1.1`, content per SPEC §5.7 in SCAFFOLD form: `### Asserted equivalence (05.1 scope)`, `### Not asserted (05.1 scope)`, `### Header allow-list extensions`, `### h2spec threshold`, `### Applies to (05.1)`, `### Does not yet apply to`. The "Does not yet apply to" enumeration explicitly defers fixture-0004 + routed-to-upstream rules to 05.2; the 05.2 brainstorming session edits this subsection in place to flip the deferred items per ADR-0052's authorisation. |
| `docs/envoy-go/CONFORMANCE_PINS.md` | Create | NEW pin file mirroring `ENVOY_TARGET.md`'s discipline. Header explaining purpose; "Pins" section with `summerwind/h2spec` tag (e.g. `v2.6.0`) + SHA256 (the digest pulled at PLAN-execution time; the planner verifies the pin is reproducible by `docker pull summerwind/h2spec@sha256:...`); "Refresh procedure" section documenting that pin updates are dedicated phase work per D-3.7; "Cross-references" section linking ADR-0051, BEHAVIOR_CONTRACT `## HTTP/2`, and `test/conformance/h2spec/`. The exact tag + SHA256 values are filled at Task 15 (the conformance-suite landing) by running `docker pull summerwind/h2spec:v2.6.0 && docker inspect --format '{{.Id}}' summerwind/h2spec:v2.6.0`. |
| `docs/envoy-go/DECISIONS.md` | Modify | Append ADR-0046 through ADR-0053 (eight ADRs — listed in `## ADRs introduced by this plan` below). Each ADR lands in the same commit as the code that consumes it (phase-00..04 precedent). |
| `docs/envoy-go/ROADMAP.md` | *Not modified by this plan* | Row 05.1 is already `in-progress` (the SPEC commit `4b45941` flipped it). Advances to `done` at state-machine step 6 in a later session per ADR-0005. Row 05 (parent) stays `in-progress` until both 05.1 and 05.2 are `done`. Row 05.2 stays `planned` until 05.1 is `done`. |
| `docs/envoy-go/STATE.md` | Modify (at exit) | Advanced to `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development` at this plan-authoring session's exit commit — matching phases 02/03/04's exit discipline per ADR-0005 §1. |
| `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md` | Create (during execution) | Append-only running log per BOOTSTRAP §5 step 3, matching phase-00..04 conventions. Created by the executor at Task 1, not by this plan-authoring session. |

---

## ADRs introduced by this plan

Eight ADRs land at execution time. Each is the first-use task's responsibility and goes into the same commit as the code that consumes it. All entries in `DECISIONS.md` are append-only (D-3.5); no landed ADR is edited. ADR numbering continues from the tail verified at PLAN-write time (**ADR-0045** is the current tail, verified by `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` → `## ADR-0045:` at `docs/envoy-go/DECISIONS.md:1450`). Per SPEC §4.4 + ADR-0045 consequence "ADR numbering shift", phase 05.1's eight ADRs land at ADR-0046..ADR-0053. The SPEC anticipated lettered ADR-P/Q/S/T/U/V/Z/X — the assigned numbers below are sequenced so that **first-use commit order matches DECISIONS.md file order** (phase-02/03/04 precedent).

The SPEC-letter-to-ADR-number map:

- **SPEC §4.4 ADR-P** (HTTP/2 codec source) → **ADR-0046** (lands Task 4, first use of `http2.Framer` via the new `framer.go`).
- **SPEC §4.4 ADR-S** (server settings defaults) → **ADR-0047** (lands Task 7, first use of `defaultServerSettings` + `writeServerInitialSettings`).
- **SPEC §4.4 ADR-Q** (server connection manager from scratch) → **ADR-0048** (lands Task 8, first use of `serverStream` state machine + `(*ServerConn)`-shaped consumer).
- **SPEC §4.4 ADR-Z** (test-only `--allow-h2c` flag) → **ADR-0049** (lands Task 11, first use of `listenerCtx{allowH2C}` plumbed through the listener manager).
- **SPEC §4.4 ADR-V** (ALPN dispatch wiring) → **ADR-0050** (lands Task 12, first use of `Filter.Handle` ALPN switch).
- **SPEC §4.4 ADR-U** (h2spec threshold + pin) → **ADR-0051** (lands Task 15, first use of `CONFORMANCE_PINS.md` + threshold-section list).
- **SPEC §4.4 ADR-T** (BEHAVIOR_CONTRACT scaffold) → **ADR-0052** (lands Task 16, first use of `BEHAVIOR_CONTRACT.md ## HTTP/2`).
- **SPEC §4.4 ADR-X** (phase-04 REVIEW carry-forward triage) → **ADR-0053** (lands Task 16, alongside ADR-0052 — same closing commit).

Summaries:

- **ADR-0046 (= SPEC ADR-P) — HTTP/2 codec source: `golang.org/x/net/http2.Framer` + `golang.org/x/net/http2/hpack` as low-level codec only.** Options considered: (P1) handcrafted RFC 9113 framer + handcrafted HPACK (highest control, highest cost — HPACK alone is a non-trivial dynamic-table state machine with security-relevant CVE history); (P2) x/net/http2 sub-packages used as low-level codec only (this PLAN's choice); (P3) build on `http2.Server` / `http2.Transport` (FORBIDDEN by D-3.2). (P2) keeps the doctrine intent (own connection manager, own state machine, own dispatch) while sidestepping the (P1) cost-of-correctness tax. Records the boundary: x/net owns frame byte-layout serialisation and HPACK table state; envoy-go owns the entire connection lifecycle, settings handshake, stream demux, flow control, error dispatch, GOAWAY/RST_STREAM/PING semantics, and the bridge to HCM's filter chain. The grep-verifiable boundary at Task 16: `! grep -nR '"golang.org/x/net/http2"' internal/ cmd/envoy-go/main.go` (excluding `_test.go`) returns zero hits OUTSIDE `internal/filter/hcm/h2/framer.go`/`hpack.go`/`settings.go` — the three files that legitimately import the package. Lands in Task 4. Supersedes nothing.

- **ADR-0047 (= SPEC ADR-S) — Phase-05.1 H2 server settings defaults: `MAX_CONCURRENT_STREAMS=100`, `INITIAL_WINDOW_SIZE=65535`, `MAX_FRAME_SIZE=16384`, `ENABLE_PUSH=0`, `NO_RFC7540_PRIORITIES=1`, `HEADER_TABLE_SIZE=4096`.** Rationale: matches Envoy's documented defaults (MAX_CONCURRENT_STREAMS=100 per Envoy's `Http2ProtocolOptions` doc); matches RFC 9113 protocol defaults where Envoy doesn't override (INITIAL_WINDOW_SIZE=65535, MAX_FRAME_SIZE=16384, HEADER_TABLE_SIZE=4096). `ENABLE_PUSH=0` because phase 05.1 disables server push (SPEC §2.1). `NO_RFC7540_PRIORITIES=1` informs clients we will silently discard PRIORITY frames (RFC 9113 §6.3 / SPEC §2.1). The differential gate does not assert SETTINGS values byte-for-byte (those are inside the structurally-equivalent framing rule per BEHAVIOR_CONTRACT `## HTTP/2`); h2spec section 6.5 only asserts RFC 9113 compliance, not Envoy-specific values. Per-listener `http2_protocol_options` field-level fidelity is silently-ignored in 05.1 (the directly-on-HCM proto field; the cluster-side typed-extension is 05.2's surface) — ADR-0041 amended to record the `http2_protocol_options` addition to the silent-ignore set. Lands in Task 7. Supersedes nothing.

- **ADR-0048 (= SPEC ADR-Q) — HCM H2 server connection manager from scratch: own `ServerConn` + `serverStream`, NOT built on `http2.Server`/`ServeConn`/`ConfigureServer`/`Transport`/`NewClientConn`.** Documents the explicit decision NOT to use the Server-or-Transport runtime constructs that `golang.org/x/net/http2` exposes even though they ostensibly fit the "low-level" framing. Rationale: those types carry their own request-routing, header-canonicalization, response-header injection, and error policies that diverge from Envoy's; we'd have to fight or unwind those to match Envoy semantics. Building directly on `http2.Framer` + `hpack` is cheaper. Records the architectural shape of `ServerConn` (per-downstream-conn state machine: preface check, SETTINGS handshake, frame loop, GOAWAY emission) and `serverStream` (per-stream state machine: idle/open/halfClosedRemote/halfClosedLocal/closed transitions per RFC 9113 §5.1). Even-numbered client-initiated stream IDs → PROTOCOL_ERROR; stream-id reuse → PROTOCOL_ERROR. Client-side `ClientConn` lands in 05.2 under a follow-up ADR (deferred per ADR-0045). Lands in Task 8. Supersedes nothing.

- **ADR-0049 (= SPEC ADR-Z) — Test-only `--allow-h2c` flag on `cmd/envoy-go` to permit HCM `codec_type: HTTP2` on a plaintext listener for h2spec conformance only.** Documents the security posture (flag is not advertised in production-marketing surface; default OFF; production builds may strip the flag via a future build tag); the flag exists solely so `test/conformance/h2spec/` can run h2c against the subject without a TLS handshake stealing test cycles. Alternative considered: run h2spec over TLS — rejected because h2spec's TLS support requires a custom CA and complicates the conformance pin; running h2c is the documented h2spec convention. **Form decision: CLI flag** (vs env var or build tag) — chosen because the testcontainers driver already constructs the subject via `os/exec` and a CLI flag is the lowest-friction option for that driver. The flag accepts no value (boolean `--allow-h2c` / no flag = off); a value-bearing form was considered and rejected as over-engineered for a single use site. Plumbed through `listener.NewManagerWithBaseDirAndAllowH2C` → per-chain `listenerCtx{allowH2C}` → `hcm.NewFilterWithCtx` build-time validation. Lands in Task 11. Supersedes nothing.

- **ADR-0050 (= SPEC ADR-V) — ALPN-driven codec selection wiring inside `Filter.Handle`, NOT at the listener-side filter-chain match step.** Records the architectural choice that codec selection happens *inside* the HCM filter (`Filter.Handle` type-asserts `*tls.Conn`, reads `ConnectionState().NegotiatedProtocol`, dispatches to H1 or H2 driver), NOT at `filter_chain_match.application_protocols[]`. Rationale: keeps phase 03's filter-chain-match SNI-only surface unchanged (per ADR-0033), keeps phase 07's filter-chain framework as the natural home for `application_protocols` chain matching when it lands, and minimises blast radius (HCM gains a small dispatch helper; listener manager gains only a constructor variant + a `listenerCtx` thread). Documents the alternative (treat ALPN as a chain-match dimension) and why it was deferred. Documents the defensive `tls.Conn.HandshakeContext(ctx)` no-op call before reading `NegotiatedProtocol` (idempotent for already-completed handshakes; if a future refactor removes the listener-side handshake, the HCM still gets correct data — SPEC §11.6 mitigation). Lands in Task 12. Supersedes nothing.

- **ADR-0051 (= SPEC ADR-U) — h2spec conformance scope, threshold, and pin.** Pins `summerwind/h2spec` by tag + SHA256 in `CONFORMANCE_PINS.md` (NEW file in 05.1, mirror of `ENVOY_TARGET.md`'s discipline; refresh procedure is dedicated phase work per D-3.7). Declares the section list under threshold: 3 (HTTP Frame Format), 4 (HPACK), 5 (Streams and Multiplexing), 6 (Frame Definitions) MINUS 6.6 (PUSH_PROMISE), 7 (Error Codes), 8 (HTTP Message Exchanges). Excludes 6.6 because phase 05.1 disables push (`ENABLE_PUSH=0` per ADR-0047); the section is conformance-irrelevant. Records the per-section pass-count expected at phase-done as "all child tests under threshold sections must report `failed=0` in h2spec's JUnit-XML output". The pin's refresh procedure is documented in `CONFORMANCE_PINS.md`. **THIS GATE IS NEWLY NON-VACUOUS** for the project — gate (c) of the phase-done set is exercised non-vacuously for the first time here. Lands in Task 15. Supersedes nothing (project's first conformance ADR).

- **ADR-0052 (= SPEC ADR-T) — BEHAVIOR_CONTRACT `## HTTP/2` subsection (SCAFFOLD form for 05.1).** Codifies the phase-05.1 codec/conformance equivalence surface (see SPEC §5.7 + 05.1 PLAN File Structure entry for `BEHAVIOR_CONTRACT.md`). **Asserted equivalence (05.1 scope):** `:status` per request (asserted by h2spec section 8 on every `direct_response` invocation); decoded body bytes on `direct_response` 2xx paths (h2spec validates indirectly via response-length + END_STREAM checks; envoy-go's unit tests assert byte equality directly); per-stream response header set-equality modulo allow-list (`:status`/`Server`/`Content-Type`/`Content-Length`/`Date` on locally-generated H2 responses). Routed-to-upstream H2 surface NOT YET ASSERTED IN 05.1 (deferred to 05.2 + fixture 0004). **Not asserted (05.1 scope):** wire-byte H2 framing; SETTINGS values byte-for-byte; WINDOW_UPDATE timing or count; stream id allocation pattern; trailers; 0-RTT TLS early data; routed-to-upstream H2 (deferred). **Header allow-list extensions:** `:status` (active in 05.1; locally-generated H2 responses); `:method`/`:path`/`:scheme`/`:authority` (forward-looking, applies-to: 05.2 routed-to-upstream). **h2spec threshold:** 3, 4, 5, 6 ex-6.6, 7, 8 — pin via `CONFORMANCE_PINS.md` per ADR-0051. **Applies to (05.1):** phase-05.1 `internal/filter/hcm/h2/` package (server-side only); the codec-neutral `directResponseAction` factoring in `internal/filter/hcm/actions.go`; the conformance suite under `test/conformance/h2spec/`. **Does not yet apply to:** routed-to-upstream H2 (05.2 + fixture 0004); HTTP/3; server push; gRPC framing; trailer forwarding; upstream H2 stream pooling; h2c production fixtures; mTLS over h2. ADR-0052 explicitly authorises 05.2's brainstorming session to EDIT this subsection in place (not replace via supersession) to flip deferred items to active rules — the in-place-edit authorisation is a SCAFFOLD-pattern feature documented here so 05.2's planner knows the disposition. Lands in Task 16. Supersedes nothing.

- **ADR-0053 (= SPEC ADR-X) — Phase-04 REVIEW Minor carry-forward triage.** Records the phase-05.1 disposition of M-2/M-4/M-5/M-6/M-7 from `docs/envoy-go/phases/04-http-1.1/REVIEW.md` (commit `04527eb`). Decisions per SPEC §12 (also recorded in `## Phase-04 REVIEW carryover resolution matrix` below): **M-2** (ADR-0043 "Doctrine: D-3.4, D-3.5" mismatched against informal supersession qualifier) — DEFERRED (cosmetic; phase 05.1 does not touch ADR-0043; future doctrine-cleanup ADR supersedes ADR-0043 with corrected attribution). **M-4** (listener-manager `Stop()`/`Listeners()` race) — DEFERRED (phase 05.1 does not touch the lock surface; race carries forward to phase 08's admin-api-and-drain phase as the natural close, where drain semantics require a correct `Listeners()` snapshot). **M-5** (phase-04 SPEC §7 failure-mode prose vs `defer upstreamConn.Close()` mechanism) — DEFERRED (phase 05.1 does NOT introduce a parallel mechanism on the H2 path because routerActionH2 is 05.2's surface; the same prose-vs-mechanism shape will reappear in 05.2's `routerActionH2.do` with `defer clientConn.Close()` — 05.2's brainstorming inherits this disposition rather than re-litigating per ADR-0053's "phase-05.2-will-repeat-the-pattern" forward-looking note). **M-6** (fixture-0003 driver heredoc YAML pattern) — DEFERRED (phase 05.1 introduces no new fixture; structured-`expectations.yaml` plan from ADR-0019 still belongs to observability / phase-06 sweep). **M-7** (`Filter.statPrefix` stored but never consumed) — DEFERRED **with phase-06-must-consume tag** (phase 05.1 does not consume `Filter.statPrefix` either; phase 06's brainstorm is required to either honour `Filter.statPrefix` (lifting M-7 to RESOLVED) or supersede ADR-0041 with a stat-naming policy that obviates the field). Additionally, ADR-0053 records that 05.1 introduces a *new* prose-vs-mechanism shape on the H2 path (the `defer` cleanup in `serverStream.dispatch`'s action invocation; analogous to phase-04 M-5's H1 prose-vs-mechanism gap) — the cosmetic gap is acknowledged and deferred to the same future SPEC-corrections ADR. Lands in Task 16 alongside ADR-0052 — same closing commit. Supersedes nothing.

If an unforeseen decision surfaces during execution that has cross-phase impact (per D-3.5), the executor writes a new sequential ADR (ADR-0054+) in the same commit as the code it decides for. If such a decision would expand phase-05.1 scope beyond SPEC §1–§4, invoke `superpowers:systematic-debugging` and then either re-scope the task in place or split per `BOOTSTRAP_PROMPT.md` §6 — noting that 05.1 SPEC §11.1 mandates `blocked` over re-split because the codec-direction axis is exhausted.

---

## Settled SPEC §10 deferred decisions

SPEC §10 leaves five 05.1-scoped implementation-detail choices to the planner (items #3, #6, #7, #10 from the master phase-05 SPEC §10 are 05.2's planner concerns and are not repeated here per 05.1 SPEC §10 narrowing). This PLAN settles them so the executor does not re-litigate. Only decisions with cross-phase impact (security tightening, new mechanism choice, interface shape) are also captured as ADRs.

1. **Streaming-body dispatch vs wait-for-END_STREAM.** **Wait for END_STREAM before invoking the action.** Master SPEC §10 #1 + ADR-0045 prescribe this for 05.1 (per master SPEC §10 #1's recommended option (a)); SPEC §5.2 step 4 reaffirms. Rationale: 05.1's only reachable action on H2 is `directResponseAction` which doesn't consume the request body anyway; `routerAction` (which would benefit from streaming) is 05.2's surface. Full streaming-body filter dispatch lands with the phase-07 framework. The dispatch helper in `serverStream.dispatch` waits for END_STREAM-on-headers OR END_STREAM-on-data before calling the action. Codified in `serverStream.dispatch` (Task 8); not separately ADRd (implementation detail of 05.1's TDD test plan).

2. **Codec-neutral factoring of `directResponseAction.body()` now (vs keeping two writers later).** **Factor codec-neutral now, in 05.1.** Master SPEC §10 #2 + ADR-0045 prescribe this; SPEC §5.5 reaffirms; the h2spec section 8 (HTTP Message Exchanges) gate exercises `direct_response` so the H2 adapter is required in 05.1. Rationale: keeping two writers would require the action to know about the codec — bad layering. The codec-neutral `body() (status, headers, body)` returns the synthesized reply; the H1 adapter (`writeH1`) byte-preserves phase-04's wire bytes; the H2 adapter (`writeH2`) is new. Codified in Task 10; not separately ADRd (implementation detail; the cross-phase concern is the byte-preservation invariant, which the unit-test golden file enforces).

3. **`H2Request`/`H2Response` shape — server-side: re-use stdlib `*http.Request` / construct response in-band.** Master SPEC §10 #3 + ADR-0045 split this per-direction; the SERVER-side request type lives in 05.1; **decision: re-use stdlib `*http.Request`** so the route-table machinery (which operates on `req.URL.Path`) and the action interface stay single-shape. The H2 codec's `serverStream.dispatch` builds the `*http.Request` from `:method` (→ `req.Method`), `:path` (→ `req.URL.Path` and `req.URL.RawQuery`), `:scheme` (→ `req.URL.Scheme`), `:authority` (→ `req.Host` and `req.URL.Host`) pseudo-headers + decoded regular HEADERS (→ `req.Header`) + the body pipe reader (→ `req.Body`). The response is constructed in-band by `writeH2(streamWriter)` (no `*http.Response` is ever materialised on the H2 server side). The CLIENT-side request/response shape is 05.2's question. Codified in `serverStream.dispatch` (Task 8); not separately ADRd (implementation detail).

4. **Conformance pin location: `docs/envoy-go/CONFORMANCE_PINS.md` (NEW file) plus a Go const mirror in `test/conformance/h2spec/h2spec.go`.** Master SPEC §10 #4 prescribes the doc file; this 05.1 PLAN ratifies (File Structure lists `CONFORMANCE_PINS.md` as a new file; the const in `h2spec.go` is a mirror of the doc-file pin, with an `// authoritative pin: docs/envoy-go/CONFORMANCE_PINS.md` comment to make the doc file the single source of truth). Codified in ADR-0051.

5. **`--allow-h2c` form: CLI flag** (vs env var, vs build tag). Master SPEC §10 #5 left this open; **decision: CLI flag** (lowest-friction for the testcontainers driver per ADR-0049's rationale). The flag is a `bool` — no value-bearing form. The planner wires the flag in `cmd/envoy-go/main.go`. Codified in ADR-0049.

The master phase-05 SPEC §10 also has items #6 (per-cluster RR counter scope), #7 (per-cluster RR distribution dimension), #8 (`:status`-first vs `content-type`-first ordering), #10 (`expectations.yaml` shape for fixture 0004), and #14 (cluster-side `dial_h2.go` factoring). Items #6, #7, #10, #14 are 05.2's deliverable per ADR-0045 and are not settled in 05.1's PLAN. Item #8 (`:status`-first ordering) is a CORRECTNESS rule (RFC 9113 §8.3) not a deferred decision — the planner ensures phase-05.1's HEADERS encoding puts `:status` first in `directResponseAction.writeH2`; h2spec section 8 catches violations; not a deferred decision in the SPEC §10 sense.

Three additional 05.1-internal implementation choices (not in SPEC §10 but settled here so the executor doesn't re-litigate):

6. **`routeTable` visibility lift.** The H2 codec sub-package needs to dispatch through the existing phase-04 `routeTable`. **Decision: introduce a `type RouteTable = *routeTable` type alias in `route.go`** (or capitalise the type itself — alias preferred for zero call-site change). The alias is purely a visibility lift; the alternative (export `routeTable` directly by capitalising) would force a wider rename across `config.go`/`connection.go`/`filter.go`. Recorded here, not ADRd.

7. **`listener.NewManagerWithBaseDirAndAllowH2C` constructor variant vs an `Options` struct refactor.** **Decision: add a single-purpose constructor variant** matching the phase-04 precedent (`NewManagerWithBaseDir` was added in phase 03 to thread `baseDir`). An `Options` struct refactor is a wider change with no current driver beyond this single flag; defer to a future phase that accumulates a third option. Recorded here, not ADRd.

8. **`test/helpers/tls.go` introduction.** **Decision: do NOT introduce `test/helpers/tls.go` in 05.1** unless `cmd/envoy-go/main_test.go`'s h2-over-TLS smoke variant cannot reasonably reuse `test/fixtures/0002-tls-tcp/pki/`'s committed PKI. Default expectation: use fixture-0002 PKI (the cert chain is `h2`-capable; advertising `h2` in the listener's `alpn_protocols` is the only addition needed). If at execution time fixture-0002 PKI proves insufficient (e.g., SAN mismatch on the dynamic test port), Task 13 introduces `test/helpers/tls.go` (~30 LoC) carrying a `H2TLSConfig()` helper. Recorded here, not ADRd.

9. **`directResponseAction.body` field rename to `bodyText`** (resolves the method-vs-field name collision). SPEC §13's acceptance check requires "a `body()` method exists" on `directResponseAction`. Phase 04's struct shape has `body string` as a field, which prevents adding a method named `body()` (Go disallows method-and-field on the same struct sharing a name). **Decision: rename the struct field `body string` → `bodyText string` at Task 10**, and add the SPEC-mandated `body() (status int, headers http.Header, body []byte)` method. The rename is mechanical: every reference to `a.body` (in `directResponseAction.do`, in `buildDirectResponseAction`, in tests) updates to `a.bodyText`. Phase-04 wire output is preserved byte-for-byte (the rename does not affect the `writeStatusReply` argument shape — `writeStatusReply(w, a.status, a.bodyText)`). The golden-file test at Task 10 step 1 catches any byte-level regression. Recorded here, not ADRd.

10. **Import-cycle resolution between `internal/filter/hcm/` and `internal/filter/hcm/h2/`.** The H2 dispatch path needs an `internal/filter/hcm` ↔ `internal/filter/hcm/h2` connection (HCM `Filter.Handle` constructs an `h2.ServerConn`; `h2.serverStream.dispatch` invokes the matched action's `writeH2`). A naive shape (h2 imports hcm for `RouteTable`, hcm imports h2 for `ServerConn`) is a Go import cycle. **Decision: one-way import — `hcm → h2` only.** The `h2` package defines a small `Dispatcher` interface (`Match(*http.Request) (Action, bool)`) and a small `Action` interface (`WriteH2(StreamWriter) error`); `h2.NewServerConn` takes a `Dispatcher`. The `hcm` package adds a NEW file `internal/filter/hcm/h2dispatch.go` carrying the adapter `h2Dispatcher` (delegates to `*routeTable.match`) plus per-action wrapper types (`h2DirectResponseAdapter`, `h2RouterActionRejection`). The Task 2-introduced `type RouteTable = *routeTable` alias in `route.go` is RETROACTIVELY DELETED at Task 9 (the alias was a planning placeholder; the final shape uses the Dispatcher interface). Task 9's commit removes the alias and adds `h2dispatch.go`. Recorded here, not ADRd.

---

## Phase-04 REVIEW carryover resolution matrix

SPEC §12 + ADR-0053 triage the seven phase-04 Minors. Per the SPEC: **M-1** and **M-3** already landed in the phase-04 follow-up commit `671a059` and the verification-and-review close `1542102` — they are NOT carried forward. The four phase-04 Important findings (I-1..I-4) all landed in `671a059`. M-2/M-4/M-5/M-6/M-7 carry forward to 05.1 per ADR-0045 + SPEC §12. Phase 05.1 lands ZERO Minors as code fixes (the SPEC §12 dispositions are all DEFERRED with rationale). Triage table:

| Phase-04 Minor | Triage | Landing task / rationale |
|---|---|---|
| M-1 (ADR-0043 trailing duplicate line in DECISIONS.md) | RESOLVED-PRIOR | Landed pre-05.1 (in `671a059`'s phase-04 follow-up commit per REVIEW.md `04527eb` recommendation). NOT carried to 05.1. |
| M-2 (ADR-0043's Doctrine `D-3.4, D-3.5` mismatched against informal supersession qualifier) | DEFERRED | Cosmetic. Phase 05.1 does not touch ADR-0043 (HTTPExpectations driver extension); the inconsistency is doctrine-attribution-only. ADR-0053 carries the explicit deferral; a future doctrine-cleanup ADR (likely under the observability or admin-API phases when multiple ADRs are amended together) supersedes ADR-0043 with a corrected doctrine attribution. |
| M-3 (`connection.go:60` `closeAfterAction` dead-branch pre-I-1 fix) | RESOLVED-PRIOR | Resolved by the I-1 landing in `671a059` (the `closeAfterAction` variable is now reachable-true via the wired-up sentinel-error path in `routerAction.do`). NOT carried to 05.1. |
| M-4 (listener-manager `Stop()`/`Listeners()` race on `rt.netLn`) | DEFERRED | Phase 05.1 does not touch `internal/listener/manager.go`'s lock surface (the only listener-manager change in 05.1 is the new `NewManagerWithBaseDirAndAllowH2C` constructor variant + `listenerCtx` plumbing, which is build-time path; runtime Stop/Listeners is unchanged). The race is inherited from phase 03's M-2 carry-forward via phase-04 and remains unresolved. ADR-0053 carries forward; phase 08's admin-api-and-drain phase is the natural close (drain semantics require a correct `Listeners()` snapshot). |
| M-5 (phase-04 SPEC §7 failure-mode prose vs `defer upstreamConn.Close()` mechanism) | DEFERRED + 05.2-WILL-REPEAT | Phase 05.1 does NOT introduce a parallel mechanism on the H2 path because 05.1 does not have the upstream-H2 surface (that's 05.2's `routerActionH2.do` with `defer clientConn.Close()`). The phase-04 H1 prose-vs-mechanism gap remains unchanged in 05.1. ADR-0053 carries forward. The same prose-vs-mechanism shape will reappear in 05.2 (`routerActionH2.do`'s `defer clientConn.Close()` per master SPEC §5.3). ADR-0053 explicitly carries this forward as a "phase-05.2-will-repeat-the-pattern" note so 05.2's brainstorming inherits the disposition rather than re-litigating. Documentation cleanup is bundled into a future SPEC-corrections ADR. |
| M-6 (fixture-0003 driver heredoc YAML pattern) | DEFERRED-AGAIN | Phase 05.1 does NOT introduce a new fixture (no fixture lands in 05.1; fixture 0004 is 05.2's deliverable). The structured-`expectations.yaml` plan from ADR-0019 still belongs to the observability / phase-06 sweep; 05.1 holds the line. ADR-0053 carries forward. |
| M-7 (`Filter.statPrefix` stored but never consumed) | DEFERRED + PHASE-06-MUST-CONSUME-TAG | Phase 05.1 does not consume `Filter.statPrefix` either (no stats subsystem yet). The phase-04 `stat_prefix` storage shape is unchanged. ADR-0053 carries the forward-looking note; phase 06's brainstorm is required to either honour `Filter.statPrefix` (lifting M-7 to RESOLVED) or supersede ADR-0041 with a stat-naming policy that obviates the field. 05.1's `PROGRESS.md` Task 16 entry includes the SPEC-noted "phase-06-must-consume" tag in the carryover-list section. |

Two RESOLVED-PRIOR items (M-1, M-3) confirm phase-04's REVIEW close. Five DEFERRED items (M-2, M-4, M-5, M-6, M-7) carry forward with documented rationale via ADR-0053. No Minor rises to a phase-05.1 blocker.

Additionally, REVIEW.md `04527eb` surfaced (as the "single most important context to surface to the phase-05 planner") that **fixture-0003 still does not differentially exercise upstream TLS** (per ADR-0035 carry-forward). Phase 05 closes this gap *for the H2 leg* via fixture 0004's full-stack HTTPS h2 — but **fixture 0004 lands in 05.2, not 05.1**. ADR-W (which closes ADR-0035's H2 leg) is therefore 05.2's deliverable per ADR-0045. 05.1 records the deferral here so the 05.2 brainstorming inherits the surfacing.

---

## Spec-review advisory responses

The SPEC's brainstorming session (per ADR-0004) ran the `spec-document-reviewer` subagent loop and reached APPROVED on iteration 2. STATE.md commit `3766559` records: "iteration 2 returned Approved with one minor attribution-slip recommendation, also fixed in §5.2 step 4c referencing ADR-0045 instead of ADR-Q." That recommendation was applied **before** the SPEC landed at `4b45941`; the SPEC at `4b45941` carries no outstanding advisory items.

This PLAN therefore has no spec-review advisory items to address. The phase-04 PLAN's `## Spec-review advisory responses` section addressed four items (i–iv) from a non-empty advisory list; the 05.1 SPEC's reviewer-approved-clean state means this section is structurally present but bullet-empty, recorded for symmetry with the phase-04 PLAN shape and to signal the cross-check was performed.

The planner re-verified at PLAN-write time that the SPEC-as-landed at `4b45941` does not contain the iteration-1 contradiction the reviewer flagged (the routerAction-on-H2 enforcement language across SPEC §2.2 / §4.2 / §5.2 step 4c was unified in iteration 2 to "no build-time guard; runtime per-stream INTERNAL_ERROR + RST_STREAM"). The PLAN's File Structure table for `actions.go` and Task 8's stream.go dispatch description both reference SPEC §5.2 step 4c verbatim.

---

## Execution preconditions

Before Task 1, the executing session must:

1. Be running in a **fresh worktree on a phase-implementation branch cut off `master`**, NOT `phase/05.1-downstream-h2-plan` (this plan's authoring branch) and NOT `phase/05.1-downstream-h2-spec` (the SPEC's authoring branch). Recommended: `.worktrees/phase-05.1-downstream-h2-impl` on branch `phase/05.1-downstream-h2-impl`. STATE.md's `last-commit` at cold-start must be the commit that landed this PLAN.md on master. Per ADR-0003: branch fast-forwards into `master` at session exit.
2. Have `docker` available (verify with `docker version`). Required for Task 15's conformance gate (`go test ./test/conformance/h2spec/...`).
3. Have Go 1.23+ installed (verify with `go version`). Native fuzzing (`testing.F`) requires Go 1.18+; 1.23 is the module floor.
4. Have `golangci-lint` installed at the ADR-0009-pinned version v1.64.8 (verify with `golangci-lint version`); install via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8` if missing.
5. `go test ./...` must be green on `master` at cold-start — this plan assumes a clean baseline (phase-04 gate (e) still holds). If not, invoke `superpowers:systematic-debugging` on the regression *before* starting Task 1.
6. `go list -m github.com/envoyproxy/go-control-plane/envoy` resolves to `v1.32.4` (ADR-0013). If a different version is recorded, invoke `superpowers:systematic-debugging` — phase 05.1 must not silently re-pin.
7. `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0045:` (or later if a mid-phase ADR has landed since this PLAN was written). If the tail is `ADR-0045`, the phase-05.1 ADRs are assigned 0046..0053 as in this PLAN. If higher, re-number phase-05.1 ADRs sequentially from `tail + 1` and update every task's ADR reference *before* starting Task 1.
8. The phase-05.1 SPEC at `docs/envoy-go/phases/05.1-downstream-h2/SPEC.md` is at commit `4b45941` (verify with `git log -1 --format=%H -- docs/envoy-go/phases/05.1-downstream-h2/SPEC.md`). If the SPEC has been amended since `4b45941`, invoke `superpowers:systematic-debugging` on the divergence — the PLAN was authored against `4b45941` and silent SPEC drift voids the PLAN's traceability.
9. Phase-04 REVIEW Important findings I-1..I-4 are present in HEAD: `git log --oneline -- internal/filter/hcm/actions.go internal/filter/hcm/connection.go internal/cluster/manager.go` shows commits `671a059` and `1542102` in the history of those files. If any commit is missing, invoke `superpowers:systematic-debugging` on the gap.
10. `go.mod` already declares `golang.org/x/net` (transitively via go-control-plane); Task 4 promotes it to a direct dependency by importing `golang.org/x/net/http2`. Verify the indirect declaration is present at PLAN-write time: `go list -m golang.org/x/net` succeeds (the module is resolvable, even if currently indirect). If the module is missing entirely, the executor adds it via `go get golang.org/x/net@<go-control-plane's-pinned-version>` at Task 4 — same minor version as already pinned by go-control-plane to avoid drift.

If any precondition fails: invoke `superpowers:systematic-debugging` on the missing dependency. Do not improvise an install path or skip a failing test.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

No code change. This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase/05.1-downstream-h2-impl
git log -1 --format=%H                                                # expect: same SHA as docs/envoy-go/STATE.md last-commit field
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: golangci-lint has version 1.64.8
go test ./...                                                         # expect: every package PASS (no FAIL, no compile error)
go list -m github.com/envoyproxy/go-control-plane/envoy               # expect: v1.32.4
grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1                  # expect: ## ADR-0045:
git log -1 --format=%H -- docs/envoy-go/phases/05.1-downstream-h2/SPEC.md
                                                                       # expect: 4b45941 (or the documented SPEC commit; if newer, follow precondition 8 guidance)
git log --oneline -- internal/filter/hcm/actions.go internal/filter/hcm/connection.go internal/cluster/manager.go | head -20
                                                                       # expect: commits 671a059 and 1542102 visible (phase-04 I-1..I-4 fixes)
go list -m golang.org/x/net                                           # expect: a resolvable version (currently indirect via go-control-plane); Task 4 promotes to direct
```

If any line fails, stop and follow the precondition's "if fails" guidance (typically: invoke `superpowers:systematic-debugging` with the specific symptom).

- [ ] **Step 2: Create `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`**

```markdown
# Phase 05.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all preconditions were satisfied at cold-start>

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** <sha — this task's commit>
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions"; phase-04 I-1..I-4 fixes confirmed present in HEAD; SPEC at 4b45941; ADR tail at 0045 (next-free 0046).
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
$ git log -1 --format=%H -- docs/envoy-go/phases/05.1-downstream-h2/SPEC.md
<verbatim>
$ git log --oneline -- internal/filter/hcm/actions.go internal/filter/hcm/connection.go internal/cluster/manager.go | head -20
<verbatim>
```
```

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: PROGRESS.md preamble + precondition verification"
```

After the commit, update the just-written PROGRESS.md entry's `**Commits:**` line with the short SHA of the commit (phase-02/03/04 precedent: a follow-up tiny commit `phase 05.1: PROGRESS SHA-fill for Task 1` lands the SHA).

---

## Task 2: `internal/filter/hcm/h2/` package skeleton — `doc.go` + `errors.go` + tests

**Files:**
- Create: `internal/filter/hcm/h2/doc.go`
- Create: `internal/filter/hcm/h2/errors.go`
- Create: `internal/filter/hcm/h2/errors_test.go`
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md` (append Task 2 entry)

Lands the new sub-package's doc-shell + the H2 error-code enum + `*Error` type. No `route.go` change — the H2 sub-package's typed handle on the route table is introduced via `h2dispatch.go` at Task 9 (per `## Settled SPEC §10 deferred decisions` #10). No SPEC-driven ADR yet — the ADR for the codec source decision (ADR-0046 / ADR-P) lands at Task 4 with the first import of `golang.org/x/net/http2`.

- [ ] **Step 1: Write `internal/filter/hcm/h2/doc.go`**

```go
// Package h2 implements envoy-go's downstream HTTP/2 server-side codec for
// phase 05.1. It drives golang.org/x/net/http2.Framer + golang.org/x/net/http2/hpack
// as low-level codec only (per doctrine D-3.2 and ADR-0046); the connection
// manager (ServerConn), per-stream state machine (serverStream), settings
// handshake, flow control, and error dispatch are all envoy-go-owned.
//
// This package is server-side only in 05.1. ClientConn / RoundTrip live in
// client.go and land in 05.2 per ADR-0045 — there is intentionally no
// client.go file in this package at phase 05.1 close.
//
// Phase 05.1 surface: see docs/envoy-go/phases/05.1-downstream-h2/SPEC.md §4.1.
// Doctrine: see docs/envoy-go/DECISIONS.md ADR-0046 (codec source: x/net/http2.Framer
// + hpack), ADR-0047 (server settings defaults), ADR-0048 (server connection
// manager from scratch), ADR-0050 (ALPN dispatch wiring), ADR-0051 (h2spec
// threshold + pin), ADR-0052 (BEHAVIOR_CONTRACT HTTP/2 subsection).
//
// Error-prefix discipline: every error returned by this package begins with
// "h2: ". Stream-scoped errors include " stream=<id>". The fuzz targets
// FuzzFrameStream and FuzzHPACKDecode grep for "h2:" to validate the
// discipline; do not break it without updating the fuzzers.
//
// What this package does NOT do: it does NOT use http2.Server,
// http2.Server.ServeConn, http2.ConfigureServer, http2.Transport, or
// http2.Transport.NewClientConn. The connection lifecycle is driven explicitly
// by ServerConn.Run. See doctrine D-3.2 and ADR-0048.
package h2
```

- [ ] **Step 2: Write `internal/filter/hcm/h2/errors_test.go` (failing tests first per TDD)**

```go
package h2

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorCodeStrings(t *testing.T) {
	cases := map[ErrCode]string{
		ErrNoError:            "NO_ERROR",
		ErrProtocolError:      "PROTOCOL_ERROR",
		ErrInternalError:      "INTERNAL_ERROR",
		ErrFlowControlError:   "FLOW_CONTROL_ERROR",
		ErrSettingsTimeout:    "SETTINGS_TIMEOUT",
		ErrStreamClosed:       "STREAM_CLOSED",
		ErrFrameSizeError:     "FRAME_SIZE_ERROR",
		ErrRefusedStream:      "REFUSED_STREAM",
		ErrCancel:             "CANCEL",
		ErrCompressionError:   "COMPRESSION_ERROR",
		ErrConnectError:       "CONNECT_ERROR",
		ErrEnhanceYourCalm:    "ENHANCE_YOUR_CALM",
		ErrInadequateSecurity: "INADEQUATE_SECURITY",
		ErrHTTP11Required:     "HTTP_1_1_REQUIRED",
	}
	for code, want := range cases {
		got := code.String()
		if got != want {
			t.Errorf("ErrCode(%d).String() = %q, want %q", uint32(code), got, want)
		}
	}
}

func TestConnError_PrefixAndShape(t *testing.T) {
	e := connError(ErrProtocolError, "bad preface")
	if !strings.HasPrefix(e.Error(), "h2: ") {
		t.Errorf("connError().Error() = %q, want h2:-prefixed", e.Error())
	}
	if got := e.Code; got != ErrProtocolError {
		t.Errorf("Code = %v, want PROTOCOL_ERROR", got)
	}
	if got := e.Stream; got != 0 {
		t.Errorf("Stream = %d, want 0 (conn-scoped)", got)
	}
}

func TestStreamError_PrefixAndShape(t *testing.T) {
	e := streamError(ErrInternalError, 5, "router action on h2")
	if !strings.HasPrefix(e.Error(), "h2: ") {
		t.Errorf("streamError().Error() = %q, want h2:-prefixed", e.Error())
	}
	if !strings.Contains(e.Error(), "stream=5") {
		t.Errorf("streamError().Error() = %q, want substring 'stream=5'", e.Error())
	}
	if got := e.Stream; got != 5 {
		t.Errorf("Stream = %d, want 5", got)
	}
}

func TestError_UnwrapsUnderlying(t *testing.T) {
	inner := errors.New("inner cause")
	e := &Error{Code: ErrInternalError, Underlying: inner}
	if got := errors.Unwrap(e); got != inner {
		t.Errorf("Unwrap = %v, want %v", got, inner)
	}
}
```

- [ ] **Step 3: Run tests; verify they fail with `undefined: ErrCode` etc.**

Run: `go test ./internal/filter/hcm/h2/...`
Expected: `errors_test.go:N: undefined: ErrCode` (or similar — package compiles only after Step 4).

- [ ] **Step 4: Write `internal/filter/hcm/h2/errors.go`**

```go
package h2

import "fmt"

// ErrCode mirrors the RFC 9113 §7 error code numeric assignments. The
// String method returns the RFC 9113 mnemonic ("PROTOCOL_ERROR" etc.) used
// in error messages and (indirectly) in fuzz test assertions.
type ErrCode uint32

const (
	ErrNoError            ErrCode = 0x0
	ErrProtocolError      ErrCode = 0x1
	ErrInternalError      ErrCode = 0x2
	ErrFlowControlError   ErrCode = 0x3
	ErrSettingsTimeout    ErrCode = 0x4
	ErrStreamClosed       ErrCode = 0x5
	ErrFrameSizeError     ErrCode = 0x6
	ErrRefusedStream      ErrCode = 0x7
	ErrCancel             ErrCode = 0x8
	ErrCompressionError   ErrCode = 0x9
	ErrConnectError       ErrCode = 0xa
	ErrEnhanceYourCalm    ErrCode = 0xb
	ErrInadequateSecurity ErrCode = 0xc
	ErrHTTP11Required     ErrCode = 0xd
)

func (c ErrCode) String() string {
	switch c {
	case ErrNoError:
		return "NO_ERROR"
	case ErrProtocolError:
		return "PROTOCOL_ERROR"
	case ErrInternalError:
		return "INTERNAL_ERROR"
	case ErrFlowControlError:
		return "FLOW_CONTROL_ERROR"
	case ErrSettingsTimeout:
		return "SETTINGS_TIMEOUT"
	case ErrStreamClosed:
		return "STREAM_CLOSED"
	case ErrFrameSizeError:
		return "FRAME_SIZE_ERROR"
	case ErrRefusedStream:
		return "REFUSED_STREAM"
	case ErrCancel:
		return "CANCEL"
	case ErrCompressionError:
		return "COMPRESSION_ERROR"
	case ErrConnectError:
		return "CONNECT_ERROR"
	case ErrEnhanceYourCalm:
		return "ENHANCE_YOUR_CALM"
	case ErrInadequateSecurity:
		return "INADEQUATE_SECURITY"
	case ErrHTTP11Required:
		return "HTTP_1_1_REQUIRED"
	default:
		return fmt.Sprintf("UNKNOWN_ERR_CODE(0x%x)", uint32(c))
	}
}

// Error carries an RFC 9113 error code plus optional stream-id (0 means
// connection-scoped) plus an optional underlying error. Error strings start
// with "h2: " — the discriminator the fuzz targets check.
type Error struct {
	Code       ErrCode
	Stream     uint32 // 0 = connection-scoped
	Msg        string
	Underlying error
}

func (e *Error) Error() string {
	var prefix string
	if e.Stream == 0 {
		prefix = fmt.Sprintf("h2: %s", e.Code)
	} else {
		prefix = fmt.Sprintf("h2: %s stream=%d", e.Code, e.Stream)
	}
	if e.Msg != "" {
		prefix += ": " + e.Msg
	}
	if e.Underlying != nil {
		prefix += ": " + e.Underlying.Error()
	}
	return prefix
}

func (e *Error) Unwrap() error { return e.Underlying }

func connError(code ErrCode, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}

func streamError(code ErrCode, stream uint32, msg string) *Error {
	return &Error{Code: code, Stream: stream, Msg: msg}
}
```

- [ ] **Step 5: Run tests; verify they pass**

```bash
go build ./internal/filter/hcm/h2/...
go test ./internal/filter/hcm/h2/...
go vet ./internal/filter/hcm/h2/...
```
Expected: build clean; tests PASS (5 subtests); vet clean.

- [ ] **Step 6: Append a Task 2 PROGRESS entry**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/hcm/h2/doc.go internal/filter/hcm/h2/errors.go \
        internal/filter/hcm/h2/errors_test.go \
        docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: h2 sub-package skeleton + errors enum"
```

After commit, follow-up `phase 05.1: PROGRESS SHA-fill for Task 2`.

---

## Task 3: `internal/filter/hcm/h2/preface.go` + tests

**Files:**
- Create: `internal/filter/hcm/h2/preface.go`
- Create: `internal/filter/hcm/h2/preface_test.go`
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The connection preface check (RFC 9113 §3.4): the server reads exactly 24 bytes from the downstream and compares them against `PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n`. Mismatch is a connection-level PROTOCOL_ERROR. Phase 05.1's ServerConn invokes this as the first read after ALPN dispatch. No ADR (the preface is RFC-mandated, not a decision).

- [ ] **Step 1: Write `internal/filter/hcm/h2/preface_test.go` (failing tests first)**

```go
package h2

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

const goodPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

func TestReadClientPreface_Good(t *testing.T) {
	r := bytes.NewReader([]byte(goodPreface))
	if err := readClientPreface(r); err != nil {
		t.Fatalf("readClientPreface(good) = %v, want nil", err)
	}
}

func TestReadClientPreface_BadByteAtEachPosition(t *testing.T) {
	for i := 0; i < len(goodPreface); i++ {
		buf := []byte(goodPreface)
		buf[i] ^= 0xff
		r := bytes.NewReader(buf)
		err := readClientPreface(r)
		if err == nil {
			t.Errorf("position %d: tampered preface accepted; want error", i)
			continue
		}
		if !strings.HasPrefix(err.Error(), "h2: PROTOCOL_ERROR") {
			t.Errorf("position %d: got %q, want h2:-PROTOCOL_ERROR-prefixed", i, err.Error())
		}
	}
}

func TestReadClientPreface_Truncated(t *testing.T) {
	r := bytes.NewReader([]byte(goodPreface[:10]))
	err := readClientPreface(r)
	if err == nil {
		t.Fatal("truncated preface accepted; want error")
	}
	if !strings.HasPrefix(err.Error(), "h2: PROTOCOL_ERROR") {
		t.Errorf("got %q, want h2:-PROTOCOL_ERROR-prefixed", err.Error())
	}
}

func TestReadClientPreface_EmptyEOF(t *testing.T) {
	r := bytes.NewReader(nil)
	err := readClientPreface(r)
	if err == nil {
		t.Fatal("empty preface accepted; want error")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) && !strings.HasPrefix(err.Error(), "h2: PROTOCOL_ERROR") {
		t.Errorf("got %q, want EOF-wrapped or h2: PROTOCOL_ERROR", err.Error())
	}
}
```

- [ ] **Step 2: Run; verify failure** (`undefined: readClientPreface`).

- [ ] **Step 3: Write `internal/filter/hcm/h2/preface.go`**

```go
package h2

import (
	"io"
)

// clientPrefaceBytes is the 24-byte HTTP/2 connection preface (RFC 9113 §3.4).
// Every h2 connection (cleartext OR after TLS+ALPN) MUST begin with these
// bytes; mismatch is a connection-level PROTOCOL_ERROR.
var clientPrefaceBytes = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")

// readClientPreface reads exactly len(clientPrefaceBytes) bytes from r and
// compares them against the canonical preface. Returns nil on match;
// returns *Error{Code: PROTOCOL_ERROR} on truncation or mismatch.
func readClientPreface(r io.Reader) error {
	buf := make([]byte, len(clientPrefaceBytes))
	if _, err := io.ReadFull(r, buf); err != nil {
		return &Error{
			Code:       ErrProtocolError,
			Msg:        "short preface",
			Underlying: err,
		}
	}
	for i, b := range clientPrefaceBytes {
		if buf[i] != b {
			return &Error{
				Code: ErrProtocolError,
				Msg:  "bad preface bytes",
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run; verify pass**

```bash
go test ./internal/filter/hcm/h2/... -run TestReadClientPreface
```

- [ ] **Step 5: Append Task 3 PROGRESS entry**

- [ ] **Step 6: Commit**

```bash
git add internal/filter/hcm/h2/preface.go internal/filter/hcm/h2/preface_test.go \
        docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: h2 connection preface read + check (RFC 9113 §3.4)"
```

After commit, follow-up SHA-fill commit.

---

## Task 4: `internal/filter/hcm/h2/framer.go` + tests + ADR-0046 (codec source)

**Files:**
- Create: `internal/filter/hcm/h2/framer.go`
- Create: `internal/filter/hcm/h2/framer_test.go`
- Modify: `go.mod` + `go.sum` (promote `golang.org/x/net` from indirect to direct dependency)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0046)
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The thin context-aware wrapper over `http2.Framer`. THIS TASK IS THE FIRST USE of `golang.org/x/net/http2` in envoy-go runtime — ADR-0046 (codec source decision) lands here in the same commit as the import.

- [ ] **Step 1: Promote `golang.org/x/net` to a direct dependency**

```bash
go get golang.org/x/net@<go-control-plane's-pinned-version>
```

The version pinned at PLAN-write time matches whatever go-control-plane v1.32.4 transitively pins (verify with `go list -m -u all | grep golang.org/x/net`). The promotion is a `go.mod` shape change (move the line out of the `// indirect` block); `go.sum` is unchanged. No new module — same SHA already in `go.sum`.

- [ ] **Step 2: Write `internal/filter/hcm/h2/framer_test.go` (failing tests first)**

Use a `net.Pipe()` between two framers. Test list:
- `TestFramer_SettingsRoundTrip`: server writes initial SETTINGS via `WriteSettings`; peer reads them via `ReadFrame`; assert frame type + settings count + the six values match.
- `TestFramer_PingRoundTrip`: server `WritePing`; peer reads PING; peer `WritePing` with ACK flag; server reads PING_ACK.
- `TestFramer_HeadersRoundTrip`: server `WriteHeaders` with hpack-encoded block; peer reads HEADERS frame; bytes match.
- `TestFramer_DataRoundTrip`: server `WriteData(streamID, false, []byte("hello"))`; peer reads DATA; bytes match; `EndStream` flag false; second `WriteData` with `endStream=true`; peer reads DATA with EndStream=true.
- `TestFramer_RSTStreamWindowUpdateGoAway`: emit each frame type; peer reads correctly.
- `TestFramer_ReadFrameCtxCancel`: reader-side ctx cancellation mid-block returns `ctx.Err()` (`context.Canceled` or `context.DeadlineExceeded` depending on cause); the wrapper translates net deadline-exceeded into `ctx.Err()` when ctx is cancelled.

```go
package h2

import (
	"context"
	"net"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestFramer_SettingsRoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	f1 := newFramer(c1)
	f2 := newFramer(c2)

	go func() {
		_ = f1.WriteSettings(
			http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 100},
			http2.Setting{ID: http2.SettingInitialWindowSize, Val: 65535},
		)
	}()

	frame, err := f2.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame = %v, want nil", err)
	}
	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		t.Fatalf("got %T, want *SettingsFrame", frame)
	}
	if sf.NumSettings() != 2 {
		t.Errorf("NumSettings = %d, want 2", sf.NumSettings())
	}
	v, ok := sf.Value(http2.SettingMaxConcurrentStreams)
	if !ok || v != 100 {
		t.Errorf("MaxConcurrentStreams = (%d, %v), want (100, true)", v, ok)
	}
}

func TestFramer_ReadFrameCtxCancel(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	f := newFramer(c1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := f.readFrameCtx(ctx)
	if err == nil {
		t.Fatal("readFrameCtx returned nil; want ctx.Err()")
	}
	if ctx.Err() == nil {
		t.Errorf("ctx.Err() = nil after cancel; want non-nil")
	}
}

// Additional tests (HeadersRoundTrip, DataRoundTrip, etc.) follow the same
// shape — net.Pipe + parallel WriteX/ReadFrame, asserting frame type + payload.
// Full set covers the §4.1 SPEC unit-test enumeration for framer.
```

- [ ] **Step 3: Run; verify failure** (`undefined: newFramer`).

- [ ] **Step 4: Write `internal/filter/hcm/h2/framer.go`**

```go
package h2

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	"golang.org/x/net/http2"
)

// framer is a thin wrapper over *http2.Framer adding context-aware reads.
// Write methods are passthrough via embedding. Phase 05.1 does NOT use
// http2.Framer.WriteRawFrame (per SPEC §4.1).
type framer struct {
	*http2.Framer
	conn net.Conn
}

// newFramer constructs a framer over conn for both reading and writing. The
// returned value embeds *http2.Framer so callers can use WriteSettings,
// WriteHeaders, WriteData, WriteRSTStream, WriteWindowUpdate, WritePing,
// WriteGoAway, and ReadFrame directly.
func newFramer(conn net.Conn) *framer {
	return &framer{
		Framer: http2.NewFramer(conn, conn),
		conn:   conn,
	}
}

// readFrameCtx reads one frame, honouring ctx cancellation by setting a read
// deadline on the underlying conn. http2.Framer.ReadFrame is otherwise blocking
// and not ctx-aware; this method bridges the two. On ctx cancel mid-read,
// returns ctx.Err() (context.Canceled or context.DeadlineExceeded).
func (f *framer) readFrameCtx(ctx context.Context) (http2.Frame, error) {
	if dl, ok := ctx.Deadline(); ok {
		_ = f.conn.SetReadDeadline(dl)
	} else {
		// Short-poll: 50ms slices so ctx cancellation is observed within
		// bounded latency. The slice is small enough to be a noisy non-issue
		// in practice.
		_ = f.conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	}
	for {
		frame, err := f.ReadFrame()
		if err == nil {
			_ = f.conn.SetReadDeadline(time.Time{})
			return frame, nil
		}
		// Translate timeout-on-deadline into ctx.Err() when ctx is done.
		var nerr net.Error
		if errors.As(err, &nerr) && nerr.Timeout() {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			// No ctx deadline → re-arm and re-loop.
			if _, hasDL := ctx.Deadline(); !hasDL {
				_ = f.conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
				continue
			}
			return nil, ctxErr(ctx, err)
		}
		// Non-timeout error: pass through. Also pass through os.ErrDeadlineExceeded
		// where ctx has a deadline (the caller imposed it; ctx.Err returns
		// DeadlineExceeded so the wrap is faithful).
		_ = f.conn.SetReadDeadline(time.Time{})
		if errors.Is(err, os.ErrDeadlineExceeded) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
		}
		return nil, err
	}
}

func ctxErr(ctx context.Context, fallback error) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	return fallback
}
```

- [ ] **Step 5: Append ADR-0046 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0046: HTTP/2 codec source — `golang.org/x/net/http2.Framer` + `golang.org/x/net/http2/hpack`

**Status:** Accepted
**Date:** <session date>
**Doctrine:** D-3.2, D-3.5
**Settles:** SPEC ADR-P, phase-05.1 §4.1 codec sub-package.

### Context

Phase 05.1 introduces envoy-go's downstream HTTP/2 codec. `BOOTSTRAP_PROMPT.md` D-3.2 permits `golang.org/x/net/http2` "as a low-level codec only — never as a server runtime." The phase-05.1 SPEC §4.1 names two specific entry points within that package: `http2.Framer` (frame byte-layout serialisation) and `http2/hpack` (HPACK encoder/decoder with dynamic-table state). The decision to use them — vs handcrafting from RFC 9113 — needs explicit codification because the HPACK dynamic-table state machine has CVE history that argues against re-implementation.

### Decision

Phase 05.1's `internal/filter/hcm/h2/` codec sub-package consumes:

- `http2.Framer` for frame read/write — wrapped by `framer` in `framer.go` to add context-aware reads via `conn.SetReadDeadline` translation.
- `http2/hpack.Encoder` + `hpack.Decoder` for header block encode/decode — held per-connection in `hpackState` (hpack.go) so the dynamic-table state is per-conn.

Three runtime constructs in `golang.org/x/net/http2` are FORBIDDEN even at phase 05.1: `http2.Server`, `http2.Server.ServeConn`, `http2.ConfigureServer`, `http2.Transport`, `http2.Transport.NewClientConn`. They carry their own request-routing, header-canonicalization, response-header injection, and error policies that diverge from Envoy's; envoy-go's connection lifecycle (preface check, settings handshake, frame dispatch, GOAWAY emission, RST_STREAM/PING semantics) is owned by `ServerConn` (conn.go) and `serverStream` (stream.go) — both written from scratch. ADR-0048 codifies the from-scratch-server-connection-manager decision.

Driver-side test use of `x/net/http2.Transport` (in `cmd/envoy-go/main_test.go` H2 smoke variant and `internal/filter/hcm/h2/conn_test.go` end-to-end tests) is permitted because that is fixture infrastructure, not envoy-go runtime — D-3.2 governs runtime, not test code.

### Consequences

- The boundary is grep-verifiable: `! grep -nR '"golang.org/x/net/http2"' internal/ cmd/envoy-go/main.go` (excluding `_test.go`) returns zero hits OUTSIDE `internal/filter/hcm/h2/framer.go`/`hpack.go`/`settings.go` — the three files that legitimately import the package. Task 16's gate-sweep verifies this.
- `golang.org/x/net` is promoted from an indirect dependency (transitively held via go-control-plane) to a direct dependency. No new module SHA — the same version go-control-plane already pins.
- A future codec-related ADR (e.g., a phase-09 HTTP/3 ADR for `quic-go`) follows this same shape: low-level codec only, with the runtime owned by envoy-go. ADR-0046 is the template for that pattern.
- The three FORBIDDEN runtime types do not carry through to test code; tests may use `http2.Transport` as a peer driver. The boundary is in the package import graph (test files vs production files) and in CI lint rules (the grep gate above).

This ADR supersedes nothing.
```

- [ ] **Step 6: Run all tests; verify pass**

```bash
go build ./...
go test ./internal/filter/hcm/h2/... -run TestFramer
go vet ./...
```
Expected: build clean; framer tests PASS; vet clean.

- [ ] **Step 7: Append Task 4 PROGRESS entry**

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/filter/hcm/h2/framer.go internal/filter/hcm/h2/framer_test.go \
        docs/envoy-go/DECISIONS.md docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: h2 framer (ctx-aware http2.Framer wrapper) [ADR-0046]"
```

After commit, follow-up SHA-fill commit.

---

## Task 5: `internal/filter/hcm/h2/hpack.go` + tests

**Files:**
- Create: `internal/filter/hcm/h2/hpack.go`
- Create: `internal/filter/hcm/h2/hpack_test.go`
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The per-connection HPACK encoder/decoder integration. ADR-0046 (Task 4) covers the codec-source decision; no new ADR here.

- [ ] **Step 1: Write `internal/filter/hcm/h2/hpack_test.go` (failing tests first)**

```go
package h2

import (
	"strings"
	"testing"

	"golang.org/x/net/http2/hpack"
)

func TestHPACK_EncodeDecodeRoundTrip(t *testing.T) {
	st := newHPACKState(4096)
	headers := []hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
		{Name: "content-length", Value: "3"},
		{Name: "server", Value: "envoy"},
		{Name: "date", Value: "Sun, 06 Apr 2025 12:00:00 GMT"},
	}
	encoded := st.encodeHeaders(headers)
	decoded, err := st.decodeBlock(encoded, true)
	if err != nil {
		t.Fatalf("decodeBlock = %v, want nil", err)
	}
	if len(decoded) != len(headers) {
		t.Fatalf("decoded len = %d, want %d", len(decoded), len(headers))
	}
	for i, h := range headers {
		if decoded[i].Name != h.Name || decoded[i].Value != h.Value {
			t.Errorf("decoded[%d] = %+v, want %+v", i, decoded[i], h)
		}
	}
}

func TestHPACK_AdversarialDecode_NoPanicReturnsCompressionError(t *testing.T) {
	st := newHPACKState(4096)
	bad := []byte{0xff, 0xff, 0xff, 0xff, 0xff}
	_, err := st.decodeBlock(bad, true)
	if err == nil {
		t.Fatal("decodeBlock(adversarial) returned nil; want COMPRESSION_ERROR")
	}
	if !strings.HasPrefix(err.Error(), "h2: COMPRESSION_ERROR") {
		t.Errorf("got %q, want h2: COMPRESSION_ERROR-prefixed", err.Error())
	}
}

func TestHPACK_UpdateMaxTableSize_PropagatesToEncoder(t *testing.T) {
	st := newHPACKState(4096)
	headers := []hpack.HeaderField{{Name: "x-custom", Value: strings.Repeat("a", 1000)}}
	out1 := st.encodeHeaders(headers)
	st.updateMaxTableSize(64)
	out2 := st.encodeHeaders(headers)
	// Encoder forced to literal (no dynamic-table indexing) when shrunk:
	// the second output should not be smaller than the first by more than
	// the indexed-prefix overhead. Just assert round-trip still works.
	dec, err := st.decodeBlock(out2, true)
	if err != nil {
		t.Fatalf("decodeBlock(post-shrink) = %v, want nil", err)
	}
	if len(dec) != 1 || dec[0].Value != headers[0].Value {
		t.Errorf("round-trip post-shrink failed; got %+v", dec)
	}
	_ = out1
}
```

- [ ] **Step 2: Run; verify failure** (`undefined: newHPACKState`).

- [ ] **Step 3: Write `internal/filter/hcm/h2/hpack.go`**

```go
package h2

import (
	"bytes"

	"golang.org/x/net/http2/hpack"
)

// hpackState carries per-connection HPACK encoder + decoder state. Phase 05.1
// uses it as a single-threaded helper (one per ServerConn); no internal mutex.
// Per ADR-0046, hpack.Encoder and hpack.Decoder are the low-level codec
// surfaces; envoy-go owns the table-size update propagation across SETTINGS.
type hpackState struct {
	enc    *hpack.Encoder
	encBuf bytes.Buffer
	dec    *hpack.Decoder
	fields []hpack.HeaderField
}

// newHPACKState constructs encoder + decoder both initialized with maxTableSize
// (defaults to 4096 per ADR-0047). The decoder's emit-callback appends into
// the fields slice; decodeBlock resets and returns it on each call.
func newHPACKState(maxTableSize uint32) *hpackState {
	st := &hpackState{}
	st.enc = hpack.NewEncoder(&st.encBuf)
	st.enc.SetMaxDynamicTableSize(maxTableSize)
	st.dec = hpack.NewDecoder(maxTableSize, func(f hpack.HeaderField) {
		st.fields = append(st.fields, f)
	})
	return st
}

// encodeHeaders writes each header through the encoder, returning the encoded
// byte block. The returned bytes alias the internal buffer; callers must copy
// before storing across encode calls.
func (s *hpackState) encodeHeaders(headers []hpack.HeaderField) []byte {
	s.encBuf.Reset()
	for _, h := range headers {
		_ = s.enc.WriteField(h)
	}
	return s.encBuf.Bytes()
}

// decodeBlock feeds block into the decoder. If endBlock is true, it calls
// dec.Close() (signaling end of header block boundary). Returns the decoded
// fields (a fresh slice) or *Error{Code: COMPRESSION_ERROR} on adversarial
// input.
func (s *hpackState) decodeBlock(block []byte, endBlock bool) ([]hpack.HeaderField, error) {
	s.fields = s.fields[:0]
	if _, err := s.dec.Write(block); err != nil {
		return nil, &Error{Code: ErrCompressionError, Msg: "hpack decode", Underlying: err}
	}
	if endBlock {
		if err := s.dec.Close(); err != nil {
			return nil, &Error{Code: ErrCompressionError, Msg: "hpack close", Underlying: err}
		}
	}
	out := make([]hpack.HeaderField, len(s.fields))
	copy(out, s.fields)
	return out, nil
}

// updateMaxTableSize propagates a peer-announced SETTINGS_HEADER_TABLE_SIZE
// change to our encoder so subsequent outgoing HEADERS frames respect the
// peer's new limit. The decoder side is bounded by our advertised value
// (newHPACKState's argument); we do NOT re-advertise after handshake in 05.1.
func (s *hpackState) updateMaxTableSize(size uint32) {
	s.enc.SetMaxDynamicTableSize(size)
}
```

- [ ] **Step 4: Run; verify pass**

```bash
go test ./internal/filter/hcm/h2/... -run TestHPACK
```

- [ ] **Step 5: Append Task 5 PROGRESS entry**

- [ ] **Step 6: Commit**

```bash
git add internal/filter/hcm/h2/hpack.go internal/filter/hcm/h2/hpack_test.go \
        docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: h2 hpack state (encoder + decoder + table-size propagation)"
```

---

## Task 6: `internal/filter/hcm/h2/flow.go` + tests

**Files:**
- Create: `internal/filter/hcm/h2/flow.go`
- Create: `internal/filter/hcm/h2/flow_test.go`
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

Connection-level + per-stream flow-control window helpers. Implementation: a small `window` struct using a mutex-guarded counter + a signal channel for blocking reservers. SPEC §11.5 mitigation (tiny-window stress test) lives in `flow_test.go`. No ADR (implementation detail).

- [ ] **Step 1: Write `internal/filter/hcm/h2/flow_test.go` (failing tests first)**

```go
package h2

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestWindow_ReserveAndReplenish(t *testing.T) {
	w := newWindow(1000)
	got, err := w.reserve(100)
	if err != nil || got != 100 {
		t.Fatalf("reserve(100) = (%d, %v), want (100, nil)", got, err)
	}
	if w.available() != 900 {
		t.Errorf("available = %d, want 900", w.available())
	}
	w.replenish(100)
	if w.available() != 1000 {
		t.Errorf("after replenish, available = %d, want 1000", w.available())
	}
}

func TestWindow_BlockingWaitFor(t *testing.T) {
	w := newWindow(0)
	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_ = w.waitFor(ctx, 50)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	w.replenish(100)
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("waitFor did not return after replenish")
	}
}

func TestWindow_CtxCancelDuringWait(t *testing.T) {
	w := newWindow(0)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := w.waitFor(ctx, 50)
	if err == nil {
		t.Fatal("waitFor returned nil; want ctx.Err()")
	}
	if ctx.Err() == nil {
		t.Errorf("ctx.Err() = nil; want non-nil")
	}
}

func TestWindow_TinyWindowStressDelivery(t *testing.T) {
	// SPEC §11.5 mitigation: INITIAL_WINDOW_SIZE = 1, send 100 bytes in
	// 1-byte chunks via WINDOW_UPDATE-driven progress. Eventual full delivery.
	w := newWindow(1)
	delivered := 0
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		for i := 0; i < 100; i++ {
			if err := w.waitFor(ctx, 1); err != nil {
				t.Errorf("waitFor at i=%d: %v", i, err)
				close(done)
				return
			}
			_, _ = w.reserve(1)
			mu.Lock()
			delivered++
			mu.Unlock()
		}
		close(done)
	}()
	for i := 0; i < 99; i++ {
		time.Sleep(time.Millisecond)
		w.replenish(1)
	}
	<-done
	mu.Lock()
	defer mu.Unlock()
	if delivered != 100 {
		t.Errorf("delivered = %d, want 100", delivered)
	}
}
```

- [ ] **Step 2: Run; verify failure** (`undefined: newWindow`).

- [ ] **Step 3: Write `internal/filter/hcm/h2/flow.go`**

```go
package h2

import (
	"context"
	"sync"
)

// window models one HTTP/2 flow-control window — either connection-level
// (one per ServerConn) or per-stream (one per serverStream, send and recv
// sides separately). Implementation: a mutex-guarded int32 counter plus a
// signal channel that replenish notifies for blocking reservers.
type window struct {
	mu sync.Mutex
	n  int32
	ch chan struct{}
}

func newWindow(initial int32) *window {
	return &window{n: initial, ch: make(chan struct{}, 1)}
}

// available reports the current window size. Used in tests.
func (w *window) available() int32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.n
}

// reserve atomically decrements up to n bytes, returning the actually-taken
// amount (which may be less than n if the window has fewer bytes available,
// or 0 if empty). Non-blocking. Callers that need >= n bytes call waitFor first.
func (w *window) reserve(n int32) (int32, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.n <= 0 {
		return 0, nil
	}
	taken := n
	if w.n < n {
		taken = w.n
	}
	w.n -= taken
	return taken, nil
}

// replenish increments the window and signals any blocking waitFor.
func (w *window) replenish(delta int32) {
	w.mu.Lock()
	w.n += delta
	w.mu.Unlock()
	select {
	case w.ch <- struct{}{}:
	default:
	}
}

// waitFor blocks until the window has at least n bytes available or ctx
// cancels. Returns nil on success, ctx.Err() on cancel.
func (w *window) waitFor(ctx context.Context, n int32) error {
	for {
		w.mu.Lock()
		if w.n >= n {
			w.mu.Unlock()
			return nil
		}
		w.mu.Unlock()
		select {
		case <-w.ch:
			// loop and re-check
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
```

- [ ] **Step 4: Run; verify pass**

```bash
go test ./internal/filter/hcm/h2/... -run TestWindow
```

- [ ] **Step 5: Append Task 6 PROGRESS entry**

- [ ] **Step 6: Commit**

```bash
git add internal/filter/hcm/h2/flow.go internal/filter/hcm/h2/flow_test.go \
        docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: h2 flow-control window helpers (conn + per-stream)"
```

---

## Task 7: `internal/filter/hcm/h2/settings.go` + tests + ADR-0047 (server settings defaults)

**Files:**
- Create: `internal/filter/hcm/h2/settings.go`
- Create: `internal/filter/hcm/h2/settings_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0047)
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The SETTINGS handshake helpers + `ServerSettings` / `clientSettings` value types. ADR-0047 lands here in the same commit (first use of `defaultServerSettings`).

- [ ] **Step 1: Write `internal/filter/hcm/h2/settings_test.go` (failing tests first)**

```go
package h2

import (
	"net"
	"testing"

	"golang.org/x/net/http2"
)

func TestServerSettings_DefaultsMatchADR0047(t *testing.T) {
	s := DefaultServerSettings
	if s.MaxConcurrentStreams != 100 {
		t.Errorf("MaxConcurrentStreams = %d, want 100", s.MaxConcurrentStreams)
	}
	if s.InitialWindowSize != 65535 {
		t.Errorf("InitialWindowSize = %d, want 65535", s.InitialWindowSize)
	}
	if s.MaxFrameSize != 16384 {
		t.Errorf("MaxFrameSize = %d, want 16384", s.MaxFrameSize)
	}
	if s.EnablePush != 0 {
		t.Errorf("EnablePush = %d, want 0", s.EnablePush)
	}
	if s.HeaderTableSize != 4096 {
		t.Errorf("HeaderTableSize = %d, want 4096", s.HeaderTableSize)
	}
}

func TestSettings_RoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	srvF := newFramer(c1)
	cliF := newFramer(c2)

	go func() {
		_ = writeServerInitialSettings(srvF, DefaultServerSettings)
	}()
	frame, err := cliF.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame = %v, want nil", err)
	}
	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		t.Fatalf("got %T, want *SettingsFrame", frame)
	}
	v, _ := sf.Value(http2.SettingMaxConcurrentStreams)
	if v != 100 {
		t.Errorf("peer-side MaxConcurrentStreams = %d, want 100", v)
	}
}

func TestReadClientSettings_AckOnFirstReadIsProtocolError(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	srvF := newFramer(c1)
	cliF := newFramer(c2)
	go func() {
		_ = cliF.WriteSettingsAck()
	}()
	var cs clientSettings
	err := readClientSettings(srvF, &cs)
	if err == nil {
		t.Fatal("readClientSettings(ACK first) returned nil; want PROTOCOL_ERROR")
	}
}
```

- [ ] **Step 2: Run; verify failure**.

- [ ] **Step 3: Write `internal/filter/hcm/h2/settings.go`**

```go
package h2

import (
	"golang.org/x/net/http2"
)

// ServerSettings is the value-typed bundle of phase-05.1 server-side SETTINGS
// values. Per ADR-0047 the values are hardcoded; the struct exists so future
// phases can vary per-listener if a use case demands.
type ServerSettings struct {
	MaxConcurrentStreams uint32
	InitialWindowSize    uint32
	MaxFrameSize         uint32
	EnablePush           uint32 // 0 = disabled (phase 05.1 always)
	NoRFC7540Priorities  uint32 // 1 = announce client we discard PRIORITY
	HeaderTableSize      uint32
}

// DefaultServerSettings is the phase-05.1 hardcoded set per ADR-0047.
var DefaultServerSettings = ServerSettings{
	MaxConcurrentStreams: 100,
	InitialWindowSize:    65535,
	MaxFrameSize:         16384,
	EnablePush:           0,
	NoRFC7540Priorities:  1,
	HeaderTableSize:      4096,
}

// clientSettings holds the values the client announces in its initial SETTINGS
// frame (or in subsequent SETTINGS updates). Only the fields phase 05.1 reads
// are present.
type clientSettings struct {
	MaxConcurrentStreams uint32
	InitialWindowSize    uint32
	MaxFrameSize         uint32
	HeaderTableSize      uint32
	EnablePush           uint32
}

// writeServerInitialSettings writes a single SETTINGS frame (no ACK flag) to
// the peer carrying the six configured values. The peer is expected to send
// SETTINGS_ACK in response (the ServerConn frame loop reads it).
func writeServerInitialSettings(fr *framer, s ServerSettings) error {
	return fr.WriteSettings(
		http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: s.MaxConcurrentStreams},
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: s.InitialWindowSize},
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: s.MaxFrameSize},
		http2.Setting{ID: http2.SettingEnablePush, Val: s.EnablePush},
		http2.Setting{ID: http2.SettingHeaderTableSize, Val: s.HeaderTableSize},
		// SETTINGS_NO_RFC7540_PRIORITIES is RFC 9218; x/net/http2 may not have
		// a constant for it. Use the numeric ID 0x9 directly.
		http2.Setting{ID: http2.SettingID(0x9), Val: s.NoRFC7540Priorities},
	)
}

// readClientSettings reads one SETTINGS frame from fr and applies its values
// to applyTo. Returns *Error{Code: PROTOCOL_ERROR} if the first frame is a
// SETTINGS_ACK (RFC 9113 §6.5: server must read client's initial SETTINGS
// before reading the ACK to its own).
func readClientSettings(fr *framer, applyTo *clientSettings) error {
	frame, err := fr.ReadFrame()
	if err != nil {
		return &Error{Code: ErrProtocolError, Msg: "read client SETTINGS", Underlying: err}
	}
	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		return &Error{Code: ErrProtocolError, Msg: "expected SETTINGS frame, got " + frame.Header().Type.String()}
	}
	if sf.IsAck() {
		return &Error{Code: ErrProtocolError, Msg: "ACK on first client SETTINGS"}
	}
	_ = sf.ForeachSetting(func(s http2.Setting) error {
		switch s.ID {
		case http2.SettingMaxConcurrentStreams:
			applyTo.MaxConcurrentStreams = s.Val
		case http2.SettingInitialWindowSize:
			applyTo.InitialWindowSize = s.Val
		case http2.SettingMaxFrameSize:
			applyTo.MaxFrameSize = s.Val
		case http2.SettingHeaderTableSize:
			applyTo.HeaderTableSize = s.Val
		case http2.SettingEnablePush:
			applyTo.EnablePush = s.Val
		}
		// Unknown settings are silently ignored per RFC 9113 §6.5.2.
		return nil
	})
	return nil
}
```

- [ ] **Step 4: Append ADR-0047 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0047: Phase-05.1 H2 server settings defaults

**Status:** Accepted
**Date:** <session date>
**Doctrine:** D-3.5
**Settles:** SPEC ADR-S; phase-05.1 §4.1 / §5 / §11 (settings handshake).
**Amends:** ADR-0041 (HCM silent-ignore set extended with `http2_protocol_options`).

### Context

Phase 05.1 needs concrete numeric values for every SETTINGS the server announces in its initial SETTINGS frame. RFC 9113 §6.5.2 defines defaults for the standard settings; envoy-go matches Envoy's documented `Http2ProtocolOptions` defaults where they diverge from RFC defaults, and matches RFC defaults where Envoy doesn't override.

### Decision

Phase 05.1 hardcodes the following ServerSettings (in `internal/filter/hcm/h2/settings.go`):

- **MAX_CONCURRENT_STREAMS = 100.** Envoy's documented default for `max_concurrent_streams`. The 101st concurrent stream from the client → REFUSED_STREAM (RFC 9113 §5.1.2).
- **INITIAL_WINDOW_SIZE = 65535.** RFC 9113 §6.9.2 protocol default. Envoy does not override.
- **MAX_FRAME_SIZE = 16384.** RFC 9113 §6.5.2 protocol default. Envoy does not override.
- **ENABLE_PUSH = 0.** Phase 05.1 disables server push entirely (SPEC §2.1). Disabling on our SETTINGS prevents the client from sending PUSH_PROMISE either.
- **SETTINGS_NO_RFC7540_PRIORITIES = 1.** Informs the client we discard PRIORITY frames (RFC 9113 §6.3 / SPEC §2.1). RFC 9218 (`SETTINGS_NO_RFC7540_PRIORITIES`) numeric ID 0x9.
- **HEADER_TABLE_SIZE = 4096.** RFC 9113 §6.5.2 protocol default + Envoy default. We do not advertise a different value; the encoder side at the peer is also 4096 unless the peer changes it via its own SETTINGS.

### HCM `http2_protocol_options` silent-ignore amendment

ADR-0041 (phase-04 HCM silent-ignore set) is amended to add the directly-on-HCM `http2_protocol_options` field. Phase 05.1 reads this field via the unmarshalled HCM proto but does NOT honour any sub-field — the values stay at the ADR-0047 defaults regardless of what the bootstrap declares. Future phases (06+) may move members from "ignored" to "honoured" via a superseding ADR. The cluster-side `HttpProtocolOptions` typed-extension is 05.2's surface and remains in the phase-04 silent-ignore set in 05.1.

### Consequences

- Differential equivalence: the gate does not assert SETTINGS values byte-for-byte (those are inside the structurally-equivalent framing rule per ADR-0052). h2spec section 6.5 only validates RFC 9113 compliance, not Envoy-specific values — the threshold accepts the values above.
- The `ServerSettings` value type is exported so future phases (or test fixtures) can vary the values per construction; the `DefaultServerSettings` global is the project-wide canonical instance.
- A future phase that needs configurable per-listener SETTINGS (e.g., to honour `http2_protocol_options.max_concurrent_streams`) supersedes ADR-0047 + ADR-0041's silent-ignore amendment with a new ADR.

This ADR supersedes nothing on its own; ADR-0041 is amended (not superseded) per the additive shape of the silent-ignore set.
```

- [ ] **Step 5: Run; verify pass**

```bash
go test ./internal/filter/hcm/h2/...
```

- [ ] **Step 6: Append Task 7 PROGRESS entry; commit**

```bash
git add internal/filter/hcm/h2/settings.go internal/filter/hcm/h2/settings_test.go \
        docs/envoy-go/DECISIONS.md docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: h2 settings handshake + DefaultServerSettings [ADR-0047]"
```

---

## Task 8: `internal/filter/hcm/h2/stream.go` + tests + ADR-0048 (server connection manager from scratch)

**Files:**
- Create: `internal/filter/hcm/h2/stream.go`
- Create: `internal/filter/hcm/h2/stream_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0048)
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The per-stream server-side state machine — the largest single piece of stateful code in 05.1. ADR-0048 (server connection manager / serverStream from scratch) lands here in the same commit (first use of the per-stream state machine).

**This task is a likely candidate for the §6.1 secondary trigger** (sub-steps blow up past 10–15 once contact with reality reveals complexity). If the executor finds the test set growing past 15 sub-test cases or the production implementation past ~300 LoC, invoke `superpowers:systematic-debugging` and consider splitting per `## Scope check` reason 3.

The task structure below is at the budgeted ~250 LoC + ~400 LoC tests, distributed across logical sub-steps. The executor can split sub-steps into separate commits if useful.

- [ ] **Step 1: Write `internal/filter/hcm/h2/stream_test.go` (failing tests first — full set)**

Test cases (each a separate `func Test...`):
- `TestServerStream_StateTransitions_HeadersOnlyEndStream`: HEADERS-with-END_STREAM → idle → halfClosedRemote (request body empty); after dispatch + writeH2 response → closed.
- `TestServerStream_StateTransitions_HeadersThenData`: HEADERS-no-END_STREAM (open), DATA(chunk1, false), DATA(chunk2, true) → halfClosedRemote at last DATA; dispatch reads body via reqBodyR; → closed.
- `TestServerStream_StateTransitions_RSTStream`: any state → recvRSTStream(CANCEL) → closed; reqBodyW closed with error.
- `TestServerStream_RecvWindowUpdate_ReplenishesSendWindow`.
- `TestServerStream_Dispatch_DirectResponse_WritesHeadersAndData`.
- `TestServerStream_Dispatch_RouterAction_EmitsRSTStreamInternalError`: a `routerAction` matched on H2 → recvRSTStream(INTERNAL_ERROR) emitted (per SPEC §5.2 step 4c).
- `TestServerStream_Dispatch_NoMatch_Returns404DirectResponse`.
- `TestServerStream_RejectsEvenClientStreamID`: ServerConn calls recvHeaders with even-numbered stream ID → returns connError(PROTOCOL_ERROR).
- `TestServerStream_RejectsStreamIDReuse`: same stream ID twice → connError(PROTOCOL_ERROR).

Each test uses a captured-output fake streamWriter to validate the wire effects without driving a real `net.Pipe`.

```go
package h2

// (full test bodies omitted from this PLAN excerpt; each follows the
// recvHeaders/recvData/dispatch sequence with assertions against a fake
// streamWriter that records WriteHeaders + WriteData calls in a slice).
```

- [ ] **Step 2: Run; verify failure** (`undefined: serverStream`).

- [ ] **Step 3: Write `internal/filter/hcm/h2/stream.go`**

The full implementation (per the SPEC §5.2 + SPEC §4.1 stream.go bullet) carries:
- The `streamState` enum + transitions per RFC 9113 §5.1.
- `serverStream` struct with the fields enumerated in the File Structure entry.
- `recvHeaders` / `recvData` / `recvRSTStream` / `recvWindowUpdate` methods + state transitions.
- `dispatch(ctx, table)` that builds the `*http.Request` from pseudo-headers, calls `table.match`, invokes the action, and writes the response.
- The `streamWriter` interface (consumed by `actions.go`'s `directResponseAction.writeH2`).
- Server-side stream-id validation: even IDs → PROTOCOL_ERROR; reuse → PROTOCOL_ERROR.

```go
package h2

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"golang.org/x/net/http2/hpack"
)

type streamState int

const (
	streamIdle streamState = iota
	streamOpen
	streamHalfClosedRemote
	streamHalfClosedLocal
	streamClosed
)

// streamWriter is the interface directResponseAction.writeH2 (in
// internal/filter/hcm/actions.go) calls to emit response frames. The
// concrete implementation is *serverStream itself.
type streamWriter interface {
	WriteHeaders(headers []hpack.HeaderField, endStream bool) error
	WriteData(b []byte, endStream bool) error
}

// directResponseWriter is a small contract bridging from the H2 dispatch
// helper to actions.go's writeH2(streamWriter). Defined here (and re-defined
// in actions.go via the interface) so actions.go does not need to import h2.
type directResponseWriter interface {
	streamWriter
}

// hcmAction is the lift of internal/filter/hcm.routeAction's H2-relevant
// methods. We define a minimal interface here so stream.go does not have a
// hard dependency on the full hcm.routeAction interface (avoids import cycles
// since hcm imports h2 to reference streamWriter, but h2 imports hcm to
// reference RouteTable). The h2 sub-package consumes it via type assertion
// on the action returned from RouteTable.match.
type hcmAction interface{}

// directResponseLike is the structural shape of a direct_response action:
// it has body() returning the synthesized reply and writeH2() that consumes
// a streamWriter. Phase-04's *directResponseAction satisfies this implicitly;
// the H2 dispatch helper type-asserts and calls writeH2.
//
// In practice, the dispatch helper imports the hcm package and references
// hcm-specific types directly; this file's concrete approach is a planner
// choice. The cleanest layering: stream.go's dispatch takes a small
// "dispatcher" interface that hides the hcm type, and actions.go's
// directResponseAction implements the dispatch contract. The concrete
// import shape is the executor's choice; the unit test asserts the
// behaviour, not the import topology.

// serverStream is one HTTP/2 server-side stream. Methods are NOT goroutine-safe;
// callers serialize via the ServerConn's frame loop (recvX) plus the stream's
// own dispatch goroutine (dispatch + WriteHeaders/WriteData). The mu mutex
// guards state transitions only.
type serverStream struct {
	id    uint32
	mu    sync.Mutex
	state streamState

	sendW *window
	recvW *window

	reqHeaders []hpack.HeaderField
	reqBodyR   *io.PipeReader
	reqBodyW   *io.PipeWriter

	// conn is a small interface exposing the parent ServerConn's framer + hpack
	// state (so stream.WriteHeaders / WriteData can encode + write).
	conn streamConn
}

// streamConn is the minimum surface stream needs from ServerConn.
type streamConn interface {
	encodeAndWriteHeaders(streamID uint32, headers []hpack.HeaderField, endStream bool) error
	writeData(streamID uint32, b []byte, endStream bool) error
	writeRSTStream(streamID uint32, code ErrCode) error
}

func newServerStream(id uint32, conn streamConn, initialSendWindow, initialRecvWindow int32) *serverStream {
	pr, pw := io.Pipe()
	return &serverStream{
		id:       id,
		state:    streamIdle,
		sendW:    newWindow(initialSendWindow),
		recvW:    newWindow(initialRecvWindow),
		reqBodyR: pr,
		reqBodyW: pw,
		conn:     conn,
	}
}

// (recvHeaders, recvData, recvRSTStream, recvWindowUpdate, dispatch,
// WriteHeaders, WriteData implementations follow.)

func (s *serverStream) WriteHeaders(headers []hpack.HeaderField, endStream bool) error {
	if err := s.conn.encodeAndWriteHeaders(s.id, headers, endStream); err != nil {
		return err
	}
	if endStream {
		s.transition(streamClosed)
	}
	return nil
}

func (s *serverStream) WriteData(b []byte, endStream bool) error {
	if err := s.conn.writeData(s.id, b, endStream); err != nil {
		return err
	}
	if endStream {
		s.transition(streamClosed)
	}
	return nil
}

func (s *serverStream) transition(to streamState) {
	s.mu.Lock()
	s.state = to
	s.mu.Unlock()
}

// buildRequest constructs an *http.Request from the decoded pseudo-headers
// + regular headers + the request body pipe reader. SPEC §10 #3 settled to
// stdlib *http.Request reuse on the server side.
func buildRequest(headers []hpack.HeaderField, body io.Reader) (*http.Request, error) {
	var method, path, scheme, authority string
	regular := http.Header{}
	for _, h := range headers {
		switch h.Name {
		case ":method":
			method = h.Value
		case ":path":
			path = h.Value
		case ":scheme":
			scheme = h.Value
		case ":authority":
			authority = h.Value
		default:
			if len(h.Name) > 0 && h.Name[0] == ':' {
				return nil, &Error{Code: ErrProtocolError, Msg: "unknown pseudo-header: " + h.Name}
			}
			regular.Add(h.Name, h.Value)
		}
	}
	if method == "" || path == "" {
		return nil, &Error{Code: ErrProtocolError, Msg: "missing :method or :path"}
	}
	u, err := url.Parse(path)
	if err != nil {
		return nil, &Error{Code: ErrProtocolError, Msg: "bad :path", Underlying: err}
	}
	u.Scheme = scheme
	u.Host = authority
	req := &http.Request{
		Method: method,
		URL:    u,
		Host:   authority,
		Proto:  "HTTP/2.0",
		Header: regular,
		Body:   io.NopCloser(body),
	}
	if path != "" {
		req.RequestURI = path
	}
	_ = strconv.Atoi // keep import alive in skeleton
	return req, nil
}
```

(The full stream.go body — recvHeaders, recvData, recvRSTStream, recvWindowUpdate, dispatch, plus the connector to actions.go's writeH2 — is the bulk of the work in this task. The test file enumerates the contract; the implementation falls out from passing each test in turn.)

- [ ] **Step 4: Append ADR-0048 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0048: HCM H2 server connection manager from scratch

**Status:** Accepted
**Date:** <session date>
**Doctrine:** D-3.2, D-3.5
**Settles:** SPEC ADR-Q; phase-05.1 §4.1 / §5.2 / §10 #1.

### Context

`golang.org/x/net/http2` exposes `http2.Server`, `http2.Server.ServeConn`, `http2.ConfigureServer`, `http2.Transport`, and `http2.Transport.NewClientConn`. These types ostensibly fit the "low-level codec only" framing because they live in the same package as `Framer` and `hpack` — but they are RUNTIMES, not codecs. They carry per-request routing, header canonicalization, response-header injection, error policies, and timeout machinery that diverge from Envoy's behaviour. ADR-0046 explicitly forbids using them.

But "don't use the runtimes" is one half of the decision. The other half: build the runtime ourselves. ADR-0048 codifies the from-scratch decision and the architectural shape.

### Decision

Phase 05.1's `internal/filter/hcm/h2/` sub-package implements:

- **`ServerConn` (conn.go)** — per-downstream-conn state machine. One `ServerConn` value owns one downstream `net.Conn` after ALPN selects "h2" (or after the `--allow-h2c` h2c path bypasses TLS). `Run()` performs the connection preface read + server-initial SETTINGS + client-initial SETTINGS exchange, then enters the frame-dispatch loop. Connection-level errors (bad preface, malformed SETTINGS, HPACK COMPRESSION_ERROR, FRAME_SIZE_ERROR on a non-DATA frame, PUSH_PROMISE received from client, stream-id reuse, even-numbered client stream id) emit GOAWAY with the appropriate code and close.

- **`serverStream` (stream.go)** — per-stream state machine implementing RFC 9113 §5.1: idle → open → half-closed (remote/local) → closed. Server-side stream IDs are odd-numbered client-initiated; even-numbered IDs from the client → PROTOCOL_ERROR. Stream-id reuse → PROTOCOL_ERROR. The dispatch helper waits for END_STREAM-on-headers OR END_STREAM-on-data before invoking the matched action (SPEC §10 #1 settled to wait-for-END_STREAM).

- **No `client.go` in 05.1.** The from-scratch `ClientConn` + `RoundTrip` is 05.2's deliverable per ADR-0045. The h2 sub-package compiles and is unit-tested in 05.1 with server-side surfaces only.

### Consequences

- The discipline is grep-verifiable: `! ls internal/filter/hcm/h2/client.go` (the file does not exist) is part of the 05.1 acceptance check (SPEC §13). Task 16's gate sweep verifies.
- A `routerAction` matched on the H2 path (theoretically possible via misconfiguration but unreachable in 05.1's production bootstraps per SPEC §5.2 step 4c) produces a per-stream INTERNAL_ERROR + RST_STREAM at runtime — the protective shape. Build-time enforcement of "no `routerAction` on H2 listener" is deferred to 05.2 because `Cluster.UseH2()` does not exist yet.
- The H2 connection manager is the project's first multi-stream concurrent state machine. The flow-control window helper (flow.go) is the synchronization primitive; the stream + conn mutexes are minimal and per-instance. SPEC §11.5 + §11.4 mitigations (tiny-window stress, HPACK table-size update propagation) are exercised in `flow_test.go` and `hpack_test.go`.

This ADR supersedes nothing.
```

- [ ] **Step 5: Run; verify pass**. Iterate via TDD on each test sub-case until full pass.

- [ ] **Step 6: Append Task 8 PROGRESS entry; commit**

```bash
git add internal/filter/hcm/h2/stream.go internal/filter/hcm/h2/stream_test.go \
        docs/envoy-go/DECISIONS.md docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: h2 serverStream state machine + dispatch [ADR-0048]"
```

---

## Task 9: `internal/filter/hcm/h2/conn.go` + tests

**Files:**
- Create: `internal/filter/hcm/h2/conn.go`
- Create: `internal/filter/hcm/h2/conn_test.go`
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The connection-manager `ServerConn` orchestrating preface + settings + frame-dispatch loop + GOAWAY emission. Largest single source file in the codec sub-package. ADR-0048 (Task 8) covers the architectural decision; no new ADR here.

**Like Task 8, this task is a §6.1 secondary-trigger candidate.** If sub-steps blow past 15, escalate.

The file is structured around the frame-dispatch switch in `Run()`:

```
1. readClientPreface(downstream)
2. writeServerInitialSettings(fr, settings)
3. readClientSettings(fr, &clientSettings)
4. writeSettingsAck(fr)
5. read settings_ack for own SETTINGS
6. frame-dispatch loop:
   - HEADERS (new stream) → construct serverStream, store, dispatch goroutine
   - HEADERS (existing stream) → discard (trailers; SPEC §2.1)
   - DATA → route to stream.recvData
   - SETTINGS → apply + writeSettingsAck
   - SETTINGS_ACK (response to ours) → discard
   - PING → emit PING_ACK
   - PING_ACK → discard
   - WINDOW_UPDATE (stream 0) → adjust conn send window
   - WINDOW_UPDATE (stream N) → route to stream.recvWindowUpdate
   - RST_STREAM → route to stream.recvRSTStream
   - GOAWAY (received) → mark conn for graceful close
   - PUSH_PROMISE (received) → connError(PROTOCOL_ERROR) + GOAWAY
   - PRIORITY → silently discard (SPEC §2.1)
7. on connection error → emit GOAWAY(code), drain writes, return error
8. on ctx.Done() → emit GOAWAY(NO_ERROR), close, return ctx.Err()
9. clean shutdown → return nil
```

- [ ] **Step 1: Write `internal/filter/hcm/h2/conn_test.go` (failing tests first)**

Test cases:
- `TestServerConn_DirectResponseRoundTrip`: drive an `http2.Transport` (peer, driver-side OK) over a `net.Pipe` against a `ServerConn`; issue `GET /health`; assert 200 + `OK\n`. Validates preface + settings + HEADERS+DATA+END_STREAM round-trip.
- `TestServerConn_ConcurrentStreams`: open 3 streams concurrently against the same conn; complete in arbitrary order; assert each gets the expected response.
- `TestServerConn_MaxConcurrentStreamsEnforcement`: open 101 concurrent streams; assert REFUSED_STREAM on the 101st.
- `TestServerConn_GOAWAYOnProtocolError_EvenStreamID`: peer opens an even-numbered client stream → server emits GOAWAY(PROTOCOL_ERROR) and closes.
- `TestServerConn_PingPingAck`.
- `TestServerConn_PushPromiseFromClient_GOAWAYProtocolError`.
- `TestServerConn_PriorityFrameSilentlyDiscarded`.
- `TestServerConn_HPACKTableSizeUpdate_PropagatesToOutgoingHEADERS`.
- `TestServerConn_TinyWindowDelivery`: peer announces `INITIAL_WINDOW_SIZE=1`; server delivers 1024-byte response via WINDOW_UPDATE-driven progress.
- `TestServerConn_BadPrefaceClosesConnection`.
- `TestServerConn_CtxCancelEmitsGOAWAY`.

The tests collectively exercise the SPEC §4.1 enumerated test set.

- [ ] **Step 2: Run; verify failure**.

- [ ] **Step 3: Write `internal/filter/hcm/h2/conn.go`**

```go
package h2

import (
	"context"
	"net"
	"sync"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/filter/hcm"
)

// ServerConn is one downstream HTTP/2 server connection. Construct with
// NewServerConn; call Run to drive the connection lifecycle.
type ServerConn struct {
	ctx        context.Context
	conn       net.Conn
	table      hcm.RouteTable
	settings   ServerSettings
	fr         *framer
	hpack      *hpackState
	sendW      *window
	recvW      *window
	streams    map[uint32]*serverStream
	streamsMu  sync.Mutex
	lastInID   uint32 // highest stream id we've seen from client (RFC 9113 §5.1.1 monotonic check)
	clientS    clientSettings
	goawaySent bool
}

// NewServerConn constructs a value. Run owns conn (will close on exit).
func NewServerConn(ctx context.Context, conn net.Conn, table hcm.RouteTable, settings ServerSettings) *ServerConn {
	return &ServerConn{
		ctx:      ctx,
		conn:     conn,
		table:    table,
		settings: settings,
		fr:       newFramer(conn),
		hpack:    newHPACKState(settings.HeaderTableSize),
		sendW:    newWindow(int32(settings.InitialWindowSize)),
		recvW:    newWindow(int32(settings.InitialWindowSize)),
		streams:  make(map[uint32]*serverStream),
	}
}

// Run drives the connection lifecycle. Returns nil on clean shutdown,
// *Error on protocol violation, ctx.Err() on cancellation. The caller
// (Filter.Handle) must close conn after Run returns.
func (s *ServerConn) Run() error {
	if err := readClientPreface(s.conn); err != nil {
		return err
	}
	if err := writeServerInitialSettings(s.fr, s.settings); err != nil {
		return err
	}
	if err := readClientSettings(s.fr, &s.clientS); err != nil {
		s.emitGoaway(ErrProtocolError)
		return err
	}
	if err := s.fr.WriteSettingsAck(); err != nil {
		return err
	}
	// Frame dispatch loop.
	for {
		if err := s.ctx.Err(); err != nil {
			s.emitGoaway(ErrNoError)
			return err
		}
		frame, err := s.fr.readFrameCtx(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				s.emitGoaway(ErrNoError)
				return s.ctx.Err()
			}
			return err
		}
		if err := s.dispatchFrame(frame); err != nil {
			var hErr *Error
			if asErr, ok := err.(*Error); ok {
				hErr = asErr
			} else {
				hErr = &Error{Code: ErrInternalError, Underlying: err}
			}
			if hErr.Stream == 0 {
				s.emitGoaway(hErr.Code)
				return err
			}
			// stream-scoped — emit RST_STREAM and continue
			_ = s.fr.WriteRSTStream(hErr.Stream, http2.ErrCode(hErr.Code))
		}
	}
}

func (s *ServerConn) dispatchFrame(frame http2.Frame) error {
	switch f := frame.(type) {
	case *http2.HeadersFrame:
		return s.onHeaders(f)
	case *http2.DataFrame:
		return s.onData(f)
	case *http2.SettingsFrame:
		return s.onSettings(f)
	case *http2.PingFrame:
		return s.onPing(f)
	case *http2.WindowUpdateFrame:
		return s.onWindowUpdate(f)
	case *http2.RSTStreamFrame:
		return s.onRSTStream(f)
	case *http2.GoAwayFrame:
		return s.onGoaway(f)
	case *http2.PushPromiseFrame:
		return connError(ErrProtocolError, "client cannot send PUSH_PROMISE")
	case *http2.PriorityFrame:
		// Silently discard per SPEC §2.1 (RFC 9113 §6.3).
		return nil
	default:
		// Unknown frame types are silently ignored per RFC 9113 §4.1.
		return nil
	}
}

// (per-frame handlers, encodeAndWriteHeaders, writeData, writeRSTStream
// methods follow; collectively ~250 LoC.)

func (s *ServerConn) emitGoaway(code ErrCode) {
	if s.goawaySent {
		return
	}
	s.goawaySent = true
	_ = s.fr.WriteGoAway(s.lastInID, http2.ErrCode(code), nil)
}

// encodeAndWriteHeaders / writeData / writeRSTStream — implements streamConn.
// (full bodies in implementation)

func (s *ServerConn) encodeAndWriteHeaders(streamID uint32, headers []hpack.HeaderField, endStream bool) error {
	encoded := s.hpack.encodeHeaders(headers)
	return s.fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: encoded,
		EndStream:     endStream,
		EndHeaders:    true,
	})
}

func (s *ServerConn) writeData(streamID uint32, b []byte, endStream bool) error {
	return s.fr.WriteData(streamID, endStream, b)
}

func (s *ServerConn) writeRSTStream(streamID uint32, code ErrCode) error {
	return s.fr.WriteRSTStream(streamID, http2.ErrCode(code))
}
```

The `internal/filter/hcm/` ↔ `internal/filter/hcm/h2/` import cycle is resolved per `## Settled SPEC §10 deferred decisions` #10: **one-way import, `hcm → h2` only.** The `h2` package defines a small `Dispatcher` interface (consumed by `h2.NewServerConn`) and a small `Action` interface (`WriteH2(StreamWriter) error`). The `hcm` package adds a NEW file `internal/filter/hcm/h2dispatch.go` carrying the adapter that delegates to `*routeTable.match` and wraps each matched action into the `h2.Action` shape. No `hcm` import in `h2`; no cycle.

- [ ] **Step 4: Add `internal/filter/hcm/h2dispatch.go` (the adapter)**

```go
// internal/filter/hcm/h2dispatch.go — adapter from hcm package to h2 sub-package.
package hcm

import (
	"net/http"

	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// h2Dispatcher implements h2.Dispatcher by delegating to *routeTable.match.
// Wraps each matched action into an h2.Action implementation.
type h2Dispatcher struct {
	table *routeTable
}

func newH2Dispatcher(table *routeTable) *h2Dispatcher {
	return &h2Dispatcher{table: table}
}

func (d *h2Dispatcher) Match(req *http.Request) (h2.Action, bool) {
	entry, ok := d.table.match(req)
	if !ok {
		return &h2DirectResponseAdapter{a: &directResponseAction{status: 404, body: "not found\n"}}, true
	}
	if dr, ok := entry.action.(*directResponseAction); ok {
		return &h2DirectResponseAdapter{a: dr}, true
	}
	// Non-direct_response action on H2 path: return a sentinel that triggers
	// per-stream INTERNAL_ERROR + RST_STREAM in stream.dispatch (SPEC §5.2 step 4c).
	return &h2RouterActionRejection{}, true
}

type h2DirectResponseAdapter struct {
	a *directResponseAction
}

func (a *h2DirectResponseAdapter) WriteH2(sw h2.StreamWriter) error {
	return a.a.writeH2(sw)
}

type h2RouterActionRejection struct{}

func (r *h2RouterActionRejection) WriteH2(sw h2.StreamWriter) error {
	return h2.NewStreamError(h2.ErrInternalError, "router action on h2 listener (SPEC §5.2 step 4c)")
}
```

Add corresponding `Action`, `Dispatcher`, `StreamWriter`, `NewStreamError` exports to the h2 package (the previously-internal `streamWriter` interface becomes `StreamWriter`; the sentinel-returning `NewStreamError` constructs a `*Error` for the rejection path).

- [ ] **Step 5: Run; iterate**

```bash
go build ./internal/filter/hcm/...
go test ./internal/filter/hcm/h2/...
```
Expected: build clean (no import cycle); tests pass.

- [ ] **Step 6: Append Task 9 PROGRESS entry; commit**

```bash
git add internal/filter/hcm/h2/conn.go internal/filter/hcm/h2/conn_test.go \
        internal/filter/hcm/h2dispatch.go \
        docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: h2 ServerConn + h2dispatch adapter (one-way hcm→h2 import)"
```

---

## Task 10: `directResponseAction` codec-neutral factoring (`internal/filter/hcm/actions.go` refactor)

**Files:**
- Modify: `internal/filter/hcm/actions.go` (codec-neutral factoring + `body string` → `bodyText string` field rename per Settled #9)
- Modify: `internal/filter/hcm/actions_test.go`
- Modify: `internal/filter/hcm/config.go` (one-line update: `buildDirectResponseAction` constructs `&directResponseAction{status:..., bodyText: is.InlineString}` reflecting the field rename)
- Create: `internal/filter/hcm/testdata/direct_response_h1.golden` (the byte-for-byte phase-04 wire output captured for the H1 compat test)
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The §5.5 codec-neutral factoring. Phase-04's `directResponseAction.do` becomes a one-line shim over a new `writeH1`; a new `writeH2` is added; a new `body()` method returns the synthesized reply for both paths to share. To make the SPEC-mandated method name `body()` legal, the existing `body string` field on the struct is renamed to `bodyText` (per `## Settled SPEC §10 deferred decisions` #9). The H1 wire output is byte-preserved (verified by a golden file capturing the phase-04 output). No new ADR (factoring is implementation-detail; the decision was settled in ADR-0045).

- [ ] **Step 1: Capture the phase-04 H1 golden bytes**

Before modifying `actions.go`, run a tiny Go program (one-off) that constructs a `directResponseAction{status: 200, body: "OK\n"}` and calls `do(ctx, req, bw)` against a `bufio.Writer` over `bytes.Buffer`; flush; write the bytes to `internal/filter/hcm/testdata/direct_response_h1.golden`. The Date header value is non-deterministic (`time.Now`) — the golden file substitutes a placeholder `<DATE>` and the comparison test substitutes-and-compares.

```go
// scratch tool — run once before refactor
package main
import ("bytes"; "bufio"; "context"; "fmt"; "net/http"; "os"; "regexp"
        "github.com/esalaine/envoy-go/internal/filter/hcm")
func main() {
    var buf bytes.Buffer
    bw := bufio.NewWriter(&buf)
    a := &hcm.DirectResponseAction{Status: 200, Body: "OK\n"} // exposed via test-only helper
    req, _ := http.NewRequest("GET", "/", nil)
    _ = a.Do(context.Background(), req, bw)
    _ = bw.Flush()
    out := regexp.MustCompile(`(?m)^Date: .+$`).ReplaceAllString(buf.String(), "Date: <DATE>")
    _ = os.WriteFile("internal/filter/hcm/testdata/direct_response_h1.golden", []byte(out), 0644)
    fmt.Println("captured")
}
```

(The test-only export of `DirectResponseAction` and `Do` happens via `internal/filter/hcm/export_test.go` — a small file that re-exports private types under capitalised names for the duration of this scratch step. Delete after capture.)

- [ ] **Step 2: Write `internal/filter/hcm/actions_test.go` extensions (failing tests first)**

Add `TestDirectResponseWriteH1Compat` (golden-byte-equivalence after Date substitution) + `TestDirectResponseWriteH2_HEADERSThenDATAEndStream` + `TestDirectResponseBody_StatusHeadersBody`. Existing `TestDirectResponseDo` is rewritten to call `writeH1` (which is what `do` will now invoke).

```go
package hcm

import (
	"bufio"
	"bytes"
	"os"
	"regexp"
	"testing"

	"golang.org/x/net/http2/hpack"
)

func TestDirectResponseWriteH1_GoldenCompat(t *testing.T) {
	a := &directResponseAction{status: 200, body: "OK\n"}
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := a.writeH1(bw); err != nil {
		t.Fatalf("writeH1 = %v", err)
	}
	_ = bw.Flush()
	got := regexp.MustCompile(`(?m)^Date: .+$`).ReplaceAllString(buf.String(), "Date: <DATE>")
	wantBytes, err := os.ReadFile("testdata/direct_response_h1.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(wantBytes) {
		t.Errorf("writeH1 output diverged from phase-04 golden:\nGOT:\n%s\nWANT:\n%s", got, wantBytes)
	}
}

type captureSW struct {
	headerCalls [][]hpack.HeaderField
	dataCalls   [][]byte
	endStream   []bool
}

func (c *captureSW) WriteHeaders(headers []hpack.HeaderField, endStream bool) error {
	c.headerCalls = append(c.headerCalls, headers)
	c.endStream = append(c.endStream, endStream)
	return nil
}
func (c *captureSW) WriteData(b []byte, endStream bool) error {
	c.dataCalls = append(c.dataCalls, append([]byte(nil), b...))
	c.endStream = append(c.endStream, endStream)
	return nil
}

func TestDirectResponseWriteH2_HEADERSThenDATAEndStream(t *testing.T) {
	a := &directResponseAction{status: 200, body: "OK\n"}
	sw := &captureSW{}
	if err := a.writeH2(sw); err != nil {
		t.Fatalf("writeH2 = %v", err)
	}
	if len(sw.headerCalls) != 1 || len(sw.dataCalls) != 1 {
		t.Fatalf("got %d header calls + %d data calls; want 1 + 1", len(sw.headerCalls), len(sw.dataCalls))
	}
	hdrs := sw.headerCalls[0]
	if hdrs[0].Name != ":status" || hdrs[0].Value != "200" {
		t.Errorf("first header = %+v, want :status=200", hdrs[0])
	}
	// Verify regular headers are present and after pseudo-headers.
	wantNames := map[string]bool{"date": false, "server": false, "content-type": false, "content-length": false}
	for _, h := range hdrs[1:] {
		if h.Name[0] == ':' {
			t.Errorf("pseudo-header %q after regular headers (RFC 9113 §8.3 violation)", h.Name)
		}
		if _, want := wantNames[h.Name]; want {
			wantNames[h.Name] = true
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Errorf("missing regular header %q", name)
		}
	}
	if string(sw.dataCalls[0]) != "OK\n" {
		t.Errorf("data = %q, want %q", sw.dataCalls[0], "OK\n")
	}
	// END_STREAM must be set on the DATA frame (the last call), not on HEADERS
	// in this test (because there's a body).
	if sw.endStream[0] /* HEADERS endStream */ {
		t.Errorf("HEADERS frame had endStream=true; expected false (body follows)")
	}
	if !sw.endStream[1] /* DATA endStream */ {
		t.Errorf("DATA frame had endStream=false; expected true (last frame)")
	}
}
```

- [ ] **Step 3: Run; verify failure** (`undefined: writeH1`/`writeH2`/`body`).

- [ ] **Step 4: Refactor `internal/filter/hcm/actions.go`**

```go
package hcm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// errCloseAfterAction (unchanged from phase 04) ...
var errCloseAfterAction = errors.New("hcm: action requested connection close")

// directResponseAction synthesizes a local-reply response. Phase 05.1
// factors the writers codec-neutral: body() returns the codec-independent
// payload; writeH1 writes HTTP/1.1 wire bytes (byte-for-byte phase-04
// preserved); writeH2 writes HTTP/2 HEADERS + DATA frames via a streamWriter.
//
// The phase-04 struct field `body string` is renamed to `bodyText` to free
// the name `body` for the codec-neutral method (SPEC §13 + Settled #9).
// All call sites update mechanically; the wire output is unchanged.
//
// Per ADR-0045 + SPEC §5.5.
type directResponseAction struct {
	status   int
	bodyText string
}

// body returns the synthesized response in codec-neutral form per SPEC §5.5
// + §13's acceptance check. status is the configured value; headers contain
// Date/Server/Content-Type/Content-Length; the returned body bytes are the
// configured inline_string.
func (a *directResponseAction) body() (status int, headers http.Header, body []byte) {
	bodyBytes := []byte(a.bodyText)
	hdrs := http.Header{}
	hdrs.Set("Date", dateHeader())
	hdrs.Set("Server", serverHeader())
	hdrs.Set("Content-Type", "text/plain")
	hdrs.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	return a.status, hdrs, bodyBytes
}

// writeH1 is the H1 adapter — writes HTTP/1.1 wire bytes by delegating to
// writeStatusReply (phase-04 preserved byte-for-byte).
func (a *directResponseAction) writeH1(w io.Writer) error {
	return writeStatusReply(w, a.status, a.bodyText)
}

// writeH2 is the H2 adapter — writes HEADERS (`:status` pseudo first per
// RFC 9113 §8.3, then regular headers in deterministic order) + DATA + END_STREAM
// via the streamWriter.
func (a *directResponseAction) writeH2(sw h2.StreamWriter) error {
	status, hdrs, body := a.body()
	headers := []hpack.HeaderField{
		{Name: ":status", Value: strconv.Itoa(status)},
		{Name: "date", Value: hdrs.Get("Date")},
		{Name: "server", Value: hdrs.Get("Server")},
		{Name: "content-type", Value: hdrs.Get("Content-Type")},
		{Name: "content-length", Value: hdrs.Get("Content-Length")},
	}
	if err := sw.WriteHeaders(headers, false /* body follows */); err != nil {
		return err
	}
	return sw.WriteData(body, true /* end stream */)
}

// do (preserved for the routeAction interface — H1 connection.go calls this
// unchanged). Behaviourally identical to phase-04 because writeH1 == old do.
func (a *directResponseAction) do(_ context.Context, _ *http.Request, bw *bufio.Writer) error {
	return a.writeH1(bw)
}

// routerAction unchanged from phase 04 (no H2 variant in 05.1 per ADR-0045).
type routerAction struct {
	cluster *cluster.Cluster
}

// (routerAction.do unchanged from phase 04 — see actions.go phase-04 code)
```

- [ ] **Step 5: Run; verify pass**

```bash
go test ./internal/filter/hcm/...                # actions_test passes including golden compat
go test ./test/fixtures/0003-http11-routing/...  # fixture-0003 unit-test still passes
```

- [ ] **Step 6: Append Task 10 PROGRESS entry; commit**

```bash
git add internal/filter/hcm/actions.go internal/filter/hcm/actions_test.go \
        internal/filter/hcm/config.go \
        internal/filter/hcm/testdata/direct_response_h1.golden \
        docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: directResponseAction codec-neutral (writeH1 + writeH2 + body)"
```

---

## Task 11: `cmd/envoy-go --allow-h2c` flag + listener-manager `listenerCtx` plumbing + ADR-0049 (--allow-h2c flag)

**Files:**
- Modify: `cmd/envoy-go/main.go`
- Modify: `internal/listener/manager.go`
- Modify: `internal/listener/manager_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0049)
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The plumbing path for the test-only `--allow-h2c` flag. ADR-0049 lands here in the same commit. The HCM `Filter.Handle` ALPN dispatch (Task 12) consumes the per-listener `listenerCtx{hasTLS, allowH2C}`; this task makes the value available.

- [ ] **Step 1: Write `internal/listener/manager_test.go` extensions (failing tests first)**

```go
func TestNewManagerWithBaseDirAndAllowH2C_HTTP2OnPlaintextWithAllow(t *testing.T) {
	bs := buildBootstrap(t, /* plaintext listener with HCM codec_type=HTTP2 */)
	cm := buildClusters(t)
	m, err := NewManagerWithBaseDirAndAllowH2C(bs, cm, "", true /* allowH2C */)
	if err != nil {
		t.Fatalf("NewManagerWithBaseDirAndAllowH2C = %v, want nil", err)
	}
	_ = m
}

func TestNewManagerWithBaseDirAndAllowH2C_HTTP2OnPlaintextWithoutAllow(t *testing.T) {
	bs := buildBootstrap(t, /* same as above */)
	cm := buildClusters(t)
	_, err := NewManagerWithBaseDirAndAllowH2C(bs, cm, "", false /* no allow */)
	if err == nil {
		t.Fatal("NewManagerWithBaseDirAndAllowH2C(allowH2C=false) accepted plaintext+HTTP2; want error")
	}
	if !strings.Contains(err.Error(), "codec_type HTTP2 requires TLS") {
		t.Errorf("error = %q, want substring 'codec_type HTTP2 requires TLS'", err.Error())
	}
}

func TestNewManager_BackwardsCompat_DefaultsAllowH2CFalse(t *testing.T) {
	// Existing NewManager + NewManagerWithBaseDir delegate to the new variant
	// with allowH2C=false. Verify a TLS+HTTP2 bootstrap still builds (TLS
	// satisfies the requirement; allowH2C is irrelevant on the TLS path).
	bs := buildBootstrap(t, /* TLS listener with HCM codec_type=HTTP2 */)
	cm := buildClusters(t)
	m, err := NewManager(bs, cm)
	if err != nil {
		t.Fatalf("NewManager = %v, want nil (TLS+HTTP2 path)", err)
	}
	_ = m
}
```

- [ ] **Step 2: Run; verify failure** (`undefined: NewManagerWithBaseDirAndAllowH2C`).

- [ ] **Step 3: Modify `internal/listener/manager.go`**

Add the new constructor + `listenerCtx` plumbing:

```go
// listenerCtx carries per-chain context that filter constructors consult at
// build time. Phase 05.1 introduces this to plumb the --allow-h2c flag through
// to hcm.NewFilterWithCtx (per ADR-0049). Future phases may extend.
type listenerCtx struct {
	hasTLS   bool
	allowH2C bool
}

// NewManagerWithBaseDirAndAllowH2C is the phase-05.1 constructor variant. It
// threads the --allow-h2c boolean from cmd/envoy-go/main into a per-chain
// listenerCtx passed into the HCM filter constructor at build time. allowH2C
// permits HCM codec_type=HTTP2 on plaintext listeners (for h2spec conformance);
// default false.
func NewManagerWithBaseDirAndAllowH2C(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, baseDir string, allowH2C bool) (*Manager, error) {
	// (body mirrors NewManagerWithBaseDir; the allowH2C bool is captured in
	// the listenerCtx constructed per-chain inside buildListenerRuntime.)
	ls := bs.GetStaticResources().GetListeners()
	if len(ls) == 0 {
		return nil, fmt.Errorf("listener: zero listeners in bootstrap")
	}
	m := &Manager{runtimes: make([]*listenerRuntime, 0, len(ls))}
	seen := make(map[string]struct{}, len(ls))
	for i, l := range ls {
		rt, err := buildListenerRuntimeWithCtx(l, i, cm, baseDir, allowH2C)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[rt.name]; dup {
			return nil, fmt.Errorf("listener: duplicate listener name %q", rt.name)
		}
		seen[rt.name] = struct{}{}
		m.runtimes = append(m.runtimes, rt)
	}
	return m, nil
}

// NewManagerWithBaseDir continues to be the phase-03 entry point; delegates
// with allowH2C=false.
func NewManagerWithBaseDir(bs *bootstrapv3.Bootstrap, cm *cluster.Manager, baseDir string) (*Manager, error) {
	return NewManagerWithBaseDirAndAllowH2C(bs, cm, baseDir, false)
}

// NewManager continues to be the phase-02 entry point; delegates as before.
func NewManager(bs *bootstrapv3.Bootstrap, cm *cluster.Manager) (*Manager, error) {
	return NewManagerWithBaseDirAndAllowH2C(bs, cm, "", false)
}

// buildListenerRuntimeWithCtx mirrors buildListenerRuntime but threads
// listenerCtx{hasTLS, allowH2C} into per-chain filter construction. The
// hasTLS bool comes from chainInfo's tlsCfg != nil at build time.
//
// (body adapts existing buildListenerRuntime; the filterRegistry constructor
// signature is now func(tc, cm, lc) -> filterHandler. The tcpproxy entry
// discards lc; the hcm entry calls hcm.NewFilterWithCtx(tc, cm, lc).)
```

The `filterRegistry` map's signature changes:

```go
type filterConstructor func(tc *anypb.Any, cm *cluster.Manager, lc listenerCtx) (filterHandler, error)

var filterRegistry = map[string]filterConstructor{
	tcpproxy.TypeURL: func(tc *anypb.Any, cm *cluster.Manager, _ listenerCtx) (filterHandler, error) {
		f, err := tcpproxy.NewFilter(tc, cm)
		if err != nil {
			return nil, err
		}
		return f, nil
	},
	hcm.TypeURL: func(tc *anypb.Any, cm *cluster.Manager, lc listenerCtx) (filterHandler, error) {
		// Bridge listenerCtx into hcm.ListenerCtx (the public shape exposed by
		// hcm so that the listener manager doesn't import hcm-internal types).
		f, err := hcm.NewFilterWithCtx(tc, cm, hcm.ListenerCtx{HasTLS: lc.hasTLS, AllowH2C: lc.allowH2C})
		if err != nil {
			return nil, err
		}
		return f, nil
	},
}
```

(The `hcm.ListenerCtx` and `hcm.NewFilterWithCtx` are added in Task 12.)

- [ ] **Step 4: Modify `cmd/envoy-go/main.go`**

```go
func main() {
	cfgPath := flag.String("c", "", "path to envoy-go.yaml (Envoy v3 Bootstrap)")
	allowH2C := flag.Bool("allow-h2c", false,
		"test-only; not for production — permits HCM codec_type=HTTP2 on plaintext listeners for h2spec conformance only")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml> [--allow-h2c]")
		os.Exit(2)
	}
	// ... existing body up to listener.NewManagerWithBaseDir; replace with:
	lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs, cm, filepath.Dir(*cfgPath), *allowH2C)
	if err != nil {
		log.Fatalf("listener manager: %v", err)
	}
	// ... rest unchanged
}
```

- [ ] **Step 5: Append ADR-0049 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0049: Test-only `--allow-h2c` CLI flag on `cmd/envoy-go`

**Status:** Accepted
**Date:** <session date>
**Doctrine:** D-3.5
**Settles:** SPEC ADR-Z; phase-05.1 §4.2 (cmd/envoy-go --allow-h2c) + §10 #5 (form decision).

### Context

Phase 05.1's gate (c) — `h2spec` conformance — must drive an HTTP/2 protocol-level test against the subject. h2spec's standard mode is h2c (cleartext HTTP/2 over plaintext TCP); h2spec's TLS mode requires a custom CA setup that complicates the conformance pin. envoy-go's HCM build-time validator otherwise rejects `codec_type: HTTP2` on plaintext listeners (no TLS handshake = no ALPN selection = no way to differentiate h2 from h1 at the listener level), so a runtime escape hatch is needed for the conformance suite to drive h2c against the subject.

### Decision

Add a test-only CLI flag `--allow-h2c` to `cmd/envoy-go/main.go`. Default OFF. When ON, the listener manager threads `listenerCtx{allowH2C: true}` into HCM filter construction; HCM's build-time validator accepts `codec_type: HTTP2` on plaintext listeners under this condition. The flag is documented in `--help` output as "test-only; not for production".

**Form: CLI flag** (vs env var, vs build tag). Rationale:
- The testcontainers driver (`test/conformance/h2spec/h2spec_test.go`) constructs the subject via `os/exec`; a CLI flag is the lowest-friction option for that driver. An env var would require setting + unsetting in the test's process environment; a build tag would require a separate test binary build.
- The flag is boolean (no value form). A value-bearing form was considered (e.g., `--allow-h2c=ports:8080,8081`) and rejected as over-engineered for a single use site. If a future phase needs per-listener gating, that's a superseding ADR.

The flag is plumbed through:

1. `cmd/envoy-go/main.go`: `flag.Bool("allow-h2c", false, ...)`.
2. `internal/listener/manager.NewManagerWithBaseDirAndAllowH2C(bs, cm, baseDir, allowH2C bool)`: NEW constructor variant. Existing `NewManager` and `NewManagerWithBaseDir` delegate with `allowH2C=false`.
3. `listenerCtx{hasTLS, allowH2C}` per-chain value passed into the `filterRegistry` constructors.
4. `hcm.NewFilterWithCtx(tc, cm, hcm.ListenerCtx{HasTLS, AllowH2C})`: NEW HCM constructor variant. Existing `NewFilter` delegates with the zero-value `ListenerCtx{HasTLS:false, AllowH2C:false}`.
5. `parseFilterWithCtx` consults `lc.HasTLS` and `lc.AllowH2C` to validate `codec_type: HTTP2` per Task 12.

### Consequences

- The flag's runtime cost is one boolean field on `Filter` and one branch in `Filter.Handle` (under the `codec_type=HTTP2` AND plaintext path). Negligible.
- A future doctrine-cleanup phase MAY add a `//go:build !production` build tag to strip the flag entirely from production binaries. 05.1 does not pre-empt that decision — the flag's CI cost is low enough that the production strip is over-engineering at this stage.
- The flag is NOT advertised in `README.md`, `MISSION.md`, or any operator-facing surface other than `--help`. The discipline relies on the documentation discipline; future phases may add a CI-time grep to catch stray references.
- The h2-over-TLS production path is the default-supported configuration in 05.1; `--allow-h2c` does not change anything for that path. Phase 05.2's fixture 0004 uses HTTPS h2 (real ALPN) and does not set `--allow-h2c`.

This ADR supersedes nothing.
```

- [ ] **Step 6: Run; iterate**

```bash
go build ./...
go test ./internal/listener/...
```
Expected: build clean (Task 12 hasn't landed yet; this build will FAIL with `undefined: hcm.NewFilterWithCtx` and `undefined: hcm.ListenerCtx`). The test MUST be deferred to Task 12.

**TDD-iteration note:** Task 11 + Task 12 are partially co-dependent — listener/manager.go references `hcm.NewFilterWithCtx`/`hcm.ListenerCtx` which Task 12 introduces. The executor commits Task 11's `manager.go`, `main.go`, and ADR-0049 in this commit, accepting a temporary build break, and Task 12's commit fixes the build. Or, the executor merges Task 11 + Task 12 into one commit. **Decision: commit Task 11 with a temporary stub file `internal/filter/hcm/listener_ctx_stub.go`** that defines the missing types as zero-value pass-throughs; Task 12's commit deletes the stub and lands the real implementation. This keeps each commit's build green per phase-04 precedent.

```go
// internal/filter/hcm/listener_ctx_stub.go — TEMPORARY, deleted in Task 12.
package hcm

import (
	"google.golang.org/protobuf/types/known/anypb"
	"github.com/esalaine/envoy-go/internal/cluster"
)

type ListenerCtx struct {
	HasTLS   bool
	AllowH2C bool
}

func NewFilterWithCtx(tc *anypb.Any, clusters *cluster.Manager, _ ListenerCtx) (*Filter, error) {
	// Stub: ignore listenerCtx; Task 12's real implementation honours it.
	return NewFilter(tc, clusters)
}
```

- [ ] **Step 7: Append Task 11 PROGRESS entry; commit**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go \
        internal/filter/hcm/listener_ctx_stub.go cmd/envoy-go/main.go \
        docs/envoy-go/DECISIONS.md docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: --allow-h2c flag + listenerCtx plumbing [ADR-0049]"
```

---

## Task 12: HCM `filter.go` ALPN dispatch + `config.go` HTTP2 permit + ADR-0050 (ALPN dispatch wiring)

**Files:**
- Modify: `internal/filter/hcm/filter.go`
- Modify: `internal/filter/hcm/filter_test.go`
- Modify: `internal/filter/hcm/config.go`
- Modify: `internal/filter/hcm/config_test.go`
- Delete: `internal/filter/hcm/listener_ctx_stub.go` (the Task 11 stub)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0050)
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The ALPN-driven codec dispatch in `Filter.Handle` + the `codec_type: HTTP2` build-time acceptance + the listenerCtx-driven validation. ADR-0050 lands here in the same commit. Closes the build gap from Task 11's temporary stub.

- [ ] **Step 1: Write `internal/filter/hcm/config_test.go` + `filter_test.go` extensions (failing tests first)**

(Test list per File Structure entries for `filter_test.go` and `config_test.go`.)

- [ ] **Step 2: Run; verify failure**.

- [ ] **Step 3: Replace the stub with real `parseFilterWithCtx`**

Delete `listener_ctx_stub.go`. Edit `config.go`:

```go
// ListenerCtx carries listener-side context the HCM filter constructor uses
// at build time. Phase 05.1 added this for the --allow-h2c flag plumbing
// (per ADR-0049 + ADR-0050). Future phases may extend.
type ListenerCtx struct {
	HasTLS   bool
	AllowH2C bool
}

// NewFilterWithCtx is the phase-05.1 constructor variant. The existing
// NewFilter delegates with the zero-value ListenerCtx (allowH2C=false,
// hasTLS=false), preserving phase-04 semantics.
func NewFilterWithCtx(tc *anypb.Any, clusters *cluster.Manager, lc ListenerCtx) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, lc)
}

func NewFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error) {
	return parseFilterWithCtx(tc, clusters, ListenerCtx{})
}

func parseFilterWithCtx(tc *anypb.Any, clusters *cluster.Manager, lc ListenerCtx) (*Filter, error) {
	// (body adapted from phase-04's parseFilter; only the codec_type switch
	// changes.)
	if got := tc.GetTypeUrl(); got != TypeURL {
		return nil, fmt.Errorf("hcm: wrong type_url %q (want %q)", got, TypeURL)
	}
	msg := &hcmv3.HttpConnectionManager{}
	if err := tc.UnmarshalTo(msg); err != nil {
		return nil, fmt.Errorf("hcm: unmarshal: %w", err)
	}

	codecType := msg.GetCodecType()
	switch codecType {
	case hcmv3.HttpConnectionManager_HTTP1, hcmv3.HttpConnectionManager_AUTO:
		// ok — H1 or ALPN-driven
	case hcmv3.HttpConnectionManager_HTTP2:
		if !lc.HasTLS && !lc.AllowH2C {
			return nil, fmt.Errorf("hcm: codec_type HTTP2 requires TLS transport_socket (or --allow-h2c for conformance testing)")
		}
	default:
		return nil, fmt.Errorf("hcm: codec_type %s is not supported in phase 05.1", codecType)
	}

	// (rest of parseFilter body unchanged; assigns codecType into Filter.codecType)
	// ... existing stat_prefix, route_config, http_filters validation ...

	return &Filter{
		table:      table,
		clusters:   clusters,
		statPrefix: statPrefix,
		codecType:  codecType,
	}, nil
}
```

(`Filter` struct gains a `codecType hcmv3.HttpConnectionManager_CodecType` field.)

- [ ] **Step 4: Modify `internal/filter/hcm/filter.go`**

```go
package hcm

import (
	"context"
	stdtls "crypto/tls"
	"log"
	"net"

	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// Handle drives one downstream connection from acceptance to close. ALPN
// dispatch (phase 05.1, ADR-0050): on a TLS conn with ALPN==h2, dispatch to
// the h2 codec; otherwise dispatch to the phase-04 H1 driver.
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	if err := ctx.Err(); err != nil {
		_ = downstream.Close()
		return
	}
	defer downstream.Close()

	switch f.codecType {
	case hcmv3.HttpConnectionManager_HTTP1:
		runConnection(ctx, downstream, f.table)
		return
	case hcmv3.HttpConnectionManager_HTTP2:
		f.runH2(ctx, downstream)
		return
	case hcmv3.HttpConnectionManager_AUTO:
		if tlsConn, ok := downstream.(*stdtls.Conn); ok {
			// Defensive: ensure handshake is complete so NegotiatedProtocol is
			// authoritative. Idempotent for already-handshaken conns.
			// SPEC §11.6 mitigation.
			_ = tlsConn.HandshakeContext(ctx)
			if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
				f.runH2(ctx, downstream)
				return
			}
		}
		runConnection(ctx, downstream, f.table)
		return
	}
}

func (f *Filter) runH2(ctx context.Context, downstream net.Conn) {
	disp := newH2Dispatcher(f.table)
	sc := h2.NewServerConn(ctx, downstream, disp, h2.DefaultServerSettings)
	if err := sc.Run(); err != nil {
		log.Printf("hcm: h2: %v", err)
	}
}
```

(`Filter` struct in config.go gains the `codecType` field; `Filter.codecType` is referenced from filter.go above.)

- [ ] **Step 5: Append ADR-0050 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0050: ALPN-driven codec selection inside `Filter.Handle`

**Status:** Accepted
**Date:** <session date>
**Doctrine:** D-3.5
**Settles:** SPEC ADR-V; phase-05.1 §4.2 / §5.4.

### Context

Phase 05.1 introduces a second HTTP codec on the same listener side. The selection between H1 and H2 happens in one of two places:

- **At the listener-side filter-chain match step**: ALPN becomes a `filter_chain_match.application_protocols[]` dimension. Each filter chain carries one codec; the listener manager picks the chain post-handshake based on the negotiated ALPN.
- **Inside `Filter.Handle`**: HCM accepts both codecs (`codec_type: AUTO`) and dispatches at runtime by reading `*tls.Conn.ConnectionState().NegotiatedProtocol`.

ADR-0033 (phase-03's filter-chain subset) explicitly limits filter-chain match to SNI; extending it to ALPN now would expand that subset and require a superseding ADR. Phase 07's filter-chain framework is the natural home for `application_protocols` chain matching.

### Decision

Phase 05.1 implements ALPN dispatch INSIDE `Filter.Handle`, not at the listener-side filter-chain match step. The dispatch logic:

1. Switch on `f.codecType` (parsed at build time from the HCM proto):
   - `HTTP1` → call phase-04's `runConnection` (H1 driver) unchanged.
   - `HTTP2` → call `runH2` which constructs an `h2.ServerConn` and runs it. Build-time validation (in `parseFilterWithCtx`) ensures `HTTP2` is only accepted on TLS listeners OR when `listenerCtx.AllowH2C` is set.
   - `AUTO` → if downstream is `*tls.Conn`, read `ConnectionState().NegotiatedProtocol`; on `"h2"` dispatch to `runH2`; otherwise (plaintext OR TLS-h1 OR TLS-empty-ALPN) dispatch to `runConnection`.

2. Defensive `tlsConn.HandshakeContext(ctx)` no-op call before reading `NegotiatedProtocol`. Idempotent for already-completed handshakes; if a future refactor removes the listener-side handshake, the HCM still gets correct data. SPEC §11.6 mitigation.

3. Listener-side `filter_chain_match.application_protocols[]` field — silently ignored (extends the phase-04 ignored set). ALPN is NOT a chain-match dimension at phase 05.1.

### Consequences

- ADR-0033's filter-chain subset is unchanged. Phase 07's filter-chain framework is the natural close for the `application_protocols` chain match.
- The `Filter.Handle` switch is small and grep-verifiable: one type-assert + one `NegotiatedProtocol` read + one branch on `"h2"`. Easy to review; easy to test.
- A misconfigured client speaking H1 against an `HTTP2`-only listener (rare; would require an h1 client and a server config explicitly forbidding h1) lands in the H2 driver and fails the preface check immediately, returning a connection-level error. Symmetrical to upstream Envoy's posture.
- Per-listener `codec_type: AUTO` is the recommended config for production; `codec_type: HTTP2` is for h2-only listeners (and requires TLS or `--allow-h2c`).
- Phase-05.2's `routerActionH2` will land on top of this same dispatch path; no changes to `Filter.Handle` are needed in 05.2.

This ADR supersedes nothing.
```

- [ ] **Step 6: Run; verify pass**

```bash
go build ./...
go test ./internal/filter/hcm/... ./internal/listener/...
go vet ./...
```
Expected: build clean (the Task 11 stub deletion + Task 12's real implementation completes the bridge); all tests PASS.

- [ ] **Step 7: Append Task 12 PROGRESS entry; commit**

```bash
git add internal/filter/hcm/filter.go internal/filter/hcm/filter_test.go \
        internal/filter/hcm/config.go internal/filter/hcm/config_test.go \
        docs/envoy-go/DECISIONS.md docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git rm internal/filter/hcm/listener_ctx_stub.go
git commit -m "phase 05.1: HCM ALPN dispatch + codec_type=HTTP2 build-time validation [ADR-0050]"
```

---

## Task 13: `cmd/envoy-go/main_test.go` h2-over-TLS bootstrap smoke variant

**Files:**
- Modify: `cmd/envoy-go/main_test.go`
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

End-to-end smoke variant: exercise the entire binary through an h2-over-TLS request → direct_response → 200 round-trip. Uses the fixture-0002 PKI (per `## Settled SPEC §10 deferred decisions` #8 — defaults to fixture-0002 PKI; introduces `test/helpers/tls.go` only if PKI proves insufficient at execution time). No new ADR.

- [ ] **Step 1: Write `TestEnvoyGoBinary_H2Smoke` in `main_test.go`**

```go
func TestEnvoyGoBinary_H2Smoke(t *testing.T) {
	// Use fixture-0002 PKI (committed cert chain with SAN matching localhost
	// and server.example.com). Advertise alpn_protocols: ["h2"] in the
	// listener and codec_type: HTTP2 in the HCM.
	pkiDir := "../../test/fixtures/0002-tls-tcp/pki"
	bootstrapYAML := /* heredoc-templated YAML — TLS listener on 127.0.0.1:0,
	   alpn_protocols: ["h2"], single filter chain with HCM codec_type=HTTP2,
	   route_config with one direct_response 200 "OK\n", admin on 127.0.0.1:0 */ ""

	cfgPath := filepath.Join(t.TempDir(), "envoy-go.yaml")
	if err := os.WriteFile(cfgPath, []byte(bootstrapYAML), 0644); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}

	bin := buildBinaryOrSkip(t)
	cmd := exec.Command(bin, "-c", cfgPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start envoy-go: %v", err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	// Wait for sentinel (mirror of the phase-04 H1 smoke variant's polling).
	addr := waitForReadySentinel(t)

	// Issue an HTTP/2 request via http2.Transport with InsecureSkipVerify.
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
		},
	}
	defer transport.CloseIdleConnections()

	req, _ := http.NewRequest("GET", "https://"+addr+"/", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK\n" {
		t.Errorf("body = %q, want %q", body, "OK\n")
	}
	if resp.ProtoMajor != 2 {
		t.Errorf("ProtoMajor = %d, want 2 (HTTP/2)", resp.ProtoMajor)
	}
	_ = pkiDir
}
```

- [ ] **Step 2: Run; verify pass**

```bash
go test ./cmd/envoy-go/... -run TestEnvoyGoBinary_H2Smoke -v
```

If fixture-0002 PKI proves insufficient (SAN mismatch on `127.0.0.1`), introduce `test/helpers/tls.go` per the §10-settled decision #8 — generate a self-signed cert in-test with SAN `127.0.0.1`. Document the introduction in PROGRESS.

- [ ] **Step 3: Append Task 13 PROGRESS entry; commit**

```bash
git add cmd/envoy-go/main_test.go docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
# Plus test/helpers/tls.go IF needed
git commit -m "phase 05.1: cmd/envoy-go/main_test.go h2-over-TLS smoke variant"
```

---

## Task 14: `internal/filter/hcm/h2/fuzz_test.go` — `FuzzFrameStream` + `FuzzHPACKDecode`

**Files:**
- Create: `internal/filter/hcm/h2/fuzz_test.go`
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

Two fuzz targets at the 30-second CI short-budget per ADR-0018. No new ADR (phase-04 fuzz precedent applies verbatim).

- [ ] **Step 1: Write `internal/filter/hcm/h2/fuzz_test.go`**

```go
package h2

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// FuzzFrameStream mutates a corpus of well-formed frame sequences and asserts
// no panic + every returned error begins with "h2:".
func FuzzFrameStream(f *testing.F) {
	// Three seed entries:
	// (1) preface only
	// (2) preface + server-initial SETTINGS (peer-side capture; we feed it as the client preface)
	// (3) preface + SETTINGS + HEADERS-with-END_STREAM
	f.Add([]byte(clientPrefaceBytes))
	f.Add(append([]byte(clientPrefaceBytes),
		// minimal SETTINGS frame: 9-byte header + 0 payload, type=4, flags=0, stream=0
		0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
	))
	f.Add(append([]byte(clientPrefaceBytes),
		0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, // SETTINGS empty
		// (HEADERS frame seed payload — left as a minimal smoke; fuzzer mutates)
	))

	f.Fuzz(func(t *testing.T, input []byte) {
		// Drive a ServerConn over an in-memory pipe whose read side returns
		// the input bytes then EOF. We don't need a real route table — a nil
		// dispatcher whose Match never matches is enough.
		// (Implementation: a fake net.Conn backed by bytes.Reader for reads.)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		conn := newReplayConn(input)
		defer conn.Close()
		// Use a stub dispatcher that always returns 404 direct response.
		disp := stubDispatcher{}
		sc := NewServerConn(ctx, conn, disp, DefaultServerSettings)
		err := sc.Run()
		// No panic: assured by reaching here.
		if err != nil && !strings.HasPrefix(err.Error(), "h2:") && err != context.DeadlineExceeded && err != context.Canceled {
			t.Errorf("error %q does not begin with 'h2:' (and is not a ctx error)", err.Error())
		}
	})
}

// FuzzHPACKDecode wraps the per-conn hpackState.decodeBlock with adversarial
// input. The underlying x/net/http2/hpack package has its own fuzzer; this is
// a wrapper-level integration test for our usage patterns.
func FuzzHPACKDecode(f *testing.F) {
	st := newHPACKState(4096)
	// Three seeds: empty, well-formed single-pseudo-header block, longer block.
	f.Add([]byte{})
	f.Add(st.encodeHeaders(nil))
	// (intentional: encoded representation of a small header set; fuzzer mutates)

	f.Fuzz(func(t *testing.T, block []byte) {
		st := newHPACKState(4096)
		_, err := st.decodeBlock(block, true)
		if err != nil && !strings.HasPrefix(err.Error(), "h2:") {
			t.Errorf("error %q does not begin with 'h2:'", err.Error())
		}
		// No panic: assured by reaching here.
	})
}

// (replayConn + stubDispatcher helpers — minimal net.Conn satisfying io.Reader
// over bytes.Reader, no-op Writer, hardcoded SetReadDeadline; stub dispatcher
// returns the 404 catch-all from the h2dispatch path.)

type replayConn struct {
	r *bytes.Reader
	w *bytes.Buffer
	// (rest of net.Conn methods — addr stubs, deadlines respected)
}

func newReplayConn(b []byte) *replayConn { return &replayConn{r: bytes.NewReader(b), w: &bytes.Buffer{}} }
// (Read/Write/Close/Local/Remote/SetDeadline/SetReadDeadline/SetWriteDeadline)
```

- [ ] **Step 2: Run; verify the fuzz infrastructure compiles + the seeds run clean**

```bash
go test ./internal/filter/hcm/h2/ -run FuzzFrameStream -fuzz=FuzzFrameStream -fuzztime=10s
go test ./internal/filter/hcm/h2/ -run FuzzHPACKDecode -fuzz=FuzzHPACKDecode -fuzztime=10s
```
Expected: each fuzz target runs the seeds + ~hundreds of mutations in 10s with no crashers.

- [ ] **Step 3: Run the full 30s budget per ADR-0018**

```bash
go test ./internal/filter/hcm/h2/ -run FuzzFrameStream -fuzz=FuzzFrameStream -fuzztime=30s
go test ./internal/filter/hcm/h2/ -run FuzzHPACKDecode -fuzz=FuzzHPACKDecode -fuzztime=30s
git status --porcelain   # expect: empty (no testdata/fuzz/ pollution per ADR-0018)
```

- [ ] **Step 4: Append Task 14 PROGRESS entry; commit**

```bash
git add internal/filter/hcm/h2/fuzz_test.go docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: h2 fuzz targets — FuzzFrameStream + FuzzHPACKDecode"
```

---

## Task 15: `test/conformance/h2spec/` + `CONFORMANCE_PINS.md` + ADR-0051 (h2spec threshold + pin)

**Files:**
- Create: `test/conformance/h2spec/h2spec.go`
- Create: `test/conformance/h2spec/h2spec_test.go`
- Create: `docs/envoy-go/CONFORMANCE_PINS.md`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0051)
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The project's first conformance suite. Gate (c) is **non-vacuous for the first time in the project** — h2spec runs against the subject and reports `failed == 0` over the threshold sections.

- [ ] **Step 1: Pull the pinned image + capture the SHA256**

```bash
docker pull summerwind/h2spec:v2.6.0
SHA=$(docker inspect --format '{{.Id}}' summerwind/h2spec:v2.6.0)
echo "Tag: v2.6.0  SHA: $SHA"
```

(The exact tag and SHA value are filled into `CONFORMANCE_PINS.md` and `h2spec.go` at this step. The version `v2.6.0` is illustrative; the executor uses the latest tagged release at PLAN-execution time.)

- [ ] **Step 2: Write `docs/envoy-go/CONFORMANCE_PINS.md`**

```markdown
# envoy-go Conformance Pins

This file is the canonical pin table for external conformance suites consumed by envoy-go's gate (c). Each pin is a tag + SHA256 digest pair; CI uses the digest, not the tag, so `latest` drift cannot accidentally break the gate. The pin's refresh procedure is per `BOOTSTRAP_PROMPT.md` D-3.7 — pin updates are dedicated phase work, with their own differential re-baselining where applicable.

---

## Pins

### `summerwind/h2spec` — HTTP/2 protocol conformance

- **Image:** `summerwind/h2spec`
- **Tag:** `v2.6.0` (or latest stable at PLAN-execution time)
- **Digest:** `sha256:<filled-at-Task-15-execution>`
- **Purpose:** Phase 05.1 gate (c) — drives HTTP/2 protocol-level conformance against an envoy-go h2c listener (subject started with `--allow-h2c`).
- **Threshold sections (per ADR-0051):** 3 (HTTP Frame Format), 4 (HPACK), 5 (Streams and Multiplexing), 6 (Frame Definitions) MINUS 6.6 (PUSH_PROMISE), 7 (Error Codes), 8 (HTTP Message Exchanges). All child tests under threshold sections must report `failed == 0` in h2spec's JUnit-XML output.
- **Excluded subsections:** 6.6 (PUSH_PROMISE) — phase 05.1 disables push (`SETTINGS_ENABLE_PUSH=0` per ADR-0047).
- **Introduced by:** phase 05.1.
- **Justifying ADR:** `ADR-0051`.
- **Source-of-truth mirror:** `test/conformance/h2spec/h2spec.go`'s `h2specImage` constant. The Go const carries an `// authoritative pin: docs/envoy-go/CONFORMANCE_PINS.md` comment so the doc file remains the single source of truth.

---

## Refresh procedure

Per D-3.7, refreshing a pin is dedicated phase work:

1. Pull the candidate version: `docker pull summerwind/h2spec:<new-tag>`.
2. Run h2spec at the candidate version against the subject (`go test ./test/conformance/h2spec/...`).
3. Investigate any new failures:
   - If the failure indicates an envoy-go regression, fix envoy-go and re-run.
   - If the failure indicates a new conformance test in a section already under threshold and the test is correct (i.e., reveals an envoy-go bug), fix envoy-go.
   - If the failure is in a section we explicitly exclude (e.g., 6.6) or is a known-deferred surface, document via a superseding ADR that updates the threshold list.
4. Update the digest in `CONFORMANCE_PINS.md`.
5. Update the `h2specImage` constant in `test/conformance/h2spec/h2spec.go`.
6. Commit. Phase id: a dedicated `pin/h2spec-refresh-YYYYMMDD` phase or a phase whose scope explicitly bundles the pin refresh.

The pin refresh is its own phase (or sub-phase); never refresh the pin as a side-effect of another phase's work.

---

## Cross-references

- `docs/envoy-go/DECISIONS.md` ADR-0051 — h2spec scope, threshold, and pin.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` `## HTTP/2` subsection — narrative form of the threshold list.
- `test/conformance/h2spec/h2spec.go` — the Go const mirror of the pin.
- `BOOTSTRAP_PROMPT.md` D-3.7 — pin discipline.
```

- [ ] **Step 3: Write `test/conformance/h2spec/h2spec.go`**

```go
// Package h2spec carries phase-05.1's h2spec conformance helper. The Go
// constants below mirror the canonical pin in
// docs/envoy-go/CONFORMANCE_PINS.md; the doc file is authoritative — the
// constants are a typed mirror for use from the test driver.
package h2spec

// authoritative pin: docs/envoy-go/CONFORMANCE_PINS.md
const (
	h2specImage  = "summerwind/h2spec"
	h2specTag    = "v2.6.0"
	h2specDigest = "sha256:<filled-at-Task-15-execution>"
)

// imageRef returns the image@digest string consumed by testcontainers-go.
func imageRef() string { return h2specImage + "@" + h2specDigest }

// thresholdSections is the set of h2spec section identifiers the conformance
// gate requires `failed == 0` on. Per ADR-0051.
var thresholdSections = []string{"3", "4", "5", "6", "7", "8"}

// excludedSubsections is the set of section.subsection identifiers excluded
// from the threshold even though their parent section is included. Per
// ADR-0051: section 6 includes everything except 6.6 (PUSH_PROMISE).
var excludedSubsections = []string{"6.6"}
```

- [ ] **Step 4: Write `test/conformance/h2spec/h2spec_test.go`** per the SPEC §5.9 shape.

(Full test body omitted from this PLAN for length — the executor implements per the SPEC §5.9 ten-step procedure. Key TDD steps below.)

- [ ] **Step 5: Iterate the test until h2spec runs `failed == 0` over the threshold sections**

```bash
go test ./test/conformance/h2spec/ -v -timeout 5m
```

Expected: PASS with a structured report quoting the per-section pass-counts. If any threshold-section test fails, follow the systematic-debugging path: identify which RFC 9113 rule the codec is violating; fix the codec; re-run.

- [ ] **Step 6: Append ADR-0051 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0051: h2spec conformance scope, threshold, and pin

**Status:** Accepted
**Date:** <session date>
**Doctrine:** D-3.6, D-3.7
**Settles:** SPEC ADR-U; phase-05.1 §3 gate (c) + §5.9 + §10 #4.

### Context

Phase 05.1 introduces the project's first NON-VACUOUS conformance gate. `BOOTSTRAP_PROMPT.md` §7.5 row (c) requires conformance suites to pass for in-scope features. Phase 05.1's in-scope feature is HTTP/2 — `summerwind/h2spec` is the canonical community-maintained protocol-conformance suite for HTTP/2.

### Decision

- **Pin:** `summerwind/h2spec` at tag `v2.6.0` (or latest stable at PLAN-execution time) + SHA256 digest. The pin lives in `docs/envoy-go/CONFORMANCE_PINS.md` (NEW file in 05.1 — sibling to `ENVOY_TARGET.md`, same refresh discipline per D-3.7).

- **Threshold sections:** 3 (HTTP Frame Format), 4 (HPACK), 5 (Streams and Multiplexing), 6 (Frame Definitions) MINUS 6.6 (PUSH_PROMISE), 7 (Error Codes), 8 (HTTP Message Exchanges).

- **Threshold rule:** all child tests under threshold sections must report `failed == 0` in h2spec's JUnit-XML output (`--junit-report=/tmp/h2spec.xml`).

- **Section 6.6 exclusion:** phase 05.1 disables push (`SETTINGS_ENABLE_PUSH=0` per ADR-0047). The PUSH_PROMISE conformance subsection is irrelevant to a non-pushing server. A future phase that re-enables push (unlikely; there's no use case in Envoy's current default config) supersedes this exclusion.

- **Driver:** `test/conformance/h2spec/h2spec_test.go` boots the subject with `--allow-h2c -c <synthetic-h2c-bootstrap>`, runs `summerwind/h2spec` via `testcontainers-go` against the subject's listener port, parses the JUnit-XML output, asserts threshold compliance.

- **CI gate:** `go test ./test/conformance/h2spec/...`. Runtime budget: ~30s wall-clock. Excluded from `-short` (`testing.Short()` skips).

### Consequences

- Gate (c) is **non-vacuous for the first time in the project**. The verification-and-review session for 05.1 records the gate-c output verbatim (per phase-04 PROGRESS shape).
- The pin's refresh procedure is documented in `CONFORMANCE_PINS.md` and is dedicated phase work per D-3.7 — never refreshed as a side-effect of another phase. A pin refresh that adds new failing sections triggers a superseding ADR that either fixes envoy-go or extends the exclusion list.
- The threshold list is intentionally not minimal — it covers the protocol surface phase 05.1 actually implements. Sections we don't implement (6.6 PUSH_PROMISE) are excluded; sections we DO implement (3, 4, 5, 6 ex-6.6, 7, 8) are required to pass.

This ADR supersedes nothing (project's first conformance ADR; sets the pattern future phases follow for h3spec, h2spec re-baselines, etc.).
```

- [ ] **Step 7: Append Task 15 PROGRESS entry; commit**

```bash
git add test/conformance/h2spec/h2spec.go test/conformance/h2spec/h2spec_test.go \
        docs/envoy-go/CONFORMANCE_PINS.md docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: h2spec conformance suite + CONFORMANCE_PINS.md [ADR-0051]"
```

---

## Task 16: BEHAVIOR_CONTRACT `## HTTP/2` SCAFFOLD + ADR-0052 + ADR-0053 + all-gates green local sweep

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (add `## HTTP/2` subsection after `## HTTP/1.1`; extend `## Header allow-list` table)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0052 + ADR-0053)
- Modify: `docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md`

The phase-05.1 closing task: codify the new equivalence surface in BEHAVIOR_CONTRACT in SCAFFOLD form, land ADR-0052 + ADR-0053 in the same commit, then run the all-gates green local sweep mirroring phase-04's Task 17 shape (gates a/b/c/d/e per SPEC §3; gate f deferred per BOOTSTRAP §5 step 6).

- [ ] **Step 1: Extend the `## Header allow-list` table in `BEHAVIOR_CONTRACT.md`**

Add five rows after the phase-04 rows (the existing block ends at the `x-request-id` row):

```
| :status | HCM-locally-generated H2 responses; required + value-asserted | Phase 05.1 | ADR-0052 |
| :method | Routed-to-upstream H2 requests; required + value-asserted (applies-to: 05.2 routed-to-upstream H2) | Phase 05.1 | ADR-0052 |
| :path | Routed-to-upstream H2 requests (applies-to: 05.2) | Phase 05.1 | ADR-0052 |
| :scheme | Routed-to-upstream H2 requests (applies-to: 05.2) | Phase 05.1 | ADR-0052 |
| :authority | Routed-to-upstream H2 requests (applies-to: 05.2) | Phase 05.1 | ADR-0052 |
```

- [ ] **Step 2: Add the new `## HTTP/2` SCAFFOLD subsection after `## HTTP/1.1`**

Append (mirror phase-04's `## HTTP/1.1` shape):

```markdown
## HTTP/2

*Introduced by phase 05.1. Justified by ADR-0046 (codec source: x/net/http2.Framer + hpack), ADR-0047 (server settings defaults), ADR-0048 (server connection manager from scratch), ADR-0050 (ALPN dispatch wiring), ADR-0051 (h2spec threshold + pin), ADR-0052 (this subsection — SCAFFOLD form for 05.1).*

Phase 05.1 introduces envoy-go's downstream HTTP/2 dataplane: ALPN-driven codec dispatch inside HCM `Filter.Handle`, a from-scratch server-side codec under `internal/filter/hcm/h2/` (`ServerConn` + `serverStream` + framer + hpack + flow + settings + preface + errors), the project's first non-vacuous conformance gate via `summerwind/h2spec` (per ADR-0051), and a codec-neutral `directResponseAction.body()` factoring that supports both H1 and H2 wire emission.

This subsection is **SCAFFOLD form** — phase 05.2's brainstorming session edits this subsection IN PLACE (per ADR-0052's authorisation) to flip the deferred-to-05.2 items to active rules when fixture 0004 lands.

### Asserted equivalence (05.1 scope)

- **Conformance, not differential.** The 05.1 H2 surface is asserted via h2spec against the subject standalone, not via a side-by-side proxy-vs-proxy fixture. The differential equivalence of the `direct_response` H2 path (status, decoded body, header set-equality, framing structure) is exercised indirectly through h2spec section 8 (HTTP Message Exchanges).
- **`:status` per request:** required + asserted by h2spec section 8 on every `direct_response` invocation.
- **Decoded body bytes** on `direct_response` 2xx paths: byte-equal to the configured `body` string (h2spec validates indirectly via response-length and END_STREAM checks; envoy-go's unit tests assert byte equality directly).
- **Per-stream response header set-equality modulo allow-list:** locally-generated H2 responses carry `:status` (required + asserted), `Server` (required, value `envoy` per ADR-0014, matched verbatim with upstream's value also `envoy`), `Content-Type`, `Content-Length`, `Date` (presence required; value not byte-compared — same as phase-01 admin/ready discipline). Routed-to-upstream H2 surface: NOT YET ASSERTED IN 05.1 (deferred to 05.2 + fixture 0004).

### Not asserted (05.1 scope)

- Wire-byte H2 framing (frame headers, frame ordering at byte level, padding bytes, HPACK encoded-bytes representation). Frame *types* and *types-on-equivalent-events* are required to match (verified via h2spec section 6); frame *byte-equivalence* is not.
- SETTINGS values byte-for-byte (h2spec section 6.5 only validates RFC 9113 compliance, not Envoy-specific values).
- WINDOW_UPDATE timing or count.
- Stream id allocation pattern.
- Trailers (per phase-05.1 trailer rule + 05.2's deferred ADR).
- 0-RTT TLS early-data behaviour.
- **Routed-to-upstream H2 request preservation, decoded body equivalence on routed-to-upstream paths, per-cluster RR distribution on H2, ALPN selection equivalence at the differential level** — ALL DEFERRED TO 05.2.

### Header allow-list extensions

See the `## Header allow-list` table above, rows added by ADR-0052: `:status` (active in 05.1; locally-generated H2 responses), `:method`/`:path`/`:scheme`/`:authority` (forward-looking, applies-to: 05.2 routed-to-upstream H2). The 05.1 scaffold inserts the rows so 05.2's brainstorming has nothing to add to the table itself; only the "applies-to" cells flip when fixture 0004 lands.

### h2spec threshold

Sections 3, 4, 5, 6 (excluding 6.6 PUSH_PROMISE), 7, 8 — all `failed == 0`. Pin: `summerwind/h2spec` at the SHA recorded in `CONFORMANCE_PINS.md` per ADR-0051.

### Applies to (05.1)

- Phase-05.1 envoy-go `internal/filter/hcm/h2/` package (server-side only).
- The codec-neutral `directResponseAction` factoring in `internal/filter/hcm/actions.go` (per SPEC §5.5).
- The conformance suite under `test/conformance/h2spec/`.

### Does not yet apply to

- Routed-to-upstream H2 (05.2 + fixture 0004 — closes ADR-0035 H2 leg).
- HTTP/3 (later).
- Server push (out of scope permanently in 05.1; potentially out of scope project-wide).
- gRPC framing.
- Trailer forwarding (deferred to phase 07 framework + gRPC family).
- Upstream H2 stream pooling (upstream-robustness family).
- h2c production fixtures (test-only path; production builds may strip the `--allow-h2c` flag in a future doctrine-cleanup phase).
- mTLS over h2 (deferred).
```

- [ ] **Step 3: Append ADR-0052 + ADR-0053 to `docs/envoy-go/DECISIONS.md`**

(Bodies per `## ADRs introduced by this plan` summaries.)

- [ ] **Step 4: Commit the BEHAVIOR_CONTRACT + ADR-0052 + ADR-0053 update**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md
git commit -m "phase 05.1: BEHAVIOR_CONTRACT HTTP/2 scaffold + REVIEW carry-forward [ADR-0052, ADR-0053]"
```

- [ ] **Step 5: All-gates green local sweep — gate (a): differential fixtures (vacuous in 05.1)**

```bash
go test ./test/differential/... -timeout=12m
```
Expected: every PRE-EXISTING fixture (0000, 0001, 0002, 0003) PASS. Gate (a) is vacuously green per ADR-0045 (no new fixture in 05.1). Quote last 30 lines of output verbatim into the PROGRESS entry; explicitly note "gate (a) — VACUOUS — no new fixture in 05.1 per ADR-0045; pre-existing fixtures (gate (b)) all green".

- [ ] **Step 6: All-gates green local sweep — gate (b): every package's unit tests**

```bash
go test -race ./...
```
Expected: every package PASS, no data races. Quote last 30 lines verbatim.

- [ ] **Step 7: All-gates green local sweep — gate (c): h2spec conformance (NEWLY NON-VACUOUS)**

```bash
go test ./test/conformance/h2spec/... -timeout=5m -v
```
Expected: PASS with the threshold sections all reporting `failed == 0`. Quote last 30 lines verbatim — this is the project's FIRST non-vacuous gate-(c) result and the verification-before-completion session will read it.

- [ ] **Step 8: All-gates green local sweep — gate (d): fuzz targets short budget (30s each)**

```bash
go test ./internal/bootstrap -run=FuzzBootstrapLoad -fuzz=FuzzBootstrapLoad -fuzztime=30s
go test ./internal/filter/tcpproxy -run=FuzzTcpProxyFilter -fuzz=FuzzTcpProxyFilter -fuzztime=30s
go test ./internal/tls -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s
go test ./internal/filter/hcm -run=FuzzHCMConfigParse -fuzz=FuzzHCMConfigParse -fuzztime=30s
go test ./internal/filter/hcm/h2 -run=FuzzFrameStream -fuzz=FuzzFrameStream -fuzztime=30s
go test ./internal/filter/hcm/h2 -run=FuzzHPACKDecode -fuzz=FuzzHPACKDecode -fuzztime=30s
git status --porcelain   # expect: empty (no testdata/fuzz/ pollution)
```
Expected for each: ~30s run, PASS. Quote each summary line into PROGRESS.

- [ ] **Step 9: All-gates green local sweep — gate (e): vet + golangci-lint + boundary grep**

```bash
go vet ./...
golangci-lint run ./...
# ADR-0046 boundary grep:
! grep -nR '"golang.org/x/net/http2"' internal/ cmd/envoy-go/main.go --include='*.go' | grep -v '_test.go' | grep -v 'internal/filter/hcm/h2/framer.go\|internal/filter/hcm/h2/hpack.go\|internal/filter/hcm/h2/settings.go\|internal/filter/hcm/h2/conn.go\|internal/filter/hcm/h2/stream.go'
# (the grep should return empty — all production-code imports of golang.org/x/net/http2 are confined to the h2 sub-package's listed files)
# ADR-0048 boundary check:
! ls internal/filter/hcm/h2/client.go
# (file does not exist; client.go is 05.2's deliverable)
```
Expected: vet clean; lint clean; boundary grep returns empty (or only flagged-OK files); `client.go` does not exist.

- [ ] **Step 10: Gate (f) deferral note**

Per `BOOTSTRAP_PROMPT.md` §5 step 6, gate (f) (REVIEW.md approval) is owned by the `superpowers:requesting-code-review` session that follows the executor's commit. Record as `f: deferred to requesting-code-review session per BOOTSTRAP §5 step 6`.

- [ ] **Step 11: Append a Task 16 PROGRESS entry with every command output verbatim**

This PROGRESS entry is the session's "verification proof" — `superpowers:verification-before-completion` reads it when phase 05.1 moves to lifecycle-state 4. Keep every last-30-lines block verbatim. Mirror phase-04's Task 17 PROGRESS shape.

- [ ] **Step 12: Commit**

```bash
git add docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md
git commit -m "phase 05.1: Task 16 — all-gates green local sweep (a vacuous; b/c/d/e green; f deferred)"
```

- [ ] **Step 13: Confirm phase-05.1 readiness for state-4 transition (do NOT advance STATE — that's a later session per ADR-0005)**

This plan-authored phase ends with Task 16 committed on `phase/05.1-downstream-h2-impl`. STATE advancement through 4 → 5 → 6 is per-session work, not this plan's responsibility.

---

## Post-plan handoff: state advancement + worktree cleanup (session-exit duties)

This section is the plan-authoring session's exit contract, not an executable task.

After the executing session commits Task 16 on `phase/05.1-downstream-h2-impl`:

1. **Fast-forward merge to master.** Per ADR-0003:
   ```bash
   cd /home/esa/git/envoy-go   # master worktree
   git merge --ff-only phase/05.1-downstream-h2-impl
   ```
2. **Advance `docs/envoy-go/STATE.md` on master** to `lifecycle-state: 4` + `next-skill: superpowers:verification-before-completion`, reflecting that the next fresh session runs verification before REVIEW. Commit with `phase 05.1: STATE.md → lifecycle-state 4`.
3. **The verification session** (next-next from the current plan-authoring session) advances STATE through 5 and 6 per the state machine. Phase-05.1 ROADMAP row 05.1 advances to `done` at state 6. Phase 05.2's STATE handoff (`active-phase: 05.2-upstream-h2`, `lifecycle-state: 1`, `next-skill: superpowers:brainstorming`) lands with the final phase-05.1 commit. Row 05 (parent) stays `in-progress` until 05.2 also reaches `done`.

**No part of this section is done by Task 16.** It lives here so the plan-authoring session knows where to leave STATE after its own commit, and so the executing session has clear context for its exit.

This plan-authoring session's own exit contract:

1. After plan-document-reviewer approves (`## Plan review loop` below), commit `PLAN.md` on `phase/05.1-downstream-h2-plan`.
2. Update `docs/envoy-go/STATE.md` on the same branch: `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development`, `next-skill-scope: <execute PLAN.md>`, `last-commit: <PLAN.md commit SHA>`.
3. Fast-forward `master` to `phase/05.1-downstream-h2-plan` per ADR-0003.
4. Exit clean.

---

## Plan review loop (invoked at end of plan-authoring session)

Per `superpowers:writing-plans` and ADR-0005: after this PLAN.md is written, dispatch the `plan-document-reviewer` subagent with the PLAN.md path + the SPEC.md path. If the reviewer returns approved → commit PLAN.md + STATE advancement (state 2 → state 3 on master). If the reviewer returns changes-requested → address feedback in place, re-dispatch (max 3 iterations per ADR-0005 + skill guidance); on iteration 3 without approval, exit blocked per `BOOTSTRAP_PROMPT.md` §5 deviations.

The reviewer's scope:

- Does the PLAN cover every SPEC §4 deliverable? (9 production source files in `h2/`; HCM ALPN dispatch + config HTTP2 + listenerCtx; codec-neutral `directResponseAction` factoring; `--allow-h2c` flag; `cmd/envoy-go/main_test.go` h2-over-TLS smoke; `test/conformance/h2spec/`; `CONFORMANCE_PINS.md`; `BEHAVIOR_CONTRACT.md ## HTTP/2` SCAFFOLD subsection; eight ADRs ADR-0046..ADR-0053; phase-04 REVIEW carry-forward.)
- Does the PLAN settle every 05.1-scoped SPEC §10 deferred decision? (5 items — see `## Settled SPEC §10 deferred decisions`.)
- Does the PLAN mitigate every SPEC §11 risk with a task-level step or an ADR? (11.1 phase-splitting → `## Scope check`; 11.2 h2spec image pin → ADR-0051; 11.3 x/net/http2 drift → `## Tech Stack` go.mod pin; 11.4 HPACK table-size negotiation → `hpack_test.go` test; 11.5 tiny-window deadlock → `flow_test.go` stress test; 11.6 ALPN dispatch race → `tlsConn.HandshakeContext(ctx)` defensive call in Task 12; 11.7 phase-04 M-7 carry-forward → `## Phase-04 REVIEW carryover resolution matrix` + ADR-0053; 11.8 directResponseAction refactor regression → Task 10 golden test; 11.9 h2spec JUnit-XML schema drift → `CONFORMANCE_PINS.md` discipline.)
- Does the PLAN resolve phase-04 REVIEW Minors triaged in SPEC §12? (5 carries forward, all DEFERRED via ADR-0053; 2 resolved-prior — see matrix.)
- Are tasks atomic (one logical commit each, 2–5 minutes per step except the well-annotated longer ones — Task 8 stream.go, Task 9 conn.go, Task 16 final sweep)?
- Does the ADR number sequence match verified DECISIONS.md tail? (ADR-0045 → ADR-0046..0053 — re-verified at Task 1 step 1.)
- Is the LoC estimate honest and does the scope-check argument hold? (Per `## Scope check`: ~2400 LoC, 16 tasks, no further coherent split axis exists; per phase-04 precedent, one-sub-phase shipment is correct.)
- Are spec-review advisory items addressed? (None in 05.1 — see `## Spec-review advisory responses`.)
- Does the import topology avoid cycles between `internal/filter/hcm/` and `internal/filter/hcm/h2/`? (Yes — `hcm → h2` via `h2dispatch.go`; `h2` does NOT import `hcm`. See Task 9 cycle resolution.)
- Are the ADR-0046 boundary grep + ADR-0048 client.go absence checks codified in Task 16's gate sweep? (Yes — Step 9.)
