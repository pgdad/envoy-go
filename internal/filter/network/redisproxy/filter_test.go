package redisproxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/stats"
)

func validAny(t *testing.T) *anypb.Any {
	t.Helper()
	msg := &redis_proxyv3.RedisProxy{
		StatPrefix:   "rp",
		Settings:     &redis_proxyv3.RedisProxy_ConnPoolSettings{OpTimeout: durationpb.New(time.Second)},
		PrefixRoutes: &redis_proxyv3.RedisProxy_PrefixRoutes{CatchAllRoute: &redis_proxyv3.RedisProxy_PrefixRoutes_Route{Cluster: "rc"}},
	}
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func TestNewFactory_TypeURLReject(t *testing.T) {
	reg := stats.NewRegistry()
	f := NewFactory(nil, reg)
	bad := &anypb.Any{TypeUrl: "type.googleapis.com/wrong.Type"}
	if _, err := f(bad, network.FactoryCtx{}); err == nil {
		t.Fatal("NewFactory accepted a wrong type_url; want a reject")
	}
}

func TestNewFactory_ValidConfig(t *testing.T) {
	reg := stats.NewRegistry()
	f := NewFactory(nil, reg)
	fif, err := f(validAny(t), network.FactoryCtx{})
	if err != nil {
		t.Fatalf("NewFactory returned error for valid config: %v", err)
	}
	if fif == nil {
		t.Fatal("NewFactory returned nil FilterInstanceFactory for valid config")
	}
}

// newFilterForTest builds a *filter directly with an injected dial closure (the
// cluster.Manager path is exercised in the differential; the unit test injects a
// fake upstream so the pump logic is tested in isolation). The IMPL may expose a
// small package-internal seam (e.g. f.dialOverride) for this; the SPEC §3.7
// production path resolves cm.Get → Cluster.Dial.
//
// This test drives Handle over a net.Pipe downstream + a scripted upstream.

func TestHandle_PingLocalReply_NoUpstream(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	dialed := false
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) {
		dialed = true
		return nil, io.EOF
	})
	down, client := net.Pipe()
	go func() {
		// net.Pipe is unbuffered + synchronous: the client MUST drain the +PONG
		// reply (else Handle's downstream.Write blocks), then Close to deliver the
		// EOF that ends the pump. (Do NOT CloseWrite — net.Pipe conns don't
		// implement it; the assertion would panic.)
		_, _ = client.Write([]byte("PING\r\n"))
		buf := make([]byte, len("+PONG\r\n"))
		_, _ = io.ReadFull(client, buf)
		if string(buf) != "+PONG\r\n" {
			t.Errorf("PING reply = %q, want +PONG\\r\\n", buf)
		}
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	// PING is local → never dials.
	if dialed {
		t.Error("PING dialed upstream; want zero upstream (AMEND-R5)")
	}
	if rs.counters["downstream_cx_total"].Load() != 1 || rs.counters["downstream_rq_total"].Load() != 1 {
		t.Errorf("cx/rq totals = %d/%d, want 1/1", rs.counters["downstream_cx_total"].Load(), rs.counters["downstream_rq_total"].Load())
	}
}

