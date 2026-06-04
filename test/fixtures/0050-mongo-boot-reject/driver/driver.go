// Package driver registers the 0050-mongo-boot-reject BOOT-REJECT differential
// fixture with the runner per phase 29.1 SPEC §8.1 + PLAN Task 16. It is a
// SYMMETRIC cross-side boot-reject fixture: a
// envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy config whose
// `stat_prefix` is MISSING MUST fail-closed at config-load on BOTH reference
// Envoy AND envoy-go, and BOTH stderr buffers must contain a shared
// case-sensitive substring.
//
// # Relationship to 0049 and the fixture-dispatch constraint
//
// 0050 is the MongoProxy-filter BOOT-REJECT analog of
// 0047-zookeeper-boot-reject (the symmetric BootRejectFixture template).
// Per the project's fixture-dispatch constraint (one fixture dir = one runner
// branch), this directory is the BOOT-REJECT branch only; the cross-side
// request arms live in 0049-mongo-requests.
//
// # Why stat_prefix-missing is a GENUINE cross-side both-reject
//
// The mongo_proxy proto marks `stat_prefix` required. Reference Envoy rejects a
// missing stat_prefix at config-load (PGV violation), and envoy-go's config
// parser rejects it with the const errStatPrefixRequired
// ("mongo_proxy: stat_prefix is required", config.go). So this is a genuine
// both-sides-reject — analogous to the zookeeper stat_prefix-required rejection
// in 0047.
//
// # Common boot-reject substring
//
// The two implementations surface the SAME violation with DIFFERENT wordings:
//
//   - reference Envoy stderr: a PGV violation naming the field (and echoing the
//     offending bootstrap, in which `stat_prefix` appears on the tcp_proxy
//     filter line).
//   - envoy-go stderr: "mongo_proxy: stat_prefix is required" — the error line
//     itself contains the snake_case token `stat_prefix`.
//
// The substring assertion uses `stat_prefix` (mirroring the 0044/0047
// precedent):
//   - SUBJECT side (the side under test): `stat_prefix` is the envoy-go error
//     wording itself — the subject stderr is JUST the error line (no YAML echo),
//     so this match is fully load-bearing.
//   - REFERENCE side: reference Envoy echoes the offending bootstrap into its
//     stderr, so `stat_prefix` (the tcp_proxy filter's required field in the
//     rejected config) appears there; the GENUINE reference-reject assertion is
//     the runner's separate refErr != nil gate.
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap. A minimal c_unused cluster (127.0.0.1:1 —
// never dialed) is declared so envoy-go's cluster manager (which runs BEFORE the
// listener manager) does not fail with a zero-cluster error before the listener
// config-load reject fires (reference_network_filter_typeurl_extensions: a
// zero-cluster boot is rejected by both sides). Same ordering sidestep as
// fixtures 0033/0041/0042/0044/0047.
//
// # Cross-references
//
//   - phase 29.1 SPEC §8.1 (boot-reject mongo-proxy fixture scope)
//   - 29.1 PLAN Task 16 (this fixture)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - fixture-0047-zookeeper-boot-reject (the symmetric template this mirrors)
//   - fixture-0049-mongo-requests (cross-side arms; the one-dir-one-branch
//     companion for the MongoProxy filter)
package driver

import (
	"context"
	"fmt"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0050-mongo-boot-reject"

	refAdminPort = 9901
	refMongoPort = 15050 // l_mongo — the single boot-reject listener (ref container port).

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr after the
	// boot-reject. `stat_prefix` is the envoy-go error wording verbatim
	// ("mongo_proxy: stat_prefix is required") — load-bearing on the subject side
	// (the subject stderr is JUST the error line, no YAML echo). The reference
	// side echoes the offending bootstrap, so `stat_prefix` matches there too; the
	// genuine reference-reject is the runner's separate refErr!=nil gate.
	expectedBootErrorSubstr = "stat_prefix"

	// mongoProxyType is the mongo_proxy typed_config @type URL — the network-filter
	// type URLs carry the extensions. segment
	// (reference_network_filter_typeurl_extensions).
	mongoProxyType = "type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy"
	tcpProxyType   = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"
)

func init() {
	fixture.RegisterFixture(fixtureName, &mongoBootRejectDriver{})
}

type mongoBootRejectDriver struct{}

// --- fixture.Driver (required) ---

func (*mongoBootRejectDriver) BackendCount() int           { return 1 } // runner fatals on n<1; spare backend unused by the boot-reject path.
func (*mongoBootRejectDriver) SubjectListenerName() string { return "l_mongo" }
func (*mongoBootRejectDriver) ReferenceListenerPort() int  { return refMongoPort }

// ReferenceBootstrap renders the self-contained single-listener boot-reject
// bootstrap. The mongo_proxy filter's `stat_prefix` is UNSET (omitted), which
// triggers the stat_prefix-required PARSE-REJECT on both sides.
func (*mongoBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refMongoPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side.
func (*mongoBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject are never invoked in the boot-reject branch
// (the runner SKIPS Drive + admin-diff for BootRejectFixture drivers).
func (*mongoBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*mongoBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*mongoBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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
func (*mongoBootRejectDriver) BootRejectScript() string { return "" }

// ExpectedBootErrorSubstring returns the literal substring the runner asserts is
// present (case-sensitive Contains) in BOTH ref + subj stderr.
func (*mongoBootRejectDriver) ExpectedBootErrorSubstring() string { return expectedBootErrorSubstr }

// renderBootRejectBootstrap returns the self-contained single-listener bootstrap
// BOTH proxies consume. The mongo_proxy filter's `stat_prefix` is UNSET
// (omitted) — this triggers the stat_prefix-required PARSE-REJECT on config-load
// on both reference Envoy + envoy-go.
//
// A minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared so
// envoy-go's cluster manager runs before the listener manager and does not fail
// with a zero-cluster error before the listener config-load reject. Same
// ordering sidestep as fixtures 0033/0041/0042/0044/0047.
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

static_resources:
  listeners:
    - name: l_mongo
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.mongo_proxy
              typed_config:
                "@type": %s
                # stat_prefix INTENTIONALLY OMITTED — the required-field violation
                # triggers the stat_prefix-required PARSE-REJECT on both sides.
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": %s
                stat_prefix: ingress_tcp
                cluster: c_unused

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
`, adminPort, listenerPort, mongoProxyType, tcpProxyType)
}

// Compile-time interface assertion. The BootRejectFixture interface lives in
// package differential (harness.go), which the driver package does not import to
// avoid an import cycle; the runner asserts the BootRejectScript/
// ExpectedBootErrorSubstring method set structurally at dispatch (the 0047
// precedent).
var _ fixture.Driver = (*mongoBootRejectDriver)(nil)
