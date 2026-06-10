// Package inputs registers the 0032-http-ratelimit fixture with the
// differential runner per phase-24.1 SPEC §7 + PLAN Task 10 (24.1) +
// PLAN Task 6 (24.2 — scenarios (f) + (g) + (d) extension).
//
// # Fixture type
//
// CROSS-SIDE (default RequiresReference=true): the runner spawns reference
// Envoy v1.37.2 + envoy-go, drives both sides via DriveReference /
// DriveSubject, and CompareBytes on the resulting per-scenario byte streams
// is the differential gate. Cross-side byte-exactness on the OVER_LIMIT
// scenario (c) + (g) is load-bearing for the AMEND-6 proto-number-faithful
// fake invariant + the AMEND-8 X-RateLimit byte-pin (24.2).
//
// # 9 scenarios (24.1 + 24.2 combined scope per parent SPEC §7.1)
//
//	(a) parse_ok           — GET /scenario_a → zero-descriptor short-circuit → echo 200.
//	(b) ok_admit           — GET /scenario_b → RLS OK → echo 200.
//	(c) over_limit_429     — GET /scenario_c → RLS OVER_LIMIT → 429 empty body.
//	(d) descriptor_actions — GET /scenario_d → 9-action chain (24.2 Task 6
//	                          extension to all 10 framework-reachable actions
//	                          minus destination_cluster) → RLS OK → echo 200.
//	(e) failure_mode_open  — GET /scenario_e (RLS stopped) → fail-open → echo 200.
//	(f) vh_inclusion       — 24.2 Task 6: GET /scenario_f_{override,include,ignore}
//	                          on vh_a (the sole virtual_host — envoy-go's HCM
//	                          enforces exactly-1 vh per config.go:227-233; all
//	                          (f) sub-routes use the default Host). Three
//	                          sub-scenarios exercising the §4.3 Axis-B table
//	                          (OVERRIDE default / INCLUDE per-route / IGNORE
//	                          per-route) via per-route TPFC `RateLimitPerRoute`.
//	                          All three return RLS OK → echo 200. Cross-side
//	                          divergence in the descriptor set surfaces as a
//	                          200 vs 200 ⇒ byte-equality holds either way; the
//	                          stronger detection lives in the unit tests
//	                          landed at Task 4.
//	(g) x_ratelimit_headers — 24.2 Task 6: GET /scenario_g → 2-descriptor
//	                          policy. The fake scripts OVER_LIMIT with
//	                          per-descriptor CurrentLimit / LimitRemaining /
//	                          DurationUntilReset populated (different Units —
//	                          SECOND and MINUTE — exercising the unitToSeconds
//	                          map). Filter-level enable_x_ratelimit_headers:
//	                          DRAFT_VERSION_03 triggers X-RateLimit emission.
//	                          The driver byte-pins the x-ratelimit-* triple
//	                          into the cross-side stream — both sides MUST
//	                          emit byte-identical values per the AMEND-8 +
//	                          §4.7 line 214 wire-order discipline (Task 5
//	                          follow-up).
//	(h) stat_surface       — OBSERVATIONAL: AssertStats on subject counters
//	                          accumulated by the preceding probes.
//
// # Assertion wiring
//
//   - (a)/(b)/(c)/(d)/(e)/(f)/(g) — CompareBytes on the per-scenario
//     status+body-classification line (the runner's cross-side byte gate);
//     scenario (g) additionally emits the X-RateLimit triple into the byte
//     stream for byte-pin.
//   - (h) — StatsAsserter.AssertStats on the SUBJECT admin /stats/prometheus.
//     NOT SubjectAsserter (per reference_differential_asserter_dispatch the
//     runner's runFixture dispatch only invokes SubjectAsserter on the
//     reference-less path; this fixture is cross-side ⇒ subject-side
//     assertions live in StatsAsserter).
//
// # Single-listener topology (parent SPEC §7.3)
//
// One listener (l_test_a) with the ratelimit filter + router terminator.
// Single virtual_host vh_a (domains: ["*"]) carries ALL scenarios — envoy-go's
// HCM enforces exactly-1 virtual_host per config.go:227-233 (single-vhost
// canonical predating phase 24 per ADR-0019/0072). Scenarios (a)/(b)/(c)/
// (d)/(e)/(g) live on dedicated routes; (f1)/(f2)/(f3) live on sub-routes
// /scenario_f_{override,include,ignore} with per-route TPFC
// `RateLimitPerRoute` exercising the §4.3 Axis-B OVERRIDE / INCLUDE / IGNORE
// arms. The vhost-level rate_limits on vh_a emits generic_key{vh:vh_a},
// walked conditionally per the §4.3 Axis-B table: SKIPPED by 24.1 routes
// b/c/d/e/g via the OVERRIDE-default vh-SKIP arm (route non-empty wins),
// SKIPPED by (a) via explicit vh_rate_limits=IGNORE TPFC (preserves 24.1
// zero-descriptor semantics), and INCLUDED only on the f-include sub-route
// (yielding the 2-descriptor `scenario=f_include|vh=vh_a` key). No
// multi-listener — avoids the freeTCPPort combined-run flake per 22.2
// REVIEW §7.4.
//
// # Fake RLS lifecycle
//
// The shared in-process gRPC RateLimitService fake (test/helpers/ratelimitgrpc/)
// is allocated a free 127.0.0.1:<port> at driver instantiation (so both YAMLs
// can templatize the cluster endpoint deterministically before either proxy
// starts). The fake is started fresh ONCE at the beginning of each driveProxy
// run with all per-scenario scripts pre-populated, stopped ONCE before
// scenario (e) which requires dial failure (fail-open admit), and never
// restarted for the remainder of the run. The driver's scripts cover every
// CanonicalKey the engine is expected to emit — per the Task 9 advisory (the
// fake returns default-OK on no-match; an unscripted key would silently pass
// through OK and mask the assertion).
//
// # Reference container ports
//
//	refAdminPort  = 9901 (standard; harness waits on this port)
//	refLATestPort = 10032 (l_test_a in the reference container)
//
// # Cross-references
//
//   - parent SPEC §4.1/§4.5/§4.6/§4.7 (descriptor engine + dispositions + byte-shape)
//   - parent SPEC §4.3 (Axis-B vh_rate_limits table — scenario (f))
//   - parent SPEC §7.1 (8-scenario matrix; 24.2 adds (f) + (g))
//   - parent SPEC §7.3 (single-listener topology)
//   - 24.1 SPEC §7 (24.1 scope scoping: a/b/c/d-core/e/h)
//   - 24.2 SPEC §3 (24.2 scope additions: (f)/(g)/(d)-extension)
//   - AMEND-1/3/6/8/10/11 (cluster-scoped stats; defaults; fake-encoding;
//     header order; cross-namespace; per-action key defaults)
//   - ADR-0010 / ADR-0166 (host.docker.internal + plaintext h2c)
//   - ADR-0197 (filter shape + dispositions — CORE slice + X-RateLimit amend)
//   - ADR-0198 (route-table accessor pair — 24.1 Task 5)
//   - ADR-0199 (RateLimitPerRoute 10th canonical — 24.2 Task 3)
//   - ADR-0200 (route-level PARSE-REJECTs — 24.1 Task 3)
//   - reference_differential_asserter_dispatch
//   - 18.2 fixture-0021 (template precedent — fixed pre-allocated port +
//     host.docker.internal/127.0.0.1 templating + NewAtAddr fake-server arm)
//   - 23 fixture-0030 (StatsAsserter + statValue/requireStatIs* helpers)
package inputs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	durationpb "google.golang.org/protobuf/types/known/durationpb"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
	"github.com/esalaine/envoy-go/test/helpers/ratelimitgrpc"
)

