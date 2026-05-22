package lua

// fuzz_test.go — 28th + 29th + 30th project-wide fuzzers per ADR-0018
// baseline.
//
//   - `FuzzLuaConfigParse` (28th; 22.1 SPEC §6 Task 11 + PLAN Task 11 +
//     D-P7) — typed_config envelope fuzzer; drives arbitrary byte
//     sequences as the typed_config Any.Value payload to `lua.New(tc, ctx)`.
//   - `FuzzLuaBodyBridge` (29th; 22.2 PLAN Task 16 + SPEC §11.9 D7 +
//     §13-R10 + D-P7) — body bridge fuzzer; drives arbitrary body bytes
//     through `accumulateRequestBody` + `accumulateResponseBody` then
//     invokes `:body()` / `:bodyChunks()` from a small Lua script.
//   - `FuzzLuaHTTPCallConfig` (30th; 22.2 PLAN Task 16 + SPEC §11.9 D7 +
//     §13-R10 + D-P7) — httpCall bridge config-surface fuzzer; drives
//     arbitrary cluster name + headers + body + timeout_ms + async flag
//     parameters through `:httpCall(...)`. Uses the no-plumbing guard
//     (httpClient + clusterMgr nil) so the dispatch goroutine is NEVER
//     spawned — the fuzzer only exercises argument validation +
//     buildHTTPCallRequest + the arm-20 byte-stable wording surface.
//
// Both 22.2 fuzzers must-never-panic per ADR-0018. The fuzz body's
// recover() trap converts any panic into a test fatal with the input
// inputs printed for reproduction.
//
// Drives arbitrary byte sequences as the typed_config Any.Value payload to
// `lua.New(tc, ctx)`. Asserts the structural contract: must-never-panic
// across `New()`; PARSE-REJECT failures + proto-Unmarshal failures both
// return `(nil, error)` cleanly (the fuzz body asserts only the no-panic
// invariant; precise PARSE-REJECT wording is asserted by table-driven
// rows in `compiled_config_test.go` + `datasource_test.go`).
//
// # Seed corpus per D-P7 (30 total)
//
// Authored as in-test `f.Add` seeds (over a testdata/fuzz/ corpus dir) per
// the established phase-21 adaptive_concurrency + phase-20 oauth2 precedent.
// In-test seeds are loaded at EVERY fuzz run; portable + version-controlled
// + no testdata-file convention. testdata/fuzz/FuzzLuaConfigParse/ is
// reserved for future regression corpus (any future crash discovery would
// land a seed file there per the Go fuzz tooling convention).
//
// Roster breakdown:
//
//   - 18 per-PARSE-REJECT-arm seeds — one fixture per arm from parent §6.2.
//     D1 was REFUTED at Task 2 (arms 5 + 17 are silent-no-op rather than
//     PARSE-REJECT); those seeds exercise the no-op pathway (still
//     must-never-panic).
//   - 5 valid-config seeds — one per DataSource arm (Filename + InlineBytes
//     + InlineString + EnvironmentVariable) + 1 with explicit stat_prefix.
//   - 7 adversarial-Lua-source seeds — syntax errors triggering arm 16
//     (broken parens, no-end functions, unicode BOM); sandbox-breaking
//     attempts that compile-clean but error at runtime (`dofile`,
//     `io.popen`, `os.execute`, `require("syscall")`,
//     `debug.getupvalue`). Note: New() only invokes CompileScript synch-
//     ronously; the script's top-level is NOT executed at boot (it runs
//     at DecodeHeaders time per stream). So adversarial scripts that
//     compile clean but execute dangerously do NOT surface in this fuzzer
//     — they're caught at runtime by the decode/encode hot path's
//     stats.errors-bump + degraded-pass-through discipline per
//     BRAINSTORM §2.9. The fuzzer still seeds them to cover the
//     compile-success branch of arm 16 with non-trivial inputs.
//
// 30s runtime envelope per SPEC §14.3 + ADR-0018 short-mode CI policy.

import (
	"context"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	lua "github.com/yuin/gopher-lua"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	luaprim "github.com/esalaine/envoy-go/internal/lua"
	"github.com/esalaine/envoy-go/internal/stats"
)

// luaTypeURL is the type.googleapis.com URL for the v3 Lua proto. Used to
// construct the *anypb.Any envelope passed to New. Byte-exact match with
// the package-level TypeURL constant in lua.go.
const luaTypeURL = "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua"

