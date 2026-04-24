// Package main regenerates the phase-03 TLS test PKI deterministically.
//
// Usage (from the repo root):
//
//	cd test/fixtures/0002-tls-tcp && go run ./pki/gen
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
	"net"
	"os"
	"path/filepath"
	"time"
)

var (
	notBefore = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter  = time.Date(2046, 1, 1, 0, 0, 0, 0, time.UTC)
)

// Deterministic seed: SHA-256 of the string "envoy-go/test/fixtures/0002-tls-tcp/pki/gen/v1".
// Flipping any byte invalidates every committed PEM; re-run `go run ./pki/gen` to regenerate.
var seed = [32]byte{
	0x9f, 0x2a, 0xd7, 0x1c, 0x55, 0x84, 0xe3, 0x62,
	0x40, 0x11, 0xaa, 0x7b, 0xbc, 0x08, 0x3e, 0x91,
	0xd4, 0x7f, 0x66, 0x9e, 0x20, 0xcb, 0x55, 0x17,
	0x8a, 0x03, 0xfa, 0x49, 0xd6, 0xe7, 0x2d, 0xb0,
}

var serials = map[string]int64{
	"server-alpha":   10,
	"server-beta":    11,
	"upstream-alpha": 20,
	"upstream-beta":  21,
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
// In Go 1.26, crypto/ecdh.P256().GenerateKey routes custom io.Reader arguments
// through crypto/internal/rand.CustomReader, which — unless the GODEBUG setting
// "cryptocustomrand=1" is active — silently replaces the caller's reader with
// the system DRBG, making the output non-deterministic.
// crypto/ecdsa.GenerateKey has the same issue via randutil.MaybeReadByte.
//
// The fix: generate the raw 32-byte P-256 scalar directly from the ChaCha8
// stream and call ecdh.P256().NewPrivateKey, which does not touch any entropy
// source. The resulting ecdh.PrivateKey is then converted to ecdsa.PrivateKey
// via the scalar D and the uncompressed public-key point bytes.
func genKey(tag string) *ecdsa.PrivateKey {
	rng := newChaCha8(tag + "-key")
	var scalar [32]byte
	var buf [8]byte
	// Rejection-sample until we get a valid in-range scalar. For P-256 the
	// order n ≈ 2^256, so rejection probability is < 2^-128 — effectively zero.
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
		// Public key bytes for P-256: 0x04 || X (32 bytes) || Y (32 bytes).
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

	// server-alpha
	genLeaf(outDir, "server-alpha", "alpha.envoy-go.test", []string{"alpha.envoy-go.test"}, nil, caCert, caKey)
	// server-beta
	genLeaf(outDir, "server-beta", "beta.envoy-go.test", []string{"beta.envoy-go.test"}, nil, caCert, caKey)
	// upstream-alpha — DNS SANs + IP SAN for subject-side 127.0.0.1 connectivity
	genLeaf(outDir, "upstream-alpha", "alpha.envoy-go.test", []string{"alpha.envoy-go.test", "localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, caCert, caKey)
	// upstream-beta
	genLeaf(outDir, "upstream-beta", "beta.envoy-go.test", []string{"beta.envoy-go.test", "localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, caCert, caKey)

	fmt.Println("ok: 9 PEMs written to", outDir)
}

func genCA(tag string) (*ecdsa.PrivateKey, []byte, *x509.Certificate) {
	key := genKey(tag)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "envoy-go test CA"},
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
