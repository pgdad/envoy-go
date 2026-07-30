package xds

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// walkNames returns the set of metric names currently registered in reg, via
// Registry.Walk (the registry exposes no Names()/Len() accessor — confirmed
// against internal/stats/registry.go: the only enumeration surface is Walk).
func walkNames(reg *stats.Registry) map[string]bool {
	names := make(map[string]bool)
	reg.Walk(func(m stats.Metric) {
		names[m.Name()] = true
	})
	return names
}

func TestRegisterSDSStats_FiveCounterDelta(t *testing.T) {
	reg := stats.NewRegistry()
	before := walkNames(reg)

	RegisterSDSStats(reg, "server_cert")

	after := walkNames(reg)
	if len(after) != len(before)+5 {
		t.Fatalf("registered %d new names, want 5 (before=%d after=%d)", len(after)-len(before), len(before), len(after))
	}

	wantSuffixes := []string{
		"update_success",
		"update_failure",
		"update_rejected",
		"update_attempt",
		"init_fetch_timeout",
	}
	for _, suffix := range wantSuffixes {
		name := "sds.server_cert." + suffix
		if !after[name] {
			t.Errorf("expected registered name %q, not found", name)
		}
	}
}

func TestRegisterSDSStats_Idempotent(t *testing.T) {
	reg := stats.NewRegistry()

	s1 := RegisterSDSStats(reg, "server_cert")
	if s1 == nil {
		t.Fatal("first RegisterSDSStats returned nil")
	}
	afterFirst := walkNames(reg)

	s2 := RegisterSDSStats(reg, "server_cert")
	if s2 == nil {
		t.Fatal("second RegisterSDSStats returned nil")
	}
	afterSecond := walkNames(reg)

	if len(afterSecond) != len(afterFirst) {
		t.Fatalf("second registration added %d names, want 0 (dedup via NewCounterIfAbsent); afterFirst=%d afterSecond=%d",
			len(afterSecond)-len(afterFirst), len(afterFirst), len(afterSecond))
	}
	if len(afterFirst) != 5 {
		t.Fatalf("first registration produced %d names, want 5", len(afterFirst))
	}
}

func TestRegisterSDSStats_InvalidNameSkipsNoPanic(t *testing.T) {
	reg := stats.NewRegistry()
	before := walkNames(reg)

	var s *SDSStats
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("RegisterSDSStats panicked on invalid secret name: %v", r)
			}
		}()
		s = RegisterSDSStats(reg, "bad name!")
	}()

	if s == nil {
		t.Fatal("RegisterSDSStats returned nil *SDSStats for invalid name, want non-nil no-op")
	}

	after := walkNames(reg)
	if len(after) != len(before) {
		t.Fatalf("invalid secret name registered %d counters, want 0", len(after)-len(before))
	}

	// Nil-safe no-op increments must not panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("increment helper panicked on no-op *SDSStats: %v", r)
			}
		}()
		s.incUpdateSuccess()
		s.incUpdateFailure()
		s.incUpdateRejected()
		s.incUpdateAttempt()
		s.incInitFetchTimeout()
	}()
}

func TestRegisterSDSStats_RecordDispatch(t *testing.T) {
	reg := stats.NewRegistry()
	s := RegisterSDSStats(reg, "server_cert")

	s.incUpdateSuccess()

	if got, want := s.updateSuccess.Load(), uint64(1); got != want {
		t.Errorf("updateSuccess.Load() = %d, want %d", got, want)
	}
	if got, want := s.updateFailure.Load(), uint64(0); got != want {
		t.Errorf("updateFailure.Load() = %d, want %d", got, want)
	}
	if got, want := s.updateRejected.Load(), uint64(0); got != want {
		t.Errorf("updateRejected.Load() = %d, want %d", got, want)
	}
	if got, want := s.updateAttempt.Load(), uint64(0); got != want {
		t.Errorf("updateAttempt.Load() = %d, want %d", got, want)
	}
	if got, want := s.initFetchTimeout.Load(), uint64(0); got != want {
		t.Errorf("initFetchTimeout.Load() = %d, want %d", got, want)
	}
}

// TestRegisterSDSStats_NilReceiverSafe covers the *SDSStats nil-pointer path
// (not just nil-field counters): a nil *SDSStats' increment helpers must also
// be no-ops.
func TestRegisterSDSStats_NilReceiverSafe(t *testing.T) {
	var s *SDSStats
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("increment helper on nil *SDSStats panicked: %v", r)
		}
	}()
	s.incUpdateSuccess()
	s.incUpdateFailure()
	s.incUpdateRejected()
	s.incUpdateAttempt()
	s.incInitFetchTimeout()
}

// ---------------------------------------------------------------------------
// Phase 80 T7 — the RegisterSDSStats skip branch is UNREACHABLE from
// production config.
//
// Phase 80 added a boot-boundary reject (internal/boot's validateSDSSecretName)
// that runs before this function is ever called on the single production path.
// The skip branch above is therefore no longer live logic: it is documented
// DEFENSE IN DEPTH, and this block is what documents it by measurement.
//
// Nothing here proposes deleting the skip branch or the nil-safe increments.
// A nil *stats.Counter Inc is a PROCESS CRASH with no recover(), so the second
// layer stays, and TestRegisterSDSStats_InvalidNameSkipsNoPanic above -- which
// still drives the "bad name!" case straight through this function -- stays
// with it.
// ---------------------------------------------------------------------------

