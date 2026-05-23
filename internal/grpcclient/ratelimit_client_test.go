package grpcclient

// ratelimit_client_test.go — DELTA-1 RateLimitClient test SCAFFOLDING per
// PLAN §Task-2 TDD Step 1. Mirrors the AuthClient test shape verbatim
// (`grpcclient_test.go` Group 2) — the RateLimitClient is the THIRD
// ADR-0158 two-tier typed wrapper; `ShouldRateLimit` is UNARY so the
// AuthClient `Check` shape is the precise precedent (NOT the bidi
// ProcessorClient).
//
// Four required tests per the PLAN Step 1 outline:
//
//   - TestRateLimitClient_ShouldRateLimit_Unary — in-process fake
//     `RegisterRateLimitServiceServer` returns a canned `RateLimitResponse`;
//     the wrapper round-trips the unary call (response struct propagated).
//   - TestRateLimitClient_Timeout — per-call `context.WithTimeout` when
//     `timeout > 0`; the fake server blocks; the per-Check timeout fires
//     and a recognizable DeadlineExceeded transport error surfaces.
//   - TestRateLimitClient_ErrorPropagation — transport error returned
//     verbatim (the server stops mid-flight → gRPC Unavailable; the wrapper
//     does NOT classify or wrap).
//   - TestRateLimitClient_Close_Idempotent — double-Close is safe via
//     sync.Once; both calls return the same cached error.
//
// Test infrastructure (helpers below) clones the AuthClient test shape:
// reuses `mkAuthPKI` + `mkH2ClusterMgr` + `mkPlainClusterMgr` from
// `grpcclient_test.go` (same package — `internal/grpcclient`); adds a
// `startTestRLSServer` + `fakeRLSServer` analogous to `startTestAuthServer`
// + `fakeAuthServer`. The PKI and h2 cluster manager are unchanged.

import (
	"context"
	stdtls "crypto/tls"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"google.golang.org/grpc"
)

// ----------------------------------------------------------------------------
// In-process gRPC ratelimit server (analog of `fakeAuthServer`).
// ----------------------------------------------------------------------------

// fakeRLSServer implements `ratelimitv3.RateLimitServiceServer`.
// `ShouldRateLimit`:
//
//   - returns `scripted` immediately if non-nil
//   - blocks on `ctx.Done()` if `scripted` is nil (used by timeout test)
type fakeRLSServer struct {
	ratelimitv3.UnimplementedRateLimitServiceServer
	scripted *ratelimitv3.RateLimitResponse
}

func (f *fakeRLSServer) ShouldRateLimit(ctx context.Context, _ *ratelimitv3.RateLimitRequest) (*ratelimitv3.RateLimitResponse, error) {
	if f.scripted != nil {
		return f.scripted, nil
	}
	// Block until the caller cancels; surfaces ctx.Err() (Canceled / DeadlineExceeded).
	<-ctx.Done()
	return nil, ctx.Err()
}

// startTestRLSServer starts a TLS-fronted `*grpc.Server` on a loopback port
// with ALPN h2; registers a `fakeRLSServer{scripted}`. Returns the bound
// port and a `stop` func (calls `GracefulStop`).
//
// Modeled byte-for-byte on `startTestAuthServer` — only the RegisterX call
// differs; the TLS listener + ALPN h2 setup is verbatim.
func startTestRLSServer(t testing.TB, pki *authTestPKI, scripted *ratelimitv3.RateLimitResponse) (uint32, func()) {
	t.Helper()
	pair, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM)
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	cfg := &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		NextProtos:   []string{"h2"},
		MinVersion:   stdtls.VersionTLS12,
		MaxVersion:   stdtls.VersionTLS13,
	}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}
	s := grpc.NewServer()
	ratelimitv3.RegisterRateLimitServiceServer(s, &fakeRLSServer{scripted: scripted})
	go func() {
		_ = s.Serve(ln)
	}()
	port := uint32(ln.Addr().(*net.TCPAddr).Port)
	stop := func() {
		s.GracefulStop()
		_ = ln.Close()
	}
	return port, stop
}

// ----------------------------------------------------------------------------
// RateLimitClient surface — four PLAN-required tests.
// ----------------------------------------------------------------------------

