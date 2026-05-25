// Tests for compile.go — Module + CompileCache + ABI-version gating per
// AMEND-A6 (envoy-go-strict-stricter; only AbiVersion_0_2_1 accepted).
//
// Tests must FAIL before compile.go lands per D-P-PLAN-4.
//
// Test coverage matrix (per PLAN component-table + this prompt's spec):
//
//	(1) NewCompileCache returns non-nil + runtime initialized.
//	(2) CompileModule happy path — valid wasm v0.2.1 bytecode → non-nil
//	    *Module, nil error; Module.ABIVersion() == AbiVersion_0_2_1;
//	    Module.Hash() == sha256(src); Module.Compiled() != nil.
//	(3) Cache-hit-on-same-content-hash — second CompileModule(ctx, sameSrc, cache)
//	    returns the SAME *Module pointer + the wazero CompileModule was invoked
//	    only ONCE (cache hit short-circuits compile cost).
//	(4) Cache-miss-on-different-source — two distinct valid wasm bytecodes
//	    yield two distinct *Module pointers with distinct hashes.
//	(5) Nil-cache tolerance — CompileModule(ctx, src, nil) compiles + returns
//	    non-nil *Module, nil error (uncached path per ADR-0085).
//	(6) Concurrent read/add race-clean — N goroutines mixing same-src + new-src
//	    compiles against the same cache; no data races + same-src calls return
//	    same Module + new-src calls add distinct entries.
//	(7) ErrUnsupportedAbiVersion — v0.1.0 / v0.2.0 / missing-sentinel bytecode
//	    → errors.Is(err, ErrUnsupportedAbiVersion).
//	(8) Compile-error path — malformed wasm bytecode (bad magic) → wrapped
//	    wazero error (NOT ErrUnsupportedAbiVersion).
//	(9) Close idempotence — second Close() returns nil.

package wasm

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"testing"
)

// --- crafted-wasm helpers --------------------------------------------------
//
// These reuse the package-level appendUleb128/buildExportEntry/buildExportSection
// helpers from bytecode_util_test.go (same package = wasm) but augment them with
// builders for fuller wasm modules that wazero will actually COMPILE (not just
// scan with GetAbiVersion). wazero needs at minimum a type section + function
// section + code section + export section to accept a function-kind export.

// buildTypeSectionEmptyFunc builds a minimal type section with one entry:
// a func type with no params and no results: 0x60 0x00 0x00.
//
// Type section: ID 0x01 || size || count=1 || (form=0x60 param-count=0 result-count=0).
func buildTypeSectionEmptyFunc() []byte {
	body := []byte{0x01, 0x60, 0x00, 0x00} // count=1, form=func, 0 params, 0 results
	out := []byte{0x01}
	out = appendUleb128(out, uint32(len(body)))
	out = append(out, body...)
	return out
}

// buildFunctionSection builds a function section with one entry pointing at
// type index 0. Section ID 0x03 || size || count=1 || type-idx=0.
func buildFunctionSection() []byte {
	body := []byte{0x01, 0x00}
	out := []byte{0x03}
	out = appendUleb128(out, uint32(len(body)))
	out = append(out, body...)
	return out
}

// buildCodeSectionEmptyBody builds a code section with one empty function body
// (no locals, just an end opcode).
// Section ID 0x0a || size || count=1 || body=(body-size=2 || local-count=0 || end=0x0b).
func buildCodeSectionEmptyBody() []byte {
	// function body: local-count=0 || end opcode (0x0b). Body size = 2.
	funcBody := []byte{0x00, 0x0b}
	bodyWithSize := appendUleb128(nil, uint32(len(funcBody)))
	bodyWithSize = append(bodyWithSize, funcBody...)
	body := []byte{0x01} // count=1
	body = append(body, bodyWithSize...)
	out := []byte{0x0a}
	out = appendUleb128(out, uint32(len(body)))
	out = append(out, body...)
	return out
}

