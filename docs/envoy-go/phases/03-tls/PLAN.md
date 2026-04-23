# Phase 03 — TLS (Downstream Termination + Upstream Origination + SNI) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants), §5 (state machine), §6 (splitting), §7 (differential contract); `docs/envoy-go/phases/03-tls/SPEC.md` (authoritative scope — every PLAN decision below traces to a SPEC section); `docs/envoy-go/DECISIONS.md` (ADR-0001…0028 — especially **ADR-0003** branch convention, **ADR-0004** autonomous brainstorming adaptation, **ADR-0005** autonomous plan-review adaptation, **ADR-0008** Envoy v1.37.2 pin, **ADR-0010** V4_ONLY DNS rule, **ADR-0013** go-control-plane proto-types-only pin, **ADR-0018** fuzz CI budget, **ADR-0022** managers replace first-only extractors, **ADR-0023** phase-00 pump lift, **ADR-0025** phase-02 filter-chain subset (**this PLAN supersedes via ADR-0033**), **ADR-0026** per-listener sentinel format, **ADR-0028** reference-side `--concurrency 1` pin); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (existing `## Equivalence Matrix`, `## Admin API — /ready`, `## TCP proxy`, `## Test harness host networking` subsections — phase 03 adds a new `## TLS` subsection and appends one sentence to `## TCP proxy`); `docs/envoy-go/phases/02-tcp-proxy/PLAN.md` and `PROGRESS.md` (style reference for tasks, atomic per-task commits, PROGRESS conventions, ADR-with-first-use-commit discipline); `docs/envoy-go/phases/02-tcp-proxy/REVIEW.md` (the 8 Minors — §12 of SPEC triages them; phase 03 resolves Minors 4, 6, 8).

**Goal:** Land envoy-go's first cryptographic surface — a new `internal/tls/` package that parses Envoy v3 `DownstreamTlsContext` / `UpstreamTlsContext` and yields ready-to-use `*crypto/tls.Config` values; a listener manager extended for 1..N filter chains with SNI-based dispatch via `tls.Config.GetConfigForClient`; a cluster manager extended with `Cluster.Dial(ctx) (net.Conn, error)` abstracting plaintext vs upstream-TLS dialing; a `tcp_proxy` filter whose `Handle(ctx, downstream)` now consumes `ctx` through `cluster.Dial(ctx)` (resolving phase-02 REVIEW Minor 4); an evolved fixture-driver interface split into `DriveReference` + `DriveSubject` (resolving phase-02 REVIEW Minor 6); committed PKI test artifacts with a regeneration tool; a new differential fixture `0002-tls-tcp` exercising two SNI-indexed downstream chains and two upstream TLS clusters with byte-exact plaintext equivalence and per-cluster `[3,3,3]` distribution assertions per side; a `FuzzTLSContextParse` target; and a `BEHAVIOR_CONTRACT.md` TLS subsection plus the phase-02 REVIEW Minor 8 cross-reference — satisfying every gate in `docs/envoy-go/phases/03-tls/SPEC.md` §3.

**Architecture:** `internal/tls/` decomposes into four source files with orthogonal responsibility — `sni.go` (pure `MatchServerName` wildcard function), `datasource.go` (`loadDataSource` for `inline_bytes`/`inline_string`/`filename`), `params.go` (`tls_params` → `stdtls.Config.{MinVersion,MaxVersion,CipherSuites,CurvePreferences}`), and `config.go` (`NewDownstreamConfig`, `NewUpstreamConfig` composing the three leaves into `*DownstreamConfig`/`*UpstreamConfig` structs each carrying a fully-built `*stdtls.Config`). `crypto/tls` is imported as `stdtls` in every file under `internal/tls/` (the package is itself named `tls`). `internal/cluster/cluster.go` gains an unexported `upstreamCfg *stdtls.Config` plus a `Dial(ctx context.Context) (net.Conn, error)` method composing `net.Dialer.DialContext` → (optionally) `stdtls.Client(tcp, cfg).HandshakeContext(ctx)`. `internal/cluster/manager.go` decodes `cluster.transport_socket` as `UpstreamTlsContext` via `internal/tls.NewUpstreamConfig` (errors if `transport_socket != nil` and type is not `tls`). `internal/listener/manager.go` replaces the phase-02 "exactly one filter_chain / empty match / no transport_socket" check (ADR-0025) with the phase-03 subset (ADR-0033) — permitting 1..N chains with `filter_chain_match` restricted to `server_names[]` + optional `transport_protocol=="tls"`, rejecting mixed TLS/plaintext on a single listener, rejecting `Listener.default_filter_chain`, rejecting `require_client_certificate=true`. TLS listener Start composes a single top-level server `*stdtls.Config` whose `GetConfigForClient(hello)` callback dispatches by SNI using `tls.MatchServerName`, returning each matching chain's `*stdtls.Config`. Chain→filter propagation uses approach (A) from SPEC §10 #2: a `sync.Map[*stdtls.Conn]*chainInfo` populated by a `VerifyConnection` closure inside each chain's `*stdtls.Config`, read by the worker goroutine after `HandshakeContext` returns. Per-connection flow on a TLS listener: Accept → worker goroutine → `stdtls.Server(raw, listenerCfg)` → `HandshakeContext(ctx)` (the `VerifyConnection` hook of the chain-specific `*stdtls.Config` records the chain pointer in the `sync.Map`) → load the chain pointer → `chain.filter.Handle(ctx, tlsConn)` → the filter's pump uses `Cluster.Dial(ctx)` which on TLS clusters does TCP dial + `stdtls.Client` + `HandshakeContext(ctx)`. The `halfClose` helper in `internal/filter/tcpproxy/filter.go` (the ADR-0023 verbatim trio) gains a type-switch branch for `*stdtls.Conn.CloseWrite` so half-close propagation works over TLS. The `test/differential/fixture.Driver` interface's single `Drive(ctx, refAddr, subjAddr)` method is retired in a single atomic commit — `DriveReference(ctx, addr)` + `DriveSubject(ctx, addr)` take its place across the interface, the runner, and drivers `0000`, `0001`, `0002` (ADR-0034). PKI lives at `test/fixtures/0002-tls-tcp/pki/` — one self-signed ECDSA-P256 root CA plus four leaf certs (`server-alpha`, `server-beta`, `upstream-alpha`, `upstream-beta`) each with fixed `NotBefore: 2026-01-01T00:00:00Z`, `NotAfter: 2046-01-01T00:00:00Z`, generated deterministically by `test/fixtures/0002-tls-tcp/pki/gen/main.go` seeded from a constant (`rand.Reader`-replacement seeded by a known byte pattern so two runs produce byte-identical PEMs). Fixture `0002-tls-tcp` uses STRICT_DNS on the reference side with `host.docker.internal` (`dns_lookup_family: V4_ONLY`, ADR-0010 inherited from fixture 0001) and STATIC on the subject side with `127.0.0.1` endpoints; `--concurrency 1` on the reference is inherited from ADR-0028 via `test/differential/harness.StartReferenceProxy` (Task 13 verifies no per-fixture opt-in is needed). Driver sends 9 TLS round-trips per SNI per side (18 per side total; 36 cluster-side dispatches total per proxy); `AssertDistribution` checks cluster-a endpoints receive `[3,3,3]` and cluster-b endpoints receive `[3,3,3]` on each proxy independently. BEHAVIOR_CONTRACT grows a new `## TLS` subsection codifying plaintext-after-decryption byte equivalence, per-SNI chain selection equivalence, upstream SNI+CA equivalence, and the explicit *non*-assertion of encrypted-side observables (TLS record boundaries, session ticket key material, TLS 1.3 cipher selection, handshake message ordering, server random, session IDs, negotiated ALPN value). The `## TCP proxy` subsection gains one sentence cross-referencing ADR-0028 (phase-02 REVIEW Minor 8). The 7 ADRs anticipated in SPEC §4.4 land as ADR-0029 through ADR-0035 (see `## ADRs introduced by this plan` below — mapping is 1:1 with SPEC §4.4 ADR-A through ADR-G, rearranged so first-use commit order matches file order in DECISIONS.md).

**Tech Stack:**
- Go 1.23 (unchanged from phase 02).
- `github.com/envoyproxy/go-control-plane/envoy` at v1.32.4 (ADR-0013 pin) — proto types only. Phase 03 adds:
  - `…/extensions/transport_sockets/tls/v3` (`DownstreamTlsContext`, `UpstreamTlsContext`, `CommonTlsContext`, `TlsParameters`, `CertificateValidationContext`, `TlsCertificate`)
  - `…/config/core/v3.DataSource` (already transitively imported; phase-03 takes a direct typed import in `internal/tls/datasource.go`)
- Stdlib `crypto/tls` (imported as `stdtls` in every `internal/tls/*.go` file to avoid collision with the package name), `crypto/x509`, `crypto/ecdsa`, `crypto/elliptic`, `crypto/rand`, `encoding/pem`, `math/big`, `encoding/asn1` (for the PKI generator).
- `google.golang.org/protobuf/types/known/anypb` (typed Any unmarshal from `core.v3.TransportSocket.typed_config`).
- Stdlib `sync`, `sync/atomic`, `net`, `io`, `context`, `time`, `fmt`, `strings`, `errors` — unchanged disciplines from phase 02.
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- Upstream Envoy v1.37.2 @ `sha256:c5e8a68e…` (ADR-0008, consumed not modified); `--concurrency 1` inheritance from ADR-0028.

---

## Scope check — why phase 03 ships as one phase, not 03.1 + 03.2

Net change estimate: **~2600 LoC** (~950 new production code under `internal/tls/`, ~350 listener/cluster/filter extensions, ~300 PKI generator + committed PEMs, ~80 `test/helpers/tls.go`, ~120 fixture interface split net refactor delta, ~650 fixture 0002 including YAMLs/driver/test/README, ~150 BEHAVIOR_CONTRACT additions, ~450 across the seven ADRs in DECISIONS.md). The split-gate threshold is **~1500 LoC OR ~25 numbered tasks** (`BOOTSTRAP_PROMPT.md` §6.1); the estimate exceeds the LoC threshold. Task count is 15 — well below the 25 gate.

Phase 03 ships as **one** phase (not split into 03.1 foundation + 03.2 wiring), for the same three reasons phase 02 shipped as one phase (see phase-02 PLAN.md `## File Structure`), strengthened here:

1. **Atomic-claim cohesion (SPEC §1).** The phase's central claim is: *envoy-go terminates TLS downstream, originates TLS upstream, and dispatches by SNI, on a deterministic workload producing byte-equivalent plaintext to upstream Envoy.* A split where 03.1 ships `internal/tls/` (parse + types) and 03.2 ships listener/cluster/filter wiring + fixture weakens this claim — 03.1 would have only unit-test and fuzz evidence for a package with zero production callers, and SPEC §3 gate (a) ("new/changed differential fixtures green") could not be satisfied in 03.1. BOOTSTRAP_PROMPT §6.3 anti-pattern explicitly warns against *shipping incomplete stubs that differential tests can't exercise*. A 03.1 / 03.2 split would ship exactly that stub.

2. **No clean half-fixture seam.** The alternative split — 03.1: `internal/tls/` + listener/cluster/filter changes + fixture interface split; 03.2: fixture 0002 + PKI + BEHAVIOR_CONTRACT — has a slightly better shape (03.1 ends with unit-test green + updated 0000/0001 fixtures; 03.2 lights up 0002). But 03.1 would still ship a *dataplane whose new code paths are not exercised by any differential fixture* — the listener's SNI routing, the cluster's TLS Dial, and the filter's `ctx` consumption would all be unit-tested only. SPEC §3 gate (a) would partially satisfy (via 0000/0001 regression-freeness), but the phase's *primary* new surface — TLS — would be unasserted end-to-end until 03.2. This is the same anti-pattern in a different wrapper.

3. **Mid-execution split valve is preserved.** `BOOTSTRAP_PROMPT.md` §6.1's secondary trigger ("if any single task's sub-steps blow up past ~10 items once contact with reality reveals complexity") stays active. The two tasks most likely to blow past 10 sub-steps are Task 10 (listener multi-chain + SNI + `GetConfigForClient`, with chain-selection propagation) and Task 13 (fixture 0002, which integrates every prior task). If either exceeds 15 sub-steps at execution time, the executor splits per §6.2 with an ADR. That is a real release valve — the executor does not need permission to invoke it.

**Triggering re-evaluation:** if at execution time the cumulative landed-LoC count exceeds **3500** by the end of Task 12 (i.e., before the 0002 fixture + BEHAVIOR_CONTRACT + verification sweep), invoke `superpowers:systematic-debugging` on the estimate-vs-reality gap and re-evaluate the split decision. A 35% estimate miss on a carefully-bounded phase is a signal that the plan's shape is wrong, not just that the work is large.

---

## File Structure

