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
// CLI positional-argument dotted form, e.g. "http2/6.5" — NOT the slash form
// "http2/6/5", which silently selects zero cases; phase 85 / ADR-0307) the
// conformance gate requires failed == 0 on. Per ADR-0051. Section 6 is
// represented as its individual subsections so that 6.6 (PUSH_PROMISE) can be
// excluded without running a separate filter pass; see the 6.6 note below.
var thresholdSections = []string{
	"http2/3",   // Starting HTTP/2 (Connection Preface)
	"http2/4",   // HTTP Frames (Frame Format, Frame Size, Header Compression)
	"http2/5",   // Streams and Multiplexing
	"http2/6.1", // Frame Definitions: DATA
	"http2/6.2", // Frame Definitions: HEADERS
	"http2/6.3", // Frame Definitions: PRIORITY
	"http2/6.4", // Frame Definitions: RST_STREAM
	"http2/6.5", // Frame Definitions: SETTINGS
	// 6.6 absent from the pinned image (measured 2026-08-08) — the exclusion
	// excludes nothing; retained as documentation of ADR-0051's intent;
	// re-adding http2/6.6 would select zero cases and trip guard layer 1;
	// see ADR-0307.
	"http2/6.7",  // Frame Definitions: PING
	"http2/6.8",  // Frame Definitions: GOAWAY
	"http2/6.9",  // Frame Definitions: WINDOW_UPDATE
	"http2/6.10", // Frame Definitions: CONTINUATION
	"http2/7",    // Error Codes
	"http2/8",    // HTTP Message Exchanges
}

// expectedSuites pins every http2/* testsuite the pinned image runs under
// thresholdSections, mapped to its minimum case count (guard layers 2+3;
// phase 85 / ADR-0307). Keyed on the <testsuite> package attribute — id
// values COLLIDE across the hpack/generic families. Changes ride the
// pin-refresh procedure (CONFORMANCE_PINS.md) only.
var expectedSuites = map[string]int{
	"http2/3.5": 2, "http2/4.1": 3, "http2/4.2": 3, "http2/4.3": 3,
	"http2/5.1": 13, "http2/5.1.1": 2, "http2/5.1.2": 1, "http2/5.3.1": 2,
	"http2/5.4.1": 2, "http2/5.5": 2,
	"http2/6.1": 3, "http2/6.2": 4, "http2/6.3": 2, "http2/6.4": 3,
	"http2/6.5": 3, "http2/6.5.2": 5, "http2/6.5.3": 2, "http2/6.7": 4,
	"http2/6.8": 1, "http2/6.9": 3, "http2/6.9.1": 3, "http2/6.9.2": 3,
	"http2/6.10": 6, "http2/7": 2,
	"http2/8.1": 1, "http2/8.1.2": 1, "http2/8.1.2.1": 4, "http2/8.1.2.2": 2,
	"http2/8.1.2.3": 7, "http2/8.1.2.6": 2, "http2/8.2": 1,
}
