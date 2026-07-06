// Package inputs registers the 0028-http-lua-multi-script-and-per-route
// fixture with the differential runner per phase 22.3 IMPL Task 5. Asserts
// per-scenario equivalence between envoy-go's envoy.filters.http.lua (the
// NEW 22.3 source_codes registry + LuaPerRoute per-route surface) and
// reference Envoy v1.37.2 across 6 CROSS-SIDE deterministic scenarios.
//
// Modeled EXACTLY on fixture-0027-http-lua-full-bridge's MultiListenerDriver
// (one listener per scenario), ReferenceHostMounts (bind-mount scripts/ into
// the reference container), host.docker.internal backend reach (ADR-0010),
// CompareBytes byte-exact dual-Drive, and the shared echobackend that
// reflects request headers as JSON. This driver does NOT implement
// BootRejectFixture (the source_codes compile-error boot-reject lives in the
// sibling fixture-0029-http-lua-source-codes-boot-reject).
//
// Topology (6 listeners, one per scenario; shared upstream cluster
// c_backend → echobackend subprocess):
//
//	l_test_a  → default_source_code only; route NO per-route
//	             → default.lua runs → x-lua-script=default                 (scenario a)
//	l_test_b  → source_codes{named_a,named_b}+default; route
//	             LuaPerRoute{name:named_a} → x-lua-script=named_a            (scenario b)
//	l_test_b2 → source_codes+default; route LuaPerRoute{name:ghost}
//	             (dangling, key absent) → SILENT NO-OP per AMEND-22.3-1
//	             → x-lua-script ABSENT                                      (scenario b dangling)
//	l_test_c  → default_source_code; route LuaPerRoute{source_code:
//	             override.lua} → x-lua-script=override                      (scenario c)
//	l_test_d  → default_source_code; route LuaPerRoute{disabled:true}
//	             → hooks skipped, default does NOT run → x-lua-script ABSENT (scenario d)
//	l_test_e  → source_codes{named_a,named_b}+default; route
//	             LuaPerRoute{name:named_b} → x-lua-script=named_b            (scenario e:
//	             distinct registry entry vs l_test_b's named_a)
//
// All 6 scenarios are CROSS-SIDE byte-exact via CompareBytes. The scripts
// perform deterministic header mutations ONLY (no :timestamp / :httpCall /
// non-deterministic API) so the reflected JSON — hence the per-scenario
// verdict line — is byte-stable across sides.
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
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0028-http-lua-multi-script-and-per-route"

	// Reference-container in-container listener ports. The runner exposes
	// each container port via testcontainers MappedPort; the driver dials
	// the host-mapped addr returned by ref.ListenerAddr(<containerPort>).
	refAdminPort   = 9901
	refLATestPort  = 10120 // l_test_a  — scenario (a) listener-default
	refLBTestPort  = 10121 // l_test_b  — scenario (b) per-route name:named_a
	refLB2TestPort = 10122 // l_test_b2 — scenario (b dangling) name:ghost
	refLCTestPort  = 10123 // l_test_c  — scenario (c) per-route source_code override
	refLDTestPort  = 10124 // l_test_d  — scenario (d) per-route disabled
	refLETestPort  = 10125 // l_test_e  — scenario (e) per-route name:named_b

	// Container-side absolute paths for the lua scripts. The runner
	// bind-mounts host scripts/<name>.lua onto these paths via
	// ReferenceHostMounts() per fixture-0026/0027 precedent.
	refContainerScriptDefault  = "/scripts/default.lua"
	refContainerScriptNamedA   = "/scripts/named_a.lua"
	refContainerScriptNamedB   = "/scripts/named_b.lua"
	refContainerScriptOverride = "/scripts/override.lua"
)

func init() {
	fixture.RegisterFixture(fixtureName, &luaDriver{})
}

// luaDriver is fixture-0028's per-driver state. No mutable state needed —
// it is a pure cross-side fixture (no boot-reject mode).
type luaDriver struct{}

// --- fixture.Driver (required) ---

