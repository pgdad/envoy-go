package router

import (
	"testing"

	"golang.org/x/net/http2/hpack"
)

// TestActionResponse_TrailersCarrier is the RED anchor for phase 84.1 Task 4:
// ActionResponse must carry an H2 trailing-HEADERS-block field so a later
// task (Task 5) can populate it from h2.H2Response.Trailers. Trailers is
// H2-only (H1/H3 ignore it, per the router.go Close-field precedent); a
// zero value means "no trailers", matching h2.H2Response.Trailers zero-value
// semantics established at Task 2.
func TestActionResponse_TrailersCarrier(t *testing.T) {
	var zero ActionResponse
	if zero.Trailers != nil {
		t.Fatalf("zero-value ActionResponse.Trailers = %#v, want nil (no trailers)", zero.Trailers)
	}

	want := []hpack.HeaderField{
		{Name: "grpc-status", Value: "0"},
		{Name: "grpc-message", Value: "ok"},
	}
	resp := ActionResponse{Trailers: want}
	if len(resp.Trailers) != len(want) {
		t.Fatalf("ActionResponse.Trailers length = %d, want %d", len(resp.Trailers), len(want))
	}
	for i := range want {
		if resp.Trailers[i] != want[i] {
			t.Fatalf("ActionResponse.Trailers[%d] = %#v, want %#v", i, resp.Trailers[i], want[i])
		}
	}

	// Round-trips by value: ActionResponse is passed/returned by value
	// throughout the router (retryExecutorH2/hedgeExecutorH2/router_weighted.go),
	// so an independent copy's Trailers slice must not alias the source once
	// re-sliced into a fresh backing array.
	cp := resp
	cp.Trailers = append([]hpack.HeaderField(nil), resp.Trailers...)
	cp.Trailers[0].Value = "1"
	if resp.Trailers[0].Value != "0" {
		t.Fatalf("source ActionResponse.Trailers mutated via independent copy: got %q, want %q", resp.Trailers[0].Value, "0")
	}
}
