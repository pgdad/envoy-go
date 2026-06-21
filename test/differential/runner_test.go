package differential

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
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
	"sync"
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
	_ "github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0011-http-fault/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0012-http-header-mutation/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0013-http-local-ratelimit/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0014-http-csrf/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0015-http-buffer/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0016-http-compressor/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0017-http-bandwidth-limit/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0019-http-jwt-authn/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0020-http-ext-authz-http/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0021-http-ext-authz-grpc/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0022-http-ext-proc-grpc/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0023-http-ext-proc-body/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0024-http-oauth2/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0025-http-adaptive-concurrency/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0026-http-lua-headers-bridge/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0027-http-lua-full-bridge/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0028-http-lua-multi-script-and-per-route/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0029-http-lua-source-codes-boot-reject/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0030-http-admission-control/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0031-http-admission-control-boot-reject/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0032-http-ratelimit/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0033-http-ratelimit-boot-reject/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0034-http-wasm-headers-bridge/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0035-http-wasm-boot-reject/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0036-http-wasm-body-and-advanced/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0038-http-wasm-perroute-and-multi-plugin/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0039-http-wasm-perroute-boot-reject/inputs"
	_ "github.com/esalaine/envoy-go/test/fixtures/0040-network-echo/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0041-network-direct-response/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0042-network-direct-response-boot-reject/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0043-network-rbac/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0044-network-rbac-boot-reject/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0045-sni-cluster/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0046-zookeeper-requests/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0047-zookeeper-boot-reject/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0048-zookeeper-responses/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0049-mongo-requests/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0050-mongo-boot-reject/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0051-mongo-responses/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0052-mongo-fault-delay/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0053-kafka-requests/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0054-kafka-boot-reject/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0055-redis-roundtrip/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0056-redis-boot-reject/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0057-thrift-roundtrip/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0058-thrift-boot-reject/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0059-lb-least-request/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0060-lb-random/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0061-lb-ring-hash/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0062-lb-ring-hash-http/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0063-lb-maglev/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0064-lb-subset/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0065-weighted-clusters/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0066-health-check-http/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0067-health-check-tcp/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0068-health-check-grpc/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0069-outlier-detection-consecutive-5xx/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0070-outlier-detection-consecutive-gateway-failure/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0071-outlier-detection-local-origin/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0072-outlier-detection-success-rate/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0073-outlier-detection-failure-percentage/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0074-circuit-breaker-max-requests/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0075-retry-loop/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0076-per-try-timeout/driver"
	"github.com/esalaine/envoy-go/test/helpers"

	// Blank-imported so the lua filter's init() boot-registration fires for
	// the differential subject's bootstrap parsing path. Mirrors the existing
	// blank-import discipline for the per-fixture driver packages above.
	// Fixture-0026's driver package lives at
	// test/fixtures/0026-http-lua-headers-bridge/inputs/ and lands at Task 14;
	// this internal-package blank-import lands here at Task 13 so the
	// HTTPLua switch-case + BootRejectFixture infrastructure compile cleanly
	// without a forward-reference to the Task 14 inputs package.
	_ "github.com/esalaine/envoy-go/internal/filter/http/lua"

	// Blank-imported so the ratelimit filter's init() boot-registration fires
	// for the differential subject's bootstrap parsing path. Mirrors the
	// HTTPLua precedent above (the per-fixture inputs packages at
	// test/fixtures/0032-http-ratelimit/inputs/ +
	// test/fixtures/0033-http-ratelimit-boot-reject/inputs/ land at Tasks 10
	// + 11; this internal-package blank-import lands here at Task 9 so the
	// HTTPGlobalRateLimitGRPC switch-case + the Task-11 BootRejectFixture
	// infrastructure compile cleanly without a forward-reference to the
	// later-task inputs packages).
	_ "github.com/esalaine/envoy-go/internal/filter/http/ratelimit"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
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
	uniformKind := fixture.TCPEcho
	if bk, ok := d.(fixture.BackendKindAware); ok {
		uniformKind = bk.BackendKind()
	}
	backends := make([]*backend, n)
	for i := 0; i < n; i++ {
		bo := &backend{idx: i, accepts: new(atomic.Uint64)}
		// Per-index override: drivers implementing fixture.PerHostBackendKind may
		// return a different kind per host index (e.g. 0069's mixed cluster of
		// {2×HTTPEcho healthy, 1×always-503}). The per-iteration local hostKind
		// ensures one index's override does not leak to the next; drivers that do
		// NOT implement the interface keep the uniform default for every host.
		hostKind := uniformKind
		if pk, ok := d.(fixture.PerHostBackendKind); ok {
			hostKind = pk.BackendKindAt(i)
		}
		switch hostKind {
		case fixture.TCPEcho, fixture.HTTPEcho:
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			if hostKind == fixture.TCPEcho {
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
		case fixture.HTTPFixedBody:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPFixedBodyBackend(ctx, root, port)
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
		case fixture.HTTPHello:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPHelloBackend(ctx, root, port)
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
		case fixture.HTTPEchoBody:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPEchoBodyBackend(ctx, root, port)
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
		case fixture.HTTPSlowStream:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPSlowStreamBackend(ctx, root, port)
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
		case fixture.HTTPFault:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPFaultBackend(ctx, root, port)
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
		case fixture.HTTPHeaderMutation:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPHeaderMutationBackend(ctx, root, port)
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
		case fixture.HTTPLocalRateLimit:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPLocalRateLimitBackend(ctx, root, port)
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
		case fixture.HTTPCsrf:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPCsrfBackend(ctx, root, port)
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
		case fixture.HTTPBuffer:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPBufferBackend(ctx, root, port)
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
		case fixture.HTTPCompressor:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPBandwidthLimit:
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPRbac:
			// Fixture 0018-http-rbac (phase 16) reuses the SHARED echobackend
			// binary introduced at phase-14 Task 10 (planner-time decision 12 / D7
			// settlement). Scenarios 5 + 6 + 8 exercise the upstream routes; the
			// remaining scenarios use direct_response and do NOT touch the echo
			// backend (the runner still spawns it because BackendCount() reports
			// the maximum across all scenarios). Because the backend is a
			// subprocess, the runner's in-process accept counter is NOT
			// incremented. The blank-import for the fixture's inputs package
			// lands at Task 12 when the inputs package is authored — at Task 11
			// the BackendKind=HTTPRbac case is wired so the switch is complete
			// for Task 12's fixture rollout.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPJwtAuthn:
			// Fixture 0019-http-jwt-authn (phase 17) reuses the SHARED
			// echobackend binary (phase-14 Task 10) for upstream-echo routes.
			// The in-process JWKS-serving subprocess (test/helpers/jwksbackend/)
			// is lifecycle-managed BY THE DRIVER at Task 11 (it needs to know
			// the per-scenario JWK Set payloads to seed); this switch-case only
			// allocates the upstream echo backend. Plaintext-only per SPEC §7.4
			// (no mTLS in phase 17). Because the backend is a subprocess, the
			// runner's in-process accept counter is NOT incremented. The
			// blank-import for the fixture's inputs package lands at Task 11
			// when the inputs package is authored — at Task 10 the switch-case
			// is wired ahead of the rollout so the BackendKind dispatch is
			// complete.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPExtAuthzHTTP:
			// Fixture 0020-http-ext-authz-http (phase 18.1) reuses the SHARED
			// echobackend binary (phase-14 Task 10) for the upstream route
			// (cluster c_backend). The in-process HTTP auth server
			// (test/helpers/extauthzhttp/) is lifecycle-managed BY THE DRIVER
			// (Task 11) because it needs per-scenario Script configuration;
			// this switch-case only allocates the upstream echo backend.
			// Plaintext-only per SPEC §7.2 + D12 (no TLS in phase 18.1).
			// Because the echo backend runs as a subprocess, the runner's
			// in-process accept counter is NOT incremented.
			// The blank-import for test/fixtures/0020-http-ext-authz-http/inputs
			// lands at Task 11 (the inputs package is now authored).
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPExtAuthzGRPC:
			// Fixture 0021-http-ext-authz-grpc (phase 18.2) reuses the SHARED
			// echobackend binary (phase-14 Task 10) for the upstream route
			// (cluster c_backend). The in-process gRPC auth server
			// (test/helpers/extauthzgrpc/) is lifecycle-managed BY THE DRIVER
			// (Task 10) because it needs per-scenario Script registrations;
			// this switch-case only allocates the upstream echo backend.
			// Plaintext-only per SPEC §7.2 + §11.P13 (no TLS in phase 18.2
			// downstream + h2c-plaintext auth cluster). Because the echo
			// backend runs as a subprocess and the extauthzgrpc helper runs
			// in-process, the runner's in-process accept counter is NOT
			// incremented. The blank-import for
			// test/fixtures/0021-http-ext-authz-grpc/inputs is wired ahead
			// of the rollout at Task 9 so the BackendKind dispatch is
			// complete — Task 10 lands the real driver.go alongside the stub
			// init.go this switch-case currently fires against.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPExtProcGRPC:
			// Fixture 0022-http-ext-proc-grpc (phase 19.1) reuses the SHARED
			// echobackend binary (phase-14 Task 10) for the upstream route
			// (cluster c_backend). The in-process bidi-stream gRPC processor
			// server (test/helpers/extprocgrpc/) is lifecycle-managed BY THE
			// DRIVER (Task 13) because it needs per-scenario Script
			// registrations; this switch-case only allocates the upstream
			// echo backend. Plaintext-only per SPEC §7.2 + parent §8 item 17
			// (no TLS in phase 19.1 downstream + h2c-plaintext processor
			// cluster). Because the echo backend runs as a subprocess and the
			// extprocgrpc helper runs in-process, the runner's in-process
			// accept counter is NOT incremented. Scenario 5 (failure_mode_allow)
			// stops the extprocgrpc server before the request.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPOAuth2:
			// Fixture 0024-http-oauth2 (phase 20) reuses the SHARED
			// echobackend binary (phase-14 Task 10) for the upstream route
			// (cluster c_backend; cookie-passthrough scenarios b1 + b2 +
			// sign-in success leg proxy through this backend). The
			// in-process OAuth 2.0 authorization-server mock
			// (test/helpers/oauthbackend/) is lifecycle-managed BY THE
			// DRIVER (Task 12) because it needs per-scenario Script
			// registrations (token_endpoint POST 200/4xx/5xx variants +
			// authorization_endpoint 302); this switch-case only allocates
			// the upstream echo backend. Plaintext-only per phase-20 SPEC
			// §7 (no TLS in phase 20 downstream + plaintext h2c absent —
			// the token_endpoint POST is HTTP/1.1 per RFC 6749 + ADR-0185).
			// Because the echo backend runs as a subprocess and the
			// oauthbackend helper runs in-process, the runner's in-process
			// accept counter is NOT incremented.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPAdaptiveConcurrency:
			// Fixture 0025-http-adaptive-concurrency (phase 21 Task 10)
			// REUSES the fixture-0010 HTTPSlowStream backend binary at
			// test/fixtures/0010-graceful-drain/backends/backend.go. The
			// slow-stream backend serves "/" as a fast 200 response (body
			// "backend1\n") for scenarios (a) parse_ok, (c) stat_surface,
			// and (d) pass_through_when_disabled, and "/slow" which streams
			// 5 KB over 5 seconds for scenario (b) overflow_503 (two
			// concurrent /slow requests against a listener configured with
			// min_concurrency=1 + max_concurrency_limit=1 cause the second
			// to be rejected with a 503 + "reached concurrency limit" body
			// per AMEND-6). REFERENCE-LESS subject-only fixture per the
			// phase-20 oauth2 + phase-07.1 iteration-probe single-directory
			// precedent. Because the backend is a subprocess, the runner's
			// in-process accept counter is NOT incremented.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPSlowStreamBackend(ctx, root, port)
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
		case fixture.HTTPLua:
			// Fixture 0026-http-lua-headers-bridge (phase 22.1 Task 14)
			// REUSES the SHARED echobackend binary at
			// test/helpers/echobackend/cmd/echobackend/ (phase-14 Task 10).
			// The echobackend reflects request headers as a JSON body —
			// scenarios (a) add_header, (b) replace_header, (c) remove_header,
			// and (f) headers_iter assert the Lua-mutated header set arrived
			// at the upstream by classifying the reflected body. Scenarios
			// (d) respond + (e) log_only do NOT round-trip through the
			// backend ((d) short-circuits at the lua filter; (e) is a
			// no-op log + pass-through). Scenario (g) compile_error never
			// reaches this dispatch — it asserts boot rejection via the
			// OPTIONAL BootRejectFixture driver interface at harness.go
			// + the runBootRejectFixture branch below. Because the backend
			// is a subprocess, the runner's in-process accept counter is
			// NOT incremented. The blank-import for the fixture's inputs
			// package lands at Task 14; at Task 13 this switch-case is
			// wired ahead of the rollout so the BackendKind dispatch is
			// complete for Task 14. Per parent §8.5 + AMEND-11.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPAdmissionControl:
			// Fixtures 0030-http-admission-control (phase 23 Task 9, cross-side
			// 4-scenario) and 0031-http-admission-control-boot-reject (boot-reject)
			// REUSE the fixture-0010 HTTPSlowStream backend at
			// test/fixtures/0010-graceful-drain/backends/backend.go. The slow-
			// stream backend serves GET / with a fast 200 OK response (body
			// "backend1\n", 8 bytes; fixed Content-Length: 8) — the fixed-body
			// guarantee is load-bearing for the cross-side byte-exact comparison
			// in scenario (b) all_admit_healthy: because the body is fixed (NOT
			// an echobackend that reflects request headers), both reference Envoy
			// v1.37.2 and envoy-go produce identical response bodies despite Envoy
			// adding x-forwarded-for and x-request-id headers when forwarding.
			// The admission_control filter admits every request (P_reject=0 for
			// healthy window per AMEND-2 RNG-independence), so both sides pass
			// through identically. The boot-reject fixture (0031) never reaches
			// this backend — the config-load reject fires before the listener
			// binds. Because the backend is a subprocess, the runner's in-process
			// accept counter is NOT incremented.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startHTTPSlowStreamBackend(ctx, root, port)
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
		case fixture.HTTPGlobalRateLimitGRPC:
			// Fixtures 0032-http-ratelimit (phase 24.1 Task 10, cross-side
			// 6-scenario a/b/c/d-core/e/h) and 0033-http-ratelimit-boot-reject
			// (phase 24.1 Task 11, boot-reject) REUSE the SHARED echobackend
			// binary at test/helpers/echobackend/cmd/echobackend/main.go for the
			// upstream route (cluster c_backend). The in-process gRPC rate-limit
			// service (test/helpers/ratelimitgrpc/) is lifecycle-managed BY THE
			// DRIVER (Tasks 10 + 11) because it needs per-scenario Script
			// registrations (OK / OVER_LIMIT / error responses keyed on
			// canonical descriptor-list strings); this switch-case only
			// allocates the upstream echo backend. Plaintext-only per parent
			// SPEC §7.2 (no TLS in phase 24.1 downstream + h2c-plaintext
			// rls cluster). Because the echo backend runs as a subprocess and
			// the ratelimitgrpc helper runs in-process, the runner's in-process
			// accept counter is NOT incremented. The fake's
			// RateLimitResponse encoding obeys D-RL5 / AMEND-6
			// (proto-number-faithful; omit unset optionals) — load-bearing for
			// the cross-side byte-exact OVER_LIMIT comparison. The
			// blank-import for fixtures 0032/0033's inputs packages lands at
			// Tasks 10 + 11; this switch-case + the internal-package blank
			// import below land at Task 9 so the BackendKind dispatch is
			// complete + the ratelimit filter's init() boot-registration fires
			// for the differential subject's bootstrap parsing path ahead of
			// the rollout.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPWasmAdvanced:
			// Fixture 0036-http-wasm-body-and-advanced (phase 25.2 Task 20)
			// REUSES the SHARED echobackend binary at
			// test/helpers/echobackend/cmd/echobackend/ (phase-14 Task 10).
			// The fixture has 2 upstream cluster definitions (cluster_a
			// primary + cluster_b httpCall target) but BOTH point at the
			// SAME backend per phase-22.2 REVIEW §7.4 freeTCPPort flake
			// mitigation — so this switch-case allocates ONE backend that
			// both clusters dial. 14 scenarios per §8.1.1: 10 cross-side
			// via CompareBytes + 4 subject-only via StatsAsserter per
			// reference_differential_asserter_dispatch. Per parent §8.5.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPWasmPerRoute:
			// Fixture 0038-http-wasm-perroute-and-multi-plugin (phase 25.3
			// Task 11) REUSES the SHARED echobackend binary at
			// test/helpers/echobackend/cmd/echobackend/ (phase-14 Task 10).
			// THREE listeners (perroute / multiplugin / reload) all back
			// onto ONE upstream cluster (cluster_a) per phase-22.2 REVIEW
			// §7.4 freeTCPPort flake mitigation — so this switch-case
			// allocates ONE backend. Cross-side arms classify response
			// headers (x-wasm-variant / x-shared) set by the wasm guests;
			// the reload arm is subject-only via StatsAsserter (vm_reload
			// triplet) per reference_differential_asserter_dispatch.
			// Because the backend is a subprocess, the runner's in-process
			// accept counter is NOT incremented.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.HTTPWasm:
			// Fixture 0034-http-wasm-headers-bridge (phase 25.1 Task 15)
			// REUSES the SHARED echobackend binary at
			// test/helpers/echobackend/cmd/echobackend/ (phase-14 Task 10).
			// The echobackend reflects request headers as a JSON body —
			// scenarios (a) add-fixed-header, (b) replace-header, (c)
			// remove-header, (f) header-iteration-count, and (g) property-
			// read-method assert the WASM-mutated header set arrived at
			// the upstream by classifying the reflected body. Scenario
			// (d) respond-shortcircuit does NOT round-trip through the
			// backend (it short-circuits at the wasm filter via
			// proxy_send_local_response). Scenario (e) log-only-
			// passthrough is a no-op log + pass-through; the cross-side
			// stat-counter delta `wasm.<plugin>.executions` is the
			// "wasm ran" assertion + lives in StatsAsserter.AssertStats
			// (mirrors fixture-0026 D3 closure for lua). Because the
			// backend is a subprocess, the runner's in-process accept
			// counter is NOT incremented. Per parent §8.5 + AMEND-A1.
			port := freeTCPPort(t)
			bo.port = port
			cmd, err := startEchoBackend(ctx, root, port)
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
		case fixture.TCPSink:
			// Silent sink backend (28.1 §8.1.1): accept + drain + never write.
			// An echoing backend would push the echoed ZK request bytes back
			// through reference Envoy's onWrite response decoder — counting
			// *_resp/decoder_error increments that envoy-go's 28.1 OnWrite
			// no-op stub never mirrors → cross-side stat divergence (D-S28.1-5).
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptSinkCounting(ln, bo.accepts)
		case fixture.TCPZKResponder:
			// ZooKeeper-aware canned responder (28.2 SPEC §5.1): for every request
			// frame, wait the fixed zkResponderDelay then write a correlated canned
			// response (+ the D-S28.2-2 trigger behaviors). The fixed delay is the
			// deterministic-threshold construction (parent D-P9).
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptZKResponder(ln, bo.accepts)
		case fixture.TCPMongoResponder:
			// MongoDB-aware canned responder (29.2 SPEC §6.1): correlated
			// OP_REPLY/OP_COMMANDREPLY frames so the reference's onWrite response
			// decoder fires + correlates.
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptMongoResponder(ln, bo.accepts)
		case fixture.TCPKafkaResponder:
			// Kafka-aware canned responder (SPEC §8.3): correlation-id-echoing
			// response frames (4-byte BE length + INT32 correlation_id + per-api_key
			// body) so the reference's response-side decoder fires + correlates.
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptKafkaResponder(ln, bo.accepts)
		case fixture.TCPRedisResponder:
			// RESP-aware canned responder (32.1 SPEC §8.3): positional canned replies
			// for the exercised data commands (SET → +OK, GET → bulk "bar"). FIFO/
			// positional — NO correlation id. PING/AUTH never reach the backend
			// (redis_proxy answers them locally — AMEND-R5).
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptRedisResponder(ln, bo.accepts)
		case fixture.TCPThriftResponder:
			// Framed-binary Thrift canned responder (SPEC §8.3): per CALL it echoes a
			// framed-binary REPLY (msgtype 2) carrying the SAME method + RECEIVED seq_id
			// and a void-success body (single STOP 0x00) so the reference's onWrite
			// response decoder fires + classifies response_success. The marker method
			// "boom" yields an EXCEPTION (msgtype 3) reply (D-S33-2 reply-EXCEPTION).
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptThriftResponder(ln, bo.accepts)
		case fixture.GRPCHealthResponder:
			// In-process h2c gRPC-SERVING + plain-H2 200 responder (SPEC §8.2, phase
			// 39.2). Answers grpc.health.v1.Health/Check ⇒ SERVING for the active gRPC
			// HC probe AND returns HTTP 200 + "backend-<idx>:" body for plain data-plane
			// requests so the load-phase 100%-live assertion holds. Host attribution via
			// response body (the 0066 backendIdxFromBody precedent) — no accept counter.
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go serveGRPCHealth(ln, bo.idx)
		case fixture.HTTP503Responder:
			// In-process always-503 HTTP/1.1 responder (phase 40.1, passive outlier
			// detection): reads one request per connection and always answers HTTP 503
			// with a "backend-<idx>:<seg>" body. Used by 0069's mixed cluster so the
			// reference's outlier detector ejects the unhealthy host. Host attribution
			// via response body (the serveGRPCHealth precedent) — no accept counter.
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptHTTP503Counting(ln, bo.idx)
		case fixture.BlockingHoldResponder:
			// In-process HTTP/1.1 responder (phase 41, circuit breaking) that holds
			// each normal "GET /<seg>" request open until a "GET /__release" control
			// request frees the current batch, then answers HTTP 200 with a
			// "backend-<idx>:<seg>" body. Used by 0074 to deterministically fill the
			// max_requests budget before probing the breaker. Host attribution via
			// response body (the acceptHTTP503Counting precedent) — no accept counter.
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptBlockingHold(ln, bo.idx)
		}
		backends[i] = bo
	}
	backendPorts := make([]int, n)
	for i, b := range backends {
		backendPorts[i] = b.port
	}

	// 1b. Reference-less fast path. Drivers that implement
	// fixture.ReferenceLessFixture and return false from RequiresReference()
	// signal to the runner that this fixture has no reference Envoy
	// counterpart (e.g. fixture 0007b-iteration-probe — envoy-go-only
	// structural assertion of the iteration-protocol state machine via the
	// hand-rolled envoy.filters.http.envoy_go_test probe filter, which does
	// not exist in upstream Envoy). The runner SKIPS reference-proxy spawn,
	// SKIPS DriveReference, SKIPS the byte-stream CompareBytes, and SKIPS
	// the admin probe diff. Only DriveSubject + the optional SubjectAsserter
	// run. SPEC §7.4 + ADR-0074. Pre-existing fixtures 0000-0007a do NOT
	// implement ReferenceLessFixture, so they default to RequiresReference()
	// = true and stay on the differential path unchanged.
	if rl, ok := d.(fixture.ReferenceLessFixture); ok && !rl.RequiresReference() {
		runReferenceLessFixture(ctx, t, root, d, backendPorts)
		return
	}

	// 1c. Boot-reject fast path. Drivers that implement
	// differential.BootRejectFixture signal to the runner that BOTH
	// reference + subject MUST reject boot when fed the broken script path
	// returned by BootRejectScript(). The runner asserts both sides exit
	// non-zero AND both sides' stderr contains the substring returned by
	// ExpectedBootErrorSubstring(). Per parent §13-R1 + §11.7.3 + AMEND-10
	// option 2 + 22.1 SPEC §6 Task 13. Used by fixture
	// 0026-http-lua-headers-bridge scenario (g) g_compile_error (driver
	// lands at Task 14). Pre-existing fixtures 0000-0025 do NOT implement
	// BootRejectFixture so they default to bypassing this branch and stay
	// on the differential / reference-less paths unchanged.
	if brf, ok := d.(BootRejectFixture); ok {
		runBootRejectFixture(ctx, t, root, pin, d, brf, backendPorts)
		return
	}

	// 2. Reference proxy. If the driver implements ReferenceLogMounter, pre-create
	// the host-side files and pass bind-mounts to StartReferenceProxyWithMounts.
	// Bind mounts use HostConfig.Binds (not ContainerMounts) because testcontainers
	// v0.27.0 silently drops MountTypeBind entries in mapToDockerMounts.
	//
	// Phase 07.2 (Task 14): if the driver implements fixture.MultiListenerDriver,
	// the reference container exposes ALL of its ReferenceListenerPorts() (>=2)
	// instead of the single ReferenceListenerPort(). The single-addr fallback
	// (else branch below) is unchanged for pre-existing fixtures (0000-0007b)
	// which do not implement MultiListenerDriver.
	bootstrap := d.ReferenceBootstrap(backendPorts)
	mld, isMulti := d.(fixture.MultiListenerDriver)
	var refPorts []int
	if isMulti {
		refPorts = mld.ReferenceListenerPorts()
	} else {
		refPorts = []int{d.ReferenceListenerPort()}
	}
	var ref *ReferenceProxy
	var err error
	if rlm, ok := d.(fixture.ReferenceLogMounter); ok {
		hostMounts := rlm.ReferenceHostMounts()
		for _, hm := range hostMounts {
			// Pre-create the host file so Docker bind-mounts a file (not a dir).
			f, ferr := os.OpenFile(hm.HostPath, os.O_CREATE|os.O_WRONLY, 0o666)
			if ferr != nil {
				t.Fatalf("ref mount pre-create %s: %v", hm.HostPath, ferr)
			}
			_ = f.Close()
			if ferr = os.Chmod(hm.HostPath, 0o666); ferr != nil {
				t.Fatalf("ref mount chmod %s: %v", hm.HostPath, ferr)
			}
		}
		ref, err = StartReferenceProxyWithMounts(ctx, pin, bootstrap, hostMounts, refPorts...)
	} else {
		ref, err = StartReferenceProxy(ctx, pin, bootstrap, refPorts...)
	}
	if err != nil {
		t.Fatalf("ref start: %v", err)
	}
	defer func() { _ = ref.Stop(context.Background()) }()
	refAddr := ref.ListenerAddr(d.ReferenceListenerPort())

	// 3. Subject proxy.
	//
	// Multi-listener fixtures derive ports as subjPort+1..+N without reserving
	// them, so a freshly-allocated base port can collide with a port grabbed in
	// the window between freeTCPPort's close and envoy-go's bind. Retry the whole
	// allocate→render→start sequence on bind collisions (seen on CI as
	// "address already in use" followed by "subject ready: EOF").
	//
	// We retry on any start error (bounded at 3) because the collision is not
	// classifiable from the returned error — the subject dies before the ready
	// sentinel, so all that surfaces is "subject ready: EOF".
	var subj *SubjectProxy
	for attempt := 1; ; attempt++ {
		subjPort := freeTCPPort(t)
		subjAdminPort := freeTCPPort(t)
		subjCfg := d.SubjectConfig(d.ReferenceListenerPort(), subjPort, backendPorts, subjAdminPort)
		var serr error
		subj, serr = StartSubjectProxy(ctx, root, subjCfg, fmt.Sprintf("127.0.0.1:%d", subjAdminPort))
		if serr == nil {
			break
		}
		if attempt >= 3 {
			t.Fatalf("subj start (attempt %d): %v", attempt, serr)
		}
		t.Logf("subj start attempt %d failed (%v); retrying with fresh ports", attempt, serr)
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

	// 5. Drive ref. Phase 07.2 (Task 14): if the driver implements
	// fixture.MultiListenerDriver, build a name->addr map across ALL listener
	// names and dispatch DriveReferenceMulti instead of the single-addr Drive.
	// Pre-existing fixtures (0000-0007b) do not implement MultiListenerDriver
	// and fall through to the single-addr DriveReference path unchanged.
	var refBytes []byte
	if isMulti {
		refAddrs := map[string]string{}
		names := mld.SubjectListenerNames()
		ports := mld.ReferenceListenerPorts()
		if len(names) != len(ports) {
			t.Fatalf("MultiListenerDriver: SubjectListenerNames()=%d != ReferenceListenerPorts()=%d", len(names), len(ports))
		}
		for i, name := range names {
			refAddrs[name] = ref.ListenerAddr(ports[i])
		}
		refBytes, err = mld.DriveReferenceMulti(ctx, refAddrs)
	} else {
		refBytes, err = d.DriveReference(ctx, refAddr)
	}
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

	// 6. Drive subj. Phase 07.2 (Task 14): if the driver implements
	// fixture.MultiListenerDriver, build a name->addr map across ALL subject
	// listener names and dispatch DriveSubjectMulti instead of the single-addr
	// Drive. Pre-existing fixtures (0000-0007b) fall through unchanged.
	var subjBytes []byte
	if isMulti {
		subjAddrs := map[string]string{}
		for _, name := range mld.SubjectListenerNames() {
			subjAddrs[name] = subj.ListenerAddr(name)
		}
		subjBytes, err = mld.DriveSubjectMulti(ctx, subjAddrs)
	} else {
		subjBytes, err = d.DriveSubject(ctx, subj.ListenerAddr(d.SubjectListenerName()))
	}
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

	// 11. Optional access-log equivalence assertion (phase 06.2, ADR-0068). Fires
	// when the driver implements fixture.AccessLogAsserter. The driver holds its
	// own per-side log-file paths (set during SubjectConfig / ReferenceBootstrap)
	// and performs the per-record three-tier assertion in-band.
	if ala, ok := d.(fixture.AccessLogAsserter); ok {
		ala.AssertAccessLog(t)
	}

	// 12. Optional alternate-config diff (phase 07.2, Task 14). Fires when the
	// driver implements fixture.AlternateConfigDriver. The runner spawns a SECOND
	// ref+subj pair using AlternateReferenceBootstrap + AlternateSubjectConfig,
	// drives the alternate listener via DriveAlternate, and accepts the returned
	// bytes — the driver performs in-band assertion (mirrors the SubjectAsserter
	// + StatsAsserter + AccessLogAsserter precedent: SPEC §12 #8). fixture-0008
	// uses this for the c4 variant where chain_other is removed to exercise the
	// default_filter_chain fallback. Pre-existing fixtures (0000-0007b) do not
	// implement AlternateConfigDriver and skip this branch unchanged.
	if acd, ok := d.(fixture.AlternateConfigDriver); ok {
		altRefBootstrap := acd.AlternateReferenceBootstrap(backendPorts)
		altRefPort := acd.AlternateReferenceListenerPort()
		altRef, err := StartReferenceProxy(ctx, pin, altRefBootstrap, altRefPort)
		if err != nil {
			t.Fatalf("alt ref start: %v", err)
		}
		defer func() { _ = altRef.Stop(context.Background()) }()
		altSubjPort := freeTCPPort(t)
		altSubjAdminPort := freeTCPPort(t)
		altSubjCfg := acd.AlternateSubjectConfig(altRefPort, altSubjPort, backendPorts, altSubjAdminPort)
		altSubj, err := StartSubjectProxy(ctx, root, altSubjCfg, fmt.Sprintf("127.0.0.1:%d", altSubjAdminPort))
		if err != nil {
			t.Fatalf("alt subj start: %v", err)
		}
		defer func() { _ = altSubj.Stop() }()
		altRefAddr := altRef.ListenerAddr(altRefPort)
		altSubjAddr := altSubj.ListenerAddr(acd.AlternateSubjectListenerName())
		altBytes, err := acd.DriveAlternate(ctx, altRefAddr, altSubjAddr)
		if err != nil {
			t.Fatalf("DriveAlternate: %v", err)
		}
		// DriveAlternate returns one byte slice the driver produced after
		// driving both ref+subj sides; the diff is intrinsic to the driver's
		// logic (in-band per SubjectAsserter precedent). The runner only
		// surfaces the error path here.
		_ = altBytes
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
		// Fixture names start with a 4-digit prefix optionally followed by a
		// single lowercase letter, then '-' and the fixture name. Examples:
		// "0006-access-log" (4-digit only), "0007a-cors" (4-digit + 'a').
		// The optional-letter form was introduced by phase 07.1's split into
		// 0007a-cors (differential) + 0007b-iteration-probe (structural).
		name := e.Name()
		if len(name) >= 5 && isNumeric(name[:4]) {
			// Bare 4-digit prefix: "0006-..."
			if name[4] == '-' {
				names = append(names, name)
				continue
			}
			// 4-digit + letter prefix: "0007a-..."
			if len(name) >= 6 && isLowerLetter(name[4]) && name[5] == '-' {
				names = append(names, name)
				continue
			}
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

// isLowerLetter reports whether b is in 'a'..'z'. Used by discoverFixtures to
// recognize the phase-07.1 split-prefix shape "NNNN<letter>-name".
func isLowerLetter(b byte) bool {
	return b >= 'a' && b <= 'z'
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

// acceptSinkCounting accepts connections, counts them, drains all reads, and
// NEVER writes (the TCPSink backend — 28.1 §8.1.1; D-S28.1-5 read-until-EOF).
// A silent sink is required for 0046-zookeeper-requests: an echoing backend
// would push the echoed ZK request bytes back through reference Envoy's onWrite
// response decoder, counting *_resp/decoder_error increments that envoy-go's
// 28.1 OnWrite no-op stub never mirrors → cross-side stat divergence.
func acceptSinkCounting(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			_, _ = io.Copy(io.Discard, c)
		}(c)
	}
}

// zkResponderDelay is the TCPZKResponder fixed pre-response delay (D-S28.2-2:
// 10 ms — 10x the 0048 slow-arm 1ms threshold, so every measured latency is
// deterministically ≥ the delay on both sides; parent D-P9).
const zkResponderDelay = 10 * time.Millisecond

// TCPZKResponder trigger opcodes (D-S28.2-2). The responder peeks the request
// frame's opcode int (bytes 4-8) for data requests only.
const (
	zkTriggerWrongXid  int32 = 6 // getacl → respond with xid+1000 (uncorrelated → decoder_error)
	zkTriggerWatchPush int32 = 3 // exists → normal response + unsolicited watch-event push
)

// acceptZKResponder accepts connections, counts them, and runs the ZooKeeper-aware
// canned-response loop on each (the TCPZKResponder backend — 28.2 SPEC §5.1; the
// acceptSinkCounting sibling). The responder parses ONLY the request frame's
// length prefix + leading xid + (for data requests) the opcode int; it is NOT a
// ZooKeeper server (no session/watch semantics).
func acceptZKResponder(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go zkRespondLoop(c)
	}
}

// zkRespondLoop reads request frames and writes canned responses until the
// client closes (read error / EOF). zxid is monotonic per connection.
func zkRespondLoop(c net.Conn) {
	defer func() { _ = c.Close() }()
	be32 := func(v int32) []byte {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(v))
		return b
	}
	be64 := func(v int64) []byte {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(v))
		return b
	}
	writeFrame := func(payload []byte) bool {
		out := append(be32(int32(len(payload))), payload...)
		_, err := c.Write(out)
		return err == nil
	}
	var zxid int64
	for {
		var lenBuf [4]byte
		if _, err := io.ReadFull(c, lenBuf[:]); err != nil {
			return // client closed / EOF
		}
		frameLen := int32(binary.BigEndian.Uint32(lenBuf[:]))
		if frameLen < 4 || frameLen > 1<<20 {
			return // malformed / hostile — drop the connection
		}
		frame := make([]byte, frameLen)
		if _, err := io.ReadFull(c, frame); err != nil {
			return
		}
		xid := int32(binary.BigEndian.Uint32(frame[0:4]))

		// The fixed pre-response delay (every response, triggers included).
		time.Sleep(zkResponderDelay)

		if xid == 0 {
			// Connect request → canned connect response (20 bytes):
			// proto_version(0) + timeout(30000) + session_id + password(len 0).
			resp := append(append(append(be32(0), be32(30000)...), be64(0x5A5A)...), be32(0)...)
			if !writeFrame(resp) {
				return
			}
			continue
		}

		// Data/control request → standard response: xid(echoed) + zxid(8,
		// monotonic) + error(4, 0) = 16 bytes. Triggers adjust.
		opcode := int32(0)
		if len(frame) >= 8 {
			opcode = int32(binary.BigEndian.Uint32(frame[4:8]))
		}
		zxid++
		respXid := xid
		if opcode == zkTriggerWrongXid {
			respXid = xid + 1000 // the wrong-xid trigger (D-S28.2-2)
		}
		resp := append(append(be32(respXid), be64(zxid)...), be32(0)...)
		if !writeFrame(resp) {
			return
		}
		if opcode == zkTriggerWatchPush {
			// The watch-event push trigger (D-S28.2-2): an unsolicited
			// watch-event frame after the normal response, in the FULL
			// ReplyHeader format (D-S28.2-1 — upstream parseWatchEvent):
			// xid(−1) + zxid(8) + error(0) + event_type(1=created) +
			// client_state(3=connected) + path-len + path.
			zxid++
			path := []byte("/zk-watch")
			push := append(append(append(append(append(
				be32(-1), be64(zxid)...), be32(0)...), be32(1)...), be32(3)...),
				append(be32(int32(len(path))), path...)...)
			if !writeFrame(push) {
				return
			}
		}
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

// acceptHTTP503Counting accepts one HTTP/1.1 request per connection and always
// writes a 503 Service Unavailable response with a "backend-<idx>:<lastSegmentOf
// Path>" body (host attribution via body, the serveGRPCHealth precedent — NO
// accept counter). Used by 0069's mixed cluster so the reference's passive
// outlier detector ejects the always-failing host.
func acceptHTTP503Counting(ln net.Listener, idx int) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
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
			_, _ = fmt.Fprintf(c, "HTTP/1.1 503 Service Unavailable\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
				len(body), body)
		}(c)
	}
}

