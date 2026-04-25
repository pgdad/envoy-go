package hcm

import (
	"context"
	"net"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

// NewFilter parses the typed_config Any into a *Filter. Errors begin with
// "hcm: "; the listener manager wraps them with "listener: %q: filter_chains[%d]: ".
func NewFilter(tc *anypb.Any, clusters *cluster.Manager) (*Filter, error) {
	return parseFilter(tc, clusters)
}

// Handle drives one downstream HTTP/1.1 connection from acceptance to close.
// On a canceled ctx, Handle returns promptly without reading any bytes.
// All errors are owned by runConnection (logging + downstream close).
//
// Filter implements internal/listener.filterHandler:
//
//	Handle(ctx context.Context, downstream net.Conn)
//
// (No error return — matches the phase-02 tcpproxy.Filter.Handle precedent.)
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	if err := ctx.Err(); err != nil {
		_ = downstream.Close()
		return
	}
	runConnection(ctx, downstream, f.table)
}
