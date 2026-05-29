# envoy-go Reference Envoy Pin

**Tag:** `envoyproxy/envoy:v1.37.2`
**SHA256:** `envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`
**Upstream release notes:** https://www.envoyproxy.io/docs/envoy/v1.37.2/version_history/v1.37/v1.37.2
**Envoy proto major version:** `v3`
**Pinned in:** ADR-0008
**Last verified:** 2026-04-21

## Refresh procedure

Per doctrine D-3.7, the pin is changed only via a dedicated phase that re-baselines the differential suite. To execute that phase:

1. Pick a new candidate tag per SPEC §5.6 selection criteria (stable, current within 6 months, no API transition in flight).
2. `docker pull envoyproxy/envoy:<new-tag>`; capture the SHA256 with `docker inspect --format='{{index .RepoDigests 0}}'`.
3. Run all differential fixtures against the new image: `go test ./test/differential/...`. Investigate any divergence — fix envoy-go to match, or extend `BEHAVIOR_CONTRACT.md` (with an ADR), or revert.
4. Update this file with the new tag, SHA, release-notes URL, and `Last verified` date.
5. Append a new ADR superseding ADR-0008 (and any contract-extension ADRs from step 3).
6. Land as a single commit on the pin-refresh phase branch.

The pin is never changed ad-hoc — every change is a phase with a green differential surface.

## Deferred proxy-wasm conformance families (forward-pointers)

The `test/conformance/proxy-wasm/` harness was seeded at phase 25.3 against `proxy-wasm-cpp-host@da3ce05d` (16 unit-test families). **10 are PORTED** and pass at phase-done (logging, stop_iteration, shared_data, endianness, exports, security, runtime, wasm_vm, bytecode_util, pairs_util — 62.5%). **6 are DEFERRED** because they presuppose substrate not implemented at the HTTP-filter scope; documented here + in `BOOTSTRAP_PROMPT.md §7.3` + `BEHAVIOR_CONTRACT.md` as forward-pointers for a future §9 WASM-host-family phase:

| Deferred family | Reason deferred |
|---|---|
| `shared_queue` | WasmService cross-VM message queues + `proxy_on_queue_ready`; not implemented at the HTTP-filter scope. |
| `signature_util` | Ed25519 signed / remote code fetch; not implemented (remote code fetch is PARSE-REJECT per the 25.1 `AsyncDataSource.Remote` envoy-go-strict departure). |
| `wasm` (TLS-cache) | Thread-local WasmHandle cache + canary; presupposes the WasmService singleton model. |
| `vm_id_handle` | Cross-VM scoping substrate; deferred with `shared_queue`/WasmService. |
| `null_vm` | Compiled-in NullVM engine; N/A for a Go host with no NullVM engine (envoy-go ships only wazero). |
| `fuzz` | libFuzzer harnesses (not gtest); covered by envoy-go's own `FuzzWasmConfigParse` + `FuzzWasmHostcallEnvelope` Go fuzzers. |

These re-enter scope when the WasmService singleton / cross-VM-queue substrate lands in a future §9 WASM-host phase (cluster-specifier-wasm, access-logger-wasm, network-filter-wasm, WasmService plugin loaders).
