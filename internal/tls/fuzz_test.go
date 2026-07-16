package tls

import (
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

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
	// nil) — a NIL provider — so this seed lands on commonTLSContextToConfig's
	// nil-provider reject (config.go:272), which fires from config.go:52, i.e.
	// BEFORE the require_client_certificate block at config.go:67. It therefore
	// does NOT reach that block's own nil guard (config.go:77, documented there
	// as unreachable) nor the xds.ParseSDSConfig wrap at config.go:90 — those
	// need a live provider and are covered by the unit tests, not by this seed.
	// What this seed pins is the "tls: "-prefix invariant across the reject.
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