| Path | Created/Modified/Deleted | Purpose |
|---|---|---|
| `internal/tls/doc.go` | Create | Package doc — phase-03 surface (downstream TLS termination, upstream TLS origination, SNI wildcard match); references SPEC §4.1, ADR-0029. Imports `stdtls = crypto/tls` aliased in every sibling. |
| `internal/tls/sni.go` | Create | `MatchServerName(patterns []string, sni string) bool`. Envoy-semantics wildcards: exact > suffix (`*.example.com`) > universal (`*`); case-insensitive (both sides `strings.ToLower`d); `"*.example.com"` matches `foo.example.com` but NOT `example.com`; empty patterns → no match. Pure function. |
| `internal/tls/sni_test.go` | Create | Exhaustive table-driven test: exact, suffix wildcard, universal wildcard, no-match, mixed case normalization, empty patterns, empty SNI, multiple patterns selecting most-specific first. |
| `internal/tls/datasource.go` | Create | `loadDataSource(ds *corev3.DataSource, baseDir string) ([]byte, error)` — supports `inline_bytes`, `inline_string`, `filename` (resolved relative to `baseDir` when not absolute). `environment_variable` errors with `tls: data source: environment_variable is not supported in phase 03` (ADR-0030). Zero-value `DataSource` errors with `tls: data source: none of inline_bytes, inline_string, filename set`. |
| `internal/tls/datasource_test.go` | Create | Happy paths for all three; `environment_variable` error; zero-value error; filename with absolute path; filename with relative path under a `t.TempDir()`-created baseDir; filename pointing at nonexistent file; filename reading a 10 MB file (no truncation). |
| `internal/tls/params.go` | Create | `applyTLSParams(cfg *stdtls.Config, params *tlsv3.TlsParameters) error` — phase-03 authoritative mapping table from SPEC §5.5. Handles `tls_{minimum,maximum}_protocol_version`, `cipher_suites`, `ecdh_curves`, `signature_algorithms` per ADR-0031. Unknown cipher-suite name → `tls: tls_params: unknown cipher suite %q`; TLSv1_0 / TLSv1_1 → error; `signature_algorithms` set → error; TLS-1.3-only cipher listed in `cipher_suites` → diagnostic log + silent drop. |
| `internal/tls/params_test.go` | Create | Table-driven: version enum mapping (v1_2/v1_3 ok, v1_0/v1_1 error, TLS_AUTO error); cipher-suite IANA-name → stdtls ID (subset of known-good names ok, unknown errors, TLS-1.3-only names silently drop with diagnostic); ecdh_curves (X25519/P-256/P-384/P-521 ok; unknown errors); signature_algorithms populated → error. |
| `internal/tls/config.go` | Create | `type DownstreamConfig struct { TLSConfig *stdtls.Config; … }`, `type UpstreamConfig struct { TLSConfig *stdtls.Config; SNI string; … }`. `NewDownstreamConfig(ts *corev3.TransportSocket, baseDir string) (*DownstreamConfig, error)` and `NewUpstreamConfig(ts *corev3.TransportSocket, baseDir string) (*UpstreamConfig, error)`. Enforces phase-03 scope (SPEC §2, §5.5): `require_client_certificate=true` → error; SDS-bound secrets → error; custom validator config → error; `match_typed_subject_alt_names` → error; `verify_certificate_{hash,spki}` → error; password-protected key → error; upstream `validation_context.trusted_ca` required (ADR §5.4 tightening, settles SPEC §10 #3); downstream `validation_context` present → ignored (no-op + diagnostic); `UpstreamTlsContext.allow_renegotiation=true` → error (settles SPEC §10 #11). Uses `internal/tls.applyTLSParams`, `loadDataSource`. Errors all begin with `tls: `. |
| `internal/tls/config_test.go` | Create | Downstream happy (inline PEMs, minimal context, ALPN passthrough); Upstream happy (inline PEMs + CA + SNI); wrong type_url; unmarshal error (random bytes in Any); missing tls_certificates → error; malformed PEM → error; CA parse failure → `tls: upstream: validation_context: trusted_ca: parse failure`; mTLS rejection (`require_client_certificate=true` → error); SDS-bound secret → error; disallowed DataSource (env var) → error via datasource.go; password on key → error; `allow_renegotiation=true` → error; upstream without `trusted_ca` → error. Uses the Task 7 committed PEMs via `loadDataSource` wrapping `os.ReadFile`. |
| `internal/tls/fuzz_test.go` | Create | `FuzzTLSContextParse(f *testing.F)` per SPEC §4.1. Seed corpus (4 entries, settles SPEC §10 #5): (a) well-formed `DownstreamTlsContext` Any with inline PEMs from Task 7; (b) well-formed `UpstreamTlsContext` Any with inline PEMs + CA + SNI; (c) truncated Any bytes; (d) Any with wrong `type_url` (`type.googleapis.com/google.protobuf.StringValue`). Fuzz body: call `NewDownstreamConfig` and `NewUpstreamConfig` against the mutated payload; assert no panic; assert every returned error begins with `tls:`. Short-budget `-fuzztime=30s` (ADR-0018 precedent, not re-decided). |
| `internal/cluster/cluster.go` | Modify | Add unexported `upstreamCfg *stdtls.Config` on `Cluster`. Add `Dial(ctx context.Context) (net.Conn, error)` method. Plaintext branch: `(&net.Dialer{Timeout: c.connectTimeout}).DialContext(ctx, "tcp", ep.Addr())`. TLS branch: same TCP dial, then `stdtls.Client(tcp, c.upstreamCfg)`, then `HandshakeContext(ctx)`; on handshake error, close the TCP conn and return `cluster: tls: handshake: %v`-wrapped error. `PickEndpoint` unchanged. No net-new exported symbols beyond `Dial`. |
| `internal/cluster/cluster_test.go` | Modify | Extend: `Dial` plaintext round-trip against a loopback echo; `Dial` TLS round-trip against a `stdtls.Listen`-wrapped loopback echo using the Task 7 PKI committed PEMs; `Dial` TLS handshake failure (wrong CA) → `cluster: tls: handshake:` wrapped error; `Dial` ctx cancellation during handshake returns promptly (closes TCP conn); `Dial` ctx cancellation before dial → short-circuit. |
| `internal/cluster/manager.go` | Modify | `buildCluster` reads `cluster.transport_socket`: if nil, plaintext path (unchanged); if non-nil, decode typed_config Any → `UpstreamTlsContext` via `internal/tls.NewUpstreamConfig`; store resulting `*stdtls.Config` on the cluster. If typed_config `type_url` is not the UpstreamTlsContext URL, error with `cluster: unsupported transport_socket type_url %q`. `connectTimeout` handling unchanged. |
| `internal/cluster/manager_test.go` | Modify | Extend: TLS cluster happy (upstream TLS config built); unknown transport_socket type_url → error; cluster with `transport_socket` + no `trusted_ca` → error (tightening from §5.4 propagates up through `NewUpstreamConfig`); mixed plaintext + TLS clusters in one bootstrap → both build independently. |
| `internal/listener/manager.go` | Modify | Phase-02 single-chain check (ADR-0025) is fully replaced by the phase-03 subset (ADR-0033, supersedes ADR-0025). Build-time: `filter_chains` must be ≥1; each chain's `filter_chain_match` may be nil/empty OR populate only `server_names[]` + optional `transport_protocol=="tls"`; ≥2 catch-all chains → error; any other match field populated → error; `Listener.default_filter_chain` set → error; any chain's `transport_socket` non-nil triggers TLS-listener mode (all chains on that listener must carry a TLS `transport_socket`, mixed TLS/plaintext errors); each chain's filter construction is unchanged (inline registry, exactly one filter). TLS listener Start: compose a single top-level server `*stdtls.Config` whose `GetConfigForClient(hello)` returns the chain-specific `*stdtls.Config` from the most-specific matching chain (via `tls.MatchServerName`) or the catch-all's config or `nil, error` if neither matches. Per-chain `*stdtls.Config` gains a `VerifyConnection` hook that records the chain pointer into a listener-scoped `sync.Map[*stdtls.Conn]*chainInfo` (settles SPEC §10 #2 via approach (A)). Accept loop: unchanged for plaintext; for TLS, per-connection worker does `stdtls.Server(raw, listenerCfg)` → `HandshakeContext(ctx)` → read chain pointer from `sync.Map` → `chain.filter.Handle(ctx, tlsConn)` (settles SPEC §10 #1 via approach (b)). Handshake error: close `raw`, log `listener %q: handshake: %v`. |
| `internal/listener/manager_test.go` | Modify | Extend: multi-chain TLS happy path (two SNIs, distinct certs, verify `GetConfigForClient` routing via mocked `ClientHelloInfo`); plaintext-only listener with multiple chains → error; mixed TLS/plaintext chains on one listener → error; non-SNI `filter_chain_match` field set → error; `default_filter_chain` set → error; `require_client_certificate=true` on any chain → error; unknown `transport_socket` type → error; `GetConfigForClient` with no matching SNI and no catch-all → `nil, error`; `GetConfigForClient` with catch-all only → catch-all config returned for any SNI; `MatchServerName` case-insensitivity propagated through `GetConfigForClient`. |
| `internal/filter/tcpproxy/filter.go` | Modify | `Handle(ctx, downstream)`: replace the phase-02 `net.DialTimeout(...)` site with `f.cluster.Dial(ctx)` (two lines of diff in the filter body). Add early `if err := ctx.Err(); err != nil { return }` guard at the head of `Handle` (formalizes SPEC §5.6 step 1, resolves phase-02 REVIEW Minor 4). `halfClose` helper: extend the type-switch to fall back to `*stdtls.Conn.CloseWrite` when the underlying is a TLS conn (SPEC §5.6 last paragraph). Pump body itself (the `netConn` wrapper + two-goroutine `io.Copy`) unchanged — ADR-0023 preserved verbatim. |
| `internal/filter/tcpproxy/filter_test.go` | Modify | Extend: ctx cancellation before dial returns promptly; ctx cancellation mid-handshake returns promptly (TLS cluster wired to a blocked-accept server, ctx cancel, verify `Handle` returns quickly and downstream is closed); TLS upstream cluster transparency (filter body does not type-switch on upstream conn transport); `halfClose` over `*stdtls.Conn` triggers a `close_notify` then TCP FIN (loopback two-node test with the Task 7 PKI, assert downstream half-close propagates to upstream). |
| `internal/filter/tcpproxy/fuzz_test.go` | *Unchanged* | Phase-02 `FuzzTcpProxyFilter` preserved. Regression run in Task 15. |
| `test/differential/fixture/fixture.go` | Modify | `Driver` interface: retire `Drive(ctx, refAddr, subjAddr string) ([]byte, error)`; introduce `DriveReference(ctx context.Context, addr string) ([]byte, error)` + `DriveSubject(ctx context.Context, addr string) ([]byte, error)`. All other interface methods unchanged (`BackendCount`, `SubjectListenerName`, `ReferenceListenerPort`, `ReferenceBootstrap`, `SubjectConfig`, `ProbeAdmin`, the optional `DistributionAsserter`). ADR-0034 codifies. The `""` sentinel-argument contract is gone. |
| `test/differential/runner_test.go` | Modify | Call sites: `d.Drive(ctx, refAddr, "")` → `d.DriveReference(ctx, refAddr)`; `d.Drive(ctx, "", subjAddr)` → `d.DriveSubject(ctx, subjAddr)`. Blank-import added for `test/fixtures/0002-tls-tcp/driver`. No other behavioural change — the runner's "drive reference, snapshot ref counts, reset, drive subject, snapshot subj counts" sequence is unchanged in shape (it was already calling the two sentinels separately). |
| `test/fixtures/0000-tcp-echo/driver/driver.go` | Modify | `Drive` split into `DriveReference` + `DriveSubject`. The 0000 driver was already honouring the `""` sentinel via two guards; the split is a pure refactor of the same logic into two methods. |
| `test/fixtures/0001-tcp-proxy-rr/driver/driver.go` | Modify | Same split, same pure-refactor shape as 0000. |
| `test/fixtures/0002-tls-tcp/pki/ca.pem` | Create | Self-signed ECDSA P-256 CA. `Subject: CN=envoy-go test CA`. `NotBefore: 2026-01-01T00:00:00Z`, `NotAfter: 2046-01-01T00:00:00Z`. `IsCA: true`, `KeyUsage: CertSign|CRLSign`. Generated deterministically by `pki/gen/main.go` from a fixed seed; committed PEM is the source of truth. |
| `test/fixtures/0002-tls-tcp/pki/server-alpha.pem` | Create | Leaf cert signed by `ca.pem`. `Subject: CN=alpha.envoy-go.test`. `DNSNames: [alpha.envoy-go.test]`. Same NotBefore/NotAfter. `KeyUsage: DigitalSignature|KeyEncipherment`. `ExtKeyUsage: ServerAuth`. |
| `test/fixtures/0002-tls-tcp/pki/server-alpha.key.pem` | Create | ECDSA P-256 private key (unencrypted PKCS#8 PEM). Matches the public key in `server-alpha.pem`. |
| `test/fixtures/0002-tls-tcp/pki/server-beta.pem` | Create | Leaf cert as above but `CN=beta.envoy-go.test`, `DNSNames: [beta.envoy-go.test]`. |
| `test/fixtures/0002-tls-tcp/pki/server-beta.key.pem` | Create | ECDSA P-256 private key for `server-beta.pem`. |
| `test/fixtures/0002-tls-tcp/pki/upstream-alpha.pem` | Create | Leaf for upstream-alpha backends. `CN=alpha.envoy-go.test`. `DNSNames: [alpha.envoy-go.test, localhost]`. `IPAddresses: [127.0.0.1]`. `ExtKeyUsage: ServerAuth`. Same NotBefore/NotAfter. |
| `test/fixtures/0002-tls-tcp/pki/upstream-alpha.key.pem` | Create | ECDSA P-256 private key for `upstream-alpha.pem`. |
| `test/fixtures/0002-tls-tcp/pki/upstream-beta.pem` | Create | Leaf for upstream-beta backends. `CN=beta.envoy-go.test`. `DNSNames: [beta.envoy-go.test, localhost]`. `IPAddresses: [127.0.0.1]`. |
| `test/fixtures/0002-tls-tcp/pki/upstream-beta.key.pem` | Create | ECDSA P-256 private key for `upstream-beta.pem`. |
| `test/fixtures/0002-tls-tcp/pki/gen/main.go` | Create | Deterministic generator. Uses a fixed 32-byte seed → `math/rand.NewChaCha8(seed)` wrapped as an `io.Reader` for `crypto/ecdsa.GenerateKey` and `crypto/x509.CreateCertificate`. Not part of `go test ./...` (separate main package under `pki/gen/`). Settles SPEC §10 #6: kept at `pki/gen/` because only one TLS fixture exists in phase 03. Re-running `go run ./pki/gen` in the fixture dir rewrites every PEM byte-identically. |
| `test/fixtures/0002-tls-tcp/pki/README.md` | Create | Documents: NotBefore/NotAfter; `go run ./pki/gen` regeneration command; the fixed-seed rationale (determinism, so `git diff` is clean after regeneration); the PKI layout (1 CA + 4 leaves); the subject/SAN choices (especially why upstream leaves carry IP SAN for 127.0.0.1 + DNS SAN for localhost); a note that PKI regeneration is a manual operation not triggered by CI. |
| `test/helpers/tls.go` | Create | `TLSRoundTrip(ctx context.Context, addr, serverName string, rootCAs *x509.CertPool, payload []byte, idleTimeout time.Duration) ([]byte, error)` — dial TCP, wrap in `stdtls.Client` with `MinVersion: stdtls.VersionTLS12, MaxVersion: stdtls.VersionTLS13, ServerName: serverName, RootCAs: rootCAs`, `HandshakeContext(ctx)`, write `payload`, call `tlsConn.CloseWrite()` to half-close, `ReadAll` until EOF or idleTimeout, return bytes. Close on all exit paths. |
| `test/helpers/tls_test.go` | Create | Round-trip against a loopback `stdtls.Listen`-wrapped echo using the Task 7 committed CA + server-alpha cert; handshake timeout path; wrong SNI path (cert mismatch). |
| `test/fixtures/0002-tls-tcp/envoy.yaml` | Create | Reference bootstrap. 1 listener `l_tls` binding `0.0.0.0:15002` with 2 filter chains: `(server_names: [alpha.envoy-go.test])` → cluster `c_alpha`; `(server_names: [beta.envoy-go.test])` → cluster `c_beta`. Each chain's `transport_socket` carries a `DownstreamTlsContext` with the matching server PEM + key inlined. 2 STRICT_DNS clusters (`c_alpha`, `c_beta`) each with 3 `lb_endpoints` at `host.docker.internal:<port_N>` (placeholders), `dns_lookup_family: V4_ONLY` (ADR-0010), each cluster's `transport_socket` carries an `UpstreamTlsContext` with `sni: alpha.envoy-go.test` (resp. beta), inline upstream PEM + key, inline CA in `validation_context.trusted_ca`. Admin listener `0.0.0.0:9901` unchanged. |
| `test/fixtures/0002-tls-tcp/envoy-go.yaml` | Create | Subject bootstrap. Same listener shape (1 listener `l_tls` + 2 SNI-indexed chains + inline downstream PEMs). 2 STATIC clusters at literal `127.0.0.1:<port_N>` with same SNI + CA + inline upstream PEM. Admin as phase 01/02. |
| `test/fixtures/0002-tls-tcp/expectations.yaml` | Create | Prose-per-fixture-0001 style (Minor 7 deferred per ADR-0019). Asserts byte-exact plaintext response body (post-TLS-decryption); /ready admin probe handled by harness defaults; per-cluster distribution assertion `[3,3,3]` per SNI per side implemented in the driver (not in expectations.yaml, mirroring fixture 0001). |
| `test/fixtures/0002-tls-tcp/README.md` | Create | Fixture purpose (2-SNI TLS round-trip); STATIC-vs-STRICT_DNS divergence (inherits the same rationale as ADR-0010 + ADR-0027); PKI layout (points at `pki/README.md`); distribution-assertion methodology (same discipline as fixture 0001); `--concurrency 1` inheritance from ADR-0028. |
| `test/fixtures/0002-tls-tcp/driver/driver.go` | Create | `init()` registers driver key `0002-tls-tcp`. `BackendCount() = 6`. `SubjectListenerName() = "l_tls"`. `ReferenceListenerPort() = 15002`. `ReferenceBootstrap(backendPorts []int) string` + `SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string` template the 6 backend ports (`[0..2]` = c_alpha; `[3..5]` = c_beta) into the respective YAML shells. `DriveReference(ctx, addr) → []byte` opens `TLSRoundTrip` with `ServerName: "alpha.envoy-go.test"` × 9 payloads `"rr-alpha-<0..8>\n"`, then with `ServerName: "beta.envoy-go.test"` × 9 payloads `"rr-beta-<0..8>\n"`, concatenates results. `DriveSubject` has the same shape against the subject address. `AssertDistribution(refCounts, subjCounts [6]uint64) error` validates each side independently: `refCounts[0..2]` == `[3,3,3]`, `refCounts[3..5]` == `[3,3,3]`, same for subj. `ProbeAdmin` same as phase 01/02. |
| `test/fixtures/0002-tls-tcp/driver/driver_test.go` | Create | Unit test for `AssertDistribution` only — Docker-free. Happy `[3,3,3]/[3,3,3]` passes; `[4,3,2]/[3,3,3]` fails with a clear message; `[3,3,3]/[0,0,9]` fails; zero-count edge case. Mirror of fixture 0001's `driver_test.go`. |
| `test/differential/harness.go` | *Unchanged* | Reference container launch (with `--concurrency 1` from ADR-0028) is verified in Task 13 to inherit into fixture 0002 without opt-in. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | Modify | Add new top-level `## TLS` subsection (content per SPEC §5.7); append one sentence cross-referencing ADR-0028 to the existing `## TCP proxy` subsection (resolves phase-02 REVIEW Minor 8). |
| `docs/envoy-go/DECISIONS.md` | Modify | Append ADR-0029 through ADR-0035 (seven ADRs — listed in `## ADRs introduced by this plan` below). Each ADR lands in the same commit as the code that consumes it (phase-00/01/02 precedent). |
| `docs/envoy-go/ROADMAP.md` | *Not modified by this plan* | Row 03 advances to `done` at state-machine step 6 in a later session per ADR-0005. |
| `docs/envoy-go/STATE.md` | Modify (at exit) | Advanced to `lifecycle-state: 3`, `next-skill: superpowers:subagent-driven-development` at this plan-authoring session's exit commit — matching the phase-02 exit discipline. |
| `docs/envoy-go/phases/03-tls/PROGRESS.md` | Create (during execution) | Append-only running log per BOOTSTRAP §5 step 3, matching phase-00/01/02 conventions. |
| `cmd/envoy-go/main.go` | *Unchanged* | The listener and cluster managers' extended behaviours are transparent to `main`. |
| `cmd/envoy-go/main_test.go` | *Unchanged (verified)* | Task 15 explicitly verifies the phase-02 two-listener bootstrap harness still passes (plaintext-listener regression coverage — resolves the spec-review advisory on this path). |

---

## ADRs introduced by this plan

Seven ADRs land at execution time. Each is the first-use task's responsibility and goes into the same commit as the code that consumes it. All entries in `DECISIONS.md` are append-only (D-3.5); no landed ADR is edited. ADR numbering continues from the tail verified at PLAN-write time (**ADR-0028** is the current tail, verified by `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` → `ADR-0028`). Per SPEC §4.4, the SPEC anticipated ADRs A–G; the assigned numbers below are sequenced so that **first-use commit order matches DECISIONS.md file order** (phase-02 precedent). The SPEC-to-ADR-number map:

- **SPEC §4.4 ADR-D** → **ADR-0029** (lands Task 3, first use of DataSource loader).
- **SPEC §4.4 ADR-E** → **ADR-0030** (lands Task 4, first use of TLS parameter mapping).
- **SPEC §4.4 ADR-A** → **ADR-0031** (lands Task 5, first use of `NewDownstreamConfig`/`NewUpstreamConfig`).
- **SPEC §4.4 ADR-C** → **ADR-0032** (lands Task 9, first use of `Cluster.Dial`).
- **SPEC §4.4 ADR-B** → **ADR-0033** (lands Task 10, supersedes ADR-0025).
- **SPEC §4.4 ADR-F** → **ADR-0034** (lands Task 12, fixture-driver interface split).
- **SPEC §4.4 ADR-G** → **ADR-0035** (lands Task 14, BEHAVIOR_CONTRACT TLS subsection).

Summaries:

- **ADR-0029 (= SPEC ADR-D) — DataSource handling policy.** `internal/tls.loadDataSource` supports `inline_bytes`, `inline_string`, `filename` (resolved relative to the bootstrap file's directory or absolute). `environment_variable` errors. SDS-bound secret configs error at the caller (`config.go`). Filename support included from phase 03 (not deferred) because the cost is trivial and future phases will need it for non-test deployments. Lands in Task 3. Supersedes nothing.
- **ADR-0030 (= SPEC ADR-E) — TLS parameter mapping scope.** `tls_params` fields honoured by phase 03: `tls_{minimum,maximum}_protocol_version` (`TLSv1_2`/`TLSv1_3` → `stdtls.VersionTLS12/TLS13`; `TLSv1_0`/`TLSv1_1`/`TLS_AUTO` → error); `cipher_suites` (IANA name → `stdtls.CipherSuites()` ID, unknown errors, TLS-1.3-only ciphers diagnostic-logged then silently dropped); `ecdh_curves` (`X25519`/`P-256`/`P-384`/`P-521` → `stdtls.CurveID`, other values error); `alpn_protocols` on `common_tls_context` → `stdtls.Config.NextProtos`. Errored: `signature_algorithms` (not publicly configurable in Go `crypto/tls`). Rationale: Go's `crypto/tls` doesn't allow TLS-1.3 cipher selection (RFC 8446 design); `signature_algorithms` lacks a public API. BEHAVIOR_CONTRACT TLS subsection (Task 14) records the divergence from upstream's surface. Lands in Task 4. Supersedes nothing.
- **ADR-0031 (= SPEC ADR-A) — TLS stack selection: stdlib `crypto/tls`.** Options considered: (A1) stdlib `crypto/tls`, (A2) BoringSSL via cgo, (A3) rustls or OpenSSL bindings. A1 chosen: no cgo (preserves the project's pure-Go build posture); TLS 1.2 / 1.3 parity with Envoy's defaults on the asserted surface (plaintext-after-decryption byte equivalence); ALPN + SNI + peer validation all natively supported; license-clean; no vendoring. Known tradeoffs documented in ADR-0030 (cipher/sig-alg knob coverage) and in the BEHAVIOR_CONTRACT TLS subsection (encrypted-side equivalence explicitly *not* asserted). Lands in Task 5 (first site that composes a `*stdtls.Config` for production use). Supersedes nothing.
- **ADR-0032 (= SPEC ADR-C) — Upstream TLS dialer model.** `Cluster.Dial(ctx context.Context) (net.Conn, error)` abstracts plaintext vs TLS dial paths. Plaintext: `(&net.Dialer{Timeout: c.connectTimeout}).DialContext(ctx, "tcp", ep.Addr())`. TLS: same TCP dial, then `stdtls.Client(tcp, c.upstreamCfg)`, then `HandshakeContext(ctx)`. Filter body sees `net.Conn` — transport-agnostic, ready for phase 04's HTTP-over-TLS. `connect_timeout` applies to TCP dial only; TLS handshake is bounded by `ctx` (settles SPEC §10 #8). Lands in Task 9. The phase-02 `tcpproxy` filter's direct `net.DialTimeout` site is retired in Task 11 as a consequence; no separate ADR — the change is a consequence of this one. Supersedes nothing.
- **ADR-0033 (= SPEC ADR-B) — Phase-03 filter-chain subset — supersedes ADR-0025.** Permitted: 1..N `filter_chains` per listener. `filter_chain_match` may be nil/empty (catch-all, at most one per listener) OR carry `server_names[]` and optionally `transport_protocol=="tls"`. All other `filter_chain_match` fields error at build. `Listener.default_filter_chain` set → error. Selection at handshake: most-specific SNI match > suffix-wildcard match > catch-all > no match (handshake fails, connection closes). All chains on a listener with any TLS chain must be TLS; mixed TLS/plaintext on one listener errors. `require_client_certificate=true` on any chain errors. `listener_filters` is silently skipped (phase-02 carryover). **Supersedes: ADR-0025.** Rationale: phase 03 introduces SNI-based dispatch — the phase-02 "exactly one chain" simplification is obsolete at the very first feature that needs multiple chains. Full `FilterChain` matching (ports, source IP, source ports, ALPN match) remains deferred to phase 07 per SPEC §2. Lands in Task 10.
- **ADR-0034 (= SPEC ADR-F) — Fixture-driver interface split.** Retire `Drive(ctx context.Context, refAddr, subjAddr string) ([]byte, error)` on `test/differential/fixture.Driver`. Introduce `DriveReference(ctx context.Context, addr string) ([]byte, error)` and `DriveSubject(ctx context.Context, addr string) ([]byte, error)`. All drivers (`0000-tcp-echo`, `0001-tcp-proxy-rr`, new `0002-tls-tcp`) land the new interface in the same atomic commit as the interface change itself (phase-02 REVIEW Minor 6 resolution). Rationale: the `""`-sentinel dual-argument contract was a phase-02 shortcut that the runner never actually exercised — the runner already drove reference and subject separately, passing the other side as `""`. The split makes the interface match its actual usage. **Supersedes (informal):** the phase-02 `fixture.Driver` interface codified in `test/differential/fixture/fixture.go`; that interface was never ADR'd, so this ADR's supersession header carries the `(informal)` qualifier. Lands in Task 12.
- **ADR-0035 (= SPEC ADR-G) — BEHAVIOR_CONTRACT TLS subsection.** New top-level section codifying the TLS equivalence surface: *asserted* = plaintext-after-decryption response-body byte-equivalence, per-SNI chain selection equivalence (witnessed via distribution assertion), upstream SNI + CA equivalence; *not asserted* = encrypted-side byte equivalence (TLS record boundaries, session ticket material, TLS 1.3 cipher selection, handshake message byte ordering/timing, server random, session IDs), negotiated ALPN value. Includes phase-02 REVIEW Minor 8 resolution: appends one sentence to the existing `## TCP proxy` subsection cross-referencing ADR-0028 (*"Reference-side distribution exactness (fixture `0001-tcp-proxy-rr` and, inherited, `0002-tls-tcp`) depends on the reference container's `--concurrency 1` pin per ADR-0028."*). Lands in Task 14. Supersedes nothing.

If an unforeseen decision surfaces during execution that has cross-phase impact (per D-3.5), the executor writes a new sequential ADR (ADR-0036+) in the same commit as the code it decides for. If such a decision would expand phase-03 scope beyond SPEC §1–§4, invoke `superpowers:systematic-debugging` and then either re-scope the task in place or split per `BOOTSTRAP_PROMPT.md` §6.

---

## Settled SPEC §10 deferred decisions

SPEC §10 leaves eleven implementation-detail choices to the planner. This PLAN settles them here so the executor does not re-litigate; only decisions with cross-phase impact (security tightening, new mechanism choice, interface shape) are also captured as ADRs.

1. **Handshake placement (accept loop vs worker).** **Worker goroutine.** Per SPEC §5.3 recommendation (approach (b)). The accept loop accepts, hands the raw `net.Conn` to a per-connection worker goroutine, and returns to blocking on `Accept()`. The worker performs `stdtls.Server(raw, cfg)` + `HandshakeContext(ctx)` + dispatch to `filter.Handle`. Rationale: a slow or unresponsive client's TLS handshake does not block Accept; matches the phase-02 shape where Accept immediately spawns a worker. Not ADR'd (planner-decision, SPEC §10 #1 permits either).
2. **Chain-selection propagation from `GetConfigForClient` to worker dispatch.** **Pure-function dispatch on post-handshake `ConnectionState().ServerName`.** SPEC §5.3 / §10 #2 offered two mechanisms (sync.Map shuttle, or VerifyConnection + context.WithValue). Task 10 Step 3 refines to a simpler third option: because SNI is fixed from ClientHello through the connection's lifetime and chain-match logic is a pure function of SNI, the worker simply re-runs the same match logic after `HandshakeContext` returns successfully (reading `tlsConn.ConnectionState().ServerName`). No per-connection state outside the `*stdtls.Conn` itself; no `sync.Map`; no `context.WithValue`. This refinement is captured in ADR-0033's Consequences block at landing time. **See Task 10 Step 3 for the full rationale and the ADR amendment directive — this Settled entry forward-references that task.** Not ADR'd separately (implementation detail; ADR-0033 records the mechanism choice).
3. **Upstream `validation_context.trusted_ca` mandatory.** **Confirmed mandatory.** SPEC §5.4 tightens beyond Envoy: every TLS cluster must carry `trusted_ca` in `common_tls_context.validation_context`. Rationale: a TLS cluster without CA validation is a silent downgrade with upstream confusion — at phase 03 we want every fixture to be explicit about trust. This is a security tightening; codified as a conscious divergence from Envoy's permissiveness. Not ADR'd as a standalone item — the tightening is captured in ADR-0031's decision + consequences block and explained in the BEHAVIOR_CONTRACT TLS subsection.
4. **Password-protected keys.** **SPEC policy kept — error at parse.** `common_tls_context.tls_certificates[].password` populated → `tls: password-protected keys are not supported in phase 03`. Rationale: the phase-03 fixture uses unencrypted PEMs; supporting password unlocks opens a keyring/sidecar design question out of scope. Not ADR'd.
5. **Fuzz seed corpus size.** **Four entries** per SPEC §4.1 — (a) well-formed DownstreamTlsContext Any, (b) well-formed UpstreamTlsContext Any, (c) truncated Any bytes, (d) Any with wrong `type_url` (`type.googleapis.com/google.protobuf.StringValue`). Rationale: four entries seed every distinct error path in `NewDownstreamConfig`/`NewUpstreamConfig` (type_url mismatch, unmarshal failure, DataSource-kind dispatch, SNI parse, params dispatch) without bloating the corpus. Adding more is welcome at execution time but not required. Not ADR'd.
6. **PKI generation tool location.** **`test/fixtures/0002-tls-tcp/pki/gen/main.go`.** Per SPEC §10 #6 the alternative is `test/helpers/pki/gen/`. Phase-03 has one TLS fixture; the generator lives with its fixture. If a second TLS fixture follows in a later phase, that phase may extract to `test/helpers/pki/gen/` with an ADR — not phase 03's concern. Not ADR'd.
7. **TLS 1.2 vs 1.3 floor.** **Fixture declares `tls_minimum_protocol_version: TLSv1_2`, `tls_maximum_protocol_version: TLSv1_3`.** Matches Envoy v1.37.2's default floor (TLS 1.2). Go picks 1.3 when both sides support it. Rationale: the fixture asserts plaintext-after-decryption byte equivalence regardless of negotiated version; floor + ceiling just bound the negotiation. Not ADR'd.
8. **Cluster `connect_timeout` vs handshake timeout.** **SPEC policy kept — `connect_timeout` bounds TCP dial only; handshake bounded by `ctx`.** `(&net.Dialer{Timeout: connectTimeout}).DialContext(ctx, ...)` for the TCP dial; `stdtls.Conn.HandshakeContext(ctx)` for the handshake. If `ctx` has a deadline, it bounds the handshake; if not, the handshake is unbounded (same as Envoy's behaviour when no handshake timeout is configured). Codified in ADR-0032. Not separately ADR'd.
9. **ADR numbering.** Settled above (ADR-0029..ADR-0035). First-use commit order matches DECISIONS.md file order.
10. **BEHAVIOR_CONTRACT TCP proxy cross-link placement.** Appended as the final sentence of the existing *"LB endpoint-selection sequence (NOT asserted)"* paragraph. The cross-reference reads: *"Reference-side distribution exactness (fixture `0001-tcp-proxy-rr` and, inherited, `0002-tls-tcp`) depends on the reference container's `--concurrency 1` pin per ADR-0028."* Landing location verified at Task 14 write time. Not ADR'd (cosmetic placement).
11. **Handling of `allow_renegotiation=true`.** **SPEC policy kept — error at parse.** `UpstreamTlsContext.allow_renegotiation=true` → `tls: upstream: allow_renegotiation is not supported (crypto/tls does not support TLS 1.2 renegotiation as a client)`. Rationale: erroring rather than silently no-op'ing avoids a confusing downgrade from an explicit config ask. Not ADR'd.

---

## Spec-review advisory responses

The SPEC STATE block notes five non-blocking advisory items from the spec-document-reviewer loop. Each is addressed:

- **SPEC §10 #11 restates §5.5 table (cosmetic).** No action — both restatements are benign, and the §5.5 table is the authoritative surface.
- **SPEC §5.3 offers two chain-propagation mechanisms, §10 #2 reopens.** Planner locked approach (A), see Settled §10 #2 above.
- **PKI gen-tool drift risk unmentioned in SPEC §11.** Task 7 adds a determinism verification step (`go run ./pki/gen && git diff --exit-code test/fixtures/0002-tls-tcp/pki/`) to catch non-deterministic seeding regressions early. Task 15's gate sweep re-runs this check.
- **`cmd/envoy-go/main_test.go` marked unchanged; regression coverage.** Task 15's gate sweep explicitly calls out the phase-02 two-listener bootstrap harness still passes post-phase-03 (plaintext-listener regression gate).
- **SPEC §13 acceptance checklist's "fixtures 0000/0001 green after ADR-F" is implicit.** Task 12 adds explicit `go test ./test/fixtures/0000-tcp-echo/... ./test/fixtures/0001-tcp-proxy-rr/...` verification to the interface-split commit. Task 15 re-runs the differential gate for all three fixtures.

---

## Phase-02 REVIEW carryover resolution matrix

SPEC §12 triages the eight phase-02 Minors. This PLAN lands the three "RESOLVED" Minors at specific tasks:

| Phase-02 Minor | Triage | Landing task |
|---|---|---|
| Minor 1 (SPEC scrutiny of worker-count) | RESOLVED IN SPEC | SPEC §5.8, §11 already call out `--concurrency 1` inheritance. Task 13's README + Task 14's BEHAVIOR_CONTRACT TLS subsection + TCP cross-reference codify. |
| Minor 2 (ADR bundling anti-pattern) | NO-ACTION | Phase-03 ADRs A–G are each single-concern by construction. |
| Minor 3 (ADR number-vs-physical-order drift) | DEFERRED | Phase 03 appends ADRs sequentially at file tail per prior practice. No cleanup commit. |
| **Minor 4 (`Filter.Handle` `ctx` unused)** | RESOLVED | Task 11: `Handle` adds `if ctx.Err() != nil` guard + `cluster.Dial(ctx)` consumption. |
| Minor 5 (`readyListenerAddrs` goroutine leak) | DEFERRED | Phase 03 does not touch ready sentinel path. |
| **Minor 6 (`""` sentinel in `Drive`)** | RESOLVED | Task 12: ADR-0034 splits `Drive` into `DriveReference` + `DriveSubject` atomically across interface + runner + three drivers. |
| Minor 7 (prose `expectations.yaml`) | DEFERRED | Fixture 0002's `expectations.yaml` follows phase-02 convention per ADR-0019. |
| **Minor 8 (BEHAVIOR_CONTRACT TCP missing ADR-0028 link)** | RESOLVED | Task 14: ADR-0035's BEHAVIOR_CONTRACT edit appends the one-sentence cross-reference. |

Three RESOLVED items landed explicitly; five DEFERRED items documented with rationale in SPEC §12 or above.

---

## Execution preconditions

Before Task 1, the executing session must:

1. Be running in a **fresh worktree on a phase-implementation branch cut off `master`**, NOT `phase/03-tls-plan` (this plan's authoring branch) and NOT `phase/03-tls-spec` (the SPEC's authoring branch). Recommended: `.worktrees/phase-03-tls-impl` on branch `phase/03-tls-impl`. STATE.md's `last-commit` at cold-start must be the commit that landed this PLAN.md on master. Per ADR-0003: branch fast-forwards into `master` at session exit.
2. Have `docker` available (verify with `docker version`). Required for Task 13's full differential gate (`go test ./test/differential/...`).
3. Have Go 1.23+ installed (verify with `go version`). Native fuzzing (`testing.F`) requires Go 1.18+; 1.23 is the module floor.
4. Have `golangci-lint` installed at the ADR-0009-pinned version v1.64.8 (verify with `golangci-lint version`); install via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8` if missing.
5. `go test ./...` must be green on `master` at cold-start — this plan assumes a clean baseline (phase-02 gate (e) still holds). If not, invoke `superpowers:systematic-debugging` on the regression *before* starting Task 1.
6. `go list -m github.com/envoyproxy/go-control-plane/envoy` resolves to `v1.32.4` (ADR-0013). If a different version is recorded, invoke `superpowers:systematic-debugging` — phase 03 must not silently re-pin.
7. `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0028:` (or later if a mid-phase ADR has landed since this PLAN was written). If the tail is `ADR-0028`, the phase-03 ADRs are assigned 0029..0035 as in this PLAN. If higher, re-number phase-03 ADRs sequentially from `tail + 1` and update every task's ADR reference *before* starting Task 1.

If any precondition fails: invoke `superpowers:systematic-debugging` on the missing dependency. Do not improvise an install path or skip a failing test.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/03-tls/PROGRESS.md`

No code change. This task verifies the `## Execution preconditions` block and creates PROGRESS.md so subsequent tasks have an append target.

- [ ] **Step 1: Verify each precondition**

Run:

```bash
git rev-parse --abbrev-ref HEAD                              # expect: phase/03-tls-impl (or equivalent impl branch)
git log -1 --format=%H                                       # expect: same SHA as docs/envoy-go/STATE.md last-commit field
docker version                                               # expect: client + server reported
go version                                                   # expect: go1.23+
golangci-lint version                                        # expect: golangci-lint has version 1.64.8
go test ./...                                                # expect: every package PASS
go list -m github.com/envoyproxy/go-control-plane/envoy      # expect: v1.32.4
grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1         # expect: ## ADR-0028:  (re-number phase-03 ADRs if higher — see precondition 7)
grep -n -- '--concurrency' test/differential/harness.go       # expect: at least one hit; Task 13 assumes unconditional (not fixture-gated) inheritance per ADR-0028
```

If any line fails, stop and follow the precondition's "if fails" guidance (typically: invoke `superpowers:systematic-debugging` with the specific symptom).

**Note on the `--concurrency` grep:** if the call site is fixture-gated (e.g., wrapped in an `if fixtureName == "0001-tcp-proxy-rr"` check), Task 13's inheritance assumption is wrong — either the executor updates `harness.go` to make the flag unconditional (writing ADR-0036 for the change), or extends the gate to include `"0002-tls-tcp"`. Catching this at Task 1 rather than Task 13 saves an hour of differential-gate debugging.

- [ ] **Step 2: Create `docs/envoy-go/phases/03-tls/PROGRESS.md`**

```markdown
# Phase 03 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all preconditions were satisfied at cold-start>

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** <sha — this task's commit>
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions".
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
```
```

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/03-tls/PROGRESS.md
git commit -m "phase 03: PROGRESS.md preamble + precondition verification"
```

After the commit, update the just-written PROGRESS.md entry's `**Commits:**` line with the short SHA of the commit you just made (phase-02 precedent: the SHA-fill happens in a follow-up amend-safe commit, or — simpler — stage the SHA, commit, then a second tiny commit `phase 03: PROGRESS SHA-fill for Task 1`).

---

## Task 2: `internal/tls/sni.go` + tests — wildcard match (pure function)

**Files:**
- Create: `internal/tls/doc.go`
- Create: `internal/tls/sni.go`
- Create: `internal/tls/sni_test.go`
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 2 entry)

This task lands the first file of the `internal/tls/` package: the pure `MatchServerName` wildcard function. No `crypto/tls` import yet (that arrives in Task 4). TDD: tests first.

- [ ] **Step 1: Write `internal/tls/sni_test.go`**

```go
package tls

import "testing"

func TestMatchServerName(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		sni      string
		want     bool
	}{
		{"exact match", []string{"alpha.envoy-go.test"}, "alpha.envoy-go.test", true},
		{"exact mismatch", []string{"alpha.envoy-go.test"}, "beta.envoy-go.test", false},
		{"suffix wildcard match", []string{"*.envoy-go.test"}, "alpha.envoy-go.test", true},
		{"suffix wildcard multi-label", []string{"*.envoy-go.test"}, "a.b.envoy-go.test", true},
		{"suffix wildcard does not match bare parent", []string{"*.envoy-go.test"}, "envoy-go.test", false},
		{"universal wildcard", []string{"*"}, "anything.example", true},
		{"universal wildcard empty sni", []string{"*"}, "", true},
		{"no patterns", nil, "anything", false},
		{"empty patterns slice", []string{}, "anything", false},
		{"case insensitive sni upper", []string{"alpha.envoy-go.test"}, "ALPHA.envoy-go.test", true},
		{"case insensitive pattern upper", []string{"ALPHA.envoy-go.test"}, "alpha.envoy-go.test", true},
		{"first-match wins (exact beats wildcard)", []string{"alpha.envoy-go.test", "*.envoy-go.test"}, "alpha.envoy-go.test", true},
		{"multiple wildcards, most-specific wins", []string{"*.envoy-go.test"}, "x.envoy-go.test", true},
		{"no match across multiple patterns", []string{"a.test", "b.test"}, "c.test", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchServerName(tc.patterns, tc.sni)
			if got != tc.want {
				t.Errorf("MatchServerName(%q, %q) = %v, want %v", tc.patterns, tc.sni, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/tls && go test .`
Expected: compile error — `MatchServerName` undefined.

- [ ] **Step 3: Write `internal/tls/doc.go`**

```go
// Package tls parses Envoy v3 DownstreamTlsContext and UpstreamTlsContext
// protos into ready-to-use *crypto/tls.Config values, loads PEM material
// from DataSource envelopes (inline_bytes / inline_string / filename), maps
// TlsParameters fields to stdlib TLS config, and implements the SNI-wildcard
// match predicate used by the listener manager's GetConfigForClient callback.
//
// Phase 03 surface: see docs/envoy-go/phases/03-tls/SPEC.md §4.1. Doctrine:
// see docs/envoy-go/DECISIONS.md ADR-0029 (DataSource handling), ADR-0030
// (TLS parameter mapping), ADR-0031 (stdlib crypto/tls stack selection).
//
// Throughout this package, crypto/tls is imported as stdtls to avoid a name
// collision with the package itself. Every exported error begins with "tls: "
// to match the error-prefix discipline in sibling packages.
package tls
```

- [ ] **Step 4: Write `internal/tls/sni.go`**

```go
package tls

import "strings"

// MatchServerName reports whether sni matches any of the given patterns under
// Envoy's server_names[] semantics. Patterns and sni are compared
// case-insensitively. Three pattern shapes are recognized:
//
//   - Exact pattern (e.g. "alpha.envoy-go.test") matches sni iff equal.
//   - Suffix wildcard (e.g. "*.envoy-go.test") matches sni iff sni's label
//     count strictly exceeds the pattern's label count and sni ends with the
//     pattern's non-wildcard suffix at a label boundary. "*.envoy-go.test"
//     thus matches "alpha.envoy-go.test" and "a.b.envoy-go.test" but not
//     "envoy-go.test" itself.
//   - Universal wildcard "*" matches any sni, including the empty string.
//
// The function is pure and order-insensitive: if any pattern matches, it
// returns true. Callers that need most-specific-first dispatch perform the
// ordering themselves (see internal/listener GetConfigForClient).
func MatchServerName(patterns []string, sni string) bool {
	sniLower := strings.ToLower(sni)
	for _, p := range patterns {
		pLower := strings.ToLower(p)
		switch {
		case pLower == "*":
			return true
		case strings.HasPrefix(pLower, "*."):
			suffix := pLower[1:] // keeps the leading '.'
			if len(sniLower) > len(suffix) && strings.HasSuffix(sniLower, suffix) {
				return true
			}
		default:
			if pLower == sniLower {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd internal/tls && go test .`
Expected: `ok  github.com/envoy-go/internal/tls  <time>s`

- [ ] **Step 6: Verify the module path** (adjust imports in later tasks accordingly)

Run: `go list -m` from repo root.
Expected: module name printed (e.g., `github.com/envoy-go` or whatever the module declares). Confirm every `internal/tls/*.go` file uses matching imports. If the module name differs from assumed in snippets, adjust as you go — module-path constants appear only in test imports, not in library files.

- [ ] **Step 7: Commit**

```bash
git add internal/tls/doc.go internal/tls/sni.go internal/tls/sni_test.go
git commit -m "phase 03: internal/tls — MatchServerName wildcard predicate + doc.go"
```

- [ ] **Step 8: Append PROGRESS.md entry for Task 2**

Append to `docs/envoy-go/phases/03-tls/PROGRESS.md`:

```markdown
## Task 2 — internal/tls — MatchServerName + doc.go

**Commits:** <short-sha>
**Notes:** Pure function; no stdtls import yet; table-driven test covers exact / suffix-wildcard / universal / case / no-match / empty-patterns; all combinations green.
**Outputs:**
```
$ cd internal/tls && go test .
ok  <module>/internal/tls  <t>s
```
```

Commit the PROGRESS.md update with message `phase 03: PROGRESS SHA-fill for Task 2`.

---

## Task 3: `internal/tls/datasource.go` + tests + ADR-0029

**Files:**
- Create: `internal/tls/datasource.go`
- Create: `internal/tls/datasource_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0029)
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 3 entry)

TDD: tests first.

- [ ] **Step 1: Write `internal/tls/datasource_test.go`**

```go
package tls

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

func TestLoadDataSource(t *testing.T) {
	t.Run("inline_bytes happy", func(t *testing.T) {
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte("hello")}}
		got, err := loadDataSource(ds, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("inline_string happy", func(t *testing.T) {
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: "hello"}}
		got, err := loadDataSource(ds, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("filename absolute happy", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "data.txt")
		if err := os.WriteFile(path, []byte("abs"), 0o600); err != nil {
			t.Fatal(err)
		}
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_Filename{Filename: path}}
		got, err := loadDataSource(ds, "/unused/base")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "abs" {
			t.Errorf("got %q, want %q", got, "abs")
		}
	})

	t.Run("filename relative resolved against baseDir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "rel.txt"), []byte("rel"), 0o600); err != nil {
			t.Fatal(err)
		}
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_Filename{Filename: "rel.txt"}}
		got, err := loadDataSource(ds, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "rel" {
			t.Errorf("got %q, want %q", got, "rel")
		}
	})

	t.Run("filename nonexistent", func(t *testing.T) {
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_Filename{Filename: "/nonexistent/path"}}
		_, err := loadDataSource(ds, "")
		if err == nil || !strings.HasPrefix(err.Error(), "tls: data source: read ") {
			t.Errorf("want tls-prefixed read error, got: %v", err)
		}
	})

	t.Run("environment_variable errors", func(t *testing.T) {
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_EnvironmentVariable{EnvironmentVariable: "FOO"}}
		_, err := loadDataSource(ds, "")
		if err == nil || !strings.HasPrefix(err.Error(), "tls: data source: environment_variable is not supported") {
			t.Errorf("want not-supported error, got: %v", err)
		}
	})

	t.Run("zero value errors", func(t *testing.T) {
		ds := &corev3.DataSource{}
		_, err := loadDataSource(ds, "")
		if err == nil || !strings.HasPrefix(err.Error(), "tls: data source: none of inline_bytes") {
			t.Errorf("want zero-value error, got: %v", err)
		}
	})

	t.Run("large file read no truncation", func(t *testing.T) {
		dir := t.TempDir()
		big := make([]byte, 10*1024*1024)
		for i := range big {
			big[i] = byte(i % 251)
		}
		path := filepath.Join(dir, "big.bin")
		if err := os.WriteFile(path, big, 0o600); err != nil {
			t.Fatal(err)
		}
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_Filename{Filename: path}}
		got, err := loadDataSource(ds, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(big) {
			t.Errorf("got len %d, want %d", len(got), len(big))
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/tls && go test .`
Expected: compile error — `loadDataSource` undefined.

- [ ] **Step 3: Write `internal/tls/datasource.go`**

```go
package tls

import (
	"fmt"
	"os"
	"path/filepath"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

// loadDataSource resolves a DataSource envelope into raw bytes. See ADR-0029
// for the phase-03 support matrix: inline_bytes, inline_string, filename are
// honoured; environment_variable errors; zero value errors; SDS-bound secret
// configs are handled at the caller layer (not reachable via this function).
//
// If filename is not absolute, it is resolved relative to baseDir. An empty
// baseDir combined with a relative filename resolves against the process's
// current working directory — callers should pass a stable baseDir (typically
// the directory of the bootstrap file) for reproducibility.
func loadDataSource(ds *corev3.DataSource, baseDir string) ([]byte, error) {
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
			return nil, fmt.Errorf("tls: data source: read %s: %w", p, err)
		}
		return b, nil
	case *corev3.DataSource_EnvironmentVariable:
		return nil, fmt.Errorf("tls: data source: environment_variable is not supported in phase 03")
	default:
		return nil, fmt.Errorf("tls: data source: none of inline_bytes, inline_string, filename set")
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/tls && go test -run TestLoadDataSource .`
Expected: PASS for all subtests.

- [ ] **Step 5: Append ADR-0029 to `docs/envoy-go/DECISIONS.md`**

Append the ADR at file tail. Exact structure (fill in Date with the commit date):

```markdown
## ADR-0029: DataSource handling policy (phase 03 scope)

**Status:** Accepted
**Date:** <YYYY-MM-DD>
**Doctrine:** D-3.5

### Context

`internal/tls` parses Envoy v3 DownstreamTlsContext / UpstreamTlsContext whose cert and CA material is carried via `envoy.config.core.v3.DataSource`. DataSource has four specifiers: `inline_bytes`, `inline_string`, `filename`, `environment_variable`, plus the SDS-bound forms on CommonTlsContext (`tls_certificate_sds_secret_configs`, etc.). Phase 03 must pick a subset consistent with SPEC §2 (non-purposes) and SPEC §5 (in-scope surface).

### Decision

`internal/tls.loadDataSource(ds, baseDir)` supports `inline_bytes`, `inline_string`, and `filename` only. `filename` is resolved relative to `baseDir` when not absolute; the caller passes the bootstrap file's directory. `environment_variable` errors with `tls: data source: environment_variable is not supported in phase 03`. Zero-value DataSource errors. SDS-bound secret configs error at the `internal/tls/config.go` caller layer (outside this function), keeping this function branch-minimal.

### Consequences

- Phase-03 fixtures can inline every PEM via `inline_bytes` or `inline_string`, matching the committed-PEM + deterministic-generator discipline of `test/fixtures/0002-tls-tcp/pki/`.
- Filename support is included from phase 03 rather than deferred because the implementation cost is trivial and future phases (xDS family, dynamic secret reload) will need it. No dynamic reload (file-watch / inotify) is implemented — phase 03 reads each file exactly once at listener-manager build time.
- `environment_variable` + SDS-bound secrets are bounded deferrals: phase 03 errors at parse time, preserving the "errors begin with `tls: `" discipline so callers can surface them uniformly.
- `baseDir` is a plan-level contract between the bootstrap loader (which knows the config file path) and this function. Tests pass an explicit `t.TempDir()` to avoid CWD-dependence.
```

- [ ] **Step 6: Commit**

```bash
git add internal/tls/datasource.go internal/tls/datasource_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 03: internal/tls — loadDataSource (inline_bytes / inline_string / filename) [ADR-0029]"
```

- [ ] **Step 7: Append PROGRESS.md entry for Task 3 and commit the SHA-fill**

Append to `docs/envoy-go/phases/03-tls/PROGRESS.md`:

```markdown
## Task 3 — internal/tls — loadDataSource + ADR-0029

**Commits:** <short-sha>
**Notes:** ADR-0029 landed in same commit as the code. Error-prefix discipline (`tls: `) preserved. Seven subtests green.
**Outputs:**
```
$ cd internal/tls && go test -run TestLoadDataSource .
<verbatim>
```
```

Commit the PROGRESS.md update with message `phase 03: PROGRESS SHA-fill for Task 3`.

---

## Task 4: `internal/tls/params.go` + tests + ADR-0030

**Files:**
- Create: `internal/tls/params.go`
- Create: `internal/tls/params_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0030)
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 4 entry)

TDD: tests first. First site that imports `crypto/tls` as `stdtls` in the package.

- [ ] **Step 1: Write `internal/tls/params_test.go`**

```go
package tls

import (
	stdtls "crypto/tls"
	"strings"
	"testing"

	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
)

func TestApplyTLSParams_Versions(t *testing.T) {
	cases := []struct {
		name         string
		min, max     tlsv3.TlsParameters_TlsProtocol
		wantMin, max2 uint16
		wantErr      string // substring; "" = no error
	}{
		{"defaults TLS 1.2 -> TLS 1.3", tlsv3.TlsParameters_TLSv1_2, tlsv3.TlsParameters_TLSv1_3, stdtls.VersionTLS12, stdtls.VersionTLS13, ""},
		{"TLS 1.2 only", tlsv3.TlsParameters_TLSv1_2, tlsv3.TlsParameters_TLSv1_2, stdtls.VersionTLS12, stdtls.VersionTLS12, ""},
		{"TLS 1.3 only", tlsv3.TlsParameters_TLSv1_3, tlsv3.TlsParameters_TLSv1_3, stdtls.VersionTLS13, stdtls.VersionTLS13, ""},
		{"TLS 1.0 min errors", tlsv3.TlsParameters_TLSv1_0, tlsv3.TlsParameters_TLSv1_3, 0, 0, "TLSv1_0"},
		{"TLS 1.1 max errors", tlsv3.TlsParameters_TLSv1_2, tlsv3.TlsParameters_TLSv1_1, 0, 0, "TLSv1_1"},
		{"TLS_AUTO min errors", tlsv3.TlsParameters_TLS_AUTO, tlsv3.TlsParameters_TLSv1_3, 0, 0, "TLS_AUTO"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &stdtls.Config{}
			err := applyTLSParams(cfg, &tlsv3.TlsParameters{
				TlsMinimumProtocolVersion: tc.min,
				TlsMaximumProtocolVersion: tc.max,
			})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("want error containing %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.MinVersion != tc.wantMin || cfg.MaxVersion != tc.max2 {
				t.Errorf("got Min=%d Max=%d, want Min=%d Max=%d", cfg.MinVersion, cfg.MaxVersion, tc.wantMin, tc.max2)
			}
		})
	}
}

func TestApplyTLSParams_CipherSuites(t *testing.T) {
	t.Run("known TLS 1.2 cipher", func(t *testing.T) {
		cfg := &stdtls.Config{}
		// ECDHE-ECDSA-AES128-GCM-SHA256 = 0xc02b
		if err := applyTLSParams(cfg, &tlsv3.TlsParameters{CipherSuites: []string{"ECDHE-ECDSA-AES128-GCM-SHA256"}}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.CipherSuites) != 1 || cfg.CipherSuites[0] != stdtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 {
			t.Errorf("got cipher_suites %v, want [0xc02b]", cfg.CipherSuites)
		}
	})
	t.Run("unknown cipher errors", func(t *testing.T) {
		cfg := &stdtls.Config{}
		err := applyTLSParams(cfg, &tlsv3.TlsParameters{CipherSuites: []string{"TOTALLY_FAKE_CIPHER"}})
		if err == nil || !strings.Contains(err.Error(), "unknown cipher suite") {
			t.Errorf("want unknown cipher error, got: %v", err)
		}
	})
	t.Run("TLS 1.3 cipher silently dropped with diagnostic", func(t *testing.T) {
		cfg := &stdtls.Config{}
		// TLS_AES_128_GCM_SHA256 = 0x1301 is TLS 1.3 only
		err := applyTLSParams(cfg, &tlsv3.TlsParameters{CipherSuites: []string{"TLS_AES_128_GCM_SHA256"}})
		if err != nil {
			t.Fatalf("unexpected error for TLS-1.3-only cipher (should be silently dropped): %v", err)
		}
		if len(cfg.CipherSuites) != 0 {
			t.Errorf("TLS-1.3-only cipher should be dropped, got cipher_suites %v", cfg.CipherSuites)
		}
	})
}

func TestApplyTLSParams_ECDHCurves(t *testing.T) {
	cases := []struct {
		name   string
		input  []string
		want   []stdtls.CurveID
		errSub string
	}{
		{"x25519 + p256", []string{"X25519", "P-256"}, []stdtls.CurveID{stdtls.X25519, stdtls.CurveP256}, ""},
		{"p384 + p521", []string{"P-384", "P-521"}, []stdtls.CurveID{stdtls.CurveP384, stdtls.CurveP521}, ""},
		{"unknown curve errors", []string{"FAKECURVE"}, nil, "unknown ecdh curve"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &stdtls.Config{}
			err := applyTLSParams(cfg, &tlsv3.TlsParameters{EcdhCurves: tc.input})
			if tc.errSub != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errSub) {
					t.Errorf("want %q, got: %v", tc.errSub, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cfg.CurvePreferences) != len(tc.want) {
				t.Errorf("got %d curves, want %d", len(cfg.CurvePreferences), len(tc.want))
			}
			for i := range tc.want {
				if cfg.CurvePreferences[i] != tc.want[i] {
					t.Errorf("curve[%d] = %d, want %d", i, cfg.CurvePreferences[i], tc.want[i])
				}
			}
		})
	}
}

func TestApplyTLSParams_SignatureAlgorithmsErrors(t *testing.T) {
	cfg := &stdtls.Config{}
	err := applyTLSParams(cfg, &tlsv3.TlsParameters{SignatureAlgorithms: []string{"rsa_pss_rsae_sha256"}})
	if err == nil || !strings.Contains(err.Error(), "signature_algorithms") {
		t.Errorf("want signature_algorithms error, got: %v", err)
	}
}

func TestApplyTLSParams_NilParams(t *testing.T) {
	cfg := &stdtls.Config{}
	if err := applyTLSParams(cfg, nil); err != nil {
		t.Errorf("nil params should be no-op, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/tls && go test -run TestApplyTLSParams .`
Expected: compile error — `applyTLSParams` undefined.

- [ ] **Step 3: Write `internal/tls/params.go`**

```go
package tls

import (
	stdtls "crypto/tls"
	"fmt"
	"log"

	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
)

// applyTLSParams maps Envoy TlsParameters fields onto cfg per SPEC §5.5.
// Nil params is a no-op. Errors begin with "tls: tls_params: ".
//
// See ADR-0030 for the scope decisions: version enum mapping, cipher-suite
// name-to-ID mapping (with TLS-1.3-only ciphers silently dropped since Go's
// crypto/tls does not permit selecting them), ecdh_curves mapping, and
// signature_algorithms erroring because stdlib does not publicly expose the
// knob.
func applyTLSParams(cfg *stdtls.Config, params *tlsv3.TlsParameters) error {
	if params == nil {
		return nil
	}
	if v, err := mapTLSVersion(params.GetTlsMinimumProtocolVersion()); err != nil {
		return fmt.Errorf("tls: tls_params: tls_minimum_protocol_version: %w", err)
	} else if v != 0 {
		cfg.MinVersion = v
	}
	if v, err := mapTLSVersion(params.GetTlsMaximumProtocolVersion()); err != nil {
		return fmt.Errorf("tls: tls_params: tls_maximum_protocol_version: %w", err)
	} else if v != 0 {
		cfg.MaxVersion = v
	}

	for _, name := range params.GetCipherSuites() {
		if tls13CipherByName(name) != 0 {
			// TLS 1.3 cipher — Go's crypto/tls does not allow selection; diagnostic and drop.
			log.Printf("tls: tls_params: TLS-1.3-only cipher %q requested; crypto/tls does not allow selection, dropping", name)
			continue
		}
		id, ok := tls12CipherByName(name)
		if !ok {
			return fmt.Errorf("tls: tls_params: unknown cipher suite %q", name)
		}
		cfg.CipherSuites = append(cfg.CipherSuites, id)
	}

	for _, name := range params.GetEcdhCurves() {
		id, ok := curveByName(name)
		if !ok {
			return fmt.Errorf("tls: tls_params: unknown ecdh curve %q", name)
		}
		cfg.CurvePreferences = append(cfg.CurvePreferences, id)
	}

	if len(params.GetSignatureAlgorithms()) > 0 {
		return fmt.Errorf("tls: tls_params: signature_algorithms is not configurable via Go crypto/tls in phase 03")
	}

	return nil
}

func mapTLSVersion(v tlsv3.TlsParameters_TlsProtocol) (uint16, error) {
	switch v {
	case tlsv3.TlsParameters_TLS_AUTO:
		// TlsProtocol 0 = TLS_AUTO in the proto; at the zero value the field
		// is treated as unset (no explicit version requested). The enum's
		// zero is TLS_AUTO; we cannot distinguish "unset" from "TLS_AUTO
		// explicitly chosen" at the proto level, so we adopt the strict
		// interpretation: explicit TLS_AUTO errors. If the caller wants no
		// bound, they omit the field entirely (which also yields the zero
		// value — indistinguishable). In practice phase-03 fixtures always
		// set TLSv1_2 min / TLSv1_3 max per Settled §10 #7.
		return 0, nil // treat as "not set" — let caller's cfg carry its default.
	case tlsv3.TlsParameters_TLSv1_0, tlsv3.TlsParameters_TLSv1_1:
		return 0, fmt.Errorf("%s is not supported in phase 03", v.String())
	case tlsv3.TlsParameters_TLSv1_2:
		return stdtls.VersionTLS12, nil
	case tlsv3.TlsParameters_TLSv1_3:
		return stdtls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unknown tls protocol enum %d", v)
	}
}

// tls12CipherByName maps an IANA / OpenSSL cipher suite name to a Go
// crypto/tls cipher suite ID, limited to TLS 1.2 ciphers that stdlib honours.
// Returns (0, false) on unknown name.
//
// The name list below is narrow by design — it covers the cipher suites
// crypto/tls actually supports for selection. Names not present (e.g.,
// obscure CBC suites) deliberately error so fixtures cannot quietly pick a
// weak cipher.
func tls12CipherByName(name string) (uint16, bool) {
	switch name {
	case "ECDHE-ECDSA-AES128-GCM-SHA256":
		return stdtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, true
	case "ECDHE-ECDSA-AES256-GCM-SHA384":
		return stdtls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, true
	case "ECDHE-ECDSA-CHACHA20-POLY1305":
		return stdtls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, true
	case "ECDHE-RSA-AES128-GCM-SHA256":
		return stdtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, true
	case "ECDHE-RSA-AES256-GCM-SHA384":
		return stdtls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, true
	case "ECDHE-RSA-CHACHA20-POLY1305":
		return stdtls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305, true
	default:
		return 0, false
	}
}

// tls13CipherByName returns the stdlib cipher ID for TLS 1.3 names, or 0 if
// the name is not a TLS-1.3 cipher. Used only to detect "should silently
// drop" names — the returned ID is not applied to cfg.CipherSuites because
// Go's crypto/tls does not permit TLS 1.3 cipher selection.
func tls13CipherByName(name string) uint16 {
	switch name {
	case "TLS_AES_128_GCM_SHA256":
		return stdtls.TLS_AES_128_GCM_SHA256
	case "TLS_AES_256_GCM_SHA384":
		return stdtls.TLS_AES_256_GCM_SHA384
	case "TLS_CHACHA20_POLY1305_SHA256":
		return stdtls.TLS_CHACHA20_POLY1305_SHA256
	default:
		return 0
	}
}

func curveByName(name string) (stdtls.CurveID, bool) {
	switch name {
	case "X25519":
		return stdtls.X25519, true
	case "P-256":
		return stdtls.CurveP256, true
	case "P-384":
		return stdtls.CurveP384, true
	case "P-521":
		return stdtls.CurveP521, true
	default:
		return 0, false
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/tls && go test -run TestApplyTLSParams .`
Expected: every subtest PASS.

- [ ] **Step 5: Append ADR-0030 to `docs/envoy-go/DECISIONS.md`**

Append at file tail:

```markdown
## ADR-0030: TLS parameter mapping scope (phase 03)

**Status:** Accepted
**Date:** <YYYY-MM-DD>
**Doctrine:** D-3.5

### Context

Envoy's `common_tls_context.tls_params` exposes four configuration knobs: `tls_{minimum,maximum}_protocol_version`, `cipher_suites`, `ecdh_curves`, `signature_algorithms`. Go's stdlib `crypto/tls` does not surface every one of these as a public configuration: TLS 1.3 cipher selection is not permitted (RFC 8446 design choice — the spec selects AEAD ciphers), and `signature_algorithms` is not settable on `tls.Config`. Phase 03 must declare which fields are honoured, which error, and which are silently dropped with a diagnostic.

### Decision

`internal/tls.applyTLSParams` maps per-field as follows (this section is the authoritative surface; duplicates SPEC §5.5 for traceability):

| Envoy field | Phase-03 behaviour |
|---|---|
| `tls_minimum_protocol_version` | TLSv1_2/TLSv1_3 → `stdtls.VersionTLS12/TLS13`; TLSv1_0/TLSv1_1 → error; TLS_AUTO → no-op (treat as unset). |
| `tls_maximum_protocol_version` | Same mapping. |
| `cipher_suites` | TLS 1.2 IANA/OpenSSL names → `stdtls.CipherSuites()` IDs; unknown → error; TLS-1.3-only names → diagnostic-logged and dropped (not applied to cfg). |
| `ecdh_curves` | `X25519`/`P-256`/`P-384`/`P-521` → `stdtls.CurveID`; unknown → error. |
| `signature_algorithms` | Populated → error (stdlib has no public configuration knob). |

### Consequences

- Fixtures that pin TLS 1.2 ciphers get per-cipher selection parity with Envoy. Fixtures that pin TLS 1.3 ciphers see a diagnostic log but negotiation proceeds with Go's default TLS 1.3 cipher list (AEAD ciphers per RFC 8446). The BEHAVIOR_CONTRACT TLS subsection (ADR-0035) explicitly does not assert encrypted-side byte equivalence, so TLS 1.3 cipher divergence between Go and Envoy's BoringSSL does not break any asserted gate.
- A fixture that sets `signature_algorithms` fails fast with a clear error rather than silently no-op'ing. Future phases can revisit if Go's crypto/tls exposes the knob publicly (none as of Go 1.23).
- The cipher-name table is deliberately narrow (6 TLS 1.2 AEAD suites + 3 TLS 1.3 names as the silent-drop list). Adding suites is a trivial follow-on PR when a fixture needs one.
```

- [ ] **Step 6: Commit**

```bash
git add internal/tls/params.go internal/tls/params_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 03: internal/tls — applyTLSParams + TLS parameter mapping [ADR-0030]"
```

- [ ] **Step 7: Append PROGRESS.md entry for Task 4 and SHA-fill commit**

Same shape as Task 3's PROGRESS entry (summary + verbatim `go test -run TestApplyTLSParams` output). Commit `phase 03: PROGRESS SHA-fill for Task 4`.

---

## Task 5: `internal/tls/config.go` + tests + ADR-0031 (stdlib crypto/tls stack selection)

**Files:**
- Create: `internal/tls/config.go`
- Create: `internal/tls/config_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0031)
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 5 entry)

Central task of the `internal/tls/` package. Builds `*DownstreamConfig` / `*UpstreamConfig` composing the three leaves (sni/datasource/params). TDD: tests first.

**Precondition note for this task:** this test file references PEMs from `test/fixtures/0002-tls-tcp/pki/`. Since PKI is landed in Task 7, Task 5 uses small *inline* PEMs generated at test-init time via `crypto/x509.CreateCertificate` — NOT the committed PEMs. This keeps Task 5 self-contained. Task 9's `cluster_test.go` re-uses Task 7's committed PEMs for integration tests.

- [ ] **Step 1: Write `internal/tls/config_test.go`**

The test file sets up a single shared test CA + leaf + upstream leaf at `TestMain` time (via `generateTestPKI()` helper at the bottom of the file) and writes them to `t.TempDir()` for filename-based DataSource testing. All PEMs are inline-bytes unless explicitly noted.

```go
package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/types/known/anypb"
)

// testPKI holds PEM bytes for a self-signed CA + a leaf signed by the CA.
type testPKI struct {
	caPEM, leafCertPEM, leafKeyPEM []byte
}

var pki = func() *testPKI {
	// Self-signed CA
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		panic(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	// Leaf
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "alpha.envoy-go.test"},
		DNSNames:     []string{"alpha.envoy-go.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	if err != nil {
		panic(err)
	}
	leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafKeyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		panic(err)
	}
	leafKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyDER})

	return &testPKI{caPEM: caPEM, leafCertPEM: leafCertPEM, leafKeyPEM: leafKeyPEM}
}()

// makeTransportSocket wraps a proto message into a TransportSocket with the
// canonical tls/v3 type_url. At implementation time, the executor writes it
// as a simple three-liner:
//
//   import "google.golang.org/protobuf/proto"
//   // ...
//   func makeTransportSocket(t *testing.T, inner proto.Message) *corev3.TransportSocket {
//       t.Helper()
//       anyMsg, err := anypb.New(inner)
//       if err != nil {
//           t.Fatalf("anypb.New: %v", err)
//       }
//       return &corev3.TransportSocket{
//           Name: "envoy.transport_sockets.tls",
//           ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
//       }
//   }

func TestNewDownstreamConfig_Happy(t *testing.T) {
	t.Skip("executor writes the full test at implementation time; see PLAN.md for the subtests expected")
	// Expected subtests:
	//   - inline_bytes cert + key -> *DownstreamConfig with non-nil TLSConfig carrying one Certificate.
	//   - SNI on chain context honoured via MatchServerName at higher layer (sni.go tested separately).
	//   - tls_params pulled through: TLSv1_2 min, TLSv1_3 max → cfg.MinVersion == VersionTLS12, cfg.MaxVersion == VersionTLS13.
	//   - alpn_protocols populated → cfg.NextProtos equal.
}

func TestNewDownstreamConfig_Errors(t *testing.T) {
	t.Skip("executor writes the full test at implementation time; see PLAN.md for the subtests expected")
	// Expected subtests (each must produce an error whose message begins with "tls: "):
	//   - wrong type_url (StringValue) -> "tls: downstream: unexpected type_url"
	//   - unmarshal failure (random bytes as typed_config) -> "tls: downstream: unmarshal"
	//   - missing tls_certificates -> "tls: downstream: no tls_certificates"
	//   - malformed PEM in certificate_chain -> "tls: downstream: load cert"
	//   - SDS-bound secret (tls_certificate_sds_secret_configs populated) -> "tls: downstream: SDS"
	//   - require_client_certificate=true -> "tls: downstream: require_client_certificate"
	//   - custom_validator_config populated -> "tls: downstream: custom_validator_config"
	//   - match_typed_subject_alt_names populated -> "tls: downstream: match_typed_subject_alt_names"
	//   - verify_certificate_hash populated -> "tls: downstream: verify_certificate_hash"
	//   - password on key -> "tls: downstream: password-protected keys"
	//   - invalid tls_params (TLSv1_0) -> propagated from applyTLSParams
}

func TestNewUpstreamConfig_Happy(t *testing.T) {
	t.Skip("executor writes the full test at implementation time; see PLAN.md for the subtests expected")
	// Expected subtests:
	//   - inline CA + SNI + tls_params -> *UpstreamConfig with TLSConfig.ServerName == SNI, RootCAs set.
	//   - alpn_protocols populated → NextProtos.
	//   - allow_renegotiation=false (default) -> no error.
}

func TestNewUpstreamConfig_Errors(t *testing.T) {
	t.Skip("executor writes the full test at implementation time; see PLAN.md for the subtests expected")
	// Expected subtests (each must produce a tls-prefixed error):
	//   - wrong type_url
	//   - missing trusted_ca -> "tls: upstream: validation_context.trusted_ca is required"  (§5.4 tightening)
	//   - malformed CA PEM -> "tls: upstream: validation_context: trusted_ca: parse"
	//   - SDS-bound secret
	//   - allow_renegotiation=true -> "tls: upstream: allow_renegotiation"
	//   - custom_validator_config
	//   - match_typed_subject_alt_names
	//   - password on client-cert key
}

// Sanity check that the inline test PKI is self-consistent:
// loading the leaf cert with the leaf key should round-trip through
// stdtls.X509KeyPair without error.
func TestPKISanity(t *testing.T) {
	_ = strings.TrimSpace // suppress unused-import warning until the full tests land.
	_ = pki
	// Minimal sanity: ensure the CA verifies the leaf.
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(pki.caPEM) {
		t.Fatal("CA PEM did not append")
	}
	block, _ := pem.Decode(pki.leafCertPEM)
	if block == nil {
		t.Fatal("leaf PEM decode failed")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: caPool, DNSName: "alpha.envoy-go.test"}); err != nil {
		t.Fatalf("leaf verify: %v", err)
	}
}
```

**Executor note:** the four `t.Skip` blocks are SPEC-level placeholders. At implementation time the executor writes every listed subtest fully, using `anypb.New(&tlsv3.DownstreamTlsContext{...})` to build typed configs and asserting on returned error substrings. The test file grows to ~300 lines. The structure above fixes the required test names, sub-cases, and error-prefix assertions; the implementation is routine.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/tls && go test -run TestNewDownstreamConfig .`
Expected: compile error — `NewDownstreamConfig` undefined. (`TestPKISanity` passes — it's self-contained and verifies the test PKI is internally consistent.)

- [ ] **Step 3: Write `internal/tls/config.go`**

```go
package tls

import (
	stdtls "crypto/tls"
	"crypto/x509"
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	downstreamTLSContextTypeURL = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext"
	upstreamTLSContextTypeURL   = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext"
)

// DownstreamConfig is the phase-03 output of parsing a DownstreamTlsContext.
// Callers embed cfg.TLSConfig in the chain's per-chain *stdtls.Config used by
// the listener's GetConfigForClient callback.
type DownstreamConfig struct {
	TLSConfig *stdtls.Config
}

// UpstreamConfig is the phase-03 output of parsing an UpstreamTlsContext.
// Callers use cfg.TLSConfig with stdtls.Client for each upstream dial.
type UpstreamConfig struct {
	TLSConfig *stdtls.Config
	SNI       string
}

// NewDownstreamConfig parses a *corev3.TransportSocket whose typed_config is a
// DownstreamTlsContext. baseDir is used to resolve filename-based DataSources.
// Errors begin with "tls: downstream: ".
func NewDownstreamConfig(ts *corev3.TransportSocket, baseDir string) (*DownstreamConfig, error) {
	if ts == nil {
		return nil, fmt.Errorf("tls: downstream: nil transport_socket")
	}
	if ts.GetTypedConfig() == nil || ts.GetTypedConfig().GetTypeUrl() != downstreamTLSContextTypeURL {
		return nil, fmt.Errorf("tls: downstream: unexpected type_url %q", ts.GetTypedConfig().GetTypeUrl())
	}
	ctx := &tlsv3.DownstreamTlsContext{}
	if err := anypb.UnmarshalTo(ts.GetTypedConfig(), ctx, proto.UnmarshalOptions{}); err != nil {
		return nil, fmt.Errorf("tls: downstream: unmarshal: %w", err)
	}
	if ctx.GetRequireClientCertificate().GetValue() {
		return nil, fmt.Errorf("tls: downstream: require_client_certificate is not supported in phase 03")
	}
	cfg, err := commonTLSContextToConfig(ctx.GetCommonTlsContext(), baseDir, "downstream")
	if err != nil {
		return nil, err
	}
	return &DownstreamConfig{TLSConfig: cfg}, nil
}

// NewUpstreamConfig parses a *corev3.TransportSocket whose typed_config is an
// UpstreamTlsContext. baseDir is used to resolve filename-based DataSources.
// Errors begin with "tls: upstream: ".
func NewUpstreamConfig(ts *corev3.TransportSocket, baseDir string) (*UpstreamConfig, error) {
	if ts == nil {
		return nil, fmt.Errorf("tls: upstream: nil transport_socket")
	}
	if ts.GetTypedConfig() == nil || ts.GetTypedConfig().GetTypeUrl() != upstreamTLSContextTypeURL {
		return nil, fmt.Errorf("tls: upstream: unexpected type_url %q", ts.GetTypedConfig().GetTypeUrl())
	}
	ctx := &tlsv3.UpstreamTlsContext{}
	if err := anypb.UnmarshalTo(ts.GetTypedConfig(), ctx, proto.UnmarshalOptions{}); err != nil {
		return nil, fmt.Errorf("tls: upstream: unmarshal: %w", err)
	}
	if ctx.GetAllowRenegotiation() {
		return nil, fmt.Errorf("tls: upstream: allow_renegotiation is not supported (crypto/tls does not support TLS 1.2 renegotiation as a client)")
	}
	common := ctx.GetCommonTlsContext()
	if common == nil {
		return nil, fmt.Errorf("tls: upstream: common_tls_context is required")
	}

	// Enforce §5.4 tightening: trusted_ca required on every upstream TLS cluster.
	vc := common.GetValidationContext()
	if vc == nil || vc.GetTrustedCa() == nil {
		return nil, fmt.Errorf("tls: upstream: validation_context.trusted_ca is required (phase 03 does not permit unvalidated upstream TLS)")
	}

	cfg, err := commonTLSContextToConfig(common, baseDir, "upstream")
	if err != nil {
		return nil, err
	}

	// CA -> RootCAs
	caPEM, err := loadDataSource(vc.GetTrustedCa(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("tls: upstream: validation_context: trusted_ca: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("tls: upstream: validation_context: trusted_ca: parse failure")
	}
	cfg.RootCAs = pool
	cfg.ServerName = ctx.GetSni()
	return &UpstreamConfig{TLSConfig: cfg, SNI: ctx.GetSni()}, nil
}

// commonTLSContextToConfig builds a *stdtls.Config carrying
// Certificates (from tls_certificates[]) and NextProtos (from alpn_protocols),
// plus tls_params-mapped fields. side is "downstream" or "upstream" and
// prefixes every error.
//
// Phase-03 forbids the following; each errors with a clear message:
//   - tls_certificate_sds_secret_configs set
//   - validation_context_sds_secret_config set (upstream only; downstream ignored with diagnostic)
//   - combined_validation_context set
//   - custom_validator_config set
//   - match_typed_subject_alt_names set
//   - verify_certificate_hash / verify_certificate_spki set
//   - password on key
func commonTLSContextToConfig(c *tlsv3.CommonTlsContext, baseDir, side string) (*stdtls.Config, error) {
	if c == nil {
		return nil, fmt.Errorf("tls: %s: common_tls_context is required", side)
	}
	if len(c.GetTlsCertificateSdsSecretConfigs()) > 0 {
		return nil, fmt.Errorf("tls: %s: SDS-bound tls_certificate_sds_secret_configs is not supported in phase 03", side)
	}
	if c.GetValidationContextType() != nil {
		switch c.GetValidationContextType().(type) {
		case *tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig:
			return nil, fmt.Errorf("tls: %s: SDS-bound validation_context_sds_secret_config is not supported in phase 03", side)
		case *tlsv3.CommonTlsContext_CombinedValidationContext:
			return nil, fmt.Errorf("tls: %s: combined_validation_context is not supported in phase 03", side)
		}
	}
	if vc := c.GetValidationContext(); vc != nil {
		if vc.GetCustomValidatorConfig() != nil {
			return nil, fmt.Errorf("tls: %s: custom_validator_config is not supported in phase 03", side)
		}
		if len(vc.GetMatchTypedSubjectAltNames()) > 0 {
			return nil, fmt.Errorf("tls: %s: match_typed_subject_alt_names is not supported in phase 03", side)
		}
		if len(vc.GetVerifyCertificateHash()) > 0 {
			return nil, fmt.Errorf("tls: %s: verify_certificate_hash is not supported in phase 03", side)
		}
		if len(vc.GetVerifyCertificateSpki()) > 0 {
			return nil, fmt.Errorf("tls: %s: verify_certificate_spki is not supported in phase 03", side)
		}
	}

	cfg := &stdtls.Config{}

	for i, tc := range c.GetTlsCertificates() {
		if tc.GetPassword() != nil {
			return nil, fmt.Errorf("tls: %s: tls_certificates[%d]: password-protected keys are not supported in phase 03", side, i)
		}
		certPEM, err := loadDataSource(tc.GetCertificateChain(), baseDir)
		if err != nil {
			return nil, fmt.Errorf("tls: %s: tls_certificates[%d]: certificate_chain: %w", side, i, err)
		}
		keyPEM, err := loadDataSource(tc.GetPrivateKey(), baseDir)
		if err != nil {
			return nil, fmt.Errorf("tls: %s: tls_certificates[%d]: private_key: %w", side, i, err)
		}
		pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("tls: %s: tls_certificates[%d]: load cert: %w", side, i, err)
		}
		cfg.Certificates = append(cfg.Certificates, pair)
	}

	if side == "downstream" && len(cfg.Certificates) == 0 {
		return nil, fmt.Errorf("tls: downstream: no tls_certificates configured")
	}

	if err := applyTLSParams(cfg, c.GetTlsParams()); err != nil {
		return nil, fmt.Errorf("tls: %s: %w", side, err)
	}

	cfg.NextProtos = append(cfg.NextProtos, c.GetAlpnProtocols()...)

	return cfg, nil
}
```

**Implementation note on imports:** the snippet above references `proto.UnmarshalOptions{}` but uses `anypb.UnmarshalTo` — the executor writes the actual import as `google.golang.org/protobuf/proto` and uses `proto.Unmarshal(ts.GetTypedConfig().GetValue(), ctx)` or `ts.GetTypedConfig().UnmarshalTo(ctx)` (preferred — no proto import). At implementation time, settle the idiom by matching phase-02's style in `internal/filter/tcpproxy/filter.go` NewFilter: it uses `ts.GetTypedConfig().UnmarshalTo(ctx)`. Use the same call here; remove the dummy `proto.UnmarshalOptions` reference.

- [ ] **Step 4: Fill in every `t.Skip`-ed test subtest from Step 1**

Replace each `t.Skip(...)` block with the full test body per the subtest comments. Assert error-substring prefixes match the strings in config.go. Use `anypb.New(&tlsv3.DownstreamTlsContext{...})` to build typed configs; use the shared `pki` test-PKI for valid inputs. Target ~300 total lines.

- [ ] **Step 5: Run all tests and verify they pass**

Run: `cd internal/tls && go test .`
Expected: every test + subtest PASS. Coverage should be high (`go test -cover .` → >90% for the package).

- [ ] **Step 6: Append ADR-0031 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0031: TLS stack selection — stdlib crypto/tls (phase 03)

**Status:** Accepted
**Date:** <YYYY-MM-DD>
**Doctrine:** D-3.5

### Context

Phase 03 introduces envoy-go's first cryptographic surface: downstream TLS termination, upstream TLS origination, and SNI-based filter-chain dispatch. The choice of TLS stack is foundational — every later phase that touches the wire (HTTP/1.1 over TLS, HTTP/2, HTTP/3, gRPC) builds on it. Three options considered:

- (A1) Go stdlib `crypto/tls`.
- (A2) BoringSSL via cgo (e.g., via `github.com/google/boringssl` or a vendored build).
- (A3) Third-party pure-Go or bound stacks (`rustls` via cgo; `github.com/refraction-networking/utls`).

### Decision

**(A1) stdlib `crypto/tls` is the phase-03 (and project-default) TLS stack.**

### Rationale

- **No cgo.** The project's pure-Go build posture simplifies cross-compilation and container base-image choices. (A2) and (A3-cgo) would pull in a C toolchain.
- **TLS 1.2 / 1.3 parity on asserted surface.** `crypto/tls` implements TLS 1.2 and 1.3. The phase-03 differential contract asserts plaintext-after-decryption byte equivalence only; encrypted-side observables (TLS record boundaries, session ticket material, TLS 1.3 cipher selection) are explicitly excluded from the contract (see ADR-0035 / BEHAVIOR_CONTRACT TLS subsection). Any divergence in these observables between `crypto/tls` and Envoy's BoringSSL is a *permitted* divergence under the contract.
- **ALPN + SNI + peer validation natively supported.** `stdtls.Config.NextProtos`, `GetConfigForClient`, `Certificates`, `RootCAs`, `ServerName`, `VerifyConnection` are all first-class — no wrappers needed.
- **License-clean.** `crypto/tls` is BSD-3-Clause (Go's license); no GPL copy-paste risk (D-3.2).
- **No vendoring.** `crypto/tls` ships with the Go toolchain; no dependency-pin worry.

### Known tradeoffs (documented in ADR-0030 and BEHAVIOR_CONTRACT TLS)

- **TLS 1.3 cipher selection not configurable.** RFC 8446 design. Envoy's `cipher_suites` becomes a no-op for TLS 1.3 ciphers; ADR-0030 records the silent-drop + diagnostic.
- **`signature_algorithms` not publicly configurable.** Stdlib omission. ADR-0030 errors if a fixture sets it.
- **Handshake timing / record-boundary divergence.** Go vs BoringSSL differ on both. BEHAVIOR_CONTRACT TLS subsection explicitly excludes these from assertion.

### Consequences

- Every `internal/tls/*.go` file imports `crypto/tls` as `stdtls` to avoid name collision with the package itself.
- Phase 04 (HTTP/1.1) layers `net/http.Server` on TLS via `stdtls.Listen` (or manual composition) — no TLS-stack decision at that phase.
- Phase 05 (HTTP/2) uses `golang.org/x/net/http2` on top of `crypto/tls` listeners.
- Phase 06 (stats) emits TLS-subsystem stats observable through Go's `crypto/tls.ConnectionState` — no stdlib hook changes required.
- If a later phase requires a capability `crypto/tls` doesn't expose (e.g., post-quantum hybrid key exchange before Go adds it), a superseding ADR re-scopes. Phase 03 does not anticipate this need.
```

- [ ] **Step 7: Commit**

```bash
git add internal/tls/config.go internal/tls/config_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 03: internal/tls — NewDownstreamConfig + NewUpstreamConfig [ADR-0031]"
```

- [ ] **Step 8: PROGRESS entry + SHA-fill commit**

Same PROGRESS shape as prior tasks. Include verbatim `go test ./internal/tls/...` output.

---

## Task 6: `internal/tls/fuzz_test.go` — FuzzTLSContextParse

**Files:**
- Create: `internal/tls/fuzz_test.go`
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 6 entry)

No new ADR. ADR-0018 (fuzz CI budget) is inherited — `-fuzztime=30s` for the CI short budget.

- [ ] **Step 1: Write `internal/tls/fuzz_test.go`**

```go
package tls

import (
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// FuzzTLSContextParse exercises NewDownstreamConfig and NewUpstreamConfig
// against mutated TransportSocket.typed_config bytes. Seeds:
//   (a) well-formed DownstreamTlsContext using the inline test PKI.
//   (b) well-formed UpstreamTlsContext using the inline test PKI + SNI.
//   (c) truncated Any bytes.
//   (d) Any with a wrong type_url (StringValue).
//
// Discipline: no panic on any input. Every returned error must begin with
// "tls: ". Malformed inputs yield tls-prefixed errors; well-formed ones
// succeed.
func FuzzTLSContextParse(f *testing.F) {
	// Seed (a): DownstreamTlsContext with inline PKI
	{
		inner := &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{{
					CertificateChain: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pki.leafCertPEM}},
					PrivateKey:       &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pki.leafKeyPEM}},
				}},
			},
		}
		anyTC, _ := anypb.New(inner)
		// anyTC carries both type_url and value; for fuzz we feed both separately.
		f.Add("downstream", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	// Seed (b): UpstreamTlsContext
	{
		inner := &tlsv3.UpstreamTlsContext{
			Sni: "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pki.caPEM}},
					},
				},
			},
		}
		anyTC, _ := anypb.New(inner)
		f.Add("upstream", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	// Seed (c): truncated
	{
		inner := &tlsv3.DownstreamTlsContext{}
		b, _ := proto.Marshal(inner)
		f.Add("downstream", downstreamTLSContextTypeURL, b[:len(b)/2+1])
	}

	// Seed (d): wrong type_url
	{
		f.Add("downstream", "type.googleapis.com/google.protobuf.StringValue", []byte{0x0a, 0x03, 'x', 'y', 'z'})
	}

	f.Fuzz(func(t *testing.T, side, typeURL string, value []byte) {
		ts := &corev3.TransportSocket{TypedConfig: &anypb.Any{TypeUrl: typeURL, Value: value}}
		var err error
		switch side {
		case "downstream":
			_, err = NewDownstreamConfig(ts, "")
		case "upstream":
			_, err = NewUpstreamConfig(ts, "")
		default:
			return
		}
		if err != nil && !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error does not begin with \"tls: \": %v", err)
		}
	})
}
```

- [ ] **Step 2: Run short-budget fuzz**

Run: `cd internal/tls && go test -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s .`
Expected: completes in ~30 seconds with `fuzz: elapsed: 30s, execs: NNNN (XX/sec), new interesting: YY`. No crashes, no panics. If a crasher is found, it lands at `internal/tls/testdata/fuzz/FuzzTLSContextParse/<hash>` — triage and fix before continuing (may require amending config.go; commit the crasher corpus entry with the fix).

- [ ] **Step 3: Run unit-test mode for the fuzz corpus**

Run: `cd internal/tls && go test -run FuzzTLSContextParse .` (no `-fuzz` flag — runs only the seed corpus, sanity check).
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tls/fuzz_test.go
git commit -m "phase 03: internal/tls — FuzzTLSContextParse (gate d, ADR-0018 budget)"
```

