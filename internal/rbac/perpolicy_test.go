package rbac

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// findCounter scans the Registry for a counter with the given name.
// Returns nil if not present.
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

func TestPerPolicyCounters_IncLazyAllocatesPolicySegment(t *testing.T) {
	reg := stats.NewRegistry()
	pc := &PerPolicyCounters{}
	pc.Inc(reg, "http.hcm.rbac.p", "policy_a", "allowed")
	pc.Inc(reg, "http.hcm.rbac.p", "policy_a", "allowed") // idempotent registration, 2 increments
	got := findCounter(reg, "http.hcm.rbac.p.policy.policy_a.allowed")
	if got == nil || got.Load() != 2 {
		t.Fatalf("per-policy counter name/value wrong: %v", got)
	}
}

func TestPerPolicyCounters_NilRegOrEmptyPolicyIsNoOp(t *testing.T) {
	pc := &PerPolicyCounters{}
	pc.Inc(nil, "b", "p", "allowed")                // nil reg → no-op (no panic)
	pc.Inc(stats.NewRegistry(), "b", "", "allowed") // empty policy name → no-op
}

// ----------------------------------------------------------------------------
// Group 81-F2 — the request-time skip-and-log backstop inside Inc.
//
// F2 exists because the MATCHER engine has NO boot-time policy-name
// enumeration: BuildMatcherEngine only allow-lists the terminal Any TypeURL,
// and Evaluate returns action.GetName() read out of the match tree at REQUEST
// time. The HTTP consumer's boot guard (source F1) covers the rules engine
// only, so a matcher-supplied Action.name arrives here having passed no
// charset check at all. Without the skip, NewCounterIfAbsent ->
// stats.checkName PANICS on the HCM dispatch goroutine, which carries no
// recover(), and the process dies.
//
// This table is NECESSARILY separate from the consumer-side F1 table: that one
// lives in internal/filter/http/rbac and the two packages cannot share it.
//
// NOTE: no test in this group calls recover(). If the backstop is removed the
// test BINARY dies with
// `panic: stats: invalid metric name: "http.hcm.rbac.rbac.policy.allow-admins.allowed"`.
// ----------------------------------------------------------------------------

// perPolicyNameCase is one row of the F2 charset table. Every verdict is
// stated against the ASSEMBLED key `<base>.policy.<name>.<suffix>`, never the
// bare name.
type perPolicyNameCase struct {
	name     string // subtest name
	policy   string // the policy name reaching Inc
	register bool   // true when Inc must register + increment a counter
}

// perPolicyNameCases is the 29-row F2 table: 17 register + 12 skip. It mirrors
// the consumer-side row set because the policy name occupies the same interior
// segment position in both.
var perPolicyNameCases = []perPolicyNameCase{
	{"Plain", "simple", true},
	{"Underscore", "with_underscore", true},
	{"MixedCase", "MixedCase", true},
	{"TrailingDigits", "digits123", true},
	{"LeadingDigit", "9leading_digit", true}, // INTERIOR position: bare probe would reject
	{"LeadingUnderscore", "_leading_underscore", true},
	{"SingleChar", "x", true},
	{"SingleDigit", "9", true},
	{"SingleUnderscore", "_", true},
	{"AllDigits", "0123456789", true},
	{"TrailingUnderscore", "trailing_", true},
	{"MixedAlnumDot", "A_B.c9", true},
	{"InteriorDot", "a.b", true},
	{"InteriorEmptySegment", "a..b", true}, // ACCEPT-PIN SPEC 13.1, deferred to stats-name-empty-segment-guards
	{"TrailingDot", "trailing.", true},     // ACCEPT-PIN: assembles to an INTERIOR double dot
	{"LeadingDot", ".leading", true},       // ACCEPT-PIN: same
	{"BareDot", ".", true},                 // ACCEPT-PIN: same
	{"Hyphen", "has-hyphen", false},
	{"IdiomaticHyphen", "allow-admins", false}, // the headline defect
	{"Space", "has space", false},
	{"Slash", "has/slash", false},
	{"Colon", "has:colon", false},
	{"Percent", "has%percent", false},
	{"Star", "has*star", false},
	{"Comma", "has,comma", false},
	{"Pipe", "has|pipe", false},
	{"Semicolon", "has;semicolon", false},
	{"Tab", "has\ttab", false},
	{"NonASCII", "café", false},
}

