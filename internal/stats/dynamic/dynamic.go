package dynamic

// File dynamic.go — per-plugin dynamic-stats Registry materializing the
// API surface per 25.2 SPEC §3.3 + ADR-0208 + AMEND-B2 + R-25.2-7. The
// package-level doc lives in doc.go (covers the AMEND-B2 namespace pin,
// the signedness pin, the Histogram-stub ADR-0060 disposition, the
// thread-safety contract, and the envoy-go-strict cap discipline per
// Q9).

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
	"sync/atomic"

	"github.com/pgdad/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Wire-shape constants per AMEND-B2 byte-pin.
// -----------------------------------------------------------------------------

// namespacePrefix is the proxy-wasm dynamic-stats namespace prefix per
// AMEND-B2 (upstream Envoy v1.37.2
// source/extensions/common/wasm/stats_handler.h:16
// `constexpr absl::string_view CustomStatNamespace = "wasmcustom"`).
// The wire-perspective stat name surfaced to the proxy-wasm guest +
// to EnumerateForAdmin's callback is `namespacePrefix + "." + name`.
const namespacePrefix = "wasmcustom"

// -----------------------------------------------------------------------------
// User-name validation — mirrors parent stats.nameRE but is local to the
// dynamic-stats package so the proxy-wasm guest's raw user name can be
// validated at the boundary before composing the full underlying stats
// name. Parent stats.nameRE rejects empty + leading-dot + trailing-dot +
// non-alphanumeric-non-underscore-non-dot.
// -----------------------------------------------------------------------------

// userNameRE is the validation regex applied to every user-supplied
// custom name at Register. Matches parent stats.IsValidName's regex:
// ASCII-letter-or-underscore prefix followed by ASCII-alphanumerics,
// underscores, and dots; trailing dot rejected.
var userNameRE = regexp.MustCompile(`^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`)

// -----------------------------------------------------------------------------
// Public types per 25.2 SPEC §3.3.
// -----------------------------------------------------------------------------

// MetricID is an opaque token returned from Register. Allocated
// monotonically-increasing from 1 (zero is reserved as a sentinel meaning
// "unassigned" so callers using zero-valued MetricID variables can detect
// uninitialized state).
type MetricID uint32

// MetricType identifies counter / gauge / histogram per AMEND-B2 byte-pin.
type MetricType int

const (
	// MetricTypeCounter identifies a monotonically-increasing counter per
	// AMEND-B2 byte-pin (= 0).
	MetricTypeCounter MetricType = iota // = 0

	// MetricTypeGauge identifies a signed-integer point-in-time gauge per
	// AMEND-B2 byte-pin (= 1). Permits negative deltas via Increment.
	MetricTypeGauge // = 1

	// MetricTypeHistogram identifies a histogram per AMEND-B2 byte-pin
	// (= 2). At 25.2 the underlying primitive is an in-package
	// atomic.Uint64 stub storing the latest Record-ed value; ADR-0060
	// defers the full Histogram primitive to a future sub-phase. See
	// doc.go for the disposition rationale.
	MetricTypeHistogram // = 2
)

// -----------------------------------------------------------------------------
// Sentinel errors. Callers use errors.Is to discriminate without parsing
// error strings.
// -----------------------------------------------------------------------------

var (
	// ErrCapExceeded is returned by Register when adding a NEW entry
	// would exceed the per-Registry maxEntries cap. Idempotent re-Register
	// of an EXISTING name bypasses the cap check (no new entry allocated).
	// Wire-up at Task 12 bumps the wasm.<plugin>.dynamic_stats_cap_exceeded
	// envoy-go-strict counter per 25.2 SPEC §7.1 #8 on this error.
	ErrCapExceeded = errors.New("dynamic: registry cap exceeded")

	// ErrBadArgument is returned for cross-type operations + invalid
	// MetricType values + invalid user names. Specific scenarios:
	//   - Register with the same name + different MetricType
	//   - Register with MetricType outside {Counter, Gauge, Histogram}
	//   - Register with an empty or invalid user name (mirrors parent
	//     stats.nameRE — empty / leading-dot / trailing-dot / disallowed-
	//     char)
	//   - Increment on a Histogram-typed metric (histograms accept Record
	//     only per AMEND-B2)
	//   - Increment on a Counter-typed metric with a negative delta (the
	//     underlying *stats.Counter is monotonic-non-negative; signed-int64
	//     delta per AMEND-B2 applies to Gauge)
	//   - Record on a Counter-typed metric (counters accept Increment
	//     only per AMEND-B2)
	ErrBadArgument = errors.New("dynamic: bad argument")

	// ErrNotFound is returned by Increment / Record / Get for an unknown
	// MetricID. The proxy-wasm guest may pass any uint32 as a metric_id
	// argument; an id that was never returned by a successful Register
	// surfaces as ErrNotFound.
	ErrNotFound = errors.New("dynamic: metric not found")
)

