// Package tap implements Envoy's HTTP tap filter (envoy.filters.http.tap):
// a per-stream, dual-sided observer that compiles a
// config/common/matcher/v3.MatchPredicate tree into a tri-state node tree at
// config time, evaluates it over request headers (DecodeHeaders) and response
// headers (EncodeHeaders), and — at STREAM END, on a match — writes a
// buffered data/tap/v3.TraceWrapper as one byte-exact protojson document to a
// per-stream file_per_tap sink file.
//
// Three constraints are load-bearing:
//
//  1. ONE SHARED VALUE. The same *tapFilter is installed in BOTH
//     HTTPFilter.Decoder and HTTPFilter.Encoder. FilterChain.Destroy()'s loop is
//     `if f.Decoder != nil { … } else if f.Encoder != nil { … }`, so a
//     both-sided filter's OnDestroy fires exactly once — and an ENCODER-ONLY
//     value's OnDestroy is UNREACHABLE whenever a Decoder is present. Splitting
//     tap into two values would silently never emit. (Same shape as the
//     compressor filter.)
//
//  2. NEVER MUTATE THE ENCODE HEADER MAP. :status is not carried in the map
//     handed to EncodeHeaders; it comes from EncoderFilterCallbacks.ResponseStatus()
//     (ADR-0196). HCM merges that same map back into the wire response, and Go's
//     header canonicalization does not strip a leading colon — so a synthetic
//     ":status" written into it would be emitted as a literal header on the wire.
//     Tap injects it into a COPY.
//
//  3. NEVER EARLY-EMIT. The trace is an end-of-stream artifact covering the
//     WHOLE stream, even when a request arm is already true at decode. The
//     tri-state (match/no-match/undetermined) exists to RESOLVE the tree, not to
//     decide when to emit.
//
// Trailers are a documented COVERAGE BOUNDARY: envoy-go's HTTP filters cannot
// observe them (the never-done HCM "Task 18"), so the two trailer match arms are
// boot-rejected and Message.trailers is never populated. This is invisible in
// the differential: protojson's EmitDefaultValues renders empty repeated
// trailers as [] byte-identically on both sides.
package tap