// acceptBlockingHold accepts one HTTP/1.1 request per connection and HOLDS each
// normal "GET /<seg>" request on a shared gate channel until a "GET /__release"
// control request closes-and-swaps the gate (freeing the current batch and
// re-arming for the next), then answers HTTP 200 with a "backend-<idx>:<seg>"
// body (host attribution via body, the acceptHTTP503Counting precedent — NO
// accept counter). Used by 0074 to deterministically fill the max_requests
// budget before probing the circuit breaker.
func acceptBlockingHold(ln net.Listener, idx int) {
	var mu sync.Mutex
	gate := make(chan struct{}) // closed by /__release to free the current batch
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			br := bufio.NewReader(c)
			req, err := http.ReadRequest(br)
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, req.Body)
			_ = req.Body.Close()
			if req.URL.Path == "/__release" {
				mu.Lock()
				old := gate
				gate = make(chan struct{}) // re-arm for the next batch
				mu.Unlock()
				close(old) // free everyone currently held
				body := "released"
				_, _ = fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
				return
			}
			// a normal request: block until the current batch is released.
			mu.Lock()
			g := gate
			mu.Unlock()
			<-g
			seg := req.URL.Path
			if i := strings.LastIndex(seg, "/"); i >= 0 && i+1 < len(seg) {
				seg = seg[i+1:]
			}
			body := fmt.Sprintf("backend-%d:%s", idx, seg)
			_, _ = fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
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

