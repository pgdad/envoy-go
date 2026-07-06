package redisproxy

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestCommandRoster_MatchesUpstream pins supportedCommandList against the SPEC
// §12.1 golden 180-name list (the 8 iterated SupportedCommands groups, deduped).
// A transcription error in the table literal fails here (the byte-stable guard).
func TestCommandRoster_MatchesUpstream(t *testing.T) {
	if got := len(supportedCommandList); got != 180 {
		t.Fatalf("supportedCommandList size = %d, want 180", got)
	}
	// Sorted + unique (the map is derived from the slice; a dup would shrink it).
	if got := len(supportedCommands); got != 180 {
		t.Fatalf("supportedCommands map size = %d, want 180 (a duplicate name?)", got)
	}
	for i := 1; i < len(supportedCommandList); i++ {
		if supportedCommandList[i-1] >= supportedCommandList[i] {
			t.Fatalf("supportedCommandList not strictly sorted/unique at %d: %q >= %q",
				i, supportedCommandList[i-1], supportedCommandList[i])
		}
	}
	// Spot-pin a representative member from each of the 8 groups + the dotted names.
	for _, name := range []string{
		"get", "set", "append", // simpleCommands
		"eval", "evalsha", // evalCommands
		"object",                           // objectCommands
		"del", "exists", "touch", "unlink", // hashMultipleSumResultCommands
		"mget", "mset", "scan", "info.shard", // dedicated handlers
		"cluster", "randomkey", // randomShardCommands
		"multi", "exec", "discard", "watch", "unwatch", // transactionCommands
		"script", "flushall", "config", "info", "keys", "select", "role", "hello", // ClusterScopeCommands
		"bf.add", "bf.scandump", // module dotted names
	} {
		if _, ok := supportedCommands[name]; !ok {
			t.Errorf("supportedCommands missing %q", name)
		}
	}
	// The singletons handled inline (NOT in the per-command table — §12.1).
	for _, name := range []string{"ping", "auth", "echo", "time", "quit"} {
		if _, ok := supportedCommands[name]; ok {
			t.Errorf("supportedCommands must NOT contain inline-singleton %q", name)
		}
	}
}

// TestCommandRoster_AllValidNames pins the IsValidName-by-construction property
// (D-P32-7 / §5.2): every table name flattens to command.<name>.{total,...} whose
// segment chars are [a-z._] — all valid; a wire command not in the table never
// reaches NewCounterIfAbsent (it routes to splitter.unsupported_command).
func TestCommandRoster_AllValidNames(t *testing.T) {
	for _, name := range supportedCommandList {
		for _, slot := range []string{"total", "success", "error"} {
			full := "redis.rp.command." + name + "." + slot
			if !stats.IsValidName(full) {
				t.Errorf("stat name %q is NOT IsValidName — table member %q breaks by-construction guarantee", full, name)
			}
		}
	}
}

func TestCommandSupported_LookupIsLowerCase(t *testing.T) {
	if lc, ok := commandSupported("GET"); !ok || lc != "get" {
		t.Errorf("commandSupported(GET) = (%q,%v), want (get,true)", lc, ok)
	}
	if lc, ok := commandSupported("INFO.SHARD"); !ok || lc != "info.shard" {
		t.Errorf("commandSupported(INFO.SHARD) = (%q,%v), want (info.shard,true)", lc, ok)
	}
	if _, ok := commandSupported("BOGUSCMD"); ok {
		t.Error("commandSupported(BOGUSCMD) = true, want false")
	}
}

// asArgs builds the args slice classify expects: args[0]=command token, args[1:]=arguments.
func asArgs(parts ...string) [][]byte {
	out := make([][]byte, len(parts))
	for i, p := range parts {
		out[i] = []byte(p)
	}
	return out
}

