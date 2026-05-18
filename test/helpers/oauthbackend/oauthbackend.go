package oauthbackend

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// ReceivedRequest captures the per-request envelope the mock observed —
// method + path + raw body bytes. Used by drivers for post-run
// assertions that the filter under test actually fired the
// token_endpoint POST with the expected body shape.
type ReceivedRequest struct {
	Method string
	Path   string
	Body   []byte
	// Header is a COPY of the request headers; safe to mutate by the
	// reader.
	Header http.Header
}

// scriptedResponse is a single (method, path) → response triple. Stored
// under sync.RWMutex protection so per-fixture driver tests can
// re-script mid-run without races.
type scriptedResponse struct {
	status  int
	body    []byte
	headers map[string]string
}

// Server is a minimal in-process scriptable OAuth 2.0 authorization-
// server mock. Goroutine-safe: scripts + received slices are RWMutex-
// guarded so concurrent Script / handle / Received calls are race-clean.
//
// Lifecycle:
//   - New(t) binds 127.0.0.1:0 (ephemeral port) + spawns srv.Serve(lis)
//     in a goroutine + registers t.Cleanup(Stop). The caller reads
//     Addr() to templatize the authorization_endpoint /
//     token_endpoint URLs into envoy.yaml + envoy-go.yaml.
//   - NewAtAddr(addr) binds the caller-supplied host:port (pre-allocated
//     by the driver via Listen+Close so bootstraps render before the
//     mock starts) + spawns the goroutine. Caller manages lifecycle
//     via Server.Stop.
//   - Script(method, path, status, body, headers) registers (or
//     replaces) the scripted response for a (method, path) pair.
//   - Default route (no script registered for the inbound path) returns
//     404 with body "oauthbackend: no script for {method} {path}".
//   - Received() returns a snapshot copy of the per-request observation
//     slice — drivers use this for end-to-end wire-shape assertions.
//   - Stop() shuts the http.Server down via Shutdown; idempotent via
//     sync.Once.
type Server struct {
	addr string
	lis  net.Listener
	srv  *http.Server

	mu       sync.RWMutex
	scripts  map[string]*scriptedResponse // key: "METHOD path"
	received []ReceivedRequest

	stopOnce sync.Once
}

// New binds an ephemeral 127.0.0.1 listener and starts the OAuth-mock
// HTTP server in a background goroutine. Cleanup is registered via
// t.Cleanup so callers do NOT need to invoke Stop explicitly.
func New(t testing.TB) *Server {
	t.Helper()
	s, err := newServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("oauthbackend: %v", err)
	}
	t.Cleanup(func() { _ = s.Stop() })
	return s
}

// NewAtAddr binds the caller-supplied host:port and starts the mock.
// Used by differential fixtures that need a stable port baked into
// envoy.yaml / envoy-go.yaml bootstraps before the mock starts.
//
// Lifecycle management is the CALLER's responsibility — there is no
// t.Cleanup. Tests that want auto-cleanup should use New(t).
func NewAtAddr(addr string) (*Server, error) {
	return newServer(addr)
}

func newServer(addr string) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	s := &Server{
		addr:    lis.Addr().String(),
		lis:     lis,
		scripts: map[string]*scriptedResponse{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)

	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return context.Background() },
	}

	go func() {
		_ = s.srv.Serve(lis)
	}()

	return s, nil
}

// Addr returns the listener's bound `host:port` string. Load-bearing
// when New allocated an ephemeral port — fixture drivers read this back
// to templatize the authorization_endpoint + token_endpoint URLs.
func (s *Server) Addr() string {
	return s.addr
}

// Script registers (or replaces) the scripted response for (method,
// path). status <= 0 defaults to 200; nil body produces an empty
// response body; nil headers map produces no custom response headers.
//
// Per ADR-0085 nil-tolerance: re-scripting the same (method, path) pair
// is supported — the latest registration wins.
func (s *Server) Script(method, path string, status int, body []byte, headers map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scripts[scriptKey(method, path)] = &scriptedResponse{
		status:  status,
		body:    body,
		headers: headers,
	}
}

