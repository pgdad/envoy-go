// TEMPORARY — deleted in Task 12. Task 12's real implementation honours the
// ListenerCtx fields; this stub is a zero-value pass-through that keeps the
// build green while Task 11 wires the plumbing.
package hcm

import (
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

// ListenerCtx carries per-listener context that the HCM filter constructor
// consults at build time. Phase 05.1 introduces this to plumb the --allow-h2c
// flag from cmd/envoy-go/main through to parseFilterWithCtx (per ADR-0049).
// Future phases may extend.
type ListenerCtx struct {
	HasTLS   bool
	AllowH2C bool
}

// NewFilterWithCtx constructs a Filter, permitting codec_type HTTP2 when
// lc.HasTLS or lc.AllowH2C is true.
//
// STUB: Task 12 replaces this with a real implementation that wires the H2
// ServerConn path. For now, the stub accepts the proto (treating HTTP2 like
// HTTP1 at runtime) so that Task 11's tests compile and pass green without the
// full ALPN dispatch being wired.
func NewFilterWithCtx(tc *anypb.Any, clusters *cluster.Manager, lc ListenerCtx) (*Filter, error) {
	// If codec_type is HTTP2 and the listenerCtx permits it (TLS or allowH2C),
	// re-encode the proto with codec_type=AUTO so that the phase-04 parseFilter
	// validation passes. Task 12 replaces this with real H2 dispatch.
	if lc.HasTLS || lc.AllowH2C {
		msg := &hcmv3.HttpConnectionManager{}
		if err := tc.UnmarshalTo(msg); err == nil && msg.GetCodecType() == hcmv3.HttpConnectionManager_HTTP2 {
			msg.CodecType = hcmv3.HttpConnectionManager_AUTO
			if replaced, err2 := anypb.New(msg); err2 == nil {
				tc = replaced
			}
		}
	}
	return NewFilter(tc, clusters)
}