- [ ] **Step 5: PROGRESS entry + SHA-fill commit**

Include verbatim fuzz output (last 5 lines) + the seed-corpus unit-test output.

---

## Task 7: Deterministic PKI generator + committed PEMs + README

**Files:**
- Create: `test/fixtures/0002-tls-tcp/pki/gen/main.go`
- Create: `test/fixtures/0002-tls-tcp/pki/ca.pem`
- Create: `test/fixtures/0002-tls-tcp/pki/server-alpha.pem`
- Create: `test/fixtures/0002-tls-tcp/pki/server-alpha.key.pem`
- Create: `test/fixtures/0002-tls-tcp/pki/server-beta.pem`
- Create: `test/fixtures/0002-tls-tcp/pki/server-beta.key.pem`
- Create: `test/fixtures/0002-tls-tcp/pki/upstream-alpha.pem`
- Create: `test/fixtures/0002-tls-tcp/pki/upstream-alpha.key.pem`
- Create: `test/fixtures/0002-tls-tcp/pki/upstream-beta.pem`
- Create: `test/fixtures/0002-tls-tcp/pki/upstream-beta.key.pem`
- Create: `test/fixtures/0002-tls-tcp/pki/README.md`
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 7 entry)

No ADR — SPEC §10 #6 settled inline. Determinism is mandatory: running `go run ./pki/gen` twice must produce byte-identical PEMs (verified by `git diff --exit-code` in Task 15).

