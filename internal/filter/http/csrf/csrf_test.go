package csrf

import (
	"net/http"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	csrfv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// helper: wrap a *CsrfPolicy in *anypb.Any with the canonical type URL.
func mustAnyFrom(t *testing.T, c *csrfv3.CsrfPolicy) *anypb.Any {
	t.Helper()
	a, err := anypb.New(c)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func newTestFactoryCtx() envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{
		Stats:      stats.NewRegistry(),
		StatPrefix: "ingress_csrf",
	}
}

// Group 1 — New factory PGV-mirror validation.

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error on nil tc")
	}
}

func TestNew_MalformedTC(t *testing.T) {
	a := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	_, err := New(a, newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error on malformed Any")
	}
}

func TestNew_FilterEnabledNil_RejectAtParseTime(t *testing.T) {
	c := &csrfv3.CsrfPolicy{}
	_, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error: filter_enabled is required (per §11.11)")
	}
}

func TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{},
	}
	_, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error: filter_enabled.default_value is required (per §11.11)")
	}
}

func TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   0,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("expected accept (silent-ignore percentage); got %v", err)
	}
}

func TestNew_FilterEnabledHundredPercent_Accepted(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("happy-path New: %v", err)
	}
}

func TestNew_ShadowEnabledAbsent_Accepted(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("shadow_enabled absent should be OK: %v", err)
	}
}

func TestNew_ShadowEnabledPresent_SilentIgnored(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
		ShadowEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 50, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("shadow_enabled present should be OK (silent-ignored): %v", err)
	}
}

// Group 2 — additional_origins parse-time discipline.

func TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse(t *testing.T) {
	tests := []struct {
		name    string
		matcher *matcherv3.StringMatcher
	}{
		{"prefix", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "pfx-"}}},
		{"suffix", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Suffix{Suffix: "-sfx"}}},
		{"contains", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Contains{Contains: "mid"}}},
		{"safe_regex", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_SafeRegex{SafeRegex: nil}}},
		{"ignore_case_with_exact", &matcherv3.StringMatcher{IgnoreCase: true, MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "case-folded.test"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &csrfv3.CsrfPolicy{
				FilterEnabled:     &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}},
				AdditionalOrigins: []*matcherv3.StringMatcher{tt.matcher},
			}
			factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			rc := mustGetRuntimeConfig(t, factory)
			if len(rc.additionalOrigins) != 0 {
				t.Errorf("non-exact StringMatcher %q must be dropped at parse; got %v",
					tt.name, rc.additionalOrigins)
			}
		})
	}
}

func TestNew_AdditionalOrigins_EmptyExactValue_Dropped(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}},
		AdditionalOrigins: []*matcherv3.StringMatcher{
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: ""}},
		},
	}
	factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rc := mustGetRuntimeConfig(t, factory)
	if len(rc.additionalOrigins) != 0 {
		t.Errorf("empty-value exact entry must be dropped; got %v", rc.additionalOrigins)
	}
}

func TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}},
		AdditionalOrigins: []*matcherv3.StringMatcher{
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "app.example.test"}},
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "Other.Test:8443"}}, // case + port preserved
		},
	}
	factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rc := mustGetRuntimeConfig(t, factory)
	want := []string{"app.example.test", "Other.Test:8443"}
	if len(rc.additionalOrigins) != len(want) {
		t.Fatalf("len mismatch; got %v want %v", rc.additionalOrigins, want)
	}
	for i, w := range want {
		if rc.additionalOrigins[i] != w {
			t.Errorf("entry %d: got %q want %q (verbatim preservation)", i, rc.additionalOrigins[i], w)
		}
	}
}

// mustGetRuntimeConfig is a test-only helper that invokes the FilterInstanceFactory
// once and inspects the captured *runtimeConfig via the per-instance filter struct's
// rc field.
func mustGetRuntimeConfig(t *testing.T, factory envoyhttp.FilterInstanceFactory) *runtimeConfig {
	t.Helper()
	hf := factory()
	d, ok := hf.Decoder.(*filter)
	if !ok {
		t.Fatalf("Decoder is not *filter; got %T", hf.Decoder)
	}
	return d.rc
}

