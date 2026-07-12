# Phase 60 Brainstorm — OPEN the xDS / dynamic-config family via **SDS** for a downstream server TLS certificate (State-of-the-World, single dedicated SDS stream, gRPC `api_config_source`, INITIAL-FETCH scoped) — the FIRST dynamic-config path in a 100%-static-bootstrap project; lifts the `tls_certificate_sds_secret_configs` reject (`tls/config.go:153`) — NOT the bootstrap `dynamic_resources` reject; builds the discovery-stream substrate + ACK/NACK handshake + a Secret applier; anticipated a **SPLIT** (60.1 substrate / 60.2 TLS-apply + differential); anticipates **ADR-0278**; +1 fixture (a driver-owned SDS management server), likely +1 fuzzer, +N `sds.*` stats, +≥1 package, +0 modules

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only; NO production `.go` changes, NO Docker, NO tests at this stage. Fresh worktree `.worktrees/phase-60-brainstorm`, branch `phase-60-xds-brainstorm`, per `feedback_git_worktrees`.
>
> **Loop re-open (AUTONOMOUS — no human pick):** the Observability tail's cheap candidates are being drained (phase 59 = tracing `custom_tags` literal, PLAN done). Per the **STANDING DIRECTIVE (human, 2026-07-12)** the loop runs AUTONOMOUSLY until the termination sentinel fires. This BRAINSTORM makes a DELIBERATE ESCALATION from "smallest cheap candidate in an already-open family" to **OPENING A NEW FAMILY** — the **xDS / dynamic-config family** — because xDS is the single largest structural gap in the project (envoy-go is 100% static-bootstrap; the reference is a dynamic-config proxy) and opening it is itself a first-class roadmap objective. Within that new family the pick is STILL the smallest-defensible-slice discipline: **SDS for one downstream server cert, SotW, initial-fetch** — the one xDS slice that opens the discovery-stream substrate while touching the LEAST of the boot model (§2.1, four declined alternatives recorded). No human pause; no `stop` file.
>
> **Baselines re-verified against master tip `958d0154` (the phase-59 PLAN squash; docs-only — production counts are the phase-58 IMPL values):** stat surface **1201** · fixtures **103** (tail `0101-stats-sink-graphite`) · fuzzers **54** · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0276** (**ADR-0277 reserved for phase-59**; this phase anticipates **ADR-0278**) · new Go packages **0** · new go.mod modules **0**. Counts are UNCHANGED at a BRAINSTORM (docs-only). All `file:line` citations below were RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — see §11.

---

## 1. Mission and scope confirmation (60 — the FAMILY-OPENING xDS row; SDS server-cert, SotW, initial-fetch)

### 1.1 What phase 60 delivers as a self-contained whole (a dynamic downstream server cert over gRPC SDS)

envoy-go today is **100% static-bootstrap**. Every listener, cluster, route, and TLS cert is materialized ONCE from `static_resources` at boot (`cluster/manager.go:75-86` walks `GetStaticResources().GetClusters()`; `listener/manager.go:200-255` walks `static_resources.listeners[]`), and the bootstrap parser **explicitly rejects** the dynamic-config entry points:

```go
// internal/bootstrap/bootstrap.go:499-504 (re-derived against master 958d0154)
if _, ok := generic["dynamic_resources"]; ok {
    return nil, fmt.Errorf("bootstrap: dynamic_resources not supported in phase 01 (see SPEC §2)")
}
if _, ok := generic["layered_runtime"]; ok {
    return nil, fmt.Errorf("bootstrap: layered_runtime not supported in phase 01 (see SPEC §2)")
}
```

Phase 60 **opens the xDS / dynamic-config family** by building the FIRST real dynamic-config path: a downstream server TLS **certificate** fetched at boot from a **Secret Discovery Service (SDS)** management server over a gRPC stream, instead of read from inline bytes / a filesystem path. The operator configures a `DownstreamTlsContext` whose `common_tls_context.tls_certificate_sds_secret_configs[0]` names a Secret and a gRPC `ConfigSource.api_config_source`; the proxy dials the named (static) SDS cluster, opens `StreamSecrets`, sends the initial `DiscoveryRequest`, receives a `Secret{tls_certificate{certificate_chain, private_key}}`, ACKs it, and serves TLS with that leaf on the listener.

The delivery is a complete, differentially observable slice: a TLS client connecting to the listener observes the SDS-delivered leaf certificate (serial / SAN), byte-for-byte identically on the reference and the subject (§2.6). This builds — for the first time in the project — the discovery-stream client, the version/nonce ACK/NACK handshake, and one resource-type applier: the three primitives every future xDS row (CDS/LDS/RDS/EDS/RTDS/ADS/Delta) will reuse.

### 1.2 Why SDS is the slice that OPENS the family with the LEAST new boot-model surgery (the killer property)

The decisive reason SDS — not CDS/LDS/RDS/EDS — is the family opener: **an SDS `ConfigSource` lives INLINE in the TLS transport-socket config, NOT in bootstrap `dynamic_resources`.** So opening xDS via SDS does **not** require lifting the `dynamic_resources` reject (`bootstrap.go:499`) or reshaping the whole static-vs-dynamic boot model — that stays for the later LDS/CDS/RDS/EDS rows. Phase 60 lifts a MUCH narrower, already-localized reject:

```go
// internal/tls/config.go:153-159 (re-derived against master 958d0154)
if len(c.GetTlsCertificateSdsSecretConfigs()) > 0 {
    return nil, fmt.Errorf("tls: %s: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03", side)
}
// ... validation_context_sds_secret_config likewise rejected (:158-159)
```

Lifting exactly ONE arm of that reject (the `tls_certificate_sds_secret_configs` case, and only for a gRPC `api_config_source`) is the minimal boot-model change that still yields a genuine, reference-comparable dynamic-config path. The `dynamic_resources`/`layered_runtime` rejects stay UNTOUCHED. This is the property that makes SDS the smallest defensible family-opener (§2.1).

### 1.3 What phase 60 does NOT deliver (forward to §8)

