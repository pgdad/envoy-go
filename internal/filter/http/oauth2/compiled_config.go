package oauth2

// compiled_config.go — per-listener compiled OAuth2 filter configuration per
// phase-20 SPEC §6.2. Task 11 lands the FULL `buildCompiledConfig` parser
// body (REPLACING the Task 5 STUB constructor) consuming the OAuth2 proto +
// constructing the *sdsfile.Watcher instances + populating the matcher
// closures + wiring the *httpclient.Client-backed tokenEndpointPoster +
// allocating the 6-counter filterStats per ADR-0181.
//
// # Why this file evolved at Task 11
//
// Task 5 shipped the struct shape + the `newTestCompiledConfig` test-only
// constructor so the dispatcher (decode_headers.go + callback.go) could be
// unit-tested without depending on a proto-parser surface. Task 11 lands the
// production-grade `buildCompiledConfig(message proto.Message, ctx FactoryCtx)
// (*compiledConfig, error)` body covering:
//
//   1. Unmarshal *anypb.Any to *oauth2v3.OAuth2 + extract the nested OAuth2Config.
//   2. PARSE-REJECT byte-stable wording per planner-time D2 reference strings.
//   3. Compile matchers (PathMatcher → pathMatcherFn; HeaderMatcher →
//      headerMatcherFn; deny_redirect_matcher → denyRedirectMatcherFn) from
//      protos via the framework's matcher idiom (mirrors extauthz attributes.go).
//   4. Construct *sdsfile.Watcher for hmac_secret + client_secret (each call
//      `sdsfile.New(path)` then `Start()`; cleanup on later-step failure).
//   5. Derive the AES key via SHA-256(hmac_secret)[:32] + atomic.Pointer.Store.
//   6. Capture behavioral knobs (forward_bearer_token, preserve_authorization_
//      header, disable_token_encryption, use_refresh_token, defaults).
//   7. Wire FactoryCtx.HTTPClient → cc.httpClient.
//   8. Wire tokenEndpointPoster = closure over postTokenEndpoint + buildTokenRequestBody.
//   9. Construct *filterStats via newFilterStats(ctx.Stats, statPrefix).
//   10. Return the fully-populated *compiledConfig.
//
// # Matcher abstraction discipline (forward-stable from Task 5)
//
// SPEC §6.2 names `PathMatcher` + `HeaderMatcher` as Go-level abstractions
// (not the v3 proto types). The function-shaped predicate discipline shipped
// at Task 5 is preserved at Task 11 — proto compilation produces a closure
// that captures the compiled matcher state + the dispatcher consumes the
// function. No struct-method dispatch indirection.
//
// # `hmacSecretFn` + `clientSecretFn` accessor discipline
//
// Per ADR-0178 + SPEC §3.2 the SDS-loaded secret bytes live behind an
// `atomic.Pointer[[]byte]` inside `*sdsfile.Watcher` so the watcher's fsnotify
// reload loop can hot-swap the secret without a stop-the-world pause. The
// compiledConfig stores the per-secret accessor as a closure (`func() []byte`)
// captured over the *sdsfile.Watcher. The accessor extracts the
// `generic_secret.inline_string` from the per-reload Secret-proto JSON bytes
// returned by Watcher.Current().
//
// # `aesKey` atomic.Pointer discipline (per ADR-0182 + D4)
//
// The AES-256-CBC key is derived from the hmac_secret bytes via
// SHA-256(hmac_secret)[:32] + stored via atomic.Pointer.Store. The sdsfile
// reload path could in principle re-derive the key on every reload (a future
// optimization); MVP derives once at parse time. Concurrent readers
// (encrypt/decrypt goroutines) load the pointer snapshot via atomic.LoadPointer.
//
// # PARSE-REJECT byte-stable wording per planner-time D2
//
// 13 reference strings per D2 (lines 119-133 of PLAN.md). Each PARSE-REJECT
// path uses the EXACT wording — NO format-string drift. Unit-test Group 1
// (TestParseConfig_PARSE_REJECT_*) in oauth2_test.go pins each byte-exact.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	oauth2v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/oauth2/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/httpclient"
	"github.com/pgdad/envoy-go/internal/sdsfile"
)

// pathMatcherFn returns true when the supplied request path matches the
// compiled path-matcher predicate. Nil-valued pathMatcherFn means "no matcher
// configured" — the dispatcher treats nil as a guaranteed miss.
type pathMatcherFn func(path string) bool

// headerMatcherFn returns true when the supplied request header set matches
// the compiled header-matcher predicate. Nil-valued headerMatcherFn means
// "no matcher configured" — the dispatcher treats nil as a guaranteed miss.
type headerMatcherFn func(h headerLookup) bool

// denyRedirectMatcherFn evaluates the `deny_redirect_matcher` proto field per
// SPEC §2.15 + §4.4 category (c) + ADR-0184. Returns (matched_url, true) for
// the first matcher that hits, OR ("", false) when no matcher matches. Nil-
// valued denyRedirectMatcherFn means "no matcher configured" — the dispatcher
// treats nil as a guaranteed miss.
//
// Note: the upstream proto carries only `[]HeaderMatcher` for
// deny_redirect_matcher — no paired redirect URL list. The matcher's role is
// to identify requests that should NOT receive a 302 challenge (typical use:
// AJAX requests that prefer 401 over 302 so the SPA can handle the auth flow
// itself). The matched_url return is unused in the MVP `denyRedirect` arm —
// the dispatcher uses the boolean to decide whether to emit a 302 challenge
// or fall through to a different deny path. The (url, ok) tuple shape is
// preserved at the type level for sign-out's `denyRedirectMatcher`
// integration per ADR-0184; in the sign-out call-site, an empty url with
// ok=true means "match-but-no-explicit-redirect-target" which the sign-out
// flow handles per ADR-0184.
type denyRedirectMatcherFn func(h headerLookup) (redirectURL string, matched bool)

