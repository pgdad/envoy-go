// Package driver implements the 0100-http-tap-bodies differential fixture.
//
// The predicate is any_match: true -- bodies are the SUBJECT of this
// fixture, not the predicate, so every stream is tapped (unlike 0099's
// and_match on headers). output_config carries max_buffered_rx_bytes and
// max_buffered_tx_bytes (both == capC) as siblings of sinks.
//
// The backend is HTTPEchoBody (kind 6): it echoes the POST body back
// verbatim as the response body, so one POST populates BOTH request.body
// (rx, capped at max_buffered_rx_bytes) and response.body (tx, capped at
// max_buffered_tx_bytes) with the same content -- one config exercises both
// directions.
//
// drive issues three POSTs whose bodies pin the three interesting arms of
// the max_buffered_*_bytes cap (capC == 20): bodyFull (10 bytes, < C: full,
// not truncated), bodyBound (20 bytes, == C: full, NOT truncated -- strict
// `>`), bodyTrunc (40 bytes, > C: truncated to the first 20 bytes).
//
// AssertStats performs the cross-side glob-and-decode trace assertions: it
// globs both sides' output directories for out_*.json trace files, decodes
// each as a datatapv3.TraceWrapper via protojson, and asserts (P1) body
// PRESENT on every arm, (P2) the as_string payload multiset, and (P3) the
// truncated-flag multiset -- each a distinct t.Errorf so the four deliberate
// breaks each land on an identifiable, isolated assertion.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"text/template"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	datatapv3 "github.com/envoyproxy/go-control-plane/envoy/data/tap/v3"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	refListenerPort = 10000
	refTapDir       = "/envoy-go-test/taps" // non-sticky container dir (see HostMount.Dir)
	refTapPrefix    = refTapDir + "/out"

	// statPrefix is the HCM stat_prefix set identically in both envoy.yaml
	// and envoy-go.yaml, so the emitted stat name matches side-to-side.
	statPrefix = "tap_probe"

	capC      = 20                                         // max_buffered_rx/tx_bytes
	bodyFull  = "0123456789"                               // 10 bytes  (< C: full, not truncated)
	bodyBound = "0123456789ABCDEFGHIJ"                     // 20 bytes  (== C: full, NOT truncated -- strict >)
	bodyTrunc = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcd" // 40 bytes  (> C: truncated to first 20)
	nTraces   = 3
)

type tapDriver struct {
	refDir  string // host dir bind-mounted onto refTapDir
	subjDir string // host dir the subject writes into directly
}

func newTapDriver() *tapDriver {
	base := os.TempDir()
	tag := fmt.Sprintf("envoy-go-0100-%d-%d", os.Getpid(), time.Now().UnixNano())
	return &tapDriver{
		refDir:  filepath.Join(base, tag+"-reference"),
		subjDir: filepath.Join(base, tag+"-subject"),
	}
}

func init() { fixture.RegisterFixture("0100-http-tap-bodies", newTapDriver()) }

// Compile-time interface assertions.
var (
	_ fixture.Driver              = (*tapDriver)(nil)
	_ fixture.BackendKindAware    = (*tapDriver)(nil)
	_ fixture.ReferenceLogMounter = (*tapDriver)(nil)
	_ fixture.StatsAsserter       = (*tapDriver)(nil)
)

