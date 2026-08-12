// Package boot builds the shared "construct everything, bind nothing" tail
// of envoy-go's boot sequence: the three filter-type registries (HTTP,
// listener, network) and the listener manager. Both cmd/envoy-go/main.go's
// normal boot path and the public github.com/pgdad/envoy-go/validate
// package call Construct, so the two can never silently diverge on what
// "valid" means (phase 51, ADR-0268).
package boot

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/bootstrap"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/drain"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	httpbuiltins "github.com/pgdad/envoy-go/internal/filter/http/builtins"
	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/filter/network/builtins"
	"github.com/pgdad/envoy-go/internal/grpcclient"
	"github.com/pgdad/envoy-go/internal/httpclient"
	"github.com/pgdad/envoy-go/internal/listener"
	"github.com/pgdad/envoy-go/internal/listener/listenerfilter"
	"github.com/pgdad/envoy-go/internal/listener/listenerfilter/tls_inspector"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/tracing"
	"github.com/pgdad/envoy-go/internal/xds"
	xdsgrpc "github.com/pgdad/envoy-go/internal/xds/xdsgrpc"
)

// Construct builds the three filter-type registries (HTTP, listener,
// network) and the listener manager for bs, exactly as
// cmd/envoy-go/main.go's normal boot path does — EXCEPT it starts no
// background goroutine and binds no listener socket (both happen later, in
// a separate lm.Start call the caller makes itself). Both main.go's normal
// boot path and the public validate package call this SAME function for
// the registry-and-listener-manager tail of the boot sequence, so the two
// can never silently diverge on what "valid" means.
//
// cm, sinks, dm, httpClient, and tracingProvider are all supplied by the
// caller rather than built here: cm must already exist before a
// grpcclient.Dialer (needed to build sinks and tracingProvider) can be
// built, so Construct cannot own cm construction while also accepting
// sinks/tracingProvider as inputs. The real boot path passes its real,
// already-constructed instances, so the returned *listener.Manager shares
// them with whatever admin/lm.Start/shutdown-drain logic runs afterward.
// The validate package passes throwaway instances (a fresh, never-Frozen
// cm, nil sinks, a throwaway drain.Manager/httpclient.Client/
// tracing.ExporterProvider) and discards the returned *listener.Manager,
// keeping only the error.
func Construct(
	bs *bootstrap.Bootstrap,
	cm *cluster.Manager,
	baseDir string,
	allowH2C bool,
	sinks []accesslog.Sink,
	dm *drain.Manager,
	httpClient *httpclient.Client,
	tracingProvider *tracing.ExporterProvider,
	sdsProvider xds.SecretProvider,
) (*listener.Manager, error) {
	httpReg := filter_http.NewHTTPRegistry()
	httpbuiltins.RegisterBuiltins(httpReg)
	httpReg.Freeze()

	lfReg := listenerfilter.NewListenerFilterRegistry()
	lfReg.Register(tls_inspector.TypeURL, tls_inspector.New)
	lfReg.Freeze()

	netReg := network.NewRegistry()
	builtins.RegisterBuiltins(netReg, builtins.Deps{
		ClusterManager:   cm,
		StatsRegistry:    bs.Stats,
		AccessLogSinks:   sinks,
		HTTPRegistry:     httpReg,
		DrainManager:     dm,
		HTTPClient:       httpClient,
		TracingExporters: tracingProvider,
	})
	netReg.Freeze()

	lm, err := listener.NewManagerWithBaseDirAndAllowH2C(
		bs.Proto, cm, baseDir, allowH2C, bs.Stats, sinks, httpReg, lfReg, dm, httpClient, netReg, sdsProvider,
	)
	if err != nil {
		return nil, maybeWrapLuaScriptLoadError(err)
	}
	return lm, nil
}

// NewTracingProvider builds a tracing.ExporterProvider using the standard
// boot-time buffer defaults (16384 bytes / 1s flush) via the
// tracesDialerAdapter/zipkinTransportAdapter bridge. Both the real boot path
// and validate call this so neither duplicates the two adapter types.
func NewTracingProvider(dialer *grpcclient.Dialer, httpClient *httpclient.Client, cm *cluster.Manager, registry *stats.Registry) *tracing.ExporterProvider {
	return tracing.NewExporterProvider(tracesDialerAdapter{dialer}, zipkinTransportAdapter{httpClient, cm}, registry, 16384, time.Second)
}

