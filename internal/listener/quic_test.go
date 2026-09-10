package listener

import (
	"context"
	stdtls "crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"sort"
	"testing"
	"time"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	quic "github.com/quic-go/quic-go"
	http3 "github.com/quic-go/quic-go/http3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/listener/listenerfilter"
	"github.com/pgdad/envoy-go/internal/stats"
)

// TestQUICGoModuleWired is a compile-time proof the quic-go v0.54.1 module is
// wired and the API leg 61.1 depends on exists. Not behavioral — Task 6
// exercises the real bind + handshake.
func TestQUICGoModuleWired(t *testing.T) {
	_ = &quic.Config{MaxIdleTimeout: 30 * time.Second, HandshakeIdleTimeout: 5 * time.Second}
	var _ func(net.PacketConn, *stdtls.Config, *quic.Config) (*quic.Listener, error) = quic.Listen
}

// pollCounter reads the registry counter named `name`, retrying until its value
// is >= want or timeout elapses, and returns the last observed value. The
// accept path Inc's the cx counter from a goroutine, so the value may lag the
// client's handshake completion — poll, do not read once.
func pollCounter(t *testing.T, reg *stats.Registry, name string, want uint64, timeout time.Duration) uint64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last uint64
	for {
		last = 0
		reg.Walk(func(m stats.Metric) {
			if m.Name() != name {
				return
			}
			if c, ok := m.(*stats.Counter); ok {
				last = c.Load()
			}
		})
		if last >= want || time.Now().After(deadline) {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// counterValue reads the registry counter named `name` ONCE and returns its
// value. Unlike pollCounter it asserts an EXACT value at the call site rather
// than polling for a minimum, which is what an "is still zero" assertion needs
// — pollCounter would return 0 both for "registered and zero" and for "not
// registered at all".
//
// ⚠️ It t.Errorf's when `name` is ABSENT from the registry and MUST NOT be
// changed to return a silent 0. The zero-assertion in
// TestQUICListener_RegistersSSLNamesAtZero would otherwise pass VACUOUSLY under
// a break that stops registering the ssl.* counters entirely: nothing
// registered ⇒ every read is 0 ⇒ "all five are zero" is trivially true.
func counterValue(t *testing.T, reg *stats.Registry, name string) int64 {
	t.Helper()
	var (
		val   uint64
		found bool
	)
	reg.Walk(func(m stats.Metric) {
		if m.Name() != name {
			return
		}
		if c, ok := m.(*stats.Counter); ok {
			val = c.Load()
			found = true
		}
	})
	if !found {
		t.Errorf("counter %q is not registered", name)
		return -1
	}
	return int64(val)
}

// driveH3 performs one real HTTP/3 GET against a live QUIC listener's bound UDP
// address, using the same http3.Transport shape as
// TestQUICListener_ServesH3GET, and returns a non-nil error if any leg of the
// round trip fails. Callers treat a failure as a PRECONDITION break, not a
// property: without a completed H3 handshake, an "the ssl.* counters are still
// zero" assertion is vacuous.
func driveH3(t *testing.T, addr string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rt := &http3.Transport{
		TLSClientConfig: &stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true}, //nolint:gosec // local test
		QUICConfig:      &quic.Config{},
	}
	defer func() { _ = rt.Close() }()
	client := &http.Client{Transport: rt}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return fmt.Errorf("H3 GET %s: status = %d, want 200", addr, resp.StatusCode)
	}
	if resp.ProtoMajor != 3 {
		return fmt.Errorf("H3 GET %s: proto major = %d, want 3", addr, resp.ProtoMajor)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		return fmt.Errorf("H3 GET %s: read body: %w", addr, err)
	}
	return nil
}

// TestQUICListener_HandshakeALPNh3 is the leg-61.1 subject-side proof: a QUIC
// listener binds UDP, and a local quic-go client completes the QUIC/TLS-1.3
// handshake negotiating ALPN h3. NO HTTP is served (leg 61.2).
func TestQUICListener_HandshakeALPNh3(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListener(t, "c_echo", testAlphaCertPEM, testAlphaKeyPEM, []string{"h3"})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManager(boot, cm, reg, testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	infos := mgr.Listeners()
	if len(infos) != 1 {
		t.Fatalf("Listeners() = %d, want 1 (QUIC listener must report its bound UDP addr)", len(infos))
	}
	addr := infos[0].Addr

	clientTLS := &stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true} //nolint:gosec // local handshake test
	conn, err := quic.DialAddr(ctx, addr, clientTLS, &quic.Config{})
	if err != nil {
		t.Fatalf("quic.DialAddr(%s): %v", addr, err)
	}
	defer func() { _ = conn.CloseWithError(0, "") }()

	tlsState := conn.ConnectionState().TLS
	if tlsState.NegotiatedProtocol != "h3" {
		t.Errorf("negotiated ALPN = %q, want %q", tlsState.NegotiatedProtocol, "h3")
	}
	if tlsState.Version != stdtls.VersionTLS13 {
		t.Errorf("TLS version = %#x, want TLS 1.3 (%#x)", tlsState.Version, stdtls.VersionTLS13)
	}
	// The accept path Inc'd downstream_cx_total for the completed handshake.
	// Accept runs in a goroutine so the Inc may lag the client's handshake
	// completion — poll briefly.
	if got := pollCounter(t, reg, "listener."+normalizeAddr(addr)+".downstream_cx_total", 1, 2*time.Second); got < 1 {
		t.Errorf("downstream_cx_total = %d, want >= 1", got)
	}
}

// TestQUICListener_ServesH3GET is the leg-61.2 subject-side proof: a QUIC
// listener whose HCM has codec_type HTTP3 and a direct_response /health route
// (mkQUICListenerHCM) serves an H3 GET to a local quic-go http3.Transport
// client, returning 200 + "OK\n" over HTTP/3. NO differential (that is 61.3).
func TestQUICListener_ServesH3GET(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListenerHCM(t, testAlphaCertPEM, testAlphaKeyPEM, hcmv3.HttpConnectionManager_HTTP3)
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	addr := mgr.Listeners()[0].Addr

	rt := &http3.Transport{
		TLSClientConfig: &stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true}, //nolint:gosec // local test
		QUICConfig:      &quic.Config{},
	}
	defer func() { _ = rt.Close() }()
	client := &http.Client{Transport: rt}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+"/health", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("H3 GET %s: %v", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.ProtoMajor != 3 {
		t.Errorf("proto major = %d, want 3 (HTTP/3)", resp.ProtoMajor)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "OK\n" {
		t.Errorf("body = %q, want %q", string(body), "OK\n")
	}
}

// TestQUICListener_RegistersSSLNamesAtZero pins SPEC §3.4: a QUIC listener
// registers all FIVE ssl.* counters (the three phase-74 outcome counters plus
// phase 75's ssl.no_certificate plus phase 94's ssl.connection_error), and they
// stay permanently ZERO
// across a COMPLETED HTTP/3 handshake — because quic-go's Accept returns
// post-handshake, so a QUIC handshake never surfaces as a per-connection event
// on the TCP serveConnection path that could increment them.
//
// ⚠️ THE ASSERTION ABOVE IS CORRECT AND UNCHANGED — five names registered,
// permanently zero across a COMPLETED HTTP/3 handshake. Only the REASON is
// replaced. This doc used to say "startQUIC hard-errors without a TLS config so
// rt.tlsMode is necessarily true". That is FALSE, is repealed by ADR-0318, and
// phase 97 does NOT un-repeal it: a QUIC listener with zero filter_chains[] and a
// QUIC-wrapped default_filter_chain satisfied startQUIC's mandatory-TLS check and
// still built with tlsMode == false, registering NONE of the five. Only the
// MECHANISM moved. quicTLSConfig() used to try rt.defaultChain.tlsCfg BEFORE
// ranging over the rt.chainByName MAP; it now runs a Start-time
// filter_chain_match selection first, then rt.chainSpecs in SLICE order, and
// consults the default slot LAST — the map range is deleted and rt.chainByName is
// only a name -> *chainInfo lookup. For the zero-filter_chains[] shape the
// Start-time selection returns the default spec anyway, so the default slot's
// tlsCfg still reaches quic.Listen and the repeal holds under BOTH orders. (No
// quic.go line anchor is cited here on purpose; the previous one rotted.)
//
// rt.tlsMode is true here because phase 96 widened the write site to cover both
// structural slots — and the shape that used to register zero now registers five.
//
// ⚠️ This is PARITY, not a departure. The reference behaves IDENTICALLY:
// ssl.handshake: 0 after five successful H3 connections, and connection_error: 0
// on a failure arm where the TCP comparator in the SAME process and the SAME
// scrape fired (phase-74 SPEC §3.4, EXECUTED). Gating QUIC out — which this
// SPEC's own first reading did — would have been the departure. Break D adds
// that refuted `rt.kind != kindQUIC` gate and this test's assertion (1) is the
// only thing in the suite that separates it from the landed behavior.
//
// ⚠️ Assertion order is load-bearing: (1) is the NAME-SET comparison and must be
// the discriminating failure under Break D. (4)'s zero check cannot discriminate
// on its own — with nothing registered every read is 0 — which is why
// counterValue errors on an absent name rather than returning a silent 0.
//
// ⚠️ Only the counter half of the pre-existing cx pin is asserted here.
// downstream_cx_active is a gauge; pollCounter type-asserts *stats.Counter and
// has no gauge equivalent, and no test in this file asserts the gauge. The gauge
// half is deliberately UNASSERTED.
func TestQUICListener_RegistersSSLNamesAtZero(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListenerHCM(t, testAlphaCertPEM, testAlphaKeyPEM, hcmv3.HttpConnectionManager_HTTP3)
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManager(boot, cm, reg, testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	infos := mgr.Listeners()
	if len(infos) != 1 {
		t.Fatalf("Listeners() = %d, want 1 (QUIC listener must report its bound UDP addr)", len(infos))
	}
	addr := infos[0].Addr
	prefix := "listener." + normalizeAddr(addr) + "."

	// (1) REGISTRATION: all five names present, spelled exactly. A cardinality
	//     assertion would pass with all five misspelled — compare the set.
	//     SORTED: listenerSSLNames sorts, so want must too; ssl.no_certificate
	//     collates LAST and phase 94's ssl.connection_error collates FIRST (it
	//     PREPENDS, at index 0). The phase-75 and phase-94 names stay ZERO here
	//     for the SAME STRUCTURAL reason as the other three — Manager.Start
	//     never launches an accept loop for kindQUIC, so serveConnection's TLS
	//     block (the ONLY ssl.no_certificate and ssl.connection_error Inc site)
	//     is unreachable. No volume of H3 traffic moves them.
	want := []string{
		prefix + "ssl.connection_error",
		prefix + "ssl.fail_verify_error",
		prefix + "ssl.fail_verify_no_cert",
		prefix + "ssl.handshake",
		prefix + "ssl.no_certificate",
	}
	if got := listenerSSLNames(reg, addr); !reflect.DeepEqual(got, want) {
		t.Errorf("QUIC listener ssl name set = %v, want %v", got, want)
	}

	// (2) drive a REAL H3 request. A failed drive would make (3) and (4)
	//     vacuous, so this is a PRECONDITION, not a property.
	if err := driveH3(t, addr); err != nil {
		t.Fatalf("precondition: H3 round trip failed, so the zero-assertion below would be vacuous: %v", err)
	}

	// (3) the cx counter DID move — proving the connection was accounted, so the
	//     zeros in (4) are a real observation and not "nothing happened".
	if got := pollCounter(t, reg, prefix+"downstream_cx_total", 1, 2*time.Second); got < 1 {
		t.Errorf("downstream_cx_total = %d, want >= 1", got)
	}

	// (4) ...and all five ssl.* counters are STILL ZERO.
	for _, n := range want {
		if v := counterValue(t, reg, n); v != 0 {
			t.Errorf("%s = %d after a completed H3 handshake, want 0", n, v)
		}
	}

	// (5) and nothing panicked — implicit, but state it: a nil-Counter Inc from
	//     the QUIC accept goroutine would crash the binary, not fail this test
	//     (reference_nil_stats_counter_inc_crashes_goroutine).
}

// TestQUICChainSelection_IndexedChainWinsOverDefaultSlot is phase-97 arm (a).
//
// SHAPE: one QUIC listener carrying an EMPTY-MATCH (universally eligible)
// filter_chains[0] AND a QUIC-TLS default_filter_chain.
//
// PROPERTY: (*listenerRuntime).quicChain() must select filter_chains[0]. The
// default_filter_chain is the LAST-RESORT slot — it is consulted only when no
// indexed chain is eligible, and an empty-match indexed chain is eligible for
// every connection. Envoy's reference has NO QUIC exception to that ordering.
//
// The assertion is an EXACT POINTER EQUALITY against
// rt.chainByName["quic_listener_chains/filter_chains[0]"], not a non-nil
// floor: chainInfo has exactly three fields (serverNames, tlsCfg,
// netChainFactory) and NO Name, so the map key is the only binding between a
// ChainSpec and its *chainInfo — identity is the only thing that can name the
// selected chain. rt.chainByName holds the default slot too, under
// "quic_listener_chains/default_filter_chain".
//
// This test lives in quic_test.go rather than manager_test.go because its
// subject is quic.go's quicChain() accessor; both files are package `listener`
// so mkQUICListenerChains and the mk* boot helpers are in scope here.
//
// ⚠️ t.Errorf per property, never t.Fatalf: a Fatalf would make every later
// assertion dead code. Fatalf is reserved for broken PRECONDITIONS (the
// listener failed to build; a chain key is absent; the two chains are the same
// pointer, which would make the equality vacuous).
func TestQUICChainSelection_IndexedChainWinsOverDefaultSlot(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListenerChains(t, nil, "FC0\n", true, true, "DFC\n")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, empty-match filter_chains[0] + QUIC-TLS default slot): %v", err)
	}
	rt := mgr.runtimes[0]

	idxKey := l.Name + "/filter_chains[0]"
	dfcKey := l.Name + "/default_filter_chain"
	indexed := rt.chainByName[idxKey]
	dflt := rt.chainByName[dfcKey]
	if indexed == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", idxKey, quicChainKeys(rt))
	}
	if dflt == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", dfcKey, quicChainKeys(rt))
	}
	if indexed == dflt {
		t.Fatalf("precondition: chainByName[%q] and chainByName[%q] are the SAME *chainInfo (%p) — a pointer-equality assertion would be vacuous", idxKey, dfcKey, indexed)
	}

	got := rt.quicChain(nil)

	// PROPERTY 1 — selection identity: quicChain() returns filter_chains[0].
	if got != indexed {
		t.Errorf("selection identity: quicChain() = %p, want filter_chains[0] = %p (an empty-match indexed chain is eligible for every connection and must be selected)", got, indexed)
	}

	// PROPERTY 2 — last-resort ordering: quicChain() does NOT return the
	// default slot. Stated separately from property 1 so a failure names the
	// default_filter_chain explicitly instead of collapsing "returned the
	// wrong chain" and "returned the last-resort slot" onto one line.
	if got == dflt {
		t.Errorf("last-resort ordering: quicChain() returned the default_filter_chain %p, but default_filter_chain is the LAST-RESORT slot and must not pre-empt the eligible filter_chains[0] %p", dflt, indexed)
	}
}

