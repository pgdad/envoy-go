package http

import (
	"net/http"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/stats"
)

func TestFilterHeadersStatus_Values(t *testing.T) {
	if Continue == StopIteration {
		t.Fatalf("Continue and StopIteration must differ")
	}
	if int(Continue) != 0 || int(StopIteration) != 1 {
		t.Fatalf("expected Continue=0, StopIteration=1; got Continue=%d, StopIteration=%d", Continue, StopIteration)
	}
}

func TestFilterDataStatus_Values(t *testing.T) {
	if int(DataContinue) != 0 || int(DataStopIterationAndBuffer) != 1 || int(DataStopIterationNoBuffer) != 2 {
		t.Fatalf("expected DataContinue=0, DataStopIterationAndBuffer=1, DataStopIterationNoBuffer=2; got %d/%d/%d", DataContinue, DataStopIterationAndBuffer, DataStopIterationNoBuffer)
	}
}

func TestFilterTrailersStatus_Values(t *testing.T) {
	if int(TrailersContinue) != 0 || int(TrailersStopIteration) != 1 {
		t.Fatalf("expected TrailersContinue=0, TrailersStopIteration=1; got %d/%d", TrailersContinue, TrailersStopIteration)
	}
}

// Compile-only assertion: a test-only fake filter implements both decoder and
// encoder interfaces with the expected method set. If any method is renamed,
// this test fails to compile.
type fakeFilter struct{}

func (fakeFilter) DecodeHeaders(http.Header, bool) FilterHeadersStatus { return Continue }
func (fakeFilter) DecodeData([]byte, bool) FilterDataStatus            { return DataContinue }
func (fakeFilter) DecodeTrailers(http.Header) FilterTrailersStatus     { return TrailersContinue }
func (fakeFilter) SetDecoderCallbacks(DecoderFilterCallbacks)          {}
func (fakeFilter) EncodeHeaders(http.Header, bool) FilterHeadersStatus { return Continue }
func (fakeFilter) EncodeData([]byte, bool) FilterDataStatus            { return DataContinue }
func (fakeFilter) EncodeTrailers(http.Header) FilterTrailersStatus     { return TrailersContinue }
func (fakeFilter) SetEncoderCallbacks(EncoderFilterCallbacks)          {}
func (fakeFilter) OnDestroy()                                          {}

func TestFilterInterfaces_Compile(t *testing.T) {
	var _ StreamDecoderFilter = fakeFilter{}
	var _ StreamEncoderFilter = fakeFilter{}
}

// TestFactoryCtx_StatsRegistryThreaded verifies the FactoryCtx Stats +
// StatPrefix fields are propagated through the HTTPFilterFactory call to the
// per-filter factory body. Per Phase 09 Task 2 (FactoryCtx framework
// extension; ADR-0100 first-use anchor): fault's New factory needs the
// *stats.Registry + stat_prefix at HCM-build time to register its 5 stat
// names per ADR-0061's pre-Freeze discipline.
func TestFactoryCtx_StatsRegistryThreaded(t *testing.T) {
	reg := stats.NewRegistry()
	var capturedStats *stats.Registry
	var capturedPrefix string
	f := HTTPFilterFactory(func(_ *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error) {
		capturedStats = ctx.Stats
		capturedPrefix = ctx.StatPrefix
		return func() HTTPFilter { return HTTPFilter{Name: "test"} }, nil
	})
	_, err := f(nil, FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if capturedStats != reg {
		t.Errorf("Stats: got %p, want %p", capturedStats, reg)
	}
	if capturedPrefix != "ingress_http" {
		t.Errorf("StatPrefix: got %q, want %q", capturedPrefix, "ingress_http")
	}
}

// TestFactoryCtx_NilStatsRegistryTolerated verifies factories that observe a
// zero-value FactoryCtx (Stats == nil, StatPrefix == "") do not crash. Per
// ADR-0085 nil-tolerance pattern: existing 07.1 filter factories (router,
// cors, envoygotest) ignore the new fields gracefully; tests that exercise
// non-stat-bearing filters may continue to pass FactoryCtx{} or
// FactoryCtx{Registry: r}.
func TestFactoryCtx_NilStatsRegistryTolerated(t *testing.T) {
	f := HTTPFilterFactory(func(_ *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error) {
		if ctx.Stats != nil {
			t.Errorf("expected nil Stats, got %p", ctx.Stats)
		}
		if ctx.StatPrefix != "" {
			t.Errorf("expected empty StatPrefix, got %q", ctx.StatPrefix)
		}
		return func() HTTPFilter { return HTTPFilter{Name: "test"} }, nil
	})
	_, err := f(nil, FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
}
