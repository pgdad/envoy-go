// Package driver registers the 0125-listener-filters-timeout fixture with the
// differential runner. See ../README.md for the fixture's purpose.
//
// THE PROPOSITION (phase 100, SPEC §7): reference Envoy ENFORCES
// Listener.listener_filters_timeout. A connection whose listener filters have
// not finished inspecting when the deadline fires is CLOSED under
// continue_on_listener_filters_timeout: false, or FALLS THROUGH to chain
// selection under true; either way the listener's
// downstream_pre_cx_timeout counter books it. A timeout of 0s DISABLES the
// deadline. envoy-go (un-fixed) never enforces it: a silent client is held
// until IT acts, and the counter name does not exist.
//
// Five plaintext TCP listeners, each a ONE-LINE diff from one base (tls_inspector,
// listener_filters_timeout 1s, fc_indexed matching transport_protocol
// raw_buffer, a last-resort default_filter_chain):
//
//	listener    delta                                         pre_cx (both sides)
//	l_false     continue: false                               51 (F1 1 + F2 50 + F3 0)
//	l_true      continue: true                                1  (T2; T1 books 0)
//	l_true_tls  continue: true, fc_indexed matches "tls"      1  (T3)
//	l_zero      listener_filters_timeout: 0s, continue: false 0  (Z1: 0s disables)
//	l_nofilt    no listener_filters, continue: false          0  (N1: nothing to time)
//
// ⚠️ WHAT THIS FIXTURE DELIBERATELY DOES NOT PIN (SPEC §7.4): downstream_cx_total
// on any listener with a drop arm (the reference counts post-filter; the
// subject pre-filter), the close KIND (the reference RSTs or FINs; docker-proxy
// rewrites it on the host path anyway — it is RECORDED, never asserted), a
// partial-byte arm, a half-close arm, any exact millisecond (windows only), and
// any downstream_listener_filter_* name (the subject emits none).
//
// Every arm on a side runs CONCURRENTLY, so a side's drive costs max(arm) =
// Z1's 16.5 s once, not the sum of the arms.
package driver

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0125-listener-filters-timeout"

// refAdminPort is the in-container reference admin port, fixed at 9901 by the
// harness.
const refAdminPort = 9901

// drivenPath is the path every GET arm requests. Both chains of every listener
// route "/" to a direct_response; the BODY names the chain.
const drivenPath = "/lft"

// wantStatus is the direct_response status on every chain. 200 — NOT a 1xx,
// which a Go client consumes as informational and never surfaces as final.
const wantStatus = 200

// The close window, in milliseconds from the client's dial returning.
//
// ⚠️ SET FROM MEASUREMENT, NOT FROM THE CONFIG. The configured deadline is
// 1000 ms on both sides; the window is the measured spread plus a margin on
// both sides of it (README §Window records the per-side figures and the
// σ-margin arithmetic). Widen it with a new measurement, never drop an arm.
const (
	windowLoMs = 700
	windowHiMs = 1800
)

// Arm timing.
const (
	silentHold = 3 * time.Second         // F1/F2: hold a silent client this long
	f2Conns    = 50                      // F2: concurrent silent clients
	f3Delay    = 300 * time.Millisecond  // F3: GET well inside the deadline
	tDelay     = 2500 * time.Millisecond // T2/T3: GET well after the deadline
	z1Open     = 16500 * time.Millisecond
	n1Open     = 2800 * time.Millisecond
	getTimeout = 5 * time.Second // a GET arm's read budget after its write
)

// Body tokens. The served body is "<token> <listener>\n", so one observed body
// names both the chain and the listener that produced it.
const (
	chainIndexed = "INDEXED"
	chainDefault = "DEFAULT"
)

