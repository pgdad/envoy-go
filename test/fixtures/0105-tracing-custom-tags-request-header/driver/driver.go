// Package driver registers the 0105-tracing-custom-tags-request-header fixture
// with the differential runner. Cloned from 0102-tracing-custom-tags-literal
// (itself cloned from 0087-tracing-otlp), it is the behavioral proof of the
// phase 62 request_header custom_tags source: cross-side EXACT (subject envoy-go
// vs reference Envoy contrib-v1.37.2 in Docker) that the request_header custom
// tag `trace_user` (configured in the HCM `tracing` block, sibling of
// `provider`, resolving from the inbound `x-trace-user` request header) appears
// as an OTLP span attribute — with the driver-sent header VALUE — on EVERY
// exported span, in addition to the 0087 baseline assertions (span count, span
// structure, deterministic attribute subset, W3C trace-id continuation
// invariant). The driver sends `x-trace-user: u-42` on every driven request (the
// present case); the `default_value: anon` and header-absent edges are covered
// by the deterministic internal/tracing/resolve_test.go unit tests, not this
// fixture.
//
// Integration shape (single-listener plaintext H1; driver-owned in-process OTLP
// TraceService gRPC receiver; HTTPFixedBody backend):
//
//  1. ReferenceBootstrap renders envoy.yaml with host.docker.internal (ADR-0010
//     STRICT_DNS) + the runner-allocated backend port + the driver-owned OTLP
//     TraceService receiver host:port (host=host.docker.internal for reference
//     Envoy). The receiver port is allocated at ReferenceBootstrap time and the
//     otlptrace.Server is bound on 0.0.0.0:<port> BEFORE the reference container
//     starts so the reference Envoy can dial it (host.docker.internal bridge alias)
//     AND the subject can dial it (127.0.0.1). SubjectConfig renders envoy-go.yaml
//     with the runner-allocated admin/listener/backend ports + the SAME OTLP port
//     (host=127.0.0.1).
//
//  2. DriveReference / DriveSubject each fire:
//     - N (numPlain) plain GET requests (fixed Host/UA, query-less path, no inbound
//     trace context). random_sampling 100% => each is a FRESH locally-sampled span.
//     - M (numCont) continuation requests carrying
//     "Traceparent: 00-<contTraceID>-<contParentID>-01". The proxy CONTINUES the
//     inbound trace — the exported span carries trace_id == contTraceID and
//     parent_span_id == contParentID.
//     Then POLL the receiver's Count() until >= numTotal (N+M) — the release barrier
//     (reference_concurrency_differential_release_barrier; the reference Envoy buffers
//     spans and flushes on a timer / buffer-fill; poll not sleep).
//     Each side's span and Resource.attributes snapshots are captured before Reset().
//
//  3. AssertStats asserts, on BOTH sides: exactly numTotal spans; for EVERY span the
//     structure (name "ingress", kind SPAN_KIND_SERVER, non-zero timing); the
//     deterministic attribute subset (see below); the service.name Resource attr; and
//     the continuation prong (M spans with trace_id == contTraceID). PLUS the
//     subject-side stats tracing.opentelemetry.spans_sent == numTotal and
//     tracing.opentelemetry.spans_dropped == 0.
//
// Decode-ran proof: Count() poll guarantees spans > 0 on both sides before
// asserting — a 0-span pass is structurally impossible.
//
// Framing NOT asserted: the Export-call count, per-call batch sizes, connection
// count, flush cadence — these legitimately vary side-to-side
// (reference_streaming_sink_differential_framing). The assertion is on the
// per-span PAYLOAD aggregated across all Export calls.
//
// SDK/scope NOT asserted: telemetry.sdk.* resource attrs and
// ScopeSpans.scope.name/version are impl-specific (envoy-go is not cpp).
//
// upstream_cluster / upstream_cluster.name FRAMEWORK GAP: envoy-go's Lua bridge
// UpstreamCluster() returns "" at the request-completion seam (not plumbed
// through). These attribute KEYS are present on both sides but their VALUES differ
// (reference emits the real cluster name "c_backend"; envoy-go emits ""). The
// VALUE is therefore UNasserted cross-side; only KEY presence is asserted on the
// subject side. Documented in README.md.
package driver

import (
	"bytes"
	"context"
	"encoding/hex"
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

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/otlptrace"
)

