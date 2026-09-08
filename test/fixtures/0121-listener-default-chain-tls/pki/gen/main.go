// Package main regenerates fixture 0121-listener-default-chain-tls's TLS PKI
// deterministically.
//
// Usage (from the repo root):
//
//	cd test/fixtures/0121-listener-default-chain-tls && go run ./pki/gen
//
// Produces byte-identical PEMs on every run. Intended to run manually; CI never
// invokes this command. The committed PEMs are the authoritative source used by
// fixture 0121; re-run only to rotate (and update the NotBefore / NotAfter
// constants below). Mirrors fixture 0004's generator structure verbatim except
// for the seed bytes, the serial map, and the leaf layout.
//
// ⚠️ THREE PEMs ONLY — ca, server leaf, server key. There is deliberately NO
// client leaf: the listener under test carries NO require_client_certificate,
// so it sends no CertificateRequest, and that absence is exactly what makes
// listener.<addr>.ssl.no_certificate move once per completed handshake. A
// client leaf in this directory would invite an arm that presents one, which
// would change the measured map.
package main

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"math/big"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"time"
)

var (
	notBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter  = time.Date(2046, 1, 1, 0, 0, 0, 0, time.UTC)
)

// Deterministic seed for this fixture's PKI. Flipping any byte invalidates
// every committed PEM; re-run `go run ./pki/gen` to regenerate.
var seed = [32]byte{
	0x01, 0x21, 0xdf, 0xc0, 0x7a, 0x4b, 0x91, 0x2e,
	0x63, 0xb8, 0x05, 0xd7, 0xaa, 0x1c, 0x38, 0xf6,
	0x52, 0x9d, 0x14, 0xe0, 0x6b, 0xc3, 0x77, 0x28,
	0x8f, 0x40, 0xa5, 0xd9, 0x11, 0x36, 0xec, 0x74,
}

var serials = map[string]int64{
	"server": 121,
}

// newChaCha8 returns a ChaCha8 PRNG seeded from the master seed XOR'd with the
// tag bytes. Each (tag, role) pair gets an independent deterministic stream.
func newChaCha8(tag string) *rand.ChaCha8 {
	var s [32]byte
	copy(s[:], seed[:])
	for i, b := range []byte(tag) {
		s[i%32] ^= b
	}
	return rand.NewChaCha8(s)
}

// genKey generates a deterministic P-256 ECDSA private key.
//
// Mirrors fixture-0002/0004's genKey verbatim: rejection-samples a 32-byte
// scalar from the ChaCha8 stream and hands it to ecdh.P256().NewPrivateKey,
// which is the ONE construction path that bypasses Go 1.26's CustomReader
// DRBG-replace behavior.
func genKey(tag string) *ecdsa.PrivateKey {
	rng := newChaCha8(tag + "-key")
	var scalar [32]byte
	var buf [8]byte
	for {
		for i := 0; i < 4; i++ {
			binary.LittleEndian.PutUint64(buf[:], rng.Uint64())
			copy(scalar[i*8:], buf[:])
		}
		ecdhKey, err := ecdh.P256().NewPrivateKey(scalar[:])
		if err != nil {
			continue // scalar was zero or >= n; astronomically rare
		}
		curve := elliptic.P256()
		d := new(big.Int).SetBytes(ecdhKey.Bytes())
		pub := ecdhKey.PublicKey().Bytes()
		byteLen := (curve.Params().BitSize + 7) / 8
		x := new(big.Int).SetBytes(pub[1 : 1+byteLen])
		y := new(big.Int).SetBytes(pub[1+byteLen:])
		return &ecdsa.PrivateKey{
			PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y},
			D:         d,
		}
	}
}

func main() {
	outDir := "pki"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	outDir = filepath.Clean(outDir)
	must(os.MkdirAll(outDir, 0o755))

	caKey, caPEM, caCert := genCA("ca")
	writePEM(filepath.Join(outDir, "ca.pem"), caPEM)

	// ⚠️ The leaf MUST carry a DNS SAN matching the serverName the driver
	// dials (driver.go's serverName = "l_dfc.fixture.test"), or every arm
	// fails verification CLIENT-side and the five ssl.* counters read zero
	// with no server-side fault at all.
	leafDNS := []string{"l_dfc.fixture.test", "localhost", "host.docker.internal"}
	leafIPs := []net.IP{net.ParseIP("127.0.0.1")}

	genLeaf(outDir, "server", "envoy-go 0121 default-chain listener", leafDNS, leafIPs, caCert, caKey)

	fmt.Println("ok: 3 PEMs written to", outDir)
}

func genCA(tag string) (*ecdsa.PrivateKey, []byte, *x509.Certificate) {
	key := genKey(tag)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "envoy-go 0121 default-chain fixture CA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	// nil rand → ecdsa.Sign uses RFC 6979 deterministic k-generation.
	der, err := x509.CreateCertificate(nil, tmpl, tmpl, &key.PublicKey, key)
	must(err)
	cert, err := x509.ParseCertificate(der)
	must(err)
	return key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert
}

func genLeaf(outDir, tag, cn string, dnsNames []string, ips []net.IP, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
	key := genKey(tag)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serials[tag]),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	// nil rand → ecdsa.Sign uses RFC 6979 deterministic k-generation.
	der, err := x509.CreateCertificate(nil, tmpl, caCert, &key.PublicKey, caKey)
	must(err)
	writePEM(filepath.Join(outDir, tag+".pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	must(err)
	writePEM(filepath.Join(outDir, tag+".key.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func writePEM(path string, pemBytes []byte) {
	must(os.WriteFile(path, pemBytes, 0o644)) //nolint:gosec // fixture PEMs are public test material
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
