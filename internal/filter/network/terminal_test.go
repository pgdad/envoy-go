package network

import (
	"context"
	"net"
	"testing"
)

// fakeTerminal satisfies TerminalFilter with zero extra methods beyond Handle.
type fakeTerminal struct {
	Marker
	handled bool
}

func (f *fakeTerminal) Handle(_ context.Context, _ net.Conn) { f.handled = true }

// bothFilter satisfies BOTH ReadFilter and TerminalFilter — the dispatch
// type-switch MUST classify it as terminal (TerminalFilter case first).
type bothFilter struct {
	Marker
}

func (bothFilter) OnNewConnection() Status                      { return Continue }
func (bothFilter) OnData(_ *Buffer, _ bool) Status              { return Continue }
func (bothFilter) SetReadFilterCallbacks(_ ReadFilterCallbacks) {}
func (bothFilter) OnDestroy()                                   {}
func (bothFilter) Handle(_ context.Context, _ net.Conn)         {}

func TestTerminalFilterSatisfied(t *testing.T) {
	var _ TerminalFilter = (*fakeTerminal)(nil)
	var _ NetworkFilter = (*fakeTerminal)(nil)
}

// A ReadFilter is also a NetworkFilter (embeds the marker via Marker).
func TestReadFilterIsNetworkFilter(t *testing.T) {
	var _ NetworkFilter = noopFilter{} // noopFilter (types_test.go) embeds Marker
}

// Dispatch type-switch is exhaustive over the two kinds.
func TestNetworkFilterClassify(t *testing.T) {
	classify := func(nf NetworkFilter) string {
		switch nf.(type) {
		case TerminalFilter:
			return "terminal"
		case ReadFilter:
			return "read"
		default:
			return "neither"
		}
	}
	if got := classify(&fakeTerminal{}); got != "terminal" {
		t.Errorf("fakeTerminal classified %q", got)
	}
	if got := classify(noopFilter{}); got != "read" {
		t.Errorf("noopFilter classified %q", got)
	}
	if got := classify(bothFilter{}); got != "terminal" {
		t.Errorf("both-satisfying classified %q, want terminal", got)
	}
}
