# Phase 51 Implementation Plan — extract `internal/boot.Construct` + `internal/filter/http/builtins` out of `cmd/envoy-go/main.go`, add the public `github.com/esalaine/envoy-go/validate` package (`Bootstrap`/`BootstrapFile`) + a `--mode validate` CLI flag — the FIRST row of the Operational tooling family

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). NOTE the execution lesson (`feedback_subagent_autocommit_claudemd`): the global CLAUDE.md makes dispatched subagents AUTO-COMMIT — do NOT fight it; the controller VERIFIES each commit (correct fileset, real non-vacuous tests via `-v` + read assertions, gates green), cleans stray next-task leak files, re-runs the full suite on the FINAL frozen HEAD, does the deliberate-break verification ITSELF, and squashes + pushes at stage-close.

**Goal:** Make bootstrap config validation reachable from OUTSIDE the binary's normal boot path: extract the tail of `cmd/envoy-go/main.go`'s construction sequence (three filter-type registries + `listener.NewManagerWithBaseDirAndAllowH2C`) into a shared `internal/boot.Construct` function; extract the inline 20-call `httpReg` registration block into a new `internal/filter/http/builtins` package; add a new public package `github.com/esalaine/envoy-go/validate` (`Bootstrap`/`BootstrapFile`); add a `--mode validate` CLI flag to `main.go`. No change to WHAT is validated anywhere — every existing strict-reject/parse-arm across `internal/bootstrap`/`internal/cluster`/`internal/listener` stays byte-for-byte unchanged. **ANCHORS ADR-0268** (its §Decision/§Consequences land atomically here — §Context already drafted at SPEC-51.md §12); ROADMAP row 51 (`bootstrap-validate-mode`) flips **`done`** at this IMPL six-gate (a single unsplit row — ADR-0106 not implicated); the Operational tooling family STAYS OPEN.

