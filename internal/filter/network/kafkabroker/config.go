package kafkabroker

import (
	"errors"

	kafka_brokerv3 "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"

	"github.com/pgdad/envoy-go/internal/stats"
)

// PARSE-REJECT arms (ADR-0080 byte-stable; SPEC §5.2). Every arm mirrors an
// upstream PGV failure — NO departure-class rejects. The error prefix for all
// kafka_broker arms is "kafka_broker: " (mirrors mongo_proxy: / zookeeper_proxy:).
// These strings are byte-stable from this commit forward — DO NOT CHANGE.
const (
	errStatPrefixRequired      = "kafka_broker: stat_prefix is required"
	errStatPrefixInvalid       = "kafka_broker: stat_prefix contains characters invalid for a stat name"
	errApiKeyOutOfRange        = "kafka_broker: api_keys_allowed/api_keys_denied values must be in [0, 32767]"
	errRewriteSpecNil          = "kafka_broker: broker_address_rewrite_spec oneof value cannot be a typed-nil"
	errRewriteRuleHostRequired = "kafka_broker: id_based_broker_address_rewrite_spec: rule host is required"
	errRewriteRulePortTooLarge = "kafka_broker: id_based_broker_address_rewrite_spec: rule port must be <= 65535"
)

// apiKeyMax is the int16 max — the api_keys_allowed/api_keys_denied PGV upper
// bound (§5.2). Each configured api key must be in [0, apiKeyMax].
const apiKeyMax = 32767

// rewriteRulePortMax is the uint16 max — the rewrite-rule port PGV upper bound.
const rewriteRulePortMax = 65535

// compiledConfig is the boot-parsed, per-listener-shared kafka_broker config
// (ADR-0079 two-step factory).
type compiledConfig struct {
	statPrefix string

	// The four active features are parsed-and-remembered for faithfulness but
	// their BEHAVIOR is deferred (§2): force_response_rewrite, the rewrite spec,
	// api_keys_allowed/denied. They drive NO runtime branch in phase 31.
	forceResponseRewrite bool
	apiKeysAllowed       []uint32
	apiKeysDenied        []uint32
	rewriteSpec          *kafka_brokerv3.IdBasedBrokerRewriteSpec

	// stats is the eagerly-created 176-counter roster (Task 6 kafkaStats), set by
	// the factory (NewFactory) once per distinct stat_prefix at boot and shared
	// across the listener's per-connection filter instances. nil until the factory
	// attaches it (test-constructed configs may leave it nil and wire a roster
	// directly onto the decoder).
	stats *kafkaStats
}

// parseConfig validates + compiles the 5-field proto (SPEC §5.2 roster). The
// four active features are parse-and-store (behavior deferred — §2). Returns
// PARSE-REJECT errors on PGV-mirror violations (ADR-0080 byte-stable). Validation
// order: stat_prefix first, then api keys, then the rewrite spec.
func parseConfig(msg *kafka_brokerv3.KafkaBroker) (*compiledConfig, error) {
	sp := msg.GetStatPrefix()
	if sp == "" {
		return nil, errors.New(errStatPrefixRequired)
	}
	if !stats.IsValidName(sp) { // config-boundary charset guard (AMEND-K7; the rbac/mongo precedent)
		return nil, errors.New(errStatPrefixInvalid)
	}
	cfg := &compiledConfig{
		statPrefix:           sp,
		forceResponseRewrite: msg.GetForceResponseRewrite(),
		apiKeysAllowed:       msg.GetApiKeysAllowed(),
		apiKeysDenied:        msg.GetApiKeysDenied(),
	}
	for _, k := range cfg.apiKeysAllowed {
		if k > apiKeyMax {
			return nil, errors.New(errApiKeyOutOfRange)
		}
	}
	for _, k := range cfg.apiKeysDenied {
		if k > apiKeyMax {
			return nil, errors.New(errApiKeyOutOfRange)
		}
	}
	if hasRewriteOneof(msg) {
		spec := msg.GetIdBasedBrokerAddressRewriteSpec()
		if spec == nil { // a set oneof wrapper holding a nil message — the typed-nil arm
			return nil, errors.New(errRewriteSpecNil)
		}
		for _, r := range spec.GetRules() {
			if r.GetHost() == "" {
				return nil, errors.New(errRewriteRuleHostRequired)
			}
			if r.GetPort() > rewriteRulePortMax {
				return nil, errors.New(errRewriteRulePortTooLarge)
			}
		}
		cfg.rewriteSpec = spec
	}
	return cfg, nil
}

// hasRewriteOneof reports whether the broker_address_rewrite_spec oneof is SET
// (even to a typed-nil) — distinguishing "field absent" from "oneof set to nil".
func hasRewriteOneof(msg *kafka_brokerv3.KafkaBroker) bool {
	_, ok := msg.GetBrokerAddressRewriteSpec().(*kafka_brokerv3.KafkaBroker_IdBasedBrokerAddressRewriteSpec)
	return ok
}
