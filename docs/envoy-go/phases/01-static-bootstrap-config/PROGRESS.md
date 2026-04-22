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

**Commits:** `399a1b9`
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

## Task 6 — FuzzBootstrapLoad fuzz target + CI job [ADR-0018]

**Commits:** `5c81c56`
**Notes:** Closes phase-01 gate (d) — `FuzzBootstrapLoad` is a Go native fuzz target (`testing.F`) in `internal/bootstrap/fuzz_test.go` (same package as `bootstrap.go` / `bootstrap_test.go`, so it can read `sampleBootstrap` directly without re-declaring it). The seed corpus has 8 entries inlined via `f.Add` — no external `testdata/fuzz/` files: (1) `sampleBootstrap` (happy bootstrap from `bootstrap_test.go`), (2) the verbatim bytes of `test/fixtures/0000-tcp-echo/envoy.yaml` (captured as `const envoyYAMLSeed`, the reference-Envoy bootstrap used by the differential gate), (3) empty string, (4) single space, (5) three NULs, (6) partial `admin:`, (7) bootstrap-shaped empty-arrays `static_resources: { listeners: [], clusters: [] }`, (8) 400-byte deeply-nested YAML (`bytes.Repeat([]byte("- "), 200)`). The fixture's `envoy-go.yaml` is NOT seeded — it is still in the phase-00 shape at this point and only gets rewritten to bootstrap shape in Task 12, so seeding it now would feed the fuzzer an input that `Load` legitimately rejects under phase-01 rules and add no coverage over `sampleBootstrap` + `envoy.yaml`. The property asserted by `f.Fuzz` is minimal-but-strong: `Load(bytes.NewReader(data))` MUST NOT panic for any byte sequence — every output is either `(*Bootstrap, nil)` or `(nil, err)` where `err.Error()` starts with `bootstrap: `. No return-value inspection inside the fuzz body: asserting the `bootstrap: ` prefix inside the harness would double-count the unit-test layer and risk false positives from mutated-but-benign inputs. CI lane `fuzz-bootstrap` (`.github/workflows/ci.yml`) runs on `ubuntu-latest`, `needs: lint-vet-test`, standalone from `differential` (no Docker), single step `go test ./internal/bootstrap/ -fuzz=FuzzBootstrapLoad -fuzztime=30s -run=^$` — the 30s budget from ADR-0018 is deliberately shorter than the 5-minute differential job so the two run in parallel without CI wall-clock pressure. No new `testdata/fuzz/FuzzBootstrapLoad/` files were committed (Go only creates those when it finds a failing input; the 30s run found zero). Seed replay (`-run=FuzzBootstrapLoad` without `-fuzz`) passed all 8 seeds first, then the 30s engine run executed 2,848,963 inputs across 32 workers with 348 new interesting discoveries and zero crashes.

