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
