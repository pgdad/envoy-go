package admin

import (
	"runtime"
	"strings"
	"testing"
)

func TestBuildVersionString_FiveTokens(t *testing.T) {
	v := BuildVersionString()
	parts := strings.Split(v, "/")
	if len(parts) != 5 {
		t.Fatalf("BuildVersionString() = %q; want 5 slash-separated tokens, got %d", v, len(parts))
	}
}

func TestBuildVersionString_GoVersionToken(t *testing.T) {
	v := BuildVersionString()
	parts := strings.Split(v, "/")
	if parts[1] != runtime.Version() {
		t.Errorf("BuildVersionString() token 2 = %q; want %q", parts[1], runtime.Version())
	}
}

func TestBuildVersionString_LiteralCleanReleaseGocrypto(t *testing.T) {
	v := BuildVersionString()
	parts := strings.Split(v, "/")
	if parts[2] != "Clean" {
		t.Errorf("BuildVersionString() token 3 = %q; want %q", parts[2], "Clean")
	}
	if parts[3] != "RELEASE" {
		t.Errorf("BuildVersionString() token 4 = %q; want %q", parts[3], "RELEASE")
	}
	if parts[4] != "Go-crypto" {
		t.Errorf("BuildVersionString() token 5 = %q; want %q", parts[4], "Go-crypto")
	}
}

func TestBuildVersionString_RevisionDefaultsToUnknownInTestBuild(t *testing.T) {
	// In a go-test build, debug.ReadBuildInfo's vcs.revision setting may
	// be empty (depending on whether the build embeds VCS info). The
	// fallback path emits "unknown". Either way, the first token must
	// be non-empty.
	v := BuildVersionString()
	parts := strings.Split(v, "/")
	if parts[0] == "" {
		t.Errorf("BuildVersionString() token 1 (revision) is empty; want non-empty (either VCS-derived or 'unknown')")
	}
}

func TestBuildVersionString_RevisionLDFlagOverride(t *testing.T) {
	// Save and restore the package-level Revision var to assert the
	// -ldflags override path: setting Revision rebuilds the version string.
	saved := Revision
	defer func() { Revision = saved }()
	Revision = "abcdef1234567890"
	v := BuildVersionString()
	parts := strings.Split(v, "/")
	// Per planner-time decision 1: <sha-short> is Revision[:7] when len(Revision) >= 7.
	if parts[0] != "abcdef1" {
		t.Errorf("BuildVersionString() token 1 with Revision=abcdef1234567890: got %q, want %q", parts[0], "abcdef1")
	}
}
