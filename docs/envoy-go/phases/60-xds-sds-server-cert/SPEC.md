# SPEC 60 — OPEN the xDS / dynamic-config family via **SDS** for a downstream server TLS certificate (State-of-the-World, a dedicated `StreamSecrets` stream over a gRPC `api_config_source`, INITIAL-FETCH scoped — no rotation)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only; NO production `.go` changes at this stage. Fresh worktree `.worktrees/phase-60-spec`, branch `phase-60-xds-sds-server-cert-spec`, off master `cbda648b`, per `feedback_git_worktrees`.
>
> **ANCHORS ADR-0278 §Context DRAFT** (§13). This is the FAMILY-OPENING xDS row; per the confirmed SPLIT (§3.0) ADR-0278 anchors at the **60.1** substrate IMPL and ADR-0280 anchors at the **60.2** TLS-apply IMPL; §Decision/§Consequences land at each leg's IMPL per ADR-0044. DECISIONS tail STAYS **ADR-0277** at this SPEC.
>
> **Baselines RE-VERIFIED against master tip `cbda648b` (the phase-60+61 fan-out registration):** stat surface **1201** · fixtures **103** (tail `0101-stats-sink-graphite`) · fuzzers **54** (verified `54` actual `^func Fuzz`) · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0277** (next-free **ADR-0278**) · new Go packages **0** · new go.mod modules **0**. Counts UNCHANGED at this SPEC (docs-only). Every `file:line` below was RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — the roster is §12.

---

## 1. Purpose / Mission

envoy-go is 100 %-static-bootstrap. Phase 60 OPENS the xDS / dynamic-config family by building the FIRST real dynamic-config path: a downstream server TLS **certificate** fetched at boot from a **Secret Discovery Service (SDS)** management server over a gRPC `StreamSecrets` stream (State-of-the-World), instead of read from inline bytes / a filesystem path. The operator configures a `DownstreamTlsContext` whose `common_tls_context.tls_certificate_sds_secret_configs[0]` names a Secret + a gRPC `ConfigSource.api_config_source`; the proxy dials the named static SDS cluster (via the existing `grpcclient.Dialer`), opens `StreamSecrets`, sends the initial `DiscoveryRequest`, receives a `Secret{tls_certificate}`, ACKs it, builds the leaf ONCE, and serves TLS with it. This stands up — for the first time — the discovery-stream client + the version/nonce ACK/NACK handshake + one resource-type applier: the three primitives every later xDS row (CDS/LDS/RDS/EDS/ADS/Delta) reuses.

The killer property: the SDS `ConfigSource` is INLINE in the TLS transport-socket config, so phase 60 lifts ONLY the narrow `internal/tls/config.go:153` `tls_certificate_sds_secret_configs` reject (downstream side only) and does NOT touch the bootstrap `dynamic_resources` reject (`bootstrap.go:499`) — the whole static-vs-dynamic boot model stays for later LDS/CDS rows.

ADR-0278 §Context is DRAFTED here (§13); §Decision/§Consequences at the IMPLs (ADR-0044). **All fourteen BRAINSTORM D-XDS-* questions are DISPOSED at this SPEC** — the load-bearing empirical ones via LIVE probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, fresh container per arm, shared bridge network, `reference_probe_fresh_container_per_arm` / `reference_docker_probe_bridge_network`):

| D-question | Disposition | Evidence class |
|---|---|---|
| **D-XDS-CONFIGSOURCE** | ACCEPT `api_config_source{api_type: GRPC, transport_api_version: V3, grpc_services:[{envoy_grpc{cluster_name}}]}` + `resource_api_version: V3`; reject `ads`/`self`/`google_grpc`/`DELTA_GRPC` (§2.3, §6). The probe config that BOOTED (§11 Arm A) used exactly this shape. | **PINNED** (Arm A) + DERIVED (reject arms) |
| **D-XDS-SOTW-VS-DELTA** | SotW (`StreamSecrets`); `type_url = envoy.extensions.transport_sockets.tls.v3.Secret` (re-derived via `proto.MessageName`, §5). Delta rejected. | **PINNED** (Arm A wire) |
| **D-XDS-HANDSHAKE** | Initial `DiscoveryRequest`: `version_info=""`, `response_nonce=""`, `resource_names=[<secret>]`, `type_url=…Secret`, `node{id,cluster}` populated, `error_detail=nil`. ACK: echo `(version_info, nonce)`. NACK: `error_detail` set + keep prior version. | **PINNED** (§11 Arm A/F: initial + ACK observed) |
| **D-XDS-INITIAL-FETCH** | Reference BLOCKS server-init (workers do NOT start, `/ready`=503) until the Secret arrives OR `initial_fetch_timeout` (default 15s) expires; on expiry it logs "initial fetch timed out", increments `sds.<n>.init_fetch_timeout`, STARTS workers cert-less (`/ready`→200), and TLS handshakes then FAIL ("no peer certificate available"). **envoy-go MVP DEPARTURE:** boot-BLOCK-then-BOOT-FAIL on timeout (§3.4) — envoy-go has no "serve a cert-less listener" state. | **PINNED** (§11 Arm B) |
| **D-XDS-MGMT-DOWN** | Reference: same as timeout — blocks init, `update_failure` climbs while dialing, then boots cert-less at `init_fetch_timeout`. Container never crashes. **envoy-go MVP:** the dial retries within the fetch window; unreachable-past-timeout ⇒ the same BOOT-FAIL as timeout (§3.4). | **PINNED** (§11 Arm B) |
| **D-XDS-SECRET-NOTFOUND** | A `DiscoveryResponse` whose `resources[]` lacks the requested secret name is ACKed but applies NOTHING (`update_success` stays 0, no `update_rejected`); the server stays init-pending → same as mgmt-down (boot-fail on timeout). | **PINNED** (§11 Arm N) |
| **D-XDS-SECRET-WIRE** | `Secret{name, tls_certificate{certificate_chain: inline/…, private_key: inline/…}}`; the served leaf is client-observable (serial/SAN/SPKI) — identical to the SDS-delivered cert. | **PINNED** (§11 Arm A: openssl s_client saw the exact delivered serial+SAN) |
| **D-XDS-STRICT-REJECT** | Six ADR-0080-distinct arms (§6). The reference SUPPORTS all six forms (they are standard reference features) ⇒ the loud rejects are a REAL envoy-go-strict DEPARTURE. | DERIVED (reference source/feature set) |
| **D-XDS-STATS** | Reference emits 14 `sds.<secret>.*` stats (§11 roster). envoy-go MVP registers a 5-counter subset: `update_success`/`update_failure`/`update_rejected`/`update_attempt`/`init_fetch_timeout`. The SDS stream IS accounted under `cluster.<sds_cluster>.upstream_cx_total`/`upstream_rq_200` (answers `reference_cluster_sink_dial_unaccounted`: NOT unaccounted — a normal cluster dial). Guard dynamic secret-name segments with `stats.IsValidName`. | **PINNED** (§11 full roster + cluster accounting) |
| **D-XDS-CONFIG-SEAM** | ONE package `internal/xds`; a `SecretProvider` blocking interface; threaded dialer→provider→`boot.Construct`→listener manager→`NewDownstreamConfig(ts, baseDir, provider)`; the block happens INSIDE the synchronous TLS build — NO async listener-warmup refactor (§3.2). Scope-risk verdict: **MANAGEABLE** (§3.2). | DERIVED (code-read of the boot/listener/TLS seam) |
| **D-XDS-NODE** | Reference HARD-REQUIRES `node.id` AND `node.cluster` for SDS — boots-FAIL without them (`TlsCertificateSdsApi: node 'id' and 'cluster' are required`). envoy-go must (a) populate the `DiscoveryRequest.node` from bootstrap `node` and (b) boot-reject an SDS config when `node.id`/`node.cluster` are absent. | **PINNED** (§11 Arm A-pre: first boot crashed on the missing node) |
| **D-XDS-FIXTURE** | ONE differential dir `0102-xds-sds-server-cert` (fixtures **103 → 104**) with a driver-owned `test/helpers/sdsserver` (NOT a BackendKind, `reference_differential_grpc_receiver_driver_owned`); assert the client-observed served leaf (serial/SAN) cross-side. Timeout/mgmt-down are SUBJECT-side boot-reject unit tests (divergent behavior ⇒ not cross-side comparable), not extra differential dirs (§8). | DECIDED |
| **D-XDS-FUZZER** | ONE NEW `FuzzDiscoveryResponseParse` (the `DiscoveryResponse`→`Secret` parse is a genuinely new untrusted wire boundary). fuzzers **54 → 55** (§6). | DECIDED |
| **D-XDS-SPLIT** | CONFIRMED SPLIT: **60.1 `xds-sds-stream-substrate`** (the `internal/xds` client + handshake + Secret parse + stats + fuzzer, unit-proven; ADR-0278) / **60.2 `xds-sds-tls-cert-apply`** (the `tls/config.go:153` lift + config-seam + initial-fetch + strict-reject + the differential; ADR-0280). ~16–18 tasks total > the ADR-0045 ~15 ceiling (§3.0). | DECIDED |

