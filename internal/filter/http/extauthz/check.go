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
// Request-side header filtering — call-site boundary (Task 4 review-fix):
//   buildAuthRequest (attributes.go, Task 4) performs the request-side header
//   filtering — allowed_headers / disallowed_headers / headers_to_add. It is
//   NOT called from this file: buildAuthRequest needs the per-stream *filter
//   (carrying the per-route-resolved f.activeRC) AND the real client request
//   headers from dcb.RequestHeaders(), neither of which exists at config-load
//   time inside buildHTTPCheckFn nor inside the mode-agnostic checkFn closure
//   (which by contract RECEIVES an already-built *authRequest). buildAuthRequest
//   is therefore called at the DecodeHeaders dispatch site (Task 9, ADR-0159) —
//   the only place with both f and the raw headers. The closure below correctly
//   just transmits the *authRequest it is handed. See the PROGRESS.md Task 4
//   entry "Deviation from PLAN Step 4" note and ADR-0160 §Decision (v)/(vii).
//
// Authorization-response header extraction (Task 5 / ADR-0161):
//   buildHTTPCheckFn compiles the authorization_response matcher triple at
//   config-load time:
//     - allowedUpstreamHeaders    → upstreamSet  (overwrite per allow path)
//     - allowedUpstreamHeadersApp → upstreamApp  (append per allow path)
//     - allowedClientHeaders      → denyHeaders  (filtered per deny path)
//   validate_mutations (bool) is threaded into the closure — when true, the
//   extracted header sets are validated by validateMutationHeaders; a violation
//   drives dispInvalid.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/grpcclient"
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
// Steps per SPEC §6.5 (updated at Task 5 per ADR-0161):
//  1. Validate server_uri set + non-empty uri (PGV-mirror — HttpService.server_uri
//     is NOT PGV-required; the factory rejects an empty one).
//  2. Construct the httpAuthClient: &http.Client{Timeout: hs.server_uri.timeout}
//     (zero timeout = no timeout; ZERO retry per D2).
//  3. Compile authorization_response matchers:
//     - allowed_upstream_headers     → allowedUpstream (set/overwrite)
//     - allowed_upstream_headers_to_append → allowedUpstreamApp (append)
//     - allowed_client_headers       → allowedClient (deny-path filter)
//  4. Return the checkFn closure capturing the matchers + validateMutations.
//
// Signature note: updated at Task 5 to accept validateMutations bool (threaded
// into the closure from compiledConfig.validateMutations parsed at config-load
// time in buildCompiledConfig). The Task 3 signature `(hs) (checkFn, error)` is
// extended to `(hs, validateMutations) (checkFn, error)` — minimal change.
func buildHTTPCheckFn(hs *ext_authzv3.HttpService, validateMutations bool) (checkFn, error) {
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

	// 3. Compile authorization_response matchers per SPEC §6.5 + ADR-0161.
	//    Each ListStringMatcher → *stringMatcherList via compileStringMatcherList
	//    (attributes.go); nil input → nil return (no filtering).
	ar := hs.GetAuthorizationResponse()
	allowedUpstream, err := compileStringMatcherList(ar.GetAllowedUpstreamHeaders())
	if err != nil {
		return nil, fmt.Errorf("ext_authz: authorization_response.allowed_upstream_headers: %w", err)
	}
	allowedUpstreamApp, err := compileStringMatcherList(ar.GetAllowedUpstreamHeadersToAppend())
	if err != nil {
		return nil, fmt.Errorf("ext_authz: authorization_response.allowed_upstream_headers_to_append: %w", err)
	}
	allowedClient, err := compileStringMatcherList(ar.GetAllowedClientHeaders())
	if err != nil {
		return nil, fmt.Errorf("ext_authz: authorization_response.allowed_client_headers: %w", err)
	}

	// 4. Return the checkFn closure capturing the matchers + validateMutations.
	return buildCheckFnClosure(hac, allowedUpstream, allowedUpstreamApp, allowedClient, validateMutations), nil
}

