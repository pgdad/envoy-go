// Package driver registers the 0085-otlp-access-log-operators fixture with the
// differential runner. It is the behavioral proof of the phase 45.2 OTLP
// access-log OPERATOR ENGINE: cross-side EXACT (subject envoy-go vs reference
// Envoy v1.37.2 in Docker) on the command-operator-TEMPLATED OTLP LogRecord —
// the substituted body, the substituted per-record attributes (incl. a nested
// kvlist and an array), and the literal resource_attributes pass-through —
// aggregated across every exported record.
//
// Integration shape (single-listener fixture; plaintext HTTP/1.1 downstream +
// plaintext h2c OTLP cluster per D-OTLP-RECEIVER — no TLS):
//
//  1. ReferenceBootstrap renders envoy.yaml with host.docker.internal (ADR-0010
//     STRICT_DNS) + the runner-allocated HTTPFixedBody backend port + the
//     driver-owned in-process OTLP LogsService receiver host:port
//     (host=host.docker.internal for reference Envoy). The receiver port is
//     allocated lazily at ReferenceBootstrap time and the otlplogs.Server is
//     bound on 0.0.0.0:<port> BEFORE the reference container starts so the
//     reference Envoy can dial it (host.docker.internal bridge alias) AND the
//     subject can dial it (127.0.0.1). SubjectConfig renders envoy-go.yaml with
//     the runner-allocated admin/listener/backend ports + the SAME OTLP port
//     (host=127.0.0.1).
//
//  2. DriveReference / DriveSubject each fire N (numRequests) identical
//     query-less GET /health requests (User-Agent: otlp-probe/1, Host:
//     otlp.example) against the proxy under test, then POLL the receiver's
//     Count() until >= N (poll-to-converge, never sleep — the reference Envoy
//     buffers OTLP records and flushes them on a timer / buffer-fill). Each
//     side's record set AND per-ResourceLogs Resource.attributes snapshots are
//     captured into the driver before the next side runs (Reset() between sides
//     gives clean per-side separation — the subject generates no records until
//     its own DriveSubject window). The returned byte stream is the per-request
//     status sequence; the runner's CompareBytes pass asserts the data-plane
//     behaved equivalently cross-side.
//
//  3. AssertStats asserts, on BOTH sides AND cross-side EXACT: exactly N records;
//     for EVERY record a substituted body == "GET /health HTTP/1.1 200 <N>"
//     (method/path/protocol/code asserted literally; %BYTES_SENT% asserted
//     ref==subj rather than hardcoded — the fixed-body size is the same both
//     sides, so cross-side equality is the robust assertion); the substituted
//     per-record attributes op_method:"GET", nested{inner_code:"200",
//     inner_authority:"otlp.example"}, arr:["GET","literal-elem"]; and on EVERY
//     received Resource.attributes snapshot the four built-in label keys (with
//     log_name=="0085") PLUS the literal resource_attributes service_name=="envoy-go-test"
//     and authority_literal=="%REQ(:AUTHORITY)%" (the LITERAL un-substituted
//     string — AMEND-OPS-1). It ALSO asserts the SUBJECT-side stat
//     access_logs.open_telemetry_access_log.logs_written == N.
//
// UNasserted (framing / non-deterministic — reference_streaming_sink_differential_framing):
// the time_unix_nano VALUE; the Export-call count / per-call batch sizes /
// connection count; %START_TIME% / %DURATION% / %UPSTREAM_HOST% (excluded from
// the config — non-deterministic / connection-keyed); the node-derived Resource
// label VALUES (zone_name/cluster_name/node_name may be empty on both sides).
//
// Subject-side unit coverage (NOT duplicated as a fixture — one fixture dir = one
// runner branch): disable_builtin_labels + resource_attributes survival is the
// internal/accesslog otlpsink_test.go TestOTLPSink_DisableBuiltinResourceAttrsSurvive
// case (AMEND-OPS-5); the unsupported-operator + bad-value-type boot-rejects are
// internal/bootstrap bootstrap_test.go reject cases.
//
// Query-less path: the data-plane request MUST use a query-less path — envoy-go's
// H1 Record.Path strips the query (AMEND-OPS-6), so cross-side EXACT on the
// %REQ(:PATH)%-substituted body requires a query-less path.
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
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/otlplogs"
)

