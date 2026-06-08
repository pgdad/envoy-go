// Package kafkabroker implements the envoy.filters.network.kafka_broker network
// filter (ADR-0228): a passive both-direction Kafka observability sniffer that
// decodes the request/response HEADER ONLY and emits full per-API-key
// request.<msg>_request / response.<msg>_response counter parity under
// kafka.<stat_prefix>. It is the 9th built-in network filter, consumer #3 of the
// ADR-0221 WriteFilter seam, and the project's first /contrib consumer. It NEVER
// mutates the byte stream (a pure copying sniffer; always Continue).
package kafkabroker