// Compile-time check: SPEC's documented public surface.
var (
	_ proto.Message               = (*csrfv3.CsrfPolicy)(nil)
	_ envoyhttp.HTTPFilterFactory = New
)

// Group 3 — DecodeHeaders non-modifying methods.

func TestDecodeHeaders_NonModifyingMethods(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "OPTIONS", "TRACE", "PROPFIND"} {
		t.Run(method, func(t *testing.T) {
			factory := mustNewListenerFactory(t, []string{"app.example.test"})
			f := factory().Decoder.(*filter)
			f.SetDecoderCallbacks(&fakeCallbacks{})
			h := http.Header{}
			h.Set(":method", method)
			h.Set("Host", "127.0.0.1:8080")
			h.Set("Origin", "https://evil.test") // would normally reject — but method short-circuits before any check
			status := f.DecodeHeaders(h, true)
			if status != envoyhttp.Continue {
				t.Errorf("non-modifying method %s: got %v want Continue", method, status)
			}
			rc := f.rc
			if rc.stats.requestValid.Load() != 0 || rc.stats.requestInvalid.Load() != 0 || rc.stats.missingSourceOrigin.Load() != 0 {
				t.Errorf("non-modifying method %s: counters touched (rv=%d ri=%d mso=%d)",
					method, rc.stats.requestValid.Load(), rc.stats.requestInvalid.Load(), rc.stats.missingSourceOrigin.Load())
			}
		})
	}
}

// Group 4 — origin extraction trichotomy.

func TestDecodeHeaders_OriginNullLiteral_MissingSourceOrigin_NoRefererFallback(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, cb := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "null")
	h.Set("Referer", "http://127.0.0.1:8080/page") // would otherwise rescue
	status := f.DecodeHeaders(h, true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration", status)
	}
	if f.rc.stats.missingSourceOrigin.Load() != 1 {
		t.Errorf("missing_source_origin should increment; got %d", f.rc.stats.missingSourceOrigin.Load())
	}
	if cb.localReply == nil || cb.localReply.status != 403 || cb.localReply.body != "Invalid origin" {
		t.Errorf("expected SendLocalReply(403, \"Invalid origin\"); got %+v", cb.localReply)
	}
}

func TestDecodeHeaders_OriginEmpty_RefererFallback(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "")
	h.Set("Referer", "http://127.0.0.1:8080/page")
	status := f.DecodeHeaders(h, true)
	if status != envoyhttp.Continue {
		t.Fatalf("got %v want Continue (Referer rescues)", status)
	}
	if f.rc.stats.requestValid.Load() != 1 {
		t.Errorf("request_valid should increment; got %d", f.rc.stats.requestValid.Load())
	}
}

func TestDecodeHeaders_OriginAbsent_RefererFallback(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Referer", "http://127.0.0.1:8080/page")
	status := f.DecodeHeaders(h, true)
	if status != envoyhttp.Continue {
		t.Fatalf("got %v want Continue", status)
	}
}

func TestDecodeHeaders_OriginAbsent_RefererAbsent_MissingSourceOrigin(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	status := f.DecodeHeaders(h, true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration", status)
	}
	if f.rc.stats.missingSourceOrigin.Load() != 1 {
		t.Errorf("missing_source_origin should increment")
	}
}

func TestDecodeHeaders_OriginUnparseable_VerbatimUsed(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "not-a-url")
	h.Set("Referer", "http://127.0.0.1:8080/page") // would otherwise allow
	status := f.DecodeHeaders(h, true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration (verbatim mismatch — no Referer fallback)", status)
	}
	if f.rc.stats.requestInvalid.Load() != 1 {
		t.Errorf("request_invalid should increment; got rv=%d ri=%d mso=%d",
			f.rc.stats.requestValid.Load(), f.rc.stats.requestInvalid.Load(), f.rc.stats.missingSourceOrigin.Load())
	}
}

// Group 5 — host:port-only equality.

func TestDecodeHeaders_SameOrigin_HostPortMatch(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "http://127.0.0.1:8080")
	if status := f.DecodeHeaders(h, true); status != envoyhttp.Continue {
		t.Fatalf("got %v want Continue", status)
	}
	if f.rc.stats.requestValid.Load() != 1 {
		t.Errorf("request_valid should increment")
	}
}

