// Package inputs registers the 0029-http-lua-source-codes-boot-reject
// fixture with the differential runner per phase 22.3 IMPL Task 5. It is a
// BOOT-REJECT fixture: a `source_codes` entry carrying a COMPILE-ERROR
// script must fail-closed at config-load on BOTH reference Envoy v1.37.2 and
// envoy-go (scenario g).
//
// This exercises the NEW 22.3 `source_codes` consume path failing closed,
// cross-side:
//   - Reference Envoy's FilterConfig ctor eagerly compiles each source_codes
//     entry → compile error → boot reject.
//   - envoy-go's Task 1 consume path calls internallua.CompileScript on each
//     source_codes entry → compile error → boot reject.
//
// Modeled EXACTLY on fixture-0026-http-lua-headers-bridge's BootRejectFixture
// mechanism. The runner's runBootRejectFixture branch (runner_test.go) calls
// BootRejectScript() once, then renders BOTH bootstraps via ReferenceBootstrap
// + SubjectConfig, starts BOTH proxies via tryStart*, asserts BOTH fail to
// boot, and asserts a common substring (ExpectedBootErrorSubstring()) appears
// in BOTH stderr buffers.
//
// Per the fixture-0026 precedent: the boot-reject bootstrap is SELF-CONTAINED
// and embeds the broken Lua source via the DataSource `inline_string` arm —
// NOT a host-mounted Filename. runBootRejectFixture calls
// tryStartReferenceProxy directly, which does NOT consult ReferenceLogMounter,
// so a Filename-arm bootstrap would fail with "Invalid path" before the lua
// filter ever PARSE-REJECTed. The ONLY structural difference from 0026 is
// that the broken script is placed in a `source_codes{bad: ...}` entry (the
// NEW 22.3 surface) rather than `default_source_code` — the listener's
// `default_source_code` is a VALID no-op so the boot-reject is attributable
// solely to the broken source_codes entry.
//
// The (f) source_codes key-empty + (h) per-route source_code DataSource-
// failure scenarios from the PLAN are NOT built here: (f) is subject-only
// (upstream accepts an empty key, so it cannot be a both-fail BootRejectFixture)
// and both are already covered byte-exact by the Task 1 + Task 2 unit
// PARSE-REJECT tests + Task 4 fuzzing.
package inputs

import (
	"context"
	"fmt"
	"sync"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0029-http-lua-source-codes-boot-reject"

	refAdminPort  = 9901
	refLATestPort = 10130 // l_test_a — the single boot-reject listener.

	// BootRejectScript() return value (relative-to-fixture-dir path of the
	// on-disk broken script). The runner discards this; the side effect
	// (flipping bootRejectMode) is the meaningful signal. The on-disk file
	// is a documentation/symmetry artifact — the actual wire payload is
	// bootRejectInlineSource below.
	bootRejectScriptRelPath = "scripts/bad_compile.lua"

	// Inline source the boot-reject bootstraps embed via the source_codes
	// `bad` entry's DataSource inline_string arm. Byte-equivalent to
	// scripts/bad_compile.lua's final line. The trailing tokens after `end`
	// are NOT valid Lua 5.1 syntax → both LuaJIT (reference) + gopher-lua
	// (subject) PARSE-REJECT at config-load when this is a source_codes
	// entry.
	bootRejectInlineSource = "function envoy_on_request(handle) end this-is-not-valid-lua-syntax"

	// A VALID no-op default_source_code so the listener's only config error
	// is the broken source_codes[bad] entry — the boot-reject is attributable
	// solely to the NEW 22.3 source_codes consume path.
	validDefaultSource = "function envoy_on_request(handle) end"

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr after
	// boot-reject. Determined EMPIRICALLY by running both proxies against the
	// source_codes compile-error bootstrap:
	//
	//   reference Envoy (LuaJIT) stderr:
	//     script load error: [string "function envoy_on_request(handle) end this-is..."]:1: '=' expected near '-'
	//   envoy-go (gopher-lua) stderr:
	//     ... lua: source_codes["bad"]: lua compile: lua_filter_chunk line:1(column:43) near '-':   parse error
	//
	// The common literal fragment both parsers emit is `near '-'` (the parser
	// points at the bad `-` token in `this-is-not-valid-lua-syntax`). The
	// upstream `script load error` wrap is NOT shared: envoy-go's
	// maybeWrapLuaScriptLoadError only adds that prefix for the
	// `default_source_code` arm (luaCompileErrorSubstring is
	// `lua: default_source_code: compile:`), NOT for the source_codes arm —
	// so `near '-'` is the genuinely-common needle for THIS case.
	expectedBootErrorSubstr = "near '-'"
)