// startHTTPFixedBodyBackend spawns the fixture-0006 HTTP/1.1 fixed-body backend
// subprocess on port. The backend returns 200 OK with a fixed 17-byte body
// "backend:v1/fixed\n" for any GET request, regardless of path or backend index.
// No TLS. Introduced for fixture 0006's BYTES_SENT Tier-E equality (ADR-0068):
// byte-identical body length across all endpoints keeps BYTES_SENT equal despite
// RR divergence. Because the backend is a subprocess, the runner's in-process
// accept counter is NOT incremented.
func startHTTPFixedBodyBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0006-access-log/backends",
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

// startHTTPHelloBackend spawns the fixture-0007a HTTP/1.1 hello backend
// subprocess on port. The backend returns 200 OK with body "hello\n" (6 bytes)
// for any request regardless of method or path. No TLS. Introduced for
// fixture 0007a-cors (Task 21) for actual-request body byte-equivalence on
// the cors differential. Because the backend is a subprocess, the runner's
// in-process accept counter is NOT incremented.
func startHTTPHelloBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0007a-cors/backends",
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

// startHTTPEchoBodyBackend spawns the fixture-0007b HTTP/1.1 echo-body backend
// subprocess on port. The backend returns 200 OK with the request body if
// non-empty, else with the fixed 8-byte body "backend\n". No TLS. Introduced
// for fixture 0007b-iteration-probe (Task 22) so the iteration-protocol
// structural fixture's modify-encode-data mode can verify in-place body
// mutation against an echoed payload, and the no-body modes have a stable
// baseline body. Because the backend is a subprocess, the runner's in-process
// accept counter is NOT incremented.
func startHTTPEchoBodyBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0007b-iteration-probe/backends",
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

