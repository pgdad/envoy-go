package xds

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"time"
)

// SecretProvider is the blocking seam internal/tls (60.2) uses to obtain
// SDS-delivered secrets at listener construction. Both methods are INITIAL-FETCH
// only (no rotation/watch — SPEC-60 scope, unchanged at phase 65) and are bounded
// by initial_fetch_timeout.
//
// The two methods are parallel chains over the SAME SotW machinery, differing in
// resource type and return type: a tls_certificate yields a *stdtls.Certificate
// (a downstream server leaf, phase 60.2 / ADR-0280); a validation_context yields
// an *x509.CertPool (a downstream mTLS trusted-CA, phase 65 / ADR-0286).
type SecretProvider interface {
	FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error)
	FetchInitialValidationContext(ctx context.Context, secretName string) (*x509.CertPool, error)
}

// StreamOpener opens one SotW SDS stream. Abstracted so unit tests inject an
// in-process opener and the boot-side internal/xds/xdsgrpc package wraps
// grpcclient.SDSClient. Exported so xdsgrpc (a sibling package, to keep
// internal/xds free of the grpcclient->cluster->tls transitive edge) can
// implement it.
type StreamOpener interface {
	StreamSecrets(ctx context.Context) (Stream, error)
}

// Provider is the concrete SecretProvider for one SDS secret config.
type Provider struct {
	opener  StreamOpener
	node    Node
	baseDir string
	timeout time.Duration
	stats   *SDSStats
}

// NewProvider builds a Provider. timeout is initial_fetch_timeout (default 15s —
// the caller passes the config value or the default). stats may be nil (no-op).
func NewProvider(opener StreamOpener, node Node, baseDir string, timeout time.Duration, stats *SDSStats) *Provider {
	return &Provider{opener: opener, node: node, baseDir: baseDir, timeout: timeout, stats: stats}
}

// FetchInitialCertificate opens the SDS stream, runs one SotW fetch for secretName,
// and returns the built leaf — blocking up to initial_fetch_timeout. On timeout /
// mgmt-unreachable / validation failure it returns a classified error and
// increments the matching sds.* counter. (At 60.2 a returned error boot-FAILS the
// listener — envoy-go's documented DEPARTURE from the reference's serve-cert-less.)
func (p *Provider) FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error) {
	p.stats.incUpdateAttempt()
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	stream, err := p.opener.StreamSecrets(ctx)
	if err != nil {
		p.stats.incUpdateFailure()
		return nil, fmt.Errorf("xds: sds: secret %q: open stream: %w", secretName, err)
	}
	cert, err := fetchSecret(stream, p.node, secretName, p.baseDir)
	if err != nil {
		switch {
		case errors.Is(err, errValidation):
			p.stats.incUpdateRejected()
			return nil, err
		case ctx.Err() != nil: // deadline / cancel during recv
			p.stats.incInitFetchTimeout()
			return nil, fmt.Errorf("xds: sds: secret %q: initial fetch timed out after %s: %w", secretName, p.timeout, ctx.Err())
		default:
			p.stats.incUpdateFailure()
			return nil, err
		}
	}
	p.stats.incUpdateSuccess()
	return cert, nil
}

// FetchInitialValidationContext performs the ONE bounded initial fetch of an
// SDS-delivered validation_context and returns the trusted-CA pool.
//
// Byte-parallel to FetchInitialCertificate: the same timeout bound, the same
// SDSStats accounting, and the same error classification (rejected /
// init-fetch-timeout / failure). A timeout or unreachable management server
// returns an error, which boot-FAILS the listener — the documented envoy-go
// DEPARTURE (ADR-0280 family; reference characterization corrected at
// ADR-0289): the reference init-holds (port unbound), then at
// initial_fetch_timeout starts workers and binds, then fails closed
// per-connection (downstream_context_secrets_not_ready) — it never serves
// TLS traffic without the resource. (SPEC-65 §11 D-SDSVC-FETCHTIMEOUT.)
func (p *Provider) FetchInitialValidationContext(ctx context.Context, secretName string) (*x509.CertPool, error) {
	p.stats.incUpdateAttempt()
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	stream, err := p.opener.StreamSecrets(ctx)
	if err != nil {
		p.stats.incUpdateFailure()
		return nil, fmt.Errorf("xds: sds: secret %q: open stream: %w", secretName, err)
	}
	pool, err := fetchValidationSecret(stream, p.node, secretName, p.baseDir)
	if err != nil {
		switch {
		case errors.Is(err, errValidation):
			p.stats.incUpdateRejected()
			return nil, err
		case ctx.Err() != nil: // deadline / cancel during recv
			p.stats.incInitFetchTimeout()
			return nil, fmt.Errorf("xds: sds: secret %q: initial fetch timed out after %s: %w", secretName, p.timeout, ctx.Err())
		default:
			p.stats.incUpdateFailure()
			return nil, err
		}
	}
	p.stats.incUpdateSuccess()
	return pool, nil
}
