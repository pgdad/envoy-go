// Package driver registers the 0123-listener-transport-protocol fixture with
// the differential runner. See ../README.md for the fixture's purpose.
//
// THE PROPOSITION (phase 98, SPEC §7): on a PLAINTEXT TCP listener carrying NO
// listener filter at all, reference Envoy stamps the connection's
// transport_protocol as "raw_buffer" before filter-chain selection runs, so a
// `filter_chain_match: {transport_protocol: raw_buffer}` chain is ELIGIBLE and
// serves. envoy-go leaves the input empty, the exact-value branch in
// listenerfilter.SelectChain rejects the chain, and the listener falls through
// to its default chain. Separately, envoy-go BOOT-REJECTS a transport_protocol
// string outside {tls, raw_buffer, quic, ""} that the reference ACCEPTS, boots
// and serves.
//
// Three plaintext TCP listeners, each carrying ONE indexed chain whose
// filter_chain_match differs only in the transport_protocol STRING, plus a
// last-resort default_filter_chain:
//
//	listener   fc_indexed match                       expected BOTH sides
//	l_bogus    transport_protocol: totally_bogus_value  DEFAULT
//	l_raw      transport_protocol: raw_buffer           INDEXED
//	l_tls      transport_protocol: tls                  DEFAULT
//
// # ⚠️ TWO WAYS THIS FIXTURE SILENTLY DISARMS ITSELF
//
// (1) ADDING `tls_inspector` — OR ANY OTHER listener filter — TO ANY LISTENER
// SILENTLY DISARMS `l_raw`. With an inspector present, envoy-go's own pipeline
// stamps inputs.TransportProtocol ("raw_buffer" for a non-TLS preamble,
// per internal/listener/listenerfilter/tls_inspector), so BOTH sides answer
// INDEXED on l_raw EVEN AT THE UN-FIXED TIP (SPEC §2.1 T2). The fixture then
// runs GREEN over a live divergence in the exact dimension it claims to cover.
// The absence of `listener_filters` from all three listeners is therefore
// LOAD-BEARING, not an omission — do not add one "for realism".
//
// (2) `l_bogus` ALONE IS A FALSE-AGREEMENT ARM. A subject that parses the
// field and then IGNORES the dimension entirely answers DEFAULT on l_bogus
// too, for the wrong reason. Only the `l_raw` / `l_tls` PAIR — one string
// apart, same shape, opposite expected chains — excludes a constant answer:
// a subject stamping a constant "tls" passes l_tls and fails l_raw; one
// stamping a constant "raw_buffer" passes l_raw and fails l_tls; one stamping
// nothing fails l_raw alone. Deleting either half of that pair reduces the
// fixture to a shape that cannot distinguish "matched" from "unenforced".
//
// ⚠️ envoy-go emits NO `no_filter_chain_match` counter (the string has no
// production site in this repository), so NO arm here may depend on it — a
// `== 0` pin on that name would read the zero value for a missing key and be
// silently vacuous on the subject rather than red. Every listener therefore
// carries a default chain, and WHICH CHAIN SERVED is read from the response
// BODY (fixture-controlled) with the per-chain
// `http.<prefix>.downstream_rq_total` counters as corroboration.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0123-listener-transport-protocol"

// refAdminPort is the in-container reference admin port. Fixed at 9901 by the
// harness (startReferenceProxy always exposes 9901/tcp as admin).
const refAdminPort = 9901

// drivenPath is the single path every arm requests. Both chains of every
// listener route "/" to a direct_response, so the path does not discriminate —
// the BODY does.
const drivenPath = "/tp"

// wantStatus is the direct_response status configured on every chain of every
// listener. It is 200 and deliberately NOT a 1xx: a 1xx is an informational
// response the Go client consumes without surfacing it as the final status,
// which would make the per-arm status assertion read a value no chain set.
const wantStatus = 200

// armCount is the number of HTTP round trips this driver issues PER LISTENER,
// per side. It is 1, and the two counter pins below (served == 1,
// non-served == 0) are arithmetic derived from it — ⚠️ changing armCount
// INVALIDATES both pins.
const armCount = 1

// chainIndexed / chainDefault name the two outcomes in failure messages.
const (
	chainIndexed = "INDEXED"
	chainDefault = "DEFAULT"
)

