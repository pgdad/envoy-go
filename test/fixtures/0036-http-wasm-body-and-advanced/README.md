# Fixture 0036 — http-wasm-body-and-advanced

Differential fixture for `envoy.filters.http.wasm` at phase 25.2
(body + trailers + tick + shared_data + foreign_function +
http_call + property + dynamic_stats + env_vars). Asserts the
post-25.1 ABI surface across a 14-scenario partition: 10 deterministic
cross-side (via `CompareBytes`) + 4 non-deterministic subject-only (via
`StatsAsserter.AssertStats` per
`reference_differential_asserter_dispatch`).

Per phase 25.2 SPEC §8.1 + Q8 + ADR-0192 precedent + Task 20 PLAN.

## Topology

Fourteen-listener topology — one listener per scenario; each listener
carries one `envoy.filters.http.wasm` HTTP filter consuming
`Wasm.config.vm_config.code.local.filename` pointing at a per-scenario
`.wasm` blob under `bytecode/`. TWO upstream cluster definitions
(`cluster_a` primary + `cluster_b` httpCall target) BOTH point at the
SAME differential echobackend per phase-22.2 REVIEW §7.4 `freeTCPPort`
flake mitigation — single backend allocation, two cluster names.

```
                  +-> l_test_a:10100 -> filter chain [wasm -> router] (bytecode/a_body_read_only.wasm)
                  +-> l_test_b:10101 -> filter chain [wasm -> router] (bytecode/b_body_mutate_passthrough.wasm)
                  +-> l_test_c:10102 -> filter chain [wasm -> router] (bytecode/c_body_mutate_replace.wasm)
                  +-> l_test_d:10103 -> filter chain [wasm -> router] (bytecode/d_trailers_add.wasm)
                  +-> l_test_e:10104 -> filter chain [wasm -> router] (bytecode/e_trailers_read.wasm)
                  +-> l_test_f:10105 -> filter chain [wasm -> router] (bytecode/f_shared_data_read_after_write.wasm)
                  +-> l_test_g:10106 -> filter chain [wasm -> router] (bytecode/g_foreign_function_deny_default.wasm)
client (driver) -+-> l_test_h:10107 -> filter chain [wasm -> router] (bytecode/h_property_stream_info.wasm)
                  +-> l_test_i:10108 -> filter chain [wasm -> router] (bytecode/i_metric_define_only.wasm)
                  +-> l_test_j:10109 -> filter chain [wasm -> router] (bytecode/j_env_vars_rejected_passthrough.wasm)
                  +-> l_test_k:10110 -> filter chain [wasm -> router] (bytecode/k_tick_fires_counter.wasm)
                  +-> l_test_l:10111 -> filter chain [wasm -> router] (bytecode/l_httpcall_success.wasm)
                  +-> l_test_m:10112 -> filter chain [wasm -> router] (bytecode/m_httpcall_unknown_cluster.wasm)
                  +-> l_test_n:10113 -> filter chain [wasm -> router] (bytecode/n_body_cap_exceeded.wasm)
                                              |
                                              +- cluster_a -> echobackend (reflects req as JSON)
                                              +- cluster_b -> echobackend (same backend; httpCall target)
```

## 14-scenario taxonomy

Per phase 25.2 SPEC §8.1.1.

### 10 cross-side (CompareBytes; deterministic)

| # | Name | Plugin call | Cross-side assertion |
|---|---|---|---|
| (a) | `a_body_read_only` | `get_http_request_body` + add `x-body-len: <len>` | Reflected `x-body-len` arrives with same value |
| (b) | `b_body_mutate_passthrough` | Parity of body length → `x-body-tag: <even\|odd>` | Reflected `x-body-tag` matches parity |
| (c) | `c_body_mutate_replace` | Uppercase body via `set_http_request_body` + add `x-body-mutated: 1` | Reflected `x-body-mutated` arrives with `1` |
| (d) | `d_trailers_add` | Add response header `x-trailers-added: 1` | Response `x-trailers-added: 1` |
| (e) | `e_trailers_read` | Read request trailers (0 expected on HTTP/1.1) → response `x-trailer-count: 0` | Response `x-trailer-count: 0` |
| (f) | `f_shared_data_read_after_write` | CAS-loop read/inc/write of `counter`; per-request `x-shared-data-counter: <n>` | After 2 requests, response `x-shared-data-counter: 2` |
| (g) | `g_foreign_function_deny_default` | `call_foreign_function("verify_signature")` → `x-foreign-result: <code>` | Both runtimes return NotFound (1) for unknown function |
| (h) | `h_property_stream_info` | Read `request.method` + `request.path` → response `x-prop-method` + `x-prop-path` | Both headers identical across sides for same probe |
| (i) | `i_metric_define_only` | Define + increment a Counter; tag request header `x-metric-defined: 1` | Reflected `x-metric-defined: 1` |
| (j) | `j_env_vars_rejected_passthrough` | `std::env::vars().count()` → response `x-env-keys: <n>` | Both sides return same count (expected 0 per AMEND-A6) |

