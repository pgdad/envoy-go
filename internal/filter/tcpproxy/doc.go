// Package tcpproxy implements the envoy.filters.network.tcp_proxy filter:
// parses an envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy proto from
// the listener's filter typed_config Any, resolves its cluster reference at
// filter-construction time against the cluster manager, and on each accepted
// downstream connection picks an endpoint via the cluster's LB, dials it
// (honoring the cluster's connect_timeout), and pumps bytes bidirectionally
// with half-close propagation.
//
// The byte pump (netConn wrapper + bidirectional io.Copy + halfClose helper)
// is lifted verbatim from cmd/envoy-go/main.go's phase-00 implementation per
// ADR-0023; the splice(2) avoidance rationale is preserved in the netConn
// type doc-comment. See SPEC §5.5.
package tcpproxy
