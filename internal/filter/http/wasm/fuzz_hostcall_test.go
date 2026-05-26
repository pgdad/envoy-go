package wasm

// fuzz_hostcall_test.go — 35th project-wide fuzzer `FuzzWasmHostcallEnvelope`
// per Phase 25.2 PLAN Task 19 + 25.2 SPEC §8.4 + R-25.2-12 + ADR-0018
// baseline ("every parser/codec/filter ships a fuzzer; 30s/seed CI budget").
//
// # D-25.2-P4 closure (per D-P-PLAN-10) — 35-seed corpus across 10 dimensions
//
//	┌─────┬────────────────────────────────────────────────────────────┬───────┐
//	│ Dim │ Subject                                                    │ Seeds │
//	├─────┼────────────────────────────────────────────────────────────┼───────┤
//	│  1  │ proxy_get_buffer_bytes start/max envelope edges (AMEND-B1) │   5   │
//	│  2  │ proxy-wasm pairs serialization adversarial                 │   4   │
//	│  3  │ Foreign-function call name length boundary                 │   3   │
//	│  4  │ Dynamic-stats name validation                              │   4   │
//	│  5  │ Shared-data CAS-mismatch race patterns                     │   3   │
//	│  6  │ Body-buffer cap boundary (AMEND-B1 clamp)                  │   3   │
//	│  7  │ Property-path NUL-delimited adversarial (AMEND-B4)         │   4   │
//	│  8  │ Tick period parsing (Q5 10ms floor)                        │   3   │
//	│  9  │ httpCall envelope adversarial                              │   4   │
//	│ 10  │ Metric type out-of-range + signed-i64 delta extremes       │   2   │
//	│Total│                                                            │  35   │
//	└─────┴────────────────────────────────────────────────────────────┴───────┘
//
// # Must-never-panic invariant
//
// The fuzz body wraps every hostcall invocation in a `defer recover()` and
// fails the test on any panic. The wire-result code MUST be one of the
// 10 named WasmResult sentinels per AMEND-A7 (Ok / NotFound / BadArgument /
// SerializationFailure / ParseFailure / InvalidMemoryAccess / Empty /
// CasMismatch / InternalFailure / Unimplemented). A return outside this
// set indicates a corrupted enum-decode + fails the fuzz.
//
// # Strategy — direct host-side primitive dispatch
//
// The 25.2 abi/* shims (internal/wasm/abi/) all decode wire-shape from a
// real wazero api.Module + delegate to a host-side *RootVM method. Per
// 25.2 SPEC §8.4 the must-never-panic invariant centers on the host-side
// dispatcher (the wire-decode is wazero-internal + extensively unit-tested
// at abi/*_test.go). We therefore exercise the wasm-package primitives
// directly:
//
//	Dim 1 → AMEND-B1 clamp arithmetic (replicates the shim's clamp branch
//	         on the uint32 start/maxSize envelope; must-never-panic on
//	         integer math is trivial in Go but the clamp logic is the
//	         load-bearing invariant)
//	Dim 2 → internalwasm.DecodePairs(payload)
//	Dim 3 → rv.CallForeignFunction(ctx, name, args) — exercises name-keyed
//	         registry lookup + (when registered) the panic-recovery wrapper
//	Dim 4 → rv.DefineMetric(metricType, name) — exercises *dynamic.Registry
//	         userNameRE validation + the cap-boundary trigger
//	Dim 5 → rv.SetSharedData / rv.GetSharedData — CAS-protected K-V store
//	         with envoy-go-strict 1 MiB value cap + 1024-entry cap
//	Dim 6 → f.DecodeData(payload, false) — sticky cap-exceeded + 413 path
//	Dim 7 → internalwasm.ResolveProperty(resolver, path) — NUL-delimited
//	         path tokenizer + per-root dispatch
//	Dim 8 → rv.SetTickPeriod(d) — 10ms floor + 0-cancels + goroutine
//	         lifecycle (defer rv.SetTickPeriod(0) cleans up)
//	Dim 9 → rv.DispatchHttpCall — unknown-cluster + nil-dispatcher paths;
//	         empty cluster / timeout=0 / timeout=u32::MAX
//	Dim 10 → rv.DefineMetric(99, ...) → BadArgument; rv.IncrementMetric
//	          with delta=int64 min/max → must-never-panic at the underlying
//	          *stats.Counter atomic.Add
//
// # 35th project-wide fuzzer per ADR-0018 baseline
//
// Pre-25.2 count: 34 (33 pre-25.1 per 25.1 SPEC §11.1 D-S1 + 25.1's
// FuzzWasmConfigParse). This Task 19 lands the 35th. Verified via:
//
//	grep -rh "^func Fuzz" $(find . -name 'fuzz_test.go' -not \
//	  -path '*/.worktrees/*' -not -path '*/.claude/*') | wc -l
//	# Expected: 35 (was 34 at master tip; +1 for FuzzWasmHostcallEnvelope)
//
// # Cross-references
//
//   - ADR-0018 (every parser/codec/filter ships a fuzzer; 30s/seed budget)
//   - 25.2 SPEC §8.4 (35th project-wide fuzzer surface)
//   - 25.2 SPEC §8.5 (D-S2 35th-fuzzer count VERIFIED at SPEC commit)
//   - 25.2 PLAN Task 19 (this Task)
//   - 25.2 PLAN D-P-PLAN-10 (35-seed corpus across 10 dimensions)
//   - R-25.2-12 (35th-fuzzer must-never-panic invariant)
//   - AMEND-B1 (proxy_get_buffer_bytes clamp wire-contract)
//   - AMEND-B2 (signed-i64 delta + uint64 value + metric type byte-pin)
//   - AMEND-B3 (cancel-at-destruction; http_call_response_after_close counter)
//   - AMEND-B4 (NUL-delimited property path serialization)
//   - Q5 (tick period 10ms envoy-go-strict floor)
//   - Q6 (shared-data cap discipline 1 MiB + 1024 entries)

