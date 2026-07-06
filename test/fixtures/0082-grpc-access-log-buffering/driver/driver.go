// Package driver registers the 0082-grpc-access-log-buffering fixture with the
// differential runner. It is the behavioral proof of the phase 44.2 gRPC ALS
// BUFFERING extension on top of the 44.1 core streaming sink: cross-side EXACT
// (subject envoy-go vs reference Envoy v1.37.2 in Docker) on the deterministic
// structured-field subset of every streamed HTTPAccessLogEntry, PLUS a
// SUBJECT-side proof that the buffer coalesced >= 2 entries into at least one
// StreamAccessLogsMessage (D-BUF-DIFFERENTIAL-DRIVE / AMEND-BUF-3).
//
// This fixture is 0081 + the two common_config buffer fields:
//   - buffer_size_bytes: 1048576 (1 MiB) — DORMANT. N small entries never reach
//     the byte cap, so the SIZE trigger never fires (the byte-accounting-fragile
//     size-cap path is deliberately AVOIDED cross-side; SPEC §8.1).
//   - buffer_flush_interval: 1s — the deterministic TIMER flush lever.
//
// Drive shape (D-BUF-DIFFERENTIAL-DRIVE): N=16 requests are fired CONCURRENTLY
// (a sync.WaitGroup fan-out against the same listener addr) so the N records
// queue into envoy-go's single process-global buffer FASTER than the 1s timer
// elapses; the next tick then flushes >= 2 as one batch. The concurrent burst +
// the wide 1s interval is the coalescence guarantee (SPEC §8.1).
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
//     query-less GET /health requests CONCURRENTLY (User-Agent: als-probe/1,
//     Host: als.example) against the proxy under test, then POLL the receiver's
//     Count() until >= N (poll-to-converge, never sleep). Each side's entry set
//     AND per-message batch sizes are snapshotted into the driver before the
//     next side runs (Reset() between sides clears entries AND batchSizes).
//
//  3. AssertStats asserts, for EVERY received entry on BOTH sides, the
//     deterministic 7-field subset (request_method=GET, path=/health,
//     authority=als.example, user_agent=als-probe/1, response_code=200,
//     response_body_bytes=17, protocol_version=HTTP11), aggregated across all
//     entries (AMEND-ALS-3/4 — the per-entry PAYLOAD is asserted, NOT
//     stream/message/batch framing which legitimately varies side-to-side). It
//     ALSO asserts the SUBJECT-side stat access_logs.grpc_access_log.logs_written
//     == N (scraped from the subject's flat /stats admin endpoint) AND the
//     SUBJECT-side maxBatchSize >= 2 (the buffering coalescence proof).
//
// Cross-side batch COUNTS are infeasible (AMEND-BUF-3): the reference buffers
// PER-WORKER-THREAD (un-pinned worker count) while envoy-go uses one
// process-global buffer. So the maxBatchSize >= 2 proof is SUBJECT-side ONLY;
// the reference's per-worker batching is its own un-pinned business.
//
// UNasserted: common_properties.* (start_time/duration/upstream_remote_address —
// non-deterministic), identifier.node (D-ALS-NODE), all framing/batch counts on
// the reference side, and every subject-absent reference field (request.scheme,
// request_id, upstream_cluster, access_log_type, response_code_details,
// wire-byte counts).
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
	"runtime"
	"sort"
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
	fixtureName = "0082-grpc-access-log-buffering"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0082 takes 10082 for the single plaintext listener.
	refAdminPort    = 9901
	refListenerPort = 10082

	// numRequests is the per-side data-plane workload — N identical query-less
	// requests, each producing exactly one streamed HTTPAccessLogEntry. Bumped to
	// 16 (vs 0081's 8) and fired CONCURRENTLY so >= 2 entries land in one 1s
	// flush interval and coalesce into a single StreamAccessLogsMessage
	// (D-BUF-DIFFERENTIAL-DRIVE).
	numRequests = 16

	// The fixed request shape asserted cross-side on every received entry.
	probePath = "/health"     // query-less (AMEND-ALS-2).
	probeHost = "als.example" // → request.authority.
	probeUA   = "als-probe/1" // → request.user_agent.

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
// running receiver handle, and the per-side entry + batch-size snapshots
// captured during Drive for the AssertStats cross-side payload assertion and the
// subject-side maxBatchSize >= 2 buffering proof.
type alsDriver struct {
	mu sync.Mutex

	alsPort int
	srv     *accessloggrpc.Server

	refEntries  []*dataaccesslogv3.HTTPAccessLogEntry
	subjEntries []*dataaccesslogv3.HTTPAccessLogEntry

	refBatchSizes  []int
	subjBatchSizes []int
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
// reference-side ALS entries + per-message batch sizes.
func (d *alsDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	out, entries, batches, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.refEntries = entries
	d.refBatchSizes = batches
	return out, nil
}

// DriveSubject fires N requests against the subject proxy and snapshots the
// subject-side ALS entries + per-message batch sizes. After the subject snapshot
// the receiver is hard-stopped SYNCHRONOUSLY via Close() (immediate
// grpc.Server.Stop — cancels the still-open proxy ALS streams and returns at
// once) for deterministic teardown; the entries are already snapshotted so
// canceling the streams loses nothing.
func (d *alsDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, entries, batches, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.subjEntries = entries
	d.subjBatchSizes = batches
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
// requests CONCURRENTLY against the proxy listener at addr, polls Count() to
// >= N, and returns the per-request status byte stream plus a snapshot of the
// received entries AND per-message batch sizes. The concurrent fan-out is the
// coalescence guarantee: the N records queue into the subject's single buffer
// within the 1s flush interval, so the next tick flushes >= 2 as one batch
// (D-BUF-DIFFERENTIAL-DRIVE). The Reset() gives clean per-side separation: the
// subject generates no access-log entries until its own DriveSubject window, so
// post-Reset entries (and batchSizes) are exclusively this side's.
func (d *alsDriver) driveSide(ctx context.Context, addr string) ([]byte, []*dataaccesslogv3.HTTPAccessLogEntry, []int, error) {
	d.mu.Lock()
	srv := d.srv
	d.mu.Unlock()
	if srv == nil {
		return nil, nil, nil, fmt.Errorf("driver: ALS receiver not running")
	}
	// Reset() is safe despite the helper's "no stream in flight" contract: on
	// the subject side the reference proxy's ALS stream is open but QUIESCENT —
	// DriveReference returned only after all N reference entries had arrived and
	// been snapshotted, and no further data-plane traffic reaches the reference
	// in the subject window, so no stray reference entry can survive this Reset.
	srv.Reset()

	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	// Concurrent fan-out (D-BUF-DIFFERENTIAL-DRIVE): fire all N requests at once
	// so the records queue into the single buffer within one flush interval.
	statuses := make([]int, numRequests)
	errs := make([]error, numRequests)
	var wg sync.WaitGroup
	wg.Add(numRequests)
	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			defer wg.Done()
			code, err := d.fireProbe(ctx, client, addr)
			statuses[idx] = code
			errs[idx] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, nil, nil, fmt.Errorf("request %d: %w", i, err)
		}
	}

	// Deterministic per-request byte stream: sort the collected statuses so the
	// cross-side CompareBytes only depends on the multiset (N×200), not the
	// goroutine completion order.
	sort.Ints(statuses)
	var b bytes.Buffer
	for _, code := range statuses {
		fmt.Fprintf(&b, "status=%d\n", code)
	}

	if err := pollCount(ctx, srv, numRequests); err != nil {
		return nil, nil, nil, err
	}
	return b.Bytes(), srv.Entries(), srv.BatchSizes(), nil
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

// AssertStats asserts the deterministic 7-field subset on EVERY received entry,
// on BOTH sides (cross-side EXACT, aggregated per AMEND-ALS-3/4), plus the
// subject-side logs_written stat == numRequests AND the subject-side
// maxBatchSize >= 2 buffering coalescence proof.
func (d *alsDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	if os.Getenv("FIXTURE_0082_DUMP") != "" {
		fmt.Fprintf(os.Stderr,
			"=== 0082 ref entries=%d subj entries=%d refBatchSizes=%v subjBatchSizes=%v ===\n",
			len(d.refEntries), len(d.subjEntries), d.refBatchSizes, d.subjBatchSizes)
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

	// SUBJECT-side batching proof (D-BUF-DIFFERENTIAL-DRIVE / AMEND-BUF-3): the
	// buffered sink coalesced >= 2 entries into at least one StreamAccessLogsMessage.
	// This BITES against a regression to the 44.1 one-entry-per-message fixed flush
	// (which would make every batch size 1). SUBJECT-side ONLY — the reference's
	// per-worker batching is its own un-pinned business (cross-side batch counts
	// are infeasible).
	maxSubj := 0
	for _, n := range d.subjBatchSizes {
		if n > maxSubj {
			maxSubj = n
		}
	}
	if maxSubj < 2 {
		t.Fatalf("subject max batch size = %d, want >= 2 (buffering did not coalesce; subjBatchSizes=%v)", maxSubj, d.subjBatchSizes)
	}
}

// assertEntries asserts the 7-field deterministic subset on every entry.
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
	// thisFile is .../test/fixtures/0082-grpc-access-log-buffering/driver/driver.go
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
