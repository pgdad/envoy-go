// Package filter is the parent package for envoy-go's filter implementations.
// HTTP-side framework + filter implementations live under filter/http/; the
// HCM (HTTP connection manager network filter) lives under filter/hcm/; the
// TCP-proxy network filter lives under filter/tcpproxy/.
//
// The HTTP filter chain framework (introduced by phase 07.1 — ADR-0071)
// provides StreamDecoderFilter / StreamEncoderFilter interfaces, a
// freeze-after-boot extension registry (ADR-0072), a typed_per_filter_config
// 3-tier merge (ADR-0073), the per-stream FilterChain state machine with
// async-resume + body buffering (ADR-0076 buffer cap; ADR-0075 sendLocalReply
// semantics), and a starter filter set: cors (real Envoy filter; ADR-0074),
// envoygotest (test-only probe; ADR-0074), router (terminal filter; ADR-0071's
// total supersession of ADR-0040). See filter/http/doc.go for the package
// overview.
package filter