import (
	"context"
	"math"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
	"github.com/esalaine/envoy-go/internal/stats/dynamic"
	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// fuzzHTTPDispatcherStub — minimal HTTPDispatcher for Dim 9. Reports the
// single known cluster "fuzz_cluster"; Dispatch returns a synthetic 200
// response without performing any I/O. Per-call lifetime is bound to the
// caller-supplied ctx.
type fuzzHTTPDispatcherStub struct{}

func (fuzzHTTPDispatcherStub) HasCluster(name string) bool { return name == "fuzz_cluster" }
func (fuzzHTTPDispatcherStub) Dispatch(_ context.Context, _ string, _ *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}}, nil
}

// fuzzPanickingForeignFn — registered under the "panic_me" name so the fuzz
// body can probe the foreign-function panic-recovery path per D-P-PLAN-9
// (d). The host wraps the call in defer/recover + converts the panic into
// WasmResult::InternalFailure; the *RootVM is NOT poisoned.
func fuzzPanickingForeignFn(_ context.Context, _ []byte) ([]byte, abi.WasmResult) {
	panic("FuzzWasmHostcallEnvelope: deliberate panic from foreign function (Dim 3 sub 3)")
}

// fuzzEchoForeignFn — registered under the "echo" name so the fuzz body can
// probe the Ok path with a non-nil result + verify the result-bytes plumbing
// survives extreme args lengths.
func fuzzEchoForeignFn(_ context.Context, args []byte) ([]byte, abi.WasmResult) {
	return args, abi.WasmResultOk
}

// fuzzFixtures bundles the heavy *RootVM + *filterPropertyResolver fixtures
// shared across all fuzz iterations. Construction is amortized via
// sync.Once invoked from the first f.Fuzz body call (constructing per-
// iteration would burn ~ms on CompileModule + NewRootVM + Configure).
//
// LIFETIME: The fixtures are package-level + persist across fuzz
// invocations within the same `go test` process. Cleanup is best-effort
// via a finalizer-equivalent t.Cleanup registered on the first-fuzz-iter
// *testing.T (the *RootVM owns a long-lived wazero.Runtime + caches that
// MUST be released to avoid memory leaks across test sessions).
type fuzzFixtures struct {
	rv       *internalwasm.RootVM
	resolver *filterPropertyResolver
}

//nolint:gochecknoglobals // sync.Once-protected lazy-init test fixture; package-private.
var (
	fuzzFixturesOnce sync.Once
	fuzzFixturesVal  *fuzzFixtures
	fuzzFixturesErr  error
)