// arm is one listener under test. The three arms are ordered; that order is
// the index-wise zip the runner performs between SubjectListenerNames() and
// ReferenceListenerPorts() (runner_test.go:1238-1249), so it must not be
// permuted in one accessor without the other.
type arm struct {
	// listener is the listener NAME. On the subject it is matched
	// byte-exactly against the ADR-0026 ready sentinel
	// ("envoy-go listener <name> ready on <addr>"), so it must appear
	// verbatim in the rendered subject bootstrap.
	listener string

	// refPort is the IN-CONTAINER reference listener port. The runner
	// publishes every port ReferenceListenerPorts() returns and hands back
	// the HOST-side mapping via ref.ListenerAddr(refPort) — the host port
	// is Docker-assigned and is NOT equal to refPort, which is why these
	// addresses are looked up per listener and never derived by offset from
	// a sibling's host address.
	//
	// ⚠️ CENSUSED AT THIS TIP, NOT INHERITED. 15123 follows the
	// `15000 + <fixture index>` convention; 15223 and 15224 are
	// deliberately OFF that convention so fixtures 0124/0125 keep 15124 and
	// 15125. Each of the three reads ZERO files under
	// `git grep -l '\b<port>\b' -- test/ internal/ cmd/` and ZERO live
	// sockets under `ss -tanH`.
	refPort int

	// indexedMatch is the filter_chain_match.transport_protocol STRING on
	// this listener's fc_indexed. It is the ONLY field that differs between
	// the three listeners' chain definitions — everything else (shape,
	// route, status, filter list) is rendered from one template.
	indexedMatch string

	// indexedPrefix / defaultPrefix are the two HCM stat_prefixes.
	//
	// 🔴 ALL SIX PREFIXES ACROSS THE THREE LISTENERS MUST BE DISTINCT, AND
	// THAT IS LOAD-BEARING. Two HCMs sharing one stat_prefix in a single
	// process panic envoy-go at boot with
	// `panic: stats: duplicate metric registration:
	// "http.<prefix>.downstream_rq_total"` (the banked phase-97 defect,
	// which fixture 0122 sits one identifier away from). This fixture
	// instantiates SIX HCMs and therefore sits six identifiers away.
	indexedPrefix string
	defaultPrefix string

	// indexedBody / defaultBody are the two direct_response bodies. They
	// are DISTINCT per chain AND per listener, so a single observed body
	// names both the listener and the chain that produced it — a body
	// reused across listeners could not tell "l_raw served its default"
	// from "the request reached l_tls".
	indexedBody string
	defaultBody string

	// wantChain / wantBody are the expectation, IDENTICAL ON BOTH SIDES.
	wantChain string
	wantBody  string
}

// arms is the fixture's roster, in listener order. l_bogus is index 0 and is
// therefore also the fixture's "primary" listener for the singular
// SubjectListenerName() / ReferenceListenerPort() accessors.
var arms = []arm{
	{
		listener: "l_bogus",
		refPort:  15123,
		// ⚠️ THIS STRING IS THE BOOT-REJECT PROBE. At the un-fixed tip
		// internal/listener/manager.go rejects any transport_protocol
		// outside {tls, raw_buffer, quic, ""} with
		// `transport_protocol %q must be "tls", "raw_buffer", "quic", or
		// empty`, so the SUBJECT FAILS TO BOOT and the whole fixture
		// fails at the subject-start step — not at an assertion. The
		// reference ACCEPTS it, boots, and serves from the default chain
		// because no connection's stamped transport_protocol can ever
		// equal it.
		indexedMatch:  "totally_bogus_value",
		indexedPrefix: "bogus_indexed",
		defaultPrefix: "bogus_default",
		indexedBody:   "bogus-indexed\n",
		defaultBody:   "bogus-default\n",
		wantChain:     chainDefault,
		wantBody:      "bogus-default\n",
	},
	{
		listener: "l_raw",
		refPort:  15223,
		// ⚠️ THIS IS THE HEADLINE ARM. The reference stamps raw_buffer on
		// a TCP connection no listener filter classified, so fc_indexed
		// is eligible and serves. A subject that only LIFTS the boot
		// reject (§3.2 T1) still leaves the input empty, still rejects
		// this chain on the exact-value branch, and still answers
		// DEFAULT — which is what this arm, and only this arm, catches.
		indexedMatch:  "raw_buffer",
		indexedPrefix: "raw_indexed",
		defaultPrefix: "raw_default",
		indexedBody:   "raw-indexed\n",
		defaultBody:   "raw-default\n",
		wantChain:     chainIndexed,
		wantBody:      "raw-indexed\n",
	},
	{
		listener: "l_tls",
		refPort:  15224,
		// ⚠️ THIS IS l_raw's MATCHED NEGATIVE, one string apart. No arm
		// here speaks TLS, so a correctly-stamped connection reads
		// raw_buffer and this chain must NOT match. It is what excludes a
		// subject that stamps a CONSTANT "tls" — such a subject passes
		// l_raw's sibling expectation for the wrong reason and is caught
		// only here.
		indexedMatch:  "tls",
		indexedPrefix: "tls_indexed",
		defaultPrefix: "tls_default",
		indexedBody:   "tls-indexed\n",
		defaultBody:   "tls-default\n",
		wantChain:     chainDefault,
		wantBody:      "tls-default\n",
	},
}

