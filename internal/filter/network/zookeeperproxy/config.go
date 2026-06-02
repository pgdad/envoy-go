package zookeeperproxy

import (
	"errors"
	"fmt"
	"time"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
)

// Wire opcodes (upstream decoder.h:30-58; AMEND-A6 — gaps, a negative value,
// and >100 values; NOT the proto's contiguous 0..26 enum).
const (
	opConnect              int32 = 0
	opCreate               int32 = 1
	opDelete               int32 = 2
	opExists               int32 = 3
	opGetData              int32 = 4
	opSetData              int32 = 5
	opGetACL               int32 = 6
	opSetACL               int32 = 7
	opGetChildren          int32 = 8
	opSync                 int32 = 9
	opPing                 int32 = 11
	opGetChildren2         int32 = 12
	opCheck                int32 = 13
	opMulti                int32 = 14
	opCreate2              int32 = 15
	opReconfig             int32 = 16
	opCheckWatches         int32 = 17
	opRemoveWatches        int32 = 18
	opCreateContainer      int32 = 19
	opCreateTTL            int32 = 21
	opClose                int32 = -11
	opSetAuth              int32 = 100
	opSetWatches           int32 = 101
	opGetEphemerals        int32 = 103
	opGetAllChildrenNumber int32 = 104
	opSetWatches2          int32 = 105
	opAddWatch             int32 = 106
)

// protoToWireOpcode maps the proto LatencyThresholdOverride_Opcode enum
// (27 contiguous values 0..26) to the wire opcode (parent SPEC §5.3 + AMEND-A6;
// upstream filter.h:311-339 opcodeMap). Used to key the latency-override map
// (consumed at 28.2) and to validate override opcodes at parse.
var protoToWireOpcode = map[zookeeper_proxyv3.LatencyThresholdOverride_Opcode]int32{
	zookeeper_proxyv3.LatencyThresholdOverride_Connect:              opConnect,
	zookeeper_proxyv3.LatencyThresholdOverride_Create:               opCreate,
	zookeeper_proxyv3.LatencyThresholdOverride_Delete:               opDelete,
	zookeeper_proxyv3.LatencyThresholdOverride_Exists:               opExists,
	zookeeper_proxyv3.LatencyThresholdOverride_GetData:              opGetData,
	zookeeper_proxyv3.LatencyThresholdOverride_SetData:              opSetData,
	zookeeper_proxyv3.LatencyThresholdOverride_GetAcl:               opGetACL,
	zookeeper_proxyv3.LatencyThresholdOverride_SetAcl:               opSetACL,
	zookeeper_proxyv3.LatencyThresholdOverride_GetChildren:          opGetChildren,
	zookeeper_proxyv3.LatencyThresholdOverride_Sync:                 opSync,
	zookeeper_proxyv3.LatencyThresholdOverride_Ping:                 opPing,
	zookeeper_proxyv3.LatencyThresholdOverride_GetChildren2:         opGetChildren2,
	zookeeper_proxyv3.LatencyThresholdOverride_Check:                opCheck,
	zookeeper_proxyv3.LatencyThresholdOverride_Multi:                opMulti,
	zookeeper_proxyv3.LatencyThresholdOverride_Create2:              opCreate2,
	zookeeper_proxyv3.LatencyThresholdOverride_Reconfig:             opReconfig,
	zookeeper_proxyv3.LatencyThresholdOverride_CheckWatches:         opCheckWatches,
	zookeeper_proxyv3.LatencyThresholdOverride_RemoveWatches:        opRemoveWatches,
	zookeeper_proxyv3.LatencyThresholdOverride_CreateContainer:      opCreateContainer,
	zookeeper_proxyv3.LatencyThresholdOverride_CreateTtl:            opCreateTTL,
	zookeeper_proxyv3.LatencyThresholdOverride_Close:                opClose,
	zookeeper_proxyv3.LatencyThresholdOverride_SetAuth:              opSetAuth,
	zookeeper_proxyv3.LatencyThresholdOverride_SetWatches:           opSetWatches,
	zookeeper_proxyv3.LatencyThresholdOverride_GetEphemerals:        opGetEphemerals,
	zookeeper_proxyv3.LatencyThresholdOverride_GetAllChildrenNumber: opGetAllChildrenNumber,
	zookeeper_proxyv3.LatencyThresholdOverride_SetWatches2:          opSetWatches2,
	zookeeper_proxyv3.LatencyThresholdOverride_AddWatch:             opAddWatch,
}

