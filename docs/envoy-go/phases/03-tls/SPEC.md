# Phase 03 — TLS (Downstream Termination + Upstream Origination + SNI)

**Phase id:** `03`
**Slug:** `03-tls`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:brainstorming` (adapted autonomous mode — see `DECISIONS.md` ADR-0004)
**Depends on:** phase 02 (done)
**Differential surface at end of phase:** pre-existing fixtures `0000-tcp-echo` and `0001-tcp-proxy-rr` remain green with no behavioural regression; new fixture `0002-tls-tcp` green, exercising downstream TLS termination with SNI-based filter-chain selection, two upstream TLS-originating clusters, and byte-exact equivalence against upstream Envoy v1.37.2 on the decrypted payload through a 3-endpoint RR-balanced cluster per SNI.

---

## 1. Purpose

Phase 03 lands envoy-go's first cryptographic surface: the data plane now terminates TLS at the listener, originates TLS at the upstream cluster, and dispatches among per-listener filter chains by the ClientHello's SNI. This is the first phase whose feature surface is visible on the wire as anything other than raw bytes — every upstream-comparison claim after phase 03 depends on the handshake-observable parts of this surface matching upstream Envoy's.

Concretely, phase 03 produces:

1. A TLS configuration package under `internal/tls/` that parses the Envoy v3 protos `envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext` and `UpstreamTlsContext` (both carried as the `type_url` of a `core.v3.TransportSocket.typed_config` Any), loads certificates and keys from their `DataSource` envelopes, and returns ready-to-use `*crypto/tls.Config` values for downstream (server-side) and upstream (client-side) use.
2. A listener manager extended to support 1..N filter chains per listener. Each chain may carry a `transport_socket` (TLS) and a `filter_chain_match` restricted to `server_names[]` (SNI) and an optional `transport_protocol == "tls"` predicate. Filter-chain selection happens inside the TLS handshake via `tls.Config.GetConfigForClient`: the listener builds a single top-level server `tls.Config` whose `GetConfigForClient` callback inspects the ClientHello's SNI, selects the most-specific matching chain, falls back to a single catch-all (empty-match) chain if present, and closes the connection (returning no config) if neither applies. Each chain has its own terminal filter, built against its own resolved cluster.
3. A cluster manager extended to hold an optional upstream `transportSocket` per cluster. Clusters with TLS configured expose a `Dial(ctx) (net.Conn, error)` method that composes `net.DialTimeout` + `tls.Client` + `HandshakeContext`; plaintext clusters expose the same `Dial` returning the raw TCP conn. The TCP proxy filter consumes `Cluster.Dial` and becomes transport-agnostic.
4. A rewired TCP proxy filter whose `Handle(ctx, downstream)` uses `Cluster.Dial(ctx)` for the upstream side, consumes `ctx` for `HandshakeContext`, and otherwise preserves the phase-00/02 half-closed bidirectional pump verbatim. Accepting and consuming `ctx` retires phase-02 REVIEW Minor 4.
5. A fixture-driver interface evolution: the phase-02 single `Drive(ctx, refAddr, subjAddr)` method is split into `DriveReference(ctx, addr)` and `DriveSubject(ctx, addr)`. The runner calls them separately (it already did — via `""` sentinels) so the contract matches the runner's actual usage. Phase-02 REVIEW Minor 6 resolved.
6. A new differential fixture `test/fixtures/0002-tls-tcp/`: one downstream-TLS-terminating listener with two SNI-indexed filter chains (`alpha.envoy-go.test` → cluster `c_alpha`; `beta.envoy-go.test` → cluster `c_beta`), each cluster with three TLS-speaking backends, upstream SNI set per cluster. Driver sends 9 TLS round-trip requests per SNI against each proxy (18 per proxy). Byte-exact plaintext equivalence on each proxy. Per-proxy distribution asserted to be exactly `[3, 3, 3]` per cluster.
7. Committed PKI artifacts under `test/fixtures/0002-tls-tcp/pki/` (one test root CA + four leaf certs: `server-alpha`, `server-beta`, `upstream-alpha`, `upstream-beta`) with ≥10-year validity from a fixed not-before date, plus a `README.md` documenting the generation command so they can be regenerated deterministically.
8. A new `BEHAVIOR_CONTRACT.md` subsection, **TLS**, codifying the equivalence surface for the TLS layer: plaintext-after-decryption byte equivalence, handshake observability rules (what IS vs IS NOT compared between proxies on the encrypted side), SNI-based filter-chain selection, upstream SNI/validation, and the TLS-parameter mapping — plus a one-sentence cross-reference to ADR-0028 in the adjacent **TCP proxy** subsection, resolving phase-02 REVIEW Minor 8.
9. A small fuzz target `internal/tls.FuzzTLSContextParse` covering the DownstreamTlsContext/UpstreamTlsContext Any parsers, satisfying §7.4 discipline for the phase's new parse surface.

After phase 03, the project has proven its fourth central engineering claim: *envoy-go speaks TLS — terminates it downstream, originates it upstream, and dispatches by SNI — on a deterministic workload that produces byte-equivalent plaintext to upstream Envoy's.* Every subsequent phase (HTTP/*, observability) layers on top of this TLS-capable dataplane.

## 2. Non-purposes

Phase 03 does **not** do any of the following. Each is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

- **Session tickets / session resumption.** `DownstreamTlsContext.session_ticket_keys`, `session_ticket_keys_sds_secret_config`, `disable_stateless_session_resumption`, `session_timeout` are all ignored (not errored). Go's `crypto/tls` enables session tickets by default with key rotation; phase-03 neither tunes nor disables them, nor exposes the key material. If any of these fields are set in a fixture, phase 03's parser records a diagnostic (at debug log level) but does not fail — matching upstream Envoy's forward-compatible posture on fields unrelated to the asserted surface. → TLS-family sub-phase, when a fixture asserts resumption semantics.
- **OCSP stapling.** `DownstreamTlsContext.ocsp_staple_policy` and `common_tls_context.tls_certificates[].ocsp_staple` are ignored. Go's `crypto/tls` supports `Certificate.OCSPStaple` as a static field but does not refresh. Phase 03 does not load, serve, or verify OCSP responses on either side. → TLS-family sub-phase.
- **mTLS (require_client_certificate).** `DownstreamTlsContext.require_client_certificate` must be absent (or `false`); setting it to `true` errors at listener build. `common_tls_context.validation_context.*` on the downstream side is ignored (no client-cert validation). The upstream side *does* consume `validation_context.trusted_ca` to validate server certs (§5.4) — this is phase-03 in-scope because upstream origination without server-cert validation would not match upstream Envoy. → TLS-family sub-phase for downstream client-cert flows.
- **SDS (Secret Discovery Service).** `common_tls_context.tls_certificate_sds_secret_configs`, `validation_context_sds_secret_config`, `combined_validation_context` all error at config build time. Phase 03 consumes secrets only inline (`DataSource.inline_bytes`, `inline_string`) or from local files (`DataSource.filename`). → xDS family.
- **SPIFFE / SAN URI trust bundles / custom validators.** `custom_validator_config`, `match_typed_subject_alt_names`, `verify_certificate_hash`, `verify_certificate_spki` all error. Phase-03 validation is CA-based (`trusted_ca`) plus hostname match (via `tls.Config.ServerName = sni`). → TLS-family sub-phase.
- **Post-quantum / hybrid key exchange.** `key_log`, post-quantum groups, and any non-stdlib curve configuration error. Phase 03 uses `crypto/tls`'s default curve preferences (P-256/X25519/P-384/P-521). → deferred unless a future phase requires it.
- **ALPN-driven filter-chain selection** (`filter_chain_match.application_protocols[]`). Errors at listener build. Phase 03 permits `common_tls_context.alpn_protocols[]` on the TLS config (passed through to `tls.Config.NextProtos`) — that is forwarded to peers — but matching filter chains on the *negotiated* ALPN is a phase-07 concern. → phase 07 filter-chain framework.
- **Non-SNI filter-chain match fields.** `destination_port`, `prefix_ranges`, `source_type`, `source_prefix_ranges`, `source_ports`, `direct_source_prefix_ranges` all error at listener build. Only `server_names[]` and `transport_protocol == "tls"` are permitted in phase 03. → phase 07.
- **`Listener.default_filter_chain`.** The Listener proto's top-level `default_filter_chain` field is rejected. Phase 03's catch-all is expressed by an entry in `filter_chains[]` whose `filter_chain_match` is empty (the "match-everything" predicate). This is a scope-limiting choice; Envoy permits both forms and prefers `default_filter_chain` for the role. Supporting both forms in phase 03 would double the match-resolution surface without a matching test win. → phase 07.
- **`listener_filters` (e.g., `tls_inspector`).** Still ignored silently (unchanged from phase 02). Phase 03's SNI peek happens inside the TLS handshake via `GetConfigForClient`, not via a pre-handshake inspector filter. → filter-chain-framework family.
- **HTTP over TLS (HTTPS).** Phase 03's TLS surface terminates into and originates from the TCP proxy filter only. No HTTP awareness; no HCM; no routing. → phase 04+.
- **Transport socket types other than `tls`.** `raw_buffer`, `starttls`, `proxy_protocol`, `quic`, `internal_upstream` all error at transport-socket decode. → relevant family phases.
- **Cluster types other than STATIC (subject side).** Unchanged from phase 02; STRICT_DNS remains reference-side only, per ADR-0010 + ADR-0027. → later phase.
- **LB policies other than ROUND_ROBIN.** Unchanged. → load-balancing family.
- **TLS access logging, handshake stats, cipher stats.** Stats land in phase 06. TLS-specific access log fields land with them. → phase 06 / observability family.
- **Graceful drain of in-flight TLS connections.** SIGINT behaviour unchanged from phase 02: listener sockets close, in-flight connections drop. → phase 08.
- **TLS inspection / passthrough mode.** Listeners do not sniff SNI to *pass through* to an upstream; they terminate. → feature-families, if ever.
- **Dynamic secret reload.** Committed PEMs are read once at listener-manager build time. No file-watch, no inotify, no reload signal. → xDS/SDS family.

## 3. Phase-done gates (specialization of §7.5)

Per doctrine `D-3.6`, phase 03 lands only when every gate below is green. The generic `BOOTSTRAP_PROMPT.md` §7.5 gate set is narrowed:

| Gate | Specialization for phase 03 |
|---|---|
| (a) new/changed differential fixtures green | New fixture `test/fixtures/0002-tls-tcp/` passes: byte-exact equivalence of decrypted response payload over 18 TLS round-trips (9 per SNI) through two 3-endpoint TLS-upstream clusters; per-proxy distribution assertion `[3, 3, 3]` per cluster per side. The handshake succeeds against the reference Envoy container using the committed test CA chain. |
| (b) all pre-existing differential fixtures still green | `0000-tcp-echo` and `0001-tcp-proxy-rr` pass without regression under their existing `expectations.yaml`. The TCP echo byte-exact and RR distribution-exact assertions still green. Admin `/ready` byte-exact still green on both. |
| (c) conformance suites pass | No conformance suite applies to phase 03 (h2spec is phase 05; h3spec later; grpc later; proxy-wasm later). This gate is vacuously green. |
| (d) new fuzzer runs clean for CI short-budget | New fuzz target `internal/tls.FuzzTLSContextParse` runs clean for its CI short-budget run (30-second policy inherited from ADR-0018). Phase-01 `internal/bootstrap.FuzzBootstrapLoad` and phase-02 `internal/filter/tcpproxy.FuzzTcpProxyFilter` also run clean (no regression). |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | Standard. Unit tests for `internal/tls/` (config parse, DataSource load, TLS parameter mapping, SNI match predicate), extended tests for `internal/listener/` (multi-chain + SNI-driven `GetConfigForClient`), extended tests for `internal/cluster/` (Dial abstraction plaintext + TLS), and extended tests for `internal/filter/tcpproxy/` (ctx-consumed upstream dial path) all part of `go test ./...`. |
| (f) `REVIEW.md` approved | Per `SKILL_ROUTING.md` state 5. |

## 4. Deliverables (files and directories)

Grouped by lifecycle. Every path below is either new or materially changed.

### 4.1 New production code

- **`internal/tls/config.go`** — parses a `*core.v3.TransportSocket` whose `typed_config` is `DownstreamTlsContext` or `UpstreamTlsContext`. Exports `NewDownstreamConfig(ts *corev3.TransportSocket) (*DownstreamConfig, error)` and `NewUpstreamConfig(ts *corev3.TransportSocket) (*UpstreamConfig, error)`. Each result carries an already-constructed `*stdtls.Config` (import alias for `crypto/tls`, since this package is itself named `tls`) plus any phase-03-observable fields. Every error begins with `tls: `.
- **`internal/tls/datasource.go`** — loader for `envoy.config.core.v3.DataSource`. Exports `loadDataSource(ds *corev3.DataSource) ([]byte, error)`. Supports `inline_bytes`, `inline_string`, and `filename` (filename relative path is resolved from the bootstrap file's directory — same discipline as Envoy's inline-vs-filename precedent; for the phase-03 differential harness, filename is unused because fixtures inline all PEMs). `environment_variable` errors with a clear "not supported in phase 03" message.
- **`internal/tls/params.go`** — TLS parameter mapping. Given `common_tls_context.tls_params`, fill `stdtls.Config.MinVersion`, `MaxVersion`, `CipherSuites`, and `CurvePreferences`. Map TLS version enums (TLSv1_2/TLSv1_3) to `stdtls.VersionTLS12/TLS13`; TLSv1_0 and TLSv1_1 error. Map IANA cipher-suite names to `stdtls.CipherSuites()` IDs; unknown names error; any TLS-1.3-only cipher in `cipher_suites` is a no-op with a diagnostic log (Go's crypto/tls does not expose TLS 1.3 cipher selection). Map `ecdh_curves` names (`X25519`, `P-256`, `P-384`, `P-521`) to `stdtls.CurveID`. `signature_algorithms` errors with a "not configurable via Go crypto/tls in phase 03" message (stdlib does not expose this knob publicly). See §5.5 for the full table; see ADR-E for rationale.
- **`internal/tls/sni.go`** — SNI-to-filter-chain match predicate. Exports `MatchServerName(patterns []string, sni string) bool` implementing Envoy's wildcard semantics: exact match wins over suffix wildcard; `"*.example.com"` matches `"foo.example.com"` but not `"example.com"`; `"*"` matches anything. Pure function; unit-tested exhaustively.
- **`internal/tls/config_test.go`** — unit tests for `NewDownstreamConfig`, `NewUpstreamConfig`: happy paths; bad type_url; missing tls_certificates; malformed PEM; CA-parse failure for upstream validation_context; error on mTLS (`require_client_certificate=true`); error on unsupported DataSource type; error on SDS config set.
- **`internal/tls/datasource_test.go`** — inline_bytes, inline_string, filename (with temp dir), environment_variable (errors), zero value (errors).
- **`internal/tls/params_test.go`** — TLS version enum mapping incl. error on TLSv1_0/v1_1; cipher-suite name-to-ID mapping incl. error on unknown; ecdh_curves mapping; signature_algorithms errors.
- **`internal/tls/sni_test.go`** — exhaustive SNI match table: exact, suffix wildcard (`*.example.com`), universal wildcard (`*`), no match, case-insensitive hostname match.
- **`internal/tls/fuzz_test.go`** — `FuzzTLSContextParse`. Seed corpus: one well-formed DownstreamTlsContext Any, one well-formed UpstreamTlsContext Any, one malformed (truncated) Any, one Any with a wrong type_url. Fuzz body: call `NewDownstreamConfig` and `NewUpstreamConfig` against the mutated bytes; assert no panic and that every returned error begins with `tls:`. Short-budget `-fuzztime=30s` per ADR-0018.

### 4.2 Changed production code

- **`internal/listener/manager.go`** — multi-filter-chain support (§5.2). The build-time filter-chain subset check is replaced: any positive number of chains is permitted. Per chain: `filter_chain_match` may be empty OR carry only `server_names[]` and optionally `transport_protocol == "tls"`; any other field errors. `transport_socket` may be nil (plaintext chain) OR carry a `DownstreamTlsContext` (TLS chain). A listener with ≥1 chain that has non-nil `transport_socket` is a TLS-terminating listener; its `filter_chain_match` fields become ClientHello-SNI predicates. A mixed listener (some chains with TLS, some without) errors — phase-03 requires all chains of a TLS listener to be TLS (simplicity). A plaintext-only listener (all chains `transport_socket` nil) behaves identically to phase 02 — but phase 03 permits multiple such chains only if each chain's match is empty or SNI-only (validated even though SNI cannot match on a plaintext connection; phase-03 rejects plaintext multi-chain as a likely configuration mistake — errors with a clear message). TLS listener startup: build a single server `*stdtls.Config` whose `GetConfigForClient(hello *stdtls.ClientHelloInfo) (*stdtls.Config, error)` callback (a) resolves the most-specific matching chain via `MatchServerName` (exact > suffix-wildcard > catch-all), (b) returns that chain's per-chain `*stdtls.Config`, (c) if no match, returns `nil` (Go's crypto/tls interprets as "use the listener-level config", but since the listener-level config is effectively this router, returning `nil` causes handshake failure with an "unknown server name" alert — matching upstream Envoy's behaviour for unmatched SNI on a chain-matching listener). The listener's Accept loop wraps each `net.Conn` in `stdtls.Server(conn, listenerConfig)`; the wrapped conn is the connection passed to the filter's `Handle` — but first the handshake is driven (see §5.3 for the exact wiring and why the handshake is driven outside `Handle` on the downstream side).
- **`internal/listener/manager_test.go`** — extended: multi-chain happy path; TLS + plaintext chains on the same listener errors; non-SNI `filter_chain_match` fields error; `default_filter_chain` set errors; `require_client_certificate=true` errors; unknown transport socket type errors; chain-with-invalid-PEM errors; `GetConfigForClient` routing correctness verified against a mocked ClientHelloInfo for each pattern shape.
- **`internal/cluster/cluster.go`** — `Cluster` gains an unexported `transportSocket` field; `Dial(ctx context.Context) (net.Conn, error)` method replaces the direct dial paths in callers. Plaintext clusters' `Dial`: `net.DialTimeout("tcp", ep.Addr(), connectTimeout)`. TLS clusters' `Dial`: dial TCP, then wrap in `stdtls.Client(tcp, upstreamCfg)`, then `HandshakeContext(ctx)`. On handshake error, close the TCP conn and return the error. The `connect_timeout` is applied to the TCP dial only; handshake is bounded by `ctx`. The resolved upstream `*stdtls.Config` (from `NewUpstreamConfig`) is shared across `Dial` calls and is safe for concurrent use per Go's crypto/tls contract.
- **`internal/cluster/manager.go`** — `buildCluster` now reads `cluster.transport_socket`. If set: decode as `UpstreamTlsContext` via `internal/tls.NewUpstreamConfig`; store on the cluster. If nil: cluster is plaintext. Other cluster-level fields (type, lb_policy, load_assignment) are unchanged.
- **`internal/cluster/cluster_test.go` / `manager_test.go`** — extended: `Dial` plaintext happy path; `Dial` TLS happy path against a `stdtls.Listen`-wrapped loopback echo server using the test-fixture PEMs; `Dial` TLS handshake failure (bad CA) returns a `cluster: tls:` wrapped error; mTLS (client-cert) is out-of-scope but loading a cluster with client certs in `UpstreamTlsContext` does not error (carried but unused at phase 03 — or explicitly ignored, decision in §10); build-time error on `transport_socket` whose type is not `tls`.
- **`internal/filter/tcpproxy/filter.go`** — `Handle(ctx, downstream)` replaces `net.DialTimeout` with `f.cluster.Dial(ctx)`. `ctx` is now consumed (via the cluster's HandshakeContext path on TLS clusters; on plaintext clusters, `ctx` is consumed by `f.cluster.Dial` wrapping `net.DialTimeout`'s deadline with a `select { case <-ctx.Done(): }` guard — or equivalently by short-circuiting early if `ctx.Err() != nil` before dial). This retires phase-02 REVIEW Minor 4. The pump body itself remains verbatim from ADR-0023.
- **`internal/filter/tcpproxy/filter_test.go`** — extended: ctx cancellation before dial returns promptly; TLS upstream cluster dial flows through without the filter noticing the transport is TLS (because `Cluster.Dial` returns a `net.Conn`-satisfying value either way).
- **`cmd/envoy-go/main.go`** — unchanged at the wiring level. The listener and cluster managers' extended behaviours are transparent to `main`.

### 4.3 New harness and fixture code

- **`test/differential/fixture/fixture.go`** — the `Driver` interface's single `Drive(ctx, refAddr, subjAddr)` method is split into `DriveReference(ctx context.Context, addr string) ([]byte, error)` and `DriveSubject(ctx context.Context, addr string) ([]byte, error)`. The `""` sentinel contract is gone. Phase-02 REVIEW Minor 6 resolved.
- **`test/differential/runner_test.go`** — call sites updated: `d.Drive(ctx, refAddr, "")` becomes `d.DriveReference(ctx, refAddr)`; `d.Drive(ctx, "", subjAddr)` becomes `d.DriveSubject(ctx, subjAddr)`. Blank-import for `test/fixtures/0002-tls-tcp/driver` added.
- **`test/fixtures/0000-tcp-echo/driver/driver.go`** — Drive split into two methods (the 0000 driver was already honouring the `""` sentinel via two guards; the split is a pure refactor of the same logic).
- **`test/fixtures/0001-tcp-proxy-rr/driver/driver.go`** — same split, same refactor.
- **`test/fixtures/0002-tls-tcp/`** — new fixture directory. Contents:
  - `envoy-go.yaml` — subject bootstrap. 1 listener (`l_tls`) binding `127.0.0.1:0`, with 2 TLS filter chains (`server_names: [alpha.envoy-go.test]` → cluster `c_alpha`; `server_names: [beta.envoy-go.test]` → cluster `c_beta`), each chain's `transport_socket` carrying a `DownstreamTlsContext` with that SNI's server cert + key inlined. Two upstream STATIC clusters (`c_alpha`, `c_beta`), each with three `lb_endpoints` and a `transport_socket` carrying an `UpstreamTlsContext` with `sni: alpha.envoy-go.test` (resp. `beta.envoy-go.test`), inline-CA validation_context, and no client certs.
  - `envoy.yaml` — reference bootstrap. Same listener shape, same SNI-indexed chains, same cert material inlined. Two STRICT_DNS clusters (same cert + CA material inlined) pointing at `host.docker.internal` with `dns_lookup_family: V4_ONLY` per ADR-0010.
  - `expectations.yaml` — response-body byte-exact on TLS-decrypted payload; all other dimensions not-applicable (no HTTP, no access log, no stats beyond /ready). Keeps the prose-heavy format from fixtures 0000/0001 — Minor 7 deferred per ADR-0019.
  - `pki/README.md` — PKI generation command (`go run ./pki/gen` or equivalent one-shot) with NotBefore/NotAfter spelled out.
  - `pki/gen/main.go` — a small Go program (NOT part of `go test ./...`) that regenerates every PEM in `pki/` deterministically when the test owner runs it. Not required to run in CI; the committed PEMs are the source of truth.
  - `pki/ca.pem`, `pki/server-alpha.pem`, `pki/server-alpha.key.pem`, `pki/server-beta.pem`, `pki/server-beta.key.pem`, `pki/upstream-alpha.pem`, `pki/upstream-alpha.key.pem`, `pki/upstream-beta.pem`, `pki/upstream-beta.key.pem` — ECDSA P-256 test certs, NotBefore 2026-01-01, NotAfter 2046-01-01 (20 years — overshoots the project's realistic lifespan). CA is self-signed; leaves are CA-signed. Server certs carry `dns_names: [alpha.envoy-go.test]` (resp. beta); upstream backend certs carry `dns_names: [alpha.envoy-go.test, localhost]` (resp. beta, localhost) so both the reference (via `host.docker.internal` alias of `alpha.envoy-go.test` injected via the fixture driver) and subject (via `localhost`) can validate. The subject side injects a `127.0.0.1` IP SAN additionally for crypto/tls's IP-address validation rules.
  - `README.md` — explains the fixture's purpose (downstream TLS + upstream TLS + SNI dispatch), the STATIC-vs-STRICT_DNS divergence (same as 0001 + ADR-0027), the PKI layout, the distribution-assertion methodology, and the `--concurrency 1` reference pin inherited from ADR-0028.
  - `driver/driver.go` — fixture driver. `BackendCount() = 6` (three per cluster; the runner allocates six ports). `SubjectListenerName() = "l_tls"`. `ReferenceListenerPort() = 15002`. `ReferenceBootstrap(backendPorts)` renders the reference YAML with `backendPorts[0..2]` as `c_alpha`'s three endpoints and `[3..5]` as `c_beta`'s. `SubjectConfig` does the same for STATIC 127.0.0.1 endpoints. `DriveReference(ctx, addr)` opens a TLS dialer against `addr` with `ServerName: "alpha.envoy-go.test"`, sends 9 plaintext payloads via the TLS tunnel, captures the responses; then does the same with `ServerName: "beta.envoy-go.test"`. Returns the concatenated plaintext bytes. `DriveSubject` does the same. `AssertDistribution(refCounts, subjCounts [6]uint64) error` checks each side's cluster-a counts are `[3,3,3]` and cluster-b counts are `[3,3,3]`. `ProbeAdmin` same as phase 02 / 01.
  - `driver/driver_test.go` — a small unit test covering the distribution-assertion helper without the harness startup cost (mirror of fixture 0001's test).
- **`test/helpers/tls.go`** — `TLSRoundTrip(ctx, addr, serverName string, clientCAs *x509.CertPool, payload []byte, idleTimeout time.Duration) ([]byte, error)`. TCPRoundTrip's TLS cousin: dial TLS with `stdtls.Config{ServerName: serverName, RootCAs: clientCAs, MinVersion: VersionTLS12}`, write payload, half-close (via TLS `CloseWrite`), read until EOF or idle timeout, return bytes.
- **`test/helpers/tls_test.go`** — round-trip against a loopback TLS echo server using the fixture's committed CA.

### 4.4 Changed documentation and state

- **`docs/envoy-go/ROADMAP.md`** — phase 03 row: `status: in-progress` during work (SPEC stage already satisfies), transitions to `done` at commit.
- **`docs/envoy-go/STATE.md`** — updated at each lifecycle transition (SPEC written → state 2, PLAN written → state 3, …, phase done → active-phase advances to `04-http-1.1`).
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — add new subsection **TLS** covering: (a) plaintext-byte-equivalence on decrypted payload; (b) handshake-layer *non*-comparison rule (what proxies may differ on the encrypted side — cipher selection, session ticket key material, handshake timing — vs what they must match — negotiated ALPN if present, certificate sent for a given SNI, TLS record boundaries NOT asserted); (c) SNI-based filter-chain selection as an assertion (same SNI → same chain on both proxies); (d) upstream SNI + validation as an assertion (same SNI sent; same CA used). Plus, in the adjacent **TCP proxy** subsection: append the one-sentence cross-reference to ADR-0028 per phase-02 REVIEW Minor 8.
- **`docs/envoy-go/DECISIONS.md`** — new ADRs introduced by phase 03 (numbers assigned at planning/implementation time; the planner may adjust). Anticipated:
  - **ADR-A:** TLS stack selection — stdlib `crypto/tls`. Options considered: (A1) `crypto/tls`, (A2) BoringSSL via cgo, (A3) third-party rustls/openssl bindings. A1 chosen: no cgo (preserves the project's pure-Go build posture); TLS 1.2/1.3 parity with Envoy's defaults; ALPN + SNI + peer-validation all natively supported; license-clean; no vendoring. Known tradeoffs documented: TLS 1.3 ciphersuite selection not configurable (RFC 8446 design — upstream by default, Envoy's knob becomes a no-op on the subject); `signature_algorithms` not publicly configurable in `crypto/tls` (phase 03 errors if a fixture sets it). These tradeoffs inform §5.5 and BEHAVIOR_CONTRACT TLS subsection. Supersedes nothing.
  - **ADR-B:** Phase-03 filter-chain subset — **supersedes ADR-0025**. Permitted: 1..N filter_chains per listener. `filter_chain_match` may be empty (catch-all, at most one per listener) OR carry only `server_names[]` and optionally `transport_protocol == "tls"`. All other match fields error. `Listener.default_filter_chain` still rejected. Selection at handshake: most-specific SNI match > suffix-wildcard match > catch-all (empty-match) > no match (handshake fails, connection closes). All chains on a TLS listener must be TLS; mixed TLS/plaintext on one listener errors.
  - **ADR-C:** Upstream TLS dialer model — `Cluster.Dial(ctx) (net.Conn, error)` abstracts plaintext vs TLS dial. Plaintext path: `net.DialTimeout`. TLS path: TCP dial + `stdtls.Client` + `HandshakeContext(ctx)`. Filter sees only `net.Conn`. Rationale: keeps the filter transport-agnostic for phase 04's HTTP-over-TLS. Supersedes nothing; the phase-02 `tcpproxy` filter's direct `net.DialTimeout` is retired as a consequence (no separate ADR).
  - **ADR-D:** DataSource handling policy — phase 03 supports `inline_bytes`, `inline_string`, and `filename` (resolved relative to the bootstrap file). `environment_variable` errors; SDS-bound secret configs error. Filename support is included from phase 03 (not deferred) because the cost is trivial and future phases will need it for non-test deployments.
  - **ADR-E:** TLS parameter mapping scope — documents the `tls_params` fields honoured by phase 03 vs ignored-with-diagnostic vs errored. Honoured: `tls_minimum_protocol_version` and `tls_maximum_protocol_version` (TLSv1_2/TLSv1_3 map to `stdtls.VersionTLS12/TLS13`; other values error); `cipher_suites` (IANA-name → `stdtls.CipherSuiteID` mapping, errors on unknown names, diagnostic-only for TLS-1.3-only ciphers since Go doesn't allow selecting them); `ecdh_curves` (names `X25519`/`P-256`/`P-384`/`P-521` → `stdtls.CurveID`, other values error); `alpn_protocols` on `common_tls_context` → `stdtls.Config.NextProtos`. Errored: `signature_algorithms` (not publicly configurable in Go `crypto/tls`). BEHAVIOR_CONTRACT TLS subsection records the divergences.
  - **ADR-F:** Fixture-driver interface split — `Drive(ctx, refAddr, subjAddr)` retired; `DriveReference(ctx, addr)` + `DriveSubject(ctx, addr)` introduced. **Supersedes (informal)** the phase-02 `fixture.Driver` interface codified in `test/differential/fixture/fixture.go` (no prior formal ADR; phase-02 REVIEW Minor 6 surfaced the issue). All fixture drivers (0000, 0001, new 0002) update in the same commit as the interface change.
  - **ADR-G:** BEHAVIOR_CONTRACT TLS subsection — codifies the TLS equivalence surface (see §5.7 below). Includes the phase-02 REVIEW Minor 8 cross-reference fix to the adjacent TCP proxy subsection in the same commit. Supersedes nothing.
  - If additional decisions emerge at plan or implementation time (e.g., a test-PKI rotation policy, a handshake-error wrapping policy, a listener-bind-over-TLS error cascade), they are ADR'd at that point. Expected starting ADR number is ADR-0029 based on phase-02's ADR-0028 tail; the planner verifies at write time.

## 5. Architecture and components

### 5.1 Module graph (new / changed shape)

```
               cmd/envoy-go/main.go
              /        |          \
             /         |           \
