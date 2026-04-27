package differential

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	_ "github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver"
	"github.com/esalaine/envoy-go/test/helpers"
)

// TestDifferential is the differential suite entry point. It discovers
// fixture directories under test/fixtures/, runs each as a subtest, and fails
// the suite if any fixture's diff verdict is not Equal.
func TestDifferential(t *testing.T) {
	if testing.Short() {
		t.Skip("differential suite; skipped under -short")
	}
	ensureDocker(t)

	root := repoRoot(t)
	fixtures := discoverFixtures(t, filepath.Join(root, "test", "fixtures"))
	pin := loadPinFromRepo(t)

	for _, fx := range fixtures {
		fx := fx
		t.Run(fx, func(t *testing.T) {
			driver, ok := fixture.DriverRegistry[fx]
			if !ok {
				// A fixture directory with no registered driver is a valid
				// intermediate state during phase rollouts (e.g. fixture-0004
				// content lands at Task 13 but its driver lands at Task 14).
				// Skip with a clear log; the next task's blank-import flips this.
				t.Skipf("no driver registered for fixture %q (driver package not yet blank-imported in runner_test.go)", fx)
			}
			runFixture(t, root, pin, fx, driver)
		})
	}
}

func runFixture(t *testing.T, root string, pin *EnvoyPin, _ string, d FixtureDriver) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. N backends, each with its own atomic.Uint64 accept counter.
	n := d.BackendCount()
	if n < 1 {
		t.Fatalf("BackendCount() returned %d; must be >=1", n)
	}
	type backend struct {
		idx     int
		ln      net.Listener
		port    int
		accepts *atomic.Uint64
		// proc is non-nil for subprocess backends (HTTPSH2). The runner's
		// in-process accept counter is NOT incremented for these; drivers
		// must derive distribution from response bodies instead.
		proc *exec.Cmd
	}
	kind := fixture.TCPEcho
	if bk, ok := d.(fixture.BackendKindAware); ok {
		kind = bk.BackendKind()
	}
	backends := make([]*backend, n)
	for i := 0; i < n; i++ {
		bo := &backend{idx: i, accepts: new(atomic.Uint64)}
		switch kind {
		case fixture.TCPEcho, fixture.HTTPEcho:
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			if kind == fixture.TCPEcho {
				go acceptEchoCounting(ln, bo.accepts)
			} else {
				go acceptHTTPEchoCounting(ln, bo.accepts, bo.idx)
			}
		case fixture.HTTPSH2:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPSH2Backend(ctx, root, port, i)
			if err != nil {
				t.Fatalf("backend[%d] start: %v", i, err)
			}
			bo.proc = cmd
			defer func(cmd *exec.Cmd) {
				// Kill the entire process group so the binary spawned by `go
				// run` (the actual backend server, re-parented to PID 1 if
				// only `go run` is killed) is reaped too. Without this the
				// orphaned backend keeps the test process's stderr fd open
				// and Cmd.WaitDelay times out causing a spurious package-
				// level FAIL.
				if cmd.Process != nil {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}(cmd)
			if err := waitTCPDial(ctx, fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second); err != nil {
				t.Fatalf("backend[%d] not ready: %v", i, err)
			}
		case fixture.HTTPStatusHeader:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPStatusHeaderBackend(ctx, root, port)
			if err != nil {
				t.Fatalf("backend[%d] start: %v", i, err)
			}
			bo.proc = cmd
			defer func(cmd *exec.Cmd) {
				if cmd.Process != nil {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}(cmd)
			if err := waitTCPDial(ctx, fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second); err != nil {
				t.Fatalf("backend[%d] not ready: %v", i, err)
			}
		}
		backends[i] = bo
	}
	backendPorts := make([]int, n)
	for i, b := range backends {
		backendPorts[i] = b.port
	}

	// 2. Reference proxy.
	bootstrap := d.ReferenceBootstrap(backendPorts)
	ref, err := StartReferenceProxy(ctx, pin, bootstrap, d.ReferenceListenerPort())
	if err != nil {
		t.Fatalf("ref start: %v", err)
	}
	defer func() { _ = ref.Stop(context.Background()) }()
	refAddr := ref.ListenerAddr(d.ReferenceListenerPort())

	// 3. Subject proxy.
	subjPort := freeTCPPort(t)
	subjAdminPort := freeTCPPort(t)
	subjCfg := d.SubjectConfig(d.ReferenceListenerPort(), subjPort, backendPorts, subjAdminPort)
	subj, err := StartSubjectProxy(ctx, root, subjCfg, fmt.Sprintf("127.0.0.1:%d", subjAdminPort))
	if err != nil {
		t.Fatalf("subj start: %v", err)
	}
	defer func() { _ = subj.Stop() }()

	// 4. Snapshot baseline accept counts before driving ref (the reference
	// container's admin probe during StartReferenceProxy may have triggered
	// accepts against host.docker.internal backends; we only credit the
	// post-baseline delta).
	refBaseline := make([]uint64, n)
	for i, b := range backends {
		refBaseline[i] = b.accepts.Load()
	}

	// 5. Drive ref.
	refBytes, err := d.DriveReference(ctx, refAddr)
	if err != nil {
		t.Fatalf("ref drive: %v", err)
	}
	refCounts := make([]uint64, n)
	for i, b := range backends {
		refCounts[i] = b.accepts.Load() - refBaseline[i]
	}
	subjBaseline := make([]uint64, n)
	for i, b := range backends {
		subjBaseline[i] = b.accepts.Load()
	}

	// 6. Drive subj.
	subjBytes, err := d.DriveSubject(ctx, subj.ListenerAddr(d.SubjectListenerName()))
	if err != nil {
		t.Fatalf("subj drive: %v", err)
	}
	subjCounts := make([]uint64, n)
	for i, b := range backends {
		subjCounts[i] = b.accepts.Load() - subjBaseline[i]
	}

	// 7. Diff response bytes.
	v, err := CompareBytes(refBytes, subjBytes)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !v.Equal {
		t.Errorf("differential mismatch:\n%s", v.HexDump)
	}

	// 8. Optional distribution assertion.
	if da, ok := d.(fixture.DistributionAsserter); ok {
		if err := da.AssertDistribution(refCounts, subjCounts); err != nil {
			t.Errorf("distribution: %v", err)
		}
	}

	// 8b. Optional per-request HTTP round-trip + status/body/header
	// orchestration (phase 04, ADR-0043). Only fires when the driver
	// implements fixture.HTTPExpectations; phase-02/phase-03 fixtures
	// do not, so this is a no-op for 0000, 0001, 0002.
	if he, ok := d.(fixture.HTTPExpectations); ok {
		for i, exp := range he.HTTPExpectations() {
			refResp, refBody, err := helpers.HTTPRoundTrip(ctx, refAddr, exp.Method, exp.Path, nil, nil)
			if err != nil {
				t.Errorf("expectation[%d]: ref round-trip: %v", i, err)
				continue
			}
			subjResp, subjBody, err := helpers.HTTPRoundTrip(ctx, subj.ListenerAddr(d.SubjectListenerName()), exp.Method, exp.Path, nil, nil)
			if err != nil {
				t.Errorf("expectation[%d]: subj round-trip: %v", i, err)
				continue
			}
			if refResp.StatusCode != exp.ExpectStatus {
				t.Errorf("expectation[%d]: ref status: got %d, want %d", i, refResp.StatusCode, exp.ExpectStatus)
			}
			if subjResp.StatusCode != exp.ExpectStatus {
				t.Errorf("expectation[%d]: subj status: got %d, want %d", i, subjResp.StatusCode, exp.ExpectStatus)
			}
			if exp.ExpectBodyEquivalent && !bytes.Equal(refBody, subjBody) {
				t.Errorf("expectation[%d]: body mismatch:\n ref:  %q\n subj: %q", i, string(refBody), string(subjBody))
			}
			refOnly, subjOnly := helpers.HTTPHeaderDiff(refResp.Header, subjResp.Header, helpers.PhaseFourHTTPAllowList)
			if len(refOnly) > 0 || len(subjOnly) > 0 {
				t.Errorf("expectation[%d]: header diff outside allow-list:\n  ref-only: %v\n  subj-only: %v", i, refOnly, subjOnly)
			}
		}
	}

	// 9. Admin /ready observation (phase 01 addition — SPEC §5.6).
	refAdm, subjAdm, err := d.ProbeAdmin(ctx, ref.AdminAddr(), subj.AdminAddr())
	if err != nil {
		t.Fatalf("admin probe: %v", err)
	}
	vAdm, err := compareAdminResponses(refAdm, subjAdm, d)
	if err != nil {
		t.Fatalf("admin compare: %v", err)
	}
	if !vAdm.Equal {
		t.Errorf("admin differential mismatch:\n%s", vAdm.HexDump)
	}

	// 10. Optional stats equivalence assertion (phase 06.1, ADR-0062). Fires
	// when the driver implements fixture.StatsAsserter. The runner passes both
	// admin addrs it already holds; the driver performs the scrape-and-diff in-
	// band (SPEC §12 #6 in-band discipline — no generic StatsExpectations data
	// structure extension).
	if sa, ok := d.(fixture.StatsAsserter); ok {
		sa.AssertStats(t, ref.AdminAddr(), subj.AdminAddr())
	}
}

func compareAdminResponses(refRaw, subjRaw []byte, _ fixture.Driver) (Verdict, error) {
	refResp, err := helpers.ParseHTTPResponse(refRaw)
	if err != nil {
		return Verdict{}, fmt.Errorf("ref parse: %w", err)
	}
	subjResp, err := helpers.ParseHTTPResponse(subjRaw)
	if err != nil {
		return Verdict{}, fmt.Errorf("subj parse: %w", err)
	}
	// Status line: exact.
	if refResp.StatusLine != subjResp.StatusLine {
		return Verdict{Equal: false, HexDump: fmt.Sprintf("status: ref=%q subj=%q", refResp.StatusLine, subjResp.StatusLine)}, nil
	}
	// Body: byte-exact.
	bv, err := CompareBytes(refResp.Body, subjResp.Body)
	if err != nil {
		return Verdict{}, err
	}
	if !bv.Equal {
		return bv, nil
	}
	// Headers: set-equal modulo allow-list.
	// Per BEHAVIOR_CONTRACT.md §Admin API — /ready, Task 7 evidence:
	// - Date: value non-deterministic (always present on both, value allow-listed)
	// - Content-Length / Transfer-Encoding: framing deviation (subject: Content-Length:5;
	//   upstream: Transfer-Encoding:chunked). Both are dropped from the set-equal check.
	allowList := map[string]struct{}{
		"Date":              {},
		"Content-Length":    {},
		"Transfer-Encoding": {},
	}
	mismatch := diffHeaders(refResp.Headers, subjResp.Headers, allowList)
	if mismatch != "" {
		return Verdict{Equal: false, HexDump: mismatch}, nil
	}
	return Verdict{Equal: true}, nil
}

func diffHeaders(ref, subj map[string]string, allow map[string]struct{}) string {
	// For each header in ref: if not in allow, require subj has it with equal value.
	var sb strings.Builder
	for k, v := range ref {
		if _, a := allow[k]; a {
			continue
		}
		sv, ok := subj[k]
		if !ok {
			fmt.Fprintf(&sb, "header %q: absent in subj (ref=%q)\n", k, v)
			continue
		}
		if sv != v {
			fmt.Fprintf(&sb, "header %q: ref=%q subj=%q\n", k, v, sv)
		}
	}
	// Reverse: headers in subj but not ref (outside allow-list).
	for k, v := range subj {
		if _, a := allow[k]; a {
			continue
		}
		if _, ok := ref[k]; !ok {
			fmt.Fprintf(&sb, "header %q: absent in ref (subj=%q)\n", k, v)
		}
	}
	return sb.String()
}

func discoverFixtures(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		// No fixtures yet is a valid intermediate state (e.g. between Task 12
		// landing the runner skeleton and Task 13 landing the first fixture).
		return nil
	}
	if err != nil {
		t.Fatalf("read fixtures dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Fixture names start with a 4-digit prefix (NNNN-name).
		if len(e.Name()) >= 5 && isNumeric(e.Name()[:4]) && e.Name()[4] == '-' {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func acceptEchoCounting(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			buf := make([]byte, 4096)
			for {
				n, err := c.Read(buf)
				if n > 0 {
					_, _ = c.Write(buf[:n])
				}
				if err != nil {
					return
				}
			}
		}(c)
	}
}

// acceptHTTPEchoCounting accepts one HTTP/1.1 request per connection and writes
// a canned response body of "backend-<idx>:<lastSegmentOfPath>". Increments
// counter on every accept (mirrors acceptEchoCounting). Settles SPEC §10 #6
// + SPEC §10 #14 (handcrafted bufio + body format).
func acceptHTTPEchoCounting(ln net.Listener, counter *atomic.Uint64, idx int) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			br := bufio.NewReader(c)
			req, err := http.ReadRequest(br)
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, req.Body)
			_ = req.Body.Close()
			seg := req.URL.Path
			if i := strings.LastIndex(seg, "/"); i >= 0 && i+1 < len(seg) {
				seg = seg[i+1:]
			}
			body := fmt.Sprintf("backend-%d:%s", idx, seg)
			_, _ = fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
				len(body), body)
		}(c)
	}
}

