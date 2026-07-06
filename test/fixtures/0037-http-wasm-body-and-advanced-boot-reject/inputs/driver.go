// Package inputs registers the 0037-http-wasm-body-and-advanced-boot-reject
// fixture with the differential runner per phase 25.2 SPEC §6.4 + §8.2 +
// D-25.2-P1 closure at 25.2 IMPL Task 21 + PLAN Task 21.
//
// SUBJECT-ONLY boot-reject fixture: a `Wasm` filter config with
// `configuration.envoy_go_strict.body_buffer_cap_bytes = 0` must
// fail-closed at config-load on envoy-go, while reference Envoy v1.37.2
// BOOTS SUCCESSFULLY against the same config (the unknown
// `envoy_go_strict` extension key is silently dropped by upstream's
// protobuf Struct parser — upstream has no equivalent field).
//
// This exercises the 25.2 SPEC §6.2 arm-19 PARSE-REJECT path
// (subject-only). Per `reference_differential_fixture_dispatch_constraint`:
// one fixture dir = ONE runner branch. Fixture-0037 occupies the
// subject-only-boot-reject branch (NEW at 25.2; orthogonal to
// fixture-0035's symmetric-boot-reject branch + fixture-0036's
// cross-side mixed-mode branch).
//
// # Chosen arm (D-25.2-P1 closure at 25.2 IMPL Task 21 first-action)
//
// Per the empirical-scrape against the 6 candidate arms {19, 20, 21, 22,
// 23, 26} per SPEC §6.4:
//   - arm 19 (envoy-go-strict-body-buffer-cap-bytes-zero) — CHOSEN.
//     Distinctive substring `envoy_go_strict_body_buffer_cap_bytes`
//     (37 chars); deterministic single-field trigger
//     (`body_buffer_cap_bytes: 0`); upstream Envoy v1.37.2 has no
//     equivalent field (unknown extension drop).
//   - arms 20, 21, 22 — viable but longer substring + identical
//     trigger shape; arm 19 wins on parsimony.
//   - arm 23 (overlarge cap) — viable but substring overlaps with arm
//     19's; arm 19 wins on simplicity-of-invariant (zero vs > 1 GiB).
//   - arm 26 (duplicate plugin name) — viable but needs TWO listeners
//     with conflicting names; arm 19 wins on single-listener simplicity.
//
// See README.md "D-25.2-P1 closure" table for full disposition.
//
// # Runner-branch shape decision (per PLAN Task 21)
//
// Extended the existing `BootRejectFixture` runner branch with a
// sibling-interface opt-in (`SubjectOnlyBootRejectFixture` at
// test/differential/harness.go). Rationale: minimal infrastructure
// delta + preserves backwards compatibility with fixtures
// 0026/0029/0031/0033/0035 (none of which implement the new sibling
// interface, so they default to the symmetric boot-reject discipline
// unchanged). The sibling-interface choice follows the established
// pattern of `ReferenceLogMounter` / `MultiListenerDriver` /
// `StatsAsserter` / `ReferenceLessFixture` at
// test/differential/fixture/fixture.go.
//
// # Bytecode reuse
//
// The reference side needs a real, validly-compiled .wasm blob to boot
// successfully. We REUSE
// test/fixtures/0036-http-wasm-body-and-advanced/bytecode/a_body_read_only.wasm
// — bind-mounted into the reference container at /bytecode/probe.wasm.
// The blob is a Rust proxy-wasm plugin that compiles cleanly under both
// V8 (upstream) and wazero (subject) per Task 20 acceptance.
//
// The SUBJECT side never reads the .wasm blob — envoy-go's
// buildCompiledConfig orders cap-field validators (arms 19-23) BEFORE
// resolveDataSource per compiled_config.go lines 844-862. Arm 19 fires
// at the parseEnvoyGoStrictFields step before the .wasm file is opened.
//
// # Cross-references
//
//   - 25.2 SPEC §6.2 row 19 (parseRejectEnvoyGoStrictBodyBufferCapBytesZero
//     byte-stable wording)
//   - 25.2 SPEC §6.4 (D-25.2-P1 fixture-0037 single-arm boot-reject
//     finalization)
//   - 25.2 SPEC §8.2 (fixture-0037 subject-only boot-reject taxonomy)
//   - internal/filter/http/wasm/compiled_config.go arm 19 (constant +
//     fire-site at parseEnvoyGoStrictFields)
//   - test/differential/harness.go BootRejectFixture +
//     SubjectOnlyBootRejectFixture interfaces
//   - test/differential/runner_test.go runBootRejectFixture branch
//     (extended for subject-only dispatch at 25.2 Task 21)
//   - project memory `reference_differential_fixture_dispatch_constraint`
//   - fixture-0035 (sibling 25.1 symmetric-boot-reject precedent;
//     informs the inline-bootstrap-Option-B2 pattern)
//   - fixture-0036 (sibling 25.2 cross-side mixed-mode + .wasm blob
//     source)
//   - ADR-0008 (envoyproxy/envoy:v1.37.2 reference Envoy pin)
//   - ADR-0208 (NEW internal/filter/http/wasm/ 25.2 package extensions
//     — the envoy-go-strict config field family lands here)
package inputs

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0037-http-wasm-body-and-advanced-boot-reject"

	// refAdminPort / refLATestPort — container-internal ports for the
	// reference Envoy listener + admin surfaces (the container's port
	// publishing maps these to host-allocated ports at test time).
	refAdminPort  = 9901
	refLATestPort = 10137 // l_test_a — the single boot-reject listener.

	// refContainerBytecodePath — where the bind-mounted .wasm blob lands
	// inside the reference Envoy container. The subject side splices the
	// same path string into its bootstrap but envoy-go never opens the
	// file (arm 19 fires before resolveDataSource per compiled_config.go
	// ordering).
	refContainerBytecodePath = "/bytecode/probe.wasm"

	// bootRejectScriptDesc — BootRejectScript() return value. Like
	// fixtures 0031/0033/0035, this is a description (NOT a filesystem
	// path) — the trigger is embedded inline in the bootstrap. The
	// runner discards the return value; the side effect (flipping
	// bootRejectMode) is the meaningful signal.
	bootRejectScriptDesc = "inline wasm filter with envoy_go_strict.body_buffer_cap_bytes = 0 (§6.2 arm 19)"

	// expectedBootErrorSubstr — the literal substring the runner asserts
	// is present (case-sensitive Contains) in SUBJECT stderr after the
	// subject's boot-reject. Per D-25.2-P1 closure: arm 19's byte-stable
	// wording is:
	//   parseRejectEnvoyGoStrictBodyBufferCapBytesZero =
	//     "wasm: config.envoy_go_strict_body_buffer_cap_bytes must be > 0 (envoy-go-strict)"
	// The 37-character literal `envoy_go_strict_body_buffer_cap_bytes`
	// IS verbatim in the const + highly distinctive (no unrelated token
	// in subject stderr contains this substring). The reference stderr
	// is NOT checked under the SubjectOnlyBootRejectFixture discipline
	// — the reference boots successfully + carries no error wording to
	// substring-match.
	expectedBootErrorSubstr = "envoy_go_strict_body_buffer_cap_bytes"
)