// TestPerPolicyCounters_Inc_NameCharset drives the F2 table. On the skip rows
// it asserts BOTH that the specific name is absent AND that the registry total
// is unchanged — a name-only assertion would miss a MIS-ASSEMBLED registration
// landing under some other key.
func TestPerPolicyCounters_Inc_NameCharset(t *testing.T) {
	for _, tc := range perPolicyNameCases {
		t.Run(tc.name, func(t *testing.T) {
			reg := stats.NewRegistry()
			pc := &PerPolicyCounters{}
			const base = "http.hcm.rbac.rbac"
			key := base + ".policy." + tc.policy + ".allowed"
			pc.Inc(reg, base, tc.policy, "allowed") // must NOT panic; no recover() here
			got := findCounter(reg, key)
			total := 0
			reg.Walk(func(stats.Metric) { total++ })
			if tc.register {
				if got == nil {
					t.Fatalf("policy %q: want counter %q registered, got none", tc.policy, key)
				}
				if got.Load() != 1 {
					t.Errorf("counter %q value: got %d, want 1", key, got.Load())
				}
				if total != 1 {
					t.Errorf("registry size: got %d, want 1", total)
				}
				return
			}
			if got != nil {
				t.Errorf("policy %q: counter %q was registered; want skipped", tc.policy, key)
			}
			if total != 0 {
				t.Errorf("registry size: got %d, want 0 (nothing may be registered under ANY key)", total)
			}
		})
	}
}

// TestPerPolicyCounters_Inc_UnNameableIsNotACachePoison pins that a skipped
// name leaves the lazy cache untouched, so a LATER nameable emission through
// the same PerPolicyCounters still registers normally (the skip must not be a
// sticky failure mode for the whole struct).
func TestPerPolicyCounters_Inc_UnNameableIsNotACachePoison(t *testing.T) {
	reg := stats.NewRegistry()
	pc := &PerPolicyCounters{}
	pc.Inc(reg, "http.hcm.rbac.rbac", "allow-admins", "allowed")
	pc.Inc(reg, "http.hcm.rbac.rbac", "allow_admins", "allowed")
	if c := findCounter(reg, "http.hcm.rbac.rbac.policy.allow_admins.allowed"); c == nil || c.Load() != 1 {
		t.Fatalf("nameable sibling after a skip: got %v, want a counter at 1", c)
	}
	total := 0
	reg.Walk(func(stats.Metric) { total++ })
	if total != 1 {
		t.Errorf("registry size: got %d, want exactly 1", total)
	}
}