const (
	fixtureName = "0105-tracing-custom-tags-request-header"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0105 takes 10105 for the single plaintext listener.
	refAdminPort    = 9901
	refListenerPort = 10105

	// numPlain plain requests (no inbound trace context) — each a FRESH local
	// sample under random_sampling 100%.
	numPlain = 8
	// numCont continuation requests (inbound traceparent) — each continues the
	// inbound trace (trace_id == contTraceID, parent_span_id == contParentID).
	numCont = 4
	// numTotal is the per-side span count target: N + M = 12.
	numTotal = numPlain + numCont

	// The fixed request shape (kept identical cross-side; query-less path).
	probePath = "/trace"
	probeHost = "trace.example"
	probeUA   = "trace-probe/1"

	// The FIXED inbound W3C trace context for the continuation requests. NOT
	// all-zero (all-zero trace-id is treated as no-incoming-context and would
	// produce a fresh local sample). The proxy CONTINUES this trace, so the
	// exported span carries trace_id == contTraceID and parent_span_id == contParentID.
	contTraceID  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 32 hex → 16 bytes 0xaa
	contParentID = "bbbbbbbbbbbbbbbb"                 // 16 hex → 8 bytes  0xbb

	// service_name baked into both bootstraps → ResourceSpans.resource.attributes
	// service.name value.
	wantServiceName = "0105"

	// Subject span stats (flat /stats internal names).
	subjSpansSentStat    = "tracing.opentelemetry.spans_sent"
	subjSpansDroppedStat = "tracing.opentelemetry.spans_dropped"

	// phase 62: the request_header custom_tags entry baked into both bootstraps'
	// `tracing` block (sibling of `provider`) — the FIRST value of x-trace-user is
	// asserted as an OTLP span attribute, by key, on EVERY exported span (both sides).
	customTagKey    = "trace_user"
	traceUserHeader = "x-trace-user"
	traceUserValue  = "u-42" // the value the driver SENDS on x-trace-user (present case)

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL Count() to the expected total; never sleep-to-wait.
	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)

// contTraceIDBytes and contParentIDBytes are the decoded byte representations of
// the fixed inbound trace context, used for binary comparison against the
// exported span's TraceId / ParentSpanId fields.
var (
	contTraceIDBytes  = mustDecodeHex(contTraceID)
	contParentIDBytes = mustDecodeHex(contParentID)
)

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("driver: decode hex %q: %v", s, err))
	}
	return b
}

func init() {
	fixture.RegisterFixture(fixtureName, &traceOTLPDriver{})
}

// traceOTLPDriver carries the per-driver lifecycle state — the OTLP receiver port
// (constant across the ref+subj run; allocated lazily at bootstrap time), the
// running receiver handle, and the per-side span + Resource.attributes snapshots
// captured during Drive for the AssertStats cross-side assertion.
type traceOTLPDriver struct {
	mu sync.Mutex

	otlpPort int
	srv      *otlptrace.Server

	refSpans  []*tracepb.Span
	subjSpans []*tracepb.Span

	refResAttrs  [][]*commonpb.KeyValue
	subjResAttrs [][]*commonpb.KeyValue
}

// allocateOTLPPort reserves a free TCP port for the OTLP receiver via
// Listen+Close. Idempotent — returns the same port on subsequent calls. Does
// NOT start the server. Mirrors the 0084 allocateOTLPPort idiom.
func (d *traceOTLPDriver) allocateOTLPPort() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.otlpPort != 0 {
		return d.otlpPort
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate OTLP port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	d.otlpPort = port
	return port
}

// ensureServer starts the in-process TraceService receiver bound to
// 0.0.0.0:<otlpPort> (so BOTH the reference container via host.docker.internal
// AND the subject via 127.0.0.1 can dial it). Idempotent — a second call is a
// no-op while the server runs. Called at ReferenceBootstrap time so the receiver
// is live before either proxy starts its OTLP gRPC stream.
func (d *traceOTLPDriver) ensureServer() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.srv != nil {
		return
	}
	if d.otlpPort == 0 {
		panic("driver: ensureServer called before allocateOTLPPort")
	}
	addr := fmt.Sprintf("0.0.0.0:%d", d.otlpPort)
	srv, err := otlptrace.NewAtAddr(addr)
	if err != nil {
		panic(fmt.Sprintf("driver: start OTLP receiver on %s: %v", addr, err))
	}
	d.srv = srv
}

