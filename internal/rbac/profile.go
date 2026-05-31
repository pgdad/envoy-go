package rbac

// Profile is an input-capability declaration: which Permission/Principal arms
// a consumer's input surface can evaluate. The engine compiles ALL arms by
// default (the phase-16 HTTP superset); a Profile lets the L4 consumer
// PARSE-REJECT the HTTP-only arms at compile WITHOUT changing consumer #1
// (R4). It keeps the engine the single owner of arm-classification (R-E).
type Profile int

const (
	// ProfileHTTP permits every arm (the phase-16 superset). HTTP rbac passes
	// this → byte-identical compile + evaluation to pre-extraction.
	ProfileHTTP Profile = iota
	// ProfileL4 permits only the L4-evaluable arms; the HTTP-only arms
	// (header / url_path permissions + principals) reject at compile.
	// (uri_template + matcher-extension arms already reject unconditionally.)
	ProfileL4
)

// armKind enumerates the HTTP-only arms a Profile gates. The L4-evaluable
// arms (any / destination_ip / destination_port / destination_port_range /
// requested_server_name permissions; any / authenticated / direct_remote_ip /
// remote_ip principals; and / or / not combinators) are never gated — permits
// returns true for them under either profile. Note: source_ip is NOT listed
// here because it is a PARSE-REJECT arm (rejected unconditionally in
// buildOnePrincipal before the profile gate is consulted).
type armKind int

const (
	armHeader  armKind = iota // permission.header / principal.header
	armURLPath                // permission.url_path / principal.url_path
)

// permits reports whether the profile allows the given HTTP-only arm.
// ProfileHTTP permits every arm. ProfileL4 rejects the HTTP-only arms
// (armHeader, armURLPath); any future L4-evaluable arm is permitted by default.
func (p Profile) permits(a armKind) bool {
	if p == ProfileHTTP {
		return true
	}
	// ProfileL4 rejects the HTTP-only arms; any future L4-evaluable arm
	// is permitted by default.
	switch a {
	case armHeader, armURLPath:
		return false
	default:
		return true
	}
}
