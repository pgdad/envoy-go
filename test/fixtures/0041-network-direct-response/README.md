# 0041-network-direct-response

Cross-side differential fixture for the phase-26.1 network read-filter
framework. It boots BOTH the envoy-go subject and reference Envoy
v1.37.2 (in Docker) with a single `filter_chains` entry whose only
network filter is the direct_response filter
(`envoy.filters.network.direct_response`) configured with an
`inline_string` response, and asserts byte-exact wire parity on the
raw TCP path.

The direct_response network filter writes its static body in
`OnNewConnection` and then closes the connection (`FlushWrite`). The
driver dials each proxy's listener, sends an empty payload, half-closes
the write side, and reads to EOF; the returned bytes must equal
`envoy-go-direct-response\n` on both sides, which the runner compares
byte-for-byte. The fixture also probes each proxy's admin `/ready`
endpoint and diffs the raw responses (status line + headers modulo the
BEHAVIOR_CONTRACT allow-list + `LIVE\n` body), mirroring 0000-tcp-echo
and 0040-network-echo.

## @type note

The direct_response `typed_config` uses
`type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config`
— note the message name is `Config`, not `DirectResponse`. The
go-control-plane v1.32.4 ↔ Envoy v1.37.2 version match (ADR-0008) makes
this URL valid on both sides.

## Cluster

Both bootstraps declare a `c_echo` backend cluster even though the
direct_response filter never routes to it: envoy-go's cluster manager
boots before the listener manager and rejects a zero-cluster bootstrap.
The cluster (STRICT_DNS `host.docker.internal` on the reference, STATIC
`127.0.0.1` on the subject — mirroring 0000-tcp-echo's §5.4
host-subprocess divergences) satisfies boot and keeps both configs
shape-identical; the runner spawns one spare TCP-echo backend
(`BackendCount() == 1`) that this fixture does not round-trip through.

## Driver

`driver/driver.go` implements the 8-method `fixture.Driver`. `init()`
registers the driver under the fixture directory name; the driver is
blank-imported into `test/differential/runner_test.go`.
