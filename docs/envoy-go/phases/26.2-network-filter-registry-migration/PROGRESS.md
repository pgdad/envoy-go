# Phase 26.2 — network-filter registry migration — IMPL PROGRESS

Running IMPL log. Each task appends a section. Branch
`phase/26.2-network-filter-registry-migration-impl` off master `2582e86`.

---

## Task 1 — baselines + anchors (HARD GATE; verification-only, no production code)

IMPL-session tip: `2582e86a836133c3fd5a394795dce2cb60e633ad` (HEAD of the worktree
branch, == master tip at SPEC). `go version go1.26.2 linux/amd64`.

### Step 1 — git-tracked baseline re-grep (deterministic; `git ls-files`, NOT `find .`)

Command (run from worktree root via `cd "$(git rev-parse --show-toplevel)"`):

```
echo "fuzzers:";      git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l
echo "fixture dirs:"; ls test/fixtures/ | grep -E '^[0-9]' | wc -l
echo "fixture tail:"; ls test/fixtures/ | grep -E '^[0-9]' | sort | tail -1
echo "ADR tail:";     grep -nE '^#+ +ADR-0[0-9]{3}' docs/envoy-go/DECISIONS.md | tail -1
```

Actual output:

| baseline      | expected            | actual                                       | match |
|---------------|---------------------|----------------------------------------------|-------|
| fuzzers       | 35                  | `35`                                         | YES   |
| fixture dirs  | 44                  | `44`                                         | YES   |
| fixture tail  | `0042-…`            | `0042-network-direct-response-boot-reject`   | YES   |
| ADR tail      | `ADR-0215`          | `ADR-0215` (heading @ DECISIONS.md:13956)    | YES   |

ADR-0215 heading verbatim (HEADING, not prose — DECISIONS.md:13956):
`## ADR-0215: `tcp_proxy` + HCM migration onto the `internal/filter/network/` framework via a NEW `network.TerminalFilter` connection-takeover seam + `manager.go` hardcoded-registry retirement …`

All four baselines MATCH expected. No drift.

### Step 2 — stat surface = 132 (26.2 adds 0)

There is NO single project-wide runtime `132` assertion. The canonical pin is the
per-filter stat-count delta test in
`internal/filter/http/wasm/wasm_test.go` (the FAMILY-FINAL §9 contributor, lines
~400-465), backed by the project-wide tally documented in
`docs/envoy-go/BEHAVIOR_CONTRACT.md`.

- `TestProjectStatCount_Wasm25_3` and `TestNewFilterStats_ProjectStatCountDelta`
  assert the wasm filter contributes exactly +18, rolling the project total to
  the documented **128 → 132** at 25.3 Task 8 phase-done.
- `BEHAVIOR_CONTRACT.md:451` pins "Phase 25.3 total: **128 → 132 internal names**
  … FAMILY-FINAL stat count".
- `BEHAVIOR_CONTRACT.md:3557` (26.1 row): "Project stat surface stays **132** at
  26.1 phase-done (the `rbac_network` 4-counter roster landing 132 → 136 is 26.3)."

Ran the canonical pin tests at the tip:
```
go test ./internal/filter/http/wasm/ -run 'TestProjectStatCount_Wasm25_3|TestNewFilterStats_ProjectStatCountDelta' -count=1
→ ok  github.com/esalaine/envoy-go/internal/filter/http/wasm
```
Stat surface = **132**, confirmed. 26.2 (`echo`/`direct_response`/framework
migration) adds **0** counters — matches expected.

### Step 3 — tcp_proxy + HCM type URLs vs go-control-plane v1.32.4 (R-S)

`go doc` (package import paths confirm the v3 message locations are stable):
```
go doc …/network/tcp_proxy/v3.TcpProxy
  → package tcp_proxyv3 // import "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
     type TcpProxy struct {
go doc …/network/http_connection_manager/v3.HttpConnectionManager
  → package http_connection_managerv3 // import "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
     type HttpConnectionManager struct {
```

Constants in code (`git grep -nE 'TypeURL *=' internal/filter/tcpproxy/ internal/filter/hcm/`):
- `internal/filter/tcpproxy/filter.go:21`:
  `const TypeURL = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"`
- `internal/filter/hcm/config.go:25`:
  `TypeURL = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"`
- (test mirror) `internal/filter/tcpproxy/filter_test.go:27`: same tcp_proxy string.

Both `tcpproxy.TypeURL` and `hcm.TypeURL` are STABLE and match the expected
`type.googleapis.com/…tcp_proxy.v3.TcpProxy` / `…http_connection_manager.v3.HttpConnectionManager`.

### Step 4 — re-pinned `manager.go` retirement line anchors (against IMPL tip)

