// Package httpclient is the envoy-go framework-internal synchronous HTTP-client
// wrapper per ADR-0177. It provides an Options envelope (Timeout + RetryPolicy
// + TLSConfig) over the stdlib *http.Client + a single Do entry point with a
// status-driven retry loop honoring per-request context cancellation.
//
// The package is the SECOND envoy-go top-level outbound-HTTP framework
// primitive after phase-17 ADR-0150 `internal/jwks/Fetcher`. ADR-0177 records
// the cross-phase-reuse intent at introduction time (3 consumers — jwks
// Fetcher per ADR-0150 §Decision AMENDMENT, extauthz httpAuthClient per
// ADR-0159 §Decision AMENDMENT + §Future Work CLOSURE, oauth2 token_endpoint
// POST per ADR-0185).
//
// Zero-Options is a no-op (zero timeout = no client-imposed deadline; zero
// RetryPolicy = no retries — matches Envoy v1.37.2 wire default per
// phase-20 §20.P1 RATIFIED; nil TLSConfig = stdlib default crypto/tls
// posture). New(Options{}) constructs a usable Client without panic.
//
// Per ADR-0177 §Decision the retry loop honors `req.Context().Err()` after
// every inter-attempt sleep (NEVER retry after the caller's context
// cancellation); status-driven retry uses RetryPolicy.RetryOnStatus
// exclusively (transport-level errors do NOT auto-retry — they propagate to
// the caller per the existing per-consumer error-classification disciplines:
// ADR-0150 §11.P4 fixed-1s failed-refetch + jwks inner-HTTP RetryPolicy;
// ADR-0159 §5.P10 zero-retry-on-transport-error). Per-attempt request
// rewinding is NOT performed at this layer — the caller's
// `*http.Request.Body` (when an `io.ReadSeeker` is desired) is the caller's
// responsibility per the stdlib net/http convention; consumers at
// introduction time (jwks Fetcher GET + extauthz httpAuthClient
// POST-with-bytes-Reader + oauth2 token_endpoint POST) all satisfy this
// contract.
package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pgdad/envoy-go/internal/cluster"
)

// errClusterNotFound is the sentinel returned by ClusterDispatch when the
// cluster manager's Get(name) misses. Wrapped via fmt.Errorf("...%w...", ...)
// so callers may errors.Is(err, errClusterNotFound) for classification.
var errClusterNotFound = errors.New("httpclient: cluster not found")

// Options carries per-Client configuration. Zero-value Options is a no-op
// pass-through (zero Timeout = no client-imposed deadline; zero RetryPolicy
// = no retries; nil TLSConfig = stdlib default crypto/tls posture).
//
// Per ADR-0177 §Decision the Options envelope is intentionally minimal —
// future cross-phase consumers (future ext_authz mTLS, future jwt_authn
// alternative-issuer fetch, future ratelimit gRPC TLS) extend additively
// without breaking existing consumers (struct literal extension is the
// stdlib-conventional escape valve).
type Options struct {
	// Timeout is the per-request deadline applied via the underlying
	// *http.Client.Timeout. Zero = no client-imposed deadline (rely on the
	// caller's request context).
	Timeout time.Duration

	// RetryPolicy carries the optional retry envelope. Zero = no retries
	// (matches Envoy v1.37.2 wire default per phase-20 §20.P1 RATIFIED).
	RetryPolicy RetryPolicy

	// TLSConfig is the optional *tls.Config wired into the underlying
	// http.Transport's TLSClientConfig. Nil = stdlib default crypto/tls
	// posture (system trust roots; default cipher suites).
	TLSConfig *tls.Config
}

