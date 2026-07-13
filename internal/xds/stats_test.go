package xds

import (
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