// -----------------------------------------------------------------------------
// registryEntry — internal per-MetricID record.
// -----------------------------------------------------------------------------

// registryEntry is the internal record for one registered metric. Exactly
// ONE of {counter, gauge, histogram} is non-nil per entry; the metricType
// field discriminates.
//
// The counter + gauge fields point at the *stats.Counter / *stats.Gauge
// registered against the parent *stats.Registry (full admin-side name
// = pluginScopePrefix + "." + namespacePrefix + "." + userName). The
// histogram field is the in-package atomic.Uint64 stub per ADR-0060
// deferral; it is NOT registered with the parent stats.Registry.
type registryEntry struct {
	// userName is the raw user-supplied custom name (without the
	// namespacePrefix or the pluginScopePrefix). The EnumerateForAdmin
	// callback receives namespacePrefix + "." + userName as the
	// wire-perspective name (`wasmcustom.<custom_name>`).
	userName string

	// metricType discriminates which of {counter, gauge, histogram} is
	// the live primitive.
	metricType MetricType

	// counter is non-nil iff metricType == MetricTypeCounter.
	counter *stats.Counter

	// gauge is non-nil iff metricType == MetricTypeGauge.
	gauge *stats.Gauge

	// histogram is non-nil iff metricType == MetricTypeHistogram. The
	// stub stores the most recently Record-ed value per the ADR-0060
	// deferred-primitive disposition. Pointer (not inline) so the
	// registryEntry struct itself can be value-copied out of the byID
	// map under RLock without copying an atomic.Uint64 (which would
	// race-trip the vet copylocks check).
	histogram *atomic.Uint64
}

// -----------------------------------------------------------------------------
// Registry — the per-plugin dynamic-stats Registry.
// -----------------------------------------------------------------------------

// Registry is the per-plugin dynamic-stats Registry per 25.2 SPEC §3.3 +
// ADR-0208 + AMEND-B2. Per-plugin means: each *compiledConfig constructs
// its OWN *Registry at config-load via NewRegistry — cross-plugin
// isolation happens via the per-plugin pluginScopePrefix (the underlying
// *stats.Registry is shared across plugins; the prefix discriminates the
// underlying stats name).
//
// Thread-safety: sync.RWMutex serializes byID + byName map mutations
// at Register (Write Lock); byID lookups at Increment / Record / Get /
// EnumerateForAdmin take a Read Lock. The underlying *stats.Counter /
// *stats.Gauge primitives use lock-free atomics so the hot Increment
// path takes ONLY a RLock + a single atomic op.
type Registry struct {
	mu sync.RWMutex

	// parentReg is the shared *stats.Registry that backs the underlying
	// Counter + Gauge primitives. Histograms are stored in the
	// in-package atomic.Uint64 stub per the ADR-0060 deferral and are
	// NOT registered with parentReg.
	parentReg *stats.Registry

	// pluginScopePrefix is the per-plugin stat-name prefix — typically
	// "wasm.<plugin_name>" (the parent *compiledConfig at Task 14 wires
	// this from PluginConfig.name per AMEND-A2 Group C). The underlying
	// stats name becomes pluginScopePrefix + "." + namespacePrefix +
	// "." + userName. The prefix may be empty in test fixtures; the
	// resulting leading-dot full name would be rejected by parent
	// stats.nameRE — Register surfaces this as ErrBadArgument.
	pluginScopePrefix string

	// maxEntries is the per-plugin envoy-go-strict cap on the
	// dynamic-stats namespace size per Q9 + AMEND-B2. Register of a
	// NEW name beyond this cap returns ErrCapExceeded; idempotent
	// re-Register of an EXISTING name bypasses the cap check.
	maxEntries uint32

	// byID maps allocated MetricID → registryEntry. Iterated by
	// EnumerateForAdmin under RLock. The entry value is copied out so
	// the EnumerateForAdmin callback can be invoked outside the
	// critical section (avoids re-entrancy hazards if the callback
	// closes over the Registry).
	byID map[MetricID]registryEntry

	// byName maps userName → MetricID for the idempotent-Register
	// fast path (re-Register of an existing (name, type) returns the
	// cached MetricID instead of allocating a new one).
	byName map[string]MetricID

	// nextID is the monotonically-increasing MetricID allocator.
	// Starts at 1 — zero is reserved as the "unassigned" sentinel so
	// callers can detect uninitialized MetricID variables.
	nextID MetricID
}

