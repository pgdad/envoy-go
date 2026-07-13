// Package driver registers the 0103-xds-sds-server-cert fixture with the
// differential runner. It is the behavioral proof of the phase 60.2 SDS
// server-cert apply lift (ADR-0280): a downstream TLS listener whose
// certificate is fetched via a State-of-the-World Secret Discovery Service
// gRPC stream (not inlined in the bootstrap), on both the reference Envoy
// (contrib-v1.37.2, Docker) and envoy-go (subject, in-process).
//
// Integration shape (single-listener downstream-TLS tcp_proxy; driver-owned
// in-process SDS gRPC receiver; TCP echo backend):
//
//  1. TWO driver-owned sdsserver.Server receivers — one per side — on two
//     separately-allocated host ports (refSDSPort / subjSDSPort), both bound
//     on 0.0.0.0 BEFORE either proxy starts, each configured with the SAME
//     committed pki/leaf.pem + pki/leaf.key.pem (secret name "server_cert").
//     ReferenceBootstrap renders envoy.yaml pointing the sds_cluster at
//     host.docker.internal:refSDSPort (ADR-0010 STRICT_DNS bridge alias);
//     SubjectConfig renders envoy-go.yaml pointing at 127.0.0.1:subjSDSPort
//     (STATIC). BOTH bootstraps carry a fixed node{id: envoygo-node,
//     cluster: envoygo-cluster} — SDS boot-fails without it
//     (internal/boot.NewSDSProvider arm 7).
//
//  2. DriveReference / DriveSubject each dial the proxy's TLS listener,
//     complete a TLS 1.2+/1.3 handshake (validating against the committed
//     pki/ca.pem, serverName "sds.envoy-go.test"), and inspect the served
//     leaf via helpers.TLSServedLeaf — NO application data is written; the
//     observable is the certificate identity, not the echoed bytes. The
//     backend cluster (BackendCount=1, TCPEcho — the runner default) exists
//     only so tcp_proxy has somewhere to route; it is never exercised.
//
//  3. The runner's Step-7 CompareBytes enforces the returned
//     "serial=<hex>\nsan=<DNSNames>\n" observable byte-identical cross-side —
//     proving both proxies served the SAME SDS-delivered leaf.
//
// Both receivers are hard-stopped via Close() (grpc.Server.Stop) — NOT
// Stop()/GracefulStop — after the subject drive completes: like
// metricsservice's StreamMetrics, SDS's StreamSecrets is long-lived (the
// proxy holds the stream open awaiting future secret rotations), so
// GracefulStop would block until the test timeout
// (reference_periodic_sink_differential_two_receivers precedent).
package driver
