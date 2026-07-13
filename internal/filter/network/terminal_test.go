package network

import (
	"context"
	"net"
	"net/http"
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

// h3Double is a local test double proving the H3Terminal interface shape is
// satisfiable by a TerminalFilter that also serves H3. Local (rather than
// reusing *hcm.Filter) because the network package must NOT import hcm (would
// be a cycle — hcm imports network for the Marker/TerminalFilter seam).
type h3Double struct{ Marker }

func (h3Double) Handle(_ context.Context, _ net.Conn)           {}
func (h3Double) ServeH3(_ http.ResponseWriter, _ *http.Request) {}

func TestH3TerminalInterfaceShape(t *testing.T) {
	var _ H3Terminal = h3Double{}
}
