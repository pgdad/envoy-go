// Package driver registers the 0083-grpc-access-log-headers fixture with the
// differential runner. It is the behavioral proof of the phase 44.3 gRPC ALS
// header-capture feature: cross-side EXACT (subject envoy-go vs reference Envoy
// v1.37.2 in Docker) on the deterministic structured-field subset of every
// streamed HTTPAccessLogEntry PLUS the captured request.request_headers /
// response.response_headers maps.
//
// Header capture (44.3): the OUTER HttpGrpcAccessLogConfig carries
// additional_request_headers_to_log ["x-req-foo", "x-req-missing", "x-req-multi"]
// and additional_response_headers_to_log ["content-type"]. Each probe sets
// X-Req-Foo: bar + a two-valued X-Req-Multi (m1, m2) and never sets
// X-Req-Missing (the configured-but-absent OMIT proof). The backend
// (HTTPFixedBody) emits Content-Type: text/plain, surfaced verbatim in the
// downstream response and captured into response.response_headers
// (AMEND-HDR-4 — a backend-origin header; server/date/x-envoy-* are avoided as
// they vary cross-side). Every entry on BOTH sides MUST carry:
//
//	request.request_headers  == {"x-req-foo":"bar", "x-req-multi":"m1,m2"}
//	                            (x-req-missing key ABSENT — the omit proof)
//	response.response_headers == {"content-type":"text/plain"}
//
// Integration shape (single-listener fixture; plaintext HTTP/1.1 downstream +
// plaintext h2c ALS cluster per D-ALS-RECEIVER — no TLS):
//
//  1. ReferenceBootstrap renders envoy.yaml with host.docker.internal (ADR-0010
//     STRICT_DNS) + the runner-allocated HTTPFixedBody backend port + the
//     driver-owned in-process ALS receiver host:port (host=host.docker.internal
//     for reference Envoy). The ALS receiver port is allocated lazily at
//     ReferenceBootstrap time and the accessloggrpc.Server is bound on
//     0.0.0.0:<port> BEFORE the reference container starts so the reference
//     Envoy can dial it (host.docker.internal bridge alias) AND the subject can
//     dial it (127.0.0.1). SubjectConfig renders envoy-go.yaml with the
//     runner-allocated admin/listener/backend ports + the SAME ALS port
//     (host=127.0.0.1).
//
//  2. DriveReference / DriveSubject each fire N (numRequests) identical
//     query-less GET /health requests (User-Agent: als-probe/1, Host:
//     als.example) against the proxy under test, then POLL the receiver's
//     Count() until >= N (poll-to-converge, never sleep — the reference Envoy
//     buffers ALS entries and flushes them on a ~1s timer). Each side's entry
//     set is snapshotted into the driver before the next side runs (Reset()
//     between sides gives clean per-side separation — the subject generates no
//     access-log entries until its own DriveSubject window). The returned byte
//     stream is the per-request status sequence; the runner's CompareBytes pass
//     asserts the data-plane behaved equivalently cross-side.
//
//  3. AssertStats asserts, for EVERY received entry on BOTH sides, the
//     deterministic 7-field subset (request_method=GET, path=/health,
//     authority=als.example, user_agent=als-probe/1, response_code=200,
//     response_body_bytes=17, protocol_version=HTTP11), aggregated across all
//     entries (AMEND-ALS-3/4 — the per-entry PAYLOAD is asserted, NOT
//     stream/message/batch framing which legitimately varies side-to-side). It
//     ALSO asserts the SUBJECT-side stat access_logs.grpc_access_log.logs_written
//     == N (scraped from the subject's flat /stats admin endpoint).
//
// UNasserted: common_properties.* (start_time/duration/upstream_remote_address —
// non-deterministic), identifier.node (D-ALS-NODE), framing, and every
// subject-absent reference field (request.scheme, request_id, upstream_cluster,
// access_log_type, response_code_details, wire-byte counts).
//
// Query-less path (AMEND-ALS-2): the data-plane request MUST use a query-less
// path. envoy-go's Record.Path is path-only while the reference's request.path
// carries the query string — a query-bearing request would DIVERGE cross-side.
// This is a documented faithful constraint.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	dataaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/accessloggrpc"
)