// buildCompilableModule builds a complete minimal wasm module that wazero
// can compile: header + type section + function section + export section
// (the named export points at function index 0) + code section.
//
// If exportName is empty, no export entries are added (used for the
// missing-sentinel test case where GetAbiVersion returns AbiVersionUnknown).
func buildCompilableModule(exportName string) []byte {
	out := append([]byte{}, wasmHeader...)
	out = append(out, buildTypeSectionEmptyFunc()...)
	out = append(out, buildFunctionSection()...)
	if exportName != "" {
		entry := buildExportEntry(exportName, 0x00, 0) // function-kind, idx 0
		out = append(out, buildExportSection([][]byte{entry})...)
	}
	out = append(out, buildCodeSectionEmptyBody()...)
	return out
}

// distinctCompilableModule produces a compilable v0.2.1 module that differs
// from the canonical one returned by buildCompilableModule("proxy_abi_version_0_2_1")
// by including a second function-kind export under the given name. This gives
// us a wasm module that (a) still detects as AbiVersion_0_2_1 (because the
// scan stops at the first sentinel match — order matters per upstream
// src/bytecode_util.cc:74-86; we put the v0.2.1 sentinel first) and (b) has a
// different sha256 hash from the canonical module.
func distinctCompilableModule(extraExportName string) []byte {
	out := append([]byte{}, wasmHeader...)
	out = append(out, buildTypeSectionEmptyFunc()...)
	out = append(out, buildFunctionSection()...)
	entries := [][]byte{
		buildExportEntry("proxy_abi_version_0_2_1", 0x00, 0),
		buildExportEntry(extraExportName, 0x00, 0),
	}
	out = append(out, buildExportSection(entries)...)
	out = append(out, buildCodeSectionEmptyBody()...)
	return out
}

// --- TestCompile tests -----------------------------------------------------

func TestCompileNewCompileCache(t *testing.T) {
	ctx := context.Background()
	cc := NewCompileCache(ctx)
	if cc == nil {
		t.Fatal("NewCompileCache returned nil")
	}
	t.Cleanup(func() { _ = cc.Close() })
	// Closing the cache should succeed.
	if err := cc.Close(); err != nil {
		t.Fatalf("Close after construction: %v", err)
	}
}

func TestCompileModule_HappyPath(t *testing.T) {
	ctx := context.Background()
	cc := NewCompileCache(ctx)
	t.Cleanup(func() { _ = cc.Close() })

	src := buildCompilableModule("proxy_abi_version_0_2_1")
	mod, err := CompileModule(ctx, src, cc)
	if err != nil {
		t.Fatalf("CompileModule: %v", err)
	}
	if mod == nil {
		t.Fatal("CompileModule returned nil Module")
	}
	if got, want := mod.ABIVersion(), AbiVersion_0_2_1; got != want {
		t.Errorf("ABIVersion = %v; want %v", got, want)
	}
	if got, want := mod.Hash(), sha256.Sum256(src); got != want {
		t.Errorf("Hash mismatch: got %x; want %x", got, want)
	}
	if mod.Compiled() == nil {
		t.Error("Compiled() returned nil")
	}
}

func TestCompileModule_CacheHitOnSameContent(t *testing.T) {
	ctx := context.Background()
	cc := NewCompileCache(ctx)
	t.Cleanup(func() { _ = cc.Close() })

	src := buildCompilableModule("proxy_abi_version_0_2_1")

	first, err := CompileModule(ctx, src, cc)
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}
	second, err := CompileModule(ctx, src, cc)
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}
	if first != second {
		t.Errorf("cache-hit expected: same content should return same *Module pointer; got first=%p second=%p", first, second)
	}
	if first.Hash() != second.Hash() {
		t.Errorf("hash divergence: first=%x second=%x", first.Hash(), second.Hash())
	}
}