func init() {
	fixture.RegisterFixture(fixtureName, &wasmAdvBootRejectDriver{})
}

// wasmAdvBootRejectDriver carries the boot-reject mode flag (flipped
// when the runner's runBootRejectFixture branch calls BootRejectScript()
// before re-rendering the bootstrap templates). Mirrors fixtures
// 0031 / 0033 / 0035 boot-reject driver shape.
type wasmAdvBootRejectDriver struct {
	mu             sync.Mutex
	bootRejectMode bool
}

// --- fixture.Driver (required) ---

func (*wasmAdvBootRejectDriver) BackendCount() int                { return 1 }
func (*wasmAdvBootRejectDriver) BackendKind() fixture.BackendKind { return fixture.HTTPWasmAdvanced }
func (*wasmAdvBootRejectDriver) SubjectListenerName() string      { return "l_test_a" }
func (*wasmAdvBootRejectDriver) ReferenceListenerPort() int       { return refLATestPort }

// ReferenceBootstrap returns the self-contained single-listener boot-
// reject bootstrap for the REFERENCE side (Envoy v1.37.2 with V8
// runtime). The filter's `configuration.envoy_go_strict.body_buffer_
// cap_bytes` field is 0 — but the entire `envoy_go_strict` key is
// silently dropped by upstream's protobuf Struct parser (upstream has
// no equivalent extension field). The wasm filter then loads the
// bind-mounted .wasm blob at /bytecode/probe.wasm — admin /ready
// returns 200.
func (d *wasmAdvBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refLATestPort, runtimeV8, refContainerBytecodePath)
}

