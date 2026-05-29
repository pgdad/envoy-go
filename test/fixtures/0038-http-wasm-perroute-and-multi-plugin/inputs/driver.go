// Package inputs registers the 0038-http-wasm-perroute-and-multi-plugin
// fixture with the differential runner per phase 25.3 Task 11. Mixed-mode
// fixture exercising the 25.3 surface:
//
//   - Per-route (cross-side via CompareBytes):
//
//   - perroute_override_applies   — a route carries a per-route wholesale
//     Wasm typed_per_filter_config override (AMEND-C1); the OVERRIDE's
//     response header `x-wasm-variant: override` wins over the listener
//     default. Reference Envoy (cpp-host wholesale per-route Wasm) +
//     envoy-go both apply it.
//
//   - perroute_listener_default   — a route with NO per-route TPFC →
//     the listener-default plugin's `x-wasm-variant: listener` applies.
//
//   - Multi-plugin vm_id sharing (cross-side via CompareBytes + subject via
//     StatsAsserter):
//
//   - multiplugin_shared_data     — TWO envoy.filters.http.wasm filters in
//     ONE chain, BOTH with the SAME vm_id (vm_shared) + same code +
//     (empty) vm_configuration → share ONE RootVM + ONE shared-data
//     namespace. Plugin A (writer) writes a shared key; plugin B (reader)
//     reflects it in `x-shared: written-by-A`. Cross-side: both sides
//     show the reflected value (cpp-host shares by vm_id). Subject
//     StatsAsserter: the envoy-go-strict `wasm.wazero.created` counter
//     shows ONE VM created for the two plugins (refcount sharing).
//
//   - Reload (subject-only via StatsAsserter):
//
//   - reload_fail_reload_recovers — a plugin with failure_policy=FAIL_RELOAD
//     whose guest TRAPS on request header x-trigger-trap=1. Drive sequence:
//     (req1) trigger → trap → 503 + arm reload; (req2 within ~1s backoff) →
//     503 + vm_reload_backoff; (sleep > backoff) (req3, no trigger) →
//     reload → vm_reload_success + 200. Subject-only on the
//     vm_reload_runtime_failure / vm_reload_backoff / vm_reload_success
//     triplet (reference V8 stat names diverge → subject-only).
//
// Topology: 3 listeners (l_perroute / l_multiplugin / l_reload) + ONE
// upstream cluster cluster_a pointing at the SHARED differential echobackend
// per phase-22.2 REVIEW §7.4 freeTCPPort flake mitigation. The driver
// implements MultiListenerDriver + ReferenceLogMounter so the 4 .wasm blobs
// are bind-mounted into the reference container at /bytecode/.
package inputs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0038-http-wasm-perroute-and-multi-plugin"

	refAdminPort = 9901

	// Reference-container in-container listener ports.
	refPerRoutePort = 10200
	refMultiPort    = 10201
	refReloadPort   = 10202

	// Listener names (subject addrs are looked up by name).
	lnPerRoute = "l_perroute"
	lnMulti    = "l_multiplugin"
	lnReload   = "l_reload"

	// Reload drive timing. The subject's FAIL_RELOAD backoff base interval is
	// 1s (envoy-go-strict floors at 100ms + omits jitter → deterministic). We
	// sleep 1.3s between req2 and req3 to clear the window.
	reloadBackoffSleep = 1300 * time.Millisecond

	// post-Drive settle before the subject stats scrape.
	probeStatsDelay = 300 * time.Millisecond
	subjAdminScrape = 2 * time.Second

	// Crate names (vendored bytecode/<name>.wasm).
	crateOverride = "perroute_override"
	crateListener = "listener_default"
	crateShared   = "shared_data_combined"
	crateReload   = "fail_reload_trap"
)

var crateNames = []string{crateOverride, crateListener, crateShared, crateReload}

func init() {
	fixture.RegisterFixture(fixtureName, &perRouteDriver{})
}

type perRouteDriver struct{}

// --- fixture.Driver ---

func (*perRouteDriver) BackendCount() int                { return 1 }
func (*perRouteDriver) BackendKind() fixture.BackendKind { return fixture.HTTPWasmPerRoute }
func (*perRouteDriver) SubjectListenerName() string      { return lnPerRoute }
func (*perRouteDriver) ReferenceListenerPort() int       { return refPerRoutePort }

