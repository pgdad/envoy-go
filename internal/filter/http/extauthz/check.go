package extauthz

// check.go — HTTP-outbound auth-check primitive per ADR-0159 disposition (b).
//
// This file lands the thin ext_authz-local HTTP client — an `httpAuthClient`
// type wrapping `*http.Client` + the configured `HttpService.server_uri.timeout`
// + `path_prefix` — and the `checkFn` closure that performs the outbound POST
// and maps the HTTP response to a `checkDisposition`.
//
// Design: disposition (b) per SPEC §3.1 + ADR-0159 — a thin local client that
// mirrors the phase-17 `internal/jwks/Fetcher` outbound-HTTP `http.Client`/
// timeout discipline WITHOUT the cache/async-refresh machinery. The two consumers
// have structurally different lifecycles; generalizing into `internal/httpclient/`
// is deferred to the THIRD outbound-HTTP consumer (a future `oauth2` phase).
//
// §5.P10 error-classification boundary:
//   - HTTP 200              → dispAllow
//   - HTTP 401 or 403       → dispDeny  (the recognized deny-status set)
//   - connect failure /
//     timeout / ctx.Err /
//     unrecognized status   → dispError
//
// Zero retry per planner-time decision D2: `HttpService` has no retry-policy
// proto field; a connect failure / timeout maps directly to dispError.
//
// The request-side header filtering (buildAuthRequest / compileStringMatcherList)
// is STUBBED at Task 3 — the closure uses the authRequest headers as-is.
// The allowed_upstream_headers / allowed_client_headers extraction is STUBBED
// at Task 3 — the disposition header fields are populated minimally.
// Real extraction + validate_mutations gating land at Task 5.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ext_authzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// httpAuthClient is the thin ext_authz-local HTTP client per ADR-0159
// disposition (b). It wraps an `*http.Client` configured with the
// `HttpService.server_uri.timeout` + the parsed `path_prefix`.
//
// It does NOT carry:
//   - a cache (unlike `internal/jwks/Fetcher`)
//   - a background-refresh goroutine (unlike `internal/jwks/Fetcher`)
//   - a retry policy (zero retry per D2)
//   - connection-management state (stateless per-request)
//
// The `httpAuthClient` is allocated once at `buildHTTPCheckFn` time (config-load
// time) and is safely shared across all per-stream `checkFn` invocations (the
// `*http.Client` is goroutine-safe per the net/http documentation).
type httpAuthClient struct {
	client     *http.Client
	baseURL    string // the parsed base URL from server_uri.uri (scheme + host)
	pathPrefix string // the path_prefix string (prepended to each request path)
}

// buildHTTPCheckFn constructs the HTTP-mode checkFn per SPEC §6.5 + ADR-0159.
//
// Steps per SPEC §6.5:
//  1. Validate server_uri set + non-empty uri (PGV-mirror — HttpService.server_uri
//     is NOT PGV-required; the factory rejects an empty one).
//  2. Construct the httpAuthClient: &http.Client{Timeout: hs.server_uri.timeout}
//     (zero timeout = no timeout; ZERO retry per D2).
//  3. Compile authorization_response matchers (STUBBED at Task 3 — nil).
//  4. Return the checkFn closure.
//
// Signature note: the PLAN's nominal signature is
//
//	buildHTTPCheckFn(hs *ext_authzv3.HttpService, ar *authRequestCfg, validateMutations bool)
//
// At Task 3, the `authRequestCfg` type is not yet introduced (Task 4 lands
// buildAuthRequest); the closure uses the authRequest headers directly
// (STUBBED). The signature used here takes only `hs *ext_authzv3.HttpService`
// — matching the existing `buildCompiledConfig` call-site in extauthz.go.
// The PLAN note confirms this is acceptable: "check what Task 2's
// buildCompiledConfig currently passes to the stub buildHTTPCheckFn".
// This deviation is documented in PROGRESS.md Task 3.
func buildHTTPCheckFn(hs *ext_authzv3.HttpService) (checkFn, error) {
	// 1. Validate server_uri.uri (PGV-mirror).
	if hs == nil || hs.GetServerUri() == nil || hs.GetServerUri().GetUri() == "" {
		return nil, errors.New("ext_authz: http_service.server_uri.uri is required")
	}

	uri := hs.GetServerUri().GetUri()

	// 2. Construct the httpAuthClient.
	//    Timeout: use the proto durationpb.Duration if set; zero = no timeout.
	timeout := durationpbToGo(hs.GetServerUri().GetTimeout())
	hac := &httpAuthClient{
		client:     &http.Client{Timeout: timeout},
		baseURL:    stripPath(uri),
		pathPrefix: hs.GetPathPrefix(),
	}

	// 3. Compile authorization_response matcher fields (STUBBED at Task 3).
	//    Real compilation of allowed_upstream_headers /
	//    allowed_upstream_headers_to_append / allowed_client_headers lands at
	//    Task 5 (compileStringMatcherList).
	//    At Task 3: these are all nil (no filtering — headers are passed through
	//    minimally or left empty in the disposition).
	//    _ = hs.GetAuthorizationResponse()  // parsed but not compiled at Task 3

	// 4. Return the checkFn closure.
	return buildCheckFnClosure(hac), nil
}

