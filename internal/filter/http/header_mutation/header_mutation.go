package header_mutation

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	commonmutationrulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	headermutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// TypeURL is the canonical envoy.filters.http.header_mutation typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go (Task 9) registers New under this key
// in the HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation"

// filterName is the canonical http_filters[].name string for header_mutation
// (matches the listener config typed_per_filter_config map keys).
const filterName = "envoy.filters.http.header_mutation"

// runtimeConfig is the per-instance parsed config shape per ADR-0109.
//
// Three fields consumed at request-eval time (listener-level requestOps,
// listener-level responseOps, mostSpecificHeaderMutationsWins); one
// HeaderMutation field silently ignored per ADR-0112 deferral
// (mutations.query_parameter_mutations).
type runtimeConfig struct {
	requestOps                      []compiledMutationOp // listener-level request mutations (proto-declared order)
	responseOps                     []compiledMutationOp // listener-level response mutations (proto-declared order)
	mostSpecificHeaderMutationsWins bool                 // precedence-order flag (default false)
}

// mutationOpKind is the discriminator for compiledMutationOp.
type mutationOpKind uint8

const (
	kindRemove mutationOpKind = iota
	kindAppend
)

// compiledMutationOp is the flat per-mutation struct produced by compileOps.
// Value-typed per planner-time decision 4 for cache locality during the
// apply-loop iteration. Read-only after New / per-route compile.
type compiledMutationOp struct {
	kind           mutationOpKind                              // kindRemove or kindAppend
	headerName     string                                      // canonicalized via http.CanonicalHeaderKey at parse time
	headerValue    string                                      // for kindAppend only ("" for kindRemove)
	appendAction   corev3.HeaderValueOption_HeaderAppendAction // 4 variants; for kindAppend only
	keepEmptyValue bool                                        // for kindAppend only
}

// New is the HTTPFilterFactory exposed at boot. Per ADR-0108 + ADR-0109 + ADR-0111:
//
//  1. tc must be non-nil (a header_mutation filter with no typed_config has
//     no behavioral effect; surface configuration mistakes at boot per
//     ADR-0072 boot-time-fail-fast).
//  2. Unmarshal tc to *headermutationv3.HeaderMutation; return error on
//     malformed Any.
//  3. Compile listener-level request + response mutations via compileOps;
//     each headerName validated against the protected set per §11.1.
//  4. Capture most_specific_header_mutations_wins from the proto.
//  5. Register per-route validator with ctx.Registry per planner-time
//     decision 3 (so BuildPerRouteConfig surfaces per-route protected-header
//     violations as boot-time errors, mirroring listener-level discipline).
//  6. Return FilterInstanceFactory closure that allocates a fresh *filter
//     per request bound to *runtimeConfig.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("header_mutation: typed_config required")
	}
	var c headermutationv3.HeaderMutation
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, fmt.Errorf("header_mutation: unmarshal: %w", err)
	}
	rc, err := buildRuntimeConfig(&c)
	if err != nil {
		return nil, err
	}
	// Register per-route validator (per planner-time decision 3 + ADR-0110).
	// Idempotent across multiple calls to New (same filter, multiple HCMs):
	// RegisterPerRouteValidator overwrites the entry, but the validator function
	// is identical so the overwrite is benign.
	if ctx.Registry != nil {
		ctx.Registry.RegisterPerRouteValidator(filterName, validatePerRouteHeaderMutation)
	}
	return func() envoyhttp.HTTPFilter {
		f := &filter{cfg: rc}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: f,
		}
	}, nil
}

// buildRuntimeConfig projects *HeaderMutation into the runtimeConfig shape per §6.2.
func buildRuntimeConfig(c *headermutationv3.HeaderMutation) (*runtimeConfig, error) {
	rc := &runtimeConfig{
		mostSpecificHeaderMutationsWins: c.GetMostSpecificHeaderMutationsWins(),
	}
	if m := c.GetMutations(); m != nil {
		ops, err := compileOps(m.GetRequestMutations())
		if err != nil {
			return nil, err
		}
		rc.requestOps = ops
		ops, err = compileOps(m.GetResponseMutations())
		if err != nil {
			return nil, err
		}
		rc.responseOps = ops
		// mutations.query_parameter_mutations silently ignored per ADR-0112.
	}
	return rc, nil
}