// --- fixture.Driver (required) ---

func (*traceOTLPDriver) BackendCount() int                { return 1 }
func (*traceOTLPDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*traceOTLPDriver) SubjectListenerName() string      { return "l_test" }
func (*traceOTLPDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// runner-allocated backend port + the driver-owned OTLP receiver host:port
// (host=host.docker.internal). It allocates the OTLP port and starts the
// receiver here so it is live before the reference container boots.
func (d *traceOTLPDriver) ReferenceBootstrap(backendPorts []int) string {
	otlpPort := d.allocateOTLPPort()
	d.ensureServer()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    refAdminPort,
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
		"OTLPHost":     "host.docker.internal",
		"OTLPPort":     otlpPort,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback) + the SAME OTLP port (host=127.0.0.1).
func (d *traceOTLPDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	otlpPort := d.allocateOTLPPort()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
		"OTLPPort":     otlpPort,
	})
}

// DriveReference fires the workload against the reference proxy and snapshots the
// reference-side spans + Resource.attributes.
func (d *traceOTLPDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	out, spans, resAttrs, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.refSpans = spans
	d.refResAttrs = resAttrs
	return out, nil
}

// DriveSubject fires the workload against the subject proxy and snapshots the
// subject-side spans + Resource.attributes. After the subject snapshot the
// receiver is hard-stopped SYNCHRONOUSLY via Close() (immediate grpc.Server.Stop
// — cancels still-open proxy OTLP streams and returns at once) for deterministic
// teardown; the spans are already snapshotted so canceling the streams loses nothing.
func (d *traceOTLPDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, spans, resAttrs, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.subjSpans = spans
	d.subjResAttrs = resAttrs
	d.mu.Lock()
	srv := d.srv
	d.srv = nil
	d.mu.Unlock()
	if srv != nil {
		srv.Close()
	}
	return out, nil
}

