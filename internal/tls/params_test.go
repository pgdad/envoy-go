package tls

import (
	stdtls "crypto/tls"
	"strings"
	"testing"

	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
)

func TestApplyTLSParams_Versions(t *testing.T) {
	cases := []struct {
		name          string
		min, max      tlsv3.TlsParameters_TlsProtocol
		wantMin, max2 uint16
		wantErr       string // substring; "" = no error
	}{
		{"defaults TLS 1.2 -> TLS 1.3", tlsv3.TlsParameters_TLSv1_2, tlsv3.TlsParameters_TLSv1_3, stdtls.VersionTLS12, stdtls.VersionTLS13, ""},
		{"TLS 1.2 only", tlsv3.TlsParameters_TLSv1_2, tlsv3.TlsParameters_TLSv1_2, stdtls.VersionTLS12, stdtls.VersionTLS12, ""},
		{"TLS 1.3 only", tlsv3.TlsParameters_TLSv1_3, tlsv3.TlsParameters_TLSv1_3, stdtls.VersionTLS13, stdtls.VersionTLS13, ""},
		{"TLS 1.0 min errors", tlsv3.TlsParameters_TLSv1_0, tlsv3.TlsParameters_TLSv1_3, 0, 0, "TLSv1_0"},
		{"TLS 1.1 max errors", tlsv3.TlsParameters_TLSv1_2, tlsv3.TlsParameters_TLSv1_1, 0, 0, "TLSv1_1"},
		// TLS_AUTO is treated as "not set" (no-op) per ADR-0030 and the Step 3
		// implementation of mapTLSVersion. The PLAN draft had this case expect
		// an error, but that contradicts the Step 3 code and ADR-0030 mapping
		// table (both unambiguously describe TLS_AUTO as no-op). Corrected here
		// to match implementation: MinVersion stays 0 (cfg default), MaxVersion
		// is set from TlsMaximumProtocolVersion.
		{"TLS_AUTO min no-op", tlsv3.TlsParameters_TLS_AUTO, tlsv3.TlsParameters_TLSv1_3, 0, stdtls.VersionTLS13, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &stdtls.Config{}
			err := applyTLSParams(cfg, &tlsv3.TlsParameters{
				TlsMinimumProtocolVersion: tc.min,
				TlsMaximumProtocolVersion: tc.max,
			})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("want error containing %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.MinVersion != tc.wantMin || cfg.MaxVersion != tc.max2 {
				t.Errorf("got Min=%d Max=%d, want Min=%d Max=%d", cfg.MinVersion, cfg.MaxVersion, tc.wantMin, tc.max2)
			}
		})
	}
}

func TestApplyTLSParams_CipherSuites(t *testing.T) {
	t.Run("known TLS 1.2 cipher", func(t *testing.T) {
		cfg := &stdtls.Config{}
		// ECDHE-ECDSA-AES128-GCM-SHA256 = 0xc02b
		if err := applyTLSParams(cfg, &tlsv3.TlsParameters{CipherSuites: []string{"ECDHE-ECDSA-AES128-GCM-SHA256"}}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.CipherSuites) != 1 || cfg.CipherSuites[0] != stdtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 {
			t.Errorf("got cipher_suites %v, want [0xc02b]", cfg.CipherSuites)
		}
	})
	t.Run("unknown cipher errors", func(t *testing.T) {
		cfg := &stdtls.Config{}
		err := applyTLSParams(cfg, &tlsv3.TlsParameters{CipherSuites: []string{"TOTALLY_FAKE_CIPHER"}})
		if err == nil || !strings.Contains(err.Error(), "unknown cipher suite") {
			t.Errorf("want unknown cipher error, got: %v", err)
		}
	})
	t.Run("TLS 1.3 cipher silently dropped with diagnostic", func(t *testing.T) {
		cfg := &stdtls.Config{}
		// TLS_AES_128_GCM_SHA256 = 0x1301 is TLS 1.3 only
		err := applyTLSParams(cfg, &tlsv3.TlsParameters{CipherSuites: []string{"TLS_AES_128_GCM_SHA256"}})
		if err != nil {
			t.Fatalf("unexpected error for TLS-1.3-only cipher (should be silently dropped): %v", err)
		}
		if len(cfg.CipherSuites) != 0 {
			t.Errorf("TLS-1.3-only cipher should be dropped, got cipher_suites %v", cfg.CipherSuites)
		}
	})
}

func TestApplyTLSParams_ECDHCurves(t *testing.T) {
	cases := []struct {
		name   string
		input  []string
		want   []stdtls.CurveID
		errSub string
	}{
		{"x25519 + p256", []string{"X25519", "P-256"}, []stdtls.CurveID{stdtls.X25519, stdtls.CurveP256}, ""},
		{"p384 + p521", []string{"P-384", "P-521"}, []stdtls.CurveID{stdtls.CurveP384, stdtls.CurveP521}, ""},
		{"unknown curve errors", []string{"FAKECURVE"}, nil, "unknown ecdh curve"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &stdtls.Config{}
			err := applyTLSParams(cfg, &tlsv3.TlsParameters{EcdhCurves: tc.input})
			if tc.errSub != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errSub) {
					t.Errorf("want %q, got: %v", tc.errSub, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cfg.CurvePreferences) != len(tc.want) {
				t.Errorf("got %d curves, want %d", len(cfg.CurvePreferences), len(tc.want))
			}
			for i := range tc.want {
				if cfg.CurvePreferences[i] != tc.want[i] {
					t.Errorf("curve[%d] = %d, want %d", i, cfg.CurvePreferences[i], tc.want[i])
				}
			}
		})
	}
}

func TestApplyTLSParams_SignatureAlgorithmsErrors(t *testing.T) {
	cfg := &stdtls.Config{}
	err := applyTLSParams(cfg, &tlsv3.TlsParameters{SignatureAlgorithms: []string{"rsa_pss_rsae_sha256"}})
	if err == nil || !strings.Contains(err.Error(), "signature_algorithms") {
		t.Errorf("want signature_algorithms error, got: %v", err)
	}
}

func TestApplyTLSParams_NilParams(t *testing.T) {
	cfg := &stdtls.Config{}
	if err := applyTLSParams(cfg, nil); err != nil {
		t.Errorf("nil params should be no-op, got: %v", err)
	}
}
