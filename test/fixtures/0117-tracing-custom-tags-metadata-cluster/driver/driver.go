// Package driver registers the 0117-tracing-custom-tags-metadata-cluster
// fixture with the differential runner. Cloned from
// 0116-tracing-custom-tags-metadata-host (the OTLP tracing custom_tags HOST
// metadata chassis; itself cloned from 0115/0114/0106/0105/0102/0087-tracing-otlp)
// with the static metadata block RELOCATED from the cluster's lb_endpoints[0]
// onto the CLUSTER itself — and with the single backend cluster SPLIT IN TWO,
// it is the behavioral proof of the phase 73 CLUSTER `metadata` custom_tags
// source: cross-side EXACT (subject envoy-go vs reference Envoy
// contrib-v1.37.2 in Docker) that a `metadata`-kind custom tag with
// `kind: {cluster: {}}` resolves a value out of the OWNING CLUSTER's STATIC
// clusters[].metadata.filter_metadata and emits it as an OTLP span attribute on
// EVERY exported span.
//
// ⚠️ TWO CLUSTERS, NOT ONE — and that is the whole point. 0116 shipped with a
// limitation it had to confess: with a single cluster it could not discriminate
// "the SELECTED source's metadata" from "the ONLY source's". This fixture
// removes that limitation. c_backend_a and c_backend_b point at the SAME single
// HTTPFixedBody backend host:port and differ ONLY in their CLUSTER-level
// metadata value (v-cluster-a-0117 vs v-cluster-b-0117). Two path-prefix routes
// ("/a" -> c_backend_a, "/b" -> c_backend_b) select between them
// DETERMINISTICALLY BY PATH, so no load-balancer spread enters the assertion
// (reference_round_robin_offset_randomized never engages) and BackendCount
// stays 1. ONE cl_hit tag on ONE HCM therefore emits TWO DIFFERENT values
// depending on which cluster was selected — the selected-not-only proof.
//
// As in 0116 there is NO writer: both sides parse the SAME
// clusters[].metadata.filter_metadata block from their (byte-identical)
// bootstrap YAML at cluster build time (the buildCluster populate loop in
// internal/cluster/manager.go retains the owning cluster's RAW per-namespace
// static metadata on cluster.Endpoint.clusterFilterMetadata; read back via
// (cluster.Endpoint).ClusterMetaLookup threaded as the 6th ResolveCustomTags
// argument from the SELECTED endpoint at
// internal/filter/hcm/accesslog_emit.go:57/:118/:179), so the resolved
// span-attribute VALUE is byte-identical on both sides without any runtime
// mutation. Note the gate is the PICKED HOST, not the matched route
// (reference_cluster_tag_gated_on_pick_not_route) — every probe here drives a
// SUCCESSFUL upstream request, so a host is always picked.
//
// Two custom_tags exercise the two resolution outcomes in one config:
//
//   - cl_hit     — metadata_key {key: envoy.test, path:[cl_k]} → resolves to
//     the OWNING cluster's static value: "v-cluster-a-0117" on a /a span and
//     "v-cluster-b-0117" on a /b span. The configured default_value
//     ("unused-default-0117") is NEVER used. This per-path value-equality
//     assertion is BOTH the CLUSTER-METADATA-SERVED proof and the
//     SELECTED-cluster proof.
//   - cl_default — metadata_key {key: envoy.test, path:[absent_k]} → the path
//     is UNSET on BOTH clusters → the default_value "fallback-0117" is emitted
//     (the absent-path default rule).
//
// ⚠️ PER-PATH SPAN PARTITIONING. The assertion is "each span carries ITS OWN
// cluster's value", so every span must be attributable to the path that
// produced it. `upstream_cluster` CANNOT serve as the discriminator — 0116
// asserts it assertAttrPresent ONLY because envoy-go emits it EMPTY
// (reference_tracing_upstream_cluster_framework_gap). The partition is
// therefore taken from the `http.url` span attribute, confirmed path-bearing on
// BOTH sides by a live per-side dump before any assertion was written to depend
// on it (see spanProbePath / partitionByPath below, and PROGRESS.md for the
// recorded dump evidence). Set FIXTURE_0117_DUMP=1 to re-run that dump.
//
// ⚠️ The namespace is "envoy.test", NOT "envoy.lb" — envoy.lb is not a
// privileged namespace (reference_envoy_lb_namespace_not_privileged), and the
// phase-38 envoy.lb scalar projection could not serve it anyway. This fixture
// is the cross-side proof that a CLUSTER-kind tag addresses an ARBITRARY
// filter_metadata namespace.
//
// ⚠️ STRING VALUES ONLY (reference_structpb_tag_cross_side_string_only). A
// struct-valued or numeric metadata value is NOT cross-side comparable: the
// reference serializes multi-key structs in an ARBITRARY key order while Go
// always sorts, scalar-vs-nested numbers use DIFFERENT reference renderers, and
// top-level scalar numbers render at ~6 significant digits on the reference
// (biting from 1000000 → 1e+06). (Envoy's YAML loader also coerces any
// non-integer scalar to a string.)
//
// Integration shape (single-listener plaintext H1; driver-owned in-process
// OTLP TraceService gRPC receiver; ONE HTTPFixedBody backend) is identical to
// 0116:
//
//  1. ReferenceBootstrap renders envoy.yaml with host.docker.internal (ADR-0010
//     STRICT_DNS) + the runner-allocated backend port (used by BOTH backend
//     clusters) + the driver-owned OTLP TraceService receiver host:port. The
//     receiver is bound on 0.0.0.0:<port> BEFORE the reference container starts
//     (host.docker.internal bridge alias) AND the subject can dial it
//     (127.0.0.1). SubjectConfig renders envoy-go.yaml with the
//     runner-allocated ports + the SAME OTLP port.
//
//  2. DriveReference / DriveSubject each fire the workload SPLIT ACROSS THE TWO
//     PATHS — numPlainPerPath plain GETs and numContPerPath continuation
//     requests (inbound Traceparent) against EACH of /a and /b — then POLL the
//     receiver's Count() until >= numTotal (the release barrier). Each side's
//     spans + Resource attributes are snapshotted before Reset().
//
//  3. AssertStats asserts, on BOTH sides: exactly numTotal spans; per-span
//     structure + the deterministic attribute subset; the service.name Resource
//     attr; the continuation prong; the PER-PATH CLUSTER metadata custom tags
//     (cl_hit == that path's OWN cluster value, cl_default == the
//     default_value); PLUS the subject-side tracing.opentelemetry.spans_sent /
//     spans_dropped stats.
//
// Decode-ran proof: Count() poll guarantees spans > 0 on both sides before
// asserting, and the partition sizes are Fatalf preconditions, so an
// empty-slice silent pass is structurally impossible.
//
// Framing / SDK / scope NOT asserted (the 0115/0116 precedent).
// upstream_cluster is the same documented framework gap.
package driver

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log"
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
	fixtureName = "0117-tracing-custom-tags-metadata-cluster"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0117 takes 10117 for the single plaintext listener.
	refAdminPort    = 9901
	refListenerPort = 10117

	// numPlainPerPath plain requests PER PATH (no inbound trace context) — each
	// a FRESH local sample under random_sampling 100%.
	numPlainPerPath = 4
	// numContPerPath continuation requests PER PATH (inbound traceparent) — each
	// continues the inbound trace (trace_id == contTraceID, parent_span_id ==
	// contParentID).
	numContPerPath = 2
	// numPerPath is the per-PATH span count: 4 + 2 = 6.
	numPerPath = numPlainPerPath + numContPerPath
	// numCont is the per-side continuation span count across both paths: 2+2 = 4.
	numCont = 2 * numContPerPath
	// numTotal is the per-side span count target: 2 * 6 = 12. Held at 12 (0116's
	// value) deliberately so the landed span-count and spans_sent assertions stay
	// consistent (reference_fixture_workload_constant_desync).
	numTotal = 2 * numPerPath

	// The fixed request shape (kept identical cross-side; query-less paths). The
	// two paths select the two clusters via prefix routes.
	probePathA = "/a"
	probePathB = "/b"
	probeHost  = "trace.example"
	probeUA    = "trace-probe/1"

	// The FIXED inbound W3C trace context for the continuation requests. NOT
	// all-zero (all-zero trace-id is treated as no-incoming-context and would
	// produce a fresh local sample). The proxy CONTINUES this trace, so the
	// exported span carries trace_id == contTraceID and parent_span_id == contParentID.
	contTraceID  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 32 hex → 16 bytes 0xaa
	contParentID = "bbbbbbbbbbbbbbbb"                 // 16 hex → 8 bytes  0xbb

	// service_name baked into both bootstraps → ResourceSpans.resource.attributes
	// service.name value.
	wantServiceName = "0117"

	// Subject span stats (flat /stats internal names).
	subjSpansSentStat    = "tracing.opentelemetry.spans_sent"
	subjSpansDroppedStat = "tracing.opentelemetry.spans_dropped"

	// phase 73: the two CLUSTER metadata custom_tags baked into both bootstraps'
	// `tracing` block. The OWNING cluster's STATIC
	// clusters[].metadata.filter_metadata block (namespace "envoy.test", key
	// "cl_k") is byte-identical on BOTH sides — NO writer, so cl_hit resolves
	// cross-side EXACT by VALUE.
	//
	//   clusterHitTagKey resolves the envoy.test/cl_k path → the SELECTED
	//   cluster's own value: clusterAValue on a probePathA span and
	//   clusterBValue on a probePathB span. The configured default is NEVER used
	//   — the cluster-metadata-served proof, the SELECTED-not-ONLY proof, and
	//   simultaneously the proof that a NON-"envoy.lb" namespace resolves on
	//   BOTH sides.
	//   clusterDefaultTagKey points at an UNSET path (absent_k) on BOTH clusters
	//   → the tag emits clusterDefaultFallback (the default_value; absent-path
	//   default rule).
	clusterHitTagKey       = "cl_hit"
	clusterDefaultTagKey   = "cl_default"
	clusterAValue          = "v-cluster-a-0117"
	clusterBValue          = "v-cluster-b-0117"
	clusterDefaultFallback = "fallback-0117"

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
// NOT start the server.
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
// AND the subject via 127.0.0.1 can dial it). Idempotent. Called at
// ReferenceBootstrap time so the receiver is live before either proxy starts.
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