const (
	fixtureName = "0083-grpc-access-log-headers"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0083 takes 10083 for the single plaintext listener.
	refAdminPort    = 9901
	refListenerPort = 10083

	// numRequests is the per-side data-plane workload — N identical query-less
	// requests, each producing exactly one streamed HTTPAccessLogEntry.
	numRequests = 8

	// The fixed request shape asserted cross-side on every received entry.
	probePath = "/health"     // query-less (AMEND-ALS-2).
	probeHost = "als.example" // → request.authority.
	probeUA   = "als-probe/1" // → request.user_agent.

	// Captured response header (44.3): the HTTPFixedBody backend sets
	// Content-Type: text/plain, surfaced verbatim downstream and captured into
	// response.response_headers under the lowercase key. Backend-origin per
	// AMEND-HDR-4 (avoid server/date/x-envoy-* — they vary cross-side).
	respContentTypeKey = "content-type"
	respContentTypeVal = "text/plain"

	// HTTPFixedBody backend serves a byte-identical 17-byte body
	// ("backend:v1/fixed\n") regardless of path → response.response_body_bytes.
	fixedBodyBytes = 17

	// Subject sink stat (flat /stats internal name) — successful sends.
	subjLogsWrittenStat = "access_logs.grpc_access_log.logs_written"

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL Count() to the expected total; never sleep-to-wait.
	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)

func init() {
	fixture.RegisterFixture(fixtureName, &alsDriver{})
}

// alsDriver carries the per-driver lifecycle state — the ALS receiver port
// (constant across the ref+subj run; allocated lazily at bootstrap time), the
// running receiver handle, and the per-side entry snapshots captured during
// Drive for the AssertStats cross-side payload assertion.
type alsDriver struct {
	mu sync.Mutex

	alsPort int
	srv     *accessloggrpc.Server

	refEntries  []*dataaccesslogv3.HTTPAccessLogEntry
	subjEntries []*dataaccesslogv3.HTTPAccessLogEntry
}

// allocateALSPort reserves a free TCP port for the ALS receiver via
// Listen+Close. Idempotent — returns the same port on subsequent calls. Does
// NOT start the server. Mirrors the 0021 allocateAuthPort idiom.
func (d *alsDriver) allocateALSPort() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.alsPort != 0 {
		return d.alsPort
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate ALS port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	d.alsPort = port
	return port
}

// ensureServer starts the in-process AccessLogService receiver bound to
// 0.0.0.0:<alsPort> (so BOTH the reference container via host.docker.internal
// AND the subject via 127.0.0.1 can dial it). Idempotent — a second call is a
// no-op while the server runs. Called at ReferenceBootstrap time so the
// receiver is live before either proxy starts its ALS gRPC stream.
func (d *alsDriver) ensureServer() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.srv != nil {
		return
	}
	if d.alsPort == 0 {
		panic("driver: ensureServer called before allocateALSPort")
	}
	addr := fmt.Sprintf("0.0.0.0:%d", d.alsPort)
	srv, err := accessloggrpc.NewAtAddr(addr)
	if err != nil {
		panic(fmt.Sprintf("driver: start ALS receiver on %s: %v", addr, err))
	}
	d.srv = srv
}

// --- fixture.Driver (required) ---

func (*alsDriver) BackendCount() int                { return 1 }
func (*alsDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*alsDriver) SubjectListenerName() string      { return "l_test" }
func (*alsDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// runner-allocated backend port + the driver-owned ALS receiver host:port
// (host=host.docker.internal). It allocates the ALS port and starts the
// receiver here so it is live before the reference container boots.
func (d *alsDriver) ReferenceBootstrap(backendPorts []int) string {
	alsPort := d.allocateALSPort()
	d.ensureServer()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    refAdminPort,
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
		"ALSHost":      "host.docker.internal",
		"ALSPort":      alsPort,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback) + the SAME ALS port (host=127.0.0.1).
func (d *alsDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	alsPort := d.allocateALSPort()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
		"ALSHost":      "127.0.0.1",
		"ALSPort":      alsPort,
	})
}

// DriveReference fires N requests against the reference proxy and snapshots the
// reference-side ALS entries.
func (d *alsDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	out, entries, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.refEntries = entries
	return out, nil
}

// DriveSubject fires N requests against the subject proxy and snapshots the
// subject-side ALS entries. After the subject snapshot the receiver is hard-
// stopped SYNCHRONOUSLY via Close() (immediate grpc.Server.Stop — cancels the
// still-open proxy ALS streams and returns at once) for deterministic teardown;
// the entries are already snapshotted so canceling the streams loses nothing.
func (d *alsDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, entries, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.subjEntries = entries
	d.mu.Lock()
	srv := d.srv
	d.srv = nil
	d.mu.Unlock()
	if srv != nil {
		srv.Close()
	}
	return out, nil
}

