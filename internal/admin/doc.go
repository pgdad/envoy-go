// Package admin serves the envoy-go admin API on HTTP/1.1. Phase 01 implements
// only GET /ready (see docs/envoy-go/BEHAVIOR_CONTRACT.md §Admin API). Phase
// 06.1 extends the surface with GET /stats/prometheus, which serves the
// Prometheus text-format exposition built from the in-tree internal/stats
// Registry threaded in by main.go (per ADR-0059); the same task wires the
// server.live gauge that the /ready handler Set(1)s under sync.Once on the
// first 200/LIVE response (SPEC §12 #3). Phase 08 extends this package with
// the remaining admin endpoints.
package admin
