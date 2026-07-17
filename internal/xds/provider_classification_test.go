package xds

import (
	"context"
	"testing"
	"time"

	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"

	"github.com/pgdad/envoy-go/internal/stats"
)

// This file closes the coverage gap RECORDED at
// docs/envoy-go/phases/65-xds-sds-validation-context/PROGRESS.md:122: the
// Provider's error-classification `switch` checks errors.Is(err, errValidation)
// BEFORE ctx.Err() != nil, and a deliberate break swapping that ordering did
// not fire any existing test — no test created the discriminating condition (a
// validation failure arriving while the initial-fetch deadline has ALREADY
// expired). The tests below create exactly that condition for BOTH fetch
// chains: the stream's Recv blocks until the provider's own
// initial_fetch_timeout context fires, then returns a response that FAILS
// validation (rather than surfacing the ctx error). The resulting error wraps
// errValidation while ctx.Err() != nil — so a swapped classification would
// count init_fetch_timeout instead of update_rejected, and these tests fail.
//
// Deterministic: no sleeps — the fake blocks on the provider's own ctx.Done()
// channel, so the discriminating race is constructed, not timed.

// raceClassifierOpener returns a Stream whose Recv waits for the fetch ctx to
// be canceled (the initial_fetch_timeout firing) and THEN returns resp with a
// nil error — modeling a management server whose (invalid) response lands just
// as the deadline expires.
type raceClassifierOpener struct {
	resp *discoveryv3.DiscoveryResponse
	ctx  context.Context
}

func (o *raceClassifierOpener) StreamSecrets(ctx context.Context) (Stream, error) {
	o.ctx = ctx
	return &raceClassifierStream{o: o}, nil
}

type raceClassifierStream struct{ o *raceClassifierOpener }

func (s *raceClassifierStream) Send(*discoveryv3.DiscoveryRequest) error { return nil }

func (s *raceClassifierStream) Recv() (*discoveryv3.DiscoveryResponse, error) {
	<-s.o.ctx.Done() // deadline has fired; deliver the (invalid) response anyway
	return s.o.resp, nil
}

func TestProvider_FetchInitialCertificate_ValidationFailureAfterDeadline_ClassifiesRejected(t *testing.T) {
	reg := stats.NewRegistry()
	sdsStats := RegisterSDSStats(reg, "server_cert")
	// An empty-resources response fails validation (wraps errValidation).
	op := &raceClassifierOpener{resp: &discoveryv3.DiscoveryResponse{
		VersionInfo: "v1", Nonce: "n1",
		Resources: nil, // empty resources → "empty resources" validation failure
	}}

	p := NewProvider(op, Node{ID: "n", Cluster: "c"}, "", 50*time.Millisecond, sdsStats)

	cert, err := p.FetchInitialCertificate(context.Background(), "server_cert")
	if err == nil {
		t.Fatal("FetchInitialCertificate() error = nil, want a validation failure")
	}
	if cert != nil {
		t.Errorf("FetchInitialCertificate() cert = %v, want nil", cert)
	}

	if got := sdsStats.updateRejected.Load(); got != 1 {
		t.Errorf("update_rejected = %d, want 1 — a validation failure must classify as REJECTED even when the deadline has already expired (errValidation is checked before ctx.Err())", got)
	}
	if got := sdsStats.initFetchTimeout.Load(); got != 0 {
		t.Errorf("init_fetch_timeout = %d, want 0 — the deadline must NOT preempt the validation classification", got)
	}
}

func TestProvider_FetchInitialValidationContext_ValidationFailureAfterDeadline_ClassifiesRejected(t *testing.T) {
	reg := stats.NewRegistry()
	sdsStats := RegisterSDSStats(reg, "validation_ca")
	// An empty-resources response fails validation (wraps errValidation).
	op := &raceClassifierOpener{resp: &discoveryv3.DiscoveryResponse{
		VersionInfo: "v1", Nonce: "n1",
		Resources: nil, // empty resources → "empty resources" validation failure
	}}

	p := NewProvider(op, Node{ID: "n", Cluster: "c"}, "", 50*time.Millisecond, sdsStats)

	pool, err := p.FetchInitialValidationContext(context.Background(), "validation_ca")
	if err == nil {
		t.Fatal("FetchInitialValidationContext() error = nil, want a validation failure")
	}
	if pool != nil {
		t.Errorf("FetchInitialValidationContext() pool = %v, want nil", pool)
	}

	if got := sdsStats.updateRejected.Load(); got != 1 {
		t.Errorf("update_rejected = %d, want 1 — a validation failure must classify as REJECTED even when the deadline has already expired (errValidation is checked before ctx.Err())", got)
	}
	if got := sdsStats.initFetchTimeout.Load(); got != 0 {
		t.Errorf("init_fetch_timeout = %d, want 0 — the deadline must NOT preempt the validation classification", got)
	}
}