// BackendCount returns 1: the runner rejects 0. The single HTTPEchoBody
// backend is the real upstream (it returns the request body, echoed, as the
// response body -- both proxies proxy the same echo).
func (d *tapDriver) BackendCount() int                { return 1 }
func (d *tapDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEchoBody }
func (d *tapDriver) SubjectListenerName() string      { return "listener_0" }
func (d *tapDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceHostMounts bind-mounts a host DIRECTORY over the container's tap
// output dir: file_per_tap creates <prefix>_<trace_id>.json per stream, and
// the test cannot predict those names (D-TAP-DIRMOUNT).
func (d *tapDriver) ReferenceHostMounts() []fixture.HostMount {
	return []fixture.HostMount{{HostPath: d.refDir, ContainerPath: refTapDir, Dir: true}}
}

func (d *tapDriver) subjPrefix() string { return filepath.Join(d.subjDir, "out") }

// --- file / template helpers (mirrors the 0099-http-tap-headers driver's
// mustReadFixtureFile/mustRender pair) ---

// fixtureDir returns the absolute path to the 0100-http-tap-bodies fixture
// root (one directory above this file's driver/ parent), derived from
// runtime.Caller -- works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0100-http-tap-bodies/driver/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

// mustReadFixtureFile reads name from the fixture root directory.
func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

// mustRender renders a text/template body with data; panics on parse/exec
// errors (driver-time misconfiguration is non-recoverable).
func mustRender(tpl string, data map[string]any) string {
	t, err := template.New("bootstrap").Parse(tpl)
	if err != nil {
		panic(fmt.Sprintf("driver: template parse: %v", err))
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("driver: template execute: %v", err))
	}
	return buf.String()
}

func (d *tapDriver) ReferenceBootstrap(backendPorts []int) string {
	return mustRender(mustReadFixtureFile("envoy.yaml"), map[string]any{
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
		"TapPrefix":    refTapPrefix,
		"Cap":          capC,
	})
}

func (d *tapDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return mustRender(mustReadFixtureFile("envoy-go.yaml"), map[string]any{
		"ListenerPort": subjListenerPort,
		"AdminPort":    subjAdminPort,
		"BackendHost":  "127.0.0.1",
		"BackendPort":  backendPorts[0],
		"TapPrefix":    d.subjPrefix(),
		"Cap":          capC,
	})
}

func (d *tapDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}
func (d *tapDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}

