# Phase 60.1 Implementation Plan — `xds-sds-stream-substrate`: a NEW `internal/xds` discovery-stream client (dial via the phase-18 `grpcclient.Dialer` → a dedicated `StreamSecrets` BIDI stream → the SotW `DiscoveryRequest`/`DiscoveryResponse` version/nonce ACK/NACK loop → a `Secret`→`*crypto/tls.Certificate` applier) + the blocking `SecretProvider` seam bounded by `initial_fetch_timeout` + a 5-counter `sds.<secret>.*` stat subset (`stats.IsValidName`-guarded) + a driver-owned `test/helpers/sdsserver` fake management server + a NEW `FuzzDiscoveryResponseParse` — the FAMILY-OPENING xDS substrate, proven at UNIT level. NO TLS integration, NO differential (those are 60.2). The keystone leg of the confirmed 60.1/60.2 split (ADR-0278).

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`).

**Goal:** Stand up — for the FIRST time in a 100%-static-bootstrap project — the xDS discovery-stream substrate: an `internal/xds` client that dials a named static SDS cluster through the existing `grpcclient.Dialer`, opens a dedicated `SecretDiscoveryService/StreamSecrets` State-of-the-World stream, runs the `version_info`/`response_nonce` ACK/NACK handshake, parses a delivered `Secret{tls_certificate}` into a ready-to-serve `*crypto/tls.Certificate`, and exposes it through a blocking `SecretProvider.FetchInitialCertificate` seam bounded by `initial_fetch_timeout` — with the 5-counter `sds.<secret>.*` stat subset and a new untrusted-wire fuzzer. Everything is proven at UNIT level against an in-process fake SDS management server; there is NO downstream-TLS wiring and NO differential fixture at 60.1 (both are 60.2).

**Architecture:** ONE new production package `internal/xds` (the greenfield `doc.go` placeholder becomes real) + ONE typed BIDI wrapper `grpcclient.SDSClient` added to the EXISTING `internal/grpcclient` package (mirroring the `ALSClient` streaming shape, keeping the cluster-manager import out of `internal/xds`) + ONE driver-owned test helper `test/helpers/sdsserver`. The SotW handshake is a pure loop over a tiny `sdsStream` send/recv seam (unit-tested with an in-memory fake stream — no gRPC needed for the protocol logic) and separately integration-tested against the real fake server through a real `*grpc.ClientConn`. **Critical dependency-direction decision (locked in D-XDS-CONFIG-SEAM below): `internal/xds` does NOT import `internal/tls`.** At 60.2 `internal/tls` will import `internal/xds` to reference the `SecretProvider` interface as a `NewDownstreamConfig` parameter — so a `tls → xds` edge is coming; an `xds → tls` edge would make it a cycle. Therefore `internal/xds` carries its OWN minimal `dataSourceBytes` helper (mirroring `tls.loadDataSource`'s inline/filename arms) rather than reusing the unexported `tls.loadDataSource`. This keeps 60.1 with ZERO changes to `internal/tls` (honoring "60.1 has NO TLS integration"). ZERO new go.mod modules — `go-control-plane/envoy v1.32.4` already carries `service/secret/v3` + `service/discovery/v3` + `extensions/transport_sockets/tls/v3`.

**Tech Stack:** Go; the NEW `internal/xds` package; the EXISTING `internal/grpcclient` (the `Dialer` + the new `SDSClient` BIDI wrapper); `internal/stats` (`NewCounterIfAbsent` + `IsValidName` for the dynamic `sds.<secret>.*` scope); `crypto/tls` (`X509KeyPair` → `*tls.Certificate`); the resolved `go-control-plane/envoy v1.32.4` discovery/secret/tls protos (`SecretDiscoveryServiceClient.StreamSecrets`, `DiscoveryRequest`/`DiscoveryResponse`, `Secret`/`TlsCertificate`, `core.Node`/`core.DataSource`); `google.golang.org/genproto/googleapis/rpc/status` (the NACK `error_detail`); `google.golang.org/grpc` + `google.golang.org/protobuf` (`anypb`/`proto.Unmarshal`/`proto.MessageName`). ZERO new go.mod modules (`go mod tidy -diff` anticipated EMPTY).

---

## Global Constraints

- **One stage = 60.1 only (the substrate leg).** This PLAN is the 60.1 IMPL decomposition; the 60.2 TLS-apply + differential is a SEPARATE later PLAN. Row 60 stays `in-progress`; it flips `done` only once BOTH legs land (ADR-0106, `reference_roadmap_split_phase_row_done`).
- **NO `internal/tls` changes at 60.1.** The `tls/config.go:153` reject STAYS wholesale-rejecting; the one-arm lift + the `provider` parameter threading are 60.2. `internal/xds` must NOT import `internal/tls` (the cycle guard above).
- **NO differential fixture, NO `test/fixtures/NNNN-*` dir, NO BackendKind at 60.1.** The only cross-process surface is the driver-owned `test/helpers/sdsserver` (a `test/helpers` package the client DIALS — NOT a runner BackendKind, `reference_differential_grpc_receiver_driver_owned`). BackendKind STAYS 38.
- **ZERO new go.mod modules.** All protos resolve at the already-present `go-control-plane/envoy v1.32.4`. Every task's build gate includes `go mod tidy -diff` (expect EMPTY).
- **TDD (`superpowers:test-driven-development`):** every code task is failing-test → run-fail → minimal-impl → run-pass → commit. NO production code without a failing test first.
- **Per-task gates (`feedback_pertask_gofmt_lint`):** every code task ends with `gofmt -l` (expect empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. Do NOT skip gofmt.
- **Worktree hygiene (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`):** subagents write to the WORKTREE path (`.worktrees/phase-60.1-impl/…`); the controller verifies `git -C <main-checkout> status` stays clean after each task and the worktree branch is unchanged (no detached HEAD). Pin worktree-relative paths in every dispatch.
- **Commit locally only (`feedback_subagents_no_push`):** subagents NEVER push; the controller squashes + pushes at stage-close.
- **Break protocol (`reference_differential_break_protocol_count1`):** every deliberate-break liveness verification AND every `-race`/`-fuzz` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Dynamic stat-name guard (`reference_dynamic_stat_name_charset_guard`):** the `sds.<secret_name>.*` names carry a DYNAMIC operator-controlled `secret_name` segment. Guard with `stats.IsValidName(<full name>)` BEFORE `NewCounterIfAbsent` (which PANICS on an invalid name); skip registration (return nil-safe no-op counters) when invalid.
- **Fuzzer count reconciliation (`reference_fuzzer_count_docs_drift`):** the actual `grep -rh '^func Fuzz' --include='*.go' . | wc -l` is **54** at PLAN time (verified); the documented running total in STATE.md is **54** (no drift). `FuzzDiscoveryResponseParse` advances both to **55** — re-verify `== 55` at the completion task.
- **Wire-format both sides (`reference_wire_format_both_sides_see_same_bytes`):** the `type_url`, the ACK/NACK version/nonce echo, and the `Secret` grammar are the reference's — adopt them VERBATIM. RE-DERIVE the Secret `type_url` at runtime via `proto.MessageName(&tlsv3.Secret{})` (`reference_network_filter_typeurl_extensions`), never a hardcoded string.

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. Today the proxy is **100%-static-bootstrap**: every listener, cluster, route, and TLS certificate materializes ONCE from `static_resources` at boot, and there is NO dynamic-config / xDS path at all — the bootstrap parser rejects `dynamic_resources` (`internal/bootstrap/bootstrap.go:499`) and the TLS builder rejects SDS-bound certs (`internal/tls/config.go:153`). You are OPENING the xDS family with its smallest defensible slice — but at 60.1 you build ONLY the **substrate**: an `internal/xds` client that can dial an SDS management server, open a `StreamSecrets` stream, run the State-of-the-World handshake, receive+ACK a `Secret`, and hand back a built `*crypto/tls.Certificate` — all proven with UNIT tests against an in-process fake server. You do NOT touch `internal/tls`, you do NOT wire anything into boot, and you do NOT add a differential fixture. Those are the 60.2 leg.

**What "SDS / State-of-the-World / the ACK-NACK handshake" means (the protocol in one breath).** SDS is one xDS resource type (a `Secret`) delivered over a gRPC bidirectional stream `SecretDiscoveryService/StreamSecrets`. State-of-the-World (SotW) means each `DiscoveryResponse` carries the FULL current state (not deltas). The client opens the stream and sends an INITIAL `DiscoveryRequest{version_info:"", response_nonce:"", resource_names:[<secret name>], type_url:<Secret type_url>, node:{id,cluster}}`. The server replies with a `DiscoveryResponse{version_info:"v1", nonce:"nonce-1", type_url:<Secret type_url>, resources:[Any(Secret)]}`. The client validates the resource; if good it **ACKs** by sending a new `DiscoveryRequest` echoing `(version_info, response_nonce)` from the response; if bad it **NACKs** by sending a `DiscoveryRequest` that keeps the PRIOR `version_info` and sets `error_detail`. Phase 60.1 needs exactly ONE resource (the first Secret), so `FetchInitialCertificate` sends the initial request, receives the first response, validates+ACKs (or NACKs), and returns the built cert — it does NOT loop for rotation (rotation is deferred).

**The node requirement (why `node{id,cluster}` is on every request).** The SPEC-60 live probe (§11 Arm A-pre) PINNED that the reference REFUSES TO BOOT for SDS without `node.id` AND `node.cluster` (`TlsCertificateSdsApi: node 'id' and 'cluster' are required`). At 60.1 the client simply POPULATES `DiscoveryRequest.node` from a `Node{ID, Cluster}` it is given (unit tests assert the fake server received them). The BOOT-REJECT that enforces "SDS configured but node empty ⇒ fail boot" is a 60.2 config-seam concern — NOT in 60.1 scope.

**The initial-fetch lifecycle (blocking, bounded, no rotation).** `SecretProvider.FetchInitialCertificate(ctx, secretName)` BLOCKS until the first valid Secret arrives OR `initial_fetch_timeout` expires (reference default 15s). On success it returns the built `*tls.Certificate`. On timeout / mgmt-server-unreachable it returns an error (at 60.2 that error will boot-FAIL the listener — envoy-go's documented DEPARTURE from the reference's "serve cert-less" behavior; at 60.1 it is just an error the unit test asserts). The cert is built ONCE; subsequent responses are not consumed (no mutable-cert seam).

