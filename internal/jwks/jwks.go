package jwks

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pgdad/envoy-go/internal/httpclient"
)

// ----------------------------------------------------------------------------
// Constants per ADR-0150 §Decision + §11.P4 + §11.P5.
// ----------------------------------------------------------------------------

const (
	// defaultCacheDuration is the per-Envoy `DefaultCacheExpirationSec=600s`.
	defaultCacheDuration = 10 * time.Minute

	// refetchBeforeExpired is the Envoy `RefetchBeforeExpiredSec=5s` constant —
	// refresh fires `cacheDuration - 5s` BEFORE TTL expiry.
	refetchBeforeExpired = 5 * time.Second

	// defaultFailedRefetchDuration is the Envoy `DefaultRefetchAfterFailedSec=1s`
	// constant; the FIXED-INTERVAL failed-refetch period (NOT exponential per
	// §11.P4 REFUTES BRAINSTORM).
	defaultFailedRefetchDuration = 1 * time.Second

	// defaultNumRetries is the inner-HTTP-request default retry count per
	// `envoy.config.core.v3.RetryPolicy.num_retries` proto comment.
	defaultNumRetries = 1

	// defaultRetryBaseInterval per Envoy proto comment.
	defaultRetryBaseInterval = 1 * time.Second

	// defaultHTTPClientTimeout — keep individual HTTP requests bounded.
	defaultHTTPClientTimeout = 30 * time.Second
)

// ----------------------------------------------------------------------------
// Canonical errors per ADR-0150 §Decision (~12 sentinels covering §11.P1 JWKS
// subset).
// ----------------------------------------------------------------------------

var (
	// ErrJwksFetchFail wraps an HTTP-level fetch failure (non-2xx after
	// retries; connection error; read error).
	ErrJwksFetchFail = errors.New("jwks: fetch failed")

	// ErrJwksParseError wraps a JWK Set JSON parse failure or a malformed
	// individual key (missing/invalid params).
	ErrJwksParseError = errors.New("jwks: parse error")

	// ErrJwksKidAlgMismatch surfaces from JWKSet.Lookup when no key in the set
	// matches the requested kid + alg.
	ErrJwksKidAlgMismatch = errors.New("jwks: no key matched kid+alg")

	// ErrJwksNotReady surfaces from Get when fast_listener=true + the initial
	// async fetch has not yet completed.
	ErrJwksNotReady = errors.New("jwks: not ready (async fetch in progress)")

	// ErrJwksNoValidKeys surfaces from ParseJWKSet when the JSON parsed but no
	// supported keys remain after filtering unsupported kty entries.
	ErrJwksNoValidKeys = errors.New("jwks: no valid keys in JWK Set")

	// ErrJwksClosed surfaces from Get / Close on a closed Fetcher.
	ErrJwksClosed = errors.New("jwks: fetcher is closed")

	// ErrJwksMissingURI surfaces from New when uri == "".
	ErrJwksMissingURI = errors.New("jwks: URI is required")

	// ErrJwksUnsupportedKty surfaces from ParseJWKSet for a single key with a
	// non-RSA / non-EC kty when surfacing the per-entry error to callers; the
	// JWK Set-level disposition silently SKIPS the entry and only fails the
	// overall parse iff zero supported keys remain (yielding ErrJwksNoValidKeys).
	ErrJwksUnsupportedKty = errors.New("jwks: unsupported kty")

	// ErrJwksUnsupportedCurve surfaces from EC key parsing for curves outside
	// the P-256 / P-384 / P-521 allow-list.
	ErrJwksUnsupportedCurve = errors.New("jwks: unsupported curve")

	// ErrJwksMalformedKey surfaces from RSA/EC parsing for missing or
	// undecodable key parameters (n/e for RSA; x/y/crv for EC).
	ErrJwksMalformedKey = errors.New("jwks: malformed key parameters")
)

// ----------------------------------------------------------------------------
// Public types.
// ----------------------------------------------------------------------------

