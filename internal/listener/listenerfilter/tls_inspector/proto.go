package tls_inspector

import (
	"fmt"

	tls_inspectorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	defaultBufferSize = 4096
	minBufferSize     = 256
	maxBufferSize     = 65536
)

func parseConfig(tc *anypb.Any) (*config, error) {
	if tc == nil {
		return &config{bufferSize: defaultBufferSize}, nil
	}
	var pb tls_inspectorv3.TlsInspector
	if err := tc.UnmarshalTo(&pb); err != nil {
		return nil, fmt.Errorf("tls_inspector: typed_config unmarshal: %w", err)
	}
	cfg := &config{bufferSize: defaultBufferSize}
	if pb.GetInitialReadBufferSize() != nil {
		v := int(pb.GetInitialReadBufferSize().GetValue())
		if v < minBufferSize {
			return nil, fmt.Errorf("tls_inspector: initial_read_buffer_size %d below floor %d", v, minBufferSize)
		}
		if v > maxBufferSize {
			v = maxBufferSize // clamp without error per ADR-0079 Decision C
		}
		cfg.bufferSize = v
	}
	// pb.EnableJa3Fingerprinting is silently ignored per SPEC §12.
	return cfg, nil
}
