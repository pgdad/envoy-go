// Package driver is the differential fixture driver for
// 0118-runtime-static-layer: the phase-77 layered_runtime static-layer
// consumer, asserted STATS-ONLY on runtime.num_keys / runtime.num_layers.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0118-runtime-static-layer"

	// In-container reference Envoy ports. Convention "10<fixture index>" —
	// 0114→10114, 0115→10115, 0116→10116, 0117→10117, so 0118→10118.
	// ⚠️ NOT 10450: that is the TLS/SDS band (0108-0113), and this is not a
	// TLS fixture. 10118 RE-DERIVED FREE at this task: zero hits in test/, in
	// any *.go, in any *.yaml.
	refAdminPort    = 9901
	refListenerPort = 10118

	// The reference-MEASURED expectations (contrib-v1.37.2, 3 fresh boots,
	// with 4 isolation arms summing exactly 1+2+1+2=6). See PLAN §1.3.
	wantNumKeys   = 6
	wantNumLayers = 2

	// The INTERNAL stat names, as they appear on the FLAT /stats endpoint on
	// BOTH sides. These are what the value assertions key on — see AssertStats
	// for why the flat endpoint and not /stats/prometheus.
	statNumKeys   = "runtime.num_keys"
	statNumLayers = "runtime.num_layers"

	// The PROMETHEUS names. Used ONLY by the departure pin: the reference
	// publishes both, envoy-go publishes NEITHER.
	promNumKeys   = "envoy_runtime_num_keys"
	promNumLayers = "envoy_runtime_num_layers"

	// dialDeadline bounds the single open-and-close probe connection.
	dialDeadline = 10 * time.Second
)

type runtimeStaticLayerDriver struct{}

func init() { fixture.RegisterFixture(fixtureName, &runtimeStaticLayerDriver{}) }

// --- fixture.Driver (required) ---

// BackendCount stays 1: this fixture drives NO backend traffic, but the runner
// rejects 0 (a t.Fatalf in the fixture-run setup) —
// reference_differential_backendcount_min_one. The default TCPEcho kind is the
// minimum viable shape (0110's posture). +0 BackendKinds.
func (*runtimeStaticLayerDriver) BackendCount() int           { return 1 }
func (*runtimeStaticLayerDriver) SubjectListenerName() string { return "l_test" }
func (*runtimeStaticLayerDriver) ReferenceListenerPort() int  { return refListenerPort }

func (d *runtimeStaticLayerDriver) ReferenceBootstrap(backendPorts []int) string {
	return mustRender(mustReadFixtureFile("envoy.yaml"), map[string]any{
		"AdminPort":    refAdminPort,
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
	})
}

func (d *runtimeStaticLayerDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return mustRender(mustReadFixtureFile("envoy-go.yaml"), map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
	})
}

// DriveReference / DriveSubject open and immediately close one TCP connection
// so the runner's CompareBytes step has a defined (empty) result on both sides.
// The row's whole observable is the gauge pair; there is no request semantics
// to compare.
func (d *runtimeStaticLayerDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}

func (d *runtimeStaticLayerDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}

