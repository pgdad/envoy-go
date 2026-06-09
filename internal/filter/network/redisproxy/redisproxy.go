package redisproxy

import (
	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	"google.golang.org/protobuf/proto"
)

// TypeURL is the canonical Any type URL for redis_proxy's typed_config. Derived
// via proto.MessageName (NEVER a hand-typed docs string —
// reference_network_filter_typeurl_extensions; the kafkabroker.go/mongoproxy.go
// precedent). Resolves to the SPEC §5.1 string (the extensions. segment).
// redis_proxy/v3 is CORE /envoy v1.32.4 (AMEND-R1) — ZERO new go.mod dep.
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&redis_proxyv3.RedisProxy{}))
