package http

import (
	"crypto/tls"
	"net"
	"net/http"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
)

// DecoderFilterCallbacks is the framework-supplied callback shape for
// decode-side filters. Every method except RequestRouteConfig is safe to call
// from any goroutine (per ADR-0071's async-resume mechanics).
type DecoderFilterCallbacks interface {
	// ContinueDecoding wakes the dispatch goroutine if it is parked on a
	// StopIteration return. Idempotent: duplicate calls are coalesced via the
	// chain's per-stream resume channel (capacity 1, non-blocking send).
	ContinueDecoding()

	// SendLocalReply synthesizes a response that enters the encode chain at
	// filter[len-1] (per ADR-0075 + SPEC §11 #4 empirical pin). First-call-wins
	// via sync.Once on the chain; second-call-after-encode-started is a no-op
	// + log line.
	//
	// Per Task 18 review (SPEC §11.2 ordered-headers compliance): the headers
	// parameter is an ordered (name, value) carrier (OrderedHeaders) so the
	// caller-supplied insertion order survives the chain's encode-iteration
	// and the wire-write layer. The unordered http.Header map cannot preserve
	// the §11.2 verbatim 6-header order — Go map iteration is non-deterministic
	// and net/http's Header.Write emits keys alphabetically sorted.
	SendLocalReply(status int, body string, headers OrderedHeaders)

	// RequestRouteConfig returns the merged proto.Message for the calling
	// filter's name (Route > VirtualHost > RouteConfiguration most-specific
	// override per ADR-0073). Nil if no per-route config applies. Lazy-cached
	// on first lookup per request.
	RequestRouteConfig() proto.Message

	// RequestRouteConfigsAllTiers returns the parsed per-route config at each
	// of the three tiers (Route, VirtualHost, RouteConfiguration), UNMERGED.
	// Used by filters whose semantics require multi-tier evaluation rather
	// than most-specific override — primarily envoy.filters.http.header_mutation
	// per its most_specific_header_mutations_wins flag (see ADR-0110 amending
	// ADR-0073). The default RequestRouteConfig method (per ADR-0073) remains
	// the canonical accessor for filters that use most-specific override
	// (cors, fault).
	//
	// Per phase 10 PLAN planner-time decision 1: this callback lives ONLY on
	// DecoderFilterCallbacks (NOT on EncoderFilterCallbacks). Filters that
	// need it on the encode side use the dcb reference set via
	// SetDecoderCallbacks (the framework wires both dcb and ecb on a both-
	// sides filter). The cors precedent at cors.go:163 (routePolicy) calls
	// f.dcb.RequestRouteConfig() from EncodeHeaders — same pattern applies.
	//
	// Returns (nil, nil, nil) when:
	//   - the chain has no perRoute config;
	//   - no scope at any tier carries an entry for the calling filter's name.
	RequestRouteConfigsAllTiers() (route, vhost, rc proto.Message)

	// EncodeHeaders / EncodeData / EncodeTrailers are encode-side injection
	// methods for filters that synthesize responses without using
	// SendLocalReply. Rare; intended for filters like header_manipulation
	// that need to inject encode-side material from a decode-side context.
	EncodeHeaders(headers http.Header, endStream bool)
	EncodeData(data []byte, endStream bool)
	EncodeTrailers(trailers http.Header)

	// DownstreamPrincipal returns the priority-ordered TLS principal-name
	// candidates from the downstream client connection: URI SANs first, then
	// DNS SANs, then the Subject DN Common Name as fallback. Returns nil (or
	// empty) for plaintext / non-mTLS connections or connections where no
	// client cert was presented. The slice mirrors Envoy v1.37.2's
	// Principal_Authenticated extraction semantics per rbac.pb.go:1432-1438.
	//
	// Per ADR-0144 §Decision (i)+(ii) (phase-16 framework primitive; lands at
	// phase-16 Task 6): HCM dispatch (H1 connection.go + H2 h2dispatch.go)
	// extracts tls.ConnectionState from the connection's *tls.Conn at chain
	// build time, builds the priority-ordered candidate list, and threads it
	// into chain.SetTLSPrincipals before RunDecodeHeaders dispatch. Each
	// per-stream callback returns the chain's seeded slice verbatim.
	//
	// Cross-phase reusable by future filters (jwt_authn / ext_authz / oauth2
	// / ext_proc) per ADR-0144 §Consequences.
	DownstreamPrincipal() []string

	// DownstreamRemoteAddr returns the downstream client connection's remote
	// address (net.Addr; the IP + port of the connecting peer). Returns nil
	// for synthetic streams (test harnesses that bypass the HCM dispatch
	// site without seeding the chain). HCM dispatch seeds the field at chain
	// build time from the per-connection net.Conn.RemoteAddr() BEFORE
	// RunDecodeHeaders, so per-stream callbacks observe a stable address for
	// the request lifetime.
	//
	// Per ADR-0165 §Decision (phase-18.2 callback-surface extension; the
	// ADR-0044 escape-valve fires). Cross-phase reusable by future filters
	// that need socket-level state — ext_proc / global_ratelimit / future
	// ext_authz extensions consume the same accessor surface.
	DownstreamRemoteAddr() net.Addr

	// DownstreamLocalAddr returns the downstream client connection's local
	// address (net.Addr; the listener's bound IP + port that accepted the
	// conn). Returns nil for synthetic streams. Seeding discipline mirrors
	// DownstreamRemoteAddr: set once at HCM dispatch from
	// net.Conn.LocalAddr() before RunDecodeHeaders.
	//
	// Per ADR-0165 §Decision. Cross-phase reusable (ext_proc /
	// global_ratelimit / future ext_authz extensions).
	DownstreamLocalAddr() net.Addr

	// DownstreamTLSServerName returns the SNI (Server Name Indication) the
	// downstream client presented during the TLS handshake, as observed via
	// tls.ConnectionState.ServerName. Returns the empty string for plaintext
	// connections, for TLS handshakes without SNI (rare in practice), or for
	// synthetic streams.
	//
	// Per ADR-0165 §Decision. Cross-phase reusable; populates ext_authz
	// AttributeContext.TLSSession.sni per the §11.P4 in-session SPEC scrape.
	DownstreamTLSServerName() string

	// DownstreamTLSPeerCertDER returns the raw DER bytes of the downstream
	// client's leaf X.509 certificate (PeerCertificates[0].Raw on a server-
	// side *tls.Conn). Returns nil for plaintext connections, for TLS
	// connections with no client cert, or for synthetic streams. The
	// returned slice is read-only — callers MUST NOT mutate the underlying
	// bytes.
	//
	// Per ADR-0165 §Decision. Cross-phase reusable; populates ext_authz
	// AttributeContext.Source.certificate when include_peer_certificate is
	// enabled per parent §5.P3.
	DownstreamTLSPeerCertDER() []byte

	// DownstreamProtocol returns a canonical short string identifying the
	// HTTP protocol version used for the downstream connection:
	// "HTTP/1.1" for the H1 dispatch path; "HTTP/2" for the H2 dispatch path.
	// Returns the empty string for synthetic streams that did not exercise
	// HCM dispatch. Seeded once per chain at HCM dispatch time.
	//
	// Per ADR-0165 §Decision. Cross-phase reusable; populates ext_authz
	// AttributeContext.Request.Http.protocol.
	DownstreamProtocol() string

	// ListenerPrincipal returns the listener's TLS server-cert principal as
	// extracted from the listener's *stdtls.Config.Certificates[0] leaf cert
	// (first URI SAN, then first DNS SAN, then Subject DN Common Name —
	// mirrors the DownstreamPrincipal extraction ordering on the listener-
	// cert side). Returns the empty string for plaintext listeners (no TLS
	// transport_socket on the selected filter_chain) or for synthetic
	// streams. The plumbing pre-extracts the principal at listener-build
	// time and threads the string through ListenerCtx → *Filter →
	// chain.SetListenerPrincipal — distinct from DownstreamPrincipal which
	// is the CLIENT-cert principal extracted per-connection.
	//
	// Per ADR-0165 §Decision. Cross-phase reusable; populates ext_authz
	// AttributeContext.Destination.principal per the §11.P4 in-session
	// SPEC scrape (reference Envoy populates this automatically from the
	// listener cert when TLS terminates at the proxy).
	ListenerPrincipal() string

	// DownstreamTLSConnectionState returns the FULL *tls.ConnectionState of
	// the downstream client connection, as observed by HCM dispatch at chain
	// build time. Returns nil for plaintext connections (no TLS handshake)
	// or for synthetic streams. The returned pointer is read-only — callers
	// MUST NOT mutate the underlying state.
	//
	// Distinct from the ADR-0165 derived projections (DownstreamTLSServerName
	// scalar / DownstreamTLSPeerCertDER raw bytes) — the full state is needed
	// by the phase-22.2 lua bridge's :connection():ssl() surface (12 ssl
	// methods consume PEM-encoded certs + cert chain + cipher suite + tls
	// version + verification details directly from the state per SPEC §11.5.4
	// wire-format conventions).
	//
	// Per ADR-0192 §Decision body anticipation + SPEC §11.5.3 (chain-side
	// extension lives INSIDE ADR-0192 per Q13 WEAK HOLD — no separate ADR).
	// Cross-phase reusable for any future filter needing full TLS handshake
	// state.
	DownstreamTLSConnectionState() *tls.ConnectionState

	// DynamicMetadata returns the per-stream cross-filter dynamic-metadata
	// Bucket (per ADR-0190). The Bucket is initialized at chain construction
	// (non-nil empty); Reset+nil-out at Destroy. Post-Destroy returns nil —
	// consumers are nil-tolerant per ADR-0085 (the Bucket Get/Set/Snapshot/
	// Reset methods all tolerate nil-receiver).
	//
	// Consumed by the phase-22.2 lua bridge's :dynamicMetadata() /
	// :dynamicTypedMetadata() / :setDynamicMetadata() surface. The Bucket is
	// per-stream sequential per ADR-0033 (no cross-filter concurrency within
	// a stream); cross-phase reusable for cross-filter state propagation per
	// ADR-0190 §Consequences.
	//
	// Per ADR-0192 §Decision body anticipation. Cross-phase reusable
	// framework primitive.
	DynamicMetadata() *dynamicmetadata.Bucket
}