// driveSide resets the receiver accumulator, fires N identical query-less
// requests against the proxy listener at addr, polls Count() to >= N, and
// returns the per-request status byte stream plus a snapshot of the received
// entries. The Reset() gives clean per-side separation: the subject generates
// no access-log entries until its own DriveSubject window, so post-Reset entries
// are exclusively this side's.
func (d *alsDriver) driveSide(ctx context.Context, addr string) ([]byte, []*dataaccesslogv3.HTTPAccessLogEntry, error) {
	d.mu.Lock()
	srv := d.srv
	d.mu.Unlock()
	if srv == nil {
		return nil, nil, fmt.Errorf("driver: ALS receiver not running")
	}
	// Reset() is safe despite the helper's "no stream in flight" contract: on
	// the subject side the reference proxy's ALS stream is open but QUIESCENT —
	// DriveReference returned only after all N reference entries had arrived and
	// been snapshotted, and no further data-plane traffic reaches the reference
	// in the subject window, so no stray reference entry can survive this Reset.
	srv.Reset()

	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	var b bytes.Buffer
	for i := 0; i < numRequests; i++ {
		code, err := d.fireProbe(ctx, client, addr)
		if err != nil {
			return nil, nil, fmt.Errorf("request %d: %w", i, err)
		}
		fmt.Fprintf(&b, "status=%d\n", code)
	}

	if err := pollCount(ctx, srv, numRequests); err != nil {
		return nil, nil, err
	}
	return b.Bytes(), srv.Entries(), nil
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent
// and returns the response status code (the body is drained and discarded).
func (d *alsDriver) fireProbe(ctx context.Context, client *http.Client, addr string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+probePath, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Host = probeHost
	req.Header.Set("User-Agent", probeUA)
	// Header-capture drive (44.3): a single-valued x-req-foo + a two-valued
	// x-req-multi (m1, m2 → captured as the comma-joined "m1,m2"). x-req-missing
	// is configured-but-NOT-set: the absent-OMIT proof (its key must NOT appear
	// in the captured request map on EITHER side).
	req.Header.Set("X-Req-Foo", "bar")
	req.Header.Add("X-Req-Multi", "m1")
	req.Header.Add("X-Req-Multi", "m2")
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// pollCount spins on srv.Count() reaching want (or the context / deadline
// elapsing). The reference Envoy buffers ALS entries and flushes them on a
// ~1s timer, so a fixed sleep would be both flaky and slow; the poll converges
// as soon as the side's entries arrive.
func pollCount(ctx context.Context, srv *accessloggrpc.Server, want int) error {
	deadline := time.Now().Add(pollDeadline)
	for {
		if n := srv.Count(); n >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("ALS receiver: timed out waiting for %d entries (got %d)", want, srv.Count())
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("ALS receiver: context done waiting for %d entries (got %d): %w", want, srv.Count(), ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes for the standard admin-diff at the runner's probe step.
func (*alsDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.StatsAsserter ---

// AssertStats asserts the deterministic 7-field subset AND the two captured
// header maps (request.request_headers + response.response_headers) on EVERY
// received entry, on BOTH sides (cross-side EXACT, aggregated per AMEND-ALS-3/4),
// plus the subject-side logs_written stat == numRequests.
func (d *alsDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	if os.Getenv("FIXTURE_0083_DUMP") != "" {
		fmt.Fprintf(os.Stderr, "=== 0083 ref entries=%d subj entries=%d ===\n", len(d.refEntries), len(d.subjEntries))
		dumpHeaderMaps("ref", d.refEntries)
		dumpHeaderMaps("subj", d.subjEntries)
	}

	// Both sides MUST have produced exactly numRequests entries (a zero-entry
	// "pass" is vacuous — prove decode actually ran on BOTH sides).
	if len(d.refEntries) != numRequests {
		t.Fatalf("reference ALS entries: got %d, want %d", len(d.refEntries), numRequests)
	}
	if len(d.subjEntries) != numRequests {
		t.Fatalf("subject ALS entries: got %d, want %d", len(d.subjEntries), numRequests)
	}

	assertEntries(t, "reference", d.refEntries)
	assertEntries(t, "subject", d.subjEntries)

	// Subject-side stat: every send succeeded.
	subjStats, err := scrapeFlatStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subject /stats: %v", err)
	}
	if got := subjStats[subjLogsWrittenStat]; got != numRequests {
		t.Fatalf("subject %s: got %d, want %d", subjLogsWrittenStat, got, numRequests)
	}
}

// assertEntries asserts the 7-field deterministic subset PLUS the two captured
// header maps (request.request_headers + response.response_headers, with
// x-req-missing OMITTED) on every entry.
func assertEntries(t fixture.TB, side string, entries []*dataaccesslogv3.HTTPAccessLogEntry) {
	t.Helper()
	for i, e := range entries {
		req := e.GetRequest()
		resp := e.GetResponse()
		if m := req.GetRequestMethod(); m != corev3.RequestMethod_GET {
			t.Fatalf("%s entry %d: request.request_method = %v, want GET", side, i, m)
		}
		if p := req.GetPath(); p != probePath {
			t.Fatalf("%s entry %d: request.path = %q, want %q", side, i, p, probePath)
		}
		if a := req.GetAuthority(); a != probeHost {
			t.Fatalf("%s entry %d: request.authority = %q, want %q", side, i, a, probeHost)
		}
		if ua := req.GetUserAgent(); ua != probeUA {
			t.Fatalf("%s entry %d: request.user_agent = %q, want %q", side, i, ua, probeUA)
		}
		if c := resp.GetResponseCode().GetValue(); c != 200 {
			t.Fatalf("%s entry %d: response.response_code = %d, want 200", side, i, c)
		}
		if bb := resp.GetResponseBodyBytes(); bb != fixedBodyBytes {
			t.Fatalf("%s entry %d: response.response_body_bytes = %d, want %d", side, i, bb, fixedBodyBytes)
		}
		if pv := e.GetProtocolVersion(); pv != dataaccesslogv3.HTTPAccessLogEntry_HTTP11 {
			t.Fatalf("%s entry %d: protocol_version = %v, want HTTP11", side, i, pv)
		}

		// --- 44.3 header-capture maps (cross-side EXACT) ---
		// request.request_headers: x-req-foo single value + x-req-multi
		// comma-joined; x-req-missing key ABSENT (configured-but-unset OMIT
		// proof). Lowercase keys (AMEND-HDR-1), verbatim values.
		reqHdrs := req.GetRequestHeaders()
		if !reflect.DeepEqual(reqHdrs, wantReqHeaders) {
			t.Fatalf("%s entry %d: request.request_headers = %v, want %v", side, i, reqHdrs, wantReqHeaders)
		}
		if _, ok := reqHdrs["x-req-missing"]; ok {
			t.Fatalf("%s entry %d: request.request_headers contains x-req-missing (must be OMITTED): %v", side, i, reqHdrs)
		}
		// response.response_headers: the backend-origin content-type (AMEND-HDR-4).
		respHdrs := resp.GetResponseHeaders()
		if !reflect.DeepEqual(respHdrs, wantRespHeaders) {
			t.Fatalf("%s entry %d: response.response_headers = %v, want %v", side, i, respHdrs, wantRespHeaders)
		}
	}
}

// wantReqHeaders / wantRespHeaders are the expected captured maps asserted on
// every entry, both sides. x-req-missing is intentionally absent (the OMIT
// proof). x-req-multi is the comma-joined two-value capture (AMEND-HDR-3).
var (
	wantReqHeaders  = map[string]string{"x-req-foo": "bar", "x-req-multi": "m1,m2"}
	wantRespHeaders = map[string]string{respContentTypeKey: respContentTypeVal}
)

// dumpHeaderMaps prints the captured per-entry request/response header maps for
// FIXTURE_0083_DUMP — the non-vacuity proof (the maps are non-empty so the
// DeepEqual assertions are live, not comparing two empty maps).
func dumpHeaderMaps(side string, entries []*dataaccesslogv3.HTTPAccessLogEntry) {
	for i, e := range entries {
		fmt.Fprintf(os.Stderr, "  %s entry %d: req=%v resp=%v\n",
			side, i, e.GetRequest().GetRequestHeaders(), e.GetResponse().GetResponseHeaders())
	}
}

// scrapeFlatStats issues GET /stats against adminAddr and returns the flat
// "name: value" lines parsed into a map. Used for the subject-side
// logs_written assertion (the access_logs. prefix has no Prometheus
// tag-extractor arm — the flat surface is the canonical internal-name lookup,
// mirroring the 0055 redis StatsAsserter).
func scrapeFlatStats(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s body: %w", url, err)
	}
	out := make(map[string]uint64)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, ": ")
		if idx < 0 {
			continue
		}
		name := line[:idx]
		v, err := strconv.ParseUint(strings.TrimSpace(line[idx+2:]), 10, 64)
		if err != nil {
			continue
		}
		out[name] = v
	}
	return out, nil
}

// --- file / template helpers (the 0021 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0083-grpc-access-log-headers/driver/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

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

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*alsDriver)(nil)
	_ fixture.BackendKindAware = (*alsDriver)(nil)
	_ fixture.StatsAsserter    = (*alsDriver)(nil)
)