// BackendCount stays 1: the TWO clusters point at the SAME single backend
// host:port, which is precisely what keeps the cluster metadata the ONLY
// difference between them. (The runner rejects 0 —
// reference_differential_backendcount_min_one.)
func (*traceOTLPDriver) BackendCount() int                { return 1 }
func (*traceOTLPDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*traceOTLPDriver) SubjectListenerName() string      { return "l_test" }
func (*traceOTLPDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// runner-allocated backend port (shared by BOTH backend clusters) + the
// driver-owned OTLP receiver host:port (host=host.docker.internal). It allocates
// the OTLP port and starts the receiver here so it is live before the reference
// container boots.
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
// + backend port (loopback, shared by BOTH backend clusters) + the SAME OTLP
// port (host=127.0.0.1).
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
// subject-side spans + Resource.attributes, then hard-stops the receiver
// synchronously (spans already snapshotted).
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

// driveSide resets the receiver accumulators, fires the workload SPLIT ACROSS
// THE TWO PATHS (numPlainPerPath plain + numContPerPath continuation requests
// against each of probePathA and probePathB), polls Count() to >= numTotal, and
// returns the per-request status byte stream plus snapshots of the received
// spans and per-ResourceSpans Resource.attributes.
//
// The byte stream records the PATH per line, so the cross-side CompareBytes
// itself pins that both sides drove the same per-path workload in the same
// order.
func (d *traceOTLPDriver) driveSide(ctx context.Context, addr string) ([]byte, []*tracepb.Span, [][]*commonpb.KeyValue, error) {
	d.mu.Lock()
	srv := d.srv
	d.mu.Unlock()
	if srv == nil {
		return nil, nil, nil, fmt.Errorf("driver: OTLP receiver not running")
	}
	srv.Reset()

	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	var b bytes.Buffer
	contTP := "00-" + contTraceID + "-" + contParentID + "-01"

	for _, path := range []string{probePathA, probePathB} {
		// numPlainPerPath plain requests (no inbound trace context).
		for i := 0; i < numPlainPerPath; i++ {
			code, err := d.fireProbe(ctx, client, addr, path, nil)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("plain request %d (%s): %w", i, path, err)
			}
			fmt.Fprintf(&b, "plain path=%s status=%d\n", path, code)
		}
		// numContPerPath continuation requests (inbound Traceparent with the
		// fixed trace-id).
		for i := 0; i < numContPerPath; i++ {
			code, err := d.fireProbe(ctx, client, addr, path, map[string]string{"Traceparent": contTP})
			if err != nil {
				return nil, nil, nil, fmt.Errorf("continuation request %d (%s): %w", i, path, err)
			}
			fmt.Fprintf(&b, "cont path=%s status=%d\n", path, code)
		}
	}

	if err := pollSpanCount(ctx, srv, numTotal); err != nil {
		return nil, nil, nil, err
	}
	return b.Bytes(), srv.Spans(), srv.ResourceAttributes(), nil
}

