package xds

import (
	"fmt"

	"github.com/pgdad/envoy-go/internal/stats"
)

// SDSStats is the 5-counter sds.<secret>.* subset (SPEC §7). Registered per-secret
// at Provider construction. All fields may be nil (invalid secret name skipped the
// registration) — every increment helper is nil-safe.
type SDSStats struct {
	updateSuccess    *stats.Counter
	updateFailure    *stats.Counter
	updateRejected   *stats.Counter
	updateAttempt    *stats.Counter
	initFetchTimeout *stats.Counter
}

// RegisterSDSStats registers (idempotently) the 5 sds.<secretName>.* counters.
// The secretName segment is operator-controlled, so each composed name is checked
// with stats.IsValidName BEFORE NewCounterIfAbsent (which PANICS on an invalid
// name — reference_dynamic_stat_name_charset_guard). On an invalid name the whole
// set is skipped and a nil-populated *SDSStats (no-op increments) is returned.
func RegisterSDSStats(reg *stats.Registry, secretName string) *SDSStats {
	s := &SDSStats{}
	get := func(suffix string) *stats.Counter {
		name := fmt.Sprintf("sds.%s.%s", secretName, suffix)
		if !stats.IsValidName(name) {
			return nil
		}
		return reg.NewCounterIfAbsent(name)
	}
	s.updateSuccess = get("update_success")
	s.updateFailure = get("update_failure")
	s.updateRejected = get("update_rejected")
	s.updateAttempt = get("update_attempt")
	s.initFetchTimeout = get("init_fetch_timeout")
	return s
}

func incNil(c *stats.Counter) {
	if c != nil {
		c.Inc()
	}
}

func (s *SDSStats) incUpdateSuccess() {
	if s != nil {
		incNil(s.updateSuccess)
	}
}

func (s *SDSStats) incUpdateFailure() {
	if s != nil {
		incNil(s.updateFailure)
	}
}

func (s *SDSStats) incUpdateRejected() {
	if s != nil {
		incNil(s.updateRejected)
	}
}

func (s *SDSStats) incUpdateAttempt() {
	if s != nil {
		incNil(s.updateAttempt)
	}
}

func (s *SDSStats) incInitFetchTimeout() {
	if s != nil {
		incNil(s.initFetchTimeout)
	}
}
