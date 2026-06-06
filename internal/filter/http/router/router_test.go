package router

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/stats"
)

// loopbackHTTPEcho starts a tiny HTTP/1.1 echo server and returns its address
// + a stop function. The server reads one request, writes one response with
// body "echo:<URL.Path>", then closes. Used to exercise routerAction.
func loopbackHTTPEcho(t *testing.T) (string, func()) {
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

// singleEndpointCluster builds a *cluster.Cluster pointing at addr by going
// through cluster.NewManager with a minimal Bootstrap. Mirrors the
// mkClusterMgr helper in internal/listener/manager_test.go:59-93.
// singleEndpointClusterWithRegistry is the Task-11 variant that ALSO returns
// the Registry the Manager registered the cluster's 8 metrics on, so test
// code can read counter values across the package boundary (cluster fields
// are unexported).
func singleEndpointClusterWithRegistry(t *testing.T, addr string) (*cluster.Cluster, *stats.Registry) {
	t.Helper()
	c, reg := singleEndpointClusterAndReg(t, addr)
	return c, reg
}

func singleEndpointCluster(t *testing.T, addr string) *cluster.Cluster {
	t.Helper()
	c, _ := singleEndpointClusterAndReg(t, addr)
	return c
}

func singleEndpointClusterAndReg(t *testing.T, addr string) (*cluster.Cluster, *stats.Registry) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort %q: %v", addr, err)
	}
	port64, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("ParseUint %q: %v", portStr, err)
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 "c_test",
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: "c_test",
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       host,
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: uint32(port64)},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	reg := stats.NewRegistry()
	cm, err := cluster.NewManager(bs, reg)
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	c, ok := cm.Get("c_test")
	if !ok {
		t.Fatal("cluster.Manager.Get(c_test) returned !ok")
	}
	return c, reg
}

func TestRouterAction_DoHappy(t *testing.T) {
	addr, stop := loopbackHTTPEcho(t)
	defer stop()

	c := singleEndpointCluster(t, addr)
	a := &routerAction{cluster: c}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	// loopbackHTTPEcho writes `Connection: close` in its response, so the
	// router action correctly signals close via errCloseAfterAction (per
	// SPEC §5.3 / SPEC §10 #3 settled). Any other error is a real failure.
	if _, err := a.do(req.Context(), req, bw); err != nil && !errors.Is(err, errCloseAfterAction) {
		t.Fatalf("do: %v", err)
	}
	_ = bw.Flush()
	if !strings.Contains(buf.String(), "echo:/x") {
		t.Errorf("expected echo:/x in response, got: %q", buf.String())
	}
}

func TestRouterAction_DoDialFailureReturns503(t *testing.T) {
	// Cluster with an unreachable endpoint (port 1 is always rejected).
	c := singleEndpointCluster(t, "127.0.0.1:1")
	a := &routerAction{cluster: c}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := a.do(req.Context(), req, bw); err != nil {
		// dial-failure becomes a 503 LOCAL REPLY; do() should NOT error
		// (it writes the local reply and returns nil).
		if !errors.Is(err, errCloseAfterAction) {
			t.Errorf("dial failure should write 503 and return nil (or sentinel), got: %v", err)
		}
	}
	_ = bw.Flush()
	if !strings.HasPrefix(buf.String(), "HTTP/1.1 503 ") {
		t.Errorf("expected 503 local reply on dial failure, got: %q", buf.String())
	}
}

func TestRouterAction_DoCtxCancel(t *testing.T) {
	addr, stop := loopbackHTTPEcho(t)
	defer stop()

	c := singleEndpointCluster(t, addr)
	a := &routerAction{cluster: c}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before do — Cluster.Dial(ctx) should return ctx.Err()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := a.do(ctx, req, bw); err != nil {
		t.Errorf("ctx-cancel should map to 503 local reply, not propagate err: %v", err)
	}
	_ = bw.Flush()
	if !strings.HasPrefix(buf.String(), "HTTP/1.1 503 ") {
		t.Errorf("ctx cancel should produce 503 local reply, got: %q", buf.String())
	}
}

// counterValue walks the Registry and returns the Load() of the counter named
// `name`, or -1 if no counter by that name is registered. Used by the Task 11
// hot-path tests to read cluster-scope counters across the package boundary
// (cluster's metric fields are unexported).
func counterValue(t *testing.T, r *stats.Registry, name string) int64 {
	t.Helper()
	got := int64(-1)
	r.Walk(func(m stats.Metric) {
		if m.Name() == name {
			if c, ok := m.(*stats.Counter); ok {
				got = int64(c.Load())
			}
		}
	})
	return got
}

