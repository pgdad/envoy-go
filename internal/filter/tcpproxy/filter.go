package tcpproxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

// TypeURL is the proto type URL phase 02 registers in the listener's inline
// filter constructor registry. Exported so the listener package can reference
// it without re-stringifying.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// Filter is one TCP proxy filter instance, bound at construction time to the
// resolved cluster it dispatches to. Immutable after NewFilter returns.
type Filter struct {
	cluster    *cluster.Cluster
	statPrefix string // unread at phase 02; SPEC §10 #8 settled — stored for forward-compat
}

// NewFilter parses tc as a TcpProxy proto and resolves its cluster reference
// against cm. Returns an error if (a) tc.TypeUrl is not the TcpProxy URL, (b)
// the proto bytes do not unmarshal, (c) the cluster reference is missing or
// names a cluster cm does not know, or (d) the proto uses weighted_clusters
// (phase 02 does not implement weighted dispatch).
//
// Every error begins with "tcpproxy: ".
func NewFilter(tc *anypb.Any, cm *cluster.Manager) (*Filter, error) {
	if got := tc.GetTypeUrl(); got != TypeURL {
		return nil, fmt.Errorf("tcpproxy: wrong type_url %q (want %q)", got, TypeURL)
	}
	msg := &tcpproxyv3.TcpProxy{}
	if err := tc.UnmarshalTo(msg); err != nil {
		return nil, fmt.Errorf("tcpproxy: unmarshal: %w", err)
	}
	switch cs := msg.GetClusterSpecifier().(type) {
	case *tcpproxyv3.TcpProxy_Cluster:
		name := cs.Cluster
		if name == "" {
			return nil, fmt.Errorf("tcpproxy: cluster reference is empty")
		}
		c, ok := cm.Get(name)
		if !ok {
			return nil, fmt.Errorf("tcpproxy: cluster %q not found", name)
		}
		return &Filter{cluster: c, statPrefix: msg.GetStatPrefix()}, nil
	case *tcpproxyv3.TcpProxy_WeightedClusters:
		return nil, fmt.Errorf("tcpproxy: weighted_clusters is not supported in phase 02")
	default:
		return nil, fmt.Errorf("tcpproxy: cluster_specifier is missing or of unsupported type %T", cs)
	}
}

// Handle pumps bytes bidirectionally between downstream and a freshly-dialed
// upstream picked via the cluster's LB. Closes downstream and upstream when
// the pump completes (or on dial failure). Logs but does not return errors.
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	if err := ctx.Err(); err != nil {
		return
	}
	upstream, _, err := f.cluster.Dial(ctx)
	if err != nil {
		log.Printf("tcpproxy: dial cluster %q: %v", f.cluster.Name(), err)
		return
	}
	defer func() { _ = upstream.Close() }()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(netConn{upstream}, netConn{downstream}); halfClose(upstream) }()
	go func() { defer wg.Done(); _, _ = io.Copy(netConn{downstream}, netConn{upstream}); halfClose(downstream) }()
	wg.Wait()
}

// netConn wraps net.Conn and hides the *net.TCPConn type, preventing
// io.Copy from using the Linux splice(2) syscall optimisation. splice can
// return 0 bytes when the source socket has data+FIN already queued, causing
// silent data loss on loopback. Using a plain Read/Write loop via a 32 KiB
// heap buffer is fast enough for the phase-01 test workload. (Lifted verbatim
// from phase 00 cmd/envoy-go/main.go per ADR-0023.)
type netConn struct{ net.Conn }

// halfClose half-closes the write side of c when c supports CloseWrite.
// As of phase 06.1 Task 9, Cluster.Dial wraps every successful conn in a
// *cluster.connWithGauge; rather than enumerate every concrete type that
// satisfies CloseWrite (which would re-couple this file to cluster's
// internal wrapper), we use the interface shape directly. *net.TCPConn,
// *stdtls.Conn, and *cluster.connWithGauge all satisfy
// `interface{ CloseWrite() error }`; transports that do not (e.g., a plain
// io.ReadWriteCloser pipe) are silently no-op'd.
func halfClose(c net.Conn) {
	type closeWriter interface{ CloseWrite() error }
	if cw, ok := c.(closeWriter); ok {
		_ = cw.CloseWrite()
	}
}
