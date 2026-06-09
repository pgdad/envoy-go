package redisproxy

import (
	"strconv"
	"strings"
	"time"
)

// supportedCommandList is the EXACT 180-command roster (lower-cased, strictly
// sorted) transcribed from SPEC §12.1 — the 8 iterated SupportedCommands groups
// of upstream Envoy v1.37.2 supported_commands.h (simpleCommands 152 + eval 2 +
// object 1 + hashMultipleSumResult 4 + mget/mset/scan/info.shard 4 + randomShard
// 2 + transaction 5 + ClusterScope 10 = 180). It is the SINGLE SOURCE OF TRUTH
// for BOTH the eager per-command stat roster (stats.go) AND the dispatch lookup.
// The dotted names (bf.add…bf.scandump, info.shard) flatten to command_bf_add /
// command_info_shard in Prometheus (dot→underscore; §5) and pass IsValidName
// (§5.2 — TestCommandRoster_AllValidNames). The inline singletons ping/auth/echo/
// time/quit are NOT here (handled in classify — §12.1). The latency histogram +
// *_fault counters are NOT created (ADR-0060 / faults — §2.3).
//
// KEEP-IN-SYNC: SPEC §12.1 golden + TestCommandRoster_MatchesUpstream.
var supportedCommandList = []string{
	"append", "bf.add", "bf.card", "bf.exists", "bf.info", "bf.insert", "bf.loadchunk",
	"bf.madd", "bf.mexists", "bf.reserve", "bf.scandump", "bitcount", "bitfield", "bitop",
	"bitpos", "cluster", "config", "copy", "decr", "decrby", "del", "discard", "dump",
	"eval", "evalsha", "exec", "exists", "expire", "expireat", "flushall", "flushdb",
	"geoadd", "geodist", "geohash", "geopos", "georadius", "georadius_ro", "georadiusbymember",
	"georadiusbymember_ro", "geosearch", "geosearchstore", "get", "getbit", "getdel", "getex",
	"getrange", "getset", "hdel", "hello", "hexists", "hget", "hgetall", "hincrby",
	"hincrbyfloat", "hkeys", "hlen", "hmget", "hmset", "hrandfield", "hscan", "hset", "hsetnx",
	"hstrlen", "hvals", "incr", "incrby", "incrbyfloat", "info", "info.shard", "keys", "lindex",
	"linsert", "llen", "lmove", "lpop", "lpos", "lpush", "lpushx", "lrange", "lrem", "lset",
	"ltrim", "mget", "mset", "msetnx", "multi", "object", "persist", "pexpire", "pexpireat",
	"pfadd", "pfcount", "pfmerge", "psetex", "pttl", "publish", "randomkey", "rename", "renamenx",
	"restore", "role", "rpop", "rpoplpush", "rpush", "rpushx", "sadd", "scan", "scard", "script",
	"sdiff", "sdiffstore", "select", "set", "setbit", "setex", "setnx", "setrange", "sinter",
	"sinterstore", "sismember", "slowlog", "smembers", "smismember", "smove", "sort", "sort_ro",
	"spop", "srandmember", "srem", "sscan", "strlen", "substr", "sunion", "sunionstore", "touch",
	"ttl", "type", "unlink", "unwatch", "watch", "xack", "xadd", "xautoclaim", "xclaim", "xdel",
	"xlen", "xpending", "xrange", "xrevrange", "xtrim", "zadd", "zcard", "zcount", "zdiff",
	"zdiffstore", "zincrby", "zinter", "zinterstore", "zlexcount", "zmscore", "zpopmax", "zpopmin",
	"zrandmember", "zrange", "zrangebylex", "zrangebyscore", "zrangestore", "zrank", "zrem",
	"zremrangebylex", "zremrangebyrank", "zremrangebyscore", "zrevrange", "zrevrangebylex",
	"zrevrangebyscore", "zrevrank", "zscan", "zscore", "zunion", "zunionstore",
}

// supportedCommands is the O(1) dispatch-lookup set derived from supportedCommandList.
var supportedCommands = func() map[string]struct{} {
	m := make(map[string]struct{}, len(supportedCommandList))
	for _, c := range supportedCommandList {
		m[c] = struct{}{}
	}
	return m
}()

// commandSupported lower-cases cmd (the decoder uppercases) and reports whether it
// is a per-command-stat-bearing command. The returned lc is the command.<lc>.*
// stat segment (a table member → IsValidName by construction, §5.2).
func commandSupported(cmd string) (lc string, ok bool) {
	lc = strings.ToLower(cmd)
	_, ok = supportedCommands[lc]
	return lc, ok
}

// action is the classify dispatch verdict kind.
type action uint8

const (
	actProxy       action = iota // forward upstream; statCmd set; command.<statCmd>.{total,success,error}
	actLocal                     // write reply in-filter (NO command.* increment); maybe closeAfter
	actUnsupported               // splitter.unsupported_command + write the -ERR unknown command reply
	actInvalid                   // splitter.invalid_request + write the -invalid request reply
)

// commandVerdict is the full dispatch decision for one decoded request (D-S32.2-5).
type commandVerdict struct {
	action     action
	reply      []byte // local/error reply bytes (actLocal/actUnsupported/actInvalid)
	closeAfter bool   // QUIT: close the downstream connection after writing reply
	statCmd    string // lower-cased command name for command.<statCmd>.* (actProxy)
}

// Local-reply byte-stable constants (the upstream wording — parent §11.5/§11.6 +
// §12.4; ResponseValues + makeError → "-<s>\r\n"). DO NOT CHANGE — asserted
// byte-identical cross-side (§8.1). (respPong/respAuthNoPassword stay in resp.go.)
var (
	respOK             = []byte("+OK\r\n")
	respInvalidRequest = []byte("-invalid request\r\n")
	respHelloOptions   = []byte("-ERR HELLO options like AUTH and SETNAME are not supported\r\n")
	respNoProto        = []byte("-NOPROTO unsupported protocol version\r\n")
)

