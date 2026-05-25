// Package inputs registers the 0035-http-wasm-boot-reject fixture with
// the differential runner per phase 25.1 SPEC §9.2 + PLAN Task 16 + D-P6.
// It is a BOOT-REJECT fixture: a `Wasm` filter config with
// `vm_config.code.local` PRESENT but `specifier` oneof UNSET must
// fail-closed at config-load on BOTH reference Envoy v1.37.2 AND envoy-go.
//
// This exercises the §9.2 arm 8 boot-reject path cross-side:
//   - Reference Envoy v1.37.2 PGV-rejects at config-load with the
//     full AsyncDataSourceValidationError.Local chain ending in
//     `caused by field: "specifier", reason: is required` (the upstream
//     AsyncDataSource proto pins `validate.rules.message.required = true`
//     on the `specifier` oneof).
//   - envoy-go's compiled_config.go arm 8 rejects at buildCompiledConfig
//     with the byte-stable wording
//     `"wasm: config.vm_config.code.local: specifier oneof required"`
//     (parseRejectDataSourceSpecifierRequired const per §9.2 row 8).
//
// Modeled EXACTLY on fixture-0033-http-ratelimit-boot-reject's
// BootRejectFixture mechanism (which itself follows the fixture-0029 /
// 0031 inline-bootstrap precedent). The runner's runBootRejectFixture
// branch (runner_test.go) calls BootRejectScript() once, then renders
// BOTH bootstraps via ReferenceBootstrap + SubjectConfig, starts BOTH
// proxies via tryStart*, asserts BOTH fail to boot, and asserts a common
// substring (ExpectedBootErrorSubstring()) appears in BOTH stderr
// buffers.
//
// # Common boot-reject substring (D-P6)
//
// Per the empirical capture finalized at Task 16:
//   - reference Envoy v1.37.2 stderr (PGV violation; full chain):
//     "Proto constraint validation failed (WasmValidationError.Config:
//     embedded message failed validation | caused by
//     PluginConfigValidationError.VmConfig: embedded message failed
//     validation | caused by VmConfigValidationError.Code: embedded
//     message failed validation | caused by
//     AsyncDataSourceValidationError.Local: embedded message failed
//     validation | caused by field: \"specifier\", reason: is required)"
//   - envoy-go stderr (parseRejectDataSourceSpecifierRequired):
//     "listener manager: listener: 'l_test_a': filter_chains[0]: hcm:
//     http_filters[0]: factory: wasm: config.vm_config.code.local:
//     specifier oneof required"
//
// Both load-bearing wordings carry the proto oneof name `specifier`
// VERBATIM (case-identical: lowercase in both sources). The runner's
// substring assertion is case-sensitive (`strings.Contains`); the
// 9-character literal `specifier` IS shared and is highly distinctive
// (no unrelated token in either stderr contains this substring). This
// is the canonical substring finalized for D-P6.
//
// # Arm choice rationale (D-P6 selection)
//
// The 25.1 SPEC §12 D-P6 enumerates arm candidates {3, 4, 5, 8, 17};
// PLAN Task 16 anticipated arm 5 with substring "required". The
// empirical capture against `envoyproxy/envoy:v1.37.2` (ADR-0008)
// surfaced that arms 3, 4, 5 all collapse to the opaque upstream
// wrapper string `"Unable to create Wasm plugin <name>"`
// (`source/extensions/common/wasm/wasm.cc:467`) WITHOUT field-name
// detail — common substring vs envoy-go's `"wasm: config.vm_config.
// code is required"` is at best `"Wasm"` / `"wasm"` (case-mismatched)
// or the generic `"required"`. Arm 8 trips PGV field-level validation
// AHEAD of the wrapper string, producing the full validation-error
// chain whose tail contains the field oneof name `specifier` —
// matched verbatim by envoy-go's arm 8 wording. The deviation from
// the anticipated arm 5 / substring "required" is documented here +
// in PROGRESS.md Task 16 D-P6 closure evidence.
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap (Option B2 per fixture-0029 / 0031 /
// 0033 precedent). The `vm_config.code.local: {}` trigger is embedded
// directly in the bootstrap rendered by renderBootRejectBootstrap. No
// host-mount or file reference is needed — the boot-reject fires at
// config-load BEFORE any .wasm bytecode is read.
//
// A minimal upstream cluster (c_unused; 127.0.0.1:1 — never dialed) is
// declared so envoy-go's cluster manager (which runs BEFORE the listener
// manager) does not fail with a zero-endpoint error before the listener-
// manager config-load reject fires. Same ordering sidestep as fixtures
// 0026 / 0029 / 0031 / 0033.
//
// # Runtime asymmetry across the two bootstraps
//
// envoy-go's compiled_config.go orders arm 11 (runtime discriminator)
// BEFORE arm 8 (specifier oneof) per the per-field walk. The
// subject-side bootstrap MUST use either an empty runtime string OR
// `envoy.wasm.runtime.wazero` per AMEND-A1 — otherwise arm 11 would
// fire first and produce a DIFFERENT byte-stable wording that breaks
// the cross-side `"specifier"` substring assertion. Conversely, the
// reference-side bootstrap MUST use `envoy.wasm.runtime.v8` (the
// upstream default) — the upstream Envoy v1.37.2 image rejects the
// `envoy.wasm.runtime.wazero` string as an unknown runtime extension
// BEFORE PGV's specifier validation runs. The driver therefore
// renders TWO distinct bootstrap strings (one per side); the trigger
// shape (`code.local: {}`) is identical.
//
// # Cross-references
//
//   - parent SPEC §9.2 (boot-reject fixture scope)
//   - 25.1 SPEC §9.2 row 8 (specifier oneof PARSE-REJECT; byte-stable wording)
//   - 25.1 SPEC §12 D-P6 (boot-reject common substring; empirically settled here)
//   - 25.1 PLAN Task 16 + D-P6 (boot-reject common stderr substring)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - ADR-0008 (`envoyproxy/envoy:v1.37.2` reference Envoy pin)
//   - fixture-0033 (nearest BootRejectFixture precedent; ratelimit)
//   - fixture-0031 (BootRejectFixture precedent; admission_control)
//   - fixture-0029 (lua source_codes BootRejectFixture precedent)
package inputs

import (
	"context"
	"fmt"
	"sync"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0035-http-wasm-boot-reject"

	refAdminPort  = 9901
	refLATestPort = 10135 // l_test_a — the single boot-reject listener.

	// BootRejectScript() return value. UNLIKE fixture-0029 (whose return
	// value names a real on-disk symmetry artifact, scripts/bad_compile.lua),
	// this fixture embeds the boot-reject trigger entirely inline in
	// renderBootRejectBootstrap (the filter's `vm_config.code.local` is set
	// to an empty map; the specifier oneof inside is unset) — there is NO
	// on-disk .wasm file or other host-side artifact. This constant is
	// therefore a description, not a filesystem path: the runner discards
	// the return value, and the side effect (flipping bootRejectMode) is
	// the meaningful signal. Mirrors fixtures 0031 / 0033 shape.
	bootRejectScriptDesc = "inline wasm filter with empty `vm_config.code.local` map (specifier oneof unset; §9.2 arm 8)"

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr after
	// boot-reject. Finalized empirically at Task 16 per D-P6:
	//
	//   reference Envoy v1.37.2 stderr (PGV violation; full chain):
	//     "Proto constraint validation failed (WasmValidationError.Config:
	//     embedded message failed validation | caused by
	//     PluginConfigValidationError.VmConfig: embedded message failed
	//     validation | caused by VmConfigValidationError.Code: embedded
	//     message failed validation | caused by
	//     AsyncDataSourceValidationError.Local: embedded message failed
	//     validation | caused by field: \"specifier\", reason: is required)"
	//   envoy-go stderr (parseRejectDataSourceSpecifierRequired):
	//     "listener manager: listener: 'l_test_a': filter_chains[0]: hcm:
	//     http_filters[0]: factory: wasm: config.vm_config.code.local:
	//     specifier oneof required"
	//
	// The two load-bearing wordings both carry the proto oneof name
	// `specifier` verbatim (case-identical lowercase). Because the runner's
	// substring assertion is case-sensitive (`strings.Contains`), the
	// 9-character literal `specifier` IS shared and is highly distinctive
	// (no unrelated token in either stderr contains this substring).
	//
	// This is the same byte-stable cross-side substring discipline as
	// fixture-0033 ("omain"), fixture-0031 ("cannot be less than 1.0%"),
	// fixture-0029 ("near '-'"), and fixture-0026 ("script load error").
	// Per AMEND-10 option 2: the substring need only appear ANYWHERE in
	// stderr (not be a prefix / regex / case-insensitive match).
	expectedBootErrorSubstr = "specifier"
)

func init() {
	fixture.RegisterFixture(fixtureName, &wasmBootRejectDriver{})
}

// wasmBootRejectDriver carries the boot-reject mode flag (flipped when the
// runner's runBootRejectFixture branch calls BootRejectScript() before
// re-rendering the bootstrap templates). Mirrors fixtures 0031 / 0033
// boot-reject driver shape.
type wasmBootRejectDriver struct {
	mu             sync.Mutex
	bootRejectMode bool
}

// --- fixture.Driver (required) ---

func (*wasmBootRejectDriver) BackendCount() int                { return 1 }
func (*wasmBootRejectDriver) BackendKind() fixture.BackendKind { return fixture.HTTPWasm }
func (*wasmBootRejectDriver) SubjectListenerName() string      { return "l_test_a" }
func (*wasmBootRejectDriver) ReferenceListenerPort() int       { return refLATestPort }

// ReferenceBootstrap returns the self-contained single-listener boot-reject
// bootstrap for the REFERENCE side (Envoy v1.37.2 with V8 runtime) once the
// runner has flipped bootRejectMode via BootRejectScript(). The filter's
// `vm_config.code.local` is set to `{}` (specifier oneof unset) which
// triggers the §9.2 arm 8 PARSE-REJECT on both sides — but the runtime
// discriminator MUST be `envoy.wasm.runtime.v8` (upstream default) for
// reference Envoy to fall through to PGV validation rather than failing
// earlier with an unknown-runtime extension lookup error.
func (d *wasmBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refLATestPort, runtimeV8)
}