const (
	fixtureName = "0032-http-ratelimit"

	// Reference-container in-container listener / admin ports. The harness
	// exposes these via testcontainers MappedPort.
	refAdminPort  = 9901
	refLATestPort = 10032

	// rlsDomain is the filter-config `domain` field; load-bearing for the
	// fake's CanonicalKey lookup (the fake key is built as
	// `domain | desc[0] | desc[1] ...`).
	rlsDomain = "domain_b"

	// rlsClusterName is the upstream RLS cluster name (matches envoy.yaml
	// + envoy-go.yaml). Used for the StatsAsserter Prometheus label query.
	rlsClusterName = "c_ratelimit"

	// nodeServiceCluster is the bootstrap `node.cluster` field (matches
	// envoy.yaml + envoy-go.yaml). Consumed by the (d) extension
	// source_cluster action to emit (source_cluster, rls_test_cluster).
	nodeServiceCluster = "rls_test_cluster"

	// tenantHeader / canaryHeader are the per-request discriminators for
	// scenario (d). Their values populate the descriptor entries the engine
	// emits; the fake script keyed on those exact values returns OK.
	tenantHeader = "x-tenant"
	tenantValue  = "tenant-x"
	canaryHeader = "x-canary"

	// loopbackIP is the value the remote_address action emits when the
	// downstream peer is the host loopback. Both reference (dockerized,
	// reaching envoy via the docker bridge) and envoy-go (loopback) see
	// "127.0.0.1" as the downstream remote_address per the ADR-0165
	// set-once-by-dispatch accessor. (Reference Envoy's downstream peer
	// from the host's perspective is the docker-bridge gateway, but the
	// reference container's view of the downstream is its localhost-mapped
	// port — both reduce to 127.0.0.1 at the descriptor.)
	loopbackIP = "127.0.0.1"

	// maskedLoopbackIP is the masked downstream IP for the (d) extension
	// masked_remote_address action with v4_prefix_mask_len=24. 127.0.0.1
	// masked with /24 = 127.0.0.0/24.
	maskedLoopbackIP = "127.0.0.0/24"

	// vhDescValue is the descriptor value emitted by vh_a's vhost-level
	// generic_key{vh:vh_a} rate_limit policy (consumed by scenario f-include).
	// 24.1 routes (b/c/d/e) + (g) use OVERRIDE-default vhost-SKIP arm (route
	// non-empty wins); scenario (a) uses an explicit vh_rate_limits=IGNORE
	// TPFC to skip the vh policy and preserve the 24.1 zero-descriptor
	// semantics.
	vhDescValue = "vh_a"
)

func init() {
	fixture.RegisterFixture(fixtureName, &rlDriver{})
}

