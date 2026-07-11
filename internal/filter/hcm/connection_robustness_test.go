package hcm

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

// HTTP/1.1 downstream-request robustness / conformance suite.
//
// The H1 read loop (runConnection → http.ReadRequest, connection.go:163)
// delegates request framing entirely to Go's net/http parser. net/http and
// upstream Envoy's HTTP/1 codec (http-parser / BalsaParser under
// RFC 9112 strict framing) disagree on several adversarial, smuggling-shaped
// inputs. This suite drives runConnection with raw bytes over an in-process
// socket pair (connPair) and pins the observed status for each shape.
//
// It has two groups:
//
//   - TestH1Robustness_ConformantRejections — inputs where envoy-go correctly
//     rejects with a 4xx, matching reference Envoy. Locks in the good behavior
//     against regression (e.g. a future codec swap that starts accepting them).
//
//   - TestH1Robustness_KnownDivergencesFromEnvoy — inputs where envoy-go
//     *accepts* (200) a request that reference Envoy v1.37.2 *rejects*
//     (400/426). These are genuine conformance gaps (request-smuggling class,
//     CWE-444) recorded in docs/TEST_GAP_ANALYSIS.md. The test asserts the
//     CURRENT (lenient) status so the suite stays green, but each row carries
//     the reference Envoy status and the test logs the divergence. If envoy-go
//     is later hardened to reject, this test fails loudly — that is the signal
//     to move the row into the ConformantRejections group and update the
//     analysis doc.
//
// Reference statuses below were captured live against
// envoyproxy/envoy:contrib-v1.37.2 (the ENVOY_TARGET.md pin) with a minimal
// HTTP1 direct-response listener on 2026-07-11.

// rawExchange writes raw request bytes to a fresh runConnection instance whose
// route table direct-responds 200 on "/", then reads and returns the response
// status code. A 2s read deadline bounds the wait; a peer close with no bytes
// yields status 0 (connection reset without a reply — itself a valid reject
// shape, though none of the cases here exercise it).
func rawExchange(t *testing.T, raw string) int {
	t.Helper()
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}
	client, server := connPair(t)
	defer func() { _ = client.Close() }()
	go runConnection(context.Background(), server, mkFilterForTable(t, tt))

	if _, err := io.WriteString(client, raw); err != nil {
		t.Fatalf("write raw request: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))

	br := bufio.NewReader(client)
	line, err := br.ReadString('\n')
	if err != nil && line == "" {
		// No status line at all — peer closed / reset without replying.
		return 0
	}
	return parseStatusLine(t, line)
}

// parseStatusLine extracts the numeric status from an HTTP/1.x status line
// ("HTTP/1.1 400 Bad Request\r\n").
func parseStatusLine(t *testing.T, line string) int {
	t.Helper()
	fields := strings.SplitN(strings.TrimSpace(line), " ", 3)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "HTTP/") {
		t.Fatalf("malformed status line: %q", line)
	}
	code, err := strconv.Atoi(fields[1])
	if err != nil {
		t.Fatalf("non-numeric status %q in line %q", fields[1], line)
	}
	return code
}

func TestH1Robustness_ConformantRejections(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		// ref is the reference Envoy v1.37.2 status (documentation only where
		// it differs from envoy-go's; both sides reject with 4xx/5xx).
		ref int
	}{
		{
			// Two Content-Length headers with different values: unambiguous
			// smuggling. Both envoy-go (via net/http) and Envoy reject.
			name: "duplicate-content-length-conflicting",
			raw:  "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\nContent-Length: 6\r\n\r\nhello",
			ref:  400,
		},
		{
			// Non-numeric Content-Length.
			name: "content-length-non-numeric",
			raw:  "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: abc\r\n\r\n",
			ref:  400,
		},
		{
			// Negative Content-Length.
			name: "content-length-negative",
			raw:  "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: -1\r\n\r\n",
			ref:  400,
		},
		{
			// Bare CR embedded in a header value.
			name: "cr-in-header-value",
			raw:  "GET / HTTP/1.1\r\nHost: x\r\nFoo: b\rar\r\n\r\n",
			ref:  400,
		},
		{
			// Whitespace inside the request-line method token.
			name: "space-in-method",
			raw:  "GE T / HTTP/1.1\r\nHost: x\r\n\r\n",
			ref:  400,
		},
		{
			// Transfer-Encoding list ending in a non-final "chunked"
			// (chunked, then identity). envoy-go → 400, Envoy → 501; both
			// reject, so this is conformant (status class, not exact code).
			name: "transfer-encoding-chunked-not-last",
			raw:  "POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nTransfer-Encoding: identity\r\n\r\n0\r\n\r\n",
			ref:  501,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rawExchange(t, tc.raw)
			if got < 400 {
				t.Errorf("status = %d, want a 4xx/5xx rejection (reference Envoy: %d)", got, tc.ref)
			}
		})
	}
}

func TestH1Robustness_KnownDivergencesFromEnvoy(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		current int // envoy-go's current (lenient) status — asserted
		envoy   int // reference Envoy v1.37.2 status — documented
		note    string
	}{
		{
			name:    "transfer-encoding-and-content-length",
			raw:     "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
			current: 200,
			envoy:   400,
			note:    "TE+CL together is the canonical request-smuggling vector (RFC 9112 §6.1, CWE-444). net/http drops CL and treats the body as chunked; Envoy rejects the message.",
		},
		{
			name:    "duplicate-content-length-identical",
			raw:     "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 0\r\nContent-Length: 0\r\n\r\n",
			current: 200,
			envoy:   400,
			note:    "net/http collapses identical duplicate Content-Length values; Envoy rejects any repeated Content-Length.",
		},
		{
			name:    "whitespace-before-header-colon",
			raw:     "GET / HTTP/1.1\r\nHost: x\r\nFoo : bar\r\n\r\n",
			current: 200,
			envoy:   400,
			note:    "Optional whitespace between field-name and colon is forbidden by RFC 9112 §5.1; net/http tolerates it, Envoy rejects.",
		},
		{
			name:    "unsupported-http-version",
			raw:     "GET / HTTP/9.9\r\nHost: x\r\n\r\n",
			current: 200,
			envoy:   426,
			note:    "net/http accepts any single-digit HTTP major.minor and serves the request; Envoy rejects an unsupported version (426 Upgrade Required).",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rawExchange(t, tc.raw)
			if got != tc.current {
				t.Errorf("status = %d, want %d (documented current envoy-go behavior).\n"+
					"If envoy-go was hardened to match reference Envoy (%d), this divergence is FIXED: "+
					"move this case into TestH1Robustness_ConformantRejections and update docs/TEST_GAP_ANALYSIS.md.",
					got, tc.current, tc.envoy)
			}
			t.Logf("KNOWN DIVERGENCE: envoy-go=%d, reference Envoy v1.37.2=%d — %s", got, tc.envoy, tc.note)
		})
	}
}
