package mongoproxy

import (
	"errors"
	"time"

	commonfaultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/fault/v3"
	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
)

// PARSE-REJECT arms (ADR-0080 byte-stable; SPEC §6; D-S29.1-1). The error prefix
// for all mongoproxy arms is "mongo_proxy: " (mirrors zookeeper_proxy: —
// zookeeperproxy/config.go:148-155). These strings are byte-stable from this
// commit forward — DO NOT CHANGE. The stat_prefix arm is the load-bearing 0050
// fixture arm; the three FaultDelay arms are unit-test-only at 29.1 (their
// fixture disposition is parent D-P5, resolved at the 29.3 SPEC).
const (
	errStatPrefixRequired      = "mongo_proxy: stat_prefix is required"
	errDelaySpecifierRequired  = "mongo_proxy: delay: a delay type must be specified"
	errDelayFixedDelayTooSmall = "mongo_proxy: delay: fixed_delay must be greater than 0s"
	errDelayDenominatorInvalid = "mongo_proxy: delay: percentage denominator is not a defined value"
)

// defaultCommands is the AMEND-B7 default remembered-set when the proto commands
// list is empty: {delete, insert, update}. An explicit list REPLACES this.
var defaultCommands = []string{"delete", "insert", "update"}

// commandAliases normalizes DECODED wire command names before the remembered-set
// lookup (AMEND-B7 / parent §11.3; upstream utility.cc:21-37 normalizes at decode
// time, NOT the configured commands entries). "find" maps to "" — find is handled
// as a query (the collection path), never a cmd.* stat.
var commandAliases = map[string]string{
	"collstats":     "collStats",
	"dbstats":       "dbStats",
	"findandmodify": "findAndModify",
	"getlasterror":  "getLastError",
	"ismaster":      "isMaster",
	"find":          "",
}

// normalizeCommand applies commandAliases to a decoded wire command name. A name
// with no alias entry is returned unchanged. "find" → "" (query path).
func normalizeCommand(name string) string {
	if canon, ok := commandAliases[name]; ok {
		return canon
	}
	return name
}

// compiledConfig is the boot-parsed, per-listener-shared mongo_proxy config
// (ADR-0079 two-step factory). The roster (stats.go) attaches at factory time
// (NewFactory in filter.go); per-connection state lives on the decoder, never here.
type compiledConfig struct {
	statPrefix string

	// stats is the eagerly-created 23-stat roster (22 counters + the gauge),
	// attached by NewFactory at boot (D-P1 eager creation). All per-connection
	// filter instances share this pointer; counter/gauge ops are atomic.
	stats *mongoStats

	// commands is the AMEND-B7 remembered-set (canonical names → membership).
	// Consumed at 29.1 by the codec's $cmd command-name lookup (§3.6).
	commands map[string]bool

	// Parsed at 29.1; CONSUMED later. accessLog (string path) → 29.3;
	// emitDynamicMetadata → 29.2; the fault-delay fields → 29.3. delayConfigured
	// records whether a (validated) delay block was present.
	accessLog           string
	emitDynamicMetadata bool
	delayConfigured     bool
	fixedDelay          time.Duration
	delayPercentNum     uint32
	delayPercentDenom   int32
}

// parseConfig validates + compiles the 5-field proto (parent §5.2 roster).
// access_log + emit_dynamic_metadata are parse-and-store (consumed later — §3.3).
// Returns PARSE-REJECT errors on PGV-mirror violations (ADR-0080 byte-stable).
// Validation order: stat_prefix first, then the delay block (Task 3).
func parseConfig(msg *mongo_proxyv3.MongoProxy) (*compiledConfig, error) {
	if msg.GetStatPrefix() == "" {
		return nil, errors.New(errStatPrefixRequired)
	}
	cfg := &compiledConfig{
		statPrefix:          msg.GetStatPrefix(),
		accessLog:           msg.GetAccessLog(),
		emitDynamicMetadata: msg.GetEmitDynamicMetadata(),
		commands:            map[string]bool{},
	}
	cmds := msg.GetCommands()
	if len(cmds) == 0 {
		cmds = defaultCommands
	}
	for _, c := range cmds {
		cfg.commands[c] = true
	}
	if err := parseDelay(msg.GetDelay(), cfg); err != nil { // Task 3
		return nil, err
	}
	return cfg, nil
}

// validDenominators is the FractionalPercent.DenominatorType enum set {HUNDRED=0,
// TEN_THOUSAND=1, MILLION=2}. go-control-plane ships NO generated FractionalPercent
// Validate(); envoy-go rejects out-of-range values for parity (parent §5.3 note).
var validDenominators = map[int32]bool{0: true, 1: true, 2: true}

// parseDelay validates + stores the FaultDelay block (AMEND-B9). The oneof
// fault_delay_secifier is REQUIRED when delay is present; fixed_delay must be
// > 0s; header_delay is parse-accepted (no delay results at 29.3 — parent §5.3);
// the percentage (if present) must carry a defined denominator. Consumed at 29.3.
func parseDelay(d *commonfaultv3.FaultDelay, cfg *compiledConfig) error {
	if d == nil {
		return nil // no delay block → nothing to validate
	}
	cfg.delayConfigured = true
	switch s := d.GetFaultDelaySecifier().(type) {
	case *commonfaultv3.FaultDelay_FixedDelay:
		fixed := s.FixedDelay.AsDuration()
		if fixed <= 0 {
			return errors.New(errDelayFixedDelayTooSmall)
		}
		cfg.fixedDelay = fixed
	case *commonfaultv3.FaultDelay_HeaderDelay_:
		// header_delay: parse-accept; produces no delay at 29.3 (parent §5.3 / D-P5).
	default: // nil oneof (delay: {}) → the specifier-required reject.
		return errors.New(errDelaySpecifierRequired)
	}
	if p := d.GetPercentage(); p != nil {
		if !validDenominators[int32(p.GetDenominator())] {
			return errors.New(errDelayDenominatorInvalid)
		}
		cfg.delayPercentNum = p.GetNumerator()
		cfg.delayPercentDenom = int32(p.GetDenominator())
	}
	return nil
}