// buildCheckFnClosure returns the checkFn closure for the given httpAuthClient
// and authorization-response matchers. Separated from buildHTTPCheckFn for testability.
//
// Parameters (all captured at config-load time):
//   - hac: the thin ext_authz-local HTTP client + base URL + path_prefix.
//   - allowedUpstream: compiled allowed_upstream_headers (set/overwrite per allow path).
//   - allowedUpstreamApp: compiled allowed_upstream_headers_to_append (append per allow path).
//   - allowedClient: compiled allowed_client_headers (deny-path filter).
//   - validateMutations: when true, run validateMutationHeaders over extracted header sets.
//
// The closure:
//  1. Builds the outbound POST URL.
//  2. Creates the POST request with authRequest headers + optional body.
//  3. Calls client.Do(req.WithContext(ctx)).
//  4. Maps the HTTP response → checkDisposition per §5.P10 + ADR-0161:
//     200 → dispAllow (extract upstreamSet/upstreamApp); 401|403 → dispDeny
//     (extract denyHeaders + text/plain fallback); else → dispError.
//  5. If validateMutations: run validateMutationHeaders over the extracted
//     header sets — a violation drives dispInvalid.
func buildCheckFnClosure(hac *httpAuthClient, allowedUpstream, allowedUpstreamApp, allowedClient *stringMatcherList, validateMutations bool) checkFn {
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

		// Copy the authRequest headers. req is an already-built *authRequest:
		// when Task 9 wires DecodeHeaders, req.headers are the request-side-
		// filtered headers produced by buildAuthRequest (attributes.go); the
		// closure faithfully transmits whatever it is handed (it does NOT itself
		// run the allowed_headers/disallowed_headers filtering — see the
		// call-site-boundary note in this file's header comment).
		for name, values := range req.headers {
			for _, v := range values {
				outReq.Header.Add(name, v)
			}
		}

		// Execute the outbound POST.
		resp, err := hac.client.Do(outReq)
		if err != nil {
			// Connect failure / timeout / context-canceled.
			// §5.P10: transport/timeout/connect failure → dispError.
			return checkDisposition{class: dispError}, fmt.Errorf("ext_authz: auth check: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Map the HTTP response status to a disposition per §5.P10 + ADR-0161.
		return mapHTTPResponseWithMatchers(resp, allowedUpstream, allowedUpstreamApp, allowedClient, validateMutations)
	}
}

// mapHTTPResponseWithMatchers maps an HTTP response from the auth service to a
// checkDisposition per the §5.P10 error-classification boundary + ADR-0161
// bidirectional header-mutation discipline:
//
//	200           → dispAllow  (extract upstreamSet/upstreamApp via matchers)
//	401 | 403     → dispDeny   (extract denyHeaders via allowedClient; text/plain fallback)
//	anything else → dispError  (unrecognized status)
//
// Parameters:
//   - allowedUpstream: nil = no upstream set headers; non-nil = extract matching response headers.
//   - allowedUpstreamApp: nil = no upstream append headers; non-nil = extract matching.
//   - allowedClient: nil = no client headers; non-nil = extract matching response headers.
//   - validateMutations: when true, run validateMutationHeaders on the extracted sets;
//     a violation drives dispInvalid (no error returned — the caller increments the invalid counter).
//
// Deny-path header-ordering per §5.P11 RATIFIED-PENDING-IMPL-TIME + SPEC §4:
// decision headers (from allowedClient) appear FIRST; the text/plain fallback
// content-type is appended AFTER (housekeeping last). The §18.P11 byte-shape
// confirmation closes at Task 13 fixture-harness diff.
func mapHTTPResponseWithMatchers(
	resp *http.Response,
	allowedUpstream, allowedUpstreamApp, allowedClient *stringMatcherList,
	validateMutations bool,
) (checkDisposition, error) {
	switch resp.StatusCode {
	case http.StatusOK: // 200
		// Allow path: extract response headers matching allowed_upstream_headers
		// (set/overwrite) and allowed_upstream_headers_to_append (append) per ADR-0161.
		upstreamSet := extractMatchingHeaders(resp.Header, allowedUpstream)
		upstreamApp := extractMatchingHeaders(resp.Header, allowedUpstreamApp)

		// validate_mutations gate: run validateMutationHeaders if configured.
		// A rejection drives dispInvalid (treated as error posture per SPEC §6.3).
		if validateMutations {
			if err := validateMutationHeaders(upstreamSet); err != nil {
				return checkDisposition{class: dispInvalid}, nil
			}
			if err := validateMutationHeaders(upstreamApp); err != nil {
				return checkDisposition{class: dispInvalid}, nil
			}
		}

		return checkDisposition{
			class:       dispAllow,
			upstreamSet: upstreamSet,
			upstreamApp: upstreamApp,
		}, nil

	case http.StatusUnauthorized, http.StatusForbidden: // 401, 403
		// Deny path: read the response body verbatim per §5.P11.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			// IO error reading deny body — treat as error per §5.P10.
			return checkDisposition{class: dispError},
				fmt.Errorf("ext_authz: read deny body: %w", err)
		}

		// Extract denyHeaders: decision headers first, then text/plain fallback
		// per SPEC §4 + parent §5.P11 + ADR-0161.
		denyHeaders := buildDenyHeaders(resp.Header, allowedClient)

		// validate_mutations gate over deny-path headers.
		if validateMutations {
			if err := validateMutationHeaders(denyHeaders); err != nil {
				return checkDisposition{class: dispInvalid}, nil
			}
		}

		return checkDisposition{
			class:       dispDeny,
			denyStatus:  uint32(resp.StatusCode),
			denyBody:    body,
			denyHeaders: denyHeaders,
		}, nil

	default:
		// Unrecognized status → dispError per §5.P10.
		return checkDisposition{class: dispError},
			fmt.Errorf("ext_authz: unrecognized auth response status %d", resp.StatusCode)
	}
}

