// Package driver implements the 0099-http-tap-headers differential fixture.
//
// The predicate is and_match{ http_request_headers_match(x-tap exact "yes"),
// http_response_headers_match(:status exact "204") }: one arm per direction,
// so the trace can only be emitted once BOTH have been observed.
//
// AMEND-TAP-BODY: the backend is the existing HTTPStatusHeader kind (kind 3):
// it reads X-Backend-Status and returns that status. Driving GET /tap with
// x-backend-status: 204 yields a bodyless request AND a bodyless 204
// response (net/http emits "204 No Content", no Content-Length, zero body
// bytes). A zero-length body message omits the `body` field entirely, so
// both sides emit structurally headers-only traces. BackendCount stays 1
// (the runner rejects 0).
//
// AssertStats (Task 14) performs the real glob-and-decode cross-side trace
// assertions: it globs both sides' output directories for out_*.json trace
// files, decodes each as a datatapv3.TraceWrapper via protojson, and asserts
// the request/response header subsets, the rq_tapped counter, and the
// documented ABSENT fields (bodies, trailers, downstream_connection).
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
	"sort"
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

	nMatch    = 3 // requests carrying x-tap: yes  -> tapped
	nNonMatch = 2 // requests carrying x-tap: no   -> NOT tapped

	// statPrefix is the HCM stat_prefix set identically in both envoy.yaml
	// and envoy-go.yaml, so the emitted stat name matches side-to-side.
	statPrefix = "tap_probe"
)

type tapDriver struct {
	refDir  string // host dir bind-mounted onto refTapDir
	subjDir string // host dir the subject writes into directly
}

func newTapDriver() *tapDriver {
	base := os.TempDir()
	tag := fmt.Sprintf("envoy-go-0099-%d-%d", os.Getpid(), time.Now().UnixNano())
	return &tapDriver{
		refDir:  filepath.Join(base, tag+"-reference"),
		subjDir: filepath.Join(base, tag+"-subject"),
	}
}

func init() { fixture.RegisterFixture("0099-http-tap-headers", newTapDriver()) }

// Compile-time interface assertions.
var (
	_ fixture.Driver              = (*tapDriver)(nil)
	_ fixture.BackendKindAware    = (*tapDriver)(nil)
	_ fixture.ReferenceLogMounter = (*tapDriver)(nil)
	_ fixture.StatsAsserter       = (*tapDriver)(nil)
)

// BackendCount returns 1: the runner rejects 0. The single HTTPStatusHeader
// backend is the real upstream (it returns the 204 both sides proxy).
func (d *tapDriver) BackendCount() int                { return 1 }
func (d *tapDriver) BackendKind() fixture.BackendKind { return fixture.HTTPStatusHeader }
func (d *tapDriver) SubjectListenerName() string      { return "listener_0" }
func (d *tapDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceHostMounts bind-mounts a host DIRECTORY over the container's tap
// output dir: file_per_tap creates <prefix>_<trace_id>.json per stream, and
// the test cannot predict those names (D-TAP-DIRMOUNT).
func (d *tapDriver) ReferenceHostMounts() []fixture.HostMount {
	return []fixture.HostMount{{HostPath: d.refDir, ContainerPath: refTapDir, Dir: true}}
}

func (d *tapDriver) subjPrefix() string { return filepath.Join(d.subjDir, "out") }

// --- file / template helpers (mirrors the 0018-http-rbac driver's
// mustReadFixtureFile/mustRender pair) ---

// fixtureDir returns the absolute path to the 0099-http-tap-headers fixture
// root (one directory above this file's driver/ parent), derived from
// runtime.Caller -- works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0099-http-tap-headers/driver/driver.go
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
	})
}

func (d *tapDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return mustRender(mustReadFixtureFile("envoy-go.yaml"), map[string]any{
		"ListenerPort": subjListenerPort,
		"AdminPort":    subjAdminPort,
		"BackendHost":  "127.0.0.1",
		"BackendPort":  backendPorts[0],
		"TapPrefix":    d.subjPrefix(),
	})
}

func (d *tapDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}
func (d *tapDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}