// TestPerPolicyCounters_Inc_SkipDiagnosticIsAggregated is the aggregated-
// diagnostic assertion: EXACTLY 2 log lines over 150 Inc calls spread across 2
// call sites (the primary base and the shadow base, as emitPrimaryCounters /
// emitShadowCounters would drive them). One line per request would be a
// request-rate log flood on a hot deny path.
//
// This shape is BUDGETED, not copied: all four landed guard-skip precedents in
// this tree are silent. The log-capture mechanics follow
// internal/stats/promskip_test.go's TestWriteProm_SkipLogStackedControl.
//
// Rebinds the process-global log destination; MUST NOT call t.Parallel().
// internal/rbac has exactly one other user of the log package — none — so
// nothing else in the package can interleave into the captured buffer.
func TestPerPolicyCounters_Inc_SkipDiagnosticIsAggregated(t *testing.T) {
	var logBuf bytes.Buffer
	origFlags, origPrefix := log.Flags(), log.Prefix()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(origFlags)
		log.SetPrefix(origPrefix)
	})

	reg := stats.NewRegistry()
	pc := &PerPolicyCounters{}
	const (
		primaryBase = "http.hcm.rbac.rp" // emitPrimaryCounters call site
		shadowBase  = "http.hcm.rbac.sh" // emitShadowCounters call site
		policy      = "allow-admins"     // un-nameable
		perSite     = 75                 // 2 sites x 75 = 150 Inc calls
	)
	for i := 0; i < perSite; i++ {
		pc.Inc(reg, primaryBase, policy, "allowed")
		pc.Inc(reg, shadowBase, policy, "shadow_allowed")
	}

	var lines []string
	for _, l := range strings.Split(logBuf.String(), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines over %d Inc calls at 2 call sites, want 2: %q", len(lines), perSite*2, lines)
	}
	want := fmt.Sprintf(logSkipInvalidPolicyNameFmt, policy)
	for i, l := range lines {
		if l != want {
			t.Errorf("line %d = %q; want %q", i, l, want)
		}
	}
	total := 0
	reg.Walk(func(stats.Metric) { total++ })
	if total != 0 {
		t.Errorf("registry size: got %d, want 0", total)
	}
}

// TestPerPolicyCounters_Inc_ValidNameEmitsNoDiagnostic is the stacked control
// for the arm above: the diagnostic must not fire on the happy path. Without
// it, a guard that logged unconditionally would still read as "2 lines" if the
// dedupe key happened to collapse.
//
// Rebinds the process-global log destination; MUST NOT call t.Parallel().
func TestPerPolicyCounters_Inc_ValidNameEmitsNoDiagnostic(t *testing.T) {
	var logBuf bytes.Buffer
	origFlags, origPrefix := log.Flags(), log.Prefix()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(origFlags)
		log.SetPrefix(origPrefix)
	})

	reg := stats.NewRegistry()
	pc := &PerPolicyCounters{}
	for i := 0; i < 150; i++ {
		pc.Inc(reg, "http.hcm.rbac.rp", "allow_admins", "allowed")
	}
	if logBuf.String() != "" {
		t.Errorf("log = %q; want empty (nameable policy must be silent)", logBuf.String())
	}
	if c := findCounter(reg, "http.hcm.rbac.rp.policy.allow_admins.allowed"); c == nil || c.Load() != 150 {
		t.Fatalf("counter: got %v, want 150 increments", c)
	}
}

// TestPerPolicyCounters_Inc_ConcurrentUnNameableStillLogsOnce pins the dedupe
// barrier under concurrency: sync.Map.LoadOrStore is the race-safe gate, so
// 100 goroutines hammering ONE un-nameable key still produce ONE line.
//
// Rebinds the process-global log destination; MUST NOT call t.Parallel().
func TestPerPolicyCounters_Inc_ConcurrentUnNameableStillLogsOnce(t *testing.T) {
	var logBuf bytes.Buffer
	var mu sync.Mutex
	origFlags, origPrefix := log.Flags(), log.Prefix()
	log.SetOutput(&lockedWriter{w: &logBuf, mu: &mu})
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(origFlags)
		log.SetPrefix(origPrefix)
	})

	reg := stats.NewRegistry()
	pc := &PerPolicyCounters{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pc.Inc(reg, "http.hcm.rbac.rp", "allow-admins", "allowed")
		}()
	}
	wg.Wait()

	mu.Lock()
	got := logBuf.String()
	mu.Unlock()
	var lines []string
	for _, l := range strings.Split(got, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) != 1 {
		t.Errorf("got %d lines over 100 concurrent Inc calls at 1 call site, want 1: %q", len(lines), lines)
	}
}

// lockedWriter serializes writes into the capture buffer so the concurrent arm
// above is itself race-clean under -race.
type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
