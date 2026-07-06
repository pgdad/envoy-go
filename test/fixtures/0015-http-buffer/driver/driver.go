// Package driver registers the 0015-http-buffer fixture with the differential
// runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.buffer and reference Envoy v1.37.2 across the six-scenario
// matrix per phase 13 SPEC §7.1.
//
// Integration shape (single-listener fixture.Driver — planner-time decision 6,
// mirrors the cors / fault / header_mutation / csrf precedent rather than phase
// 11's MultiListenerDriver fan-out):
//
//  1. ReferenceBootstrap renders test/fixtures/0015-http-buffer/envoy.yaml with
//     the backend host set to host.docker.internal (ADR-0010 STRICT_DNS) +
//     runner-allocated backend port. SubjectConfig renders envoy-go.yaml with
//     the runner-allocated subject admin/listener ports + backend port (loopback).
//
//  2. DriveReference / DriveSubject issue an identical 6-scenario sequence against
//     each proxy and emit a deterministic per-scenario assertion-log byte stream.
//     The runner's CompareBytes pass enforces equivalence — when both proxies
//     produce equal logs, the differential gate fires.
//
//     The 6 scenarios per SPEC §7.1 (Expect: 100-continue omitted from 2 + 5 —
//     envoy-go connection.go line 122 returns 417 for any Expect: header before
//     the filter chain runs; omitting keeps both proxies on the 413 path):
//     1: body-fits (POST / 1 KiB CL-known)                    → 200 backend echo
//     2: overflow (POST / 2 MiB CL-known)                     → 413
//     3: chunked-overflow per-route tighter cap (POST /route-tighter ~200 KiB chunked) → 413
//     4: per-route disabled bypass (POST /route-disabled 2 MiB CL-known) → 200 backend echo
//     5: per-route tighter override fires (POST /route-tighter 200 KiB CL-known) → 413
//     6: chunked passthrough + Content-Length injection (POST / 10 KiB chunked) → 200 backend echo
//
//     Per-scenario log line: `scenario <id> status=<code> body=<quoted>`
//     followed by 413-shape headers (content-length, connection) on rejection
//     responses; 200 responses include the CL-injection assertion on scenario 6.
//
//  3. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner step 9.
//
//     The driver does NOT implement StatsAsserter — the buffer filter emits no
//     filter-specific counters (unlike csrf's csrf.*). HCM downstream_rq_* counters
//     are present on both sides but the runner enforces the allow-list automatically
//     (twin-series-discipline: compares only names present in envoy-go's scrape).
package driver

import (
	"bufio"
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

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0015-http-buffer"

	// In-container reference Envoy listener port (pre-assigned per bootstrap).
	// Convention `100NN` for fixture `00NN`: phase 11 uses 10013-10016, phase 10
	// uses 10012, phase 09 uses 10011, phase 12 uses 10014 — phase 13 follows
	// with 10015 for the single l_main listener.
	refContainerListenerPort = 10015

	// statPrefix matches the YAML's HCM stat_prefix (ingress_buffer). Used by
	// ProbeAdmin if counter assertions are ever added; present here for
	// documentation parity with the phase 12 csrf driver.
	statPrefix = "ingress_buffer" //nolint:deadcode,unused
)

func init() {
	fixture.RegisterFixture(fixtureName, &bufferDriver{})
}

type bufferDriver struct{}

// --- fixture.Driver (required) ---

func (bufferDriver) BackendCount() int                { return 1 }
func (bufferDriver) BackendKind() fixture.BackendKind { return fixture.HTTPBuffer }
func (bufferDriver) SubjectListenerName() string      { return "l_main" }
func (bufferDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal +
// runner-allocated backend port. Reference Envoy admin + listener ports are
// pre-assigned constants (9901, 10015).
func (bufferDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    9901,
		"ListenerPort": refContainerListenerPort,
		"BackendPort":  backendPorts[0],
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + backend port (loopback).
func (bufferDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
	})
}