// TokenResponse is a convenience helper that registers a standard
// 200 OK JSON success response on POST {path} (typically "/token") per
// RFC 6749 §5.1. Each non-empty token field is included in the JSON
// envelope; an empty refreshToken / idToken value is omitted.
//
// Per RFC 6749 §5.1 Content-Type is "application/json".
func (s *Server) TokenResponse(path, accessToken, refreshToken, idToken string, expiresIn int) {
	payload := map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
	}
	if refreshToken != "" {
		payload["refresh_token"] = refreshToken
	}
	if idToken != "" {
		payload["id_token"] = idToken
	}
	body, _ := json.Marshal(payload)
	s.Script("POST", path, http.StatusOK, body, map[string]string{
		"Content-Type": "application/json",
	})
}

// Received returns a snapshot copy of the per-request observation
// slice. Safe to mutate by the caller — the snapshot is independent of
// the live recording slice.
func (s *Server) Received() []ReceivedRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.received) == 0 {
		return nil
	}
	out := make([]ReceivedRequest, len(s.received))
	copy(out, s.received)
	return out
}

// Reset clears the per-request observation slice. Used by drivers
// between scenarios to isolate per-scenario observability.
func (s *Server) Reset() {
	s.mu.Lock()
	s.received = s.received[:0]
	s.mu.Unlock()
}

// Stop terminates the server cleanly. Idempotent via sync.Once.
func (s *Server) Stop() error {
	var err error
	s.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = s.srv.Shutdown(ctx)
	})
	return err
}

// handle is the request dispatcher: record the observation + look up
// the scripted response + write it; 404 fall-through with a
// diagnostic body when no script is registered for the (method, path)
// pair.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	// Record the observation under the write lock so concurrent
	// Received() snapshots see the up-to-date slice.
	headerCopy := make(http.Header, len(r.Header))
	for k, vv := range r.Header {
		dst := make([]string, len(vv))
		copy(dst, vv)
		headerCopy[k] = dst
	}
	s.mu.Lock()
	s.received = append(s.received, ReceivedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Body:   body,
		Header: headerCopy,
	})
	resp, ok := s.scripts[scriptKey(r.Method, r.URL.Path)]
	s.mu.Unlock()

	if !ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, "oauthbackend: no script for %s %s\n", r.Method, r.URL.Path)
		return
	}

	for k, v := range resp.headers {
		w.Header().Set(k, v)
	}
	status := resp.status
	if status <= 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if len(resp.body) > 0 {
		_, _ = w.Write(resp.body)
	}
}

// scriptKey returns the internal map key for a (method, path) pair.
func scriptKey(method, path string) string {
	return method + " " + path
}

// -----------------------------------------------------------------------------
// Envelope helpers — used by drivers to seed cookie-passthrough scenarios
// and tampered-envelope scenarios.
// -----------------------------------------------------------------------------

// ValidCookieEnvelope builds the 5-cookie envelope the envoy.filters.
// http.oauth2 filter accepts on the (b1) cookie-passthrough leg. The
// envelope mirrors the per-phase-20 SPEC §6.4 + ADR-0181 + ADR-0179
// shape:
//
//   - BearerToken : AES-256-CBC-encrypted accessToken (per ADR-0182)
//   - OauthHMAC   : Base64URL-raw HMAC over 5-input newline-joined
//     (domain, expiresEpoch, BearerToken, idToken, RefreshToken)
//   - OauthExpires: decimal-string of expiresEpoch
//   - IdToken     : verbatim idToken value (or empty)
//   - RefreshToken: AES-256-CBC-encrypted refreshToken (per ADR-0182)
//
// The cookie names match DefaultCookieNames at oauth2/cookies.go. The
// helper exists in oauthbackend (not in test/helpers/oauthbackend
// /helpers) because every fixture driver that seeds an envelope needs
// it; cross-fixture reuse is enabled by housing the helper at the
// oauthbackend package level.
func ValidCookieEnvelope(hmacSecret []byte, accessToken, refreshToken, idToken, domain string, expiresEpoch int64) []*http.Cookie {
	bt := envEncrypt([]byte(accessToken), hmacSecret)
	rt := ""
	if refreshToken != "" {
		rt = envEncrypt([]byte(refreshToken), hmacSecret)
	}
	expiresStr := fmt.Sprintf("%d", expiresEpoch)
	h := envComputeHMAC(domain, expiresStr, bt, idToken, rt, hmacSecret)

	out := []*http.Cookie{
		{Name: "BearerToken", Value: bt},
		{Name: "OauthHMAC", Value: h},
		{Name: "OauthExpires", Value: expiresStr},
	}
	if idToken != "" {
		out = append(out, &http.Cookie{Name: "IdToken", Value: idToken})
	}
	if rt != "" {
		out = append(out, &http.Cookie{Name: "RefreshToken", Value: rt})
	}
	return out
}

