# Phase 18.2 SPEC — `envoy.filters.http.ext_authz` (gRPC service mode + `internal/grpcclient/` primitive)

> **Lifecycle state:** SPEC.md authored; this commit flips ROADMAP row `18.2` `planned → in-progress` (parent row `18` stays `in-progress`; row `18.1` stays `done`) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md`. This SPEC is the authoritative input to the 18.2 PLAN.

**Parent:** `docs/envoy-go/phases/18-http-filter-ext-authz/SPEC.md` (the parent master SPEC — carries the cross-cutting design §4, the 13-pin empirical-pin block §5, the 12-amendment block §6, the 9-ADR anchor map §7, the §3 ADR-0045 split rationale + §8 parent-rollup discipline). This 18.2 SPEC details the gRPC-mode surface only; it REFERENCES the parent's §4/§5/§6/§7/§8 rather than repeating them.

**Sibling predecessor:** `docs/envoy-go/phases/18.1-ext-authz-http/SPEC.md` (the foundational filter scaffold + HTTP-mode delivery; `done` at `9dd79ca` per phase-18.1 REVIEW.md §4 Gate E). The mode-agnostic `compiledConfig`/`checkFn`/`checkDisposition` envelope + the 6-counter `filterStats` + the deny-path `SendLocalReply` mechanism + the per-route 5th-canonical REUSE + the async-resume outbound-call leg + boot-registration ALL LAND AT 18.1 and are REUSED unchanged at 18.2 — 18.2 supplies a second `checkFn` constructor (the gRPC arm) + extends `attributes.go` with the gRPC `AttributeContext` builder + ships the NEW `internal/grpcclient/` primitive. This SPEC supersedes `docs/envoy-go/phases/18.2-ext-authz-grpc/README.md` (the sibling stub authored at the parent SPEC commit).

**Sub-phase scope (per parent SPEC §2):** 18.2 lands `envoy.filters.http.ext_authz` in **gRPC service mode** — activates the `grpc_service` arm in the existing `internal/filter/http/extauthz/` package's `compiledConfig` dispatch (replacing the 18.1 PARSE-REJECT; ADR-0157 §Decision amendment); lands the NEW `internal/grpcclient/` gRPC-client framework primitive (ADR-0158 — envoy-go's FIRST gRPC infrastructure of any kind); the gRPC-mode `AttributeContext`/`CheckRequest` builder (ADR-0160 gRPC-mode portion); the `CheckResponse` → `checkDisposition` mapping incl. `OkHttpResponse`/`DeniedHttpResponse` header mutation (ADR-0161 gRPC-mode portion); differential fixture `0021-http-ext-authz-grpc` (8 scenarios); 1 NEW test-helper `test/helpers/extauthzgrpc/` (the FIRST in-process gRPC server in envoy-go's test tree); the 23rd fuzzer `FuzzCheckResponseMapping`.

**ADR continuity:** Phase 18.1 closed at ADR-0164 (the ADR-0045 split-application ADR landed at the parent SPEC commit). 18.2-landing ADRs are **ADR-0158 (§Decision + §Consequences; §Context already anchored at the parent SPEC commit per ADR-0044), ADR-0157 §Decision AMENDMENT (gRPC arm activation), ADR-0160 (gRPC-mode portion §Decision + §Consequences), ADR-0161 (gRPC-mode portion §Decision + §Consequences)**. ADR-0044 escape-valve held in reserve for ~0–1 impl-time-unanticipated ADRs (the §18.P13 in-session closure at SPEC time removes the most-likely escape-valve surface; see §11.P13 below). Next-free ADR is **ADR-0165**.

**Authored:** 2026-05-15.

---

## 1. Purpose

Phase 18.2 lands `envoy.filters.http.ext_authz` in **gRPC service mode** — the secondary-transport landing — completing the phase-18 ADR-0045 split. The mode-agnostic infrastructure (the `compiledConfig` envelope, the `checkDisposition` value, the disposition-application logic, the deny-path `SendLocalReply` mechanism, the async-resume leg, the 6-counter `filterStats`, the per-route 5th-canonical REUSE, boot-registration) is REUSED unchanged from 18.1; 18.2 supplies the gRPC-specific transport + marshalling + builder code. The seven architectural primitives:

1. **NEW `internal/grpcclient/` framework primitive (ADR-0158).** envoy-go's FIRST gRPC infrastructure of any kind. Composes `google.golang.org/grpc` v1.70.0 (already an indirect dep at master tip `0ff9813`; promoted to a direct dep by this phase) + the `envoy.service.auth.v3.Authorization/Check` client stub (ships in go-control-plane v1.32.4 per parent §5.P1 — no codegen). Couples to envoy-go's existing `internal/cluster.Manager` for `core.GrpcService.EnvoyGrpc.cluster_name` resolution via `grpc.WithContextDialer` delegating to `(*cluster.Cluster).Dial(ctx)` (the §18.P13 closure at §11 below confirms this coupling RATIFIED). The package exposes a generic `Dialer` (cluster-name → `*grpc.ClientConn`) + a thin ext_authz-typed `AuthClient` wrapper (`Check(ctx, *CheckRequest) (*CheckResponse, error)`); future ext_proc + global_ratelimit reuse the `Dialer` and add their own typed wrappers per ADR-0158 §Context. One `*grpc.ClientConn` per (cluster_name, compiledConfig) pair — created at config-load time (in `buildGRPCCheckFn`), shared across all per-stream `Check()` calls (gRPC manages its own transport-level reconnect). Closed-on-process-exit for MVP (no hot-reload yet).

2. **`grpc_service` arm activation (ADR-0157 §Decision amendment).** The `*ExtAuthz_GrpcService` switch-arm in `buildCompiledConfig` (`extauthz.go`) — which 18.1 PARSE-REJECTed with `"ext_authz: grpc_service mode not yet supported (lands in phase 18.2)"` — now calls the NEW `buildGRPCCheckFn(gs *ext_authzv3.ExtAuthz_GrpcService, ctx envoyhttp.FactoryCtx) (checkFn, error)`. The `compiledConfig` struct's field shape is UNCHANGED from 18.1 (per ADR-0157 §Decision: "the mode-agnostic `compiledConfig` struct shape is field-final at 18.1") — 18.2 only swaps the `checkFn` closure. The `core.GrpcService.GoogleGrpc` arm PARSE-REJECTs envoy-go-strict (`"ext_authz: grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)"` — mirrors the ADR-0008 V3-only discipline at the GrpcService level).

3. **`buildGRPCCheckFn` in `check.go` — the gRPC-mode `checkFn` analog of `buildHTTPCheckFn`.** Steps: (i) validate `gs.GrpcService.envoy_grpc.cluster_name` PGV-mirror (`min_len: 1` per parent §5.P1); (ii) PARSE-REJECT on `gs.GrpcService.google_grpc` arm; (iii) look up the referenced cluster via `clusterManager.Get(cluster_name)`; PARSE-REJECT if not found, if `cluster.UseH2() == false` (gRPC requires HTTP/2 — `http2_protocol_options{}` MUST be set on the auth cluster); (iv) construct a `*grpcclient.AuthClient` via `grpcclient.NewAuthClient(clusterManager, cluster_name, gs.GrpcService.timeout)` — the `Dialer` internally calls `grpc.NewClient("passthrough:///"+cluster_name, grpc.WithContextDialer(cluster.Dial), grpc.WithTransportCredentials(creds))` where `creds` is `insecure.NewCredentials()` for plaintext clusters or a no-op handshaker (TLS terminates at the `cluster.Dial` layer, not gRPC's TLS layer — the cluster manager already returns a TLS-wrapped `net.Conn` for TLS clusters per the §18.P13 closure); (v) compile the `authorization_response`-analog header-mutation matcher set (gRPC-mode's `OkHttpResponse`/`DeniedHttpResponse` carry header-mutation fields proto-faithful; no per-mode matcher difference here — `validate_mutations` is consumed identically); (vi) return the `checkFn` closure capturing the `*AuthClient` + the merged listener+route `context_extensions` + `validateMutations` + the `encode_raw_headers` discipline + the `pack_as_bytes` shape.

4. **`buildAttributeContext` in `attributes.go` (ADR-0160 gRPC-mode portion).** Constructs the `*authv3.AttributeContext` for each per-stream Check call per parent §5.P4 RATIFIED-AND-REFINED + §11.P4 in-session closure below. Populated set: `source.address.socket_address` (downstream remote IP+port from `f.cb.StreamInfo()` or equivalent); `destination.address.socket_address` (local listener IP+port); `destination.principal` (populated AUTOMATICALLY from the listener TLS cert per the §11.P4 in-session finding — NOT gated by `include_peer_certificate`); `source.principal` (gRPC `Peer.principal` populated via the phase-16 ADR-0144 `DownstreamPrincipal()` reuse — joined-string of URI SAN | DNS SAN | CN per ADR-0144); `request.http.{id, method, headers, path, host, scheme, size, protocol, body | raw_body}` per parent §5.P4 — pseudo-headers `:authority`/`:method`/`:path`/`:scheme` INCLUDED in the headers map, lowercased; `body` (string) when `pack_as_bytes: false`, `raw_body` (bytes) when `pack_as_bytes: true`; `request.time` as a `*timestamppb.Timestamp` (Go-side construction via `timestamppb.Now()`); `tls_session.sni` populated ONLY when `include_tls_session: true` AND the connection is TLS (per parent §5.P3 RATIFIED + §11.P4 in-session SNI confirmation); `peer.certificate` populated ONLY when `include_peer_certificate: true` AND the downstream presented a client cert (parent §5.P3); `metadata_context` + `route_metadata_context` are EMPTY-PROTO-MESSAGE (deferred dynamic-metadata family per §8 item 2); `context_extensions` is the listener-level + per-route `CheckSettings.context_extensions` merge (per-route wins on key conflicts per the proto map-merge convention). The `encode_raw_headers` flag governs the `headers`-vs-`header_map` proto-field choice — `false` (default) populates the legacy `headers` map; `true` populates the `header_map` field as a `core.HeaderMap` preserving header order. Envoy ALSO injects `x-envoy-auth-partial-body: <bool>` + `x-forwarded-proto: <scheme>` + `x-request-id: <uuid>` into the headers map (per §11.P4 in-session finding — these are HCM-injected before ext_authz runs, not ext_authz-specific).

5. **`mapGRPCResponse(*authv3.CheckResponse) checkDisposition` in `check.go` (ADR-0161 gRPC-mode portion).** Per parent §5.P10 RATIFIED + §5.P11 RATIFIED: dispatch on `resp.Status.Code` — `0` (OK) AND `resp.HttpResponse` is `*CheckResponse_OkResponse` (or nil oneof) → **`dispAllow`**; `0` (OK) AND empty `CheckResponse{}` (oneof nil, no `HttpResponse`) → **`dispAllow`**; any non-zero `Status.Code` (PERMISSION_DENIED / UNAUTHENTICATED / any other) AND `resp.HttpResponse` is `*CheckResponse_DeniedResponse` (or nil oneof) → **`dispDeny`**. Allow-path extracts from `OkHttpResponse`: `headers` (`[]*core.HeaderValueOption` — each entry's `append_action` governs set-vs-append) compiled into `upstreamSet` (`OVERWRITE_IF_EXISTS_OR_ADD` or `OVERWRITE_IF_EXISTS` or `APPEND_IF_EXISTS_OR_ADD` — for MVP envoy-go treats OVERWRITE arms as set and APPEND as append per phase-10 header_mutation precedent); `headers_to_remove` (drop from upstream); `response_headers_to_add` (DEFERRED for 18.2 — see §8 item 5 — couples to a future stash-for-HCM-response-mutation framework primitive analogous to phase-14 ADR-0131's encode-side primitive). Deny-path extracts from `DeniedHttpResponse`: `status.code` → `denyStatus` (the auth-decision status code); `body` → `denyBody` (verbatim per parent §5.P11); `headers` → `denyHeaders` (verbatim — UNLIKE HTTP-mode which filters through `allowed_client_headers`, gRPC-mode `DeniedHttpResponse.headers` are applied verbatim per parent §5.P11). `validate_mutations` gating applies identically to both modes — a header-name/value safety violation in `upstreamSet`/`upstreamApp`/`denyHeaders` → `dispInvalid` → `invalid` counter + error posture per ADR-0161 (HTTP-mode portion) + ADR-0156. Transport-level errors (gRPC connect failure, timeout, `ctx.Err()`) → **`dispError`** per parent §5.P10; `failure_mode_allow` / `status_on_error` posture applies identically to both modes.

6. **NEW `test/helpers/extauthzgrpc/` test-helper.** The FIRST in-process gRPC server in envoy-go's test tree. Spawn-per-fixture lifecycle (mirroring `test/helpers/extauthzhttp/` + `test/helpers/jwksbackend/`). API: `New(t testing.TB) *Server` returns a started server bound to `127.0.0.1:0` (ephemeral) over plaintext h2c (TLS-fronted variant gated on whether fixture 0021 needs a TLS-auth-cluster scenario — see §7); `(*Server).Addr() string` returns `host:port`; `(*Server).Script(scenarioName, response)` registers a scriptable `*authv3.CheckResponse` keyed by some discriminator (HTTP path? `:authority`? context-extension value? — the IMPL picks; the fixture configs use the discriminator); `(*Server).Stop()` graceful-shutdown. The helper imports `google.golang.org/grpc` (the first non-indirect grpc import in envoy-go) and `envoy.service.auth.v3` (the first ext_authz proto-binding consumption from a test helper).

7. **Differential fixture `0021-http-ext-authz-grpc`.** 8 scenarios (7 mirroring 0020 + 1 gRPC-only `OkHttpResponse` mutation per the §7 matrix below). Topology mirrors 0020's three-listener split for the `failure_mode_allow` variant (l_test_a allow/deny/body/per-route, l_test_b error→`status_on_error`, l_test_c `failure_mode_allow`). The 23rd fuzzer = NEW `FuzzCheckResponseMapping` at `internal/filter/http/extauthz/fuzz_test.go` — fuzz arbitrary `*authv3.CheckResponse` protobytes → `proto.Unmarshal` → `mapGRPCResponse` → `checkDisposition`, exercising the most error-prone gRPC-mode surface (status.code edge cases; OkHttpResponse/DeniedHttpResponse oneof branches; header-mutation extraction). Existing `FuzzExtAuthzConfigParse` corpus extends to include `grpc_service` config variants — the EXISTING 22nd fuzzer covers the gRPC config-parse path automatically once the arm is activated.

After phase 18.2, the project has its FIRST gRPC infrastructure (`internal/grpcclient/` — cross-phase-reusable for ext_proc + global_ratelimit), the ext_authz filter is dual-mode operational (HTTP service mode from 18.1 + gRPC service mode from 18.2), the parent row `18` closes `done` AT THE SAME COMMIT as row `18.2` (per parent SPEC §8 rollup discipline), and the §9 HTTP filters family has 11 rows landed. The next §9 family-row is numbered `19`.

### 1.1 Empirical-finding-driven scope (per parent SPEC §6 + this SPEC §11)

The 12 §6 amendments in the parent SPEC are the empirical-finding-driven scope revisions for phase 18. The amendments load-bearing for 18.2: **amendment 4** (`include_peer_certificate`/`include_tls_session` are honored-as-gates — §11.P4 in-session confirms the SNI population), **amendment 5** (`AttributeContext` population refined — `Timestamp` shape, omitted-when-empty, lowercased-headers; §11.P4 in-session refines further: `destination.principal` populated AUTOMATICALLY from listener TLS cert, NOT gated; HCM injects `x-forwarded-proto` + `x-request-id` in addition to `x-envoy-auth-partial-body`), **amendment 12** (gRPC dial / TLS-to-auth-cluster — §11.P13 in-session closes RATIFIED). Amendments 1, 6, 7, 8, 9, 10, 11 carry over from 18.1 unchanged (they apply to both modes); amendments 2 (the matcher subset) + 3 (5th-canonical REUSE) are 18.1-load-bearing only. This 18.2 SPEC's §6/§11 carry the in-session refinements; the formal §6 amendment block remains in the parent SPEC.

---

## 2. Non-purposes

Phase 18.2 is a single-sub-phase slice completing the phase-18 ADR-0045 split. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to activate the gRPC service mode under the existing 07.1 framework + the ONE new framework primitive at ADR-0158.

- **2.1 No new fields on `compiledConfig`.** Per ADR-0157 §Decision: the mode-agnostic `compiledConfig` struct shape is field-final at 18.1. 18.2 supplies a different `checkFn` constructor + reuses every other field unchanged.
- **2.2 No new `filterStats` counters.** The 6-counter set (`ok`/`denied`/`error`/`disabled`/`failure_mode_allowed`/`invalid`) is mode-agnostic per ADR-0163; 18.2 exercises them identically. BEHAVIOR_CONTRACT.md stat-table stays at 77 names.
- **2.3 No new ADR-0125 canonical.** 18.2 reuses the 5th canonical 18.1 confirmed via ADR-0163 — `CheckSettings.context_extensions` becomes consumed (it was a parse-only no-op in HTTP-mode 18.1), but the per-route override shape is unchanged.
- **2.4 No ext_authz encode-side filter half.** `Encoder: nil` carries over from 18.1; `allowed_client_headers_on_success` + `OkHttpResponse.response_headers_to_add` both remain DEFERRED (decode-side-only filter shape — see §8 below + parent §6 amendment 9).
- **2.5 Deferred `ExtAuthz` fields** (per parent §4.4 + §8 below): unchanged from 18.1 — the four `*metadata_context_namespaces` + `filter_enabled`/`filter_enabled_metadata`/`deny_at_disable` + `enable_dynamic_metadata_ingestion` + `filter_metadata` + `charge_cluster_response_stats` + `bootstrap_metadata_labels_key` + `emit_filter_state_stats` + `decoder_header_mutation_rules` — all silent-ignored.
- **2.6 Deferred gRPC-specific fields:** `core.GrpcService.GoogleGrpc` arm PARSE-REJECTs envoy-go-strict; `core.GrpcService.initial_metadata` SILENT-IGNORED for MVP (couples to a future control-plane metadata family); `core.GrpcService.retry_policy` SILENT-IGNORED for MVP (gRPC client retry is a follow-up); `OkHttpResponse.response_headers_to_add` DEFERRED (decode-side-only; same family as `allowed_client_headers_on_success`); `OkHttpResponse.query_parameters_to_set` / `query_parameters_to_remove` DEFERRED (path-query subsystem ADR-0112); `CheckResponse.dynamic_metadata` DEFERRED (dynamic-metadata family); `OkHttpResponse.dynamic_metadata` DEFERRED (deprecated; same family).
- **2.7 No xDS-driven dynamic auth-cluster reconfiguration.** The fixture uses static cluster config; xDS-CDS replacement of the auth cluster is not exercised (envoy-go's MVP cluster manager is static-config-only as of master tip). Future xDS-CDS phase covers.
- **2.8 No `response_code_details` emission** — unchanged from 18.1 + 17 + 16; ext_authz's deny-path `response_code_details` (`ext_authz_denied`) is a documented divergence-window joint with phase-16/17/18.1 (§9).
- **2.9 No filter-chain ordering surgery.** ext_authz is already boot-registered at 18.1 (alphabetical between `envoygotest` and `fault`); 18.2 changes nothing about registration.

---

## 3. Framework survey result (ONE new framework primitive in 18.2 + FIVE reuses)

The framework survey evaluated reuse of phase-09-through-18.1 primitives BEFORE proposing new. Findings for 18.2:

- **`internal/cluster/Manager` + `Cluster.Dial(ctx)`:** **REUSED** — the §18.P13 in-session closure (§11) confirms that `Cluster.Dial(ctx)` returns a TLS-wrapped `net.Conn` for TLS clusters (the cluster's `transport_socket` → `parsedTLS *stdtls.Config` resolution is owned by the cluster manager). `grpc.WithContextDialer(cluster.Dial)` integrates cleanly — the gRPC layer treats it as a raw transport, never needing to know about TLS. `Cluster.UseH2()` is the config-validation gate (the auth cluster MUST have `http2_protocol_options{}` set for gRPC framing).
- **Phase-16 ADR-0144 `DownstreamPrincipal() []string`:** **REUSED** — the `AttributeContext.source.principal` (a single string per the proto) is the priority-ordered first value from `DownstreamPrincipal()` (URI SAN → DNS SAN → CN). The ext_authz gRPC-mode is the SECOND cross-phase consumer of ADR-0144 (phase-16 rbac introduced + first-consumed; phase-17 jwt_authn deferred; phase-18.2 is the second consumer).
- **Phase-09 async-resume + per-request cancellable `context.Context`:** **REUSED** — same `StopIteration` + goroutine + `ContinueDecoding()` discipline as 18.1's HTTP-mode `checkFn` invocation. The `OnDestroy` cancel-path threads `ctx` through `AuthClient.Check(ctx, ...)` (which internally threads it into `*grpc.ClientConn.Invoke`); a context cancellation surfaces as `dispError`. NO NEW filter-callback primitive.
- **Phase-13 ADR-0128 decode-side body-buffering:** **REUSED unchanged** — `with_request_body` materializes the body identically; the body is then attached to `AttributeContext.request.http.{body, raw_body}` per `pack_as_bytes`. ADR-0162 §Consequences's `pack_as_bytes` 18.2 honoring lands here.
- **ADR-0085 `SendLocalReply`:** **REUSED unchanged** — deny-path emission identical to HTTP-mode.

**ONE new primitive — `internal/grpcclient/` (ADR-0158).** envoy-go's FIRST gRPC infrastructure. Cross-phase-reusable for future ext_proc + global_ratelimit (the strategic intent recorded in ADR-0158 §Context). See §6.1 + §6.2 for the API + lifecycle.

**ZERO ADR-0125 canonical amendment.** 18.2 consumes `CheckSettings.context_extensions` (the gRPC-mode-only field) but the 5th-canonical disabled-OR-narrower-override shape itself is unchanged — `ExtAuthzPerRoute` carries the same oneof shape 18.1 ratified at parent §5.P2. ADR-0163's no-amendment classification holds.

### 3.1 The `internal/grpcclient/` API surface (ADR-0158 §Decision)

**Disposition: a thin generic `Dialer` + an ext_authz-specific `AuthClient`.** Rationale: the `Dialer` is the cross-phase-reusable layer (cluster-manager coupling + connection lifecycle + transport credentials selection); the typed `Authorization/Check` stub composition lives in an `AuthClient` wrapper to keep the typed concerns ext_authz-specific (future ext_proc/global_ratelimit each add their own typed wrappers against the shared `Dialer`). Mirrors the discipline phase-17 ADR-0150 settled for `internal/jwks/Fetcher` (the JWKS-specific concerns lived in a typed wrapper around stdlib `http.Client`).

**Package surface (~150–250 LoC production):**

```go
package grpcclient

