package rbac

import (
	"net"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	configrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	networkrbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	"google.golang.org/protobuf/types/known/anypb"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

// findCounter scans the Registry for a counter with the given name (reuses the
// Walk-based lookup shape from internal/rbac/perpolicy_test.go). Returns nil if
// not present.
func findCounter(reg *stats.Registry, name string) *stats.Counter {
	var found *stats.Counter
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			if c, ok := m.(*stats.Counter); ok {
				found = c
			}
		}
	})
	return found
}

func TestTypeURLViaProtoMessageName(t *testing.T) {
	// memory reference_network_filter_typeurl_extensions: derive, do not hand-type.
	if TypeURL != "type.googleapis.com/envoy.extensions.filters.network.rbac.v3.RBAC" {
		t.Fatalf("TypeURL = %q", TypeURL)
	}
}

func mustAny(t *testing.T, m *networkrbacv3.RBAC) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestNew_StatPrefixRequired(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	_, err := factory(mustAny(t, &networkrbacv3.RBAC{ /* StatPrefix empty */ }), network.FactoryCtx{})
	if err == nil {
		t.Fatal("empty stat_prefix must PARSE-REJECT")
	}
}

func TestNew_DelayDenyRejected(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	cfg := &networkrbacv3.RBAC{StatPrefix: "p", DelayDeny: durationpb.New(0)}
	if _, err := factory(mustAny(t, cfg), network.FactoryCtx{}); err == nil {
		t.Fatal("delay_deny must PARSE-REJECT (AMEND-A9)")
	}
}

func TestNew_HTTPOnlyArmRejectedViaProfileL4(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	cfg := &networkrbacv3.RBAC{
		StatPrefix: "p",
		Rules: &configrbacv3.RBAC{
			Action: configrbacv3.RBAC_ALLOW,
			Policies: map[string]*configrbacv3.Policy{
				"x": {
					Permissions: []*configrbacv3.Permission{{Rule: &configrbacv3.Permission_Header{}}},
					Principals:  []*configrbacv3.Principal{{Identifier: &configrbacv3.Principal_Any{Any: true}}},
				},
			},
		},
	}
	if _, err := factory(mustAny(t, cfg), network.FactoryCtx{}); err == nil {
		t.Fatal("permission.header must reject for L4 (ProfileL4)")
	}
}

func TestNew_FourStaticCountersRegistered(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	cfg := &networkrbacv3.RBAC{StatPrefix: "lis"}
	if _, err := factory(mustAny(t, cfg), network.FactoryCtx{}); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for _, name := range []string{"lis.rbac.allowed", "lis.rbac.denied", "lis.rbac.shadow_allowed", "lis.rbac.shadow_denied"} {
		if findCounter(reg, name) == nil {
			t.Errorf("static counter %q not registered (predeclared-empty for scrape stability)", name)
		}
	}
}

// TestProjectStatCount_RBACNetwork26_3 pins the per-package metric surface to
// EXACTLY four static counters per rbac_network chain (allowed + denied +
// shadow_allowed + shadow_denied). There is NO per-policy counter family (F2 —
// the network consumer never constructs PerPolicyCounters). A change to this
// count is a deliberate surface change and must update this pin.
func TestProjectStatCount_RBACNetwork26_3(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	if _, err := factory(mustAny(t, &networkrbacv3.RBAC{StatPrefix: "lis"}), network.FactoryCtx{}); err != nil {
		t.Fatal(err)
	}
	const want = 4 // allowed + denied + shadow_allowed + shadow_denied (no per-policy — F2)
	got := 0
	reg.Walk(func(stats.Metric) { got++ })
	if got != want {
		t.Errorf("rbac_network stat-count = %d; want %d (4 static; NO per-policy — F2)", got, want)
	}
}

func TestNew_ShadowRulesStatPrefixSegment(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	cfg := &networkrbacv3.RBAC{StatPrefix: "lis", ShadowRulesStatPrefix: "sh"}
	if _, err := factory(mustAny(t, cfg), network.FactoryCtx{}); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	// Enforced counters unaffected by shadow_rules_stat_prefix (SPEC §7.1).
	for _, name := range []string{"lis.rbac.allowed", "lis.rbac.denied"} {
		if findCounter(reg, name) == nil {
			t.Errorf("enforced counter %q not registered", name)
		}
	}
	// shadow_rules_stat_prefix inserts a segment between `rbac.` and the two
	// shadow_* counters ONLY.
	for _, name := range []string{"lis.rbac.sh.shadow_allowed", "lis.rbac.sh.shadow_denied"} {
		if findCounter(reg, name) == nil {
			t.Errorf("shadow counter %q not registered with shadow_rules_stat_prefix segment", name)
		}
	}
	// The non-segmented shadow names must NOT exist when ShadowRulesStatPrefix is set.
	for _, name := range []string{"lis.rbac.shadow_allowed", "lis.rbac.shadow_denied"} {
		if findCounter(reg, name) != nil {
			t.Errorf("shadow counter %q must carry the shadow_rules_stat_prefix segment", name)
		}
	}
}