// startHTTPSlowStreamBackend spawns the fixture-0010 HTTP/1.1 slow-stream backend
// subprocess on port. The backend serves /slow which streams 5KB at 1KB/s
// (5s total), and / which returns 200 OK with body "backend1\n". No TLS.
// Introduced for fixture 0010-graceful-drain (phase 08.2 Task 12) for the
// stable 5s in-flight window needed by the graceful-drain differential.
// Because the backend is a subprocess, the runner's in-process accept counter
// is NOT incremented.
func startHTTPSlowStreamBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0010-graceful-drain/backends",
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

// startHTTPFaultBackend spawns the fixture-0011 HTTP/1.1 backend subprocess
// on port. The backend serves / with body "backend\n" (8 bytes). No TLS.
// Introduced for fixture 0011-http-fault (phase 09 Task 10) to provide the
// deterministic-body backend the per-scenario equivalence assertions expect.
// Because the backend is a subprocess, the runner's in-process accept counter
// is NOT incremented.
func startHTTPFaultBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0011-http-fault/backends",
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

// startHTTPHeaderMutationBackend spawns test/fixtures/0012-http-header-mutation/
// backends/backend.go on the runner-allocated port. Mirrors startHTTPFaultBackend.
func startHTTPHeaderMutationBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0012-http-header-mutation/backends",
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

// startHTTPLocalRateLimitBackend spawns test/fixtures/0013-http-local-ratelimit/
// backends/backend.go on the runner-allocated port. Mirrors startHTTPHeaderMutationBackend.
func startHTTPLocalRateLimitBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0013-http-local-ratelimit/backends",
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

// startHTTPCsrfBackend spawns test/fixtures/0014-http-csrf/
// backends/backend.go on the runner-allocated port. Mirrors startHTTPLocalRateLimitBackend.
func startHTTPCsrfBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0014-http-csrf/backends",
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

// startHTTPBufferBackend spawns test/fixtures/0015-http-buffer/
// backends/backend.go on the runner-allocated port. Mirrors startHTTPCsrfBackend.
func startHTTPBufferBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0015-http-buffer/backends",
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

// startEchoBackend spawns the SHARED test/helpers/echobackend/cmd/echobackend
// binary on the runner-allocated port. Used by fixture.HTTPCompressor (phase 14
// fixture 0016) and any future fixture wiring fixture.HTTPCompressor. Mirrors
// startHTTPBufferBackend modulo the binary path. Introduced by phase 14 Task 10
// per planner-time decision 12 (D7 settlement) — the shared echobackend helper
// at test/helpers/echobackend/ replaces the per-fixture-backend pattern for
// echo-style upstreams.
func startEchoBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/helpers/echobackend/cmd/echobackend",
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