**Architecture:** ZERO new framework piece, ZERO new filter/Sink type. THREE new Go packages: `internal/boot` (the `Construct`/`NewTracingProvider` shared construction seam, plus the relocated `maybeWrapLuaScriptLoadError`/`tracesDialerAdapter`/`zipkinTransportAdapter` helpers — all moved verbatim out of `main.go`, none rewritten), `internal/filter/http/builtins` (mirrors `internal/filter/network/builtins`'s `RegisterBuiltins(reg)` convention, but with NO `Deps` struct — SPEC-51 §1.1 AMEND-VALIDATE-HTTPBUILTINS-NO-DEPS: HTTP filter construction defers all boot-singleton injection to per-chain build time, unlike some network filters), and `github.com/esalaine/envoy-go/validate` (the FIRST public, non-`internal/` package in this repo — `Bootstrap(io.Reader, baseDir string, allowH2C bool) error` + `BootstrapFile(path string) error`, both calling `internal/boot.Construct` with throwaway dependencies). `main.go`'s own boot path is REWIRED to call the SAME `internal/boot.Construct`/`internal/filter/http/builtins.RegisterBuiltins`, so it and `validate` can never silently diverge on what "valid" means. A new `--mode validate` flag calls `validate.Bootstrap` directly (not `BootstrapFile`, so `-allow-h2c` composes). ZERO differential surface (no wire behavior) — the regression anchor for the refactor portion (Tasks 2-3) is the FULL EXISTING 96-fixture differential suite plus the FULL EXISTING `cmd/envoy-go` test suite, both of which must stay byte-identical.

**Tech Stack:** Go 1.23.0; pure stdlib (`io`, `os`, `flag`, `path/filepath`, `time`) plus the ALREADY-imported `internal/bootstrap`/`internal/cluster`/`internal/listener`/`internal/drain`/`internal/httpclient`/`internal/grpcclient`/`internal/tracing`/`internal/accesslog`/`internal/stats` packages. **ZERO new go.mod modules.**

## Global Constraints

- **Counts at IMPL exit** (re-verify the baseline at Task 1, do NOT assume): stat surface **1200** (H2 cluster; non-H2 **1196**) → **1200** (+0); fixtures **96** → **96** (+0, no differential surface); fuzzers **52** → **52** (+0 — D-VALIDATE-FUZZER resolved NO new fuzzer at SPEC); BackendKind **38** → **38** (+0); DECISIONS tail **ADR-0267** → **ADR-0268** (next-free ADR-0269); **+3 packages** (`internal/boot`, `internal/filter/http/builtins`, `validate`), **+0 go.mod modules**.
- **Module path:** `github.com/esalaine/envoy-go`. Go **1.23.0**.
- **No new dependency:** `go mod tidy -diff` MUST be EMPTY at every task.
- **Process anchors:** ADR-0044 (ADR §Context landed at SPEC-51.md §12; §Decision+§Consequences land at THIS IMPL) · ADR-0045 (sub-split soft gate — escape-valve UNCONSUMED; re-checked at Task 1) · ADR-0106 (per-leg rows; row 51 flips `done` here, no parent rollup — a single unsplit row) · ADR-0268 (this phase — ANCHORED here).
- **TDD** (`superpowers:test-driven-development`): failing-test → run-fail → minimal-impl → run-pass → commit, every task that has new/changed behavior. Tasks 2-3 (pure extraction, no behavior change) invert this slightly: the "test" IS the existing full suite staying green — see each task's own Step ordering.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): `gofmt -l` (empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`.
- **Worktree hygiene** (`feedback_subagent_worktree_detach`/`_path_targeting`): subagents write to the WORKTREE path; the controller verifies the main checkout stays clean + the branch is undetached after each task.
- **Refactor regression discipline (LOAD-BEARING for Tasks 2-3):** a pure code-movement task is verified by the EXISTING test suite staying green, not by new assertions. Task 2 gates on the FULL `cmd/envoy-go` test suite (`go test ./cmd/envoy-go/... -count=1`) plus a `internal/filter/http/builtins`-local unit test. Task 3 gates on the FULL EXISTING differential suite (`go test ./test/differential/... -run TestDifferential -count=1`, all 96 fixtures) AND the FULL `cmd/envoy-go` test suite — this is the byte-stability proof that moving ~120 lines out of `main.go`'s `func main()` changed NOTHING observable.
- **Boundary reuse (LOAD-BEARING, pinned at SPEC-51.md §3.2, AMEND-VALIDATE-DEPGRAPH):** `internal/boot.Construct` does NOT build the cluster manager — `cluster.NewManagerWithBaseDir` stays a call the CALLER (both `main.go` and `validate.Bootstrap`) makes itself, identically. Do NOT "helpfully" move `cluster.NewManagerWithBaseDir` into `Construct` — it would create a dependency cycle with `sinks`/`tracingProvider`, which the caller must build using the `cm`-derived dialer BEFORE `Construct` is even callable.
- **CLI uses `validate.Bootstrap`, not `BootstrapFile` (SPEC §3.5):** the `--mode validate` handler in `main.go` opens the file itself and calls `validate.Bootstrap(f, filepath.Dir(*cfgPath), *allowH2C)` so `-allow-h2c` composes. Do NOT simplify this to `validate.BootstrapFile(*cfgPath)` — that silently drops `-allow-h2c`.
- **Exit codes (pinned at SPEC):** `0` valid / `1` invalid / `2` usage error (unrecognized `--mode` value, or the pre-existing missing-`-c` case).
- **No new fuzzer** (D-VALIDATE-FUZZER, resolved at SPEC): `internal/boot`/`validate` package tests are ordinary unit tests, not fuzz targets — `Construct`/`Bootstrap` operate on an already-`bootstrap.Load`-validated proto, not a new untrusted-input boundary. Re-verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l` stays **52** at Task 1 AND Task 6 (a regression guard that nothing accidentally added/removed one).

---

## Orientation — read before Task 1 (the zero-context brief)

You are moving ALREADY-WORKING code out of a 517-line `func main()` into three new packages, then adding ~50 lines of genuinely new code (the `validate` package + the `--mode` flag). **No existing strict-reject/parse-arm changes anywhere.** The riskiest part of this phase is Task 3 (the `internal/boot.Construct` extraction) — it touches the SAME `main.go` block that builds the listener manager for every real boot, so its regression gate is the FULL 96-fixture differential suite, not a scoped subset.

**What ALREADY works (do NOT re-derive; verified fresh at PLAN time, 2026-07-05, commit `78dcd772` + the phase-51 SPEC commit `915cddf2` — re-confirm line numbers before editing, files evolve):**

- **`cmd/envoy-go/main.go`** (517 lines) — `func main()` spans lines 63-434. The exact construction sequence (SPEC-51.md §1.1/§3.1 table, re-verified): `flag.String("c", ...)`/`flag.Bool("allow-h2c", ...)` (`:64-67`) → `bootstrap.Load` (`:76`) → `drain.New(30*time.Second)` (`:95`) → `bootstrap.AdminSocket` (`:97`) → `cluster.NewManagerWithBaseDir(bs.Proto, filepath.Dir(*cfgPath), bs.Stats)` (`:103-106`, **STAYS in main.go** — AMEND-VALIDATE-DEPGRAPH) → access-log sinks (`:112-173`, unaffected by this phase) → `dialer := grpcclient.New(cm)` (`:132`) → `httpClient := httpclient.New(...)` (`:146`) → `tracingProvider := tracing.NewExporterProvider(tracesDialerAdapter{dialer}, zipkinTransportAdapter{httpClient, cm}, bs.Stats, 16384, time.Second)` (`:147`, **Task 3 replaces this line** with `boot.NewTracingProvider(dialer, httpClient, cm, bs.Stats)`) → stats sinks (`:191-230`, unaffected) → `httpReg` block (`:263-317`, **Task 2 replaces this block**) → `lfReg`/`netReg`/`lm` block (`:332-356`, **Task 3 replaces this block**) → `admin.New`/`admSrv.Start` (`:370-374`) → `lm.Start(ctx)` (`:379-382`, socket binding happens HERE, unaffected) → `bs.Stats.Freeze()` (`:393`) → `cm.StartHealthChecks`/`cm.StartOutlierDetection` (`:395-396`) → stats-flush goroutine (`:401-408`) → ready sentinels (`:411-414`) → `<-ctx.Done()` (`:416`) → drain sequence (`:417-432`).
- **`main.go`'s `httpReg` block (`:263-317`, ~55 lines)** — `httpReg := filter_http.NewHTTPRegistry()` then 20 `httpReg.Register(TypeURL, New)` calls (router, adaptive_concurrency, admission_control, bandwidthlimit, buffer, compressor, cors, csrf, envoygotest, extauthz, extproc, fault, header_mutation, jwtauthn, localratelimit, lua, oauth2, ratelimit, rbac, wasm) then 5 `RegisterPerRouteValidator` calls (header_mutation, oauth2, lua, ratelimit, wasm) then `httpReg.Freeze()`. **Confirmed (SPEC-51.md §1.1 AMEND-VALIDATE-HTTPBUILTINS-NO-DEPS): every one of these 20 `Register` calls is a bare constructor function reference — NONE captures `cm`/`drainMgr`/`httpClient`/`tracingProvider` in a closure.**
- **`main.go`'s `lfReg`/`netReg`/`lm` block (`:332-356`)**:
```go
lfReg := listenerfilter.NewListenerFilterRegistry()
lfReg.Register(tls_inspector.TypeURL, tls_inspector.New)
lfReg.Freeze()

netReg := network.NewRegistry()
builtins.RegisterBuiltins(netReg, builtins.Deps{
	ClusterManager:   cm,
	StatsRegistry:    bs.Stats,
	AccessLogSinks:   sinks,
	HTTPRegistry:     httpReg,
	DrainManager:     drainMgr,
	HTTPClient:       httpClient,
	TracingExporters: tracingProvider,
})
netReg.Freeze()

lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C, bs.Stats, sinks, httpReg, lfReg, drainMgr, httpClient, netReg)
if err != nil {
	log.Fatalf("listener manager: %v", maybeWrapLuaScriptLoadError(err))
}
```
- **`main.go:436-517`** — `luaCompileErrorSubstring` const (`:443`), `scriptLoadErrorWrapPrefix` const (`:457`), `tracesDialerAdapter` type + `NewTracesClient` method (`:465-469`), `zipkinTransportAdapter` type + `HasCluster`/`Dispatch` methods + the `var _ tracing.ZipkinTransport = zipkinTransportAdapter{}` compile-time assertion (`:471-489`), `maybeWrapLuaScriptLoadError` func (`:509-517`). **ALL of this moves verbatim into `internal/boot`** — doc comments included, unchanged logic.
- **`internal/filter/network/builtins/builtins.go`** (102 lines) — the pattern `internal/filter/http/builtins` mirrors WHERE APPLICABLE: `Deps` struct (`:34-46`, nil-tolerant fields documented), `RegisterBuiltins(reg *network.Registry, deps Deps)` (`:54`, does NOT call `Freeze` — "the caller freezes"). **`internal/filter/http/builtins` needs NO `Deps` struct** (confirmed above) — its `RegisterBuiltins` takes only `reg`.
- **`internal/filter/http/registry.go`** — `HTTPRegistry.Lookup(typeURL string) (HTTPFilterFactory, bool)`, `KnownTypeURLs() []string` (sorted), `PerRouteValidator(filterName string) func(proto.Message) error` (returns `nil` if none registered) — all EXISTING, reused by Task 2's unit test to prove parity.
- **`internal/bootstrap/bootstrap.go`** — `Load(r io.Reader) (*Bootstrap, error)` (`:431`), `result := &Bootstrap{Proto: bs, Stats: stats.NewRegistry()}` (`:458` — confirms `Load` ALWAYS returns a fresh, never-shared stats registry, which is why `validate.Bootstrap` needs no separate throwaway-registry construction of its own).
- **`internal/bootstrap/bootstrap_test.go`** — the `sampleBootstrap` const (`:11-38`, a minimal valid bootstrap: one TCP listener `l_tcp` + one STATIC cluster `c_echo`) that `TestLoad_RejectsDynamicResources`/`TestLoad_RejectsLayeredRuntime` (`:64-95`) both extend via string concatenation. Task 4's reused-fixture tests build on the SAME `sampleBootstrap` shape (copied into `validate_test.go`, since `internal/bootstrap`'s const is unexported to that package).
- **`internal/cluster/manager.go`** — `NewManagerWithBaseDir(bs *bootstrapv3.Bootstrap, baseDir string, registry *stats.Registry) (*Manager, error)` (`:85`); `buildCluster` (`:390`) calls `internaltls.NewUpstreamConfig(ts, baseDir)` (`:512`) when a cluster has a `transport_socket`.
- **`internal/tls/datasource.go:31`** — `os.ReadFile(p)` where `p` is `filepath.Join(baseDir, filename)` for a non-absolute `filename` (`:27-29`) — a RELATIVE filename that doesn't exist under `baseDir` is the cheapest possible way to trigger a construction-time (not parse-time) failure, per D-VALIDATE-TEST-FIXTURES/D-VALIDATE-CLI-TEST-CONFIG-SHAPE.
- **`internal/filter/http/lua/lua.go:72`** — `TypeURL = "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua"`; the config field is `default_source_code` (a `DataSource`, supports `inline_string`); `main.go:443`'s `luaCompileErrorSubstring = "lua: default_source_code: compile:"` is the byte-stable substring a broken Lua script's compile failure surfaces with.
- **`cmd/envoy-go/main_test.go`** (926 lines) — the build-the-real-binary-and-exec convention (`TestEnvoyGoBinary_TwoListenerCutover`, `:32-121`): `exec.Command("go", "build", "-o", bin, ".")` (`:46-51`) then `exec.CommandContext(ctx, bin, "-c", cfgPath)` (`:97-99`) with `cmd.StdoutPipe()`/`cmd.Stderr = os.Stderr`/`cmd.Start()` (`:99-107`).
- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** (5410 lines) — confirmed the file this phase's contract-delta task edits (D-VALIDATE-CONTRACT-FILE resolved: it exists at the repo root under `docs/envoy-go/`). Per-feature sections use a `## <Family> — <feature> (per phase NN ADR-NNNN)` heading near the top of the file (e.g. `:743` `## Stats sinks — the metrics_service gRPC sink`), plus a per-phase count-delta paragraph appended near the counts-history section (e.g. `:4678` the phase-47.1 paragraph, format: **"Phase NN — OLD → NEW (delta) (one-line hook):** ..."`).

---

## D-question resolutions (the SPEC §11 D-VALIDATE-* PLAN pins — settled here)