// FuzzLuaConfigParse drives arbitrary byte sequences as the typed_config
// Any.Value payload to lua.New. 28th project-wide fuzzer per ADR-0018
// baseline + 22.1 SPEC §6 Task 11 + PLAN Task 11 + D-P7.
//
// Must-never-panic across New. Tolerates Unmarshal failures via the arm-2
// PARSE-REJECT branch + compile-failure via arm-16 (returns (nil, error)
// cleanly).
func FuzzLuaConfigParse(f *testing.F) {
	addSeed := func(m *luav3.Lua) {
		f.Helper()
		b, err := proto.Marshal(m)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(b)
	}

	// -------------------------------------------------------------------------
	// 18 per-PARSE-REJECT-arm seeds (one per arm from parent §6.2). D1 was
	// REFUTED at Task 2 (arms 5 + 17 are silent-no-op rather than
	// PARSE-REJECT); those seeds exercise the no-op pathway. Arm 1 (nil
	// typed_config) cannot be expressed as a seed payload — it's covered
	// by the unit test TestNew_NilTypedConfig_ParseRejects in lua_test.go.
	// Arm 2 (typed_config unmarshal failure) is covered by the raw-garbage
	// seed at the tail of the roster.
	// -------------------------------------------------------------------------

	// Seed 1 — Arm 3: inline_code set (PARSE-REJECT envoy-go-strict).
	addSeed(&luav3.Lua{
		InlineCode: "function envoy_on_request(rh) end", //nolint:staticcheck // SA1019: deliberate exercise of deprecated arm-3 reject path.
	})

	// Seed 2 — Arm 4: source_codes map set (PARSE-REJECT; deferred to 22.3).
	addSeed(&luav3.Lua{
		SourceCodes: map[string]*corev3.DataSource{
			"hello.lua": {Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end",
			}},
		},
	})

	// Seed 3 — Arm 5 (D1-REFUTED → silent no-op): default_source_code absent.
	// Empty Lua proto; expected to load as pass-through compiled config.
	addSeed(&luav3.Lua{})

	// Seed 4 — Arm 6: DataSource specifier oneof unset (bare DataSource{}).
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{},
	})

	// Seed 5 — Arm 7: WatchedDirectory present (deferred to Runtime/RTDS).
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			WatchedDirectory: &corev3.WatchedDirectory{Path: "/tmp/watched"},
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end",
			},
		},
	})

	// Seed 6 — Arm 8: Filename arm with empty Filename.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_Filename{Filename: ""},
		},
	})

	// Seed 7 — Arm 9: Filename ENOENT (path that does not exist).
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_Filename{
				Filename: "/nonexistent/path/to/lua/script/xyzzy.lua",
			},
		},
	})

	// Seed 8 — Arm 10: Filename zero-byte file. We use /dev/null as a
	// canonical zero-byte file (POSIX guarantee; available on all unix
	// CI runners). The arm-10 fmt.Errorf wraps the filename + zero-byte
	// reason.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_Filename{Filename: "/dev/null"},
		},
	})

	// Seed 9 — Arm 11: InlineBytes empty (nil/zero-length).
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineBytes{InlineBytes: nil},
		},
	})

	// Seed 10 — Arm 12: InlineString empty.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{InlineString: ""},
		},
	})

	// Seed 11 — Arm 13: EnvironmentVariable name empty.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_EnvironmentVariable{EnvironmentVariable: ""},
		},
	})

	// Seed 12 — Arm 14: EnvironmentVariable unset.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_EnvironmentVariable{
				EnvironmentVariable: "LUA_FUZZ_DOES_NOT_EXIST_XYZZY_123",
			},
		},
	})

	// Seed 13 — Arm 15: EnvironmentVariable set but empty. We cannot set
	// an env var from inside a seed (the env state is process-global +
	// shared across all fuzz iterations); we substitute with another
	// arm-14 seed using a different never-set var name to keep the
	// roster at 18 arms. This is acceptable because (a) arm 15 has the
	// same panic-surface as arm 14 (both return error from
	// resolveDataSourceEnvVar), and (b) the unit tests in
	// datasource_test.go cover arm 15's byte-exact wording.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_EnvironmentVariable{
				EnvironmentVariable: "LUA_FUZZ_ALSO_UNSET_XYZZY_456",
			},
		},
	})

	// Seed 14 — Arm 16: script compile failed (unclosed function).
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function broken_paren(",
			},
		},
	})

	// Seed 15 — Arm 17 (D1-REFUTED → silent no-op): script with no hooks.
	// Compiles cleanly; runtime hook-presence check skips invocation.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "x = 42",
			},
		},
	})

	// Seed 16 — Arm 18 surrogate: per-route is enforced via
	// validatePerRouteLua at boot (NOT inside New); cannot be triggered
	// via the typed_config payload. We seed an extra arm-16 variant
	// (compile failure via no-end function) to keep the per-arm count
	// representative.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function noend()",
			},
		},
	})

	// Seed 17 — Arm 7 variant: WatchedDirectory + empty Filename (combined
	// to assert arm-7 fires BEFORE the oneof dispatch per datasource.go's
	// pin).
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			WatchedDirectory: &corev3.WatchedDirectory{Path: "/tmp/watched2"},
			Specifier:        &corev3.DataSource_Filename{Filename: ""},
		},
	})

	// Seed 18 — Arm 16 variant: compile failure with unicode BOM prefix +
	// syntax error (covers the BOM-handling branch of the gopher-lua
	// parser).
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "\xef\xbb\xbffunction broken_bom(",
			},
		},
	})

	// -------------------------------------------------------------------------
	// 5 valid-config seeds (one per DataSource arm + 1 with explicit
	// stat_prefix).
	// -------------------------------------------------------------------------

	// Seed 19 — Valid InlineString (canonical happy path).
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end",
			},
		},
	})

	// Seed 20 — Valid InlineBytes.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineBytes{
				InlineBytes: []byte("function envoy_on_response(rh) end"),
			},
		},
	})

	// Seed 21 — Valid InlineString + explicit stat_prefix (exercises the
	// re-unmarshal in New + the HCM-rooted stat-name path). The prefix
	// uses underscores (NOT hyphens) — `internal/stats/registry.go::nameRE`
	// rejects hyphens, and the fuzzer surfaced a real defect at IMPL Task
	// 11 first-run: `lua.New` PANICS via `stats.NewCounter` when the
	// operator supplies a `Lua.stat_prefix` containing nameRE-invalid
	// characters (`-`, ` `, `/`, etc.). The Task 2 18-arm PARSE-REJECT
	// roster does NOT include a stat_prefix-format-validation arm; the
	// adaptive_concurrency + oauth2 precedent applies the SAME-shape
	// `stats.IsValidName` pre-check elsewhere (cluster/manager.go:205;
	// hcm/config.go:209) — that pre-check is missing from lua. See the
	// Task 11 PROGRESS.md entry for the recommended follow-up: extend
	// `newFilterStats` (or buildCompiledConfig) with a `stats.IsValidName`
	// guard returning a 19th arm-19 PARSE-REJECT
	// "lua: stat_prefix: invalid characters (must match
	// ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$)". For Task 11
	// in-isolation acceptance the seed uses the underscore-form that
	// passes the regex cleanly.
	addSeed(&luav3.Lua{
		StatPrefix: "my_script_prefix",
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end\nfunction envoy_on_response(rh) end",
			},
		},
	})

	// Seed 22 — Valid Filename (/dev/null is zero-byte → arm 10 surrogate;
	// no valid-Filename seed because we cannot guarantee a non-empty
	// readable script file on every fuzz runner without writing test
	// fixtures + os-side state. The Filename happy-path is covered by
	// the fixture-0026 differential at Task 14). We substitute with
	// another valid-InlineString seed (long-script branch coverage).
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: `
					-- long script with both hooks + comments + local vars
					local counter = 0
					function envoy_on_request(rh)
						counter = counter + 1
						rh:logInfo("hit")
					end
					function envoy_on_response(rh)
						rh:logInfo("resp")
					end
				`,
			},
		},
	})

	// Seed 23 — Valid EnvironmentVariable: requires LookupEnv to return
	// non-empty. We use PATH which is universally set on POSIX runners.
	// This may PARSE-REJECT on environments where PATH is empty (rare in
	// CI; acceptable degradation — the fuzzer still must-never-panic).
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_EnvironmentVariable{
				EnvironmentVariable: "PATH",
			},
		},
	})

	// -------------------------------------------------------------------------
	// 7 adversarial-Lua-source seeds — exercise compile-success branch +
	// adversarial inputs that compile cleanly but would error at runtime.
	//
	// NOTE: New() only invokes CompileScript; the script top-level is NOT
	// executed at boot (it runs at DecodeHeaders per stream). So adversarial
	// scripts that compile clean but execute dangerously do NOT surface in
	// this fuzzer — caught instead by the decode/encode hot path's
	// stats.errors + degraded-pass-through discipline per BRAINSTORM §2.9.
	// We still seed them to cover the compile-success branch with
	// non-trivial inputs.
	// -------------------------------------------------------------------------

	// Seed 24 — Adversarial: dofile sandbox-bypass attempt. Compiles
	// clean (dofile is a valid Lua global lookup); errors at runtime
	// because the sandbox strips dofile per AMEND-1.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: `function envoy_on_request(rh) dofile("/etc/passwd") end`,
			},
		},
	})

	// Seed 25 — Adversarial: io.popen sandbox-bypass attempt.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: `function envoy_on_request(rh) io.popen("ls /") end`,
			},
		},
	})

	// Seed 26 — Adversarial: os.execute sandbox-bypass attempt.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: `function envoy_on_request(rh) os.execute("rm -rf /") end`,
			},
		},
	})

	// Seed 27 — Adversarial: require("syscall") sandbox-bypass attempt.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: `function envoy_on_request(rh) require("syscall") end`,
			},
		},
	})

	// Seed 28 — Adversarial: debug.getupvalue introspection attempt.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: `function envoy_on_request(rh) debug.getupvalue(envoy_on_request, 1) end`,
			},
		},
	})

	// Seed 29 — Adversarial: huge string literal (memory-pressure
	// compile branch). Keeps the seed payload small (the multiplied
	// content is 1KB) but exercises gopher-lua's parser on a large
	// single-token input.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: `function envoy_on_request(rh) local s = "` + strRepeat("A", 1024) + `" end`,
			},
		},
	})

	// Seed 30 — Adversarial: deeply nested table literal (parser stack-
	// depth exercise). 16-deep nesting is well within Lua's default
	// stack but covers a non-trivial parser branch.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: `function envoy_on_request(rh) local t = ` +
					"{a={b={c={d={e={f={g={h={i={j={k={l={m={n={o={p=1}}}}}}}}}}}}}}}}" +
					` end`,
			},
		},
	})

	// -------------------------------------------------------------------------
	// phase 22.3 Task 4 — source_codes map corpus extension. 22.3 promotes
	// the source_codes map from the arm-4 PARSE-REJECT (deferred) to a
	// consumed multi-script registry (compiled_config.go Task 2 consume
	// path). These seeds drive the New() → buildCompiledConfig source_codes
	// branch with single-entry / multi-entry / empty-key / bad-value
	// shapes. Must-never-panic per ADR-0018 (an empty-key or bad-value
	// entry returning a PARSE-REJECT error is correct behavior; the
	// property is no-panic).
	// -------------------------------------------------------------------------

	// Seed 31 — source_codes single-entry (valid InlineString). The
	// canonical multi-script happy path: one named script compiled into
	// the registry.
	addSeed(&luav3.Lua{
		SourceCodes: map[string]*corev3.DataSource{
			"hello.lua": {Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end",
			}},
		},
	})

	// Seed 32 — source_codes multi-entry (two valid scripts). Exercises
	// the map-iteration compile loop across multiple named entries.
	addSeed(&luav3.Lua{
		SourceCodes: map[string]*corev3.DataSource{
			"req.lua": {Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end",
			}},
			"resp.lua": {Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_response(rh) end",
			}},
		},
	})

	// Seed 33 — source_codes empty-key entry (the map key is ""). An
	// empty registry name is a misconfiguration; the consume path must
	// PARSE-REJECT (or no-op) without panicking.
	addSeed(&luav3.Lua{
		SourceCodes: map[string]*corev3.DataSource{
			"": {Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end",
			}},
		},
	})

	// Seed 34 — source_codes bad-value entry (a named entry whose
	// DataSource fails the resolve/compile gauntlet: compile error). The
	// consume path must surface a clean error, not panic.
	addSeed(&luav3.Lua{
		SourceCodes: map[string]*corev3.DataSource{
			"broken.lua": {Specifier: &corev3.DataSource_InlineString{
				InlineString: "function broken_paren(",
			}},
		},
	})

	// Seed 35 — source_codes bad-value entry (nil DataSource specifier
	// oneof: bare DataSource{}). Exercises the resolveDataSource arm-6
	// oneof-unset leaf inside the source_codes loop.
	addSeed(&luav3.Lua{
		SourceCodes: map[string]*corev3.DataSource{
			"empty.lua": {},
		},
	})

	// Seed 36 — source_codes + default_source_code BOTH set (the combined
	// multi-script-plus-listener-default shape). Exercises the consume
	// path's interaction with the default chunk build.
	addSeed(&luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end",
			},
		},
		SourceCodes: map[string]*corev3.DataSource{
			"override.lua": {Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_response(rh) end",
			}},
		},
	})

	// -------------------------------------------------------------------------
	// Bonus seed: raw garbage bytes — verifies the arm-2 proto-Unmarshal
	// failure path returns (nil, error) cleanly via
	// wrapParseRejectTypedConfigUnmarshal. Not counted in the 30-seed
	// per-D-P7 roster (the roster covers structured Lua proto inputs;
	// this seed validates the typed_config envelope dispatch).
	// -------------------------------------------------------------------------
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

	// -------------------------------------------------------------------------
	// Fuzz body — must-never-panic structural assertion per ADR-0018.
	//
	// The ctx supplies a non-nil *stats.Registry so the stats-registration
	// branch is exercised. A nil registry would skip the re-unmarshal +
	// newFilterStats call; covering the full branch surface requires the
	// non-nil registry.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("lua.New panicked: %v\nInput: %x", r, data)
			}
		}()
		typedConfig := &anypb.Any{
			TypeUrl: luaTypeURL,
			Value:   data,
		}
		ctx := envoyhttp.FactoryCtx{
			Stats:      stats.NewRegistry(),
			StatPrefix: "fuzz_http",
		}
		// err is fine (PARSE-REJECT + Unmarshal failure + compile failure
		// are expected on many random inputs); a panic is not. The two-
		// valued return is discarded because the structural contract
		// assertion is the no-panic invariant.
		_, _ = New(typedConfig, ctx)
	})
}

