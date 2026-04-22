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

## Task 2 — bootstrap.Load happy path + dynamic_resources/layered_runtime rejection

**Commits:** `d98c5fa`
**Notes:** `internal/bootstrap.Load` implements the three-stage pipeline codified by ADR-0012 — `gopkg.in/yaml.v3` decodes the input into `map[string]interface{}`, `encoding/json.Marshal` re-emits as JSON, and `google.golang.org/protobuf/encoding/protojson.Unmarshal` (with `DiscardUnknown: false` per ADR-0016) binds the JSON into an `envoy.config.bootstrap.v3.Bootstrap`. The pipeline was chosen over `sigs.k8s.io/yaml` (extra dep wrapping `yaml.v2`, no new capability) and over a direct YAML-to-proto `protoreflect` walker (non-canonical, duplicates `protojson`'s Any/well-known-type handling); `yaml.v3` was already a direct require from the phase-00 loader so no new module is introduced, and `protojson` is the canonical Go proto-JSON codec. Phase-01 unsupported surfaces (`dynamic_resources`, `layered_runtime`) are rejected at the `map[string]interface{}` stage — before `protojson` touches the bytes — so the error messages name the top-level key and reference SPEC §2 without requiring proto-reflection. The `typed_config` Any inside filter chains (phase-01 fixtures use only `envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy`) is preserved but not interpreted: `bootstrap.go` blank-imports `go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3` so `protoregistry.GlobalTypes` can resolve the `@type` URL for round-trip, while envoy-go code does not inspect or act on the Any contents (ADR-0016 consequences §3: later phases extend the blank-import list as fixtures introduce new filter types). Every error returned by `Load` begins with the sentinel `bootstrap: ` so callers can distinguish loader errors from other packages. `go mod tidy` promoted `google.golang.org/protobuf v1.36.11` from indirect to direct (as anticipated by ADR-0013 Consequences §3); `go 1.23.0` directive unchanged.

**Outputs:**
```
$ GOTOOLCHAIN=local go test ./internal/bootstrap/ -run TestLoad -v
=== RUN   TestLoad_HappyPath
--- PASS: TestLoad_HappyPath (0.00s)
=== RUN   TestLoad_RejectsDynamicResources
--- PASS: TestLoad_RejectsDynamicResources (0.00s)
=== RUN   TestLoad_RejectsLayeredRuntime
--- PASS: TestLoad_RejectsLayeredRuntime (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.005s

$ GOTOOLCHAIN=local go test ./internal/bootstrap/ -race -run TestLoad -v
=== RUN   TestLoad_HappyPath
--- PASS: TestLoad_HappyPath (0.01s)
=== RUN   TestLoad_RejectsDynamicResources
--- PASS: TestLoad_RejectsDynamicResources (0.00s)
=== RUN   TestLoad_RejectsLayeredRuntime
--- PASS: TestLoad_RejectsLayeredRuntime (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.019s

$ GOTOOLCHAIN=local go vet ./...
(no output — exit 0)

$ GOTOOLCHAIN=local golangci-lint run ./...
(no output — exit 0)

$ GOTOOLCHAIN=local go test ./... -timeout 5m
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	(cached)
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
?   	github.com/esalaine/envoy-go/internal/admin	[no test files]
ok  	github.com/esalaine/envoy-go/internal/bootstrap	(cached)
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
ok  	github.com/esalaine/envoy-go/test/differential	(cached)
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	(cached)
```

## Task 3 — bootstrap.Load error-path tests (syntax, unknown field, empty)

**Commits:** `f3ad272`
**Notes:** locks behavior, no prod changes needed — `internal/bootstrap.Load` (landed Task 2, commit `d98c5fa`) already rejects YAML syntax errors with the `bootstrap: yaml parse:` prefix, unknown top-level fields via `protojson.UnmarshalOptions{DiscardUnknown: false}` producing the `bootstrap: protojson:` prefix, and empty documents via the `generic == nil` check producing `bootstrap: empty document`; these three tests append to `internal/bootstrap/bootstrap_test.go` to pin those contracts so future refactors cannot silently weaken the loader's error surface. The PLAN's exact YAML-syntax-error input `"not: valid: yaml: at all: :::"` was retained verbatim — `gopkg.in/yaml.v3` flags it as a parse error (not as a string scalar nor as protojson input), so no assertion adjustment was needed. No production code touched, no ADRs, no `go.mod` drift; `sampleBootstrap` const and the three Task 2 tests are unchanged.

**Outputs:**
```
$ GOTOOLCHAIN=local go test ./internal/bootstrap/ -v
=== RUN   TestLoad_HappyPath
--- PASS: TestLoad_HappyPath (0.00s)
=== RUN   TestLoad_RejectsDynamicResources
--- PASS: TestLoad_RejectsDynamicResources (0.00s)
=== RUN   TestLoad_RejectsLayeredRuntime
--- PASS: TestLoad_RejectsLayeredRuntime (0.00s)
=== RUN   TestLoad_YAMLSyntaxError
--- PASS: TestLoad_YAMLSyntaxError (0.00s)
=== RUN   TestLoad_UnknownTopLevelField
--- PASS: TestLoad_UnknownTopLevelField (0.00s)
=== RUN   TestLoad_EmptyDocument
--- PASS: TestLoad_EmptyDocument (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.005s
```

## Task 4 — bootstrap.AdminSocket extractor

**Commits:** `0176329`
**Notes:** `AdminSocket(bs *bootstrapv3.Bootstrap) (host string, port uint32, err error)` is the first of the phase-01 extractor family — a thin function that walks the proto tree with the generated `GetAdmin()/GetAddress()/GetSocketAddress()/GetAddress()/GetPortValue()` accessors and returns the admin listener's host+port, erroring if the admin block is missing or if its address is not a `socket_address`. The extractor pattern (vs. methods on the proto) is deliberate: the `Bootstrap` proto is owned by `go-control-plane` and doctrine D-3.2 forbids wrapping it in envoy-go types, so validation/projection logic lives as free functions in `internal/bootstrap` that take `*bootstrapv3.Bootstrap` and return primitive/error tuples. Proto getters safely handle nil receivers (returning zero values), so the three nil-guards are belt-and-suspenders for producing a specific error message at each level rather than a generic "missing" at the leaf — callers that want granular diagnostics get them. All errors begin with the sentinel `bootstrap: ` matching `Load`'s contract; the missing-admin path returns `bootstrap: missing admin`. The missing-admin test uses a minimal YAML with `static_resources: { listeners: [], clusters: [] }` — `Load` accepts this because admin is optional at the proto level and admin-presence validation is the extractor's job, not the loader's. No ADRs, no `go.mod` drift, no production changes beyond the new exported function.

**Outputs:**
```
$ GOTOOLCHAIN=local go test ./internal/bootstrap/ -v
=== RUN   TestLoad_HappyPath
--- PASS: TestLoad_HappyPath (0.00s)
=== RUN   TestLoad_RejectsDynamicResources
--- PASS: TestLoad_RejectsDynamicResources (0.00s)
=== RUN   TestLoad_RejectsLayeredRuntime
--- PASS: TestLoad_RejectsLayeredRuntime (0.00s)
=== RUN   TestLoad_YAMLSyntaxError
--- PASS: TestLoad_YAMLSyntaxError (0.00s)
=== RUN   TestLoad_UnknownTopLevelField
--- PASS: TestLoad_UnknownTopLevelField (0.00s)
=== RUN   TestLoad_EmptyDocument
--- PASS: TestLoad_EmptyDocument (0.00s)
=== RUN   TestAdminSocket_HappyPath
--- PASS: TestAdminSocket_HappyPath (0.00s)
=== RUN   TestAdminSocket_MissingAdmin
--- PASS: TestAdminSocket_MissingAdmin (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.006s
```

## Task 5 — bootstrap.FirstListenerSocket + FirstClusterEndpointSocket extractors [ADR-0017]

**Commits:** _pending SHA-fill_
**Notes:** Twin extractors landing the remaining phase-01 projections from the `Bootstrap` proto into primitive host+port pairs, following the same pattern established by Task 4's `AdminSocket`: free functions in `internal/bootstrap` that walk the proto tree via generated `Get*()` accessors and return `(host string, port uint32, err error)` — the `*bootstrapv3.Bootstrap` receiver keeps the D-3.2 boundary intact (no wrapping of the upstream proto) while the primitive return type insulates callers from the proto's deep-nesting idioms. `FirstListenerSocket` enforces exactly one listener at `static_resources.listeners[0].address.socket_address`; `FirstClusterEndpointSocket` enforces exactly-one at each level of the `clusters[0].load_assignment.endpoints[0].lb_endpoints[0].endpoint.address.socket_address` path. The "exactly one" constraint is phase-01-specific — fixtures `0000-tcp-echo` and the sampleBootstrap each have a single listener and a single cluster with one endpoint — and future phases that introduce multi-listener or multi-endpoint bootstraps will either add siblings (`AllListenerSockets`, `ClusterEndpointSockets(name string)`) or supersede these extractors with an ADR; the phase-01 shape is deliberately narrow rather than speculatively general. Error messages begin with the `bootstrap: ` sentinel matching the loader and `AdminSocket` contract, and each "wrong count" error includes the observed count (`got %d`) so test assertions can pin both the path and the violation. ADR-0017 ([node-field-semantics]) lands alongside the extractors to close the loose end in SPEC §2: the loader parses `node` as-is without enforcing presence of `node.id` / `node.cluster` — YAGNI, since no phase-01 consumer of `node` exists and admin/xDS semantics that would drive field-presence rules land in phase 08+. The happy-path tests pin that both extractors read `127.0.0.1:0` from the shared `sampleBootstrap` fixture; error-path tests exercise zero-listener, two-listener, and empty-endpoints cases. The two-listener case uses `strings.Replace` on `sampleBootstrap` to prepend a `name: l_a`-only entry before the full `l_tcp` body — `Load` accepts this because listener `address` is optional at the proto level, and the extractor's `len(ls) != 1` check fires before any address deref, so the `got 2` assertion pins the count-check branch (not a derivative "missing address" from the malformed first listener). No production changes beyond the two new exported functions; no `go.mod` drift.

**Outputs:**
```
$ GOTOOLCHAIN=local go test ./internal/bootstrap/ -v
=== RUN   TestLoad_HappyPath
--- PASS: TestLoad_HappyPath (0.00s)
=== RUN   TestLoad_RejectsDynamicResources
--- PASS: TestLoad_RejectsDynamicResources (0.00s)
=== RUN   TestLoad_RejectsLayeredRuntime
--- PASS: TestLoad_RejectsLayeredRuntime (0.00s)
=== RUN   TestLoad_YAMLSyntaxError
--- PASS: TestLoad_YAMLSyntaxError (0.00s)
=== RUN   TestLoad_UnknownTopLevelField
--- PASS: TestLoad_UnknownTopLevelField (0.00s)
=== RUN   TestLoad_EmptyDocument
--- PASS: TestLoad_EmptyDocument (0.00s)
=== RUN   TestAdminSocket_HappyPath
--- PASS: TestAdminSocket_HappyPath (0.00s)
=== RUN   TestAdminSocket_MissingAdmin
--- PASS: TestAdminSocket_MissingAdmin (0.00s)
=== RUN   TestFirstListenerSocket_HappyPath
--- PASS: TestFirstListenerSocket_HappyPath (0.00s)
=== RUN   TestFirstListenerSocket_ZeroListeners
--- PASS: TestFirstListenerSocket_ZeroListeners (0.00s)
=== RUN   TestFirstListenerSocket_TwoListeners
--- PASS: TestFirstListenerSocket_TwoListeners (0.00s)
=== RUN   TestFirstClusterEndpointSocket_HappyPath
--- PASS: TestFirstClusterEndpointSocket_HappyPath (0.00s)
=== RUN   TestFirstClusterEndpointSocket_EmptyEndpoints
--- PASS: TestFirstClusterEndpointSocket_EmptyEndpoints (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.006s

$ GOTOOLCHAIN=local go test ./internal/bootstrap/ -v -race
(all 13 tests PASS; ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.025s)

$ GOTOOLCHAIN=local go vet ./...
(no output — exit 0)

$ GOTOOLCHAIN=local golangci-lint run ./...
(no output — exit 0)

$ GOTOOLCHAIN=local go test ./... -timeout 5m
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	(cached)
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
?   	github.com/esalaine/envoy-go/internal/admin	[no test files]
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.005s
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
ok  	github.com/esalaine/envoy-go/test/differential	(cached)
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	(cached)
```