// newSDSProviderAndClient pre-scans bs for the (single, MVP) downstream
// SDS-bound TLS context and, when present, builds the blocking
// xds.SecretProvider that internal/tls consults at listener construction
// (phase 60.2, ADR-0280), ALONG WITH the *grpcclient.SDSClient the provider's
// opener wraps. Both NewSDSProvider (the ordinary boot path, which keeps the
// client open for the provider's lifetime) and NewValidateSDSProvider
// (--mode validate, which closes the never-dialed client and substitutes the
// no-fetch sentinel) share this ENTIRE pre-scan, so every present and future
// pre-scan arm is inherited by construction — parity by CODE-PATH, not by
// re-derivation (phase 86, ADR-0308).
//
// Returns (nil, nil, nil) when no SDS cert is configured anywhere (the nil
// provider threads harmlessly — the tls lift only engages when a listener
// actually carries tls_certificate_sds_secret_configs).
//
// Enforces the NODE boot-requirement (arm 7, SPEC §6/§11 Arm A-pre): SDS
// configured while bootstrap node.id/node.cluster are empty ⇒ boot-fail. The
// cluster/secret/timeout are extracted via xds.ParseSDSConfig (arms 1-4,8,9);
// the *grpcclient.SDSClient edge is carried by xdsgrpc.NewOpener so internal/xds
// stays grpcclient-free (the ADR-0278 cycle guard).
func newSDSProviderAndClient(dialer *grpcclient.Dialer, bs *bootstrap.Bootstrap, baseDir string, registry *stats.Registry) (xds.SecretProvider, *grpcclient.SDSClient, error) {
	dtcTypeURL := "type.googleapis.com/" + string(proto.MessageName(&tlsv3.DownstreamTlsContext{}))
	var found []*tlsv3.SdsSecretConfig
	seen := 0
	for _, l := range bs.Proto.GetStaticResources().GetListeners() {
		chains := append([]*listenerv3.FilterChain{}, l.GetFilterChains()...)
		if dfc := l.GetDefaultFilterChain(); dfc != nil {
			chains = append(chains, dfc)
		}
		for _, fc := range chains {
			ts := fc.GetTransportSocket()
			if ts == nil || ts.GetTypedConfig().GetTypeUrl() != dtcTypeURL {
				continue
			}
			var dtc tlsv3.DownstreamTlsContext
			if err := ts.GetTypedConfig().UnmarshalTo(&dtc); err != nil {
				continue // a malformed transport_socket surfaces at the listener build, not here
			}
			ctc := dtc.GetCommonTlsContext()
			if sc := ctc.GetTlsCertificateSdsSecretConfigs(); len(sc) > 0 {
				seen++
				found = sc
			}
			// Phase 65 (ADR-0286): also detect an SDS-bound validation_context, so a
			// listener using SDS ONLY for the downstream mTLS trusted-CA (with a
			// static server cert) builds a provider. Without this the pre-scan
			// returns (nil, nil) and the tls apply-point's nil-provider reject fires.
			//
			// seen++ on BOTH arms is deliberate: a context using SDS for the server
			// cert AND the validation_context trips the seen>1 guard below — the
			// DEFERRED compose-two edge (the single-slot provider model holds ONE
			// secretName and ONE *SDSStats; SPEC-65 §2/§3.4).
			if vsc, ok := ctc.GetValidationContextType().(*tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig); ok {
				seen++
				found = []*tlsv3.SdsSecretConfig{vsc.ValidationContextSdsSecretConfig}
			}
			// Phase 66 (D6): also detect an SDS-bound combined_validation_context
			// (CVC) — the counterpart to the arm above, so the ordinary
			// single-listener CVC case (no static server cert, no cert-SDS) builds a
			// provider too. Without this the pre-scan returns (nil, nil) and
			// commonTLSContextToConfig's retained phase-03 reject fires — a loud
			// boot-FAIL with the misleading-but-safe "tls: downstream:
			// combined_validation_context is not supported in phase 03" message —
			// even though the SDS-delivered CA pool is right there.
			//
			// E2's condition, honored here too (D2): a pure-inline CVC contributes NO
			// secret. Skipping (rather than counting) keeps seen==0 for the
			// single-listener case, so the retained reject fires in tls/config.go
			// rather than ParseSDSConfig tripping on a nil entry.
			//
			// seen++ here composes with the arms above exactly as phase 65 already
			// documented: a context using SDS for the server cert or the plain
			// validation_context AND the CVC's SDS half trips the seen>1 guard below
			// — the DEFERRED compose-two edge stays shut for the CVC shape too.
			if cvc, ok := ctc.GetValidationContextType().(*tlsv3.CommonTlsContext_CombinedValidationContext); ok {
				if sc := cvc.CombinedValidationContext.GetValidationContextSdsSecretConfig(); sc != nil {
					seen++
					found = []*tlsv3.SdsSecretConfig{sc}
				}
			}
		}
	}
	if seen == 0 {
		return nil, nil, nil
	}
	if seen > 1 {
		return nil, nil, fmt.Errorf("xds: sds: multiple SDS-bound downstream TLS contexts unsupported (MVP takes one)")
	}
	node := bs.Proto.GetNode()
	if node.GetId() == "" || node.GetCluster() == "" {
		return nil, nil, fmt.Errorf("xds: sds: node.id and node.cluster are required for SDS")
	}
	secretName, clusterName, timeout, err := xds.ParseSDSConfig(found)
	if err != nil {
		return nil, nil, err
	}
	// The reject MUST precede the dial. Placed after NewSDSClient, an
	// unreachable-cluster or absent-SDS-server config fails at the dial
	// first and MASKS this error, which would force every observation of
	// the name reject to stand up a live SDS server (phase 80, T5).
	if err := validateSDSSecretName(secretName); err != nil {
		return nil, nil, err
	}
	client, err := grpcclient.NewSDSClient(dialer, clusterName)
	if err != nil {
		return nil, nil, fmt.Errorf("xds: sds: dial cluster %q: %w", clusterName, err)
	}
	opener := xdsgrpc.NewOpener(client)
	sdsStats := xds.RegisterSDSStats(registry, secretName)
	return xds.NewProvider(opener, xds.Node{ID: node.GetId(), Cluster: node.GetCluster()}, baseDir, timeout, sdsStats), client, nil
}