// AsyncFetch carries optional JwksAsyncFetch settings. Zero-value field
// defaults mirror Envoy: FastListener=false; FailedRefetchDuration=1s.
type AsyncFetch struct {
	// FastListener: if true, New() returns immediately and the initial fetch
	// runs in a background goroutine; calls to Get before the fetch completes
	// return ErrJwksNotReady. If false (default), New() performs a blocking
	// initial fetch and returns its error on failure.
	FastListener bool

	// FailedRefetchDuration: the FIXED-INTERVAL pacing for the outer refresh
	// loop after a failure (1s default per §11.P4 REFUTES exponential-backoff).
	FailedRefetchDuration time.Duration
}

// RetryPolicy carries the inner-HTTP-request retry policy (NOT the outer
// refresh schedule). Mirrors envoy.config.core.v3.RetryPolicy semantic.
type RetryPolicy struct {
	// NumRetries: number of retries AFTER the initial attempt (so total
	// attempts = NumRetries + 1). Default 1.
	NumRetries uint32

	// BaseInterval: starting retry interval; doubles each retry up to
	// MaxInterval. Default 1s.
	BaseInterval time.Duration

	// MaxInterval: upper bound on retry interval. Default 10*BaseInterval.
	MaxInterval time.Duration
}

// JWKSet is a parsed RFC 7517 §5 JWK Set.
type JWKSet struct {
	// Keys: the parsed JWKs in source order. After ParseJWKSet, every entry
	// has a non-nil Key field pointing at a *rsa.PublicKey or *ecdsa.PublicKey.
	Keys []*JWK
}

// JWK represents one key in a JWK Set with its parsed public key.
type JWK struct {
	Kid string
	Alg string
	Use string
	Kty string
	Key crypto.PublicKey
}

// Fetcher is an opaque HTTP-outbound JWKS fetcher with async background
// refresh and thread-safe cache. One per RemoteJwks JwtProvider.
//
// Per ADR-0150 §Decision AMENDMENT (phase-20 IMPL Task 2b): the Fetcher no
// longer owns its own `*http.Client`; it consumes the shared
// `*httpclient.Client` framework primitive (ADR-0177). The inner-HTTP retry
// loop (the jwks-internal exponential `BaseInterval * 2^attempt` schedule)
// is PRESERVED VERBATIM at the jwks consumer layer — `Options.RetryPolicy`
// is left zero at the httpclient site because jwks's retry shape (exponential
// per §11.P4 REFUTES BRAINSTORM) diverges from httpclient's fixed-
// `PerAttemptDelay` shape (a deliberate consumer-layer choice).
type Fetcher struct {
	uri                   string
	cacheDuration         time.Duration
	failedRefetchDuration time.Duration
	retryPolicy           *RetryPolicy
	client                *httpclient.Client

	mu           sync.Mutex
	keys         *JWKSet
	ready        chan struct{} // closed once initial fetch completes (success or fail)
	notReadyErr  error         // populated under mu; set when initial fetch fails (fast_listener=true)
	readyOnce    sync.Once     // guards ready-channel close (idempotent)
	refreshTimer *time.Timer

	closed atomic.Bool
}

// ----------------------------------------------------------------------------
// Constructor.
// ----------------------------------------------------------------------------

