// internal/filter/network/upstreamcluster.go — connection-scoped
// upstream-cluster-override ctx channel (ADR-0219). handleTerminal wraps the
// terminal call ctx with the override a read filter (sni_cluster) published;
// the terminal filter (tcp_proxy) reads it via UpstreamClusterOverride. This is
// the reader half of the narrow override seam (the writer half is
// ReadFilterCallbacks.SetUpstreamCluster → chainRuntime.upstreamClusterOverride).

package network

import "context"

type upstreamClusterKey struct{}

// withUpstreamClusterOverride returns ctx carrying override for the terminal
// filter to read via UpstreamClusterOverride. Internal to the framework's
// terminal handoff; not part of the public filter API.
func withUpstreamClusterOverride(ctx context.Context, override string) context.Context {
	return context.WithValue(ctx, upstreamClusterKey{}, override)
}

// UpstreamClusterOverride returns the per-connection upstream-cluster-override a
// read filter published (ADR-0219), or ("", false) if none. tcp_proxy reads it
// at Handle to override its configured cluster.
func UpstreamClusterOverride(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(upstreamClusterKey{}).(string)
	return v, ok
}
