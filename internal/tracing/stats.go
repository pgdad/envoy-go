package tracing

import (
	"fmt"

	"github.com/esalaine/envoy-go/internal/stats"
)

// HCMCounters holds the 5 HCM-scoped tracing decision counters registered under
// http.<statPrefix>.tracing.* (D-TRACE-STATS). Record dispatches a per-request
// SampleClass onto the matching counter; at most one is bumped per request.
type HCMCounters struct {
	clientEnabled  *stats.Counter
	healthCheck    *stats.Counter
	notTraceable   *stats.Counter
	randomSampling *stats.Counter
	serviceForced  *stats.Counter
}

// RegisterHCMCounters allocates the 5 HCM-scoped tracing decision counters under
// http.<statPrefix>.tracing.* (D-TRACE-STATS). The prefix is re-validated via
// stats.IsValidName (defense-in-depth; the hcm config.go guard already validates
// it for the shared http.<prefix>. namespace) so a malformed prefix returns an
// error rather than tripping Registry.NewCounter's panic-on-invalid-name. The
// health_check and service_forced counters register but stay 0 at 46.1a (no
// health-check detection / x-envoy-force-trace honoring yet).
func RegisterHCMCounters(reg *stats.Registry, statPrefix string) (*HCMCounters, error) {
	base := "http." + statPrefix + ".tracing."
	if !stats.IsValidName(base + "random_sampling") {
		return nil, fmt.Errorf("tracing: invalid stat_prefix %q", statPrefix)
	}
	return &HCMCounters{
		clientEnabled:  reg.NewCounter(base + "client_enabled"),
		healthCheck:    reg.NewCounter(base + "health_check"),
		notTraceable:   reg.NewCounter(base + "not_traceable"),
		randomSampling: reg.NewCounter(base + "random_sampling"),
		serviceForced:  reg.NewCounter(base + "service_forced"),
	}, nil
}

// Record increments the HCM tracing.* counter matching class. NoClass (and any
// unrecognized class) increments none.
func (c *HCMCounters) Record(class SampleClass) {
	switch class {
	case ClientEnabled:
		c.clientEnabled.Inc()
	case HealthCheck:
		c.healthCheck.Inc()
	case NotTraceable:
		c.notTraceable.Inc()
	case RandomSampling:
		c.randomSampling.Inc()
	case ServiceForced:
		c.serviceForced.Inc()
	}
}
