package zookeeperproxy

import (
	"github.com/pgdad/envoy-go/internal/filter/network/statroster"
	"github.com/pgdad/envoy-go/internal/stats"
)

// opNames is the per-opcode counter-name table (the upstream stats-macro
// spelling, all lowercase, transcribed verbatim from the live reference
// envoyproxy/envoy:v1.37.2 admin /stats dump; probe date 2026-06-02).
// Family membership is NOT uniform (live-dump confirmed) — the per-family
// lists below encode the asymmetries exactly as the upstream macro declares them.
//
// KEEP-IN-SYNC: internal/stats/name.go (the .zookeeper. inline-prefix arm) and
// the 0046 fixture's expected-counter tables key off these suffixes.

// rqOpNames is the 28-element opname list for the <op>_rq family.
// Includes connect_readonly and setauth; excludes auth (no auth_rq macro counter —
// live-dump confirmed asymmetry).
var rqOpNames = []string{
	"addwatch",
	"check",
	"checkwatches",
	"close",
	"connect_readonly",
	"connect",
	"create2",
	"createcontainer",
	"create",
	"createttl",
	"delete",
	"exists",
	"getacl",
	"getallchildrennumber",
	"getchildren2",
	"getchildren",
	"getdata",
	"getephemerals",
	"multi",
	"ping",
	"reconfig",
	"removewatches",
	"setacl",
	"setauth",
	"setdata",
	"setwatches2",
	"setwatches",
	"sync",
}

// rqBytesOpNames is the 29-element opname list for the <op>_rq_bytes family.
// Includes auth, connect_readonly, and setauth (all three extra vs _rq family).
var rqBytesOpNames = []string{
	"addwatch",
	"auth",
	"check",
	"checkwatches",
	"close",
	"connect_readonly",
	"connect",
	"create2",
	"createcontainer",
	"create",
	"createttl",
	"delete",
	"exists",
	"getacl",
	"getallchildrennumber",
	"getchildren2",
	"getchildren",
	"getdata",
	"getephemerals",
	"multi",
	"ping",
	"reconfig",
	"removewatches",
	"setacl",
	"setauth",
	"setdata",
	"setwatches2",
	"setwatches",
	"sync",
}

// decoderErrorOpNames is the 28-element opname list for the <op>_decoder_error family.
// Includes auth and setauth; excludes connect_readonly.
var decoderErrorOpNames = []string{
	"addwatch",
	"auth",
	"check",
	"checkwatches",
	"close",
	"connect",
	"create2",
	"createcontainer",
	"create",
	"createttl",
	"delete",
	"exists",
	"getacl",
	"getallchildrennumber",
	"getchildren2",
	"getchildren",
	"getdata",
	"getephemerals",
	"multi",
	"ping",
	"reconfig",
	"removewatches",
	"setacl",
	"setauth",
	"setdata",
	"setwatches2",
	"setwatches",
	"sync",
}

// respOpNames is the 28-element opname list shared by the _resp, _resp_bytes,
// _resp_fast, and _resp_slow families.
// Includes auth and setauth; excludes connect_readonly (no connect_readonly_resp —
// live-dump confirmed asymmetry).
var respOpNames = []string{
	"addwatch",
	"auth",
	"check",
	"checkwatches",
	"close",
	"connect",
	"create2",
	"createcontainer",
	"create",
	"createttl",
	"delete",
	"exists",
	"getacl",
	"getallchildrennumber",
	"getchildren2",
	"getchildren",
	"getdata",
	"getephemerals",
	"multi",
	"ping",
	"reconfig",
	"removewatches",
	"setacl",
	"setauth",
	"setdata",
	"setwatches2",
	"setwatches",
	"sync",
}

