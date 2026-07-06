// Package inputs registers the 0026-http-lua-headers-bridge fixture with
// the differential runner per phase 22.1 SPEC §9 + Task 14 PLAN. Asserts
// per-scenario equivalence between envoy-go's envoy.filters.http.lua
// (22.1 headers-bridge surface) and reference Envoy v1.37.2 across the
// 7-scenario matrix (6 wire-interactive (a)-(f) full cross-side byte-
// exact via CompareBytes + scenario (g) substring-match via the NEW
// BootRejectFixture interface introduced at Task 13).
//
// Topology (six listeners, one per wire-interactive scenario; all share
// one upstream cluster c_backend → echobackend subprocess):
//
//	l_test_a → scripts/a_add_header.lua    (rh:headers():add)
//	l_test_b → scripts/b_replace_header.lua (rh:headers():replace)
//	l_test_c → scripts/c_remove_header.lua  (rh:headers():remove)
//	l_test_d → scripts/d_respond.lua        (rh:respond short-circuit)
//	l_test_e → scripts/e_log_only.lua       (rh:logInfo pass-through)
//	l_test_f → scripts/f_headers_iter.lua   (pairs() count → header)
//
// Per-scenario script paths are spliced into the bootstrap templates via
// {{.ScriptA}}..{{.ScriptF}} substitutions. The reference container
// receives container-side absolute paths under /scripts/ (bind-mounted
// from the host fixture's scripts/ subdirectory via ReferenceHostMounts).
// The subject (envoy-go) runs on the host directly + uses host-side
// absolute paths under {{.FixtureDir}}/scripts/.
//
// **Scenario (g) script-compile-error** is wire-orthogonal: the runner's
// BootRejectFixture branch (test/differential/harness.go) renders both
// bootstraps via this driver's ReferenceBootstrap + SubjectConfig. Under
// Task-15 Option B2 (PLAN §6 Task 15 RECOMMENDED), the boot-reject mode
// returns a self-contained single-listener bootstrap that embeds the
// broken Lua source via the DataSource InlineString arm (parent §6.2
// arm 12 → arm 16 compile-failure) rather than referencing the on-disk
// scripts/g_compile_error.lua file. This eliminates the host-mount
// dependency that the harness's tryStartReferenceProxy lacks (it does
// NOT consult ReferenceLogMounter, so a Filename-arm bootstrap would
// fail with "Invalid path" before the lua filter ever PARSE-REJECTed).
// The runner then asserts both proxies exited non-zero AND both
// captured stderr buffers contain the literal substring "script load
// error" per AMEND-10 option 2 + parent §13-R1 + the Task-15-pinned
// envoy-go wording wrap at cmd/envoy-go/main.go.
//
// **Scenario (e) `:logInfo` cross-side assertion** per D3 closure at
// PLAN session (locked at parent §11.7.7 RECOMMENDED option (a)): the
// per-probe byte stream emits a constant `scenario e ran=1` token (the
// literal log line is NOT cross-side asserted — gopher-lua's log
// formatting diverges from upstream spdlog per AMEND-9 row 3). The
// cross-side **stat-counter delta** assertion lives in AssertStats
// (StatsAsserter) which scrapes /stats/prometheus on both sides AFTER
// Drive completes + asserts both sides incremented
// `envoy_http_lua_scenario_e_executions{envoy_http_conn_manager_prefix="hcm_e"}`
// by 1. The cumulative-delta-in-AssertStats discipline mirrors
// fixture-0023's 9-counter PRESENCE-check pattern + the SPEC's
// "stat-counter delta IS the 'Lua ran' assertion" promise (it's just
// performed in the AssertStats hook rather than inline in Drive — the
// driver does not have access to either admin addr during Drive, so the
// in-Drive inline scrape path used by fixture-0025 reference-less driver
// is not available in the cross-side dual-Drive fixture-0026 path).
//
// **Fixture-0026 GREEN is DEFERRED to Task 15** per 22.1 PLAN Task 14
// acceptance criteria: Task 14 lands the directory + the 7 .lua source
// files + the driver impl + the README + the bootstrap templates +
// expectations.yaml; Task 15 lands the cmd/envoy-go/main.go boot-reject
// path "script load error: " wording wrap that lets scenario (g) pass
// the envoy-go-side substring assertion.
package inputs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0026-http-lua-headers-bridge"

	// Reference-container in-container listener ports. The runner exposes
	// each container port via testcontainers MappedPort; the driver dials
	// the host-mapped addr returned by ref.ListenerAddr(<containerPort>).
	// Admin port stays at the standard 9901.
	refAdminPort  = 9901
	refLATestPort = 10026 // l_test_a — scenario (a) add_header
	refLBTestPort = 10027 // l_test_b — scenario (b) replace_header
	refLCTestPort = 10028 // l_test_c — scenario (c) remove_header
	refLDTestPort = 10029 // l_test_d — scenario (d) respond
	refLETestPort = 10030 // l_test_e — scenario (e) log_only
	refLFTestPort = 10031 // l_test_f — scenario (f) headers_iter

	// Container-side absolute paths for the per-scenario .lua source
	// files. The runner bind-mounts host scripts/<scenario>.lua onto these
	// paths via ReferenceHostMounts() per fixture-0019 PKI-mount precedent.
	refContainerScriptA = "/scripts/a_add_header.lua"
	refContainerScriptB = "/scripts/b_replace_header.lua"
	refContainerScriptC = "/scripts/c_remove_header.lua"
	refContainerScriptD = "/scripts/d_respond.lua"
	refContainerScriptE = "/scripts/e_log_only.lua"
	refContainerScriptF = "/scripts/f_headers_iter.lua"
	refContainerScriptG = "/scripts/g_compile_error.lua"

	// Scenario (g) BootRejectFixture wiring per parent §13-R1 + AMEND-10
	// option 2 substring-match. The runner asserts both proxies exit
	// non-zero AND both stderr buffers contain expectedBootErrorSubstring
	// (case-sensitive Contains). BootRejectScript() returns the relative-
	// to-fixture-dir path of the on-disk broken-script source (kept for
	// driver-side documentation symmetry with the (a)-(f) script paths +
	// for the Task-14 README / inputs/ contract — the runner discards the
	// return value).
	//
	// Per Task-15 Option B2 (recommended at PLAN §6 Task 15): the
	// bootRejectMode bootstraps DO NOT reference the broken file on disk.
	// Instead, ReferenceBootstrap + SubjectConfig render a self-contained
	// single-listener bootstrap that embeds the broken Lua source via the
	// DataSource InlineString arm (parent §6.2 arm 12 — non-empty inline
	// string → straight to arm-16 compile-failure). This eliminates the
	// host-mount dependency for the boot-reject path: tryStartReferenceProxy
	// (harness.go) does NOT consult ReferenceLogMounter, so an on-disk
	// /scripts/g_compile_error.lua path would fail with "Invalid path"
	// before the lua filter ever runs. Embedding inline sidesteps the
	// harness gap without modifying tryStart* (Option B1's approach).
	bootRejectScriptRelPath = "scripts/g_compile_error.lua"
	expectedBootErrorSubstr = "script load error"

	// Inline source the boot-reject mode bootstraps embed via the
	// DataSource InlineString arm. Must match scripts/g_compile_error.lua
	// byte-equivalently (the on-disk file is the (a)-(f) symmetry artifact;
	// this constant is the actual wire-substitution payload). The trailing
	// tokens `this-is-not-valid-lua-syntax` after `end` are NOT valid Lua
	// 5.1 syntax — both LuaJIT (reference) and gopher-lua (subject) parser
	// PARSE-REJECT at config-load with the upstream-pinned wrap
	// "script load error: <detail>" (parent §11.7.5 + Task 15 envoy-go-
	// side wording wrap at cmd/envoy-go/main.go).
	bootRejectInlineSource = "function envoy_on_request(rh) end this-is-not-valid-lua-syntax"

	// Scenario (e) per D3 closure stat-counter delta assertion. The
	// internal name `http.hcm_e.lua.scenario_e.executions` flattens to
	// the Prometheus base `envoy_http_lua_scenario_e_executions` + label
	// `envoy_http_conn_manager_prefix="hcm_e"` per Rule SN2 (internal-dot
	// transform at internal/stats/name.go:72 substitutes `.` → `_` in the
	// SN2 rest segment). Reference Envoy v1.37.2 emits the same
	// flattening per Envoy's standard `<HCM>.lua.<config_prefix>.<stat>`
	// tag-extractor mapping; both sides expose the stat on
	// /stats/prometheus.
	scenarioEExecutionsPromName = "envoy_http_lua_scenario_e_executions"
	scenarioEHCMPrefix          = "hcm_e"
)