// TamperedStateCookie returns a state cookie whose value is a tampered
// HMAC envelope — useful for scenarios that exercise the filter's
// bad-state-cookie deny path per SPEC §6.6 + ADR-0180. The tampering
// flips the first byte of the HMAC component; the constant-time HMAC
// compare at hmac.validateHMAC then fails.
func TamperedStateCookie(name string, originalValue string) *http.Cookie {
	if len(originalValue) == 0 {
		return &http.Cookie{Name: name, Value: "tampered"}
	}
	b := []byte(originalValue)
	b[0] ^= 0x01
	return &http.Cookie{Name: name, Value: string(b)}
}

// -----------------------------------------------------------------------------
// Internal HMAC / AES helpers — mirror the oauth2 filter's hmac.go +
// tokens.go shapes so drivers do not need to import the (internal) filter
// package. Kept private to this helper package; drivers consume via
// ValidCookieEnvelope.
// -----------------------------------------------------------------------------

// envComputeHMAC mirrors oauth2/hmac.go::computeHMAC: 5-input newline-
// joined HMAC-SHA256 with Base64URL-raw encoding. See ADR-0179 + AMEND-2.
func envComputeHMAC(domain, expires, token, idToken, refreshToken string, hmacSecret []byte) string {
	totalLen := len(domain) + len(expires) + len(token) +
		len(idToken) + len(refreshToken) + 4
	msg := make([]byte, 0, totalLen)
	msg = append(msg, domain...)
	msg = append(msg, '\n')
	msg = append(msg, expires...)
	msg = append(msg, '\n')
	msg = append(msg, token...)
	msg = append(msg, '\n')
	msg = append(msg, idToken...)
	msg = append(msg, '\n')
	msg = append(msg, refreshToken...)

	mac := hmac.New(sha256.New, hmacSecret)
	mac.Write(msg)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// envEncrypt mirrors oauth2/tokens.go::encryptToken: AES-256-CBC with a
// random 16-byte IV; SHA-256(hmacSecret)[:32] key; PKCS#7 padding;
// base64.RawURLEncoding(IV ‖ CT) envelope.
func envEncrypt(plaintext, hmacSecret []byte) string {
	key := sha256.Sum256(hmacSecret)
	block, _ := aes.NewCipher(key[:])
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		panic("oauthbackend: rand.Read: " + err.Error())
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)
	envelope := make([]byte, 0, aes.BlockSize+len(ct))
	envelope = append(envelope, iv...)
	envelope = append(envelope, ct...)
	return base64.RawURLEncoding.EncodeToString(envelope)
}

// pkcs7Pad mirrors oauth2/tokens.go::pkcs7Pad.
func pkcs7Pad(plaintext []byte, blockSize int) []byte {
	padLen := blockSize - (len(plaintext) % blockSize)
	out := make([]byte, len(plaintext)+padLen)
	copy(out, plaintext)
	for i := len(plaintext); i < len(out); i++ {
		out[i] = byte(padLen)
	}
	return out
}