**Why `internal/xds` cannot import `internal/tls` (the load-bearing package-layout decision).** At 60.2, `internal/tls`'s `NewDownstreamConfig` gains a `provider SecretProvider` parameter, so `internal/tls` will import `internal/xds` for the `SecretProvider` type. If `internal/xds` also imported `internal/tls` (e.g. to reuse `tls.loadDataSource`), that would be an import CYCLE. So `internal/xds` is fully self-contained: it carries its own `dataSourceBytes(*corev3.DataSource, baseDir string) ([]byte, error)` (mirroring `tls.loadDataSource`'s `inline_bytes`/`inline_string`/`filename` arms; `environment_variable` → error; none-set → error) and builds the cert with `crypto/tls.X509KeyPair`. This is a small, justified duplication — the alternative (extracting a shared `internal/datasource` package + exporting `loadDataSource`) is a larger refactor that would touch `internal/tls` at 60.1, violating "NO TLS integration at 60.1". `internal/xds` imports only: `internal/grpcclient`, `internal/stats`, `crypto/tls`, the go-control-plane protos, `google.golang.org/protobuf/*`, `google.golang.org/genproto/.../status`. It imports NOTHING from `internal/tls`, `internal/bootstrap`, `internal/listener`, or `internal/boot`.

**The unit-test seam (why the SotW loop is a pure function over a tiny interface).** The protocol logic (send initial request, recv, validate, ACK/NACK) is tested TWO ways: (1) a pure `fetchSecret(stream sdsStream, ...)` over a 2-method `sdsStream` interface (`Send(*DiscoveryRequest) error` / `Recv() (*DiscoveryResponse, error)`) driven by an in-memory fake stream — no gRPC, deterministic, covers ACK/NACK/name-mismatch/malformed; and (2) an integration test where `Provider.FetchInitialCertificate` dials the REAL in-process `test/helpers/sdsserver` through a real `*grpc.ClientConn`, covering the end-to-end dial→stream→cert path + the timeout + the mgmt-down cases. This mirrors how `internal/grpcclient`'s streaming wrappers + `test/helpers/accessloggrpc` are tested.

### Key source seams (RE-DERIVED at PLAN time against master `565ac12a`; re-confirm line numbers before editing — files evolve)