// quicChainKeys returns rt.chainByName's keys sorted, for precondition
// failure messages in the phase-97 chain-selection arms.
func quicChainKeys(rt *listenerRuntime) []string {
	keys := make([]string, 0, len(rt.chainByName))
	for k := range rt.chainByName {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// quicIneligibleDestPort is the destination_port stamped on the indexed chain
// of the phase-97 arms (b) and (c). It is deliberately NOT the listener's
// port: mkQUICListenerChains configures `port_value: 0`, and manager.go builds
// rt.addr from the CONFIGURED address+port (manager.go:837,
// fmt.Sprintf("%s:%d", ...)), so a runtime that has never been Started carries
// rt.addr == "127.0.0.1:0". Only startQUIC's post-bind
// `rt.addr = udpConn.LocalAddr().String()` ever replaces that with the
// OS-picked port, and these arms never call Start. Any non-zero
// destination_port is therefore ineligible here, and 65000 carries no other
// meaning — it is not a port anything in this package binds.
//
// ⚠️ Because the comparison is against port 0 rather than against a real bound
// port, these arms prove "a destination_port that does not equal the
// listener's port makes the chain ineligible" and NOT "the selector reads the
// RESOLVED port". The second claim needs a Started listener, and it is not
// constructible from config (see the port-question note on arm (b) below).
const quicIneligibleDestPort uint32 = 65000

// TestQUICChainSelection_IneligibleIndexedChainFallsBackToDefaultSlot is
// phase-97 arm (b): the NEGATIVE half of the matched pair whose positive half
// is TestQUICChainSelection_IndexedChainWinsOverDefaultSlot (arm (a)).
//
// SHAPE: one QUIC listener whose filter_chains[0] carries
// `destination_port: 65000` — a port this listener is not bound to — plus a
// QUIC-TLS default_filter_chain.
//
// PROPERTY: (*listenerRuntime).quicChain() must select the
// default_filter_chain. filter_chains[0] is INELIGIBLE
// (chainmatch.go:119, `c.DestinationPort != 0 && c.DestinationPort !=
// inputs.DestinationPort`), and SelectChain's pass-1 empty-eligible branch
// (chainmatch.go:87-92) then hands back defaultChain. The default slot is the
// LAST-RESORT slot and this is exactly the case that resorts to it.
//
// 🟢 THIS ARM IS GREEN AT THE UN-FIXED TIP, AND THAT IS NOT COVERAGE.
// At this tip quicChain() is nullary and consults NOTHING:
//
//	if rt.defaultChain != nil { return rt.defaultChain }
//
// (quic.go:74-81). It answers "the default chain" on EVERY QUIC connection,
// evaluating no dimension of any filter_chain_match. Arm (b)'s expected answer
// happens to BE the default chain, so the subject agrees with the reference
// here for a reason the mechanism cannot supply — coincidence, not evaluation.
// A reader who sees this arm pass before the fix MUST NOT read that as the
// destination_port dimension being enforced.
//
// ⚠️ WHAT THE PAIR RULES OUT, WHICH NEITHER ARM RULES OUT ALONE.
// Arm (a) alone (empty match ⇒ indexed wins) is satisfiable by a hypothetical
// selector that returns filter_chains[0] unconditionally. Arm (b) alone
// (ineligible match ⇒ default wins) is satisfiable by a selector that returns
// the default slot unconditionally — which is EXACTLY what the tip does. Only
// the pair, whose two listeners differ in ONE field (filter_chains[0]'s
// filter_chain_match: absent vs `destination_port: 65000`) and are byte-
// identical everywhere else, rules out both constant answers at once: no
// constant can be right on both arms. That is the gate; a single positive arm
// is not.
//
// ⚠️ THE PORT QUESTION (roster row 6). A "port-0 variant of arm (b)" that
// reddens when `rt.addr = udpConn.LocalAddr().String()` is MOVED above the
// quicTLSConfig() call inside startQUIC is NOT CONSTRUCTIBLE, for two
// independent reasons:
//
//  1. The assignment executes on BOTH sides of the move; only its ORDER within
//     startQUIC changes. Nothing between the two candidate positions reads
//     rt.addr — the span is quic.Listen, `rt.udpConn = udpConn`,
//     `rt.quicCloser = ql` — and quicTLSConfig() itself consults only
//     rt.defaultChain / rt.chainByName. registerListenerMetrics, the one
//     rt.addr reader in startQUIC, already sits AFTER both positions. No
//     chain selection runs while startQUIC runs: serveQUICConnection is
//     reached only from the accept goroutine, which is launched last.
//
//  2. Even given an observation window, exploiting it needs a chain whose
//     destination_port EQUALS the OS-picked resolved port — a value that does
//     not exist until the bind, and so cannot be written into a listener
//     config that NewManager consumed before Start. Nor can the pre-resolve
//     state be asserted from the other side: destination_port 0 means
//     "unspecified" and SKIPS the dimension (chainmatch.go:119), so no chain
//     can spell "the port is still 0".
//
// A roster row whose target arm does not exist is a VACUOUS control. What row
// 6 can still falsify, if restated as a DELETION rather than a move — never
// assigning the resolved address at all — is the pair of live-handshake arms
// TestQUICListener_HandshakeSucceeds and
// TestQUICListener_RegistersSSLNamesAtZero: both take their dial target from
// Listeners()[0].Addr, which is rt.addr, so a deleted resolve makes them dial
// "127.0.0.1:0" and fail at DialAddr. No new arm is needed for that, and none
// is added here.
func TestQUICChainSelection_IneligibleIndexedChainFallsBackToDefaultSlot(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	fcm := &listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(quicIneligibleDestPort)}
	l := mkQUICListenerChains(t, fcm, "FC0\n", true, true, "DFC\n")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, destination_port=%d filter_chains[0] + QUIC-TLS default slot): %v", quicIneligibleDestPort, err)
	}
	rt := mgr.runtimes[0]

	idxKey := l.Name + "/filter_chains[0]"
	dfcKey := l.Name + "/default_filter_chain"
	indexed := rt.chainByName[idxKey]
	dflt := rt.chainByName[dfcKey]
	if indexed == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", idxKey, quicChainKeys(rt))
	}
	if dflt == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", dfcKey, quicChainKeys(rt))
	}
	if indexed == dflt {
		t.Fatalf("precondition: chainByName[%q] and chainByName[%q] are the SAME *chainInfo (%p) — a pointer-equality assertion would be vacuous", idxKey, dfcKey, indexed)
	}

	// PRECONDITION — the ineligibility this arm depends on is real: the
	// runtime's address must still carry port 0, so a chain naming port 65000
	// cannot match. Asserted rather than assumed; if a future change resolves
	// rt.addr at build time this arm would silently stop testing fallback.
	if rt.addr != "127.0.0.1:0" {
		t.Fatalf("precondition: rt.addr = %q, want \"127.0.0.1:0\" (a never-Started runtime must carry the CONFIGURED port, or destination_port=%d might be eligible after all)", rt.addr, quicIneligibleDestPort)
	}

	got := rt.quicChain(nil)

	// PROPERTY 1 — fallback identity: quicChain() returns the
	// default_filter_chain.
	if got != dflt {
		t.Errorf("fallback identity: quicChain() = %p, want default_filter_chain = %p (filter_chains[0] names destination_port %d, which this listener is not bound to, so no indexed chain is eligible and the last-resort slot must be selected)", got, dflt, quicIneligibleDestPort)
	}

	// PROPERTY 2 — ineligibility is honored: quicChain() does NOT return the
	// ineligible indexed chain. Stated separately from property 1 so a failure
	// names filter_chains[0] explicitly instead of collapsing "returned the
	// wrong chain" and "returned a chain whose match block excludes this
	// connection" onto one line.
	if got == indexed {
		t.Errorf("ineligibility honored: quicChain() returned filter_chains[0] %p, but its filter_chain_match names destination_port %d and the listener's address is %q — an ineligible chain must never be selected", indexed, quicIneligibleDestPort, rt.addr)
	}
}

