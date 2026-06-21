package router

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/stats"
)

// heldBackend is a controllable HTTP/1.1 backend whose serve() BLOCKS on a
// shared release channel until the test closes it. It mirrors scriptedBackend
// (Connection: close per response, recorded bodies) but the release gate makes
// the concurrency-deterministic: every accepted attempt parks indefinitely, so
// the test can POLL the registry until exactly the expected number of hedges
// have launched (the hedge-trigger timers fire while attempts are held), THEN
// close the gate to let them all return 200 and unblock the executor. This
// follows reference_concurrency_differential_release_barrier (poll a gauge/
// counter, never a fixed sleep).
type heldBackend struct {
	addr    string
	stop    func()
	release chan struct{}
	mu      sync.Mutex
	bodies  [][]byte
	conns   int64
	status  int
}

func newHeldBackend(t *testing.T, status int) *heldBackend {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &heldBackend{addr: ln.Addr().String(), release: make(chan struct{}), status: status}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go b.serve(conn)
		}
	}()
	b.stop = func() { _ = ln.Close(); <-done }
	return b
}

func (b *heldBackend) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	body, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	atomic.AddInt64(&b.conns, 1)
	b.mu.Lock()
	b.bodies = append(b.bodies, body)
	b.mu.Unlock()
	<-b.release // PARK until the test opens the gate
	status := b.status
	text := http.StatusText(status)
	if text == "" {
		text = "Status"
	}
	respBody := fmt.Sprintf("resp:%d", status)
	resp := fmt.Sprintf(
		"HTTP/1.1 %d %s\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		status, text, len(respBody), respBody,
	)
	_, _ = c.Write([]byte(resp))
}

// recordedBodies returns a snapshot of the request bodies the held backend has
// accepted so far (mirrors scriptedBackend.recordedBodies). len ⇒ number of
// attempts that reached the backend.
func (b *heldBackend) recordedBodies() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([][]byte, len(b.bodies))
	copy(out, b.bodies)
	return out
}

// counterValueOf reads the current value of a registry counter (mirrors
// checkCounter's lookup but returns the value instead of asserting). -1 ⇒ absent.
func counterValueOf(reg *stats.Registry, name string) int64 {
	got := int64(-1)
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			if c, ok := m.(*stats.Counter); ok {
				got = int64(c.Load())
			}
		}
	})
	return got
}

// pollCounter spins (poll 1ms, deadline 5s, NO fixed sleep) until the named
// counter reaches want, t.Fatal'ing on timeout so a bug surfaces as a failure
// rather than a hang.
func pollCounter(t *testing.T, reg *stats.Registry, name string, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if counterValueOf(reg, name) == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pollCounter: %s did not reach %d within 5s (last=%d)", name, want, counterValueOf(reg, name))
}

func TestHedgePolicy_TriggersConcurrency(t *testing.T) {
	cases := []struct {
		name string
		hp   *HedgePolicy
		want bool
	}{
		{"nil", nil, false},
		{"default", &HedgePolicy{InitialRequests: 1}, false},
		{"hedge_on_ptt", &HedgePolicy{InitialRequests: 1, HedgeOnPerTryTimeout: true}, true},
		{"fanout", &HedgePolicy{InitialRequests: 3}, true},
		{"chance", &HedgePolicy{InitialRequests: 1, AdditionalRequestChanceNum: 50, AdditionalRequestChanceDen: 100}, true},
	}
	for _, c := range cases {
		if got := c.hp.TriggersConcurrency(); got != c.want {
			t.Errorf("%s: TriggersConcurrency()=%v want %v", c.name, got, c.want)
		}
	}
}

