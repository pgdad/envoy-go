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

	// The PROMETHEUS names. Used by assertPrometheusExpositionParity: as of
	// phase 79 BOTH sides publish BOTH names. Through phase 78 the subject
	// published NEITHER and this pair keyed an absence pin.
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
// PROMETHEUS RENDERER dropped them through phase 78. internal/stats.ExtractTags
// recognizes THIRTEEN top-level segments and returns an error for anything else:
//
//	cluster. http. listener. server. runtime. access_logs. tracing. sds. wasm.
//	    the `case strings.HasPrefix(internal, ...)` arms of the ExtractTags switch
//	mongo. kafka. redis. thrift.
//	    the root-anchored `strings.CutPrefix(internal, ...)` calls in its default arm
//
// ⚠️ `runtime.`, `access_logs.` and `tracing.` are PHASE-79 additions and `sds.`
// is a PHASE-80 addition. Before phase 79 the roster was NINE, and between
// phases 79 and 80 it was one short of the figure above — which is why in-tree
// prose elsewhere still carries both stale figures. Treat any count you find
// here or elsewhere as stale until you have re-counted from the switch.
//
// ⚠️ FOUR FURTHER detectors exist and are NOT top-level — they are MID-NAME
// (INFIX) `strings.Index` matches, declared as the `lrlSegment`, `blSegment`,
// `rbacSegment` and `zkSegment` consts. They widen the set of accepted NAMES but
// NOT the set of accepted ROOTS:
//
//	.http_local_rate_limit.   .http_bandwidth_limit.   .rbac.   .zookeeper.
//
// Each fires on ANY dot-free leading segment, so `ANYTHING_AT_ALL.rbac.allowed`
// parses clean (residual `rbac.allowed`) while the ROOT-anchored `rbac.allowed`
// does NOT parse — the head must be non-empty. Counting these four as roots is
// the standing documentation error; the top-level answer is THIRTEEN, and the
// two species must never be summed.
//
// ⚠️ NO name.go LINE NUMBERS ARE CITED ABOVE, DELIBERATELY. Every line cite this
// comment previously carried went stale inside a single phase. Grep the symbols
// named here instead; the authoritative roster is the `noRecognizedSegmentErrFmt`
// const in internal/stats/name.go.
//
// internal/stats.WriteProm's Walk callback SKIPS a metric whose flattenToProm
// errors. `runtime.` was not in that dispatch when the gauges landed, so the two
// names vanished from the prometheus exposition — and through phase 78 that skip
// was SILENT, with no log and no error, which is why the gap survived
// registration, the whole unit suite and go vet alike. Phase 79 changed BOTH
// halves: the `runtime.` arm landed, AND WriteProm now emits one aggregated log
// line per call naming the metrics it skipped. The skip still returns no error,
// so the log is the only signal. Asserting /stats/prometheus at phase 77, as
// that PLAN specified, would have made this row PERMANENTLY RED for a reason
// that had nothing to do with layered_runtime.
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
	refProm, err := scrapePromSamples(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats/prometheus: %v", err)
	}
	subjProm, err := scrapePromSamples(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats/prometheus: %v", err)
	}

	// fixture.TB has EXACTLY Errorf/Fatalf/Helper — no Logf
	// (reference_fixture_tb_has_no_logf). Diagnostics go through log.Printf.
	// This line is also the evidence that the leg RAN at all.
	// ⚠️ The prometheus fields RECORD BOTH SIDES' VALUES AND LABEL TEXT, not
	// just presence. Presence alone was enough while this row pinned an
	// ABSENCE; now that it asserts PARITY, a green run must leave behind the
	// measured numbers it was green on.
	refPromKeys, refPromKeyLabels := foldPromSamples(refProm[promNumKeys])
	subjPromKeys, subjPromKeyLabels := foldPromSamples(subjProm[promNumKeys])
	log.Printf("0118 AssertStats: ref num_keys=%d num_layers=%d | subj num_keys=%d num_layers=%d "+
		"| prom-exposition ref_num_keys_present=%t(=%d labels=%q) subj_num_keys_present=%t(=%d labels=%q)",
		ref[statNumKeys], ref[statNumLayers], subj[statNumKeys], subj[statNumLayers],
		len(refProm[promNumKeys]) > 0, refPromKeys, refPromKeyLabels,
		len(subjProm[promNumKeys]) > 0, subjPromKeys, subjPromKeyLabels)

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

	d.assertPrometheusExpositionParity(t, refProm, subjProm)
}