func init() {
	fixture.RegisterFixture(fixtureName, &luaDriver{})
}

// luaDriver carries per-driver state: the boot-reject mode flag (flipped
// when the runner's runBootRejectFixture branch calls BootRejectScript()
// before re-rendering the bootstrap templates).
type luaDriver struct {
	mu sync.Mutex

	// bootRejectMode flips true when the runner calls BootRejectScript()
	// (the runner does this once at the start of the boot-reject branch
	// before invoking ReferenceBootstrap + SubjectConfig). Subsequent
	// ReferenceBootstrap / SubjectConfig calls splice g_compile_error.lua
	// into all 6 listener script slots. The flag is one-way (no reset) —
	// the runner instantiates a fresh fixture run per t.Run sub-test so
	// cross-test contamination cannot occur.
	bootRejectMode bool
}

// --- fixture.Driver (required) ---

func (*luaDriver) BackendCount() int                { return 1 }
func (*luaDriver) BackendKind() fixture.BackendKind { return fixture.HTTPLua }

// SubjectListenerName returns l_test_a per Driver-interface contract.
// The runner uses MultiListenerDriver below for the multi-listener
// dispatch; this single-name hook is the single-addr fallback.
func (*luaDriver) SubjectListenerName() string { return "l_test_a" }

// ReferenceListenerPort returns l_test_a's port per Driver-interface
// contract. The runner uses MultiListenerDriver below for the
// multi-listener dispatch; this single-port hook is the single-port
// fallback.
func (*luaDriver) ReferenceListenerPort() int { return refLATestPort }