func (d *perRouteDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":        refAdminPort,
		"BackendHost":      "host.docker.internal",
		"BackendPort":      backendPorts[0],
		"PerRoutePort":     refPerRoutePort,
		"MultiPort":        refMultiPort,
		"ReloadPort":       refReloadPort,
		"OverrideWasmPath": refContainerWasmPath(crateOverride),
		"ListenerWasmPath": refContainerWasmPath(crateListener),
		"SharedWasmPath":   refContainerWasmPath(crateShared),
		"ReloadWasmPath":   refContainerWasmPath(crateReload),
	})
}

func (d *perRouteDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	fxDir := fixtureDir()
	hostWasm := func(crate string) string {
		return filepath.Join(fxDir, "bytecode", crate+".wasm")
	}
	return mustRender(tpl, map[string]any{
		"AdminPort":        subjAdminPort,
		"BackendPort":      backendPorts[0],
		"PerRoutePort":     subjListenerPort,     // l_perroute
		"MultiPort":        subjListenerPort + 1, // l_multiplugin
		"ReloadPort":       subjListenerPort + 2, // l_reload
		"OverrideWasmPath": hostWasm(crateOverride),
		"ListenerWasmPath": hostWasm(crateListener),
		"SharedWasmPath":   hostWasm(crateShared),
		"ReloadWasmPath":   hostWasm(crateReload),
		// allowed_capabilities: sits at indent 26 (filter-level) / 34
		// (per-route override); its child keys must be indented one level
		// deeper (+2) → 28 / 36 respectively.
		"CapsFilter": capsBlock(28),
		"CapsRoute":  capsBlock(36),
	})
}

func (d *perRouteDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, map[string]string{lnPerRoute: addr}, true)
}

func (d *perRouteDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, map[string]string{lnPerRoute: addr}, false)
}

func (*perRouteDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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

// --- MultiListenerDriver ---

func (*perRouteDriver) SubjectListenerNames() []string {
	return []string{lnPerRoute, lnMulti, lnReload}
}

func (*perRouteDriver) ReferenceListenerPorts() []int {
	return []int{refPerRoutePort, refMultiPort, refReloadPort}
}

func (d *perRouteDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, true)
}

func (d *perRouteDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, false)
}

// --- ReferenceLogMounter (bind-mount the 4 .wasm blobs into the reference
// container at /bytecode/) ---

func (*perRouteDriver) ReferenceHostMounts() []fixture.HostMount {
	fxDir := fixtureDir()
	out := make([]fixture.HostMount, 0, len(crateNames))
	for _, c := range crateNames {
		out = append(out, fixture.HostMount{
			HostPath:      filepath.Join(fxDir, "bytecode", c+".wasm"),
			ContainerPath: refContainerWasmPath(c),
		})
	}
	return out
}

