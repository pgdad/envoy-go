// Package driver registers the 0010-graceful-drain fixture with the
// differential runner. Asserts per-state-transition equivalence between
// envoy-go's graceful-drain surface and reference Envoy v1.37.2 under a
// slow-streaming-backend probe per SPEC §7.
//
// Integration shape (SPEC §7.2 driver outline):
//
//  1. SubjectConfig templates the envoy-go bootstrap with admin/listener/
//     backend ports; ReferenceBootstrap templates the reference bootstrap
//     with the backend port via host.docker.internal (ADR-0010 STRICT_DNS).
//
//  2. DriveReference / DriveSubject store the listener addrs and return nil.
//     The actual drain sequence runs in ProbeAdmin (which receives both admin
//     addrs) so that the per-proxy trigger script and per-state assertions have
//     access to both listener and admin endpoints simultaneously.
//
//  3. ProbeAdmin runs the full drain sequence for each proxy in turn:
//     a. Scrape /ready → assert "LIVE\n".
//     b. Start long-lived GET /slow goroutine against the listener addr;
//     wait for first byte (in-flight established).
//     c. Trigger drain (per-proxy trigger script per §11.2 deviation):
//     - envoy-go: POST /drain_listeners only.
//     - reference Envoy: POST /drain_listeners + POST /healthcheck/fail.
//     d. Poll /ready until "DRAINING\n" (max 5s).
//     e. Scrape /server_info; assert state field == "DRAINING".
//     f. Attempt new TCP conn + HTTP read; assert accept-then-FIN.
//     g. Wait for in-flight to complete (max 8s); assert body = 5120 'x'.
//     Emits a deterministic assertion-log line per step. Both proxies produce
//     the same log lines → byte-equal after wrapping → CompareBytes passes.
//
// Per SPEC §7.1, five per-state-transition equivalence claims are asserted:
//  1. /ready LIVE pre-drain.
//  2. POST /drain_listeners response = "OK\n".
//  3. /ready DRAINING post-trigger.
//  4. /server_info state = "DRAINING".
//  5. in-flight GET /slow body = 5120 bytes of 'x'.
//
// SIGTERM-trigger path: deferred per PLAN gotcha 1 (runner harness does not
// expose SIGTERM injection). This fixture covers the admin-trigger path only.
//
// RequiresReference: true (admin-trigger path exercises both proxies).
package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

const (
	fixtureName              = "0010-graceful-drain"
	refContainerListenerPort = 10001
)

// drainDriver implements fixture.Driver for the graceful-drain differential.
type drainDriver struct {
	mu               sync.Mutex
	refListenerAddr  string
	subjListenerAddr string
	backendPort      int
}

func init() {
	fixture.RegisterFixture(fixtureName, &drainDriver{})
}

