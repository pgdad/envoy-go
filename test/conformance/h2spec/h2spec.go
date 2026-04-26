// Package h2spec carries phase-05.1's h2spec conformance helper. The Go
// constants below mirror the canonical pin in
// docs/envoy-go/CONFORMANCE_PINS.md; the doc file is authoritative — the
// constants are a typed mirror for use from the test driver.
package h2spec

// authoritative pin: docs/envoy-go/CONFORMANCE_PINS.md
const (
	h2specImage  = "summerwind/h2spec"
	h2specTag    = "latest" // v2.6.0 tag not published; latest resolves to same image
	h2specDigest = "sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0"
)

// imageRef returns the image@digest string consumed by testcontainers-go.
func imageRef() string { return h2specImage + "@" + h2specDigest }

// thresholdSections is the set of h2spec section identifiers (using h2spec's
// CLI positional-argument form) the conformance gate requires failed == 0 on.
// Per ADR-0051. Section 6 is represented as its individual subsections so that
// 6.6 (PUSH_PROMISE) can be excluded without running a separate filter pass.
var thresholdSections = []string{
	"http2/3",   // Starting HTTP/2 (Connection Preface)
	"http2/4",   // HTTP Frames (Frame Format, Frame Size, Header Compression)
	"http2/5",   // Streams and Multiplexing
	"http2/6/1", // Frame Definitions: DATA
	"http2/6/2", // Frame Definitions: HEADERS
	"http2/6/3", // Frame Definitions: PRIORITY
	"http2/6/4", // Frame Definitions: RST_STREAM
	"http2/6/5", // Frame Definitions: SETTINGS
	// http2/6/6 = PUSH_PROMISE — excluded per ADR-0051 (ENABLE_PUSH=0)
	"http2/6/7",  // Frame Definitions: PING
	"http2/6/8",  // Frame Definitions: GOAWAY
	"http2/6/9",  // Frame Definitions: WINDOW_UPDATE
	"http2/6/10", // Frame Definitions: CONTINUATION
	"http2/7",    // Error Codes
	"http2/8",    // HTTP Message Exchanges
}

// excludedSubsections documents the sections excluded from the threshold.
// Per ADR-0051: section 6.6 (PUSH_PROMISE) — phase 05.1 disables push
// (SETTINGS_ENABLE_PUSH=0 per ADR-0047). Kept for documentation purposes.
var excludedSubsections = []string{"http2/6/6"} //nolint:unused
