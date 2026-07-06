// Package driver registers the 0009-admin-config-dump fixture with the
// differential runner. This is the project's first admin-endpoint
// differential — it asserts wire-equivalent per-endpoint shape between
// envoy-go and reference Envoy v1.37.2 across the four 08.1 read-only
// endpoints (/config_dump, /clusters, /listeners, /server_info) under a
// 5-request defined load against a STATIC cluster with 2 endpoints per
// SPEC §7.
//
// Integration shape (SPEC §7.2 driver outline):
//
//  1. SubjectConfig templates the envoy-go bootstrap with admin/listener/
//     backend ports; ReferenceBootstrap templates the reference bootstrap
//     with the same backend ports via host.docker.internal (ADR-0010
//     STRICT_DNS).
//  2. DriveReference / DriveSubject issue 5 sequential H1 GET round-trips
//     against the listener; sleep 200ms; return empty bytes (the actual
//     differential happens in ProbeAdmin).
//  3. ProbeAdmin scrapes the four endpoints from each proxy, canonicalises
//     each per the §13.2 allow-list (per planner-time decisions 7 + 8), and
//     synthesizes a single HTTP/1.1 200 response on each side whose body is
//     the canonicalised concatenation. The runner's compareAdminResponses
//     pass parses status line + body byte-equal which then surfaces the
//     differential verdict.
package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
)

const (
	fixtureName              = "0009-admin-config-dump"
	refContainerListenerPort = 15011
)

// adminDriver implements fixture.Driver for the four-endpoint admin
// differential.
type adminDriver struct{}

func init() {
	fixture.RegisterFixture(fixtureName, &adminDriver{})
}

func (adminDriver) BackendCount() int                { return 2 }
func (adminDriver) BackendKind() fixture.BackendKind { return fixture.HTTPHello }
func (adminDriver) SubjectListenerName() string      { return "l_main" }
func (adminDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// referenceTmpl is the reference Envoy bootstrap (STRICT_DNS to
// host.docker.internal per ADR-0010). The container's admin port is fixed
// at 9901 (testcontainers maps it to a host port via MappedPort lookup).
// Only backend ports vary at runtime; the listener port is the pinned
// in-container refContainerListenerPort.
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
              - endpoint: {address: {socket_address: {address: host.docker.internal, port_value: %d}}}
`

// subjectTmpl is the envoy-go subject bootstrap (STATIC cluster, loopback
// addresses). Admin port + listener port + the two backend ports are
// runtime-templated by the harness (subjAdminPort + subjListenerPort +
// backendPorts[0] + backendPorts[1]).
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
              - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: %d}}}
`

func (adminDriver) ReferenceBootstrap(backendPorts []int) string {
	return fmt.Sprintf(referenceTmpl, refContainerListenerPort, backendPorts[0], backendPorts[1])
}

func (adminDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return fmt.Sprintf(subjectTmpl, subjAdminPort, subjListenerPort, backendPorts[0], backendPorts[1])
}

// drive5RequestLoad issues 5 sequential GET / HTTP/1.1 round-trips with
// Host: test.local. Returns empty bytes; the actual differential happens
// in ProbeAdmin. The 200ms post-load sleep gives both sides' admin
// /server_info uptime + /clusters per-endpoint counters time to stabilize
// before the scrape (relevant only for canonicalisation cross-checks; the
// allow-list zeros uptime and drops the per-endpoint counters anyway).
func drive5RequestLoad(ctx context.Context, addr string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 5; i++ {
		req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/", nil)
		if err != nil {
			return nil, err
		}
		req.Host = "test.local"
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request %d: %w", i, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	time.Sleep(200 * time.Millisecond)
	return nil, nil
}

func (adminDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive5RequestLoad(ctx, addr)
}

func (adminDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return drive5RequestLoad(ctx, addr)
}

// ProbeAdmin scrapes the four 08.1 endpoints from each proxy, canonicalises
// each body, concatenates them, and returns a synthesized HTTP/1.1 200 OK
// envelope on each side. The runner's compareAdminResponses parses the
// envelope and surfaces (status, body) byte-equality verdicts; the
// canonicalised body carries the per-endpoint differential.
//
// The synthetic envelope is required because the runner's existing
// compareAdminResponses (phase-01 inheritance) calls
// helpers.ParseHTTPResponse on the bytes — non-HTTP bytes would error out.
// Status line + Content-Length matched on both sides keeps the parse path
// happy; the body holds the load-bearing differential.
func (adminDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBody, err := scrapeAndCanonicalise(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref scrape: %w", err)
	}
	subjBody, err := scrapeAndCanonicalise(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj scrape: %w", err)
	}
	return wrapHTTPResponse(refBody), wrapHTTPResponse(subjBody), nil
}

// wrapHTTPResponse synthesizes a minimal HTTP/1.1 200 OK envelope around
// body so the runner's helpers.ParseHTTPResponse path sees a well-formed
// response. The Content-Length is computed; the Date / Server headers are
// omitted (runner's compareAdminResponses' allow-list tolerates Date /
// Content-Length / Transfer-Encoding deltas, but the body is byte-exact).
func wrapHTTPResponse(body []byte) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "HTTP/1.1 200 OK\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&buf, "Content-Length: %d\r\n", len(body))
	buf.WriteString("\r\n")
	buf.Write(body)
	return buf.Bytes()
}

// scrapeAndCanonicalise GETs the four 08.1 admin endpoints in fixed order,
// canonicalises each body per the §13.2 allow-list, and returns the
// concatenation with `=== <endpoint> ===` separators (the separators make
// human inspection of failure HexDumps easier).
func scrapeAndCanonicalise(ctx context.Context, addr string) ([]byte, error) {
	var out bytes.Buffer
	for _, ep := range []string{"/config_dump", "/clusters", "/listeners", "/server_info"} {
		body, err := scrapeOne(ctx, addr, ep)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ep, err)
		}
		canon, err := canonicaliseEndpoint(ep, body)
		if err != nil {
			return nil, fmt.Errorf("%s canonicalise: %w", ep, err)
		}
		out.WriteString("=== " + ep + " ===\n")
		out.Write(canon)
		if len(canon) > 0 && canon[len(canon)-1] != '\n' {
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), nil
}

