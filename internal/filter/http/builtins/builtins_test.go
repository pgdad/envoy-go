package builtins

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency"
	"github.com/esalaine/envoy-go/internal/filter/http/admission_control"
	"github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit"
	"github.com/esalaine/envoy-go/internal/filter/http/buffer"
	"github.com/esalaine/envoy-go/internal/filter/http/compressor"
	"github.com/esalaine/envoy-go/internal/filter/http/cors"
	"github.com/esalaine/envoy-go/internal/filter/http/csrf"
	"github.com/esalaine/envoy-go/internal/filter/http/envoygotest"
	"github.com/esalaine/envoy-go/internal/filter/http/extauthz"
	"github.com/esalaine/envoy-go/internal/filter/http/extproc"
	"github.com/esalaine/envoy-go/internal/filter/http/fault"
	"github.com/esalaine/envoy-go/internal/filter/http/header_mutation"
	"github.com/esalaine/envoy-go/internal/filter/http/jwtauthn"
	"github.com/esalaine/envoy-go/internal/filter/http/localratelimit"
	"github.com/esalaine/envoy-go/internal/filter/http/lua"
	"github.com/esalaine/envoy-go/internal/filter/http/oauth2"
	"github.com/esalaine/envoy-go/internal/filter/http/ratelimit"
	"github.com/esalaine/envoy-go/internal/filter/http/rbac"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/filter/http/wasm"
)

func TestRegisterBuiltins_AllTwentyTypeURLsResolve(t *testing.T) {
	reg := filter_http.NewHTTPRegistry()
	RegisterBuiltins(reg)

	wantTypeURLs := []string{
		router.TypeURL, adaptive_concurrency.TypeURL, admission_control.TypeURL,
		bandwidthlimit.TypeURL, buffer.TypeURL, compressor.TypeURL, cors.TypeURL,
		csrf.TypeURL, envoygotest.TypeURL, extauthz.TypeURL, extproc.TypeURL,
		fault.TypeURL, header_mutation.TypeURL, jwtauthn.TypeURL,
		localratelimit.TypeURL, lua.TypeURL, oauth2.TypeURL, ratelimit.TypeURL,
		rbac.TypeURL, wasm.TypeURL,
	}
	if got, want := len(reg.KnownTypeURLs()), len(wantTypeURLs); got != want {
		t.Fatalf("KnownTypeURLs(): got %d entries, want %d", got, want)
	}
	for _, tu := range wantTypeURLs {
		if _, ok := reg.Lookup(tu); !ok {
			t.Errorf("Lookup(%q): not registered", tu)
		}
	}
}

func TestRegisterBuiltins_FivePerRouteValidatorsRegistered(t *testing.T) {
	reg := filter_http.NewHTTPRegistry()
	RegisterBuiltins(reg)

	if v := reg.PerRouteValidator("envoy.filters.http.header_mutation"); v == nil {
		t.Error("header_mutation: no per-route validator registered")
	}
	if v := reg.PerRouteValidator("envoy.filters.http.oauth2"); v == nil {
		t.Error("oauth2: no per-route validator registered")
	}
	if v := reg.PerRouteValidator("envoy.filters.http.lua"); v == nil {
		t.Error("lua: no per-route validator registered")
	}
	if v := reg.PerRouteValidator("envoy.filters.http.ratelimit"); v == nil {
		t.Error("ratelimit: no per-route validator registered")
	}
	if v := reg.PerRouteValidator("envoy.filters.http.wasm"); v == nil {
		t.Error("wasm: no per-route validator registered")
	}
	// A filter with NO per-route validator (e.g. router) must return nil, not panic.
	if v := reg.PerRouteValidator("envoy.filters.http.router"); v != nil {
		t.Error("router: expected no per-route validator, got one")
	}
}

func TestRegisterBuiltins_DoesNotFreeze(t *testing.T) {
	reg := filter_http.NewHTTPRegistry()
	RegisterBuiltins(reg)
	// RegisterBuiltins must NOT call Freeze — the caller freezes. Prove the
	// registry still accepts a Register call after RegisterBuiltins returns.
	reg.Register("type.googleapis.com/test.PostRegisterProbe", func(_ *anypb.Any, _ filter_http.FactoryCtx) (filter_http.FilterInstanceFactory, error) {
		return nil, nil
	})
}