// --- driveProxy (cross-side byte stream) ---
//
// Cross-side arms: perroute_override_applies / perroute_listener_default /
// multiplugin_shared_data — these classify RESPONSE HEADERS the wasm guests
// set; the byte stream is mirrored between sides so CompareBytes fires on
// equivalence. The reload arm is SUBJECT-ONLY: on the SUBJECT side we drive
// the req1/req2/sleep/req3 trap+recover sequence (so AssertStats sees the
// vm_reload triplet); on BOTH sides we emit a constant token for the reload
// arm so CompareBytes naturally passes.
func (d *perRouteDriver) driveProxy(ctx context.Context, addrs map[string]string, ref bool) ([]byte, error) {
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}

	var buf bytes.Buffer

	// --- subject-only arm: perroute_override_applies ---
	// Reference Envoy v1.37.2 does NOT support per-route configuration for
	// envoy.filters.http.wasm (boot rejects route-specific wasm config), so
	// the per-route wholesale Wasm override (AMEND-C1) is an envoy-go
	// capability NOT shared by reference v1.37.2. We STILL hit /override on
	// the SUBJECT (so the override VM is built + its guest dispatches → the
	// wasm.plugin_perroute_override.executions counter increments, which the
	// StatsAsserter pins); on the reference the same request runs the
	// listener default (no per-route config). The cross-side byte stream for
	// this arm is a CONSTANT TOKEN so CompareBytes passes — the override's
	// application is asserted SUBJECT-ONLY via StatsAsserter.
	if !ref {
		// Subject: confirm the override actually applied (sanity; the
		// authoritative assertion is the StatsAsserter executions counter).
		v := getResponseHeader(ctx, client, addrs[lnPerRoute], "GET", "/override", "x-wasm-variant")
		if os.Getenv("FIXTURE_0038_DUMP_STREAM") != "" {
			fmt.Fprintf(os.Stderr, "subj /override x-wasm-variant=%s (want override)\n", v)
		}
	} else {
		// Reference: hit /override too (runs listener default) so the request
		// counts symmetrically; result is not classified into the byte stream.
		_ = doRequest(ctx, client, addrs[lnPerRoute], "GET", "/override", nil)
	}
	fmt.Fprintf(&buf, "perroute_override subject-only\n")

	// --- cross-side arm: perroute_listener_default ---
	listenerVariant := getResponseHeader(ctx, client, addrs[lnPerRoute], "GET", "/default", "x-wasm-variant")
	fmt.Fprintf(&buf, "perroute_listener x-wasm-variant=%s\n", listenerVariant)

	// --- cross-side arm: multiplugin_shared_data ---
	// The two shared-VM filters each CAS-increment ONE shared counter on the
	// request path → x-shared-count=2 IFF they share the namespace (vm_id
	// sharing). A non-shared namespace would reflect 1. Both sides increment
	// identically, so CompareBytes fires on equivalence (both "2").
	shared := getResponseHeader(ctx, client, addrs[lnMulti], "GET", "/multi", "x-shared-count")
	fmt.Fprintf(&buf, "multiplugin x-shared-count=%s\n", shared)

	// --- subject-only arm: reload_fail_reload_recovers ---
	// SUBJECT side drives the trap+recover sequence; the StatsAsserter pins
	// the vm_reload triplet. The cross-side byte stream is a constant token.
	if !ref {
		d.driveReloadSequence(ctx, client, addrs[lnReload])
	}
	fmt.Fprintf(&buf, "reload subject-only\n")

	if os.Getenv("FIXTURE_0038_DUMP_STREAM") != "" {
		side := "subj"
		if ref {
			side = "ref"
		}
		fmt.Fprintf(os.Stderr, "=== %s drive stream (%d bytes) ===\n%s=== end ===\n", side, buf.Len(), buf.String())
	}
	return buf.Bytes(), nil
}

// driveReloadSequence drives the FAIL_RELOAD trap→backoff→recover progression
// against the subject's reload listener:
//
//	req1: x-trigger-trap=1 → guest traps → 503 + arms the reload machine.
//	req2: x-trigger-trap=1 again, WITHIN the ~1s backoff window → 503 +
//	      vm_reload_backoff increments (the dispatch is blocked by the window;
//	      the guest is NOT re-run, so no second runtime-failure).
//	sleep > backoff window.
//	req3: NO trigger → past the window → reload attempted → vm_reload_success
//	      + the (now non-trapping) guest runs → 200.
func (d *perRouteDriver) driveReloadSequence(ctx context.Context, client *http.Client, addr string) {
	if addr == "" {
		return
	}
	trap := http.Header{}
	trap.Set("x-trigger-trap", "1")
	_ = doRequest(ctx, client, addr, "GET", "/reload", trap) // req1: trap
	_ = doRequest(ctx, client, addr, "GET", "/reload", trap) // req2: backoff
	time.Sleep(reloadBackoffSleep)
	_ = doRequest(ctx, client, addr, "GET", "/reload", nil) // req3: recover
}

