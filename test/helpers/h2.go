// Package helpers H2 driver — driver-side H2 round-tripper for fixture-0004.
//
// Per D-3.2: this file lives under test/, NOT under internal/. The forbidden
// runtime imports rule (no http2.Server/Transport in production code) does
// NOT apply here — this is fixture infrastructure, not envoy-go runtime.
package helpers

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// H2RoundTrip dials addr over TLS (with the given config + NextProtos h2),
// constructs a fresh *http2.ClientConn, issues one HTTP/2 request, returns
// (status, response-headers, response-body, error).
//
// A fresh *http2.Transport is constructed per call (no caching) so consecutive
// invocations from the same goroutine never reuse a *http2.ClientConn — required
// for fixture 0004's RR distribution determinism per ADR-0028 + the 05.2 PLAN
// "Settled SPEC §10 deferred decisions" #13.
//
// The returned respHeaders include the :status pseudo-header as the first element
// (the codec preserves wire order; pseudo-headers are first per RFC 9113 §8.3).
func H2RoundTrip(
	ctx context.Context,
	addr string,
	tlsConf *tls.Config,
	method, path string,
	headers []hpack.HeaderField,
	body []byte,
) (status int, respHeaders []hpack.HeaderField, respBody []byte, err error) {
	d := &net.Dialer{}
	rawConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("dial: %w", err)
	}
	tlsConn := tls.Client(rawConn, tlsConf)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return 0, nil, nil, fmt.Errorf("handshake: %w", err)
	}
	if alpn := tlsConn.ConnectionState().NegotiatedProtocol; alpn != "h2" {
		_ = tlsConn.Close()
		return 0, nil, nil, fmt.Errorf("alpn negotiated %q, want %q", alpn, "h2")
	}
	tr := &http2.Transport{TLSClientConfig: tlsConf}
	cc, err := tr.NewClientConn(tlsConn)
	if err != nil {
		_ = tlsConn.Close()
		return 0, nil, nil, fmt.Errorf("NewClientConn: %w", err)
	}
	defer func() { _ = cc.Close() }()
	// Construct *http.Request and round-trip via the standard library Transport surface.
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("https://%s%s", addr, path), bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("NewRequestWithContext: %w", err)
	}
	for _, hf := range headers {
		req.Header.Add(hf.Name, hf.Value)
	}
	resp, err := cc.RoundTrip(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("RoundTrip: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("read body: %w", err)
	}
	for k, vv := range resp.Header {
		for _, v := range vv {
			respHeaders = append(respHeaders, hpack.HeaderField{Name: k, Value: v})
		}
	}
	// Prepend :status pseudo-header for caller convenience.
	respHeaders = append([]hpack.HeaderField{{Name: ":status", Value: strconv.Itoa(resp.StatusCode)}}, respHeaders...)
	return resp.StatusCode, respHeaders, respBody, nil
}

// H2CRoundTrip dials addr over PLAINTEXT TCP and speaks HTTP/2 prior-knowledge
// (h2c, RFC 7540 §3.4 — no TLS, no ALPN), issues one HTTP/2 request, and returns
// (status, response-headers, response-body, error). A fresh *http2.ClientConn is
// constructed per call (no caching).
//
// Used by fixture 0079 to hit the H2HoldResponder backend's /__release control
// path (the backend serves h2c only; a plain HTTP/1.1 control request would not
// parse). The scheme is "http" so the h2c framing is used without TLS.
func H2CRoundTrip(
	ctx context.Context,
	addr string,
	method, path string,
	headers []hpack.HeaderField,
	body []byte,
) (status int, respHeaders []hpack.HeaderField, respBody []byte, err error) {
	d := &net.Dialer{}
	rawConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("dial: %w", err)
	}
	tr := &http2.Transport{
		AllowHTTP: true, // permit "http" scheme (h2c)
		// DialTLSContext is consulted even for AllowHTTP; return the raw conn
		// unwrapped (no TLS) so the transport speaks h2c prior-knowledge.
		DialTLSContext: func(_ context.Context, _, _ string, _ *tls.Config) (net.Conn, error) {
			return rawConn, nil
		},
	}
	cc, err := tr.NewClientConn(rawConn)
	if err != nil {
		_ = rawConn.Close()
		return 0, nil, nil, fmt.Errorf("NewClientConn: %w", err)
	}
	defer func() { _ = cc.Close() }()
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://%s%s", addr, path), bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("NewRequestWithContext: %w", err)
	}
	for _, hf := range headers {
		req.Header.Add(hf.Name, hf.Value)
	}
	resp, err := cc.RoundTrip(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("RoundTrip: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("read body: %w", err)
	}
	for k, vv := range resp.Header {
		for _, v := range vv {
			respHeaders = append(respHeaders, hpack.HeaderField{Name: k, Value: v})
		}
	}
	respHeaders = append([]hpack.HeaderField{{Name: ":status", Value: strconv.Itoa(resp.StatusCode)}}, respHeaders...)
	return resp.StatusCode, respHeaders, respBody, nil
}