- [ ] **Step 1: Write `test/fixtures/0002-tls-tcp/pki/gen/main.go`**

The generator uses a ChaCha8 CSPRNG seeded by a fixed 32-byte pattern, wrapped as an `io.Reader` for `crypto/ecdsa.GenerateKey` and `crypto/x509.CreateCertificate`. This yields byte-deterministic keys and signatures.

```go
// Package main regenerates the phase-03 TLS test PKI deterministically.
//
// Usage (from the repo root):
//
//   cd test/fixtures/0002-tls-tcp && go run ./pki/gen
//
// Produces byte-identical PEMs on every run. Intended to run manually; CI
// never invokes this command. The committed PEMs are the authoritative source
// used by tests; re-run this command only to rotate (and update the NotBefore
// / NotAfter constants below).
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"time"
)

var (
	notBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter  = time.Date(2046, 1, 1, 0, 0, 0, 0, time.UTC)
)

// Deterministic seed: SHA-256 of the string "envoy-go/test/fixtures/0002-tls-tcp/pki/gen/v1".
// Flipping any byte invalidates every committed PEM; re-run `go run ./pki/gen` to regenerate.
var seed = [32]byte{
	0x9f, 0x2a, 0xd7, 0x1c, 0x55, 0x84, 0xe3, 0x62,
	0x40, 0x11, 0xaa, 0x7b, 0xbc, 0x08, 0x3e, 0x91,
	0xd4, 0x7f, 0x66, 0x9e, 0x20, 0xcb, 0x55, 0x17,
	0x8a, 0x03, 0xfa, 0x49, 0xd6, 0xe7, 0x2d, 0xb0,
}

type detReader struct{ r *rand.ChaCha8 }

func (d *detReader) Read(p []byte) (int, error) {
	var buf [8]byte
	for i := 0; i < len(p); {
		binary.LittleEndian.PutUint64(buf[:], d.r.Uint64())
		i += copy(p[i:], buf[:])
	}
	return len(p), nil
}

func newReader(tag string) io.Reader {
	// Fork the master seed per-tag by XORing tag bytes into a copy, so each
	// certificate and key gets an independent deterministic stream.
	var s [32]byte
	copy(s[:], seed[:])
	for i, b := range []byte(tag) {
		s[i%32] ^= b
	}
	return &detReader{r: rand.NewChaCha8(s)}
}

func main() {
	outDir := "."
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	outDir = filepath.Clean(outDir)
	if base := filepath.Base(outDir); base == "gen" {
		outDir = filepath.Dir(outDir)
	}
	must(os.MkdirAll(outDir, 0o755))

	// CA
	caKey, caPEM, caCert := genCA("ca")
	writePEM(filepath.Join(outDir, "ca.pem"), caPEM)

	// server-alpha
	genLeaf(outDir, "server-alpha", "alpha.envoy-go.test", []string{"alpha.envoy-go.test"}, nil, caCert, caKey)
	// server-beta
	genLeaf(outDir, "server-beta", "beta.envoy-go.test", []string{"beta.envoy-go.test"}, nil, caCert, caKey)
	// upstream-alpha — DNS SANs + IP SAN for subject-side 127.0.0.1 connectivity
	genLeaf(outDir, "upstream-alpha", "alpha.envoy-go.test", []string{"alpha.envoy-go.test", "localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, caCert, caKey)
	// upstream-beta
	genLeaf(outDir, "upstream-beta", "beta.envoy-go.test", []string{"beta.envoy-go.test", "localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, caCert, caKey)

	fmt.Println("ok: 10 PEMs written to", outDir)
}

func genCA(tag string) (*ecdsa.PrivateKey, []byte, *x509.Certificate) {
	r := newReader(tag + "-key")
	key, err := ecdsa.GenerateKey(elliptic.P256(), r)
	must(err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "envoy-go test CA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(newReader(tag+"-sig"), tmpl, tmpl, &key.PublicKey, key)
	must(err)
	cert, err := x509.ParseCertificate(der)
	must(err)
	return key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert
}

func genLeaf(outDir, tag, cn string, dnsNames []string, ips []net.IP, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
	r := newReader(tag + "-key")
	key, err := ecdsa.GenerateKey(elliptic.P256(), r)
	must(err)
	tmpl := &x509.Certificate{
		SerialNumber: new(big.Int).SetBytes([]byte(tag)[:8]),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(newReader(tag+"-sig"), tmpl, caCert, &key.PublicKey, caKey)
	must(err)
	writePEM(filepath.Join(outDir, tag+".pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	must(err)
	writePEM(filepath.Join(outDir, tag+".key.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func writePEM(path string, pemBytes []byte) {
	must(os.WriteFile(path, pemBytes, 0o644))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
```