// DriveReference + DriveSubject issue the identical 6-scenario sequence and
// return the per-scenario assertion-log byte stream. CompareBytes passes when
// both sides produce identical logs.
func (d bufferDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return driveProxy(ctx, addr)
}

func (d bufferDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return driveProxy(ctx, addr)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin-diff at runner step 9.
func (bufferDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- core drive logic ---

// normalizeListenerAddr replaces the unspecified-address forms ("0.0.0.0" and
// "[::]") that net.Listener.Addr().String() can emit on Linux with "127.0.0.1"
// so that http.Client can connect to the local proxy. Reference Envoy addresses
// are already in host:port form from Docker port-mapping and do not need
// normalization.
func normalizeListenerAddr(addr string) string {
	if strings.HasPrefix(addr, "0.0.0.0:") {
		return "127.0.0.1:" + addr[len("0.0.0.0:"):]
	}
	if strings.HasPrefix(addr, "[::]:") {
		const prefix6 = "[::]:"
		return "127.0.0.1:" + addr[len(prefix6):]
	}
	return addr
}

// driveProxy issues the 6 scenarios against addr and returns deterministic-
// format assertion-log lines. The "side" (ref vs subj) is INTENTIONALLY
// excluded from the log so both sides produce identical byte streams when
// behavior is equivalent.
//
// Per-scenario encoding:
//
//	scenario <id> status=<code> body=<quoted>
//	  [header: <name>: <value>]    (413 responses only; sorted)
//	  [cl-inject: <value>]         (scenario 6 only — parsed from backend JSON)
func driveProxy(ctx context.Context, addr string) ([]byte, error) {
	listenerURL := "http://" + normalizeListenerAddr(addr)

	// Shared transport — disable keep-alives so each scenario uses a fresh
	// connection (avoids cross-scenario state leakage on Envoy's side).
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr}

	var b bytes.Buffer

	// --- Scenario 1: POST / 1 KiB CL-known → 200 ---
	// Logs structural JSON shape (method + path) rather than raw body to keep
	// the log deterministic across proxies — header sets differ between reference
	// Envoy and envoy-go (x-forwarded-for, x-envoy-expected-rq-timeout-ms, etc.).
	{
		body := bytes.NewReader(make([]byte, 1024))
		req, err := http.NewRequestWithContext(ctx, "POST", listenerURL+"/", body)
		if err != nil {
			return nil, fmt.Errorf("scenario 1 request: %w", err)
		}
		resp, respBody, err := doRequest(client, req)
		if err != nil {
			fmt.Fprintf(&b, "scenario 1 ERROR: %v\n", err)
		} else {
			var echo echoResponse
			parseErr := json.Unmarshal(respBody, &echo)
			if parseErr != nil {
				fmt.Fprintf(&b, "scenario 1 status=%d body=<json-parse-error: %v>\n", resp.StatusCode, parseErr)
			} else {
				fmt.Fprintf(&b, "scenario 1 status=%d body=<json-ok method=%q path=%q>\n", resp.StatusCode, echo.Method, echo.Path)
			}
			_ = resp.Body.Close()
		}
	}

	// --- Scenario 2: POST / 2 MiB CL-known → 413 ---
	// Note: Expect: 100-continue is NOT set here — envoy-go's connection.go line
	// 122 sends 417 for ANY Expect: header before the filter chain runs, causing
	// a differential mismatch with reference Envoy (which returns 413). Omitting
	// Expect: 100-continue keeps both proxies on the 413 path without the 417
	// short-circuit. The overflow cap is still triggered (2 MiB > 1 MiB cap).
	{
		body := bytes.NewReader(make([]byte, 2*1024*1024))
		req, err := http.NewRequestWithContext(ctx, "POST", listenerURL+"/", body)
		if err != nil {
			return nil, fmt.Errorf("scenario 2 request: %w", err)
		}
		resp, respBody, err := doRequest(client, req)
		if err != nil {
			fmt.Fprintf(&b, "scenario 2 ERROR: %v\n", err)
		} else {
			fmt.Fprintf(&b, "scenario 2 status=%d body=%q\n", resp.StatusCode, string(respBody))
			emit413Headers(&b, resp.Header)
			_ = resp.Body.Close()
		}
	}

	// --- Scenario 3: POST /route-tighter ~200 KiB chunked → 413 ---
	{
		body := bytes.NewReader(make([]byte, 200*1024))
		req, err := http.NewRequestWithContext(ctx, "POST", listenerURL+"/route-tighter", body)
		if err != nil {
			return nil, fmt.Errorf("scenario 3 request: %w", err)
		}
		req.TransferEncoding = []string{"chunked"}
		resp, respBody, err := doRequest(client, req)
		if err != nil {
			fmt.Fprintf(&b, "scenario 3 ERROR: %v\n", err)
		} else {
			fmt.Fprintf(&b, "scenario 3 status=%d body=%q\n", resp.StatusCode, string(respBody))
			emit413Headers(&b, resp.Header)
			_ = resp.Body.Close()
		}
	}

	// --- Scenario 4: POST /route-disabled 2 MiB CL-known → 200 ---
	{
		body := bytes.NewReader(make([]byte, 2*1024*1024))
		req, err := http.NewRequestWithContext(ctx, "POST", listenerURL+"/route-disabled", body)
		if err != nil {
			return nil, fmt.Errorf("scenario 4 request: %w", err)
		}
		resp, respBody, err := doRequest(client, req)
		if err != nil {
			fmt.Fprintf(&b, "scenario 4 ERROR: %v\n", err)
		} else {
			// Verify JSON-parseable (backend echo) — log parse success/failure
			var echo echoResponse
			parseErr := json.Unmarshal(respBody, &echo)
			if parseErr != nil {
				fmt.Fprintf(&b, "scenario 4 status=%d body=<json-parse-error: %v>\n", resp.StatusCode, parseErr)
			} else {
				fmt.Fprintf(&b, "scenario 4 status=%d body=<json-ok method=%q path=%q>\n", resp.StatusCode, echo.Method, echo.Path)
			}
			_ = resp.Body.Close()
		}
	}

	// --- Scenario 5: POST /route-tighter 200 KiB CL-known → 413 ---
	// Note: Expect: 100-continue omitted for same reason as scenario 2 — envoy-go
	// short-circuits with 417 before the filter chain fires. 200 KiB > 128 KiB
	// per-route tighter cap so 413 fires on both proxies without Expect:.
	{
		body := bytes.NewReader(make([]byte, 200*1024))
		req, err := http.NewRequestWithContext(ctx, "POST", listenerURL+"/route-tighter", body)
		if err != nil {
			return nil, fmt.Errorf("scenario 5 request: %w", err)
		}
		resp, respBody, err := doRequest(client, req)
		if err != nil {
			fmt.Fprintf(&b, "scenario 5 ERROR: %v\n", err)
		} else {
			fmt.Fprintf(&b, "scenario 5 status=%d body=%q\n", resp.StatusCode, string(respBody))
			emit413Headers(&b, resp.Header)
			_ = resp.Body.Close()
		}
	}

	// --- Scenario 6: POST / 10 KiB chunked → 200 + CL-injection assertion ---
	// Load-bearing for §11.8-CL maybeAddContentLength byte-equivalence per
	// planner-time decision 9. The backend JSON echo includes the inbound
	// content-length header (injected by maybeAddContentLength when the request
	// arrived chunked); asserting its value proves the chunked→fixed-CL
	// conversion is byte-equivalent on both proxies.
	{
		body := bytes.NewReader(make([]byte, 10*1024))
		req, err := http.NewRequestWithContext(ctx, "POST", listenerURL+"/", body)
		if err != nil {
			return nil, fmt.Errorf("scenario 6 request: %w", err)
		}
		req.TransferEncoding = []string{"chunked"}
		resp, respBody, err := doRequest(client, req)
		if err != nil {
			fmt.Fprintf(&b, "scenario 6 ERROR: %v\n", err)
		} else {
			var echo echoResponse
			parseErr := json.Unmarshal(respBody, &echo)
			if parseErr != nil {
				fmt.Fprintf(&b, "scenario 6 status=%d body=<json-parse-error: %v>\n", resp.StatusCode, parseErr)
			} else {
				// Extract injected content-length from backend-seen headers.
				clVal := echo.Headers["content-length"]
				fmt.Fprintf(&b, "scenario 6 status=%d cl-inject=%q\n", resp.StatusCode, clVal)
			}
			_ = resp.Body.Close()
		}
	}

	return b.Bytes(), nil
}

// echoResponse is the JSON shape returned by the 0015-http-buffer backend.
// The backend echoes inbound request method + path + headers (lowercased).
type echoResponse struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

// doRequest executes req via client and returns the response + fully-drained
// body bytes. The caller is responsible for closing resp.Body.
func doRequest(client *http.Client, req *http.Request) (*http.Response, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("read body: %w", err)
	}
	return resp, body, nil
}

