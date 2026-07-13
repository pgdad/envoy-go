package driver

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/sdsserver"
)

const (
	fixtureName = "0103-xds-sds-server-cert"

	// In-container reference Envoy listener port. Convention "100NN" for
	// fixture "00NN"; fixture 0103 takes 10443 for the single TLS listener.
	refListenerPort = 10443

	// secretName is the SDS resource name delivered on both sides — fixed
	// identically in both config templates' tls_certificate_sds_secret_configs.
	secretName = "server_cert"

	// serverName is the TLS ServerName the driver dials with — must match the
	// committed leaf's SAN (pki/leaf.pem: SAN=DNS:sds.envoy-go.test).
	serverName = "sds.envoy-go.test"
)

func init() {
	fixture.RegisterFixture(fixtureName, &sdsCertDriver{})
}

// sdsCertDriver carries the per-driver lifecycle state — TWO private SDS
// receivers (one per side, mirroring the 0089 metrics_service precedent),
// the committed PKI, and a sync.Once-guarded ensure().
type sdsCertDriver struct {
	once sync.Once

	refSDSPort  int
	subjSDSPort int
	refSrv      *sdsserver.Server
	subjSrv     *sdsserver.Server

	caPool  *x509.CertPool
	leafPEM []byte
	keyPEM  []byte
}

// ensure reads the committed PKI, builds the caPool, allocates the two
// receiver ports (via Listen+Close), and starts both in-process SDS
// receivers bound to 0.0.0.0:<port> — each configured to deliver the SAME
// committed leaf under the secret name "server_cert". Idempotent — safe to
// call from both ReferenceBootstrap and SubjectConfig regardless of order.
// Both servers are live before either proxy opens its StreamSecrets stream.
func (d *sdsCertDriver) ensure() {
	d.once.Do(func() {
		d.leafPEM = mustReadPKIFile("leaf.pem")
		d.keyPEM = mustReadPKIFile("leaf.key.pem")
		caPEM := mustReadPKIFile("ca.pem")

		d.caPool = x509.NewCertPool()
		if !d.caPool.AppendCertsFromPEM(caPEM) {
			panic("driver: pki/ca.pem: no certificates parsed")
		}

		d.refSDSPort = mustAllocatePort()
		d.subjSDSPort = mustAllocatePort()

		var err error
		d.refSrv, err = sdsserver.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", d.refSDSPort), sdsserver.WithSecret(secretName, d.leafPEM, d.keyPEM))
		if err != nil {
			panic(fmt.Sprintf("driver: start reference SDS receiver: %v", err))
		}
		d.subjSrv, err = sdsserver.NewAtAddr(fmt.Sprintf("0.0.0.0:%d", d.subjSDSPort), sdsserver.WithSecret(secretName, d.leafPEM, d.keyPEM))
		if err != nil {
			panic(fmt.Sprintf("driver: start subject SDS receiver: %v", err))
		}
	})
}

// mustAllocatePort reserves a free TCP port via Listen+Close. Mirrors the
// 0089 idiom.
func mustAllocatePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate SDS port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// --- fixture.Driver (required) ---

func (*sdsCertDriver) BackendCount() int           { return 1 }
func (*sdsCertDriver) SubjectListenerName() string { return "l_sds_tls" }
func (*sdsCertDriver) ReferenceListenerPort() int  { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// reference-side SDS receiver port + the runner-allocated backend port. It
// ensures the receivers are live here so they are up before the reference
// container boots.
func (d *sdsCertDriver) ReferenceBootstrap(backendPorts []int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"SDSHost":     "host.docker.internal",
		"SDSPort":     d.refSDSPort,
		"BackendPort": backendPorts[0],
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + the loopback backend port + the subject-side SDS receiver port
// (host=127.0.0.1).
func (d *sdsCertDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.ensure()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"ListenPort":  subjListenerPort,
		"BackendPort": backendPorts[0],
		"SDSPort":     d.subjSDSPort,
	})
}

// DriveReference completes a TLS handshake against the reference proxy's
// listener and returns the served leaf's identity.
func (d *sdsCertDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveSide(ctx, addr)
}

// DriveSubject completes a TLS handshake against the subject proxy's
// listener and returns the served leaf's identity. Both proxies hold their
// long-lived SDS StreamSecrets stream open until this point — after the
// subject drive both receivers are hard-stopped via closeServers().
func (d *sdsCertDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, err := d.driveSide(ctx, addr)
	d.closeServers()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// closeServers hard-stops both receivers (idempotent via sdsserver.Server's
// own sync.Once). Called after the subject drive for deterministic teardown
// — GracefulStop would block on the still-open StreamSecrets streams.
func (d *sdsCertDriver) closeServers() {
	if d.refSrv != nil {
		d.refSrv.Close()
	}
	if d.subjSrv != nil {
		d.subjSrv.Close()
	}
}

// driveSide dials addr, completes a TLS handshake (validating against the
// committed CA + serverName), and returns the served leaf's identity as
// "serial=<hex>\nsan=<comma-joined DNSNames>\n" — the byte-exact observable
// the runner's CompareBytes step enforces cross-side. No application data is
// written; the observable is the certificate identity, not echoed bytes.
func (d *sdsCertDriver) driveSide(ctx context.Context, addr string) ([]byte, error) {
	leaf, err := helpers.TLSServedLeaf(ctx, addr, serverName, d.caPool)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("serial=%X\nsan=%s\n", leaf.SerialNumber, strings.Join(leaf.DNSNames, ","))), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin-diff at the runner's
// probe step.
func (*sdsCertDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- file / template helpers (the 0089 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0103-xds-sds-server-cert/driver/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

func mustReadPKIFile(name string) []byte {
	path := filepath.Join(fixtureDir(), "pki", name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read pki/%s: %v", name, err))
	}
	return b
}

func mustRender(tpl string, data map[string]any) string {
	t, err := template.New("bootstrap").Parse(tpl)
	if err != nil {
		panic(fmt.Sprintf("driver: template parse: %v", err))
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("driver: template execute: %v", err))
	}
	return buf.String()
}

// Compile-time interface assertion.
var _ fixture.Driver = (*sdsCertDriver)(nil)
