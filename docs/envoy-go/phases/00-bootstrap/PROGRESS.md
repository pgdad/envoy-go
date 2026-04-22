# Phase 00 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim.

## Preamble — execution preconditions

The executor verified PLAN.md's "Execution preconditions" section (lines 84–94) at session start:

- Worktree: `.worktrees/phase-00-bootstrap-impl` on branch `phase/00-bootstrap-impl` off `master`. ✓ created.
- Docker: `docker version` reports client v28.4.0 / Docker Desktop 4.41.2 server. ✓
- Go: `go version go1.26.2 linux/amd64` (above the 1.23 floor set in SPEC §10 #3). ✓
- `golangci-lint`: PLAN's pinned version `v1.55.2` **failed to install** via `go install` on Go 1.26.2 with:
  ```
  ../../go/pkg/mod/golang.org/x/tools@v0.14.0/internal/tokeninternal/tokeninternal.go:78:9: invalid array length -delta * delta (constant -256 of type int64)
  ```
  Root cause: `golangci-lint@v1.55.2` transitively pins `golang.org/x/tools@v0.14.0`, whose `tokeninternal.go:78` uses a const-array-length idiom that Go 1.22+ rejects under stricter constant-arithmetic rules. This affects every Go version ≥ 1.22 — including PLAN's own 1.23 floor — so the PLAN precondition as written is internally inconsistent.

  Resolution: installed `v1.64.8` via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`. v1.64.8 is the last v1.x stable release; its config format matches what Task 5 writes and it supports every SPEC §5.5 baseline linter. The pin bump is to be codified in **ADR-0009** by Task 5 (superseding PLAN's precondition step 4 and PLAN's Task 15 CI install command). PLAN's reserved "ADR-0009 for Task 16 runner fallback" shifts to ADR-0010 if that contingency materializes.

## Task 1 — Initialize Go module

**Commits:** f31501f
**Notes:** Created `go.mod` with `module github.com/esalaine/envoy-go` and `go 1.23` directives. `go mod tidy && go build ./...` run successfully (no-op build, no source files yet).
**Outputs:**
```
$ go mod tidy && go build ./...
go: warning: "all" matched no packages
go: warning: "./..." matched no packages
```

## Task 2 — ADR-0005, ADR-0006

**Commits:** 31172f1
**Notes:** Appended ADR-0005 (autonomous-planning adaptation) and ADR-0006 (module path `github.com/esalaine/envoy-go`) to DECISIONS.md verbatim from PLAN.md Task 2. No command outputs to quote (pure Markdown append).

## Task 3 — internal/ package placeholders

**Commits:** f2e4576
**Notes:** Created 12 placeholder doc.go files under internal/{bootstrap,listener,cluster,tcp,http,tls,filter,xds,admin,stats,accesslog,runtime}/ per SPEC §4 future-phase mapping. Each file contains only the package-doc comment and the `package <name>` line.
**Outputs:**
```
$ go vet ./internal/...
```

## Task 4 — Envoy pin, ENVOY_TARGET.md, ADR-0008

**Commits:** 0819740
**Notes:** Pinned upstream Envoy at `envoyproxy/envoy:v1.37.2` (SHA256 `c5e8a68e52f4d`). Smoke-tested admin `/ready` on the pulled image. Updated ENVOY_TARGET.md with tag, SHA, release-notes URL, and the refresh procedure. Appended ADR-0008. Note: ADR-0007 slot is intentionally left empty here; Task 6 lands ADR-0007 (minimal YAML schema) and the controller will address physical ordering inside DECISIONS.md at that point.
**Outputs:**
```
$ docker pull envoyproxy/envoy:v1.37.2
v1.37.2: Pulling from envoyproxy/envoy
4f4fb700ef54: Pulling fs layer
e2dbee44c34b: Pulling fs layer
bd23ccda478c: Pulling fs layer
b86abf6ed0de: Pulling fs layer
342cd4258481: Pulling fs layer
54bcd46fe54a: Pulling fs layer
c547fb821bb9: Pulling fs layer
de47083ed7d7: Pulling fs layer
541e4430a844: Pulling fs layer
4f4fb700ef54: Already exists
4f4fb700ef54: Pull complete
541e4430a844: Download complete
e2dbee44c34b: Download complete
c547fb821bb9: Download complete
bd23ccda478c: Download complete
b86abf6ed0de: Download complete
54bcd46fe54a: Download complete
de47083ed7d7: Download complete
de47083ed7d7: Pull complete
e2dbee44c34b: Pull complete
bd23ccda478c: Pull complete
54bcd46fe54a: Pull complete
541e4430a844: Pull complete
c547fb821bb9: Pull complete
b86abf6ed0de: Pull complete
342cd4258481: Download complete
342cd4258481: Pull complete
Digest: sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
Status: Downloaded newer image for envoyproxy/envoy:v1.37.2
docker.io/envoyproxy/envoy:v1.37.2

$ docker inspect --format='{{index .RepoDigests 0}}' envoyproxy/envoy:v1.37.2
envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd

$ curl -fsS http://127.0.0.1:9901/ready
LIVE
```

## Task 5 — golangci-lint config (.golangci.yml)

**Commits:** 1a57bd3
**Notes:** Created .golangci.yml with SPEC §5.5 baseline linters (govet, errcheck, staticcheck, unused, ineffassign, gofmt, goimports, misspell, revive), plus linter settings for goimports local-prefixes (github.com/esalaine/envoy-go), misspell US locale, and revive rules (package-comments, exported). Note on golangci-lint version pin: PLAN's precondition pinned v1.55.2, which fails to install via `go install` on Go 1.22+ due to x/tools@v0.14.0 incompatibility (full root cause in this file's preamble). Local tooling is v1.64.8 (latest v1.x). ADR-0009 codifying the v1.64.8 pin is deferred to Task 15 (CI config — where the install command is codified). Task 5's YAML is version-agnostic across the v1.x series.
**Outputs:**
```
$ golangci-lint run ./...
<empty>
```

## Task 6 — minimal config schema + parser

**Commits:** 9756b78
**Notes:** Created cmd/envoy-go/config.go + config_test.go implementing the minimal YAML schema per ADR-0007 (listener.{address,port}, upstream.{address,port}; unknown fields rejected). TDD: wrote tests first, confirmed RED (undefined: loadConfig), then implemented. Added gopkg.in/yaml.v3@v3.0.1 dependency. Appended ADR-0007 to DECISIONS.md (physical order: after ADR-0008 in the file; the ADR numbers are authoritative, not the file position — per the append-only doctrine in BOOTSTRAP_PROMPT §4.1 invariant 4). Lint fixes applied: added package comment to config.go (revive package-comments rule), ran gofmt on config_test.go (alignment in map literal).
**Outputs:**
```
$ go test ./cmd/envoy-go/ -run TestLoadConfig
# github.com/esalaine/envoy-go/cmd/envoy-go [github.com/esalaine/envoy-go/cmd/envoy-go.test]
cmd/envoy-go/config_test.go:17:14: undefined: loadConfig
cmd/envoy-go/config_test.go:40:17: undefined: loadConfig
cmd/envoy-go/config_test.go:53:15: undefined: loadConfig
FAIL	github.com/esalaine/envoy-go/cmd/envoy-go [build failed]
FAIL

$ go test ./cmd/envoy-go/ -run TestLoadConfig -v
=== RUN   TestLoadConfig_Valid
--- PASS: TestLoadConfig_Valid (0.00s)
=== RUN   TestLoadConfig_RejectsMissingFields
=== RUN   TestLoadConfig_RejectsMissingFields/missing_listener
=== RUN   TestLoadConfig_RejectsMissingFields/missing_upstream
=== RUN   TestLoadConfig_RejectsMissingFields/missing_listener_address
=== RUN   TestLoadConfig_RejectsMissingFields/port_zero
--- PASS: TestLoadConfig_RejectsMissingFields (0.00s)
    --- PASS: TestLoadConfig_RejectsMissingFields/missing_listener (0.00s)
    --- PASS: TestLoadConfig_RejectsMissingFields/missing_upstream (0.00s)
    --- PASS: TestLoadConfig_RejectsMissingFields/missing_listener_address (0.00s)
    --- PASS: TestLoadConfig_RejectsMissingFields/port_zero (0.00s)
=== RUN   TestLoadConfig_RejectsUnknownFields
--- PASS: TestLoadConfig_RejectsUnknownFields (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.001s
```

**Follow-up:** ran `go mod tidy` post-Task 6 to reclassify gopkg.in/yaml.v3 as a direct dependency and add the missing `h1:` hash line for `gopkg.in/check.v1`. Commit: e335ce7.

## Task 7 — subject TCP-pump binary + TCP test helpers

**Commits:** d733e08
**Notes:** Implemented `cmd/envoy-go/main.go` (flag parse, config load, TCP listen+accept loop, per-conn io.Copy pump, ready-sentinel stdout contract) and `test/helpers/tcp.go` (TCPRoundTrip reusable helper). Full TDD: helpers test RED → GREEN; main test RED → GREEN. Full test suite PASS, lint clean.

**Linux splice(2) deviation from PLAN verbatim:** The plan's `io.Copy(upstream, client)` / `io.Copy(client, upstream)` between two `*net.TCPConn` values triggers Go's `splice(2)` optimisation on Linux. `splice(fd, fd)` (same socket as both source and destination) returns 0 bytes, causing the echo backend's data to be silently dropped. Fix: introduce `netConn struct{ net.Conn }` wrapper in `main.go` (hiding the concrete `*net.TCPConn` type so `io.Copy` falls back to a 32 KiB heap-buffer loop) and `echoConn struct{ net.Conn }` in `main_test.go` for the same reason in `acceptEcho`. All other logic is verbatim from the PLAN.
**Outputs:**
```
$ go test ./test/helpers/        # RED
# github.com/esalaine/envoy-go/test/helpers [github.com/esalaine/envoy-go/test/helpers.test]
test/helpers/tcp_test.go:29:15: undefined: TCPRoundTrip
FAIL	github.com/esalaine/envoy-go/test/helpers [build failed]
FAIL

$ go test ./test/helpers/ -v     # GREEN
=== RUN   TestTCPRoundTrip_EchoBackend
--- PASS: TestTCPRoundTrip_EchoBackend (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s

$ go test ./cmd/envoy-go/ -run TestEnvoyGoBinary    # RED
--- FAIL: TestEnvoyGoBinary_EchoesThroughUpstream (0.08s)
    main_test.go:44: build: exit status 1
        # github.com/esalaine/envoy-go/cmd/envoy-go
        runtime.main_main·f: function main is undeclared in the main package
FAIL
FAIL	github.com/esalaine/envoy-go/cmd/envoy-go	0.078s
FAIL

$ go test ./cmd/envoy-go/ -run TestEnvoyGoBinary -v # GREEN
=== RUN   TestEnvoyGoBinary_EchoesThroughUpstream
--- PASS: TestEnvoyGoBinary_EchoesThroughUpstream (0.13s)
PASS
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.129s

$ go test ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.133s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
?   	github.com/esalaine/envoy-go/internal/admin	[no test files]
?   	github.com/esalaine/envoy-go/internal/bootstrap	[no test files]
?   	github.com/esalaine/envoy-go/internal/cluster	[no test files]
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
?   	github.com/esalaine/envoy-go/internal/listener	[no test files]
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
?   	github.com/esalaine/envoy-go/internal/tls	[no test files]
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s

$ golangci-lint run ./...
(empty)
```

## Task 8 — differential byte-compare + hex-dump helper

**Commits:** c5bb5c2
**Notes:** Implemented test/differential/diff.go (CompareBytes + hexWindow) and diff_test.go (3 table cases: equal, diverge-at-first-byte, different-lengths). Phase-00's echo fixture uses byte-exact equivalence per expectations.yaml (later task). Doc.go establishes package-differential context. Verbatim plan code passed lint without adjustment — staticcheck did not flag the `eq` variable (the value is used in the `if eq` guard that follows the loop), and errcheck did not flag fmt.Fprintf on strings.Builder.
**Outputs:**
```
$ go test ./test/differential/ -run TestCompareBytes   # RED
# github.com/esalaine/envoy-go/test/differential [github.com/esalaine/envoy-go/test/differential.test]
test/differential/diff_test.go:6:12: undefined: CompareBytes
test/differential/diff_test.go:16:12: undefined: CompareBytes
test/differential/diff_test.go:32:10: undefined: CompareBytes
FAIL	github.com/esalaine/envoy-go/test/differential [build failed]
FAIL

$ go test ./test/differential/ -run TestCompareBytes -v # GREEN
=== RUN   TestCompareBytes_Equal
--- PASS: TestCompareBytes_Equal (0.00s)
=== RUN   TestCompareBytes_DivergesAtFirstByte
--- PASS: TestCompareBytes_DivergesAtFirstByte (0.00s)
=== RUN   TestCompareBytes_DifferentLengths
--- PASS: TestCompareBytes_DifferentLengths (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	0.002s
```

## Task 9 — differential pin loader + ready-line scanner

**Commits:** 9b4e86f
**Notes:** Implemented test/differential/harness.go (EnvoyPin struct, parseEnvoyTarget regex parser, readyTimeout constant, scanForLine helper) and harness_test.go (2 parser tests: valid pin, missing-tag rejection). TDD: RED then GREEN. Tasks 10 and 11 will extend harness.go with reference/subject proxy types.

**DONE_WITH_CONCERNS:** The `unused` linter flags both `readyTimeout` and `scanForLine` as unused (they are forward-declared for Tasks 10/11 but not yet referenced within the package). Per Task 9 instructions, this is reported without a fix. The controller must decide: (a) add `//nolint:unused` markers on both declarations with a note "used by Task 10/11", or (b) add a forward-reference no-op call site. `golangci-lint run ./...` exits non-zero until one of these resolutions is applied.
**Outputs:**
```
$ go test ./test/differential/ -run TestParseEnvoyTarget   # RED
# github.com/esalaine/envoy-go/test/differential [github.com/esalaine/envoy-go/test/differential.test]
test/differential/harness_test.go:14:14: undefined: parseEnvoyTarget
test/differential/harness_test.go:28:15: undefined: parseEnvoyTarget
FAIL	github.com/esalaine/envoy-go/test/differential [build failed]
FAIL

$ go test ./test/differential/ -run TestParseEnvoyTarget -v # GREEN
=== RUN   TestParseEnvoyTarget_PullsTagAndDigest
--- PASS: TestParseEnvoyTarget_PullsTagAndDigest (0.00s)
=== RUN   TestParseEnvoyTarget_RejectsMissingTag
--- PASS: TestParseEnvoyTarget_RejectsMissingTag (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	0.001s
```

**Follow-up:** added //nolint:unused markers to readyTimeout and scanForLine with references to Tasks 10/11 as their future consumers. Commit: 81ae9ee.

## Task 10 — differential reference proxy (upstream Envoy via testcontainers)

**Commits:** a789b18
**Notes:** Added `github.com/testcontainers/testcontainers-go@v0.27.0` and `github.com/docker/go-connections@v0.4.0` as direct dependencies. Extended `test/differential/harness.go` with `ReferenceProxy` struct, `StartReferenceProxy`, `AdminAddr`, `ListenerAddr`, and `Stop`. Removed `//nolint:unused` from `readyTimeout` (now consumed by `StartReferenceProxy`); retained it on `scanForLine` (still unused until Task 11). Extended `test/differential/harness_test.go` with `TestReferenceProxy_Starts`, `ensureDocker`, and `loadPinFromRepo`. Full suite PASS, lint clean.

**Docker-socket path note:** `/var/run/docker.sock` does not exist on this machine. Docker Desktop exposes the socket at `$HOME/.docker/desktop/docker.sock`. `ensureDocker` was extended (relative to the PLAN's verbatim code) to try both paths. testcontainers-go was invoked with `DOCKER_HOST=unix://$HOME/.docker/desktop/docker.sock`. The Envoy image was already cached from Task 4; no pull occurred.

**Outputs:**
```
$ go test ./test/differential/ -run TestReferenceProxy_Starts   # RED
# github.com/esalaine/envoy-go/test/differential [github.com/esalaine/envoy-go/test/differential.test]
test/differential/harness_test.go:50:14: undefined: StartReferenceProxy
FAIL	github.com/esalaine/envoy-go/test/differential [build failed]
FAIL

$ DOCKER_HOST="unix://${HOME}/.docker/desktop/docker.sock" go test ./test/differential/ -run TestReferenceProxy_Starts -v -timeout 120s   # GREEN
=== RUN   TestReferenceProxy_Starts
2026/04/22 08:08:49 github.com/testcontainers/testcontainers-go - Connected to docker:
  Server Version: 28.1.1
  API Version: 1.43
  Operating System: Docker Desktop
  Total Memory: 64296 MB
  Resolved Docker Host: unix:///home/esa/.docker/desktop/docker.sock
  Resolved Docker Socket Path: /var/run/docker.sock
  Test SessionID: 22c89f611766a0b8ffeffb41428d4c8168f0b5c968faa79d055f4ec01437eaa9
  Test ProcessID: 0e30a3ff-4980-4082-a800-9d1413671d55
2026/04/22 08:08:49 Failed to get image auth for https://index.docker.io/v1/. Setting empty credentials for the image: testcontainers/ryuk:0.6.0. Setting empty credentials for the image: testcontainers/ryuk:0.6.0. Error is:credentials not found in native keychain
2026/04/22 08:08:51 🐳 Creating container for image testcontainers/ryuk:0.6.0
2026/04/22 08:08:51 ✅ Container created: 4443a80226e2
2026/04/22 08:08:51 🐳 Starting container: 4443a80226e2
2026/04/22 08:08:51 ✅ Container started: 4443a80226e2
2026/04/22 08:08:51 🚧 Waiting for container id 4443a80226e2 image: testcontainers/ryuk:0.6.0. Waiting for: &{Port:8080/tcp timeout:<nil> PollInterval:100ms}
2026/04/22 08:08:51 🐳 Creating container for image envoyproxy/envoy:v1.37.2
2026/04/22 08:08:51 ✅ Container created: 0f3ba6538a60
2026/04/22 08:08:51 🐳 Starting container: 0f3ba6538a60
2026/04/22 08:08:51 ✅ Container started: 0f3ba6538a60
2026/04/22 08:08:51 🚧 Waiting for container id 0f3ba6538a60 image: envoyproxy/envoy:v1.37.2. Waiting for: &{timeout:0x28e5150747b0 Port:9901/tcp Path:/ready StatusCodeMatcher:0x85ffc0 ResponseMatcher:0x934f80 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/22 08:08:51 🐳 Terminating container: 0f3ba6538a60
2026/04/22 08:08:52 🚫 Container terminated: 0f3ba6538a60
--- PASS: TestReferenceProxy_Starts (2.41s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.479s
```