// TestQUICChainSelection_IneligibleIndexedChainNoDefaultSlotSelectsNothing is
// phase-97 arm (c): arm (b) with the default_filter_chain REMOVED.
//
// SHAPE: one QUIC listener whose only chain is filter_chains[0] carrying
// `destination_port: 65000`, and NO default_filter_chain at all. The boot
// guard permits this: the only listener-level rejection in this area is
// `catchAllCount > 1` (manager.go:715), and one chain omitting server_names
// counts once.
//
// PROPERTY: (*listenerRuntime).quicChain() must return NIL. No indexed chain
// is eligible and there is no default slot, which is precisely
// SelectChain's (nil, ErrNoChainMatched) branch (chainmatch.go:87-92).
//
// 🔴 WHY NIL IS THE RIGHT ANSWER AND NOT A DEGENERATE ONE. nil is not an
// error value that gets swallowed — it is the input that makes
// serveQUICConnection CLOSE the connection:
//
//	ci := rt.quicChain()
//	if ci == nil { _ = conn.CloseWithError(0, ""); return }
//
// (quic.go:123-126). Closing is exactly what the pinned Envoy reference does
// when no filter chain matches a connection ("no filter chain found"). So
// "returns nil" and "the connection is refused" are the same behavior seen
// from two sides, and this arm pins the side that is unit-observable without a
// live handshake.
//
// 🔴 THIS ARM IS RED AT THE UN-FIXED TIP. quicChain() is nullary; with
// rt.defaultChain nil it falls through to the map walk and returns an
// ARBITRARY entry (quic.go:74-81) — here the sole entry, the INELIGIBLE
// filter_chains[0]. The tip therefore serves a connection through a chain
// whose filter_chain_match excludes it, where the reference would have closed
// the connection.
//
// ⚠️ WHAT THE (b)/(c) PAIR RULES OUT. Arms (b) and (c) differ in ONE field —
// `withDefault` — and are otherwise byte-identical, same ineligible indexed
// chain included. (b) alone is satisfiable by "always return the default
// slot", which is the tip's actual behavior. (c) alone is satisfiable by
// "return nil whenever there is no default slot", which would break arm (a)'s
// sibling shape. Together they force the selector to distinguish
// ELIGIBILITY from PRESENCE: the default slot is consulted because no indexed
// chain matched, not because it exists, and its absence must produce nil
// rather than a fallback onto the chain that was just ruled out.
func TestQUICChainSelection_IneligibleIndexedChainNoDefaultSlotSelectsNothing(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	fcm := &listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(quicIneligibleDestPort)}
	l := mkQUICListenerChains(t, fcm, "FC0\n", false, false, "")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, destination_port=%d filter_chains[0], NO default slot): %v", quicIneligibleDestPort, err)
	}
	rt := mgr.runtimes[0]

	idxKey := l.Name + "/filter_chains[0]"
	dfcKey := l.Name + "/default_filter_chain"
	indexed := rt.chainByName[idxKey]
	if indexed == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", idxKey, quicChainKeys(rt))
	}
	// PRECONDITION — the default slot really is absent, in BOTH bindings.
	// rt.defaultChain is what quicChain() branches on; chainByName is what an
	// identity assertion would look the chain up in. A builder that quietly
	// built a default slot would make this arm's "want nil" unreachable.
	if rt.defaultChain != nil {
		t.Fatalf("precondition: rt.defaultChain = %p, want nil (mkQUICListenerChains was called with withDefault=false)", rt.defaultChain)
	}
	if dflt, ok := rt.chainByName[dfcKey]; ok {
		t.Fatalf("precondition: chainByName[%q] is PRESENT (%p) despite withDefault=false (keys: %v)", dfcKey, dflt, quicChainKeys(rt))
	}
	if len(rt.chainByName) != 1 {
		t.Fatalf("precondition: len(chainByName) = %d, want 1 (keys: %v)", len(rt.chainByName), quicChainKeys(rt))
	}
	if rt.addr != "127.0.0.1:0" {
		t.Fatalf("precondition: rt.addr = %q, want \"127.0.0.1:0\" (a never-Started runtime must carry the CONFIGURED port, or destination_port=%d might be eligible after all)", rt.addr, quicIneligibleDestPort)
	}

	got := rt.quicChain(nil)

	// PROPERTY 1 — no-match selects nothing: quicChain() returns nil.
	if got != nil {
		t.Errorf("no-match selects nothing: quicChain() = %p, want nil (filter_chains[0] names destination_port %d against listener address %q and there is no default_filter_chain, so NO chain is selectable)", got, quicIneligibleDestPort, rt.addr)
	}

	// PROPERTY 2 — the ineligible chain is not the fallback. Stated
	// separately so a failure names WHICH non-nil chain came back: "returned
	// non-nil" and "returned the very chain whose match block excludes this
	// connection" are different defects and a single assertion would conflate
	// them.
	if got == indexed {
		t.Errorf("ineligible chain is not the fallback: quicChain() returned filter_chains[0] %p, whose filter_chain_match names destination_port %d — with no default_filter_chain the absence of an eligible chain must yield nil, which is what makes serveQUICConnection close the connection", indexed, quicIneligibleDestPort)
	}
}

// TestQUICChainSelection_TransportProtocolQUICMatches is phase-97 arm (d), the
// POSITIVE half of the transport_protocol pair completed by
// TestQUICChainSelection_TransportProtocolTLSDoesNotMatch (arm (e)).
//
// SHAPE: one QUIC listener whose filter_chains[0] carries
// `transport_protocol: "quic"`, plus a QUIC-TLS default_filter_chain.
//
// PROPERTY: (*listenerRuntime).quicChain() must select filter_chains[0]. A
// QUIC connection's transport protocol IS "quic", so the chain's stated
// dimension is satisfied and the eligible indexed chain pre-empts the
// last-resort default slot.
//
// 🔴 THIS ARM IS RED AT THE UN-FIXED TIP. quicChain() is nullary and returns
// rt.defaultChain unconditionally when a default slot exists (quic.go:74-81),
// so the tip serves through the default slot and never looks at the indexed
// chain's transport_protocol at all.
//
// ⚠️ ARM (d) ALONE CANNOT TELL "THE CONSTANT WAS STAMPED AND MATCHED" FROM
// "THE DIMENSION IS UNENFORCED". A selector that never populates
// ChainMatchInputs.TransportProtocol at all — leaving it "" — makes
// chainmatch.go:128 (`c.TransportProtocol != "" && c.TransportProtocol !=
// inputs.TransportProtocol`) fire and the chain INELIGIBLE, so arm (d) alone
// distinguishes "stamped" from "not stamped" only in the negative direction.
// What it cannot distinguish is the opposite failure: a selector that DROPS
// the transport_protocol comparison entirely (or a ChainSpec whose
// TransportProtocol was never parsed through, leaving it "") makes EVERY chain
// eligible on this dimension, and arm (d) is green under that too, because
// "quic" was the right answer anyway. Arm (e) is the half that catches it —
// its listener is byte-identical except for the single string `"tls"`, and it
// is only correct if a NON-matching value actually excludes the chain. The
// pair is the gate; arm (d) alone is not.
//
// ⚠️ THE ASYMMETRY THAT MAKES THIS DIMENSION INTERESTING, AND WHY THE COMING
// FIX MUST STAMP THE CONSTANT. `matches` compares by exact, case-sensitive
// `!=` (chainmatch.go:128). The CHAIN's empty string means "unspecified" and
// skips the dimension — but there is NO REVERSE WILDCARD: an empty
// *inputs*.TransportProtocol is just a value that nothing equals. So a chain
// spelling `"quic"` against an unset input is INELIGIBLE, not
// universally-eligible. The QUIC selector therefore cannot leave the input
// blank and rely on a wildcard; it must stamp the literal constant "quic", or
// every `transport_protocol: "quic"` chain in every QUIC listener silently
// falls through to the default slot — which is exactly the shape of the bug
// this phase pins.
//
// PARSE PRECONDITION: `parseChainSpec`'s enum gate accepts exactly
// {"", "tls", "raw_buffer", "quic"} (manager.go:985-991), so NEITHER arm may
// boot-reject and both are genuine runtime arms. This is verified by BUILDING
// both listeners — each arm's NewManager t.Fatalf names a boot-reject
// explicitly — not by reading the switch.
func TestQUICChainSelection_TransportProtocolQUICMatches(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	fcm := &listenerv3.FilterChainMatch{TransportProtocol: "quic"}
	l := mkQUICListenerChains(t, fcm, "FC0\n", true, true, "DFC\n")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, transport_protocol=%q filter_chains[0] + QUIC-TLS default slot) BOOT-REJECTED: %v — parseChainSpec's enum gate must accept %q, or this arm is not a runtime arm at all", "quic", err, "quic")
	}
	rt := mgr.runtimes[0]

	idxKey := l.Name + "/filter_chains[0]"
	dfcKey := l.Name + "/default_filter_chain"
	indexed := rt.chainByName[idxKey]
	dflt := rt.chainByName[dfcKey]
	if indexed == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", idxKey, quicChainKeys(rt))
	}
	if dflt == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", dfcKey, quicChainKeys(rt))
	}
	if indexed == dflt {
		t.Fatalf("precondition: chainByName[%q] and chainByName[%q] are the SAME *chainInfo (%p) — a pointer-equality assertion would be vacuous", idxKey, dfcKey, indexed)
	}
	// PRECONDITION — the string really reached the parsed spec. A spec whose
	// TransportProtocol was silently dropped to "" would make this arm pass
	// for the wrong reason: chainmatch.go:128 skips the dimension on "", so
	// the chain would be eligible regardless of what the selector stamps.
	if got := quicChainSpecByName(t, rt, idxKey).TransportProtocol; got != "quic" {
		t.Fatalf("precondition: chainSpecs[%q].TransportProtocol = %q, want %q — the configured value must survive parseChainSpec or this arm tests nothing", idxKey, got, "quic")
	}

	got := rt.quicChain(nil)

	// PROPERTY 1 — matching transport_protocol selects the indexed chain.
	if got != indexed {
		t.Errorf("transport_protocol match selects indexed: quicChain() = %p, want filter_chains[0] = %p (a QUIC connection's transport protocol is %q, which this chain names, so the chain is eligible)", got, indexed, "quic")
	}

	// PROPERTY 2 — the eligible chain pre-empts the last-resort slot. Stated
	// separately so a failure names the default_filter_chain rather than
	// collapsing "wrong chain" and "fell through to last resort" onto one line.
	if got == dflt {
		t.Errorf("eligible chain pre-empts last resort: quicChain() returned the default_filter_chain %p, but filter_chains[0] %p names transport_protocol %q and is eligible, so the last-resort slot must not be consulted", dflt, indexed, "quic")
	}
}

