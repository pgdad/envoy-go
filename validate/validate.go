// Package validate validates an Envoy v3 Bootstrap config the same way
// envoy-go's normal boot path would construct it: parsing, building the
// cluster manager (including upstream TLS cert resolution), and building
// every listener's full filter chain (routes, HTTP filters, TLS
// certificates, Lua compilation) — without binding any socket, opening any
// access-log file, dialing any stats-sink UDP socket, or starting the admin
// server / any background loop. Motivated by Kubernetes Gateway API
// implementations (e.g. Envoy Gateway) needing to validate envoy-go-
// generated bootstrap config before applying it to a live proxy (phase 51,
// ADR-0268).
package validate

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pgdad/envoy-go/internal/boot"
	"github.com/pgdad/envoy-go/internal/bootstrap"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/drain"
	"github.com/pgdad/envoy-go/internal/grpcclient"
	"github.com/pgdad/envoy-go/internal/httpclient"
)

// Bootstrap validates the config read from r. baseDir resolves relative
// file paths within the config (TLS certs, Lua scripts) the same way
// cmd/envoy-go/main.go's own filepath.Dir(cfgPath) does. allowH2C mirrors
// main.go's -allow-h2c test-only flag (permits HCM codec_type=HTTP2 on
// plaintext listeners). Returns nil if the configuration is valid, or a
// descriptive error otherwise — the first error encountered; envoy-go's
// validation is fail-fast throughout, not multi-diagnostic.
func Bootstrap(r io.Reader, baseDir string, allowH2C bool) error {
	bs, err := bootstrap.Load(r)
	if err != nil {
		return err
	}
	cm, err := cluster.NewManagerWithBaseDir(bs.Proto, baseDir, bs.Stats)
	if err != nil {
		return err
	}
	dm := drain.New(30 * time.Second)
	httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})
	dialer := grpcclient.New(cm)
	tracingProvider := boot.NewTracingProvider(dialer, httpClient, cm, bs.Stats)
	defer func() { _ = tracingProvider.CloseAll() }()
	_, err = boot.Construct(bs, cm, baseDir, allowH2C, nil, dm, httpClient, tracingProvider)
	return err
}

// BootstrapFile opens path and validates it (with allowH2C false — the
// -allow-h2c flag is test-only, not a production Gateway API concern),
// using its directory as baseDir — mirroring cmd/envoy-go/main.go's own
// cfgPath/filepath.Dir(cfgPath) pairing.
func BootstrapFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return Bootstrap(f, filepath.Dir(path), false)
}