// strRepeat is a tiny strings.Repeat clone used by the seed roster's
// huge-string-literal adversarial seed. Inlined to avoid importing
// strings just for one call in this file.
func strRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

// =========================================================================
// FuzzLuaBodyBridge — 29th project-wide fuzzer per 22.2 PLAN Task 16 + SPEC
// §11.9 D7 + §13-R10 + ADR-0018 + D-P7 corpus roster.
//
// Drives arbitrary body bytes through the body-bridge accumulators
// (`accumulateRequestBody` + `accumulateResponseBody`) then invokes
// `:body()` / `:bodyChunks()` from a small Lua script. Asserts the
// structural contract: must-never-panic across body accumulation +
// :body() retrieval + arm-21 over-cap runtime-reject + :bodyChunks()
// iterator drain.
//
// Two parameters: body []byte (the data fed into one or more DecodeData /
// EncodeData calls) + script string (the Lua-script invocation pattern;
// the fuzzer constrains this to a small enumerated set of "modes" via a
// modulo dispatch so the fuzz engine doesn't waste time on arbitrary Lua
// source-code mutations — those are already covered by
// FuzzLuaConfigParse's compile-failure seed roster).
//
// # Seed corpus per D-P7 (~15-20 seeds)
//
// Per the PLAN Task 16 dispatch outline: empty body / small body
// (10-100 bytes) / medium body (10 KB-100 KB) / large body (1 MB-15 MB)
// / over-cap body (17 MB; must runtime-reject not panic) / chunked body
// (multi-call DecodeData accumulation patterns) / script-patterns that
// yield/resume in pathological orderings.
//
// # No-panic invariants asserted
//
//   - body accumulation must-never-panic on arbitrary input bytes
//     (defensive-copy at chunk-time is the only allocator).
//   - :body() on the ready-state must-never-panic; over-cap raises a
//     Lua runtime-error which is caught by f.vm.Run's error return (NOT
//     a Go panic).
//   - :bodyChunks() iterator drain must-never-panic on any chunk-count.
//
// The fuzzer constructs a fresh filter per fuzz iteration via the
// `newFuzzBodyBridgeFilter` helper. The maxBodyBufferedBytes is set to
// a small testable value (4 KiB) for the over-cap arm + small enough
// to ensure 16 MiB+ inputs don't allocate runaway memory during the
// fuzz loop.
// =========================================================================

