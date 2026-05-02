package listenerfilter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

// stubFilter is a test-only implementation of ListenerFilter whose
// behavior is fully programmable per test case.
type stubFilter struct {
	onInspect func(ctx context.Context, peeker Peeker, inputs *ChainMatchInputs) (ListenerFilterStatus, error)
	destroyed bool
}

func (s *stubFilter) Inspect(ctx context.Context, peeker Peeker, inputs *ChainMatchInputs) (ListenerFilterStatus, error) {
	return s.onInspect(ctx, peeker, inputs)
}
func (s *stubFilter) OnDestroy() { s.destroyed = true }

func TestPipelineRunZeroFilters(t *testing.T) {
	cli, srv := net.Pipe()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	p := &Pipeline{}
	if err := p.Run(context.Background(), nil, pc, inputs, 1000); err != nil {
		t.Errorf("Run(nil filters): got %v, want nil", err)
	}
}

func TestPipelineRunContinuePath(t *testing.T) {
	cli, srv := net.Pipe()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	f1 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, in *ChainMatchInputs) (ListenerFilterStatus, error) {
		in.ServerName = "f1"
		return Continue, nil
	}}
	f2 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, in *ChainMatchInputs) (ListenerFilterStatus, error) {
		in.TransportProtocol = "f2"
		return Continue, nil
	}}
	p := &Pipeline{}
	err := p.Run(context.Background(), []ListenerFilter{f1, f2}, pc, inputs, 1000)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if inputs.ServerName != "f1" || inputs.TransportProtocol != "f2" {
		t.Errorf("inputs not populated by both filters; got %+v", inputs)
	}
	if !f1.destroyed || !f2.destroyed {
		t.Errorf("OnDestroy not called: f1=%v f2=%v", f1.destroyed, f2.destroyed)
	}
}

func TestPipelineRunStopIterationPath(t *testing.T) {
	cli, srv := net.Pipe()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	f1 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, in *ChainMatchInputs) (ListenerFilterStatus, error) {
		in.ServerName = "f1"
		return StopIteration, nil
	}}
	f2Fired := false
	f2 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, in *ChainMatchInputs) (ListenerFilterStatus, error) {
		f2Fired = true
		return Continue, nil
	}}
	p := &Pipeline{}
	if err := p.Run(context.Background(), []ListenerFilter{f1, f2}, pc, inputs, 1000); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f2Fired {
		t.Errorf("f2 fired despite f1 returning StopIteration")
	}
	if inputs.ServerName != "f1" {
		t.Errorf("ServerName=%q, want \"f1\"", inputs.ServerName)
	}
	if !f1.destroyed || !f2.destroyed {
		t.Errorf("OnDestroy not called: f1=%v f2=%v", f1.destroyed, f2.destroyed)
	}
}

func TestPipelineRunFilterError(t *testing.T) {
	cli, srv := net.Pipe()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	want := errors.New("inspect failure")
	f := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
		return Continue, want
	}}
	p := &Pipeline{}
	err := p.Run(context.Background(), []ListenerFilter{f}, pc, inputs, 1000)
	if err == nil || !errors.Is(err, want) {
		t.Errorf("Run: got %v, want errors.Is %v", err, want)
	}
}

func TestPipelineRunTimeoutSharedAcrossFilters(t *testing.T) {
	cli, srv := net.Pipe()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	// f1 sleeps for 50ms; f2 will see ctx already expired since the per-
	// pipeline budget is 30ms.
	f1 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
		}
		return Continue, nil
	}}
	f2Fired := false
	f2 := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
		f2Fired = true
		return Continue, nil
	}}
	p := &Pipeline{}
	err := p.Run(context.Background(), []ListenerFilter{f1, f2}, pc, inputs, 30) // 30ms budget
	if err == nil {
		t.Errorf("Run: got nil, want timeout error")
	}
	if f2Fired {
		t.Errorf("f2 fired despite per-pipeline timeout exhausted by f1")
	}
}

func TestPipelineRunZeroTimeoutDisablesEnforcement(t *testing.T) {
	cli, srv := net.Pipe()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	f := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
		// Sleeps for longer than any reasonable budget would allow; with
		// timeoutMs=0 the pipeline does not enforce.
		time.Sleep(10 * time.Millisecond)
		return Continue, nil
	}}
	p := &Pipeline{}
	if err := p.Run(context.Background(), []ListenerFilter{f}, pc, inputs, 0); err != nil {
		t.Errorf("Run with timeoutMs=0: %v, want nil", err)
	}
}

func TestPipelineRunPropagatesError(t *testing.T) {
	cli, srv := net.Pipe()
	defer func() { _ = cli.Close() }()
	defer func() { _ = srv.Close() }()
	pc := NewPeekerConn(srv).(*peekerConn)
	inputs := &ChainMatchInputs{}
	f := &stubFilter{onInspect: func(ctx context.Context, _ Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
		return Continue, fmt.Errorf("filter-specific")
	}}
	p := &Pipeline{}
	err := p.Run(context.Background(), []ListenerFilter{f}, pc, inputs, 1000)
	if err == nil {
		t.Errorf("Run: got nil, want filter-specific error")
	}
}