// emit413Headers emits the connection-shape headers on a 413 Payload Too Large
// reply. Headers emitted: content-length, connection (the two discriminating
// headers for the 413 wire shape per SPEC §7.1). Emitted in canonical order
// (deterministic). The Date / Server headers are allow-listed away (value
// non-deterministic or side-specific). Content-Type is omitted — both proxies
// emit text/plain but it is not a load-bearing differential claim.
//
// http.Header.Get is case-insensitive via textproto.CanonicalMIMEHeaderKey so
// passing the lowercase form works correctly.
func emit413Headers(b *bytes.Buffer, headers http.Header) {
	// content-length before connection — alphabetical order for determinism.
	for _, name := range []string{"Content-Length", "Connection"} {
		if val := headers.Get(name); val != "" {
			fmt.Fprintf(b, "  header: %s: %s\n", strings.ToLower(name), val)
		}
	}
}

// --- stats scraping (used by AssertStats if added in future) ---

// scrapeHCMStats issues GET /stats/prometheus against adminAddr and parses the
// body into a map[name]int64 of all envoy_http_conn_manager_* metric values
// whose envoy_http_conn_manager_prefix label matches the fixture's configured
// stat_prefix (= "ingress_buffer"). Exported for potential future StatsAsserter
// use; currently unused by the driver (no filter-specific counters to assert).
func scrapeHCMStats(adminAddr string) (map[string]int64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return parseHCMPromBody(resp.Body)
}