// New constructs a Fetcher. Behavior depends on fast_listener (per ADR-0150
// §Decision (iii)):
//
//   - fast_listener=false (DEFAULT): performs a BLOCKING initial fetch;
//     returns error from New on failure (fail-loud-at-listener-load per
//     planner-time decision 3).
//   - fast_listener=true: returns immediately; spawns a goroutine for the
//     initial fetch; subsequent Get() returns ErrJwksNotReady until the ready
//     channel closes.
//
// cacheDuration defaults to 10 minutes if zero (DefaultCacheExpirationSec).
// retryPolicy is honored on each HTTP request via a retry loop honoring
// NumRetries + BaseInterval + MaxInterval; nil retryPolicy uses defaults.
//
// Per ADR-0150 §Decision AMENDMENT (phase-20 IMPL Task 2b): `httpClient` is
// the shared `*httpclient.Client` framework primitive (ADR-0177). A nil
// `httpClient` allocates a per-Fetcher default `httpclient.New(httpclient.
// Options{Timeout: defaultHTTPClientTimeout})` preserving the phase-17-pinned
// 30-second per-request timeout — supports test ergonomics (test sites pass
// nil) AND the boot-time consumer's threading of the shared singleton (the
// jwtauthn factory threads the shared `*httpclient.Client` from
// `cmd/envoy-go/main.go` per the introduction-time-3-consumer pattern).
func New(uri string, cacheDuration time.Duration, asyncFetch *AsyncFetch, retryPolicy *RetryPolicy, httpClient *httpclient.Client) (*Fetcher, error) {
	if uri == "" {
		return nil, ErrJwksMissingURI
	}
	if cacheDuration == 0 {
		cacheDuration = defaultCacheDuration
	}
	failedRefetch := defaultFailedRefetchDuration
	fastListener := false
	if asyncFetch != nil {
		if asyncFetch.FailedRefetchDuration > 0 {
			failedRefetch = asyncFetch.FailedRefetchDuration
		}
		fastListener = asyncFetch.FastListener
	}

	// Per ADR-0150 §Decision AMENDMENT: nil-tolerant default allocation
	// preserves the phase-17-pinned per-request timeout when callers omit
	// the shared singleton (test ergonomics + nil-tolerance per ADR-0085).
	if httpClient == nil {
		httpClient = httpclient.New(httpclient.Options{Timeout: defaultHTTPClientTimeout})
	}

	f := &Fetcher{
		uri:                   uri,
		cacheDuration:         cacheDuration,
		failedRefetchDuration: failedRefetch,
		retryPolicy:           retryPolicy,
		client:                httpClient,
		ready:                 make(chan struct{}),
	}

	if fastListener {
		// Non-blocking: spawn the initial fetch goroutine; New returns
		// immediately. Get() before completion → ErrJwksNotReady.
		go f.refreshLoop(true)
		return f, nil
	}

	// Blocking: perform the initial fetch on the calling goroutine; on failure
	// return error from New (fail-loud-at-listener-load per ADR-0150
	// §Decision (iii) + planner-time decision 3).
	set, err := f.doFetch(context.Background())
	if err != nil {
		// Ensure ready is closed so any future Get sees the error (not a hang).
		f.readyOnce.Do(func() { close(f.ready) })
		f.mu.Lock()
		f.notReadyErr = err
		f.mu.Unlock()
		return nil, err
	}
	f.mu.Lock()
	f.keys = set
	f.refreshTimer = time.AfterFunc(nextRefreshDelay(f.cacheDuration), f.scheduledRefresh)
	f.mu.Unlock()
	f.readyOnce.Do(func() { close(f.ready) })
	return f, nil
}

// ----------------------------------------------------------------------------
// Public API.
// ----------------------------------------------------------------------------

