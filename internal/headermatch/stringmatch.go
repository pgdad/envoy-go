// Package headermatch evaluates Envoy `config/route/v3.HeaderMatcher` protos
// against a request/response header map.
//
// CONTRACT: every Match/match call takes a LOWERCASE-KEYED http.Header. Use
// Lowercase to normalize before matching. Raw map indexing is used throughout
// (never http.Header.Get), because Go's textproto canonicalization does not
// preserve the leading colon of pseudo-headers such as ":status".
package headermatch

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	tmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

type smKind int

const (
	smExact smKind = iota
	smPrefix
	smSuffix
	smContains
	smSafeRegex
)

// stringMatcher is a compiled type/matcher/v3.StringMatcher.
type stringMatcher struct {
	kind       smKind
	s          string // already case-folded when ignoreCase
	re         *regexp.Regexp
	ignoreCase bool
}

func newStringMatcher(sm *tmatcherv3.StringMatcher) (*stringMatcher, error) {
	if sm == nil {
		return nil, errors.New("headermatch: nil string_match")
	}
	out := &stringMatcher{ignoreCase: sm.GetIgnoreCase()}
	fold := func(s string) string {
		if out.ignoreCase {
			return strings.ToLower(s)
		}
		return s
	}
	switch p := sm.GetMatchPattern().(type) {
	case *tmatcherv3.StringMatcher_Exact:
		out.kind, out.s = smExact, fold(p.Exact)
	case *tmatcherv3.StringMatcher_Prefix:
		out.kind, out.s = smPrefix, fold(p.Prefix)
	case *tmatcherv3.StringMatcher_Suffix:
		out.kind, out.s = smSuffix, fold(p.Suffix)
	case *tmatcherv3.StringMatcher_Contains:
		out.kind, out.s = smContains, fold(p.Contains)
	case *tmatcherv3.StringMatcher_SafeRegex:
		// ignore_case is ignored for safe_regex (Envoy semantics).
		re, err := regexp.Compile("^(?:" + p.SafeRegex.GetRegex() + ")$")
		if err != nil {
			return nil, fmt.Errorf("headermatch: safe_regex: %w", err)
		}
		out.kind, out.re = smSafeRegex, re
	case *tmatcherv3.StringMatcher_Custom:
		return nil, errors.New("headermatch: string_match.custom is not supported")
	case nil:
		return nil, errors.New("headermatch: string_match has no match_pattern set")
	default:
		return nil, fmt.Errorf("headermatch: unsupported string_match arm %T", p)
	}
	return out, nil
}

func (m *stringMatcher) match(v string) bool {
	if m.kind == smSafeRegex {
		return m.re.MatchString(v)
	}
	if m.ignoreCase {
		v = strings.ToLower(v)
	}
	switch m.kind {
	case smExact:
		return v == m.s
	case smPrefix:
		return strings.HasPrefix(v, m.s)
	case smSuffix:
		return strings.HasSuffix(v, m.s)
	case smContains:
		return strings.Contains(v, m.s)
	}
	return false
}
