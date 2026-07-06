// Package inputs registers the 0039-http-wasm-perroute-boot-reject fixture
// with the differential runner per phase 25.3 Task 12 + D-25.3-P1 closure.
//
// SUBJECT-ONLY boot-reject fixture: a `Wasm` filter whose
// VmConfig.environment_variables.key_values carries MORE than 64 entries
// must fail-closed at config-load on envoy-go (the envoy-go-strict 64-entry
// cap — parseRejectEnvVarsCapExceeded), while reference Envoy v1.37.2 BOOTS
// SUCCESSFULLY against the same config (upstream has NO such cap — it accepts
// arbitrarily many environment_variables entries).
//
// This exercises the 25.3 PARSE-REJECT arm C (env_vars cap-exceeded;
// internal/filter/http/wasm/compiled_config.go parseRejectEnvVarsCapExceeded).
// Per `reference_differential_fixture_dispatch_constraint`: one fixture dir =
// ONE runner branch. Fixture-0039 occupies the subject-only-boot-reject
// branch (mirroring fixture-0037; orthogonal to fixture-0035's symmetric-
// boot-reject branch + fixture-0036/0038's cross-side branches).
//
// # Chosen arm (D-25.3-P1 closure at 25.3 IMPL Task 12 first-action)
//
// Two candidate arms were empirically scraped against reference Envoy
// v1.37.2 (Docker `envoyproxy/envoy:v1.37.2`):
//
//   - Arm A (cap-exceeded): VmConfig.environment_variables.key_values with
//     65 entries. Reference v1.37.2 BOOTS SUCCESSFULLY (admin /ready=200,
//     "starting main dispatch loop" — upstream has NO entry cap). envoy-go
//     boot-REJECTS with parseRejectEnvVarsCapExceeded. => SUBJECT-ONLY.
//     CHOSEN — cleanest subject-only asymmetry, mirrors fixture-0037's
//     "reference has no equivalent constraint" property, single-listener.
//
//   - Arm B (key collision): a key in BOTH host_env_keys AND key_values.
//     Reference v1.37.2 ALSO boot-REJECTS, but with a DIFFERENT byte-stable
//     wording: `Key <K> is duplicated in
//     envoy.extensions.wasm.v3.VmConfig.environment_variables for <plugin>.
//     All the keys must be unique.` => SYMMETRIC, but cross-side substrings
//     diverge (envoy-go's parseRejectEnvVarsKeyCollision wording differs).
//     NOT chosen — arm A is the cleaner subject-only fixture + matches the
//     0037 precedent shape; one-dir-one-branch forbids carrying both.
//
// See README.md "D-25.3-P1 closure" for the full empirical disposition.
//
// # Runner-branch shape decision
//
// Reuses the existing `BootRejectFixture` + `SubjectOnlyBootRejectFixture`
// sibling-interface dispatch at test/differential/harness.go (introduced at
// 25.2 Task 21 for fixture-0037). No infrastructure delta: this driver
// implements both interfaces + returns true from SubjectOnly(). The runner's
// runBootRejectFixture branch boots the reference SUCCESSFULLY (asserts
// cancel != nil, err == nil), tears it down, then asserts the subject
// boot-REJECTS with the substring in stderr.
//
// # Bytecode
//
// The reference side needs a real, validly-compiled .wasm blob to boot
// successfully. This fixture is SELF-CONTAINED: it vendors a minimal valid
// .wasm at 0039/bytecode/probe.wasm (copied from sibling fixture-0038's
// listener_default.wasm — a proxy-wasm plugin that compiles cleanly under
// both V8 (upstream) and wazero (subject)). The blob is bind-mounted into
// the reference container at /bytecode/probe.wasm via ReferenceLogMounter.
//
// The SUBJECT side never reads the .wasm blob — envoy-go's
// buildCompiledConfig parses VmConfig.environment_variables (arm C cap
// validator) at the parseEnvVars step before resolveDataSource opens the
// .wasm file; the cap-exceeded reject fires first.
//
// # Cross-references
//
//   - internal/filter/http/wasm/compiled_config.go parseRejectEnvVarsCapExceeded
//     (arm C byte-stable wording + parseEnvVars fire-site)
//   - internal/wasm/env_vars.go AssembleEnvVars / ErrEnvVarsCapExceeded
//     (envVarsMaxEntries = 64; envVarsMaxValueBytes = 4096)
//   - test/differential/harness.go BootRejectFixture +
//     SubjectOnlyBootRejectFixture interfaces
//   - test/differential/runner_test.go runBootRejectFixture branch
//   - project memory `reference_differential_fixture_dispatch_constraint`
//   - fixture-0037 (sibling 25.2 subject-only-boot-reject precedent + template)
//   - fixture-0038 (sibling 25.3 cross-side perroute fixture + .wasm source)
//   - ADR-0008 (envoyproxy/envoy:v1.37.2 reference Envoy pin)
package inputs

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0039-http-wasm-perroute-boot-reject"

	// refAdminPort / refLATestPort — container-internal ports for the
	// reference Envoy listener + admin surfaces (the container's port
	// publishing maps these to host-allocated ports at test time).
	refAdminPort  = 9901
	refLATestPort = 10139 // l_test_a — the single boot-reject listener.

	// refContainerBytecodePath — where the bind-mounted .wasm blob lands
	// inside the reference Envoy container. The subject side splices the
	// same path string into its bootstrap but envoy-go never opens the
	// file (arm C fires at parseEnvVars before resolveDataSource).
	refContainerBytecodePath = "/bytecode/probe.wasm"

	// envVarsEntryCount — the number of key_values entries spliced into the
	// VmConfig.environment_variables block. 65 = envVarsMaxEntries (64) + 1,
	// so the assembled env exceeds the envoy-go-strict cap by exactly one
	// entry — the minimal trigger for arm C. Reference Envoy v1.37.2 accepts
	// all 65 entries (no upstream cap), boots successfully.
	envVarsEntryCount = 65

	// bootRejectScriptDesc — BootRejectScript() return value. Like
	// fixtures 0031/0033/0035/0037, this is a description (NOT a filesystem
	// path) — the trigger is embedded inline in the bootstrap. The runner
	// discards the return value; the side effect (flipping bootRejectMode)
	// is the meaningful signal.
	bootRejectScriptDesc = "inline wasm filter with 65 environment_variables.key_values entries (arm C — envoy-go-strict 64-entry cap exceeded)"

	// expectedBootErrorSubstr — the literal substring the runner asserts is
	// present (case-sensitive Contains) in SUBJECT stderr after the
	// subject's boot-reject. Per D-25.3-P1 closure: arm C's byte-stable
	// wording is parseRejectEnvVarsCapExceeded =
	//   "wasm: config.vm_config.environment_variables exceeds the
	//    envoy-go-strict cap (max 64 entries, max 4096 bytes per value)"
	// The fragment `environment_variables exceeds the envoy-go-strict cap`
	// is verbatim in the const + highly distinctive (no unrelated token in
	// subject stderr contains it). The reference stderr is NOT checked under
	// the SubjectOnlyBootRejectFixture discipline — the reference boots
	// successfully + carries no error wording to substring-match.
	expectedBootErrorSubstr = "environment_variables exceeds the envoy-go-strict cap"
)