// lspec is one listener. Order is the index-wise zip the runner performs
// between SubjectListenerNames() and ReferenceListenerPorts().
type lspec struct {
	name    string
	refPort int // in-container reference port — CENSUSED (SPEC §7.2)
	// filters: whether the listener carries the tls_inspector listener filter.
	filters bool
	// timeout is the listener_filters_timeout literal.
	timeout string
	// cont is continue_on_listener_filters_timeout.
	cont bool
	// indexedMatch is fc_indexed's filter_chain_match.transport_protocol.
	indexedMatch string
	// wantPreCx is downstream_pre_cx_timeout after the drive, BY VALUE.
	wantPreCx uint64
}

var listeners = []lspec{
	{name: "l_false", refPort: 15125, filters: true, timeout: "1s", cont: false, indexedMatch: "raw_buffer", wantPreCx: 1 + f2Conns},
	{name: "l_true", refPort: 15228, filters: true, timeout: "1s", cont: true, indexedMatch: "raw_buffer", wantPreCx: 1},
	{name: "l_true_tls", refPort: 15229, filters: true, timeout: "1s", cont: true, indexedMatch: "tls", wantPreCx: 1},
	{name: "l_zero", refPort: 15230, filters: true, timeout: "0s", cont: false, indexedMatch: "raw_buffer", wantPreCx: 0},
	{name: "l_nofilt", refPort: 15231, filters: false, timeout: "1s", cont: false, indexedMatch: "raw_buffer", wantPreCx: 0},
}

func init() { fixture.RegisterFixture(fixtureName, &lftDriver{}) }

// lftDriver is STATEFUL: each side's Drive records its observations so
// AssertStats can assert every arm absolutely, per side, one Errorf per
// property. A Drive-time error would become t.Fatalf and MASK every later arm.
type lftDriver struct {
	mu    sync.Mutex
	obs   map[string]*sideObs // side -> observations
	addrs map[string]map[string]string
}

// closeObs is one silent client's outcome.
type closeObs struct {
	closed bool   // the server ended the connection before the hold elapsed
	ms     int64  // ms from dial-return to the close (or to the hold's end)
	bytes  int    // bytes the server sent before closing
	kind   string // FIN / RST / other:<err> / open / dial:<err> — RECORDED, not pinned
}

// getObs is one GET arm's outcome.
type getObs struct {
	status int
	body   string
	err    string
}

type sideObs struct {
	f1     closeObs
	f2     []closeObs
	z1, n1 closeObs
	get    map[string]getObs // arm -> outcome (F3, T1, T2, T3)
}

// getArm is one timed GET: which listener, how long to stay silent first, and
// the chain whose body must answer.
type getArm struct {
	id        string
	listener  string
	delay     time.Duration
	wantChain string
}

var getArms = []getArm{
	{id: "F3", listener: "l_false", delay: f3Delay, wantChain: chainIndexed},
	{id: "T1", listener: "l_true", delay: 0, wantChain: chainIndexed},
	{id: "T2", listener: "l_true", delay: tDelay, wantChain: chainIndexed},
	{id: "T3", listener: "l_true_tls", delay: tDelay, wantChain: chainDefault},
}

var (
	_ fixture.Driver              = (*lftDriver)(nil)
	_ fixture.MultiListenerDriver = (*lftDriver)(nil)
	_ fixture.StatsAsserter       = (*lftDriver)(nil)
)

// --- fixture.Driver ---

// BackendCount is 1: a never-dialed placeholder (the runner rejects 0 and
// envoy-go boot-rejects an absent static_resources.clusters key).
func (*lftDriver) BackendCount() int { return 1 }

func (*lftDriver) SubjectListenerName() string { return listeners[0].name }

func (*lftDriver) ReferenceListenerPort() int { return listeners[0].refPort }

func (*lftDriver) ReferenceBootstrap(backendPorts []int) string {
	return renderBootstrap("0.0.0.0", refAdminPort, func(i int) int { return listeners[i].refPort }, backendPorts[0])
}