// TestH1ClusterAction_HedgeDispatch — the Task-8 dispatch wiring. A route built
// through the WIDENED H1ClusterAction(c, hps, sm, rp, hp) with a TRIGGERING
// hedge_policy{hedge_on_per_try_timeout:true} must route to hedgeExecutorH1 — a
// held backend + a small perTryTimeout makes a hedge fire (upstream_rq_retry>0),
// which the single-attempt doH1ClusterAction path would NEVER produce. A nil hp
// (the byte-stable default) routes to the existing single-attempt path
// (upstream_rq_retry==0 for a single 200).
func TestH1ClusterAction_HedgeDispatch(t *testing.T) {
	t.Run("triggering_hp_routes_to_hedge", func(t *testing.T) {
		b := newHeldBackend(t, 200)
		defer b.stop()
		c, reg := singleEndpointClusterWithRegistry(t, b.addr)
		c.EnsureRetryStats()
		rp := mkRetryPolicyPTT(t, "5xx", 3, 8*time.Millisecond)
		hp := &HedgePolicy{InitialRequests: 1, HedgeOnPerTryTimeout: true}
		act := H1ClusterAction(c, nil, cluster.SubsetMatch{}, rp, hp)

		req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
		req.URL.Path = "/x"

		type out struct {
			status int
			err    error
		}
		done := make(chan out, 1)
		go func() {
			resp, _, err := act(req.Context(), req)
			done <- out{resp.Status, err}
		}()

		// A hedge fires while the primary is held: upstream_rq_retry reaches the
		// num_retries budget — only the hedge path produces this.
		pollCounter(t, reg, "cluster.c_test.upstream_rq_retry", 3)
		close(b.release)
		select {
		case o := <-done:
			if o.err != nil {
				t.Fatalf("hedge dispatch: %v", o.err)
			}
			if o.status != 200 {
				t.Errorf("status=%d want 200", o.status)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("hedge dispatch did not return after release")
		}
		if got := counterValueOf(reg, "cluster.c_test.upstream_rq_retry"); got == 0 {
			t.Errorf("upstream_rq_retry=0 — hedge did NOT dispatch (single-attempt path taken)")
		}
		checkCounter(t, reg, "cluster.c_test.upstream_rq_per_try_timeout", 0) // AMEND-H1
	})

	t.Run("nil_hp_routes_to_single_attempt", func(t *testing.T) {
		b := newScriptedBackend(t, func(int64) int { return 200 }, 0)
		defer b.stop()
		c, reg := singleEndpointClusterWithRegistry(t, b.addr)
		act := H1ClusterAction(c, nil, cluster.SubsetMatch{}, nil, nil)

		req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
		req.URL.Path = "/x"
		resp, _, err := act(req.Context(), req)
		if err != nil {
			t.Fatalf("single-attempt dispatch: %v", err)
		}
		if resp.Status != 200 {
			t.Errorf("status=%d want 200", resp.Status)
		}
		// Byte-stable: the single-attempt path never registers/increments
		// upstream_rq_retry — the counter is absent (counterValueOf ⇒ -1) since
		// EnsureRetryStats is never called on the nil-rp/nil-hp path. Any value > 0
		// would mean a hedge/retry executor ran.
		if got := counterValueOf(reg, "cluster.c_test.upstream_rq_retry"); got > 0 {
			t.Errorf("upstream_rq_retry=%d want absent/0 (nil hp ⇒ byte-stable single-attempt)", got)
		}
		if n := len(b.recordedBodies()); n != 1 {
			t.Errorf("driver calls=%d want 1", n)
		}
	})
}

func TestHedgeExecutorH1_PrimaryOnly(t *testing.T) {
	b := newScriptedBackend(t, func(int64) int { return 200 }, 0)
	defer b.stop()
	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicy(t, "5xx", 3), hp: &HedgePolicy{InitialRequests: 1}}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"
	resp, _, err := hedgeExecutorH1(req.Context(), a, req)
	if err != nil {
		t.Fatalf("hedgeExecutorH1: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status=%d want 200", resp.Status)
	}
	if n := len(b.recordedBodies()); n != 1 {
		t.Errorf("driver calls=%d want 1", n)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_total", 1)
}

// hedgeOnPTTAction builds a routerAction wired for the release-gated hedge
// tests: a held backend, hedge_on_per_try_timeout:true, a small perTryTimeout so
// the hedge-trigger timers fire promptly, and num_retries hedge slots.
func hedgeOnPTTAction(t *testing.T, b *heldBackend, numRetries uint32, ptt time.Duration) (*routerAction, *stats.Registry) {
	t.Helper()
	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{
		cluster: c,
		rp:      mkRetryPolicyPTT(t, "5xx", numRetries, ptt),
		hp:      &HedgePolicy{InitialRequests: 1, HedgeOnPerTryTimeout: true},
	}
	return a, reg
}

// runHedgeReleaseGated fires hedgeExecutorH1 in a goroutine against a held
// backend, polls until num_retries hedges have launched AND the limit is hit,
// then opens the gate and joins. Returns the final status. The held attempts all
// resolve to b.status (200 here) once released. The original is NOT self-canceled
// at its deadline (AMEND-H1), so it can WIN even after every hedge launches.
func runHedgeReleaseGated(t *testing.T, b *heldBackend, a *routerAction, reg *stats.Registry, numRetries int64) int {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	type out struct {
		status int
		err    error
	}
	done := make(chan out, 1)
	go func() {
		resp, _, err := hedgeExecutorH1(req.Context(), a, req)
		done <- out{resp.Status, err}
	}()

	// the original + N hedges launch while attempts are held; then a further
	// timer fires with no slot ⇒ limit_exceeded==1.
	pollCounter(t, reg, "cluster.c_test.upstream_rq_retry", numRetries)
	pollCounter(t, reg, "cluster.c_test.upstream_rq_retry_limit_exceeded", 1)

	close(b.release) // let the held attempts return b.status

	select {
	case o := <-done:
		if o.err != nil {
			t.Fatalf("hedgeExecutorH1: %v", o.err)
		}
		return o.status
	case <-time.After(5 * time.Second):
		t.Fatal("hedgeExecutorH1 did not return after release")
		return 0
	}
}

// TestHedgeExecutorH1_HedgeOnPerTryTimeout_OriginalWins — held 200 backend,
// hedge_on_per_try_timeout, num_retries:3, small perTryTimeout. Poll until
// retry==3 && limit_exceeded==1; release ⇒ final 200, upstream_rq_retry==3, and
// the load-bearing AMEND-H1: upstream_rq_per_try_timeout==0.
func TestHedgeExecutorH1_HedgeOnPerTryTimeout_OriginalWins(t *testing.T) {
	b := newHeldBackend(t, 200)
	defer b.stop()
	a, reg := hedgeOnPTTAction(t, b, 3, 8*time.Millisecond)

	status := runHedgeReleaseGated(t, b, a, reg, 3)
	if status != 200 {
		t.Errorf("final status=%d want 200", status)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 3)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_backoff_exponential", 3)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_per_try_timeout", 0) // AMEND-H1
}

