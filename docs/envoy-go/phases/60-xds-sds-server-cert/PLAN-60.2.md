# Phase 60.2 Implementation Plan — `xds-sds-tls-cert-apply`: plumb the 60.1 `internal/xds` SDS substrate into the downstream TLS context (the one-arm `tls/config.go` reject lift + the boot-side config seam + the initial-fetch BLOCK-then-BOOT-FAIL departure + the six-plus-one strict-reject roster + the driver-owned-SDS differential) — the OBSERVABLE end-to-end leg that flips ROADMAP row 60 `done`. The applier leg of the confirmed 60.1/60.2 split (ADR-0280).

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). Subagents auto-commit per CLAUDE.md — the controller verifies each commit, cleans leak files, squashes at stage-close, re-runs the suite on the frozen HEAD (`feedback_subagent_autocommit_claudemd`).

**Goal:** Take the unit-proven 60.1 `internal/xds` SDS discovery-stream substrate (a `SecretProvider` that dials a static SDS cluster, runs the SotW handshake, and returns a built `*crypto/tls.Certificate`) and make it OBSERVABLE end-to-end: an operator configures a `DownstreamTlsContext` whose `common_tls_context.tls_certificate_sds_secret_configs[0]` names a Secret + a gRPC `api_config_source`, and envoy-go dials the SDS management server at boot, blocks on the first Secret (bounded by `initial_fetch_timeout`), builds the leaf, and serves downstream TLS with it — proven by a cross-side differential that observes the served leaf (serial + SAN) identical between the reference and envoy-go.

**Architecture:** ONE new pure helper in `internal/xds` (`ParseSDSConfig` — validates a `SdsSecretConfig` and extracts `(secretName, clusterName, timeout)`, carrying the six ADR-0080-distinct ConfigSource/structural rejects; NO new package, NO grpcclient/cluster/tls import — the cycle guard holds). The `internal/tls/config.go:153` wholesale reject becomes a downstream-gated LIFT that consumes a threaded `xds.SecretProvider`. The provider is BUILT ONCE at boot by a new `boot.NewSDSProvider` (mirroring `boot.NewTracingProvider` — it pre-scans the bootstrap listeners for the SDS-bound downstream TLS context, enforces the node boot-requirement, and constructs `xds.NewProvider(xdsgrpc.NewOpener(grpcclient.NewSDSClient(dialer, cluster)), …)`), then THREADED as an `xds.SecretProvider` value through `boot.Construct` → the listener manager → `NewDownstreamConfig`. The differential reuses the 60.1-built `test/helpers/sdsserver` (extended with a `0.0.0.0`-binding constructor) reachable by BOTH proxies (the containerized reference via `host.docker.internal` STRICT_DNS; the host subject via `127.0.0.1`), exactly the `0089-stats-sink-metrics-service` driver-owned-gRPC-server precedent.

**Tech Stack:** Go; the EXISTING `internal/xds` (add `ParseSDSConfig`) + `internal/xds/xdsgrpc` (`NewOpener`) + `internal/grpcclient` (`NewSDSClient`) 60.1 API; `internal/tls` (the lift); `internal/listener` + `internal/boot` + `cmd/envoy-go` + `validate` (the config seam); `crypto/tls` + `crypto/x509` (the served-leaf accessor); the go-control-plane `extensions/transport_sockets/tls/v3` (`SdsSecretConfig`), `config/core/v3` (`ConfigSource`/`ApiConfigSource`/`ApiVersion`/`GrpcService`), `service/discovery/v3` protos (resolved at `go-control-plane/envoy v1.32.4`); `test/helpers/sdsserver` + `test/helpers` (TLS); the `test/differential` fixture harness. ZERO new go.mod modules; ZERO new production Go packages (60.1 built them).

---

## Global Constraints

