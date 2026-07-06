package router

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/bits"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/stats"
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
	return singleEndpointClusterCB(t, addr, nil)
}

// singleEndpointClusterCB is the circuit-breaker-configurable variant of
// singleEndpointClusterAndReg: when cb is non-nil it is attached to the static
// cluster's circuit_breakers field so the built *cluster.Cluster carries a live
// max_requests budget (Phase 41 / ADR-0248). cb==nil reproduces the original
// nil-circuitBreaker cluster (TryAcquireRequest is a no-op true).
func singleEndpointClusterCB(t *testing.T, addr string, cb *clusterv3.CircuitBreakers) (*cluster.Cluster, *stats.Registry) {
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
				CircuitBreakers:      cb,
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

// TestApplyHashKey pins the per-request hash_policy fold (SPEC §3.2 / Envoy
// HashPolicyImpl::generateHash): rotl64(prev,1)^new fold, first contributor
// verbatim, nullopt policies skipped, terminal short-circuits once the
// accumulator is non-empty.
func TestApplyHashKey(t *testing.T) {
	hv := func(m map[string]string) func(string) (string, bool) {
		return func(n string) (string, bool) { v, ok := m[n]; return v, ok }
	}
	hdr := func(name string) HashPolicy { return HashPolicy{Kind: HashKindHeader, HeaderName: name} }
	src := HashPolicy{Kind: HashKindSourceIP}

	// 1. single header → cluster.HashHeaderValues([]string{value}) stuffed.
	_, key, ok := applyHashKey(context.Background(), []HashPolicy{hdr("x-hash")}, hv(map[string]string{"x-hash": "alpha"}), "")
	if !ok || key != cluster.HashHeaderValues([]string{"alpha"}) {
		t.Fatalf("header key: ok=%v key=%#x", ok, key)
	}
	// 2. absent header → no contribution → has=false.
	if _, _, ok := applyHashKey(context.Background(), []HashPolicy{hdr("x-hash")}, hv(nil), ""); ok {
		t.Fatal("absent header must leave ctx keyless")
	}
	// 3. two policies fold rotl64(prev,1)^new, first verbatim.
	_, got, _ := applyHashKey(context.Background(), []HashPolicy{hdr("x-hash"), src},
		hv(map[string]string{"x-hash": "alpha"}), "10.0.0.7:5555")
	first := cluster.HashHeaderValues([]string{"alpha"})
	want := bits.RotateLeft64(first, 1) ^ cluster.HashSourceIP("10.0.0.7:5555")
	if got != want {
		t.Fatalf("fold: got %#x want %#x", got, want)
	}
	// 4. terminal short-circuits: header(terminal)+src → only header contributes.
	ht := HashPolicy{Kind: HashKindHeader, HeaderName: "x-hash", Terminal: true}
	_, got2, _ := applyHashKey(context.Background(), []HashPolicy{ht, src},
		hv(map[string]string{"x-hash": "alpha"}), "10.0.0.7:5555")
	if got2 != first {
		t.Fatalf("terminal: got %#x want %#x (header only)", got2, first)
	}
	// 5. terminal on a nullopt policy does NOT short-circuit (acc still empty).
	htAbsent := HashPolicy{Kind: HashKindHeader, HeaderName: "absent", Terminal: true}
	_, got3, _ := applyHashKey(context.Background(), []HashPolicy{htAbsent, src},
		hv(nil), "10.0.0.7:5555")
	if got3 != cluster.HashSourceIP("10.0.0.7:5555") {
		t.Fatalf("terminal-on-nullopt must not short-circuit; got %#x", got3)
	}
	// 6. no policies → has=false.
	if _, _, ok := applyHashKey(context.Background(), nil, hv(nil), ""); ok {
		t.Fatal("no policy → keyless")
	}
	// 7. on contribution, the returned ctx is a NEW ctx (cluster.WithHashKey
	//    ran) — non-vacuous proof the key rides the ctx. hashKeyFrom is
	//    unexported by internal/cluster (no cross-package witness), so we assert
	//    the ctx identity changed; Task 5's dispatch differential is the
	//    end-to-end proof the LB actually consumes the carried key.
	base := context.Background()
	ctx, _, ok := applyHashKey(base, []HashPolicy{hdr("x-hash")}, hv(map[string]string{"x-hash": "alpha"}), "")
	if !ok || ctx == base {
		t.Fatalf("contributing fold must return a new key-carrying ctx: ok=%v ctxChanged=%v", ok, ctx != base)
	}
	// keyless fold returns the input ctx unchanged (no-hash LB fallback).
	if ctx0, _, ok := applyHashKey(base, nil, hv(nil), ""); ok || ctx0 != base {
		t.Fatalf("keyless fold must return ctx unchanged: ok=%v ctxChanged=%v", ok, ctx0 != base)
	}
}

// blockingHTTPBackend starts a TCP listener that accepts connections, reads the
// inbound HTTP/1.1 request, then BLOCKS the response until release is closed.
// After release fires it answers every held (and subsequent) request with a
// minimal 200. Returns the addr + a release func + a stop func. Used by the
// circuit-breaker admission test to hold a max_requests slot open across a
// concurrent admission.
func blockingHTTPBackend(t *testing.T) (addr string, release func(), stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gate := make(chan struct{})
	var once sync.Once
	done := make(chan struct{})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				// Drain the request line + headers (one read is enough for the
				// tiny GET the test sends); we only need to keep the slot held.
				buf := make([]byte, 4096)
				_, _ = c.Read(buf)
				<-gate // hold the request → the max_requests slot stays acquired
				_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"))
				_ = c.Close()
			}(conn)
		}
	}()
	return ln.Addr().String(),
		func() { once.Do(func() { close(gate) }) },
		func() { _ = ln.Close(); close(done) }
}

