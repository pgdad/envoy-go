// Package main regenerates the phase-27 sni_cluster TLS test PKI
// deterministically.
//
// Usage (from the repo root):
//
//	cd test/fixtures/0045-sni-cluster && go run ./pki/gen
//
// Produces byte-identical PEMs on every run. Intended to run manually; CI
// never invokes this command. The committed PEMs are the authoritative source
// used by tests; re-run this command only to rotate (and update the NotBefore
// / NotAfter constants below).
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
	"os"
	"path/filepath"
	"time"
)

var (
	notBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter  = time.Date(2046, 1, 1, 0, 0, 0, 0, time.UTC)
)

// Deterministic seed: SHA-256 of the string "envoy-go/test/fixtures/0045-sni-cluster/pki/gen/v1".
// Flipping any byte invalidates every committed PEM; re-run `go run ./pki/gen` to regenerate.
var seed = [32]byte{
	0x3b, 0x7e, 0x9c, 0xf1, 0x2d, 0x85, 0x41, 0xa6,
	0x68, 0x0f, 0xcc, 0x52, 0x94, 0x1b, 0x77, 0xe3,
	0xd9, 0x5a, 0x3f, 0x08, 0xb2, 0xe4, 0x71, 0x9d,
	0x6c, 0x23, 0xf8, 0x45, 0xa0, 0x87, 0x1e, 0x5b,
}

var serials = map[string]int64{
	"server": 10,
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

// genKey generates a deterministic P-256 ECDSA private key. Mirrors the
// technique in 0002-tls-tcp/pki/gen/main.go (Go 1.26 custom-reader bypass).
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
			continue
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

	// server — covers both foo.example.com and unknown.example.com (SANs) so a
	// single filter chain can terminate TLS for all three fixture arms. The
	// no-SNI arm uses InsecureSkipVerify so it does not need a matching SAN.
	genLeaf(outDir, "server", "foo.example.com",
		[]string{"foo.example.com", "unknown.example.com"},
		caCert, caKey)

	fmt.Println("ok: 3 PEMs written to", outDir)
}

func genCA(tag string) (*ecdsa.PrivateKey, []byte, *x509.Certificate) {
	key := genKey(tag)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "envoy-go test CA (0045)"},
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

func genLeaf(outDir, tag, cn string, dnsNames []string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
	key := genKey(tag)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serials[tag]),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
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
	must(os.WriteFile(path, pemBytes, 0o644))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