- **`internal/grpcclient/grpcclient.go`** — `type Dialer struct { mgr *cluster.Manager }` (`:82`); `New(mgr) *Dialer` (`:89`); `DialContext(ctx, clusterName) (*grpc.ClientConn, error)` (`:109`) — PARSE-REJECTs unknown-cluster (`:118`) + non-H2 (`:121`). `connHolder` (`:167`, the shared `sync.Once`-guarded close base) + `dialConn(d, kind, clusterName)` (`:194`, the shared nil-check + `DialContext(context.Background(), …)`). **`ALSClient` (`:302`–`:345`) is the EXACT template** for the new `SDSClient`: `type ALSClient struct { connHolder; stub accesslogv3.AccessLogServiceClient }`; `NewALSClient(d, clusterName) (*ALSClient, error)` (`:315`, `dialConn(d, "ALS client", clusterName)` + `accesslogv3.NewAccessLogServiceClient(conn)`); `StreamAccessLogs(ctx) (…_StreamAccessLogsClient, error)` (`:329`, nil-guarded); `Close() error` (`:340`, nil-receiver-tolerant, delegates to `connHolder.close()`). **ADD the `SDSClient` wrapper here** (BIDI `StreamSecrets`, no per-call timeout — the ALSClient streaming shape, NOT the unary OTLP shape).
- **`internal/xds/doc.go`** (`:1`–`:4`) — the phase-00 greenfield placeholder (`// Package xds is a phase-00 placeholder. The real implementation lands in the xDS family (phases 09+)…`). **REPLACE with the real package doc.**
- **`internal/tls/config.go:153`** — `if len(c.GetTlsCertificateSdsSecretConfigs()) > 0 { return nil, fmt.Errorf("tls: %s: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03", side) }`. **NOT TOUCHED at 60.1** (read for context only — the 60.2 lift site).
- **`internal/tls/datasource.go:20`** — `func loadDataSource(ds *corev3.DataSource, baseDir string) ([]byte, error)` — UNEXPORTED; the grammar `internal/xds`'s own `dataSourceBytes` MIRRORS: `DataSource_InlineBytes`→bytes; `DataSource_InlineString`→[]byte(s); `DataSource_Filename`→`os.ReadFile` (relative to baseDir if not absolute); `DataSource_EnvironmentVariable`→error; default→error. **NOT imported** (the cycle guard).
- **`internal/stats/registry.go`** — `func IsValidName(name string) bool` (`:60`, the user-input-boundary guard); `func (r *Registry) NewCounter(name) *Counter` (`:84`, PANICS on invalid/duplicate/frozen — boot-time); `func (r *Registry) NewCounterIfAbsent(name) *Counter` (`:161`, idempotent, PERMITTED post-Freeze per ADR-0117, but STILL panics on an invalid name via `checkName`). **The `sds.<secret>.*` counters use `NewCounterIfAbsent` gated by an explicit `IsValidName` pre-check** (dynamic secret-name segment; the guard prevents the panic).
- **`test/helpers/accessloggrpc/accessloggrpc.go`** — the driver-owned in-process gRPC server TEMPLATE: `type Server struct { accesslogv3.UnimplementedAccessLogServiceServer; addr string; lis net.Listener; grpcSrv *grpc.Server; mu sync.RWMutex; … stopOnce sync.Once }` (`:49`); `New(t testing.TB) *Server` (`:73`, binds `127.0.0.1:0` + `t.Cleanup(Stop)`); `newServer(addr)` (`:109`, `net.Listen` + `grpc.NewServer()` + `RegisterAccessLogServiceServer` + `go grpcSrv.Serve(lis)`); the `StreamAccessLogs` drain loop (`Recv` until `io.EOF`, accumulate under the mutex); `Addr()`; `Stop()` (idempotent `GracefulStop` via `sync.Once`). **COPY this skeleton for `test/helpers/sdsserver`** (swap `AccessLogService` → `SecretDiscoveryService`; the handler Recv's the initial request, Send's a configured `DiscoveryResponse{Secret}`, records requests, keeps Recv'ing the ACK).
- **`test/helpers/accessloggrpc/doc.go`** — the 40-line package-doc precedent for `test/helpers/sdsserver/doc.go`.

### Proto facts (RE-DERIVED at PLAN time against `go-control-plane/envoy@v1.32.4`; re-confirm at IMPL)

- **`secretv3 "…/envoy/service/secret/v3"`** (`sds_grpc.pb.go`): `NewSecretDiscoveryServiceClient(cc) SecretDiscoveryServiceClient` (`:41`); `SecretDiscoveryServiceClient.StreamSecrets(ctx, opts...) (SecretDiscoveryService_StreamSecretsClient, error)` (`:33`); the client stream interface `SecretDiscoveryService_StreamSecretsClient { Send(*discoveryv3.DiscoveryRequest) error; Recv() (*discoveryv3.DiscoveryResponse, error); grpc.ClientStream }` (`:85`). Server side (the fake): `SecretDiscoveryServiceServer` interface with `StreamSecrets(SecretDiscoveryService_StreamSecretsServer) error` (`:119`); `UnimplementedSecretDiscoveryServiceServer` (`:126`, embed it); `RegisterSecretDiscoveryServiceServer(s grpc.ServiceRegistrar, srv)` (`:146`); the server stream interface `SecretDiscoveryService_StreamSecretsServer { Send(*discoveryv3.DiscoveryResponse) error; Recv() (*discoveryv3.DiscoveryRequest, error); grpc.ServerStream }` (`:180`).
- **`discoveryv3 "…/envoy/service/discovery/v3"`** (`discovery.pb.go`): `DiscoveryRequest{ VersionInfo string; Node *corev3.Node; ResourceNames []string; TypeUrl string; ResponseNonce string; ErrorDetail *status.Status }` — getters at `:291/:298/:305/:319/:326/:333`. `DiscoveryResponse{ VersionInfo string; Resources []*anypb.Any; TypeUrl string; Nonce string }` — getters at `:419/:426/:440/:447`. The `ErrorDetail` type is `status "google.golang.org/genproto/googleapis/rpc/status"` (`discovery.pb.go:13`) — for a NACK, build `&statuspb.Status{Message: <reason>}`.
- **`tlsv3 "…/envoy/extensions/transport_sockets/tls/v3"`** (`secret.pb.go` + `common.pb.go`): `Secret{ GetName() string (secret.pb.go:181); GetTlsCertificate() *TlsCertificate (:195); GetSessionTicketKeys() (:202); GetValidationContext() (:209); GetGenericSecret() (:216) }` — the oneof; 60.1 accepts ONLY `tls_certificate`, rejects the others. `TlsCertificate{ GetCertificateChain() *corev3.DataSource (common.pb.go:573); GetPrivateKey() *corev3.DataSource (:580) }`.
- **`corev3 "…/envoy/config/core/v3"`**: `Node{ Id string; Cluster string }` (built as `&corev3.Node{Id: node.ID, Cluster: node.Cluster}`); `DataSource` (the `GetSpecifier()` oneof: `DataSource_InlineBytes`/`DataSource_InlineString`/`DataSource_Filename`/`DataSource_EnvironmentVariable`).
- **type_url (RE-DERIVE at runtime, never hardcode):** `proto.MessageName(&tlsv3.Secret{})` == `envoy.extensions.transport_sockets.tls.v3.Secret` (VERIFIED this session via a throwaway program). The wire form is `"type.googleapis.com/" + string(proto.MessageName(&tlsv3.Secret{}))`. The applier compares `resource.GetTypeUrl()` (an Any's type URL already carries the `type.googleapis.com/` prefix) against this.
- **`anypb`/`proto`:** `resource *anypb.Any`; unmarshal via `resource.UnmarshalTo(&tlsv3.Secret{})` (checks the type URL) OR `anypb.UnmarshalTo`. Use `(*anypb.Any).UnmarshalTo(m)` which validates the embedded type URL matches `m`.

### Discipline (honor on EVERY task) — the memory traps that bite this row

- **`reference_dynamic_stat_name_charset_guard`** — `IsValidName` BEFORE `NewCounterIfAbsent` for every `sds.<secret>.*` name; skip+no-op on invalid.
- **`reference_differential_grpc_receiver_driver_owned`** — `test/helpers/sdsserver` is a driver-owned test package the client DIALS; NOT a runner BackendKind (BackendKind STAYS 38).
- **`reference_fuzzer_count_docs_drift`** — reconcile `^func Fuzz` count (54 → 55) BEFORE and AFTER; STATE.md documents 54.
- **`feedback_brief_citations_not_evidence`** — RE-DERIVE every `file:line` against the master tip at IMPL time; the citations in this PLAN were re-derived against `565ac12a` but the tree may drift.
- **`reference_fatalf_makes_assertions_unreachable`** — in tests asserting multiple independent properties, use `Errorf` per property; `Fatalf` only for a broken precondition.
- **`reference_network_filter_typeurl_extensions`** — verify the Secret type_url via `proto.MessageName`, NOT a SPEC/PLAN string literal; blank-import is unnecessary here because the applier references the typed `tlsv3.Secret{}` directly (no protojson registry round-trip at 60.1).

---

## D-question resolutions (the SPEC §1/§3 D-XDS-* PLAN pins — settled here)

**D-XDS-CONFIG-SEAM → ONE package `internal/xds`; `SecretProvider` interface HOMED in `internal/xds`; `internal/xds` does NOT import `internal/tls` (the cycle guard); the `grpcclient.SDSClient` dial wrapper lives in `internal/grpcclient`.** The SPEC (§3.2) DECIDED one package and left the dial-wrapper home to the PLAN. Pinned: the BIDI dial wrapper lands in `internal/grpcclient` (mirroring `ALSClient`/`MetricsServiceClient`) so `internal/xds` stays free of the `internal/cluster` import and consumes the wrapper through a small interface (for unit-test injection). `internal/xds` carries its own `dataSourceBytes` (the cycle guard — see the orientation brief). Acyclic import graph: `xds → {grpcclient, stats, crypto/tls, go-control-plane protos, protobuf, genproto/status}`; `grpcclient → cluster`; NO edge into `tls`/`bootstrap`/`listener`/`boot` from `xds`.

**D-XDS-SPLIT (60.1 slice) → the 60.1 substrate is ~10 tasks, comfortably under the ADR-0045 ~15 ceiling.** The 60.1/60.2 split is CONFIRMED at the SPEC (ADR-0045 escape-valve CONSUMED). This leg: the grpcclient wrapper + the xds package (skeleton/datasource/typeurl, the Secret applier, the SotW loop, the stats, the provider) + the fake server + the fuzzer + completion. No further sub-split.

**D-XDS-STATS (60.1 registration) → the 5-counter `sds.<secret>.*` subset is DYNAMIC (per-secret, registered at `Provider` construction via `NewCounterIfAbsent` under an `IsValidName` guard); the STATIC no-SDS stat surface stays 1201 at 60.1.** Because 60.1 has NO boot integration (no `Provider` is constructed in any boot path yet — that is 60.2), the static surface a no-SDS boot registers is UNCHANGED (1201). The 5 counters materialize only when a `Provider` is constructed (unit tests construct one with a test `*stats.Registry` and assert a +5 delta). At 60.2, the differential's configured secret surfaces the 5 names. This is the honest dynamic-scope treatment (`reference_stats_sink_emits_used_only`) — the 60.1 IMPL reports stat surface **1201 (+0 static)** and proves the +5 dynamic delta by a unit test.

**D-XDS-FUZZER → ONE new `FuzzDiscoveryResponseParse` in `internal/xds` (fuzzers 54 → 55).** The `DiscoveryResponse`→`Secret`→`X509KeyPair` path is a genuinely new untrusted-wire boundary (the mgmt server is untrusted input). Seed with a valid single-Secret response + malformed variants (wrong type_url, non-Secret Any, empty resources, garbage PEM, wrong-oneof Secret). No-panic invariant over arbitrary bytes.

**D-XDS-NODE (60.1 slice) → the client POPULATES `DiscoveryRequest.node` from a `Node{ID, Cluster}` it is given; the node-required BOOT-REJECT is 60.2.** At 60.1 the `Node` is a plain struct field on the `Provider`/`Config`; unit tests assert the fake server received the populated `node.id`/`node.cluster`. The `xds: sds: node.id and node.cluster are required for SDS` boot-reject (SPEC §6 arm 7) is a 60.2 config-seam concern.

**D-XDS-INITIAL-FETCH (60.1 slice) → `FetchInitialCertificate` BLOCKS bounded by `initial_fetch_timeout` (default 15s); on timeout / mgmt-unreachable it returns a classified error + increments `init_fetch_timeout`/`update_failure`.** The boot-FAIL DEPARTURE (envoy-go boot-fails where the reference serves cert-less) is a 60.2 concern — at 60.1 the error is just returned + asserted by a unit test (with a small test timeout, e.g. 200ms, to keep the suite fast).

---

## File structure (decomposition locked here)

**Production (created / modified):**
- `internal/xds/doc.go` — MODIFY: replace the phase-00 placeholder with the real package doc.
- `internal/xds/secret.go` — CREATE: `dataSourceBytes(*corev3.DataSource, baseDir) ([]byte, error)`; `secretTypeURL() string` (via `proto.MessageName(&tlsv3.Secret{})`); `parseSecret(resource *anypb.Any, wantName, baseDir string) (*stdtls.Certificate, error)` (Any→Secret→validate name+oneof→`dataSourceBytes`×2→`X509KeyPair`).
- `internal/xds/stream.go` — CREATE: `Node{ID, Cluster string}`; the `sdsStream interface { Send(*discoveryv3.DiscoveryRequest) error; Recv() (*discoveryv3.DiscoveryResponse, error) }`; `fetchSecret(stream sdsStream, node Node, secretName, baseDir string) (*stdtls.Certificate, error)` (the SotW initial-request→Recv→parse→ACK/NACK loop) + the classified error sentinels (`errValidation` for a NACK-worthy reject).
- `internal/xds/stats.go` — CREATE: `type SDSStats struct { updateSuccess, updateFailure, updateRejected, updateAttempt, initFetchTimeout *stats.Counter }`; `RegisterSDSStats(reg *stats.Registry, secretName string) *SDSStats` (the `IsValidName` guard + `NewCounterIfAbsent` ×5; nil-safe no-op on invalid); nil-safe `(*SDSStats).incX()` helpers.
- `internal/xds/provider.go` — CREATE: `SecretProvider interface { FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error) }`; `streamOpener interface { StreamSecrets(ctx context.Context) (sdsStream, error) }` (the dial seam for injection); `Provider struct` + `NewProvider(opener streamOpener, node Node, baseDir string, timeout time.Duration, stats *SDSStats) *Provider`; `(*Provider).FetchInitialCertificate` (context-timeout + `opener.StreamSecrets` + `fetchSecret` + the stat increments).
- `internal/grpcclient/grpcclient.go` — MODIFY: ADD `SDSClient` (BIDI `StreamSecrets` wrapper) + `NewSDSClient(d *Dialer, clusterName string) (*SDSClient, error)` + `(*SDSClient).StreamSecrets(ctx) (secretv3.SecretDiscoveryService_StreamSecretsClient, error)` + `(*SDSClient).Close() error`.

**Test (created / modified):**
- `internal/xds/secret_test.go` — `dataSourceBytes` (inline/filename/env-reject/none) + `secretTypeURL` + `parseSecret` (valid / wrong name / wrong oneof / non-Secret Any / bad PEM).
- `internal/xds/stream_test.go` — `fetchSecret` over an in-memory fake stream (ACK on success; NACK on validation failure keeping prior version; name-mismatch → error/no-op; the initial-request node/resource_names/type_url assertion).
- `internal/xds/stats_test.go` — `RegisterSDSStats` +5 delta + invalid-name skip + `Record` dispatch.
- `internal/xds/provider_test.go` — `FetchInitialCertificate` against the real `test/helpers/sdsserver` (success; timeout; mgmt-down/dial-fail; name-not-found) + the stat-increment assertions.
- `internal/xds/fuzz_test.go` — `FuzzDiscoveryResponseParse`.
- `internal/grpcclient/grpcclient_test.go` — MODIFY: `NewSDSClient` unknown-cluster + non-H2 rejects + nil-receiver `Close`; a `StreamSecrets` round-trip against `test/helpers/sdsserver` (optional integration, if the existing test harness makes a cluster+manager cheap — else defer the round-trip to `provider_test.go`).
- `test/helpers/sdsserver/sdsserver.go` + `test/helpers/sdsserver/doc.go` + `test/helpers/sdsserver/sdsserver_test.go` — CREATE: the driver-owned fake SDS management server + a self-test.

**Docs (completion task):**
- `docs/envoy-go/phases/60-xds-sds-server-cert/PROGRESS-60.1.md` (scaffolded Task 1, finalized at completion).
- `docs/envoy-go/DECISIONS.md` — ADR-0278 §Decision/§Consequences body (ANCHORS at this 60.1 IMPL, ADR-0044; §Context already drafted at the SPEC §13).
- `docs/envoy-go/STATE.md` — active-phase header flip (phase 60.1 IMPL done; NEXT = phase-60.2 PLAN).
- `docs/envoy-go/ROADMAP.md` — row 60 STAYS `in-progress` (does NOT flip `done` — 60.2 pending, ADR-0106). NO deferred-sentence change at 60.1 (the xDS family stays open unchanged; the sentence is authored at the row-completing 60.2 IMPL).
- **NO `BEHAVIOR_CONTRACT.md` change at 60.1** (the xDS/SDS section lands at the 60.2 IMPL, per SPEC §9 — 60.1 has no observable end-to-end behavior).

---

## Task 1: Phase scaffolding — PROGRESS-60.1.md + baselines + the D-XDS PLAN-pin record

**Files:**
- Create: `docs/envoy-go/phases/60-xds-sds-server-cert/PROGRESS-60.1.md`

- [ ] **Step 1: Record the baseline counts** — run and record the verbatim outputs in PROGRESS-60.1.md:
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                    # expect 104 (tail 0102-tracing-custom-tags-literal)
grep -rh '^func Fuzz' --include='*.go' . | wc -l                     # expect 54
grep -c '' <(grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md)   # DECISIONS tail = ADR-0277 (next-free ADR-0278)
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go     # BackendKind tail = 38
ls internal/xds/                                                     # expect: doc.go only (the placeholder)
```
Baseline: stat surface **1201** / fixtures **104** / fuzzers **54** / BackendKind **38** / DECISIONS tail **ADR-0277** (next-free **ADR-0278**) / new Go packages **0** / new go.mod modules **0**.

- [ ] **Step 2: Write the PROGRESS-60.1.md scaffold** — a header (phase 60.1 IMPL, the SPEC-60 reference + the "60.1 substrate sub-leg of the confirmed 60.1/60.2 split" note, the worktree branch `phase-60-xds-sds-stream-substrate-impl`), a task checklist mirroring this plan (Tasks 1–10), the baseline-counts block, and the anticipated exit counts: stat surface **1201 (+0 static; +5 dynamic `sds.<secret>.*` proven by a unit delta)** / fixtures **104 (+0)** / fuzzers **55** (`FuzzDiscoveryResponseParse`) / BackendKind **38 (+0)** / DECISIONS **ADR-0277 → ADR-0278** (anchored at this IMPL) / new Go packages **+1** (`internal/xds`) + **+1 test package** (`test/helpers/sdsserver`) / **0 new go.mod modules**.

- [ ] **Step 3: Record the D-XDS PLAN pins** — a short section restating the four D-question resolutions above (CONFIG-SEAM one-package + no-tls-import cycle guard + grpcclient-homed wrapper; STATS dynamic +0-static; FUZZER +1; NODE/INITIAL-FETCH 60.1 slice) so the executing engineer sees the pinned decisions without re-reading the SPEC. (Bookkeeping — not a code change.)

- [ ] **Step 4: Commit**
```bash
git add docs/envoy-go/phases/60-xds-sds-server-cert/PROGRESS-60.1.md
git commit -m "phase 60.1 Task 1: PROGRESS scaffold + baselines + the D-XDS PLAN pins (config-seam/stats/fuzzer/node)"
```

---

## Task 2: `internal/xds` skeleton — package doc + `dataSourceBytes` + `secretTypeURL` [TDD]

**Files:**
- Modify: `internal/xds/doc.go`
- Create: `internal/xds/secret.go`
- Test: `internal/xds/secret_test.go`

The foundation: the DataSource→bytes helper (the cycle-guard duplicate of `tls.loadDataSource`) + the runtime-derived Secret type_url. No stream, no gRPC yet.

- [ ] **Step 1: Write the failing tests** in `secret_test.go`:
  - `secretTypeURL()` returns `"type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret"`.
  - `dataSourceBytes(&corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte("PEM")}}, "")` ⇒ `([]byte("PEM"), nil)`.
  - `dataSourceBytes(&corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: "PEM"}}, "")` ⇒ `([]byte("PEM"), nil)`.
  - `dataSourceBytes` with a `DataSource_Filename` pointing at a `t.TempDir()`-written file (relative to a baseDir) ⇒ the file bytes.
  - `dataSourceBytes` with a `DataSource_EnvironmentVariable` ⇒ a non-nil error whose message contains `environment_variable`.
  - `dataSourceBytes(&corev3.DataSource{}, "")` (none set) ⇒ a non-nil error whose message contains `inline_bytes`.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/xds/ -count=1` ⇒ FAIL (package symbols undefined).

