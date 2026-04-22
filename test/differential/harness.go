package differential

import (
	"bufio"
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

// scanForLine reads lines from r until one of `needle` substrings appears or
// ctx is done. Returns the matching full line.
func scanForLine(ctx context.Context, r io.Reader, needle string) (string, error) {
	br := bufio.NewReader(r)
	out := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		for {
			line, err := br.ReadString('\n')
			if strings.Contains(line, needle) {
				out <- line
				return
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()
	select {
	case line := <-out:
		return line, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
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
		Cmd:          []string{"envoy", "--config-yaml", bootstrap, "--log-level", "warn"},
		WaitingFor:   wait.ForHTTP("/ready").WithPort("9901/tcp").WithStartupTimeout(readyTimeout),
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
	cmd          *exec.Cmd
	listenerAddr string
	tmpDir       string
}

// StartSubjectProxy builds cmd/envoy-go from repoRoot, writes cfg to a temp
// file, starts the subject as a subprocess, waits for the ready sentinel, and
// returns a handle. The harness owns the subprocess lifetime; callers must
// call Stop to release.
func StartSubjectProxy(ctx context.Context, repoRoot, cfg string) (*SubjectProxy, error) {
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
	line, err := scanForLine(readyCtx, stdout, "envoy-go ready on ")
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("subject ready: %w", err)
	}
	addr := readyAddr(line)
	if addr == "" {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("subject ready line malformed: %q", line)
	}

	return &SubjectProxy{cmd: cmd, listenerAddr: addr, tmpDir: tmp}, nil
}

// ListenerAddr returns the host:port the subject is listening on (parsed from
// the ready sentinel).
func (s *SubjectProxy) ListenerAddr() string { return s.listenerAddr }

// Stop kills and reaps the subject and cleans up its temp directory.
func (s *SubjectProxy) Stop() error {
	_ = s.cmd.Process.Kill()
	_, _ = s.cmd.Process.Wait()
	return os.RemoveAll(s.tmpDir)
}

func readyAddr(line string) string {
	const prefix = "envoy-go ready on "
	i := strings.Index(line, prefix)
	if i < 0 {
		return ""
	}
	return strings.TrimRight(line[i+len(prefix):], "\r\n")
}

// FixtureDriver is re-exported from the fixture sub-package for callers that
// imported it from here before the refactor. New code should import
// test/differential/fixture directly.
type FixtureDriver = fixture.FixtureDriver

// RegisterFixture re-exports fixture.RegisterFixture for backward compat.
func RegisterFixture(name string, d FixtureDriver) { fixture.RegisterFixture(name, d) }
