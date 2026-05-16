package extproc

// attributes.go — `ProcessingRequest` envelope builder per SPEC §6.6 +
// ADR-0174 (encoder-side accessor consumption) + ADR-0165 (decoder-side
// accessor consumption) + ADR-0144 (DownstreamPrincipal) — LANDS AT TASK 9.
//
// Production surface (consumed at Task 11's DecodeHeaders + EncodeHeaders
// bodies):
//
//   - buildRequestHeadersProcessingRequest(f, headers, endStream, allowlist)
//     → *ProcessingRequest with the request_headers oneof + an `attributes`
//     map driven by the configured `request_attributes` allowlist. Pulls
//     per-stream state from f.dcb (the ADR-0165 6-method accessor surface
//     + the ADR-0144 DownstreamPrincipal method).
//
//   - buildResponseHeadersProcessingRequest(f, headers, endStream, allowlist)
//     → symmetric *ProcessingRequest with the response_headers oneof.
//     Pulls per-stream state from f.ecb (the ADR-0174 6-method encoder-side
//     accessor surface). Per planner-time decision D10 the encoder-side
//     surface intentionally OMITS DownstreamPrincipal — the
//     `source.principal` attribute is therefore EMPTY on the encode side
//     (skipped from the envelope) at 19.1. The D10 hypothesis is HELD at
//     Task 9 per the PROGRESS.md Task 9 entry; an empirical-pin
//     falsification at Task 13 fixture-harness scrape would AMEND ADR-0174
//     §Decision in-place to add the 7th encoder-side method (NOT settled
//     at 19.1 IMPL — the PLAN's strong hypothesis is 6 suffices).
//
//   - buildAttributeEnvelope(allowlist, addressFn, ..., principalFn) →
//     map[string]*structpb.Struct driven by the SPEC §6.6 hypothesis-table.
//     Pluggable accessor closures decouple the envelope-population logic
//     from the per-side callback surface (DecoderFilterCallbacks vs
//     EncoderFilterCallbacks); the two `build*Request` helpers above are
//     thin wrappers that bind the per-side accessor closures + the
//     per-stage HttpHeaders construction.
//
//   - lowercaseHeaderMap(http.Header) → map[string]string (single-value
//     per key; multi-value joined with `,`). Mirrors the phase-18.2
//     ext_authz attributes.go precedent — the reference Envoy v1.37.2
//     internal lowercase + comma-join discipline for the per-stage
//     headers carrier. UNLIKE the phase-18.2 ext_authz `request.http.headers`
//     map (which is the AttributeContext field), the 19.1 ext_proc
//     `HttpHeaders.headers` field is a `*core.HeaderMap` — an ORDERED list
//     of (key, value) pairs (proto type `repeated HeaderValue`). The
//     lowercaseHeaderMap helper is consumed via a separate
//     `headerMapToHeaderMap(http.Header) *corev3.HeaderMap` conversion
//     below; the map form is kept for symmetry with the phase-18.2
//     helper + for future fuzzer/test consumers.
//
//   - sourcePrincipalFirstOrEmpty([]string) → string. The first-or-empty
//     helper used by the `source.principal` attribute extraction on the
//     decode side (the ADR-0144 DownstreamPrincipal returns a
//     priority-ordered slice; the SPEC §6.6 hypothesis-table populates
//     `source.principal` from `[0]` or `""`).
//
// **Attribute-name → accessor mapping** (per SPEC §6.6 hypothesis-table;
// RATIFIED-PENDING-IMPL-TIME closure at Task 13 fixture-harness scrape per
// parent §5.P4-class):
//
//   - `source.address` ← DownstreamRemoteAddr() (both sides)
//   - `destination.address` ← DownstreamLocalAddr() (both sides)
//   - `connection.requested_server_name` ← DownstreamTLSServerName() (both sides)
//   - `connection.subject_local_certificate` ← derived from listener cert
//     + ADR-0144 (the IMPL settles a reasonable hypothesis: the
//     ListenerPrincipal() string is the proxy-side TLS-identity surface
//     extracted from the listener cert — the same string flows into the
//     `connection.principal` attribute; the empirical-pin closure at
//     Task 13 may bisect these into distinct attribute values once the
//     reference Envoy v1.37.2 scrape evidence is in hand. At 19.1 IMPL
//     both `connection.subject_local_certificate` AND `connection.principal`
//     map to the same ListenerPrincipal() return value — see the
//     attributeNameToAccessor switch below.)
//   - `request.protocol` ← DownstreamProtocol() (both sides)
//   - `connection.principal` ← ListenerPrincipal() (both sides)
//   - `source.principal` ← DownstreamPrincipal()[0] OR "" on the decode
//     side; ABSENT on the encode side (D10 — no DownstreamPrincipal on
//     EncoderFilterCallbacks). When the accessor closure returns "" the
//     attribute is SKIPPED entirely from the envelope (the per-attr
//     presence is gated on a non-empty accessor return — plaintext
//     connections, synthetic streams, missing principals → silent skip
//     rather than empty-value populate).
//
// **Empty-value gate**: every attribute that resolves to an empty value
// (empty string, nil net.Addr) is SKIPPED from the envelope. The
// per-stage envelope therefore carries only attributes whose accessors
// supplied substantive per-stream state. An empty allowlist OR an
// allowlist where every entry resolves to an empty value → NIL envelope
// (the ProcessingRequest.Attributes field is left unset; the wire shape
// carries no `attributes` map at all). Forward-compat: unrecognized
// attribute-name entries in the allowlist are silently dropped (new CEL
// attribute names in future Envoy versions do not crash; the IMPL emits
// only the recognized roster).
//
// **CEL attribute value encoding**: each attribute value is wrapped in a
// `*structpb.Struct` with a single field named `"value"` whose
// `*structpb.Value` is the scalar — `StringValue` for string-typed
// attributes (per ADR-0170 §Decision + ADR-0174 §Consequences orientation;
// closure at Task 13 fixture scrape). The map-of-Struct wrapping is the
// proto type required by `ProcessingRequest.attributes` (proto field
// `map[string]*structpb.Struct`); the inner `{value: <scalar>}` shape is
// the IMPL's hypothesis for the CEL scalar-attribute encoding — closure
// at Task 13 against the reference Envoy v1.37.2 scrape evidence.
//
// **No ADR-0044 escape-valve fires at Task 9** — the attribute-name
// mapping is an IMPL-level concern per D3 (refinement against reference
// Envoy v1.37.2 closes at Task 13 fixture-harness scrape per parent
// §5.P4-class). The D10 hypothesis ("6 methods suffice on the encoder
// side") is HELD: ADR-0174 §Decision stays at 6 methods. The Task 13
// scrape may falsify D10 (if response-side `source.principal` is
// observed in the reference Envoy v1.37.2 ProcessingRequest); the
// remediation path is an in-place AMENDMENT of ADR-0174 §Decision +
// callbacks.go + chain.go + chain_test.go at Task 13. At Task 9 the
// hypothesis is documented + the test asserts the hypothesis HOLDS.

