package kafkabroker

import (
	kafka_brokerv3 "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"
	"google.golang.org/protobuf/proto"
)

// TypeURL is the config @type for the kafka_broker filter. Pinned by TestTypeURL;
// NEVER hand-typed. Carries the `extensions.` segment per
// reference_network_filter_typeurl_extensions.
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&kafka_brokerv3.KafkaBroker{}))