// TestQUICChainSelection_TransportProtocolTLSDoesNotMatch is phase-97 arm (e):
// arm (d) with the single string `"quic"` replaced by `"tls"`. Everything else
// about the two listeners is byte-identical.
//
// PROPERTY: (*listenerRuntime).quicChain() must select the
// default_filter_chain. A QUIC connection's transport protocol is "quic", not
// "tls"; `matches` compares by exact case-sensitive `!=` (chainmatch.go:128),
// so filter_chains[0] is INELIGIBLE and SelectChain's empty-eligible branch
// hands back the default slot.
//
// 🟢 THIS ARM IS GREEN AT THE UN-FIXED TIP, AND THAT IS NOT COVERAGE.
// quicChain() is nullary and returns rt.defaultChain unconditionally
// (quic.go:74-81) — it evaluates no dimension of any filter_chain_match. Arm
// (e)'s correct answer happens to BE the default slot, so the subject agrees
// with the reference for a reason the mechanism cannot supply. A reader who
// sees this arm pass before the fix MUST NOT read that as the
// transport_protocol dimension being enforced. Its only falsifiability arrives
// later, from a negative-control roster row.
//
// ⚠️ WHAT THE (d)/(e) PAIR RULES OUT. Arm (d) alone is green under a selector
// that never compares transport_protocol at all (everything eligible, "quic"
// was right anyway); arm (e) alone is green under a selector that never
// selects an indexed chain (which is the tip). Only the pair — one field
// apart, `"quic"` vs `"tls"` — excludes both, because no constant answer and
// no dropped comparison is right on both arms at once.
//
// PARSE PRECONDITION: `"tls"` is in parseChainSpec's accepted enum domain
// alongside `"quic"` (manager.go:985-991), so this arm boot-builds too and is
// a genuine runtime arm rather than a rejection pin. Verified by BUILDING the
// listener, not by reading the switch.
func TestQUICChainSelection_TransportProtocolTLSDoesNotMatch(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	fcm := &listenerv3.FilterChainMatch{TransportProtocol: "tls"}
	l := mkQUICListenerChains(t, fcm, "FC0\n", true, true, "DFC\n")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, transport_protocol=%q filter_chains[0] + QUIC-TLS default slot) BOOT-REJECTED: %v — parseChainSpec's enum gate must accept %q, or this arm is a rejection pin and not a runtime arm", "tls", err, "tls")
	}
	rt := mgr.runtimes[0]

	idxKey := l.Name + "/filter_chains[0]"
	dfcKey := l.Name + "/default_filter_chain"
	indexed := rt.chainByName[idxKey]
	dflt := rt.chainByName[dfcKey]
	if indexed == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", idxKey, quicChainKeys(rt))
	}
	if dflt == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", dfcKey, quicChainKeys(rt))
	}
	if indexed == dflt {
		t.Fatalf("precondition: chainByName[%q] and chainByName[%q] are the SAME *chainInfo (%p) — a pointer-equality assertion would be vacuous", idxKey, dfcKey, indexed)
	}
	// PRECONDITION — the string really reached the parsed spec. A spec whose
	// TransportProtocol was silently dropped to "" would make the chain
	// universally eligible (chainmatch.go:128 skips ""), which is the opposite
	// of what this arm needs.
	if got := quicChainSpecByName(t, rt, idxKey).TransportProtocol; got != "tls" {
		t.Fatalf("precondition: chainSpecs[%q].TransportProtocol = %q, want %q — the configured value must survive parseChainSpec or this arm's ineligibility is fictional", idxKey, got, "tls")
	}

	got := rt.quicChain(nil)

	// PROPERTY 1 — non-matching transport_protocol falls back to the default
	// slot.
	if got != dflt {
		t.Errorf("transport_protocol mismatch falls back: quicChain() = %p, want default_filter_chain = %p (a QUIC connection's transport protocol is %q, not %q, and the comparison is exact and case-sensitive)", got, dflt, "quic", "tls")
	}

	// PROPERTY 2 — the non-matching chain is not selected. Stated separately
	// so a failure names filter_chains[0] and the offending value rather than
	// collapsing "wrong chain" and "an excluded chain was selected".
	if got == indexed {
		t.Errorf("non-matching chain not selected: quicChain() returned filter_chains[0] %p, whose filter_chain_match names transport_protocol %q — a QUIC connection is %q and there is no reverse wildcard, so this chain must be ineligible", indexed, "tls", "quic")
	}
}

// quicChainSpecByName returns the parsed *listenerfilter.ChainSpec whose Name
// is `name`, for the phase-97 chain-selection arms' parse preconditions.
// t.Fatalf's if absent: a missing spec means the arm's premise about what was
// configured is wrong, which is a broken precondition and not a property
// failure.
//
// It exists because chainInfo carries NO match state at all — the parsed match
// dimensions live in rt.chainSpecs, keyed by the same "<listener>/filter_chains[i]"
// spelling used for rt.chainByName. An arm that wants to prove its configured
// filter_chain_match survived parseChainSpec must look there, not at the
// *chainInfo it asserts identity against.
func quicChainSpecByName(t *testing.T, rt *listenerRuntime, name string) *listenerfilter.ChainSpec {
	t.Helper()
	for _, s := range rt.chainSpecs {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("precondition: no chainSpec named %q in rt.chainSpecs (%d specs)", name, len(rt.chainSpecs))
	return nil
}

// TestQUICChainSelection_ApplicationProtocolsH3Matches is phase-97 arm (f),
// the POSITIVE half of the application_protocols pair completed by
// TestQUICChainSelection_ApplicationProtocolsH2DoesNotMatch (arm (g)).
//
// SHAPE: one QUIC listener whose filter_chains[0] carries
// `application_protocols: ["h3"]`, plus a QUIC-TLS default_filter_chain.
//
// PROPERTY: (*listenerRuntime).quicChain() must select filter_chains[0]. Any
// connection that reaches the QUIC serve path negotiated "h3", so the chain's
// stated ALPN dimension is satisfied (chainmatch.go:131 via alpnMatchAny) and
// the eligible indexed chain pre-empts the last-resort default slot.
//
// 🔴 THIS ARM IS RED AT THE UN-FIXED TIP. quicChain() is nullary and returns
// rt.defaultChain unconditionally when a default slot exists (quic.go:74-81);
// it never reads any chain's application_protocols.
//
// 🔴 ON QUIC THIS INPUT IS A LISTENER CONSTANT, NOT THE CLIENT'S OFFER LIST —
// AND THAT IS A DIFFERENT MECHANISM FROM THE TCP PATH'S.
// On TCP, ChainMatchInputs.ApplicationProtocols is populated by the
// tls_inspector listener filter from the ClientHello's ALPN extension: it is
// the client's full OFFER LIST, several entries wide, and `alpnMatchAny`
// (chainmatch.go:293-302) is a genuine any-of intersection over it.
// On QUIC there is no ClientHello inspection at all. The handshake is already
// complete when quic-go's Accept returns, and the only ALPN fact available is
// crypto/tls.ConnectionState.NegotiatedProtocol — a SCALAR, the single
// negotiated value, not a list. mkQUICListenerChains hard-wires the QUIC
// config's NextProtos to exactly ["h3"], so the negotiation has exactly one
// possible outcome and every connection reaching serveQUICConnection
// negotiated "h3" BY CONSTRUCTION. These arms therefore agree with the pinned
// Envoy reference by a DIFFERENT mechanism than the TCP arms do: a one-element
// input derived from a listener constant, not an intersection against a
// client-supplied set.
//
// ⚠️ A FUTURE ROW ADDING A SECOND QUIC ALPN WOULD BREAK THAT CONSTANT
// SILENTLY. The moment NextProtos becomes e.g. ["h3", "h3-29"], the negotiated
// value is no longer determined by the listener config, arm (g)'s "h2" is no
// longer the only non-negotiable value, and — worse — arm (f) keeps passing
// while no longer testing what its name says, because "h3" remains ONE of the
// possible outcomes rather than THE outcome. Nothing in these arms fails when
// that happens. Any row that widens the QUIC ALPN list MUST revisit this pair
// and re-derive what the negotiated scalar can be.
func TestQUICChainSelection_ApplicationProtocolsH3Matches(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	fcm := &listenerv3.FilterChainMatch{ApplicationProtocols: []string{"h3"}}
	l := mkQUICListenerChains(t, fcm, "FC0\n", true, true, "DFC\n")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, application_protocols=[%q] filter_chains[0] + QUIC-TLS default slot): %v", "h3", err)
	}
	rt := mgr.runtimes[0]

	idxKey := l.Name + "/filter_chains[0]"
	dfcKey := l.Name + "/default_filter_chain"
	indexed := rt.chainByName[idxKey]
	dflt := rt.chainByName[dfcKey]
	if indexed == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", idxKey, quicChainKeys(rt))
	}
	if dflt == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", dfcKey, quicChainKeys(rt))
	}
	if indexed == dflt {
		t.Fatalf("precondition: chainByName[%q] and chainByName[%q] are the SAME *chainInfo (%p) — a pointer-equality assertion would be vacuous", idxKey, dfcKey, indexed)
	}
	// PRECONDITION — the configured list survived parseChainSpec. An empty
	// ApplicationProtocols would SKIP the dimension entirely
	// (chainmatch.go:131), making the chain universally eligible and this arm
	// green for the wrong reason.
	if got := quicChainSpecByName(t, rt, idxKey).ApplicationProtocols; len(got) != 1 || got[0] != "h3" {
		t.Fatalf("precondition: chainSpecs[%q].ApplicationProtocols = %v, want [\"h3\"] — the configured list must survive parseChainSpec or this arm tests nothing", idxKey, got)
	}
	// PRECONDITION — the listener's ALPN really is the single value "h3", so
	// the negotiated scalar this arm reasons about has exactly one possible
	// outcome. If a row widens NextProtos, this fires instead of the arm
	// passing while silently no longer testing its own claim.
	if got := dflt.tlsCfg.NextProtos; len(got) != 1 || got[0] != "h3" {
		t.Fatalf("precondition: default_filter_chain tlsCfg.NextProtos = %v, want exactly [\"h3\"] — a QUIC connection's ApplicationProtocols input is the NEGOTIATED SCALAR, so a widened list breaks this arm's premise", got)
	}

	got := rt.quicChain(nil)

	// PROPERTY 1 — matching ALPN selects the indexed chain.
	if got != indexed {
		t.Errorf("ALPN match selects indexed: quicChain() = %p, want filter_chains[0] = %p (a connection reaching the QUIC serve path negotiated %q, which this chain names, so the chain is eligible)", got, indexed, "h3")
	}

	// PROPERTY 2 — the eligible chain pre-empts the last-resort slot. Stated
	// separately so a failure names the default_filter_chain rather than
	// collapsing "wrong chain" and "fell through to last resort".
	if got == dflt {
		t.Errorf("eligible chain pre-empts last resort: quicChain() returned the default_filter_chain %p, but filter_chains[0] %p names application_protocols [%q] and is eligible, so the last-resort slot must not be consulted", dflt, indexed, "h3")
	}
}

