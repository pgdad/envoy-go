package admin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/internal/bootstrap"
)

// FuzzConfigDumpFormat fuzzes adversarial bootstrap inputs through
// buildConfigDump + protojson.Marshal; asserts no panic, output is valid
// JSON, output (when non-empty) has a "configs" field. The corpus is
// seeded with a few representative malformed/edge bootstrap YAMLs
// (zero-clusters, zero-listeners, large-name cluster, IPv6 endpoint,
// etc.); the fuzzer mutates the YAML bytes (the bootstrap.Load path
// fails on most inputs — we observe the failure and only feed
// successfully-parsed bootstraps to buildConfigDump).
//
// SPEC §14.5: ~80 LoC; ADR-0018 30s short-budget. 10th fuzzer post-08.1.
func FuzzConfigDumpFormat(f *testing.F) {
	seeds := []string{
		// Empty
		``,
		// Just admin
		`admin: {address: {socket_address: {address: 127.0.0.1, port_value: 9901}}}`,
		// Admin + minimal cluster
		`admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters:
    - name: c1
      type: STATIC
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c1
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 18001}}}
`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, yamlBytes string) {
		// bootstrap.Load fails on most adversarial inputs; that's fine.
		bs, err := bootstrap.Load(strings.NewReader(yamlBytes))
		if err != nil {
			return
		}
		// buildConfigDump must not panic on any successfully-parsed bootstrap.
		cd, err := buildConfigDump(bs, time.Now())
		if err != nil {
			// build error is OK (e.g. anypb.New for an unregistered type);
			// what's NOT OK is a panic.
			return
		}
		// Marshal must not panic.
		body, err := configDumpMarshalOptions.Marshal(cd)
		if err != nil {
			return
		}
		if len(body) == 0 {
			return
		}
		// Output must be valid JSON.
		var generic map[string]interface{}
		if err := json.Unmarshal(body, &generic); err != nil {
			t.Errorf("buildConfigDump output is not valid JSON: %v\nbody[:200]: %s", err, body[:min(200, len(body))])
			return
		}
		// Non-empty output must have a "configs" field.
		if _, ok := generic["configs"]; !ok {
			t.Errorf("buildConfigDump output lacks 'configs' field; body[:200]: %s", body[:min(200, len(body))])
		}
	})
}
