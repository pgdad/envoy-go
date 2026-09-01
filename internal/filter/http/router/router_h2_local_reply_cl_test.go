package router

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// contentLengthArity reports every value carried under a case-insensitive
// content-length field name, in carrier order. Returned as a slice (not a
// count) so a failure message can name the offending values.
func contentLengthArity(hdrs envoyhttp.OrderedHeaders) []string {
	var got []string
	for _, h := range hdrs {
		if strings.EqualFold(h.Name, "content-length") {
			got = append(got, h.Value)
		}
	}
	return got
}

// assertExactlyOneParseableContentLength is the ONE assertion every arm of
// TestH2LocalReplyContentLengthAlwaysEmitted runs.
//
// ⚠️ ARITY EXACTLY ONE, NOT "AT LEAST ONE". A duplicate content-length on an
// H/2 carrier is itself a protocol fault (RFC 9110 8.6), so "at least one"
// would pass a set that must not ship.
//
// ⚠️ NO LITERAL VALUE IS PINNED beyond "parses as a non-negative integer".
// writeH2Reply (h2dispatch.go) rewrites a present content-length from
// len(body) before the field reaches the wire, so pinning the composer's
// literal here would pin a value the writer overwrites. What this row needs
// is that the field is PRESENT at the encode seam; its wire value is pinned
// separately, in internal/filter/hcm.
//
// Errorf, never Fatalf: an arm must report BOTH the arity fault and any value
// fault in one run, and a Fatalf here would make the sibling arms' output
// depend on this one.
func assertExactlyOneParseableContentLength(t *testing.T, label string, hdrs envoyhttp.OrderedHeaders) {
	t.Helper()
	got := contentLengthArity(hdrs)
	if len(got) != 1 {
		t.Errorf("%s: content-length field arity = %d %v, want exactly 1", label, len(got), got)
		return
	}
	n, err := strconv.Atoi(got[0])
	if err != nil {
		t.Errorf("%s: content-length = %q does not parse as an integer: %v", label, got[0], err)
		return
	}
	if n < 0 {
		t.Errorf("%s: content-length = %d, want a non-negative integer", label, n)
	}
}

// TestH2LocalReplyContentLengthAlwaysEmitted pins the phase-93 charter: on the
// HTTP/2 leg a locally generated reply carries a Content-Length, ALWAYS.
//
// Three arms, and the middle one is a NEGATIVE CONTROL on the assertion
// itself:
//
//   - helper — the composer h2LocalReplyHeaders in isolation.
//   - h1_sibling_control — the same assertion over the H/1 sibling
//     localReplyHeaders(0), which has emitted the field since phase 04.
//     ⚠️ THIS ARM MUST BE GREEN IN BOTH DIRECTIONS. It is what makes a red
//     `helper` arm a statement about H/2 rather than about a broken helper.
//   - live_502_dial_failure — drives the REAL doH2ClusterAction against a
//     closed port, so the assertion runs over a header set an actual
//     production path composed rather than over a hand-built one.
func TestH2LocalReplyContentLengthAlwaysEmitted(t *testing.T) {
	t.Run("helper", func(t *testing.T) {
		assertExactlyOneParseableContentLength(t, "h2LocalReplyHeaders()", h2LocalReplyHeaders(len(bad502Body)))
	})

	t.Run("h1_sibling_control", func(t *testing.T) {
		assertExactlyOneParseableContentLength(t, "localReplyHeaders(0)", localReplyHeaders(0))
	})

	t.Run("live_502_dial_failure", func(t *testing.T) {
		pki := mkH2BackendPKI(t)
		// Port 1 is always refused, so the pool's dial fails and
		// doH2ClusterAction takes the synthesized-502 arm (router_h2.go).
		c := h2EndpointCluster(t, "127.0.0.1:1", pki)
		a := &routerActionH2{cluster: c}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
		if err != nil {
			t.Fatalf("doH2ClusterAction: %v", err)
		}
		// Guard the precondition: if this stopped being the 502 local-reply
		// arm, the content-length assertion below would be measuring some
		// other header set and would pass or fail for the wrong reason.
		if resp.Status != 502 {
			t.Fatalf("precondition: status = %d, want 502 (dial-failure local reply)", resp.Status)
		}
		assertExactlyOneParseableContentLength(t, "doH2ClusterAction dial-failure 502", resp.Headers)
	})
}