func TestNew_NilTypedConfig(t *testing.T) {
	factory := NewFactory(stats.NewRegistry())
	if _, err := factory(nil, network.FactoryCtx{}); err == nil {
		t.Fatal("nil typed_config must PARSE-REJECT")
	}
}

func TestNew_NilRegistryTolerance(t *testing.T) {
	// ADR-0085 nil-tolerance: NewFactory(nil) must succeed on a valid config
	// and must not panic. The guard `if reg != nil` in the factory means no
	// counters are registered, but the FilterInstanceFactory must still be
	// returned without error.
	factory := NewFactory(nil)
	cfg := &networkrbacv3.RBAC{StatPrefix: "p"}
	fif, err := factory(mustAny(t, cfg), network.FactoryCtx{})
	if err != nil {
		t.Fatalf("nil registry must not prevent a valid-config parse: %v", err)
	}
	if fif == nil {
		t.Fatal("nil registry parse returned nil FilterInstanceFactory")
	}
}

func TestNew_ShadowSideHTTPOnlyArmRejectedViaProfileL4(t *testing.T) {
	// M1: mirror TestNew_HTTPOnlyArmRejectedViaProfileL4 but place the
	// HTTP-only arm in ShadowRules. The shadow engine build path must also
	// ProfileL4-reject permission.header.
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	cfg := &networkrbacv3.RBAC{
		StatPrefix: "p",
		ShadowRules: &configrbacv3.RBAC{
			Action: configrbacv3.RBAC_ALLOW,
			Policies: map[string]*configrbacv3.Policy{
				"x": {
					Permissions: []*configrbacv3.Permission{{Rule: &configrbacv3.Permission_Header{}}},
					Principals:  []*configrbacv3.Principal{{Identifier: &configrbacv3.Principal_Any{Any: true}}},
				},
			},
		},
	}
	if _, err := factory(mustAny(t, cfg), network.FactoryCtx{}); err == nil {
		t.Fatal("shadow permission.header must also reject for L4 (ProfileL4)")
	}
}

func TestNew_ReturnsFilterInstanceFactory(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	fif, err := factory(mustAny(t, &networkrbacv3.RBAC{StatPrefix: "lis"}), network.FactoryCtx{})
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if fif == nil {
		t.Fatal("FilterInstanceFactory is nil")
	}
	nf := fif()
	if nf == nil {
		t.Fatal("FilterInstanceFactory produced a nil NetworkFilter")
	}
	rf, ok := nf.(network.ReadFilter)
	if !ok {
		t.Fatal("network filter does not implement ReadFilter")
	}
	if got := rf.OnNewConnection(); got != network.Continue {
		t.Errorf("OnNewConnection = %v, want Continue (stub must not sticky-halt)", got)
	}
}

// --- Task 10: OnData decision + sticky-halt regression ---

// fakeCallbacks implements network.ReadFilterCallbacks for the decision tests.
// It exposes the *fakeConn (close-recorder) and records the response-code-
// details string via the optional-interface sink the rbac filter type-asserts
// (interface{ SetResponseCodeDetails(string) }) — the same mechanism the chain's
// concrete *callbacks supplies in production (Task 6 TestResponseCodeDetailsSink).
type fakeCallbacks struct {
	conn *fakeConn
	dm   *dynamicmetadata.Bucket
	rcd  string
}

func newFakeCallbacks(conn *fakeConn) *fakeCallbacks {
	return &fakeCallbacks{conn: conn, dm: dynamicmetadata.NewBucket()}
}

func (cb *fakeCallbacks) Connection() network.Connection           { return cb.conn }
func (cb *fakeCallbacks) ContinueReading()                         {}
func (cb *fakeCallbacks) DynamicMetadata() *dynamicmetadata.Bucket { return cb.dm }
func (cb *fakeCallbacks) SetResponseCodeDetails(s string)          { cb.rcd = s }