**D-VALIDATE-CONTRACT-FILE → `docs/envoy-go/BEHAVIOR_CONTRACT.md`** (confirmed to exist at the repo root; SPEC §9's assumption was correct). Task 6 adds a new `## Bootstrap config validation (per phase 51 ADR-0268)` section (mirroring the `## Stats sinks — ...` heading precedent) plus a phase-51 count-delta paragraph in the counts-history section.

**D-VALIDATE-TEST-FIXTURES → 3 NEW construction-boundary tests + 4 REUSED `internal/bootstrap` reject-arm tests + 1 positive-path test, all in a new `validate/validate_test.go`.** Reused verbatim (as table entries, copying the YAML bodies from `internal/bootstrap/bootstrap_test.go` since that package's `sampleBootstrap` const is unexported): the `dynamic_resources`-present reject (mirrors `TestLoad_RejectsDynamicResources`), the `layered_runtime`-present reject (mirrors `TestLoad_RejectsLayeredRuntime`), the YAML-syntax-error reject (mirrors `TestLoad_YAMLSyntaxError`), the empty-document reject (mirrors `TestLoad_EmptyDocument`) — each fed through `validate.Bootstrap` instead of `bootstrap.Load` directly, proving `validate.Bootstrap` doesn't swallow a `Load`-level rejection. NEW construction-boundary-specific (bootstrap-VALID, fails only at cluster/listener construction — exactly the class `bootstrap.Load` alone cannot catch): (1) a cluster with a `transport_socket` referencing a nonexistent relative TLS cert filename (fails inside `cluster.NewManagerWithBaseDir` → `internal/tls.NewUpstreamConfig` → `os.ReadFile`); (2) a listener HTTP filter chain with a Lua filter whose `default_source_code.inline_string` is syntactically invalid Lua (fails inside `listener.NewManagerWithBaseDirAndAllowH2C`, exercising the relocated `maybeWrapLuaScriptLoadError` wrap — assert the `"script load error: "` prefix appears); (3) a listener HTTP filter chain referencing an unregistered/unknown HTTP-filter `typed_config` type_url (fails via the frozen `httpReg`'s type_url resolution). Plus ONE fully-valid bootstrap (`sampleBootstrap`, copied verbatim) asserting `validate.Bootstrap` returns `nil` — the positive-path proof.

**D-VALIDATE-ALIAS-CONVENTION → keep `internal/filter/network/builtins` UNALIASED (its existing `main.go`/`internal/boot` import spelling), alias the new sibling `httpbuiltins`.** No existing codebase convention forces a different scheme (there is no prior instance of two same-named packages imported into the same file before this phase); `httpbuiltins`/`netbuiltins`-style short prefixes matching each package's filter-kind are the clearest, matching this file's existing `filter_http`/`network` aliasing style for OTHER same-name collisions.

**D-VALIDATE-CLI-TEST-CONFIG-SHAPE → the nonexistent-relative-TLS-cert-filename scenario (same shape as D-VALIDATE-TEST-FIXTURES's construction-boundary test #1), reused verbatim for the new `main_test.go` CLI-subprocess test's "bad config" case** — cheapest to construct (no Lua interpreter invoked, no HTTP-filter-registry involved, just a single missing file), deterministic (no timing/networking), and it's a scenario `bootstrap.Load` alone provably cannot catch (proving the CLI test exercises REAL construction depth, not merely re-testing `Load`).

---

## File structure (decomposition locked here)

**Production (NEW packages):**
- `internal/boot/boot.go` — `Construct`, `NewTracingProvider`, `maybeWrapLuaScriptLoadError` + its two consts, `tracesDialerAdapter`, `zipkinTransportAdapter` (all relocated verbatim from `main.go`).
- `internal/filter/http/builtins/builtins.go` — `RegisterBuiltins(reg *filter_http.HTTPRegistry)` (no `Deps`).
- `validate/validate.go` — `Bootstrap(r io.Reader, baseDir string, allowH2C bool) error`, `BootstrapFile(path string) error`.

**Production (modified):**
- `cmd/envoy-go/main.go` — the `httpReg` block (Task 2) and the `lfReg`/`netReg`/`lm`/`tracingProvider`-construction block + the relocated helpers (Task 3) are replaced by calls into the two new internal packages; a new `--mode` flag + validate-mode branch (Task 5).

**Test (created):**
- `internal/filter/http/builtins/builtins_test.go` (Task 2).
- `internal/boot/boot_test.go` (Task 3).
- `validate/validate_test.go` (Task 4).

**Test (modified):**
- `cmd/envoy-go/main_test.go` — ONE new CLI-subprocess test (Task 5).

**Docs (completion task, Task 6):**
- `docs/envoy-go/phases/51-bootstrap-validate-mode/PROGRESS-51.md`, `docs/envoy-go/DECISIONS.md` (ADR-0268 §Decision/§Consequences — ANCHORS the row), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` (**row 51 flips `done`**).

---

## Task 1: Phase scaffolding — PROGRESS-51.md + baselines + the final ADR-0045 split re-check

**Files:**
- Create: `docs/envoy-go/phases/51-bootstrap-validate-mode/PROGRESS-51.md`

- [ ] **Step 1: Record the baseline counts** (verbatim outputs in PROGRESS-51.md):
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/*/ | wc -l                                   # expect 96
grep -rh '^func Fuzz' --include='*.go' . | wc -l                 # expect 52
go mod tidy -diff                                                # expect EMPTY (clean)
ls internal/boot internal/filter/http/builtins validate 2>&1     # expect "No such file or directory" x3 (none exist yet)
grep -c 'httpReg.Register(' cmd/envoy-go/main.go                 # expect 20
grep -c 'RegisterPerRouteValidator(httpReg)' cmd/envoy-go/main.go # expect 5
```
Baseline: stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **96** / fuzzers **52** / BackendKind **38** / DECISIONS tail **ADR-0267** (next-free **ADR-0268**).

- [ ] **Step 2: Write the PROGRESS-51.md scaffold** — a header (phase 51 IMPL, the SPEC-51 reference + "the FIRST Operational-tooling-family row; ANCHORS ADR-0268; row 51 flips `done` at this IMPL" note, the worktree branch), a task checklist mirroring this plan (6 tasks), the baseline block above, the **ADR-0045 split confirmation (NO sub-split — the escape-valve stays UNCONSUMED; ~120 LoC moved verbatim across two package extractions + ~50 LoC of genuinely new `validate` package code + ~20 LoC of CLI flag wiring — comfortably under the gate)**, and the anticipated exit counts: stat **1200** (+0) / fixtures **96** (+0) / fuzzers **52** (+0) / BackendKind **38** (+0) / DECISIONS **ADR-0268** (next-free ADR-0269) / **+3 packages, +0 go.mod modules**.

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/51-bootstrap-validate-mode/PROGRESS-51.md
git commit -m "phase 51 Task 1: PROGRESS scaffold + baselines + ADR-0045 NO-sub-split re-check (bootstrap-validate-mode; ANCHORS ADR-0268; row 51 flips done at this IMPL)"
```

---

## Task 2: `internal/filter/http/builtins` extraction — mirrors the network sibling WHERE applicable (no `Deps` struct) [TDD]

**Files:**
- Create: `internal/filter/http/builtins/builtins.go`, `internal/filter/http/builtins/builtins_test.go`
- Modify: `cmd/envoy-go/main.go`

**Interfaces:**
- Produces: `func RegisterBuiltins(reg *filter_http.HTTPRegistry)` (Task 3's `internal/boot.Construct` consumes it).

- [ ] **Step 1: Write the failing test** in `internal/filter/http/builtins/builtins_test.go`:
```go
package builtins

import (
	"testing"

	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency"
	"github.com/esalaine/envoy-go/internal/filter/http/admission_control"
	"github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit"
	"github.com/esalaine/envoy-go/internal/filter/http/buffer"
	"github.com/esalaine/envoy-go/internal/filter/http/compressor"
	"github.com/esalaine/envoy-go/internal/filter/http/cors"
	"github.com/esalaine/envoy-go/internal/filter/http/csrf"
	"github.com/esalaine/envoy-go/internal/filter/http/envoygotest"
	"github.com/esalaine/envoy-go/internal/filter/http/extauthz"
	"github.com/esalaine/envoy-go/internal/filter/http/extproc"
	"github.com/esalaine/envoy-go/internal/filter/http/fault"
	"github.com/esalaine/envoy-go/internal/filter/http/header_mutation"
	"github.com/esalaine/envoy-go/internal/filter/http/jwtauthn"
	"github.com/esalaine/envoy-go/internal/filter/http/localratelimit"
	"github.com/esalaine/envoy-go/internal/filter/http/lua"
	"github.com/esalaine/envoy-go/internal/filter/http/oauth2"
	"github.com/esalaine/envoy-go/internal/filter/http/ratelimit"
	"github.com/esalaine/envoy-go/internal/filter/http/rbac"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/filter/http/wasm"
)

func TestRegisterBuiltins_AllTwentyTypeURLsResolve(t *testing.T) {
	reg := filter_http.NewHTTPRegistry()
	RegisterBuiltins(reg)

	wantTypeURLs := []string{
		router.TypeURL, adaptive_concurrency.TypeURL, admission_control.TypeURL,
		bandwidthlimit.TypeURL, buffer.TypeURL, compressor.TypeURL, cors.TypeURL,
		csrf.TypeURL, envoygotest.TypeURL, extauthz.TypeURL, extproc.TypeURL,
		fault.TypeURL, header_mutation.TypeURL, jwtauthn.TypeURL,
		localratelimit.TypeURL, lua.TypeURL, oauth2.TypeURL, ratelimit.TypeURL,
		rbac.TypeURL, wasm.TypeURL,
	}
	if got, want := len(reg.KnownTypeURLs()), len(wantTypeURLs); got != want {
		t.Fatalf("KnownTypeURLs(): got %d entries, want %d", got, want)
	}
	for _, tu := range wantTypeURLs {
		if _, ok := reg.Lookup(tu); !ok {
			t.Errorf("Lookup(%q): not registered", tu)
		}
	}
}

func TestRegisterBuiltins_FivePerRouteValidatorsRegistered(t *testing.T) {
	reg := filter_http.NewHTTPRegistry()
	RegisterBuiltins(reg)

	for _, name := range []string{"header_mutation", "oauth2", "lua", "ratelimit", "rbac_per_route_unused_probe"} {
		_ = name // placeholder loop var reused below per filter — see per-filter checks
	}
	if v := reg.PerRouteValidator("envoy.filters.http.header_mutation"); v == nil {
		t.Error("header_mutation: no per-route validator registered")
	}
	if v := reg.PerRouteValidator("envoy.filters.http.oauth2"); v == nil {
		t.Error("oauth2: no per-route validator registered")
	}
	if v := reg.PerRouteValidator("envoy.filters.http.lua"); v == nil {
		t.Error("lua: no per-route validator registered")
	}
	if v := reg.PerRouteValidator("envoy.filters.http.ratelimit"); v == nil {
		t.Error("ratelimit: no per-route validator registered")
	}
	if v := reg.PerRouteValidator("envoy.filters.http.wasm"); v == nil {
		t.Error("wasm: no per-route validator registered")
	}
	// A filter with NO per-route validator (e.g. router) must return nil, not panic.
	if v := reg.PerRouteValidator("envoy.filters.http.router"); v != nil {
		t.Error("router: expected no per-route validator, got one")
	}
}

func TestRegisterBuiltins_DoesNotFreeze(t *testing.T) {
	reg := filter_http.NewHTTPRegistry()
	RegisterBuiltins(reg)
	// RegisterBuiltins must NOT call Freeze — the caller freezes. Prove the
	// registry still accepts a Register call after RegisterBuiltins returns.
	reg.Register("type.googleapis.com/test.PostRegisterProbe", func(any) (filter_http.HTTPFilterFactory, error) {
		return nil, nil
	})
}
```
(NOTE: `RegisterPerRouteValidator` filter names — `header_mutation.RegisterPerRouteValidator(reg)` etc. — register under whatever string key each filter package's own `RegisterPerRouteValidator` function passes internally, typically the filter's own `envoy.filters.http.<name>` canonical name; if a filter's exact key differs, grep `grep -n 'RegisterPerRouteValidator' internal/filter/http/header_mutation/*.go` etc. to confirm before finalizing this test — do not guess at Step 3.)

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/filter/http/builtins/... -count=1` ⇒ FAIL (package doesn't exist yet / `RegisterBuiltins` undefined).

- [ ] **Step 3: Implement.** First, `grep -n 'RegisterPerRouteValidator' internal/filter/http/{header_mutation,oauth2,lua,ratelimit,wasm}/*.go` to confirm each filter's exact per-route-validator registration key (fix Step 1's test if any differ from the `envoy.filters.http.<name>` guess above). Create `internal/filter/http/builtins/builtins.go`:
```go
// Package builtins registers the twenty built-in HTTP filters (router,
// adaptive_concurrency, admission_control, bandwidthlimit, buffer,
// compressor, cors, csrf, envoygotest, extauthz, extproc, fault,
// header_mutation, jwtauthn, localratelimit, lua, oauth2, ratelimit, rbac,
// wasm) plus their five per-route validators (header_mutation, oauth2, lua,
// ratelimit, wasm) into an *filter_http.HTTPRegistry. Unlike
// internal/filter/network/builtins, no Deps struct is needed: HTTP filter
// construction defers all boot-singleton injection (ClusterManager,
// DrainManager, HTTPClient, TracingExporters) to per-chain build time via
// hcm.ListenerCtx/FactoryCtx, not to registration time — none of the 20
// Register calls below captures a boot singleton in a closure (phase 51,
// ADR-0268).
package builtins

import (
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency"
	"github.com/esalaine/envoy-go/internal/filter/http/admission_control"
	"github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit"
	"github.com/esalaine/envoy-go/internal/filter/http/buffer"
	"github.com/esalaine/envoy-go/internal/filter/http/compressor"
	"github.com/esalaine/envoy-go/internal/filter/http/cors"
	"github.com/esalaine/envoy-go/internal/filter/http/csrf"
	"github.com/esalaine/envoy-go/internal/filter/http/envoygotest"
	"github.com/esalaine/envoy-go/internal/filter/http/extauthz"
	"github.com/esalaine/envoy-go/internal/filter/http/extproc"
	"github.com/esalaine/envoy-go/internal/filter/http/fault"
	"github.com/esalaine/envoy-go/internal/filter/http/header_mutation"
	"github.com/esalaine/envoy-go/internal/filter/http/jwtauthn"
	"github.com/esalaine/envoy-go/internal/filter/http/localratelimit"
	"github.com/esalaine/envoy-go/internal/filter/http/lua"
	"github.com/esalaine/envoy-go/internal/filter/http/oauth2"
	"github.com/esalaine/envoy-go/internal/filter/http/ratelimit"
	"github.com/esalaine/envoy-go/internal/filter/http/rbac"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/filter/http/wasm"
)

// RegisterBuiltins registers the twenty built-in HTTP filters and their five
// per-route validators into reg. It mirrors the registration calls in
// cmd/envoy-go/main.go and does NOT Freeze (the caller freezes after any
// additional registration).
func RegisterBuiltins(reg *filter_http.HTTPRegistry) {
	reg.Register(router.TypeURL, router.New)
	reg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)
	reg.Register(admission_control.TypeURL, admission_control.New)
	reg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)
	reg.Register(buffer.TypeURL, buffer.New)
	reg.Register(compressor.TypeURL, compressor.New)
	reg.Register(cors.TypeURL, cors.New)
	reg.Register(csrf.TypeURL, csrf.New)
	reg.Register(envoygotest.TypeURL, envoygotest.New)
	reg.Register(extauthz.TypeURL, extauthz.New)
	reg.Register(extproc.TypeURL, extproc.New)
	reg.Register(fault.TypeURL, fault.New)
	reg.Register(header_mutation.TypeURL, header_mutation.New)
	reg.Register(jwtauthn.TypeURL, jwtauthn.New)
	reg.Register(localratelimit.TypeURL, localratelimit.New)
	reg.Register(lua.TypeURL, lua.New)
	reg.Register(oauth2.TypeURL, oauth2.New)
	reg.Register(ratelimit.TypeURL, ratelimit.New)
	reg.Register(rbac.TypeURL, rbac.New)
	reg.Register(wasm.TypeURL, wasm.New)
	header_mutation.RegisterPerRouteValidator(reg)
	oauth2.RegisterPerRouteValidator(reg)
	lua.RegisterPerRouteValidator(reg)
	ratelimit.RegisterPerRouteValidator(reg)
	wasm.RegisterPerRouteValidator(reg)
}
```

- [ ] **Step 4: Run to verify it passes** — `go test ./internal/filter/http/builtins/... -v -count=1` ⇒ PASS.

- [ ] **Step 5: Rewire `main.go`'s `httpReg` block (`:263-317`) to use the new package.** Replace the ~55-line block with:
```go
httpReg := filter_http.NewHTTPRegistry()
httpbuiltins.RegisterBuiltins(httpReg)
httpReg.Freeze()
```
Add the import `httpbuiltins "github.com/esalaine/envoy-go/internal/filter/http/builtins"` and DELETE all 20 now-unused individual `internal/filter/http/<name>` imports (`router` through `wasm`, per the import block read at PLAN time) — `filter_http` itself STAYS imported (still used for `filter_http.NewHTTPRegistry()`, until Task 3 removes it too). Run `goimports -w cmd/envoy-go/main.go` (or manually delete) to confirm no unused-import compile error.

- [ ] **Step 6: Full regression — the EXISTING `cmd/envoy-go` test suite must stay green (byte-stability proof for this refactor):**
```bash
go build ./... && echo BUILD_OK
go test ./cmd/envoy-go/... -count=1
```
Expected: ALL existing tests PASS unchanged (`TestEnvoyGoBinary_TwoListenerCutover`, `TestEnvoyGoBinary_HCMSmoke`, `TestMain_StatsPrometheusEndpointResponds`, `TestEnvoyGoBinary_TLSInspectorBootWiring`, `TestEnvoyGoBinary_AccessLogSmoke`, `TestEnvoyGoBinary_H2Smoke`, `TestMain_FourNewAdminEndpointsRespond200`) — this proves the 20-filter registration behaves identically after extraction.

- [ ] **Step 7: Per-task gates + commit**
```bash
gofmt -l internal/filter/http/builtins/ cmd/envoy-go/main.go
golangci-lint run ./internal/filter/http/builtins/... ./cmd/envoy-go/...
go vet ./internal/filter/http/builtins/... ./cmd/envoy-go/...
go build ./...
go mod tidy -diff
git add internal/filter/http/builtins/ cmd/envoy-go/main.go
git commit -m "phase 51 Task 2: extract internal/filter/http/builtins (RegisterBuiltins, no Deps struct -- AMEND-VALIDATE-HTTPBUILTINS-NO-DEPS) from main.go's inline httpReg block; ADR-0268"
```

---

## Task 3: `internal/boot.Construct` extraction — the shared no-duplication seam [regression-gated, full 96-fixture differential + full cmd/envoy-go suite]

**Files:**
- Create: `internal/boot/boot.go`, `internal/boot/boot_test.go`
- Modify: `cmd/envoy-go/main.go`

**Interfaces:**
- Consumes: `httpbuiltins.RegisterBuiltins` (Task 2).
- Produces: `func Construct(bs *bootstrap.Bootstrap, cm *cluster.Manager, baseDir string, allowH2C bool, sinks []accesslog.Sink, dm *drain.Manager, httpClient *httpclient.Client, tracingProvider *tracing.ExporterProvider) (*listener.Manager, error)` and `func NewTracingProvider(dialer *grpcclient.Dialer, httpClient *httpclient.Client, cm *cluster.Manager, registry *stats.Registry) *tracing.ExporterProvider` (Task 4's `validate` package AND `main.go`'s real boot path both consume both).

This is the highest-risk task — it moves the code that builds the listener manager for EVERY real boot. Per AMEND-VALIDATE-DEPGRAPH (SPEC-51.md §3.2), `Construct` does NOT build `cm` — `cm` stays a `main.go`-local call (`:103-106`, UNCHANGED, not touched by this task).

- [ ] **Step 1: Create `internal/boot/boot.go`** with the relocated helpers (verbatim from `main.go:436-489`/`:509-517`, doc comments included) plus the two new functions:
```go
// Package boot builds the shared "construct everything, bind nothing" tail
// of envoy-go's boot sequence: the three filter-type registries (HTTP,
// listener, network) and the listener manager. Both cmd/envoy-go/main.go's
// normal boot path and the public github.com/esalaine/envoy-go/validate
// package call Construct, so the two can never silently diverge on what
// "valid" means (phase 51, ADR-0268).
package boot

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/drain"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	httpbuiltins "github.com/esalaine/envoy-go/internal/filter/http/builtins"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/filter/network/builtins"
	"github.com/esalaine/envoy-go/internal/grpcclient"
	"github.com/esalaine/envoy-go/internal/httpclient"
	"github.com/esalaine/envoy-go/internal/listener"
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter"
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector"
	"github.com/esalaine/envoy-go/internal/stats"
	"github.com/esalaine/envoy-go/internal/tracing"
)

// Construct builds the three filter-type registries (HTTP, listener,
// network) and the listener manager for bs, exactly as
// cmd/envoy-go/main.go's normal boot path does — EXCEPT it starts no
// background goroutine and binds no listener socket (both happen later, in
// a separate lm.Start call the caller makes itself). Both main.go's normal
// boot path and the public validate package call this SAME function for
// the registry-and-listener-manager tail of the boot sequence, so the two
// can never silently diverge on what "valid" means.
//
// cm, sinks, dm, httpClient, and tracingProvider are all supplied by the
// caller rather than built here: cm must already exist before a
// grpcclient.Dialer (needed to build sinks and tracingProvider) can be
// built, so Construct cannot own cm construction while also accepting
// sinks/tracingProvider as inputs. The real boot path passes its real,
// already-constructed instances, so the returned *listener.Manager shares
// them with whatever admin/lm.Start/shutdown-drain logic runs afterward.
// The validate package passes throwaway instances (a fresh, never-Frozen
// cm, nil sinks, a throwaway drain.Manager/httpclient.Client/
// tracing.ExporterProvider) and discards the returned *listener.Manager,
// keeping only the error.
func Construct(
	bs *bootstrap.Bootstrap,
	cm *cluster.Manager,
	baseDir string,
	allowH2C bool,
	sinks []accesslog.Sink,
	dm *drain.Manager,
	httpClient *httpclient.Client,
	tracingProvider *tracing.ExporterProvider,
) (*listener.Manager, error) {
	httpReg := filter_http.NewHTTPRegistry()
	httpbuiltins.RegisterBuiltins(httpReg)
	httpReg.Freeze()

	lfReg := listenerfilter.NewListenerFilterRegistry()
	lfReg.Register(tls_inspector.TypeURL, tls_inspector.New)
	lfReg.Freeze()

	netReg := network.NewRegistry()
	builtins.RegisterBuiltins(netReg, builtins.Deps{
		ClusterManager:   cm,
		StatsRegistry:    bs.Stats,
		AccessLogSinks:   sinks,
		HTTPRegistry:     httpReg,
		DrainManager:     dm,
		HTTPClient:       httpClient,
		TracingExporters: tracingProvider,
	})
	netReg.Freeze()

	lm, err := listener.NewManagerWithBaseDirAndAllowH2C(
		bs.Proto, cm, baseDir, allowH2C, bs.Stats, sinks, httpReg, lfReg, dm, httpClient, netReg,
	)
	if err != nil {
		return nil, maybeWrapLuaScriptLoadError(err)
	}
	return lm, nil
}

// NewTracingProvider builds a tracing.ExporterProvider using the standard
// boot-time buffer defaults (16384 bytes / 1s flush) via the
// tracesDialerAdapter/zipkinTransportAdapter bridge. Both the real boot path
// and validate call this so neither duplicates the two adapter types.
func NewTracingProvider(dialer *grpcclient.Dialer, httpClient *httpclient.Client, cm *cluster.Manager, registry *stats.Registry) *tracing.ExporterProvider {
	return tracing.NewExporterProvider(tracesDialerAdapter{dialer}, zipkinTransportAdapter{httpClient, cm}, registry, 16384, time.Second)
}

// luaCompileErrorSubstring is the byte-stable arm-16 wrap prefix emitted by
// internal/filter/http/lua/compiled_config.go::wrapParseRejectScriptCompileFailed
// (`"lua: default_source_code: compile: %w"`). Detecting this substring lets
// the boot-reject sink identify a Lua script-compile failure that surfaced
// through the HCM filter-factory error chain
// (`listener: %q: filter_chains[%d]: hcm: http_filters[%d]: factory: <inner>`).
// Phase 22.1 Task 15 + parent §13-W + 22.1 SPEC §6 Task 15.
const luaCompileErrorSubstring = "lua: default_source_code: compile:"

// scriptLoadErrorWrapPrefix is the literal wording prefix the upstream
// Envoy v1.37.2 lua filter prints to stderr on script-compile failure per
// `source/extensions/filters/common/lua/lua.cc` (parent §11.7.5).
const scriptLoadErrorWrapPrefix = "script load error: "

// tracesDialerAdapter bridges *grpcclient.Dialer to the unexported
// tracing.tracesClientDialer interface (single method NewTracesClient).
// Phase 46.1b (ADR-0260).
type tracesDialerAdapter struct{ d *grpcclient.Dialer }

func (a tracesDialerAdapter) NewTracesClient(clusterName string) (tracing.TracesClient, error) {
	return grpcclient.NewOTLPTracesClient(a.d, clusterName)
}

// zipkinTransportAdapter bridges the shared *httpclient.Client +
// *cluster.Manager to the tracing.ZipkinTransport seam (HasCluster/Dispatch).
// Phase 46.2 (D-TRACE-ZIPKIN-TRANSPORT-WIRING).
type zipkinTransportAdapter struct {
	c  *httpclient.Client
	cm *cluster.Manager
}

func (a zipkinTransportAdapter) HasCluster(name string) bool { _, ok := a.cm.Get(name); return ok }

func (a zipkinTransportAdapter) Dispatch(ctx context.Context, clusterName string, req *http.Request) (*http.Response, error) {
	return a.c.ClusterDispatch(ctx, clusterName, req, a.cm)
}

var _ tracing.ZipkinTransport = zipkinTransportAdapter{}

// maybeWrapLuaScriptLoadError inspects the supplied error for the arm-16
// Lua compile-failure substring. When matched, returns a new error wrapping
// the original with the upstream-parity prefix "script load error: ".
// Otherwise the original error is returned unchanged.
func maybeWrapLuaScriptLoadError(err error) error {
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), luaCompileErrorSubstring) {
		return err
	}
	return fmt.Errorf("%s%w", scriptLoadErrorWrapPrefix, err)
}
```

- [ ] **Step 2: Write `internal/boot/boot_test.go`** — a table-driven test proving `Construct` behaves for (a) a fully valid minimal bootstrap (one TCP listener, one STATIC cluster) ⇒ returns a non-nil `*listener.Manager`, nil error; (b) a Lua-compile-failure bootstrap ⇒ returns a nil manager and an error whose `.Error()` contains `"script load error: "` (proving `maybeWrapLuaScriptLoadError` still fires from inside `Construct`). Build the two bootstraps via `bootstrap.Load(strings.NewReader(yaml))` first (reusing `internal/bootstrap`'s public `Load`), constructing `cm` via `cluster.NewManagerWithBaseDir` and throwaway `dm`/`httpClient`/`tracingProvider` via `drain.New`/`httpclient.New`/`boot.NewTracingProvider` — i.e. exercise `Construct` the SAME way `validate.Bootstrap` (Task 4) will:
```go
package boot

import (
	"strings"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/drain"
	"github.com/esalaine/envoy-go/internal/grpcclient"
	"github.com/esalaine/envoy-go/internal/httpclient"
)

const validYAML = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`

func mustConstruct(t *testing.T, yaml string) (*bootstrap.Bootstrap, error) {
	t.Helper()
	bs, err := bootstrap.Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("bootstrap.Load: %v", err)
	}
	cm, err := cluster.NewManagerWithBaseDir(bs.Proto, t.TempDir(), bs.Stats)
	if err != nil {
		t.Fatalf("cluster.NewManagerWithBaseDir: %v", err)
	}
	dm := drain.New(30 * time.Second)
	httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})
	dialer := grpcclient.New(cm)
	tracingProvider := NewTracingProvider(dialer, httpClient, cm, bs.Stats)
	_, err = Construct(bs, cm, t.TempDir(), false, nil, dm, httpClient, tracingProvider)
	return bs, err
}

func TestConstruct_ValidBootstrap_ReturnsNilError(t *testing.T) {
	if _, err := mustConstruct(t, validYAML); err != nil {
		t.Fatalf("Construct: got error %v, want nil", err)
	}
}

func TestConstruct_LuaCompileFailure_WrapsWithScriptLoadErrorPrefix(t *testing.T) {
	luaYAML := `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: hcm_local
                route_config:
                  name: rc
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_echo }
                http_filters:
                  - name: envoy.filters.http.lua
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua
                      default_source_code:
                        inline_string: "this is not ((( valid lua syntax"
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`
	_, err := mustConstruct(t, luaYAML)
	if err == nil {
		t.Fatal("Construct: want error for invalid Lua syntax, got nil")
	}
	if !strings.Contains(err.Error(), "script load error: ") {
		t.Errorf("error should contain the script-load-error wrap prefix: %q", err.Error())
	}
}
```

- [ ] **Step 3: Run to verify.** `go test ./internal/boot/... -v -count=1` ⇒ PASS (both new tests).

- [ ] **Step 4: Rewire `main.go`.** Replace the `tracingProvider := tracing.NewExporterProvider(...)` line (`:147`) with:
```go
tracingProvider := boot.NewTracingProvider(dialer, httpClient, cm, bs.Stats)
```
Replace the `lfReg`/`netReg`/`lm` block (`:332-356`) with:
```go
lm, err := boot.Construct(bs, cm, filepath.Dir(*cfgPath), *allowH2C, sinks, drainMgr, httpClient, tracingProvider)
if err != nil {
	log.Fatalf("listener manager: %v", err)
}
```
DELETE `main.go:436-517` (the relocated `luaCompileErrorSubstring`/`scriptLoadErrorWrapPrefix`/`tracesDialerAdapter`/`zipkinTransportAdapter`/`maybeWrapLuaScriptLoadError` — now living in `internal/boot`, unused in `main.go`). Add the import `"github.com/esalaine/envoy-go/internal/boot"`. DELETE the now-unused imports: `filter_http` (no longer referenced — `boot.Construct` owns `httpReg` construction), `network "github.com/esalaine/envoy-go/internal/filter/network"`, `"github.com/esalaine/envoy-go/internal/filter/network/builtins"`, `"github.com/esalaine/envoy-go/internal/listener"` (confirm via `grep -n '\blistener\.' cmd/envoy-go/main.go` that no other call site remains — `lm`'s type is now inferred via `:=` from `boot.Construct`'s return, so no explicit `listener.` reference is needed), `"github.com/esalaine/envoy-go/internal/listener/listenerfilter"`, `"github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector"`, `"github.com/esalaine/envoy-go/internal/tracing"` (confirm via `grep -n '\btracing\.' cmd/envoy-go/main.go` no reference remains). `internal/cluster` STAYS (still used for `cluster.NewManagerWithBaseDir` at `:103`). `internal/grpcclient`/`internal/httpclient`/`internal/drain` STAY (all still directly used elsewhere in `main.go`).

- [ ] **Step 5: Full regression — the EXISTING full differential suite AND the full `cmd/envoy-go` suite must stay green (the byte-stability proof this refactor demands):**
```bash
go build ./... && echo BUILD_OK
go test ./cmd/envoy-go/... -count=1
go test ./test/differential/... -run TestDifferential -count=1
```
Expected: ALL 96 differential fixtures PASS (`TestDifferential/<fixture>` for every one — this is the single most important gate in this phase: it proves moving the registry/listener-manager construction out of `func main()` changed NOTHING observable about ANY landed feature). ALL `cmd/envoy-go` tests PASS unchanged.

- [ ] **Step 6: `-race` on the touched packages** (`reference_full_suite_race_after_background_mutator` — `main.go` still starts the stats-flush/health-check/outlier-detection goroutines AFTER `boot.Construct` returns, unaffected by this refactor, but confirm no new race was introduced):
```bash
go test ./internal/boot/... -race -count=1
go test ./cmd/envoy-go/... -race -count=1
```

- [ ] **Step 7: Per-task gates + commit**
```bash
gofmt -l internal/boot/ cmd/envoy-go/main.go
golangci-lint run ./internal/boot/... ./cmd/envoy-go/...
go vet ./internal/boot/... ./cmd/envoy-go/...
go build ./...
go mod tidy -diff
git add internal/boot/ cmd/envoy-go/main.go
git commit -m "phase 51 Task 3: extract internal/boot.Construct + NewTracingProvider (AMEND-VALIDATE-DEPGRAPH -- Construct does NOT build cm, since sinks/tracingProvider already depend on it) -- full 96-fixture differential + cmd/envoy-go suite prove byte-stability; ADR-0268"
```

---

## Task 4: The public `validate` package — `Bootstrap`/`BootstrapFile` [TDD]

**Files:**
- Create: `validate/validate.go`, `validate/validate_test.go`

**Interfaces:**
- Consumes: `boot.Construct`/`boot.NewTracingProvider` (Task 3).
- Produces: `func Bootstrap(r io.Reader, baseDir string, allowH2C bool) error`, `func BootstrapFile(path string) error` (Task 5's `--mode validate` CLI flag consumes `Bootstrap` directly).

- [ ] **Step 1: Write the failing tests** in `validate/validate_test.go`:
```go
package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleValidBootstrap = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`

func TestBootstrap_ValidConfig_ReturnsNil(t *testing.T) {
	if err := Bootstrap(strings.NewReader(sampleValidBootstrap), t.TempDir(), false); err != nil {
		t.Fatalf("Bootstrap: got %v, want nil", err)
	}
}

// --- REUSED from internal/bootstrap/bootstrap_test.go's Load-level reject arms ---

func TestBootstrap_ReusesLoad_RejectsDynamicResources(t *testing.T) {
	yaml := sampleValidBootstrap + `
dynamic_resources:
  ads_config:
    api_type: GRPC
`
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for dynamic_resources, got nil")
	}
	if !strings.Contains(err.Error(), "dynamic_resources") {
		t.Errorf("error should name dynamic_resources: %q", err.Error())
	}
}

func TestBootstrap_ReusesLoad_RejectsLayeredRuntime(t *testing.T) {
	yaml := sampleValidBootstrap + `
layered_runtime:
  layers:
    - name: static_layer
      static_layer: {}
`
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for layered_runtime, got nil")
	}
	if !strings.Contains(err.Error(), "layered_runtime") {
		t.Errorf("error should name layered_runtime: %q", err.Error())
	}
}

func TestBootstrap_ReusesLoad_YAMLSyntaxError(t *testing.T) {
	err := Bootstrap(strings.NewReader("not: valid: yaml: at all: :::"), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want yaml parse error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: yaml parse:") {
		t.Errorf("error prefix: %q", err.Error())
	}
}

func TestBootstrap_ReusesLoad_EmptyDocument(t *testing.T) {
	err := Bootstrap(strings.NewReader(""), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want empty-document error, got nil")
	}
	if !strings.Contains(err.Error(), "empty document") {
		t.Errorf("error: %q", err.Error())
	}
}

// --- NEW: construction-boundary failures bootstrap.Load ALONE cannot catch ---

func TestBootstrap_BadTLSCertPath_FailsAtClusterConstruction(t *testing.T) {
	yaml := `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners: []
  clusters:
    - name: c_tls_upstream
      type: STATIC
      connect_timeout: 1s
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
          common_tls_context:
            tls_certificates:
              - certificate_chain:
                  filename: does-not-exist-cert.pem
                private_key:
                  filename: does-not-exist-key.pem
      load_assignment:
        cluster_name: c_tls_upstream
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for a nonexistent TLS cert file, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist-cert.pem") {
		t.Errorf("error should name the missing file: %q", err.Error())
	}
}

func TestBootstrap_LuaCompileFailure_FailsAtListenerConstruction(t *testing.T) {
	yaml := `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: hcm_local
                route_config:
                  name: rc
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_echo }
                http_filters:
                  - name: envoy.filters.http.lua
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua
                      default_source_code:
                        inline_string: "this is not ((( valid lua syntax"
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for invalid Lua syntax, got nil")
	}
	if !strings.Contains(err.Error(), "script load error: ") {
		t.Errorf("error should contain the script-load-error wrap prefix: %q", err.Error())
	}
}

func TestBootstrap_UnknownHTTPFilterTypeURL_FailsAtListenerConstruction(t *testing.T) {
	yaml := `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: hcm_local
                route_config:
                  name: rc
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_echo }
                http_filters:
                  - name: envoy.filters.http.totally_unregistered_filter
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for an unregistered filter name, got nil")
	}
}

// --- BootstrapFile ---

func TestBootstrapFile_ValidConfig_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.yaml")
	if err := os.WriteFile(path, []byte(sampleValidBootstrap), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := BootstrapFile(path); err != nil {
		t.Fatalf("BootstrapFile: got %v, want nil", err)
	}
}

func TestBootstrapFile_MissingFile_ReturnsError(t *testing.T) {
	if err := BootstrapFile(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
		t.Fatal("BootstrapFile: want error for a missing file, got nil")
	}
}
```
(NOTE: the `TestBootstrap_UnknownHTTPFilterTypeURL...` fixture reuses the `Router` message type_url under an UNREGISTERED filter NAME `envoy.filters.http.totally_unregistered_filter` — per `reference_sibling_reject_test_needs_real_typeurl`, protojson resolves the Any's `@type` against the real proto registry, so the type_url itself must be a REAL, resolvable message; it's the filter NAME dispatch, not the type_url, that's being tested here as unregistered. Confirm this is indeed how `httpReg`'s resolution keys filters — by `name` or by `typed_config`'s type_url — at Step 3 before finalizing; adjust the fixture if the dispatch key differs.)

- [ ] **Step 2: Run to verify they fail** — `go test ./validate/... -count=1` ⇒ FAIL (package doesn't exist yet).

- [ ] **Step 3: Implement** `validate/validate.go`:
```go
// Package validate validates an Envoy v3 Bootstrap config the same way
// envoy-go's normal boot path would construct it: parsing, building the
// cluster manager (including upstream TLS cert resolution), and building
// every listener's full filter chain (routes, HTTP filters, TLS
// certificates, Lua compilation) — without binding any socket, opening any
// access-log file, dialing any stats-sink UDP socket, or starting the admin
// server / any background loop. Motivated by Kubernetes Gateway API
// implementations (e.g. Envoy Gateway) needing to validate envoy-go-
// generated bootstrap config before applying it to a live proxy (phase 51,
// ADR-0268).
package validate

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/esalaine/envoy-go/internal/boot"
	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/drain"
	"github.com/esalaine/envoy-go/internal/grpcclient"
	"github.com/esalaine/envoy-go/internal/httpclient"
)

// Bootstrap validates the config read from r. baseDir resolves relative
// file paths within the config (TLS certs, Lua scripts) the same way
// cmd/envoy-go/main.go's own filepath.Dir(cfgPath) does. allowH2C mirrors
// main.go's -allow-h2c test-only flag (permits HCM codec_type=HTTP2 on
// plaintext listeners). Returns nil if the configuration is valid, or a
// descriptive error otherwise — the first error encountered; envoy-go's
// validation is fail-fast throughout, not multi-diagnostic.
func Bootstrap(r io.Reader, baseDir string, allowH2C bool) error {
	bs, err := bootstrap.Load(r)
	if err != nil {
		return err
	}
	cm, err := cluster.NewManagerWithBaseDir(bs.Proto, baseDir, bs.Stats)
	if err != nil {
		return err
	}
	dm := drain.New(30 * time.Second)
	httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})
	dialer := grpcclient.New(cm)
	tracingProvider := boot.NewTracingProvider(dialer, httpClient, cm, bs.Stats)
	_, err = boot.Construct(bs, cm, baseDir, allowH2C, nil, dm, httpClient, tracingProvider)
	return err
}

// BootstrapFile opens path and validates it (with allowH2C false — the
// -allow-h2c flag is test-only, not a production Gateway API concern),
// using its directory as baseDir — mirroring cmd/envoy-go/main.go's own
// cfgPath/filepath.Dir(cfgPath) pairing.
func BootstrapFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return Bootstrap(f, filepath.Dir(path), false)
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./validate/... -v -count=1` ⇒ PASS (all 10 tests). If `TestBootstrap_UnknownHTTPFilterTypeURL...` fails because the dispatch key is actually the type_url (not the `name` field), fix the fixture per Step 1's inline note and re-run.

- [ ] **Step 5: `-race`** — `go test ./validate/... -race -count=1` ⇒ PASS.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l validate/
golangci-lint run ./validate/...
go vet ./validate/...
go build ./...
go mod tidy -diff
git add validate/
git commit -m "phase 51 Task 4: add the public validate package (Bootstrap/BootstrapFile) -- the FIRST non-internal/non-cmd/non-test package in this repo; ADR-0268"
```

---

## Task 5: `--mode validate` CLI flag [TDD, subprocess test]

**Files:**
- Modify: `cmd/envoy-go/main.go`, `cmd/envoy-go/main_test.go`

**Interfaces:**
- Consumes: `validate.Bootstrap` (Task 4, called directly — NOT `BootstrapFile`, so `-allow-h2c` composes).

- [ ] **Step 1: Write the failing test** in `main_test.go` (following the file's established build-and-exec convention, `TestEnvoyGoBinary_TwoListenerCutover:32-121`):
```go
func TestEnvoyGoBinary_ModeValidate(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "envoy-go")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	goodCfg := filepath.Join(t.TempDir(), "good.yaml")
	goodYAML := `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`
	if err := os.WriteFile(goodCfg, []byte(goodYAML), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	badCfg := filepath.Join(t.TempDir(), "bad.yaml")
	badYAML := `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners: []
  clusters:
    - name: c_tls_upstream
      type: STATIC
      connect_timeout: 1s
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
          common_tls_context:
            tls_certificates:
              - certificate_chain:
                  filename: does-not-exist-cert.pem
                private_key:
                  filename: does-not-exist-key.pem
      load_assignment:
        cluster_name: c_tls_upstream
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`
	if err := os.WriteFile(badCfg, []byte(badYAML), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// (a) Good config: exit 0, stdout contains "configuration OK".
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-c", goodCfg, "--mode", "validate")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("good config: --mode validate failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "configuration OK") {
		t.Errorf("good config: stdout = %q, want it to contain %q", out, "configuration OK")
	}

	// (b) Bad config: exit 1, stderr contains a recognizable substring.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	cmd2 := exec.CommandContext(ctx2, bin, "-c", badCfg, "--mode", "validate")
	out2, err2 := cmd2.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err2, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("bad config: got err=%v, want *exec.ExitError with exit code 1 (out=%s)", err2, out2)
	}
	if !strings.Contains(string(out2), "does-not-exist-cert.pem") {
		t.Errorf("bad config: output = %q, want it to name the missing file", out2)
	}

	// (c) Unknown --mode value: exit 2 (usage error).
	ctx3, cancel3 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel3()
	cmd3 := exec.CommandContext(ctx3, bin, "-c", goodCfg, "--mode", "bogus")
	out3, err3 := cmd3.CombinedOutput()
	var exitErr3 *exec.ExitError
	if !errors.As(err3, &exitErr3) || exitErr3.ExitCode() != 2 {
		t.Fatalf("unknown --mode: got err=%v, want *exec.ExitError with exit code 2 (out=%s)", err3, out3)
	}
}
```
Add `"errors"` to `main_test.go`'s import block if not already present (`grep -n '"errors"' cmd/envoy-go/main_test.go` to confirm).

- [ ] **Step 2: Run to verify it fails** — `go test ./cmd/envoy-go/... -run TestEnvoyGoBinary_ModeValidate -count=1` ⇒ FAIL (`--mode` flag doesn't exist yet — `flag.Parse` will error on the unrecognized flag, or the binary boots normally and never exits early).

- [ ] **Step 3: Implement.** In `main.go`, add the `mode` flag alongside `cfgPath`/`allowH2C` (`:64-71`):
```go
cfgPath := flag.String("c", "", "path to envoy-go.yaml (Envoy v3 Bootstrap)")
allowH2C := flag.Bool("allow-h2c", false,
	"test-only; not for production — permits HCM codec_type=HTTP2 on plaintext listeners for h2spec conformance only")
mode := flag.String("mode", "", `operation mode: empty (default) boots normally; "validate" validates the config named by -c and exits without booting, mirroring upstream Envoy's --mode validate`)
flag.Parse()
if *mode != "" && *mode != "validate" {
	fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml> [--mode validate] [--allow-h2c]")
	os.Exit(2)
}
if *cfgPath == "" {
	fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml> [--mode validate] [--allow-h2c]")
	os.Exit(2)
}
if *mode == "validate" {
	f, err := os.Open(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = validate.Bootstrap(f, filepath.Dir(*cfgPath), *allowH2C)
	_ = f.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("configuration OK")
	os.Exit(0)
}
```
Add the import `"github.com/esalaine/envoy-go/validate"`.

- [ ] **Step 4: Run to verify it passes** — `go test ./cmd/envoy-go/... -run TestEnvoyGoBinary_ModeValidate -v -count=1` ⇒ PASS (all three sub-assertions: good config exit 0, bad config exit 1 + stderr substring, unknown `--mode` exit 2).

- [ ] **Step 5: Full regression — confirm the pre-existing (absent-`--mode`) boot path is BYTE-IDENTICAL:**
```bash
go test ./cmd/envoy-go/... -count=1
```
Expected: ALL existing tests (which never pass `--mode`) PASS unchanged — proving the new flag is purely additive.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l cmd/envoy-go/
golangci-lint run ./cmd/envoy-go/...
go vet ./cmd/envoy-go/...
go build ./...
go mod tidy -diff
git add cmd/envoy-go/main.go cmd/envoy-go/main_test.go
git commit -m "phase 51 Task 5: --mode validate CLI flag (0 valid / 1 invalid / 2 usage error) -- calls validate.Bootstrap directly (not BootstrapFile) so -allow-h2c composes; ADR-0268"
```

---

## Task 6: ADR-0268 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + PROGRESS close + the fuzzer-count reconcile [six-gate]

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/51-bootstrap-validate-mode/PROGRESS-51.md`, `next-prompt.txt`

- [ ] **Step 1: ADR-0268 §Decision/§Consequences** — append to the EXISTING `## ADR-0268 — ...` entry in `docs/envoy-go/DECISIONS.md` (its §Context was drafted at SPEC-51.md §12 — locate via `grep -n 'ADR-0268' docs/envoy-go/DECISIONS.md`; if the §Context draft was NOT already copied into DECISIONS.md at SPEC time — confirm via that grep, per this project's OWN precedent the ADR body normally lands ENTIRELY at IMPL, with only the SPEC.md file itself holding the draft — then add the FULL entry now, copying §Context from `SPEC-51.md` §12 verbatim plus new §Decision/§Consequences sections). §Decision: summarize the exact shipped shape (the `Construct`/`NewTracingProvider` signatures, the `internal/filter/http/builtins` no-`Deps` design, `validate.Bootstrap`/`BootstrapFile`, the `--mode validate` flag + 0/1/2 exit codes). §Consequences: no change to what's validated; the Operational tooling family opens and stays open; the FULL differential suite is the byte-stability regression anchor and passed unchanged; ZERO new go.mod modules.

- [ ] **Step 2: BEHAVIOR_CONTRACT.md** — add a new `## Bootstrap config validation (per phase 51 ADR-0268)` section (mirroring the `## Stats sinks — ...` heading precedent, `:743`) describing the `--mode validate` CLI flag + the `validate` package's public API and validation depth (full construction, no bind/sinks/background-loops). Append a phase-51 count-delta paragraph to the counts-history section (mirroring the phase-47.1 paragraph's format, `:4678`): **"Phase 51 — 1200 → 1200 (+0, UNCHANGED)** (bootstrap-validate-mode — the FIRST Operational-tooling-family row opens; `internal/boot.Construct` + `internal/filter/http/builtins` extraction + the public `validate` package + `--mode validate` CLI flag) adds ZERO new stat shapes (a pure refactor + new entry-point, no new filter/sink/registration call site). Fixtures **96 → 96** (+0 — no differential surface; the FULL EXISTING 96-fixture suite is the regression anchor for the `internal/boot`/`internal/filter/http/builtins` extraction, proven byte-identical). Fuzzers **52 → 52** (+0 — D-VALIDATE-FUZZER resolved no new fuzzer warranted). BackendKind **38 → 38** (+0). **THREE new Go packages** (`internal/boot`, `internal/filter/http/builtins`, `github.com/esalaine/envoy-go/validate` — the FIRST public, non-`internal/` package in this repo) + **ZERO new go.mod modules**. Records **ADR-0268**. **ROADMAP row 51 (`bootstrap-validate-mode`) FLIPS `done`** (the sole leg — ADR-0106; NO parent rollup). The Operational tooling family STAYS OPEN."

- [ ] **Step 3: Final six-gate — run the FULL project verification suite on the final frozen HEAD:**
```bash
go build ./... && echo BUILD_OK
gofmt -l . | grep -v '^vendor/' ; echo "gofmt: $?"    # expect empty listing
golangci-lint run ./...
go vet ./...
go test ./... -count=1
go test ./test/differential/... -run TestDifferential -race -count=1
go mod tidy -diff                                     # expect EMPTY
grep -rh '^func Fuzz' --include='*.go' . | wc -l       # expect 52 (UNCHANGED — reconcile confirms no drift)
ls -d test/fixtures/*/ | wc -l                         # expect 96 (UNCHANGED)
```
All must be green/matching before proceeding.

- [ ] **Step 4: STATE.md** — roll the active-phase header to `phase 51 (bootstrap-validate-mode) IMPL done` (lifecycle-state 3 → 4, TERMINAL for this phase), recording: the final six-gate result, the exit counts (1200/96/52/38/ADR-0268, next-free ADR-0269), the THREE new packages, and `Next → a new phase pick (row 51 done; no phase 52 chartered yet)`.

- [ ] **Step 5: ROADMAP.md** — flip row 51 to `done`; update the "Operational tooling family" section prose to note the row is `done` and the family stays open pending a future pick (xDS-sourced dry-validation, an admin-exposed live-reload-and-validate endpoint, an RTDS/SDS validate companion — unchanged deferred list from the BRAINSTORM/SPEC).

- [ ] **Step 6: PROGRESS-51.md** — mark all 6 tasks `[x]`, record the final six-gate output, the ANCHORS ADR-0268 note, and `Status: IMPL DONE (6/6)`.

- [ ] **Step 7: `next-prompt.txt`** — roll forward: row 51 is `done`, NO phase 52 chartered yet ⇒ per the router's OWN termination-sentinel rule, this is the "none chartered, awaiting a human pick" state (NOT all-work-complete, since a human still needs to pick the next feature) — do NOT create the `stop` sentinel; instead leave `next-prompt.txt` awaiting a human pick, mirroring the `3d10670d` precedent ("next-prompt.txt: re-open the loop — phase 50 IMPL landed, row 50 done, NO phase chartered; awaiting a human pick").

- [ ] **Step 8: Commit (controller squash-and-land, per `feedback_execution_style`/`feedback_push_to_origin` — NOT a subagent push)**
```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md docs/envoy-go/phases/51-bootstrap-validate-mode/PROGRESS-51.md next-prompt.txt
git commit -m "phase 51 (bootstrap-validate-mode) IMPL: internal/boot.Construct + internal/filter/http/builtins + public validate package + --mode validate CLI flag -- ANCHORS ADR-0268, FLIPS ROW 51 done"
```

---

## Self-review checklist (run before dispatching Task 1)

- **Spec coverage:** SPEC-51.md §3.2 (Construct signature) → Task 3. §3.3 (httpbuiltins, no Deps) → Task 2. §3.4 (validate package) → Task 4. §3.5 (CLI flag, 0/1/2 exit codes) → Task 5. §8.1 (unit test envelope) → Task 4. §8.2 (CLI subprocess test) → Task 5. §12 (ADR-0268 context) → Task 6 Step 1. All five D-VALIDATE-* + both AMEND findings are threaded through the relevant task's Interfaces/Step content, not merely restated.
- **Placeholder scan:** every Step above contains complete, runnable code or exact shell commands — no "TODO"/"handle appropriately"/"similar to Task N" language.
- **Type consistency:** `Construct`'s signature (`bs *bootstrap.Bootstrap, cm *cluster.Manager, baseDir string, allowH2C bool, sinks []accesslog.Sink, dm *drain.Manager, httpClient *httpclient.Client, tracingProvider *tracing.ExporterProvider) (*listener.Manager, error)` is IDENTICAL across Task 3 (definition), Task 3's own test, and Task 4 (`validate.Bootstrap`'s call site) — verified consistent. `NewTracingProvider`'s signature is identical across Task 3 (definition) and Task 4 (call site).