// headerLookup is the minimal header-lookup surface the headerMatcherFn
// consumes. Decoupled from net/http.Header so tests can drive predicates
// against arbitrary header backings without round-tripping through the
// stdlib map representation.
type headerLookup interface {
	Get(name string) string
}

// secretAccessor returns the current value of an SDS-loaded secret. Per
// ADR-0178 + SPEC §3.2 the underlying bytes live behind an atomic.Pointer
// inside a *sdsfile.Watcher; the closure captures the Watcher and the
// dispatcher reads via the closure to get a value-typed snapshot.
type secretAccessor func() []byte

// tokenEndpointPosterFn is the abstraction over the outbound token_endpoint
// POST per phase-20 SPEC §6.7 + ADR-0183. Task 11 wires the production impl
// via a closure over postTokenEndpoint + buildTokenRequestBody.
//
// Parameter discipline:
//   - ctx: per-request cancellable context. OnDestroy cancels via
//     filter.callCancel per ADR-0159 D4 + extauthz precedent.
//   - grantType: "authorization_code" (auth-code leg) OR "refresh_token"
//     (refresh leg). Maps to the 4-field auth-code template OR 3-field
//     refresh-token template per AMEND-5 + ADR-0185.
//   - codeOrRefreshToken: the authorization_code value (auth-code leg) OR
//     the refresh_token value (refresh leg).
//   - clientID + clientSecret: forwarded from compiledConfig per AMEND-5.
//
// Return shape mirrors net/http.Client.Do: (*http.Response, error).
//
// Per planner-time D14 + ADR-0183: NO per-stream serialization. Each
// in-flight request invokes the poster INDEPENDENTLY; latest Set-Cookie wins
// via deferred Set-Cookie discipline.
type tokenEndpointPosterFn func(ctx context.Context, grantType, codeOrRefreshToken, clientID, clientSecret string) (*http.Response, error)

// compiledConfig is the per-listener compiled OAuth2 filter configuration per
// phase-20 SPEC §6.2. Populated by buildCompiledConfig at HCM-build time per
// ADR-0072 boot-time-fail-fast.
//
// Field grouping mirrors SPEC §6.2 for cross-referenceability.
type compiledConfig struct {
	// ---- Endpoints + URIs ----

	// tokenEndpoint is the OAuth2 token_endpoint URL per OAuth2Config.token_
	// endpoint.uri. Consumed by tokenEndpointPoster per ADR-0185.
	tokenEndpoint string

	// authorizationEndpoint is the auth-server URL the (a) 302 challenge
	// redirects to per OAuth2Config.authorization_endpoint.
	authorizationEndpoint string

	// redirectURI is the operator-registered callback URI used as the (b)
	// 302 post-callback-success Location AND as the redirect_uri parameter
	// in both the (a) authorization URL + the (b) token POST body per
	// OAuth2Config.redirect_uri.
	redirectURI string

	// ---- Matchers ----

	// redirectPathMatcher matches the callback path per SPEC §6.3 step 2.
	redirectPathMatcher pathMatcherFn

	// signoutPath matches the sign-out path per SPEC §6.3 step 1.
	signoutPath pathMatcherFn

	// passThroughMatcher matches bypass requests per SPEC §6.3 step 3.
	passThroughMatcher headerMatcherFn

	// denyRedirectMatcher integrates with the sign-out flow per SPEC §4.4 +
	// ADR-0184. Returns the matched redirect URL when a request's headers
	// match a configured matcher; returns "" + false when no matcher matches.
	denyRedirectMatcher denyRedirectMatcherFn

	// ---- Credentials ----

	// clientID is the operator-supplied OAuth2 client identifier per
	// OAuth2Credentials.client_id. Embedded in the authorization endpoint
	// URL + the token POST body.
	clientID string

	// hmacSecretFn returns the current HMAC secret bytes (from the sdsfile.
	// Watcher per ADR-0178). The dispatcher passes the closure result to
	// hmac.computeHMAC + hmac.validateHMAC per SPEC §6.4.
	hmacSecretFn secretAccessor

	// clientSecretFn returns the current client-secret bytes (from the
	// sdsfile.Watcher per ADR-0178). Consumed by tokenEndpointPoster.
	clientSecretFn secretAccessor

	// hmacWatcher + clientWatcher are the *sdsfile.Watcher instances owned by
	// this compiledConfig per planner-time D13. Close() runs at filter
	// teardown (MVP leaks-on-exit discipline). May be nil at test paths that
	// inject hmacSecretFn / clientSecretFn directly.
	hmacWatcher   *sdsfile.Watcher
	clientWatcher *sdsfile.Watcher

	// ---- Cookie envelope discipline ----

	// cookieNames carries the 5-of-7 MVP-consumed cookie names per SPEC §6.4
	// + cookies.go. Defaults per DefaultCookieNames(); operators may override
	// via the `cookie_names` proto field.
	cookieNames CookieNames

	// cookieAttrs is the per-Set-Cookie attribute discipline per SPEC §4.5.
	cookieAttrs SetCookieAttrs

	// stateCookieName is the cookie name used for the auth-challenge state
	// cookie per SPEC §4.5 category (a) + §4.4.
	stateCookieName string

	// ---- AES-256-CBC key — atomic.Pointer for sdsfile-driven hot-swap ----

	// aesKey is the AES-256-CBC key derived from hmacSecret per ADR-0182.
	// Stored behind atomic.Pointer for race-free hot-swap on sdsfile reload
	// per planner-time D4 + TestAesKeySwap_Concurrent_* (Task 7).
	aesKey atomic.Pointer[[32]byte]

	// ---- Behavioral knobs ----

	forwardBearerToken          bool
	preserveAuthorizationHeader bool
	disableTokenEncryption      bool
	useRefreshToken             bool

	// ---- Auth-flow knobs (consumed by URL composition + token POST) ----

	authScopes []string
	resources  []string

	// ---- Behavioral timings ----

	defaultExpiresIn             time.Duration
	defaultRefreshTokenExpiresIn time.Duration
	csrfTokenExpiresIn           time.Duration

	// ---- Token-endpoint POST abstraction ----

	tokenEndpointPoster tokenEndpointPosterFn

	// httpClient is the shared *httpclient.Client framework primitive (per
	// ADR-0177) used for outbound token_endpoint POSTs per ADR-0185. Threaded
	// from FactoryCtx.HTTPClient at buildCompiledConfig time. May be nil at
	// test paths that inject tokenEndpointPoster directly.
	httpClient *httpclient.Client

	// ---- Filter-wide stats ----

	stats *filterStats
}