// extractMatchingHeaders extracts response headers that match the given
// stringMatcherList into a []headerKV slice (decision-order: the order the
// net/http response headers map iterates — Go maps are unordered; for the
// allow-path upstream injection the ordering is not user-visible). A nil
// matcher returns nil (no headers extracted). Header names are lowercased
// for matching (per Envoy's internal lowercase-header convention); the
// canonical form is stored in the headerKV for consumption by the HCM chain.
func extractMatchingHeaders(headers http.Header, sml *stringMatcherList) []headerKV {
	if sml == nil {
		return nil
	}
	var result []headerKV
	for name, values := range headers {
		nameLower := strings.ToLower(name)
		if sml.matchAny(nameLower) {
			// Emit one headerKV per value (multi-value headers produce multiple entries).
			for _, v := range values {
				result = append(result, headerKV{name: nameLower, value: v})
			}
		}
	}
	return result
}

// buildDenyHeaders constructs the deny-path header set per SPEC §4 + parent §5.P11
// + ADR-0161:
//
//  1. Extract auth-response headers matching allowedClient (decision headers).
//  2. If content-type is not already present in the extracted set, append a
//     "content-type: text/plain" fallback entry.
//
// Decision headers appear FIRST; the housekeeping content-type fallback is LAST.
// Header names are lowercased for matching and storage (Envoy convention).
func buildDenyHeaders(headers http.Header, allowedClient *stringMatcherList) []headerKV {
	// Step 1: extract decision headers (allowedClient-filtered).
	decisionHeaders := extractMatchingHeaders(headers, allowedClient)

	// Step 2: check if content-type was supplied in the allowed set.
	hasContentType := false
	for _, kv := range decisionHeaders {
		if kv.name == "content-type" {
			hasContentType = true
			break
		}
	}

	// Append text/plain fallback if content-type was NOT supplied by the auth service
	// in the allowed set per SPEC §4 + parent §5.P11 RATIFIED.
	if !hasContentType {
		decisionHeaders = append(decisionHeaders, headerKV{name: "content-type", value: "text/plain"})
	}

	return decisionHeaders
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

// ---------------------------------------------------------------------------
// buildGRPCCheckFn — gRPC-mode checkFn constructor per SPEC §6.5 + ADR-0157
// §Decision AMENDMENT (Task 3 WIRE-UP; body lands at Task 5/6).
// ---------------------------------------------------------------------------

// buildGRPCCheckFn constructs the gRPC-mode checkFn per SPEC §6.5.
//
// Task 3 (this stub): returns sentinel `errors.New("ext_authz: grpc_service:
// TODO (Task 5)")`. The 18.1 PARSE-REJECT call-site in `buildCompiledConfig`
// is rewired to invoke this constructor — flipping the
// `*ExtAuthz_GrpcService` arm from PARSE-REJECT-with-18.1-wording to a
// real-but-stubbed dispatch. The sentinel makes the existing 18.1 Group 1
// PARSE-REJECT tests fail with the NEW wording at this task; Task 5 lands
// the real body and updates those tests.
//
// Task 5 real body (per SPEC §6.5):
//  1. `GoogleGrpc` arm PARSE-REJECT envoy-go-strict
//     (`"ext_authz: grpc_service: google_grpc arm not supported"`).
//  2. `EnvoyGrpc.cluster_name` PGV-mirror (`min_len: 1`) — PARSE-REJECT empty.
//  3. Cluster-manager lookup via `ctx.ClusterManager.Get(cluster_name)` —
//     PARSE-REJECT on unknown / `!UseH2()`.
//  4. Construct `*grpcclient.AuthClient` via `grpcclient.New(mgr)` +
//     `grpcclient.NewAuthClient(d, name, durationpbToGo(gs.Timeout))`.
//  5. `initial_metadata` + `retry_policy` SILENT-IGNORED per SPEC §2.6 +
//     §8 items 2+3.
//  6. Return the closure body per Task 5/6 (buildAttributeContext +
//     mapGRPCResponse).
//
// Signature (PLAN Task 3 §Steps):
//
//	buildGRPCCheckFn(
//	    gs *corev3.GrpcService,
//	    ctx envoyhttp.FactoryCtx,
//	    validateMutations bool,
//	    includePeerCert bool,
//	    includeTlsSession bool,
//	    encodeRawHeaders bool,
//	    packAsBytes bool,
//	) (checkFn, error)
//
// The six trailing boolean gates + `validateMutations` are captured INTO the
// closure at Task 5 — the gRPC-mode-specific config is captured in closure
// lexical scope, NOT promoted to `compiledConfig` struct fields per SPEC §6.5
// step 5.
//
// Task 6 real body — per SPEC §6.5 + ADR-0161 gRPC-mode portion. See the §body
// comment block above for the 6-step sequence.
func buildGRPCCheckFn(
	gs *corev3.GrpcService,
	ctx envoyhttp.FactoryCtx,
	validateMutations bool,
	includePeerCert bool,
	includeTlsSession bool,
	encodeRawHeaders bool,
	packAsBytes bool,
) (checkFn, error) {
	if gs == nil {
		return nil, errors.New("ext_authz: grpc_service is required")
	}

	// Step 1: GoogleGrpc arm PARSE-REJECT envoy-go-strict per SPEC §6.5 +
	// ADR-0157 §Decision AMENDMENT (Task 3). envoy-go uses
	// google.golang.org/grpc directly; the GoogleGrpc native-channel arm is
	// not supported. The error wording mirrors the V3-only ADR-0008 discipline
	// at the GrpcService level.
	if _, isGoogle := gs.GetTargetSpecifier().(*corev3.GrpcService_GoogleGrpc_); isGoogle {
		return nil, errors.New("ext_authz: grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)")
	}

	// Step 2: EnvoyGrpc.cluster_name PGV-mirror (`min_len: 1`). PARSE-REJECT
	// empty / missing per SPEC §6.5.
	envoyGrpc := gs.GetEnvoyGrpc()
	if envoyGrpc == nil {
		return nil, errors.New("ext_authz: grpc_service: envoy_grpc arm required (target_specifier must be set)")
	}
	clusterName := envoyGrpc.GetClusterName()
	if clusterName == "" {
		return nil, errors.New("ext_authz: grpc_service: envoy_grpc.cluster_name must be non-empty")
	}

	// Step 3: Cluster-manager lookup + UseH2 gate per SPEC §6.5 + the §11.P13
	// in-session SPEC scrape (gRPC requires HTTP/2 framing).
	if ctx.ClusterManager == nil {
		return nil, errors.New("ext_authz: grpc_service: cluster manager not available (FactoryCtx.ClusterManager is nil)")
	}
	clu, ok := ctx.ClusterManager.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("ext_authz: grpc_service: unknown cluster %q", clusterName)
	}
	if !clu.UseH2() {
		return nil, fmt.Errorf("ext_authz: grpc_service: cluster %q must have http2_protocol_options{} set (gRPC requires HTTP/2)", clusterName)
	}

	// Step 4: Construct the *grpcclient.AuthClient. The Dialer is goroutine-safe
	// and stateless (per grpcclient.New doc); the AuthClient owns the
	// *grpc.ClientConn and is captured into the closure. Per ADR-0158 §Decision
	// D2 the ClientConn leaks-on-exit at MVP (no Close call from the filter).
	dialer := grpcclient.New(ctx.ClusterManager)
	timeout := durationpbToGo(gs.GetTimeout())
	ac, err := grpcclient.NewAuthClient(dialer, clusterName, timeout)
	if err != nil {
		return nil, fmt.Errorf("ext_authz: grpc_service: %w", err)
	}

	// Step 5: initial_metadata + retry_policy SILENT-IGNORED per SPEC §2.6 +
	// §8 items 2+3. (No code; the proto fields parse but envoy-go does not
	// consume them — auth-service designers are free to set them in their
	// config files for forward-compat without an envoy-go PARSE-REJECT.)

	// Step 6: Return the per-stream closure capturing *AuthClient + the four
	// boolean gates + validateMutations. The closure body:
	//   - builds the AttributeContext via buildAttributeContext (Task 5);
	//   - issues the Check RPC via ac.Check;
	//   - on transport error → dispError with the verbatim error;
	//   - on success → mapGRPCResponse maps the CheckResponse → checkDisposition
	//     per SPEC §6.7 + ADR-0161 gRPC-mode portion.
	return func(ctx context.Context, req *authRequest) (checkDisposition, error) {
		attrCtx := buildAttributeContext(req, encodeRawHeaders, packAsBytes, includePeerCert, includeTlsSession)
		checkReq := &authv3.CheckRequest{Attributes: attrCtx}
		resp, err := ac.Check(ctx, checkReq)
		if err != nil {
			// Transport error / DeadlineExceeded / Canceled — verbatim
			// propagation per ADR-0158 §Decision D7. The caller's
			// applyDisposition normalizes the error to dispError posture.
			return checkDisposition{class: dispError}, err
		}
		return mapGRPCResponse(resp, validateMutations), nil
	}, nil
}

