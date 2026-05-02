package listenerfilter

import (
	"net"
	"testing"
)

func TestChainMatchInputsZeroValueIsBenign(t *testing.T) {
	var c ChainMatchInputs
	if c.DestinationIP != nil {
		t.Errorf("zero ChainMatchInputs DestinationIP should be nil; got %v", c.DestinationIP)
	}
	if c.DestinationPort != 0 {
		t.Errorf("zero ChainMatchInputs DestinationPort should be 0; got %d", c.DestinationPort)
	}
	if c.IsLoopbackSource() {
		t.Errorf("zero ChainMatchInputs IsLoopbackSource should be false")
	}
}

func TestChainMatchInputsIsLoopbackSource(t *testing.T) {
	cases := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"IPv4 127.0.0.1", net.ParseIP("127.0.0.1"), true},
		{"IPv4 127.255.255.254", net.ParseIP("127.255.255.254"), true},
		{"IPv6 ::1", net.ParseIP("::1"), true},
		{"IPv4 192.168.1.1", net.ParseIP("192.168.1.1"), false},
		{"IPv4 10.0.0.1", net.ParseIP("10.0.0.1"), false},
		{"IPv6 2001:db8::1", net.ParseIP("2001:db8::1"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := ChainMatchInputs{SourceIP: tc.ip}
			if got := c.IsLoopbackSource(); got != tc.want {
				t.Errorf("IsLoopbackSource()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestStatusEnumValues(t *testing.T) {
	if Continue != 0 || StopIteration != 1 {
		t.Errorf("Status enum drift: Continue=%d StopIteration=%d", Continue, StopIteration)
	}
}
