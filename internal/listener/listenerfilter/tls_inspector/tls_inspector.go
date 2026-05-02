package tls_inspector

import (
	"context"
	"errors"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/listener/listenerfilter"
)

// TypeURL is the proto type_url for the tls_inspector listener filter, per
// upstream go-control-plane.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector"

// New is the ListenerFilterFactory entry point. Parses the typed_config
// (TlsInspector proto), validates initial_read_buffer_size against the
// [256, 65536] envelope (defaults 4096 if unset), and returns a
// per-connection FilterInstanceFactory closure capturing the parsed config.
func New(tc *anypb.Any, ctx listenerfilter.FactoryCtx) (listenerfilter.FilterInstanceFactory, error) {
	cfg, err := parseConfig(tc)
	if err != nil {
		return nil, err
	}
	return func() listenerfilter.ListenerFilter {
		return &filter{cfg: cfg}
	}, nil
}

// config is the parsed tls_inspector configuration.
type config struct {
	bufferSize int
}

// filter is the per-connection ListenerFilter instance.
type filter struct {
	cfg *config
}

// Inspect peeks the connection preamble; if a TLS ClientHello is detected,
// populates inputs with extracted SNI + ALPN. Else sets
// inputs.TransportProtocol = "raw_buffer". Always returns Continue (the
// pipeline advances regardless of inspection outcome).
func (f *filter) Inspect(ctx context.Context, peeker listenerfilter.Peeker, inputs *listenerfilter.ChainMatchInputs) (listenerfilter.ListenerFilterStatus, error) {
	buf, err := peeker.Peek(f.cfg.bufferSize)
	if err != nil && len(buf) == 0 {
		// Connection closed before any bytes arrived; non-fatal — let the
		// chain-match algorithm operate on the un-inspected facts.
		if errors.Is(err, context.Canceled) {
			return listenerfilter.Continue, ctx.Err()
		}
		inputs.TransportProtocol = "raw_buffer"
		return listenerfilter.Continue, nil
	}
	sni, alpns, ok := parseClientHello(buf)
	if !ok {
		inputs.TransportProtocol = "raw_buffer"
		return listenerfilter.Continue, nil
	}
	inputs.TransportProtocol = "tls"
	if sni != "" {
		inputs.ServerName = sni
	}
	if len(alpns) > 0 {
		inputs.ApplicationProtocols = alpns
	}
	return listenerfilter.Continue, nil
}

// OnDestroy releases per-connection resources. tls_inspector holds none.
func (f *filter) OnDestroy() {}
