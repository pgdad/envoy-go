package tracing

import (
	"bytes"
	"encoding/json"
	"os"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

// ResolveCustomTags resolves the ordered (already first-wins-deduped) specs
// against a per-request header lookup into span attributes. A literal spec yields
// its static value; a request_header spec yields the FIRST value of the named
// header (SPEC-62 §11 D-RH-MULTIVALUE), or the DefaultValue when the header is
// absent and a default was configured, or NOTHING when the header is absent and no
// default was set (omit-on-missing — D-RH-MISSING). headerLookup may be nil (no
// request headers available), in which case request_header specs use default /
// omit. A metadata spec yields the serialized dynamic-metadata value at its
// namespace+path (metaLookup, present-empty EMITS "" per the request_header
// default rule), or the DefaultValue when the value is absent/unresolvable and a
// default was configured, or NOTHING otherwise. metaLookup may be nil (no
// dynamic-metadata bucket available), in which case metadata specs use default /
// omit. A kindMetadataRoute spec yields the serialized static route-config
// metadata value at its namespace+path (routeMetaLookup, same present-empty
// EMITS "" default rule), descending the FULL MetaPath from the WHOLE namespace
// struct routeMetaLookup returns (distinct from metaLookup's two-arg,
// pre-keyed-to-MetaPath[0] shape). routeMetaLookup may be nil (no route
// metadata available), in which case kindMetadataRoute specs use default / omit.
// A kindMetadataHost spec yields the serialized static metadata value of the
// SELECTED UPSTREAM ENDPOINT at its namespace+path (hostMetaLookup, same
// present-empty EMITS "" default rule), likewise descending the FULL MetaPath
// from the WHOLE namespace struct hostMetaLookup returns (the same one-arg shape
// as routeMetaLookup, NOT metaLookup's two-arg pre-keyed one). hostMetaLookup
// may be nil (no host lookup available), and picked may be the ZERO Endpoint —
// at the span-capable local-reply sites, and ALSO at the post-decision upstream
// emit sites (which forward the router's own picked) whenever the upstream
// ACQUIRE failed: circuit-breaker / pool-overflow / connect-failure / the
// defensive H2 grant-race, since doH1ClusterAction and doH2ClusterAction
// initialize picked := cluster.Endpoint{} and assign only AFTER the acquire
// succeeds. That second source is the phase-72 named departure B2; POST-acquire
// failures forward the REAL endpoint and resolve normally. Either way, a ZERO
// picked makes kindMetadataHost specs use default / omit.
// The returned []KV has unique keys (the specs were deduped at parse), so
// BuildServerSpan's upsert only ever overrides a colliding BUILT-IN.
func ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool), metaLookup func(ns, key string) (*structpb.Value, bool), routeMetaLookup func(ns string) (*structpb.Value, bool), hostMetaLookup func(ns string) (*structpb.Value, bool)) []KV {
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
		case kindMetadata:
			// Mirror kindRequestHeader's default rule (NOT the kindEnvironment
			// omit-on-empty rule): a present, resolvable value EMITS its serialized
			// form (incl. a present-empty string ""); an absent/unresolvable/boundary
			// value uses the DefaultValue when configured, else omits. metaLookup may
			// be nil (no dynamic-metadata bucket) → default / omit. MetaPath[0] always
			// exists (validated at parse: min 1 segment).
			var v *structpb.Value
			var ok bool
			if metaLookup != nil {
				v, ok = metaLookup(s.MetaNamespace, s.MetaPath[0])
			}
			if ok {
				v, ok = descend(v, s.MetaPath[1:])
			}
			if ok {
				if str, emit := structpbValueToString(v); emit {
					out = append(out, KV{Key: s.Key, Str: str})
					continue
				}
			}
			if s.HasDefault {
				out = append(out, KV{Key: s.Key, Str: s.DefaultValue})
			} // else omit (append nothing)
		case kindMetadataRoute:
			// Mirror kindMetadata's default rule (present-empty EMITS ""), but the ROUTE
			// source yields the WHOLE namespace struct, so descend the FULL MetaPath
			// (the [1:] slice is a REQUEST-only Bucket-pre-keying artifact).
			// routeMetaLookup may be nil (no route metadata) → default / omit.
			var v *structpb.Value
			var ok bool
			if routeMetaLookup != nil {
				v, ok = routeMetaLookup(s.MetaNamespace)
			}
			if ok {
				v, ok = descend(v, s.MetaPath) // FULL path, NOT s.MetaPath[1:]
			}
			if ok {
				if str, emit := structpbValueToString(v); emit {
					out = append(out, KV{Key: s.Key, Str: str})
					continue
				}
			}
			if s.HasDefault {
				out = append(out, KV{Key: s.Key, Str: s.DefaultValue})
			} // else omit (append nothing)
		case kindMetadataHost:
			// Mirror kindMetadataRoute exactly: the HOST source yields the WHOLE
			// namespace struct (the selected upstream endpoint's
			// lb_endpoints[].metadata.filter_metadata[ns]), so descend the FULL
			// MetaPath (the [1:] slice is a REQUEST-only Bucket-pre-keying artifact).
			// hostMetaLookup may be nil, and picked may be the ZERO Endpoint — at
			// the 5 span-capable local-reply sites, and ALSO at the post-decision
			// upstream emit sites in connection.go / h2dispatch.go / h3dispatch.go
			// (which forward rf.Picked()) whenever the upstream ACQUIRE failed,
			// since doH1ClusterAction / doH2ClusterAction initialize
			// picked := cluster.Endpoint{} and assign only AFTER the acquire
			// succeeds — the phase-72 departure B2. POST-acquire failures forward
			// the REAL endpoint and resolve normally. → default / omit.
			var v *structpb.Value
			var ok bool
			if hostMetaLookup != nil {
				v, ok = hostMetaLookup(s.MetaNamespace)
			}
			if ok {
				v, ok = descend(v, s.MetaPath) // FULL path, NOT s.MetaPath[1:]
			}
			if ok {
				if str, emit := structpbValueToString(v); emit {
					out = append(out, KV{Key: s.Key, Str: str})
					continue
				}
			}
			if s.HasDefault {
				out = append(out, KV{Key: s.Key, Str: s.DefaultValue})
			} // else omit (append nothing)
		}
	}
	return out
}

