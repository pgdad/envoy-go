package driver

import "testing"

func TestClassifyBody(t *testing.T) {
	cases := []struct {
		body    string
		want    string
		wantErr bool
	}{
		{"region-a:0", "a", false},
		{"region-a:4", "a", false},
		{"backend-0:health", "b", false},
		{"backend-4:", "b", false},
		{"garbage", "", true},
		{"", "", true},
	}
	for _, c := range cases {
		got, err := classifyBody([]byte(c.body))
		if (err != nil) != c.wantErr {
			t.Errorf("classifyBody(%q): err = %v, wantErr %v", c.body, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("classifyBody(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

// TestConstants guards against reference_fixture_workload_constant_desync:
// the topology constants must stay internally consistent.
func TestConstants(t *testing.T) {
	if membershipTotal != regionAHosts+regionBHosts {
		t.Errorf("membershipTotal=%d != regionAHosts+regionBHosts=%d", membershipTotal, regionAHosts+regionBHosts)
	}
	if backendCount != regionBHosts {
		t.Errorf("backendCount=%d must equal regionBHosts=%d (region B is the runner-spawned pool)", backendCount, regionBHosts)
	}
	if degradedAHosts >= regionAHosts {
		t.Errorf("degradedAHosts=%d must be < regionAHosts=%d (some region-A hosts must stay healthy)", degradedAHosts, regionAHosts)
	}
}

func TestToggleResponder_StartsHealthyAndToggles(t *testing.T) {
	r, err := newToggleResponder(0)
	if err != nil {
		t.Fatal(err)
	}
	if !r.healthy.Load() {
		t.Error("toggleResponder must start healthy")
	}
	r.SetHealthy(false)
	if r.healthy.Load() {
		t.Error("SetHealthy(false) must clear the healthy flag")
	}
}