// rlDriver carries per-driver lifecycle state — the pre-allocated RLS fake
// port + the running fake handle (toggled across scenarios that require a
// dial failure).
type rlDriver struct {
	mu sync.Mutex

	// rlsPort is the pre-allocated port the RLS fake binds to (on 0.0.0.0);
	// shared between ReferenceBootstrap and SubjectConfig so both YAMLs
	// templatize the SAME cluster endpoint deterministically before either
	// proxy starts. Allocated lazily on first use (whichever of
	// ReferenceBootstrap / SubjectConfig fires first).
	rlsPort int

	// rlsSrv is the currently-running fake. nil before driveProxy starts it
	// and between stopRLS / setupRLS toggles. Lifecycle managed inside
	// driveProxy (Setup-at-start, toggle around e + h's fail-open arm,
	// teardown at end).
	rlsSrv *ratelimitgrpc.Server
}

// --- lazy fake-port allocation ---

// allocateRLSPort allocates a free TCP port for the RLS fake. Called lazily
// by ReferenceBootstrap / SubjectConfig (whichever fires first). Idempotent —
// returns the same port on subsequent calls. Does NOT start the server; the
// server is started fresh at the beginning of each driveProxy call.
func (d *rlDriver) allocateRLSPort() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rlsPort != 0 {
		return d.rlsPort
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate rls port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	d.rlsPort = port
	return port
}

// setupRLS starts the in-process fake bound to the pre-allocated port,
// pre-populates the per-scenario scripts, and stores the handle on the
// driver. Idempotent — an already-running fake is stopped first.
//
// Scripts (all under domain_b):
//
//	scenario=b                                                                           → OK
//	scenario=c                                                                           → OVER_LIMIT
//	scenario=d;tenant=tenant-x;remote_address=127.0.0.1;header_match=canaried;
//	source_cluster=rls_test_cluster;masked_remote_address=127.0.0.0/24;tier=gold;
//	region=us-east;query_match=premium_plan                                              → OK
//	scenario=e                                                                           → OK (defensive)
//	scenario=f_override                                                                  → OK
//	scenario=f_include|vh=vh_a                                                           → OK (2-descriptor key)
//	scenario=f_ignore                                                                    → OK
//	tier=bronze|scope=burst                                                              → OVER_LIMIT (multi-descriptor
//	                                                                                       with per-descriptor CurrentLimit /
//	                                                                                       LimitRemaining / DurationUntilReset
//	                                                                                       for X-RateLimit emission)
//
// Per the Task 9 advisory every expected canonical key is EXPLICITLY scripted
// so a default-OK on no-match cannot silently satisfy the test. The (a)
// scenario does NOT script anything (the engine emits zero descriptors;
// ShouldRateLimit never fires).
func (d *rlDriver) setupRLS() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rlsSrv != nil {
		d.rlsSrv.Stop()
		d.rlsSrv = nil
	}
	if d.rlsPort == 0 {
		return fmt.Errorf("driver: setupRLS called before rlsPort allocation")
	}
	// Bind all interfaces so the reference Envoy container can reach the
	// service via host.docker.internal (bridge gateway) on plain Linux Docker;
	// loopback-only binds are unreachable from containers outside Docker Desktop.
	addr := fmt.Sprintf("0.0.0.0:%d", d.rlsPort)
	srv, err := ratelimitgrpc.NewAtAddr(addr)
	if err != nil {
		return fmt.Errorf("driver: start rls fake on %s: %w", addr, err)
	}
	d.rlsSrv = srv

	// Script (b) — single-entry descriptor → OK.
	srv.Script(canonicalKeyFor1([][2]string{{"scenario", "b"}}),
		respOKForDescriptors(1))

	// Script (c) — single-entry descriptor → OVER_LIMIT.
	// AMEND-6 / D-RL5: construct the response with ONLY the fields the
	// scenario emits — OverallCode + per-descriptor OVER_LIMIT statuses. No
	// RawBody, no Quota, no per-descriptor optionals (current_limit /
	// limit_remaining / duration_until_reset / quota). Go-protobuf's
	// zero-value/nil omission keeps the wire bytes byte-equivalent to the
	// reference Envoy v1.37.2 emit shape.
	srv.Script(canonicalKeyFor1([][2]string{{"scenario", "c"}}),
		respOverLimitForDescriptors(1))

	// Script (d) — 9-entry descriptor (24.2 Task 6 extension to all 10
	// framework-reachable actions minus destination_cluster). AMEND-6
	// entries-in-action-list order:
	//   generic_key → request_headers → remote_address → header_value_match
	//   (24.1 core) → source_cluster → masked_remote_address → metadata →
	//   query_parameters → query_parameter_value_match (24.2 Task 6).
	srv.Script(canonicalKeyFor1([][2]string{
		{"scenario", "d"},
		{"tenant", tenantValue},
		{"remote_address", loopbackIP},
		{"header_match", "canaried"},
		{"source_cluster", nodeServiceCluster},
		{"masked_remote_address", maskedLoopbackIP},
		{"tier", "gold"},
		{"region", "us-east"},
		{"query_match", "premium_plan"},
	}), respOKForDescriptors(1))

	// Script (e) — defensive (the fake is STOPPED before the request so the
	// dial fails first; the script ensures a regression where the fake stays
	// up surfaces as a counter mismatch in (h) rather than as a silent pass).
	srv.Script(canonicalKeyFor1([][2]string{{"scenario", "e"}}),
		respOKForDescriptors(1))

	// Script (f-override) — Phase 24.2 Task 6 §4.3 OVERRIDE-default arm.
	// Route has rate_limits → vhost SKIPPED → ONLY route descriptor emitted.
	srv.Script(canonicalKeyFor1([][2]string{{"scenario", "f_override"}}),
		respOKForDescriptors(1))

	// Script (f-include) — Phase 24.2 Task 6 §4.3 INCLUDE arm. BOTH route
	// AND vhost descriptors emitted (route first per AMEND-6 walk order).
	// The CanonicalKey has TWO descriptor segments separated by '|':
	//   domain_b | scenario=f_include | vh=vh_a
	srv.Script(canonicalKeyForMulti([][][2]string{
		{{"scenario", "f_include"}},
		{{"vh", vhDescValue}},
	}), respOKForDescriptors(2))

	// Script (f-ignore) — Phase 24.2 Task 6 §4.3 IGNORE arm. Vhost SKIPPED
	// unconditionally → ONLY route descriptor emitted.
	srv.Script(canonicalKeyFor1([][2]string{{"scenario", "f_ignore"}}),
		respOKForDescriptors(1))

	// Script (g) — Phase 24.2 Task 6 X-RateLimit byte-pin. TWO descriptors
	// (one entry each — generic_key{tier:bronze} + generic_key{scope:burst}).
	// The fake returns OVER_LIMIT with per-descriptor CurrentLimit /
	// LimitRemaining / DurationUntilReset populated. Different Units
	// (SECOND vs MINUTE) exercise the unitToSeconds map. MIN-selection picks
	// the SECOND status (LimitRemaining=2 < 7) per headers.go strict-`<`
	// comparison. The quota-policy suffix appends segments for BOTH
	// descriptors (the MIN's own descriptor included per upstream).
	srv.Script(canonicalKeyForMulti([][][2]string{
		{{"tier", "bronze"}},
		{{"scope", "burst"}},
	}), respOverLimitWithStatuses([]xrlStatus{
		{requestsPerUnit: 10, unit: ratelimitv3.RateLimitResponse_RateLimit_SECOND, limitRemaining: 2, durationSeconds: 1},
		{requestsPerUnit: 100, unit: ratelimitv3.RateLimitResponse_RateLimit_MINUTE, limitRemaining: 7, durationSeconds: 60},
	}))

	if os.Getenv("FIXTURE_0032_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[driver] scripted keys: scenario=b, scenario=c, "+
			"scenario=d;tenant=tenant-x;remote_address=127.0.0.1;header_match=canaried;"+
			"source_cluster=rls_test_cluster;masked_remote_address=127.0.0.0/24;tier=gold;"+
			"region=us-east;query_match=premium_plan, scenario=e, scenario=f_override, "+
			"scenario=f_include|vh=vh_a, scenario=f_ignore, tier=bronze|scope=burst "+
			"(all under domain %q)\n", rlsDomain)
	}
	return nil
}

