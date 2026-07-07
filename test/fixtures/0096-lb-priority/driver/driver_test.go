package driver

import "testing"

func TestClassifyBody(t *testing.T) {
	cases := []struct {
		body    string
		want    string
		wantErr bool
	}{
		{"tier0:0", "0", false},
		{"tier0:4", "0", false},
		{"backend-0:health", "1", false},
		{"backend-4:", "1", false},
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
	if membershipTotal != tier0Hosts+tier1Hosts {
		t.Errorf("membershipTotal=%d != tier0Hosts+tier1Hosts=%d", membershipTotal, tier0Hosts+tier1Hosts)
	}
	if backendCount != tier1Hosts {
		t.Errorf("backendCount=%d must equal tier1Hosts=%d (tier 1 is the runner-spawned pool)", backendCount, tier1Hosts)
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
