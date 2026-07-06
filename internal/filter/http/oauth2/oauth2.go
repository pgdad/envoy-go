// Package oauth2 implements the envoy.filters.http.oauth2.v3.OAuth2
// authentication filter (sign-in + refresh + sign-out + pass_through under
// the phase-07.1 framework). Per phase-20 SPEC §6.3 + ADR-0180 +
// AMEND-3 the filter is a 3-flow state-machine with a 4-emission-category
// wire surface (302 auth-challenge / 302 post-callback-success / 302
// sign-out / 401 with constant body) — NO 500 anywhere.
//
// # File layout (phase-20 SPEC §6.11)
//
// Production files (10): oauth2.go (this file; filter type + factory +
// RegisterPerRouteValidator); compiled_config.go (STUB at Task 5; FULL
// buildCompiledConfig body at Task 11 per planner-time D11);
// decode_headers.go (DecodeHeaders dispatcher + handleUnauthenticated +
// handlePassThrough + handleValidCookies + handleRefreshFailure per SPEC
// §6.3); callback.go (handleCallback + applyTokenEndpointResponse skeleton
// + handleBadState per SPEC §6.8); signout.go (Task 9 — handleSignout per
// SPEC §6.9 + ADR-0184); oauth_client.go (Task 10 — postTokenEndpoint +
// buildTokenRequestBody + urlEncode per SPEC §6.7 + ADR-0185); cookies.go
// (5-cookie envelope parser + writer per ADR-0181); hmac.go (5-input HMAC
// composition per ADR-0179); tokens.go (AES-256-CBC token encryption per
// ADR-0182); stats.go (6-counter filterStats per ADR-0181 + AMEND-4).
//
// Test files (6): oauth2_test.go + cookies_test.go + hmac_test.go +
// tokens_test.go + oauth_client_test.go (Task 10) + fuzz_test.go (Task 12).
//
// # Per-route discipline (SPEC §5)
//
// The v1.37.x oauth2 proto has NO `OAuth2PerRoute` message at all per
// SPEC §20.P7 RATIFIED — strongest-form evidence (the proto file has no
// per-route-override message arm). Listener-scoped only.
// `RegisterPerRouteValidator(httpReg)` in this file (called from
// cmd/envoy-go/main.go pre-Freeze) wires an HCM-parse-time PARSE-REJECT
// for any route-level or virtualHost-level TPFC placement per SPEC §5.2
// + planner-time D2 (byte-stable error wording).
//
// # Async-resume integration (SPEC §6.8)
//
// The callback flow + the refresh-token flow park the decode goroutine on
// the phase-09 async-resume primitive while the token_endpoint POST is in
// flight. Task 5 lands the dispatcher + handler skeletons; Task 8 lands
// the refresh-token rotation; Task 10 wires the full token-POST body.
package oauth2

