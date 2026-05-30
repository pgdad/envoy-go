package network

import "testing"

func TestStatusValues(t *testing.T) {
	if Continue != 0 || StopIteration != 1 {
		t.Fatalf("Status enum drift: Continue=%d StopIteration=%d", Continue, StopIteration)
	}
}

// Compile-time assertion that a minimal ReadFilter satisfies the interface.
type noopFilter struct{}

func (noopFilter) OnNewConnection() Status                      { return Continue }
func (noopFilter) OnData(_ *Buffer, _ bool) Status              { return Continue }
func (noopFilter) SetReadFilterCallbacks(_ ReadFilterCallbacks) {}
func (noopFilter) OnDestroy()                                   {}

var _ ReadFilter = noopFilter{}