**Outputs:**
```
$ GOTOOLCHAIN=local go test ./internal/bootstrap/ -run=FuzzBootstrapLoad -v
=== RUN   FuzzBootstrapLoad
=== RUN   FuzzBootstrapLoad/seed#0
=== RUN   FuzzBootstrapLoad/seed#1
=== RUN   FuzzBootstrapLoad/seed#2
=== RUN   FuzzBootstrapLoad/seed#3
=== RUN   FuzzBootstrapLoad/seed#4
=== RUN   FuzzBootstrapLoad/seed#5
=== RUN   FuzzBootstrapLoad/seed#6
=== RUN   FuzzBootstrapLoad/seed#7
--- PASS: FuzzBootstrapLoad (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#0 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#1 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#2 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#3 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#4 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#5 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#6 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#7 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.005s

$ GOTOOLCHAIN=local go test ./internal/bootstrap/ -fuzz=FuzzBootstrapLoad -fuzztime=30s -run=^$
fuzz: elapsed: 0s, gathering baseline coverage: 0/8 completed
fuzz: elapsed: 0s, gathering baseline coverage: 8/8 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 38075 (12677/sec), new interesting: 85 (total: 93)
fuzz: elapsed: 6s, execs: 38075 (0/sec), new interesting: 85 (total: 93)
fuzz: elapsed: 9s, execs: 38075 (0/sec), new interesting: 85 (total: 93)
fuzz: elapsed: 12s, execs: 271609 (77949/sec), new interesting: 86 (total: 94)
fuzz: elapsed: 15s, execs: 1323493 (350627/sec), new interesting: 160 (total: 168)
fuzz: elapsed: 18s, execs: 1878344 (184921/sec), new interesting: 255 (total: 263)
fuzz: elapsed: 21s, execs: 2108123 (76617/sec), new interesting: 293 (total: 301)
fuzz: elapsed: 24s, execs: 2445813 (112551/sec), new interesting: 316 (total: 324)
fuzz: elapsed: 27s, execs: 2845357 (133195/sec), new interesting: 346 (total: 354)
fuzz: elapsed: 30s, execs: 2848963 (1202/sec), new interesting: 348 (total: 356)
fuzz: elapsed: 31s, execs: 2848963 (0/sec), new interesting: 348 (total: 356)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.082s

$ GOTOOLCHAIN=local go vet ./...
(no output — exit 0)

$ GOTOOLCHAIN=local golangci-lint run ./...
(no output — exit 0)

$ GOTOOLCHAIN=local go test ./... -timeout 5m
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	(cached)
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
?   	github.com/esalaine/envoy-go/internal/admin	[no test files]
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.006s
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

## Task 7 — Empirically observe upstream Envoy /ready bytes [ADR-0015]

**Commits:** `90957e1`

**Notes:** Pure-observation task, no code changes. Ran the minimal `/tmp/envoy-ready-probe.yaml` bootstrap (admin on `0.0.0.0:9901`, empty `static_resources`) under `envoyproxy/envoy:v1.37.2` (digest `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`, matching ADR-0008) on Docker Desktop engine `28.1.1` (client `28.4.0`). Captured the ready-state `/ready` response byte-exact via `curl -s -i` and confirmed the shape with a separate body-only `xxd` dump; the evidence file `upstream-ready-observation.md` quotes the raw bytes and the hex dump verbatim. Pre-init capture attempted twice: a 20-attempt loop with 50ms spacing, then a 40-attempt loop with no inter-attempt sleep — 60 probes total, zero non-200 HTTP responses. The only "non-200" outcome was attempt 1 of loop 1 returning empty (TCP not yet listening — kernel RST, not a pre-init HTTP response). Per ADR-0015 option (b), pre-init is declared unobservable from this bootstrap shape and the admin server emits a phase-01-chosen `503 Service Unavailable` / `PRE_INITIALIZING\n` pre-init response that is documented-but-test-irrelevant (the phase-01 differential harness observes only the ready state because `cmd/envoy-go` calls `MarkReady` before printing the ready sentinel). Container cleanly removed (`docker ps -a --filter name=envoy-ready-probe` is empty post-cleanup). Three files land in this commit: the new evidence doc, ADR-0015 appended to DECISIONS.md (sequenced after ADR-0018 to preserve append-only chronology), and this PROGRESS entry. No `/tmp/` capture artefacts are committed — they are named in the evidence file's §"Capture artefacts" but re-generated by re-running the Task 7 procedure if needed.

**Outputs:**

Ready-state capture (`curl -s -i http://127.0.0.1:9901/ready`, raw file contents with `^M` denoting CR, `$` denoting LF):

```
HTTP/1.1 200 OK^M$
content-type: text/plain; charset=UTF-8^M$
cache-control: no-cache, max-age=0^M$
x-content-type-options: nosniff^M$
date: Wed, 22 Apr 2026 21:38:35 GMT^M$
server: envoy^M$
transfer-encoding: chunked^M$
^M$
LIVE$
```

Body-only hex (`curl -s http://127.0.0.1:9901/ready | xxd`) — 5 bytes, trailing LF, no CRLF:

```
00000000: 4c49 5645 0a                             LIVE.
```

Pre-init probe summary (20-attempt loop, 50ms spacing):

```
attempt 1  @ 1776893925.359630: <empty — pre-accept>
attempt 2  @ 1776893925.417792: HTTP/1.1 200 OK
attempt 3  @ 1776893925.476026: HTTP/1.1 200 OK
...
attempt 20 @ 1776893926.485507: HTTP/1.1 200 OK
=== CAPTURED=0 ===
```

Pre-init probe summary (40-attempt tight loop, no inter-attempt sleep):

```
attempts 1–40: all < HTTP/1.1 200 OK
NON-2xx CAPTURED: none
```

Outcome: pre-init unobservable from this bootstrap shape; ADR-0015 accepts option (b).

## Task 8 — admin.Server + /ready ready-state byte-exact [ADR-0014]

**Commits:** `cb6bed3`