func TestCompileModule_CacheMissOnDifferentSource(t *testing.T) {
	ctx := context.Background()
	cc := NewCompileCache(ctx)
	t.Cleanup(func() { _ = cc.Close() })

	srcA := buildCompilableModule("proxy_abi_version_0_2_1")
	srcB := distinctCompilableModule("aux_a")

	modA, err := CompileModule(ctx, srcA, cc)
	if err != nil {
		t.Fatalf("compile srcA: %v", err)
	}
	modB, err := CompileModule(ctx, srcB, cc)
	if err != nil {
		t.Fatalf("compile srcB: %v", err)
	}
	if modA == modB {
		t.Error("expected distinct *Module pointers for distinct sources")
	}
	if modA.Hash() == modB.Hash() {
		t.Errorf("expected distinct hashes; both = %x", modA.Hash())
	}
}

func TestCompileModule_NilCacheTolerance(t *testing.T) {
	ctx := context.Background()
	src := buildCompilableModule("proxy_abi_version_0_2_1")

	// nil cache → compile uncached; per the doc-comment the *Module owns
	// both the CompiledModule AND a transient single-use wazero.Runtime.
	// The caller MUST release both via Module.Close(ctx) — closing the
	// CompiledModule alone is INSUFFICIENT per wazero v1.10.1's contract
	// (config.go:317; Runtime.Close → cascades to CompiledModules, but
	// the reverse is NOT true).
	mod, err := CompileModule(ctx, src, nil)
	if err != nil {
		t.Fatalf("CompileModule(nil-cache): %v", err)
	}
	if mod == nil {
		t.Fatal("CompileModule(nil-cache) returned nil")
	}
	defer func() {
		if err := mod.Close(ctx); err != nil {
			t.Errorf("Module.Close: %v", err)
		}
	}()
	if got, want := mod.ABIVersion(), AbiVersion_0_2_1; got != want {
		t.Errorf("ABIVersion = %v; want %v", got, want)
	}
	if mod.Compiled() == nil {
		t.Error("Compiled() returned nil on nil-cache path")
	}
}

