// Package driver registers the 0047-zookeeper-boot-reject BOOT-REJECT
// differential fixture with the runner per phase 28.1b SPEC §5.2. It is a
// SYMMETRIC cross-side boot-reject fixture: an
// envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy config
// whose `stat_prefix` is MISSING (the PGV-required
// `(validate.rules).string.min_len = 1` field, unset) MUST fail-closed at
// config-load on BOTH reference Envoy v1.37.2 AND envoy-go, and BOTH stderr
// buffers must contain a shared case-sensitive substring.
//
// # Relationship to 0044 and fixture-dispatch constraint
//
// 0047 is the ZooKeeperProxy-filter BOOT-REJECT analog of
// 0044-network-rbac-boot-reject (the symmetric BootRejectFixture template).
// Per the project's fixture-dispatch constraint (one fixture dir = one runner
// branch), this directory is the BOOT-REJECT branch only; the cross-side
// request arms live in 0046-zookeeper-requests.
//
// # Why stat_prefix-missing is a GENUINE cross-side both-reject
//
// The zookeeper_proxy proto marks `stat_prefix` PGV-required:
// `(validate.rules).string = {min_len: 1}` annotation on the field. Reference
// Envoy v1.37.2 (C++ proto with the same PGV annotation) ALSO rejects a
// missing stat_prefix at config-load. So this is a genuine PGV-mirror
// both-sides-reject (SPEC §5.2 lists the missing-stat_prefix case as
// boot-reject PARITY for zookeeper_proxy) — analogous to the
// rbac-network-stat-prefix-required rejection in 0044.
//
// # Common boot-reject substring (empirically pinned at Task 8)
//
// The two implementations surface the SAME violation with DIFFERENT wordings
// (captured live, dockerized v1.37.2):
//
//   - reference Envoy v1.37.2 stderr (PGV violation; PascalCase field name):
//     "Proto constraint validation failed
//     (ZooKeeperProxyValidationError.StatPrefix: value length must be at
//     least 1 characters)"
//   - envoy-go stderr (errStatPrefixRequired const, config.go:149;
//     snake_case field name): "zookeeper_proxy: stat_prefix is required"
//
// IMPORTANT — the two ERROR wordings share NO distinctive case-sensitive token
// by their error lines alone (ref names the field PascalCase `StatPrefix`;
// envoy-go names it snake_case `stat_prefix`). The substring assertion uses
// `stat_prefix` (mirroring the 0044 precedent):
//   - SUBJECT side (the side under test): `stat_prefix` is the envoy-go error
//     wording itself — the subject stderr is JUST the error line (no YAML
//     echo), so this match is fully load-bearing.
//   - REFERENCE side: reference Envoy echoes the offending bootstrap into its
//     stderr, so `stat_prefix` (the tcp_proxy filter's required field in the
//     rejected config) appears there; the GENUINE reference-reject assertion
//     is the runner's separate `refErr != nil` gate.
//
// Both sides rejecting is the cross-side parity claim; the per-side substring
// pins the envoy-go wording. See PROGRESS.md Task 8 for the full capture.
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap (Option B precedent from fixtures 0033/0042):
// the stat_prefix-missing zookeeper_proxy config is embedded directly in the
// rendered bootstrap. A minimal c_unused cluster (127.0.0.1:1 — never dialed)
// is declared so envoy-go's cluster manager (which runs BEFORE the listener
// manager) does not fail with a zero-cluster error before the listener-manager
// config-load reject fires. Same ordering sidestep as fixtures 0033/0041/0042.
//
// # Cross-references
//
//   - parent SPEC §5.2 (boot-reject zookeeper-proxy fixture scope; PGV-mirror)
//   - 28.1b PLAN Task 8 (this fixture)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - fixture-0044-network-rbac-boot-reject (the symmetric template this mirrors)
//   - fixture-0046-zookeeper-requests (cross-side arms; the one-dir-one-branch
//     companion for the ZooKeeperProxy filter)
package driver

import (
	"context"
	"fmt"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0047-zookeeper-boot-reject"

	refAdminPort = 9901
	refZKPort    = 15049 // l_zk — the single boot-reject listener (ref container port).

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr after the
	// boot-reject. `stat_prefix` is the envoy-go error wording verbatim
	// ("zookeeper_proxy: stat_prefix is required") — load-bearing on the subject
	// side (the subject stderr is JUST the error line, no YAML echo). The
	// reference side uses PascalCase "StatPrefix" in its PGV error and ALSO
	// echoes the offending bootstrap, so `stat_prefix` matches the reference
	// stderr via the echoed config; the genuine reference-reject is the runner's
	// separate refErr!=nil gate. See the package doc + PROGRESS.md Task 8.
	expectedBootErrorSubstr = "stat_prefix"
)

func init() {
	fixture.RegisterFixture(fixtureName, &zkBootRejectDriver{})
}

type zkBootRejectDriver struct{}

// --- fixture.Driver (required) ---

func (*zkBootRejectDriver) BackendCount() int           { return 1 } // runner fatals on n<1; spare TCP-echo backend unused by the boot-reject path.
func (*zkBootRejectDriver) SubjectListenerName() string { return "l_zk" }
func (*zkBootRejectDriver) ReferenceListenerPort() int  { return refZKPort }

// ReferenceBootstrap renders the self-contained single-listener boot-reject
// bootstrap. The zookeeper_proxy filter's `stat_prefix` is UNSET (omitted),
// which triggers the PGV min_len=1 PARSE-REJECT on both sides.
func (*zkBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refZKPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side.
func (*zkBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject are never invoked in the boot-reject branch
// (the runner SKIPS Drive + admin-diff for BootRejectFixture drivers).
func (*zkBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*zkBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*zkBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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

// --- harness.BootRejectFixture ---

// BootRejectScript returns "" — this fixture embeds the boot-reject trigger
// (the missing stat_prefix) entirely inline; there is NO on-disk script.
func (*zkBootRejectDriver) BootRejectScript() string { return "" }

// ExpectedBootErrorSubstring returns the literal substring the runner asserts is
// present (case-sensitive Contains) in BOTH ref + subj stderr.
func (*zkBootRejectDriver) ExpectedBootErrorSubstring() string { return expectedBootErrorSubstr }

// renderBootRejectBootstrap returns the self-contained single-listener bootstrap
// BOTH proxies consume. The zookeeper_proxy filter's `stat_prefix` is UNSET
// (omitted) — this triggers the PGV min_len=1 stat_prefix-required PARSE-REJECT
// on config-load on both reference Envoy + envoy-go.
//
// A minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared so
// envoy-go's cluster manager runs before the listener manager and does not fail
// with a zero-cluster error before the listener config-load reject. Same
// ordering sidestep as fixtures 0033/0041/0042.
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

static_resources:
  listeners:
    - name: l_zk
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.zookeeper_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy
                # stat_prefix INTENTIONALLY OMITTED — PGV min_len=1 violation
                # triggers the stat_prefix-required PARSE-REJECT on both sides.
                max_packet_bytes: 1048576
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
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
`, adminPort, listenerPort)
}

// Compile-time interface assertion.
var _ fixture.Driver = (*zkBootRejectDriver)(nil)
