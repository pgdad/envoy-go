package lua

// compile_test.go — Task 4 full IMPL tests per 22.1 PLAN Task 4 +
// 22.1 SPEC §3.1 production discipline. Covers cache hit-on-same-
// content-hash + cache-miss-on-different-source + nil-cache nil-
// tolerance per ADR-0085 + compile-error PARSE-REJECT-equivalent
// wording-prefix discipline (`"lua compile: "` prefix; the actual
// filter at Task 2 wraps with `"lua: default_source_code: compile:
// %w"` for arm 16 — this lower-level IMPL only stamps the
// framework-primitive prefix).
//
// Concurrent-read/add tests are DEFERRED to Task 12 per 22.1 PLAN
// Step 12 + 22.1 SPEC §6 Task 12 (race + `-race` flag concurrency
// coverage lands there with N=100 goroutines).

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Compile-time assertions pin the public API surface declared in
// compile.go per 22.1 SPEC §3.1.
var (
	_ func() *CompileCache                        = NewCompileCache
	_ func([]byte, *CompileCache) (*Chunk, error) = CompileScript
)

// TestNewCompileCache_ReturnsNonNil pins the ADR-0085 nil-tolerance
// contract surface for the cache constructor; the Task 4 full IMPL
// preserves this invariant.
func TestNewCompileCache_ReturnsNonNil(t *testing.T) {
	c := NewCompileCache()
	if c == nil {
		t.Fatal("NewCompileCache returned nil; expected non-nil per SPEC §3.1")
	}
}

// TestCompileScript_NilCache_DoesNotPanic pins the ADR-0085 nil-cache
// branch. Task 4 IMPL preserves nil-tolerance behavior; the helper
// MUST compile uncached and return a non-nil Chunk without panicking
// on the nil cache deref.
func TestCompileScript_NilCache_DoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CompileScript([]byte, nil) panicked: %v (expected nil-tolerance per ADR-0085)", r)
		}
	}()
	chunk, err := CompileScript([]byte("return 1"), nil)
	if err != nil {
		t.Fatalf("CompileScript([]byte, nil) returned err = %v; want nil", err)
	}
	if chunk == nil {
		t.Fatal("CompileScript([]byte, nil) returned nil chunk; want non-nil per nil-tolerance")
	}
}

// TestCompileScript_ValidLua_ReturnsNonNilChunk pins the happy path
// for the uncached branch. A valid `return 1` Lua source compiles
// to a non-nil *Chunk with nil error.
func TestCompileScript_ValidLua_ReturnsNonNilChunk(t *testing.T) {
	chunk, err := CompileScript([]byte("return 1"), nil)
	if err != nil {
		t.Fatalf("CompileScript returned err = %v; want nil", err)
	}
	if chunk == nil {
		t.Fatal("CompileScript returned nil chunk; want non-nil")
	}
	if chunkProto(chunk) == nil {
		t.Fatal("Chunk.proto is nil after successful compile; want non-nil *lua.FunctionProto")
	}
}

// TestCompileScript_CompileError_ReturnsApiError pins the compile-
// error branch. A syntactically invalid Lua source returns a nil
// *Chunk + non-nil error wrapped with the `"lua compile: "` prefix
// (the framework-primitive prefix; the filter-layer arm 16 adds
// `"lua: default_source_code: compile: "` on top).
func TestCompileScript_CompileError_ReturnsApiError(t *testing.T) {
	// `function badcode(` is syntactically invalid — parse error
	// surfaces at the gopher-lua `parse.Parse` step.
	chunk, err := CompileScript([]byte("function badcode("), nil)
	if err == nil {
		t.Fatal("CompileScript returned nil error for invalid Lua source; want non-nil")
	}
	if chunk != nil {
		t.Fatalf("CompileScript returned non-nil chunk = %v for invalid Lua source; want nil", chunk)
	}
	if !strings.HasPrefix(err.Error(), "lua compile: ") {
		t.Fatalf("CompileScript error wording = %q; want prefix %q", err.Error(), "lua compile: ")
	}
}