### 4 subject-only (StatsAsserter; non-deterministic)

Per `reference_differential_asserter_dispatch`: subject-side assertions
MUST use StatsAsserter (NOT SubjectAsserter, which only runs on the
reference-less path).

| # | Name | Plugin call | Subject-side stat assertion |
|---|---|---|---|
| (k) | `k_tick_fires_counter` | `set_tick_period(50ms)` + `proxy_on_tick` increments counter | `wasm.plugin_k.tick_invocations >= 5` (after 250ms wait) |
| (l) | `l_httpcall_success` | `dispatch_http_call("cluster_b", ...)` | `http_call_dispatched >= 1` + `http_call_response >= 1` |
| (m) | `m_httpcall_unknown_cluster` | `dispatch_http_call("nonexistent_cluster", ...)` | `http_call_dispatch_unknown_cluster >= 1` |
| (n) | `n_body_cap_exceeded` | `Action::Pause` on body callbacks; probe sends 2 KiB > 1 KiB cap | `body_buffer_cap_exceeded >= 1` + `envoy_go.failures >= 1` |

## Deliberate-break liveness verification

Per `reference_differential_asserter_dispatch` + the phase-23 fixture-
0030 dead-vacuous-assertion lesson + the 25.1 Task 15+17 follow-up:
every StatsAsserter arm MUST be proven LIVE by deliberately breaking it,
verifying FAIL, then restoring + verifying GREEN.

See `docs/envoy-go/phases/25.2-http-filter-wasm-body-and-advanced-
bridge/PROGRESS.md` Task 20 entry for the captured FAIL outputs of the
4 subject-only arms + the restored GREEN evidence.

## Directory layout

```
test/fixtures/0036-http-wasm-body-and-advanced/
  README.md             # this file
  envoy.yaml            # reference Envoy bootstrap (14 listeners + wasm filter chain)
  envoy-go.yaml         # subject envoy-go bootstrap (same topology; host paths)
  expectations.yaml     # human-readable per-scenario expectations (doc aid)
  inputs/
    driver.go           # registered Driver impl + MultiListenerDriver +
                        # BackendKindAware + ReferenceLogMounter +
                        # StatsAsserter (4 arms)
  scripts/              # Rust source crates (NOT built at test time)
    README.md           # operator reproducibility (cargo build invocation)
    .gitignore          # target/ + Cargo.lock
    {a..n}_<name>/
      Cargo.toml + src/lib.rs
  bytecode/             # vendored .wasm blobs (committed to git per Q9 + AMEND-A1)
    {a..n}_<name>.wasm
```

The vendored `.wasm` blobs are committed so the test suite does NOT
require a Rust toolchain to run; operators rebuild via the
`scripts/README.md` instructions when modifying the source. Per
AMEND-A1: `proxy-wasm-rust-sdk =0.2.4` pinned exact; the resulting blobs
export `proxy_abi_version_0_2_1` which both reference Envoy v1.37.2
(V8) and envoy-go (wazero, AMEND-A6 strict-stricter) accept.

## Cross-references

- **25.2 SPEC §8.1** — 14-scenario partition table.
- **25.2 SPEC §8.1.1** — per-scenario details.
- **25.2 Q8** — single-listener mixed-mode dispatch.
- **ADR-0192** — single-fixture multi-scenario precedent.
- **R-25.2-11** — fixture-0036 scope ratification.
- **AMEND-A1** — `proxy-wasm-rust-sdk =0.2.4` pin.
- **AMEND-A6** — env_vars deferred to 25.3.
- **AMEND-A9** — empty default ForeignFunctionRegistry.
- **AMEND-B3** — 9 NEW envoy-go-strict counters (tick_invocations,
  http_call_dispatched, http_call_response,
  http_call_dispatch_unknown_cluster, body_buffer_cap_exceeded,
  envoy_go.failures, etc.).
- **D-P-PLAN-12** — vendored .wasm reproduction discipline.
- **phase-22.2 REVIEW §7.4** — freeTCPPort flake mitigation (2-cluster
  topology with single backend).
- **`reference_differential_asserter_dispatch`** memory — StatsAsserter
  discipline for cross-side fixtures + mandatory deliberate-break
  liveness verification.
- **`reference_differential_fixture_dispatch_constraint`** — one fixture
  dir = one runner branch (new `BackendKind=HTTPWasmAdvanced=26`).
- **fixture-0034** — sibling precedent (25.1 HTTPWasm; headers-bridge
  MVP).