var _ network.ReadFilterCallbacks = (*fakeCallbacks)(nil)

// allowingConn has remote IP 10.100.0.5, which falls inside the 10.100.0.0/24
// CIDR that denyAllConfig's principal matches → the policy fires → Allowed.
// allowAllConfig (any principal) also allows this connection. The IP is
// intentionally outside the 203.0.113.0/24 range used by shadowDenyConfig, so
// the shadow-deny counter test still exercises a genuine shadow-deny.
func allowingConn() *fakeConn {
	return &fakeConn{
		local:  &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 8443},
		remote: &net.TCPAddr{IP: net.ParseIP("10.100.0.5"), Port: 51000},
	}
}

// denyingConn has remote IP 192.168.1.5, which is OUTSIDE the 10.100.0.0/24
// CIDR that denyAllConfig's only principal requires → no policy matches →
// default-deny. The two helpers thus differ in a fact the engine actually
// discriminates on, making the deny test exercise real L4 discrimination.
func denyingConn() *fakeConn {
	return &fakeConn{
		local:  &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 8443},
		remote: &net.TCPAddr{IP: net.ParseIP("192.168.1.5"), Port: 51000},
	}
}

// allowAllConfig: ALLOW action with an `any` principal + `any` permission →
// every connection matches → Allowed.
func allowAllConfig(t *testing.T) *networkrbacv3.RBAC {
	t.Helper()
	return &networkrbacv3.RBAC{
		StatPrefix: "lis",
		Rules: &configrbacv3.RBAC{
			Action: configrbacv3.RBAC_ALLOW,
			Policies: map[string]*configrbacv3.Policy{
				"p": {
					Permissions: []*configrbacv3.Permission{{Rule: &configrbacv3.Permission_Any{Any: true}}},
					Principals:  []*configrbacv3.Principal{{Identifier: &configrbacv3.Principal_Any{Any: true}}},
				},
			},
		},
	}
}

// denyAllConfig: ALLOW action whose only principal requires a direct_remote_ip
// in 10.100.0.0/24. allowingConn (10.100.0.5) IS in this range → policy matches
// → Allowed. denyingConn (192.168.1.5) is NOT in this range → no match →
// default-deny. Against the SAME config the two helpers produce opposite outcomes,
// so their names reflect real L4 discrimination. Uses an L4-legal
// principal/permission so it compiles under ProfileL4.
func denyAllConfig(t *testing.T) *networkrbacv3.RBAC {
	t.Helper()
	return &networkrbacv3.RBAC{
		StatPrefix: "lis",
		Rules: &configrbacv3.RBAC{
			Action: configrbacv3.RBAC_ALLOW,
			Policies: map[string]*configrbacv3.Policy{
				"p": {
					Permissions: []*configrbacv3.Permission{{Rule: &configrbacv3.Permission_Any{Any: true}}},
					Principals: []*configrbacv3.Principal{{
						Identifier: &configrbacv3.Principal_DirectRemoteIp{DirectRemoteIp: &corev3.CidrRange{
							AddressPrefix: "10.100.0.0",
							PrefixLen:     &wrapperspb.UInt32Value{Value: 24},
						}},
					}},
				},
			},
		},
	}
}

// newFilterWith constructs the real *filter via the factory (NewFactory(reg) →
// factory(any(cfg)) → FilterInstanceFactory() → NetworkFilter), type-asserts to
// *filter, and wires the callbacks. The shared registry is stored on a returned
// closure-free struct so assertCounter can read the predeclared counters.
func newFilterWith(t *testing.T, cfg *networkrbacv3.RBAC, cb network.ReadFilterCallbacks) (*filter, *stats.Registry) {
	t.Helper()
	reg := stats.NewRegistry()
	fif, err := NewFactory(reg)(mustAny(t, cfg), network.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory rejected config: %v", err)
	}
	nf := fif()
	f, ok := nf.(*filter)
	if !ok {
		t.Fatalf("NetworkFilter is %T, want *filter", nf)
	}
	f.SetReadFilterCallbacks(cb)
	return f, reg
}

// assertCounter reads the predeclared `lis.rbac.<suffix>` counter from the
// registry the factory registered against.
func assertCounter(t *testing.T, reg *stats.Registry, suffix string, want uint64) {
	t.Helper()
	name := "lis.rbac." + suffix
	c := findCounter(reg, name)
	if c == nil {
		t.Fatalf("counter %q not registered", name)
	}
	if got := c.Load(); got != want {
		t.Errorf("counter %q = %d, want %d", name, got, want)
	}
}

