// Package driver registers the 0058-thrift-boot-reject BOOT-REJECT differential
// fixture with the runner per phase 33 SPEC §8.2 + PLAN Task 13. It is a
// SYMMETRIC cross-side boot-reject fixture: a
// envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy config whose
// `stat_prefix` is MISSING MUST fail-closed at config-load on BOTH the contrib
// reference Envoy (envoyproxy/envoy:contrib-v1.37.2) AND envoy-go, and BOTH
// stderr buffers must contain a shared case-sensitive substring.
//
// # Relationship to 0057 and the fixture-dispatch constraint
//
// 0058 is the ThriftProxy-filter BOOT-REJECT analog of 0056-redis-boot-reject
// (and 0050-mongo / 0054-kafka — the symmetric BootRejectFixture template).
// Per the project's fixture-dispatch constraint (one fixture dir = one runner
// branch — reference_differential_fixture_dispatch_constraint), this directory
// is the BOOT-REJECT branch only; the cross-side request arms live in
// 0057-thrift-roundtrip.
//
// # Why stat_prefix-missing is a GENUINE cross-side both-reject
//
// The thrift_proxy proto marks `stat_prefix` PGV-required (min 1 rune).
// Reference Envoy rejects a missing stat_prefix at config-load (PGV violation),
// and envoy-go's config parser rejects it with the const errStatPrefixRequired
// ("thrift_proxy: stat_prefix is required", config.go — the ADR-0080 byte-stable
// reject string). So this is a genuine both-sides-reject (SPEC §8.2 / §6.2),
// analogous to the redis/mongo/kafka stat_prefix-required rejections in
// 0056/0050/0054. The other PGV arms (route/route-action/thrift-filter-name) +
// the un-chosen transport/protocol DEPARTURE arms are UNIT-TESTED (Task 3/4
// config_test.go + route_test.go) — the load-bearing FIXTURE arm is the
// missing-stat_prefix one only.
//
// # Common boot-reject substring (honest cross-impl divergence)
//
// The PRIMARY, load-bearing claim of this fixture is that BOTH sides FAIL TO
// BOOT (the runner's refErr != nil && subjErr != nil gate in
// runBootRejectFixture). The shared substring is a SECONDARY sanity check on the
// rejected stderr.
//
// The two implementations word the SAME violation DIFFERENTLY (empirically
// captured against envoyproxy/envoy:contrib-v1.37.2 — see README):
//
//   - reference Envoy stderr: a PGV violation in CamelCase —
//     "Proto constraint validation failed (ThriftProxyValidationError.StatPrefix:
//     value length must be at least 1 characters)" — plus an echo of the
//     offending bootstrap YAML (the `--config-yaml` boot path the harness uses
//     echoes the FULL bootstrap).
//   - envoy-go stderr: "thrift_proxy: stat_prefix is required" — snake_case.
//
// So lowercase `stat_prefix` does NOT appear in the reference's genuine stderr
// (the field was OMITTED from the bootstrap, so there is no `stat_prefix:` line
// to echo; the reference renders the violation as CamelCase `StatPrefix`).
//
// The substring is therefore `thrift_proxy` — the strongest token that GENUINELY
// appears in BOTH real stderrs from a NON-circular source (the 0056 `redis_proxy`
// precedent):
//   - SUBJECT side: the error line itself ("thrift_proxy: stat_prefix is
//     required") — the subject stderr is the error line (no extra YAML echo).
//   - REFERENCE side: the echoed bootstrap's REAL filter
//     `name: envoy.filters.network.thrift_proxy` and the
//     `thrift_proxy.v3.ThriftProxy` typed_config @type — load-bearing config
//     tokens that SELECT this filter, NOT a comment injected to satisfy the
//     assertion. The GENUINE reference-reject assertion remains the runner's
//     separate refErr != nil gate. (Verified empirically: the `--config-yaml`
//     boot echoes lowercase `thrift_proxy` twice — the filter name + the @type.)
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap. A minimal c_unused cluster (127.0.0.1:1 —
// never dialed) is declared so envoy-go's cluster manager (which runs BEFORE the
// listener manager) does not fail with a zero-cluster error before the listener
// config-load reject fires (reference_network_filter_typeurl_extensions: a
// zero-cluster boot is rejected by both sides). Same ordering sidestep as
// fixtures 0033/0041/0042/0044/0047/0050/0054/0056.
//
// # Cross-references
//
//   - phase 33 SPEC §8.2 (thrift-proxy boot-reject fixture scope) + §6.2
//     (boot-stderr substring parity).
//   - 33 PLAN Task 13 (this fixture).
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch).
//   - fixture-0056-redis-boot-reject (the symmetric template this mirrors —
//     same CamelCase-vs-snake_case substring asymmetry, same `<filter>_proxy`
//     substring resolution).
//   - fixture-0057-thrift-roundtrip (cross-side arms; the one-dir-one-branch
//     companion for the ThriftProxy filter).
package driver