import (
	"errors"
	"net/http"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// TypeURL is the canonical envoy.filters.http.oauth2 typed_config type URL
// per phase-20 SPEC §6.1. Boot wiring in cmd/envoy-go/main.go (Task 11)
// registers `New` under this key in the HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.oauth2.v3.OAuth2"

// filterName is the canonical http_filters[].name string for oauth2 (matches
// the listener config typed_per_filter_config map keys). Used by
// RegisterPerRouteValidator + the per-route TPFC PARSE-REJECT path per
// SPEC §5.2 + planner-time D2.
const filterName = "envoy.filters.http.oauth2"

// perRouteTPFCRejectMsg is the BYTE-STABLE error wording the per-route
// validator returns when an oauth2 TPFC entry is found at route or
// virtualHost level per phase-20 SPEC §5.2 + planner-time D2 + D15.
//
// The wording is INTENTIONALLY pinned via a package-level const so a
// regression on the byte-stable contract surfaces at the TestRegisterPerRouteValidator
// assertion (Task 5). Mirrors the header_mutation per-route validator
// discipline at internal/filter/http/header_mutation/header_mutation.go.
const perRouteTPFCRejectMsg = "oauth2: typed_per_filter_config not supported at route or virtualHost level; oauth2 is listener-scoped only"

// filter is the per-stream OAuth2 filter instance per phase-20 SPEC §6.1 +
// ADR-0180. Decoder-only per SPEC §6.12 + the §9 family-row precedent
// (csrf / buffer / rbac / jwt_authn / extauthz are all decoder-only).
//
// The async-resume guard fields (mu/done/callCtx/callCancel) are declared
// here per ADR-0159 precedent — the callback flow + the refresh-token
// flow park the decode goroutine on the phase-09 async-resume primitive
// while the token_endpoint POST is in flight (per SPEC §6.8). Task 5
// declares the fields + OnDestroy guard; Task 8 + Task 10 wire the full
// outbound HTTP call.
type filter struct {
	cc *compiledConfig

	dcb envoyhttp.DecoderFilterCallbacks

	// Async-resume guard per planner-time D4 precedent (ADR-0159). OnDestroy
	// sets done=true under mu + calls callCancel; the resume goroutine checks
	// done under mu before touching dcb. Full wiring at Task 8 + Task 10.
	mu         sync.Mutex
	done       bool
	callCancel func() // set when a token-endpoint POST is in flight

	// pendingSetCookies holds the deferred Set-Cookie envelope to be emitted
	// on the upstream response per phase-20 ADR-0183 + SPEC §6.8 (refresh-
	// token rotation success leg) + Task 8. The encode-side emission of
	// these headers onto the response lands at Task 11 (full filter
	// integration); Task 8 records the pending envelope on the success-leg
	// of applyRefreshTokenResponse + the dispatcher's ContinueDecoding
	// resumes the parked decode goroutine.
	//
	// Read + written under f.mu to avoid races with the resume goroutine.
	// Empty (nil or zero-length) → no pending Set-Cookie envelope. Per
	// SPEC §6.8 + ADR-0183 the success-leg of the refresh-token rotation
	// is the ONLY caller that populates this field at Task 8; the
	// callback-flow success-leg (Task 10) reuses the same primitive.
	pendingSetCookies envoyhttp.OrderedHeaders

	// refreshDone is closed by the resume goroutine when
	// applyRefreshTokenResponse returns. Tests use waitRefreshDone() to
	// synchronize with the async dispatch without sleeping. Allocated by
	// handleRefresh under f.mu; nil when no refresh is in flight.
	//
	// Production code paths do NOT need to wait — the Envoy chain itself
	// resumes via dcb.ContinueDecoding() / dcb.SendLocalReply() called
	// from the resume goroutine.
	refreshDone chan struct{}

	// callbackDone is the auth-code POST counterpart to refreshDone, closed
	// by the resume goroutine when applyTokenEndpointResponse returns.
	// Tests use waitCallbackDone() to synchronize without sleeping.
	// Allocated by handleCallback under f.mu; nil when no auth-code POST
	// is in flight. Symmetric to refreshDone per the Task 12 follow-up
	// wire-up (handleCallback parks the decode goroutine on the same
	// phase-09 async-resume primitive as handleRefresh).
	callbackDone chan struct{}
}

// waitRefreshDone blocks until the in-flight refresh-token POST goroutine
// completes (signaled via close of f.refreshDone). Test-only — production
// code paths use the standard Envoy chain resumption via dcb.ContinueDecoding.
// Returns immediately if no refresh is in flight (refreshDone is nil).
func (f *filter) waitRefreshDone() {
	f.mu.Lock()
	ch := f.refreshDone
	f.mu.Unlock()
	if ch == nil {
		return
	}
	<-ch
}

// waitCallbackDone blocks until the in-flight auth-code POST goroutine
// completes (signaled via close of f.callbackDone). Test-only — symmetric
// to waitRefreshDone for the callback flow per the Task 12 follow-up
// wire-up. Returns immediately if no callback is in flight.
func (f *filter) waitCallbackDone() {
	f.mu.Lock()
	ch := f.callbackDone
	f.mu.Unlock()
	if ch == nil {
		return
	}
	<-ch
}

// snapshotPendingSetCookies returns a copy of f.pendingSetCookies under f.mu
// for race-clean test assertion access. Test-only.
func (f *filter) snapshotPendingSetCookies() envoyhttp.OrderedHeaders {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pendingSetCookies) == 0 {
		return nil
	}
	out := make(envoyhttp.OrderedHeaders, len(f.pendingSetCookies))
	copy(out, f.pendingSetCookies)
	return out
}

// Compile-time assertion: *filter implements the decoder-only filter interface
// per SPEC §6.12 + the §9 family-row precedent (csrf / buffer / rbac /
// jwt_authn / extauthz decoder-only).
var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)