// TestQUICChainSelection_ApplicationProtocolsH2DoesNotMatch is phase-97 arm
// (g): arm (f) with the single ALPN string `"h3"` replaced by `"h2"`.
// Everything else about the two listeners is byte-identical.
//
// PROPERTY: (*listenerRuntime).quicChain() must select the
// default_filter_chain. The listener's QUIC config offers exactly ["h3"], so
// no connection on this listener can have negotiated "h2"; `alpnMatchAny`
// finds no intersection (chainmatch.go:131, 293-302) and filter_chains[0] is
// INELIGIBLE, so SelectChain's empty-eligible branch hands back the default
// slot.
//
// 🟢 THIS ARM IS GREEN AT THE UN-FIXED TIP, AND THAT IS NOT COVERAGE.
// quicChain() is nullary and returns rt.defaultChain unconditionally
// (quic.go:74-81) — it evaluates no dimension of any filter_chain_match. Arm
// (g)'s correct answer happens to BE the default slot, so the subject agrees
// with the reference for a reason the mechanism cannot supply. A reader who
// sees this arm pass before the fix MUST NOT read that as the
// application_protocols dimension being enforced. Its only falsifiability
// arrives later, from a negative-control roster row.
//
// ⚠️ WHAT THE (f)/(g) PAIR RULES OUT. Arm (f) alone is green under a selector
// that never compares application_protocols at all (everything eligible, "h3"
// was right anyway); arm (g) alone is green under a selector that never
// selects an indexed chain (which is the tip). Only the pair — one ALPN string
// apart — excludes both.
//
// 🔴 SAME MECHANISM CAVEAT AS ARM (f), RESTATED BECAUSE IT IS LOAD-BEARING
// HERE TOO: on QUIC the ApplicationProtocols input is the listener's own
// NEGOTIATED CONSTANT, not a client offer list.
// crypto/tls.ConnectionState.NegotiatedProtocol is a SCALAR and the QUIC
// config's NextProtos is exactly ["h3"], so "h2" is unreachable BY
// CONSTRUCTION rather than by a client declining to offer it. That is a
// different mechanism from the TCP path's tls_inspector-derived offer list,
// and **a future row adding a second QUIC ALPN would break the constant
// silently** — "h2" could then become negotiable, and this arm would flip from
// "structurally impossible" to "happens not to occur" with no test failing to
// announce it. The NextProtos precondition below is the tripwire.
func TestQUICChainSelection_ApplicationProtocolsH2DoesNotMatch(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	fcm := &listenerv3.FilterChainMatch{ApplicationProtocols: []string{"h2"}}
	l := mkQUICListenerChains(t, fcm, "FC0\n", true, true, "DFC\n")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, application_protocols=[%q] filter_chains[0] + QUIC-TLS default slot): %v", "h2", err)
	}
	rt := mgr.runtimes[0]

	idxKey := l.Name + "/filter_chains[0]"
	dfcKey := l.Name + "/default_filter_chain"
	indexed := rt.chainByName[idxKey]
	dflt := rt.chainByName[dfcKey]
	if indexed == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", idxKey, quicChainKeys(rt))
	}
	if dflt == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", dfcKey, quicChainKeys(rt))
	}
	if indexed == dflt {
		t.Fatalf("precondition: chainByName[%q] and chainByName[%q] are the SAME *chainInfo (%p) — a pointer-equality assertion would be vacuous", idxKey, dfcKey, indexed)
	}
	// PRECONDITION — the configured list survived parseChainSpec. An empty
	// ApplicationProtocols would SKIP the dimension (chainmatch.go:131),
	// making the chain universally eligible, which is the opposite of what
	// this arm needs.
	if got := quicChainSpecByName(t, rt, idxKey).ApplicationProtocols; len(got) != 1 || got[0] != "h2" {
		t.Fatalf("precondition: chainSpecs[%q].ApplicationProtocols = %v, want [\"h2\"] — the configured list must survive parseChainSpec or this arm's ineligibility is fictional", idxKey, got)
	}
	// PRECONDITION / TRIPWIRE — the listener offers exactly ["h3"], so "h2" is
	// structurally unreachable. A row that widens NextProtos to include "h2"
	// (or anything else) must fail HERE rather than leave this arm quietly
	// asserting something that is no longer true.
	if got := dflt.tlsCfg.NextProtos; len(got) != 1 || got[0] != "h3" {
		t.Fatalf("precondition: default_filter_chain tlsCfg.NextProtos = %v, want exactly [\"h3\"] — this arm's premise is that %q is UNNEGOTIABLE on this listener, which a widened ALPN list silently destroys", got, "h2")
	}

	got := rt.quicChain(nil)

	// PROPERTY 1 — non-matching ALPN falls back to the default slot.
	if got != dflt {
		t.Errorf("ALPN mismatch falls back: quicChain() = %p, want default_filter_chain = %p (this listener offers only %q, so no connection negotiated %q and filter_chains[0] is ineligible)", got, dflt, "h3", "h2")
	}

	// PROPERTY 2 — the non-matching chain is not selected. Stated separately
	// so a failure names filter_chains[0] and the offending ALPN rather than
	// collapsing "wrong chain" and "an excluded chain was selected".
	if got == indexed {
		t.Errorf("non-matching chain not selected: quicChain() returned filter_chains[0] %p, whose filter_chain_match names application_protocols [%q] — the negotiated protocol on this listener is always %q, so this chain must be ineligible", indexed, "h2", "h3")
	}
}

// quicSNIAlpha is the SNI the phase-97 driven server_names arms (h) and (i)
// stamp on filter_chains[0].filter_chain_match.server_names, and the name the
// MATCHING client (arm h) puts in its ClientHello.
//
// It is testAlphaCertPEM's ONE SAN, verified with `openssl x509 -ext
// subjectAltName`: subject CN = alpha.envoy-go.test, SAN DNS:alpha.envoy-go.test,
// valid 2026-01-01 .. 2046-01-01. That matters only for arm (h); arm (i) dials
// a name the cert does NOT carry and stays green through InsecureSkipVerify
// (see quicSNIOther).
const quicSNIAlpha = "alpha.envoy-go.test"

// quicSNIOther is the NON-matching name arm (i) sends. It is deliberately NOT a
// SAN of testAlphaCertPEM, and NOT a suffix/wildcard relative to quicSNIAlpha —
// chainmatch.go's sniMatchAny accepts exact names, "*.suffix" patterns and "*",
// so a name like "sub.alpha.envoy-go.test" would be ambiguous about WHICH rule
// excluded it. "other.envoy-go.test" shares only the registrable suffix and
// matches under none of the three rules.
const quicSNIOther = "other.envoy-go.test"

// mkH3ClientTLS builds the client-side *stdtls.Config that the phase-97 driven
// SNI arms dial with. It exists so the SNI-transmission control
// (assertH3ClientTransmitsSNI) and the arms themselves provably share ONE
// client shape — a control that proves a DIFFERENT config transmits SNI proves
// nothing about the config the arm dials with.
//
// 🔴 ServerName IS THE POINT, AND IT IS ABSENT EVERYWHERE ELSE IN THIS PACKAGE.
// All three pre-existing HTTP/3 client sites — driveH3 (quic_test.go:97), the
// inline http3.Transport in TestQUICListener_ServesH3GET (quic_test.go:~195)
// and h3GetHealth (quic_negative_test.go:61) — construct
// `&stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true}` with
// NO ServerName, and every one of them dials a `127.0.0.1:<port>` literal.
// crypto/tls omits the SNI extension entirely for an IP-literal server name
// (RFC 6066 §3 forbids literal addresses in server_name), so those clients send
// NO SNI AT ALL. An arm that asserted server_names behavior while dialing
// through one of them would be measuring the empty string, not a name — a
// vacuous green in arm (i)'s direction and an unexplainable red in arm (h)'s.
// http3.Transport preserves a non-empty ServerName (quic-go v0.54.1
// http3/transport.go:340, `if tlsConf.ServerName == ""`), so setting it here
// reaches the ClientHello.
//
// ⚠️ InsecureSkipVerify is MANDATORY for arm (i), not merely convenient. Arm (i)
// sends a name the server's only certificate does not carry. With verification
// on, the client would abort at CERTIFICATE VALIDATION — which is a handshake
// outcome, not a chain-selection outcome — and the arm would "pass" for a reason
// that has nothing to do with filter_chain_match. Skipping verification is what
// lets a non-matching SNI complete the handshake and fall through to the chain
// selector, which is the only thing arm (i) is about.
func mkH3ClientTLS(sni string) *stdtls.Config {
	return &stdtls.Config{
		ServerName:         sni,
		NextProtos:         []string{"h3"},
		InsecureSkipVerify: true, //nolint:gosec // local test: arm (i) dials a name the test cert does not carry
	}
}