func init() { fixture.RegisterFixture(fixtureName, &tpDriver{}) }

// tpDriver is the fixture driver. It is STATEFUL by design: DriveReferenceMulti
// and DriveSubjectMulti record each side's per-listener observation so
// AssertStats can assert every arm ABSOLUTELY, per side, with one Errorf per
// property. Asserting inside Drive instead would force a returned error, which
// the runner turns into t.Fatalf — and a Fatalf on the first failing listener
// would MASK the other two arms' verdicts, which is precisely the
// first-divergence masking this three-arm roster exists to avoid.
type tpDriver struct {
	mu  sync.Mutex
	obs map[string]map[string]observation // side -> listener -> observation
}

// observation is one arm's measured response.
type observation struct {
	status int
	body   string
}

// Compile-time interface assertions.
//
// ⚠️ THESE ARE MANDATORY, NOT DECORATIVE. The runner dispatches both the
// multi-listener path (runner_test.go:1238) and the stats path
// (runner_test.go:1352) via SILENT type assertions with NO else branch. A
// signature typo makes ok == false, and the fixture then runs the SINGLE-addr
// path against l_bogus only, or skips the entire stats leg, with no diagnostic
// whatsoever.
var (
	_ fixture.Driver              = (*tpDriver)(nil)
	_ fixture.MultiListenerDriver = (*tpDriver)(nil)
	_ fixture.StatsAsserter       = (*tpDriver)(nil)
)

// --- fixture.Driver ---

// BackendCount is 1 — a throwaway backend no arm ever dials (every route on
// every chain is a direct_response). It is 1 and not 0 for TWO independent
// reasons: the runner rejects BackendCount() == 0 outright
// (runner_test.go:246-248), and envoy-go's cluster manager boot-rejects a
// bootstrap whose static_resources.clusters key is absent — so BOTH bootstraps
// carry a c_unused STATIC cluster pointed at this allocated-but-never-dialed
// port.
func (*tpDriver) BackendCount() int { return 1 }

// SubjectListenerName returns listener[0]. ⚠️ It is NOT dead code even though
// this driver implements MultiListenerDriver: the runner computes the primary
// addresses before the multi branch, and the single-addr fallback path uses it.
func (*tpDriver) SubjectListenerName() string { return arms[0].listener }

// ReferenceListenerPort returns listener[0]'s in-container port, for the same
// reason.
func (*tpDriver) ReferenceListenerPort() int { return arms[0].refPort }

// ReferenceBootstrap renders the reference contrib-Envoy bootstrap. The three
// listener ports are the FIXED in-container ports from the arms roster (the
// runner publishes each one), admin is the harness-fixed 9901, and the binds
// are on 0.0.0.0 because the host reaches them through Docker's published
// mapping. backendPorts[0] is templated into the never-dialed c_unused cluster.
func (*tpDriver) ReferenceBootstrap(backendPorts []int) string {
	return renderBootstrap("0.0.0.0", refAdminPort, func(i int) int { return arms[i].refPort }, backendPorts[0])
}

