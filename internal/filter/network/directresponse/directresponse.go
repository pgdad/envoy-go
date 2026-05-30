package directresponse

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	drv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/direct_response/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
)

// TypeURL is the typed_config Any type URL for the direct_response network
// filter (note: the message is .Config, not .DirectResponse).
const TypeURL = "type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config"

// responseCodeDetails is the internal RCD string set on write (parity with
// upstream; no operator-visible surface per SPEC §2.10 / D-P26.1-5b).
const responseCodeDetails = "DirectResponse"

// PARSE-REJECT wording (§6.1). Byte-stable; pinned in
// TestParseRejectConstants_ByteStable (Task 9).
const (
	parseRejectResponseSpecifierRequired = "direct_response: response.specifier is required"
	parseRejectFilenameRead              = "direct_response: response.filename: %s: %w"
	parseRejectEnvVarUnset               = "direct_response: response.environment_variable: %s: unset"
)

// compiledConfig holds the boot-resolved response body shared by every
// per-connection filter instance (per-config validation cost paid once).
type compiledConfig struct{ body []byte }

// New parses+validates the direct_response typed_config once at boot and
// returns a per-connection FilterInstanceFactory. Mirrors the ADR-0079
// two-step factory shape (NetworkFilterFactory signature).
func New(tc *anypb.Any, ctx network.FactoryCtx) (network.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("direct_response: invalid typed_config: nil")
	}
	cfg := &drv3.Config{}
	if err := tc.UnmarshalTo(cfg); err != nil {
		return nil, fmt.Errorf("direct_response: invalid typed_config: %w", err)
	}
	body, err := resolveDataSource(cfg.GetResponse(), ctx.BaseDir)
	if err != nil {
		return nil, err
	}
	cc := &compiledConfig{body: body}
	return func() network.ReadFilter { return &filter{cfg: cc} }, nil
}

// resolveDataSource mirrors internal/tls/datasource.go:loadDataSource (baseDir-
// relative Filename), with the 4 specifier arms (D-P26.1-2). An absent/unset
// specifier is the boot-reject parity arm (§6.1; D-P26.1-3).
func resolveDataSource(ds *corev3.DataSource, baseDir string) ([]byte, error) {
	if ds == nil || ds.GetSpecifier() == nil {
		return nil, errors.New(parseRejectResponseSpecifierRequired)
	}
	switch s := ds.GetSpecifier().(type) {
	case *corev3.DataSource_InlineString:
		return []byte(s.InlineString), nil
	case *corev3.DataSource_InlineBytes:
		return s.InlineBytes, nil
	case *corev3.DataSource_Filename:
		p := s.Filename
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf(parseRejectFilenameRead, p, err)
		}
		return b, nil
	case *corev3.DataSource_EnvironmentVariable:
		v, ok := os.LookupEnv(s.EnvironmentVariable)
		if !ok {
			return nil, fmt.Errorf(parseRejectEnvVarUnset, s.EnvironmentVariable)
		}
		return []byte(v), nil
	default:
		// forward-guard for a future 5th DataSource oneof variant (unreachable today; the 4 arms are exhaustive and the nil specifier is caught above).
		return nil, errors.New(parseRejectResponseSpecifierRequired)
	}
}

// filter is the per-connection direct_response read-filter instance.
type filter struct {
	cfg *compiledConfig
	cb  network.ReadFilterCallbacks
}

// OnNewConnection writes the fixed response body with end_stream, sets the RCD
// on the per-connection callbacks (optional-interface sink, satisfied by the
// concrete callbacks per Task 6's TestResponseCodeDetailsSink), then closes the
// connection with FlushWrite and halts the chain.
func (f *filter) OnNewConnection() network.Status {
	f.cb.Connection().Write(f.cfg.body, true)
	if s, ok := f.cb.(interface{ SetResponseCodeDetails(string) }); ok {
		s.SetResponseCodeDetails(responseCodeDetails)
	}
	f.cb.Connection().Close(network.FlushWrite)
	return network.StopIteration
}

// OnData halts the chain: direct_response has already responded+closed at
// OnNewConnection, so any subsequent bytes are dropped.
func (f *filter) OnData(*network.Buffer, bool) network.Status { return network.StopIteration }

// SetReadFilterCallbacks stores the per-connection callbacks.
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks) { f.cb = cb }

// OnDestroy releases per-connection resources (none held).
func (f *filter) OnDestroy() {}