// ---------------------------------------------------------------------------
// mapGRPCResponse — CheckResponse → checkDisposition dispatch per SPEC §6.7
// + ADR-0161 gRPC-mode portion. Task 6.
// ---------------------------------------------------------------------------

// mapGRPCResponse maps a *authv3.CheckResponse from the auth service to a
// checkDisposition per the SPEC §6.7 6-row dispatch table:
//
//	| HttpResponse oneof                | Status.Code | Disposition                               |
//	|-----------------------------------|-------------|-------------------------------------------|
//	| nil (empty oneof)                 | 0 (OK)      | dispAllow (implicit allow)                |
//	| nil (empty oneof)                 | non-zero    | dispDeny (default 403 — status-only deny) |
//	| *CheckResponse_OkResponse         | 0 (OK)      | dispAllow (canonical; buildAllowDispositionGRPC) |
//	| *CheckResponse_OkResponse         | non-zero    | dispError (structurally inconsistent — envoy-go-strict per SPEC §6.7 commentary) |
//	| *CheckResponse_DeniedResponse     | non-zero    | dispDeny (canonical; buildDenyDispositionGRPC) |
//	| *CheckResponse_DeniedResponse     | 0 (OK)      | dispError (BEHAVIOR_CONTRACT divergence-window per SPEC §6.7 + §13.4 — envoy-go-strict) |
//
// `validateMutations` is threaded to the allow/deny dispositions per ADR-0161
// gRPC-mode portion — a violation drives dispInvalid.
//
// `resp == nil` is treated as the "nil HttpResponse + status-zero" row (allow):
// a defensive choice for the unlikely case that the grpcclient layer returned
// a nil response without an error — matches reference Envoy's permissive
// default per the SPEC §6.7 commentary.
func mapGRPCResponse(resp *authv3.CheckResponse, validateMutations bool) checkDisposition {
	if resp == nil {
		// Defensive: nil response + nil error → allow (the empty-CheckResponse row).
		return checkDisposition{class: dispAllow}
	}

	statusCode := resp.GetStatus().GetCode() // int32; OK == 0.

	switch httpResp := resp.GetHttpResponse().(type) {
	case nil:
		// Empty oneof — status-only dispatch per SPEC §6.7.
		if statusCode == 0 {
			// Implicit allow (the empty CheckResponse — auth service signaled
			// OK without supplying a structured allow response).
			return checkDisposition{class: dispAllow}
		}
		// Status-only deny — default 403 per SPEC §6.7.
		return checkDisposition{
			class:      dispDeny,
			denyStatus: 403,
			denyBody:   nil,
		}

	case *authv3.CheckResponse_OkResponse:
		if statusCode != 0 {
			// Structurally inconsistent — OkResponse with non-zero status.
			// Envoy-go-strict treats this as dispError per SPEC §6.7 commentary
			// (auth-server bug surface).
			return checkDisposition{class: dispError}
		}
		return buildAllowDispositionGRPC(httpResp.OkResponse, validateMutations)

	case *authv3.CheckResponse_DeniedResponse:
		if statusCode == 0 {
			// BEHAVIOR_CONTRACT-documented divergence-window per SPEC §6.7
			// commentary + §13.4 — DeniedResponse with zero status. Envoy-go-strict
			// surfaces this as dispError (auth-server bug surface).
			return checkDisposition{class: dispError}
		}
		return buildDenyDispositionGRPC(httpResp.DeniedResponse, validateMutations)

	default:
		// Unknown oneof arm — defensive fall-through. The proto's `HttpResponse`
		// oneof only has the two arms above; a future proto extension landing
		// here surfaces as dispError under the envoy-go-strict discipline.
		return checkDisposition{class: dispError}
	}
}

