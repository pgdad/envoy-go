package boot

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// G4 — the phase-80 boot-boundary secret-name reject (T5), exercised across
// its POSITIVE, SEGMENT, NEGATIVE, FORK-ADJACENT and CHARSET arms.
//
// A guard that rejects everything passes a positive-only gate, so the negative
// and fork-adjacent arms are load-bearing, not decoration.
//
// Every expectation below is anchored on stats.IsValidName rather than on a
// hand-read of the name: two of these cells invert the obvious reading, and
// both were mis-stated by an earlier hand-written probe.
//
//   - "a..b" is a VALID bare stats name (the charset regex permits an
//     interior empty segment) yet the guard REJECTS it, because the guard
//     also enforces segment well-formedness.
//   - "1leading_digit" is an INVALID bare stats name (the regex forbids a
//     leading digit) yet the guard ACCEPTS it, because the assembled name is
//     "sds.1leading_digit.<suffix>" and the "sds." prefix supplies the
//     leading character while the fixed suffix supplies the trailing one.
//
// wantBareValid pins that surprising half so a future edit cannot quietly
// re-align the two columns.
func TestValidateSDSSecretName_Arms(t *testing.T) {
	cases := []struct {
		arm           string
		in            string
		wantReject    bool
		wantBareValid bool
	}{
		// POSITIVE: the reference accepts this and hoists the hyphen
		// verbatim into the label value; envoy-go boot-fails. Departure.
		{"positive", "server-cert", true, false},

		// SEGMENT: rejected by the empty-segment leg, not the charset leg.
		// "" is unreachable in production (xds.ParseSDSConfig rejects an
		// empty name first), so it is asserted against the predicate
		// directly -- an isolated unit test of a bare-name-only predicate
		// would certify an INCOMPLETE guard, since "" assembles to
		// "sds..init_fetch_timeout", which stats.IsValidName ACCEPTS.
		{"segment", "trailing_dot.", true, false},
		{"segment", ".lead", true, false},
		{"segment", "a..b", true, true},
		{"segment", "", true, false},

		// NEGATIVE: the four real corpus secret names. All must be ACCEPTED
		// -- this is what proves the reject is a no-op on the fixture corpus.
		{"negative", "server_cert", false, true},
		{"negative", "validation_ca", false, true},
		{"negative", "rccf_validation_ca", false, true},
		{"negative", "edf_validation_ca", false, true},

		// FORK-ADJACENT: dots inside a name are legal, and the projection
		// arm's first-dot fork is what gives them meaning. Accepting these
		// is deliberate.
		{"fork-adjacent", "my.dotted.cert", false, true},
		{"fork-adjacent", "1leading_digit", false, false},
		{"fork-adjacent", "UPPER", false, true},

		// CHARSET: rejected by stats.IsValidName, not by the segment leg.
		{"charset", "a/b", true, false},
	}

	for _, tc := range cases {
		err := validateSDSSecretName(tc.in)

		if tc.wantReject && err == nil {
			t.Errorf("[%s] validateSDSSecretName(%q) = nil, want a reject", tc.arm, tc.in)
		}
		if !tc.wantReject && err != nil {
			t.Errorf("[%s] validateSDSSecretName(%q) = %v, want accept", tc.arm, tc.in, err)
		}
		if tc.wantReject && err != nil {
			// Assert the STRING, never a byte volume: a volume check
			// passes on a hang and on the wrong message alike.
			want := "xds: sds: invalid secret name: " + strconv.Quote(tc.in)
			if !strings.Contains(err.Error(), want) {
				t.Errorf("[%s] validateSDSSecretName(%q) error = %q, want substring %q",
					tc.arm, tc.in, err.Error(), want)
			}
		}
		if got := stats.IsValidName(tc.in); got != tc.wantBareValid {
			t.Errorf("[%s] stats.IsValidName(%q) = %v, want %v (the bare-name column is pinned "+
				"because it does NOT track the guard's verdict)", tc.arm, tc.in, got, tc.wantBareValid)
		}
	}
}

// sdsAcceptedPrintableBytes is the exact set of printable-ASCII single-byte
// secret names that validateSDSSecretName ACCEPTS, in byte order. Derived from
// the guard rather than hand-typed, and re-derived by the sweep below on every
// run. internal/xds's skip-unreachability test pins the SAME literal against
// its own mirror of the predicate; the shared literal is what ties the two
// packages together across the import edge that forbids internal/xds from
// importing internal/boot.
const sdsAcceptedPrintableBytes = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz"

// TestValidateSDSSecretName_PrintableByteSweep is the predicate's own negative
// control: it proves the guard is NEITHER all-accept NOR all-reject over the
// 95 printable ASCII bytes, and pins exactly which bytes fall on each side.
func TestValidateSDSSecretName_PrintableByteSweep(t *testing.T) {
	var accepted, rejected []string
	denominator := 0
	for b := 32; b < 127; b++ {
		denominator++
		name := string(rune(b))
		if err := validateSDSSecretName(name); err == nil {
			accepted = append(accepted, name)
		} else {
			rejected = append(rejected, name)
		}
	}

	if denominator != 95 {
		t.Errorf("swept %d printable bytes, want 95", denominator)
	}
	if len(accepted) == 0 {
		t.Errorf("the guard accepted NOTHING over %d bytes: an all-reject guard passes a "+
			"positive-only gate", denominator)
	}
	if len(rejected) == 0 {
		t.Errorf("the guard rejected NOTHING over %d bytes: an all-accept guard is not a guard",
			denominator)
	}
	if got := strings.Join(accepted, ""); got != sdsAcceptedPrintableBytes {
		t.Errorf("accepted byte set = %q, want %q", got, sdsAcceptedPrintableBytes)
	}
	if got, want := len(accepted)+len(rejected), denominator; got != want {
		t.Errorf("accepted+rejected = %d, want %d", got, want)
	}
}

