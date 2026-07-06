// Package driver registers the 0088-tracing-zipkin fixture with the differential
// runner. It is the behavioral proof of the phase 46.2 Zipkin v2-JSON span export
// + B3 propagation: cross-side EXACT (subject envoy-go vs reference Envoy
// contrib-v1.37.2 in Docker) on the per-span PAYLOAD aggregated across all POSTed
// spans — the span count, the span structure (name, kind, timing), the
// deterministic tag subset, and the B3 trace-id continuation invariant.
//
// Integration shape (single-listener plaintext H1; driver-owned in-process HTTP
// Zipkin v2 collector; HTTPFixedBody backend):
//
//  1. ReferenceBootstrap renders envoy.yaml with host.docker.internal (ADR-0010
//     STRICT_DNS) + the runner-allocated backend port + the driver-owned Zipkin
//     collector host:port (host=host.docker.internal for reference Envoy). The
//     collector port is allocated at ReferenceBootstrap time and the
//     zipkincollector is bound on 0.0.0.0:<port> BEFORE the reference container
//     starts so the reference Envoy can dial it (host.docker.internal bridge
//     alias) AND the subject can dial it (127.0.0.1). SubjectConfig renders
//     envoy-go.yaml with the runner-allocated admin/listener/backend ports + the
//     SAME Zipkin port (host=127.0.0.1).
//
//  2. DriveReference / DriveSubject each fire:
//     - N (numPlain) plain GET requests (fixed Host/UA, query-less path, no
//     inbound trace context). random_sampling 100% => each is a FRESH
//     locally-sampled span with a random trace-id.
//     - M (numCont) continuation requests carrying a single "b3" header
//     "<contTraceID>-<contSpanID>-1" (the 3-field B3 form). The proxy CONTINUES
//     the inbound trace — under shared_span_context:false the exported span
//     carries traceId == contTraceID and parentId == contSpanID (the incoming
//     span-id becomes the server span's parent).
//     Then POLL the collector's Count() until >= numTotal (N+M) — the release
//     barrier (reference_concurrency_differential_release_barrier; the reference
//     Envoy buffers spans and flushes on a timer / buffer-fill; poll not sleep).
//     Each side's span snapshot is captured before Reset().
//
//  3. AssertStats asserts, on BOTH sides: exactly numTotal spans; for EVERY span
//     the structure (name == probeHost authority, kind "SERVER", non-zero
//     timestamp/duration); the deterministic tag subset (see below); and the
//     continuation prong (exactly M spans with traceId == contTraceID, each with
//     parentId == contSpanID; the other N carry random trace-ids). PLUS the
//     subject-side stats tracing.zipkin.spans_sent == numTotal and
//     tracing.zipkin.spans_dropped == 0.
//
// Decode-ran proof: the Count() poll guarantees spans > 0 on both sides before
// asserting — a 0-span pass is structurally impossible.
//
// Framing NOT asserted: the POST count, per-POST batch sizes, connection count,
// flush cadence — these legitimately vary side-to-side
// (reference_streaming_sink_differential_framing). The assertion is on the
// per-span PAYLOAD aggregated across all POSTs.
//
// shared_span_context:false CHOICE: the fixture pins shared_span_context:false
// (NOT the Envoy default true) so the continuation prong asserts parentId ==
// contSpanID directly — more discriminating than the shared:true form (which
// would reuse the incoming span-id as the server's own id + set shared=true).
// Documented in README.md.
//
// upstream_cluster / upstream_cluster.name / peer.address FRAMEWORK GAP: envoy-go
// emits upstream_cluster = "" (not plumbed at the request-completion seam) while
// the reference emits the real cluster name; peer.address is env-specific. These
// tag KEYS are present on both sides but their VALUES differ — the VALUE is
// UNasserted cross-side; only KEY presence is asserted. Documented in README.md.
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
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/zipkincollector"
)

