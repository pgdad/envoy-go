package xds

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// mustValidSecretAnyBytes builds a real, valid tls.v3.Secret{Name:"server_cert",
// tls_certificate{inline PEM}} Any and returns its marshaled .Value bytes, for
// use as a well-formed seed in the fuzz corpus. It generates its own self-signed
// PEM inline (rather than reusing secret_test.go's selfSignedPEM, which takes a
// *testing.T and can't accept a *testing.F).
func mustValidSecretAnyBytes(f *testing.F) []byte {
	f.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		f.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "secret.envoy-go.test"},
		DNSNames:     []string{"secret.envoy-go.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		f.Fatalf("x509.CreateCertificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		f.Fatalf("x509.MarshalPKCS8PrivateKey: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

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

// FuzzDiscoveryResponseParse fuzzes the untrusted DiscoveryResponse -> Secret
// parse path (applyResponse, which calls parseSecret). resourceBytes is fed as
// the raw bytes of an Any whose type_url is the Secret type URL, exercising
// arbitrary (including malformed and non-proto) wire content against the
// parser. The only invariant asserted is: no panic.
func FuzzDiscoveryResponseParse(f *testing.F) {
	// A valid single-Secret response (bytes) + malformed variants seed the corpus.
	f.Add([]byte{}, "server_cert")                   // empty
	f.Add([]byte("garbage"), "server_cert")          // non-proto
	f.Add(mustValidSecretAnyBytes(f), "server_cert") // a real Secret Any (helper)
	f.Fuzz(func(t *testing.T, resourceBytes []byte, wantName string) {
		// Wrap arbitrary bytes as an Any of the Secret type_url and run the
		// applier path (applyResponse -> parseSecret). Must never panic.
		resp := &discoveryv3.DiscoveryResponse{
			TypeUrl:   secretTypeURL(),
			Resources: []*anypb.Any{{TypeUrl: secretTypeURL(), Value: resourceBytes}},
		}
		_, _ = applyResponse(resp, wantName, "") // ignore the (expected) errors; assert no panic
		_, _ = proto.Marshal(resp)               // exercise the round-trip too
	})
}