// -----------------------------------------------------------------------------
// Constructor.
// -----------------------------------------------------------------------------

// NewRegistry constructs a fresh per-plugin dynamic-stats Registry.
//
// Arguments:
//   - parentReg: the shared *stats.Registry that backs the underlying
//     Counter + Gauge primitives. Returns nil if parentReg is nil
//     (defensive nil-tolerance per ADR-0085).
//   - pluginScopePrefix: the per-plugin stat-name prefix (typically
//     "wasm.<plugin_name>"). The full underlying stats name becomes
//     pluginScopePrefix + "." + "wasmcustom" + "." + userName. The
//     prefix may be empty; consumers are responsible for validating
//     non-empty at parse time if their wire-shape requires it.
//   - maxEntries: the per-plugin envoy-go-strict cap on the dynamic-
//     stats namespace size per Q9 + AMEND-B2. Zero means no new
//     Register calls succeed (all return ErrCapExceeded).
//
// Per 25.2 SPEC §3.3 conceptual signature; this implementation
// substitutes (parentReg, pluginScopePrefix) for the SPEC's
// `pluginScope *stats.Scope` argument because the parent stats package
// does NOT yet expose a Scope type — the flat-Registry-with-
// hierarchical-dotted-names model suffices for the per-plugin scoping
// discipline (see doc.go for the rationale).
func NewRegistry(parentReg *stats.Registry, pluginScopePrefix string, maxEntries uint32) *Registry {
	if parentReg == nil {
		// Defensive nil-tolerance per ADR-0085 — consumers nil-check
		// before invoking Register / Increment / etc.
		return nil
	}
	return &Registry{
		parentReg:         parentReg,
		pluginScopePrefix: pluginScopePrefix,
		maxEntries:        maxEntries,
		byID:              make(map[MetricID]registryEntry),
		byName:            make(map[string]MetricID),
		nextID:            1, // zero is the unassigned sentinel.
	}
}

// -----------------------------------------------------------------------------
// Register — idempotent metric definition.
// -----------------------------------------------------------------------------

