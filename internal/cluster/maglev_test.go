package cluster

import "testing"

func TestIsPrime(t *testing.T) {
	cases := []struct {
		n    uint64
		want bool
	}{
		{0, false}, {1, false}, {2, true}, {3, true}, {4, false},
		{100, false},     // the reference's rejected composite (AMEND-M5)
		{65537, true},    // the default table_size (Fermat prime 2^16+1)
		{5000011, true},  // the PGV cap — verified prime (a faithful cap is itself prime)
		{5000012, false}, // cap+1, composite
		{5000009, false}, // a near-cap odd composite (= 7 * 714287; oracle-pinned)
	}
	for _, c := range cases {
		if got := isPrime(c.n); got != c.want {
			t.Errorf("isPrime(%d) = %v, want %v", c.n, got, c.want)
		}
	}
}

func TestMaglev_BuildFullyFilledNoMinusOne(t *testing.T) {
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	if uint64(len(mg.table)) != 65537 {
		t.Fatalf("table len = %d, want 65537", len(mg.table))
	}
	for i, idx := range mg.table {
		if idx < 0 || idx >= len(mg.endpoints) {
			t.Fatalf("table[%d] = %d, want a valid endpoint index (no -1 / out of range)", i, idx)
		}
	}
}

func TestMaglev_MinMaxEntriesPerHost_Default3Host(t *testing.T) {
	// D-M4 / D-M2: 65537 slots over 3 equal hosts → min=floor(65537/3)=21845,
	// max=ceil(65537/3)=21846 (the live reference gauge values).
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	if mg.minPerHost != 21845 || mg.maxPerHost != 21846 {
		t.Errorf("min/max per host = %d/%d, want 21845/21846", mg.minPerHost, mg.maxPerHost)
	}
}

func TestMaglev_SortByAddrIsLoadBearing(t *testing.T) {
	// The table layout is determined by the pre-build sort on Addr() (AMEND-M2).
	// Two endpoint SLICES that are permutations of each other must build the SAME
	// table (the build sorts internally).
	a := []Endpoint{{Host: "127.0.0.1", Port: 9003}, {Host: "127.0.0.1", Port: 9001}, {Host: "127.0.0.1", Port: 9002}}
	b := []Endpoint{{Host: "127.0.0.1", Port: 9001}, {Host: "127.0.0.1", Port: 9002}, {Host: "127.0.0.1", Port: 9003}}
	ma := newMaglevWithRNG(a, maglevCfg{tableSize: 65537}, seqRNG(0))
	mb := newMaglevWithRNG(b, maglevCfg{tableSize: 65537}, seqRNG(0))
	for i := range ma.table {
		if ma.endpoints[ma.table[i]].Addr() != mb.endpoints[mb.table[i]].Addr() {
			t.Fatalf("table[%d] addr mismatch under permuted input: %q vs %q",
				i, ma.endpoints[ma.table[i]].Addr(), mb.endpoints[mb.table[i]].Addr())
		}
	}
}

func TestMaglev_SmallPrimeHandComputed(t *testing.T) {
	// M=7 prime, 1 host → every slot maps to host 0, table fully filled.
	mg := newMaglevWithRNG(eps(1), maglevCfg{tableSize: 7}, seqRNG(0))
	if len(mg.table) != 7 {
		t.Fatalf("table len = %d, want 7", len(mg.table))
	}
	for i, idx := range mg.table {
		if idx != 0 {
			t.Fatalf("single-host table[%d] = %d, want 0", i, idx)
		}
	}
	if mg.minPerHost != 7 || mg.maxPerHost != 7 {
		t.Errorf("single-host min/max = %d/%d, want 7/7", mg.minPerHost, mg.maxPerHost)
	}
}

func TestMaglev_SameKeySameEndpoint(t *testing.T) {
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	ep1, _, _ := mg.Pick(0xABCDEF, true)
	ep2, _, _ := mg.Pick(0xABCDEF, true)
	if ep1 != ep2 {
		t.Errorf("same key picked different endpoints: %v vs %v", ep1, ep2)
	}
}

func TestMaglev_PickIndexesTable(t *testing.T) {
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	for _, hk := range []uint64{0, 1, 65536, 65537, 1 << 40, ^uint64(0)} {
		ep, _, err := mg.Pick(hk, true)
		if err != nil {
			t.Fatalf("Pick(%d): %v", hk, err)
		}
		want := mg.endpoints[mg.table[hk%mg.tableSize]]
		if ep != want {
			t.Errorf("Pick(%d) = %v, want table[%d]=%v", hk, ep, hk%mg.tableSize, want)
		}
	}
}

func TestMaglev_NoHashFallbackUsesRNG(t *testing.T) {
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(7))
	ep, _, err := mg.Pick(0, false) // hashKey ignored; rng()=7 → table[7]
	if err != nil {
		t.Fatal(err)
	}
	if ep != mg.endpoints[mg.table[7]] {
		t.Errorf("no-hash fallback with rng()=7 picked %v, want table[7]=%v", ep, mg.endpoints[mg.table[7]])
	}
}

func TestMaglev_EmptySet(t *testing.T) {
	mg := newMaglevWithRNG(nil, maglevCfg{tableSize: 65537}, seqRNG(0))
	_, release, err := mg.Pick(123, true)
	if err != errNoEndpoints {
		t.Errorf("err = %v, want errNoEndpoints", err)
	}
	if release == nil {
		t.Error("release must be non-nil even on error (interface contract)")
	}
}

func TestMaglev_RandomKeysNeverPanicAlwaysValid(t *testing.T) {
	// D-S37-5: the unit-level property test in lieu of a fuzzer.
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	rng := seqRNG(1, 2, 3, 1<<63, ^uint64(0), 0)
	for i := 0; i < 1000; i++ {
		ep, rel, err := mg.Pick(rng(), true)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Port < 1000 { // eps(n) ports start at 1000
			t.Fatalf("pick %d: invalid endpoint %v", i, ep)
		}
		rel()
	}
}