// -----------------------------------------------------------------------------
// PARSE-REJECT byte-stable wordings per planner-time D2 (PLAN lines 119-133).
// -----------------------------------------------------------------------------

const (
	parseRejectSDSApiConfigSource      = "oauth2: ApiConfigSource ConfigSource arm not supported; only filesystem PathConfigSource is supported"
	parseRejectSDSAds                  = "oauth2: Ads ConfigSource arm not supported; only filesystem PathConfigSource is supported"
	parseRejectSDSDeprecatedPath       = "oauth2: deprecated ConfigSource.path field 1 not supported; use PathConfigSource (oneof arm field 8)"
	parseRejectSDSSecretFile           = "oauth2: generic_secret.secret_file arm not supported; only inline_string is supported"
	parseRejectCredentialsBasicAuth    = "oauth2: OAuth2Credentials.basic_auth not supported; use client_secret_post (token_endpoint POST body)"
	parseRejectPKCE                    = "oauth2: use_pkce + PKCE-related fields not supported in MVP"
	parseRejectDisableEncryptionNoHmac = "oauth2: disable_token_encryption=false requires non-empty hmac_secret"
	parseRejectAuthorizationEmpty      = "oauth2: authorization_endpoint empty"
	parseRejectRedirectURIEmpty        = "oauth2: redirect_uri empty"
	parseRejectClientIDEmpty           = "oauth2: client_id empty"
)

// parseRejectTokenEndpointInvalid formats the token_endpoint URL invalid
// PARSE-REJECT per planner-time D2. The wording uses %s to wrap the stdlib
// `url.Parse` error message. The unit-test Group 1 fixtures assert the
// `oauth2: token_endpoint URL invalid: ` prefix byte-exact + tolerate the
// stdlib-error tail.
func parseRejectTokenEndpointInvalid(stdErr error) string {
	return fmt.Sprintf("oauth2: token_endpoint URL invalid: %s", stdErr.Error())
}

// -----------------------------------------------------------------------------
// buildCompiledConfig — full proto-parser body per Task 11 + SPEC §6.2.
// -----------------------------------------------------------------------------