// TestValidateSDSSecretName_LongestSuffixSuffices re-verifies the property the
// guard's single-suffix check rests on: all five sds.<secret>.* suffixes are
// valid-or-invalid TOGETHER for every secret name. ADR-0065 Consequences (b)
// asserts the same rule for cluster names, but its stated reason ("the
// suffixes differ only in their last 4 chars") does not transfer to this
// suffix set, so the property is re-derived here rather than inherited.
func TestValidateSDSSecretName_LongestSuffixSuffices(t *testing.T) {
	agree := func(name string) bool {
		first := stats.IsValidName("sds." + name + "." + sdsStatSuffixes[0])
		for _, s := range sdsStatSuffixes[1:] {
			if stats.IsValidName("sds."+name+"."+s) != first {
				return false
			}
		}
		return true
	}

	single, singleBad := 0, 0
	for b := 32; b < 127; b++ {
		single++
		if !agree(string(rune(b))) {
			singleBad++
		}
	}
	double, doubleBad := 0, 0
	for a := 32; a < 127; a++ {
		for b := 32; b < 127; b++ {
			double++
			if !agree(string(rune(a)) + string(rune(b))) {
				doubleBad++
			}
		}
	}

	if single != 95 {
		t.Errorf("single-byte denominator = %d, want 95", single)
	}
	if double != 9025 {
		t.Errorf("two-byte denominator = %d, want 9025", double)
	}
	if singleBad != 0 {
		t.Errorf("single-byte suffix disagreements = %d, want 0", singleBad)
	}
	if doubleBad != 0 {
		t.Errorf("two-byte suffix disagreements = %d, want 0", doubleBad)
	}
	if got, want := sdsLongestStatSuffix(), "init_fetch_timeout"; got != want {
		t.Errorf("sdsLongestStatSuffix() = %q, want %q", got, want)
	}
}

// TestNewSDSProvider_SecretNameRejectPrecedesDial is the ORDERING assertion,
// and it is the reason T5 pins a line rather than a range.
//
// Both arms point at an SDS cluster that does not exist, so the dial is
// guaranteed to fail. The reject arm must nevertheless surface the NAME error:
// if the guard sat after grpcclient.NewSDSClient, the dial error would mask it
// and every observation of the reject would require a live SDS server.
//
// The second arm is the discriminating control -- a guard placed correctly but
// firing on everything would swallow the dial error too.
func TestNewSDSProvider_SecretNameRejectPrecedesDial(t *testing.T) {
	const unknownCluster = "no_such_sds_cluster"

	rejectYAML := fmt.Sprintf(sdsSecretNameYAMLTemplate, "server-cert", unknownCluster)
	bs, dialer := loadSDSBootstrapAndDialer(t, rejectYAML)
	_, err := NewSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
	if err == nil {
		t.Errorf("NewSDSProvider with an unprojectable secret name: got nil error, want a reject")
	} else {
		if want := `xds: sds: invalid secret name: "server-cert"`; !strings.Contains(err.Error(), want) {
			t.Errorf("NewSDSProvider error = %q, want substring %q", err.Error(), want)
		}
		if unwanted := "dial cluster"; strings.Contains(err.Error(), unwanted) {
			t.Errorf("NewSDSProvider error = %q, must NOT contain %q: the dial ran first and "+
				"MASKED the name reject", err.Error(), unwanted)
		}
	}

	dialYAML := fmt.Sprintf(sdsSecretNameYAMLTemplate, "server_cert", unknownCluster)
	bs2, dialer2 := loadSDSBootstrapAndDialer(t, dialYAML)
	_, err2 := NewSDSProvider(dialer2, bs2, t.TempDir(), bs2.Stats)
	if err2 == nil {
		t.Errorf("NewSDSProvider with a VALID secret name and an unknown SDS cluster: got nil "+
			"error, want the dial error (%q)", unknownCluster)
	} else if want := "dial cluster " + strconv.Quote(unknownCluster); !strings.Contains(err2.Error(), want) {
		t.Errorf("NewSDSProvider error = %q, want substring %q: a valid name must still reach "+
			"the dial", err2.Error(), want)
	}
}

// TestNewSDSProvider_ValidSecretNameStillBuilds is the READY control: the
// reject must not perturb the accepting path.
func TestNewSDSProvider_ValidSecretNameStillBuilds(t *testing.T) {
	yaml := fmt.Sprintf(sdsSecretNameYAMLTemplate, "server_cert", "sds_cluster")
	bs, dialer := loadSDSBootstrapAndDialer(t, yaml)
	provider, err := NewSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
	if err != nil {
		t.Errorf("NewSDSProvider with a valid secret name: got error %v, want nil", err)
	}
	if provider == nil {
		t.Errorf("NewSDSProvider with a valid secret name: got nil provider, want non-nil")
	}
}

// sdsSecretNameYAMLTemplate takes (secretName, sdsClusterName). The sds_cluster
// endpoint is never connected to by these tests -- NewSDSClient resolves the
// cluster without dialing a socket -- so the port only has to be one no other
// test binds.
const sdsSecretNameYAMLTemplate = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tls
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
          transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                tls_certificate_sds_secret_configs:
                  - name: %s
                    sds_config: {resource_api_version: V3, api_config_source: {api_type: GRPC, transport_api_version: V3, grpc_services: [{envoy_grpc: {cluster_name: %s}}]}}
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
    - name: sds_cluster
      type: STATIC
      connect_timeout: 1s
      typed_extension_protocol_options:
        envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
          "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
          explicit_http_config:
            http2_protocol_options: {}
      load_assignment:
        cluster_name: sds_cluster
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 42301 }
`
