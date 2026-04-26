// Package fixtures_0004 holds fixture data; PKI is generated via go:generate.
//
// The committed PEMs under pki/ are the authoritative source. CI does NOT run
// `go generate`; only humans run it (and only to rotate). The generator is
// deterministic — `git diff --exit-code pki/` is clean across re-runs.
//
//go:generate go run ./pki/gen
package fixtures_0004