// wireOpcodeToOpname maps wire opcodes to the upstream stats-macro opname
// (parent §7.2). NOTE: SetAuth's opname is "auth" (not "setauth"): upstream
// routes SetAuth request accounting to the auth_* family + the dynamic
// auth.<scheme>_rq counter. The upstream macro DOES declare a setauth_*
// counter family — present in the eager roster (stats.go) but never
// incremented by the filter (live-dump confirmed; AMEND-A3 prose corrected at
// Task 8). connect is NOT here (it has no wire opcode — the decoder
// dispatches it by xid sniffing, AMEND-A5).
var wireOpcodeToOpname = map[int32]string{
	opCreate:               "create",
	opDelete:               "delete",
	opExists:               "exists",
	opGetData:              "getdata",
	opSetData:              "setdata",
	opGetACL:               "getacl",
	opSetACL:               "setacl",
	opGetChildren:          "getchildren",
	opSync:                 "sync",
	opPing:                 "ping",
	opGetChildren2:         "getchildren2",
	opCheck:                "check",
	opMulti:                "multi",
	opCreate2:              "create2",
	opReconfig:             "reconfig",
	opCheckWatches:         "checkwatches",
	opRemoveWatches:        "removewatches",
	opCreateContainer:      "createcontainer",
	opCreateTTL:            "createttl",
	opClose:                "close",
	opSetAuth:              "auth",
	opSetWatches:           "setwatches",
	opGetEphemerals:        "getephemerals",
	opGetAllChildrenNumber: "getallchildrennumber",
	opSetWatches2:          "setwatches2",
	opAddWatch:             "addwatch",
}

// compiledConfig is the boot-parsed, per-listener-shared zookeeper_proxy config
// (ADR-0079 two-step factory). The roster counters (stats.go) attach at factory
// time (NewFactory in zookeeperproxy.go); per-connection state lives on the
// decoder, never here.
type compiledConfig struct {
	statPrefix     string
	maxPacketBytes uint32

	// stats is the eagerly-created 201-counter roster, attached by NewFactory at
	// boot (D-P5 creation parity; ADR-0222 §4.4). All per-connection filter
	// instances share this pointer; counter increments are atomic.
	stats *rosterStats

	// Latency fields: parsed + validated at 28.1; CONSUMED at 28.2 (SPEC §4.3).
	enableLatencyThresholdMetrics bool
	defaultLatencyThreshold       time.Duration
	latencyThresholdOverrides     map[int32]time.Duration // keyed by WIRE opcode

	// Flags consumed at 28.1 (gate increments, never creation — AMEND-A2).
	enablePerOpcodeRequestBytesMetrics bool
	enablePerOpcodeDecoderErrorMetrics bool
	// Parsed at 28.1; consumed at 28.2.
	enablePerOpcodeResponseBytesMetrics bool
}

const (
	defaultMaxPacketBytes         = 1 << 20 // 1 MiB (parent §5.2)
	defaultLatencyThresholdMillis = 100     // 100 ms (parent §5.2)
)

