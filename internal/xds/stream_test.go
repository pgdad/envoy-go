package xds

import (
	"errors"
	"io"
	"strings"
	"testing"

	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/protobuf/types/known/anypb"
)

// fakeStream is an in-memory Stream: it records every Send'd
// *DiscoveryRequest and returns programmed Recv responses (FIFO), falling
// back to recvErr (default io.EOF) once the programmed responses are
// exhausted.
type fakeStream struct {
	sent    []*discoveryv3.DiscoveryRequest
	resps   []*discoveryv3.DiscoveryResponse
	recvErr error
}

func (f *fakeStream) Send(r *discoveryv3.DiscoveryRequest) error {
	f.sent = append(f.sent, r)
	return nil
}

func (f *fakeStream) Recv() (*discoveryv3.DiscoveryResponse, error) {
	if len(f.resps) == 0 {
		if f.recvErr != nil {
			return nil, f.recvErr
		}
		return nil, io.EOF
	}
	r := f.resps[0]
	f.resps = f.resps[1:]
	return r, nil
}

// validSecret builds a valid tls_certificate Secret named name, wrapped in
// an *anypb.Any, using a fresh self-signed key pair.
func validSecretAny(t *testing.T, name string) *anypb.Any {
	t.Helper()
	certPEM, keyPEM := selfSignedPEM(t)
	sec := &tlsv3.Secret{
		Name: name,
		Type: &tlsv3.Secret_TlsCertificate{TlsCertificate: &tlsv3.TlsCertificate{
			CertificateChain: inlineDS(certPEM),
			PrivateKey:       inlineDS(keyPEM),
		}},
	}
	return anyOf(t, sec)
}

func TestFetchSecret_InitialRequestShape(t *testing.T) {
	fs := &fakeStream{
		resps: []*discoveryv3.DiscoveryResponse{
			{
				VersionInfo: "v1",
				Nonce:       "n1",
				TypeUrl:     secretTypeURL(),
				Resources:   []*anypb.Any{validSecretAny(t, "server_cert")},
			},
		},
	}
	node := Node{ID: "node-1", Cluster: "cluster-1"}
	if _, err := fetchSecret(fs, node, "server_cert", ""); err != nil {
		t.Fatalf("fetchSecret() unexpected error: %v", err)
	}
	if len(fs.sent) < 1 {
		t.Fatalf("sent has %d requests, want at least 1", len(fs.sent))
	}
	req := fs.sent[0]
	if req.GetVersionInfo() != "" {
		t.Errorf("sent[0].VersionInfo = %q, want empty", req.GetVersionInfo())
	}
	if req.GetResponseNonce() != "" {
		t.Errorf("sent[0].ResponseNonce = %q, want empty", req.GetResponseNonce())
	}
	if got, want := req.GetResourceNames(), []string{"server_cert"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("sent[0].ResourceNames = %v, want %v", got, want)
	}
	if req.GetTypeUrl() != secretTypeURL() {
		t.Errorf("sent[0].TypeUrl = %q, want %q", req.GetTypeUrl(), secretTypeURL())
	}
	if req.GetNode().GetId() != "node-1" {
		t.Errorf("sent[0].Node.Id = %q, want %q", req.GetNode().GetId(), "node-1")
	}
	if req.GetNode().GetCluster() != "cluster-1" {
		t.Errorf("sent[0].Node.Cluster = %q, want %q", req.GetNode().GetCluster(), "cluster-1")
	}
	if req.GetErrorDetail() != nil {
		t.Errorf("sent[0].ErrorDetail = %v, want nil", req.GetErrorDetail())
	}
}

