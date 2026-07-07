package lua

// compile.go — Chunk + CompileCache + NewCompileCache + CompileScript
// per 22.1 SPEC §3.1 production signatures.
//
// Task 4 IMPL — content-addressed compile cache.
//
// CompileScript drives gopher-lua's two-stage compile pipeline
// (`parse.Parse` → AST; `lua.Compile` → bytecode `*FunctionProto`)
// without instantiating an `*lua.LState`. The resulting
// `*FunctionProto` is bytecode-only — at script Run time each
// per-stream `*lua.LState` cheaply loads it via
// `state.NewFunctionFromProto(proto)` + `state.Push` + `state.PCall`
// without re-parsing. The same `*FunctionProto` is safe to share
// across N concurrent `*lua.LState`s per the gopher-lua API contract
// (verified at IMPL Task 12 concurrent-VM-construction tests).
//
// CompileCache discipline (per PLAN D-P5 + ADR-0085):
//
//   1. Compute hash := sha256.Sum256(src) (32 bytes; collision-
//      resistant + content-addressed; matches the SPEC §3.1
//      `hash [32]byte` field type).
//   2. If cache != nil: RLock; check store[hash]; if hit, RUnlock +
//      return cached. RUnlock.
//   3. Parse + compile via gopher-lua (expensive; performed OUTSIDE
//      the lock — long-held write lock would serialize all compile
//      misses across the filter).
//   4. Build *Chunk{proto}.
//   5. If cache != nil: Lock; DOUBLE-CHECK store[hash] (another
//      goroutine may have raced ahead during step 3 + already
//      stored the same content); if present, return that one (drop
//      the just-compiled chunk); else store new. Unlock.
//   6. Return.
//
// Nil-tolerance per ADR-0085: CompileScript(src, nil) skips both
// cache branches; just parses + compiles + returns. No panic on
// nil-deref. Useful for one-shot compilation paths (e.g. fuzz
// harness; direct test invocations).
//
// Error wording: gopher-lua parse/compile errors are wrapped as
// `fmt.Errorf("lua compile: %w", err)`. The `%w` verb preserves
// `errors.Is` / `errors.As` chains so downstream consumers (the
// filter-layer arm 16 `"lua: default_source_code: compile: %w"`
// wrapper + the BootRejectFixture stderr-substring assertion) can
// reach the original gopher-lua sentinel.

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// chunkSourceName is the fixed source-name stamped onto every
// compiled chunk via `parse.Parse` + `lua.Compile`. Lua error
// messages reference this name (e.g. `[string "lua_filter_chunk"]:
// 5: error message`). Holding it stable across calls keeps log
// output reproducible across config-load + per-stream runs; at
// 22.3 the per-filter source name MAY surface from the SourceCodes
// map key (deferred).
const chunkSourceName = "lua_filter_chunk"

// Chunk wraps a compiled *FunctionProto safe for cross-VM reuse. Cache
// identity at the CompileCache is the SHA-256(src) map key computed at
// CompileScript; the proto is loaded onto each per-stream *lua.LState
// via state.NewFunctionFromProto without recompilation.
type Chunk struct {
	proto *lua.FunctionProto
}

// CompileCache is a content-addressed compile cache, keyed by
// sha256(src). Owned by the *compiledConfig (filter-config-instance
// scope per PLAN D-P5; GC-driven eviction — no manual evict; cache
// lifetime == compiledConfig lifetime). Safe for concurrent
// read/add via internal sync.RWMutex.
type CompileCache struct {
	mu    sync.RWMutex
	store map[[32]byte]*Chunk
}

// NewCompileCache constructs an empty cache. Caller responsibility:
// keep alive for the duration of the dependent compiledConfig.
func NewCompileCache() *CompileCache {
	return &CompileCache{
		store: make(map[[32]byte]*Chunk),
	}
}

// CompileScript compiles src via gopher-lua's parser. If cache is
// non-nil, caches by sha256(src) — subsequent calls with the same src
// return the cached *Chunk without re-parsing. If cache is nil,
// compiles uncached (per ADR-0085 nil-tolerance discipline). Returns
// the underlying gopher-lua parse/compile error wrapped via `%w` as
// `"lua compile: <inner>"` on failure.
func CompileScript(src []byte, cache *CompileCache) (*Chunk, error) {
	hash := sha256.Sum256(src)

	// Cache fast-path: RLock check. The lookup is held under
	// RLock for the minimum window — release before the expensive
	// parse + compile.
	if cache != nil {
		cache.mu.RLock()
		if existing, ok := cache.store[hash]; ok {
			cache.mu.RUnlock()
			return existing, nil
		}
		cache.mu.RUnlock()
	}

	// Parse + compile OUTSIDE the lock. Multiple goroutines may
	// race here; the double-checked write at step 5 below dedupes.
	ast, err := parse.Parse(bytes.NewReader(src), chunkSourceName)
	if err != nil {
		return nil, fmt.Errorf("lua compile: %w", err)
	}
	proto, err := lua.Compile(ast, chunkSourceName)
	if err != nil {
		return nil, fmt.Errorf("lua compile: %w", err)
	}

	chunk := &Chunk{proto: proto}

	// Cache write-path: Lock + DOUBLE-CHECK store[hash]. If another
	// goroutine populated the slot during our parse/compile window,
	// drop the just-compiled chunk + return the existing — preserves
	// the cache-hit pointer-equality invariant across concurrent
	// racing callers.
	if cache != nil {
		cache.mu.Lock()
		if existing, ok := cache.store[hash]; ok {
			cache.mu.Unlock()
			return existing, nil
		}
		cache.store[hash] = chunk
		cache.mu.Unlock()
	}

	return chunk, nil
}
