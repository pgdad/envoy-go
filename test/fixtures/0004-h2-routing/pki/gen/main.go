// Package main regenerates the phase-05.2 H2 fixture's TLS PKI deterministically.
//
// Usage (from the repo root):
//
//	cd test/fixtures/0004-h2-routing && go run ./pki/gen
//
// Produces byte-identical PEMs on every run. Intended to run manually; CI never
// invokes this command. The committed PEMs are the authoritative source used by
// fixture 0004; re-run only to rotate (and update the NotBefore / NotAfter
// constants below). Mirrors fixture 0002's generator structure verbatim except
// for the seed bytes, the serial map, and the leaf-cert layout (1 listener +
// 3 backends, all carrying the same SAN set so the same PKI works for both
// the host-side STATIC subject and the Docker-side STRICT_DNS reference).
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

// Deterministic seed: SHA-256 of the string "envoy-go/test/fixtures/0004-h2-routing/pki/gen/v1".
// Flipping any byte invalidates every committed PEM; re-run `go run ./pki/gen` to regenerate.
var seed = [32]byte{
	0x4a, 0xc2, 0x18, 0x77, 0x91, 0x3e, 0xbb, 0x55,
	0x29, 0x6e, 0x82, 0xd4, 0x10, 0xa7, 0x6f, 0x33,
	0xe1, 0x09, 0x5b, 0xcc, 0x7d, 0x4e, 0x90, 0x21,
	0xb6, 0x3a, 0xf2, 0x68, 0x05, 0x8d, 0xc4, 0x7e,
}

var serials = map[string]int64{
	"listener":  10,
	"backend-0": 20,
	"backend-1": 21,
	"backend-2": 22,
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
// Mirrors fixture-0002's genKey verbatim: rejection-samples a 32-byte scalar
// from the ChaCha8 stream and hands it to ecdh.P256().NewPrivateKey, which is
// the ONE construction path that bypasses Go 1.26's CustomReader DRBG-replace
// behavior (cryptocustomrand=1 GODEBUG would otherwise be required to make
// ecdh.GenerateKey / ecdsa.GenerateKey honor a custom io.Reader).
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

	// CA
	caKey, caPEM, caCert := genCA("ca")
	writePEM(filepath.Join(outDir, "ca.pem"), caPEM)

	// Every leaf carries the same SAN set: DNS localhost + DNS host.docker.internal +
	// IP 127.0.0.1. Both subject (STATIC, dials 127.0.0.1) and reference (STRICT_DNS,
	// resolves host.docker.internal to the host's IP) validate the same cert.
	leafDNS := []string{"localhost", "host.docker.internal"}
	leafIPs := []net.IP{net.ParseIP("127.0.0.1")}

	genLeaf(outDir, "listener", "envoy-go test listener", leafDNS, leafIPs, caCert, caKey)
	genLeaf(outDir, "backend-0", "envoy-go test backend-0", leafDNS, leafIPs, caCert, caKey)
	genLeaf(outDir, "backend-1", "envoy-go test backend-1", leafDNS, leafIPs, caCert, caKey)
	genLeaf(outDir, "backend-2", "envoy-go test backend-2", leafDNS, leafIPs, caCert, caKey)

	fmt.Println("ok: 9 PEMs written to", outDir)
}

func genCA(tag string) (*ecdsa.PrivateKey, []byte, *x509.Certificate) {
	key := genKey(tag)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "envoy-go h2 fixture CA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	// nil rand → ecdsa.Sign uses RFC 6979 deterministic k-generation (no entropy consumed).
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
	// nil rand → ecdsa.Sign uses RFC 6979 deterministic k-generation (no entropy consumed).
	der, err := x509.CreateCertificate(nil, tmpl, caCert, &key.PublicKey, caKey)
	must(err)
	writePEM(filepath.Join(outDir, tag+".pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	must(err)
	writePEM(filepath.Join(outDir, tag+".key.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func writePEM(path string, pemBytes []byte) {
	must(os.WriteFile(path, pemBytes, 0o644))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