// startHTTPSH2Backend spawns the fixture-0004 H/2 backend subprocess on port
// with --cert / --key pointing at the fixture-local PKI's backend-<idx> leaf,
// and BACKEND_IDX env var supplying the idx that flows into response bodies.
// Mirrors fixture-0002's PKI layout: pki/backend-<idx>.pem + pki/backend-<idx>.key.pem.
//
// The backend is a `go run` subprocess (no pre-build step) so the runner does
// not have to manage a binary cache. Backend startup is observed by polling
// the bound port via waitTCPDial — once the TLS-h2 server is accepting
// connections, the test driver can issue requests.
func startHTTPSH2Backend(ctx context.Context, repoRoot string, port, idx int) (*exec.Cmd, error) {
	pkiDir := filepath.Join(repoRoot, "test", "fixtures", "0004-h2-routing", "pki")
	cert := filepath.Join(pkiDir, fmt.Sprintf("backend-%d.pem", idx))
	key := filepath.Join(pkiDir, fmt.Sprintf("backend-%d.key.pem", idx))
	for _, p := range []string{cert, key} {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("pki: stat %s: %w", p, err)
		}
	}
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0004-h2-routing/backends",
		"--port", fmt.Sprintf("%d", port),
		"--cert", cert,
		"--key", key,
	)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), fmt.Sprintf("BACKEND_IDX=%d", idx))
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	// Put the backend (and its `go run` parent) into its own process group so
	// the deferred cleanup can kill the entire group atomically. Without this,
	// killing `go run` leaves the actual backend binary orphaned and holding
	// the test's stderr fd, causing Cmd.WaitDelay to fire on test exit.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	return cmd, nil
}

// startHTTPStatusHeaderBackend spawns the fixture-0005 HTTP/1.1 status-header
// backend subprocess on port. The backend reads the X-Backend-Status request
// header and returns that status code; absent or invalid → 200. No TLS.
// Introduced for fixture 0005's controlled-502 path (ADR-0062).
func startHTTPStatusHeaderBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0005-prometheus-stats/backends",
		"--port", fmt.Sprintf("%d", port),
	)
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	return cmd, nil
}

// waitTCPDial polls addr until a TCP dial succeeds or the timeout elapses.
// Used to observe subprocess-backend readiness without requiring a custom
// stdout sentinel from the backend binary.
func waitTCPDial(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("waitTCPDial: %s did not become reachable within %v", addr, timeout)
		}
		c, err := (&net.Dialer{Timeout: 200 * time.Millisecond}).DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = c.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