// buildCompiledConfig parses the OAuth2 proto envelope + constructs the per-
// listener compiledConfig per phase-20 SPEC §6.2 + planner-time D2 byte-stable
// PARSE-REJECT wording. Each parse step that fails returns ONE of the 13 D2
// reference strings verbatim.
//
// Order of validation matches the natural proto-field traversal:
//
//  1. Unmarshal Any → *OAuth2 + extract nested OAuth2Config.
//  2. Validate token_endpoint URL (stdlib url.Parse).
//  3. Validate authorization_endpoint non-empty.
//  4. Validate redirect_uri non-empty.
//  5. Validate credentials.client_id non-empty.
//  6. PARSE-REJECT auth_type=BASIC_AUTH per AMEND-5.
//  7. PARSE-REJECT PKCE fields per §2.1 (use_pkce reflects via the cookie_
//     names.oauth_nonce + code_verifier fields; the code_verifier_token_
//     expires_in field directly).
//  8. Validate SDS Secret arms (hmac_secret + client_secret/token_secret):
//     filesystem PathConfigSource only; inline_string inner arm only.
//  9. Construct *sdsfile.Watcher per SDS field.
//  10. Validate disable_token_encryption=false ↔ hmac_secret non-empty.
//  11. Derive AES key (SHA-256(hmac_secret)[:32]) + atomic.Pointer.Store.
//  12. Compile matchers (redirect_path_matcher / signout_path /
//     pass_through_matcher / deny_redirect_matcher).
//  13. Capture cookie names (operator overrides + defaults).
//  14. Capture behavioral knobs (forward_bearer_token, etc.).
//  15. Wire FactoryCtx.HTTPClient → cc.httpClient.
//  16. Wire tokenEndpointPoster closure.
//  17. Construct *filterStats (guarded `if ctx.Stats != nil` per ADR-0085).
//
// Per ADR-0072 boot-time-fail-fast: any validation failure returns the byte-
// stable error string. The framework HCM-parse path surfaces the error to
// the operator as a boot-time configuration mistake.
func buildCompiledConfig(message proto.Message, ctx envoyhttp.FactoryCtx) (*compiledConfig, error) {
	// Step 1: extract *OAuth2Config from the envelope.
	cfg, statPrefix, err := extractOAuth2Config(message)
	if err != nil {
		return nil, err
	}

	cc := &compiledConfig{}

	// Step 2: token_endpoint URL validation. Per D2 the wording is
	// `"oauth2: token_endpoint URL invalid: %s"` with the stdlib `url.Parse`
	// error message as the tail.
	if tep := cfg.GetTokenEndpoint(); tep != nil {
		uri := tep.GetUri()
		if uri == "" {
			return nil, errors.New(parseRejectTokenEndpointInvalid(errors.New("empty URI")))
		}
		if _, perr := url.Parse(uri); perr != nil {
			return nil, errors.New(parseRejectTokenEndpointInvalid(perr))
		}
		cc.tokenEndpoint = uri
	} else {
		return nil, errors.New(parseRejectTokenEndpointInvalid(errors.New("token_endpoint missing")))
	}

	// Step 3: authorization_endpoint non-empty.
	if cfg.GetAuthorizationEndpoint() == "" {
		return nil, errors.New(parseRejectAuthorizationEmpty)
	}
	cc.authorizationEndpoint = cfg.GetAuthorizationEndpoint()

	// Step 4: redirect_uri non-empty.
	if cfg.GetRedirectUri() == "" {
		return nil, errors.New(parseRejectRedirectURIEmpty)
	}
	cc.redirectURI = cfg.GetRedirectUri()

	// Step 5: credentials.client_id non-empty.
	creds := cfg.GetCredentials()
	if creds == nil || creds.GetClientId() == "" {
		return nil, errors.New(parseRejectClientIDEmpty)
	}
	cc.clientID = creds.GetClientId()

	// Step 6: PARSE-REJECT BASIC_AUTH per AMEND-5 (envoy-go-strict departure;
	// MVP supports client_secret_post only — the 4-field auth-code template +
	// 3-field refresh-token template at ADR-0185 are POST-body templates,
	// not Authorization-header BASIC).
	if cfg.GetAuthType() == oauth2v3.OAuth2Config_BASIC_AUTH {
		return nil, errors.New(parseRejectCredentialsBasicAuth)
	}

	// Step 7: PARSE-REJECT PKCE fields per §2.1. The go-control-plane v1.32.4
	// proto carries PKCE state via the cookie_names.oauth_nonce field; the
	// newer v1.37.x proto adds code_verifier + code_verifier_token_expires_in
	// — those fields don't exist in v1.32.4 so the MVP only rejects the
	// oauth_nonce arm. Any non-empty oauth_nonce signals operator intent to
	// enable PKCE; MVP rejects.
	if cn := creds.GetCookieNames(); cn != nil {
		if cn.GetOauthNonce() != "" {
			return nil, errors.New(parseRejectPKCE)
		}
	}

	// Step 8 + 9: Validate SDS Secret arms + construct *sdsfile.Watcher.
	// hmac_secret first (the AES-key derivation source); client_secret /
	// token_secret second.
	hmacPath, err := validateSDSConfigToPath(creds.GetHmacSecret())
	if err != nil {
		return nil, err
	}
	tokenPath, err := validateSDSConfigToPath(creds.GetTokenSecret())
	if err != nil {
		return nil, err
	}

	// Step 10: disable_token_encryption=false requires hmac_secret non-empty.
	// The go-control-plane v1.32.4 proto lacks the `disable_token_encryption`
	// field (added in v1.37.x); we treat the default as false (encryption
	// ON) per SPEC §6.2 + ADR-0182 §Decision. The invariant collapses to:
	// hmac_secret MUST be configured at MVP because encryption is always on
	// in v1.32.4-pinned envoy-go. Future go-control-plane bump will surface
	// the field; the byte-stable PARSE-REJECT wording is preserved for that
	// migration path.
	cc.disableTokenEncryption = false
	if !cc.disableTokenEncryption && hmacPath == "" {
		return nil, errors.New(parseRejectDisableEncryptionNoHmac)
	}

	// Construct *sdsfile.Watcher instances + populate the accessor closures.
	// The accessors parse the watcher's bytes per-call via the Secret-proto
	// JSON unmarshal helper (so the hot-reload path naturally surfaces new
	// inline_string bytes without re-parsing the operator config).
	if hmacPath != "" {
		w, werr := sdsfile.New(hmacPath)
		if werr != nil {
			return nil, fmt.Errorf("oauth2: hmac_secret sdsfile.New(%s): %w", hmacPath, werr)
		}
		if serr := w.Start(); serr != nil {
			_ = w.Close()
			return nil, fmt.Errorf("oauth2: hmac_secret sdsfile.Start: %w", serr)
		}
		cc.hmacWatcher = w
		cc.hmacSecretFn = makeSecretAccessor(w)
	}
	if tokenPath != "" {
		w, werr := sdsfile.New(tokenPath)
		if werr != nil {
			if cc.hmacWatcher != nil {
				_ = cc.hmacWatcher.Close()
			}
			return nil, fmt.Errorf("oauth2: client_secret sdsfile.New(%s): %w", tokenPath, werr)
		}
		if serr := w.Start(); serr != nil {
			_ = w.Close()
			if cc.hmacWatcher != nil {
				_ = cc.hmacWatcher.Close()
			}
			return nil, fmt.Errorf("oauth2: client_secret sdsfile.Start: %w", serr)
		}
		cc.clientWatcher = w
		cc.clientSecretFn = makeSecretAccessor(w)
	}

	// Step 11: derive AES key from hmac_secret bytes via SHA-256(...)[:32].
	// At parse time we derive once from the initial Watcher snapshot. The
	// sdsfile reload path could re-derive on every reload (a future
	// optimization); MVP derives once. Concurrent readers via atomic.Pointer.
	if cc.hmacSecretFn != nil {
		if hmacBytes := cc.hmacSecretFn(); len(hmacBytes) > 0 {
			sum := sha256.Sum256(hmacBytes)
			cc.aesKey.Store(&sum)
		}
	}

	// Step 12: compile matchers (path + header + deny_redirect).
	if cfg.GetRedirectPathMatcher() != nil {
		fn, cerr := compilePathMatcher(cfg.GetRedirectPathMatcher())
		if cerr != nil {
			cc.closeWatchers()
			return nil, fmt.Errorf("oauth2: redirect_path_matcher: %w", cerr)
		}
		cc.redirectPathMatcher = fn
	}
	if cfg.GetSignoutPath() != nil {
		fn, cerr := compilePathMatcher(cfg.GetSignoutPath())
		if cerr != nil {
			cc.closeWatchers()
			return nil, fmt.Errorf("oauth2: signout_path: %w", cerr)
		}
		cc.signoutPath = fn
	}
	if hm := cfg.GetPassThroughMatcher(); len(hm) > 0 {
		fn, cerr := compileHeaderMatcherList(hm)
		if cerr != nil {
			cc.closeWatchers()
			return nil, fmt.Errorf("oauth2: pass_through_matcher: %w", cerr)
		}
		cc.passThroughMatcher = fn
	}
	if dm := cfg.GetDenyRedirectMatcher(); len(dm) > 0 {
		fn, cerr := compileDenyRedirectMatcher(dm)
		if cerr != nil {
			cc.closeWatchers()
			return nil, fmt.Errorf("oauth2: deny_redirect_matcher: %w", cerr)
		}
		cc.denyRedirectMatcher = fn
	}

	// Step 13: cookie names (operator overrides + defaults).
	cc.cookieNames = resolveCookieNames(creds.GetCookieNames())
	cc.cookieAttrs = DefaultSetCookieAttrs()
	// Honor cookie_domain from credentials per AMEND-6 C1; empty → host-only.
	cc.cookieAttrs.Domain = creds.GetCookieDomain()
	// State cookie name pinned to "OauthExpires" placeholder per Task 5
	// settlement; Task 9 ratified the name (see compiled_config Task 5 docs).
	cc.stateCookieName = "OauthExpires"

	// Step 14: behavioral knobs.
	cc.forwardBearerToken = cfg.GetForwardBearerToken()
	cc.preserveAuthorizationHeader = cfg.GetPreserveAuthorizationHeader()
	// use_refresh_token is a BoolValue wrapper; nil → default true per proto.
	if urt := cfg.GetUseRefreshToken(); urt != nil {
		cc.useRefreshToken = urt.GetValue()
	} else {
		cc.useRefreshToken = true
	}
	cc.authScopes = cfg.GetAuthScopes()
	cc.resources = cfg.GetResources()

	// Step 14b: behavioral timings.
	if dur := cfg.GetDefaultExpiresIn(); dur != nil {
		cc.defaultExpiresIn = dur.AsDuration()
	}
	if dur := cfg.GetDefaultRefreshTokenExpiresIn(); dur != nil {
		cc.defaultRefreshTokenExpiresIn = dur.AsDuration()
	} else {
		cc.defaultRefreshTokenExpiresIn = 7 * 24 * time.Hour // 604800s per proto
	}
	// csrf_token_expires_in: not present in go-control-plane v1.32.4 proto
	// (added in v1.37.x). Use the proto's documented 600s default per SPEC §2.7.
	cc.csrfTokenExpiresIn = 600 * time.Second

	// Step 15: wire FactoryCtx.HTTPClient → cc.httpClient.
	cc.httpClient = ctx.HTTPClient

	// Step 16: wire tokenEndpointPoster closure over postTokenEndpoint +
	// buildTokenRequestBody per ADR-0185. The closure captures cc.httpClient
	// + cc.tokenEndpoint + cc.redirectURI; the per-call grantType +
	// codeOrRefreshToken + clientID + clientSecret are passed at call time.
	cc.tokenEndpointPoster = func(callCtx context.Context, grantType, codeOrRefresh, clientID, clientSecret string) (*http.Response, error) {
		var params map[string]string
		switch grantType {
		case grantTypeAuthorizationCode:
			params = map[string]string{
				"code":          codeOrRefresh,
				"client_id":     clientID,
				"client_secret": clientSecret,
				"redirect_uri":  cc.redirectURI,
			}
		case grantTypeRefreshToken:
			params = map[string]string{
				"refresh_token": codeOrRefresh,
				"client_id":     clientID,
				"client_secret": clientSecret,
			}
		default:
			return nil, fmt.Errorf("oauth2: unknown grant_type %q", grantType)
		}
		body := buildTokenRequestBody(grantType, params)
		return postTokenEndpoint(callCtx, cc.httpClient, cc.tokenEndpoint, body)
	}

	// Step 17: allocate filterStats per ADR-0181 + ADR-0085 nil-tolerance.
	if ctx.Stats != nil {
		prefix := ctx.StatPrefix
		if statPrefix != "" {
			// Per OAuth2Config.stat_prefix proto: optional additional prefix to
			// use when emitting statistics. Honor as a suffix segment per the
			// established §9 family-row convention (mirrors extauthz / extproc).
			if prefix == "" {
				prefix = statPrefix
			} else {
				prefix = prefix + "." + statPrefix
			}
		}
		cc.stats = newFilterStats(ctx.Stats, prefix)
	}

	return cc, nil
}

