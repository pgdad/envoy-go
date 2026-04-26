# Phase 05.2 — Upstream HTTP/2 (client-side codec, `Cluster.DialH2`, `routerActionH2`, fixture 0004)

**Phase id:** `05.2`
**Slug:** `05.2-upstream-h2`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:brainstorming` (adapted autonomous mode — see `DECISIONS.md` ADR-0004)
**Depends on:** phase 05.1 (done at master `bc4fca4`)
**Parent phase:** `05-http-2` (in-progress; split into `05.1` + `05.2` per ADR-0045; this sub-phase closes the parent on phase-done)
**Master design document:** `docs/envoy-go/phases/05-http-2/SPEC.md` (commit `612cdea`) — phase 05.2 carves the remaining slice of its §4 deliverables (the slice 05.1 explicitly deferred per its §2.2). The master SPEC remains authoritative for cross-cutting design (codec choice rationale, RFC compliance, equivalence shape); the 05.1 SPEC at `docs/envoy-go/phases/05.1-downstream-h2/SPEC.md` is authoritative for the server-side codec internals 05.2 builds on top of.
**Differential surface at end of sub-phase:** NEW fixture `test/fixtures/0004-h2-routing/` is differentially green (gate (a) is **non-vacuous** for the first time on the H2 surface): full-stack HTTPS h2 between the proxy and a STATIC cluster of three upstream h2 backends, 27 sequential requests per side (9 × `/health` direct_response 200 / 9 × `/api/v1/<n>` router-action / 9 × `/missing/<n>` direct_response 404), `:status` per-request equivalence, decoded-body byte-equivalence on the 9 `/health` responses, per-cluster RR distribution `[3, 3, 3]` per side over the 9 router-action requests, 404 status equivalence on the 9 `/missing` requests against upstream Envoy v1.37.2. Pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing` remain green (gate (b)). The h2spec conformance gate (c) — newly non-vacuous in 05.1 at 53/53 PASS against the ADR-0051 pin — remains 53/53 PASS against this sub-phase's HEAD (the flow-control ADR-0055 extends the threshold language but does NOT add new section requirements; per-section pass counts stay at the 05.1 baseline). Fuzz (d), build/vet/lint/test (e), REVIEW (f) apply normally. ROADMAP row `05.2` flips `planned → in-progress` at the SPEC commit and `→ done` at the phase-done commit; on the same phase-done commit, the parent row `05-http-2` flips `in-progress → done`.

---

## 1. Purpose

Phase 05.2 closes envoy-go's HTTP/2 dataplane on the upstream-origination side: the cluster manager learns to read `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]`, the cluster builder validates the H2-cluster invariants (TLS transport_socket + ALPN h2), `Cluster.DialH2(ctx)` returns a fresh `*h2.ClientConn` per call (per-request fresh dial, mirroring phase-04's H1 ADR-0039 — pooling deferred to the upstream-robustness family per master SPEC §2), the new `ClientConn` + `RoundTrip` in `internal/filter/hcm/h2/client.go` reuses the 05.1 codec primitives (framer, hpack, flow control, settings, errors) on the client side of the conn-state-machine, and the new `routerActionH2` action variant in `internal/filter/hcm/actions.go` dispatches a downstream H2 stream's request through the upstream H2 client and writes the response back through the 05.1 server-side stream-write helpers. With this lands the project's first full-stack H2 differential fixture (`0004-h2-routing`), closing the H2 leg of ADR-0035 (the carry-forward of "fixture-0003 still does not differentially exercise upstream TLS" from phase-04 REVIEW); the H1+TLS upstream gap remains open and is carried forward unchanged.

Concretely, phase 05.2 produces:

1. **`internal/filter/hcm/h2/client.go`** — the from-scratch client-side H2 connection manager that the 05.1 codec sub-package explicitly omitted (per 05.1 SPEC §2.2 + ADR-0048). `NewClientConn(ctx, upstream net.Conn) (*ClientConn, error)` writes the client preface (`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n`), exchanges initial SETTINGS, and returns a usable conn. `(*ClientConn).RoundTrip(ctx, req H2Request) (H2Response, error)` allocates a single odd-numbered client-initiated stream (per RFC 9113 §5.1.1 — the conn's monotonic stream-id allocator starts at 1 and increments by 2), writes HEADERS+DATA+END_STREAM (or HEADERS-only with END_STREAM for bodyless requests), reads the response HEADERS+DATA+END_STREAM, returns the assembled `H2Response`. Per ADR-0056 (anticipated, see §4.4), 05.2 uses the `ClientConn` for exactly one round-trip per instance (per-request fresh conn); the conn supports multi-RT in principle but the router does not exploit it. `(*ClientConn).Close()` emits a graceful GOAWAY with `last-stream-id` followed by a TCP-level FIN — symmetric to the 05.1 server-side close. The package layout is unchanged from 05.1 (`conn.go`/`stream.go`/`framer.go`/`hpack.go`/`flow.go`/`preface.go`/`settings.go`/`errors.go`/`client.go`); `client.go` is the only **new** file in the package.

2. **`internal/cluster/dial_h2.go`** — `Cluster.DialH2(ctx) (*h2.ClientConn, error)`. Calls `Cluster.Dial(ctx)` (existing phase-03 helper); type-asserts the returned conn to `*stdtls.Conn` (errors `cluster: dial h2: not a TLS conn` if not — H2 over plaintext is out-of-scope per 05.1 §2.1 and master SPEC §2); inspects `ConnectionState().NegotiatedProtocol` (errors `cluster: dial h2: alpn negotiated %q, want "h2"` if not h2); wraps the conn via `h2.NewClientConn(ctx, raw)`; returns. Failure on any of the three steps surfaces a typed error that `routerActionH2.do` translates into a downstream 502 local-reply.

3. **`internal/cluster/manager.go`** (extended) — config builder learns to read `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]` (the standardised cluster-side H2-config carrier on the v3 proto), peeks at the `explicit_http_config` discriminator, sets a `useH2 bool` flag on the resulting `*Cluster`. Validation: if `useH2 == true`, the cluster's `transport_socket` MUST be present, MUST be type `envoy.transport_sockets.tls`, and the parsed TLS config's `alpn_protocols` MUST include `"h2"`; otherwise build-time error. The manager exposes `Cluster.UseH2() bool` for the HCM filter-build-time variant selection. All other fields of `HttpProtocolOptions` (every `common_http_protocol_options` field, `auto_config`, `upstream_http_filters[]`, `connection_pool_per_downstream_connection`, every inner field of `explicit_http_config.http2_protocol_options.{initial_stream_window_size, ..., override_stream_error_on_invalid_http_message}`, every inner field of `explicit_http_config.http_protocol_options`) are silently ignored at phase 05.2 (the cluster advertises the same hardcoded SETTINGS as the server-side uses, per ADR-0047) — recorded in §9 below as the 05.2 amendment to the silently-ignored set.

4. **`internal/cluster/cluster.go`** (extended) — `Cluster.UseH2() bool` accessor added; the existing `Cluster.Dial(ctx)` is unchanged. A blank import `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` is added so `protojson` can round-trip the `HttpProtocolOptions` extension at unit-test time. The blank import is also added to `internal/bootstrap/bootstrap.go` so fixture-0004's bootstraps round-trip cleanly through the loader.

5. **`internal/filter/hcm/actions.go`** (extended) — new `routerActionH2` action variant alongside the phase-04 `routerAction` (which 05.1 left unchanged) and the codec-neutral `directResponseAction` (which 05.1 refactored). Build-time choice: at `NewFilter` time, the route's resolved cluster's `UseH2()` is checked; if true, a `routerActionH2{cluster *cluster.Cluster}` is constructed; if false, the existing phase-04 `routerAction{cluster *cluster.Cluster}` is constructed. `routerActionH2.do(ctx, req H2Request, w H2ResponseWriter) error`:
   - Calls `r.cluster.DialH2(ctx)`; on error → write a 502 local-reply via the H2 stream writer (HEADERS `{:status 502, content-type text/plain, server envoy, content-length …, date …}` + DATA `bad gateway\n` + END_STREAM) and return nil. The 502 prose body matches the H1 path's wording byte-for-byte except for the framing.
   - `defer clientConn.Close()` (per-request fresh conn per ADR-0056; close emits graceful GOAWAY).
   - Calls `clientConn.RoundTrip(ctx, req)`; on H2 protocol error → 502 local-reply; on context cancellation → emit RST_STREAM(CANCEL) on both the upstream and downstream streams and return nil.
   - Writes the upstream response back through `w`: `w.Headers(resp.Headers, false)` (pseudo-headers `:status` first, then regular headers in deterministic order), copy DATA frames, end-stream marker on the final DATA frame (the planner picks DATA-with-END over trailing-HEADERS for response close per master SPEC §5.3 step 4 and ADR-0058 — both are valid per RFC 9113 §8.1; DATA-with-END is the simpler shape). Trailers received from the upstream are observed by the codec but discarded by the action (per ADR-0058 — see §2.1 trailer rule inherited from 05.1 §2.1; the rationale ADR formalises this for the upstream side).
   - Returns nil on success; returns an `*h2.Error` only if the *downstream* stream write fails (in which case the per-stream goroutine in 05.1's `serverStream.dispatch` emits RST_STREAM(INTERNAL_ERROR) on the downstream and the conn keeps running).

6. **A flow-control discipline tightening of the 05.1 codec primitives** (per the 05.1 REVIEW Important findings I-1/I-2/I-3 and Minor M-3/M-9/M-11; ADR-0055). The tightening applies to the *server-side* codec primitives (which 05.2 reuses on the client side) and is the load-bearing prerequisite for the upstream-H2 surface to handle realistic response bodies (>16 KB) and tight peer flow-control windows without protocol violations. Concretely:
   - **I-1 fix:** `ServerConn.writeData` (and the new `ClientConn.writeData`, which shares the implementation) caps each outgoing DATA chunk at `min(conn-window-available, stream-window-available, peer.MaxFrameSize)`. The peer's `MaxFrameSize` is read from the SETTINGS the peer sent (defaulting to 16384 if not yet received). Regression test: a >16384-byte body with the peer advertising `MaxFrameSize: 16384` produces ≥2 DATA frames on the wire, no peer-side `FRAME_SIZE_ERROR`.
   - **I-2 fix:** the `writeData` inner loop now reserves against BOTH the connection-level `sendW` AND the per-stream `sendW` before writing each chunk; both windows are debited atomically. Regression test: `INITIAL_WINDOW_SIZE: 16` + 100-byte response body produces ~7 DATA frames before completion, no `FLOW_CONTROL_ERROR`.
   - **I-3 fix:** receive-side flow control is now enforced. On every inbound DATA chunk, `ServerConn.recvW` and `serverStream.recvW` are decremented by the chunk's length. Once the cumulative debit on either window crosses a half-window high-water threshold, the codec emits `WriteWindowUpdate(0, n)` (conn-level) and/or `WriteWindowUpdate(streamID, n)` (stream-level) to replenish. Regression test: a stream with a body >65 KB completes (no deadlock); a connection with cumulative inbound DATA >65 KB across multiple streams stays alive.
   - **M-3 fix:** the `waitFor`+`reserve` pair on the `window` flow-control primitive is collapsed into a single mutex-guarded operation `reserveBlocking(ctx, max int32) (taken int32, err error)`. The dead `if taken <= 0` recovery branch in `writeData` is deleted. Regression test (race-detector): concurrent multi-stream writes against a window primed at boundary values produce no over-reservation.
   - **M-9 fix:** `serverStream.recvWindowUpdate` and `ServerConn.onWindowUpdate` bounds-check the addition against `2³¹ - 1`; on overflow the codec emits `RST_STREAM(FLOW_CONTROL_ERROR)` (stream-scoped) or `GOAWAY(FLOW_CONTROL_ERROR)` (conn-scoped). Regression test: a sequence of WINDOW_UPDATE frames totalling > 2³¹ - 1 triggers the correct error code.
   - **M-7 disposition:** kept-and-consumed under I-3 (the `recvW` fields are no longer dead — they're read on every inbound DATA chunk and replenished on the half-window threshold).
   - **M-11 fix (folded into the flow-control ADR's stream-state hardening):** `serverStream.recvData` checks the stream state BEFORE appending to `s.reqBody`, not after. One-line reorder; eliminates a memory-waste path on closed streams.
   - The flow-control ADR (ADR-0055) extends `BEHAVIOR_CONTRACT.md ## HTTP/2`'s `## h2spec threshold` paragraph with non-default `MaxFrameSize` and tight-window language but does NOT add new h2spec section requirements; per-section pass counts at the ADR-0051 pin remain at the 05.1 baseline.

7. **A new differential fixture `test/fixtures/0004-h2-routing/`** — first full-stack H2 fixture; first fixture exercising HTTPS h2 end-to-end (proxy ↔ backend). Contents enumerated in §4.3. Workload (mirror of fixture 0003 per master SPEC §5.10 + 05.1 SPEC's deferred enumeration): 27 sequential requests per side via the differential runner (subject + reference each receive 27 H2 requests; per-side `[3, 3, 3]` distribution over the 9 router-action requests; per-request body equivalence on the 9 direct-response 200s; per-request status equivalence on all 27).

8. **A `test/helpers/h2.go` H2RoundTrip helper** for the fixture-0004 driver — driver-side use of `golang.org/x/net/http2.Transport` is permitted (D-3.2 governs runtime, not test code, per 05.1's `cmd/envoy-go/main_test.go` precedent).

9. **A `BEHAVIOR_CONTRACT.md ## HTTP/2` extension** — the 05.1 SCAFFOLD (per ADR-0052's authorisation for in-place edits) flips its "Does not yet apply to" entries for routed-to-upstream H2 to "Applies to (05.2)", adds the routed-to-upstream rules (verbatim `:method`/`:path`/`:scheme`/`:authority` forwarding, per-cluster RR distribution `[3, 3, 3]` per side on H2, ALPN selection equivalence at the differential level, fixture-0004 surface), and closes ADR-0035's H2 leg via fixture 0004's full-stack HTTPS h2 (per ADR-0057, anticipated).

10. **Phase-05.1 REVIEW Minor carry-forward triage (M-4/M-5/M-8/M-10/M-12 + the integration-test coverage gap discovered during the 05.1 follow-up batch).** The carry-forward dispositions land in 05.2 per the 05.1 REVIEW.md `Recommendation` Path A consequences (see §12 below). M-3/M-7/M-9/M-11 are absorbed into ADR-0055. M-5 (`translateFramerErr` helper extraction) is absorbed into ADR-0055 as a cosmetic prerequisite. M-4/M-8/M-10/M-12 carry forward to later phases per per-finding disposition (§12). The integration-test coverage gap for the monotonic-id-reuse rejection branch (`internal/filter/hcm/h2/conn.go:308-319`) lands as a small test-addition task in 05.2's PLAN.

After phase 05.2, the project has proven the second half of its sixth central engineering claim: *envoy-go originates upstream HTTP/2 over TLS — it dials, confirms ALPN, runs an own client connection manager, opens streams under flow-control discipline, multiplexes (capability — not exercised by the fixture, which uses per-request fresh conns per ADR-0056), and produces a structurally-equivalent end-to-end framing surface plus per-stream behaviourally-equivalent responses to upstream Envoy on a deterministic full-stack HTTPS h2 workload.* Phase 05's parent ROADMAP row flips to `done` at this commit; the project advances to phase 06 (observability-baseline) at lifecycle-state 1.

## 2. Non-purposes

Phase 05.2 does **not** do any of the following. Most are inherited verbatim from the master phase-05 SPEC §2 / 05.1 SPEC §2.1; a few are scope-narrowings introduced by the 05.1/05.2 split per ADR-0045 that 05.2 explicitly *closes* (i.e. items that 05.1 deferred to 05.2 and 05.2 now delivers — those are listed in §1, not here). Each non-purpose is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

### 2.1 Inherited from master phase-05 SPEC §2 / 05.1 SPEC §2.1 (no change)

- **HTTP/3 / QUIC.** Both HCM `codec_type: HTTP3` and any QUIC transport socket continue to error at build, unchanged from phases 04 / 05.1. → HTTP/3 + QUIC family.
- **HTTP/2 server push (`PUSH_PROMISE`).** Phase 05.2's H2 client NEVER emits `PUSH_PROMISE` (clients can't legally send these), and the SETTINGS handshake from 05.1's `ClientConn` advertises `SETTINGS_ENABLE_PUSH = 0` to disable push from the upstream side. h2spec section 6.6 remains excluded from the conformance threshold per ADR-0051.
- **HTTP/2 PRIORITY-driven scheduling.** RFC 9113 deprecated PRIORITY. Phase 05.2's `ClientConn` reads PRIORITY frames per RFC 9113 §6.3 and silently discards them (mirroring 05.1's `ServerConn`). The advertised `SETTINGS_NO_RFC7540_PRIORITIES = 1` informs upstream peers of this.
- **Adaptive flow-control / BDP estimation.** Phase 05.2's flow-control implements the RFC 9113 §5.2 baseline + ADR-0055's discipline tightening (outbound `MaxFrameSize` chunking, per-stream send-window enforcement, inbound WINDOW_UPDATE emission, overflow bounds-check) only. NO dynamic resizing of `SETTINGS_INITIAL_WINDOW_SIZE` after the handshake; NO BDP estimation; NO adaptive window growth. Hardcoded initial windows: connection-level 65535 (protocol default), per-stream 65535 (protocol default — phase 05.2 does NOT advertise a larger window). The replenishment policy is the half-window high-water threshold; this is a static policy, not adaptive. → upstream-robustness family or a dedicated perf phase.
- **Upstream H2 stream pooling / multiplexing across requests.** `Cluster.DialH2` returns a *fresh* `*h2.ClientConn` per upstream request; within a single `routerActionH2.do`, exactly one stream is opened on the new conn and the conn is closed immediately after the response is consumed. The multiplexing benefit of H2 is intentionally unrealised on the upstream side at phase 05.2; pooling lands with the upstream-robustness family (which also covers H1 pooling). Per ADR-0056, the phase-05.2 differential surface does not assert pool/non-pool — Envoy pools, envoy-go does not, both produce the same per-request `:status`/body output, both produce per-side `[3, 3, 3]` distribution under the sequential-request workload, the cross-conn frame counts differ but those are not in the equivalence matrix.
- **Trailer support (request and response).** Phase 04 set `req.Trailer = nil` after `http.ReadRequest` (stdlib H1 limitation); 05.1's downstream H2 path observes trailing-HEADERS frames per RFC 9113 §8.1 but discards them; 05.2's upstream H2 path observes them on the upstream conn but the router action discards them in both directions. The router emits END_STREAM via DATA-with-END, never via trailing HEADERS. The fixture-0004 driver does not exercise trailers. ADR-0058 (anticipated) records the trailer rationale formally for 05.2 (it was carried forward in 05.1 §2.1 but the formal ADR landed in neither phase yet because the upstream-side surface is required to make it concrete). → phase 07 framework + gRPC family.
- **gRPC-specific behaviour.** No `grpc-status` translation, no gRPC-Web bridging, no `grpc-timeout` honouring. The phase-05.2 differential surface is plain H2 routing only (`/api/v1/<n>` paths return text bodies, not gRPC frames). → gRPC family.
- **0-RTT (TLS 1.3 early data).** crypto/tls supports 0-RTT only via TLS sessions and explicit opt-in; phase-05.2 does not opt in. → later phase if ever.
- **HTTP/1.1 → HTTP/2 upgrade ("h2c upgrade").** Phase-04's HCM rejects `Upgrade: h2c` request headers with 501; 05.1 did not change that; 05.2 does not change that. The h2c-prior-knowledge surface is exercised ONLY by `test/conformance/h2spec/` via `--allow-h2c` (05.1 deliverable, unchanged in 05.2); no fixture asserts h2c equivalence.
- **HCM `tracing`, `access_log[]`, `http_protocol_options` (deprecated direct field), `common_http_protocol_options`, `server_header_transformation`, `local_reply_config`, `internal_redirect_policy`, `request_id_extension`, `path_with_escaped_slashes_action`, `merge_slashes`, `xff_num_trusted_hops`, `via`, `proxy_100_continue`, `stream_idle_timeout`, `request_timeout`, `request_headers_timeout`, `drain_timeout`, `delayed_close_timeout`, `forward_client_cert_details`, `original_ip_detection_extensions`.** All silently ignored, unchanged from phases 04 / 05.1 (per ADR-N + its 05.1 amendment).
- **Stats, access logs, tracing, runtime overrides.** All deferred. → phase 06.
- **HTTP filters other than `[router]`.** Unchanged from phase 04 (per ADR-0042). → phase 07.
- **Per-route filter config.** Unchanged from phase 04. → phase 07.
- **Route match predicates beyond `prefix` and `path`.** Unchanged from phase 04 (per ADR-0038). → phase 07.
- **Multi-vhost matching.** Unchanged from phase 04. → phase 07.
- **Filter-chain matching beyond ALPN-derived codec selection.** Phase 03's SNI-keyed filter-chain match plus phase 04's empty-match plaintext rule continue to apply. ALPN remains *not* a `filter_chain_match` field at phase 05.2 — codec selection happens *inside* the HCM filter (per ADR-0050 from 05.1). The `filter_chain_match.application_protocols[]` field is silently ignored if present. → phase 07.
- **Cluster types other than STATIC (subject side).** Unchanged from phase 02. The fixture-0004 reference uses `STRICT_DNS` for the same reason fixtures 0001/0002/0003 do (Docker testcontainers hostname resolution per ADR-0010); the subject uses `STATIC`. → later phase.
- **LB policies other than ROUND_ROBIN.** Unchanged. The per-cluster H2 RR counter is per-`Cluster` (phase-02 ADR-0024 scope); H2 and H1 dials on the same cluster would share a counter, but fixture 0004's cluster is H2-only so this question doesn't surface. → load-balancing family.
- **Graceful drain of in-flight HTTP/2 requests on shutdown.** SIGINT behaviour unchanged from phase 04 / 05.1: listener sockets close, in-flight conns drop. Neither the H2 server (`ServerConn`) nor the H2 client (`ClientConn`) emits a graceful GOAWAY tied to a SIGINT signal — only the per-request `defer clientConn.Close()` in `routerActionH2.do` emits a GOAWAY (and that one is per-conn, post-RoundTrip, not shutdown-triggered). → phase 08.
- **HTTP/2 max-concurrent-streams enforcement on the upstream side.** Phase 05.2's `ClientConn` advertises `SETTINGS_MAX_CONCURRENT_STREAMS = 100` symmetrically with the server side (per ADR-0047 inherited; 05.2 does not separately ADR a different client-side default). Because 05.2 uses one stream per `ClientConn` lifetime, this setting is unexercised in production; unit tests cover the receive-side enforcement (the upstream peer can advertise its own `MaxConcurrentStreams`, which the client respects when allocating new streams — but with one stream per conn, the limit is irrelevant in practice).

### 2.2 Items 05.1 explicitly deferred to 05.2 — DELIVERED IN 05.2 (NOT a non-purpose)

These items are listed in 05.1 SPEC §2.2 as deferred-to-05.2; they are **not** non-purposes of 05.2 and are not repeated in §2.1 above. They are §1's deliverables. Listed here only for the reviewer's audit-trail of the 05.1 → 05.2 boundary closure:

- Upstream HTTP/2 origination (`internal/filter/hcm/h2/client.go`) → §1 #1.
- Upstream H2 dial helper (`internal/cluster/dial_h2.go`) and `Cluster.UseH2()` accessor → §1 #2 / §1 #4.
- Cluster `HttpProtocolOptions` parsing in `internal/cluster/manager.go` → §1 #3.
- `routerActionH2` variant in `internal/filter/hcm/actions.go` → §1 #5.
- Differential fixture `0004-h2-routing/` → §1 #7.
- `test/helpers/h2.go` H2RoundTrip helper → §1 #8.
- Blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` in `internal/cluster/cluster.go` (and in `internal/bootstrap/bootstrap.go`) → §1 #4.
- `BEHAVIOR_CONTRACT.md ## HTTP/2` upstream + fixture-0004 rules (in-place edit per ADR-0052) → §1 #9.
- ADRs deferred to 05.2: per-request fresh upstream H2 dial (ADR-0056), closes ADR-0035 H2 leg (ADR-0057), trailers observed but not forwarded (ADR-0058) → §4.4. (The 05.1 SPEC §2.2 used letters R/W/Y for these; the planner re-verifies next-free at write time and assigns ADR-0056..ADR-0058 to R/W/Y respectively.)

### 2.3 Newly out-of-scope at 05.2 (specific narrowings)

- **The H1+TLS upstream gap from ADR-0035.** Phase 05.2 closes the H2 leg of ADR-0035 via fixture 0004's full-stack HTTPS h2 (per ADR-0057). The H1+TLS upstream gap remains open after 05.2 — a future HTTPS-H1 fixture (or an extension of fixture 0003 to TLS upstream) closes the H1 leg. ADR-0057 explicitly carries the H1 leg forward with a "phase-05.2-follow-up" tag pointing at later phases (likely between 05.2 and 06, or folded into phase 07's filter-chain framework, or staying open into HTTP-filter-family phases — 05.2 does not pre-decide).
- **Mixed-codec clusters (a single cluster used by both H1 and H2 listeners).** Fixture 0004's cluster is H2-only (the `HttpProtocolOptions.explicit_http_config.http2_protocol_options` discriminator is set). Phase-04's fixture 0003 uses an H1 cluster (no `HttpProtocolOptions`, falling into the silent-ignore set). The two never share a cluster. The per-`Cluster` RR counter scope (ADR-0024) is preserved (H2 and H1 dials on the same cluster *would* share a counter, but 05.2 does not exercise that combination). → load-balancing family or a future phase explicitly adding mixed-codec clusters.
- **Upstream H2 connection re-use across `routerActionH2` invocations.** Per ADR-0056, every router-action invocation dials a fresh upstream conn. Cross-invocation pooling is the upstream-robustness family's deliverable.
- **Upstream H2 with mTLS.** Fixture 0004 uses one-way TLS (server presents cert, client trusts the fixture-local CA; client does not present a cert). The cluster's TLS config does not set `validation_context.match_typed_subject_alt_names` for client-cert auth; the upstream backend's TLS config does not require client certs. → mTLS phase or later.
- **Upstream H2 with HTTP/2 settings tuning per cluster.** Every inner field of `HttpProtocolOptions.explicit_http_config.http2_protocol_options.{initial_stream_window_size, initial_connection_window_size, max_concurrent_streams, hpack_table_size, allow_metadata, allow_connect, max_outbound_frames, max_outbound_control_frames, max_consecutive_inbound_frames_with_empty_payload, max_inbound_priority_frames_per_stream, max_inbound_window_update_frames_per_data_frame_sent, stream_error_on_invalid_http_messaging, override_stream_error_on_invalid_http_message}` is silently ignored (advertised settings are the hardcoded ADR-0047 defaults regardless of config). → upstream-robustness family or a perf phase.

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for 05.2)

Per doctrine `D-3.6`, phase 05.2 lands only when every gate below is green. The generic six-gate set is narrowed:

| Gate | Specialization for phase 05.2 |
|---|---|
| (a) new/changed differential fixtures green | **Non-vacuous (first time on the H2 surface).** New fixture `test/fixtures/0004-h2-routing/` passes: 27 H2 requests per proxy (9 `/health` + 9 `/api/v1/<n>` + 9 `/missing/<n>`); `:status` equivalence per request on both sides; decoded body equivalence on the 9 `/health` direct-response requests (200 + body `OK\n`); 404 status equivalence on the 9 `/missing` requests (body relaxed under the phase-04 framing-divergence rule extended for H2 local-reply prose); per-cluster RR distribution `[3, 3, 3]` on EACH SIDE over the 9 `/api` requests (9 sequential streams; 3 per backend per side; same local-correctness property as fixtures 0001/0002/0003 per ADR-0024). Driver opens a fresh `*http2.ClientConn` per request via `golang.org/x/net/http2.Transport` over TLS with the fixture's pinned PKI and `NextProtos: ["h2"]` (driver-side use OK; runtime use forbidden per D-3.2). |
| (b) all pre-existing differential fixtures still green | `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing` all pass without regression under their existing `expectations.yaml`. The 05.1 codec primitives (framer, hpack, flow control, settings, errors, server-side stream/conn) are touched by ADR-0055's flow-control discipline tightening — the changes are additive (new code paths for outbound chunking, per-stream send-window debiting, inbound WINDOW_UPDATE emission) and do not regress the 05.1 server-side surface; verified by re-running the existing `internal/filter/hcm/h2/` unit tests + h2spec gate (c) below. |
| (c) conformance suites pass | `test/conformance/h2spec/` runs the upstream `summerwind/h2spec` image (still pinned at the ADR-0051 SHA `summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0` — 05.2 does NOT bump the pin; pin bumps require their own phase per D-3.7) against the same 05.1 `--allow-h2c` h2c listener and reports `failed == 0` over the same threshold section list (3, 4, 5, 6 ex-6.6, 7, 8). 53/53 PASS at the 05.1 baseline; the flow-control tightening must NOT regress this. The ADR-0055 threshold-language extension is *prose* clarification, not new section requirements. |
| (d) new/existing fuzzers run clean for CI short-budget | The 05.1 fuzz targets `internal/filter/hcm/h2.FuzzFrameStream` and `internal/filter/hcm/h2.FuzzHPACKDecode` continue to run clean for their CI short-budget runs (30-second policy per ADR-0018). Phase 05.2 does NOT introduce a new fuzz target — the upstream-H2 surface (client preface, client SETTINGS exchange, RoundTrip request encoding, response decoding) is exercised by the existing `FuzzFrameStream` (which mutates frame sequences regardless of conn role) plus the fixture-0004 differential. A dedicated upstream-side fuzzer is deferred to a future hardening phase. The phase-01..05.1 fuzzers (`FuzzBootstrapLoad`, `FuzzTcpProxyFilter`, `FuzzTLSContextParse`, `FuzzHCMConfigParse`, `FuzzFrameStream`, `FuzzHPACKDecode`) all run clean (no regression). |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | Standard. Unit tests for `internal/filter/hcm/h2/client.go` (client-preface emit, client-side SETTINGS exchange, `RoundTrip` happy path, `RoundTrip` ctx cancel, `RoundTrip` upstream RST_STREAM, `RoundTrip` upstream GOAWAY, `Close` graceful GOAWAY emit) plus extended tests for `internal/cluster/` (`DialH2` happy path, ALPN-mismatch error, not-TLS error, dial-timeout error; `HttpProtocolOptions` parsing happy path; `HttpProtocolOptions` cluster without TLS → build error; `HttpProtocolOptions` cluster with TLS but ALPN lacks `h2` → build error; `HttpProtocolOptions` with `auto_config` → silently ignored at 05.2 — the build does NOT error per the 05.2 silent-ignore rule, deviating from the master SPEC §5.8 prose which suggested erroring; rationale documented in §5.5 of this SPEC, no separate ADR per the small-mechanical-design-decision precedent — the SPEC document itself is the durable D-3.4 record) plus extended tests for `internal/filter/hcm/actions_test.go` (`routerActionH2` build-time variant selection by `Cluster.UseH2()`; `routerActionH2.do` happy path against an in-process h2 backend; 502 local-reply on dial failure; 502 local-reply on RoundTrip protocol error; ctx cancellation emits RST_STREAM(CANCEL)). Plus the new integration test for the monotonic-id-reuse rejection branch (`internal/filter/hcm/h2/conn.go:308-319`) per the 05.1 REVIEW carry-forward — see §12. Plus the regression tests for ADR-0055's flow-control tightening listed in §1 #6. |
| (f) `REVIEW.md` approved | Per `SKILL_ROUTING.md` state 5. |

## 4. Deliverables (files and directories)

Grouped by lifecycle. Every path below is either new or materially changed in 05.2. Paths the 05.1 SPEC §4 listed as **NOT in 05.1** are this phase's deliverables; they are repeated here as the 05.2 surface.

### 4.1 New production code (in 05.2)

- **`internal/filter/hcm/h2/client.go`** — the client-side connection manager. Exports `ClientConn` (the per-upstream-conn client-side H2 connection manager; one per upstream `*stdtls.Conn` after `Cluster.DialH2` confirms ALPN h2). `NewClientConn(ctx, upstream net.Conn) (*ClientConn, error)` writes the client preface, exchanges initial SETTINGS (sends ours; reads peer's; SETTINGS_ACKs the peer's; expects SETTINGS_ACK for ours), returns a usable conn. The implementation reuses 05.1's `framer`, `hpack`, `flow`, `settings`, `errors` primitives; it adds a small client-side stream-id allocator (`nextStreamID uint32` starting at 1, incrementing by 2 — RFC 9113 §5.1.1) and a single-stream `clientStream` type analogous to 05.1's `serverStream` but with the half-closed/closed semantics inverted (the client closes its half by emitting END_STREAM on its DATA; the server closes its half by emitting END_STREAM on its response). `(*ClientConn).RoundTrip(ctx, req H2Request) (H2Response, error)` allocates a stream id, writes HEADERS+DATA+END_STREAM (or HEADERS-only with END_STREAM for bodyless requests — fixture-0004 uses bodyless GETs only), enters a per-stream read loop reading HEADERS and DATA frames until END_STREAM, returns the assembled `H2Response{Status int, Headers []hpack.HeaderField, Body []byte}`. `(*ClientConn).Close() error` emits `WriteGoAway(lastStreamID, NO_ERROR, []byte("client close"))` (per RFC 9113 §6.8 graceful-close pattern), closes the underlying conn, returns nil. Per ADR-0056, 05.2 uses `RoundTrip` exactly once per `ClientConn` instance; the conn supports multi-RT in principle (the stream-id allocator is monotonic, not reset per call) but the router does not exploit it.
- **`internal/filter/hcm/h2/client_test.go`** (or appended to existing `conn_test.go` — planner choice) — exhaustive unit tests covering: client-preface emit + SETTINGS exchange happy path; `RoundTrip` happy path (HEADERS+END_STREAM out, HEADERS+DATA+END_STREAM in); `RoundTrip` with body (HEADERS+DATA+END_STREAM out); `RoundTrip` ctx cancel during write → RST_STREAM(CANCEL); `RoundTrip` ctx cancel during read → RST_STREAM(CANCEL); `RoundTrip` peer sends RST_STREAM mid-response → returns `*h2.Error{Code: CANCEL or whatever the peer sent}`; `RoundTrip` peer sends GOAWAY mid-conn → returns `*h2.Error{Code: NO_ERROR or whatever}`; `RoundTrip` peer sends DATA after END_STREAM → returns `*h2.Error{Code: STREAM_CLOSED}`; `Close` emits GOAWAY(NO_ERROR) and closes the underlying conn cleanly; `RoundTrip` after `Close` returns an error; multi-`RoundTrip` smoke (capability check — exercises stream-id monotonicity 1, 3, 5; not exercised in production). Test peer is `golang.org/x/net/http2.Server` (driver-side use OK per D-3.2); a small in-process h2 backend is started inside the test, the `ClientConn` is built against its `net.Conn` after a TCP+TLS dial. The flow-control regression tests from ADR-0055 (>16384-byte body chunked correctly; tight INITIAL_WINDOW_SIZE drip-fed correctly; >65 KB inbound triggers WINDOW_UPDATE; overflow bounds-check) live ALONGSIDE the existing `flow_test.go` / `conn_test.go` server-side tests since the flow-control primitives are shared between `ServerConn` and `ClientConn`.
- **`internal/cluster/dial_h2.go`** — `Cluster.DialH2(ctx) (*h2.ClientConn, error)`. Implementation:
  ```go
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
      alpn := tlsConn.ConnectionState().NegotiatedProtocol
      if alpn != "h2" {
          _ = tlsConn.Close()
          return nil, fmt.Errorf("cluster: dial h2: alpn negotiated %q, want \"h2\"", alpn)
      }
      cc, err := h2.NewClientConn(ctx, tlsConn)
      if err != nil {
          _ = tlsConn.Close()
          return nil, fmt.Errorf("cluster: dial h2: client conn: %w", err)
      }
      return cc, nil
  }
  ```
  The defer-vs-explicit-close rationale: each error branch closes the conn explicitly because there's no `defer` that would catch them safely (the function returns the conn on success, where the caller takes ownership; on error there's no caller-owned conn). This mirrors the discipline 05.1's `Filter.Handle` uses for the early-return error paths in ALPN dispatch.
- **`internal/cluster/dial_h2_test.go`** — `DialH2` happy path against an in-process h2-over-TLS backend (uses `crypto/tls` + `golang.org/x/net/http2.ConfigureServer` driver-side); `DialH2` over an h2-over-TLS backend that negotiates `http/1.1` instead of `h2` (alpn-mismatch error path); `DialH2` over a plaintext backend (not-a-TLS-conn error path); `DialH2` with a cancelled context (dial-timeout error path); `DialH2` over a TLS backend that fails the handshake (TLS error path bubbles through `Cluster.Dial`'s existing error handling — phase-03 surface).
- **`internal/cluster/manager.go`** (extended) — config builder learns to read `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]`. Implementation shape:
  - At cluster-build time, before constructing the `*Cluster`, the builder peeks at the proto's `typed_extension_protocol_options` map for the `envoy.extensions.upstreams.http.v3.HttpProtocolOptions` key. If absent → `useH2 = false` (phase-04 H1 path, unchanged).
  - If present, the typed proto is unmarshalled into an `*upstreamshttpv3.HttpProtocolOptions` (via the blank-imported registration from §1 #4).
  - Discriminator: `UpstreamProtocolOptions.(type)` switch:
    - `*upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig`: switch on the inner `ProtocolConfig.(type)`:
      - `*ExplicitHttpConfig_Http2ProtocolOptions`: `useH2 = true`. All inner fields silently ignored at 05.2.
      - `*ExplicitHttpConfig_HttpProtocolOptions` (the H1 discriminator): `useH2 = false` (the cluster behaves as H1, which is the default anyway). All inner fields silently ignored. This branch is silent; no error.
    - `*upstreamshttpv3.HttpProtocolOptions_AutoConfig`: silently ignored at 05.2 — `useH2 = false` (the cluster falls back to H1 semantics). Master SPEC §5.8 originally suggested erroring here; the 05.2 SPEC narrows to silent-ignore for consistency with phase-04's silent-ignore discipline (ADR-N + its 05.1 amendment) and to preserve forward-compat as more discriminators land in later phases.
    - Nil / empty `UpstreamProtocolOptions`: silently ignored — `useH2 = false`.
  - Validation when `useH2 == true`:
    - Cluster MUST have a `transport_socket` (build error: `cluster %q: HttpProtocolOptions.http2_protocol_options requires transport_socket`).
    - The transport_socket MUST be type `envoy.transport_sockets.tls` (build error: `cluster %q: HttpProtocolOptions.http2_protocol_options requires transport_socket of type tls, got %q`).
    - The parsed TLS config's `alpn_protocols` MUST include `"h2"` (build error: `cluster %q: HttpProtocolOptions.http2_protocol_options requires alpn_protocols to include "h2", got %v`).
  - The `*Cluster` gains a `useH2 bool` field; expose via `Cluster.UseH2() bool` (added in `cluster.go` per below).
- **`internal/cluster/cluster.go`** (extended) — `Cluster.UseH2() bool` accessor returning the `useH2` field set at build time. The existing `Cluster.Dial(ctx) (net.Conn, error)` is unchanged — `DialH2` is a *separate* helper, not a method swap. Blank import added: `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"`.
- **`internal/cluster/cluster_test.go`** + **`internal/cluster/manager_test.go`** (extended) — cases:
  - H2-cluster build (positive): `HttpProtocolOptions.explicit_http_config.http2_protocol_options{}` + TLS transport_socket + `alpn_protocols: ["h2"]` → builds successfully; `UseH2() == true`.
  - H2-cluster build without TLS: `HttpProtocolOptions.explicit_http_config.http2_protocol_options{}` + no transport_socket → build error (the exact diagnostic from §4.1 above).
  - H2-cluster build with TLS but `alpn_protocols` lacks `h2`: `HttpProtocolOptions.explicit_http_config.http2_protocol_options{}` + TLS + `alpn_protocols: ["http/1.1"]` → build error.
  - H2-cluster build with TLS but no `alpn_protocols` field at all: build error (the field is required for h2 by RFC 7301).
  - H1-cluster with `HttpProtocolOptions.explicit_http_config.http_protocol_options{}` (the H1 discriminator) → silently ignored, `UseH2() == false`.
  - Cluster with `HttpProtocolOptions.auto_config` → silently ignored, `UseH2() == false`. (Per the 05.2 narrowing of master SPEC §5.8.)
  - Cluster with no `typed_extension_protocol_options` map at all → silently ignored, `UseH2() == false` (phase-04 baseline, no regression).
  - `protojson` round-trip of a fixture-0004-shaped bootstrap (with `HttpProtocolOptions` on the cluster) round-trips cleanly via the blank-imported registration.
- **`internal/filter/hcm/actions.go`** (extended) — new `routerActionH2` action variant alongside the existing phase-04 `routerAction` and the codec-neutral `directResponseAction` from 05.1. Implementation shape:
  ```go
  type routerActionH2 struct {
      cluster *cluster.Cluster
  }

  func (r *routerActionH2) do(ctx context.Context, req h2.H2Request, w h2.H2ResponseWriter) error {
      cc, err := r.cluster.DialH2(ctx)
      if err != nil {
          return r.write502(w, fmt.Sprintf("upstream dial: %v", err))
      }
      defer cc.Close()
      resp, err := cc.RoundTrip(ctx, req)
      if err != nil {
          if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
              return w.RST(h2.ErrCodeCancel)
          }
          return r.write502(w, fmt.Sprintf("upstream roundtrip: %v", err))
      }
      // Pseudo-headers first per RFC 9113 §8.3: writeH2 helper from 05.1's stream writer
      // already enforces this — reuses the codec-neutral writer if the planner picks that
      // factoring; otherwise writes pseudo-headers explicitly.
      if err := w.Headers(resp.Headers, false); err != nil {
          return err  // surface to per-stream goroutine in serverStream.dispatch
      }
      if err := w.Data(resp.Body, true); err != nil {
          return err
      }
      return nil
  }

  func (r *routerActionH2) write502(w h2.H2ResponseWriter, detail string) error {
      // Local-reply prose body; same wording as H1 path's 502 modulo framing.
      // The detail string is not exposed to the client (it's a server-side log only;
      // 05.2 does not introduce structured logging — phase 06's deliverable).
      hdrs := []hpack.HeaderField{
          {Name: ":status", Value: "502"},
          {Name: "content-type", Value: "text/plain"},
          {Name: "server", Value: "envoy"},
          {Name: "content-length", Value: "12"},
          // Date header omitted from this prose; the implementation uses the same
          // dateNowRFC7231 helper as 05.1's directResponseAction.writeH2.
      }
      _ = w.Headers(hdrs, false)
      _ = w.Data([]byte("bad gateway\n"), true)
      return nil
  }
  ```
  The route-table builder gains a per-route check: at `NewFilter` time, after resolving the route's target cluster, it inspects `cluster.UseH2()`; if true → constructs `routerActionH2{cluster}`; if false → constructs the existing `routerAction{cluster}`. The build-time choice is logged once per route per the existing phase-04 build-log discipline.
- **`internal/filter/hcm/actions_test.go`** (extended) — cases:
  - `routerActionH2` variant selection: build a route pointing at a cluster with `UseH2() == true` → resulting action is `*routerActionH2`. Build the same route pointing at a cluster with `UseH2() == false` → resulting action is `*routerAction`.
  - `routerActionH2.do` happy path: in-process h2 backend (driver-side stdlib `http.Server` with `http2.ConfigureServer`); a fake `H2ResponseWriter` captures the response; assert `:status` + decoded body match the backend's response.
  - `routerActionH2.do` 502 on dial failure: cluster pointing at a closed port; assert response is `:status 502` + body `bad gateway\n`.
  - `routerActionH2.do` 502 on RoundTrip protocol error: in-process backend that emits a malformed HEADERS frame; assert `:status 502`.
  - `routerActionH2.do` ctx cancellation: cancel the ctx mid-RoundTrip; assert RST_STREAM(CANCEL) on the downstream stream writer.
  - `routerActionH2.do` upstream returns 5xx: in-process backend returns 503; assert downstream `:status 503` + body forwarded (NOT translated to 502 — only protocol errors translate to 502; HTTP-status-level errors pass through).

### 4.2 Changed production code (in 05.2)

- **`internal/filter/hcm/h2/conn.go`** + **`flow.go`** + **`stream.go`** (extended per ADR-0055) — flow-control discipline tightening:
  - `ServerConn.writeData` (and the new `ClientConn.writeData`) caps each chunk at `min(connWindow, streamWindow, peer.MaxFrameSize)` per I-1/I-2.
  - `ServerConn.onData` (and `ClientConn`'s symmetric path) decrements `s.recvW` and `ss.recvW`; emits `WriteWindowUpdate(0, n)` and `WriteWindowUpdate(streamID, n)` once the half-window high-water threshold is crossed per I-3.
  - `flow.go`'s `window.waitFor` + `window.reserve` are collapsed into a single `window.reserveBlocking(ctx, max int32) (taken int32, err error)` per M-3.
  - `serverStream.recvWindowUpdate` and `ServerConn.onWindowUpdate` add `int31_max` overflow bounds-checks per M-9.
  - `serverStream.recvData` checks state BEFORE appending to `s.reqBody` per M-11.
  - `framer.go`'s duplicated `http2.ConnectionError`/`StreamError`/`ErrFrameTooLarge` translation block is extracted into `translateFramerErr(err) error` per M-5 — cosmetic prerequisite for the new `ClientConn`'s framer wrapper to share the same translation.
- **`internal/filter/hcm/filter.go`** (extended) — `NewFilter` learns to read each route's target cluster's `UseH2()` accessor and pick the action variant. The ALPN dispatch in `Filter.Handle` is **unchanged from 05.1** (per ADR-0050; the dispatch is downstream-side only — the upstream-side codec choice is per-route, not per-conn).
- **`internal/filter/hcm/connection.go`** (the H1 driver) — UNCHANGED in 05.2. The H1 driver continues to invoke `routerAction.do` for H1-cluster routes; the `routerActionH2` is reachable only from the H2 stream dispatch in `internal/filter/hcm/h2/stream.go`'s `dispatch` helper, which already knows how to invoke an `actionH2`-shaped action via the existing codec-neutral action interface.
- **`internal/cluster/cluster.go`** + **`internal/cluster/manager.go`** — see §4.1 (extended in 05.2).
- **`internal/bootstrap/bootstrap.go`** — adds the blank import `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` (carries the `HttpProtocolOptions` proto) so `protojson` can round-trip 05.2-fixture bootstraps. Per ADR-0016's amendment policy, this addition is documented in PROGRESS, not a new ADR.
- **`internal/listener/manager.go`** — UNCHANGED in 05.2. The 05.1 listener-manager change (`NewManagerWithBaseDirAndAllowH2C` + `listenerCtx{hasTLS, allowH2C}` plumbing) is sufficient; 05.2 does not introduce listener-side H2 changes (the upstream dial is per-cluster, not per-listener).
- **`internal/filter/tcpproxy/`** — unchanged.
- **`internal/tls/`** — unchanged. The phase-03 `alpn_protocols` plumbing already covers what 05.2 needs on the cluster side (the cluster's `transport_socket` + `alpn_protocols` use the same parsing as the listener's, per the phase-03 design).
- **`cmd/envoy-go/main.go`** — UNCHANGED in 05.2. The `--allow-h2c` flag from 05.1 stays; no new CLI surface is needed (the upstream dial is bootstrap-config-driven, not CLI-driven).
- **`cmd/envoy-go/main_test.go`** — UNCHANGED in 05.2 OR optionally extended with a fifth bootstrap variant exercising an H2 listener + H2 cluster + `routerActionH2` end-to-end with an in-process h2 backend (planner choice — the fixture-0004 differential covers the same surface; the smoke test is redundant). Recommendation: skip the smoke addition to keep the diff focused; the differential is the integration check.

### 4.3 New harness and fixture code (in 05.2)

- **`test/differential/runner_test.go`** (or adjacent files) — call sites updated for fixture 0004's blank import; if the planner adopts the optional `H2Expectations` extension on the `Driver` interface (per master SPEC §4.3 + §10 #3, deferred to 05.2 per the master SPEC's "planner picks final shape"), the runner's per-fixture loop calls it after `DriveSubject`/`DriveReference`. **Recommendation: do NOT add the `H2Expectations` extension**; the fixture-0004 driver issues + asserts in-band, mirroring fixture 0003's style. Rationale: smaller surface, less new harness machinery, easier to reason about for the reviewer; the per-fixture in-band assertion pattern is established. The planner records the choice in PLAN.md.
- **`test/fixtures/0004-h2-routing/`** — new fixture directory. Contents:
  - **`envoy-go.yaml`** — subject bootstrap. 1 listener (`l_h2`) binding `127.0.0.1:0` with a `transport_socket: tls` carrying a self-signed cert pair (regenerated at fixture build time, mirroring fixture-0002's `pki/gen/` pattern) and `alpn_protocols: ["h2", "http/1.1"]`. 1 filter chain with empty `filter_chain_match`. 1 HCM network filter with `codec_type: AUTO` so ALPN drives codec selection per-connection (the driver advertises only `h2` so the H2 path is exercised exclusively). Same routes as fixture 0003 (`/health` direct_response 200, `/api` → cluster `c_h2_backend`, `/missing` direct_response 404). 1 STATIC cluster `c_h2_backend` with three TLS upstream endpoints, each carrying a `transport_socket: tls` with `alpn_protocols: ["h2"]` and a `validation_context` pointing at the same fixture-local CA. The cluster has `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"] = {explicit_http_config: {http2_protocol_options: {}}}` (empty inner — every inner field is silently ignored at 05.2 per §1 #3).
  - **`envoy.yaml`** — reference bootstrap. Same listener shape, same HCM, same routes. 1 STRICT_DNS cluster `c_h2_backend` pointing at `host.docker.internal` × three TLS ports with `dns_lookup_family: V4_ONLY` per ADR-0010; same `transport_socket` + `HttpProtocolOptions` shape as the subject. The HCM `route_config` is identical between the two bootstraps (verbatim, modulo cluster.address differences). The reference is invoked with `--concurrency 1` per ADR-0028.
  - **`expectations.yaml`** — prose description of the 27-request workload (mirroring fixture 0003's prose form per the M-6 carry-forward decision). Allow-list lines enumerated for `Date`, `Server`, `x-envoy-*`, `x-forwarded-*`, `x-request-id`, plus the H2-pseudo-header presence rule (`:status` required + asserted; `:method`/`:path`/`:scheme`/`:authority` required + asserted on the upstream-side preservation surface — verified via in-process backend assertions on the headers received).
  - **`README.md`** — explains the fixture's purpose (HCM H2 dispatch + route match + router H2 + direct_response H2 + closes ADR-0035 H2 leg), the STATIC-vs-STRICT_DNS divergence (same as 0001/0002/0003), the ALPN-h2 e2e shape, the upstream-TLS-now-exercised closure of ADR-0035 H2 leg, the `--concurrency 1` reference pin inherited from ADR-0028, the `[3,3,3]` per-side RR distribution rule (local-correctness only; cross-side sequence is NOT asserted, mirroring phase-04's relaxation per ADR-K).
  - **`pki/gen/main.go`** — port of fixture-0002's `pki/gen/` PKI generator emitting CA + leafs for the listener's server cert AND for the H2 backend's three endpoints. Run-time: `go generate ./test/fixtures/0004-h2-routing/...`. The leafs use the SAN `localhost` + `127.0.0.1` + `host.docker.internal` so both subject (loopback) and reference (testcontainers Docker bridge → host) trust the same chain.
  - **`pki/*.pem`** — generated artefacts; committed (mirroring fixture 0002).
  - **`driver/driver.go`** — `BackendCount() = 3`. `SubjectListenerName() = "l_h2"`. `ReferenceListenerPort() = 15004`. `ReferenceBootstrap(backendPorts)` and `SubjectConfig` render the YAMLs with the three backend ports. `DriveReference(ctx, addr)` / `DriveSubject(ctx, addr)`: each issues 27 H2 requests against the proxy (the proxy listener is HTTPS h2; client uses `golang.org/x/net/http2.Transport` with the fixture CA in `RootCAs` and `NextProtos: ["h2"]`):
    - 9 × `GET /health` → expects `:status 200`, body `OK\n`.
    - 9 × `GET /api/v1/<n>` n=0..8 → expects `:status 200`, body `backend-<idx>:v1/<n>` for the picked backend (RR index per side; per-request body equivalence is NOT asserted across sides, mirroring phase-04's relaxation per ADR-K — only `:status` and per-side distribution).
    - 9 × `GET /missing/<n>` → expects `:status 404`, body relaxed.
    The 27 requests are issued sequentially; each request opens a fresh `*http2.ClientConn` (transport pooling disabled on the driver: `Transport.DisableKeepAlives = true` AND each request uses a fresh `Transport` to be doubly sure no client-side pool re-uses a conn — the fixture-0001 + fixture-0003 driver uses an analogous discipline). This keeps the fixture's per-side RR distribution deterministic (each request → one upstream-conn from the proxy → one backend pick).
    `AssertDistribution(refCounts, subjCounts [3]uint64) error` checks each side's `c_h2_backend` distribution is exactly `[3, 3, 3]` over the 9 router-action requests. Returns the concatenated status-code bytes + decoded-body bytes from all 27 requests.
    `ProbeAdmin` — same as phase 02 / 03 / 04. Admin remains H1 in phase 05.2 (no admin-on-H2 surface).
  - **`driver/driver_test.go`** — distribution-assertion helper unit test (mirror of fixture 0003's). Exercises the `[3,3,3]`-vs-`[4,3,2]` discrimination edge case.
  - **`backends/main.go`** — small Go program that starts an HTTPS h2 echo server on a configurable port, reading cert paths + an instance-id from flags / env vars. Implementation: `net.Listen("tcp", ":port")`; `tls.NewListener` with the loaded cert + `NextProtos: []string{"h2"}`; `&http.Server{Handler: echoHandler, TLSConfig: tlsConfig}`; the server uses Go's `http2.ConfigureServer` (driver-side use OK) to enable H2 on the stdlib `http.Server` — this is *test backend*, not envoy-go runtime. The handler writes `backend-<idx>:<path-suffix>` as the response body where `<idx>` is the backend's instance id (env var) and `<path-suffix>` is the path component after the last `/api/v1/` (so `/api/v1/3` → `v1/3` → response body `backend-N:v1/3`).
- **`test/helpers/h2.go`** + **`h2_test.go`** — `H2RoundTrip(ctx context.Context, addr string, tlsConf *tls.Config, method, path string, headers []hpack.HeaderField, body []byte) (status int, respHeaders []hpack.HeaderField, respBody []byte, err error)`: dials TLS, opens an h2 client conn (using `golang.org/x/net/http2.Transport` on the driver side), issues one request, returns the response. Used by the 0004 driver. Returns the body separately from the response object because callers want raw bytes for byte-compare. Test peer: an in-process h2 server in the test file.

### 4.4 Changed documentation and state (in 05.2)

- **`docs/envoy-go/ROADMAP.md`** — row 05.2: `status: planned → in-progress` flipped at the SPEC commit (per the corrected pattern from phase 05's `8d18320` and 05.1's SPEC commit, recorded in `BOOTSTRAP_PROMPT.md` §4.1 invariant 3); transitions to `done` at the 05.2 phase-done commit. Row 05 (parent): `in-progress → done` AT THE SAME phase-done commit (the parent only moves to `done` once both 05.1 AND 05.2 are `done`; 05.1 is already `done` per `bc4fca4`, so 05.2's phase-done commit closes both rows). Row 06: stays `planned` until phase 06's brainstorm enters; 06's `depends-on` is `05`, which is now satisfied.
- **`docs/envoy-go/STATE.md`** — updated at each lifecycle transition (SPEC drafted = state 2 candidate; PLAN written = state 3; impl complete = state 4; verified = state 5; reviewed = state 6 → phase 06 entry at lifecycle-state 0 OR 1 depending on whether 06's row already exists in ROADMAP — it does, per the 00-bootstrap seed, so state 1).
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** (extended in-place per ADR-0052's authorisation) — the `## HTTP/2` SCAFFOLD subsection from 05.1 is edited in place to add the upstream + fixture-0004 rules. Specifically:
  - **Asserted equivalence**: the "(05.1 scope)" tag is replaced with "(05.1 + 05.2 scope)" or similar; new bullets for routed-to-upstream `:method`/`:path`/`:scheme`/`:authority` verbatim forwarding (witnessed by the in-process backend in fixture 0004's tests asserting on received pseudo-headers); per-cluster RR distribution `[3, 3, 3]` per side (witnessed by `AssertDistribution`); ALPN selection equivalence at the differential level (witnessed by both proxies serving `:status 200` on an h2-only listener with the driver advertising `h2`); decoded-body equivalence on `direct_response` 2xx paths still applies (the 05.1 conformance-only assertion is now also witnessed by the fixture-0004 differential).
  - **Not asserted**: trailers (per ADR-0058) — the 05.1 wording stays; clarify that 05.2 also does not assert trailers despite now having the upstream surface that *could* observe them; add upstream connection re-use (per ADR-0056) as explicitly not asserted; cross-side request body bytes for routed-to-upstream requests (mirror of phase-04's ADR-K relaxation extended to H2 — the 9 `/api` request bodies are never compared across sides, only the `:status` codes and the per-side distribution).
  - **Header allow-list extensions**: the `:method`/`:path`/`:scheme`/`:authority` rows in the table at the top of `BEHAVIOR_CONTRACT.md` flip their "applies-to" column from `phase 05.2 routed-to-upstream H2 (forward-looking)` to `phase 05.2 routed-to-upstream H2 (active per ADR-0057)`. The in-place edit is authorised by ADR-0052.
  - **h2spec threshold**: unchanged (sections 3, 4, 5, 6 ex-6.6, 7, 8 — pin still at the ADR-0051 SHA). ADR-0055 adds prose about non-default `MaxFrameSize` / tight-window discipline to the threshold paragraph but does NOT add new section requirements.
  - **Applies to (05.1 + 05.2)**: enumerate the package surfaces from 05.1 (server-side codec, `--allow-h2c`, `directResponseAction.writeH2`) plus 05.2's surfaces (`internal/filter/hcm/h2/client.go`, `internal/cluster/dial_h2.go`, `internal/cluster/manager.go HttpProtocolOptions` reader, `routerActionH2` action, fixture 0004's full-stack HTTPS h2).
  - **Does not yet apply to**: `routed-to-upstream H2` and `fixture 0004` are removed from the deferred enumeration. The remaining deferred items: HTTP/3, server push, gRPC framing, trailer forwarding, upstream H2 stream pooling, h2c production fixtures, mTLS over h2, mixed-codec clusters.
- **`docs/envoy-go/CONFORMANCE_PINS.md`** — UNCHANGED in 05.2 (no pin bump; D-3.7 reserves pin bumps for dedicated phases). The `## Refresh procedure` section landed in the 05.1 follow-up batch per `4ec0f7d` (or wherever the I-4 fix landed in 05.1's tail per REVIEW.md Path A); 05.2 does not touch the file.
- **`docs/envoy-go/DECISIONS.md`** — new ADRs introduced by phase 05.2 (numbers assigned at planner/implementation time; expected starting number ADR-0055 based on `DECISIONS.md` tail at ADR-0054 per `bc4fca4`; the planner verifies next-free at write time per ADR-0004's autonomous-numbering rule). Anticipated:

  - **ADR-0055** (Flow-control discipline for from-scratch H2 codec): bundles the 05.1 REVIEW Important findings I-1 (outbound `MaxFrameSize` chunking on `ServerConn.writeData`), I-2 (per-stream send-window enforcement on outgoing DATA), I-3 (inbound WINDOW_UPDATE emission and per-stream `recvW` enforcement), Minor M-3 (`writeData` dead `if taken <= 0` branch / `reserveBlocking` collapse), M-7 (`recvW` field disposition — kept-and-consumed under I-3), M-9 (WINDOW_UPDATE delta overflow bounds-check), M-11 (`recvData` state-check before append), and as a cosmetic prerequisite M-5 (`translateFramerErr` helper extraction so `ClientConn` can share the framer translation). Status: Accepted. Doctrine: D-3.6 (every phase is a green build — and the from-scratch codec must be RFC-correct under realistic peer settings, not just under the conformance-suite peer's defaults). Settles: 05.1 REVIEW I-1/I-2/I-3 + M-3/M-5/M-7/M-9/M-11. Context: enumerates the 05.1 REVIEW dormant gaps and the routed-to-upstream surface that activates them. Decision: enumerates the seven specific code-level changes from §1 #6 above. Consequences: the 05.1 codec primitives are now load-bearing for realistic upstream H2 workloads; the regression tests are committed alongside the production fix; `BEHAVIOR_CONTRACT.md ## HTTP/2`'s threshold language is extended (in-place edit per ADR-0052) with non-default `MaxFrameSize` / tight-window prose; the per-section pass counts at the ADR-0051 pin remain at the 05.1 baseline (no new section requirements).
  - **ADR-0056** (Per-request fresh upstream H2 dial): mirrors phase-04 ADR-0039 (per-request fresh upstream H1 dial). Documents that upstream H2 multiplexing is intentionally unrealised at phase 05.2; pooling lands with the upstream-robustness family. The phase-05.2 differential surface does not assert pool/non-pool — Envoy pools, envoy-go does not, both produce the same per-request `:status`/body output, both produce per-side `[3,3,3]` distribution under sequential-request workload, the cross-conn frame counts differ but those are not in the equivalence matrix. Carry-forward: the upstream-robustness family, when it lands, brings H2 pooling and supersedes ADR-0056 with a pooling discipline ADR.
  - **ADR-0057** (Closes ADR-0035 H2 leg via fixture 0004's full-stack HTTPS h2): documents that ADR-0035's narrowed scope (fixture-0002 plaintext upstream backends) is now superseded for the H2 surface specifically: fixture-0004 has full-stack HTTPS h2 between proxy and upstream backends, so the upstream-TLS code path (phase-03's `Cluster.Dial` TLS branch + phase-05.2's `DialH2`) is now under differential coverage. The H1+TLS upstream gap remains open from ADR-0035 — a future HTTPS-H1 fixture (or an extension of fixture 0003 to TLS upstream) closes the H1 leg; phase 05.2 does not. ADR-0057 explicitly carries forward the H1+TLS upstream gap with a "phase-05.2-follow-up" tag (planned for between phase 05.2 and phase 06, or folded into phase 07's filter-chain framework, or staying open into HTTP-filter-family phases — 05.2 does not pre-decide).
  - **ADR-0058** (Trailers observed but not forwarded — H2 router): documents that the 05.1 H2 server-side codec correctly observes trailing HEADERS frames per RFC 9113 §8.1 (h2spec section 8 asserts this) and the 05.2 H2 client-side codec also observes them on the upstream conn, but the `routerActionH2` action discards trailers in both directions. The router emits END_STREAM on the response HEADERS or final DATA, never via a trailing HEADERS frame. The fixture-0004 driver does not exercise trailers. Forward-looking: phase 07's filter-chain framework + the gRPC family land trailer forwarding (where `grpc-status` is carried in trailers).

  (The planner re-verifies the next-free ADR number at PLAN write time; ADR-0055..ADR-0058 is the expected mapping based on the `DECISIONS.md` tail at `bc4fca4` being ADR-0054.)

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 05.2)

Phase 05.2 adds one new file to the 05.1 sub-package and modifies several existing ones. The 05.1 server-side surface is reused unchanged on the structural level (the flow-control tightening per ADR-0055 is a behavioural change inside existing files, not a structural reshape):

```
cmd/envoy-go/main.go                 (UNCHANGED)
cmd/envoy-go/main_test.go            (UNCHANGED — recommendation: do not add fifth bootstrap variant)
internal/listener/manager.go         (UNCHANGED)
internal/listener/manager_test.go    (UNCHANGED)
internal/cluster/cluster.go          (MODIFIED: UseH2() accessor; blank import for upstreams/http/v3)
internal/cluster/manager.go          (MODIFIED: HttpProtocolOptions reader; build-time validation)
internal/cluster/dial_h2.go          (NEW: Cluster.DialH2)
internal/cluster/cluster_test.go     (MODIFIED: H2-cluster build positive + negative cases; DialH2 cases)
internal/cluster/manager_test.go     (MODIFIED: HttpProtocolOptions parser cases)
internal/cluster/dial_h2_test.go     (NEW)
internal/filter/hcm/filter.go        (MODIFIED: NewFilter picks routerActionH2 based on UseH2();
                                       ALPN dispatch in Handle is UNCHANGED from 05.1)
internal/filter/hcm/config.go        (UNCHANGED — codec_type/HCM-side fields are 05.1's surface)
internal/filter/hcm/actions.go       (MODIFIED: routerActionH2 variant added; routerAction unchanged;
                                       directResponseAction unchanged from 05.1)
internal/filter/hcm/actions_test.go  (MODIFIED: routerActionH2 cases)
internal/filter/hcm/connection.go    (UNCHANGED)
internal/filter/hcm/h2dispatch.go    (UNCHANGED — the per-stream dispatcher invokes directResponseAction
                                       OR routerActionH2 via the codec-neutral action interface)
internal/filter/hcm/h2/              (sub-package; client.go is the only NEW file)
   conn.go         (MODIFIED per ADR-0055: writeData I-1/I-2 chunking; onData I-3 recv-window
                    debit + WINDOW_UPDATE emission; onWindowUpdate M-9 overflow bounds-check)
   stream.go       (MODIFIED per ADR-0055: recvWindowUpdate M-9 bounds-check;
                    recvData M-11 state-before-append reorder)
   framer.go       (MODIFIED per ADR-0055/M-5: translateFramerErr helper extracted;
                    ClientConn's framer wrapper consumes it)
   hpack.go        (UNCHANGED — encoder/decoder integration is conn-role-agnostic)
   flow.go         (MODIFIED per ADR-0055/M-3: window.reserveBlocking replaces waitFor+reserve)
   preface.go      (UNCHANGED — server-side preface read; client-side preface write is NEW
                    in client.go, not added here, to preserve the 05.1 file boundaries)
   settings.go     (MODIFIED: NEW writeClientInitialSettings + readServerSettings helpers
                    alongside the existing server-side helpers; the existing exports are unchanged)
   errors.go       (UNCHANGED)
   client.go       (NEW: ClientConn + clientStream + RoundTrip + Close; reuses framer/hpack/
                    flow/settings/errors primitives; ~400 LoC)
   <existing test files>  (MODIFIED: regression tests per ADR-0055; new client_test.go OR
                            appended cases to conn_test.go for the client-side surface)
   client_test.go  (NEW OR appended — planner choice)

internal/bootstrap/bootstrap.go      (MODIFIED: blank import for upstreams/http/v3)

test/conformance/h2spec/             (UNCHANGED — pin and threshold list stay at 05.1 baseline)

test/fixtures/0004-h2-routing/       (NEW fixture)
   envoy.yaml, envoy-go.yaml, expectations.yaml, README.md
   pki/gen/main.go, pki/*.pem
   driver/driver.go, driver/driver_test.go
   backends/main.go

test/differential/runner_test.go     (MODIFIED: blank-import for fixture 0004; no new
                                       interface extension — fixture asserts in-band)

test/helpers/h2.go + h2_test.go      (NEW: H2RoundTrip helper)

docs/envoy-go/BEHAVIOR_CONTRACT.md   (MODIFIED: ## HTTP/2 in-place edit per ADR-0052)
docs/envoy-go/CONFORMANCE_PINS.md    (UNCHANGED)
docs/envoy-go/DECISIONS.md           (APPENDED: ADR-0055..ADR-0058 — 05.2's four ADRs;
                                       planner verifies next-free at write time)
docs/envoy-go/ROADMAP.md             (row 05.2: planned → in-progress at SPEC commit; → done
                                       at phase-done commit; row 05 parent → done at same commit)
docs/envoy-go/STATE.md               (updated at each lifecycle transition; advances to phase 06
                                       lifecycle-state 1 at 05.2 phase-done)
docs/envoy-go/phases/05.2-upstream-h2/SPEC.md / PLAN.md / PROGRESS.md / REVIEW.md
docs/envoy-go/phases/05-http-2/SPEC.md  (UNCHANGED — master design doc, read-only at parent)
docs/envoy-go/phases/05.1-downstream-h2/  (UNCHANGED — closed read-only history)
```

### 5.2 Downstream HTTP/2 connection lifecycle — UNCHANGED FROM 05.1

The downstream-conn lifecycle is the 05.1 surface (per 05.1 SPEC §5.2): `Filter.Handle` ALPN dispatch → `h2.NewServerConn` → `Run()` → preface check → SETTINGS handshake → frame loop → per-stream dispatch goroutine → action invocation. **Phase 05.2 does NOT change this lifecycle.** The only behavioural change is inside the codec primitives (per ADR-0055): outbound DATA chunking now respects `MaxFrameSize` AND per-stream send-window; inbound DATA decrements `recvW` and triggers WINDOW_UPDATE emission on the half-window threshold; WINDOW_UPDATE deltas are bounds-checked. These changes are transparent to the dispatch layer.

The action-variant choice (per-route, at filter-build time) is the only structural change to the downstream-side dispatch path: a route whose target cluster has `UseH2() == true` now dispatches to `routerActionH2` instead of `routerAction`. The dispatch goroutine in `serverStream.dispatch` invokes the action through the same codec-neutral action interface; the H2 stream-write helpers (`Headers`/`Data`/`RST`) are the same ones `directResponseAction.writeH2` used in 05.1.

### 5.3 Upstream HTTP/2 connection lifecycle (NEW)

A `routerActionH2.do` invocation drives the following sequence on the upstream side:

1. `r.cluster.DialH2(ctx)` — calls `Cluster.Dial(ctx)` (phase-03 `*stdtls.Conn` after handshake), confirms `NegotiatedProtocol == "h2"`, wraps in `h2.NewClientConn(ctx, raw)`. Errors propagate as the `cluster: dial h2: …` diagnostics from §4.1.
2. `h2.NewClientConn`:
   a. Writes the client preface bytes (`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n`) to the upstream conn.
   b. Writes the client's initial SETTINGS frame (`writeClientInitialSettings` from `settings.go`).
   c. Reads the server's initial SETTINGS frame; applies the values to the local `serverSettings` view (the peer-advertised settings the client respects, e.g. `MaxFrameSize`, `MaxConcurrentStreams`, `InitialWindowSize`, `HeaderTableSize`); writes SETTINGS_ACK.
   d. Reads the server's SETTINGS_ACK for our own initial SETTINGS (the server-side server-conn always SETTINGS_ACKs immediately per 05.1 SPEC §5.2 step 3.c — symmetric here).
   e. Spawns a frame-read goroutine that runs the conn-level read loop (DATA/HEADERS/RST_STREAM/SETTINGS/PING/WINDOW_UPDATE/GOAWAY dispatch — same shape as 05.1's `ServerConn.Run` minus the new-stream-from-HEADERS path because the client doesn't accept new streams from the server, only from itself).
   f. Returns the `*ClientConn` ready for `RoundTrip`.
3. `cc.RoundTrip(ctx, req)`:
   a. Allocates a fresh stream id (`atomic.AddUint32(&cc.nextStreamID, 2)` returning previous-value + 2; first call returns 1, second 3, third 5, …).
   b. Constructs a `clientStream` with the new id, registers it in `cc.streams`.
   c. Encodes the request HEADERS via the conn's hpack encoder (pseudo-headers `:method`/`:path`/`:scheme`/`:authority` first per RFC 9113 §8.3, then regular headers in deterministic order — per the same discipline 05.1's `directResponseAction.writeH2` follows).
   d. Writes HEADERS frame (with END_HEADERS flag; END_STREAM flag if no body — fixture-0004 GETs are bodyless, so END_STREAM here in practice).
   e. (If body present:) writes DATA frame(s) chunked per ADR-0055's I-1/I-2 discipline; final DATA carries END_STREAM.
   f. Waits on the per-stream response channel (the conn-level frame-read goroutine routes inbound HEADERS+DATA+END_STREAM frames addressed to this stream id into the channel).
   g. On receipt of HEADERS: decodes via the conn's hpack decoder; extracts `:status`; constructs the partial `H2Response`.
   h. On receipt of DATA: decrements `cs.recvW`/`cc.recvW` per ADR-0055/I-3; if half-window threshold crossed, emit WINDOW_UPDATE; appends body bytes to the response buffer.
   i. On receipt of END_STREAM (either on the response HEADERS for an empty body, or on the final DATA, or on a trailing HEADERS frame which is observed-and-discarded per ADR-0058): completes the response, returns to caller.
   j. On receipt of RST_STREAM(code) for this stream: returns `*h2.Error{Code: code, Stream: id}`.
   k. On receipt of GOAWAY (conn-level): finishes any in-flight streams strictly before the GOAWAY's `last-stream-id`; returns `*h2.Error{Code: NO_ERROR}` for streams above the cutoff.
   l. On ctx cancel: emits RST_STREAM(CANCEL) for this stream id; returns ctx.Err().
4. `cc.Close()` (deferred by `routerActionH2.do`): emits GOAWAY(NO_ERROR) with the highest stream id we've allocated as `last-stream-id`; closes the underlying conn (which sends a TCP FIN); the frame-read goroutine sees the EOF and exits cleanly.
5. The `routerActionH2.do` write-back to the downstream stream uses the same `H2ResponseWriter` interface 05.1's `directResponseAction.writeH2` uses — pseudo-headers first, then regular headers (the upstream's response headers are forwarded verbatim modulo the standard pseudo-header reorder discipline; `Server`/`Date`/etc. are forwarded as-received because the H2 router does NOT inject local headers on routed responses, mirroring phase-04's H1 router).

### 5.4 Direct-response codec-neutral path — UNCHANGED FROM 05.1

The `directResponseAction` factoring (per 05.1 SPEC §5.5) is unchanged in 05.2. `body() (status int, headers http.Header, body []byte)` returns the codec-neutral synthesised reply; `writeH1` writes HTTP/1.1 wire bytes; `writeH2` writes HEADERS+DATA+END_STREAM via the per-stream send-side helpers. Phase 05.2 does NOT touch this code path; fixture 0004's `/health` and `/missing` paths exercise it via the existing 05.1 surface.

The optional 05.1 follow-up FU-7 (elide the empty trailing DATA frame when `bodyText == ""`) remains out-of-scope for 05.2. Fixture 0004's `/health` body is `OK\n` (3 bytes, non-empty) and `/missing` body is `not found\n` (10 bytes, non-empty), so the optimisation is unmotivated by 05.2's surface. Carried forward unchanged.

### 5.5 Cluster H2 build-time validation

The cluster builder at `internal/cluster/manager.go` accepts `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]` and reads its discriminator. The behaviour matrix:

| Cluster `HttpProtocolOptions` shape | `useH2` | Build action | Rationale |
|---|---|---|---|
| Field absent (no `typed_extension_protocol_options` map, or map without the key) | `false` | Build OK (phase-04 H1 path) | Phase-04 baseline; no regression. |
| Present with `explicit_http_config.http2_protocol_options{}` (empty inner) | `true` | Validate TLS+ALPN h2; build OK or error | Per §1 #3 / §4.1; the active 05.2 H2-cluster path. |
| Present with `explicit_http_config.http_protocol_options{}` (H1 discriminator) | `false` | Build OK (silently ignored inner fields) | Forward-compat: H1 typed-options become 05.2-relevant only when phase 06+ adds per-cluster H1 settings; until then, 05.2 silently honours the discriminator's H1 selection but ignores its inner config. |
| Present with `auto_config{}` | `false` | Build OK (silently ignored — H1 fallback) | 05.2 narrowing of master SPEC §5.8: silent-ignore preserves forward-compat as more discriminators land in later phases. The master SPEC suggested erroring; the 05.2 SPEC narrows to silent-ignore for consistency with phase-04's silent-ignore discipline. |
| Present but `UpstreamProtocolOptions` is nil/empty | `false` | Build OK (silently ignored) | Defensive; same as field-absent. |
| Present with `http2_protocol_options{}` BUT no `transport_socket` | n/a | Build error | TLS required for h2 (per master SPEC §5.8). |
| Present with `http2_protocol_options{}` + non-TLS `transport_socket` | n/a | Build error | TLS required for h2. |
| Present with `http2_protocol_options{}` + TLS `transport_socket` BUT `alpn_protocols` lacks `h2` | n/a | Build error | ALPN h2 negotiation required (per RFC 7301 + ADR-0057). |

Inner-field treatment when `useH2 == true`:

- `initial_stream_window_size`, `initial_connection_window_size`, `max_concurrent_streams`, `hpack_table_size`, `allow_metadata`, `allow_connect`, `max_outbound_frames`, `max_outbound_control_frames`, `max_consecutive_inbound_frames_with_empty_payload`, `max_inbound_priority_frames_per_stream`, `max_inbound_window_update_frames_per_data_frame_sent`, `stream_error_on_invalid_http_messaging`, `override_stream_error_on_invalid_http_message` — all silently ignored at 05.2; the cluster advertises the hardcoded ADR-0047 defaults regardless.
- `common_http_protocol_options.{idle_timeout, max_connection_duration, max_headers_count, max_response_headers_kb, max_stream_duration, headers_with_underscores_action}` — silently ignored.
- `upstream_http_filters[]` — silently ignored (filter chain on upstream is phase 07's surface).
- `connection_pool_per_downstream_connection` — silently ignored (pooling is upstream-robustness family).

### 5.6 Flow-control discipline (per ADR-0055)

The 05.1 codec primitives implement the RFC 9113 §5.2 baseline (per-stream and per-connection flow-control windows, decremented on DATA send, replenished on WINDOW_UPDATE recv) but have three dormant gaps the 05.1 REVIEW identified as Important:

- **I-1:** outbound DATA was bounded only by the conn-level `sendW`, not by `MaxFrameSize`. With the peer's default `MaxFrameSize: 16384`, any body larger than 16 KB that fit in the conn-window was written as a single oversized frame, triggering peer-side `FRAME_SIZE_ERROR`.
- **I-2:** outbound DATA did not reserve against the per-stream `sendW`. A peer with a small per-stream initial window could be over-fed; the peer would then RFC-permittedly abort the conn with `FLOW_CONTROL_ERROR`.
- **I-3:** receive-side flow-control was allocated but never enforced. The server advertised `INITIAL_WINDOW_SIZE: 65535` (the default conn-window) and never replenished it. After cumulative inbound DATA across all streams reached the conn-window, the conn deadlocked.

Plus three Minor findings tied to the same primitives:

- **M-3:** `writeData`'s `waitFor`+`reserve` pair was non-atomic; under concurrent multi-stream writes, a window race could have over-reserved.
- **M-9:** WINDOW_UPDATE deltas were not overflow-bounds-checked against `2³¹ - 1`.
- **M-11:** `recvData` appended to the body buffer BEFORE checking stream-state validity, wasting memory on closed streams.

ADR-0055 enumerates the seven specific code-level fixes (per §1 #6); §4.2 lists the modified files; §3 gate (e) enumerates the regression tests. The ADR's prose extension to `BEHAVIOR_CONTRACT.md ## HTTP/2`'s threshold paragraph is in-place per ADR-0052; the per-section pass counts at the ADR-0051 pin remain at the 05.1 baseline.

The flow-control fix is *load-bearing* for 05.2: without it, fixture 0004's full-stack HTTPS h2 would trip I-1/I-2/I-3 on the upstream backends' realistic responses (the test backends respond with `backend-N:v1/<n>` — small, ~16 bytes — so I-1 doesn't trip on the *fixture* even without the fix; but routed-to-upstream H2 is the real-world surface that needs the fix, and ADR-0055 is the right place to land it before the surface goes live). The regression tests (per §1 #6) drive the boundary cases that fixture 0004 doesn't exercise.

### 5.7 BEHAVIOR_CONTRACT phase-05.2 in-place edit (per ADR-0057's adjacent authorisation under ADR-0052)

The 05.1 SCAFFOLD subsection at `BEHAVIOR_CONTRACT.md` lines 267-315 (per the 05.1 REVIEW findings) is edited in place. The intended SPEC-binding form for the planner to lift into the file:

- **Asserted equivalence (05.1 + 05.2 scope)** — was "(05.1 scope)"; the bullets are extended:
  - **`:status` per request:** required + asserted on every fixture-0004 request (was: only the conformance gate via h2spec section 8 in 05.1).
  - **Decoded body bytes** on `direct_response` 2xx paths: byte-equal to the configured `body` string. NEW: now witnessed by fixture 0004's 9 `/health` requests on both sides (was: only h2spec indirectly + envoy-go unit tests).
  - **Per-stream response header set-equality modulo allow-list:** locally-generated H2 responses carry `:status` (required + asserted), `Server` (matched verbatim with upstream's `envoy`), `Content-Type`, `Content-Length`, `Date` (presence required; value not byte-compared). **NEW:** routed-to-upstream H2 responses now in scope: `:status` required + asserted; `Server`/`Content-Type`/`Content-Length`/`Date` headers from the upstream backend forwarded verbatim (no router-injected headers); the per-stream response header set-equality between sides asserted modulo the same allow-list.
  - **NEW:** routed-to-upstream H2 request preservation — `:method`/`:path`/`:scheme`/`:authority` forwarded verbatim from downstream to upstream (witnessed by the in-process backend in fixture 0004's tests asserting on received pseudo-headers). The path normalisation discussed in master SPEC §5.7 is *empty* on the H2 side (the path is the bytes of the `:path` pseudo-header — there's no stdlib net/http parsing to inject normalisations).
  - **NEW:** route-match selection equivalence on H2: same method + path → same matched route on both proxies (witnessed indirectly by per-side `[3,3,3]` distribution + `:status` per request).
  - **NEW:** per-cluster RR distribution `[3, 3, 3]` per side over the 9 router-action requests (local-correctness; cross-side sequence is NOT asserted, mirroring phase-04's relaxation).
  - **NEW:** ALPN selection equivalence at the differential level: a downstream client that advertises only `h2` reaches the H2 driver on both proxies (witnessed by fixture 0004's `:status 200` on every routed response).
- **Not asserted (05.1 + 05.2 scope)** — extended:
  - Wire-byte H2 framing — unchanged from 05.1.
  - SETTINGS values byte-for-byte — unchanged from 05.1.
  - WINDOW_UPDATE timing or count — unchanged from 05.1; ADR-0055's tightening adds frame counts that depend on body size and peer window behaviour, which are inherently non-deterministic across the two proxies.
  - Stream id allocation pattern — unchanged from 05.1.
  - Trailers — unchanged from 05.1; ADR-0058 formalises the upstream-side discard rule.
  - 0-RTT TLS early-data behaviour — unchanged from 05.1.
  - **NEW:** Connection re-use upstream (per ADR-0056): Envoy pools, envoy-go does not; both produce the same per-request `:status`/body output; cross-conn frame counts differ.
  - **NEW:** Cross-side request body bytes for routed-to-upstream requests (mirror of phase-04's ADR-K relaxation extended to H2) — fixture 0004's 9 `/api` request bodies are bodyless GETs, so this rule is unexercised in 05.2; carried forward as the rule for any future POST/PUT-bearing fixture.
- **Header allow-list extensions** — the table at the top of `BEHAVIOR_CONTRACT.md` has its existing rows for `:method`/`:path`/`:scheme`/`:authority` flipped from `applies-to: phase 05.2 routed-to-upstream H2 (forward-looking)` to `applies-to: phase 05.2 routed-to-upstream H2 (active per ADR-0057)`.
- **h2spec threshold:** sections 3, 4, 5, 6 (excluding 6.6 PUSH_PROMISE), 7, 8 — all `failed == 0`. Pin: `summerwind/h2spec` at the SHA recorded in `CONFORMANCE_PINS.md`. **ADR-0055 prose extension:** the from-scratch H2 codec respects `MaxFrameSize` chunking on outbound DATA, per-stream send-window enforcement, inbound WINDOW_UPDATE emission on a half-window high-water threshold, and overflow bounds-checks on WINDOW_UPDATE deltas. These properties are validated by the regression unit tests (per §1 #6) and by the existing h2spec section 5/6 coverage at the pinned SHA; no new section requirements are added.
- **Applies to (05.1 + 05.2):** phase-05.1 `internal/filter/hcm/h2/` server-side codec (unchanged); the codec-neutral `directResponseAction` factoring; the `--allow-h2c` test-only flag; the conformance suite under `test/conformance/h2spec/`. **NEW:** phase-05.2 `internal/filter/hcm/h2/client.go` (`ClientConn` + `RoundTrip` + `Close`); `internal/cluster/dial_h2.go`; `internal/cluster/manager.go HttpProtocolOptions` reader + validation; `Cluster.UseH2()` accessor; `routerActionH2` action variant in `internal/filter/hcm/actions.go`; fixture `0004-h2-routing` (full-stack HTTPS h2); `test/helpers/h2.go H2RoundTrip` helper.
- **Does not yet apply to:** HTTP/3; server push; gRPC framing; trailer forwarding (per ADR-0058); upstream H2 stream pooling (per ADR-0056); h2c production fixtures; mTLS over h2; mixed-codec clusters (single cluster used by both H1 and H2 listeners). **REMOVED from this list (now active):** routed-to-upstream H2 (active per ADR-0057), fixture 0004 (active per §4.3).

### 5.8 Fixture 0004 architectural shape

Fixture 0004 is the project's first full-stack H2 differential. The runner architecture is unchanged from fixtures 0001/0002/0003 (per ADR-0019 + the test-helpers convention from 00-bootstrap), with three structural choices specific to 05.2:

1. **STATIC subject + STRICT_DNS reference** for the cluster type, per ADR-0010 and the fixture 0001/0002/0003 precedent. The STATIC subject points at `127.0.0.1:<dyn>` × three backend ports (the fixture's `backends/main.go` instances launched on per-test-run dynamic ports); the STRICT_DNS reference points at `host.docker.internal:<dyn>` × the same three ports, with `dns_lookup_family: V4_ONLY`.
2. **PKI generation at fixture build time** via `pki/gen/main.go`, mirroring fixture-0002's pattern. The CA + leaf certs cover SANs `localhost`, `127.0.0.1`, and `host.docker.internal` so both subject and reference can validate the same chain. Generated PEMs are committed under `pki/`. Run-time regeneration via `go generate ./test/fixtures/0004-h2-routing/...`.
3. **Per-side `[3, 3, 3]` distribution assertion**, NOT cross-side (mirror of phase-04's local-correctness relaxation per ADR-K). The driver issues 9 sequential `/api` requests; the per-cluster RR counter on each side picks the next backend; assertion is `[3, 3, 3]` on each side (the per-side sequence is not aligned because envoy-go's per-request fresh-conn discipline (per ADR-0039 H1 / ADR-0056 H2) and Envoy's pooling produce different conn-level timing — but the local distribution is identical).

The fixture's reference is invoked with `--concurrency 1` per ADR-0028 (single-worker Envoy keeps RR distribution deterministic across runs). The subject is single-process by definition.

The 27-request workload is chosen to mirror fixture 0003 exactly so the regression is mechanical to compare side-by-side; the planner records this lineage in the fixture README.

## 6. Data flow

### 6.1 Routed-to-upstream H2 request → response (the new shape in 05.2)

Plain-text-after-decryption view of one router-action request on phase 05.2 (this is the master SPEC §6.1 flow, repeated here as a 05.2 deliverable):

```
[client] -- TLS handshake (ALPN: h2) --> [listener]
[client] -- preface bytes ("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n") --> [serverConn]
[client] -- SETTINGS --> [serverConn]
[serverConn] -- SETTINGS_ACK --> [client]
[serverConn] -- SETTINGS --> [client]
[client] -- SETTINGS_ACK --> [serverConn]
[client] -- HEADERS{:method GET, :path /api/v1/3, :scheme https, :authority …; END_HEADERS, END_STREAM} --> [serverConn]
[serverConn] -- demux frame to streamN --> [serverStream(N)]
[serverStream] -- build *http.Request from pseudo-headers + decoded HEADERS --> [routeTable.match(req)]
[routeTable] -- returns matching routeEntry{action: routerActionH2{cluster: c_h2_backend}} --> [serverStream]
[serverStream] -- routerActionH2.do(ctx, req, streamWriter) --> [routerActionH2]
[routerActionH2] -- cluster.DialH2(ctx) --> [Cluster][TLS dial][ALPN "h2"]
[Cluster] -- *h2.ClientConn --> [routerActionH2]
[routerActionH2] -- defer clientConn.Close() (per-request fresh per ADR-0056) --> registered
[routerActionH2] -- clientConn.RoundTrip(ctx, req) --> [ClientConn]
[ClientConn] -- preface + SETTINGS exchange (mirror of server side) --> [upstream]
[ClientConn] -- HEADERS+END_STREAM (no body — bodyless GET) --> [upstream]
[upstream] -- HEADERS{:status 200, content-type text/plain, content-length 16, server …, …} --> [ClientConn]
[upstream] -- DATA{"backend-1:v1/3"; END_STREAM} --> [ClientConn]
[ClientConn] -- *H2Response{Status:200, Headers, Body:"backend-1:v1/3"} --> [routerActionH2]
[routerActionH2] -- streamWriter.Headers(resp.Headers, false) --> [serverStream]
[routerActionH2] -- streamWriter.Data(resp.Body, true) --> [serverStream]
[serverStream] -- HEADERS{:status 200, ...; END_HEADERS} --> [client]
[serverStream] -- DATA{"backend-1:v1/3"; END_STREAM} --> [client]
[serverStream] -- transition to closed --> [serverConn]
[deferred clientConn.Close()] -- WriteGoAway(lastStreamID, NO_ERROR, "client close") --> [upstream]
[deferred clientConn.Close()] -- TCP FIN --> [upstream]
```

### 6.2 502 bad-gateway local-reply (dial failure or upstream protocol error)

```
[serverStream] -- routerActionH2.do(ctx, req, streamWriter) --> [routerActionH2]
[routerActionH2] -- cluster.DialH2(ctx) --> ERROR ("alpn negotiated %q, want \"h2\"" or similar)
[routerActionH2] -- streamWriter.Headers({":status":"502", ...}, false) --> [serverStream]
[routerActionH2] -- streamWriter.Data("bad gateway\n", true) --> [serverStream]
[serverStream] -- HEADERS{:status 502, content-type text/plain, server envoy, content-length 12, date …} --> [client]
[serverStream] -- DATA{"bad gateway\n"; END_STREAM} --> [client]
[serverStream] -- transition to closed --> [serverConn]
```

The 502 prose body matches the H1 path's wording byte-for-byte except for the framing. The upstream conn (if a TCP/TLS dial succeeded but ALPN returned wrong) is closed by `DialH2`'s explicit error-branch close (per §4.1's `dial_h2.go` snippet); the deferred `clientConn.Close()` in `routerActionH2.do` is unreachable on this path because the early-return on `DialH2` error precedes the `defer cc.Close()` registration (see the `routerActionH2.do` snippet in §4.1: the dial-error branch returns *before* the defer line, so no defer is registered when `cc == nil`).

### 6.3 ctx cancellation mid-RoundTrip

```
[serverStream] -- routerActionH2.do(ctx, req, streamWriter) --> [routerActionH2]
[routerActionH2] -- cluster.DialH2(ctx) --> *h2.ClientConn (success)
[routerActionH2] -- defer clientConn.Close() --> registered
[routerActionH2] -- clientConn.RoundTrip(ctx, req) --> [ClientConn]
[ClientConn] -- HEADERS+END_STREAM --> [upstream]
[upstream] -- HEADERS{:status 200, ...} --> [ClientConn]
[upstream] -- DATA{...} --> (in-flight)
[ctx canceled mid-DATA-read]
[ClientConn] -- WriteRSTStream(streamID, CANCEL) --> [upstream]
[ClientConn] -- returns ctx.Err() --> [routerActionH2]
[routerActionH2] -- streamWriter.RST(CANCEL) --> [serverStream]
[serverStream] -- WriteRSTStream(streamN, CANCEL) --> [client]
[serverStream] -- transition to closed --> [serverConn]
[deferred clientConn.Close()] -- WriteGoAway(lastStreamID, NO_ERROR, "client close") --> [upstream]
[deferred clientConn.Close()] -- TCP FIN --> [upstream]
```

## 7. Error handling and failure modes

The phase-05.2 H2 codec (server + client) follows RFC 9113's two-tier error model, inheriting 05.1's discipline unchanged for the server side and applying the symmetric discipline on the client side:

- **Connection-level errors** (server or client) trigger GOAWAY + close. Examples (NEW on the client side): bad server preface (server doesn't write the server preface; we expect SETTINGS — if we read something else, PROTOCOL_ERROR); malformed server SETTINGS (FRAME_SIZE_ERROR or PROTOCOL_ERROR); HPACK COMPRESSION_ERROR on a server-sent HEADERS frame; PUSH_PROMISE received from the server (we advertise `ENABLE_PUSH=0`, so this is a peer protocol violation → PROTOCOL_ERROR); GOAWAY received from the server (graceful — finish in-flight streams, don't open new ones).
- **Stream-level errors** trigger RST_STREAM + per-stream cleanup, conn keeps running. NEW on the client side: stream-level FRAME_SIZE_ERROR on response DATA, response RST_STREAM received with a non-CANCEL code (the `RoundTrip` returns `*h2.Error{Code: code}` to `routerActionH2`), DATA after END_STREAM (STREAM_CLOSED), HEADERS for an unexpected stream id (server allocated a server-initiated stream — illegal because we advertise `ENABLE_PUSH=0`).

The `routerActionH2.do` failure modes:

- **Upstream dial fails (TCP, TLS, or ALPN-mismatch)** → write a 502 local-reply via H2 (HEADERS `{:status: 502, content-type: text/plain, server: envoy, content-length: 12, date: …}` + DATA `bad gateway\n` + END_STREAM) on the downstream stream. Return nil (the connection-level error handler doesn't run; this is a per-stream-level recovery).
- **Upstream H2 protocol error (RoundTrip returns a non-CANCEL `*h2.Error`)** → 502 local-reply, close the upstream conn (handled by the deferred `cc.Close()`), return nil.
- **Upstream returns a 5xx HTTP-status response** → forward verbatim. Only *protocol errors* translate to 502; HTTP-status-level errors pass through (mirror of phase-04's H1 router behaviour). The fixture 0004 driver does not exercise this directly, but the unit test for `routerActionH2.do` covers it.
- **ctx cancellation mid-RoundTrip** → the upstream-side `RoundTrip` emits RST_STREAM(CANCEL) on the upstream stream; the action emits RST_STREAM(CANCEL) on the downstream stream via `w.RST(h2.ErrCodeCancel)`; returns nil. The downstream-side conn keeps running; the upstream-side conn is closed by the deferred `cc.Close()`.
- **Downstream stream write fails mid-response (e.g. downstream conn died between HEADERS and DATA)** → the action returns the underlying error to `serverStream.dispatch`, which emits RST_STREAM(INTERNAL_ERROR) on the downstream and the conn keeps running. The deferred `cc.Close()` still emits the GOAWAY on the upstream.

The per-stream `defer cc.Close()` in the action closes the upstream `ClientConn` regardless of error path. This satisfies the same prose-vs-mechanism shape as phase-04's H1 path (per ADR-0053's M-5 disposition + its 05.2-will-repeat-the-pattern note); the cosmetic gap is acknowledged and remains deferred to a future SPEC-corrections ADR (per ADR-0053's disposition).

Listener-bind error semantics carried forward unchanged from phase-02 (`log.Fatalf` on bind failure; admin `/ready` never reaches Ready). Cluster-build error semantics carried forward unchanged from phase-02 + phase-03 + 05.2's H2-cluster validation (build-time errors prevent the listener manager from starting; runtime errors in `DialH2` translate to per-stream 502s).

## 8. Testing scope for phase 05.2

### 8.1 Unit tests (under `internal/filter/hcm/h2/`)

NEW: client-side coverage in `client_test.go` (or appended to `conn_test.go` per planner choice). Per §4.1's `client_test.go` bullet: client-preface emit + SETTINGS exchange happy path; `RoundTrip` happy path (HEADERS+END_STREAM out, HEADERS+DATA+END_STREAM in); `RoundTrip` with body; `RoundTrip` ctx cancel during write; `RoundTrip` ctx cancel during read; `RoundTrip` peer RST_STREAM; `RoundTrip` peer GOAWAY; `RoundTrip` peer DATA after END_STREAM; `Close` graceful GOAWAY; `RoundTrip` after `Close`; multi-`RoundTrip` capability check.

NEW: ADR-0055 regression tests (per §1 #6; can live in `flow_test.go` / `conn_test.go` since the primitives are shared between server and client): >16384-byte body chunked correctly with peer `MaxFrameSize: 16384`; `INITIAL_WINDOW_SIZE: 16` + 100-byte response body produces ~7 DATA frames + no `FLOW_CONTROL_ERROR`; >65 KB inbound body completes (no deadlock); WINDOW_UPDATE deltas overflow → RST_STREAM/GOAWAY(FLOW_CONTROL_ERROR); concurrent multi-stream writes against window primed at boundary values produce no over-reservation (race-detector test).

NEW (per 05.1 REVIEW carry-forward): integration test for the monotonic-id-reuse rejection branch at `internal/filter/hcm/h2/conn.go:308-319`. Currently only the even-id branch is covered by `TestServerConn_GOAWAYOnProtocolError_EvenStreamID`. Add a sibling `TestServerConn_GOAWAYOnProtocolError_StreamIDReuse` that opens a stream id N, completes it, and then sends another HEADERS on the same N — assert GOAWAY(PROTOCOL_ERROR).

UNCHANGED: 05.1 server-side tests continue to pass (no behavioural regression from ADR-0055's tightening; the regression tests prove this by exercising the boundary cases the existing tests don't reach).

### 8.2 Unit tests (under `internal/cluster/`)

NEW: `dial_h2_test.go` per §4.1 — `DialH2` happy path; ALPN-mismatch; not-TLS; ctx-cancel; TLS handshake error.

EXTENDED: `cluster_test.go` + `manager_test.go` per §4.1 — `HttpProtocolOptions` parser positive + negative cases; `auto_config` silent-ignore (5.5 narrowing); `UseH2()` accessor.

### 8.3 Unit tests (under `internal/filter/hcm/`)

NEW: `actions_test.go` cases per §4.1 — `routerActionH2` variant selection by `Cluster.UseH2()`; `routerActionH2.do` happy path against in-process h2 backend; 502 on dial failure; 502 on RoundTrip protocol error; ctx cancel emits RST_STREAM(CANCEL); upstream 5xx forwarded verbatim.

UNCHANGED: 05.1 cases (ALPN dispatch in `Filter.Handle`; `codec_type: HTTP2` build-time validation; `directResponseAction.writeH1`/`writeH2`).

### 8.4 End-to-end smoke (under `cmd/envoy-go/main_test.go`)

UNCHANGED in 05.2 (the recommendation is to skip the optional fifth bootstrap variant — fixture 0004 is the integration check).

### 8.5 Differential (under `test/differential/` + `test/fixtures/0004-h2-routing/`)

NEW NON-VACUOUS GATE: the 27-request workload of §5.8 against both proxies. `:status` equivalence per request + decoded body equivalence on the 9 `/health` direct-response paths + per-side `[3,3,3]` RR distribution + 404 status equivalence on the 9 `/missing`. Pre-existing fixtures `0000`/`0001`/`0002`/`0003` remain green (no regression from the codec changes).

### 8.6 Conformance (under `test/conformance/h2spec/`)

UNCHANGED: 53/53 PASS at the ADR-0051 pin. The flow-control tightening must not regress this.

### 8.7 Fuzz (under `internal/filter/hcm/h2/`)

UNCHANGED: 05.1 fuzz targets `FuzzFrameStream` + `FuzzHPACKDecode` continue to pass at the 30s ADR-0018 budget. No new fuzz target in 05.2 (the upstream-H2 surface is covered by the existing fuzzers + fixture-0004 differential — see §3 gate (d) rationale).

## 9. Out-of-scope (explicitly deferred)

Beyond §2's non-purposes, phase 05.2 silently ignores the following at parse time (no error, no honoured behaviour):

- HCM `http2_protocol_options` (the directly-on-HCM proto field) — already silently ignored from 05.1 per the phase-05.1 amendment to ADR-N. Unchanged in 05.2.
- Cluster `HttpProtocolOptions.common_http_protocol_options` (every field — `idle_timeout`, `max_connection_duration`, `max_headers_count`, `max_response_headers_kb`, `max_stream_duration`, `headers_with_underscores_action`).
- Cluster `HttpProtocolOptions.upstream_http_filters[]` (filter chain on upstream is phase 07's surface).
- Cluster `HttpProtocolOptions.connection_pool_per_downstream_connection`.
- Cluster `HttpProtocolOptions.explicit_http_config.http2_protocol_options.{initial_stream_window_size, initial_connection_window_size, max_concurrent_streams, hpack_table_size, allow_metadata, allow_connect, max_outbound_frames, max_outbound_control_frames, max_consecutive_inbound_frames_with_empty_payload, max_inbound_priority_frames_per_stream, max_inbound_window_update_frames_per_data_frame_sent, stream_error_on_invalid_http_messaging, override_stream_error_on_invalid_http_message}` (every inner field).
- Cluster `HttpProtocolOptions.explicit_http_config.http_protocol_options.*` (the H1 discriminator's inner fields).
- Cluster `HttpProtocolOptions.auto_config` (entire branch — silent-ignore, falls back to H1; 05.2 narrowing of master SPEC §5.8 per §5.5 above).
- Listener `filter_chain_match.application_protocols[]` — already silently ignored from 05.1; unchanged in 05.2.
- HCM `internal_address_config`, `path_with_escaped_slashes_action`, `add_user_agent`, `proxy_status_config`, `typed_header_validation_config`, `original_ip_detection_extensions`, `early_header_mutation_extensions`, `header_validation_config` — already silently ignored from 05.1; unchanged in 05.2.

The full silently-ignored set is the union of phase-04's (per ADR-N), phase-05.1's amendment, and phase-05.2's amendment above. ADR-N is amended (not superseded) to record the cluster-side `HttpProtocolOptions` inner-field additions. The amendment shape mirrors the 05.1 amendment shape (a single appended sub-section under ADR-N's Consequences, listing the newly-ignored fields).

## 10. Deferred decisions (the planner / implementer settles these)

This list is narrowed from master phase-05 SPEC §10 to the items that are scoped to 05.2. Items #1, #2, #3 (server-side), #5, #8 from the master SPEC §10 were scoped to 05.1 and are not repeated here.

1. **Whether to thread an `H2Request`/`H2Response` type-pair or re-use stdlib `*http.Request`/`*http.Response` shapes on the CLIENT side.** Master SPEC §10 #3 + ADR-0045 split this per-direction. 05.1 reused stdlib `*http.Request` on the server-side per the 05.1 SPEC §5.2 step 4a recommendation. 05.2 has the symmetric choice on the client side: a phase-05.2-internal `H2Request`/`H2Response` type pair (cleaner separation; exposes the codec-internal types to the action layer) vs reusing stdlib `*http.Request`/`*http.Response` (cheaper; same shape as the server side). **Recommendation: introduce a small `H2Request`/`H2Response` struct pair internal to `internal/filter/hcm/h2/`** so the codec's client-side surface is symmetric with its server-side surface; the action layer translates from the route-table's `*http.Request` to the codec's `H2Request` at the action-invocation boundary. Rationale: the request passes through the codec twice (server-side decoded → action → client-side encoded), and the type-pair makes the codec-internal shape explicit instead of overloading stdlib types. The planner records the choice in PLAN.md.
2. **Whether `routerActionH2` and `routerAction` share an interface or are concrete types invoked via a switch.** Master SPEC §10 #6. Phase 04's `actions.go` uses a small interface (`action.do(...)`); phase 05.1 kept it for the `directResponseAction.writeH1`/`writeH2` factoring. **Recommendation: keep the interface shape; both `routerAction.do` and `routerActionH2.do` satisfy it.** The codec-neutral interface is the same one `directResponseAction` already satisfies (per the 05.1 codec-neutral factoring). The H2 stream dispatch's `dispatch` helper invokes `action.do(...)` polymorphically; the action variant carries the codec choice internally. The planner records the choice in PLAN.md.
3. **Whether the per-cluster RR counter is per-`Cluster` (current phase-02 ADR-0024 scope) or per-`Cluster`-per-codec.** Master SPEC §10 #7. **Decision: keep per-`Cluster` (no change to ADR-0024 scope).** Fixture 0004's cluster is H2-only; the question is dormant in 05.2's scope. If a future phase introduces a mixed-codec cluster (single cluster used by both H1 and H2 listeners), an ADR will land at that time deciding per-codec scoping. The planner records the decision in PLAN.md (no ADR needed in 05.2 because the question is dormant).
4. **Whether `routerActionH2.do`'s 502 local-reply emits a `Date` header.** The master SPEC's prose includes `Date` in the locally-generated 502 response headers; the §4.1 snippet above omits it for prose brevity. **Decision: include `Date`** (matches the 05.1 `directResponseAction.writeH2` discipline; uses the same `dateNowRFC7231` helper). The planner ensures the production code matches the master SPEC's prose.
5. **Whether the `ClientConn` exposes a way to wait for SETTINGS_ACK before allowing `RoundTrip` calls.** Master SPEC §5.3 step 2 prescribes "exchange initial SETTINGS" before returning the conn from `NewClientConn`; the planner picks whether the SETTINGS_ACK wait is synchronous (blocks `NewClientConn` until our SETTINGS is ACKed) or asynchronous (returns after sending; `RoundTrip` blocks on the SETTINGS_ACK if it's still pending). **Recommendation: synchronous wait inside `NewClientConn`** — the per-request fresh-conn discipline (ADR-0056) means we'd otherwise `RoundTrip` immediately after `NewClientConn`, so the wait is unavoidable; doing it inside the constructor surfaces SETTINGS-handshake errors as constructor errors instead of mid-request errors. The planner records the choice in PLAN.md.
6. **Whether to add a fifth `cmd/envoy-go/main_test.go` bootstrap variant exercising H2-end-to-end.** Master SPEC §4.2 mentioned the optional addition. **Recommendation: skip it** — fixture 0004 covers the same surface differentially; the smoke test would be redundant. The planner records the choice in PLAN.md (no production code change).
7. **Whether to factor the `ClientConn`'s frame-read goroutine identically to `ServerConn.Run`'s loop.** The two loops have similar shape (read frame → dispatch by type → handle conn-level vs stream-level frames) but differ on the new-stream-from-HEADERS path (server accepts new client streams; client does NOT accept new server streams because we advertise `ENABLE_PUSH=0`). **Recommendation: keep the two loops separate** rather than extracting a generic `runFrameLoop(direction)` helper — the server/client asymmetries (PUSH_PROMISE handling, stream-id allocator direction, settings-application differences) are subtle enough that a shared loop introduces risk of hidden differential bugs. The planner records the choice in PLAN.md.
8. **Whether the `RoundTrip` response-body buffer is bounded.** A malicious upstream could send an unbounded response body to OOM the proxy. **Recommendation: NOT bounded in 05.2** (matches 05.1's per-stream `reqBody` discipline, which is also unbounded; the future bounding is a hardening-phase concern, likely tied to per-cluster `max_response_headers_kb` / future `max_response_body_bytes` settings). The planner records this as a known limitation in PLAN.md; no production code change beyond what §1 #1 prescribes.
9. **Concrete ADR numbers for ADR-0055..ADR-0058.** Per `DECISIONS.md` tail at `bc4fca4` being ADR-0054, the next-free is ADR-0055; 05.2's four ADRs land at ADR-0055..ADR-0058. The planner re-verifies next-free at write time (per ADR-0004's autonomous-numbering rule) and assigns the four anticipated topics (flow-control discipline, per-request fresh dial, ADR-0035 H2 leg closure, trailers observed-but-not-forwarded) to the four numbers in the order they're authored in PLAN.md.
10. **Whether the integration-test for the monotonic-id-reuse rejection branch (per the 05.1 REVIEW carry-forward) lives alongside `TestServerConn_GOAWAYOnProtocolError_EvenStreamID` or in a sibling file.** The existing test is in `conn_test.go`. **Recommendation: alongside, in the same file.** Easy to find; same test peer; same fixture pattern. The planner records the choice in PLAN.md.

## 11. Risks and mitigations

### 11.1 Phase-splitting risk (mid-execution)

**Risk:** 05.2 is the largest sub-phase since 05.1, scoped at ~12–15 TDD tasks + ~1300–1700 LoC (`client.go` ~400 LoC + flow-control tightening ~200 LoC + cluster-side parser ~250 LoC + `routerActionH2` ~150 LoC + fixture 0004 ~600 LoC + helpers + ADRs). The phase-2 split gate (~25 tasks OR ~1500 LoC) may trip when the planner writes `PLAN.md`.

**Mitigation:** the SPEC is *split-friendly* if needed. Three plausible split axes:

- **Split by surface within 05.2.** 05.2.1 = client-side codec + `DialH2` + `HttpProtocolOptions` parser + ADR-0055 (the load-bearing flow-control fix); 05.2.2 = `routerActionH2` + fixture 0004 + ADR-0057. Disadvantage: 05.2.1 has no differential fixture, only unit-test coverage (gate (a) vacuously green for 05.2.1; the integration check happens only in 05.2.2).
- **Split by ADR scope.** 05.2.1 = ADR-0055 (flow-control discipline tightening, which touches 05.1 codec primitives only — no `client.go`, no `DialH2`); 05.2.2 = the rest of 05.2 (everything depending on the upstream surface). Advantage: 05.2.1 lands as a clean codec-fix sub-phase that closes 05.1's REVIEW Importants without introducing the upstream surface; 05.2.2 inherits the cleaned-up codec primitives. Disadvantage: introduces a third sub-phase under phase 05, which the BOOTSTRAP §6 discipline tolerates but the project hasn't done before.
- **Defer the integration test for the monotonic-id-reuse branch.** If the LoC budget is tight, the small test addition (per §12 + §10 #10) carries forward to a future codec-hardening phase. Saves ~30 LoC; not a meaningful split.

**Recommendation to planner:** run the LoC + task estimate. If under threshold (likely), do not split. If over: prefer the **split-by-ADR-scope** axis (05.2.1 = ADR-0055 only; 05.2.2 = rest) because it leaves a cleanly-reviewable codec-fix sub-phase. **Avoid** the split-by-surface-within-05.2 axis because 05.2.1 in that form has no differential fixture and the project has already had one sub-phase (05.1) with vacuous gate (a); two in a row is a process smell.

### 11.2 Flow-control discipline tightening regresses 05.1's h2spec gate

**Risk:** ADR-0055's tightening touches the flow-control primitives (`flow.go`/`conn.go`/`stream.go`). h2spec section 5 (Streams and Multiplexing) and 6 (Frame Definitions including 6.9 WINDOW_UPDATE) cover the on-wire correctness; a subtle bug in the new `reserveBlocking` or in the half-window WINDOW_UPDATE policy could regress 53/53 to <53.

**Mitigation:** the regression tests for ADR-0055 (per §1 #6) cover the specific discipline boundaries (>16 KB body, `INITIAL_WINDOW_SIZE: 1`, >65 KB inbound, overflow). The h2spec gate is run as part of gate (c) on every CI build of 05.2. The state-3 re-entry pattern (per BOOTSTRAP §5.2) is the recovery mechanism: if 05.2's first impl pass regresses h2spec, the verifier re-enters at state 3 (not state 4) for the fix.

### 11.3 `Cluster.DialH2` ALPN-confirm race against TLS handshake completion

**Risk:** `Cluster.DialH2` reads `NegotiatedProtocol` immediately after `Cluster.Dial(ctx)` returns. If a future refactor of `Cluster.Dial` returns a `*stdtls.Conn` before the handshake completes (it doesn't today — phase-03's `Cluster.Dial` calls `tls.Handshake` synchronously per the existing code), the ALPN read returns `""` and the H2 dial errors with `alpn negotiated "", want "h2"`.

**Mitigation:** the phase-03 `Cluster.Dial` handshake-completion contract is asserted by phase-03's tests (the dial returns only after a successful handshake). 05.2 adds a defensive `tlsConn.HandshakeContext(ctx)` no-op call in `DialH2` before reading `NegotiatedProtocol`, mirroring the 05.1 SPEC §11.6 mitigation in `Filter.Handle`. The call is idempotent for already-completed handshakes; if a future refactor changes the dial discipline, `DialH2` still gets correct data.

### 11.4 Per-request fresh upstream dial increases latency under load

**Risk:** per-request TCP+TLS+H2 handshake on every router invocation is more expensive than connection pooling. Under load, latency increases linearly with request rate; tail latency suffers from TLS handshake variance.

**Mitigation:** per ADR-0056, this is *intentional* in 05.2 — pooling is the upstream-robustness family's deliverable. The fixture 0004 workload is 27 sequential requests, so the overhead is bounded (~270ms for 27 TLS handshakes at ~10ms each); the differential gate doesn't measure latency (per BOOTSTRAP §7.2's "Timing" row). Production guidance: pooling is required for production workloads, which 05.2's project state doesn't yet support — phase 09+ (upstream-robustness family) is the load-bearing roadmap item.

### 11.5 Fixture 0004 PKI generation in CI

**Risk:** the `pki/gen/main.go` generator is run via `go generate` at fixture build time; if the generator drifts from the committed PEMs (e.g. someone runs `go generate` locally and commits stale outputs), the differential breaks unpredictably.

**Mitigation:** the committed PEMs are deterministic (the generator uses fixed seeds for the RSA keys per the fixture-0002 precedent); CI does NOT run `go generate` automatically — the committed PEMs are used as-is. The README documents the regeneration procedure (`go generate ./test/fixtures/0004-h2-routing/...` + commit). Matches the fixture-0002 discipline exactly.

### 11.6 Master SPEC §5.8 "build error on auto_config" vs 05.2 SPEC's silent-ignore

**Risk:** a reviewer reading the master SPEC §5.8 expects `auto_config` to error at build; this 05.2 SPEC narrows to silent-ignore (per §5.5 + §9). The mismatch could surface as a SPEC-vs-implementation gap if the planner / reviewer inadvertently re-derives from the master SPEC.

**Mitigation:** §5.5 of this SPEC explicitly notes the narrowing rationale (consistency with phase-04's silent-ignore discipline + forward-compat as more discriminators land in later phases). The narrowing is a small-mechanical-design-decision and does not warrant its own ADR — the SPEC document itself is the durable D-3.4 record (per BOOTSTRAP §4 layout, this SPEC lands under `docs/envoy-go/phases/05.2-upstream-h2/SPEC.md` and is committed as part of phase-05.2's history; future readers find the rationale by reading §5.5 of this SPEC, not by chasing an ADR). The unit-test cases (per §4.1 / §8.2) explicitly assert the silent-ignore behaviour — `auto_config` builds without error. The master SPEC remains read-only history (per BOOTSTRAP §6); the 05.2 SPEC is the active design for 05.2's surface and supersedes the master SPEC where it deviates.

### 11.7 The flow-control ADR-0055 is the largest single design concern in 05.2

**Risk:** ADR-0055 bundles seven distinct fixes across three production files (`conn.go`, `flow.go`, `stream.go`) plus a cosmetic helper extraction (`framer.go`). A bundled ADR is harder to review than per-fix ADRs; a future supersession that wants to re-litigate (say) just the inbound WINDOW_UPDATE half-window threshold has to supersede the entire bundle.

**Mitigation:** the bundle is intentional per the 05.1 REVIEW.md `Recommendation` Path A wording ("a single dedicated ADR documenting flow-control discipline for the from-scratch H2 codec end-to-end"). The seven fixes are interlinked (the `reserveBlocking` collapse is required for the per-stream send-window enforcement to be race-free; the overflow bounds-check is required for the WINDOW_UPDATE emission to be safe; etc.); separating them into per-fix ADRs would create cross-references that are harder to read than the bundled form. The ADR's Consequences section enumerates each fix individually so a future supersession can target precisely.

### 11.8 The ADR-0058 trailer rule asymmetry vs upstream Envoy

**Risk:** envoy-go discards trailers on both downstream and upstream H2; upstream Envoy forwards trailers (relevant for gRPC's `grpc-status` carrier). The differential surface is asymmetric in principle.

**Mitigation:** the fixture-0004 driver does not exercise trailers (bodyless GETs only); the divergence is unobservable on the 05.2 differential gate. ADR-0058 documents the rule and the deferral to phase 07 + gRPC family. `BEHAVIOR_CONTRACT.md ## HTTP/2`'s "Not asserted" subsection explicitly enumerates trailers — a future fixture exercising trailers would be a gRPC-family deliverable, not a 05.2 follow-up. The asymmetry is bounded.

### 11.9 The `routerActionH2`'s 502 local-reply prose body diverges from H1 path

**Risk:** the H1 path's 502 body (set in phase 04's `routerAction.do`) and the H2 path's 502 body (new in `routerActionH2.do`) might diverge in wording, casing, or trailing-newline discipline. The differential allow-list relaxes 502 body bytes (under the phase-04 framing-divergence rule extended for H2 local-reply prose); the divergence is unobservable on the differential gate but visible to operators reading proxy logs and to clients receiving the body.

**Mitigation:** the §4.1 snippet enumerates the exact 502 body wording (`bad gateway\n`); the planner ensures the H2 implementation uses the same constant the H1 path uses (refactor opportunity: extract the 502 body string to a shared constant in `internal/filter/hcm/actions.go`). Unit-test asserts byte-equivalence between the H1 and H2 502 body bytes (a small additional unit test on top of the fixture-0004 differential).

## 12. Phase-05.1 REVIEW carry-forward triage

Phase-05.1 closed with `REVIEW.md` (`d69446a`) verdict APPROVED WITH FOLLOW-UPS; per the recommendation `Path A`, a single follow-up commit closed I-4 (CONFORMANCE_PINS.md refresh procedure) + M-1 (`hpackBlocked` dead code) + M-2 (`validateClientStreamID` dead code) + M-13 (BEHAVIOR_CONTRACT prose tightening) + M-14 (no-match 404 body alignment) + M-15 (ADR-0046 prose correction via ADR-0054 supersession) + M-16 (smoke-only docstring) + M-17 (connection.go fall-through doc comment) + M-6 (fuzzer `errors.Is`). Three Important findings (I-1/I-2/I-3) and six Minor findings (M-3/M-4/M-5/M-7/M-8/M-9/M-10/M-11/M-12) plus one discovered coverage gap carry forward to 05.2 per the 05.1 REVIEW.md `Recommendation` Path A consequences and STATE.md `next-skill-scope`. Phase-05.2 disposition:

### 12.1 Absorbed into ADR-0055 (flow-control discipline)

- **I-1 — `ServerConn.writeData` does not respect `SETTINGS_MAX_FRAME_SIZE`.** Bundled into ADR-0055 / §1 #6.
- **I-2 — `ServerConn.writeData` does not respect per-stream send window.** Bundled into ADR-0055 / §1 #6.
- **I-3 — Receive-side flow control is allocated but never enforced.** Bundled into ADR-0055 / §1 #6. The `recvW` fields (`ServerConn.recvW`, `serverStream.recvW`) become consumed; M-7 disposition is "kept-and-consumed under I-3".
- **M-3 — `writeData` dead `if taken <= 0` recovery branch.** Bundled into ADR-0055 / §1 #6. The `waitFor`+`reserve` collapse into `reserveBlocking` removes the branch. The atomicity fix also removes the latent over-reservation race the dead branch was presumably defending against.
- **M-5 — `framer.readFrameCtx` and `framer.tryReadFrame` duplicate translation block.** Bundled into ADR-0055 / §1 #6 as a cosmetic prerequisite. The new `ClientConn`'s framer wrapper consumes the extracted `translateFramerErr` helper, preventing duplication on the client-side.
- **M-7 — `ServerConn.recvW` and `serverStream.recvW` are dead today.** Disposition: kept-and-consumed under I-3 per ADR-0055.
- **M-9 — `serverStream.recvWindowUpdate` accepts deltas without window-overflow check (also `ServerConn.onWindowUpdate`).** Bundled into ADR-0055 / §1 #6 as a stream-state hardening prerequisite. Both sites get the bounds-check in the same commit.
- **M-11 — `serverStream.recvData` writes to `s.reqBody` *before* checking state-transition validity.** Bundled into ADR-0055 / §1 #6 as a stream-state hardening item. One-line reorder; no behavioural change beyond the memory-waste fix.

### 12.2 Carried forward to later phases (per-finding disposition)

- **M-4 — `readClientPreface` is not ctx-aware.** *Defer to phase 06 or 07.* The 05.1 REVIEW noted "Phase-04 H1 has the same shape on the H1 read path (no regression)"; the proper fix is at the listener-manager level via uniform OS read deadlines, which is a phase 06/07 concern. ADR-0058 (or a separate carry-forward ADR — planner picks) carries the deferral.
- **M-8 — `excludedSubsections []string{"http2/6/6"}` is unused and `//nolint:unused`-suppressed.** *Defer; promote to const or move to `CONFORMANCE_PINS.md` doc-prose.* This is a cosmetic doctrine D-3.4 cleanup that does not depend on 05.2's surface. The planner chooses whether to fold it into the 05.2 PLAN as a small task or carry forward to a future doctrine-cleanup phase. **Recommendation: fold into the 05.2 PLAN as a 5-minute task** since the change is mechanical and the file already changes in 05.2 (the BEHAVIOR_CONTRACT references it via the new "Applies to" enumeration).
- **M-10 — `ServerConn` has no `SETTINGS_TIMEOUT`.** *Defer; flag for 06.* RFC 9113 §6.5.3's "MAY" leaves this optional; the 05.1 REVIEW notes "h2spec sends SETTINGS_ACK promptly" so the gap is dormant. The proper fix lands with the listener-manager's per-conn timeout policies in phase 06 or 08. ADR-0058 (or a separate carry-forward ADR) carries the deferral with a "phase-06-or-08-must-consider" tag.
- **M-12 — `ServerConn.closedStreams` map has no upper bound.** *Defer to a long-lived-conn phase.* The map grows ~24 bytes per closed stream; observable only in long-lived production sessions, which 05.2 doesn't introduce (the per-request fresh upstream conn discipline + the differential's sequential-conn pattern keep stream counts bounded per conn). The fix is to cap at e.g. last 1024 stream IDs (a small ring buffer); the planner chooses whether to fold this into 05.2 as a small task or carry forward. **Recommendation: defer** — fixture 0004 doesn't exercise the long-lived-conn surface; the cap is a hardening-phase item.

### 12.3 Discovered-during-batch coverage gap

- **Integration test for the monotonic-id-reuse rejection branch at `internal/filter/hcm/h2/conn.go:308-319`.** Currently only the even-id branch is covered by `TestServerConn_GOAWAYOnProtocolError_EvenStreamID`. The rejection branch itself is correct and tested at unit level; only the integration coverage is missing. *Land in 05.2's PLAN as a small test-addition task* per §10 #10 above. Adds ~30 LoC; planner-time choice on file location (recommendation: alongside the existing test in `conn_test.go`).

### 12.4 Optional FU-7 future-tightening (out-of-scope unless reason emerges)

- **Elide the empty trailing DATA frame when `bodyText == ""` in `directResponseAction.writeH2`.** The 05.1 follow-up batch (per the 05.1 REVIEW) flagged this as a future-tightening opportunity. *Out-of-scope for both 05.1 and 05.2 unless the 05.2 brainstorming session finds an upstream-H2 alignment reason to fold it in.* Per §5.4 above: fixture 0004's `direct_response` bodies are non-empty (`OK\n`, `not found\n`), so 05.2 has no upstream-H2 alignment motive. **Disposition: confirmed out-of-scope for 05.2.** Carries forward to a future phase if a fixture with empty `body` arises.

### 12.5 Non-inputs (already closed in 05.1's follow-up batch)

The 05.1 REVIEW.md `Recommendation` Path A landing closed I-4 + M-1 / M-2 / M-6 / M-13 / M-14 / M-15 / M-16 / M-17. None of these need SPEC §3 mention in 05.2. Listed here only for the reviewer's audit of which 05.1 REVIEW findings did NOT carry forward.

ADR-0055 (flow-control discipline) is the formal landing for §12.1's eight items (I-1/I-2/I-3 + M-3/M-5/M-7/M-9/M-11). ADR-0058 (trailers) OR a separate carry-forward ADR is the formal landing for §12.2's deferred items (M-4/M-10) — the planner picks whether to bundle or separate at PLAN write time. M-8 + the integration-test gap land as plain task entries in PLAN.md (no ADR; cosmetic / coverage-gap items per ADR-0017 doctrine that "small mechanical fixes do not require ADRs").

Additionally, the 05.1 REVIEW surfaced (as the "single most important context to surface to the phase-05.2 planner") that **the three flow-control discipline gaps (I-1/I-2/I-3) form a coherent ADR-shaped unit.** This SPEC's §1 #6 + §5.6 + ADR-0055 deliver exactly that unit — the SPEC honours the REVIEW's framing.

## 13. Acceptance checklist (for the reviewer of this sub-phase's final state)

A reviewer (phase 05.2's `superpowers:requesting-code-review` subagent) signs off when every item below is verifiable from the on-disk state:

- [ ] All six phase-done gates (a–f) green per §3, with gate (a) **non-vacuous** (fixture 0004 differential green; first non-vacuous gate-(a) on the H2 surface).
- [ ] `internal/filter/hcm/h2/client.go` exists; `ClientConn` + `RoundTrip` + `Close` implemented; reuses 05.1 codec primitives; no `http2.Server` or `http2.Transport` runtime is used by envoy-go runtime code (driver-side use in `cmd/envoy-go/main_test.go`, `test/fixtures/0004-h2-routing/`, `test/conformance/h2spec/`, `test/helpers/h2.go`, and the new `client_test.go` is permitted and grep-verifiable).
- [ ] `internal/cluster/dial_h2.go` exists; `Cluster.DialH2` is the only API for upstream H2 dial; it asserts TLS + ALPN h2; failure paths emit the diagnostics enumerated in §4.1.
- [ ] `internal/cluster/manager.go` parses `typed_extension_protocol_options["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]` per §5.5's behaviour matrix; `Cluster.UseH2()` accessor is grep-verifiable; build-time validation rejects H2-cluster without TLS / ALPN h2 with the diagnostics enumerated in §4.1.
- [ ] Blank import for `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"` exists in `internal/cluster/cluster.go` AND in `internal/bootstrap/bootstrap.go`.
- [ ] `routerActionH2` action variant exists in `internal/filter/hcm/actions.go`; build-time variant selection by `Cluster.UseH2()` is grep-verifiable; `routerActionH2.do` happy path + 502 paths + ctx-cancel path covered by unit tests.
- [ ] ADR-0055's seven fixes (I-1 chunking, I-2 per-stream send-window, I-3 inbound WINDOW_UPDATE, M-3 `reserveBlocking`, M-9 overflow bounds-check, M-11 state-before-append, M-5 `translateFramerErr` helper) are individually grep-verifiable; regression tests for each are present in `internal/filter/hcm/h2/`'s test files.
- [ ] `BEHAVIOR_CONTRACT.md ## HTTP/2` is edited in place per ADR-0052; the "Asserted equivalence" + "Not asserted" + "Header allow-list extensions" + "Applies to" + "Does not yet apply to" subsections reflect the 05.1 + 05.2 unified scope per §5.7. The header allow-list table at the top of the file has its `:method`/`:path`/`:scheme`/`:authority` rows flipped to `applies-to: phase 05.2 routed-to-upstream H2 (active per ADR-0057)`.
- [ ] All four 05.2 ADRs (the planner-assigned ADR-0055..ADR-0058 mapping to flow-control discipline / fresh upstream dial / closes-ADR-0035-H2-leg / trailers-not-forwarded) appear in `DECISIONS.md` with full Context/Decision/Consequences sections per ADR-0001's template. The ADR-numbering-shift discipline from ADR-0045 + ADR-0004 is honoured (the planner verified next-free at write time and the four numbers are contiguous).
- [ ] Fixture `0004-h2-routing/` is committed in full: `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` + `pki/gen/main.go` + `pki/*.pem` + `driver/driver.go` + `driver/driver_test.go` + `backends/main.go`. The fixture's PKI is deterministic + committed; the `--concurrency 1` reference invocation is honoured.
- [ ] `test/helpers/h2.go` + `h2_test.go` exist; `H2RoundTrip` helper signature matches §4.3; consumed by the fixture-0004 driver.
- [ ] `test/conformance/h2spec/` is UNCHANGED; pin still at the ADR-0051 SHA; 53/53 PASS.
- [ ] No phase-04 or 05.1 fixture (`0000`/`0001`/`0002`/`0003`) regressed under the unrestricted `go test ./test/differential/...` run.
- [ ] `STATE.md` is at lifecycle-state 6 (or appropriate end state for the 05.2 sub-phase); `ROADMAP.md` row 05.2 is `done`; row 05 (parent) is `done` (flipped at the same phase-done commit); row 06 is `planned`. The §5.3 phase-done commit's message names every ADR introduced or referenced.
- [ ] `PROGRESS.md` quotes the command outputs of all six gates per the §5.3 verification protocol; SHA-fill for each task entry per the phase-04 + 05.1 convention.
- [ ] The phase-05.1 REVIEW carry-forward triage (§12) is faithfully recorded: the eight §12.1 items are landed in ADR-0055; the §12.2 deferred items are noted in PLAN.md (and in ADR-0058 or a separate ADR per the planner's choice); the §12.3 integration-test for monotonic-id-reuse is committed.
- [ ] The 05.1/05.2 boundary is now closed: `internal/filter/hcm/h2/client.go` exists; `internal/cluster/dial_h2.go` exists; `test/fixtures/0004-h2-routing/` exists; `BEHAVIOR_CONTRACT.md ## HTTP/2` no longer enumerates "routed-to-upstream H2" or "fixture 0004" in its "Does not yet apply to" subsection.
- [ ] The integration-test for the monotonic-id-reuse rejection branch at `internal/filter/hcm/h2/conn.go:308-319` is committed (`TestServerConn_GOAWAYOnProtocolError_StreamIDReuse` or similar — planner-named).

When all boxes above are checked, phase 05.2 is `done`, phase 05 (parent) is `done`, and the project advances to phase 06 (observability-baseline) at lifecycle-state 1.