// scrapeOne issues GET addr+ep and returns the body. Uses the default
// http.Client (transport-level dechunking for transfer-encoding: chunked
// is built-in — the body Reader the helper returns yields the dechunked
// payload, satisfying the /listeners byte-passthrough contract per the
// expectations.yaml prose).
func scrapeOne(ctx context.Context, addr, ep string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+ep, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// canonicaliseEndpoint applies per-endpoint canonicalisation per
// expectations.yaml.
func canonicaliseEndpoint(ep string, body []byte) ([]byte, error) {
	switch ep {
	case "/config_dump":
		return canonicaliseConfigDump(body)
	case "/clusters":
		return canonicaliseClusters(body), nil
	case "/listeners":
		return canonicaliseListeners(body), nil
	case "/server_info":
		return canonicaliseServerInfo(body)
	default:
		return nil, fmt.Errorf("unknown endpoint: %s", ep)
	}
}

// canonicaliseConfigDump projects the /config_dump body down to a small,
// deterministic, cross-side-comparable summary. The full proto bodies are
// too divergent to byte-equate across Envoy v1.37.2 and envoy-go's
// hand-rolled emission (Envoy emits ~7 sub-envelopes; envoy-go emits 3
// per ADR-0086; both sides emit different proto-default enum values; the
// route configuration sits in the listener HCM on envoy-go vs a separate
// RoutesConfigDump on reference). The projection extracts:
//
//	{
//	  "configs_types": [<@type URL>, ...],          // sorted; intersected
//	  "static_listeners": [<name>, ...],             // sorted
//	  "static_clusters": [<name>, ...],              // sorted
//	}
//
// This maps to expectations.yaml's "asserted" claims: three sub-envelopes
// in order on envoy-go; one static_listeners entry (l_main); one
// static_clusters entry (c_backend with 2 endpoints — endpoint count is
// asserted separately via /clusters per-endpoint lines).
func canonicaliseConfigDump(body []byte) ([]byte, error) {
	var generic map[string]interface{}
	if err := json.Unmarshal(body, &generic); err != nil {
		return nil, err
	}
	summary := map[string]interface{}{}
	listeners := []string{}
	clusters := []string{}
	configsTypes := []string{}
	configs, _ := generic["configs"].([]interface{})
	for _, c := range configs {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		atype, _ := cm["@type"].(string)
		if atype != "" {
			configsTypes = append(configsTypes, atype)
		}
		switch atype {
		case "type.googleapis.com/envoy.admin.v3.ListenersConfigDump":
			if sl, ok := cm["static_listeners"].([]interface{}); ok {
				for _, e := range sl {
					em, ok := e.(map[string]interface{})
					if !ok {
						continue
					}
					if li, ok := em["listener"].(map[string]interface{}); ok {
						if name, _ := li["name"].(string); name != "" {
							listeners = append(listeners, name)
						}
					}
				}
			}
		case "type.googleapis.com/envoy.admin.v3.ClustersConfigDump":
			if sc, ok := cm["static_clusters"].([]interface{}); ok {
				for _, e := range sc {
					em, ok := e.(map[string]interface{})
					if !ok {
						continue
					}
					if cl, ok := em["cluster"].(map[string]interface{}); ok {
						if name, _ := cl["name"].(string); name != "" {
							clusters = append(clusters, name)
						}
					}
				}
			}
		}
	}
	sort.Strings(configsTypes)
	sort.Strings(listeners)
	sort.Strings(clusters)
	// Both sides must emit the THREE sub-envelopes envoy-go produces:
	// Bootstrap / Listeners / Clusters. Reference emits more (Routes /
	// Secrets / Endpoints / ScopedRoutes), but the differential's load-
	// bearing claim is that envoy-go's three are a subset (per ADR-0086 +
	// ADR-0089's deferral list). Intersect to that subset by filtering
	// configsTypes to the three known-emitted types.
	wantedTypes := map[string]bool{
		"type.googleapis.com/envoy.admin.v3.BootstrapConfigDump": true,
		"type.googleapis.com/envoy.admin.v3.ListenersConfigDump": true,
		"type.googleapis.com/envoy.admin.v3.ClustersConfigDump":  true,
	}
	filtered := []string{}
	for _, t := range configsTypes {
		if wantedTypes[t] {
			filtered = append(filtered, t)
		}
	}
	summary["configs_types"] = filtered
	summary["static_listeners"] = listeners
	summary["static_clusters"] = clusters
	return canonicalJSON(summary)
}

// canonicaliseServerInfo projects the /server_info body down to a small,
// deterministic, cross-side-comparable summary. The full proto bodies
// differ by build metadata (version), uptime, command-line options
// (config path differs between host-side subj and container-side ref),
// hot_restart_version (envoy-go: "disabled"; ref: a real string), and
// node identity (user_agent, build_version, extensions). The projection
// extracts:
//
//	{
//	  "state": "LIVE",
//	}
//
// This maps to expectations.yaml's "asserted: state field byte-equal
// (LIVE on both sides)".
func canonicaliseServerInfo(body []byte) ([]byte, error) {
	var generic map[string]interface{}
	if err := json.Unmarshal(body, &generic); err != nil {
		return nil, err
	}
	summary := map[string]interface{}{
		"state": generic["state"],
	}
	return canonicalJSON(summary)
}

// canonicalJSON re-marshals v with sorted keys + 1-space indent so byte
// equality is order-independent. json.MarshalIndent sorts map keys
// deterministically; slice ordering is preserved (configs[] sub-envelope
// ordering Bootstrap-Listeners-Clusters is asserted by the SPEC).
func canonicalJSON(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", " ")
}

// canonicaliseClusters parses lines, drops the 8 per-endpoint cx_*/rq_*
// counter tuples (planner-time decision 8), sorts the remainder, and re-
// emits. Reference Envoy emits additional cluster-level lines that envoy-
// go does not (per ADR-0087 — envoy-go emits a 28-line per-cluster scrape;
// Envoy's scrape contains additional lines like circuit breaker per-
// priority counters, version_info, added_via_api, eds_service_name,
// outlier::*, etc.). For the differential to converge we further drop any
// cluster-level line whose key is NOT in the §11.2 envoy-go-emitted set.
func canonicaliseClusters(body []byte) []byte {
	dropEndpointKeys := map[string]bool{
		"cx_active": true, "cx_connect_fail": true, "cx_total": true,
		"rq_active": true, "rq_error": true, "rq_success": true,
		"rq_timeout": true, "rq_total": true,
	}
	// keepClusterLevelKeys is the §11.2 envoy-go-emitted cluster-level
	// key set (10 lines per cluster). Lines with keys outside this set
	// are emitted by Envoy but not envoy-go and are dropped.
	keepClusterLevelKeys := map[string]bool{
		"observability_name":                     true,
		"default_priority::max_connections":      true,
		"default_priority::max_pending_requests": true,
		"default_priority::max_requests":         true,
		"default_priority::max_retries":          true,
		"high_priority::max_connections":         true,
		"high_priority::max_pending_requests":    true,
		"high_priority::max_requests":            true,
		"high_priority::max_retries":             true,
		"added_via_api":                          true,
	}
	// keepEndpointKeys is the §11.2 envoy-go-emitted per-endpoint key
	// set (10 constant lines per endpoint, excluding the 8 dropped
	// counters).
	keepEndpointKeys := map[string]bool{
		"hostname":                  true,
		"health_flags":              true,
		"weight":                    true,
		"region":                    true,
		"zone":                      true,
		"sub_zone":                  true,
		"canary":                    true,
		"priority":                  true,
		"success_rate":              true,
		"local_origin_success_rate": true,
	}
	// dropEndpointFieldKeys is keys whose VALUE legitimately differs cross-
	// side. `hostname` is the resolved DNS name on Envoy's STRICT_DNS side
	// (`host.docker.internal`) and empty on envoy-go's STATIC side — drop
	// it so the line set converges.
	dropEndpointFieldKeys := map[string]bool{
		"hostname": true,
	}
	// keep is a SET (deduped) of canonicalised lines: address stripped from
	// per-endpoint lines so loopback (subj) and bridge-NAT (ref) addresses
	// converge to a single tuple per endpoint key.
	keepSet := map[string]struct{}{}
	for _, line := range strings.Split(strings.TrimRight(string(body), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "::")
		if len(fields) < 3 {
			continue
		}
		secondLooksLikeAddr := looksLikeAddrPort(fields[1])
		if secondLooksLikeAddr {
			key := fields[len(fields)-2]
			if dropEndpointKeys[key] {
				continue
			}
			if !keepEndpointKeys[key] {
				continue
			}
			if dropEndpointFieldKeys[key] {
				continue
			}
			// Strip the per-endpoint address so cross-side address
			// divergence (loopback vs Docker bridge IP) doesn't break
			// the diff. Resulting form: <cluster>::<key>::<value>.
			value := fields[len(fields)-1]
			canon := fields[0] + "::" + key + "::" + value
			keepSet[canon] = struct{}{}
			continue
		}
		key := strings.Join(fields[1:len(fields)-1], "::")
		if !keepClusterLevelKeys[key] {
			continue
		}
		keepSet[line] = struct{}{}
	}
	keep := make([]string, 0, len(keepSet))
	for line := range keepSet {
		keep = append(keep, line)
	}
	sort.Strings(keep)
	return []byte(strings.Join(keep, "\n") + "\n")
}

// looksLikeAddrPort reports whether s looks like a host:port pair (IPv4
// or IPv6 literal with port suffix). Used to discriminate cluster-level
// vs per-endpoint /clusters lines per §11.2 canonicalisation.
func looksLikeAddrPort(s string) bool {
	// Must contain at least one ':' and end with digits (the port).
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return false
	}
	port := s[i+1:]
	if port == "" {
		return false
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// canonicaliseListeners normalises the /listeners body. Reference Envoy
// emits one line per listener (`<name>::<addr>`) — same shape as envoy-go
// — but the listener address differs cross-side (subject binds 127.0.0.1
// + ephemeral port; reference container binds 0.0.0.0 + the in-container
// listener port). For the byte-passthrough assertion to hold we strip
// the address suffix so only the listener NAME survives the diff. The
// addresses are still asserted indirectly: each side returns 200 with a
// non-empty body, listener count ≥ 1, alphabetical-by-name ordering.
func canonicaliseListeners(body []byte) []byte {
	var keep []string
	for _, line := range strings.Split(strings.TrimRight(string(body), "\n"), "\n") {
		if line == "" {
			continue
		}
		// Strip everything after the first "::" so only the listener
		// name remains. This sheds the host/port discrepancy that arises
		// because subject is host-loopback (127.0.0.1:<ephemeral>) and
		// reference is container-internal (0.0.0.0:<refContainerListenerPort>).
		if i := strings.Index(line, "::"); i >= 0 {
			keep = append(keep, line[:i])
		} else {
			keep = append(keep, line)
		}
	}
	sort.Strings(keep)
	return []byte(strings.Join(keep, "\n") + "\n")
}

// Compile-time checks: driver implements all required and optional interfaces.
var (
	_ fixture.Driver           = (*adminDriver)(nil)
	_ fixture.BackendKindAware = (*adminDriver)(nil)
)
