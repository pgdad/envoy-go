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

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
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
	// Per-route validator registration happens BEFORE Freeze() in
	// cmd/envoy-go/main.go via RegisterPerRouteValidator (exported). Do NOT
	// call it here: New is invoked by the listener constructor which runs AFTER
	// Freeze, so any call here would panic. The RegisterPerRouteValidator
	// function exists so main.go can wire it pre-Freeze cleanly.
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

// RegisterPerRouteValidator registers the header_mutation per-route validator
// with the supplied HTTPRegistry. MUST be called BEFORE httpReg.Freeze() in
// cmd/envoy-go/main.go — the registry rejects registrations after Freeze, and
// New is called during listener construction (after Freeze). Introduced to
// correct the Task 9 design where New attempted to register post-Freeze.
func RegisterPerRouteValidator(reg interface {
	RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)
}) {
	reg.RegisterPerRouteValidator(filterName, validatePerRouteHeaderMutation)
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

// applyOps iterates ops in proto-declared order, applying each to headers.
// Per ADR-0109 + SPEC §6.6.
func applyOps(headers http.Header, ops []compiledMutationOp) {
	for _, op := range ops {
		switch op.kind {
		case kindRemove:
			headers.Del(op.headerName)
		case kindAppend:
			applyAppendAction(headers, op)
		}
	}
}

// applyAppendAction implements the AppendAction × 4 + keep_empty_value
// boundary per SPEC §6.6 + §11.2 + ADR-0109. The keep_empty_value=false
// silent-skip on empty value fires FIRST, BEFORE the AppendAction switch
// (per §11.2 conclusion (c)).
func applyAppendAction(headers http.Header, op compiledMutationOp) {
	if op.headerValue == "" && !op.keepEmptyValue {
		return // silent skip per §11.2
	}
	switch op.appendAction {
	case corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD:
		headers.Add(op.headerName, op.headerValue)
	case corev3.HeaderValueOption_ADD_IF_ABSENT:
		if headers.Get(op.headerName) == "" {
			headers.Add(op.headerName, op.headerValue)
		}
	case corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD:
		headers.Set(op.headerName, op.headerValue)
	case corev3.HeaderValueOption_OVERWRITE_IF_EXISTS:
		if headers.Get(op.headerName) != "" {
			headers.Set(op.headerName, op.headerValue)
		}
	}
}

// compileForRequest projects a per-route HeaderMutationPerRoute proto.Message
// into a request-mutations slice. Sub-microsecond on small per-route configs
// (typical: <5 ops per tier). Per planner-time decision 2 we re-compile fresh
// per request rather than caching; the cost is negligible.
//
// Returns nil for nil input or for messages that fail the type-assertion (the
// per-route validator at HCM-build time per ADR-0111 already rejects per-route
// configs containing protected-header mutations, so the compileOps call here
// is expected to succeed; defensive on error → return nil).
func (f *filter) compileForRequest(msg proto.Message) []compiledMutationOp {
	if msg == nil {
		return nil
	}
	pr, ok := msg.(*headermutationv3.HeaderMutationPerRoute)
	if !ok {
		return nil
	}
	m := pr.GetMutations()
	if m == nil {
		return nil
	}
	ops, err := compileOps(m.GetRequestMutations())
	if err != nil {
		// Per-route validator at HCM-build time rejects this case; defensive return.
		return nil
	}
	return ops
}

// compileForResponse projects a per-route HeaderMutationPerRoute proto.Message
// into a response-mutations slice. Symmetric to compileForRequest.
func (f *filter) compileForResponse(msg proto.Message) []compiledMutationOp {
	if msg == nil {
		return nil
	}
	pr, ok := msg.(*headermutationv3.HeaderMutationPerRoute)
	if !ok {
		return nil
	}
	m := pr.GetMutations()
	if m == nil {
		return nil
	}
	ops, err := compileOps(m.GetResponseMutations())
	if err != nil {
		return nil
	}
	return ops
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

// DecodeHeaders implements the header_mutation filter's decode-side discipline
// per SPEC §6.6 + ADR-0109 + ADR-0110. Apply listener-level cfg.requestOps
// FIRST (per the proto comment at header_mutation.pb.go:141–142), then
// per-route tiers in flag-controlled order (per §6.5 algorithm + §11.5
// empirical confirmation).
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	applyOps(headers, f.cfg.requestOps)
	if f.dcb == nil {
		return envoyhttp.Continue
	}
	routeMsg, vhMsg, rcMsg := f.dcb.RequestRouteConfigsAllTiers()
	routeOps := f.compileForRequest(routeMsg)
	vhOps := f.compileForRequest(vhMsg)
	rcOps := f.compileForRequest(rcMsg)

	if !f.cfg.mostSpecificHeaderMutationsWins {
		// DEFAULT (flag=false): Route → VHost → RC; least-specific wins overlap
		applyOps(headers, routeOps)
		applyOps(headers, vhOps)
		applyOps(headers, rcOps)
	} else {
		// flag=true: RC → VHost → Route; most-specific wins overlap
		applyOps(headers, rcOps)
		applyOps(headers, vhOps)
		applyOps(headers, routeOps)
	}
	return envoyhttp.Continue
}

// EncodeHeaders implements the header_mutation filter's encode-side discipline
// per SPEC §6.8 — symmetric to DecodeHeaders modulo (a) reads cfg.responseOps;
// (b) compiles response-side mutations via compileForResponse; (c) uses the
// SAME f.dcb.RequestRouteConfigsAllTiers callback (DECODER-ONLY per planner-
// time decision 1; the dcb is set during chain wiring regardless of decode
// vs encode firing — mirrors cors precedent at cors.go:163).
func (f *filter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	applyOps(headers, f.cfg.responseOps)
	if f.dcb == nil {
		return envoyhttp.Continue
	}
	routeMsg, vhMsg, rcMsg := f.dcb.RequestRouteConfigsAllTiers()
	routeOps := f.compileForResponse(routeMsg)
	vhOps := f.compileForResponse(vhMsg)
	rcOps := f.compileForResponse(rcMsg)

	if !f.cfg.mostSpecificHeaderMutationsWins {
		applyOps(headers, routeOps)
		applyOps(headers, vhOps)
		applyOps(headers, rcOps)
	} else {
		applyOps(headers, rcOps)
		applyOps(headers, vhOps)
		applyOps(headers, routeOps)
	}
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