- [ ] **Step 3: Implement** — replace `internal/xds/doc.go` with the real package doc:
```go
// Package xds implements the envoy-go xDS / dynamic-config substrate. Phase 60.1
// opens the family with the Secret Discovery Service (SDS): a client that dials a
// named static SDS cluster (via internal/grpcclient.Dialer), opens a dedicated
// SecretDiscoveryService/StreamSecrets State-of-the-World stream, runs the
// version/nonce ACK/NACK handshake, and parses a delivered Secret{tls_certificate}
// into a *crypto/tls.Certificate exposed through the blocking SecretProvider seam
// (bounded by initial_fetch_timeout). Initial-fetch only — no rotation. This
// package does NOT import internal/tls (internal/tls imports this package at 60.2
// for the SecretProvider seam; an xds->tls edge would cycle). See ADR-0278.
package xds
```
And create `internal/xds/secret.go`:
```go
package xds

import (
	"fmt"
	"os"
	"path/filepath"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
)

// secretTypeURL returns the wire type URL for an SDS Secret resource, derived at
// runtime from the proto descriptor (never hardcoded — reference_network_filter_typeurl_extensions).
func secretTypeURL() string {
	return "type.googleapis.com/" + string(proto.MessageName(&tlsv3.Secret{}))
}

// dataSourceBytes resolves a core.DataSource into raw bytes. It MIRRORS
// internal/tls.loadDataSource's phase-03 grammar (inline_bytes / inline_string /
// filename honored; environment_variable + zero-value error) but is duplicated
// here deliberately: internal/xds must NOT import internal/tls (the 60.2 cycle
// guard — see doc.go / ADR-0278). A non-absolute filename resolves relative to
// baseDir.
func dataSourceBytes(ds *corev3.DataSource, baseDir string) ([]byte, error) {
	switch s := ds.GetSpecifier().(type) {
	case *corev3.DataSource_InlineBytes:
		return s.InlineBytes, nil
	case *corev3.DataSource_InlineString:
		return []byte(s.InlineString), nil
	case *corev3.DataSource_Filename:
		p := s.Filename
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("xds: sds: data source: read %s: %w", p, err)
		}
		return b, nil
	case *corev3.DataSource_EnvironmentVariable:
		return nil, fmt.Errorf("xds: sds: data source: environment_variable is not supported")
	default:
		return nil, fmt.Errorf("xds: sds: data source: none of inline_bytes, inline_string, filename set")
	}
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/xds/ -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/xds/ && golangci-lint run ./internal/xds/... && go vet ./internal/xds/... && go build ./... && go mod tidy -diff
git add internal/xds/doc.go internal/xds/secret.go internal/xds/secret_test.go
git commit -m "phase 60.1 Task 2: internal/xds skeleton — dataSourceBytes (cycle-guard duplicate of tls.loadDataSource) + runtime secretTypeURL (D-XDS-CONFIG-SEAM)"
```

---

## Task 3: The `Secret` applier — `parseSecret` (Any→Secret→`*tls.Certificate`) [TDD]

**Files:**
- Modify: `internal/xds/secret.go`
- Test: `internal/xds/secret_test.go`

The resource applier: validate the Any's type URL, unmarshal to `Secret`, confirm the name, confirm the `tls_certificate` oneof arm, build the leaf via `dataSourceBytes` + `X509KeyPair`.

- [ ] **Step 1: Write the failing tests** in `secret_test.go` (use a test helper `selfSignedPEM(t)` returning a valid cert+key PEM pair — generate once via `crypto/x509` + `crypto/ecdsa` in the test, or embed a fixed known-good pair):
  - `parseSecret(anyOf(&tlsv3.Secret{Name:"server_cert", Type:&tlsv3.Secret_TlsCertificate{TlsCertificate:&tlsv3.TlsCertificate{CertificateChain: inlineDS(certPEM), PrivateKey: inlineDS(keyPEM)}}}), "server_cert", "")` ⇒ a non-nil `*tls.Certificate` whose `Leaf`/first `Certificate` parses (assert `len(cert.Certificate) == 1`).
  - wrong name: `parseSecret(anyOf(Secret{Name:"other", …}), "server_cert", "")` ⇒ a non-nil error containing `name` (the requested-name mismatch).
  - wrong oneof: a `Secret{Name:"server_cert", Type:&tlsv3.Secret_ValidationContext{…}}` ⇒ error containing `tls_certificate`.
  - non-Secret Any: an `anypb.Any` wrapping a `discoveryv3.DiscoveryRequest{}` (wrong type URL) ⇒ error containing `type` (the `UnmarshalTo` type-mismatch).
  - bad PEM: a `Secret` whose cert bytes are `[]byte("not pem")` ⇒ error containing `load`/`X509KeyPair`.
  - (helper) `anyOf(m proto.Message) *anypb.Any` via `anypb.New(m)`; `inlineDS(b []byte) *corev3.DataSource` via `&corev3.DataSource{Specifier:&corev3.DataSource_InlineBytes{InlineBytes:b}}`.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/xds/ -run TestParseSecret -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `parseSecret` in `secret.go`:
```go
import (
	stdtls "crypto/tls"
	"google.golang.org/protobuf/types/known/anypb"
	// + tlsv3, fmt already imported
)

// parseSecret validates one DiscoveryResponse resource and builds the served leaf.
// It requires: the Any resolves to a tls.v3.Secret; Secret.name == wantName; the
// secret's oneof is tls_certificate; and the certificate_chain/private_key
// DataSources yield a valid X509 key pair. Returns a classified error (wrapping
// errValidation, Task 4) so the caller can NACK + increment update_rejected.
func parseSecret(resource *anypb.Any, wantName, baseDir string) (*stdtls.Certificate, error) {
	var sec tlsv3.Secret
	if err := resource.UnmarshalTo(&sec); err != nil {
		return nil, fmt.Errorf("xds: sds: resource is not a %s: %w", secretTypeURL(), err)
	}
	if sec.GetName() != wantName {
		return nil, fmt.Errorf("xds: sds: response secret name %q != requested %q", sec.GetName(), wantName)
	}
	tc := sec.GetTlsCertificate()
	if tc == nil {
		return nil, fmt.Errorf("xds: sds: secret %q is not a tls_certificate (unsupported oneof arm)", wantName)
	}
	certPEM, err := dataSourceBytes(tc.GetCertificateChain(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: secret %q: certificate_chain: %w", wantName, err)
	}
	keyPEM, err := dataSourceBytes(tc.GetPrivateKey(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: secret %q: private_key: %w", wantName, err)
	}
	pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: secret %q: load cert: %w", wantName, err)
	}
	return &pair, nil
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/xds/ -run TestParseSecret -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/xds/ && golangci-lint run ./internal/xds/... && go build ./... && go mod tidy -diff
git add internal/xds/secret.go internal/xds/secret_test.go
git commit -m "phase 60.1 Task 3: parseSecret — Any->Secret->X509KeyPair applier (name + tls_certificate-oneof validation)"
```

