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
