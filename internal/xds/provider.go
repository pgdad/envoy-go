package xds

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"fmt"
	"time"
)

// SecretProvider is the blocking seam internal/tls (60.2) uses to obtain an
// SDS-delivered downstream server certificate at listener construction. Bounded by
// initial_fetch_timeout. INITIAL-FETCH only — no rotation.
type SecretProvider interface {
	FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error)
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
