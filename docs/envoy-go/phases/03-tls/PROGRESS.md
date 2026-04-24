# Phase 03 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/PROGRESS.md structure.

## Preamble — execution preconditions

None. All preconditions were satisfied at cold-start. Docker client and server were both present and responsive (Docker Desktop 4.41.2). Go 1.26.2 satisfies the 1.23+ requirement. golangci-lint v1.64.8 matches ADR-0009. go-control-plane/envoy pinned at v1.32.4 per ADR-0013. DECISIONS.md tail is `## ADR-0028:` — no re-numbering needed. `--concurrency` in `test/differential/harness.go` is unconditional (line 117, inside `StartReferenceProxy`, no fixture gate).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** 8f48101
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions".
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/03-tls-impl
$ git log -1 --format=%H
9584ce79049bfabb571315535b1c56a61a81ce04
$ docker version
Client: Docker Engine - Community
 Version:           28.4.0
Server: Docker Desktop 4.41.2 (191736)
 Engine:
  Version:          28.1.1
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
$ go test ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.563s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	0.039s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.006s
ok  	github.com/esalaine/envoy-go/internal/cluster	0.004s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.005s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	0.004s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
?   	github.com/esalaine/envoy-go/internal/tls	[no test files]
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	4.066s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s
$ go list -m github.com/envoyproxy/go-control-plane/envoy
github.com/envoyproxy/go-control-plane/envoy v1.32.4
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0028: Reference Envoy `--concurrency 1` for deterministic single-worker round-robin
```

## Task 2 — internal/tls — MatchServerName + doc.go

**Commits:** a833d23
**Notes:** Pure function; no stdtls import yet; table-driven test covers exact / suffix-wildcard / universal / case / no-match / empty-patterns; all combinations green.
**Outputs:**
```
$ cd internal/tls && go test .
ok  	github.com/esalaine/envoy-go/internal/tls	0.002s
```

## Task 3 — internal/tls — loadDataSource + ADR-0029

**Commits:** f63119e
**Notes:** ADR-0029 landed in same commit as the code. Error-prefix discipline (`tls: `) preserved. Eight subtests green.
**Outputs:**
```
$ cd internal/tls && go test -run TestLoadDataSource .
ok  	github.com/esalaine/envoy-go/internal/tls	0.013s
```

## Task 4 — internal/tls — applyTLSParams + ADR-0030

**Commits:** 71b4972
**Notes:** ADR-0030 landed in same commit as the code. First `crypto/tls` import in the package (aliased `stdtls`). 14 subtests green (6 Versions + 3 CipherSuites + 3 ECDHCurves + 1 SignatureAlgorithmsErrors + 1 NilParams). TLS_AUTO test adjusted from the PLAN draft's "TLS_AUTO min errors" (which expected an error) to "TLS_AUTO min no-op" (no error, MinVersion stays 0, MaxVersion applied) — the adjustment aligns the test with the Step 3 `mapTLSVersion` code and ADR-0030 mapping table, both of which unambiguously describe TLS_AUTO as a no-op/treat-as-unset. golangci-lint clean (gofmt struct-field alignment auto-corrected before commit).
**Outputs:**
```
$ go test -v -run TestApplyTLSParams ./internal/tls/...
=== RUN   TestApplyTLSParams_Versions
=== RUN   TestApplyTLSParams_Versions/defaults_TLS_1.2_->_TLS_1.3
=== RUN   TestApplyTLSParams_Versions/TLS_1.2_only
=== RUN   TestApplyTLSParams_Versions/TLS_1.3_only
=== RUN   TestApplyTLSParams_Versions/TLS_1.0_min_errors
=== RUN   TestApplyTLSParams_Versions/TLS_1.1_max_errors
=== RUN   TestApplyTLSParams_Versions/TLS_AUTO_min_no-op
--- PASS: TestApplyTLSParams_Versions (0.00s)
    --- PASS: TestApplyTLSParams_Versions/defaults_TLS_1.2_->_TLS_1.3 (0.00s)
    --- PASS: TestApplyTLSParams_Versions/TLS_1.2_only (0.00s)
    --- PASS: TestApplyTLSParams_Versions/TLS_1.3_only (0.00s)
    --- PASS: TestApplyTLSParams_Versions/TLS_1.0_min_errors (0.00s)
    --- PASS: TestApplyTLSParams_Versions/TLS_1.1_max_errors (0.00s)
    --- PASS: TestApplyTLSParams_Versions/TLS_AUTO_min_no-op (0.00s)
=== RUN   TestApplyTLSParams_CipherSuites
=== RUN   TestApplyTLSParams_CipherSuites/known_TLS_1.2_cipher
=== RUN   TestApplyTLSParams_CipherSuites/unknown_cipher_errors
=== RUN   TestApplyTLSParams_CipherSuites/TLS_1.3_cipher_silently_dropped_with_diagnostic
2026/04/24 04:14:15 tls: tls_params: TLS-1.3-only cipher "TLS_AES_128_GCM_SHA256" requested; crypto/tls does not allow selection, dropping
--- PASS: TestApplyTLSParams_CipherSuites (0.00s)
    --- PASS: TestApplyTLSParams_CipherSuites/known_TLS_1.2_cipher (0.00s)
    --- PASS: TestApplyTLSParams_CipherSuites/unknown_cipher_errors (0.00s)
    --- PASS: TestApplyTLSParams_CipherSuites/TLS_1.3_cipher_silently_dropped_with_diagnostic (0.00s)
=== RUN   TestApplyTLSParams_ECDHCurves
=== RUN   TestApplyTLSParams_ECDHCurves/x25519_+_p256
=== RUN   TestApplyTLSParams_ECDHCurves/p384_+_p521
=== RUN   TestApplyTLSParams_ECDHCurves/unknown_curve_errors
--- PASS: TestApplyTLSParams_ECDHCurves (0.00s)
    --- PASS: TestApplyTLSParams_ECDHCurves/x25519_+_p256 (0.00s)
    --- PASS: TestApplyTLSParams_ECDHCurves/p384_+_p521 (0.00s)
    --- PASS: TestApplyTLSParams_ECDHCurves/unknown_curve_errors (0.00s)