// SubjectConfig renders envoy-go's equivalent bootstrap: the SAME listener /
// chain / HCM / route shape as the reference, adapted to loopback addressing
// with the runner's allocated ports.
//
// ⚠️ THE THREE SUBJECT LISTENER PORTS ARE subjListenerPort, +1 AND +2. The
// runner hands a driver ONE listener port; multi-listener fixtures derive the
// rest by consecutive offset (the 0018/0036 convention). That is SAFE HERE
// because startSubjectWithRetry allocates via freeTCPPortBlock, which probes
// the whole block [base, base+16) bindable on the WILDCARD address before
// returning the base (harness_test.go:270-316). Three of sixteen is well
// inside the reservation.
//
// The first parameter (refListenerPort) is deliberately unused: the subject
// binds its own allocated ports and never needs the reference's.
func (*tpDriver) SubjectConfig(_ int, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return renderBootstrap("127.0.0.1", subjAdminPort, func(i int) int { return subjListenerPort + i }, backendPorts[0])
}

// DriveReference drives arm[0] (l_bogus) against the single address the runner
// passes. UNREACHABLE while MultiListenerDriver is implemented — the runner
// dispatches DriveReferenceMulti instead (runner_test.go:1238). It is
// implemented as a genuine single-arm drive rather than by DERIVING the two
// sibling addresses: on the reference side the host ports are Docker-assigned
// and bear no arithmetic relation to each other, so any offset derivation here
// would be quietly wrong.
func (d *tpDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveAll(ctx, "ref", map[string]string{arms[0].listener: addr}, arms[:1])
}

// DriveSubject drives arm[0] (l_bogus). UNREACHABLE, same reasoning.
func (d *tpDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveAll(ctx, "subj", map[string]string{arms[0].listener: addr}, arms[:1])
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*tpDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.MultiListenerDriver ---

// SubjectListenerNames returns the three subject listener names in roster
// order. ⚠️ The runner t.Fatalf's when this length differs from
// ReferenceListenerPorts() and zips the two INDEX-WISE, so both accessors are
// derived from the SAME ordered roster rather than written out twice.
func (*tpDriver) SubjectListenerNames() []string {
	out := make([]string, len(arms))
	for i, a := range arms {
		out[i] = a.listener
	}
	return out
}

// ReferenceListenerPorts returns the three in-container reference ports in the
// order matching SubjectListenerNames().
func (*tpDriver) ReferenceListenerPorts() []int {
	out := make([]int, len(arms))
	for i, a := range arms {
		out[i] = a.refPort
	}
	return out
}

// DriveReferenceMulti issues one GET per listener against the reference.
func (d *tpDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveAll(ctx, "ref", addrs, arms)
}

// DriveSubjectMulti issues one GET per listener against the subject.
func (d *tpDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveAll(ctx, "subj", addrs, arms)
}

// driveAll issues armCount GET requests per listener and emits a deterministic
// per-listener byte stream for the runner's CompareBytes pass.
//
// ⚠️ THE SIDE LABEL IS INTENTIONALLY EXCLUDED from the emitted bytes: both
// sides must produce IDENTICAL bytes when behavior is equivalent, and a side
// token would make every run diverge. The label is used only for the local
// record and for error text.
//
// A returned error becomes t.Fatalf in the runner, so only a genuine TRANSPORT
// failure returns one. A wrong-chain body is recorded and asserted later in
// AssertStats, where each arm gets its own Errorf.
func (d *tpDriver) driveAll(ctx context.Context, side string, addrs map[string]string, roster []arm) ([]byte, error) {
	var b bytes.Buffer
	seen := make(map[string]observation, len(roster))
	for _, a := range roster {
		addr := addrs[a.listener]
		if addr == "" {
			return nil, fmt.Errorf("%s: no address supplied for listener %q (have %d entries)", side, a.listener, len(addrs))
		}
		var last observation
		for i := 0; i < armCount; i++ {
			resp, body, err := helpers.HTTPRoundTrip(ctx, addr, http.MethodGet, drivenPath, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: GET %s%s: %w", side, a.listener, addr, drivenPath, err)
			}
			last = observation{status: resp.StatusCode, body: string(body)}
		}
		fmt.Fprintf(&b, "listener %s status=%d body=%q\n", a.listener, last.status, last.body)
		seen[a.listener] = last
	}
	d.record(side, seen)
	return b.Bytes(), nil
}

func (d *tpDriver) record(side string, seen map[string]observation) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.obs == nil {
		d.obs = map[string]map[string]observation{}
	}
	d.obs[side] = seen
}

