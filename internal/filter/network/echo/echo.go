package echo

import (
	"fmt"

	echov3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/echo/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
)

// TypeURL is the canonical Any type URL for the echo filter's typed_config.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.network.echo.v3.Echo"

// New is the NetworkFilterFactory for the echo filter. It is registered at
// boot under TypeURL. Echo's proto config (echov3.Echo) has no fields; an
// empty/absent typed_config body is accepted (parent AMEND-A2). No field-level
// rejection is performed.
func New(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
	cfg := &echov3.Echo{}
	if tc != nil && len(tc.GetValue()) > 0 {
		if err := tc.UnmarshalTo(cfg); err != nil {
			return nil, fmt.Errorf("echo: invalid typed_config: %w", err)
		}
	}
	return func() network.NetworkFilter { return &echoFilter{} }, nil
}

// echoFilter reflects every received byte back to the downstream connection.
type echoFilter struct {
	network.Marker
	cb network.ReadFilterCallbacks
}

// OnNewConnection returns Continue — echo has nothing to do at connection open.
func (f *echoFilter) OnNewConnection() network.Status { return network.Continue }

// OnData writes the buffered bytes back to the downstream connection, drains
// the buffer, and returns StopIteration (no further filters need to run).
func (f *echoFilter) OnData(buf *network.Buffer, endStream bool) network.Status {
	f.cb.Connection().Write(buf.Bytes(), endStream)
	buf.Drain(buf.Len())
	return network.StopIteration
}

// SetReadFilterCallbacks stores the per-connection callbacks handle.
func (f *echoFilter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks) { f.cb = cb }

// OnDestroy is a no-op; echo holds no per-connection resources.
func (f *echoFilter) OnDestroy() {}