---

## Task 4: The SotW handshake loop — `fetchSecret` over the `sdsStream` seam [TDD]

**Files:**
- Create: `internal/xds/stream.go`
- Test: `internal/xds/stream_test.go`

The protocol heart: send the initial `DiscoveryRequest`, `Recv` the first response, parse+validate, ACK (or NACK). Tested with an in-memory fake stream (no gRPC).

- [ ] **Step 1: Write the failing tests** in `stream_test.go` — define a `fakeStream` implementing `sdsStream` that records every `Send`'d `*DiscoveryRequest` and returns programmed `Recv` responses:
```go
type fakeStream struct {
	sent  []*discoveryv3.DiscoveryRequest
	resps []*discoveryv3.DiscoveryResponse // popped FIFO on each Recv
	recvErr error                          // returned once resps is empty (default io.EOF)
}
func (f *fakeStream) Send(r *discoveryv3.DiscoveryRequest) error { f.sent = append(f.sent, r); return nil }
func (f *fakeStream) Recv() (*discoveryv3.DiscoveryResponse, error) {
	if len(f.resps) == 0 { if f.recvErr != nil { return nil, f.recvErr }; return nil, io.EOF }
	r := f.resps[0]; f.resps = f.resps[1:]; return r, nil
}
```
  - **initial-request shape**: drive `fetchSecret` with a fake whose first `Recv` returns a valid `DiscoveryResponse{VersionInfo:"v1", Nonce:"n1", TypeUrl:secretTypeURL(), Resources:[anyOf(validSecret("server_cert"))]}`; assert `sent[0]` has `VersionInfo==""`, `ResponseNonce==""`, `ResourceNames==["server_cert"]`, `TypeUrl==secretTypeURL()`, `Node.Id=="node-1"`, `Node.Cluster=="cluster-1"`, `ErrorDetail==nil`.
  - **ACK on success**: assert `fetchSecret` returns a non-nil `*tls.Certificate` + nil error, AND `sent[1]` (the ACK) echoes `VersionInfo=="v1"`, `ResponseNonce=="n1"`, `ErrorDetail==nil`.
  - **NACK on validation failure**: a first response carrying a `Secret` with the WRONG name ⇒ `fetchSecret` returns an error wrapping `errValidation`; `sent[1]` (the NACK) has `VersionInfo==""` (the prior version, unchanged) + a non-nil `ErrorDetail` whose `Message` is non-empty.
  - **transport error**: a fake whose `Recv` returns `io.EOF` immediately (no responses) ⇒ `fetchSecret` returns a non-`errValidation` error (a transport class).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/xds/ -run TestFetchSecret -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `internal/xds/stream.go`:
```go
package xds

import (
	stdtls "crypto/tls"
	"errors"
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
)

// Node carries the DiscoveryRequest node identity (id + cluster), populated from
// bootstrap `node`. Both are REQUIRED for SDS by the reference (the boot-reject
// that enforces this is 60.2; 60.1 just transmits them).
type Node struct {
	ID      string
	Cluster string
}

// sdsStream is the 2-method send/recv seam fetchSecret operates over — satisfied
// by the real *grpcclient SDS client stream AND by an in-memory fake in tests.
type sdsStream interface {
	Send(*discoveryv3.DiscoveryRequest) error
	Recv() (*discoveryv3.DiscoveryResponse, error)
}

// errValidation classifies a delivered-but-invalid Secret (a NACK-worthy reject —
// the caller increments update_rejected), distinct from a transport error.
var errValidation = errors.New("xds: sds: secret validation failed")

// fetchSecret runs one State-of-the-World fetch: sends the initial DiscoveryRequest
// for secretName, Recv's the first response, parses+validates its Secret, and ACKs
// (echoing version_info+nonce) on success or NACKs (keeping the prior version,
// setting error_detail) on a validation failure. Returns the built leaf on success.
func fetchSecret(stream sdsStream, node Node, secretName, baseDir string) (*stdtls.Certificate, error) {
	typeURL := secretTypeURL()
	initial := &discoveryv3.DiscoveryRequest{
		VersionInfo:   "",
		ResponseNonce: "",
		ResourceNames: []string{secretName},
		TypeUrl:       typeURL,
		Node:          &corev3.Node{Id: node.ID, Cluster: node.Cluster},
	}
	if err := stream.Send(initial); err != nil {
		return nil, fmt.Errorf("xds: sds: send initial request: %w", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("xds: sds: recv response: %w", err)
	}
	cert, verr := applyResponse(resp, secretName, baseDir)
	if verr != nil {
		// NACK: keep the prior version_info (""), set error_detail.
		nack := &discoveryv3.DiscoveryRequest{
			VersionInfo:   initial.VersionInfo,
			ResponseNonce: resp.GetNonce(),
			ResourceNames: []string{secretName},
			TypeUrl:       typeURL,
			Node:          initial.Node,
			ErrorDetail:   &statuspb.Status{Message: verr.Error()},
		}
		_ = stream.Send(nack)
		return nil, verr
	}
	// ACK: echo the accepted (version_info, nonce).
	ack := &discoveryv3.DiscoveryRequest{
		VersionInfo:   resp.GetVersionInfo(),
		ResponseNonce: resp.GetNonce(),
		ResourceNames: []string{secretName},
		TypeUrl:       typeURL,
		Node:          initial.Node,
	}
	if err := stream.Send(ack); err != nil {
		return nil, fmt.Errorf("xds: sds: send ack: %w", err)
	}
	return cert, nil
}

// applyResponse extracts resources[0] and applies it (parseSecret), returning a
// validation-classified error (wrapping errValidation) on any parse/validate
// failure. A response with no resources for the requested name is a validation
// failure (the server delivered nothing usable).
func applyResponse(resp *discoveryv3.DiscoveryResponse, secretName, baseDir string) (*stdtls.Certificate, error) {
	if len(resp.GetResources()) == 0 {
		return nil, fmt.Errorf("%w: empty resources", errValidation)
	}
	cert, err := parseSecret(resp.GetResources()[0], secretName, baseDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errValidation, err)
	}
	return cert, nil
}
```
> **Note on the name-mismatch → NACK-vs-no-op nuance (SPEC §11 Arm N):** the reference ACKs-but-applies-nothing when the response lacks the requested name (a no-op ACK, staying init-pending). At 60.1's `fetchSecret` we treat a name mismatch / missing resource as a `errValidation` (a NACK path) — the honest local classification for "the delivered secret is not usable". The 60.2 provider-level behavior (block-then-timeout when nothing usable arrives) is the same OBSERVABLE outcome (boot-fail on timeout). Document this as a 60.1 substrate simplification in PROGRESS-60.1; the `Provider` (Task 8) treats a persistent no-usable-secret as the initial-fetch-timeout path.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/xds/ -run TestFetchSecret -count=1` ⇒ PASS.

- [ ] **Step 5: Liveness break (prove the ACK-echo assertion bites)** — temporarily change the ACK to `VersionInfo: ""` (drop the echo); run `go test ./internal/xds/ -run TestFetchSecret -count=1` ⇒ the ACK-echo subtest FAILS; revert.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/xds/ && golangci-lint run ./internal/xds/... && go vet ./internal/xds/... && go build ./... && go mod tidy -diff
git add internal/xds/stream.go internal/xds/stream_test.go
git commit -m "phase 60.1 Task 4: fetchSecret — SotW initial-request/recv/ACK-NACK handshake over the sdsStream seam (D-XDS-HANDSHAKE)"
```

---

## Task 5: The `sds.<secret>.*` 5-counter stat subset — `RegisterSDSStats` + the `IsValidName` guard [TDD]

**Files:**
- Create: `internal/xds/stats.go`
- Test: `internal/xds/stats_test.go`

The dynamic per-secret stat scope (SPEC §7). `IsValidName` BEFORE `NewCounterIfAbsent` (`reference_dynamic_stat_name_charset_guard`).

- [ ] **Step 1: Write the failing tests** in `stats_test.go`:
  - **+5 delta**: `reg := stats.NewRegistry(); before := len(reg.Names())` (or the registry's count accessor — confirm the accessor name at IMPL); `RegisterSDSStats(reg, "server_cert")`; assert exactly 5 new names, each `"sds.server_cert.<suffix>"` for suffix in `{update_success, update_failure, update_rejected, update_attempt, init_fetch_timeout}`.
  - **idempotent**: calling `RegisterSDSStats(reg, "server_cert")` twice ⇒ still 5 names (NewCounterIfAbsent dedup), returns non-nil both times.
  - **invalid-name skip**: `RegisterSDSStats(reg, "bad name!")` (a name failing `IsValidName` once composed) ⇒ registers 0 counters (no panic) and returns a non-nil `*SDSStats` whose increment helpers are no-ops (nil-safe).
  - **Record dispatch**: after registering, `s.incUpdateSuccess()` increments only `sds.server_cert.update_success` (assert via the counter's value accessor).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/xds/ -run TestSDSStats -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `internal/xds/stats.go`:
```go
package xds

import (
	"fmt"

	"github.com/pgdad/envoy-go/internal/stats"
)

// SDSStats is the 5-counter sds.<secret>.* subset (SPEC §7). Registered per-secret
// at Provider construction. All fields may be nil (invalid secret name skipped the
// registration) — every increment helper is nil-safe.
type SDSStats struct {
	updateSuccess    *stats.Counter
	updateFailure    *stats.Counter
	updateRejected   *stats.Counter
	updateAttempt    *stats.Counter
	initFetchTimeout *stats.Counter
}