**Notes:** First real `internal/admin` code lands — phase-00's `doc.go` placeholder is rewritten to a phase-01 package comment, and two new files (`admin.go`, `admin_test.go`) implement and pin the `/ready` ready-state response. `Server` is a four-field struct (`addr`, `ln`, `httpSrv`, `ready atomic.Bool`) with the three-method surface `New` / `Start` / `MarkReady` / `Close` plus the private `handleReady` — `atomic.Bool` (Go 1.19+ type) is the lock-free primitive used by Task 9's concurrency tests, chosen over `sync.RWMutex` because the ready bit is strictly monotonic (set-once, read-many) and the stdlib idiom is canonical. `Start` binds synchronously, spawns a goroutine that calls `s.httpSrv.Serve(ln)`, and returns the bound `ln.Addr().String()` so tests passing port `0` can discover the kernel-assigned port atomically. `Close` is best-effort and idempotent (nil-safe — `s.httpSrv` or `s.ln` may be nil if `Start` was never called or failed mid-way), using `http.Server.Close` rather than `Shutdown` because phase-01 does not gate on graceful drain (explicit phase-08 follow-up). `handleReady` sets five response headers unconditionally (`Content-Type`, `Cache-Control`, `X-Content-Type-Options`, `Server`, and `Content-Length` per branch), writes `LIVE\n` (5 bytes) + 200 when `ready.Load()` is true, and `PRE_INITIALIZING\n` (17 bytes) + 503 otherwise — the pre-init branch is covered by Task 9, not this task's test. The `Date` header is not set by envoy-go code; Go's `net/http` server auto-inserts RFC 7231 `Date` on every response, matching upstream's observed `date:` header and allow-listed in ADR-0015's harness rules. ADR-0014 pins the `Server: envoy` value (byte-exact with upstream v1.37.2, no version suffix, minimises differential allow-list entries).

**Divergences from the PLAN Task 8 snippet** — enumerated per the "reality vs. PLAN guess" principle:

1. **`Content-Type` value.** PLAN snippet used `text/plain`; Task 7 evidence observed `text/plain; charset=UTF-8`. Resolution: emit `text/plain; charset=UTF-8` (charset token exactly `UTF-8`, hyphenated, uppercase, per evidence §Observations). Pinned by the test assertion `resp.Header.Get("Content-Type") != "text/plain; charset=UTF-8"`.
2. **`X-Content-Type-Options` header.** PLAN snippet omitted it; Task 7 evidence captured `x-content-type-options: nosniff` as the third header on the wire. Resolution: emit `X-Content-Type-Options: nosniff` and pin via the test. Without this, the subject would diverge from upstream on a security-relevant response header and the differential diff would flag a header-set mismatch.
3. **`Cache-Control` value.** PLAN snippet's `no-cache, max-age=0` was a guess that Task 7 evidence confirmed verbatim (including the comma-plus-single-space separator). Resolution: kept as-is.
4. **Response framing.** PLAN snippet was silent on framing; Task 7 evidence shows upstream emits `transfer-encoding: chunked` with no `Content-Length`. Per ADR-0015's option (b) and the evidence §"Framing" allow-list, the phase-01 subject emits `Content-Length: 5` (and `Content-Length: 17` for pre-init) as a documented BEHAVIOR_CONTRACT deviation; Task 14's differential harness dechunks upstream before byte-comparing the body, so the logical body bytes remain identical. Resolution: explicit `h.Set("Content-Length", strconv.Itoa(len(body)))` per branch rather than relying on Go's `net/http` implicit length inference — the explicit form is deterministic and documents intent. Phase-02+ may switch to chunked framing without an ADR if the harness normaliser remains in place.
5. **Pre-init response body.** PLAN snippet's `PRE_INITIALIZING\n` (17 bytes) was a guess — Task 7 determined pre-init is unobservable across 60 probes against an empty-bootstrap v1.37.2 container. Resolution: kept `PRE_INITIALIZING\n` + `Content-Length: 17` + status 503 per ADR-0015 option (b), for unit-test determinism in Task 9. The phase-01 differential harness never observes the subject's pre-init window (`cmd/envoy-go` calls `MarkReady` before printing the ready sentinel that the harness waits on), so the chosen body is documented-but-test-irrelevant for the gate.
6. **Header name casing on the wire.** Go's `net/http` server serves header names in canonical case (`Content-Type`), differing from upstream's observed lowercase (`content-type`). Resolution: accepted because HTTP/1.1 header names are case-insensitive per RFC 7230 §3.2; Task 14's `diffHeaders` helper performs case-insensitive comparison (BEHAVIOR_CONTRACT will codify this). The Task 8 unit-test uses `http.Header.Get` which normalises to canonical case on both sides, so the assertion casing matches — the lowercase-vs-title-case question is only visible on raw wire bytes, which Task 14 parses into a case-insensitive map before diffing.

