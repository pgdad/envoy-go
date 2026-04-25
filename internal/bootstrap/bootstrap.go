package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	// Blank-imported so the filter extension's proto descriptor is registered
	// with protoregistry.GlobalTypes, which lets protojson round-trip the
	// typed_config Any without envoy-go interpreting its contents (ADR-0016).
	// Phase 01 fixtures only use tcp_proxy; later phases register additional
	// filters as fixtures introduce them.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	// Phase 04 (HTTP/1.1) registers the HCM network filter, the router HTTP
	// filter, and the route-config proto so protojson round-trips fixtures
	// 0003-* and any future HTTP fixtures without interpreting typed_config.
	// Per ADR-0016 the addition is a registry-population mechanism, not a
	// new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

// Load parses r as YAML (upstream Envoy's YAML shape), converts to JSON, and
// unmarshals into an Envoy v3 Bootstrap proto. Unknown fields at any depth
// cause an error (ADR-0016). The phase-01 unsupported surfaces
// dynamic_resources and layered_runtime cause an error even though the proto
// itself defines them.
//
// Every error returned by Load begins with "bootstrap: ".
func Load(r io.Reader) (*bootstrapv3.Bootstrap, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: read: %w", err)
	}
	var generic map[string]interface{}
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("bootstrap: yaml parse: %w", err)
	}
	if generic == nil {
		return nil, fmt.Errorf("bootstrap: empty document")
	}
	if _, ok := generic["dynamic_resources"]; ok {
		return nil, fmt.Errorf("bootstrap: dynamic_resources not supported in phase 01 (see SPEC §2)")
	}
	if _, ok := generic["layered_runtime"]; ok {
		return nil, fmt.Errorf("bootstrap: layered_runtime not supported in phase 01 (see SPEC §2)")
	}
	jsonBytes, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: to json: %w", err)
	}
	bs := &bootstrapv3.Bootstrap{}
	opts := protojson.UnmarshalOptions{DiscardUnknown: false}
	if err := opts.Unmarshal(jsonBytes, bs); err != nil {
		return nil, fmt.Errorf("bootstrap: protojson: %w", err)
	}
	return bs, nil
}

// AdminSocket returns host and port from admin.address.socket_address. Errors
// if admin is missing or the address is not a socket_address.
func AdminSocket(bs *bootstrapv3.Bootstrap) (host string, port uint32, err error) {
	adm := bs.GetAdmin()
	if adm == nil {
		return "", 0, fmt.Errorf("bootstrap: missing admin")
	}
	addr := adm.GetAddress()
	if addr == nil {
		return "", 0, fmt.Errorf("bootstrap: missing admin.address")
	}
	sa := addr.GetSocketAddress()
	if sa == nil {
		return "", 0, fmt.Errorf("bootstrap: admin.address is not a socket_address")
	}
	return sa.GetAddress(), sa.GetPortValue(), nil
}

// Phase-01 `FirstListenerSocket` and `FirstClusterEndpointSocket` helpers
// retired in phase 02 — listener and cluster traversal moved into
// `internal/listener.Manager` and `internal/cluster.Manager` respectively
// (ADR-0022).
