package hcm

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// connPair returns a connected pair of net.Conn, both ends in-process.
func connPair(t *testing.T) (clientSide, serverSide net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	type result struct {
		c   net.Conn
		err error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := ln.Accept()
		ch <- result{c, err}
	}()
	c1, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	return c1, r.c
}

func writeRequest(t *testing.T, w io.Writer, method, path string, headers ...string) {
	t.Helper()
	hdr := "Host: example\r\n"
	for _, h := range headers {
		hdr += h + "\r\n"
	}
	_, err := io.WriteString(w, method+" "+path+" HTTP/1.1\r\n"+hdr+"Content-Length: 0\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
}

func readResponseStatus(t *testing.T, r io.Reader) int {
	t.Helper()
	resp, err := http.ReadResponse(bufio.NewReader(r), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestRunConnection_DirectResponseHappy(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, body: "OK\n"}},
	}}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()

	go runConnection(context.Background(), server, tt)

	writeRequest(t, client, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Errorf("status: got %d, want 200", got)
	}
}

func TestRunConnection_KeepAliveTwoRequests(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, body: "OK\n"}},
	}}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, tt)

	writeRequest(t, client, "GET", "/health")
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("first status: got %d, want 200", got)
	}
	writeRequest(t, client, "GET", "/health", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("second status: got %d, want 200", got)
	}
}

func TestRunConnection_RouteNotFoundReturns404(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, body: "OK\n"}},
	}}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, tt)

	writeRequest(t, client, "GET", "/missing", "Connection: close")
	if got := readResponseStatus(t, client); got != 404 {
		t.Errorf("status: got %d, want 404", got)
	}
}

func TestRunConnection_ExpectHeaderReturns417(t *testing.T) {
	tt := &routeTable{}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, tt)

	writeRequest(t, client, "GET", "/x", "Expect: 100-continue", "Connection: close")
	if got := readResponseStatus(t, client); got != 417 {
		t.Errorf("status: got %d, want 417", got)
	}
}

func TestRunConnection_UpgradeReturns501(t *testing.T) {
	tt := &routeTable{}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, tt)

	writeRequest(t, client, "GET", "/x", "Upgrade: websocket", "Connection: Upgrade")
	if got := readResponseStatus(t, client); got != 501 {
		t.Errorf("status: got %d, want 501", got)
	}
}

func TestRunConnection_BadRequestReturns400(t *testing.T) {
	tt := &routeTable{}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, tt)

	if _, err := io.WriteString(client, "GARBAGE\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if got := readResponseStatus(t, client); got != 400 {
		t.Errorf("status: got %d, want 400", got)
	}
}

// loopbackHTTPCloseEcho is a tiny HTTP/1.1 server that accepts upstream
// connections one at a time, reads one request, and writes a 200 response
// carrying `Connection: close`. The server returns its address and a stop
// function. Used to verify that routerAction.do propagates the upstream's
// Connection: close back to the connection loop via errCloseAfterAction
// (REVIEW.md I-1 from REVIEW.md 04527eb; SPEC §5.3).
func loopbackHTTPCloseEcho(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				body := "echo:" + req.URL.Path
				resp := fmt.Sprintf(
					"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
					len(body), body,
				)
				_, _ = c.Write([]byte(resp))
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close(); <-done }
}

// TestRunConnection_UpstreamConnectionCloseClosesDownstream is the I-1
// regression test: when the upstream's response carries `Connection: close`
// and the downstream request did NOT, runConnection must close the
// downstream after delivering that response (per SPEC §5.3 — "also break if
// the action's response carried Connection: close"). Pre-fix the connection
// loops back to read another request; post-fix it returns errCloseAfterAction
// from routerAction.do and the connection loop exits.
func TestRunConnection_UpstreamConnectionCloseClosesDownstream(t *testing.T) {
	addr, stop := loopbackHTTPCloseEcho(t)
	defer stop()

	c := singleEndpointCluster(t, addr)
	tt := &routeTable{routes: []routeEntry{
		{match: matchPrefix("/"), action: &routerAction{cluster: c}},
	}}

	client, server := connPair(t)
	defer func() { _ = client.Close() }()

	loopDone := make(chan struct{})
	go func() {
		runConnection(context.Background(), server, tt)
		close(loopDone)
	}()

	// Send TWO HTTP/1.1 requests back-to-back without Connection: close on
	// either. Pre-fix: both round-trips succeed (loop re-reads after each
	// upstream's Connection: close is silently ignored downstream). Post-fix:
	// only the first round-trip produces a response; the second request's
	// bytes are dropped on the floor when the loop closes.
	writeRequest(t, client, "GET", "/x")
	writeRequest(t, client, "GET", "/y")

	// Read the first response — must be 200.
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("first response status: got %d, want 200", got)
	}

	// Now bound the read for what comes next so we don't hang. We want to
	// see EOF (or a use-of-closed-connection style error) — NOT a second
	// "HTTP/1.1 200 OK" status line, which is what pre-fix code produced.
	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	n, err := client.Read(buf)
	if err == nil {
		// Got more bytes — that means the loop did NOT close after the
		// upstream's Connection: close. Read the rest and surface what we got.
		extra, _ := io.ReadAll(client)
		t.Errorf("expected EOF after first response (downstream should close per upstream Connection: close); got byte %q + %q",
			string(buf[:n]), string(extra))
	}

	// Confirm the connection loop returned (closed downstream).
	select {
	case <-loopDone:
		// good — runConnection returned
	case <-time.After(2 * time.Second):
		t.Error("runConnection did not return; expected close after upstream Connection: close")
	}
}

func TestRunConnection_BodyDrainedBetweenRequests(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/post"), action: &directResponseAction{status: 200, body: "ok\n"}},
	}}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, tt)

	body := strings.Repeat("x", 64)
	if _, err := io.WriteString(client,
		"POST /post HTTP/1.1\r\nHost: example\r\nContent-Length: "+strconv.Itoa(len(body))+"\r\n\r\n"+body); err != nil {
		t.Fatal(err)
	}
	if got := readResponseStatus(t, client); got != 200 {
		t.Fatalf("first status: got %d, want 200", got)
	}
	writeRequest(t, client, "GET", "/post", "Connection: close")
	if got := readResponseStatus(t, client); got != 200 {
		t.Errorf("second status (post-drain): got %d, want 200", got)
	}
}