// Dialer constructs *grpc.ClientConn values for envoy-go cluster references.
// It couples to internal/cluster.Manager for cluster-name → endpoint + TLS
// resolution. Cross-phase-reusable for future ext_proc + global_ratelimit.
type Dialer struct {
    mgr *cluster.Manager
}

// New returns a Dialer rooted at the supplied cluster manager.
func New(mgr *cluster.Manager) *Dialer

// DialContext returns a *grpc.ClientConn for the named cluster. The
// underlying transport is provided by (*cluster.Cluster).Dial — the cluster
// manager owns endpoint selection + TLS termination; gRPC is layered on
// top of the resulting net.Conn via grpc.WithContextDialer. PARSE-REJECTs
// (via error return) if the cluster does not exist or UseH2()==false.
// The returned ClientConn is owned by the caller and Close()d at shutdown.
func (d *Dialer) DialContext(ctx context.Context, clusterName string) (*grpc.ClientConn, error)

// AuthClient wraps a *grpc.ClientConn with the typed
// envoy.service.auth.v3.AuthorizationClient stub.
type AuthClient struct {
    conn   *grpc.ClientConn
    stub   authv3.AuthorizationClient
    target string // cluster_name — for logs/errors
}

// NewAuthClient dials the named cluster and returns a typed AuthClient.
// timeout is per-Check (applied via context.WithTimeout in Check).
func NewAuthClient(d *Dialer, clusterName string, timeout time.Duration) (*AuthClient, error)

