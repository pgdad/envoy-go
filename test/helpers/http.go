package helpers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// HTTPRoundTrip dials addr, writes one HTTP/1.1 request, reads one response,
// drains the response body into memory, and returns the response (with Body
// replaced by an in-memory reader) plus the body bytes. The connection is
// closed on every exit path.
//
// Used by:
//   - test/fixtures/0003-http11-routing/driver to drive the differential gate.
//   - test/differential/runner_test.go's per-request HTTPExpectations
//     orchestration pass (ADR-0043).
//
// ctx governs the dial deadline (via net.Dialer.DialContext) and is propagated
// to the request via http.NewRequestWithContext.
func HTTPRoundTrip(ctx context.Context, addr, method, path string, headers http.Header, body []byte) (*http.Response, []byte, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+addr+path, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("new request: %w", err)
	}
	if headers != nil {
		for k, vv := range headers {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
	}
	// HTTP/1.1, default Connection: close per request to keep RR distribution
	// deterministic on the fixture-0003 subject side (per-request fresh dial,
	// ADR-0039). Callers that want keep-alive can override.
	if req.Header.Get("Connection") == "" {
		req.Header.Set("Connection", "close")
	}

	if err := req.Write(conn); err != nil {
		return nil, nil, fmt.Errorf("write request: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read body: %w", err)
	}
	resp.Body = io.NopCloser(strings.NewReader(""))
	return resp, bodyBytes, nil
}