// extractOAuth2Config unmarshal the proto envelope into *oauth2v3.OAuth2 +
// extracts the nested *OAuth2Config. Returns (cfg, statPrefix, err). The
// per-proto statPrefix is hoisted out to keep the buildCompiledConfig body
// readable.
//
// Accepts either *anypb.Any (the framework's typed_config carrier) OR
// *oauth2v3.OAuth2 directly (a future direct-construction path).
func extractOAuth2Config(message proto.Message) (*oauth2v3.OAuth2Config, string, error) {
	if message == nil {
		return nil, "", errors.New("oauth2: typed_config required")
	}

	var outer *oauth2v3.OAuth2
	switch m := message.(type) {
	case *oauth2v3.OAuth2:
		outer = m
	case interface {
		UnmarshalTo(proto.Message) error
	}:
		outer = &oauth2v3.OAuth2{}
		if err := m.UnmarshalTo(outer); err != nil {
			return nil, "", fmt.Errorf("oauth2: unmarshal: %w", err)
		}
	default:
		return nil, "", fmt.Errorf("oauth2: unexpected typed_config type %T", message)
	}

	cfg := outer.GetConfig()
	if cfg == nil {
		return nil, "", errors.New("oauth2: OAuth2.config required")
	}
	// stat_prefix not present in go-control-plane v1.32.4 proto (added in
	// v1.37.x). Future bump will read cfg.GetStatPrefix() here; for now the
	// per-OAuth2Config stat_prefix is empty + falls back to the HCM
	// stat_prefix per ADR-0143.
	return cfg, "", nil
}