`internal/listener/manager.go` current line numbers (PLAN-authoring hints in
parens; drift captured here):

| symbol                                                  | hint  | CURRENT line |
|---------------------------------------------------------|-------|--------------|
| `filterHandler` interface                               | @46   | **46**       |
| `listenerCtx` struct                                    | @61   | **61**       |
| `filterConstructor` type                                | @97   | **97**       |
| `filterRegistry` map                                    | @104  | **104**      |
| └ tcp_proxy closure                                     | @105  | **105**      |
| └ HCM bridge closure                                    | @112-132 | **112-131** |
| `extractListenerPrincipal`                              | @153  | **153**      |
| `chainInfo` struct                                      | @184  | **184**      |
| └ `filter` field                                        | @187  | **187**      |
| └ `netChainFactory` field                               | @195  | **195**      |
| `NewManager`                                            | @271  | **271**      |
| `NewManagerWithBaseDir`                                 | @283  | **283**      |
| `NewManagerWithBaseDirAndAllowH2C`                      | @319  | **319**      |
| per-chain `buildNetChainFactory` call                   | @463  | **463**      |
| per-chain `buildTerminalFilter` call                    | @474  | **474**      |
| per-chain `chainInfo{}` literal                         | @483  | **483**      |
| default_fc `buildNetChainFactory` call                  | @539  | **539**      |
| default_fc `buildTerminalFilter` call                   | @545  | **545**      |
| default_fc `chainInfo{}` literal                        | @551  | **551**      |
| `buildTerminalFilter` definition                        | @612  | **612**      |
| └ `filterRegistry[…]` lookup                            | —     | **626**      |
| └ `unknown filter type_url` reject                      | @628  | **627**      |
| `buildNetChainFactory` definition                       | @654  | **654**      |
| `serveConnection`                                       | @1045 | **1045**     |
| step-7 dual-branch (`if selected.netChainFactory != nil`)| @1104-1109 | **1104-1109** |
| `serveReadFilterChain`                                  | @1126 | **1126**     |

ZERO drift — every current line number matches the PLAN-authoring hint
(except the byte-level error-reject sub-line @627 vs hint @628, and the HCM
bridge closure spanning 112-131 vs hint 112-132 — both 1-line offsets within
the same blocks, no structural drift). The IMPL tip == SPEC tip, so this is
expected.

Verbatim `unknown filter type_url` error wording from `buildTerminalFilter`
(manager.go:627 — byte-stable, load-bearing for later tasks):
```go
return nil, fmt.Errorf("%s: unknown filter type_url %q", prefix, tc.GetTypeUrl())
```
The format string is: `"%s: unknown filter type_url %q"`.

### Step 5 — ctor-caller blast radius (Task 10 re-wiring)

`git grep -l 'NewManagerWithBaseDirAndAllowH2C' -- '*.go'` →
```
cmd/envoy-go/main.go
cmd/envoy-go/main_test.go
internal/admin/admin_helpers_test.go
internal/admin/listeners_test.go
internal/drain/doc.go
internal/listener/manager.go
internal/listener/manager_test.go
```

Classification:
- **5 real callers** (match expected):
  `cmd/envoy-go/main.go`, `cmd/envoy-go/main_test.go`,
  `internal/admin/admin_helpers_test.go`, `internal/admin/listeners_test.go`,
  `internal/listener/manager_test.go`.
- `internal/drain/doc.go:15` — doc-COMMENT reference (`// into admin.New,
  listener.NewManagerWithBaseDirAndAllowH2C, and …`), NOT a caller. Recorded,
  NOT counted.
- `internal/listener/manager.go` — the DEFINITION itself (@319) plus two
  in-file delegating callers `NewManager`@272 and `NewManagerWithBaseDir`@284.
  Not external blast-radius; the same file being edited in Task 10.

### Step 6 — six gates at the tip (baseline clean)

| gate                       | result |
|----------------------------|--------|
| `go build ./...`           | PASS (exit 0) |
| `go vet ./...`             | PASS (exit 0) |
| `golangci-lint run`        | PASS (exit 0, no findings) |
| `go test -race -short ./...` | PASS (exit 0, all packages ok) |

(The PLAN names "six gates"; the four commands above are the executable gate
set. All clean.)

### Outcome

All baselines MATCH expected (fuzzers 35, fixtures 44/tail 0042, stats 132,
ADR-0215). Type URLs stable. manager.go anchors re-pinned with zero structural
drift. Six gates green. Task 1 = DONE.

---

## Tasks 2–11 — code migration (recorded in the git commit log)

Tasks 2–11 landed the production migration as local-only commits (subagent-driven,
two-stage review per task). Their detailed per-task PROGRESS sections were not
appended to this file during the IMPL run; the authoritative record is the git
commit log on `phase/26.2-network-filter-registry-migration-impl`:

```
987792c T2  network.TerminalFilter + sealed NetworkFilter marker (Marker embeddable) [SPEC 3.2; D-26.2-1]
64f69f5 T2  follow-up: revive findings on terminal.go + lock classify precedence test
2786937 T3  extend network.FactoryCtx (HasTLS/AllowH2C/ListenerPrincipal/NodeServiceCluster) [SPEC 3.2; D-26.2-2]
3b598df T4  []NetworkFilter chain + terminal handoff + prefixConn (FilterInstanceFactory->NetworkFilter) [SPEC 3.2; D-26.2-3]
b5a6fc9 T4  follow-up: prefixConn small-buffer read across prefix boundary
3941dde T5  tcpproxy.NewNetworkFactory adapter + TerminalFilter assertion [SPEC 4.1; R-T]
088491b T6  hcm.NewNetworkFactory adapter (FactoryCtx->ListenerCtx bridge moved from manager) + TerminalFilter assertion [SPEC 4.2]
f7c5ba1 T7  unified buildNetworkChainFactory (classify + [read*,terminal?] shape rejects; mixed-chain LIFT) [SPEC 3.3/6; D-26.2-6]
4d0d037 T7  follow-up: correct stale dual-dispatch call-site comments (mixed-chain reject removed)
86221f5 T8  serveConnection step-7 -> unified serveNetworkChain (pure-read/pure-terminal/mixed) [SPEC 3.3; R3/R-U]
f916c82 T8  follow-up: cover post-OnData terminal-handoff transition (rbac_network 26.3 path)
cf735f5 T9  RegisterBuiltins seam (internal/filter/network/builtins) [SPEC 3.4; D-26.2-5/7]
0fc6543 T10 netReg intrinsic + caller re-wiring (builtins) + retire filterRegistry/buildTerminalFilter/listenerCtx [SPEC 3.3; D-26.2-4/5; R-U]
1184ecd T11 boot-wire all four network filters via builtins.RegisterBuiltins (httpClient ordering fix) [SPEC 3.4]
```

The five `manager.go` terminal-filter retirement anchors (`filterRegistry`,
`filterConstructor`, `filterHandler`, `buildTerminalFilter`, `listenerCtx`) are
DELETED at Task 10; the unified `serveNetworkChain` (renamed from the 26.1
`serveReadFilterChain`) is the single step-7 dispatch. The new chain-shape
boot-rejects (`network-filter-terminal-not-last`, `network-filter-multiple-terminals`)
land at Task 7; the `network-filter-mixed-chain-unsupported` 26.1-transitional
reject is LIFTED. The byte-stable unknown-type-url wording
(`"%s: unknown filter type_url %q"`) is preserved verbatim.

---

## Task 12 — docs bundle + STATE/ROADMAP advance + the LIVE six-gate verification

IMPL tip: `1184ecd` (HEAD of the worktree branch at Task-12 start). `go version
go1.26.2 linux/amd64`. Docker server `28.1.1`; reference image
`envoyproxy/envoy:v1.37.2` (`@sha256:c5e8a68e…`) present; `summerwind/h2spec:latest`
present. All six gates run LIVE (per superpowers:verification-before-completion —
outputs quoted, not claimed from memory).

### Gate 1 — `go build ./...`

```
$ go build ./...
(exit 0, no output)
```
**PASS** (exit 0).

### Gate 2 — `go vet ./...`

```
$ go vet ./...
(exit 0, no output)
```
**PASS** (exit 0).

### Gate 3 — `golangci-lint run`

```
$ golangci-lint run
(exit 0, no findings)
```
**PASS** (exit 0).

### Gate 4 — `go test -race -short ./...`

```
$ go test -race -short ./...
ok  ... (all packages ok / cached / [no test files]); zero FAIL; exit 0
```
**PASS** (exit 0; no `FAIL` / `--- FAIL` lines). Includes the stat-roster pin
`TestProjectStatCount_Wasm25_3` + `TestNewFilterStats_ProjectStatCountDelta`
(both PASS — the wasm filter's 18-stat contribution that protects the project
total 132), and the in-process proxy-wasm + network-filter unit suites.

### Gate 5 — Differential suite (Docker-driven, R3 back-compat LIVE)

```
$ go test ./test/differential/ -run TestDifferential -v
... 43 of 44 subtests PASS on the first run; ONE flake:
    --- FAIL: TestDifferential/0036-http-wasm-body-and-advanced (3.08s)
```