// stopRLS stops the running fake. Idempotent. Called once before scenario (e)
// (which requires dial failure → fail-open admit) and never restarted within
// the same driveProxy run.
func (d *rlDriver) stopRLS() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rlsSrv != nil {
		d.rlsSrv.Stop()
		d.rlsSrv = nil
	}
}

// --- script-key helpers ---

// canonicalKeyFor1 builds the CanonicalKey for a SINGLE descriptor with the
// supplied entries (in order). Mirrors the format the descriptor engine emits
// at runtime + the fake reads at ShouldRateLimit time. Per the
// ratelimitgrpc.CanonicalKey contract:
//
//	domain "|" descriptor[0]
//	where descriptor[0] = "key=value;key=value;..." in entry order.
//
// Single-descriptor requests are the 24.1 baseline (scenarios b/c/d/e all
// produce one descriptor); 24.2 (f-include) + (g) produce 2-descriptor
// requests via canonicalKeyForMulti.
func canonicalKeyFor1(entries [][2]string) string {
	return rlsDomain + "|" + joinEntries(entries)
}

// canonicalKeyForMulti builds the CanonicalKey for a multi-descriptor request:
//
//	domain "|" descriptor[0] "|" descriptor[1] ...
//
// Each descriptor[i] is its entries joined as "key=value;key=value;..." in
// entry order. Used by scenario (f-include) (2 descriptors: route + vhost)
// and scenario (g) (2 descriptors: two single-action policies).
func canonicalKeyForMulti(descriptors [][][2]string) string {
	parts := make([]string, 0, 1+len(descriptors))
	parts = append(parts, rlsDomain)
	for _, d := range descriptors {
		parts = append(parts, joinEntries(d))
	}
	return strings.Join(parts, "|")
}

// joinEntries joins the supplied (key,value) entries with ";" — the entries-
// within-descriptor separator per the ratelimitgrpc.CanonicalKey contract.
func joinEntries(entries [][2]string) string {
	segs := make([]string, len(entries))
	for i, e := range entries {
		segs[i] = e[0] + "=" + e[1]
	}
	return strings.Join(segs, ";")
}

// respOKForDescriptors builds an OK RateLimitResponse with per-descriptor OK
// statuses (n entries; matches the request's descriptor count). AMEND-6: only
// OverallCode + Statuses[i].Code are set; all other optionals (RawBody,
// DynamicMetadata, Quota, per-descriptor CurrentLimit / LimitRemaining /
// DurationUntilReset / Quota) are zero-value / nil and elided by Go-protobuf.
func respOKForDescriptors(n int) *ratelimitv3.RateLimitResponse {
	statuses := make([]*ratelimitv3.RateLimitResponse_DescriptorStatus, n)
	for i := range statuses {
		statuses[i] = &ratelimitv3.RateLimitResponse_DescriptorStatus{
			Code: ratelimitv3.RateLimitResponse_OK,
		}
	}
	return &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OK,
		Statuses:    statuses,
	}
}

