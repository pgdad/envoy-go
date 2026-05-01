package http

import "fmt"

// ChainShapeEntry is a (name, type_url) pair lifted from one entry in the
// HCM proto's http_filters[] slice. ValidateChainShape consumes a slice of
// these in declaration order. The (Name, TypeURL) pair is sufficient for the
// four chain-shape validation rules; the typed_config payload bytes are
// consumed by the per-entry factory invocation that the caller performs after
// ValidateChainShape returns.
type ChainShapeEntry struct {
	Name    string
	TypeURL string
}

// ValidateChainShape applies the four canonical http_filters[] chain-shape
// validations per SPEC §1 #6 and ADR-0071's partial supersession of ADR-0042:
//
//  1. Empty chain — at least 1 entry (the router) must be present.
//  2. Last entry must equal the (routerName, routerTypeURL) terminal pair.
//  3. Duplicate filter names anywhere in the chain.
//  4. Unknown type_url — every entry's TypeURL must resolve via registry.Lookup.
//
// All error messages begin with "hcm: " — caller (HCM config parser) uses the
// returned error verbatim. The 30s FuzzFilterChainParse_ChainShape gate (per
// ADR-0018) probes this function for panic-freedom and canonical error-prefix
// discipline.
//
// On success, returns the per-entry factory slice in declaration order; the
// caller then invokes each factory with its typed_config Any to allocate the
// HTTPFilterFactory closure. The hcm package's parseFilterWithCtx is the sole
// production caller (Task 13).
func ValidateChainShape(entries []ChainShapeEntry, registry *HTTPRegistry, routerName, routerTypeURL string) ([]HTTPFilterFactory, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("hcm: http_filters: must contain at least 1 entry (the router)")
	}
	last := entries[len(entries)-1]
	if last.Name != routerName || last.TypeURL != routerTypeURL {
		return nil, fmt.Errorf("hcm: http_filters: last entry must be %q (router); got %q (%s)", routerName, last.Name, last.TypeURL)
	}
	seen := make(map[string]struct{}, len(entries))
	factories := make([]HTTPFilterFactory, 0, len(entries))
	for i, e := range entries {
		if _, dup := seen[e.Name]; dup {
			return nil, fmt.Errorf("hcm: http_filters: duplicate filter name %q", e.Name)
		}
		seen[e.Name] = struct{}{}
		f, ok := registry.Lookup(e.TypeURL)
		if !ok {
			return nil, fmt.Errorf("hcm: http_filters[%d]: unknown type_url %q (registry: known are %v)", i, e.TypeURL, registry.KnownTypeURLs())
		}
		factories = append(factories, f)
	}
	return factories, nil
}
