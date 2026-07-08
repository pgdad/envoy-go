package driver

import "testing"

func TestClassifyBody(t *testing.T) {
	cases := []struct {
		body        string
		wantCluster string
		wantHost    int
		wantErr     bool
	}{
		{"c_pt_a:0", "c_pt_a", 0, false},
		{"c_pt_c:4", "c_pt_c", 4, false},
		{"garbage", "", 0, true},
		{"", "", 0, true},
	}
	for _, c := range cases {
		gotC, gotH, err := classifyBody([]byte(c.body))
		if (err != nil) != c.wantErr {
			t.Errorf("classifyBody(%q): err=%v wantErr=%v", c.body, err, c.wantErr)
			continue
		}
		if gotC != c.wantCluster || gotH != c.wantHost {
			t.Errorf("classifyBody(%q) = (%q,%d), want (%q,%d)", c.body, gotC, gotH, c.wantCluster, c.wantHost)
		}
	}
}

func TestConstants(t *testing.T) {
	if healthyPerCluster != hostsPerCluster-degradedPerCluster {
		t.Errorf("healthyPerCluster=%d != hostsPerCluster-degradedPerCluster", healthyPerCluster)
	}
	if degradedPerCluster*2 >= hostsPerCluster {
		t.Errorf("degradedPerCluster=%d must keep >50%% healthy for the B/C no-panic arms", degradedPerCluster)
	}
}