- **One stage = 60.2 only (the TLS-apply leg).** This PLAN is the 60.2 IMPL decomposition. When it lands at its six-gate, ROADMAP **row 60 flips `in-progress` → `done`** (ADR-0106, `reference_roadmap_split_phase_row_done` — BOTH legs now landed: 60.1 substrate + 60.2 apply). The xDS FAMILY STAYS OPEN.
- **The cycle guard is LOAD-BEARING and must stay intact (ADR-0278).** `internal/xds` proper imports NEITHER `internal/grpcclient` NOR `internal/cluster` NOR `internal/tls`. At 60.2 `internal/tls` imports `internal/xds` ONLY for the `xds.SecretProvider` interface type — acyclic BECAUSE `xds` is grpcclient/cluster/tls-free. The provider VALUE is constructed in `internal/boot`/`main.go` via `xdsgrpc.NewOpener(grpcclient.NewSDSClient(dialer, cluster))` and threaded DOWN as an `xds.SecretProvider`. Do **NOT** add a `grpcclient`/`cluster` import to `internal/xds` or a `grpcclient`/`xdsgrpc` import to `internal/tls`. VERIFY with `go list -deps` at every edit site (see Task 3/5 gates). The verified-intact baseline at PLAN time: `go list -deps ./internal/tls` shows none of boot/listener/grpcclient/cluster/xds; `go list -deps ./internal/xds` shows none of tls/grpcclient/cluster/listener/boot.
- **NO differential dir except `0103-xds-sds-server-cert` (fixtures 104 → 105).** The timeout / mgmt-down / secret-not-found cases are SUBJECT-side UNIT tests (the reference serves cert-less while envoy-go boot-fails — divergent, so NOT cross-side comparable, `reference_differential_fixture_dispatch_constraint`: one dir = one runner branch). BackendKind STAYS **38** (the SDS server is DIALED, not a runner backend — `reference_differential_grpc_receiver_driver_owned`).
- **ZERO new go.mod modules; ZERO new production Go packages.** All protos resolve at the present `go-control-plane/envoy v1.32.4`. Every code task's build gate includes `go mod tidy -diff` (expect EMPTY).
- **TDD (`superpowers:test-driven-development`):** every code task is failing-test → run-fail → minimal-impl → run-pass → commit. NO production code without a failing test first.
- **Per-task gates (`feedback_pertask_gofmt_lint`):** every code task ends with `gofmt -l` (expect empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. Do NOT skip gofmt.
- **Worktree hygiene (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`):** subagents write to the WORKTREE path (`.worktrees/phase-60.2-impl/…`); the controller verifies `git -C <main-checkout> status` stays clean after each task and the worktree branch is unchanged (no detached HEAD). Pin worktree-relative paths in every dispatch.
- **Commit locally only (`feedback_subagents_no_push`):** subagents NEVER push; the controller squashes + pushes at stage-close.
- **Break protocol (`reference_differential_break_protocol_count1`):** every deliberate-break liveness verification runs the fixture with `-count=1 -run 'TestDifferential/0103-xds-sds-server-cert'` (go-test caching serves a stale PASS otherwise, `reference_differential_run_selector`: the FULL subtest name, never a bare `0103`). When a break fires, CONFIRM WHICH assertion failed (`reference_deliberate_break_wrong_assertion` — a break can abort earlier and mask the intended one; ensure the break leaves the TLS handshake intact so `CompareBytes` over the served-leaf observable is what fires, not a handshake/boot error).
- **Distinct reject substrings (ADR-0080).** All seven reject arms (§ below) carry ADR-0080-distinct substrings. Adopt the reference's wire grammar verbatim where cross-side (`reference_wire_format_both_sides_see_same_bytes`); the rejects themselves are documented envoy-go-strict DEPARTURES (the reference SUPPORTS these forms).
- **ADR body lands at THIS IMPL (ADR-0044):** ADR-0280 §Decision/§Consequences are authored at this 60.2 IMPL (the §Context frame re-uses SPEC-60 §13). DECISIONS tail **ADR-0278 → ADR-0280** (ADR-0279 is reserved for HTTP/3 phase 61); next-free after this IMPL is **ADR-0281**.
- **Fuzzers 55, unchanged.** 60.1 added `FuzzDiscoveryResponseParse` (fuzzers 54 → 55). 60.2 adds NO fuzzer (`ParseSDSConfig` operates on already-typed protos, not untrusted wire bytes — the untrusted-wire boundary was the 60.1 `DiscoveryResponse` parse). Re-verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l` == 55 at completion (`reference_fuzzer_count_docs_drift`).

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. At 60.1 the `internal/xds` SDS substrate was built and unit-proven; at 60.2 you make it OBSERVABLE by wiring it into the downstream TLS build and adding the cross-side differential. Today the proxy is otherwise 100%-static-bootstrap: TLS certificates materialize ONCE at boot from inline/filename bytes (`internal/tls/config.go`), and the SDS-bound cert path is REJECTED wholesale (`internal/tls/config.go:153`). You lift ONLY that one reject (downstream side), leaving the bootstrap `dynamic_resources` reject and the whole static boot model untouched.

**The 60.1 substrate you consume (VERIFIED against the frozen 60.1 IMPL HEAD `f72ebd5c` — RE-VERIFY at IMPL, `feedback_brief_citations_not_evidence`):**
- `internal/xds/provider.go`: `type SecretProvider interface { FetchInitialCertificate(ctx context.Context, secretName string) (*crypto/tls.Certificate, error) }`; `type StreamOpener interface { StreamSecrets(ctx) (Stream, error) }`; `type Stream interface { Send(*discoveryv3.DiscoveryRequest) error; Recv() (*discoveryv3.DiscoveryResponse, error) }`; `type Provider struct{…}`; `func NewProvider(opener StreamOpener, node Node, baseDir string, timeout time.Duration, stats *SDSStats) *Provider`. `(*Provider).FetchInitialCertificate` opens the stream, runs `fetchSecret`, classifies the error (validation → `update_rejected`; deadline → `init_fetch_timeout` + a `"initial fetch timed out after %s"` wrap; else → `update_failure`) and returns the leaf on success.
- `internal/xds/stream.go`: `type Node struct { ID, Cluster string }`.
- `internal/xds/stats.go`: `type SDSStats struct{…}`; `func RegisterSDSStats(reg *stats.Registry, secretName string) *SDSStats` (the 5-counter `sds.<secret>.*` dynamic subset, `IsValidName`-guarded, nil-safe).
- `internal/xds/xdsgrpc/opener.go`: `func NewOpener(client *grpcclient.SDSClient) xds.StreamOpener` — the BOOT-SIDE adapter that carries the `grpcclient` edge OUT of `internal/xds` (this is what keeps `tls → xds` acyclic).
- `internal/grpcclient/grpcclient.go:349+`: `type SDSClient struct{…}`; `func NewSDSClient(d *Dialer, clusterName string) (*SDSClient, error)` (dials via `d.DialContext`); `func (c *SDSClient) StreamSecrets(ctx) (secretv3.SecretDiscoveryService_StreamSecretsClient, error)`; `func (c *SDSClient) Close() error`.
- `test/helpers/sdsserver/sdsserver.go`: the driver-owned fake SDS server. `func New(t testing.TB, opts ...Option) *Server` (binds `127.0.0.1:0`); `WithSecret(name, certPEM, keyPEM)`; `Silent()`; `Addr()`; `Requests()`; `Stop()`. **It has NO `0.0.0.0`-binding constructor yet — you add `NewAtAddr` in Task 6** (the differential's containerized reference cannot reach `127.0.0.1`).

**The config-seam design (the biggest scope risk — DISPOSED as MANAGEABLE, SPEC §3.2; the precedent is `boot.NewTracingProvider`).** The tracing provider (`internal/boot/boot.go:96` `NewTracingProvider(dialer, httpClient, cm, registry)`) is built ONCE in `main.go` from the dialer and threaded through `boot.Construct` into the listener build, where it is consulted per-listener. SDS mirrors this EXACTLY, with one difference: `xds.Provider` is bound to ONE cluster at construction (its opener wraps one `SDSClient`), so `boot.NewSDSProvider` must DISCOVER the SDS cluster before building the provider. It does so by pre-scanning the bootstrap's static listeners for the (single, MVP) downstream SDS-bound TLS context — this pre-scan is the config-seam's cost, and it is bounded: it walks `bs.Proto.GetStaticResources().GetListeners()` → each `filter_chains[].transport_socket` (+ `default_filter_chain`) → unmarshals a `DownstreamTlsContext` → reads `common_tls_context.tls_certificate_sds_secret_configs`. It enforces the node boot-requirement (arm 7) and calls `xds.ParseSDSConfig` to extract `(secret, cluster, timeout)`, then builds `xds.NewProvider(xdsgrpc.NewOpener(grpcclient.NewSDSClient(dialer, cluster)), node, baseDir, timeout, xds.RegisterSDSStats(registry, secret))`. If no SDS is configured it returns `(nil, nil)` — a nil provider threads harmlessly (the lift only engages `if len(sdsConfigs) > 0`).

**The MVP cap (documented, defensible):** at most ONE `SdsSecretConfig` across the whole bootstrap (the differential has exactly one). `ParseSDSConfig` rejects `len(configs) > 1`; `NewSDSProvider` rejects a SECOND distinct SDS-bound listener context. A later rotation/multi-secret row lifts this. This matches the `xds.Provider`-bound-to-one-cluster shape of the 60.1 API.

**The double-parse (acknowledged, harmless).** `boot.NewSDSProvider` calls `ParseSDSConfig` to get the CLUSTER (to dial); `commonTLSContextToConfig`'s lift calls `ParseSDSConfig` again to get the SECRET NAME (to fetch) + to apply the arm rejects. Both run the SAME pure function over the SAME config and agree. This keeps the reject roster authoritative at the TLS-build site (where `config_test.go` exercises it directly with a fake provider) while letting boot discover the cluster.

**The reject roster (SPEC §6; all ADR-0080-distinct):**
- Arms 1–4, 8, 9 (ConfigSource/structural) live in **`xds.ParseSDSConfig`** (`"xds: sds: …"` prefix): (1) `ads`-sourced, (2) `self`-sourced, (3) `DELTA_GRPC` api_type, (4) `google_grpc` transport, (8) empty `SdsSecretConfig.name`, (9) missing `sds_config`; plus the MVP multi-config cap and the V3-required checks.
- Arm 5 (downstream `validation_context_sds_secret_config`) — the EXISTING `tls/config.go:158-159` reject, UNCHANGED.
- Arm 6 (upstream `tls_certificate_sds_secret_configs`) — the SAME `tls: %s: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03` string, now GATED on `side != "downstream"` (byte-identical for upstream; downstream lifts).
- Arm 7 (NODE boot-requirement) lives in **`boot.NewSDSProvider`** (`"xds: sds: node.id and node.cluster are required for SDS"`): SDS configured while bootstrap `node.id`/`node.cluster` empty ⇒ boot-fail (PARITY with the reference's hard boot-fail, SPEC §11 Arm A-pre; the `parseStatsdTCPArm` precedent at `internal/bootstrap/bootstrap.go:657` is the exact node-check pattern).
- Arm 10 (runtime NACK on a non-`tls_certificate` Secret) is ALREADY in the 60.1 `applyResponse`/`parseSecret` — no 60.2 work.

**The initial-fetch BLOCK-then-BOOT-FAIL departure (SPEC §3.4).** `FetchInitialCertificate` already blocks bounded by `initial_fetch_timeout` (default 15s) and returns a classified error on timeout/mgmt-down (60.1). At 60.2 the lift PROPAGATES that error, so the listener build FAILS and boot FAILS. This is a documented DEPARTURE: the reference instead starts workers cert-less and rejects every handshake (SPEC §11 Arm B). envoy-go boot-failing is strictly safer and matches its synchronous boot model. Proven by a `config_test.go` unit test (a fake provider returning the timeout error → `NewDownstreamConfig` returns an error containing `initial fetch timed out`), NOT a differential dir.

**The differential (SPEC §8; the `0089` precedent).** ONE new dir `test/fixtures/0103-xds-sds-server-cert`. Both proxies configure a downstream TLS listener whose cert is SDS-delivered by a driver-owned `sdsserver` (one per side, bound `0.0.0.0:<preAllocatedPort>`; reference dials `host.docker.internal:<refPort>` via STRICT_DNS + the harness's `ExtraHosts: host.docker.internal:host-gateway`; subject dials `127.0.0.1:<subjPort>`). Both servers deliver the SAME committed self-signed leaf (a distinctive serial + SAN). The driver opens a TLS client to each proxy, captures the served leaf `PeerCertificates[0]`, and RETURNS a formatted `serial=…\nsan=…` observable; the runner's Step-7 `CompareBytes` enforces it byte-identical cross-side — no new asserter/runner change (the idiomatic DriveReference/DriveSubject + CompareBytes cross-side capture; SubjectAsserter would NOT run on the cross-side path, `reference_differential_asserter_dispatch`). Prove the assertion LIVE by temporarily configuring the two per-side servers with DIFFERENT leaves (different serials) → `CompareBytes` fails on the `serial=` line (`reference_differential_break_protocol_count1` / `reference_deliberate_break_wrong_assertion`).

### Key source seams (RE-DERIVED at PLAN time against master `f72ebd5c` — re-confirm line numbers before editing; the 60.1 IMPL may have shifted them)

- **`internal/tls/config.go`** — `func NewDownstreamConfig(ts *corev3.TransportSocket, baseDir string) (*DownstreamConfig, error)` (`:34`); `func NewUpstreamConfig(…)` (`:79`); `func commonTLSContextToConfig(c *tlsv3.CommonTlsContext, baseDir, side string) (*stdtls.Config, error)` (`:149`); the wholesale SDS reject at **`:153-155`** (`if len(c.GetTlsCertificateSdsSecretConfigs()) > 0 { return nil, fmt.Errorf("tls: %s: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03", side) }`); the `validation_context_sds` reject `:158-159` (arm 5, KEEP); the static cert loop `:181-198` appending to `cfg.Certificates`; `cfg := &stdtls.Config{}` at `:179`; the downstream empty-cert check `:200-202`.
- **`internal/listener/manager.go`** — `func NewManagerWithBaseDirAndAllowH2C(bs, cm, baseDir, allowH2C, registry, accessLogSinks, httpRegistry, lfRegistry, dm, httpClient, netReg) (*Manager, error)` (`:254`); the two thin wrappers `NewManager*` at `:204`/`:219` that call it; `func buildListenerRuntimeWithCtx(…, nodeServiceCluster string, netReg *network.Registry)` (`:340`, called at `:268`); the TWO `internaltls.NewDownstreamConfig(ts, baseDir)` call sites at **`:382`** and **`:457`**; the node access `nodeServiceCluster := bs.GetNode().GetCluster()` at `:264`.
- **`internal/boot/boot.go`** — `func Construct(bs, cm, baseDir, allowH2C, sinks, dm, httpClient, tracingProvider) (*listener.Manager, error)` (`:53`); the `NewManagerWithBaseDirAndAllowH2C` call at `:83`; `func NewTracingProvider(dialer, httpClient, cm, registry) *tracing.ExporterProvider` (`:96`, the PRECEDENT for `NewSDSProvider`).
- **`cmd/envoy-go/main.go`** — `dialer := grpcclient.New(cm)` (`:134`); `tracingProvider := boot.NewTracingProvider(…)` (`:150`); `minNode := &corev3.Node{Id: bs.Proto.GetNode().GetId(), Cluster: bs.Proto.GetNode().GetCluster()}` (`:155`, the node-access precedent); `boot.Construct(bs, cm, filepath.Dir(*cfgPath), *allowH2C, sinks, drainMgr, httpClient, tracingProvider)` (`:295`).
- **`validate/validate.go`** — `tracingProvider := boot.NewTracingProvider(…)` (`:46`); `boot.Construct(bs, cm, baseDir, allowH2C, nil, dm, httpClient, tracingProvider)` (`:48`). Validate passes a **nil** SDS provider (no SDS fetch in validate mode).
- **`internal/bootstrap/bootstrap.go:657`** — `node := result.Proto.GetNode(); if node.GetId() == "" || node.GetCluster() == "" { return fmt.Errorf(...) }` — the arm-7 node-check pattern to mirror.

### Proto facts (RE-DERIVED at PLAN time against `go-control-plane/envoy@v1.32.4`; re-confirm at IMPL)

- `tlsv3 "…/envoy/extensions/transport_sockets/tls/v3"`: `SdsSecretConfig.GetName() string` (`secret.pb.go:118`); `SdsSecretConfig.GetSdsConfig() *corev3.ConfigSource` (`:125`); `DownstreamTlsContext.GetCommonTlsContext()`; `CommonTlsContext.GetTlsCertificateSdsSecretConfigs() []*SdsSecretConfig`.
- `corev3 "…/envoy/config/core/v3"` (`config_source.pb.go`): `ConfigSource.GetApiConfigSource() *ApiConfigSource` (`:642`); `.GetAds() *AggregatedConfigSource` (`:649`); `.GetSelf() *SelfConfigSource` (`:656`); `.GetInitialFetchTimeout() *durationpb.Duration` (`:663`); `.GetResourceApiVersion() ApiVersion` (`:670`). `ApiConfigSource.GetApiType() ApiConfigSource_ApiType` (`:239`); `.GetTransportApiVersion() ApiVersion` (`:246`); `.GetGrpcServices() []*GrpcService` (`:260`). Enums: `ApiVersion_V3 = 2` (`:42`); `ApiConfigSource_GRPC = 2` (`:100`), `ApiConfigSource_DELTA_GRPC = 3` (`:104`), `ApiConfigSource_REST = 1` (`:98`). `GrpcService.GetEnvoyGrpc() *GrpcService_EnvoyGrpc` (`grpc_service.pb.go:96`); `.GetGoogleGrpc()` (`:103`); `GrpcService_EnvoyGrpc.GetClusterName() string` (`:215`).
- `durationpb`: `(*durationpb.Duration).AsDuration() time.Duration` (from `google.golang.org/protobuf/types/known/durationpb`).
- The `DownstreamTlsContext` transport-socket type URL (for the boot pre-scan's typed_config check): derive via `"type.googleapis.com/" + string(proto.MessageName(&tlsv3.DownstreamTlsContext{}))` (`reference_network_filter_typeurl_extensions` — never hardcode).

### Discipline (honor on EVERY task) — the memory traps that bite this row

- **`feedback_brief_citations_not_evidence`** — RE-DERIVE every `file:line` against the master tip at IMPL time; the 60.1 IMPL may have shifted the numbers cited above.
- **`reference_fatalf_makes_assertions_unreachable`** — in tests asserting multiple independent properties, `Errorf` per property; `Fatalf` only for a broken precondition.
- **`reference_differential_break_protocol_count1` / `reference_differential_run_selector` / `reference_deliberate_break_wrong_assertion`** — the `-count=1` + full-subtest-name break protocol; confirm WHICH assertion fires.
- **`reference_differential_grpc_receiver_driver_owned`** — `sdsserver` is DIALED by the proxies; NOT a BackendKind (stays 38).
- **`reference_docker_probe_bridge_network` / `reference_host_gateway_ip_docker_desktop`** — the containerized reference reaches the host SDS server via `host.docker.internal` STRICT_DNS (gRPC accepts hostnames — do NOT use `HostGatewayIP`, which is only for the hostname-rejecting UDP statsd sink).
- **`reference_dynamic_stat_name_charset_guard`** — the `sds.<secret>.*` counters (registered via `RegisterSDSStats`) already carry the `IsValidName` guard (60.1); the differential's secret name is a valid identifier.
- **`reference_roadmap_split_phase_row_done`** — row 60 flips `done` ONLY at this 60.2 IMPL (both legs landed).
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — after row 60 → `done`, re-run the sentinel check-(2) grep; the xDS deferred sentence stays EXACTLY ONE live "candidates:" match (the family stays open).

---

## Design pins settled here (the 60.2 D-question resolutions)

**CONFIG-SEAM → thread a pre-built `xds.SecretProvider` value (Design "built provider", SPEC §3.2 / router-pinned).** `boot.NewSDSProvider(dialer, bs, baseDir, registry) (xds.SecretProvider, error)` pre-scans the bootstrap listeners, enforces arm 7, and constructs the provider via `xdsgrpc.NewOpener(grpcclient.NewSDSClient(dialer, cluster))`. Threaded through `boot.Construct` → `NewManagerWithBaseDirAndAllowH2C` → `buildListenerRuntimeWithCtx` → `NewDownstreamConfig(ts, baseDir, provider)`. Rejected alternative: a lazy per-cluster resolver built at the `NewDownstreamConfig` call site (would avoid the pre-scan but contradicts the router's "thread the built `*xds.Provider`" and the 60.1 `Provider`-bound-to-one-cluster API). The pre-scan is the accepted, bounded cost.

**REJECT-HOME → arms 1–4/8/9 in `xds.ParseSDSConfig` (`xds: sds:` prefix); arm 5 unchanged in `tls/config.go`; arm 6 = the existing string gated on `side != "downstream"`; arm 7 in `boot.NewSDSProvider` (`xds: sds: node…required`); arm 10 already in the 60.1 applier.** `config_test.go` exercises arms 1–6 directly on `NewDownstreamConfig`/`NewUpstreamConfig`; arm 7 via a `NewSDSProvider` unit test.

**MVP CAP → one `SdsSecretConfig` total.** `ParseSDSConfig` rejects `len > 1`; `NewSDSProvider` rejects a second distinct SDS-bound listener context. Documented boundary (BEHAVIOR_CONTRACT + ADR-0280).

**SERVED-LEAF ASSERTION → the idiomatic DriveReference/DriveSubject + Step-7 `CompareBytes` cross-side capture** (no new asserter interface / runner change). The driver returns a `serial=<hex>\nsan=<sorted DNSNames>` observable; both sides serve the SAME SDS leaf ⇒ identical ⇒ pass. Proven live by a two-different-certs break. (Rejected: a new `ServedCertAsserter` interface + runner dispatch — more surface for no gain; `CompareBytes` runs on the cross-side path and enforces equality, which is the property.)

**VALIDATE MODE → nil SDS provider.** `validate/validate.go` passes `nil`; a validate of an SDS-bound bootstrap REJECTS at the lift's `provider == nil` arm (validate does not dial/fetch). Reasonable MVP stance (the reference's static validation likewise does not fetch SDS).

**NO new fuzzer / no new go.mod / no new production package / BackendKind 38.**

---

## File structure (decomposition locked here)

**Production (modified):**
- `internal/xds/config.go` — CREATE: `func ParseSDSConfig(configs []*tlsv3.SdsSecretConfig) (secretName, clusterName string, timeout time.Duration, err error)` (arms 1–4, 8, 9 + multi-cap + V3 checks). No new imports beyond `tlsv3`, `corev3`, `time`, `durationpb`, `fmt` — NO grpcclient/cluster/tls (cycle guard).
- `internal/tls/config.go` — MODIFY: add `provider xds.SecretProvider` param to `NewDownstreamConfig` (`:34`) + `commonTLSContextToConfig` (`:149`); replace the `:153` wholesale reject with the downstream-gated lift; `NewUpstreamConfig` (`:79`) passes `nil`.
- `internal/listener/manager.go` — MODIFY: add `sdsProvider xds.SecretProvider` param to `NewManagerWithBaseDirAndAllowH2C` (`:254`), the two wrappers (`:204`/`:219`, pass `nil`), and `buildListenerRuntimeWithCtx` (`:340`, called at `:268`); pass it to both `NewDownstreamConfig` calls (`:382`/`:457`).
- `internal/boot/boot.go` — MODIFY: add `sdsProvider xds.SecretProvider` param to `Construct` (`:53`, pass into the manager call `:83`); ADD `func NewSDSProvider(dialer *grpcclient.Dialer, bs *bootstrap.Bootstrap, baseDir string, registry *stats.Registry) (xds.SecretProvider, error)` (the pre-scan + arm 7 + provider build).
- `cmd/envoy-go/main.go` — MODIFY: build `sdsProvider, err := boot.NewSDSProvider(dialer, bs, filepath.Dir(*cfgPath), bs.Stats)` after `:150`; pass into `boot.Construct` at `:295`.
- `validate/validate.go` — MODIFY: pass `nil` for the new `Construct` SDS-provider param (`:48`).

**Test helper (modified):**
- `test/helpers/sdsserver/sdsserver.go` — MODIFY: ADD `func NewAtAddr(addr string, opts ...Option) (*Server, error)` (a `0.0.0.0`-bindable constructor with no `t.Cleanup`, mirroring `metricsservice.NewAtAddr`).
- `test/helpers/tls.go` — MODIFY: ADD `func TLSServedLeaf(ctx context.Context, addr, serverName string, rootCAs *x509.CertPool) (*x509.Certificate, error)` (handshake + return `ConnectionState().PeerCertificates[0]`).

**Test (created / modified):**
- `internal/xds/config_test.go` — CREATE: `ParseSDSConfig` accept + arms 1–4, 8, 9 + multi-cap + V3.
- `internal/tls/config_test.go` — MODIFY: SDS accept (fake provider); arms 1–6; timeout-propagation; nil-provider reject. Update all existing `NewDownstreamConfig(ts, "")` callers to `NewDownstreamConfig(ts, "", nil)`.
- `internal/tls/fuzz_test.go` — MODIFY: `NewDownstreamConfig(ts, "")` → `NewDownstreamConfig(ts, "", nil)` (`:87`).
- `internal/listener/manager_test.go`, `internal/listener/tls_handshake_negative_test.go`, `internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go` — MODIFY: add `nil` for the new manager SDS-provider param at every `NewManagerWithBaseDirAndAllowH2C(...)` call (mechanical, compiler-enumerated).
- `internal/boot/boot_test.go` (or the existing boot test file) — CREATE/MODIFY: `NewSDSProvider` no-SDS → nil; arm 7 (SDS + empty node) → reject; success → non-nil provider whose `FetchInitialCertificate` returns the fake server's cert.
- `test/helpers/sdsserver/sdsserver_test.go` — MODIFY: a `NewAtAddr` self-test.

**Fixture (created):**
- `test/fixtures/0103-xds-sds-server-cert/` — CREATE: `driver/driver.go` (+ `doc.go`), `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `pki/` (committed `leaf.pem`, `leaf.key.pem`, `ca.pem`).
- `test/differential/runner_test.go` — MODIFY: add one blank-import line `_ "github.com/pgdad/envoy-go/test/fixtures/0103-xds-sds-server-cert/driver"`.

**Docs (completion tasks):**
- `docs/envoy-go/phases/60-xds-sds-server-cert/PROGRESS-60.2.md` (scaffolded Task 1, finalized at completion).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the new xDS/SDS section.
- `docs/envoy-go/DECISIONS.md` — ADR-0280 §Decision/§Consequences.
- `docs/envoy-go/ROADMAP.md` — row 60 → `done`; the xDS deferred sentence UPDATE.
- `docs/envoy-go/STATE.md` — active-phase header flip.

---

## Task 1: Phase scaffolding — PROGRESS-60.2.md + baselines + the 60.2 design pins

**Files:**
- Create: `docs/envoy-go/phases/60-xds-sds-server-cert/PROGRESS-60.2.md`

- [ ] **Step 1: Record the baseline counts** — run and record the verbatim outputs in PROGRESS-60.2.md:
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                    # expect 104 (tail 0102-tracing-custom-tags-literal)
grep -rh '^func Fuzz' --include='*.go' . | wc -l                     # expect 55
grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1       # expect ADR-0278 (next-to-land ADR-0280; ADR-0279=HTTP/3)
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go     # BackendKind tail = 38
go list -deps ./internal/tls | grep -E 'internal/(grpcclient|cluster|xds|boot|listener)$' || echo "TLS-CLEAN"   # expect TLS-CLEAN
go list -deps ./internal/xds | grep -E 'internal/(tls|grpcclient|cluster|listener|boot)$' || echo "XDS-CLEAN"   # expect XDS-CLEAN
```
Baseline: stat surface **1201** / fixtures **104** / fuzzers **55** / BackendKind **38** / DECISIONS tail **ADR-0278** (next-to-land **ADR-0280**) / new Go packages **0** / new go.mod modules **0** / cycle guard INTACT.

- [ ] **Step 2: Write the PROGRESS-60.2.md scaffold** — a header (phase 60.2 IMPL, the SPEC-60 reference + the "60.2 TLS-apply sub-leg of the confirmed 60.1/60.2 split; row 60 flips `done` at THIS six-gate" note, the worktree branch `phase-60-xds-sds-tls-cert-apply-impl`), a task checklist mirroring this plan (Tasks 1–9), the baseline-counts block, and the anticipated exit counts: stat surface **1201 → +5 dynamic `sds.<secret>.*`** (the `sds.*` counters now register at boot under the differential's SDS config — the DYNAMIC surface materializes; the static no-SDS surface stays 1201; RE-VERIFY the count convention — the differential asserts the 5 named counters exist) / fixtures **104 → 105** (`0103-xds-sds-server-cert`) / fuzzers **55 (+0)** / BackendKind **38 (+0)** / DECISIONS **ADR-0278 → ADR-0280** / new Go packages **0** / new go.mod modules **0** / **ROADMAP row 60 → `done`**.

- [ ] **Step 3: Record the 60.2 design pins** — a short section restating the "Design pins settled here" block above (config-seam built-provider + pre-scan; reject-home map; MVP one-secret cap; served-leaf CompareBytes; validate=nil) so the executing engineer sees the pinned decisions without re-reading the SPEC/PLAN preamble. (Bookkeeping — not a code change.)

- [ ] **Step 4: Commit**
```bash
git add docs/envoy-go/phases/60-xds-sds-server-cert/PROGRESS-60.2.md
git commit -m "phase 60.2 Task 1: PROGRESS scaffold + baselines + the 60.2 design pins (config-seam/reject-home/mvp-cap/served-leaf)"
```

---

## Task 2: `xds.ParseSDSConfig` — the SdsSecretConfig validator + extractor (arms 1–4, 8, 9) [TDD]

**Files:**
- Create: `internal/xds/config.go`
- Test: `internal/xds/config_test.go`

**Interfaces:**
- Produces: `func ParseSDSConfig(configs []*tlsv3.SdsSecretConfig) (secretName, clusterName string, timeout time.Duration, err error)` — consumed by `internal/tls`'s lift (Task 3, for `secretName`) and `boot.NewSDSProvider` (Task 5, for `clusterName`/`timeout`). NO grpcclient/cluster/tls import (the cycle guard).

- [ ] **Step 1: Write the failing tests** in `config_test.go`. Helpers: `sdsCfg(name, cluster string, mut ...func(*corev3.ConfigSource))` builds a valid `*tlsv3.SdsSecretConfig` (api_config_source, GRPC, V3, envoy_grpc→cluster, resource_api_version V3) that `mut` can corrupt for the reject arms.
  - **accept**: `ParseSDSConfig([]*tlsv3.SdsSecretConfig{sdsCfg("server_cert","sds_cluster")})` ⇒ `("server_cert","sds_cluster", 15*time.Second, nil)` (default timeout when `initial_fetch_timeout` unset).
  - **accept with explicit timeout**: a config whose `sds_config.initial_fetch_timeout = durationpb.New(3*time.Second)` ⇒ `timeout == 3*time.Second`.
  - **arm 8 empty name**: `sdsCfg("","sds_cluster")` ⇒ err contains `name is required`.
  - **arm 9 missing sds_config**: a config with `SdsConfig: nil` ⇒ err contains `sds_config is required`.
  - **arm 1 ads**: `sds_config = &corev3.ConfigSource{ConfigSourceSpecifier:&corev3.ConfigSource_Ads{Ads:&corev3.AggregatedConfigSource{}}}` ⇒ err contains `ads-sourced`.
  - **arm 2 self**: `ConfigSource_Self` ⇒ err contains `self-sourced`.
  - **arm 3 DELTA_GRPC**: api_type `ApiConfigSource_DELTA_GRPC` ⇒ err contains `DELTA_GRPC`.
  - **arm 4 google_grpc**: grpc_service `GrpcService_GoogleGrpc` ⇒ err contains `google_grpc`.
  - **non-api_config_source**: a `ConfigSource` with neither ads/self/api_config_source (e.g. `Path`) ⇒ err contains `api_config_source`.
  - **non-V3 resource**: `resource_api_version = ApiVersion_V2` ⇒ err contains `resource_api_version`.
  - **non-V3 transport**: `transport_api_version = ApiVersion_V2` ⇒ err contains `transport_api_version`.
  - **empty cluster_name**: envoy_grpc cluster `""` ⇒ err contains `cluster_name is required`.
  - **multi-cap**: two configs ⇒ err contains `multiple`.
  - Use `Errorf`-per-case in table form; do NOT `Fatalf` mid-table.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/xds/ -run TestParseSDSConfig -count=1` ⇒ FAIL (undefined `ParseSDSConfig`).

- [ ] **Step 3: Implement** `internal/xds/config.go`:
```go
package xds

import (
	"fmt"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
)

// defaultInitialFetchTimeout mirrors the reference's SDS initial_fetch_timeout
// default (15s, SPEC §3.4 / §11 Arm B) when the config leaves it unset.
const defaultInitialFetchTimeout = 15 * time.Second

// ParseSDSConfig validates a downstream tls_certificate_sds_secret_configs list
// and extracts the (secretName, clusterName, initial_fetch_timeout) triple the
// SDS provider needs. MVP: exactly ONE config (a fallback list is a later row).
// Every reject is ADR-0080-distinct and "xds: sds:"-prefixed — the reference
// SUPPORTS all these forms, so they are documented envoy-go-strict DEPARTURES.
// This function imports NEITHER internal/grpcclient NOR internal/cluster NOR
// internal/tls (the ADR-0278 cycle guard): it operates on already-typed protos.
func ParseSDSConfig(configs []*tlsv3.SdsSecretConfig) (secretName, clusterName string, timeout time.Duration, err error) {
	if len(configs) != 1 {
		return "", "", 0, fmt.Errorf("xds: sds: multiple tls_certificate_sds_secret_configs unsupported (MVP takes one, got %d)", len(configs))
	}
	sc := configs[0]
	name := sc.GetName()
	if name == "" {
		return "", "", 0, fmt.Errorf("xds: sds: SdsSecretConfig name is required")
	}
	cs := sc.GetSdsConfig()
	if cs == nil {
		return "", "", 0, fmt.Errorf("xds: sds: SdsSecretConfig sds_config is required")
	}
	if cs.GetAds() != nil {
		return "", "", 0, fmt.Errorf("xds: sds: ads-sourced ConfigSource unsupported (only api_config_source)")
	}
	if cs.GetSelf() != nil {
		return "", "", 0, fmt.Errorf("xds: sds: self-sourced ConfigSource unsupported (only api_config_source)")
	}
	acs := cs.GetApiConfigSource()
	if acs == nil {
		return "", "", 0, fmt.Errorf("xds: sds: ConfigSource must be an api_config_source")
	}
	if cs.GetResourceApiVersion() != corev3.ApiVersion_V3 {
		return "", "", 0, fmt.Errorf("xds: sds: resource_api_version must be V3")
	}
	switch acs.GetApiType() {
	case corev3.ApiConfigSource_GRPC:
		// ok
	case corev3.ApiConfigSource_DELTA_GRPC:
		return "", "", 0, fmt.Errorf("xds: sds: DELTA_GRPC api_type unsupported (only GRPC / State-of-the-World)")
	default:
		return "", "", 0, fmt.Errorf("xds: sds: api_type must be GRPC")
	}
	if acs.GetTransportApiVersion() != corev3.ApiVersion_V3 {
		return "", "", 0, fmt.Errorf("xds: sds: transport_api_version must be V3")
	}
	gs := acs.GetGrpcServices()
	if len(gs) != 1 {
		return "", "", 0, fmt.Errorf("xds: sds: exactly one grpc_service required (got %d)", len(gs))
	}
	if gs[0].GetGoogleGrpc() != nil {
		return "", "", 0, fmt.Errorf("xds: sds: google_grpc transport unsupported (only envoy_grpc)")
	}
	eg := gs[0].GetEnvoyGrpc()
	if eg == nil {
		return "", "", 0, fmt.Errorf("xds: sds: grpc_service must be envoy_grpc")
	}
	cluster := eg.GetClusterName()
	if cluster == "" {
		return "", "", 0, fmt.Errorf("xds: sds: envoy_grpc cluster_name is required")
	}
	timeout = defaultInitialFetchTimeout
	if d := cs.GetInitialFetchTimeout(); d != nil {
		timeout = d.AsDuration()
	}
	return name, cluster, timeout, nil
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/xds/ -run TestParseSDSConfig -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + cycle-guard re-check + commit**
```bash
gofmt -l internal/xds/ && golangci-lint run ./internal/xds/... && go vet ./internal/xds/... && go build ./... && go mod tidy -diff
go list -deps ./internal/xds | grep -E 'internal/(tls|grpcclient|cluster|listener|boot)$' || echo "XDS-CLEAN"   # MUST print XDS-CLEAN
git add internal/xds/config.go internal/xds/config_test.go
git commit -m "phase 60.2 Task 2: xds.ParseSDSConfig — SdsSecretConfig validator/extractor (arms 1-4,8,9 + V3 + one-config MVP cap), cycle guard intact"
```

---

## Task 3: The `tls/config.go` one-arm reject lift + the `provider` param [TDD]

**Files:**
- Modify: `internal/tls/config.go`
- Modify: `internal/tls/config_test.go`, `internal/tls/fuzz_test.go`

**Interfaces:**
- Consumes: `xds.SecretProvider` (60.1), `xds.ParseSDSConfig` (Task 2).
- Produces: `func NewDownstreamConfig(ts *corev3.TransportSocket, baseDir string, provider xds.SecretProvider) (*DownstreamConfig, error)` (new 3rd param) — consumed by the listener manager (Task 4). `commonTLSContextToConfig` gains a trailing `provider xds.SecretProvider` param.

- [ ] **Step 1: Write the failing tests** in `config_test.go`. Add a fake provider + SDS-config helpers (mirror the `internal/xds` helpers but downstream-shaped):
```go
type fakeProvider struct {
	cert *stdtls.Certificate
	err  error
}
func (f *fakeProvider) FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error) {
	return f.cert, f.err
}
// sdsDownstreamTS builds a DownstreamTlsContext whose common_tls_context has a
// single valid tls_certificate_sds_secret_configs entry (api_config_source GRPC/V3).
func sdsDownstreamTS(t *testing.T, secret, cluster string) *corev3.TransportSocket { /* … */ }
```
  - **accept (fake provider returns a real leaf)**: build a `*stdtls.Certificate` from `pki.leafCertPEM`/`pki.leafKeyPEM`; `NewDownstreamConfig(sdsDownstreamTS(t,"server_cert","sds_cluster"), "", &fakeProvider{cert: leaf})` ⇒ nil err, `len(cfg.TLSConfig.Certificates) == 1`.
  - **timeout propagation**: `&fakeProvider{err: fmt.Errorf("xds: sds: secret \"server_cert\": initial fetch timed out after 15s")}` ⇒ err contains `initial fetch timed out`.
  - **nil provider + valid SDS config**: `NewDownstreamConfig(sdsDownstreamTS(...), "", nil)` ⇒ err contains `requires a live SDS provider`.
  - **arm 6 (upstream)**: `NewUpstreamConfig(sdsUpstreamTS(t,"c","cl"), "")` (an `UpstreamTlsContext` with `tls_certificate_sds_secret_configs`) ⇒ err contains `tls: upstream: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03` (byte-identical to the pre-60.2 string).
  - **arm 1 ads (downstream, fake provider present)**: an SDS config whose ConfigSource is `ads` ⇒ err contains `ads-sourced` (ParseSDSConfig fires before the fetch).
  - **arm 5 (validation_context_sds, unchanged)**: assert the existing `:158` reject still returns `validation_context_sds_secret_config is not supported`.
  - Update EVERY existing `NewDownstreamConfig(ts, "")` in `config_test.go` (`:114,141,165,185,208,224,247,267,296,332,360,388,416,442,468`) to `NewDownstreamConfig(ts, "", nil)`; add `"context"` to the test imports.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tls/ -count=1` ⇒ FAIL (signature mismatch + undefined behavior).

- [ ] **Step 3: Implement** in `config.go`. Add imports `"context"` and `xds "github.com/pgdad/envoy-go/internal/xds"`. Change the signatures:
```go
func NewDownstreamConfig(ts *corev3.TransportSocket, baseDir string, provider xds.SecretProvider) (*DownstreamConfig, error) {
	// … unchanged prologue …
	cfg, err := commonTLSContextToConfig(ctx.GetCommonTlsContext(), baseDir, "downstream", provider)
	// … unchanged tail …
}

func NewUpstreamConfig(ts *corev3.TransportSocket, baseDir string) (*UpstreamConfig, error) {
	// … unchanged; the internal call passes nil (upstream never fetches SDS) …
	cfg, err := commonTLSContextToConfig(common, baseDir, "upstream", nil)
	// …
}
```
Then rewrite the `:153` block inside `commonTLSContextToConfig(c *tlsv3.CommonTlsContext, baseDir, side string, provider xds.SecretProvider)`. REPLACE the old wholesale reject:
```go
	// SDS-bound downstream server certificate (phase 60.2, ADR-0280). The
	// upstream/validate paths keep the byte-identical reject (arm 6). Downstream
	// with a provider present: validate the SdsSecretConfig (arms 1-4,8,9 live in
	// xds.ParseSDSConfig), then BLOCK on the first Secret (bounded by
	// initial_fetch_timeout inside the provider) and hold the built leaf; it is
	// appended to cfg.Certificates after cfg is created below. On timeout /
	// mgmt-unreachable the provider returns a classified error that PROPAGATES
	// here → the listener build FAILS → boot FAILS (the documented envoy-go
	// DEPARTURE from the reference's serve-cert-less behavior, SPEC §3.4).
	var sdsCert *stdtls.Certificate
	if len(c.GetTlsCertificateSdsSecretConfigs()) > 0 {
		if side != "downstream" {
			return nil, fmt.Errorf("tls: %s: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03", side)
		}
		secretName, _, _, err := xds.ParseSDSConfig(c.GetTlsCertificateSdsSecretConfigs())
		if err != nil {
			return nil, err
		}
		if provider == nil {
			return nil, fmt.Errorf("tls: downstream: SDS-delivered certificate requires a live SDS provider (unavailable in this mode)")
		}
		cert, err := provider.FetchInitialCertificate(context.Background(), secretName)
		if err != nil {
			return nil, fmt.Errorf("tls: downstream: SDS secret %q: %w", secretName, err)
		}
		sdsCert = cert
	}
```
Keep the `validation_context_sds`/`combined`/`custom_validator`/`match_typed_san`/`verify_*` rejects (`:156-177`) unchanged. After `cfg := &stdtls.Config{}` (`:179`), append the SDS leaf BEFORE the static loop:
```go
	cfg := &stdtls.Config{}
	if sdsCert != nil {
		cfg.Certificates = append(cfg.Certificates, *sdsCert)
	}
	for i, tc := range c.GetTlsCertificates() { /* … unchanged … */ }
```
The `:200` downstream empty-cert check now passes for an SDS-only config (the SDS leaf is in `cfg.Certificates`). Fix `fuzz_test.go:87` to `NewDownstreamConfig(ts, "", nil)`.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tls/ -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + cycle-guard re-check + commit**
```bash
gofmt -l internal/tls/ && golangci-lint run ./internal/tls/... && go vet ./internal/tls/... && go build ./... && go mod tidy -diff
go list -deps ./internal/tls | grep -E 'internal/(grpcclient|cluster|boot|listener)$' || echo "TLS-CLEAN"   # MUST print TLS-CLEAN (tls->xds is allowed; tls->grpcclient/cluster/boot/listener is NOT)
git add internal/tls/config.go internal/tls/config_test.go internal/tls/fuzz_test.go
git commit -m "phase 60.2 Task 3: tls/config.go one-arm SDS lift (downstream-gated) + provider param; arms 1-6 + timeout-propagation; cycle guard intact (tls->xds only)"
```

---

## Task 4: Thread the `sdsProvider` through the listener manager [mechanical + regression]

**Files:**
- Modify: `internal/listener/manager.go`
- Modify: `internal/listener/manager_test.go`, `internal/listener/tls_handshake_negative_test.go`, `internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go`

**Interfaces:**
- Consumes: `NewDownstreamConfig(ts, baseDir, provider)` (Task 3).
- Produces: `NewManagerWithBaseDirAndAllowH2C(…, sdsProvider xds.SecretProvider)` (new trailing param) — consumed by `boot.Construct` (Task 5).

- [ ] **Step 1: Add the param + thread it.** Import `xds "github.com/pgdad/envoy-go/internal/xds"` in `manager.go`. Add a trailing `sdsProvider xds.SecretProvider` param to `NewManagerWithBaseDirAndAllowH2C` (`:254`) and to `buildListenerRuntimeWithCtx` (`:340`); pass `sdsProvider` at the `:268` call. At the two `internaltls.NewDownstreamConfig(ts, baseDir)` sites (`:382`, `:457`) → `internaltls.NewDownstreamConfig(ts, baseDir, sdsProvider)`. The two thin wrappers `NewManagerWithBaseDir` (`:219`) and the `:204` ctor pass `nil` (they are non-SDS internal/test entry points).

- [ ] **Step 2: Update all call sites (compiler-enumerated).** Add `nil` (the SDS provider) at every `NewManagerWithBaseDirAndAllowH2C(...)` call. The full list (RE-DERIVE with `grep -rn 'NewManagerWithBaseDirAndAllowH2C(' --include='*.go' .` — add the trailing `, nil` before the closing paren): `internal/boot/boot.go:83` (handled in Task 5, threads the real provider); `internal/listener/tls_handshake_negative_test.go:41`; `internal/listener/manager_test.go` (~30 sites: `:1408,1576,1596,1757,1787,2163,2391,2655,2674,2691,2720,2806,2835,2863,2887,2902,2940,2964,2977,2995,3018,3042,3155,3379,3408,3429`); `internal/admin/admin_helpers_test.go:114`; `internal/admin/listeners_test.go:347`.

- [ ] **Step 3: Run the touched packages** — `go build ./... && go test ./internal/listener/ ./internal/admin/ -count=1` ⇒ PASS (behavior unchanged; nil provider = the pre-60.2 reject for any SDS config, which no existing test hits).

- [ ] **Step 4: Per-task gates + commit**
```bash
gofmt -l internal/listener/ internal/admin/ && golangci-lint run ./internal/listener/... ./internal/admin/... && go vet ./internal/listener/... && go build ./...
git add internal/listener/ internal/admin/
git commit -m "phase 60.2 Task 4: thread sdsProvider through the listener manager (NewManagerWithBaseDirAndAllowH2C -> buildListenerRuntimeWithCtx -> NewDownstreamConfig); nil at all existing callers"
```

---

## Task 5: `boot.NewSDSProvider` (pre-scan + node arm 7) + `boot.Construct` param + main/validate wiring [TDD]

**Files:**
- Modify: `internal/boot/boot.go`
- Modify: `cmd/envoy-go/main.go`, `validate/validate.go`
- Test: `internal/boot/boot_test.go` (create if absent; else the existing boot test file)

**Interfaces:**
- Consumes: `xds.ParseSDSConfig` (Task 2), `xds.NewProvider`/`xds.Node`/`xds.RegisterSDSStats` + `xdsgrpc.NewOpener` + `grpcclient.NewSDSClient` (60.1), `NewManagerWithBaseDirAndAllowH2C(…, sdsProvider)` (Task 4).
- Produces: `func NewSDSProvider(dialer *grpcclient.Dialer, bs *bootstrap.Bootstrap, baseDir string, registry *stats.Registry) (xds.SecretProvider, error)`; `Construct(…, sdsProvider xds.SecretProvider)`.

- [ ] **Step 1: Write the failing tests** in `boot_test.go`. Use `test/helpers/sdsserver` (60.1) as the fake management server for the success case, and hand-built `*bootstrap.Bootstrap` protos for the reject/no-SDS cases. Import `sdsserver`, `grpcclient`, `cluster`, `bootstrap`, `stats`, plus the tls/listener protos to build a bootstrap with a downstream SDS listener.
  - **no SDS configured** → `NewSDSProvider(dialer, bsNoSDS, "", reg)` ⇒ `(nil, nil)`.
  - **arm 7 (SDS + empty node)** → a bootstrap with an SDS-bound downstream listener but `node` unset ⇒ err contains `node.id and node.cluster are required for SDS`.
  - **success** → a bootstrap with `node{id,cluster}` + a downstream SDS listener whose `sds_cluster` is a real static cluster pointing at a running `sdsserver.New(t, WithSecret("server_cert", pkiCert, pkiKey))` (build a `cluster.Manager` over that bootstrap so the dialer can resolve `sds_cluster`); `NewSDSProvider(...)` ⇒ non-nil provider; `provider.FetchInitialCertificate(ctx, "server_cert")` returns a cert whose `Leaf`/parsed serial matches the delivered cert. (This is the boot-side integration proof; the pure handshake is 60.1-proven.)
  - **arm 1 (bad ConfigSource) surfaces at boot** → an SDS listener with an `ads` ConfigSource ⇒ `NewSDSProvider` err contains `ads-sourced` (the pre-scan's ParseSDSConfig rejects).
  - Use `Errorf`-per-case where independent; `Fatalf` only for a broken precondition (e.g. the cluster manager failing to build).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/boot/ -run TestNewSDSProvider -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement `NewSDSProvider` + the `Construct` param** in `boot.go`. Add imports `xds`, `xdsgrpc`, `tlsv3`, `corev3`, `proto`. Sketch:
```go
// NewSDSProvider pre-scans bs for the (single, MVP) downstream SDS-bound TLS
// context and, when present, builds the blocking xds.SecretProvider that
// internal/tls consults at listener construction (phase 60.2, ADR-0280). It
// mirrors NewTracingProvider: built once at boot from the shared dialer, then
// threaded through Construct into the listener build. Returns (nil, nil) when no
// SDS cert is configured (the nil provider threads harmlessly — the tls lift
// only engages when a listener actually carries tls_certificate_sds_secret_configs).
//
// Enforces the NODE boot-requirement (arm 7, SPEC §6/§11 Arm A-pre): SDS
// configured while bootstrap node.id/node.cluster are empty ⇒ boot-fail. The
// cluster/secret/timeout are extracted via xds.ParseSDSConfig (arms 1-4,8,9);
// the *grpcclient.SDSClient edge is carried by xdsgrpc.NewOpener so internal/xds
// stays grpcclient-free (the ADR-0278 cycle guard).
func NewSDSProvider(dialer *grpcclient.Dialer, bs *bootstrap.Bootstrap, baseDir string, registry *stats.Registry) (xds.SecretProvider, error) {
	dtcTypeURL := "type.googleapis.com/" + string(proto.MessageName(&tlsv3.DownstreamTlsContext{}))
	var found []*tlsv3.SdsSecretConfig
	seen := 0
	for _, l := range bs.Proto.GetStaticResources().GetListeners() {
		chains := append([]*listenerv3.FilterChain{}, l.GetFilterChains()...)
		if dfc := l.GetDefaultFilterChain(); dfc != nil {
			chains = append(chains, dfc)
		}
		for _, fc := range chains {
			ts := fc.GetTransportSocket()
			if ts == nil || ts.GetTypedConfig().GetTypeUrl() != dtcTypeURL {
				continue
			}
			var dtc tlsv3.DownstreamTlsContext
			if err := ts.GetTypedConfig().UnmarshalTo(&dtc); err != nil {
				continue // a malformed transport_socket surfaces at the listener build, not here
			}
			if sc := dtc.GetCommonTlsContext().GetTlsCertificateSdsSecretConfigs(); len(sc) > 0 {
				seen++
				found = sc
			}
		}
	}
	if seen == 0 {
		return nil, nil
	}
	if seen > 1 {
		return nil, fmt.Errorf("xds: sds: multiple SDS-bound downstream TLS contexts unsupported (MVP takes one)")
	}
	node := bs.Proto.GetNode()
	if node.GetId() == "" || node.GetCluster() == "" {
		return nil, fmt.Errorf("xds: sds: node.id and node.cluster are required for SDS")
	}
	secretName, clusterName, timeout, err := xds.ParseSDSConfig(found)
	if err != nil {
		return nil, err
	}
	client, err := grpcclient.NewSDSClient(dialer, clusterName)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: dial cluster %q: %w", clusterName, err)
	}
	opener := xdsgrpc.NewOpener(client)
	stats := xds.RegisterSDSStats(registry, secretName)
	return xds.NewProvider(opener, xds.Node{ID: node.GetId(), Cluster: node.GetCluster()}, baseDir, timeout, stats), nil
}
```
Add the trailing `sdsProvider xds.SecretProvider` param to `Construct` (`:53`) and pass it at the `NewManagerWithBaseDirAndAllowH2C` call (`:83`). NOTE the double-parse (pre-scan here + the tls lift's ParseSDSConfig) is intentional and agrees (both pure over the same config).

- [ ] **Step 4: Wire `main.go` + `validate.go`.** In `main.go` after `:150` (next to `tracingProvider`):
```go
	sdsProvider, err := boot.NewSDSProvider(dialer, bs, filepath.Dir(*cfgPath), bs.Stats)
	if err != nil {
		return fmt.Errorf("sds provider: %w", err)   // or the file's existing fatal-error idiom
	}
```
and pass `sdsProvider` into `boot.Construct(...)` at `:295`. In `validate/validate.go:48` pass `nil` for the new SDS-provider param (validate does not dial/fetch SDS).

- [ ] **Step 5: Run to verify they pass** — `go test ./internal/boot/ ./validate/ -count=1 && go build ./...` ⇒ PASS.

- [ ] **Step 6: Per-task gates + cycle-guard re-check + commit**
```bash
gofmt -l internal/boot/ cmd/envoy-go/ validate/ && golangci-lint run ./internal/boot/... ./cmd/... ./validate/... && go vet ./internal/boot/... && go build ./... && go mod tidy -diff
go list -deps ./internal/tls | grep -E 'internal/(grpcclient|cluster|boot)$' || echo "TLS-CLEAN"   # still TLS-CLEAN
git add internal/boot/ cmd/envoy-go/main.go validate/validate.go internal/boot/boot_test.go
git commit -m "phase 60.2 Task 5: boot.NewSDSProvider (pre-scan + node arm 7 + provider build via xdsgrpc) + Construct param + main/validate wiring"
```

---

## Task 6: `sdsserver.NewAtAddr` (0.0.0.0 bind) + `helpers.TLSServedLeaf` (served-leaf accessor) [TDD]

**Files:**
- Modify: `test/helpers/sdsserver/sdsserver.go`, `test/helpers/sdsserver/sdsserver_test.go`
- Modify: `test/helpers/tls.go`

**Interfaces:**
- Produces: `func NewAtAddr(addr string, opts ...Option) (*Server, error)`; `func TLSServedLeaf(ctx context.Context, addr, serverName string, rootCAs *x509.CertPool) (*x509.Certificate, error)` — both consumed by the 0103 driver (Task 7).

- [ ] **Step 1: Write the failing tests.** In `sdsserver_test.go`: `NewAtAddr("127.0.0.1:0", WithSecret("s", cert, key))` ⇒ non-nil server; dial it (reuse the existing test's dial harness) and assert the delivered Secret name. In a `test/helpers` test (or exercised via the 0103 driver — but add a focused unit test): `TLSServedLeaf` against an in-process `crypto/tls` server serving a known leaf ⇒ returns that leaf's `SerialNumber`.

- [ ] **Step 2: Run to verify they fail** — `go test ./test/helpers/... -count=1` ⇒ FAIL (undefined symbols).

- [ ] **Step 3: Implement.** In `sdsserver.go` (the shared `newServer(addr, opts...)` already exists):
```go
// NewAtAddr binds the caller-supplied host:port (e.g. "0.0.0.0:<preAllocatedPort>"
// so a Docker reference-Envoy can dial the host) and starts the server. No
// t.Cleanup — the CALLER (a fixture driver) owns the lifecycle via Server.Stop.
// Mirrors metricsservice.NewAtAddr.
func NewAtAddr(addr string, opts ...Option) (*Server, error) {
	return newServer(addr, opts...)
}
```
In `test/helpers/tls.go` add:
```go
// TLSServedLeaf dials addr, completes a TLS handshake (validating against rootCAs
// + serverName), and returns the leaf the SERVER presented
// (ConnectionState().PeerCertificates[0]) — used by cross-side differentials that
// assert the served certificate identity. It performs no application I/O.
func TLSServedLeaf(ctx context.Context, addr, serverName string, rootCAs *x509.CertPool) (*x509.Certificate, error) {
	d := &net.Dialer{}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tls served leaf: dial: %w", err)
	}
	conn := stdtls.Client(raw, &stdtls.Config{ServerName: serverName, RootCAs: rootCAs, MinVersion: stdtls.VersionTLS12, MaxVersion: stdtls.VersionTLS13})
	defer func() { _ = conn.Close() }()
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("tls served leaf: handshake: %w", err)
	}
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("tls served leaf: no peer certificates")
	}
	return certs[0], nil
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./test/helpers/... -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/ && golangci-lint run ./test/helpers/... && go build ./... && go vet ./test/helpers/...
git add test/helpers/sdsserver/ test/helpers/tls.go
git commit -m "phase 60.2 Task 6: sdsserver.NewAtAddr (0.0.0.0 bind for the differential) + helpers.TLSServedLeaf (served-leaf accessor)"
```

---

## Task 7: The `0103-xds-sds-server-cert` differential fixture + prove the served-leaf assertion live [fixture]

**Files:**
- Create: `test/fixtures/0103-xds-sds-server-cert/{driver/driver.go, driver/doc.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md, pki/{leaf.pem,leaf.key.pem,ca.pem}}`
- Modify: `test/differential/runner_test.go` (one blank import)

**The design (SPEC §8; the `0089-stats-sink-metrics-service` + `0002-tls-tcp` precedents):** BackendCount `1` (one TCP echo backend the listener's `tcp_proxy` targets; the observable is the served leaf, not the echo bytes). Two driver-owned `sdsserver` instances (one per side), each `NewAtAddr("0.0.0.0:<preAllocatedPort>", WithSecret("server_cert", leafPEM, leafKeyPEM))`, started idempotently in `ensure()` (called from `ReferenceBootstrap`/`SubjectConfig`), stopped in `closeServers()` (via `Server.Stop`; the proxies hold long-lived SDS streams). Reference config → `host.docker.internal:<refPort>` STRICT_DNS; subject config → `127.0.0.1:<subjPort>`. Both deliver the SAME committed `pki/leaf.pem` (distinctive serial + `SAN=DNS:sds.envoy-go.test`). Both configs carry `node{id: envoygo-node, cluster: envoygo-cluster}` (arm 7). `DriveReference`/`DriveSubject` call `helpers.TLSServedLeaf(ctx, addr, "sds.envoy-go.test", caPool)` and return `fmt.Sprintf("serial=%X\nsan=%s\n", leaf.SerialNumber, strings.Join(leaf.DNSNames, ","))`; the runner's Step-7 `CompareBytes` enforces it byte-identical cross-side.

- [ ] **Step 1: Generate + commit the PKI.** Generate a self-signed leaf with a fixed distinctive serial and `SAN=DNS:sds.envoy-go.test` (its own CA so both proxies' TLS clients validate it). Commit `pki/ca.pem`, `pki/leaf.pem`, `pki/leaf.key.pem`. (Mirror `test/fixtures/0002-tls-tcp/pki/`. Use a one-shot Go/openssl generator; DO NOT leave the generator in the tree.)

- [ ] **Step 2: Write the two config templates** (`text/template`, rendered by the driver — the `0089` `mustReadFixtureFile`+`mustRender` pattern). Both declare: a `static_resources.clusters` entry `sds_cluster` (STRICT_DNS, `http2_protocol_options {}` — the SDS server is plaintext h2c) whose endpoint is `{{.SDSHost}}:{{.SDSPort}}`; the echo backend cluster; a listener `sds_tls_listener` with a `filter_chain` whose `transport_socket` is a `DownstreamTlsContext` with `common_tls_context.tls_certificate_sds_secret_configs: [{name: server_cert, sds_config: {resource_api_version: V3, api_config_source: {api_type: GRPC, transport_api_version: V3, grpc_services: [{envoy_grpc: {cluster_name: sds_cluster}}]}}}]` and a `tcp_proxy` to the echo cluster; a top-level `node: {id: envoygo-node, cluster: envoygo-cluster}`. Reference (`envoy.yaml`) uses `{{.SDSHost}}=host.docker.internal`; subject (`envoy-go.yaml`) uses `127.0.0.1`. (Base the `DownstreamTlsContext` + `tls_inspector`-free single-chain shape on `0002-tls-tcp/envoy.yaml:38-80`, swapping inline `tls_certificates` for `tls_certificate_sds_secret_configs`.)

- [ ] **Step 3: Write the driver** (`driver/driver.go`) implementing `fixture.Driver` + registering `"0103-xds-sds-server-cert"` in `init()`. Key pieces (model on `0089/driver/driver.go`):
  - fields: `refSDSPort, subjSDSPort int` (pre-allocated via `mustAllocatePort`); `refSrv, subjSrv *sdsserver.Server`; `caPool *x509.CertPool`; `leafPEM, keyPEM []byte` (read from `pki/`); a `sync.Once`-guarded `ensure()`.
  - `BackendCount() int { return 1 }`; `SubjectListenerName() string { return "sds_tls_listener" }`; `ReferenceListenerPort() int { return 10443 }`.
  - `ensure()`: read PKI (`fixtureDir()` via `runtime.Caller`); build `caPool`; start `refSrv, _ = sdsserver.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", d.refSDSPort), sdsserver.WithSecret("server_cert", d.leafPEM, d.keyPEM))` and likewise `subjSrv`.
  - `ReferenceBootstrap(backendPorts)`: `d.ensure()`; render `envoy.yaml` with `SDSHost=host.docker.internal, SDSPort=d.refSDSPort, BackendPort=backendPorts[0]`.
  - `SubjectConfig(refPort, subjPort, backendPorts, adminPort)`: `d.ensure()`; render `envoy-go.yaml` with `SDSHost=127.0.0.1, SDSPort=d.subjSDSPort, BackendPort=backendPorts[0], ListenPort=subjPort, AdminPort=adminPort`.
  - `driveSide(ctx, addr)`: `leaf, err := helpers.TLSServedLeaf(ctx, addr, "sds.envoy-go.test", d.caPool)`; on err return `(nil, err)`; else `return []byte(fmt.Sprintf("serial=%X\nsan=%s\n", leaf.SerialNumber, strings.Join(leaf.DNSNames, ","))), nil`. `DriveReference`/`DriveSubject` both delegate to `driveSide` (then `closeServers()` may run after subject — but keep both servers up until BOTH drives complete; stop in a `t.Cleanup`-equivalent the harness offers, or leave `Stop` to a package-level teardown — follow `0089`'s `closeServers()` timing which runs after subject drive).
  - `ProbeAdmin`: copy the standard `/ready` GET-and-return-bytes body from an existing driver (e.g. `0089`).

- [ ] **Step 4: Register + write README/expectations.** Add `_ "github.com/pgdad/envoy-go/test/fixtures/0103-xds-sds-server-cert/driver"` to `runner_test.go` (after the last existing blank import). `expectations.yaml` + `README.md` per the fixture-dir convention (document: SDS downstream server cert, SotW/initial-fetch, the served-leaf cross-side assertion, the two per-side driver-owned servers, `node` required).

- [ ] **Step 5: Run the fixture** (Docker required):
```bash
go test ./test/differential/ -run 'TestDifferential/0103-xds-sds-server-cert' -count=1 -v
```
Expected: PASS (both proxies serve the committed leaf; `CompareBytes` over `serial=…/san=…` matches).

- [ ] **Step 6: PROVE the served-leaf assertion LIVE (`reference_differential_break_protocol_count1` + `reference_deliberate_break_wrong_assertion`).** Temporarily configure `refSrv` with a DIFFERENT leaf (a second committed/generated cert with a different serial) — so the reference serves a different served leaf than the subject. Re-run `-count=1 -run 'TestDifferential/0103-xds-sds-server-cert'`; CONFIRM the failure is `CompareBytes` reporting the `serial=` line differs (NOT a handshake/boot error — the handshake still completes; the mismatch is purely in the observable). Revert to the single shared leaf; re-run ⇒ PASS. Record the break evidence in PROGRESS-60.2.md.

- [ ] **Step 7: Commit**
```bash
gofmt -l test/fixtures/0103-xds-sds-server-cert/ && go vet ./test/fixtures/0103-xds-sds-server-cert/...
git add test/fixtures/0103-xds-sds-server-cert/ test/differential/runner_test.go
git commit -m "phase 60.2 Task 7: 0103-xds-sds-server-cert differential (driver-owned SDS server per side, served-leaf cross-side via CompareBytes); assertion proven live"
```

---

## Task 8: BEHAVIOR_CONTRACT xDS/SDS section [docs]

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: Add a new xDS / SDS section** (RE-DERIVE exact insertion point + existing wording conventions at IMPL). Content (SPEC §9): SDS downstream server-cert SUPPORTED (SotW, `api_config_source` GRPC/V3, initial-fetch, NO rotation — the leaf is built once); `node.id`+`node.cluster` REQUIRED for SDS (parity with the reference boot-requirement); the SIX+ONE reject DEPARTURES (ads / self / DELTA_GRPC / google_grpc / downstream validation_context-SDS / upstream tls_certificate-SDS reject loudly — the reference accepts; plus the one-SDS-secret MVP cap); the initial-fetch-timeout DEPARTURE (envoy-go BOOT-FAILS on `initial_fetch_timeout` expiry / mgmt-unreachable where the reference serves cert-less and rejects handshakes); rotation/re-delivery IGNORED (initial-fetch only); the `dynamic_resources`/`layered_runtime` rejects STILL stand. The `sds.<secret>.*` 5-counter subset is emitted (dynamic, per-secret).

- [ ] **Step 2: Commit**
```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 60.2 Task 8: BEHAVIOR_CONTRACT xDS/SDS section (SDS downstream server cert, node-required, the 6+1 reject departures, initial-fetch boot-fail departure)"
```

---

## Task 9: ADR-0280 + STATE + ROADMAP row 60 `done` + sentinel re-check + final verify [docs + verify]

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`
- Finalize: `docs/envoy-go/phases/60-xds-sds-server-cert/PROGRESS-60.2.md`

- [ ] **Step 1: Write ADR-0280** (`## ADR-0280 — the SDS downstream-server-cert TLS-apply leg …`) with §Context (re-use the SPEC-60 §13 frame + the ADR-0278 §Consequences hand-off naming 60.2/ADR-0280), §Decision (the config-seam built-provider + pre-scan; the one-arm downstream-gated lift; the 6+1 reject roster homes; the initial-fetch BLOCK-then-BOOT-FAIL departure; the MVP one-secret cap; the served-leaf differential; the cycle guard preserved — `tls → xds` only, the grpcclient edge in `xdsgrpc`), and §Consequences (row 60 `done`; fixtures 104→105; the `sds.*` dynamic surface materialized; family STAYS OPEN; the deferred candidates that remain). ADR-0044: §Decision/§Consequences land HERE (the IMPL). DECISIONS tail → **ADR-0280**.

- [ ] **Step 2: Flip ROADMAP row 60 → `done` + update the xDS deferred sentence.** Edit `docs/envoy-go/ROADMAP.md:122` (row 60 status `in-progress` → `done`, ADR-0106 — both legs landed). Update the xDS family deferred sentence (`:181`): keep it a SINGLE live "candidates:" sentence (the family STAYS OPEN), noting SDS-server-cert (SotW/initial-fetch) now DONE and the remaining candidates (SDS rotation + validation_context/upstream SDS + CDS/EDS + LDS/RDS + ADS + Delta + RTDS + reconnection-backoff + google_grpc). Re-run the sentinel check-(2) grep and confirm EXACTLY ONE live xDS "candidates:" match (`reference_sentinel_deferred_sentence_live_vs_historical`):
```bash
grep -oE 'remaining deferred \(not-yet-chartered\) candidates:[^.]*\.' docs/envoy-go/ROADMAP.md   # xDS line still present, exactly one xDS match
```

- [ ] **Step 3: Flip STATE.** Update the active-phase header: `phase 60.2 (xds-sds-tls-cert-apply) IMPL done` (row 60 → `done`; NEXT = the roller's next self-pick per the standing directive — the sentinel governs, HTTP/3 row 61 is the banked follow-on). Record the exit counts.

- [ ] **Step 4: Finalize PROGRESS-60.2.md** — check off all tasks, record the break evidence (Task 7 Step 6), the final counts, and the six-gate results.

- [ ] **Step 5: The six-gate completion verify** (`superpowers:verification-before-completion` — evidence before assertions):
```bash
gofmt -l . | (grep . && echo GOFMT_DIRTY || echo GOFMT_CLEAN)
golangci-lint run ./... 
go build ./... && echo BUILD_OK
go vet ./... && echo VET_OK
go mod tidy -diff && echo MODTIDY_CLEAN
go test ./... -count=1                                    # full unit suite green
go list -deps ./internal/tls | grep -E 'internal/(grpcclient|cluster|boot|listener)$' || echo TLS-CLEAN
go list -deps ./internal/xds | grep -E 'internal/(tls|grpcclient|cluster|listener|boot)$' || echo XDS-CLEAN
grep -rh '^func Fuzz' --include='*.go' . | wc -l          # 55 (unchanged)
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l         # 105
go test ./test/differential/ -count=1                     # FULL 105-dir differential green (Docker); byte-stable except the new 0103
```
Confirm the full differential is byte-stable except `0103` (`reference_fixture_workload_constant_desync` / `reference_differential_fullsuite_startup_flake` — isolate-re-run any startup flake, do not re-classify).

- [ ] **Step 6: Commit**
```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md docs/envoy-go/phases/60-xds-sds-server-cert/PROGRESS-60.2.md
git commit -m "phase 60.2 Task 9: ADR-0280 + STATE + ROADMAP row 60 done (both legs landed, ADR-0106) + xDS deferred sentence + six-gate verify"
```

**Then the controller (not a subagent):** squash the task commits into one `phase 60.2 (xds-sds-tls-cert-apply) IMPL: …` stage commit, re-run the six-gate on the frozen HEAD, ROLL `next-prompt.txt` forward (row 60 done; next stage = the roller's self-pick — HTTP/3 row 61's next leg or a fresh pick per the standing directive; re-run the termination sentinel MECHANICALLY), fold the `next-prompt.txt` edit into the squash, and PUSH (`feedback_push_to_origin`).

---

## Self-Review (run against SPEC-60 §3/§6/§7/§8/§9 + the router)

**Spec coverage:**
- §3.2 config seam (built provider + pre-scan, MANAGEABLE) → Tasks 4 + 5. ✓
- §3.3 one-arm downstream-gated lift + `provider` param → Task 3. ✓
- §3.4 initial-fetch BLOCK-then-BOOT-FAIL departure → Task 3 (timeout propagation) + Task 5 (the provider bounds it). ✓
- §6 reject roster: arms 1–4/8/9 (Task 2, `xds.ParseSDSConfig`); arm 5 unchanged (Task 3); arm 6 side-gated (Task 3); arm 7 node (Task 5); arm 10 already 60.1. ✓
- §7 the 5-counter `sds.*` subset materializes at boot via `RegisterSDSStats` in `NewSDSProvider` (Task 5); the differential surfaces the names (Task 7). ✓
- §8 ONE differential dir `0103` + served-leaf cross-side + the timeout/mgmt-down/secret-not-found as SUBJECT-side unit tests (Task 3/5), NOT extra dirs → Task 7. ✓
- §9 BEHAVIOR_CONTRACT → Task 8. ✓ ADR-0280 + row 60 `done` → Task 9. ✓

**Placeholder scan:** the mechanical `+nil` sweep (Task 4) is enumerated by grep, not left as "update callers"; every new function has full code; the driver + templates have concrete shapes. The PKI generation (Task 7 Step 1) and the exact BEHAVIOR_CONTRACT/ADR insertion points are RE-DERIVED at IMPL (docs targets that legitimately depend on the tip).

**Type consistency:** `ParseSDSConfig(configs) (secretName, clusterName string, timeout time.Duration, err error)` is consumed with the same shape in Task 3 (`secretName`) and Task 5 (`clusterName`, `timeout`). `NewDownstreamConfig(ts, baseDir, provider xds.SecretProvider)` is threaded identically in Tasks 3→4→5. `NewSDSProvider(dialer, bs, baseDir, registry) (xds.SecretProvider, error)` matches its `main.go`/`validate.go`/test consumers. The cycle guard (`tls → xds` only; grpcclient edge in `xdsgrpc`) is re-checked with `go list -deps` at Tasks 2/3/5/9.

**Cross-cutting risks flagged:** the double-parse (boot pre-scan + tls lift, both `ParseSDSConfig`, agree); the reject-ordering shift in `commonTLSContextToConfig` (SDS side/arm rejects stay at `:153`; only the cert-append moves after `cfg` creation — order preserved for the validation_context arms); the differential's Docker reachability (`host.docker.internal` STRICT_DNS, NOT `HostGatewayIP`); the two-per-side servers enabling the clean liveness break.
