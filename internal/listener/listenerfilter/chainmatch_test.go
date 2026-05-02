package listenerfilter

import (
	"errors"
	"net"
	"testing"
)

func cidr(s string) *net.IPNet { _, n, _ := net.ParseCIDR(s); return n }

func TestSelectChainEmptyMatchUniversallyEligible(t *testing.T) {
	cs := &ChainSpec{Name: "catchall", Empty: true}
	inputs := ChainMatchInputs{DestinationPort: 8080, SourceIP: net.ParseIP("10.0.0.1")}
	got, err := SelectChain(inputs, []*ChainSpec{cs}, nil)
	if err != nil {
		t.Fatalf("SelectChain: %v", err)
	}
	if got != cs {
		t.Errorf("got %v, want %v", got, cs)
	}
}

func TestSelectChainDestinationPortBeatsSourcePrefix(t *testing.T) {
	dstport := &ChainSpec{Name: "dstport", DestinationPort: 8080}
	srcprefix := &ChainSpec{Name: "srcprefix", SourcePrefixRanges: []*net.IPNet{cidr("127.0.0.0/8")}}
	inputs := ChainMatchInputs{DestinationPort: 8080, SourceIP: net.ParseIP("127.0.0.1")}
	got, err := SelectChain(inputs, []*ChainSpec{dstport, srcprefix}, nil)
	if err != nil {
		t.Fatalf("SelectChain: %v", err)
	}
	if got != dstport {
		t.Errorf("expected dstport (priority slot 0); got %v", got)
	}
}

func TestSelectChainDefaultChainOnNoMatch(t *testing.T) {
	specific := &ChainSpec{Name: "loopback", SourcePrefixRanges: []*net.IPNet{cidr("127.0.0.0/8")}}
	def := &ChainSpec{Name: "default"}
	inputs := ChainMatchInputs{SourceIP: net.ParseIP("10.0.0.1")} // not in 127.0.0.0/8
	got, err := SelectChain(inputs, []*ChainSpec{specific}, def)
	if err != nil {
		t.Fatalf("SelectChain: %v", err)
	}
	if got != def {
		t.Errorf("expected default chain; got %v", got)
	}
}

func TestSelectChainEmptyMatchBeatsDefault(t *testing.T) {
	emptyMatch := &ChainSpec{Name: "empty", Empty: true}
	def := &ChainSpec{Name: "default"}
	inputs := ChainMatchInputs{SourceIP: net.ParseIP("10.0.0.1")}
	got, err := SelectChain(inputs, []*ChainSpec{emptyMatch}, def)
	if err != nil {
		t.Fatalf("SelectChain: %v", err)
	}
	if got != emptyMatch {
		t.Errorf("empty-match chain should beat default; got %v", got)
	}
}

func TestSelectChainNoEligibleNoDefault(t *testing.T) {
	specific := &ChainSpec{Name: "loopback", SourcePrefixRanges: []*net.IPNet{cidr("127.0.0.0/8")}}
	inputs := ChainMatchInputs{SourceIP: net.ParseIP("10.0.0.1")}
	got, err := SelectChain(inputs, []*ChainSpec{specific}, nil)
	if !errors.Is(err, ErrNoChainMatched) {
		t.Errorf("got (%v, %v), want (nil, ErrNoChainMatched)", got, err)
	}
	if got != nil {
		t.Errorf("got chain=%v on no-match, want nil", got)
	}
}

func TestSelectChainPrefixRangesLongerWins(t *testing.T) {
	a := &ChainSpec{Name: "a", PrefixRanges: []*net.IPNet{cidr("192.168.0.0/16")}}
	b := &ChainSpec{Name: "b", PrefixRanges: []*net.IPNet{cidr("192.168.1.0/24")}}
	inputs := ChainMatchInputs{DestinationIP: net.ParseIP("192.168.1.50")}
	got, err := SelectChain(inputs, []*ChainSpec{a, b}, nil)
	if err != nil {
		t.Fatalf("SelectChain: %v", err)
	}
	if got != b {
		t.Errorf("expected longer-prefix chain b (/24); got %v", got)
	}
}

func TestSelectChainServerNamesSpecificity(t *testing.T) {
	exact := &ChainSpec{Name: "exact", ServerNames: []string{"foo.example.test"}}
	suffix := &ChainSpec{Name: "suffix", ServerNames: []string{"*.example.test"}}
	universal := &ChainSpec{Name: "universal", ServerNames: []string{"*"}}
	inputs := ChainMatchInputs{ServerName: "foo.example.test"}
	// All three eligible; exact wins.
	got, err := SelectChain(inputs, []*ChainSpec{universal, suffix, exact}, nil)
	if err != nil {
		t.Fatalf("SelectChain: %v", err)
	}
	if got != exact {
		t.Errorf("expected exact-match SNI chain; got %v", got)
	}
}

func TestSelectChainSourceTypeLocal(t *testing.T) {
	local := &ChainSpec{Name: "local", SourceTypeLocal: true}
	universal := &ChainSpec{Name: "u", Empty: true}
	inputs := ChainMatchInputs{SourceIP: net.ParseIP("127.0.0.1")}
	got, err := SelectChain(inputs, []*ChainSpec{local, universal}, nil)
	if err != nil {
		t.Fatalf("SelectChain: %v", err)
	}
	if got != local {
		t.Errorf("expected source_type:LOCAL chain; got %v", got)
	}
}

func TestSelectChainSourceTypeExternalSkipsLoopback(t *testing.T) {
	ext := &ChainSpec{Name: "ext", SourceTypeExternal: true}
	universal := &ChainSpec{Name: "u", Empty: true}
	inputs := ChainMatchInputs{SourceIP: net.ParseIP("127.0.0.1")} // loopback
	got, err := SelectChain(inputs, []*ChainSpec{ext, universal}, nil)
	if err != nil {
		t.Fatalf("SelectChain: %v", err)
	}
	// ext is eliminated (loopback != external); universal wins.
	if got != universal {
		t.Errorf("expected universal chain (ext eliminated); got %v", got)
	}
}

func TestSelectChainApplicationProtocolsTieBreaker(t *testing.T) {
	h2 := &ChainSpec{Name: "h2", ApplicationProtocols: []string{"h2"}}
	h1 := &ChainSpec{Name: "h1", ApplicationProtocols: []string{"http/1.1"}}
	inputs := ChainMatchInputs{ApplicationProtocols: []string{"h2"}}
	got, err := SelectChain(inputs, []*ChainSpec{h1, h2}, nil)
	if err != nil {
		t.Fatalf("SelectChain: %v", err)
	}
	if got != h2 {
		t.Errorf("expected h2 chain (ALPN match); got %v", got)
	}
}