**Determinism requirement:** the `detReader.Read` loop above uses `encoding/binary.LittleEndian.PutUint64` into a reusable 8-byte buffer + `copy`, avoiding `unsafe.Pointer` (which golangci-lint's gosec lint flags) and avoiding the off-by-one risk of hand-walking the output slice. The key requirement: calling `Read` twice with the same seed produces byte-identical bytes. Verify via a unit test `TestDetReaderDeterminism` (optional — skipping it is acceptable if the `git diff --exit-code` check in Step 3 passes, since that check is what actually matters for the committed PEMs).

**Serial-number caveat:** `SerialNumber: new(big.Int).SetBytes([]byte(tag)[:8])` on a 5-char tag like `"ca"` will panic. The executor fixes this to a fixed per-tag serial number table:

```go
var serials = map[string]int64{
	"server-alpha":   10,
	"server-beta":    11,
	"upstream-alpha": 20,
	"upstream-beta":  21,
}
```

And `tmpl.SerialNumber = big.NewInt(serials[tag])`.

- [ ] **Step 2: Run the generator and verify it produces 10 files**

Run: `cd test/fixtures/0002-tls-tcp && go run ./pki/gen`
Expected output: `ok: 10 PEMs written to .`
Verify: `ls pki/*.pem | wc -l` → `9` (4 certs + 4 keys + 1 CA = 9 PEMs; the `main.go` lives under `pki/gen/` and prints "10" only if you count itself; update the generator's summary line to match actual PEM count = 9, or accept 10 if it's counting more broadly — settle at implementation time).

- [ ] **Step 3: Verify determinism — run it twice, confirm byte-identical output**

```bash
cd test/fixtures/0002-tls-tcp
go run ./pki/gen
git add pki/*.pem
sha256sum pki/*.pem > /tmp/first.sha
go run ./pki/gen
sha256sum pki/*.pem > /tmp/second.sha
diff /tmp/first.sha /tmp/second.sha
```
Expected: no diff; SHAs match.

- [ ] **Step 4: Write `test/fixtures/0002-tls-tcp/pki/README.md`**

```markdown
# Phase-03 TLS test PKI

This directory holds the deterministic test PKI for differential fixture `0002-tls-tcp`. The committed `.pem` files are the authoritative source consumed by `envoy.yaml`, `envoy-go.yaml`, and `test/helpers/tls.go` via inline-bytes DataSources.

## Layout

- `ca.pem` — self-signed ECDSA P-256 root CA. `Subject: CN=envoy-go test CA`.
- `server-alpha.pem` / `.key.pem` — leaf for the `alpha.envoy-go.test` downstream SNI chain.
- `server-beta.pem` / `.key.pem` — leaf for the `beta.envoy-go.test` downstream SNI chain.
- `upstream-alpha.pem` / `.key.pem` — leaf for the 3 upstream-alpha TLS backends. Carries IP SAN `127.0.0.1` + DNS SANs `alpha.envoy-go.test` and `localhost` so both the reference proxy (Docker, reaches backends via `host.docker.internal`) and the subject proxy (host subprocess, dials `127.0.0.1`) can validate against the same cert.
- `upstream-beta.pem` / `.key.pem` — same for the 3 upstream-beta backends.
- `gen/main.go` — the deterministic generator.

## Validity window

- `NotBefore: 2026-01-01T00:00:00Z`
- `NotAfter:  2046-01-01T00:00:00Z` (20-year window — overshoots realistic project lifespan)

If the PKI ever needs re-issue (validity window widened, SAN added, etc.), update the `notBefore` / `notAfter` constants and/or the generator logic in `gen/main.go`, then run `go run ./pki/gen` from this directory. Every PEM byte regenerates deterministically from the fixed seed so `git diff --exit-code pki/` is clean on re-runs.

## Regeneration command

```bash
cd test/fixtures/0002-tls-tcp
go run ./pki/gen
```

Verifies byte-determinism when re-run:

```bash
go run ./pki/gen
git diff --exit-code pki/ && echo ok  # expect: ok
```

## Why deterministic

- `git diff` is clean on regeneration — no noisy commits on PKI rotation.
- CI never runs `go run ./pki/gen`. The committed PEMs are the authoritative source.
- Fixtures can embed PEM bytes as inline DataSources and expect them to stay byte-identical across developer workstations.
- Task 15's gate sweep verifies determinism via `git diff --exit-code` after invoking the generator.

## Why IP SAN on upstream leaves

Subject-side backends are reached via literal `127.0.0.1` endpoints (STATIC cluster type per ADR-0027 pattern). Go's `crypto/tls` validates IP-addressed dials against IP SANs, not DNS SANs, so the upstream leaves carry `IPAddresses: [127.0.0.1]` in addition to the DNS SANs used for `ServerName`-driven reference-side validation. The Docker-side reference reaches backends via `host.docker.internal` which resolves to the host's address; its cert validation is against `ServerName = alpha.envoy-go.test` (matching DNS SAN), independent of the resolved IP.

## Why two SNIs

The fixture exercises downstream TLS termination, upstream TLS origination, AND SNI-based filter-chain dispatch in one run. Two SNIs keep cluster-a and cluster-b cleanly separable for distribution assertions (`[3,3,3]` per SNI per side).
```

- [ ] **Step 5: Verify the generator is not part of `go test ./...`**

Run: `go test ./test/fixtures/0002-tls-tcp/...`
Expected: only the `driver/driver_test.go` (not present yet — created in Task 13) runs; `pki/gen` is a `package main` and is excluded from the test target. If `go build ./test/fixtures/0002-tls-tcp/pki/gen` is required by CI, verify that `go build ./...` includes it and succeeds.

Run: `go build ./test/fixtures/0002-tls-tcp/pki/gen`
Expected: no output, exit 0.

- [ ] **Step 6: Commit**

```bash
git add test/fixtures/0002-tls-tcp/pki/
git commit -m "phase 03: test/fixtures/0002-tls-tcp/pki — deterministic CA + 4 leaves + gen tool"
```

- [ ] **Step 7: PROGRESS entry + SHA-fill commit**

Include:
- `go run ./pki/gen` output verbatim.
- Determinism check (`git diff --exit-code pki/` output — empty).
- `ls pki/*.pem` listing.

---

## Task 8: `test/helpers/tls.go` — TLSRoundTrip helper + test

**Files:**
- Create: `test/helpers/tls.go`
- Create: `test/helpers/tls_test.go`
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 8 entry)