func (*luaDriver) BackendCount() int                { return 1 }
func (*luaDriver) BackendKind() fixture.BackendKind { return fixture.HTTPLua }

func (*luaDriver) SubjectListenerName() string { return "l_test_a" }
func (*luaDriver) ReferenceListenerPort() int  { return refLATestPort }

// ReferenceBootstrap renders envoy.yaml with the per-scenario script
// container paths spliced in.
func (*luaDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":      refAdminPort,
		"LATestPort":     refLATestPort,
		"LBTestPort":     refLBTestPort,
		"LB2TestPort":    refLB2TestPort,
		"LCTestPort":     refLCTestPort,
		"LDTestPort":     refLDTestPort,
		"LETestPort":     refLETestPort,
		"BackendHost":    "host.docker.internal",
		"BackendPort":    backendPorts[0],
		"ScriptDefault":  refContainerScriptDefault,
		"ScriptNamedA":   refContainerScriptNamedA,
		"ScriptNamedB":   refContainerScriptNamedB,
		"ScriptOverride": refContainerScriptOverride,
	})
}

// SubjectConfig renders envoy-go.yaml with host-side script paths. The
// runner-allocated subjAdminPort splices into the admin socket address so
// the StartSubjectProxy "127.0.0.1:<subjAdminPort>" probe matches the
// bootstrap-bound admin listener.
func (*luaDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	fxDir := fixtureDir()
	return mustRender(tpl, map[string]any{
		"AdminPort":      subjAdminPort,
		"LATestPort":     subjListenerPort,
		"LBTestPort":     subjListenerPort + 1,
		"LB2TestPort":    subjListenerPort + 2,
		"LCTestPort":     subjListenerPort + 3,
		"LDTestPort":     subjListenerPort + 4,
		"LETestPort":     subjListenerPort + 5,
		"BackendPort":    backendPorts[0],
		"ScriptDefault":  filepath.Join(fxDir, "scripts", "default.lua"),
		"ScriptNamedA":   filepath.Join(fxDir, "scripts", "named_a.lua"),
		"ScriptNamedB":   filepath.Join(fxDir, "scripts", "named_b.lua"),
		"ScriptOverride": filepath.Join(fxDir, "scripts", "override.lua"),
	})
}

func (d *luaDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, deriveAddrsFromSingle(addr), "ref")
}

func (d *luaDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, deriveAddrsFromSingle(addr), "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
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
	return []string{"l_test_a", "l_test_b", "l_test_b2", "l_test_c", "l_test_d", "l_test_e"}
}

func (*luaDriver) ReferenceListenerPorts() []int {
	return []int{refLATestPort, refLBTestPort, refLB2TestPort, refLCTestPort, refLDTestPort, refLETestPort}
}

func (d *luaDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "ref")
}

func (d *luaDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "subj")
}

// --- fixture.ReferenceLogMounter ---

func (*luaDriver) ReferenceHostMounts() []fixture.HostMount {
	fxDir := fixtureDir()
	return []fixture.HostMount{
		{HostPath: filepath.Join(fxDir, "scripts", "default.lua"), ContainerPath: refContainerScriptDefault},
		{HostPath: filepath.Join(fxDir, "scripts", "named_a.lua"), ContainerPath: refContainerScriptNamedA},
		{HostPath: filepath.Join(fxDir, "scripts", "named_b.lua"), ContainerPath: refContainerScriptNamedB},
		{HostPath: filepath.Join(fxDir, "scripts", "override.lua"), ContainerPath: refContainerScriptOverride},
	}
}

// --- scenarioResult + driveProxy ---

type scenarioResult struct {
	statusCode int
	body       []byte
	headers    http.Header
	err        error
}