// SubjectConfig binds the five subject listeners at subjListenerPort+0..+4,
// inside the runner's probed 16-port block.
func (*lftDriver) SubjectConfig(_ int, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return renderBootstrap("127.0.0.1", subjAdminPort, func(i int) int { return subjListenerPort + i }, backendPorts[0])
}

// DriveReference / DriveSubject are UNREACHABLE while MultiListenerDriver is
// implemented; they refuse rather than derive sibling addresses.
func (*lftDriver) DriveReference(context.Context, string) ([]byte, error) {
	return nil, errors.New("0125 drives only through DriveReferenceMulti")
}

func (*lftDriver) DriveSubject(context.Context, string) ([]byte, error) {
	return nil, errors.New("0125 drives only through DriveSubjectMulti")
}

func (*lftDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.MultiListenerDriver ---

func (*lftDriver) SubjectListenerNames() []string {
	out := make([]string, len(listeners))
	for i, l := range listeners {
		out[i] = l.name
	}
	return out
}

func (*lftDriver) ReferenceListenerPorts() []int {
	out := make([]int, len(listeners))
	for i, l := range listeners {
		out[i] = l.refPort
	}
	return out
}

func (d *lftDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.drive(ctx, "ref", addrs)
}

func (d *lftDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.drive(ctx, "subj", addrs)
}

// drive runs EVERY arm on one side concurrently and emits a side-independent,
// timing-free verdict stream for the runner's CompareBytes. Only a missing
// address returns an error; every per-arm failure is recorded and asserted in
// AssertStats.
func (d *lftDriver) drive(ctx context.Context, side string, addrs map[string]string) ([]byte, error) {
	for _, l := range listeners {
		if addrs[l.name] == "" {
			return nil, fmt.Errorf("%s: no address supplied for listener %q (have %d entries)", side, l.name, len(addrs))
		}
	}
	o := &sideObs{f2: make([]closeObs, f2Conns), get: map[string]getObs{}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	start := time.Now()

	wg.Add(1)
	go func() { defer wg.Done(); o.z1 = silentProbe(ctx, addrs["l_zero"], z1Open) }()
	wg.Add(1)
	go func() { defer wg.Done(); o.n1 = silentProbe(ctx, addrs["l_nofilt"], n1Open) }()
	wg.Add(1)
	go func() { defer wg.Done(); o.f1 = silentProbe(ctx, addrs["l_false"], silentHold) }()
	for i := 0; i < f2Conns; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); o.f2[i] = silentProbe(ctx, addrs["l_false"], silentHold) }(i)
	}
	for _, a := range getArms {
		wg.Add(1)
		go func(a getArm) {
			defer wg.Done()
			g := getProbe(ctx, addrs[a.listener], a.delay)
			mu.Lock()
			o.get[a.id] = g
			mu.Unlock()
		}(a)
	}
	wg.Wait()
	log.Printf("%s: %s drive wall time %d ms", fixtureName, side, time.Since(start).Milliseconds())

	d.mu.Lock()
	if d.obs == nil {
		d.obs = map[string]*sideObs{}
		d.addrs = map[string]map[string]string{}
	}
	d.obs[side] = o
	d.addrs[side] = addrs
	d.mu.Unlock()

	logTimings(side, o)

	var b bytes.Buffer
	fmt.Fprintf(&b, "F1 l_false %s\n", closeVerdict(o.f1))
	inWin, totalBytes := 0, 0
	for _, c := range o.f2 {
		if c.closed && inWindow(c.ms) {
			inWin++
		}
		totalBytes += c.bytes
	}
	fmt.Fprintf(&b, "F2 l_false closed_in_window=%d/%d bytes=%d\n", inWin, f2Conns, totalBytes)
	for _, a := range getArms {
		g := o.get[a.id]
		fmt.Fprintf(&b, "%s %s status=%d body=%q err=%q\n", a.id, a.listener, g.status, g.body, g.err)
	}
	fmt.Fprintf(&b, "Z1 l_zero open_at_%dms=%t\n", z1Open.Milliseconds(), !o.z1.closed && o.z1.kind == "open")
	fmt.Fprintf(&b, "N1 l_nofilt open_at_%dms=%t\n", n1Open.Milliseconds(), !o.n1.closed && o.n1.kind == "open")
	return b.Bytes(), nil
}

