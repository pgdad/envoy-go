package kafkabroker

import (
	"testing"

	kafka_brokerv3 "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"
)

func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := parseConfig(&kafka_brokerv3.KafkaBroker{StatPrefix: "kprobe"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.statPrefix != "kprobe" {
		t.Fatalf("statPrefix = %q, want kprobe", cfg.statPrefix)
	}
	if cfg.forceResponseRewrite {
		t.Fatalf("forceResponseRewrite = true, want false (default)")
	}
	if cfg.rewriteSpec != nil {
		t.Fatalf("rewriteSpec = %v, want nil (default)", cfg.rewriteSpec)
	}
	if len(cfg.apiKeysAllowed) != 0 || len(cfg.apiKeysDenied) != 0 {
		t.Fatalf("api key lists non-empty by default: allowed=%v denied=%v", cfg.apiKeysAllowed, cfg.apiKeysDenied)
	}
}

func TestParseConfig_StatPrefixRequired(t *testing.T) {
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{})
	if err == nil || err.Error() != errStatPrefixRequired {
		t.Fatalf("err = %v, want %q", err, errStatPrefixRequired)
	}
}

func TestParseConfig_StatPrefixInvalidChars(t *testing.T) {
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{StatPrefix: "bad name"}) // space → IsValidName false
	if err == nil || err.Error() != errStatPrefixInvalid {
		t.Fatalf("err = %v, want %q", err, errStatPrefixInvalid)
	}
}

func TestParseConfig_ForceResponseRewriteAccepted(t *testing.T) {
	cfg, err := parseConfig(&kafka_brokerv3.KafkaBroker{StatPrefix: "k", ForceResponseRewrite: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.forceResponseRewrite {
		t.Fatalf("forceResponseRewrite = false, want true")
	}
}

func TestParseConfig_ApiKeyOutOfRange(t *testing.T) {
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{StatPrefix: "k", ApiKeysAllowed: []uint32{32768}})
	if err == nil || err.Error() != errApiKeyOutOfRange {
		t.Fatalf("err = %v, want %q", err, errApiKeyOutOfRange)
	}
}

func TestParseConfig_ApiKeysDeniedOutOfRange(t *testing.T) {
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{StatPrefix: "k", ApiKeysDenied: []uint32{40000}})
	if err == nil || err.Error() != errApiKeyOutOfRange {
		t.Fatalf("err = %v, want %q", err, errApiKeyOutOfRange)
	}
}

func TestParseConfig_ApiKeysInRangeAccepted(t *testing.T) {
	cfg, err := parseConfig(&kafka_brokerv3.KafkaBroker{
		StatPrefix:     "k",
		ApiKeysAllowed: []uint32{0, 18, 32767},
		ApiKeysDenied:  []uint32{1, 32767},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.apiKeysAllowed) != 3 || len(cfg.apiKeysDenied) != 2 {
		t.Fatalf("api keys not remembered: allowed=%v denied=%v", cfg.apiKeysAllowed, cfg.apiKeysDenied)
	}
}

func TestParseConfig_RewriteRuleHostRequired(t *testing.T) {
	spec := &kafka_brokerv3.IdBasedBrokerRewriteSpec{
		Rules: []*kafka_brokerv3.IdBasedBrokerRewriteRule{{Id: 1, Port: 9092}}, // Host empty
	}
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{
		StatPrefix: "k",
		BrokerAddressRewriteSpec: &kafka_brokerv3.KafkaBroker_IdBasedBrokerAddressRewriteSpec{
			IdBasedBrokerAddressRewriteSpec: spec,
		},
	})
	if err == nil || err.Error() != errRewriteRuleHostRequired {
		t.Fatalf("err = %v, want %q", err, errRewriteRuleHostRequired)
	}
}

func TestParseConfig_RewriteRulePortTooLarge(t *testing.T) {
	spec := &kafka_brokerv3.IdBasedBrokerRewriteSpec{
		Rules: []*kafka_brokerv3.IdBasedBrokerRewriteRule{{Id: 1, Host: "broker0", Port: 65536}},
	}
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{
		StatPrefix: "k",
		BrokerAddressRewriteSpec: &kafka_brokerv3.KafkaBroker_IdBasedBrokerAddressRewriteSpec{
			IdBasedBrokerAddressRewriteSpec: spec,
		},
	})
	if err == nil || err.Error() != errRewriteRulePortTooLarge {
		t.Fatalf("err = %v, want %q", err, errRewriteRulePortTooLarge)
	}
}

func TestParseConfig_OneofTypedNil(t *testing.T) {
	// A non-nil oneof wrapper holding a nil *IdBasedBrokerRewriteSpec.
	_, err := parseConfig(&kafka_brokerv3.KafkaBroker{
		StatPrefix:               "k",
		BrokerAddressRewriteSpec: &kafka_brokerv3.KafkaBroker_IdBasedBrokerAddressRewriteSpec{},
	})
	if err == nil || err.Error() != errRewriteSpecNil {
		t.Fatalf("err = %v, want %q", err, errRewriteSpecNil)
	}
}

func TestParseConfig_RewriteSpecAccepted(t *testing.T) {
	spec := &kafka_brokerv3.IdBasedBrokerRewriteSpec{
		Rules: []*kafka_brokerv3.IdBasedBrokerRewriteRule{{Id: 1, Host: "broker0", Port: 9092}},
	}
	cfg, err := parseConfig(&kafka_brokerv3.KafkaBroker{
		StatPrefix: "k",
		BrokerAddressRewriteSpec: &kafka_brokerv3.KafkaBroker_IdBasedBrokerAddressRewriteSpec{
			IdBasedBrokerAddressRewriteSpec: spec,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.rewriteSpec == nil || len(cfg.rewriteSpec.GetRules()) != 1 {
		t.Fatalf("rewriteSpec not remembered: %v", cfg.rewriteSpec)
	}
}
