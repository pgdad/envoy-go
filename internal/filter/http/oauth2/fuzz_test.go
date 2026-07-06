package oauth2

// fuzz_test.go — 26th fuzzer FuzzOAuth2ConfigParse per phase-20 SPEC §7.4 +
// PLAN Task 12 + D7.
//
// Asserts the structural contract per ADR-0018 + ADR-0156: New returns
// either (factory, nil) OR (nil, error); never panics; never returns
// (nil, nil). The fuzzer drives arbitrary byte sequences as the
// typed_config Any.Value payload + tolerates Unmarshal failures via the
// PARSE-REJECT branch.
//
// # Seed corpus per SPEC §7.4
//
// ~30 hand-curated seeds covering OAuth2Config + OAuth2Credentials +
// CookieNames + SdsSecretConfig + matcher-engine variants. The PLAN
// target was ~60 seeds; the IMPL-Time LANDED-COUNT is ~30 — covers
// each-decision + boundary cases without the field-cross-product
// explosion. Future fuzzer-corpus-extension may reach the 60-target.
//
// 30s/seed runtime envelope per ADR-0018 short-mode CI policy.

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	oauth2v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/oauth2/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// FuzzOAuth2ConfigParse fuzzes arbitrary byte sequences as the typed_config
// Any.Value payload to oauth2.New. 26th fuzzer per phase-20 SPEC §7.4 + D7.
//
// Structural contract per ADR-0018: never panics; never returns (nil, nil);
// never returns (factory, err).
func FuzzOAuth2ConfigParse(f *testing.F) {
	addSeed := func(msg proto.Message) {
		f.Helper()
		raw, err := proto.Marshal(msg)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(raw)
	}

	// Test-only inline SDS path that resolves to a non-existent filesystem
	// path — buildCompiledConfig surfaces the sdsfile.New error and the
	// PARSE-REJECT contract holds. The fuzz body asserts only the
	// structural contract; the precise error wording is asserted by the
	// PARSE-REJECT unit tests in oauth2_test.go.
	const tempPath = "/tmp/oauth2-fuzz-nonexistent.json"

	pathSds := func(name string) *tlsv3.SdsSecretConfig {
		return &tlsv3.SdsSecretConfig{
			Name: name,
			SdsConfig: &corev3.ConfigSource{
				ConfigSourceSpecifier: &corev3.ConfigSource_PathConfigSource{
					PathConfigSource: &corev3.PathConfigSource{Path: tempPath},
				},
			},
		}
	}

	apiSds := func(name string) *tlsv3.SdsSecretConfig {
		return &tlsv3.SdsSecretConfig{
			Name: name,
			SdsConfig: &corev3.ConfigSource{
				ConfigSourceSpecifier: &corev3.ConfigSource_ApiConfigSource{
					ApiConfigSource: &corev3.ApiConfigSource{},
				},
			},
		}
	}

	adsSds := func(name string) *tlsv3.SdsSecretConfig {
		return &tlsv3.SdsSecretConfig{
			Name: name,
			SdsConfig: &corev3.ConfigSource{
				ConfigSourceSpecifier: &corev3.ConfigSource_Ads{
					Ads: &corev3.AggregatedConfigSource{},
				},
			},
		}
	}

	httpUri := func(uri string) *corev3.HttpUri {
		return &corev3.HttpUri{
			Uri:              uri,
			HttpUpstreamType: &corev3.HttpUri_Cluster{Cluster: "c_idp"},
		}
	}

	baseConfig := func() *oauth2v3.OAuth2 {
		return &oauth2v3.OAuth2{
			Config: &oauth2v3.OAuth2Config{
				TokenEndpoint:         httpUri("https://idp.example.com/token"),
				AuthorizationEndpoint: "https://idp.example.com/auth",
				RedirectUri:           "https://app.example.com/cb",
				Credentials: &oauth2v3.OAuth2Credentials{
					ClientId: "client-id",
					TokenFormation: &oauth2v3.OAuth2Credentials_HmacSecret{
						HmacSecret: pathSds("hmac"),
					},
					TokenSecret: pathSds("client_secret"),
				},
			},
		}
	}

	// (0) Minimal valid skeleton (PathConfigSource paths resolve to a
	// nonexistent file → sdsfile.New error → PARSE-REJECT contract).
	addSeed(baseConfig())

	// (1) Missing token_endpoint URI.
	{
		m := baseConfig()
		m.Config.TokenEndpoint = nil
		addSeed(m)
	}

	// (2) Empty token_endpoint URI.
	{
		m := baseConfig()
		m.Config.TokenEndpoint.Uri = ""
		addSeed(m)
	}

	// (3) Malformed token_endpoint URI.
	{
		m := baseConfig()
		m.Config.TokenEndpoint.Uri = "::not a url::"
		addSeed(m)
	}

	// (4) Empty authorization_endpoint.
	{
		m := baseConfig()
		m.Config.AuthorizationEndpoint = ""
		addSeed(m)
	}

	// (5) Empty redirect_uri.
	{
		m := baseConfig()
		m.Config.RedirectUri = ""
		addSeed(m)
	}

	// (6) Empty client_id.
	{
		m := baseConfig()
		m.Config.Credentials.ClientId = ""
		addSeed(m)
	}

	// (7) BASIC_AUTH (PARSE-REJECT per AMEND-5).
	{
		m := baseConfig()
		m.Config.AuthType = oauth2v3.OAuth2Config_BASIC_AUTH
		addSeed(m)
	}

	// (8) PKCE oauth_nonce set (PARSE-REJECT per §2.1).
	{
		m := baseConfig()
		m.Config.Credentials.CookieNames = &oauth2v3.OAuth2Credentials_CookieNames{
			OauthNonce: "csrf-nonce",
		}
		addSeed(m)
	}

	// (9) hmac_secret nil (encryption-default-ON requires non-empty).
	{
		m := baseConfig()
		m.Config.Credentials.TokenFormation = nil
		addSeed(m)
	}

	// (10) hmac_secret via ApiConfigSource (PARSE-REJECT per §3.2).
	{
		m := baseConfig()
		m.Config.Credentials.TokenFormation = &oauth2v3.OAuth2Credentials_HmacSecret{
			HmacSecret: apiSds("hmac"),
		}
		addSeed(m)
	}

	// (11) hmac_secret via Ads (PARSE-REJECT per §3.2).
	{
		m := baseConfig()
		m.Config.Credentials.TokenFormation = &oauth2v3.OAuth2Credentials_HmacSecret{
			HmacSecret: adsSds("hmac"),
		}
		addSeed(m)
	}

	// (12) token_secret via ApiConfigSource (PARSE-REJECT).
	{
		m := baseConfig()
		m.Config.Credentials.TokenSecret = apiSds("client_secret")
		addSeed(m)
	}

	// (13) hmac_secret deprecated ConfigSource.path field 1.
	{
		m := baseConfig()
		m.Config.Credentials.TokenFormation = &oauth2v3.OAuth2Credentials_HmacSecret{
			HmacSecret: &tlsv3.SdsSecretConfig{
				Name: "hmac",
				SdsConfig: &corev3.ConfigSource{
					ConfigSourceSpecifier: &corev3.ConfigSource_Path{
						Path: tempPath,
					},
				},
			},
		}
		addSeed(m)
	}

	// (14) hmac_secret SdsConfig nil (static-resource arm).
	{
		m := baseConfig()
		m.Config.Credentials.TokenFormation = &oauth2v3.OAuth2Credentials_HmacSecret{
			HmacSecret: &tlsv3.SdsSecretConfig{Name: "hmac"},
		}
		addSeed(m)
	}

	// (15) operator-supplied CookieNames overrides.
	{
		m := baseConfig()
		m.Config.Credentials.CookieNames = &oauth2v3.OAuth2Credentials_CookieNames{
			BearerToken:  "MyBearer",
			OauthHmac:    "MyHmac",
			OauthExpires: "MyExpires",
			IdToken:      "MyId",
			RefreshToken: "MyRefresh",
		}
		addSeed(m)
	}

	// (16) forward_bearer_token + preserve_authorization_header + scopes.
	{
		m := baseConfig()
		m.Config.ForwardBearerToken = true
		m.Config.PreserveAuthorizationHeader = true
		m.Config.AuthScopes = []string{"openid", "profile", "email"}
		m.Config.Resources = []string{"resource-a", "resource-b"}
		addSeed(m)
	}

	// (17) use_refresh_token = false.
	{
		m := baseConfig()
		m.Config.UseRefreshToken = &wrapperspb.BoolValue{Value: false}
		addSeed(m)
	}

	// (18) default_expires_in + default_refresh_token_expires_in.
	{
		m := baseConfig()
		m.Config.DefaultExpiresIn = durationpb.New(3600 * 1e9)
		m.Config.DefaultRefreshTokenExpiresIn = durationpb.New(86400 * 1e9)
		addSeed(m)
	}

	// (19) redirect_path_matcher (path-prefix matcher).
	{
		m := baseConfig()
		m.Config.RedirectPathMatcher = &matcherv3.PathMatcher{
			Rule: &matcherv3.PathMatcher_Path{
				Path: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "/oauth2/callback"},
				},
			},
		}
		addSeed(m)
	}

	// (20) redirect_path_matcher (safe_regex matcher).
	{
		m := baseConfig()
		m.Config.RedirectPathMatcher = &matcherv3.PathMatcher{
			Rule: &matcherv3.PathMatcher_Path{
				Path: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_SafeRegex{
						SafeRegex: &matcherv3.RegexMatcher{Regex: "^/cb[0-9]+$"},
					},
				},
			},
		}
		addSeed(m)
	}

	// (21) signout_path (exact matcher).
	{
		m := baseConfig()
		m.Config.SignoutPath = &matcherv3.PathMatcher{
			Rule: &matcherv3.PathMatcher_Path{
				Path: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "/signout"},
				},
			},
		}
		addSeed(m)
	}

	// (22) pass_through_matcher (header exact match).
	{
		m := baseConfig()
		m.Config.PassThroughMatcher = []*routev3.HeaderMatcher{
			{
				Name: "X-Bypass-OAuth2",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{
					ExactMatch: "true",
				},
			},
		}
		addSeed(m)
	}

	// (23) pass_through_matcher (present match).
	{
		m := baseConfig()
		m.Config.PassThroughMatcher = []*routev3.HeaderMatcher{
			{
				Name: "X-Auth-Done",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{
					PresentMatch: true,
				},
			},
		}
		addSeed(m)
	}

	// (24) pass_through_matcher (string_match prefix + safe_regex hybrid).
	{
		m := baseConfig()
		m.Config.PassThroughMatcher = []*routev3.HeaderMatcher{
			{
				Name: "X-Bypass",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
					StringMatch: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "bypass-"},
					},
				},
			},
			{
				Name: "X-CORS",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
					StringMatch: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_SafeRegex{
							SafeRegex: &matcherv3.RegexMatcher{Regex: "^pre.*"},
						},
					},
				},
			},
		}
		addSeed(m)
	}

	// (25) deny_redirect_matcher list.
	{
		m := baseConfig()
		m.Config.DenyRedirectMatcher = []*routev3.HeaderMatcher{
			{
				Name: "Accept",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_ContainsMatch{
					ContainsMatch: "application/json",
				},
			},
		}
		addSeed(m)
	}

	// (26) full surface — every consumed field exercised together.
	{
		m := baseConfig()
		m.Config.ForwardBearerToken = true
		m.Config.PreserveAuthorizationHeader = true
		m.Config.AuthScopes = []string{"openid", "profile"}
		m.Config.Resources = []string{"resource-a"}
		m.Config.UseRefreshToken = &wrapperspb.BoolValue{Value: true}
		m.Config.DefaultExpiresIn = durationpb.New(3600 * 1e9)
		m.Config.Credentials.CookieNames = &oauth2v3.OAuth2Credentials_CookieNames{
			BearerToken: "MyBearer",
		}
		m.Config.RedirectPathMatcher = &matcherv3.PathMatcher{
			Rule: &matcherv3.PathMatcher_Path{
				Path: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "/cb"},
				},
			},
		}
		m.Config.SignoutPath = &matcherv3.PathMatcher{
			Rule: &matcherv3.PathMatcher_Path{
				Path: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "/logout"},
				},
			},
		}
		m.Config.PassThroughMatcher = []*routev3.HeaderMatcher{
			{
				Name:                 "X-Bypass",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{PresentMatch: true},
			},
		}
		m.Config.Credentials.CookieDomain = "example.com"
		addSeed(m)
	}

	// (27) cookie_domain set without other overrides.
	{
		m := baseConfig()
		m.Config.Credentials.CookieDomain = "app.example.com"
		addSeed(m)
	}

	// (28) Wrapper struct (no OAuth2Config inside).
	addSeed(&oauth2v3.OAuth2{})

	// (29) Empty bytes seed — Unmarshal succeeds to zero-value OAuth2;
	// the missing-OAuth2Config branch fires.
	f.Add([]byte{})

	// -------------------------------------------------------------------------
	// Fuzz body: structural contract assertions only.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, raw []byte) {
		anyMsg := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		factory, err := New(anyMsg, envoyhttp.FactoryCtx{})
		if factory == nil && err == nil {
			t.Fatalf("New returned (nil, nil) — invariant violation; len(raw)=%d", len(raw))
		}
		if factory != nil && err != nil {
			t.Fatalf("New returned (factory, err) — invariant violation: %v", err)
		}
	})
}