// respOverLimitForDescriptors builds an OVER_LIMIT RateLimitResponse with
// per-descriptor OVER_LIMIT statuses. AMEND-6: only OverallCode + Statuses[i].Code
// are set; no RawBody, no per-descriptor optionals.
func respOverLimitForDescriptors(n int) *ratelimitv3.RateLimitResponse {
	statuses := make([]*ratelimitv3.RateLimitResponse_DescriptorStatus, n)
	for i := range statuses {
		statuses[i] = &ratelimitv3.RateLimitResponse_DescriptorStatus{
			Code: ratelimitv3.RateLimitResponse_OVER_LIMIT,
		}
	}
	return &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OVER_LIMIT,
		Statuses:    statuses,
	}
}

// xrlStatus describes a single per-descriptor status for the X-RateLimit
// byte-pin in scenario (g). The fields map 1:1 onto the proto
// `RateLimitResponse.DescriptorStatus` X-RateLimit-bearing fields:
//
//	current_limit.requests_per_unit  = requestsPerUnit
//	current_limit.unit               = unit
//	limit_remaining                  = limitRemaining
//	duration_until_reset.seconds     = durationSeconds
//
// AMEND-6: only the four fields are set on each DescriptorStatus; no `name`,
// no `quota`, no nanos on the duration. The OverallCode is OVER_LIMIT.
type xrlStatus struct {
	requestsPerUnit uint32
	unit            ratelimitv3.RateLimitResponse_RateLimit_Unit
	limitRemaining  uint32
	durationSeconds int64
}

// respOverLimitWithStatuses builds an OVER_LIMIT RateLimitResponse with the
// per-descriptor statuses populated for X-RateLimit emission (scenario g).
// Mirrors the upstream byte-shape: OverallCode + Statuses[] each carrying
// Code=OVER_LIMIT + CurrentLimit{RequestsPerUnit, Unit} + LimitRemaining +
// DurationUntilReset{Seconds}.
func respOverLimitWithStatuses(items []xrlStatus) *ratelimitv3.RateLimitResponse {
	statuses := make([]*ratelimitv3.RateLimitResponse_DescriptorStatus, len(items))
	for i, it := range items {
		statuses[i] = &ratelimitv3.RateLimitResponse_DescriptorStatus{
			Code: ratelimitv3.RateLimitResponse_OVER_LIMIT,
			CurrentLimit: &ratelimitv3.RateLimitResponse_RateLimit{
				RequestsPerUnit: it.requestsPerUnit,
				Unit:            it.unit,
			},
			LimitRemaining:     it.limitRemaining,
			DurationUntilReset: &durationpb.Duration{Seconds: it.durationSeconds},
		}
	}
	return &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OVER_LIMIT,
		Statuses:    statuses,
	}
}

// --- fixture.Driver (required) ---

func (*rlDriver) BackendCount() int                { return 1 }
func (*rlDriver) BackendKind() fixture.BackendKind { return fixture.HTTPGlobalRateLimitGRPC }
func (*rlDriver) SubjectListenerName() string      { return "l_test_a" }
func (*rlDriver) ReferenceListenerPort() int       { return refLATestPort }

// ReferenceBootstrap renders envoy.yaml with the reference container ports
// + host.docker.internal backend / RLS-fake hosts (the reference container
// reaches host-side services via host.docker.internal per ADR-0010).
func (d *rlDriver) ReferenceBootstrap(backendPorts []int) string {
	rlsPort := d.allocateRLSPort()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   refAdminPort,
		"LATestPort":  refLATestPort,
		"BackendHost": "host.docker.internal",
		"BackendPort": backendPorts[0],
		"RLSHost":     "host.docker.internal",
		"RLSPort":     rlsPort,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated ports + the
// loopback backend / RLS-fake hosts (envoy-go runs on the host directly —
// no docker translation).
func (d *rlDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	rlsPort := d.allocateRLSPort()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"LATestPort":  subjListenerPort,
		"BackendPort": backendPorts[0],
		"RLSHost":     "127.0.0.1",
		"RLSPort":     rlsPort,
	})
}

// DriveReference issues the 9-scenario probe sequence (a/b/c/d/e/f1/f2/f3/g)
// against the reference proxy. Scenario (h) is OBSERVATIONAL — it issues NO
// additional requests; AssertStats reads the counters accumulated by the
// subject-side probes.
func (d *rlDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "ref")
}

