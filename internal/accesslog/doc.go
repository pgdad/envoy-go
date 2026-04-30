// Package accesslog provides envoy-go's access-log subsystem.
//
// The package is documented inline in accesslog.go's package comment. See
// ADR-0066 for the architectural decision to ship a thin in-tree shape with
// no third-party access-log dependency.
//
// Sinks are opened by cmd/envoy-go/main.go between bootstrap.Load and
// listener.Run, threaded through the HCM filter chain, and closed via
// defer sink.Close() after listener.Shutdown returns. SIGTERM-while-pending
// drain semantics are Phase 08's deliverable.
package accesslog
