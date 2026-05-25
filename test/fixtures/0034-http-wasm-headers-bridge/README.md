# Fixture 0034 — http-wasm-headers-bridge

Differential fixture for `envoy.filters.http.wasm` at phase 25.1
(headers-bridge MVP). Asserts byte-equivalence between envoy-go's
implementation (wazero runtime) and reference Envoy v1.34.0 (V8
runtime) across a 7-scenario matrix covering the 25.1 ABI surface:
add/replace/remove header, send-local-response short-circuit, log-
only pass-through, header-map-pairs iteration, and property-tree
read for `request.method`.

Status: Task 15 lands the fixture directory + driver + 7 Rust source
crates + 7 vendored `.wasm` blobs + `BackendKind=HTTPWasm` registration.
Task 16 lands the companion boot-reject fixture (`0035-http-wasm-
boot-reject`).

## Topology

Seven-listener topology — one listener per scenario; each listener
carries one `envoy.filters.http.wasm` HTTP filter consuming
`Wasm.config.vm_config.code.local.filename` pointing at a per-scenario
`.wasm` source file under `bytecode/`. All listeners share one upstream
cluster `c_backend` → SHARED echobackend
(`test/helpers/echobackend/cmd/echobackend/`) which reflects request
headers as JSON in the response body (the driver classifies the
reflected JSON to assert the WASM-mutated header set).

```
                  +-> l_test_a:10034 -> filter chain [wasm -> router] (bytecode/a_add_header.wasm)
                  +-> l_test_b:10035 -> filter chain [wasm -> router] (bytecode/b_replace_header.wasm)
                  +-> l_test_c:10036 -> filter chain [wasm -> router] (bytecode/c_remove_header.wasm)
client (driver) -+-> l_test_d:10037 -> filter chain [wasm -> router] (bytecode/d_respond_shortcircuit.wasm)
                  +-> l_test_e:10038 -> filter chain [wasm -> router] (bytecode/e_log_only.wasm)
                  +-> l_test_f:10039 -> filter chain [wasm -> router] (bytecode/f_header_iter.wasm)
                  +-> l_test_g:10040 -> filter chain [wasm -> router] (bytecode/g_property_method.wasm)
                                              |
                                              +- cluster c_backend -> echobackend
                                                                       (reflects req headers as JSON)
```

Reference Envoy reaches the host-spawned echobackend via
`host.docker.internal:<BackendPort>` per ADR-0010. The per-scenario
`.wasm` blobs are bind-mounted from `test/fixtures/0034-http-wasm-
headers-bridge/bytecode/` into the reference container at `/bytecode/`
via `ReferenceHostMounts()` (mirrors the fixture-0026 scripts/ mount
precedent). envoy-go (subject) runs on the host directly + reads the
host-side `bytecode/` files via absolute path.

The driver implements `fixture.MultiListenerDriver` so the runner
exposes all 7 container ports + dials each per-listener host-mapped
addr — `DriveSubjectMulti` / `DriveReferenceMulti` receive the per-name
addr map.

## 7-scenario taxonomy

Per phase 25.1 SPEC §9.1:

| # | Name | Plugin call (Rust SDK) | Wire-output assertion (cross-side) |
|---|---|---|---|
| (a) | `a_add_header` | `self.add_http_request_header("x-wasm-injected", "hello")` | Reflected request header `x-wasm-injected: hello` present at echobackend |
| (b) | `b_replace_header` | `self.set_http_request_header("user-agent", Some("envoy-go-wasm/1.0"))` | Reflected `user-agent: envoy-go-wasm/1.0` (driver injects baseline `user-agent: integration-test/0.1` so the replace has something to replace) |
| (c) | `c_remove_header` | `self.set_http_request_header("x-blocked", None)` | Reflected request without `x-blocked` header (driver injects `x-blocked: yes` so the remove has something to remove) |
| (d) | `d_respond_shortcircuit` | `self.send_http_response(403, vec![], Some(b"denied"))` + `Action::Pause` | Client receives byte-pinned tuple: status `403`; body `denied` (6 bytes); `content-length: 6` auto-set; NO upstream round-trip |
| (e) | `e_log_only` | `hostcalls::log(LogLevel::Info, "wasm hit")` | Reflected request unchanged at upstream + **stat-counter delta** `wasm.<plugin>.executions` increments by 1 per probe. The literal log line is NOT cross-side asserted (wazero log sink format vs spdlog format diverges); the stat-counter IS the "wasm ran" assertion + lives in `AssertStats` (StatsAsserter; mirrors fixture-0026 D3 closure for lua). |
| (f) | `f_header_iter` | `self.get_http_request_headers().len()` + add `x-headers-count: N` | Reflected header `x-headers-count: N` where N is the request-header count. Count-only deterministic per parent §4.5 D6 guardrail (b) — both sides sort by name in `GetHeaderMap`. |
| (g) | `g_property_method` | `self.get_property(vec!["request", "method"])` + add `x-request-method: GET` | Reflected header `x-request-method: GET`. The driver always issues GET so both sides produce the same string. |