// SetDecoderCallbacks stores the per-stream callbacks reference for later
// SendLocalReply + ContinueDecoding use. Per SPEC §6.1 + ADR-0180.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// DecodeData is the body pass-through path per SPEC §6.12 + the decoder-only
// precedent. OAuth2 does NOT consume request bodies at MVP (the callback
// flow's only body-bearing leg is the OUTBOUND token_endpoint POST, not the
// inbound request body). Returns DataContinue unconditionally.
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers is the trailers pass-through path per SPEC §6.12.
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy implements the per-stream teardown per planner-time D4 precedent
// (ADR-0159) + SPEC §6.8 async-resume discipline. Steps:
//
//  1. Acquire f.mu and set f.done = true under the lock. The resume-after-
//     OnDestroy guard: any resume goroutine that acquires f.mu after
//     OnDestroy will observe done=true and abort the callback touch without
//     panicking or touching the already-destroyed stream.
//  2. Call f.callCancel() (if non-nil) to cancel the in-flight
//     token_endpoint POST's context.Context, causing the in-flight client.Do
//     to return promptly (per the net/http cancellation contract).
//
// Task 5 lands the guard skeleton; Task 8 + Task 10 wire the full outbound-
// call cancellation. At Task 5 the guard is a no-op for streams that never
// fired a token_endpoint POST (the common case for the 4-emission-category
// synchronous-deny paths).
func (f *filter) OnDestroy() {
	f.mu.Lock()
	f.done = true
	cancel := f.callCancel
	f.mu.Unlock()

	// Cancel the in-flight outbound call's context OUTSIDE the lock to
	// avoid holding mu during the cancel (which may trigger net/http
	// cleanup).
	if cancel != nil {
		cancel()
	}
}

// New is the HTTPFilterFactory exposed at boot per phase-20 SPEC §6.1 +
// ADR-0072 + ADR-0180. Task 11 lands the FULL factory body wiring Tasks 2-10
// into a fully-functional api.HTTPFilterFactory.
//
// Per ADR-0072 boot-time-fail-fast:
//  1. tc must be non-nil (a typed_config-less oauth2 listing has no
//     behavioral effect; surface configuration mistakes at boot).
//  2. Unmarshal tc → *oauth2v3.OAuth2 envelope.
//  3. Call buildCompiledConfig(message, ctx) which executes the full 17-step
//     parser + matcher compilation + *sdsfile.Watcher construction + AES-key
//     derivation + tokenEndpointPoster wiring + filterStats allocation per
//     SPEC §6.2 + planner-time D2 byte-stable PARSE-REJECT wording.
//  4. Return FilterInstanceFactory closure that allocates a fresh *filter per
//     stream bound to the shared *compiledConfig.
//
// Per SPEC §6.12 + the §9 family-row precedent (csrf / buffer / rbac /
// jwt_authn / extauthz are all decoder-only), the returned HTTPFilter has
// Decoder=f + Encoder=nil — the oauth2 filter participates ONLY on the
// decode path. The deferred Set-Cookie envelope per ADR-0183 (refresh-token
// rotation success leg) is recorded on f.pendingSetCookies; the encode-side
// emission lands at the upstream-response Set-Cookie write per the framework's
// Set-Cookie-injection discipline (a future framework primitive may emit
// these headers on the encode path; MVP records them for cross-stream
// observability).
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("oauth2: typed_config required")
	}
	cc, err := buildCompiledConfig(tc, ctx)
	if err != nil {
		return nil, err
	}
	return func() envoyhttp.HTTPFilter {
		f := &filter{cc: cc}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: nil, // decoder-only per SPEC §6.12
		}
	}, nil
}

// RegisterPerRouteValidator registers the oauth2 per-route validator with
// the supplied HTTPRegistry per phase-20 SPEC §6.1 + §5.2 + planner-time
// D2 + D15. MUST be called BEFORE httpReg.Freeze() in cmd/envoy-go/main.go
// (the registry rejects post-Freeze registrations per ADR-0072 + the
// header_mutation precedent).
//
// The validator REJECTS any non-nil *anypb.Any TPFC entry at route or
// virtualHost level with the BYTE-STABLE error wording pinned at
// `perRouteTPFCRejectMsg`. Per SPEC §5 + §20.P7 RATIFIED — the v1.37.x
// oauth2 proto has NO `OAuth2PerRoute` message at all (strongest-form
// REUSE-by-absence; the absence itself is the lesson per the THIRD
// CONSECUTIVE §9 row to skip ADR-0125 amendment).
//
// Per ADR-0044 in-place edit discipline — NOT a new ADR; the discipline
// lives on ADR-0180 §Decision.
func RegisterPerRouteValidator(reg interface {
	RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)
}) {
	reg.RegisterPerRouteValidator(filterName, validatePerRouteOAuth2)
}

// validatePerRouteOAuth2 is the per-route validator registered with the
// framework's *HTTPRegistry per SPEC §5.2 + planner-time D2. At HCM-build
// time, BuildPerRouteConfig invokes this against each parsed
// proto.Message at each tier (Route, VirtualHost, RouteConfiguration);
// returns the perRouteTPFCRejectMsg error byte-exact.
//
// Per SPEC §5 — the v1.37.x oauth2 proto has NO `OAuth2PerRoute` message
// at all; therefore the validator REJECTS UNCONDITIONALLY when invoked
// with any non-nil message at any tier. The wording is identical across
// tiers (the framework wraps with the location prefix per perroute.go's
// error wrapping).
func validatePerRouteOAuth2(_ proto.Message) error {
	return errors.New(perRouteTPFCRejectMsg)
}
