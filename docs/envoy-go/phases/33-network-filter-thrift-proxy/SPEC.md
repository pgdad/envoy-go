# Phase 33 SPEC — `thrift_proxy` network filter (`envoy.filters.network.thrift_proxy`): a single-pair single-route terminal routing proxy (framed×binary, payload_passthrough) on the REUSED ADR-0230 upstream-pool seam — the project's SECOND terminal proxy; the §9 family-CLOSING row

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (PLAN authoring; SKILL_ROUTING state 2 → 3). This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks. This is a SINGLE flat §9 row (directly executable; SPEC → PLAN → IMPL), NOT a parent pre-split (contrast phase 32). The pre-authorized 33.1/33.2 split axis stays unconsumed (§3.0 / D-T9a). After phase 33 phase-done the §9 Network-filters family CLOSES.

**Goal:** Land `envoy.filters.network.thrift_proxy` — the project's SECOND terminal routing proxy (after redis_proxy) — at a single-pair single-route-terminal MVP: decode the Thrift message-begin envelope (method name + message type + `seq_id`) for ONE transport×protocol pair (**framed × binary**, pinned live — D-T2) under `payload_passthrough`, route by method name through a `route_config` exact-match/match-all route to ONE upstream cluster, round-trip each request through the **REUSED ADR-0230 upstream-pool seam** (one-conn-per-downstream, synchronous single-flight, positional correlation), answer a routing miss with a LOCAL `UnknownMethod` Thrift exception (zero upstream — the redis-PING analogue, D-T5), and emit the global `thrift.<stat_prefix>.` request/response stat roster — in ONE flat phase, **framework-ZERO-touch** (the FIRST reuse of a prior framework seam unchanged), ZERO new go.mod dep (thrift_proxy is a CORE `/envoy` extension — D-T1).

**Architecture:** A NEW `internal/filter/network/thriftproxy/` package implements `network.TerminalFilter` (NOT `ReadFilter`/`WriteFilter` — it TERMINATES the downstream connection via `Handle(ctx, conn)`, it does not observe a `tcp_proxy`-owned chain — the redis_proxy precedent): an in-house Thrift codec (`thrift.go` — framed-transport frame decode [4-byte BE length prefix] + binary-protocol message-begin decode [method name + message type + `seq_id`] + the opaque struct-skip-for-passthrough + the reply-frame classifier + the local `AppException` encoder) + a `route_config` method-routing table + a `TerminalFilter.Handle` request→reply pump (decode message-begin → match the route by method name → on a MISS write a local `UnknownMethod` exception → on a HIT round-trip the raw request frame through the REUSED `UpstreamConn` seam and forward the raw reply frame downstream). The seam (`internal/filter/network/upstreampool.go`, ADR-0230) is REUSED UNCHANGED — thrift is its anticipated SECOND consumer; phase 33 adds ZERO framework-seam code. The EAGER 25-name roster (24 counters + 1 gauge `request_active`) lands under `thrift.<stat_prefix>.`; the `request_time_ms` histogram is deferred (ADR-0060); the 2 close-direction counters are created-but-never-incremented (the framework-gap coverage boundary, D-T8). The differential proof is TWO-pronged (the §9 SECOND row whose downstream RESPONSE bytes are load-bearing — thrift_proxy GENERATES them by proxying): downstream-Thrift-response byte-equivalence PLUS cross-side `StatsAsserter` parity.

**Tech stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227; thrift_proxy is a CORE extension present in both the standard and contrib images — the project boots the contrib image, a behavioral superset). go-control-plane **`/envoy` v1.32.4** (ADR-0008 — thrift_proxy/v3 is a CORE `/envoy` subpackage, where redis_proxy/v3 lives; **ZERO new go.mod dep** — D-T1). Reuses `internal/filter/network/` (26.1/26.2 `TerminalFilter` seam + the ADR-0230 upstream-pool seam, REUSED unchanged), `internal/cluster/` (`Manager.Get` + `Cluster.Dial`), `internal/filter/network/redisproxy/` (the terminal-routing-proxy package SHAPE), `internal/stats/` (06.1 counters + gauges + `IsValidName`; `internal/stats/name.go` tag-extractor — the NEW `thrift.` SINGLE-label-hoist arm, the redis-32 shape), the differential harness + `StatsAsserter`. ZERO framework-seam extension.

**Authored:** 2026-06-09. **Empirical-pin probe date:** 2026-06-09.

---

## 1. Purpose / Mission

Phase 33 lands `thrift_proxy` (`envoy.filters.network.thrift_proxy`, proto `envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy` in the CORE `/envoy v1.32.4` module), the **SEVENTH §9 Network-filters-family row** (after the phase-26 family-parent + first three filters, the phase-27 `sni_cluster` flat row, the phase-28 `zookeeper_proxy` parent, the phase-29 `mongo_proxy` parent, the phase-31 `kafka_broker` flat row, and the phase-32 `redis_proxy` parent; the phase-30 contrib pin-refresh was an infra row, not a family member). It is the project's **SECOND terminal routing proxy** (after redis_proxy at phase 32): it IS the connection terminator (no `tcp_proxy` behind it → no observational MVP), the **FIRST row to REUSE a previously-built framework seam unchanged** (the ADR-0230 upstream-pool seam, validating its redis-scoped YAGNI sizing), and the **LAST §9 Network-filters candidate** — its phase-done CLOSES the §9 Network-filters family.