§4.5 guardrail compliance: all 7 scenarios use only the 24-hostcall
surface; HTTP/1.1 only; no memory traps; no float-formatted logs.

## Directory layout

```
test/fixtures/0034-http-wasm-headers-bridge/
  README.md             # this file
  envoy.yaml            # reference Envoy bootstrap (7 listeners + wasm filter chain)
  envoy-go.yaml         # subject envoy-go bootstrap (same topology; host paths)
  expectations.yaml     # human-readable per-scenario expectations (doc aid)
  inputs/
    driver.go           # registered Driver impl + MultiListenerDriver + BackendKindAware
                        # + ReferenceLogMounter + StatsAsserter
  scripts/              # Rust source crates (NOT built at test time)
    README.md           # operator reproducibility (cargo build invocation)
    .gitignore          # target/ + Cargo.lock (per-crate build artifacts)
    a_add_header/Cargo.toml + src/lib.rs
    b_replace_header/Cargo.toml + src/lib.rs
    c_remove_header/Cargo.toml + src/lib.rs
    d_respond_shortcircuit/Cargo.toml + src/lib.rs
    e_log_only/Cargo.toml + src/lib.rs
    f_header_iter/Cargo.toml + src/lib.rs
    g_property_method/Cargo.toml + src/lib.rs
  bytecode/             # vendored .wasm blobs (committed to git per Q9 + AMEND-A1)
    a_add_header.wasm
    b_replace_header.wasm
    c_remove_header.wasm
    d_respond_shortcircuit.wasm
    e_log_only.wasm
    f_header_iter.wasm
    g_property_method.wasm
```

The vendored `.wasm` blobs are committed so the test suite does NOT
require a Rust toolchain to run; operators rebuild via the
`scripts/README.md` instructions when modifying the source. Per
AMEND-A1: `proxy-wasm-rust-sdk =0.2.4` pinned exact; the resulting
blobs export `proxy_abi_version_0_2_1` which both reference Envoy
v1.34.0 (V8) and envoy-go (wazero, AMEND-A6 strict-stricter) accept.

## Cross-side scope summary

- **Scenarios (a)-(g)**: full cross-side byte-exact via `CompareBytes`.
  The Drive byte stream emits per-scenario lines of the form
  `scenario <id> status=<code> body=<verdict>` (see `emitScenario` +
  `classifyBody` in `inputs/driver.go`). Body verdicts insulate from
  non-substantive byte divergences (e.g., upstream-only headers in the
  reflected JSON) via per-scenario classification.
- **Scenario (e) stat-counter delta** lives in `StatsAsserter.
  AssertStats` (called by the runner with both admin addrs after Drive
  completes); both sides MUST agree the `wasm.<plugin>.executions`
  counter for plugin `plugin_e` equals 1 after one probe.

## Cross-references

- **25.1 SPEC §9.1** — 7-scenario fixture-0034 details.
- **25.1 SPEC §4.5** — D6 guardrail (24-hostcall surface).
- **25.1 PLAN Task 15** — fixture-0034 file roster.
- **AMEND-A1** — `proxy-wasm-rust-sdk =0.2.4` pin.
- **AMEND-A2** — `wasm.<plugin>.executions` counter surface.
- **AMEND-A6** — envoy-go-strict-stricter (`proxy_abi_version_0_2_1` only).
- **parent §8.5** — `BackendKind=HTTPWasm` enum + dispatch.
- **fixture-0026** — sibling precedent (`HTTPLua`; same structural shape).