// commandsWithoutMandatoryArgs is the upstream supported_commands.h set of table
// commands legitimately callable with zero arguments (so len(args)==1 is NOT a
// bad-arity reject). The inline singletons (ping/time/quit/auth/echo) are handled
// BEFORE the arity check (classify switch) and need no entry. D-S32.2-3: the exact
// membership is transcribed from upstream supported_commands.h — start from the
// empty set + add only commands the 0055 matrix or upstream confirms zero-arg-legal.
// KEEP-IN-SYNC: upstream supported_commands.h commandsWithoutMandatoryArgs().
var commandsWithoutMandatoryArgs = map[string]struct{}{
	// HELLO with no-arg is a valid ClusterScopeCommand (proxied; §3.3/§12.4).
	"hello": {},
}

// validArity applies the minimal upstream rule: a command with < 2 array elements
// (name only, no args) that is NOT in commandsWithoutMandatoryArgs is invalid.
func validArity(lc string, argc int) bool {
	if argc >= 2 {
		return true
	}
	_, ok := commandsWithoutMandatoryArgs[lc]
	return ok
}

// classify is the dispatch decision for a decoded (cmd UPPERCASED, args) request.
// args[0] is the AS-RECEIVED command token; args[1:] are the arguments.
func classify(cmd string, args [][]byte) commandVerdict {
	// Inline-singleton local-reply commands (NOT in the per-command table — §12.1).
	switch cmd {
	case "PING":
		return commandVerdict{action: actLocal, reply: respPong}
	case "AUTH":
		return commandVerdict{action: actLocal, reply: respAuthNoPassword}
	case "ECHO":
		if len(args) == 2 {
			return commandVerdict{action: actLocal, reply: encodeBulk(args[1])}
		}
		return commandVerdict{action: actInvalid, reply: respInvalidRequest}
	case "TIME":
		return commandVerdict{action: actLocal, reply: encodeTime()}
	case "QUIT":
		return commandVerdict{action: actLocal, reply: respOK, closeAfter: true}
	case "HELLO":
		if v, ok := classifyHello(args); ok {
			return v // error-form local reply; HELLO 2 / no-arg falls through to proxy
		}
	}
	lc, ok := commandSupported(cmd)
	if !ok {
		return commandVerdict{action: actUnsupported, reply: unknownCommandError(args)}
	}
	if !validArity(lc, len(args)) {
		return commandVerdict{action: actInvalid, reply: respInvalidRequest}
	}
	return commandVerdict{action: actProxy, statCmd: lc}
}

// classifyHello returns the local error verdict for a HELLO error-form (>2 args →
// options-unsupported; a non-"2" proto arg incl. "3"/non-numeric → NOPROTO), or
// (_, false) for HELLO 2 / no-arg (proxied — a ClusterScopeCommand; §3.3/§12.4).
func classifyHello(args [][]byte) (commandVerdict, bool) {
	if len(args) > 2 {
		return commandVerdict{action: actLocal, reply: respHelloOptions}, true
	}
	if len(args) == 2 && string(args[1]) != "2" {
		return commandVerdict{action: actLocal, reply: respNoProto}, true
	}
	return commandVerdict{}, false
}

// encodeBulk renders a RESP bulk string "$<len>\r\n<bytes>\r\n" (the ECHO reply).
func encodeBulk(b []byte) []byte {
	out := make([]byte, 0, len(b)+16)
	out = append(out, '$')
	out = strconv.AppendInt(out, int64(len(b)), 10)
	out = append(out, '\r', '\n')
	out = append(out, b...)
	out = append(out, '\r', '\n')
	return out
}

// encodeTime renders the TIME reply: a 2-element array of bulk strings carrying
// the local wall-clock unix seconds + microseconds (NON-DETERMINISTIC — §12.4;
// upstream's dispatcher.timeSource().systemTime()). Shape-tested only.
func encodeTime() []byte {
	now := time.Now()
	secs := strconv.FormatInt(now.Unix(), 10)
	micros := strconv.FormatInt(int64(now.Nanosecond())/1000, 10)
	var b []byte
	b = append(b, '*', '2', '\r', '\n')
	b = append(b, encodeBulk([]byte(secs))...)
	b = append(b, encodeBulk([]byte(micros))...)
	return b
}

// unknownCommandError builds the byte-stable "-ERR unknown command '<name>', with
// args beginning with: <args>\r\n" reply (the upstream wording — parent §11.5;
// <name> is the AS-RECEIVED args[0], original case; <args> is the AS-RECEIVED
// arguments args[1:] joined by ", "). D-S32.2-2: the EXACT form is confirmed LIVE
// at the 0055 UNKNOWN cross-side differential arm (Task 10) — the contrib
// reference Envoy emits the arguments verbatim after "beginning with: ", e.g.
// `-ERR unknown command 'BOGUSCMD', with args beginning with: x\r\n` for
// respArray("BOGUSCMD","x"). The wire is shared, so the subject MUST match the
// reference byte-for-byte (reference_wire_format_both_sides_see_same_bytes).
func unknownCommandError(args [][]byte) []byte {
	var name string
	var argStrs []string
	if len(args) > 0 {
		name = string(args[0])
		argStrs = make([]string, 0, len(args)-1)
		for _, a := range args[1:] {
			argStrs = append(argStrs, string(a))
		}
	}
	return []byte("-ERR unknown command '" + name + "', with args beginning with: " +
		strings.Join(argStrs, ", ") + "\r\n")
}
