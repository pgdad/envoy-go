package listener

import (
	"context"
	"net"
	"testing"
	"time"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestListener_AcceptLoop_IncsCxTotalAndCxActive verifies SPEC §5.5's
// listener-side hot-path discipline: on each accepted connection the listener
// increments `downstream_cx_total` (counter, monotonic) AND `downstream_cx_active`
// (gauge, +1); on connection close the active gauge decrements (-1). Polls
// within ~1s of the dial because the accept goroutine, the per-conn handler
// goroutine, and the deferred-Dec on close are all asynchronous relative to
// the dialer.
func TestListener_AcceptLoop_IncsCxTotalAndCxActive(t *testing.T) {
	// Stand up a tiny TCP "echo" backend so the tcp_proxy filter establishes a
	// real upstream conn and keeps the downstream conn alive long enough for
	// the test to observe cxActive == 1 before the close. Without a live
	// backend the upstream Dial fails, the filter immediately closes the
	// downstream, and the gauge transitions 0→1→0 too fast to poll.
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer func() { _ = backend.Close() }()
	go func() {
		for {
			c, aerr := backend.Accept()
			if aerr != nil {
				return
			}
			// Drain forever; close happens when the listener closes.
			go func(conn net.Conn) {
				defer func() { _ = conn.Close() }()
				buf := make([]byte, 256)
				for {
					if _, rerr := conn.Read(buf); rerr != nil {
						return
					}
				}
			}(c)
		}
	}()
	bAddr := backend.Addr().(*net.TCPAddr)

	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", uint32(bAddr.Port))
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_accept", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	r := stats.NewRegistry()
	lm, err := NewManager(boot, cm, r, testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := lm.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer lm.Stop()

	// Locate the per-listener metrics on the Registry so the test can read the
	// exact pointers Inc'd/Dec'd by the accept loop. Walk by name suffix so we
	// don't depend on the resolved port form of the listener-address segment.
	// Registration happens at Start time (post-bind, when the OS-picked port is
	// known) — see registerListenerMetrics in manager.go for rationale.
	var cxTotal *stats.Counter
	var cxActive *stats.Gauge
	r.Walk(func(m stats.Metric) {
		switch m.Type() {
		case stats.MetricCounter:
			if endsWith(m.Name(), ".downstream_cx_total") {
				cxTotal = m.(*stats.Counter)
			}
		case stats.MetricGauge:
			if endsWith(m.Name(), ".downstream_cx_active") {
				cxActive = m.(*stats.Gauge)
			}
		}
	})
	if cxTotal == nil || cxActive == nil {
		t.Fatalf("listener metrics not registered: cxTotal=%v cxActive=%v", cxTotal, cxActive)
	}

	infos := lm.Listeners()
	if len(infos) != 1 {
		t.Fatalf("Listeners() = %d entries, want 1", len(infos))
	}
	addr := infos[0].Addr

	// Dial one connection and assert post-accept state.
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Poll for cx_total Inc'd to 1 and cx_active Inc'd to 1 within 1s.
	if !pollUntil(time.Second, func() bool {
		return cxTotal.Load() == 1 && cxActive.Load() == 1
	}) {
		t.Errorf("post-accept: cxTotal=%d (want 1), cxActive=%d (want 1)",
			cxTotal.Load(), cxActive.Load())
	}

	// Close the conn from the dialer side; the per-conn handler goroutine
	// should observe EOF and run its deferred cxActive.Dec().
	_ = conn.Close()

	// Poll for cx_active Dec'd to 0; cx_total stays at 1 (counter is monotonic).
	if !pollUntil(2*time.Second, func() bool {
		return cxActive.Load() == 0
	}) {
		t.Errorf("post-close: cxActive=%d (want 0); cxTotal=%d (want 1, still)",
			cxActive.Load(), cxTotal.Load())
	}
	if got := cxTotal.Load(); got != 1 {
		t.Errorf("post-close: cxTotal=%d, want 1 (counter must be monotonic)", got)
	}
}

// pollUntil returns true if pred() returns true within the budget. Polls every
// 5ms — small enough that the asynchronous-accept cases settle quickly.
func pollUntil(budget time.Duration, pred func() bool) bool {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return pred()
}

// endsWith is a tiny strings.HasSuffix wrapper kept local to avoid importing
// the "strings" package solely for this test.
func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
