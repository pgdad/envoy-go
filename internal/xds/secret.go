package xds

import (
	stdtls "crypto/tls"
	"fmt"
	"os"
	"path/filepath"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// secretTypeURL returns the wire type URL for an SDS Secret resource, derived at
// runtime from the proto descriptor (never hardcoded — reference_network_filter_typeurl_extensions).
func secretTypeURL() string {
	return "type.googleapis.com/" + string(proto.MessageName(&tlsv3.Secret{}))
}

// dataSourceBytes resolves a core.DataSource into raw bytes. It MIRRORS
// internal/tls.loadDataSource's phase-03 grammar (inline_bytes / inline_string /
// filename honored; environment_variable + zero-value error) but is duplicated
// here deliberately: internal/xds must NOT import internal/tls (the 60.2 cycle
// guard — see doc.go / ADR-0278). A non-absolute filename resolves relative to
// baseDir.
func dataSourceBytes(ds *corev3.DataSource, baseDir string) ([]byte, error) {
	switch s := ds.GetSpecifier().(type) {
	case *corev3.DataSource_InlineBytes:
		return s.InlineBytes, nil
	case *corev3.DataSource_InlineString:
		return []byte(s.InlineString), nil
	case *corev3.DataSource_Filename:
		p := s.Filename
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("xds: sds: data source: read %s: %w", p, err)
		}
		return b, nil
	case *corev3.DataSource_EnvironmentVariable:
		return nil, fmt.Errorf("xds: sds: data source: environment_variable is not supported")
	default:
		return nil, fmt.Errorf("xds: sds: data source: none of inline_bytes, inline_string, filename set")
	}
}

// parseSecret validates one DiscoveryResponse resource and builds the served leaf.
// It requires: the Any resolves to a tls.v3.Secret; Secret.name == wantName; the
// secret's oneof is tls_certificate; and the certificate_chain/private_key
// DataSources yield a valid X509 key pair. Returns a classified error (wrapping
// errValidation, Task 4) so the caller can NACK + increment update_rejected.
func parseSecret(resource *anypb.Any, wantName, baseDir string) (*stdtls.Certificate, error) {
	var sec tlsv3.Secret
	if err := resource.UnmarshalTo(&sec); err != nil {
		return nil, fmt.Errorf("xds: sds: resource is not a %s: %w", secretTypeURL(), err)
	}
	if sec.GetName() != wantName {
		return nil, fmt.Errorf("xds: sds: response secret name %q != requested %q", sec.GetName(), wantName)
	}
	tc := sec.GetTlsCertificate()
	if tc == nil {
		return nil, fmt.Errorf("xds: sds: secret %q is not a tls_certificate (unsupported oneof arm)", wantName)
	}
	certPEM, err := dataSourceBytes(tc.GetCertificateChain(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: secret %q: certificate_chain: %w", wantName, err)
	}
	keyPEM, err := dataSourceBytes(tc.GetPrivateKey(), baseDir)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: secret %q: private_key: %w", wantName, err)
	}
	pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("xds: sds: secret %q: load cert: %w", wantName, err)
	}
	return &pair, nil
}