import (
	"net"
	"net/http"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocsvcv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------------------------------------------------------------------------
// buildRequestHeadersProcessingRequest — decode-side per SPEC §6.3 + §6.6.
// ---------------------------------------------------------------------------

// buildRequestHeadersProcessingRequest constructs the *ProcessingRequest sent
// at the request_headers stage. Reads per-stream state from f.dcb (the
// ADR-0165 6-method accessor surface + the ADR-0144 DownstreamPrincipal
// method). Wires the HttpHeaders {headers, end_of_stream} pair + the
// attributes envelope from the configured `request_attributes` allowlist.
//
// Per parent §5.P1: the allowlist controls which CEL attributes the
// ProcessingRequest carries. An empty allowlist → no `attributes` field
// (the wire shape carries no `attributes` map at all; the
// ProcessingRequest.Attributes proto field is left nil).
//
// Per ADR-0167: f is non-nil on the decode side; the dcb non-nilness is a
// pre-condition enforced at Task 11's DecodeHeaders entry. The function is
// nil-tolerant on f.dcb (returns a partial envelope with empty accessors)
// to keep the helper pure-function-testable without a real per-stream
// chain wiring.
func buildRequestHeadersProcessingRequest(
	f *filter,
	headers http.Header,
	endStream bool,
	allowlist []string,
) *extprocsvcv3.ProcessingRequest {
	req := &extprocsvcv3.ProcessingRequest{
		Request: &extprocsvcv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocsvcv3.HttpHeaders{
				Headers:     headerMapToHeaderMap(headers),
				EndOfStream: endStream,
			},
		},
	}

	// Bind the per-side accessor closures via f.dcb. Each closure is a
	// nil-tolerant wrapper around the corresponding DecoderFilterCallbacks
	// method — the helper itself is pure-function-testable via the
	// buildAttributeEnvelope direct call path (Group 11 tests exercise both).
	dcb := f.dcb
	envelope := buildAttributeEnvelope(allowlist,
		func() net.Addr {
			if dcb == nil {
				return nil
			}
			return dcb.DownstreamRemoteAddr()
		},
		func() net.Addr {
			if dcb == nil {
				return nil
			}
			return dcb.DownstreamLocalAddr()
		},
		func() string {
			if dcb == nil {
				return ""
			}
			return dcb.DownstreamTLSServerName()
		},
		func() string {
			if dcb == nil {
				return ""
			}
			return dcb.DownstreamProtocol()
		},
		func() string {
			if dcb == nil {
				return ""
			}
			return dcb.ListenerPrincipal()
		},
		func() string {
			if dcb == nil {
				return ""
			}
			return sourcePrincipalFirstOrEmpty(dcb.DownstreamPrincipal())
		},
	)
	if len(envelope) > 0 {
		req.Attributes = envelope
	}
	return req
}