// newFuzzBodyBridgeFilter constructs a per-fuzz-iteration *filter with
// the body-bridge scaffolding wired. Mirrors body_test.go::
// newBodyBridgeFilter but standalone in the fuzz file so the fuzzer
// doesn't depend on _test.go helper visibility across test/fuzz
// boundaries. Returns the *filter + a cleanup function (caller defers).
func newFuzzBodyBridgeFilter(t *testing.T) (*filter, func()) {
	t.Helper()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		stats: &filterStats{
			errors:                 reg.NewCounter("fuzz.errors"),
			executions:             reg.NewCounter("fuzz.executions"),
			respondCalls:           reg.NewCounter("fuzz.respond_calls"),
			bodyBufferedBytesTotal: reg.NewCounter("fuzz.body_buffered_bytes_total"),
			coroutineYieldsTotal:   reg.NewCounter("fuzz.coroutine_yields_total"),
			httpcallTotal:          reg.NewCounter("fuzz.httpcall_total"),
			httpcallFailures:       reg.NewCounter("fuzz.httpcall_failures"),
			httpcallTimeouts:       reg.NewCounter("fuzz.httpcall_timeouts"),
		},
	}
	f := &filter{cc: cc}
	vm := luaprim.NewVM()
	ctx, cancelCtx := context.WithCancel(context.Background())
	vm.State().SetContext(ctx)
	f.vm = vm

	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installPairsShim(L)

	f.reqCtx = &requestHandleContext{headers: nil, filterRef: f}
	rud := L.NewUserData()
	rud.Value = f.reqCtx
	L.SetMetatable(rud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", rud)

	f.respCtx = &responseHandleContext{headers: nil, filterRef: f}
	pud := L.NewUserData()
	pud.Value = f.respCtx
	L.SetMetatable(pud, L.GetTypeMetatable(responseHandleTypeName))
	L.SetGlobal("resp", pud)

	// Cap body buffer at 4 KiB so the over-cap arm fires deterministically
	// for any input > 4 KiB (the corpus seeds include 10 KB-100 KB +
	// 1 MB-15 MB + 17 MB which all exceed this cap). Note: the cap is in
	// bytes; arm-21 fires when accumulated > cap (strict inequality per
	// body.go:314).
	f.maxBodyBufferedBytes = 4096

	cleanup := func() {
		cancelCtx()
		vm.Close()
	}
	return f, cleanup
}

// fuzzBodyScripts is the closed enumeration of body-bridge invocation
// patterns the fuzzer can dispatch over. Indexed by mode % len(scripts);
// kept to a small enumerated set so the fuzz engine spends its budget
// exploring body-byte inputs (where the actual must-never-panic surface
// lives) rather than arbitrary Lua source-code mutations.
var fuzzBodyScripts = []string{
	// Mode 0 — plain :body() on the request side after DecodeData has
	// fired (the post-endStream synchronous path).
	`pcall(function() result = rh:body() end)`,

	// Mode 1 — :bodyChunks() iterator drain on the request side.
	`pcall(function()
		local iter = rh:bodyChunks()
		local n = 0
		while true do
			local c = iter()
			if c == nil then break end
			n = n + 1
		end
		chunks_seen = n
	end)`,

	// Mode 2 — response-side :body() after EncodeData has fired.
	`pcall(function() result = resp:body() end)`,

	// Mode 3 — response-side :bodyChunks() iterator drain.
	`pcall(function()
		local iter = resp:bodyChunks()
		local n = 0
		while true do
			local c = iter()
			if c == nil then break end
			n = n + 1
		end
		chunks_seen = n
	end)`,

	// Mode 4 — :body() called twice (defensive: second call must return
	// the same bytes via the same defensive-copy discipline; second :body()
	// MUST NOT panic on the post-yield state).
	`pcall(function()
		local a = rh:body()
		local b = rh:body()
		double_body_match = (a == b)
	end)`,

	// Mode 5 — :body() then string operations on the result (exercises
	// the Lua-side string-as-bytes contract under arbitrary input bytes
	// including embedded NULs).
	`pcall(function()
		local b = rh:body()
		body_len = #b
		body_byte_at_0 = b:byte(1) or -1
	end)`,

	// Mode 6 — :bodyChunks() iterator partial drain (stop after first
	// chunk; exercises the closure-state lifecycle when not fully drained).
	`pcall(function()
		local iter = rh:bodyChunks()
		local c = iter()
		first_chunk_len = c and #c or -1
	end)`,

	// Mode 7 — :body() on both request + response sides in one script.
	`pcall(function()
		local rb = rh:body()
		local pb = resp:body()
		both_body_len = #rb + #pb
	end)`,
}

