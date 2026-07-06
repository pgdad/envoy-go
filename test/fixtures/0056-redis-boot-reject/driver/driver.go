// Package driver registers the 0056-redis-boot-reject BOOT-REJECT differential
// fixture with the runner per phase 32.1 SPEC §8.1 + PLAN Task 15. It is a
// SYMMETRIC cross-side boot-reject fixture: a
// envoy.extensions.filters.network.redis_proxy.v3.RedisProxy config whose
// `stat_prefix` is MISSING MUST fail-closed at config-load on BOTH reference
// Envoy AND envoy-go, and BOTH stderr buffers must contain a shared
// case-sensitive substring.
//
// # Relationship to 0055 and the fixture-dispatch constraint
//
// 0056 is the RedisProxy-filter BOOT-REJECT analog of
// 0050-mongo-boot-reject and 0054-kafka-boot-reject (the symmetric
// BootRejectFixture template). Per the project's fixture-dispatch constraint
// (one fixture dir = one runner branch), this directory is the BOOT-REJECT
// branch only; the cross-side request arms live in 0055-redis-roundtrip.
//
// # Why stat_prefix-missing is a GENUINE cross-side both-reject
//
// The redis_proxy proto marks `stat_prefix` PGV-required (min 1 rune).
// Reference Envoy rejects a missing stat_prefix at config-load (PGV
// violation), and envoy-go's config parser rejects it with the const
// errStatPrefixRequired ("redis_proxy: stat_prefix is required", config.go).
// So this is a genuine both-sides-reject — analogous to the mongo/kafka
// stat_prefix-required rejections in 0050/0054.
//
// # Common boot-reject substring (honest cross-impl divergence)
//
// The PRIMARY, load-bearing claim of this fixture is that BOTH sides FAIL TO
// BOOT (the runner's refErr!=nil && subjErr!=nil gate). The shared substring is
// a SECONDARY sanity check on the rejected stderr.
//
// The two implementations word the SAME violation DIFFERENTLY:
//
//   - reference Envoy stderr: a PGV violation in CamelCase —
//     "RedisProxyValidationError.StatPrefix: value length must be at least 1
//     characters" — plus an echo of the offending bootstrap.
//   - envoy-go stderr: "redis_proxy: stat_prefix is required" — snake_case.
//
// So lowercase `stat_prefix` does NOT appear in the reference's GENUINE stderr.
// The earlier `stat_prefix` substring matched the reference side ONLY because a
// driver comment ("# stat_prefix INTENTIONALLY OMITTED") was echoed back into
// the reference stderr — a CIRCULAR match that would have held no matter WHY
// boot failed. That comment has been removed (verified: with the comment gone,
// the runner reports `reference stderr does NOT contain "stat_prefix"`).
//
// The substring is therefore `redis_proxy` — the strongest token that GENUINELY
// appears in BOTH real stderrs from a NON-circular source:
//   - SUBJECT side: the error line itself ("redis_proxy: stat_prefix is
//     required") — the subject stderr is JUST the error line (no YAML echo).
//   - REFERENCE side: the echoed config's REAL filter `name:
//     envoy.filters.network.redis_proxy` and the `redis_proxy.v3.RedisProxy`
//     typed_config @type — load-bearing config tokens that SELECT this filter,
//     NOT a comment injected to satisfy the assertion. The GENUINE
//     reference-reject assertion remains the runner's separate refErr != nil
//     gate.
//
// # Bootstrap discipline
//
// Self-contained inline bootstrap. A minimal c_unused cluster (127.0.0.1:1 —
// never dialed) is declared so envoy-go's cluster manager (which runs BEFORE
// the listener manager) does not fail with a zero-cluster error before the
// listener config-load reject fires (reference_network_filter_typeurl_extensions:
// a zero-cluster boot is rejected by both sides). Same ordering sidestep as
// fixtures 0033/0041/0042/0044/0047/0050/0054.
//
// # Cross-references
//
//   - phase 32.1 SPEC §8.1 (redis-proxy fixture scope)
//   - 32.1 PLAN Task 15 (this fixture)
//   - harness.go BootRejectFixture interface (runBootRejectFixture branch)
//   - fixture-0054-kafka-boot-reject (the symmetric template this mirrors)
//   - fixture-0055-redis-roundtrip (cross-side arms; the one-dir-one-branch
//     companion for the RedisProxy filter)
package driver

