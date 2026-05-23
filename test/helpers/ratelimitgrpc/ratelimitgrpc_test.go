package ratelimitgrpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	commonratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

// dialTestClient opens a plaintext h2c grpc.ClientConn to the supplied
// address, registers it for teardown via t.Cleanup, and returns the
// RateLimitServiceClient.
func dialTestClient(t *testing.T, addr string) ratelimitv3.RateLimitServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q): %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return ratelimitv3.NewRateLimitServiceClient(conn)
}

// mkRequest builds a *ratelimitv3.RateLimitRequest with the supplied
// domain + one descriptor whose entries are the supplied key=value pairs
// (passed as interleaved strings: key1, value1, key2, value2, ...).
func mkRequest(domain string, kvs ...string) *ratelimitv3.RateLimitRequest {
	if len(kvs)%2 != 0 {
		panic("mkRequest: kvs must be interleaved key/value pairs")
	}
	entries := make([]*commonratelimitv3.RateLimitDescriptor_Entry, 0, len(kvs)/2)
	for i := 0; i < len(kvs); i += 2 {
		entries = append(entries, &commonratelimitv3.RateLimitDescriptor_Entry{
			Key:   kvs[i],
			Value: kvs[i+1],
		})
	}
	return &ratelimitv3.RateLimitRequest{
		Domain: domain,
		Descriptors: []*commonratelimitv3.RateLimitDescriptor{
			{Entries: entries},
		},
	}
}

// TestNew_StartsServerOnEphemeralPort verifies that New binds to an
// ephemeral 127.0.0.1 port and Addr() returns the bound `host:port`.
func TestNew_StartsServerOnEphemeralPort(t *testing.T) {
	srv := New(t)
	addr := srv.Addr()
	if addr == "" {
		t.Fatal("Addr: empty after New")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	if host != "127.0.0.1" {
		t.Errorf("host: got %q, want %q", host, "127.0.0.1")
	}
	if port == "0" {
		t.Errorf("port: got %q, want non-zero ephemeral", port)
	}
}

// TestNewAtAddr_BindsToSuppliedAddress verifies that NewAtAddr binds the
// gRPC server to the caller-supplied address (allocated upstream via the
// Listen+Close + rebind idiom used by fixture drivers). A scripted
// ShouldRateLimit round-trip confirms the rebound server is serving.
func TestNewAtAddr_BindsToSuppliedAddress(t *testing.T) {
	// Allocate a free port via Listen+Close (the same idiom the fixture
	// driver uses to pin a stable rls-cluster endpoint before bootstrap
	// YAMLs render).
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	wantAddr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("probe close: %v", err)
	}

	srv, err := NewAtAddr(wantAddr)
	if err != nil {
		t.Fatalf("NewAtAddr(%q): %v", wantAddr, err)
	}
	t.Cleanup(srv.Stop)

	if got := srv.Addr(); got != wantAddr {
		t.Errorf("Addr: got %q, want %q (rebound to a different port)", got, wantAddr)
	}

	// Sanity: the rebound server actually serves ShouldRateLimit round-trips.
	req := mkRequest("envoy", "generic_key", "ping")
	wantResp := &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OK,
		Statuses: []*ratelimitv3.RateLimitResponse_DescriptorStatus{
			{Code: ratelimitv3.RateLimitResponse_OK},
		},
	}
	srv.Script(CanonicalKey(req), wantResp)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := client.ShouldRateLimit(ctx, req)
	if err != nil {
		t.Fatalf("ShouldRateLimit: %v", err)
	}
	if resp.GetOverallCode() != ratelimitv3.RateLimitResponse_OK {
		t.Fatalf("OverallCode: got %v, want OK", resp.GetOverallCode())
	}
}

// TestNewAtAddr_BindFailureReturnsError verifies that NewAtAddr returns a
// non-nil error when net.Listen fails (here: re-binding the same in-use
// port). Drivers rely on this error path to surface bind failures cleanly.
func TestNewAtAddr_BindFailureReturnsError(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold listen: %v", err)
	}
	defer func() { _ = held.Close() }()

	srv, err := NewAtAddr(held.Addr().String())
	if err == nil {
		// Bind unexpectedly succeeded — clean up the unwanted server.
		srv.Stop()
		t.Fatalf("NewAtAddr(%q): want bind error, got nil", held.Addr().String())
	}
	if srv != nil {
		t.Errorf("NewAtAddr(%q): want nil *Server on error, got %v", held.Addr().String(), srv)
	}
}