// runReferenceLessFixture is the runner's per-fixture loop variant for drivers
// that implement fixture.ReferenceLessFixture and return false from
// RequiresReference(). It spawns ONLY the subject proxy, drives subject, and
// invokes the driver's SubjectAsserter (in-band assertion per SPEC §12 #8).
// No reference-Envoy container is spawned; no byte-stream comparison runs;
// no admin diff fires. Used by fixture 0007b-iteration-probe (Task 22) for
// the envoy-go-only iteration-protocol structural assertion.
//
// The reference-related arguments to d.SubjectConfig are zero-valued (the
// fixture has no reference, so refListenerPort = 0). Drivers that go through
// this branch MUST tolerate that — 0007b's SubjectConfig template ignores
// the refListenerPort argument.
func runReferenceLessFixture(ctx context.Context, t *testing.T, root string, d FixtureDriver, backendPorts []int) {
	t.Helper()
	// Subject proxy.
	// NOTE: no bind-collision retry here (see runFixture's subject-start retry
	// loop); extend it to this site if the 0036/0034-style flake shape ever
	// appears in this runner.
	subjPort := freeTCPPort(t)
	subjAdminPort := freeTCPPort(t)
	subjCfg := d.SubjectConfig(0, subjPort, backendPorts, subjAdminPort)
	subj, err := StartSubjectProxy(ctx, root, subjCfg, fmt.Sprintf("127.0.0.1:%d", subjAdminPort))
	if err != nil {
		t.Fatalf("subj start: %v", err)
	}
	defer func() { _ = subj.Stop() }()

	// Drive subj. The driver's DriveSubject returns whatever bytes it captures
	// per-mode; the in-band SubjectAsserter inspects the captured bytes
	// against the embedded expectation table.
	subjBytes, err := d.DriveSubject(ctx, subj.ListenerAddr(d.SubjectListenerName()))
	if err != nil {
		t.Fatalf("subj drive: %v", err)
	}

	// In-band per-mode assertion. If the driver does NOT implement
	// SubjectAsserter, the runner only validates that DriveSubject returned
	// without error — the structural assertion lives entirely in DriveSubject
	// in that case (driver-internal t.Errorf via captured *testing.T).
	if sa, ok := d.(fixture.SubjectAsserter); ok {
		sa.AssertSubject(t, subjBytes)
	}
}

// runBootRejectFixture is the runner's per-fixture loop variant for drivers
// that implement differential.BootRejectFixture (per parent §13-R1 + §11.7.3
// + 22.1 SPEC §6 Task 13 + AMEND-10 option 2). Used by fixture
// 0026-http-lua-headers-bridge scenario (g) g_compile_error.lua at Task 14.
//
// Discipline:
//  1. Renders the reference + subject bootstraps via the driver's existing
//     ReferenceBootstrap + SubjectConfig templates. The driver is responsible
//     for splicing BootRejectScript() into those templates as the lua
//     filter's DataSource Filename source (the same template path the
//     non-reject scenarios use, just pointing at the intentionally-broken
//     script).
//  2. Calls tryStartReferenceProxy + tryStartSubjectProxy (NOT the
//     t.Fatalf-on-failure StartReferenceProxy / StartSubjectProxy variants).
//  3. Asserts BOTH calls return a non-nil err (boot rejection on both sides).
//     If either succeeds (i.e., boot DID NOT reject), t.Fatalf — the driver's
//     broken script failed to reject.
//  4. Asserts BOTH captured stderr buffers contain ExpectedBootErrorSubstring()
//     as a SUBSTRING (case-sensitive, anywhere in stderr — matches AMEND-10
//     option 2 cross-side carve-out at parent §13.7).
//
// Substring match discipline: case-sensitive `strings.Contains` on the full
// captured stderr buffer for each side. NOT a prefix match, NOT a regex, NOT
// case-insensitive. Per parent §11.7.5 the upstream wording is
// `"script load error: <detail>"` from
// source/extensions/filters/common/lua/lua.cc; the envoy-go-side wrapping
// `"script load error"` substring landing happens at Task 15
// cmd/envoy-go/main.go. The substring assertion does NOT pin the bytes
// AROUND the substring (gopher-lua vs LuaJIT detail wording diverges per
// AMEND-9; the wire NEVER carries the detail string).
//
// The BackendPorts argument is currently unused — the boot-reject fires
// BEFORE the listener binds, so the backend never receives a request. The
// argument is threaded through for symmetry with the differential and
// reference-less branches; future boot-reject fixtures may need backend
// addressability (e.g., the bootstrap references a backend cluster port).
func runBootRejectFixture(ctx context.Context, t *testing.T, root string, pin *EnvoyPin, d FixtureDriver, brf BootRejectFixture, backendPorts []int) {
	t.Helper()
	_ = brf.BootRejectScript() // driver-side: spliced into the bootstrap templates by ReferenceBootstrap / SubjectConfig.
	wantSubstring := brf.ExpectedBootErrorSubstring()
	if wantSubstring == "" {
		t.Fatalf("BootRejectFixture: ExpectedBootErrorSubstring() returned empty string — substring assertion requires a non-empty needle")
	}

	// Subject-only dispatch — per 25.2 fixture-0037 + SubjectOnlyBootRejectFixture
	// sibling interface at harness.go. The reference Envoy v1.37.2 MUST boot
	// SUCCESSFULLY (the trigger is an envoy-go-strict-only validator with no
	// upstream-equivalent; the unknown extension field is silently dropped by
	// upstream's protobuf parser), and only the subject envoy-go boot-REJECTS
	// with the substring in stderr. Per D-25.2-P1 closure at 25.2 IMPL Task 21
	// first-action + reference_differential_fixture_dispatch_constraint (one
	// fixture dir = ONE runner branch — fixture-0037 occupies this branch).
	subjectOnly := false
	if sob, ok := d.(SubjectOnlyBootRejectFixture); ok {
		subjectOnly = sob.SubjectOnly()
	}

	// Reference side — render the bootstrap then try to start it. If the
	// driver implements fixture.ReferenceLogMounter, pre-create the host-side
	// files + pass bind-mounts to tryStartReferenceProxy (needed by subject-
	// only boot-reject fixtures whose reference MUST boot successfully — its
	// wasm filter needs the on-disk .wasm blob inside the container).
	bootstrap := d.ReferenceBootstrap(backendPorts)
	refPort := d.ReferenceListenerPort()
	var hostMounts []fixture.HostMount
	if rlm, ok := d.(fixture.ReferenceLogMounter); ok {
		hostMounts = rlm.ReferenceHostMounts()
		for _, hm := range hostMounts {
			// Pre-create the host file ONLY if it does not already exist;
			// 0037's bind-mount points at a real (pre-existing) .wasm blob
			// borrowed from fixture-0036/bytecode and we MUST NOT truncate it.
			// 0029/0031/0033/0035 boot-reject fixtures do not implement
			// ReferenceLogMounter so this branch is moot for them.
			if _, statErr := os.Stat(hm.HostPath); statErr == nil {
				continue
			}
			f, ferr := os.OpenFile(hm.HostPath, os.O_CREATE|os.O_WRONLY, 0o666)
			if ferr != nil {
				t.Fatalf("ref mount pre-create %s: %v", hm.HostPath, ferr)
			}
			_ = f.Close()
			if ferr = os.Chmod(hm.HostPath, 0o666); ferr != nil {
				t.Fatalf("ref mount chmod %s: %v", hm.HostPath, ferr)
			}
		}
	}
	refCancel, refStderr, refErr := tryStartReferenceProxy(ctx, pin, bootstrap, hostMounts, refPort)
	if subjectOnly {
		// Subject-only discipline: the reference MUST boot SUCCESSFULLY.
		// If the reference rejected boot, that means our "reference accepts
		// the unknown extension field silently" assumption is wrong — fail
		// the test with the captured stderr for diagnostic.
		if refCancel != nil {
			// Reference DID come up — tear it down + continue to subject-side.
			refCancel()
		}
		if refErr != nil {
			t.Fatalf("SubjectOnlyBootRejectFixture: reference proxy FAILED to boot — expected SUCCESS (the trigger is an envoy-go-strict-only validator with no upstream-equivalent; the unknown extension field should be silently dropped by upstream's protobuf parser)\n--- reference err: %v\n--- reference stderr (%d bytes):\n%s", refErr, refStderr.Len(), refStderr.String())
		}
	} else {
		// Symmetric discipline (fixture-0026/0029/0031/0033/0035 precedent):
		// the reference MUST boot-reject with the substring in stderr.
		if refCancel != nil {
			// Surprising success path: the reference DID come up. Tear it
			// down + fail the test — the broken script failed to reject.
			refCancel()
		}
		if refErr == nil {
			t.Fatalf("BootRejectFixture: reference proxy started cleanly — expected boot rejection for broken script %q", brf.BootRejectScript())
		}
	}

	// Subject side — render the subject config then try to start it. The
	// subject's listener port is freshly allocated (the boot-reject fires
	// before the listener binds; the port value is supplied for template
	// completeness but never consumed).
	//
	// NOTE: a bind-collision retry (runFixture's subject-start retry loop) must
	// NEVER be added here — this path asserts that the start FAILS, and a retry
	// loop would mask the expected boot rejection.
	subjPort := freeTCPPort(t)
	subjAdminPort := freeTCPPort(t)
	subjCfg := d.SubjectConfig(refPort, subjPort, backendPorts, subjAdminPort)
	subjCancel, subjStderr, subjErr := tryStartSubjectProxy(ctx, root, subjCfg, fmt.Sprintf("127.0.0.1:%d", subjAdminPort))
	if subjCancel != nil {
		// Surprising success path: the subject DID come up. Tear it down
		// + fail the test — the broken script failed to reject.
		subjCancel()
	}
	if subjErr == nil {
		t.Fatalf("BootRejectFixture: subject proxy started cleanly — expected boot rejection for broken script %q", brf.BootRejectScript())
	}

	// Substring assertions — case-sensitive Contains against stderr buffers.
	// For subject-only discipline only the subject stderr is checked (the
	// reference booted successfully so there's no error wording to match).
	// For symmetric discipline both stderr buffers are checked. The captured
	// boot-reject error string from tryStart* (refErr / subjErr) is
	// informational only; the AMEND-10 option 2 carve-out asserts the
	// substring in stderr, not in the error string.
	if !subjectOnly {
		if !strings.Contains(refStderr.String(), wantSubstring) {
			t.Fatalf("BootRejectFixture: reference stderr does NOT contain %q\n--- reference err: %v\n--- reference stderr (%d bytes):\n%s", wantSubstring, refErr, refStderr.Len(), refStderr.String())
		}
	}
	if !strings.Contains(subjStderr.String(), wantSubstring) {
		t.Fatalf("BootRejectFixture: subject stderr does NOT contain %q\n--- subject err: %v\n--- subject stderr (%d bytes):\n%s", wantSubstring, subjErr, subjStderr.Len(), subjStderr.String())
	}
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

// TestZKResponderBackend unit-tests the TCPZKResponder accept loop against a raw
// TCP client (no proxies): canned connect response, xid-echoed standard
// responses with the fixed pre-response delay, the wrong-xid trigger (getacl),
// and the watch-event-push trigger (exists). This proves the backend BEFORE the
// docker-dependent 0048 fixture consumes it (Task 9).
func TestZKResponderBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	var accepts atomic.Uint64
	go acceptZKResponder(ln, &accepts)

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	readFrame := func() []byte {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var lenBuf [4]byte
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			t.Fatalf("read frame length: %v", err)
		}
		frame := make([]byte, binary.BigEndian.Uint32(lenBuf[:]))
		if _, err := io.ReadFull(conn, frame); err != nil {
			t.Fatalf("read frame body: %v", err)
		}
		return frame
	}
	writeFrame := func(payload []byte) {
		t.Helper()
		var lenBuf [4]byte
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
		if _, err := conn.Write(append(lenBuf[:], payload...)); err != nil {
			t.Fatal(err)
		}
	}
	be32 := func(v int32) []byte {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(v))
		return b
	}

	// 1. Connect request (leading int32 == 0) → 20-byte connect response, after ≥ the fixed delay.
	start := time.Now()
	connectReq := append(append(append(append(be32(0), make([]byte, 8)...), be32(30000)...), make([]byte, 8)...), be32(0)...)
	writeFrame(connectReq)
	resp := readFrame()
	if elapsed := time.Since(start); elapsed < zkResponderDelay {
		t.Fatalf("connect response arrived after %v, want >= %v (the fixed-delay discipline)", elapsed, zkResponderDelay)
	}
	if len(resp) != 20 || int32(binary.BigEndian.Uint32(resp[0:4])) != 0 {
		t.Fatalf("connect response: len=%d leading=%d, want len=20 leading=0", len(resp), int32(binary.BigEndian.Uint32(resp[0:4])))
	}

	// 2. Data request (getdata, xid 7) → standard 16-byte response echoing xid 7.
	writeFrame(append(append(be32(7), be32(4)...), be32(0)...)) // xid 7, op getdata(4), path-len 0
	resp = readFrame()
	if len(resp) != 16 || int32(binary.BigEndian.Uint32(resp[0:4])) != 7 {
		t.Fatalf("data response: len=%d xid=%d, want len=16 xid=7", len(resp), int32(binary.BigEndian.Uint32(resp[0:4])))
	}

	// 3. Wrong-xid trigger: getacl (op 6, xid 9) → response carries xid 9+1000.
	writeFrame(append(append(be32(9), be32(6)...), be32(0)...))
	resp = readFrame()
	if got := int32(binary.BigEndian.Uint32(resp[0:4])); got != 1009 {
		t.Fatalf("wrong-xid trigger: response xid = %d, want 1009", got)
	}

	// 4. Watch-push trigger: exists (op 3, xid 10) → normal response THEN a watch event
	//    (xid −1, FULL ReplyHeader format — D-S28.2-1: xid+zxid+error+type+state+path ≥ 28 bytes).
	writeFrame(append(append(be32(10), be32(3)...), be32(0)...))
	resp = readFrame()
	if got := int32(binary.BigEndian.Uint32(resp[0:4])); got != 10 {
		t.Fatalf("watch-push trigger: first response xid = %d, want 10", got)
	}
	push := readFrame()
	if got := int32(binary.BigEndian.Uint32(push[0:4])); got != -1 {
		t.Fatalf("watch-push trigger: push frame xid = %d, want -1", got)
	}
	if len(push) < 28 {
		t.Fatalf("watch-push frame = %d bytes, want >= 28 (full ReplyHeader — D-S28.2-1)", len(push))
	}
	// After xid(4)+zxid(8)+error(4)=16 bytes, event_type must be 1 (created).
	if got := int32(binary.BigEndian.Uint32(push[16:20])); got != 1 {
		t.Fatalf("watch-push event_type = %d, want 1", got)
	}

	if accepts.Load() != 1 {
		t.Fatalf("accepts = %d, want 1", accepts.Load())
	}
}

func TestMongoResponderBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	var accepts atomic.Uint64
	go acceptMongoResponder(ln, &accepts)

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	// An OP_QUERY(2004) requestID 11 → a correlated OP_REPLY(1) whose responseTo == 11.
	req := mongoReqFrame(11, 2004, "db.collection1")
	if _, err := c.Write(req); err != nil {
		t.Fatalf("write: %v", err)
	}
	hdr := make([]byte, 16)
	if _, err := io.ReadFull(c, hdr); err != nil {
		t.Fatalf("read reply header: %v", err)
	}
	msgLen := int32(binary.LittleEndian.Uint32(hdr[0:4]))
	responseTo := int32(binary.LittleEndian.Uint32(hdr[8:12]))
	opCode := int32(binary.LittleEndian.Uint32(hdr[12:16]))
	if opCode != 1 {
		t.Errorf("reply opCode = %d, want 1 (OP_REPLY)", opCode)
	}
	if responseTo != 11 {
		t.Errorf("reply responseTo = %d, want 11 (correlation echo)", responseTo)
	}
	rest := make([]byte, msgLen-16)
	if _, err := io.ReadFull(c, rest); err != nil {
		t.Fatalf("read reply body: %v", err)
	}
}

// TCPMongoResponder trigger markers (D-S29.2-2 / SPEC §6.1). The responder peeks
// the request frame's requestID (bytes 4-8) + opCode (bytes 12-16) only. A marker
// requestID selects a reply-flag variant or the unanswered-query withhold; the
// driver assigns these requestIDs so both sides see identical correlated bytes.
const (
	mongoReplyCursorNotFound int32 = 0x01 // responseFlags 0x01
	mongoReplyQueryFailure   int32 = 0x02 // responseFlags 0x02
)

// mongoMarkerWithhold is the requestID the responder treats as the unanswered-
// query trigger: it reads the request but writes NO reply (the gauge stays at 1
// while the connection is open — §3.4 / §6.2 arm 4).
const mongoMarkerWithhold int32 = 7777

// mongoMarkerCursorNotFound / mongoMarkerQueryFailure / mongoMarkerValidCursor /
// mongoMarkerUncorrelated select the reply variant by requestID.
const (
	mongoMarkerCursorNotFound int32 = 7001
	mongoMarkerQueryFailure   int32 = 7002
	mongoMarkerValidCursor    int32 = 7003
	mongoMarkerMalformedReply int32 = 7004
	mongoMarkerUncorrelated   int32 = 7005
)

// mongoMarkerRemoteClose is the requestID the responder treats as the upstream/
// REMOTE-close trigger (29.3 Task 11, 0052 arm 4(ii)): it reads the request frame
// then RETURNS, closing the backend conn (the deferred c.Close()) WITHOUT writing
// a reply → the query stays in-flight while the UPSTREAM end EOFs first. The
// tcp_proxy pump that copies upstream→downstream returns on that EOF and records
// CloseDirectionRemote, so the mongo OnDestroy keys cx_destroy_remote_with_active_rq.
// Distinct from mongoMarkerWithhold, which withholds the reply but keeps the
// backend conn OPEN (the driver then closes its own end → LOCAL).
const mongoMarkerRemoteClose int32 = 7006

// acceptMongoResponder accepts connections, counts them, and runs the
// MongoDB-aware canned-response loop on each (the TCPMongoResponder backend —
// 29.2 SPEC §6.1; the acceptZKResponder sibling).
func acceptMongoResponder(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go mongoRespondLoop(c)
	}
}

// mongoRespondLoop reads complete request frames (16-byte LE MsgHeader framing)
// and writes correlated canned responses until the client closes. It parses ONLY
// messageLength + requestID + opCode; it is NOT a MongoDB server.
func mongoRespondLoop(c net.Conn) {
	defer func() { _ = c.Close() }()
	le32 := func(v int32) []byte {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(v))
		return b
	}
	le64 := func(v int64) []byte {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, uint64(v))
		return b
	}
	// respFrame builds a response with responseTo echoed; opCode is OP_REPLY(1) or
	// OP_COMMANDREPLY(2011); a fresh responder requestID (constant 90000).
	respFrame := func(responseTo, opCode int32, body []byte) []byte {
		out := append(le32(int32(16+len(body))), le32(90000)...)
		out = append(out, le32(responseTo)...)
		out = append(out, le32(opCode)...)
		return append(out, body...)
	}
	emptyDoc := []byte{0x05, 0x00, 0x00, 0x00, 0x00} // {len=5}{terminator}
	replyBody := func(flags int32, cursorID int64, ndocs int32) []byte {
		out := append(le32(flags), le64(cursorID)...)
		out = append(out, le32(0)...)     // startingFrom
		out = append(out, le32(ndocs)...) // numberReturned
		for i := int32(0); i < ndocs; i++ {
			out = append(out, emptyDoc...)
		}
		return out
	}
	commandReplyBody := func() []byte { return append(append([]byte(nil), emptyDoc...), emptyDoc...) }

	for {
		var hdr [16]byte
		if _, err := io.ReadFull(c, hdr[:]); err != nil {
			return // client closed / EOF
		}
		msgLen := int32(binary.LittleEndian.Uint32(hdr[0:4]))
		if msgLen < 16 || msgLen > 1<<20 {
			return // malformed / hostile
		}
		body := make([]byte, msgLen-16)
		if _, err := io.ReadFull(c, body); err != nil {
			return
		}
		reqID := int32(binary.LittleEndian.Uint32(hdr[4:8]))
		opCode := int32(binary.LittleEndian.Uint32(hdr[12:16]))

		switch opCode {
		case 2004: // OP_QUERY → a correlated OP_REPLY, variant by the marker requestID
			switch reqID {
			case mongoMarkerWithhold:
				// withhold — no reply (the unanswered-query gauge arm)
			case mongoMarkerRemoteClose:
				// REMOTE-close (0052 arm 4(ii)): read the request, then close the
				// backend conn (the deferred c.Close()) WITHOUT replying → the upstream
				// end EOFs first → CloseDirectionRemote → cx_destroy_remote_with_active_rq.
				return
			case mongoMarkerCursorNotFound:
				_, _ = c.Write(respFrame(reqID, 1, replyBody(mongoReplyCursorNotFound, 0, 0)))
			case mongoMarkerQueryFailure:
				_, _ = c.Write(respFrame(reqID, 1, replyBody(mongoReplyQueryFailure, 0, 0)))
			case mongoMarkerValidCursor:
				_, _ = c.Write(respFrame(reqID, 1, replyBody(0, 4242, 1)))
			case mongoMarkerMalformedReply:
				// a well-framed OP_REPLY whose numberReturned LIES: claims 1 doc but the
				// body carries NONE (only the 20-byte fixed header, no trailing doc) →
				// decodeReply's parseDocument hits an empty reader → decoding_error on
				// BOTH sides (same bytes; reference_wire_format_both_sides_see_same_bytes).
				malformed := append(le32(0), le64(0)...)  // responseFlags + cursorID
				malformed = append(malformed, le32(0)...) // startingFrom
				malformed = append(malformed, le32(1)...) // numberReturned = 1 (no doc follows — the lie)
				_, _ = c.Write(respFrame(reqID, 1, malformed))
			case mongoMarkerUncorrelated:
				// a reply whose responseTo matches NO sent query (responseTo = reqID+50000)
				_, _ = c.Write(respFrame(reqID+50000, 1, replyBody(0, 0, 0)))
			default:
				_, _ = c.Write(respFrame(reqID, 1, replyBody(0, 0, 0))) // plain empty reply
			}
		case 2010: // OP_COMMAND → a correlated OP_COMMANDREPLY
			_, _ = c.Write(respFrame(reqID, 2011, commandReplyBody()))
		default:
			// OP_INSERT(2002) / OP_GET_MORE(2005) / OP_KILL_CURSORS(2007): no reply
			// (fire-and-forget; D-S29.2-2 — get_more not exercised by the load-bearing
			// arms). Read-and-drop.
		}
	}
}

// mongoReqFrame builds a minimal request frame for the responder unit test.
func mongoReqFrame(reqID, opCode int32, fullColl string) []byte {
	le32 := func(v int32) []byte {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(v))
		return b
	}
	body := append(le32(0), append([]byte(fullColl), 0x00)...) // flags + cstring collection
	body = append(body, le32(0)...)                            // numberToSkip
	body = append(body, le32(0)...)                            // numberToReturn
	body = append(body, 0x05, 0x00, 0x00, 0x00, 0x00)          // empty query doc
	out := append(le32(int32(16+len(body))), le32(reqID)...)
	out = append(out, le32(0)...)      // responseTo
	out = append(out, le32(opCode)...) // opCode
	return append(out, body...)
}