// TestHedgeExecutorH1_HedgeOnPerTryTimeout_NeverIncrementsPerTryTimeout — drives
// the all-slow held path and asserts the upstream_rq_per_try_timeout delta is 0
// (load-bearing AMEND-H1: the hedge path NEVER calls IncUpstreamRqPerTryTimeout).
func TestHedgeExecutorH1_HedgeOnPerTryTimeout_NeverIncrementsPerTryTimeout(t *testing.T) {
	b := newHeldBackend(t, 200)
	defer b.stop()
	a, reg := hedgeOnPTTAction(t, b, 2, 8*time.Millisecond)

	before := counterValueOf(reg, "cluster.c_test.upstream_rq_per_try_timeout")
	_ = runHedgeReleaseGated(t, b, a, reg, 2)
	after := counterValueOf(reg, "cluster.c_test.upstream_rq_per_try_timeout")
	if after-before != 0 {
		t.Errorf("upstream_rq_per_try_timeout delta=%d want 0 (AMEND-H1)", after-before)
	}
}

// TestHedgeExecutorH1_HedgeOnPerTryTimeout_LimitExceeded — after all hedges
// launch a further timer fires with no slot; assert IncUpstreamRqRetryLimitExceeded
// ==1 AND the request still returns 200 (AMEND-H2: limit_exceeded is "no more
// hedges", not a final failure).
func TestHedgeExecutorH1_HedgeOnPerTryTimeout_LimitExceeded(t *testing.T) {
	b := newHeldBackend(t, 200)
	defer b.stop()
	a, reg := hedgeOnPTTAction(t, b, 2, 8*time.Millisecond)

	status := runHedgeReleaseGated(t, b, a, reg, 2)
	if status != 200 {
		t.Errorf("final status=%d want 200 (AMEND-H2: limit_exceeded is not a final failure)", status)
	}
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_limit_exceeded", 1)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_per_try_timeout", 0) // AMEND-H1
}

// pollUntil spins (poll 1ms, deadline 5s, NO fixed sleep) until cond() is true,
// t.Fatal'ing on timeout so a bug surfaces as a failure rather than a hang.
func pollUntil(t *testing.T, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pollUntil: %s did not become true within 5s", msg)
}

// TestHedgeExecutorH1_Fanout_BudgetCounted — initial_requests:3, num_retries:5,
// ample budget. A held backend parks all 3 up-front attempts (1 primary + 2
// budget-counted). Poll until 3 attempts are held (recordedBodies==3) OR
// upstream_rq_retry==2, then release. Expect IncUpstreamRqRetry==2, first
// acceptable wins (200), total driver calls == 3.
func TestHedgeExecutorH1_Fanout_BudgetCounted(t *testing.T) {
	b := newHeldBackend(t, 200)
	defer b.stop()
	c, reg := singleEndpointClusterWithRegistry(t, b.addr)
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicy(t, "5xx", 5), hp: &HedgePolicy{InitialRequests: 3}}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	type out struct {
		status int
		err    error
	}
	done := make(chan out, 1)
	go func() {
		resp, _, err := hedgeExecutorH1(req.Context(), a, req)
		done <- out{resp.Status, err}
	}()

	// 1 primary + 2 budget-counted attempts all park on the held backend.
	pollCounter(t, reg, "cluster.c_test.upstream_rq_retry", 2)
	pollUntil(t, "3 attempts held", func() bool { return len(b.recordedBodies()) == 3 })

	close(b.release)
	select {
	case o := <-done:
		if o.err != nil {
			t.Fatalf("hedgeExecutorH1: %v", o.err)
		}
		if o.status != 200 {
			t.Errorf("final status=%d want 200", o.status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hedgeExecutorH1 did not return after release")
	}

	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 2)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry_backoff_exponential", 2)
	checkCounter(t, reg, "cluster.c_test.upstream_rq_per_try_timeout", 0)
	if n := len(b.recordedBodies()); n != 3 {
		t.Errorf("driver calls=%d want 3", n)
	}
}