func TestOnNewConnection_NeverStopIteration_StickyHaltRegression(t *testing.T) {
	// memory reference_network_read_filter_onnewconnection_halts: a StopIteration
	// from OnNewConnection sets sticky connHalted blocking all OnData. rbac_network
	// MUST Continue from OnNewConnection.
	f, _ := newFilterWith(t, allowAllConfig(t), newFakeCallbacks(allowingConn()))
	if got := f.OnNewConnection(); got != network.Continue {
		t.Fatalf("OnNewConnection = %v, want Continue (sticky-halt-safe)", got)
	}
}

func TestOnData_AllowContinues_IncrementsAllowed(t *testing.T) {
	cb := newFakeCallbacks(allowingConn())
	f, reg := newFilterWith(t, allowAllConfig(t), cb)
	if got := f.OnData(&network.Buffer{}, false); got != network.Continue {
		t.Fatalf("allow OnData = %v, want Continue", got)
	}
	if cb.conn.closed {
		t.Error("allow must not close the connection")
	}
	if cb.rcd != "" {
		t.Errorf("allow must not set response-code-details; got %q", cb.rcd)
	}
	assertCounter(t, reg, "allowed", 1)
	assertCounter(t, reg, "denied", 0)
}

func TestOnData_EnforcedDeny_ClosesNoFlushWithRCD_IncrementsDenied(t *testing.T) {
	cb := newFakeCallbacks(denyingConn()) // no policy matches → default-deny
	f, reg := newFilterWith(t, denyAllConfig(t), cb)
	if got := f.OnData(&network.Buffer{}, false); got != network.StopIteration {
		t.Fatalf("deny OnData = %v, want StopIteration", got)
	}
	if !cb.conn.closed || cb.conn.closeType != network.NoFlush {
		t.Errorf("enforced deny must Close(NoFlush); closed=%v type=%v", cb.conn.closed, cb.conn.closeType)
	}
	if cb.rcd != "rbac_deny_close" {
		t.Errorf("response-code-details = %q, want rbac_deny_close", cb.rcd)
	}
	assertCounter(t, reg, "denied", 1)
	assertCounter(t, reg, "allowed", 0)
}

func TestOnData_OneTimeDecidesOnce(t *testing.T) {
	cb := newFakeCallbacks(allowingConn())
	f, reg := newFilterWith(t, allowAllConfig(t), cb) // ONE_TIME_ON_FIRST_BYTE (default)
	f.OnData(&network.Buffer{}, false)
	f.OnData(&network.Buffer{}, false)  // second OnData is pass-through, no re-decide
	assertCounter(t, reg, "allowed", 1) // decided once
}

func TestOnData_Continuous_RedecidesEachCall(t *testing.T) {
	cfg := allowAllConfig(t)
	cfg.EnforcementType = networkrbacv3.RBAC_CONTINUOUS
	cb := newFakeCallbacks(allowingConn())
	f, reg := newFilterWith(t, cfg, cb)
	f.OnData(&network.Buffer{}, false)
	f.OnData(&network.Buffer{}, false) // CONTINUOUS re-decides → ticks again
	assertCounter(t, reg, "allowed", 2)
}

// shadowDenyConfig: an allow-all enforced engine PLUS a shadow engine that
// default-denies (ALLOW + non-matching CIDR principal). The enforced disposition
// stays Allowed; only the shadow_denied counter ticks.
func shadowDenyConfig(t *testing.T) *networkrbacv3.RBAC {
	t.Helper()
	cfg := allowAllConfig(t)
	cfg.ShadowRules = &configrbacv3.RBAC{
		Action: configrbacv3.RBAC_ALLOW,
		Policies: map[string]*configrbacv3.Policy{
			"s": {
				Permissions: []*configrbacv3.Permission{{Rule: &configrbacv3.Permission_Any{Any: true}}},
				Principals: []*configrbacv3.Principal{{
					Identifier: &configrbacv3.Principal_DirectRemoteIp{DirectRemoteIp: &corev3.CidrRange{
						AddressPrefix: "203.0.113.0",
						PrefixLen:     &wrapperspb.UInt32Value{Value: 24},
					}},
				}},
			},
		},
	}
	return cfg
}

