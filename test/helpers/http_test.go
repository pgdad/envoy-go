package helpers

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// startEcho starts an in-process HTTP/1.1 echo server that reads one request
// per accepted connection, writes one canned response, and closes.
func startEcho(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				buf := make([]byte, 4096)
				_, _ = c.Read(buf)
				_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\nContent-Type: text/plain\r\n\r\nhello"))
			}(c)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func TestHTTPRoundTrip_Happy(t *testing.T) {
	addr, cleanup := startEcho(t)
	defer cleanup()
	resp, body, err := HTTPRoundTrip(context.Background(), addr, "GET", "/x", nil, nil)
	if err != nil {
		t.Fatalf("HTTPRoundTrip: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if string(body) != "hello" {
		t.Errorf("body: got %q, want %q", string(body), "hello")
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain" {
		t.Errorf("Content-Type: got %q, want %q", got, "text/plain")
	}
}

func TestHTTPRoundTrip_CtxCanceledBeforeDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := HTTPRoundTrip(ctx, "10.255.255.1:9", "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected ctx-canceled error, got nil")
	}
}

func TestHTTPRoundTrip_ConnectionRefused(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()
	_, _, err := HTTPRoundTrip(context.Background(), addr, "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected connection-refused error, got nil")
	}
}

func TestHTTPRoundTrip_BodyClosedAfterReturn(t *testing.T) {
	addr, cleanup := startEcho(t)
	defer cleanup()
	resp, _, err := HTTPRoundTrip(context.Background(), addr, "GET", "/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	more, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ReadAll on returned body: %v", err)
	}
	if len(more) != 0 {
		t.Errorf("expected drained body, got %d bytes", len(more))
	}
}

func TestHTTPRoundTrip_SetHeaders(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		buf := make([]byte, 4096)
		n, _ := c.Read(buf)
		req := string(buf[:n])
		body := req
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: " +
			strconv.Itoa(len(body)) + "\r\n\r\n" + body))
	}()
	hdr := http.Header{"X-Test": []string{"yes"}}
	_, body, err := HTTPRoundTrip(context.Background(), ln.Addr().String(), "POST", "/p", hdr, []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "X-Test: yes") {
		t.Errorf("X-Test header missing from request: %s", got)
	}
	if !strings.Contains(got, "payload") {
		t.Errorf("payload missing from request: %s", got)
	}
}