NO `dynamic_resources` (LDS/CDS/RDS/EDS) — the `bootstrap.go:499` reject STAYS. NO **ADS** (single muxed stream) — SDS runs on its own dedicated `StreamSecrets` stream (§2.3). NO **Delta xDS** — State-of-the-World only (§2.4). NO **RTDS** / Runtime layer (`internal/runtime` stays a placeholder; §2.1). NO SDS **rotation / dynamic re-delivery** — INITIAL-FETCH only; the cert is built once after the first response and is thereafter static (§2.5 — this is the property that avoids a mutable-cert seam). NO SDS **`validation_context`** (the CA/trust-bundle arm, `tls/config.go:158`) — that reject STAYS. NO **upstream (client) SDS cert** — downstream server cert only. NO `google_grpc` transport, NO `self`/`ads` ConfigSource — each rejected loudly (§2.7). NO reconnection/backoff hardening beyond a documented MVP (§2.8).

### 1.4 Phase-done as the family-OPENING row (family STAYS OPEN; sentinel implications)

Phase 60 is the FIRST xDS-family row. The ROADMAP §9 stub `### xDS / dynamic config family` today reads only: *"ADS, delta xDS, LDS, CDS, RDS, EDS, SDS, RTDS, reconnection, initial-fetch timeout."* At the phase-60 IMPL the controller expands it into a row + a LIVE deferred-candidate sentence (§2.9). After phase 60 the family STAYS OPEN (every other resource type + ADS + Delta + RTDS remain, §8). **Sentinel:** opening xDS drops the "never-opened families" set (sentinel check-(3)) from **five** `{HTTP/3, gRPC, xDS, Runtime, WASM}` to **four** `{HTTP/3, gRPC, Runtime, WASM}` — still ≥1, so check-(3) STILL prints and the loop continues (`reference_sentinel_deferred_sentence_live_vs_historical`).

### 1.5 ADR-0045 split readiness — anticipated a **SPLIT** (60.1 substrate / 60.2 TLS-apply + differential) *(SPEC confirms)*

This is a genuinely NEW subsystem (a gRPC discovery stream + a stateful ACK/NACK handshake + a Secret applier + TLS plumbing + a listener-warmup/initial-fetch gate + new stats + a driver-owned management-server test fixture + an ADR). The anticipated task count exceeds the ADR-0045 `~15` ceiling, so a SPLIT is anticipated (unlike the phase-58/59 single flat rows). Anticipated cut (SPEC re-decides the exact boundary — D-XDS-SPLIT):

- **60.1 (`xds-sds-stream-substrate`)** — the keystone: the `internal/xds` discovery-stream client (reusing `grpcclient.Dialer`, §3), the SotW `DiscoveryRequest`/`DiscoveryResponse` ACK/NACK + version/nonce handshake, the `Secret` parse, the `sds.*` stats scaffold, proven at UNIT level against an in-process fake SDS server. No TLS integration yet — envoy-go can open an SDS stream, send the initial request, receive+ACK a Secret. (Substrate-only leg with unit tests, no differential surface — the phase-51 precedent.)
- **60.2 (`xds-sds-tls-cert-apply`)** — plumb the SDS-delivered Secret into the downstream TLS context (lift `tls/config.go:153`, the one arm), the listener boot/warmup **initial-fetch** gate (§2.5), and the driver-owned-SDS differential fixture asserting the served leaf cert (§2.6). The OBSERVABLE end-to-end row.

This mirrors the landed keystone/applier splits: phase 39.1/39.2 (health-check substrate then checkers) and phase 46.1a/46.1b (tracing header-engine then span+OTLP). The escape valve is documented ARMABLE for a further split if 60.2's warmup surgery surprises upward. **If** the SPEC finds the total under `~15` tasks (plausible if the substrate leg is thin), it MAY land as a single flat row — SPEC decides.

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for xDS/SDS. `internal/xds/doc.go` (`:1-5`) is the phase-00 placeholder that names this exact family expansion ("The real implementation lands in the xDS family (phases 09+)"). This BRAINSTORM is that expansion's first row.

### 1.7 Phase 60's relationship to the existing seams (reuse the gRPC Dialer + the TLS build; add the discovery stream)

The project already has TWO of the three primitives a dynamic-config path needs:
- **(a) the gRPC dial seam** — `grpcclient.Dialer` (`grpcclient.go`; ADR-0158) maps a cluster name → `*grpc.ClientConn`, already used by ext_authz/ALS/OTLP/metrics_service. The SDS management server is a NAMED STATIC cluster; the SDS stream client dials it through the SAME Dialer (§3.1). The Dialer's doc even anticipates "future hot-reload phases will introduce a close-on-replacement discipline" — SDS is a first customer.
- **(b) the TLS build** — `tls.Config` (`tls/config.go`) builds a server TLS context from cert bytes; once the SDS-delivered leaf is in hand, the EXISTING build path consumes it (the only change is the SOURCE of the bytes + the reject lift).

The ONE genuinely novel primitive is **(c) the discovery-stream client + ACK/NACK handshake** — the `internal/xds` package. That is the family-opening substrate; everything else is reuse + a narrow reject lift + config threading + a warmup gate.

---

## 2. Design decisions

### 2.1 Family + subject: OPEN xDS with SDS (downstream server cert, SotW, initial-fetch) *(SELF-PICKED per the standing directive → phase 60 row registered)*

Made AUTONOMOUSLY (no human pick) per the 2026-07-12 standing directive. The escalation to a NEW family is deliberate (§ header note); within xDS the pick is the smallest-defensible-slice.