const (
	fixtureName = "0088-tracing-zipkin"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0088 takes 10088 for the single plaintext listener (one above 0087).
	refAdminPort    = 9901
	refListenerPort = 10088

	// numPlain plain requests (no inbound trace context) — each a FRESH local
	// sample under random_sampling 100% with a random trace-id.
	numPlain = 8
	// numCont continuation requests (inbound b3) — each continues the inbound
	// trace (traceId == contTraceID, parentId == contSpanID under shared:false).
	numCont = 4
	// numTotal is the per-side span count target: N + M = 12.
	numTotal = numPlain + numCont

	// The fixed request shape (kept identical cross-side; query-less path).
	probePath = "/trace"
	probeHost = "trace.example"
	probeUA   = "trace-probe/1"

	// The FIXED inbound B3 trace context for the continuation requests. The b3
	// header is the 3-field form "<traceID>-<spanID>-<sampled>": the span-id field
	// is the upstream caller's span, which becomes our server span's PARENT under
	// shared_span_context:false. contTraceID is 128-bit (32 hex) to match
	// trace_id_128bit:true (the emitted traceId stays 32 hex). NOT all-zero (an
	// all-zero id is treated as no-incoming-context => a fresh local sample).
	contTraceID = "0123456789abcdef0123456789abcdef" // 32 hex → 128-bit trace-id
	contSpanID  = "fedcba9876543210"                 // 16 hex → 64-bit span-id

	// Subject span stats (flat /stats internal names).
	subjSpansSentStat    = "tracing.zipkin.spans_sent"
	subjSpansDroppedStat = "tracing.zipkin.spans_dropped"

	// traceIDHexLen is the emitted traceId width under trace_id_128bit:true (32
	// lowercase hex chars). spanIDHexLen is the parentId/id width (16 hex).
	traceIDHexLen = 32

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL Count() to the expected total; never sleep-to-wait.
	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)

func init() {
	fixture.RegisterFixture(fixtureName, &traceZipkinDriver{})
}

// traceZipkinDriver carries the per-driver lifecycle state — the Zipkin collector
// port (constant across the ref+subj run; allocated lazily at bootstrap time), the
// running collector handle, and the per-side span snapshots captured during Drive
// for the AssertStats cross-side assertion.
type traceZipkinDriver struct {
	mu sync.Mutex

	zipkinPort int
	col        *zipkincollector.Collector

	refSpans  []zipkincollector.ReceivedSpan
	subjSpans []zipkincollector.ReceivedSpan
}

// allocateZipkinPort reserves a free TCP port for the Zipkin collector via
// Listen+Close. Idempotent — returns the same port on subsequent calls. Does
// NOT start the collector. Mirrors the 0087 allocateOTLPPort idiom.
func (d *traceZipkinDriver) allocateZipkinPort() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.zipkinPort != 0 {
		return d.zipkinPort
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate Zipkin port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	d.zipkinPort = port
	return port
}

// ensureCollector starts the in-process HTTP Zipkin collector bound to
// 0.0.0.0:<zipkinPort> (so BOTH the reference container via host.docker.internal
// AND the subject via 127.0.0.1 can dial it). Idempotent — a second call is a
// no-op while the collector runs. Called at ReferenceBootstrap time so the
// collector is live before either proxy POSTs its first span.
func (d *traceZipkinDriver) ensureCollector() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.col != nil {
		return
	}
	if d.zipkinPort == 0 {
		panic("driver: ensureCollector called before allocateZipkinPort")
	}
	addr := fmt.Sprintf("0.0.0.0:%d", d.zipkinPort)
	col, err := zipkincollector.NewAtAddr(addr)
	if err != nil {
		panic(fmt.Sprintf("driver: start Zipkin collector on %s: %v", addr, err))
	}
	d.col = col
}

// --- fixture.Driver (required) ---

func (*traceZipkinDriver) BackendCount() int                { return 1 }
func (*traceZipkinDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*traceZipkinDriver) SubjectListenerName() string      { return "l_test" }
func (*traceZipkinDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// runner-allocated backend port + the driver-owned Zipkin collector host:port
// (host=host.docker.internal). It allocates the Zipkin port and starts the
// collector here so it is live before the reference container boots.
func (d *traceZipkinDriver) ReferenceBootstrap(backendPorts []int) string {
	zipkinPort := d.allocateZipkinPort()
	d.ensureCollector()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    refAdminPort,
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
		"ZipkinHost":   "host.docker.internal",
		"ZipkinPort":   zipkinPort,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback) + the SAME Zipkin port (host=127.0.0.1).
func (d *traceZipkinDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	zipkinPort := d.allocateZipkinPort()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
		"ZipkinPort":   zipkinPort,
	})
}