// buildCheckFnClosure returns the checkFn closure for the given httpAuthClient.
// Separated from buildHTTPCheckFn for testability.
//
// The closure:
//  1. Builds the outbound POST URL: hac.baseURL (pre-stripped at build time) +
//     path_prefix + request path. stripPath runs exactly once per checkFn
//     lifetime (at buildHTTPCheckFn time), not per request.
//  2. Creates the POST request with the authRequest headers + optional body.
//  3. Calls client.Do(req.WithContext(ctx)).
//  4. Maps the HTTP response → checkDisposition per §5.P10:
//     200 → dispAllow; 401|403 → dispDeny; anything else → dispError.
func buildCheckFnClosure(hac *httpAuthClient) checkFn {
	return func(ctx context.Context, req *authRequest) (checkDisposition, error) {
		// Build the outbound POST target URL.
		// hac.baseURL is the pre-stripped scheme+host (computed once at
		// buildHTTPCheckFn time). path_prefix is prepended to the authRequest
		// path per SPEC §6.5.
		targetURL := hac.baseURL + joinPaths(hac.pathPrefix, req.path)

		// Create the HTTP request body (nil when req.body is empty per SPEC §6.5).
		var bodyReader io.Reader
		if len(req.body) > 0 {
			bodyReader = bytes.NewReader(req.body)
		}

		// Build the outbound POST request.
		outReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bodyReader)
		if err != nil {
			return checkDisposition{class: dispError}, fmt.Errorf("ext_authz: build request: %w", err)
		}

		// Copy the authRequest headers (request-side-filtered headers from Task 4;
		// at Task 3 these are the authRequest headers as-is).
		for name, values := range req.headers {
			for _, v := range values {
				outReq.Header.Add(name, v)
			}
		}

		// Execute the outbound POST.
		resp, err := hac.client.Do(outReq)
		if err != nil {
			// Connect failure / timeout / context-cancelled.
			// §5.P10: transport/timeout/connect failure → dispError.
			return checkDisposition{class: dispError}, fmt.Errorf("ext_authz: auth check: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Map the HTTP response status to a disposition per §5.P10.
		return mapHTTPResponse(resp)
	}
}

// mapHTTPResponse maps an HTTP response from the auth service to a
// checkDisposition per the §5.P10 error-classification boundary:
//
//	200         → dispAllow
//	401 | 403   → dispDeny   (the recognized deny-status set per parent SPEC §5.P10)
//	anything else → dispError (unrecognized status)
//
// Header extraction (allowed_upstream_headers / allowed_client_headers) is
// STUBBED at Task 3 — the disposition header fields are populated minimally.
// Real extraction + validate_mutations gating land at Task 5.
func mapHTTPResponse(resp *http.Response) (checkDisposition, error) {
	switch resp.StatusCode {
	case http.StatusOK: // 200
		// Allow path: minimal stub — no header extraction at Task 3.
		// Real allowed_upstream_headers / allowed_upstream_headers_to_append
		// extraction lands at Task 5.
		return checkDisposition{
			class:       dispAllow,
			upstreamSet: nil, // STUBBED — Task 5 extracts allowed_upstream_headers
			upstreamApp: nil, // STUBBED — Task 5 extracts allowed_upstream_headers_to_append
		}, nil

	case http.StatusUnauthorized, http.StatusForbidden: // 401, 403
		// Deny path: read the response body verbatim per §5.P11.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			// IO error reading deny body — treat as error per §5.P10.
			return checkDisposition{class: dispError},
				fmt.Errorf("ext_authz: read deny body: %w", err)
		}
		// Header extraction (allowed_client_headers) is STUBBED at Task 3.
		// Real allowed_client_headers-filtered extraction + text/plain fallback
		// lands at Task 5. Minimally: no denyHeaders populated at Task 3.
		return checkDisposition{
			class:       dispDeny,
			denyStatus:  uint32(resp.StatusCode),
			denyBody:    body,
			denyHeaders: nil, // STUBBED — Task 5 extracts allowed_client_headers
		}, nil

	default:
		// Unrecognized status → dispError per §5.P10.
		return checkDisposition{class: dispError},
			fmt.Errorf("ext_authz: unrecognized auth response status %d", resp.StatusCode)
	}
}

