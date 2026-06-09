package redisproxy

import (
	"testing"

	redis_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// settings builds a minimal valid ConnPoolSettings (op_timeout required).
func settings() *redis_proxyv3.RedisProxy_ConnPoolSettings {
	return &redis_proxyv3.RedisProxy_ConnPoolSettings{OpTimeout: durationpb.New(1_000_000_000)}
}

// catchAll builds a prefix_routes with a catch_all_route → cluster.
func catchAll(cluster string) *redis_proxyv3.RedisProxy_PrefixRoutes {
	return &redis_proxyv3.RedisProxy_PrefixRoutes{
		CatchAllRoute: &redis_proxyv3.RedisProxy_PrefixRoutes_Route{Cluster: cluster},
	}
}

func valid() *redis_proxyv3.RedisProxy {
	return &redis_proxyv3.RedisProxy{StatPrefix: "redis_a", Settings: settings(), PrefixRoutes: catchAll("redis_cluster")}
}

func TestParseConfig_StoresFields(t *testing.T) {
	cfg, err := parseConfig(valid())
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.statPrefix != "redis_a" {
		t.Errorf("statPrefix = %q, want redis_a", cfg.statPrefix)
	}
	if cfg.catchAllCluster != "redis_cluster" {
		t.Errorf("catchAllCluster = %q, want redis_cluster", cfg.catchAllCluster)
	}
	if cfg.opTimeout != 1_000_000_000 {
		t.Errorf("opTimeout = %v, want 1s", cfg.opTimeout)
	}
}

func TestParseConfig_StatPrefixRequired(t *testing.T) {
	m := valid()
	m.StatPrefix = ""
	_, err := parseConfig(m)
	if err == nil || err.Error() != errStatPrefixRequired {
		t.Fatalf("err = %v, want %q", err, errStatPrefixRequired)
	}
}

func TestParseConfig_StatPrefixInvalidName(t *testing.T) {
	m := valid()
	m.StatPrefix = "bad prefix!" // not IsValidName → reject at the user-input boundary
	_, err := parseConfig(m)
	if err == nil || err.Error() != errStatPrefixInvalid {
		t.Fatalf("err = %v, want %q", err, errStatPrefixInvalid)
	}
}

func TestParseConfig_SettingsRequired(t *testing.T) {
	m := valid()
	m.Settings = nil
	_, err := parseConfig(m)
	if err == nil || err.Error() != errSettingsRequired {
		t.Fatalf("err = %v, want %q", err, errSettingsRequired)
	}
}

func TestParseConfig_OpTimeoutRequired(t *testing.T) {
	m := valid()
	m.Settings = &redis_proxyv3.RedisProxy_ConnPoolSettings{} // settings present, op_timeout absent
	_, err := parseConfig(m)
	if err == nil || err.Error() != errOpTimeoutRequired {
		t.Fatalf("err = %v, want %q", err, errOpTimeoutRequired)
	}
}

func TestParseConfig_NoUpstream(t *testing.T) {
	// prefix_routes omitted AND prefix_routes: {} both → the runtime no-upstream reject.
	for _, pr := range []*redis_proxyv3.RedisProxy_PrefixRoutes{nil, {}} {
		m := valid()
		m.PrefixRoutes = pr
		_, err := parseConfig(m)
		if err == nil || err.Error() != errNoUpstream {
			t.Fatalf("prefix_routes=%v: err = %v, want %q", pr, err, errNoUpstream)
		}
	}
}

func TestParseConfig_CatchAllClusterRequired(t *testing.T) {
	m := valid()
	m.PrefixRoutes = &redis_proxyv3.RedisProxy_PrefixRoutes{
		CatchAllRoute: &redis_proxyv3.RedisProxy_PrefixRoutes_Route{Cluster: ""}, // present, empty cluster
	}
	_, err := parseConfig(m)
	if err == nil || err.Error() != errCatchAllClusterRequired {
		t.Fatalf("err = %v, want %q", err, errCatchAllClusterRequired)
	}
}

func TestParseConfig_UnknownClusterTolerated(t *testing.T) {
	// AMEND-R2 arm C: an unknown catch_all cluster name does NOT reject at parse —
	// it is resolved lazily at Handle. parseConfig stores the name verbatim.
	m := valid()
	m.PrefixRoutes = catchAll("nonexistent_cluster")
	cfg, err := parseConfig(m)
	if err != nil {
		t.Fatalf("parseConfig must tolerate an unknown cluster at parse: %v", err)
	}
	if cfg.catchAllCluster != "nonexistent_cluster" {
		t.Errorf("catchAllCluster = %q, want nonexistent_cluster", cfg.catchAllCluster)
	}
}

func TestParseRejectConstants_ByteStable(t *testing.T) {
	// ADR-0080 byte-stable wording guard (D-S32.1-3). DO NOT update these to match
	// a code change — a mismatch means the production wording regressed. Every arm
	// carries the "redis_proxy: " prefix (the kafka_broker:/mongo_proxy: precedent).
	want := map[string]string{
		"stat_prefix":         "redis_proxy: stat_prefix is required",
		"stat_prefix_invalid": "redis_proxy: stat_prefix is not a valid metric name",
		"settings":            "redis_proxy: settings: value is required",
		"op_timeout":          "redis_proxy: settings.op_timeout: value is required",
		"no_upstream":         "redis_proxy: cannot configure a redis-proxy without any upstream",
		"catch_all_cluster":   "redis_proxy: catch_all_route.cluster is required",
	}
	got := map[string]string{
		"stat_prefix": errStatPrefixRequired, "stat_prefix_invalid": errStatPrefixInvalid,
		"settings": errSettingsRequired, "op_timeout": errOpTimeoutRequired,
		"no_upstream": errNoUpstream, "catch_all_cluster": errCatchAllClusterRequired,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s arm = %q, want %q", k, got[k], w)
		}
	}
}