// ReferenceBootstrap renders envoy.yaml with the per-scenario script
// container paths spliced into {{.ScriptA}}..{{.ScriptF}}. In normal
// mode each slot carries its own scenario script; in boot-reject mode
// (Option B2) returns the self-contained single-listener inline-string
// bootstrap that embeds the broken Lua source via the DataSource
// InlineString arm — sidesteps the harness gap that
// tryStartReferenceProxy does NOT consult ReferenceLogMounter (the
// reference container would otherwise fail with "Invalid path:
// /scripts/g_compile_error.lua" before the lua filter ever PARSE-REJECTs).
// The InlineString arm routes straight to arm 16 (compile-failure) on
// both sides per parent §6.2 + 22.1 SPEC §6.1.
func (d *luaDriver) ReferenceBootstrap(backendPorts []int) string {
	if d.inBootRejectMode() {
		return renderBootRejectBootstrap(refAdminPort, refLATestPort)
	}
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   refAdminPort,
		"LATestPort":  refLATestPort,
		"LBTestPort":  refLBTestPort,
		"LCTestPort":  refLCTestPort,
		"LDTestPort":  refLDTestPort,
		"LETestPort":  refLETestPort,
		"LFTestPort":  refLFTestPort,
		"BackendHost": "host.docker.internal",
		"BackendPort": backendPorts[0],
		"ScriptA":     refContainerScriptA,
		"ScriptB":     refContainerScriptB,
		"ScriptC":     refContainerScriptC,
		"ScriptD":     refContainerScriptD,
		"ScriptE":     refContainerScriptE,
		"ScriptF":     refContainerScriptF,
	})
}

// SubjectConfig renders envoy-go.yaml with host-side script paths.
// Mirrors ReferenceBootstrap's boot-reject-mode logic for the broken
// script splice in scenario (g): under boot-reject mode (Option B2)
// returns a self-contained single-listener bootstrap with the broken
// Lua source embedded via the DataSource InlineString arm. The runner-
// allocated subjAdminPort is spliced into the admin socket address so
// the StartSubjectProxy "127.0.0.1:<subjAdminPort>" probe matches the
// bootstrap-bound admin listener. AssertStats receives the same addr
// via runner-side dispatch.
func (d *luaDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if d.inBootRejectMode() {
		return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
	}
	tpl := mustReadFixtureFile("envoy-go.yaml")
	fxDir := fixtureDir()
	subjScriptA := filepath.Join(fxDir, "scripts", "a_add_header.lua")
	subjScriptB := filepath.Join(fxDir, "scripts", "b_replace_header.lua")
	subjScriptC := filepath.Join(fxDir, "scripts", "c_remove_header.lua")
	subjScriptD := filepath.Join(fxDir, "scripts", "d_respond.lua")
	subjScriptE := filepath.Join(fxDir, "scripts", "e_log_only.lua")
	subjScriptF := filepath.Join(fxDir, "scripts", "f_headers_iter.lua")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"LATestPort":  subjListenerPort,
		"LBTestPort":  subjListenerPort + 1,
		"LCTestPort":  subjListenerPort + 2,
		"LDTestPort":  subjListenerPort + 3,
		"LETestPort":  subjListenerPort + 4,
		"LFTestPort":  subjListenerPort + 5,
		"BackendPort": backendPorts[0],
		"ScriptA":     subjScriptA,
		"ScriptB":     subjScriptB,
		"ScriptC":     subjScriptC,
		"ScriptD":     subjScriptD,
		"ScriptE":     subjScriptE,
		"ScriptF":     subjScriptF,
	})
}

// inBootRejectMode returns true once BootRejectScript() has been called
// by the runner's runBootRejectFixture branch. Read with the per-driver
// mutex so reads in ReferenceBootstrap / SubjectConfig race-safely
// against the BootRejectScript writer (Go's race detector flags
// unsynchronized bool reads even though atomic visibility is fine).
func (d *luaDriver) inBootRejectMode() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.bootRejectMode
}

