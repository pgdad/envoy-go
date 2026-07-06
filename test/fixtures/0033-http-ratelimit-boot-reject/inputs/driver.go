// Package inputs registers the 0033-http-ratelimit-boot-reject fixture with
// the differential runner per phase 24.1 SPEC §7.2 + PLAN Task 11 + D-RL4.
// It is a BOOT-REJECT fixture: a `RateLimit` filter config with EMPTY `domain`
// must fail-closed at config-load on BOTH reference Envoy v1.37.2 AND envoy-go.
//
// This exercises the §5.1 arm 1 boot-reject path cross-side:
//   - Reference Envoy v1.37.2 PGV-rejects at config-load with a `Domain` /
//     `min_len: 1` violation (the upstream RateLimit proto pins
//     `validate.rules.string.min_len = 1` on the `domain` field).
//   - envoy-go's compiled_config.go arm 1 rejects at buildCompiledConfig with
//     the byte-stable wording `"ratelimit: domain is required"`
//     (parseRejectDomainRequired const per SPEC §5.1 row 1).
//
// Modeled EXACTLY on fixture-0031-http-admission-control-boot-reject's
// BootRejectFixture mechanism (which itself follows the fixture-0029
// inline-bootstrap precedent). The runner's runBootRejectFixture branch
// (runner_test.go) calls BootRejectScript() once, then renders BOTH bootstraps
// via ReferenceBootstrap + SubjectConfig, starts BOTH proxies via tryStart*,
// asserts BOTH fail to boot, and asserts a common substring
// (ExpectedBootErrorSubstring()) appears in BOTH stderr buffers.
//
// # Common boot-reject substring (D-RL4)
//
// Per the empirical capture finalized at Task 11:
//   - reference Envoy v1.37.2 stderr (PGV violation):
//     "Proto constraint validation failed
//     (RateLimitValidationError.Domain: value length must be at least 1 characters)"
//   - envoy-go stderr (parseRejectDomainRequired):
//     "listener manager: listener: 'l_test_a': filter_chains[0]: hcm:
//     http_filters[0]: factory: ratelimit: domain is required"
//
// The two load-bearing wordings DIFFER in case on the field name: upstream
// emits the PGV-generated `Domain` (capital D, matching the proto field
// camel-case name as used in the Go-protoc validation error name), while
// envoy-go emits the lowercase wire name `domain` (matching the proto
// field's lowercase wire identifier). Because the runner's substring
// assertion is case-sensitive (`strings.Contains`), the common substring
// is the 5-character fragment `omain` — present in both `Domain`
// (capital D) and `domain` (lowercase d). This is the distinctive
// substring finalized for D-RL4.
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap (Option B2 per fixture-0029 / 0031
// precedent): the empty `domain` is embedded directly in the bootstrap
// rendered by renderBootRejectBootstrap. No host-mount or file reference
// is needed. The runner's tryStartReferenceProxy does NOT consult
// ReferenceLogMounter, so inline bootstraps are the correct approach.
//
// A minimal upstream cluster (c_unused; 127.0.0.1:1 — never dialed) is
// declared so envoy-go's cluster manager (which runs BEFORE the listener
// manager) does not fail with a zero-endpoint error before the listener-
// manager config-load reject fires. Same ordering sidestep as fixture-0026
// / 0029 / 0031.
//
// A minimal-but-syntactically-valid RLS cluster (c_ratelimit at 127.0.0.1:1)
// with the mandatory http2_protocol_options:{} is declared and referenced
// from the filter's rate_limit_service.envoy_grpc.cluster_name. This ensures
// the PGV / envoy-go parse path proceeds PAST the rate_limit_service-shape
// arms (which fire AFTER `domain` in envoy-go's order per compiled_config.go,
// but are also independently checked by upstream PGV — providing a complete
// rls cluster reference removes any ambiguity about which arm fired). The
// cluster is never dialed because the boot-reject fires at config-load,
// strictly before any request reaches a listener binding.
//
// # Cross-references
//
//   - parent SPEC §7.2 (boot-reject fixture scope)
//   - parent SPEC §5.1 row 1 (domain empty PARSE-REJECT; byte-stable wording)
//   - 24.1 PLAN Task 11 + D-RL4 (boot-reject common stderr substring)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - fixture-0031 (nearest BootRejectFixture precedent; admission_control)
//   - fixture-0029 (lua source_codes BootRejectFixture precedent)
//   - fixture-0026 (original lua BootRejectFixture precedent)
package inputs

import (
	"context"
	"fmt"
	"sync"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0033-http-ratelimit-boot-reject"

	refAdminPort  = 9901
	refLATestPort = 10133 // l_test_a — the single boot-reject listener.

	// BootRejectScript() return value. UNLIKE fixture-0029 (whose return
	// value names a real on-disk symmetry artifact, scripts/bad_compile.lua),
	// this fixture embeds the boot-reject trigger entirely inline in
	// renderBootRejectBootstrap (the filter's `domain` field is omitted /
	// empty) — there is NO on-disk script file. This constant is therefore
	// a description, not a filesystem path: the runner discards the return
	// value, and the side effect (flipping bootRejectMode) is the meaningful
	// signal. Mirrors fixture-0031's bootRejectScriptDesc shape.
	bootRejectScriptDesc = "inline ratelimit filter with empty `domain` (§5.1 arm 1)"

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr after
	// boot-reject. Finalized empirically at Task 11 per D-RL4:
	//
	//   reference Envoy v1.37.2 stderr (PGV violation; load-bearing wording):
	//     "Proto constraint validation failed
	//     (RateLimitValidationError.Domain: value length must be at least 1 characters)"
	//   envoy-go stderr (parseRejectDomainRequired; load-bearing wording):
	//     "listener manager: listener: 'l_test_a': filter_chains[0]: hcm:
	//     http_filters[0]: factory: ratelimit: domain is required"
	//
	// The two load-bearing wordings DIFFER in case on the field name:
	// upstream emits `Domain` (capital D — matches the proto field
	// camel-case name used by Go-protoc's PGV validation error type);
	// envoy-go emits `domain` (lowercase — matches the proto field's wire
	// name as named in §5.1 row 1's byte-stable wording).
	//
	// Because the runner's substring assertion is case-sensitive
	// (`strings.Contains`), neither `Domain` nor `domain` is a SHARED
	// substring across the two load-bearing wordings alone. The 5-character
	// fragment `omain` IS shared:
	//   - upstream: "RateLimitValidationError.Domain" → contains "omain"
	//   - envoy-go: "ratelimit: domain is required" → contains "omain"
	//
	// `omain` is the distinctive substring finalized for D-RL4: it is a
	// case-insensitive-equivalent fragment of the field name `domain`
	// (the only token both implementations share in the load-bearing
	// portion of their respective wordings), and it does not collide with
	// any unrelated token in either stderr.
	//
	// This is the same byte-stable cross-side substring discipline as
	// fixture-0031 ("cannot be less than 1.0%"), fixture-0029 ("near '-'"),
	// and fixture-0026 ("script load error"). Per AMEND-10 option 2: the
	// substring need only appear ANYWHERE in stderr (not be a prefix /
	// regex / case-insensitive match).
	expectedBootErrorSubstr = "omain"
)

func init() {
	fixture.RegisterFixture(fixtureName, &rlBootRejectDriver{})
}

// rlBootRejectDriver carries the boot-reject mode flag (flipped when the
// runner's runBootRejectFixture branch calls BootRejectScript() before
// re-rendering the bootstrap templates). Mirrors fixture-0031's
// acBootRejectDriver shape.
type rlBootRejectDriver struct {
	mu             sync.Mutex
	bootRejectMode bool
}

// --- fixture.Driver (required) ---

func (*rlBootRejectDriver) BackendCount() int                { return 1 }
func (*rlBootRejectDriver) BackendKind() fixture.BackendKind { return fixture.HTTPGlobalRateLimitGRPC }
func (*rlBootRejectDriver) SubjectListenerName() string      { return "l_test_a" }
func (*rlBootRejectDriver) ReferenceListenerPort() int       { return refLATestPort }

// ReferenceBootstrap returns the self-contained single-listener boot-reject
// bootstrap once the runner has flipped bootRejectMode via BootRejectScript().
// The filter's `domain` is empty (omitted) which triggers the §5.1 arm 1
// PARSE-REJECT on both sides.
func (d *rlBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refLATestPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side. The runner-
// allocated subjAdminPort splices into the admin socket address.
func (d *rlBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject / ProbeAdmin are required by the Driver
// interface but never invoked in the boot-reject branch (the runner SKIPS
// Drive + admin-diff for BootRejectFixture drivers).

func (*rlBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*rlBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*rlBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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

// BootRejectScript flips bootRejectMode and returns a description of the
// inline boot-reject trigger (NOT a filesystem path — this fixture has no
// on-disk script). The runner discards the return value; the side effect is
// the signal.
func (d *rlBootRejectDriver) BootRejectScript() string {
	d.mu.Lock()
	d.bootRejectMode = true
	d.mu.Unlock()
	return bootRejectScriptDesc
}

// ExpectedBootErrorSubstring returns the literal substring the runner asserts
// is present (case-sensitive Contains) in BOTH ref + subj stderr.
// Per D-RL4 + the empirical capture at Task 11: "omain" appears in:
//   - upstream: "RateLimitValidationError.Domain: value length must be at least 1 characters"
//     (capital `Domain` → contains "omain")
//   - envoy-go: "ratelimit: domain is required"
//     (lowercase `domain` → contains "omain")
func (*rlBootRejectDriver) ExpectedBootErrorSubstring() string { return expectedBootErrorSubstr }

// renderBootRejectBootstrap returns the self-contained single-listener
// bootstrap BOTH proxies consume. The ratelimit filter omits the `domain`
// field (empty string default per proto3) — this triggers §5.1 arm 1
// PARSE-REJECT on config-load on both reference Envoy + envoy-go.
//
// A minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared so
// envoy-go's cluster manager runs before the listener manager and does not
// fail with a zero-endpoint error before the listener config-load reject.
// Same ordering sidestep as fixture-0026 / 0029 / 0031.
//
// A minimal-but-syntactically-valid c_ratelimit cluster (127.0.0.1:1 with
// http2_protocol_options:{}) is also declared and referenced from the
// filter's rate_limit_service.envoy_grpc.cluster_name. This ensures the
// PGV / envoy-go parse path proceeds PAST the rate_limit_service-shape
// arms; the boot-reject is therefore unambiguously attributable to the
// empty `domain` field (§5.1 arm 1). The cluster is never dialed because
// the boot-reject fires at config-load, strictly before any listener
// binds.
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

static_resources:
  listeners:
    - name: l_test_a
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: hcm_bootreject
                route_config:
                  name: rc_bootreject
                  virtual_hosts:
                    - name: vh_bootreject
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_unused }
                http_filters:
                  - name: envoy.filters.http.ratelimit
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit
                      # domain field intentionally OMITTED (empty string) —
                      # triggers §5.1 arm 1 PARSE-REJECT on both sides.
                      rate_limit_service:
                        grpc_service:
                          envoy_grpc:
                            cluster_name: c_ratelimit
                          timeout: 1s
                        transport_api_version: V3
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
    - name: c_ratelimit
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      typed_extension_protocol_options:
        envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
          "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
          explicit_http_config:
            http2_protocol_options: {}
      load_assignment:
        cluster_name: c_ratelimit
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, listenerPort)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*rlBootRejectDriver)(nil)
	_ fixture.BackendKindAware = (*rlBootRejectDriver)(nil)
)
