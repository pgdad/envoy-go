// Package conformance hosts protocol-conformance drivers (h2spec, h3spec,
// grpc-conformance, proxy-wasm conformance). Phase 00 creates the package
// skeleton so the directory tree matches BOOTSTRAP_PROMPT.md §4. The first
// driver lands in the phase that introduces the protocol it tests:
//
//   - h2spec: phase 05 (HTTP/2)
//   - h3spec: HTTP/3 family
//   - grpc:   gRPC family
//   - proxy-wasm: WASM host family
package conformance