func TestFetchSecret_AckOnSuccess(t *testing.T) {
	fs := &fakeStream{
		resps: []*discoveryv3.DiscoveryResponse{
			{
				VersionInfo: "v1",
				Nonce:       "n1",
				TypeUrl:     secretTypeURL(),
				Resources:   []*anypb.Any{validSecretAny(t, "server_cert")},
			},
		},
	}
	node := Node{ID: "node-1", Cluster: "cluster-1"}
	cert, err := fetchSecret(fs, node, "server_cert", "")
	if err != nil {
		t.Fatalf("fetchSecret() unexpected error: %v", err)
	}
	if cert == nil {
		t.Fatalf("fetchSecret() returned nil cert with nil error")
	}
	if len(fs.sent) != 2 {
		t.Fatalf("sent has %d requests, want 2 (initial + ACK)", len(fs.sent))
	}
	ack := fs.sent[1]
	if ack.GetVersionInfo() != "v1" {
		t.Errorf("ACK VersionInfo = %q, want %q", ack.GetVersionInfo(), "v1")
	}
	if ack.GetResponseNonce() != "n1" {
		t.Errorf("ACK ResponseNonce = %q, want %q", ack.GetResponseNonce(), "n1")
	}
	if ack.GetErrorDetail() != nil {
		t.Errorf("ACK ErrorDetail = %v, want nil", ack.GetErrorDetail())
	}
}

func TestFetchSecret_NackOnValidationFailure(t *testing.T) {
	fs := &fakeStream{
		resps: []*discoveryv3.DiscoveryResponse{
			{
				VersionInfo: "v1",
				Nonce:       "n1",
				TypeUrl:     secretTypeURL(),
				// wrong name: requested "server_cert", delivered "other"
				Resources: []*anypb.Any{validSecretAny(t, "other")},
			},
		},
	}
	node := Node{ID: "node-1", Cluster: "cluster-1"}
	_, err := fetchSecret(fs, node, "server_cert", "")
	if err == nil {
		t.Fatal("fetchSecret() expected error, got nil")
	}
	if !errors.Is(err, errValidation) {
		t.Errorf("fetchSecret() error = %v, want it to wrap errValidation", err)
	}
	if len(fs.sent) != 2 {
		t.Fatalf("sent has %d requests, want 2 (initial + NACK)", len(fs.sent))
	}
	nack := fs.sent[1]
	if nack.GetVersionInfo() != "" {
		t.Errorf("NACK VersionInfo = %q, want empty (prior version, unchanged)", nack.GetVersionInfo())
	}
	if nack.GetErrorDetail() == nil {
		t.Fatalf("NACK ErrorDetail = nil, want non-nil")
	}
	if nack.GetErrorDetail().GetMessage() == "" {
		t.Errorf("NACK ErrorDetail.Message is empty, want non-empty")
	}
}

func TestFetchSecret_TransportError(t *testing.T) {
	fs := &fakeStream{} // Recv returns io.EOF immediately, no responses programmed
	node := Node{ID: "node-1", Cluster: "cluster-1"}
	_, err := fetchSecret(fs, node, "server_cert", "")
	if err == nil {
		t.Fatal("fetchSecret() expected error, got nil")
	}
	if errors.Is(err, errValidation) {
		t.Errorf("fetchSecret() error = %v, want a transport-class error, not errValidation", err)
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("fetchSecret() error = %v, want it to wrap io.EOF", err)
	}
}

// validVCSecretAny builds an Any-wrapped Secret{name, validation_context{trusted_ca}}
// for the stream-arm tests — the validation_context sibling of validSecretAny
// (:42). It delegates to vcSecret (secret_test.go:218), which builds the exact
// same shape (inline_bytes trusted_ca) for the applier tests.
func validVCSecretAny(t *testing.T, name string) *anypb.Any {
	t.Helper()
	caPEM, _ := selfSignedPEM(t)
	return vcSecret(t, name, caPEM)
}

func TestFetchValidationSecret_InitialRequestShape(t *testing.T) {
	fs := &fakeStream{resps: []*discoveryv3.DiscoveryResponse{{
		VersionInfo: "v1", Nonce: "n1",
		Resources: []*anypb.Any{validVCSecretAny(t, "validation_ca")},
	}}}
	if _, err := fetchValidationSecret(fs, Node{ID: "id", Cluster: "cl"}, "validation_ca", ""); err != nil {
		t.Fatalf("fetchValidationSecret: %v", err)
	}
	if len(fs.sent) < 1 {
		t.Fatal("no request sent")
	}
	init := fs.sent[0]
	if got := init.GetTypeUrl(); got != secretTypeURL() {
		t.Errorf("initial TypeUrl = %q, want %q (the Secret type URL is SHARED by both oneof arms)", got, secretTypeURL())
	}
	if got := init.GetResourceNames(); len(got) != 1 || got[0] != "validation_ca" {
		t.Errorf("initial ResourceNames = %v, want [validation_ca]", got)
	}
	if init.GetVersionInfo() != "" {
		t.Errorf("initial VersionInfo = %q, want empty", init.GetVersionInfo())
	}
	if init.GetResponseNonce() != "" {
		t.Errorf("initial ResponseNonce = %q, want empty", init.GetResponseNonce())
	}
	if init.GetNode().GetId() != "id" || init.GetNode().GetCluster() != "cl" {
		t.Errorf("initial Node = %v, want {id, cl}", init.GetNode())
	}
}

