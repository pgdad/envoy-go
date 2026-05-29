# Fixture 0038 — wasm guest source code

Rust source code (proxy-wasm-rust-sdk =0.2.4) for the phase-25.3 fixture
`0038-http-wasm-perroute-and-multi-plugin`. Each crate directory holds one
`Cargo.toml` + `src/lib.rs` (~30-90 LoC guest filter). Per phase 25.3
Task 11 + AMEND-A1 + D-P-PLAN-12.

The compiled `.wasm` blobs are vendored at `../bytecode/` and committed to
git so the test suite does NOT require a Rust toolchain to run. The
differential runner loads the vendored blobs via the `DataSource.Filename`
arm on BOTH reference Envoy v1.37.2 (V8 runtime) AND envoy-go (wazero
runtime); the per-crate `bytecode/<crate>.wasm` paths are spliced into the
bootstrap templates at runner time.

## Crates

| crate | role | response/request effect |
|---|---|---|
| `perroute_override` | per-route wholesale Wasm TPFC override | response `x-wasm-variant: override` |
| `listener_default` | listener-default plugin | response `x-wasm-variant: listener` |
| `shared_data_combined` | BOTH filters of the vm_id-shared chain (role via PluginConfig.configuration `writer`/`reader`) | writer sets shared key `x0038-shared=written-by-A` on request; reader reflects `x-shared: <value>` on response |
| `fail_reload_trap` | FAIL_RELOAD plugin | panics (traps) when request `x-trigger-trap: 1`; else response `x-reload: ok` |

See `../README.md` for the scenario partition table (cross-side via
`CompareBytes` vs subject-only via `StatsAsserter`).

## Operator reproducibility

### Prerequisites

- `rustup` + `rustc 1.94.0` (or newer compatible)
- `cargo 1.94.0` (or newer compatible)
- `wasm32-wasip1` target installed:
  ```bash
  rustup target add wasm32-wasip1
  ```

### Build all 4

```bash
cd test/fixtures/0038-http-wasm-perroute-and-multi-plugin/scripts
for d in perroute_override listener_default shared_data_combined fail_reload_trap; do
  (cd "$d" && cargo build --release --target wasm32-wasip1)
  cp "$d/target/wasm32-wasip1/release/$d.wasm" "../bytecode/$d.wasm"
done
```

The per-crate `target/` + `Cargo.lock` are gitignored (see `.gitignore`);
only the vendored `.wasm` blobs + the `Cargo.toml` + `src/lib.rs` source
files are committed.

## Reference Envoy compatibility

The vendored `.wasm` blobs export `proxy_abi_version_0_2_1` (per
proxy-wasm-rust-sdk 0.2.4 baseline) which BOTH the reference Envoy V8
runtime AND envoy-go's wazero runtime accept.

## Cross-references

- **25.3 Task 11** — fixture-0038 file roster.
- **AMEND-A1** — `proxy-wasm-rust-sdk =0.2.4` pin.
- **AMEND-C1** — per-route wholesale Wasm message replacement (no
  WasmPerRoute type) + vm_id-scoped RootVM + shared-data sharing.
- **D-P-PLAN-12** — vendored .wasm reproduction discipline.
- **fixture-0036 scripts/** — 25.2 precedent for the per-crate Cargo crate
  + vendor pattern.