func TestDecodeHeaders_CrossOrigin_HostMismatch(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "https://evil.test")
	if status := f.DecodeHeaders(h, true); status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration", status)
	}
	if f.rc.stats.requestInvalid.Load() != 1 {
		t.Errorf("request_invalid should increment")
	}
}

func TestDecodeHeaders_AdditionalOriginsExactMatch(t *testing.T) {
	factory := mustNewListenerFactory(t, []string{"app.example.test"})
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "https://app.example.test") // scheme stripped → app.example.test → matches additional_origins
	if status := f.DecodeHeaders(h, true); status != envoyhttp.Continue {
		t.Fatalf("got %v want Continue", status)
	}
}

func TestDecodeHeaders_NoCaseFolding_UppercaseRejected(t *testing.T) {
	factory := mustNewListenerFactory(t, []string{"app.example.test"})
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "HTTPS://APP.EXAMPLE.TEST")
	if status := f.DecodeHeaders(h, true); status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration (case preserved per §11.7 A2/A3)", status)
	}
}

func TestDecodeHeaders_NoDefaultPortStripping_PortMismatch(t *testing.T) {
	factory := mustNewListenerFactory(t, []string{"app.example.test"})
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "https://app.example.test:443") // hostAndPort = app.example.test:443 ≠ app.example.test
	if status := f.DecodeHeaders(h, true); status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration (default port preserved per §11.7 A4)", status)
	}
}

func TestDecodeHeaders_TrailingSlashStripped_Allow(t *testing.T) {
	factory := mustNewListenerFactory(t, []string{"app.example.test"})
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "https://app.example.test/") // path "/" dropped via URL parser → app.example.test
	if status := f.DecodeHeaders(h, true); status != envoyhttp.Continue {
		t.Fatalf("got %v want Continue (trailing slash stripped per §11.7 A7)", status)
	}
}

func TestDecodeHeaders_OperatorFootgun_FullURLEntry_NeverMatches(t *testing.T) {
	// Per §11.8 amendment: additional_origins entry "https://app.example.test"
	// (full URL with scheme) NEVER matches a real Origin header — operator
	// footgun documented at BEHAVIOR_CONTRACT §13.4.
	factory := mustNewListenerFactory(t, []string{"https://app.example.test"})
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "https://app.example.test")
	if status := f.DecodeHeaders(h, true); status != envoyhttp.StopIteration {
		t.Fatalf("operator footgun: full-URL entry never matches; got %v want StopIteration", status)
	}
}

// Test helpers (lives at the end of csrf_test.go).

func newPostHeaders(host string) http.Header {
	h := http.Header{}
	h.Set(":method", "POST")
	h.Set("Host", host)
	return h
}

func mustNewListenerFactory(t *testing.T, additional []string) envoyhttp.FilterInstanceFactory {
	t.Helper()
	matchers := make([]*matcherv3.StringMatcher, 0, len(additional))
	for _, a := range additional {
		matchers = append(matchers, &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: a}})
	}
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
		AdditionalOrigins: matchers,
	}
	factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return factory
}

func freshFilter(t *testing.T, factory envoyhttp.FilterInstanceFactory) (*filter, *fakeCallbacks) {
	t.Helper()
	hf := factory()
	f := hf.Decoder.(*filter)
	cb := &fakeCallbacks{}
	f.SetDecoderCallbacks(cb)
	return f, cb
}

type localReplyArgs struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

type fakeCallbacks struct {
	localReply *localReplyArgs
	perRoute   proto.Message
}

func (c *fakeCallbacks) ContinueDecoding() {}
func (c *fakeCallbacks) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.localReply = &localReplyArgs{status: status, body: body, headers: headers}
}
func (c *fakeCallbacks) RequestRouteConfig() proto.Message { return c.perRoute }
func (c *fakeCallbacks) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *fakeCallbacks) EncodeHeaders(_ http.Header, _ bool) {}
func (c *fakeCallbacks) EncodeData(_ []byte, _ bool)         {}
func (c *fakeCallbacks) EncodeTrailers(_ http.Header)        {}
