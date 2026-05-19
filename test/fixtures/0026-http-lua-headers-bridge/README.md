# Fixture 0026 — http-lua-headers-bridge

Differential fixture for `envoy.filters.http.lua` at phase 22.1
(headers-bridge MVP). Asserts byte-equivalence between envoy-go's
implementation and reference Envoy v1.37.2 across a 7-scenario matrix
covering the 22.1 bridge surface: `:headers()` add/replace/remove,
`:respond()` short-circuit, `:logInfo()` pass-through, `pairs()`
iteration count, and script-compile-error boot rejection.

Status: Task 14 lands the fixture directory + driver + 7 `.lua` source
files. **Fixture-0026 GREEN is DEFERRED to Task 15** which adds the
`"script load error: "` wording-pinning at `cmd/envoy-go/main.go`
boot-reject path per parent §13-W. With Task 15 the fixture lights up
end-to-end: 6 scenarios (a)-(f) full cross-side byte-exact via
`CompareBytes`; scenario (g) substring-match via `BootRejectFixture`.

## Topology

Six-listener topology — one listener per wire-interactive scenario;
each listener carries one `envoy.filters.http.lua` HTTP filter consuming
`Lua.default_source_code` via the `DataSource.Filename` arm pointing at
a per-scenario `.lua` source file. All listeners share one upstream
cluster `c_backend` → SHARED echobackend (`test/helpers/echobackend/cmd/
echobackend/`) which reflects request headers as JSON in the response
body (the driver classifies the reflected JSON to assert the Lua-mutated
header set).

```
                  ┌─→ l_test_a:10026 ─→ filter chain [lua → router] (a_add_header.lua)
                  ├─→ l_test_b:10027 ─→ filter chain [lua → router] (b_replace_header.lua)
client (driver) ──┼─→ l_test_c:10028 ─→ filter chain [lua → router] (c_remove_header.lua)
                  ├─→ l_test_d:10029 ─→ filter chain [lua → router] (d_respond.lua)
                  ├─→ l_test_e:10030 ─→ filter chain [lua → router] (e_log_only.lua)
                  └─→ l_test_f:10031 ─→ filter chain [lua → router] (f_headers_iter.lua)
                                              │
                                              └─ cluster c_backend ─→ echobackend
                                                                       (reflects req headers as JSON)
```

Reference Envoy reaches the host-spawned echobackend via
`host.docker.internal:<BackendPort>` per ADR-0010. The per-scenario
`.lua` files are bind-mounted from `test/fixtures/0026-http-lua-headers-
bridge/scripts/` into the reference container at `/scripts/` via
`ReferenceHostMounts()` (mirrors the fixture-0019 PKI-mount precedent).
envoy-go (subject) runs on the host directly + reads the host-side
`scripts/` files via absolute path.

The driver implements `fixture.MultiListenerDriver` so the runner
exposes all 6 container ports + dials each per-listener host-mapped
addr — `DriveSubjectMulti` / `DriveReferenceMulti` receive the per-name
addr map.

## 7-scenario taxonomy

Per phase 22.1 SPEC §9.1 + parent §8.2:

| # | Name | Lua script | Wire-output assertion (cross-side) |
|---|---|---|---|
| (a) | `a_add_header` | `rh:headers():add("x-lua-injected", "hello")` | Reflected request header `x-lua-injected: hello` present at echobackend (driver classifies JSON body via `reflectedHeaders` helper) |
| (b) | `b_replace_header` | `rh:headers():replace("user-agent", "envoy-go-lua/1.0")` | Reflected `user-agent: envoy-go-lua/1.0` (driver injects baseline `user-agent: integration-test/0.1` so the replace has something to replace) |
| (c) | `c_remove_header` | `rh:headers():remove("x-blocked")` | Reflected request without `x-blocked` header (driver injects `x-blocked: yes` so the remove has something to remove) |
| (d) | `d_respond` | `rh:respond({[":status"]="403"}, "denied")` | Client receives byte-pinned tuple: status `403`; body `denied` (6 bytes, no trailing newline); `content-length: 6`; `content-type: text/plain`; NO upstream round-trip. Per parent §11.6.7 + AMEND-7. |
| (e) | `e_log_only` | `rh:logInfo("lua hit")` | Reflected request unchanged at upstream + **stat-counter delta** `envoy_http_lua_scenario_e_executions{envoy_http_conn_manager_prefix="hcm_e"}` increments by 1 per probe. Per D3 closure (parent §11.7.7 option (a)): the literal log line is NOT cross-side asserted (gopher-lua vs spdlog formatting diverges per AMEND-9); the stat-counter IS the "Lua ran" assertion + lives in `AssertStats` (NOT inline in Drive — the driver does not have access to either admin addr during Drive). |
| (f) | `f_headers_iter` | `local n=0; for k,v in pairs(rh:headers()) do n=n+1 end; rh:headers():add("x-headers-count", tostring(n))` | Reflected header `x-headers-count: N` where N is the probe's request-header count. Count-only deterministic per §11 D7 closure — bridge `__pairs` snapshots `http.Header` alphabetically + walks by index; both sides agree on N regardless of iteration order. |
| (g) | `g_compile_error` | Lua source with intentional syntax error | **Both sides exit non-zero at config-load + both sides' stderr contains literal substring `"script load error"`** (case-sensitive Contains) per AMEND-10 option 2. Wire-orthogonal: scenario (g) does NOT round-trip; the runner's `BootRejectFixture` branch (per parent §13-R1 + Task 13) asserts the boot rejection on both sides. envoy-go-side wording-pin lands at Task 15 (`cmd/envoy-go/main.go:60-66` wraps gopher-lua's compile error with `"script load error: "` prefix). |

## Directory layout

```
test/fixtures/0026-http-lua-headers-bridge/
  README.md             # this file
  envoy.yaml            # reference Envoy bootstrap (6 listeners + lua filter chain)
  envoy-go.yaml         # subject envoy-go bootstrap (same topology; host paths)
  expectations.yaml     # human-readable per-scenario expectations (doc aid)
  inputs/
    driver.go           # registered Driver impl + MultiListenerDriver + BackendKindAware
                        # + ReferenceLogMounter + StatsAsserter + BootRejectFixture
  scripts/              # per-scenario .lua source files (Filename DataSource arm)
    a_add_header.lua
    b_replace_header.lua
    c_remove_header.lua
    d_respond.lua
    e_log_only.lua
    f_headers_iter.lua
    g_compile_error.lua # intentional syntax error
```

The `scripts/` subdirectory exploits the `DataSource.Filename` arm
naturally (vs all-inline-strings collapsed into the YAML) — adds
DataSource-arm coverage for free + improves per-scenario readability
per parent §8.4 + AMEND-11. Mirrors the fixture-0019 PKI-subdirectory
pattern.

## Cross-side scope summary

- **Scenarios (a)-(f)**: full cross-side byte-exact via `CompareBytes`.
  The Drive byte stream emits per-scenario lines of the form
  `scenario <id> status=<code> body=<verdict>` (see `emitScenario` +
  `classifyBody` in `inputs/driver.go`). Body verdicts insulate from
  non-substantive byte divergences (e.g., upstream-only headers in the
  reflected JSON) via per-scenario classification.
- **Scenario (g)**: substring-match `"script load error"` via
  `BootRejectFixture` interface (NOT byte-exact stderr). The runner's
  `runBootRejectFixture` branch renders both bootstraps with all 6
  listener slots pointing at `g_compile_error.lua`, asserts both
  proxies exit non-zero, then asserts both stderr buffers contain the
  literal substring `"script load error"` (case-sensitive).
- **Scenario (e) stat-counter delta** lives in `StatsAsserter.
  AssertStats` (called by the runner with both admin addrs after Drive
  completes); both sides MUST agree the executions counter for HCM
  prefix `hcm_e` and Lua stat_prefix `scenario_e` equals 1 after one
  probe.

## Boot-reject mode

The driver's `BootRejectScript()` returns `scripts/g_compile_error.lua`
+ flips an internal `bootRejectMode` flag. The runner's
`runBootRejectFixture` branch (`test/differential/runner_test.go`) calls
`BootRejectScript()` ONCE at branch entry; subsequent
`ReferenceBootstrap` + `SubjectConfig` calls splice the broken script
path into ALL 6 listener slots (any one listener's lua filter triggers
the boot-reject before any listener binds — symmetric across both
sides). `ExpectedBootErrorSubstring()` returns the literal
`"script load error"` per AMEND-10 + parent §11.7.5 + §13-W. The
discipline is documented in the `BootRejectFixture` interface at
`test/differential/harness.go` per Task 13.

## Cross-references

- **22.1 SPEC §9** — 7-scenario fixture-0026 details + driver impl shape.
- **parent §8.1-§8.5** — fixture-0026 disposition + AMEND-10 substring-match.
- **parent §11.6.7 + AMEND-7** — `:respond()` wire-pin (scenario d).
- **parent §11.7.7 + D3 closure** — scenario (e) option (a).
- **parent §13-R1** — `BootRejectFixture` infra (Task 13).
- **parent §13-W** — envoy-go-side `"script load error: "` wording-pin (Task 15).
- **AMEND-11** — `BackendKind=HTTPLua` + `scripts/` subdir.
- **ADR-0188** — `internal/lua/` framework primitive.
- **ADR-0189** — `internal/filter/http/lua/` package shape.