const (
	fixtureName = "0085-otlp-access-log-operators"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0085 takes 10085 for the single plaintext listener.
	refAdminPort    = 9901
	refListenerPort = 10085

	// numRequests is the per-side data-plane workload — N identical query-less
	// requests, each producing exactly one exported OTLP LogRecord.
	numRequests = 8

	// The fixed request shape (kept identical cross-side). Unlike 0084 these DO
	// surface on the record (operator-substituted): method/path/protocol/authority.
	probePath = "/health"      // query-less (AMEND-OPS-6) → %REQ(:PATH)%.
	probeHost = "otlp.example" // → :authority → %REQ(:AUTHORITY)%.
	probeUA   = "otlp-probe/1" // → user-agent.

	// log_name baked into both bootstraps → the Resource.attributes log_name value.
	wantLogName = "0085"

	// The literal (un-substituted) resource_attributes leaves (AMEND-OPS-1):
	// request operators in resource_attributes pass through VERBATIM.
	wantServiceName    = "envoy-go-test"
	wantAuthorityLit   = "%REQ(:AUTHORITY)%"
	resServiceNameKey  = "service_name"
	resAuthorityLitKey = "authority_literal"

	// The substituted attribute leaves (operators evaluated per record).
	wantOpMethod       = "GET"          // %REQ(:METHOD)%
	wantInnerCode      = "200"          // %RESPONSE_CODE%
	wantInnerAuthority = "otlp.example" // %REQ(:AUTHORITY)%
	wantArrLiteral     = "literal-elem"

	// Subject sink stat (flat /stats internal name) — successful exports.
	subjLogsWrittenStat = "access_logs.open_telemetry_access_log.logs_written"

	// Converge-poll discipline (reference_concurrency_differential_release_barrier):
	// POLL Count() to the expected total; never sleep-to-wait.
	pollInterval = 200 * time.Millisecond
	pollDeadline = 30 * time.Second
)

// bodyRe matches the substituted body "GET /health HTTP/1.1 200 <bytesSent>"
// with the trailing %BYTES_SENT% as a non-empty digit run (the value itself is
// asserted ref==subj, not against this regex).
var bodyRe = regexp.MustCompile(`^GET /health HTTP/1\.1 200 \d+$`)

func init() {
	fixture.RegisterFixture(fixtureName, &otlpDriver{})
}

// otlpDriver carries the per-driver lifecycle state — the OTLP receiver port
// (constant across the ref+subj run; allocated lazily at bootstrap time), the
// running receiver handle, and the per-side record + Resource.attributes
// snapshots captured during Drive for the AssertStats cross-side assertion.
type otlpDriver struct {
	mu sync.Mutex

	otlpPort int
	srv      *otlplogs.Server

	refRecords  []*logspb.LogRecord
	subjRecords []*logspb.LogRecord

	refResAttrs  [][]*commonpb.KeyValue
	subjResAttrs [][]*commonpb.KeyValue
}

// allocateOTLPPort reserves a free TCP port for the OTLP receiver via
// Listen+Close. Idempotent — returns the same port on subsequent calls. Does
// NOT start the server.
func (d *otlpDriver) allocateOTLPPort() int {
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

// ensureServer starts the in-process LogsService receiver bound to
// 0.0.0.0:<otlpPort> (so BOTH the reference container via host.docker.internal
// AND the subject via 127.0.0.1 can dial it). Idempotent. Called at
// ReferenceBootstrap time so the receiver is live before either proxy starts.
func (d *otlpDriver) ensureServer() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.srv != nil {
		return
	}
	if d.otlpPort == 0 {
		panic("driver: ensureServer called before allocateOTLPPort")
	}
	addr := fmt.Sprintf("0.0.0.0:%d", d.otlpPort)
	srv, err := otlplogs.NewAtAddr(addr)
	if err != nil {
		panic(fmt.Sprintf("driver: start OTLP receiver on %s: %v", addr, err))
	}
	d.srv = srv
}