func (d *tpDriver) recorded(side string) map[string]observation {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.obs[side]
}

// --- fixture.StatsAsserter ---

// AssertStats performs the fixture's two assertion families, per side:
//
//  1. THE BODY, ABSOLUTELY, PER ARM. The body is fixture-controlled and is the
//     primary discriminator of which chain served. A cross-side CompareBytes
//     cannot see a defect BOTH sides share, so each arm's expected body is
//     asserted absolutely on each side in addition to the cross-side diff.
//
//  2. THE TWO PER-CHAIN COUNTERS, as corroboration:
//     http.<indexedPrefix>.downstream_rq_total and
//     http.<defaultPrefix>.downstream_rq_total, one of which must read armCount
//     and the other 0, according to the arm's wantChain.
//
// ⚠️ EVERY COUNTER PIN IS GUARDED BY AN EXPLICIT PRESENCE CHECK. A scrape map
// returns the ZERO VALUE for a missing key, so a `== 0` pin on a name a side
// never emits would be silently vacuous rather than red. SPEC §7.3 obliges
// this fixture to prove each counter NAME is emitted by BOTH sides before
// pinning its value; the presence check is that proof, and it is what goes red
// if a side stops emitting a chain's scope.
//
// ⚠️ NO ARM READS `no_filter_chain_match`. The divergence on that name is
// NAME-level: the reference emits it at 0 and the subject does not emit the
// name at all, so a pin on it would pass for the wrong reason forever.
//
// Per-property Errorf (NOT Fatalf — a Fatalf here would make every later arm's
// assertion unreachable dead code). Fatalf is reserved for a broken
// precondition: the scrape itself failing, or a side having no recorded drive.
func (d *tpDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refSt, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subjSt, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}

	for _, side := range []struct {
		name   string
		stats  map[string]uint64
		probes map[string]observation
	}{
		{"ref", refSt, d.recorded("ref")},
		{"subj", subjSt, d.recorded("subj")},
	} {
		if len(side.probes) == 0 {
			t.Fatalf("%s: no drive observations recorded — the drive step did not run, so every "+
				"assertion below would be vacuous", side.name)
			return
		}
		for _, a := range arms {
			assertArm(t, side.name, a, side.probes, side.stats)
		}
	}
}

