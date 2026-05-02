package listenerfilter

import (
	"errors"
	"net"
	"testing"
)

// FuzzFilterChainMatch fuzzes adversarial ChainMatchInputs corners +
// adversarial chain-spec lists into SelectChain. Per SPEC §15.6.
func FuzzFilterChainMatch(f *testing.F) {
	// Seed corpus: SPEC §11.1, §11.2, §11.3 pin shapes.
	f.Add(uint32(8080), uint32(0), "127.0.0.1", "")         // §11.3-shape inputs
	f.Add(uint32(0), uint32(54321), "10.0.0.1", "foo.test") // non-loopback inputs
	f.Add(uint32(443), uint32(0), "::1", "")                // IPv6 loopback
	f.Add(uint32(80), uint32(12345), "192.168.1.1", "*")    // wildcard SNI
	f.Fuzz(func(t *testing.T, dstPort, srcPort uint32, srcIPStr, sni string) {
		ip := net.ParseIP(srcIPStr)
		inputs := ChainMatchInputs{
			DestinationIP:   net.ParseIP("0.0.0.0"),
			DestinationPort: dstPort,
			SourceIP:        ip,
			SourcePort:      srcPort,
			ServerName:      sni,
		}
		// Build a varied chain set covering the 8 priority dimensions.
		chains := []*ChainSpec{
			{Name: "a", DestinationPort: 8080},
			{Name: "b", SourcePrefixRanges: []*net.IPNet{mustCIDR("127.0.0.0/8")}},
			{Name: "c", ServerNames: []string{"foo.test", "*.bar.test"}},
			{Name: "d", Empty: true},
		}
		def := &ChainSpec{Name: "default"}

		// Assertion (i): never panic.
		got, err := SelectChain(inputs, chains, def)
		// Assertion (ii): result is one of input chains OR default OR
		// (nil, ErrNoChainMatched / ErrAmbiguousChainMatch).
		if err != nil {
			if !errors.Is(err, ErrNoChainMatched) && !errors.Is(err, ErrAmbiguousChainMatch) {
				t.Errorf("unexpected error type: %v", err)
			}
			if got != nil {
				t.Errorf("err non-nil but chain non-nil: %v / %v", err, got)
			}
			return
		}
		valid := got == def
		for _, c := range chains {
			if got == c {
				valid = true
			}
		}
		if !valid {
			t.Errorf("returned chain not in input set or default: %v", got)
		}
		// Assertion (iii): returned chain's match dimensions all satisfied.
		if got != def && !matches(got, &inputs) {
			t.Errorf("returned chain %v does not match inputs %+v", got, inputs)
		}
		// Assertion (iv): deterministic.
		got2, err2 := SelectChain(inputs, chains, def)
		if got != got2 || (err == nil) != (err2 == nil) {
			t.Errorf("non-deterministic: first=(%v,%v) second=(%v,%v)", got, err, got2, err2)
		}
	})
}

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}