func init() {
	fixture.RegisterFixture(fixtureName, &wasmPerrouteBootRejectDriver{})
}

// wasmPerrouteBootRejectDriver carries the boot-reject mode flag (flipped
// when the runner's runBootRejectFixture branch calls BootRejectScript()
// before re-rendering the bootstrap templates). Mirrors fixtures
// 0031 / 0033 / 0035 / 0037 boot-reject driver shape.
type wasmPerrouteBootRejectDriver struct {
	mu             sync.Mutex
	bootRejectMode bool
}

// --- fixture.Driver (required) ---

func (*wasmPerrouteBootRejectDriver) BackendCount() int { return 1 }
func (*wasmPerrouteBootRejectDriver) BackendKind() fixture.BackendKind {
	return fixture.HTTPWasmAdvanced
}
func (*wasmPerrouteBootRejectDriver) SubjectListenerName() string { return "l_test_a" }
func (*wasmPerrouteBootRejectDriver) ReferenceListenerPort() int  { return refLATestPort }

// ReferenceBootstrap returns the self-contained single-listener boot-reject
// bootstrap for the REFERENCE side (Envoy v1.37.2 with V8 runtime). The
// filter's VmConfig.environment_variables.key_values carries 65 entries —
// upstream Envoy v1.37.2 has NO entry cap, so it accepts all 65 + the wasm
// filter loads the bind-mounted .wasm blob at /bytecode/probe.wasm — admin
// /ready returns 200.
func (d *wasmPerrouteBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refLATestPort, runtimeV8, refContainerBytecodePath)
}