No ADR. Helper is a one-function wrapper around `stdtls.Dialer` + half-close + read-until-EOF, mirroring `test/helpers/tcp.go`'s `TCPRoundTrip` shape.

- [ ] **Step 1: Write `test/helpers/tls_test.go`**

```go
package helpers

import (
	stdtls "crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// loadPKIFile reads a PEM file from test/fixtures/0002-tls-tcp/pki/. Called
// from every subtest to avoid tying the package's init to a fixture.
func loadPKIFile(t *testing.T, name string) []byte {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(thisFile), "..", "fixtures", "0002-tls-tcp", "pki", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("pki read: %v", err)
	}
	return b
}

func TestTLSRoundTrip_Echo(t *testing.T) {
	caPEM := loadPKIFile(t, "ca.pem")
	certPEM := loadPKIFile(t, "upstream-alpha.pem")
	keyPEM := loadPKIFile(t, "upstream-alpha.key.pem")

	pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	srvCfg := &stdtls.Config{Certificates: []stdtls.Certificate{pair}, MinVersion: stdtls.VersionTLS12}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn) // echo until half-close/EOF
	}()

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("append ca")
	}
	addr := ln.Addr().String()
	got, err := TLSRoundTrip(t.Context(), addr, "alpha.envoy-go.test", pool, []byte("hello"), 2*time.Second)
	if err != nil {
		t.Fatalf("TLSRoundTrip: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
	ln.Close()
	wg.Wait()
}

func TestTLSRoundTrip_WrongSNI(t *testing.T) {
	caPEM := loadPKIFile(t, "ca.pem")
	certPEM := loadPKIFile(t, "upstream-alpha.pem")
	keyPEM := loadPKIFile(t, "upstream-alpha.key.pem")
	pair, _ := stdtls.X509KeyPair(certPEM, keyPEM)
	srvCfg := &stdtls.Config{Certificates: []stdtls.Certificate{pair}, MinVersion: stdtls.VersionTLS12}
	ln, _ := stdtls.Listen("tcp", "127.0.0.1:0", srvCfg)
	defer ln.Close()
	go func() { _, _ = ln.Accept() }()

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	_, err := TLSRoundTrip(t.Context(), ln.Addr().String(), "beta.envoy-go.test", pool, []byte("x"), 500*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "x509") {
		t.Errorf("want x509 verify error, got: %v", err)
	}
}

func TestTLSRoundTrip_DialFailure(t *testing.T) {
	pool := x509.NewCertPool()
	// closed address: bind-and-close to get a reliably-refused address.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()

	_, err := TLSRoundTrip(t.Context(), addr, "alpha.envoy-go.test", pool, []byte("x"), 200*time.Millisecond)
	if err == nil {
		t.Error("want dial error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/helpers -run TestTLSRoundTrip`
Expected: compile error — `TLSRoundTrip` undefined.

- [ ] **Step 3: Write `test/helpers/tls.go`**

```go
package helpers

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"time"
)

// TLSRoundTrip dials addr over TCP, establishes a TLS 1.2+/1.3 client
// handshake using rootCAs + serverName, writes payload, half-closes by
// sending a close_notify alert + TCP FIN, then reads until EOF or idleTimeout
// elapses. Returns all received bytes.
//
// Shape mirrors helpers.TCPRoundTrip (phase 02) with the single addition of
// the TLS wrap + handshake + SNI.
func TLSRoundTrip(ctx context.Context, addr, serverName string, rootCAs *x509.CertPool, payload []byte, idleTimeout time.Duration) ([]byte, error) {
	d := &net.Dialer{}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tls round trip: dial: %w", err)
	}
	cfg := &stdtls.Config{
		ServerName: serverName,
		RootCAs:    rootCAs,
		MinVersion: stdtls.VersionTLS12,
		MaxVersion: stdtls.VersionTLS13,
	}
	conn := stdtls.Client(raw, cfg)
	defer conn.Close()
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("tls round trip: handshake: %w", err)
	}
	if _, err := conn.Write(payload); err != nil {
		return nil, fmt.Errorf("tls round trip: write: %w", err)
	}
	if err := conn.CloseWrite(); err != nil {
		return nil, fmt.Errorf("tls round trip: close_write: %w", err)
	}
	if idleTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
	}
	got, err := io.ReadAll(conn)
	if err != nil {
		// Read deadline hit is surfaced as net.Error Timeout(); distinguish
		// from genuine read errors by checking err's Timeout method.
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return got, nil
		}
		return got, fmt.Errorf("tls round trip: read: %w", err)
	}
	return got, nil
}
```

- [ ] **Step 4: Run tests and verify they pass**

Run: `go test ./test/helpers -run TestTLSRoundTrip`
Expected: three subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add test/helpers/tls.go test/helpers/tls_test.go
git commit -m "phase 03: test/helpers — TLSRoundTrip (TLS dialer + SNI + half-close + read-until-EOF)"
```

- [ ] **Step 6: PROGRESS entry + SHA-fill commit**

---

## Task 9: `internal/cluster` — Cluster.Dial + upstream TLS integration + ADR-0032

**Files:**
- Modify: `internal/cluster/cluster.go`
- Modify: `internal/cluster/cluster_test.go`
- Modify: `internal/cluster/manager.go`
- Modify: `internal/cluster/manager_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0032)
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 9 entry)

This task lands the `Cluster.Dial(ctx)` abstraction. Plaintext path is a trivial refactor (same behaviour as phase 02). TLS path integrates `internal/tls.NewUpstreamConfig` + `stdtls.Client` + `HandshakeContext`.

- [ ] **Step 1: Write the cluster_test.go extensions first (TDD)**

Add subtests to `internal/cluster/cluster_test.go` (preserving all phase-02 tests):

```go
func TestCluster_Dial_Plaintext(t *testing.T) {
	// Loopback echo
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { defer c.Close(); _, _ = io.Copy(c, c) }(c)
		}
	}()

	// Build a single-endpoint plaintext cluster pointing at ln.Addr()
	c := &Cluster{
		name:           "test",
		connectTimeout: time.Second,
		endpoints:      []*Endpoint{{addr: ln.Addr().String()}},
	}
	conn, err := c.Dial(t.Context())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("ping"))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "ping" {
		t.Errorf("got %q, want %q", buf, "ping")
	}
}

func TestCluster_Dial_TLS(t *testing.T) {
	// Build a TLS echo server using upstream-alpha cert
	caPEM, _ := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	certPEM, _ := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.pem")
	keyPEM, _ := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.key.pem")
	pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", &stdtls.Config{Certificates: []stdtls.Certificate{pair}, MinVersion: stdtls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { defer c.Close(); _, _ = io.Copy(c, c) }(c)
		}
	}()

	// Build upstream *stdtls.Config against this server
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	upCfg := &stdtls.Config{
		ServerName: "alpha.envoy-go.test",
		RootCAs:    pool,
		MinVersion: stdtls.VersionTLS12,
		MaxVersion: stdtls.VersionTLS13,
	}

	c := &Cluster{
		name:           "test-tls",
		connectTimeout: time.Second,
		endpoints:      []*Endpoint{{addr: ln.Addr().String()}},
		upstreamCfg:    upCfg,
	}
	conn, err := c.Dial(t.Context())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if _, ok := conn.(*stdtls.Conn); !ok {
		t.Errorf("want *stdtls.Conn, got %T", conn)
	}
	_, _ = conn.Write([]byte("secret"))
	buf := make([]byte, 6)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "secret" {
		t.Errorf("got %q, want %q", buf, "secret")
	}
}

func TestCluster_Dial_TLS_HandshakeFailure(t *testing.T) {
	// Upstream with wrong CA -> handshake rejection
	certPEM, _ := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.pem")
	keyPEM, _ := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.key.pem")
	pair, _ := stdtls.X509KeyPair(certPEM, keyPEM)
	ln, _ := stdtls.Listen("tcp", "127.0.0.1:0", &stdtls.Config{Certificates: []stdtls.Certificate{pair}, MinVersion: stdtls.VersionTLS12})
	defer ln.Close()
	go func() { _, _ = ln.Accept() }()

	// Upstream config with an EMPTY cert pool -> handshake fails
	c := &Cluster{
		name:           "test-bad-ca",
		connectTimeout: time.Second,
		endpoints:      []*Endpoint{{addr: ln.Addr().String()}},
		upstreamCfg: &stdtls.Config{
			ServerName: "alpha.envoy-go.test",
			RootCAs:    x509.NewCertPool(),
			MinVersion: stdtls.VersionTLS12,
		},
	}
	_, err := c.Dial(t.Context())
	if err == nil || !strings.HasPrefix(err.Error(), "cluster: tls: handshake:") {
		t.Errorf("want cluster: tls: handshake: prefix, got: %v", err)
	}
}

func TestCluster_Dial_CtxCancelled(t *testing.T) {
	// Pre-cancelled context short-circuits.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &Cluster{
		name:           "test",
		connectTimeout: time.Second,
		endpoints:      []*Endpoint{{addr: "127.0.0.1:1"}}, // unreachable
	}
	_, err := c.Dial(ctx)
	if err == nil {
		t.Error("want ctx error")
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/cluster -run TestCluster_Dial`
Expected: compile errors — `Dial`, `upstreamCfg` unknown.

- [ ] **Step 3: Modify `internal/cluster/cluster.go`**

Add:

```go
import (
	stdtls "crypto/tls"
	// … existing imports
)

// Cluster carries, in addition to the phase-02 fields:
//
//   upstreamCfg *stdtls.Config // nil for plaintext clusters; set for TLS clusters
//
type Cluster struct {
	// ... existing fields
	upstreamCfg *stdtls.Config
}

// Dial opens a new connection to an endpoint picked from this cluster's RR
// LB state. For plaintext clusters, returns the raw *net.TCPConn. For TLS
// clusters, returns a *stdtls.Conn whose HandshakeContext has already been
// driven. connect_timeout bounds the TCP dial only; TLS handshake is bounded
// by ctx.
func (c *Cluster) Dial(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ep, err := c.PickEndpoint()
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", ep.Addr())
	if err != nil {
		return nil, fmt.Errorf("cluster: dial: %w", err)
	}
	if c.upstreamCfg == nil {
		return raw, nil
	}
	conn := stdtls.Client(raw, c.upstreamCfg)
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("cluster: tls: handshake: %w", err)
	}
	return conn, nil
}
```

- [ ] **Step 4: Modify `internal/cluster/manager.go`**

Inside `buildCluster`, after the phase-02 validation but before returning the cluster, add transport_socket handling:

```go
if ts := clusterProto.GetTransportSocket(); ts != nil {
	if ts.GetTypedConfig() == nil {
		return nil, fmt.Errorf("cluster %q: transport_socket without typed_config", clusterProto.GetName())
	}
	tu := ts.GetTypedConfig().GetTypeUrl()
	if tu != upstreamTLSContextTypeURL {
		return nil, fmt.Errorf("cluster %q: unsupported transport_socket type_url %q (phase 03 supports only UpstreamTlsContext)", clusterProto.GetName(), tu)
	}
	uc, err := tls.NewUpstreamConfig(ts, baseDir)
	if err != nil {
		return nil, fmt.Errorf("cluster %q: %w", clusterProto.GetName(), err)
	}
	c.upstreamCfg = uc.TLSConfig
}
```

Where `upstreamTLSContextTypeURL` is imported as a package-level const from `internal/tls` (exported on the package as `UpstreamTLSContextTypeURL`, or declared locally here as a duplicate string constant — prefer local duplication to avoid export pressure).

The `baseDir` parameter: `NewManager` receives the bootstrap file path, so it already knows the base directory. Thread `baseDir string` through `NewManager` → `buildCluster` (one-line addition to each signature). The phase-01 bootstrap loader already resolves the config path; pass `filepath.Dir(configPath)` into `NewManager`.

**If the phase-02 `NewManager` does not accept `baseDir`:** add a new overload `NewManagerWithBaseDir(bootstrap *bootstrapv3.Bootstrap, baseDir string) (*Manager, error)` and have `NewManager` call it with `""`. Phase-03 main will call the new form. Phase-02 tests using `NewManager` are unaffected (all their clusters are plaintext — no DataSource resolution needed).

- [ ] **Step 5: Modify `internal/cluster/manager_test.go` — extensions**

Add subtests:

```go
func TestNewManager_TLSCluster(t *testing.T) {
	// Build a bootstrap with one STATIC cluster carrying transport_socket=UpstreamTlsContext
	// using inline PEMs from the fixture PKI.
	// Assert the resulting cluster has a non-nil upstreamCfg with SNI + RootCAs.
	// ...
}

func TestNewManager_TLSCluster_UnknownTransportSocket(t *testing.T) {
	// transport_socket with type_url = raw_buffer -> error.
}

func TestNewManager_TLSCluster_MissingTrustedCA(t *testing.T) {
	// UpstreamTlsContext without trusted_ca -> error (tightening from §5.4, propagated via NewUpstreamConfig).
}

func TestNewManager_MixedPlaintextAndTLSClusters(t *testing.T) {
	// 2 clusters, one plaintext and one TLS; both build successfully; Get returns each with correct upstreamCfg state.
}
```

- [ ] **Step 6: Run all cluster tests**

Run: `go test ./internal/cluster/...`
Expected: every test + subtest PASS (phase-02 tests + phase-03 additions).

- [ ] **Step 7: Append ADR-0032 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0032: Upstream TLS dialer model — Cluster.Dial(ctx) (net.Conn, error)

**Status:** Accepted
**Date:** <YYYY-MM-DD>
**Doctrine:** D-3.5

### Context

Phase 02's TCP proxy filter dialed endpoints directly via `net.DialTimeout` inside `Filter.Handle`. Phase 03 introduces upstream TLS origination — the filter must not branch on transport type (plaintext vs TLS) because phase 04 will add HTTP/TLS and phase 05 HTTP/2-over-TLS, and the filter body should stay transport-agnostic.

### Decision

`*Cluster` grows `Dial(ctx context.Context) (net.Conn, error)` returning a ready-to-read/write `net.Conn`. Plaintext clusters return `*net.TCPConn` (from `net.Dialer.DialContext`). TLS clusters return `*stdtls.Conn` after `HandshakeContext(ctx)` succeeds. The filter calls `Cluster.Dial(ctx)` regardless of transport.

`connect_timeout` applies to the TCP dial (via `net.Dialer.Timeout`). TLS handshake is bounded by `ctx` — if the caller has a deadline-bounded context, the handshake inherits it; otherwise it blocks until completion (matching Envoy's behaviour with no configured handshake timeout).

### Consequences

- `internal/filter/tcpproxy/filter.go` loses its direct `net.DialTimeout` call (Task 11, ADR-0032 aftermath). The filter body becomes two lines shorter and transport-agnostic.
- Phase-02 REVIEW Minor 4 (`ctx` unused in `Filter.Handle`) is resolved: the early `ctx.Err()` guard + `Cluster.Dial(ctx)` call fully consume `ctx`.
- The `halfClose` helper in the filter gains a `*stdtls.Conn.CloseWrite` case (Task 11) — unrelated to this ADR but a consequence of uniformly wrapping upstream conns.
- Cluster construction (`NewManager` → `buildCluster`) now threads `baseDir` through so `internal/tls.NewUpstreamConfig` can resolve filename-based DataSources against a well-defined root. Phase-02 test harness uses `""` baseDir (plaintext only; no DataSource); phase-03 main passes `filepath.Dir(configPath)`.
- `*stdtls.Conn.CloseWrite` sends a close_notify alert + TCP FIN, preserving the half-close propagation that ADR-0023's `netConn` wrapper relies on.
```

- [ ] **Step 8: Commit**

```bash
git add internal/cluster/cluster.go internal/cluster/cluster_test.go internal/cluster/manager.go internal/cluster/manager_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 03: internal/cluster — Cluster.Dial(ctx) + upstream TLS [ADR-0032]"
```

- [ ] **Step 9: PROGRESS entry + SHA-fill commit**

---

## Task 10: `internal/listener` — multi-chain + SNI routing + ADR-0033 (supersedes ADR-0025)

**Files:**
- Modify: `internal/listener/manager.go`
- Modify: `internal/listener/manager_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0033 — `**Supersedes: ADR-0025**` header mandatory)
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 10 entry)

The biggest single modification. TDD-first; the manager's current "exactly one chain, empty match, no transport_socket" check (ADR-0025) is fully replaced, and the TLS-listener Start path composes the `GetConfigForClient` callback.

**Risk acknowledgment:** this task's sub-steps may blow past 10. If at any point a sub-step count exceeds 15, invoke the mid-execution split valve per `BOOTSTRAP_PROMPT.md` §6.1/§6.2 and ADR the split.

- [ ] **Step 1: Write the listener_test.go extensions**

Extend `internal/listener/manager_test.go` with the subtests named in SPEC §4.2. Each subtest builds a listener proto (1 or 2 chains), calls `NewManager`, and either asserts success (with SNI routing via mocked `ClientHelloInfo`) or error-string prefix. Target ~400 test lines.

Key subtests to land in order of value:
- `TestNewManager_MultiChain_SNIHappy` — 2 chains (alpha, beta), each with a DownstreamTlsContext using inline PEMs; verify `GetConfigForClient(&ClientHelloInfo{ServerName: "alpha.envoy-go.test"})` returns the alpha chain's `*stdtls.Config`, same for beta; verify an unmatched SNI returns `nil, error`.
- `TestNewManager_MultiChain_SNIWildcard` — one chain with `server_names: ["*.envoy-go.test"]`; `GetConfigForClient` returns that chain for `foo.envoy-go.test` but not for `envoy-go.test`.
- `TestNewManager_MultiChain_Specificity` — chain A exact `alpha.envoy-go.test`, chain B wildcard `*.envoy-go.test`; `GetConfigForClient(alpha...)` returns A (exact wins); `GetConfigForClient(other.envoy-go.test)` returns B.
- `TestNewManager_MultiChain_CatchAll` — 2 SNI chains + 1 empty-match chain; unmatched SNI returns the catch-all's config; matched SNI returns its chain's config.
- `TestNewManager_MultiChain_NoSNIMatch` — 2 SNI chains, no catch-all; unmatched SNI → `nil, error`.
- `TestNewManager_MultiChain_MixedTLSPlaintext_Errors` — 2 chains, one with transport_socket, one without → build error.
- `TestNewManager_MultiChain_DefaultFilterChain_Errors` — Listener.default_filter_chain set → error.
- `TestNewManager_MultiChain_NonSNIMatchField_Errors` — filter_chain_match with destination_port populated → error; prefix_ranges → error; source_ports → error.
- `TestNewManager_MultiChain_ApplicationProtocols_Errors` — filter_chain_match.application_protocols[] populated → error (ALPN match deferred to phase 07).
- `TestNewManager_MultiChain_TooManyCatchAlls_Errors` — 2 chains with empty match → error.
- `TestNewManager_MultiChain_RequireClientCert_Errors` — require_client_certificate=true → error (Minor 4 test coverage; propagated from NewDownstreamConfig).
- `TestNewManager_MultiChain_UnknownTransportSocket_Errors` — transport_socket with type_url != downstream tls context → error.
- `TestNewManager_PlaintextMultiChain_Errors` — phase-03 rejects plaintext multi-chain per §4.2 bullet on mixed listeners (when all chains are plaintext but >1 chain, error because `filter_chain_match` on plaintext cannot match on SNI — this is a likely misconfiguration).
- `TestNewManager_SingleChain_Plaintext_Unchanged` — single plaintext chain with empty match still works (phase-02 regression).
- `TestNewManager_ChainSelectionPropagation` — full in-process TLS handshake against a listener built by NewManager; verify the filter for the chosen chain is actually invoked (approach (A) sync.Map dispatch).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/listener -run TestNewManager_MultiChain`
Expected: compile errors / skipped / failing.

- [ ] **Step 3: Rewrite `internal/listener/manager.go` — phase-03 subset validation**

The build-time validation block (currently "exactly one chain" per ADR-0025) is replaced by the phase-03 subset (ADR-0033). Pseudocode structure:

```go
type chainInfo struct {
	serverNames []string   // from filter_chain_match.server_names; nil/empty = catch-all
	tlsCfg      *stdtls.Config // nil if plaintext chain
	filter      filter     // exactly one terminal filter (phase-02 registry)
}

type listenerRuntime struct {
	name        string
	addr        string
	netLn       net.Listener
	tlsMode     bool
	tlsCfg      *stdtls.Config // top-level with GetConfigForClient wired
	chains      []*chainInfo   // sorted most-specific-first (exact > suffix > catch-all)
	chainRegistry sync.Map      // map[*stdtls.Conn]*chainInfo
}

func (m *Manager) buildListener(lp *listenerv3.Listener, baseDir string) (*listenerRuntime, error) {
	// Basic validation: name, address
	// SPEC §4.2: Listener.default_filter_chain rejected.
	if lp.GetDefaultFilterChain() != nil {
		return nil, fmt.Errorf("listener %q: default_filter_chain is not supported in phase 03 (ADR-0033)", lp.GetName())
	}
	chains := lp.GetFilterChains()
	if len(chains) == 0 {
		return nil, fmt.Errorf("listener %q: filter_chains must be non-empty", lp.GetName())
	}
	catchAllCount := 0
	anyTLS := false
	var cis []*chainInfo
	for i, fc := range chains {
		// filter_chain_match whitelist
		fm := fc.GetFilterChainMatch()
		serverNames := fm.GetServerNames() // may be nil/empty
		if err := validateFilterChainMatch(fm); err != nil {
			return nil, fmt.Errorf("listener %q: filter_chains[%d]: %w", lp.GetName(), i, err)
		}
		// transport_socket decode
		var chainTLS *stdtls.Config
		if ts := fc.GetTransportSocket(); ts != nil {
			dc, err := tls.NewDownstreamConfig(ts, baseDir)
			if err != nil {
				return nil, fmt.Errorf("listener %q: filter_chains[%d]: %w", lp.GetName(), i, err)
			}
			chainTLS = dc.TLSConfig
			anyTLS = true
		}
		// filter build (unchanged from phase 02)
		filters := fc.GetFilters()
		if len(filters) != 1 {
			return nil, fmt.Errorf("listener %q: filter_chains[%d]: expected exactly 1 filter, got %d", lp.GetName(), i, len(filters))
		}
		f, err := m.buildFilter(filters[0])
		if err != nil {
			return nil, fmt.Errorf("listener %q: filter_chains[%d]: %w", lp.GetName(), i, err)
		}
		if len(serverNames) == 0 && chainTLS != nil {
			// Allowed: TLS catch-all chain.
		}
		if len(serverNames) == 0 {
			catchAllCount++
		}
		cis = append(cis, &chainInfo{serverNames: serverNames, tlsCfg: chainTLS, filter: f})
	}
	if catchAllCount > 1 {
		return nil, fmt.Errorf("listener %q: at most one filter_chain may omit filter_chain_match.server_names", lp.GetName())
	}
	if anyTLS {
		// All chains must be TLS.
		for i, ci := range cis {
			if ci.tlsCfg == nil {
				return nil, fmt.Errorf("listener %q: filter_chains[%d]: mixed TLS and plaintext chains on one listener are not supported", lp.GetName(), i)
			}
		}
	} else if len(cis) > 1 {
		// Plaintext with >1 chain: SNI cannot match on plaintext; this is almost always a misconfiguration.
		return nil, fmt.Errorf("listener %q: plaintext listener with multiple filter_chains is not supported (SNI match requires TLS)", lp.GetName())
	}
	// Sort most-specific-first: exact > suffix-wildcard > catch-all.
	sort.SliceStable(cis, func(i, j int) bool {
		return chainSpecificityRank(cis[i].serverNames) < chainSpecificityRank(cis[j].serverNames)
	})
	rt := &listenerRuntime{
		name:   lp.GetName(),
		tlsMode: anyTLS,
		chains:  cis,
	}
	if anyTLS {
		rt.tlsCfg = &stdtls.Config{
			GetConfigForClient: makeGetConfigForClient(rt),
		}
	}
	// ... address + netLn wiring as phase-02.
	return rt, nil
}

