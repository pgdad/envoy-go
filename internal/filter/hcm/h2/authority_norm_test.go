package h2

import (
	"testing"

	"golang.org/x/net/http2/hpack"
)

func hf(name, value string) hpack.HeaderField { return hpack.HeaderField{Name: name, Value: value} }

func carrierValues(hs []hpack.HeaderField, name string) []string {
	var out []string
	for _, h := range hs {
		if h.Name == name {
			out = append(out, h.Value)
		}
	}
	return out
}

func TestAuthorityNormalization(t *testing.T) {
	tests := []struct {
		name string
		in   []hpack.HeaderField
		// P1/P3: the regular `host` must be gone from BOTH outputs, always.
		// P2:    the effective authority.
		wantAuthority string
	}{
		{
			// POSITIVE CONTROL: must PASS on the UNPATCHED tip. If this ever
			// fails, the table is vacuously red and proves nothing.
			name:          "P_authority_only",
			in:            []hpack.HeaderField{hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"), hf(":authority", "a.example")},
			wantAuthority: "a.example",
		},
		{
			// ARM A — both present. :authority WINS (it was present); the
			// regular host is suppressed from carrier AND decode map.
			name:          "A_both",
			in:            []hpack.HeaderField{hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"), hf(":authority", "a.example"), hf("host", "h.example")},
			wantAuthority: "a.example",
		},
		{
			// ARM B — host only. :authority ABSENT => PROMOTE.
			name:          "B_host_only",
			in:            []hpack.HeaderField{hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"), hf("host", "h.example")},
			wantAuthority: "h.example",
		},
		{
			// ARM C — :authority PRESENT-AND-EMPTY. D-90-SCOPE: PRESENT wins,
			// so the authority stays EMPTY.
			name:          "C_empty_authority",
			in:            []hpack.HeaderField{hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"), hf(":authority", ""), hf("host", "h.example")},
			wantAuthority: "",
		},
		{
			// ARM E — FIRST OCCURRENCE WINS as the promotion source.
			name:          "E_dup_host_first_wins",
			in:            []hpack.HeaderField{hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"), hf("host", "first.example"), hf("host", "second.example")},
			wantAuthority: "first.example",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			h2req := buildH2Request(tc.in, nil)

			// P2 — the effective authority.
			if h2req.Authority != tc.wantAuthority {
				t.Errorf("Authority = %q, want %q", h2req.Authority, tc.wantAuthority)
			}
			// P1 — regular host absent from the upstream carrier.
			if got := carrierValues(h2req.Headers, "host"); len(got) != 0 {
				t.Errorf("carrier carries host %v, want none", got)
			}

			req, err := buildRequest(tc.in, nil)
			if err != nil {
				t.Errorf("buildRequest: unexpected error %v", err)
				return
			}
			// P3 — regular host absent from the decode map. NOTE the CANONICAL
			// key: regular.Add() routes through textproto, so it lands under
			// "Host", never "host".
			if got := req.Header.Values("Host"); len(got) != 0 {
				t.Errorf("decode map carries Host %v, want none", got)
			}
			// P4 — BOTH fields, because buildRequest sets both and NC2 proved
			// corrupting both alone is invisible to the whole tree.
			if req.Host != tc.wantAuthority {
				t.Errorf("req.Host = %q, want %q", req.Host, tc.wantAuthority)
			}
			if req.URL.Host != tc.wantAuthority {
				t.Errorf("req.URL.Host = %q, want %q", req.URL.Host, tc.wantAuthority)
			}
		})
	}
}

// D_dup_authority pins the behavior D-90-DUP deliberately LEAVES ALONE.
// :method/:path/:scheme each reject a duplicate with PROTOCOL_ERROR; :authority
// does NOT, and this row does not add that reject — it belongs to the deferred
// arm-C row, which owns the reject shape decision. Without this arm, "we did not
// change duplicate handling" is a claim, not a measurement.
func TestAuthorityNormalization_DuplicateAuthorityUnchanged(t *testing.T) {
	in := []hpack.HeaderField{
		hf(":method", "GET"), hf(":path", "/"), hf(":scheme", "https"),
		hf(":authority", "first.example"), hf(":authority", "second.example"),
	}
	req, err := buildRequest(in, nil)
	if err != nil {
		t.Errorf("buildRequest: got error %v, want nil (duplicate :authority is NOT a reject at this row)", err)
		return
	}
	if req.Host != "second.example" {
		t.Errorf("req.Host = %q, want %q (last-wins, unchanged by row 90)", req.Host, "second.example")
	}
	if got := buildH2Request(in, nil).Authority; got != "second.example" {
		t.Errorf("Authority = %q, want %q (last-wins, unchanged by row 90)", got, "second.example")
	}
}