// DriveSubject issues the same sequence against envoy-go.
func (d *rlDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin diff.
func (*rlDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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

// --- core probe sequence ---

// driveProxy issues the 9-scenario probe sequence against the listener
// address provided in addr. Scenario (h) is OBSERVATIONAL — it does NOT
// re-burst requests; it asserts the counter deltas accumulated by the
// preceding probes via StatsAsserter.AssertStats.
//
// Lifecycle (single stop, no mid-stream restart):
//
//  1. Start the fake with all per-scenario scripts pre-populated.
//  2. Scenarios a/b/c/d/f1/f2/f3/g — all RLS-up paths (admit + OVER_LIMIT).
//  3. STOP the fake.
//  4. Scenario e — RLS unreachable → fail-open admit.
//  5. Teardown — fake stays stopped.
//
// Probe order: a → b → c → d → f1 → f2 → f3 → g → STOP-fake → e. Scenario
// (e) is LAST in the sequence (so the fake-stop happens after every other
// probe). The Counter-delta accounting in AssertStats reflects this order.
//
// After this returns the runner's CompareBytes pass enforces cross-side
// byte-equivalence + the StatsAsserter runs against the subject admin and
// asserts the four cluster-scoped counter values accumulated by the
// non-error scenarios:
//
//	ok                   = 5 (b + d + f-override + f-include + f-ignore)
//	over_limit           = 2 (c + g)
//	error                = 1 (e)
//	failure_mode_allowed = 1 (e)
//
// Scenario (a) is zero-descriptor (no RLS call → no counter increment); (g)
// is OVER_LIMIT not OK.
func (d *rlDriver) driveProxy(ctx context.Context, addr, side string) ([]byte, error) {
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	baseURL := "http://" + addr

	if err := d.setupRLS(); err != nil {
		return nil, fmt.Errorf("[%s] setup rls: %w", side, err)
	}

	var b bytes.Buffer

	// (a) parse_ok — no rate_limits → echo 200.
	emitScenario(&b, "a", runGet(ctx, client, baseURL+"/scenario_a", nil, side, "a"))

	// (b) ok_admit — RLS scripted OK → echo 200.
	emitScenario(&b, "b", runGet(ctx, client, baseURL+"/scenario_b", nil, side, "b"))

	// (c) over_limit_429 — RLS scripted OVER_LIMIT → 429 empty body.
	emitScenario(&b, "c", runGet(ctx, client, baseURL+"/scenario_c", nil, side, "c"))

	// (d) descriptor_actions — 9-action chain. Phase 24.2 Task 6 EXTENDED
	// from 4 → 9 actions (the 5 remaining of the 10 canonical actions,
	// minus destination_cluster). The path carries a query string consumed
	// by query_parameters + query_parameter_value_match. The fake matches
	// the 9-entry CanonicalKey → OK → echo 200.
	emitScenario(&b, "d", runGet(ctx, client,
		baseURL+"/scenario_d?region=us-east&plan=premium",
		http.Header{
			tenantHeader: []string{tenantValue},
			canaryHeader: []string{"true"},
		}, side, "d"))

	// (f-override) Phase 24.2 Task 6 §4.3 Axis-B OVERRIDE-default arm.
	// Route has rate_limits → vhost SKIPPED. NO TPFC ⇒ proto-zero
	// vh_rate_limits=OVERRIDE. The fake matches the 1-descriptor key
	// `scenario=f_override` under domain_b → OK → echo 200.
	emitScenario(&b, "f_override", runGet(ctx, client,
		baseURL+"/scenario_f_override", nil, side, "f_override"))

	// (f-include) Phase 24.2 Task 6 §4.3 Axis-B INCLUDE arm. Per-route
	// RateLimitPerRoute.vh_rate_limits=INCLUDE → BOTH route AND vhost
	// descriptors emitted (route first per AMEND-6 walk order). The fake
	// matches the 2-descriptor key `scenario=f_include|vh=vh_a` → OK →
	// echo 200.
	emitScenario(&b, "f_include", runGet(ctx, client,
		baseURL+"/scenario_f_include", nil, side, "f_include"))

	// (f-ignore) Phase 24.2 Task 6 §4.3 Axis-B IGNORE arm. Per-route
	// RateLimitPerRoute.vh_rate_limits=IGNORE → vhost SKIPPED uncon-
	// ditionally. The fake matches the 1-descriptor key
	// `scenario=f_ignore` → OK → echo 200.
	emitScenario(&b, "f_ignore", runGet(ctx, client,
		baseURL+"/scenario_f_ignore", nil, side, "f_ignore"))

	// (g) x_ratelimit_headers — Phase 24.2 Task 6. Two-descriptor policy
	// (tier=bronze + scope=burst). The fake returns OVER_LIMIT with
	// per-descriptor CurrentLimit / LimitRemaining / DurationUntilReset
	// populated. The filter emits the X-RateLimit triple per AMEND-8 +
	// §4.7. The driver byte-pins the response headers into the cross-side
	// stream so any divergence in MIN-selection / unit→seconds mapping /
	// quota-policy suffix surfaces as a CompareBytes failure.
	gRes := runGet(ctx, client, baseURL+"/scenario_g", nil, side, "g")
	emitScenarioG(&b, gRes)

	// STOP the fake before scenario (e) — forces the gRPC dial to fail fast.
	d.stopRLS()

	// (e) failure_mode_open — RLS unreachable → fail-open admit → echo 200.
	emitScenario(&b, "e", runGet(ctx, client, baseURL+"/scenario_e", nil, side, "e"))

	// Teardown — fake stays stopped (the next fixture run starts a fresh one).
	return b.Bytes(), nil
}

// --- scenario probe primitives ---

// scenarioResult is the per-scenario observation captured for the byte stream
// the runner's CompareBytes pass compares between sides.
type scenarioResult struct {
	statusCode int
	body       []byte
	headers    http.Header
	err        error
}

// runGet issues a single GET against the supplied URL with optional headers
// and returns the response status + body + headers. Errors are folded into
// the scenarioResult; the caller's emitScenario maps them into the byte
// stream as a stable "status=ERR body=ERR" line (both sides see the same on
// a shared transport failure).
func runGet(ctx context.Context, client *http.Client, url string, hdr http.Header, side, label string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("do request: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("read body: %w", err)}
	}
	res := scenarioResult{statusCode: resp.StatusCode, body: body, headers: resp.Header.Clone()}
	if os.Getenv("FIXTURE_0032_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] %s: status=%d body=%q headers=%v\n",
			side, label, res.statusCode, string(res.body), res.headers)
	}
	return res
}

// emitScenario formats the per-scenario verdict line into the byte stream.
//
//	scenario <id> status=<code> body=<ok|mismatch(...)>
//
// The side label is NOT emitted (the byte stream must be identical per-side
// for the CompareBytes differential gate to fire on equivalence). On request
// error both sides emit "status=ERR body=ERR" — symmetrical and stable.
func emitScenario(b *bytes.Buffer, id string, res scenarioResult) {
	if res.err != nil {
		fmt.Fprintf(b, "scenario %s status=ERR body=ERR\n", id)
		return
	}
	bodyVerdict := classifyBody(id, res.body)
	fmt.Fprintf(b, "scenario %s status=%d body=%s\n", id, res.statusCode, bodyVerdict)
}

// emitScenarioG formats scenario (g)'s verdict line + the X-RateLimit triple
// byte-pin per Phase 24.2 Task 6. The byte stream carries:
//
//	scenario g status=<code> body=<ok|mismatch(...)>
//	scenario g x-ratelimit-limit=<value>
//	scenario g x-ratelimit-remaining=<value>
//	scenario g x-ratelimit-reset=<value>
//
// Both sides MUST emit byte-identical x-ratelimit-* values per §4.7 line 214
// + AMEND-8 + the Task 5 follow-up wire-order discipline. CompareBytes
// detects any cross-side divergence on the MIN-selection / unit→seconds map /
// quota-policy suffix.
//
// On request error or absent header (a regression in DRAFT_VERSION_03 gating)
// the value is emitted as "ABSENT" — symmetrical and stable.
func emitScenarioG(b *bytes.Buffer, res scenarioResult) {
	emitScenario(b, "g", res)
	if res.err != nil {
		fmt.Fprintln(b, "scenario g x-ratelimit-limit=ERR")
		fmt.Fprintln(b, "scenario g x-ratelimit-remaining=ERR")
		fmt.Fprintln(b, "scenario g x-ratelimit-reset=ERR")
		return
	}
	for _, name := range []string{"x-ratelimit-limit", "x-ratelimit-remaining", "x-ratelimit-reset"} {
		val := res.headers.Get(name)
		if val == "" {
			val = "ABSENT"
		}
		fmt.Fprintf(b, "scenario g %s=%s\n", name, val)
	}
}

// classifyBody returns the per-scenario body verdict. The body is NOT
// emitted byte-for-byte because Envoy adds per-hop headers (x-forwarded-for,
// x-request-id, x-envoy-*) that the echobackend reflects into its JSON body
// — those headers diverge across the two sides. Instead each scenario
// classifies the body structurally:
//
//	allow scenarios (a/b/d/e/f_override/f_include/f_ignore):
//	  ⇒ body is the echobackend echo JSON (object with method+path keys).
//
//	over_limit scenarios (c/g):
//	  ⇒ body is empty (no RawBody on the scripted OVER_LIMIT response).
func classifyBody(id string, body []byte) string {
	switch id {
	case "a", "b", "d", "e", "f_override", "f_include", "f_ignore":
		if !isEchoBody(body) {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", string(body))
		}
		return "ok"
	case "c", "g":
		if len(body) == 0 {
			return "ok"
		}
		return fmt.Sprintf("mismatch(want_empty,got=%q)", string(body))
	}
	return "skip"
}

// isEchoBody returns true iff body is a JSON object containing at least the
// "method" and "path" keys — the structural signature of the echobackend
// response (test/helpers/echobackend/echobackend.go::buildEcho).
func isEchoBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	_, hasMethod := m["method"]
	_, hasPath := m["path"]
	return hasMethod && hasPath
}

