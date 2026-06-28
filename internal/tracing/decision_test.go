package tracing

import (
	"encoding/hex"
	"net/http"
	"testing"
)

// span/trace/uuid byte fixtures consumed by fakeRand.Read in Decide's call order:
// SpanID (8) [, TraceID (16) on the fresh path] [, UUID (16) when no inbound x-request-id].
var (
	spanFixture  = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	traceFixture = []byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f}
	uuidFixture  = []byte{0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f}
)

const (
	contTraceHex  = "0af7651916cd43dd8448eb211c80319c"
	contParentHex = "b7ad6b7169203331"
)

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestDecide(t *testing.T) {
	type tc struct {
		name    string
		headers map[string]string
		cfg     TracingConfig
		floats  []float64
		bytes   []byte

		wantSample    bool
		wantReason    TraceReason
		wantClass     SampleClass
		wantContinued bool

		wantSpanHex     string // if non-empty, assert SpanID
		wantTraceHex    string // if non-empty, assert TraceID
		wantParentHex   string // if non-empty, assert ParentSpanID
		wantReqIDNibble byte   // if non-zero, assert RequestID[14]
		wantReqIDExact  string // if non-empty, assert full RequestID
	}

	cases := []tc{
		{
			name:            "fresh-random-sampled",
			cfg:             TracingConfig{RandomSampling: 100, OverallSampling: 100},
			floats:          []float64{0.0, 0.0}, // random roll fires, overall roll passes
			bytes:           cat(spanFixture, traceFixture, uuidFixture),
			wantSample:      true,
			wantReason:      Sampled,
			wantClass:       RandomSampling,
			wantContinued:   false,
			wantSpanHex:     "0102030405060708",
			wantTraceHex:    "101112131415161718191a1b1c1d1e1f",
			wantReqIDNibble: '9',
		},
		{
			name:            "fresh-random-not-sampled",
			cfg:             TracingConfig{RandomSampling: 0, OverallSampling: 100},
			floats:          []float64{0.0}, // random roll: 0*100 < 0 is false
			bytes:           cat(spanFixture, traceFixture, uuidFixture),
			wantSample:      false,
			wantReason:      NoTrace,
			wantClass:       RandomSampling,
			wantContinued:   false,
			wantSpanHex:     "0102030405060708",
			wantTraceHex:    "101112131415161718191a1b1c1d1e1f",
			wantReqIDNibble: '4',
		},
		{
			name:            "continued",
			headers:         map[string]string{"Traceparent": "00-" + contTraceHex + "-" + contParentHex + "-01"},
			cfg:             TracingConfig{RandomSampling: 100, OverallSampling: 100},
			bytes:           cat(spanFixture, uuidFixture), // no trace read on the continued path
			wantSample:      true,
			wantReason:      Sampled, // continued+sampled => the nibble reflects the inbound sampled bit
			wantClass:       NotTraceable,
			wantContinued:   true,
			wantSpanHex:     "0102030405060708",
			wantTraceHex:    contTraceHex,
			wantParentHex:   contParentHex,
			wantReqIDNibble: '9',
		},
		{
			name:          "continued-bypasses-local-caps",
			headers:       map[string]string{"Traceparent": "00-" + contTraceHex + "-" + contParentHex + "-01"},
			cfg:           TracingConfig{RandomSampling: 0, OverallSampling: 0}, // caps would suppress a local sample
			bytes:         cat(spanFixture, uuidFixture),
			wantSample:    true,    // continued bypasses local caps
			wantReason:    Sampled, // continued+sampled => the nibble reflects the inbound sampled bit
			wantClass:     NotTraceable,
			wantContinued: true,
			wantTraceHex:  contTraceHex,
			wantParentHex: contParentHex,
		},
		{
			name:          "continued-not-sampled",
			headers:       map[string]string{"Traceparent": "00-" + contTraceHex + "-" + contParentHex + "-00"},
			cfg:           TracingConfig{RandomSampling: 100, OverallSampling: 100},
			bytes:         cat(spanFixture, uuidFixture),
			wantSample:    false,
			wantReason:    NoTrace,
			wantClass:     NotTraceable,
			wantContinued: true,
		},
		{
			name:            "client-force",
			headers:         map[string]string{"X-Client-Trace-Id": "abc"},
			cfg:             TracingConfig{ClientSampling: 100, RandomSampling: 0, OverallSampling: 100},
			floats:          []float64{0.0, 0.0}, // client roll fires, overall roll passes
			bytes:           cat(spanFixture, traceFixture, uuidFixture),
			wantSample:      true,
			wantReason:      Client,
			wantClass:       ClientEnabled,
			wantContinued:   false,
			wantReqIDNibble: 'b',
		},
		{
			name:            "client-force-suppressed-falls-through-random-sampled",
			headers:         map[string]string{"X-Client-Trace-Id": "abc"},
			cfg:             TracingConfig{ClientSampling: 0, RandomSampling: 100, OverallSampling: 100},
			floats:          []float64{0.5, 0.0, 0.0}, // client roll (>= 0 => no force), random roll fires, overall passes
			bytes:           cat(spanFixture, traceFixture, uuidFixture),
			wantSample:      true,
			wantReason:      Sampled,
			wantClass:       RandomSampling,
			wantContinued:   false,
			wantReqIDNibble: '9',
		},
		{
			name:            "client-force-suppressed-falls-through-random-not-sampled",
			headers:         map[string]string{"X-Client-Trace-Id": "abc"},
			cfg:             TracingConfig{ClientSampling: 0, RandomSampling: 0, OverallSampling: 100},
			floats:          []float64{0.5, 0.5}, // client roll (no force), random roll (no sample); no overall roll
			bytes:           cat(spanFixture, traceFixture, uuidFixture),
			wantSample:      false,
			wantReason:      NoTrace,
			wantClass:       RandomSampling,
			wantContinued:   false,
			wantReqIDNibble: '4',
		},
		{
			name:          "overall-cap-suppresses-random",
			cfg:           TracingConfig{RandomSampling: 100, OverallSampling: 0},
			floats:        []float64{0.0, 0.0}, // random roll fires, overall roll (>=0 always) suppresses
			bytes:         cat(spanFixture, traceFixture, uuidFixture),
			wantSample:    false,
			wantReason:    NoTrace,
			wantClass:     RandomSampling,
			wantContinued: false,
		},
		{
			name:          "overall-cap-suppresses-client-force",
			headers:       map[string]string{"X-Client-Trace-Id": "abc"},
			cfg:           TracingConfig{ClientSampling: 100, RandomSampling: 0, OverallSampling: 0},
			floats:        []float64{0.0, 0.0}, // client roll fires, overall roll suppresses
			bytes:         cat(spanFixture, traceFixture, uuidFixture),
			wantSample:    false,
			wantReason:    NoTrace,
			wantClass:     ClientEnabled,
			wantContinued: false,
		},
		{
			name:           "preserve-existing-request-id",
			headers:        map[string]string{"X-Request-Id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
			cfg:            TracingConfig{RandomSampling: 100, OverallSampling: 100},
			floats:         []float64{0.0, 0.0},            // random roll fires, overall passes => Sampled
			bytes:          cat(spanFixture, traceFixture), // no uuid: inbound id preserved
			wantSample:     true,
			wantReason:     Sampled,
			wantClass:      RandomSampling,
			wantContinued:  false,
			wantReqIDExact: "aaaaaaaa-bbbb-9ccc-dddd-eeeeeeeeeeee",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range c.headers {
				h.Set(k, v)
			}
			rng := &fakeRand{
				floats: append([]float64(nil), c.floats...),
				bytes:  append([]byte(nil), c.bytes...),
			}
			cfg := c.cfg
			d := Decide(h, &cfg, rng)

			if d.Sample != c.wantSample {
				t.Errorf("Sample = %v, want %v", d.Sample, c.wantSample)
			}
			if d.Reason != c.wantReason {
				t.Errorf("Reason = %v, want %v", d.Reason, c.wantReason)
			}
			if d.Class != c.wantClass {
				t.Errorf("Class = %v, want %v", d.Class, c.wantClass)
			}
			if d.Continued != c.wantContinued {
				t.Errorf("Continued = %v, want %v", d.Continued, c.wantContinued)
			}
			if c.wantSpanHex != "" {
				if got := hex.EncodeToString(d.SpanID[:]); got != c.wantSpanHex {
					t.Errorf("SpanID = %s, want %s", got, c.wantSpanHex)
				}
			}
			if c.wantTraceHex != "" {
				if got := hex.EncodeToString(d.TraceID[:]); got != c.wantTraceHex {
					t.Errorf("TraceID = %s, want %s", got, c.wantTraceHex)
				}
			}
			if c.wantParentHex != "" {
				if got := hex.EncodeToString(d.ParentSpanID[:]); got != c.wantParentHex {
					t.Errorf("ParentSpanID = %s, want %s", got, c.wantParentHex)
				}
			}
			if c.wantReqIDNibble != 0 {
				if len(d.RequestID) < 15 {
					t.Fatalf("RequestID = %q, too short to hold the nibble", d.RequestID)
				}
				if d.RequestID[14] != c.wantReqIDNibble {
					t.Errorf("RequestID[14] = %q, want %q (RequestID=%q)", d.RequestID[14], c.wantReqIDNibble, d.RequestID)
				}
			}
			if c.wantReqIDExact != "" && d.RequestID != c.wantReqIDExact {
				t.Errorf("RequestID = %q, want %q", d.RequestID, c.wantReqIDExact)
			}
		})
	}
}
