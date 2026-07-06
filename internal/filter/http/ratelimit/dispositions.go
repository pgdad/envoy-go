package ratelimit

// dispositions.go — the §4.6 OK / OVER_LIMIT / error disposition dispatch +
// the §4.7 byte-shape body per phase-24.1 PLAN Task 7. Called from the async
// goroutine in decode_headers.go::DecodeHeaders under f.mu (the resume-after-
// OnDestroy guard per the ext_authz precedent at extauthz.go:1044).
//
// # Dispatch table (per parent SPEC §4.6)
//
//	+--------------------------+----------------------------------------------+
//	| OverallCode + err        | Disposition                                  |
//	+--------------------------+----------------------------------------------+
//	| OK + nil err             | ok.Inc; apply RLS RequestHeadersToAdd to     |
//	|                          | the upstream request map; ContinueDecoding   |
//	+--------------------------+----------------------------------------------+
//	| OVER_LIMIT + nil err     | over_limit.Inc; SendLocalReply per the §4.7  |
//	|                          | byte-shape (status from cc.rateLimitedStatus,|
//	|                          | body from resp.RawBody, AMEND-8 header order)|
//	+--------------------------+----------------------------------------------+
//	| any err != nil           | error.Inc; FORK on cc.failureModeDeny:       |
//	| OR nil resp              |   false (DEFAULT/fail-open): failure_mode_   |
//	| OR UNKNOWN OverallCode   |     allowed.Inc; ContinueDecoding             |
//	|                          |   true  (fail-closed): SendLocalReply(       |
//	|                          |     cc.statusOnError, "", nil)               |
//	+--------------------------+----------------------------------------------+
//
// # §4.7 OVER_LIMIT byte-shape (AMEND-8 header order)
//
// Headers emitted via SendLocalReply, in order:
//
//   1. `x-envoy-ratelimited: true` (UNLESS cc.disableXEnvoyRateLimitedHeader)
//   2. RLS resp.ResponseHeadersToAdd (in RLS-given order)
//   3. cc.responseHeadersToAdd (in config order; parsed at Task 3)
//
// Body: resp.RawBody (verbatim; may be empty).
// Status: cc.rateLimitedStatus (default 429; clamp <400→429 per Task 3).
//
// # rc-details + gRPC status: ABSENT-BY-API at 24.1
//
// Parent SPEC §4.7 specifies `response_code_details = "request_rate_limited"`
// (OVER_LIMIT) and `"rate_limiter_error"` (error/fail-closed). The 3-arg
// envoyhttp SendLocalReply API (callbacks.go:35: `SendLocalReply(status,
// body, headers)`) does NOT carry an rc-details slot. The admission_control
// precedent at admission_control/decode_headers.go:25 documents the same
// gap ("denied_by_admission_control" rc-details ABSENT-BY-API per PD-2.503).
// The constants are pinned at this file (rcDetailsRequestRateLimited /
// rcDetailsRateLimiterError) for forward 24.2 consumption when/if the API
// extends to a 4-arg variant.
//
// Similarly the gRPC status code (8 RESOURCE_EXHAUSTED vs 14 UNAVAILABLE per
// cc.rateLimitedAsResourceExhausted) requires a gRPC trailer envelope on
// the response — also ABSENT-BY-API at 24.1. The mapping helper
// rateLimitedAsResourceExhaustedGrpcCode is pinned for forward consumption.
//
// # Cross-references
//
//   - parent SPEC §4.6 (decode-side dispositions)
//   - parent SPEC §4.7 (OVER_LIMIT byte-shape + AMEND-8 header order +
//     error/fail-closed byte-shape)
//   - parent SPEC §1.1 AMEND-8 (header order)
//   - compiled_config.go (cc.rateLimitedStatus / cc.statusOnError /
//     cc.disableXEnvoyRateLimitedHeader / cc.responseHeadersToAdd /
//     cc.rateLimitedAsResourceExhausted / cc.failureModeDeny)
//   - stats.go (ok / over_limit / error / failure_mode_allowed counters per
//     AMEND-1)
//   - internal/filter/http/extauthz/extauthz.go:1065 applyDisposition
//     (precedent for the under-mu disposition dispatch pattern)
//   - internal/filter/http/admission_control/decode_headers.go (the
//     SendLocalReply rc-details "ABSENT-BY-API" precedent)