// drive issues three POSTs /echo against the HTTPEchoBody backend, one per
// cap arm (full / boundary / truncated -- see the const block). The backend
// echoes the request body back as the response body, so each POST populates
// BOTH request.body and response.body with the same content.
func (d *tapDriver) drive(ctx context.Context, addr string) ([]byte, error) {
	c := &http.Client{Timeout: 5 * time.Second}
	var out bytes.Buffer
	post := func(body string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/echo",
			strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("content-type", "text/plain")
		resp, err := c.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		fmt.Fprintf(&out, "POST %d %d\n", len(body), resp.StatusCode)
		return nil
	}
	for _, b := range []string{bodyFull, bodyBound, bodyTrunc} {
		if err := post(b); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes (status line + headers + body) for the
// runner's standard admin-diff at the probe step (matches the
// 0099-and-earlier precedent; compareAdminResponses parses these as a
// wire-format HTTP response, so a body-only read is not sufficient here).
func (d *tapDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
	refBytes, err := helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err := helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// AssertStats performs the cross-side glob-and-decode trace assertions.
// Called by the runner (not AssertSubject: 0100 is cross-side, so
// AssertSubject would never run -- see reference_differential_asserter_dispatch).
func (d *tapDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	// (1) trace counts. The reference writes at stream end but the container's
	// filesystem lands asynchronously from the host's view; poll it. The subject
	// writes in-process and is already done.
	refFiles := pollTraces(t, d.refDir, nTraces, 30*time.Second)
	subjFiles := pollTraces(t, d.subjDir, nTraces, 5*time.Second)
	if got := len(refFiles); got != nTraces {
		t.Errorf("reference trace count = %d, want %d (any_match: true taps every stream)", got, nTraces)
	}
	if got := len(subjFiles); got != nTraces {
		t.Errorf("subject trace count = %d, want %d (any_match: true taps every stream)", got, nTraces)
	}

	// (2) rq_tapped on both sides.
	const statName = "http." + statPrefix + ".tap.rq_tapped"
	if got := scrapeCounter(t, ctx, refAdminAddr, statName); got != nTraces {
		t.Errorf("reference %s = %d, want %d", statName, got, nTraces)
	}
	if got := scrapeCounter(t, ctx, subjAdminAddr, statName); got != nTraces {
		t.Errorf("subject %s = %d, want %d", statName, got, nTraces)
	}

	// (3)+ per-trace payload assertions on BOTH sides.
	wantReq := map[string]string{":method": "POST", ":path": "/echo"}
	wantResp := map[string]string{":status": "200", "content-type": "text/plain"}
	assertSide(t, "reference", refFiles, wantReq, wantResp)
	assertSide(t, "subject", subjFiles, wantReq, wantResp)
}

// bodyObs is one trace's observed (as_string, truncated) pair for a single
// direction (request or response).
type bodyObs struct {
	asString  string
	truncated bool
}

// wantPayloads/wantTruncated are the expected multisets across the three
// POSTs, independent of file order (out_*.json filenames are unpredictable
// trace IDs, not driven-order). bodyTrunc's first capC bytes equal bodyBound
// verbatim (both are the "0123456789ABCDEFGHIJ" prefix), so the boundary and
// truncated arms capture the SAME 20-char payload -- only their `truncated`
// flags differ.
var (
	wantPayloads  = []string{bodyFull, bodyBound, bodyTrunc[:capC]}
	wantTruncated = []bool{false, false, true} // full=false, boundary=false (strict >), truncated=true
)

func assertSide(t fixture.TB, side string, files []string, wantReq, wantResp map[string]string) {
	t.Helper()
	var reqObs, respObs []bodyObs
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: read %s: %v", side, path, err) // broken precondition
		}
		var tw datatapv3.TraceWrapper
		if err := protojson.Unmarshal(b, &tw); err != nil {
			t.Fatalf("%s: decode %s: %v", side, filepath.Base(path), err) // broken precondition
		}
		bt := tw.GetHttpBufferedTrace()
		if bt == nil {
			t.Fatalf("%s: %s: trace is not an http_buffered_trace", side, filepath.Base(path)) // broken precondition
		}
		name := side + "/" + filepath.Base(path)

		// header subsets (secondary to the body block; unchanged shape from 0099).
		assertSubset(t, name+" request.headers", bt.GetRequest().GetHeaders(), wantReq)
		assertSubset(t, name+" response.headers", bt.GetResponse().GetHeaders(), wantResp)

		// (P1) body PRESENT on every arm -- the inverse of 0099's absent-assertion.
		// [break (a): inert DecodeData/EncodeData hooks fire HERE -- bodies stay nil.]
		// The two directions are checked and collected INDEPENDENTLY (no
		// Fatalf/continue) so a one-sided regression (e.g. Encoder-only
		// inert) doesn't hide the other direction's P1/P2/P3 signal.
		reqBody := bt.GetRequest().GetBody()
		respBody := bt.GetResponse().GetBody()
		if reqBody == nil {
			t.Errorf("%s: request.body must be PRESENT (non-empty POST); got nil", name)
		} else {
			reqObs = append(reqObs, bodyObs{reqBody.GetAsString(), reqBody.GetTruncated()})
		}
		if respBody == nil {
			t.Errorf("%s: response.body must be PRESENT (echoed body); got nil", name)
		} else {
			respObs = append(respObs, bodyObs{respBody.GetAsString(), respBody.GetTruncated()})
		}
	}

	// (P2) payload multiset -- cross-side EXACT.
	// [breaks (b) ignore-cap and (d) wrong-oneof fire HERE: (b) makes the
	// truncated arm's payload 40 bytes instead of the 20-byte prefix; (d)
	// decodes as_string empty because the wrong oneof arm was populated.]
	assertPayloadMultiset(t, side+" request.body.as_string", reqObs, wantPayloads)
	assertPayloadMultiset(t, side+" response.body.as_string", respObs, wantPayloads)

	// (P3) truncated-flag multiset -- cross-side EXACT.
	// [break (c) strict->->= fires HERE ONLY: it flips the boundary arm's flag
	// false->true, making the multiset {false,true,true} (2 trues) instead of
	// {false,false,true} (1 true), while P2's captured prefix is unchanged --
	// per reference_deliberate_break_wrong_assertion, this separation from P2
	// is load-bearing.]
	assertTruncatedMultiset(t, side+" request.body.truncated", reqObs, wantTruncated)
	assertTruncatedMultiset(t, side+" response.body.truncated", respObs, wantTruncated)
}

// assertPayloadMultiset asserts the sorted as_string values across obs equal
// the sorted want values -- a SET/multiset comparison because file order
// (unpredictable trace IDs) need not match POST-drive order.
func assertPayloadMultiset(t fixture.TB, what string, obs []bodyObs, want []string) {
	t.Helper()
	got := make([]string, len(obs))
	for i, o := range obs {
		got[i] = o.asString
	}
	sort.Strings(got)
	wantSorted := append([]string(nil), want...)
	sort.Strings(wantSorted)
	if !slices.Equal(got, wantSorted) {
		t.Errorf("%s multiset = %q, want %q", what, got, wantSorted)
	}
}

// assertTruncatedMultiset asserts the sorted truncated bools across obs
// equal the sorted want bools (equivalently: same count of true).
func assertTruncatedMultiset(t fixture.TB, what string, obs []bodyObs, want []bool) {
	t.Helper()
	got := make([]bool, len(obs))
	for i, o := range obs {
		got[i] = o.truncated
	}
	boolSort := func(s []bool) { sort.Slice(s, func(i, j int) bool { return !s[i] && s[j] }) }
	boolSort(got)
	wantSorted := append([]bool(nil), want...)
	boolSort(wantSorted)
	if !slices.Equal(got, wantSorted) {
		t.Errorf("%s multiset = %v, want %v", what, got, wantSorted)
	}
}

// assertSubset checks want ⊆ got, comparing as a SET: header ORDER is an
// UNasserted boundary (envoy-go sorts; the reference emits codec order).
func assertSubset(t fixture.TB, what string, got []*corev3.HeaderValue, want map[string]string) {
	t.Helper()
	have := make(map[string]string, len(got))
	for _, hv := range got {
		have[hv.GetKey()] = hv.GetValue()
	}
	for k, v := range want {
		gv, ok := have[k]
		if !ok {
			t.Errorf("%s: missing key %q (have %v)", what, k, keysOf(have))
			continue
		}
		if gv != v {
			t.Errorf("%s: key %q = %q, want %q", what, k, gv, v)
		}
	}
}

// keysOf returns the sorted keys of a map[string]string, for readable
// failure messages.
func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// pollTraces globs dir for out_*.json trace files until len == want or
// deadline expires, returning whatever it last saw. It NEVER Fatalfs on a
// short count, so assertion (1) in AssertStats can report the real number.
func pollTraces(t fixture.TB, dir string, want int, deadline time.Duration) []string {
	t.Helper()
	pattern := filepath.Join(dir, "out_*.json")
	end := time.Now().Add(deadline)
	var files []string
	for {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("pollTraces: glob %s: %v", pattern, err) // broken precondition
		}
		files = matches
		if len(files) >= want || time.Now().After(end) {
			return files
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// scrapeCounter fetches the plain-text admin /stats endpoint (NOT
// /stats/prometheus, whose names are mangled by ExtractTags) and parses the
// "<name>: <value>" line for name. Fatalf if the line is absent -- a missing
// counter is a broken precondition (on the reference side its absence would
// mean the filter never parsed).
func scrapeCounter(t fixture.TB, ctx context.Context, adminAddr, name string) int {
	t.Helper()
	url := "http://" + adminAddr + "/stats"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("scrapeCounter: build request %s: %v", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("scrapeCounter: GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("scrapeCounter: read %s: %v", url, err)
	}
	prefix := name + ": "
	for _, line := range bytes.Split(b, []byte("\n")) {
		s := string(bytes.TrimSpace(line))
		if !bytes.HasPrefix([]byte(s), []byte(prefix)) {
			continue
		}
		var v int
		if _, err := fmt.Sscanf(s, prefix+"%d", &v); err != nil {
			t.Fatalf("scrapeCounter: parse %q: %v", s, err)
		}
		return v
	}
	t.Fatalf("scrapeCounter: %s: line %q not found in /stats", adminAddr, name) // broken precondition
	return 0
}
