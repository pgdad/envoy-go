// Package inputs registers the 0031-http-admission-control-boot-reject
// fixture with the differential runner per phase-23 SPEC §7.2 + §5.1 +
// AMEND-8 + Task 9 brief. It is a BOOT-REJECT fixture: a
// sr_threshold.default_value < 1.0% config must fail-closed at config-load
// on BOTH reference Envoy v1.37.2 and envoy-go.
//
// This exercises the §5.1 arm 2 boot-reject path cross-side:
//   - Reference Envoy v1.37.2 rejects at config-load:
//     `"Success rate threshold cannot be less than 1.0%."`
//     (config.cc:25-27 per AMEND-8)
//   - envoy-go's compiled_config.go arm 2 rejects at buildCompiledConfig:
//     `"admission_control: sr_threshold cannot be less than 1.0%"`
//     (parseRejectSrThresholdTooLow constant per SPEC §5.1 row 2)
//
// Modeled EXACTLY on fixture-0029-http-lua-source-codes-boot-reject's
// BootRejectFixture mechanism. The runner's runBootRejectFixture branch
// calls BootRejectScript() once (the side-effect: bootRejectMode flipped),
// then renders BOTH bootstraps via ReferenceBootstrap + SubjectConfig,
// starts BOTH proxies via tryStart*, asserts BOTH fail to boot, and asserts
// a common substring (ExpectedBootErrorSubstring()) appears in BOTH stderr
// buffers.
//
// # Common boot-reject substring
//
// Per AMEND-8 + SPEC §5.1 row 2 empirical comparison:
//   - upstream stderr: "Success rate threshold cannot be less than 1.0%."
//   - envoy-go stderr: "admission_control: sr_threshold cannot be less than 1.0%"
//
// The common literal fragment both emit is "cannot be less than 1.0%"
// (case-sensitive substring present in both).
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap (Option B2 per fixture-0026/0029 precedent):
// the sr_threshold.default_value is embedded directly in the bootstrap YAML.
// No host-mount or file reference is needed. The runner's tryStartReferenceProxy
// does NOT consult ReferenceLogMounter, so inline bootstraps are the correct
// approach for boot-reject fixtures.
//
// A minimal upstream cluster (c_unused; 127.0.0.1:1 — never dialed) is
// declared so envoy-go's cluster manager (which runs BEFORE the listener
// manager) does not fail with a zero-endpoint error before the listener-
// manager config-load reject fires. Same ordering sidestep as fixture-0026
// + 0029.
//
// # Cross-references
//
//   - SPEC §7.2 (boot-reject fixture scope)
//   - SPEC §5.1 row 2 (sr_threshold < 1.0% reject; byte-stable wording)
//   - AMEND-8 (boot-reject roster; upstream config.cc:25-27 wording)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - fixture-0029 (nearest BootRejectFixture precedent)
//   - fixture-0026 (original BootRejectFixture precedent)
package inputs

import (
	"context"
	"fmt"
	"sync"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0031-http-admission-control-boot-reject"

	refAdminPort  = 9901
	refLATestPort = 10131 // l_test_a — the single boot-reject listener.

	// BootRejectScript() return value. UNLIKE fixture-0029 (whose return
	// value names a real on-disk symmetry artifact, scripts/bad_compile.lua),
	// this fixture embeds the boot-reject trigger entirely inline in
	// renderBootRejectBootstrap (sr_threshold.default_value.value=0.5) — there
	// is NO on-disk script file. This constant is therefore a description, not
	// a filesystem path: the runner discards the return value, and the side
	// effect (flipping bootRejectMode) is the meaningful signal.
	bootRejectScriptDesc = "inline sr_threshold.default_value.value=0.5 (< 1.0%)"

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr after
	// boot-reject. Determined empirically per AMEND-8 + SPEC §5.1 row 2:
	//
	//   reference Envoy v1.37.2 stderr:
	//     "Success rate threshold cannot be less than 1.0%."
	//     (config.cc:25-27)
	//   envoy-go stderr:
	//     "admission_control: sr_threshold cannot be less than 1.0%"
	//     (parseRejectSrThresholdTooLow constant in compiled_config.go)
	//
	// The common fragment both emit is "cannot be less than 1.0%".
	expectedBootErrorSubstr = "cannot be less than 1.0%"
)

