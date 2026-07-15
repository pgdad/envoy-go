package tracing

// BuildHTTPURL assembles the http.url span-attribute value scheme://host+pathAndQuery,
// byte-truncating pathAndQuery (the :path pseudo-header = path+query) to maxPathTagLen
// bytes FIRST — the reference max_path_tag_length semantics (D-MPTL-TARGET / -QUERY /
// -TRUNCUNIT, SPEC-64 §11). The scheme://host prefix is NEVER truncated. A cap of 0
// yields an empty path (scheme://host only, D-MPTL-ZERO). maxPathTagLen is the resolved
// cap from TracingConfig.MaxPathTagLength (default 256; ALWAYS set by NewConfig).
func BuildHTTPURL(scheme, host, pathAndQuery string, maxPathTagLen uint32) string {
	if len(pathAndQuery) > int(maxPathTagLen) { // int() cast: cap <= math.MaxUint32, no overflow on 64-bit
		pathAndQuery = pathAndQuery[:maxPathTagLen]
	}
	return scheme + "://" + host + pathAndQuery
}
