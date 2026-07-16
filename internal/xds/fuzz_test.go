package xds

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// mustValidSecretAnyBytes builds a real, valid tls.v3.Secret{Name:"server_cert",
// tls_certificate{inline PEM}} Any and returns its marshaled .Value bytes, for
// use as a well-formed seed in the fuzz corpus.
func mustValidSecretAnyBytes(f *testing.F) []byte {
	f.Helper()
	certPEM, keyPEM := selfSignedPEM(f)
	sec := &tlsv3.Secret{
		Name: "server_cert",
		Type: &tlsv3.Secret_TlsCertificate{TlsCertificate: &tlsv3.TlsCertificate{
			CertificateChain: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: certPEM}},
			PrivateKey:       &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: keyPEM}},
		}},
	}
	a, err := anypb.New(sec)
	if err != nil {
		f.Fatalf("anypb.New: %v", err)
	}
	return a.GetValue()
}

// mustValidValidationSecretAnyBytes builds a real, valid
// tls.v3.Secret{Name:"validation_ca", validation_context{trusted_ca: inline CA
// PEM}} Any and returns its marshaled .Value bytes — the phase-65 seed for the
// validation_context (SDS trust-anchor) parse arm.
func mustValidValidationSecretAnyBytes(f *testing.F) []byte {
	f.Helper()
	caPEM, _ := selfSignedPEM(f)
	sec := &tlsv3.Secret{
		Name: "validation_ca",
		Type: &tlsv3.Secret_ValidationContext{ValidationContext: &tlsv3.CertificateValidationContext{
			TrustedCa: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: caPEM}},
		}},
	}
	a, err := anypb.New(sec)
	if err != nil {
		f.Fatalf("anypb.New: %v", err)
	}
	return a.GetValue()
}

// FuzzDiscoveryResponseParse fuzzes the untrusted DiscoveryResponse -> Secret
// parse paths (applyResponse -> parseSecret, and applyValidationResponse ->
// parseValidationSecret). resourceBytes is fed as the raw bytes of an Any whose
// type_url is the Secret type URL, exercising arbitrary (including malformed and
// non-proto) wire content against both parsers. The only invariant asserted is:
// no panic.
func FuzzDiscoveryResponseParse(f *testing.F) {
	// A valid single-Secret response (bytes) + malformed variants seed the corpus.
	f.Add([]byte{}, "server_cert")                   // empty
	f.Add([]byte("garbage"), "server_cert")          // non-proto
	f.Add(mustValidSecretAnyBytes(f), "server_cert") // a real tls_certificate Secret Any
	// Phase 65: a real validation_context Secret Any (SDS trust anchor).
	f.Add(mustValidValidationSecretAnyBytes(f), "validation_ca")
	f.Fuzz(func(t *testing.T, resourceBytes []byte, wantName string) {
		// Wrap arbitrary bytes as an Any of the Secret type_url and run both
		// applier paths (applyResponse -> parseSecret, applyValidationResponse
		// -> parseValidationSecret). Must never panic.
		resp := &discoveryv3.DiscoveryResponse{
			TypeUrl:   secretTypeURL(),
			Resources: []*anypb.Any{{TypeUrl: secretTypeURL(), Value: resourceBytes}},
		}
		_, _ = applyResponse(resp, wantName, "")           // ignore the (expected) errors; assert no panic
		_, _ = applyValidationResponse(resp, wantName, "") // the phase-65 validation_context arm
		_, _ = proto.Marshal(resp)                         // exercise the round-trip too
	})
}