// rosterSuffixes returns the exact 201 upstream-macro counter suffixes:
// 4 plain + 28 _rq + 29 _rq_bytes + 28 _decoder_error + 28 _resp +
// 28 _resp_bytes + 28 _resp_fast + 28 _resp_slow (AMEND-A2; R2).
// The returned slice is in construction order (not sorted); callers that need
// a deterministic sort should sort the result.
func rosterSuffixes() []string {
	out := make([]string, 0, 201)
	out = append(out, "decoder_error", "request_bytes", "response_bytes", "watch_event")
	for _, op := range rqOpNames {
		out = append(out, op+"_rq")
	}
	for _, op := range rqBytesOpNames {
		out = append(out, op+"_rq_bytes")
	}
	for _, op := range decoderErrorOpNames {
		out = append(out, op+"_decoder_error")
	}
	for _, op := range respOpNames {
		out = append(out, op+"_resp", op+"_resp_bytes", op+"_resp_fast", op+"_resp_slow")
	}
	return out
}

// rosterStats holds the 201 eagerly-created macro counters (D-P5 creation
// parity; created once per distinct stat_prefix at config parse) plus the lazily
// created dynamic per-scheme auth counters.
type rosterStats struct {
	prefix   string // "<stat_prefix>.zookeeper."
	reg      *stats.Registry
	counters map[string]*stats.Counter // keyed by suffix; all 201 created eagerly
}

// newRosterStats eagerly creates all 201 macro counters under
// <statPrefix>.zookeeper. via the shared statroster scaffold (NewCounterIfAbsent
// — post-Freeze-permitted and idempotent across listeners sharing a stat_prefix;
// the rbac newFilterStats precedent; ADR-0222 §4.4; D-P5).
func newRosterStats(reg *stats.Registry, statPrefix string) *rosterStats {
	prefix := statPrefix + ".zookeeper."
	return &rosterStats{
		prefix:   prefix,
		reg:      reg,
		counters: statroster.New(reg, prefix, rosterSuffixes()),
	}
}

// inc increments the macro counter for suffix. Unknown suffix is a programming
// error → panic (the roster is closed; dynamic names go through authSchemeCounter).
func (rs *rosterStats) inc(suffix string) { statroster.Inc("zookeeperproxy", rs.counters, suffix) }

// add adds delta to the macro counter for suffix (the *_bytes accounting).
// Unknown suffix is a programming error → panic.
func (rs *rosterStats) add(suffix string, delta uint64) {
	statroster.Add("zookeeperproxy", rs.counters, suffix, delta)
}

// authSchemeBuiltins is the upstream builtin auth-scheme set (filter.cc:45-46 at
// v1.37.2: rememberBuiltins {"auth_rq","digest_rq","host_rq","ip_rq",
// "ping_response_rq","world_rq","x509_rq"}). A scheme outside this set takes the
// unknown_scheme fallback — upstream getBuiltin parity (live-verified: scheme
// "foobar" increments auth.unknown_scheme_rq, never auth.foobar_rq). This also
// bounds dynamic-counter cardinality and makes charset sanitization unnecessary
// (arbitrary bytes are never used in a counter name).
// NOTE: "ping_response" is in the set because upstream shares one StatNameSet —
// an auth request with scheme "ping_response" genuinely creates
// auth.ping_response_rq upstream. Mirrored for exactness.
var authSchemeBuiltins = map[string]bool{
	"auth": true, "digest": true, "host": true, "ip": true,
	"ping_response": true, "world": true, "x509": true,
}

// authSchemeCounter returns (lazily creating) the dynamic per-scheme auth
// request counter <stat_prefix>.zookeeper.auth.<scheme>_rq for BUILTIN schemes,
// or <stat_prefix>.zookeeper.auth.unknown_scheme_rq for anything else
// (upstream getBuiltin-fallback parity). NewCounterIfAbsent is REQUIRED:
// decode time is post-Freeze runtime.
func (rs *rosterStats) authSchemeCounter(scheme string) *stats.Counter {
	if !authSchemeBuiltins[scheme] {
		scheme = "unknown_scheme"
	}
	return rs.reg.NewCounterIfAbsent(rs.prefix + "auth." + scheme + "_rq")
}
