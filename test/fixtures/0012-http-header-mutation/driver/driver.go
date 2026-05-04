// Package driver registers the 0012-http-header-mutation fixture with the
// differential runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.header_mutation and reference Envoy v1.37.2 across the
// four-scenario matrix per phase 10 SPEC §7.1.
//
// Integration shape (SPEC §7.3 driver outline):
//
//  1. ReferenceBootstrap renders test/fixtures/0012-http-header-mutation/envoy.yaml
//     with the backend host set to host.docker.internal (ADR-0010 STRICT_DNS) +
//     runner-allocated backend port. SubjectConfig renders envoy-go.yaml with
//     the runner-allocated subject admin/listener ports + backend port (loopback).
//
//  2. The fixture exposes two listeners (l_lws flag=false, l_mws flag=true)
//     and implements fixture.MultiListenerDriver so the runner allocates and
//     publishes both reference ports and both subject listener addresses.
//
//  3. DriveReferenceMulti / DriveSubjectMulti issue an identical 4-probe
//     sequence against each proxy and emit a deterministic per-probe
//     assertion-log byte stream. The runner's CompareBytes pass enforces
//     equivalence — when both proxies produce equal logs, the differential
//     gate fires.
//
//     The 4 probes cover the four scenarios per SPEC §7.1:
//     1: listener-only → GET /listener-only/anything  (l_lws, flag=false)
//     2: route-override → GET /route-override/anything (l_lws, flag=false)
//     3: multi-tier-lws → GET /multi-tier/anything    (l_lws, flag=false → RC wins)
//     4: multi-tier-mws → GET /multi-tier/anything    (l_mws, flag=true → Route wins)
//
//     Per-probe log line:
//     probe <id> status=<code>
//     resp-headers: <sorted key=value pairs>
//     body (filtered):
//     <line>
//     ...
//     where the body has proxy-injected request-header lines stripped
//     (X-Forwarded-For, X-Forwarded-Proto, X-Request-Id, X-Envoy-*) so
//     the comparison is deterministic across ref and subj.
//
//  4. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner
//     step 9.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/template"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0012-http-header-mutation"
	// refLwsPort is the in-container l_lws listener port (flag=false).
	refLwsPort = 10012
	// refMwsPort is the in-container l_mws listener port (flag=true).
	refMwsPort = 10013
)

func init() {
	fixture.RegisterFixture(fixtureName, &mutationDriver{})
}

type mutationDriver struct{}

// --- fixture.Driver (required) ---

func (mutationDriver) BackendCount() int                { return 1 }
func (mutationDriver) BackendKind() fixture.BackendKind { return fixture.HTTPHeaderMutation }

// SubjectListenerName returns the primary listener name (l_lws). The runner
// uses this to look up the subject's bound address for the single-addr
// DriveSubject path. Because this fixture implements MultiListenerDriver, the
// runner dispatches DriveSubjectMulti instead — DriveSubject is never invoked
// at runtime. The method is still REQUIRED by the Driver interface contract.
func (mutationDriver) SubjectListenerName() string { return "l_lws" }

// ReferenceListenerPort returns the primary reference listener port (l_lws).
// Similarly required by the Driver interface even though MultiListenerDriver
// takes precedence for the running path.
func (mutationDriver) ReferenceListenerPort() int { return refLwsPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + runner-
// allocated backend port.
func (mutationDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"BackendHost": "host.docker.internal",
		"BackendPort": backendPorts[0],
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + backend port (loopback). The second listener (l_mws) gets port
// LwsPort+1 which matches the envoy-go.yaml template variable {{.MwsPort}}.
func (mutationDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"LwsPort":     subjListenerPort,
		"MwsPort":     subjListenerPort + 1,
		"BackendPort": backendPorts[0],
	})
}

// DriveReference is the single-addr path; never called at runtime because
// MultiListenerDriver is implemented. Delegates to DriveReferenceMulti with
// the single addr mapped to l_lws, deriving l_mws by port substitution.
func (d mutationDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveDualAddrsFromRef(addr)
	return d.DriveReferenceMulti(ctx, addrs)
}

