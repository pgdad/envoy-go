package redisproxy

import "testing"

func TestIsLocalReply(t *testing.T) {
	for _, c := range []string{"PING", "AUTH"} {
		if !isLocalReply(c) {
			t.Errorf("isLocalReply(%q) = false, want true", c)
		}
	}
	for _, c := range []string{"SET", "GET", "ECHO", "TIME", "QUIT", "HELLO", ""} {
		if isLocalReply(c) {
			t.Errorf("isLocalReply(%q) = true, want false (proxied or 32.2)", c)
		}
	}
}

func TestLocalReply_Bytes(t *testing.T) {
	if got := localReply("PING"); string(got) != "+PONG\r\n" {
		t.Errorf("localReply(PING) = %q", got)
	}
	if got := localReply("AUTH"); string(got) != "-ERR Client sent AUTH, but no password is set\r\n" {
		t.Errorf("localReply(AUTH) = %q", got)
	}
	if got := localReply("SET"); got != nil {
		t.Errorf("localReply(SET) = %q, want nil (proxied)", got)
	}
}