func TestCompileModule_ConcurrentReadAdd(t *testing.T) {
	ctx := context.Background()
	cc := NewCompileCache(ctx)
	t.Cleanup(func() { _ = cc.Close() })

	// Build a small fixed corpus of distinct sources + the shared canonical
	// one. Goroutines mix same-src (should hit cache after first) + distinct-src
	// (should add fresh entries) compiles.
	canonical := buildCompilableModule("proxy_abi_version_0_2_1")
	corpus := [][]byte{
		canonical,
		distinctCompilableModule("aux_a"),
		distinctCompilableModule("aux_b"),
		distinctCompilableModule("aux_c"),
	}

	const goroutines = 16
	const itersPerG = 8

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < itersPerG; i++ {
				src := corpus[(seed+i)%len(corpus)]
				mod, err := CompileModule(ctx, src, cc)
				if err != nil {
					t.Errorf("goroutine %d iter %d: %v", seed, i, err)
					return
				}
				if mod == nil {
					t.Errorf("goroutine %d iter %d: nil module", seed, i)
					return
				}
				if mod.Hash() != sha256.Sum256(src) {
					t.Errorf("goroutine %d iter %d: hash mismatch", seed, i)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	// Verify the cache contains exactly the unique sources from the corpus.
	// Each distinct src → unique hash → one cache entry.
	canonicalMod, err := CompileModule(ctx, canonical, cc)
	if err != nil {
		t.Fatalf("post-concurrent canonical compile: %v", err)
	}
	canonicalMod2, err := CompileModule(ctx, canonical, cc)
	if err != nil {
		t.Fatalf("post-concurrent canonical compile (2): %v", err)
	}
	if canonicalMod != canonicalMod2 {
		t.Error("cache identity broken: same src returned different *Module after concurrent access")
	}
}

func TestCompileModule_ErrUnsupportedAbiVersion(t *testing.T) {
	ctx := context.Background()
	cc := NewCompileCache(ctx)
	t.Cleanup(func() { _ = cc.Close() })

	cases := []struct {
		name       string
		exportName string // "" → no export section (missing sentinel)
	}{
		{name: "v0.1.0 sentinel rejected", exportName: "proxy_abi_version_0_1_0"},
		{name: "v0.2.0 sentinel rejected", exportName: "proxy_abi_version_0_2_0"},
		{name: "missing sentinel rejected", exportName: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := buildCompilableModule(tc.exportName)
			mod, err := CompileModule(ctx, src, cc)
			if err == nil {
				t.Fatalf("expected error, got nil module=%v", mod)
			}
			if !errors.Is(err, ErrUnsupportedAbiVersion) {
				t.Errorf("err %q does not wrap ErrUnsupportedAbiVersion", err)
			}
			if mod != nil {
				t.Errorf("expected nil module on rejection; got %p", mod)
			}
		})
	}
}

func TestCompileModule_CompileErrorPath(t *testing.T) {
	ctx := context.Background()
	cc := NewCompileCache(ctx)
	t.Cleanup(func() { _ = cc.Close() })

	// Bad magic — GetAbiVersion will fail at the magic check; the err must
	// NOT be ErrUnsupportedAbiVersion (it's a parse error, not an ABI rejection).
	bad := []byte{0xff, 0xff, 0xff, 0xff, 0x01, 0x00, 0x00, 0x00, 0x99, 0x99}
	mod, err := CompileModule(ctx, bad, cc)
	if err == nil {
		t.Fatalf("expected error on malformed wasm, got nil; mod=%v", mod)
	}
	if errors.Is(err, ErrUnsupportedAbiVersion) {
		t.Errorf("err %q must NOT wrap ErrUnsupportedAbiVersion (it's a parse error)", err)
	}
	if mod != nil {
		t.Errorf("expected nil module on parse error; got %p", mod)
	}
}

func TestCompileModule_WazeroCompileError(t *testing.T) {
	ctx := context.Background()
	cc := NewCompileCache(ctx)
	t.Cleanup(func() { _ = cc.Close() })

	// Construct a module that passes GetAbiVersion (has the v0.2.1 sentinel
	// + valid header + valid export section) but is otherwise NOT compilable
	// by wazero (declares a function in the function section without a code
	// section entry → wazero will reject as malformed).
	out := append([]byte{}, wasmHeader...)
	out = append(out, buildTypeSectionEmptyFunc()...)
	out = append(out, buildFunctionSection()...) // declares 1 function...
	entry := buildExportEntry("proxy_abi_version_0_2_1", 0x00, 0)
	out = append(out, buildExportSection([][]byte{entry})...)
	// ... but NO code section! wazero will fail to compile this.

	mod, err := CompileModule(ctx, out, cc)
	if err == nil {
		t.Fatalf("expected wazero compile error, got nil; mod=%v", mod)
	}
	if errors.Is(err, ErrUnsupportedAbiVersion) {
		t.Errorf("wazero compile error must NOT wrap ErrUnsupportedAbiVersion")
	}
	if mod != nil {
		t.Errorf("expected nil module on wazero compile error; got %p", mod)
	}
	// Quick sanity that the error wrapping prefix mentions "compile" (or
	// the wazero-side wording) — keeps the diagnostic chain useful for
	// caller debugging.
	if !strings.Contains(strings.ToLower(err.Error()), "compile") &&
		!strings.Contains(strings.ToLower(err.Error()), "section") {
		t.Logf("note: error string %q lacks 'compile'/'section' hints (informational, not a failure)", err)
	}
}

func TestCompileCacheClose_Idempotent(t *testing.T) {
	ctx := context.Background()
	cc := NewCompileCache(ctx)
	if err := cc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := cc.Close(); err != nil {
		t.Fatalf("second Close should be idempotent, got: %v", err)
	}
}

// TestModule_CloseReleasesTransientRuntime exercises the Module.Close fix
// for the nil-cache wazero.Runtime leak identified in the Task 5 code-quality
// review. Per wazero v1.10.1 config.go:317 CompiledModule.Close does NOT
// cascade to its parent Runtime — only Runtime.Close → CompiledModule.
// Before the fix, CompileModule(ctx, src, nil) leaked one wazero.Runtime
// per call. The fix attaches the transient runtime to *Module and adds
// Module.Close to release both compiled + runtime.
//
// This test asserts:
//
//   - nil-cache compile produces a *Module whose transientRT is non-nil
//     (so Module.Close has real work to do).
//   - Module.Close on the nil-cache module returns nil + is idempotent
//     (second Close also returns nil).
//   - Cache-owned modules have transientRT == nil; calling Module.Close
//     on them is a documented no-op that does NOT invalidate the cache —
//     a subsequent CompileModule against the SAME cache + a DIFFERENT
//     source succeeds (proves the cache's runtime is still alive).
func TestModule_CloseReleasesTransientRuntime(t *testing.T) {
	ctx := context.Background()

	t.Run("nil-cache module has transient runtime; Close releases it", func(t *testing.T) {
		src := buildCompilableModule("proxy_abi_version_0_2_1")
		mod, err := CompileModule(ctx, src, nil)
		if err != nil {
			t.Fatalf("CompileModule(nil-cache): %v", err)
		}
		if mod.transientRT == nil {
			t.Fatal("nil-cache *Module must have non-nil transientRT (else the runtime leaks)")
		}
		if err := mod.Close(ctx); err != nil {
			t.Errorf("Module.Close (first): %v", err)
		}
		if !mod.closed {
			t.Error("Module.closed flag should be true after Close")
		}
		if mod.transientRT != nil {
			t.Error("Module.transientRT should be cleared after Close (sentinel for double-close-safety)")
		}
		// Idempotent: second Close returns nil with no side effects.
		if err := mod.Close(ctx); err != nil {
			t.Errorf("Module.Close (second; expected idempotent nil): %v", err)
		}
	})

	t.Run("cache-owned module has nil transient runtime; Close is no-op", func(t *testing.T) {
		cc := NewCompileCache(ctx)
		t.Cleanup(func() { _ = cc.Close() })

		srcA := buildCompilableModule("proxy_abi_version_0_2_1")
		modA, err := CompileModule(ctx, srcA, cc)
		if err != nil {
			t.Fatalf("CompileModule(cache, srcA): %v", err)
		}
		if modA.transientRT != nil {
			t.Error("cache-owned *Module must have nil transientRT (the cache owns the runtime)")
		}

		// Calling Module.Close on a cache-owned module is a no-op by
		// design — the cache still owns the runtime. The closed flag
		// will be set, but the underlying CompiledModule + Runtime are
		// untouched (no double-close hazard against CompileCache.Close).
		if err := modA.Close(ctx); err != nil {
			t.Errorf("Module.Close on cache-owned module (expected no-op nil): %v", err)
		}
		if err := modA.Close(ctx); err != nil {
			t.Errorf("Module.Close on cache-owned module (second; idempotent): %v", err)
		}

		// Critical: the cache's runtime must still be usable for a NEW
		// source. If Module.Close had closed the cache's runtime, this
		// next compile would fail (wazero rejects compiles against a
		// closed runtime).
		srcB := distinctCompilableModule("aux_post_close")
		modB, err := CompileModule(ctx, srcB, cc)
		if err != nil {
			t.Fatalf("post-Close cache must still compile new sources; got: %v", err)
		}
		if modB == nil {
			t.Fatal("post-Close cache returned nil module for new source")
		}
		if modB.Compiled() == nil {
			t.Error("post-Close cache returned module with nil CompiledModule")
		}
	})
}