// --- fixture.StatsAsserter ---

// AssertStats performs the subject-only scenario (h) counter assertion
// after the runner's cross-side CompareBytes + admin diff. Reads the SUBJECT
// admin /stats/prometheus (refAdminAddr is unused — the reference counters
// are NOT asserted at 24.1) and asserts the four
// `cluster.c_ratelimit.ratelimit.*` counters at the deterministic deltas
// accumulated by the b/c/d/f1/f2/f3/g/e probe sequence:
//
//	ok                   = 5  (scenarios (b) + (d) + (f-override) + (f-include) + (f-ignore))
//	over_limit           = 2  (scenarios (c) + (g))
//	error                = 1  (scenario  (e) RLS unreachable → applyError arm)
//	failure_mode_allowed = 1  (scenario  (e) failure_mode_deny:false fail-open
//	                           — incremented INSIDE the applyError arm per
//	                           dispositions.go::applyError)
//
// Per reference_differential_asserter_dispatch the subject-side counter
// assertions MUST live in StatsAsserter (NOT SubjectAsserter; the latter
// fires only on the reference-less path). The deliberate-break recipe in
// the Task 10 PROGRESS entry proves AssertStats is LIVE by temporarily
// asserting a wrong value (must FAIL), then reverting (must GREEN).
func (d *rlDriver) AssertStats(t fixture.TB, _ /*refAdminAddr*/, subjAdminAddr string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	statsBody, err := scrapeStats(ctx, subjAdminAddr)
	if err != nil {
		t.Errorf("scenario h stat_surface: scrape /stats/prometheus: %v", err)
		return
	}
	statsOut := string(statsBody)

	if os.Getenv("FIXTURE_0032_DUMP_STATS") != "" {
		for _, line := range strings.Split(statsOut, "\n") {
			if strings.Contains(line, "ratelimit") || strings.Contains(line, "c_ratelimit") {
				fmt.Fprintf(os.Stderr, "[subj] %s\n", line)
			}
		}
	}

	// 4 cluster-scoped counter expectations per the b/c/d/f1/f2/f3/g/e
	// probe sequence (24.1 4-counter expectations EXTENDED for the 24.2
	// (f) 3-sub-scenario ok-admits + (g) over_limit).
	type expect struct {
		stat    string
		want    int64
		comment string
	}
	expectations := []expect{
		{stat: "ok", want: 5, comment: "scenarios (b) + (d) + (f-override) + (f-include) + (f-ignore) admit"},
		{stat: "over_limit", want: 2, comment: "scenarios (c) + (g) OVER_LIMIT"},
		{stat: "error", want: 1, comment: "scenario (e) RLS unreachable"},
		{stat: "failure_mode_allowed", want: 1, comment: "scenario (e) failure_mode_deny:false"},
	}
	for _, exp := range expectations {
		got, present := clusterRatelimitCounter(statsOut, rlsClusterName, exp.stat)
		if !present {
			t.Errorf("scenario h: cluster.%s.ratelimit.%s absent from /stats/prometheus (%s)",
				rlsClusterName, exp.stat, exp.comment)
			continue
		}
		if got != exp.want {
			t.Errorf("scenario h: cluster.%s.ratelimit.%s = %d; want %d (%s)",
				rlsClusterName, exp.stat, got, exp.want, exp.comment)
		}
	}
}