// SubjectConfig renders the SUBJECT (envoy-go) side's boot-reject bootstrap.
// The runtime discriminator MUST be `envoy.wasm.runtime.wazero` per AMEND-A1
// (envoy-go uses wazero exclusively). If `envoy.wasm.runtime.v8` were used,
// envoy-go's arm 11 (runtime discriminator) would fire BEFORE arm 8
// (specifier oneof) per buildCompiledConfig's per-field walk order, producing
// a DIFFERENT byte-stable wording that breaks the cross-side `"specifier"`
// substring assertion. The runner-allocated subjAdminPort + subjListenerPort
// splice into the admin/listener socket addresses.
func (d *wasmBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort, runtimeWazero)
}

// DriveReference / DriveSubject / ProbeAdmin are required by the Driver
// interface but never invoked in the boot-reject branch (the runner SKIPS
// Drive + admin-diff for BootRejectFixture drivers).

func (*wasmBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*wasmBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*wasmBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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
// on-disk .wasm file or other host-side artifact). The runner discards the
// return value; the side effect is the signal.
func (d *wasmBootRejectDriver) BootRejectScript() string {
	d.mu.Lock()
	d.bootRejectMode = true
	d.mu.Unlock()
	return bootRejectScriptDesc
}

// ExpectedBootErrorSubstring returns the literal substring the runner asserts
// is present (case-sensitive Contains) in BOTH ref + subj stderr.
// Per D-P6 + the empirical capture at Task 16: "specifier" appears in:
//   - upstream: "...AsyncDataSourceValidationError.Local: embedded message failed validation | caused by field: \"specifier\", reason: is required"
//     (lowercase `specifier` — proto oneof name verbatim)
//   - envoy-go: "wasm: config.vm_config.code.local: specifier oneof required"
//     (lowercase `specifier` — proto oneof name verbatim)
func (*wasmBootRejectDriver) ExpectedBootErrorSubstring() string { return expectedBootErrorSubstr }

// runtime discriminator strings spliced into the bootstrap per side.
const (
	runtimeV8     = "envoy.wasm.runtime.v8"
	runtimeWazero = "envoy.wasm.runtime.wazero"
)

// renderBootRejectBootstrap returns the self-contained single-listener
// bootstrap each proxy consumes. The wasm filter sets
// `vm_config.code.local: {}` (specifier oneof unset) — this triggers
// §9.2 arm 8 PARSE-REJECT on config-load on both reference Envoy + envoy-go.
//
// The `runtime` field is supplied PER SIDE: reference uses
// `envoy.wasm.runtime.v8` (upstream default; required for upstream PGV
// validation to reach the specifier check) and subject uses
// `envoy.wasm.runtime.wazero` per AMEND-A1 (required for envoy-go's
// per-field walk to fall through arm 11 to arm 8).
//
// A minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared so
// envoy-go's cluster manager runs before the listener manager and does not
// fail with a zero-endpoint error before the listener config-load reject.
// Same ordering sidestep as fixtures 0026 / 0029 / 0031 / 0033.
func renderBootRejectBootstrap(adminPort, listenerPort int, runtime string) string {
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
                  - name: envoy.filters.http.wasm
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
                      config:
                        name: plugin_bootreject
                        root_id: rootid_bootreject
                        vm_config:
                          vm_id: vm_bootreject
                          runtime: %s
                          code:
                            local: {}
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
`, adminPort, listenerPort, runtime)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*wasmBootRejectDriver)(nil)
	_ fixture.BackendKindAware = (*wasmBootRejectDriver)(nil)
)