// assertArm asserts one listener's body and its two counters on one side.
func assertArm(t fixture.TB, side string, a arm, probes map[string]observation, stats map[string]uint64) {
	t.Helper()

	indexedTotal := "http." + a.indexedPrefix + ".downstream_rq_total"
	defaultTotal := "http." + a.defaultPrefix + ".downstream_rq_total"

	iv, iok := stats[indexedTotal]
	dv, dok := stats[defaultTotal]
	got, gok := probes[a.listener]

	// fixture.TB has NO Logf (only Errorf/Fatalf/Helper); log.Printf records
	// the observed values regardless of pass/fail, so a green run still leaves
	// the measurement in the transcript.
	log.Printf("%s: %s %s status=%d body=%q (recorded=%t) %s=%d(present=%t) %s=%d(present=%t) want=%s",
		fixtureName, side, a.listener, got.status, got.body, gok,
		indexedTotal, iv, iok, defaultTotal, dv, dok, a.wantChain)

	// PROPERTY 1 — the arm actually ran.
	if !gok {
		t.Errorf("%s %s: no observation recorded for this listener; the remaining assertions for "+
			"it would read zero values and be vacuous", side, a.listener)
		return
	}

	// PROPERTY 2 — status. Asserted BEFORE the body: a body compared against a
	// response that never reached a chain is not evidence.
	if got.status != wantStatus {
		t.Errorf("%s %s: status = %d, want %d", side, a.listener, got.status, wantStatus)
	}

	// PROPERTY 3 — the BODY names the chain that served. A wrong-chain body is
	// reported BY NAME, not as a generic mismatch.
	if got.body != a.wantBody {
		switch got.body {
		case a.indexedBody:
			t.Errorf("%s %s: served by the INDEXED chain (body %q), want the %s chain — "+
				"filter_chain_match{transport_protocol: %q} matched a connection it must not match",
				side, a.listener, got.body, a.wantChain, a.indexedMatch)
		case a.defaultBody:
			t.Errorf("%s %s: served by the DEFAULT chain (body %q), want the %s chain — "+
				"filter_chain_match{transport_protocol: %q} was NOT eligible; on a plaintext TCP "+
				"listener with no listener filter the connection's transport_protocol must be "+
				"stamped %q before chain selection",
				side, a.listener, got.body, a.wantChain, a.indexedMatch, "raw_buffer")
		default:
			t.Errorf("%s %s: body %q, want %q (neither of this listener's two chain bodies)",
				side, a.listener, got.body, a.wantBody)
		}
	}

	// PROPERTY 4 — the two counters. wantIndexed/wantDefault are arm
	// arithmetic over armCount; exactly one of them is nonzero.
	wantIndexed, wantDefault := uint64(0), uint64(armCount)
	if a.wantChain == chainIndexed {
		wantIndexed, wantDefault = uint64(armCount), 0
	}
	for _, c := range []struct {
		name    string
		val     uint64
		present bool
		want    uint64
	}{
		{indexedTotal, iv, iok, wantIndexed},
		{defaultTotal, dv, dok, wantDefault},
	} {
		if !c.present {
			t.Errorf("%s %s: %s ABSENT from the scrape, want present and == %d (a missing key reads "+
				"as 0, which would make this pin vacuous rather than red)",
				side, a.listener, c.name, c.want)
			continue
		}
		if c.val != c.want {
			t.Errorf("%s %s: %s = %d, want %d", side, a.listener, c.name, c.val, c.want)
		}
	}
}

// scrapeStats issues GET http://<addr>/stats — the FLAT admin text form, NOT
// /stats/prometheus — and parses "name: value" lines into a map[name]uint64.
//
// THE FLAT ENDPOINT IS A DELIBERATE CHOICE, measured rather than assumed. Of
// the 88 non-test fixture drivers that declare AssertStats, 31 scrape
// /stats/prometheus and 57 scrape the flat /stats; the shape precedent for
// this fixture, 0122-quic-chain-selection, is one of the 57. The names this
// fixture pins are plain dotted HCM counters carrying no address token, so
// they are cross-side comparable verbatim on the flat surface, and the flat
// form sidesteps the prometheus spelling (envoy_http_downstream_rq_total with
// the prefix moved into an envoy_http_conn_manager_prefix LABEL) and its
// registration-order listing entirely.
//
// Driver-side helpers are DUPLICATED locally rather than imported from
// test/differential, which would create an import cycle (fixture drivers are
// imported BY the runner).
//
// The line split is on the LAST ": " so a stat name containing a colon is not
// truncated, and a non-numeric value (histograms, special formats) is SKIPPED
// rather than coerced.
func scrapeStats(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read %s body: %w", url, err)
	}

	out := make(map[string]uint64)
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, ": ")
		if idx < 0 {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimSpace(line[idx+2:]), 10, 64)
		if err != nil {
			continue // skip non-numeric (histograms, special formats)
		}
		out[line[:idx]] = v
	}
	return out, nil
}

// --- bootstrap rendering ---