This SPEC refines the phase-33 BRAINSTORM (`docs/envoy-go/phases/33-network-filter-thrift-proxy/BRAINSTORM.md`, Q0/Q1/Q-seam/Q-split decisions) against the AS-BUILT §9 framework + the §10 D-T1..D-T9 empirical pins EXECUTED IN-SESSION (parallel-subagent fan-out) against (1) the live contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2`, (2) go-control-plane `/envoy` v1.32.4 bindings, and (3) upstream Envoy v1.37.2 CORE source (`source/extensions/filters/network/thrift_proxy/`). It anchors the ADR-0231 §Context draft into DECISIONS.md (§Decision/§Consequences body lands at the IMPL per ADR-0044).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D-T1..D-T9 scrape CONFIRMED the SPEC-blocking pins (D-T1 TypeURL + dep; D-T2 the framed×binary round-trip ran live; D-T5 the local-reply semantics) and REFINED/REFUTED several BRAINSTORM hypotheses. The load-bearing amendments, each carried into the relevant §§ below:

- **AMEND-T1 (D-T1 — TypeURL + ZERO new dep; CONFIRMS BRAINSTORM).** `proto.MessageName(&thrift_proxyv3.ThriftProxy{})` (run in a throwaway `/tmp/dt1probe` module) = `envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy` → `@type` = `type.googleapis.com/envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy` (carries the `extensions.` segment per `reference_network_filter_typeurl_extensions`). thrift_proxy/v3 is a subpackage of the ALREADY-DIRECT `/envoy v1.32.4` module (exactly where `redis_proxy/v3` lives — CORE, NOT `/contrib`); its transitive imports (`config/core/v3`, `config/route/v3`, `config/accesslog/v3`, `cncf/xds/go`, `protoc-gen-validate`, `wrapperspb`, `anypb`) are ALL already in the envoy-go closure → **importing thrift_proxy/v3 adds ZERO new go.mod module requirements** (the redis D32-1 posture, the OPPOSITE of kafka_broker's first-`/contrib` dep). Go package alias `thrift_proxyv3`. The `route.pb.go` route-config bindings ship in the same subpackage. See §5.1 / §11.1.
- **AMEND-T2 (D-T2 — framed×binary is the pinned pair; the round-trip ran LIVE; CONFIRMS BRAINSTORM).** A `[thrift_proxy {transport: FRAMED, protocol: BINARY, payload_passthrough: true, route_config: match-all → cluster}]` listener BOOTED clean on the contrib reference image (first try; the working YAML is §11.2) and a hand-crafted framed-binary CALL round-tripped through it to a canned backend and back. The wire format (FRAMED = 4-byte big-endian frame-length prefix; BINARY strict message-begin = `0x80 0x01 0x00 <msgtype>` + i32 method-name-length + method-name bytes + i32 `seq_id`, `MinMessageBeginLength=12`; msgtype `Call=1`/`Reply=2`/`Exception=3`/`Oneway=4`) is transcribed in Appendix A. Under `payload_passthrough` the decoder reads ONLY the message-begin then consumes the struct body as opaque bytes (`request_passthrough`/`response_passthrough` increment). AUTO transport/protocol auto-detects a framed-binary frame identically, so explicit FRAMED+BINARY and AUTO produce the same decode for the MVP traffic. See §3.2 / §7 / Appendix A.
- **AMEND-T3 (D-T3 — the roster is EAGER + table-bounded (24 counters + 1 gauge + 1 histogram), and the Prometheus arm is SINGLE-label-HOIST; REFINES + REFUTES the BRAINSTORM's hypothesized roster).** Source `ALL_THRIFT_FILTER_STATS` (`stats.h`) = 19 COUNTERs + 1 GAUGE (`request_active`, Accumulate) + 1 HISTOGRAM (`request_time_ms`, Milliseconds); the live `/stats` adds 5 ROUTER counters (`route_missing`, `unknown_cluster`, `no_healthy_upstream`, `upstream_rq_maintenance_mode`, `shadow_request_submit_failure`) under the same `thrift.<sp>.` scope → **24 counters + 1 gauge + 1 histogram = 26 names** present-at-0 from boot, ALL keyed on the FIXED roster (NOT per-method-dynamic — the method name drives ROUTING, not a counter). REFUTES the BRAINSTORM's hypothesized members: there is **NO `response_business_exception`** in v1.37.2; the roster ADDS `request_passthrough`/`response_passthrough` (payload_passthrough counters), `request_internal_error`, `downstream_cx_max_requests`, `downstream_response_drain_close`, and the 5 router counters. The Prometheus exposition is **SINGLE-label-HOIST**: `envoy_thrift_<leaf>{envoy_thrift_prefix="<stat_prefix>"}` (live-quoted in §11.3) — the redis-32 / mongo `.rbac.` TAG-EXTRACTOR shape, NOT the kafka INLINE shape. envoy-go mirrors EAGER creation at config parse over the fixed roster (the redis/kafka precedent); the histogram is DEFERRED (ADR-0060); the 2 close-direction counters are created-but-never-incremented (AMEND-T6). Stat surface **1091 → 1116 (+25)**. See §7.
- **AMEND-T4 (D-T5 SPEC-BLOCKING — a routing MISS is answered LOCALLY with a Thrift `UnknownMethod` EXCEPTION, ZERO upstream; a route HIT proxies; CONFIRMS the local-reply hypothesis — the redis-PING analogue).** Live-confirmed: a config whose only route matches method `onlythis`, driven with method `somethingelse`, returned a downstream EXCEPTION frame (`msgtype 3`, `AppExceptionType::UnknownMethod`=1, message `no route for method 'somethingelse'`) with `cluster.<name>.upstream_cx_total`/`upstream_rq_total` staying **0** (no backend dial). Source (`router/router_impl.cc`): a null route → `sendLocalReply(AppException(UnknownMethod, "no route for method '{}'"))`; a decoding error after a usable message-begin → a local `ProtocolError`(=7) exception + `FlushWrite` close. On the miss path the live stats moved `route_missing`+`response_exception` (and `request*` did NOT move — the miss is accounted only via those two; the message is never counted as a serviced call). This is BLOCKING because it determines BOTH the `0057` fixture's exercised methods AND the round-trip vs local-reply stat counts. The exact `UnknownMethod` exception byte layout (captured live) is in Appendix A. See §3.3 / §7.3 / §8.1.
- **AMEND-T5 (D-T6 — the reference REMAPS the upstream `seq_id` to 0 and RESTORES the original downstream; correlation is POSITIONAL not seq_id-keyed; the MVP passes `seq_id` through — downstream byte-equivalence holds; REFINES the BRAINSTORM §2.4).** Live: the reference rewrote the upstream `seq_id` to 0 (the backend received `seq_id=0`) and mapped it back to the original (1) on the downstream reply. Source: the router holds a single `upstream_request_` (one in-flight RPC per upstream conn) and correlates POSITIONALLY (no `seq_id`-keyed map; `router_impl.cc` does no seq_id rewrite — the remap is the conn-manager/upstream-request layer's per-connection transaction-id mapping). envoy-go's one-conn-per-downstream synchronous single-flight MVP **passes `seq_id` through unchanged** (no remap needed — single-flight makes positional correlation trivially correct), so the downstream reply carries the ORIGINAL `seq_id` on BOTH sides → downstream byte-equivalence holds. The UPSTREAM `seq_id` differs per-side (reference 0 / subject passthrough) but the upstream bytes are never asserted (the canned backend echoes whatever it receives; only the DOWNSTREAM reply is the differential proof). The `seq_id` is decoded + may be echo-validated, but is NOT demux-load-bearing (the §2.4 decision validated). See §3.2 / §8.3.
- **AMEND-T6 (D-T8 — the close-direction counters `cx_destroy_local/remote_with_active_rq` DO fire on the LIVE routing-miss path; the MVP creates-but-never-increments them as the framework-gap coverage boundary; SHARPER than redis-32, which never exercised them).** Live: on the routing-miss local-reply path `thrift.<sp>.cx_destroy_local_with_active_rq` went to **1** (the reference `FlushWrite`-closes after the local exception with the rq still active) + `downstream_response_drain_close` went to 1; on the SUCCESS round-trip both close-direction counters stayed 0. The network framework records close TYPE not DIRECTION (`reference_close_direction_framework_gap`); keying close counters by direction is a framework-SURGERY sub-phase (deferred project-wide — the §9 zero-touch AMEND-R9 posture). The MVP EAGER-creates both counters (present-at-0 roster parity) but NEVER increments them, and the MVP's local-reply path KEEPS THE DOWNSTREAM CONNECTION OPEN (it does not `FlushWrite`-close after the exception — the framework-zero-touch choice). On the `0057` miss arm the reference moves the TWO counters `cx_destroy_local_with_active_rq`+`downstream_response_drain_close` while the subject keeps them at 0 → those two are asserted PER-SIDE (subject==0; NOT cross-equal), a documented BEHAVIOR_CONTRACT coverage boundary (the redis D-P32-9 / mongo D-P4 close-direction precedent). `cx_destroy_remote_with_active_rq` stays 0 on BOTH sides for the MVP traffic (no remote-close-with-active-rq path is exercised) — it is created-but-never-incremented like its local twin. See §7.2 / §7.6 / §8.1.
- **AMEND-T7 (D-T4 — only `stat_prefix` is a hard PGV gate; `route_config` is NOT PGV-required; the un-chosen transport/protocol pairs are envoy-go-strict DEPARTURE rejects, NOT cross-side boot-rejects).** Live `--mode validate`: omitting `stat_prefix` REJECTS with `Proto constraint validation failed (ThriftProxyValidationError.StatPrefix: value length must be at least 1 characters)`; omitting `route_config` VALIDATES OK (an absent route table simply means every request routing-misses → local exception). The proto's `transport`/`protocol` enums are PGV `defined_only` only (any defined enum value is parse-accepted — so the reference parse-ACCEPTS `UNFRAMED`/`HEADER`/`COMPACT`/`LAX_BINARY`/`TWITTER` and decodes them at runtime). The MVP supports only `framed×binary` (+ AUTO), so an explicit un-chosen transport/protocol value is an **envoy-go-strict DEPARTURE reject** at config-load (`thrift-proxy-unsupported-transport`/`thrift-proxy-unsupported-protocol`) — UNIT-TESTED, NOT a cross-side `0058` boot-reject arm (the reference boots fine with those values). The load-bearing `0058-thrift-boot-reject` arm is the missing-`stat_prefix` PGV reject. See §6 / §8.2.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0230** (the upstream-pool seam, phase 32.1); next-free **ADR-0231**. This SPEC anchors the ADR-0231 §Context draft (the thrift_proxy filter — ONE ADR; NO parent-umbrella ADR since no split, NO seam ADR since the ADR-0230 seam is REUSED unchanged — contrast redis-32's TWO ADRs). The §Decision/§Consequences body lands at the phase-33 IMPL per ADR-0044. No ADR number is consumed beyond the §Context draft. The ADR-0209 escape-valve reserve carried from the §9 family STANDS-UNCONSUMED. All nine D-T pins are RESOLVED this session (§11); the remaining open items are PLAN/IMPL D-questions (§12), not empirical pins.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **The rest of the 3×3 transport×protocol matrix** — the un-chosen transports (`UNFRAMED`, `HEADER`) + protocols (`LAX_BINARY`, `COMPACT`, `TWITTER`) + the `AUTO` detectors for non-framed-binary traffic. The MVP decodes ONE pair (framed×binary; AUTO accepted because it auto-detects framed-binary identically). An explicit un-chosen value is an envoy-go-strict DEPARTURE reject (AMEND-T7). A future codec-breadth sub-phase.
- **Full struct-body parsing (`payload_passthrough` off)** — the MVP routes on the message-begin envelope and forwards the struct body opaquely (passthrough mode). Full recursive struct parsing (the field-type-keyed skip walk for `payload_to_metadata` / header-routing on payload fields) is DEFERRED; `payload_passthrough: false` (the full-parse mode) is parse-accepted-behavior-deferred (the MVP still message-begin-decodes + opaque-forwards, but does NOT emit the `request_passthrough`/`response_passthrough` counters and is a documented coverage boundary). A future struct-parsing sub-phase.
- **Full `route_config` richness** — `service_name` matches, `headers` matches, `invert`, multiple routes beyond a single exact-match/match-all leg, `weighted_clusters`, `cluster_header`, `request_mirror_policies`, `strip_service_name`, `metadata_match`, `Trds` (xDS dynamic route discovery). The MVP consumes a single `method_name` exact-match (empty `""` = match-all) → ONE `cluster`. The rest is PARSE-ACCEPTED-behavior-DEFERRED (the redis `prefix_routes.routes[]` precedent). A future routing sub-phase.
- **The nested thrift-filters beyond the router** — `envoy.filters.thrift.rate_limit`, `envoy.filters.thrift.header_to_metadata`, `envoy.filters.thrift.payload_to_metadata`. The MVP consumes the router (the implicit terminal — auto-appended when `thrift_filters` is OMITTED, D-T5/§11.2); a non-empty `thrift_filters` list is PARSE-ACCEPTED-behavior-DEFERRED. A future thrift-filter-chain sub-phase.
- **seq_id-keyed multiplexing / shared per-host pool / deep pending queue / two-goroutine pipelining** — the ADR-0230 deferred surface. The MVP REUSES the one-conn-per-downstream synchronous single-flight model (passing `seq_id` through, AMEND-T5); out-of-order multiplexing matched by `seq_id` is a future thrift latency/fan-out sub-phase that EXTENDS the seam (exactly the surface ADR-0230 names).
- **The close-direction-keyed counters** (`cx_destroy_local_with_active_rq` / `cx_destroy_remote_with_active_rq`) — created-but-never-incremented (AMEND-T6, D-T8); the framework-surgery sub-phase that would key close counters by direction is deferred project-wide (`reference_close_direction_framework_gap`). A BEHAVIOR_CONTRACT coverage boundary; asserted per-side on the `0057` miss arm.
- **`max_requests_per_connection`** (+ its `downstream_cx_max_requests` counter) — connection-recycling; parse-accepted-deferred; the counter exists-at-0.
- **`access_log` / `header_keys_preserve_case`** — observ-only / header-transport casing; parse-accepted-deferred.
- **`request_time_ms` HISTOGRAM** — deferred per ADR-0060 (project-wide histogram deferral); BEHAVIOR_CONTRACT coverage-boundary record. (There is exactly ONE thrift histogram.)
- **Runtime-key gating** — envoy-go has no runtime layer; the filter behaves at key defaults (envoy-go-strict departure; the Runtime + hot-restart family row is the future home).
- **Real-Thrift-server integration fixtures** — out of scope; the hermetic synthesized `TCPThriftResponder` only (a real Thrift server adds container weight + a real service definition + nondeterministic framing; the probe used a hand-built canned-REPLY raw-socket responder ONLY to pin the reference, and that responder IS the fixture-backend template).
- **Per-route applicability** — the ThriftProxy proto has NO `*PerRoute` message (`grep -rniE PerRoute` over thrift_proxy/v3 → nothing; D-T1/§11.1). thrift_proxy's `route_config` is its OWN internal method-routing table, not the HTTP route-config `typed_per_filter_config` mechanism. The ADR-0125 roster is untouched.
- **The remaining protocol proxies** — NONE. Thrift is the LAST §9 candidate; after phase 33 phase-done the §9 Network-filters family CLOSES.

---

## 3. The `thrift_proxy` filter package + framework reuse (ADR-0231)

### 3.0 Split disposition — D-T9a RESOLVED (single flat phase; the pre-authorized 33.1/33.2 split stays unconsumed)

ADR-0045 split-gate fires at `> ~25 tasks OR > ~1500 production LoC`. Phase-33 surface (re-estimated against the §11 findings; the seam is REUSED → zero new seam LoC, the load-bearing driver of redis's 2-way split is GONE):

| Unit | Anticipated production LoC |
|---|---|
| `thriftproxy` package: TypeURL + config parse (`stat_prefix`/`transport`/`protocol`/`route_config`/`thrift_filters`/`payload_passthrough` + PGV arms + the 2 departure rejects) | ~150–230 |
| Thrift codec (`thrift.go` — framed frame decode + binary message-begin decode + opaque struct-skip-for-passthrough + reply-frame classifier + the `AppException` encoder; ONE transport×protocol pair) | ~250–420 |
| The `route_config` method-routing table + match (single exact-match/match-all → one cluster) | ~60–110 |
| Filter glue (`TerminalFilter.Handle` request→reply pump + the seam round-trip consumption + the local-reply path) | ~180–280 |
| The EAGER 25-name roster (`stats.go`) + the inc accessors | ~110–170 |
| `internal/stats/name.go` `thrift.` SINGLE-label-hoist arm (AMEND-T3) | ~25–40 |
| 11th `builtins.RegisterBuiltins` registration + `bootstrap.go` blank-import (the `/envoy` path) | ~15–25 |
| Fixtures `0057`/`0058` drivers + the new `TCPThriftResponder` BackendKind | ~350–550 (test) |
| The 42nd fuzzer `FuzzThriftDecode` | ~60 |

Net production ~790–1275 LoC, ~16–20 tasks — BOTH axes comfortably UNDER the gate. **Single flat phase 33 — no pre-split.** The closest precedent is **kafka-31** (a §9 protocol filter with NO new framework seam → a single flat row, ~805–1200 LoC / 13–18 tasks). The pre-authorized **33.1-codec / 33.2-stats** split axis (33.1 = the codec + config parse + a minimal match-all round-trip on the reused seam proven via differential; 33.2 = the full stat roster + the `thrift.` prom arm + the differential matrix + the fuzzer + the parent ROLLUP) STAYS UNCONSUMED (the kafka-31 / mongo-29.1 "pre-authorized split stands unconsumed" precedent). The PLAN re-checks the gate at PLAN time per ADR-0045.

### 3.1 ZERO framework-seam extension — the ADR-0230 upstream-pool seam is REUSED UNCHANGED (the FIRST reuse)

Phase 33 touches NO `internal/filter/network/` framework code. It is a pure consumer of the as-built §9 machinery, and the FIRST row to REUSE the ADR-0230 upstream connection-pool / cluster-routing seam (`internal/filter/network/upstreampool.go`) UNCHANGED:

- the **`TerminalFilter.Handle(ctx, conn)` seam** (26.2) — thrift_proxy IS a terminal filter (the redis_proxy precedent; signature UNCHANGED).
- the **ADR-0230 upstream-pool seam** (`upstreampool.go`, REUSED unchanged): the thriftproxy terminal constructs an `*network.UpstreamConn` via `network.NewUpstreamConn(dial, onRequest)` (the dial closure over the boot-resolved cluster's `Cluster.Dial`, keeping `internal/filter/network` free of an `internal/cluster` import — the `upstreamcluster.go` decoupling; `onRequest` = `cluster.IncUpstreamRqTotal`), then per routed request: `Send(ctx, rawRequestFrame)` (lazy-dials on the first call) + decode the reply frame from `Reader()` + `Close()` in the `Handle` defer. One-conn-per-downstream, synchronous single-flight, positional/FIFO correlation. NO seam modification, NO new exported surface, NO new seam ADR. thrift's `seq_id` is parse-validated but passed-through (NOT demux-load-bearing — AMEND-T5). This is the FIRST reuse of ADR-0230 (validating its YAGNI redis-scoping); the deferred multiplexing surface (seq_id-keyed demux / shared pool / deep queue / pipelining) stays DEFERRED for a future thrift latency/fan-out sub-phase.

**NOT consumed:** the ADR-0221 `network.WriteFilter` seam + the 28.1b post-handoff read seam + the ADR-0226 async halt/resume seam (those govern OBSERVATION of a `tcp_proxy`-terminated chain; a terminal routing proxy owns both ends directly via the upstream-pool seam — the redis_proxy posture); the 29.3 `CloseDirection` machinery (the 2 close-direction counters are created-but-never-incremented — AMEND-T6); the ADR-0217 dynamic-metadata Bucket (thrift_proxy emits none); the `request_time_ms` HISTOGRAM (deferred, ADR-0060).

### 3.2 NEW: `internal/filter/network/thriftproxy/` (Go package `thriftproxy`)

Single-token-joined per the `directresponse`/`snicluster`/`zookeeperproxy`/`mongoproxy`/`kafkabroker`/`redisproxy` precedent. Implements `network.TerminalFilter` (one boot-parsed instance per listener-chain; per-connection state — the `bufio.Reader` + the `*network.UpstreamConn` — lives on `Handle`'s stack, the redis_proxy `filter.go` shape). Anticipated layout (the PLAN finalizes the split):

- `thriftproxy.go` — `TypeURL` (via `proto.MessageName(&thrift_proxyv3.ThriftProxy{})`, pinned by an IMPL Task-1 test, NEVER hand-typed) + `NewFactory`.
- `config.go` — the config parse (`stat_prefix` PGV-required → boot-reject; `transport`/`protocol` accepted iff `∈ {AUTO, FRAMED}×{AUTO, BINARY}` else the departure reject; `route_config` parsed into the method-routing table — NOT required; `thrift_filters` router-consumed / rest parse-accepted; `payload_passthrough` consumed; `max_requests_per_connection`/`access_log`/`header_keys_preserve_case` parse-accepted-deferred) + `stats.IsValidName(stat_prefix)` config-boundary guard.
- `thrift.go` — the Thrift codec: framed-transport frame decode (4-byte BE length prefix; bounds-checked `>0 && <= maxFrameSize`) + binary-protocol message-begin decode (magic `0x8001` + msgtype byte + i32 name-len + name + i32 `seq_id`; Appendix A) + the opaque struct-skip-for-passthrough (forward the remaining frame bytes raw) + the reply-frame classifier (msgtype → `response_reply`/`response_exception`/`response_invalid_type`; for a REPLY, peek the first result-struct field header → `response_success` [STOP/field-id 0] vs `response_error` [field-id ≥ 1]) + the local `AppException` encoder (the `UnknownMethod` exception frame, Appendix A).
- `route.go` — the `route_config` method-routing table (a slice of `{methodName string, cluster string}`; empty `methodName` = match-all) + `match(method) → (cluster, ok)`.
- `stats.go` — the EAGER 25-name roster (24 counters + 1 gauge `request_active`) + the inc accessors (the redis `redisStats` shape).
- `filter.go` — the `TerminalFilter.Handle` request→reply pump (the redis `filter.go` shape: decode message-begin → `request`/`request_call|oneway`/`request_passthrough`/`request_active` inc → match route → MISS: `route_missing`+`response_exception`+write the local `UnknownMethod` exception (keep conn open) → HIT: `Send(rawFrame)` through the seam, decode+classify the reply, `response`/`response_reply`/`response_success|error`/`response_passthrough` inc, forward the raw reply frame downstream).
- `doc.go` — the package doc.

The codec, the route table, and the roster all live INSIDE the package (extract-at-second-consumer; YAGNI). NO new top-level package; NO framework change; the upstream-pool seam is REUSED (§3.1).

### 3.3 The request→reply pump + local-reply semantics (AMEND-T4 / D-T5)

The `Handle` pump (the redis `serveRequest` analogue) per decoded message-begin:

1. **Decode + count the request.** `request` +1; `request_call` (CALL) or `request_oneway` (ONEWAY) +1; `request_passthrough` +1 (when `payload_passthrough`); `request_active` gauge inc (dec on completion). A malformed frame → `request_decoding_error` +1 (+ a local `ProtocolError` exception if the message-begin was decoded, else silent close — the redis `protocol_error` analogue); an invalid msgtype → `request_invalid_type` +1.
2. **Match the route by method name.** On a MISS (no route, or no matching `method_name`): `route_missing` +1, `response_exception` +1, write the local `UnknownMethod` Thrift EXCEPTION frame downstream (Appendix A; echoing the request's method name + `seq_id`), KEEP the connection open (the framework-zero-touch choice — the reference `FlushWrite`-closes + moves `cx_destroy_local_with_active_rq`/`downstream_response_drain_close`, asserted per-side, AMEND-T6). On an unresolvable cluster → `unknown_cluster` +1 (the redis unresolvable-upstream analogue). On a no-healthy-host cluster → `no_healthy_upstream` +1.
3. **Round-trip a HIT.** `Send(ctx, rawRequestFrame)` through the REUSED seam (lazy-dial; `seq_id` passed through, AMEND-T5) → decode the reply frame from `Reader()` → classify (`response`/`response_reply`/`response_success`|`response_error`|`response_exception`/`response_passthrough`/`response_invalid_type`/`response_decoding_error`) → forward the RAW reply frame downstream (byte-equivalent; §8). `request_active` gauge dec.

The MVP fixture pins a SINGLE-method route (`method_name: "Ping"` → one cluster), giving BOTH a route-HIT arm (the driver sends `Ping` → round-trip) AND a route-MISS arm (the driver sends a different method → local `UnknownMethod` exception) from ONE `route_config` (the redis hit/local-reply two-arm shape).

### 3.4 Registration as the 11th built-in + the `/envoy` blank-import (D-T1)

- `internal/filter/network/builtins/builtins.go` — the **11th** `RegisterBuiltins` entry: `reg.Register(thriftproxy.TypeURL, thriftproxy.NewFactory(deps.ClusterManager, deps.StatsRegistry))` (the redis_proxy two-dep shape — thriftproxy needs BOTH the cluster `Manager` (to resolve a route's cluster → `*cluster.Cluster` at `Handle` time) AND the stats registry; both closure-captured from `builtins.Deps`, the network `FactoryCtx` carries neither). Mirror the parallel registration in `cmd/envoy-go/main.go` if it lists them explicitly (IMPL Task-1 greps to confirm — ADR-0072 makes order behavior-neutral).
- `internal/bootstrap/bootstrap.go` — a blank-import of the proto descriptor at the **`/envoy`** (CORE) path: `_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"` (the redis_proxy/v3 blank-import precedent, `bootstrap.go:119`). This holds the proto descriptor registration; it adds NO go.mod dep (the `/envoy` module is already direct — AMEND-T1). The thrift_proxy/v3 subpackage carries both `thrift_proxy.pb.go` and `route.pb.go`, so the one blank-import registers the route-config descriptors too.

