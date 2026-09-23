package listener

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients drives the REAL
// tls_inspector (not the ctx-aware installSlowListenerFilter stub) through a
// manager-built listener: listener_filters_timeout 1s, continue_on_... false
// (default), N concurrent SILENT clients over real loopback TCP. Each client
// holds a 3 s read deadline of its own, and the test separates the three
// outcomes the tip's abort test conflates:
//
//   - closed: the SERVER closed (EOF or ECONNRESET, zero bytes) before 2 s;
//   - fellThrough: bytes arrived — the pipeline reported success and the
//     tcp_proxy chain's tagged backend answered (the deadline-only race);
//   - open: the CLIENT's own deadline expired — the server never acted.
//
// Concurrency is load-bearing: a single connection passes a racy
// two-clock repair most of the time.
func TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients(t *testing.T) {
	const n = 20
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	cm := mkClusterMgr(t, "c_a", "127.0.0.1", uint32(addrA.Port))
	l := &listenerv3.Listener{
		Name: "l_lf_real_inspector",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:           []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_a")}}},
		ListenerFilters:        []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)},
		ListenerFiltersTimeout: durationpb.New(1 * time.Second),
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	addr := mgr.Listeners()[0].Addr

	type outcome struct {
		kind string // closed | fellThrough | open | late | other
		ms   int64
	}
	out := make([]outcome, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, derr := net.DialTimeout("tcp", addr, 2*time.Second)
			if derr != nil {
				out[i] = outcome{kind: "other: " + derr.Error()}
				return
			}
			defer func() { _ = c.Close() }()
			start := time.Now()
			_ = c.SetReadDeadline(start.Add(3 * time.Second))
			buf := make([]byte, 16)
			nr, rerr := c.Read(buf)
			ms := time.Since(start).Milliseconds()
			switch {
			case nr > 0:
				out[i] = outcome{"fellThrough", ms}
			case errors.Is(rerr, os.ErrDeadlineExceeded):
				out[i] = outcome{"open", ms}
			case errors.Is(rerr, io.EOF) || errors.Is(rerr, syscall.ECONNRESET):
				if ms < 2000 {
					out[i] = outcome{"closed", ms}
				} else {
					out[i] = outcome{"late", ms}
				}
			default:
				out[i] = outcome{"other: " + rerr.Error(), ms}
			}
		}(i)
	}
	wg.Wait()
	counts := map[string]int{}
	for _, o := range out {
		counts[o.kind]++
	}
	if counts["closed"] != n {
		t.Errorf("want all %d silent clients closed by the server before 2s (listener_filters_timeout 1s, continue=false); got %v", n, counts)
	}
	for i, o := range out {
		if o.kind == "closed" && o.ms < 900 {
			t.Errorf("client %d closed at %d ms, before the 1s deadline", i, o.ms)
		}
	}
}