// drive sends nMatch matching + nNonMatch non-matching BODYLESS GETs. The
// backend echoes X-Backend-Status, so every response is a bodyless 204 --
// which is what makes both sides emit structurally headers-only traces.
func (d *tapDriver) drive(ctx context.Context, addr string) ([]byte, error) {
	c := &http.Client{Timeout: 5 * time.Second}
	var out bytes.Buffer
	send := func(xtap string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/tap", nil)
		if err != nil {
			return err
		}
		req.Header.Set("x-tap", xtap)
		req.Header.Set("x-backend-status", "204")
		resp, err := c.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		fmt.Fprintf(&out, "%s %d\n", xtap, resp.StatusCode)
		return nil
	}
	for i := 0; i < nMatch; i++ {
		if err := send("yes"); err != nil {
			return nil, err
		}
	}
	for i := 0; i < nNonMatch; i++ {
		if err := send("no"); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes (status line + headers + body) for the
// runner's standard admin-diff at the probe step (matches the
// 0097/0098-and-earlier precedent; compareAdminResponses parses these as a
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
// Called by the runner (not AssertSubject: 0099 is cross-side, so
// AssertSubject would never run -- see reference_differential_asserter_dispatch).
func (d *tapDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	// (1) trace counts. The reference writes at stream end but the container's
	// filesystem lands asynchronously from the host's view; poll it. The subject
	// writes in-process and is already done.
	refFiles := pollTraces(t, d.refDir, nMatch, 30*time.Second)
	subjFiles := pollTraces(t, d.subjDir, nMatch, 5*time.Second)
	if got := len(refFiles); got != nMatch {
		t.Errorf("reference trace count = %d, want %d (no trace for the %d non-matching streams)", got, nMatch, nNonMatch)
	}
	if got := len(subjFiles); got != nMatch {
		t.Errorf("subject trace count = %d, want %d (no trace for the %d non-matching streams)", got, nMatch, nNonMatch)
	}

	// (2) rq_tapped on both sides.
	const statName = "http." + statPrefix + ".tap.rq_tapped"
	if got := scrapeCounter(t, ctx, refAdminAddr, statName); got != nMatch {
		t.Errorf("reference %s = %d, want %d", statName, got, nMatch)
	}
	if got := scrapeCounter(t, ctx, subjAdminAddr, statName); got != nMatch {
		t.Errorf("subject %s = %d, want %d", statName, got, nMatch)
	}

	// (3)-(7) per-trace payload assertions on BOTH sides.
	wantReq := map[string]string{
		":method": "GET", ":path": "/tap", "x-tap": "yes", "x-backend-status": "204",
	}
	wantResp := map[string]string{":status": "204", "content-type": "text/plain"}
	assertSide(t, "reference", refFiles, wantReq, wantResp)
	assertSide(t, "subject", subjFiles, wantReq, wantResp)
}

func assertSide(t fixture.TB, side string, files []string, wantReq, wantResp map[string]string) {
	t.Helper()
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
			t.Fatalf("%s: %s: trace is not an http_buffered_trace", side, filepath.Base(path))
		}
		name := side + "/" + filepath.Base(path)

		// (5b) RAW BYTES: no "body" key at all. A decode cannot tell an OMITTED
		// field from an explicit `null` -- protojson unmarshals `"body": null`
		// straight back to a nil Body -- so a regression to EmitUnpopulated is
		// invisible to assertion (5) below. This is the only check that sees it.
		if bytes.Contains(b, []byte(`"body"`)) {
			t.Errorf("%s: raw trace must contain NO \"body\" key "+
				"(EmitDefaultValues omits nil message fields; EmitUnpopulated would render \"body\": null)", name)
		}

		// (3) request header subset. D-TAP-SUBSET: :authority, :scheme,
		// x-request-id, x-forwarded-proto, x-envoy-*, date, server, connection and
		// user-agent are UNasserted coverage boundaries.
		assertSubset(t, name+" request.headers", bt.GetRequest().GetHeaders(), wantReq)
		// (4) response header subset.
		assertSubset(t, name+" response.headers", bt.GetResponse().GetHeaders(), wantResp)

		// (5) bodies ABSENT -- the GET->204 shape makes this a POSITIVE assertion.
		if bt.GetRequest().GetBody() != nil {
			t.Errorf("%s: request.body must be ABSENT (bodyless GET); got %v", name, bt.GetRequest().GetBody())
		}
		if bt.GetResponse().GetBody() != nil {
			t.Errorf("%s: response.body must be ABSENT (204 No Content); got %v", name, bt.GetResponse().GetBody())
		}

		// (6) trailers == [] -- cross-side EXACT despite envoy-go never seeing a
		// trailer, because EmitDefaultValues renders empty repeated as [].
		if n := len(bt.GetRequest().GetTrailers()); n != 0 {
			t.Errorf("%s: request.trailers must be empty; got %d", name, n)
		}
		if n := len(bt.GetResponse().GetTrailers()); n != 0 {
			t.Errorf("%s: response.trailers must be empty; got %d", name, n)
		}

		// (7) downstream_connection ABSENT (both record_* flags unset).
		if bt.GetDownstreamConnection() != nil {
			t.Errorf("%s: downstream_connection must be ABSENT; got %v", name, bt.GetDownstreamConnection())
		}
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