// TestHedgeExecutorH1_Fanout_AcquireGatesSpawn — initial_requests:5 but a
// retryBudgetCB() granting only 1 retry ⇒ at most 2 goroutines spawn (1 primary +
// 1 granted); the rest do NOT spawn (acquire-gates-spawn). Assert
// upstream_rq_retry==1, the overflow counter bumped, and total driver calls == 2.
func TestHedgeExecutorH1_Fanout_AcquireGatesSpawn(t *testing.T) {
	b := newHeldBackend(t, 200)
	defer b.stop()
	c, reg := singleEndpointClusterCB(t, b.addr, retryBudgetCB())
	c.EnsureRetryStats()
	a := &routerAction{cluster: c, rp: mkRetryPolicy(t, "5xx", 5), hp: &HedgePolicy{InitialRequests: 5}}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	type out struct {
		status int
		err    error
	}
	done := make(chan out, 1)
	go func() {
		resp, _, err := hedgeExecutorH1(req.Context(), a, req)
		done <- out{resp.Status, err}
	}()

	// acquire-gates-spawn: only 1 primary + 1 granted retry ⇒ 2 attempts held;
	// the budget-DENY on the 3rd fan-out attempt bumps overflow and stops the loop.
	pollCounter(t, reg, "cluster.c_test.upstream_rq_retry", 1)
	pollUntil(t, "overflow bumped", func() bool {
		return counterValueOf(reg, "cluster.c_test.upstream_rq_retry_overflow") >= 1
	})
	pollUntil(t, "2 attempts held", func() bool { return len(b.recordedBodies()) == 2 })

	close(b.release)
	select {
	case o := <-done:
		if o.err != nil {
			t.Fatalf("hedgeExecutorH1: %v", o.err)
		}
		if o.status != 200 {
			t.Errorf("final status=%d want 200", o.status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hedgeExecutorH1 did not return after release")
	}

	checkCounter(t, reg, "cluster.c_test.upstream_rq_retry", 1)
	if got := counterValueOf(reg, "cluster.c_test.upstream_rq_retry_overflow"); got < 1 {
		t.Errorf("upstream_rq_retry_overflow=%d want >=1 (acquire-gates-spawn)", got)
	}
	if n := len(b.recordedBodies()); n != 2 {
		t.Errorf("driver calls=%d want 2 (1 primary + 1 granted)", n)
	}
}

// TestAdditionalRequestChance_Deterministic — drawAdditional boundary contract:
// num=0 ⇒ never (false for any seq); num==den ⇒ always (true for any seq); a
// partial draw varies across seq but is stable per seq.
func TestAdditionalRequestChance_Deterministic(t *testing.T) {
	for _, seq := range []uint64{0, 1, 2, 7, 42, 99, 100, 12345} {
		if drawAdditional(0, 100, seq) {
			t.Errorf("drawAdditional(0,100,%d)=true want false (0 numerator ⇒ never)", seq)
		}
		if !drawAdditional(100, 100, seq) {
			t.Errorf("drawAdditional(100,100,%d)=false want true (full ⇒ always)", seq)
		}
	}
	// stable per seq: same (num,den,seq) always yields the same decision (the
	// loop below additionally proves it VARIES across seq, so this is not vacuous).
	for seq := uint64(0); seq < 200; seq++ {
		want := (seq % 100) < 50
		if drawAdditional(50, 100, seq) != want {
			t.Errorf("drawAdditional(50,100,%d)=%v want %v (deterministic per seq)", seq, !want, want)
		}
	}
	// den==0 guard ⇒ false
	if drawAdditional(50, 0, 7) {
		t.Error("drawAdditional(50,0,7)=true want false (den==0 guard)")
	}
	// partial varies across seq (not all-same)
	allSame := true
	first := drawAdditional(50, 100, 0)
	for seq := uint64(1); seq < 100; seq++ {
		if drawAdditional(50, 100, seq) != first {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("drawAdditional(50,100,*) constant across seq want varying")
	}
}

func TestNewHedgePolicy_InitialRequestsReject(t *testing.T) {
	if _, err := NewHedgePolicy(0, false, 0, 0); err == nil {
		t.Fatal("initial_requests:0 must reject")
	}
	if _, err := NewHedgePolicy(1, true, 0, 0); err != nil {
		t.Fatalf("initial_requests:1 must accept: %v", err)
	}
}
