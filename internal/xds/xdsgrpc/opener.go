// Package xdsgrpc is the boot-side production adapter wiring a
// *grpcclient.SDSClient into the internal/xds substrate's xds.StreamOpener
// seam.
//
// It lives in its own sibling package — NOT in internal/xds — deliberately:
// internal/grpcclient transitively imports internal/cluster, which imports
// internal/tls. At phase 60.2, internal/tls imports internal/xds (to
// reference xds.SecretProvider as a param type for the SDS-sourced downstream
// certificate seam). If internal/xds itself imported internal/grpcclient, that
// would close a real import cycle:
//
//	tls -> xds -> grpcclient -> cluster -> tls
//
// By keeping the grpcclient dependency here, in xdsgrpc, internal/xds stays
// free of internal/grpcclient (and therefore internal/cluster and
// internal/tls) entirely — only xds.Stream / xds.StreamOpener (interface
// seams) cross the package boundary. internal/xds/doc.go documents the
// resulting guarantee ("this package does NOT import internal/tls"). 60.2's
// boot path constructs the *grpcclient.SDSClient, wraps it here via
// NewOpener, and hands the result (an xds.StreamOpener) to xds.NewProvider.
package xdsgrpc

import (
	"context"

	"github.com/pgdad/envoy-go/internal/grpcclient"
	"github.com/pgdad/envoy-go/internal/xds"
)

// Opener adapts a *grpcclient.SDSClient to xds.StreamOpener.
type Opener struct {
	client *grpcclient.SDSClient
}

// NewOpener wraps client as an xds.StreamOpener.
func NewOpener(client *grpcclient.SDSClient) xds.StreamOpener {
	return &Opener{client: client}
}

// StreamSecrets opens the underlying SDS bidi stream. The returned
// secretv3.SecretDiscoveryService_StreamSecretsClient satisfies xds.Stream
// structurally (Send(*DiscoveryRequest) error / Recv() (*DiscoveryResponse,
// error)) with no further adapting.
func (o *Opener) StreamSecrets(ctx context.Context) (xds.Stream, error) {
	s, err := o.client.StreamSecrets(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}