// buildTargetURL constructs the outbound POST target URL by combining a
// pre-stripped base URL, the path_prefix, and the request path. The
// path_prefix is prepended to the request path per SPEC §6.5 + §18.P4.
//
// base must already have its path component stripped (i.e. scheme+host only,
// as returned by stripPath). This function is used by unit tests for the
// path-joining surface; the live closure uses hac.baseURL + joinPaths directly.
//
// Rules:
//   - base is the scheme+host (e.g. "http://auth.example.com:9191").
//   - The path_prefix is prepended to path to form the full outbound path.
//   - Double-slash is avoided (path_prefix trailing slash + path leading slash).
//
// Example: base="http://auth:9191", pathPrefix="/auth-prefix", path="/api"
// → "http://auth:9191/auth-prefix/api"
func buildTargetURL(base, pathPrefix, path string) string {
	// Build the outbound path: path_prefix + request path.
	// Avoid double-slash between prefix and path.
	outPath := joinPaths(pathPrefix, path)

	return base + outPath
}

// stripPath returns the scheme+host portion of a URI (dropping any path).
// E.g. "http://auth.example.com:9191/some/path" → "http://auth.example.com:9191".
// If the URI has no path separator after the scheme+host, it is returned as-is.
func stripPath(uri string) string {
	// Find scheme separator "://"
	schemeEnd := strings.Index(uri, "://")
	if schemeEnd < 0 {
		return uri
	}
	afterScheme := uri[schemeEnd+3:]
	// Find the first "/" in the host part.
	slashIdx := strings.Index(afterScheme, "/")
	if slashIdx < 0 {
		return uri // no path; return as-is
	}
	return uri[:schemeEnd+3+slashIdx]
}

// joinPaths joins a path prefix and a path, avoiding double slashes.
// E.g. joinPaths("/auth", "/api") → "/auth/api"
//
//	joinPaths("", "/api") → "/api"
//	joinPaths("/auth", "") → "/auth"
func joinPaths(prefix, path string) string {
	if prefix == "" {
		return path
	}
	if path == "" {
		return prefix
	}
	// Avoid double slash: if prefix ends with "/" and path starts with "/",
	// strip the leading "/" from path.
	if strings.HasSuffix(prefix, "/") && strings.HasPrefix(path, "/") {
		return prefix + path[1:]
	}
	// If neither has the separator, add it.
	if !strings.HasSuffix(prefix, "/") && !strings.HasPrefix(path, "/") {
		return prefix + "/" + path
	}
	return prefix + path
}

// durationpbToGo converts a *durationpb.Duration to a time.Duration.
// Returns 0 (no timeout) when d is nil.
func durationpbToGo(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}