// TestDoH1ClusterAction_CircuitBreakerOverflow503 — Phase 41 Task 6 (ADR-0248).
// With circuit_breakers{max_requests:1} a single in-flight upstream request
// holds the only DEFAULT-priority slot; a concurrent doH1ClusterAction admission
// overflows and returns a 503 ActionResponse WITHOUT acquiring (so it must not
// release). Once the held request completes and releases its slot, a fresh
// admission succeeds — proving every exit path releases exactly once (no
// double-release, no leak).
func TestDoH1ClusterAction_CircuitBreakerOverflow503(t *testing.T) {
	addr, release, stop := blockingHTTPBackend(t)
	defer stop()

	cb := &clusterv3.CircuitBreakers{
		Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
			{Priority: corev3.RoutingPriority_DEFAULT, MaxRequests: wrapperspb.UInt32(1)},
		},
	}
	c, reg := singleEndpointClusterCB(t, addr, cb)
	a := &routerAction{cluster: c}

	mkReq := func() *http.Request {
		req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
		req.URL.Path = "/x"
		return req
	}

	// 1) Fire the slot-holding request in the background. It acquires the only
	//    max_requests slot and blocks inside the backend until release().
	heldDone := make(chan ActionResponse, 1)
	go func() {
		resp, _, _ := doH1ClusterAction(context.Background(), a, mkReq())
		heldDone <- resp
	}()

	// Wait until the held request has actually acquired its slot (rq_open gauge
	// flips to 1) before probing the overflow — avoids racing the goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if gaugeIsOne(t, reg, "cluster.c_test.circuit_breakers.default.rq_open") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("held request never acquired the max_requests slot (rq_open stayed 0)")
		}
		time.Sleep(time.Millisecond)
	}

	// 2) A concurrent admission overflows → 503, picked is the zero Endpoint,
	//    and it must NOT have acquired a slot (so it must not release one).
	resp, picked, err := doH1ClusterAction(context.Background(), a, mkReq())
	if err != nil {
		t.Fatalf("overflow admission returned err: %v", err)
	}
	if resp.Status != 503 {
		t.Fatalf("overflow admission Status = %d, want 503", resp.Status)
	}
	if !picked.IsZero() {
		t.Fatalf("overflow admission picked a host (%v), want the zero Endpoint", picked)
	}
	if got := counterValue(t, reg, "cluster.c_test.upstream_rq_pending_overflow"); got != 1 {
		t.Fatalf("upstream_rq_pending_overflow = %d, want 1", got)
	}

	// 3) Release the held request; its deferred ReleaseRequest fires, clearing
	//    the slot. A fresh admission must now succeed (proves clean release; the
	//    overflow path did not erroneously release a slot it never held).
	release()
	// The held doH1ClusterAction returns only AFTER its deferred ReleaseRequest
	// has run, so receiving on heldDone guarantees the slot is freed.
	if held := <-heldDone; held.Status != 200 {
		t.Fatalf("held request final Status = %d, want 200", held.Status)
	}
	if gaugeIsOne(t, reg, "cluster.c_test.circuit_breakers.default.rq_open") {
		t.Fatal("rq_open still 1 after the held request released its slot")
	}
	resp3, _, err := doH1ClusterAction(context.Background(), a, mkReq())
	if err != nil {
		t.Fatalf("post-release admission returned err: %v", err)
	}
	if resp3.Status != 200 {
		t.Fatalf("post-release admission Status = %d, want 200 (slot freed)", resp3.Status)
	}
	// Exactly one overflow ever — no double-count, no spurious second overflow.
	if got := counterValue(t, reg, "cluster.c_test.upstream_rq_pending_overflow"); got != 1 {
		t.Fatalf("upstream_rq_pending_overflow after release = %d, want 1 (no double-release/leak)", got)
	}
}