// Check invokes Authorization/Check on the wrapped ClientConn. The timeout
// configured at construction is applied via context.WithTimeout on each call.
func (a *AuthClient) Check(ctx context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error)

// Close releases the underlying *grpc.ClientConn.
func (a *AuthClient) Close() error
```

**Internal construction.** `DialContext` builds a `*grpc.ClientConn` via:

```go
conn, err := grpc.NewClient(
    "passthrough:///" + clusterName,
    grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
        c, _, err := clu.Dial(ctx)
        return c, err
    }),
    grpc.WithTransportCredentials(insecure.NewCredentials()),
)
```

The `passthrough:///` resolver scheme tells gRPC to forward to a single endpoint without name resolution (we delegate to `Cluster.PickEndpoint()` inside `Cluster.Dial`). `WithContextDialer` lets us hand gRPC the cluster-manager's pre-dialed (and TLS-wrapped, if applicable) `net.Conn`. `insecure.NewCredentials()` is used because TLS is terminated at the cluster-manager layer, not the gRPC layer — gRPC sees a plain `net.Conn` regardless of upstream cluster TLS state (which the §11.P13 in-session scrape RATIFIES against Envoy v1.37.2).

**Connection lifecycle.** One `*grpc.ClientConn` per (cluster_name, `*compiledConfig`) pair. Created in `buildGRPCCheckFn` at config-load time; the closure captures `*AuthClient`; shared across all per-stream Check calls (gRPC manages its own transport-level reconnect via the underlying `grpc.ClientConn`'s sub-channel state machine). On process exit, the `AuthClient` is GC'd; `Close()` is NOT explicitly called for MVP (envoy-go has no config hot-reload yet; the process lifecycle bounds the connection). When envoy-go gains hot-reload, the close-on-replacement discipline lands per a future ADR.

**Cross-phase reuse intent (ADR-0158 §Consequences).** ext_proc reuses `Dialer.DialContext` + composes its own `*ProcessorClient` wrapping `envoy.service.ext_proc.v3.ExternalProcessor.Process` (bidi-stream — extends the Check unary pattern). global_ratelimit reuses `Dialer.DialContext` + composes a `*RateLimitClient` wrapping `envoy.service.ratelimit.v3.RateLimitService.ShouldRateLimit` (unary, structurally identical to ext_authz's Check). The `Dialer` surface is intentionally minimal to support these extensions; no future client coupling is anticipated to require `Dialer` API changes.

### 3.2 No filter-chain ordering surgery

Per §2.9. ext_authz is already registered at 18.1; 18.2 is a within-package extension.

---

## 4. Deny-path wire shape (gRPC-mode additions)

Per parent SPEC §5.P11 RATIFIED + §6 amendment 11 + 18.1 SPEC §4. The mode-agnostic deny-path `SendLocalReply` mechanism is REUSED unchanged. The gRPC-mode-specific extraction:

- **status** — `DeniedHttpResponse.status.code` (a `*type.v3.HttpStatus`; the auth-decision status code). Like HTTP-mode, NOT fixed by the filter. NOTE: parent §5.P10 RATIFIED that a non-zero `CheckResponse.status.code` (any gRPC status, not just `PERMISSION_DENIED`) → **deny**; the HTTP-level deny status comes from `DeniedHttpResponse.status` (or defaults to `403` if `DeniedHttpResponse` is nil but `status.code` is non-zero — the §11.P10 carry-forward).
- **body** — `DeniedHttpResponse.body` reproduced verbatim. `content-length` synthesized by ADR-0085.
- **headers** — `DeniedHttpResponse.headers` (`[]*core.HeaderValueOption`) applied **verbatim** — UNLIKE HTTP-mode which filters through `AuthorizationResponse.allowed_client_headers`. This is the per parent §5.P11 RATIFIED finding (the gRPC mode passes auth-supplied headers wholesale, including `content-type`; HTTP-mode filters via the matcher). `validate_mutations` gating applies if set — a violation drives `dispInvalid` → `invalid` counter + error posture per ADR-0161.
- **`content-type` fallback** — gRPC mode does NOT apply the `text/plain` fallback; `DeniedHttpResponse.headers` carry the operator's chosen `content-type` if any (consistent with the verbatim-pass-through discipline). When `DeniedHttpResponse.headers` does NOT include `content-type`, the framework defaults to no explicit content-type header (Envoy v1.37.2 behavior — see parent §5.P11).

The **error** disposition extraction is identical to HTTP-mode (parent §5.P10 + 18.1 SPEC §4) — transport failure / `ctx.Err()` / gRPC `Unavailable`/`DeadlineExceeded` → `dispError` → `failure_mode_allow` / `status_on_error` posture. The `failure_mode_allow_header_add` upstream `x-envoy-auth-failure-mode-allowed: true` injection is identical.

The over-limit 413 + `connection: close` edge case (parent §5.P5 + ADR-0162) is REUSED unchanged from 18.1's `effectiveWithRequestBody(pr)` + ADR-0128 body-buffering interaction — the auth service is never contacted on `allow_partial_message:false` over-limit; this applies identically to gRPC mode.

---

## 5. Per-route discipline — `context_extensions` consumed (5th-canonical UNCHANGED) + SHARED-stats UNCHANGED

Per parent SPEC §5.P2 RATIFIED + §6 amendment 3 + ADR-0163. 18.1 ratified the 5th-canonical REUSE classification + the PGV wrinkles (`disabled: const:true`; `override` oneof PGV-required). 18.2 adds ONE consumption-status change: **`CheckSettings.context_extensions` is now consumed** (it was a parse-only no-op in HTTP-mode 18.1 per the gRPC-mode-only proto doc-note; 18.2 activates it).

**Listener+per-route merge:** the proto provides NO listener-level `context_extensions` source — `ExtAuthz` has no top-level `context_extensions` field, and `core.GrpcService.initial_metadata` is gRPC-headers, not `AttributeContext` attributes (a distinct concern; DEFERRED per §8 item 2). The EFFECTIVE map for a request is therefore the per-route `CheckSettings.context_extensions` map when the per-route arm is `check_settings`, else empty. The map is assigned directly to `AttributeContext.context_extensions` in `buildAttributeContext`; there is no listener-vs-route conflict to resolve under MVP. A future listener-level source (an ADR-0125-roster amendment or a top-level `ExtAuthz` field if the upstream proto adds one) would extend `buildGRPCCheckFn`'s closure with a listener-baseline map and merge per-route on top per the proto map-merge convention (per-route wins on key collision).

**Per-route 5th-canonical classification UNCHANGED** — the override surface (`disabled` bool OR `check_settings` narrower override) is the same; ADR-0163 codifies. NO ADR-0125 amendment.

**Per-route stats SHARED** unchanged from 18.1 — the per-route override adjusts `context_extensions`/buffering but still calls the same auth service; ADR-0163 SHARED-stats discipline holds.

---

## 6. compiledConfig + code shapes

### 6.1 Public surface

`internal/filter/http/extauthz/` package — public surface UNCHANGED from 18.1. `TypeURL` constant unchanged; `New` factory signature unchanged. 18.2 is a within-package internal extension.

`internal/grpcclient/` package — NEW public surface per §3.1 above. Single new top-level package. Cross-phase-reusable.

### 6.2 `compiledConfig` shape — UNCHANGED from 18.1 (per ADR-0157)

The `compiledConfig` struct landed at 18.1 (`extauthz.go`) is field-final at 18.1 per ADR-0157 §Decision. 18.2 ONLY swaps the `checkFn` closure — no struct field is added, renamed, or moved. The 18.1 `checkFn` was the HTTP-mode closure; 18.2's `checkFn` is the gRPC-mode closure (a different closure produced by a different constructor — `buildGRPCCheckFn` vs `buildHTTPCheckFn`).

```go
// Unchanged from 18.1 — repeated here for reader convenience.
type compiledConfig struct {
    checkFn                   checkFn          // 18.1: HTTP-mode closure; 18.2: gRPC-mode closure
    withRequestBody           *bufferSettings
    failureModeAllow          bool
    failureModeAllowHeaderAdd bool
    statusOnError             uint32
    validateMutations         bool
    clearRouteCache           bool
    allowedHeaders            *stringMatcherList
    disallowedHeaders         *stringMatcherList
    stats                     *filterStats // 6 counters — SHARED with listener+route
    // (18.2 additions inside the closure capture, not in the struct itself)
}
```

The gRPC-mode `checkFn` closure captures: the `*grpcclient.AuthClient` (the dialer + connection + typed stub), the merged listener+route `context_extensions` builder (route-specific extensions thread in via the per-route `*compiledPerRoute`), the `encode_raw_headers` flag, the `pack_as_bytes` shape from `withRequestBody`, the `include_peer_certificate` / `include_tls_session` gates, and the `*compiledConfig.validateMutations` flag (already on the struct). The `*compiledPerRoute` value continues to carry per-route extensions per ADR-0163.

### 6.3 `DecodeHeaders` body — UNCHANGED from 18.1

The top-level dispatch in 18.1's `DecodeHeaders` (18.1 SPEC §6.3) is REUSED unchanged. Steps 1–7 (resolve per-route → check `disabled` short-circuit → check `with_request_body` → build authRequest → async outbound call → resume → apply disposition) are mode-agnostic. The mode-specific behavior lives entirely INSIDE the `cc.checkFn(ctx, authReq)` invocation at step 6.

NOTE: the `authRequest` type 18.1 defined (`method`, `path`, `headers`, `body`) is RETAINED for both modes. In gRPC mode the closure builds the `AttributeContext` from the `*authRequest` (plus per-stream filter state — `f.cb` for principal/tls_session/socket-address access) inside `buildAttributeContext`. This keeps the `DecodeHeaders` body purely mode-agnostic.

### 6.4 `buildCompiledConfig` — `services` oneof dispatch AMENDMENT

The `buildCompiledConfig` switch in `extauthz.go` (the 18.1 version PARSE-REJECTed the gRPC arm):

```go
// 18.2 amendment — replaces the 18.1 PARSE-REJECT on the gRPC arm.
switch s := raw.Services.(type) {
case nil:
    return nil, errors.New("ext_authz: services oneof must be set (neither grpc_service nor http_service is configured)")
case *ext_authzv3.ExtAuthz_HttpService:
    cc.checkFn, err = buildHTTPCheckFn(s.HttpService, cc.validateMutations) // unchanged
case *ext_authzv3.ExtAuthz_GrpcService:
    cc.checkFn, err = buildGRPCCheckFn(s.GrpcService, ctx) // 18.2 NEW — replaces the 18.1 PARSE-REJECT
}
```

ADR-0157 §Decision is AMENDED in 18.2 IMPL to record this swap. The error wording change (no `_ParseReject_` strings introduced; the structural amendment is a single switch-arm flip).

### 6.5 `buildGRPCCheckFn` — gRPC-mode `checkFn` construction

`buildGRPCCheckFn(gs *core.GrpcService, ctx envoyhttp.FactoryCtx) (checkFn, error)`:

1. **`GoogleGrpc` arm PARSE-REJECT.** If `gs.TargetSpecifier` is `*core.GrpcService_GoogleGrpc_` → `errors.New("ext_authz: grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)")` (matches the parent §4.3 V3-only-transport-discipline framing).
2. **`EnvoyGrpc` arm validation.** `gs.TargetSpecifier.(*core.GrpcService_EnvoyGrpc_).EnvoyGrpc.ClusterName` PGV-mirror (`min_len: 1`). Look up the cluster: `clu, ok := ctx.ClusterManager.Get(clusterName)`. If !ok → `errors.New("ext_authz: grpc_service: unknown cluster "+clusterName)`. If !clu.UseH2() → `errors.New("ext_authz: grpc_service: cluster "+clusterName+" must have http2_protocol_options{} set")`.
3. **Construct the `*grpcclient.AuthClient`.** `dialer := grpcclient.New(ctx.ClusterManager)`; `ac, err := grpcclient.NewAuthClient(dialer, clusterName, durationpbToGo(gs.Timeout))`. `durationpbToGo` is REUSED from `check.go` 18.1.
4. **Compile the listener-level context-extensions baseline.** `gs.InitialMetadata` is DEFERRED for MVP (see §2.6 + §8 item 4). The listener-level context-extensions effective map is empty for MVP; the only contribution is per-route `CheckSettings.context_extensions`.
5. **Return the `checkFn` closure.** Per §2.1 + ADR-0157 §Decision the `compiledConfig` struct is field-final. The gRPC-mode-specific configuration (`includePeerCertificate`/`includeTlsSession`/`encodeRawHeaders`/`packAsBytes` — all derived from `raw *ext_authzv3.ExtAuthz`) is captured DIRECTLY in the closure's lexical scope at `buildGRPCCheckFn` time, NOT promoted to `compiledConfig` struct fields. This preserves the field-final invariant while keeping the gRPC-specific config accessible to the closure.

The `checkFn` signature `(ctx, *authRequest) (checkDisposition, error)` is locked at 18.1 (ADR-0157 §Decision). To keep the closure signature mode-agnostic, **`*authRequest` is extended at 18.2 to carry the per-stream state `buildAttributeContext` needs** beyond the HTTP-mode fields (method/path/headers/body): the source/destination `socket_address` values, the TLS `ConnectionState.ServerName` (when the connection is TLS), the `tls.ConnectionState.PeerCertificates[0]` (when a client cert is presented), and the `DownstreamPrincipal()` slice (already computed at the HCM dispatch via the phase-16 ADR-0144 plumbing).

**Original SPEC step 5 wording (preserved for grep-archaeology):**

> Per §2.1 + ADR-0157 §Decision the `compiledConfig` struct is field-final. The gRPC-mode-specific configuration (`includePeerCertificate`/`includeTlsSession`/`encodeRawHeaders`/`packAsBytes` — all derived from `raw *ext_authzv3.ExtAuthz`) is captured DIRECTLY in the closure's lexical scope at `buildGRPCCheckFn` time, NOT promoted to `compiledConfig` struct fields. This preserves the field-final invariant while keeping the gRPC-specific config accessible to the closure. NO new `DecoderFilterCallbacks` primitive — all this state is captured at `DecodeHeaders` time into the existing `*authRequest` struct, mirroring how 18.1's HTTP-mode closure consumes its `*authRequest` without re-reaching into `dcb`. This is finding §13.5 of this SPEC + the analog of ADR-0144's plumbing discipline (state captured once per stream, threaded through; the filter callbacks expose only `DownstreamPrincipal()`). The 18.2 IMPL's specific field-names on the extended `*authRequest` are settled at PLAN/IMPL (§12 item 3); the SPEC commits to the principle.

**AMENDMENT — phase-18.2 PLAN-time D3 + D12 settle + IMPL Task 4 landing (Amendment date: 2026-05-15; cites ADR-0165).** The "NO new `DecoderFilterCallbacks` primitive" clause is FLIPPED: at PLAN time the planner verified that the SPEC's required populated set for `AttributeContext` (per §15 item 4 + §11.P4 RATIFICATION) is unsatisfiable without callback extension — the master-tip surface (`DownstreamPrincipal()`-only) cannot reach socket addresses, the downstream TLS ServerName / PeerCertDER, the H1-vs-H2 protocol, or the listener-cert principal from inside a per-stream `dcb` callback. **6 new methods land on `DecoderFilterCallbacks` at IMPL Task 4 per ADR-0165** (`DownstreamRemoteAddr`, `DownstreamLocalAddr`, `DownstreamTLSServerName`, `DownstreamTLSPeerCertDER`, `DownstreamProtocol`, `ListenerPrincipal`). The "state captured into the existing `*authRequest`" framing still holds, but the SOURCE of capture is now the 6 new `DecoderFilterCallbacks` accessors (seeded at HCM dispatch via 6 new chain primitives mirroring the ADR-0144 `tlsPrincipals` pattern at `chain.go:107 + 551 + 483`). The ADR-0157 §Decision mode-agnostic closure shape is unchanged — `(ctx, *authRequest)` stays the closure signature. See ADR-0165 §Context for full rationale + cross-phase reuse intent.

The closure body — per-Check invocation:

```
checkFn := func(ctx context.Context, req *authRequest) (checkDisposition, error) {
    attrCtx := buildAttributeContext(req, encodeRawHeaders, packAsBytes, includePeerCertificate, includeTlsSession)
    checkReq := &authv3.CheckRequest{Attributes: attrCtx}
    resp, err := ac.Check(ctx, checkReq)
    if err != nil { return checkDisposition{class: dispError}, err }
    return mapGRPCResponse(resp, validateMutations), nil
}
```

NOTE: `perRouteExtensions` threads into `*authRequest` (already populated at `DecodeHeaders` time from the resolved per-route `*compiledPerRoute`) rather than being captured by the closure — same plumbing discipline. The closure stays purely a function of `(ctx, *authRequest)` per ADR-0157.

### 6.6 `buildAttributeContext` in `attributes.go`

`buildAttributeContext(req *authRequest, encodeRawHeaders bool, packAsBytes bool, includePeerCert bool, includeTlsSession bool) *authv3.AttributeContext`. ALL per-stream state (socket addresses, TLS state, principal slice, per-route context extensions) is read from the extended `*authRequest` — populated once at `DecodeHeaders` time when the per-stream `dcb` is in scope.

**Original SPEC §6.6 wording (preserved for grep-archaeology):**

> The builder is a pure function of `*authRequest` + the four config booleans; NO `DecoderFilterCallbacks` parameter (per §6.5's mode-agnostic-closure invariant + §13.5 no-new-callback-surface).

**AMENDMENT — phase-18.2 PLAN-time D3 + D12 settle + IMPL Task 4 landing (Amendment date: 2026-05-15; cites ADR-0165).** The `buildAttributeContext` function signature itself remains a pure function of `*authRequest` + the 4 config booleans — `DecoderFilterCallbacks` is NOT passed in as a parameter (the §6.5 mode-agnostic-closure invariant is preserved). What changes: the "§13.5 no-new-callback-surface" half of the original justification is FLIPPED — **6 new methods land on `DecoderFilterCallbacks` at IMPL Task 4 per ADR-0165** (`DownstreamRemoteAddr`, `DownstreamLocalAddr`, `DownstreamTLSServerName`, `DownstreamTLSPeerCertDER`, `DownstreamProtocol`, `ListenerPrincipal`). The SOURCE of the per-stream state capture into `*authRequest` is now those 6 callbacks (called at `DecodeHeaders` time from `dispatchOutboundCheck` per Task 8). The §6.6 builder still consumes ONLY the `*authRequest` + the 4 booleans — no callback parameter — so the pure-function shape is preserved. See ADR-0165 §Context + §Decision for the full settle.

1. Build `source`: `Peer{address: socket_address from req.remoteAddr, principal: first-of(req.downstreamPrincipal) or empty}` (per parent §5.P3 + ADR-0144).
2. Build `destination`: `Peer{address: socket_address from req.localAddr, principal: from req.listenerPrincipal — populated automatically from the listener TLS cert SAN/CN per §11.P4 in-session finding}`. `req.listenerPrincipal` is captured at HCM dispatch time alongside the existing ADR-0144 `DownstreamPrincipal()` plumbing (no new framework primitive — the cert info is already accessible from the listener's `*stdtls.Config`; the IMPL plumbs it analogously to ADR-0144's seeding pattern).
3. Build `request.http`: `id` from `x-request-id` header or generated; `method` from `req.method`; `headers` map from `req.headers` (lowercased keys; pseudo-headers included; `x-envoy-auth-partial-body` already injected by 18.1's body path; `x-forwarded-proto` + `x-request-id` already present in the upstream headers per §11.P4 finding); `path` from `req.path`; `host` from `:authority`; `scheme` from `:scheme`; `size` from total body length (post-truncation per `with_request_body`); `protocol` from connection protocol (`HTTP/1.1` / `HTTP/2`); `body` (string) if `!packAsBytes`, else `raw_body` (bytes).
4. Build `request.time`: `timestamppb.Now()` (or `timestamppb.New(req.streamStartTime)` if the IMPL plumbs stream-start-time into `*authRequest`).
5. If `includeTlsSession` AND the connection is TLS: populate `tls_session.sni` from `req.tlsServerName` (the downstream TLS `ConnectionState.ServerName` captured at HCM dispatch). Other `TLSSession` fields (`subjectAltName`, etc.) are NOT populated — only `sni` per the proto's documented populated set + the §11.P4 in-session evidence showing ONLY `sni` populated.
6. If `includePeerCert` AND the downstream presented a client cert: populate `source.certificate` from `req.peerCertDER` (the DER-encoded leaf cert captured at HCM dispatch). For TLS without client cert, leave empty (parent §5.P3 + 18.1's ADR-0144 plumbing already provides the underlying state; `req.peerCertDER` is `nil` in that case).
7. If `encodeRawHeaders`: populate `request.http.header_map` (a `core.HeaderMap` preserving order) instead of `request.http.headers` (a map). For MVP envoy-go honors the toggle but defaults to `headers` (the legacy field) — both Envoy and envoy-go produce IDENTICAL `headers` maps when `encode_raw_headers: false`; whether the IMPL ships `header_map` population is a 18.2 IMPL deferred-decision (see §12 item 6).
8. Set `context_extensions` from `req.perRouteContextExtensions` (the merged per-route map populated at `DecodeHeaders` time per §5).
9. Return.

### 6.7 `mapGRPCResponse` in `check.go`

`mapGRPCResponse(resp *authv3.CheckResponse, validateMutations bool) checkDisposition`:

```
if resp == nil:                          return {class: dispAllow}  // defensive — gRPC OK with nil body
status := resp.GetStatus().GetCode()    // int32, 0 = OK
switch httpResp := resp.HttpResponse.(type) {
case *authv3.CheckResponse_OkResponse:
    // Allow path. Status==0 with OkResponse is the canonical RATIFIED shape per
    // parent §5.P10. Non-zero status with OkResponse is structurally inconsistent
    // (proto-correct but semantically malformed); envoy-go-strict treats it as
    // dispError to surface auth-server bugs rather than silently allowing.
    if status != 0: return {class: dispError, …}
    return buildAllowDispositionGRPC(httpResp.OkResponse, validateMutations)
case *authv3.CheckResponse_DeniedResponse:
    // Deny path. Non-zero status with DeniedResponse is the canonical shape per
    // parent §5.P10. The status==0 case with DeniedResponse is proto-correct but
    // structurally inconsistent; envoy-go-strict treats it as dispError to
    // surface auth-server bugs. (Parent §5.P10 does NOT scrape this combination;
    // envoy-go is strictly safer than v1.37.2's lenient acceptance — documented
    // as a divergence-window in BEHAVIOR_CONTRACT.md §13.4.)
    if status == 0: return {class: dispError, …}
    return buildDenyDispositionGRPC(httpResp.DeniedResponse, status)
case nil:
    // Empty CheckResponse oneof — parent §5.P10 RATIFIED: status==0 → allow
    // (the "empty CheckResponse{} → allow" rule); status!=0 → deny with default 403
    if status == 0: return {class: dispAllow}
    return {class: dispDeny, denyStatus: 403}
}
```

`buildAllowDispositionGRPC`: extract `OkHttpResponse.headers` (`HeaderValueOption[]` with `append_action`) into `upstreamSet` (OVERWRITE_IF_EXISTS_OR_ADD + ADD_IF_ABSENT arms) vs `upstreamApp` (APPEND_IF_EXISTS_OR_ADD arm). The four `append_action` enum values do NOT cleanly group into two buckets — `OVERWRITE_IF_EXISTS` has add-only-if-present semantics distinct from `OVERWRITE_IF_EXISTS_OR_ADD`, and `ADD_IF_ABSENT` differs from both. The IMPL settles the exact 4-arm dispatch table (§12 item 5) consulting the phase-10 header_mutation precedent + the upstream Envoy behavior. Extract `OkHttpResponse.headers_to_remove` into a NEW `upstreamDel []string` field on `checkDisposition` (the cleanest path — avoids overloading set-with-empty-value semantics); the 18.2 IMPL extends `checkDisposition` with `upstreamDel`. Validate via `validateMutationHeaders` if gating set.

`buildDenyDispositionGRPC`: extract `DeniedHttpResponse.status.code` → `denyStatus`; `DeniedHttpResponse.body` → `denyBody`; `DeniedHttpResponse.headers` → `denyHeaders` verbatim (no `allowed_client_headers` filter — per parent §5.P11); validate via `validateMutationHeaders` if gating set; if status.code is zero in DeniedHttpResponse but the outer CheckResponse.status.code is non-zero, default to 403 per parent §5.P10.

This routine is the 23rd fuzzer's primary target (see §7.3).

### 6.8 File layout — extend existing files (no new `grpc_*.go`)

Per the 18.1 SPEC §1.6 framing ("the gRPC-mode `AttributeContext` builder is added in 18.2") and the existing convention that `check.go` houses both modes' check logic + `attributes.go` houses both modes' builders, 18.2 extends both files in place rather than creating `grpc_check.go` + `grpc_attributes.go`. Rationale: the gRPC-mode functions are deeply coupled to existing utilities in those files (`compileStringMatcherList`, `validateMutationHeaders`, `durationpbToGo`, `headerKVToOrderedHeaders`) — physical co-location keeps the surface area tight + matches the 18.1 SPEC's framing. Approximate LoC additions: `check.go` +250–400 LoC (buildGRPCCheckFn + mapGRPCResponse + buildAllowDispositionGRPC + buildDenyDispositionGRPC); `attributes.go` +200–350 LoC (buildAttributeContext + helpers). `internal/grpcclient/` is the NEW directory.

### 6.9 Compile-time invariants

- `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` — unchanged from 18.1.
- `var _ authv3.AuthorizationServer = nil` is NOT asserted (envoy-go is a client; the server interface lives in the test helper).
- `var _ grpc.ClientConnInterface = (*grpcclient.AuthClient)(nil).conn` is NOT asserted (grpcclient is a thin wrapper, not a re-implementation).

---

## 7. Differential fixture `0021-http-ext-authz-grpc`

Per parent SPEC §2 + this SPEC §1. **8 scenarios** — 7 mirroring 0020's HTTP-mode matrix translated to gRPC + 1 gRPC-only `OkHttpResponse` upstream mutation scenario. Three-listener topology mirroring 0020's pattern (a/b/c).

### 7.1 Per-request matrix

| # | Scenario | Listener / Per-route | Auth-server script (gRPC) | Expected disposition | Counter delta assertion |
|---|---|---|---|---|---|
| 1 | gRPC allow | l_test_a / default | `CheckResponse{status:0, OkResponse: OkHttpResponse{}}` | 200 backend echo | `ok=1` |
| 2 | gRPC deny | l_test_a / default | `CheckResponse{status:7, DeniedResponse: DeniedHttpResponse{status:403, body:"access denied", headers:[x-authz-denied-reason]}}` | 403 + body byte-exact + verbatim headers | `denied=1` |
| 3 | error → `status_on_error` | l_test_b / default | server unreachable; `failure_mode_allow:false`; `status_on_error:503` | 503 + empty body | `error=1` |
| 4 | `failure_mode_allow` | l_test_c / default | server unreachable; `failure_mode_allow:true` + `failure_mode_allow_header_add:true` | 200 backend echo + `x-envoy-auth-failure-mode-allowed` arrives upstream | `error=1` + `failure_mode_allowed=1` |
| 5 | `with_request_body` (gRPC) | l_test_a / `/scenario5` | auth inspects `attributes.request.http.body`; allows | 200 backend echo (body materialized via ADR-0128, attached to AttributeContext.request.http.body) | `ok=1` |
| 6 | per-route `disabled` | l_test_a / `/disabled` | (no auth call made) | 200 backend echo | **NO `ext_authz` counter increments** (per parent §6 amendment 7) |
| 7 | per-route `check_settings` w/ `context_extensions` | l_test_a / `/ctx` | per-route `context_extensions: {policy: "scenario7"}`; auth inspects + allows | 200 backend echo; auth received `AttributeContext.context_extensions[policy] == "scenario7"` | `ok=1` (SHARED stats) |
| 8 | `OkHttpResponse` upstream mutation (gRPC-only) | l_test_a / `/scenario8` | `CheckResponse{status:0, OkResponse: OkHttpResponse{headers:[x-injected-by-authz:scenario8 (OVERWRITE), x-also-appended:append1 (APPEND)]}}`; allows | 200 backend echo with the injected header AND the appended header (set + append) visible on the upstream request | `ok=1` |

The 7 scenario differs from 0020's scenario 7 (which exercised `disable_request_body_buffering`) — in 18.2 we exercise the gRPC-only `context_extensions` field instead, because (a) `disable_request_body_buffering` was already covered for HTTP-mode in 0020 and the mode-agnostic body-buffering machinery 18.1 already verified, and (b) `context_extensions` is the gRPC-only `CheckSettings` field whose effect is observable in the auth server's received `CheckRequest`. Both are 5th-canonical-`check_settings` arms — the per-route discipline is exercised identically.

### 7.2 Topology + test-helper

`envoy.yaml` + `envoy-go.yaml` each wire three HCM listeners (l_test_a/b/c) with the ext_authz filter (gRPC-mode) + a router, an echo upstream cluster (REUSES `test/helpers/echobackend/`), and the ext_authz `grpc_service.envoy_grpc.cluster_name: "authz_grpc"` pointing at the auth_grpc cluster — which references a NEW test-helper: an **in-process gRPC `Authorization/Check` server** under `test/helpers/extauthzgrpc/`. The helper is spawned-per-fixture (mirrors `test/helpers/extauthzhttp/` lifecycle). Plaintext h2c for the auth cluster (no TLS-to-auth-cluster fixture-scenario — the §18.P13 closure already RATIFIED TLS plumbing via the in-session SPEC scrape; fixture-harness TLS-to-auth scenarios deferred to a future TLS+auth-cluster integration test if needed). Three listeners separate scenarios 3+4 (distinct `failure_mode_allow` values which are listener-scoped per 18.1 SPEC §10 notable lesson — `CheckSettings` cannot override `failure_mode_allow`).

**Known testing gap (listener-side TLS).** `include_tls_session.sni` and `include_peer_certificate.source.certificate` populations + the `destination.principal` automatic-from-listener-cert finding are NOT exercised by fixture 0021 — the fixture uses a plaintext listener to keep the scenario matrix focused. The §11.P4 in-session SPEC scrape (this document) RATIFIED the SNI population behaviorally against reference Envoy v1.37.2, so the wire-shape is captured. Behavioral verification of envoy-go's own AttributeContext output for these fields lives in **unit tests against a mocked TLS state in `*authRequest`** (Group 12 per §14.1) — the unit test fakes a populated `req.tlsServerName` / `req.peerCertDER` / `req.listenerPrincipal` and asserts `buildAttributeContext`'s output. The acceptance §15 item 4 verification path is unit-test-only for the gate-conditional fields, NOT differential. A future TLS-listener-extension fixture (a follow-up integration test) could close the differential gap if a behavior delta surfaces; the current scope DEFERS this per the cost-vs-coverage tradeoff (the §11.P4 in-session scrape is the load-bearing empirical evidence).

The driver in `inputs/driver.go` exercises the 8 scenarios; the harness asserts response status + body byte-equivalence on allow AND deny paths, `/stats/prometheus` counter-delta equivalence on the 5 reachable counters, backend-arrival header assertions (allow-path upstream injection + `OkHttpResponse` mutation), and auth-server received-`CheckRequest`-content assertions (per-scenario discriminator + `context_extensions` for scenario 7).

### 7.3 23rd fuzzer — `FuzzCheckResponseMapping`

NEW `FuzzCheckResponseMapping` at `internal/filter/http/extauthz/fuzz_test.go` (alongside the 22nd `FuzzExtAuthzConfigParse`). Fuzz signature:

```go
func FuzzCheckResponseMapping(f *testing.F) {
    f.Add([]byte{0x00}) // empty proto
    // ... seed with valid CheckResponse encodings: OK+OkResponse{}, OK+DeniedResponse, non-OK+OkResponse, etc.
    f.Fuzz(func(t *testing.T, data []byte) {
        var resp authv3.CheckResponse
        if err := proto.Unmarshal(data, &resp); err != nil { return }
        disp := mapGRPCResponse(&resp, /*validateMutations=*/true)
        // invariants: disp.class is one of {dispAllow, dispDeny, dispError, dispInvalid}
        switch disp.class {
        case dispAllow, dispDeny, dispError, dispInvalid: // OK
        default: t.Fatalf("invalid class %v", disp.class)
        }
        // additional invariants on deny: denyStatus in [100, 599]; denyBody is a copy not aliased; denyHeaders pass validation
    })
}
```

Corpus seeds: 6–10 encoded `CheckResponse` variants covering OkResponse with/without header mutations, DeniedResponse with various status codes (zero, 401, 403, 500, 999), empty oneof + various status codes, oversized header values, invalid status codes (e.g., 1000+), and the malformed cases (status==0 with DeniedResponse, status!=0 with OkResponse). 30s/seed under ADR-0018 budget.

The existing `FuzzExtAuthzConfigParse` corpus extends to include `grpc_service` config variants (now that the arm is no longer PARSE-REJECT in 18.2) — this is NOT a new fuzzer, just corpus growth on the 22nd fuzzer. The 23rd fuzzer count comes from `FuzzCheckResponseMapping`.

### 7.4 `test/helpers/extauthzgrpc/` package

The FIRST in-process gRPC server in envoy-go's test tree. Surface:

```go
package extauthzgrpc

// Server is an in-process envoy.service.auth.v3.Authorization/Check server.
// Spawn-per-fixture lifecycle. Plaintext h2c on a randomly-bound port.
type Server struct {
    addr string
    grpcSrv *grpc.Server
    scripts map[string]*authv3.CheckResponse // keyed by discriminator
    mu sync.RWMutex
}

// New starts a Server bound to 127.0.0.1:0 (ephemeral). Returns the started
// Server. Use Addr() to read back the bound host:port.
func New(t testing.TB) *Server

// Addr returns the host:port the Server is bound to.
func (s *Server) Addr() string

// Script registers a CheckResponse for the given discriminator. The fixture
// driver controls how the auth server picks the response per request — for
// 0021 the discriminator is the `:path` value (so each fixture URL routes
// to its own scripted CheckResponse).
func (s *Server) Script(discriminator string, resp *authv3.CheckResponse)

// Stop graceful-shutdowns the gRPC server.
func (s *Server) Stop()
```

The discriminator is the request's `:path` extracted from `req.Attributes.Request.Http.Path` (so scenario URLs `/scenario1`, `/scenario2`, … each get their own scripted response). Per-fixture cleanup via `t.Cleanup(s.Stop)`. ~150–220 LoC production + ~100 LoC tests.

---

## 8. Deferred items (18.2 slice; per parent SPEC §4.4 + §8 + 18.1 SPEC §8 carry-forward)

For future-phase consideration (none are blockers for closing rows 18.2 + 18; all auditable in the ADR-0040 deferral trail). 18.2 carries forward all 11 items 18.1 deferred + adds the gRPC-specific deferrals:

1. **18.1 carry-forwards (unchanged):** `*metadata_context_namespaces` (4 fields), `filter_enabled` family (3 fields), `enable_dynamic_metadata_ingestion`, `filter_metadata`, `charge_cluster_response_stats` + cluster-scoped stat triple, `emit_filter_state_stats`, `bootstrap_metadata_labels_key`, `decoder_header_mutation_rules`, `allowed_client_headers_on_success`, `response_code_details` emission, access-log integration. The `disabled` counter remains STRUCTURALLY UNREACHABLE.
2. **gRPC `core.GrpcService.initial_metadata`** — SILENT-IGNORED. This is a `[]HeaderValue` of metadata-pairs to send on every Check call (gRPC-mode equivalent of a request-header bundle). MVP does NOT thread these into the per-Check context.
3. **gRPC `core.GrpcService.retry_policy`** — SILENT-IGNORED. gRPC client retry is a follow-up; the current MVP single-attempt-then-error matches 18.1's `httpAuthClient` zero-retry discipline + the parent §5.P10 error-classification boundary.
4. **gRPC `core.GrpcService_GoogleGrpc`** — **envoy-go-strict exclusion** (PARSE-REJECT per §6.5 step 1 + parent §4.3 + ADR-0008 V3-only-transport-discipline). NOT a deferral — listed here for surface-completeness; §9's tally treats it correctly. envoy-go uses `google.golang.org/grpc` directly; the `GoogleGrpc` arm (which configures a self-contained gRPC client with its own dial/TLS/retry semantics) is permanently out-of-scope for envoy-go.
5. **`OkHttpResponse.response_headers_to_add`** — DEFERRED (decode-side-only; would inject headers into the downstream RESPONSE on allow — same family as `allowed_client_headers_on_success`). Documented as a divergence-window joint with §13.4 phase-18.1 forward-pointer notes carry-forward.
6. **`OkHttpResponse.query_parameters_to_set` / `query_parameters_to_remove`** — DEFERRED (path-query subsystem ADR-0112; joint with HTTP-mode's analogous deferrals).
7. **`OkHttpResponse.dynamic_metadata`** + **`CheckResponse.dynamic_metadata`** — DEFERRED (dynamic-metadata family — joint with 18.1 + 17 + 16's same-family deferrals).
8. **`encode_raw_headers: true` (the `header_map` arm)** — CONDITIONALLY DEFERRED. The flag PARSES; whether the `header_map` field is populated when set true is a 18.2 IMPL decision per §12 item 6. Default (`false`) populates the legacy `headers` map — byte-equivalent to reference Envoy at the AttributeContext level.
9. **xDS-CDS-driven auth-cluster reconfig** — DEFERRED (envoy-go has no xDS-CDS yet; static config only). The `*grpcclient.AuthClient` lifecycle is tied to the static `compiledConfig`; when xDS-CDS lands, hot-replacement gains a close-on-replacement discipline.
10. **TLS-fronted gRPC auth cluster fixture coverage** — the §18.P13 in-session SPEC scrape (§11) RATIFIED the TLS-to-auth-cluster path; fixture 0021 uses plaintext h2c for the auth cluster to keep the fixture topology simple. A future integration test MAY extend coverage if a behavior gap surfaces.

---

## 9. Cross-references against phase-18.1 + phase-17 deferred-items lists — forward-pointer pickup

- **18.1 item 1 — gRPC service mode:** **CLOSED.** This is the explicit deliverable of 18.2.
- **18.1 item 8 — `CheckSettings.context_extensions` HTTP-mode no-op:** **CLOSED** for gRPC mode — `context_extensions` is now consumed proto-faithful in 18.2 (the listener+route merged map populates `AttributeContext.context_extensions`). The HTTP-mode "no-op" framing remains accurate for the HTTP service mode (the field has no HTTP-mode effect by proto design); the consumption in gRPC mode closes the joint deferral.
- **18.1 items 2, 3, 4, 5, 6, 7, 9, 10, 11:** NO PICKUP (carry-forward).
- **Phase-17 items 3, 4, 9 + 1-2 + 5-8 + 10-12:** NO PICKUP (jwt_authn-specific concerns).
- **Phase-16 items:** NO PICKUP.

**Forward-pointer net change for phase 18.2**: **2 closures** (item 1 + item 8 from 18.1's list — the gRPC sequenced split closes; `context_extensions` consumption closes for gRPC mode). 18.2 adds ~5 new gRPC-specific deferred items (§8 items 2/3/5/6/7/8 — but item 4 GoogleGrpc PARSE-REJECT is not a deferral; it's an envoy-go-strict exclusion). Net new deferred items: ~5; net closures: 2; net deferred-cluster delta: +3 vs 18.1.

---

## 10. ADR anchor map (18.2 subset; full 9-ADR map in parent SPEC §7)

The 18.2-landing ADRs. Per the ADR-0044 ADR-on-impl convention: ADR-0158 §Context was anchored at the parent SPEC commit `308e9b6`; §Decision + §Consequences LAND at the Lands-in-Task per ADR-0044. ADR-0160 + ADR-0161 gRPC-mode portions LAND at 18.2 IMPL. ADR-0157 §Decision is AMENDED at 18.2 IMPL.

| ADR | Subject (18.2 portion) | Lands-in-Task (hypothesis) |
|---|---|---|
| **ADR-0158** | `internal/grpcclient/` framework primitive — `Dialer` (cluster-name → `*grpc.ClientConn`) + `AuthClient` (typed `Authorization/Check` wrapper); cluster-manager coupling via `grpc.WithContextDialer(cluster.Dial)`; cross-phase-reusable for ext_proc + global_ratelimit | Task 3 |
| **ADR-0157 §Decision AMENDMENT** | `*ExtAuthz_GrpcService` switch-arm activation: replace the 18.1 PARSE-REJECT with `buildGRPCCheckFn`; `core.GrpcService.GoogleGrpc` arm PARSE-REJECT envoy-go-strict; `core.GrpcService.{initial_metadata, retry_policy}` SILENT-IGNORED | Task 4 |
| **ADR-0160** (gRPC-mode portion) | `buildAttributeContext` in `attributes.go` — source/destination `Peer` (incl. `principal` via ADR-0144); `request.http` per parent §5.P4 + §11.P4 in-session refinement; `request.time` as `Timestamp`; `tls_session.sni` gated by `include_tls_session`; `source.certificate` gated by `include_peer_certificate`; `destination.principal` populated automatically (NOT gated; §11.P4 in-session finding); `context_extensions` merged listener+per-route; `encode_raw_headers` `headers`-vs-`header_map` discipline | Task 5 |
| **ADR-0161** (gRPC-mode portion) | `mapGRPCResponse` + `OkHttpResponse`/`DeniedHttpResponse` extraction in `check.go`; verbatim deny-header pass-through (UNLIKE HTTP-mode's matcher filter); allow-path `headers` (set/append per `append_action`) + `headers_to_remove`; the `response_headers_to_add` deferral; `validate_mutations` gating in both directions | Task 6 |

**ADR-0044 escape-valve** held in reserve for ~0–1 impl-time-unanticipated ADRs. The §11.P13 in-session closure at SPEC time REMOVES the most-likely escape-valve surface (the cluster-manager coupling for `EnvoyGrpc` cluster-name resolution — confirmed RATIFIED with no new TLS-layer lift needed). Other possible surfaces: (i) a per-Check `context.WithTimeout` interaction with the async-resume goroutine; (ii) graceful-shutdown of `*grpc.ClientConn` if the IMPL chooses to wire it (currently MVP skips it). NEITHER appears load-bearing. **NO ADR-0125 amendment.**

**Next-free ADR after phase 18.2** = ADR-0165 (no new ADR numbers are anticipated to land in 18.2; ADR-0158 was reserved at the parent SPEC commit; ADR-0157/0160/0161 are amendments to existing).

---

## 11. Empirical-pin block — §18.P4 `tls_session` + §18.P13 closures (in-session SPEC scrape, 2026-05-15)

> **§11 carries the SPEC-time closure of the two parent-§5 pins that were RATIFIED-PENDING after the phase-18 parent SPEC scrape on 2026-05-14.** Per the user's pin-closure pick at SPEC-session entry (in-session settle), this section replaces the would-be IMPL-time fixture-harness closure with a live SPEC-time scrape against reference Envoy v1.37.2.

**Probe date:** 2026-05-15. **Reference Envoy:** `envoyproxy/envoy:v1.37.2` at the `ENVOY_TARGET.md` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. **Auth server:** a Go gRPC `Authorization/Check` server (~75 LoC) using `google.golang.org/grpc` v1.81.1 + go-control-plane v1.32.4's `service/auth/v3` bindings, fronted with TLS (CA + leaf cert chain generated for the scrape, SAN = `host.docker.internal`). **Envoy config:** a TLS-fronted downstream listener (SAN = `downstream.scrape.test`) wiring ext_authz gRPC-mode with `include_peer_certificate: true` + `include_tls_session: true` + `with_request_body: {max_request_bytes: 1024, allow_partial_message: true}` pointing at a TLS-fronted gRPC auth cluster (`EnvoyGrpc`-cluster-reference; `http2_protocol_options: {}` set; `transport_socket: UpstreamTlsContext` with `trusted_ca` + `match_typed_subject_alt_names`). **Drive:** `curl -X POST -d 'hello-from-scrape' https://downstream.scrape.test:10443/scrape-test-path` (via `--resolve` + `--cacert`).

### 11.1 §18.P4 closure — `tls_session.sni` population (RATIFIED, in-session)

**Observation.** The logged `CheckRequest.attributes.tls_session` populated set is **`{sni: "downstream.scrape.test"}`** — ONLY the `sni` field, populated with the exact SNI value sent by the curl client. No other `TLSSession` fields appear in the JSON (the protojson `EmitUnpopulated: false` rendering omits empty fields).

**Disposition: RATIFIED.** Parent §5.P4 RATIFIED-AND-REFINED already held for the gRPC `AttributeContext` populated set in general; the `tls_session` portion was explicitly **RATIFIED-PENDING-IMPL-TIME** because the parent SPEC scrape used a plaintext listener. The 18.2 in-session scrape closes the pin: `tls_session.sni` populates from the downstream TLS `ConnectionState.ServerName`; no other `TLSSession` fields are populated. The `include_tls_session: true` gate semantics from parent §5.P3 are RATIFIED (the populated-set observed only with the gate true).

**Verbatim evidence (from `/tmp/scrape-18.2/checkrequest-evidence.log`):**

```json
{
  "attributes": {
    "destination": {
      "address": { "socketAddress": {"address":"172.17.0.2","portValue":10443}},
      "principal": "downstream.scrape.test"
    },
    "source": {
      "address": {"socketAddress": {"address":"172.17.0.1","portValue":58476}}
    },
    "request": {
      "http": {
        "id": "9941000723061342897",
        "method": "POST", "path": "/scrape-test-path",
        "host": "downstream.scrape.test:10443",
        "scheme": "https", "protocol": "HTTP/1.1", "size": "17",
        "body": "hello-from-scrape",
        "headers": {
          ":authority": "downstream.scrape.test:10443",
          ":method": "POST", ":path": "/scrape-test-path", ":scheme": "https",
          "accept": "*/*", "content-length": "17",
          "content-type": "application/x-www-form-urlencoded",
          "user-agent": "curl/8.5.0",
          "x-envoy-auth-partial-body": "false",
          "x-forwarded-proto": "https",
          "x-request-id": "eed3400a-64fc-450b-9315-d30a080f244e"
        }
      },
      "time": "2026-05-15T09:38:18.351477Z"
    },
    "metadataContext": {},
    "routeMetadataContext": {},
    "tlsSession": { "sni": "downstream.scrape.test" }
  }
}
```

**Additional findings from the same probe (refining parent §5.P4 + §5.P3):**

- **`destination.principal` populates AUTOMATICALLY from the listener TLS cert** (the leaf cert's CN/SAN — here `downstream.scrape.test`). **NOT gated by `include_peer_certificate`** (which gates the `source.certificate` field — the downstream client cert; here unset because curl didn't present a client cert). This refines parent §5.P3's framing: `include_peer_certificate` is the **client** cert gate; `destination.principal` is a separate populated-automatically field reflecting the **listener** cert identity. §11.P4 records this refinement; ADR-0160 gRPC-mode portion's §Decision body codifies (Task 5).
- **`source.principal` is NOT set** because no client cert was presented. ADR-0144 `DownstreamPrincipal()` returns an empty slice in this case; the gRPC `Peer.principal` is `""`. This matches parent §5.P3 + ADR-0144 §Decision (case (c) plaintext → empty).
- **`request.time` renders as RFC3339 string in protojson** (`2026-05-15T09:38:18.351477Z`) but the underlying proto type is still `Timestamp{seconds, nanos}` (per parent §5.P4 RATIFIED-AND-REFINED + the proto definition). The JSON rendering is a protojson canonicalization, not a proto-shape change — ADR-0160 gRPC-mode portion's IMPL constructs `Timestamp` via `timestamppb.Now()`.
- **HCM injects `x-forwarded-proto: https` + `x-request-id: <uuid>` + `x-envoy-auth-partial-body: false` into the `request.http.headers` map** — these are HCM-injected headers visible to ext_authz, not ext_authz-specific. Parent §5.P4 noted `x-envoy-auth-partial-body`; the in-session scrape extends the finding to the two `x-forwarded-*` + `x-request-id` headers. The 18.2 `buildAttributeContext` does NOT inject these — HCM has already added them by the time ext_authz runs at decode time.
- **`metadataContext` + `routeMetadataContext` render as empty objects `{}`** even with no fields set (a protojson default for messages). This confirms the deferred-no-effect MVP framing (§8 item 1 carry-forward; ADR-0160 gRPC-mode portion populates them as empty messages, not nil).
- **`request.http.headers` is a map (legacy field)**; `header_map` (the ordered alternative gated by `encode_raw_headers`) is not populated because `encode_raw_headers` defaulted to false. §8 item 8 records the conditional deferral.

### 11.2 §18.P13 closure — gRPC dial + TLS-to-auth-cluster plumbing (RATIFIED, in-session)

**Observation.** Envoy successfully dialed the TLS-fronted gRPC auth cluster (TLS handshake via `transport_socket: UpstreamTlsContext` with `trusted_ca: ca.crt` + `match_typed_subject_alt_names: [DNS: host.docker.internal]`), completed the `Authorization/Check` unary RPC round-trip, and forwarded the request upstream after receiving `CheckResponse{status: OK}`. The curl client saw `HTTP/1.1 200 OK` + the direct_response body `ok\n`. The auth-cluster configuration:

```yaml
- name: authz_grpc
  type: STRICT_DNS
  dns_lookup_family: V4_ONLY
  typed_extension_protocol_options:
    envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
      "@type": ...HttpProtocolOptions
      explicit_http_config:
        http2_protocol_options: {}
  load_assignment: { ... endpoint: host.docker.internal:9443 }
  transport_socket:
    name: envoy.transport_sockets.tls
    typed_config:
      "@type": ...UpstreamTlsContext
      sni: host.docker.internal
      common_tls_context:
        validation_context:
          trusted_ca: { filename: /certs/ca.crt }
          match_typed_subject_alt_names: [{ san_type: DNS, matcher: { exact: host.docker.internal }}]
```

**Disposition: RATIFIED.** Parent §5.P13 was RATIFIED-PENDING-IMPL-TIME because the parent scrape used a plaintext h2c auth cluster; the cluster-reference → cluster-manager coupling + TLS-to-auth-cluster path were not behaviorally exercised. The 18.2 in-session scrape closes the pin: Envoy resolves `EnvoyGrpc.cluster_name → endpoint + transport_socket` via the standard cluster-manager pipeline (no ext_authz-specific path), dials over TLS via the upstream TLS transport socket, multiplexes the gRPC framing on top via h2 (`http2_protocol_options: {}` required for gRPC framing — confirmed: without this the dial would emit `Upstream HTTP/1.1 request to gRPC server` and fail).

**envoy-go-side implications (informing ADR-0158 §Decision + Task 3 IMPL):**

- The `internal/grpcclient/Dialer` couples to `internal/cluster.Manager` for cluster-name resolution. The cluster manager already owns endpoint selection (`Cluster.PickEndpoint`) + TLS termination (`Cluster.Dial(ctx)` returns a TLS-wrapped `net.Conn` for TLS clusters per the existing cluster.go).
- `grpc.WithContextDialer(cluster.Dial)` is the integration point. gRPC layers framing on top of the dialer-returned `net.Conn` without participating in TLS — the cluster manager owns it. `grpc.WithTransportCredentials(insecure.NewCredentials())` is used because TLS is NOT handled by gRPC's transport credentials layer (the `net.Conn` is already TLS-wrapped if applicable). The §11.P13 in-session scrape RATIFIES this design indirectly: Envoy's own implementation behaves analogously (gRPC framing layered on top of the upstream transport socket).
- The auth cluster MUST have `UseH2() == true` (set via `http2_protocol_options: {}`). PARSE-REJECT if the operator misconfigures (a config-time guard in `buildGRPCCheckFn` per §6.5).
- envoy-go inherits the cluster manager's existing TLS-validation discipline (the `parsedTLS *stdtls.Config` field on `*Cluster` per `internal/cluster/cluster.go`). No new TLS-layer lift is needed — REMOVES the most-likely ADR-0044 escape-valve surface.

**§18.P13 RATIFIED at SPEC time** — the 18.2 IMPL has zero TLS-plumbing escape-valve risk.

### 11.3 Reference to parent §5 for the other 11 pins

The parent SPEC §5 carries the IN-SESSION RATIFICATIONS for §18.P1 / §18.P2 / §18.P3 / §18.P5 / §18.P6 / §18.P7 / §18.P8 / §18.P9 / §18.P10 / §18.P11 / §18.P12 (11 of 13 pins). Of these, three were RATIFIED-PENDING-IMPL-TIME at the parent SPEC commit and have since CLOSED RATIFIED at 18.1 IMPL: §18.P6 (Task 8), §18.P7 (Task 8), §18.P11 (Task 13). After the 18.2 in-session SPEC scrape closes §18.P4 `tls_session` + §18.P13, **all 13 parent-§5 pins are CLOSED RATIFIED at the 18.2 SPEC commit.**

---

## 12. Deferred decisions (the planner / implementer settles these)

1. **`test/helpers/extauthzgrpc/` exact API** — script discriminator (the §7.4 sketch proposes `:path` keyed; the IMPL may choose another key like the gRPC `:authority` or a `context_extensions` value if more flexible).
2. **`*grpc.ClientConn` close-on-process-exit discipline** — whether to register an `os.Exit`-time cleanup hook in the package, or just let GC handle it for MVP. Per §3.1 + ADR-0158, MVP leaks-on-exit; the IMPL may add a hook if cheap.
3. **`*authRequest` vs new state-bag for gRPC-mode per-stream state** — §6.5 step 5's note about whether to extend `authRequest` with TLS-state fields (so the `checkFn` signature stays `(ctx, *authRequest)`) OR to introduce a per-stream context bag. The cleanest path is extending `authRequest` (no new types); the IMPL settles.
4. **`grpc.NewClient` resolver target string** — `passthrough:///` vs `dns:///` (the §6.5 step 3 proposal uses `passthrough:///` since the cluster-manager owns name resolution; the IMPL may prefer a custom resolver scheme if it cleans up logging). The behavior is functionally identical.
5. **`*core.HeaderValueOption` append_action enum mapping** — `OkHttpResponse.headers` carry an `append_action` enum (`APPEND_IF_EXISTS_OR_ADD`, `OVERWRITE_IF_EXISTS_OR_ADD`, `OVERWRITE_IF_EXISTS`, `ADD_IF_ABSENT`). The §6.7 sketch groups OVERWRITE arms as set and APPEND_IF_EXISTS_OR_ADD as append; ADD_IF_ABSENT and OVERWRITE_IF_EXISTS need treatment. The IMPL settles per the phase-10 header_mutation enum-handling precedent.
6. **`encode_raw_headers` `header_map` arm activation** — whether 18.2 IMPL populates `request.http.header_map` when the flag is true, or DEFERRED (the legacy `headers` map suffices for fixture 0021's byte-equivalence checks per the §11.P4 in-session scrape — the harness compares the auth-server's received-CheckRequest against expectations, and reference Envoy populates `headers` by default). The IMPL settles per cost.
7. **gRPC `Unimplemented` / `NotFound` etc. statuses on the `Status` proto** — parent §5.P10 mapped `any non-zero status → deny`. Whether to distinguish gRPC-transport errors (Unavailable, DeadlineExceeded) from `CheckResponse.status.code` non-zero values is the IMPL's call — the §6.7 sketch separates them (transport error in `ac.Check`'s error return → dispError; CheckResponse with non-zero status.code → dispDeny). Confirm against reference Envoy at IMPL.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052; lands at 18.2 phase-done)

1. **§13.1 — `## HTTP filter chain` → `### envoy.filters.http.ext_authz` subsection.** Flip the 18.1-anchored "gRPC mode — see phase 18.2" forward-pointers to ACTUAL gRPC content. Specifically: (a) `services` oneof — `grpc_service` arm is now CONSUMED, dispatch to `buildGRPCCheckFn`; (b) NEW subsection on the gRPC-mode `AttributeContext` populated set referencing §11.P4 in-session findings (incl. the `destination.principal` automatic population + HCM-injected headers refinement); (c) NEW subsection on the gRPC-mode allow-path `OkHttpResponse` mutation + deny-path `DeniedHttpResponse` verbatim header pass-through (CONTRAST with HTTP-mode's matcher-filtered headers); (d) the `core.GrpcService.GoogleGrpc` PARSE-REJECT note; (e) the `core.GrpcService.{initial_metadata, retry_policy}` SILENT-IGNORED note.
2. **§13.2 — `## Stat-name mapping` → 77-name table UNCHANGED.** ext_authz's 6 counters are mode-agnostic; no new rows.
3. **§13.3 — `## Equivalence Matrix` → NEW row for fixture `0021-http-ext-authz-grpc`** with byte-exact body + status + verbatim deny-headers + AttributeContext-content assertion + counter-delta equivalence.
4. **§13.4 — NEW `### Phase 18.2 forward-pointer notes` subsection** covering the §8 deferral list (12 items — 18.1 carry-forwards + new gRPC-specific). 18.2 closes 2 phase-18.1 forward-pointers (gRPC arm activation + `context_extensions` consumption).
5. **§13.5 — `## HTTPFilterCallbacks` — original SPEC wording (preserved for grep-archaeology):**

   > 18.2 reuses ADR-0144 `DownstreamPrincipal()` as the sole TLS-aware decoder-callback method; ALL other per-stream state needed by `buildAttributeContext` (`socket_address` source/destination, TLS `ServerName`, peer cert DER, listener-cert-derived principal, stream-start-time, per-route context-extensions) is captured at `DecodeHeaders` time into the extended `*authRequest` struct (per §6.5–§6.6) — mirroring ADR-0144's seed-at-HCM-dispatch plumbing pattern. NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2; the SPEC author verified that the existing callback surface at master tip (`internal/filter/http/callbacks.go`) exposes only `DownstreamPrincipal() []string` for TLS-derived state and the IMPL must NOT extend it.

   **AMENDMENT — phase-18.2 PLAN-time D3 + D12 settle + IMPL Task 4 landing (Amendment date: 2026-05-15; cites ADR-0165).** The hard constraint above is FLIPPED at phase-18.2 PLAN time per planner-time decisions D3 + D12. At PLAN time the planner re-verified the master-tip callback surface against the SPEC's own §15 acceptance item 4 + §11.P4 RATIFICATION (populated `tls_session.sni`, `source.certificate`, source + destination socket addresses, `destination.principal`, `request.http.protocol`) and determined the populated set is UNSATISFIABLE without callback extension — the existing `DownstreamPrincipal()` alone covers only the client-cert principal candidates. The PLAN settled by AMENDING this §13.5 + §6.5 step 5 + §6.6 in-place at Task 4, and **6 new methods land on `DecoderFilterCallbacks` at IMPL Task 4 per ADR-0165** (cross-phase-reusable callback-surface extension): `DownstreamRemoteAddr() net.Addr`, `DownstreamLocalAddr() net.Addr`, `DownstreamTLSServerName() string`, `DownstreamTLSPeerCertDER() []byte`, `DownstreamProtocol() string`, `ListenerPrincipal() string`. The 6 methods anchor the cross-phase-reusable framework primitive (consumed by ext_proc + global_ratelimit + future ext_authz extensions) and the ADR-0044 escape-valve fires per ADR-0165 §Context. The "per-stream state captured into the extended `*authRequest`" framing of §6.5–§6.6 still holds — but the SOURCE of that capture is now the 6 new `DecoderFilterCallbacks` accessors (seeded at HCM dispatch via 6 new chain primitives mirroring the ADR-0144 `tlsPrincipals`/`SetTLSPrincipals`/`DownstreamPrincipal()` pattern). The behaviorally significant divergence vs reference Envoy that the original §13.5 would have produced (UNPOPULATED `tls_session.sni` + `source.certificate` + `destination.principal` + socket addresses + `request.http.protocol`) is avoided. See ADR-0165 §Context for the falsified-at-PLAN-time analysis + cross-phase reuse rationale.
6. **§13.6 — `## Per-route canonical patterns cross-reference` — NO new entry; the 5th-canonical reuse note already covers gRPC mode** (ADR-0163 records the explicit no-amendment 5th-canonical-REUSE decision).
7. **NEW top-level `## gRPC client framework primitive (per phase 18.2 ADR-0158)` umbrella** — analogous to phase-17 `## JWKS framework primitive` umbrella. Documents: the `Dialer` API + connection-lifecycle (per-cluster cached `*grpc.ClientConn` via `grpc.WithContextDialer(cluster.Dial)` — TLS terminates at the cluster-manager layer, not gRPC's transport credentials); the `AuthClient` typed wrapper; cross-phase reuse intent for ext_proc + global_ratelimit; the §11.P13 in-session RATIFICATION note (no new TLS-layer lift).
8. **Stat-table headnote unchanged at 77 names** + a clarification note that 6 of the 11 ext_authz counters carry over from 18.1 unchanged and 0 new are added at 18.2.

---

## 14. Testing strategy

### 14.1 Unit tests

- `internal/grpcclient/` — Group 1: `Dialer.DialContext` happy path (plaintext cluster); PARSE-REJECT for unknown cluster; PARSE-REJECT for `UseH2: false` cluster; TLS-cluster dialing (against a TLS test-helper). Group 2: `AuthClient.Check` happy path + timeout propagation + context-cancel propagation. Group 3: `AuthClient.Close` idempotency.
- `internal/filter/http/extauthz/extauthz_test.go` extensions — Group 10 (NEW): `buildGRPCCheckFn` parse-time validation (cluster not found, UseH2 false, GoogleGrpc PARSE-REJECT). Group 11 (NEW): `mapGRPCResponse` mapping — OK+OkResponse → allow; non-zero status+DeniedResponse → deny verbatim headers; empty CheckResponse → allow; malformed (status==0 + DeniedResponse) → error; OkResponse with header mutations → upstreamSet/upstreamApp population; `validate_mutations` rejection. Group 12 (NEW): `buildAttributeContext` — populated set (request.http fields, request.time, source/destination peers, principal via ADR-0144); gates (`include_*` true vs false; tls_session population conditional on TLS connection); context_extensions merge (listener+per-route).
- Existing Group 1–9 from 18.1 are UNCHANGED (the mode-agnostic behavior is unchanged).

### 14.2 Race detector + lint

`go test -race ./internal/grpcclient/... ./internal/filter/http/extauthz/...` + repo-wide race clean. The gRPC client + per-stream cancellation path is a likely race-detector exercise surface. Specific concerns the race tests must cover:

- **`OnDestroy`-driven cancel during in-flight `AuthClient.Check`.** Same primitive 18.1 ratified for HTTP-mode: `mu`/`done` guard + `context.WithCancel`; the per-Check context threads into `*grpc.ClientConn.Invoke` which honors `ctx.Done()` and returns promptly. The 18.2 IMPL adds `TestOnDestroy_CancelsInFlightGRPCCheck` parallel to 18.1's HTTP-mode equivalent.
- **`*grpc.ClientConn` shared across goroutines.** `grpc.ClientConn` is documented goroutine-safe; concurrent `Check` calls from multiple per-stream goroutines on the same `*AuthClient` exercise the gRPC library's internal stream multiplexing. The race tests run concurrent fixture-driven Check calls.
- **`*grpc.ClientConn` lifecycle vs. process-exit.** MVP leaks-on-exit per §12 item 2; no explicit `Close()` call. The race-detector tests must NOT call `Close()` either (matches production behavior). A future hot-reload phase will land a close-on-replacement test.
- **`OnDestroy`-after-Check-completed.** Same race surface 18.1's `mu`/`done` guard already protects — the 18.2 IMPL re-exercises the guard against the gRPC-mode closure to confirm no behavioral delta.

### 14.3 Fuzzer

23rd fuzzer `FuzzCheckResponseMapping` per §7.3. Existing 22 fuzzers re-run clean. Existing `FuzzExtAuthzConfigParse` corpus extends with `grpc_service` config variants (config-parse path automatically; not a new fuzzer).

### 14.4 h2spec + differential

h2spec 53/53 PASS at the ADR-0051 pin (NO H2 wire-shape change between 18.1 and 18.2; ext_authz uses gRPC over H2 to the upstream auth cluster, not to the downstream client). 22 differential fixtures green at 18.2 phase-done (0000–0021; 0021 NEW; 0000–0020 carry-forward).

### 14.5 Six-gate checklist (A/B/C/D/E/F per BOOTSTRAP_PROMPT.md §7.5)

- **Gate A** (build + vet + lint): green; new `internal/grpcclient/` package compiles clean; `internal/filter/http/extauthz/` recompiles clean with the gRPC-mode additions.
- **Gate B** (race tests): green; `go test -race ./internal/grpcclient/... ./internal/filter/http/extauthz/...` + repo-wide.
- **Gate C** (h2spec): 53/53 PASS at the ADR-0051 pin.
- **Gate D** (fuzzers): 23 fuzzers green at 30s each.
- **Gate E** (differential): 22/22 fixtures green (0000–0021).
- **Gate F** (BEHAVIOR_CONTRACT): the §13 edit bundle landed; `tools/check_behavior_contract.sh` (or analog) green.

---

## 15. Acceptance checklist (for the reviewer)

The 18.2 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following against the landed artefacts:

1. **`internal/grpcclient/` package per ADR-0158:** `internal/grpcclient/{grpcclient.go, dialer.go OR analog, doc.go, *_test.go}` landed; `Dialer` + `AuthClient` types per §3.1 + §6 public surface; `grpc.WithContextDialer(cluster.Dial)` + `WithTransportCredentials(insecure.NewCredentials())` integration; PARSE-REJECT for unknown cluster + `UseH2()==false`; cross-phase-reuse forward-pointer for ext_proc + global_ratelimit in ADR-0158 §Decision.
2. **`grpc_service` arm activation per ADR-0157 §Decision AMENDMENT:** `*ExtAuthz_GrpcService` switch-arm in `buildCompiledConfig` calls `buildGRPCCheckFn` (NOT PARSE-REJECT); ADR-0157 §Decision amended at the 18.2 IMPL anchor; `core.GrpcService.GoogleGrpc` arm PARSE-REJECTs envoy-go-strict; `initial_metadata` + `retry_policy` SILENT-IGNORED.
3. **`buildGRPCCheckFn` per §6.5:** cluster-manager lookup + `UseH2()` gate; `*grpcclient.AuthClient` constructed once at config-load time; closure captures `*AuthClient` + merged context_extensions + flags; per-Check timeout via `context.WithTimeout`.
4. **`buildAttributeContext` per ADR-0160 gRPC-mode portion + §6.6:** source/destination `Peer` per §11.P4; `request.http` populated set per parent §5.P4 + §11.P4 refinements; `request.time` as `Timestamp`; `tls_session.sni` populated only when `include_tls_session: true` AND TLS connection (per §11.P4 in-session RATIFICATION); `source.certificate` populated only when `include_peer_certificate: true` AND client cert presented; `destination.principal` populated AUTOMATICALLY from listener TLS cert (NOT gated); `source.principal` via ADR-0144 `DownstreamPrincipal()` first-value; `context_extensions` merge listener+per-route.
5. **`mapGRPCResponse` per ADR-0161 gRPC-mode portion + §6.7:** OkResponse → allow with `OkHttpResponse.headers` set/append + `headers_to_remove`; DeniedResponse → deny with verbatim headers (NOT filtered through `allowed_client_headers`); empty CheckResponse → allow; non-zero status with no oneof → deny default 403; transport error → dispError + `failure_mode_allow`/`status_on_error` posture; `validate_mutations` gating identical to HTTP-mode → dispInvalid → `invalid` counter.
6. **`compiledConfig` shape UNCHANGED:** no new field added (per §2.1 + ADR-0157 §Decision); 18.2 swaps only the `checkFn` closure.
7. **Per-route `context_extensions` consumption per §5:** the gRPC-mode-only field consumed proto-faithful; merged with listener-level (empty for MVP — `initial_metadata` deferred) at per-Check time; populates `AttributeContext.context_extensions`.
8. **Empirical pins:** parent §5 11 pins + §11.P4 + §11.P13 = all 13 pins CLOSED RATIFIED at the 18.2 SPEC commit (§11). 18.2 IMPL has zero RATIFIED-PENDING pins.
9. **Differential fixture `0021-http-ext-authz-grpc` per §7:** 8 scenarios (7 mirroring 0020 + 1 gRPC-only `OkHttpResponse` mutation); three-listener topology (l_test_a/b/c); byte-exact body + status on allow + deny paths; cross-side counter-delta equivalence on 5 reachable counters (`ok`, `denied`, `error`, `failure_mode_allowed`, `invalid`); auth-server received-CheckRequest assertions including the `context_extensions` content (scenario 7); 1 NEW test-helper `test/helpers/extauthzgrpc/` (FIRST in-process gRPC server).
10. **23rd fuzzer per §7.3:** `FuzzCheckResponseMapping` at `internal/filter/http/extauthz/fuzz_test.go`; 30s ADR-0018 budget; existing 22 fuzzers re-run clean; existing `FuzzExtAuthzConfigParse` corpus extended (config-parse path automatically covered without a new fuzzer).
11. **BEHAVIOR_CONTRACT.md populated** per Gate F: §13.1 ext_authz subsection's "gRPC mode — see phase 18.2" forward-pointers flipped to substantive gRPC content; §13.3 NEW row for 0021; §13.4 NEW `### Phase 18.2 forward-pointer notes`; NEW top-level `## gRPC client framework primitive (per phase 18.2 ADR-0158)` umbrella; §13.2 stat-table UNCHANGED at 77.
12. **DECISIONS.md populated** per ADR-on-impl convention: ADR-0158 §Decision + §Consequences landed (§Context was at the parent SPEC commit); ADR-0157 §Decision AMENDMENT landed; ADR-0160 + ADR-0161 gRPC-mode portion §Decision + §Consequences landed. NO new ADR numbers (ADR-0165 remains next-free; the §11.P13 closure removed the most-likely ADR-0044 escape-valve trigger; if an unanticipated ADR DOES land, it is ADR-0165).
13. **ROADMAP.md** row `18.2` flips `in-progress → done` AT THE SAME COMMIT as parent row `18` flips `in-progress → done` (per parent SPEC §8 parent-rollup discipline). The commit-message body MUST explicitly name BOTH transitions for grep-verifiability.
14. **All six phase-done gates green** at the 18.2 phase-done commit: build/vet/lint clean; race-test clean repo-wide; h2spec 53/53 PASS; 23 fuzzers green at 30s; 22 differential fixtures green (0000–0021); BEHAVIOR_CONTRACT.md populated.
15. **No master mutation outside the 18.2 squash-merge commit** — all work landed on the 18.2 worktree branches per ADR-0005 §Decision 4 + project memory `feedback_git_worktrees.md`; master tip advances only at the squash-merge commit + SHA-fill follow-up.

End of phase 18.2 SPEC.