import (
	"context"
	"fmt"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0058-thrift-boot-reject"

	refAdminPort = 9901
	// In-container reference Envoy listener port for l_thrift — the single
	// boot-reject listener. 0057-thrift-roundtrip takes 19147; 0058 takes 19148.
	refThriftPort = 19148

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr after the
	// boot-reject. It is a SECONDARY sanity check; the PRIMARY, load-bearing
	// claim is the runner's separate "both sides fail to boot" gate
	// (refErr!=nil && subjErr!=nil).
	//
	// The two implementations word the SAME violation DIFFERENTLY (captured
	// empirically against envoyproxy/envoy:contrib-v1.37.2):
	//   - subject (envoy-go): "thrift_proxy: stat_prefix is required" — snake_case.
	//   - reference (C++ Envoy): "ThriftProxyValidationError.StatPrefix: value
	//     length must be at least 1 characters" — CamelCase `StatPrefix`.
	// So lowercase `stat_prefix` does NOT appear in the reference's genuine
	// stderr (the omitted field is never echoed). `thrift_proxy` is the strongest
	// token GENUINELY present in BOTH (the 0056 `redis_proxy` precedent):
	//   - subject: the error line itself ("thrift_proxy: stat_prefix is required").
	//   - reference: the echoed bootstrap's REAL filter
	//     `name: envoy.filters.network.thrift_proxy` + the
	//     `thrift_proxy.v3.ThriftProxy` @type — load-bearing config tokens, NOT a
	//     comment injected to satisfy the assertion.
	expectedBootErrorSubstr = "thrift_proxy"

	// thriftProxyType is the thrift_proxy typed_config @type URL. The
	// network-filter type URLs carry the extensions. segment
	// (reference_network_filter_typeurl_extensions). thrift_proxy is a CORE
	// /envoy extension (D-T1); the FQN is
	// envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy.
	thriftProxyType = "type.googleapis.com/envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy"
)

func init() {
	fixture.RegisterFixture(fixtureName, &thriftBootRejectDriver{})
}

type thriftBootRejectDriver struct{}

// --- fixture.Driver (required) ---

func (*thriftBootRejectDriver) BackendCount() int           { return 1 } // runner fatals on n<1; spare backend unused by the boot-reject path.
func (*thriftBootRejectDriver) SubjectListenerName() string { return "l_thrift" }
func (*thriftBootRejectDriver) ReferenceListenerPort() int  { return refThriftPort }

// ReferenceBootstrap renders the self-contained single-listener boot-reject
// bootstrap. The thrift_proxy filter's `stat_prefix` is UNSET (omitted), which
// triggers the stat_prefix-required PARSE-REJECT on both sides.
func (*thriftBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refThriftPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side.
func (*thriftBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject are never invoked in the boot-reject branch
// (the runner SKIPS Drive + admin-diff for BootRejectFixture drivers).
func (*thriftBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*thriftBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*thriftBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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

// BootRejectScript returns "" — this fixture embeds the boot-reject trigger
// (the missing stat_prefix) entirely inline; there is NO on-disk script.
func (*thriftBootRejectDriver) BootRejectScript() string { return "" }

// ExpectedBootErrorSubstring returns the literal substring the runner asserts is
// present (case-sensitive Contains) in BOTH ref + subj stderr.
func (*thriftBootRejectDriver) ExpectedBootErrorSubstring() string { return expectedBootErrorSubstr }

// renderBootRejectBootstrap returns the self-contained single-listener bootstrap
// BOTH proxies consume. The thrift_proxy filter's `stat_prefix` is UNSET
// (omitted) — this triggers the stat_prefix-required PARSE-REJECT on config-load
// on both reference Envoy + envoy-go. NOTE: the bootstrap deliberately carries
// NO driver comment naming the asserted substring; the `thrift_proxy` token the
// runner asserts comes from the REAL filter name + @type, not from a comment
// injected to satisfy the assertion.
//
// A minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared so
// envoy-go's cluster manager runs before the listener manager and does not fail
// with a zero-cluster error before the listener config-load reject. Same
// ordering sidestep as fixtures 0033/0041/0042/0044/0047/0050/0054/0056.
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

static_resources:
  listeners:
    - name: l_thrift
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.thrift_proxy
              typed_config:
                "@type": %s
                transport: FRAMED
                protocol: BINARY
                route_config:
                  name: thrift_routes
                  routes:
                    - match: { method_name: "Ping" }
                      route: { cluster: c_unused }

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
`, adminPort, listenerPort, thriftProxyType)
}

// Compile-time interface assertion. The BootRejectFixture interface lives in
// package differential (harness.go), which the driver package does not import to
// avoid an import cycle; the runner asserts the BootRejectScript/
// ExpectedBootErrorSubstring method set structurally at dispatch (the 0050/0056
// precedent).
var _ fixture.Driver = (*thriftBootRejectDriver)(nil)
