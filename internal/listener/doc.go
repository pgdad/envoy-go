// Package listener materializes one listener per static_resources.listeners[]
// entry of an Envoy v3 Bootstrap proto, wires each listener's single filter
// chain to its terminal filter via an inline filter constructor registry,
// binds each listener's TCP socket on Start, and runs one Accept goroutine
// per listener dispatching accepted connections into the filter's Handle.
//
// Phase 02 supports a deliberately narrow filter_chain shape (exactly one
// chain, exactly one terminal filter, empty filter_chain_match, no transport
// socket); ADR-0025 codifies the subset and points at phase 07 for the full
// filter chain framework. The inline filter registry is also a phase-02
// simplification — phase 07 generalises it into an exported package.
//
// See SPEC §5.2, §5.3, ADR-0025.
package listener
