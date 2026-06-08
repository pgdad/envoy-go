package kafkabroker

import "testing"

func TestTypeURL(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}
