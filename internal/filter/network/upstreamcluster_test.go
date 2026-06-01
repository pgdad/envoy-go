package network

import (
	"context"
	"net"
	"testing"
)

// recordingTerminal captures the ctx passed to Handle for override inspection.
// (Distinct from recordTerminal in chain_test.go, which captures bytes not ctx.)
type recordingTerminal struct {
	Marker
	gotCtx context.Context
}

func (r *recordingTerminal) Handle(ctx context.Context, _ net.Conn) { r.gotCtx = ctx }

func TestUpstreamClusterOverrideRoundTrip(t *testing.T) {
	ctx := withUpstreamClusterOverride(context.Background(), "foo.example.com")
	got, ok := UpstreamClusterOverride(ctx)
	if !ok || got != "foo.example.com" {
		t.Fatalf("UpstreamClusterOverride = (%q,%v), want (foo.example.com,true)", got, ok)
	}
}

func TestUpstreamClusterOverrideAbsent(t *testing.T) {
	got, ok := UpstreamClusterOverride(context.Background())
	if ok || got != "" {
		t.Fatalf("UpstreamClusterOverride = (%q,%v), want (\"\",false)", got, ok)
	}
}

// handleTerminal must wrap the ctx with the override iff one is set, and pass it
// through unchanged otherwise. A recordingTerminal captures the ctx it receives.
func TestHandleTerminalThreadsOverrideWhenSet(t *testing.T) {
	rec := &recordingTerminal{}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.upstreamClusterOverride = "foo.example.com"
	rt.handleTerminal(context.Background())
	if got, ok := UpstreamClusterOverride(rec.gotCtx); !ok || got != "foo.example.com" {
		t.Fatalf("terminal ctx override = (%q,%v), want (foo.example.com,true)", got, ok)
	}
}

func TestHandleTerminalNoOverrideLeavesCtxClean(t *testing.T) {
	rec := &recordingTerminal{}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.handleTerminal(context.Background())
	if got, ok := UpstreamClusterOverride(rec.gotCtx); ok || got != "" {
		t.Fatalf("terminal ctx override = (%q,%v), want (\"\",false) when no override set", got, ok)
	}
}
