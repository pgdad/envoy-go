// Package listenerfilter implements envoy-go's listener-side filter dispatch
// pipeline (the listener-filter framework introduced by phase 07.2). The
// package defines:
//
//   - ListenerFilter: the per-connection filter interface (Inspect + OnDestroy).
//   - ListenerFilterStatus: 2-state enum (Continue, StopIteration); no
//     async-resume per ADR-0079.
//   - ChainMatchInputs: 8-field struct holding the chain-match dimensions
//     populated from connection-level facts (DestinationIP/Port,
//     SourceIP/Port) plus listener-filter-contributed fields (ServerName,
//     TransportProtocol, ApplicationProtocols).
//   - Peeker: peek-without-consume interface backed by peekerConn (a
//     bufio.Reader-wrapped net.Conn).
//   - ListenerFilterRegistry: boot-populated, frozen-after-boot registry
//     mapping type_url → factory; mirrors *filter_http.HTTPRegistry from
//     07.1 (ADR-0072) and *stats.Registry from 06.1 (ADR-0059).
//   - Pipeline: per-connection sequential dispatch with shared per-pipeline
//     timeout (ADR-0082).
//   - chainmatch.SelectChain: 8-dimension precedence algorithm (ADR-0081)
//     consulting default_filter_chain (ADR-0080) on no-match.
//
// Architecture: the listener manager's accept-loop allocates a Pipeline per
// accepted connection, wraps the raw conn in a peekerConn, runs the
// listener-filter pipeline (which populates ChainMatchInputs), then calls
// chainmatch.SelectChain to pick the filter chain. The selected chain's TLS
// handshake (if any) runs next, then dispatch falls through to the chain's
// terminal filter unchanged. See SPEC §5.2 for the per-connection lifecycle
// and SPEC §5.5 for the chain-match algorithm.
//
// Concurrency: single-goroutine-per-connection drives the pipeline +
// chain-match + dispatch; no shared mutable state on the hot path.
// ListenerFilterRegistry locks only at boot (Register/Lookup); post-Freeze
// reads are lock-free. See SPEC §5.6.
//
// Introduced by phase 07.2.
package listenerfilter