### 3.5 REUSES (not new primitives)

- `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b/29.3/32.1) — the `TerminalFilter.Handle` seam, the registry, the `builtins.RegisterBuiltins` seam, the chain runtime, AND the ADR-0230 upstream-pool seam (`upstreampool.go` — REUSED unchanged, §3.1).
- `internal/cluster/` (02/05.2/06.1) — `Manager.Get(name)` host-resolution + `Cluster.Dial(ctx)` (the TCP dial path the same `tcp_proxy`/redis_proxy use) + `IncUpstreamRqTotal` + the per-cluster traffic-stat roster (`cluster.<name>.upstream_cx_total`/`upstream_rq_total`/…, manager.go — the upstream-traffic-stat source the seam reuses; the per-side pooling pin D-T9b).
- `internal/filter/network/redisproxy/` (32.1/32.2) — the terminal-routing-proxy PACKAGE SHAPE precedent (config parse + an in-package codec + the `TerminalFilter.Handle` round-trip pump + the seam consumption + the eager `redisStats` roster + the `redis.` prom arm); thriftproxy mirrors this structure for a different protocol.
- `internal/stats/` (06.1) — counters + **gauges** (the 3rd mirrored-gauge consumer after mongo's `op_query_active` + redis's downstream gauges — thrift's `request_active`) + `NewCounterIfAbsent`/`NewGaugeIfAbsent` (eager-roster idempotent across listeners sharing a `stat_prefix` — the kafka/redis precedent) + `IsValidName` (the config-boundary `stat_prefix` guard; the roster is fixed → no codec-boundary guard needed, D-T7); `internal/stats/name.go` (the NEW `thrift.` SINGLE-label-hoist arm, AMEND-T3 — the redis `redis.` arm shape, `name.go:315`).
- The differential harness + `fixture.StatsAsserter` + the flat `GET /stats` admin endpoint (added at 32.1) + the `GET /stats/prometheus` scrape (+ the fixture-dispatch + asserter-dispatch + `-count=1` break-protocol memory constraints) — booting the contrib reference image (ADR-0227).
- `envoy.extensions.filters.network.thrift_proxy.v3` proto bindings (go-control-plane `/envoy` v1.32.4 — ALREADY a dep, AMEND-T1).
- The freeze-after-boot registry discipline (ADR-0072/0079), the two-step factory (ADR-0079), atomic landing + six-gate (ADR-0052), byte-stable PARSE-REJECT wording (ADR-0080).

---

## 4. Framework primitives — 0 framework-seam extensions (REUSE) + 1 NEW filter package + 0 new go.mod deps

Phase 33 adds NO framework delta (contrast redis-32's 32.1 seam BUILD). Its "newness" is entirely in the `thriftproxy` package + the cross-side differential against the contrib reference Envoy. The framework GROWTH story PAUSES — and CLOSES THE LOOP: phase 33 demonstrates the as-built ADR-0230 upstream-pool seam is sufficient for a SECOND terminal routing proxy with ZERO seam churn, validating the seam's redis-scoped YAGNI sizing at its first reuse (the defer-with-allowance / consume-at-second-consumer discipline at rest). The deferred multiplexing surface (seq_id-keyed demux / shared pool / deep queue / pipelining) WOULD extend the seam, but it is deferred (§2 / §8 of the BRAINSTORM).

---

## 5. Proto-field roster (per §11.1 D-T1 + §11.7 D-T4)

All rosters transcribed from go-control-plane `/envoy` v1.32.4 (`extensions/filters/network/thrift_proxy/v3/thrift_proxy.pb.go` + `route.pb.go` + the `.pb.validate.go` files); verified by `proto.MessageName` run in-session.

### 5.1 TypeURL

`proto.MessageName(&thrift_proxyv3.ThriftProxy{})` = `envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy` → **`@type` = `type.googleapis.com/envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy`** (the `extensions.` segment per `reference_network_filter_typeurl_extensions`; pinned by an IMPL Task-1 `proto.MessageName` test, NEVER the docs string). The filter registration name (the listener filter-chain `name`) is `envoy.filters.network.thrift_proxy`. Go package alias `thrift_proxyv3`; CORE `/envoy` (NOT `/contrib`) — ZERO new go.mod dep (AMEND-T1).

### 5.2 `envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy` (`[#next-free-field: 11]`)

| Go field | proto field | tag | Go type | PGV | 33 disposition |
|---|---|---|---|---|---|
| `StatPrefix` | `stat_prefix` | 1 | `string` | **required `min_len 1`** → `value length must be at least 1 runes` | REQUIRED → boot-reject (the `0058` fixture arm) |
| `Transport` | `transport` | 2 | `TransportType` enum | `defined_only` (default `AUTO_TRANSPORT`=0) | accept `{AUTO_TRANSPORT, FRAMED}`; else departure reject (AMEND-T7) |
| `Protocol` | `protocol` | 3 | `ProtocolType` enum | `defined_only` (default `AUTO_PROTOCOL`=0) | accept `{AUTO_PROTOCOL, BINARY}`; else departure reject |
| `RouteConfig` | `route_config` | 4 | `*RouteConfiguration` | recurse only; **NOT required** | parsed into the method-routing table; absent → all-miss (AMEND-T7) |
| `ThriftFilters` | `thrift_filters` | 5 | `[]*ThriftFilter` | recurse each | OMITTED → implicit router; non-empty list parse-accepted-deferred |
| `PayloadPassthrough` | `payload_passthrough` | 6 | `bool` | none | consumed (true → passthrough-counter inc; §7) |
| `MaxRequestsPerConnection` | `max_requests_per_connection` | 7 | `*wrapperspb.UInt32Value` | recurse only | parse-accept-deferred (`downstream_cx_max_requests` exist-at-0) |
| `Trds` | `trds` | 8 | `*Trds` | recurse only | parse-accept-deferred (xDS dynamic routes) |
| `AccessLog` | `access_log` | 9 | `[]*accesslogv3.AccessLog` | recurse each | parse-accept-deferred |
| `HeaderKeysPreserveCase` | `header_keys_preserve_case` | 10 | `bool` | none | parse-accept-deferred |

**Enums** (both PGV `defined_only`): `TransportType` = `AUTO_TRANSPORT`(0)/`FRAMED`(1)/`UNFRAMED`(2)/`HEADER`(3); `ProtocolType` = `AUTO_PROTOCOL`(0)/`BINARY`(1)/`LAX_BINARY`(2)/`COMPACT`(3)/`TWITTER`(4, deprecated). The MVP pair is `FRAMED`(1)×`BINARY`(1); YAML values are the bare enum names `FRAMED`/`BINARY`. NO `*PerRoute` message exists (§2). The package declares 4 messages (`Trds`, `ThriftProxy`, `ThriftFilter`, `ThriftProtocolOptions`).

### 5.3 `ThriftFilter` + the router

`ThriftFilter{ Name string (PGV required min_len 1 → "value length must be at least 1 runes"); ConfigType oneof{ TypedConfig *anypb.Any } }`. The router's registered name is **`envoy.filters.thrift.router`**; it is AUTO-APPENDED only when `thrift_filters` is COMPLETELY EMPTY/OMITTED (source `config.cc`: `if (config.thrift_filters().empty()) { … default router … }` — there is NO "append-router-if-last-isn't-router" logic). → the MVP OMITS `thrift_filters` to get the implicit default router (a partial list without a terminal router is NOT auto-fixed).

### 5.4 `route.pb.go` — `RouteConfiguration` / `Route` / `RouteMatch` / `RouteAction`

- `RouteConfiguration{ Name string (no PGV); Routes []*Route (recurse, no min_items); ValidateClusters *BoolValue }`.
- `Route{ Match *RouteMatch (**required** → "value is required"); Route *RouteAction (**required**) }`.
- `RouteMatch` — the `match_specifier` oneof is **PGV-required** (`value is required`; typed-nil → `oneof value cannot be a typed-nil`): `MethodName string` (tag 1) | `ServiceName string` (tag 2); + `Invert bool` (3); `Headers []*HeaderMatcher` (4). **Match-all** = `method_name: ""` (the oneof must be present; empty `method_name` has no min_len → it is the wildcard). The MVP consumes `method_name` exact-match (empty = match-all).
- `RouteAction` — the `cluster_specifier` oneof is **PGV-required** (`value is required`): `Cluster string` (tag 1, **`min_len 1`** → "value length must be at least 1 runes") | `WeightedClusters *WeightedCluster` (2) | `ClusterHeader string` (6, min_len 1 + regex). + `MetadataMatch`(3), `RateLimits`(4), `StripServiceName`(5), `RequestMirrorPolicies`(7). The MVP consumes `cluster` (single cluster).

Minimal MVP route_config (the live working YAML, §11.2):
```yaml
route_config:
  name: thrift_routes
  routes:
  - match: { method_name: "" }     # "" == match-all; or an exact method e.g. "Ping"
    route: { cluster: thriftcluster }
```

---

## 6. PARSE-REJECT roster (per §11.7 + ADR-0080)

### 6.1 Wording discipline

Per ADR-0080 byte-stable PARSE-REJECT discipline: each arm is a named constant with byte-stable wording verified by a table test at IMPL. Boot-reject PARITY arms (mirroring an upstream PGV failure) are distinguished from envoy-go-strict DEPARTURE arms. The C++/Go PGV idiom difference holds (`value length must be at least 1 characters` C++ live vs `1 runes` Go binding) — envoy-go's reject wording is its OWN ADR-0080 constant; the boot-reject differential checks BOTH sides reject at boot (a boot-stderr substring matched per-side), not exact cross-impl string equality (the kafka `0054`/redis `0056` precedent).

### 6.2 Boot-reject arms

- `thrift-proxy-stat-prefix-required` — boot-reject PARITY (the `stat_prefix` PGV min-1-rune rule, §5.2). The load-bearing `0058` fixture arm. Reference C++ wording (live `--mode validate`): `Proto constraint validation failed (ThriftProxyValidationError.StatPrefix: value length must be at least 1 characters)`.
- `thrift-proxy-route-match-required` / `thrift-proxy-route-action-required` / `thrift-proxy-route-match-specifier-required` / `thrift-proxy-route-cluster-required` — boot-reject PARITY for a malformed `route_config` route (a `Route` with no `match`/no `route`; a `RouteMatch` with no specifier; a `RouteAction` with no `cluster_specifier` or an empty `cluster`). UNIT-TESTED (these exercise the route-table parse; `route_config` itself is NOT required, so the load-bearing fixture arm is the missing-`stat_prefix` one — the redis `0056` precedent carries only the load-bearing required-field arm).
- `thrift-proxy-thrift-filter-name-required` — boot-reject PARITY for a `ThriftFilter` with an empty `name`. UNIT-TESTED (deferred-feature arm).
- `thrift-proxy-unsupported-transport` / `thrift-proxy-unsupported-protocol` — **envoy-go-strict DEPARTURE** rejects (the MVP supports only `{AUTO, FRAMED}×{AUTO, BINARY}`; an explicit `UNFRAMED`/`HEADER`/`COMPACT`/`LAX_BINARY`/`TWITTER` is rejected at config-load — fail-fast rather than silently-fail-at-decode). UNIT-TESTED, NOT a cross-side `0058` arm (the reference parse-ACCEPTS these enum values and decodes them at runtime — AMEND-T7; a cross-side boot-reject is impossible since the reference boots fine). Recorded in BEHAVIOR_CONTRACT as a departure.
- Framework-level: unknown network-filter `typed_config` type_url → existing boot-reject (no new arm).
- `route_config` ABSENT: NOT a reject (validates OK; every request then routing-misses — AMEND-T7). The subject must NOT reject a routeless config at parse.

---

## 7. Stat surface (per §11.2/§11.3 D-T3 + AMEND-T3/T6)

### 7.1 Scope/naming — `thrift.<stat_prefix>.<leaf>` (AMEND-T3)

Upstream: the filter stats live under `thrift.<stat_prefix>.` (live-probed: `thrift.thriftprobe.request`, `thrift.thriftprobe.request_call`, `thrift.thriftprobe.response_reply`, `thrift.thriftprobe.route_missing`). The roster is a FIXED, table-bounded set (NOT per-method-dynamic — the method name drives ROUTING, not a counter; contrast redis's per-command-dynamic / kafka's per-api-key). envoy-go mirrors this internal naming exactly (the differential `StatsAsserter` + the Prometheus arm depend on it). Because no stat segment is wire-derived, `stats.IsValidName` is satisfied BY CONSTRUCTION (the redis D-P32-7 static-table posture, D-T7); the only config-derived segment (`stat_prefix`) is `stats.IsValidName`-guarded at the config boundary.

### 7.2 The EAGER 25-name roster (24 counters + 1 gauge; created at config parse)

Created EAGER at config parse (the kafka/redis precedent; `NewCounterIfAbsent`/`NewGaugeIfAbsent` idempotent across listeners sharing a `stat_prefix`), giving `thrift.<sp>.*` present-at-0 from boot. The `request_time_ms` HISTOGRAM is DEFERRED (ADR-0060) — NOT created/counted.

| Leaf | Kind | MVP increment |
|---|---|---|
| `request` | COUNTER | per serviced request (HIT path) |
| `request_call` | COUNTER | CALL msgtype |
| `request_oneway` | COUNTER | ONEWAY msgtype |
| `request_passthrough` | COUNTER | when `payload_passthrough` |
| `request_decoding_error` | COUNTER | malformed request frame |
| `request_invalid_type` | COUNTER | invalid request msgtype |
| `request_internal_error` | COUNTER | **exist-at-0** (internal-error path deferred) |
| `request_active` | **GAUGE** (Accumulate) | inc on dispatch / dec on completion |
| `response` | COUNTER | per reply (HIT path) |
| `response_reply` | COUNTER | REPLY msgtype |
| `response_success` | COUNTER | REPLY void/field-id-0 |
| `response_error` | COUNTER | REPLY field-id ≥ 1 |
| `response_exception` | COUNTER | EXCEPTION reply OR a local `UnknownMethod` (miss) |
| `response_passthrough` | COUNTER | when `payload_passthrough` |
| `response_decoding_error` | COUNTER | malformed reply frame |
| `response_invalid_type` | COUNTER | invalid reply msgtype |
| `route_missing` | COUNTER | routing miss → local exception |
| `unknown_cluster` | COUNTER | route cluster unresolvable |
| `no_healthy_upstream` | COUNTER | route cluster has no healthy host |
| `shadow_request_submit_failure` | COUNTER | **exist-at-0** (request mirroring deferred) |
| `upstream_rq_maintenance_mode` | COUNTER | **exist-at-0** (no maintenance mode) |
| `cx_destroy_local_with_active_rq` | COUNTER | **exist-at-0** — created, NEVER incremented (AMEND-T6, D-T8) |
| `cx_destroy_remote_with_active_rq` | COUNTER | **exist-at-0** — created, NEVER incremented (AMEND-T6) |
| `downstream_cx_max_requests` | COUNTER | **exist-at-0** (`max_requests_per_connection` deferred) |
| `downstream_response_drain_close` | COUNTER | **exist-at-0** — created, NEVER incremented (the miss-path drain; per-side, AMEND-T6) |
| **Total created** | **24 COUNTER + 1 GAUGE = 25** | |
| `request_time_ms` | HISTOGRAM | **DEFERRED (ADR-0060)** — NOT created/counted |

### 7.3 Local-reply / round-trip stat accounting (AMEND-T4)

- **Route HIT (round-trip):** `request` +1, `request_call` +1, `request_passthrough` +1, `request_active` +1/−1, `cluster.<name>.upstream_cx_total`/`upstream_rq_total` +1, then `response` +1, `response_reply` +1, `response_success` +1, `response_passthrough` +1, and the downstream reply bytes (the byte-equivalence prong, §8). (Live-confirmed roster on the success arm.)
- **Route MISS (local `UnknownMethod` exception):** `route_missing` +1, `response_exception` +1, the local exception bytes (Appendix A), `cluster.<name>.upstream_cx_total`/`upstream_rq_total` stay **0** (no dial). `request*` do NOT move (live-confirmed — the miss is accounted only via `route_missing`+`response_exception`). The reference ALSO moves `cx_destroy_local_with_active_rq`+`downstream_response_drain_close`; the subject keeps them at 0 (per-side, §7.6).

### 7.4 Prometheus exposition — the `thrift.` SINGLE-label-HOIST arm (AMEND-T3)

Reference Envoy `contrib-v1.37.2` `/stats/prometheus` (probed live): thrift stats emit as **`envoy_thrift_<leaf>{envoy_thrift_prefix="<stat_prefix>"} <v>`** — the metric name is flat `envoy_thrift_<leaf>`, the `stat_prefix` is hoisted into the `envoy_thrift_prefix` label (quoted live: `envoy_thrift_request{envoy_thrift_prefix="thriftprobe"} 1`; `envoy_thrift_response_reply{envoy_thrift_prefix="thriftprobe"} 1`). This is the **redis-32 `redis.` / mongo `.rbac.` TAG-EXTRACTOR shape (ADR-0218/AMEND-R4)** generalized to a `thrift.` root — NOT the kafka INLINE shape. The `internal/stats/name.go` `thrift.` arm (mirror the `redis.` arm at `name.go:315`): detect `thrift.<prefix>.<rest>` (leading literal `thrift.` + a dot-free `<prefix>` segment) → metric name `envoy_thrift_` + `<rest>` flattened (dot→underscore) + label `envoy_thrift_prefix="<prefix>"`. The roster is fixed (no dynamic command names), so shape-based detection over a dot-free prefix segment is unambiguous. KEEP-IN-SYNC with `internal/filter/network/thriftproxy/stats.go`. The exact form is pinned by the §11.3 live probe.

### 7.5 Project stat-count delta

1091 → **1116** (+25; all 24 counters + 1 gauge eager-created at config parse). The `request_time_ms` histogram is NOT counted (deferred, ADR-0060). The cluster `upstream_*` traffic stats live under the existing `cluster.<name>.*` scope (reused via the seam — they are not a thrift roster, the redis AMEND-R6 posture).

### 7.6 envoy-go-strict departure flags (BEHAVIOR_CONTRACT at IMPL)

- The `request_time_ms` HISTOGRAM unmirrored (ADR-0060).
- The 2 close-direction counters `cx_destroy_local/remote_with_active_rq` created-but-never-incremented + `downstream_response_drain_close` never-incremented (the framework-gap coverage boundary, AMEND-T6/D-T8); on the `0057` miss arm the reference moves `cx_destroy_local_with_active_rq`+`downstream_response_drain_close` → asserted PER-SIDE (subject==0; NOT cross-equal), the redis D-P32-9 / mongo D-P4 close-direction precedent.
- The deferred-active-feature stats (`downstream_cx_max_requests` / `shadow_request_submit_failure` / `upstream_rq_maintenance_mode` / `request_internal_error` exist-at-0; the un-chosen transport×protocol pairs; the non-router thrift_filters; full route_config richness; full struct parsing).
- The un-chosen transport/protocol DEPARTURE reject (the reference parse-accepts; envoy-go fail-fasts — AMEND-T7).
- `payload_passthrough: false` (full struct parse) deferred → message-begin-decode-forward without passthrough counters (coverage boundary).
- The UPSTREAM `seq_id` per-side difference (reference remaps to 0 / subject passes through — AMEND-T5); not asserted (only downstream bytes are).
- The `upstream_cx_*` pooling per-side asymmetry (D-T9b) where the reference's pool diverges (the redis D-P32-9 precedent); for the MVP's sequential one-call-per-connection traffic, `upstream_cx_total`/`upstream_rq_total` are cross-equal (1 each, live-confirmed) — per-side pinning applies only if concurrent downstream conns are exercised (not in the MVP).
- Runtime-key gating unmirrored (no runtime layer → key defaults).

---

## 8. Differential fixture taxonomy (+2)

Full cross-side against the contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2`. Per `reference_differential_fixture_dispatch_constraint`: cross-side and boot-reject fixtures are SEPARATE directories. Per `reference_differential_asserter_dispatch`: every subject-side stat assertion uses `fixture.StatsAsserter` and MUST be proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). The load-bearing proof is TWO-pronged (the §9 SECOND row whose downstream RESPONSE bytes are load-bearing — thrift_proxy GENERATES them by proxying): downstream-Thrift-response byte-equivalence PLUS cross-side `StatsAsserter`. Numbering continues from `0056` (master-tip tail `0056-redis-boot-reject`); re-pinned at IMPL Task 1.

**Fixture-design caveats from the §11.2 live probe:** (i) the fixture backend MUST be a real listening TCP responder echoing a well-formed framed-binary REPLY (the new `TCPThriftResponder`, §8.3); if the cluster can't establish upstream the reference local-replies `no_healthy_upstream`/`unknown_cluster` instead of round-tripping — verify the round-trip ran via `cluster.<name>.upstream_cx_rx_bytes_total > 0` AND `request_call > 0` (thrift_proxy does NOT emit a listener `downstream_cx_rx_bytes_total` — that is an HCM stat; use `request*`/the cluster bytes as the decode-ran witness); (ii) all stat assertions are post-first-connection (the eager-roster boot-window difference); (iii) drive ONE Thrift CALL per connection (or carefully sequence frames) — the framed single-flight model is the MVP contract; (iv) the canned backend echoes the RECEIVED `seq_id` (it is seq_id-agnostic — the reference sends 0, the subject passes the original through; the DOWNSTREAM reply carries the original on both sides, AMEND-T5).

### 8.1 `0057-thrift-roundtrip` (cross-side; multi-arm)

Chain `[thrift_proxy {stat_prefix, transport: FRAMED, protocol: BINARY, payload_passthrough: true, route_config: method "Ping" → cluster}]` as the TERMINAL on BOTH sides (NO `tcp_proxy` — thrift_proxy terminates); the fixture backend is the new `TCPThriftResponder` (§8.3); the driver sends hand-crafted framed-binary Thrift CALL frames (Appendix A). Arms (the PLAN finalizes the exact roster):

1. **route-HIT round-trip arm** — a framed-binary CALL for method `Ping` (seq_id 1) → matches the route → round-trips to the backend → the backend echoes a framed-binary REPLY (empty result struct = void success) → the proxy forwards it downstream. Asserts cross-side: downstream reply bytes byte-identical; `thrift.<sp>.request`/`request_call`/`request_passthrough`/`response`/`response_reply`/`response_success`/`response_passthrough` +1; `cluster.<name>.upstream_cx_total`/`upstream_rq_total` +1 (cross-equal — D-T9b).
2. **route-MISS local-exception arm** — a CALL for a method NOT matching the route (e.g. `Pong`) → local `UnknownMethod` EXCEPTION (Appendix A), NO backend dial. Asserts cross-side: downstream EXCEPTION bytes byte-identical; `route_missing`/`response_exception` +1; `cluster.<name>.upstream_cx_total`/`upstream_rq_total` stay 0. **Per-side coverage boundary (NOT cross-asserted):** `cx_destroy_local_with_active_rq`/`downstream_response_drain_close` (reference moves them; subject==0 — AMEND-T6).
3. *(optional, PLAN-decided)* **reply-EXCEPTION arm** — the backend replies an EXCEPTION msgtype → `response_exception` +1 (distinguishes a backend exception from a local miss). **reply-ERROR arm** — the backend replies a REPLY with field-id ≥ 1 → `response_error` +1.
4. **deliberate-break liveness proof** — recorded in driver comments + README per the `0030` lesson, run with `-count=1` (prove each asserted counter LIVE; the cross-side `StatsAsserter` + downstream byte-equivalence are the load-bearing proofs).

(Per the cross-side-XOR-boot-reject fixture-dispatch constraint, all cross-side arms share this ONE dir.)

### 8.2 `0058-thrift-boot-reject` (boot-reject; separate dir)

Missing `stat_prefix` → both sides reject at boot (the §6.2 `thrift-proxy-stat-prefix-required` arm; boot-stderr-substring parity per §6.2). The route/route-action/thrift-filter-name PGV arms + the un-chosen transport/protocol DEPARTURE arms (§6.2) are UNIT-TESTED (the un-chosen-pair arms CANNOT be a cross-side boot-reject — the reference boots fine, AMEND-T7); the load-bearing required-field arm is the missing-`stat_prefix` one (the redis `0056`/kafka `0054` precedent).

### 8.3 New BackendKind (anticipated value 33)

A NEW BackendKind — a synthesized **`TCPThriftResponder`**: per connection it loops reading a framed-binary CALL (4-byte BE frame-len + the message-begin), parses the method name + msgtype + `seq_id`, and writes back a framed-binary REPLY (msgtype 2) echoing the SAME method name + RECEIVED `seq_id` with an empty result struct (a STOP byte = void success). It is seq_id-AGNOSTIC (echoes whatever it receives — AMEND-T5). An optional second mode replies an EXCEPTION (msgtype 3) or an error REPLY (field-id ≥ 1) for the §8.1 arm 3 arms. Contrast the existing silent `TCPSink`(28) / the `TCPMongoResponder`(30) / the `TCPKafkaResponder`(31, correlation-id-echoing) / the `TCPRedisResponder`(32, FIFO/positional, no correlation id). (BackendKind values are a GLOBAL monotonic counter in `test/differential/fixture/fixture.go`, DECOUPLED from phase numbers — thrift is phase 33 and the next-free BackendKind is 33 only coincidentally.) The exact canned-reply table is pinned at IMPL; the §11.2 hand-built raw-socket responder is the template. BackendKind tail 32 → 33.

### 8.4 Total fixture-dir count + conformance

58 → **60** (+2: `0057` cross-side, `0058` boot-reject). No new conformance harness (matches 26/27/28/29/31/32). The h2spec 53/53 + proxy-wasm 10/10 gates re-run asserted-unaffected at the six-gate (image-independent; phase 33 touches no HTTP/h2/proxy-wasm path).

---

## 9. Behavior-contract delta (the 33 bundle; ADR-0052 atomic landing)

At IMPL final task, `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains:

- A NEW `### envoy.filters.network.thrift_proxy` subsection — proto `…thrift_proxy.v3.ThriftProxy`; the single-pair single-route terminal envelope (framed-transport 4-byte-length frame + binary-protocol message-begin [method name + message type + `seq_id`] + payload_passthrough opaque-body-forward); the `route_config` method-routing (exact-match/match-all → one cluster); the REUSED ADR-0230 upstream round-trip (one-conn-per-downstream, single-flight, positional, seq_id pass-through); the local `UnknownMethod` exception on a routing miss; the EAGER 25-name roster under `thrift.<stat_prefix>.`; the 11th built-in; the `thrift.` SINGLE-label-hoist Prometheus arm.
- The stat table 1091 → 1116 (+25).
- Coverage-boundary / departure records: the `request_time_ms` histogram unmirrored (ADR-0060); the 2 close-direction counters + `downstream_response_drain_close` created-but-never-incremented (the framework-gap boundary, asserted per-side on the `0057` miss arm); the un-chosen transport×protocol pairs (DEPARTURE reject); the non-router thrift_filters / full route_config richness / full struct parsing parse-accepted-behavior-deferred; the upstream `seq_id` per-side difference; the `upstream_cx_*` per-side pooling pin; runtime-keys-at-defaults.

---

## 10. Per-task structure (~16–20 tasks; PLAN decomposes)

Indicative spine for the PLAN (TDD per task; per-task `gofmt -l` + `golangci-lint` on touched pkgs per `feedback_pertask_gofmt_lint`; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`):

| # | Task | SPEC anchor |
|---|---|---|
| 1 | First-task baselines/anchors gate: re-confirm fuzzers **41** + fixtures **58** (tail `0056`) + stat surface **1091** + DECISIONS.md tail **ADR-0230** (this SPEC drafts 0231) + BackendKind tail **32** via the canonical recipes; re-confirm `proto.MessageName(&thrift_proxyv3.ThriftProxy{})` + that `/envoy v1.32.4` already carries thrift_proxy/v3 (ZERO new dep); re-pin the as-built anchors (`upstreampool.go` seam API, `builtins.go` registration site, `bootstrap.go` blank-import block, `name.go:315` redis arm, `redisproxy/{filter,stats}.go` shapes) against the IMPL-session tip | §11 / §3 |
| 2 | The `thriftproxy` package skeleton + `TypeURL` (TDD: a `proto.MessageName` pinning test) + the `thrift_proxy/v3` blank-import in `bootstrap.go` + `go mod tidy` adds nothing | §3.4/§5.1 |
| 3 | Config parse: `stat_prefix` (required → boot-reject) + `transport`/`protocol` (accept `{AUTO,FRAMED}×{AUTO,BINARY}`; else the departure reject) + `payload_passthrough` + the parse-accept-deferred fields + `stats.IsValidName(stat_prefix)` guard (TDD: all reject arms byte-stable) | §5/§6 |
| 4 | The `route_config` method-routing table parse + match (exact `method_name` / empty=match-all → cluster) + the route PGV arms (TDD: match-all, exact-match, miss, malformed-route rejects) | §5.4/§3.2 |
| 5 | The framed-transport frame decode (4-byte BE length + bounds) + binary-protocol message-begin decode (magic/msgtype/name/seq_id) (TDD per Appendix A + partial-frame reassembly + malformed-length/bad-magic throw) | Appendix A |
| 6 | The reply-frame classifier (msgtype → reply/exception/invalid_type; REPLY first-field peek → success/error) + the opaque struct-skip-for-passthrough (forward raw body) + the local `AppException` (`UnknownMethod`) encoder (TDD per Appendix A) | Appendix A/§3.2 |
| 7 | The EAGER 25-name roster (`stats.go`) + the inc accessors + a `TestStatRoster` (TDD: roster present-at-0; the gauge; the exist-at-0 members; the 2 never-incremented close-direction counters) | §7.2 |
| 8 | The `TerminalFilter.Handle` request→reply pump (the redis `serveRequest` shape): decode → count request → match route → MISS local exception (keep conn open) / HIT seam round-trip → count response → forward reply; `request_active` gauge inc/dec balanced (TDD: hit, miss, decoding-error, invalid-type) | §3.3/§7.3 |
| 9 | Registration as the 11th built-in (`builtins.RegisterBuiltins` + main.go parity) + boot smoke (TDD) | §3.4 |
| 10 | The `thrift.` SINGLE-label-hoist Prometheus arm in `internal/stats/name.go` (TDD: `thrift.thriftprobe.request` → `envoy_thrift_request{envoy_thrift_prefix="thriftprobe"}`) | §7.4 |
| 11 | The new `TCPThriftResponder` BackendKind (framed-binary canned REPLY echoing method+seq_id; seq_id-agnostic) | §8.3 |
| 12 | The `0057-thrift-roundtrip` cross-side fixture (route-HIT round-trip + route-MISS local-exception arms; downstream byte-equivalence + `StatsAsserter`; the per-side cx_destroy boundary on the miss arm) + driver | §8.1 |
| 13 | The `0058-thrift-boot-reject` fixture (missing `stat_prefix`) + the unit-tested route/departure reject arms | §8.2/§6.2 |
| 14 | The 42nd fuzzer `FuzzThriftDecode` (no-panic + no-mutation + bounded-allocation over the framed-binary message-begin decoder + the reply classifier) | §14 |
| 15 | Full differential re-verify (the 58 prior dirs byte-exact back-compat + the 2 new dirs green) + the deliberate-break liveness proofs (`-count=1`) | §8 |
| 16 | Completion bundle: BEHAVIOR_CONTRACT 33 subsection (1091 → 1116) + ADR-0231 §Decision/§Consequences body (ADR-0044 in-place) + STATE/ROADMAP row 33 `in-progress → done` (flat §9 row — NO parent rollup; the §9 family CLOSES) + the six-gate evidence | §9 / §15 |

The PLAN re-checks the ADR-0045 gate; if it trips, consume the pre-authorized 33.1/33.2 split (§3.0).

---

## 11. SPEC-time empirical-pin block (D-T1..D-T9 — executed IN-SESSION 2026-06-09)

Parallel-subagent-fan-out scrape executed this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-09.** **Reference source corpus:**

1. **The live `envoyproxy/envoy:contrib-v1.37.2` docker image** (id `7edd5b0f…`, present locally): a real boot of a `[thrift_proxy {stat_prefix: thriftprobe, transport: FRAMED, protocol: BINARY, payload_passthrough: true, route_config: match-all → thriftcluster}]` TERMINAL listener on a docker BRIDGE network (`reference_docker_probe_bridge_network`) with a STRICT_DNS backend (`thriftbackend:9090` — a hand-built raw-socket canned-REPLY Thrift responder); admin `/stats` + `/stats/prometheus` scrapes pre- and post-connection; a hand-crafted framed-binary CALL driven through the listener (round-trip CONFIRMED ran: `cluster.thriftcluster.upstream_cx_rx_bytes_total: 21`, `thrift.thriftprobe.request_call: 1`, `response_success: 1`); a routing-miss config probe (local `UnknownMethod` exception confirmed, no backend dial); `--mode validate` boot-reject probe.
2. **go-control-plane `/envoy` v1.32.4 bindings** at `~/go/pkg/mod/.../envoy@v1.32.4/extensions/filters/network/thrift_proxy/v3/`: `thrift_proxy.pb.go` + `route.pb.go` + the `.pb.validate.go` files; `proto.MessageName` run in a throwaway `/tmp/dt1probe` module + `go mod tidy` zero-new-dep check.
3. **Upstream Envoy v1.37.2 CORE source** via raw.githubusercontent.com at tag v1.37.2: `source/extensions/filters/network/thrift_proxy/` — `stats.h`, `decoder.cc`, `conn_manager.cc`, `framed_transport_impl.cc`, `binary_protocol_impl.{h,cc}`, `auto_transport_impl.cc`, `auto_protocol_impl.cc`, `router/router_impl.cc`, `router/config.h`, `config.cc`, `thrift.h`; the `api/.../v3/thrift_proxy.proto` + `route.proto`.
4. **envoy-go codebase** at master tip `337d3c2`: `internal/filter/network/{upstreampool.go,redisproxy/,builtins/}`; `internal/stats/name.go`; `internal/bootstrap/bootstrap.go`; `test/differential/fixture/fixture.go`; `go.mod`.

### Summary disposition table (9 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D-T1 (SPEC-BLOCKING) — TypeURL + CORE `/envoy` dep | **CONFIRMED** (@type carries `extensions.`; thrift_proxy/v3 is CORE `/envoy v1.32.4`; ZERO new dep) | T1 |
| §11.2 | D-T2 (SPEC-BLOCKING) — framed×binary wire + the pair | **CONFIRMED LIVE** (the round-trip ran; framed 4-byte-len + binary strict message-begin; payload_passthrough message-begin-only; AUTO auto-detects identically) | T2 |
| §11.3 | D-T3 (SPEC-BLOCKING) — stat roster + prom form | **REFINES + REFUTES** (24 counters + 1 gauge + 1 histogram; no `response_business_exception`; +passthrough/router stats; SINGLE-label-HOIST prom arm — the redis shape) | T3 |
| §11.4 | D-T5 (SPEC-BLOCKING) — router / local-reply | **CONFIRMED** (routing miss → local `UnknownMethod` exception, zero upstream; decode error → local `ProtocolError`; hit → proxy) | T4 |
| §11.5 | D-T6 — seq_id echo + correlation | RESOLVES (reference remaps upstream seq_id→0 + restores downstream; positional correlation; MVP passes through → downstream byte-equivalent) | T5 |
| §11.6 | D-T4 — config PGV arms + boot-reject | RESOLVES (only `stat_prefix` PGV-required; `route_config` NOT required; transport/protocol `defined_only`; the un-chosen pairs are envoy-go-strict departures) | T7 |
| §11.7 | D-T7 — `IsValidName` placement | RESOLVES (fixed roster → satisfied by construction; config-boundary guard for `stat_prefix` only) | — |
| §11.8 | D-T8 — close-direction counters | RESOLVES (they EXIST + fire on the live miss path; created-but-never-incremented; per-side on the miss arm) | T6 |
| §11.9 | D-T9 — ADR-0045 envelope + per-side pooling | RESOLVES (~790–1275 LoC / 16–20 tasks → single row; `upstream_cx_total`/`upstream_rq_total` cross-equal=1 for sequential traffic) | — |

### 11.1 D-T1 (SPEC-BLOCKING) — TypeURL + the CORE `/envoy` dep: CONFIRMED

`proto.MessageName(&thrift_proxyv3.ThriftProxy{})` (run in a throwaway `/tmp/dt1probe` module, `require github.com/envoyproxy/go-control-plane/envoy v1.32.4`) = `envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy` → `@type` = `type.googleapis.com/envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy` (carries the `extensions.` segment — `reference_network_filter_typeurl_extensions` holds). Generated files in `.../envoy@v1.32.4/extensions/filters/network/thrift_proxy/v3/`: `thrift_proxy.pb.go` (package `thrift_proxyv3`), `route.pb.go`, the `.pb.validate.go` + `_vtproto.pb.go` variants. The package declares 4 messages (`Trds`, `ThriftProxy`, `ThriftFilter`, `ThriftProtocolOptions`); NO `*PerRoute`. `go list -deps` confirms the full transitive closure (`config/core/v3`, `config/route/v3`, `config/accesslog/v3`, `cncf/xds/go`, `protoc-gen-validate`, `wrapperspb`, `anypb`, the genproto api/rpc) resolves entirely within modules ALREADY in envoy-go's go.mod (`/envoy v1.32.4` direct; `cncf/xds`, `protoc-gen-validate`, genproto already present) → **ZERO new go.mod module requirement**; `go mod tidy` adds nothing with the first consumer (the config parse + the `bootstrap.go` blank-import). The CORRECTED dep framing (thrift is CORE `/envoy`, NOT `/contrib`) holds — exactly the redis D32-1 posture.

### 11.2 D-T2 (SPEC-BLOCKING) — framed×binary wire + the working config: CONFIRMED LIVE

The reference `[thrift_proxy]` TERMINAL listener BOOTED clean on the FIRST try. The exact working filter block (reusable verbatim at the IMPL fixture):

```yaml
- name: envoy.filters.network.thrift_proxy
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy
    stat_prefix: thriftprobe
    transport: FRAMED
    protocol: BINARY
    payload_passthrough: true
    route_config:
      name: thrift_routes
      routes:
      - match: { method_name: "" }   # "" == match-all
        route: { cluster: thriftcluster }
```
(Cluster `thriftcluster`: `type: STRICT_DNS`, `lb_policy: ROUND_ROBIN`, endpoint `thriftbackend:9090`. No explicit `thrift_filters` — the router is implicit, §5.3.) The driver sent a framed-binary CALL (`method=ping, seq_id=1`, framelen 21); the proxy round-tripped it to the canned backend and returned a framed-binary REPLY (`msgtype 2`, method `ping`, `seq_id 1`, empty struct) — reply hex `00000011800100020000000470696e670000000100`. The wire format (framed 4-byte BE frame-len; binary strict message-begin `0x8001 00 <msgtype>` + i32 name-len + name + i32 seq_id, `MinMessageBeginLength=12`; msgtype `Call=1`/`Reply=2`/`Exception=3`/`Oneway=4`; struct = field-type-keyed, STOP=0x00 terminates; under `payload_passthrough` the decoder reads only the message-begin then consumes the body opaquely) is transcribed in Appendix A. AUTO transport/protocol auto-detects a framed-binary frame identically (source `auto_transport_impl.cc`/`auto_protocol_impl.cc`).

### 11.3 D-T3 (SPEC-BLOCKING) — the stat roster + the SINGLE-label-hoist prom arm

**Source roster** (`stats.h`, `ALL_THRIFT_FILTER_STATS`): 19 COUNTERs (`cx_destroy_local_with_active_rq`, `cx_destroy_remote_with_active_rq`, `downstream_cx_max_requests`, `downstream_response_drain_close`, `request`, `request_call`, `request_decoding_error`, `request_invalid_type`, `request_oneway`, `request_passthrough`, `request_internal_error`, `response`, `response_decoding_error`, `response_error`, `response_exception`, `response_invalid_type`, `response_passthrough`, `response_reply`, `response_success`) + 1 GAUGE (`request_active`, Accumulate) + 1 HISTOGRAM (`request_time_ms`, Milliseconds). **NO `response_business_exception`** (REFUTES the BRAINSTORM hypothesis). **Live** `/stats | grep thrift` adds 5 ROUTER counters under the same scope: `route_missing`, `unknown_cluster`, `no_healthy_upstream`, `upstream_rq_maintenance_mode`, `shadow_request_submit_failure`. Total present-at-0 from boot: **24 counters + 1 gauge + 1 histogram = 26 names**. On the SUCCESS round-trip the live counters that moved: `request`, `request_call`, `request_passthrough`, `response`, `response_reply`, `response_success`, `response_passthrough` (each +1) + `request_time_ms` recorded + `request_active` returned to 0.

**Prometheus arm (live `/stats/prometheus | grep thrift`):** **SINGLE-label-HOIST** — `envoy_thrift_<leaf>{envoy_thrift_prefix="thriftprobe"}`. Verbatim lines:
```
envoy_thrift_request{envoy_thrift_prefix="thriftprobe"} 1
envoy_thrift_request_call{envoy_thrift_prefix="thriftprobe"} 1
envoy_thrift_request_passthrough{envoy_thrift_prefix="thriftprobe"} 1
envoy_thrift_response{envoy_thrift_prefix="thriftprobe"} 1
envoy_thrift_response_reply{envoy_thrift_prefix="thriftprobe"} 1
envoy_thrift_response_success{envoy_thrift_prefix="thriftprobe"} 1
envoy_thrift_request_time_ms_count{envoy_thrift_prefix="thriftprobe"} 1
envoy_thrift_request_time_ms_sum{envoy_thrift_prefix="thriftprobe"} 1.05...
```
This is the redis-32 / mongo `.rbac.` TAG-EXTRACTOR shape (one hoisted `envoy_thrift_prefix` label), NOT the kafka INLINE shape (REFUTES the BRAINSTORM "INLINE/MULTI/SINGLE TBD"). The histogram emits the full `_bucket{…,le}`/`_sum`/`_count` triplet (deferred per ADR-0060 — envoy-go does not mirror it).

### 11.4 D-T5 (SPEC-BLOCKING) — router / local-reply semantics: CONFIRMED

**Routing MISS** (live + source `router/router_impl.cc`): a config matching only `method_name: "onlythis"`, driven with `somethingelse`, returned a downstream EXCEPTION (`msgtype 3`, `AppExceptionType::UnknownMethod`=1, message `no route for method 'somethingelse'`) — exact reply hex in Appendix A — with `cluster.thriftcluster.upstream_cx_total`/`upstream_rq_total` staying **0** (NO backend dial). Source: a null route → `sendLocalReply(AppException(UnknownMethod, fmt::format("no route for method '{}'", methodName)))`. Live stats moved: `route_missing: 1`, `response_exception: 1`, `downstream_response_drain_close: 1`, `cx_destroy_local_with_active_rq: 1`; `request_call` stayed 0 (the miss is NOT counted as a serviced call). **Decoding ERROR** (source `conn_manager.cc`): an `EnvoyException` after a usable message-begin → `sendLocalReply(AppException(ProtocolError=7, what))` + `FlushWrite` close; before any metadata → raw connection close, no Thrift reply; `request_decoding_error` increments. **Route HIT** → dials the matched cluster, round-trips, proxies the reply. The local reply is encoded with the SAME transport+protocol as the request.

### 11.5 D-T6 — seq_id echo + positional correlation

Live: the reference REWROTE the upstream `seq_id` to **0** (the canned backend received `seq_id=0`) and mapped it back to the ORIGINAL (1) on the downstream reply. Source: `router_impl.cc` does NO seq_id rewrite (grep `seq`/`SequenceId` → none); the remap is the conn-manager/upstream-request layer's per-connection transaction-id mapping. Correlation is POSITIONAL — the router holds a single `upstream_request_` (one in-flight RPC per upstream conn), matches the reply by the `UpstreamRequest` lifecycle, then `cleanup()`. So `seq_id` is NOT load-bearing for correlation. envoy-go's one-conn-per-downstream synchronous single-flight MVP passes `seq_id` through unchanged (single-flight makes positional correlation trivially correct) → the downstream reply carries the ORIGINAL `seq_id` on BOTH sides → downstream byte-equivalence holds. The UPSTREAM `seq_id` differs per-side (reference 0 / subject passthrough) but is never asserted (only downstream bytes are; the canned backend echoes whatever it receives). The §2.4 positional-correlation decision is VALIDATED.

### 11.6 D-T4 — config PGV arms + boot-reject

Live `--mode validate`: **missing `stat_prefix`** → REJECTED, `Proto constraint validation failed (ThriftProxyValidationError.StatPrefix: value length must be at least 1 characters)`. **Missing `route_config`** → validates OK (NOT required; absent route table → all requests routing-miss → local exception). The Go PGV binding emits `value length must be at least 1 runes` (the C++/Go idiom difference). Proto `transport`/`protocol` are PGV `defined_only` only (`value must be one of the defined enum values` for an out-of-range value) — any DEFINED enum value (incl. `UNFRAMED`/`HEADER`/`COMPACT`/`LAX_BINARY`/`TWITTER`) parse-ACCEPTS and decodes at runtime. The MVP's un-chosen-pair reject is therefore an envoy-go-strict DEPARTURE (the reference boots fine) — unit-tested, NOT a cross-side boot-reject (§6.2). Route-level PGV: `Route.match`/`Route.route` required; `RouteMatch` specifier oneof required; `RouteAction` cluster_specifier required + `cluster` min_len 1; `ThriftFilter.name` min_len 1 (§5.4).

### 11.7 D-T7 — `IsValidName` placement: satisfied BY CONSTRUCTION

The thrift roster is FIXED + table-bounded (no per-method/per-route stat segment is wire-derived — the method name drives ROUTING, not a counter, in the MVP). So NO arbitrary wire-derived dynamic segment ever reaches `NewCounterIfAbsent` → the charset guard (`reference_dynamic_stat_name_charset_guard`) is trivially satisfied (the redis D-P32-7 static-table posture). The only config-derived segment (`stat_prefix`) is validated at parse via `stats.IsValidName` at the config boundary (the redis/mongo/kafka precedent). The `FuzzThriftDecode` no-panic fuzzer still asserts the framed-binary decoder never panics + never over-allocates on a malformed frame (the framed length prefix is bounds-checked).

### 11.8 D-T8 — close-direction counters: EXIST + fire on the miss path; created-but-never-incremented

Source (`stats.h` + `conn_manager.cc onEvent`→`resetAllRpcs`): `cx_destroy_local_with_active_rq` / `cx_destroy_remote_with_active_rq` are incremented on a `LocalClose`/`RemoteClose` event respectively WHILE an RPC is active (`!rpcs_.empty()`). Live: on the SUCCESS round-trip both stayed 0 (clean close, no active rq); on the routing-MISS local-reply path `cx_destroy_local_with_active_rq` went to **1** (the reference `FlushWrite`-closes after the local exception with the rq active) + `downstream_response_drain_close: 1`. This is SHARPER than redis-32 (whose roster has NO close-direction counter and never exercised one). The network framework records close TYPE not DIRECTION (`reference_close_direction_framework_gap`); keying close counters by direction is a framework-SURGERY sub-phase (deferred project-wide — the §9 zero-touch AMEND-R9 posture). DISPOSITION: the MVP EAGER-creates both counters (present-at-0 roster parity) but NEVER increments them, and its local-reply path KEEPS the downstream connection OPEN (no `FlushWrite`-close → no drain). On the `0057` miss arm the reference moves `cx_destroy_local_with_active_rq`+`downstream_response_drain_close` while the subject stays 0 → these are asserted PER-SIDE (subject==0; NOT cross-equal), a documented coverage boundary (the redis D-P32-9 / mongo D-P4 close-direction precedent).

### 11.9 D-T9 — ADR-0045 envelope + per-side pooling

**(a) Split-gate.** ~790–1275 production LoC / ~16–20 tasks (§3.0) — BOTH the LoC leg (≤ ~1500) and the task leg (≤ ~25) fit UNDER the gate. SINGLE FLAT ROW 33 holds (the kafka-31 precedent); the pre-authorized 33.1/33.2 escape-valve STAYS UNCONSUMED. The PLAN re-checks. **(b) Per-side pooling.** Live success round-trip: `cluster.thriftcluster.upstream_cx_total: 1`, `upstream_rq_total: 1`, `upstream_cx_rx_bytes_total: 21`, `upstream_cx_tx_bytes_total: 21` (one downstream call → one upstream cx + one upstream rq; the framed CALL/REPLY are both 21 bytes). envoy-go's one-conn-per-downstream MVP produces the SAME (1 cx, 1 rq) for the sequential one-call-per-connection fixture traffic → `upstream_cx_total`/`upstream_rq_total` are cross-EQUAL (contrast redis D-P32-9, where MULTIPLE downstream conns to one backend diverged ref=1/subj=N). Per-side pinning applies only if concurrent downstream conns are exercised — NOT in the MVP.

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S33-1** — the exact file split of `thriftproxy/` (the PLAN finalizes `thrift.go` vs a separate `route.go`/`exception.go`; anticipated per §3.2).
- **D-S33-2** — whether the `0057` optional reply-EXCEPTION / reply-ERROR arms (§8.1 arm 3) land in the IMPL or stay unit-tested (anticipated: at least the reply-EXCEPTION arm earns a fixture arm to exercise `response_exception` from a backend reply, distinct from the local-miss exception).
- **D-S33-3** — the `request_active` gauge differential treatment (LIVE-mirrored via a held-open arm, or quiesced-to-0 asserted post-workload — the redis `downstream_rq_active` precedent; anticipated quiesced-0 + an optional held-open arm).
- **D-S33-4** — the reply-frame success/error classification under `payload_passthrough` (peek the first result-struct field header for field-id 0 vs ≥1 — §3.2; confirm against the reference's passthrough behavior at IMPL, the canned backend's void-success reply pins `response_success`).
- **D-S33-5** — the exact `FuzzThriftDecode` corpus seeds (a valid framed-binary CALL + a truncated frame + a bad-magic frame + an oversized length prefix).
- **D-S33-6** — whether the malformed-frame path emits a local `ProtocolError` exception (the reference does, post-metadata) or silently closes in the MVP (anticipated: count `request_decoding_error`; the local `ProtocolError` exception is an optional refinement — the byte-equivalence on a malformed-frame arm is a PLAN call).
- ADR-0045 split-gate FINAL re-check at PLAN.

---

## 13. ADR continuity

This SPEC anchors the **ADR-0231 §Context** (the thrift_proxy filter) into DECISIONS.md (tail ADR-0230 → ADR-0231; next-free → ADR-0232). ONE ADR — NO parent-umbrella ADR (single flat row), NO seam ADR (the ADR-0230 seam REUSED unchanged) — contrast redis-32's TWO ADRs (ADR-0229 filter + ADR-0230 seam). The §Decision/§Consequences body lands at the phase-33 IMPL per ADR-0044. The ADR-0209 escape-valve reserve STANDS-UNCONSUMED.

---

## 14. The 42nd fuzzer — `FuzzThriftDecode`

`FuzzThriftDecode` (in `internal/filter/network/thriftproxy/fuzz_test.go`) — no-panic / no-mutation / bounded-allocation over the framed-binary message-begin decoder + the reply-frame classifier. Asserts: an arbitrary byte slice never panics the decoder; the 4-byte framed length prefix is bounds-checked (no over-allocation on a huge declared length); a truncated/bad-magic/invalid-msgtype frame returns a clean error (not a panic); the decoder never mutates its input buffer. Canonical recipe count 41 → **42** (`grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`).

---

## 15. Exit — counts + ROADMAP/STATE at SPEC-DONE

At the SPEC-DONE commit (counts UNCHANGED at the SPEC; they advance at the IMPL):

- stat surface **1091** (→ **1116** at the IMPL, +25).
- differential fixtures **58** (→ **60** at the IMPL: `0057`/`0058`).
- fuzzers **41** (→ **42** at the IMPL: `FuzzThriftDecode`).
- BackendKind tail **32** (→ **33** at the IMPL: `TCPThriftResponder`).
- DECISIONS.md tail **ADR-0230**; this SPEC anchors the **ADR-0231 §Context** (next-free → ADR-0232).
- ROADMAP row 33 STAYS `in-progress` (it flips `→ done` at the phase-33 IMPL six-gate — a flat §9 row, NO parent rollup); the §9 candidate-roster paragraph stays (thrift the sole open candidate; the family CLOSES after phase 33 phase-done).
- spec-document-reviewer gate applies at this SPEC.
- Next → the **phase-33 PLAN** (`superpowers:writing-plans` — decompose §10 into bite-sized TDD tasks; re-check the ADR-0045 gate).

---

## Appendix A — the framed×binary Thrift wire format (transcribed from the live probe + upstream source)

**FRAMED transport:** a 4-byte big-endian (network-order) frame-length prefix (signed int32, `> 0` and `<= maxFrameSize`), then exactly that many payload bytes.

**BINARY protocol strict message-begin** (`MinMessageBeginLength = 12`):
- bytes 0–1: magic `0x8001` (version 1, MSB set; `version != 0x8001` → throw `invalid binary protocol version`).
- byte 2: zero (unused).
- byte 3: **message type** — raw byte, range-checked (`Call=1`, `Reply=2`, `Exception=3`, `Oneway=4`; out-of-range → `request_invalid_type`/`response_invalid_type`).
- bytes 4–7: i32 big-endian method-name length `N`.
- bytes 8..8+N: the method-name ascii.
- next 4 bytes: i32 big-endian `seq_id`.
- then the struct payload. Under `payload_passthrough` the decoder reads NO further struct fields — it consumes the rest of the frame (`frameSize − bodyStart`) as opaque bytes (`request_passthrough`/`response_passthrough`). A minimal void struct = a single STOP byte `0x00`.

**FieldType enum** (non-passthrough struct-skip, not used by the MVP): `Stop=0, Bool=2, Byte=3, Double=4, I16=6, I32=8, I64=10, String=11, Struct=12, Map=13, Set=14, List=15`.

**A canned framed-binary CALL** for method `ping`, `seq_id 1` (the §11.2 driver):
```
payload = b'\x80\x01\x00\x01' + i32be(4) + b'ping' + i32be(1) + b'\x00'   # CALL, name "ping", seq 1, empty struct
frame   = i32be(len(payload)) + payload                                    # = 00000011 8001000100000004 70696e67 00000001 00
```

**A canned framed-binary REPLY** echoing the received method + `seq_id` (the `TCPThriftResponder`, void success):
```
payload = b'\x80\x01\x00\x02' + i32be(len(method)) + method + i32be(seq_id) + b'\x00'   # REPLY, empty result struct
frame   = i32be(len(payload)) + payload
```

**The local `UnknownMethod` EXCEPTION** (the reference's miss-path reply, live-captured for method `somethingelse`, `seq_id 1`) — an `AppException` TStruct `{1: string message, 2: i32 type}`:
```
80 01 00 03                                 # version + EXCEPTION(3)
00 00 00 0d 73 6f...                        # i32 name-len 13 + "somethingelse"
00 00 00 01                                 # seq_id 1 (echoes the request)
0b 00 01 00 00 00 23 6e 6f 20 72 6f...      # field: type STRING(0x0b) id 1, len 0x23=35, "no route for method 'somethingelse'"
08 00 02 00 00 00 01                        # field: type I32(0x08) id 2, value 1 (UnknownMethod)
00                                          # STOP
```
envoy-go encodes this exact struct (method name + `seq_id` from the missed request) for downstream byte-equivalence on the `0057` miss arm.
