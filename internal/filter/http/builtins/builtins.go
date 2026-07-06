// Package builtins registers the twenty built-in HTTP filters (router,
// adaptive_concurrency, admission_control, bandwidthlimit, buffer,
// compressor, cors, csrf, envoygotest, extauthz, extproc, fault,
// header_mutation, jwtauthn, localratelimit, lua, oauth2, ratelimit, rbac,
// wasm) plus their five per-route validators (header_mutation, oauth2, lua,
// ratelimit, wasm) into an *filter_http.HTTPRegistry. Unlike
// internal/filter/network/builtins, no Deps struct is needed: HTTP filter
// construction defers all boot-singleton injection (ClusterManager,
// DrainManager, HTTPClient, TracingExporters) to per-chain build time via
// hcm.ListenerCtx/FactoryCtx, not to registration time — none of the 20
// Register calls below captures a boot singleton in a closure (phase 51,
// ADR-0268).
package builtins

import (
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/http/adaptive_concurrency"
	"github.com/pgdad/envoy-go/internal/filter/http/admission_control"
	"github.com/pgdad/envoy-go/internal/filter/http/bandwidthlimit"
	"github.com/pgdad/envoy-go/internal/filter/http/buffer"
	"github.com/pgdad/envoy-go/internal/filter/http/compressor"
	"github.com/pgdad/envoy-go/internal/filter/http/cors"
	"github.com/pgdad/envoy-go/internal/filter/http/csrf"
	"github.com/pgdad/envoy-go/internal/filter/http/envoygotest"
	"github.com/pgdad/envoy-go/internal/filter/http/extauthz"
	"github.com/pgdad/envoy-go/internal/filter/http/extproc"
	"github.com/pgdad/envoy-go/internal/filter/http/fault"
	"github.com/pgdad/envoy-go/internal/filter/http/header_mutation"
	"github.com/pgdad/envoy-go/internal/filter/http/jwtauthn"
	"github.com/pgdad/envoy-go/internal/filter/http/localratelimit"
	"github.com/pgdad/envoy-go/internal/filter/http/lua"
	"github.com/pgdad/envoy-go/internal/filter/http/oauth2"
	"github.com/pgdad/envoy-go/internal/filter/http/ratelimit"
	"github.com/pgdad/envoy-go/internal/filter/http/rbac"
	"github.com/pgdad/envoy-go/internal/filter/http/router"
	"github.com/pgdad/envoy-go/internal/filter/http/wasm"
)

// RegisterBuiltins registers the twenty built-in HTTP filters and their five
// per-route validators into reg. It mirrors the registration calls in
// cmd/envoy-go/main.go and does NOT Freeze (the caller freezes after any
// additional registration).
func RegisterBuiltins(reg *filter_http.HTTPRegistry) {
	reg.Register(router.TypeURL, router.New)
	reg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)
	reg.Register(admission_control.TypeURL, admission_control.New)
	reg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)
	reg.Register(buffer.TypeURL, buffer.New)
	reg.Register(compressor.TypeURL, compressor.New)
	reg.Register(cors.TypeURL, cors.New)
	reg.Register(csrf.TypeURL, csrf.New)
	reg.Register(envoygotest.TypeURL, envoygotest.New)
	reg.Register(extauthz.TypeURL, extauthz.New)
	reg.Register(extproc.TypeURL, extproc.New)
	reg.Register(fault.TypeURL, fault.New)
	reg.Register(header_mutation.TypeURL, header_mutation.New)
	reg.Register(jwtauthn.TypeURL, jwtauthn.New)
	reg.Register(localratelimit.TypeURL, localratelimit.New)
	reg.Register(lua.TypeURL, lua.New)
	reg.Register(oauth2.TypeURL, oauth2.New)
	reg.Register(ratelimit.TypeURL, ratelimit.New)
	reg.Register(rbac.TypeURL, rbac.New)
	reg.Register(wasm.TypeURL, wasm.New)
	header_mutation.RegisterPerRouteValidator(reg)
	oauth2.RegisterPerRouteValidator(reg)
	lua.RegisterPerRouteValidator(reg)
	ratelimit.RegisterPerRouteValidator(reg)
	wasm.RegisterPerRouteValidator(reg)
}