// TestServer_Script_ReturnsScripted verifies that a registered Script is
// returned at ShouldRateLimit by canonical descriptor-list key, and that
// an unregistered key returns the default OK response (NOT an error) so
// unscripted scenarios pass through cleanly.
func TestServer_Script_ReturnsScripted(t *testing.T) {
	srv := New(t)

	// OVER_LIMIT scripted on the "user=alice" descriptor.
	denyReq := mkRequest("envoy", "user", "alice")
	denyResp := &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OVER_LIMIT,
		Statuses: []*ratelimitv3.RateLimitResponse_DescriptorStatus{
			{Code: ratelimitv3.RateLimitResponse_OVER_LIMIT},
		},
	}
	srv.Script(CanonicalKey(denyReq), denyResp)

	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Scripted OVER_LIMIT.
	got, err := client.ShouldRateLimit(ctx, denyReq)
	if err != nil {
		t.Fatalf("ShouldRateLimit(denyReq): %v", err)
	}
	if got.GetOverallCode() != ratelimitv3.RateLimitResponse_OVER_LIMIT {
		t.Errorf("scripted OverallCode: got %v, want OVER_LIMIT", got.GetOverallCode())
	}
	if len(got.GetStatuses()) != 1 || got.GetStatuses()[0].GetCode() != ratelimitv3.RateLimitResponse_OVER_LIMIT {
		t.Errorf("scripted Statuses: %+v; want one OVER_LIMIT entry", got.GetStatuses())
	}

	// Unscripted key — default OK + per-descriptor OK statuses.
	missReq := mkRequest("envoy", "user", "bob")
	got, err = client.ShouldRateLimit(ctx, missReq)
	if err != nil {
		t.Fatalf("ShouldRateLimit(missReq): %v", err)
	}
	if got.GetOverallCode() != ratelimitv3.RateLimitResponse_OK {
		t.Errorf("default OverallCode: got %v, want OK", got.GetOverallCode())
	}
	if len(got.GetStatuses()) != 1 || got.GetStatuses()[0].GetCode() != ratelimitv3.RateLimitResponse_OK {
		t.Errorf("default Statuses: %+v; want one OK entry", got.GetStatuses())
	}
}