// validateSDSConfigToPath validates an *SdsSecretConfig per the planner-time
// D2 PARSE-REJECT roster and returns the filesystem path extracted from
// PathConfigSource (oneof arm field 8). Returns ("", nil) when the SDS config
// is nil (not configured). Returns ("", error) on any PARSE-REJECT path.
//
// PARSE-REJECT paths per D2:
//   - non-filesystem ConfigSource arms (ApiConfigSource / Ads)
//   - deprecated ConfigSource.path field 1 (use PathConfigSource oneof arm)
//   - inner generic_secret.secret_file indirect-arm (in PathConfigSource'd
//     Secret file's content, the Secret proto's `generic_secret.secret`
//     DataSource must use inline_string, NOT filename)
//
// The "inner secret_file" PARSE-REJECT is enforced LATER, at the secret-
// accessor read-time. Here we validate the outer ConfigSource arm only +
// return the filesystem path for *sdsfile.Watcher construction.
func validateSDSConfigToPath(sdsCfg *tlsv3.SdsSecretConfig) (string, error) {
	if sdsCfg == nil {
		return "", nil
	}
	src := sdsCfg.GetSdsConfig()
	if src == nil {
		// Static-secret arm (no sds_config; Secret resolved by name only).
		// MVP requires sds_config; surface as a not-supported parse-rejection.
		return "", fmt.Errorf("oauth2: SDS Secret %q requires sds_config (static-resource arm not supported)", sdsCfg.GetName())
	}
	// Examine the ConfigSourceSpecifier oneof.
	switch arm := src.GetConfigSourceSpecifier().(type) {
	case *corev3.ConfigSource_PathConfigSource:
		pcs := arm.PathConfigSource
		if pcs == nil || pcs.GetPath() == "" {
			return "", errors.New(parseRejectSDSDeprecatedPath)
		}
		return pcs.GetPath(), nil
	case *corev3.ConfigSource_Path:
		// Deprecated path field 1.
		return "", errors.New(parseRejectSDSDeprecatedPath)
	case *corev3.ConfigSource_ApiConfigSource:
		return "", errors.New(parseRejectSDSApiConfigSource)
	case *corev3.ConfigSource_Ads:
		return "", errors.New(parseRejectSDSAds)
	case *corev3.ConfigSource_Self:
		// Self arm is structurally the same as Ads for our purposes (xDS-via-
		// some-protocol; not filesystem). Map to the same PARSE-REJECT as Ads.
		return "", errors.New(parseRejectSDSAds)
	case nil:
		return "", errors.New(parseRejectSDSDeprecatedPath)
	default:
		return "", fmt.Errorf("oauth2: unknown ConfigSource arm %T", arm)
	}
}