// Register allocates a MetricID + registers the metric under
// pluginScopePrefix + "." + "wasmcustom" + "." + name (the underlying
// admin-side stats name) per AMEND-B2 namespace pin.
//
// Idempotency:
//   - Same (name, metricType) → returns the cached MetricID + nil (no
//     duplicate underlying stats primitive registration).
//   - Same name with DIFFERENT metricType → returns 0 + ErrBadArgument
//     (the existing entry is preserved; cpp-host upstream behavior on
//     same-name-different-type re-Register is undefined — envoy-go
//     fails fast rather than overwrite).
//
// Cap discipline:
//   - New (name, metricType) at the cap boundary returns 0 +
//     ErrCapExceeded. Idempotent re-Register of an EXISTING name
//     bypasses the cap check (no new entry allocated).
//
// Validation:
//   - metricType outside {Counter, Gauge, Histogram} returns 0 +
//     ErrBadArgument.
//   - name fails userNameRE (empty / leading-dot / trailing-dot /
//     disallowed-char) returns 0 + ErrBadArgument.
//   - The full underlying stats name (pluginScopePrefix + "." +
//     "wasmcustom" + "." + name) fails parent stats.IsValidName (e.g.
//     leading-dot from empty pluginScopePrefix) returns 0 +
//     ErrBadArgument.
//
// Concurrency:
//   - The fast-path idempotent re-Register checks byName under RLock
//     first. If absent, the slow path takes the Write Lock + re-checks
//     byName (race-safe under contention) + performs the cap check +
//     allocates a new MetricID + registers the underlying primitive
//     via the parent stats.Registry's IfAbsent methods (per ADR-0117
//     post-Freeze idempotent registration).
func (r *Registry) Register(metricType MetricType, name string) (MetricID, error) {
	// Discriminator validation per AMEND-B2 byte-pin.
	switch metricType {
	case MetricTypeCounter, MetricTypeGauge, MetricTypeHistogram:
		// OK.
	default:
		return 0, fmt.Errorf("%w: invalid MetricType %d (must be 0/1/2 per AMEND-B2)", ErrBadArgument, int(metricType))
	}

	// User-name validation. Empty + leading-dot + trailing-dot +
	// disallowed-char rejected. The parent stats.nameRE also rejects
	// these but the consumer-side error wrapping uses ErrBadArgument
	// (parent stats.NewCounter panics on invalid name — we MUST
	// pre-validate to avoid the panic).
	if !userNameRE.MatchString(name) {
		return 0, fmt.Errorf("%w: invalid metric name %q (must match %s)", ErrBadArgument, name, userNameRE.String())
	}

	// Fast-path: idempotent re-Register check under RLock.
	r.mu.RLock()
	if existingID, ok := r.byName[name]; ok {
		existingEntry := r.byID[existingID]
		r.mu.RUnlock()
		if existingEntry.metricType != metricType {
			return 0, fmt.Errorf(
				"%w: metric %q already registered with MetricType %d; cannot re-register with MetricType %d",
				ErrBadArgument, name, int(existingEntry.metricType), int(metricType),
			)
		}
		return existingID, nil
	}
	r.mu.RUnlock()

	// Slow path: allocate under Write Lock. Re-check byName under the
	// Write Lock in case a concurrent Register inserted between the
	// RUnlock and the Lock (race-safe idempotency).
	r.mu.Lock()
	defer r.mu.Unlock()
	if existingID, ok := r.byName[name]; ok {
		existingEntry := r.byID[existingID]
		if existingEntry.metricType != metricType {
			return 0, fmt.Errorf(
				"%w: metric %q already registered with MetricType %d; cannot re-register with MetricType %d",
				ErrBadArgument, name, int(existingEntry.metricType), int(metricType),
			)
		}
		return existingID, nil
	}

	// Cap check. The check is at the Write-Lock-protected critical
	// section so concurrent Registers cannot both observe `len < cap`
	// and both allocate (avoiding the spurious-overflow-slot bug
	// guarded against by TestRegistry_Concurrent_CapBoundary_
	// NoSpuriousSlot).
	if uint32(len(r.byID)) >= r.maxEntries {
		return 0, fmt.Errorf("%w: %d entries at cap %d", ErrCapExceeded, len(r.byID), r.maxEntries)
	}

	// Compose the full underlying stats name + validate it via the
	// parent stats package's exported IsValidName helper. The empty-
	// pluginScopePrefix case produces a leading-dot full name which
	// parent stats.nameRE rejects; surface as ErrBadArgument so the
	// caller doesn't trip the parent's panic-on-invalid-name path.
	fullName := r.composeFullName(name)
	if !stats.IsValidName(fullName) {
		return 0, fmt.Errorf("%w: composed full stats name %q is invalid (check pluginScopePrefix)", ErrBadArgument, fullName)
	}

	// Allocate MetricID + register the underlying primitive.
	id := r.nextID
	r.nextID++

	entry := registryEntry{
		userName:   name,
		metricType: metricType,
	}
	switch metricType {
	case MetricTypeCounter:
		// IfAbsent variant per ADR-0117 — permits post-Freeze
		// registration (envoy-go's *stats.Registry is Freeze-d at
		// boot-end; dynamic Register fires at config-load + at runtime
		// when the proxy-wasm guest calls proxy_define_metric).
		entry.counter = r.parentReg.NewCounterIfAbsent(fullName)
	case MetricTypeGauge:
		entry.gauge = r.parentReg.NewGaugeIfAbsent(fullName)
	case MetricTypeHistogram:
		// In-package atomic.Uint64 stub per the ADR-0060 deferred-
		// primitive disposition. NOT registered with the parent
		// *stats.Registry — surfaces only via EnumerateForAdmin.
		entry.histogram = new(atomic.Uint64)
	}

	r.byID[id] = entry
	r.byName[name] = id
	return id, nil
}