func init() {
	fixture.RegisterFixture(fixtureName, &acBootRejectDriver{})
}

// acBootRejectDriver carries the boot-reject mode flag (flipped when the
// runner's runBootRejectFixture branch calls BootRejectScript() before
// re-rendering the bootstrap templates). Mirrors fixture-0029's luaDriver
// shape.
type acBootRejectDriver struct {
	mu             sync.Mutex
	bootRejectMode bool
}

// --- fixture.Driver (required) ---

func (*acBootRejectDriver) BackendCount() int                { return 1 }
func (*acBootRejectDriver) BackendKind() fixture.BackendKind { return fixture.HTTPAdmissionControl }
func (*acBootRejectDriver) SubjectListenerName() string      { return "l_test_a" }
func (*acBootRejectDriver) ReferenceListenerPort() int       { return refLATestPort }

// ReferenceBootstrap returns the self-contained single-listener boot-reject
// bootstrap once the runner has flipped bootRejectMode via BootRejectScript().
// The sr_threshold.default_value is set to 0.5 (< 1.0%) which triggers the
// §5.1 arm 2 PARSE-REJECT on both sides.
func (d *acBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refLATestPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side. The runner-
// allocated subjAdminPort splices into the admin socket address.
func (d *acBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject / ProbeAdmin are required by the Driver
// interface but never invoked in the boot-reject branch (the runner SKIPS
// Drive + admin-diff for BootRejectFixture drivers).

func (*acBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*acBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (*acBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
	refBytes, err := helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err := helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- differential.BootRejectFixture ---

// BootRejectScript flips bootRejectMode and returns a description of the
// inline boot-reject trigger (NOT a filesystem path — this fixture has no
// on-disk script). The runner discards the return value; the side effect is
// the signal.
func (d *acBootRejectDriver) BootRejectScript() string {
	d.mu.Lock()
	d.bootRejectMode = true
	d.mu.Unlock()
	return bootRejectScriptDesc
}

// ExpectedBootErrorSubstring returns the literal substring the runner asserts
// is present (case-sensitive Contains) in BOTH ref + subj stderr.
// Per AMEND-8 + SPEC §5.1 row 2: "cannot be less than 1.0%" appears in:
//   - upstream: "Success rate threshold cannot be less than 1.0%."
//   - envoy-go: "admission_control: sr_threshold cannot be less than 1.0%"
func (*acBootRejectDriver) ExpectedBootErrorSubstring() string { return expectedBootErrorSubstr }

// renderBootRejectBootstrap returns the self-contained single-listener
// bootstrap BOTH proxies consume. The admission_control filter carries a
// sr_threshold.default_value of 0.5 (< 1.0%) — this triggers §5.1 arm 2
// PARSE-REJECT on config-load on both reference Envoy + envoy-go.
//
// A minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared so
// envoy-go's cluster manager runs before the listener manager and does not
// fail with a zero-endpoint error before the listener config-load reject.
// Same ordering sidestep as fixture-0026 + 0029.
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

static_resources:
  listeners:
    - name: l_test_a
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: hcm_bootreject
                route_config:
                  name: rc_bootreject
                  virtual_hosts:
                    - name: vh_bootreject
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_unused }
                http_filters:
                  - name: envoy.filters.http.admission_control
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.admission_control.v3.AdmissionControl
                      sr_threshold:
                        default_value:
                          value: 0.5
                      success_criteria:
                        http_criteria:
                          http_success_status:
                            - { start: 100, end: 500 }
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, listenerPort)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*acBootRejectDriver)(nil)
	_ fixture.BackendKindAware = (*acBootRejectDriver)(nil)
)
