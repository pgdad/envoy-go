package sdsserver

import (
	"bytes"
	"context"
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
	secretv3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// secretTypeURL is the SDS resource type_url for a tls/v3 Secret.
const secretTypeURL = "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret"

// genSelfSignedCert generates a fresh self-signed ECDSA cert/key PEM pair for
// self-test use only (mirrors the internal/tls/config_test.go pki idiom).
func genSelfSignedCert(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sdsserver test cert"},
		DNSNames:     []string{"sdsserver.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("x509.MarshalPKCS8PrivateKey: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// dialTestClient opens a plaintext h2c grpc.ClientConn to the supplied
// address, registers it for teardown via t.Cleanup, and returns the
// SecretDiscoveryServiceClient.
func dialTestClient(t *testing.T, addr string) secretv3.SecretDiscoveryServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q): %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return secretv3.NewSecretDiscoveryServiceClient(conn)
}

// TestStreamSecrets_DeliversConfiguredSecret drives the server the way a real
// SDS client (the internal/xds provider) does: dial, open StreamSecrets, send
// an initial DiscoveryRequest naming the configured secret, and Recv the
// DiscoveryResponse. Asserts the delivered Secret{Name} + tls_certificate AND
// that Requests() recorded the request's node/resource_names — proving the
// exchange is non-vacuous (reference_docker_probe_bridge_network discipline).
func TestStreamSecrets_DeliversConfiguredSecret(t *testing.T) {
	certPEM, keyPEM := genSelfSignedCert(t)
	srv := New(t, WithSecret("server_cert", certPEM, keyPEM))
	client := dialTestClient(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamSecrets(ctx)
	if err != nil {
		t.Fatalf("StreamSecrets: %v", err)
	}
	req := &discoveryv3.DiscoveryRequest{
		ResourceNames: []string{"server_cert"},
		TypeUrl:       secretTypeURL,
		Node: &corev3.Node{
			Id:      "n",
			Cluster: "c",
		},
	}
	if err := stream.Send(req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if len(resp.GetResources()) != 1 {
		t.Fatalf("Resources: got %d, want 1", len(resp.GetResources()))
	}
	var sec tlsv3.Secret
	if err := resp.GetResources()[0].UnmarshalTo(&sec); err != nil {
		t.Fatalf("UnmarshalTo(Secret): %v", err)
	}
	if sec.GetName() != "server_cert" {
		t.Errorf("Secret.Name: got %q, want %q", sec.GetName(), "server_cert")
	}
	tlsCert := sec.GetTlsCertificate()
	if tlsCert == nil {
		t.Fatal("Secret.TlsCertificate: nil")
	}
	if got := tlsCert.GetCertificateChain().GetInlineBytes(); string(got) != string(certPEM) {
		t.Errorf("CertificateChain.InlineBytes: got %q, want %q", got, certPEM)
	}
	if got := tlsCert.GetPrivateKey().GetInlineBytes(); string(got) != string(keyPEM) {
		t.Errorf("PrivateKey.InlineBytes: got %q, want %q", got, keyPEM)
	}

	// Non-vacuous exchange proof: the server actually received + recorded the
	// request (not just replayed a canned response without decoding).
	reqs := srv.Requests()
	if len(reqs) < 1 {
		t.Fatalf("Requests: got %d, want >= 1", len(reqs))
	}
	got := reqs[0]
	if got.GetNode().GetId() != "n" {
		t.Errorf("Requests()[0].Node.Id: got %q, want %q", got.GetNode().GetId(), "n")
	}
	if got.GetNode().GetCluster() != "c" {
		t.Errorf("Requests()[0].Node.Cluster: got %q, want %q", got.GetNode().GetCluster(), "c")
	}
	if want := []string{"server_cert"}; len(got.GetResourceNames()) != 1 || got.GetResourceNames()[0] != want[0] {
		t.Errorf("Requests()[0].ResourceNames: got %v, want %v", got.GetResourceNames(), want)
	}
}

// fetchOne drives exactly one StreamSecrets exchange against a running server:
// dial, open the stream, Send a single DiscoveryRequest naming `name`, and Recv
// the single DiscoveryResponse. Factored out of the per-arm tests so a
// validation_context fetch and a tls_certificate fetch exercise the SAME client
// path — any divergence is then the SERVER's, not the driver's.
func fetchOne(t *testing.T, addr, name string) *discoveryv3.DiscoveryResponse {
	t.Helper()
	client := dialTestClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamSecrets(ctx)
	if err != nil {
		t.Fatalf("StreamSecrets: %v", err)
	}
	req := &discoveryv3.DiscoveryRequest{
		ResourceNames: []string{name},
		TypeUrl:       secretTypeURL,
		Node:          &corev3.Node{Id: "n", Cluster: "c"},
	}
	if err := stream.Send(req); err != nil {
		t.Fatalf("Send: %v", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	return resp
}

// TestWithValidationContext_ServesValidationSecret: the server delivers a
// Secret{name, validation_context{trusted_ca: inline}} — the phase-65 resource
// shape PINNED by the reference's own config_dump (SPEC-65 §11 D-SDSVC-RESOURCE).
func TestWithValidationContext_ServesValidationSecret(t *testing.T) {
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")
	srv := New(t, WithValidationContext("validation_ca", caPEM))

	resp := fetchOne(t, srv.Addr(), "validation_ca")

	if got := len(resp.GetResources()); got != 1 {
		t.Fatalf("Resources = %d, want 1", got)
	}
	var sec tlsv3.Secret
	if err := resp.GetResources()[0].UnmarshalTo(&sec); err != nil {
		t.Fatalf("UnmarshalTo Secret: %v", err)
	}
	if sec.GetName() != "validation_ca" {
		t.Errorf("Secret.Name = %q, want validation_ca", sec.GetName())
	}
	vc := sec.GetValidationContext()
	if vc == nil {
		t.Fatal("Secret is not a validation_context — wrong oneof arm served")
	}
	if got := vc.GetTrustedCa().GetInlineBytes(); !bytes.Equal(got, caPEM) {
		t.Errorf("trusted_ca inline_bytes = %q, want the configured CA PEM", got)
	}
	// The type URL is SHARED with the tls_certificate arm — same Secret message
	// (D1), so the generic derivation in buildResponse needs no second arm.
	if got, want := resp.GetTypeUrl(), secretTypeURL; got != want {
		t.Errorf("TypeUrl = %q, want %q", got, want)
	}
}

// TestStop_Idempotent verifies calling Stop twice does not panic.
func TestStop_Idempotent(t *testing.T) {
	srv := New(t)
	srv.Stop()
	srv.Stop()
}

// TestNewAtAddr_DeliversConfiguredSecret proves NewAtAddr (the caller-owned-
// lifecycle constructor consumed by the 0103 differential driver) binds the
// supplied address and serves exactly like New(t), and that the caller (not
// t.Cleanup) is responsible for Stop.
func TestNewAtAddr_DeliversConfiguredSecret(t *testing.T) {
	certPEM, keyPEM := genSelfSignedCert(t)
	srv, err := NewAtAddr("127.0.0.1:0", WithSecret("s", certPEM, keyPEM))
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	if srv == nil {
		t.Fatal("NewAtAddr: got nil server, nil error")
	}
	defer srv.Stop()

	client := dialTestClient(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamSecrets(ctx)
	if err != nil {
		t.Fatalf("StreamSecrets: %v", err)
	}
	req := &discoveryv3.DiscoveryRequest{
		ResourceNames: []string{"s"},
		TypeUrl:       secretTypeURL,
		Node:          &corev3.Node{Id: "n", Cluster: "c"},
	}
	if err := stream.Send(req); err != nil {
		t.Fatalf("Send: %v", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if len(resp.GetResources()) != 1 {
		t.Fatalf("Resources: got %d, want 1", len(resp.GetResources()))
	}
	var sec tlsv3.Secret
	if err := resp.GetResources()[0].UnmarshalTo(&sec); err != nil {
		t.Fatalf("UnmarshalTo(Secret): %v", err)
	}
	if sec.GetName() != "s" {
		t.Errorf("Secret.Name: got %q, want %q", sec.GetName(), "s")
	}
}