func inWindow(ms int64) bool { return ms >= windowLoMs && ms <= windowHiMs }

// closeVerdict is the timing-free, side-independent verdict for one silent arm.
func closeVerdict(c closeObs) string {
	return fmt.Sprintf("closed=%t in_window=%t bytes=%d", c.closed, c.closed && inWindow(c.ms), c.bytes)
}

// silentProbe dials addr, sends NOTHING, and reads until the server closes or
// hold elapses. The clock starts when the dial returns. The close kind is
// RECORDED (FIN = io.EOF, RST = ECONNRESET) and never asserted.
func silentProbe(ctx context.Context, addr string, hold time.Duration) closeObs {
	var dl net.Dialer
	c, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return closeObs{kind: "dial:" + err.Error()}
	}
	t0 := time.Now()
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(t0.Add(hold))
	buf := make([]byte, 4096)
	n := 0
	for {
		k, rerr := c.Read(buf)
		n += k
		if rerr == nil {
			continue
		}
		ms := time.Since(t0).Milliseconds()
		var ne net.Error
		switch {
		case errors.As(rerr, &ne) && ne.Timeout():
			return closeObs{closed: false, ms: ms, bytes: n, kind: "open"}
		case errors.Is(rerr, io.EOF):
			return closeObs{closed: true, ms: ms, bytes: n, kind: "FIN"}
		case errors.Is(rerr, syscall.ECONNRESET):
			return closeObs{closed: true, ms: ms, bytes: n, kind: "RST"}
		default:
			return closeObs{closed: true, ms: ms, bytes: n, kind: "other:" + rerr.Error()}
		}
	}
}

// getProbe dials addr, stays silent for delay, then sends one HTTP/1.1 GET and
// reads the response. A connection the server closed during the silence
// surfaces as a recorded error, not a returned one.
func getProbe(ctx context.Context, addr string, delay time.Duration) getObs {
	var dl net.Dialer
	c, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return getObs{err: "dial: " + err.Error()}
	}
	defer func() { _ = c.Close() }()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return getObs{err: "ctx: " + ctx.Err().Error()}
		}
	}
	_ = c.SetDeadline(time.Now().Add(getTimeout))
	req := "GET " + drivenPath + " HTTP/1.1\r\nHost: lft\r\nConnection: close\r\n\r\n"
	if _, err := io.WriteString(c, req); err != nil {
		return getObs{err: "write: " + err.Error()}
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		return getObs{err: "read: " + err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return getObs{status: resp.StatusCode, body: string(body), err: "body: " + err.Error()}
	}
	return getObs{status: resp.StatusCode, body: string(body)}
}

