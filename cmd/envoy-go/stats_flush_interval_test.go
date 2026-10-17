package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// flushValidateYAML is the TestEnvoyGoBinary_ModeValidate good config, with a
// stats_flush_interval line appended (phase 102).
const flushValidateYAML = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`

// TestEnvoyGoBinary_ModeValidate_StatsFlushInterval runs --mode validate on
// the built binary: 0s and -1s exit 1 naming their reference class; 0.001s
// (the lower bound) validates.
func TestEnvoyGoBinary_ModeValidate_StatsFlushInterval(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	cases := []struct {
		value, wantErr string // wantErr "" means exit 0 + "configuration OK"
	}{
		{"0s", "value must be inside range [1ms, 5m0s)"},
		{"-1s", "Invalid duration: Expected positive duration"},
		{"0.001s", ""},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "cfg.yaml")
			if err := os.WriteFile(cfg, []byte(flushValidateYAML+"stats_flush_interval: "+c.value+"\n"), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, bin, "-c", cfg, "--mode", "validate").CombinedOutput()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("--mode validate failed: %v\n%s", err, out)
				}
				if !strings.Contains(string(out), "configuration OK") {
					t.Errorf("stdout = %q, want it to contain %q", out, "configuration OK")
				}
				return
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("got err=%v, want *exec.ExitError with exit code 1 (out=%s)", err, out)
			}
			if !strings.Contains(string(out), c.wantErr) {
				t.Errorf("output = %q, want it to contain %q", out, c.wantErr)
			}
		})
	}
}
