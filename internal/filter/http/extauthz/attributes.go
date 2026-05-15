package extauthz

// attributes.go — HTTP-mode AuthorizationRequest builder + StringMatcher machinery.
//
// This file lands:
//   - stringMatcherList type (moved here from the placeholder in extauthz.go)
//   - compileStringMatcherList: ListStringMatcher → *stringMatcherList (error)
//   - buildAuthRequest: filter client request headers + append headers_to_add
//   - validateMutationHeaders: validate_mutations rule set (authored here, consumed Task 5)
//
// ADR anchor: ADR-0160 (HTTP-mode portion).
//
// Design: The stringMatcherList type is defined here alongside its constructor
// compileStringMatcherList and its matchAny method, because all three are
// tightly coupled — matching the rbac evaluator.go pattern of keeping the type,
// its constructor, and its methods together in one file. The placeholder type
// declared in extauthz.go (Task 2) is replaced: the full definition lives here.
//
// compileStringMatcherList:
//   Compiles a *matcherv3.ListStringMatcher into a *stringMatcherList.
//   Each *matcherv3.StringMatcher element is compiled individually (same
//   switch-on-MatchPattern logic as rbac/evaluator.go matchString + the
//   internal/matcher compileStringMatcher — reused in spirit, not called
//   directly since those operate on a different variant type or a different
//   interface shape). D5: safe_regex with google_re2 engine arm is honored
//   (Go regexp, RE2-compatible); other engine arms and nil safe_regex
//   PARSE-REJECT. custom arm PARSE-REJECT (no extension registry).
//
// buildAuthRequest (D6 disposition):
//   Request-side header filtering:
//     1. Determine the effective allow-list:
//        - If cc.allowedHeaders != nil: use it (top-level primary path).
//        - Else if hs.AuthorizationRequest.AllowedHeaders != nil: compile and
//          use the deprecated field (honored-if-present, backward-compat).
//        - Else: nil = all-pass.
//     2. Filter incoming headers: if effective allow-list != nil, keep only
//        headers where matchAny returns true (case-insensitive header name
//        comparison per HTTP/1.1 semantics).
//     3. Remove headers matching cc.disallowedHeaders (overrides allowed).
//     4. Set static headers from hs.AuthorizationRequest.HeadersToAdd
//        (overwrites any header with the same canonical key).
//
// validateMutationHeaders (D7; consumed at Task 5):
//   Validates headerKV slice per the phase-10 header_mutation protected-header
//   discipline: :-prefixed pseudo-headers REJECTED; invalid header-name
//   characters REJECTED; invalid header-value characters REJECTED.

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	ext_authzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

// ---------------------------------------------------------------------------
// stringMatcherList — compiled form of a ListStringMatcher proto.
// Defined here alongside compileStringMatcherList; the placeholder in
// extauthz.go is replaced by this definition at Task 4.
// ---------------------------------------------------------------------------

// stringMatcherList is the compiled form of a ListStringMatcher proto.
// It holds a slice of compiled individual matchers; matchAny returns true
// if any single matcher matches (OR semantics). An empty matchers slice
// matches nothing. A nil *stringMatcherList pointer means "all pass" (the
// caller is responsible for the nil check).
type stringMatcherList struct {
	matchers []compiledStringMatcher
}

// compiledStringMatcher is the internal interface for a single compiled
// StringMatcher pattern. Mirrors the stringMatcherEval interface from
// internal/matcher but operates on the envoy type/matcher/v3 proto variant.
type compiledStringMatcher interface {
	matches(candidate string) bool
}