// PARSE-REJECT arms (ADR-0080 byte-stable; SPEC §6; D-S28.1-2). The
// stat-prefix arm is the load-bearing 0047 fixture arm; the latency PGV-mirror
// arms are unit-test-only at 28.1 (their fixture disposition is parent D-P4 = 28.2's).
// These strings are byte-stable from this commit forward — DO NOT CHANGE.
const (
	errStatPrefixRequired               = "zookeeper_proxy: stat_prefix is required"
	errLatencyOverrideThresholdRequired = "zookeeper_proxy: latency_threshold_overrides: threshold is required"
	errLatencyOverrideThresholdTooSmall = "zookeeper_proxy: latency_threshold_overrides: threshold must be at least 1ms"
	errLatencyOverrideOpcodeUndefined   = "zookeeper_proxy: latency_threshold_overrides: opcode is not a defined opcode"
	errDefaultLatencyThresholdTooSmall  = "zookeeper_proxy: default_latency_threshold must be at least 1ms"
	errLatencyOverrideDuplicateOpcode   = "zookeeper_proxy: latency_threshold_overrides: duplicate opcode"
)

// parseConfig validates + compiles the 9-field proto (parent §5.2 roster).
// access_log is parse-accept-IGNORED (upstream parity — completely unread
// upstream; parent §11.2). Returns PARSE-REJECT errors on PGV-mirror violations
// (ADR-0080 byte-stable; SPEC §6; D-S28.1-2).
//
// Validation order: stat_prefix first, then default_latency_threshold, then
// per-override checks (opcode-defined → threshold-required → threshold-too-small
// → duplicate). The opcode-defined check uses protoToWireOpcode membership,
// which covers exactly the 27 defined proto enum values — equivalent to the
// generated _name map and preferred here to keep both membership and mapping in
// one lookup.
func parseConfig(msg *zookeeper_proxyv3.ZooKeeperProxy) (*compiledConfig, error) {
	if msg.GetStatPrefix() == "" {
		return nil, errors.New(errStatPrefixRequired)
	}

	cfg := &compiledConfig{
		statPrefix:                          msg.GetStatPrefix(),
		maxPacketBytes:                      defaultMaxPacketBytes,
		defaultLatencyThreshold:             defaultLatencyThresholdMillis * time.Millisecond,
		latencyThresholdOverrides:           map[int32]time.Duration{},
		enableLatencyThresholdMetrics:       msg.GetEnableLatencyThresholdMetrics(),
		enablePerOpcodeRequestBytesMetrics:  msg.GetEnablePerOpcodeRequestBytesMetrics(),
		enablePerOpcodeResponseBytesMetrics: msg.GetEnablePerOpcodeResponseBytesMetrics(),
		enablePerOpcodeDecoderErrorMetrics:  msg.GetEnablePerOpcodeDecoderErrorMetrics(),
	}
	if msg.GetMaxPacketBytes() != nil {
		cfg.maxPacketBytes = msg.GetMaxPacketBytes().GetValue()
	}
	if msg.GetDefaultLatencyThreshold() != nil {
		cfg.defaultLatencyThreshold = msg.GetDefaultLatencyThreshold().AsDuration()
		if cfg.defaultLatencyThreshold < time.Millisecond {
			return nil, errors.New(errDefaultLatencyThresholdTooSmall)
		}
	}
	for _, o := range msg.GetLatencyThresholdOverrides() {
		// Fail-closed on unknown opcodes: protoToWireOpcode membership is the
		// canonical defined-opcode check (covers the 27 proto enum values).
		wire, defined := protoToWireOpcode[o.GetOpcode()]
		if !defined {
			return nil, fmt.Errorf("%s: %d", errLatencyOverrideOpcodeUndefined, o.GetOpcode())
		}
		if o.GetThreshold() == nil {
			return nil, errors.New(errLatencyOverrideThresholdRequired)
		}
		if o.GetThreshold().AsDuration() < time.Millisecond {
			return nil, errors.New(errLatencyOverrideThresholdTooSmall)
		}
		if _, dup := cfg.latencyThresholdOverrides[wire]; dup {
			return nil, fmt.Errorf("%s: %v", errLatencyOverrideDuplicateOpcode, o.GetOpcode())
		}
		cfg.latencyThresholdOverrides[wire] = o.GetThreshold().AsDuration()
	}
	return cfg, nil
}