// bootGuardMirror mirrors internal/boot's validateSDSSecretName. It is a
// duplicate rather than a call because internal/boot imports internal/xds, so
// the reverse edge would be an import cycle.
//
// The duplicate is tied to the original by a shared, byte-pinned artifact
// rather than by inspection: sdsBootAcceptedPrintableBytes below is the same
// literal internal/boot's own sweep asserts its REAL guard produces. If either
// side drifts, that side's sweep goes red.
func bootGuardMirror(name string) bool {
	assembled := "sds." + name + ".init_fetch_timeout"
	for _, seg := range strings.Split(assembled, ".") {
		if seg == "" {
			return false
		}
	}
	return stats.IsValidName(assembled)
}

// sdsBootAcceptedPrintableBytes is byte-identical to
// internal/boot.sdsAcceptedPrintableBytes.
const sdsBootAcceptedPrintableBytes = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz"

// counterPtrs returns the five counter pointers in registration order. The
// POINTERS are what this test asserts on, not the registry's name set: a name
// present in the registry and a non-nil field are separate facts, and it is
// the nil FIELD that crashes the process when incremented.
func counterPtrs(s *SDSStats) []*stats.Counter {
	return []*stats.Counter{
		s.updateSuccess, s.updateFailure, s.updateRejected, s.updateAttempt, s.initFetchTimeout,
	}
}

func TestRegisterSDSStats_SkipBranchUnreachableFromBootGuard(t *testing.T) {
	denominator, allFive, allNil, partial := 0, 0, 0, 0
	var mirrorAccepted []string
	var mismatches []string
	var acceptedButNotAllFive []string

	for b := 32; b < 127; b++ {
		denominator++
		name := string(rune(b))

		reg := stats.NewRegistry()
		s := RegisterSDSStats(reg, name)
		if s == nil {
			t.Errorf("RegisterSDSStats(%q) returned a nil *SDSStats", name)
			continue
		}

		nonNil := 0
		for _, p := range counterPtrs(s) {
			if p != nil {
				nonNil++
			}
		}
		switch nonNil {
		case 5:
			allFive++
		case 0:
			allNil++
		default:
			partial++
		}

		guardAccepts := bootGuardMirror(name)
		if guardAccepts {
			mirrorAccepted = append(mirrorAccepted, name)
			// THE CLAIM: every name the boot guard lets through
			// registers all five counters, so the skip branch cannot
			// fire on a config that reached this call.
			if nonNil != 5 {
				acceptedButNotAllFive = append(acceptedButNotAllFive,
					fmt.Sprintf("%q -> %d/5 non-nil", name, nonNil))
			}
		}

		// Containment: guard-reject must be a SUPERSET of skip-all. The
		// guard may be stricter (it is, on exactly one byte); it must never
		// be laxer, or a name would pass boot and then silently skip.
		if !guardAccepts && nonNil == 5 {
			mismatches = append(mismatches, fmt.Sprintf("%q (guard REJECTS, register-all-five)", name))
		}
		if guardAccepts && nonNil == 0 {
			t.Errorf("CONTAINMENT VIOLATED: %q passes the boot guard but registers NOTHING", name)
		}
	}

	if denominator != 95 {
		t.Errorf("swept %d printable bytes, want 95", denominator)
	}
	if allFive != 64 {
		t.Errorf("allFive = %d, want 64", allFive)
	}
	if allNil != 31 {
		t.Errorf("allNil = %d, want 31", allNil)
	}
	if partial != 0 {
		t.Errorf("partial = %d, want 0: a PARTIAL registration is the shape that leaves a nil "+
			"counter reachable behind a non-nil sibling", partial)
	}
	if len(acceptedButNotAllFive) != 0 {
		t.Errorf("names accepted by the boot guard that did NOT register all five: %v",
			acceptedButNotAllFive)
	}
	// Exactly one byte is rejected by the guard yet registers cleanly: ".",
	// which assembles to "sds...init_fetch_timeout" -- a name the stats
	// charset regex ACCEPTS (dots are legal characters) but whose interior
	// empty segment the guard's segment leg refuses.
	if want := []string{`"." (guard REJECTS, register-all-five)`}; len(mismatches) != 1 || mismatches[0] != want[0] {
		t.Errorf("guard-stricter-than-skip set = %v, want exactly %v", mismatches, want)
	}
	if got := strings.Join(mirrorAccepted, ""); got != sdsBootAcceptedPrintableBytes {
		t.Errorf("mirror-accepted byte set = %q, want %q (this literal is pinned identically in "+
			"internal/boot; a mismatch means the mirror drifted from the real guard)",
			got, sdsBootAcceptedPrintableBytes)
	}
}

// TestRegisterSDSStats_CorpusSecretNamesRegisterAllFive is the concrete
// counterpart to the sweep: the four secret names the fixture corpus actually
// carries all register five non-nil pointers, so the reject is a no-op on the
// corpus and the skip branch never fires there either.
func TestRegisterSDSStats_CorpusSecretNamesRegisterAllFive(t *testing.T) {
	for _, name := range []string{"server_cert", "validation_ca", "rccf_validation_ca", "edf_validation_ca"} {
		if !bootGuardMirror(name) {
			t.Errorf("corpus secret name %q is rejected by the boot guard; the reject is NOT a "+
				"no-op on the corpus", name)
		}
		reg := stats.NewRegistry()
		s := RegisterSDSStats(reg, name)
		if s == nil {
			t.Errorf("RegisterSDSStats(%q) returned a nil *SDSStats", name)
			continue
		}
		for i, p := range counterPtrs(s) {
			if p == nil {
				t.Errorf("RegisterSDSStats(%q): counter pointer %d is nil; incrementing it is a "+
					"process crash with no recover()", name, i)
			}
		}
	}
}
