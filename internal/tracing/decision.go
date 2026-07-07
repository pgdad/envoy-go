package tracing

import "net/http"

// SampleClass identifies which HCM tracing.* counter a sampling decision
// increments (D-TRACE-SAMPLING). It mirrors the reference's Tracing::Reason
// classification: at most one of these counters is bumped per request, and
// NoClass means none is.
type SampleClass int

const (
	// ClientEnabled marks a request force-traced by an x-client-trace-id (the
	// client_enabled counter).
	ClientEnabled SampleClass = iota
	// HealthCheck marks a health-check request (the health_check counter);
	// unused at 46.1a but reserved for parity with the reference vocabulary.
	HealthCheck
	// NotTraceable marks a CONTINUED trace whose decision came in on the wire
	// (the not_traceable counter); the local sampling caps do not apply. The
	// COUNTER class is orthogonal to the x-request-id nibble: a continued trace's
	// nibble reflects its inbound sampled bit (Sampled => '9', not-sampled => '4'),
	// while this class stays not_traceable regardless.
	NotTraceable
	// RandomSampling marks a locally random/overall-sampled request (the
	// random_sampling counter).
	RandomSampling
	// ServiceForced marks a service-forced trace (the service_forced counter);
	// unused at 46.1a but reserved for parity with the reference vocabulary.
	ServiceForced
	// NoClass marks "increment no counter" (the zero-decision / suppressed
	// paths that bump none of the HCM tracing.* classification counters).
	NoClass
)

// Decision is the per-request tracing decision produced by Decide: whether to
// sample, the x-request-id REASON nibble, the HCM-counter Class, the trace
// identity (continued-or-fresh) and the generated/stamped x-request-id.
type Decision struct {
	Sample       bool
	Reason       TraceReason // NoTrace / Sampled / Client (the x-request-id nibble)
	Class        SampleClass // the HCM-counter class (NoClass => increment none)
	Continued    bool
	TraceID      [16]byte
	SpanID       [8]byte // fresh; the upstream traceparent span-id (+ the 46.1b span_id)
	ParentSpanID [8]byte // incoming parent (continued) or zero
	TraceState   string  // pass-through
	RequestID    string  // the generated/stamped x-request-id
}

// Decide runs the §11 request-tracing precedence using the W3C traceparent as the
// continued-trace source: it extracts an inbound traceparent (if any) and delegates
// to DecideWithContext. It is the thin traceparent-flavored wrapper over the
// extraction-agnostic engine (D-TRACE-ZIPKIN-DECIDE-SEAM); a B3 (or other) caller
// extracts its own context and calls DecideWithContext directly.
func Decide(h http.Header, cfg *TracingConfig, rng RandSource) Decision {
	ic, ok := ExtractTraceparent(h)
	return DecideWithContext(h, ic, ok, cfg, rng)
}

// DecideWithContext runs the §11 request-tracing precedence over the incoming
// headers, an already-extracted inbound trace context (ic, valid when continued),
// the parsed TracingConfig and the randomness seam. A fresh span-id is always read
// first (byte-stability: extraction consumes no rng). When continued, the supplied
// ic CONTINUES that trace authoritatively (its sampled bit is honored, bypassing the
// local random_sampling/overall_sampling caps; the x-request-id reason nibble
// reflects that inbound sampled bit — Sampled when sampled, NoTrace when not — while
// the COUNTER class stays not_traceable); otherwise the decision is local: an
// x-client-trace-id force-traces (subject to client_sampling, else falling through to
// random sampling), the default path random-samples, and overall_sampling then caps
// any locally-decided sample. Finally the x-request-id is preserved+stamped (if
// inbound) or freshly generated, carrying the decision REASON in its version nibble.
func DecideWithContext(h http.Header, ic TraceContext, continued bool, cfg *TracingConfig, rng RandSource) Decision {
	var d Decision
	_, _ = rng.Read(d.SpanID[:]) // a fresh span-id always (upstream traceparent + 46.1b span)
	if continued {
		d.Continued = true
		d.TraceID = ic.TraceID
		d.ParentSpanID = ic.ParentID
		d.Sample = ic.Sampled
		if ic.Sampled {
			d.Reason = Sampled
		} // else d.Reason stays NoTrace (the zero value) for a continued-not-sampled trace
		d.Class = NotTraceable
		d.TraceState = ic.TraceState
	} else {
		_, _ = rng.Read(d.TraceID[:]) // fresh trace
		switch {
		case h.Get("X-Client-Trace-Id") != "" && rng.Float64()*100 < cfg.ClientSampling:
			d.Sample = true
			d.Reason = Client
			d.Class = ClientEnabled
		default:
			// A client_sampling-suppressed force (x-client-trace-id present but the
			// Float64 draw in the case above failed) falls through HERE to random
			// sampling (§11) — the same decision, reason, and class as the no-header
			// path, with the identical rng stream (the client-sampling draw was
			// already consumed inside the case condition iff the header was present).
			d.Sample = rng.Float64()*100 < cfg.RandomSampling
			if d.Sample {
				d.Reason = Sampled
			}
			d.Class = RandomSampling
		}
		// overall_sampling caps the LOCALLY-decided sample (NOT a continued trace)
		if d.Sample && rng.Float64()*100 >= cfg.OverallSampling {
			d.Sample = false
			d.Reason = NoTrace
		}
	}
	if existing := h.Get("X-Request-Id"); existing != "" {
		d.RequestID = StampRequestID(existing, d.Reason)
	} else {
		d.RequestID = GenerateRequestID(d.Reason, rng)
	}
	return d
}