// kafkaMarkerUncorrelated is the correlation_id the Kafka responder treats as the
// unregistered-correlation trigger (SPEC §8.3): instead of echoing the request's
// correlation_id it emits a response whose correlation_id was NEVER sent
// (correlation_id+50000) → the subject's + reference's response-side correlation
// recover MISSES → response.failure on BOTH sides (the unregistered arm). A high
// sentinel the driver assigns so both sides see identical wire bytes
// (reference_wire_format_both_sides_see_same_bytes). The mongoMarkerUncorrelated
// (runner_test.go) precedent.
const kafkaMarkerUncorrelated int32 = 0x6BAD0000

// kafkaMarkerNoReply is the correlation_id the Kafka responder treats as the
// suppress-the-reply trigger (Task 13): the responder reads the full request frame
// (so the request-side decoder on both proxies fires) but writes NO response. This
// lets a fixture isolate the RESPONSE side to specific arms — request-only arms use
// this marker so no echoed response traverses the chain to perturb the response
// counters (the divergence-free request-arm construction). A high sentinel distinct
// from kafkaMarkerUncorrelated.
const kafkaMarkerNoReply int32 = 0x6BAD0001

// acceptKafkaResponder accepts connections, counts them, and runs the Kafka-aware
// canned-response loop on each (the TCPKafkaResponder backend — SPEC §8.3; the
// acceptMongoResponder sibling).
func acceptKafkaResponder(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go kafkaRespondLoop(c)
	}
}

// kafkaRespondLoop reads complete request frames (4-byte BE INT32 length prefix +
// frame) and writes correlation-id-echoing canned responses until the client
// closes. It parses ONLY api_key + api_version + correlation_id (the request
// header's leading 8 bytes); it is NOT a Kafka broker. The response body is a
// minimal per-api_key shape the reference can decode (kafkaResponseBody) — the
// exact bytes are live-verified at Task 13.
func kafkaRespondLoop(c net.Conn) {
	defer func() { _ = c.Close() }()
	beI16 := func(v int16) []byte {
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(v))
		return b
	}
	beI32 := func(v int32) []byte {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(v))
		return b
	}
	for {
		var lenPfx [4]byte
		if _, err := io.ReadFull(c, lenPfx[:]); err != nil {
			return // client closed / EOF
		}
		n := int32(binary.BigEndian.Uint32(lenPfx[:]))
		if n < 8 || n > 1<<20 {
			return // malformed / hostile (need ≥ api_key+api_version+correlation_id)
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(c, body); err != nil {
			return
		}
		apiKey := int16(binary.BigEndian.Uint16(body[0:2]))
		apiVersion := int16(binary.BigEndian.Uint16(body[2:4]))
		correlationID := int32(binary.BigEndian.Uint32(body[4:8]))

		if correlationID == kafkaMarkerNoReply {
			// suppress the reply entirely (request-only arms): read the request so the
			// request-side decoder fires, but write NOTHING so no response perturbs the
			// response counters. Continue reading further frames (the loop).
			continue
		}
		respCorrelation := correlationID
		if correlationID == kafkaMarkerUncorrelated {
			// emit a response whose correlation_id was NEVER sent → response.failure
			respCorrelation = correlationID + 50000
		}
		// respPayload = correlation_id INT32 ++ per-api_key body (NO response-header
		// tagged fields — non-flexible headers, and ALWAYS for ApiVersions per
		// AMEND-K5). out = 4-byte BE length of respPayload ++ respPayload.
		respPayload := append(beI32(respCorrelation), kafkaResponseBody(apiKey, apiVersion, beI16, beI32)...)
		out := append(beI32(int32(len(respPayload))), respPayload...)
		_, _ = c.Write(out)
	}
}

// kafkaResponseBody returns a minimal valid response BODY (after the
// correlation_id; the response header carries no tagged fields here) for the
// given api_key/api_version. Task 13 live-verifies and may refine these per key
// against the reference decoder. Keep this easy to adjust per api_key.
func kafkaResponseBody(apiKey, apiVersion int16, beI16 func(int16) []byte, beI32 func(int32) []byte) []byte {
	switch apiKey {
	case 18: // ApiVersions: error_code INT16 (0=NONE) ++ api_keys ARRAY (INT32 count 0)
		// v0 (non-flexible) is the simplest known-valid shape: an empty api_keys
		// array. Higher versions add throttle_time_ms / compact arrays / tagged
		// fields — Task 13 fills those if a higher version is exercised.
		return append(beI16(0), beI32(0)...)
	default:
		// Generic fallback: empty body. Task 13 fills per-key bodies (e.g. Metadata,
		// Produce, Fetch) once the live reference decoder pins the exact shape.
		return nil
	}
}

// kafkaReqFrame builds a minimal request frame for the responder unit test: a
// 4-byte BE length prefix + api_key INT16 + api_version INT16 + correlation_id
// INT32 + an empty (length −1) NULLABLE_STRING client_id (no tagged fields —
// v0/non-flexible request header). It is NOT a complete request body; the
// responder only reads the leading 8 bytes.
func kafkaReqFrame(apiKey, apiVersion int16, correlationID int32) []byte {
	beI16 := func(v int16) []byte {
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(v))
		return b
	}
	beI32 := func(v int32) []byte {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(v))
		return b
	}
	body := append(beI16(apiKey), beI16(apiVersion)...)
	body = append(body, beI32(correlationID)...)
	body = append(body, beI16(-1)...) // client_id NULLABLE_STRING = null (length −1)
	return append(beI32(int32(len(body))), body...)
}

func TestKafkaResponderBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	var accepts atomic.Uint64
	go acceptKafkaResponder(ln, &accepts)

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	// An ApiVersions(18) v0 request, correlation_id 11 → a correlated response whose
	// correlation_id == 11 and whose body is the empty-array ApiVersions v0 shape.
	if _, err := c.Write(kafkaReqFrame(18, 0, 11)); err != nil {
		t.Fatalf("write: %v", err)
	}
	readResp := func() (int32, []byte) {
		t.Helper()
		var lenPfx [4]byte
		if _, err := io.ReadFull(c, lenPfx[:]); err != nil {
			t.Fatalf("read length prefix: %v", err)
		}
		n := int32(binary.BigEndian.Uint32(lenPfx[:]))
		if n < 4 || n > 1<<20 {
			t.Fatalf("response length = %d, out of range", n)
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(c, payload); err != nil {
			t.Fatalf("read response payload: %v", err)
		}
		return int32(binary.BigEndian.Uint32(payload[0:4])), payload[4:]
	}
	corr, respBody := readResp()
	if corr != 11 {
		t.Errorf("response correlation_id = %d, want 11 (correlation echo)", corr)
	}
	// ApiVersions v0 body: error_code INT16 (0) ++ api_keys count INT32 (0) = 6 bytes.
	wantBody := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(respBody, wantBody) {
		t.Errorf("ApiVersions v0 body = % x, want % x", respBody, wantBody)
	}

	// The uncorrelated marker: the echoed correlation_id differs (was never sent).
	if _, err := c.Write(kafkaReqFrame(18, 0, kafkaMarkerUncorrelated)); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	markerCorr, _ := readResp()
	if markerCorr == kafkaMarkerUncorrelated {
		t.Errorf("marker response correlation_id = %d, want a DIFFERENT (unregistered) id", markerCorr)
	}
	if markerCorr != kafkaMarkerUncorrelated+50000 {
		t.Errorf("marker response correlation_id = %d, want %d (correlation_id+50000)", markerCorr, kafkaMarkerUncorrelated+50000)
	}
}

// acceptRedisResponder accepts connections, counts them, and runs the RESP-aware
// canned-response loop on each (the TCPRedisResponder backend — 32.1 SPEC §8.3;
// the acceptKafkaResponder sibling).
func acceptRedisResponder(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go redisRespondLoop(c)
	}
}

// redisRespondLoop reads RESP request frames (array-of-bulk: *<n>\r\n followed by
// n bulk strings $<len>\r\n<bytes>\r\n) and writes positional canned replies until
// the client closes. It is NOT a Redis server — it parses ONLY the command name
// (first bulk element, uppercased) and the first arg (the key). Reply table
// (32.2 command-matrix extension): SET → "+OK\r\n"; GET key "nope" → "$-1\r\n"
// (null bulk, GET-miss); GET any other key → "$3\r\nbar\r\n" (GET-hit);
// INCR/DEL → ":1\r\n"; any other → "-ERR unsupported\r\n". FIFO/positional —
// NO correlation id (contrast kafkaRespondLoop's correlation-id echo).
// PING/AUTH never reach the backend (redis_proxy answers them locally — AMEND-R5).
func redisRespondLoop(c net.Conn) {
	defer func() { _ = c.Close() }()
	r := bufio.NewReader(c)
	for {
		// Read the array header: *<n>\r\n
		line, err := r.ReadString('\n')
		if err != nil {
			return // EOF or error
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) < 2 || line[0] != '*' {
			return // malformed
		}
		var n int
		if _, err := fmt.Sscanf(line[1:], "%d", &n); err != nil || n < 1 {
			return // malformed or empty array
		}
		// Read the first bulk string (command name).
		cmdLine, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmdLine = strings.TrimRight(cmdLine, "\r\n")
		if len(cmdLine) < 2 || cmdLine[0] != '$' {
			return // malformed
		}
		var cmdLen int
		if _, err := fmt.Sscanf(cmdLine[1:], "%d", &cmdLen); err != nil || cmdLen < 1 {
			return
		}
		cmdBytes := make([]byte, cmdLen+2) // +2 for \r\n
		if _, err := io.ReadFull(r, cmdBytes); err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimRight(string(cmdBytes), "\r\n"))
		// Read the remaining n-1 bulk elements, capturing the first arg (the key) so
		// GET can distinguish hit/miss (32.2 command matrix).
		var firstArg string
		for i := 1; i < n; i++ {
			hdr, err := r.ReadString('\n')
			if err != nil {
				return
			}
			hdr = strings.TrimRight(hdr, "\r\n")
			if len(hdr) < 2 || hdr[0] != '$' {
				return
			}
			var argLen int
			if _, err := fmt.Sscanf(hdr[1:], "%d", &argLen); err != nil || argLen < 0 {
				return
			}
			argBytes := make([]byte, argLen+2) // +2 for \r\n
			if _, err := io.ReadFull(r, argBytes); err != nil {
				return
			}
			if i == 1 {
				firstArg = strings.TrimRight(string(argBytes), "\r\n")
			}
		}
		// Write the canned reply (32.2 reply table: $-1 GET-miss keyed on the first
		// arg "nope", :1 INCR/DEL; the existing SET/GET-hit arms unchanged).
		var reply string
		switch cmd {
		case "SET":
			reply = "+OK\r\n"
		case "GET":
			if firstArg == "nope" {
				reply = "$-1\r\n" // GET-miss (null bulk — §8.4; the driver's miss key is "nope")
			} else {
				reply = "$3\r\nbar\r\n" // GET-hit
			}
		case "INCR", "DEL":
			reply = ":1\r\n"
		default:
			reply = "-ERR unsupported\r\n"
		}
		if _, err := c.Write([]byte(reply)); err != nil {
			return
		}
	}
}

// acceptThriftResponder accepts connections, counts them, and runs the framed-binary
// Thrift canned-response loop on each (the TCPThriftResponder backend — SPEC §8.3;
// the acceptRedisResponder sibling).
func acceptThriftResponder(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go thriftRespondLoop(c)
	}
}

