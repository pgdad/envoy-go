package zookeeperproxy

import (
	"strings"
	"testing"
	"time"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestParseConfig_AllFieldsAndDefaults(t *testing.T) {
	// Minimal config: only stat_prefix → all defaults.
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk"})
	if err != nil {
		t.Fatalf("parseConfig minimal: %v", err)
	}
	if cfg.statPrefix != "zk" {
		t.Errorf("statPrefix = %q, want zk", cfg.statPrefix)
	}
	if cfg.maxPacketBytes != 1024*1024 {
		t.Errorf("maxPacketBytes = %d, want 1 MiB default (parent §5.2)", cfg.maxPacketBytes)
	}
	if cfg.defaultLatencyThreshold != 100*time.Millisecond {
		t.Errorf("defaultLatencyThreshold = %v, want 100ms default", cfg.defaultLatencyThreshold)
	}
	if cfg.enableLatencyThresholdMetrics || cfg.enablePerOpcodeRequestBytesMetrics ||
		cfg.enablePerOpcodeResponseBytesMetrics || cfg.enablePerOpcodeDecoderErrorMetrics {
		t.Error("all enable_* flags must default false")
	}

	// Full config: every field set.
	full, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
		StatPrefix:                          "zk2",
		AccessLog:                           "/dev/null", // parse-accept-IGNORE (upstream parity)
		MaxPacketBytes:                      wrapperspb.UInt32(512),
		EnableLatencyThresholdMetrics:       true,
		DefaultLatencyThreshold:             durationpb.New(250 * time.Millisecond),
		EnablePerOpcodeRequestBytesMetrics:  true,
		EnablePerOpcodeResponseBytesMetrics: true,
		EnablePerOpcodeDecoderErrorMetrics:  true,
		LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
			{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping, Threshold: durationpb.New(5 * time.Millisecond)},
		},
	})
	if err != nil {
		t.Fatalf("parseConfig full: %v", err)
	}
	if full.maxPacketBytes != 512 || full.defaultLatencyThreshold != 250*time.Millisecond {
		t.Error("explicit max_packet_bytes / default_latency_threshold not honored")
	}
	// The override map is keyed by WIRE opcode (proto Ping=10 → wire 11; AMEND-A6).
	if got, ok := full.latencyThresholdOverrides[opPing]; !ok || got != 5*time.Millisecond {
		t.Errorf("override[wire opPing=11] = (%v, %v), want (5ms, true)", got, ok)
	}
}

// max_packet_bytes accepts ANY uint32 including 0 (no PGV bound; 0 → every
// packet oversized → decoder_error at decode time; upstream parity).
func TestParseConfig_MaxPacketBytesZeroAccepted(t *testing.T) {
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk", MaxPacketBytes: wrapperspb.UInt32(0)})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.maxPacketBytes != 0 {
		t.Errorf("maxPacketBytes = %d, want 0 (explicitly-set zero is honored, not defaulted)", cfg.maxPacketBytes)
	}
}

// The proto→wire opcode mapping — byte-stable against the parent §5.3 + §11.4
// pinned values. Spot-pins every divergent value (the gaps + negatives + >100s).
func TestProtoToWireOpcodeMapping(t *testing.T) {
	want := map[zookeeper_proxyv3.LatencyThresholdOverride_Opcode]int32{
		zookeeper_proxyv3.LatencyThresholdOverride_Connect:              0,
		zookeeper_proxyv3.LatencyThresholdOverride_Create:               1,
		zookeeper_proxyv3.LatencyThresholdOverride_Delete:               2,
		zookeeper_proxyv3.LatencyThresholdOverride_Exists:               3,
		zookeeper_proxyv3.LatencyThresholdOverride_GetData:              4,
		zookeeper_proxyv3.LatencyThresholdOverride_SetData:              5,
		zookeeper_proxyv3.LatencyThresholdOverride_GetAcl:               6,
		zookeeper_proxyv3.LatencyThresholdOverride_SetAcl:               7,
		zookeeper_proxyv3.LatencyThresholdOverride_GetChildren:          8,
		zookeeper_proxyv3.LatencyThresholdOverride_Sync:                 9,
		zookeeper_proxyv3.LatencyThresholdOverride_Ping:                 11, // proto 10 → wire 11 (the gap)
		zookeeper_proxyv3.LatencyThresholdOverride_GetChildren2:         12,
		zookeeper_proxyv3.LatencyThresholdOverride_Check:                13,
		zookeeper_proxyv3.LatencyThresholdOverride_Multi:                14,
		zookeeper_proxyv3.LatencyThresholdOverride_Create2:              15,
		zookeeper_proxyv3.LatencyThresholdOverride_Reconfig:             16,
		zookeeper_proxyv3.LatencyThresholdOverride_CheckWatches:         17,
		zookeeper_proxyv3.LatencyThresholdOverride_RemoveWatches:        18,
		zookeeper_proxyv3.LatencyThresholdOverride_CreateContainer:      19,
		zookeeper_proxyv3.LatencyThresholdOverride_CreateTtl:            21,  // proto 19 → wire 21 (the gap at 20)
		zookeeper_proxyv3.LatencyThresholdOverride_Close:                -11, // proto 20 → wire −11
		zookeeper_proxyv3.LatencyThresholdOverride_SetAuth:              100, // proto 21 → wire 100
		zookeeper_proxyv3.LatencyThresholdOverride_SetWatches:           101,
		zookeeper_proxyv3.LatencyThresholdOverride_GetEphemerals:        103,
		zookeeper_proxyv3.LatencyThresholdOverride_GetAllChildrenNumber: 104,
		zookeeper_proxyv3.LatencyThresholdOverride_SetWatches2:          105,
		zookeeper_proxyv3.LatencyThresholdOverride_AddWatch:             106,
	}
	if len(protoToWireOpcode) != 27 {
		t.Fatalf("protoToWireOpcode has %d entries, want 27 (the proto enum is 27 contiguous values)", len(protoToWireOpcode))
	}
	for proto, wire := range want {
		if got := protoToWireOpcode[proto]; got != wire {
			t.Errorf("protoToWireOpcode[%v] = %d, want %d", proto, got, wire)
		}
	}
}

