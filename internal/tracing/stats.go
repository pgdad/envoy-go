package tracing

import (
	"fmt"

	"github.com/pgdad/envoy-go/internal/stats"
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

// TracerCounters holds the 2 process-global tracer-scoped counters registered
// under tracing.opentelemetry.* (D-TRACE-STATS-FINAL). These are static names
// (no dynamic segment) so no IsValidName guard is required.
type TracerCounters struct {
	spansSent    *stats.Counter
	spansDropped *stats.Counter
}

// RegisterTracerCounters allocates the 2 tracer-scoped counters
// tracing.opentelemetry.spans_sent and tracing.opentelemetry.spans_dropped.
func RegisterTracerCounters(reg *stats.Registry) *TracerCounters {
	return &TracerCounters{
		spansSent:    reg.NewCounter("tracing.opentelemetry.spans_sent"),
		spansDropped: reg.NewCounter("tracing.opentelemetry.spans_dropped"),
	}
}

// ZipkinCounters holds the 2 process-global tracer-scoped counters registered
// under tracing.zipkin.* (D-TRACE-ZIPKIN-STATS-FINAL). These are static names
// (no dynamic segment) so no IsValidName guard is required.
type ZipkinCounters struct {
	spansSent    *stats.Counter
	spansDropped *stats.Counter
}

// RegisterZipkinCounters allocates the 2 tracer-scoped counters
// tracing.zipkin.spans_sent and tracing.zipkin.spans_dropped.
func RegisterZipkinCounters(reg *stats.Registry) *ZipkinCounters {
	return &ZipkinCounters{
		spansSent:    reg.NewCounter("tracing.zipkin.spans_sent"),
		spansDropped: reg.NewCounter("tracing.zipkin.spans_dropped"),
	}
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