import (
	"net/http"

	ratelimitservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// ----------------------------------------------------------------------------
// rc-details + gRPC code constants — pinned for byte-stability + forward
// 24.2 consumption (ABSENT-BY-API on the 3-arg SendLocalReply at 24.1).
// ----------------------------------------------------------------------------

const (
	// rcDetailsRequestRateLimited is the parent SPEC §4.7 OVER_LIMIT
	// response_code_details string. ABSENT-BY-API on the 3-arg
	// SendLocalReply at 24.1; pinned for byte-stability + forward consumption.
	rcDetailsRequestRateLimited = "request_rate_limited"

	// rcDetailsRateLimiterError is the parent SPEC §4.7 error/fail-closed
	// response_code_details string. ABSENT-BY-API at 24.1; pinned for forward
	// consumption.
	rcDetailsRateLimiterError = "rate_limiter_error"

	// headerXEnvoyRateLimited is the canonical name of the
	// `x-envoy-ratelimited` response header injected on OVER_LIMIT replies
	// per AMEND-8 + ratelimit.cc OVER_LIMIT codepath. Lowercase per the
	// upstream wire form (HTTP headers are case-insensitive but the upstream
	// reference uses lowercase; envoy-go preserves the on-wire byte form).
	headerXEnvoyRateLimited = "x-envoy-ratelimited"
)

const (
	// grpcCodeResourceExhausted is the canonical gRPC RESOURCE_EXHAUSTED
	// status code (per google.golang.org/grpc/codes). Selected on OVER_LIMIT
	// replies when cc.rateLimitedAsResourceExhausted is true.
	grpcCodeResourceExhausted = 8

	// grpcCodeUnavailable is the canonical gRPC UNAVAILABLE status code.
	// Selected on OVER_LIMIT replies when cc.rateLimitedAsResourceExhausted
	// is false (the default).
	grpcCodeUnavailable = 14
)

// rateLimitedAsResourceExhaustedGrpcCode maps the
// cc.rateLimitedAsResourceExhausted gate to the gRPC status code per parent
// SPEC §4.7. Pinned for forward 24.2 consumption — the gRPC trailer envelope
// emission is ABSENT-BY-API at 24.1.
func rateLimitedAsResourceExhaustedGrpcCode(asResourceExhausted bool) int {
	if asResourceExhausted {
		return grpcCodeResourceExhausted
	}
	return grpcCodeUnavailable
}

// ----------------------------------------------------------------------------
// applyDisposition — the per-call dispatch entry-point.
// ----------------------------------------------------------------------------

// applyDisposition dispatches an RLS response (resp) + transport error (err)
// to one of three arms per the §4.6 table at the file header. Called from
// the async goroutine in decode_headers.go under f.mu (the resume-after-
// OnDestroy guard).
//
// Precondition: f.mu is held by the caller; f.done is false.
//
// Parameters:
//   - headers: the request header map from DecodeHeaders. Mutations land on
//     the SAME map the HCM chain forwards upstream (the ext_authz
//     applyUpstreamMutations precedent at extauthz.go:849); only the OK arm
//     applies the RLS RequestHeadersToAdd to it.
//   - resp:    the *RateLimitResponse from rlsCallFn (nil on transport error).
//   - err:     the transport error from rlsCallFn (nil on successful RPC,
//     INCLUDING OK + OVER_LIMIT codes).
func (f *filter) applyDisposition(headers http.Header, resp *ratelimitservicev3.RateLimitResponse, err error) {
	// Error arm: any transport error OR a nil response OR an unknown
	// OverallCode (UNKNOWN = 0 — the proto-zero default; signals the RLS
	// returned a malformed envelope).
	if err != nil || resp == nil || resp.GetOverallCode() == ratelimitservicev3.RateLimitResponse_UNKNOWN {
		// Phase 24.2 Task 5 (D-RL12 + ADR-0197 X-RateLimit slice): the
		// transport-error + UNKNOWN-OverallCode arms feed through
		// applyError. The store-statuses-on-failOpen branch is INSIDE
		// applyError so the failure_mode_deny gate properly suppresses the
		// store on fail-CLOSED (the nullptr-mutate 500 path emits NO
		// X-RateLimit headers per D-RL12). For nil-resp the slice is nil
		// anyway; no store occurs on either side.
		f.applyError(resp)
		return
	}

	switch resp.GetOverallCode() {
	case ratelimitservicev3.RateLimitResponse_OK:
		// Phase 24.2 Task 5: store the per-descriptor statuses for the
		// encode-side X-RateLimit DRAFT_VERSION_03 emission (ADR-0197
		// X-RateLimit slice + D-RL12). Read by encode.go::encodeHeaders.
		// Stored UNDER f.mu (the caller holds it via the resume-after-
		// OnDestroy guard at decode_headers.go:261-263).
		f.responseStatuses = resp.GetStatuses()
		f.applyOK(headers, resp)
	case ratelimitservicev3.RateLimitResponse_OVER_LIMIT:
		// Phase 24.2 Task 5: store statuses BEFORE SendLocalReply. The
		// applyOverLimit path calls dcb.SendLocalReply which synchronously
		// enters the encode chain (chain.go::beginLocalReply at chain.go:1214)
		// + invokes this filter's EncodeHeaders inline; encodeHeaders reads
		// f.responseStatuses (lock-free — the store is sequenced before
		// SendLocalReply on the same goroutine per encode.go's re-entrancy
		// note).
		f.responseStatuses = resp.GetStatuses()
		f.applyOverLimit(resp)
	default:
		// Defensive — any future-added OverallCode value falls through to the
		// error arm. The proto today only defines {UNKNOWN, OK, OVER_LIMIT}.
		f.applyError(resp)
	}
}

// ----------------------------------------------------------------------------
// applyOK — RLS returned OK (admit). Apply RequestHeadersToAdd; resume the chain.
// ----------------------------------------------------------------------------

// applyOK applies the OK arm: increment the ok counter, apply the RLS
// RequestHeadersToAdd to the request header map (so the upstream-forwarded
// request carries them — mirrors ext_authz/extauthz.go:1083
// applyUpstreamMutations), and resume the parked chain via ContinueDecoding.
//
// Per parent SPEC §3.1: RequestHeadersToAdd lands on the UPSTREAM REQUEST
// (the request the proxy forwards to the upstream); ResponseHeadersToAdd
// lands on the DOWNSTREAM RESPONSE (only consulted on the OVER_LIMIT arm at
// 24.1; the OK + downstream-response merge is part of the X-RateLimit slice
// at 24.2 per D-RL7 — forward-pointer below).
//
// 24.2: X-RateLimit DRAFT_VERSION_03 injection point (parent §6.6 + ADR-0197
// X-RateLimit slice). At 24.1: x-ratelimit-limit / -remaining / -reset
// headers are NOT emitted; cc.enableXRateLimitHeaders is parsed but ignored.
// The OK-side ResponseHeadersToAdd merge onto the downstream response is
// part of the encode-side path landing at 24.2.
func (f *filter) applyOK(headers http.Header, resp *ratelimitservicev3.RateLimitResponse) {
	if c := f.cc.stats.ok; c != nil {
		c.Inc()
	}
	// Apply RLS RequestHeadersToAdd to the upstream request headers.
	// AMEND-8 / ext_authz precedent: each entry is appended (Add) — the
	// proto's HeaderValueOption.append_action is NOT modeled at 24.1; the
	// RLS RequestHeadersToAdd field uses the simpler *corev3.HeaderValue
	// (no append_action discriminator). Canonical-case the key per Go
	// convention (http.Header.Add canonicalizes internally — but we
	// canonicalize here too for symmetry with the Task-3
	// compileResponseHeaders parse path).
	for _, hv := range resp.GetRequestHeadersToAdd() {
		if hv == nil || hv.GetKey() == "" {
			continue
		}
		headers.Add(http.CanonicalHeaderKey(hv.GetKey()), hv.GetValue())
	}
	f.dcb.ContinueDecoding()
}

// ----------------------------------------------------------------------------
// applyOverLimit — RLS returned OVER_LIMIT (reject). Emit the §4.7 byte-shape.
// ----------------------------------------------------------------------------

// applyOverLimit applies the OVER_LIMIT arm per parent SPEC §4.7 + AMEND-8:
//
//  1. over_limit counter Inc.
//  2. Build the AMEND-8-ordered headers slice:
//     [a] `x-envoy-ratelimited: true` (UNLESS cc.disableXEnvoyRateLimitedHeader)
//     [b] RLS resp.ResponseHeadersToAdd (in RLS-given order)
//     [c-pre] X-RateLimit DRAFT_VERSION_03 triple (UNLESS OFF / no current_limit)
//     [c] cc.responseHeadersToAdd (in config order)
//  3. SendLocalReply(cc.rateLimitedStatus, resp.RawBody, headers).
//
// Phase 24.2 Task 5 follow-up (AMEND-8 wire-order I-1 — parent SPEC §4.7
// line 214): the X-RateLimit triple is applied INLINE here BETWEEN the
// `x-envoy-ratelimited` slot and the filter-config `response_headers_to_add`
// slot — NOT later via the encode-side hook. The encode-side
// `encode.go::encodeHeaders` hook still runs synchronously via
// `chain.go::beginLocalReply → RunEncodeHeaders`, but its `headers.Set`
// becomes a no-op-set-equal idempotent overwrite (the same
// `buildXRateLimitHeaders` source produces byte-identical values; HTTP
// header keys canonicalize identically under `http.Header.Set` /
// `http.Header.Add`). On OK / fail-OPEN dispositions the encode-side hook
// remains the SOLE emission point (no inline emission on the admit path —
// the upstream provides the response headers and X-RateLimit layers on top
// via encode-side Set).
//
// The rc-details ("request_rate_limited") + the gRPC status code (8/14) are
// ABSENT-BY-API at 24.1 (see file-header).
func (f *filter) applyOverLimit(resp *ratelimitservicev3.RateLimitResponse) {
	if c := f.cc.stats.overLimit; c != nil {
		c.Inc()
	}

	// Capacity hint: 1 (x-envoy-ratelimited slot) + RLS slice + 3 (X-RateLimit
	// triple) + config slice.
	headers := make(envoyhttp.OrderedHeaders, 0, 1+len(resp.GetResponseHeadersToAdd())+3+len(f.cc.responseHeadersToAdd))

	// [a] x-envoy-ratelimited: true — unless suppressed.
	if !f.cc.disableXEnvoyRateLimitedHeader {
		headers = append(headers, envoyhttp.HeaderField{
			Name:  headerXEnvoyRateLimited,
			Value: "true",
		})
	}

	// [b] RLS-supplied response headers, in RLS-given order. Skip empty-key
	// entries (defensive — the proto's HeaderValue.key is conventionally
	// non-empty but the wrapper is not required).
	for _, hv := range resp.GetResponseHeadersToAdd() {
		if hv == nil || hv.GetKey() == "" {
			continue
		}
		headers = append(headers, envoyhttp.HeaderField{
			Name:  hv.GetKey(),
			Value: hv.GetValue(),
		})
	}

	// [c-pre] X-RateLimit DRAFT_VERSION_03 triple — inlined here per parent
	// SPEC §4.7 line 214 wire-order discipline (Task 5 follow-up; spec +
	// code-quality I-1 fix). Gated on the same two predicates as the encode-
	// side hook:
	//   - cc.enableXRateLimitHeaders == true (DRAFT_VERSION_03 enabled)
	//   - buildXRateLimitHeaders returns ok=true (≥1 status carries current_limit)
	// The encode-side hook (encode.go::encodeHeaders) reads f.responseStatuses
	// (stored at applyDisposition) and performs the SAME computation; on the
	// OVER_LIMIT path its `headers.Set` becomes a no-op-set-equal idempotent
	// overwrite of these inline-applied values (byte-identical via the shared
	// buildXRateLimitHeaders source).
	if f.cc.enableXRateLimitHeaders {
		if limit, remaining, reset, ok := buildXRateLimitHeaders(resp.GetStatuses()); ok {
			headers = append(headers,
				envoyhttp.HeaderField{Name: headerXRateLimitLimit, Value: limit},
				envoyhttp.HeaderField{Name: headerXRateLimitRemaining, Value: remaining},
				envoyhttp.HeaderField{Name: headerXRateLimitReset, Value: reset},
			)
		}
	}

	// [c] Filter-config response headers, in config order. Pre-canonicalized
	// at Task-3 compileResponseHeaders.
	for _, kv := range f.cc.responseHeadersToAdd {
		headers = append(headers, envoyhttp.HeaderField{
			Name:  kv.name,
			Value: kv.value,
		})
	}

	// Status: cc.rateLimitedStatus (default 429; clamped <400→429 at Task 3).
	// Body: resp.RawBody verbatim (string conversion is byte-exact for HTTP
	// body purposes — Go's string + []byte share the same UTF-8 byte slice).
	f.dcb.SendLocalReply(f.cc.rateLimitedStatus, string(resp.GetRawBody()), headers)
	// ContinueDecoding() is required even after SendLocalReply: when the async
	// goroutine calls SendLocalReply from outside the dispatch goroutine, the
	// dispatch goroutine is still parked in parkDecode (waiting on
	// decodeResumeCh — chain.go:316-325). SendLocalReply alone sets
	// c.localReplyDone but does NOT unblock the park. ContinueDecoding()
	// wakes the parked goroutine so it can observe localReplyDone=true and
	// return. Mirrors the ext_authz deny-path precedent (extauthz.go:1097-1111)
	// and the phase-09 fault filter SendLocalReply+ContinueDecoding pattern
	// (fault.go:299-324).
	f.dcb.ContinueDecoding() // wake the parked dispatch goroutine
}

// ----------------------------------------------------------------------------
// applyError — RLS call errored OR returned UNKNOWN. Fork on cc.failureModeDeny.
// ----------------------------------------------------------------------------

// applyError applies the error arm per parent SPEC §4.6: the error counter
// ALWAYS increments; the failureModeDeny gate forks the response:
//
//   - false (DEFAULT, fail-OPEN): failure_mode_allowed counter Inc;
//     ContinueDecoding (admit the request).
//   - true  (fail-CLOSED): SendLocalReply(cc.statusOnError, "", nil) — no
//     body, no headers (the "nullptr-mutate" shape per upstream ratelimit.cc
//     error codepath; ABSENT-BY-API rc-details "rate_limiter_error").
//
// Phase 24.2 Task 5 (D-RL12 + ADR-0197 X-RateLimit slice): the optional
// `resp` argument carries the (possibly nil) RLS response so the fail-OPEN
// branch can capture per-descriptor statuses for X-RateLimit emission on the
// upstream-then-encode admit path. The fail-CLOSED branch MUST NOT store —
// the 500 nullptr-mutate path emits NO X-RateLimit per D-RL12.
func (f *filter) applyError(resp *ratelimitservicev3.RateLimitResponse) {
	if c := f.cc.stats.error; c != nil {
		c.Inc()
	}

	if !f.cc.failureModeDeny {
		// Fail-open: increment the fail-open counter + admit.
		if c := f.cc.stats.failureModeAllowed; c != nil {
			c.Inc()
		}
		// Phase 24.2 Task 5: store statuses on fail-OPEN when the RLS
		// returned a non-nil response (a partial-success / UNKNOWN-overall
		// envelope may still carry per-descriptor statuses). The upstream-
		// produced response then carries X-RateLimit at the encode phase.
		// On transport error (nil resp) the GetStatuses() reflection-safe
		// accessor returns nil — clean no-op.
		f.responseStatuses = resp.GetStatuses()
		f.dcb.ContinueDecoding()
		return
	}

	// Fail-closed: emit the statusOnError reply with empty body + nil headers.
	f.dcb.SendLocalReply(f.cc.statusOnError, "", nil)
	// ContinueDecoding() is required even after SendLocalReply: when the async
	// goroutine calls SendLocalReply from outside the dispatch goroutine, the
	// dispatch goroutine is still parked in parkDecode (waiting on
	// decodeResumeCh — chain.go:316-325). SendLocalReply alone sets
	// c.localReplyDone but does NOT unblock the park. ContinueDecoding()
	// wakes the parked goroutine so it can observe localReplyDone=true and
	// return. Mirrors the ext_authz failureModeDeny precedent
	// (extauthz.go:1146-1156) and the phase-09 fault filter precedent
	// (fault.go:299-324).
	f.dcb.ContinueDecoding() // wake the parked dispatch goroutine
}