**`freeAddr` helper** — declared in `admin_test.go` per PLAN Step 3 but not yet consumed at this task. Task 9's concurrency tests use it. A `//nolint:unused` directive is applied with a Task-9 reference to prevent `golangci-lint` `unused` from flagging the dead code for one commit; the directive is removed in Task 9 when the first consumer lands.

**Outputs:**

```
$ GOTOOLCHAIN=local go test ./internal/admin/ -run TestServer_ReadyState -v
=== RUN   TestServer_ReadyState
--- PASS: TestServer_ReadyState (0.01s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	0.012s

$ GOTOOLCHAIN=local go test -race ./internal/admin/ -v
=== RUN   TestServer_ReadyState
--- PASS: TestServer_ReadyState (0.01s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	1.017s

$ GOTOOLCHAIN=local go vet ./...
(no output — exit 0)

$ GOTOOLCHAIN=local golangci-lint run ./...
(no output — exit 0)

$ GOTOOLCHAIN=local go test ./... -timeout 5m
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	(cached)
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	0.012s
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

## Task 9 — admin.Server pre-init + MarkReady atomicity + Close idempotency tests

**Commits:** `c2cb3fb`

**Notes:** Three concurrency/state tests append to `internal/admin/admin_test.go`, closing phase-01 gate (c)'s admin-server surface: `TestServer_PreInit_BeforeMarkReady` pins the pre-init branch of `handleReady` (`Start` without `MarkReady` → non-200 status with body distinct from `LIVE\n`, per ADR-0015 option (b) / Task 8's chosen `503 Service Unavailable` + `PRE_INITIALIZING\n`); `TestServer_MarkReady_IsAtomic` fires 50 concurrent `GET /ready` probes against a goroutine while the main test races `s.MarkReady()` in parallel — the `sync/atomic.Bool` field introduced by Task 8 is the contract under test, and `-race` must see zero `DATA RACE` reports; `TestServer_Close_Idempotent` exercises three `Close` sequences — close-before-Start (no listener bound yet, relies on the `s.httpSrv == nil && s.ln == nil` nil-guard), first-Close after a successful `Start`, and second-Close on the same server. All three tests use `s.Start()` with `"127.0.0.1:0"` rather than pre-computing a free port; the `freeAddr` helper scaffolded by Task 8 has no consumer and is **deleted** per option (a) — dead code shouldn't persist past the task that needed it. The `//nolint:unused` directive is gone with it, and the `net` import is dropped from `admin_test.go` (the three new tests use only `io`, `net/http`, `testing`, `time`).

**admin.go was NOT modified.** The PLAN flagged a potential idempotency concern — "the second `Close` may return `http.ErrServerClosed`" — but this is incorrect for Go 1.23's stdlib: `http.Server.Close()` is already idempotent. Verified with a throw-away `go run` probe that called `srv.Close()` three times against a `Serve`-ing server and received `<nil>` on all three. The `http.ErrServerClosed` sentinel is returned by `Serve`/`ListenAndServe`, not by `Close`; subsequent `Close` calls short-circuit cleanly because the server's internal `inShutdown` atomic has already flipped. The nil-guards in `admin.go`'s Close (`if s.httpSrv != nil`, `if s.ln != nil`) handle the Close-before-Start case, and stdlib handles the double-Close case, so no `closed atomic.Bool` short-circuit flag is needed. All four tests (existing `TestServer_ReadyState` plus the three new ones) PASS under `-race` on first run and reliably across 5 repeated runs of `TestServer_Close_Idempotent` (`-count=5`).

**Self-review:** 4 tests total in `admin_test.go` (`TestServer_ReadyState` + `TestServer_PreInit_BeforeMarkReady` + `TestServer_MarkReady_IsAtomic` + `TestServer_Close_Idempotent`). `freeAddr` deleted (option a). Zero `//nolint` directives remain in the admin package. `admin.go` untouched. Full gate sweep green.

**Outputs:**