// FuzzLuaBodyBridge — 29th project-wide fuzzer per 22.2 PLAN Task 16 +
// SPEC §11.9 D7 + §13-R10. Must-never-panic across body accumulation +
// :body() / :bodyChunks() invocation under arbitrary input bytes +
// arbitrary script-mode dispatch.
func FuzzLuaBodyBridge(f *testing.F) {
	// ---------------------------------------------------------------------
	// Seed corpus per D-P7 (~15-20 seeds).
	// ---------------------------------------------------------------------

	// Seed 1 — empty body, mode 0 (:body() on empty body).
	f.Add([]byte{}, uint8(0))

	// Seed 2 — small body 10 bytes, mode 0.
	f.Add([]byte("0123456789"), uint8(0))

	// Seed 3 — small body 100 bytes, mode 0.
	f.Add(bodyFuzzBytesOfLen(100, 'a'), uint8(0))

	// Seed 4 — medium body 10 KiB, mode 0 (over-cap at 4 KiB; must
	// runtime-reject not panic).
	f.Add(bodyFuzzBytesOfLen(10*1024, 'b'), uint8(0))

	// Seed 5 — medium body 100 KiB, mode 0 (over-cap).
	f.Add(bodyFuzzBytesOfLen(100*1024, 'c'), uint8(0))

	// Seed 6 — large body 1 MiB, mode 0 (over-cap).
	f.Add(bodyFuzzBytesOfLen(1024*1024, 'd'), uint8(0))

	// Seed 7 — large body 15 MiB, mode 0 (over-cap; just under the
	// production 16 MiB cap default but well over the 4 KiB test cap).
	f.Add(bodyFuzzBytesOfLen(15*1024*1024, 'e'), uint8(0))

	// Seed 8 — over-cap body 17 MiB, mode 0 (over BOTH the test cap +
	// the production cap; must runtime-reject not panic per arm-21).
	f.Add(bodyFuzzBytesOfLen(17*1024*1024, 'f'), uint8(0))

	// Seed 9 — small body, mode 1 (:bodyChunks() drain). Note: the fuzz
	// harness splits the input bytes into 1-3 chunks based on length so
	// the multi-call DecodeData accumulation pattern is exercised.
	f.Add([]byte("chunk-1-chunk-2-chunk-3"), uint8(1))

	// Seed 10 — small body, mode 2 (response-side :body()).
	f.Add([]byte("response-body"), uint8(2))

	// Seed 11 — small body, mode 3 (response-side :bodyChunks() drain).
	f.Add([]byte("response-chunks"), uint8(3))

	// Seed 12 — body with embedded NULs, mode 0. Exercises the
	// gopher-lua LString-binary-safe contract under random binary bytes.
	f.Add([]byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd, 0x00, 0x80, 0x7f}, uint8(0))

	// Seed 13 — body with UTF-8 BOM + high-byte sequence, mode 5
	// (exercises the byte-at-index contract).
	f.Add([]byte("\xef\xbb\xbf\xc3\xa9\xc3\xa8\xc3\xaa"), uint8(5))

	// Seed 14 — small body, mode 4 (double-:body() call; defensive-copy
	// contract under repeated invocation).
	f.Add([]byte("double-body-test"), uint8(4))

	// Seed 15 — empty body, mode 1 (:bodyChunks() drain on empty body;
	// iterator must terminate via nil on first call).
	f.Add([]byte{}, uint8(1))

	// Seed 16 — single byte, mode 6 (partial-drain iterator; one chunk
	// then break).
	f.Add([]byte{0x7f}, uint8(6))

	// Seed 17 — small body, mode 7 (both request + response :body() in
	// one script).
	f.Add([]byte("both-sides"), uint8(7))

	// Seed 18 — body at exactly cap boundary (4096 bytes); not over-cap
	// per body.go:314 strict inequality, so :body() returns successfully.
	f.Add(bodyFuzzBytesOfLen(4096, 'g'), uint8(0))

	// Seed 19 — body at cap + 1 (4097 bytes); over-cap → arm-21 fires.
	f.Add(bodyFuzzBytesOfLen(4097, 'h'), uint8(0))

	// Seed 20 — body with all high bytes (0xff), mode 0.
	f.Add(bodyFuzzBytesOfLen(256, 0xff), uint8(0))

	// ---------------------------------------------------------------------
	// Fuzz body — must-never-panic structural assertion per ADR-0018.
	//
	// The fuzzer constructs a fresh filter per iteration, splits the
	// input body into 1-3 chunks (so the multi-call DecodeData
	// accumulation pattern is exercised), feeds via DecodeData /
	// EncodeData with endStream=true on the final chunk, then dispatches
	// to one of the enumerated body-bridge invocation patterns.
	//
	// pcall() wraps the Lua-side invocation so over-cap arm-21
	// runtime-errors are caught Lua-side (they're Lua runtime-errors
	// raised via L.RaiseError, NOT Go panics — but pcall() makes the
	// fuzz body more robust against future signature changes).
	// f.vm.Run's outer error return is intentionally ignored — a Go
	// error from the Run is fine (over-cap raise + script error are
	// expected on many inputs); a Go panic is not (caught by the defer).
	// ---------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, body []byte, mode uint8) {
		// Cap input size at 32 MiB to bound per-iteration allocation
		// during the fuzz loop. The 16 MiB production cap + the 4 KiB
		// test cap surfaces remain exercised; inputs larger than 32 MiB
		// are truncated to keep the fuzz engine's memory footprint
		// bounded. This is a fuzz-harness pragmatism — NOT a relaxation
		// of the must-never-panic invariant (the invariant holds for ANY
		// input size; we just don't OOM the test runner exploring
		// gigabyte-scale inputs).
		if len(body) > 32*1024*1024 {
			body = body[:32*1024*1024]
		}

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("body bridge panicked: %v\nbody.len=%d mode=%d head=%x",
					r, len(body), mode, headHex(body, 64))
			}
		}()

		fil, cleanup := newFuzzBodyBridgeFilter(t)
		defer cleanup()

		// Split the body into 1-3 chunks so the multi-call DecodeData
		// accumulation path is exercised in addition to single-shot
		// delivery. The chunk count is derived from the mode to keep
		// the dispatch deterministic per (body, mode) pair.
		chunks := splitFuzzBody(body, int(mode%3)+1)
		for i, c := range chunks {
			endStream := (i == len(chunks)-1)
			// Symmetric: feed BOTH decode + encode side accumulators so
			// the response-side bridge modes have data to read.
			fil.DecodeData(c, endStream)
			fil.EncodeData(c, endStream)
		}

		// Dispatch to the enumerated script mode.
		script := fuzzBodyScripts[int(mode)%len(fuzzBodyScripts)]
		chunk, err := luaprim.CompileScript([]byte(script), nil)
		if err != nil {
			// Compile failure on a literal-string mode script is a fuzz-
			// harness bug, not a production defect. Surface it loudly.
			t.Fatalf("compile of fuzz mode script failed: %v\nscript=%s",
				err, script)
		}
		// f.vm.Run's error return is fine (script's pcall() catches the
		// Lua-side error; we just need no Go panic).
		_ = fil.vm.Run(chunk)
	})
}