func TestOnData_Shadow_TicksShadowCounter_NotEnforced(t *testing.T) {
	cb := newFakeCallbacks(allowingConn())
	f, reg := newFilterWith(t, shadowDenyConfig(t), cb)
	if got := f.OnData(&network.Buffer{}, false); got != network.Continue {
		t.Fatalf("OnData = %v, want Continue (enforced allows; shadow does not affect disposition)", got)
	}
	if cb.conn.closed {
		t.Error("shadow deny must NOT close the connection")
	}
	assertCounter(t, reg, "allowed", 1)        // enforced allowed
	assertCounter(t, reg, "shadow_denied", 1)  // shadow walk denied
	assertCounter(t, reg, "shadow_allowed", 0) // shadow walk did not allow
}

func TestOnData_ShadowWritesMetadataPair(t *testing.T) {
	// enforced allow + shadow deny configured. The shadow walk writes the
	// (shadow_engine_result, shadow_effective_policy_id) pair to the per-
	// connection dynamic-metadata bucket (ADR-0217; the FIRST per-connection
	// production WRITE through *dynamicmetadata.Bucket).
	cb := newFakeCallbacks(allowingConn())
	f, reg := newFilterWith(t, shadowDenyConfig(t), cb)
	if got := f.OnData(&network.Buffer{}, false); got != network.Continue {
		t.Fatalf("OnData = %v, want Continue (enforced allows; a shadow write must not flip the disposition)", got)
	}

	bucket := cb.DynamicMetadata()
	res, ok := bucket.Get("envoy.filters.network.rbac", "shadow_engine_result")
	if !ok || res.GetStringValue() != "denied" {
		t.Errorf("shadow_engine_result = %v ok=%v, want denied", res, ok)
	}
	// shadow_effective_policy_id written only when the matched policy id is
	// non-empty. shadowDenyConfig default-denies (no policy fires) → sname=="" →
	// the id key is absent here, which is the upstream-parity behavior.
	if id, ok := bucket.Get("envoy.filters.network.rbac", "shadow_effective_policy_id"); ok {
		if id.GetStringValue() == "" {
			t.Error("shadow_effective_policy_id written but empty")
		}
	}
	assertCounter(t, reg, "shadow_denied", 1)
}

// shadowAllowFiresConfig: an allow-all enforced engine PLUS a shadow engine
// whose single named policy ("shadow_p") uses an `any` principal + `any`
// permission → it ALWAYS fires against any connection. The shadow walk's
// Evaluate therefore returns a non-empty matched-policy name ("shadow_p"),
// driving the sname != "" branch in OnData that writes shadow_effective_policy_id.
// The shadow ACTION is ALLOW, so the shadow result is "allowed".
func shadowAllowFiresConfig(t *testing.T) *networkrbacv3.RBAC {
	t.Helper()
	cfg := allowAllConfig(t)
	cfg.ShadowRules = &configrbacv3.RBAC{
		Action: configrbacv3.RBAC_ALLOW,
		Policies: map[string]*configrbacv3.Policy{
			"shadow_p": {
				Permissions: []*configrbacv3.Permission{{Rule: &configrbacv3.Permission_Any{Any: true}}},
				Principals:  []*configrbacv3.Principal{{Identifier: &configrbacv3.Principal_Any{Any: true}}},
			},
		},
	}
	return cfg
}

func TestOnData_ShadowPolicyFires_WritesEffectivePolicyId(t *testing.T) {
	// A shadow policy ("shadow_p", any/any under a shadow ALLOW) FIRES against the
	// connection, so the shadow walk yields a non-empty matched-policy name
	// (sname != "") → shadow_effective_policy_id IS written. This covers the
	// sname != "" branch that shadowDenyConfig (default-deny, no policy fires)
	// leaves uncovered. The enforced engine still allows → Continue.
	cb := newFakeCallbacks(allowingConn())
	f, reg := newFilterWith(t, shadowAllowFiresConfig(t), cb)
	if got := f.OnData(&network.Buffer{}, false); got != network.Continue {
		t.Fatalf("OnData = %v, want Continue (enforced allows; a shadow write must not flip the disposition)", got)
	}

	bucket := cb.DynamicMetadata()
	id, ok := bucket.Get("envoy.filters.network.rbac", "shadow_effective_policy_id")
	if !ok {
		t.Fatal("shadow_effective_policy_id must be written when a shadow policy fires")
	}
	if id.GetStringValue() == "" {
		t.Error("shadow_effective_policy_id written but empty")
	}
	if got := id.GetStringValue(); got != "shadow_p" {
		t.Errorf("shadow_effective_policy_id = %q, want shadow_p (the fired policy name)", got)
	}
	// The shadow ALLOW action fires → shadow_engine_result is "allowed".
	res, ok := bucket.Get("envoy.filters.network.rbac", "shadow_engine_result")
	if !ok || res.GetStringValue() != "allowed" {
		t.Errorf("shadow_engine_result = %v ok=%v, want allowed", res, ok)
	}
	assertCounter(t, reg, "shadow_allowed", 1)
	assertCounter(t, reg, "allowed", 1) // enforced allowed
}