**Why SDS-server-cert (SotW, initial-fetch) is smallest-defensible for OPENING the family:**
1. It opens the discovery-stream substrate (the family's whole point) — a real gRPC `StreamSecrets` stream with the version/nonce ACK/NACK handshake, reusable by every later xDS row.
2. It touches the LEAST of the boot model: the SDS `ConfigSource` is inline in the TLS context, so the `dynamic_resources` reject (`bootstrap.go:499`) stays; only the narrow `tls/config.go:153` arm lifts (§1.2).
3. It reuses the MOST substrate: `grpcclient.Dialer` for the dial (§3.1) + the existing `tls.Config` build for the cert (§1.7).
4. Scoped to INITIAL-FETCH-only, the cert is built once and stays static — NO mutable-cert seam, NO running-subsystem mutation (§2.5). This is the least-new-lifecycle property.
5. It is cleanly differentially observable: a TLS client sees the served leaf cert identically on both sides (§2.6).
6. The single resource type (a `Secret`) is bounded and small.

**Rejected alternatives (recorded per the standing directive; each SIZED against source this session):**
- **RTDS (Runtime Discovery Service)** — the simplest APPLIER (a runtime resource is a flat key→value map; no listener/cluster rebuild). BUT `internal/runtime/doc.go` (`:1-5`) is a greenfield PLACEHOLDER — there is NO runtime layer, and (crucially) NOTHING in envoy-go READS runtime values, so an RTDS-delivered value applies to nothing OBSERVABLE — it is not differentially provable without ALSO building a runtime layer AND a runtime consumer AND (likely) an admin `/runtime` dump. RTDS also spans TWO family stubs (it appears under both `### xDS` and `### Runtime + hot restart family` — "Runtime layer (RTDS consumer)"), so its ownership is ambiguous. It presupposes the whole Runtime layer. HIGH scope, LOW observability. Deferred.
- **CDS-only dynamic clusters** — clusters have a clean `Manager.Get(name)` seam (`cluster/manager.go:206`) and CDS is the canonical "hello-world" of xDS. BUT it REQUIRES lifting the `dynamic_resources` reject (`bootstrap.go:499`) and reshaping the boot model; the cluster `Manager` builds ONCE from static bootstrap (`:75-86`) with no post-boot insertion seam; and CDS clusters commonly reference **EDS** for endpoints, so you must either drag in EDS or add a documented "inline/STRICT_DNS endpoints only" strict-reject. Plus cluster-warmup semantics. Medium-high; more boot-model surgery than SDS. Deferred.
- **LDS-only dynamic listeners** — observable (the listener starts serving) BUT the heaviest lifecycle: `listener.Manager.NewManager` is an 11-parameter construction that binds sockets (`listener/manager.go:254-255`), and dynamic listener add/remove/in-place-update needs socket bind/unbind + connection draining. Highest lifecycle complexity + the `dynamic_resources` lift. Deferred.
- **SDS with rotation (dynamic re-delivery)** — the "real" SDS story (certs rotate without a listener restart). BUT that needs a MUTABLE cert seam in the TLS stack (the served cert must swap under live connections) — a genuinely new running-subsystem mutation. Scoping phase 60 to INITIAL-FETCH-only defers exactly this. It is the immediate 60-follow-on. Deferred (§8).
- **ADS (single muxed stream) as the opener** — ADS multiplexes ALL resource types onto one stream with a strict apply-ordering (CDS→EDS→LDS→RDS). It presupposes ≥2 resource-type appliers to be meaningful and adds the muxing/ordering machinery. Larger than a single dedicated stream. Deferred; the reconnection/mux story lands once ≥2 resource types exist (§8).

### 2.2 Scope: DOWNSTREAM server cert ONLY; `validation_context` SDS + upstream SDS stay rejected *(self-answered; the incremental-arm precedent)*

The `CommonTlsContext` (`tls/config.go`) has two SDS entry points: `tls_certificate_sds_secret_configs` (the server's own leaf cert+key; `:153`) and `validation_context_sds_secret_config` (the peer-verification CA/trust bundle; `:158`). Phase 60 lifts ONLY the first, and ONLY on the DOWNSTREAM (server) side. The `validation_context` SDS arm and the UPSTREAM (client-cert) SDS arm stay PARSE-REJECTED with their existing/distinct substrings (envoy-go-strict, ADR-0080). This mirrors the project's landed incremental-arm posture (e.g. OTel `envoy_grpc`-transport-only, Zipkin `HTTP_JSON`-only). A downstream server cert is a complete, useful, deterministic capability and is the most directly observable SDS surface (client-side capture of the served leaf).

### 2.3 Transport: a DEDICATED SDS stream (`StreamSecrets`), NOT ADS *(self-answered; SPEC confirms D-XDS-CONFIGSOURCE)*

The SDS `ConfigSource` supports `api_config_source` (a dedicated gRPC stream to a named cluster) and `ads` (mux onto the aggregated stream). Phase 60 supports `api_config_source` with a single `ENVOY_GRPC` `GrpcService` targeting a named static cluster; `ads`/`self`/`google_grpc` reject loudly (§2.7). The service is **`envoy.service.secret.v3.SecretDiscoveryService`** (confirmed present: `service/secret/v3/sds_grpc.pb.go` exposes `SecretDiscoveryServiceClient` with `StreamSecrets` [SotW] and `DeltaSecrets` [Delta]). Phase 60 uses `StreamSecrets` (SotW; §2.4). There is a LANDED precedent for the ConfigSource-oneof dispatch: the oauth2 filter already switches on `ConfigSource_ApiConfigSource` and rejects it in favor of filesystem SDS (`oauth2/compiled_config.go:657-658`, `parseRejectSDSApiConfigSource` `:274`) — phase 60 does the INVERSE (accepts the gRPC `ApiConfigSource` arm for the TLS-cert case). **D-XDS-CONFIGSOURCE** pins the exact accepted `ApiConfigSource` shape (transport_api_version V3, `ENVOY_GRPC`, one `grpc_service`) + the reject substrings for the other arms.

### 2.4 Wire model: State-of-the-World (`StreamSecrets`), NOT Delta *(self-answered; SPEC confirms D-XDS-SOTW-VS-DELTA)*

SotW is the simpler protocol: the client sends a `DiscoveryRequest{version_info, node, resource_names, type_url, response_nonce, error_detail}`; the server returns a `DiscoveryResponse{version_info, resources[], type_url, nonce}` carrying the FULL state; the client ACKs by echoing `(version_info, nonce)` or NACKs by setting `error_detail` and keeping the prior `version_info`. Delta (`DeltaSecrets`) adds incremental add/remove tracking — deferred. **D-XDS-SOTW-VS-DELTA** confirms SotW and that the reference accepts a SotW SDS stream identically. The `type_url` for a Secret is `type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret` (SPEC re-derives via `proto.MessageName`, per `reference_network_filter_typeurl_extensions` — do NOT trust the SPEC string; blank-import the proto).

### 2.5 Lifecycle: INITIAL-FETCH, BLOCKING warmup — the cert is built ONCE, then static (no mutable-cert seam) *(self-answered shape; the empirical warmup behavior is D-XDS-INITIAL-FETCH)*

The `tls.Config` is built at listener-construction time from cert bytes (`tls/config.go`). Phase 60 scopes SDS to INITIAL-FETCH: listener construction (or first-serve) **blocks** on the first `DiscoveryResponse` carrying the Secret, bounded by the SDS `ConfigSource.initial_fetch_timeout` (reference default 15s). Once the leaf arrives, the TLS context is built ONCE and is thereafter immutable — subsequent SDS updates are IGNORED (rotation deferred, §8). This is the property that avoids a mutable-cert seam and any running-subsystem mutation — the ONLY new lifecycle is one warmup gate. **D-XDS-INITIAL-FETCH (live-probe, MUST):** does the reference BLOCK the listener/TLS handshake on the initial fetch? What happens on `initial_fetch_timeout` expiry — does the reference serve without a cert (handshake fails), tear the listener down, or boot-fail? This is the single most load-bearing empirical question and the biggest scope risk (whether the TLS build can be cleanly deferred without a broader listener-warmup refactor — a code-read design D-question too, D-XDS-CONFIG-SEAM).

### 2.6 Fixture posture: ONE new differential fixture with a DRIVER-OWNED SDS management server *(self-answered direction; SPEC confirms D-XDS-FIXTURE)*

A downstream SDS server cert IS differentially provable: a fixture stands up an SDS management server that delivers a known `Secret`, points both the reference and the subject listener's `DownstreamTlsContext` at it, and asserts the client-observed served leaf cert (serial / SAN / SPKI) is identical cross-side. Per `reference_differential_grpc_receiver_driver_owned`, a gRPC service the proxy DIALS is a `test/helpers` server, NOT a runner `BackendKind` — so the SDS management server is a driver-owned `test/helpers/sdsserver` (analogous to the OTLP/ALS receivers), and BackendKind stays **38 (+0)**. The stream is proxy→server (the proxy dials), reachable via a shared bridge network + the host-gateway IP (`reference_docker_probe_bridge_network`, `reference_host_gateway_ip_docker_desktop`). Anticipated fixtures **103 → 104** (ONE new `NNNN-xds-sds-server-cert` dir); the cross-side assertion is per-property `Errorf` on the served leaf (never `Fatalf`, `reference_fatalf_makes_assertions_unreachable`) and its liveness proven by a `-count=1` deliberate break (`reference_differential_break_protocol_count1`, `reference_deliberate_break_wrong_assertion`). **D-XDS-FIXTURE** pins the fixture shape (BackendCount ≥1 per `reference_differential_backendcount_min_one`; the served-cert capture mechanism; whether the SDS-down / NACK / init-fetch-timeout scenarios are separate fixture dirs per the one-dir-one-runner-branch constraint, `reference_differential_fixture_dispatch_constraint`).

### 2.7 The unsupported ConfigSource / SDS arms reject loudly with DISTINCT substrings — an envoy-go-strict DEPARTURE *(self-answered; ADR-0080)*

The reference supports ADS-sourced SDS, Delta SDS, `google_grpc`, validation_context SDS, upstream SDS, and rotation; envoy-go rejecting them is a documented envoy-go-strict DEPARTURE (like the OTel-transport and Zipkin-version rejects), NOT a parity claim. Each reject carries its own distinct substring (ADR-0080 anti-silent-divergence), anticipated (SPEC re-derives exact text — D-XDS-STRICT-REJECT):
- `xds: sds: ads-sourced ConfigSource unsupported` (only `api_config_source` supported)
- `xds: sds: DeltaSecrets (delta xDS) unsupported` (only StreamSecrets/SotW)
- `xds: sds: google_grpc transport unsupported` (only ENVOY_GRPC)
- `tls: <side>: validation_context_sds_secret_config is not supported` (existing `:158` reject STAYS)
- upstream (client-cert) SDS stays rejected (the reject arm on the upstream TLS path)
Plus any PGV-derived structural rejects on the `SdsSecretConfig` (an empty `name`, a missing `sds_config`). **D-XDS-STRICT-REJECT** confirms (one probe arm) that the reference ACCEPTS these forms — so the departure is real (the reference boots where envoy-go rejects), per `reference_strict_reject_sibling_typeurl_gap` (each lifted/rejected arm is EXPLICIT, never a silent fall-through).

### 2.8 Reconnection / management-server-down posture: a documented MVP *(self-answered shape; the reference behavior is D-XDS-MGMT-DOWN)*

The SDS management server may be unreachable at boot or drop mid-stream. Phase 60 ships a documented MVP (a single reconnect attempt / bounded retry, exact policy SPEC-pinned) rather than the full exponential-backoff reconnection story (deferred to a dedicated "reconnection" row, §8). **D-XDS-MGMT-DOWN (live-probe, MUST):** what does the reference do when the SDS server is DOWN at boot — boot-and-serve-degraded (TLS handshake fails until the server appears), block the listener, or crash? And on a mid-stream drop, does it keep serving the last-good cert (yes, for initial-fetch-scoped) and reconnect? The MVP mirrors the reference's boot-time behavior for the initial fetch; full backoff is deferred.

### 2.9 Stat surface hypothesis: +N `sds.*` (+ reused `cluster.*`) *(self-answered direction; SPEC pins the exact set via a live probe)*

The reference emits per-secret SDS stats under a dynamic scope, anticipated a subset of `sds.<secret_name>.update_success` / `update_rejected` / `update_failure` / `update_empty` / `init_fetch_timeout` / `update_attempt` / `update_time` (a gauge). The SDS stream itself reuses the mgmt cluster's existing `cluster.<sds_cluster>.upstream_cx_*` counters (already in the surface — no new names there; but note `reference_cluster_sink_dial_unaccounted`: verify whether an SDS stream dialed via the `grpcclient.Dialer` is or is NOT accounted under `cluster.*`, and DOCUMENT the answer). Anticipated stat surface **1201 → ~1207–1209** (best estimate; SPEC pins the exact subset via a live `/stats` probe — `reference_stats_sink_emits_used_only`: the reference emits only USED stats, so assert a named subset, not the whole registry). Dynamic stat segments derived from the secret NAME must pass `stats.IsValidName` before `NewCounterIfAbsent` (`reference_dynamic_stat_name_charset_guard` — the codec-boundary guard) — a secret name is operator-controlled and may contain invalid charset.

---

## 3. Framework-survey result — reuse the gRPC Dialer + the TLS build; add the `internal/xds` discovery-stream client; ZERO new modules (60 anticipated)

### 3.1 Framework: a NEW `internal/xds` discovery-stream client (reuses `grpcclient.Dialer`)

The novel primitive is the SDS discovery-stream client, anticipated in `internal/xds` (the greenfield placeholder package becomes real, `xds/doc.go:1-5`). It reuses **`grpcclient.Dialer`** (`grpcclient.go`; ADR-0158) to obtain the `*grpc.ClientConn` for the named static SDS cluster (the SAME dial seam as ext_authz/ALS/OTLP), then wraps `secretv3.NewSecretDiscoveryServiceClient(conn).StreamSecrets(ctx)` and runs the SotW send/receive + ACK/NACK loop (§2.4). A likely small interface (a `SecretProvider` the TLS build blocks on for the first Secret — §2.5) is the seam between `internal/xds` and `internal/tls`. **D-XDS-CONFIG-SEAM** pins the package layout (one `internal/xds` package vs a `internal/xds` + `internal/xds/sds` split) and the provider interface shape.

### 3.2 NEW packages: ≥1 production (`internal/xds`) + a test helper (`test/helpers/sdsserver`)

At least ONE new production package (`internal/xds`, greenfield → real). Possibly a second (`internal/xds/sds`) if the SPEC separates the generic discovery-stream core from the Secret-specific applier — deferred to D-XDS-CONFIG-SEAM. Plus a driver-owned `test/helpers/sdsserver` (a fake SDS management server, NOT a package the production binary imports). Anticipated new Go packages **+1 to +3** (SPEC pins).

### 3.3 go.mod modules: NONE

The discovery-service protos are ALREADY reachable via the resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module — CONFIRMED this session: `service/secret/v3/sds_grpc.pb.go` (`SecretDiscoveryServiceClient`, `StreamSecrets`/`DeltaSecrets`), `service/discovery/v3/discovery.pb.go` (`DiscoveryRequest`/`DiscoveryResponse`/`DeltaDiscoveryRequest`/`DeltaDiscoveryResponse`), and the `Secret` type under `envoy.extensions.transport_sockets.tls.v3`. NO new proto module. `go mod tidy -diff` anticipated EMPTY (new EXISTING-module imports only). New go.mod modules **0**.

### 3.4 REUSES

- **phase-18 (ADR-0158) `grpcclient.Dialer`** — the cluster-name → `*grpc.ClientConn` dial seam for the SDS management-server stream (§3.1).
- **phase-03 `internal/tls`** — the `tls.Config` server-context build consumes the SDS-delivered leaf; ONE arm of the `:153` reject lifts (§1.2/§2.2).
- **the oauth2 ConfigSource-oneof dispatch precedent** (`oauth2/compiled_config.go:657`) as the template for the `api_config_source`-accept / `ads`/`self`-reject arms (§2.3).
- **the driver-owned gRPC-receiver pattern** (`reference_differential_grpc_receiver_driver_owned`, the OTLP/ALS/metrics_service receivers) for `test/helpers/sdsserver` (§2.6).
- **bootstrap `node`** — already parsed/available (`bootstrap.go:657` `result.Proto.GetNode()`, `GetId()`/`GetCluster()`); the SDS `DiscoveryRequest.node` populates from it (whether the reference REQUIRES node for SDS is D-XDS-NODE).

---

## 4. Bootstrap-level applicability — a PER-LISTENER TLS transport-socket config (NOT bootstrap `dynamic_resources`)

The SDS `ConfigSource` is INLINE in the listener's `DownstreamTlsContext.common_tls_context.tls_certificate_sds_secret_configs[]` (§1.2). So — unlike a hypothetical LDS/CDS row — phase 60 makes NO change to the `dynamic_resources` boot path (`bootstrap.go:499` reject STAYS). The only bootstrap-adjacent requirement is a STATIC cluster naming the SDS management server (the `api_config_source.grpc_service` targets it), which the existing static-cluster machinery already provides. The fixture configures the SDS secret + the gRPC `api_config_source` on the listener's TLS context.

---

## 5. Stat surface hypothesis — +N `sds.*` (60)

### 5.1 Stat names (SPEC confirms via a live `/stats` probe)

Anticipated a named subset of `sds.<secret>.{update_success, update_rejected, update_failure, update_empty, init_fetch_timeout, update_attempt}` (counters) + `sds.<secret>.update_time` (gauge). The SDS stream reuses the mgmt cluster's `cluster.<name>.upstream_cx_*` (verify accounting per `reference_cluster_sink_dial_unaccounted`). Exact names + scope path are live-probed (`reference_stats_sink_emits_used_only`; assert a named subset).

### 5.2 envoy-go-strict departure flags

The rejected ConfigSource/SDS arms (§2.7) are a documented envoy-go-strict DEPARTURE (the reference supports them; envoy-go rejects loudly) — recorded in BEHAVIOR_CONTRACT. No new stat for the departures themselves.

### 5.3 Anticipated surface arithmetic

Stat surface **1201 → ~1207–1209** (best estimate; SPEC pins). Guard dynamic secret-name segments with `stats.IsValidName` before registration (`reference_dynamic_stat_name_charset_guard`).

---

## 6. Edit-site enumeration — RE-DERIVED this session (SPEC re-derives + pins D-XDS-CONFIG-SEAM / D-XDS-DOCSHAPE)

Each `file:line` RE-DERIVED against master `958d0154` this session (`feedback_brief_citations_not_evidence`); the SPEC re-derives again.

**NEW production — `internal/xds/`:**
1. **`internal/xds`** (greenfield → real; `xds/doc.go:1-5` placeholder replaced) — the SDS discovery-stream client: dial via `grpcclient.Dialer`, `StreamSecrets`, the SotW `DiscoveryRequest`/`DiscoveryResponse` ACK/NACK + version/nonce loop, the `Secret` parse, the initial-fetch blocking gate, the `sds.*` stat registration. [ADD — the family-opening substrate; 60.1]

**Production — `internal/tls/config.go`:**
2. **Lift the ONE `tls_certificate_sds_secret_configs` reject arm** (`:153-154`) for a gRPC `api_config_source` SDS secret — parse the `SdsSecretConfig{name, sds_config}`, dispatch to the `internal/xds` provider, block on the first Secret (§2.5), build the server context from it. Reject the other arms (`ads`/`google_grpc`/`self`/delta; the `validation_context` arm `:158` STAYS). [EDIT — 60.2]

**Production — `internal/listener/manager.go`:**
3. **Listener warmup/initial-fetch gate** (`NewManager*`, `:200-255`; `Start`, `:855`) — thread the SDS provider so listener construction/first-serve blocks on the initial fetch (§2.5, D-XDS-CONFIG-SEAM — the exact seam). [EDIT — 60.2; the biggest scope risk]

**Production — `cmd/envoy-go/main.go`:**
4. **Wire the `internal/xds` client** (the SDS provider) into the boot sequence alongside the existing Dialer construction (`internal/boot.Construct` per ADR-0268). [EDIT — 60.2]

**Test:**
5. **`internal/xds/*_test.go`** — unit tests for the discovery-stream client against an in-process fake SDS server (SotW ACK/NACK; Secret parse; init-fetch timeout; NACK). [ADD — 60.1]
6. **`internal/tls/config_test.go`** — accept a gRPC-SDS cert config; reject the `ads`/`google_grpc`/delta/validation_context arms (distinct substrings). [ADD — 60.2]
7. **A NEW fuzzer** — likely `FuzzDiscoveryResponseParse` / `FuzzSecretParse` (the `DiscoveryResponse`→`Secret` parse is a NEW wire-parse surface; a genuinely new fuzzer, not a seed — but SPEC confirms per D-XDS-FUZZER). fuzzers **54 → 55** (anticipated). [ADD — 60.1]

**Test helper:**
8. **`test/helpers/sdsserver`** — a driver-owned fake SDS management server delivering a known Secret (§2.6). [ADD]

**Fixture:**
9. **`test/fixtures/NNNN-xds-sds-server-cert`** (new) — a downstream `DownstreamTlsContext` with a gRPC-SDS cert; assert the client-observed served leaf identically cross-side. Possibly separate dirs for the SDS-down / NACK / init-fetch-timeout scenarios (one-dir-one-runner-branch, D-XDS-FIXTURE). fixtures **103 → 104** (or more). [ADD — 60.2]

**BEHAVIOR_CONTRACT (`docs/envoy-go/BEHAVIOR_CONTRACT.md`):**
10. **a NEW xDS/SDS section** — SDS server-cert supported (SotW, api_config_source, initial-fetch); the departures (ads/delta/google_grpc/validation_context/upstream/rotation reject loudly); the `dynamic_resources` reject STILL stands. SPEC RE-DERIVES the exact lines. [EDIT]

**ROADMAP / STATE / DECISIONS (controller-owned; NOT edited by this BRAINSTORM):**
11. **ROADMAP** — the controller adds row 60 (`in-progress`), expands the `### xDS / dynamic config family` stub into a LIVE deferred-candidate sentence, and (if split) adds 60.1/60.2 rows. **STATE.md** — active-phase flips to phase 60. **DECISIONS.md** — ADR-0278 §Context at the SPEC, §Decision/§Consequences at the IMPL (ADR-0044). [Controller/SPEC/IMPL — NOT this stage.]

SPEC pins **D-XDS-DOCSHAPE** (this full edit-site roster, RE-DERIVED) + **D-XDS-CONFIG-SEAM** (the `internal/xds` package layout + the `SecretProvider` interface + the listener-warmup seam).

---

## 7. Anticipated ADRs — 1 (possibly 2) at the phase-60 IMPL: ADR-0278 (xDS SDS substrate)

**ADR-0278 (opening xDS via SDS: the discovery-stream substrate + the SotW ACK/NACK handshake + the downstream SDS server-cert apply + the initial-fetch warmup + the strict-reject posture).** §Context drafted at the SPEC (the gap's provenance: the 100%-static boot model + the `tls/config.go:153` + `bootstrap.go:499` rejects + the ROADMAP xDS stub), §Decision/§Consequences at the IMPL per ADR-0044. A SECOND **seam ADR** is plausible (the `internal/xds` discovery-stream-client interface + the `SecretProvider` blocking seam — a cross-phase-reusable substrate that CDS/LDS/RDS/EDS will build on, so it may warrant its own ADR unlike the phase-58/59 folded seams). The SPEC decides 1-vs-2. Next-free after: **ADR-0279** (if one) or **ADR-0280** (if two). (ADR-0277 is reserved for phase-59.)

---

## 8. Deferred items (the xDS family candidate list — for the controller's deferred sentence)

- **SDS rotation / dynamic re-delivery** — the mutable-cert seam (a rotated cert swaps under live connections); the immediate 60-follow-on. Carries forward.
- **SDS `validation_context`** (CA/trust-bundle via SDS; `tls/config.go:158` reject) + **upstream (client-cert) SDS**. Carries forward.
- **CDS** (dynamic clusters) + **EDS** (endpoints) — need the `dynamic_resources` lift + a cluster-insertion/warmup seam. Carries forward.
- **LDS** (dynamic listeners) + **RDS** (dynamic routes) — the socket/route lifecycle. Carries forward.
- **ADS** (single muxed stream, CDS→EDS→LDS→RDS apply-ordering) — meaningful once ≥2 resource types exist. Carries forward.
- **Delta xDS** (`DeltaSecrets` and the delta variants of each service). Carries forward.
- **RTDS** + the **Runtime layer** (`internal/runtime` placeholder → real) + a runtime CONSUMER + admin `/runtime` — spans the Runtime family too (§2.1). Carries forward.
- **Reconnection / exponential backoff** hardening + **`initial_fetch_timeout`** edge cases beyond the MVP (§2.8). Carries forward.
- **`google_grpc` transport** for xDS (vs the supported `ENVOY_GRPC`). Carries forward.

After row 60 the family STAYS OPEN (all of the above remain) ⇒ the sentinel check-(2)/(3) STILL print ⇒ the loop continues. The controller writes the LIVE deferred sentence at the phase-60 IMPL (re-run the check-(2) grep, EXACTLY ONE live "candidates:" match, `reference_sentinel_deferred_sentence_live_vs_historical`).

---

## 9. Cross-references against prior phases' deferred-items lists — a NEW family opens

Phase 60 does NOT pick up a deferred candidate from an existing open family (Observability/Operational-tooling) — it OPENS the xDS family (the standing-directive escalation, § header). The `internal/xds`/`internal/runtime` placeholders (`doc.go:1-5` each) have named this expansion since phase 00. The **`local_ratelimit` deferral clusters** (phase-11) named an "xDS cluster-state" deferral — orthogonal to SDS (that is a global-ratelimit descriptor concern, not TLS-cert SDS). No prior deferred-item is CONSUMED here; the xDS family's own deferred list is CREATED here (§8).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D-XDS-CONFIGSOURCE** — the accepted `api_config_source` shape (transport_api_version V3, `ENVOY_GRPC`, one `grpc_service` → named static cluster); reject `ads`/`self`/`google_grpc` with distinct substrings. §2.3.
- **D-XDS-SOTW-VS-DELTA** — confirm SotW (`StreamSecrets`), reject Delta; the `type_url` `envoy.extensions.transport_sockets.tls.v3.Secret` (re-derive via `proto.MessageName`, blank-import). §2.4.
- **D-XDS-HANDSHAKE** — the initial `DiscoveryRequest` shape (empty `version_info`, `resource_names=[secret]`, `node` populated, `type_url` set) + the ACK (echo version+nonce) + the NACK (`error_detail`, keep prior version). ONE fresh-container probe against `envoyproxy/envoy:contrib-v1.37.2` observed at a driver-owned SDS server (`reference_probe_fresh_container_per_arm`, `reference_envoy_contrib_image_tagging`, `reference_docker_probe_bridge_network`). §2.4.
- **D-XDS-INITIAL-FETCH (MUST — the load-bearing probe)** — does the reference BLOCK the listener/TLS handshake on the initial fetch? `initial_fetch_timeout` (default 15s) expiry behavior (serve-without-cert / tear-down / boot-fail)? §2.5.
- **D-XDS-MGMT-DOWN (MUST)** — reference behavior when the SDS server is DOWN at boot + on a mid-stream drop (serve-degraded / block / crash / reconnect). §2.8.
- **D-XDS-SECRET-NOTFOUND** — the response-without-the-requested-secret / empty-resources semantics (`update_empty`?). §2.9.
- **D-XDS-SECRET-WIRE** — the exact `Secret{tls_certificate{certificate_chain, private_key}}` shape delivered + how the served leaf is client-observable (serial/SAN/SPKI). §2.6.
- **D-XDS-STRICT-REJECT** — the exact reject substrings (ADR-0080 distinct) for ads/delta/google_grpc/validation_context/upstream/rotation; confirm (one probe arm) the reference ACCEPTS these forms (the departure is real). §2.7.
- **D-XDS-STATS** — the exact `sds.*` name subset + scope path (live `/stats` probe); whether the SDS stream is accounted under `cluster.*` (`reference_cluster_sink_dial_unaccounted`); the `stats.IsValidName` guard on secret-name segments. §2.9/§5.
- **D-XDS-CONFIG-SEAM** — the `internal/xds` package layout (one package vs `internal/xds`+`internal/xds/sds`) + the `SecretProvider` blocking-interface shape + the listener-warmup seam (`listener/manager.go:200-255,855`) — RE-DERIVE + a code-read of whether the TLS build defers cleanly without a broader warmup refactor. §2.5/§3.1.
- **D-XDS-NODE** — does the reference REQUIRE `node.id`/`node.cluster` for SDS (as it does for TCP statsd, `bootstrap.go:657-659`)? The node is already available; whether it must be populated on the `DiscoveryRequest`. §3.4.
- **D-XDS-FIXTURE** — ONE `NNNN-xds-sds-server-cert` dir vs several (happy-path + SDS-down + NACK + init-fetch-timeout, one-dir-one-runner-branch, `reference_differential_fixture_dispatch_constraint`); the driver-owned `test/helpers/sdsserver` shape; the served-cert cross-side assertion (StatsAsserter/SubjectAsserter, `reference_differential_asserter_dispatch`); BackendCount ≥1. fixtures **103 → 104**(+). §2.6.
- **D-XDS-FUZZER** — a NEW `FuzzDiscoveryResponseParse`/`FuzzSecretParse` (the discovery-response→Secret parse is a new wire surface — likely a NEW fuzzer, not a seed); reconcile the running total (`reference_fuzzer_count_docs_drift`) before AND after. fuzzers **54 → 55** (anticipated). §6.
- **D-XDS-SPLIT** — the ADR-0045 disposition: a SPLIT (60.1 substrate / 60.2 TLS-apply + differential) is anticipated; SPEC confirms the exact cut or folds to a single row if under ~15 tasks. §1.5.

---

## 11. Prior-phase lessons applied

- **`feedback_brief_citations_not_evidence`** — EVERY `file:line` here (`bootstrap.go:499-504`/`:657-659`, `tls/config.go:153-159`, `cluster/manager.go:75-86,206`, `listener/manager.go:200-255,855`, `grpcclient.go`, `oauth2/compiled_config.go:274,657-658`, `xds/doc.go:1-5`, `runtime/doc.go:1-5`, the `service/secret/v3/sds_grpc.pb.go` symbols) was RE-DERIVED from source this session; the SPEC re-derives again.
- **`feedback_git_worktrees` / `feedback_subagent_worktree_path_targeting`** — all work in `.worktrees/phase-60-brainstorm` (verified via `git rev-parse --show-toplevel`); ONE deliverable file; no shared-doc edits.
- **`reference_differential_grpc_receiver_driver_owned`** — the SDS management server is a driver-owned `test/helpers/sdsserver`, NOT a runner BackendKind (BackendKind stays 38). §2.6.
- **`reference_probe_fresh_container_per_arm`** + **`reference_envoy_contrib_image_tagging`** — each SPEC probe arm (D-XDS-HANDSHAKE / INITIAL-FETCH / MGMT-DOWN / SECRET-WIRE / STRICT-REJECT / STATS) runs on a FRESH container against `envoyproxy/envoy:contrib-v1.37.2`. §10.
- **`reference_docker_probe_bridge_network`** + **`reference_host_gateway_ip_docker_desktop`** — the SDS stream (proxy→server) needs a shared bridge + a reachable driver-owned server; verify the stream ACTUALLY exchanged messages (not a vacuous empty capture). §2.6/§10.
- **`reference_network_filter_typeurl_extensions`** — re-derive the Secret `type_url` via `proto.MessageName` (blank-import the proto), NOT the SPEC string; the `Secret` lives under `extensions.transport_sockets.tls.v3`. §2.4.
- **`reference_stats_sink_emits_used_only`** + **`reference_dynamic_stat_name_charset_guard`** — assert a NAMED SUBSET of `sds.*` (the reference emits only used stats); guard secret-name-derived segments with `stats.IsValidName` before `NewCounterIfAbsent` (which PANICS). §5/§2.9.
- **`reference_cluster_sink_dial_unaccounted`** — verify + DOCUMENT whether the SDS stream dialed via `grpcclient.Dialer` is accounted under `cluster.*` (do not reuse `Cluster.Dial` blindly). §2.9.
- **`reference_differential_fixture_dispatch_constraint`** + **`reference_differential_asserter_dispatch`** + **`reference_differential_backendcount_min_one`** — a new dir per runner branch; the served-cert assertion needs the right asserter; BackendCount ≥1. §2.6.
- **`reference_fatalf_makes_assertions_unreachable`** + **`reference_differential_break_protocol_count1`** + **`reference_deliberate_break_wrong_assertion`** — per-property `Errorf`; prove each assertion LIVE with a `-count=1` break confirming WHICH fired. §2.6.
- **`reference_fuzzer_count_docs_drift`** — reconcile the running total before AND after (a NEW fuzzer moves it 54 → 55; confirm against `^func Fuzz`). §6/§10.
- **`reference_strict_reject_sibling_typeurl_gap`** + **ADR-0080** — each unsupported ConfigSource/SDS arm gets an EXPLICIT distinct-substring reject, never a silent fall-through. §2.7.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — the controller writes the xDS deferred sentence at the IMPL; re-run the check-(2) grep (EXACTLY ONE live "candidates:" match); opening xDS drops check-(3) from 5 → 4 never-opened families (still prints). §1.4/§8.
- **ADR-0106 (`reference_roadmap_split_phase_row_done`)** — if split, row 60 flips `done` only once BOTH legs (60.1 + 60.2) land. §1.5.

---

## 12. Section closeout

**Settled:** the escalation to OPEN the xDS family (the standing-directive autonomous pick, § header/§2.1); the subject (SDS for a downstream server TLS cert, SotW, single dedicated `StreamSecrets` stream, gRPC `api_config_source`, INITIAL-FETCH scoped — over four declined alternatives {RTDS, CDS, LDS, SDS-rotation} + ADS-as-opener, §2.1); the killer property (SDS is inline in the TLS context → opens xDS WITHOUT lifting the `dynamic_resources` reject; only the narrow `tls/config.go:153` arm lifts, §1.2); the reuse story (`grpcclient.Dialer` for the dial + the existing `tls.Config` build for the cert + the oauth2 ConfigSource-dispatch precedent, §3/§1.7); the lifecycle (initial-fetch blocking warmup → the cert built once, no mutable-cert seam, §2.5); the strict-reject departures (ads/delta/google_grpc/validation_context/upstream/rotation reject loudly, distinct substrings, §2.7); the fixture posture (a driver-owned `test/helpers/sdsserver` + ONE new differential dir asserting the served leaf, §2.6); the envelope (a SPLIT — 60.1 substrate / 60.2 TLS-apply+differential — anticipated, ADR-0278 (+ possibly a seam ADR), §1.5/§7). The novel production code is the `internal/xds` discovery-stream client (the family-opening substrate) + the one-arm `tls/config.go` reject lift + the listener-warmup gate.

**Anticipated moves at the phase-60 IMPL (docs-only now):** the `internal/xds` SDS discovery-stream client (SotW ACK/NACK + init-fetch gate + `sds.*` stats) + the `tls/config.go:153` one-arm lift + the listener-warmup seam + the `main.go`/`boot.Construct` wiring + unit tests (in-process fake SDS) + the `tls/config_test.go` accept/reject tests + a NEW `FuzzDiscoveryResponseParse` + the driver-owned `test/helpers/sdsserver` + the `NNNN-xds-sds-server-cert` differential fixture + the BEHAVIOR_CONTRACT xDS section + ADR-0278 + (controller) the ROADMAP row 60 / xDS-stub expansion / STATE flip. Counts: stat surface **1201 → ~1207–1209** (SPEC pins) · fixtures **103 → 104**(+) · fuzzers **54 → 55** (a new parse fuzzer) · BackendKind **38 (+0)** (driver-owned server, not a BackendKind) · DECISIONS anchor **ADR-0278** (next-free ADR-0279/0280) · new Go packages **+1 to +3** (`internal/xds`[+`/sds`] + `test/helpers/sdsserver`) · new go.mod modules **0**.

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `958d0154`):** stat surface **1201** · fixtures **103** · fuzzers **54** · BackendKind **38** · DECISIONS tail **ADR-0276** (ADR-0277 reserved phase-59; this phase anticipates ADR-0278). Row 60 registers `in-progress` when the controller registers it per the §Schema invariant (NOT at this BRAINSTORM — the controller owns the shared-doc edits).

**Next → the phase-60 SPEC** (the D-XDS-* live-probe arms against `envoyproxy/envoy:contrib-v1.37.2` — especially D-XDS-INITIAL-FETCH + D-XDS-MGMT-DOWN + D-XDS-HANDSHAKE + D-XDS-STATS, each at a driver-owned SDS server on a bridge network; re-derive every §6 edit site + the Secret `type_url` via `proto.MessageName`; pin D-XDS-CONFIG-SEAM (the `internal/xds` layout + the listener-warmup code-read) + D-XDS-SPLIT (the 60.1/60.2 cut); draft ADR-0278 §Context).
