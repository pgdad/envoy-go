package redisproxy

// isLocalReply reports whether cmd (UPPERCASED by decodeRequest) is answered
// in-filter with zero upstream traffic. The 32.1 set is PING + AUTH (AMEND-R5 /
// D-P32-6 32.1 subset); ECHO/TIME/QUIT/HELLO are 32.2 follow-ons.
func isLocalReply(cmd string) bool {
	switch cmd {
	case "PING", "AUTH":
		return true
	default:
		return false
	}
}

// localReply returns the byte-stable local reply for a local-reply command, or
// nil for a proxied command. PING ignores its argument (does NOT echo — the
// reference behavior, parent §11.6). AUTH answers the no-password-set error (the
// 32.1 posture: no downstream_auth_password is consumed — SPEC §2.4).
func localReply(cmd string) []byte {
	switch cmd {
	case "PING":
		return respPong
	case "AUTH":
		return respAuthNoPassword
	default:
		return nil
	}
}