// ---------------------------------------------------------------------------
// buildResponseHeadersProcessingRequest — encode-side per SPEC §6.4 + §6.6.
// ---------------------------------------------------------------------------

// buildResponseHeadersProcessingRequest constructs the *ProcessingRequest
// sent at the response_headers stage. Symmetric to the decode-side helper —
// reads per-stream state from f.ecb (the NEW ADR-0174 6-method encoder-side
// accessor surface that landed at Task 5).
//
// **D10 hypothesis HELD**: the encoder-side accessor surface OMITS
// DownstreamPrincipal (NO 7th method on EncoderFilterCallbacks per
// ADR-0174 §Decision planner-time settle). The `source.principal`
// attribute closure therefore returns "" — gated by the empty-value-skip
// discipline below, the attribute is SKIPPED from the envelope. The
// PLAN's strong hypothesis is 6 methods suffice; closure of the D10
// hypothesis fires at Task 13 fixture-harness scrape per parent §5.P4-class.
func buildResponseHeadersProcessingRequest(
	f *filter,
	headers http.Header,
	endStream bool,
	allowlist []string,
) *extprocsvcv3.ProcessingRequest {
	req := &extprocsvcv3.ProcessingRequest{
		Request: &extprocsvcv3.ProcessingRequest_ResponseHeaders{
			ResponseHeaders: &extprocsvcv3.HttpHeaders{
				Headers:     headerMapToHeaderMap(headers),
				EndOfStream: endStream,
			},
		},
	}

	ecb := f.ecb
	envelope := buildAttributeEnvelope(allowlist,
		func() net.Addr {
			if ecb == nil {
				return nil
			}
			return ecb.DownstreamRemoteAddr()
		},
		func() net.Addr {
			if ecb == nil {
				return nil
			}
			return ecb.DownstreamLocalAddr()
		},
		func() string {
			if ecb == nil {
				return ""
			}
			return ecb.DownstreamTLSServerName()
		},
		func() string {
			if ecb == nil {
				return ""
			}
			return ecb.DownstreamProtocol()
		},
		func() string {
			if ecb == nil {
				return ""
			}
			return ecb.ListenerPrincipal()
		},
		// D10: encoder-side has NO DownstreamPrincipal accessor. The
		// closure returns "" verbatim — the empty-value-skip discipline
		// below silently drops the `source.principal` attribute even when
		// the allowlist lists it. The disposition is documented at
		// ADR-0174 §Decision + at the PROGRESS.md Task 9 entry.
		func() string { return "" },
	)
	if len(envelope) > 0 {
		req.Attributes = envelope
	}
	return req
}

