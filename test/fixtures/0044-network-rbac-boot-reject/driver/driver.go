// Package driver registers the 0044-network-rbac-boot-reject BOOT-REJECT
// differential fixture with the runner per phase 26.3 SPEC §8 + §10 + PLAN
// Task 14. It is a SYMMETRIC cross-side boot-reject fixture: an
// envoy.extensions.filters.network.rbac.v3.RBAC config whose `stat_prefix` is
// MISSING (the PGV-required `(validate.rules).string.min_len = 1` field, unset)
// MUST fail-closed at config-load on BOTH reference Envoy v1.37.2 AND envoy-go,
// and BOTH stderr buffers must contain a shared case-sensitive substring.
//
// # Why stat_prefix-missing is a GENUINE cross-side both-reject
//
// The network RBAC proto marks `stat_prefix` PGV-required:
// `rbac.pb.validate.go:178` — `if utf8.RuneCountInString(m.GetStatPrefix()) < 1`
// → RBACValidationError{field: "StatPrefix", reason: "value length must be at
// least 1 runes"} (the go-control-plane Go validator wording). This is the
// `(validate.rules).string = {min_len: 1}` annotation on the field. Reference
// Envoy v1.37.2 (C++ proto with the same PGV annotation) ALSO rejects a missing
// stat_prefix at config-load — its C++ stderr wording is
// "RBACValidationError.StatPrefix: value length must be at least 1 characters"
// (captured live at Task 14). So this is a
// genuine PGV-mirror both-sides-reject (SPEC §10 lists
// `rbac-network-stat-prefix-required` as boot-reject PARITY) — distinct from the
// envoy-go-strict-only rejects (HTTP-only matcher arm / delay_deny / invalid
// stat_prefix) which upstream silently ACCEPTS (the unknown extension field is
// dropped by upstream's protobuf parser) and which are therefore subject-side-
// only rejects covered by the Task-8/Task-13 unit tests, NOT cross-side fixtures
// (reference_differential_fixture_dispatch_constraint).
//
// # Common boot-reject substring (empirically pinned at Task 14)
//
// The two implementations surface the SAME violation with DIFFERENT wordings
// (captured live, dockerized v1.37.2):
//
//   - reference Envoy v1.37.2 stderr (PGV violation; PascalCase field name):
//     "Proto constraint validation failed (RBACValidationError.StatPrefix:
//     value length must be at least 1 characters)"
//   - envoy-go stderr (parseRejectStatPrefixRequired const at rbac.go:44;
//     snake_case field name): "rbac_network: stat_prefix is required"
//
// IMPORTANT — the two ERROR wordings share NO distinctive case-sensitive token
// (ref names the field PascalCase `StatPrefix`; envoy-go names it snake_case
// `stat_prefix`; the longest common case-sensitive substring of the two error
// lines is the non-distinctive 5-char `refix`). This is the honest empirical
// finding recorded in PROGRESS.md Task 14: BOTH sides genuinely boot-REJECT a
// missing stat_prefix (a real PGV-mirror cross-side both-reject), but the
// error wordings are not byte-comparable.
//
// The runner's substring discipline (AMEND-10 option 2, case-sensitive
// strings.Contains anywhere in each stderr) is satisfied by `stat_prefix`:
//   - SUBJECT side (the side under test): `stat_prefix` is the envoy-go error
//     wording itself — the subject stderr is JUST the 126-byte error line (it
//     does NOT echo the bootstrap YAML), so this match is fully load-bearing
//     (a deliberate-break to PascalCase `StatPrefix` FAILS the subject — proven
//     at Task 14, recorded in PROGRESS.md).
//   - REFERENCE side: reference Envoy echoes the offending bootstrap into its
//     stderr, so `stat_prefix` (the tcp_proxy filter's required field in the
//     rejected config) appears there; the GENUINE reference-reject assertion is
//     the runner's separate `refErr != nil` gate (the runner FATALS if the
//     reference boots cleanly), which the live run confirms fires on the
//     `RBACValidationError.StatPrefix` PGV violation.
//
// Both sides rejecting is the cross-side parity claim; the per-side substring
// pins the envoy-go wording. See PROGRESS.md Task 14 for the full capture +
// the deliberate-break proof.
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap (Option B precedent from fixtures 0033/0042):
// the stat_prefix-missing rbac_network config is embedded directly in the
// rendered bootstrap. A minimal c_unused cluster (127.0.0.1:1 — never dialed) is
// declared so envoy-go's cluster manager (which runs BEFORE the listener
// manager) does not fail with a zero-cluster error before the listener-manager
// config-load reject fires on the rbac_network filter. Same ordering sidestep as
// fixtures 0033/0041/0042.
//
// # Cross-references
//
//   - parent SPEC §8 + §10 (boot-reject network-rbac fixture scope; PGV-mirror)
//   - 26.3 PLAN Task 14 (boot-reject common stderr substring)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - fixture-0042-network-direct-response-boot-reject (nearest network
//     BootRejectFixture precedent — the `specifier`-substring shape this mirrors)
package driver