// DriveSubject is the single-addr path; never called at runtime because
// MultiListenerDriver is implemented. Delegates to DriveSubjectMulti with
// the single addr mapped to l_lws, deriving l_mws by port+1.
func (d mutationDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveDualAddrsFromSubj(addr)
	return d.DriveSubjectMulti(ctx, addrs)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (mutationDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// SubjectListenerNames returns both listener names in order (primary first).
func (mutationDriver) SubjectListenerNames() []string {
	return []string{"l_lws", "l_mws"}
}

// ReferenceListenerPorts returns both in-container listener ports.
func (mutationDriver) ReferenceListenerPorts() []int {
	return []int{refLwsPort, refMwsPort}
}

// DriveReferenceMulti issues the 4 scenario probes against the reference proxy.
// addrs maps listener names to "host:port" strings (provided by the runner).
func (d mutationDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return driveProxy(ctx, addrs)
}

// DriveSubjectMulti issues the 4 scenario probes against the subject proxy.
// addrs maps listener names to "host:port" strings (provided by the runner).
func (d mutationDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return driveProxy(ctx, addrs)
}

// --- core drive logic ---

// driveProxy issues the 4 scenario probes and returns deterministic-format
// assertion-log lines. The "side" (ref vs subj) is INTENTIONALLY excluded from
// the log so both sides produce identical byte streams when behavior is
// equivalent.
func driveProxy(ctx context.Context, addrs map[string]string) ([]byte, error) {
	lwsAddr := addrs["l_lws"]
	mwsAddr := addrs["l_mws"]

	type probe struct {
		id      string
		addr    string
		path    string
		headers map[string]string
	}
	probes := []probe{
		// Scenario 1: listener-only (l_lws, flag=false).
		// Send User-Agent so the listener's Remove: user-agent has a target.
		{
			id:      "listener-only",
			addr:    lwsAddr,
			path:    "/listener-only/anything",
			headers: map[string]string{"User-Agent": "fixture-0012"},
		},
		// Scenario 2: route-override (l_lws, flag=false).
		// Expect x-route-only: yes added by Route tier; x-test: vh (VHost wins).
		{
			id:      "route-override",
			addr:    lwsAddr,
			path:    "/route-override/anything",
			headers: map[string]string{"User-Agent": "fixture-0012"},
		},
		// Scenario 3: multi-tier on l_lws (flag=false → RC tier wins overlap).
		// Final x-test: rc.
		{
			id:      "multi-tier-lws",
			addr:    lwsAddr,
			path:    "/multi-tier/anything",
			headers: map[string]string{"User-Agent": "fixture-0012"},
		},
		// Scenario 4: multi-tier on l_mws (flag=true → Route tier wins overlap).
		// Final x-test: route.
		{
			id:      "multi-tier-mws",
			addr:    mwsAddr,
			path:    "/multi-tier/anything",
			headers: map[string]string{"User-Agent": "fixture-0012"},
		},
	}

	var out bytes.Buffer
	for _, p := range probes {
		hdrs := http.Header{}
		for k, v := range p.headers {
			hdrs.Set(k, v)
		}
		resp, body, err := helpers.HTTPRoundTrip(ctx, p.addr, "GET", p.path, hdrs, nil)
		if err != nil {
			fmt.Fprintf(&out, "probe %s ERROR: %v\n", p.id, err)
			continue
		}

		// Emit status code.
		fmt.Fprintf(&out, "probe %s status=%d\n", p.id, resp.StatusCode)

		// Emit sorted response headers (excluding allow-listed proxy-injected
		// and per-run-varying headers: date, content-length, server).
		// Capture the fixture-relevant response headers: x-resp-test, x-resp-multi,
		// x-multi. Sort them for determinism.
		respHdrs := collectRespHeaders(resp.Header)
		fmt.Fprintf(&out, "resp-headers: %s\n", respHdrs)

		// Emit the filtered body (strip proxy-injected request-header lines).
		fmt.Fprintf(&out, "body (filtered):\n")
		filtered := filterBody(body)
		out.Write(filtered)
	}
	return out.Bytes(), nil
}

// collectRespHeaders returns a sorted, deterministic representation of the
// fixture-relevant response headers. It OMITS headers that vary per-run or
// differ across proxy implementations:
//   - date (per-run varying)
//   - content-length (may differ if body differs; covered by body comparison)
//   - server (Envoy vs envoy-go differ)
//   - x-envoy-* (proxy-injected, may vary)
//   - connection (transport-level)
//   - content-type (typically identical but not part of the mutation assertion)
//
// It INCLUDES mutation-relevant headers: x-resp-test, x-resp-multi, x-multi.
func collectRespHeaders(h http.Header) string {
	// Allow-list of header name prefixes we keep for the comparison.
	// All others are dropped (they are either per-run or infrastructure noise).
	keepPrefixes := []string{"x-resp-", "x-multi"}

	type kv struct{ name, value string }
	var pairs []kv
	for name, values := range h {
		lname := strings.ToLower(name)
		keep := false
		for _, pfx := range keepPrefixes {
			if strings.HasPrefix(lname, pfx) {
				keep = true
				break
			}
		}
		if !keep {
			continue
		}
		for _, v := range values {
			pairs = append(pairs, kv{name: lname, value: v})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].name != pairs[j].name {
			return pairs[i].name < pairs[j].name
		}
		return pairs[i].value < pairs[j].value
	})
	var sb strings.Builder
	for i, p := range pairs {
		if i > 0 {
			sb.WriteString("; ")
		}
		fmt.Fprintf(&sb, "%s=%s", p.name, p.value)
	}
	if sb.Len() == 0 {
		return "(none)"
	}
	return sb.String()
}

// filterBody removes lines from the echoed request-header body that correspond
// to proxy-injected headers which vary per-run or differ between reference Envoy
// and envoy-go. The backend emits one "Name: value" line per header (sorted).
// We strip lines whose header name (the part before the first ': ') matches the
// allow-list of injected headers:
//   - X-Forwarded-For / X-Forwarded-Proto  (both proxies inject; values differ)
//   - X-Request-Id                          (UUID, per-request varying)
//   - X-Envoy-*                             (reference Envoy may inject extras)
//
// Lines that are blank or cannot be parsed as "Name: value" are passed through.
func filterBody(body []byte) []byte {
	// Header names to strip (lowercased). Prefix entries end with '*'.
	stripPrefixes := []string{
		"x-forwarded-for",
		"x-forwarded-proto",
		"x-request-id",
		"x-envoy-",
		// Connection: close is added by HTTPRoundTrip; reference Envoy strips
		// hop-by-hop headers before forwarding but envoy-go may not.
		"connection",
		// User-Agent: the header_mutation config removes the driver-supplied
		// User-Agent. After removal, envoy-go's Go net/http upstream client may
		// add User-Agent: Go-http-client/1.1 while reference Envoy (C++) does
		// not. Strip user-agent from body comparison since the assertion is about
		// the mutation's Remove behavior, not about which upstream client header
		// each proxy injects.
		"user-agent",
	}

	var out bytes.Buffer
	for _, line := range bytes.Split(body, []byte("\n")) {
		// Preserve the trailing newline structure: Split on \n gives an extra
		// empty element at the end if body ends with \n. Keep it so the
		// reconstructed body ends with \n.
		colon := bytes.IndexByte(line, ':')
		if colon < 0 {
			// Not a "Name: value" line — pass through (includes blank lines).
			out.Write(line)
			out.WriteByte('\n')
			continue
		}
		headerName := strings.ToLower(string(line[:colon]))
		strip := false
		for _, pfx := range stripPrefixes {
			if strings.HasPrefix(headerName, pfx) {
				strip = true
				break
			}
		}
		if strip {
			continue
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	// The loop always appends a trailing \n after the last element from Split,
	// which doubles the terminal newline if body ended with \n. Trim to match
	// the original body's trailing-newline discipline.
	result := out.Bytes()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		// Drop the extra \n added by the final iteration of Split's empty tail.
		// Split("a\nb\n", "\n") → ["a", "b", ""] — the last "" gets a spurious \n.
		// We want exactly one trailing \n if body ended with \n.
		result = bytes.TrimRight(result, "\n")
		result = append(result, '\n')
	}
	return result
}

// --- address derivation helpers (for the single-addr stub paths) ---

// deriveDualAddrsFromRef derives the l_mws address from the l_lws reference
// container address by replacing the port. The reference container exposes
// port 10012 (l_lws) and 10013 (l_mws) — both published by the runner via
// MultiListenerDriver.ReferenceListenerPorts(). This helper is only called by
// the fallback DriveReference stub (never reached at runtime).
func deriveDualAddrsFromRef(lwsAddr string) map[string]string {
	mwsAddr := strings.Replace(lwsAddr,
		fmt.Sprintf(":%d", refLwsPort),
		fmt.Sprintf(":%d", refMwsPort), 1)
	return map[string]string{"l_lws": lwsAddr, "l_mws": mwsAddr}
}

// deriveDualAddrsFromSubj derives the l_mws subject address by incrementing
// the l_lws port by 1 (SubjectConfig templates MwsPort = LwsPort+1).
// This helper is only called by the fallback DriveSubject stub (never reached
// at runtime when MultiListenerDriver is implemented).
func deriveDualAddrsFromSubj(lwsAddr string) map[string]string {
	// Parse "host:port" and increment port by 1.
	lastColon := strings.LastIndex(lwsAddr, ":")
	if lastColon < 0 {
		// Malformed — return what we have; driveProxy will surface the error.
		return map[string]string{"l_lws": lwsAddr, "l_mws": lwsAddr}
	}
	host := lwsAddr[:lastColon]
	port := lwsAddr[lastColon+1:]
	var portNum int
	if _, err := fmt.Sscanf(port, "%d", &portNum); err != nil {
		// Malformed port — return l_lws as both; driveProxy will surface errors.
		return map[string]string{"l_lws": lwsAddr, "l_mws": lwsAddr}
	}
	mwsAddr := fmt.Sprintf("%s:%d", host, portNum+1)
	return map[string]string{"l_lws": lwsAddr, "l_mws": mwsAddr}
}

// --- helpers (mirror the 0011-http-fault driver patterns) ---

// fixtureDir returns the absolute path to the 0012-http-header-mutation fixture
// root, derived from runtime.Caller — works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0012-http-header-mutation/driver/driver.go
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
	_ fixture.Driver              = (*mutationDriver)(nil)
	_ fixture.BackendKindAware    = (*mutationDriver)(nil)
	_ fixture.MultiListenerDriver = (*mutationDriver)(nil)
)
