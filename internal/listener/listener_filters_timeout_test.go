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

	"github.com/pgdad/envoy-go/internal/listener/listenerfilter"
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

// ---- phase 101 (ADR-0323): the validity rule, one arm per row ----
//
// parseListenerFiltersTimeout computes whole milliseconds from the Duration's
// FIELDS: nil gives 15000; a negative seconds OR nanos field is rejected; seconds
// above 9223372035 are rejected; anything else is seconds*1000 + nanos/1e6,
// truncated, so 0s and every sub-millisecond value give 0 (disabled). Each arm
// below is single-cause and names the property it fails on.

// lftParse parses {s, n} through the production parser for listener "l_lft".
// The uint64 conversion is a no-op on the uint64 parser; it keeps this file
// compiling against a narrower carrier, so a width regression reads as a
// per-arm value failure instead of a package build failure.
func lftParse(s int64, n int32) (uint64, error) {
	v, err := parseListenerFiltersTimeout("l_lft", &durationpb.Duration{Seconds: s, Nanos: n})
	return uint64(v), err
}

// lftAccept requires {s, n} to parse without error to exactly wantMs.
func lftAccept(t *testing.T, label string, s int64, n int32, wantMs uint64) {
	t.Helper()
	ms, err := lftParse(s, n)
	if err != nil {
		t.Errorf("%s: accept: got error %v, want accepted", label, err)
		return
	}
	if ms != wantMs {
		t.Errorf("%s: value: got %d ms, want %d ms", label, ms, wantMs)
	}
}

// lftReject requires {s, n} to be rejected; a non-empty sub must appear in the
// error (the phrase is pinned, never the whole string).
func lftReject(t *testing.T, label string, s int64, n int32, sub string) {
	t.Helper()
	ms, err := lftParse(s, n)
	if err == nil {
		t.Errorf("%s: reject: accepted with %d ms, want an error", label, ms)
		return
	}
	if sub != "" && !strings.Contains(err.Error(), sub) {
		t.Errorf("%s: reject message: %q does not contain %q", label, err.Error(), sub)
	}
}

// TestParseListenerFiltersTimeoutAboveUint32MsAccepted: 4294968s is 4294968000
// ms, 704 ms past 2^32 ms; a uint32 carrier wraps it to 704.
func TestParseListenerFiltersTimeoutAboveUint32MsAccepted(t *testing.T) {
	lftAccept(t, "4294968s", 4294968, 0, 4294968000)
}

// TestParseListenerFiltersTimeoutNegativeSecondsRejected: -1s is rejected as
// the reference rejects it (Expected positive duration).
func TestParseListenerFiltersTimeoutNegativeSecondsRejected(t *testing.T) {
	lftReject(t, "-1s", -1, 0, "expected a positive duration")
}

// TestParseListenerFiltersTimeoutNegativeNanosRejected: -0.5s arrives as
// {0, -5e8}; a seconds-only sign check would accept it.
func TestParseListenerFiltersTimeoutNegativeNanosRejected(t *testing.T) {
	lftReject(t, "-0.5s", 0, -500000000, "expected a positive duration")
}

// TestParseListenerFiltersTimeoutAboveMaxSecondsRejected: 9223372036s is one
// second past the reference's measured ceiling (Duration out-of-range).
func TestParseListenerFiltersTimeoutAboveMaxSecondsRejected(t *testing.T) {
	lftReject(t, "9223372036s", 9223372036, 0, "duration out of range")
}

// TestParseListenerFiltersTimeoutProtoMaxRejected: the protobuf Duration
// maximum, 315576000000s, is past the ceiling too.
func TestParseListenerFiltersTimeoutProtoMaxRejected(t *testing.T) {
	lftReject(t, "315576000000s", 315576000000, 0, "duration out of range")
}

// TestParseListenerFiltersTimeoutNanosOutOfRangeRejected: nanos above
// 999999999 reach the parser only from a Duration built in Go (YAML cannot
// carry them); {9223372035, 2147483647} would overflow Run's int64 deadline.
func TestParseListenerFiltersTimeoutNanosOutOfRangeRejected(t *testing.T) {
	lftReject(t, "{0s, 1000000000ns}", 0, 1000000000, "duration out of range")
	lftReject(t, "{9223372035s, 2147483647ns}", 9223372035, 2147483647, "duration out of range")
}

// TestParseListenerFiltersTimeoutMaxAccepted: the largest accepted input,
// 9223372035.999999999s, is 9223372035999 ms.
func TestParseListenerFiltersTimeoutMaxAccepted(t *testing.T) {
	lftAccept(t, "9223372035.999999999s", 9223372035, 999999999, 9223372035999)
}

// TestParseListenerFiltersTimeoutSubMillisecondTruncatesToZero: 0.0005s
// truncates to 0, which disables the timeout.
func TestParseListenerFiltersTimeoutSubMillisecondTruncatesToZero(t *testing.T) {
	lftAccept(t, "0.0005s", 0, 500000, 0)
}