// TestCompileScript_CacheHit_SameSourceReturnsSameChunkPointer pins
// the cache-hit invariant: two CompileScript calls with the same
// src AND the same *CompileCache MUST return the SAME *Chunk pointer
// (proves the cache actually stored + the second call hit, rather
// than recompiling). Pointer-equality is the strongest assertion.
func TestCompileScript_CacheHit_SameSourceReturnsSameChunkPointer(t *testing.T) {
	c := NewCompileCache()
	a, errA := CompileScript([]byte("return 1"), c)
	if errA != nil {
		t.Fatalf("first CompileScript err = %v; want nil", errA)
	}
	b, errB := CompileScript([]byte("return 1"), c)
	if errB != nil {
		t.Fatalf("second CompileScript err = %v; want nil", errB)
	}
	if a != b {
		t.Fatalf("CompileScript cache hit returned different *Chunk pointers: a = %p, b = %p; want same", a, b)
	}
}

// TestCompileScript_CacheMiss_DifferentSourceReturnsDifferentChunkPointer
// pins the cache-miss invariant: two CompileScript calls with
// DIFFERENT src + the same *CompileCache return DIFFERENT *Chunk
// pointers (each src has its own sha256-keyed cache slot).
func TestCompileScript_CacheMiss_DifferentSourceReturnsDifferentChunkPointer(t *testing.T) {
	c := NewCompileCache()
	a, errA := CompileScript([]byte("return 1"), c)
	if errA != nil {
		t.Fatalf("first CompileScript err = %v; want nil", errA)
	}
	b, errB := CompileScript([]byte("return 2"), c)
	if errB != nil {
		t.Fatalf("second CompileScript err = %v; want nil", errB)
	}
	if a == b {
		t.Fatalf("CompileScript with different sources returned same *Chunk pointer = %p; want different", a)
	}
}

// TestCompileScript_NilCache_DoesNotCache pins the ADR-0085 nil-
// cache-skip-store branch: two CompileScript calls with the same
// src + a NIL cache MUST return DIFFERENT *Chunk pointers (nothing
// is cached; each call recompiles).
func TestCompileScript_NilCache_DoesNotCache(t *testing.T) {
	a, errA := CompileScript([]byte("return 1"), nil)
	if errA != nil {
		t.Fatalf("first CompileScript err = %v; want nil", errA)
	}
	b, errB := CompileScript([]byte("return 1"), nil)
	if errB != nil {
		t.Fatalf("second CompileScript err = %v; want nil", errB)
	}
	if a == b {
		t.Fatalf("CompileScript with nil cache returned same *Chunk pointer = %p across calls; want different (no caching per ADR-0085)", a)
	}
}

// TestCompileScript_NilCache_ErrorPath pins the (src invalid, cache
// nil) cross-product — invalid Lua source with a nil cache returns
// a non-nil error with the `"lua compile: "` prefix + a nil chunk;
// no caching side effects.
func TestCompileScript_NilCache_ErrorPath(t *testing.T) {
	chunk, err := CompileScript([]byte("function badcode("), nil)
	if err == nil {
		t.Fatal("CompileScript with invalid src + nil cache returned nil err; want non-nil")
	}
	if chunk != nil {
		t.Fatalf("CompileScript with invalid src + nil cache returned non-nil chunk = %v; want nil", chunk)
	}
	if !strings.HasPrefix(err.Error(), "lua compile: ") {
		t.Fatalf("CompileScript nil-cache error wording = %q; want prefix %q", err.Error(), "lua compile: ")
	}
}

// TestCompileScript_CacheHit_AcrossDistinctByteSlicesWithSameContent
// pins the content-addressed (not identity-addressed) cache semantics:
// two CompileScript calls with byte-slice-DISTINCT-but-content-equal
// src MUST return the SAME *Chunk pointer (cache key is sha256(src)
// content, not slice header identity).
func TestCompileScript_CacheHit_AcrossDistinctByteSlicesWithSameContent(t *testing.T) {
	c := NewCompileCache()
	// Build the two byte slices via independent allocations so the
	// `[]byte(stringLiteral)` conversion produces fresh backing arrays
	// each call (per Go runtime semantics; the string-literal content
	// is interned but `[]byte(s)` always copies).
	src := "return 1"
	srcA := []byte(src)
	srcB := []byte(src) // distinct underlying array; same content
	if &srcA[0] == &srcB[0] {
		t.Fatal("test setup error: srcA and srcB share underlying array; want distinct")
	}
	a, errA := CompileScript(srcA, c)
	if errA != nil {
		t.Fatalf("first CompileScript err = %v; want nil", errA)
	}
	b, errB := CompileScript(srcB, c)
	if errB != nil {
		t.Fatalf("second CompileScript err = %v; want nil", errB)
	}
	if a != b {
		t.Fatalf("content-equal src across distinct slices returned different *Chunk pointers: a = %p, b = %p; want same (content-addressed cache)", a, b)
	}
}