// NewSDSProvider pre-scans bs for the (single, MVP) downstream SDS-bound TLS
// context and, when present, builds the blocking xds.SecretProvider that
// internal/tls consults at listener construction (phase 60.2, ADR-0280). It
// mirrors NewTracingProvider: built once at boot from the shared dialer, then
// threaded through Construct into the listener build. Returns (nil, nil) when no
// SDS cert is configured (the nil provider threads harmlessly — the tls lift
// only engages when a listener actually carries tls_certificate_sds_secret_configs).
func NewSDSProvider(dialer *grpcclient.Dialer, bs *bootstrap.Bootstrap, baseDir string, registry *stats.Registry) (xds.SecretProvider, error) {
	p, _, err := newSDSProviderAndClient(dialer, bs, baseDir, registry)
	return p, err // the client stays open — the boot path's provider owns it via the opener
}

// NewValidateSDSProvider runs the ENTIRE boot pre-scan (parity by CODE-PATH:
// every present and future pre-scan arm is inherited by construction), then
// CLOSES the never-dialed lazy client (D-86-CONN: validate is a library
// consumed by long-running Gateway controllers; leaking one lazy
// *grpc.ClientConn per reconcile call is not acceptable hygiene) and returns
// the no-fetch sentinel. Returns (nil, nil) at seen==0 exactly like the boot
// path — the nil provider threads harmlessly. Every reject returned here is a
// BOOT-PARITY reject: byte-identical to what NewSDSProvider returns for the
// same bootstrap (phase 86, ADR-0308).
func NewValidateSDSProvider(dialer *grpcclient.Dialer, bs *bootstrap.Bootstrap, baseDir string, registry *stats.Registry) (xds.SecretProvider, error) {
	p, client, err := newSDSProviderAndClient(dialer, bs, baseDir, registry)
	if err != nil {
		return nil, err // these ARE the boot-parity rejects
	}
	if p == nil {
		return nil, nil // no SDS anywhere: the nil provider threads harmlessly
	}
	_ = client.Close() // idempotent + nil-safe (grpcclient_test.go:1911/:1982)
	return xds.NoFetchProvider(), nil
}