// TestParseListenerFiltersTimeoutTruncatesNotRounds: 0.0019s is 1 ms, not 2.
func TestParseListenerFiltersTimeoutTruncatesNotRounds(t *testing.T) {
	lftAccept(t, "0.0019s", 0, 1900000, 1)
}

// TestParseListenerFiltersTimeoutHalfSecond: 0.5s, below the old 1s floor.
func TestParseListenerFiltersTimeoutHalfSecond(t *testing.T) {
	lftAccept(t, "0.5s", 0, 500000000, 500)
}

// TestParseListenerFiltersTimeoutNinetySeconds: 90s, above the old 60s cap.
func TestParseListenerFiltersTimeoutNinetySeconds(t *testing.T) {
	lftAccept(t, "90s", 90, 0, 90000)
}

// TestParseListenerFiltersTimeoutSixtyOneSeconds: 61s parses to 61000, not a
// value clamped to 60000.
func TestParseListenerFiltersTimeoutSixtyOneSeconds(t *testing.T) {
	lftAccept(t, "61s", 61, 0, 61000)
}

// TestParseListenerFiltersTimeoutZeroParsesToZero: an explicit 0s is 0.
func TestParseListenerFiltersTimeoutZeroParsesToZero(t *testing.T) {
	lftAccept(t, "0s", 0, 0, 0)
}

// TestParseListenerFiltersTimeoutNilParsesToDefault: an absent field is 15000.
func TestParseListenerFiltersTimeoutNilParsesToDefault(t *testing.T) {
	ms, err := parseListenerFiltersTimeout("l_lft", nil)
	if err != nil {
		t.Errorf("nil: accept: got error %v, want accepted", err)
		return
	}
	if ms != 15000 {
		t.Errorf("nil: value: got %d ms, want 15000 ms", ms)
	}
}

// lftPeekFilter blocks in a 5-byte Peek, then continues.
type lftPeekFilter struct{}

func (lftPeekFilter) Inspect(_ context.Context, p listenerfilter.Peeker, _ *listenerfilter.ChainMatchInputs) (listenerfilter.ListenerFilterStatus, error) {
	_, _ = p.Peek(5)
	return listenerfilter.Continue, nil
}

func (lftPeekFilter) OnDestroy() {}

// lftLoopbackPair returns both ends of one real loopback TCP connection.
func lftLoopbackPair(t *testing.T) (peer, server net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	type acc struct {
		c   net.Conn
		err error
	}
	ch := make(chan acc, 1)
	go func() {
		c, err := ln.Accept()
		ch <- acc{c, err}
	}()
	peer, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	a := <-ch
	if a.err != nil {
		_ = peer.Close()
		t.Fatalf("accept: %v", a.err)
	}
	return peer, a.c
}

// lftPipeHold parses {s, n}, runs Pipeline.Run with the PARSED value against a
// silent loopback peer, and requires Run to still be blocked after hold: the
// parsed value must not collapse to a short or already-expired deadline.
func lftPipeHold(t *testing.T, label string, s int64, n int32, hold time.Duration) {
	t.Helper()
	v, err := parseListenerFiltersTimeout("l_lft", &durationpb.Duration{Seconds: s, Nanos: n})
	if err != nil {
		t.Errorf("%s: pipeline: parse rejected: %v", label, err)
		return
	}
	peer, server := lftLoopbackPair(t)
	pc := listenerfilter.NewPeekerConn(server)
	type res struct {
		err error
		el  time.Duration
	}
	ch := make(chan res, 1)
	start := time.Now()
	go func() {
		var p listenerfilter.Pipeline
		err := p.Run(context.Background(), []listenerfilter.ListenerFilter{lftPeekFilter{}}, listenerfilter.AsPeeker(pc), &listenerfilter.ChainMatchInputs{}, v)
		ch <- res{err, time.Since(start)}
	}()
	returned := false
	select {
	case r := <-ch:
		returned = true
		t.Errorf("%s: pipeline hold: Run returned after %v (err %v), want still blocked at %v", label, r.el, r.err, hold)
	case <-time.After(hold):
	}
	_ = peer.Close()
	_ = pc.Close()
	if !returned {
		<-ch
	}
}

// TestListenerFilterTimeoutPipelineHoldsAtMax drives Run with the parsed
// maximum. A uint32 carrier wraps it to ~24 days, which still holds, so this
// arm guards the overflow edge, not the width.
func TestListenerFilterTimeoutPipelineHoldsAtMax(t *testing.T) {
	lftPipeHold(t, "9223372035.999999999s", 9223372035, 999999999, 1000*time.Millisecond)
}

// TestListenerFilterTimeoutPipelineHoldsPastUint32Wrap drives Run with the
// parsed 4294968s. A uint32 carrier wraps it to 704 ms, inside the hold: this
// is the behavioral arm that discriminates the width.
func TestListenerFilterTimeoutPipelineHoldsPastUint32Wrap(t *testing.T) {
	lftPipeHold(t, "4294968s", 4294968, 0, 1000*time.Millisecond)
}