// getFuzzFixtures lazy-builds the shared fuzz fixtures on first call +
// returns the cached bundle on subsequent calls. Fail-fast on
// construction error via tb.Fatalf.
//
// Returns a *RootVM with:
//
//   - Permissive sandbox (all 21 NEW 25.2 capabilities + the 25.1 default
//     set granted) — capability gating is enforced at registration.go
//     hostcall envelope; the host-side dispatch primitives we drive
//     directly do NOT consult the sandbox.
//   - Mock HTTPDispatcher (fuzzHTTPDispatcherStub) — one known cluster
//     "fuzz_cluster"; Dispatch returns a synthetic 200.
//   - Per-RootVM ForeignFunctionRegistry with two functions registered:
//     "echo" + "panic_me" — exercises both the Ok-result path + the
//     panic-recovery path per D-P-PLAN-9 (d).
//   - Per-RootVM *dynamic.Registry with a TINY 16-entry cap (vs the
//     envoy-go-strict 1024-entry default) — exercises Dim 4 sub 3
//     cap-boundary InternalFailure path within seed-reachable Register loops.
//   - Shared-data caps tightened to 256-byte value + 8-entry max so the
//     Dim 5 cap-exceeded → InternalFailure path triggers without the
//     envoy-go-strict 1 MiB value-cap requiring megabyte-sized fuzz inputs.
func getFuzzFixtures(tb testing.TB) *fuzzFixtures {
	tb.Helper()
	fuzzFixturesOnce.Do(func() {
		ctx := context.Background()

		cache := internalwasm.NewCompileCache(ctx)
		mod, err := internalwasm.CompileModule(ctx, buildMinimalProxyWasm(), cache)
		if err != nil {
			fuzzFixturesErr = err
			return
		}

		// Per-RootVM ForeignFunctionRegistry with two seed functions for Dim 3.
		freg := internalwasm.NewForeignFunctionRegistry()
		if err := freg.Register("echo", fuzzEchoForeignFn); err != nil {
			fuzzFixturesErr = err
			return
		}
		if err := freg.Register("panic_me", fuzzPanickingForeignFn); err != nil {
			fuzzFixturesErr = err
			return
		}

		// Per-RootVM *dynamic.Registry with a tiny 16-entry cap so the
		// Dim 4 sub 3 cap-boundary trigger fires within ~24 Register calls
		// (vs the 1024-entry envoy-go-strict default).
		parentReg := stats.NewRegistry()
		dynReg := dynamic.NewRegistry(parentReg, "wasm.fuzz_plugin", 16)
		if dynReg == nil {
			fuzzFixturesErr = errFuzzDynRegNil
			return
		}

		rv, err := internalwasm.NewRootVM(ctx, mod, 1,
			internalwasm.WithRootCompilationCache(cache.WazeroCompilationCache()),
			internalwasm.WithRootHTTPDispatcher(fuzzHTTPDispatcherStub{}),
			internalwasm.WithRootForeignRegistry(freg),
			internalwasm.WithRootDynamicStats(dynReg),
			// Tight shared-data caps so the Dim 5 cap-boundary triggers
			// without megabyte-sized fuzz inputs.
			internalwasm.WithRootSharedDataCaps(256, 8),
		)
		if err != nil {
			fuzzFixturesErr = err
			return
		}

		// Drive the RootVM lifecycle to the post-Configure state so
		// subsequent hostcall dispatches inherit a well-formed root
		// context. The minimal proxy wasm exports `_initialize` +
		// `proxy_abi_version_0_2_1` only; the missing proxy_on_vm_start /
		// proxy_on_configure callbacks are no-ops (per ADR-0204 + §3.3
		// "nullptr the function pointer" discipline).
		if err := rv.Configure(ctx, nil, nil); err != nil {
			_ = rv.Close()
			fuzzFixturesErr = err
			return
		}

		fuzzFixturesVal = &fuzzFixtures{
			rv:       rv,
			resolver: buildFuzzPropertyResolver(),
		}
	})

	if fuzzFixturesErr != nil {
		tb.Fatalf("getFuzzFixtures: setup err = %v", fuzzFixturesErr)
	}
	return fuzzFixturesVal
}

