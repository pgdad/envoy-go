// Package http provides envoy-go's HTTP filter chain framework — the iteration
// protocol, extension registry, per-stream FilterChain state machine, and
// per-route config 3-tier merge.
//
// Architecture (per ADR-0071, ADR-0072, ADR-0073):
//
//   - Filter interfaces (StreamDecoderFilter / StreamEncoderFilter) live in
//     types.go; status enums (FilterHeadersStatus / FilterDataStatus /
//     FilterTrailersStatus) and the two-step HTTPFilterFactory /
//     FilterInstanceFactory pattern live alongside.
//   - Callback contracts (DecoderFilterCallbacks / EncoderFilterCallbacks)
//     live in callbacks.go.
//   - The extension registry HTTPRegistry (Register / Lookup / Freeze) lives
//     in registry.go; freeze-after-boot invariant mirrors *stats.Registry
//     LBP-1 from ADR-0059.
//   - Per-stream state in chain.go: FilterChain owns filter instances,
//     decode/encode iteration cursors, body buffers (capped at
//     filterBufferLimitBytes = 1 << 20), async-resume signal channels, and
//     the merged per-route config map (lazy-cached on first lookup).
//   - typed_per_filter_config 3-tier merge in perroute.go: most-specific
//     override (Route > VirtualHost > RouteConfiguration); no field-merge.
//
// Concurrency invariant (per ADR-0071): single-goroutine-per-request
// iteration. The HCM dispatch goroutine is the only goroutine that drives
// chain.runDecode* / chain.runEncode*. Filter callbacks called from
// filter-spawned goroutines are signal-only (channel send to wake the
// dispatch goroutine).
//
// External dependencies: Go stdlib + google.golang.org/protobuf +
// github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3
// (proto types only; blank-imported in internal/bootstrap/bootstrap.go at Task 20) +
// internal/cluster (router sub-package only). NO third-party
// filter-chain-engine / filter-iteration library.
package http
