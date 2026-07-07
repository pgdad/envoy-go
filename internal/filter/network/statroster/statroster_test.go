package statroster

import (
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

func TestNew_EagerCreationAndIdempotence(t *testing.T) {
	reg := stats.NewRegistry()
	suffixes := []string{"alpha", "beta_bytes"}
	a := New(reg, "p.proto.", suffixes)
	if len(a) != 2 {
		t.Fatalf("created %d counters, want 2", len(a))
	}
	if a["alpha"].Name() != "p.proto.alpha" {
		t.Errorf("name = %q, want p.proto.alpha (prefix+suffix verbatim)", a["alpha"].Name())
	}
	if a["alpha"].Load() != 0 {
		t.Errorf("counter starts at %d, want 0", a["alpha"].Load())
	}
	// A second roster sharing the prefix shares the SAME counter instances
	// (NewCounterIfAbsent — the shared-stat_prefix listener case).
	b := New(reg, "p.proto.", suffixes)
	if a["alpha"] != b["alpha"] {
		t.Fatal("shared prefix must share the same counter instances")
	}
}

func TestIncAdd(t *testing.T) {
	reg := stats.NewRegistry()
	c := New(reg, "p.", []string{"x"})
	Inc("pkgtag", c, "x")
	Add("pkgtag", c, "x", 41)
	if got := c["x"].Load(); got != 42 {
		t.Fatalf("counter = %d, want 42", got)
	}
}

func TestUnknownSuffixPanicsWithPackageTag(t *testing.T) {
	reg := stats.NewRegistry()
	c := New(reg, "p.", []string{"x"})
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Inc(unknown) must panic (the roster is closed)")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "pkgtag: unknown roster suffix") {
			t.Fatalf("panic message = %v, want the pkg-tagged wording", r)
		}
	}()
	Inc("pkgtag", c, "not_a_counter")
}