// buildAllowDispositionGRPC builds the dispAllow disposition from an
// *OkHttpResponse per ADR-0161 gRPC-mode portion (D5 4-arm dispatch table).
//
// The 4-arm `OkHttpResponse.headers[i].append_action` dispatch per D5:
//
//	| append_action enum         | Slice              | headerKV.action                |
//	|----------------------------|--------------------|--------------------------------|
//	| APPEND_IF_EXISTS_OR_ADD    | upstreamApp        | appendDispatchDefault          |
//	| OVERWRITE_IF_EXISTS_OR_ADD | upstreamSet        | appendDispatchDefault          |
//	| OVERWRITE_IF_EXISTS        | upstreamSet        | appendDispatchOverwriteOnly    |
//	| ADD_IF_ABSENT              | upstreamSet        | appendDispatchAddIfAbsent      |
//
// `OkHttpResponse.headers_to_remove` is extracted into `upstreamDel []string`.
//
// `OkHttpResponse.response_headers_to_add` is SILENT-IGNORED per D11 (the
// envoy-go filter is decoder-only per ADR-0156 — no encode-side leg to inject
// headers into the downstream response on allow).
//
// `validate_mutations: true` runs `validateMutationHeaders` over the extracted
// `upstreamSet` + `upstreamApp` slices — a violation drives `dispInvalid`
// (the caller increments the `invalid` counter; the 403 + error-posture
// follows per SPEC §6.3).
//
// Header names are LOWERCASED for storage per Envoy's internal lowercase-header
// convention (mirrors the 18.1 HTTP-mode `extractMatchingHeaders` discipline).
func buildAllowDispositionGRPC(okResp *authv3.OkHttpResponse, validateMutations bool) checkDisposition {
	if okResp == nil {
		// Empty OkHttpResponse — no mutations; bare allow.
		return checkDisposition{class: dispAllow}
	}

	var upstreamSet, upstreamApp []headerKV
	for _, hv := range okResp.GetHeaders() {
		header := hv.GetHeader()
		if header == nil {
			continue
		}
		name := strings.ToLower(header.GetKey())
		value := header.GetValue()
		switch hv.GetAppendAction() {
		case corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD:
			upstreamApp = append(upstreamApp, headerKV{name: name, value: value, action: appendDispatchDefault})
		case corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD:
			upstreamSet = append(upstreamSet, headerKV{name: name, value: value, action: appendDispatchDefault})
		case corev3.HeaderValueOption_OVERWRITE_IF_EXISTS:
			upstreamSet = append(upstreamSet, headerKV{name: name, value: value, action: appendDispatchOverwriteOnly})
		case corev3.HeaderValueOption_ADD_IF_ABSENT:
			upstreamSet = append(upstreamSet, headerKV{name: name, value: value, action: appendDispatchAddIfAbsent})
		default:
			// Unknown enum value — defensive: fall back to APPEND semantics
			// (the proto enum default; matches the reference Envoy behavior for
			// forward-compat with future enum extensions).
			upstreamApp = append(upstreamApp, headerKV{name: name, value: value, action: appendDispatchDefault})
		}
	}

	// headers_to_remove → upstreamDel ([]string). Lowercase the names for
	// consistent matching against the upstream request map.
	var upstreamDel []string
	for _, name := range okResp.GetHeadersToRemove() {
		if name == "" {
			continue
		}
		upstreamDel = append(upstreamDel, strings.ToLower(name))
	}

	// response_headers_to_add SILENT-IGNORED per D11. The proto field parses
	// (we accept it without error) but the envoy-go filter is decoder-only
	// and cannot inject into the downstream response per ADR-0156.

	// validate_mutations gate per ADR-0161 §Decision (v): a violation drives
	// dispInvalid (the invalid counter increments at applyDisposition time).
	// Both upstreamSet and upstreamApp are validated; upstreamDel is a name-only
	// list with no value to validate (the names are lowercased ASCII per Envoy
	// convention; pseudo-header `:`-prefixed names are NOT validated against
	// upstreamDel here — `headers.Del` on a `:`-prefixed name is a no-op in
	// net/http because the canonical form differs from the storage key).
	if validateMutations {
		if err := validateMutationHeaders(upstreamSet); err != nil {
			return checkDisposition{class: dispInvalid}
		}
		if err := validateMutationHeaders(upstreamApp); err != nil {
			return checkDisposition{class: dispInvalid}
		}
	}

	return checkDisposition{
		class:       dispAllow,
		upstreamSet: upstreamSet,
		upstreamApp: upstreamApp,
		upstreamDel: upstreamDel,
	}
}