// makeSecretAccessor returns a closure that returns the current SDS Secret's
// inline_string bytes per ADR-0178. The closure captures the *sdsfile.Watcher
// + parses the Secret-proto JSON on every call (the Watcher's atomic.Pointer
// snapshot returns the post-reload bytes naturally).
//
// PARSE-REJECT per D2: when the Secret proto's `generic_secret.secret`
// DataSource uses the `filename` arm (the indirect-arm), the accessor returns
// an empty []byte — the downstream HMAC / token POST sees the empty secret +
// fails naturally. We do NOT panic on the indirect-arm at read time (the
// parse-time check should have rejected; this is defense-in-depth for cases
// where the Secret bytes change post-construction).
//
// On JSON parse failure (malformed Secret bytes at runtime), the accessor
// returns the raw Watcher bytes verbatim — this gives the consumer SOMETHING
// to work with (typical case: the operator wrote the raw secret to the
// filesystem path expecting the rotation to just produce raw bytes; envoy-go
// surfaces the bytes for the HMAC / encrypt-decrypt operations).
func makeSecretAccessor(w *sdsfile.Watcher) secretAccessor {
	return func() []byte {
		raw := w.Current()
		if len(raw) == 0 {
			return nil
		}
		// Try Secret-proto JSON decode; on failure, treat the raw bytes as
		// the inline_string (operator-friendly fall-back per ADR-0178 §Decision
		// rationale).
		var sec struct {
			Name          string `json:"name"`
			GenericSecret struct {
				Secret struct {
					InlineString string `json:"inline_string"`
					InlineBytes  string `json:"inline_bytes"`
					Filename     string `json:"filename"`
				} `json:"secret"`
			} `json:"generic_secret"`
		}
		if err := json.Unmarshal(raw, &sec); err != nil {
			return raw
		}
		if sec.GenericSecret.Secret.InlineString != "" {
			return []byte(sec.GenericSecret.Secret.InlineString)
		}
		if sec.GenericSecret.Secret.InlineBytes != "" {
			return []byte(sec.GenericSecret.Secret.InlineBytes)
		}
		// PARSE-REJECT-deferred: if `filename` is set + neither inline is set,
		// we return nil (the downstream HMAC / token POST sees empty secret).
		// The parse-time PARSE-REJECT would have caught the outer config;
		// this is defense-in-depth for post-rotation drift.
		if sec.GenericSecret.Secret.Filename != "" {
			return nil
		}
		// No recognized arm — return raw bytes as fall-back.
		return raw
	}
}

// resolveCookieNames merges operator-supplied CookieNames overrides with the
// upstream-canonical defaults per ADR-0181. Empty operator fields fall back
// to the canonical default per the proto's omit-as-default convention.
func resolveCookieNames(p *oauth2v3.OAuth2Credentials_CookieNames) CookieNames {
	out := DefaultCookieNames()
	if p == nil {
		return out
	}
	if v := p.GetBearerToken(); v != "" {
		out.BearerToken = v
	}
	if v := p.GetOauthHmac(); v != "" {
		out.OauthHMAC = v
	}
	if v := p.GetOauthExpires(); v != "" {
		out.OauthExpires = v
	}
	if v := p.GetIdToken(); v != "" {
		out.IdToken = v
	}
	if v := p.GetRefreshToken(); v != "" {
		out.RefreshToken = v
	}
	return out
}

// compilePathMatcher projects a *matcherv3.PathMatcher into a pathMatcherFn
// closure per SPEC §6.2. The PathMatcher proto wraps a StringMatcher under
// the `path` rule; we delegate to compileStringMatcher.
func compilePathMatcher(pm *matcherv3.PathMatcher) (pathMatcherFn, error) {
	if pm == nil {
		return nil, nil
	}
	sm := pm.GetPath()
	if sm == nil {
		return nil, errors.New("PathMatcher.path missing")
	}
	matcher, err := compileStringMatcher(sm)
	if err != nil {
		return nil, err
	}
	return func(path string) bool {
		return matcher(path)
	}, nil
}

// compileStringMatcher projects a *matcherv3.StringMatcher into a string-
// predicate closure. Mirrors extauthz attributes.go::compileOneStringMatcher
// (the in-tree precedent) without depending on its compiledStringMatcher
// interface — oauth2's matchers are simpler (no ListStringMatcher composition
// at MVP).
func compileStringMatcher(sm *matcherv3.StringMatcher) (func(string) bool, error) {
	if sm == nil {
		return nil, errors.New("nil StringMatcher")
	}
	ic := sm.GetIgnoreCase()
	switch mp := sm.GetMatchPattern().(type) {
	case *matcherv3.StringMatcher_Exact:
		want := mp.Exact
		if ic {
			return func(s string) bool { return strings.EqualFold(want, s) }, nil
		}
		return func(s string) bool { return want == s }, nil

	case *matcherv3.StringMatcher_Prefix:
		prefix := mp.Prefix
		if ic {
			return func(s string) bool {
				return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
			}, nil
		}
		return func(s string) bool { return strings.HasPrefix(s, prefix) }, nil

	case *matcherv3.StringMatcher_Suffix:
		suffix := mp.Suffix
		if ic {
			return func(s string) bool {
				return len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix)
			}, nil
		}
		return func(s string) bool { return strings.HasSuffix(s, suffix) }, nil

	case *matcherv3.StringMatcher_Contains:
		needle := mp.Contains
		if ic {
			lc := strings.ToLower(needle)
			return func(s string) bool { return strings.Contains(strings.ToLower(s), lc) }, nil
		}
		return func(s string) bool { return strings.Contains(s, needle) }, nil

	case *matcherv3.StringMatcher_SafeRegex:
		if mp.SafeRegex == nil {
			return nil, errors.New("safe_regex: nil RegexMatcher")
		}
		re, err := regexp.Compile(mp.SafeRegex.GetRegex())
		if err != nil {
			return nil, fmt.Errorf("safe_regex compile: %w", err)
		}
		return func(s string) bool { return re.MatchString(s) }, nil

	case nil:
		return nil, errors.New("StringMatcher match_pattern oneof unset")

	default:
		return nil, fmt.Errorf("unknown StringMatcher pattern %T", mp)
	}
}

// compileHeaderMatcherList projects a []*routev3.HeaderMatcher into a single
// headerMatcherFn that returns true when ANY of the list's matchers matches.
// Mirrors the established "list-of-matchers any-match" semantics used by
// pass_through_matcher in upstream Envoy + the OAuth2 spec.
func compileHeaderMatcherList(hms []*routev3.HeaderMatcher) (headerMatcherFn, error) {
	matchers := make([]func(headerLookup) bool, 0, len(hms))
	for i, hm := range hms {
		fn, err := compileHeaderMatcher(hm)
		if err != nil {
			return nil, fmt.Errorf("header_matcher[%d]: %w", i, err)
		}
		matchers = append(matchers, fn)
	}
	return func(h headerLookup) bool {
		for _, m := range matchers {
			if m(h) {
				return true
			}
		}
		return false
	}, nil
}