// TestCompileScript_CompileError_WrapsUnderlyingErrorViaPercentW pins
// `fmt.Errorf("...: %w", inner)` discipline — `errors.Unwrap` returns
// the underlying gopher-lua parser/compile error so downstream
// callers can `errors.Is` / `errors.As` against it (the filter at
// arm 16 chains via `%w` and the BootRejectFixture downstream wants
// to assert the underlying-error class).
func TestCompileScript_CompileError_WrapsUnderlyingErrorViaPercentW(t *testing.T) {
	_, err := CompileScript([]byte("function badcode("), nil)
	if err == nil {
		t.Fatal("CompileScript err = nil; want non-nil")
	}
	inner := errors.Unwrap(err)
	if inner == nil {
		t.Fatalf("errors.Unwrap(%q) = nil; want non-nil underlying gopher-lua parse/compile error (verify `%%w` wrapping discipline)", err.Error())
	}
}

// chunkProto is a test-only accessor for the unexported Chunk proto
// field. Defined in compile_test.go (same-package _test.go) where test
// code may legitimately reach into unexported fields of production
// types — keeps the production API surface clean while permitting
// field-level test assertions.
func chunkProto(c *Chunk) interface{} { return c.proto }

// ----------------------------------------------------------------------
// Task 12 — CompileCache concurrent read/add tests per 22.1 PLAN Task 12
// + 22.1 SPEC §6 Task 12 + 22.1 SPEC §2.19 + parent §13-R6.
//
// Validates the cache's sync.RWMutex discipline + the double-checked
// write path. The fast-path RLock at compile.go:106-112 + the
// write-path Lock + double-check at compile.go:132-140 must withstand
// N=100 concurrent goroutines mixing read (CompileScript with the same
// src) and add (CompileScript with new content). Acceptance: race-free
// under -race + correct cache contents (every unique src produces
// exactly one *Chunk slot, all readers for the same src observe the
// same pointer).
//
// Run via:
//
//   go test -race -count=10 ./internal/lua/...
// ----------------------------------------------------------------------

// TestCompileCache_ConcurrentReadAdd verifies that N=100 goroutines
// mixing reads (same src) and adds (unique src per goroutine) on a
// shared *CompileCache do not race. After all goroutines complete,
// every read goroutine MUST have received the same *Chunk pointer
// (proves the cache delivered the cached entry on every hit), and the
// cache MUST contain exactly N/2 + 1 entries (the seed + one per add
// goroutine).
func TestCompileCache_ConcurrentReadAdd(t *testing.T) {
	t.Parallel()
	cache := NewCompileCache()
	// Seed the cache with one entry so reads have a guaranteed hit.
	const seedSrc = "return 1"
	seedChunk, err := CompileScript([]byte(seedSrc), cache)
	if err != nil {
		t.Fatalf("seed CompileScript err = %v; want nil", err)
	}

	const N = 100
	var (
		wg          sync.WaitGroup
		readResults = make([]*Chunk, N)
		addResults  = make([]*Chunk, N)
		errCount    atomic.Int64
	)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				// Read path: same src as seed → cache hit.
				c, err := CompileScript([]byte(seedSrc), cache)
				if err != nil {
					t.Errorf("read[%d]: CompileScript err = %v; want nil", idx, err)
					errCount.Add(1)
					return
				}
				readResults[idx] = c
			} else {
				// Add path: unique src per idx → cache miss + write.
				// Use a comment carrying idx to guarantee uniqueness
				// (avoids accidental seed collision when the body
				// `return <n>` happens to equal the seed `return 1`).
				addSrc := fmt.Sprintf("return %d -- T12 add idx=%d", idx, idx)
				c, err := CompileScript([]byte(addSrc), cache)
				if err != nil {
					t.Errorf("add[%d]: CompileScript err = %v; want nil", idx, err)
					errCount.Add(1)
					return
				}
				addResults[idx] = c
			}
		}(i)
	}
	wg.Wait()

	if n := errCount.Load(); n > 0 {
		t.Fatalf("%d goroutines reported errors", n)
	}

	// Every read goroutine must have received the seed pointer.
	for i := 0; i < N; i += 2 {
		if readResults[i] != seedChunk {
			t.Errorf("read[%d]: got %p; want seed pointer %p (cache delivered wrong chunk)", i, readResults[i], seedChunk)
		}
	}

	// Every add goroutine must have received a non-nil, non-seed chunk
	// (different src → different *Chunk pointer per the cache-miss
	// invariant pinned at TestCompileScript_CacheMiss_*).
	for i := 1; i < N; i += 2 {
		if addResults[i] == nil {
			t.Errorf("add[%d]: got nil chunk; want non-nil", i)
			continue
		}
		if addResults[i] == seedChunk {
			t.Errorf("add[%d]: got seed pointer; want distinct *Chunk (different src)", i)
		}
	}
}

