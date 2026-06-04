package mongoproxy

import (
	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/proto"
)

// TypeURL is the canonical Any type URL for mongo_proxy's typed_config. Derived
// via proto.MessageName (NEVER a hand-typed docs string —
// reference_network_filter_typeurl_extensions; the zookeeperproxy.go:17
// precedent). Resolves to the parent §5.1 string (the extensions. segment).
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&mongo_proxyv3.MongoProxy{}))