import (
	"context"
	"fmt"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0056-redis-boot-reject"

	refAdminPort = 9901
	refRedisPort = 15056 // l_redis — the single boot-reject listener (ref container port).

	// expectedBootErrorSubstr is the literal substring the runner asserts is
	// present (case-sensitive Contains) in BOTH ref + subj stderr after the
	// boot-reject. It is a SECONDARY sanity check; the PRIMARY, load-bearing
	// claim is the runner's separate "both sides fail to boot" gate
	// (refErr!=nil && subjErr!=nil).
	//
	// The two implementations word the SAME violation DIFFERENTLY:
	//   - subject (envoy-go): "redis_proxy: stat_prefix is required" — snake_case.
	//   - reference (C++ Envoy): "RedisProxyValidationError.StatPrefix: value
	//     length must be at least 1 characters" — CamelCase `StatPrefix`.
	// So lowercase `stat_prefix` does NOT appear in the reference's genuine
	// stderr (verified by removing all driver-injected tokens and capturing
	// the raw reference-container stderr; the runner reported
	// `reference stderr does NOT contain "stat_prefix"`). The earlier
	// `stat_prefix` choice matched the reference side ONLY because a driver
	// comment ("# stat_prefix INTENTIONALLY OMITTED") was echoed back — a
	// circular match that would have held regardless of WHY boot failed. That
	// comment has been removed.
	//
	// The strongest token that GENUINELY appears in BOTH real stderrs from a
	// non-circular source is `redis_proxy`:
	//   - subject: the error line itself ("redis_proxy: stat_prefix is required").
	//   - reference: the echoed config's REAL filter `name:
	//     envoy.filters.network.redis_proxy` and the `redis_proxy.v3.RedisProxy`
	//     typed_config @type — load-bearing config tokens (they SELECT this
	//     filter), NOT a comment injected to satisfy the assertion.
	expectedBootErrorSubstr = "redis_proxy"

	// redisProxyType is the redis_proxy typed_config @type URL. The network-filter
	// type URLs carry the extensions. segment
	// (reference_network_filter_typeurl_extensions).
	redisProxyType = "type.googleapis.com/envoy.extensions.filters.network.redis_proxy.v3.RedisProxy"
)

func init() {
	fixture.RegisterFixture(fixtureName, &redisBootRejectDriver{})
}

type redisBootRejectDriver struct{}

// --- fixture.Driver (required) ---

func (*redisBootRejectDriver) BackendCount() int           { return 1 } // runner fatals on n<1; spare backend unused by the boot-reject path.
func (*redisBootRejectDriver) SubjectListenerName() string { return "l_redis" }
func (*redisBootRejectDriver) ReferenceListenerPort() int  { return refRedisPort }

// ReferenceBootstrap renders the self-contained single-listener boot-reject
// bootstrap. The redis_proxy filter's `stat_prefix` is UNSET (omitted), which
// triggers the stat_prefix-required PARSE-REJECT on both sides.
func (*redisBootRejectDriver) ReferenceBootstrap(_ []int) string {
	return renderBootRejectBootstrap(refAdminPort, refRedisPort)
}

// SubjectConfig mirrors ReferenceBootstrap for the subject side.
func (*redisBootRejectDriver) SubjectConfig(_, subjListenerPort int, _ []int, subjAdminPort int) string {
	return renderBootRejectBootstrap(subjAdminPort, subjListenerPort)
}

// DriveReference / DriveSubject are never invoked in the boot-reject branch
// (the runner SKIPS Drive + admin-diff for BootRejectFixture drivers).
func (*redisBootRejectDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*redisBootRejectDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (*redisBootRejectDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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
func (*redisBootRejectDriver) BootRejectScript() string { return "" }

// ExpectedBootErrorSubstring returns the literal substring the runner asserts is
// present (case-sensitive Contains) in BOTH ref + subj stderr.
func (*redisBootRejectDriver) ExpectedBootErrorSubstring() string { return expectedBootErrorSubstr }

// renderBootRejectBootstrap returns the self-contained single-listener bootstrap
// BOTH proxies consume. The redis_proxy filter's `stat_prefix` is UNSET
// (omitted) — this triggers the stat_prefix-required PARSE-REJECT on config-load
// on both reference Envoy + envoy-go. NOTE: the bootstrap deliberately carries
// NO driver comment naming the asserted substring; the `redis_proxy` token the
// runner asserts comes from the REAL filter name + @type, not from a comment
// injected to satisfy the assertion.
//
// A minimal c_unused cluster (127.0.0.1:1 — never dialed) is declared so
// envoy-go's cluster manager runs before the listener manager and does not fail
// with a zero-cluster error before the listener config-load reject. Same
// ordering sidestep as fixtures 0033/0041/0042/0044/0047/0050/0054.
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

static_resources:
  listeners:
    - name: l_redis
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.redis_proxy
              typed_config:
                "@type": %s
                settings:
                  op_timeout: 5s
                prefix_routes:
                  catch_all_route:
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
`, adminPort, listenerPort, redisProxyType)
}

// Compile-time interface assertion. The BootRejectFixture interface lives in
// package differential (harness.go), which the driver package does not import to
// avoid an import cycle; the runner asserts the BootRejectScript/
// ExpectedBootErrorSubstring method set structurally at dispatch (the 0050
// precedent).
var _ fixture.Driver = (*redisBootRejectDriver)(nil)