// driveProxy runs all 6 scenarios sequentially + emits a per-scenario
// verdict line into the byte buffer. The byte stream is identical per side
// (no side label emitted) so CompareBytes fires on equivalence.
func (d *luaDriver) driveProxy(ctx context.Context, addrs map[string]string, _ string) ([]byte, error) {
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}

	var buf bytes.Buffer

	// (a) listener-default → x-lua-script=default.
	emitScenario(&buf, "a", probe(ctx, client, addrs["l_test_a"], "/scenario_a"))
	// (b) per-route name:named_a → x-lua-script=named_a.
	emitScenario(&buf, "b", probe(ctx, client, addrs["l_test_b"], "/scenario_b"))
	// (b dangling) per-route name:ghost → x-lua-script ABSENT.
	emitScenario(&buf, "b2", probe(ctx, client, addrs["l_test_b2"], "/scenario_b2"))
	// (c) per-route source_code override → x-lua-script=override.
	emitScenario(&buf, "c", probe(ctx, client, addrs["l_test_c"], "/scenario_c"))
	// (d) per-route disabled → x-lua-script ABSENT.
	emitScenario(&buf, "d", probe(ctx, client, addrs["l_test_d"], "/scenario_d"))
	// (e) per-route name:named_b → x-lua-script=named_b.
	emitScenario(&buf, "e", probe(ctx, client, addrs["l_test_e"], "/scenario_e"))

	return buf.Bytes(), nil
}

// emitScenario formats the per-scenario verdict line. The verdict is the
// classified x-lua-script reflected-header value (or `x-lua-script=absent`).
func emitScenario(buf *bytes.Buffer, id string, r scenarioResult) {
	if r.err != nil {
		fmt.Fprintf(buf, "scenario %s status=ERR body=ERR (%v)\n", id, r.err)
		return
	}
	fmt.Fprintf(buf, "scenario %s status=%d body=%s\n", id, r.statusCode, classifyBody(r.body))
}

// classifyBody parses the echobackend reflected JSON + returns the
// normalized x-lua-script verdict. Returns `x-lua-script=<value>` when the
// header is present, `x-lua-script=absent` when absent (the AMEND-22.3-1
// dangling-name + the disabled scenarios), or a mismatch token when the
// body is not a parseable echo envelope.
func classifyBody(body []byte) string {
	hdrs := reflectedHeaders(body)
	if hdrs == nil {
		return fmt.Sprintf("mismatch(not_echo_json,got=%q)", trim(body))
	}
	if v, ok := hdrs["x-lua-script"]; ok {
		return "x-lua-script=" + v
	}
	return "x-lua-script=absent"
}

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

func trim(body []byte) string {
	const max = 80
	s := string(body)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func probe(ctx context.Context, client *http.Client, addr, path string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+path, nil)
	if err != nil {
		return scenarioResult{err: err}
	}
	resp, err := client.Do(req)
	if err != nil {
		return scenarioResult{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return scenarioResult{err: err}
	}
	return scenarioResult{statusCode: resp.StatusCode, body: body, headers: resp.Header}
}

// --- address-derivation helper (single-addr fallback) ---
//
// The runner invokes DriveReferenceMulti / DriveSubjectMulti directly per
// MultiListenerDriver dispatch, so this single-addr fallback is unused at
// runtime. Kept for the Driver-interface single-addr contract.
func deriveAddrsFromSingle(s1Addr string) map[string]string {
	lastColon := strings.LastIndex(s1Addr, ":")
	if lastColon < 0 {
		return map[string]string{"l_test_a": s1Addr}
	}
	hostPart := s1Addr[:lastColon]
	port, err := strconv.Atoi(s1Addr[lastColon+1:])
	if err != nil {
		return map[string]string{"l_test_a": s1Addr}
	}
	return map[string]string{
		"l_test_a":  s1Addr,
		"l_test_b":  fmt.Sprintf("%s:%d", hostPart, port+1),
		"l_test_b2": fmt.Sprintf("%s:%d", hostPart, port+2),
		"l_test_c":  fmt.Sprintf("%s:%d", hostPart, port+3),
		"l_test_d":  fmt.Sprintf("%s:%d", hostPart, port+4),
		"l_test_e":  fmt.Sprintf("%s:%d", hostPart, port+5),
	}
}

// --- file / template helpers (mirrors fixture-0026/0027) ---

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
)
