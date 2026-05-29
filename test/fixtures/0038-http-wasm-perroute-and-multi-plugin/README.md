# Fixture 0038 — http-wasm-perroute-and-multi-plugin

Differential fixture for `envoy.filters.http.wasm` at phase 25.3: per-route
wholesale `Wasm` typed_per_filter_config override (AMEND-C1), multi-plugin
`vm_id`-shared VM + shared-data namespace, and `failure_policy=FAIL_RELOAD`
reload recovery. Asserts the 25.3 surface across a 4-arm partition: 3
deterministic cross-side (via `CompareBytes`) + 1 non-deterministic
subject-only (via `StatsAsserter.AssertStats` per
`reference_differential_asserter_dispatch`) — plus a subject-side
complement on the multi-plugin arm.

Per phase 25.3 Task 11.

## Topology

Three-listener mixed-mode topology — ONE upstream cluster `cluster_a`
points at the SHARED differential echobackend per phase-22.2 REVIEW §7.4
`freeTCPPort` flake mitigation.

```
                  +-> l_perroute    -> [wasm listener_default -> router]
                  |       route /override carries a per-route wholesale Wasm
                  |       TPFC override (perroute_override.wasm)
client (driver) -+-> l_multiplugin  -> [wasm A(writer) -> wasm B(reader) -> router]
                  |       both filters share vm_id=vm_shared (shared_data_combined.wasm)
                  +-> l_reload      -> [wasm fail_reload_trap (FAIL_RELOAD) -> router]
                                              |
                                              +- cluster_a -> echobackend
```

## Arm taxonomy

### 2 cross-side (CompareBytes; deterministic)

| arm | listener / route | guest effect | cross-side assertion |
|---|---|---|---|
| `perroute_listener_default` | l_perroute `/default` | no per-route TPFC → listener default sets `x-wasm-variant: listener` | Response `x-wasm-variant: listener` on BOTH sides |
| `multiplugin_shared_data` | l_multiplugin `/multi` | A writes shared key `x0038-shared=written-by-A`; B reads it + sets `x-shared` | Response `x-shared: written-by-A` on BOTH sides (vm_id-scoped sharing; cpp-host shares by vm_id). DISCRIMINATING: a non-shared namespace would reflect `absent`. |

#### Per-route override — a reference-vs-subject DIVERGENCE (subject-only)

The `perroute_override_applies` arm is **subject-only**, NOT cross-side.
Reference Envoy **v1.37.2 does NOT support per-route (route/vhost-specific)
configuration for `envoy.filters.http.wasm`** — boot rejects with:

> The filter envoy.filters.http.wasm doesn't support virtual host or route
> specific configurations

The per-route wholesale `Wasm` override (AMEND-C1) is therefore an
**envoy-go capability not shared by reference v1.37.2**. The reference
bootstrap carries NO per-route TPFC on `/override` (the listener default
applies there). The subject DOES carry the per-route override and its
application is asserted SUBJECT-ONLY via the StatsAsserter (the override
plugin's own scoped `executions` counter increments). The `/override`
response-header byte stream is a constant token across sides. This
divergence is intrinsic to the v1.37.2 pin (not a fixture bug or an
envoy-go bug); see the driver + envoy.yaml notes.

### subject-only (StatsAsserter; non-deterministic)

Per `reference_differential_asserter_dispatch`: subject-side assertions
MUST use StatsAsserter (NOT SubjectAsserter). Reference Envoy's V8 reload
stat names diverge from envoy-go's `wasm.<plugin>.vm_reload_*` triplet, so
the reload counters are subject-only.

| arm | subject-side stat assertion |
|---|---|
| `perroute_override_applies` | `wasm.plugin_perroute_override.executions >= 1` (the per-route override VM dispatched on `/override` — the DISCRIMINATING proof the override took over the stream; the override plugin's own scoped counter only increments if the per-route resolution applied it). |
| `multiplugin_shared_data` (complement) | `wasm.plugin_multi_a.executions >= 1` AND `wasm.plugin_multi_b.executions >= 1` (both filters of the shared-VM chain dispatched their guest). The DISCRIMINATING sharing proof is the cross-side `x-shared` arm above. |
| `reload_fail_reload_recovers` | `wasm.plugin_reload.vm_reload_runtime_failure >= 1` (req1 trap armed the reload) + `wasm.plugin_reload.vm_reload_backoff >= 1` (req2 blocked within the ~1s backoff window) + `wasm.plugin_reload.vm_reload_success >= 1` (req3 past the window reloaded successfully) |

The reload sequence is driven on the SUBJECT side only (req1 trap / req2
within-backoff / sleep 1.3s / req3 recover); the cross-side byte stream for
the reload arm is a constant token.

## Deliberate-break liveness verification

Per `reference_differential_asserter_dispatch` + the phase-23 fixture-0030
dead-vacuous-assertion lesson: every subject-side StatsAsserter arm MUST be
proven LIVE by deliberately breaking it, verifying FAIL, then restoring +
verifying GREEN. The four subject-side arms
(`plugin_multi_a.executions` / `plugin_multi_b.executions` /
`vm_reload_runtime_failure` / `vm_reload_backoff` / `vm_reload_success`)
each have a recorded deliberate-break cycle in
`docs/envoy-go/phases/25.3-http-filter-wasm-perroute-and-conformance/PROGRESS.md`
Task 11 — "deliberate-break liveness" subsection.

## Directory layout

```
test/fixtures/0038-http-wasm-perroute-and-multi-plugin/
  README.md             # this file
  envoy.yaml            # reference Envoy bootstrap (3 listeners; v8 runtime)
  envoy-go.yaml         # subject envoy-go bootstrap (same topology; wazero; host paths)
  expectations.yaml     # human-readable per-arm expectations (doc aid)
  inputs/
    driver.go           # Driver + MultiListenerDriver + BackendKindAware +
                        # ReferenceLogMounter + StatsAsserter
  scripts/              # Rust source crates (NOT built at test time)
    README.md           # operator reproducibility (cargo build invocation)
    .gitignore          # target/ + Cargo.lock
    perroute_override/  listener_default/  shared_data_combined/  fail_reload_trap/
      Cargo.toml + src/lib.rs
  bytecode/             # vendored .wasm blobs (committed to git)
    perroute_override.wasm  listener_default.wasm
    shared_data_combined.wasm  fail_reload_trap.wasm
```

## Cross-references

- **25.3 Task 11** — fixture-0038 file roster.
- **AMEND-C1** — per-route wholesale Wasm message replacement (NO
  WasmPerRoute type) + vm_id-scoped RootVM + shared-data sharing.
- **AMEND-C3 / R-25.3-3** — failure_policy=FAIL_RELOAD reload-on-RuntimeError
  + the vm_reload triplet counters.
- **AMEND-A1** — `proxy-wasm-rust-sdk =0.2.4` pin.
- **AMEND-A5** — StrictDefaultDeny capability sandbox (explicit allow-list).
- **phase-22.2 REVIEW §7.4** — freeTCPPort flake mitigation (single backend).
- **`reference_differential_asserter_dispatch`** memory — StatsAsserter
  discipline for cross-side fixtures + mandatory deliberate-break liveness.
- **`reference_differential_fixture_dispatch_constraint`** — one fixture
  dir = one runner branch (new `BackendKind=HTTPWasmPerRoute=27`).
- **fixture-0036** — sibling precedent (25.2 HTTPWasmAdvanced).