// RegisterSDSStats registers (idempotently) the 5 sds.<secretName>.* counters.
// The secretName segment is operator-controlled, so each composed name is checked
// with stats.IsValidName BEFORE NewCounterIfAbsent (which PANICS on an invalid
// name — reference_dynamic_stat_name_charset_guard). On an invalid name the whole
// set is skipped and a nil-populated *SDSStats (no-op increments) is returned.
func RegisterSDSStats(reg *stats.Registry, secretName string) *SDSStats {
	s := &SDSStats{}
	get := func(suffix string) *stats.Counter {
		name := fmt.Sprintf("sds.%s.%s", secretName, suffix)
		if !stats.IsValidName(name) {
			return nil
		}
		return reg.NewCounterIfAbsent(name)
	}
	s.updateSuccess = get("update_success")
	s.updateFailure = get("update_failure")
	s.updateRejected = get("update_rejected")
	s.updateAttempt = get("update_attempt")
	s.initFetchTimeout = get("init_fetch_timeout")
	return s
}

func incNil(c *stats.Counter) { if c != nil { c.Inc() } }

func (s *SDSStats) incUpdateSuccess()    { if s != nil { incNil(s.updateSuccess) } }
func (s *SDSStats) incUpdateFailure()    { if s != nil { incNil(s.updateFailure) } }
func (s *SDSStats) incUpdateRejected()   { if s != nil { incNil(s.updateRejected) } }
func (s *SDSStats) incUpdateAttempt()    { if s != nil { incNil(s.updateAttempt) } }
func (s *SDSStats) incInitFetchTimeout() { if s != nil { incNil(s.initFetchTimeout) } }
```
> **Confirm at IMPL:** the `stats.Registry` name-listing accessor (for the +5 delta assertion) + the `*stats.Counter.Inc`/value accessor names (`Value()` / `Get()` — RE-DERIVE against `internal/stats/`). The `Counter.Inc` method exists (used across the codebase); the value accessor's exact name must be confirmed.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/xds/ -run TestSDSStats -count=1` ⇒ PASS.

- [ ] **Step 5: Liveness break** — temporarily drop the `IsValidName` guard (`return reg.NewCounterIfAbsent(name)` unconditionally); run the invalid-name test `-count=1` ⇒ it PANICS/FAILS (proving the guard is load-bearing); revert.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/xds/ && golangci-lint run ./internal/xds/... && go vet ./internal/xds/... && go build ./... && go mod tidy -diff
git add internal/xds/stats.go internal/xds/stats_test.go
git commit -m "phase 60.1 Task 5: sds.<secret>.* 5-counter subset + IsValidName guard before NewCounterIfAbsent (D-XDS-STATS; reference_dynamic_stat_name_charset_guard)"
```

---

## Task 6: The `grpcclient.SDSClient` BIDI wrapper [TDD]

**Files:**
- Modify: `internal/grpcclient/grpcclient.go`
- Test: `internal/grpcclient/grpcclient_test.go`

The typed BIDI wrapper mirroring `ALSClient` (streaming, no per-call timeout). Homed in `grpcclient` to keep the `cluster.Manager` import out of `internal/xds`.

- [ ] **Step 1: Write the failing tests** in `grpcclient_test.go` (mirror the existing `NewALSClient` reject tests — RE-DERIVE their shape at IMPL):
  - `NewSDSClient(New(mgrWithUnknownCluster), "nope")` ⇒ `(nil, err)` whose message contains `unknown cluster` (the `DialContext` reject passthrough).
  - `NewSDSClient(New(mgrWithNonH2Cluster), "plain")` ⇒ `(nil, err)` containing `http2` (the non-H2 reject). (Reuse whatever cluster-manager test fixture the existing ALS/Auth reject tests use.)
  - `NewSDSClient(nil, "c")` ⇒ `(nil, err)` containing `dialer is nil` (the `dialConn` nil-guard).
  - `(*SDSClient)(nil).Close()` ⇒ `nil` (nil-receiver tolerance).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/grpcclient/ -run TestSDSClient -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** — add to `grpcclient.go` (after the `ALSClient` block), plus the import `secretv3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"`:
```go
// ----------------------------------------------------------------------------
// SDSClient — the typed SecretDiscoveryService/StreamSecrets BIDI wrapper (ADR-0278).
// ----------------------------------------------------------------------------

// SDSClient wraps a *grpc.ClientConn with the typed
// secretv3.SecretDiscoveryServiceClient stub. One *SDSClient per SDS secret
// config (cluster_name), owned by the internal/xds SecretProvider and Close()d at
// shutdown. The ALSClient precedent (ADR-0158) but BIDI — StreamSecrets is a
// bidirectional stream (Send *DiscoveryRequest / Recv *DiscoveryResponse), unlike
// ALS's client-streaming.
//
// Concurrency: a *SDSClient is safe for concurrent use — the underlying
// *grpc.ClientConn is goroutine-safe. Each StreamSecrets call opens a distinct
// bidi stream; an individual stream is NOT itself concurrency-safe and is driven
// by a single caller.
type SDSClient struct {
	connHolder
	stub secretv3.SecretDiscoveryServiceClient
}

// NewSDSClient dials the named cluster via d.DialContext and wraps the resulting
// *grpc.ClientConn in a typed SDSClient. On dial error returns (nil, err) verbatim
// (already cluster-named via DialContext's wrapping). The NewALSClient shape.
func NewSDSClient(d *Dialer, clusterName string) (*SDSClient, error) {
	conn, err := dialConn(d, "SDS client", clusterName)
	if err != nil {
		return nil, err
	}
	return &SDSClient{
		connHolder: connHolder{conn: conn},
		stub:       secretv3.NewSecretDiscoveryServiceClient(conn),
	}, nil
}

// StreamSecrets opens the bidirectional StreamSecrets RPC. The caller (the SDS
// provider) drives Send(*DiscoveryRequest) + Recv(*DiscoveryResponse); ctx bounds
// the stream lifetime (the provider applies initial_fetch_timeout via ctx).
func (c *SDSClient) StreamSecrets(ctx context.Context) (secretv3.SecretDiscoveryService_StreamSecretsClient, error) {
	if c == nil || c.stub == nil {
		return nil, errors.New("grpcclient: StreamSecrets: nil SDSClient / stub")
	}
	return c.stub.StreamSecrets(ctx)
}

// Close releases the underlying *grpc.ClientConn. Idempotent (shared connHolder
// sync.Once), the ALSClient.Close shape. A nil receiver is tolerated (returns nil).
func (c *SDSClient) Close() error {
	if c == nil {
		return nil
	}
	return c.connHolder.close()
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/grpcclient/ -run TestSDSClient -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/grpcclient/ && golangci-lint run ./internal/grpcclient/... && go vet ./internal/grpcclient/... && go build ./... && go mod tidy -diff
git add internal/grpcclient/grpcclient.go internal/grpcclient/grpcclient_test.go
git commit -m "phase 60.1 Task 6: grpcclient.SDSClient — the BIDI StreamSecrets wrapper (the ALSClient streaming precedent, ADR-0158)"
```

---

## Task 7: The driver-owned fake SDS management server — `test/helpers/sdsserver` [built + self-test]

**Files:**
- Create: `test/helpers/sdsserver/sdsserver.go`, `test/helpers/sdsserver/doc.go`, `test/helpers/sdsserver/sdsserver_test.go`

An in-process `SecretDiscoveryService` gRPC server (the `accessloggrpc` template) that delivers a configured `Secret` and records received requests. NOT a runner BackendKind (`reference_differential_grpc_receiver_driver_owned`).

- [ ] **Step 1: Write the failing self-test** in `sdsserver_test.go`:
  - `srv := New(t, WithSecret("server_cert", certPEM, keyPEM))`; dial `srv.Addr()` with a plain insecure `grpc.NewClient`; open `StreamSecrets`; `Send` an initial `DiscoveryRequest{ResourceNames:["server_cert"], TypeUrl:<Secret type_url>, Node:{Id:"n",Cluster:"c"}}`; `Recv` ⇒ a `DiscoveryResponse` whose `Resources[0]` unmarshals to a `Secret{Name:"server_cert"}` with a `tls_certificate`.
  - after the round-trip, `srv.Requests()` returns ≥1 recorded `*DiscoveryRequest` whose `Node.Id=="n"`, `Node.Cluster=="c"`, `ResourceNames==["server_cert"]` (proving the decode is non-vacuous — `reference_docker_probe_bridge_network` discipline: verify the exchange actually happened).
  - `srv.Stop()` is idempotent (call twice, no panic).

- [ ] **Step 2: Run to verify they fail** — `go test ./test/helpers/sdsserver/ -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `sdsserver.go` (the `accessloggrpc.Server` skeleton, swapped to `SecretDiscoveryService`):
```go
package sdsserver

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	secretv3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

// Server is a minimal in-process SecretDiscoveryService gRPC server that delivers
// a configured Secret on the first StreamSecrets request and records every received
// DiscoveryRequest (goroutine-safe). Driver-owned (reference_differential_grpc_receiver_driver_owned)
// — the client DIALS it; it is NOT a runner BackendKind. Plaintext h2c (no TLS).
type Server struct {
	secretv3.UnimplementedSecretDiscoveryServiceServer

	addr    string
	lis     net.Listener
	grpcSrv *grpc.Server

	secretName string
	certPEM    []byte
	keyPEM     []byte
	silent     bool // when true, never Send a response (drives the client's initial-fetch timeout)

	mu       sync.RWMutex
	requests []*discoveryv3.DiscoveryRequest

	stopOnce sync.Once
}

type Option func(*Server)

// WithSecret configures the delivered Secret{name, tls_certificate{inline PEM}}.
func WithSecret(name string, certPEM, keyPEM []byte) Option {
	return func(s *Server) { s.secretName = name; s.certPEM = certPEM; s.keyPEM = keyPEM }
}

// Silent makes the server accept the stream but never Send a response — used to
// drive the client's initial_fetch_timeout path (Provider test).
func Silent() Option { return func(s *Server) { s.silent = true } }