func TestFetchValidationSecret_AckOnSuccess(t *testing.T) {
	fs := &fakeStream{resps: []*discoveryv3.DiscoveryResponse{{
		VersionInfo: "v7", Nonce: "n7",
		Resources: []*anypb.Any{validVCSecretAny(t, "validation_ca")},
	}}}
	pool, err := fetchValidationSecret(fs, Node{ID: "id", Cluster: "cl"}, "validation_ca", "")
	if err != nil {
		t.Fatalf("fetchValidationSecret: %v", err)
	}
	if pool == nil {
		t.Error("pool is nil on success")
	}
	if len(fs.sent) != 2 {
		t.Fatalf("sent %d requests, want 2 (initial + ACK)", len(fs.sent))
	}
	ack := fs.sent[1]
	if ack.GetVersionInfo() != "v7" {
		t.Errorf("ACK VersionInfo = %q, want v7 (echo the accepted version)", ack.GetVersionInfo())
	}
	if ack.GetResponseNonce() != "n7" {
		t.Errorf("ACK ResponseNonce = %q, want n7", ack.GetResponseNonce())
	}
	if ack.GetErrorDetail() != nil {
		t.Errorf("ACK carries ErrorDetail %v, want nil", ack.GetErrorDetail())
	}
}

func TestFetchValidationSecret_NackOnValidationFailure(t *testing.T) {
	// A tls_certificate secret served where a validation_context was requested:
	// parseValidationSecret rejects it -> errValidation -> NACK.
	fs := &fakeStream{resps: []*discoveryv3.DiscoveryResponse{{
		VersionInfo: "v2", Nonce: "n2",
		Resources: []*anypb.Any{validSecretAny(t, "validation_ca")},
	}}}
	_, err := fetchValidationSecret(fs, Node{ID: "id", Cluster: "cl"}, "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errValidation) {
		t.Errorf("error = %v, want it to wrap errValidation", err)
	}
	if len(fs.sent) != 2 {
		t.Fatalf("sent %d requests, want 2 (initial + NACK)", len(fs.sent))
	}
	nack := fs.sent[1]
	if nack.GetVersionInfo() != "" {
		t.Errorf("NACK VersionInfo = %q, want empty (keep the PRIOR version on reject)", nack.GetVersionInfo())
	}
	if nack.GetResponseNonce() != "n2" {
		t.Errorf("NACK ResponseNonce = %q, want n2", nack.GetResponseNonce())
	}
	if nack.GetErrorDetail() == nil {
		t.Error("NACK ErrorDetail is nil, want the validation failure detail")
	}
}

func TestFetchValidationSecret_TransportError(t *testing.T) {
	fs := &fakeStream{recvErr: errors.New("boom")}
	_, err := fetchValidationSecret(fs, Node{ID: "id", Cluster: "cl"}, "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, errValidation) {
		t.Errorf("error = %v, want a TRANSPORT error, not errValidation (a transport failure must not classify as rejected)", err)
	}
	if !strings.Contains(err.Error(), "recv response") {
		t.Errorf("error = %q, want it to mention recv response", err.Error())
	}
}

func TestApplyValidationResponse_EmptyResources(t *testing.T) {
	_, err := applyValidationResponse(&discoveryv3.DiscoveryResponse{VersionInfo: "v1", Nonce: "n1"}, "validation_ca", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errValidation) {
		t.Errorf("error = %v, want it to wrap errValidation", err)
	}
	if !strings.Contains(err.Error(), "empty resources") {
		t.Errorf("error = %q, want it to mention empty resources", err.Error())
	}
}
