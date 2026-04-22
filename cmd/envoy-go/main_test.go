package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/test/helpers"
)

// TestEnvoyGoBinary_EchoesThroughUpstream is a fast in-process integration
// test (no Docker, ~2s end-to-end) that runs under both `go test ./...` and
// `go test -short ./...` — i.e. on every CI run including the unit job. It is
// the only end-to-end exercise of the subject binary outside the differential
// suite; do not add a -short skip here.
func TestEnvoyGoBinary_EchoesThroughUpstream(t *testing.T) {
	// 1. Start an in-process echo backend on a random port.
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer func() { _ = backend.Close() }()
	go acceptEcho(backend)

	backendAddr := backend.Addr().(*net.TCPAddr)

	// 2. Pick a free port for the subject's listener.
	listenerPort := freeTCPPort(t)

	// 3. Build the subject binary into a temp file.
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// 4. Write the subject config.
	cfgPath := filepath.Join(tmp, "envoy-go.yaml")
	cfg := fmt.Sprintf(`
listener:
  address: 127.0.0.1
  port: %d
upstream:
  address: 127.0.0.1
  port: %d
`, listenerPort, backendAddr.Port)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// 5. Start the subject and wait for the ready sentinel.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	waitForReady(t, stdout, fmt.Sprintf("127.0.0.1:%d", listenerPort), 5*time.Second)

	// 6. Drive a payload through the subject and verify echo.
	resp, err := helpers.TCPRoundTrip(ctx,
		fmt.Sprintf("127.0.0.1:%d", listenerPort),
		[]byte("ping-7-fixture\n"), 500*time.Millisecond)
	if err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if string(resp) != "ping-7-fixture\n" {
		t.Errorf("got %q, want %q", resp, "ping-7-fixture\n")
	}
}

// echoConn wraps net.Conn to hide *net.TCPConn from io.Copy so it uses a
// heap buffer instead of splice(2). splice(fd, fd) (same fd as src and dst)
// returns 0 on Linux, causing a silent empty echo.
type echoConn struct{ net.Conn }

func acceptEcho(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) { defer func() { _ = c.Close() }(); _, _ = io.Copy(echoConn{c}, echoConn{c}) }(c)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitForReady(t *testing.T, r io.Reader, expectAddr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	br := bufio.NewReader(r)
	for time.Now().Before(deadline) {
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			t.Fatalf("ready: %v", err)
		}
		if strings.Contains(line, "envoy-go ready on "+expectAddr) {
			return
		}
	}
	t.Fatalf("ready sentinel not seen within %s", timeout)
}