// DriveReference fires the workload against the reference proxy and snapshots the
// reference-side spans.
func (d *traceZipkinDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	out, spans, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.refSpans = spans
	return out, nil
}

// DriveSubject fires the workload against the subject proxy and snapshots the
// subject-side spans. After the subject snapshot the collector is hard-stopped
// SYNCHRONOUSLY via Stop() for deterministic teardown; the spans are already
// snapshotted so stopping the server loses nothing.
func (d *traceZipkinDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, spans, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.subjSpans = spans
	d.mu.Lock()
	col := d.col
	d.col = nil
	d.mu.Unlock()
	if col != nil {
		col.Stop()
	}
	return out, nil
}

// driveSide resets the collector accumulator, fires numPlain plain requests then
// numCont continuation requests against the proxy listener at addr, polls Count()
// to >= numTotal, and returns the per-request status byte stream plus a snapshot
// of the received spans. The Reset() gives clean per-side separation: the subject
// generates no spans until its own DriveSubject window, so post-Reset spans are
// exclusively this side's.
func (d *traceZipkinDriver) driveSide(ctx context.Context, addr string) ([]byte, []zipkincollector.ReceivedSpan, error) {
	d.mu.Lock()
	col := d.col
	d.mu.Unlock()
	if col == nil {
		return nil, nil, fmt.Errorf("driver: Zipkin collector not running")
	}
	// Reset() is safe despite the helper's "no POST in flight" contract: on the
	// subject side the reference proxy is QUIESCENT — DriveReference returned only
	// after all numTotal reference spans had arrived and been snapshotted, and no
	// further data-plane traffic reaches the reference in the subject window.
	col.Reset()

	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	var b bytes.Buffer

	// N plain requests (no inbound trace context).
	for i := 0; i < numPlain; i++ {
		code, err := d.fireProbe(ctx, client, addr, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("plain request %d: %w", i, err)
		}
		fmt.Fprintf(&b, "plain status=%d\n", code)
	}

	// M continuation requests (inbound b3 with the fixed 3-field trace context).
	b3 := contTraceID + "-" + contSpanID + "-1"
	for i := 0; i < numCont; i++ {
		code, err := d.fireProbe(ctx, client, addr, map[string]string{"b3": b3})
		if err != nil {
			return nil, nil, fmt.Errorf("continuation request %d: %w", i, err)
		}
		fmt.Fprintf(&b, "cont status=%d\n", code)
	}

	if err := pollSpanCount(ctx, col, numTotal); err != nil {
		return nil, nil, err
	}
	return b.Bytes(), col.Spans(), nil
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent
// (plus any extra inbound headers) and returns the response status code (the body
// is drained and discarded).
func (d *traceZipkinDriver) fireProbe(ctx context.Context, client *http.Client, addr string, extra map[string]string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+probePath, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Host = probeHost
	req.Header.Set("User-Agent", probeUA)
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// pollSpanCount spins on col.Count() reaching want (or the context / deadline
// elapsing). The reference Envoy buffers spans and flushes them on a timer /
// buffer-fill, so a fixed sleep would be both flaky and slow; the poll converges
// as soon as the side's spans arrive.
func pollSpanCount(ctx context.Context, col *zipkincollector.Collector, want int) error {
	deadline := time.Now().Add(pollDeadline)
	for {
		if n := col.Count(); n >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Zipkin collector: timed out waiting for %d spans (got %d)", want, col.Count())
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Zipkin collector: context done waiting for %d spans (got %d): %w", want, col.Count(), ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes for the standard admin-diff at the runner's probe step.
func (*traceZipkinDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertStats asserts, on BOTH sides: exactly numTotal spans; the stable per-span
// structure + tag subset; the continuation prong (M spans with traceId ==
// contTraceID / parentId == contSpanID, N with random trace-ids); PLUS the
// subject-side spans_sent == numTotal / spans_dropped == 0.
func (d *traceZipkinDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	dump := os.Getenv("FIXTURE_0088_DUMP") != ""
	if dump {
		fmt.Fprintf(os.Stderr, "=== 0088 ref spans=%d subj spans=%d ===\n", len(d.refSpans), len(d.subjSpans))
		dumpSpans(os.Stderr, "ref", d.refSpans)
		dumpSpans(os.Stderr, "subj", d.subjSpans)
	}

	// Non-vacuous counts: both sides MUST have produced exactly numTotal spans
	// (a zero-span "pass" is vacuous — prove decode actually ran on BOTH sides).
	if len(d.refSpans) != numTotal {
		t.Fatalf("reference Zipkin spans: got %d, want %d", len(d.refSpans), numTotal)
	}
	if len(d.subjSpans) != numTotal {
		t.Fatalf("subject Zipkin spans: got %d, want %d", len(d.subjSpans), numTotal)
	}

	// Per-span structure + tag assertions on BOTH sides.
	assertSpans(t, "reference", d.refSpans)
	assertSpans(t, "subject", d.subjSpans)

	// Continuation prong: exactly M spans with traceId == contTraceID and
	// parentId == contSpanID; the other N carry random trace-ids — cross-side EXACT.
	assertContinuationSpans(t, "reference", d.refSpans)
	assertContinuationSpans(t, "subject", d.subjSpans)

	// Subject-side span stats (the reference emits DIFFERENT tracing stat names —
	// subject-only, mirroring the 0087 subject-specific stat assertion).
	subjStats, err := scrapeFlatStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subject /stats: %v", err)
	}
	if got := subjStats[subjSpansSentStat]; got != numTotal {
		t.Fatalf("subject %s: got %d, want %d", subjSpansSentStat, got, numTotal)
	}
	if got := subjStats[subjSpansDroppedStat]; got != 0 {
		t.Fatalf("subject %s: got %d, want 0", subjSpansDroppedStat, got)
	}
}

// assertSpans asserts the stable per-span structure + tag subset on every span.
func assertSpans(t fixture.TB, side string, spans []zipkincollector.ReceivedSpan) {
	t.Helper()
	for i, sp := range spans {
		// Name == the request :authority (Zipkin names the span after the Host;
		// D-TRACE-ZIPKIN-SPAN-NAME — distinct from the OTLP "ingress").
		if sp.Name != probeHost {
			t.Fatalf("%s span %d: name = %q, want %q", side, i, sp.Name, probeHost)
		}
		if sp.Kind != "SERVER" {
			t.Fatalf("%s span %d: kind = %q, want \"SERVER\"", side, i, sp.Kind)
		}

		// Non-zero timestamp + duration (PRESENCE, not value).
		if sp.Timestamp <= 0 {
			t.Fatalf("%s span %d: timestamp = %d, want > 0", side, i, sp.Timestamp)
		}
		if sp.Duration <= 0 {
			t.Fatalf("%s span %d: duration = %d, want > 0", side, i, sp.Duration)
		}

		// traceId width per trace_id_128bit:true (32 lowercase hex chars).
		if len(sp.TraceID) != traceIDHexLen {
			t.Fatalf("%s span %d: traceId = %q (len %d), want len %d (trace_id_128bit)",
				side, i, sp.TraceID, len(sp.TraceID), traceIDHexLen)
		}

		// Deterministic VALUE-assertable tags.
		tags := sp.Tags
		assertTagValue(t, side, i, tags, "http.method", "GET")
		assertTagValue(t, side, i, tags, "http.status_code", "200")
		assertTagValue(t, side, i, tags, "component", "proxy")
		assertTagValue(t, side, i, tags, "downstream_cluster", "-")
		assertTagValue(t, side, i, tags, "response_flags", "-")
		assertTagValue(t, side, i, tags, "request_size", "0")
		assertTagValue(t, side, i, tags, "response_size", "17") // HTTPFixedBody 17-byte body
		assertTagValue(t, side, i, tags, "user_agent", probeUA)

		// PRESENT-only tags (value varies by host/path encoding; UNasserted exact).
		assertTagPresent(t, side, i, tags, "http.url")
		assertTagPresent(t, side, i, tags, "http.protocol")

		// KEY-PRESENT-only tags (value UNasserted):
		//   upstream_cluster / upstream_cluster.name — EMPTY on the subject
		//   (framework gap: envoy-go emits "" while the reference emits the cluster
		//   name); peer.address — env-specific; guid:x-request-id — value varies.
		assertTagPresent(t, side, i, tags, "upstream_cluster")
		assertTagPresent(t, side, i, tags, "upstream_cluster.name")
		assertTagPresent(t, side, i, tags, "peer.address")
		assertTagPresent(t, side, i, tags, "guid:x-request-id")
	}
}

// assertContinuationSpans filters the spans by traceId == contTraceID and asserts
// exactly numCont such spans exist, each with parentId == contSpanID (the inbound
// span-id from the b3 header becomes the server span's parent under
// shared_span_context:false). The remaining numPlain spans MUST carry a different
// (random) trace-id — proving the continuation/fresh discrimination is live.
func assertContinuationSpans(t fixture.TB, side string, spans []zipkincollector.ReceivedSpan) {
	t.Helper()
	var cont, fresh int
	for i, sp := range spans {
		if sp.TraceID == contTraceID {
			cont++
			if sp.ParentID != contSpanID {
				t.Fatalf("%s continuation span %d: parentId = %q, want %q",
					side, i, sp.ParentID, contSpanID)
			}
		} else {
			fresh++
		}
	}
	if cont != numCont {
		t.Fatalf("%s continuation spans: got %d with traceId=%s, want %d",
			side, cont, contTraceID, numCont)
	}
	if fresh != numPlain {
		t.Fatalf("%s fresh spans: got %d with a non-continuation traceId, want %d",
			side, fresh, numPlain)
	}
}

// assertTagPresent fails if key is absent from the tag map.
func assertTagPresent(t fixture.TB, side string, spanIdx int, tags map[string]string, key string) {
	t.Helper()
	if _, ok := tags[key]; !ok {
		t.Fatalf("%s span %d: missing tag key %q (present keys: %v)",
			side, spanIdx, key, mapKeys(tags))
	}
}

// assertTagValue fails if key is absent or its value != want.
func assertTagValue(t fixture.TB, side string, spanIdx int, tags map[string]string, key, want string) {
	t.Helper()
	got, ok := tags[key]
	if !ok {
		t.Fatalf("%s span %d: missing tag key %q (present keys: %v)",
			side, spanIdx, key, mapKeys(tags))
		return
	}
	if got != want {
		t.Fatalf("%s span %d: tag %q = %q, want %q", side, spanIdx, key, got, want)
	}
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// dumpSpans writes a debug summary of all spans to w (FIXTURE_0088_DUMP).
func dumpSpans(w io.Writer, side string, spans []zipkincollector.ReceivedSpan) {
	for i, sp := range spans {
		_, _ = fmt.Fprintf(w, "%s span[%d] name=%q kind=%q trace=%s id=%s parent=%s shared=%v ts=%d dur=%d\n",
			side, i, sp.Name, sp.Kind, sp.TraceID, sp.ID, sp.ParentID, sp.Shared, sp.Timestamp, sp.Duration)
		for k, v := range sp.Tags {
			_, _ = fmt.Fprintf(w, "  %s tag %q = %q\n", side, k, v)
		}
	}
}

// scrapeFlatStats issues GET /stats against adminAddr and returns the flat
// "name: value" lines parsed into a map. Mirrors the 0087 scraper pattern.
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

// --- file / template helpers (the 0087 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0088-tracing-zipkin/driver/driver.go
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
	_ fixture.Driver           = (*traceZipkinDriver)(nil)
	_ fixture.BackendKindAware = (*traceZipkinDriver)(nil)
	_ fixture.StatsAsserter    = (*traceZipkinDriver)(nil)
)