// driveSide resets the receiver accumulators, fires numPlain plain requests then
// numCont continuation requests against the proxy listener at addr, polls
// Count() to >= numTotal, and returns the per-request status byte stream plus
// snapshots of the received spans and per-ResourceSpans Resource.attributes.
// The Reset() gives clean per-side separation: the subject generates no spans
// until its own DriveSubject window, so post-Reset spans are exclusively this
// side's.
func (d *traceOTLPDriver) driveSide(ctx context.Context, addr string) ([]byte, []*tracepb.Span, [][]*commonpb.KeyValue, error) {
	d.mu.Lock()
	srv := d.srv
	d.mu.Unlock()
	if srv == nil {
		return nil, nil, nil, fmt.Errorf("driver: OTLP receiver not running")
	}
	// Reset() is safe despite the helper's "no Export in flight" contract: on the
	// subject side the reference proxy's OTLP stream is open but QUIESCENT —
	// DriveReference returned only after all numTotal reference spans had arrived
	// and been snapshotted, and no further data-plane traffic reaches the reference
	// in the subject window.
	srv.Reset()

	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	var b bytes.Buffer

	// N plain requests (no inbound trace context).
	for i := 0; i < numPlain; i++ {
		code, err := d.fireProbe(ctx, client, addr, nil)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("plain request %d: %w", i, err)
		}
		fmt.Fprintf(&b, "plain status=%d\n", code)
	}

	// M continuation requests (inbound Traceparent with the fixed trace-id).
	contTP := "00-" + contTraceID + "-" + contParentID + "-01"
	for i := 0; i < numCont; i++ {
		code, err := d.fireProbe(ctx, client, addr, map[string]string{"Traceparent": contTP})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("continuation request %d: %w", i, err)
		}
		fmt.Fprintf(&b, "cont status=%d\n", code)
	}

	if err := pollSpanCount(ctx, srv, numTotal); err != nil {
		return nil, nil, nil, err
	}
	return b.Bytes(), srv.Spans(), srv.ResourceAttributes(), nil
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent
// (plus any extra inbound headers) and returns the response status code (the body
// is drained and discarded).
func (d *traceOTLPDriver) fireProbe(ctx context.Context, client *http.Client, addr string, extra map[string]string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+probePath, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Host = probeHost
	req.Header.Set("User-Agent", probeUA)
	req.Header.Set(traceUserHeader, traceUserValue)
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

// pollSpanCount spins on srv.Count() reaching want (or the context / deadline
// elapsing). The reference Envoy buffers spans and flushes them on a timer /
// buffer-fill, so a fixed sleep would be both flaky and slow; the poll converges
// as soon as the side's spans arrive.
func pollSpanCount(ctx context.Context, srv *otlptrace.Server, want int) error {
	deadline := time.Now().Add(pollDeadline)
	for {
		if n := srv.Count(); n >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("OTLP receiver: timed out waiting for %d spans (got %d)", want, srv.Count())
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("OTLP receiver: context done waiting for %d spans (got %d): %w", want, srv.Count(), ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes for the standard admin-diff at the runner's probe step.
func (*traceOTLPDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertStats asserts, on BOTH sides: exactly numTotal spans; the stable
// per-span structure + attribute subset; the service.name Resource attr; the
// continuation prong (M spans with trace_id == contTraceID); PLUS the
// subject-side spans_sent == numTotal / spans_dropped == 0.
func (d *traceOTLPDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	dump := os.Getenv("FIXTURE_0105_DUMP") != ""

	if dump {
		fmt.Fprintf(os.Stderr, "=== 0105 ref spans=%d resAttrs=%d subj spans=%d resAttrs=%d ===\n",
			len(d.refSpans), len(d.refResAttrs), len(d.subjSpans), len(d.subjResAttrs))
		dumpSpans(os.Stderr, "ref", d.refSpans)
		dumpSpans(os.Stderr, "subj", d.subjSpans)
	}

	// Non-vacuous counts: both sides MUST have produced exactly numTotal spans
	// (a zero-span "pass" is vacuous — prove decode actually ran on BOTH sides).
	if len(d.refSpans) != numTotal {
		t.Fatalf("reference OTLP spans: got %d, want %d", len(d.refSpans), numTotal)
	}
	if len(d.subjSpans) != numTotal {
		t.Fatalf("subject OTLP spans: got %d, want %d", len(d.subjSpans), numTotal)
	}

	// Per-span structure + attribute assertions on BOTH sides.
	assertSpans(t, "reference", d.refSpans)
	assertSpans(t, "subject", d.subjSpans)

	// Continuation prong: M spans with trace_id == contTraceID and
	// parent_span_id == contParentID — cross-side EXACT.
	assertContinuationSpans(t, "reference", d.refSpans)
	assertContinuationSpans(t, "subject", d.subjSpans)

	// service.name Resource attr == wantServiceName on BOTH sides.
	assertServiceName(t, "reference", d.refResAttrs)
	assertServiceName(t, "subject", d.subjResAttrs)

	// phase 62: the request_header custom_tags entry (trace_user, resolved from
	// x-trace-user) is present as an OTLP span attribute, by key, on EVERY span —
	// BOTH sides.
	assertCustomTag(t, "reference", d.refSpans)
	assertCustomTag(t, "subject", d.subjSpans)

	// Subject-side span stats (the reference emits DIFFERENT tracing stat names —
	// subject-only, mirroring the 0084 and 0086 subject-specific stat assertion).
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

// assertSpans asserts the stable per-span structure + attribute subset on every
// span in the slice.
func assertSpans(t fixture.TB, side string, spans []*tracepb.Span) {
	t.Helper()
	for i, sp := range spans {
		// Name and kind
		if sp.GetName() != "ingress" {
			t.Fatalf("%s span %d: name = %q, want \"ingress\"", side, i, sp.GetName())
		}
		if sp.GetKind() != tracepb.Span_SPAN_KIND_SERVER {
			t.Fatalf("%s span %d: kind = %v, want SPAN_KIND_SERVER (2)", side, i, sp.GetKind())
		}

		// Non-zero timestamps and ordering.
		start := sp.GetStartTimeUnixNano()
		end := sp.GetEndTimeUnixNano()
		if start == 0 {
			t.Fatalf("%s span %d: StartTimeUnixNano == 0", side, i)
		}
		if end == 0 {
			t.Fatalf("%s span %d: EndTimeUnixNano == 0", side, i)
		}
		if start >= end {
			t.Fatalf("%s span %d: StartTimeUnixNano %d >= EndTimeUnixNano %d", side, i, start, end)
		}

		// Attribute subset.
		attrs := spanAttrMap(sp)
		assertAttrString(t, side, i, attrs, "http.method", "GET")
		assertAttrPresent(t, side, i, attrs, "http.url") // value varies by host/path encoding
		assertAttrPresent(t, side, i, attrs, "http.protocol")
		assertAttrPresent(t, side, i, attrs, "user_agent")
		// Cross-side stable string attrs with known fixed values:
		assertAttrString(t, side, i, attrs, "component", "proxy")
		assertAttrString(t, side, i, attrs, "downstream_cluster", "-")
		assertAttrString(t, side, i, attrs, "response_flags", "-")
		// Numeric-ish attrs: normalize to string for cross-side comparison
		// (the reference may emit STRING, envoy-go currently emits INT — see
		// CRITICAL section in the task spec and README for the resolution).
		assertAttrNormalized(t, side, i, attrs, "http.status_code", "200")
		assertAttrNormalized(t, side, i, attrs, "request_size", "0")
		// response_size is 17 bytes (HTTPFixedBody "backend:v1/fixed\n").
		assertAttrNormalized(t, side, i, attrs, "response_size", "17")
		// guid:x-request-id KEY must be present (value varies — UNasserted).
		assertAttrPresent(t, side, i, attrs, "guid:x-request-id")
		// upstream_cluster / upstream_cluster.name KEY present (value UNasserted —
		// framework gap: envoy-go emits "" while reference emits "c_backend").
		assertAttrPresent(t, side, i, attrs, "upstream_cluster")
		assertAttrPresent(t, side, i, attrs, "upstream_cluster.name")
	}
}

// assertContinuationSpans filters the spans by trace_id == contTraceIDBytes and
// asserts exactly numCont such spans exist, each with parent_span_id ==
// contParentIDBytes (the inbound parent span id from the Traceparent header).
func assertContinuationSpans(t fixture.TB, side string, spans []*tracepb.Span) {
	t.Helper()
	var contSpans []*tracepb.Span
	for _, sp := range spans {
		if bytes.Equal(sp.GetTraceId(), contTraceIDBytes) {
			contSpans = append(contSpans, sp)
		}
	}
	if len(contSpans) != numCont {
		t.Fatalf("%s continuation spans: got %d with trace_id=%s, want %d",
			side, len(contSpans), contTraceID, numCont)
	}
	for i, sp := range contSpans {
		if !bytes.Equal(sp.GetParentSpanId(), contParentIDBytes) {
			t.Fatalf("%s continuation span %d: parent_span_id = %x, want %s",
				side, i, sp.GetParentSpanId(), contParentID)
		}
	}
}

// assertServiceName asserts that every Resource.attributes snapshot carries
// service.name == wantServiceName. Both sides must have at least one snapshot.
func assertServiceName(t fixture.TB, side string, resAttrs [][]*commonpb.KeyValue) {
	t.Helper()
	if len(resAttrs) == 0 {
		t.Fatalf("%s: no Resource.attributes snapshots received", side)
	}
	for i, attrs := range resAttrs {
		byKey := make(map[string]*commonpb.AnyValue, len(attrs))
		for _, kv := range attrs {
			byKey[kv.GetKey()] = kv.GetValue()
		}
		v, ok := byKey["service.name"]
		if !ok {
			t.Fatalf("%s ResourceSpans %d: missing resource attr key \"service.name\" (got keys %v)",
				side, i, mapKeys(byKey))
		}
		if got := v.GetStringValue(); got != wantServiceName {
			t.Fatalf("%s ResourceSpans %d: service.name = %q, want %q", side, i, got, wantServiceName)
		}
	}
}

// assertCustomTag asserts the phase-62 request_header custom tag on EVERY span,
// cross-side by KEY (OTLP attribute order is non-deterministic — SPEC §11). The
// driver sends x-trace-user: <traceUserValue> on every request, so every span
// carries trace_user == traceUserValue (the present case). Errorf per property so
// one failure does not mask the rest.
func assertCustomTag(t fixture.TB, side string, spans []*tracepb.Span) {
	t.Helper()
	for i, sp := range spans {
		v, ok := spanAttrMap(sp)[customTagKey]
		if !ok {
			t.Errorf("%s span %d: missing custom tag key %q (present: %v)", side, i, customTagKey, mapKeys(spanAttrMap(sp)))
			continue
		}
		if got := v.GetStringValue(); got != traceUserValue {
			t.Errorf("%s span %d: %s = %q, want %q", side, i, customTagKey, got, traceUserValue)
		}
	}
}

// spanAttrMap builds a map of attr key → *AnyValue from a span's Attributes slice.
func spanAttrMap(sp *tracepb.Span) map[string]*commonpb.AnyValue {
	m := make(map[string]*commonpb.AnyValue, len(sp.GetAttributes()))
	for _, kv := range sp.GetAttributes() {
		m[kv.GetKey()] = kv.GetValue()
	}
	return m
}

// assertAttrPresent fails if key is absent from the attr map.
func assertAttrPresent(t fixture.TB, side string, spanIdx int, attrs map[string]*commonpb.AnyValue, key string) {
	t.Helper()
	if _, ok := attrs[key]; !ok {
		t.Fatalf("%s span %d: missing attr key %q (present keys: %v)",
			side, spanIdx, key, mapKeys(attrs))
	}
}

// assertAttrString fails if key is absent or its string value != want.
// Only applies to STRING-typed AnyValue — if the value is INT, fails with a
// type mismatch (use assertAttrNormalized for int/string attrs).
func assertAttrString(t fixture.TB, side string, spanIdx int, attrs map[string]*commonpb.AnyValue, key, want string) {
	t.Helper()
	v, ok := attrs[key]
	if !ok {
		t.Fatalf("%s span %d: missing attr key %q", side, spanIdx, key)
		return
	}
	got := v.GetStringValue()
	if got != want {
		t.Fatalf("%s span %d: attr %q = %q, want %q (type=%T)", side, spanIdx, key, got, want, v.GetValue())
	}
}

// assertAttrNormalized asserts that the normalized string representation of the
// attr value (int or string) matches want. Handles both INT (AnyValue_IntValue)
// and STRING (AnyValue_StringValue) types — the reference Envoy cpp impl may emit
// these as STRING while envoy-go currently emits INT; normalized comparison makes
// the assertion type-agnostic (CRITICAL — attr value-TYPE parity).
func assertAttrNormalized(t fixture.TB, side string, spanIdx int, attrs map[string]*commonpb.AnyValue, key, want string) {
	t.Helper()
	v, ok := attrs[key]
	if !ok {
		t.Fatalf("%s span %d: missing attr key %q", side, spanIdx, key)
		return
	}
	var got string
	switch vt := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		got = vt.StringValue
	case *commonpb.AnyValue_IntValue:
		got = strconv.FormatInt(vt.IntValue, 10)
	default:
		t.Fatalf("%s span %d: attr %q has unexpected value type %T", side, spanIdx, key, v.GetValue())
		return
	}
	if got != want {
		t.Fatalf("%s span %d: attr %q = %q, want %q", side, spanIdx, key, got, want)
	}
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// dumpSpans writes a debug summary of all span attrs to w (FIXTURE_0105_DUMP).
func dumpSpans(w io.Writer, side string, spans []*tracepb.Span) {
	for i, sp := range spans {
		_, _ = fmt.Fprintf(w, "%s span[%d] name=%q kind=%v trace=%x parent=%x\n",
			side, i, sp.GetName(), sp.GetKind(), sp.GetTraceId(), sp.GetParentSpanId())
		for _, kv := range sp.GetAttributes() {
			switch vt := kv.GetValue().GetValue().(type) {
			case *commonpb.AnyValue_StringValue:
				_, _ = fmt.Fprintf(w, "  %s attr %q = STRING(%q)\n", side, kv.GetKey(), vt.StringValue)
			case *commonpb.AnyValue_IntValue:
				_, _ = fmt.Fprintf(w, "  %s attr %q = INT(%d)\n", side, kv.GetKey(), vt.IntValue)
			default:
				_, _ = fmt.Fprintf(w, "  %s attr %q = OTHER(%T)\n", side, kv.GetKey(), kv.GetValue().GetValue())
			}
		}
	}
}

// scrapeFlatStats issues GET /stats against adminAddr and returns the flat
// "name: value" lines parsed into a map. Mirrors the 0084 + 0086 scraper pattern.
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

// --- file / template helpers (the 0084 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0105-tracing-custom-tags-request-header/driver/driver.go
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
	_ fixture.Driver           = (*traceOTLPDriver)(nil)
	_ fixture.BackendKindAware = (*traceOTLPDriver)(nil)
	_ fixture.StatsAsserter    = (*traceOTLPDriver)(nil)
)