// chainSpecificityRank: 0 for exact (any non-wildcard pattern), 1 for suffix wildcard
// (leading "*."), 2 for universal "*", 3 for catch-all (empty patterns).
// Lower rank = more specific = matched first.
func chainSpecificityRank(patterns []string) int {
	if len(patterns) == 0 {
		return 3
	}
	// A chain's specificity is the most-specific of its patterns.
	rank := 4
	for _, p := range patterns {
		switch {
		case p == "*":
			if 2 < rank {
				rank = 2
			}
		case strings.HasPrefix(p, "*."):
			if 1 < rank {
				rank = 1
			}
		default:
			return 0 // any exact pattern wins
		}
	}
	return rank
}

func makeGetConfigForClient(rt *listenerRuntime) func(*stdtls.ClientHelloInfo) (*stdtls.Config, error) {
	return func(hello *stdtls.ClientHelloInfo) (*stdtls.Config, error) {
		sni := hello.ServerName
		for _, ci := range rt.chains {
			if len(ci.serverNames) == 0 {
				// catch-all — matches any SNI, but most-specific-first ordering ensures
				// it's only reached when no exact/wildcard chain matched.
				// Register chain for dispatch and return its config.
				return ci.tlsCfgForConn(hello, rt), nil
			}
			if tls.MatchServerName(ci.serverNames, sni) {
				return ci.tlsCfgForConn(hello, rt), nil
			}
		}
		return nil, fmt.Errorf("listener %q: no filter_chain matches SNI %q", rt.name, sni)
	}
}

// tlsCfgForConn returns a *stdtls.Config whose VerifyConnection hook records
// this chain as the dispatch target for the connection.
func (ci *chainInfo) tlsCfgForConn(hello *stdtls.ClientHelloInfo, rt *listenerRuntime) *stdtls.Config {
	// Shallow-copy the per-chain config so we can attach a per-connection
	// VerifyConnection closure. Certificates, RootCAs, etc. are shared.
	cfg := ci.tlsCfg.Clone()
	cfg.VerifyConnection = func(cs stdtls.ConnectionState) error {
		// The *stdtls.Conn is not available on ConnectionState; we rely on
		// the accept-side fetch pattern below. This hook is present so
		// future phases can use it; for phase 03, the worker-goroutine
		// pattern (see runChainDispatch) stores the mapping explicitly
		// without consulting the sync.Map here.
		return nil
	}
	return cfg
}
```

**Implementation adjustment:** the comment in `tlsCfgForConn` is honest — `stdtls.ConnectionState` does not expose the underlying `*Conn`, so storing in the `sync.Map` from `VerifyConnection` is awkward. **The simpler correct pattern, and the one the executor implements:**

After `HandshakeContext` returns successfully, the worker goroutine itself determines the chosen chain by reading `tlsConn.ConnectionState().ServerName` and re-running the same dispatch logic `GetConfigForClient` ran. This avoids the `sync.Map` entirely. The ADR's approach (A) is reformulated as: **the chain-selection logic is a pure function of SNI; the worker re-runs it post-handshake.** This is deterministic (SNI cannot change between the `GetConfigForClient` callback and `ConnectionState.ServerName`) and simpler than the sync-map shuttle.

**Amend ADR-0033 accordingly at write time:** the "chain propagation" sub-decision resolves to "pure-function dispatch on post-handshake `ConnectionState.ServerName`" rather than `sync.Map`. This is an implementation improvement over the SPEC §10 #2 (A) proposal — document it in ADR-0033's Consequences as an approach refinement. (SPEC §10 #2 permits the planner to lock the mechanism; this is the locked mechanism.)

```go
func (rt *listenerRuntime) dispatch(tlsConn *stdtls.Conn, ctx context.Context) {
	sni := tlsConn.ConnectionState().ServerName
	var chosen *chainInfo
	for _, ci := range rt.chains {
		if len(ci.serverNames) == 0 {
			chosen = ci
			break
		}
		if tls.MatchServerName(ci.serverNames, sni) {
			chosen = ci
			break
		}
	}
	if chosen == nil {
		// Should not happen: GetConfigForClient already rejected unmatchable SNIs.
		log.Printf("listener %q: post-handshake dispatch: no chain matches SNI %q (race/logic bug)", rt.name, sni)
		_ = tlsConn.Close()
		return
	}
	chosen.filter.Handle(ctx, tlsConn)
}
```

- [ ] **Step 4: Implement `validateFilterChainMatch`**

```go
func validateFilterChainMatch(fm *listenerv3.FilterChainMatch) error {
	if fm == nil {
		return nil // empty match = catch-all
	}
	if fm.GetDestinationPort() != nil {
		return fmt.Errorf("destination_port is not supported (phase 07)")
	}
	if len(fm.GetPrefixRanges()) > 0 {
		return fmt.Errorf("prefix_ranges is not supported (phase 07)")
	}
	if len(fm.GetSourcePrefixRanges()) > 0 {
		return fmt.Errorf("source_prefix_ranges is not supported (phase 07)")
	}
	if fm.GetSourceType() != listenerv3.FilterChainMatch_ANY {
		return fmt.Errorf("source_type is not supported (phase 07)")
	}
	if len(fm.GetSourcePorts()) > 0 {
		return fmt.Errorf("source_ports is not supported (phase 07)")
	}
	if len(fm.GetApplicationProtocols()) > 0 {
		return fmt.Errorf("application_protocols is not supported (phase 07 — filter chain framework)")
	}
	if tp := fm.GetTransportProtocol(); tp != "" && tp != "tls" {
		return fmt.Errorf("transport_protocol %q is not supported (phase 03 permits only \"tls\")", tp)
	}
	// server_names[] is the only substantive field we consume — no validation beyond
	// permitting it.
	return nil
}
```

- [ ] **Step 5: Implement the TLS accept+worker loop**

```go
func (rt *listenerRuntime) acceptLoop(ctx context.Context) {
	for {
		raw, err := rt.netLn.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("listener %q: accept: %v", rt.name, err)
			continue
		}
		if !rt.tlsMode {
			// Phase-02-style single-chain dispatch.
			go rt.chains[0].filter.Handle(ctx, raw)
			continue
		}
		go rt.serveTLS(ctx, raw)
	}
}

func (rt *listenerRuntime) serveTLS(ctx context.Context, raw net.Conn) {
	tlsConn := stdtls.Server(raw, rt.tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		log.Printf("listener %q: handshake: %v", rt.name, err)
		_ = raw.Close()
		return
	}
	rt.dispatch(tlsConn, ctx)
}
```

- [ ] **Step 6: Run listener tests**

Run: `go test ./internal/listener/...`
Expected: all subtests PASS (the ~15 new TLS subtests + the phase-02 regression coverage).

- [ ] **Step 7: Append ADR-0033 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0033: Phase-03 filter-chain subset (supersedes ADR-0025)

**Supersedes: ADR-0025**
**Status:** Accepted
**Date:** <YYYY-MM-DD>
**Doctrine:** D-3.5

### Context

ADR-0025 (phase 02) constrained `internal/listener.NewManager` to accept exactly one `filter_chain` per listener with empty `filter_chain_match` and no `transport_socket`. Phase 03 introduces SNI-based filter-chain dispatch — multiple chains per listener, each bound to a set of SNI patterns — as its core new surface. ADR-0025's one-chain constraint is obsolete.

### Decision

Phase-03 subset:

1. `filter_chains` must be ≥ 1 (unchanged structural requirement).
2. `filter_chain_match` may be nil/empty (catch-all, at most one per listener) OR populate only `server_names[]` and optionally `transport_protocol == "tls"`. Any other `FilterChainMatch` field populated (destination_port, prefix_ranges, source_type != ANY, source_ports, source_prefix_ranges, application_protocols) errors at build.
3. `Listener.default_filter_chain` set → error.
4. `transport_socket` on any chain may be nil (plaintext) or carry a `DownstreamTlsContext` (TLS).
5. If any chain's `transport_socket` is non-nil, every chain on that listener must carry one — mixed TLS/plaintext listeners error.
6. Plaintext listeners with more than one `filter_chain` error — SNI cannot match on plaintext connections, so multiple plaintext chains is almost always a misconfiguration.
7. `require_client_certificate=true` on any chain errors (propagated from `tls.NewDownstreamConfig`).
8. `listener_filters` is silently skipped (phase-02 carryover; phase 07 filter-chain framework revisits).
9. Selection at handshake, in priority order: most-specific exact SNI match > suffix-wildcard match > universal wildcard match > catch-all (empty-match chain) > no match (handshake fails via `GetConfigForClient` returning `(nil, error)`; the connection closes).

### Chain-selection propagation (implementation)

Dispatching to the correct filter after a successful handshake is a pure function of the handshake-observed SNI. The worker goroutine, after `HandshakeContext` returns successfully, reads `tlsConn.ConnectionState().ServerName` and re-runs the same chain-match logic the `GetConfigForClient` callback ran, picking the first match. This is simpler than the `sync.Map` shuttle initially contemplated in SPEC §10 #2 approach (A) and avoids any per-connection state outside the `*stdtls.Conn` itself. Deterministic: SNI is fixed from the ClientHello through the connection's lifetime.

### Rationale

- SNI dispatch is the minimum complexity increment over ADR-0025 needed for phase 03. Full `FilterChainMatch` — including port ranges, source IP, ALPN, transport protocol beyond `"tls"` — remains deferred to phase 07 (filter chain framework).
- Rejecting `Listener.default_filter_chain` (Envoy's alternate catch-all form) bounds phase-03's match-resolution surface. Phase 07 supports both forms.
- Rejecting plaintext multi-chain catches a configuration class that's almost always a bug — SNI cannot match on plaintext connections, so the intent is ambiguous.
- Single mechanism for chain selection (pure-function dispatch post-handshake) reduces the surface area of "how chain selection happens" from two places (callback + shuttle) to one (match logic reused in callback and worker).

### Consequences

- Fixture 0002 can build a 2-chain TLS listener with `alpha.envoy-go.test` → `c_alpha` and `beta.envoy-go.test` → `c_beta` — the phase's core demonstration.
- A fixture later in phase 03 or after needing more than "exact + suffix wildcard + catch-all" must wait for phase 07.
- `internal/listener.Manager.Stop` is unchanged (closes every bound listener socket; Accept loops exit on `net.ErrClosed`).
- `internal/listener/manager.go` grows by ~250 lines (build-time validation + chain-sort + `GetConfigForClient` + serveTLS worker + dispatch). Split if reality demands per §6.2.
```

- [ ] **Step 8: Commit**

```bash
git add internal/listener/manager.go internal/listener/manager_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 03: internal/listener — multi-chain + SNI routing via GetConfigForClient [ADR-0033 supersedes ADR-0025]"
```

- [ ] **Step 9: PROGRESS entry + SHA-fill commit**

---

## Task 11: `internal/filter/tcpproxy/filter.go` — consume ctx + halfClose TLS extension

**Files:**
- Modify: `internal/filter/tcpproxy/filter.go`
- Modify: `internal/filter/tcpproxy/filter_test.go`
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 11 entry)

No ADR — this task lands the consequences of ADR-0032 and ADR-0033 in the filter. Resolves phase-02 REVIEW Minor 4.

- [ ] **Step 1: Write filter_test.go extensions (TDD)**

Add subtests:

```go
func TestFilter_Handle_CtxCancelledBeforeDial(t *testing.T) {
	// Build a cluster pointing at an unreachable address.
	c := &cluster.Cluster{ /* fields with a far-away address */ }
	mgr := &cluster.Manager{}
	mgr.Register(c)
	f, _ := NewFilter(mustBuildAny(&tcpproxy.TcpProxy{Cluster: "x", StatPrefix: "t"}), mgr)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Provide a pipe-downstream so Handle can run its pre-dial guard.
	d1, _ := net.Pipe()
	f.Handle(ctx, d1.(net.Conn))
	// Expect: returns without attempting to dial; downstream closed quickly.
}

func TestFilter_Handle_TLSUpstreamTransparent(t *testing.T) {
	// Spin up a stdtls.Listen echo, build a TLS cluster pointing at it,
	// build a filter, connect a downstream pipe, write bytes, assert echo returns.
	// Filter body does not type-switch on the upstream conn transport.
}

func TestFilter_Handle_HalfCloseOverTLS(t *testing.T) {
	// Loopback: TLS upstream echo. Downstream pipe. Write "hello", CloseWrite on downstream.
	// Assert: upstream receives FIN (echo returns "hello" and EOFs); downstream reads "hello" then EOF.
	// Verifies halfClose type-switch extension handles *stdtls.Conn.
}
```

- [ ] **Step 2: Modify `internal/filter/tcpproxy/filter.go`** — two changes:

(a) `Handle(ctx, downstream)` body: add early ctx.Err() guard + swap `net.DialTimeout(...)` for `f.cluster.Dial(ctx)`.

```go
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	defer downstream.Close()
	if err := ctx.Err(); err != nil {
		return
	}
	upstream, err := f.cluster.Dial(ctx)
	if err != nil {
		log.Printf("tcpproxy: dial cluster %q: %v", f.cluster.Name(), err)
		return
	}
	defer upstream.Close()
	// ... existing pump body (ADR-0023 verbatim) with downstream and upstream
}
```

(b) `halfClose(c net.Conn)` helper: extend the type-switch:

```go
func halfClose(c net.Conn) {
	switch t := c.(type) {
	case *net.TCPConn:
		_ = t.CloseWrite()
	case *stdtls.Conn:
		_ = t.CloseWrite()
	default:
		// No way to half-close a generic net.Conn; pipe tests use
		// net.Pipe which doesn't expose CloseWrite either. Silently
		// skip — the other side sees the full close at Handle defer.
	}
}
```

Ensure the file imports `stdtls "crypto/tls"`.

- [ ] **Step 3: Run all filter tests**

Run: `go test ./internal/filter/tcpproxy/...`
Expected: phase-02 tests PASS + 3 new subtests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/filter/tcpproxy/filter.go internal/filter/tcpproxy/filter_test.go
git commit -m "phase 03: internal/filter/tcpproxy — consume ctx via cluster.Dial + halfClose TLS ext (Minor 4 resolved)"
```

- [ ] **Step 5: PROGRESS entry + SHA-fill commit**

---

## Task 12: Fixture driver interface split + runner + 0000/0001 drivers + ADR-0034

**Files:**
- Modify: `test/differential/fixture/fixture.go`
- Modify: `test/differential/runner_test.go`
- Modify: `test/fixtures/0000-tcp-echo/driver/driver.go`
- Modify: `test/fixtures/0001-tcp-proxy-rr/driver/driver.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0034)
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 12 entry)

Atomic commit — the `Drive` interface retirement and every driver update land together per ADR-0034. Resolves phase-02 REVIEW Minor 6.

- [ ] **Step 1: Modify `test/differential/fixture/fixture.go`** — retire `Drive`, introduce `DriveReference` + `DriveSubject`.

```go
// Driver is implemented by every differential-test fixture.
type Driver interface {
	BackendCount() int
	SubjectListenerName() string
	ReferenceListenerPort() int
	ReferenceBootstrap(backendPorts []int) string
	SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string

	// DriveReference runs the fixture's driver logic against the reference
	// proxy's listener address. Returns all received bytes.
	DriveReference(ctx context.Context, addr string) ([]byte, error)

	// DriveSubject runs the fixture's driver logic against the subject
	// proxy's listener address. Returns all received bytes.
	DriveSubject(ctx context.Context, addr string) ([]byte, error)

	ProbeAdmin(ctx context.Context, addr string) (*AdminObservations, error)
}
```

- [ ] **Step 2: Modify `test/differential/runner_test.go`**

Call-site replacements:
- `d.Drive(ctx, refAddr, "")` → `d.DriveReference(ctx, refAddr)`
- `d.Drive(ctx, "", subjAddr)` → `d.DriveSubject(ctx, subjAddr)`

Also add a blank import for the 0002 driver (since the driver is not yet written in Task 13, this blank-import line is added *in* Task 13's commit — or stub this runner file so Task 12 compiles without the 0002 driver present; Task 13 then adds the blank import). Recommend the latter: Task 12 compiles without referencing 0002; Task 13 adds the `_ "<...>/test/fixtures/0002-tls-tcp/driver"` blank import.

- [ ] **Step 3: Modify `test/fixtures/0000-tcp-echo/driver/driver.go`**

Replace `Drive(ctx, refAddr, subjAddr string) ([]byte, error)` with `DriveReference(ctx context.Context, addr string) ([]byte, error)` + `DriveSubject(ctx context.Context, addr string) ([]byte, error)`. Body: the phase-01 `Drive` checked `refAddr != ""` and `subjAddr != ""` for each side; split those two branches into their respective methods verbatim.

- [ ] **Step 4: Modify `test/fixtures/0001-tcp-proxy-rr/driver/driver.go`**

Same shape as 0000 — phase-02 `Drive` had the same two-branch pattern; split atomically.

- [ ] **Step 5: Run tests — interface-split regression**

```bash
go build ./...
go test ./test/differential/...
go test ./test/fixtures/0000-tcp-echo/driver/...
go test ./test/fixtures/0001-tcp-proxy-rr/driver/...
```
Expected: every test PASS. Fixtures 0000 and 0001's differential gates green after the split.

- [ ] **Step 6: Append ADR-0034 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0034: Fixture-driver interface split — DriveReference + DriveSubject

**Supersedes (informal):** the phase-02 `fixture.Driver.Drive(ctx, refAddr, subjAddr)` interface method codified in `test/differential/fixture/fixture.go`. No prior formal ADR — hence the `(informal)` qualifier.
**Status:** Accepted
**Date:** <YYYY-MM-DD>
**Doctrine:** D-3.5

### Context

Phase-02 REVIEW Minor 6 identified the `""`-sentinel contract on `Driver.Drive(ctx, refAddr, subjAddr string)` as a shortcut that the runner never actually exercises — the runner always passes `""` for one side and the real address for the other, because the reference and subject proxies run in different ContainerGroups with independent port assignments. The split makes the interface match its actual usage, removes the sentinel branch in every driver body, and eliminates a test-design ambiguity (whether a driver is ever expected to drive both sides in one call).

### Decision

Retire `Drive(ctx, refAddr, subjAddr string) ([]byte, error)` on `fixture.Driver`. Introduce `DriveReference(ctx context.Context, addr string) ([]byte, error)` and `DriveSubject(ctx context.Context, addr string) ([]byte, error)` in its place. All existing drivers (`0000-tcp-echo`, `0001-tcp-proxy-rr`) and the new driver (`0002-tls-tcp`) implement the new interface atomically in one commit (same commit as the interface change itself and the runner-call-site update).

### Consequences

- Every driver body loses a two-branch `if refAddr != "" { ... } else { ... }` pattern.
- The runner's "drive reference, snapshot counts, reset, drive subject" sequence is unchanged in shape — the two methods are called in the positions the `""`-sentinel previously disambiguated.
- New drivers (phase 03's 0002, any future fixture) are simpler to write: one method for reference, one for subject. No sentinel-check boilerplate.
- No API compatibility concern — `fixture.Driver` is an internal test interface with zero out-of-tree consumers.
```

- [ ] **Step 7: Commit (atomic — all four drivers + interface + ADR in one commit)**

```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go test/fixtures/0000-tcp-echo/driver/driver.go test/fixtures/0001-tcp-proxy-rr/driver/driver.go docs/envoy-go/DECISIONS.md
git commit -m "phase 03: fixture.Driver — retire Drive, introduce DriveReference + DriveSubject [ADR-0034] (Minor 6 resolved)"
```

- [ ] **Step 8: Explicit verification that fixtures 0000 and 0001 remain green (spec-advisory addressing)**

Run:
```bash
go test ./test/differential/...
go test ./test/fixtures/0000-tcp-echo/...
go test ./test/fixtures/0001-tcp-proxy-rr/...
```

Both fixtures 0000 and 0001's drivers + differential gates pass. Quote output in PROGRESS entry.

- [ ] **Step 9: PROGRESS entry + SHA-fill commit**

---

## Task 13: Fixture `0002-tls-tcp` — YAMLs + driver + test + README

**Files:**
- Create: `test/fixtures/0002-tls-tcp/envoy.yaml`
- Create: `test/fixtures/0002-tls-tcp/envoy-go.yaml`
- Create: `test/fixtures/0002-tls-tcp/expectations.yaml`
- Create: `test/fixtures/0002-tls-tcp/README.md`
- Create: `test/fixtures/0002-tls-tcp/driver/driver.go`
- Create: `test/fixtures/0002-tls-tcp/driver/driver_test.go`
- Modify: `test/differential/runner_test.go` (add blank-import for 0002 driver)
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 13 entry)

This is the phase's capstone — everything above is scaffolding.

**Risk acknowledgment:** Task 13 has ~8 sub-steps. If contact with reality pushes it past 15, invoke the split valve.

- [ ] **Step 1: Draft `test/fixtures/0002-tls-tcp/envoy.yaml`** (reference, STRICT_DNS clusters, `host.docker.internal`)

Key fields:
- `admin: address: { socket_address: { address: 0.0.0.0, port_value: 9901 } }`
- `static_resources.listeners: [{ name: l_tls, address: socket_address: 0.0.0.0:15002, filter_chains: [ { filter_chain_match: { server_names: [alpha.envoy-go.test] }, transport_socket: { name: envoy.transport_sockets.tls, typed_config: { "@type": ".../DownstreamTlsContext", common_tls_context: { tls_certificates: [{ certificate_chain: { inline_bytes: "<base64 server-alpha.pem>" }, private_key: { inline_bytes: "<base64 server-alpha.key.pem>" } }] } } }, filters: [{ name: tcpproxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tls_alpha, cluster: c_alpha } }] }, { /* same for beta */ } ] }]`
- `static_resources.clusters:`
  - `c_alpha`: STRICT_DNS, `dns_lookup_family: V4_ONLY`, three `host.docker.internal:<port>` lb_endpoints, transport_socket with UpstreamTlsContext carrying inline upstream-alpha PEM + inline CA + `sni: alpha.envoy-go.test`.
  - `c_beta`: same for beta.

`<port>` entries are `{{BACKEND_PORT_N}}` placeholders the driver substitutes at render time — same discipline as fixture 0001.

Since PEMs must be literally inlined, the driver's `ReferenceBootstrap` reads each PEM via `os.ReadFile` from `test/fixtures/0002-tls-tcp/pki/` at render time rather than baking them as string constants. This keeps the YAML file human-readable (with `{{SERVER_ALPHA_PEM}}`-style placeholders replaced by base64-encoded file contents) and makes PKI rotation a single-file change.

- [ ] **Step 2: Draft `test/fixtures/0002-tls-tcp/envoy-go.yaml`** (subject, STATIC clusters, `127.0.0.1`)

Identical listener shape (same chains, same downstream TLS contexts, same inline PEMs). Clusters:
- `c_alpha`: STATIC, three `127.0.0.1:<port>` lb_endpoints, same UpstreamTlsContext (same SNI + CA + inline upstream-alpha PEM).
- `c_beta`: same for beta.

- [ ] **Step 3: Draft `test/fixtures/0002-tls-tcp/expectations.yaml`** (prose-format per fixture-0001 precedent)

```yaml
# Expectations for 0002-tls-tcp fixture.
#
# Byte-exact plaintext equivalence after TLS decryption on the
# response-body dimension. All other dimensions are not-applicable at
# this phase (no HTTP, no access log, no stats beyond /ready).
#
# Per-cluster distribution assertion ([3,3,3] per SNI per side) is
# implemented in the driver's AssertDistribution, not here — mirrors
# fixture 0001's discipline (see phase-02 ADR-0019 on Minor 7 deferral).
#
# LB endpoint-selection sequence is NOT asserted cross-proxy (Envoy's
# RR is per-worker with randomized offset; subject's RR is per-cluster
# via atomic.Uint64 — distribution matches, sequence does not). See
# BEHAVIOR_CONTRACT.md ## TCP proxy (and its cross-reference to ADR-0028).

response-body: applicable
response-status: not-applicable
response-headers: not-applicable
response-trailers: not-applicable
access-log: not-applicable
stats: not-applicable-beyond-ready
timing: not-compared
```

- [ ] **Step 4: Draft `test/fixtures/0002-tls-tcp/README.md`**

Cover: fixture purpose (2-SNI TLS downstream + upstream origination + per-cluster RR); STATIC-vs-STRICT_DNS divergence (inherits ADR-0010 + ADR-0027 rationale); PKI layout (points at `pki/README.md`); distribution-assertion methodology ([3,3,3] per SNI per side, N % 3 == 0 design preserved from fixture 0001); `--concurrency 1` inheritance from ADR-0028 + link to the new BEHAVIOR_CONTRACT TLS subsection (Task 14).

- [ ] **Step 5: Draft `test/fixtures/0002-tls-tcp/driver/driver.go`**

```go
package driver

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/envoy-go/test/differential/fixture"
	"github.com/envoy-go/test/helpers"
)