// sdsStatSuffixes is the five-counter sds.<secret>.* suffix set that
// xds.RegisterSDSStats composes. Kept here only so sdsLongestStatSuffix (and
// the tests that sweep it) can derive the assembled name that
// validateSDSSecretName checks, rather than hard-coding one.
var sdsStatSuffixes = []string{
	"update_success",
	"update_failure",
	"update_rejected",
	"update_attempt",
	"init_fetch_timeout",
}

// sdsLongestStatSuffix returns the longest member of sdsStatSuffixes.
// Validating the assembled name built from the LONGEST suffix suffices: all
// five suffixes agree (valid or invalid together) for every secret name --
// re-verified exhaustively at phase 80 over 95 single-byte and 9025 two-byte
// names, zero disagreements. ADR-0065 Consequences (b) states the same rule
// for cluster names, but its stated REASON there ("the suffixes differ only
// in their last 4 chars") does NOT transfer to this suffix set, which differs
// in both length and prefix -- hence the independent re-verification.
func sdsLongestStatSuffix() string {
	longest := ""
	for _, s := range sdsStatSuffixes {
		if len(s) > len(longest) {
			longest = s
		}
	}
	return longest
}

// invalidSecretNameErrFmt is the byte-stable boot-reject wording for an SDS
// secret name that cannot form a clean metric name.
const invalidSecretNameErrFmt = "xds: sds: invalid secret name: %q " +
	"(must contain only ASCII letters, digits, underscore, or dot, " +
	"and form a valid metric-name segment)"

// validateSDSSecretName rejects an operator-supplied SDS secret name that
// cannot form a clean metric name, at the boot boundary and BEFORE the SDS
// dial. ADR-0065 Consequences (e) mandates boundary validation for metric
// names derived from user input.
//
// Two legs, both applied to the ASSEMBLED name rather than to the bare secret
// name:
//
//  1. Segment well-formedness. stats.IsValidName is a CHARSET guard only and
//     ACCEPTS an interior empty segment, so a bare-name-only check passes ""
//     and "trailing_dot.": they assemble to sds..init_fetch_timeout and
//     sds.trailing_dot..init_fetch_timeout, both of which register cleanly and
//     would then project to envoy_sds__update_success where the reference
//     serves envoy_sds_update_success with the name hoisted into a label.
//  2. The stats charset, via stats.IsValidName on the same assembled name.
//
// Sanitizing is FORECLOSED by ADR-0065's own Context: two names differing only
// in invalid characters would collapse to a single label value, a silent
// data-loss bug. So this rejects rather than rewrites.
//
// DEPARTURE, NOT A FIX (phase 80). The reference ACCEPTS a hyphenated secret
// name such as "server-cert": it boots green and serves
// envoy_sds_update_success{envoy_xds_resource_name="server-cert"} 1 with the
// hyphen verbatim in the label value. The same config boot-FAILS here. That is
// an envoy-go-strict departure recorded deliberately, not a defect being
// repaired.
//
// The xds.RegisterSDSStats skip branch and the nil-safe increment helpers both
// STAY: two-layer defense is the established pattern here, and deleting the
// nil-safe path is a proven boot-path SIGSEGV (a nil *stats.Counter Inc is a
// process crash with no recover()).
func validateSDSSecretName(name string) error {
	assembled := "sds." + name + "." + sdsLongestStatSuffix()
	for _, seg := range strings.Split(assembled, ".") {
		if seg == "" {
			return fmt.Errorf(invalidSecretNameErrFmt, name)
		}
	}
	if !stats.IsValidName(assembled) {
		return fmt.Errorf(invalidSecretNameErrFmt, name)
	}
	return nil
}

// luaCompileErrorSubstring is the byte-stable arm-16 wrap prefix emitted by
// internal/filter/http/lua/compiled_config.go::wrapParseRejectScriptCompileFailed
// (`"lua: default_source_code: compile: %w"`). Detecting this substring lets
// the boot-reject sink identify a Lua script-compile failure that surfaced
// through the HCM filter-factory error chain
// (`listener: %q: filter_chains[%d]: hcm: http_filters[%d]: factory: <inner>`).
// Phase 22.1 Task 15 + parent §13-W + 22.1 SPEC §6 Task 15.
const luaCompileErrorSubstring = "lua: default_source_code: compile:"

