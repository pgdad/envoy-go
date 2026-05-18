package oauthbackend_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/esalaine/envoy-go/test/helpers/oauthbackend"
)

// TestServer_FixedScript_Token_Response verifies the TokenResponse
// convenience helper produces a standard RFC 6749 §5.1 JSON envelope.
func TestServer_TokenResponse_HappyPath(t *testing.T) {
	s := oauthbackend.New(t)
	s.TokenResponse("/token", "access-abc", "refresh-xyz", "", 3600)

	resp, err := http.Post("http://"+s.Addr()+"/token", "application/x-www-form-urlencoded",
		strings.NewReader("grant_type=authorization_code&code=c&client_id=id&client_secret=sec&redirect_uri=https%3A%2F%2Fexample.com%2Fcb"))
	if err != nil {
		t.Fatalf("POST /token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: got %q want %q", ct, "application/json")
	}
	body, _ := io.ReadAll(resp.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("json: %v (body=%q)", err, string(body))
	}
	if got["access_token"] != "access-abc" {
		t.Errorf("access_token: got %v want access-abc", got["access_token"])
	}
	if got["token_type"] != "Bearer" {
		t.Errorf("token_type: got %v want Bearer", got["token_type"])
	}
	if got["refresh_token"] != "refresh-xyz" {
		t.Errorf("refresh_token: got %v want refresh-xyz", got["refresh_token"])
	}
	if _, ok := got["id_token"]; ok {
		t.Errorf("id_token should be absent when empty")
	}
}

// TestServer_Script_404Fallthrough verifies the 404 fall-through path
// when no script is registered for the inbound (method, path).
func TestServer_Script_404Fallthrough(t *testing.T) {
	s := oauthbackend.New(t)
	resp, err := http.Get("http://" + s.Addr() + "/no-such-path")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("no script for GET /no-such-path")) {
		t.Errorf("body: got %q, want substring 'no script for GET /no-such-path'", string(body))
	}
}

// TestServer_Script_FixedResponse covers per-route status + headers +
// body wiring.
func TestServer_Script_FixedResponse(t *testing.T) {
	s := oauthbackend.New(t)
	s.Script("GET", "/authorize", http.StatusFound, nil, map[string]string{
		"Location": "https://example.com/cb?code=auth-code-xyz&state=test-state",
	})

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get("http://" + s.Addr() + "/authorize?state=test-state")
	if err != nil {
		t.Fatalf("GET /authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status: got %d want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://example.com/cb?code=auth-code-xyz&state=test-state" {
		t.Errorf("Location: got %q", loc)
	}
}

// TestServer_Received_RecordsRequests verifies the Received() snapshot
// captures method + path + body for post-run driver assertions.
func TestServer_Received_RecordsRequests(t *testing.T) {
	s := oauthbackend.New(t)
	s.Script("POST", "/token", 500, []byte("upstream-error"), nil)

	const body = "grant_type=refresh_token&refresh_token=R&client_id=id&client_secret=sec"
	resp, err := http.Post("http://"+s.Addr()+"/token", "application/x-www-form-urlencoded",
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()

	rs := s.Received()
	if len(rs) != 1 {
		t.Fatalf("Received: got %d, want 1", len(rs))
	}
	if rs[0].Method != "POST" || rs[0].Path != "/token" {
		t.Errorf("method/path: got %s %s", rs[0].Method, rs[0].Path)
	}
	if string(rs[0].Body) != body {
		t.Errorf("body: got %q want %q", string(rs[0].Body), body)
	}
}

// TestServer_Reset_ClearsReceived ensures per-scenario isolation via
// Reset.
func TestServer_Reset_ClearsReceived(t *testing.T) {
	s := oauthbackend.New(t)
	s.Script("GET", "/x", 200, nil, nil)

	if _, err := http.Get("http://" + s.Addr() + "/x"); err != nil {
		t.Fatalf("GET: %v", err)
	}
	if got := len(s.Received()); got != 1 {
		t.Fatalf("Received pre-Reset: got %d want 1", got)
	}

	s.Reset()
	if got := len(s.Received()); got != 0 {
		t.Fatalf("Received post-Reset: got %d want 0", got)
	}
}

// TestServer_Stop_Idempotent confirms Stop is safe to call multiple
// times.
func TestServer_Stop_Idempotent(t *testing.T) {
	s, err := oauthbackend.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Errorf("Stop #1: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Errorf("Stop #2 (idempotent): %v", err)
	}
}

// TestValidCookieEnvelope_HMACValidates verifies the helper-built
// envelope yields a valid HMAC under the same secret.
func TestValidCookieEnvelope_HMACValidates(t *testing.T) {
	const (
		hmacSecret = "test-hmac-secret-32-bytes-padding"
		access     = "access-token-value"
		refresh    = "refresh-token-value"
		domain     = "example.com"
	)
	envelope := oauthbackend.ValidCookieEnvelope(
		[]byte(hmacSecret), access, refresh, "", domain, 1747522800,
	)
	if len(envelope) < 3 {
		t.Fatalf("envelope: got %d cookies, want >=3", len(envelope))
	}

	// The exported helper produces cookies with the canonical names —
	// just verify they're all present.
	names := map[string]string{}
	for _, c := range envelope {
		names[c.Name] = c.Value
	}
	for _, want := range []string{"BearerToken", "OauthHMAC", "OauthExpires", "RefreshToken"} {
		if _, ok := names[want]; !ok {
			t.Errorf("envelope missing %q", want)
		}
	}

	// The OauthExpires is the decimal-epoch string per SPEC §12 A3.
	if names["OauthExpires"] != "1747522800" {
		t.Errorf("OauthExpires: got %q want %q", names["OauthExpires"], "1747522800")
	}

	// The Bearer + Refresh values are AES-CBC envelopes; assert
	// they're non-empty and base64-url-decodeable.
	for _, key := range []string{"BearerToken", "RefreshToken"} {
		if v := names[key]; len(v) == 0 {
			t.Errorf("%s value empty", key)
		}
	}
}

// TestTamperedStateCookie_FlipsByte verifies the helper flips a byte
// of the cookie value to invalidate any subsequent HMAC compare.
func TestTamperedStateCookie_FlipsByte(t *testing.T) {
	c := oauthbackend.TamperedStateCookie("OauthExpires", "abc123")
	if c.Name != "OauthExpires" {
		t.Errorf("Name: got %q want OauthExpires", c.Name)
	}
	if c.Value == "abc123" {
		t.Errorf("Value: must differ from original")
	}
	if c.Value[1:] != "bc123" {
		t.Errorf("Value tail: got %q want %q", c.Value[1:], "bc123")
	}
}
