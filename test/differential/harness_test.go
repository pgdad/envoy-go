package differential

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseEnvoyTarget_PullsTagAndDigest(t *testing.T) {
	src := `# envoy-go Reference Envoy Pin

**Tag:** ` + "`envoyproxy/envoy:v1.34.0`" + `
**SHA256:** ` + "`sha256:abc123def456`" + `
`
	pin, err := parseEnvoyTarget(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parseEnvoyTarget: %v", err)
	}
	if pin.Tag != "envoyproxy/envoy:v1.34.0" {
		t.Errorf("Tag: got %q", pin.Tag)
	}
	if pin.SHA256 != "sha256:abc123def456" {
		t.Errorf("SHA256: got %q", pin.SHA256)
	}
}

func TestParseEnvoyTarget_RejectsMissingTag(t *testing.T) {
	src := "no tag here\n**SHA256:** `sha256:abc`\n"
	if _, err := parseEnvoyTarget(strings.NewReader(src)); err == nil {
		t.Fatalf("parseEnvoyTarget accepted input without Tag")
	}
}

func TestReferenceProxy_Starts(t *testing.T) {
	if testing.Short() {
		t.Skip("differential test; skipped under -short")
	}
	ensureDocker(t)

	pin := loadPinFromRepo(t)
	const cfg = `
admin: { address: { socket_address: { address: 0.0.0.0, port_value: 9901 } } }
`
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ref, err := StartReferenceProxy(ctx, pin, cfg)
	if err != nil {
		t.Fatalf("StartReferenceProxy: %v", err)
	}
	defer func() { _ = ref.Stop(context.Background()) }()
	if ref.AdminAddr() == "" {
		t.Errorf("AdminAddr empty")
	}
}

func ensureDocker(t *testing.T) {
	t.Helper()
	// Try canonical path first; Docker Desktop on Linux bind-mounts it or
	// exposes it at $HOME/.docker/desktop/docker.sock.
	paths := []string{
		"/var/run/docker.sock",
		os.Getenv("HOME") + "/.docker/desktop/docker.sock",
	}
	for _, p := range paths {
		if conn, err := net.Dial("unix", p); err == nil {
			_ = conn.Close()
			return
		}
	}
	t.Fatalf("docker unavailable: no reachable socket at %v", paths)
}

func loadPinFromRepo(t *testing.T) *EnvoyPin {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	f, err := os.Open(filepath.Join(repoRoot, "docs", "envoy-go", "ENVOY_TARGET.md"))
	if err != nil {
		t.Fatalf("open pin: %v", err)
	}
	defer func() { _ = f.Close() }()
	pin, err := parseEnvoyTarget(f)
	if err != nil {
		t.Fatalf("parse pin: %v", err)
	}
	return pin
}
