package helpers

import (
	"net/http"
	"strings"
)

// PhaseFourHTTPAllowList is the default allow-list for the phase-04 HTTP/1.1
// fixture (0003). Each entry is matched case-insensitively against the
// header name. An entry ending in '*' is a prefix-allow (e.g., "x-envoy-*"
// matches "x-envoy-attempt-count" and "x-envoy-expected-rq-timeout-ms").
// Codified in ADR-0044.
var PhaseFourHTTPAllowList = []string{
	"date",
	"server",
	"content-length",
	"transfer-encoding",
	"x-envoy-*",
	"x-forwarded-*",
	"x-request-id",
}

// HTTPHeaderDiff returns the symmetric difference of header names between
// refHeaders and subjHeaders, after lowercasing and applying allowList.
// refOnly contains lowercased names present in ref but not in subj (and not
// allow-listed). subjOnly is the mirror.
//
// allowList entries are matched case-insensitively; an entry ending in '*'
// is a prefix-allow.
func HTTPHeaderDiff(refHeaders, subjHeaders http.Header, allowList []string) (refOnly, subjOnly []string) {
	allowed := func(name string) bool {
		lname := strings.ToLower(name)
		for _, e := range allowList {
			le := strings.ToLower(e)
			if strings.HasSuffix(le, "*") {
				if strings.HasPrefix(lname, strings.TrimSuffix(le, "*")) {
					return true
				}
				continue
			}
			if lname == le {
				return true
			}
		}
		return false
	}
	subjSet := map[string]struct{}{}
	for k := range subjHeaders {
		subjSet[strings.ToLower(k)] = struct{}{}
	}
	refSet := map[string]struct{}{}
	for k := range refHeaders {
		refSet[strings.ToLower(k)] = struct{}{}
	}
	for k := range refSet {
		if _, ok := subjSet[k]; ok {
			continue
		}
		if allowed(k) {
			continue
		}
		refOnly = append(refOnly, k)
	}
	for k := range subjSet {
		if _, ok := refSet[k]; ok {
			continue
		}
		if allowed(k) {
			continue
		}
		subjOnly = append(subjOnly, k)
	}
	return refOnly, subjOnly
}