// scriptLoadErrorWrapPrefix is the literal wording prefix the upstream
// Envoy v1.37.2 lua filter prints to stderr on script-compile failure per
// `source/extensions/filters/common/lua/lua.cc` (parent §11.7.5). The
// envoy-go-side boot-reject path wraps the surfaced error with this prefix
// so the cross-side substring assertion at fixture-0026 scenario (g) (per
// AMEND-10 option 2 + parent §13-R1) lands on both proxies.
//
// The trailing colon + space match upstream's wording byte-exactly; the
// substring assertion in test/differential/runner_test.go::runBootRejectFixture
// only checks for `"script load error"` (no colon) per AMEND-10 wording
// discipline — the colon is preserved here for upstream-parity readability
// of operator-facing stderr.
const scriptLoadErrorWrapPrefix = "script load error: "

// tracesDialerAdapter bridges *grpcclient.Dialer to the unexported
// tracing.tracesClientDialer interface (single method NewTracesClient). It
// allows callers to pass the shared grpcclient.Dialer into
// tracing.NewExporterProvider without creating an import cycle (tracing
// never imports grpcclient; the structural match is verified by the compiler
// at the NewExporterProvider call site). Phase 46.1b (ADR-0260).
type tracesDialerAdapter struct{ d *grpcclient.Dialer }

func (a tracesDialerAdapter) NewTracesClient(clusterName string) (tracing.TracesClient, error) {
	return grpcclient.NewOTLPTracesClient(a.d, clusterName)
}

// zipkinTransportAdapter bridges the shared *httpclient.Client + *cluster.Manager
// to the tracing.ZipkinTransport seam (HasCluster/Dispatch). It lets callers pass
// the HTTP cluster-dispatch transport into tracing.NewExporterProvider without
// internal/tracing importing internal/httpclient or internal/cluster (no import
// cycle): HasCluster gates the boot-time collector-cluster existence check and
// Dispatch binds the cluster manager into (*httpclient.Client).ClusterDispatch
// for the Zipkin arm's v2-JSON POSTs. Phase 46.2 (D-TRACE-ZIPKIN-TRANSPORT-WIRING).
type zipkinTransportAdapter struct {
	c  *httpclient.Client
	cm *cluster.Manager
}

func (a zipkinTransportAdapter) HasCluster(name string) bool { _, ok := a.cm.Get(name); return ok }

func (a zipkinTransportAdapter) Dispatch(ctx context.Context, clusterName string, req *http.Request) (*http.Response, error) {
	return a.c.ClusterDispatch(ctx, clusterName, req, a.cm)
}

var _ tracing.ZipkinTransport = zipkinTransportAdapter{}

// maybeWrapLuaScriptLoadError inspects the supplied error for the arm-16
// Lua compile-failure substring (the byte-stable wrap emitted by the lua
// filter's `buildCompiledConfig` per `compiled_config.go::wrapParseReject
// ScriptCompileFailed`). When matched, returns a new error wrapping the
// original with the upstream-parity prefix `"script load error: "` per
// parent §11.7.5 + §13-W + 22.1 SPEC §6 Task 15.
//
// When the substring is NOT present (i.e., the listener-manager error is
// from a NON-lua filter / a non-compile lua failure / a NON-filter source
// such as bind / cluster construction), the original error is returned
// unchanged — the wrap is filter-scoped, not generic.
//
// The match is a `strings.Contains` (case-sensitive) against the error's
// `Error()` string rather than `errors.As` against a typed sentinel
// because the arm-16 wrap is a string-format chain — the wrapped inner is
// a `*lua.ApiError` value but the prefix lives in the wrap layer, not in
// a typed wrapper. The substring is the contract per parent §6.1
// byte-stable PARSE-REJECT wording discipline.
func maybeWrapLuaScriptLoadError(err error) error {
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), luaCompileErrorSubstring) {
		return err
	}
	return fmt.Errorf("%s%w", scriptLoadErrorWrapPrefix, err)
}
