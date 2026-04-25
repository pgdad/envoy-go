package helpers

import (
	"net/http"
	"reflect"
	"sort"
	"testing"
)

func TestHTTPHeaderDiff_Identical(t *testing.T) {
	a := http.Header{"X-A": {"1"}, "X-B": {"2"}}
	b := http.Header{"X-A": {"1"}, "X-B": {"2"}}
	refOnly, subjOnly := HTTPHeaderDiff(a, b, nil)
	if len(refOnly) != 0 || len(subjOnly) != 0 {
		t.Errorf("identical: got refOnly=%v subjOnly=%v, want both empty", refOnly, subjOnly)
	}
}

func TestHTTPHeaderDiff_RefOnlyAndSubjOnly(t *testing.T) {
	a := http.Header{"X-A": {"1"}, "X-Ref": {"r"}}
	b := http.Header{"X-A": {"1"}, "X-Subj": {"s"}}
	refOnly, subjOnly := HTTPHeaderDiff(a, b, nil)
	sort.Strings(refOnly)
	sort.Strings(subjOnly)
	if !reflect.DeepEqual(refOnly, []string{"x-ref"}) {
		t.Errorf("refOnly: got %v, want [x-ref]", refOnly)
	}
	if !reflect.DeepEqual(subjOnly, []string{"x-subj"}) {
		t.Errorf("subjOnly: got %v, want [x-subj]", subjOnly)
	}
}

func TestHTTPHeaderDiff_AllowListExact(t *testing.T) {
	a := http.Header{"Date": {"Tue, 01 Apr 2026 12:00:00 GMT"}, "X-A": {"1"}}
	b := http.Header{"Date": {"Tue, 01 Apr 2026 12:00:01 GMT"}, "X-A": {"1"}}
	refOnly, subjOnly := HTTPHeaderDiff(a, b, []string{"date"})
	if len(refOnly) != 0 || len(subjOnly) != 0 {
		t.Errorf("with date allow-listed: got refOnly=%v subjOnly=%v", refOnly, subjOnly)
	}
}

func TestHTTPHeaderDiff_AllowListPrefix(t *testing.T) {
	a := http.Header{"X-Envoy-Attempt-Count": {"1"}, "X-Envoy-Expected-Rq-Timeout-Ms": {"15000"}}
	b := http.Header{}
	refOnly, subjOnly := HTTPHeaderDiff(a, b, []string{"x-envoy-*"})
	if len(refOnly) != 0 || len(subjOnly) != 0 {
		t.Errorf("with x-envoy-* allow-listed: got refOnly=%v subjOnly=%v", refOnly, subjOnly)
	}
}

func TestHTTPHeaderDiff_CaseInsensitive(t *testing.T) {
	a := http.Header{"X-FOO": {"1"}}
	b := http.Header{"x-foo": {"1"}}
	refOnly, subjOnly := HTTPHeaderDiff(a, b, nil)
	if len(refOnly) != 0 || len(subjOnly) != 0 {
		t.Errorf("case-insensitive: got refOnly=%v subjOnly=%v", refOnly, subjOnly)
	}
}

func TestHTTPHeaderDiff_AllowListCaseInsensitive(t *testing.T) {
	a := http.Header{"DATE": {"x"}}
	b := http.Header{}
	refOnly, _ := HTTPHeaderDiff(a, b, []string{"DATE"})
	if len(refOnly) != 0 {
		t.Errorf("allow-list case-insensitive: got %v", refOnly)
	}
}

func TestPhaseFourHTTPAllowList_DefaultEntries(t *testing.T) {
	want := map[string]bool{
		"date": true, "server": true, "content-length": true, "transfer-encoding": true,
		"x-envoy-*": true, "x-forwarded-*": true, "x-request-id": true,
	}
	got := map[string]bool{}
	for _, e := range PhaseFourHTTPAllowList {
		got[e] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PhaseFourHTTPAllowList:\n got: %v\n want: %v", got, want)
	}
}