// TestRouterAction_Do_IncsUpstreamRqTotalAndStatusClass — Phase 06.1 Task 11
// hot path (H1 router): driving routerAction.do against a backend returning
// 200 Inc's c.upstreamRqTotal by 1 AND c.upstreamRq2xx by 1, per SPEC §5.5
// (Increment paths table, "routerAction.do (H1)" row).
func TestRouterAction_Do_IncsUpstreamRqTotalAndStatusClass(t *testing.T) {
	addr, stop := loopbackHTTPEcho(t)
	defer stop()

	c, reg := singleEndpointClusterWithRegistry(t, addr)
	a := &routerAction{cluster: c}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := a.do(req.Context(), req, bw); err != nil && !errors.Is(err, errCloseAfterAction) {
		t.Fatalf("do: %v", err)
	}
	_ = bw.Flush()

	if got := counterValue(t, reg, "cluster.c_test.upstream_rq_total"); got != 1 {
		t.Errorf("upstream_rq_total = %d, want 1", got)
	}
	if got := counterValue(t, reg, "cluster.c_test.upstream_rq_2xx"); got != 1 {
		t.Errorf("upstream_rq_2xx = %d, want 1", got)
	}
}

// TestRouterAction_Do_DialFailureInc5xx — Phase 06.1 Task 11: on a Dial
// failure path the action emits a 503 local reply; the cluster-scope
// status-class counter for the 5xx class Inc's once. This mirrors the
// "5xx Inc lands on the dial-failure local-reply path too" annotation in
// PLAN Task 11 Step 3.
func TestRouterAction_Do_DialFailureInc5xx(t *testing.T) {
	c, reg := singleEndpointClusterWithRegistry(t, "127.0.0.1:1") // unreachable
	a := &routerAction{cluster: c}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	_, _ = a.do(req.Context(), req, bw)
	_ = bw.Flush()

	if got := counterValue(t, reg, "cluster.c_test.upstream_rq_total"); got != 1 {
		t.Errorf("upstream_rq_total on dial-failure = %d, want 1", got)
	}
	if got := counterValue(t, reg, "cluster.c_test.upstream_rq_5xx"); got != 1 {
		t.Errorf("upstream_rq_5xx on 503 local-reply = %d, want 1", got)
	}
}

// emitCaptureSink is a minimal accesslog.Sink test double that records
// submitted records in-memory. Mirrors the hcm-package emitCaptureSink so
// the byte-preserved tests below compile in this package. Per BRAINSTORM
// §6.8: tests are byte-preserved; private helpers like this one are
// duplicated rather than exported across the package boundary.
type emitCaptureSink struct{ recs []*accesslog.Record }

func (s *emitCaptureSink) Submit(r any) { s.recs = append(s.recs, r.(*accesslog.Record)) }
func (s *emitCaptureSink) Close() error { return nil }

// TestRouterAction_EmitsAccessLog_HappyPath verifies that routerAction.do
// submits one access-log record with a non-empty UpstreamHost and BytesSent > 0
// when the upstream responds successfully.
func TestRouterAction_EmitsAccessLog_HappyPath(t *testing.T) {
	addr, stop := loopbackHTTPEcho(t)
	defer stop()

	cs := &emitCaptureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	c := singleEndpointCluster(t, addr)
	a := &routerAction{cluster: c, filter: f}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := a.do(req.Context(), req, bw); err != nil && !errors.Is(err, errCloseAfterAction) {
		t.Fatalf("do: %v", err)
	}
	_ = bw.Flush()

	if len(cs.recs) != 1 {
		t.Fatalf("captured %d records, want 1", len(cs.recs))
	}
	r := cs.recs[0]
	if r.ResponseCode != 200 {
		t.Errorf("ResponseCode = %d, want 200", r.ResponseCode)
	}
	if r.BytesSent <= 0 {
		t.Errorf("BytesSent = %d, want > 0", r.BytesSent)
	}
	if r.UpstreamHost == "" {
		t.Errorf("UpstreamHost is empty, want non-empty (routed request)")
	}
}

// TestRouterAction_EmitsAccessLog_DialFailure verifies that routerAction.do
// emits an access-log record with status 503 and an empty UpstreamHost on
// the dial-failure path (port 1 is always rejected).
func TestRouterAction_EmitsAccessLog_DialFailure(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	c := singleEndpointCluster(t, "127.0.0.1:1")
	a := &routerAction{cluster: c, filter: f}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	_, _ = a.do(req.Context(), req, bw)
	_ = bw.Flush()

	if len(cs.recs) != 1 {
		t.Fatalf("captured %d records, want 1", len(cs.recs))
	}
	r := cs.recs[0]
	if r.ResponseCode != 503 {
		t.Errorf("ResponseCode = %d, want 503 (dial-failure local-reply)", r.ResponseCode)
	}
	if r.UpstreamHost != "" {
		t.Errorf("UpstreamHost = %q, want empty on dial failure", r.UpstreamHost)
	}
}
