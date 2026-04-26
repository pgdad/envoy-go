package h2spec

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ─── Step 1: Skip gates ───────────────────────────────────────────────────────

// TestH2Spec is the phase-05.1 non-vacuous conformance gate (gate c).
// It boots an envoy-go subject with a synthetic h2c bootstrap, runs
// summerwind/h2spec via testcontainers-go, parses the JUnit-XML output, and
// asserts failed == 0 for every section in thresholdSections.
func TestH2Spec(t *testing.T) {
	// Step 1a: skip under -short.
	if testing.Short() {
		t.Skip("h2spec conformance suite is not -short (requires Docker)")
	}

	// Step 1b: skip if Docker is not available.
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH — skipping h2spec conformance")
	}
	dockerOK := exec.Command("docker", "version")
	if out, err := dockerOK.CombinedOutput(); err != nil {
		t.Skipf("docker version failed (%v) — skipping h2spec conformance\n%s", err, out)
	}

	// ─── Step 2: Build the subject binary ────────────────────────────────────
	bin := buildBinary(t)

	// ─── Step 3: Write the synthetic h2c bootstrap ───────────────────────────
	listenerPort := freeTCPPort(t)
	adminPort := freeTCPPort(t)

	bootstrapYAML := fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_h2c
      address:
        socket_address: { address: 0.0.0.0, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP2
                stat_prefix: h2spec_conformance
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, listenerPort)

	cfgPath := filepath.Join(t.TempDir(), "h2c-bootstrap.yaml")
	if err := os.WriteFile(cfgPath, []byte(bootstrapYAML), 0o600); err != nil {
		t.Fatalf("write h2c bootstrap: %v", err)
	}

	// ─── Step 4: Start the subject subprocess ────────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "--allow-h2c", "-c", cfgPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start subject: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	// ─── Step 5: Wait for the ready sentinel ─────────────────────────────────
	waitForReady(t, stdout, "l_h2c", 30*time.Second)

	// ─── Step 6: Run h2spec via testcontainers-go ────────────────────────────
	sectionArgs := thresholdSections
	// Full CLI: h2spec <sections...> -h host.docker.internal -p <port> -j /tmp/h2spec.xml
	// We write the report to /tmp inside the container and copy it out via
	// CopyFileFromContainer (avoids bind-mount issues with Docker Desktop's VM).
	const reportInsidePath = "/tmp/h2spec.xml"
	cmdArgs := append(sectionArgs,
		"-h", "host.docker.internal",
		"-p", fmt.Sprintf("%d", listenerPort),
		"-j", reportInsidePath,
		"-S", // --strict: run strict test cases too
	)

	req := testcontainers.ContainerRequest{
		Image:      imageRef(),
		Cmd:        cmdArgs,
		WaitingFor: wait.ForExit().WithPollInterval(500 * time.Millisecond),
		HostConfigModifier: func(hc *container.HostConfig) {
			// On Linux Docker adds the host-gateway alias so
			// host.docker.internal resolves to the host.
			hc.ExtraHosts = []string{"host.docker.internal:host-gateway"}
		},
	}

	h2specCtx, h2specCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer h2specCancel()

	c, err := testcontainers.GenericContainer(h2specCtx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start h2spec container: %v", err)
	}
	defer func() { _ = c.Terminate(context.Background()) }()

	// wait.ForExit already waited; drain container logs for debugging.
	logs, _ := c.Logs(h2specCtx)
	if logs != nil {
		logBytes, _ := io.ReadAll(logs)
		_ = logs.Close()
		if len(logBytes) > 0 {
			t.Logf("h2spec output:\n%s", logBytes)
		}
	}

	// ─── Step 7: Copy the JUnit XML report out of the container ──────────────
	reportReader, err := c.CopyFileFromContainer(h2specCtx, reportInsidePath)
	if err != nil {
		t.Fatalf("copy h2spec JUnit XML from container (%s): %v", reportInsidePath, err)
	}
	xmlBytes, err := io.ReadAll(reportReader)
	_ = reportReader.Close()
	if err != nil {
		t.Fatalf("read h2spec JUnit XML: %v", err)
	}

	// ─── Step 8: Parse the JUnit XML ─────────────────────────────────────────
	// Strip any null bytes that h2spec may pad at the end of the XML file.
	xmlBytes = []byte(strings.ReplaceAll(string(xmlBytes), "\x00", ""))
	suites, err := parseJUnitXML(xmlBytes)
	if err != nil {
		t.Fatalf("parse JUnit XML: %v", err)
	}

	// ─── Step 9: Assert threshold compliance ─────────────────────────────────
	assertThreshold(t, suites)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// buildBinary compiles cmd/envoy-go to a temp dir and returns the binary path.