// fireProbe issues one query-less GET at path with the fixed Host + User-Agent
// (plus any extra inbound headers) and returns the response status code.
func (d *traceOTLPDriver) fireProbe(ctx context.Context, client *http.Client, addr, path string, extra map[string]string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil)
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

// pollSpanCount spins on srv.Count() reaching want (or the context / deadline
// elapsing).
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

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
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
// continuation prong; the PER-PATH CLUSTER metadata custom tags (cl_hit ==
// that path's OWN cluster value / cl_default == the default_value); PLUS the
// subject-side spans_sent == numTotal / spans_dropped == 0.
func (d *traceOTLPDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	dump := os.Getenv("FIXTURE_0117_DUMP") != ""

	// Always RECORD the per-side partition sizes (fixture.TB has no Logf).
	logPartitionSummary("reference", d.refSpans)
	logPartitionSummary("subject", d.subjSpans)

	if dump {
		fmt.Fprintf(os.Stderr, "=== 0117 ref spans=%d resAttrs=%d subj spans=%d resAttrs=%d ===\n",
			len(d.refSpans), len(d.refResAttrs), len(d.subjSpans), len(d.subjResAttrs))
		dumpSpans(os.Stderr, "ref", d.refSpans)
		dumpSpans(os.Stderr, "subj", d.subjSpans)
		// The PARTITION RECONNAISSANCE dump: the raw http.url per span plus the
		// path this driver recovers from it. This is the evidence the per-path
		// partition mechanism rests on.
		dumpPartition(os.Stderr, "ref", d.refSpans)
		dumpPartition(os.Stderr, "subj", d.subjSpans)
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

	// Continuation prong: numCont spans with trace_id == contTraceID and
	// parent_span_id == contParentID — cross-side EXACT.
	assertContinuationSpans(t, "reference", d.refSpans)
	assertContinuationSpans(t, "subject", d.subjSpans)

	// service.name Resource attr == wantServiceName on BOTH sides.
	assertServiceName(t, "reference", d.refResAttrs)
	assertServiceName(t, "subject", d.subjResAttrs)

	// phase 73: the two CLUSTER metadata custom tags, asserted PER PATH —
	// cl_hit resolves the OWNING cluster's static metadata value (cross-side
	// EXACT by VALUE, and DIFFERENT between the two paths: the
	// cluster-metadata-served proof, the SELECTED-not-ONLY proof, the NO-writer
	// proof, and the proof that a NON-"envoy.lb" namespace resolves on BOTH
	// sides), cl_default falls to its default_value. Both sides.
	assertClusterTags(t, "reference", d.refSpans)
	assertClusterTags(t, "subject", d.subjSpans)

	// Subject-side span stats (the reference emits DIFFERENT tracing stat names).
	subjStats, err := scrapeFlatStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subject /stats: %v", err)
	}
	if got := subjStats[subjSpansSentStat]; got != numTotal {
		t.Errorf("subject %s: got %d, want %d", subjSpansSentStat, got, numTotal)
	}
	if got := subjStats[subjSpansDroppedStat]; got != 0 {
		t.Errorf("subject %s: got %d, want 0", subjSpansDroppedStat, got)
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
		// Numeric-ish attrs: normalize to string for cross-side comparison.
		assertAttrNormalized(t, side, i, attrs, "http.status_code", "200")
		assertAttrNormalized(t, side, i, attrs, "request_size", "0")
		// response_size is 17 bytes (HTTPFixedBody "backend:v1/fixed\n") on
		// EITHER path — the backend serves the same fixed body regardless of path.
		assertAttrNormalized(t, side, i, attrs, "response_size", "17")
		// guid:x-request-id KEY must be present (value varies — UNasserted).
		assertAttrPresent(t, side, i, attrs, "guid:x-request-id")
		// upstream_cluster / upstream_cluster.name KEY present (value UNasserted —
		// framework gap: envoy-go emits "" while reference emits the cluster name.
		// This is EXACTLY why upstream_cluster cannot be the per-path partition
		// discriminator; see partitionByPath.)
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
			t.Errorf("%s continuation span %d: parent_span_id = %x, want %s",
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
			t.Errorf("%s ResourceSpans %d: service.name = %q, want %q", side, i, got, wantServiceName)
		}
	}
}