// Get returns the cached JWKSet. Returns ErrJwksNotReady in fast_listener-mode
// while the initial fetch has not yet completed. Returns ErrJwksClosed after
// Close. Safe for concurrent invocation.
func (f *Fetcher) Get(ctx context.Context) (*JWKSet, error) {
	if f.closed.Load() {
		return nil, ErrJwksClosed
	}
	// Snapshot the ready-channel state non-blockingly; if not yet ready, the
	// initial fetch is still in flight.
	select {
	case <-f.ready:
		// fall-through to load
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrJwksNotReady
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.notReadyErr != nil && f.keys == nil {
		return nil, f.notReadyErr
	}
	if f.keys == nil {
		return nil, ErrJwksNotReady
	}
	return f.keys, nil
}

// Close terminates the background refresh goroutine. Idempotent.
func (f *Fetcher) Close() error {
	if f.closed.Swap(true) {
		return nil
	}
	f.mu.Lock()
	if f.refreshTimer != nil {
		f.refreshTimer.Stop()
	}
	f.mu.Unlock()
	return nil
}

// Lookup returns the matching public key per Envoy's pickKeyAlgWithKid logic:
//
//   - If kid is non-empty AND any key has matching Kid: among kid-matched keys,
//     prefer one whose Alg == alg (case-insensitive); fall back to first kid-
//     match.
//   - If kid is empty: prefer first key with matching Alg (case-insensitive);
//     fall back to first key with empty Alg.
//
// Returns ErrJwksKidAlgMismatch on no match.
func (s *JWKSet) Lookup(kid, alg string) (crypto.PublicKey, error) {
	if s == nil || len(s.Keys) == 0 {
		return nil, ErrJwksKidAlgMismatch
	}
	algLower := strings.ToLower(alg)
	if kid != "" {
		var fallback *JWK
		for _, k := range s.Keys {
			if k.Kid != kid {
				continue
			}
			if fallback == nil {
				fallback = k
			}
			if alg != "" && strings.ToLower(k.Alg) == algLower {
				return k.Key, nil
			}
		}
		if fallback != nil {
			return fallback.Key, nil
		}
		return nil, ErrJwksKidAlgMismatch
	}
	// kid empty: prefer first alg-match; fall back to first key with empty Alg.
	var emptyAlgFallback *JWK
	for _, k := range s.Keys {
		if alg != "" && strings.ToLower(k.Alg) == algLower {
			return k.Key, nil
		}
		if k.Alg == "" && emptyAlgFallback == nil {
			emptyAlgFallback = k
		}
	}
	if emptyAlgFallback != nil {
		return emptyAlgFallback.Key, nil
	}
	return nil, ErrJwksKidAlgMismatch
}

// ----------------------------------------------------------------------------
// Internal: refresh loop + scheduling.
// ----------------------------------------------------------------------------

// refreshLoop is invoked for the FastListener path's initial fetch. After the
// first attempt (success or failure) it schedules the next refresh via the
// timer-based scheduledRefresh chain.
func (f *Fetcher) refreshLoop(initial bool) {
	if f.closed.Load() {
		return
	}
	set, err := f.doFetch(context.Background())
	f.mu.Lock()
	if f.closed.Load() {
		f.mu.Unlock()
		return
	}
	var delay time.Duration
	if err != nil {
		if initial {
			f.notReadyErr = err
		}
		delay = f.failedRefetchDuration
	} else {
		f.keys = set
		f.notReadyErr = nil
		delay = nextRefreshDelay(f.cacheDuration)
	}
	f.refreshTimer = time.AfterFunc(delay, f.scheduledRefresh)
	f.mu.Unlock()
	if initial {
		f.readyOnce.Do(func() { close(f.ready) })
	}
}

// scheduledRefresh is invoked by the AfterFunc timer. It performs one fetch
// attempt and reschedules the next refresh.
func (f *Fetcher) scheduledRefresh() {
	if f.closed.Load() {
		return
	}
	set, err := f.doFetch(context.Background())
	f.mu.Lock()
	if f.closed.Load() {
		f.mu.Unlock()
		return
	}
	var delay time.Duration
	if err != nil {
		// On refresh failure we keep the previously-cached set intact (per
		// Envoy semantic) and schedule the next attempt at the failed-refetch
		// interval.
		delay = f.failedRefetchDuration
	} else {
		f.keys = set
		delay = nextRefreshDelay(f.cacheDuration)
	}
	f.refreshTimer = time.AfterFunc(delay, f.scheduledRefresh)
	f.mu.Unlock()
}

// nextRefreshDelay returns `max(cacheDuration - 5s, 0)` per §11.P5.
func nextRefreshDelay(cacheDuration time.Duration) time.Duration {
	d := cacheDuration - refetchBeforeExpired
	if d < 0 {
		return 0
	}
	return d
}

// ----------------------------------------------------------------------------
// Internal: HTTP fetch with retry policy.
// ----------------------------------------------------------------------------

// doFetch performs one outer fetch cycle: at most (NumRetries+1) HTTP attempts
// per the configured RetryPolicy. Returns the parsed JWK Set on success or
// an error wrapping ErrJwksFetchFail / ErrJwksParseError.
func (f *Fetcher) doFetch(ctx context.Context) (*JWKSet, error) {
	rp := f.effectiveRetryPolicy()
	var lastErr error
	interval := rp.BaseInterval
	maxAttempts := int(rp.NumRetries) + 1
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(interval):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			interval *= 2
			if interval > rp.MaxInterval {
				interval = rp.MaxInterval
			}
		}
		body, err := f.doHTTPGet(ctx)
		if err != nil {
			lastErr = err
			continue
		}
		set, parseErr := ParseJWKSet(body)
		if parseErr != nil {
			// Parse error is NOT retried (the bytes are deterministic; retrying
			// the same upstream returns the same bytes).
			return nil, parseErr
		}
		return set, nil
	}
	if lastErr == nil {
		lastErr = ErrJwksFetchFail
	}
	return nil, fmt.Errorf("%w: %v", ErrJwksFetchFail, lastErr)
}