func TestHandle_ProxiedRoundTrip_SetThenGet(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	// scripted upstream: SET → +OK, GET → $3\r\nbar\r\n (positional).
	upSrv, upClient := net.Pipe()
	go func() {
		br := bufio.NewReader(upSrv)
		// read SET (3-bulk array), reply +OK
		_, _, _, _ = decodeRequest(br)
		_, _ = upSrv.Write([]byte("+OK\r\n"))
		// read GET (2-bulk array), reply bulk bar
		_, _, _, _ = decodeRequest(br)
		_, _ = upSrv.Write([]byte("$3\r\nbar\r\n"))
	}()
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return upClient, nil })
	down, client := net.Pipe()
	got := make(chan []byte, 1)
	go func() {
		// net.Pipe is synchronous/unbuffered in BOTH directions: a Write blocks
		// until the reader drains it, so we MUST interleave writes with reads —
		// write SET, drain +OK, write GET, drain $3\r\nbar\r\n, then Close to
		// deliver the EOF that ends the pump.
		_, _ = client.Write([]byte("*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"))
		okBuf := make([]byte, len("+OK\r\n"))
		_, _ = io.ReadFull(client, okBuf)
		_, _ = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
		barBuf := make([]byte, len("$3\r\nbar\r\n"))
		_, _ = io.ReadFull(client, barBuf)
		_ = client.Close()
		got <- append(okBuf, barBuf...)
	}()
	f.Handle(context.Background(), down)
	if g := string(<-got); g != "+OK\r\n$3\r\nbar\r\n" {
		t.Errorf("downstream replies = %q, want +OK then $3 bar", g)
	}
	if rs.counters["downstream_rq_total"].Load() != 2 {
		t.Errorf("downstream_rq_total = %d, want 2", rs.counters["downstream_rq_total"].Load())
	}
}

func TestHandle_UnknownClusterGracefulClose(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	// a dial closure that always errors models cm.Get-miss → graceful close, no panic.
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.ErrClosedPipe })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n")) // a proxied command
		_ = client.Close()
	}()
	done := make(chan struct{})
	go func() { f.Handle(context.Background(), down); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Handle hung on an unresolvable upstream; want graceful close")
	}
}

func TestHandle_EOFCleanClose(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.EOF })
	down, client := net.Pipe()
	_ = client.Close() // immediate EOF, no request
	f.Handle(context.Background(), down)
	if rs.counters["downstream_cx_total"].Load() != 1 {
		t.Errorf("downstream_cx_total = %d, want 1 (connection counted even with no request)", rs.counters["downstream_cx_total"].Load())
	}
	if got := rs.counters["downstream_cx_protocol_error"].Load(); got != 0 {
		t.Errorf("downstream_cx_protocol_error = %d, want 0 (a clean EOF is not a protocol error)", got)
	}
}

func newTestFilter(rs *redisStats, dial network.UpstreamDialFunc) *filter {
	return &filter{
		cfg: &compiledConfig{statPrefix: "rp", catchAllCluster: "rc"},
		st:  rs,
		dialSource: func(context.Context) (network.UpstreamDialFunc, func(), error) {
			return dial, nil, nil
		},
	}
}

// TestHandle_UnknownCommand: BOGUSCMD → splitter.unsupported_command +1, -ERR
// unknown command reply, zero upstream.
func TestHandle_UnknownCommand(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	dialed := false
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { dialed = true; return nil, io.EOF })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("*2\r\n$8\r\nBOGUSCMD\r\n$1\r\nx\r\n"))
		buf := make([]byte, 256)
		n, _ := client.Read(buf)
		if !strings.HasPrefix(string(buf[:n]), "-ERR unknown command 'BOGUSCMD'") {
			t.Errorf("reply = %q, want -ERR unknown command 'BOGUSCMD'…", buf[:n])
		}
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	if dialed {
		t.Error("unknown command dialed upstream; want zero upstream")
	}
	if rs.counters["splitter.unsupported_command"].Load() != 1 {
		t.Errorf("splitter.unsupported_command = %d, want 1", rs.counters["splitter.unsupported_command"].Load())
	}
}

// TestHandle_EchoWrongArity: ECHO (arity 1) → splitter.invalid_request +1.
func TestHandle_EchoWrongArity(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.EOF })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("*1\r\n$4\r\nECHO\r\n"))
		buf := make([]byte, 64)
		n, _ := client.Read(buf)
		if string(buf[:n]) != "-invalid request\r\n" {
			t.Errorf("reply = %q, want -invalid request", buf[:n])
		}
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	if rs.counters["splitter.invalid_request"].Load() != 1 {
		t.Errorf("splitter.invalid_request = %d, want 1", rs.counters["splitter.invalid_request"].Load())
	}
}

