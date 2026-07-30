package boot

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/test/helpers/sdsserver"
)

// G5 — the STACKED skip-line invariant for the sds.* family.
//
// "Zero skips" ALONE is worthless here: it is satisfied just as well by
// "nothing was ever registered". That is not hypothetical, it is the exact
// ambiguity this row exists to remove -- before the projection arm landed, a
// working registration and a silently-skipped one produced a BYTE-IDENTICAL
// /stats/prometheus exposition. So the invariant is two legs, asserted
// together:
//
//  1. after a clean SDS boot the aggregated skip line names ZERO sds.* entries,
//     AND
//  2. the five projected names are PRESENT in the exposition.
//
// Either leg alone is vacuous. Two further controls below keep the MATCHERS
// themselves honest, because an empty match is not a zero result until you have
// measured the input.

// skipLineMarker is the stable tail of internal/stats' aggregate skip-report
// line (promSkipLogFmt, unexported there). Everything after it is the sorted,
// ", "-joined list of skipped metric names.
const skipLineMarker = "with no recognized top-level segment: "

// skipLineSep matches internal/stats' promSkipLogSep.
const skipLineSep = ", "

// captureWriteProm runs stats.WriteProm over reg, returning the exposition and
// whatever WriteProm logged (the aggregate skip line, when there is one).
func captureWriteProm(t *testing.T, reg *stats.Registry) (exposition string, logged string) {
	t.Helper()
	var logBuf bytes.Buffer
	prevOut, prevFlags, prevPrefix := log.Writer(), log.Flags(), log.Prefix()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	}()

	var out bytes.Buffer
	if err := stats.WriteProm(&out, reg); err != nil {
		t.Fatalf("stats.WriteProm: %v", err)
	}
	return out.String(), logBuf.String()
}

// skippedNames returns the metric names named on the aggregate skip line, or
// nil when no skip line was emitted.
func skippedNames(logged string) []string {
	i := strings.Index(logged, skipLineMarker)
	if i < 0 {
		return nil
	}
	tail := strings.TrimSpace(logged[i+len(skipLineMarker):])
	if tail == "" {
		return nil
	}
	if nl := strings.IndexByte(tail, '\n'); nl >= 0 {
		tail = tail[:nl]
	}
	return strings.Split(tail, skipLineSep)
}

// sdsFamilyLines returns the exposition's metric lines belonging to the sds.*
// family. sds_cluster and ssl_context_update_by_sds are EXCLUDED: a loose
// "sds" substring match over-counts badly, because the SDS transport cluster's
// own stats carry envoy_cluster_name="sds_cluster".
func sdsFamilyLines(exposition string) []string {
	var out []string
	for _, line := range strings.Split(exposition, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "sds") {
			continue
		}
		if strings.Contains(line, "sds_cluster") || strings.Contains(line, "ssl_context_update_by_sds") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// bootSDSForStats stands up an SDS server, builds the provider and performs the
// initial fetch -- a clean SDS boot -- returning the boot registry.
func bootSDSForStats(t *testing.T) *stats.Registry {
	t.Helper()
	certPEM, keyPEM, _ := genLeafSelfSignedCert(t)
	srv := sdsserver.New(t, sdsserver.WithSecret("server_cert", certPEM, keyPEM))

	_, portStr, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("net.SplitHostPort(%q): %v", srv.Addr(), err)
	}
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("strconv.ParseUint(%q): %v", portStr, err)
	}

	yaml := fmt.Sprintf(sdsListenerYAMLTemplate, "test-node", "test-cluster", grpcSdsConfigFlow, uint32(port))
	bs, dialer := loadSDSBootstrapAndDialer(t, yaml)

	provider, err := NewSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
	if err != nil {
		t.Fatalf("NewSDSProvider: %v", err)
	}
	if provider == nil {
		t.Fatalf("NewSDSProvider: got nil provider, want non-nil")
	}
	if _, err := provider.FetchInitialCertificate(context.Background(), "server_cert"); err != nil {
		t.Fatalf("FetchInitialCertificate: %v", err)
	}
	return bs.Stats
}