// --- fixture.Driver (required) ---

func (*otlpDriver) BackendCount() int                { return 1 }
func (*otlpDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }
func (*otlpDriver) SubjectListenerName() string      { return "l_test" }
func (*otlpDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// runner-allocated backend port + the driver-owned OTLP receiver host:port
// (host=host.docker.internal). It allocates the OTLP port and starts the
// receiver here so it is live before the reference container boots.
func (d *otlpDriver) ReferenceBootstrap(backendPorts []int) string {
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
func (d *otlpDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	otlpPort := d.allocateOTLPPort()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
		"OTLPHost":     "127.0.0.1",
		"OTLPPort":     otlpPort,
	})
}

// DriveReference fires N requests against the reference proxy and snapshots the
// reference-side OTLP records + Resource.attributes.
func (d *otlpDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	out, records, resAttrs, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.refRecords = records
	d.refResAttrs = resAttrs
	return out, nil
}

// DriveSubject fires N requests against the subject proxy and snapshots the
// subject-side OTLP records + Resource.attributes. After the subject snapshot
// the receiver is hard-stopped SYNCHRONOUSLY via Close() (immediate
// grpc.Server.Stop — cancels the still-open proxy OTLP streams and returns at
// once) for deterministic teardown; the records are already snapshotted so
// canceling the streams loses nothing.
func (d *otlpDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, records, resAttrs, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.subjRecords = records
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

// driveSide resets the receiver accumulators, fires N identical query-less
// requests against the proxy listener at addr, polls Count() to >= N, and
// returns the per-request status byte stream plus snapshots of the received
// records and per-ResourceLogs Resource.attributes.
func (d *otlpDriver) driveSide(ctx context.Context, addr string) ([]byte, []*logspb.LogRecord, [][]*commonpb.KeyValue, error) {
	d.mu.Lock()
	srv := d.srv
	d.mu.Unlock()
	if srv == nil {
		return nil, nil, nil, fmt.Errorf("driver: OTLP receiver not running")
	}
	// Reset() is safe despite the helper's "no Export in flight" contract: on the
	// subject side the reference proxy's OTLP stream is open but QUIESCENT —
	// DriveReference returned only after all N reference records had arrived and
	// been snapshotted, and no further data-plane traffic reaches the reference in
	// the subject window, so no stray reference record can survive this Reset.
	srv.Reset()

	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	var b bytes.Buffer
	for i := 0; i < numRequests; i++ {
		code, err := d.fireProbe(ctx, client, addr)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("request %d: %w", i, err)
		}
		fmt.Fprintf(&b, "status=%d\n", code)
	}

	if err := pollCount(ctx, srv, numRequests); err != nil {
		return nil, nil, nil, err
	}
	return b.Bytes(), srv.Records(), srv.ResourceAttributes(), nil
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent
// and returns the response status code (the body is drained and discarded).
func (d *otlpDriver) fireProbe(ctx context.Context, client *http.Client, addr string) (int, error) {
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
// elapsing).
func pollCount(ctx context.Context, srv *otlplogs.Server, want int) error {
	deadline := time.Now().Add(pollDeadline)
	for {
		if n := srv.Count(); n >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("OTLP receiver: timed out waiting for %d records (got %d)", want, srv.Count())
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("OTLP receiver: context done waiting for %d records (got %d): %w", want, srv.Count(), ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes for the standard admin-diff at the runner's probe step.
func (*otlpDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertStats asserts, on BOTH sides AND cross-side EXACT: exactly numRequests
// records; the substituted body + per-record attributes on EVERY record; the
// four built-in Resource label keys (with log_name == "0085") PLUS the literal
// resource_attributes on EVERY snapshot. PLUS the subject-side logs_written stat.
func (d *otlpDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	if os.Getenv("FIXTURE_0085_DUMP") != "" {
		fmt.Fprintf(os.Stderr, "=== 0085 ref records=%d resAttrs=%d subj records=%d resAttrs=%d ===\n",
			len(d.refRecords), len(d.refResAttrs), len(d.subjRecords), len(d.subjResAttrs))
		for i, r := range d.refRecords {
			fmt.Fprintf(os.Stderr, "  ref[%d] body=%q\n", i, r.GetBody().GetStringValue())
		}
		for i, r := range d.subjRecords {
			fmt.Fprintf(os.Stderr, "  subj[%d] body=%q\n", i, r.GetBody().GetStringValue())
		}
	}

	// Both sides MUST have produced exactly numRequests records (a zero-record
	// "pass" is vacuous — prove the engine actually ran on BOTH sides).
	if len(d.refRecords) != numRequests {
		t.Fatalf("reference OTLP records: got %d, want %d", len(d.refRecords), numRequests)
	}
	if len(d.subjRecords) != numRequests {
		t.Fatalf("subject OTLP records: got %d, want %d", len(d.subjRecords), numRequests)
	}

	// Body: per-side homogeneity + format + cross-side EXACT (covers %BYTES_SENT%).
	refBody := assertBody(t, "reference", d.refRecords)
	subjBody := assertBody(t, "subject", d.subjRecords)
	if refBody != subjBody {
		t.Fatalf("body mismatch cross-side: reference %q != subject %q", refBody, subjBody)
	}

	// Attributes: substituted leaves on each side + cross-side EXACT.
	assertAttributes(t, "reference", d.refRecords)
	assertAttributes(t, "subject", d.subjRecords)

	// Resource.attributes: built-ins + literal pass-through on every snapshot.
	assertResourceLabels(t, "reference", d.refResAttrs)
	assertResourceLabels(t, "subject", d.subjResAttrs)

	// Subject-side stat: every export succeeded.
	subjStats, err := scrapeFlatStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subject /stats: %v", err)
	}
	if got := subjStats[subjLogsWrittenStat]; got != numRequests {
		t.Fatalf("subject %s: got %d, want %d", subjLogsWrittenStat, got, numRequests)
	}
}

// assertBody asserts every record's body matches "GET /health HTTP/1.1 200 <N>"
// (method/path/protocol/code literal; %BYTES_SENT% a digit run), that all
// records on the side are identical (all N requests are identical), and returns
// the (homogeneous) body for the cross-side equality check at the call site.
func assertBody(t fixture.TB, side string, records []*logspb.LogRecord) string {
	t.Helper()
	first := records[0].GetBody().GetStringValue()
	if !bodyRe.MatchString(first) {
		t.Fatalf("%s record 0: body %q does not match %q", side, first, bodyRe.String())
	}
	for i, r := range records {
		if got := r.GetBody().GetStringValue(); got != first {
			t.Fatalf("%s record %d: body %q != record 0 body %q (all requests identical)", side, i, got, first)
		}
	}
	return first
}

// assertAttributes asserts the substituted per-record attributes on every record:
// op_method (string "GET"), nested (kvlist {inner_code:"200", inner_authority:
// "otlp.example"}), arr (array ["GET","literal-elem"]).
func assertAttributes(t fixture.TB, side string, records []*logspb.LogRecord) {
	t.Helper()
	for i, r := range records {
		byKey := attrsByKey(r.GetAttributes())

		// op_method — a substituted string.
		if v := byKey["op_method"].GetStringValue(); v != wantOpMethod {
			t.Fatalf("%s record %d: op_method = %q, want %q", side, i, v, wantOpMethod)
		}

		// nested — a kvlist with two substituted string leaves.
		nested := byKey["nested"].GetKvlistValue()
		if nested == nil {
			t.Fatalf("%s record %d: nested is not a kvlist_value (got %v)", side, i, byKey["nested"])
		}
		nestedByKey := attrsByKey(nested.GetValues())
		if v := nestedByKey["inner_code"].GetStringValue(); v != wantInnerCode {
			t.Fatalf("%s record %d: nested.inner_code = %q, want %q", side, i, v, wantInnerCode)
		}
		if v := nestedByKey["inner_authority"].GetStringValue(); v != wantInnerAuthority {
			t.Fatalf("%s record %d: nested.inner_authority = %q, want %q", side, i, v, wantInnerAuthority)
		}

		// arr — an array of two string leaves: [%REQ(:METHOD)% → "GET", literal].
		arr := byKey["arr"].GetArrayValue()
		if arr == nil {
			t.Fatalf("%s record %d: arr is not an array_value (got %v)", side, i, byKey["arr"])
		}
		vals := arr.GetValues()
		if len(vals) != 2 {
			t.Fatalf("%s record %d: arr has %d elements, want 2", side, i, len(vals))
		}
		if v := vals[0].GetStringValue(); v != wantOpMethod {
			t.Fatalf("%s record %d: arr[0] = %q, want %q (substituted %%REQ(:METHOD)%%)", side, i, v, wantOpMethod)
		}
		if v := vals[1].GetStringValue(); v != wantArrLiteral {
			t.Fatalf("%s record %d: arr[1] = %q, want %q (literal)", side, i, v, wantArrLiteral)
		}
	}
}

// assertResourceLabels asserts that every received Resource.attributes snapshot
// carries the four built-in label keys {log_name, zone_name, cluster_name,
// node_name} with log_name == "0085", PLUS the literal resource_attributes
// service_name == "envoy-go-test" and authority_literal == "%REQ(:AUTHORITY)%"
// (the LITERAL un-substituted string — AMEND-OPS-1). The node-derived built-in
// three may be empty on BOTH sides — only their KEY presence is asserted.
func assertResourceLabels(t fixture.TB, side string, resAttrs [][]*commonpb.KeyValue) {
	t.Helper()
	if len(resAttrs) == 0 {
		t.Fatalf("%s: no Resource.attributes snapshots received", side)
	}
	wantKeys := []string{"log_name", "zone_name", "cluster_name", "node_name"}
	for i, attrs := range resAttrs {
		byKey := attrsByKey(attrs)
		for _, k := range wantKeys {
			if _, ok := byKey[k]; !ok {
				t.Fatalf("%s ResourceLogs %d: missing built-in label key %q (got keys %v)", side, i, k, keysOf(byKey))
			}
		}
		if v := byKey["log_name"].GetStringValue(); v != wantLogName {
			t.Fatalf("%s ResourceLogs %d: log_name = %q, want %q", side, i, v, wantLogName)
		}
		// The literal resource_attributes (AMEND-OPS-1): present + exact values,
		// authority_literal emitted VERBATIM (NOT operator-substituted).
		if v := byKey[resServiceNameKey].GetStringValue(); v != wantServiceName {
			t.Fatalf("%s ResourceLogs %d: %s = %q, want %q", side, i, resServiceNameKey, v, wantServiceName)
		}
		if v := byKey[resAuthorityLitKey].GetStringValue(); v != wantAuthorityLit {
			t.Fatalf("%s ResourceLogs %d: %s = %q, want %q (literal, NOT substituted)", side, i, resAuthorityLitKey, v, wantAuthorityLit)
		}
	}
}

// attrsByKey indexes a []*KeyValue by key for leaf lookups.
func attrsByKey(attrs []*commonpb.KeyValue) map[string]*commonpb.AnyValue {
	byKey := make(map[string]*commonpb.AnyValue, len(attrs))
	for _, kv := range attrs {
		byKey[kv.GetKey()] = kv.GetValue()
	}
	return byKey
}

func keysOf(m map[string]*commonpb.AnyValue) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// scrapeFlatStats issues GET /stats against adminAddr and returns the flat
// "name: value" lines parsed into a map. Used for the subject-side logs_written
// assertion (the access_logs. prefix has no Prometheus tag-extractor arm).
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
	// thisFile is .../test/fixtures/0085-otlp-access-log-operators/driver/driver.go
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
	_ fixture.Driver           = (*otlpDriver)(nil)
	_ fixture.BackendKindAware = (*otlpDriver)(nil)
	_ fixture.StatsAsserter    = (*otlpDriver)(nil)
)