// TestHandle_SuccessOnErrorReply: a backend -ERR reply counts command.get.success
// (the round-trip completed — §4.2), NOT error.
func TestHandle_SuccessOnErrorReply(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	upSrv, upClient := net.Pipe()
	go func() {
		br := bufio.NewReader(upSrv)
		_, _, _, _ = decodeRequest(br)
		_, _ = upSrv.Write([]byte("-ERR backend boom\r\n"))
	}()
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return upClient, nil })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
		buf := make([]byte, 64)
		_, _ = client.Read(buf)
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	if rs.command("get", "success").Load() != 1 || rs.command("get", "error").Load() != 0 {
		t.Errorf("GET success/error = %d/%d, want 1/0 (a -ERR reply is success — round-trip completed)",
			rs.command("get", "success").Load(), rs.command("get", "error").Load())
	}
	if rs.command("get", "total").Load() != 1 {
		t.Errorf("GET total = %d, want 1", rs.command("get", "total").Load())
	}
}

// TestHandle_ErrorOnTransportFailure: a dial/Send failure counts command.get.error.
func TestHandle_ErrorOnTransportFailure(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.ErrClosedPipe })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	// total Incs at dispatch (before the failed dial); error Incs on the transport failure.
	if rs.command("get", "total").Load() != 1 || rs.command("get", "error").Load() != 1 {
		t.Errorf("GET total/error = %d/%d, want 1/1 (dispatch then transport failure)",
			rs.command("get", "total").Load(), rs.command("get", "error").Load())
	}
}

// TestHandle_QuitCloses: QUIT → +OK then the pump returns (connection closes).
func TestHandle_QuitCloses(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.EOF })
	down, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		_, _ = client.Write([]byte("*1\r\n$4\r\nQUIT\r\n"))
		buf := make([]byte, len("+OK\r\n"))
		_, _ = io.ReadFull(client, buf)
		if string(buf) != "+OK\r\n" {
			t.Errorf("QUIT reply = %q, want +OK", buf)
		}
		// QUIT must close downstream → the client's next read errors (peer closed).
		if _, err := client.Read(make([]byte, 1)); err == nil {
			t.Error("connection still open after QUIT; want closed (read should error)")
		}
		// Do NOT client.Close() — QUIT itself must end the pump (downstream.Close()).
		close(done)
	}()
	go f.Handle(context.Background(), down)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("QUIT did not close the connection")
	}
}

// TestHandle_ProtocolErrorIncrements: a malformed frame → downstream_cx_protocol_error.
func TestHandle_ProtocolErrorIncrements(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.EOF })
	down, client := net.Pipe()
	go func() {
		_, _ = client.Write([]byte("+bad\r\n")) // reply-type byte in request position → errProtocol
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	if rs.counters["downstream_cx_protocol_error"].Load() != 1 {
		t.Errorf("downstream_cx_protocol_error = %d, want 1", rs.counters["downstream_cx_protocol_error"].Load())
	}
}

// TestHandle_CxActiveGauge: cx_active is 1 during Handle, 0 after.
func TestHandle_CxActiveGauge(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	f := newTestFilter(rs, func(context.Context) (net.Conn, error) { return nil, io.EOF })
	down, client := net.Pipe()
	mid := make(chan int64, 1)
	go func() {
		_, _ = client.Write([]byte("PING\r\n"))
		buf := make([]byte, len("+PONG\r\n"))
		_, _ = io.ReadFull(client, buf)
		mid <- rs.gauges["downstream_cx_active"].Load() // sampled while Handle is live
		_ = client.Close()
	}()
	f.Handle(context.Background(), down)
	if got := <-mid; got != 1 {
		t.Errorf("downstream_cx_active during Handle = %d, want 1", got)
	}
	if got := rs.gauges["downstream_cx_active"].Load(); got != 0 {
		t.Errorf("downstream_cx_active after Handle = %d, want 0", got)
	}
	if got := rs.gauges["downstream_rq_active"].Load(); got != 0 {
		t.Errorf("downstream_rq_active after Handle = %d, want 0 (defer-balanced)", got)
	}
}