// logTimings RECORDS every silent connection's raw outcome and the F1+F2 close
// spread, pass or fail, so a green run still leaves the measurement the window
// is set from. fixture.TB has no Logf.
func logTimings(side string, o *sideObs) {
	log.Printf("%s: %s F1 closed=%t ms=%d bytes=%d kind=%s", fixtureName, side, o.f1.closed, o.f1.ms, o.f1.bytes, o.f1.kind)
	kinds := map[string]int{}
	var ms []float64
	if o.f1.closed {
		ms = append(ms, float64(o.f1.ms))
	}
	for i, c := range o.f2 {
		log.Printf("%s: %s F2[%02d] closed=%t ms=%d bytes=%d kind=%s", fixtureName, side, i, c.closed, c.ms, c.bytes, c.kind)
		kinds[c.kind]++
		if c.closed {
			ms = append(ms, float64(c.ms))
		}
	}
	log.Printf("%s: %s Z1 closed=%t ms=%d bytes=%d kind=%s", fixtureName, side, o.z1.closed, o.z1.ms, o.z1.bytes, o.z1.kind)
	log.Printf("%s: %s N1 closed=%t ms=%d bytes=%d kind=%s", fixtureName, side, o.n1.closed, o.n1.ms, o.n1.bytes, o.n1.kind)
	for _, a := range getArms {
		g := o.get[a.id]
		log.Printf("%s: %s %s %s status=%d body=%q err=%q", fixtureName, side, a.id, a.listener, g.status, g.body, g.err)
	}
	kk := make([]string, 0, len(kinds))
	for k, v := range kinds {
		kk = append(kk, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(kk)
	if len(ms) == 0 {
		log.Printf("%s: %s F1+F2 SPREAD n=0 (no close observed) F2 kinds %s", fixtureName, side, strings.Join(kk, ","))
		return
	}
	lo, hi, sum := ms[0], ms[0], 0.0
	for _, v := range ms {
		lo, hi, sum = math.Min(lo, v), math.Max(hi, v), sum+v
	}
	mean := sum / float64(len(ms))
	ss := 0.0
	for _, v := range ms {
		ss += (v - mean) * (v - mean)
	}
	sd := math.Sqrt(ss / float64(len(ms)))
	log.Printf("%s: %s F1+F2 SPREAD n=%d min=%.0f max=%.0f mean=%.1f sd=%.2f F2 kinds %s",
		fixtureName, side, len(ms), lo, hi, mean, sd, strings.Join(kk, ","))
}

// --- fixture.StatsAsserter ---

// AssertStats asserts, per side, every arm absolutely and the per-listener
// downstream_pre_cx_timeout BY VALUE, one Errorf per property. Fatalf is
// reserved for a broken precondition (a failed scrape, an unrecorded drive).
func (d *lftDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	d.mu.Lock()
	obs, addrs := d.obs, d.addrs
	d.mu.Unlock()
	for _, side := range []struct{ name, admin string }{{"ref", refAdminAddr}, {"subj", subjAdminAddr}} {
		o := obs[side.name]
		if o == nil {
			t.Fatalf("%s: no drive observations recorded — every assertion would be vacuous", side.name)
			return
		}
		assertArms(t, side.name, o)
		prom, err := scrapePromByAddr(side.admin)
		if err != nil {
			t.Fatalf("%s: scrape /stats/prometheus: %v", side.name, err)
			return
		}
		for i, l := range listeners {
			label := listenerLabel(side.name, l, addrs[side.name][l.name])
			assertPreCx(t, side.name, i, l, label, prom)
		}
	}
}

func assertArms(t fixture.TB, side string, o *sideObs) {
	t.Helper()
	// F1 — one silent client on l_false: closed, in window, zero bytes.
	if !o.f1.closed {
		t.Errorf("%s F1 l_false: silent client NOT closed by the server within %d ms (kind=%s) — "+
			"listener_filters_timeout 1s with continue:false must close it", side, silentHold.Milliseconds(), o.f1.kind)
	} else {
		if !inWindow(o.f1.ms) {
			t.Errorf("%s F1 l_false: closed at %d ms, want within [%d, %d] ms", side, o.f1.ms, windowLoMs, windowHiMs)
		}
		if o.f1.bytes != 0 {
			t.Errorf("%s F1 l_false: server sent %d bytes before closing, want 0", side, o.f1.bytes)
		}
	}
	// F2 — 50 concurrent silent clients: EVERY one closed in the window.
	var notClosed, outWin, withBytes []string
	for i, c := range o.f2 {
		switch {
		case !c.closed:
			notClosed = append(notClosed, fmt.Sprintf("#%d(%s@%dms)", i, c.kind, c.ms))
		case !inWindow(c.ms):
			outWin = append(outWin, fmt.Sprintf("#%d@%dms", i, c.ms))
		}
		if c.bytes != 0 {
			withBytes = append(withBytes, fmt.Sprintf("#%d:%dB", i, c.bytes))
		}
	}
	if len(notClosed) > 0 {
		t.Errorf("%s F2 l_false: %d of %d concurrent silent clients NOT closed within %d ms: %s",
			side, len(notClosed), f2Conns, silentHold.Milliseconds(), strings.Join(notClosed, " "))
	}
	if len(outWin) > 0 {
		t.Errorf("%s F2 l_false: %d of %d closes outside [%d, %d] ms: %s",
			side, len(outWin), f2Conns, windowLoMs, windowHiMs, strings.Join(outWin, " "))
	}
	if len(withBytes) > 0 {
		t.Errorf("%s F2 l_false: %d connections received bytes, want 0 each: %s", side, len(withBytes), strings.Join(withBytes, " "))
	}
	// F3 / T1 / T2 / T3 — the served chain, by body.
	for _, a := range getArms {
		g, ok := o.get[a.id]
		if !ok {
			t.Errorf("%s %s %s: no observation recorded", side, a.id, a.listener)
			continue
		}
		if g.err != "" {
			t.Errorf("%s %s %s: GET after %d ms failed: %s", side, a.id, a.listener, a.delay.Milliseconds(), g.err)
			continue
		}
		if g.status != wantStatus {
			t.Errorf("%s %s %s: status %d, want %d", side, a.id, a.listener, g.status, wantStatus)
		}
		if want := bodyFor(a.wantChain, a.listener); g.body != want {
			t.Errorf("%s %s %s: body %q, want %q", side, a.id, a.listener, g.body, want)
		}
	}
	// Z1 — 0s disables: still open at 16.5 s. N1 — no filters: still open.
	if o.z1.closed || o.z1.kind != "open" {
		t.Errorf("%s Z1 l_zero: silent client ended at %d ms (kind=%s), want still open at %d ms — "+
			"listener_filters_timeout 0s disables the deadline", side, o.z1.ms, o.z1.kind, z1Open.Milliseconds())
	}
	if o.n1.closed || o.n1.kind != "open" {
		t.Errorf("%s N1 l_nofilt: silent client ended at %d ms (kind=%s), want still open at %d ms — "+
			"a listener with no listener filters has nothing to time out", side, o.n1.ms, o.n1.kind, n1Open.Milliseconds())
	}
}

// assertPreCx pins downstream_pre_cx_timeout BY VALUE on this listener's own
// address label. A MISSING series is a hard failure, never read as 0.
func assertPreCx(t fixture.TB, side string, _ int, l lspec, label string, prom map[string]map[string]uint64) {
	t.Helper()
	const metric = "envoy_listener_downstream_pre_cx_timeout"
	series, ok := prom[metric][label]
	log.Printf("%s: %s S %s %s{envoy_listener_address=%q} = %d (present=%t) want %d",
		fixtureName, side, l.name, metric, label, series, ok, l.wantPreCx)
	if !ok {
		have := make([]string, 0, len(prom[metric]))
		for k := range prom[metric] {
			have = append(have, k)
		}
		sort.Strings(have)
		t.Errorf("%s S %s: %s{envoy_listener_address=%q} ABSENT (series present: %v), want present and == %d",
			side, l.name, metric, label, have, l.wantPreCx)
		return
	}
	if series != l.wantPreCx {
		t.Errorf("%s S %s: %s = %d, want %d", side, l.name, metric, series, l.wantPreCx)
	}
}

// listenerLabel is each side's OWN envoy_listener_address spelling: the
// reference renders "0.0.0.0_<in-container port>"; envoy-go renders its
// configured bind address with ':' and '.' folded to '_'.
func listenerLabel(side string, l lspec, addr string) string {
	if side == "ref" {
		return "0.0.0.0_" + strconv.Itoa(l.refPort)
	}
	return strings.NewReplacer(":", "_", ".", "_").Replace(addr)
}

func bodyFor(chain, listener string) string { return chain + " " + listener + "\n" }

// scrapePromByAddr fetches /stats/prometheus and returns
// metric -> envoy_listener_address -> value, keeping ONLY series that carry an
// envoy_listener_address label.
func scrapePromByAddr(adminAddr string) (map[string]map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	const key = `envoy_listener_address="`
	out := map[string]map[string]uint64{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		open := strings.IndexByte(line, '{')
		closeIdx := strings.LastIndexByte(line, '}')
		if strings.HasPrefix(line, "#") || open < 0 || closeIdx < open {
			continue
		}
		labels := line[open+1 : closeIdx]
		ki := strings.Index(labels, key)
		if ki < 0 {
			continue
		}
		val := labels[ki+len(key):]
		end := strings.IndexByte(val, '"')
		if end < 0 {
			continue
		}
		rest := strings.Fields(line[closeIdx+1:])
		if len(rest) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(rest[0], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			continue
		}
		name := line[:open]
		if out[name] == nil {
			out[name] = map[string]uint64{}
		}
		out[name][val[:end]] += uint64(v)
	}
	return out, nil
}

// --- bootstrap rendering ---

// renderBootstrap renders BOTH sides from one template, differing only in bind
// address, admin port and listener ports.
func renderBootstrap(bindAddr string, adminPort int, portFor func(i int) int, backendPort int) string {
	var b strings.Builder
	fmt.Fprintf(&b, bootstrapHeadTmpl, bindAddr, adminPort)
	for i, l := range listeners {
		lf := ""
		if l.filters {
			lf = listenerFiltersBlock
		}
		fmt.Fprintf(&b, listenerTmpl,
			l.name,         // 1
			bindAddr,       // 2
			portFor(i),     // 3
			l.timeout,      // 4
			l.cont,         // 5
			lf,             // 6
			l.indexedMatch, // 7
			strconv.Quote(bodyFor(chainIndexed, l.name)), // 8
			strconv.Quote(bodyFor(chainDefault, l.name)), // 9
			strings.TrimPrefix(l.name, "l_"),             // 10 stat_prefix stem
		)
	}
	fmt.Fprintf(&b, clustersTmpl, backendPort)
	return b.String()
}

const bootstrapHeadTmpl = `admin:
  address:
    socket_address: { address: %s, port_value: %d }
static_resources:
  listeners:
`

const listenerFiltersBlock = `      listener_filters:
        - name: envoy.filters.listener.tls_inspector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
`

// listenerTmpl: stat_prefixes are <stem>_indexed / <stem>_default — ALL TEN
// DISTINCT (two HCMs sharing a stat_prefix panic envoy-go at boot).
const listenerTmpl = `    - name: %[1]s
      address:
        socket_address: { address: %[2]s, port_value: %[3]d }
      listener_filters_timeout: %[4]s
      continue_on_listener_filters_timeout: %[5]t
%[6]s      filter_chains:
        - name: fc_indexed
          filter_chain_match:
            transport_protocol: %[7]s
          filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: %[10]s_indexed
                route_config:
                  name: rc_%[10]s_indexed
                  virtual_hosts:
                    - name: vh_%[10]s_indexed
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 200
                            body: { inline_string: %[8]s }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
      default_filter_chain:
        name: fc_default
        filters:
          - name: envoy.filters.network.http_connection_manager
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
              stat_prefix: %[10]s_default
              route_config:
                name: rc_%[10]s_default
                virtual_hosts:
                  - name: vh_%[10]s_default
                    domains: ["*"]
                    routes:
                      - match: { prefix: "/" }
                        direct_response:
                          status: 200
                          body: { inline_string: %[9]s }
              http_filters:
                - name: envoy.filters.http.router
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`

const clustersTmpl = `  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`