// h3GetChainBody performs ONE H3 GET /health against addr with the given SNI
// and returns the response status and body.
//
// ⚠️ IT RETURNS THE BODY, and callers assert it. mkHCMFilterQUICChain gives the
// two chains DIFFERENT direct_response bodies and the SAME status (222), so the
// body is the ONLY observable that names WHICH chain served. "the dial returned"
// answers "did the handshake complete", which on this listener is true no matter
// which chain is selected — the TLS config and the filter chain are drawn from
// two different accessors.
//
// Transport/dial failures are t.Fatalf PRECONDITIONS: without a completed round
// trip there is no body, and a body assertion on "" would be measuring the
// failure, not the selection.
func h3GetChainBody(t *testing.T, ctx context.Context, addr, sni string) (int, string) {
	t.Helper()
	tr := &http3.Transport{TLSClientConfig: mkH3ClientTLS(sni), QUICConfig: &quic.Config{}}
	defer func() { _ = tr.Close() }()
	client := &http.Client{Transport: tr}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+"/health", nil)
	if err != nil {
		t.Fatalf("precondition: NewRequestWithContext(sni=%q): %v", sni, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("precondition: H3 GET https://%s/health with SNI %q: %v", addr, sni, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("precondition: read body (sni=%q): %v", sni, err)
	}
	return resp.StatusCode, string(body)
}

// assertH3ClientTransmitsSNI proves that the client shape mkH3ClientTLS returns
// actually PUTS `sni` on the wire, by standing a throwaway quic-go listener
// whose *stdtls.Config records ClientHelloInfo.ServerName and dialing it with
// that exact shape.
//
// ⚠️ WHY THIS CONTROL EXISTS. "I set a struct field" is not "the bytes went
// out". crypto/tls silently drops the server_name extension for IP literals,
// http3.Transport rewrites an EMPTY ServerName from the URL host, and both
// behaviors are invisible from the arm's own assertions: at the un-fixed tip
// quicChain() consults nothing, so NEITHER arm (h) nor arm (i) can distinguish
// "the SNI arrived and was ignored" from "no SNI was ever sent". Without this
// control, arm (h)'s red and arm (i)'s green are both explainable by a client
// that transmits nothing. This is the only place in the phase where the SNI is
// observed by a server, and it is observed OUTSIDE the subject on purpose — the
// subject's production code is not touched by this phase.
//
// It is a PRECONDITION (t.Fatalf), not a property: if the client sends no name,
// arm (h) is not a failing server_names test, it is a broken probe.
func assertH3ClientTransmitsSNI(t *testing.T, ctx context.Context, sni string) {
	t.Helper()
	cert, err := stdtls.X509KeyPair([]byte(testAlphaCertPEM), []byte(testAlphaKeyPEM))
	if err != nil {
		t.Fatalf("precondition: X509KeyPair(testAlpha): %v", err)
	}
	seen := make(chan string, 4)
	srvTLS := &stdtls.Config{
		Certificates: []stdtls.Certificate{cert},
		NextProtos:   []string{"h3"},
		MinVersion:   stdtls.VersionTLS13,
		GetConfigForClient: func(chi *stdtls.ClientHelloInfo) (*stdtls.Config, error) {
			select {
			case seen <- chi.ServerName:
			default:
			}
			return nil, nil // nil config = "use the one I was called on"
		},
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("precondition: ListenPacket(udp, 127.0.0.1:0) for the SNI control: %v", err)
	}
	ql, err := quic.Listen(pc, srvTLS, &quic.Config{})
	if err != nil {
		_ = pc.Close()
		t.Fatalf("precondition: quic.Listen for the SNI control: %v", err)
	}
	defer func() { _ = ql.Close() }()
	// Accept without closing: quic-go completes the handshake off the accept
	// path, but draining the queue keeps the connection from lingering, and NOT
	// closing it avoids racing the client's DialAddr return.
	go func() { _, _ = ql.Accept(ctx) }()

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := quic.DialAddr(dialCtx, ql.Addr().String(), mkH3ClientTLS(sni), &quic.Config{})
	if err != nil {
		t.Fatalf("precondition: SNI control dial to the throwaway listener: %v", err)
	}
	defer func() { _ = conn.CloseWithError(0, "") }()

	select {
	case got := <-seen:
		if got != sni {
			t.Fatalf("precondition: the ClientHello carried ServerName %q, want %q — mkH3ClientTLS does not transmit the SNI the driven arms depend on", got, sni)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("precondition: no ClientHello was observed by the SNI control listener within 5s")
	}
}

// assertQUICAcceptCounterPointers is the pre-traffic pointer gate for the
// phase-97 DRIVEN arms (h) and (i).
//
// 🔴 IT MUST RUN BEFORE THE FIRST BYTE CROSSES THE WIRE. There is no recover()
// anywhere in non-test internal/listener, and quicAcceptLoop (quic.go:102-103)
// dereferences rt.downstreamCxTotal and rt.downstreamCxActive on the ACCEPT
// GOROUTINE for every connection. A nil there is a PROCESS CRASH that takes the
// whole test binary with it, not a failing test — so the pointers are checked
// while the listener is still idle.
//
// ⚠️ DEPARTURE FROM assertDefaultChainTLSPosture (manager_test.go:6412), stated
// rather than silently copied. That helper asserts rt.tlsMode plus the FIVE
// ssl.* pointers, because on the TCP path serveConnection Inc's them. On the
// QUIC path it does NOT apply: Manager.Start never launches serveConnection for
// kindQUIC, so no ssl.* counter is ever Inc'd on a QUIC listener — that is
// exactly what TestQUICListener_RegistersSSLNamesAtZero pins (all five
// registered, all five still zero after a real H3 round trip). Asserting the
// five here would be asserting pointers nothing dereferences: a green that
// buys no crash safety. The two counters below are the ones the QUIC accept
// goroutine actually touches, and they are registered UNCONDITIONALLY by
// registerListenerMetrics (manager.go:409-410, above the `if rt.tlsMode`
// gate), so this gate is not a restatement of the tlsMode computation either.
//
// ⚠️ t.Fatalf, not t.Errorf — the opposite of assertDefaultChainTLSPosture's
// choice, and deliberately. There, ordering is the diagnostic and the crash is
// the point being demonstrated. Here a nil pointer means the arm cannot be
// driven at all, and stopping is what PREVENTS the crash from erasing every
// other arm's result in the same binary.
func assertQUICAcceptCounterPointers(t *testing.T, rt *listenerRuntime) {
	t.Helper()
	if rt.downstreamCxTotal == nil {
		t.Fatalf("precondition: rt.downstreamCxTotal = nil before any connection — quicAcceptLoop Inc's it on the accept goroutine and a nil *stats.Counter Inc CRASHES the test binary")
	}
	if rt.downstreamCxActive == nil {
		t.Fatalf("precondition: rt.downstreamCxActive = nil before any connection — quicAcceptLoop Inc's and serveQUICConnection Dec's it on goroutines with no recover()")
	}
}

// TestQUICChainSelection_MatchingServerNameSelectsIndexedChain is phase-97
// arm (h).
//
// SHAPE: one QUIC listener whose filter_chains[0] carries
// `server_names: ["alpha.envoy-go.test"]`, plus a QUIC-TLS
// default_filter_chain. A real HTTP/3 client dials the bound UDP address
// sending exactly that name as SNI.
//
// PROPERTY: the connection must be served by filter_chains[0]. The SNI matches
// its server_names, so the chain is eligible (chainmatch.go:125,
// `len(c.ServerNames) > 0 && !sniMatchAny(...)`), and an eligible indexed chain
// pre-empts the last-resort default slot. Observed through the response BODY:
// filter_chains[0]'s direct_response returns "FC0\n" and the default slot's
// returns "DFC\n" (mkHCMFilterQUICChain), both under status 222.
//
// 🔴 THIS ARM CANNOT BE A NIL-CONN ARM, unlike arms (a)-(g). ServerName exists
// only on a real ClientHello: there is no production path that hands
// quicChain() an SNI without a connection, so a synthesized one would be an
// INVENTED INPUT — it would measure a function signature this phase has not
// landed, not a behavior the listener has. That is why this arm Starts the
// manager and drives real H3 traffic, and why its failure mode is a wrong BODY
// rather than a wrong pointer.
//
// 🔴 EXPECTED RED AT THE UN-FIXED TIP. quicChain() (quic.go:74-81) is nullary
// and returns rt.defaultChain whenever it is non-nil, so this connection is
// served by the default slot and the body comes back "DFC\n".
//
// ⚠️ WHAT IS AND IS NOT PROVEN HERE. The response body proves WHICH CHAIN
// SERVED — not merely that a handshake completed, which on this listener is
// true under either selection because the TLS config comes from quicTLSConfig()
// and the filters come from quicChain(). The SNI-transmission control
// (assertH3ClientTransmitsSNI) separately proves the name reached A server's
// ClientHello. What NO arm in this phase can prove at the un-fixed tip is that
// the name reached THIS listener's selector, because at this tip the selector
// reads nothing at all; that becomes observable only once the fix lands, and
// arm (i) is the matched negative that will then rule out a constant answer.
func TestQUICChainSelection_MatchingServerNameSelectsIndexedChain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// PRECONDITION, before anything is built: the client shape this arm dials
	// with actually transmits the SNI. Without this the arm's red is
	// indistinguishable from "no name was ever sent".
	assertH3ClientTransmitsSNI(t, ctx, quicSNIAlpha)

	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	fcm := &listenerv3.FilterChainMatch{ServerNames: []string{quicSNIAlpha}}
	l := mkQUICListenerChains(t, fcm, "FC0\n", true, true, "DFC\n")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManager(boot, cm, reg, testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, server_names=[%q] filter_chains[0] + QUIC-TLS default slot): %v", quicSNIAlpha, err)
	}
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("precondition: Start: %v", err)
	}
	defer mgr.Stop()

	rt := mgr.runtimes[0]
	// BEFORE THE FIRST BYTE.
	assertQUICAcceptCounterPointers(t, rt)

	infos := mgr.Listeners()
	if len(infos) != 1 {
		t.Fatalf("precondition: Listeners() = %d, want 1", len(infos))
	}
	addr := infos[0].Addr

	status, body := h3GetChainBody(t, ctx, addr, quicSNIAlpha)

	// PROPERTY 1 — the request was served by the chain terminal at all. 222 is
	// mkHCMFilterQUICChain's sentinel on BOTH chains, so this separates "a
	// direct_response route ran" from "some framework default answered"; it
	// does NOT name a chain.
	if status != 222 {
		t.Errorf("chain terminal reached: status = %d, want 222 (both chains' direct_response status) — a different status means no configured route answered", status)
	}

	// PROPERTY 2 — selection: filter_chains[0] served. This is the arm.
	if body != "FC0\n" {
		t.Errorf("SNI selection: body = %q, want %q — the ClientHello carried server_name %q which matches filter_chains[0].filter_chain_match.server_names, so the eligible indexed chain must serve, not the last-resort default slot", body, "FC0\n", quicSNIAlpha)
	}

	// PROPERTY 3 — last-resort ordering, stated separately so a failure names
	// the default_filter_chain explicitly instead of collapsing "wrong body"
	// and "the last-resort slot pre-empted an eligible indexed chain".
	if body == "DFC\n" {
		t.Errorf("last-resort ordering: the default_filter_chain served (body %q) while filter_chains[0] was ELIGIBLE for this connection (server_names matched SNI %q); default_filter_chain is consulted only when no indexed chain is eligible", body, quicSNIAlpha)
	}

	// PROPERTY 4 — the connection was accounted, proving the traffic above went
	// through THIS listener's accept path and not, say, a stale address.
	prefix := "listener." + normalizeAddr(addr) + "."
	if got := pollCounter(t, reg, prefix+"downstream_cx_total", 1, 2*time.Second); got < 1 {
		t.Errorf("accounting: %sdownstream_cx_total = %d, want >= 1", prefix, got)
	}
}