// TestDoH1ClusterAction_ConnPoolOverflow503 — Phase 43.1 Task 7 (ADR-0252).
// With circuit_breakers{max_connections:1, max_pending_requests:0} a single
// in-flight upstream request holds the only connection permit; a concurrent
// doH1ClusterAction finds the cap reached AND the wait-queue full ⇒ AcquireH1
// returns the errConnPoolOverflow sentinel ⇒ the action fails fast with a 503.
//
// The new overflow branch (router.go) must:
//   - return Status:503 (load-shed), NOT the 502 dial shape,
//   - NOT set localOrigin (an overflow is a load-shed, not a connect failure —
//     so a connect-failure retry_on must NOT match it),
//   - NOT call IncStatusClass (the dedicated upstream_rq_pending_overflow
//     counter — already incremented inside the pool — is the signal; the
//     router must not double-count onto upstream_rq_5xx),
//   - NOT call RecordUpstreamResult (no host was engaged for the shed request).
//
// This is the REAL-overflow approach (no seam/fake cluster): a blocking backend
// holds the single permit and a concurrent admission drives a genuine
// errConnPoolOverflow through AcquireH1, exactly as the live HCM path will.
func TestDoH1ClusterAction_ConnPoolOverflow503(t *testing.T) {
	addr, release, stop := blockingHTTPBackend(t)
	defer stop()

	cb := &clusterv3.CircuitBreakers{
		Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
			{
				Priority:           corev3.RoutingPriority_DEFAULT,
				MaxConnections:     wrapperspb.UInt32(1),
				MaxPendingRequests: wrapperspb.UInt32(0),
			},
		},
	}
	c, reg := singleEndpointClusterCB(t, addr, cb)
	a := &routerAction{cluster: c}

	mkReq := func() *http.Request {
		req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
		req.URL.Path = "/x"
		return req
	}

	// 1) Fire the permit-holding request in the background. It engages the only
	//    max_connections permit (cx_open flips to 1) and blocks in the backend.
	heldDone := make(chan ActionResponse, 1)
	go func() {
		resp, _, _ := doH1ClusterAction(context.Background(), a, mkReq())
		heldDone <- resp
	}()

	// Wait until the held request has actually engaged its permit (cx_open gauge
	// flips to 1) before probing the overflow — avoids racing the goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if gaugeIsOne(t, reg, "cluster.c_test.circuit_breakers.default.cx_open") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("held request never engaged the max_connections permit (cx_open stayed 0)")
		}
		time.Sleep(time.Millisecond)
	}

	// 2) A concurrent admission finds the cap reached + the queue full ⇒
	//    errConnPoolOverflow ⇒ 503, NOT localOrigin, picked is the zero Endpoint.
	resp, picked, err := doH1ClusterAction(context.Background(), a, mkReq())
	if err != nil {
		t.Fatalf("overflow admission returned err: %v", err)
	}
	if resp.Status != 503 {
		t.Fatalf("overflow admission Status = %d, want 503", resp.Status)
	}
	if resp.localOrigin {
		t.Fatalf("overflow admission localOrigin = true, want false (a load-shed is not a connect failure)")
	}
	if !picked.IsZero() {
		t.Fatalf("overflow admission picked a host (%v), want the zero Endpoint", picked)
	}
	// The pool already incremented upstream_rq_pending_overflow; the router must
	// not double-count by also bumping the upstream 5xx class.
	if got := counterValue(t, reg, "cluster.c_test.upstream_rq_pending_overflow"); got != 1 {
		t.Fatalf("upstream_rq_pending_overflow = %d, want 1", got)
	}
	if got := counterValue(t, reg, "cluster.c_test.upstream_rq_5xx"); got > 0 {
		t.Fatalf("upstream_rq_5xx = %d, want 0 (overflow must NOT IncStatusClass)", got)
	}

	// 3) Release the held request; a fresh admission must now succeed (proves the
	//    overflow path did not leak or wrongly release a permit it never held).
	release()
	// The held doH1ClusterAction returns only AFTER its deferred conn-close has
	// released the permit, so receiving on heldDone guarantees the permit is
	// freed and cx_open must have cleared back to 0 (mirrors the CB analogue's
	// rq_open post-release check).
	if held := <-heldDone; held.Status != 200 {
		t.Fatalf("held request final Status = %d, want 200", held.Status)
	}
	if gaugeIsOne(t, reg, "cluster.c_test.circuit_breakers.default.cx_open") {
		t.Fatal("cx_open still 1 after the held request released its permit")
	}
	resp3, _, err := doH1ClusterAction(context.Background(), a, mkReq())
	if err != nil {
		t.Fatalf("post-release admission returned err: %v", err)
	}
	if resp3.Status != 200 {
		t.Fatalf("post-release admission Status = %d, want 200 (permit freed)", resp3.Status)
	}
	if got := counterValue(t, reg, "cluster.c_test.upstream_rq_pending_overflow"); got != 1 {
		t.Fatalf("upstream_rq_pending_overflow after release = %d, want 1 (no double-count)", got)
	}
}

// gaugeIsOne reports whether the gauge named `name` in registry r currently
// reads exactly 1. Returns false if no such gauge is registered.
func gaugeIsOne(t *testing.T, r *stats.Registry, name string) bool {
	t.Helper()
	one := false
	r.Walk(func(m stats.Metric) {
		if m.Name() != name {
			return
		}
		if g, ok := m.(*stats.Gauge); ok {
			one = g.Load() == 1
		}
	})
	return one
}

// TestWithDownstreamRemoteAddr round-trips the ctx-carried downstream remote
// address (set by HCM dispatch; consumed by the source_ip hash_policy producer).
func TestWithDownstreamRemoteAddr(t *testing.T) {
	if got := downstreamRemoteAddrFrom(WithDownstreamRemoteAddr(context.Background(), "1.2.3.4:5")); got != "1.2.3.4:5" {
		t.Fatalf("round-trip: got %q want %q", got, "1.2.3.4:5")
	}
	if got := downstreamRemoteAddrFrom(context.Background()); got != "" {
		t.Fatalf("unset: got %q want empty", got)
	}
}
