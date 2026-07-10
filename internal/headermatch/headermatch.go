package headermatch

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

// Lowercase returns a fresh http.Header whose keys are all lowercase. The input
// is never mutated, and no value slice is aliased. Pseudo-header keys (":method")
// are already lowercase and pass through unchanged.
//
// Two source keys that fold to the same lowercase key (e.g. "X-Tap" and "x-tap")
// have their values MERGED, in map-iteration order. That order is not
// deterministic, but HCM never produces such a pair: it canonicalizes ordinary
// headers and writes pseudo-headers by raw assignment.
func Lowercase(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, v := range h {
		lk := strings.ToLower(k)
		// append onto a nil/existing slice copies the elements; it never aliases v.
		out[lk] = append(out[lk], v...)
	}
	return out
}

type hmKind int

const (
	hmString hmKind = iota // exact/prefix/suffix/contains/safe_regex/string_match, all via stringMatcher
	hmPresent
	hmRange
)

// Matcher is a compiled config/route/v3.HeaderMatcher.
type Matcher struct {
	name         string // lowercased
	kind         hmKind
	sm           *stringMatcher
	presentWant  bool
	rangeStart   int64
	rangeEnd     int64
	invert       bool
	treatMissing bool
}

// New compiles one HeaderMatcher. All 8 HeaderMatchSpecifier arms are accepted
// (the 5 deprecated ones included); an unset oneof is an error.
func New(hm *routev3.HeaderMatcher) (*Matcher, error) {
	if hm == nil {
		return nil, errors.New("headermatch: nil HeaderMatcher")
	}
	if hm.GetName() == "" {
		return nil, errors.New("headermatch: HeaderMatcher.name is required")
	}
	m := &Matcher{
		name:         strings.ToLower(hm.GetName()),
		invert:       hm.GetInvertMatch(),
		treatMissing: hm.GetTreatMissingHeaderAsEmpty(),
	}
	//nolint:staticcheck // intentionally supporting deprecated Envoy header matcher arms
	switch a := hm.GetHeaderMatchSpecifier().(type) {
	case *routev3.HeaderMatcher_ExactMatch: // f4 (deprecated)
		m.kind, m.sm = hmString, &stringMatcher{kind: smExact, s: a.ExactMatch}
	case *routev3.HeaderMatcher_PrefixMatch: // f9 (deprecated)
		m.kind, m.sm = hmString, &stringMatcher{kind: smPrefix, s: a.PrefixMatch}
	case *routev3.HeaderMatcher_SuffixMatch: // f10 (deprecated)
		m.kind, m.sm = hmString, &stringMatcher{kind: smSuffix, s: a.SuffixMatch}
	case *routev3.HeaderMatcher_ContainsMatch: // f12 (deprecated)
		m.kind, m.sm = hmString, &stringMatcher{kind: smContains, s: a.ContainsMatch}
	case *routev3.HeaderMatcher_SafeRegexMatch: // f11 (deprecated)
		// NOTE: this arm carries a bare *tmatcherv3.RegexMatcher, NOT a
		// StringMatcher — compile it directly. safe_regex is a FULL match.
		if a.SafeRegexMatch == nil {
			return nil, errors.New("headermatch: nil safe_regex_match")
		}
		re, err := regexp.Compile("^(?:" + a.SafeRegexMatch.GetRegex() + ")$")
		if err != nil {
			return nil, fmt.Errorf("headermatch: safe_regex_match: %w", err)
		}
		m.kind, m.sm = hmString, &stringMatcher{kind: smSafeRegex, re: re}
	case *routev3.HeaderMatcher_StringMatch: // f13
		sm, err := newStringMatcher(a.StringMatch)
		if err != nil {
			return nil, err
		}
		m.kind, m.sm = hmString, sm
	case *routev3.HeaderMatcher_PresentMatch: // f7
		m.kind, m.presentWant = hmPresent, a.PresentMatch
	case *routev3.HeaderMatcher_RangeMatch: // f6
		if a.RangeMatch == nil {
			return nil, errors.New("headermatch: nil range_match")
		}
		m.kind = hmRange
		m.rangeStart, m.rangeEnd = a.RangeMatch.GetStart(), a.RangeMatch.GetEnd()
	case nil:
		return nil, errors.New("headermatch: HeaderMatcher has no header_match_specifier set")
	default:
		return nil, fmt.Errorf("headermatch: unsupported header_match_specifier arm %T", a)
	}
	return m, nil
}

// Match reports whether h satisfies the matcher. h MUST be lowercase-keyed
// (see Lowercase). Raw map indexing is used so pseudo-headers are visible.
func (m *Matcher) Match(h http.Header) bool {
	vals, present := h[m.name]
	var res bool
	switch {
	case m.kind == hmPresent:
		res = present == m.presentWant
	case !present && !m.treatMissing:
		res = false
	default:
		v := ""
		if present {
			v = strings.Join(vals, ",")
		}
		res = m.evalValue(v)
	}
	if m.invert {
		return !res
	}
	return res
}

func (m *Matcher) evalValue(v string) bool {
	switch m.kind {
	case hmString:
		return m.sm.match(v)
	case hmRange:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return false
		}
		return n >= m.rangeStart && n < m.rangeEnd
	}
	return false
}