// compileOps projects []*HeaderMutation (the proto primitive in
// config/common/mutation_rules/v3) into []compiledMutationOp. Each input op
// must EITHER set Action.Remove (kindRemove) OR set Action.Append (kindAppend).
// Validates each headerName against the protected-header set per §11.1.
// Returns error on the first protected-header violation.
//
// Used by both:
//   - New: compiles listener-level mutations
//   - validatePerRouteHeaderMutation: compiles per-route HeaderMutationPerRoute
//     mutations to surface protected-header violations at HCM-build time
func compileOps(in []*commonmutationrulesv3.HeaderMutation) ([]compiledMutationOp, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]compiledMutationOp, 0, len(in))
	for _, m := range in {
		switch a := m.GetAction().(type) {
		case *commonmutationrulesv3.HeaderMutation_Remove:
			name := a.Remove
			if isProtectedHeader(name) {
				return nil, fmt.Errorf("header_mutation: %q is :-prefixed or host; may not be modified", name)
			}
			out = append(out, compiledMutationOp{
				kind:       kindRemove,
				headerName: http.CanonicalHeaderKey(name),
			})
		case *commonmutationrulesv3.HeaderMutation_Append:
			hvo := a.Append
			if hvo == nil || hvo.GetHeader() == nil {
				continue // defensive: empty Append is a no-op
			}
			name := hvo.GetHeader().GetKey()
			if isProtectedHeader(name) {
				return nil, fmt.Errorf("header_mutation: %q is :-prefixed or host; may not be modified", name)
			}
			out = append(out, compiledMutationOp{
				kind:           kindAppend,
				headerName:     http.CanonicalHeaderKey(name),
				headerValue:    hvo.GetHeader().GetValue(),
				appendAction:   hvo.GetAppendAction(),
				keepEmptyValue: hvo.GetKeepEmptyValue(),
			})
		default:
			// Unknown / unset action — defensive skip.
			continue
		}
	}
	return out, nil
}

// isProtectedHeader returns true if name is in the 6-name protected set per §11.1.
//
// Per planner-time decision 5: prefix-check on `:` future-proofs against new
// pseudo-headers (Envoy may add `:protocol` or `:upgrade` later); equality
// on `host` is case-insensitive (Envoy rejects `host`, `Host`, `HOST`
// symmetrically per §11.1 conclusion (b)).
func isProtectedHeader(name string) bool {
	if strings.HasPrefix(name, ":") {
		return true
	}
	return strings.EqualFold(name, "host")
}

// validatePerRouteHeaderMutation is the per-route validator registered with
// the framework's *HTTPRegistry per planner-time decision 3 + ADR-0110.
// At HCM-build time, BuildPerRouteConfig invokes this against each parsed
// HeaderMutationPerRoute proto.Message at each tier (Route, VirtualHost,
// RouteConfiguration). Returns the first protected-header violation as an
// error; framework wraps with location prefix.
func validatePerRouteHeaderMutation(msg proto.Message) error {
	pr, ok := msg.(*headermutationv3.HeaderMutationPerRoute)
	if !ok {
		// Defensive: should not happen if BuildPerRouteConfig parses the
		// typed_config Any to *HeaderMutationPerRoute correctly.
		return nil
	}
	m := pr.GetMutations()
	if m == nil {
		return nil
	}
	if _, err := compileOps(m.GetRequestMutations()); err != nil {
		return err
	}
	if _, err := compileOps(m.GetResponseMutations()); err != nil {
		return err
	}
	return nil
}

// filter is the per-request header_mutation instance. Per-instance state is
// race-free by the single-goroutine-per-stream invariant per ADR-0071.
//
// Per-route configs are resolved per-request via f.dcb.RequestRouteConfigsAllTiers
// (Tasks 7/8 land the resolution + apply-loop). Task 5 lands the stub
// DecodeHeaders/EncodeHeaders that simply return Continue.
type filter struct {
	cfg *runtimeConfig

	dcb envoyhttp.DecoderFilterCallbacks
	ecb envoyhttp.EncoderFilterCallbacks
}

// Statically assert the both-sides interface conformance (matches cors + fault precedents).
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// DecodeHeaders is STUBBED in Task 5. Task 7 lands the full body per SPEC §6.6.
func (f *filter) DecodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}

// EncodeHeaders is STUBBED in Task 5. Task 8 lands the full body per SPEC §6.8.
func (f *filter) EncodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}

// Pass-through methods (data + trailers + OnDestroy).
func (f *filter) DecodeData([]byte, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) EncodeData([]byte, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (f *filter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (f *filter) OnDestroy() {} // no timers, no async state — nothing to release