// renderBootstrap builds one side's complete bootstrap. BOTH sides are rendered
// by THIS function, differing only in bind address, admin port and the three
// listener ports.
//
// ⚠️ ONE RENDERER, NOT TWO LITERAL TEMPLATES, ON PURPOSE. The proposition under
// test is that two proxies given the SAME listener shape select DIFFERENT
// chains. Two hand-maintained YAML blobs make that shape-identity a review
// exercise that silently rots; one renderer makes it structural. The only
// fields that may ever differ cross-side are the three parameters below — if a
// future edit needs a fourth, that difference is a fixture-design decision and
// belongs in a comment, not in a second copy of the template.
func renderBootstrap(bindAddr string, adminPort int, portFor func(i int) int, backendPort int) string {
	var b strings.Builder
	fmt.Fprintf(&b, bootstrapHeadTmpl, bindAddr, adminPort)
	for i, a := range arms {
		fmt.Fprintf(&b, listenerTmpl,
			a.listener,               // 1 listener name
			bindAddr,                 // 2 bind address
			portFor(i),               // 3 listener port
			a.indexedMatch,           // 4 fc_indexed transport_protocol
			a.indexedPrefix,          // 5 fc_indexed stat_prefix (+ route/vhost names)
			yamlQuote(a.indexedBody), // 6 fc_indexed direct_response body
			a.defaultPrefix,          // 7 default chain stat_prefix (+ route/vhost names)
			yamlQuote(a.defaultBody), // 8 default chain direct_response body
		)
	}
	fmt.Fprintf(&b, clustersTmpl, backendPort)
	return b.String()
}

// yamlQuote renders a body as a YAML double-quoted scalar with its newline
// escaped.
//
// ⚠️ THE ESCAPE IS REQUIRED. The bodies carry a real trailing newline; splicing
// one verbatim into a single-line `inline_string: "..."` would terminate the
// YAML scalar mid-value and produce a bootstrap that does not parse. YAML's
// double-quoted style and Go's quoted string agree on the escapes these
// ASCII-only bodies need, so strconv.Quote is exact here — it would NOT be for
// a body carrying non-ASCII or a literal backslash, neither of which this
// fixture uses.
func yamlQuote(s string) string { return strconv.Quote(s) }

// bootstrapHeadTmpl opens the bootstrap. Args: bind address, admin port.
const bootstrapHeadTmpl = `admin:
  address:
    socket_address: { address: %s, port_value: %d }
static_resources:
  listeners:
`

// listenerTmpl renders ONE listener: a plaintext TCP socket_address, ONE
// indexed filter chain carrying a filter_chain_match on transport_protocol
// alone, and a last-resort default_filter_chain.
//
// ⚠️ THERE IS NO `listener_filters` KEY HERE, AND THERE MUST NEVER BE ONE —
// see the package doc comment. An unclassified plaintext connection is the
// entire experiment.
//
// ⚠️ THERE IS NO `transport_socket` KEY EITHER. All three listeners are
// PLAINTEXT, including l_tls — whose name refers to the STRING in its
// filter_chain_match, not to any TLS it terminates. Giving l_tls a TLS
// transport socket would make its indexed chain legitimately eligible on the
// reference and destroy its role as l_raw's matched negative.
//
// Args (positional, in template order): 1 listener name, 2 bind address,
// 3 listener port, 4 fc_indexed transport_protocol, 5 fc_indexed stat_prefix,
// 6 fc_indexed body, 7 default chain stat_prefix, 8 default chain body.
const listenerTmpl = `    - name: %[1]s
      address:
        socket_address: { address: %[2]s, port_value: %[3]d }
      filter_chains:
        - name: fc_indexed
          filter_chain_match:
            transport_protocol: %[4]s
          filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: %[5]s
                route_config:
                  name: rc_%[5]s
                  virtual_hosts:
                    - name: vh_%[5]s
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 200
                            body: { inline_string: %[6]s }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
      default_filter_chain:
        name: fc_default
        filters:
          - name: envoy.filters.network.http_connection_manager
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
              stat_prefix: %[7]s
              route_config:
                name: rc_%[7]s
                virtual_hosts:
                  - name: vh_%[7]s
                    domains: ["*"]
                    routes:
                      - match: { prefix: "/" }
                        direct_response:
                          status: 200
                          body: { inline_string: %[8]s }
              http_filters:
                - name: envoy.filters.http.router
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`

// clustersTmpl closes the bootstrap with the single placeholder cluster. Arg:
// the runner-allocated backend port.
//
// ⚠️ NO ROUTE DIALS THIS CLUSTER — every route on every chain is a
// direct_response. It exists because envoy-go's cluster manager boot-rejects a
// bootstrap with an absent static_resources.clusters key. It is rendered on
// BOTH sides so the two bootstraps stay shape-identical; on the reference the
// 127.0.0.1 endpoint is container-local and, being STATIC and never dialed, is
// never resolved or connected to.
const clustersTmpl = `  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`
