// Package helpers H3 driver — driver-side HTTP/3 round-tripper shared by
// BOTH sides of a cross-side differential fixture (reference Envoy container
// and subject envoy-go), each reached over QUIC/UDP.
//
// Per D-3.2 (see h2.go): this file lives under test/, NOT under internal/.
// The production import-hygiene gate confining quic-go to
// internal/listener/quic.go does NOT apply here — this is fixture
// infrastructure, not envoy-go runtime.
package helpers

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// H3RoundTrip issues a single HTTP/3 request to the QUIC listener at the
// pinned UDP addr and returns the decoded status/headers/body.
//
// Like H2RoundTrip (h2.go), it PINS the dialed address: the http3.Transport's
// Dial closure always dials `addr` regardless of the request URL authority
// and IGNORES any Alt-Svc / server-preferred-address the server advertises
// (a reference Envoy container advertises its INTERNAL port, not the
// host-mapped one). A fresh Transport is constructed per call (no connection
// caching) so consecutive invocations never reuse a QUIC connection.
func H3RoundTrip(
	ctx context.Context,
	addr string,
	tlsConf *tls.Config,
	method, path string,
	headers http.Header,
	body []byte,
) (status int, respHeaders http.Header, respBody []byte, err error) {
	tr := &http3.Transport{
		TLSClientConfig: tlsConf,
		QUICConfig:      &quic.Config{},
		// Dial pins the UDP dial to `addr` regardless of the request URL
		// host, and never follows Alt-Svc to a server-advertised address.
		Dial: func(ctx context.Context, _ string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
			return quic.DialAddr(ctx, addr, tlsCfg, cfg)
		},
	}
	defer func() { _ = tr.Close() }()

	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	// The URL host is cosmetic (the Dial closure pins the real dial); use
	// `addr` so req.Host is well-formed.
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("https://%s%s", addr, path), rdr)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("NewRequestWithContext: %w", err)
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("RoundTrip: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, fmt.Errorf("read body: %w", err)
	}
	return resp.StatusCode, resp.Header, respBody, nil
}