func init() {
	fixture.RegisterFixture(fixtureName, &luaDriver{})
}

// luaDriver carries the boot-reject mode flag (flipped when the runner's
// runBootRejectFixture branch calls BootRejectScript() before re-rendering
// the bootstrap templates). Mirrors fixture-0026's driver shape.
type luaDriver struct {
	mu             sync.Mutex
	bootRejectMode bool
}

// --- fixture.Driver (required) ---

func (*luaDriver) BackendCount() int                { return 1 }
func (*luaDriver) BackendKind() fixture.BackendKind { return fixture.HTTPLua }
func (*luaDriver) SubjectListenerName() string      { return "l_test_a" }
func (*luaDriver) ReferenceListenerPort() int       { return refLATestPort }

// ReferenceBootstrap returns the self-contained single-listener boot-reject
// bootstrap (Option B2 per fixture-0026 precedent) once the runner has
// flipped bootRejectMode. The non-reject path is never exercised (this is a
// pure boot-reject fixture) but renders the same bootstrap for shape
// consistency.
func (d *luaDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refLATestPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side. The runner-
// allocated subjAdminPort splices into the admin socket address so the
// StartSubjectProxy probe matches the bootstrap-bound admin listener.
func (d *luaDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject / ProbeAdmin are required by the Driver
// interface but never invoked in the boot-reject branch (the runner SKIPS
// Drive + admin-diff for BootRejectFixture drivers).

func (*luaDriver) DriveReference(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (*luaDriver) DriveSubject(_ context.Context, _ string) ([]byte, error)   { return nil, nil }
func (*luaDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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

// BootRejectScript flips bootRejectMode and returns the symmetry-artifact
// path. The runner discards the return value; the side effect is the signal.
func (d *luaDriver) BootRejectScript() string {
	d.mu.Lock()
	d.bootRejectMode = true
	d.mu.Unlock()
	return bootRejectScriptRelPath
}

// ExpectedBootErrorSubstring returns the literal substring the runner asserts
// is present (case-sensitive Contains) in BOTH ref + subj stderr.
func (*luaDriver) ExpectedBootErrorSubstring() string { return expectedBootErrorSubstr }

// renderBootRejectBootstrap returns the self-contained single-listener
// bootstrap BOTH proxies consume. The lua filter's source_codes{bad: ...}
// entry carries the COMPILE-ERROR script via inline_string → both sides
// fail-closed at config-load. The default_source_code is a VALID no-op so the
// boot-reject is attributable solely to the broken source_codes entry.
//
// A minimal-but-valid upstream cluster with a loopback dummy endpoint
// (127.0.0.1:1 — never dialed) is declared so envoy-go's cluster manager
// (which constructs BEFORE the listener manager) does not fail with a
// zero-endpoint error BEFORE the listener-manager lua compile-reject — same
// ordering sidestep fixture-0026 uses.
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
                  - name: envoy.filters.http.lua
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua
                      stat_prefix: scenario_g
                      default_source_code:
                        inline_string: %q
                      source_codes:
                        bad:
                          inline_string: %q
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
`, adminPort, listenerPort, validDefaultSource, bootRejectInlineSource)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*luaDriver)(nil)
	_ fixture.BackendKindAware = (*luaDriver)(nil)
)