func TestSDSStatsProjection_NoSDSSkips_AndFiveNamesPresent(t *testing.T) {
	reg := bootSDSForStats(t)
	exposition, logged := captureWriteProm(t, reg)

	// LEG 1 -- the aggregated skip line names ZERO sds.* entries.
	var sdsSkipped []string
	for _, name := range skippedNames(logged) {
		if strings.HasPrefix(name, "sds.") {
			sdsSkipped = append(sdsSkipped, name)
		}
	}
	if len(sdsSkipped) != 0 {
		t.Errorf("LEG 1: the WriteProm skip line names %d sds.* entries %v, want 0",
			len(sdsSkipped), sdsSkipped)
	}

	// LEG 2 -- the five projected names are PRESENT. Without this leg, LEG 1
	// is satisfied by a registry that never registered anything at all.
	wantNames := []string{
		"envoy_sds_init_fetch_timeout",
		"envoy_sds_update_attempt",
		"envoy_sds_update_failure",
		"envoy_sds_update_rejected",
		"envoy_sds_update_success",
	}
	const wantLabel = `{envoy_xds_resource_name="server_cert"}`

	family := sdsFamilyLines(exposition)
	got := make(map[string]string, len(family))
	for _, line := range family {
		if i := strings.IndexAny(line, "{ "); i > 0 {
			got[line[:i]] = line
		}
	}
	var missing, unlabeled []string
	for _, name := range wantNames {
		line, ok := got[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		if !strings.Contains(line, wantLabel) {
			unlabeled = append(unlabeled, line)
		}
	}
	// Report missing and extra SEPARATELY -- a bare count is blind to a rename.
	var extra []string
	want := make(map[string]bool, len(wantNames))
	for _, n := range wantNames {
		want[n] = true
	}
	for n := range got {
		if !want[n] {
			extra = append(extra, n)
		}
	}
	sort.Strings(extra)
	if len(missing) != 0 {
		t.Errorf("LEG 2: sds family names MISSING from /stats/prometheus: %v", missing)
	}
	if len(extra) != 0 {
		t.Errorf("LEG 2: unexpected EXTRA sds family names in /stats/prometheus: %v", extra)
	}
	if len(unlabeled) != 0 {
		t.Errorf("LEG 2: sds family lines missing the hoisted label %s: %v", wantLabel, unlabeled)
	}

	// CONTROL A -- matcher non-vacuity in the family-absent direction. The
	// SAME exposition carries a dozen lines that a loose "sds" substring
	// match would sweep up, every one of them the SDS transport cluster's own
	// stats. So when the narrowed matcher returns nothing, that is a real
	// zero and not a broken matcher.
	loose, clusterLabeled := 0, 0
	for _, line := range strings.Split(exposition, "\n") {
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "sds") {
			continue
		}
		loose++
		if strings.Contains(line, `envoy_cluster_name="sds_cluster"`) {
			clusterLabeled++
		}
	}
	if clusterLabeled == 0 {
		t.Errorf("CONTROL A: zero lines carry envoy_cluster_name=%q -- the loose matcher found "+
			"nothing to over-count, so it cannot demonstrate that the narrowed matcher works",
			"sds_cluster")
	}
	if loose <= len(family) {
		t.Errorf("CONTROL A: loose sds matches = %d, narrowed = %d; the loose matcher must "+
			"over-count, otherwise the exclusion list is untested", loose, len(family))
	}
	if got, want := loose-clusterLabeled, len(family); got != want {
		t.Errorf("CONTROL A: loose(%d) - sds_cluster-labeled(%d) = %d, want %d (the narrowed "+
			"family size); an unaccounted line means the exclusion list is incomplete",
			loose, clusterLabeled, got, want)
	}
}

// TestSDSStatsProjection_SkipLineParserFindsAnSDSSkip is CONTROL B: the
// skip-line parser used by LEG 1 must actually be able to attribute a skip to
// the sds family. Without it, LEG 1 passes on a parser that never matches
// anything.
//
// "sds.nodots" is registered-but-unprojectable by construction: it is a valid
// stats name, but the projection arm needs an sds.<secret>.<rest> shape and
// rejects a single trailing segment.
func TestSDSStatsProjection_SkipLineParserFindsAnSDSSkip(t *testing.T) {
	reg := stats.NewRegistry()
	reg.NewCounter("sds.nodots")
	reg.NewCounter("cluster.backend.upstream_rq_total")

	exposition, logged := captureWriteProm(t, reg)

	names := skippedNames(logged)
	if len(names) == 0 {
		t.Fatalf("CONTROL B: no skip line parsed from WriteProm's log output %q; LEG 1's parser "+
			"cannot distinguish 'no sds skips' from 'parses nothing'", logged)
	}
	var sdsSkipped []string
	for _, n := range names {
		if strings.HasPrefix(n, "sds.") {
			sdsSkipped = append(sdsSkipped, n)
		}
	}
	if want := []string{"sds.nodots"}; len(sdsSkipped) != 1 || sdsSkipped[0] != want[0] {
		t.Errorf("CONTROL B: sds-attributed skips = %v, want %v", sdsSkipped, want)
	}
	// And the unprojectable name really is absent from the exposition, which
	// is why the skip line is the only place it can be observed.
	if strings.Contains(exposition, "nodots") {
		t.Errorf("CONTROL B: %q appears in the exposition; it was expected to be skipped", "sds.nodots")
	}
	// The projectable sibling must still be emitted, so the skip is scoped.
	if !strings.Contains(exposition, "envoy_cluster_upstream_rq_total") {
		t.Errorf("CONTROL B: the projectable sibling is missing from the exposition; the skip " +
			"was not scoped to the unprojectable name")
	}
}
