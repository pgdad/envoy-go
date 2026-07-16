package xds

import (
	stdtls "crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
)

// Node carries the DiscoveryRequest node identity (id + cluster), populated from
// bootstrap `node`. Both are REQUIRED for SDS by the reference (the boot-reject
// that enforces this is 60.2; 60.1 just transmits them).
type Node struct {
	ID      string
	Cluster string
}

// Stream is the 2-method send/recv seam fetchSecret operates over — satisfied
// by the real *grpcclient SDS client stream (via the internal/xds/xdsgrpc
// boot-side adapter) AND by an in-memory fake in tests. Exported so a sibling
// package can construct a StreamOpener that returns one.
type Stream interface {
	Send(*discoveryv3.DiscoveryRequest) error
	Recv() (*discoveryv3.DiscoveryResponse, error)
}

// errValidation classifies a delivered-but-invalid Secret (a NACK-worthy reject —
// the caller increments update_rejected), distinct from a transport error.
var errValidation = errors.New("xds: sds: secret validation failed")

// fetchSecret runs one State-of-the-World fetch: sends the initial DiscoveryRequest
// for secretName, Recv's the first response, parses+validates its Secret, and ACKs
// (echoing version_info+nonce) on success or NACKs (keeping the prior version,
// setting error_detail) on a validation failure. Returns the built leaf on success.
func fetchSecret(stream Stream, node Node, secretName, baseDir string) (*stdtls.Certificate, error) {
	typeURL := secretTypeURL()
	initial := &discoveryv3.DiscoveryRequest{
		VersionInfo:   "",
		ResponseNonce: "",
		ResourceNames: []string{secretName},
		TypeUrl:       typeURL,
		Node:          &corev3.Node{Id: node.ID, Cluster: node.Cluster},
	}
	if err := stream.Send(initial); err != nil {
		return nil, fmt.Errorf("xds: sds: send initial request: %w", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("xds: sds: recv response: %w", err)
	}
	cert, verr := applyResponse(resp, secretName, baseDir)
	if verr != nil {
		// NACK: keep the prior version_info (""), set error_detail.
		nack := &discoveryv3.DiscoveryRequest{
			VersionInfo:   initial.VersionInfo,
			ResponseNonce: resp.GetNonce(),
			ResourceNames: []string{secretName},
			TypeUrl:       typeURL,
			Node:          initial.Node,
			ErrorDetail:   &statuspb.Status{Message: verr.Error()},
		}
		_ = stream.Send(nack)
		return nil, verr
	}
	// ACK: echo the accepted (version_info, nonce).
	ack := &discoveryv3.DiscoveryRequest{
		VersionInfo:   resp.GetVersionInfo(),
		ResponseNonce: resp.GetNonce(),
		ResourceNames: []string{secretName},
		TypeUrl:       typeURL,
		Node:          initial.Node,
	}
	if err := stream.Send(ack); err != nil {
		return nil, fmt.Errorf("xds: sds: send ack: %w", err)
	}
	return cert, nil
}

// applyResponse extracts resources[0] and applies it (parseSecret), returning a
// validation-classified error (wrapping errValidation) on any parse/validate
// failure. A response with no resources for the requested name is a validation
// failure (the server delivered nothing usable).
func applyResponse(resp *discoveryv3.DiscoveryResponse, secretName, baseDir string) (*stdtls.Certificate, error) {
	if len(resp.GetResources()) == 0 {
		return nil, fmt.Errorf("%w: empty resources", errValidation)
	}
	cert, err := parseSecret(resp.GetResources()[0], secretName, baseDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errValidation, err)
	}
	return cert, nil
}

// fetchValidationSecret runs ONE State-of-the-World exchange for an SDS-delivered
// validation_context: send the initial DiscoveryRequest, receive one
// DiscoveryResponse, ACK on success / NACK on validation failure, and return the
// trusted-CA pool.
//
// It is byte-parallel to fetchSecret (stream.go:38) — the version/nonce ACK/NACK
// dance is IDENTICAL and errValidation is SHARED. Only the applier call and the
// return type differ. The duplication is deliberate (SPEC-65 §3.2, Option A): the
// landed *stdtls.Certificate chain is load-bearing for the passing 0103
// differential, and generifying it would put 0103 at risk for no behavior gain.
//
// The requested type URL is secretTypeURL() — the SAME as the cert path: a
// validation_context is the same tls.v3.Secret message, differing only in the
// Type oneof arm. There is no second resource type on the wire.
func fetchValidationSecret(stream Stream, node Node, secretName, baseDir string) (*x509.CertPool, error) {
	initial := &discoveryv3.DiscoveryRequest{
		VersionInfo:   "",
		ResponseNonce: "",
		ResourceNames: []string{secretName},
		TypeUrl:       secretTypeURL(),
		Node:          &corev3.Node{Id: node.ID, Cluster: node.Cluster},
	}
	if err := stream.Send(initial); err != nil {
		return nil, fmt.Errorf("xds: sds: send initial request: %w", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("xds: sds: recv response: %w", err)
	}
	pool, verr := applyValidationResponse(resp, secretName, baseDir)
	if verr != nil {
		nack := &discoveryv3.DiscoveryRequest{
			VersionInfo:   initial.VersionInfo, // keep the PRIOR version on reject
			ResponseNonce: resp.GetNonce(),
			ResourceNames: []string{secretName},
			TypeUrl:       secretTypeURL(),
			Node:          &corev3.Node{Id: node.ID, Cluster: node.Cluster},
			ErrorDetail:   &statuspb.Status{Message: verr.Error()},
		}
		_ = stream.Send(nack) // best-effort: the fetch already failed
		return nil, verr
	}
	ack := &discoveryv3.DiscoveryRequest{
		VersionInfo:   resp.GetVersionInfo(),
		ResponseNonce: resp.GetNonce(),
		ResourceNames: []string{secretName},
		TypeUrl:       secretTypeURL(),
		Node:          &corev3.Node{Id: node.ID, Cluster: node.Cluster},
	}
	if err := stream.Send(ack); err != nil {
		return nil, fmt.Errorf("xds: sds: send ack: %w", err)
	}
	return pool, nil
}

// applyValidationResponse validates a DiscoveryResponse carrying a
// validation_context Secret and returns the trusted-CA pool. Every failure wraps
// errValidation so the caller NACKs (and the Provider classifies it as
// update_rejected rather than update_failure). Sibling of applyResponse
// (stream.go:86); like it, only resources[0] is read.
func applyValidationResponse(resp *discoveryv3.DiscoveryResponse, secretName, baseDir string) (*x509.CertPool, error) {
	if len(resp.GetResources()) == 0 {
		return nil, fmt.Errorf("%w: empty resources", errValidation)
	}
	pool, err := parseValidationSecret(resp.GetResources()[0], secretName, baseDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errValidation, err)
	}
	return pool, nil
}
