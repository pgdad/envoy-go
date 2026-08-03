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

⚠️ **`rustc`/`cargo` 1.94.0 EXACTLY — not "or newer".** An earlier
revision of this file said *"1.94.0 (or newer compatible)"*; that was
measurably false and is corrected here (phase 82 IMPL Task 13).

- `rustup` + **`rustc 1.94.0`** / **`cargo 1.94.0`**, pinned explicitly:
  ```bash
  rustup toolchain install 1.94.0
  rustup target add --toolchain 1.94.0 wasm32-wasip1
  ```
- Always invoke cargo as `cargo +1.94.0 …` (see below). Note that
  `rustup target add` applies to ONE toolchain — installing the target
  for `stable` does NOT install it for `1.94.0`.

#### Why the toolchain is pinned exactly

Measured on rustc **1.96.0** (`ac68faa20`, the `stable` toolchain) with
the recipe below, the link step **fails outright**:

```
error: linking with `rust-lld` failed: exit status: 1
  rust-lld: error: ... undefined symbol: proxy_close_stream
  rust-lld: error: ... undefined symbol: proxy_http_call
  rust-lld: error: ... undefined symbol: proxy_continue_stream
  ... (43 error lines across 32 distinct `proxy_*` host imports)
```

This is a wholesale loss of the proxy-wasm **host imports** — every
`proxy_*` hostcall the SDK declares, not one specific symbol. Adding
`-C link-arg=--allow-undefined` makes the link succeed but produces a
**different blob** (139046 B vs 139655 B at the phase-82 baseline), so
it is a *divergence*, not a fix. Do not use it.

Versions between 1.94.0 and 1.96.0 are **untested** — 1.95.0 was not
exercised because the `wasm32-wasip1` target was not installed for it.
Treat anything other than 1.94.0 as unsupported.

#### Why `Cargo.lock` is committed for `l_httpcall_success`

`Cargo.toml` pins `proxy-wasm = "=0.2.4"`, but proxy-wasm's own
dependency on the **`log`** crate is a floating caret range. `log` is
therefore a *transitive, unpinned* input to the blob:

| rustc | `log` | result vs the committed blob |
|---|---|---|
| 1.94.0 | **0.4.30** | **byte-identical** (`cmp` clean) |
| 1.94.0 | 0.4.33 | **same size**, 49791 bytes differ |
| 1.96.0 | any | does not link (see above) |

⚠️ The 0.4.33 build is the **same byte size** as the correct one — so a
size check alone does NOT catch the drift. Reproduction must be gated on
`cmp`/`sha256sum`, never on size.

`l_httpcall_success/Cargo.lock` is committed to freeze `log` (and
`hashbrown`, `foldhash`, `allocator-api2`, `equivalent`). The other 13
scenario crates remain gitignored — see `.gitignore` for the deliberately
narrow exception.

### Build one scenario

```bash
cd test/fixtures/0036-http-wasm-body-and-advanced/scripts/a_body_read_only
cargo +1.94.0 build --release --target wasm32-wasip1
cp target/wasm32-wasip1/release/a_body_read_only.wasm \
   ../../bytecode/a_body_read_only.wasm
```

For `l_httpcall_success`, the committed `Cargo.lock` is honored
automatically; add `--locked` to make a drifted lock a hard error:

```bash
cd test/fixtures/0036-http-wasm-body-and-advanced/scripts/l_httpcall_success
cargo +1.94.0 build --locked --release --target wasm32-wasip1
cp target/wasm32-wasip1/release/l_httpcall_success.wasm \
   ../../bytecode/l_httpcall_success.wasm
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
  (cd "$d" && cargo +1.94.0 build --release --target wasm32-wasip1)
  cp "$d/target/wasm32-wasip1/release/$d.wasm" "../bytecode/$d.wasm"
done
```

The per-crate `target/` + `Cargo.lock` are gitignored (see
`.gitignore`); only the vendored `.wasm` blobs + the `Cargo.toml` +
`src/lib.rs` source files are committed. **One exception**:
`l_httpcall_success/Cargo.lock` IS committed (rationale above).

## Blob checksums

⚠️ **This table is a DOCUMENTARY pin, not an executable gate.** Nothing
in CI verifies it — CI carries no Rust toolchain, and the repo has no
golden over `bytecode/`. It exists so a reviewer facing a binary diff
can tell a correct rebuild from a drifted one. Verify by hand:

```bash
cd test/fixtures/0036-http-wasm-body-and-advanced
sha256sum bytecode/l_httpcall_success.wasm
```

| blob | sha256 | bytes |
|---|---|---|
| `l_httpcall_success.wasm` | `207db77ebe9451e31056536a6fca1ab4be83e63e5774bcca83b55605f71b2523` | 139681 |

Built with rustc 1.94.0 + the committed `Cargo.lock` (`log` 0.4.30).
Phase 82 IMPL Task 13 added `self.resume_http_request();` to
`on_http_call_response`; that one line moved the blob from
`4e630adf…` / 139655 B to the row above — **+26 bytes**, measured.

⚠️ **An import-list diff does NOT discriminate the two blobs**: both
import exactly 36 host functions and the import sets are *identical*
(measured: `proxy_continue_stream` is present in BOTH, so its presence
proves nothing about whether the guest calls it). The only sound check is a
**runtime probe**: against reference Envoy the pre-fix blob hangs the
request until the client times out (curl exit 28, 0 bytes); the current
blob answers `HTTP/1.1 200` with `x-httpcall-status: 200` in ~3 ms.

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
