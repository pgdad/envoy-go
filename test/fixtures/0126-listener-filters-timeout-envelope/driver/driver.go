// Package driver registers the 0126-listener-filters-timeout-envelope fixture
// with the differential runner. See ../README.md for the fixture's purpose.
//
// THE PROPOSITION (phase 101, SPEC §7): reference Envoy accepts
// Listener.listener_filters_timeout outside envoy-go's old [1s, 60s] envelope
// and enforces it as a millisecond count truncated from the Duration: 0.5s
// closes a silent client at ~500 ms, 4294968s does NOT wrap (a uint32 of ms
// would wrap it to ~705 ms), and 0.0005s truncates to 0 = disabled. The
// un-fixed envoy-go boot-rejects every one of these values.
//
// Six plaintext TCP listeners, each a ONE-LINE diff from one base
// (tls_inspector, fc_indexed matching transport_protocol tls, a last-resort
// default_filter_chain):
//
//	listener      delta                                 pre_cx (both sides)
//	l_half        0.5s, continue: false                 1 (H1)
//	l_half_true   0.5s, continue: true                  1 (T1 falls through)
//	l_one_true    1s, continue: true (T1's MIRROR)      0 (T2's GET beats it)
//	l_wrap        4294968s, continue: false             0 (W1: no wrap)
//	l_subms       0.0005s, continue: false              0 (M1: truncates to 0)
//	l_nofilt_120  no listener_filters, 120s, false      0 (N1: nothing to time)
//
// Every listener's fc_indexed matches "tls" and every client is plaintext, so
// every served body is the DEFAULT chain's: on a `true` listener the body
// cannot tell a fall-through from an early GET. The discriminator is the
// counter (S): l_half_true 1 beside its mirror l_one_true 0.
//
// ⚠️ WHAT THIS FIXTURE DELIBERATELY DOES NOT PIN (SPEC §7.7): any reject
// message, a close KIND (recorded, never asserted), an exact millisecond
// (windows only), downstream_cx_total on a drop listener, any
// downstream_listener_filter_* name, and the 1 ms immediate-request outcome.
//
// Every arm on a side runs CONCURRENTLY, so a side's drive costs max(arm),
// ~1.5 s (W1/M1's holds; H1 closes at ~0.5 s), not the sum of the arms.
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

const fixtureName = "0126-listener-filters-timeout-envelope"

// refAdminPort is the in-container reference admin port, fixed at 9901 by the
// harness.
const refAdminPort = 9901

// drivenPath is the path every GET arm requests. Both chains of every listener
// route "/" to a direct_response; the BODY names the chain.
const drivenPath = "/lft"

// wantStatus is the direct_response status on every chain.
const wantStatus = 200

// The H1 close window, in milliseconds from the client's dial returning.
//
// ⚠️ SET FROM MEASUREMENT, NOT FROM THE CONFIG. The configured deadline is
// 500 ms on both sides; README §Window records the per-side spreads and the
// σ-margin arithmetic. The window excludes an immediate close and a mis-parse
// to the 1 s mirror or the 15 s default. Widen it with a new measurement,
// never drop the arm.
const (
	windowLoMs = 350
	windowHiMs = 900
)

// Arm timing.
const (
	h1Hold     = 2000 * time.Millisecond // H1: hold a silent client this long
	openHold   = 1500 * time.Millisecond // W1/M1: still open at this point
	tDelay     = 800 * time.Millisecond  // T1/T2: silent this long, then GET
	getTimeout = 5 * time.Second         // a GET arm's read budget after its write
)

// Body tokens. The served body is "<token> <listener>\n", so one observed body
// names both the chain and the listener that produced it.
const (
	chainIndexed = "INDEXED"
	chainDefault = "DEFAULT"
)

// indexedMatch is fc_indexed's filter_chain_match.transport_protocol on EVERY
// listener: no plaintext client ever matches it.
const indexedMatch = "tls"

// lspec is one listener. Order is the index-wise zip the runner performs
// between SubjectListenerNames() and ReferenceListenerPorts().
type lspec struct {
	name    string
	refPort int // in-container reference port — CENSUSED (SPEC §7.3)
	// filters: whether the listener carries the tls_inspector listener filter.
	filters bool
	// timeout is the listener_filters_timeout literal — spelled in seconds,
	// never "500ms" (protojson).
	timeout string
	// cont is continue_on_listener_filters_timeout.
	cont bool
	// wantPreCx is downstream_pre_cx_timeout after the drive, BY VALUE.
	wantPreCx uint64
}

