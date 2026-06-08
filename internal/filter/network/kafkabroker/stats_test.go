package kafkabroker

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestStatRoster(t *testing.T) {
	reg := stats.NewRegistry()
	ks := newKafkaStats(reg, "kprobe")
	if got := len(ks.counters); got != 176 {
		t.Fatalf("roster size = %d, want 176", got)
	}
	for _, suf := range []string{
		"request.produce_request", "response.metadata_response",
		"request.api_versions_request", "response.api_versions_response",
		"request.unknown", "request.failure", "response.unknown", "response.failure",
	} {
		c, ok := ks.counters[suf]
		if !ok {
			t.Errorf("counter %q absent from eager roster", suf)
			continue
		}
		if c.Load() != 0 {
			t.Errorf("counter %q = %d at creation, want 0", suf, c.Load())
		}
	}
}

func TestStatRoster_Idempotent(t *testing.T) {
	reg := stats.NewRegistry()
	a := newKafkaStats(reg, "kprobe")
	b := newKafkaStats(reg, "kprobe") // a second listener sharing the prefix — no panic, SAME counters
	if a.counters["request.produce_request"] != b.counters["request.produce_request"] {
		t.Fatal("shared stat_prefix must share the same counter instances")
	}
}

// inc-accessor smoke: each accessor increments the right counter.
func TestStatRoster_IncAccessors(t *testing.T) {
	reg := stats.NewRegistry()
	ks := newKafkaStats(reg, "k")
	ks.incRequest("produce")
	ks.incResponse("metadata")
	ks.incRequestUnknown()
	ks.incRequestFailure()
	ks.incResponseUnknown()
	ks.incResponseFailure()
	if ks.counters["request.produce_request"].Load() != 1 {
		t.Error("incRequest")
	}
	if ks.counters["response.metadata_response"].Load() != 1 {
		t.Error("incResponse")
	}
	if ks.counters["request.unknown"].Load() != 1 {
		t.Error("incRequestUnknown")
	}
	if ks.counters["request.failure"].Load() != 1 {
		t.Error("incRequestFailure")
	}
	if ks.counters["response.unknown"].Load() != 1 {
		t.Error("incResponseUnknown")
	}
	if ks.counters["response.failure"].Load() != 1 {
		t.Error("incResponseFailure")
	}
}