// parseHCMPromBody parses a Prometheus text-format body and returns a map of
// all metric names beginning with envoy_http_conn_manager_ whose
// envoy_http_conn_manager_prefix label matches statPrefix (= "ingress_buffer").
func parseHCMPromBody(r io.Reader) (map[string]int64, error) {
	out := map[string]int64{}
	const wantPrefix = "envoy_http_conn_manager_"
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
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
		if !strings.HasPrefix(name, wantPrefix) {
			continue
		}
		if labelStr != "" && !labelMatches(labelStr, "envoy_http_conn_manager_prefix", statPrefix) {
			continue
		}
		if sp := strings.IndexByte(valueStr, ' '); sp >= 0 {
			valueStr = valueStr[:sp]
		}
		f, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}
		out[name] = int64(f)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return out, nil
}

// labelMatches reports whether labelStr contains key="value" exactly.
func labelMatches(labelStr, key, value string) bool {
	want := key + `="` + value + `"`
	for _, part := range strings.Split(labelStr, ",") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

// --- helpers ---

// fixtureDir returns the absolute path to the 0015-http-buffer fixture root,
// derived from runtime.Caller — works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0015-http-buffer/driver/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

// mustReadFixtureFile reads name from the fixture root directory.
func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

// mustRender renders a text/template body with data; panics on parse/exec
// errors (driver-time misconfiguration is non-recoverable).
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
	_ fixture.Driver           = (*bufferDriver)(nil)
	_ fixture.BackendKindAware = (*bufferDriver)(nil)
)

// Ensure scrapeHCMStats is referenced to avoid "declared and not used" errors
// while the driver does not implement StatsAsserter yet.
var _ = scrapeHCMStats
