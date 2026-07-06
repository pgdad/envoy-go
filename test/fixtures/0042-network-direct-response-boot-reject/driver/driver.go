// Package driver registers the 0042-network-direct-response-boot-reject
// BOOT-REJECT differential fixture with the runner per phase 26.1 SPEC §8.3 +
// PLAN Task 16 + D-P26.1-4. It is a SYMMETRIC boot-reject fixture: a
// `direct_response` network filter whose `response` is present but whose
// DataSource `specifier` oneof is UNSET (`response: {}`) MUST fail-closed at
// config-load on BOTH reference Envoy v1.37.2 AND envoy-go, and BOTH stderr
// buffers must contain a shared case-sensitive substring.
//
// # Reject arm (D-P26.1-4)
//
// The empirically-pinned reject arm is `response: {}` (Response message
// PRESENT, DataSource specifier oneof UNSET). The alternative arm — `response`
// ABSENT entirely — does NOT reject on the reference: upstream Envoy treats the
// `Config.response` field itself as optional and boots successfully when it is
// absent (only validating the `specifier` oneof once a `response` message is
// present). envoy-go rejects BOTH arms (its resolveDataSource catches a nil
// DataSource and a nil specifier identically), but only the `response: {}` arm
// produces a SYMMETRIC cross-side reject, so that is the arm this fixture pins.
//
// # Common boot-reject substring (D-P26.1-4)
//
// Per the empirical capture finalized at Task 16 (dockerized v1.37.2):
//
//   - reference Envoy v1.37.2 stderr (PGV violation; load-bearing wording):
//     "Proto constraint validation failed (ConfigValidationError.Response:
//     embedded message failed validation | caused by field: \"specifier\",
//     reason: is required)"
//   - envoy-go stderr (parseRejectResponseSpecifierRequired; Task-9 const):
//     "listener manager: listener: \"l_dr\": filter_chains[0]: filters[0]:
//     direct_response: response.specifier is required"
//
// Both wordings name the DataSource oneof field `specifier` verbatim
// (case-identical): upstream quotes the proto oneof field name
// (`field: "specifier"`), and envoy-go names the same field in its byte-stable
// const (`response.specifier is required`). The runner's substring assertion is
// case-sensitive (`strings.Contains`), and the 9-character token `specifier`
// appears in both — it is the distinctive shared fragment finalized for
// D-P26.1-4. (The fragment `is required` is ALSO shared, but `specifier` is the
// more distinctive load-bearing token — the oneof field name both
// implementations independently surface.)
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap (Option B precedent from fixture-0033): the
// unset `response: {}` is embedded directly in the rendered bootstrap. A
// minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared so
// envoy-go's cluster manager (which runs BEFORE the listener manager) does not
// fail with a zero-cluster error before the listener-manager config-load reject
// fires on the direct_response filter. Same ordering sidestep as
// fixture-0033/0041.
//
// # Cross-references
//
//   - parent SPEC §8.3 (boot-reject network fixture scope)
//   - parent SPEC §6.1 (direct_response specifier-required PARSE-REJECT)
//   - 26.1 PLAN Task 16 + D-P26.1-4 (boot-reject common stderr substring)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - fixture-0033 (nearest BootRejectFixture precedent; http ratelimit)
//   - fixture-0041 (the cross-side direct_response sibling fixture)
package driver

import (
	"context"
	"fmt"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0042-network-direct-response-boot-reject"

	refAdminPort = 9901
	refDRPort    = 15042 // l_dr — the single boot-reject listener (ref container port).

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr after the
	// boot-reject. Finalized empirically at Task 16 per D-P26.1-4:
	//
	//   reference Envoy v1.37.2 stderr (PGV violation; load-bearing wording):
	//     "...ConfigValidationError.Response: embedded message failed validation
	//      | caused by field: \"specifier\", reason: is required"
	//   envoy-go stderr (parseRejectResponseSpecifierRequired; Task-9 const):
	//     "...direct_response: response.specifier is required"
	//
	// Both wordings name the DataSource oneof field `specifier` verbatim
	// (case-identical), so the 9-character token `specifier` is a shared
	// case-sensitive substring. It is the distinctive fragment finalized for
	// D-P26.1-4 (the oneof field name both implementations independently
	// surface). Per AMEND-10 option 2 the substring need only appear ANYWHERE
	// in each stderr.
	expectedBootErrorSubstr = "specifier"
)

func init() {
	fixture.RegisterFixture(fixtureName, &drBootRejectDriver{})
}

type drBootRejectDriver struct{}

// --- fixture.Driver (required) ---

func (*drBootRejectDriver) BackendCount() int           { return 1 } // runner fatals on n<1; spare TCP-echo backend is unused by the boot-reject path
func (*drBootRejectDriver) SubjectListenerName() string { return "l_dr" }
func (*drBootRejectDriver) ReferenceListenerPort() int  { return refDRPort }

// ReferenceBootstrap renders the self-contained single-listener boot-reject
// bootstrap. The direct_response filter's `response: {}` has its specifier
// oneof UNSET, which triggers the §6.1 PARSE-REJECT on both sides.
func (*drBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refDRPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side. The runner-
// allocated subjAdminPort + subjListenerPort splice into the templates.
func (*drBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject / ProbeAdmin are required by the Driver
// interface but never invoked in the boot-reject branch (the runner SKIPS
// Drive + admin-diff for BootRejectFixture drivers).

func (*drBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*drBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*drBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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
// (the unset `response: {}`) entirely inline in renderBootRejectBootstrap;
// there is NO on-disk script. The runner discards the return value.
func (*drBootRejectDriver) BootRejectScript() string { return "" }

// ExpectedBootErrorSubstring returns the literal substring the runner asserts
// is present (case-sensitive Contains) in BOTH ref + subj stderr.
// Per D-P26.1-4 + the empirical capture at Task 16: "specifier" appears in:
//   - upstream: "...caused by field: \"specifier\", reason: is required"
//   - envoy-go: "...direct_response: response.specifier is required"
func (*drBootRejectDriver) ExpectedBootErrorSubstring() string { return expectedBootErrorSubstr }

// renderBootRejectBootstrap returns the self-contained single-listener
// bootstrap BOTH proxies consume. The direct_response filter's `response: {}`
// has its DataSource specifier oneof UNSET (empty message) — this triggers the
// §6.1 PARSE-REJECT on config-load on both reference Envoy + envoy-go.
//
// A minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared so
// envoy-go's cluster manager runs before the listener manager and does not
// fail with a zero-cluster error before the listener config-load reject. Same
// ordering sidestep as fixture-0033/0041.
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

static_resources:
  listeners:
    - name: l_dr
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.direct_response
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config
                # response PRESENT but its DataSource specifier oneof is UNSET —
                # triggers §6.1 specifier-required PARSE-REJECT on both sides.
                response: {}

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

// Compile-time interface assertions.
var (
	_ fixture.Driver = (*drBootRejectDriver)(nil)
)