// New binds an ephemeral 127.0.0.1 listener + starts the server in a goroutine,
// registering t.Cleanup(Stop). Read Addr() to dial.
func New(t testing.TB, opts ...Option) *Server {
	t.Helper()
	s, err := newServer("127.0.0.1:0", opts...)
	if err != nil {
		t.Fatalf("sdsserver: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

func newServer(addr string, opts ...Option) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	s := &Server{addr: lis.Addr().String(), lis: lis, grpcSrv: grpc.NewServer()}
	for _, o := range opts {
		o(s)
	}
	secretv3.RegisterSecretDiscoveryServiceServer(s.grpcSrv, s)
	go func() { _ = s.grpcSrv.Serve(lis) }()
	return s, nil
}

func (s *Server) Addr() string { return s.addr }

// StreamSecrets records each received request and, on the first one (unless
// Silent), Sends a DiscoveryResponse carrying the configured Secret, then keeps
// draining (the client's ACK) until the stream closes.
func (s *Server) StreamSecrets(stream secretv3.SecretDiscoveryService_StreamSecretsServer) error {
	first := true
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.requests = append(s.requests, req)
		s.mu.Unlock()
		if first && !s.silent {
			first = false
			resp, berr := s.buildResponse(req.GetResourceNames())
			if berr != nil {
				return berr
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
		}
	}
}

func (s *Server) buildResponse(names []string) (*discoveryv3.DiscoveryResponse, error) {
	sec := &tlsv3.Secret{
		Name: s.secretName,
		Type: &tlsv3.Secret_TlsCertificate{TlsCertificate: &tlsv3.TlsCertificate{
			CertificateChain: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: s.certPEM}},
			PrivateKey:       &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: s.keyPEM}},
		}},
	}
	any, err := anypb.New(sec)
	if err != nil {
		return nil, err
	}
	return &discoveryv3.DiscoveryResponse{
		VersionInfo: "v1",
		Nonce:       "nonce-1",
		TypeUrl:     "type.googleapis.com/" + string(sec.ProtoReflect().Descriptor().FullName()),
		Resources:   []*anypb.Any{any},
	}, nil
}

// Requests returns a snapshot copy of the received DiscoveryRequests (arrival order).
func (s *Server) Requests() []*discoveryv3.DiscoveryRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*discoveryv3.DiscoveryRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

// Stop GracefulStops the server; idempotent via sync.Once.
func (s *Server) Stop() { s.stopOnce.Do(s.grpcSrv.GracefulStop) }
```
And `doc.go` (a ~10-line package doc mirroring `accessloggrpc/doc.go`).

- [ ] **Step 4: Run to verify they pass** — `go test ./test/helpers/sdsserver/ -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/sdsserver/ && golangci-lint run ./test/helpers/sdsserver/... && go vet ./test/helpers/sdsserver/... && go build ./... && go mod tidy -diff
git add test/helpers/sdsserver/
git commit -m "phase 60.1 Task 7: test/helpers/sdsserver — driver-owned fake SDS management server (accessloggrpc precedent; reference_differential_grpc_receiver_driver_owned)"
```

---

## Task 8: The `SecretProvider` seam — `Provider.FetchInitialCertificate` (blocking, `initial_fetch_timeout`) [TDD]

**Files:**
- Create: `internal/xds/provider.go`
- Test: `internal/xds/provider_test.go`

The public seam `internal/tls` (60.2) blocks on. Integration-tested against the real `test/helpers/sdsserver`.

- [ ] **Step 1: Write the failing tests** in `provider_test.go`:
  - **success**: stand up `sdsserver.New(t, WithSecret("server_cert", certPEM, keyPEM))`; build a `Provider` whose `streamOpener` dials the server (see the opener note below) with `Node{ID:"n",Cluster:"c"}`, `timeout: 2*time.Second`, a fresh `SDSStats`; `cert, err := p.FetchInitialCertificate(ctx, "server_cert")` ⇒ non-nil cert, nil err; assert `sds.server_cert.update_attempt == 1` + `update_success == 1`; assert `srv.Requests()[0].Node.Id == "n"` (node populated end-to-end).
  - **timeout**: `sdsserver.New(t, WithSecret(...), Silent())` (never responds); `Provider` with `timeout: 200*time.Millisecond`; `FetchInitialCertificate` ⇒ non-nil error (context deadline) + `init_fetch_timeout == 1`.
  - **mgmt-down**: point the opener at a closed/never-bound address (dial fails on the stream RPC); `FetchInitialCertificate` ⇒ non-nil error + `update_failure == 1`.
  - **rejected**: `sdsserver.New(t, WithSecret("WRONG_NAME", …))` (delivers a mismatched name); `FetchInitialCertificate(ctx, "server_cert")` with `timeout: 500ms` ⇒ non-nil error + `update_rejected == 1` (the `errValidation` path).

> **The `streamOpener` for tests:** `Provider` depends on a small `streamOpener interface { StreamSecrets(ctx) (sdsStream, error) }`. In tests, inject a fake opener that dials `srv.Addr()` via a plain `grpc.NewClient(..., insecure)` + `secretv3.NewSecretDiscoveryServiceClient(conn).StreamSecrets(ctx)` and adapts the returned client stream to `sdsStream` (it already has `Send`/`Recv`). In production (60.2), the opener wraps `grpcclient.NewSDSClient(dialer, cluster).StreamSecrets` — a `grpcSDSOpener` adapter defined in Task 8 too (so production has a real opener), but NOT wired into boot at 60.1.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/xds/ -run TestProvider -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `internal/xds/provider.go`:
```go
package xds

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/pgdad/envoy-go/internal/grpcclient"
)

// SecretProvider is the blocking seam internal/tls (60.2) uses to obtain an
// SDS-delivered downstream server certificate at listener construction. Bounded by
// initial_fetch_timeout. INITIAL-FETCH only — no rotation.
type SecretProvider interface {
	FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error)
}

// streamOpener opens one SotW SDS stream. Abstracted so unit tests inject an
// in-process opener and production wraps grpcclient.SDSClient.
type streamOpener interface {
	StreamSecrets(ctx context.Context) (sdsStream, error)
}

// Provider is the concrete SecretProvider for one SDS secret config.
type Provider struct {
	opener  streamOpener
	node    Node
	baseDir string
	timeout time.Duration
	stats   *SDSStats
}

// NewProvider builds a Provider. timeout is initial_fetch_timeout (default 15s —
// the caller passes the config value or the default). stats may be nil (no-op).
func NewProvider(opener streamOpener, node Node, baseDir string, timeout time.Duration, stats *SDSStats) *Provider {
	return &Provider{opener: opener, node: node, baseDir: baseDir, timeout: timeout, stats: stats}
}

// FetchInitialCertificate opens the SDS stream, runs one SotW fetch for secretName,
// and returns the built leaf — blocking up to initial_fetch_timeout. On timeout /
// mgmt-unreachable / validation failure it returns a classified error and
// increments the matching sds.* counter. (At 60.2 a returned error boot-FAILS the
// listener — envoy-go's documented DEPARTURE from the reference's serve-cert-less.)
func (p *Provider) FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error) {
	p.stats.incUpdateAttempt()
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	stream, err := p.opener.StreamSecrets(ctx)
	if err != nil {
		p.stats.incUpdateFailure()
		return nil, fmt.Errorf("xds: sds: secret %q: open stream: %w", secretName, err)
	}
	cert, err := fetchSecret(stream, p.node, secretName, p.baseDir)
	if err != nil {
		switch {
		case errors.Is(err, errValidation):
			p.stats.incUpdateRejected()
			return nil, err
		case ctx.Err() != nil: // deadline / cancel during recv
			p.stats.incInitFetchTimeout()
			return nil, fmt.Errorf("xds: sds: secret %q: initial fetch timed out after %s: %w", secretName, p.timeout, ctx.Err())
		default:
			p.stats.incUpdateFailure()
			return nil, err
		}
	}
	p.stats.incUpdateSuccess()
	return cert, nil
}

// grpcSDSOpener adapts a *grpcclient.SDSClient to streamOpener (the production
// opener — built at 60.2 boot from the dialer; defined here so the substrate is
// complete, but NOT wired into any boot path at 60.1).
type grpcSDSOpener struct{ client *grpcclient.SDSClient }

// NewGRPCOpener wraps a *grpcclient.SDSClient as a streamOpener.
func NewGRPCOpener(client *grpcclient.SDSClient) streamOpener { return &grpcSDSOpener{client: client} }

