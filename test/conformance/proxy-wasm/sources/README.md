# proxy-wasm conformance — guest sources

Rust guest source code (proxy-wasm-rust-sdk =0.2.4, AMEND-A1) for the
in-process conformance families ported at phase-25.3 Task 14. Each
per-family directory holds one `Cargo.toml` + `src/lib.rs`. The compiled
`.wasm` blobs are built **offline** (CI does NOT build Rust) and **vendored**
under `../bytecode/`, committed to git. Mirrors the fixture-0036
`scripts/README.md` reproduction discipline.

The harness loads each blob via `loadFamilyWasm(t, name)` (reads
`../bytecode/<name>.wasm`); the Go family sub-tests live in
`../families_test.go`. The blobs export `proxy_abi_version_0_2_1` per AMEND-A6
so `wasm.CompileModule` accepts them.

## Batch-A families (Task 14)

| crate / blob               | family           | guest behavior                                              |
| -------------------------- | ---------------- | ----------------------------------------------------------- |
| `logging`                  | logging          | `hostcalls::log` at all 5 severities on request headers.    |
| `stop_iteration_pause`     | stop_iteration   | returns `Action::Pause` from `proxy_on_request_headers`.    |
| `stop_iteration_continue`  | stop_iteration   | returns `Action::Continue`.                                 |
| `shared_data`              | shared_data      | CAS round-trip in `on_vm_start` (set/get/CAS-match/CAS-miss). |
| `pairs_util`               | pairs_util       | reads request-header pairs (`get_map`), echoes count + a value. |
| `endianness`               | endianness       | writes known u32/u64 LE bytes to shared-data in `on_vm_start`. |

## Batch-B families (Task 14)

| crate / blob               | family           | guest behavior                                              |
| -------------------------- | ---------------- | ----------------------------------------------------------- |
| `exports`                  | exports          | `on_vm_start`: env (`environ_get`), clock (`clock_time_get`), `random_get` → shared-data. |
| `security`                 | security         | `on_request_headers`: `proxy_log` then `get_current_time` (traps on deny sentinel). |
| `runtime`                  | runtime          | `on_request_headers`: `unreachable!()` (panic=abort → wasm trap). |
| `wasm_vm`                  | wasm_vm          | `on_request_headers`: per-stream `calls` counter → `x-stream-count`. |
| _(none)_                   | bytecode_util    | NO guest — asserts at the `CompileModule` boundary with hand-crafted modules. |

## Prerequisites (offline build)

- `rustup` + a recent `rustc`/`cargo` with the `wasm32-wasip1` target:
  ```bash
  rustup target add wasm32-wasip1
  ```

## Build + vendor one family

```bash
cd test/conformance/proxy-wasm/sources/logging
cargo build --release --target wasm32-wasip1 --offline
cp target/wasm32-wasip1/release/logging.wasm ../../bytecode/logging.wasm
```

## Build + vendor all Batch-A blobs

```bash
cd test/conformance/proxy-wasm/sources
for d in logging stop_iteration_pause stop_iteration_continue \
         shared_data pairs_util endianness \
         exports security runtime wasm_vm; do
  (cd "$d" && cargo build --release --target wasm32-wasip1 --offline)
  cp "$d/target/wasm32-wasip1/release/$d.wasm" "../bytecode/$d.wasm"
done
```

Per-crate `target/` + `Cargo.lock` are gitignored (see `.gitignore`); only the
vendored `.wasm` blobs + the `Cargo.toml` / `src/lib.rs` sources are committed.