// composeFullName returns the underlying admin-side stats name:
// pluginScopePrefix + "." + namespacePrefix + "." + userName.
// If pluginScopePrefix is empty the leading dot trips parent stats.nameRE
// rejection; the Register call surfaces this as ErrBadArgument.
func (r *Registry) composeFullName(userName string) string {
	return r.pluginScopePrefix + "." + namespacePrefix + "." + userName
}

// -----------------------------------------------------------------------------
// Increment — SIGNED int64 delta per AMEND-B2.
// -----------------------------------------------------------------------------

// Increment adds SIGNED delta to the metric identified by id, per
// AMEND-B2 (`proxy_increment_metric(metric_id, delta)` with delta =
// signed int64 to permit negative gauge deltas).
//
// Semantics by metricType:
//   - Counter: delta MUST be non-negative (the underlying *stats.Counter
//     is monotonic-non-negative). Negative delta returns ErrBadArgument.
//     The Counter.Add(uint64(delta)) op is lock-free atomic.
//   - Gauge: delta may be negative (signed-int64 per AMEND-B2). The
//     Gauge.Add(delta) op is lock-free atomic on int64.
//   - Histogram: ErrBadArgument (histograms accept Record only per
//     AMEND-B2).
//
// Returns ErrNotFound on unknown id.
func (r *Registry) Increment(id MetricID, delta int64) error {
	r.mu.RLock()
	entry, ok := r.byID[id]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: id=%d", ErrNotFound, uint32(id))
	}
	switch entry.metricType {
	case MetricTypeCounter:
		if delta < 0 {
			return fmt.Errorf("%w: negative delta %d on Counter (Counters are monotonic-non-negative)", ErrBadArgument, delta)
		}
		entry.counter.Add(uint64(delta))
		return nil
	case MetricTypeGauge:
		entry.gauge.Add(delta)
		return nil
	case MetricTypeHistogram:
		return fmt.Errorf("%w: Increment not supported on Histogram (use Record per AMEND-B2)", ErrBadArgument)
	default:
		// Should not be reachable — Register validates metricType.
		return fmt.Errorf("%w: unknown MetricType %d", ErrBadArgument, int(entry.metricType))
	}
}

// -----------------------------------------------------------------------------
// Record — UNSIGNED uint64 value per AMEND-B2.
// -----------------------------------------------------------------------------

// Record sets UNSIGNED value on the metric identified by id, per AMEND-B2
// (`proxy_record_metric(metric_id, value)` with value = unsigned uint64).
//
// Semantics by metricType:
//   - Counter: ErrBadArgument (counters accept Increment only per
//     AMEND-B2).
//   - Gauge: Gauge.Set(int64(value)) — overwrite semantic. Lock-free
//     atomic on int64. (Note: the int64 reinterpretation of a uint64
//     value > math.MaxInt64 surfaces as a negative gauge value; the
//     caller is responsible for the unsigned interpretation per
//     AMEND-B2.)
//   - Histogram: stores value in the in-package atomic.Uint64 stub
//     (latest-value semantics per the ADR-0060 deferred-primitive
//     disposition).
//
// Returns ErrNotFound on unknown id.
func (r *Registry) Record(id MetricID, value uint64) error {
	r.mu.RLock()
	entry, ok := r.byID[id]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: id=%d", ErrNotFound, uint32(id))
	}
	switch entry.metricType {
	case MetricTypeCounter:
		return fmt.Errorf("%w: Record not supported on Counter (use Increment per AMEND-B2)", ErrBadArgument)
	case MetricTypeGauge:
		// Reinterpret as int64 — values > math.MaxInt64 surface as
		// negative gauge values per AMEND-B2 caller-responsibility.
		entry.gauge.Set(int64(value)) //nolint:gosec // wire-shape per AMEND-B2 — reinterpret allowed.
		return nil
	case MetricTypeHistogram:
		entry.histogram.Store(value)
		return nil
	default:
		// Should not be reachable.
		return fmt.Errorf("%w: unknown MetricType %d", ErrBadArgument, int(entry.metricType))
	}
}