// spanProbePath recovers the probe path a span was produced by, from its
// `http.url` attribute.
//
// ⚠️ Why http.url and not upstream_cluster: 0116 asserts upstream_cluster
// PRESENCE only, because envoy-go emits it EMPTY
// (reference_tracing_upstream_cluster_framework_gap) — it therefore carries no
// cross-side information at all. http.url IS populated on both sides; its exact
// rendering (scheme/host encoding) differs enough that 0116 declined to assert
// it by VALUE, which is why this driver uses a SUFFIX test rather than an
// equality test, and why the recovered path was dumped live on BOTH sides
// before any assertion was written against it (FIXTURE_0117_DUMP=1).
//
// Returns "" when the span's http.url is absent or matches neither path.
func spanProbePath(sp *tracepb.Span) string {
	v, ok := spanAttrMap(sp)["http.url"]
	if !ok {
		return ""
	}
	url := v.GetStringValue()
	switch {
	case strings.HasSuffix(url, probePathA):
		return probePathA
	case strings.HasSuffix(url, probePathB):
		return probePathB
	default:
		return ""
	}
}

// partitionByPath splits spans into the probePathA set, the probePathB set and
// the UNCLASSIFIED remainder, using spanProbePath.
func partitionByPath(spans []*tracepb.Span) (aSpans, bSpans, unknown []*tracepb.Span) {
	for _, sp := range spans {
		switch spanProbePath(sp) {
		case probePathA:
			aSpans = append(aSpans, sp)
		case probePathB:
			bSpans = append(bSpans, sp)
		default:
			unknown = append(unknown, sp)
		}
	}
	return aSpans, bSpans, unknown
}

