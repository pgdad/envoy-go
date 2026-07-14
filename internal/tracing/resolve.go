package tracing

import "os"

// ResolveCustomTags resolves the ordered (already first-wins-deduped) specs
// against a per-request header lookup into span attributes. A literal spec yields
// its static value; a request_header spec yields the FIRST value of the named
// header (SPEC-62 §11 D-RH-MULTIVALUE), or the DefaultValue when the header is
// absent and a default was configured, or NOTHING when the header is absent and no
// default was set (omit-on-missing — D-RH-MISSING). headerLookup may be nil (no
// request headers available), in which case request_header specs use default /
// omit. The returned []KV has unique keys (the specs were deduped at parse), so
// BuildServerSpan's upsert only ever overrides a colliding BUILT-IN.
func ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool)) []KV {
	if len(specs) == 0 {
		return nil
	}
	out := make([]KV, 0, len(specs))
	for _, s := range specs {
		switch s.Kind {
		case kindLiteral:
			out = append(out, KV{Key: s.Key, Str: s.LiteralValue})
		case kindRequestHeader:
			if headerLookup != nil {
				// The lookup's bool is TRUE for a present header even with an empty
				// value, so a present empty-valued header emits KV{Key, ""} (present),
				// NOT the default (SPEC §2 modeled edge).
				if vs, ok := headerLookup(s.HeaderName); ok && len(vs) > 0 {
					out = append(out, KV{Key: s.Key, Str: vs[0]}) // FIRST value
					continue
				}
			}
			if s.HasDefault {
				out = append(out, KV{Key: s.Key, Str: s.DefaultValue})
			} // else omit (append nothing)
		case kindEnvironment:
			// The env is process-STATIC; os.LookupEnv reports present-ness so a
			// PRESENT-but-EMPTY var ("") is distinguished from an ABSENT one
			// (D-ENV-EMPTYVAL, SPEC §11 arm G). Resolved value = the env value if
			// present, else the DefaultValue. The tag is OMITTED iff the resolved
			// value is empty — a present-empty var, an absent var with no default,
			// and an absent var with an empty default all omit; only a NON-EMPTY
			// resolved value emits. headerLookup is IGNORED (an env tag needs no
			// request header). This DIVERGES from kindRequestHeader's present-empty
			// edge (which emits ""), a probe-justified difference (SPEC §3.3).
			v, present := os.LookupEnv(s.EnvName)
			if !present {
				v = s.DefaultValue
			}
			if v != "" {
				out = append(out, KV{Key: s.Key, Str: v})
			}
		}
	}
	return out
}
