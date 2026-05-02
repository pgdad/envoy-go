package listenerfilter

import (
	"errors"
	"net"
	"strings"
)

// ChainSpec is the parsed match-dimension shape for one filter_chain (or
// the default_filter_chain). Constructed at NewManager-build time from the
// proto's FilterChainMatch message; immutable thereafter.
type ChainSpec struct {
	Name string
	// Empty: true means filter_chain_match is absent or all-zero — the
	// chain is universally eligible (catch-all). When true, every other
	// field is ignored.
	Empty bool
	// DestinationPort: 0 means unspecified; non-zero means the chain
	// requires the connection's local port to equal this value.
	DestinationPort uint32
	// PrefixRanges: empty means unspecified; non-empty means the chain
	// requires conn.LocalAddr().IP to fall in at least one CIDR.
	PrefixRanges []*net.IPNet
	// ServerNames: empty means unspecified; non-empty means SNI must match
	// at least one entry per chainSpecificityRank semantics (exact > suffix
	// > universal > catch-all). Re-uses the existing
	// internal/listener/manager.go:chainSpecificityRank logic at the
	// tie-breaker level.
	ServerNames []string
	// TransportProtocol: "" means unspecified; "tls" or "raw_buffer" means
	// the chain requires the listener-filter pipeline to have set
	// inputs.TransportProtocol to the matching value.
	TransportProtocol string
	// ApplicationProtocols: empty means unspecified; non-empty means
	// inputs.ApplicationProtocols must contain at least one matching entry.
	ApplicationProtocols []string
	// SourceTypeLocal / SourceTypeExternal: at most one true; true means
	// the chain requires conn.RemoteAddr().IP to be loopback / non-loopback
	// respectively. Both false means source_type unspecified (ANY).
	SourceTypeLocal    bool
	SourceTypeExternal bool
	// SourcePrefixRanges: empty means unspecified; non-empty means
	// conn.RemoteAddr().IP must fall in at least one CIDR.
	SourcePrefixRanges []*net.IPNet
	// SourcePorts: empty means unspecified; non-empty means
	// conn.RemoteAddr().Port must equal at least one entry.
	SourcePorts []uint32
}

// ErrNoChainMatched is returned by SelectChain when no chain in chains is
// eligible AND defaultChain is nil.
var ErrNoChainMatched = errors.New("no filter_chain matches connection")

// ErrAmbiguousChainMatch is returned by SelectChain when two chains have
// identical specificity vectors AND identical sub-ordering values for the
// highest-priority specific dimension. The listener manager detects this at
// NewManager-build time (it pre-runs SelectChain on a sample input or
// duplicate-matches the chain specs structurally) and rejects the bootstrap.
var ErrAmbiguousChainMatch = errors.New("ambiguous filter_chain selection")

// priorityOrder is the 8-dimension specificity priority vector per SPEC
// §5.5 and the upstream filter_chain_match.proto comments. Index 0 = most
// specific dimension.
const (
	prioDestinationPort      = 0
	prioPrefixRanges         = 1
	prioServerNames          = 2
	prioTransportProtocol    = 3
	prioApplicationProtocols = 4
	prioSourceType           = 5
	prioSourcePrefixRanges   = 6
	prioSourcePorts          = 7
	prioCount                = 8
)