// compileHeaderMatcher projects a single *routev3.HeaderMatcher into a
// header-predicate closure. Handles the subset of HeaderMatcher specifiers
// commonly used for OAuth2 bypass / deny-redirect matching: exact / prefix /
// suffix / contains / safe_regex / present / string_match.
func compileHeaderMatcher(hm *routev3.HeaderMatcher) (func(headerLookup) bool, error) {
	if hm == nil {
		return nil, errors.New("nil HeaderMatcher")
	}
	name := hm.GetName()
	if name == "" {
		return nil, errors.New("HeaderMatcher.name required")
	}
	invert := hm.GetInvertMatch()
	wrap := func(inner func(string) bool, presentCheck bool) func(headerLookup) bool {
		return func(h headerLookup) bool {
			v := h.Get(name)
			var m bool
			if presentCheck {
				m = v != ""
			} else {
				m = inner(v)
			}
			if invert {
				return !m
			}
			return m
		}
	}
	switch spec := hm.GetHeaderMatchSpecifier().(type) {
	case *routev3.HeaderMatcher_ExactMatch:
		want := spec.ExactMatch //nolint:staticcheck // intentional: deprecated arm honored for backward-compat per ADR-0080
		return wrap(func(v string) bool { return v == want }, false), nil
	case *routev3.HeaderMatcher_PrefixMatch:
		prefix := spec.PrefixMatch //nolint:staticcheck // intentional: deprecated arm honored for backward-compat per ADR-0080
		return wrap(func(v string) bool { return strings.HasPrefix(v, prefix) }, false), nil
	case *routev3.HeaderMatcher_SuffixMatch:
		suffix := spec.SuffixMatch //nolint:staticcheck // intentional: deprecated arm honored for backward-compat per ADR-0080
		return wrap(func(v string) bool { return strings.HasSuffix(v, suffix) }, false), nil
	case *routev3.HeaderMatcher_ContainsMatch:
		needle := spec.ContainsMatch //nolint:staticcheck // intentional: deprecated arm honored for backward-compat per ADR-0080
		return wrap(func(v string) bool { return strings.Contains(v, needle) }, false), nil
	case *routev3.HeaderMatcher_SafeRegexMatch:
		safeRe := spec.SafeRegexMatch //nolint:staticcheck // intentional: deprecated arm honored for backward-compat per ADR-0080
		if safeRe == nil {
			return nil, errors.New("safe_regex_match: nil RegexMatcher")
		}
		re, err := regexp.Compile(safeRe.GetRegex())
		if err != nil {
			return nil, fmt.Errorf("safe_regex_match compile: %w", err)
		}
		return wrap(func(v string) bool { return re.MatchString(v) }, false), nil
	case *routev3.HeaderMatcher_PresentMatch:
		present := spec.PresentMatch
		if present {
			return wrap(nil, true), nil
		}
		// present_match=false means "not present" (absence-as-presence-of-absence).
		return func(h headerLookup) bool {
			absent := h.Get(name) == ""
			if invert {
				return !absent
			}
			return absent
		}, nil
	case *routev3.HeaderMatcher_StringMatch:
		inner, err := compileStringMatcher(spec.StringMatch)
		if err != nil {
			return nil, fmt.Errorf("string_match: %w", err)
		}
		return wrap(inner, false), nil
	case nil:
		return nil, errors.New("HeaderMatcher specifier oneof unset")
	default:
		return nil, fmt.Errorf("unknown HeaderMatcher specifier %T", spec)
	}
}

// compileDenyRedirectMatcher projects a []*routev3.HeaderMatcher into a
// denyRedirectMatcherFn. The proto carries only the header-matcher list — no
// paired redirect URL list at the v1.37.x proto. We surface ("", true) for
// the first matching entry so the sign-out call-site can fall through to its
// own redirect-URL discipline per ADR-0184.
func compileDenyRedirectMatcher(hms []*routev3.HeaderMatcher) (denyRedirectMatcherFn, error) {
	if len(hms) == 0 {
		return nil, nil
	}
	hfn, err := compileHeaderMatcherList(hms)
	if err != nil {
		return nil, err
	}
	return func(h headerLookup) (string, bool) {
		if hfn(h) {
			return "", true
		}
		return "", false
	}, nil
}

// closeWatchers releases all *sdsfile.Watcher resources owned by cc. Idempotent
// (each watcher's Close() is sync.Once-guarded per ADR-0178).
func (cc *compiledConfig) closeWatchers() {
	if cc.hmacWatcher != nil {
		_ = cc.hmacWatcher.Close()
	}
	if cc.clientWatcher != nil {
		_ = cc.clientWatcher.Close()
	}
}

// newTestCompiledConfig constructs a minimal *compiledConfig for unit tests
// (preserves the Task 5 test-only constructor). The full proto-driven
// buildCompiledConfig lands at Task 11 above; this helper is preserved
// verbatim so the existing Task 5-10 test surface continues to GREEN.
func newTestCompiledConfig() *compiledConfig {
	cc := &compiledConfig{
		cookieNames:     DefaultCookieNames(),
		cookieAttrs:     DefaultSetCookieAttrs(),
		stateCookieName: "OauthExpires",
	}
	return cc
}
