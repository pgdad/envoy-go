package jwksbackend

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// jwkSetBody is a minimal RFC 7517 §A.1 JWK Set fragment used as the canned
// route payload in several tests below. Byte-exact match is asserted.
const jwkSetBody = `{"keys":[{"kty":"RSA","kid":"k1","alg":"RS256","use":"sig","n":"0vx7","e":"AQAB"}]}`

// startTestServer is the per-test lifecycle helper: it spawns a Server on an
// ephemeral port with the supplied routes and returns the base URL plus a
// cleanup func.
func startTestServer(t *testing.T, routes map[string]string) (*Server, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := New(ctx, "127.0.0.1:0", routes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv, "http://" + srv.Addr()
}

// httpGet performs a GET against base+path with a short deadline and returns
// (status, body, content-type, err).
func httpGet(t *testing.T, base, path string) (int, []byte, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return 0, nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, "", err
	}
	return resp.StatusCode, body, resp.Header.Get("Content-Type"), nil
}

func TestNew_StartsServerOnConfiguredAddr_ServesRoutes(t *testing.T) {
	srv, base := startTestServer(t, map[string]string{
		"/jwks.json": jwkSetBody,
	})
	if srv.Addr() == "" {
		t.Fatal("Addr: empty after New")
	}
	// Verify the bound address is actually a 127.0.0.1:<port> form.
	host, port, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", srv.Addr(), err)
	}
	if host != "127.0.0.1" {
		t.Errorf("host: got %q, want %q", host, "127.0.0.1")
	}
	if port == "0" {
		t.Errorf("port: got %q, want non-zero ephemeral", port)
	}
	status, body, _, err := httpGet(t, base, "/jwks.json")
	if err != nil {
		t.Fatalf("GET /jwks.json: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status: got %d, want 200", status)
	}
	if string(body) != jwkSetBody {
		t.Errorf("body: got %q, want %q", string(body), jwkSetBody)
	}
}

func TestServer_RouteServesJWKSetJSON_ByteExact(t *testing.T) {
	_, base := startTestServer(t, map[string]string{
		"/jwks.json": jwkSetBody,
	})
	status, body, ct, err := httpGet(t, base, "/jwks.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status: got %d, want 200", status)
	}
	if ct != "application/json" {
		t.Errorf("content-type: got %q, want %q", ct, "application/json")
	}
	// Byte-exact assertion (no Content-Length re-derivation, no whitespace
	// normalization). Load-bearing because the JWKS body is the JWK Set the
	// jwt_authn filter consumes verbatim per RFC 7517.
	if string(body) != jwkSetBody {
		t.Errorf("body byte-mismatch:\n got: %q\nwant: %q", string(body), jwkSetBody)
	}
}

func TestServer_MissingRoute_Returns404(t *testing.T) {
	_, base := startTestServer(t, map[string]string{
		"/jwks.json": jwkSetBody,
	})
	status, _, _, err := httpGet(t, base, "/does-not-exist")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if status != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", status)
	}
}

func TestServer_MultiRouteDispatch(t *testing.T) {
	const bodyA = `{"keys":[{"kty":"RSA","kid":"a"}]}`
	const bodyB = `{"keys":[{"kty":"EC","kid":"b"}]}`
	const bodyC = `{"keys":[{"kty":"OKP","kid":"c"}]}`
	_, base := startTestServer(t, map[string]string{
		"/a/jwks.json": bodyA,
		"/b/jwks.json": bodyB,
		"/c/jwks.json": bodyC,
	})
	cases := []struct {
		path, want string
	}{
		{"/a/jwks.json", bodyA},
		{"/b/jwks.json", bodyB},
		{"/c/jwks.json", bodyC},
	}
	for _, tc := range cases {
		status, body, _, err := httpGet(t, base, tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		if status != http.StatusOK {
			t.Errorf("GET %s: status got %d, want 200", tc.path, status)
		}
		if string(body) != tc.want {
			t.Errorf("GET %s: body got %q, want %q", tc.path, string(body), tc.want)
		}
	}
}

func TestServer_Stop_ClosesListener(t *testing.T) {
	srv, base := startTestServer(t, map[string]string{
		"/jwks.json": jwkSetBody,
	})
	// Sanity: GET succeeds before Stop.
	if status, _, _, err := httpGet(t, base, "/jwks.json"); err != nil || status != http.StatusOK {
		t.Fatalf("pre-stop GET: status=%d err=%v", status, err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Post-Stop dial against the listener address MUST fail (connection refused
	// once the listener has closed). Using a short deadline so the test fails
	// fast if Stop did NOT actually release the listener.
	_, _, _, err := httpGet(t, base, "/jwks.json")
	if err == nil {
		t.Fatal("post-Stop GET: want error (listener closed); got nil")
	}
	// Accept either "connection refused" or "EOF" as evidence of closed
	// listener; the exact wording is OS-dependent.
	msg := err.Error()
	if !strings.Contains(msg, "refused") && !strings.Contains(msg, "EOF") && !strings.Contains(msg, "reset") {
		// Not fatal — log for visibility but accept other transport errors.
		t.Logf("post-Stop GET error (accepted): %v", err)
	}
}

func TestServer_Stop_Idempotent(t *testing.T) {
	srv, _ := startTestServer(t, map[string]string{
		"/jwks.json": jwkSetBody,
	})
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop #1: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Errorf("Stop #2: idempotent call returned %v; want nil", err)
	}
}
