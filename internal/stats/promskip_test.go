package stats

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

// promSkipUnprojectableName is a metric name that NO ExtractTags acceptor
// recognizes, so WriteProm must drop it from the exposition and name it in the
// aggregate skip line.
//
// Unprojectable by construction, re-derived at this tip rather than assumed:
// it matches none of the THIRTEEN top-level prefix detectors live in name.go
// (cluster. http. listener. server. runtime. access_logs. tracing. sds. wasm. in
// the switch; mongo. kafka. redis. thrift. in the default arm) and contains none of
// the four mid-name segments (.http_local_rate_limit. .http_bandwidth_limit.
// .rbac. .zookeeper.). It is also a real Envoy server stat name, so the control
// is not built on an invented shape.
//
// NOTE for future editors: `runtime.num_keys` is NOT a valid choice here. The
// phase-79 byte-mirror arms gave `runtime.` a top-level acceptor, so it now
// projects to envoy_runtime_num_keys and would silently turn leg B vacuous.
// Nor is any `sds.`-rooted name: the phase-80 arm gave `sds.` a top-level
// acceptor too. Any replacement must be re-checked against flattenToProm, not
// against prose.
const promSkipUnprojectableName = "filesystem.flushed_by_timer"

// promSkipProjectingName is the stacked control's OTHER leg: a name that DOES
// project (Rule SN5, byte-mirror residual, no labels). It shares the registry
// with the unprojectable name so the skip line can be checked for what it must
// NOT say as well as for what it must say.
const promSkipProjectingName = "server.live"

// TestWriteProm_SkipLogStackedControl is a STACKED control over WriteProm's
// aggregate skip-report line: one PROJECTING name and one DROPPED name in the
// SAME registry.
//
// A purely positive assertion ("the line mentions the dropped name") cannot
// catch an OVER-firing report — a skip line emitted over every registered name,
// or one line per name, satisfies it. The four legs below are individually
// falsifiable and are each their own t.Errorf, so one failing leg does not make
// the others dead code:
//
//	leg 1  exactly ONE non-empty log line per WriteProm call (aggregation)
//	leg 2  that line NAMES the dropped name (the positive signal)
//	leg 3  that line does NOT name the projecting name (the negative leg)
//	leg 4  the projecting name really IS in the exposition, so leg 3 is not
//	       vacuously satisfied by a metric that was silently dropped too
//
// The test rebinds the process-global log destination and therefore MUST NOT
// call t.Parallel(). internal/stats has exactly one other user of the log
// package — prom.go's skip line itself — so nothing else in the package can
// interleave into the captured buffer.
func TestWriteProm_SkipLogStackedControl(t *testing.T) {
	var logBuf bytes.Buffer
	origFlags := log.Flags()
	origPrefix := log.Prefix()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(origFlags)
		log.SetPrefix(origPrefix)
	})

	r := NewRegistry()
	r.NewGauge(promSkipProjectingName).Set(1)
	r.NewCounter(promSkipUnprojectableName).Inc()

	var out bytes.Buffer
	if err := WriteProm(&out, r); err != nil {
		t.Fatalf("WriteProm returned an error: %v", err)
	}

	var lines []string
	for _, l := range strings.Split(logBuf.String(), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	captured := strings.Join(lines, "\n")

	// Leg 1 — aggregation: exactly one non-empty line, regardless of how many
	// names were skipped. A per-name report fails here.
	if len(lines) != 1 {
		t.Errorf("skip log: got %d non-empty line(s), want exactly 1; captured %q", len(lines), logBuf.String())
	}

	// Leg 2 — the positive signal: the dropped name is reported.
	if !strings.Contains(captured, promSkipUnprojectableName) {
		t.Errorf("skip log does not name the dropped metric %q; captured %q", promSkipUnprojectableName, captured)
	}

	// Leg 3 — the negative leg: a name that projected must NOT be reported. An
	// over-firing report that logs every registered name fails HERE and, by
	// design, only here.
	if strings.Contains(captured, promSkipProjectingName) {
		t.Errorf("skip log names the PROJECTING metric %q, so it over-fires; captured %q", promSkipProjectingName, captured)
	}

	// Leg 4 — liveness cross-check: leg 3 is only meaningful if the projecting
	// name actually reached the exposition. Without this, a WriteProm that
	// dropped both metrics and logged neither would pass leg 3 vacuously.
	const wantExposition = "envoy_server_live 1"
	if !strings.Contains(out.String(), wantExposition) {
		t.Errorf("exposition is missing %q, so the negative leg is vacuous; exposition:\n%s", wantExposition, out.String())
	}
}
