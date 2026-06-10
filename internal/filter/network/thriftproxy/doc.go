// Package thriftproxy implements the envoy.filters.network.thrift_proxy network
// filter (ADR-0231): the project's SECOND terminal routing proxy. It decodes the
// Thrift framed×binary message-begin envelope under payload_passthrough, routes by
// method name through a route_config to one upstream cluster, round-trips each
// request through the REUSED ADR-0230 upstream-pool seam (one-conn-per-downstream,
// synchronous single-flight, positional correlation), and answers a routing miss
// with a local UnknownMethod Thrift exception. The 11th network-filter built-in.
package thriftproxy