var listeners = []lspec{
	{name: "l_half", refPort: 15126, filters: true, timeout: "0.5s", cont: false, wantPreCx: 1},
	{name: "l_half_true", refPort: 15232, filters: true, timeout: "0.5s", cont: true, wantPreCx: 1},
	{name: "l_one_true", refPort: 15233, filters: true, timeout: "1s", cont: true, wantPreCx: 0},
	{name: "l_wrap", refPort: 15234, filters: true, timeout: "4294968s", cont: false, wantPreCx: 0},
	{name: "l_subms", refPort: 15235, filters: true, timeout: "0.0005s", cont: false, wantPreCx: 0},
	{name: "l_nofilt_120", refPort: 15236, filters: false, timeout: "120s", cont: false, wantPreCx: 0},
}

func init() { fixture.RegisterFixture(fixtureName, &lfeDriver{}) }

// lfeDriver is STATEFUL: each side's Drive records its observations so
// AssertStats can assert every arm absolutely, per side, one Errorf per
// property. A Drive-time error would become t.Fatalf and MASK every later arm.
type lfeDriver struct {
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
	h1, w1, m1 closeObs
	get        map[string]getObs // arm -> outcome (T1, T2, N1)
}

// openArm is one silent client that must still be open at openHold.
type openArm struct {
	id       string
	listener string
	why      string
}

var openArms = []openArm{
	{id: "W1", listener: "l_wrap", why: "4294968s is 4294968000 ms; a uint32 of ms wraps it to ~705 ms"},
	{id: "M1", listener: "l_subms", why: "0.0005s truncates to 0 ms, which disables the deadline"},
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
	{id: "T1", listener: "l_half_true", delay: tDelay, wantChain: chainDefault},
	{id: "T2", listener: "l_one_true", delay: tDelay, wantChain: chainDefault},
	{id: "N1", listener: "l_nofilt_120", delay: 0, wantChain: chainDefault},
}

var (
	_ fixture.Driver              = (*lfeDriver)(nil)
	_ fixture.MultiListenerDriver = (*lfeDriver)(nil)
	_ fixture.StatsAsserter       = (*lfeDriver)(nil)
)

// --- fixture.Driver ---

// BackendCount is 1: a never-dialed placeholder (the runner rejects 0 and
// envoy-go boot-rejects an absent static_resources.clusters key).
func (*lfeDriver) BackendCount() int { return 1 }

func (*lfeDriver) SubjectListenerName() string { return listeners[0].name }

func (*lfeDriver) ReferenceListenerPort() int { return listeners[0].refPort }

func (*lfeDriver) ReferenceBootstrap(backendPorts []int) string {
	return renderBootstrap("0.0.0.0", refAdminPort, func(i int) int { return listeners[i].refPort }, backendPorts[0])
}

// SubjectConfig binds the six subject listeners at subjListenerPort+0..+5,
// inside the runner's probed 16-port block.
func (*lfeDriver) SubjectConfig(_ int, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return renderBootstrap("127.0.0.1", subjAdminPort, func(i int) int { return subjListenerPort + i }, backendPorts[0])
}

// DriveReference / DriveSubject are UNREACHABLE while MultiListenerDriver is
// implemented; they refuse rather than derive sibling addresses.
func (*lfeDriver) DriveReference(context.Context, string) ([]byte, error) {
	return nil, errors.New("0126 drives only through DriveReferenceMulti")
}

func (*lfeDriver) DriveSubject(context.Context, string) ([]byte, error) {
	return nil, errors.New("0126 drives only through DriveSubjectMulti")
}