// descend walks v through the sequence of struct-field segment keys in segs
// (a []string clone of the ratelimit descendStructpbValue path-walk shape —
// NOT imported, so internal/tracing stays filter-free). Each non-terminal
// segment requires the current value to be a StructValue; a non-struct
// intermediate, a missing field, or a nil field breaks the chain and returns
// (nil, false). An empty segs returns (v, true) — the terminal is v itself.
func descend(v *structpb.Value, segs []string) (*structpb.Value, bool) {
	cur := v
	for _, seg := range segs {
		st := cur.GetStructValue()
		if st == nil {
			return nil, false
		}
		next, ok := st.GetFields()[seg]
		if !ok || next == nil {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// structpbValueToString serializes a resolved dynamic-metadata value to its span
// string form (the P3 table, SPEC §11): a StringValue yields its raw text
// (including "" — the present-empty EMIT edge); a NullValue is a boundary that
// falls to default/omit (false); every other kind (number / bool / struct / list)
// serializes via protojson.Marshal + json.Compact. json.Compact is LOAD-BEARING:
// it strips protojson's detrand whitespace so a list marshals to the stable
// ["x","y","z"] (not ["x", "y", "z"]).
func structpbValueToString(v *structpb.Value) (string, bool) {
	switch k := v.GetKind().(type) {
	case *structpb.Value_StringValue:
		return k.StringValue, true // raw, incl. "" (present-empty EMIT)
	case *structpb.Value_NullValue:
		return "", false // boundary → default/omit
	default:
		b, err := protojson.Marshal(v)
		if err != nil {
			return "", false
		}
		var buf bytes.Buffer
		if err := json.Compact(&buf, b); err != nil {
			return "", false
		}
		return buf.String(), true
	}
}