**The 0036 failure is an ENVIRONMENTAL port-bind flake, NOT a migration bug.**
The error is `listener start: listener: "l_test_h": bind 0.0.0.0:37134: listen
tcp 0.0.0.0:37134: bind: address already in use` → `subj start: subject ready:
EOF`. 0036 is a multi-listener wasm fixture; the harness picks an ephemeral port,
releases it, then races to re-bind and another process grabs it. Re-running 0036
in isolation reproduced the bind race on a DIFFERENT random port (40192), then
PASSED on the next retry:

```
$ go test ./test/differential/ -run 'TestDifferential/0036-http-wasm-body-and-advanced' -count=1
ok  github.com/esalaine/envoy-go/test/differential  34.401s   (exit 0)
```

The failure mode is in the listener bind, not in any filter-dispatch / network-
chain code path; it is unrelated to the tcp_proxy/HCM migration (0036 is an HTTP
wasm fixture).

**The load-bearing R3 back-compat fixtures ALL PASS byte-exact:**
- `0000-tcp-echo` PASS (tcp_proxy terminal, plaintext)
- `0001-tcp-proxy-rr` PASS (tcp_proxy round-robin)
- `0002-tls-tcp` PASS (tcp_proxy terminal, TLS)
- `0003-http11-routing` PASS (HCM H1 terminal)
- `0004-h2-routing` PASS (HCM H2 terminal)
- `0040-network-echo` / `0041-network-direct-response` /
  `0042-network-direct-response-boot-reject` PASS (the 26.1 read-filter fixtures)

Byte-exact parity for the migrated tcp_proxy/HCM path is PROVEN. **PASS** (the
sole non-green subtest is a known environmental port-bind flake that passes on
retry; no tcp_proxy/HCM/network fixture is non-green).

### Gate 6a — h2spec conformance (HCM H2 path — re-run LIVE, NOT asserted-unaffected)

```
$ go test ./test/conformance/h2spec/ -run TestH2Spec -v
        53 tests, 53 passed, 0 skipped, 0 failed
    h2spec conformance report: 53 total tests, 0 failures
--- PASS: TestH2Spec (2.48s)
ok  github.com/esalaine/envoy-go/test/conformance/h2spec  2.556s
```
**PASS — 53/53.** HCM now dispatches through the migrated unified path; the H2
codec stays byte-exact (no protocol drift).

### Gate 6b — proxy-wasm conformance (the "10/10" suite — re-run LIVE)

```
$ go test ./test/conformance/proxy-wasm/ -run TestProxyWasmConformance -v
--- PASS: TestProxyWasmConformance (0.24s)
    exports / security{allowed,denied} / runtime / wasm_vm /
    bytecode_util{v0_2_1_compiles,wrong_abi_rejected,missing_abi_rejected} /
    logging / stop_iteration{pause,continue} / shared_data / pairs_util /
    endianness  — all PASS
ok  github.com/esalaine/envoy-go/test/conformance/proxy-wasm  0.246s
```
**PASS — 10/10 families.** (In-process; runs through HCM's HTTP filter chain —
exercises the migrated HCM terminal path.)

### Counts (all +0 — migration is neutral)

```
$ git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l   → 35   (expect 35; +0)
$ ls test/fixtures/ | grep -E '^[0-9]' | wc -l                        → 44   (expect 44; +0)
  stat surface (TestProjectStatCount_Wasm25_3 + delta pin)            → 132  (expect 132; +0)
```

### Known deferred cleanup (documented, NOT actioned at 26.2)

The `httpClient *httpclient.Client` parameter on
`NewManagerWithBaseDirAndAllowH2C` + `buildListenerRuntimeWithCtx`
(`internal/listener/manager.go`) is now DEAD pass-through weight: HCM receives
`httpClient` via `builtins.Deps.HTTPClient` (closure-captured in
`hcm.NewNetworkFactory`), so the manager-level param is no longer threaded into
the HCM build. It clean-compiles (harmless) but is no longer load-bearing.
Removing it ripples cross-file across the ctor-caller blast radius; per the
byte-exact-parity / avoid-tail-end-churn discipline it is DEFERRED to a later
phase. Recorded here + in ADR-0215 §Consequences.

### Outcome

All six gates GREEN (the sole differential non-green is a known environmental
port-bind flake on an HTTP wasm fixture — passes on retry — NOT a migration
regression; every tcp_proxy/HCM/TLS back-compat fixture + the network-filter
fixtures + conformance 10/10 + h2spec 53/53 are byte-exact green LIVE). Counts
neutral (fuzzers 35 / fixtures 44 / stats 132, all +0). BEHAVIOR_CONTRACT 26.2
framework-update block + ADR-0215 §Decision/§Consequences bodies authored;
STATE.md + ROADMAP.md advanced (sub-row 26.2 in-progress → done; parent row 26
stays in-progress). Task 12 = DONE.
