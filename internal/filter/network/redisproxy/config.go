package redisproxy

import (
	"errors"
	"time"

	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"

	"github.com/pgdad/envoy-go/internal/stats"
)

// PARSE-REJECT arms (ADR-0080 byte-stable; SPEC §6; D-S32.1-3). The error prefix
// for all redisproxy arms is "redis_proxy: " (mirrors kafka_broker: / mongo_proxy:
// / zookeeper_proxy:). These strings are byte-stable from this commit forward — DO
// NOT CHANGE. Only errStatPrefixRequired is fixture-proven (the 0056 arm, §6.1);
// the rest are unit-test-only at 32.1 (D-P32-5; the kafka 0054 / mongo 0050
// precedent). The boot-reject differential matches a per-side stderr SUBSTRING,
// not exact cross-impl equality (§6).
const (
	errStatPrefixRequired      = "redis_proxy: stat_prefix is required"
	errStatPrefixInvalid       = "redis_proxy: stat_prefix is not a valid metric name"
	errSettingsRequired        = "redis_proxy: settings: value is required"
	errOpTimeoutRequired       = "redis_proxy: settings.op_timeout: value is required"
	errNoUpstream              = "redis_proxy: cannot configure a redis-proxy without any upstream"
	errCatchAllClusterRequired = "redis_proxy: catch_all_route.cluster is required"
)

// compiledConfig is the boot-parsed, per-listener-shared redis_proxy config
// (ADR-0079 two-step factory). The roster (stats.go) is NOT here — it attaches to
// the filter struct at NewFactory (filter.go). The catch_all cluster is stored by
// NAME (resolved lazily at Handle, tolerant of an unknown cluster — §3.3).
type compiledConfig struct {
	statPrefix      string
	catchAllCluster string        // the catch_all_route.cluster name (lazy-resolved at Handle)
	opTimeout       time.Duration // parsed + stored; CONSUMPTION (bounding the round-trip) is 32.2
}

// parseConfig validates + compiles the proto (SPEC §5 roster, inherited). PGV
// mirrors: stat_prefix required + IsValidName-guarded; settings required;
// settings.op_timeout required; catch_all_route.cluster required when present. A
// runtime reject fires when NEITHER catch_all_route NOR routes[] supplies an
// upstream. An unknown catch_all cluster is TOLERATED (AMEND-R2 arm C). All
// deferred fields (faults, downstream_auth_*, routes[], read_policy, the rest of
// ConnPoolSettings) parse-accept standalone (SPEC §6.3). Pure — no registry.
func parseConfig(msg *redis_proxyv3.RedisProxy) (*compiledConfig, error) {
	sp := msg.GetStatPrefix()
	if sp == "" {
		return nil, errors.New(errStatPrefixRequired)
	}
	if !stats.IsValidName(sp) {
		// the cluster manager.go:205 / reference_dynamic_stat_name_charset_guard
		// precedent: a metric-name-invalid prefix → reject at the user-input
		// boundary (NewCounterIfAbsent would otherwise PANIC at NewFactory).
		return nil, errors.New(errStatPrefixInvalid)
	}
	s := msg.GetSettings()
	if s == nil {
		return nil, errors.New(errSettingsRequired)
	}
	ot := s.GetOpTimeout()
	if ot == nil {
		return nil, errors.New(errOpTimeoutRequired)
	}
	cluster, err := upstreamCluster(msg.GetPrefixRoutes())
	if err != nil {
		return nil, err
	}
	return &compiledConfig{statPrefix: sp, catchAllCluster: cluster, opTimeout: ot.AsDuration()}, nil
}

// upstreamCluster extracts the catch_all_route.cluster name. Returns errNoUpstream
// when prefix_routes is omitted/empty (neither catch_all_route nor routes[]
// supplies an upstream) and errCatchAllClusterRequired when a catch_all_route is
// present with an empty cluster. routes[]-only configs are 32.2 (SPEC §2.4);
// at 32.1 only catch_all_route supplies the upstream.
func upstreamCluster(pr *redis_proxyv3.RedisProxy_PrefixRoutes) (string, error) {
	if pr == nil || (pr.GetCatchAllRoute() == nil && len(pr.GetRoutes()) == 0) {
		return "", errors.New(errNoUpstream)
	}
	car := pr.GetCatchAllRoute()
	if car == nil {
		// routes[]-only: parse-accepted but unconsumed at 32.1; with no
		// catch_all there is no 32.1 upstream → treat as no-upstream.
		return "", errors.New(errNoUpstream)
	}
	if car.GetCluster() == "" {
		return "", errors.New(errCatchAllClusterRequired)
	}
	return car.GetCluster(), nil
}