import (
	"context"
	"fmt"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0044-network-rbac-boot-reject"

	refAdminPort = 9901
	refRBACPort  = 15046 // l_rbac — the single boot-reject listener (ref container port).

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr after the
	// boot-reject. `stat_prefix` is the envoy-go error wording verbatim
	// ("rbac_network: stat_prefix is required") — load-bearing on the subject
	// side (the subject stderr is JUST the error line, no YAML echo; a
	// deliberate-break to "StatPrefix" FAILS the subject, proven at Task 14).
	// The reference side uses PascalCase "StatPrefix" in its PGV error and ALSO
	// echoes the offending bootstrap, so `stat_prefix` matches the reference
	// stderr via the echoed config; the genuine reference-reject is the runner's
	// separate refErr!=nil gate. See the package doc + PROGRESS.md Task 14.
	expectedBootErrorSubstr = "stat_prefix"
)

func init() {
	fixture.RegisterFixture(fixtureName, &rbacBootRejectDriver{})
}

type rbacBootRejectDriver struct{}

// --- fixture.Driver (required) ---

func (*rbacBootRejectDriver) BackendCount() int           { return 1 } // runner fatals on n<1; spare TCP-echo backend unused by the boot-reject path.
func (*rbacBootRejectDriver) SubjectListenerName() string { return "l_rbac" }
func (*rbacBootRejectDriver) ReferenceListenerPort() int  { return refRBACPort }

// ReferenceBootstrap renders the self-contained single-listener boot-reject
// bootstrap. The rbac_network filter's `stat_prefix` is UNSET (omitted), which
// triggers the PGV min_len=1 PARSE-REJECT on both sides.
func (*rbacBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refRBACPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side.
func (*rbacBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject are never invoked in the boot-reject branch
// (the runner SKIPS Drive + admin-diff for BootRejectFixture drivers).
func (*rbacBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*rbacBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*rbacBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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
func (*rbacBootRejectDriver) BootRejectScript() string { return "" }

// ExpectedBootErrorSubstring returns the literal substring the runner asserts is
// present (case-sensitive Contains) in BOTH ref + subj stderr.
func (*rbacBootRejectDriver) ExpectedBootErrorSubstring() string { return expectedBootErrorSubstr }

// renderBootRejectBootstrap returns the self-contained single-listener bootstrap
// BOTH proxies consume. The rbac_network filter's `stat_prefix` is UNSET
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
    - name: l_rbac
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.rbac
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.rbac.v3.RBAC
                # stat_prefix INTENTIONALLY OMITTED — PGV min_len=1 violation
                # triggers the stat_prefix-required PARSE-REJECT on both sides.
                rules:
                  action: ALLOW
                  policies:
                    p_any:
                      permissions:
                        - any: true
                      principals:
                        - any: true
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
var _ fixture.Driver = (*rbacBootRejectDriver)(nil)