func (*lfeDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

func (*lfeDriver) SubjectListenerNames() []string {
	out := make([]string, len(listeners))
	for i, l := range listeners {
		out[i] = l.name
	}
	return out
}

func (*lfeDriver) ReferenceListenerPorts() []int {
	out := make([]int, len(listeners))
	for i, l := range listeners {
		out[i] = l.refPort
	}
	return out
}

func (d *lfeDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.drive(ctx, "ref", addrs)
}

func (d *lfeDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.drive(ctx, "subj", addrs)
}

// drive runs EVERY arm on one side concurrently and emits a side-independent,
// timing-free verdict stream for the runner's CompareBytes. Only a missing
// address returns an error; every per-arm failure is recorded and asserted in
// AssertStats.
func (d *lfeDriver) drive(ctx context.Context, side string, addrs map[string]string) ([]byte, error) {
	for _, l := range listeners {
		if addrs[l.name] == "" {
			return nil, fmt.Errorf("%s: no address supplied for listener %q (have %d entries)", side, l.name, len(addrs))
		}
	}
	o := &sideObs{get: map[string]getObs{}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	start := time.Now()

	wg.Add(3)
	go func() { defer wg.Done(); o.h1 = silentProbe(ctx, addrs["l_half"], h1Hold) }()
	go func() { defer wg.Done(); o.w1 = silentProbe(ctx, addrs["l_wrap"], openHold) }()
	go func() { defer wg.Done(); o.m1 = silentProbe(ctx, addrs["l_subms"], openHold) }()
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
	fmt.Fprintf(&b, "H1 l_half closed=%t in_window=%t bytes=%d\n", o.h1.closed, o.h1.closed && inWindow(o.h1.ms), o.h1.bytes)
	fmt.Fprintf(&b, "W1 l_wrap open_at_%dms=%t\n", openHold.Milliseconds(), isOpen(o.w1))
	fmt.Fprintf(&b, "M1 l_subms open_at_%dms=%t\n", openHold.Milliseconds(), isOpen(o.m1))
	for _, a := range getArms {
		g := o.get[a.id]
		fmt.Fprintf(&b, "%s %s status=%d body=%q err=%q\n", a.id, a.listener, g.status, g.body, g.err)
	}
	return b.Bytes(), nil
}

func inWindow(ms int64) bool { return ms >= windowLoMs && ms <= windowHiMs }

func isOpen(c closeObs) bool { return !c.closed && c.kind == "open" }

func (o *sideObs) open(id string) closeObs {
	if id == "W1" {
		return o.w1
	}
	return o.m1
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
	req := "GET " + drivenPath + " HTTP/1.1\r\nHost: lfe\r\nConnection: close\r\n\r\n"
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

// logTimings RECORDS every silent connection's raw outcome and every GET's
// observed body, pass or fail, so a green run still leaves the measurement the
// window is set from. fixture.TB has no Logf.
func logTimings(side string, o *sideObs) {
	for _, s := range []struct {
		id, listener string
		c            closeObs
	}{{"H1", "l_half", o.h1}, {"W1", "l_wrap", o.w1}, {"M1", "l_subms", o.m1}} {
		log.Printf("%s: %s %s %s closed=%t ms=%d bytes=%d kind=%s",
			fixtureName, side, s.id, s.listener, s.c.closed, s.c.ms, s.c.bytes, s.c.kind)
	}
	for _, a := range getArms {
		g := o.get[a.id]
		log.Printf("%s: %s %s %s status=%d body=%q err=%q", fixtureName, side, a.id, a.listener, g.status, g.body, g.err)
	}
}

// --- fixture.StatsAsserter ---

// AssertStats asserts, per side, every arm absolutely and the per-listener
// downstream_pre_cx_timeout BY VALUE, one Errorf per property. Fatalf is
// reserved for a broken precondition (a failed scrape, an unrecorded drive).
func (d *lfeDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
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
		for _, l := range listeners {
			assertPreCx(t, side.name, l, listenerLabel(side.name, l, addrs[side.name][l.name]), prom)
		}
	}
}

func assertArms(t fixture.TB, side string, o *sideObs) {
	t.Helper()
	// H1 — one silent client on l_half (0.5s, false): closed, in window, 0 bytes.
	if !o.h1.closed {
		t.Errorf("%s H1 l_half: silent client NOT closed by the server within %d ms (kind=%s) — "+
			"listener_filters_timeout 0.5s with continue:false must close it", side, h1Hold.Milliseconds(), o.h1.kind)
	} else {
		if !inWindow(o.h1.ms) {
			t.Errorf("%s H1 l_half: closed at %d ms, want within [%d, %d] ms", side, o.h1.ms, windowLoMs, windowHiMs)
		}
		if o.h1.bytes != 0 {
			t.Errorf("%s H1 l_half: server sent %d bytes before closing, want 0", side, o.h1.bytes)
		}
	}
	// W1 / M1 — still open at openHold.
	for _, a := range openArms {
		if c := o.open(a.id); !isOpen(c) {
			t.Errorf("%s %s %s: silent client ended at %d ms (kind=%s), want still open at %d ms — %s",
				side, a.id, a.listener, c.ms, c.kind, openHold.Milliseconds(), a.why)
		}
	}
	// T1 / T2 / N1 — the served chain, by body.
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
}

// assertPreCx pins downstream_pre_cx_timeout BY VALUE on this listener's own
// address label. A MISSING series is a hard failure, never read as 0.
func assertPreCx(t fixture.TB, side string, l lspec, label string, prom map[string]map[string]uint64) {
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
			l.name,       // 1
			bindAddr,     // 2
			portFor(i),   // 3
			l.timeout,    // 4
			l.cont,       // 5
			lf,           // 6
			indexedMatch, // 7
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

// listenerTmpl: stat_prefixes are <stem>_indexed / <stem>_default — ALL
// TWELVE DISTINCT (two HCMs sharing a stat_prefix panic envoy-go at boot).
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
