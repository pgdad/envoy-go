# Phase 51 Brainstorm — bootstrap config validation as a public, importable library + a `--mode validate` CLI flag (the FIRST row of a NEW "Operational tooling" family; a Go equivalent of upstream Envoy's `--mode validate`)

> **Lifecycle stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only, direct-on-master (the phase-44 through 50 family-row BRAINSTORM precedent). Row 51 registers `in-progress` AT this BRAINSTORM commit (the ROADMAP §Schema invariant — NOT pre-populated). This is also the FIRST row of a brand-new "Operational tooling" family — no prior family row precedent to extend.
>
> **The pick is already made** (bootstrap validation usable from outside the binary's normal boot path — a human decision, motivated by Kubernetes Gateway API implementations such as Envoy Gateway needing to validate envoy-go-generated bootstrap config before applying it to a live proxy, without needing a full running instance to do so). This BRAINSTORM settles the SCOPE, not the "which feature" question.
>
> **User dialogue (4 questions, 2026-07-05):**
> - **Q1 — validation depth → FULL CONSTRUCTION.** `bootstrap.Load` + `cluster.NewManagerWithBaseDir` + full listener-manager construction (filter chains, TLS certs, Lua compilation, HCM route tables) — everything the normal boot path does EXCEPT binding a socket or starting the admin server / background loops. Rejected the cheaper "parse-only" alternative (wrapping `bootstrap.Load` alone) because it would miss the error class Gateway API controllers actually care about: bad TLS cert refs, malformed filter wiring, broken Lua scripts, bad routes — all of which only surface during cluster/listener construction, not at the proto-parse layer.
> - **Q2 — sink side effects → SKIP BOTH.** Access-log file opens and stats-sink UDP dials (statsd/dog_statsd) are excluded from the validate path entirely. Neither has real diagnostic value in a dry run (a UDP dial never verifies reachability; a file open would create/truncate a real file as an unwanted side effect of a "just checking" operation), and `internal/filter/network/builtins.Deps.AccessLogSinks` is already documented nil-tolerant, so passing an empty sink slice is a supported, safe input — not a workaround.
> - **Q3 — public package API shape → `validate.Bootstrap(io.Reader, baseDir string, allowH2C bool) error` + `validate.BootstrapFile(path string) error`.** A single error return (fail-fast), mirroring how EVERY existing internal validation path in this project already behaves (first bad thing wins — `parseDogStatsdSinkConfig`, `parseStatsdSinkConfig`, the whole `internal/bootstrap` parse-arm family). Rejected a "return multiple diagnostics" shape: `internal/bootstrap`/`internal/cluster`/`internal/listener` are ALL fail-fast internally today; making the NEW public wrapper collect multiple errors would require restructuring long-landed internal validation paths this phase has no reason to touch.
> - **Q4 — CLI integration → `--mode validate` flag on `cmd/envoy-go/main.go`,** mirroring upstream Envoy's own flag verbatim (`envoy --mode validate`). `envoy-go -c bootstrap.yaml --mode validate` calls the new public package and exits 0 ("configuration OK") or 1 (error to stderr) instead of booting; the default/absent `--mode` is byte-identical to today's behavior.

---

## 1. Mission and scope confirmation (51 — the first row of a new "Operational tooling" family)

### 1.1 What phase 51 delivers as a self-contained whole

A way to validate an Envoy v3 Bootstrap config the SAME way envoy-go's normal boot path would construct it — catching the same class of errors (bad TLS cert paths, malformed filter chains, broken Lua scripts, unresolvable cluster references, every existing strict-reject arm across `internal/bootstrap`) — WITHOUT booting a proxy: no socket bind, no admin server, no background goroutines (health checks, outlier detection, stats flush ticker). Usable two ways: (a) as an importable Go library (`github.com/esalaine/envoy-go/validate`, a NEW top-level PUBLIC package — the first non-`internal/`, non-`cmd/`, non-`test/` package in this repo) by an external Go module such as Envoy Gateway's own controller code; (b) as a CLI flag (`envoy-go -c <config> --mode validate`) for any non-Go caller (shell scripts, CI, `kubectl`-adjacent tooling).

### 1.2 What phase 51 does NOT deliver (forward to §8)

Any change to what IS validated — every existing strict-reject/parse-arm in `internal/bootstrap`, `internal/cluster`, `internal/listener` stays byte-for-byte unchanged; this phase only makes that EXISTING validation logic reachable from a new entry point. No dry-run of access-log file writability or stats-sink UDP reachability (§Q2 — explicitly out of scope, not merely deferred: neither has diagnostic value). No structured/multi-error diagnostics (§Q3). No xDS/dynamic-config validation (this project's xDS family is unrelated — this phase is scoped to the STATIC bootstrap file only, the same surface `bootstrap.Load` already covers). No admin-API-exposed "reload and validate" endpoint (a DIFFERENT feature, not requested).

### 1.3 Phase-done as the FIRST row of a NEW "Operational tooling" family (family opens, stays open)

None of the ten pre-seeded `§9` feature families (HTTP filters, network filters, load balancing, upstream robustness, HTTP/3+QUIC, gRPC, xDS/dynamic config, Observability, runtime+hot-restart, WASM host) fit this: it is not a wire-protocol feature, it adds no differential surface, and it touches no filter/LB/xDS code path. `docs/envoy-go/ROADMAP.md` gains a NEW family heading, **"Operational tooling family,"** opened at this phase. Deferred candidates for the same family (not chartered, surfaced only as ideas for a future pick — see §8): a `--mode check-config`-equivalent that also dry-validates xDS-sourced (not just static-file) config; an admin-exposed live-reload-and-validate endpoint; a `--mode validate` companion for RTDS/SDS-sourced dynamic secrets. NO parent rollup applies (ADR-0106 is about split-phase legs within one row; this is a single unsplit row that also happens to open a new family — the two are independent).

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW (escape-valve unconsumed)

The bulk of the work is a REFACTOR (extracting existing `main.go` logic into two new internal packages) plus a thin new public package and one CLI flag — no new wire behavior, no new filter, no new config surface to parse. Anticipated LoC: the `internal/filter/http/builtins` extraction (~120–150 LoC moved, not net-new — mechanical `Register` calls +5 per-route-validator calls, currently inlined in `main.go`), the `internal/boot.Construct` extraction (~100–150 LoC moved/adapted from `main.go`'s existing construction sequence), the new `validate` package (~40–60 LoC — two thin wrapper functions + doc comments), the `--mode` flag plumbing in `main.go` (~20–30 LoC). Comfortably under the ADR-0045 gate as a single flat row; the 51.1/51.2 escape-valve stays UNCONSUMED unless SPEC-time inspection of `main.go`'s exact construction sequence surfaces unexpected coupling (e.g., a dependency on `drainMgr`/`httpClient`/`tracingProvider` inside listener construction that isn't cleanly nil-safe or cheaply-throwaway-constructible — SPEC pins this, see §10).

### 1.5 Seed-stub alignment + package placement

THREE new packages, ZERO new go.mod modules:
- `internal/boot` (new) — the shared "construct everything, bind nothing" function, called by BOTH `cmd/envoy-go/main.go`'s normal boot path and the new `validate` package.
- `internal/filter/http/builtins` (new) — mirrors the ALREADY-LANDED `internal/filter/network/builtins` package's shape (a `Deps` struct + a `RegisterBuiltins(reg, deps)` function), extracting the `httpReg` registration block currently inlined in `main.go`.
- `github.com/esalaine/envoy-go/validate` (new, PUBLIC — no `internal/` prefix, the FIRST such package in this repo) — the two-function library surface an external module actually imports.

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for this row (unlike the phase-11 local_ratelimit precedent noted in project memory — that note is unrelated to this phase).

### 1.7 Phase 51's relationship to the existing seams (a construction-path refactor over ALREADY-LANDED boot logic)

REUSES: `internal/bootstrap.Load` (unchanged — the parse + strict-semantic-validation layer this whole phase sits on top of); `internal/cluster.NewManagerWithBaseDir` (unchanged); `internal/listener.NewManagerWithBaseDirAndAllowH2C` (unchanged — already separates "construct" from "bind," which is the load-bearing seam this phase exploits); `internal/filter/network/builtins.RegisterBuiltins` (unchanged — the EXISTING pattern this phase's NEW `internal/filter/http/builtins` package mirrors); `internal/stats.NewRegistry`, `internal/drain.New`, `internal/httpclient.New` (all already cheap/side-effect-free at construction time — reused as throwaway instances, never Frozen/exposed/started). NEW: `internal/boot.Construct` (a shared function, not a new abstraction layer — a straight-line extraction of `main.go`'s existing construction sequence); `internal/filter/http/builtins.RegisterBuiltins` (mirrors the network sibling); `validate.Bootstrap`/`validate.BootstrapFile` (the public wrapper); `--mode` flag on `main.go`.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: opens the Operational tooling family with bootstrap validation *(Q0 → phase 51 row registered)*

A human picked this feature directly (not via the ROADMAP-loop-reopen "cheapest deferred candidate" pattern the Observability family rows have used — this is a genuinely NEW family, motivated by an external consumer's need: a Kubernetes Gateway API controller wanting to validate generated envoy-go config before applying it).

### 2.2 Validation depth: full construction, stop before bind/start *(Q1)*

Exploits an ALREADY-EXISTING seam in `cmd/envoy-go/main.go`'s boot sequence: `listener.NewManagerWithBaseDirAndAllowH2C(...)` fully constructs every filter chain — HCM route tables, all HTTP/network/listener filters, TLS certificate loading, Lua script compilation — and returns a ready `*listener.Manager`, WITHOUT calling `net.Listen` anywhere. The actual socket binds happen later, inside a SEPARATE `lm.Start(ctx)` call (per-listener `net.Listen("tcp", rt.addr)` at `internal/listener/manager.go:822`), and inside `admSrv.Start()` (the admin HTTP listener). Validate mode therefore runs:
1. `bootstrap.Load` (parse + strict semantic checks — unchanged, the existing foundation).
2. `cluster.NewManagerWithBaseDir` (builds the cluster manager, resolves TLS certs from the base directory).
3. Build `httpReg`/`lfReg`/`netReg` (the three filter-type registries — SAME registration calls `main.go` already makes, now shared via `internal/filter/http/builtins` + the already-shared `internal/filter/network/builtins`).
4. `listener.NewManagerWithBaseDirAndAllowH2C` (builds every filter chain — the single most error-catching step).
5. STOP. No `admin.New`/`admSrv.Start`, no `lm.Start`, no `net.Listen` anywhere, no background goroutine.

This is the closest achievable match to upstream Envoy's own `--mode validate` semantics (construct fully, never bind) without inventing new machinery — the "construct vs. bind" split already exists in the landed code, this phase just makes it independently callable.

### 2.3 Sink side effects: excluded from the validate path *(Q2)*

Between cluster construction and listener construction, the normal boot path ALSO opens access-log files (`accesslog.NewAsyncFileSink`, real file I/O) and dials UDP sockets for statsd/dog_statsd stats sinks (`statssink.NewStatsdSink`/`NewDogStatsdSink`, real `net.DialUDP` calls). Validate mode passes an EMPTY `[]accesslog.Sink` into `internal/filter/network/builtins.Deps.AccessLogSinks` instead of opening real files — confirmed safe by that struct's own doc comment ("Nil-tolerant where the underlying adapter/constructor is... accessLogSinks"). Stats-sink construction is skipped entirely (validate mode never touches `bs.StatsSinkConfigs`/`StatsdSinkConfigs`/`DogStatsdSinkConfigs` at all — there is no equivalent "empty stats sink" to construct, and skipping them entirely has no effect on filter-chain construction, which does not depend on stats sinks). Neither omission weakens the validation: a UDP dial never proves reachability (UDP is connectionless — Go's `net.DialUDP` succeeds even for an address nothing is listening on), and a real file open's only failure mode (a bad path/permissions) is exactly the kind of environment-dependent side effect a "dry run, safe to run anywhere including CI" validator should NOT have.

### 2.4 Public package API: single fail-fast error, `io.Reader`-based *(Q3)*

```go
package validate // github.com/esalaine/envoy-go/validate

// Bootstrap validates an Envoy v3 Bootstrap config the same way envoy-go's
// normal boot path would construct it — parsing, building the cluster
// manager, and building every listener's full filter chain (routes, HTTP
// filters, TLS certificates, Lua compilation) — without binding any socket
// or starting the admin server / background loops. baseDir resolves
// relative file paths within the config (TLS certs, Lua scripts) the same
// way cmd/envoy-go/main.go's own filepath.Dir(cfgPath) does. Returns nil if
// the configuration is valid, or a descriptive error otherwise (the first
// error encountered — envoy-go's validation is fail-fast throughout, not
// multi-diagnostic).
func Bootstrap(r io.Reader, baseDir string, allowH2C bool) error

// BootstrapFile opens path and validates it, using its directory as
// baseDir — mirroring cmd/envoy-go/main.go's own cfgPath/filepath.Dir(cfgPath)
// pairing.
func BootstrapFile(path string) error
```

Mirrors `bootstrap.Load(io.Reader)`'s own shape so a caller that already has config in memory (e.g., a Gateway controller that just rendered a bootstrap YAML template) can validate it directly without writing a temp file first. `BootstrapFile` is the convenience wrapper for the common CLI case.

### 2.5 CLI integration: `--mode validate`, mirroring upstream Envoy *(Q4)*

```
$ envoy-go -c bootstrap.yaml --mode validate
configuration OK
$ echo $?
0

$ envoy-go -c broken.yaml --mode validate
load config: listener[0]: filter_chains[0]: ...
$ echo $?
1
```

`flag.String("mode", "", ...)` on `main.go`; an absent or empty `--mode` preserves EXACTLY today's behavior (boot normally) — this is an ADDITIVE flag, not a replacement of the existing `-c` contract. SPEC pins the exact accepted `--mode` value set (anticipated: just `validate`, matching upstream Envoy's mode enum subset envoy-go actually needs — `serve`/default is implicit, not a named value, since introducing a required `--mode serve` would be a breaking CLI change to every existing caller).

### 2.6 The `internal/boot.Construct` extraction — the shared no-duplication seam

```go
package boot // internal/boot

// Construct builds a cluster manager and a listener manager for the given
// bootstrap config exactly as cmd/envoy-go/main.go's normal boot path does,
// EXCEPT it opens no access-log files, dials no stats-sink sockets, binds no
// listener socket, and starts no background goroutine (health checks,
// outlier detection, stats flush). Both cmd/envoy-go/main.go's own boot path
// and the public validate package call this SAME function, so the two can
// never silently diverge on what "valid" means.
func Construct(r io.Reader, baseDir string, allowH2C bool) (*bootstrap.Bootstrap, *cluster.Manager, *listener.Manager, error)
```

`main.go`'s normal boot path calls `boot.Construct`, then proceeds with what it ALREADY does today with a real (non-throwaway) registry/sinks/etc. — opening real access-log files and stats sinks, then `admin.New`/`admSrv.Start`/`lm.Start`. This is the SPEC's central design question (§10): exactly how much of `main.go`'s current per-boot state (the real `bs.Stats` registry vs. `Construct`'s own throwaway one; how sinks get threaded back in after `Construct` returns for the REAL boot path, since `Construct` builds `netReg`/`httpReg` with an EMPTY sink list even for the real boot) gets resolved. The validate package's OWN call to `boot.Construct` uses everything as throwaway (a fresh `stats.NewRegistry()`, `drain.New(...)`, `httpclient.New(...)` — all cheap, no goroutines/sockets at construction time) and discards the returned `*cluster.Manager`/`*listener.Manager`, checking only the error.

### 2.7 The `internal/filter/http/builtins` extraction — mirroring the landed network sibling

Currently `main.go` builds `httpReg` via ~25 individual `httpReg.Register(TypeURL, New)` calls plus 5 `RegisterPerRouteValidator` calls, inline, before `httpReg.Freeze()`. This is extracted into a new package mirroring `internal/filter/network/builtins`'s EXISTING shape (a `Deps` struct + a `RegisterBuiltins(reg, deps)` function that does NOT freeze — the caller freezes, per that package's own established convention) — so `internal/boot.Construct` and `main.go` share ONE registration list instead of risking two copies that silently drift as new HTTP filters are added in future phases.

### 2.8 Error message stability — reuse existing wording verbatim, no new wrapping

The errors `validate.Bootstrap` surfaces are the SAME errors `bootstrap.Load`/`cluster.NewManagerWithBaseDir`/`listener.NewManagerWithBaseDirAndAllowH2C` already produce today (unchanged call sites, just a new caller) — no new error-wrapping layer, no new prefix, no attempt to make errors "prettier" for a validate-mode audience. This keeps `--mode validate`'s stderr output byte-identical to what the SAME bad config would print via `log.Fatalf` during a normal boot attempt (minus the `log.Fatalf`-added timestamp prefix) — a deliberate consistency choice, not an oversight.

### 2.9 No differential surface — this phase has none

Unlike every other landed phase, this feature has NO wire behavior to compare against a live reference Envoy — `--mode validate`/`validate.Bootstrap` never talks to a network peer, never produces observable proxy behavior. The differential harness (`test/differential/`) is NOT extended this phase; testing is unit tests (`internal/boot`, `validate`) + one CLI-subprocess test in the EXISTING `cmd/envoy-go/main_test.go` (which already builds-and-execs the real binary as its established convention).

---

## 3. Framework-survey result — a construction-path refactor + one thin public wrapper; ZERO new go.mod modules

### 3.1 Framework: no new framework piece

No new `Sink`, no new filter type, no new registry kind. `internal/boot` is a straight-line extraction of an EXISTING sequence, not a new abstraction.

### 3.2 NEW packages: THREE — `internal/boot`, `internal/filter/http/builtins`, `github.com/esalaine/envoy-go/validate` (the first public one in this repo).

### 3.3 go.mod modules: anticipated NONE. Pure refactor + `io`/`flag`-standard-library CLI plumbing. `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES

`bootstrap.Load`, `cluster.NewManagerWithBaseDir`, `listener.NewManagerWithBaseDirAndAllowH2C` (all unchanged); `internal/filter/network/builtins.RegisterBuiltins` (the pattern this phase's new HTTP sibling mirrors); `internal/stats.NewRegistry`/`internal/drain.New`/`internal/httpclient.New` (all already cheap/inert at construction — reused as throwaway instances); `cmd/envoy-go/main_test.go`'s existing build-and-exec-the-binary subprocess-test convention.

---

## 4. Applicability — a NEW operational entry point, not a bootstrap-config-surface addition

Unlike every prior Observability/LB/filter-family row, this phase adds NO new bootstrap YAML field, NO new proto message, NO new `stats_sinks[]`/filter/LB-policy TypeURL. It adds a new WAY TO INVOKE existing, already-landed validation logic — a Go library entry point and a CLI flag — over the EXACT SAME `-c <bootstrap.yaml>` config surface every other phase already targets.

---

## 5. Stat surface hypothesis — N/A this phase (validate mode uses a throwaway, never-exposed registry)

`internal/boot.Construct`'s stats registry (real boot path: the process's actual `bs.Stats`; validate path: a fresh throwaway `stats.NewRegistry()`) is NEVER Frozen, NEVER Walked, NEVER exposed via `/stats` — a one-shot construction-then-discard object for the validate path. Anticipated stat surface UNCHANGED at **1200** (this phase touches no stat-registration call site's SHAPE, only how the construction sequence is invoked).

---

## 6. Differential fixture envelope — N/A this phase (no wire behavior, no live reference Envoy comparison)

No new differential fixture. Testing is unit-level (`internal/boot`/`validate` package tests covering a battery of valid + deliberately-broken bootstrap configs, reusing EXISTING strict-reject fixtures scattered through `internal/bootstrap`/`internal/cluster`/`internal/listener` tests where practical) plus ONE new CLI-subprocess test added to the EXISTING `cmd/envoy-go/main_test.go` (build the real binary, run `--mode validate` against a good config and a bad config, assert exit code + stderr/stdout). Fixtures **96 → 96** (unchanged). No new BackendKind (tail stays **38** — no differential fixture is created). No new fuzzer anticipated (D-VALIDATE-FUZZER, SPEC pins — see §10; the construction path this phase reuses is ALREADY exercised by every existing bootstrap-parse fuzzer, since `validate.Bootstrap` calls the SAME `bootstrap.Load` those fuzzers already target).

---

## 7. Anticipated ADRs — 1 at the phase-51 IMPL: ADR-0268 (the validate-mode construction-path extraction + public package)

ADR-0268 (the `internal/boot`/`internal/filter/http/builtins` extraction + the public `validate` package + the `--mode validate` CLI flag; §Context drafted at the SPEC, §Decision/§Consequences landed in-place at the IMPL per ADR-0044). NO seam ADR beyond this one — the extraction reuses `internal/filter/network/builtins`'s ALREADY-ACCEPTED pattern (no new architectural precedent, just its HTTP-registry sibling). next-free after 51: ADR-0269.

---

## 8. Deferred items

- A `--mode`-equivalent that also dry-validates xDS-sourced (dynamic, not static-file) config — orthogonal to this phase, which is scoped to the static bootstrap file `bootstrap.Load` already parses.
- An admin-API-exposed live-reload-and-validate endpoint (a genuinely different feature: this phase is a one-shot CLI/library call, not a running-server capability).
- A `--mode validate` companion for RTDS/SDS-sourced dynamic secrets (out of scope — envoy-go's RTDS support is itself deferred per the `§9` Runtime + hot restart family).
- Structured/multi-error diagnostics (§2.4) — would require restructuring long-landed fail-fast internal validation paths; no consumer need identified this phase.
- Any change to WHAT is validated (every existing strict-reject/parse-arm across `internal/bootstrap`/`internal/cluster`/`internal/listener` stays byte-for-byte unchanged) — this phase only adds a new way to REACH that existing logic.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup

None — this is the FIRST row of a brand-new family; there is no prior "Operational tooling" deferred-items list to pick up from. (Unrelated to this phase: the Observability family's own deferred list — `graphite`/OTLP-metrics sinks, the plain-statsd `tcp_cluster_name` transport, tracing extras, the tap filter — remains open and unaffected by this phase.)

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (code-reading pins against the CURRENT `main.go`/`internal/listener`/`internal/cluster` — no live Docker probe needed, since this phase has no wire behavior)

- **D-VALIDATE-CONSTRUCT-BOUNDARY (LOAD-BEARING):** trace `cmd/envoy-go/main.go`'s EXACT current construction sequence line-by-line (fresh at SPEC time — line numbers drift) to confirm the "everything up to but not including `admin.New`/`lm.Start`" boundary is genuinely clean — i.e., that NOTHING between `bootstrap.Load` and `listener.NewManagerWithBaseDirAndAllowH2C`'s return actually binds a socket, dials a real (non-UDP-connectionless) connection, or starts a goroutine, beyond the two already-identified sink-construction side effects (§2.3). If a THIRD hidden side effect is found, SPEC decides whether to exclude it too (matching the sink precedent) or whether it's cheap/safe enough to include.
- **D-VALIDATE-BOOT-REUSE:** exactly how `main.go`'s OWN normal-boot call site adapts to call `internal/boot.Construct` — does it pass its REAL `bs.Stats`/sinks/drainMgr/httpClient/tracingProvider into `Construct` (requiring `Construct`'s signature to accept them as parameters rather than always building throwaway ones internally), or does `main.go` keep building `httpReg`/`lfReg`/`netReg` itself and only `validate` calls a DIFFERENT throwaway-everything entry point? (The BRAINSTORM's §2.6 sketch assumes the former — a single `Construct` function parameterized enough for both callers — but the exact parameter list is a SPEC-time design task, not assumed here.)
- **D-VALIDATE-MODE-VALUES:** the exact accepted `--mode` string set (anticipated: bare `validate` only — no `--mode serve`/default value, since that would be a breaking change to the existing `-c`-only invocation every current caller uses).
- **D-VALIDATE-FUZZER:** does `internal/boot.Construct`/`validate.Bootstrap` warrant its own dedicated fuzzer, or does reuse of the EXISTING `internal/bootstrap` fuzzer family suffice (since `Construct` calls the SAME `bootstrap.Load` those fuzzers already target, and the construction steps AFTER that call operate on an ALREADY-validated `*bootstrapv3.Bootstrap` proto, not raw untrusted bytes)? Anticipated: no new fuzzer (§6).
- **D-VALIDATE-EXIT-CODES:** confirm the exact exit-code contract (`0` = valid, `1` = invalid — matching upstream Envoy's own `--mode validate` convention) and whether a THIRD code is warranted for "flag usage error" (e.g., `--mode validate` without `-c`) — likely reusing the EXISTING `os.Exit(2)` usage-error convention already in `main.go`.

---

## 11. Prior-phase lessons applied

- `feedback_execution_style` / `feedback_git_worktrees` / `feedback_subagents_no_push` / `feedback_subagent_autocommit_claudemd` / `feedback_pertask_gofmt_lint` — subagent-driven IMPL in a fresh worktree; controller squashes + pushes at stage-close. Applies at IMPL despite this phase having no differential surface — the refactor + new-package + CLI-flag work still benefits from the same task-by-task TDD discipline.
- `reference_full_suite_race_after_background_mutator` — worth a full-package `-race` check on `cmd/envoy-go` and any package touching the shared `internal/boot.Construct`, since `main.go`'s NORMAL boot path still starts background goroutines (stats flusher, health checks) that this phase's refactor must not accidentally destabilize.
- No differential-specific lessons apply (`reference_differential_*`) — this phase creates no fixture.
- The general project doctrine `D-3.4` (context isolation — every artifact readable with zero prior context) applies as always; this BRAINSTORM was written from a live conversation, not from disk-only context, so the SPEC stage must independently re-verify every file/line citation here against HEAD before relying on it (per the project's own standing "re-verify, do not assume" discipline observed throughout prior phases' PLAN/IMPL stages).

---

## 12. Section closeout

- **Subject:** bootstrap config validation reachable from OUTSIDE the binary's normal boot path — a public importable Go package (`github.com/esalaine/envoy-go/validate`) plus a `--mode validate` CLI flag, motivated by Kubernetes Gateway API implementations (e.g. Envoy Gateway) needing to validate envoy-go-generated bootstrap config before applying it live.
- **Q1 validation depth:** FULL CONSTRUCTION (`bootstrap.Load` + cluster manager + listener manager/filter-chain construction) — stop before `admin.Start`/`lm.Start`/any `net.Listen`.
- **Q2 sink side effects:** SKIP BOTH access-log file opens and stats-sink UDP dials — neither has diagnostic value in a dry run, and empty sinks are an already-documented-safe input.
- **Q3 public API shape:** `validate.Bootstrap(io.Reader, baseDir string, allowH2C bool) error` + `validate.BootstrapFile(path string) error` — single fail-fast error, matching this project's existing internal validation style throughout.
- **Q4 CLI integration:** `--mode validate` flag on `cmd/envoy-go/main.go`, mirroring upstream Envoy's own flag; absent/empty `--mode` is byte-identical to today's behavior.
- **Scope:** extract `internal/boot.Construct` (the shared "construct everything, bind nothing" function) + extract `internal/filter/http/builtins` (mirroring the landed `internal/filter/network/builtins` pattern) + the new public `validate` package (two thin wrapper functions) + the `--mode` CLI flag. NO change to what is validated — every existing strict-reject/parse-arm stays byte-for-byte unchanged.
- **Anticipated counts:** stat **1200** (+0) / fixtures **96** (+0 — no differential surface) / fuzzers **52** (anticipated unchanged, D-VALIDATE-FUZZER pins) / BackendKind **38** (+0) / DECISIONS **ADR-0268** (next-free ADR-0269); THREE new packages (`internal/boot`, `internal/filter/http/builtins`, `validate` — the first public one), ZERO new go.mod modules.
- **Load-bearing SPEC questions:** D-VALIDATE-CONSTRUCT-BOUNDARY (confirm no hidden third side effect between parse and full construction) + D-VALIDATE-BOOT-REUSE (the exact `Construct` signature/parameterization letting BOTH callers share it) + D-VALIDATE-MODE-VALUES + D-VALIDATE-FUZZER + D-VALIDATE-EXIT-CODES.
- **Row 51** registers `in-progress` at this BRAINSTORM commit; flips `done` at the phase-51 IMPL six-gate (NO parent rollup — ADR-0106 is not implicated, this is a single unsplit row). The NEW "Operational tooling" family STAYS OPEN (deferred candidates: xDS-sourced dry-validation, an admin-exposed live-reload-and-validate endpoint, an RTDS/SDS validate companion).
- **Next → the phase-51 SPEC** (`SPEC-51.md` — resolve the §10 D-VALIDATE-* questions by reading the CURRENT `main.go`/`internal/listener`/`internal/cluster` fresh against HEAD; draft the ADR-0268 §Context; docs-only direct-on-master).