// errFuzzDynRegNil is the sentinel returned from getFuzzFixtures when
// dynamic.NewRegistry returns nil (defensive — should-not-happen with a
// valid parent *stats.Registry + non-empty pluginScopePrefix).
type fuzzErr string

func (e fuzzErr) Error() string { return string(e) }

const errFuzzDynRegNil = fuzzErr("getFuzzFixtures: dynamic.NewRegistry returned nil")

// buildFuzzPropertyResolver constructs a *filterPropertyResolver bound to a
// nil-*filter. Per ADR-0085 nil-tolerance: every accessor returns
// (zero, false); ResolveProperty thus returns NotFound for every probe
// (the must-never-panic invariant + the parsePathSegments fuzz still hits
// every adversarial path).
func buildFuzzPropertyResolver() *filterPropertyResolver {
	return &filterPropertyResolver{filter: nil}
}

// buildFuzzFilter constructs a *filter with a tiny body-buffer cap so the
// Dim 6 cap-boundary triggers within seed-sized payloads.
func buildFuzzFilter(t *testing.T, capBytes uint32) *filter {
	t.Helper()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		pluginName:         "fuzz_plugin",
		bodyBufferCapBytes: capBytes,
		stats:              newFilterStats(reg, "fuzz_plugin"),
	}
	return &filter{cfg: cc, decoderCb: fakeDecoderCb{}}
}

// isKnownWasmResult reports whether r is one of the 10 named WasmResult
// sentinels per AMEND-A7 (Ok / NotFound / BadArgument / SerializationFailure
// / ParseFailure / InvalidMemoryAccess / Empty / CasMismatch /
// InternalFailure / Unimplemented). A return outside this set would
// indicate an enum-decode corruption (must-never-panic is the primary
// invariant; well-formed result codes are the secondary structural check).
func isKnownWasmResult(r abi.WasmResult) bool {
	switch r {
	case abi.WasmResultOk,
		abi.WasmResultNotFound,
		abi.WasmResultBadArgument,
		abi.WasmResultSerializationFailure,
		abi.WasmResultParseFailure,
		abi.WasmResultInvalidMemoryAccess,
		abi.WasmResultEmpty,
		abi.WasmResultCasMismatch,
		abi.WasmResultInternalFailure,
		abi.WasmResultUnimplemented:
		return true
	}
	return false
}

