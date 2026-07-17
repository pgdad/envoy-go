package tls

import (
	"crypto/x509"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// cvcFuzzProvider is a live xds.SecretProvider consulted only by the
// "downstream-sds" dispatch side below. Several phase-66 CVC branches — E1, E2,
// the four re-pointed default_validation_context sub-field rejects (the
// inlineVC selector's shared four-reject block in commonTLSContextToConfig,
// which falls back to the CVC's default_validation_context past E1/E2), and
// the well-formed-accept path — are gated on provider != nil inside
// commonTLSContextToConfig's CombinedValidationContext arm and
// NewDownstreamConfig's require_client_certificate block's CVC arm. The
// existing "downstream" side always dispatches with a NIL provider (see seed
// (e)'s note below), so EVERY combined_validation_context-shaped input sent
// through it lands on the earlier retained "combined_validation_context is not
// supported in phase 03" gate and never reaches those branches — verified by
// execution (see the phase-66 task-5 seed-run notes). cvcFuzzProvider always
// serves pki's CA pool so a well-formed CVC's FetchInitialValidationContext call
// SUCCEEDS; seeds that must fail before that call (E1/E2/E3/the four sub-field
// rejects) never invoke it.
var cvcFuzzProvider = &fakeProvider{pool: func() *x509.CertPool {
	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(pki.caPEM) {
		panic("fuzz_test.go: cvcFuzzProvider: pki.caPEM: no certificates parsed")
	}
	return p
}()}

// FuzzTLSContextParse exercises NewDownstreamConfig and NewUpstreamConfig
// against mutated TransportSocket.typed_config bytes. Seeds:
//
//	(a) well-formed DownstreamTlsContext using the inline test PKI.
//	(b) well-formed UpstreamTlsContext using the inline test PKI + SNI.
//	(c) truncated Any bytes.
//	(d) Any with a wrong type_url (StringValue).
//
// Discipline: no panic on any input. Every returned error must begin with
// "tls: ". Malformed inputs yield tls-prefixed errors; well-formed ones
// succeed.
func FuzzTLSContextParse(f *testing.F) {
	// Seed (a): DownstreamTlsContext with inline PKI
	{
		inner := &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{{
					CertificateChain: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pki.leafCertPEM}},
					PrivateKey:       &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pki.leafKeyPEM}},
				}},
			},
		}
		anyTC, _ := anypb.New(inner)
		// anyTC carries both type_url and value; for fuzz we feed both separately.
		f.Add("downstream", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	// Seed (b): UpstreamTlsContext
	{
		inner := &tlsv3.UpstreamTlsContext{
			Sni: "alpha.envoy-go.test",
			CommonTlsContext: &tlsv3.CommonTlsContext{
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
					ValidationContext: &tlsv3.CertificateValidationContext{
						TrustedCa: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pki.caPEM}},
					},
				},
			},
		}
		anyTC, _ := anypb.New(inner)
		f.Add("upstream", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	// Seed (c): truncated — use a non-empty context so proto.Marshal returns
	// at least one byte and the half+1 slice is valid.
	{
		inner := &tlsv3.DownstreamTlsContext{
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{{
					CertificateChain: &corev3.DataSource{
						Specifier: &corev3.DataSource_InlineString{InlineString: "x"},
					},
				}},
			},
		}
		b, _ := proto.Marshal(inner)
		// b is guaranteed non-empty because the context has a non-zero field.
		f.Add("downstream", downstreamTLSContextTypeURL, b[:len(b)/2+1])
	}

	// Seed (d): wrong type_url
	{
		f.Add("downstream", "type.googleapis.com/google.protobuf.StringValue", []byte{0x0a, 0x03, 'x', 'y', 'z'})
	}

	// Seed (e), phase 65: a downstream require_client_certificate=true + an SDS
	// validation_context. The fuzz body dispatches NewDownstreamConfig(ts, "",
	// nil) — a NIL provider — so this seed lands on
	// commonTLSContextToConfig's ValidationContextSdsSecretConfig arm's
	// retained nil-provider reject ("SDS-bound validation_context_sds_secret_config
	// is not supported in phase 03"), which fires from NewDownstreamConfig's call
	// into commonTLSContextToConfig, i.e. BEFORE the require_client_certificate
	// block. It therefore does NOT reach that block's own SDS-VC nil guard
	// (documented there as UNREACHABLE defense-in-depth) nor the
	// xds.ParseSDSConfig wrap inside that same arm — those need a live provider
	// and are covered by the unit tests, not by this seed. What this seed pins
	// is the "tls: "-prefix invariant across the reject.
	{
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext: &tlsv3.CommonTlsContext{
				TlsCertificates: []*tlsv3.TlsCertificate{{
					CertificateChain: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pki.leafCertPEM}},
					PrivateKey:       &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pki.leafKeyPEM}},
				}},
				ValidationContextType: &tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
					ValidationContextSdsSecretConfig: &tlsv3.SdsSecretConfig{Name: "validation_ca"},
				},
			},
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	// Seed (f), phase 66 task 5: a well-formed combined_validation_context
	// (default_validation_context.trusted_ca + a valid
	// validation_context_sds_secret_config) plus require_client_certificate:
	// true, dispatched via "downstream-sds" (cvcFuzzProvider — a LIVE provider).
	// commonTLSContextToConfig's CombinedValidationContext arm NO-OPs past its E1
	// (default_validation_context is required) and E2
	// (validation_context_sds_secret_config is required) checks; the four
	// sub-field rejects don't fire (default_validation_context carries only
	// trusted_ca); NewDownstreamConfig's require_client_certificate block's CVC
	// arm then fetches cvcFuzzProvider's pool and installs it as
	// ClientCAs. NewDownstreamConfig returns a NIL error — the phase-66
	// apply-point's happy path. Verified by execution: this shape (built with the
	// same cvcCTC() helper) is what TestCVC_RequireTrue_InstallsSDSPoolAsClientCAs
	// asserts on directly, and the seed-run log (task-5-report.md) shows
	// err=<nil> for this entry.
	{
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext:         cvcCTC(),
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream-sds", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	// Seed (g), phase 66 task 5: E1's shape — a combined_validation_context with
	// no default_validation_context, dispatched via "downstream-sds" so the CVC
	// arm's provider != nil gate is open and the E1 check in
	// commonTLSContextToConfig's CombinedValidationContext arm
	// (`cvc.GetDefaultValidationContext() == nil`) actually fires:
	// "combined_validation_context.default_validation_context is required".
	// Mirrors TestCVC_MissingDefaultValidationContext_E1's construction. The
	// error propagates UNWRAPPED from NewDownstreamConfig (it returns whatever
	// commonTLSContextToConfig returns) before the require_client_certificate
	// block is ever reached.
	{
		ctc := cvcCTC(func(c *tlsv3.CommonTlsContext_CombinedCertificateValidationContext) {
			c.DefaultValidationContext = nil
		})
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext:         ctc,
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream-sds", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	// Seed (h), phase 66 task 5: E2's shape — a combined_validation_context with
	// a default_validation_context but no validation_context_sds_secret_config,
	// dispatched via "downstream-sds" so the E2 check in
	// commonTLSContextToConfig's CombinedValidationContext arm
	// (`cvc.GetValidationContextSdsSecretConfig() == nil`) actually fires:
	// "combined_validation_context.validation_context_sds_secret_config is
	// required". Mirrors TestCVC_MissingSDSSecretConfig_E2's construction.
	{
		ctc := cvcCTC(func(c *tlsv3.CommonTlsContext_CombinedCertificateValidationContext) {
			c.ValidationContextSdsSecretConfig = nil
		})
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext:         ctc,
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream-sds", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	// Seed (i), phase 66 task 5: E3's shape — a well-formed
	// combined_validation_context with require_client_certificate: false,
	// dispatched via "downstream-sds". commonTLSContextToConfig SUCCEEDS (the CVC
	// arm is a well-formed NO-OP), so NewDownstreamConfig reaches its own C1/D1
	// guard (the isCVC + !GetRequireClientCertificate().GetValue() check that
	// precedes the require_client_certificate block): "combined_validation_context
	// requires require_client_certificate: true in phase 03". Mirrors the
	// "require_client_certificate: false" case of TestCVC_RequireFalse_Rejected_E3.
	{
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: false},
			CommonTlsContext:         cvcCTC(),
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream-sds", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	// Seeds (j)-(m), phase 66 task 5: a well-formed combined_validation_context
	// (both halves present, so E1/E2 don't fire) whose default_validation_context
	// additionally carries one of the four rejected sub-fields, dispatched via
	// "downstream-sds" with require_client_certificate: true. Past E1/E2,
	// commonTLSContextToConfig's inlineVC selector falls back to
	// cvc.GetDefaultValidationContext() (GetValidationContext() is nil under
	// the CVC oneof) and the shared four-reject block fires — mirroring
	// TestCVC_DefaultVC_CustomValidatorConfig_Rejected,
	// TestCVC_DefaultVC_MatchTypedSAN_Rejected, TestCVC_DefaultVC_VerifyCertHash_Rejected,
	// and TestCVC_DefaultVC_VerifyCertSpki_Rejected respectively.
	{
		ctc := cvcWithDefaultVC(func(vc *tlsv3.CertificateValidationContext) {
			vc.CustomValidatorConfig = &corev3.TypedExtensionConfig{Name: "x"}
		})
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext:         ctc,
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream-sds", anyTC.GetTypeUrl(), anyTC.GetValue())
	}
	{
		ctc := cvcWithDefaultVC(func(vc *tlsv3.CertificateValidationContext) {
			vc.MatchTypedSubjectAltNames = []*tlsv3.SubjectAltNameMatcher{{}}
		})
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext:         ctc,
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream-sds", anyTC.GetTypeUrl(), anyTC.GetValue())
	}
	{
		ctc := cvcWithDefaultVC(func(vc *tlsv3.CertificateValidationContext) {
			vc.VerifyCertificateHash = []string{"x"}
		})
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext:         ctc,
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream-sds", anyTC.GetTypeUrl(), anyTC.GetValue())
	}
	{
		ctc := cvcWithDefaultVC(func(vc *tlsv3.CertificateValidationContext) {
			vc.VerifyCertificateSpki = []string{"x"}
		})
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext:         ctc,
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream-sds", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	// Seed (n), phase 66 task 5: a pure-inline CVC shape — a
	// combined_validation_context whose default_validation_context carries a
	// trusted_ca and whose validation_context_sds_secret_config is ABSENT (the
	// E2 shape) — but dispatched via the EXISTING "downstream" side (a NIL
	// provider), not "downstream-sds". Unlike seed (h) above, this lands on the
	// CombinedValidationContext arm's retained "combined_validation_context is
	// not supported in phase 03" gate BEFORE the E2 check is ever reached — the
	// same property TestCVC_E1E2_DoNotPreemptTheRetainedReject pins for the
	// E2-shaped case. This is the realistic nil-provider entry point (QUIC,
	// validate.Bootstrap, and main.go when boot.NewSDSProvider returns (nil,
	// nil)); it pins that such callers never see a leaked E1/E2 message, only the
	// byte-identical retained one — and, unlike seed (e) (which lands on the
	// SEPARATE ValidationContextSdsSecretConfig arm's nil-provider gate), this
	// seed is the first to cover the CVC-specific retained gate.
	{
		ctc := cvcCTC(func(c *tlsv3.CommonTlsContext_CombinedCertificateValidationContext) {
			c.ValidationContextSdsSecretConfig = nil
		})
		inner := &tlsv3.DownstreamTlsContext{
			RequireClientCertificate: &wrapperspb.BoolValue{Value: true},
			CommonTlsContext:         ctc,
		}
		anyTC, err := anypb.New(inner)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add("downstream", anyTC.GetTypeUrl(), anyTC.GetValue())
	}

	f.Fuzz(func(t *testing.T, side, typeURL string, value []byte) {
		ts := &corev3.TransportSocket{
			ConfigType: &corev3.TransportSocket_TypedConfig{
				TypedConfig: &anypb.Any{TypeUrl: typeURL, Value: value},
			},
		}
		var err error
		switch side {
		case "downstream":
			_, err = NewDownstreamConfig(ts, "", nil)
		case "downstream-sds":
			// Phase 66 task 5: dispatches through cvcFuzzProvider (a live
			// provider) so CVC seeds reach E1/E2/E3/the four sub-field
			// rejects/the well-formed accept path instead of the earlier
			// nil-provider retained gate. See cvcFuzzProvider's doc comment.
			_, err = NewDownstreamConfig(ts, "", cvcFuzzProvider)
		case "upstream":
			_, err = NewUpstreamConfig(ts, "")
		default:
			return
		}
		if err != nil && !strings.HasPrefix(err.Error(), "tls: ") {
			t.Errorf("error does not begin with \"tls: \": %v", err)
		}
	})
}
