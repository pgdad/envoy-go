package tcpproxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/drain"
	"github.com/pgdad/envoy-go/internal/filter/network"
)

// TypeURL is the proto type URL phase 02 registers in the listener's inline
// filter constructor registry. Exported so the listener package can reference
// it without re-stringifying.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// Filter is one TCP proxy filter instance. It retains both the cluster manager
// (for per-connection override resolution, ADR-0219) and the boot-resolved
// default cluster (no-override fallback). Immutable after NewFilter returns.
type Filter struct {
	network.Marker                  // sealed-marker embed: grants isNetworkFilter() so *Filter satisfies network.TerminalFilter (26.2 Task 5; R-T)
	cm             *cluster.Manager // retained for per-connection override resolution (ADR-0219)
	defaultCluster *cluster.Cluster // the boot-resolved configured cluster (no-override fallback)
	statPrefix     string           // unread at phase 02; SPEC §10 #8 settled — stored for forward-compat
	dm             *drain.Manager   // 08.2 Task 10: nil-tolerant drain manager for in-flight Inc/Dec
	hashOnSourceIP bool             // 36.1 Task 6: TcpProxy.hash_policy source_ip → stuff cluster.WithHashKey(HashSourceIP) into ctx before Dial
}

// Compile-time assertion: *Filter is a network.TerminalFilter (R-T). Its
// existing Handle(ctx, net.Conn) is byte-identical to the TerminalFilter
// signature; only the network.Marker embed was added to seal it.
var _ network.TerminalFilter = (*Filter)(nil)

// NewFilter parses tc as a TcpProxy proto and resolves its cluster reference
// against cm. dm is the drain manager used for per-connection in-flight
// tracking (ADR-0096); pass nil if drain tracking is not needed.
// Returns an error if (a) tc.TypeUrl is not the TcpProxy URL, (b)
// the proto bytes do not unmarshal, (c) the cluster reference is missing or
// names a cluster cm does not know, or (d) the proto uses weighted_clusters
// (phase 02 does not implement weighted dispatch).
//
// Every error begins with "tcpproxy: ".
func NewFilter(tc *anypb.Any, cm *cluster.Manager, dm *drain.Manager) (*Filter, error) {
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
		// 36.1 Task 6: parse TcpProxy.hash_policy. source_ip → consistent-hash
		// key from the bare client IP; any other specifier DEPARTURE-rejects at
		// parse (fail-fast). No hash_policy → hashOnSourceIP stays false (the
		// existing byte-stable behavior; ctx untouched in Handle).
		hashOnSourceIP := false
		for _, hp := range msg.GetHashPolicy() {
			switch hp.GetPolicySpecifier().(type) {
			case *typev3.HashPolicy_SourceIp_:
				hashOnSourceIP = true
			default:
				return nil, fmt.Errorf("tcpproxy: hash_policy specifier %T is not supported (only source_ip)", hp.GetPolicySpecifier())
			}
		}
		return &Filter{cm: cm, defaultCluster: c, statPrefix: msg.GetStatPrefix(), dm: dm, hashOnSourceIP: hashOnSourceIP}, nil
	case *tcpproxyv3.TcpProxy_WeightedClusters:
		return nil, fmt.Errorf("tcpproxy: weighted_clusters is not supported in phase 02")
	default:
		return nil, fmt.Errorf("tcpproxy: cluster_specifier is missing or of unsupported type %T", cs)
	}
}

// NewNetworkFactory returns a network.NetworkFilterFactory that builds the
// tcp_proxy terminal filter once per chain at boot (capturing cm + dm) and
// yields that SHARED *Filter per accepted connection. tcp_proxy is
// conn-stateless (per-connection state lives on Handle's stack), so the
// shared instance preserves the retired chainInfo.filter semantic. The
// FactoryCtx per-chain fields are ignored (tcp_proxy has no listener-ctx
// dependency). The existing NewFilter parse-reject errors surface verbatim
// (byte-stable; R-S).
func NewNetworkFactory(cm *cluster.Manager, dm *drain.Manager) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		f, err := NewFilter(tc, cm, dm)
		if err != nil {
			return nil, err
		}
		return func() network.NetworkFilter { return f }, nil
	}
}

// Handle pumps bytes bidirectionally between downstream and a freshly-dialed
// upstream picked via the effective cluster's LB. The effective cluster is
// resolved per-connection: if ctx carries an upstream-cluster override
// (published by sni_cluster via network.UpstreamClusterOverride, ADR-0219)
// that names a known cluster, it is used; otherwise the boot-resolved default
// cluster is used. An unknown override cluster → close downstream with zero
// bytes (F-NOROUTE, D27-4). Closes downstream and upstream when the pump
// completes (or on dial failure). Logs but does not return errors.
func (f *Filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	if err := ctx.Err(); err != nil {
		return
	}
	eff := f.defaultCluster
	if override, ok := network.UpstreamClusterOverride(ctx); ok && override != "" {
		c, found := f.cm.Get(override)
		if !found {
			// F-NOROUTE (D27-4): unknown override cluster → close downstream, zero
			// bytes. Envoy increments downstream_cx_no_route + NoFlush-closes; that
			// counter family is NOT mirrored (§7.2). The deferred downstream.Close()
			// (FIN) yields zero-byte body parity regardless of FIN-vs-RST (D27-S3).
			log.Printf("tcpproxy: per-connection override cluster %q not found", override)
			return
		}
		eff = c
	}
	// 08.2 (Task 10) drain Inc/Dec per ADR-0096 + planner-time decision 5:
	// per-connection granularity (TCP-proxy has no per-request semantic).
	// Inc at conn-begin (after ctx.Err check, before Dial); Dec via defer
	// for pair-balance on all early-return paths (dial failure, etc.).
	if f.dm != nil {
		f.dm.Inc()
		defer f.dm.Dec()
	}
	// 36.1 Task 6: if a source_ip hash_policy was configured, stuff the
	// consistent-hash key (xxHash64 of the bare client IP, port stripped) into
	// ctx so eff's ring_hash LB can pick a host with source-IP affinity. No
	// hash_policy → ctx is left untouched (byte-stable; PICK-INPUT unset).
	if f.hashOnSourceIP {
		ctx = cluster.WithHashKey(ctx, cluster.HashSourceIP(downstream.RemoteAddr().String()))
	}
	upstream, _, err := eff.Dial(ctx)
	if err != nil {
		log.Printf("tcpproxy: dial cluster %q: %v", eff.Name(), err)
		return
	}
	defer func() { _ = upstream.Close() }()

	// 29.3 (§3.5): record which pump EOF'd first so a mongo_proxy chain can key
	// cx_destroy_local/remote_with_active_rq. The downstream→upstream pump returns
	// on downstream (LOCAL) EOF; the upstream→downstream pump on upstream (REMOTE)
	// EOF. Additive — chains without a readChainConn (no SetCloseDirection method)
	// are unaffected. The chain does first-wins CAS internally.
	recordDir := func(d network.CloseDirection) {
		if sd, ok := downstream.(interface{ SetCloseDirection(network.CloseDirection) }); ok {
			sd.SetCloseDirection(d)
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(netConn{upstream}, netConn{downstream})
		recordDir(network.CloseDirectionLocal) // downstream EOF first → local
		halfClose(upstream)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(netConn{downstream}, netConn{upstream})
		recordDir(network.CloseDirectionRemote) // upstream EOF first → remote
		halfClose(downstream)
	}()
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