// The wire-opcode→opname table: digit-suffixed names intact + the SetAuth→auth
// aliasing (there are no setauth_* counters; AMEND-A3).
func TestWireOpcodeToOpname(t *testing.T) {
	cases := map[int32]string{
		opGetData:              "getdata",
		opCreate2:              "create2",
		opGetChildren2:         "getchildren2",
		opSetWatches2:          "setwatches2",
		opGetAllChildrenNumber: "getallchildrennumber",
		opClose:                "close",
		opSetAuth:              "auth", // SetAuth's opname is auth (no setauth_* counters)
		opPing:                 "ping",
	}
	for opcode, want := range cases {
		if got := wireOpcodeToOpname[opcode]; got != want {
			t.Errorf("wireOpcodeToOpname[%d] = %q, want %q", opcode, got, want)
		}
	}
}

// ADR-0080 byte-stable PARSE-REJECT discipline: every arm's wording is a named
// constant pinned by this table test. Prefix: "zookeeper_proxy: " (SPEC §6;
// D-S28.1-2 finalized HERE — these strings are byte-stable from this commit on).
func TestParseRejectConstants_ByteStable(t *testing.T) {
	cases := []struct{ name, constant, want string }{
		{"stat-prefix-required", errStatPrefixRequired, "zookeeper_proxy: stat_prefix is required"},
		{"latency-override-threshold-required", errLatencyOverrideThresholdRequired, "zookeeper_proxy: latency_threshold_overrides: threshold is required"},
		{"latency-override-threshold-too-small", errLatencyOverrideThresholdTooSmall, "zookeeper_proxy: latency_threshold_overrides: threshold must be at least 1ms"},
		{"latency-override-opcode-undefined", errLatencyOverrideOpcodeUndefined, "zookeeper_proxy: latency_threshold_overrides: opcode is not a defined opcode"},
		{"default-latency-threshold-too-small", errDefaultLatencyThresholdTooSmall, "zookeeper_proxy: default_latency_threshold must be at least 1ms"},
		{"latency-override-duplicate-opcode", errLatencyOverrideDuplicateOpcode, "zookeeper_proxy: latency_threshold_overrides: duplicate opcode"},
	}
	for _, tc := range cases {
		if tc.constant != tc.want {
			t.Errorf("%s = %q, want %q (byte-stable; ADR-0080)", tc.name, tc.constant, tc.want)
		}
	}
}

// Each PGV-mirror arm fires (SPEC §6.1 + §6.2; the parse code lands at 28.1,
// the latency arms' FIXTURE disposition is parent D-P4 = 28.2's).
func TestParseConfig_RejectArms(t *testing.T) {
	ms := func(d time.Duration) *durationpb.Duration { return durationpb.New(d) }
	cases := []struct {
		name    string
		msg     *zookeeper_proxyv3.ZooKeeperProxy
		wantErr string
	}{
		{"missing stat_prefix", &zookeeper_proxyv3.ZooKeeperProxy{}, errStatPrefixRequired},
		{"empty stat_prefix", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: ""}, errStatPrefixRequired},
		{"override threshold missing", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
				{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping}}}, errLatencyOverrideThresholdRequired},
		{"override threshold below 1ms", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
				{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping, Threshold: ms(500 * time.Microsecond)}}}, errLatencyOverrideThresholdTooSmall},
		{"override opcode undefined", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
				{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Opcode(999), Threshold: ms(5 * time.Millisecond)}}}, errLatencyOverrideOpcodeUndefined},
		{"default threshold below 1ms", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			DefaultLatencyThreshold: ms(500 * time.Microsecond)}, errDefaultLatencyThresholdTooSmall},
		{"duplicate override opcode", &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
				{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping, Threshold: ms(5 * time.Millisecond)},
				{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping, Threshold: ms(7 * time.Millisecond)},
			}}, errLatencyOverrideDuplicateOpcode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfig(tc.msg)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("parseConfig() err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// 1ms exactly is ACCEPTED (PGV gte = inclusive).
func TestParseConfig_OneMillisecondAccepted(t *testing.T) {
	_, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
		DefaultLatencyThreshold: durationpb.New(time.Millisecond),
		LatencyThresholdOverrides: []*zookeeper_proxyv3.LatencyThresholdOverride{
			{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_Ping, Threshold: durationpb.New(time.Millisecond)}}})
	if err != nil {
		t.Fatalf("parseConfig(1ms thresholds) = %v, want nil (gte is inclusive)", err)
	}
}
