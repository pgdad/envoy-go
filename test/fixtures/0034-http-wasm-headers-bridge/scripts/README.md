# Fixture 0034 — wasm guest source code

Rust source code (proxy-wasm-rust-sdk =0.2.4) for the 7 cross-side
scenarios of fixture `0034-http-wasm-headers-bridge`. Each per-
scenario directory holds one `Cargo.toml` + `src/lib.rs` (~30 LoC
guest filter). Per phase 25.1 Task 15 PLAN + AMEND-A1.

The compiled `.wasm` blobs are vendored at `../bytecode/` and
committed to git per Q9 + AMEND-A1. The differential runner loads
the vendored blobs via the `DataSource.Filename` arm on BOTH
reference Envoy v1.34.0 (V8 runtime) AND envoy-go (wazero runtime);
the per-scenario `bytecode/<scenario>.wasm` paths are spliced into
the bootstrap templates at runner time.

## Scenarios

| # | Directory | Plugin behavior |
|---|---|---|
| a | `a_add_header/` | `proxy_add_header_map_value(HTTP_REQUEST_HEADERS, "x-wasm-injected", "hello")` |
| b | `b_replace_header/` | `proxy_replace_header_map_value(HTTP_REQUEST_HEADERS, "user-agent", "envoy-go-wasm/1.0")` |
| c | `c_remove_header/` | `proxy_remove_header_map_value(HTTP_REQUEST_HEADERS, "x-blocked")` |
| d | `d_respond_shortcircuit/` | `proxy_send_local_response(403, "", "denied", &[], 0)` + Action::Pause |
| e | `e_log_only/` | `proxy_log(INFO, "wasm hit")` via `hostcalls::log` |
| f | `f_header_iter/` | `proxy_get_header_map_pairs(HTTP_REQUEST_HEADERS)` → count → add `x-headers-count: N` |
| g | `g_property_method/` | `proxy_get_property("request.method")` → add `x-request-method: GET` |

§4.5 guardrail compliance: all 7 scenarios use only the 24-hostcall
surface; HTTP/1.1 only; no memory traps; no float-formatted logs.

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
cd test/fixtures/0034-http-wasm-headers-bridge/scripts/a_add_header
cargo build --release --target wasm32-wasip1
cp target/wasm32-wasip1/release/a_add_header.wasm \
   ../../bytecode/a_add_header.wasm
```

### Build all 7

```bash
cd test/fixtures/0034-http-wasm-headers-bridge/scripts
for d in a_add_header b_replace_header c_remove_header \
         d_respond_shortcircuit e_log_only f_header_iter \
         g_property_method; do
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
v1.34.0 V8 runtime AND envoy-go's wazero runtime accept per AMEND-A6
envoy-go-strict-stricter discipline (envoy-go REJECTS the older
0.2.0 / 0.1.0 sentinels; 0.2.1 is the only common version both
implementations support).

## Cross-references

- **25.1 SPEC §9.1** — 7-scenario fixture-0034 table.
- **25.1 SPEC §4.5** — 24-hostcall guardrail + cross-side determinism.
- **25.1 PLAN Task 15** — fixture-0034 file roster.
- **AMEND-A1** — `proxy-wasm-rust-sdk =0.2.4` pin.
- **AMEND-A6** — envoy-go-strict-stricter (`proxy_abi_version_0_2_1` only).
- **parent §8.5** — `BackendKind=HTTPWasm` enum + dispatch.