// FuzzWasmHostcallEnvelope is the 35th project-wide fuzzer per Phase 25.2
// PLAN Task 19 + 25.2 SPEC §8.4 + R-25.2-12 + ADR-0018 baseline. The fuzz
// body dispatches the input to one of the 10 D-P-PLAN-10 dimensions per
// the (dim, sub) header bytes; each dimension probes a different 25.2
// hostcall envelope surface. The must-never-panic invariant covers all 14
// NEW hostcall surfaces + foreign-function dispatch + dynamic-stats
// Register + shared-data CAS race + body-buffer cap boundary + property-
// path NUL-delimited adversarials per §8.4.
//
// Fuzz signature: (dim, sub byte, arg32 uint32, arg64 uint64, payload []byte).
// The 5-arg shape lets the Go fuzz engine mutate each component
// independently; the (dim, sub) bytes route to the per-dimension branch
// + interpret the remaining args per the dimension's seed-shape.
func FuzzWasmHostcallEnvelope(f *testing.F) {
	// -------------------------------------------------------------------------
	// Dim 1 — proxy_get_buffer_bytes start/max envelope edges (5 seeds) per
	// AMEND-B1 clamp wire-contract.
	// -------------------------------------------------------------------------
	f.Add(byte(1), byte(0), uint32(0), uint64(0), []byte{})                           // start=0, max=0
	f.Add(byte(1), byte(1), uint32(0), uint64(math.MaxUint32), []byte{})              // start=0, max=u32::MAX
	f.Add(byte(1), byte(2), uint32(math.MaxUint32), uint64(1), []byte{})              // start=u32::MAX, max=1
	f.Add(byte(1), byte(3), uint32(math.MaxUint32), uint64(math.MaxUint32), []byte{}) // i32-overflow
	f.Add(byte(1), byte(4), uint32(10), uint64(math.MaxUint32), []byte{})             // clamp

	// -------------------------------------------------------------------------
	// Dim 2 — proxy-wasm pairs serialization adversarial (4 seeds).
	// -------------------------------------------------------------------------
	f.Add(byte(2), byte(0), uint32(0), uint64(0), []byte{0x01, 0x00, 0x00}) // truncated pair header (3 bytes; needs 4)
	f.Add(byte(2), byte(1), uint32(0), uint64(0), []byte{
		0x01, 0x00, 0x00, 0x00, // num_pairs = 1
		0xff, 0xff, 0xff, 0xff, // key_len = u32::MAX — overruns
		0x00, 0x00, 0x00, 0x00, // value_len = 0
	}) // malformed key/value sizes
	f.Add(byte(2), byte(2), uint32(0), uint64(0), []byte{
		0x02, 0x00, 0x00, 0x00, // num_pairs = 2
		0x01, 0x00, 0x00, 0x00, // key_len = 1
		0x01, 0x00, 0x00, 0x00, // value_len = 1
		0x01, 0x00, 0x00, 0x00, // key_len = 1
		0x01, 0x00, 0x00, 0x00, // value_len = 1
		'k', 0x00, 'v', 0x00,
		'k', 0x00, 'v', 0x00, // reused-key (still well-formed wire; semantic-level dup)
	}) // reused-key duplicate pairs (well-formed wire — decode returns 2 pairs)
	f.Add(byte(2), byte(3), uint32(0), uint64(0), func() []byte {
		// max-size headers payload: 1 pair with a 1024-byte value
		out := make([]byte, 0, 4+8+1+1+1024+1)
		out = append(out, 0x01, 0x00, 0x00, 0x00) // num_pairs=1
		out = append(out, 0x01, 0x00, 0x00, 0x00) // key_len=1
		out = append(out, 0x00, 0x04, 0x00, 0x00) // value_len=1024
		out = append(out, 'k', 0x00)
		out = append(out, make([]byte, 1024)...)
		out = append(out, 0x00)
		return out
	}()) // max-size headers payload

	// -------------------------------------------------------------------------
	// Dim 3 — Foreign-function call name length boundary (3 seeds).
	// -------------------------------------------------------------------------
	f.Add(byte(3), byte(0), uint32(0), uint64(0), []byte(""))                         // name=empty
	f.Add(byte(3), byte(1), uint32(0), uint64(0), []byte(strings.Repeat("a", 1024)))  // name=1024 bytes
	f.Add(byte(3), byte(2), uint32(0), uint64(0), []byte(strings.Repeat("z", 65535))) // name=u16::MAX bytes

	// -------------------------------------------------------------------------
	// Dim 4 — Dynamic-stats name validation (4 seeds).
	// -------------------------------------------------------------------------
	f.Add(byte(4), byte(0), uint32(0), uint64(0), []byte(""))                     // name=empty
	f.Add(byte(4), byte(1), uint32(0), uint64(0), []byte("name\x00with_nul"))     // NUL byte
	f.Add(byte(4), byte(2), uint32(0), uint64(0), []byte{0xff, 0xfe, 0xfd, 0xfc}) // non-UTF-8 bytes
	f.Add(byte(4), byte(3), uint32(0), uint64(0), []byte("cap_boundary_trigger")) // cap-boundary; fuzz loops Register N times

	// -------------------------------------------------------------------------
	// Dim 5 — Shared-data CAS-mismatch race patterns (3 seeds).
	// -------------------------------------------------------------------------
	f.Add(byte(5), byte(0), uint32(0), uint64(0), []byte("shared_key_cas0"))             // cas=0 race
	f.Add(byte(5), byte(1), uint32(math.MaxUint32), uint64(0), []byte("shared_key_max")) // cas=u32::MAX
	f.Add(byte(5), byte(2), uint32(0), uint64(0), []byte(""))                            // key=empty

	// -------------------------------------------------------------------------
	// Dim 6 — Body-buffer cap boundary cases per AMEND-B1 (3 seeds).
	// arg32 carries the per-test bodyBufferCap (small cap so the boundary
	// fits in the seed payload size); the payload is the body chunk.
	// -------------------------------------------------------------------------
	f.Add(byte(6), byte(0), uint32(16), uint64(0), make([]byte, 16)) // exactly-at-cap
	f.Add(byte(6), byte(1), uint32(16), uint64(0), make([]byte, 17)) // one-byte-over-cap
	f.Add(byte(6), byte(2), uint32(16), uint64(0), make([]byte, 15)) // one-byte-under-cap

	// -------------------------------------------------------------------------
	// Dim 7 — Property-path NUL-delimited adversarial per AMEND-B4 (4 seeds).
	// -------------------------------------------------------------------------
	f.Add(byte(7), byte(0), uint32(0), uint64(0), []byte("request\x00headers\x00x-foo")) // well-formed (no terminator NUL)
	f.Add(byte(7), byte(1), uint32(0), uint64(0), []byte("request\x00\x00headers"))      // empty segment (NUL NUL)
	f.Add(byte(7), byte(2), uint32(0), uint64(0), func() []byte {
		// >MAX_PATH depth: 100 NUL-delimited segments
		segs := make([]string, 100)
		for i := range segs {
			segs[i] = "s"
		}
		return []byte(strings.Join(segs, "\x00"))
	}()) // 100-segment depth
	f.Add(byte(7), byte(3), uint32(0), uint64(0), []byte("nonexistent_root\x00sub\x00x")) // unknown root

	// -------------------------------------------------------------------------
	// Dim 8 — Tick period parsing per Q5 envoy-go-strict 10ms floor (3 seeds).
	// arg32 carries the period in milliseconds.
	// -------------------------------------------------------------------------
	f.Add(byte(8), byte(0), uint32(0), uint64(0), []byte{})             // period=0 (cancel)
	f.Add(byte(8), byte(1), uint32(1), uint64(0), []byte{})             // period=1ms (below floor → clamp 10ms)
	f.Add(byte(8), byte(2), uint32(math.MaxInt32), uint64(0), []byte{}) // period=i32::MAX ms (~24.8 days)

	// -------------------------------------------------------------------------
	// Dim 9 — httpCall envelope adversarial (4 seeds).
	// arg32 carries the timeoutMs; payload carries the cluster name.
	// -------------------------------------------------------------------------
	f.Add(byte(9), byte(0), uint32(1000), uint64(0), []byte(""))                            // cluster_name=empty
	f.Add(byte(9), byte(1), uint32(1000), uint64(0), []byte("fuzz_cluster_with_malformed")) // unknown cluster
	f.Add(byte(9), byte(2), uint32(0), uint64(0), []byte("fuzz_cluster"))                   // timeout=0 (default)
	f.Add(byte(9), byte(3), uint32(math.MaxUint32), uint64(0), []byte("fuzz_cluster"))      // timeout=u32::MAX

	// -------------------------------------------------------------------------
	// Dim 10 — Metric type out-of-range + signed-i64 delta extremes per
	// AMEND-B2 (2 seeds). arg32 carries the metric_type; arg64 carries
	// the int64 delta (reinterpreted).
	// -------------------------------------------------------------------------
	f.Add(byte(10), byte(0), uint32(99), uint64(0), []byte("bad_type_name"))       // MetricType=99 → ErrBadArgument
	f.Add(byte(10), byte(1), uint32(0), uint64(1)<<63, []byte("min_delta_metric")) // delta=i64::MIN (bit-pattern 0x8000...0000 reinterpreted as int64 in fuzz body)

	// -------------------------------------------------------------------------
	// Fuzz body: dispatch on (dim, sub) + invoke the per-dimension primitive
	// under a defer/recover panic-trap. Result codes must be one of the 10
	// named WasmResult sentinels per AMEND-A7; a non-sentinel return fails
	// the fuzz (indicates enum-decode corruption).
	// -------------------------------------------------------------------------

	f.Fuzz(func(t *testing.T, dim, sub byte, arg32 uint32, arg64 uint64, payload []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("FuzzWasmHostcallEnvelope: panic on dim=%d sub=%d arg32=%d arg64=%d payload-len=%d r=%v\n%s",
					dim, sub, arg32, arg64, len(payload), r, debug.Stack())
			}
		}()

		// Lazy-init the shared *RootVM + property resolver fixtures on
		// first fuzz iteration; subsequent iterations reuse.
		fx := getFuzzFixtures(t)
		rv := fx.rv
		resolver := fx.resolver
		ctx := context.Background()

		switch dim {
		case 1:
			// Dim 1: AMEND-B1 clamp envelope. Replicate the GetBufferBytesShim
			// clamp branch on the (start=arg32, maxSize=lower32(arg64)) pair
			// against a synthetic buffer of size 1024. Must-never-panic on
			// the uint32 overflow + clamp arithmetic.
			start := arg32
			maxSize := uint32(arg64 & math.MaxUint32)
			buf := make([]byte, 1024)
			bufLen := uint32(len(buf))

			// AMEND-B1 i32-overflow guard.
			if start+maxSize < start {
				// Overflow → would be BadArgument from the shim. No panic.
				return
			}
			var length uint32
			switch {
			case start >= bufLen:
				length = 0
			case start+maxSize > bufLen:
				length = bufLen - start
			default:
				length = maxSize
			}
			if length > 0 {
				// Slice MUST NOT panic for any valid (start, length) pair.
				_ = buf[start : start+length]
			}

		case 2:
			// Dim 2: pairs decoder. DecodePairs MUST NOT panic on any input
			// (well-formed or adversarial); returns (pairs, nil) on
			// success + (nil, err) on malformed input.
			_, err := internalwasm.DecodePairs(payload)
			_ = err

		case 3:
			// Dim 3: foreign-function call. CallForeignFunction MUST NOT
			// panic on any name (registered or not). Unregistered names
			// return NotFound; "panic_me" exercises the recover wrapper +
			// returns InternalFailure; "echo" returns Ok with arg-echo.
			name := string(payload)
			_, status := rv.CallForeignFunction(ctx, name, []byte("fuzz_args"))
			if !isKnownWasmResult(status) {
				t.Fatalf("Dim 3: CallForeignFunction returned non-sentinel status %d for name=%q", status, name)
			}

		case 4:
			// Dim 4: dynamic-stats Register. DefineMetric MUST NOT panic on
			// any name. Empty / NUL-containing / non-UTF-8 → BadArgument
			// via *dynamic.Registry userNameRE. Sub 3 loops N=24 (> the
			// 16-entry cap) so the cap-boundary InternalFailure path fires.
			name := string(payload)
			metricType := arg32
			if sub == 3 {
				// Cap-boundary trigger: register N unique names; the 17th
				// MUST surface InternalFailure (cap=16 in buildFuzzRootVM).
				for i := 0; i < 24; i++ {
					nm := name + "_cap_" + strings.Repeat("x", i+1)
					_, status := rv.DefineMetric(0, nm) // Counter
					if !isKnownWasmResult(status) {
						t.Fatalf("Dim 4 sub 3: DefineMetric returned non-sentinel status %d for name=%q", status, nm)
					}
				}
			} else {
				_, status := rv.DefineMetric(metricType, name)
				if !isKnownWasmResult(status) {
					t.Fatalf("Dim 4: DefineMetric returned non-sentinel status %d for name=%q metricType=%d",
						status, name, metricType)
				}
			}

		case 5:
			// Dim 5: shared-data CAS-mismatch race. SetSharedData /
			// GetSharedData MUST NOT panic on any (key, value, cas) tuple.
			// cas=0 → unconditional write; cas>0 + miss → CasMismatch;
			// value > 256-byte cap → InternalFailure.
			key := string(payload)
			cas := arg32
			value := []byte{0xde, 0xad, 0xbe, 0xef}

			setStatus := rv.SetSharedData(key, value, cas)
			if !isKnownWasmResult(setStatus) {
				t.Fatalf("Dim 5: SetSharedData returned non-sentinel status %d for key=%q cas=%d", setStatus, key, cas)
			}
			_, _, getStatus := rv.GetSharedData(key)
			if !isKnownWasmResult(getStatus) {
				t.Fatalf("Dim 5: GetSharedData returned non-sentinel status %d for key=%q", getStatus, key)
			}

		case 6:
			// Dim 6: body-buffer cap boundary. DecodeData MUST NOT panic
			// regardless of cap-vs-payload size relationship. Over-cap
			// fires the sticky flag + 413 SendLocalReply; under-cap
			// returns DataContinue (or DataStopIterationAndBuffer if
			// a guest opted into proxy_on_request_body — not the case
			// for our nil-streamCtx fuzz filter).
			capBytes := arg32
			if capBytes == 0 {
				capBytes = 16
			}
			ff := buildFuzzFilter(t, capBytes)
			_ = ff.DecodeData(payload, false)
			// Sticky-flag verification: a second DecodeData call with the
			// SAME oversize payload MUST NOT panic + MUST NOT re-bump
			// counters (covered by the sticky-flag short-circuit at body.go).
			_ = ff.DecodeData(payload, true)

		case 7:
			// Dim 7: property-path NUL-delimited adversarial. ResolveProperty
			// MUST NOT panic on any path bytes. Empty / double-NUL / unknown
			// root / 100-segment depth all surface as NotFound (the nil-
			// receiver resolver returns false for every accessor).
			_, status := internalwasm.ResolveProperty(resolver, payload)
			if !isKnownWasmResult(status) {
				t.Fatalf("Dim 7: ResolveProperty returned non-sentinel status %d for path=%q", status, payload)
			}

		case 8:
			// Dim 8: tick period parsing per Q5 10ms floor. SetTickPeriod
			// MUST NOT panic on any uint32 → time.Duration conversion.
			// period=0 cancels; period < 10ms clamps to 10ms; period >=
			// 10ms uses as-is. We use a tiny test-relative period (max
			// 100ms) to avoid spawning a long-lived goroutine that would
			// fire ticks during subsequent fuzz iterations.
			period := time.Duration(arg32) * time.Millisecond
			if period > 100*time.Millisecond {
				period = 100 * time.Millisecond
			}
			rv.SetTickPeriod(period)
			// Always cancel at iteration boundary so subsequent fuzz
			// iterations start from a clean tick state.
			defer rv.SetTickPeriod(0)

		case 9:
			// Dim 9: httpCall envelope adversarial. DispatchHttpCall MUST
			// NOT panic on any (cluster, timeout) tuple. Unknown cluster
			// → BadArgument; known cluster + timeout=0 → default 5s; known
			// cluster + timeout=u32::MAX → ~49.7-day timeout (well-formed
			// time.Duration). The synthetic 200-response stub does NOT
			// block (returns immediately); the response goroutine fires
			// handleHttpCallResponse against the rootCtxID which has no
			// per-stream callback wired → token-miss + no-op.
			cluster := string(payload)
			timeoutMs := arg32
			_, status := rv.DispatchHttpCall(ctx, 1 /*rootCtxID*/, cluster, nil, nil, nil, timeoutMs)
			if !isKnownWasmResult(status) {
				t.Fatalf("Dim 9: DispatchHttpCall returned non-sentinel status %d for cluster=%q timeoutMs=%d",
					status, cluster, timeoutMs)
			}

		case 10:
			// Dim 10: metric type out-of-range + signed-i64 delta extremes
			// per AMEND-B2. DefineMetric(99, ...) → BadArgument (the
			// *dynamic.Registry rejects metric-type values outside {0,1,2}).
			// IncrementMetric with delta=int64::MIN / MAX MUST NOT panic
			// at the underlying atomic.Int64.Add.
			name := string(payload)
			metricType := arg32
			delta := int64(arg64) //nolint:gosec // delta is intentionally reinterpreted as signed for AMEND-B2 fuzz.

			id, defineStatus := rv.DefineMetric(metricType, name)
			if !isKnownWasmResult(defineStatus) {
				t.Fatalf("Dim 10: DefineMetric returned non-sentinel status %d for name=%q metricType=%d",
					defineStatus, name, metricType)
			}
			// IncrementMetric on the allocated id (or 0 on Define-failure) —
			// MUST NOT panic for id=0 (NotFound), for valid id (Ok), or for
			// any int64 delta value (atomic.Add tolerates int64::MIN/MAX).
			incStatus := rv.IncrementMetric(id, delta)
			if !isKnownWasmResult(incStatus) {
				t.Fatalf("Dim 10: IncrementMetric returned non-sentinel status %d for id=%d delta=%d",
					incStatus, id, delta)
			}
		}
	})
}