// drive dials the listener once and closes it. It returns an EMPTY, non-nil
// slice on both sides so CompareBytes has a defined result. The dial is not
// decoration: it proves the listener actually BOUND on this side, which a
// stats-only fixture would otherwise never establish.
func (*runtimeStaticLayerDriver) drive(ctx context.Context, addr string) ([]byte, error) {
	dctx, cancel := context.WithTimeout(ctx, dialDeadline)
	defer cancel()
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(dctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	_ = conn.Close()
	return []byte{}, nil
}

// ProbeAdmin returns /ready ONLY. ⚠️ It deliberately does NOT reach for
// /runtime: compareAdminResponses compares the body BYTE-EXACT and the
// reference's /runtime body is non-deterministic in FOUR independent ways
// (per-request JSON key order; a per-process Struct debug-string marker —
// 8 distinct strings across 13 fresh processes; a leaked DebugString for
// empty-map values that still counts as a key; and a non-deterministic
// within-layer collision winner). All four contaminate the BODY only; the two
// gauges asserted by AssertStats are immune.
func (*runtimeStaticLayerDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.StatsAsserter ---

// AssertStats is the runner's step-10 stats leg (ADR-0062). It scrapes the FLAT
// /stats endpoint on BOTH sides and pins the two phase-77 runtime gauges, then
// separately pins the PROMETHEUS-EXPOSITION DEPARTURE described below.
//
// ⚠️ THE ENDPOINT IS THE FLAT /stats, NOT /stats/prometheus — AND THAT IS A
// MEASURED FINDING, NOT A STYLE CHOICE. The phase-77 PLAN §1.3 specified
// /stats/prometheus on the strength of the REFERENCE's line shape
// (`envoy_runtime_num_keys{} 6`). EXECUTED at this task, against the shipped
// config on both sides:
//
//	reference  /stats             runtime.num_keys: 6      runtime.num_layers: 2
//	reference  /stats/prometheus  envoy_runtime_num_keys{} 6   ...num_layers{} 2
//	subject    /stats             runtime.num_keys: 6      runtime.num_layers: 2
//	subject    /stats/prometheus  *** BOTH NAMES ABSENT ***
//
// The gauges ARE registered and DO carry the correct values on the subject; the
// PROMETHEUS RENDERER drops them. internal/stats.ExtractTags recognizes only
// the top-level segments `cluster.|http.|listener.|server.` (plus the hoisting
// prefixes rbac./mongo./redis./thrift./…) and returns an error for anything
// else; internal/stats.WriteProm's Walk callback SILENTLY SKIPS a metric whose
// flattenToProm errors ("skip malformed names (defense-in-depth; should not
// occur)"). `runtime.` was never added to that dispatch when the gauges landed,
// so the two names vanish from the prometheus exposition with no log and no
// error. Asserting /stats/prometheus as the PLAN specified would have made this
// row PERMANENTLY RED for a reason that has nothing to do with layered_runtime.
//
// The flat endpoint is a sound cross-side seam HERE specifically because the
// two names carry NO address and NO dynamic segment, so the internal name is
// cross-side IDENTICAL — the hazard behind
// reference_listener_stat_scope_cross_side_divergence (which is why listener
// stats must go through /stats/prometheus, where the address is a LABEL) does
// not arise.
//
// ⚠️ THE DISPATCH IS A SILENT TYPE ASSERTION with no else, no log and no skip
// (runner_test.go's step 10, FIRST addr = REFERENCE). A signature typo makes
// ok == false and this whole leg vanishes GREEN while the compiler, go vet and
// golangci-lint all stay quiet. The compile-time assertion below is the
// tripwire; the log.Printf line below is the only RUNTIME proof the leg ran.
//
// ⚠️ NO PRECONDITION IS AVAILABLE FROM THE OTHER runtime.* NAMES. Measured at
// the phase-77 PLAN and re-confirmed here: runtime.load_success and
// runtime.override_dir_not_exists both read 1 on the reference even with NO
// layered_runtime block at all, so neither is a "a static layer loaded" guard.
// The absent-check plus a non-zero num_layers is the honest substitute.
//
// ⚠️ envoy-go publishes 2 runtime.* names where the reference publishes 9
// (measured here: admin_overrides_active, deprecated_feature_seen_since_
// process_start, deprecated_feature_use, load_error, load_success, num_keys,
// num_layers, override_dir_exists, override_dir_not_exists). The project
// asserts NAMED SUBSETS cross-side, never full-set equality
// (reference_stats_sink_emits_used_only), so this creates no divergence here —
// but a future row asserting full runtime.* name-set equality WILL fail,
// correctly.
func (d *runtimeStaticLayerDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	// Scrapes are PRECONDITIONS, not properties -> Fatalf
	// (reference_fatalf_makes_assertions_unreachable).
	ref, err := scrapeFlat(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeFlat(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}
	refProm, err := scrapeProm(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats/prometheus: %v", err)
	}
	subjProm, err := scrapeProm(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats/prometheus: %v", err)
	}

	// fixture.TB has EXACTLY Errorf/Fatalf/Helper — no Logf
	// (reference_fixture_tb_has_no_logf). Diagnostics go through log.Printf.
	// This line is also the evidence that the leg RAN at all.
	_, refPromKeysOK := refProm[promNumKeys]
	_, subjPromKeysOK := subjProm[promNumKeys]
	log.Printf("0118 AssertStats: ref num_keys=%d num_layers=%d | subj num_keys=%d num_layers=%d "+
		"| prom-exposition ref_num_keys_present=%t subj_num_keys_present=%t",
		ref[statNumKeys], ref[statNumLayers], subj[statNumKeys], subj[statNumLayers],
		refPromKeysOK, subjPromKeysOK)

	want := map[string]uint64{
		statNumKeys:   wantNumKeys,   // 1(A) + 2(B) + 1(C) + 2(D), UNION
		statNumLayers: wantNumLayers, // L1 + L2
	}
	for _, n := range []string{statNumKeys, statNumLayers} {
		refVal, refOK := ref[n]
		subjVal, subjOK := subj[n]
		// ⚠️ THE ABSENT CHECK IS SEPARATE FROM THE VALUE CHECK, and it
		// `continue`s. A gauge that fails to REGISTER reads as 0 == 0 through a
		// single-value lookup and would pass VACUOUSLY.
		if !refOK {
			t.Errorf("ref: %s ABSENT from /stats", n)
			continue
		}
		if !subjOK {
			t.Errorf("subj: %s ABSENT from /stats", n)
			continue
		}
		if refVal != want[n] {
			t.Errorf("ref %s = %d, want %d", n, refVal, want[n])
		}
		if subjVal != want[n] {
			t.Errorf("subj %s = %d, want %d", n, subjVal, want[n])
		}
		if refVal != subjVal {
			t.Errorf("cross-side mismatch %s: ref=%d subj=%d", n, refVal, subjVal)
		}
	}

	// ⚠️ THE TRANSPOSITION CHECK, and it is only possible because 6 != 2.
	// A build that wires num_keys to the layer count and num_layers to the key
	// count passes every per-name value check above ONLY if the two wants are
	// equal. They are not, so the checks above already catch it — this is the
	// named diagnosis rather than two cryptic mismatches.
	if ref[statNumKeys] == want[statNumLayers] && ref[statNumLayers] == want[statNumKeys] {
		t.Errorf("ref: the two gauges look TRANSPOSED (num_keys=%d num_layers=%d)",
			ref[statNumKeys], ref[statNumLayers])
	}
	if subj[statNumKeys] == want[statNumLayers] && subj[statNumLayers] == want[statNumKeys] {
		t.Errorf("subj: the two gauges look TRANSPOSED (num_keys=%d num_layers=%d)",
			subj[statNumKeys], subj[statNumLayers])
	}

	d.assertPrometheusExpositionDeparture(t, refProm, subjProm)
}

// assertPrometheusExpositionDeparture PINS the measured cross-side asymmetry of
// the PROMETHEUS exposition, in BOTH directions, so it cannot drift silently.
//
// ⚠️ THIS IS A DEPARTURE PIN, NOT A PARITY CLAIM. The reference emits both
// gauges to /stats/prometheus; envoy-go emits NEITHER, because
// internal/stats.ExtractTags does not recognize a `runtime.` top-level segment
// and internal/stats.WriteProm silently skips any metric whose name fails to
// flatten. The gauges themselves are correct — only the renderer drops them.
//
// Prose alone would not hold this: a documented gap that nothing executes is
// exactly the shape this lineage keeps rediscovering. So it is asserted, and it
// is asserted SYMMETRICALLY:
//
//   - the REFERENCE side must KEEP publishing both names (a regression there
//     would otherwise be invisible, since this row no longer reads the
//     prometheus endpoint for its values);
//   - the SUBJECT side must STILL be missing both names. ⚠️ THE DAY
//     internal/stats LEARNS `runtime.`, THIS ROW GOES RED ON PURPOSE — that is
//     the signal to move the value assertions above back onto
//     /stats/prometheus as the phase-77 PLAN originally specified, and to
//     delete this function.
func (*runtimeStaticLayerDriver) assertPrometheusExpositionDeparture(t fixture.TB, refProm, subjProm map[string]uint64) {
	t.Helper()

	for name, want := range map[string]uint64{promNumKeys: wantNumKeys, promNumLayers: wantNumLayers} {
		v, ok := refProm[name]
		if !ok {
			t.Errorf("ref: %s ABSENT from /stats/prometheus — the REFERENCE has always published it "+
				"(measured `%s{} %d` on the pinned image); this row's value assertions moved to the flat "+
				"/stats endpoint, so nothing else would catch a reference-side regression here", name, name, want)
			continue
		}
		if v != want {
			t.Errorf("ref %s = %d on /stats/prometheus, want %d", name, v, want)
		}
	}

	for _, name := range []string{promNumKeys, promNumLayers} {
		if v, ok := subjProm[name]; ok {
			t.Errorf("subj: %s is NOW PRESENT on /stats/prometheus (= %d) — the phase-77 prometheus-exposition "+
				"gap has been CLOSED. That is good news and this row is deliberately RED to force the follow-up: "+
				"move this fixture's value assertions from the flat /stats endpoint back onto /stats/prometheus "+
				"(as the phase-77 PLAN §1.3 specified) and DELETE "+
				"assertPrometheusExpositionDeparture", name, v)
		}
	}
}

// --- file / template helpers (the 0103/0108/0109/0110 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0118-runtime-static-layer/driver/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

func mustRender(tpl string, data map[string]any) string {
	t, err := template.New("bootstrap").Parse(tpl)
	if err != nil {
		panic(fmt.Sprintf("driver: template parse: %v", err))
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("driver: template execute: %v", err))
	}
	return buf.String()
}

// scrapeFlat issues GET http://<addr>/stats and returns a map keyed by the
// INTERNAL stat name. Both sides serve the same "<name>: <value>\n" text shape,
// and the two names this row cares about carry no address and no dynamic
// segment, so the key is cross-side IDENTICAL.
//
// Lines whose value does not parse as a non-negative finite number are SKIPPED,
// not errored: the reference interleaves histogram lines whose value is a
// percentile summary ("P0(nan,nan) P25(...)") or the literal
// "No recorded values". Skipping them cannot mask an absent gauge — the ABSENT
// check in AssertStats is a separate, `continue`ing branch keyed on map
// membership, so a name that never lands in the map fails loudly rather than
// reading 0.
func scrapeFlat(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	var body bytes.Buffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read %s body: %w", url, err)
	}

	out := map[string]uint64{}
	for _, line := range strings.Split(body.String(), "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok || name == "" {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			continue // histogram summaries, "No recorded values", text values
		}
		out[name] = uint64(v)
	}
	return out, nil
}

