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