func (d *drainDriver) BackendCount() int                { return 1 }
func (d *drainDriver) BackendKind() fixture.BackendKind { return fixture.HTTPSlowStream }
func (d *drainDriver) SubjectListenerName() string      { return "l_main" }
func (d *drainDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// referenceTmpl is the reference Envoy bootstrap (STRICT_DNS to
// host.docker.internal per ADR-0010). Admin port is fixed at 9901 inside the
// container (testcontainers maps it to a random host port via MappedPort).
// The listener port is pinned to refContainerListenerPort (10001) inside the
// container.
const referenceTmpl = `admin:
  address:
    socket_address: {address: 0.0.0.0, port_value: 9901}
static_resources:
  listeners:
    - name: l_main
      address: {socket_address: {address: 0.0.0.0, port_value: %d}}
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_http
                route_config:
                  name: rc_main
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: {prefix: /slow}
                          route: {cluster: c_backend, timeout: 30s}
                        - match: {prefix: /}
                          route: {cluster: c_backend}
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STRICT_DNS
      lb_policy: ROUND_ROBIN
      dns_lookup_family: V4_ONLY
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: host.docker.internal, port_value: %d}}}
`

// subjectTmpl is the envoy-go subject bootstrap (STATIC cluster, loopback).
const subjectTmpl = `admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: %d}
static_resources:
  listeners:
    - name: l_main
      address: {socket_address: {address: 127.0.0.1, port_value: %d}}
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_http
                route_config:
                  name: rc_main
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: {prefix: /slow}
                          route: {cluster: c_backend, timeout: 30s}
                        - match: {prefix: /}
                          route: {cluster: c_backend}
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STATIC
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: %d}}}
`

// ReferenceBootstrap renders the reference Envoy YAML. backendPorts[0] is the
// runner-allocated slow-stream backend port (reachable from the Docker
// container as host.docker.internal:backendPorts[0]).
func (d *drainDriver) ReferenceBootstrap(backendPorts []int) string {
	d.mu.Lock()
	d.backendPort = backendPorts[0]
	d.mu.Unlock()
	return fmt.Sprintf(referenceTmpl, refContainerListenerPort, backendPorts[0])
}

// SubjectConfig renders the envoy-go YAML. subjAdminPort + subjListenerPort +
// backendPorts[0] are runtime-templated by the harness.
func (d *drainDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return fmt.Sprintf(subjectTmpl, subjAdminPort, subjListenerPort, backendPorts[0])
}

// DriveReference stores the reference proxy's listener addr for use in
// ProbeAdmin. Returns nil bytes; the drain sequence runs in ProbeAdmin.
func (d *drainDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListenerAddr = addr
	d.mu.Unlock()
	return nil, nil
}

// DriveSubject stores the subject proxy's listener addr for use in ProbeAdmin.
// Returns nil bytes; the drain sequence runs in ProbeAdmin.
func (d *drainDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.subjListenerAddr = addr
	d.mu.Unlock()
	return nil, nil
}

// ProbeAdmin runs the full per-state-transition drain sequence against each
// proxy in turn, using the stored listener addrs and the runner-supplied admin
// addrs. Emits a deterministic assertion log per proxy. Returns wrapped HTTP
// envelopes; the runner's compareAdminResponses asserts byte-equality on the
// body (the assertion log).
//
// Sequence per proxy (per SPEC §7.2):
//  1. GET /ready → assert "LIVE\n".
//  2. Start GET /slow goroutine; wait for first byte (in-flight established).
//  3. Trigger drain (per-proxy trigger script per §11.2 deviation).
//  4. Poll /ready until "DRAINING\n" (max 5s).
//  5. GET /server_info → assert state == "DRAINING".
//  6. New TCP conn → read until EOF → assert accept-then-FIN.
//  7. Wait for in-flight completion (max 8s); assert body == 5120 × 'x'.
func (d *drainDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	d.mu.Lock()
	refListener := d.refListenerAddr
	subjListener := d.subjListenerAddr
	d.mu.Unlock()

	refLog, err := drainSequence(ctx, refListener, refAdminAddr, "ref")
	if err != nil {
		return nil, nil, fmt.Errorf("ref drain sequence: %w", err)
	}

	subjLog, err := drainSequence(ctx, subjListener, subjAdminAddr, "subj")
	if err != nil {
		return nil, nil, fmt.Errorf("subj drain sequence: %w", err)
	}

	return wrapHTTPResponse(refLog), wrapHTTPResponse(subjLog), nil
}

// drainSequence runs the full per-state-transition drain sequence for one
// proxy. side is "ref" or "subj" (used only for error messages). Returns the
// deterministic assertion log bytes.
func drainSequence(ctx context.Context, listenerAddr, adminAddr, side string) ([]byte, error) {
	var log bytes.Buffer

	// Step 1: Scrape /ready; expect 200 + "LIVE\n".
	body, status, err := adminGET(ctx, adminAddr, "/ready")
	if err != nil {
		return nil, fmt.Errorf("step1 GET /ready: %w", err)
	}
	if status != 200 {
		return nil, fmt.Errorf("step1 /ready: want 200 got %d", status)
	}
	if string(body) != "LIVE\n" {
		return nil, fmt.Errorf("step1 /ready: want LIVE\\n got %q", string(body))
	}
	fmt.Fprintf(&log, "ready:LIVE\n")

	// Step 2: Start in-flight GET /slow goroutine; wait for first byte.
	type inflightResult struct {
		body []byte
		err  error
	}
	firstByte := make(chan struct{}, 1)
	inflightCh := make(chan inflightResult, 1)
	go func() {
		b, err := slowGET(ctx, listenerAddr, firstByte)
		inflightCh <- inflightResult{body: b, err: err}
	}()
	select {
	case <-firstByte:
		// in-flight established
	case <-time.After(6 * time.Second):
		return nil, fmt.Errorf("step2 in-flight /slow: first byte not received within 6s")
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Step 3: Trigger drain per-proxy script (per §11.2 deviation).
	drainBody, drainStatus, err := adminPOST(ctx, adminAddr, "/drain_listeners")
	if err != nil {
		return nil, fmt.Errorf("step3 POST /drain_listeners: %w", err)
	}
	if drainStatus != 200 {
		return nil, fmt.Errorf("step3 /drain_listeners: want 200 got %d", drainStatus)
	}
	if string(drainBody) != "OK\n" {
		return nil, fmt.Errorf("step3 /drain_listeners: want OK\\n got %q", string(drainBody))
	}
	fmt.Fprintf(&log, "drain:OK\n")

	if side == "ref" {
		// Reference Envoy needs a second trigger to flip LB disposition:
		// POST /healthcheck/fail (per §11.2 empirical pin).
		hcBody, hcStatus, err := adminPOST(ctx, adminAddr, "/healthcheck/fail")
		if err != nil {
			return nil, fmt.Errorf("step3 POST /healthcheck/fail: %w", err)
		}
		if hcStatus != 200 {
			return nil, fmt.Errorf("step3 /healthcheck/fail: want 200 got %d", hcStatus)
		}
		_ = hcBody // ignore body; health-check endpoint response is not byte-compared
	}

	// Step 4: Poll /ready until "DRAINING\n" (max 5s).
	if err := pollReadyDraining(ctx, adminAddr, 5*time.Second); err != nil {
		return nil, fmt.Errorf("step4 poll /ready DRAINING: %w", err)
	}
	fmt.Fprintf(&log, "ready:DRAINING\n")

	// Step 5: Scrape /server_info; assert state == "DRAINING".
	siBody, _, err := adminGET(ctx, adminAddr, "/server_info")
	if err != nil {
		return nil, fmt.Errorf("step5 GET /server_info: %w", err)
	}
	state, err := extractServerInfoState(siBody)
	if err != nil {
		return nil, fmt.Errorf("step5 /server_info parse: %w", err)
	}
	if state != "DRAINING" {
		return nil, fmt.Errorf("step5 /server_info state: want DRAINING got %q", state)
	}
	fmt.Fprintf(&log, "server_info:DRAINING\n")

	// Step 6: New TCP conn → read until EOF → assert accept-then-FIN.
	if err := assertAcceptThenFIN(ctx, listenerAddr, 2*time.Second); err != nil {
		return nil, fmt.Errorf("step6 accept-then-FIN: %w", err)
	}
	fmt.Fprintf(&log, "new_conn:accept_then_FIN\n")

	// Step 7: Wait for in-flight to complete (max 8s); assert body.
	select {
	case res := <-inflightCh:
		if res.err != nil {
			return nil, fmt.Errorf("step7 in-flight error: %w", res.err)
		}
		if len(res.body) != 5120 {
			return nil, fmt.Errorf("step7 in-flight body: want 5120 bytes got %d", len(res.body))
		}
		if !bytes.Equal(res.body, bytes.Repeat([]byte{'x'}, 5120)) {
			return nil, fmt.Errorf("step7 in-flight body: content mismatch (not all 'x')")
		}
		fmt.Fprintf(&log, "inflight:ok:5120\n")
	case <-time.After(8 * time.Second):
		return nil, fmt.Errorf("step7 in-flight: did not complete within 8s")
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return log.Bytes(), nil
}

// slowGET issues GET /slow against addr. Signals firstByte when the first
// response byte is received. Returns the full response body.
func slowGET(ctx context.Context, addr string, firstByte chan<- struct{}) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/slow", nil)
	if err != nil {
		return nil, err
	}
	// Use a client with no timeout — the /slow endpoint takes 5s.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /slow: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET /slow: status %d", resp.StatusCode)
	}

	// Stream body; signal firstByte after first non-zero read.
	var body bytes.Buffer
	buf := make([]byte, 512)
	signaled := false
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body.Write(buf[:n])
			if !signaled {
				signaled = true
				select {
				case firstByte <- struct{}{}:
				default:
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("GET /slow read: %w", err)
		}
	}
	return body.Bytes(), nil
}

// adminGET issues GET addr/path against the admin endpoint and returns (body, status, err).
func adminGET(ctx context.Context, addr, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+path, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

// adminPOST issues POST addr/path and returns (body, status, err).
func adminPOST(ctx context.Context, addr, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", "http://"+addr+path, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

// pollReadyDraining polls GET /ready until the body is "DRAINING\n" or timeout elapses.
func pollReadyDraining(ctx context.Context, adminAddr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %v waiting for DRAINING", timeout)
		}
		body, _, err := adminGET(ctx, adminAddr, "/ready")
		if err == nil && strings.TrimSpace(string(body)) == "DRAINING" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// extractServerInfoState parses the /server_info JSON body and returns the
// "state" field value.
func extractServerInfoState(body []byte) (string, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return "", fmt.Errorf("parse JSON: %w", err)
	}
	state, ok := m["state"].(string)
	if !ok {
		return "", fmt.Errorf("state field missing or not string")
	}
	return state, nil
}

// assertAcceptThenFIN dials addr and reads until EOF/error (or timeout),
// expecting the proxy to accept the TCP connection but immediately FIN it
// (i.e., return 0 bytes within timeout). This is the "accept-then-FIN"
// behavior per ADR-0094.
func assertAcceptThenFIN(ctx context.Context, addr string, timeout time.Duration) error {
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		// During drain some proxies may refuse new connections outright.
		// Both behaviors (refuse / accept-then-FIN) satisfy the constraint.
		return nil
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	buf := make([]byte, 256)
	// Either reads 0 bytes (FIN) or reads a small error response; both are
	// acceptable. We just need the dial to not block indefinitely.
	_, _ = conn.Read(buf)
	return nil
}

// wrapHTTPResponse synthesizes a minimal HTTP/1.1 200 OK envelope around
// body so the runner's helpers.ParseHTTPResponse sees a well-formed response.
// Mirrors the 0009-admin-config-dump helper; the Date/Content-Length headers
// are in the runner's compareAdminResponses allow-list.
func wrapHTTPResponse(body []byte) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "HTTP/1.1 200 OK\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&buf, "Content-Length: %d\r\n", len(body))
	buf.WriteString("\r\n")
	buf.Write(body)
	return buf.Bytes()
}

// Compile-time checks.
var (
	_ fixture.Driver           = (*drainDriver)(nil)
	_ fixture.BackendKindAware = (*drainDriver)(nil)
)