// doHTTPGet performs one HTTP GET against f.uri; returns body bytes on 2xx or
// an error.
//
// Per ADR-0150 §Decision AMENDMENT (phase-20 IMPL Task 2b): delegates to the
// shared `*httpclient.Client.Do` method (ADR-0177). The `Do` signature is
// bit-for-bit identical to `(*http.Client).Do` so the call-site shape is
// unchanged; only the receiver type changed (from `*http.Client` to
// `*httpclient.Client`). The jwks-internal exponential retry schedule
// (§11.P4 REFUTES BRAINSTORM) is preserved in `doFetch` above; the
// httpclient `Options.RetryPolicy` is left zero at the jwks consumer site.
func (f *Fetcher) doHTTPGet(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.uri, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// effectiveRetryPolicy returns an instance with defaults filled. Nil-tolerant.
func (f *Fetcher) effectiveRetryPolicy() RetryPolicy {
	rp := RetryPolicy{
		NumRetries:   defaultNumRetries,
		BaseInterval: defaultRetryBaseInterval,
	}
	if f.retryPolicy != nil {
		// Honor caller's NumRetries verbatim (including 0).
		rp.NumRetries = f.retryPolicy.NumRetries
		if f.retryPolicy.BaseInterval > 0 {
			rp.BaseInterval = f.retryPolicy.BaseInterval
		}
		if f.retryPolicy.MaxInterval > 0 {
			rp.MaxInterval = f.retryPolicy.MaxInterval
		}
	}
	if rp.MaxInterval == 0 {
		rp.MaxInterval = 10 * rp.BaseInterval
	}
	return rp
}

// ----------------------------------------------------------------------------
// JWK Set parsing per RFC 7517 §5.
// ----------------------------------------------------------------------------

// rawJWKSet is the JSON-decoding shape for the outer envelope.
type rawJWKSet struct {
	Keys *[]rawJWK `json:"keys"`
}

// rawJWK is the JSON-decoding shape for one JWK.
type rawJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	// RSA params.
	N string `json:"n"`
	E string `json:"e"`
	// EC params.
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// ParseJWKSet parses RFC 7517 §5 JWK Set JSON. Supports kty=RSA (n+e) and
// kty=EC (crv+x+y) for the P-256 / P-384 / P-521 curve allow-list. Unsupported
// kty entries are SILENTLY SKIPPED; the overall parse fails iff zero supported
// keys remain (yielding ErrJwksNoValidKeys).
func ParseJWKSet(raw []byte) (*JWKSet, error) {
	var rs rawJWKSet
	if err := json.Unmarshal(raw, &rs); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrJwksParseError, err)
	}
	if rs.Keys == nil {
		return nil, fmt.Errorf("%w: missing keys array", ErrJwksNoValidKeys)
	}
	if len(*rs.Keys) == 0 {
		return nil, fmt.Errorf("%w: empty keys array", ErrJwksNoValidKeys)
	}

	set := &JWKSet{}
	var lastParseErr error
	for i := range *rs.Keys {
		rk := &(*rs.Keys)[i]
		jwk, err := parseOneJWK(rk)
		if err != nil {
			if errors.Is(err, ErrJwksUnsupportedKty) {
				// Silently skip unsupported kty entries.
				continue
			}
			// Other parse errors: capture last but continue scanning so a
			// single bad RSA/EC entry does not poison an otherwise-valid set.
			lastParseErr = err
			continue
		}
		set.Keys = append(set.Keys, jwk)
	}
	if len(set.Keys) == 0 {
		if lastParseErr != nil {
			return nil, lastParseErr
		}
		return nil, ErrJwksNoValidKeys
	}
	return set, nil
}

