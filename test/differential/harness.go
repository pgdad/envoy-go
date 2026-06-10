package differential

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

// EnvoyPin captures the upstream image identity from ENVOY_TARGET.md.
type EnvoyPin struct {
	Tag    string // e.g. envoyproxy/envoy:v1.34.0
	SHA256 string // e.g. sha256:<hex>
}

var (
	tagLineRE    = regexp.MustCompile(`(?m)^\*\*Tag:\*\*\s+` + "`" + `([^` + "`" + `]+)` + "`")
	sha256LineRE = regexp.MustCompile(`(?m)^\*\*SHA256:\*\*\s+` + "`" + `([^` + "`" + `]+)` + "`")
)

func parseEnvoyTarget(r io.Reader) (*EnvoyPin, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	tagM := tagLineRE.FindSubmatch(src)
	if tagM == nil {
		return nil, fmt.Errorf("ENVOY_TARGET.md: missing **Tag:** line")
	}
	shaM := sha256LineRE.FindSubmatch(src)
	if shaM == nil {
		return nil, fmt.Errorf("ENVOY_TARGET.md: missing **SHA256:** line")
	}
	return &EnvoyPin{Tag: string(tagM[1]), SHA256: string(shaM[1])}, nil
}

// readyTimeout is the wall-clock budget the harness allows each proxy to
// declare itself ready (admin /ready 200 for the reference, ready sentinel on
// stdout for the subject). Generous on purpose; SPEC §11 mitigates flakiness
// by surfacing failures, not retrying.
const readyTimeout = 30 * time.Second