bootstrap.Load   admin.Server   listener.Manager
                                        |
                                   filter registry (inline, unchanged)
                                        |
                               internal/filter/tcpproxy.Filter
                                        |
                                cluster.Manager ──► cluster.Cluster ──► roundRobin LB
                                          |                    |
                                          |                    └──► Cluster.Dial
                                          |                           (plaintext | TLS)
                                          ▼
                                     internal/tls
                                        │
                                        ├── config.go (DownstreamConfig, UpstreamConfig)
                                        ├── datasource.go (inline_bytes / inline_string / filename)
                                        ├── params.go (tls_params → stdtls.Config)
                                        └── sni.go (MatchServerName wildcard rules)
```

Imports: `internal/tls` depends on `stdtls = crypto/tls`, `crypto/x509`, `envoy/extensions/transport_sockets/tls/v3` (proto types), `envoy/config/core/v3` (for DataSource). `listener` adds a dependency on `internal/tls` (for `NewDownstreamConfig` and `MatchServerName`). `cluster` adds a dependency on `internal/tls` (for `NewUpstreamConfig`). `filter/tcpproxy` loses its direct `net.DialTimeout` call — it now delegates to `cluster.Dial`. No cyclic imports. Package-level import of `crypto/tls` is aliased as `stdtls` everywhere `internal/tls` is in scope to avoid name collision.

### 5.2 Listener manager — multi-chain + SNI routing

**Build-time changes (`NewManager`):** the phase-02 "exactly one filter_chain / empty match / no transport_socket" check from ADR-0025 is replaced by the phase-03 subset (ADR-B):

1. `filter_chains` must be ≥ 1.
2. For each chain:
   - `filter_chain_match` may be nil/empty (catch-all) OR populate only `server_names[]` and optionally `transport_protocol` (which must equal `"tls"` if set). Any other populated field errors.
   - `transport_socket` may be nil (plaintext chain) or carry a `DownstreamTlsContext` (TLS chain).
   - Exactly one `filters[]` entry whose `typed_config` type_url is the TCP proxy URL (same as phase 02). The filter is built against the cluster manager as before.
3. At most one chain per listener may have an empty/nil match (the catch-all).
4. If *any* chain has a non-nil `transport_socket`, *every* chain on that listener must have one — a mixed listener errors. (Simplification — Envoy permits mixing; phase 03 does not.)
5. `Listener.default_filter_chain` set → error.
6. `listener_filters` → ignored silently (phase-02 carry-over).

**TLS-listener Start:** for every TLS listener, the manager composes a server-side `*stdtls.Config` whose fields are all stubs *except* `GetConfigForClient`. The callback:

```go
func (hello *stdtls.ClientHelloInfo) (*stdtls.Config, error) {
    sni := hello.ServerName          // lowercased by crypto/tls
    for _, c := range chainsByMostSpecificFirst {
        if tls.MatchServerName(c.serverNames, sni) {
            return c.tlsConfig, nil  // chain-specific config
        }
    }
    if catchAll != nil {
        return catchAll.tlsConfig, nil
    }
    return nil, fmt.Errorf("listener %q: no chain matches SNI %q", listenerName, sni)
}
```

The accept loop wraps each accepted `net.Conn` with `stdtls.Server(raw, serverCfg)`; the resulting `*stdtls.Conn` is the "downstream" handed to the chain's filter. The handshake is driven *before* the filter sees the connection — either (a) synchronously in the accept loop (blocks the Accept goroutine, simpler) or (b) inside the per-connection worker goroutine (does not block Accept; preferred — SPEC §10 #1 settled by planner). Either way, the SNI-based chain resolution happens during `HandshakeContext`; the filter dispatched to is the one the callback selected. This means the accept loop must learn the chosen chain *after* the handshake (via an auxiliary `atomic.Pointer` set by the callback closure, keyed by the connection). See §10 #1 for the planner's call between (a) and (b).

**Plaintext-listener Start:** identical to phase 02 — the single chain's filter receives raw `net.Conn`s. If a plaintext listener has ≥ 2 chains declared, build errors (per §4.2 bullet on mixed listeners).

### 5.3 TLS listener handshake wiring (the subtle bit)

For a TLS listener, the sequence per accepted connection is:

1. Accept loop receives `raw net.Conn`.
2. Worker goroutine takes over: `stdtlsConn := stdtls.Server(raw, listenerCfg)`.
3. Worker calls `stdtlsConn.HandshakeContext(ctx)`. Inside that handshake, Go's crypto/tls invokes `listenerCfg.GetConfigForClient(hello)` — our callback returns the chain-specific `*stdtls.Config` *and* stores the selected chain's filter pointer in the `stdtls.ClientHelloInfo.Context()`-associated map (via a `connKey`-keyed registry on the listener struct, set before the return and fetched after `HandshakeContext` returns). An alternative — and preferred — approach: the callback returns a `*stdtls.Config` whose own `VerifyConnection` (a stdlib hook called post-handshake) records the chain into a per-connection context. SPEC §10 #2 defers the exact mechanism to the planner; both approaches are valid Go.
4. On handshake success, worker invokes `chain.filter.Handle(ctx, stdtlsConn)`. The filter treats the TLS conn as a `net.Conn` — CloseWrite still works on `*stdtls.Conn`, preserving the half-close semantics from ADR-0023.
5. On handshake error, worker closes `raw` and logs `listener %q: handshake: %v`.

The listener does not maintain a TLS session cache beyond crypto/tls's defaults. Session ticket rotation uses crypto/tls's built-in 24-hour ticket-key lifetime. None of this is asserted across proxies by the differential contract (see §5.7 BEHAVIOR_CONTRACT TLS additions).

### 5.4 Cluster — upstream TLS dialer

`Cluster` exposes:

```go
func (c *Cluster) Dial(ctx context.Context) (net.Conn, error)
```

For plaintext clusters: `net.DialTimeout("tcp", ep.Addr(), c.connectTimeout)` with a `ctx.Err()` guard before the dial. (The `Dialer{Timeout, Deadline}` form via `DialContext` handles both uniformly; preferred.)

For TLS clusters: `Dialer{Timeout: c.connectTimeout}.DialContext(ctx, "tcp", ep.Addr())`, then `stdtls.Client(tcpConn, c.upstreamCfg)`, then `HandshakeContext(ctx)`. On handshake error, close `tcpConn`. The upstream `*stdtls.Config` carries `ServerName = sni` (from `UpstreamTlsContext.sni`), `RootCAs = x509.NewCertPool() with CA bytes from validation_context.trusted_ca`, `NextProtos = common_tls_context.alpn_protocols[]` if set, and `MinVersion/MaxVersion/CipherSuites/CurvePreferences` from `common_tls_context.tls_params`.

If `validation_context.trusted_ca` is unset, `Dial` errors at build time (phase 03 requires server-cert validation on every TLS cluster — a conscious tightening beyond what Envoy permits for some configurations). Rationale: a TLS cluster that doesn't validate its upstream is a silent downgrade; at phase 03 we would rather the fixture be explicit.

### 5.5 TLS parameter mapping (authoritative per-field table)

This table is the phase-03 authority; any divergence found during implementation must either update this table or extend via ADR.

| Envoy field | Phase-03 behaviour |
|---|---|
| `tls_params.tls_minimum_protocol_version` | Honoured. `TLSv1_2`/`TLSv1_3` → `stdtls.VersionTLS12/TLS13`. `TLSv1_0`, `TLSv1_1` → error. `TLS_AUTO` → no-op (treat as unset, per ADR-0030 — the proto-zero ambiguity prevents distinguishing "unset" from "explicitly chosen"). |
| `tls_params.tls_maximum_protocol_version` | Honoured. Same mapping. |
| `tls_params.cipher_suites` | Honoured for TLS 1.2 suites (IANA name → `stdtls.CipherSuites()` ID; unknown name errors). TLS-1.3-only suites logged as diagnostic and dropped — `crypto/tls` does not allow TLS 1.3 cipher selection. |
| `tls_params.ecdh_curves` | Honoured. Names `X25519`/`P-256`/`P-384`/`P-521` → `stdtls.CurveID`. Unknown error. |
| `tls_params.signature_algorithms` | **Error**. `crypto/tls` does not expose a public configuration knob. ADR-E. |
| `common_tls_context.tls_certificates[].certificate_chain` | Honoured as DataSource per §4.1 `datasource.go`. |
| `common_tls_context.tls_certificates[].private_key` | Honoured as DataSource. |
| `common_tls_context.tls_certificates[].password` | **Error**. Phase-03 does not decrypt password-protected keys. |
| `common_tls_context.tls_certificates[].ocsp_staple` | Ignored (no-op + diagnostic). |
| `common_tls_context.tls_certificate_sds_secret_configs` | **Error**. SDS = xDS family. |
| `common_tls_context.validation_context.trusted_ca` | Honoured on upstream (sets `RootCAs`). Ignored on downstream (no client-cert validation in phase 03). |
| `common_tls_context.validation_context.match_typed_subject_alt_names` | **Error**. Custom SAN matching deferred. |
| `common_tls_context.validation_context.verify_certificate_{hash,spki}` | **Error**. |
| `common_tls_context.validation_context.custom_validator_config` | **Error**. |
| `common_tls_context.alpn_protocols` | Honoured → `stdtls.Config.NextProtos` on both sides. |
| `DownstreamTlsContext.require_client_certificate` | If `true` → error. If `false` or unset → ignored (no-op). |
| `DownstreamTlsContext.session_ticket_keys*` | Ignored (crypto/tls defaults apply). |
| `DownstreamTlsContext.session_timeout` | Ignored. |
| `DownstreamTlsContext.disable_stateless_session_resumption` | Ignored. |
| `DownstreamTlsContext.ocsp_staple_policy` | Ignored. |
| `UpstreamTlsContext.sni` | Honoured → `stdtls.Config.ServerName`. |
| `UpstreamTlsContext.allow_renegotiation` | **Error** if true. Go's crypto/tls does not support TLS 1.2 renegotiation as a client; erroring rather than silently no-op'ing avoids a confusing downgrade. |

### 5.6 TCP proxy filter — transport-agnostic Handle

`Filter.Handle(ctx, downstream)` keeps its shape but now:

1. Early-returns if `ctx.Err() != nil`.
2. Picks endpoint (unchanged).
3. Replaces `net.DialTimeout` with `f.cluster.Dial(ctx)`. This is the *sole* code change in the filter body.
4. Pumps bytes via the ADR-0023 verbatim `netConn{}` + two-goroutine `io.Copy` + `halfClose` dance. No change.

Because `*stdtls.Conn` satisfies `net.Conn` and its `CloseWrite()` method is honoured, `halfClose` continues to work: the wrapper prefers `*net.TCPConn.CloseWrite`, but a simple type-switch extension (fall back to `*stdtls.Conn.CloseWrite`) is the only adjustment required. This is a one-line change in `halfClose`.

### 5.7 BEHAVIOR_CONTRACT — new TLS subsection (content preview)

The phase's BEHAVIOR_CONTRACT addition (subject of ADR-G) codifies:

- **Asserted:** plaintext response-body byte-equivalence on TLS-terminated, TLS-originated connections (same rule as phase 02's TCP proxy body-equivalence, applied post-decryption). Chain-selection equivalence: given the same SNI, both proxies must select the logically-equivalent chain and dispatch to the logically-equivalent upstream cluster (witnessed indirectly via distribution assertion per SNI).
- **Asserted:** on the upstream side, both proxies send the same SNI for a given cluster (the fixture's `sni` field is consumed identically; any divergence shows up as an upstream handshake failure, breaking the differential gate). Both proxies validate against the same CA (same `trusted_ca` material in both configs).
- **Not asserted:** encrypted-side equivalence — TLS record boundaries, session ticket material, ticket-key rotation timing, TLS 1.3 cipher selection (Go vs BoringSSL defaults differ), handshake message byte ordering/timing, server random, session IDs. These are free to differ.
- **Not asserted:** negotiated ALPN. ALPN pass-through is honoured on both sides (both proxies include `alpn_protocols` in their `NextProtos`); the negotiated value is not surfaced to the fixture driver in phase 03.
- **Asserted conditionally:** the server certificate *identity* (subject CN / SAN) sent to a client for a given SNI must match on both proxies. Not a byte-compare of the cert (they are the same committed PEM — trivially equal); rather, a rule that says "both proxies pick the cert whose SAN matches the SNI."
- **Cross-reference:** the adjacent **TCP proxy** subsection is amended in the same commit to append: "*Reference-side distribution exactness (fixture `0001-tcp-proxy-rr` and, inherited, `0002-tls-tcp`) depends on the reference container's `--concurrency 1` pin per ADR-0028.*" Resolves phase-02 REVIEW Minor 8.

### 5.8 Fixture `0002-tls-tcp` — two-SNI TLS round-trip

**Topology:**

- Six host-side test backends, each a `stdtls.Listen`-wrapped TCP echo server. Three use the `upstream-alpha` cert; three use the `upstream-beta` cert. Each holds its own `atomic.Uint64` accept counter.
- Reference Envoy container's bootstrap declares one TLS listener (`l_tls`) with two SNI-indexed chains, each chain's `DownstreamTlsContext` inlining `server-alpha`/`server-beta` PEMs and their private keys. Two STRICT_DNS clusters (`c_alpha`, `c_beta`), each originating TLS with `sni: alpha.envoy-go.test` / `beta.envoy-go.test`, each validating upstream cert against the inline CA. Runner substitutes the six backend ports before container start; `dns_lookup_family: V4_ONLY` applies.
- Subject's bootstrap declares the same topology with STATIC clusters at `127.0.0.1` + six distinct `port_value`s. Driver's `SubjectConfig` injects them at runtime. Same cert material inlined.

**Drive sequence:**

1. Reset all six backend counters.
2. `DriveReference(ctx, refAddr)`:
   - Open a TLS dialer with `ServerName: "alpha.envoy-go.test"`, send 9 plaintext payloads `"rr-alpha-<0..8>\n"`, capture responses.
   - Open a TLS dialer with `ServerName: "beta.envoy-go.test"`, send 9 plaintext payloads `"rr-beta-<0..8>\n"`, capture responses.
   - Return the concatenated response stream (alpha's 9 + beta's 9).
3. Snapshot `refCounts [6]uint64`.
4. Reset counters.
5. `DriveSubject(ctx, subjAddr)`: identical drive shape.
6. Snapshot `subjCounts [6]uint64`.
7. Differential diff on concatenated response streams — byte-exact equivalence (echo round-trip, plaintext-after-TLS).
8. Distribution assertion: each side, cluster-a counters = `[3, 3, 3]`, cluster-b counters = `[3, 3, 3]`. Exact, no tolerance. `N % 3 == 0` design preserved.

**Why two SNIs and not one:** the fixture exercises (i) downstream termination, (ii) upstream origination, and (iii) SNI-based filter-chain selection in a single fixture run. Splitting into two fixtures doubles harness cost without proportional coverage win. Both SNIs route to their own cluster, so the distribution assertion also covers per-cluster RR correctness in the presence of multiple clusters — a property phase 02 unit-tested but did not fixture-test.

**Why SNI `*.envoy-go.test`:** RFC 6761 reserves `.test` for non-routable names. Using a fictitious TLD avoids any accidental DNS lookup on developer workstations or CI runners.

## 6. Data flow

### 6.1 Startup

```
1. main loads bootstrap → *bootstrapv3.Bootstrap (unchanged).
2. main builds cluster.Manager:
   - For each cluster with transport_socket set: parse UpstreamTlsContext,
     build upstream *stdtls.Config, store on Cluster.