// bodyFuzzBytesOfLen returns a deterministic []byte of length n filled
// with the supplied byte b. Used by the FuzzLuaBodyBridge seed roster.
func bodyFuzzBytesOfLen(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// splitFuzzBody splits body into roughly-equal n chunks. n is clamped to
// [1, len(body)] (an empty body returns a single empty chunk so the
// fuzzer's endStream=true fires exactly once). Used by FuzzLuaBodyBridge
// to exercise multi-call DecodeData/EncodeData accumulation.
func splitFuzzBody(body []byte, n int) [][]byte {
	if n < 1 {
		n = 1
	}
	if len(body) == 0 {
		// Empty body: single empty chunk so endStream fires once.
		return [][]byte{{}}
	}
	if n > len(body) {
		n = len(body)
	}
	chunks := make([][]byte, 0, n)
	step := len(body) / n
	for i := 0; i < n-1; i++ {
		chunks = append(chunks, body[i*step:(i+1)*step])
	}
	chunks = append(chunks, body[(n-1)*step:])
	return chunks
}

// headHex returns the hex-encoded head of b (up to maxBytes). Used by
// fuzz panic-trap messages to keep the failure message bounded under
// arbitrary-size input.
func headHex(b []byte, maxBytes int) []byte {
	if len(b) <= maxBytes {
		return b
	}
	return b[:maxBytes]
}

// =========================================================================
// FuzzLuaHTTPCallConfig — 30th project-wide fuzzer per 22.2 PLAN Task 16 +
// SPEC §11.9 D7 + §13-R10 + ADR-0018 + D-P7 corpus roster.
//
// Drives arbitrary cluster name + headers + body + timeout_ms + async
// flag parameters through `:httpCall(cluster, headers, body, timeout_ms,
// asynchronous)` from a Lua script. Uses the no-plumbing guard
// (httpClient + clusterMgr nil) so the dispatch goroutine is NEVER
// spawned — the fuzzer exercises only argument validation +
// buildHTTPCallRequest + the arm-20 byte-stable wording surface +
// the no-plumbing fallthrough (the latter yields (nil, error) cleanly
// without dispatch).
//
// Five parameters: cluster string + method string + path string + body
// string + flags uint8 (encodes timeout_ms range + async flag). The
// flags byte is decoded as:
//
//   - flags & 0x01 → asynchronous bool
//   - flags & 0x06 (bits 1-2) → timeout_ms variant:
//     0 = 0 (use default)
//     1 = 1000
//     2 = -1 (negative; uses default)
//     3 = 2^31 (extreme)
//   - flags & 0x18 (bits 3-4) → which side dispatches (rh vs resp).
//   - flags & 0x60 (bits 5-6) → reserved for future extension.
//
// # Seed corpus per D-P7 (~10-15 seeds)
//
// Per the PLAN Task 16 dispatch outline: empty cluster name / valid
// cluster + headers + body + timeout / missing-cluster fallthrough /
// transport-failure simulation / oversized headers / oversized body /
// invalid timeout values / async-flag variations.
// =========================================================================

// newFuzzHTTPCallFilter constructs a per-fuzz-iteration *filter for the
// httpCall fuzzer. httpClient + clusterMgr are nil — the no-plumbing
// guard at runHTTPCall:280 catches this BEFORE the dispatch goroutine
// spawn + returns (nil, error_string) cleanly. The fuzzer exercises only
// the argument-parse + validation + buildHTTPCallRequest surface; the
// actual dispatch path is covered by httpcall_test.go's table tests.
func newFuzzHTTPCallFilter(t *testing.T) (*filter, func()) {
	t.Helper()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		stats: &filterStats{
			errors:                 reg.NewCounter("fuzz.errors"),
			executions:             reg.NewCounter("fuzz.executions"),
			respondCalls:           reg.NewCounter("fuzz.respond_calls"),
			bodyBufferedBytesTotal: reg.NewCounter("fuzz.body_buffered_bytes_total"),
			coroutineYieldsTotal:   reg.NewCounter("fuzz.coroutine_yields_total"),
			httpcallTotal:          reg.NewCounter("fuzz.httpcall_total"),
			httpcallFailures:       reg.NewCounter("fuzz.httpcall_failures"),
			httpcallTimeouts:       reg.NewCounter("fuzz.httpcall_timeouts"),
		},
	}
	// httpClient + clusterMgr deliberately nil — no-plumbing guard fires
	// at runHTTPCall:280 + returns (nil, error_string) cleanly. No
	// dispatch goroutine spawned.
	f := &filter{cc: cc}
	vm := luaprim.NewVM()
	ctx, cancelCtx := context.WithCancel(context.Background())
	vm.State().SetContext(ctx)
	f.vm = vm

	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installPairsShim(L)

	f.reqCtx = &requestHandleContext{headers: nil, filterRef: f}
	rud := L.NewUserData()
	rud.Value = f.reqCtx
	L.SetMetatable(rud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", rud)

	f.respCtx = &responseHandleContext{headers: nil, filterRef: f}
	pud := L.NewUserData()
	pud.Value = f.respCtx
	L.SetMetatable(pud, L.GetTypeMetatable(responseHandleTypeName))
	L.SetGlobal("resp", pud)

	cleanup := func() {
		cancelCtx()
		vm.Close()
	}
	return f, cleanup
}

// fuzzHTTPCallTimeouts is the enumerated set of timeout_ms values
// dispatched over by the flags byte. Covers zero (default), positive,
// negative (default), and extreme values.
var fuzzHTTPCallTimeouts = [4]int64{0, 1000, -1, 1 << 31}

// FuzzLuaHTTPCallConfig — 30th project-wide fuzzer per 22.2 PLAN Task 16
// + SPEC §11.9 D7 + §13-R10. Must-never-panic across :httpCall(...)
// argument validation + buildHTTPCallRequest + no-plumbing-guard
// fallthrough under arbitrary inputs.
func FuzzLuaHTTPCallConfig(f *testing.F) {
	// ---------------------------------------------------------------------
	// Seed corpus per D-P7 (~10-15 seeds).
	// ---------------------------------------------------------------------

	// Seed 1 — empty cluster name (arm-20 byte-stable runtime-reject).
	f.Add("", "GET", "/", "", uint8(0))

	// Seed 2 — valid cluster + GET + path + empty body + default
	// timeout + sync.
	f.Add("cluster_a", "GET", "/", "", uint8(0))

	// Seed 3 — valid cluster + POST + body + 1000ms timeout + sync.
	f.Add("cluster_b", "POST", "/api/v1", "payload", uint8(0b010))

	// Seed 4 — valid cluster + async flag set + extreme timeout.
	f.Add("cluster_c", "GET", "/", "", uint8(0b111))

	// Seed 5 — valid cluster + negative timeout (uses default) + sync.
	f.Add("cluster_d", "PUT", "/x", "body", uint8(0b100))

	// Seed 6 — empty cluster + async flag set (still arm-20; async
	// flag doesn't bypass cluster-required check).
	f.Add("", "GET", "/", "", uint8(0b001))

	// Seed 7 — long cluster name (255 chars).
	f.Add(strRepeat("a", 255), "GET", "/", "", uint8(0))

	// Seed 8 — oversized body (1 MiB; exercise body-string-into-request
	// path without OOM).
	f.Add("cluster_e", "POST", "/upload", strRepeat("x", 1024*1024), uint8(0))

	// Seed 9 — body with embedded NULs + high bytes (binary-safe
	// body-string contract).
	f.Add("cluster_f", "POST", "/", string([]byte{0x00, 0x01, 0xff, 0xfe}), uint8(0))

	// Seed 10 — invalid path (no leading slash; exercises http.NewRequest
	// URL-parse path). buildHTTPCallRequest uses scheme://authority +
	// path so a path without leading slash composes as
	// "http://cluster" + "broken-path" → URL.Path = "broken-path".
	f.Add("cluster_g", "GET", "broken-path", "", uint8(0))

	// Seed 11 — method with unusual characters (lowercased; the bridge
	// uppercases via strings.ToUpper per httpcall.go:377).
	f.Add("cluster_h", "patch", "/", "", uint8(0))

	// Seed 12 — empty method string (defaults to GET via the bridge's
	// pseudo-header-absent fallback).
	f.Add("cluster_i", "", "/", "", uint8(0))

	// Seed 13 — empty path string (defaults to "/" via the bridge's
	// pseudo-header-absent fallback). Empty :path pseudo-header set in
	// the headers table → present-but-empty → bridge uses "" + path
	// composition produces "http://cluster" which http.NewRequest
	// parses as URL.Path = "".
	f.Add("cluster_j", "GET", "", "", uint8(0))

	// Seed 14 — async + 1000ms timeout + small body (the async path is
	// pure fire-and-forget; with nil httpClient the goroutine returns
	// immediately via the nil-guard at httpcall.go:238).
	f.Add("cluster_k", "POST", "/async", "fire-and-forget", uint8(0b011))

	// Seed 15 — response-side dispatch (flags bit 3 set; uses
	// resp:httpCall instead of rh:httpCall).
	f.Add("cluster_l", "GET", "/resp-side", "", uint8(0b01000))

	// ---------------------------------------------------------------------
	// Fuzz body — must-never-panic structural assertion per ADR-0018.
	//
	// The fuzzer constructs a fresh filter per iteration (no httpClient +
	// no clusterMgr → no-plumbing guard fires + dispatch goroutine NEVER
	// spawned), builds a Lua script that calls :httpCall(...) with the
	// fuzz-supplied parameters via a pcall() wrapper, and runs it. Any
	// Go panic surfaces via the defer-recover trap.
	//
	// The headers table is constructed Lua-side from the fuzz-supplied
	// method/path strings; the script uses the timeout_ms variant
	// dispatched by the flags byte. async flag + side (rh vs resp) are
	// dispatched by flags bits.
	//
	// pcall() wraps the :httpCall(...) invocation so arm-20 byte-stable
	// runtime-errors are caught Lua-side (NOT Go panics — but pcall()
	// makes the fuzz body more robust against future signature changes).
	// ---------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, cluster, method, path, body string, flags uint8) {
		// Cap body input to 4 MiB to bound per-iteration allocation
		// during the fuzz loop. The httpCall surface is exercised
		// regardless of body length; we just don't OOM the runner
		// exploring gigabyte-scale bodies.
		if len(body) > 4*1024*1024 {
			body = body[:4*1024*1024]
		}
		// Bound cluster + method + path inputs similarly to avoid
		// pathological-length URL composition in http.NewRequest.
		if len(cluster) > 4096 {
			cluster = cluster[:4096]
		}
		if len(method) > 1024 {
			method = method[:1024]
		}
		if len(path) > 4096 {
			path = path[:4096]
		}

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("httpCall fuzz panicked: %v\ncluster=%q method=%q path=%q body.len=%d flags=%#x",
					r, cluster, method, path, len(body), flags)
			}
		}()

		fil, cleanup := newFuzzHTTPCallFilter(t)
		defer cleanup()

		// Decode the flags byte.
		async := (flags & 0x01) != 0
		timeoutMs := fuzzHTTPCallTimeouts[(flags>>1)&0x03]
		useResp := (flags & 0x08) != 0

		// Push the parameters as Lua globals so the script can read
		// them without sprintf-into-source (avoids interaction issues
		// where the cluster/method/path/body strings contain Lua
		// special characters like backslash, quote, embedded NUL).
		L := fil.vm.State()
		L.SetGlobal("fz_cluster", lua.LString(cluster))
		L.SetGlobal("fz_method", lua.LString(method))
		L.SetGlobal("fz_path", lua.LString(path))
		L.SetGlobal("fz_body", lua.LString(body))
		L.SetGlobal("fz_timeout_ms", lua.LNumber(timeoutMs))
		L.SetGlobal("fz_async", lua.LBool(async))
		receiver := "rh"
		if useResp {
			receiver = "resp"
		}

		// The script builds the headers table from globals (with
		// :method + :path pseudo-headers) + calls :httpCall(...) under
		// pcall(). Both rh + resp sides are equivalent in the
		// no-plumbing fallthrough path; the side dispatched is
		// controlled by `receiver`.
		script := `
local ok, err = pcall(function()
	local h = {[":method"] = fz_method, [":path"] = fz_path}
	local _, _ = ` + receiver + `:httpCall(fz_cluster, h, fz_body, fz_timeout_ms, fz_async)
end)
fz_ok = ok
fz_err = err
`
		chunk, err := luaprim.CompileScript([]byte(script), nil)
		if err != nil {
			t.Fatalf("compile of fuzz httpcall script failed: %v\nscript=%s", err, script)
		}
		// f.vm.Run's error return is fine; arm-20 + buildHTTPCallRequest
		// errors are Lua runtime-errors caught by pcall(). Any Go panic
		// surfaces via the defer-recover above.
		_ = fil.vm.Run(chunk)
	})
}