// scrapeProm issues GET http://<addr>/stats/prometheus and returns a map keyed
// by the metric NAME with the `{...}` label set STRIPPED ENTIRELY, colliding by
// SUMMING. Cloned from 0110/driver.go.
//
// ⚠️ The two gauges carry an EMPTY label set and each sample is preceded by its
// own `# TYPE` line:
//
//	# TYPE envoy_runtime_num_keys gauge
//	envoy_runtime_num_keys{} 6
//	# TYPE envoy_runtime_num_layers gauge
//	envoy_runtime_num_layers{} 2
//
// The brace branch below handles `{}` correctly (open=…, closeIdx=open+1, so
// the name is everything before `{` and the value everything after `}`). A
// hand-rolled `grep -c` on either name would return 2 per scrape because of the
// `# TYPE` line; the `#` skip below means that bites only a shell gate.
func scrapeProm(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	var body bytes.Buffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read %s body: %w", url, err)
	}

	out := map[string]uint64{}
	for _, line := range strings.Split(body.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, rest string
		if open := strings.IndexByte(line, '{'); open >= 0 {
			closeIdx := strings.LastIndexByte(line, '}')
			if closeIdx < open {
				continue // malformed: no closing brace
			}
			name = line[:open]
			rest = strings.TrimSpace(line[closeIdx+1:])
		} else {
			sp := strings.IndexByte(line, ' ')
			if sp < 0 {
				continue
			}
			name = line[:sp]
			rest = strings.TrimSpace(line[sp+1:])
		}
		// Strip an optional trailing timestamp ("<value> <timestamp>").
		if sp := strings.IndexByte(rest, ' '); sp >= 0 {
			rest = rest[:sp]
		}
		// ParseFloat, not ParseUint: the exposition format permits float values,
		// and histogram lines can carry nan/inf. Non-finite and negative values
		// are skipped rather than converted (uint64(NaN) is undefined).
		v, err := strconv.ParseFloat(rest, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			continue
		}
		out[name] += uint64(v)
	}
	return out, nil
}

// Compile-time interface assertions. ⚠️ The StatsAsserter one is MANDATORY:
// the runner dispatches step 10 via a SILENT type assertion, so a signature
// typo makes ok == false and the whole assertion NEVER RUNS while every tool
// stays quiet.
var (
	_ fixture.Driver        = (*runtimeStaticLayerDriver)(nil)
	_ fixture.StatsAsserter = (*runtimeStaticLayerDriver)(nil)
)