// parseOneJWK parses one rawJWK into a *JWK with a typed public key. Returns
// ErrJwksUnsupportedKty for non-RSA/non-EC kty values (caller skips).
func parseOneJWK(rk *rawJWK) (*JWK, error) {
	switch rk.Kty {
	case "RSA":
		pub, err := parseRSA(rk)
		if err != nil {
			return nil, err
		}
		return &JWK{Kid: rk.Kid, Alg: rk.Alg, Use: rk.Use, Kty: rk.Kty, Key: pub}, nil
	case "EC":
		pub, err := parseEC(rk)
		if err != nil {
			return nil, err
		}
		return &JWK{Kid: rk.Kid, Alg: rk.Alg, Use: rk.Use, Kty: rk.Kty, Key: pub}, nil
	default:
		return nil, fmt.Errorf("%w: kty=%q", ErrJwksUnsupportedKty, rk.Kty)
	}
}

// parseRSA constructs an *rsa.PublicKey from base64url-decoded n + e.
func parseRSA(rk *rawJWK) (*rsa.PublicKey, error) {
	if rk.N == "" || rk.E == "" {
		return nil, fmt.Errorf("%w: RSA missing n or e", ErrJwksParseError)
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(rk.N)
	if err != nil {
		return nil, fmt.Errorf("%w: RSA n decode: %v", ErrJwksParseError, err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(rk.E)
	if err != nil {
		return nil, fmt.Errorf("%w: RSA e decode: %v", ErrJwksParseError, err)
	}
	if len(nBytes) == 0 || len(eBytes) == 0 {
		return nil, fmt.Errorf("%w: RSA n or e empty", ErrJwksMalformedKey)
	}
	pub := &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}
	if pub.E == 0 {
		return nil, fmt.Errorf("%w: RSA e=0", ErrJwksMalformedKey)
	}
	return pub, nil
}

// parseEC constructs an *ecdsa.PublicKey from crv + x + y.
func parseEC(rk *rawJWK) (*ecdsa.PublicKey, error) {
	if rk.Crv == "" || rk.X == "" || rk.Y == "" {
		return nil, fmt.Errorf("%w: EC missing crv/x/y", ErrJwksParseError)
	}
	var curve elliptic.Curve
	switch rk.Crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("%w: EC crv=%q", ErrJwksUnsupportedCurve, rk.Crv)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(rk.X)
	if err != nil {
		return nil, fmt.Errorf("%w: EC x decode: %v", ErrJwksParseError, err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(rk.Y)
	if err != nil {
		return nil, fmt.Errorf("%w: EC y decode: %v", ErrJwksParseError, err)
	}
	pub := &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}
	if !curve.IsOnCurve(pub.X, pub.Y) {
		return nil, fmt.Errorf("%w: EC point not on curve %s", ErrJwksMalformedKey, rk.Crv)
	}
	return pub, nil
}
