package admin

import (
	"runtime"
	"runtime/debug"
)

// Revision is the VCS revision the binary was built from. Default-initialized
// from runtime/debug.ReadBuildInfo's vcs.revision setting at init time, or
// "unknown" when no VCS info is embedded (e.g., go-test builds, raw `go run`).
// Release builds override via:
//
//	go build -ldflags "-X github.com/esalaine/envoy-go/internal/admin.Revision=<sha>" ...
//
// The 7-char abbreviation is taken at format time (per BuildVersionString),
// matching Envoy's version-string convention (`5afe27f...` rather than the
// full 40-char SHA).
var Revision = readRevision()

func readRevision() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			return s.Value
		}
	}
	return "unknown"
}

// BuildVersionString returns the version string emitted in /server_info's
// `version` field per SPEC §6.5 + planner-time decision 1 (option A; 5-token
// mirror of Envoy's `<sha>/1.37.2/Clean/RELEASE/BoringSSL` shape):
//
//	<sha-short>/<go-version>/Clean/RELEASE/Go-crypto
//
// Where:
//   - <sha-short> = first 7 chars of Revision (or full Revision if shorter,
//     including the literal "unknown" fallback).
//   - <go-version> = runtime.Version() (e.g., "go1.23.0").
//   - "Clean" + "RELEASE" + "Go-crypto" = literal tokens (Envoy emits
//     "Clean/RELEASE/BoringSSL"; envoy-go substitutes "Go-crypto" because
//     it links against Go stdlib crypto/tls, not BoringSSL).
//
// The /server_info equivalence claim allow-lists the version field in the
// differential matrix per SPEC §13.2; this format is for byte-shape
// consistency with the differential parser and operator-side `curl /server_info`
// readability, not byte equality with Envoy.
func BuildVersionString() string {
	rev := Revision
	if len(rev) >= 7 {
		rev = rev[:7]
	}
	return rev + "/" + runtime.Version() + "/Clean/RELEASE/Go-crypto"
}