// TestServer_AMEND6_ProtoNumberFaithful_UnsetOptionalsOmitted asserts the
// D-RL5 / AMEND-6 wire-byte discipline: a RateLimitResponse constructed
// with ONLY OverallCode + per-descriptor Code set produces wire bytes
// identical to the same response when its optional fields are explicitly
// left as zero-value / nil. Cross-side byte-exactness depends on the fake
// emitting deterministic, minimal bytes — Go-protobuf elides zero-value
// scalars + nil-pointer messages by default, so this test pins that
// invariant against accidental future regressions (e.g., someone wiring
// a default RawBody / DynamicMetadata / Quota into the fake's defaults).
//
// The test marshals the response the fake's no-match path actually
// returns + asserts proto.Equal against a "minimal" reference that sets
// only OverallCode + Statuses[i].Code — proving the fake's default path
// holds the AMEND-6 contract.
func TestServer_AMEND6_ProtoNumberFaithful_UnsetOptionalsOmitted(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := mkRequest("envoy", "generic_key", "vacuous")
	got, err := client.ShouldRateLimit(ctx, req)
	if err != nil {
		t.Fatalf("ShouldRateLimit: %v", err)
	}

	// Reference: ONLY OverallCode + per-descriptor Code. All other
	// optionals (ResponseHeadersToAdd / RequestHeadersToAdd / RawBody /
	// DynamicMetadata / Quota; per-descriptor CurrentLimit /
	// LimitRemaining / DurationUntilReset / Quota) left zero-value /
	// nil. proto.Equal compares parsed field presence, not raw bytes,
	// so this asserts the fake did NOT set any optional field.
	want := &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OK,
		Statuses: []*ratelimitv3.RateLimitResponse_DescriptorStatus{
			{Code: ratelimitv3.RateLimitResponse_OK},
		},
	}
	if !proto.Equal(got, want) {
		t.Errorf("AMEND-6 default response shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}

	// Belt-and-braces: assert the optional fields are zero-value / nil
	// at the Go-struct level (catches accidental zero-but-set states
	// like an empty-but-non-nil DynamicMetadata or RawBody).
	if got.GetRawBody() != nil {
		t.Errorf("RawBody: got %v; want nil (AMEND-6 omit unset optional)", got.GetRawBody())
	}
	if got.GetDynamicMetadata() != nil {
		t.Errorf("DynamicMetadata: got %v; want nil (AMEND-6 omit unset optional)", got.GetDynamicMetadata())
	}
	if got.GetQuota() != nil {
		t.Errorf("Quota: got %v; want nil (AMEND-6 omit unset optional)", got.GetQuota())
	}
	if len(got.GetResponseHeadersToAdd()) != 0 {
		t.Errorf("ResponseHeadersToAdd: got %v; want empty (AMEND-6 omit unset optional)", got.GetResponseHeadersToAdd())
	}
	if len(got.GetRequestHeadersToAdd()) != 0 {
		t.Errorf("RequestHeadersToAdd: got %v; want empty (AMEND-6 omit unset optional)", got.GetRequestHeadersToAdd())
	}
	if len(got.GetStatuses()) == 1 {
		ds := got.GetStatuses()[0]
		if ds.GetCurrentLimit() != nil {
			t.Errorf("DescriptorStatus.CurrentLimit: got %v; want nil (AMEND-6)", ds.GetCurrentLimit())
		}
		if ds.GetLimitRemaining() != 0 {
			t.Errorf("DescriptorStatus.LimitRemaining: got %v; want 0 (AMEND-6)", ds.GetLimitRemaining())
		}
		if ds.GetDurationUntilReset() != nil {
			t.Errorf("DescriptorStatus.DurationUntilReset: got %v; want nil (AMEND-6)", ds.GetDurationUntilReset())
		}
		if ds.GetQuota() != nil {
			t.Errorf("DescriptorStatus.Quota: got %v; want nil (AMEND-6)", ds.GetQuota())
		}
	}
}

// TestCanonicalKey_DeterministicOrdering verifies that CanonicalKey
// produces stable output for the same request shape — load-bearing
// because the fake's Script lookup keys on it and drivers compute the
// same key independently.
func TestCanonicalKey_DeterministicOrdering(t *testing.T) {
	req := &ratelimitv3.RateLimitRequest{
		Domain: "envoy",
		Descriptors: []*commonratelimitv3.RateLimitDescriptor{
			{Entries: []*commonratelimitv3.RateLimitDescriptor_Entry{
				{Key: "generic_key", Value: "foo"},
				{Key: "user", Value: "alice"},
			}},
			{Entries: []*commonratelimitv3.RateLimitDescriptor_Entry{
				{Key: "remote_address", Value: "10.0.0.1"},
			}},
		},
	}
	want := "envoy|generic_key=foo;user=alice|remote_address=10.0.0.1"
	if got := CanonicalKey(req); got != want {
		t.Errorf("CanonicalKey: got %q, want %q", got, want)
	}

	// Repeated calls are stable.
	for i := 0; i < 5; i++ {
		if got := CanonicalKey(req); got != want {
			t.Errorf("CanonicalKey (iter %d): got %q, want %q", i, got, want)
		}
	}

	// nil request → empty key (degenerate but well-defined).
	if got := CanonicalKey(nil); got != "" {
		t.Errorf("CanonicalKey(nil): got %q, want empty string", got)
	}

	// Empty-descriptors request → just the domain.
	emptyReq := &ratelimitv3.RateLimitRequest{Domain: "envoy"}
	if got := CanonicalKey(emptyReq); got != "envoy" {
		t.Errorf("CanonicalKey(empty descriptors): got %q, want %q", got, "envoy")
	}
}

// TestServer_Stop_Closes verifies that Stop terminates the listener and
// subsequent ShouldRateLimit calls fail.
func TestServer_Stop_Closes(t *testing.T) {
	srv := New(t)
	client := dialTestClient(t, srv.Addr())

	// Sanity: pre-Stop the server responds (default OK path).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.ShouldRateLimit(ctx, mkRequest("envoy", "generic_key", "ping")); err != nil {
		t.Fatalf("pre-Stop ShouldRateLimit: %v", err)
	}

	srv.Stop()

	// Post-Stop ShouldRateLimit MUST fail. The client may take a brief
	// moment to learn the connection is gone; use a fresh client with a
	// short deadline so the dial detects the closed listener.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel2()
	conn2, dialErr := grpc.NewClient(srv.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if dialErr != nil {
		// Acceptable: dial reports the closed listener at NewClient time.
		return
	}
	defer func() { _ = conn2.Close() }()
	client2 := ratelimitv3.NewRateLimitServiceClient(conn2)
	if _, err := client2.ShouldRateLimit(ctx2, mkRequest("envoy", "generic_key", "ping")); err == nil {
		t.Fatal("post-Stop ShouldRateLimit: want error (listener closed); got nil")
	}
}

// TestServer_ConcurrentClient_NoRace verifies that concurrent
// ShouldRateLimit calls against a single server instance do not trigger
// the race detector. Also exercises the Script-vs-ShouldRateLimit
// concurrent-access path via the RWMutex.
func TestServer_ConcurrentClient_NoRace(t *testing.T) {
	srv := New(t)
	scriptedReq := mkRequest("envoy", "generic_key", "ok")
	srv.Script(CanonicalKey(scriptedReq), &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OK,
		Statuses: []*ratelimitv3.RateLimitResponse_DescriptorStatus{
			{Code: ratelimitv3.RateLimitResponse_OK},
		},
	})

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)
	errs := make([]error, concurrency)
	for i := 0; i < concurrency; i++ {
		i := i
		go func() {
			defer wg.Done()
			client := dialTestClient(t, srv.Addr())
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			resp, err := client.ShouldRateLimit(ctx, scriptedReq)
			if err != nil {
				errs[i] = fmt.Errorf("goroutine[%d] ShouldRateLimit: %w", i, err)
				return
			}
			if resp.GetOverallCode() != ratelimitv3.RateLimitResponse_OK {
				errs[i] = fmt.Errorf("goroutine[%d]: OverallCode = %v; want OK", i, resp.GetOverallCode())
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}