// readyListenerAddrs reads lines from r until the terminal `envoy-go ready`
// sentinel is observed, collecting every `envoy-go listener <name> ready on
// <addr>` line into a name→addr map. ADR-0026 codifies the phase-02 sentinel
// contract.
func readyListenerAddrs(ctx context.Context, r io.Reader) (map[string]string, error) {
	br := bufio.NewReader(r)
	out := make(chan map[string]string, 1)
	errCh := make(chan error, 1)
	go func() {
		addrs := map[string]string{}
		re := regexp.MustCompile(`^envoy-go listener (\S+) ready on (\S+)$`)
		for {
			line, err := br.ReadString('\n')
			trimmed := strings.TrimRight(line, "\r\n")
			if trimmed == "envoy-go ready" {
				out <- addrs
				return
			}
			if m := re.FindStringSubmatch(trimmed); m != nil {
				addrs[m[1]] = m[2]
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()
	select {
	case a := <-out:
		return a, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReferenceProxy is the upstream Envoy container managed by the harness.
type ReferenceProxy struct {
	container testcontainers.Container
	adminAddr string
	tcpAddrs  map[int]string // listener port (in-container) → host:port
}

// StartReferenceProxy launches the pinned Envoy image with the supplied
// bootstrap YAML, waits for admin /ready to return 200, and returns a handle.
// listenerPorts are container-internal TCP ports that should be exposed and
// looked up by AdminAddr / ListenerAddr.
func StartReferenceProxy(ctx context.Context, pin *EnvoyPin, bootstrap string, listenerPorts ...int) (*ReferenceProxy, error) {
	exposed := []string{"9901/tcp"}
	for _, p := range listenerPorts {
		exposed = append(exposed, fmt.Sprintf("%d/tcp", p))
	}
	req := testcontainers.ContainerRequest{
		Image:        pin.SHA256,
		ExposedPorts: exposed,
		// --concurrency 1 forces a single worker thread so round-robin LB
		// state is per-process-cluster (not per-worker). Over N connections
		// with M endpoints where M | N the distribution is exactly N/M per
		// backend — satisfying phase-02 SPEC §5.8's per-proxy distribution
		// assertion. See ADR-0028.
		Cmd:        []string{"envoy", "--config-yaml", bootstrap, "--log-level", "warn", "--concurrency", "1"},
		WaitingFor: wait.ForHTTP("/ready").WithPort("9901/tcp").WithStartupTimeout(readyTimeout),
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.ExtraHosts = []string{"host.docker.internal:host-gateway"}
		},
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start reference: %w", err)
	}
	host, err := c.Host(ctx)
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	adminMapped, err := c.MappedPort(ctx, "9901/tcp")
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	tcp := map[int]string{}
	for _, p := range listenerPorts {
		mapped, err := c.MappedPort(ctx, nat.Port(fmt.Sprintf("%d/tcp", p)))
		if err != nil {
			_ = c.Terminate(ctx)
			return nil, err
		}
		tcp[p] = fmt.Sprintf("%s:%s", host, mapped.Port())
	}
	return &ReferenceProxy{
		container: c,
		adminAddr: fmt.Sprintf("%s:%s", host, adminMapped.Port()),
		tcpAddrs:  tcp,
	}, nil
}

// StartReferenceProxyWithMounts is like StartReferenceProxy but also bind-mounts
// host files into the container before start. Each mount maps a host-side file
// path to a container-side path. The host files must already exist (Docker bind-
// mount of a file, not a directory, requires the host file to be pre-created).
// Introduced for fixture 0006-access-log (ADR-0068) to surface the reference
// Envoy's container-side access log (/envoy-go-test/envoy-access.log) to a
// host-visible path for log-comparison.
//
// Bind mounts are implemented via HostConfig.Binds (the "<hostPath>:<containerPath>"
// format) because testcontainers-go v0.27.0's Mounts / ContainerMounts path
// silently drops MountTypeBind entries in mapToDockerMounts.
func StartReferenceProxyWithMounts(ctx context.Context, pin *EnvoyPin, bootstrap string, hostMounts []fixture.HostMount, listenerPorts ...int) (*ReferenceProxy, error) {
	exposed := []string{"9901/tcp"}
	for _, p := range listenerPorts {
		exposed = append(exposed, fmt.Sprintf("%d/tcp", p))
	}
	// Build the Binds slice in Docker bind format: "hostPath:containerPath".
	binds := make([]string, 0, len(hostMounts))
	for _, m := range hostMounts {
		binds = append(binds, m.HostPath+":"+m.ContainerPath)
	}
	req := testcontainers.ContainerRequest{
		Image:        pin.SHA256,
		ExposedPorts: exposed,
		Cmd:          []string{"envoy", "--config-yaml", bootstrap, "--log-level", "warn", "--concurrency", "1"},
		WaitingFor:   wait.ForHTTP("/ready").WithPort("9901/tcp").WithStartupTimeout(readyTimeout),
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.ExtraHosts = []string{"host.docker.internal:host-gateway"}
			hc.Binds = append(hc.Binds, binds...)
		},
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start reference: %w", err)
	}
	host, err := c.Host(ctx)
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	adminMapped, err := c.MappedPort(ctx, "9901/tcp")
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	tcp := map[int]string{}
	for _, p := range listenerPorts {
		mapped, err := c.MappedPort(ctx, nat.Port(fmt.Sprintf("%d/tcp", p)))
		if err != nil {
			_ = c.Terminate(ctx)
			return nil, err
		}
		tcp[p] = fmt.Sprintf("%s:%s", host, mapped.Port())
	}
	return &ReferenceProxy{
		container: c,
		adminAddr: fmt.Sprintf("%s:%s", host, adminMapped.Port()),
		tcpAddrs:  tcp,
	}, nil
}

// AdminAddr returns the host:port for the container's admin listener (9901/tcp).
func (r *ReferenceProxy) AdminAddr() string { return r.adminAddr }

// ListenerAddr returns the host:port for an exposed in-container listener port.
func (r *ReferenceProxy) ListenerAddr(containerPort int) string { return r.tcpAddrs[containerPort] }

// Stop terminates the container.
func (r *ReferenceProxy) Stop(ctx context.Context) error {
	return r.container.Terminate(ctx)
}

// SubjectProxy is the envoy-go subprocess managed by the harness.
type SubjectProxy struct {
	cmd           *exec.Cmd
	listenerAddrs map[string]string
	adminAddr     string
	tmpDir        string
}

// StartSubjectProxy builds cmd/envoy-go from repoRoot, writes cfg to a temp
// file, starts the subject as a subprocess, waits for the ready sentinel, and
// returns a handle. The harness owns the subprocess lifetime; callers must
// call Stop to release. subjAdminAddr is the pre-allocated admin host:port
// that the caller interpolated into cfg; the harness records it so callers
// can reach the subject's admin surface without re-parsing the bootstrap.
func StartSubjectProxy(ctx context.Context, repoRoot, cfg, subjAdminAddr string) (*SubjectProxy, error) {
	tmp, err := os.MkdirTemp("", "envoy-go-subject-*")
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(tmp, "envoy-go")
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/envoy-go")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("build subject: %w\n%s", err, out)
	}
	cfgPath := filepath.Join(tmp, "envoy-go.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}

	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("start subject: %w", err)
	}

	readyCtx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	addrs, err := readyListenerAddrs(readyCtx, stdout)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("subject ready: %w", err)
	}

	return &SubjectProxy{cmd: cmd, listenerAddrs: addrs, adminAddr: subjAdminAddr, tmpDir: tmp}, nil
}

// ListenerAddr returns the host:port the subject is listening on for the
// named listener (parsed from the per-listener ready sentinel). Returns "" if
// the name is unknown. ADR-0026.
func (s *SubjectProxy) ListenerAddr(name string) string { return s.listenerAddrs[name] }

// AdminAddr returns the subject's admin host:port (pre-allocated by the caller
// and interpolated into the subject bootstrap).
func (s *SubjectProxy) AdminAddr() string { return s.adminAddr }

// Stop kills and reaps the subject and cleans up its temp directory.
func (s *SubjectProxy) Stop() error {
	_ = s.cmd.Process.Kill()
	_, _ = s.cmd.Process.Wait()
	return os.RemoveAll(s.tmpDir)
}

// FixtureDriver is re-exported from the fixture sub-package for callers that
// imported it from here before the refactor. New code should import
// test/differential/fixture directly.
type FixtureDriver = fixture.Driver

// RegisterFixture re-exports fixture.RegisterFixture for backward compat.
func RegisterFixture(name string, d FixtureDriver) { fixture.RegisterFixture(name, d) }

// BootRejectFixture is an OPTIONAL driver-side interface that signals the
// runner to assert BOTH the reference Envoy AND the envoy-go subject REJECT
// boot when fed an intentionally broken filter-config (e.g., a Lua script
// with a compile error). Per parent SPEC §13-R1 + §11.7.3 + phase-22.1 SPEC
// §6 Task 13 + AMEND-10 (option 2 substring-match).
//
// The runner's runBootRejectFixture branch (paralleling runReferenceLessFixture
// at runner_test.go) does the following:
//  1. Renders the per-side bootstrap with the broken script path interpolated
//     (the driver's existing ReferenceBootstrap + SubjectConfig templates point
//     at the path returned by BootRejectScript()).
//  2. Invokes tryStartReferenceProxy + tryStartSubjectProxy (NOT the
//     t.Fatalf-on-failure StartReferenceProxy / StartSubjectProxy variants).
//  3. Asserts BOTH calls return a non-nil err (boot rejection on both sides).
//  4. Asserts BOTH captured stderr buffers contain ExpectedBootErrorSubstring()
//     as a SUBSTRING (case-sensitive, anywhere in stderr — matches the AMEND-10
//     option 2 "both sides exit non-zero AND both sides' stderr contains the
//     substring `\"script load error\"`" promise).
//
// Used by fixture-0026-http-lua-headers-bridge scenario (g) g_compile_error.lua
// (lands at Task 14). Drivers that do NOT implement this interface bypass the
// boot-reject branch entirely.
//
// The BootRejectScript path is relative to the fixture directory (e.g.,
// "scripts/g_compile_error.lua") — matches the §9.4 scripts/ subdirectory
// convention. The driver is responsible for splicing this path into the
// ReferenceBootstrap + SubjectConfig templates as the lua filter's DataSource
// Filename source. Introduced by phase 22.1 Task 13.
type BootRejectFixture interface {
	// BootRejectScript returns the path (relative to the fixture directory)
	// to the broken Lua script the runner uses to drive the boot-reject
	// assertion. Per §9.4 the path SHOULD live under scripts/ — e.g.,
	// "scripts/g_compile_error.lua".
	BootRejectScript() string

	// ExpectedBootErrorSubstring returns the substring the runner asserts is
	// present (case-sensitive) in BOTH reference + subject stderr after the
	// boot-reject. For fixture-0026 scenario (g) this is the literal string
	// "script load error" per parent §11.7.5 + AMEND-10 option 2.
	ExpectedBootErrorSubstring() string
}

// SubjectOnlyBootRejectFixture is an OPTIONAL sibling-interface extension to
// BootRejectFixture, introduced at phase 25.2 IMPL Task 21 for fixture-0037
// per 25.2 SPEC §8.2 + D-25.2-P1 closure. It signals the runner that the
// boot-reject is SUBJECT-ONLY — the envoy-go subject MUST boot-reject with
// the substring (ExpectedBootErrorSubstring()) in its stderr, but the
// reference Envoy v1.37.2 MUST boot SUCCESSFULLY because the trigger is an
// envoy-go-strict-only validator (e.g., arm 19 envoy_go_strict_body_buffer_
// cap_bytes = 0) for which upstream Envoy has NO equivalent field — the
// unknown extension field is silently dropped by upstream's protobuf parser.
//
// Drivers that implement BootRejectFixture but NOT this interface default to
// the SYMMETRIC boot-reject discipline (both sides must boot-reject with the
// substring; the existing fixture-0026/0029/0031/0033/0035 precedent).
//
// Drivers that implement BOTH interfaces and return true from SubjectOnly()
// get the asymmetric discipline:
//  1. Render the reference bootstrap via ReferenceBootstrap(); start the
//     reference proxy via tryStartReferenceProxy; assert it boots SUCCESSFULLY
//     (cancel returned non-nil, err is nil). Tear down the reference proxy.
//  2. Render the subject config via SubjectConfig(); start the subject via
//     tryStartSubjectProxy; assert it boot-REJECTS (cancel is nil, err is
//     non-nil) with the substring in its stderr.
//  3. Skip the cross-side request/response phase entirely (no driving of
//     either side; this is a config-load-only fixture).
//
// Per reference_differential_fixture_dispatch_constraint: one fixture dir =
// ONE runner branch. Fixture-0037 occupies the subject-only-boot-reject
// branch; the existing fixture-0035 occupies the symmetric-boot-reject
// branch; the cross-side fixture-0036 occupies the cross-side branch.
type SubjectOnlyBootRejectFixture interface {
	// SubjectOnly returns true to opt into the asymmetric subject-only-boot-
	// reject discipline. Drivers returning false (or not implementing this
	// interface) default to the symmetric boot-reject discipline.
	SubjectOnly() bool
}

// bootRejectTimeout is the wall-clock budget the harness gives each proxy to
// exit with a boot-reject before tryStart* gives up. Generous: the reference
// container exits within a few seconds typically; the subject subprocess exits
// within hundreds of milliseconds. The harness surfaces timeouts as errors
// without retrying.
const bootRejectTimeout = 20 * time.Second

// tryStartReferenceProxy is the NON-t.Fatalf-ing variant of StartReferenceProxy
// for the runBootRejectFixture branch (per parent §11.7.3 + §13-R1). Instead
// of t.Fatalf-ing on container-start failure or admin /ready timeout, it
// returns the boot error + a captured stderr buffer for substring assertion
// against ExpectedBootErrorSubstring().
//
// On success (the container DID come up — surprising for a boot-reject fixture
// but tolerated for symmetry with tryStartSubjectProxy below), it returns a
// non-nil cancel func that terminates the container and a nil err. The
// caller MUST invoke cancel even on success to release the container.
//
// On boot-reject (the expected path for fixture-0026 scenario (g)), it returns
// a nil cancel func + the captured stderr buffer + a non-nil err describing
// the boot-reject (typically the wait-for-/ready timeout the testcontainers
// startup probe surfaces when Envoy exits before binding admin). The caller
// inspects err + scans the stderr buffer for ExpectedBootErrorSubstring().
//
// The stderr buffer captures the reference container's complete stderr stream
// from container start through container exit (or the bootRejectTimeout
// deadline, whichever fires first). The capture is best-effort — Docker's log
// retrieval can lag a moment after the container exits; the harness reads
// until EOF or context cancellation.
func tryStartReferenceProxy(ctx context.Context, pin *EnvoyPin, bootstrap string, hostMounts []fixture.HostMount, listenerPorts ...int) (cancel func(), stderrBuf *bytes.Buffer, err error) {
	exposed := []string{"9901/tcp"}
	for _, p := range listenerPorts {
		exposed = append(exposed, fmt.Sprintf("%d/tcp", p))
	}
	// Build the Binds slice in Docker bind format: "hostPath:containerPath".
	// Mirrors StartReferenceProxyWithMounts. Required for subject-only boot-
	// reject fixtures (e.g., fixture-0037) where the REFERENCE side MUST boot
	// successfully — its wasm filter needs the on-disk .wasm blob accessible
	// inside the container.
	binds := make([]string, 0, len(hostMounts))
	for _, m := range hostMounts {
		binds = append(binds, m.HostPath+":"+m.ContainerPath)
	}
	// AutoRemove is OFF so we can pull logs after the container exits.
	req := testcontainers.ContainerRequest{
		Image:        pin.SHA256,
		ExposedPorts: exposed,
		Cmd:          []string{"envoy", "--config-yaml", bootstrap, "--log-level", "warn", "--concurrency", "1"},
		// We deliberately use a short WaitingFor so the testcontainers
		// startup path fails fast when the container exits without binding
		// admin (the boot-reject path). The substring-match assertion lives
		// in the caller against the captured stderr buffer.
		WaitingFor: wait.ForHTTP("/ready").WithPort("9901/tcp").WithStartupTimeout(bootRejectTimeout),
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.ExtraHosts = []string{"host.docker.internal:host-gateway"}
			hc.AutoRemove = false
			hc.Binds = append(hc.Binds, binds...)
		},
	}
	startCtx, startCancel := context.WithTimeout(ctx, bootRejectTimeout+5*time.Second)
	defer startCancel()
	c, startErr := testcontainers.GenericContainer(startCtx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	stderrBuf = &bytes.Buffer{}
	// Regardless of start success/failure, attempt to drain the container
	// logs into stderrBuf so the caller can substring-assert. Some failure
	// paths (image-pull failure, port-conflict) yield no container handle —
	// in those cases the buffer stays empty and the caller falls back to
	// the err string for diagnostic.
	if c != nil {
		// Drain logs with a short timeout — we want stderr, not to block.
		drainCtx, drainCancel := context.WithTimeout(ctx, 5*time.Second)
		defer drainCancel()
		if rc, lerr := c.Logs(drainCtx); lerr == nil {
			_, _ = io.Copy(stderrBuf, rc)
			_ = rc.Close()
		}
	}

	if startErr != nil {
		// Boot-reject path: terminate the container if it exists + return
		// the error + captured stderr.
		if c != nil {
			_ = c.Terminate(context.Background())
		}
		return nil, stderrBuf, fmt.Errorf("reference boot-reject: %w", startErr)
	}

	// Surprising success path: the container DID come up. Return a cancel
	// func that terminates it, and a nil err.
	return func() { _ = c.Terminate(context.Background()) }, stderrBuf, nil
}

// tryStartSubjectProxy is the NON-t.Fatalf-ing variant of StartSubjectProxy
// for the runBootRejectFixture branch (per parent §11.7.3 + §13-R1). Instead
// of t.Fatalf-ing on subprocess-start failure or ready-sentinel timeout, it
// returns the boot error + a captured stderr buffer for substring assertion
// against ExpectedBootErrorSubstring().
//
// On success (the subject DID come up — surprising for a boot-reject fixture
// but tolerated for symmetry with tryStartReferenceProxy above), it returns a
// non-nil cancel func that kills + reaps the subprocess and a nil err. The
// caller MUST invoke cancel even on success to release the subprocess.
//
// On boot-reject (the expected path for fixture-0026 scenario (g)), it returns
// a nil cancel func + the captured stderr buffer + a non-nil err describing
// the boot-reject (typically subprocess exit-status-non-zero before the ready
// sentinel fires). The caller inspects err + scans the stderr buffer for
// ExpectedBootErrorSubstring().
//
// The stderr buffer captures the subject subprocess's complete stderr stream
// from start through exit (or the bootRejectTimeout deadline). Build failures
// (`go build` non-zero exit) are returned as errors with the build output in
// the err message — the stderr buffer remains empty in that case.
func tryStartSubjectProxy(ctx context.Context, repoRoot, cfg, subjAdminAddr string) (cancel func(), stderrBuf *bytes.Buffer, err error) {
	stderrBuf = &bytes.Buffer{}
	tmp, terr := os.MkdirTemp("", "envoy-go-subject-bootreject-*")
	if terr != nil {
		return nil, stderrBuf, terr
	}
	bin := filepath.Join(tmp, "envoy-go")
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/envoy-go")
	build.Dir = repoRoot
	if out, berr := build.CombinedOutput(); berr != nil {
		_ = os.RemoveAll(tmp)
		return nil, stderrBuf, fmt.Errorf("build subject: %w\n%s", berr, out)
	}
	cfgPath := filepath.Join(tmp, "envoy-go.yaml")
	if werr := os.WriteFile(cfgPath, []byte(cfg), 0o600); werr != nil {
		_ = os.RemoveAll(tmp)
		return nil, stderrBuf, werr
	}

	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, perr := cmd.StdoutPipe()
	if perr != nil {
		_ = os.RemoveAll(tmp)
		return nil, stderrBuf, perr
	}
	// Capture stderr into the buffer for substring assertion. We tee to
	// os.Stderr as well so the test author can see the rejection wording
	// during local iteration; the substring assertion runs against stderrBuf.
	cmd.Stderr = io.MultiWriter(stderrBuf, os.Stderr)
	if serr := cmd.Start(); serr != nil {
		_ = os.RemoveAll(tmp)
		return nil, stderrBuf, fmt.Errorf("start subject: %w", serr)
	}

	// Race the ready sentinel against subprocess exit. For a boot-reject the
	// subprocess exits before "envoy-go ready" appears on stdout — the
	// readyListenerAddrs goroutine returns io.EOF (or similar) which we
	// surface as the boot-reject error.
	readyCtx, readyCancel := context.WithTimeout(ctx, bootRejectTimeout)
	defer readyCancel()
	addrs, rerr := readyListenerAddrs(readyCtx, stdout)
	if rerr != nil {
		// Boot-reject path: kill (in case it's still alive) + reap + clean up.
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(tmp)
		return nil, stderrBuf, fmt.Errorf("subject boot-reject: %w", rerr)
	}

	// Surprising success path: the subject DID come up. Return a cancel func
	// that kills+reaps+cleans the temp dir, and a nil err. addrs / subjAdminAddr
	// are intentionally unused on this path — the caller's boot-reject
	// assertion is going to t.Fatalf because boot did NOT reject.
	_ = addrs
	_ = subjAdminAddr
	return func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(tmp)
	}, stderrBuf, nil
}
