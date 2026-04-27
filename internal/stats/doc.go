// Package stats is envoy-go's in-tree atomic-counter Registry plus the
// Prometheus text-format writer that walks the registry on /stats/prometheus
// requests. Per ADR-0059 the package owns the canonical observation surface;
// no third-party Prometheus library is consumed at runtime. Per ADR-0060
// histograms are deferred to a later sub-phase. Per ADR-0061 the
// hierarchical-dotted-name flattening rules SN1–SN8 govern the
// internal-name → Prometheus-name mapping (see name.go and BEHAVIOR_CONTRACT.md
// §Stat-name mapping). Per ADR-0064 stats_config.stats_tags is hardcoded;
// the bootstrap proto's stats_config.stats_tags[] field is silently ignored.
//
// The LBP-1 invariant ("list before play") — registry.Freeze() is called
// from cmd/envoy-go/main.go after admin server starts accepting and before
// listener manager begins accepting connections; post-Freeze NewCounter /
// NewGauge calls panic with "stats: registry frozen: cannot register %q
// post-boot". This is what makes the Walk-under-RLock-plus-atomic-Load
// read path lock-free against hot-path increments.
//
// API surface:
//   - NewRegistry() *Registry                 -- boot-time registry alloc
//   - (*Registry).NewCounter(name) *Counter   -- boot-time counter registration
//   - (*Registry).NewGauge(name) *Gauge       -- boot-time gauge registration
//   - (*Registry).Walk(fn func(Metric))       -- scrape-time iteration
//   - (*Registry).Freeze()                    -- LBP-1 boot-end seal
//   - WriteProm(w io.Writer, r *Registry)     -- Prometheus exposition writer
package stats