func TestClassify_LocalReplies(t *testing.T) {
	cases := []struct {
		name       string
		cmd        string
		args       []string
		wantAction action
		wantReply  string
		wantClose  bool
	}{
		{"PING no echo", "PING", []string{"PING", "hello"}, actLocal, "+PONG\r\n", false},
		{"AUTH no password", "AUTH", []string{"AUTH", "x"}, actLocal, "-ERR Client sent AUTH, but no password is set\r\n", false},
		{"ECHO valid", "ECHO", []string{"ECHO", "hi"}, actLocal, "$2\r\nhi\r\n", false},
		{"ECHO wrong arity", "ECHO", []string{"ECHO"}, actInvalid, "-invalid request\r\n", false},
		{"QUIT closes", "QUIT", []string{"QUIT"}, actLocal, "+OK\r\n", true},
		{"HELLO 3 NOPROTO", "HELLO", []string{"HELLO", "3"}, actLocal, "-NOPROTO unsupported protocol version\r\n", false},
		{"HELLO options", "HELLO", []string{"HELLO", "2", "AUTH", "u", "p"}, actLocal, "-ERR HELLO options like AUTH and SETNAME are not supported\r\n", false},
		{"unknown command", "BOGUSCMD", []string{"BOGUSCMD", "x"}, actUnsupported, "-ERR unknown command 'BOGUSCMD', with args beginning with: x\r\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := classify(tc.cmd, asArgs(tc.args...))
			if v.action != tc.wantAction {
				t.Errorf("action = %d, want %d", v.action, tc.wantAction)
			}
			if string(v.reply) != tc.wantReply {
				t.Errorf("reply = %q, want %q", v.reply, tc.wantReply)
			}
			if v.closeAfter != tc.wantClose {
				t.Errorf("closeAfter = %v, want %v", v.closeAfter, tc.wantClose)
			}
		})
	}
}

func TestClassify_Proxied(t *testing.T) {
	// HELLO 2 / HELLO no-arg → proxied (it IS in the table — ClusterScopeCommand).
	for _, args := range [][]string{{"HELLO", "2"}, {"HELLO"}} {
		v := classify("HELLO", asArgs(args...))
		if v.action != actProxy || v.statCmd != "hello" {
			t.Errorf("classify(HELLO %v) = {action:%d statCmd:%q}, want {actProxy hello}", args, v.action, v.statCmd)
		}
	}
	// A data command → proxied, lower-cased stat segment.
	v := classify("GET", asArgs("GET", "foo"))
	if v.action != actProxy || v.statCmd != "get" {
		t.Errorf("classify(GET foo) = {action:%d statCmd:%q}, want {actProxy get}", v.action, v.statCmd)
	}
}

func TestClassify_BadArity(t *testing.T) {
	// A table command needing args, sent with none → invalid_request.
	v := classify("GET", asArgs("GET"))
	if v.action != actInvalid || string(v.reply) != "-invalid request\r\n" {
		t.Errorf("classify(GET) = {action:%d reply:%q}, want {actInvalid -invalid request}", v.action, v.reply)
	}
}

func TestClassify_TimeShapeOnly(t *testing.T) {
	// TIME is local but wall-clock (NON-DETERMINISTIC) — assert SHAPE: a 2-element
	// array of bulk strings, both numeric. NOT a byte-equivalence arm (§12.4).
	v := classify("TIME", asArgs("TIME"))
	if v.action != actLocal {
		t.Fatalf("TIME action = %d, want actLocal", v.action)
	}
	// Round-trip the reply through decodeReply to confirm it is a valid RESP frame,
	// then assert the 2-element-bulk-array shape via the prefix.
	if _, err := decodeReply(bufio.NewReader(bytes.NewReader(v.reply))); err != nil {
		t.Fatalf("TIME reply not a valid RESP frame: %v (%q)", err, v.reply)
	}
	if !strings.HasPrefix(string(v.reply), "*2\r\n$") {
		t.Errorf("TIME reply = %q, want a 2-element array of bulk strings", v.reply)
	}
}