// SubjectConfig renders the SUBJECT (envoy-go) side's boot-reject bootstrap.
// The runtime discriminator MUST be `envoy.wasm.runtime.wazero` per AMEND-A1
// (envoy-go uses wazero exclusively); a different runtime string would fire
// the runtime-discriminator arm BEFORE the env_vars cap arm + break the
// substring assertion. The runner-allocated subjAdminPort + subjListenerPort
// splice into the admin/listener socket addresses; the .wasm filename is
// spliced for shape-symmetry with the reference side but envoy-go never reads
// it (arm C fires at parseEnvVars before resolveDataSource).
func (d *wasmPerrouteBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort, runtimeWazero, refContainerBytecodePath)
}

// DriveReference / DriveSubject / ProbeAdmin are required by the Driver
// interface but never invoked in the boot-reject branch (the runner SKIPS
// Drive + admin-diff for BootRejectFixture drivers).

func (*wasmPerrouteBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*wasmPerrouteBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*wasmPerrouteBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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
// inline boot-reject trigger (NOT a filesystem path — this fixture embeds
// the trigger inline in renderBootRejectBootstrap). The runner discards the
// return value; the side effect is the signal.
func (d *wasmPerrouteBootRejectDriver) BootRejectScript() string {
	d.mu.Lock()
	d.bootRejectMode = true
	d.mu.Unlock()
	return bootRejectScriptDesc
}

// ExpectedBootErrorSubstring returns the literal substring the runner asserts
// is present (case-sensitive Contains) in SUBJECT stderr after boot-reject.
// Per D-25.3-P1 closure at 25.3 IMPL Task 12:
// "environment_variables exceeds the envoy-go-strict cap" — verbatim fragment
// of the parseRejectEnvVarsCapExceeded constant (arm C).
func (*wasmPerrouteBootRejectDriver) ExpectedBootErrorSubstring() string {
	return expectedBootErrorSubstr
}

// --- differential.SubjectOnlyBootRejectFixture ---

// SubjectOnly returns true — fixture-0039 is the subject-only-boot-reject
// branch per `reference_differential_fixture_dispatch_constraint` (one
// fixture dir = ONE runner branch). The reference Envoy v1.37.2 side BOOTS
// SUCCESSFULLY against the same config (upstream has NO env_vars entry cap);
// only the subject envoy-go boot-REJECTS on the envoy-go-strict 64-entry cap.
func (*wasmPerrouteBootRejectDriver) SubjectOnly() bool { return true }

// --- fixture.ReferenceLogMounter ---
//
// Bind-mount the real .wasm blob into the reference container. The host-side
// path is this fixture's OWN vendored bytecode/probe.wasm (self-contained;
// copied from sibling fixture-0038's listener_default.wasm — a proxy-wasm
// plugin that compiles cleanly under both V8 and wazero). The runner consults
// ReferenceLogMounter at the runBootRejectFixture branch to honor the existing
// host file + splice the bind-mount into tryStartReferenceProxy.
func (*wasmPerrouteBootRejectDriver) ReferenceHostMounts() []fixture.HostMount {
	return []fixture.HostMount{
		{
			HostPath:      vendoredWasmBlobHostPath(),
			ContainerPath: refContainerBytecodePath,
		},
	}
}

// vendoredWasmBlobHostPath returns the absolute path to this fixture's OWN
// vendored .wasm blob at 0039/bytecode/probe.wasm. Using runtime.Caller to
// locate this source file (driver.go) is the established discipline at
// fixture-0037/inputs/driver.go::sharedWasmBlobHostPath; mirroring it here.
func vendoredWasmBlobHostPath() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		// Defensive fallback — should never fire under the standard go test
		// invocation; if it does, the bind-mount dangles + the reference boot
		// fails with a file-not-found which propagates as a clear test failure.
		return ""
	}
	// thisFile == .../test/fixtures/0039-http-wasm-perroute-boot-reject/inputs/driver.go
	// fixtureRoot == .../test/fixtures/0039-http-wasm-perroute-boot-reject/
	fixtureRoot := filepath.Dir(filepath.Dir(thisFile))
	return filepath.Join(fixtureRoot, "bytecode", "probe.wasm")
}