```
$ GOTOOLCHAIN=local go test -race ./internal/admin/ -v -count=1
=== RUN   TestServer_ReadyState
--- PASS: TestServer_ReadyState (0.01s)
=== RUN   TestServer_PreInit_BeforeMarkReady
--- PASS: TestServer_PreInit_BeforeMarkReady (0.01s)
=== RUN   TestServer_MarkReady_IsAtomic
--- PASS: TestServer_MarkReady_IsAtomic (0.02s)
=== RUN   TestServer_Close_Idempotent
--- PASS: TestServer_Close_Idempotent (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	1.051s

$ GOTOOLCHAIN=local go vet ./...
(no output — exit 0)

$ GOTOOLCHAIN=local golangci-lint run ./...
(no output — exit 0)

$ GOTOOLCHAIN=local go test ./... -timeout 5m -count=1
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.182s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	0.043s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.007s
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
ok  	github.com/esalaine/envoy-go/test/differential	1.832s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s
```

## Task 10 — BEHAVIOR_CONTRACT Admin API subsection (SPEC §5.5)

**Commits:** `0979230`

**Notes:** Pure docs task, zero code changes. Two files touched: `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains a new H2 section `## Admin API — /ready` (em-dash, U+2014, matching `expectations.yaml`'s allow-list reference verbatim) inserted BETWEEN `## Timing tolerances` and `## Test harness host networking` — not appended to EOF, not placed above `## Equivalence Matrix`. The new section's body is drawn from Task 7's evidence file (`upstream-ready-observation.md`), not from PLAN.md's guessed shape: ready-state response lists all six upstream headers in wire order (`content-type`, `cache-control`, `x-content-type-options`, `date`, `server`, `transfer-encoding`) with byte-exact values and the 5-byte `LIVE\n` body; the single framing deviation (`Content-Length: 5` on the subject vs. `transfer-encoding: chunked` upstream) is documented as a phase-02+ follow-up that the Task 14 harness normalises by dechunking upstream before body comparison; header-name case is noted (RFC 7230 §3.2 case-insensitivity covers the lowercase-on-wire vs. Go-canonical divergence). The pre-init subsection captures ADR-0015 option (b): upstream's pre-init bytes were unobservable across 60 probes, so the subject emits a chosen `503 Service Unavailable` / `PRE_INITIALIZING\n` (17 bytes) response that is documented-but-test-irrelevant for phase 01. The second edit extends the previously stubbed `## Header allow-list` section: the stub's template prose is retained and a canonical table is introduced with one entry — `date` scoped to the Admin `/ready` response, value non-deterministic per request, presence required on both sides but value not byte-compared, introduced by Phase 01 under ADR-0015. Future phases extend this table rather than re-introducing one. No ADRs, no `go.mod` drift, no test output to quote (pure docs).

## Task 11 — test/helpers/http_response.go parser [ADR-0019]

**Commits:** `3a2218b`

**Notes:** Added a small HTTP/1.1 response parser to the `test/helpers/` package — colocated with phase-00's `tcp.go` round-tripper, establishing `test/helpers/` as the shared test-side protocol-primitives home. Two files created: `test/helpers/http_response.go` (60 lines: `HTTPResponse{StatusLine, Headers map[string]string, Body []byte}` + `ParseHTTPResponse(raw []byte) (*HTTPResponse, error)` + internal `joinHeaderValues`) and `test/helpers/http_response_test.go` (three table-driven-ish tests: Simple / MultiValueHeader / Malformed). Implementation leans on `net/http.ReadResponse` for status-line and header parsing (stdlib, D-3.2 permitted) rather than hand-rolling a state machine; body is drained via a chunked `Read` loop into a `[]byte` to decouple the returned `Body` from the bufio reader's lifetime. Status line is reconstructed via `fmt.Sprintf("%s %s", resp.Proto, resp.Status)` because `http.ReadResponse` consumes it; this yields `"HTTP/1.1 200 OK"` verbatim, matching the on-wire line. Headers are canonicalised with `textproto.CanonicalMIMEHeaderKey` on write so callers see `"Content-Type"` rather than whatever case the wire used — consistent with Go stdlib convention and simpler for expectations.yaml diff logic downstream. Multi-value headers (two `X-A:` lines) are joined by `", "` — RFC 7230 §3.2.2 permits list-header concatenation, and the `strings.Contains` asserts in the test pin the join behaviour without over-specifying delimiter order. Malformed input propagates `http.ReadResponse`'s error wrapped with `http_response: parse:` prefix for grep-friendly identification in harness logs. Fail-first loop honoured: wrote the `_test.go` file first, ran `go test ./test/helpers/` to confirm the `undefined: ParseHTTPResponse` compile error, then wrote the implementation and re-ran to green. ADR-0019 appended at the tail of `DECISIONS.md` (after ADR-0014, the previous tail entry in the document though higher-numbered ADRs — 0015-0018 — were authored earlier and live mid-document due to earlier-task insertions; numbering-wise, 0019 is next and the file-order tail is where it belongs per the "append to tail" instruction). All four gate checks green: `go vet ./...` clean, `golangci-lint run ./test/helpers/` clean, `go test ./test/helpers/ -v` 4/4 PASS (3 new + pre-existing `TestTCPRoundTrip_EchoBackend`), full `go test ./... -timeout 5m` green.