// thriftRespondLoop reads framed-binary Thrift CALL frames (4-byte BE frame-length +
// binary message-begin: magic 0x8001 + zero + msgtype + i32 name-len + name + i32
// seq_id + opaque body) and writes a canned REPLY per CALL until the client closes.
// It is NOT a Thrift server — it parses ONLY the message-begin (method + seq_id) and
// echoes a framed-binary REPLY (msgtype 2) carrying the SAME method + RECEIVED seq_id
// and a void-success body (single STOP 0x00). seq_id-AGNOSTIC (echoes whatever it
// receives — AMEND-T5). The marker method "boom" (thriftMarkerException) yields a
// framed-binary EXCEPTION (msgtype 3) reply carrying an AppException TStruct body
// (D-S33-2 reply-EXCEPTION). The wire format is DUPLICATED here (the TCPRedisResponder
// self-contained precedent — no internal/filter/network/thriftproxy import).
func thriftRespondLoop(c net.Conn) {
	defer func() { _ = c.Close() }()
	r := bufio.NewReader(c)
	for {
		// Read the 4-byte BE frame-length prefix.
		var lenPfx [4]byte
		if _, err := io.ReadFull(r, lenPfx[:]); err != nil {
			return // EOF between frames (clean end) or error
		}
		frameLen := int32(binary.BigEndian.Uint32(lenPfx[:]))
		if frameLen < 12 || int64(frameLen) > 100*1024*1024 {
			return // out-of-range frame length
		}
		payload := make([]byte, frameLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return
		}
		if binary.BigEndian.Uint16(payload[0:2]) != 0x8001 {
			return // bad magic
		}
		nameLen := int32(binary.BigEndian.Uint32(payload[4:8]))
		if nameLen < 0 || 8+int64(nameLen)+4 > int64(len(payload)) {
			return
		}
		method := string(payload[8 : 8+nameLen])
		off := 8 + nameLen
		seqID := int32(binary.BigEndian.Uint32(payload[off : off+4]))

		// Build the reply: void-success REPLY (msgtype 2) by default; an EXCEPTION
		// (msgtype 3) for the marker method (D-S33-2). Both echo method + seq_id.
		var reply []byte
		if method == thriftMarkerException {
			reply = thriftExceptionFrame(method, seqID)
		} else {
			reply = thriftReplyFrame(method, seqID)
		}
		if _, err := c.Write(reply); err != nil {
			return
		}
	}
}

// thriftFrame wraps a binary message-begin payload (built by the caller) in the
// 4-byte BE frame-length prefix (Appendix A). DUPLICATED from the filter package's
// wire format (the TCPRedisResponder self-contained precedent).
func thriftFrame(payload []byte) []byte {
	var lenPfx [4]byte
	binary.BigEndian.PutUint32(lenPfx[:], uint32(len(payload)))
	frame := make([]byte, 0, 4+len(payload))
	frame = append(frame, lenPfx[:]...)
	frame = append(frame, payload...)
	return frame
}

// thriftMsgBegin builds a binary strict message-begin: magic 0x8001 + zero + msgtype
// + i32 name-len + name + i32 seq_id (Appendix A).
func thriftMsgBegin(msgType uint8, method string, seqID int32) []byte {
	p := []byte{0x80, 0x01, 0x00, msgType}
	var i32 [4]byte
	binary.BigEndian.PutUint32(i32[:], uint32(len(method)))
	p = append(p, i32[:]...)
	p = append(p, method...)
	binary.BigEndian.PutUint32(i32[:], uint32(seqID))
	p = append(p, i32[:]...)
	return p
}

// thriftReplyFrame builds a framed-binary REPLY (msgtype 2) echoing method + seqID
// with a void-success body (single STOP 0x00 — an empty result struct → response_success).
func thriftReplyFrame(method string, seqID int32) []byte {
	p := thriftMsgBegin(2, method, seqID)
	p = append(p, 0x00) // STOP — void success
	return thriftFrame(p)
}

// thriftExceptionFrame builds a framed-binary EXCEPTION (msgtype 3) echoing method +
// seqID, carrying an AppException TStruct body {1: string "backend exception", 2: i32 6}
// (TApplicationException; type 6 = INTERNAL_ERROR). The body shape mirrors the filter
// package's encodeUnknownMethod layout (field 1 STRING id 1, field 2 I32 id 2, STOP) so
// the reference's response decoder classifies it as response_exception.
func thriftExceptionFrame(method string, seqID int32) []byte {
	p := thriftMsgBegin(3, method, seqID)
	msg := "backend exception"
	// field 1: STRING (0x0b) id 1 → i32 len + bytes
	p = append(p, 0x0b, 0x00, 0x01)
	var i32 [4]byte
	binary.BigEndian.PutUint32(i32[:], uint32(len(msg)))
	p = append(p, i32[:]...)
	p = append(p, msg...)
	// field 2: I32 (0x08) id 2 → i32 value (exception type 6 = INTERNAL_ERROR)
	p = append(p, 0x08, 0x00, 0x02)
	binary.BigEndian.PutUint32(i32[:], 6)
	p = append(p, i32[:]...)
	// STOP
	p = append(p, 0x00)
	return thriftFrame(p)
}

// thriftMarkerException is the request method name the Thrift responder treats as
// the reply-EXCEPTION trigger (D-S33-2 / SPEC §8.3): instead of a void-success
// REPLY (msgtype 2) it answers a framed-binary EXCEPTION (msgtype 3) echoing the
// SAME method + RECEIVED seq_id, carrying an AppException TStruct body. This lets
// the 0057 fixture's reply-EXCEPTION arm exercise response_exception from a BACKEND
// reply (distinct from the local route-miss exception). An in-band request marker —
// the kafkaMarkerUncorrelated request-keyed precedent (the responder stays keyed by
// BackendKind; the per-request behavior is selected from the wire).
const thriftMarkerException = "boom"

// thriftReqFrame builds a framed-binary Thrift CALL request frame for the responder
// unit test (Appendix A): 4-byte BE frame-length + binary message-begin (magic
// 0x8001 + zero + msgtype CALL(1) + i32 name-len + name + i32 seq_id) + a STOP(0x00)
// void body. The same wire format as internal/filter/network/thriftproxy/thrift.go,
// DUPLICATED here (the TCPRedisResponder self-contained precedent — no filter import).
func thriftReqFrame(method string, seqID int32) []byte {
	var p []byte
	p = append(p, 0x80, 0x01, 0x00, 0x01) // version magic + CALL(1)
	var i32 [4]byte
	binary.BigEndian.PutUint32(i32[:], uint32(len(method)))
	p = append(p, i32[:]...)
	p = append(p, method...)
	binary.BigEndian.PutUint32(i32[:], uint32(seqID))
	p = append(p, i32[:]...)
	p = append(p, 0x00) // STOP — void body
	var frame []byte
	binary.BigEndian.PutUint32(i32[:], uint32(len(p)))
	frame = append(frame, i32[:]...)
	frame = append(frame, p...)
	return frame
}

// TestThriftResponderBackend exercises the TCPThriftResponder canned-response loop
// (the acceptRedisResponder/acceptKafkaResponder sibling). It sends a framed-binary
// CALL("ping", seq 7) and asserts the responder echoes a framed-binary REPLY
// (msgtype 2) with the SAME method + RECEIVED seq_id and a void-success body (single
// STOP 0x00). The exception marker method ("boom") yields a framed-binary EXCEPTION
// (msgtype 3) reply (D-S33-2).
func TestThriftResponderBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	var accepts atomic.Uint64
	go acceptThriftResponder(ln, &accepts)

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	r := bufio.NewReader(c)

	// readReply reads ONE framed-binary REPLY/EXCEPTION frame and returns the
	// msgtype, method, seq_id, and the opaque body bytes (after the message-begin).
	readReply := func() (uint8, string, int32, []byte) {
		t.Helper()
		var lenPfx [4]byte
		if _, err := io.ReadFull(r, lenPfx[:]); err != nil {
			t.Fatalf("read frame length: %v", err)
		}
		n := int32(binary.BigEndian.Uint32(lenPfx[:]))
		if n < 12 || n > 1<<20 {
			t.Fatalf("frame length = %d, out of range", n)
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(r, payload); err != nil {
			t.Fatalf("read frame payload: %v", err)
		}
		if got := binary.BigEndian.Uint16(payload[0:2]); got != 0x8001 {
			t.Fatalf("reply magic = %#04x, want 0x8001", got)
		}
		mt := payload[3]
		nameLen := int32(binary.BigEndian.Uint32(payload[4:8]))
		method := string(payload[8 : 8+nameLen])
		off := 8 + nameLen
		seqID := int32(binary.BigEndian.Uint32(payload[off : off+4]))
		return mt, method, seqID, payload[off+4:]
	}

	// Void-success arm: CALL("ping", seq 7) → REPLY msgtype 2, method "ping", seq 7,
	// body single STOP 0x00.
	if _, err := c.Write(thriftReqFrame("ping", 7)); err != nil {
		t.Fatalf("write call: %v", err)
	}
	mt, method, seqID, body := readReply()
	if mt != 2 {
		t.Errorf("reply msgtype = %d, want 2 (Reply)", mt)
	}
	if method != "ping" {
		t.Errorf("reply method = %q, want \"ping\" (echo)", method)
	}
	if seqID != 7 {
		t.Errorf("reply seq_id = %d, want 7 (received-seq_id echo)", seqID)
	}
	if !bytes.Equal(body, []byte{0x00}) {
		t.Errorf("reply body = % x, want 00 (void-success STOP)", body)
	}

	// Exception arm: CALL(marker "boom", seq 9) → EXCEPTION msgtype 3, method "boom",
	// seq 9 (D-S33-2 reply-EXCEPTION).
	if _, err := c.Write(thriftReqFrame(thriftMarkerException, 9)); err != nil {
		t.Fatalf("write exception call: %v", err)
	}
	mt, method, seqID, _ = readReply()
	if mt != 3 {
		t.Errorf("exception reply msgtype = %d, want 3 (Exception)", mt)
	}
	if method != thriftMarkerException {
		t.Errorf("exception reply method = %q, want %q (echo)", method, thriftMarkerException)
	}
	if seqID != 9 {
		t.Errorf("exception reply seq_id = %d, want 9 (received-seq_id echo)", seqID)
	}
}

// serveGRPCHealth is the in-process h2c backend for GRPCHealthResponder (phase
// 39.2, BackendKind 34). It muxes two request classes on a single h2c listener:
//
//   - gRPC (Content-Type: application/grpc over HTTP/2): delegated to a real
//     gRPC server that registers grpc.health.v1.Health and responds SERVING for
//     the unnamed service ("") — satisfying the active gRPC HC probe.
//
//   - Plain-H2 data-plane: returns HTTP 200 + "backend-<idx>:<path>" body so
//     the load-phase 100%-live assertion holds and host attribution works via
//     response body (the 0066 backendIdxFromBody precedent). No accept counter
//     is maintained; the driver derives distribution from the response body.
func serveGRPCHealth(ln net.Listener, idx int) {
	gs := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(gs, hs)
	mux := func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			gs.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "backend-%d:%s", idx, r.URL.Path)
	}
	srv := &http.Server{Handler: h2c.NewHandler(http.HandlerFunc(mux), &http2.Server{})}
	_ = srv.Serve(ln)
}