### 1.1 Empirical-finding-driven scope amendments (per ADR-0044)

Three probe findings AMEND the BRAINSTORM anticipations:

1. **D-XDS-NODE is a HARD boot-requirement, not a soft "does it populate node" question.** The reference REFUSED TO BOOT with the missing node (§11 Arm A-pre). envoy-go must both populate `DiscoveryRequest.node` from bootstrap `node` AND add a boot-reject when an SDS-bound TLS context is configured while `node.id`/`node.cluster` are empty. This is a NEW reject arm the BRAINSTORM did not enumerate (§6, arm 7).

2. **The initial fetch is a SERVER-INIT-manager block, and on timeout the reference SERVES CERT-LESS (broken handshakes), it does NOT boot-fail.** The BRAINSTORM left the timeout behavior open ("serve-without-cert / tear-down / boot-fail"). PINNED: serve-cert-less. Because envoy-go builds TLS synchronously at listener construction and has NO "start workers with a broken listener" state, envoy-go adopts a documented DEPARTURE — BOOT-FAIL on `initial_fetch_timeout` expiry (§3.4) — strictly safer than serving a listener that rejects every handshake.

3. **The SDS stream IS accounted under `cluster.*`** (`cluster.<sds_cluster>.upstream_cx_total: 1`, `upstream_rq_200: 1`). This resolves `reference_cluster_sink_dial_unaccounted` for the SDS case: reusing `grpcclient.Dialer` (which dials THROUGH the cluster `Manager`) gives the normal cluster cx/rq accounting for free — no special-casing needed, and no `Cluster.Dial` bypass.

Every other BRAINSTORM decision held (SotW; api_config_source-only; downstream-only; initial-fetch-only; grpcclient.Dialer reuse; the `tls/config.go:153` one-arm lift; the driver-owned fixture; the split).

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.3 + §8)

NO `dynamic_resources` (LDS/CDS/RDS/EDS) — the `bootstrap.go:499` reject STAYS. NO **ADS** (single muxed stream) — a dedicated `StreamSecrets` stream only. NO **Delta xDS** (`DeltaSecrets`). NO **RTDS** / Runtime layer. NO SDS **rotation / dynamic re-delivery** — INITIAL-FETCH only; the leaf is built ONCE and thereafter immutable (subsequent responses are ignored — no mutable-cert seam). NO SDS **`validation_context`** (the `tls/config.go:158` reject STAYS). NO **upstream (client-cert) SDS** — downstream server cert only (the upstream `commonTLSContextToConfig` arm keeps rejecting). NO `google_grpc`, `self`, or `ads` ConfigSource. NO reconnection/backoff hardening beyond the MVP dial-retry-within-the-fetch-window (§3.4). NO matching of the reference's full 14-stat `sds.*` surface (envoy-go registers a 5-counter subset, §7).

---

## 3. The change — a new `internal/xds` discovery-stream client + a config seam + a one-arm `tls/config.go` reject lift (ADR-0278 / ADR-0280)

### 3.0 Split disposition — a CONFIRMED SPLIT (60.1 substrate / 60.2 TLS-apply); ADR-0045 escape-valve CONSUMED

Anticipated ~16–18 tasks (§10) > the ADR-0045 `~15` ceiling. The cut mirrors the landed keystone/applier splits (39.1/39.2 health-check, 46.1a/46.1b tracing):