// assertClusterTags asserts the phase-73 CLUSTER metadata custom tags on EVERY
// span, cross-side by KEY (OTLP attribute order is non-deterministic — arbitrary
// PER PROCESS, not merely non-config-order) AND by VALUE, PARTITIONED BY PATH.
//
// cl_hit == THAT PATH'S OWN cluster value proves four things at once: the
// OWNING CLUSTER's metadata was actually SERVED (NOT a vacuous default match —
// the configured default "unused-default-0117" is never asserted); the
// namespace/path binding resolved; that — because "envoy.test" is NOT
// "envoy.lb" — a CLUSTER-kind tag addresses an ARBITRARY filter_metadata
// namespace on BOTH sides; and, because the SAME tag on the SAME HCM yields
// DIFFERENT values on the two paths, that the resolution is against the
// SELECTED cluster and not merely the only/first one (the limitation 0116 had
// to confess).
//
// cl_default == the default_value proves the absent-path → default rule.
//
// Errorf per property (continue past one span's failure so one bad span does
// not mask the rest). The partition SIZES are Fatalf preconditions: a broken
// partition would silently turn every value assertion into dead code.
func assertClusterTags(t fixture.TB, side string, spans []*tracepb.Span) {
	t.Helper()

	aSpans, bSpans, unknown := partitionByPath(spans)
	if len(unknown) != 0 {
		t.Fatalf("%s: %d span(s) could not be attributed to %q or %q by their http.url attribute "+
			"(partition precondition broken — the per-path assertions below would be dead code)",
			side, len(unknown), probePathA, probePathB)
	}
	if len(aSpans) != numPerPath || len(bSpans) != numPerPath {
		t.Fatalf("%s: per-path span partition = %d/%d for %q/%q, want %d/%d each",
			side, len(aSpans), len(bSpans), probePathA, probePathB, numPerPath, numPerPath)
	}

	assertClusterTagsForPath(t, side, probePathA, aSpans, clusterAValue)
	assertClusterTagsForPath(t, side, probePathB, bSpans, clusterBValue)
}