// scrapeStats fetches /stats/prometheus from the supplied admin endpoint.
func scrapeStats(ctx context.Context, adminAddr string) ([]byte, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("scrape /stats/prometheus: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// clusterRatelimitCounter returns the value of the
// `envoy_cluster_ratelimit.<stat>{envoy_cluster_name="<cluster>"}` Prometheus
// counter, or (0, false) if no matching line is found. The Prometheus form
// reflects the absolute name `cluster.<cluster>.ratelimit.<stat>` via the
// SN1 flattening rule (internal/stats/name.go:42-51):
//
//	cluster.c_ratelimit.ratelimit.ok
//	  → tail="c_ratelimit.ratelimit.ok"
//	  → label envoy_cluster_name="c_ratelimit", rest="ratelimit.ok"
//	  → base = "envoy_cluster_" + rest = "envoy_cluster_ratelimit.ok"
//
// NOTE: unlike SN2 (which applies a dot→underscore transform on the `rest`
// segment per the phase-09 follow-up), SN1 does NOT apply the transform.
// The emitted Prometheus name therefore contains a literal '.' between
// "ratelimit" and the leaf — non-standard but functional (Prometheus
// rejects metric names with dots in general; envoy-go's exposition emits
// the line as-is). The lookup helper matches the literal form. A future
// stats-cleanup phase MAY extend SN1 with the dot→underscore transform;
// this fixture is the first cross-namespace cluster-scoped stat consumer
// per AMEND-1/AMEND-10 + ADR-0197 and the empirical Prometheus shape
// surfaces here.
//
// Absent ≡ 0 semantics for counters is honored at the call-site via the
// boolean: callers that expect a non-zero value treat absent as a failure
// (the counter MUST be registered + scraped after the probe sequence).
func clusterRatelimitCounter(statsOut, cluster, stat string) (int64, bool) {
	// Match both the literal-dot form (SN1 as-implemented) and the
	// underscore-normalized form (SN1 after a future dot→underscore
	// extension) so the helper survives a stats-cleanup phase without
	// fixture churn.
	needlePrefixes := []string{
		"envoy_cluster_ratelimit." + stat,
		"envoy_cluster_ratelimit_" + stat,
	}
	labelNeedle := fmt.Sprintf(`envoy_cluster_name="%s"`, cluster)
	for _, line := range strings.Split(statsOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		matched := false
		for _, p := range needlePrefixes {
			// Match prefix + delimiter (space OR `{`) so we don't match
			// e.g., "envoy_cluster_ratelimit.over_limit" when looking up
			// "envoy_cluster_ratelimit.over" (defensive — both stat suffixes
			// share a common prefix in the over_limit/over case).
			if strings.HasPrefix(line, p+" ") || strings.HasPrefix(line, p+"{") {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if !strings.Contains(line, labelNeedle) {
			continue
		}
		lastSpace := strings.LastIndex(line, " ")
		if lastSpace < 0 {
			continue
		}
		valStr := strings.TrimSpace(line[lastSpace+1:])
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		return int64(val), true
	}
	return 0, false
}

// --- file/template helpers ---

// fixtureDir returns the absolute path to the test/fixtures/0032-http-
// ratelimit/ directory (the parent of inputs/).
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
	_ fixture.Driver           = (*rlDriver)(nil)
	_ fixture.BackendKindAware = (*rlDriver)(nil)
	_ fixture.StatsAsserter    = (*rlDriver)(nil)
)
