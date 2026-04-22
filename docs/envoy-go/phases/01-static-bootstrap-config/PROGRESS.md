# Phase 01 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim.

## Preamble — execution preconditions

None. All preconditions from PLAN.md's "Execution preconditions" block were satisfied at cold-start: worktree at `phase/01-static-bootstrap-config-impl` cut off `master`, baseline `go test ./...` green, `go` toolchain 1.26.2 with `GOTOOLCHAIN=local` honoured to keep `go.mod`'s `go 1.23.0` directive intact, `golangci-lint` v1.64.8 on PATH, and the phase-00 committed tail at `f8598ca` (STATE pointer → PLAN). No deviations.

## Task 1 — Add go-control-plane + protojson direct deps

**Commits:** `52fbd95`
**Notes:** Pinned `github.com/envoyproxy/go-control-plane/envoy` at `v1.32.4` (tag date 2024-12-19), the nested proto-types module, rather than the parent `github.com/envoyproxy/go-control-plane@v0.13.x`: upstream has split the envoy proto packages out of the parent module into a separate semver-independent module at path `/envoy`, and `go mod tidy` resolves the phase-01 import `envoy/config/bootstrap/v3` through that nested module. The parent module is intentionally **not** pinned as a direct require so that doctrine D-3.2's proto-types-only boundary is visible in `go.mod` (any future import of `github.com/envoyproxy/go-control-plane/pkg/...` would surface as a new direct require needing a superseding ADR). `google.golang.org/protobuf` was resolved by `go get` at `v1.36.11`; it is still `// indirect` at this commit because Task 1 does not itself import `protojson` — Task 2 adds the first real import in `internal/bootstrap/bootstrap.go` and that will promote it. ADR-0013 captures the full rationale, including the module-split observation that invalidated PLAN.md's `v0.13.x` hint.

**Outputs:**
```
$ GOTOOLCHAIN=local go get github.com/envoyproxy/go-control-plane/envoy@v1.32.4
go: upgraded cel.dev/expr v0.16.0 => v0.19.0
go: upgraded github.com/cncf/xds/go v0.0.0-20240723142845-024c85f92f20 => v0.0.0-20240905190251-b4127c9b8d78
go: upgraded github.com/envoyproxy/go-control-plane/envoy v1.32.3 => v1.32.4
go: upgraded github.com/envoyproxy/protoc-gen-validate v1.1.0 => v1.2.1
go: upgraded golang.org/x/net v0.30.0 => v0.34.0
go: upgraded golang.org/x/sync v0.8.0 => v0.10.0
go: upgraded golang.org/x/text v0.19.0 => v0.21.0
go: upgraded google.golang.org/genproto/googleapis/api v0.0.0-20240814211410-ddb44dafa142 => v0.0.0-20241202173237-19429a94021a
go: upgraded google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 => v0.0.0-20241202173237-19429a94021a
go: upgraded google.golang.org/grpc v1.67.1 => v1.70.0

$ GOTOOLCHAIN=local go get google.golang.org/protobuf
(no output — resolved to v1.36.11)

$ GOTOOLCHAIN=local go mod tidy
(no output — tidy is silent when deps are cached and go.mod is consistent with the import graph of probe.go)

$ GOTOOLCHAIN=local go build ./...
(no output — exit 0; probe.go's `import _ "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"` compiles cleanly, confirming the proto package is importable)

$ GOTOOLCHAIN=local go vet ./... && golangci-lint run ./...
(no output — exit 0; run after probe.go was deleted, with go.mod still carrying the direct require for `go-control-plane/envoy v1.32.4`)

$ GOTOOLCHAIN=local go test ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.149s
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
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/differential	1.816s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	0.001s
```
