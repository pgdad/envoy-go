// Package network is the L4 read-filter chain framework: the per-connection
// sequential read-filter dispatch pipeline (the L4 analog of
// internal/filter/http/). It supplies the ReadFilter interface + two-step
// factory types (types.go), the per-connection ReadFilterCallbacks + Connection
// accessor surface (callbacks.go), the drainable connection read Buffer
// (buffer.go), the freeze-after-boot type_url → factory Registry (registry.go),
// and the per-connection chain runner + runtime context (chain.go).
//
// The framework deliberately mirrors internal/listener/listenerfilter/ (the
// closest structural analog — a per-connection sequential-dispatch pipeline
// with a freeze-after-boot registry; ADR-0213) and internal/filter/http/ (the
// filter-framework precedent — two-step factory + callbacks injection). The
// Registry name + boot-time discipline are reserved per ADR-0214.
package network
