# proxy-wasm conformance harness

In-process proxy-wasm conformance suite for envoy-go (phase-25.3 Task 13
scaffold; families land at Task 14). Per ADR-0212 + D-25.3-P4.

The suite ports a subset of the upstream
[proxy-wasm-cpp-host](https://github.com/proxy-wasm/proxy-wasm-cpp-host)
conformance/regression test roster onto envoy-go's wazero-backed
`internal/wasm.RootVM`. Unlike the upstream C++ host tests (which run the guest
against the cpp-host's V8/wasmtime/WAMR engines), this harness runs the SAME
vendored guest `.wasm` blobs **in-process** against envoy-go's wazero runtime —
no Docker, no Rust-in-CI, no engine matrix. It is a pure `go test`.

## The 16-file cpp-host roster: 10 ported, 6 deferred

The upstream cpp-host `test/` directory carries 16 host-side test files. This
phase PORTS 10 of them (62.5% of the roster) and DEFERS 6 as forward-pointers.

### 10 PORTED families (land at Task 14)

| Family           | What it exercises                                                            |
| ---------------- | ---------------------------------------------------------------------------- |
| `logging`        | `proxy_log` at every severity; host-side log capture (`WithRootLogSink`).    |
| `stop_iteration` | `proxy_on_*` returning `Pause` / `Continue`; ProxyAction wire values + trap. |
| `shared_data`    | `proxy_{get,set}_shared_data` CAS semantics; host-observable via `GetSharedData`. |
| `endianness`     | Little-endian wire encoding of multi-byte ABI args (LEB128 + i32/i64 fields).|
| `exports`        | Required export discovery (`proxy_abi_version_0_2_1`, `_initialize`, `malloc`).|
| `security`       | Out-of-bounds memory access / bad-pointer hostcall args → `InvalidMemoryAccess`.|
| `runtime`        | VM lifecycle: `proxy_on_vm_start` / `proxy_on_configure` / context create.   |
| `wasm_vm`        | Module instantiation + per-stream context isolation (no cross-stream leak).  |
| `bytecode_util`  | ABI-version sentinel detection + section parsing (mirrors `internal/wasm/bytecode_util.go`). |
| `pairs_util`     | Header/metadata pair (de)serialization wire format (mirrors `internal/wasm/pairs.go`). |

### 6 DEFERRED families (forward-pointers; documented in BOOTSTRAP_PROMPT.md §7.5)

| Family           | Why deferred                                                                 |
| ---------------- | ---------------------------------------------------------------------------- |
| `shared_queue`   | Cross-VM message queue ABI — not yet implemented in envoy-go's RootVM.       |
| `signature_util` | Module signature verification — out of scope until a code-signing phase.     |
| `wasm` (TLS-cache)| Thread-local-storage compiled-module cache — wazero's model differs; revisit.|
| `vm_id_handle`   | vm_id handle lifecycle — partially covered by `internal/wasm/registry.go`; the conformance angle defers. |
| `null_vm`        | The cpp-host "null VM" (compiled-in C++ guest) — N/A for a pure-wazero host. |
| `fuzz`           | Fuzz harnesses — envoy-go fuzzes via `internal/wasm/*_fuzz` + `FuzzWasmConfigParse`. |

The 6 deferred families are NOT silently dropped: each is a documented
forward-pointer in `BOOTSTRAP_PROMPT.md §7.5` (that roster lands at Task 15).
The 10/6 split reflects what envoy-go's wazero host can meaningfully assert
today; `null_vm` is permanently N/A (no compiled-in guest), the other 5 unlock
as their underlying RootVM features land.

## Reproduction discipline (vendored `.wasm`, no Docker, no Rust-in-CI)

Mirrors the fixture-0036 `scripts/README.md` reproduction discipline.

- Guest sources live under `sources/<family>/` (one `Cargo.toml` + `src/lib.rs`
  per family, or a C/C++ equivalent for the families ported from cpp-host C++
  fixtures).
- The compiled `.wasm` blobs are built **offline** (the operator runs the build
  locally; CI does NOT build Rust/C++) and **vendored** under `bytecode/`,
  committed to git. The harness loads them via `loadFamilyWasm(t, name)` which
  reads `bytecode/<name>.wasm`.
- The blobs export `proxy_abi_version_0_2_1` (per the AMEND-A6 envoy-go-strict-
  stricter ABI gate) so `wasm.CompileModule` accepts them.

### Prerequisites (offline build)

- `rustup` + a recent `rustc`/`cargo`, with the `wasm32-wasip1` target:
  ```bash
  rustup target add wasm32-wasip1
  ```
  (C/C++-sourced families build with the cpp-host's documented `clang
  --target=wasm32-wasi` toolchain; see the per-family `sources/<family>/README`
  added at Task 14.)

### Build + vendor one family (Task 14 pattern)

```bash
cd test/conformance/proxy-wasm/sources/logging
cargo build --release --target wasm32-wasip1
cp target/wasm32-wasip1/release/logging.wasm ../../bytecode/logging.wasm
```

Per-crate `target/` + `Cargo.lock` are gitignored; only the vendored `.wasm`
blobs + the `Cargo.toml` / `src/lib.rs` sources are committed.

## Deliberate-break liveness (per family)

Every ported family MUST be proven LIVE before it counts: deliberately break
its assertion (flip an expected value, or corrupt the vendored blob) and
confirm the family's `t.Run` subtest FAILS, then revert. A family whose
assertion passes vacuously (e.g. asserting on a log line the guest never emits)
is a DEAD test and is not acceptable. This mirrors the differential-fixture
"prove the assertion is live" discipline carried throughout the phase-2x WASM
work.

At Task 13 the family registry (`conformanceFamilies` in `conformance.go`) is
EMPTY, so `TestProxyWasmConformance` passes vacuously — this proves the harness
builds + the driver shape is correct before any family lands.

## Files

- `conformance.go` — shared harness helpers (`loadFamilyWasm`,
  `newConformanceRootVM`, `conformanceSandbox`, `recordingABICallbacks`,
  `assertLogContains`, the `conformanceFamilies` registry).
- `conformance_test.go` — `TestProxyWasmConformance` driver (ranges over the
  registry; vacuous pass at Task 13).
- `bytecode/` — vendored prebuilt `.wasm` blobs (added at Task 14).
- `sources/<family>/` — guest source for each blob (added at Task 14).

## Cross-references

- **ADR-0212** — in-process proxy-wasm conformance harness decision.
- **D-25.3-P4** — in-process `go test` + vendored-`.wasm` reproduction.
- **test/conformance/h2spec/** — structural precedent (Docker-based; different
  mechanism, same package-layout + doc style).
- **test/fixtures/0036-http-wasm-body-and-advanced/scripts/README.md** —
  vendored-`.wasm` reproduction-discipline precedent.
- **BOOTSTRAP_PROMPT.md §7.5** — the 6 deferred families' forward-pointer roster
  (lands at Task 15).