func buildBinary(t *testing.T) string {
	t.Helper()
	// Locate the repo root by walking up from this source file.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate repo root")
	}
	// thisFile: .../test/conformance/h2spec/h2spec_test.go
	// repoRoot: ../../../  (3 levels up)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "envoy-go")

	buildCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	build := exec.CommandContext(buildCtx, "go", "build", "-o", bin, "./cmd/envoy-go")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build envoy-go: %v\n%s", err, out)
	}
	return bin
}

// freeTCPPort returns an available localhost TCP port.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// waitForReady reads lines from r until it sees the per-listener sentinel for
// listenerName or the terminal "envoy-go ready" sentinel.
func waitForReady(t *testing.T, r io.Reader, listenerName string, timeout time.Duration) {
	t.Helper()
	re := regexp.MustCompile(`^envoy-go listener (\S+) ready on (\S+)$`)
	deadline := time.Now().Add(timeout)
	br := bufio.NewReader(r)
	for time.Now().Before(deadline) {
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			t.Fatalf("subject stdout: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "envoy-go ready" {
			return
		}
		if m := re.FindStringSubmatch(line); m != nil && m[1] == listenerName {
			return
		}
	}
	t.Fatalf("ready sentinel for %q not seen within %s", listenerName, timeout)
}

// ─── JUnit XML parsing ───────────────────────────────────────────────────────

// junitTestSuites is the root element of JUnit XML output from h2spec.
type junitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	TestSuites []junitTestSuite `xml:"testsuite"`
}

// junitTestSuite represents a single <testsuite> in the JUnit XML.
type junitTestSuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

// junitTestCase represents a single <testcase> in a testsuite.
type junitTestCase struct {
	Name    string        `xml:"name,attr"`
	Failure *junitFailure `xml:"failure"`
}

// junitFailure is the <failure> element inside a test case.
type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

// parseJUnitXML parses h2spec's JUnit-XML output.
func parseJUnitXML(data []byte) ([]junitTestSuite, error) {
	var suites junitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		return nil, fmt.Errorf("xml unmarshal: %w", err)
	}
	return suites.TestSuites, nil
}

// ─── Threshold assertion ─────────────────────────────────────────────────────

// assertThreshold checks that every test suite with a non-zero test count has
// zero failures. Reports per-suite pass-counts and names of any failing tests.
func assertThreshold(t *testing.T, suites []junitTestSuite) {
	t.Helper()

	type suiteResult struct {
		name     string
		tests    int
		failures int
		failed   []string
	}

	var results []suiteResult
	totalTests := 0
	totalFailures := 0

	for _, s := range suites {
		if s.Tests == 0 {
			continue
		}
		res := suiteResult{
			name:     s.Name,
			tests:    s.Tests,
			failures: s.Failures + s.Errors,
		}
		for _, tc := range s.TestCases {
			if tc.Failure != nil {
				msg := tc.Failure.Message
				if msg == "" {
					msg = strings.TrimSpace(tc.Failure.Text)
				}
				res.failed = append(res.failed, fmt.Sprintf("%s: %s", tc.Name, msg))
			}
		}
		results = append(results, res)
		totalTests += s.Tests
		totalFailures += res.failures
	}

	// Print structured report regardless of pass/fail.
	t.Logf("h2spec conformance report: %d total tests, %d failures", totalTests, totalFailures)
	for _, r := range results {
		passed := r.tests - r.failures
		status := "PASS"
		if r.failures > 0 {
			status = "FAIL"
		}
		t.Logf("  [%s] %s: %d/%d passed", status, r.name, passed, r.tests)
		for _, f := range r.failed {
			t.Logf("       FAILED: %s", f)
		}
	}

	// Assert.
	if totalFailures > 0 {
		t.Errorf("h2spec threshold violated: %d test(s) failed across %d suite(s) — see per-suite report above",
			totalFailures, len(results))
	}
}
