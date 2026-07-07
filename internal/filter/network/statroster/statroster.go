// Package statroster provides the shared scaffold behind the protocol proxies'
// fixed stat rosters (zookeeperproxy, mongoproxy, kafkabroker, redisproxy,
// thriftproxy): eager creation of a closed suffix table under one prefix plus
// the panic-on-unknown-suffix increment mechanics. Stat NAMES stay entirely
// with the callers — each package keeps its own suffix tables and dynamic
// counter extras; this package concatenates prefix+suffix verbatim, so the
// emitted names are byte-identical to the previous per-package scaffolds.
package statroster

import (
	"fmt"

	"github.com/pgdad/envoy-go/internal/stats"
)

// New eagerly creates one counter per suffix under prefix via
// NewCounterIfAbsent — post-Freeze-permitted and idempotent across listeners
// sharing a stat_prefix (the rbac newFilterStats precedent) — and returns the
// suffix-keyed map the owning package stores on its stats struct.
func New(reg *stats.Registry, prefix string, suffixes []string) map[string]*stats.Counter {
	counters := make(map[string]*stats.Counter, len(suffixes))
	for _, suf := range suffixes {
		counters[suf] = reg.NewCounterIfAbsent(prefix + suf)
	}
	return counters
}

// Inc increments the roster counter for suffix. An unknown suffix is a
// programming error → panic (the roster is closed; dynamic names go through
// each package's own lazy helpers). pkg tags the panic message with the
// owning package, preserving the per-package wording.
func Inc(pkg string, counters map[string]*stats.Counter, suffix string) {
	c, ok := counters[suffix]
	if !ok {
		panic(fmt.Sprintf("%s: unknown roster suffix %q", pkg, suffix))
	}
	c.Inc()
}

// Add adds delta to the roster counter for suffix (the *_bytes accounting).
// Unknown suffix is a programming error → panic, exactly as Inc.
func Add(pkg string, counters map[string]*stats.Counter, suffix string, delta uint64) {
	c, ok := counters[suffix]
	if !ok {
		panic(fmt.Sprintf("%s: unknown roster suffix %q", pkg, suffix))
	}
	c.Add(delta)
}