// buildDenyDispositionGRPC builds the dispDeny disposition from a
// *DeniedHttpResponse per ADR-0161 gRPC-mode portion + parent SPEC §5.P11.
//
// Per the parent SPEC §5.P11 RATIFIED in-session evidence, gRPC-mode
// `DeniedHttpResponse.headers` is applied VERBATIM — NO `allowed_client_headers`
// filter. This is UNLIKE HTTP-mode (which filters response headers through
// `allowed_client_headers`). The verbatim discipline matches reference Envoy
// v1.37.2's gRPC-mode behavior: the auth service has full authority over the
// downstream deny response headers (the gRPC envelope is structured precisely
// so the auth service can dictate the deny response shape).
//
// Per SPEC §6.7: `DeniedHttpResponse.status.code` defaults to 403 when zero
// (the proto enum 0 is `Empty`, not `OK`; treating it as "default deny" is
// the documented fallback).
//
// `validate_mutations: true` runs `validateMutationHeaders` over the extracted
// `denyHeaders` slice — a violation drives `dispInvalid` (the caller increments
// the `invalid` counter).
//
// Header names are LOWERCASED for storage per Envoy's internal lowercase-header
// convention (mirrors the 18.1 HTTP-mode `buildDenyHeaders` discipline).
func buildDenyDispositionGRPC(deniedResp *authv3.DeniedHttpResponse, validateMutations bool) checkDisposition {
	if deniedResp == nil {
		// Empty DeniedHttpResponse — default 403 with no body/headers.
		return checkDisposition{
			class:      dispDeny,
			denyStatus: 403,
		}
	}

	// status.code: default 403 per SPEC §6.7 when zero.
	denyStatus := uint32(deniedResp.GetStatus().GetCode())
	if denyStatus == 0 {
		denyStatus = 403
	}

	// headers: VERBATIM pass-through per parent SPEC §5.P11 — NO
	// `allowed_client_headers` filter (UNLIKE HTTP-mode).
	var denyHeaders []headerKV
	for _, hv := range deniedResp.GetHeaders() {
		header := hv.GetHeader()
		if header == nil {
			continue
		}
		// The append_action on DeniedHttpResponse is honored at the wire-write
		// layer only if reference Envoy distinguishes; envoy-go's deny path is
		// SendLocalReply which is a one-shot per-key emit (no incremental append
		// semantics in the local-reply HTTP framing). We carry the value verbatim;
		// the headerKV.action stays at appendDispatchDefault since the deny path
		// does not use applyUpstreamMutations.
		denyHeaders = append(denyHeaders, headerKV{
			name:  strings.ToLower(header.GetKey()),
			value: header.GetValue(),
		})
	}

	// validate_mutations gate over deny-path headers per ADR-0161 §Decision (v).
	if validateMutations {
		if err := validateMutationHeaders(denyHeaders); err != nil {
			return checkDisposition{class: dispInvalid}
		}
	}

	return checkDisposition{
		class:       dispDeny,
		denyStatus:  denyStatus,
		denyBody:    []byte(deniedResp.GetBody()),
		denyHeaders: denyHeaders,
	}
}

// packAsBytesFromWRB returns `wrb.packAsBytes` when `wrb` is non-nil; false
// otherwise. Helper for the `buildGRPCCheckFn` call-site in
// `buildCompiledConfig` per the PLAN Task 3 §Steps wording. The
// `packAsBytes` proto field lives on `BufferSettings` (see
// `compiledConfig.withRequestBody.packAsBytes`); when `with_request_body` is
// unset, `cc.withRequestBody` is nil and `packAsBytes` is implicitly false.
func packAsBytesFromWRB(wrb *bufferSettings) bool {
	return wrb != nil && wrb.packAsBytes
}