- **60.1 `xds-sds-stream-substrate`** — the family-opening keystone: the `internal/xds` discovery-stream client (dial via `grpcclient.Dialer`, `StreamSecrets`, the SotW `DiscoveryRequest`/`DiscoveryResponse` ACK/NACK version/nonce loop, the `Secret` parse into a `*stdtls.Certificate`, the `sds.*` stat registration, the `SecretProvider` interface), proven at UNIT level against an in-process fake SDS server + the new `FuzzDiscoveryResponseParse`. NO TLS integration, NO differential (a substrate-only leg — the phase-39.1/46.1a precedent). Anchors **ADR-0278**.
- **60.2 `xds-sds-tls-cert-apply`** — plumb the provider into the downstream TLS context (lift `tls/config.go:153`, downstream arm only), the config seam (§3.2), the initial-fetch boot-block-then-fail gate (§3.4), the six strict-reject arms (§6), and the driver-owned-SDS differential (§8). The OBSERVABLE end-to-end row. Anchors **ADR-0280**.

Row 60 flips `done` only once BOTH legs land (ADR-0106, `reference_roadmap_split_phase_row_done`).

### 3.1 The `internal/xds` discovery-stream client (60.1)

The greenfield `internal/xds/doc.go:1-4` placeholder becomes real. New in `internal/xds`:

- **A typed SDS stream client** — mirrors the `grpcclient.ALSClient`/`MetricsServiceClient` shape but BIDI (SDS `StreamSecrets` is `Send(*DiscoveryRequest)` + `Recv(*DiscoveryResponse)`, unlike ALS's client-streaming — RE-DERIVED from `service/secret/v3/sds_grpc.pb.go:85-99`). It obtains the `*grpc.ClientConn` from `grpcclient.Dialer.DialContext(ctx, sdsClusterName)` and wraps `secretv3.NewSecretDiscoveryServiceClient(conn).StreamSecrets(ctx)`. (A new typed wrapper `grpcclient.NewSDSClient` is the natural home for the dial, keeping `internal/xds` free of the cluster-manager import — mirrors `NewALSClient`; the SPEC leaves wrapper-here-vs-in-xds to the PLAN, but PINS the reuse of `Dialer.DialContext`.)
- **The SotW handshake loop** — send the initial `DiscoveryRequest{version_info:"", response_nonce:"", resource_names:[secretName], type_url:secretTypeURL, node:<bootstrap node>}`; on `Recv`, ACK with `DiscoveryRequest{version_info:resp.version_info, response_nonce:resp.nonce, …}`; on a parse/validation failure NACK with `error_detail` set + the PRIOR `version_info` (§11 PINS the initial + ACK shapes).
- **The `Secret` applier** — extract `resources[0]` (an `*anypb.Any`), verify `type_url == …Secret`, unmarshal to `*tlsv3.Secret`, confirm `GetName() == requested`, read `GetTlsCertificate().GetCertificateChain()` + `GetPrivateKey()` via the EXISTING `tls.loadDataSource` grammar, `stdtls.X509KeyPair` → a `*stdtls.Certificate`. A response missing the requested name is a no-op ACK (§11 Arm N).
- **The `SecretProvider` interface** (the seam to `internal/tls`, §3.2) — a blocking `FetchInitialCertificate(ctx, secretName) (*stdtls.Certificate, error)` bounded by `initial_fetch_timeout`.
- **The `sds.*` stats** (§7) registered under a `sds.<secret_name>.` scope, secret-name segment guarded by `stats.IsValidName` (`reference_dynamic_stat_name_charset_guard`).

### 3.2 The config seam — a blocking `SecretProvider` threaded dialer→Construct→listener→`NewDownstreamConfig` (60.2; D-XDS-CONFIG-SEAM; the biggest scope risk — DISPOSED as MANAGEABLE)

RE-DERIVED boot/TLS control flow against `cbda648b`:

- The downstream TLS config is built SYNCHRONOUSLY at listener construction: `buildListenerRuntimeWithCtx` (`listener/manager.go:340`) calls `internaltls.NewDownstreamConfig(ts, baseDir)` at `:382` and `:457`, which flows into `commonTLSContextToConfig` (`tls/config.go:149`) where the `tls_certificate_sds_secret_configs` reject lives (`:153`). The result is stored per-chain (`chainTLS`, `selected.tlsCfg` at `:1060`).
- The boot ordering (RE-DERIVED, `cmd/envoy-go/main.go`): `cluster.NewManagerWithBaseDir` (`:105`) → `dialer := grpcclient.New(cm)` (`:134`) → sinks + `tracingProvider` (`:150`) → `boot.Construct(bs, cm, baseDir, allowH2C, sinks, drainMgr, httpClient, tracingProvider)` (`:295`) → `listener.NewManagerWithBaseDirAndAllowH2C` (`boot.go:83`). The dialer EXISTS before listeners are built.

**Verdict — MANAGEABLE, NO async warmup refactor.** Because the TLS build is already a synchronous, blocking boot step, the initial-fetch block fits INSIDE it: the SDS provider is built at boot (from the dialer, mirroring how sinks/`tracingProvider` are built and passed IN — `boot.go:42-52` documents exactly this "cm exists before the dialer exists before the sinks" ordering), threaded as a NEW parameter through `boot.Construct` → `listener.NewManagerWithBaseDirAndAllowH2C` → `buildListenerRuntimeWithCtx` → `NewDownstreamConfig(ts, baseDir, provider)`. Inside `commonTLSContextToConfig`, the `:153` arm (downstream side only) dispatches to `provider.FetchInitialCertificate`, BLOCKS on the first Secret (bounded by `initial_fetch_timeout`), and appends the returned `*stdtls.Certificate` to `cfg.Certificates` — the SAME slice the static `tls_certificates[]` path fills (`:197`). No new listener state, no worker-thread warming, no mutable-cert seam. The `validate` boot path (which builds a throwaway listener manager) passes a nil/no-op provider (SDS configs are absent in validate fixtures).

**Package layout DECIDED:** ONE package `internal/xds` (NOT `internal/xds` + `internal/xds/sds`). A single Secret resource type does not justify a generic-core/specific-applier split; the discovery-stream loop and the Secret applier co-locate cleanly. A later CDS/EDS row may extract a generic core then — deferred.

### 3.3 The one-arm reject lift (60.2)

`commonTLSContextToConfig` (`tls/config.go:149`) is SHARED by downstream and upstream (`NewDownstreamConfig` `:34` and `NewUpstreamConfig` `:79`). The lift is therefore GATED on `side == "downstream"`:

```go
// tls/config.go:153 — the current wholesale reject, REPLACED:
if len(c.GetTlsCertificateSdsSecretConfigs()) > 0 {
    if side != "downstream" || provider == nil {
        return nil, fmt.Errorf("tls: %s: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03", side)
    }
    // downstream + provider present: parse the SdsSecretConfig, dispatch to SDS, block on the first Secret.
    // (multiple configs / validation_context_sds arms handled per §6)
}
```

The upstream path keeps the byte-identical reject (departure preserved). The `provider == nil` guard preserves the `validate`/no-SDS boot path.

### 3.4 Initial-fetch lifecycle — BLOCK then BOOT-FAIL on timeout (envoy-go DEPARTURE; D-XDS-INITIAL-FETCH / D-XDS-MGMT-DOWN)

`FetchInitialCertificate` blocks up to `initial_fetch_timeout` (the `SdsSecretConfig.sds_config.initial_fetch_timeout`, reference default 15s — mirror the default). Outcomes:

- **Secret arrives in time** → build the leaf, return it, boot proceeds (matches the reference: workers start, `/ready`→200, TLS serves the SDS leaf — §11 Arm A).
- **Timeout expires OR the mgmt server is unreachable past the window** → `FetchInitialCertificate` returns an error; `commonTLSContextToConfig` propagates it; the listener build FAILS; boot FAILS with a clear `tls: downstream: SDS secret %q: initial fetch timed out after %s` (or `… mgmt server unreachable`). This is a **documented DEPARTURE**: the reference instead starts workers CERT-LESS and rejects every handshake (§11 Arm B — `no peer certificate available`). envoy-go boot-failing is strictly safer (no silently-broken listener) and matches envoy-go's synchronous boot model. Recorded in BEHAVIOR_CONTRACT (§9) + ADR-0280.

The dial retry within the window is inherent to the gRPC stream (the `Dialer` reconnects); full exponential-backoff reconnection is deferred (§2).

---

## 4. Framework primitives — +1 production package, +1 test helper, 0 new go.mod modules

- **NEW production package `internal/xds`** (greenfield `doc.go` → real). Possibly a thin addition to `internal/grpcclient` (an `SDSClient` typed wrapper) — an EXISTING package. New Go packages **+1** (`internal/xds`) — the `test/helpers/sdsserver` (below) is a test package (production packages count only). If the PLAN homes the dial wrapper in a new sub-package the count adjusts; the SPEC pins **+1 production package** as the floor.
- **NEW test helper `test/helpers/sdsserver`** — a driver-owned fake SDS management server (`reference_differential_grpc_receiver_driver_owned`; the OTLP/ALS-receiver precedent). NOT imported by the production binary; NOT a runner BackendKind (BackendKind stays **38**).
- **go.mod modules: NONE.** CONFIRMED this session: the resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module carries `service/secret/v3` (`SecretDiscoveryServiceClient`, `StreamSecrets`/`DeltaSecrets`, `RegisterSecretDiscoveryServiceServer`), `service/discovery/v3` (`DiscoveryRequest`/`DiscoveryResponse`), and `extensions/transport_sockets/tls/v3` (`Secret`, `SdsSecretConfig`, `TlsCertificate`). A throwaway probe program compiled and linked against all three (§11). `go mod tidy -diff` anticipated EMPTY (existing-module imports only). New go.mod modules **0**.

---

## 5. Proto-field roster (RE-DERIVED @ go-control-plane/envoy v1.32.4)

`type_url`s RE-DERIVED via `proto.MessageName` (a throwaway program, `reference_network_filter_typeurl_extensions` — NOT trusting a SPEC string):
- `Secret` → **`envoy.extensions.transport_sockets.tls.v3.Secret`** (wire form `type.googleapis.com/…`).
- `DiscoveryRequest` → `envoy.service.discovery.v3.DiscoveryRequest`.
- `DiscoveryResponse` → `envoy.service.discovery.v3.DiscoveryResponse`.

| Message / field | Getter (source) | Phase-60 disposition |
|---|---|---|
| `SdsSecretConfig.name` | `GetName()` (`extensions/transport_sockets/tls/v3/secret.pb.go:118`) | the requested secret name; empty ⇒ reject |
| `SdsSecretConfig.sds_config` | `GetSdsConfig() *core.ConfigSource` (`secret.pb.go:125`) | the `ConfigSource`; missing ⇒ reject; must be `api_config_source` GRPC/V3 |
| `ConfigSource.api_config_source` / `.ads` / `.self` | `GetApiConfigSource()` / `GetAds()` / `GetSelf()` | ACCEPT `api_config_source`; reject `ads`/`self` |
| `ConfigSource.resource_api_version` | `GetResourceApiVersion()` | require V3 |
| `ConfigSource.initial_fetch_timeout` | `GetInitialFetchTimeout()` | the boot-block bound (default 15s) |
| `ApiConfigSource.api_type` | `GetApiType()` | require `GRPC`; reject `DELTA_GRPC`/`REST` |
| `ApiConfigSource.transport_api_version` | `GetTransportApiVersion()` | require V3 |
| `ApiConfigSource.grpc_services[].envoy_grpc.cluster_name` / `.google_grpc` | `GetGrpcServices()`, `GetEnvoyGrpc().GetClusterName()` / `GetGoogleGrpc()` | the SDS cluster; reject `google_grpc` |
| `DiscoveryRequest.{version_info,node,resource_names,type_url,response_nonce,error_detail}` | `discovery.pb.go:291/298/305/319/326/333` | the request/ACK/NACK fields (§3.1) |
| `DiscoveryResponse.{version_info,resources,type_url,nonce}` | `discovery.pb.go:419/426/440/447` | the response fields (§3.1) |
| `Secret.name` / `.tls_certificate` | `GetName()` (`secret.pb.go:181`), `GetTlsCertificate()` (`:195`) | the applied secret; other oneof arms (`validation_context`/`session_ticket_keys`/`generic_secret`) rejected |
| `TlsCertificate.{certificate_chain,private_key}` | `GetCertificateChain()`, `GetPrivateKey()` | fed to `stdtls.X509KeyPair` via `loadDataSource` |

---

## 6. PARSE/BOOT-REJECT roster + fuzzer (all ADR-0080-distinct substrings)

**Tier A — envoy-go-strict DEPARTURES (the reference SUPPORTS these; envoy-go rejects loudly):**
1. `ads`-sourced ConfigSource → `xds: sds: ads-sourced ConfigSource unsupported (only api_config_source)`.
2. `self`-sourced ConfigSource → `xds: sds: self-sourced ConfigSource unsupported (only api_config_source)`.
3. `DELTA_GRPC` api_type → `xds: sds: DELTA_GRPC api_type unsupported (only GRPC / State-of-the-World)`.
4. `google_grpc` transport → `xds: sds: google_grpc transport unsupported (only envoy_grpc)`.
5. downstream `validation_context_sds_secret_config` → `tls: downstream: SDS-bound validation_context_sds_secret_config is not supported in phase 03` (the EXISTING `tls/config.go:158-159` arm STAYS).
6. upstream `tls_certificate_sds_secret_configs` → `tls: upstream: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03` (the EXISTING `:153` reject, now GATED on `side != "downstream"`, §3.3).

**Tier A′ — the NODE boot-requirement (PINNED-driven, §1.1):**
7. an SDS-bound downstream TLS context configured while bootstrap `node.id`/`node.cluster` are empty → `xds: sds: node.id and node.cluster are required for SDS` (mirrors the reference's HARD boot-fail, §11 Arm A-pre — the reference REFUSES to boot; this is PARITY, not a departure).

**Tier B — structural PGV-parity / defensive rejects (mirror the reference's boot-reject):**
8. empty `SdsSecretConfig.name` → `xds: sds: SdsSecretConfig name is required`.
9. missing `SdsSecretConfig.sds_config` → `xds: sds: SdsSecretConfig sds_config is required`.
10. a `DiscoveryResponse` resource that is not a `Secret` type_url, or a `Secret` whose oneof is not `tls_certificate` → NACK / `xds: sds: response resource is not a tls_certificate Secret` (a runtime NACK, not a boot-reject).

**Additional strict caps (defensive, MVP-scoped):** more than one `tls_certificate_sds_secret_configs` entry MAY reject as `xds: sds: multiple tls_certificate_sds_secret_configs unsupported` (the reference allows a fallback list; envoy-go MVP takes one) — the PLAN decides whether to cap or take `[0]`.

**Fuzzer (D-XDS-FUZZER).** ONE NEW `FuzzDiscoveryResponseParse` in `internal/xds` — the `DiscoveryResponse`→`Secret`→`X509KeyPair` path is a genuinely new untrusted wire boundary (the mgmt server is untrusted input). Seed with a valid single-Secret response + malformed variants (wrong type_url, non-Secret Any, empty resources, garbage PEM). This is a NEW `func Fuzz`, so **fuzzers 54 → 55** (`reference_fuzzer_count_docs_drift`: reconcile actual `^func Fuzz` = 54 before, 55 after).

---

## 7. Stat surface — a 5-counter `sds.<secret>.*` subset (D-XDS-STATS)

The reference emits 14 `sds.<secret_name>.*` stats (§11 roster). envoy-go's MVP registers a defensible 5-COUNTER subset (`reference_stats_sink_emits_used_only` — assert a named subset, not the whole registry):

- `sds.<secret>.update_success` — incremented on a successfully-applied Secret.
- `sds.<secret>.update_failure` — a transport/dial failure (connect fail, stream error).
- `sds.<secret>.update_rejected` — a delivered Secret that fails validation (NACK).
- `sds.<secret>.update_attempt` — each request/response round.
- `sds.<secret>.init_fetch_timeout` — the initial fetch timed out (before envoy-go boot-fails, §3.4).

(NOT matched: the `control_plane.*` gauges, `version`/`version_text` text-readouts, `update_time`/`update_duration`, `key_rotation_failed` — rotation-only or admin-readout stats out of the MVP scope.)

The secret-name segment is operator-controlled ⇒ guard with `stats.IsValidName` before `NewCounterIfAbsent` (`reference_dynamic_stat_name_charset_guard` — which PANICS on an invalid name). The SDS stream's cx/rq are ALREADY accounted under `cluster.<sds_cluster>.upstream_cx_total`/`upstream_rq_200` (PINNED §11) — no new cluster-stat work.

**Surface arithmetic:** these are DYNAMIC per-secret registrations (registered when an SDS secret is parsed), so they do not add to the static `1201` baseline the way a fixed counter does — they materialize per configured secret at parse time. The differential fixture (one secret) surfaces 5 names. The SPEC pins the 5-counter SET + the `stats.IsValidName` guard; the exact "stat surface" count-delta is an IMPL reconciliation (anticipated **+5 registration guards** in the `internal/xds` registration path — the count the IMPL reports is the number of NewCounterIfAbsent call sites, i.e. **1201 → 1206** if counted as 5 fixed registration sites, or unchanged if counted purely dynamically; the IMPL pins which convention applies, consistent with how prior dynamic-scope stats were counted).

---

## 8. Differential fixture taxonomy — +1 (D-XDS-FIXTURE)

**ONE new differential dir `test/fixtures/0102-xds-sds-server-cert`** (fixtures **103 → 104**):

- A driver-owned `test/helpers/sdsserver` gRPC `SecretDiscoveryService` delivering one known `Secret{tls_certificate}` (a fixed self-signed leaf with a distinctive serial/SAN), reachable by BOTH proxies on a shared bridge network (`reference_docker_probe_bridge_network`; the proxy DIALS the server ⇒ driver-owned, `reference_differential_grpc_receiver_driver_owned`). `BackendCount ≥ 1` (`reference_differential_backendcount_min_one`).
- Both proxies' `envoy.yaml`/`envoy-go.yaml` configure the downstream `DownstreamTlsContext` with `tls_certificate_sds_secret_configs` → `api_config_source{GRPC, V3}` → a static `sds_cluster` naming the driver-owned server, AND a `node{id, cluster}` (PINNED REQUIRED, §11 Arm A-pre).
- The driver opens a TLS client to each proxy's listener and asserts the SERVED LEAF (serial + SAN + SPKI) is byte-identical cross-side (both dial the same server, so both serve the same SDS leaf — §11 Arm A confirms the client observes the exact delivered cert). Assert each property with `Errorf`, NOT `Fatalf` (`reference_fatalf_makes_assertions_unreachable`). The right asserter is the served-cert accessor (`reference_differential_asserter_dispatch` — this is a cross-side capture, not a StatsAsserter-only path).
- Prove the new assertion LIVE with a deliberate `-count=1` break confirming WHICH assertion fires (`reference_differential_break_protocol_count1`, `reference_deliberate_break_wrong_assertion`).

**NOT differential dirs (divergent behavior ⇒ not cross-side comparable):** the mgmt-down / init-fetch-timeout / secret-not-found cases (§11 Arm B/N) — the reference serves cert-less while envoy-go boot-fails (§3.4). These are SUBJECT-SIDE boot-reject / unit tests (`internal/xds` + `internal/tls` config tests + a subject-side boot-reject fixture asserting the timeout error substring), NOT extra differential dirs (`reference_differential_fixture_dispatch_constraint` — one dir = one runner branch). The strict-reject arms (§6) are subject-side boot-reject unit tests (the reference accepts, so no cross-side dir).

---

## 9. Behavior-contract delta (`docs/envoy-go/BEHAVIOR_CONTRACT.md`; landed at the 60.2 IMPL)

A NEW xDS/SDS section: SDS downstream server-cert SUPPORTED (SotW, `api_config_source` GRPC/V3, initial-fetch, no rotation); `node.id`+`node.cluster` REQUIRED for SDS (parity with the reference boot-requirement); the DEPARTURES (ads/self/DELTA_GRPC/google_grpc/validation_context-SDS/upstream-SDS reject loudly — the reference accepts); the initial-fetch-timeout DEPARTURE (envoy-go BOOT-FAILS where the reference serves cert-less); rotation/re-delivery ignored (initial-fetch-only); the `dynamic_resources`/`layered_runtime` rejects STILL stand. Exact lines RE-DERIVED and written at the IMPL.

---

## 10. Test plan + per-task structure (~16–18 tasks across two legs; PLAN decomposes)

TDD (`superpowers:test-driven-development`); each task a red→green with a `-count=1` liveness break where load-bearing.

**60.1 `xds-sds-stream-substrate` (unit-proven, ~8 tasks):**
1. `internal/xds` skeleton + `SecretProvider` interface + the `grpcclient` SDS dial wrapper (BIDI `StreamSecrets`).
2. The SotW handshake loop (initial request, ACK, NACK) — unit-tested against an in-process fake SDS server (SotW round-trip; version/nonce echo; NACK on bad secret).
3. The `Secret` applier (Any→Secret→X509KeyPair; name mismatch = no-op; non-Secret / non-tls_certificate = NACK).
4. `FetchInitialCertificate` blocking + `initial_fetch_timeout` (returns the leaf on success; error on timeout).
5. The `sds.*` 5-counter registration + the `stats.IsValidName` guard.
6. `FuzzDiscoveryResponseParse` + seeds; reconcile fuzzer 54 → 55.
7. `internal/xds` unit-test hardening (mgmt-down, secret-not-found, malformed PEM).
8. ADR-0278 body + STATE + verify (six-gate on `internal/xds` + `internal/grpcclient`).

**60.2 `xds-sds-tls-cert-apply` (differential, ~9 tasks):**
9. The `tls/config.go:153` one-arm lift (downstream-gated) + the `provider` param on `NewDownstreamConfig`/`commonTLSContextToConfig`; `config_test.go` accept + the six reject arms.
10. The node boot-requirement reject (arm 7) — RE-DERIVE the bootstrap node access.
11. The config seam: thread `provider` through `boot.Construct` → `listener.NewManagerWithBaseDirAndAllowH2C` → `buildListenerRuntimeWithCtx`; wire the provider in `main.go` (built from the dialer, next to sinks/`tracingProvider`).
12. The init-fetch boot-fail departure + a subject-side boot-reject test (timeout error substring).
13. `test/helpers/sdsserver` (driver-owned fake SDS server).
14. The `0102-xds-sds-server-cert` differential fixture (served-leaf cross-side; assertion proven live).
15. BEHAVIOR_CONTRACT xDS section.
16. ADR-0280 body + STATE + ROADMAP (row 60 `done` per ADR-0106 once both legs land; the xDS deferred sentence UPDATED) + verify (six-gate + full 104-dir differential, byte-stable except `0102`).

(The PLAN may split/merge; total ~16–18 across the two legs.)

---

## 11. SPEC-time empirical-pin block (D-XDS-* live probes — executed IN-SESSION 2026-07-12, `envoyproxy/envoy:contrib-v1.37.2`, FRESH container per arm, shared user-defined bridge `probe60-net`)

Docker is Docker-Desktop-on-Linux (context `desktop-linux`) — `--network host` and `172.17.0.1` do NOT reach the host (VM-isolated), so the memory-endorsed pattern was used: a driver-owned SDS server CONTAINERIZED on a shared bridge, reached by STRICT_DNS alias (`reference_docker_probe_bridge_network`). A ~90-line throwaway Go SDS `SecretDiscoveryServiceServer` (StreamSecrets returning one `Secret{tls_certificate{inline PEM}}`, self-signed leaf `CN=sds-probe-leaf serial=53866A6C…AFDC1059 SAN=DNS:sds.probe.example`) was built, containerized, and pointed-at by the reference's `tls_certificate_sds_secret_configs → api_config_source{GRPC,V3} → sds_cluster`. Decode VERIFIED non-vacuous (the server logged received `DiscoveryRequest`s; openssl s_client saw the served leaf). **All probe programs/images/containers/networks deleted after — this SPEC is docs-only.**

**Arm A-pre (D-XDS-NODE) — HARD boot-requirement.** With no `node` block, the reference REFUSED TO BOOT: `critical … error 'TlsCertificateSdsApi: node 'id' and 'cluster' are required. Set it either in 'node' config or via --service-node and --service-cluster options.' initializing config` → `exiting`. ⇒ `node.id` AND `node.cluster` are REQUIRED for SDS (§6 arm 7, §1.1 amendment 1).

**Arm A (D-XDS-CONFIGSOURCE / SOTW / HANDSHAKE / SECRET-WIRE / STATS) — the success path.** With `node{id:probe60-node, cluster:probe60-cluster}` added, the reference BOOTED, `/ready`→200, and served TLS. The SDS server received:
```
RECV DiscoveryRequest: version_info="" type_url="type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret" resource_names=[server_cert] response_nonce="" node.id="probe60-node" node.cluster="probe60-cluster" error_detail=<nil>   # the INITIAL request
RECV DiscoveryRequest: version_info="v1" … response_nonce="nonce-1" …                                                                                                                                                  # the ACK (echo version+nonce)
```
`openssl s_client -connect :10443` observed the served leaf = `subject=CN=sds-probe-leaf`, `serial=53866A6CEA9CDF54FD9C6EBB47FE8C83AFDC1059`, `SAN=DNS:sds.probe.example` — byte-identical to the SDS-delivered cert ⇒ the served leaf IS client-observable + differential-provable. The SDS stream was accounted under `cluster.sds_cluster.upstream_cx_total: 1`, `upstream_cx_active: 1`, `upstream_rq_200: 1`, `upstream_rq_total: 1` (⇒ NOT unaccounted, §1.1 amendment 3). Full `sds.server_cert.*` roster observed (14): `update_success`, `update_failure`, `update_rejected`, `update_attempt`, `init_fetch_timeout`, `key_rotation_failed`, `update_time`, `update_duration` (histogram), `version`, `version_text`, `control_plane.{connected_state, identifier, pending_requests, rate_limit_enforced}`. (`version_text:"v1"`, `update_success` incrementing on the success path.)

**Arm B (D-XDS-INITIAL-FETCH / MGMT-DOWN) — mgmt server DOWN at boot.** SDS cluster pointed at a dead alias; backend up. The container STAYED UP (no crash). For the first 15s: `/ready`=503, `listener_manager.workers_started: 0`, `total_listeners_active: 1`, `sds.server_cert.update_failure` climbing (8→10), `update_success: 0`. At exactly `initial_fetch_timeout` (15s default): log `warning … gRPC config: initial fetch timed out for type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret` → `all dependencies initialized. starting workers`; `/ready`→200, `workers_started: 1`, `sds.server_cert.init_fetch_timeout: 1`. BUT the TLS handshake then FAILED: `no peer certificate available` / `SSL handshake has read 0 bytes`. ⇒ the reference BLOCKS server-init on the fetch, then on timeout SERVES CERT-LESS (broken handshakes) — it does NOT boot-fail. Drives envoy-go's boot-fail DEPARTURE (§3.4, §1.1 amendment 2).

**Arm N (D-XDS-SECRET-NOTFOUND) — response missing the requested secret.** The server delivered a `Secret` named `wrong_name` while the reference requested `server_cert`. The reference ACKed (`version_info=v1`, `response_nonce=nonce-1`, `error_detail=nil`) but applied NOTHING: `update_success: 0`, `update_rejected: 0`, `update_failure: 0`, stayed init-pending (`/ready` never reached 200 within the window → same as mgmt-down). ⇒ a response lacking the requested secret name is a no-op ACK, not a NACK.

*(Probe harness: a throwaway `probe60cmd/` Go program + an alpine image `probe60-sds` reusing a self-signed cert; the source was written to a temp package inside the worktree, static-built to the scratchpad, and DELETED before this docs-only commit — `git status` verified clean, no `.go`/probe leftovers.)*

---

## 12. Edit-site roster (D-XDS-DOCSHAPE — RE-DERIVED against master `cbda648b`)

**NEW production — `internal/xds/` (60.1):**
- `internal/xds/*.go` (greenfield `doc.go:1-4` → real) — the SDS discovery-stream client, the SotW ACK/NACK handshake loop, the `Secret` applier, the `SecretProvider` interface, the `FetchInitialCertificate` blocking gate, the `sds.*` 5-counter registration. [ADD]
- `internal/grpcclient/grpcclient.go` — a typed `SDSClient` / `NewSDSClient(d *Dialer, cluster string)` BIDI wrapper reusing `dialConn` (the `ALSClient` `:315` shape; BIDI `StreamSecrets` `sds_grpc.pb.go:85-99`). [ADD — or homed in `internal/xds`; PLAN decides]

**Production — `internal/tls/config.go` (60.2):**
- `:153` — REPLACE the wholesale `tls_certificate_sds_secret_configs` reject with the downstream-gated lift + the ConfigSource/api_type/transport rejects (§3.3, §6). [EDIT]
- `:34` `NewDownstreamConfig` + `:149` `commonTLSContextToConfig` — ADD a `provider SecretProvider` parameter (nil on the upstream/validate paths). [EDIT]

**Production — `internal/listener/manager.go` (60.2):**
- `:254` `NewManagerWithBaseDirAndAllowH2C` + `:340` `buildListenerRuntimeWithCtx` + the two `NewDownstreamConfig` call sites (`:382`, `:457`) — thread the `provider` (§3.2). [EDIT]

**Production — `internal/boot/boot.go` + `cmd/envoy-go/main.go` (60.2):**
- `boot.go:53` `Construct` signature + `:83` the listener-manager call — ADD the `provider` param. [EDIT]
- `main.go:~150` — build the SDS `SecretProvider` from `dialer` (next to sinks/`tracingProvider`) and pass it into `boot.Construct` (`:295`). [EDIT]

**Test (60.1 + 60.2):**
- `internal/xds/*_test.go` — the discovery-stream client against an in-process fake SDS server (SotW ACK/NACK, Secret parse, name-mismatch no-op, timeout, mgmt-down). [ADD]
- `internal/xds/fuzz_test.go` — `FuzzDiscoveryResponseParse`. [ADD — fuzzers 54 → 55]
- `internal/tls/config_test.go` — accept a downstream gRPC-SDS cert config; reject the six arms + the node-required arm (distinct substrings). [ADD]
- `test/helpers/sdsserver/` — the driver-owned fake SDS management server. [ADD]

**Fixture (60.2):**
- `test/fixtures/0102-xds-sds-server-cert/` (new) — the downstream SDS server-cert differential (§8). [ADD — fixtures 103 → 104]

**Docs:**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the new xDS/SDS section (§9). [EDIT — 60.2 IMPL]
- `docs/envoy-go/ROADMAP.md:122` (row 60 → `done` once both legs land, ADR-0106) + `:181` (the xDS family deferred sentence UPDATE) [EDIT — IMPL]; `docs/envoy-go/STATE.md` (active-phase header) [EDIT — each stage]; `docs/envoy-go/DECISIONS.md` — ADR-0278 §Context here (§13); §Decision/§Consequences at each leg's IMPL. [ADD]

---

## 13. ADR continuity — the ADR-0278 §Context DRAFT (anchored at the 60.1 IMPL; ADR-0280 at 60.2)

**ADR-0278 §Context (draft).** envoy-go is 100 %-static-bootstrap: every listener/cluster/route/TLS cert materializes ONCE from `static_resources` at boot, and the bootstrap parser rejects the dynamic-config entry points (`bootstrap.go:499` `dynamic_resources`, `layered_runtime`). The reference is a dynamic-config proxy; the `internal/xds` (and `internal/runtime`) `doc.go` placeholders have named this family expansion since phase 00. Phase 60 OPENS the xDS family with its smallest defensible slice: a downstream server TLS certificate fetched at boot from a Secret Discovery Service over a dedicated gRPC `StreamSecrets` stream (State-of-the-World, initial-fetch, no rotation). The decisive property that makes SDS the opener: the SDS `ConfigSource` is INLINE in the TLS transport-socket config (`common_tls_context.tls_certificate_sds_secret_configs`), so opening xDS lifts ONLY the narrow `internal/tls/config.go:153` reject (downstream side) and does NOT lift the `dynamic_resources` reject or reshape the static boot model — the whole LDS/CDS/RDS/EDS lifecycle stays deferred. The 60.1 substrate (this ADR) builds the three primitives every later xDS row reuses: a discovery-stream client (reusing the phase-18 `grpcclient.Dialer` cluster-dial seam, so the SDS stream is accounted under the normal `cluster.*.upstream_cx`/`rq` counters — PINNED §11), the SotW version/nonce ACK/NACK handshake, and one Secret→`stdtls.Certificate` applier, plus a 5-counter `sds.<secret>.*` stat subset (`stats.IsValidName`-guarded) and a new `FuzzDiscoveryResponseParse` over the untrusted mgmt-server wire. SPEC-60 live probes against `envoyproxy/envoy:contrib-v1.37.2` (§11, fresh container per arm, driver-owned SDS server on a shared bridge) PINNED: the initial `DiscoveryRequest` shape (empty version/nonce, `resource_names=[secret]`, `type_url=…Secret`, `node{id,cluster}` populated) and the ACK (echo version+nonce); that `node.id` AND `node.cluster` are a HARD reference boot-requirement for SDS (the reference REFUSES to boot without them); the client-observable served leaf (differential-provable); and — CONTRADICTING the BRAINSTORM's open timeout question — that the reference BLOCKS server-init on the initial fetch, then on `initial_fetch_timeout` (15s) SERVES CERT-LESS (every handshake fails) rather than boot-failing. Because envoy-go builds TLS synchronously at listener construction with no "serve a cert-less listener" state, envoy-go adopts a documented DEPARTURE (ADR-0280): the initial fetch blocks INSIDE the synchronous TLS build (no async listener-warmup refactor — the config-seam scope risk is DISPOSED as MANAGEABLE), and on timeout / mgmt-unreachable envoy-go BOOT-FAILS with a clear error. The unsupported ConfigSource/SDS arms (ads/self/DELTA_GRPC/google_grpc/validation_context-SDS/upstream-SDS) reject loudly with distinct substrings — a documented envoy-go-strict DEPARTURE (the reference supports them), the SAME posture as the landed OTel-transport / Zipkin-version rejects (ADR-0080). A CONFIRMED SPLIT (60.1 substrate / 60.2 TLS-apply + differential, the 39.1/39.2 + 46.1a/46.1b precedent); +1 production package (`internal/xds`) + a driver-owned `test/helpers/sdsserver` / +1 differential fixture (`0102`) / +1 fuzzer / a 5-counter `sds.*` subset / +0 go.mod modules (go-control-plane v1.32.4 already carries `service/secret/v3` + `service/discovery/v3`). §Decision/§Consequences land at the 60.1 IMPL (ADR-0278) and the 60.2 IMPL (ADR-0280) per ADR-0044. ANCHORS ADR-0278.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

**Counts UNCHANGED at this SPEC (docs-only; re-verified against master tip `cbda648b`):** stat surface **1201** · fixtures **103** · fuzzers **54** · BackendKind **38** · DECISIONS tail **ADR-0277** (next-free **ADR-0278**) · new Go packages **0** · new go.mod modules **0**.

**Anticipated across the phase-60 IMPLs (60.1 + 60.2):** stat surface **1201 → +5 `sds.*` counter registrations** (IMPL pins the count convention for dynamic-scope stats, §7) · fixtures **103 → 104** (`0102-xds-sds-server-cert`) · fuzzers **54 → 55** (`FuzzDiscoveryResponseParse`) · BackendKind **38 (+0)** (driver-owned server) · DECISIONS tail **ADR-0277 → ADR-0280** (ADR-0278 @ 60.1, ADR-0280 @ 60.2; next-free **ADR-0281**) · new Go packages **+1** (`internal/xds`) · new go.mod modules **0**.

**ROADMAP/STATE at SPEC-DONE:** row 60 STAYS `in-progress` (flips `done` only once BOTH legs land at their IMPL six-gates, ADR-0106). The xDS family STAYS OPEN (rotation + validation_context/upstream SDS + CDS/EDS + LDS/RDS + ADS + Delta + RTDS + reconnection-backoff + google_grpc remain) ⇒ sentinel check-(3) drops never-opened families 5 → 4 (still ≥1, still prints); the LIVE deferred sentence is written by the controller at the IMPL (`reference_sentinel_deferred_sentence_live_vs_historical` — EXACTLY ONE live "candidates:" match). STATE active-phase header flips to `phase 60 SPEC done` (NEXT = the phase-60.1 PLAN).

**Next → the phase-60.1 PLAN** (the TDD decomposition of §10's substrate leg over this SPEC; every `file:line` RE-DERIVED against the master tip; ADR-0045 split confirmed; PROGRESS scaffolded).