// TestRateLimitClient_ShouldRateLimit_Unary verifies the wrapper round-trips
// a unary call: NewRateLimitClient dials the cluster + the typed
// `ShouldRateLimit` stub returns the scripted `*RateLimitResponse`.
func TestRateLimitClient_ShouldRateLimit_Unary(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	scripted := &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OK,
	}
	port, stop := startTestRLSServer(t, pki, scripted)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_rls", port)
	d := New(mgr)

	rlc, err := NewRateLimitClient(d, "c_rls", 2*time.Second)
	if err != nil {
		t.Fatalf("NewRateLimitClient: %v", err)
	}
	if rlc == nil {
		t.Fatalf("NewRateLimitClient: nil RateLimitClient")
	}
	t.Cleanup(func() { _ = rlc.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := rlc.ShouldRateLimit(ctx, &ratelimitv3.RateLimitRequest{Domain: "test"})
	if err != nil {
		t.Fatalf("ShouldRateLimit: %v", err)
	}
	if resp == nil {
		t.Fatalf("ShouldRateLimit: nil response")
	}
	if resp.GetOverallCode() != ratelimitv3.RateLimitResponse_OK {
		t.Errorf("ShouldRateLimit: OverallCode = %v; want OK", resp.GetOverallCode())
	}
}

// TestRateLimitClient_Timeout verifies per-call `context.WithTimeout` when
// `timeout > 0`. The fake server blocks past the configured timeout;
// `ShouldRateLimit` must return a transport DeadlineExceeded error.
func TestRateLimitClient_Timeout(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	// nil scripted → fake server blocks until ctx.Done; the per-call timeout fires first.
	port, stop := startTestRLSServer(t, pki, nil)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_rls", port)
	d := New(mgr)

	// Very short per-call timeout so the test runs quickly.
	rlc, err := NewRateLimitClient(d, "c_rls", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewRateLimitClient: %v", err)
	}
	t.Cleanup(func() { _ = rlc.Close() })

	// Caller's ctx is generous; the per-call timeout fires first.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := rlc.ShouldRateLimit(ctx, &ratelimitv3.RateLimitRequest{Domain: "test"})
	if err == nil {
		t.Fatalf("ShouldRateLimit: err = nil; want timeout transport error")
	}
	if resp != nil {
		t.Errorf("ShouldRateLimit: resp = %v; want nil on timeout error", resp)
	}
	if !isDeadlineExceededTransportErr(err) {
		t.Errorf("ShouldRateLimit err = %v; want DeadlineExceeded transport error", err)
	}
}

// TestRateLimitClient_ErrorPropagation verifies the transport-error-verbatim
// discipline (mirrors the AuthClient D7 contract): gRPC `Unavailable`
// (server-down) propagates as the error return WITHOUT being mapped.
func TestRateLimitClient_ErrorPropagation(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestRLSServer(t, pki, &ratelimitv3.RateLimitResponse{})
	mgr := mkH2ClusterMgr(t, pki, "c_rls", port)
	d := New(mgr)

	rlc, err := NewRateLimitClient(d, "c_rls", 2*time.Second)
	if err != nil {
		t.Fatalf("NewRateLimitClient: %v", err)
	}
	t.Cleanup(func() { _ = rlc.Close() })

	// Kill the RLS server BEFORE the call — the next dial sub-channel
	// state transition sees Unavailable.
	stop()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	resp, err := rlc.ShouldRateLimit(ctx, &ratelimitv3.RateLimitRequest{Domain: "test"})
	if err == nil {
		t.Fatalf("ShouldRateLimit: err = nil; want Unavailable / transport error")
	}
	if resp != nil {
		t.Errorf("ShouldRateLimit: resp = %v; want nil on transport error", resp)
	}
	// The error MUST surface as the error return — the wrapper does NOT
	// synthesize a *RateLimitResponse to carry it. This codifies the
	// transport-error-verbatim contract inherited from AuthClient D7.
}

// TestRateLimitClient_Close_Idempotent verifies the sync.Once-guarded Close:
// repeated `Close()` calls return cleanly (the same cached error from the
// first call); double-Close is safe and does not panic.
func TestRateLimitClient_Close_Idempotent(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestRLSServer(t, pki, &ratelimitv3.RateLimitResponse{})
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_rls", port)
	d := New(mgr)

	rlc, err := NewRateLimitClient(d, "c_rls", 2*time.Second)
	if err != nil {
		t.Fatalf("NewRateLimitClient: %v", err)
	}

	// First Close: cached err captured (likely nil — gRPC ClientConn.Close
	// returns nil under normal conditions).
	err1 := rlc.Close()
	// Second Close: must return the SAME error as the first (sync.Once-cached).
	err2 := rlc.Close()
	// Third Close: still cached.
	err3 := rlc.Close()

	// All three must agree.
	if (err1 == nil) != (err2 == nil) || (err2 == nil) != (err3 == nil) {
		t.Errorf("Close idempotency: err1=%v, err2=%v, err3=%v; want all equal", err1, err2, err3)
	}
	if err1 != nil && (err1.Error() != err2.Error() || err2.Error() != err3.Error()) {
		t.Errorf("Close idempotency: err1=%q, err2=%q, err3=%q; want all equal", err1, err2, err3)
	}

	// Concurrent Close — guards the sync.Once + race-detector cleanliness.
	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			errs[i] = rlc.Close()
		}()
	}
	wg.Wait()

	// All N goroutines must observe the SAME cached error (whether nil or non-nil).
	for i, e := range errs {
		if (err1 == nil) != (e == nil) {
			t.Errorf("Close[%d]: err = %v; first = %v; want same nil-ness", i, e, err1)
			continue
		}
		if err1 != nil && err1.Error() != e.Error() {
			t.Errorf("Close[%d]: err = %q; first = %q; want same wording", i, e, err1)
		}
	}

	// Nil-receiver tolerance — the public API is safe to call on nil per
	// the AuthClient.Close precedent.
	var nilRLC *RateLimitClient
	if err := nilRLC.Close(); err != nil {
		t.Errorf("nil RateLimitClient Close: err = %v; want nil", err)
	}
}

// TestRateLimitClient_NewRateLimitClient_PropagatesDialError verifies that a
// dial-time PARSE-REJECT (unknown cluster) surfaces as the constructor's
// error return — mirrors AuthClient's NewAuthClient propagation contract.
// The error must mention the cluster name to aid diagnostics.
func TestRateLimitClient_NewRateLimitClient_PropagatesDialError(t *testing.T) {
	t.Parallel()
	mgr := mkPlainClusterMgr(t, "c_other", 9999) // wrong name → unknown-cluster
	d := New(mgr)

	rlc, err := NewRateLimitClient(d, "c_missing", time.Second)
	if err == nil {
		_ = rlc.Close()
		t.Fatalf("NewRateLimitClient: err = nil; want PARSE-REJECT propagation")
	}
	if rlc != nil {
		t.Errorf("NewRateLimitClient: rlc = %v; want nil on error", rlc)
	}
	if !strings.Contains(err.Error(), "c_missing") {
		t.Errorf("NewRateLimitClient err = %q; want substring %q", err.Error(), "c_missing")
	}
}