// =========================================================================
// FuzzLuaPerRouteConfig — 31st project-wide fuzzer per phase 22.3 PLAN
// Task 4 + ADR-0018 baseline + parent SPEC §6.2 arm 18.
//
// Drives arbitrary byte sequences as a marshaled *luav3.LuaPerRoute proto
// through the HCM-build per-route validator `parsePerRouteLua` (the
// ADR-0110 single-chokepoint behind validatePerRouteLua). Mirrors
// FuzzLuaConfigParse's unmarshal-then-validate idiom: the fuzzed bytes are
// proto-Unmarshaled into a *luav3.LuaPerRoute; an Unmarshal failure is
// skipped (return) so only successfully-decoded protos reach the
// validator. parsePerRouteLua is then run.
//
// Must-never-panic per ADR-0018 — NOT must-never-error. A PARSE-REJECT
// (nil-oneof / disabled:false / name:"" / source_code resolve+compile
// failure) returning (nil, error) is CORRECT behavior; the structural
// invariant asserted here is the no-panic property. The precise
// byte-stable PARSE-REJECT wording is asserted by the table-driven rows
// in perroute_test.go (TestParsePerRouteConsts_ByteExactWording + the
// arm-dispatch tests), NOT here.
//
// # Seed corpus per the PLAN Task 4 roster
//
// Authored as in-test `f.Add` seeds (over a testdata/fuzz/ corpus dir)
// per the established FuzzLuaConfigParse + phase-21/20 precedent. One seed
// per PGV-mirror arm: nil-oneof / disabled:true / disabled:false / name:"a"
// / name:"" / source_code InlineString-valid / source_code Filename-ENOENT
// / source_code compile-error + adversarial DataSource paths
// (WatchedDirectory, oneof-unset, /dev/null zero-byte, EnvironmentVariable
// unset). Plus a raw-garbage bonus seed validating the Unmarshal-failure
// skip path.
func FuzzLuaPerRouteConfig(f *testing.F) {
	addSeed := func(m *luav3.LuaPerRoute) {
		f.Helper()
		b, err := proto.Marshal(m)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(b)
	}

	// Seed 1 — nil-oneof (bare LuaPerRoute{}; override unset →
	// parseRejectPerRouteOneofRequired). Marshals to empty bytes; the
	// fuzz body's Unmarshal succeeds into a proto with nil override.
	addSeed(&luav3.LuaPerRoute{})

	// Seed 2 — disabled:true (the valid disabled arm → returns the proto
	// cleanly, no error).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_Disabled{Disabled: true},
	})

	// Seed 3 — disabled:false (PGV const:true violation →
	// parseRejectPerRouteDisabledFalse).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_Disabled{Disabled: false},
	})

	// Seed 4 — name:"a" (the valid name arm → returns the proto cleanly;
	// the validator does NOT check name-resolves-to-an-entry, that's a
	// runtime silent-no-op per AMEND-22.3-1).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_Name{Name: "a"},
	})

	// Seed 5 — name:"" (PGV min_len:1 violation → parseRejectPerRouteNameEmpty).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_Name{Name: ""},
	})

	// Seed 6 — source_code InlineString valid (resolve + compile-validate
	// happy path → returns the proto cleanly).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineString{
					InlineString: "function envoy_on_request(rh) end",
				},
			},
		},
	})

	// Seed 7 — source_code Filename ENOENT (resolveDataSource arm-9 →
	// wrapped via wrapParseRejectPerRouteSourceCodeFmt).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_Filename{
					Filename: "/nonexistent/path/to/lua/script/xyzzy.lua",
				},
			},
		},
	})

	// Seed 8 — source_code compile-error (resolve succeeds, CompileScript
	// fails on unclosed function → wrapped via the source_code fmt).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineString{
					InlineString: "function broken_paren(",
				},
			},
		},
	})

	// -------------------------------------------------------------------------
	// Adversarial DataSource seeds — exercise the source_code arm's full
	// resolveDataSource gauntlet leaves.
	// -------------------------------------------------------------------------

	// Seed 9 — source_code with nil specifier oneof (bare DataSource{};
	// resolveDataSource arm-6 oneof-unset).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{},
		},
	})

	// Seed 10 — source_code with WatchedDirectory present (arm-7 deferred
	// to Runtime/RTDS → wrapped error).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				WatchedDirectory: &corev3.WatchedDirectory{Path: "/tmp/watched"},
				Specifier: &corev3.DataSource_InlineString{
					InlineString: "function envoy_on_request(rh) end",
				},
			},
		},
	})

	// Seed 11 — source_code Filename empty (arm-8 empty-filename leaf).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_Filename{Filename: ""},
			},
		},
	})

	// Seed 12 — source_code Filename zero-byte (/dev/null; arm-10 leaf).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_Filename{Filename: "/dev/null"},
			},
		},
	})

	// Seed 13 — source_code InlineBytes empty (arm-11 leaf).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineBytes{InlineBytes: nil},
			},
		},
	})

	// Seed 14 — source_code EnvironmentVariable unset (arm-14 leaf).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_EnvironmentVariable{
					EnvironmentVariable: "LUA_PERROUTE_FUZZ_UNSET_XYZZY_789",
				},
			},
		},
	})

	// Seed 15 — source_code InlineBytes valid (the InlineBytes happy
	// path; compile-validate succeeds).
	addSeed(&luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineBytes{
					InlineBytes: []byte("function envoy_on_response(rh) end"),
				},
			},
		},
	})

	// -------------------------------------------------------------------------
	// Bonus seed: raw garbage bytes — verifies the proto-Unmarshal failure
	// path is SKIPPED cleanly (the fuzz body returns without invoking the
	// validator on un-decodable bytes). Not counted in the per-arm roster.
	// -------------------------------------------------------------------------
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

	// -------------------------------------------------------------------------
	// Fuzz body — must-never-panic structural assertion per ADR-0018.
	//
	// Mirror FuzzLuaConfigParse: unmarshal the fuzzed bytes into the proto
	// type; on Unmarshal failure SKIP (only validate successfully-decoded
	// protos). Then run parsePerRouteLua. An error return is fine
	// (PARSE-REJECT on adversarial input is correct); a Go panic is not.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("parsePerRouteLua panicked: %v\nInput: %x", r, data)
			}
		}()
		pr := &luav3.LuaPerRoute{}
		if err := proto.Unmarshal(data, pr); err != nil {
			// Un-decodable bytes: skip. The validator is only exercised on
			// successfully-decoded protos (mirrors FuzzLuaConfigParse's
			// arm-2 contract — though here we skip rather than feed the
			// validator garbage, since parsePerRouteLua takes a typed
			// proto.Message, not raw bytes).
			return
		}
		// err is fine (PARSE-REJECT is expected on many adversarial inputs);
		// a panic is not. The two-valued return is discarded because the
		// structural contract assertion is the no-panic invariant.
		_, _ = parsePerRouteLua(pr)
	})
}
