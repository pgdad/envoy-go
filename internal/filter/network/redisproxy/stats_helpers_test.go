package redisproxy

import "github.com/pgdad/envoy-go/internal/stats"

// command returns the per-command counter for (lower-cased name, slot∈{total,
// success,error}), or nil if the name is not a supported command. Test-only
// introspection helper (production increments go through the incCommand*
// accessors, which index the table directly).
func (rs *redisStats) command(name, slot string) *stats.Counter {
	cs, ok := rs.commands[name]
	if !ok {
		return nil
	}
	switch slot {
	case "total":
		return cs.total
	case "success":
		return cs.success
	case "error":
		return cs.errc
	}
	return nil
}