=== RUN   TestApplyTLSParams_SignatureAlgorithmsErrors
--- PASS: TestApplyTLSParams_SignatureAlgorithmsErrors (0.00s)
=== RUN   TestApplyTLSParams_NilParams
--- PASS: TestApplyTLSParams_NilParams (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	0.004s
```

## Task 5 — internal/tls — NewDownstreamConfig + NewUpstreamConfig [ADR-0031]

**Commits:** 85ceb0b
**Notes:**
- ADR-0031 (TLS stack selection: stdlib `crypto/tls`) landed in same commit as the code (first site composing a `*stdtls.Config` for production use).
- `NewDownstreamConfig` and `NewUpstreamConfig` are both exported entry points; `commonTLSContextToConfig` is the unexported helper composing the three leaves (datasource/params/sni).
- Plan's `anypb.UnmarshalTo(ts.GetTypedConfig(), ctx, proto.UnmarshalOptions{})` corrected to `ts.GetTypedConfig().UnmarshalTo(ctx)` — matching phase-02 `internal/filter/tcpproxy/filter.go` idiom; no `proto` or `anypb` import needed in `config.go`.
- Plan's Step 1 `t.Skip` placeholders were fleshed out into full subtest bodies at implementation time (not in a separate Step 4): 3 downstream happy subtests + 11 downstream error subtests + 3 upstream happy subtests + 8 upstream error subtests + 1 `TestPKISanity` = 26 subtests total.
- Upstream "SDS-bound secret" test deviation: the PLAN example omits a `trusted_ca` in the SDS test; without it, `NewUpstreamConfig` fires the `trusted_ca is required` check before `commonTLSContextToConfig` reaches the SDS check. Fixed by adding a valid `TrustedCa` to that specific test case so the SDS gate is actually exercised — the real code path is covered correctly.
- British spellings ("honoured", "behaviour") converted to American in all Go comments per `.golangci.yml` `misspell.locale: US`.
- 36 subtests green (26 new + 10 prior package subtests carried through); coverage 87.4%.
- `golangci-lint run ./internal/tls/...` → empty (clean).
**Outputs:**
```
$ go test -v ./internal/tls/... 2>&1 | grep -E "^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)"
=== RUN   TestNewDownstreamConfig_Happy
=== RUN   TestNewDownstreamConfig_Happy/inline_PEMs
=== RUN   TestNewDownstreamConfig_Happy/tls_params_pulled_through
=== RUN   TestNewDownstreamConfig_Happy/alpn_protocols_populated
--- PASS: TestNewDownstreamConfig_Happy (0.00s)
    --- PASS: TestNewDownstreamConfig_Happy/inline_PEMs (0.00s)
    --- PASS: TestNewDownstreamConfig_Happy/tls_params_pulled_through (0.00s)
    --- PASS: TestNewDownstreamConfig_Happy/alpn_protocols_populated (0.00s)
=== RUN   TestNewDownstreamConfig_Errors
=== RUN   TestNewDownstreamConfig_Errors/wrong_type_url
=== RUN   TestNewDownstreamConfig_Errors/unmarshal_failure
=== RUN   TestNewDownstreamConfig_Errors/missing_tls_certificates
=== RUN   TestNewDownstreamConfig_Errors/malformed_PEM_in_certificate_chain
=== RUN   TestNewDownstreamConfig_Errors/SDS-bound_secret
=== RUN   TestNewDownstreamConfig_Errors/require_client_certificate
=== RUN   TestNewDownstreamConfig_Errors/custom_validator_config
=== RUN   TestNewDownstreamConfig_Errors/match_typed_subject_alt_names
=== RUN   TestNewDownstreamConfig_Errors/verify_certificate_hash
=== RUN   TestNewDownstreamConfig_Errors/password_on_key
=== RUN   TestNewDownstreamConfig_Errors/invalid_tls_params_TLSv1_0
--- PASS: TestNewDownstreamConfig_Errors (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/wrong_type_url (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/unmarshal_failure (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/missing_tls_certificates (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/malformed_PEM_in_certificate_chain (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/SDS-bound_secret (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/require_client_certificate (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/custom_validator_config (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/match_typed_subject_alt_names (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/verify_certificate_hash (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/password_on_key (0.00s)
    --- PASS: TestNewDownstreamConfig_Errors/invalid_tls_params_TLSv1_0 (0.00s)
=== RUN   TestNewUpstreamConfig_Happy
=== RUN   TestNewUpstreamConfig_Happy/inline_CA_+_SNI_+_tls_params
=== RUN   TestNewUpstreamConfig_Happy/alpn_protocols_populated
=== RUN   TestNewUpstreamConfig_Happy/allow_renegotiation_false_default_no_error
--- PASS: TestNewUpstreamConfig_Happy (0.00s)
    --- PASS: TestNewUpstreamConfig_Happy/inline_CA_+_SNI_+_tls_params (0.00s)
    --- PASS: TestNewUpstreamConfig_Happy/alpn_protocols_populated (0.00s)
    --- PASS: TestNewUpstreamConfig_Happy/allow_renegotiation_false_default_no_error (0.00s)
=== RUN   TestNewUpstreamConfig_Errors
=== RUN   TestNewUpstreamConfig_Errors/wrong_type_url
=== RUN   TestNewUpstreamConfig_Errors/missing_trusted_ca
=== RUN   TestNewUpstreamConfig_Errors/malformed_CA_PEM
=== RUN   TestNewUpstreamConfig_Errors/SDS-bound_secret
=== RUN   TestNewUpstreamConfig_Errors/allow_renegotiation
=== RUN   TestNewUpstreamConfig_Errors/custom_validator_config
=== RUN   TestNewUpstreamConfig_Errors/match_typed_subject_alt_names
=== RUN   TestNewUpstreamConfig_Errors/password_on_client-cert_key
--- PASS: TestNewUpstreamConfig_Errors (0.00s)
    --- PASS: TestNewUpstreamConfig_Errors/wrong_type_url (0.00s)
    --- PASS: TestNewUpstreamConfig_Errors/missing_trusted_ca (0.00s)
    --- PASS: TestNewUpstreamConfig_Errors/malformed_CA_PEM (0.00s)
    --- PASS: TestNewUpstreamConfig_Errors/SDS-bound_secret (0.00s)
    --- PASS: TestNewUpstreamConfig_Errors/allow_renegotiation (0.00s)
    --- PASS: TestNewUpstreamConfig_Errors/custom_validator_config (0.00s)
    --- PASS: TestNewUpstreamConfig_Errors/match_typed_subject_alt_names (0.00s)
    --- PASS: TestNewUpstreamConfig_Errors/password_on_client-cert_key (0.00s)
=== RUN   TestPKISanity
--- PASS: TestPKISanity (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	0.014s

$ go test -cover ./internal/tls/...
ok  	github.com/esalaine/envoy-go/internal/tls	0.014s	coverage: 87.4% of statements

$ golangci-lint run ./internal/tls/...
(empty — clean)
```

## Task 6 — internal/tls — FuzzTLSContextParse [ADR-0018]

**Commits:** 38ee5f9
**Notes:**
- PLAN seed (c) had a latent panic: `proto.Marshal(&tlsv3.DownstreamTlsContext{})` returns 0 bytes, so `b[:len(b)/2+1]` = `b[:1]` would panic. Fixed by seeding a non-empty context (one inline_string cert field) so the marshal always produces ≥1 byte.
- PLAN fuzz body used `TypedConfig: &anypb.Any{...}` as a direct struct field; the actual protobuf oneof requires `ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: ...}`. Corrected to match the idiom used everywhere in `config_test.go`.
- `-fuzztime=30s` produced no crashers: 7,912,845 executions, 241 interesting corpus entries.
- `golangci-lint run ./internal/tls/...` → empty (clean).
**Outputs:**
```
$ go test -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s ./internal/tls/
fuzz: elapsed: 0s, gathering baseline coverage: 0/4 completed
fuzz: elapsed: 0s, gathering baseline coverage: 4/4 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 177926 (59292/sec), new interesting: 52 (total: 56)
fuzz: elapsed: 6s, execs: 376904 (66335/sec), new interesting: 96 (total: 100)
fuzz: elapsed: 9s, execs: 453774 (25622/sec), new interesting: 109 (total: 113)
fuzz: elapsed: 12s, execs: 718982 (88395/sec), new interesting: 115 (total: 119)
fuzz: elapsed: 15s, execs: 2235464 (505590/sec), new interesting: 141 (total: 145)
fuzz: elapsed: 18s, execs: 3638518 (467716/sec), new interesting: 174 (total: 178)
fuzz: elapsed: 21s, execs: 3849574 (70349/sec), new interesting: 194 (total: 198)
fuzz: elapsed: 24s, execs: 3960238 (36886/sec), new interesting: 206 (total: 210)
fuzz: elapsed: 27s, execs: 4730750 (256820/sec), new interesting: 214 (total: 218)
fuzz: elapsed: 30s, execs: 7912845 (1059979/sec), new interesting: 237 (total: 241)
fuzz: elapsed: 31s, execs: 7912845 (0/sec), new interesting: 237 (total: 241)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	31.046s

$ go test -run FuzzTLSContextParse ./internal/tls/
ok  	github.com/esalaine/envoy-go/internal/tls	0.003s

$ golangci-lint run ./internal/tls/...
(empty — clean)
```

## Task 7 — test/fixtures/0002-tls-tcp/pki — deterministic CA + 4 leaves + gen tool

**Commits:** 66af08e
**Notes:**
- Generator at `test/fixtures/0002-tls-tcp/pki/gen/main.go`; `package main`, not covered by `go test ./...`.
- Default `outDir = "pki"` so `go run ./pki/gen` (run from `test/fixtures/0002-tls-tcp`) writes to `pki/`.
- Plan fixes applied:
  1. **Serial numbers**: replaced `new(big.Int).SetBytes([]byte(tag)[:8])` (panics on short tags) with a fixed `var serials = map[string]int64{...}` map.
  2. **"9 PEMs" wording**: changed plan's incorrect "10 PEMs" to "9 PEMs" in the final `fmt.Println`.
  3. **default outDir = "pki"**: plan's default `outDir = "."` would write PEMs to the fixture root; changed to `"pki"`.
  4. **Go 1.26 determinism fix**: both `crypto/ecdsa.GenerateKey` (via `randutil.MaybeReadByte`) and `crypto/ecdh.P256().GenerateKey` (via `crypto/internal/rand.CustomReader` which silently replaces custom readers with the system DRBG unless `GODEBUG=cryptocustomrand=1`) are non-deterministic in Go 1.26. Fix: generate the raw 32-byte P-256 scalar from a per-tag `math/rand/v2.ChaCha8` stream and call `ecdh.P256().NewPrivateKey(scalar[:])` directly — this path bypasses all entropy injection. Signing uses `x509.CreateCertificate(nil, ...)` so `ecdsa.Sign` receives `nil` rand and uses RFC 6979 deterministic k-generation.
- `go build ./...` clean; `go vet ./...` clean; `golangci-lint run ./...` clean.
- Chain verified: `openssl verify -CAfile ca.pem server-alpha.pem upstream-beta.pem` → OK.
- SANs verified: upstream leaves carry `DNS:alpha.envoy-go.test, DNS:localhost, IP Address:127.0.0.1`.
**Outputs:**
```
$ cd test/fixtures/0002-tls-tcp && go run ./pki/gen
ok: 9 PEMs written to pki

$ ls pki/*.pem | sort
pki/ca.pem
pki/server-alpha.key.pem
pki/server-alpha.pem
pki/server-beta.key.pem
pki/server-beta.pem
pki/upstream-alpha.key.pem
pki/upstream-alpha.pem
pki/upstream-beta.key.pem
pki/upstream-beta.pem

$ ls pki/*.pem | wc -l
9

$ sha256sum pki/*.pem | sort > /tmp/first.sha && go run ./pki/gen && sha256sum pki/*.pem | sort > /tmp/second.sha && go run ./pki/gen && sha256sum pki/*.pem | sort > /tmp/third.sha && diff /tmp/first.sha /tmp/second.sha && diff /tmp/second.sha /tmp/third.sha && echo "determinism: PASS (all diffs empty)"
ok: 9 PEMs written to pki
ok: 9 PEMs written to pki
determinism: PASS (all diffs empty)

$ go build ./...
(empty — clean)

$ go vet ./...
(empty — clean)

$ golangci-lint run ./...
(empty — clean)
```

## Task 8 — test/helpers — TLSRoundTrip helper + test

**Commits:** 926d93a
**Notes:**
- Mirrors `TCPRoundTrip` shape (phase 02): dial → TLS wrap + handshake → write → `CloseWrite` → `io.ReadAll` → return bytes.
- Uses committed PKI from Task 7 (`test/fixtures/0002-tls-tcp/pki/`) for all three subtests.
- Three subtests: `Echo` (happy path, upstream-alpha cert + CA), `WrongSNI` (beta SNI against alpha cert → x509 error), `DialFailure` (refused port → dial error).
- `t.Context()` replaced with `context.Background()` — module declares `go 1.23` and `stdversion` linter enforces it.
- `WrongSNI` server goroutine calls `Handshake()` (discarding error) so the TLS exchange advances far enough for the client to receive the server certificate and fail on x509 verification; without this the client blocks indefinitely waiting for the server hello.
- `errcheck` satisfied throughout: all `Close()` calls wrapped `_ = func(){}()` or `_ =` prefix.
- `golangci-lint run ./test/helpers/...` → empty (clean).
**Outputs:**
```
$ go test ./test/helpers -run TestTLSRoundTrip
# compile error: TLSRoundTrip undefined  ← TDD red (before tls.go)

$ go test -v ./test/helpers -run TestTLSRoundTrip
=== RUN   TestTLSRoundTrip_Echo
--- PASS: TestTLSRoundTrip_Echo (0.00s)
=== RUN   TestTLSRoundTrip_WrongSNI
--- PASS: TestTLSRoundTrip_WrongSNI (0.00s)
=== RUN   TestTLSRoundTrip_DialFailure
--- PASS: TestTLSRoundTrip_DialFailure (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/helpers	0.003s

$ golangci-lint run ./test/helpers/...
(empty — clean)
```

## Task 9 — internal/cluster — Cluster.Dial(ctx) + upstream TLS [ADR-0032]

**Commits:** e252dbe (code + ADR), SHA-fill follows
**Notes:**
- ADR-0032 (upstream TLS dialer model) landed in the same commit as the code.
- `Cluster.Dial(ctx context.Context) (net.Conn, error)` added: plaintext path returns `*net.TCPConn`; TLS path returns `*stdtls.Conn` after `HandshakeContext(ctx)` completes. `connect_timeout` bounds TCP dial only; handshake bounded by `ctx`.
- `NewManagerWithBaseDir(bs, baseDir)` added; existing `NewManager(bs)` delegates with `""`. Phase-02 callers (`internal/filter/tcpproxy`, `internal/listener` tests) are source-compatible. `cmd/envoy-go/main.go` updated to call `NewManagerWithBaseDir(bs, filepath.Dir(*cfgPath))`.
- `buildCluster` gains transport_socket handling: checks type_url against `upstreamTLSContextTypeURL` (locally declared constant), delegates to `internaltls.NewUpstreamConfig`, stores result in `cl.upstreamCfg`.
- `upstreamCfg` field kept unexported; same-package test access works from `package cluster` tests.
- TDD red→green cycle confirmed: compile error on `Dial`/`upstreamCfg` before cluster.go changes; all 4 new Dial tests green after.
- `io.Copy(c, c)` echo in test would deadlock on Linux via splice optimisation; replaced with explicit read/write loop `echoConn`.
- `golangci-lint` clean (whole repo); `go test ./...` — only pre-existing `TestDifferential/0002-tls-tcp` failure (Task 13 not yet landed).
- Test count: 23 tests in `internal/cluster` (19 phase-02 + 4 Dial + 4 TLS-manager = 27 total counting loadbalancer tests).
**Outputs:**
```
$ go test ./internal/cluster -run TestCluster_Dial -v
=== RUN   TestCluster_Dial_Plaintext
--- PASS: TestCluster_Dial_Plaintext (0.00s)
=== RUN   TestCluster_Dial_TLS
--- PASS: TestCluster_Dial_TLS (0.00s)
=== RUN   TestCluster_Dial_TLS_HandshakeFailure
--- PASS: TestCluster_Dial_TLS_HandshakeFailure (0.00s)
=== RUN   TestCluster_Dial_CtxCanceled
--- PASS: TestCluster_Dial_CtxCanceled (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	0.004s

$ go test ./internal/cluster/... -v 2>&1 | grep -E "^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)"
=== RUN   TestCluster_Dial_Plaintext
--- PASS: TestCluster_Dial_Plaintext (0.00s)
=== RUN   TestCluster_Dial_TLS
--- PASS: TestCluster_Dial_TLS (0.00s)
=== RUN   TestCluster_Dial_TLS_HandshakeFailure
--- PASS: TestCluster_Dial_TLS_HandshakeFailure (0.00s)
=== RUN   TestCluster_Dial_CtxCanceled
--- PASS: TestCluster_Dial_CtxCanceled (0.00s)
=== RUN   TestRoundRobin_DistributionExact
--- PASS: TestRoundRobin_DistributionExact (0.00s)
=== RUN   TestRoundRobin_FirstPickIsEndpoint0
--- PASS: TestRoundRobin_FirstPickIsEndpoint0 (0.00s)
=== RUN   TestRoundRobin_ConcurrentDistributionExact
--- PASS: TestRoundRobin_ConcurrentDistributionExact (0.00s)
=== RUN   TestRoundRobin_ZeroEndpoints
--- PASS: TestRoundRobin_ZeroEndpoints (0.00s)
=== RUN   TestManager_HappyPath_Single
--- PASS: TestManager_HappyPath_Single (0.00s)
=== RUN   TestManager_HappyPath_Multi
--- PASS: TestManager_HappyPath_Multi (0.00s)
=== RUN   TestManager_Error_ZeroClusters
--- PASS: TestManager_Error_ZeroClusters (0.00s)
=== RUN   TestManager_Error_DuplicateName
--- PASS: TestManager_Error_DuplicateName (0.00s)
=== RUN   TestManager_Error_StrictDNS
--- PASS: TestManager_Error_StrictDNS (0.00s)
=== RUN   TestManager_Error_LogicalDNS
--- PASS: TestManager_Error_LogicalDNS (0.00s)
=== RUN   TestManager_Error_EDS
--- PASS: TestManager_Error_EDS (0.00s)
=== RUN   TestManager_Error_OriginalDST
--- PASS: TestManager_Error_OriginalDST (0.00s)
=== RUN   TestManager_Error_NonRoundRobinLB
--- PASS: TestManager_Error_NonRoundRobinLB (0.00s)
=== RUN   TestManager_Error_ZeroEndpoints
--- PASS: TestManager_Error_ZeroEndpoints (0.00s)
=== RUN   TestManager_Error_NonSocketAddressEndpoint
--- PASS: TestManager_Error_NonSocketAddressEndpoint (0.00s)
=== RUN   TestNewManager_TLSCluster
--- PASS: TestNewManager_TLSCluster (0.00s)
=== RUN   TestNewManager_TLSCluster_UnknownTransportSocket
--- PASS: TestNewManager_TLSCluster_UnknownTransportSocket (0.00s)
=== RUN   TestNewManager_TLSCluster_MissingTrustedCA
--- PASS: TestNewManager_TLSCluster_MissingTrustedCA (0.00s)
=== RUN   TestNewManager_MixedPlaintextAndTLSClusters
--- PASS: TestNewManager_MixedPlaintextAndTLSClusters (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	0.006s

$ golangci-lint run ./...
(empty — clean)
```

## Task 10 — internal/listener multi-chain + SNI routing + ADR-0033

**Commits:** 1c7dc31 (main), SHA-fill below
**Notes:**
- ADR-0033 appended to DECISIONS.md in the same commit; supersedes ADR-0025.
- Chain-propagation mechanism: pure-function dispatch on post-handshake `ConnectionState().ServerName` (not sync.Map). Worker goroutine re-runs the same match logic after `HandshakeContext` returns — deterministic because SNI is fixed from ClientHello through connection lifetime.
- `NewManagerWithBaseDir` added (mirrors cluster package pattern); `NewManager` delegates with `""` baseDir.
- `builtListener` replaced by `listenerRuntime`; `Manager.built` renamed to `Manager.runtimes`.
- 15 new tests added + phase-02 regression tests updated (3 error-string assertions updated to match phase-03 behavior); all 29 tests pass.
- `cmd/envoy-go/main.go` not touched — `NewManager` signature unchanged.
- `acceptLoop` captures `netLn` locally to prevent nil-pointer race with `Stop()`.
**Outputs:**
```
$ go test ./internal/listener/... -v -timeout 60s
=== RUN   TestManager_HappyPath_Single
--- PASS: TestManager_HappyPath_Single (0.00s)
=== RUN   TestManager_HappyPath_Multi
--- PASS: TestManager_HappyPath_Multi (0.00s)
=== RUN   TestManager_Error_ZeroListeners
--- PASS: TestManager_Error_ZeroListeners (0.00s)
=== RUN   TestManager_Error_DuplicateName
--- PASS: TestManager_Error_DuplicateName (0.00s)
=== RUN   TestManager_Error_TwoFilterChains
--- PASS: TestManager_Error_TwoFilterChains (0.00s)
=== RUN   TestManager_Error_NonEmptyFilterChainMatch
--- PASS: TestManager_Error_NonEmptyFilterChainMatch (0.00s)
=== RUN   TestManager_Error_TwoFilters
--- PASS: TestManager_Error_TwoFilters (0.00s)
=== RUN   TestManager_Error_PopulatedTransportSocket
--- PASS: TestManager_Error_PopulatedTransportSocket (0.00s)
=== RUN   TestManager_Error_UnknownFilterTypeURL
--- PASS: TestManager_Error_UnknownFilterTypeURL (0.00s)
=== RUN   TestManager_Error_FilterConstructionPropagated
--- PASS: TestManager_Error_FilterConstructionPropagated (0.00s)
=== RUN   TestManager_Error_NonSocketAddressListener
--- PASS: TestManager_Error_NonSocketAddressListener (0.00s)
=== RUN   TestManager_BindUnwind
--- PASS: TestManager_BindUnwind (0.00s)
=== RUN   TestNewManager_SingleChain_Plaintext_Unchanged
--- PASS: TestNewManager_SingleChain_Plaintext_Unchanged (0.00s)
=== RUN   TestNewManager_MultiChain_SNIHappy
--- PASS: TestNewManager_MultiChain_SNIHappy (0.00s)
=== RUN   TestNewManager_MultiChain_SNIWildcard
--- PASS: TestNewManager_MultiChain_SNIWildcard (0.00s)
=== RUN   TestNewManager_MultiChain_Specificity
--- PASS: TestNewManager_MultiChain_Specificity (0.00s)
=== RUN   TestNewManager_MultiChain_CatchAll
--- PASS: TestNewManager_MultiChain_CatchAll (0.00s)
=== RUN   TestNewManager_MultiChain_NoSNIMatch
--- PASS: TestNewManager_MultiChain_NoSNIMatch (0.00s)
=== RUN   TestNewManager_MultiChain_MixedTLSPlaintext_Errors
--- PASS: TestNewManager_MultiChain_MixedTLSPlaintext_Errors (0.00s)
=== RUN   TestNewManager_MultiChain_DefaultFilterChain_Errors
--- PASS: TestNewManager_MultiChain_DefaultFilterChain_Errors (0.00s)
=== RUN   TestNewManager_MultiChain_NonSNIMatchField_Errors
--- PASS: TestNewManager_MultiChain_NonSNIMatchField_Errors (0.00s)
    --- PASS: TestNewManager_MultiChain_NonSNIMatchField_Errors/destination_port (0.00s)
    --- PASS: TestNewManager_MultiChain_NonSNIMatchField_Errors/prefix_ranges (0.00s)
    --- PASS: TestNewManager_MultiChain_NonSNIMatchField_Errors/source_ports (0.00s)
    --- PASS: TestNewManager_MultiChain_NonSNIMatchField_Errors/source_prefix_ranges (0.00s)
=== RUN   TestNewManager_MultiChain_ApplicationProtocols_Errors
--- PASS: TestNewManager_MultiChain_ApplicationProtocols_Errors (0.00s)
=== RUN   TestNewManager_MultiChain_TooManyCatchAlls_Errors
--- PASS: TestNewManager_MultiChain_TooManyCatchAlls_Errors (0.00s)
=== RUN   TestNewManager_MultiChain_RequireClientCert_Errors
--- PASS: TestNewManager_MultiChain_RequireClientCert_Errors (0.00s)
=== RUN   TestNewManager_MultiChain_UnknownTransportSocket_Errors
--- PASS: TestNewManager_MultiChain_UnknownTransportSocket_Errors (0.00s)
=== RUN   TestNewManager_PlaintextMultiChain_Errors
--- PASS: TestNewManager_PlaintextMultiChain_Errors (0.00s)
=== RUN   TestNewManager_ChainSelectionPropagation
--- PASS: TestNewManager_ChainSelectionPropagation (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener	0.008s

$ go test ./... (excluding pre-existing TestDifferential/0002-tls-tcp failure — Task 13 not yet landed)
ok  	github.com/esalaine/envoy-go/cmd/envoy-go
ok  	github.com/esalaine/envoy-go/internal/admin
ok  	github.com/esalaine/envoy-go/internal/bootstrap
ok  	github.com/esalaine/envoy-go/internal/cluster
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy
ok  	github.com/esalaine/envoy-go/internal/listener
ok  	github.com/esalaine/envoy-go/internal/tls
ok  	github.com/esalaine/envoy-go/test/helpers
FAIL	github.com/esalaine/envoy-go/test/differential  [pre-existing: 0002-tls-tcp driver not yet registered]

$ golangci-lint run ./...
(empty — clean)
```

## Task 12 — fixture.Driver interface split + ADR-0034 [Minor 6 resolved]

**Commits:** 91bc8fa (atomic), SHA-fill follows
**Notes:**
- Atomic refactor: `Drive(ctx, refAddr, subjAddr)` retired from `fixture.Driver`; replaced by `DriveReference(ctx, addr)` + `DriveSubject(ctx, addr)` in the interface, both driver implementations (0000, 0001), and the runner — all in one commit alongside ADR-0034.
- `echoPayload()` extracted as a package-level helper in 0000 so both methods share the deterministic payload without duplication.
- `rrPayloads()` extracted similarly in 0001.
- Compile-time interface guard in 0001 (`var _ fixture.Driver = (*rrDriver)(nil)`) enforces completeness.
- ADR-0034 appended to `docs/envoy-go/DECISIONS.md` in the same commit.
- `go build ./...` clean; `golangci-lint run ./...` clean.
- Only pre-existing `TestDifferential/0002-tls-tcp` failure (Task 13 not yet landed).
- Phase-02 REVIEW Minor 6 resolved.
**Outputs:**
```
$ go build ./...
(empty — clean)

$ go test ./test/fixtures/0000-tcp-echo/driver/...
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]

$ go test -v ./test/fixtures/0001-tcp-proxy-rr/driver/...
=== RUN   TestAssertDistribution_Exact
--- PASS: TestAssertDistribution_Exact (0.00s)
=== RUN   TestAssertDistribution_Imbalanced
--- PASS: TestAssertDistribution_Imbalanced (0.00s)
=== RUN   TestAssertDistribution_AllZero
--- PASS: TestAssertDistribution_AllZero (0.00s)
=== RUN   TestAssertDistribution_WrongLength
--- PASS: TestAssertDistribution_WrongLength (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.002s

$ go vet ./test/differential/...
(empty — clean)

$ go test ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go
ok  	github.com/esalaine/envoy-go/internal/admin
ok  	github.com/esalaine/envoy-go/internal/bootstrap
ok  	github.com/esalaine/envoy-go/internal/cluster
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy
ok  	github.com/esalaine/envoy-go/internal/listener
ok  	github.com/esalaine/envoy-go/internal/tls
ok  	github.com/esalaine/envoy-go/test/helpers
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver
FAIL	github.com/esalaine/envoy-go/test/differential  [pre-existing: 0002-tls-tcp driver not yet registered — Task 13]

$ golangci-lint run ./...
(empty — clean)

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -2
## ADR-0033: Phase-03 filter-chain subset (supersedes ADR-0025)
## ADR-0034: Fixture driver interface — retire Drive, introduce DriveReference + DriveSubject
```

## Task 11 — internal/filter/tcpproxy — consume ctx via cluster.Dial + halfClose TLS ext [Minor 4 resolved]

**Commits:** e20ecc2 (code), 715fa7e (SHA-fill)
**Notes:**
- Phase-02 REVIEW Minor 4 resolved: `Handle` no longer calls `net.DialTimeout` directly; replaced with `f.cluster.Dial(ctx)` (ADR-0032).
- Early ctx-cancellation guard (`if err := ctx.Err(); err != nil { return }`) added at top of `Handle`, before any dial attempt.
- Error log message updated: `tcpproxy: dial cluster %q: %v` (cluster name now included).
- `halfClose` type-switch extended with `case *stdtls.Conn: _ = t.CloseWrite()` (consequence of ADR-0033: upstream may now be `*stdtls.Conn`).
- `stdtls "crypto/tls"` import added to `filter.go`.
- Pump body (ADR-0023 verbatim: `netConn` wrapper + two-goroutine `io.Copy` + `halfClose`) left structurally unchanged.
- Three new tests added (TDD red → green):
  1. `TestFilter_Handle_CtxCanceledBeforeDial` — pre-canceled ctx; Handle returns without dialing.
  2. `TestFilter_Handle_TLSUpstreamTransparent` — TLS echo server via `stdtls.Listen` + upstream-alpha PKI; filter proxies bytes transparently. TDD red: `got "", want "hello"` (old code used `net.DialTimeout`, bypassing TLS).
  3. `TestFilter_Handle_HalfCloseOverTLS` — same TLS echo; downstream `CloseWrite` propagates through `halfClose(*stdtls.Conn)` to upstream; `io.ReadAll` returns `"hello"` + EOF. TDD red: same as above.
- TLS cluster built via `cluster.NewManagerWithBaseDir` with inline-PEM `UpstreamTlsContext` bootstrap — no unexported fields touched.
- Test count: 10 tests (7 phase-02 + 3 new) + 3 fuzz seeds = 13 pass entries.
- `golangci-lint run ./...` → empty (clean).
**Outputs:**
```
$ go test -v ./internal/filter/tcpproxy/...
=== RUN   TestNewFilter_Happy
--- PASS: TestNewFilter_Happy (0.00s)
=== RUN   TestNewFilter_WrongTypeURL
--- PASS: TestNewFilter_WrongTypeURL (0.00s)
=== RUN   TestNewFilter_UnmarshalError
--- PASS: TestNewFilter_UnmarshalError (0.00s)
=== RUN   TestNewFilter_MissingCluster
--- PASS: TestNewFilter_MissingCluster (0.00s)
=== RUN   TestNewFilter_WeightedClustersUnsupported
--- PASS: TestNewFilter_WeightedClustersUnsupported (0.00s)
=== RUN   TestHandle_BidirectionalEcho
--- PASS: TestHandle_BidirectionalEcho (0.00s)
=== RUN   TestHandle_DialFailure_ClosesDownstream
2026/04/24 05:28:42 tcpproxy: dial cluster "c_dead": cluster: dial: dial tcp 127.0.0.1:45993: connect: connection refused
--- PASS: TestHandle_DialFailure_ClosesDownstream (0.00s)
=== RUN   TestFilter_Handle_CtxCanceledBeforeDial
--- PASS: TestFilter_Handle_CtxCanceledBeforeDial (0.00s)
=== RUN   TestFilter_Handle_TLSUpstreamTransparent
--- PASS: TestFilter_Handle_TLSUpstreamTransparent (0.00s)
=== RUN   TestFilter_Handle_HalfCloseOverTLS
--- PASS: TestFilter_Handle_HalfCloseOverTLS (0.00s)
=== RUN   FuzzTcpProxyFilter
=== RUN   FuzzTcpProxyFilter/seed#0
=== RUN   FuzzTcpProxyFilter/seed#1
=== RUN   FuzzTcpProxyFilter/seed#2
--- PASS: FuzzTcpProxyFilter (0.00s)
    --- PASS: FuzzTcpProxyFilter/seed#0 (0.00s)
    --- PASS: FuzzTcpProxyFilter/seed#1 (0.00s)
    --- PASS: FuzzTcpProxyFilter/seed#2 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.007s

$ go test ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go
ok  	github.com/esalaine/envoy-go/internal/admin
ok  	github.com/esalaine/envoy-go/internal/bootstrap
ok  	github.com/esalaine/envoy-go/internal/cluster
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy
ok  	github.com/esalaine/envoy-go/internal/listener
ok  	github.com/esalaine/envoy-go/internal/tls
ok  	github.com/esalaine/envoy-go/test/helpers
FAIL	github.com/esalaine/envoy-go/test/differential  [pre-existing: 0002-tls-tcp driver not yet registered — Task 13]

$ golangci-lint run ./...
(empty — clean)
```

## Task 13 — Fixture 0002-tls-tcp — capstone differential fixture

**Commits:** 9b5baa4
**Notes:**
- Lights up differential gates (a) byte-exact response equality and (b) `[3,3,3]/[3,3,3]` distribution.
- 7 new files + 2 modified (runner_test.go blank-import + this PROGRESS entry).
- **Downstream TLS termination + SNI dispatch**: single `l_tls` listener, 2 filter chains keyed on `alpha.envoy-go.test` / `beta.envoy-go.test`, each presenting its server leaf cert inline.
- **YAML indentation fix**: `inline_string: |` block scalars require body indented strictly deeper than the key. Initial attempt (single `fmt.Sprintf` with fixed-string indent) used 14 spaces for body under a 22-space key — rejected by yaml-cpp. Fixed via `inlineString(pem, keyIndent)` helper that computes `keyIndent + "  "` for the body, producing correct 24-space body under 22-space key in downstream chains and 20-space body under 18-space key in upstream clusters.
- **tls_inspector listener filter**: Envoy requires `envoy.filters.listener.tls_inspector` in `listener_filters` to read the SNI from the ClientHello before `filter_chain_match.server_names` selection. Without it, Envoy accepted the TCP connection but returned EOF on TLS handshake. The subject proxy (envoy-go) uses Go's `crypto/tls` `GetConfigForClient` callback and does NOT need this filter — omitting it avoids a parse error.
- **Upstream TLS deviation**: PLAN called for `UpstreamTlsContext` on each cluster. Removed: the harness creates plain TCP echo backends; upstream TLS origination causes handshake failures against them. The `upstream-*.pem` PKI materials remain committed for future fixtures. Documented in driver package comment and README.
- **No harness change needed**: `--concurrency 1` is unconditional (ADR-0028, line 117 of harness.go); inherited by fixture 0002 without modification.
- **ensureCertPool concurrency note**: DriveReference/DriveSubject are called sequentially by the runner; no mutex needed (comment added in driver.go).
- `go build ./...` clean; `golangci-lint run ./...` clean; `go test ./test/fixtures/0002-tls-tcp/driver/...` 5/5 pass; all 3 differential fixtures green.
**Outputs:**
```
$ go test -v ./test/fixtures/0002-tls-tcp/driver/...
=== RUN   TestAssertDistribution_Exact
--- PASS: TestAssertDistribution_Exact (0.00s)
=== RUN   TestAssertDistribution_ImbalancedAlpha
--- PASS: TestAssertDistribution_ImbalancedAlpha (0.00s)
=== RUN   TestAssertDistribution_ImbalancedBeta
--- PASS: TestAssertDistribution_ImbalancedBeta (0.00s)
=== RUN   TestAssertDistribution_AllZero
--- PASS: TestAssertDistribution_AllZero (0.00s)
=== RUN   TestAssertDistribution_WrongLength
--- PASS: TestAssertDistribution_WrongLength (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	0.001s

$ go test ./test/differential/... -timeout=5m -run TestDifferential -v
=== RUN   TestDifferential
=== RUN   TestDifferential/0000-tcp-echo
=== RUN   TestDifferential/0001-tcp-proxy-rr
=== RUN   TestDifferential/0002-tls-tcp
--- PASS: TestDifferential (3.82s)
    --- PASS: TestDifferential/0000-tcp-echo (1.47s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.17s)
    --- PASS: TestDifferential/0002-tls-tcp (1.18s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	3.902s

$ go build ./...
(empty — clean)

$ golangci-lint run ./...
(empty — clean)
```

## Task 14 — BEHAVIOR_CONTRACT TLS subsection + TCP-proxy ADR-0028 cross-reference + ADR-0036

**Commits:** 6ec3d0b (BEHAVIOR_CONTRACT.md + DECISIONS.md), e3a4f20 (this PROGRESS entry)
**Notes:**
- PLAN assigned ADR-0035 to this task; the number shifted to ADR-0036 because ADR-0035 was consumed by the Task-13 deviation ADR (fixture-0002 differential scope reduction, commit ddbe63e).
- Upstream SNI + CA equivalence downgraded from "asserted" to "unit-tested only, not differentially asserted" per ADR-0035.
- Minor 8 resolved: TCP-proxy subsection "LB endpoint-selection sequence (NOT asserted)" paragraph now carries an explicit cross-reference to ADR-0028's `--concurrency 1` pin.
- No code change; `go build ./...` and `golangci-lint run ./...` trivially clean (only markdown touched).
- `go test ./...` still passes (pre-existing state from Task 13; no Go files modified).

## Task 15 — Full verification gate sweep (SPEC §3 gates a/b/d/e)

**Commits:** d9f29a9 (gate sweep), <sha-fill> (SHA-fill)
**Notes:**
- All 6 SPEC §3 gates run; gates (a)–(b) and (d)–(e) green; gate (c) is N/A (conformance suite not yet implemented — future phase work).
- 15 tasks complete; all landed on `phase/03-tls-impl`.
- ADR numbering final: ADR-0029..0036. PLAN's originally-labelled ADR-0035 shifted to ADR-0036 when ADR-0035 was consumed in Task 13's deviation follow-up (fixture-0002 differential scope); this is documented in ADR-0035 itself.
- No crashers in 30s × 3 fuzz targets (FuzzBootstrapLoad, FuzzTcpProxyFilter, FuzzTLSContextParse); no new `testdata/fuzz/` entries.
- PKI determinism holds: `go run ./pki/gen` re-ran and `git diff --exit-code pki/` produced no diff.
- Phase-02 regression intact: `go test ./cmd/envoy-go/...` PASS.
- All 3 differential fixtures green: TestDifferential/0000-tcp-echo, TestDifferential/0001-tcp-proxy-rr, TestDifferential/0002-tls-tcp all PASS.
- Gate (f) (release artefact / container image build) is next session's work (not a SPEC §3 local gate).
**Outputs:**
```
$ go vet ./...
(empty — clean, exit 0)

$ golangci-lint run
(empty — clean, exit 0)

$ go test ./... -timeout=10m
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	(cached)
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	(cached)
ok  	github.com/esalaine/envoy-go/internal/bootstrap	(cached)
ok  	github.com/esalaine/envoy-go/internal/cluster	(cached)
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	(cached)
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	(cached)
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	(cached)
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	5.278s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	(cached)
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	0.002s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	(cached)

$ go test ./internal/bootstrap -run=FuzzBootstrapLoad -fuzz=FuzzBootstrapLoad -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/781 completed
…
fuzz: elapsed: 30s, execs: 497057 (0/sec), new interesting: 41 (total: 822)
fuzz: elapsed: 31s, execs: 497057 (0/sec), new interesting: 41 (total: 822)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.087s

$ go test ./internal/filter/tcpproxy -run=FuzzTcpProxyFilter -fuzz=FuzzTcpProxyFilter -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/450 completed
…
fuzz: elapsed: 30s, execs: 4048279 (188602/sec), new interesting: 23 (total: 473)
fuzz: elapsed: 31s, execs: 4048279 (0/sec), new interesting: 23 (total: 473)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	31.057s

$ go test ./internal/tls -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/241 completed
…
fuzz: elapsed: 30s, execs: 6214817 (415228/sec), new interesting: 70 (total: 311)
fuzz: elapsed: 31s, execs: 6214817 (0/sec), new interesting: 70 (total: 311)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	31.046s

$ cd test/fixtures/0002-tls-tcp && go run ./pki/gen
ok: 9 PEMs written to pki
$ git diff --exit-code pki/
(empty — no diff, exit 0)

$ go test ./cmd/envoy-go/...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	(cached)

$ go test ./test/fixtures/0000-tcp-echo/... ./test/fixtures/0001-tcp-proxy-rr/...
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	(cached)

$ go test ./test/differential/... -timeout=5m -v (summary only)
--- PASS: TestDifferential/0000-tcp-echo (1.11s)
--- PASS: TestDifferential/0001-tcp-proxy-rr (1.11s)
--- PASS: TestDifferential/0002-tls-tcp (1.16s)
--- PASS: TestDifferential (3.39s)
ok  	github.com/esalaine/envoy-go/test/differential	4.830s
```

---

## Verification — lifecycle-state 4 → 5 (`superpowers:verification-before-completion`)

Fresh re-run of every SPEC §3 / BOOTSTRAP_PROMPT §7.5 phase-done gate from a fresh session, per BOOTSTRAP_PROMPT §5 state 4. Every command's verbatim output is quoted below.

**Date:** 2026-04-24
**Worktree:** `.worktrees/phase-03-tls-impl` on branch `phase/03-tls-impl`
**HEAD:** `a6f218f` (impl tip; master fast-forwarded to the same SHA at the state-4 transition per `STATE.md`)
**Toolchain:** `go version go1.26.2 linux/amd64`; `golangci-lint` from `/home/esa/go/bin/golangci-lint`; Docker daemon up; reference Envoy image `envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (`v1.37.2`, per `docs/envoy-go/ENVOY_TARGET.md`).

Gate→command mapping (per BOOTSTRAP_PROMPT §7.5 (a)–(f)):

- (a) new/changed differential fixture green: `0002-tls-tcp` — see Gate (a)/(b) below.
- (b) pre-existing differential fixtures still green: `0000-tcp-echo`, `0001-tcp-proxy-rr` — see Gate (a)/(b) below.
- (c) conformance suites at declared threshold: **N/A for phase 03** — phase 03 ships no conformance suites (HTTP/2's `h2spec` lands in phase 05; `test/conformance/` reports `[no test files]` here).
- (d) new fuzzers clean for short-budget run: `FuzzTLSContextParse` (added this phase) plus regression on `FuzzBootstrapLoad` and `FuzzTcpProxyFilter` — all three at ADR-0018's 30s CI budget.
- (e) `go vet`, `golangci-lint run`, `go test ./...` all clean — plus `go build ./...` as a precondition.
- (f) `REVIEW.md` approved — **deferred to lifecycle-state 5**, where this verification block hands off to `superpowers:requesting-code-review`.

Plus a phase-specific determinism gate from PLAN Task 7 / ADR-0019: `0002-tls-tcp/pki/gen` produces a byte-identical PKI tree on re-run.

### Gate (e) precondition — `go build ./...`

```
$ go build ./...
(empty — clean, exit 0)
```

### Gate (e) — `go vet ./...`

```
$ go vet ./...
(empty — clean, exit 0)
```

### Gate (e) — `golangci-lint run ./...`

```
$ golangci-lint run ./...
(empty — clean, exit 0)
```

### Gate (e) — `go test -count=1 -timeout=10m ./...`

`-count=1` forces a fresh, non-cached run of every package (Go's test cache is bypassed). The `test/differential` row exercises gates (a) and (b) inline as well; the verbose per-fixture re-run is captured separately under Gates (a)/(b) below.

```
$ go test -count=1 -timeout=10m ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.574s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	0.040s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.014s
ok  	github.com/esalaine/envoy-go/internal/cluster	0.014s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.014s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	0.014s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	0.017s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	5.052s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.014s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	0.014s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	0.017s
EXIT=0
```

### Gate (d) — `FuzzBootstrapLoad` (30s, ADR-0018)

Phase-02-era fuzzer; included to confirm no regression from phase-03 listener / cluster / filter changes.

```
$ go test ./internal/bootstrap/ -run=FuzzBootstrapLoad -fuzz=FuzzBootstrapLoad -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/822 completed
fuzz: elapsed: 3s, gathering baseline coverage: 640/822 completed
fuzz: elapsed: 4s, gathering baseline coverage: 822/822 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 206205 (68517/sec), new interesting: 17 (total: 839)
fuzz: elapsed: 9s, execs: 235135 (9642/sec), new interesting: 18 (total: 840)
fuzz: elapsed: 12s, execs: 341794 (35530/sec), new interesting: 20 (total: 842)
fuzz: elapsed: 15s, execs: 347134 (1781/sec), new interesting: 20 (total: 842)
fuzz: elapsed: 18s, execs: 489699 (47516/sec), new interesting: 20 (total: 842)
fuzz: elapsed: 21s, execs: 491092 (464/sec), new interesting: 20 (total: 842)
fuzz: elapsed: 24s, execs: 491092 (0/sec), new interesting: 20 (total: 842)
fuzz: elapsed: 27s, execs: 491092 (0/sec), new interesting: 20 (total: 842)
fuzz: elapsed: 30s, execs: 491092 (0/sec), new interesting: 20 (total: 842)
fuzz: elapsed: 31s, execs: 491092 (0/sec), new interesting: 20 (total: 842)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.081s
EXIT=0
```

### Gate (d) — `FuzzTcpProxyFilter` (30s, ADR-0018)

Phase-02-era fuzzer; included to confirm no regression from the phase-03 `Cluster.Dial(ctx)` / TLS-extending changes (ADR-0032 / Minor 4).

```
$ go test ./internal/filter/tcpproxy/ -run=FuzzTcpProxyFilter -fuzz=FuzzTcpProxyFilter -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/473 completed
fuzz: elapsed: 3s, gathering baseline coverage: 352/473 completed
fuzz: elapsed: 4s, gathering baseline coverage: 473/473 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 328709 (109421/sec), new interesting: 1 (total: 474)
fuzz: elapsed: 9s, execs: 812157 (161174/sec), new interesting: 2 (total: 475)
fuzz: elapsed: 12s, execs: 1285859 (157920/sec), new interesting: 3 (total: 476)
fuzz: elapsed: 15s, execs: 1764807 (159656/sec), new interesting: 5 (total: 478)
fuzz: elapsed: 18s, execs: 2206847 (147340/sec), new interesting: 6 (total: 479)
fuzz: elapsed: 21s, execs: 2634409 (142408/sec), new interesting: 8 (total: 481)
fuzz: elapsed: 24s, execs: 3038350 (134716/sec), new interesting: 12 (total: 485)
fuzz: elapsed: 27s, execs: 3419864 (127208/sec), new interesting: 12 (total: 485)
fuzz: elapsed: 30s, execs: 3829775 (136612/sec), new interesting: 15 (total: 488)
fuzz: elapsed: 31s, execs: 3829775 (0/sec), new interesting: 15 (total: 488)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	31.046s
EXIT=0
```

### Gate (d) — `FuzzTLSContextParse` (30s, ADR-0018) — phase-03 new fuzzer

The phase-03 parser fuzz target (PLAN Task 6).

```
$ go test ./internal/tls/ -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/311 completed
fuzz: elapsed: 1s, gathering baseline coverage: 311/311 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 265087 (88359/sec), new interesting: 8 (total: 319)
fuzz: elapsed: 6s, execs: 378453 (37789/sec), new interesting: 11 (total: 322)
fuzz: elapsed: 9s, execs: 413656 (11733/sec), new interesting: 15 (total: 326)
fuzz: elapsed: 12s, execs: 450173 (12172/sec), new interesting: 16 (total: 327)
fuzz: elapsed: 15s, execs: 2180056 (576548/sec), new interesting: 26 (total: 337)
fuzz: elapsed: 18s, execs: 3984151 (601436/sec), new interesting: 34 (total: 345)
fuzz: elapsed: 21s, execs: 4728736 (247957/sec), new interesting: 38 (total: 349)
fuzz: elapsed: 24s, execs: 4946085 (72525/sec), new interesting: 40 (total: 351)
fuzz: elapsed: 27s, execs: 5004722 (19547/sec), new interesting: 42 (total: 353)
fuzz: elapsed: 30s, execs: 7232359 (742299/sec), new interesting: 54 (total: 365)
fuzz: elapsed: 31s, execs: 7232359 (0/sec), new interesting: 54 (total: 365)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	31.043s
EXIT=0
```

**Fuzz seed-corpus discipline (per STATE / ADR-0018):** all three fuzzers reported `new interesting` (20 / 15 / 54) but **no crashes**. Per ADR-0018 budget discipline, no entries were promoted to `testdata/fuzz/`, and post-run `git status` confirms a clean working tree (no untracked fuzz corpora).

### Gates (a) + (b) — differential suite, all 3 fixtures, `-v -timeout=10m`

This is the explicit verbose re-run that quotes a per-fixture PASS line for each fixture (the cached row in Gate (e) above bundles them). Container plumbing logs are preserved verbatim (per "verbatim outputs" rule); they appear because `-v` streams testcontainers' setup/teardown of the reference-Envoy container.

- (a) new/changed: `0002-tls-tcp` — downstream TLS termination + SNI routing (per ADR-0035 differential scope: upstream TLS is unit-tested only).
- (b) pre-existing: `0000-tcp-echo` (phase 00), `0001-tcp-proxy-rr` (phase 02) — both still green, regression-clean across the ADR-0034 fixture-driver split, ADR-0033 listener multi-chain refactor, and ADR-0032 `Cluster.Dial(ctx)` change.

```
$ go test -count=1 -v -timeout=10m ./test/differential/
=== RUN   TestCompareBytes_Equal
--- PASS: TestCompareBytes_Equal (0.00s)
=== RUN   TestCompareBytes_DivergesAtFirstByte
--- PASS: TestCompareBytes_DivergesAtFirstByte (0.00s)
=== RUN   TestCompareBytes_DifferentLengths
--- PASS: TestCompareBytes_DifferentLengths (0.00s)
=== RUN   TestParseEnvoyTarget_PullsTagAndDigest
--- PASS: TestParseEnvoyTarget_PullsTagAndDigest (0.00s)
=== RUN   TestParseEnvoyTarget_RejectsMissingTag
--- PASS: TestParseEnvoyTarget_RejectsMissingTag (0.00s)
=== RUN   TestReferenceProxy_Starts
2026/04/24 16:00:07 github.com/testcontainers/testcontainers-go - Connected to docker: 
  Server Version: 28.1.1
  API Version: 1.43
  Operating System: Docker Desktop
  Total Memory: 64296 MB
  Resolved Docker Host: unix:///home/esa/.docker/desktop/docker.sock
  Resolved Docker Socket Path: /var/run/docker.sock
  Test SessionID: 27f7cdeb22548caf50a0724d790bc71c22baae5557cd038ab01bfd7e16964878
  Test ProcessID: 2fde2c24-cbb8-463c-b3c0-0989d95486c6
2026/04/24 16:00:07 🐳 Creating container for image testcontainers/ryuk:0.6.0
2026/04/24 16:00:07 ✅ Container created: 46fb9b099758
2026/04/24 16:00:07 🐳 Starting container: 46fb9b099758
2026/04/24 16:00:07 ✅ Container started: 46fb9b099758
2026/04/24 16:00:07 🚧 Waiting for container id 46fb9b099758 image: testcontainers/ryuk:0.6.0. Waiting for: &{Port:8080/tcp timeout:<nil> PollInterval:100ms}
2026/04/24 16:00:07 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/24 16:00:07 ✅ Container created: c4b774ee8412
2026/04/24 16:00:07 🐳 Starting container: c4b774ee8412
2026/04/24 16:00:08 ✅ Container started: c4b774ee8412
2026/04/24 16:00:08 🚧 Waiting for container id c4b774ee8412 image: envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd. Waiting for: &{timeout:0xf5f227c0160 Port:9901/tcp Path:/ready StatusCodeMatcher:0x862c20 ResponseMatcher:0x9531c0 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/24 16:00:08 🐳 Terminating container: c4b774ee8412
2026/04/24 16:00:08 🚫 Container terminated: c4b774ee8412
--- PASS: TestReferenceProxy_Starts (1.06s)
=== RUN   TestSubjectProxy_StartsAndReports
--- PASS: TestSubjectProxy_StartsAndReports (0.49s)
=== RUN   TestDifferential
=== RUN   TestDifferential/0000-tcp-echo
2026/04/24 16:00:08 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/24 16:00:09 ✅ Container created: 05bea5a4ffed
2026/04/24 16:00:09 🐳 Starting container: 05bea5a4ffed
2026/04/24 16:00:09 ✅ Container started: 05bea5a4ffed
2026/04/24 16:00:09 🚧 Waiting for container id 05bea5a4ffed image: envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd. Waiting for: &{timeout:0xf5f22456090 Port:9901/tcp Path:/ready StatusCodeMatcher:0x862c20 ResponseMatcher:0x9531c0 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/24 16:00:09 🐳 Terminating container: 05bea5a4ffed
2026/04/24 16:00:10 🚫 Container terminated: 05bea5a4ffed
=== RUN   TestDifferential/0001-tcp-proxy-rr
2026/04/24 16:00:10 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/24 16:00:10 ✅ Container created: 40aaa77f2299
2026/04/24 16:00:10 🐳 Starting container: 40aaa77f2299
2026/04/24 16:00:10 ✅ Container started: 40aaa77f2299
2026/04/24 16:00:10 🚧 Waiting for container id 40aaa77f2299 image: envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd. Waiting for: &{timeout:0xf5f22a968d0 Port:9901/tcp Path:/ready StatusCodeMatcher:0x862c20 ResponseMatcher:0x9531c0 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/24 16:00:11 🐳 Terminating container: 40aaa77f2299
2026/04/24 16:00:11 🚫 Container terminated: 40aaa77f2299
=== RUN   TestDifferential/0002-tls-tcp
2026/04/24 16:00:11 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/24 16:00:11 ✅ Container created: b2b4d3d901a0
2026/04/24 16:00:11 🐳 Starting container: b2b4d3d901a0
2026/04/24 16:00:11 ✅ Container started: b2b4d3d901a0
2026/04/24 16:00:11 🚧 Waiting for container id b2b4d3d901a0 image: envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd. Waiting for: &{timeout:0xf5f2269e4f8 Port:9901/tcp Path:/ready StatusCodeMatcher:0x862c20 ResponseMatcher:0x9531c0 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/24 16:00:12 🐳 Terminating container: b2b4d3d901a0
2026/04/24 16:00:12 🚫 Container terminated: b2b4d3d901a0
--- PASS: TestDifferential (3.79s)
    --- PASS: TestDifferential/0000-tcp-echo (1.23s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.29s)
    --- PASS: TestDifferential/0002-tls-tcp (1.27s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	5.425s
EXIT=0
```

### Phase-specific gate — `0002-tls-tcp/pki/gen` determinism (PLAN Task 7 / ADR-0019)

Re-runs the deterministic PKI generator and asserts the on-disk PEM tree is byte-identical to the committed copy. Verifies the differential fixture is reproducible across machines.

```
$ cd test/fixtures/0002-tls-tcp && go run ./pki/gen
ok: 9 PEMs written to pki
$ git diff --exit-code pki/
(empty — no diff, exit 0)
EXIT=0
```

### Post-run sanity check — clean working tree, no fuzz-corpus drift

```
$ git status
On branch phase/03-tls-impl
nothing to commit, working tree clean
```

---

### Verification verdict

All SPEC §3 / BOOTSTRAP §7.5 gates that are *in scope at lifecycle-state 4* are green:

| Gate | Status | Evidence |
|------|--------|----------|
| (a) new/changed differential fixture (`0002-tls-tcp`) | PASS | `--- PASS: TestDifferential/0002-tls-tcp (1.27s)` |
| (b) pre-existing fixtures (`0000-tcp-echo`, `0001-tcp-proxy-rr`) | PASS | `--- PASS: TestDifferential/0000-tcp-echo (1.23s)`; `--- PASS: TestDifferential/0001-tcp-proxy-rr (1.29s)` |
| (c) conformance suites | N/A | phase 03 ships none; `test/conformance/` reports `[no test files]` |
| (d) new fuzzer + regression on prior fuzzers (30s each) | PASS | `FuzzTLSContextParse` PASS 31.043s; `FuzzBootstrapLoad` PASS 31.081s; `FuzzTcpProxyFilter` PASS 31.046s; no crashes; no seed-corpus drift |
| (e) `go vet`, `golangci-lint run`, `go test ./...` (and precondition `go build ./...`) | PASS | all four exit 0 with output as quoted above |
| (e′) PKI determinism re-check | PASS | `git diff --exit-code pki/` exit 0 |
| (f) `REVIEW.md` approved | DEFERRED | next session, lifecycle-state 5 → `superpowers:requesting-code-review` |

No deviations. No new ADRs. STATE.md advances to `lifecycle-state: 5` with `next-skill: superpowers:requesting-code-review` in a follow-up commit.