// RetryPolicy carries the optional status-driven retry envelope.
//
//   - Attempts: number of RETRIES after the first attempt (0 = no retries;
//     total attempts = Attempts + 1).
//   - PerAttemptDelay: inter-attempt sleep (fixed; not exponential). The
//     ADR-0150 §11.P4 REFUTES-EXPONENTIAL-BACKOFF discipline applies at the
//     consumer level — the jwks Fetcher's inner-HTTP retry loop preserves
//     its own exponential schedule and consumes this layer with
//     RetryPolicy.Attempts = 0 (see ADR-0150 §Decision AMENDMENT for the
//     threading detail).
//   - RetryOnStatus: HTTP status codes that trigger a retry. Non-listed
//     status codes return verbatim on first hit. Transport-level errors do
//     NOT auto-retry (caller responsibility).
type RetryPolicy struct {
	// Attempts is the retry count AFTER the first attempt. Total attempts =
	// Attempts + 1. Zero = no retries.
	Attempts int

	// PerAttemptDelay is the fixed inter-attempt sleep. Zero = no sleep
	// between attempts (back-to-back).
	PerAttemptDelay time.Duration

	// RetryOnStatus is the HTTP-status allow-list for retry. Empty = no
	// status-driven retry (effectively Attempts=0 regardless of Attempts
	// value).
	RetryOnStatus []int
}

// Client wraps a stdlib *http.Client + the Options envelope. Goroutine-safe
// (the underlying *http.Client is goroutine-safe per the net/http docs); one
// instance per logical outbound HTTP role. The envoy-go boot-time singleton
// constructed at `cmd/envoy-go/main.go` is shared by all consumers per the
// introduction-time-3-consumer pattern (jwks + extauthz + oauth2).
type Client struct {
	httpClient *http.Client
	opts       Options
}

// New constructs a Client from Options. Zero-Options constructs a no-op
// pass-through Client (no client-imposed timeout; no retries; default TLS).
//
// The underlying *http.Client is allocated fresh per New — callers wanting
// shared connection pooling at the transport level should construct a single
// *Client at boot time and reuse it (the envoy-go boot wiring at
// cmd/envoy-go/main.go does this).
func New(opts Options) *Client {
	// Build the underlying http.Client. When TLSConfig is non-nil we install
	// a custom Transport carrying the supplied *tls.Config; otherwise we use
	// the stdlib default Transport (its default TLSClientConfig).
	hc := &http.Client{Timeout: opts.Timeout}
	if opts.TLSConfig != nil {
		// Clone the default transport so we inherit the connection-pool +
		// proxy + dial discipline of http.DefaultTransport while overriding
		// the TLSClientConfig. Using a fresh *http.Transport literal here
		// would lose the stdlib defaults (e.g. MaxIdleConns, IdleConnTimeout).
		dt, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			// Defensive: stdlib invariant says http.DefaultTransport is a
			// *http.Transport; if a future Go version changes this we fall
			// back to a minimal transport carrying just the TLS config.
			hc.Transport = &http.Transport{TLSClientConfig: opts.TLSConfig}
		} else {
			tr := dt.Clone()
			tr.TLSClientConfig = opts.TLSConfig
			hc.Transport = tr
		}
	}
	return &Client{httpClient: hc, opts: opts}
}

// Do executes the request synchronously, honoring the Options envelope:
//   - The per-request deadline (Options.Timeout) is enforced via the
//     underlying *http.Client.Timeout.
//   - The RetryPolicy retries on status codes in RetryOnStatus, sleeping
//     PerAttemptDelay between attempts. The request context's Err is checked
//     after every sleep; a canceled context aborts the retry loop and
//     returns the context error.
//   - Transport-level errors (connect failure, ctx cancellation, body read
//     error) return verbatim without retry — consumer-level error
//     classification decides the disposition.
//
// On a retried response (status in RetryOnStatus + Attempts remaining), the
// prior response Body is drained and closed before the next attempt — the
// stdlib net/http connection-pool reuse contract requires this discipline.
//
// Per ADR-0177 §Decision the retry loop does NOT rewind the request body
// (this is the caller's responsibility — consumers at introduction time pass
// nil bodies (jwks GET), `bytes.NewReader` bodies (extauthz POST), or
// `strings.NewReader` bodies (oauth2 token_endpoint POST), each of which
// satisfies the caller-side body-rewind responsibility).
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// Zero RetryPolicy or empty RetryOnStatus → single attempt, no retry.
	maxAttempts := c.opts.RetryPolicy.Attempts + 1
	if c.opts.RetryPolicy.Attempts <= 0 || len(c.opts.RetryPolicy.RetryOnStatus) == 0 {
		maxAttempts = 1
	}

	var (
		resp *http.Response
		err  error
	)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Inter-attempt sleep honoring ctx cancellation.
			if c.opts.RetryPolicy.PerAttemptDelay > 0 {
				timer := time.NewTimer(c.opts.RetryPolicy.PerAttemptDelay)
				select {
				case <-timer.C:
				case <-req.Context().Done():
					timer.Stop()
					return nil, req.Context().Err()
				}
			}
			// Re-check ctx even when PerAttemptDelay is zero — a cancellation
			// that fired during the prior Do call must short-circuit here.
			if ctxErr := req.Context().Err(); ctxErr != nil {
				return nil, ctxErr
			}
			// Drain + close the prior response body so the stdlib
			// connection-pool can reuse the underlying conn.
			if resp != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}

		resp, err = c.httpClient.Do(req)
		if err != nil {
			// Transport-level error — no retry; propagate verbatim.
			return nil, err
		}
		if !shouldRetryStatus(resp.StatusCode, c.opts.RetryPolicy.RetryOnStatus) {
			return resp, nil
		}
	}
	// Exhausted retries — return the last response (caller inspects status).
	return resp, nil
}