// -----------------------------------------------------------------------------
// Get — current value as uint64.
// -----------------------------------------------------------------------------

// Get returns the current value of the metric identified by id as a
// uint64. Counter values are naturally unsigned; Gauge values are
// reinterpreted from int64 to uint64 (caller is responsible for the
// signed interpretation if the gauge can carry negative values per
// AMEND-B2). Histogram values are the latest Record-ed value per the
// ADR-0060 deferred-primitive disposition.
//
// Returns ErrNotFound on unknown id.
func (r *Registry) Get(id MetricID) (uint64, error) {
	r.mu.RLock()
	entry, ok := r.byID[id]
	r.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("%w: id=%d", ErrNotFound, uint32(id))
	}
	switch entry.metricType {
	case MetricTypeCounter:
		return entry.counter.Load(), nil
	case MetricTypeGauge:
		return uint64(entry.gauge.Load()), nil //nolint:gosec // wire-shape per AMEND-B2 — reinterpret allowed.
	case MetricTypeHistogram:
		return entry.histogram.Load(), nil
	default:
		// Should not be reachable.
		return 0, fmt.Errorf("%w: unknown MetricType %d", ErrBadArgument, int(entry.metricType))
	}
}

// -----------------------------------------------------------------------------
// EnumerateForAdmin — admin /stats lazy enumeration.
// -----------------------------------------------------------------------------

// EnumerateForAdmin walks the registry + invokes fn for each registered
// metric. The callback receives the WIRE-PERSPECTIVE name
// (`wasmcustom.<custom_name>`) + the current value as uint64.
//
// The wire-perspective name (vs the admin-side full name
// `wasm.<plugin>.wasmcustom.<custom_name>`) is byte-faithful to the
// proxy-wasm guest's view per AMEND-B2 REFINEMENT — the admin /stats
// endpoint's Prometheus scrape already surfaces the full name via the
// parent stats.Registry's Walk; this enumerator exists for the
// proxy-wasm-guest-visible introspection use case (consumed at a future
// Layer-A phase per the §3.3 + ADR-0208 anchor).
//
// The callback is invoked once per registered metric. Iteration order
// is map-iteration-order (non-deterministic); callers requiring
// deterministic ordering MUST sort post-walk.
//
// Concurrency: takes a Read Lock for the entry-set snapshot, then
// invokes the callback OUTSIDE the critical section (avoids re-entrancy
// hazards if the callback closes over the Registry). The underlying
// primitive value reads happen inside the callback via the captured
// entry pointers (which are stable for the lifetime of the entry —
// registryEntry is value-stored in byID + the primitive pointers it
// holds are stable).
func (r *Registry) EnumerateForAdmin(fn func(name string, value uint64)) {
	r.mu.RLock()
	// Copy the entry slice out so we can invoke the callback outside
	// the RLock (the callback could otherwise observe stale state if
	// it triggered a concurrent Register that completed during the
	// callback dispatch — but the entry pointers are stable so the
	// callback's value read sees the live primitive).
	entries := make([]registryEntry, 0, len(r.byID))
	for _, e := range r.byID {
		entries = append(entries, e)
	}
	r.mu.RUnlock()

	for _, e := range entries {
		var value uint64
		switch e.metricType {
		case MetricTypeCounter:
			value = e.counter.Load()
		case MetricTypeGauge:
			value = uint64(e.gauge.Load()) //nolint:gosec // wire-shape per AMEND-B2.
		case MetricTypeHistogram:
			value = e.histogram.Load()
		}
		wireName := namespacePrefix + "." + e.userName
		fn(wireName, value)
	}
}