// ---------------------------------------------------------------------------
// buildAttributeEnvelope — pluggable-accessor envelope builder per SPEC §6.6.
// ---------------------------------------------------------------------------

// buildAttributeEnvelope evaluates the SPEC §6.6 hypothesis-table against
// the supplied per-attribute accessor closures + the configured allowlist.
// Returns a map suitable for direct assignment to
// ProcessingRequest.Attributes; returns nil when the allowlist is empty
// OR every allowlist entry resolves to an empty value.
//
// **Per-attribute population gate** (the empty-value-skip discipline): an
// attribute is populated IFF the accessor closure returns a non-zero value
// (non-empty string; non-nil net.Addr with non-empty String() output). The
// per-stage envelope therefore carries only attributes whose accessors
// supplied substantive per-stream state — plaintext connections,
// synthetic streams, missing principals all silently skip rather than
// populate empty-value entries.
//
// **Unrecognized attribute-name entries** in the allowlist are silently
// dropped (forward-compat — new CEL attribute names in future Envoy
// versions do not crash; the IMPL emits only the recognized roster).
//
// The accessor closures are positional per the SPEC §6.6 table ordering:
//
//	addressFn         → source.address
//	localAddressFn    → destination.address
//	tlsServerNameFn   → connection.requested_server_name
//	protocolFn        → request.protocol
//	listenerPrincFn   → connection.principal AND
//	                    connection.subject_local_certificate (the IMPL
//	                    settles both to the listener principal at 19.1;
//	                    closure at Task 13 may bisect)
//	sourcePrincFn     → source.principal (DECODE-side only — empty on
//	                    encode side per D10; gated by empty-value-skip)
//
// Carryforward F (Task 11 review-fix): the planner-time `peerCertDERFn`
// closure parameter was DROPPED at Task 11 (YAGNI — the SPEC §6.6 table at
// 19.1 has no `source.certificate` attribute; the parameter was a parked
// forward-compat placeholder. Trivially restorable when Task 13 fixture
// evidence demands per the phase-18.2 attributes.go pattern).
func buildAttributeEnvelope(
	allowlist []string,
	addressFn func() net.Addr,
	localAddressFn func() net.Addr,
	tlsServerNameFn func() string,
	protocolFn func() string,
	listenerPrincFn func() string,
	sourcePrincFn func() string,
) map[string]*structpb.Struct {
	if len(allowlist) == 0 {
		return nil
	}
	out := make(map[string]*structpb.Struct, len(allowlist))
	for _, name := range allowlist {
		switch name {
		case "source.address":
			if v := addrToString(addressFn()); v != "" {
				out[name] = scalarStringStruct(v)
			}
		case "destination.address":
			if v := addrToString(localAddressFn()); v != "" {
				out[name] = scalarStringStruct(v)
			}
		case "connection.requested_server_name":
			if v := tlsServerNameFn(); v != "" {
				out[name] = scalarStringStruct(v)
			}
		case "connection.subject_local_certificate":
			// Derived from listener cert + ADR-0144. The IMPL settles
			// the value to the ListenerPrincipal() return at 19.1 (the
			// proxy-side TLS-identity surface extracted from the
			// listener cert; same string the `connection.principal`
			// attribute carries). Closure at Task 13 fixture scrape
			// may bisect these into distinct values once the reference
			// Envoy v1.37.2 evidence is in hand.
			if v := listenerPrincFn(); v != "" {
				out[name] = scalarStringStruct(v)
			}
		case "request.protocol":
			if v := protocolFn(); v != "" {
				out[name] = scalarStringStruct(v)
			}
		case "connection.principal":
			if v := listenerPrincFn(); v != "" {
				out[name] = scalarStringStruct(v)
			}
		case "source.principal":
			if v := sourcePrincFn(); v != "" {
				out[name] = scalarStringStruct(v)
			}
		default:
			// Unrecognized attribute name — silently dropped (forward-
			// compat per parent §5.P4-class).
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// ---------------------------------------------------------------------------
// Helpers — header-map projection, scalar-struct wrap, principal first-or-empty.
// ---------------------------------------------------------------------------

// lowercaseHeaderMap converts an http.Header (canonicalized keys) into a
// map[string]string with lowercased keys, joining multi-value headers with
// `,` per the reference Envoy v1.37.2 internal lowercase + comma-join
// discipline. Mirrors the phase-18.2 ext_authz attributes.go helper
// verbatim (same signature, same semantics).
//
// HTTP/2 pseudo-headers (:authority, :method, :path, :scheme) are INCLUDED
// (NOT skipped) — the ext_proc HttpHeaders.headers carrier accepts
// :-prefixed names verbatim (per the proto's `*core.HeaderMap` shape +
// the reference Envoy v1.37.2 behavior of forwarding pseudo-headers into
// the per-stage payload).
//
// A nil input returns a non-nil empty map (the proto-faithful contract —
// the per-stage map field is rendered as `{}` rather than absent).
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
		out[lk] = strings.Join(vs, ",")
	}
	return out
}

// headerMapToHeaderMap converts an http.Header into the proto-required
// *corev3.HeaderMap (an ordered list of HeaderValue {Key, Value} pairs).
// Header names are lowercased per the reference Envoy v1.37.2 convention
// (the HttpHeaders proto doc: "All header keys will be lower-cased,
// because HTTP header keys are case-insensitive"). Multi-value headers
// expand to multiple HeaderValue entries with the same lowercased key —
// the HeaderMap is an ORDERED list, not a map, so the per-value entries
// carry the same key + their respective values (mirrors the wire shape
// of repeated HTTP/1.1 headers).
//
// A nil input returns a non-nil empty HeaderMap (the proto-faithful
// contract — the per-stage Headers field is rendered as `{}` rather than
// absent).
func headerMapToHeaderMap(h http.Header) *corev3.HeaderMap {
	hm := &corev3.HeaderMap{
		Headers: make([]*corev3.HeaderValue, 0, len(h)),
	}
	for k, vs := range h {
		lk := strings.ToLower(k)
		if len(vs) == 0 {
			hm.Headers = append(hm.Headers, &corev3.HeaderValue{Key: lk})
			continue
		}
		for _, v := range vs {
			hm.Headers = append(hm.Headers, &corev3.HeaderValue{Key: lk, Value: v})
		}
	}
	return hm
}

// sourcePrincipalFirstOrEmpty returns the first element of the
// DownstreamPrincipal() priority-ordered candidate slice, or the empty
// string when the slice is empty or nil. Mirrors the phase-18.2 ext_authz
// `firstOrEmpty` helper verbatim (same signature, same semantics; renamed
// here for grep-discoverability — the source.principal attribute is the
// sole consumer at 19.1).
func sourcePrincipalFirstOrEmpty(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return ss[0]
}

// addrToString returns the canonical string form of a net.Addr (typically
// "<ip>:<port>" for *net.TCPAddr). Returns the empty string for nil
// inputs OR for Addrs whose String() method returns empty (synthetic
// streams that bypass HCM dispatch without seeding the chain).
//
// The IMPL settles a string-typed attribute value at 19.1 (matches the
// phase-18.2 ext_authz convention of address-as-string; the SPEC §6.6
// table closes at Task 13 fixture-harness scrape — the reference Envoy
// v1.37.2 evidence may surface a structured `*core.Address`-typed
// attribute value, at which point this helper bisects + the
// attribute-name → accessor mapping switches to a per-attribute encoding
// arm in buildAttributeEnvelope).
func addrToString(a net.Addr) string {
	if a == nil {
		return ""
	}
	return a.String()
}

// scalarStringStruct wraps a string scalar into the
// `*structpb.Struct{fields: {"value": <StringValue>}}` shape that the
// 19.1 IMPL uses for CEL scalar-attribute encoding. The wrapping is the
// proto type required by ProcessingRequest.attributes
// (`map[string]*structpb.Struct`); the inner `{value: <scalar>}` shape is
// the 19.1 hypothesis — closure at Task 13 fixture scrape may revise the
// wrapping (e.g. to a direct `*structpb.Value` populated as the only
// field, or to a different field name; the reference Envoy v1.37.2
// evidence is the load-bearing closure pin).
func scalarStringStruct(s string) *structpb.Struct {
	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"value": structpb.NewStringValue(s),
		},
	}
}