// TestParseListenerFiltersTimeoutZeroDisables pins the phase-100 0s fold: an
// EXPLICIT zero listener_filters_timeout disables the timeout (lfTimeoutMs 0,
// Pipeline.Run's no-deadline branch), as on the reference, while a NIL field
// keeps the 15000 ms default (TestParseListenerFiltersTimeoutDefault). Before
// the fold, zero and nil both read 15000.
func TestParseListenerFiltersTimeoutZeroDisables(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkListener("l_zero", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo"))
	l.ListenerFiltersTimeout = durationpb.New(0)
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if got := mgr.runtimes[0].lfTimeoutMs; got != 0 {
		t.Errorf("lfTimeoutMs for an explicit 0s = %d, want 0 (disabled)", got)
	}
}

// TestListenerFilterTimeoutPreCxTimeoutByValue reads
// listener.<addr>.downstream_pre_cx_timeout BY NAME through the registry (a
// test naming the struct field would not compile before the counter exists)
// on a listener with the REAL tls_inspector, 1s, continue=true. The name must
// EXIST before any traffic: a missing name is a hard failure, never read as 0.
// Then, on one manager, in order:
//
//   - immediate: a client that sends at once is served and books 0;
//   - silent: a client silent for 1.3 s, then sending, is served (and its
//     fall-through connection stays live) and books exactly 1;
//   - cancel: a manager-ctx cancel during inspection ends the pipeline with
//     context.Canceled, which is NOT a timeout and books 0.
func TestListenerFilterTimeoutPreCxTimeoutByValue(t *testing.T) {
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	cm := mkClusterMgr(t, "c_a", "127.0.0.1", uint32(addrA.Port))
	l := &listenerv3.Listener{
		Name: "l_lf_pre_cx",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:                     []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_a")}}},
		ListenerFilters:                  []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)},
		ListenerFiltersTimeout:           durationpb.New(1 * time.Second),
		ContinueOnListenerFiltersTimeout: true,
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, reg, nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	addr := mgr.Listeners()[0].Addr
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	name := "listener." + strings.ReplaceAll(host, ".", "_") + "_" + port + ".downstream_pre_cx_timeout"
	preCx := func() (uint64, bool) {
		var (
			v     uint64
			found bool
		)
		reg.Walk(func(m stats.Metric) {
			if c, ok := m.(*stats.Counter); ok && m.Name() == name {
				v, found = c.Load(), true
			}
		})
		return v, found
	}
	if v, ok := preCx(); !ok {
		t.Fatalf("name existence: counter %q is not registered before traffic (a missing name is a failure, never a zero)", name)
	} else if v != 0 {
		t.Fatalf("boot value: %s = %d before any traffic, want 0", name, v)
	}

	// dial opens one client connection and returns it with its dial time.
	dial := func() (net.Conn, time.Time) {
		c, derr := net.DialTimeout("tcp", addr, 2*time.Second)
		if derr != nil {
			t.Fatalf("dial: %v", derr)
		}
		return c, time.Now()
	}

	// immediate: sends at once -> classified raw_buffer, served, books 0.
	c1, _ := dial()
	if _, werr := c1.Write([]byte("hello")); werr != nil {
		t.Fatalf("immediate: write: %v", werr)
	}
	_ = c1.SetReadDeadline(time.Now().Add(2 * time.Second))
	b := make([]byte, 1)
	if n, rerr := c1.Read(b); n != 1 || b[0] != 'A' {
		t.Errorf("immediate: want the tagged backend's 'A', got n=%d err=%v", n, rerr)
	}
	_ = c1.Close()
	if v, _ := preCx(); v != 0 {
		t.Errorf("immediate: %s = %d after a client that sent at once, want 0", name, v)
	}

	// silent: 1.3 s of silence, then a send -> the pipeline timed out at 1 s,
	// fell through (continue=true), is served, books exactly 1.
	c2, _ := dial()
	time.Sleep(1300 * time.Millisecond)
	if _, werr := c2.Write([]byte("hello")); werr != nil {
		t.Errorf("silent: write at 1.3s: %v", werr)
	}
	_ = c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, rerr := c2.Read(b); n != 1 || b[0] != 'A' {
		t.Errorf("silent: want the fall-through connection served ('A'), got n=%d err=%v", n, rerr)
	}
	// The fall-through connection must stay LIVE: no stale listener-filter
	// read deadline may survive into the tcp_proxy dispatch.
	_ = c2.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if n, rerr := c2.Read(b); n != 0 || !errors.Is(rerr, os.ErrDeadlineExceeded) {
		t.Errorf("silent liveness: want the fall-through connection still open (client deadline), got n=%d err=%v", n, rerr)
	}
	_ = c2.Close()
	if v, _ := preCx(); v != 1 {
		t.Errorf("silent: %s = %d after one timed-out inspection, want 1", name, v)
	}

	// cancel: a manager-ctx cancel at 300 ms during inspection ends the
	// pipeline with context.Canceled (the pipeline's clock fires on the parent
	// cancel too) BEFORE the 1 s deadline; that is not a timeout: books 0.
	c3, start3 := dial()
	time.Sleep(300 * time.Millisecond)
	cancel()
	_ = c3.SetReadDeadline(start3.Add(3 * time.Second))
	n3, rerr3 := c3.Read(b)
	ms3 := time.Since(start3).Milliseconds()
	t.Logf("cancel: the server acted at %d ms (n=%d err=%v)", ms3, n3, rerr3)
	switch {
	case errors.Is(rerr3, os.ErrDeadlineExceeded):
		t.Errorf("cancel: the server never acted on the manager-ctx cancel (client deadline after %d ms)", ms3)
	case ms3 >= 900:
		t.Errorf("cancel: the server acted at %d ms — at/after the 1s deadline, not at the 300 ms cancel (n=%d err=%v)", ms3, n3, rerr3)
	}
	_ = c3.Close()
	if v, _ := preCx(); v != 1 {
		t.Errorf("cancel: %s = %d after a manager-ctx cancel, want 1 (unchanged: a cancel is not a timeout)", name, v)
	}
}

// TestListenerFilterTimeoutPreCxTimeoutOnAbort is the continue=false mirror of
// TestListenerFilterTimeoutPreCxTimeoutByValue: a timed-out inspection that
// CLOSES the connection books downstream_pre_cx_timeout too (the reference
// books it under both continue values). Read BY NAME; a missing name fails.
func TestListenerFilterTimeoutPreCxTimeoutOnAbort(t *testing.T) {
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	cm := mkClusterMgr(t, "c_a", "127.0.0.1", uint32(addrA.Port))
	l := &listenerv3.Listener{
		Name: "l_lf_pre_cx_abort",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:           []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_a")}}},
		ListenerFilters:        []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)},
		ListenerFiltersTimeout: durationpb.New(1 * time.Second),
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, reg, nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	addr := mgr.Listeners()[0].Addr
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	name := "listener." + strings.ReplaceAll(host, ".", "_") + "_" + port + ".downstream_pre_cx_timeout"
	preCx := func() (uint64, bool) {
		var (
			v     uint64
			found bool
		)
		reg.Walk(func(m stats.Metric) {
			if c, ok := m.(*stats.Counter); ok && m.Name() == name {
				v, found = c.Load(), true
			}
		})
		return v, found
	}
	if _, ok := preCx(); !ok {
		t.Fatalf("name existence: counter %q is not registered before traffic (a missing name is a failure, never a zero)", name)
	}
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	start := time.Now()
	_ = c.SetReadDeadline(start.Add(3 * time.Second))
	b := make([]byte, 1)
	n, rerr := c.Read(b)
	if n != 0 || !(errors.Is(rerr, io.EOF) || errors.Is(rerr, syscall.ECONNRESET)) || time.Since(start) >= 2*time.Second {
		t.Errorf("abort: want the SERVER to close a silent client before 2s; got n=%d err=%v after %v", n, rerr, time.Since(start))
	}
	if v, _ := preCx(); v != 1 {
		t.Errorf("abort: %s = %d after one timed-out, closed inspection (continue=false), want 1", name, v)
	}
}