// matchAny returns true if candidate matches any of the compiled matchers
// in the list (OR semantics). An empty list matches nothing.
//
// Header name matching is case-insensitive at the call-site (the callers in
// buildAuthRequest lower-case or use canonical header names before calling
// matchAny). Individual compiled matchers honor their own ignore_case flag.
func (sml *stringMatcherList) matchAny(candidate string) bool {
	if sml == nil {
		return false
	}
	for _, m := range sml.matchers {
		if m.matches(candidate) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// compileStringMatcherList — ListStringMatcher → *stringMatcherList
// ---------------------------------------------------------------------------

// compileStringMatcherList compiles a *matcherv3.ListStringMatcher into an
// internal *stringMatcherList. The compilation is proto-faithful per
// planner-time decision D5 (ADR-0160):
//
//   - Exact / Prefix / Suffix / Contains: compiled with ignore_case honored.
//   - SafeRegex: google_re2 engine arm honored (Go regexp, RE2-compatible);
//     nil RegexMatcher → PARSE-REJECT; invalid regex → PARSE-REJECT.
//     Other engine arms: there is only one valid arm in the proto (google_re2);
//     the default arm handles nil EngineType.
//   - Custom: PARSE-REJECT (no envoy-go string-matcher extension registry).
//   - Nil MatchPattern: PARSE-REJECT.
//
// A nil lsm input → nil return (no error): nil means "no matcher configured".
// A non-nil lsm with zero patterns → non-nil *stringMatcherList with no
// matchers (matches nothing).
//
// NOTE: The function signature changes from the Task 2 stub — the stub
// returned (*stringMatcherList) only; this returns (*stringMatcherList, error)
// per the real implementation requirement. buildCompiledConfig in extauthz.go
// is updated at Task 4 Step 4 to handle the error return.
func compileStringMatcherList(lsm *matcherv3.ListStringMatcher) (*stringMatcherList, error) {
	if lsm == nil {
		return nil, nil
	}
	patterns := lsm.GetPatterns()
	sml := &stringMatcherList{
		matchers: make([]compiledStringMatcher, 0, len(patterns)),
	}
	for i, sm := range patterns {
		m, err := compileOneStringMatcher(sm)
		if err != nil {
			return nil, fmt.Errorf("ext_authz: string_matcher[%d]: %w", i, err)
		}
		sml.matchers = append(sml.matchers, m)
	}
	return sml, nil
}

// compileOneStringMatcher compiles a single *matcherv3.StringMatcher into a
// compiledStringMatcher. Called per-pattern by compileStringMatcherList.
func compileOneStringMatcher(sm *matcherv3.StringMatcher) (compiledStringMatcher, error) {
	if sm == nil {
		return nil, errors.New("nil StringMatcher")
	}
	ic := sm.GetIgnoreCase()
	switch mp := sm.GetMatchPattern().(type) {
	case *matcherv3.StringMatcher_Exact:
		return &smExact{want: mp.Exact, ignoreCase: ic}, nil

	case *matcherv3.StringMatcher_Prefix:
		return &smPrefix{prefix: mp.Prefix, ignoreCase: ic}, nil

	case *matcherv3.StringMatcher_Suffix:
		return &smSuffix{suffix: mp.Suffix, ignoreCase: ic}, nil

	case *matcherv3.StringMatcher_Contains:
		return &smContains{needle: mp.Contains, ignoreCase: ic}, nil

	case *matcherv3.StringMatcher_SafeRegex:
		if mp.SafeRegex == nil {
			return nil, errors.New("safe_regex: nil RegexMatcher")
		}
		// D5: google_re2 engine arm honored; other engine arms:
		// The proto only defines google_re2 as a valid engine type;
		// a nil EngineType (unset oneof) is treated as google_re2 per
		// the reference Envoy v1.37.2 behavior (the google_re2 arm is the
		// only valid arm in the v1.37.2 proto; treating nil as an implicit
		// google_re2 mirrors the rbac evaluator.go matchString behavior).
		// Future non-google_re2 arms would surface as unknown types here.
		re, err := regexp.Compile(mp.SafeRegex.GetRegex())
		if err != nil {
			return nil, fmt.Errorf("safe_regex compile: %w", err)
		}
		return &smRegex{re: re}, nil

	case *matcherv3.StringMatcher_Custom:
		// PARSE-REJECT: no envoy-go string-matcher extension registry.
		// custom is a TypedExtensionConfig plugin point; envoy-go-strict
		// rejects it at parse time (mirrors internal/matcher precedent).
		return nil, errors.New("custom string matcher extension unsupported in this build")

	case nil:
		return nil, errors.New("StringMatcher match_pattern oneof unset")

	default:
		return nil, fmt.Errorf("unknown StringMatcher pattern %T", mp)
	}
}

// ---------------------------------------------------------------------------
// compiledStringMatcher implementations.
// ---------------------------------------------------------------------------

type smExact struct {
	want       string
	ignoreCase bool
}

func (m *smExact) matches(s string) bool {
	if m.ignoreCase {
		return strings.EqualFold(m.want, s)
	}
	return m.want == s
}

type smPrefix struct {
	prefix     string
	ignoreCase bool
}

func (m *smPrefix) matches(s string) bool {
	if m.ignoreCase {
		return len(s) >= len(m.prefix) && strings.EqualFold(s[:len(m.prefix)], m.prefix)
	}
	return strings.HasPrefix(s, m.prefix)
}

type smSuffix struct {
	suffix     string
	ignoreCase bool
}

func (m *smSuffix) matches(s string) bool {
	if m.ignoreCase {
		if len(s) < len(m.suffix) {
			return false
		}
		return strings.EqualFold(s[len(s)-len(m.suffix):], m.suffix)
	}
	return strings.HasSuffix(s, m.suffix)
}

type smContains struct {
	needle     string
	ignoreCase bool
}

func (m *smContains) matches(s string) bool {
	if m.ignoreCase {
		return strings.Contains(strings.ToLower(s), strings.ToLower(m.needle))
	}
	return strings.Contains(s, m.needle)
}

type smRegex struct {
	re *regexp.Regexp
}

func (m *smRegex) matches(s string) bool {
	return m.re.MatchString(s)
}

// ---------------------------------------------------------------------------
// buildAuthRequest — HTTP-mode AuthorizationRequest builder (ADR-0160).
// ---------------------------------------------------------------------------

// buildAuthRequest constructs the *authRequest for the outbound auth check POST.
// It filters client request headers through the effective allow-list + disallowed
// headers, then appends static headers from AuthorizationRequest.headers_to_add.
//
// Parameters:
//   - f: the per-stream filter (carries f.activeRC for cc.allowedHeaders /
//     cc.disallowedHeaders access).
//   - hs: the parsed HttpService (carries AuthorizationRequest with headers_to_add
//   - deprecated allowed_headers).
//   - headers: the incoming client request headers (from DecodeHeaders; Task 9
//     wires this; at Task 4 tests pass it directly).
//   - body: the buffered request body (nil when with_request_body is unset or
//     the body is empty). Task 6 wires the real body.
//   - path: the request path (path_prefix prepend is done in check.go's closure,
//     NOT here — buildAuthRequest stores the path as-is).
//
// D6 disposition (deprecated AuthorizationRequest.allowed_headers):
//
//	When cc.allowedHeaders (top-level primary path) is non-nil, the deprecated
//	field is ignored (top-level wins). When cc.allowedHeaders is nil AND
//	hs.AuthorizationRequest.AllowedHeaders is non-nil, the deprecated field is
//	compiled and used as the effective allow-list (honored-if-present for
//	backward-compat). If compilation of the deprecated field fails (malformed
//	pattern), the filter falls through to nil (all-pass) to avoid hard errors
//	from a deprecated field — consistent with the "honored-if-present /
//	silent-degrade" intent.
func buildAuthRequest(f *filter, hs *ext_authzv3.HttpService, headers http.Header, body []byte, path string) *authRequest {
	cc := f.activeRC

	// Determine the effective allow-list (D6: top-level primary; deprecated
	// honored-if-present as fallback).
	effectiveAllowed := cc.allowedHeaders
	if effectiveAllowed == nil && hs != nil {
		ar := hs.GetAuthorizationRequest()
		if ar != nil && ar.GetAllowedHeaders() != nil {
			// D6: deprecated field present; compile it.
			compiled, err := compileStringMatcherList(ar.GetAllowedHeaders())
			if err == nil {
				effectiveAllowed = compiled
			}
			// On error: fall through to nil (all-pass) — deprecated-field
			// failures should not hard-reject the request at runtime.
		}
	}

	// Build the filtered header set.
	// Header names in http.Header are in canonical form (Title-Case) per the
	// net/http package (e.g. "Authorization", "X-Forwarded-For"). Envoy's
	// internal representation lowercases header names; the allowed_headers /
	// disallowed_headers matchers in Envoy configs are written against lowercase
	// header names. To honor proto-faithful matching (e.g. the proto example
	// `exact: "authorization"` matching the `Authorization` header), we match
	// against the lowercased header name. This is consistent with reference Envoy
	// v1.37.2 behavior where all header names are lowercased before matching.
	filtered := make(http.Header)
	for name, values := range headers {
		// Use the lowercased header name for matcher evaluation to honor the
		// Envoy convention (header names in configs are lowercase).
		nameLower := strings.ToLower(name)
		if effectiveAllowed != nil && !effectiveAllowed.matchAny(nameLower) {
			// Not in the allow-list — skip.
			continue
		}
		// Remove headers matching disallowedHeaders (overrides allowed).
		if cc.disallowedHeaders != nil && cc.disallowedHeaders.matchAny(nameLower) {
			continue
		}
		// Include this header (preserving the canonical form as stored).
		filtered[name] = values
	}

	// Apply headers_to_add from AuthorizationRequest (overwrites same-key headers).
	if hs != nil {
		ar := hs.GetAuthorizationRequest()
		if ar != nil {
			for _, hv := range ar.GetHeadersToAdd() {
				if hv.GetKey() == "" {
					continue
				}
				canonical := http.CanonicalHeaderKey(hv.GetKey())
				filtered.Set(canonical, hv.GetValue())
			}
		}
	}

	return &authRequest{
		method:  http.MethodPost, // HTTP-mode always POSTs per SPEC §6.5
		path:    path,
		headers: filtered,
		body:    body,
	}
}

// ---------------------------------------------------------------------------
// validateMutationHeaders — validate_mutations rule set (D7; Task 5 consumes).
// ---------------------------------------------------------------------------

// validateMutationHeaders validates a slice of headerKV pairs per the
// planner-time decision D7 rule set — the phase-10 header_mutation
// protected-header discipline:
//
//  1. :-prefixed pseudo-headers REJECTED.
//  2. Invalid header-name characters REJECTED (space, control chars, etc.).
//  3. Invalid header-value characters REJECTED (bare CR, LF, NUL).
//
// Returns an error on the FIRST violation (short-circuit). The caller
// (Task 5) drives the invalid disposition + invalid counter increment on error.
//
// NOTE: This function is AUTHORED here but NOT YET WIRED into the disposition
// path — that lands at Task 5. It is exposed here for unit testing (Group 3).
func validateMutationHeaders(hdrs []headerKV) error {
	for _, hdr := range hdrs {
		if err := validateMutationHeaderName(hdr.name); err != nil {
			return err
		}
		if err := validateMutationHeaderValue(hdr.value); err != nil {
			return fmt.Errorf("header %q: %w", hdr.name, err)
		}
	}
	return nil
}

// validateMutationHeaderName validates a header name per D7:
//  1. :-prefixed pseudo-headers → rejected (mirrors isProtectedHeader in header_mutation).
//  2. Invalid HTTP token characters → rejected.
//
// HTTP header names are "token" per RFC 7230 §3.2: tchar = "!" / "#" / "$" /
// "%" / "&" / "'" / "*" / "+" / "-" / "." / "^" / "_" / "`" / "|" / "~" /
// DIGIT / ALPHA. Space, CTL, separators are invalid.
func validateMutationHeaderName(name string) error {
	if name == "" {
		return errors.New("ext_authz: header name must not be empty")
	}
	// :-prefixed pseudo-headers are protected per D7.
	if strings.HasPrefix(name, ":") {
		return fmt.Errorf("ext_authz: header name %q is a :-prefixed pseudo-header; mutations rejected per D7", name)
	}
	// Validate HTTP token characters per RFC 7230 §3.2.6.
	for i := 0; i < len(name); i++ {
		if !isTokenChar(name[i]) {
			return fmt.Errorf("ext_authz: header name %q contains invalid character at position %d (0x%02x)", name, i, name[i])
		}
	}
	return nil
}

// validateMutationHeaderValue validates a header value per D7:
// bare CR, LF, or NUL in header values are invalid per RFC 7230 §3.2.6.
// (CRLF folding is obsoleted; bare CR/LF/NUL inject headers or corrupt the
// response.)
func validateMutationHeaderValue(value string) error {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == 0x00 || c == '\n' || c == '\r' {
			return fmt.Errorf("ext_authz: header value contains invalid character at position %d (0x%02x)", i, c)
		}
	}
	return nil
}

// isTokenChar returns true if c is a valid HTTP token character per
// RFC 7230 §3.2.6. Token chars: ALPHA / DIGIT and the tchar symbols
// "!" "#" "$" "%" "&" "'" "*" "+" "-" "." "^" "_" "`" "|" "~".
// Space (0x20) and control characters (0x00–0x1f, 0x7f) are NOT token chars.
// Separator characters (, ; = () <> @{}\/"[]) are also excluded.
func isTokenChar(c byte) bool {
	// Fast path: control chars and DEL are always invalid.
	if c <= 0x1f || c == 0x7f {
		return false
	}
	// Separators per RFC 7230 §3.2.6.
	switch c {
	case '(', ')', '<', '>', '@', ',', ';', ':', '\\', '"', '/', '[', ']', '?', '=', '{', '}', ' ', '\t':
		return false
	}
	return true
}

// buildAuthRequest is the entry point called from check.go's closure
// (and from Task 9's DecodeHeaders dispatch) to construct the *authRequest
// for the outbound auth check POST. See the function definition above.
// Task 9 wires f.dcb.RequestHeaders() → headers and the body accumulator;
// until Task 9 the closure passes the authRequest.headers directly (STUBBED).