// TestQUICChainSelection_NonMatchingServerNameFallsBackToDefaultSlot is
// phase-97 arm (i): the MATCHED NEGATIVE of arm (h).
//
// SHAPE: BYTE-IDENTICAL listener config to arm (h) —
// mkQUICListenerChains(nil-differing only in nothing; the same fcm shape, the
// same bodies, the same default slot). The ONLY difference between the two arms
// is the name the client puts in its ClientHello: arm (h) sends
// "alpha.envoy-go.test", this arm sends "other.envoy-go.test".
//
// PROPERTY: the connection must be served by the default_filter_chain.
// filter_chains[0] requires server_names ["alpha.envoy-go.test"] and the SNI
// does not match under any of sniMatchAny's three rules (exact, "*.suffix",
// "*"), so the indexed chain is INELIGIBLE and selection resorts to the
// last-resort slot. Observed as body "DFC\n".
//
// 🟢 THIS ARM IS GREEN AT THE UN-FIXED TIP, AND THAT IS NOT COVERAGE — it is
// the false-agreement class, the same one arm (b) sits in. quicChain()
// (quic.go:74-81) is nullary at this tip:
//
//	if rt.defaultChain != nil { return rt.defaultChain }
//
// It answers "the default chain" on EVERY QUIC connection, having evaluated no
// dimension of any filter_chain_match and having never looked at the SNI. This
// arm's correct answer HAPPENS TO BE the default chain, so the subject agrees
// with the reference for a reason the mechanism cannot supply. A reader who
// sees this arm pass before the fix MUST NOT read it as the server_names
// dimension being enforced — nothing here is enforced.
//
// ⚠️ THE PAIR IS THE GATE, NOT EITHER ARM. Arm (h) alone is satisfiable by a
// selector that returns filter_chains[0] unconditionally. This arm alone is
// satisfiable by a selector that returns the default slot unconditionally —
// which is EXACTLY what the tip does. Only the pair, whose two LISTENERS are
// identical and whose two CLIENTS differ in one string, rules out both constant
// answers at once.
//
// ⚠️ THIS ARM MUST FAIL THE MATCH, NOT THE HANDSHAKE. The client keeps
// InsecureSkipVerify (mkH3ClientTLS): testAlphaCertPEM carries the single SAN
// "alpha.envoy-go.test", so a verifying client sending "other.envoy-go.test"
// would abort at certificate validation and never reach chain selection at all
// — the arm would report a green that came from a REJECTED handshake rather
// than from a fall-through. The server's certificate is unchanged and is not
// what this arm is about.
func TestQUICChainSelection_NonMatchingServerNameFallsBackToDefaultSlot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	assertH3ClientTransmitsSNI(t, ctx, quicSNIOther)

	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	fcm := &listenerv3.FilterChainMatch{ServerNames: []string{quicSNIAlpha}}
	l := mkQUICListenerChains(t, fcm, "FC0\n", true, true, "DFC\n")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManager(boot, cm, reg, testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, server_names=[%q] filter_chains[0] + QUIC-TLS default slot): %v", quicSNIAlpha, err)
	}
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("precondition: Start: %v", err)
	}
	defer mgr.Stop()

	rt := mgr.runtimes[0]
	// BEFORE THE FIRST BYTE.
	assertQUICAcceptCounterPointers(t, rt)

	infos := mgr.Listeners()
	if len(infos) != 1 {
		t.Fatalf("precondition: Listeners() = %d, want 1", len(infos))
	}
	addr := infos[0].Addr

	status, body := h3GetChainBody(t, ctx, addr, quicSNIOther)

	// PROPERTY 1 — the chain terminal was reached (see arm (h)).
	if status != 222 {
		t.Errorf("chain terminal reached: status = %d, want 222 (both chains' direct_response status)", status)
	}

	// PROPERTY 2 — fall-through: the default slot served.
	if body != "DFC\n" {
		t.Errorf("SNI fall-through: body = %q, want %q — the ClientHello carried server_name %q, which matches none of filter_chains[0].filter_chain_match.server_names [%q], so the indexed chain is ineligible and the last-resort default_filter_chain must serve", body, "DFC\n", quicSNIOther, quicSNIAlpha)
	}

	// PROPERTY 3 — the ineligible chain did NOT serve, named separately.
	if body == "FC0\n" {
		t.Errorf("ineligible chain served: body = %q — filter_chains[0] requires SNI %q and the connection presented %q; a chain whose server_names do not match must never be selected", body, quicSNIAlpha, quicSNIOther)
	}

	// PROPERTY 4 — the connection was accounted.
	prefix := "listener." + normalizeAddr(addr) + "."
	if got := pollCounter(t, reg, prefix+"downstream_cx_total", 1, 2*time.Second); got < 1 {
		t.Errorf("accounting: %sdownstream_cx_total = %d, want >= 1", prefix, got)
	}
}