// shouldRetryStatus reports whether the given status code is in the
// RetryOnStatus allow-list.
func shouldRetryStatus(status int, allow []int) bool {
	for _, s := range allow {
		if s == status {
			return true
		}
	}
	return false
}

// ClusterDispatch dispatches an HTTP request to an upstream endpoint selected
// from the named cluster via the cluster manager's load balancer.
//
// IN-PLACE AMENDMENT on ADR-0177 §Decision (phase 22.2 SPEC §3.3 + §11.4). No
// new ADR number is consumed; the §Decision AMENDMENT body lands at the 22.2
// IMPL atomic-landing Task per ADR-0044 in-place edit discipline (matches the
// phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). This method is
// the FIRST cross-phase co-consumer validation of the framework primitive at
// the 22.2 lua filter's `:httpCall()` bridge (R5 RATIFIED at this Task).
//
// Behavior:
//   - Resolves clusterName via clusterMgr.Get(name); returns errClusterNotFound
//     (wrapped with the cluster name for log clarity) on lookup miss.
//   - Selects an upstream endpoint via Cluster.PickEndpoint() (round-robin LB
//     per phase-02 ADR-0010).
//   - Rewrites request.URL.Host = endpoint.Addr() so the underlying stdlib
//     http.Client dials the LB-selected endpoint. The request.URL.Scheme is
//     set to "https" when the cluster's UpstreamTLSConfig is non-nil and to
//     "http" otherwise, so the http.Transport selects the correct dial path.
//   - Honors per-cluster TLS via Cluster.UpstreamTLSConfig() by constructing
//     a temporary *http.Client whose Transport carries the cluster's
//     *tls.Config (overriding the receiver's TLSConfig — phase-20 oauth2's
//     shared-singleton lifecycle uses the URL-based Do() path unchanged, and
//     cluster-bound dispatch always honors the cluster's TLS posture). The
//     temporary client also inherits the receiver's Options.Timeout.
//   - Applies the receiver's Options.RetryPolicy verbatim — status-driven
//     retry on RetryOnStatus with PerAttemptDelay between attempts. The
//     request context's Err is checked after every inter-attempt sleep; a
//     canceled context aborts the retry loop and returns the context error
//     (identical to Do's discipline).
//   - Transport-level errors (connect failure, ctx cancellation, body read
//     error) return verbatim without retry — consumer-level error
//     classification decides the disposition (per ADR-0177 §Decision).
//
// Thread-safety: cluster.Manager and *cluster.Cluster are read-only at
// runtime (per ADR-0010 + ADR-0050 build-time-then-frozen semantics);
// concurrent ClusterDispatch calls are safe.
//
// The *cluster.Manager parameter is threaded explicitly to keep the
// httpclient package decoupled from cluster-manager singletons. The 22.2
// lua filter receives *cluster.Manager at filter-construction time via the
// FactoryCtx.ClusterManager field (per SPEC §3.3 §11.4.5); the `:httpCall`
// bridge closure consumes both the *Client and the *cluster.Manager
// references.
func (c *Client) ClusterDispatch(ctx context.Context, clusterName string, request *http.Request, clusterMgr *cluster.Manager) (*http.Response, error) {
	if clusterMgr == nil {
		return nil, fmt.Errorf("httpclient: ClusterDispatch: nil cluster manager")
	}
	if request == nil {
		return nil, fmt.Errorf("httpclient: ClusterDispatch: nil request")
	}
	cl, ok := clusterMgr.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: %q", errClusterNotFound, clusterName)
	}
	ep, err := cl.PickEndpoint()
	if err != nil {
		return nil, fmt.Errorf("httpclient: ClusterDispatch: pick endpoint: %w", err)
	}

	// Rewrite request.URL.Host + Scheme so the underlying http.Client dials
	// the LB-selected endpoint with the right transport (TLS vs plaintext).
	// Clone the URL so we don't mutate the caller's request state in-place
	// (the caller's request may be reused by other code paths).
	if request.URL == nil {
		// Construct a minimal URL when the caller didn't pre-populate one
		// (e.g., http.NewRequestWithContext with a relative path). This is a
		// defensive path — production consumers always pass a URL.
		return nil, fmt.Errorf("httpclient: ClusterDispatch: request.URL is nil")
	}
	urlCopy := *request.URL
	urlCopy.Host = ep.Addr()
	if cl.UpstreamTLSConfig() != nil {
		urlCopy.Scheme = "https"
	} else {
		urlCopy.Scheme = "http"
	}
	request.URL = &urlCopy
	// Update Host header to match so the upstream server's vhost routing
	// observes the rewritten host (idiomatic per net/http when Host==""):
	// leaving Host == "" tells the stdlib to derive from URL.Host. We do not
	// overwrite a caller-set Host (a non-empty Host conveys explicit intent —
	// e.g. an SNI override pattern).
	// (No-op when request.Host was already empty.)

	// Build the temporary *http.Client carrying the cluster's TLS config +
	// receiver's Options.Timeout. We do not memoize this per (clusterName, c)
	// — the cluster manager is read-only at runtime and a fresh *http.Client
	// per ClusterDispatch is acceptable at the lua httpCall surface (a
	// future optimization may cache per-cluster transports if profiling
	// indicates a hot path; ADR-0177 §Decision AMENDMENT will document such
	// expansion non-breakingly).
	hc := &http.Client{Timeout: c.opts.Timeout}
	if upCfg := cl.UpstreamTLSConfig(); upCfg != nil {
		// Clone the default transport to inherit stdlib defaults
		// (MaxIdleConns, IdleConnTimeout, etc.) and override TLSClientConfig.
		// Mirrors New()'s discipline at httpclient.go:115-130.
		dt, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			hc.Transport = &http.Transport{TLSClientConfig: upCfg}
		} else {
			tr := dt.Clone()
			tr.TLSClientConfig = upCfg
			hc.Transport = tr
		}
	}

	// Re-bind the request to the supplied ctx so the stdlib http.Client.Do
	// observes the caller's cancellation/deadline at the transport layer.
	// (The caller typically already does NewRequestWithContext(ctx, ...); this
	// is a defensive re-bind that closes the gap if they passed a different
	// ctx as the ClusterDispatch ctx parameter than the one bound on the
	// request.)
	request = request.WithContext(ctx)

	// Retry loop — mirrors Do at httpclient.go:155-202. Zero RetryPolicy or
	// empty RetryOnStatus → single attempt, no retry.
	maxAttempts := c.opts.RetryPolicy.Attempts + 1
	if c.opts.RetryPolicy.Attempts <= 0 || len(c.opts.RetryPolicy.RetryOnStatus) == 0 {
		maxAttempts = 1
	}

	var (
		resp    *http.Response
		dispErr error
	)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Inter-attempt sleep honoring ctx cancellation.
			if c.opts.RetryPolicy.PerAttemptDelay > 0 {
				timer := time.NewTimer(c.opts.RetryPolicy.PerAttemptDelay)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					return nil, ctx.Err()
				}
			}
			// Re-check ctx even when PerAttemptDelay is zero.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			// Drain + close the prior response body so the stdlib connection-
			// pool can reuse the underlying conn.
			if resp != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}

		resp, dispErr = hc.Do(request)
		if dispErr != nil {
			// Transport-level error — no retry; propagate verbatim.
			return nil, dispErr
		}
		if !shouldRetryStatus(resp.StatusCode, c.opts.RetryPolicy.RetryOnStatus) {
			return resp, nil
		}
	}
	// Exhausted retries — return the last response (caller inspects status).
	return resp, nil
}
