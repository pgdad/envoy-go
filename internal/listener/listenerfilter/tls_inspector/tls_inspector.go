package tls_inspector

import (
	"context"
	"encoding/binary"
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
//
// The peek is incremental: first 5 bytes (TLS record header) to learn the
// record length, then exactly 5+recordLen bytes capped at bufferSize. This
// avoids the bufio.Reader.Peek deadlock that would otherwise occur when the
// configured bufferSize exceeds the ClientHello's actual byte length —
// bufio's Peek(n) blocks until n bytes are available, which would hang the
// handshake forever (a typical ClientHello is ~250-350 bytes; the default
// bufferSize is 4096). Phase 07.2 Task 10 surfaced this deadlock when the
// listener-filter pipeline began running on real network connections.
func (f *filter) Inspect(ctx context.Context, peeker listenerfilter.Peeker, inputs *listenerfilter.ChainMatchInputs) (listenerfilter.ListenerFilterStatus, error) {
	// Step 1: peek the 5-byte TLS record header to learn the record length.
	hdr, err := peeker.Peek(5)
	if err != nil && len(hdr) == 0 {
		if errors.Is(err, context.Canceled) {
			return listenerfilter.Continue, ctx.Err()
		}
		inputs.TransportProtocol = "raw_buffer"
		return listenerfilter.Continue, nil
	}
	if len(hdr) < 5 || hdr[0] != 0x16 {
		// Too short for a TLS record header, or not a Handshake record.
		inputs.TransportProtocol = "raw_buffer"
		return listenerfilter.Continue, nil
	}
	// Step 2: derive the full record length, cap by bufferSize, and peek that
	// exact number of bytes. The cap tolerates pathological clients sending
	// out-of-spec record lengths.
	recordLen := int(binary.BigEndian.Uint16(hdr[3:5]))
	want := 5 + recordLen
	if want > f.cfg.bufferSize {
		want = f.cfg.bufferSize
	}
	buf, err := peeker.Peek(want)
	if err != nil && len(buf) == 0 {
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