func TestOnData_NoShadowConfigured_NoMetadata(t *testing.T) {
	cb := newFakeCallbacks(allowingConn())
	f, _ := newFilterWith(t, allowAllConfig(t), cb) // no shadow
	f.OnData(&network.Buffer{}, false)
	if _, ok := cb.DynamicMetadata().Get("envoy.filters.network.rbac", "shadow_engine_result"); ok {
		t.Error("no shadow configured → no metadata write")
	}
}

func TestOnData_WhollyInactive_Passthrough_NoCounters(t *testing.T) {
	// Neither enforced engine set → wholly inactive (rbac.pb.go:33) →
	// passthrough Continue, NO counters (mirrors HTTP consumer BOTH-nil gate).
	cb := newFakeCallbacks(allowingConn())
	f, reg := newFilterWith(t, &networkrbacv3.RBAC{StatPrefix: "lis"}, cb)
	if got := f.OnData(&network.Buffer{}, false); got != network.Continue {
		t.Fatalf("inactive OnData = %v, want Continue", got)
	}
	if cb.conn.closed {
		t.Error("inactive filter must not close the connection")
	}
	assertCounter(t, reg, "allowed", 0)
	assertCounter(t, reg, "denied", 0)
}

func TestOnData_Continuous_WhollyInactive_Passthrough(t *testing.T) {
	// CONTINUOUS enforcement_type + no enforced engine (both rules+matcher nil) →
	// the wholly-inactive gate is reached before the decided/enforcementContinuous
	// logic fires on every call. Every OnData must return Continue with NO counters
	// incremented, locking the ordering invariant (inactive check precedes the
	// CONTINUOUS re-decide path).
	cfg := &networkrbacv3.RBAC{
		StatPrefix:      "lis",
		EnforcementType: networkrbacv3.RBAC_CONTINUOUS,
	}
	cb := newFakeCallbacks(allowingConn())
	f, reg := newFilterWith(t, cfg, cb)
	for i := 0; i < 3; i++ {
		if got := f.OnData(&network.Buffer{}, false); got != network.Continue {
			t.Fatalf("call %d: inactive CONTINUOUS OnData = %v, want Continue", i, got)
		}
	}
	if cb.conn.closed {
		t.Error("inactive filter must not close the connection")
	}
	assertCounter(t, reg, "allowed", 0)
	assertCounter(t, reg, "denied", 0)
}

// ----------------------------------------------------------------------------
// Task 15: PARSE-REJECT byte-stable wording table (D-P6).
// ----------------------------------------------------------------------------

// TestParseRejectConstants_ByteStable pins the EXACT operator-visible reject
// wording for all four verb-free rbac_network PARSE-REJECT consts. Drift here is
// an operator-visible behavior change and must be deliberate. The two
// stat_prefix-invalid consts were added at Task 13 (fuzzer-found stat_prefix
// validation). The ProfileL4 HTTP-only-arm reject wording is pinned separately
// in internal/rbac (profile_test.go's TestProfileL4RejectWording_ByteStable).
func TestParseRejectConstants_ByteStable(t *testing.T) {
	cases := []struct{ name, got, want string }{
		{"StatPrefixRequired", parseRejectStatPrefixRequired, "rbac_network: stat_prefix is required"},
		{"StatPrefixInvalid", parseRejectStatPrefixInvalid, "rbac_network: stat_prefix contains characters invalid for a metric name"},
		{"ShadowStatPrefixInvalid", parseRejectShadowStatPrefixInvalid, "rbac_network: shadow_rules_stat_prefix contains characters invalid for a metric name"},
		{"DelayDeny", parseRejectDelayDeny, "rbac_network: delay_deny is unsupported"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("byte-stable drift: %s\n const: %q\n  want: %q", tc.name, tc.got, tc.want)
		}
	}
}
