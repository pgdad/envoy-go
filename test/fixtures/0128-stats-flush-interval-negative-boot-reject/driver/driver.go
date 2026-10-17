// Package driver registers the 0128-stats-flush-interval-negative-boot-reject
// BOOT-REJECT differential fixture with the runner per phase 102 SPEC §6. It
// is a SYMMETRIC boot-reject fixture: a top-level bootstrap
// `stats_flush_interval: -0.5s` MUST fail-closed at config-load on BOTH
// reference Envoy v1.37.2 AND envoy-go, and BOTH stderr buffers must contain
// the reference's negative-duration phrase.
//
// # Reject arm
//
// `-0.5s` is a negative NANOS value (seconds 0, nanos -500000000). The
// reference refuses a negative Duration before its range check, with the
// `Invalid duration` class, not the range class. The value is chosen over
// `-1s` because seconds 0 is the case that still reaches the lower range bound:
// an envoy-go that checked the range first (or collapsed the classes into one
// message) would print the range phrase for `-0.5s` and fail this fixture,
// while `-1s` (seconds -1, below 300 and not 0) would skip the lower bound and
// let that mutant pass. The bootstrap carries NO `stats_sinks`, so the claim is
// sink-independent.
//
// # Common boot-reject substring
//
//   - reference Envoy v1.37.2 stderr: "... Invalid duration: Expected
//     positive duration ..."
//   - envoy-go stderr: "load config: bootstrap: stats_flush_interval: Invalid
//     duration: Expected positive duration"
//
// The needle stops before the reference's `: goo.gle/debug...` token and does
// not contain `bootstrap:`, which the reference never prints.
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap (fixture-0042 shape). One listener `l_a`
// with an HCM direct_response (status 222, inline body: envoy-go refuses a
// direct_response without a body, so a body-less route would boot-reject the
// subject for an unrelated reason) and a minimal c_unused cluster
// (127.0.0.1:1, never dialed). Neither is reached: the reject fires while the
// bootstrap is loaded, before any listener binds.
//
// # Cross-references
//
//   - phase 102 SPEC §6 (two boot-reject directories) and §6.4 (NC cells)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - fixture-0127 (the range sibling)
//   - fixture-0042 (the BootRejectFixture shape this copies)
package driver

import (
	"context"
	"fmt"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0128-stats-flush-interval-negative-boot-reject"

	refAdminPort = 9901
	refPort      = 15128 // l_a — the single boot-reject listener (ref container port).

	// flushInterval is the rejected top-level stats_flush_interval value.
	flushInterval = "-0.5s"

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr.
	expectedBootErrorSubstr = "Invalid duration: Expected positive duration"
)

func init() {
	fixture.RegisterFixture(fixtureName, &flushNegativeBootRejectDriver{})
}

type flushNegativeBootRejectDriver struct{}

// --- fixture.Driver (required) ---

func (*flushNegativeBootRejectDriver) BackendCount() int           { return 1 } // runner fatals on n<1; the backend is unused by the boot-reject path
func (*flushNegativeBootRejectDriver) SubjectListenerName() string { return "l_a" }
func (*flushNegativeBootRejectDriver) ReferenceListenerPort() int  { return refPort }

// ReferenceBootstrap renders the self-contained boot-reject bootstrap.
func (*flushNegativeBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side.
func (*flushNegativeBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject / ProbeAdmin are required by the Driver
// interface but never invoked in the boot-reject branch.

func (*flushNegativeBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*flushNegativeBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*flushNegativeBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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

// BootRejectScript returns "": the trigger is inline in the bootstrap.
func (*flushNegativeBootRejectDriver) BootRejectScript() string { return "" }

// ExpectedBootErrorSubstring returns the substring asserted in BOTH stderrs.
func (*flushNegativeBootRejectDriver) ExpectedBootErrorSubstring() string {
	return expectedBootErrorSubstr
}

// renderBootRejectBootstrap returns the bootstrap BOTH proxies consume. The
// top-level stats_flush_interval is the only rejected field; there is no
// stats_sinks entry.
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

stats_flush_interval: %s

static_resources:
  listeners:
    - name: l_a
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: l_a
                route_config:
                  name: r_a
                  virtual_hosts:
                    - name: vh_a
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 222
                            body: { inline_string: l_a }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

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
`, adminPort, flushInterval, listenerPort)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver = (*flushNegativeBootRejectDriver)(nil)
)