// promGauge pairs a PROMETHEUS name with its expected value.
//
// ⚠️ A SLICE, not a map. The superseded departure pin iterated a map literal,
// so its t.Errorf roster reordered between runs; a failure diff that reorders
// is unreadable, and an ordered roster is also what makes "exactly which
// assertions fired" a checkable statement.
type promGauge struct {
	name string
	want uint64
}

var promGauges = []promGauge{
	{name: promNumKeys, want: wantNumKeys},
	{name: promNumLayers, want: wantNumLayers},
}

// assertPrometheusExpositionParity asserts CROSS-SIDE PARITY of the PROMETHEUS
// exposition for the two runtime gauges: present on both sides, same value on
// both sides, and STILL zero-label on both sides.
//
// ⚠️ THIS SUPERSEDES A DEPARTURE PIN, AND THE FLIP IS THE POINT. Through phase
// 78 this function asserted the two names were ABSENT from the SUBJECT's
// /stats/prometheus: internal/stats.ExtractTags did not recognize a `runtime.`
// top-level segment, and internal/stats.WriteProm silently skips any metric
// whose name fails to flatten, so the renderer dropped two correct gauges with
// no log and no error. The pin was written to go RED the day that changed.
// Phase 79 is that day — internal/stats now carries a byte-mirror `runtime.`
// arm — and the observed RED was exactly the two subject-absence branches, so
// the pin discharged its purpose and is replaced rather than deleted.
//
// ⚠️ CONVERTED, NOT DELETED. Deleting in favor of "the generic prometheus
// comparison" is not available: there is no generic prometheus differential in
// the runner, and this fixture's ProbeAdmin returns /ready only. Deleting would
// leave the prometheus projection of these gauges with ZERO assertions on
// EITHER side while reading as cleanup.
//
// ⚠️ THE FLAT-/stats LEGS IN AssertStats ARE DELIBERATELY KEPT AS A SECOND
// SEAM. They are the only thing that distinguishes "the GAUGE is wrong" from
// "the RENDERER is wrong": a flat-endpoint failure alongside a prometheus
// failure indicts the gauge, a prometheus-only failure indicts the projection.
//
// ⚠️ THE LABEL LEG IS NOT DECORATION. Name-and-value parity alone is blind to a
// build that hoists part of an internal name into a prometheus LABEL — the key
// and the summed value both survive that move untouched. Asserting the label
// text is empty is what makes this assertion able to fail on it.
func (*runtimeStaticLayerDriver) assertPrometheusExpositionParity(t fixture.TB, refProm, subjProm map[string][]promSample) {
	t.Helper()

	for _, g := range promGauges {
		refSamples, refOK := refProm[g.name]
		subjSamples, subjOK := subjProm[g.name]

		// ⚠️ THE ABSENT CHECK IS SEPARATE FROM THE VALUE CHECK, and each branch
		// `continue`s — mirroring the flat-endpoint guard in AssertStats. A
		// name that never lands in the map folds to 0, and 0 == 0 through a
		// bare lookup is a VACUOUS pass.
		if !refOK {
			t.Errorf("ref: %s ABSENT from /stats/prometheus — the REFERENCE has always published it "+
				"(measured `%s{} %d` on the pinned image)", g.name, g.name, g.want)
			continue
		}
		if !subjOK {
			t.Errorf("subj: %s ABSENT from /stats/prometheus — phase 79 taught internal/stats the "+
				"`runtime.` top-level segment, so the projection is required to emit it. An absence here "+
				"means the byte-mirror arm was dropped from ExtractTags, or WriteProm is again skipping "+
				"the name silently", g.name)
			continue
		}

		refVal, refLabels := foldPromSamples(refSamples)
		subjVal, subjLabels := foldPromSamples(subjSamples)

		if refVal != g.want {
			t.Errorf("ref %s = %d on /stats/prometheus, want %d", g.name, refVal, g.want)
		}
		if subjVal != g.want {
			t.Errorf("subj %s = %d on /stats/prometheus, want %d", g.name, subjVal, g.want)
		}
		if refVal != subjVal {
			t.Errorf("cross-side mismatch %s on /stats/prometheus: ref=%d subj=%d", g.name, refVal, subjVal)
		}

		// ⚠️ Empty covers BOTH zero-label spellings (`name{} 6` and `name 6`) —
		// see scrapePromSamples. A non-empty label text is a HOISTING change,
		// which every name-keyed check above is blind to.
		if refLabels != "" {
			t.Errorf("ref: %s carries a NON-EMPTY label set %s on /stats/prometheus — this row asserts a "+
				"BYTE-MIRROR projection with no labels on either side", g.name, refLabels)
		}
		if subjLabels != "" {
			t.Errorf("subj: %s carries a NON-EMPTY label set %s on /stats/prometheus — the `runtime.` arm "+
				"is a BYTE-MIRROR arm; hoisting any segment into a label is a cross-side divergence the "+
				"name-and-value legs above cannot see", g.name, subjLabels)
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

// promSample is ONE exposition line for one metric name: the RAW label text
// between `{` and `}`, plus the value.
//
// ⚠️ labels is EMPTY for BOTH zero-label spellings — the reference's
// `envoy_runtime_num_keys{} 6` and envoy-go's brace-less
// `envoy_runtime_num_keys 6`. That normalization is the whole reason the parity
// assertion can run cross-side at all; see scrapePromSamples.
type promSample struct {
	labels string
	value  uint64
}

// foldPromSamples reduces every sample recorded for ONE metric name to the
// SUMMED value — preserving the collide-by-summing semantics this scrape has
// always had — and to the set of NON-EMPTY label texts observed, joined for an
// error message. An empty second return means every sample was zero-label.
func foldPromSamples(samples []promSample) (uint64, string) {
	var sum uint64
	var withLabels []string
	for _, s := range samples {
		sum += s.value
		if s.labels != "" {
			withLabels = append(withLabels, "{"+s.labels+"}")
		}
	}
	return sum, strings.Join(withLabels, " ")
}

// scrapePromSamples issues GET http://<addr>/stats/prometheus and returns, per
// metric NAME, every sample line observed for it — value AND label text.
// Cloned from 0110/driver.go, then made LABEL-AWARE at phase 79.
//
// ⚠️ IT IS LABEL-AWARE ON PURPOSE, AND THAT IS LOAD-BEARING. The previous
// version discarded the `{...}` text entirely and returned map[string]uint64.
// A cross-side parity assertion built on that shape passes SILENTLY against a
// build that starts hoisting part of the internal name into a LABEL: the name
// key and the summed value are both unchanged, so nothing observes the move.
// Returning the label text lets the parity assertion below state the property
// it actually means — same name, same value, and STILL no labels on either
// side.
//
// ⚠️ MEASURED zero-label BRACE DIVERGENCE (executed at this task, both sides,
// same run):
//
//	reference  `envoy_runtime_num_keys{} 6`     — braces present, empty
//	subject    `envoy_runtime_num_keys 6`       — braces OMITTED
//
// so a RAW-LINE comparison of the two expositions is never valid here. Both
// forms must go through the two-branch parser below, which yields labels == ""
// for each. (The subject additionally emits a `# HELP` line the reference does
// not; the `#` skip below absorbs it. A hand-rolled `grep -c` on either name
// would therefore return 2 or 3 per scrape — that bites only a shell gate.)
func scrapePromSamples(adminAddr string) (map[string][]promSample, error) {
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

	out := map[string][]promSample{}
	for _, line := range strings.Split(body.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, labels, rest string
		if open := strings.IndexByte(line, '{'); open >= 0 {
			closeIdx := strings.LastIndexByte(line, '}')
			if closeIdx < open {
				continue // malformed: no closing brace
			}
			name = line[:open]
			// ⚠️ `{}` yields "" here, exactly as the brace-less branch does.
			// That is the deliberate normalization of the measured cross-side
			// brace divergence — NOT a discarded label set.
			labels = strings.TrimSpace(line[open+1 : closeIdx])
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
		out[name] = append(out[name], promSample{labels: labels, value: uint64(v)})
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