// EncoderFilterCallbacks is the framework-supplied callback shape for
// encode-side filters.
type EncoderFilterCallbacks interface {
	// ContinueEncoding wakes the dispatch goroutine if it is parked on an
	// encode-side StopIteration return. Same coalescing discipline as
	// ContinueDecoding.
	ContinueEncoding()

	// EncodeHeaders / EncodeData / EncodeTrailers are encode-side injection
	// methods (rare).
	EncodeHeaders(headers http.Header, endStream bool)
	EncodeData(data []byte, endStream bool)
	EncodeTrailers(trailers http.Header)

	// OverwriteBody registers a replacement encode-side body. Filters MUST
	// call this only from inside their EncodeData(data, endStream)
	// implementation; the chain dispatch substitutes resp.Body before the
	// wire-write path consumes it. Not goroutine-safe — the encode chain
	// runs synchronously in the dispatch goroutine.
	//
	// Per ADR-0131 §Decision (vi); first encode-side framework primitive in
	// envoy-go (phase 14; symmetric to phase-13 ADR-0128 decode-side
	// primitives).
	OverwriteBody(b []byte)

	// BufferEncodedBody returns the chain-accumulated encode-side body bytes
	// the chain has appended on prior EncodeData(data, endStream) calls where
	// this (or another encode-side) filter returned DataStopIterationAndBuffer.
	// Returns nil before any buffering has occurred OR after the chain has
	// released the buffer downstream at end_stream (see ADR-0175 §Decision
	// release-and-clear discipline). The returned slice aliases the chain's
	// internal buffer; callers MUST NOT mutate beyond its capacity (append is
	// the recommended mutation API — re-set via subsequent OverwriteBody or
	// in-place edit through the slice header is permitted only inside an
	// EncodeData callback when the calling filter holds the dispatch
	// goroutine, never concurrently). Not goroutine-safe — the encode chain
	// runs synchronously in the dispatch goroutine per ADR-0071.
	//
	// Per ADR-0175 §Decision; envoy-go's FIRST encode-side body-buffering
	// framework primitive — the symmetric mirror of phase-13 ADR-0128
	// decode-side body-accumulation (the decode side accumulates on
	// DataStopIterationAndBuffer into chain.decodeBuf; the encode side
	// mirrors via chain.encodeBuf accessed through this reader). Distinct
	// from ADR-0131's OverwriteBody (per-call replacement; the chain
	// substitutes the supplied bytes verbatim on the SAME EncodeData call)
	// in semantics: BufferEncodedBody is buffer-and-hold — the chain
	// accumulates bytes across calls + releases at end_stream. The two
	// primitives are COMPLEMENTARY and BOTH live on EncoderFilterCallbacks
	// post-19.2 (phase 14 compressor uses OverwriteBody for per-call
	// replacement; phase 19.2 ext_proc uses BufferEncodedBody for full-body
	// inspection-then-mutate against the external processor).
	//
	// Cross-phase reuse intent (per ADR-0175 §Decision + §Consequences): any
	// future encode-side filter needing full-body inspection or mutation
	// consumes this primitive without re-paying the chain-side plumbing
	// cost. Specifically anticipated cross-phase consumers: a hypothetical
	// encode-side lua body-callback filter (response-body Lua transforms);
	// an encode-side content-injection filter (HTML-rewrite / response-
	// header-driven body splice). Neither consumer exists at 19.2 — the
	// primitive is anchored once at 19.2 IMPL to amortize the framework
	// surgery against the SINGLE current consumer (ext_proc body-mode) AND
	// the named-forward-pointer future consumers above.
	BufferEncodedBody() []byte

	// The 6 methods below are the symmetric encoder-side mirror of the
	// ADR-0165 DecoderFilterCallbacks extension. They land at phase-19.1
	// Task 5 per ADR-0174 (the ADR-0044 escape-valve firing at SPEC time per
	// BRAINSTORM §11 lesson (h); planner-time decision D10 settles 6 — NOT
	// 7 — methods, DownstreamPrincipal staying decode-side-specific per
	// ADR-0144's framing).
	//
	// The 6 underlying chain fields ALREADY exist per ADR-0165's plumbing
	// (downstreamRemoteAddr / downstreamLocalAddr / downstreamTLSServerName /
	// downstreamTLSPeerCertDER / downstreamProtocol / listenerPrincipal); HCM
	// dispatch SETs them ONCE at chain build time BEFORE either RunDecodeHeaders
	// OR RunEncodeHeaders dispatch via the existing SetX primitives. The
	// encoder-side reader methods consume the SAME chain fields the decoder-
	// side accessors consume — no new chain plumbing, no new seeding primitive,
	// no new HCM dispatch wiring. ADR-0071's chain-ownership invariant
	// continues to apply: the chain owns the fields, the callbacks are read-
	// only consumers, and the read happens after the SET completes (no race
	// surface introduced).
	//
	// PRE-REQUISITE for the ext_proc response_attributes envelope at the
	// response_headers stage (phase-19.1 attributes.go Task 9 — encoder-side
	// consumption from the ext_proc filter's EncodeHeaders callback). Cross-
	// phase reusable: any future encode-side filter that needs socket / TLS /
	// listener state (response-mutating filters, response-stage-attribute-
	// emitting filters) consumes the same accessor surface without re-paying
	// the plumbing cost.

	// DownstreamRemoteAddr returns the downstream client connection's remote
	// address (net.Addr; the IP + port of the connecting peer). Returns nil
	// for synthetic streams (test harnesses that bypass the HCM dispatch
	// site without seeding the chain). HCM dispatch seeds the field at chain
	// build time from the per-connection net.Conn.RemoteAddr() BEFORE
	// RunDecodeHeaders / RunEncodeHeaders, so per-stream callbacks observe a
	// stable address for the request lifetime.
	//
	// Per ADR-0174 §Decision (the symmetric encoder-side mirror of ADR-0165).
	// Cross-phase reusable by future encode-side filters that need socket-
	// level state.
	DownstreamRemoteAddr() net.Addr

	// DownstreamLocalAddr returns the downstream client connection's local
	// address (net.Addr; the listener's bound IP + port that accepted the
	// conn). Returns nil for synthetic streams. Seeding discipline mirrors
	// DownstreamRemoteAddr: set once at HCM dispatch from
	// net.Conn.LocalAddr() before RunDecodeHeaders / RunEncodeHeaders.
	//
	// Per ADR-0174 §Decision. Cross-phase reusable.
	DownstreamLocalAddr() net.Addr

	// DownstreamTLSServerName returns the SNI (Server Name Indication) the
	// downstream client presented during the TLS handshake, as observed via
	// tls.ConnectionState.ServerName. Returns the empty string for plaintext
	// connections, for TLS handshakes without SNI (rare in practice), or for
	// synthetic streams.
	//
	// Per ADR-0174 §Decision. Cross-phase reusable; populates ext_proc
	// response_attributes envelope's tls_session.sni field at the response_
	// headers stage.
	DownstreamTLSServerName() string

	// DownstreamTLSPeerCertDER returns the raw DER bytes of the downstream
	// client's leaf X.509 certificate (PeerCertificates[0].Raw on a server-
	// side *tls.Conn). Returns nil for plaintext connections, for TLS
	// connections with no client cert, or for synthetic streams. The
	// returned slice is read-only — callers MUST NOT mutate the underlying
	// bytes.
	//
	// Per ADR-0174 §Decision. Cross-phase reusable; populates ext_proc
	// response_attributes envelope's source.certificate field when
	// configured by the response_attributes allowlist.
	DownstreamTLSPeerCertDER() []byte

	// DownstreamProtocol returns a canonical short string identifying the
	// HTTP protocol version used for the downstream connection:
	// "HTTP/1.1" for the H1 dispatch path; "HTTP/2" for the H2 dispatch path.
	// Returns the empty string for synthetic streams that did not exercise
	// HCM dispatch. Seeded once per chain at HCM dispatch time.
	//
	// Per ADR-0174 §Decision. Cross-phase reusable; populates ext_proc
	// response_attributes envelope's request.http.protocol field.
	DownstreamProtocol() string

	// ListenerPrincipal returns the listener's TLS server-cert principal as
	// extracted from the listener's *stdtls.Config.Certificates[0] leaf cert
	// (first URI SAN, then first DNS SAN, then Subject DN Common Name —
	// mirrors the DownstreamPrincipal extraction ordering on the listener-
	// cert side). Returns the empty string for plaintext listeners (no TLS
	// transport_socket on the selected filter_chain) or for synthetic
	// streams. The plumbing pre-extracts the principal at listener-build
	// time and threads the string through ListenerCtx → *Filter →
	// chain.SetListenerPrincipal — distinct from DownstreamPrincipal which
	// is the CLIENT-cert principal (and remains decoder-side-only per
	// ADR-0144's framing + ADR-0174 planner-time D10).
	//
	// Per ADR-0174 §Decision. Cross-phase reusable; populates ext_proc
	// response_attributes envelope's destination.principal field.
	ListenerPrincipal() string

	// DownstreamTLSConnectionState (encoder side) returns the SAME
	// *tls.ConnectionState the decoder side observes — no separate chain
	// field, no separate seeding. HCM dispatch SETs once via
	// SetTLSConnectionState BEFORE either RunDecodeHeaders OR RunEncodeHeaders
	// dispatch; both callback sides observe the same value. Nil for
	// plaintext / synthetic streams.
	//
	// Per ADR-0192 §Decision body anticipation. Symmetric to the encoder-side
	// mirror of ADR-0165's DecoderFilterCallbacks accessors landed at
	// phase-19.1 per ADR-0174. Cross-phase reusable for encode-side filters
	// needing access to the full TLS handshake state (response-mutating
	// filters; response-stage attribute emission against TLS-session
	// metadata).
	DownstreamTLSConnectionState() *tls.ConnectionState

	// DynamicMetadata (encoder side) returns the SAME per-stream cross-
	// filter dynamic-metadata Bucket the decoder side observes (per-stream
	// sequential per ADR-0033 — no cross-side concurrency; bucket entries
	// persist across decode → encode iteration within a stream per ADR-0190
	// §Consequences). Post-Destroy returns nil; consumers are nil-tolerant
	// per ADR-0085.
	//
	// Per ADR-0192 §Decision body anticipation. Consumed by the phase-22.2
	// encode-side lua bridge's :dynamicMetadata() / :setDynamicMetadata()
	// surface. Cross-phase reusable for cross-filter encode-side state
	// propagation.
	DynamicMetadata() *dynamicmetadata.Bucket
}

// Compile-time assertion: a real concrete proto type satisfies proto.Message,
// confirming the import wiring (the package's RequestRouteConfig contract
// returns a value of this interface type).
var _ proto.Message = (*anypb.Any)(nil)