// SubjectConfig renders the SUBJECT (envoy-go) side's boot-reject
// bootstrap. The runtime discriminator MUST be
// `envoy.wasm.runtime.wazero` per AMEND-A1 (envoy-go uses wazero
// exclusively). If `envoy.wasm.runtime.v8` were used, envoy-go's arm
// 11 (runtime discriminator) would fire BEFORE arm 19 per
// buildCompiledConfig's per-field walk order, producing a DIFFERENT
// byte-stable wording that breaks the substring assertion. The
// runner-allocated subjAdminPort + subjListenerPort splice into the
// admin/listener socket addresses; the .wasm filename is spliced for
// shape-symmetry with the reference side but envoy-go never reads it
// (arm 19 fires at buildCompiledConfig before resolveDataSource per
// compiled_config.go lines 844-862 ordering).
func (d *wasmAdvBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort, runtimeWazero, refContainerBytecodePath)
}

// DriveReference / DriveSubject / ProbeAdmin are required by the Driver
// interface but never invoked in the boot-reject branch (the runner
// SKIPS Drive + admin-diff for BootRejectFixture drivers).

func (*wasmAdvBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*wasmAdvBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*wasmAdvBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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

// BootRejectScript flips bootRejectMode and returns a description of
// the inline boot-reject trigger (NOT a filesystem path — this fixture
// embeds the trigger inline in renderBootRejectBootstrap). The runner
// discards the return value; the side effect is the signal.
func (d *wasmAdvBootRejectDriver) BootRejectScript() string {
	d.mu.Lock()
	d.bootRejectMode = true
	d.mu.Unlock()
	return bootRejectScriptDesc
}

// ExpectedBootErrorSubstring returns the literal substring the runner
// asserts is present (case-sensitive Contains) in SUBJECT stderr after
// boot-reject. Per D-25.2-P1 closure at 25.2 IMPL Task 21:
// "envoy_go_strict_body_buffer_cap_bytes" — verbatim 37-character
// fragment of the parseRejectEnvoyGoStrictBodyBufferCapBytesZero
// constant per 25.2 SPEC §6.2 row 19.
func (*wasmAdvBootRejectDriver) ExpectedBootErrorSubstring() string {
	return expectedBootErrorSubstr
}

// --- differential.SubjectOnlyBootRejectFixture ---

// SubjectOnly returns true — fixture-0037 is the subject-only-boot-
// reject branch per `reference_differential_fixture_dispatch_constraint`
// (one fixture dir = ONE runner branch). The reference Envoy v1.37.2
// side BOOTS SUCCESSFULLY against the same config (the unknown
// envoy_go_strict extension key is silently dropped by upstream's
// protobuf Struct parser); only the subject envoy-go boot-REJECTS.
func (*wasmAdvBootRejectDriver) SubjectOnly() bool { return true }

// --- fixture.ReferenceLogMounter ---
//
// Bind-mount the real .wasm blob into the reference container. The
// host-side path is the sibling fixture-0036's
// bytecode/a_body_read_only.wasm (a Rust proxy-wasm plugin that
// compiles cleanly under both V8 and wazero per Task 20 acceptance).
// The runner consults ReferenceLogMounter at the runBootRejectFixture
// branch (extended at 25.2 Task 21 to pre-create / honor existing
// host files + splice bind-mounts into tryStartReferenceProxy).
func (*wasmAdvBootRejectDriver) ReferenceHostMounts() []fixture.HostMount {
	return []fixture.HostMount{
		{
			HostPath:      sharedWasmBlobHostPath(),
			ContainerPath: refContainerBytecodePath,
		},
	}
}

// sharedWasmBlobHostPath returns the absolute path to the .wasm blob
// REUSED from sibling fixture-0036. Using runtime.Caller to locate
// this source file (driver.go) is the established discipline at
// fixture-0036/inputs/driver.go::fixtureDir; mirroring it here keeps
// the runtime-resolution shape consistent across the two siblings.
func sharedWasmBlobHostPath() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		// Defensive fallback — should never fire under the standard go test
		// invocation; if it does, the bind-mount will dangle + the reference
		// boot will fail with a file-not-found which propagates as a clear
		// test failure in the subject-only discipline.
		return ""
	}
	// thisFile == .../test/fixtures/0037-http-wasm-body-and-advanced-boot-reject/inputs/driver.go
	// fixturesRoot == .../test/fixtures/
	fixturesRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	return filepath.Join(fixturesRoot, "0036-http-wasm-body-and-advanced", "bytecode", "a_body_read_only.wasm")
}

