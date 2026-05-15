package extauthz

// attributes.go — HTTP-mode AuthorizationRequest builder + gRPC-mode AttributeContext
// builder + StringMatcher machinery.
//
// This file lands:
//   - stringMatcherList type (moved here from the placeholder in extauthz.go)
//   - compileStringMatcherList: ListStringMatcher → *stringMatcherList (error)
//   - buildAuthRequest: filter client request headers + append headers_to_add (18.1)
//   - validateMutationHeaders: validate_mutations rule set (authored here, consumed Task 5 of 18.1)
//   - buildAttributeContext: pure-function gRPC-mode AttributeContext builder (18.2 Task 5)
//   - 5 helpers (addressFromNetAddr / lowercaseHeaderMap / firstOrEmpty /
//     bodyStringIfNotBytes / bodyBytesIfBytes) consumed by buildAttributeContext
//
// ADR anchor: ADR-0160 (HTTP-mode portion lands at phase-18.1 Task 4;
// gRPC-mode portion lands at phase-18.2 Task 5).
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
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
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
// headers, then applies the pre-compiled static headers from cc.headersToAdd
// (AuthorizationRequest.headers_to_add, pre-compiled at buildCompiledConfig time).
//
// Parameters:
//   - f: the per-stream filter (carries f.activeRC for cc.allowedHeaders /
//     cc.disallowedHeaders / cc.headersToAdd access).
//   - headers: the incoming client request headers (from DecodeHeaders; Task 9
//     wires this; at Task 4 tests pass it directly).
//   - body: the buffered request body (nil when with_request_body is unset or
//     the body is empty). Task 6 wires the real body.
//   - path: the client's request path extracted from the :path pseudo-header
//     (path_prefix prepend is done in check.go's closure, NOT here —
//     buildAuthRequest stores the path as-is).
//
// D6 disposition (deprecated AuthorizationRequest.allowed_headers):
//
//	When cc.allowedHeaders (top-level primary path) is non-nil, the deprecated
//	field is ignored (top-level wins — cc.deprecatedAllowedHeaders is nulled
//	out in buildCompiledConfig when allowedHeaders is set). When cc.allowedHeaders
//	is nil AND cc.deprecatedAllowedHeaders is non-nil, the pre-compiled deprecated
//	field is used as the effective allow-list. Compilation is done ONCE at
//	config-load time in buildCompiledConfig; malformed patterns produce a
//	PARSE-REJECT at that point, not at request time.
//
// Task 9 review fix: removed hs *ext_authzv3.HttpService parameter. All data
// previously sourced from hs (headers_to_add, deprecated allowed_headers) is now
// pre-compiled into cc at buildCompiledConfig time and accessed via cc.headersToAdd
// + cc.deprecatedAllowedHeaders respectively.
func buildAuthRequest(f *filter, headers http.Header, body []byte, path string) *authRequest {
	cc := f.activeRC

	// Determine the effective allow-list (D6: top-level primary; deprecated
	// honored-if-present as fallback). Both fields are pre-compiled at
	// config-load time (buildCompiledConfig). No per-request compilation.
	effectiveAllowed := cc.allowedHeaders
	if effectiveAllowed == nil {
		effectiveAllowed = cc.deprecatedAllowedHeaders
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
	//
	// HTTP/2 pseudo-headers (:path, :method, :authority, :scheme) are skipped
	// unconditionally. They are NOT forwarded as regular HTTP/1.1 headers to the
	// auth service: Go's net/http client rejects `:` prefixed header names as
	// invalid (RFC 7230 §3.2). The path is extracted at the call site and passed
	// to buildAuthRequest via the path parameter; other pseudo-headers are not
	// forwarded in the 18.1 HTTP-mode implementation.
	filtered := make(http.Header)
	for name, values := range headers {
		// Skip HTTP/2 pseudo-headers — not valid as HTTP/1.1 header field names.
		if strings.HasPrefix(name, ":") {
			continue
		}
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

	// Apply pre-compiled headers_to_add from cc (overwrites same-key headers).
	// These are pre-compiled from AuthorizationRequest.headers_to_add at
	// buildCompiledConfig time so they are always present regardless of whether
	// the hs proto was passed at dispatch time.
	for _, kv := range cc.headersToAdd {
		filtered.Set(kv.name, kv.value)
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

// ---------------------------------------------------------------------------
// buildAttributeContext — gRPC-mode AttributeContext builder (ADR-0160 gRPC-mode
// portion; SPEC §6.6; phase-18.2 Task 5).
// ---------------------------------------------------------------------------
//
// Pure function of *authRequest + four boolean gates per SPEC §6.6 + the
// AMENDMENT (ADR-0165 / Amendment date 2026-05-15). NO DecoderFilterCallbacks
// parameter — all per-stream state is read from the extended *authRequest;
// the 6-method DecoderFilterCallbacks extension per ADR-0165 is consumed at
// the dispatchOutboundCheck seeding site (Task 8), NOT here.
//
// Populated set faithfully reproduces the §11.P4 RATIFIED in-session SPEC scrape:
//
//   - source = Peer{Address: socket_address from req.remoteAddr,
//     Principal: first-of(req.downstreamPrincipal)}
//   - destination = Peer{Address: socket_address from req.localAddr,
//     Principal: req.listenerPrincipal (AUTOMATIC)}
//   - request.http = AttributeContext_HttpRequest{Id, Method, Headers (lowercased map;
//     pseudo-headers + HCM-injected x-forwarded-proto / x-request-id /
//     x-envoy-auth-partial-body INCLUDED), Path, Host, Scheme,
//     Size, Protocol, Body | RawBody (per pack_as_bytes)}
//   - request.time = Timestamp from req.streamStartTime (or time.Now() if zero)
//   - tls_session.sni — populated IFF includeTlsSession AND req.tlsServerName != ""
//     (the conditional gate per §11.P4); ONLY sni populated.
//   - source.certificate — populated IFF includePeerCert AND len(req.peerCertDER) > 0
//     (the conditional gate per parent §5.P3).
//   - encode_raw_headers — D6 in-this-task DEFERRED for MVP: when true, the
//     header_map field is NOT populated (no-op); the legacy headers map suffices
//     for fixture 0021's byte-equivalence assertion per §11.P4 + SPEC §12 item 6.
//   - context_extensions = req.perRouteContextExtensions (empty for MVP-no-per-route
//     per SPEC §5; Task 7 wires the merge).
//   - metadata_context = empty *core.Metadata (NOT nil — the §11.P4 in-session
//     scrape shows `metadataContext: {}` rendered).
//   - route_metadata_context = empty *core.Metadata (NOT nil — same as above).
//
// Returns a freshly-allocated *authv3.AttributeContext suitable for shipping
// in a *authv3.CheckRequest.
func buildAttributeContext(
	req *authRequest,
	encodeRawHeaders bool,
	packAsBytes bool,
	includePeerCert bool,
	includeTlsSession bool,
) *authv3.AttributeContext {
	// Step 1: source = Peer{address, principal}. Principal sourced from the
	// ADR-0144 DownstreamPrincipal() join-list — first element (or "") per
	// SPEC §6.6 step 1 + parent §5.P3.
	source := &authv3.AttributeContext_Peer{
		Address:   addressFromNetAddr(req.remoteAddr),
		Principal: firstOrEmpty(req.downstreamPrincipal),
	}

	// Step 2: destination = Peer{address, principal}. Listener-principal
	// populated AUTOMATICALLY (NOT gated by include_peer_certificate) per
	// §11.P4 in-session SPEC scrape — see the "destination.principal populates
	// AUTOMATICALLY from the listener TLS cert" finding.
	destination := &authv3.AttributeContext_Peer{
		Address:   addressFromNetAddr(req.localAddr),
		Principal: req.listenerPrincipal,
	}

	// Step 3: request.http — pseudo-headers + HCM-injected headers INCLUDED in
	// the lowercased headers map per §11.P4. The :authority / :scheme pseudo-
	// header values are also surfaced separately on Host / Scheme (proto
	// convention — these duplicate the headers-map entries).
	httpReq := &authv3.AttributeContext_HttpRequest{
		Id:       req.requestID,
		Method:   req.method,
		Headers:  lowercaseHeaderMap(req.headers),
		Path:     req.path,
		Host:     req.headers.Get(":authority"),
		Scheme:   req.headers.Get(":scheme"),
		Size:     int64(len(req.body)),
		Protocol: req.protocol,
		Body:     bodyStringIfNotBytes(req.body, packAsBytes),
		RawBody:  bodyBytesIfBytes(req.body, packAsBytes),
	}

	// Step 7: encode_raw_headers — DEFERRED for MVP per SPEC §6.6 step 7 + §8
	// item 8. When the flag is true the proto's header_map field MAY be populated
	// in lieu of (or in addition to) the legacy headers map. The 18.2 IMPL keeps
	// the legacy headers map and leaves header_map nil — the §11.P4 RATIFIED
	// scrape shows reference Envoy populates the legacy headers map by default,
	// and fixture 0021's byte-equivalence assertion is satisfied by that map. The
	// flag PARSES (config-side) but produces no AttributeContext difference.
	_ = encodeRawHeaders // flag honored as no-op for MVP per §8 item 8

	// Step 4: request.time — timestamppb.New(req.streamStartTime), or
	// timestamppb.Now() when streamStartTime is the zero value (SPEC §6.6
	// step 4 IMPL settles for the zero-value fallback). The Group 12 tests
	// assert non-zero AsTime() output, which both arms satisfy.
	t := req.streamStartTime
	if t.IsZero() {
		t = time.Now()
	}

	request := &authv3.AttributeContext_Request{
		Time: timestamppb.New(t),
		Http: httpReq,
	}

	// Step 9: metadata_context + route_metadata_context populated as EMPTY
	// proto messages (NOT nil pointers) per §11.P4 in-session evidence
	// ("`metadataContext` + `routeMetadataContext` render as empty objects `{}`
	// even with no fields set"). Deferred dynamic-metadata family per SPEC §8
	// item 1 — the empty-message shape is the forward-compat placeholder.
	ac := &authv3.AttributeContext{
		Source:               source,
		Destination:          destination,
		Request:              request,
		MetadataContext:      &corev3.Metadata{},
		RouteMetadataContext: &corev3.Metadata{},
		// Step 8: context_extensions — req.perRouteContextExtensions merged at
		// resolve time (empty for MVP-no-per-route per SPEC §5; Task 7 wires it).
		ContextExtensions: req.perRouteContextExtensions,
	}

	// Step 5: tls_session.sni — gated by includeTlsSession AND req.tlsServerName != ""
	// per §11.P4 RATIFICATION. ONLY sni populated; other TLSSession fields
	// (currently the proto only defines sni at v1.32.4) stay empty.
	if includeTlsSession && req.tlsServerName != "" {
		ac.TlsSession = &authv3.AttributeContext_TLSSession{Sni: req.tlsServerName}
	}

	// Step 6: source.certificate — gated by includePeerCert AND
	// len(req.peerCertDER) > 0 per parent §5.P3. The proto field type is
	// `string`; the proto docs say "the certificate contents are encoded in URL
	// and PEM format". envoy-go MVP stores the raw DER bytes coerced to string
	// (the IMPL settles per cost — URL+PEM encoding can be added later if a
	// behavior delta surfaces against reference Envoy v1.37.2). The §11.P4
	// in-session scrape did not exercise this field (curl presented no client
	// cert), so the encoding choice is a 18.2 IMPL settle documented in the
	// ADR-0160 gRPC-mode-portion §Consequences.
	if includePeerCert && len(req.peerCertDER) > 0 {
		source.Certificate = string(req.peerCertDER)
	}

	// Step 10: return.
	return ac
}

// ---------------------------------------------------------------------------
// buildAttributeContext helpers (consumed by the function above).
// ---------------------------------------------------------------------------

// addressFromNetAddr wraps a net.Addr into a *core.Address with the
// SocketAddress arm + PortValue port specifier. The IP and port are extracted
// from the underlying *net.TCPAddr (the canonical type used by net.Listen +
// (*net.TCPConn).RemoteAddr/LocalAddr).
//
// A nil net.Addr returns nil — the caller must accept a nil Address (proto
// will marshal it as absence of the field). A non-TCPAddr type returns a
// best-effort Address parsed via SplitHostPort.
//
// The address string uses IP.String() to handle IPv4 / IPv6 formatting per
// the net.IP convention (IPv4 → dotted-quad; IPv6 → bracketed-form NOT applied
// at the SocketAddress.address level — Envoy's proto convention is bare IP,
// no brackets).
func addressFromNetAddr(a net.Addr) *corev3.Address {
	if a == nil {
		return nil
	}
	var ip string
	var port uint32
	switch v := a.(type) {
	case *net.TCPAddr:
		if v.IP != nil {
			ip = v.IP.String()
		}
		port = uint32(v.Port)
	default:
		// Best-effort: SplitHostPort on the canonical "host:port" string form.
		// Unparseable addresses produce an empty IP + zero port.
		h, p, err := net.SplitHostPort(a.String())
		if err == nil {
			ip = h
			var pn int
			if _, err := fmt.Sscanf(p, "%d", &pn); err == nil {
				port = uint32(pn)
			}
		}
	}
	return &corev3.Address{
		Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address: ip,
				PortSpecifier: &corev3.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
}

// lowercaseHeaderMap converts an http.Header (canonicalized keys) into a
// map[string]string with lowercased keys, joining multi-value headers with
// "," per the §11.P4 reference Envoy convention (the in-session scrape shows
// single-value rendering — Envoy's internal lowercase + comma-join discipline
// for the AttributeContext.request.http.headers map).
//
// HTTP/2 pseudo-headers (:authority, :method, :path, :scheme) are INCLUDED
// (NOT skipped — unlike HTTP-mode buildAuthRequest, which strips them because
// net/http rejects ":"-prefixed names; the gRPC-mode AttributeContext.headers
// map is a proto map and accepts pseudo-headers verbatim per §11.P4).
//
// A nil input returns an empty (non-nil) map — the proto map field is then
// marshaled as `{}` rather than absent, matching the reference Envoy shape.
func lowercaseHeaderMap(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		lk := strings.ToLower(k)
		if len(vs) == 0 {
			out[lk] = ""
			continue
		}
		if len(vs) == 1 {
			out[lk] = vs[0]
			continue
		}
		// Multi-value join with "," per reference Envoy v1.37.2 internal
		// lowercase+comma-join discipline (HTTP semantics: equivalent to a
		// single value containing the joined string).
		out[lk] = strings.Join(vs, ",")
	}
	return out
}

// firstOrEmpty returns the first element of a string slice, or "" when the
// slice is empty or nil. Used to extract the source.principal from the
// req.downstreamPrincipal (ADR-0144) joined-string slice per SPEC §6.6 step 1.
func firstOrEmpty(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return ss[0]
}

// bodyStringIfNotBytes returns string(body) when !packAsBytes, else "".
// Used to populate AttributeContext.request.http.Body (string-arm of the
// body/raw_body pair per SPEC §6.6 step 3 + ADR-0162 pack_as_bytes
// gRPC-mode differentiator).
func bodyStringIfNotBytes(body []byte, packAsBytes bool) string {
	if packAsBytes {
		return ""
	}
	return string(body)
}

// bodyBytesIfBytes returns body when packAsBytes, else nil.
// Used to populate AttributeContext.request.http.RawBody (bytes-arm).
func bodyBytesIfBytes(body []byte, packAsBytes bool) []byte {
	if !packAsBytes {
		return nil
	}
	return body
}