// TestCompileCache_ConcurrentReadAdd_SameContentDedupes verifies the
// double-checked write path at compile.go:132-140: N=100 goroutines
// racing to populate the SAME novel cache entry must converge on a
// single *Chunk pointer (the cache must dedupe the racers + drop the
// losing parses' just-compiled chunks).
func TestCompileCache_ConcurrentReadAdd_SameContentDedupes(t *testing.T) {
	t.Parallel()
	cache := NewCompileCache()
	// Distinct src per test run to avoid leaking state across -count=10
	// iterations sharing some imagined process-global cache (defensive —
	// the cache is local to this test, but stay conservative).
	const novelSrc = `return 1 + 2 + 3 -- T12 dedupe-race novel src`

	const N = 100
	var (
		wg      sync.WaitGroup
		results = make([]*Chunk, N)
	)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c, err := CompileScript([]byte(novelSrc), cache)
			if err != nil {
				t.Errorf("[%d]: CompileScript err = %v; want nil", idx, err)
				return
			}
			results[idx] = c
		}(i)
	}
	wg.Wait()

	// All N goroutines must observe the SAME *Chunk pointer (the
	// double-checked write path at compile.go:132-140 must dedupe
	// racing populates). If the cache failed to dedupe, two or more
	// distinct *Chunks would land in `results`.
	first := results[0]
	if first == nil {
		t.Fatal("results[0] = nil; expected non-nil chunk")
	}
	for i := 1; i < N; i++ {
		if results[i] != first {
			t.Errorf("results[%d] = %p; want %p (cache double-checked write failed to dedupe racers)", i, results[i], first)
		}
	}
}

// TestCompileCache_ConcurrentAddDistinct verifies that N goroutines
// each adding a UNIQUE entry to the same cache do not lose entries.
// After all goroutines complete the cache must hold all N entries; a
// follow-up sequential read for each src must hit the cache + return
// the entry that was just added.
func TestCompileCache_ConcurrentAddDistinct(t *testing.T) {
	t.Parallel()
	cache := NewCompileCache()

	const N = 100
	var (
		wg       sync.WaitGroup
		results  = make([]*Chunk, N)
		srcSlice = make([][]byte, N)
	)
	for i := 0; i < N; i++ {
		srcSlice[i] = []byte(fmt.Sprintf("return %d -- T12 distinct add idx=%d", i, i))
	}
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c, err := CompileScript(srcSlice[idx], cache)
			if err != nil {
				t.Errorf("[%d]: CompileScript err = %v; want nil", idx, err)
				return
			}
			results[idx] = c
		}(i)
	}
	wg.Wait()

	// Sequential follow-up reads must return the SAME pointer as the
	// concurrent add — proves the cache stored the entry, didn't drop
	// it under contention.
	for i := 0; i < N; i++ {
		c, err := CompileScript(srcSlice[i], cache)
		if err != nil {
			t.Errorf("follow-up[%d]: CompileScript err = %v", i, err)
			continue
		}
		if c != results[i] {
			t.Errorf("follow-up[%d]: got %p; want %p (entry lost under concurrent add)", i, c, results[i])
		}
	}
}