// TestQUICChainSelection_TLSConfigAndChainAgreeOnOneChain is phase-97 arm (j):
// the ACCESSOR-IDENTITY pin.
//
// SHAPE: one QUIC listener carrying an EMPTY-MATCH (universally eligible)
// QUIC-TLS-wrapped filter_chains[0], plus a default_filter_chain with **NO
// transport_socket** — a PLAINTEXT default slot.
//
// PROPERTY: the two accessors quic.go exposes must describe ONE chain.
// serveQUICConnection draws the filter chain from quicChain() (quic.go:123) and
// hands http3.Server a TLSConfig from quicTLSConfig() (quic.go:144); startQUIC
// hands quic.Listen the same quicTLSConfig() (quic.go:32). If those two
// accessors resolve to DIFFERENT chains, the connection is terminated with one
// chain's certificate and served by another chain's filters — a split every
// other arm in this phase is blind to, because arms (a)-(i) exercise exactly
// one of the two accessors each.
//
// 🔴 THE PLAINTEXT DEFAULT SLOT IS MANDATORY, AND IT IS THE WHOLE ARM.
// quicTLSConfig() and quicChain() differ in EXACTLY ONE conjunct at this tip:
//
//	quicTLSConfig:  if rt.defaultChain != nil && rt.defaultChain.tlsCfg != nil
//	quicChain:      if rt.defaultChain != nil
//
// If BOTH slots carried TLS, that extra `tlsCfg != nil` would be true, both
// accessors would take their default-slot branch, and they would AGREE — the
// arm would be a VACUOUS GREEN at the un-fixed tip, proving nothing about
// either. Only a default slot whose tlsCfg is nil separates them. The
// preconditions below assert that separation rather than assuming it: a green
// from this arm is worthless unless `dflt.tlsCfg == nil` held when it ran.
//
// 🔴 THE INDEXED CHAIN MUST BE ELIGIBLE, for the mirror-image reason. With an
// INELIGIBLE filter_chains[0] the CORRECT post-fix answer is that quicChain()
// selects the default slot (nil TLS) while quicTLSConfig() still finds the
// indexed chain's config — the two accessors would legitimately disagree, and
// this pin would be RED AGAINST CORRECT CODE, which is exactly as bad as being
// vacuous. filter_chains[0] therefore carries NO filter_chain_match (fcm nil ⇒
// spec.Empty), and that is asserted as a precondition, not assumed.
//
// ⚠️ WHY THIS SHAPE BUILDS AT ALL — a known divergence CONSUMED as a fixture.
// A QUIC listener normally cannot express a plaintext chain: the
// filter_chains[] loop rejects a missing transport_socket outright with
// "quic listener requires a transport_socket (mandatory TLS)"
// (manager.go:658). That reject covers `filter_chains[i]` ONLY. The
// default_filter_chain branch carries no such kind check: its transport_socket
// branch is simply skipped, dfcTLS is left nil, and the listener builds. That
// asymmetry is the banked D2-QUICTS divergence — deliberately unrepaired — and
// this arm does not fix it, it USES it, because a TLS-less default slot is the
// only fixture that can separate the two accessors. Any future repair of
// D2-QUICTS must revisit this arm along with mkQUICListenerChains's
// `defaultTLS=false` callers.
//
// ⚠️ THE ASSERTION IS POINTER IDENTITY, NOT "BOTH NON-NIL". quicTLSConfig()
// returns a *stdtls.Config, NOT a *chainInfo, so the two accessors cannot be
// compared directly. The comparison is `rt.quicTLSConfig() == selected.tlsCfg`
// where `selected` is what quicChain() returned — the config pointer the
// listener will actually use against the config pointer belonging to the chain
// that will actually serve. A "both non-nil" check would pass on this very
// listener at the un-fixed tip in the fixed direction and prove nothing about
// agreement.
//
// 🔴 EXPECTED RED AT THE UN-FIXED TIP, with the split fully specified:
// quicChain() returns the default slot (defaultChain != nil), whose netChainFactory
// serves "DFC\n" and whose tlsCfg is NIL; quicTLSConfig() skips that slot on the
// tlsCfg conjunct and loops chainByName, returning filter_chains[0]'s config. So
// the TLS comes from filter_chains[0] and the FILTERS come from the default
// slot — nil == non-nil is false and the identity assertion fires.
func TestQUICChainSelection_TLSConfigAndChainAgreeOnOneChain(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListenerChains(t, nil, "FC0\n", true, false, "DFC\n")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, empty-match QUIC-TLS filter_chains[0] + PLAINTEXT default slot): %v", err)
	}
	rt := mgr.runtimes[0]

	idxKey := l.Name + "/filter_chains[0]"
	dfcKey := l.Name + "/default_filter_chain"
	indexed := rt.chainByName[idxKey]
	dflt := rt.chainByName[dfcKey]
	if indexed == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", idxKey, quicChainKeys(rt))
	}
	if dflt == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", dfcKey, quicChainKeys(rt))
	}
	if indexed == dflt {
		t.Fatalf("precondition: chainByName[%q] and chainByName[%q] are the SAME *chainInfo (%p) — every assertion below would be vacuous", idxKey, dfcKey, indexed)
	}

	// PRECONDITION — the SEPARATOR. If the default slot carried TLS, both
	// accessors would take their default-slot branch and AGREE at the un-fixed
	// tip; this arm would be a vacuous green.
	if dflt.tlsCfg != nil {
		t.Fatalf("precondition: the default_filter_chain's tlsCfg is NON-nil (%p) — with TLS in both slots quicTLSConfig() and quicChain() agree trivially at this tip and this pin proves NOTHING; the default slot must carry no transport_socket", dflt.tlsCfg)
	}
	// PRECONDITION — there must be a config to draw. A nil indexed tlsCfg would
	// make the identity assertion nil == nil, i.e. green for the wrong reason.
	if indexed.tlsCfg == nil {
		t.Fatalf("precondition: filter_chains[0].tlsCfg is nil — with no TLS anywhere the identity assertion degenerates to nil == nil")
	}
	// PRECONDITION — filter_chains[0] must be ELIGIBLE. With an ineligible
	// indexed chain the CORRECT post-fix answer is a legitimate disagreement
	// between the accessors, and this pin would fail against correct code.
	if len(rt.chainSpecs) != 1 {
		t.Fatalf("precondition: len(rt.chainSpecs) = %d, want 1", len(rt.chainSpecs))
	}
	if !rt.chainSpecs[0].Empty {
		t.Fatalf("precondition: chainSpecs[0] (%q) is NOT an empty match — an ineligible indexed chain makes a correct implementation disagree between the accessors, so this pin would be RED AGAINST CORRECT CODE", rt.chainSpecs[0].Name)
	}

	selected := rt.quicChain(nil)
	if selected == nil {
		t.Fatalf("precondition: quicChain() returned nil on a listener with two chains")
	}

	// PROPERTY 1 — ACCESSOR IDENTITY. The *stdtls.Config the listener will use
	// must be the one belonging to the chain that will serve. Pointer equality,
	// not "both non-nil".
	if got := rt.quicTLSConfig(); got != selected.tlsCfg {
		t.Errorf("accessor identity: quicTLSConfig() = %p but quicChain()'s chain carries tlsCfg %p — the connection would be TERMINATED with one chain's TLS config and SERVED by another chain's filters (filter_chains[0].tlsCfg=%p, default_filter_chain.tlsCfg=%p)", got, selected.tlsCfg, indexed.tlsCfg, dflt.tlsCfg)
	}

	// PROPERTY 2 — selection: the eligible empty-match filter_chains[0] must be
	// the chain that serves. Stated separately from property 1 so a failure
	// names WHICH chain was picked rather than only that the two accessors
	// disagreed; at the un-fixed tip both fire, and property 2 is what
	// identifies the default slot as the one supplying the filters.
	if selected != indexed {
		t.Errorf("selection identity: quicChain() = %p, want filter_chains[0] = %p (it is empty-match and therefore eligible for every connection; default_filter_chain %p is the LAST-RESORT slot)", selected, indexed, dflt)
	}
}

// TestQUICChainSelection_TLSConfigPrefersIndexedChainOverTLSDefaultSlot is
// phase-97 arm (k): the TWO-CANDIDATE half of the quicTLSConfig() ordering pin.
//
// SHAPE: one QUIC listener carrying an EMPTY-MATCH (universally eligible)
// QUIC-TLS-wrapped filter_chains[0] AND a QUIC-TLS default_filter_chain. BOTH
// slots carry a non-nil, DISTINCT *stdtls.Config.
//
// PROPERTY: rt.quicTLSConfig() must return the INDEXED chain's tlsCfg by
// POINTER. The Start-time chain selection (selectQUICChain(nil)) picks the
// eligible empty-match filter_chains[0], and the last-resort default slot must
// NOT pre-empt it — even though the default slot also carries a usable TLS
// config and would therefore satisfy a "first slot with non-nil tlsCfg" walk.
//
// 🔴 WHY THIS ARM EXISTS — ARM (j) CANNOT COVER THIS, BY CONSTRUCTION.
// Arm (j) (TestQUICChainSelection_TLSConfigAndChainAgreeOnOneChain)
// deliberately uses a PLAINTEXT default slot: its own precondition hard-
// requires dflt.tlsCfg == nil, because a TLS-bearing default slot would make
// quicTLSConfig() and quicChain() agree trivially and turn arm (j) into a
// vacuous green. But that same precondition means arm (j)'s listener has
// exactly ONE TLS-bearing candidate. Any resolution order whatsoever — default
// slot first, indexed chain first, map-order accident — returns the same
// pointer when only one candidate has a config. Arm (j) therefore agrees by
// SINGLE-CANDIDACY, not by ordering, and it stays green under a quicTLSConfig()
// body that consults the default slot FIRST.
//
// That is measured, not asserted: the ten-arm roster (a)-(j) went ALL-GREEN at
// Task 10, BEFORE Task 11 rewrote quicTLSConfig()'s resolution order, and it
// stays green if that rewrite is neutralized. Arm (k) supplies the missing
// two-candidate case, and it is the only arm in this file that reddens when the
// default slot is tried first.
//
// ⚠️ THE ANTI-VACUITY PRECONDITIONS ARE THE WHOLE POINT. Both tlsCfg pointers
// must be non-nil AND NOT EQUAL TO EACH OTHER. If the two slots shared one
// *stdtls.Config the pointer assertion would hold under every possible
// resolution order and this arm would prove exactly what arm (j) already
// cannot. mkQUICListenerChains builds the two transport sockets from separate
// mkQUICDownstreamTS calls, so the configs are distinct objects — but that is a
// property of a HELPER this arm does not own, so it is asserted here rather
// than assumed.
//
// ⚠️ filter_chains[0] must be ELIGIBLE (fcm nil ⇒ spec.Empty), asserted below.
// With an ineligible indexed chain the correct answer would be the default
// slot's config and this pin would be RED AGAINST CORRECT CODE.
func TestQUICChainSelection_TLSConfigPrefersIndexedChainOverTLSDefaultSlot(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListenerChains(t, nil, "FC0\n", true, true, "DFC\n")
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("precondition: NewManager(quic, empty-match QUIC-TLS filter_chains[0] + QUIC-TLS default slot): %v", err)
	}
	rt := mgr.runtimes[0]

	idxKey := l.Name + "/filter_chains[0]"
	dfcKey := l.Name + "/default_filter_chain"
	indexed := rt.chainByName[idxKey]
	dflt := rt.chainByName[dfcKey]
	if indexed == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", idxKey, quicChainKeys(rt))
	}
	if dflt == nil {
		t.Fatalf("precondition: chainByName[%q] is absent (keys: %v)", dfcKey, quicChainKeys(rt))
	}
	if indexed == dflt {
		t.Fatalf("precondition: chainByName[%q] and chainByName[%q] are the SAME *chainInfo (%p) — every assertion below would be vacuous", idxKey, dfcKey, indexed)
	}

	// PRECONDITION — TWO CANDIDATES. Both slots must carry a config, or the
	// arm degenerates into arm (j)'s single-candidate shape and stops
	// discriminating the resolution ORDER.
	if indexed.tlsCfg == nil {
		t.Fatalf("precondition: filter_chains[0].tlsCfg is nil — with no config on the indexed chain there is nothing for the default slot to pre-empt")
	}
	if dflt.tlsCfg == nil {
		t.Fatalf("precondition: the default_filter_chain's tlsCfg is nil — with only ONE TLS-bearing candidate every resolution order returns the same pointer and this arm is VACUOUS (that single-candidate shape is arm (j)'s, and it is exactly what this arm exists to complement)")
	}
	// PRECONDITION — ANTI-VACUITY. The two configs must be DIFFERENT objects.
	if indexed.tlsCfg == dflt.tlsCfg {
		t.Fatalf("precondition: filter_chains[0].tlsCfg and default_filter_chain.tlsCfg are the SAME *stdtls.Config (%p) — the pointer assertion below would hold under EVERY resolution order and prove nothing", indexed.tlsCfg)
	}
	// PRECONDITION — filter_chains[0] must be ELIGIBLE.
	if len(rt.chainSpecs) != 1 {
		t.Fatalf("precondition: len(rt.chainSpecs) = %d, want 1", len(rt.chainSpecs))
	}
	if !rt.chainSpecs[0].Empty {
		t.Fatalf("precondition: chainSpecs[0] (%q) is NOT an empty match — an ineligible indexed chain makes the default slot's config the CORRECT answer, so this pin would be RED AGAINST CORRECT CODE", rt.chainSpecs[0].Name)
	}

	// PROPERTY 1 — ORDERING. quicTLSConfig() must return the INDEXED chain's
	// config, not the default slot's.
	got := rt.quicTLSConfig()
	if got != indexed.tlsCfg {
		t.Errorf("quicTLSConfig ordering: got %p, want filter_chains[0].tlsCfg %p — filter_chains[0] is empty-match and therefore eligible for the Start-time selection, and the last-resort default_filter_chain (tlsCfg %p) must never pre-empt an eligible indexed chain", got, indexed.tlsCfg, dflt.tlsCfg)
	}

	// PROPERTY 2 — the default slot did NOT win, named separately so a failure
	// says WHICH slot supplied the config rather than only that it was wrong.
	if got == dflt.tlsCfg {
		t.Errorf("default slot pre-empted: quicTLSConfig() = %p = default_filter_chain.tlsCfg — the default slot was consulted BEFORE the Start-time chain selection, which is the pre-Task-11 resolution order this arm exists to exclude", got)
	}
}