// renderBootRejectBootstrap returns the self-contained single-listener
// boot-reject bootstrap that BOTH the reference Envoy container and the
// envoy-go subject subprocess consume in scenario (g). Option B2 per
// PLAN Task 15: embeds the broken Lua source via DataSource InlineString
// (parent §6.2 arm 12 → arm 16 compile-failure) rather than referencing
// the on-disk scripts/g_compile_error.lua file. This eliminates the
// host-mount dependency that tryStartReferenceProxy lacks (the harness
// gap surfaced by Task 14 — tryStartReferenceProxy does NOT consult
// ReferenceLogMounter, so a Filename-arm bootstrap would fail with
// "Invalid path" before the lua filter ever PARSE-REJECTed).
//
// The bootstrap declares a minimal-but-valid upstream cluster with a
// loopback dummy endpoint (127.0.0.1:1 — never dialed): envoy-go's
// cluster manager constructs at boot BEFORE the listener manager
// (cmd/envoy-go/main.go:91 vs :189), so a zero-endpoint cluster would
// fail at cluster-manager time with `cluster: "<name>": zero endpoints
// across all locality groups` — surfacing BEFORE the listener-manager
// arm-16 lua compile-reject. The dummy endpoint sidesteps that ordering
// without affecting the boot-reject signal (the listener bind never
// happens; the endpoint is never reachable).
//
// Identical bootstrap for both sides: the inline-string source is byte-
// equivalent + neither side reaches the listener-bind step, so admin
// port + listener port differences are immaterial (the boot-reject
// happens during config parse). The caller passes the per-side admin /
// listener ports so the bootstrap shape stays consistent with the
// non-reject path's bootstrap shape for diagnostic readability.
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
`, adminPort, listenerPort, bootRejectInlineSource)
}

// DriveReference drives all 6 wire-interactive scenarios (a)..(f)
// against the reference proxy's listener addr map. The runner provides
// the multi-listener addr map via DriveReferenceMulti below; this
// single-addr hook is the Driver-interface fallback (derives the +1..+5
// peer addrs via deriveAddrsFromRef when the runner falls through to
// single-addr DriveReference).
func (d *luaDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveAddrsFromRef(addr)
	return d.driveProxy(ctx, addrs, "ref")
}

// DriveSubject mirrors DriveReference for the subject side.
func (d *luaDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveAddrsFromSubj(addr)
	return d.driveProxy(ctx, addrs, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes per the standard admin-diff at runner
// step 9.
func (*luaDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.MultiListenerDriver ---

func (*luaDriver) SubjectListenerNames() []string {
	return []string{
		"l_test_a", "l_test_b", "l_test_c",
		"l_test_d", "l_test_e", "l_test_f",
	}
}

func (*luaDriver) ReferenceListenerPorts() []int {
	return []int{
		refLATestPort, refLBTestPort, refLCTestPort,
		refLDTestPort, refLETestPort, refLFTestPort,
	}
}

func (d *luaDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "ref")
}

func (d *luaDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "subj")
}

// --- fixture.ReferenceLogMounter ---
//
// Bind-mounts each per-scenario .lua source file from the host fixture's
// scripts/ subdirectory into the reference container at /scripts/. The
// envoy.yaml template's {{.ScriptX}} substitutions reference these
// container-side absolute paths. Mirrors fixture-0019 PKI-mount precedent
// per parent §8.4 + AMEND-11.
func (*luaDriver) ReferenceHostMounts() []fixture.HostMount {
	fxDir := fixtureDir()
	return []fixture.HostMount{
		{HostPath: filepath.Join(fxDir, "scripts", "a_add_header.lua"), ContainerPath: refContainerScriptA},
		{HostPath: filepath.Join(fxDir, "scripts", "b_replace_header.lua"), ContainerPath: refContainerScriptB},
		{HostPath: filepath.Join(fxDir, "scripts", "c_remove_header.lua"), ContainerPath: refContainerScriptC},
		{HostPath: filepath.Join(fxDir, "scripts", "d_respond.lua"), ContainerPath: refContainerScriptD},
		{HostPath: filepath.Join(fxDir, "scripts", "e_log_only.lua"), ContainerPath: refContainerScriptE},
		{HostPath: filepath.Join(fxDir, "scripts", "f_headers_iter.lua"), ContainerPath: refContainerScriptF},
		{HostPath: filepath.Join(fxDir, "scripts", "g_compile_error.lua"), ContainerPath: refContainerScriptG},
	}
}

// --- differential.BootRejectFixture (Task 13) ---
//
// Per parent §13-R1 + AMEND-10 option 2 substring-match. The runner's
// runBootRejectFixture branch (test/differential/runner_test.go) calls
// BootRejectScript() once at the start of the branch. We use this call
// as the signal to flip bootRejectMode = true so subsequent
// ReferenceBootstrap / SubjectConfig calls splice the broken script path
// into all 6 listener slots (any one triggers the boot-reject).

// BootRejectScript returns the relative-to-fixture-dir path of the
// broken script. As a side effect, sets bootRejectMode = true so
// subsequent ReferenceBootstrap + SubjectConfig calls render the
// broken-script bootstraps. The runner discards the return value (per
// runner_test.go:1397 `_ = brf.BootRejectScript()`); the side effect is
// the meaningful signal.
func (d *luaDriver) BootRejectScript() string {
	d.mu.Lock()
	d.bootRejectMode = true
	d.mu.Unlock()
	return bootRejectScriptRelPath
}

// ExpectedBootErrorSubstring returns the literal substring the runner
// asserts is present (case-sensitive) in BOTH ref + subj stderr after
// the boot-reject. Per AMEND-10 option 2 + parent §11.7.5 + parent §13-W
// the substring is "script load error" (the upstream wording from
// source/extensions/filters/common/lua/lua.cc + the envoy-go-side
// wrapping at Task 15 cmd/envoy-go/main.go).
func (*luaDriver) ExpectedBootErrorSubstring() string {
	return expectedBootErrorSubstr
}

// --- fixture.StatsAsserter (scenario (e) cross-side delta) ---
//
// Per D3 closure at PLAN session + parent §11.7.7 RECOMMENDED option (a):
// the stat-counter delta IS the "Lua ran" assertion for scenario (e).
// Driver-side discipline: emit a constant `scenario e ran=1` token into
// the Drive byte stream (so CompareBytes still fires for scenario (e));
// cross-side delta-check lives here in AssertStats which has access to
// both admin addrs.
//
// Per-scenario probe count: the driver fires ONE GET / per scenario per
// proxy run, so the executions counter for each per-scenario stat_prefix
// (scenario_a / scenario_b / ... / scenario_f) should be exactly 1 on
// both ref + subj sides post-Drive. We focus on scenario_e because
// that's the assertion the D3 closure targets; the other counters are
// asserted via the cross-side byte stream comparison.
func (d *luaDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeLuaStats(refAdminAddr)
	if err != nil {
		t.Errorf("scenario (e) AssertStats: scrape ref /stats/prometheus: %v", err)
		return
	}
	subjStats, err := scrapeLuaStats(subjAdminAddr)
	if err != nil {
		t.Errorf("scenario (e) AssertStats: scrape subj /stats/prometheus: %v", err)
		return
	}

	if os.Getenv("FIXTURE_0026_DUMP_STATS") != "" {
		fmt.Fprintf(os.Stderr, "=== ref lua stats ===\n")
		for k, v := range refStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
		fmt.Fprintf(os.Stderr, "=== subj lua stats ===\n")
		for k, v := range subjStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
	}

	// Scenario (e) D3 cross-side: the lua filter under l_test_e (HCM
	// stat_prefix hcm_e + Lua stat_prefix scenario_e) emitted
	// executions counter is asserted on BOTH sides — both should equal 1
	// after a single per-Drive scenario_e probe per the
	// "stat-counter delta IS the 'Lua ran' assertion" promise.
	refE, refPresent := lookupCounter(refStats, scenarioEExecutionsPromName, scenarioEHCMPrefix)
	subjE, subjPresent := lookupCounter(subjStats, scenarioEExecutionsPromName, scenarioEHCMPrefix)
	if !refPresent {
		t.Errorf("scenario (e): ref /stats/prometheus does not expose %s{envoy_http_conn_manager_prefix=%q} — counter MUST be present per parent §7.2 + AMEND-2 unconditional-allocation",
			scenarioEExecutionsPromName, scenarioEHCMPrefix)
	}
	if !subjPresent {
		t.Errorf("scenario (e): subj /stats/prometheus does not expose %s{envoy_http_conn_manager_prefix=%q} — envoy-go regression (counter MUST be present per parent §7.2 + AMEND-2 unconditional-allocation)",
			scenarioEExecutionsPromName, scenarioEHCMPrefix)
	}
	if refPresent && subjPresent && refE != subjE {
		t.Errorf("scenario (e) cross-side delta mismatch: ref=%d subj=%d (both sides MUST agree on the per-scenario executions count per D3 closure + parent §11.7.7 option (a))",
			refE, subjE)
	}
	// Per-probe absolute value: ONE probe fired per scenario per Drive,
	// so the executions counter MUST equal 1 on both sides.
	if refPresent && refE != 1 {
		t.Errorf("scenario (e) ref executions counter = %d; want 1 (one probe per Drive)", refE)
	}
	if subjPresent && subjE != 1 {
		t.Errorf("scenario (e) subj executions counter = %d; want 1 (one probe per Drive)", subjE)
	}
}

// --- driveProxy / per-scenario emit ---

// scenarioResult mirrors fixture-0023's shape.
type scenarioResult struct {
	statusCode int
	body       []byte
	headers    http.Header
	err        error
}

// driveProxy runs the 6 wire-interactive scenarios (a)..(f) sequentially
// + emits a per-scenario verdict line into the byte buffer. The byte
// stream is identical per side (no side label is emitted) so CompareBytes
// fires on equivalence.
func (d *luaDriver) driveProxy(ctx context.Context, addrs map[string]string, side string) ([]byte, error) {
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}

	var buf bytes.Buffer

	// Scenario (a) — add_header.
	resA := runScenarioA(ctx, client, addrs["l_test_a"], side)
	emitScenario(&buf, "a", resA)

	// Scenario (b) — replace_header.
	resB := runScenarioB(ctx, client, addrs["l_test_b"], side)
	emitScenario(&buf, "b", resB)

	// Scenario (c) — remove_header.
	resC := runScenarioC(ctx, client, addrs["l_test_c"], side)
	emitScenario(&buf, "c", resC)

	// Scenario (d) — respond short-circuit (403/denied).
	resD := runScenarioD(ctx, client, addrs["l_test_d"], side)
	emitScenario(&buf, "d", resD)

	// Scenario (e) — log_only pass-through. The cross-side stat-delta
	// lives in AssertStats; the byte stream emits a constant ran=1 token.
	resE := runScenarioE(ctx, client, addrs["l_test_e"], side)
	emitScenario(&buf, "e", resE)

	// Scenario (f) — headers_iter count.
	resF := runScenarioF(ctx, client, addrs["l_test_f"], side)
	emitScenario(&buf, "f", resF)

	return buf.Bytes(), nil
}

// emitScenario formats the per-scenario verdict line into the byte
// stream. Mirrors fixture-0023's per-scenario emit shape. Per-scenario
// body classification insulates from non-substantive byte divergences
// (e.g., upstream Envoy's `x-envoy-internal: true` header in the
// reflected JSON vs envoy-go's lack of forwarding).
func emitScenario(buf *bytes.Buffer, id string, r scenarioResult) {
	if r.err != nil {
		fmt.Fprintf(buf, "scenario %s status=ERR body=ERR (%v)\n", id, r.err)
		return
	}
	verdict := classifyBody(id, r.body, r.headers)
	fmt.Fprintf(buf, "scenario %s status=%d body=%s\n", id, r.statusCode, verdict)
}

// classifyBody returns the per-scenario body verdict per 22.1 SPEC §9.1.
//
//   - (a): echobackend reflects request headers as JSON; ASSERT
//     `x-lua-injected: hello` is present in the reflected headers map.
//   - (b): assert `user-agent: envoy-go-lua/1.0` is the reflected value.
//   - (c): assert `x-blocked` is ABSENT from the reflected headers map
//     (the script removed it before forwarding upstream).
//   - (d): no upstream round-trip; assert body byte-exact "denied".
//   - (e): assert echobackend reflected JSON (request unchanged); the
//     stat-counter delta cross-side check lives in AssertStats.
//   - (f): assert `x-headers-count: <N>` is present in the reflected
//     headers map; the value is order-independent + deterministic per
//     §11 D7 closure (alphabetical-snapshot in bridge __pairs).
func classifyBody(id string, body []byte, headers http.Header) string {
	switch id {
	case "a":
		hdrs := reflectedHeaders(body)
		if hdrs == nil {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", trim(body))
		}
		if v, ok := hdrs["x-lua-injected"]; ok && v == "hello" {
			return "ok"
		}
		return fmt.Sprintf("mismatch(x-lua-injected,reflected=%v)", hdrs["x-lua-injected"])
	case "b":
		hdrs := reflectedHeaders(body)
		if hdrs == nil {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", trim(body))
		}
		if v, ok := hdrs["user-agent"]; ok && v == "envoy-go-lua/1.0" {
			return "ok"
		}
		return fmt.Sprintf("mismatch(user-agent,reflected=%v)", hdrs["user-agent"])
	case "c":
		hdrs := reflectedHeaders(body)
		if hdrs == nil {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", trim(body))
		}
		if _, present := hdrs["x-blocked"]; present {
			return fmt.Sprintf("mismatch(x-blocked_still_present,reflected=%v)", hdrs["x-blocked"])
		}
		return "ok"
	case "d":
		if string(body) == "denied" {
			ct := strings.ToLower(headers.Get("content-type"))
			cl := headers.Get("content-length")
			// AMEND-7 wire-pin: content-length: 6 auto-set + content-
			// type: text/plain default. Tolerate text/plain with charset
			// suffix variants (upstream emits bare `text/plain`).
			if cl != "6" {
				return fmt.Sprintf("mismatch(content-length=%q,want=6)", cl)
			}
			if !strings.HasPrefix(ct, "text/plain") {
				return fmt.Sprintf("mismatch(content-type=%q,want_prefix=text/plain)", ct)
			}
			return "ok"
		}
		return fmt.Sprintf("mismatch(body,got=%q,want=denied)", string(body))
	case "e":
		hdrs := reflectedHeaders(body)
		if hdrs == nil {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", trim(body))
		}
		// Per D3 closure: byte stream emits ran=1 constant; cross-side
		// stat-counter delta lives in AssertStats.
		return "ran=1"
	case "f":
		hdrs := reflectedHeaders(body)
		if hdrs == nil {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", trim(body))
		}
		if _, present := hdrs["x-headers-count"]; !present {
			return fmt.Sprintf("mismatch(x-headers-count_absent,reflected_keys=%v)", reflectedKeys(hdrs))
		}
		// The actual count is order-independent + deterministic per
		// §11.2 D7 closure (alphabetical-snapshot in bridge __pairs).
		// Both sides agree on N regardless of map-iteration order on
		// either side; the cross-side byte-exact assertion is on
		// PRESENCE + the (deterministic) numeric value.
		return "x-headers-count=" + hdrs["x-headers-count"]
	}
	return "skip"
}

// reflectedHeaders parses the echobackend JSON body + returns the
// reflected `headers` map (lowercased canonical keys per ADR-0072).
// Returns nil if the body is not a parseable echo envelope.
func reflectedHeaders(body []byte) map[string]string {
	if len(body) == 0 {
		return nil
	}
	var rec struct {
		Method  string            `json:"method"`
		Path    string            `json:"path"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil
	}
	if rec.Method == "" || rec.Path == "" {
		return nil
	}
	out := map[string]string{}
	for k, v := range rec.Headers {
		out[strings.ToLower(k)] = v
	}
	return out
}

