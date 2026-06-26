package accesslog

import (
	"reflect"
	"testing"
)

// fakeLookup builds a lookup closure backed by a map[string][]string. A name
// present in the map (even with an empty/zero-length value slice) is PRESENT;
// a name absent from the map is NOT present.
func fakeLookup(src map[string][]string) func(name string) ([]string, bool) {
	return func(name string) ([]string, bool) {
		vals, ok := src[name]
		return vals, ok
	}
}

func TestCaptureHeaders(t *testing.T) {
	cases := []struct {
		name    string
		names   []string
		src     map[string][]string
		want    map[string]string
		wantNil bool // assert the returned map IS nil (byte-stable sentinel)
	}{
		{
			name:  "present single",
			names: []string{"x-req-foo"},
			src:   map[string][]string{"x-req-foo": {"bar"}},
			want:  map[string]string{"x-req-foo": "bar"},
		},
		{
			name:  "absent omitted",
			names: []string{"x-req-missing"},
			src:   map[string][]string{},
			want:  map[string]string{}, // non-nil empty map; key omitted (AMEND-HDR-2)
		},
		{
			name:  "present-empty kept",
			names: []string{"x-req-foo"},
			src:   map[string][]string{"x-req-foo": {""}},
			want:  map[string]string{"x-req-foo": ""}, // key present, empty value (AMEND-HDR-2)
		},
		{
			name:  "multi-value comma-joined wire-order",
			names: []string{"x-req-multi"},
			src:   map[string][]string{"x-req-multi": {"m1", "m2"}},
			want:  map[string]string{"x-req-multi": "m1,m2"},
		},
		{
			name:  "multi-value reversed not sorted",
			names: []string{"x-req-multi"},
			src:   map[string][]string{"x-req-multi": {"ZZZ", "AAA"}},
			want:  map[string]string{"x-req-multi": "ZZZ,AAA"}, // wire order, NOT sorted (AMEND-HDR-3)
		},
		{
			name:  "name lowercase value verbatim",
			names: []string{"x-req-foo"},
			src:   map[string][]string{"x-req-foo": {"BarVal"}},
			want:  map[string]string{"x-req-foo": "BarVal"}, // value case preserved (no double-lowercase)
		},
		{
			name:    "empty names nil map",
			names:   nil,
			src:     map[string][]string{"x-req-foo": {"bar"}},
			want:    nil,
			wantNil: true,
		},
		{
			name:    "empty-slice names nil map",
			names:   []string{},
			src:     map[string][]string{"x-req-foo": {"bar"}},
			want:    nil,
			wantNil: true,
		},
		{
			name:  "mixed set",
			names: []string{"x-a", "x-missing", "x-b"},
			src: map[string][]string{
				"x-a": {"av"},
				"x-b": {"b1", "b2"},
			},
			want: map[string]string{
				"x-a": "av",
				"x-b": "b1,b2",
			}, // the absent x-missing is omitted (AMEND-HDR-2)
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CaptureHeaders(c.names, fakeLookup(c.src))
			if c.wantNil {
				if got != nil {
					t.Fatalf("CaptureHeaders(%v) = %#v, want nil", c.names, got)
				}
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("CaptureHeaders(%v) = %#v, want %#v", c.names, got, c.want)
			}
			// Defensively assert the all-absent case returns a non-nil empty map.
			if len(c.want) == 0 && got == nil {
				t.Fatalf("CaptureHeaders(%v) = nil, want non-nil empty map", c.names)
			}
		})
	}
}