**Outputs:**

```
$ GOTOOLCHAIN=local go test ./test/helpers/ -v
=== RUN   TestParseHTTPResponse_Simple
--- PASS: TestParseHTTPResponse_Simple (0.00s)
=== RUN   TestParseHTTPResponse_MultiValueHeader
--- PASS: TestParseHTTPResponse_MultiValueHeader (0.00s)
=== RUN   TestParseHTTPResponse_Malformed
--- PASS: TestParseHTTPResponse_Malformed (0.00s)
=== RUN   TestTCPRoundTrip_EchoBackend
--- PASS: TestTCPRoundTrip_EchoBackend (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s

$ GOTOOLCHAIN=local go vet ./...
(no output — exit 0)

$ GOTOOLCHAIN=local golangci-lint run ./test/helpers/
(no output — exit 0)

$ GOTOOLCHAIN=local go test ./... -timeout 5m
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.130s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	(cached)
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
ok  	github.com/esalaine/envoy-go/test/differential	1.780s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s
```

## Task 12 — Cutover — rewire cmd/envoy-go to bootstrap + admin [ADR-0020]

**Commits:** `08e09a9`

**Notes:** The largest-in-phase single-commit cutover switches every caller of the phase-00 minimal-schema config contract to the phase-01 Envoy v3 Bootstrap contract simultaneously — no intermediate commits could stay green. `cmd/envoy-go/main.go` is rewritten to call `bootstrap.Load` + `bootstrap.AdminSocket` + `bootstrap.FirstListenerSocket` + `bootstrap.FirstClusterEndpointSocket`, stand up `admin.New(adminAddr)` with `Start()` + `MarkReady()` before the ready sentinel, and otherwise reuse the phase-00 `pump` / `halfClose` / `netConn` logic byte-for-byte (SPEC §5.3 requires the pump be untouched). The ready sentinel format is preserved byte-exact — `envoy-go ready on <listenerAddr>\n` — so `harness.readyAddr`'s `strings.TrimRight` parser continues to work without modification; the admin address flows through the new pre-allocated `subjAdminAddr` channel threaded through `StartSubjectProxy` rather than being scraped from stdout. `admSrv.Start()`'s bound-address return is discarded with `_` because the caller pre-allocates the admin port and interpolates it into the bootstrap before starting the subject, so the bound value is always identical to what the caller already knows. `cmd/envoy-go/main_test.go`'s `TestEnvoyGoBinary_EchoesThroughUpstream` is rewritten in place (per ADR-0020: same file, same test name) — the test now allocates `adminPort` in addition to `listenerPort`, emits bootstrap-shaped YAML with three port interpolations (admin, listener, backend), and uses the unchanged `waitForReady` helper against the unchanged sentinel format. `test/fixtures/0000-tcp-echo/envoy-go.yaml` is rewritten with bootstrap shape and three documented divergences from the reference `envoy.yaml` (STATIC vs STRICT_DNS cluster type, no `dns_lookup_family`, `127.0.0.1` addresses vs `0.0.0.0` + `host.docker.internal`). `test/fixtures/0000-tcp-echo/driver/driver.go`'s `SubjectConfig` is rewritten to the 4-port template (`refListenerPort` discarded via `_ =`, admin/listener/backend interpolated); the `echoDriver` receiver type and `RegisterFixture` init hook are unchanged. `test/differential/fixture/fixture.go`'s `Driver.SubjectConfig` interface method gains the fourth `subjAdminPort int` parameter. `test/differential/harness.go`'s `SubjectProxy` gains an `adminAddr string` field populated by the new `subjAdminAddr string` parameter on `StartSubjectProxy`, plus a public `AdminAddr() string` getter — both Task 14 and future phases consume this. `test/differential/runner_test.go`'s `runFixture` allocates `subjAdminPort := freeTCPPort(t)` alongside `subjPort`, threads it into both `d.SubjectConfig(...)` and `StartSubjectProxy(...)`. No `ProbeAdmin` wiring yet — Task 14 owns that surface. PLAN did not list `test/differential/harness_test.go` under modified files but `TestSubjectProxy_StartsAndReports` inside it referenced the phase-00 schema and called `StartSubjectProxy` with the 3-arg signature, so it would not compile; updated it in-place to the new bootstrap shape + 4-arg call and added a symmetric `AdminAddr()` assertion. The phase-00 `cmd/envoy-go/config.go` + `config_test.go` files are left untouched — they compile as orphan dead code (no `main.go` caller, no other importer) and their `TestLoadConfig_*` tests still pass because `loadConfig` is defined adjacent; Task 13 deletes them in a purely mechanical diff per the PLAN scope note. ADR-0020 appended to `DECISIONS.md` pinning the rewrite-vs-replacement choice for `main_test.go`.