// assertClusterTagsForPath asserts cl_hit == wantHit and cl_default ==
// clusterDefaultFallback on every span of one path's partition.
func assertClusterTagsForPath(t fixture.TB, side, path string, spans []*tracepb.Span, wantHit string) {
	t.Helper()
	for i, sp := range spans {
		attrs := spanAttrMap(sp)

		// cl_hit: present with the OWNING cluster's static value (cross-side
		// EXACT VALUE, and DIFFERENT between the two paths).
		if v, ok := attrs[clusterHitTagKey]; !ok {
			t.Errorf("%s %s span %d: missing custom tag key %q (present: %v)", side, path, i, clusterHitTagKey, mapKeys(attrs))
		} else if got := v.GetStringValue(); got != wantHit {
			t.Errorf("%s %s span %d: %s = %q, want %q (the OWNING cluster's static clusters[].metadata value — cluster-metadata-served AND selected-not-only proof)",
				side, path, i, clusterHitTagKey, got, wantHit)
		}

		// cl_default: the path is unset on both clusters → the default_value is emitted.
		if v, ok := attrs[clusterDefaultTagKey]; !ok {
			t.Errorf("%s %s span %d: missing custom tag key %q (present: %v)", side, path, i, clusterDefaultTagKey, mapKeys(attrs))
		} else if got := v.GetStringValue(); got != clusterDefaultFallback {
			t.Errorf("%s %s span %d: %s = %q, want %q (the default_value — absent-path fallback)",
				side, path, i, clusterDefaultTagKey, got, clusterDefaultFallback)
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
		t.Errorf("%s span %d: missing attr key %q (present keys: %v)",
			side, spanIdx, key, mapKeys(attrs))
	}
}

// assertAttrString fails if key is absent or its string value != want.
func assertAttrString(t fixture.TB, side string, spanIdx int, attrs map[string]*commonpb.AnyValue, key, want string) {
	t.Helper()
	v, ok := attrs[key]
	if !ok {
		t.Errorf("%s span %d: missing attr key %q", side, spanIdx, key)
		return
	}
	got := v.GetStringValue()
	if got != want {
		t.Errorf("%s span %d: attr %q = %q, want %q (type=%T)", side, spanIdx, key, got, want, v.GetValue())
	}
}

// assertAttrNormalized asserts that the normalized string representation of the
// attr value (int or string) matches want.
func assertAttrNormalized(t fixture.TB, side string, spanIdx int, attrs map[string]*commonpb.AnyValue, key, want string) {
	t.Helper()
	v, ok := attrs[key]
	if !ok {
		t.Errorf("%s span %d: missing attr key %q", side, spanIdx, key)
		return
	}
	var got string
	switch vt := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		got = vt.StringValue
	case *commonpb.AnyValue_IntValue:
		got = strconv.FormatInt(vt.IntValue, 10)
	default:
		t.Errorf("%s span %d: attr %q has unexpected value type %T", side, spanIdx, key, v.GetValue())
		return
	}
	if got != want {
		t.Errorf("%s span %d: attr %q = %q, want %q", side, spanIdx, key, got, want)
	}
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// dumpSpans writes a debug summary of all span attrs to w (FIXTURE_0117_DUMP).
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

// dumpPartition writes the PARTITION RECONNAISSANCE record (FIXTURE_0117_DUMP):
// per span the raw http.url, the recovered path, and the two cluster tag values
// — plus the resulting partition sizes. This is the evidence the per-path
// partition mechanism rests on.
func dumpPartition(w io.Writer, side string, spans []*tracepb.Span) {
	for i, sp := range spans {
		attrs := spanAttrMap(sp)
		url := "<ABSENT>"
		if v, ok := attrs["http.url"]; ok {
			url = fmt.Sprintf("%q", v.GetStringValue())
		}
		upstream := "<ABSENT>"
		if v, ok := attrs["upstream_cluster"]; ok {
			upstream = fmt.Sprintf("%q", v.GetStringValue())
		}
		_, _ = fmt.Fprintf(w, "PARTITION %s span[%d] http.url=%s -> path=%q | upstream_cluster=%s | %s=%q %s=%q\n",
			side, i, url, spanProbePath(sp), upstream,
			clusterHitTagKey, attrs[clusterHitTagKey].GetStringValue(),
			clusterDefaultTagKey, attrs[clusterDefaultTagKey].GetStringValue())
	}
	a, b, unknown := partitionByPath(spans)
	_, _ = fmt.Fprintf(w, "PARTITION %s sizes: %s=%d %s=%d unknown=%d\n",
		side, probePathA, len(a), probePathB, len(b), len(unknown))
}

// scrapeFlatStats issues GET /stats against adminAddr and returns the flat
// "name: value" lines parsed into a map.
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

// --- file / template helpers (the 0115/0116 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0117-tracing-custom-tags-metadata-cluster/driver/driver.go
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

// logPartitionSummary records the per-side partition sizes through log.Printf —
// fixture.TB carries no Logf (reference_fixture_tb_has_no_logf), so RECORDING a
// diagnostic (as opposed to failing on it) has to go through the std logger.
func logPartitionSummary(side string, spans []*tracepb.Span) {
	a, b, unknown := partitionByPath(spans)
	log.Printf("0117 partition %s: %s=%d %s=%d unknown=%d", side, probePathA, len(a), probePathB, len(b), len(unknown))
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*traceOTLPDriver)(nil)
	_ fixture.BackendKindAware = (*traceOTLPDriver)(nil)
	_ fixture.StatsAsserter    = (*traceOTLPDriver)(nil)
)