func (o *grpcSDSOpener) StreamSecrets(ctx context.Context) (sdsStream, error) {
	s, err := o.client.StreamSecrets(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil // *…_StreamSecretsClient satisfies sdsStream (Send/Recv)
}
```
> **Note:** the `errValidation`-vs-timeout ordering matters — check `errValidation` FIRST (a mismatched-name reject is a rejection even if the deadline also fired). Confirm the `secretv3.SecretDiscoveryService_StreamSecretsClient` satisfies `sdsStream` structurally (it has `Send(*DiscoveryRequest) error` + `Recv() (*DiscoveryResponse, error)` — verified in the proto facts). If Go's structural typing needs an explicit adapter (it should not — the method set matches), add a thin wrapper.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/xds/ -run TestProvider -count=1` ⇒ PASS.

- [ ] **Step 5: Liveness break** — temporarily swap the timeout-case and failure-case stat increments (`incInitFetchTimeout` ↔ `incUpdateFailure`); run the timeout + mgmt-down subtests `-count=1` ⇒ both FAIL (proving each asserts its OWN counter); revert.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/xds/ && golangci-lint run ./internal/xds/... && go vet ./internal/xds/... && go build ./... && go mod tidy -diff
git add internal/xds/provider.go internal/xds/provider_test.go
git commit -m "phase 60.1 Task 8: SecretProvider + Provider.FetchInitialCertificate (blocking, initial_fetch_timeout, classified sds.* stats) proven vs the fake server (D-XDS-INITIAL-FETCH)"
```

---

## Task 9: `FuzzDiscoveryResponseParse` — the untrusted-wire parse fuzzer [TDD]

**Files:**
- Create: `internal/xds/fuzz_test.go`

The `DiscoveryResponse`→`Secret`→`X509KeyPair` path is a new untrusted-wire boundary (`reference_fuzzer_count_docs_drift`: 54 → 55).

- [ ] **Step 1: Confirm the pre-count** — `grep -rh '^func Fuzz' --include='*.go' . | wc -l` ⇒ **54**.

- [ ] **Step 2: Write `FuzzDiscoveryResponseParse`** — fuzz over arbitrary bytes fed as a `DiscoveryResponse`'s single resource + arbitrary requested-name; assert no panic:
```go
package xds

import (
	"testing"

	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func FuzzDiscoveryResponseParse(f *testing.F) {
	// A valid single-Secret response (bytes) + malformed variants seed the corpus.
	f.Add([]byte{}, "server_cert")                 // empty
	f.Add([]byte("garbage"), "server_cert")        // non-proto
	f.Add(mustValidSecretAnyBytes(f), "server_cert") // a real Secret Any (helper)
	f.Fuzz(func(t *testing.T, resourceBytes []byte, wantName string) {
		// Wrap arbitrary bytes as an Any of the Secret type_url and run the
		// applier path (applyResponse -> parseSecret). Must never panic.
		resp := &discoveryv3.DiscoveryResponse{
			TypeUrl:   secretTypeURL(),
			Resources: []*anypb.Any{{TypeUrl: secretTypeURL(), Value: resourceBytes}},
		}
		_, _ = applyResponse(resp, wantName, "") // ignore the (expected) errors; assert no panic
		_ = proto.Marshal(resp)                  // exercise the round-trip too
	})
}
```
(Define `mustValidSecretAnyBytes(f)` = marshal a real `Secret{Name:"server_cert", tls_certificate{inline PEM}}` Any and return its `.Value` bytes.)

- [ ] **Step 3: Run + fuzz** — `go test ./internal/xds/ -run FuzzDiscoveryResponseParse -count=1` ⇒ PASS; then `go test ./internal/xds/ -fuzz FuzzDiscoveryResponseParse -fuzztime 30s -count=1` ⇒ no crashers.

- [ ] **Step 4: Confirm the post-count** — `grep -rh '^func Fuzz' --include='*.go' . | wc -l` ⇒ **55**; record in PROGRESS-60.1.md.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/xds/ && golangci-lint run ./internal/xds/... && go build ./... && go mod tidy -diff
git add internal/xds/fuzz_test.go
git commit -m "phase 60.1 Task 9: FuzzDiscoveryResponseParse — the untrusted DiscoveryResponse->Secret wire boundary (fuzzers 54->55; D-XDS-FUZZER)"
```

---

## Task 10: Completion — ADR-0278 body + STATE/ROADMAP/PROGRESS + the six-gate verify

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0278 §Decision/§Consequences), `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/60-xds-sds-server-cert/PROGRESS-60.1.md`

- [ ] **Step 1: Write the ADR-0278 §Decision + §Consequences body** (the §Context is already drafted at SPEC §13 — carry it verbatim into DECISIONS.md and append the Decision/Consequences). §Decision: ONE package `internal/xds` (no `internal/tls` import — the cycle guard; own `dataSourceBytes`); the `grpcclient.SDSClient` BIDI wrapper (the ALSClient precedent); the `fetchSecret` SotW loop over the `sdsStream` seam; the `parseSecret` Any→Secret→`*tls.Certificate` applier; the blocking `SecretProvider.FetchInitialCertificate` bounded by `initial_fetch_timeout`; the 5-counter `sds.<secret>.*` dynamic subset (`IsValidName`-guarded); the driver-owned `test/helpers/sdsserver`; `FuzzDiscoveryResponseParse`. §Consequences: +1 production package + 1 test helper; +1 fuzzer (55); +0 static stat surface (5 dynamic per-secret counters); +0 go.mod modules; +0 fixtures/BackendKind; the 60.2 leg (ADR-0280) plumbs the provider into `internal/tls` + the differential + the boot-fail-on-timeout DEPARTURE; row 60 stays `in-progress` until 60.2 (ADR-0106).

- [ ] **Step 2: Update STATE.md** — active-phase header → `phase 60.1 (xds-sds-stream-substrate) IMPL done`; DECISIONS tail `ADR-0277 → ADR-0278`; fuzzers `54 → 55`; new packages `+1` (`internal/xds`); NEXT = the phase-60.2 PLAN (`xds-sds-tls-cert-apply`). Record the landed task commit shas.

- [ ] **Step 3: Update ROADMAP.md** — row 60 STAYS `in-progress` (do NOT flip `done` — 60.2 pending, ADR-0106); NO deferred-sentence change (the xDS family stays open unchanged; the sentence is authored at the 60.2 row-completing IMPL). Add the `xDS-family row` slug note if not already present (sentinel check-(3) — confirm the family row summary names the slug so check-(3) stays honest).

- [ ] **Step 4: Finalize PROGRESS-60.1.md** — check off all tasks; record the exit counts (stat surface **1201 +0 static / +5 dynamic proven**; fixtures **104**; fuzzers **55**; BackendKind **38**; DECISIONS **ADR-0278**; new packages **+1** prod + **+1** test; go.mod **+0**) + the six-gate outputs.

- [ ] **Step 5: The six-gate verify** (record every output in PROGRESS-60.1.md):
```bash
gofmt -l internal/ test/ cmd/                          # expect: no output
golangci-lint run ./...                                # expect: exit 0
go vet ./...                                           # expect: clean
go build ./...                                         # expect: clean
go mod tidy -diff                                      # expect: EMPTY (0 new modules)
go test -race -count=1 ./internal/xds/... ./internal/grpcclient/... ./test/helpers/sdsserver/...   # expect: all ok
grep -rh '^func Fuzz' --include='*.go' . | wc -l       # expect: 55
```
> **NO full differential run at 60.1** — there is no new fixture and no production request-path change (the `internal/xds` package is not imported by any boot path yet). The full `test/differential` suite is a 60.2 gate. (Optionally spot-run `go build ./...` confirms the whole tree still compiles with the new package present.)

- [ ] **Step 6: Commit**
```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md docs/envoy-go/phases/60-xds-sds-server-cert/PROGRESS-60.1.md
git commit -m "phase 60.1 Task 10: ADR-0278 body + STATE/ROADMAP/PROGRESS + six-gate GREEN (row 60 stays in-progress; NEXT = 60.2 PLAN)"
```

---

## Self-Review (run against SPEC-60 §3.1/§3.3/§7/§10 with fresh eyes)

**1. Spec coverage (SPEC §3.1 the 60.1 substrate scope):**
- ✅ NEW `internal/xds` discovery-stream client dialing via `grpcclient.Dialer` → Tasks 2–4, 6, 8 (the wrapper dials via `Dialer`; the opener wraps it).
- ✅ dedicated `StreamSecrets` BIDI stream → Task 6 (`SDSClient.StreamSecrets`).
- ✅ SotW `DiscoveryRequest`/`DiscoveryResponse` ACK/NACK version+nonce loop → Task 4 (`fetchSecret`).
- ✅ parse a `Secret` → `*crypto/tls.Certificate` → Task 3 (`parseSecret`).
- ✅ the blocking `SecretProvider.FetchInitialCertificate` bounded by `initial_fetch_timeout` (default 15s) → Task 8.
- ✅ the `sds.*` 5-counter subset under `sds.<secret_name>.` with `IsValidName` guard → Task 5.
- ✅ proven at UNIT level against an in-process fake SDS server → Task 7 (`test/helpers/sdsserver`) + Task 8 (integration).
- ✅ NEW `FuzzDiscoveryResponseParse` (fuzzers 54 → 55) → Task 9.
- ✅ NO TLS integration + NO differential/fixture at 60.1 → enforced in Global Constraints; Task 10 skips the differential gate.

**2. Placeholder scan:** every code step carries actual code; the two IMPL-time re-derivations that are genuinely unknowable at PLAN time are flagged explicitly (the `stats.Registry` name-listing accessor + the `Counter` value accessor in Task 5; the existing `NewALSClient` reject-test cluster-manager fixture in Task 6) — these are "confirm the accessor name," not "figure out the design." No `TODO`/`handle edge cases`/`similar to Task N`.

**3. Type consistency:** `sdsStream` (Task 4) is consumed by `fetchSecret` (Task 4), `Provider` (Task 8), and `grpcSDSOpener` (Task 8) — same 2-method shape throughout. `SDSStats` increment helpers (`incUpdateAttempt`/`incUpdateSuccess`/`incUpdateFailure`/`incUpdateRejected`/`incInitFetchTimeout`, Task 5) match their call sites in `Provider.FetchInitialCertificate` (Task 8). `secretTypeURL()` (Task 2) is used by `parseSecret` (Task 3), `fetchSecret` (Task 4), and `FuzzDiscoveryResponseParse` (Task 9) — one definition. `errValidation` (Task 4) is checked via `errors.Is` in `Provider` (Task 8). `Node{ID,Cluster}` (Task 4) flows into `NewProvider` (Task 8) and the `corev3.Node` build (Task 4). `NewSDSClient`/`StreamSecrets`/`Close` (Task 6) match `grpcSDSOpener` (Task 8).

**Coverage gaps found + resolved:** none — the four SPEC-60 60.1-scope sections (§3.1 client, §3.3 read-for-context-only, §7 stats, §10 test plan tasks 1–8) all map to tasks. The SPEC §10 "task 8: ADR-0278 body + STATE + verify" maps to this PLAN's Task 10; the SPEC's 8-task sketch expands to 10 bite-sized tasks (the applier + the stats + the fuzzer split out for independent test cycles).

---

## Execution Handoff

**Plan complete and saved to `docs/envoy-go/phases/60-xds-sds-server-cert/PLAN-60.1.md`.** Per the project discipline (`feedback_execution_style`, `feedback_subagent_autocommit_claudemd`, `feedback_subagents_no_push`), the phase-60.1 IMPL runs **subagent-driven** (`superpowers:subagent-driven-development`): a fresh subagent per task in the worktree `.worktrees/phase-60.1-impl` (branch `phase-60-xds-sds-stream-substrate-impl`), the controller reviewing + verifying each task's commit between tasks (re-deriving citations per `feedback_brief_citations_not_evidence`, cleaning any leak files, watching for HEAD-detach + stale-path targeting), then squashing + pushing at stage-close.
