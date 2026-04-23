package helpers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"time"
)

// HTTPResponse is the structured form of an HTTP/1.1 response on the wire.
// The dimension-aware differential diff compares each field per fixture's
// expectations.yaml.
type HTTPResponse struct {
	StatusLine string
	Headers    map[string]string // canonical name → joined value; multi-value joined by ", " per textproto
	Body       []byte
}

// ParseHTTPResponse parses raw HTTP/1.1 response bytes (status line + headers
// + body) into a structured form. Uses net/http for status+headers and then
// reads the body to EOF. Errors on malformed status line.
func ParseHTTPResponse(raw []byte) (*HTTPResponse, error) {
	br := bufio.NewReader(bytes.NewReader(raw))
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return nil, fmt.Errorf("http_response: parse: %w", err)
	}
	body := make([]byte, 0, len(raw))
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if rerr != nil {
			break
		}
	}
	_ = resp.Body.Close()
	// Reconstruct the status line (http.ReadResponse consumes it).
	status := fmt.Sprintf("%s %s", resp.Proto, resp.Status)
	hdrs := map[string]string{}
	for k, vs := range resp.Header {
		hdrs[textproto.CanonicalMIMEHeaderKey(k)] = joinHeaderValues(vs)
	}
	return &HTTPResponse{StatusLine: status, Headers: hdrs, Body: body}, nil
}

// HTTPGetReadyRaw issues a raw-socket GET /ready and reads the full wire
// response. Not using net/http.Client because the diff needs the status line
// and headers as on-the-wire bytes (net/http's response object discards some
// wire detail like header ordering that the diff's set-equal allow-list
// tolerates but the body/status exact-match rule does not).
func HTTPGetReadyRaw(ctx context.Context, addr string) ([]byte, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}
	req := "GET /ready HTTP/1.1\r\nHost: " + addr + "\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return nil, err
	}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, rerr := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if rerr != nil {
			break
		}
	}
	return buf, nil
}

func joinHeaderValues(vs []string) string {
	if len(vs) == 1 {
		return vs[0]
	}
	out := ""
	for i, v := range vs {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}
