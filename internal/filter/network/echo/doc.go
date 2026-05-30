// Package echo implements the envoy.filters.network.echo network read-filter.
// On every OnData call it writes the received bytes back to the downstream
// connection (via Connection().Write) and drains the buffer, then returns
// StopIteration so no further filters run. The echo filter's typed_config is
// intentionally empty (envoy.extensions.filters.network.echo.v3.Echo has no
// fields); an absent or zero-length typed_config body is also accepted
// (parent AMEND-A2 / SPEC §4.1).
package echo