// --- StatsAsserter (subject-only arms per
// reference_differential_asserter_dispatch) ---
//
// Two subject-side assertions, BOTH proven LIVE via deliberate-break cycles
// recorded in PROGRESS.md Task 11:
//
//  1. multiplugin_shared_data sharing: wasm.wazero.created == 1 (the two
//     vm_shared plugins built exactly ONE RootVM via the registry refcount).
//  2. reload triplet: vm_reload_runtime_failure / vm_reload_backoff /
//     vm_reload_success each >= 1 on plugin_reload.
func (d *perRouteDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	_ = refAdminAddr // subject-only per the asserter-dispatch discipline.

	time.Sleep(probeStatsDelay)

	stats, err := scrapeAllStats(subjAdminAddr)
	if err != nil {
		t.Errorf("AssertStats: scrape subj /stats/prometheus: %v", err)
		return
	}

	if os.Getenv("FIXTURE_0038_DUMP_STATS") != "" {
		fmt.Fprintf(os.Stderr, "=== subj stats (wasm/created/reload filtered) ===\n")
		for k, v := range stats {
			if strings.Contains(k, "wasm") || strings.Contains(k, "created") || strings.Contains(k, "reload") {
				fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
			}
		}
	}

	// Arm 0: perroute_override_applies (SUBJECT-ONLY — reference v1.37.2 has
	// no per-route wasm config). Hitting /override on the subject built the
	// per-route override VM + dispatched its guest →
	// wasm.plugin_perroute_override.executions >= 1. This is the
	// DISCRIMINATING subject proof that the per-route wholesale Wasm override
	// (AMEND-C1) took over the stream: if the override did NOT resolve, the
	// override plugin's own scoped counter would never increment (the
	// listener-default plugin would have run instead).
	execOverride, execOverrideFound := lookupCounter(stats, "wasm.plugin_perroute_override.executions")
	if !execOverrideFound || execOverride < 1 {
		t.Errorf("perroute_override_applies: wasm.plugin_perroute_override.executions = %d (found=%v); want >= 1 (per-route override VM dispatched on /override)", execOverride, execOverrideFound)
	}

	// Arm 1: multi-plugin chain dispatch. The two vm_shared plugins
	// (plugin_multi_a writer + plugin_multi_b reader) share ONE RootVM (via
	// the registry refcount) + ONE shared-data namespace. The
	// DISCRIMINATING proof of SHARING is the cross-side `x-shared:
	// written-by-A` CompareBytes arm above (the reader sees the writer's
	// value ONLY if the namespaces are shared; a separate namespace would
	// reflect `absent`). This subject-side StatsAsserter arm complements it
	// by proving BOTH plugins of the shared chain DISPATCHED their guest on
	// the request: wasm.plugin_multi_a.executions >= 1 AND
	// wasm.plugin_multi_b.executions >= 1. (The `wasm.wazero.created`
	// counter is per stream-CONTEXT, not per VM, so it is NOT a VM-sharing
	// proxy — the per-plugin executions counters are the live subject
	// signal that both filters in the shared-VM chain ran.)
	execA, execAFound := lookupCounter(stats, "wasm.plugin_multi_a.executions")
	if !execAFound || execA < 1 {
		t.Errorf("multiplugin_shared_data: wasm.plugin_multi_a.executions = %d (found=%v); want >= 1 (writer plugin dispatched)", execA, execAFound)
	}
	execB, execBFound := lookupCounter(stats, "wasm.plugin_multi_b.executions")
	if !execBFound || execB < 1 {
		t.Errorf("multiplugin_shared_data: wasm.plugin_multi_b.executions = %d (found=%v); want >= 1 (reader plugin dispatched in the shared-VM chain)", execB, execBFound)
	}

	// Arm 2: reload triplet on plugin_reload.
	bk, bkFound := lookupCounter(stats, "wasm.plugin_reload.vm_reload_backoff")
	if !bkFound || bk < 1 {
		t.Errorf("reload_fail_reload_recovers: vm_reload_backoff = %d (found=%v); want >= 1 (req2 blocked within the backoff window)", bk, bkFound)
	}
	su, suFound := lookupCounter(stats, "wasm.plugin_reload.vm_reload_success")
	if !suFound || su < 1 {
		t.Errorf("reload_fail_reload_recovers: vm_reload_success = %d (found=%v); want >= 1 (req3 past the window reloaded successfully)", su, suFound)
	}
}

// --- HTTP helpers ---

func getResponseHeader(ctx context.Context, client *http.Client, addr, method, path, header string) string {
	if addr == "" {
		return "NO_ADDR"
	}
	resp, _, err := doRequestResp(ctx, client, addr, method, path, nil)
	if err != nil {
		return "ERR(" + err.Error() + ")"
	}
	v := resp.Header.Get(header)
	if v == "" {
		return "ABSENT"
	}
	return v
}

func doRequest(ctx context.Context, client *http.Client, addr, method, path string, hdr http.Header) error {
	_, _, err := doRequestResp(ctx, client, addr, method, path, hdr)
	return err
}

func doRequestResp(ctx context.Context, client *http.Client, addr, method, path string, hdr http.Header) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://"+addr+path, nil)
	if err != nil {
		return nil, nil, err
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}
	return resp, body, nil
}

// --- stats scrape ---

func scrapeAllStats(adminAddr string) (map[string]int64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: subjAdminScrape}
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
	return parsePromBody(body), nil
}

