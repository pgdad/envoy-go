package ratelimit

// encode.go — the per-stream encode-side hook for X-RateLimit DRAFT_VERSION_03
// header injection per parent SPEC §6.6 + ADR-0197 X-RateLimit slice + the
// AMEND-8 header order. Replaces the 24.1 EncodeHeaders STUB at
// ratelimit.go:240 per D-RL7.
//
// # Per-disposition behavior (D-RL12 + AMEND-8)
//
//	+-----------------+--------------------------+---------------------------+
//	| Disposition     | dispositions.go arm      | X-RateLimit emission      |
//	+-----------------+--------------------------+---------------------------+
//	| OK              | applyOK   → ContinueDec  | YES — stored statuses     |
//	|                 |                           | read at EncodeHeaders     |
//	+-----------------+--------------------------+---------------------------+
//	| OVER_LIMIT 429  | applyOverLimit →          | YES — encode chain runs   |
//	|                 |  SendLocalReply (encode   | synchronously inside      |
//	|                 |  chain runs synchronously)| beginLocalReply; this     |
//	|                 |                           | filter's EncodeHeaders is |
//	|                 |                           | invoked (chain.go:1254)   |
//	+-----------------+--------------------------+---------------------------+
//	| fail-OPEN err   | applyError(failOpen=true) | YES — ContinueDecoding    |
//	|                 |  → ContinueDecoding       | wakes dispatch; encode    |
//	|                 |                           | chain runs upstream-then- |
//	|                 |                           | encode.                   |
//	+-----------------+--------------------------+---------------------------+
//	| fail-CLOSED err | applyError(failOpen=false)| NO — statuses NEVER       |
//	|                 |  → SendLocalReply(status, | stored on this path per   |
//	|                 |  "", nil) — nullptr-mutate| D-RL12; EncodeHeaders     |
//	|                 |                           | sees nil responseStatuses |
//	|                 |                           | and no-ops.               |
//	+-----------------+--------------------------+---------------------------+
//
// # Re-entrancy / lock discipline
//
// dispositions.go's OVER_LIMIT + fail-CLOSED arms call SendLocalReply, which
// synchronously enters the encode chain (chain.go::beginLocalReply runs
// RunEncodeHeaders inline). The encode chain dispatch calls this filter's
// EncodeHeaders from the SAME goroutine that holds f.mu via applyDisposition.
//
// To avoid re-entrant mu acquisition (sync.Mutex is non-reentrant in Go), the
// responseStatuses field on *filter is stored as a plain assignment under
// f.mu in dispositions.go and READ here WITHOUT acquiring f.mu. The
// happens-before is supplied by either:
//   - (OK / fail-OPEN) the dispatch goroutine's ContinueDecoding signal +
//     subsequent encode-chain dispatch sequencing
//   - (OVER_LIMIT) the synchronous SendLocalReply → RunEncodeHeaders call
//     chain from within the SAME goroutine (trivially happens-after the
//     store)
//
// Both paths sequence the store-then-read through Go's memory model without
// requiring re-entrant locking. The race detector should observe no races
// because the store completes BEFORE the SendLocalReply call (in case OVER_
// LIMIT) AND BEFORE ContinueDecoding signals the dispatch goroutine (in
// cases OK / fail-OPEN). OnDestroy guards via f.done before calling
// applyDisposition, so a post-OnDestroy store is impossible.
//
// # Cross-references
//
//   - headers.go (buildXRateLimitHeaders byte construction)
//   - dispositions.go (the store-statuses sites on OK/OVER_LIMIT/fail-OPEN)
//   - ratelimit.go (EncodeHeaders dispatches here)
//   - parent SPEC §4.7 + §6.6 + AMEND-8
//   - ADR-0197 X-RateLimit slice (§Decision amendment at this Task's commit)
//   - upstream ratelimit_headers.cc:13-65 at v1.37.2

import (
	"net/http"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// encodeHeaders is the body of EncodeHeaders dispatched from ratelimit.go.
// Injects the three X-RateLimit DRAFT_VERSION_03 headers onto the response
// headers map when:
//   - cc.enableXRateLimitHeaders == true (DRAFT_VERSION_03 enabled at config)
//   - f.responseStatuses != nil + non-empty + ≥1 status carries current_limit
//
// Returns envoyhttp.Continue unconditionally — header injection does not
// gate the chain.
func (f *filter) encodeHeaders(headers http.Header) envoyhttp.FilterHeadersStatus {
	// Gate 1: filter-config gate. DRAFT_VERSION_03 OFF (the proto-zero
	// default RateLimit_OFF) ⇒ NO emission on any disposition.
	if f.cc == nil || !f.cc.enableXRateLimitHeaders {
		return envoyhttp.Continue
	}

	// Gate 2: no stored statuses ⇒ no headers to emit. This is the
	// disposition-aware nil-suppression that implements D-RL12: the fail-
	// CLOSED 500 path never stores statuses (dispositions.go::applyError on
	// failOpen=false) so this no-ops cleanly. The zero-descriptor short-
	// circuit at DecodeHeaders (no RLS call) also leaves responseStatuses
	// nil (no statuses to surface — match upstream behavior of NOT emitting
	// the headers in that case).
	statuses := f.responseStatuses
	if len(statuses) == 0 {
		return envoyhttp.Continue
	}

	// Build the byte-shape per upstream ratelimit_headers.cc:13-65.
	limit, remaining, reset, ok := buildXRateLimitHeaders(statuses)
	if !ok {
		// No status carried current_limit ⇒ upstream returns empty header
		// map without injecting any of the three headers. Mirror verbatim.
		return envoyhttp.Continue
	}

	// Inject the three headers. http.Header.Set canonicalizes the key
	// (textproto.CanonicalMIMEHeaderKey: lowercase ⇒ "X-Ratelimit-Limit"
	// etc.); HTTP headers are case-insensitive on the wire so canonical-case
	// is semantically equivalent. The cors filter precedent at
	// cors/cors.go:138 uses Set verbatim with mixed-case input.
	headers.Set(headerXRateLimitLimit, limit)
	headers.Set(headerXRateLimitRemaining, remaining)
	headers.Set(headerXRateLimitReset, reset)

	return envoyhttp.Continue
}