3. main starts admin server (unchanged).
4. main builds listener.Manager:
   - For each listener:
     - Enumerate chains.
     - For chains with transport_socket: parse DownstreamTlsContext, build
       per-chain *stdtls.Config.
     - Build the terminal filter for each chain (unchanged).
     - If TLS listener: compose the top-level *stdtls.Config with
       GetConfigForClient routing callback.
5. main calls lm.Start — bind sockets (unchanged).
6. main marks admin ready, prints sentinels (unchanged).
```

### 6.2 Connection (TLS listener)

```
1. Accept goroutine receives raw net.Conn C.
2. Worker goroutine: wrap C in stdtls.Server(C, listenerCfg).
3. Call HandshakeContext(ctx). Inside:
   - GetConfigForClient(hello) fires; SNI → chain lookup → per-chain *stdtls.Config.
   - Chain pointer recorded per-connection (§10 #2 settles how).
4. Handshake success → worker calls chain.filter.Handle(ctx, stdtlsConn).
5. filter.Handle:
   - cluster.Dial(ctx) → either raw net.Conn (plaintext) or handshaked *stdtls.Conn (TLS).
   - Pump bytes bidirectionally with half-close propagation.
6. Handshake failure → close C, log, return.
```

### 6.3 Connection (plaintext listener, unchanged)

Same as phase 02 — the single chain's filter receives the raw `net.Conn`.

### 6.4 Shutdown

Unchanged from phase 02: SIGINT cancels ctx; `lm.Stop()` closes listeners; in-flight TLS connections drop when the main goroutine returns (no graceful drain, no session ticket flush). → phase 08.

## 7. Error handling and failure modes

Single rule (preserved): every error crossing a package boundary begins with `<package>: ` (`tls:`, `listener:`, `cluster:`, `tcpproxy:`).

| Failure site | Class | Handling |
|---|---|---|
| `tls.NewDownstreamConfig` / `NewUpstreamConfig`: wrong type_url, unmarshal error, bad PEM, `require_client_certificate=true`, SDS-bound secret, disallowed DataSource kind, `signature_algorithms` set, password-protected key, etc. | build-time | Return error; surfaced via listener/cluster manager with `<pkg>: ` wrap. |
| `listener.NewManager`: non-SNI match field, mixed TLS/plaintext chains, ≥2 catch-all chains, `default_filter_chain` set, unknown transport_socket type, TLS config build error, duplicate listener name, unknown filter type_url, etc. | build-time | Return error; `main` logs and exits non-zero. |
| `cluster.NewManager`: TLS cluster without `trusted_ca`, unknown transport_socket type, TLS config build error, any phase-02 cluster error | build-time | Same. |
| `listener` Start: bind error | startup | Same unwind behaviour as phase 02. |
| `listener` TLS handshake: `GetConfigForClient` returns no match; cert rejected by client; protocol version mismatch; etc. | runtime | Log `listener %q: handshake: %v`; close raw conn; do not invoke filter. |
| `cluster.Dial`: TCP dial failure | runtime | Propagate; filter logs and closes downstream. |
| `cluster.Dial`: TLS handshake failure (CA reject, SNI mismatch, protocol mismatch) | runtime | Propagate with `cluster: tls: handshake: %v` wrap; filter logs and closes downstream. |
| `filter.Handle` pump error | runtime | Unchanged from phase 02 (silently dropped by `_ = io.Copy`). |
| SIGINT | shutdown | Unchanged. |
| Bootstrap loader on `dynamic_resources` / `layered_runtime` | build-time | Unchanged (phase-01 behaviour preserved). |

## 8. Testing scope for phase 03

Three layers.

### 8.1 Unit tests

- `internal/tls/config_test.go`, `datasource_test.go`, `params_test.go`, `sni_test.go`, `fuzz_test.go` — coverage enumerated in §4.1.
- `internal/listener/manager_test.go` — extended for multi-chain, SNI match, TLS handshake config construction, build-time error cases.
- `internal/cluster/cluster_test.go` + `manager_test.go` — `Dial` plaintext and TLS, build-time error cases.
- `internal/filter/tcpproxy/filter_test.go` — ctx consumption, TLS-upstream transparency.
- `cmd/envoy-go/main_test.go` — unchanged; reuses phase-02 two-listener bootstrap harness.

### 8.2 Fixture-level (differential)

- `test/fixtures/0000-tcp-echo/` — unchanged behaviour; driver refactored for Drive→DriveReference/DriveSubject split.
- `test/fixtures/0001-tcp-proxy-rr/` — same.
- `test/fixtures/0002-tls-tcp/` — new; two-SNI TLS round-trip with byte-exact plaintext equivalence and per-cluster distribution assertion.

### 8.3 Conformance

None for phase 03.

## 9. Out-of-scope (explicitly deferred)

All items in §2 remain deferred. Additionally:

- **Dynamic PKI reload.** Committed PEMs are static per fixture run. → xDS/SDS family.
- **TLS 1.3 cipher selection tuning.** Stdlib does not expose it. → recorded in ADR-E; later phase may revisit if Go's crypto/tls public API changes.
- **Post-handshake stats surface.** No cipher/ALPN/peer-cert stats. → phase 06.
- **Graceful termination of in-flight TLS connections at SIGTERM.** → phase 08.
- **PROXY protocol v1/v2.** Not a phase-03 concern; no fixture exercises it. → later phase under a transport-related family.
- **TLS passthrough (proxy without termination).** Not in scope. → feature-families if ever.

## 10. Deferred decisions (the planner / implementer settles these)

These are intentionally left open for the planning or implementation session to decide. None change the shape of the SPEC; all are implementation-detail choices whose outcome is recorded in PLAN.md or as PROGRESS/ADR notes.

1. **Handshake placement.** Either (a) accept loop performs `HandshakeContext` synchronously before dispatching to the filter's goroutine, or (b) per-connection worker goroutine performs it. The SPEC assumes (b) (non-blocking Accept). The planner may choose (a) with a one-liner rationale in PLAN.md. No ADR required unless the choice has cross-phase impact.
2. **Chain selection propagation from callback to filter dispatch.** Two equivalent approaches: (A) closure-captured `sync.Map[*stdtls.Conn]*chainInfo` populated inside `GetConfigForClient`; (B) a fresh `*stdtls.Config` per callback whose `VerifyConnection` hook records chain identity via a `context.WithValue` pattern. (A) is simpler; (B) is more idiomatic but requires wrapping `HandshakeContext` in a custom context. Planner picks; recommend (A) for phase 03.
3. **Upstream `validation_context.trusted_ca` mandatory-vs-optional.** SPEC §5.4 tightens beyond Envoy: trusted_ca is required on every TLS cluster. The planner confirms or relaxes this (with an ADR if relaxing — phase-03 validation permissiveness has security implications).
4. **Password-protected keys.** SPEC errors; planner may relax to "ignore password, try un-encrypted parse" if a fixture demands it. No such demand in the phase-03 fixture.
5. **Fuzz seed corpus size beyond the four initial entries.** Extensible; the planner may add a fifth/sixth for coverage of exotic DataSource shapes.
6. **PKI regeneration tool location.** `test/fixtures/0002-tls-tcp/pki/gen/main.go` is the SPEC recommendation; the planner may instead extract to `test/helpers/pki/gen/` if a second TLS fixture follows. Phase-03 has one TLS fixture; either location works.
7. **TLS 1.2 vs TLS 1.3 floor for the fixture.** Recommend `TLSv1_2` minimum on both downstream and upstream with `TLSv1_3` maximum (Go picks 1.3). Matches Envoy's v1.37.2 default floor. Planner confirms.
8. **Cluster connect_timeout vs handshake timeout.** The SPEC applies `connect_timeout` only to the TCP dial and bounds the handshake by `ctx`. An alternative: a separate handshake-timeout field (Envoy doesn't expose one at the cluster level in phase-03 scope). Planner picks; recommend the SPEC's approach.
9. **ADR numbering.** The SPEC lists anticipated ADRs A–G with explicit purposes. At landing time, the planner assigns sequential numbers starting from the current highest ADR + 1 (expected ADR-0029..ADR-0035 based on phase-02's ADR-0028 tail).
10. **BEHAVIOR_CONTRACT TCP proxy cross-link placement.** Phase-02 REVIEW Minor 8's one-liner goes in the existing "LB endpoint-selection sequence (NOT asserted)" paragraph; planner verifies position at write time.
11. **Handling of `allow_renegotiation=true`.** SPEC errors. Planner may choose "ignore with diagnostic" if desired; erroring is the safer default.

## 11. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Test PKI expiry silently fails a future CI run. | NotBefore 2026-01-01, NotAfter 2046-01-01 — 20-year window. README notes the expiry; a future phase can regenerate. |
| Go's `crypto/tls` rejecting a cert that upstream Envoy (BoringSSL) accepts, or vice versa. | Test certs are RFC-conformant ECDSA P-256 with SAN set; both BoringSSL and Go accept. Fixture validation runs both proxies against the same CA material. |
| `host.docker.internal` SAN matching on the reference side vs `127.0.0.1` / `localhost` on the subject side. | Upstream leaf certs carry BOTH SAN sets (`alpha.envoy-go.test` + `localhost` + `127.0.0.1` IP SAN for subject; `alpha.envoy-go.test` matches reference-side SNI which is `alpha.envoy-go.test` — the DNS resolution via `host.docker.internal` happens at the transport layer, not at cert validation; the cert's CN/SAN only needs to match the *SNI* the client sent, not the IP/hostname it connected to). This is a standard TLS+SNI design and matches Envoy's behaviour. |
| TLS 1.3 cipher selection diverging between crypto/tls and BoringSSL causing encrypted-side byte differences. | BEHAVIOR_CONTRACT §TLS explicitly does NOT assert encrypted-side byte equivalence. Plaintext-after-decryption is what the fixture asserts; the differential harness diffs decrypted bytes. |
| `--concurrency 1` pin from ADR-0028 not inherited for fixture 0002, causing per-worker RR randomization on the reference side. | ADR-0028 is container-wide for the reference image (set in `test/differential/harness.go` StartReferenceProxy). Fixture 0002 inherits it automatically (same code path). Phase-03 SPEC explicitly calls out this inheritance so the planner verifies the harness doesn't need a new per-fixture opt-in. |
| ClientHelloInfo SNI being lowercased / normalized by crypto/tls, causing a case-sensitivity mismatch vs the fixture's `server_names` config. | `MatchServerName` normalizes both sides to lowercase before comparing. Unit-tested. |
| `halfClose` on `*stdtls.Conn` not triggering a TCP FIN because the TLS layer needs a close_notify alert first. | `*stdtls.Conn.CloseWrite()` does exactly this: sends a `close_notify` alert then half-closes the underlying TCP. The existing `halfClose` type-switch extends to include the TLS conn case. Unit-tested in `filter_test.go`. |
| Upstream handshake deadlocking Handle under ctx cancellation if `HandshakeContext` doesn't honour ctx. | `crypto/tls.HandshakeContext` honours ctx cancellation per its documentation. The unit test verifies by creating a blocked-read TLS server and cancelling mid-handshake. |
| Fuzz discovering a panic in a TLS context parse path (e.g., malformed DataSource). | `FuzzTLSContextParse` runs in CI short-budget and nightly long-budget; ADR-0018 policy applies. |
| Fixture 0002 flaking because of TLS session resumption changing the handshake trace between runs. | BEHAVIOR_CONTRACT explicitly does not assert handshake trace equivalence. The plaintext body assertion is stable across resumption or full handshake. |
| Phase-02 REVIEW Minor 5 (readyListenerAddrs goroutine leak) resurfacing under TLS listener's longer startup. | The ready sentinel path is unchanged from phase 02 — the listener manager prints sentinels *after* the top-level tls.Config is composed and bound. Minor 5 remains deferred; no new risk. |

## 12. Phase-02 REVIEW carryover triage

Phase-02 REVIEW ("Phase 02 — TCP Proxy Review", §Findings/Minor) lists eight Minors. Phase-03 triage:

1. **Minor 1 (SPEC author scrutiny of worker-count assumptions).** RESOLVED IN SPEC. §5.8 and §11 explicitly call out the `--concurrency 1` inheritance. BEHAVIOR_CONTRACT TLS subsection records the reference-side single-worker dependency.
2. **Minor 2 (ADR-0028 bundling bug fix in Consequences).** NO-ACTION. Phase-03 ADRs A–G are each single-concern; the ADR-bundling anti-pattern is simply avoided here. No project-level discipline ADR needed.
3. **Minor 3 (ADR number-vs-physical-order drift).** DEFERRED. Phase-03 appends ADRs sequentially at file tail per prior practice; numerical order is authoritative (BOOTSTRAP_PROMPT §4.1 invariant 4). If the drift becomes painful, a later phase can either adopt the two-step reorder pattern the reviewer proposed or formalize acceptance via a doctrine note in BOOTSTRAP_PROMPT. Phase 03 does neither.
4. **Minor 4 (`Filter.Handle` `ctx` unused).** RESOLVED. Phase-03 `cluster.Dial(ctx)` consumes `ctx` (plaintext path: `DialContext`; TLS path: additionally `HandshakeContext(ctx)`). An early `ctx.Err()` guard in `Handle` formalizes the consumption. No ADR needed — the change is a natural consequence of ADR-C.
5. **Minor 5 (`readyListenerAddrs` goroutine leak).** DEFERRED. Phase 03 does not touch the ready sentinel path. A future phase or a dedicated cleanup commit resolves.
6. **Minor 6 (`""` sentinel in fixture `Drive`).** RESOLVED. ADR-F splits `Drive` into `DriveReference` + `DriveSubject`. All existing drivers (0000, 0001) and the new driver (0002) land on the new interface in the same commit.
7. **Minor 7 (prose-heavy `expectations.yaml`).** DEFERRED. Fixture 0002's `expectations.yaml` follows the phase-02 convention; the structured-entries conversion is phase-06 or phase-08 scope per ADR-0019.
8. **Minor 8 (BEHAVIOR_CONTRACT TCP proxy missing ADR-0028 link).** RESOLVED. ADR-G's BEHAVIOR_CONTRACT edit lands the one-liner cross-reference in the adjacent **TCP proxy** subsection in the same commit as the TLS subsection.

Three of eight resolved, two explicitly deferred with rationale, one cross-cutting-doctrine deferred, one anti-pattern avoided by construction, one documented in the SPEC body (Minor 1). No Minor rises to a phase-03 blocker.

## 13. Acceptance checklist (for the reviewer of this phase's final state)

- [ ] `internal/tls/config.go`, `datasource.go`, `params.go`, `sni.go`, `fuzz_test.go` exist and build. Unit tests pass. Errors begin with `tls: `.
- [ ] `internal/listener/manager.go` supports multi-filter-chain construction with SNI-based `GetConfigForClient` routing. Build-time errors match §4.2 / §5.2. Unit tests pass.
- [ ] `internal/cluster/cluster.go` exposes `Dial(ctx) (net.Conn, error)`; plaintext and TLS paths both unit-tested.
- [ ] `internal/filter/tcpproxy/filter.go` consumes `ctx` via `cluster.Dial(ctx)`. Pump body unchanged.
- [ ] `internal/tls/fuzz_test.go` `FuzzTLSContextParse` runs clean on CI short budget. Phase-01 `FuzzBootstrapLoad` + phase-02 `FuzzTcpProxyFilter` also clean.
- [ ] `test/differential/fixture/fixture.go` carries `DriveReference` + `DriveSubject` replacing `Drive`. All three fixture drivers updated in the same commit. Runner updated.
- [ ] `test/fixtures/0002-tls-tcp/` contains `envoy-go.yaml`, `envoy.yaml`, `expectations.yaml`, `README.md`, `pki/` (CA + 4 leaf cert+key pairs + README + gen tool), `driver/driver.go`, `driver/driver_test.go`. Differential gate green.
- [ ] `test/helpers/tls.go` `TLSRoundTrip` exists and is used by the 0002 driver.
- [ ] `BEHAVIOR_CONTRACT.md` contains a new **TLS** subsection (ADR-G). The adjacent **TCP proxy** subsection gains the one-line cross-reference to ADR-0028 (Minor 8 resolved).
- [ ] `DECISIONS.md` contains ADRs A–G (with actual sequential numbers assigned at landing). ADR-B names ADR-0025 in its `**Supersedes:**` header. ADR-F names the phase-02 `fixture.Driver` with the `(informal)` qualifier (no prior ADR). Each ADR carries Status/Date/Doctrine plus Context/Decision/Rationale/Consequences.
- [ ] `ROADMAP.md` row for phase 03 is `status: done` at commit time.
- [ ] `STATE.md` advances to phase 04 with `lifecycle-state: 1` / `next-skill: superpowers:brainstorming` at commit time.
- [ ] `go build ./...`, `go vet ./...`, `golangci-lint run`, `go test ./...` all clean (captured in PROGRESS.md per §7.5(e)).
- [ ] Commit messages follow `BOOTSTRAP_PROMPT.md` §5.3 format and reference the ADRs introduced or referenced.
- [ ] Phase-02 REVIEW carryovers triaged per §12; each Resolve item landed in code, each Defer item called out in PROGRESS or a later phase's SPEC.
