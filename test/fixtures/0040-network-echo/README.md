# 0040-network-echo

Cross-side differential fixture for the phase-26.1 network read-filter
framework. It boots BOTH the envoy-go subject and reference Envoy
v1.37.2 (in Docker) with a single `filter_chains` entry whose only
network filter is the echo filter
(`envoy.filters.network.echo`), and asserts byte-exact wire parity
on the raw TCP path.

The echo network filter reflects the inbound byte stream straight back
to the downstream connection. The driver pumps a deterministic
multi-line payload (`ping-0\n` … `ping-9\n`) through each proxy's
listener and reads to 1s idle, and the runner compares the two response
streams byte-for-byte. Because the payload spans several
newline-separated lines, it exercises the filter's `OnData` read loop
across buffer reads on both sides.

The driver does NOT half-close the write side (it uses
`helpers.TCPRoundTripNoHalfClose` rather than the half-closing
`TCPRoundTrip` 0000-tcp-echo uses). A downstream FIN co-arriving with
the payload makes reference Envoy v1.37.2's echo filter tear the
connection down before flushing the echoed bytes back — characterised
in this fixture: a FIN simultaneous with the payload yields zero echoed
bytes from the reference, while any gap before the FIN (or no FIN at all)
yields the full echo. Leaving the write side open and draining on the
idle timeout is byte-exact on both the reference and the subject for the
pure read-only echo filter. The
fixture also probes each proxy's admin `/ready` endpoint and diffs the
raw responses (status line + headers modulo the BEHAVIOR_CONTRACT
allow-list + `LIVE\n` body), mirroring 0000-tcp-echo.

## @type note

The echo `typed_config` uses
`type.googleapis.com/envoy.extensions.filters.network.echo.v3.Echo`
— the `extensions.*` form (verified this session via `proto.MessageName`
plus a boot smoke). The filter `name:` field stays the canonical
`envoy.filters.network.echo`. The go-control-plane v1.32.4 ↔ Envoy
v1.37.2 version match (ADR-0008) makes this URL valid on both sides.

## Cluster

Both bootstraps declare a `c_echo` backend cluster even though the echo
filter never routes to it: envoy-go's cluster manager boots before the
listener manager and rejects a zero-cluster bootstrap. The cluster
(STRICT_DNS `host.docker.internal` on the reference, STATIC `127.0.0.1`
on the subject — mirroring 0000-tcp-echo's §5.4 host-subprocess
divergences) satisfies boot and keeps both configs shape-identical; the
runner spawns one spare TCP-echo backend (`BackendCount() == 1`) that
this fixture does not round-trip through.

## Driver

`driver/driver.go` implements the 8-method `fixture.Driver`. `init()`
registers the driver under the fixture directory name; the driver is
blank-imported into `test/differential/runner_test.go`.