**Outputs:**

```
$ GOTOOLCHAIN=local go test ./... -timeout 10m
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.577s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	0.038s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.008s
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
ok  	github.com/esalaine/envoy-go/test/differential	2.871s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	0.002s

$ GOTOOLCHAIN=local go test ./test/differential/... -v -timeout 5m
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
--- PASS: TestReferenceProxy_Starts (0.99s)
=== RUN   TestSubjectProxy_StartsAndReports
--- PASS: TestSubjectProxy_StartsAndReports (0.48s)
=== RUN   TestDifferential
=== RUN   TestDifferential/0000-tcp-echo
--- PASS: TestDifferential (1.22s)
    --- PASS: TestDifferential/0000-tcp-echo (1.22s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.767s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
```

## Task 13 — Delete phase-00 cmd/envoy-go/config.go + config_test.go [ADR-0021]

**Commits:** `739b1ba`

**Notes:** Mechanical deletion of the two phase-00 orphan files left behind by Task 12's cutover (`08e09a9`). `cmd/envoy-go/config.go` (the minimal-schema `loadConfig` + `Config` struct parsing top-level `listener` / `upstream` blocks) and `cmd/envoy-go/config_test.go` (the `TestLoadConfig_*` happy-path + unknown-field-rejection cases) had no callers after Task 12 rewrote `main.go` to consume `internal/bootstrap.Load` directly — `grep -r loadConfig` across the tree returned only hits inside the two deleted files themselves, confirming no external importer. `cmd/envoy-go/` now contains exactly `main.go` and `main_test.go` per ADR-0021's consequence clause. ADR-0021 appended to `DECISIONS.md` with the mandatory `**Supersedes:** ADR-0007` header per `BOOTSTRAP_PROMPT.md` §4.1 invariant 4; ADR-0007 itself is NOT edited (verified via `git diff --numstat -- docs/envoy-go/DECISIONS.md` showing `35 insertions, 0 deletions` — additions-only, the ADR-0001..ADR-0011 range including ADR-0007 is byte-identical to the pre-commit tree). The new configuration contract is fully codified by ADR-0012 (yaml.v3 → json → protojson pipeline), ADR-0013 (`github.com/envoyproxy/go-control-plane/envoy` proto-types pin), and ADR-0016 (`DiscardUnknown: false` strict-unknown-field rejection + Any preservation exception). Doctrine D-3.6 (green build) satisfied: `go vet ./...` clean, `golangci-lint run ./...` clean, `go test ./... -timeout 10m` all packages green including the cmd-level `TestEnvoyGoBinary_EchoesThroughUpstream` that Task 12 rewrote in place.

**Outputs:**

```
$ git rm cmd/envoy-go/config.go cmd/envoy-go/config_test.go
rm 'cmd/envoy-go/config.go'
rm 'cmd/envoy-go/config_test.go'

$ ls cmd/envoy-go/
main.go
main_test.go

$ GOTOOLCHAIN=local go vet ./...
(no output — clean)

$ GOTOOLCHAIN=local golangci-lint run ./...
(no output — clean)

$ GOTOOLCHAIN=local go test ./... -timeout 10m
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.511s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	(cached)
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
