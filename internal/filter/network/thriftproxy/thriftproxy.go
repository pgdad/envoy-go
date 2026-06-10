package thriftproxy

import (
	thrift_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
	"google.golang.org/protobuf/proto"
)

// TypeURL is the canonical Any type URL for thrift_proxy's typed_config. Derived
// via proto.MessageName (NEVER a hand-typed docs string —
// reference_network_filter_typeurl_extensions; the redisproxy.go precedent).
// thrift_proxy/v3 is CORE /envoy v1.32.4 (AMEND-T1) — ZERO new go.mod dep.
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&thrift_proxyv3.ThriftProxy{}))