// runtime discriminator strings spliced into the bootstrap per side.
const (
	runtimeV8     = "envoy.wasm.runtime.v8"
	runtimeWazero = "envoy.wasm.runtime.wazero"
)

// renderBootRejectBootstrap returns the self-contained single-listener
// bootstrap each proxy consumes. The wasm filter's
// VmConfig.environment_variables.key_values block carries envVarsEntryCount
// (65) entries — this triggers arm C PARSE-REJECT on envoy-go's
// compiled_config.go (envoy-go-strict 64-entry cap) + is accepted in full by
// reference Envoy v1.37.2 (no upstream cap).
//
// The `runtime` field is supplied PER SIDE: reference uses
// `envoy.wasm.runtime.v8` (upstream default) + subject uses
// `envoy.wasm.runtime.wazero` per AMEND-A1.
//
// A minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared so the
// cluster manager runs before the listener manager and does not fail with a
// zero-endpoint error before the listener config-load reject. Same ordering
// sidestep as fixtures 0026 / 0029 / 0031 / 0033 / 0035 / 0037.
func renderBootRejectBootstrap(adminPort, listenerPort int, rt, wasmPath string) string {
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
                stat_prefix: hcm_bootreject_39
                route_config:
                  name: rc_bootreject_39
                  virtual_hosts:
                    - name: vh_bootreject_39
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_unused }
                http_filters:
                  - name: envoy.filters.http.wasm
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
                      config:
                        name: plugin_bootreject_39
                        root_id: rootid_bootreject_39
                        vm_config:
                          vm_id: vm_bootreject_39
                          runtime: %s
                          code:
                            local:
                              filename: %s
                          environment_variables:
                            key_values:
%s
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
`, adminPort, listenerPort, rt, wasmPath, renderEnvVarsKeyValues(envVarsEntryCount))
}

// renderEnvVarsKeyValues emits `n` YAML key_values entries indented under the
// `key_values:` mapping (30 spaces — matching the bootstrap's nesting depth).
// Each entry is `kN: vN`. n=65 exceeds the envoy-go-strict 64-entry cap by
// one, the minimal arm-C trigger.
func renderEnvVarsKeyValues(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "                              k%d: v%d\n", i, i)
	}
	// Trim the trailing newline — the bootstrap template's `%s\n` (the line
	// after `key_values:`) already terminates the block.
	return strings.TrimRight(b.String(), "\n")
}

// Compile-time interface assertions.
var (
	_ fixture.Driver              = (*wasmPerrouteBootRejectDriver)(nil)
	_ fixture.BackendKindAware    = (*wasmPerrouteBootRejectDriver)(nil)
	_ fixture.ReferenceLogMounter = (*wasmPerrouteBootRejectDriver)(nil)
)