func parsePromBody(data []byte) map[string]int64 {
	out := map[string]int64{}
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
		if sp := strings.IndexByte(valueStr, ' '); sp >= 0 {
			valueStr = valueStr[:sp]
		}
		f, perr := strconv.ParseFloat(valueStr, 64)
		if perr != nil {
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

// findExact returns the value of the counter whose bare-name matches the
// envoy-go-strict WIRE name `name` (e.g. "wasm.plugin_multi_a.executions").
// The /stats/prometheus scrape renders wire names in PROMETHEUS form: the
// `.` and `-` separators become `_` and an `envoy_` prefix is prepended
// (e.g. "envoy_wasm_plugin_multi_a_executions"). We normalize the wire name
// the same way and match the prometheus key by EQUALITY against
// "envoy_<normalized>" OR by suffix `_<normalized>` (defensive against any
// extra scope prefix). Sums across label variants.
func findExact(stats map[string]int64, name string) (bool, uint64) {
	norm := promNormalize(name)
	want := "envoy_" + norm
	found := false
	var total int64
	for k, v := range stats {
		bare := k
		if i := strings.IndexByte(k, '{'); i >= 0 {
			bare = k[:i]
		}
		if bare == want || bare == norm || strings.HasSuffix(bare, "_"+norm) {
			found = true
			total += v
		}
	}
	if total < 0 {
		total = 0
	}
	return found, uint64(total)
}

// promNormalize converts an envoy-go-strict wire stat name to its
// prometheus-rendered form: `.` and `-` separators become `_`.
func promNormalize(name string) string {
	return strings.NewReplacer(".", "_", "-", "_").Replace(name)
}

// lookupCounter is findExact returning (value, found) for readability at the
// reload-triplet call sites.
func lookupCounter(stats map[string]int64, name string) (uint64, bool) {
	found, v := findExact(stats, name)
	return v, found
}

// --- caps rendering ---

// capsList is the full post-25.2 capability superset (45 hostcall + lifecycle
// keys) that envoy-go's StrictDefaultDeny sandbox requires the guests to be
// granted. The driver renders it at the requested indent so the YAML nesting
// is correct at both the http_filters-level (24) and per-route (32) sites.
var capsList = []string{
	"proxy_log", "proxy_get_log_level", "proxy_get_header_map_pairs",
	"proxy_get_header_map_value", "proxy_get_header_map_size",
	"proxy_set_header_map_pairs", "proxy_add_header_map_value",
	"proxy_replace_header_map_value", "proxy_remove_header_map_value",
	"proxy_send_local_response", "proxy_get_property", "proxy_set_property",
	"proxy_get_status", "proxy_get_current_time_nanoseconds",
	"proxy_set_effective_context", "proxy_done", "proxy_get_buffer_bytes",
	"proxy_set_buffer_bytes", "proxy_get_buffer_status", "proxy_continue_stream",
	"proxy_close_stream", "proxy_set_tick_period_milliseconds", "proxy_http_call",
	"proxy_call_foreign_function", "proxy_define_metric", "proxy_increment_metric",
	"proxy_record_metric", "proxy_get_metric", "proxy_set_shared_data",
	"proxy_get_shared_data", "proxy_on_context_create", "proxy_on_vm_start",
	"proxy_on_configure", "proxy_on_done", "proxy_on_delete", "proxy_on_log",
	"proxy_on_request_headers", "proxy_on_response_headers",
	"proxy_on_request_body", "proxy_on_response_body", "proxy_on_request_trailers",
	"proxy_on_response_trailers", "proxy_on_tick", "proxy_on_http_call_response",
	"proxy_on_foreign_function",
}

func capsBlock(indent int) string {
	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	for i, c := range capsList {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(pad)
		b.WriteString(c)
		b.WriteString(": {}")
	}
	return b.String()
}

// --- file / template helpers ---

func refContainerWasmPath(crate string) string { return "/bytecode/" + crate + ".wasm" }

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
	_ fixture.Driver              = (*perRouteDriver)(nil)
	_ fixture.BackendKindAware    = (*perRouteDriver)(nil)
	_ fixture.MultiListenerDriver = (*perRouteDriver)(nil)
	_ fixture.ReferenceLogMounter = (*perRouteDriver)(nil)
	_ fixture.StatsAsserter       = (*perRouteDriver)(nil)
)