func reflectedKeys(hdrs map[string]string) []string {
	keys := make([]string, 0, len(hdrs))
	for k := range hdrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func trim(body []byte) string {
	const max = 80
	s := string(body)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// --- per-scenario request functions ---

func runScenarioA(ctx context.Context, client *http.Client, addr, _ string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/scenario_a", nil)
	if err != nil {
		return scenarioResult{err: err}
	}
	return doRequest(client, req)
}

func runScenarioB(ctx context.Context, client *http.Client, addr, _ string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/scenario_b", nil)
	if err != nil {
		return scenarioResult{err: err}
	}
	// Supply a baseline user-agent so the replace() has something to
	// replace + the cross-side comparison is unambiguous.
	req.Header.Set("user-agent", "integration-test/0.1")
	return doRequest(client, req)
}

func runScenarioC(ctx context.Context, client *http.Client, addr, _ string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/scenario_c", nil)
	if err != nil {
		return scenarioResult{err: err}
	}
	// Supply x-blocked so the remove() has something to remove.
	req.Header.Set("x-blocked", "yes")
	return doRequest(client, req)
}

func runScenarioD(ctx context.Context, client *http.Client, addr, _ string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/scenario_d", nil)
	if err != nil {
		return scenarioResult{err: err}
	}
	return doRequest(client, req)
}

func runScenarioE(ctx context.Context, client *http.Client, addr, _ string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/scenario_e", nil)
	if err != nil {
		return scenarioResult{err: err}
	}
	return doRequest(client, req)
}

func runScenarioF(ctx context.Context, client *http.Client, addr, _ string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/scenario_f", nil)
	if err != nil {
		return scenarioResult{err: err}
	}
	return doRequest(client, req)
}

func doRequest(client *http.Client, req *http.Request) scenarioResult {
	resp, err := client.Do(req)
	if err != nil {
		return scenarioResult{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return scenarioResult{err: err}
	}
	return scenarioResult{
		statusCode: resp.StatusCode,
		body:       body,
		headers:    resp.Header,
	}
}

// --- stats scrape (cross-side scenario (e) delta) ---

// scrapeLuaStats fetches /stats/prometheus + returns lua-related
// counters keyed by name|labelstr per the fixture-0023 scrape pattern
// (mirrors test/fixtures/0023-http-ext-proc-body/inputs/driver.go's
// scrapeExtProcStats + parseExtProcPromBody).
func scrapeLuaStats(adminAddr string) (map[string]int64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseLuaPromBody(body), nil
}

// parseLuaPromBody parses the prometheus-format response body + extracts
// the lua-related counter values. Keys are `<base_name>{<labels>}` (or
// bare `<base_name>` when no labels are present).
func parseLuaPromBody(data []byte) map[string]int64 {
	out := map[string]int64{}
	const wantInfix = "_lua_"
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, valueStr, labelStr string
		if idx := strings.IndexByte(line, '{'); idx >= 0 {
			name = line[:idx]
			closeIdx := strings.LastIndexByte(line, '}')
			if closeIdx < 0 || closeIdx+1 >= len(line) {
				continue
			}
			labelStr = line[idx+1 : closeIdx]
			valueStr = strings.TrimSpace(line[closeIdx+1:])
		} else {
			sp := strings.LastIndexByte(line, ' ')
			if sp < 0 {
				continue
			}
			name = line[:sp]
			valueStr = strings.TrimSpace(line[sp+1:])
		}
		if !strings.Contains(name, wantInfix) {
			continue
		}
		if sp := strings.IndexByte(valueStr, ' '); sp >= 0 {
			valueStr = valueStr[:sp]
		}
		f, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}
		key := name
		if labelStr != "" {
			key = name + "{" + labelStr + "}"
		}
		out[key] = int64(f)
	}
	return out
}

// lookupCounter searches the stats map for a metric matching wantName +
// having the given HCM conn-manager-prefix label value. Returns
// (value, true) on first match.
func lookupCounter(stats map[string]int64, wantName, hcmPrefix string) (int64, bool) {
	wantLabel := `envoy_http_conn_manager_prefix="` + hcmPrefix + `"`
	for k, v := range stats {
		bareName := k
		var labels string
		if i := strings.IndexByte(k, '{'); i >= 0 {
			bareName = k[:i]
			labels = k[i:]
		}
		if bareName != wantName {
			continue
		}
		if !strings.Contains(labels, wantLabel) {
			continue
		}
		return v, true
	}
	return 0, false
}

// --- address-derivation helpers ---

// deriveAddrsFromRef derives the per-listener addr map for the reference
// container from the single-listener fallback addr. The reference
// container exposes each container port via a SEPARATE host-mapped port
// (testcontainers `MappedPort` returns distinct host ports per container
// port), so the host-port arithmetic this fixture uses (port + i) does
// NOT hold for the ref side. This helper is the Driver-interface single-
// addr fallback; the runner invokes DriveReferenceMulti directly per
// MultiListenerDriver dispatch — so this fallback is actually unused at
// runtime. Defensive impl: substitute container port within the addr
// string (works only if testcontainers happens to map the consecutive
// container ports to consecutive host ports — UNRELIABLE).
func deriveAddrsFromRef(s1Addr string) map[string]string {
	replace := func(addr string, fromPort, toPort int) string {
		return strings.Replace(addr,
			fmt.Sprintf(":%d", fromPort),
			fmt.Sprintf(":%d", toPort), 1)
	}
	return map[string]string{
		"l_test_a": s1Addr,
		"l_test_b": replace(s1Addr, refLATestPort, refLBTestPort),
		"l_test_c": replace(s1Addr, refLATestPort, refLCTestPort),
		"l_test_d": replace(s1Addr, refLATestPort, refLDTestPort),
		"l_test_e": replace(s1Addr, refLATestPort, refLETestPort),
		"l_test_f": replace(s1Addr, refLATestPort, refLFTestPort),
	}
}

// deriveAddrsFromSubj derives the per-listener addr map for the subject
// from the single-listener fallback addr. The subject's MultiListener
// listeners bind to consecutive host ports starting from subjListenerPort
// per the SubjectConfig template — so port arithmetic works on this side.
func deriveAddrsFromSubj(s1Addr string) map[string]string {
	lastColon := strings.LastIndex(s1Addr, ":")
	if lastColon < 0 {
		return map[string]string{
			"l_test_a": s1Addr, "l_test_b": s1Addr, "l_test_c": s1Addr,
			"l_test_d": s1Addr, "l_test_e": s1Addr, "l_test_f": s1Addr,
		}
	}
	hostPart := s1Addr[:lastColon]
	portStr := s1Addr[lastColon+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return map[string]string{
			"l_test_a": s1Addr, "l_test_b": s1Addr, "l_test_c": s1Addr,
			"l_test_d": s1Addr, "l_test_e": s1Addr, "l_test_f": s1Addr,
		}
	}
	return map[string]string{
		"l_test_a": s1Addr,
		"l_test_b": fmt.Sprintf("%s:%d", hostPart, port+1),
		"l_test_c": fmt.Sprintf("%s:%d", hostPart, port+2),
		"l_test_d": fmt.Sprintf("%s:%d", hostPart, port+3),
		"l_test_e": fmt.Sprintf("%s:%d", hostPart, port+4),
		"l_test_f": fmt.Sprintf("%s:%d", hostPart, port+5),
	}
}

// --- file / template helpers (mirrors fixture-0023 + fixture-0025) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(thisFile))
}

func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

func mustRender(tpl string, data map[string]any) string {
	t, err := template.New("bootstrap").Parse(tpl)
	if err != nil {
		panic(fmt.Sprintf("driver: template parse: %v", err))
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("driver: template execute: %v", err))
	}
	return buf.String()
}

// Compile-time interface assertions.
var (
	_ fixture.Driver              = (*luaDriver)(nil)
	_ fixture.BackendKindAware    = (*luaDriver)(nil)
	_ fixture.MultiListenerDriver = (*luaDriver)(nil)
	_ fixture.ReferenceLogMounter = (*luaDriver)(nil)
	_ fixture.StatsAsserter       = (*luaDriver)(nil)
)