// SelectChain runs the 2-pass eligibility-then-specificity algorithm per
// SPEC §5.5 + §7.3. Returns the winning chain, or defaultChain if no chain
// in chains is eligible, or (nil, ErrNoChainMatched) if neither path yields
// a result.
func SelectChain(inputs ChainMatchInputs, chains []*ChainSpec, defaultChain *ChainSpec) (*ChainSpec, error) {
	// Pass 1: eligibility.
	eligible := make([]*ChainSpec, 0, len(chains))
	for _, c := range chains {
		if matches(c, &inputs) {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		if defaultChain != nil {
			return defaultChain, nil
		}
		return nil, ErrNoChainMatched
	}
	// Pass 2: specificity scoring.
	best := eligible[0]
	bestScore := specificityScore(best)
	for _, c := range eligible[1:] {
		cScore := specificityScore(c)
		switch {
		case cScore > bestScore:
			best = c
			bestScore = cScore
		case cScore == bestScore:
			best = breakTie(best, c, &inputs)
			if best == nil {
				return nil, ErrAmbiguousChainMatch
			}
		}
	}
	return best, nil
}

// matches reports whether every non-zero dimension of c is satisfied by
// inputs. Empty-match chains return true unconditionally.
func matches(c *ChainSpec, inputs *ChainMatchInputs) bool {
	if c.Empty {
		return true
	}
	if c.DestinationPort != 0 && c.DestinationPort != inputs.DestinationPort {
		return false
	}
	if len(c.PrefixRanges) > 0 && !ipInAny(inputs.DestinationIP, c.PrefixRanges) {
		return false
	}
	if len(c.ServerNames) > 0 && !sniMatchAny(c.ServerNames, inputs.ServerName) {
		return false
	}
	if c.TransportProtocol != "" && c.TransportProtocol != inputs.TransportProtocol {
		return false
	}
	if len(c.ApplicationProtocols) > 0 && !alpnMatchAny(c.ApplicationProtocols, inputs.ApplicationProtocols) {
		return false
	}
	if c.SourceTypeLocal && !inputs.IsLoopbackSource() {
		return false
	}
	if c.SourceTypeExternal && inputs.IsLoopbackSource() {
		return false
	}
	if len(c.SourcePrefixRanges) > 0 && !ipInAny(inputs.SourceIP, c.SourcePrefixRanges) {
		return false
	}
	if len(c.SourcePorts) > 0 && !portInAny(inputs.SourcePort, c.SourcePorts) {
		return false
	}
	return true
}

// specificityScore returns an 8-bit integer where bit prioCount-1-i is set
// iff priority slot i is specified (specific) on c. Higher score = more
// specific. Bit ordering puts the most-significant-bit on the highest-
// priority dimension so a numerical compare reflects the priority order.
func specificityScore(c *ChainSpec) uint8 {
	if c.Empty {
		return 0
	}
	var s uint8
	if c.DestinationPort != 0 {
		s |= 1 << (prioCount - 1 - prioDestinationPort)
	}
	if len(c.PrefixRanges) > 0 {
		s |= 1 << (prioCount - 1 - prioPrefixRanges)
	}
	if len(c.ServerNames) > 0 {
		s |= 1 << (prioCount - 1 - prioServerNames)
	}
	if c.TransportProtocol != "" {
		s |= 1 << (prioCount - 1 - prioTransportProtocol)
	}
	if len(c.ApplicationProtocols) > 0 {
		s |= 1 << (prioCount - 1 - prioApplicationProtocols)
	}
	if c.SourceTypeLocal || c.SourceTypeExternal {
		s |= 1 << (prioCount - 1 - prioSourceType)
	}
	if len(c.SourcePrefixRanges) > 0 {
		s |= 1 << (prioCount - 1 - prioSourcePrefixRanges)
	}
	if len(c.SourcePorts) > 0 {
		s |= 1 << (prioCount - 1 - prioSourcePorts)
	}
	return s
}

// breakTie compares a vs b on the per-dimension finer-grain criteria when
// their specificity vectors are identical. Returns the winner; returns nil
// if a and b are entirely indistinguishable (a NewManager-time config error
// the listener manager surfaces as ErrAmbiguousChainMatch).
func breakTie(a, b *ChainSpec, inputs *ChainMatchInputs) *ChainSpec {
	// PrefixRanges: longer prefix wins (smaller IPNet).
	if len(a.PrefixRanges) > 0 && len(b.PrefixRanges) > 0 {
		la := longestPrefix(inputs.DestinationIP, a.PrefixRanges)
		lb := longestPrefix(inputs.DestinationIP, b.PrefixRanges)
		if la > lb {
			return a
		}
		if lb > la {
			return b
		}
	}
	// SourcePrefixRanges: longer prefix wins.
	if len(a.SourcePrefixRanges) > 0 && len(b.SourcePrefixRanges) > 0 {
		la := longestPrefix(inputs.SourceIP, a.SourcePrefixRanges)
		lb := longestPrefix(inputs.SourceIP, b.SourcePrefixRanges)
		if la > lb {
			return a
		}
		if lb > la {
			return b
		}
	}
	// ServerNames: SNI specificity (exact > suffix > universal > catch-all).
	if len(a.ServerNames) > 0 && len(b.ServerNames) > 0 {
		ra := sniSpecificityRank(a.ServerNames)
		rb := sniSpecificityRank(b.ServerNames)
		if ra < rb {
			return a
		} // lower rank = more specific
		if rb < ra {
			return b
		}
	}
	// All other dimensions are exact-value match — no sub-ordering. If we
	// reach here, the two chains are indistinguishable on this input.
	return nil
}

// ipInAny reports whether ip falls in at least one of the CIDRs.
func ipInAny(ip net.IP, cidrs []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}

// portInAny reports whether p equals at least one of the ports.
func portInAny(p uint32, ports []uint32) bool {
	for _, q := range ports {
		if p == q {
			return true
		}
	}
	return false
}

// longestPrefix returns the longest prefix-length CIDR in cidrs that
// contains ip; 0 if none.
func longestPrefix(ip net.IP, cidrs []*net.IPNet) int {
	best := 0
	for _, c := range cidrs {
		if c.Contains(ip) {
			ones, _ := c.Mask.Size()
			if ones > best {
				best = ones
			}
		}
	}
	return best
}

// sniMatchAny reports whether sni matches any of the patterns (exact OR
// suffix-wildcard OR universal-wildcard OR catch-all).
func sniMatchAny(patterns []string, sni string) bool {
	for _, p := range patterns {
		if p == "*" || p == sni {
			return true
		}
		if strings.HasPrefix(p, "*.") && strings.HasSuffix(sni, p[1:]) {
			return true
		}
	}
	return false
}

// alpnMatchAny reports whether at least one element of want is in offered.
func alpnMatchAny(want, offered []string) bool {
	for _, w := range want {
		for _, o := range offered {
			if w == o {
				return true
			}
		}
	}
	return false
}

// sniSpecificityRank mirrors internal/listener/manager.go:chainSpecificityRank
// (preserved from phase 03 per ADR-0033 clause 9 → ADR-0078). Lower rank =
// more specific. Used as the SNI sub-ordering tie-breaker WITHIN the
// server_names priority slot per SPEC §5.5.
//
//	0: any non-wildcard pattern
//	1: any suffix-wildcard ("*.foo.test")
//	2: universal-wildcard ("*")
//	3: catch-all (empty patterns slice — unused here since
//	   matches() rejects this case before breakTie sees it)
func sniSpecificityRank(patterns []string) int {
	if len(patterns) == 0 {
		return 3
	}
	rank := 4
	for _, p := range patterns {
		switch {
		case p == "*":
			if 2 < rank {
				rank = 2
			}
		case strings.HasPrefix(p, "*."):
			if 1 < rank {
				rank = 1
			}
		default:
			return 0
		}
	}
	return rank
}
