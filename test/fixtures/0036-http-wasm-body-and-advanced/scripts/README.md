# Fixture 0036 — wasm guest source code

Rust source code (proxy-wasm-rust-sdk =0.2.4) for the 14 scenarios of
fixture `0036-http-wasm-body-and-advanced`. Each per-scenario directory
holds one `Cargo.toml` + `src/lib.rs` (~30-60 LoC guest filter). Per
phase 25.2 Task 20 PLAN + AMEND-A1 + D-P-PLAN-12.

The compiled `.wasm` blobs are vendored at `../bytecode/` and committed
to git per Q9 + AMEND-A1. The differential runner loads the vendored
blobs via the `DataSource.Filename` arm on BOTH reference Envoy v1.37.2
(V8 runtime) AND envoy-go (wazero runtime); the per-scenario
`bytecode/<scenario>.wasm` paths are spliced into the bootstrap
templates at runner time.

## Scenarios

See `../README.md` for the 14-scenario partition table (10 cross-side +
4 subject-only).

## Operator reproducibility

### Prerequisites

- `rustup` + `rustc 1.94.0` (or newer compatible)
- `cargo 1.94.0` (or newer compatible)
- `wasm32-wasip1` target installed:
  ```bash
  rustup target add wasm32-wasip1
  ```

### Build one scenario

```bash
cd test/fixtures/0036-http-wasm-body-and-advanced/scripts/a_body_read_only
cargo build --release --target wasm32-wasip1
cp target/wasm32-wasip1/release/a_body_read_only.wasm \
   ../../bytecode/a_body_read_only.wasm
```

### Build all 14

```bash
cd test/fixtures/0036-http-wasm-body-and-advanced/scripts
for d in a_body_read_only b_body_mutate_passthrough c_body_mutate_replace \
         d_trailers_add e_trailers_read f_shared_data_read_after_write \
         g_foreign_function_deny_default h_property_stream_info \
         i_metric_define_only j_env_vars_rejected_passthrough \
         k_tick_fires_counter l_httpcall_success m_httpcall_unknown_cluster \
         n_body_cap_exceeded; do
  (cd "$d" && cargo build --release --target wasm32-wasip1)
  cp "$d/target/wasm32-wasip1/release/$d.wasm" "../bytecode/$d.wasm"
done
```

The per-crate `target/` + `Cargo.lock` are gitignored (see
`.gitignore`); only the vendored `.wasm` blobs + the `Cargo.toml` +
`src/lib.rs` source files are committed.

## Reference Envoy compatibility

The vendored `.wasm` blobs export `proxy_abi_version_0_2_1` (per
proxy-wasm-rust-sdk 0.2.4 baseline) which BOTH the reference Envoy
V8 runtime AND envoy-go's wazero runtime accept per AMEND-A6
envoy-go-strict-stricter discipline.

## Cross-references

- **25.2 SPEC §8.1.1** — 14-scenario fixture-0036 details.
- **25.2 Task 20 PLAN** — fixture-0036 file roster.
- **AMEND-A1** — `proxy-wasm-rust-sdk =0.2.4` pin.
- **D-P-PLAN-12** — vendored .wasm reproduction discipline.
- **fixture-0034 scripts/** — 25.1 precedent for the same per-scenario
  Cargo crate + vendor pattern.