// runtime discriminator strings spliced into the bootstrap per side.
const (
	runtimeV8     = "envoy.wasm.runtime.v8"
	runtimeWazero = "envoy.wasm.runtime.wazero"
)

// renderBootRejectBootstrap returns the self-contained single-listener
// bootstrap each proxy consumes. The wasm filter sets
// `configuration.envoy_go_strict.body_buffer_cap_bytes: 0` — this
// triggers arm 19 PARSE-REJECT on envoy-go's compiled_config.go +
// is silently dropped by upstream's protobuf Struct parser on
// reference Envoy v1.37.2.
//
// The `runtime` field is supplied PER SIDE: reference uses
// `envoy.wasm.runtime.v8` (upstream default) + subject uses
// `envoy.wasm.runtime.wazero` per AMEND-A1.
//
// A minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared
// so envoy-go's cluster manager runs before the listener manager and
// does not fail with a zero-endpoint error before the listener config-
// load reject. Same ordering sidestep as fixtures
// 0026 / 0029 / 0031 / 0033 / 0035.
func renderBootRejectBootstrap(adminPort, listenerPort int, runtime, wasmPath string) string {
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
                stat_prefix: hcm_bootreject_37
                route_config:
                  name: rc_bootreject_37
                  virtual_hosts:
                    - name: vh_bootreject_37
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_unused }
                http_filters:
                  - name: envoy.filters.http.wasm
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
                      config:
                        name: plugin_bootreject_37
                        root_id: rootid_bootreject_37
                        vm_config:
                          vm_id: vm_bootreject_37
                          runtime: %s
                          code:
                            local:
                              filename: %s
                        configuration:
                          "@type": type.googleapis.com/google.protobuf.Struct
                          value:
                            envoy_go_strict:
                              body_buffer_cap_bytes: 0
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
`, adminPort, listenerPort, runtime, wasmPath)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver              = (*wasmAdvBootRejectDriver)(nil)
	_ fixture.BackendKindAware    = (*wasmAdvBootRejectDriver)(nil)
	_ fixture.ReferenceLogMounter = (*wasmAdvBootRejectDriver)(nil)
)
