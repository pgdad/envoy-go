// Package driver registers the 0006-access-log fixture with the differential
// runner. This is the project's first per-record access-log differential — it
// asserts field-by-field three-tier equivalence between envoy-go and reference
// Envoy v1.37.2 on the 15-operator default-format access-log per ADR-0068.
//
// Integration shape (SPEC §7.3 driver outline):
//
//  1. SubjectConfig(refListenerPort, subjListenerPort, backendPorts, subjAdminPort)
//     templates the envoy-go bootstrap with the subject log path
//     (<t.TempDir()>/subject.log generated at driver construction time via
//     os.TempDir()+rand) and the 3 backend ports, and stores the path.
//
//  2. ReferenceBootstrap(backendPorts) templates the reference bootstrap with
//     the 3 backend ports. The reference log path
//     (<t.TempDir()>/reference.log) is generated at construction time and
//     returned by ReferenceHostMounts() for the runner to bind-mount to
//     /envoy-go-test/envoy-access.log inside the container.
//
//  3. DriveReference(ctx, addr) / DriveSubject(ctx, addr): each issues 5
//     sequential H1 GETs [/health, /api/v1/foo, /api/v1/bar, /api/v1/baz,
//     /notfound], collects 200 response bodies (for the runner's byte-compare),
//     and stores the listener addr.
//
//  4. AssertAccessLog(t) — invoked by the runner after ProbeAdmin — polls
//     both log files until each has ≥ 5 lines (or the 5s deadline), then
//     applies the per-record three-tier matrix per ADR-0068.
//
// Adopted-prophylactically per 06.1 REVIEW M-8: the drain discipline uses a
// 25ms polling loop instead of an arbitrary time.Sleep.
package driver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

const fixtureName = "0006-access-log"
const refContainerListenerPort = 15006

// /tmp in the envoy image is sticky+world-writable, so with
// fs.protected_regular=2 (Ubuntu CI default) envoy (uid 101) gets EACCES
// opening the bind-mounted file owned by the host runner uid with O_CREAT.
// A file mounted under a non-sticky directory is exempt from that sysctl.
const refContainerLogPath = "/envoy-go-test/envoy-access.log"

func init() {
	fixture.RegisterFixture(fixtureName, newAccessLogDriver())
}

// newAccessLogDriver constructs the driver and pre-allocates per-test-unique
// host-side log file paths under os.TempDir(). These paths are generated at
// driver construction (init time) and used throughout the fixture run so that
// SubjectConfig / ReferenceBootstrap / AssertAccessLog all see the same paths.
//
// The paths are unique per process via os.Getpid() + nanoseconds, which is
// sufficient for the single-process test runner.
func newAccessLogDriver() *accessLogDriver {
	base := os.TempDir()
	tag := fmt.Sprintf("envoy-go-0006-%d-%d", os.Getpid(), time.Now().UnixNano())
	return &accessLogDriver{
		subjLogPath: filepath.Join(base, tag+"-subject.log"),
		refLogPath:  filepath.Join(base, tag+"-reference.log"),
	}
}

// accessLogDriver implements:
//   - fixture.Driver
//   - fixture.BackendKindAware
//   - fixture.ReferenceLogMounter  (tells the runner to bind-mount refLogPath)
//   - fixture.AccessLogAsserter    (three-tier matrix assertion)
type accessLogDriver struct {
	mu               sync.Mutex
	subjLogPath      string // host path for subject's access log
	refLogPath       string // host path for reference's access log (bind-mounted)
	subjListenerAddr string
	refListenerAddr  string
}

// BackendCount returns 3 — one per RR endpoint in c_backend.
func (*accessLogDriver) BackendCount() int { return 3 }

// BackendKind selects the HTTPFixedBody backend spawner so backends return the
// fixed 17-byte body "backend:v1/fixed\n" for BYTES_SENT Tier-E equality.
func (*accessLogDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFixedBody }

// SubjectListenerName is the listener name the driver's DriveSubject targets.
func (*accessLogDriver) SubjectListenerName() string { return "l_h1" }

// ReferenceListenerPort is the in-container port the reference Envoy listens on.
func (*accessLogDriver) ReferenceListenerPort() int { return refContainerListenerPort }

// ReferenceHostMounts returns the bind-mount descriptor for the reference log
// file. The runner pre-creates the host file and calls
// StartReferenceProxyWithMounts. (fixture.ReferenceLogMounter interface.)
func (d *accessLogDriver) ReferenceHostMounts() []fixture.HostMount {
	return []fixture.HostMount{
		{HostPath: d.refLogPath, ContainerPath: refContainerLogPath},
	}
}

// ReferenceBootstrap returns the reference Envoy bootstrap YAML with backend
// ports filled in.
func (d *accessLogDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("0006: expected 3 backend ports, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(referenceTmpl, backendPorts[0], backendPorts[1], backendPorts[2])
}

// SubjectConfig returns the envoy-go bootstrap YAML with all dynamic values
// filled in. The access_log path is the pre-allocated subjLogPath.
func (d *accessLogDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 3 {
		panic(fmt.Sprintf("0006: expected 3 backend ports, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(subjectTmpl,
		subjAdminPort,
		subjListenerPort,
		d.subjLogPath,
		backendPorts[0],
		backendPorts[1],
		backendPorts[2],
	)
}

// DriveReference issues the 5-request workload against the reference proxy,
// stores the addr for AssertAccessLog, and returns the 200 response bodies.
// addr may be "localhost:{port}" (testcontainers maps container ports to localhost);
// we normalize it to "127.0.0.1:{port}" so the Go HTTP client sends
// `Host: 127.0.0.1:{port}` — matching the subject side and satisfying
// the Tier-E AUTHORITY assertion.
func (d *accessLogDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	if strings.HasPrefix(addr, "localhost:") {
		addr = "127.0.0.1:" + addr[len("localhost:"):]
	}
	d.mu.Lock()
	d.refListenerAddr = addr
	d.mu.Unlock()
	return driveWorkload(ctx, addr)
}

// DriveSubject issues the 5-request workload against the subject proxy,
// stores the addr for AssertAccessLog, and returns the 200 response bodies.
func (d *accessLogDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.subjListenerAddr = addr
	d.mu.Unlock()
	return driveWorkload(ctx, addr)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// raw HTTP response bytes for the runner's byte-comparison step.
func (*accessLogDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
	refBytes, err := httpGetRaw(ctx, "http://"+refAdminAddr+"/ready")
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	subjBytes, err := httpGetRaw(ctx, "http://"+subjAdminAddr+"/ready")
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// AssertAccessLog implements fixture.AccessLogAsserter (ADR-0068). Invoked by
// the runner after ProbeAdmin. It polls both log files until each has ≥ 5 lines
// (hard deadline 5s, 25ms poll per Decision G / 06.1 REVIEW M-8), then applies
// the per-record per-field three-tier matrix.
func (d *accessLogDriver) AssertAccessLog(t fixture.TB) {
	t.Helper()

	// Poll subject log. envoy-go's AsyncFileSink writes on each record, so
	// 5 lines appear quickly; a 5s deadline is generous.
	subjLines, err := pollLogLines(d.subjLogPath, 5, 5*time.Second, 25*time.Millisecond)
	if err != nil {
		t.Fatalf("access-log: subject log poll: %v", err)
		return
	}
	// Poll reference log. Envoy v1.37.2 uses a periodic access-log buffer
	// flush (default ~1s). A 30s deadline guarantees the flush fires at least
	// once for each of the 5 buffered records within the poll window.
	refLines, err := pollLogLines(d.refLogPath, 5, 30*time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("access-log: reference log poll: %v", err)
		return
	}

	if len(subjLines) != len(refLines) {
		t.Errorf("access-log: record count mismatch: subject=%d reference=%d", len(subjLines), len(refLines))
		return
	}

	for i := range subjLines {
		subjTuple, err := ParseLogLine(subjLines[i])
		if err != nil {
			t.Errorf("record %d: subject parse: %v (line=%q)", i+1, err, subjLines[i])
			continue
		}
		refTuple, err := ParseLogLine(refLines[i])
		if err != nil {
			t.Errorf("record %d: reference parse: %v (line=%q)", i+1, err, refLines[i])
			continue
		}
		AssertRecord(t, i+1, subjTuple, refTuple)
	}
}

// driveWorkload issues the 5-request workload:
//
//	GET /health         → 200  (direct_response)
//	GET /api/v1/foo     → 200  (routed)
//	GET /api/v1/bar     → 200  (routed)
//	GET /api/v1/baz     → 200  (routed)
//	GET /notfound       → 404  (direct_response)
//
// Returns the concatenated bytes of all 200 response bodies.
func driveWorkload(ctx context.Context, addr string) ([]byte, error) {
	type req struct {
		path   string
		status int
	}
	plan := []req{
		{"/health", 200},
		{"/api/v1/foo", 200},
		{"/api/v1/bar", 200},
		{"/api/v1/baz", 200},
		{"/notfound", 404},
	}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}

	var out bytes.Buffer
	for i, r := range plan {
		url := "http://" + addr + r.path
		httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("req[%d]: build: %w", i, err)
		}
		resp, err := client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("req[%d]: do: %w", i, err)
		}
		body, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if rerr != nil {
			return nil, fmt.Errorf("req[%d]: read body: %w", i, rerr)
		}
		if resp.StatusCode != r.status {
			return nil, fmt.Errorf("req[%d] %s: got status %d, want %d (body=%q)",
				i, r.path, resp.StatusCode, r.status, string(body))
		}
		if resp.StatusCode == 200 {
			out.Write(body)
		}
	}
	return out.Bytes(), nil
}

// httpGetRaw issues GET and returns a synthetic raw HTTP/1.1 response
// (status line + headers + body) as bytes.
func httpGetRaw(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var buf strings.Builder
	fmt.Fprintf(&buf, "HTTP/1.1 %s\r\n", resp.Status)
	for k, vs := range resp.Header {
		for _, v := range vs {
			fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
		}
	}
	buf.WriteString("\r\n")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	buf.Write(body)
	return []byte(buf.String()), nil
}

// pollLogLines polls path at interval until the file has at least minLines
// non-empty lines, or deadline is reached. Returns the lines (including empty
// ones stripped) or an error on deadline-trip.
func pollLogLines(path string, minLines int, deadline, interval time.Duration) ([]string, error) {
	limit := time.Now().Add(deadline)
	for {
		lines, err := readLogLines(path)
		if err == nil && len(lines) >= minLines {
			return lines, nil
		}
		if time.Now().After(limit) {
			got := 0
			if err == nil {
				got = len(lines)
			}
			return nil, fmt.Errorf("timeout after %v waiting for %d lines in %s (got %d, readErr=%v)",
				deadline, minLines, path, got, err)
		}
		time.Sleep(interval)
	}
}

// readLogLines reads the file at path and returns non-empty lines.
func readLogLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, sc.Err()
}

// logLineRE is the regex that parses the Envoy default-format access-log line
// into 15 positional capture groups. Anchored on the literal delimiters
// observed in the empirical pin (SPEC §11):
//
//	[<START_TIME>] "<METHOD> <PATH> <PROTOCOL>" <RCODE> <RFLAGS> <BREC> <BSENT> <DUR> <SVC_TIME> "<XFF>" "<UA>" "<XRI>" "<AUTH>" "<UHOST>"
//
// The regex is used by ParseLogLine (exported for driver_test.go unit tests).
var logLineRE = regexp.MustCompile(
	`^\[([^\]]+)\]` + // 1: START_TIME (inside [...])
		` "([^ ]+)` + // 2: :METHOD (inside open-quote)
		` ([^ ]+)` + // 3: :PATH
		` ([^"]+)"` + // 4: PROTOCOL (closes the quoted request-line block)
		` ([^ ]+)` + // 5: RESPONSE_CODE
		` ([^ ]+)` + // 6: RESPONSE_FLAGS
		` ([^ ]+)` + // 7: BYTES_RECEIVED
		` ([^ ]+)` + // 8: BYTES_SENT
		` ([^ ]+)` + // 9: DURATION
		` ([^ ]+)` + // 10: RESP-SVC-TIME
		` "([^"]*)"` + // 11: X-FORWARDED-FOR (quoted)
		` "([^"]*)"` + // 12: USER-AGENT (quoted)
		` "([^"]*)"` + // 13: X-REQUEST-ID (quoted)
		` "([^"]*)"` + // 14: :AUTHORITY (quoted)
		` "([^"]*)"` + // 15: UPSTREAM_HOST (quoted)
		`$`,
)

// LogTuple holds the 15 positional fields parsed from one access-log line.
// Field names match the SPEC §6 operator table.
type LogTuple struct {
	StartTime     string // field 1
	Method        string // field 2
	Path          string // field 3
	Protocol      string // field 4
	ResponseCode  string // field 5
	ResponseFlags string // field 6
	BytesReceived string // field 7
	BytesSent     string // field 8
	Duration      string // field 9
	SvcTime       string // field 10
	XForwardedFor string // field 11
	UserAgent     string // field 12
	XRequestID    string // field 13
	Authority     string // field 14
	UpstreamHost  string // field 15
}

// ParseLogLine parses one access-log line into a LogTuple via logLineRE.
// Exported for driver_test.go round-trip tests.
func ParseLogLine(line string) (LogTuple, error) {
	m := logLineRE.FindStringSubmatch(line)
	if m == nil {
		return LogTuple{}, fmt.Errorf("no regex match on line: %q", line)
	}
	return LogTuple{
		StartTime:     m[1],
		Method:        m[2],
		Path:          m[3],
		Protocol:      m[4],
		ResponseFlags: m[6],
		BytesReceived: m[7],
		BytesSent:     m[8],
		Duration:      m[9],
		SvcTime:       m[10],
		XForwardedFor: m[11],
		UserAgent:     m[12],
		XRequestID:    m[13],
		Authority:     m[14],
		UpstreamHost:  m[15],
		ResponseCode:  m[5],
	}, nil
}

// startTimeRE validates Tier-F START_TIME: RFC3339 ms-precision UTC.
var startTimeRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

// upstreamHostRE validates Tier-F UPSTREAM_HOST for routed records: <host>:<port>.
var upstreamHostRE = regexp.MustCompile(`^[^:]+:\d+$`)

// AssertRecord applies the three-tier matrix (ADR-0068) to a pair of parsed
// log tuples at record index recIdx (1-indexed). Exported for unit tests.
func AssertRecord(t fixture.TB, recIdx int, subj, ref LogTuple) {
	t.Helper()

	// Helper for Tier-E (byte-equal cross-side).
	assertE := func(field, sVal, rVal string) {
		t.Helper()
		if sVal != rVal {
			t.Errorf("record %d / field %s: Tier-E mismatch: subject=%q reference=%q",
				recIdx, field, sVal, rVal)
		}
	}

	// Helper for Tier-S (subject must emit "-"; reference unconstrained).
	assertS := func(field, sVal string) {
		t.Helper()
		if sVal != "-" {
			t.Errorf("record %d / field %s: Tier-S violation: subject=%q (must be \"-\")",
				recIdx, field, sVal)
		}
	}

	// Field 1: START_TIME — Tier F.
	if !startTimeRE.MatchString(subj.StartTime) {
		t.Errorf("record %d / field START_TIME: Tier-F format: subject=%q does not match RFC3339-ms-UTC",
			recIdx, subj.StartTime)
	}
	if !startTimeRE.MatchString(ref.StartTime) {
		t.Errorf("record %d / field START_TIME: Tier-F format: reference=%q does not match RFC3339-ms-UTC",
			recIdx, ref.StartTime)
	}

	// Field 2: :METHOD — Tier E.
	assertE("METHOD", subj.Method, ref.Method)

	// Field 3: :PATH — Tier E.
	assertE("PATH", subj.Path, ref.Path)

	// Field 4: PROTOCOL — Tier E.
	assertE("PROTOCOL", subj.Protocol, ref.Protocol)

	// Field 5: RESPONSE_CODE — Tier E.
	assertE("RESPONSE_CODE", subj.ResponseCode, ref.ResponseCode)

	// Field 6: RESPONSE_FLAGS — Tier S (subject must emit "-").
	assertS("RESPONSE_FLAGS", subj.ResponseFlags)

	// Field 7: BYTES_RECEIVED — Tier S (subject must emit "-").
	assertS("BYTES_RECEIVED", subj.BytesReceived)

	// Field 8: BYTES_SENT — Tier E.
	assertE("BYTES_SENT", subj.BytesSent, ref.BytesSent)

	// Field 9: DURATION — Tier F (int ms ≥ 0).
	if d, err := strconv.ParseInt(subj.Duration, 10, 64); err != nil || d < 0 {
		t.Errorf("record %d / field DURATION: Tier-F format: subject=%q (must be int>=0)",
			recIdx, subj.Duration)
	}
	if d, err := strconv.ParseInt(ref.Duration, 10, 64); err != nil || d < 0 {
		t.Errorf("record %d / field DURATION: Tier-F format: reference=%q (must be int>=0)",
			recIdx, ref.Duration)
	}

	// Field 10: RESP(X-ENVOY-UPSTREAM-SERVICE-TIME) — Tier S.
	// Reference Envoy injects X-Envoy-Upstream-Service-Time on routed requests
	// (emits the upstream service time in ms, e.g. "0"). envoy-go does not
	// inject this header (Decision A), so subject MUST emit "-" for this field.
	assertS("RESP_SVC_TIME", subj.SvcTime)

	// Field 11: X-FORWARDED-FOR — Tier S.
	assertS("X_FORWARDED_FOR", subj.XForwardedFor)

	// Field 12: USER-AGENT — Tier E.
	assertE("USER_AGENT", subj.UserAgent, ref.UserAgent)

	// Field 13: X-REQUEST-ID — Tier S.
	assertS("X_REQUEST_ID", subj.XRequestID)

	// Field 14: :AUTHORITY — Tier E after host-part normalization.
	// Both sides send to 127.0.0.1 (subject direct; reference inside container
	// via host.docker.internal → 127.0.0.1 internal. The authority value in the
	// log is the :authority header the client sends, which is the host:port the
	// driver dialed. Per §7.5 recommendation: strip the port and assert the host
	// part is equal.
	subjAuthHost := authorityHost(subj.Authority)
	refAuthHost := authorityHost(ref.Authority)
	if subjAuthHost != refAuthHost {
		t.Errorf("record %d / field AUTHORITY: Tier-E host mismatch: subject=%q reference=%q (normalized: %q vs %q)",
			recIdx, subj.Authority, ref.Authority, subjAuthHost, refAuthHost)
	}

	// Field 15: UPSTREAM_HOST — Tier F.
	// For direct_response records: both must be "-".
	// For routed records: must match <host>:<port> on both sides (values may differ).
	if subj.UpstreamHost == "-" && ref.UpstreamHost == "-" {
		// direct_response path — both must be "-".
	} else if subj.UpstreamHost == "-" && ref.UpstreamHost != "-" {
		t.Errorf("record %d / field UPSTREAM_HOST: Tier-F: subject='-' but reference=%q (expected both '-' or both host:port)",
			recIdx, ref.UpstreamHost)
	} else if subj.UpstreamHost != "-" && ref.UpstreamHost == "-" {
		t.Errorf("record %d / field UPSTREAM_HOST: Tier-F: subject=%q but reference='-' (expected both '-' or both host:port)",
			recIdx, subj.UpstreamHost)
	} else {
		// Both are non-"-"; validate format on each side.
		if !upstreamHostRE.MatchString(subj.UpstreamHost) {
			t.Errorf("record %d / field UPSTREAM_HOST: Tier-F format: subject=%q does not match host:port",
				recIdx, subj.UpstreamHost)
		}
		if !upstreamHostRE.MatchString(ref.UpstreamHost) {
			t.Errorf("record %d / field UPSTREAM_HOST: Tier-F format: reference=%q does not match host:port",
				recIdx, ref.UpstreamHost)
		}
	}
}

// authorityHost strips the port from a host:port authority string. If the
// string contains no ':', it is returned as-is.
func authorityHost(authority string) string {
	if i := strings.LastIndexByte(authority, ':'); i >= 0 {
		return authority[:i]
	}
	return authority
}

// subjectTmpl is the envoy-go bootstrap template.
// Parameters: adminPort, listenerPort, logPath, backendPort0, backendPort1, backendPort2.
var subjectTmpl = `admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_h1
      address: { socket_address: { address: 127.0.0.1, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                access_log:
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: %s
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
                route_config:
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { path: /health }
                          direct_response: { status: 200, body: { inline_string: "OK\n" } }
                        - match: { path: /notfound }
                          direct_response: { status: 404, body: { inline_string: "not found\n" } }
                        - match: { prefix: /api/v1/ }
                          route: { cluster: c_backend }
  clusters:
    - name: c_backend
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address: { socket_address: { address: 127.0.0.1, port_value: %d } }
              - endpoint:
                  address: { socket_address: { address: 127.0.0.1, port_value: %d } }
              - endpoint:
                  address: { socket_address: { address: 127.0.0.1, port_value: %d } }
`

// referenceTmpl is the reference Envoy bootstrap template.
// Parameters: backendPort0, backendPort1, backendPort2.
//
// The access log is written to /envoy-go-test/envoy-access.log (bind-mounted
// to the host at refLogPath). Envoy v1.37.2 flushes the file access logger
// buffer on a periodic timer (default 1s). AssertAccessLog polls with a 30s
// deadline to accommodate the timer.
var referenceTmpl = `admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_h1
      address: { socket_address: { address: 0.0.0.0, port_value: 15006 } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                access_log:
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: /envoy-go-test/envoy-access.log
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
                route_config:
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { path: /health }
                          direct_response: { status: 200, body: { inline_string: "OK\n" } }
                        - match: { path: /notfound }
                          direct_response: { status: 404, body: { inline_string: "not found\n" } }
                        - match: { prefix: /api/v1/ }
                          route: { cluster: c_backend }
  clusters:
    - name: c_backend
      type: STRICT_DNS
      dns_lookup_family: V4_ONLY
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address: { socket_address: { address: host.docker.internal, port_value: %d } }
              - endpoint:
                  address: { socket_address: { address: host.docker.internal, port_value: %d } }
              - endpoint:
                  address: { socket_address: { address: host.docker.internal, port_value: %d } }
`

// Compile-time interface checks.
var (
	_ fixture.Driver              = (*accessLogDriver)(nil)
	_ fixture.BackendKindAware    = (*accessLogDriver)(nil)
	_ fixture.ReferenceLogMounter = (*accessLogDriver)(nil)
	_ fixture.AccessLogAsserter   = (*accessLogDriver)(nil)
)