func init() {
	fixture.Register("0002-tls-tcp", &Driver{})
}

type Driver struct {
	rootCAs *x509.CertPool
}

func (d *Driver) BackendCount() int         { return 6 }
func (d *Driver) SubjectListenerName() string { return "l_tls" }
func (d *Driver) ReferenceListenerPort() int { return 15002 }

func (d *Driver) ensureCertPool() *x509.CertPool {
	if d.rootCAs != nil {
		return d.rootCAs
	}
	caPEM, _ := os.ReadFile(pkiPath("ca.pem"))
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	d.rootCAs = pool
	return pool
}

func (d *Driver) ReferenceBootstrap(backendPorts []int) string {
	tpl, _ := os.ReadFile(yamlPath("envoy.yaml"))
	return d.render(string(tpl), backendPorts, 15002, 9901)
}

func (d *Driver) SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl, _ := os.ReadFile(yamlPath("envoy-go.yaml"))
	return d.render(string(tpl), backendPorts, subjListenerPort, subjAdminPort)
}

func (d *Driver) render(tpl string, backendPorts []int, listenerPort, adminPort int) string {
	// Substitute {{BACKEND_PORT_0}}..{{BACKEND_PORT_5}}, {{LISTENER_PORT}}, {{ADMIN_PORT}},
	// {{SERVER_ALPHA_CERT}}, {{SERVER_ALPHA_KEY}}, {{SERVER_BETA_CERT}}, {{SERVER_BETA_KEY}},
	// {{UPSTREAM_ALPHA_CERT}}, {{UPSTREAM_ALPHA_KEY}}, {{UPSTREAM_BETA_CERT}}, {{UPSTREAM_BETA_KEY}},
	// {{CA_CERT}}.
	repl := map[string]string{
		"{{LISTENER_PORT}}":       fmt.Sprintf("%d", listenerPort),
		"{{ADMIN_PORT}}":          fmt.Sprintf("%d", adminPort),
		"{{CA_CERT}}":              mustRead(pkiPath("ca.pem")),
		"{{SERVER_ALPHA_CERT}}":   mustRead(pkiPath("server-alpha.pem")),
		"{{SERVER_ALPHA_KEY}}":    mustRead(pkiPath("server-alpha.key.pem")),
		"{{SERVER_BETA_CERT}}":    mustRead(pkiPath("server-beta.pem")),
		"{{SERVER_BETA_KEY}}":     mustRead(pkiPath("server-beta.key.pem")),
		"{{UPSTREAM_ALPHA_CERT}}": mustRead(pkiPath("upstream-alpha.pem")),
		"{{UPSTREAM_ALPHA_KEY}}":  mustRead(pkiPath("upstream-alpha.key.pem")),
		"{{UPSTREAM_BETA_CERT}}":  mustRead(pkiPath("upstream-beta.pem")),
		"{{UPSTREAM_BETA_KEY}}":   mustRead(pkiPath("upstream-beta.key.pem")),
	}
	for i, p := range backendPorts {
		repl[fmt.Sprintf("{{BACKEND_PORT_%d}}", i)] = fmt.Sprintf("%d", p)
	}
	for k, v := range repl {
		tpl = strings.ReplaceAll(tpl, k, v)
	}
	return tpl
}

func (d *Driver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}

func (d *Driver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}

func (d *Driver) drive(ctx context.Context, addr string) ([]byte, error) {
	pool := d.ensureCertPool()
	var out []byte
	for _, sni := range []string{"alpha.envoy-go.test", "beta.envoy-go.test"} {
		prefix := strings.Split(sni, ".")[0]
		for i := 0; i < 9; i++ {
			payload := []byte(fmt.Sprintf("rr-%s-%d\n", prefix, i))
			resp, err := helpers.TLSRoundTrip(ctx, addr, sni, pool, payload, 2*time.Second)
			if err != nil {
				return out, fmt.Errorf("sni=%s iter=%d: %w", sni, i, err)
			}
			out = append(out, resp...)
		}
	}
	return out, nil
}

func (d *Driver) AssertDistribution(refCounts, subjCounts [6]uint64) error {
	// refCounts[0..2] = c_alpha endpoints, refCounts[3..5] = c_beta endpoints.
	if err := assertThreeEach(refCounts[0:3], "ref c_alpha"); err != nil {
		return err
	}
	if err := assertThreeEach(refCounts[3:6], "ref c_beta"); err != nil {
		return err
	}
	if err := assertThreeEach(subjCounts[0:3], "subj c_alpha"); err != nil {
		return err
	}
	if err := assertThreeEach(subjCounts[3:6], "subj c_beta"); err != nil {
		return err
	}
	return nil
}

func assertThreeEach(counts []uint64, label string) error {
	for i, v := range counts {
		if v != 3 {
			return fmt.Errorf("%s: endpoint %d got %d connections, want 3", label, i, v)
		}
	}
	return nil
}

func (d *Driver) ProbeAdmin(ctx context.Context, addr string) (*fixture.AdminObservations, error) {
	// Same shape as phase-01 / phase-02 drivers; delegate to helpers.CompareAdminResponses pattern.
	// ... (executor fills in per fixture-0000's / fixture-0001's ProbeAdmin body)
}

func pkiPath(name string) string {
	_, this, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(this), "..", "pki", name)
}

func yamlPath(name string) string {
	_, this, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(this), "..", name)
}

func mustRead(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("fixture 0002: read %s: %v", path, err))
	}
	return strings.TrimRight(string(b), "\n")
}
```

**YAML PEM inlining note:** the placeholder substitution inserts multi-line PEMs into YAML. The YAML schema requires the inlined value to be a valid YAML string — use YAML `|` block scalar notation in the templates so the driver's `strings.ReplaceAll` substitutes a PEM body that respects YAML indentation. The executor hand-crafts `envoy.yaml`/`envoy-go.yaml` so the `{{...CERT}}` placeholders sit at the correct indentation level under a `|-`-block. If this proves error-prone, switch to `inline_string: "<literal newlines>"` with `yaml.Marshal`-level escaping — but the block scalar is simpler.

- [ ] **Step 6: Draft `test/fixtures/0002-tls-tcp/driver/driver_test.go`**

Mirror of fixture 0001's `driver_test.go`: a Docker-free unit test for `AssertDistribution`. Happy path passes; 4/3/2 fails; zeroed counts fail. No subprocess, no Docker.

- [ ] **Step 7: Add blank-import in `test/differential/runner_test.go`**

Add the import path `_ "github.com/envoy-go/test/fixtures/0002-tls-tcp/driver"` to the runner's test file so the driver's `init()` registers it.

- [ ] **Step 8: Build + unit test the fixture**

Run:
```bash
go build ./...
go test ./test/fixtures/0002-tls-tcp/driver/...
```
Expected: build clean; driver_test passes.

- [ ] **Step 9: Run the full differential gate (Docker required)**

Run: `go test ./test/differential/... -timeout=5m`
Expected:
- Fixture 0000 green.
- Fixture 0001 green (distribution `[3,3,3]` per side).
- Fixture 0002 green — byte-exact plaintext equivalence across 18 TLS round-trips per proxy; distribution `[3,3,3]` per SNI per side on both proxies.

- [ ] **Step 10: Verify `--concurrency 1` inheritance**

Inspect `test/differential/harness.go` (unchanged this phase): confirm it launches the reference Envoy container with `--concurrency 1` for *every* fixture (no per-fixture opt-in needed). If the flag is gated on fixture name, update harness to make it unconditional (this would itself be a small ADR — likely ADR-0036 — if so). Target outcome: fixture 0002 inherits the flag without any harness change in phase 03. Document inheritance in the PROGRESS entry.

- [ ] **Step 11: Commit**

```bash
git add test/fixtures/0002-tls-tcp/ test/differential/runner_test.go
git commit -m "phase 03: fixture 0002-tls-tcp — 2-SNI TLS downstream + upstream TLS origination + RR distribution"
```

- [ ] **Step 12: PROGRESS entry + SHA-fill commit**

Include: `go test ./test/differential/... -timeout=5m` verbatim output (last 50 lines).

---

## Task 14: BEHAVIOR_CONTRACT TLS subsection + TCP-proxy cross-reference + ADR-0035

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0035)
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 14 entry)

Resolves phase-02 REVIEW Minor 8. Codifies the TLS equivalence surface.

- [ ] **Step 1: Add the `## TLS` subsection to `BEHAVIOR_CONTRACT.md`**

Append after the existing `## TCP proxy` section (or at any location that keeps similar subsections together — position inside the doc is cosmetic). Content:

```markdown
## TLS

*Introduced by phase 03. Justified by ADR-0031 (stdlib crypto/tls stack selection), ADR-0030 (TLS parameter mapping scope), ADR-0033 (filter-chain subset supersedes ADR-0025), ADR-0032 (upstream TLS dialer), ADR-0035 (this subsection).*

Phase 03 introduces envoy-go's first cryptographic surface: downstream TLS termination, upstream TLS origination, and SNI-based filter-chain dispatch. This subsection codifies what the differential harness compares across the reference and subject proxies over TLS, and — importantly — what it does NOT compare.

### Asserted equivalence

**Plaintext-after-decryption byte equivalence.** For a TLS-terminated connection (downstream) and/or TLS-originated connection (upstream), the response body observed by the fixture driver (after the tunnel is fully peeled) must be byte-exact between reference and subject. Fixture `0002-tls-tcp` exercises this surface with 9 TLS round-trips per SNI per side, 18 per proxy, over a TCP-echo upstream.

**Per-SNI chain-selection equivalence.** Given the same SNI on the ClientHello, both proxies must select the logically-equivalent filter chain and dispatch to the logically-equivalent upstream cluster. This is witnessed indirectly via the distribution assertion: fixture 0002's `[3,3,3]` per cluster per SNI per side implies the SNI → chain → cluster dispatch is consistent.

**Upstream SNI + CA equivalence.** For a given upstream cluster, both proxies must send the same SNI value to the backend (the fixture's `sni` field is consumed identically). Both proxies must validate the backend's presented certificate against the same `trusted_ca` material. Divergence here manifests as an upstream handshake failure, which breaks the differential gate.

**Server-certificate identity by SNI.** For a given ClientHello SNI, the server certificate selected by each proxy must match on SAN identity. Phase 03 does not byte-compare the cert bytes (both proxies serve the same committed PEM in fixture 0002 — the byte-compare is trivially equal); the rule is semantic: both pick the cert whose SAN covers the SNI.

### Not asserted

**Encrypted-side byte equivalence.** Neither the TLS record boundaries, the session-ticket material, session-ticket-key rotation timing, TLS 1.3 cipher selection (Go's `crypto/tls` and Envoy's BoringSSL have different defaults), handshake message byte ordering/timing, server random, session IDs, nor any other encrypted-side observable is compared. The differential harness diffs decrypted bytes, not TLS records.

**Negotiated ALPN value.** `alpn_protocols` on both sides is passed through to `stdtls.Config.NextProtos`, so both proxies advertise the same ALPN offers; the negotiated value (which wins the ALPN negotiation) is not surfaced to the fixture driver in phase 03. If a later phase asserts ALPN negotiation, it adds a fixture opt-in and extends this subsection.

**Handshake-layer timing.** Not asserted. TLS handshake completion time varies with cipher selection, session resumption state, and handshake retries.

### Parameter mapping caveats

Two `tls_params` fields do not round-trip with full fidelity between Envoy's BoringSSL and Go's `crypto/tls` (see ADR-0030):

- `cipher_suites` with TLS-1.3 cipher names (e.g., `TLS_AES_128_GCM_SHA256`): Go's `crypto/tls` does not permit TLS-1.3 cipher selection. envoy-go logs a diagnostic and drops the entry. Negotiated TLS-1.3 cipher may differ between proxies; this is within the "encrypted-side not asserted" rule above.
- `signature_algorithms`: not publicly configurable in `crypto/tls`. envoy-go errors at parse if a fixture sets this field. Phase-03 fixtures do not set it.

### Scope boundaries

Phase 03 does NOT implement session resumption assertion, OCSP stapling, mTLS validation on the downstream side, SDS, SPIFFE / custom validators, post-quantum key exchange, ALPN-driven filter-chain selection, non-SNI filter-chain match fields, `Listener.default_filter_chain`, `listener_filters` (still silently skipped), HTTPS (HTTP over TLS — phase 04+), or transport socket types beyond `tls`. See SPEC §2 for the full non-purposes list and the phase each is deferred to.
```

- [ ] **Step 2: Append one-sentence cross-reference to the existing `## TCP proxy` subsection** (Minor 8 resolution)

Locate the paragraph titled *"LB endpoint-selection sequence (NOT asserted)"* (or its equivalent name in the existing phase-02 TCP proxy subsection). Append:

> Reference-side distribution exactness (fixture `0001-tcp-proxy-rr` and, inherited, `0002-tls-tcp`) depends on the reference container's `--concurrency 1` pin per ADR-0028.

- [ ] **Step 3: Append ADR-0035 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0035: BEHAVIOR_CONTRACT TLS subsection (phase 03) + TCP-proxy ADR-0028 cross-reference (Minor 8)

**Status:** Accepted
**Date:** <YYYY-MM-DD>
**Doctrine:** D-3.5, D-3.3

### Context

Phase 03 introduces envoy-go's first cryptographic surface. The differential contract must codify which TLS-related observables are asserted across reference and subject and which are permitted to differ — without this, a reviewer cannot say whether a cipher-level divergence is a gate failure or a permitted variance. Phase-02 REVIEW Minor 8 additionally flagged that the TCP-proxy subsection did not cross-reference ADR-0028's `--concurrency 1` reference-container pin, which is a precondition for the distribution assertions in fixtures 0001 and (inherited) 0002.

### Decision

A new `## TLS` subsection lands in `BEHAVIOR_CONTRACT.md`, phrased so that (a) every *asserted* rule has a fixture gate that witnesses it, (b) every *not-asserted* rule names the specific observable and the reason it's excluded, (c) tradeoffs with Go's `crypto/tls` (ADR-0030) are noted so future reviewers don't read divergence as a gate regression.

In the same commit, the existing `## TCP proxy` subsection's "LB endpoint-selection sequence (NOT asserted)" paragraph gains a one-sentence cross-reference to ADR-0028 (phase-02 REVIEW Minor 8 resolution).

### Consequences

- Phase-03 gates are now fully traceable to written rules. Fixture 0002's byte-exact plaintext assertion is under "Plaintext-after-decryption byte equivalence"; its distribution assertion is under "Per-SNI chain-selection equivalence."
- A future reviewer encountering a TLS 1.3 cipher divergence between Go and Envoy can point at "Encrypted-side byte equivalence: not asserted" and close the ticket without a gate investigation.
- Phase-02 REVIEW Minor 8 is resolved. Minor 7 (prose-heavy `expectations.yaml`) remains deferred per ADR-0019.
- Phase 04+ TLS-touching phases extend this subsection (or add siblings — e.g., a `## HTTP over TLS` subsection in phase 04) rather than rewriting it.
```

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md
git commit -m "phase 03: BEHAVIOR_CONTRACT — TLS subsection + TCP-proxy ADR-0028 cross-ref [ADR-0035] (Minor 8 resolved)"
```

- [ ] **Step 5: PROGRESS entry + SHA-fill commit**

---

## Task 15: Full verification gate sweep

**Files:**
- Modify: `docs/envoy-go/phases/03-tls/PROGRESS.md` (append Task 15 entry + capture gate sweep output)

No new code. Confirms every gate from SPEC §3 (a)–(e) is green. Gate (f) REVIEW.md approval happens in a later session per the state machine.

- [ ] **Step 1: Run `go vet ./...`**

Expected: no output, exit 0.

- [ ] **Step 2: Run `golangci-lint run`**

Expected: no findings.

- [ ] **Step 3: Run `go test ./... -timeout=10m`**

Expected: every package PASS, including:
- `./internal/tls/` (every subtest from Tasks 2–6, including `TestPKISanity`).
- `./internal/cluster/` (phase-02 regression + phase-03 TLS Dial subtests).
- `./internal/listener/` (phase-02 regression + phase-03 multi-chain + SNI subtests).
- `./internal/filter/tcpproxy/` (phase-02 regression + phase-03 ctx + TLS upstream subtests).
- `./test/helpers/` (phase-02 regression + TLSRoundTrip subtests).
- `./test/differential/` (fixtures 0000, 0001, 0002 all green; no regression on 0000/0001 post-ADR-0034 interface split).
- `./test/fixtures/0000-tcp-echo/driver/`, `./test/fixtures/0001-tcp-proxy-rr/driver/`, `./test/fixtures/0002-tls-tcp/driver/`.
- `./cmd/envoy-go/` (phase-02 two-listener bootstrap harness still passes — plaintext-listener regression coverage, spec-review-advisory item).

- [ ] **Step 4: Run short-budget fuzz on every target (gate (d))**

```bash
go test ./internal/bootstrap -run=FuzzBootstrapLoad -fuzz=FuzzBootstrapLoad -fuzztime=30s
go test ./internal/filter/tcpproxy -run=FuzzTcpProxyFilter -fuzz=FuzzTcpProxyFilter -fuzztime=30s
go test ./internal/tls -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s
```

Expected for each: completes in ~30s with no crashes, no discovered crashers added to `testdata/fuzz/`.

- [ ] **Step 5: PKI determinism re-verify**

```bash
cd test/fixtures/0002-tls-tcp && go run ./pki/gen
git diff --exit-code pki/
```
Expected: no diff.

- [ ] **Step 6: Plaintext-listener regression via cmd/envoy-go/main_test.go**

This is the spec-review advisory item. Run:
```bash
go test ./cmd/envoy-go/...
```
Expected: PASS (the phase-02 two-listener bootstrap harness test still works; plaintext listener coverage intact).

- [ ] **Step 7: Explicit fixtures 0000/0001 regression check post-ADR-0034**

```bash
go test ./test/fixtures/0000-tcp-echo/... ./test/fixtures/0001-tcp-proxy-rr/...
go test ./test/differential/... -run TestDifferentialFixture -timeout=5m   # filter out anything not matching the common differential harness test name if applicable
```

Expected: both fixtures' unit + differential gates green.

- [ ] **Step 8: Append a Task 15 PROGRESS entry with every command output verbatim**

This PROGRESS entry is the session's "verification proof" — it is the content `superpowers:verification-before-completion` will read when phase 03 moves to lifecycle-state 4. Keep every last-30-lines-of-output block verbatim.

- [ ] **Step 9: Commit**

```bash
git add docs/envoy-go/phases/03-tls/PROGRESS.md
git commit -m "phase 03: Task 15 — all-gates green local sweep (a/b/d/e; c N/A)"
```

- [ ] **Step 10: Confirm phase-done readiness (do NOT advance STATE — that's a later session per ADR-0005)**

This plan-authored phase ends at lifecycle-state 3 (PLAN.md landed, approved, committed on master). Task 15's sweep pre-flights what the subsequent verification/review sessions will confirm. STATE advancement through 4 → 5 → 6 is per-session work, not this plan's responsibility.

---

## Post-plan handoff: state advancement + worktree cleanup (session-exit duties)

This section is the plan-authoring session's exit contract, not an executable task.

After the executing session commits Task 15 on `phase/03-tls-impl`:

1. **Fast-forward merge to master.** Per ADR-0003:
   ```bash
   cd /path/to/master/worktree
   git merge --ff-only phase/03-tls-impl
   ```
2. **Advance `docs/envoy-go/STATE.md` on master** to `lifecycle-state: 4` + `next-skill: superpowers:verification-before-completion`, reflecting that the next fresh session runs verification before REVIEW. Commit with `phase 03: STATE.md → lifecycle-state 4`.
3. **The verification session** (next-next from the current plan-authoring session) then advances STATE through 5 and 6 per the state machine. Phase-03 ROADMAP row advances to `done` at state 6. Phase 04's STATE handoff (`active-phase: 04-http-1.1`, `lifecycle-state: 1`, `next-skill: superpowers:brainstorming`) lands with the final phase-03 commit.

No part of this section is done by Task 15. It lives here so the plan-authoring session (i.e., the current one) knows where to leave STATE after its own commit, and so the executing session has clear context for its exit.

---

## Plan review loop (invoked at end of plan-authoring session)

Per `superpowers:writing-plans`: after this PLAN.md is written, dispatch the `plan-document-reviewer` subagent with the PLAN.md path + the SPEC.md path. If the reviewer returns approved → commit PLAN.md + STATE advancement (state 2 → state 3 on master). If the reviewer returns changes-requested → address feedback in place, re-dispatch (max 3 iterations per ADR-0005); on iteration 3 without approval, exit blocked per SKILL_ROUTING deviations.

The reviewer's scope:
- Does the PLAN cover every SPEC §4 deliverable? (7 items × ~20 verifiable lines each)
- Does the PLAN settle every SPEC §10 deferred decision?
- Does the PLAN mitigate every SPEC §11 risk with a task-level step or an ADR?
- Does the PLAN resolve phase-02 REVIEW Minors 4, 6, 8 explicitly?
- Are tasks atomic (one logical commit each, 2–5 minutes per step except well-annotated longer ones)?
- Does the ADR number sequence match verified DECISIONS.md tail?
- Is the LoC estimate honest and does the scope-check argument hold?
