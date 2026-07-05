# Phase 51 SPEC — bootstrap config validation reachable from OUTSIDE the binary's normal boot path: a shared `internal/boot.Construct` extraction + a new public `validate` package (`Bootstrap`/`BootstrapFile`) + a `--mode validate` CLI flag — the FIRST row of the Operational tooling family

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-51 BRAINSTORM (`docs/envoy-go/phases/51-bootstrap-validate-mode/BRAINSTORM.md`, landing commit `78dcd772`). This SPEC charters phase **51** as a single flat row (BRAINSTORM §1.4; the ADR-0045 51.1/51.2 escape-valve stays UNCONSUMED — confirmed below, §3.0): extract the tail of `cmd/envoy-go/main.go`'s construction sequence (the three filter-type registries + `listener.NewManagerWithBaseDirAndAllowH2C`) into a new shared `internal/boot.Construct` function; extract the inline `httpReg` registration block into a new `internal/filter/http/builtins` package mirroring the LANDED `internal/filter/network/builtins` pattern; add a new public (non-`internal/`) package `github.com/esalaine/envoy-go/validate` exposing `Bootstrap(io.Reader, baseDir string, allowH2C bool) error` + `BootstrapFile(path string) error`; add a `--mode validate` CLI flag to `main.go`. No differential surface (no wire behavior); no new stat, fixture, or BackendKind. Counts at SPEC commit UNCHANGED (stat surface **1200** [H2 cluster; non-H2 **1196**] / fixtures **96** / fuzzers **52** / BackendKind **38** / DECISIONS tail **ADR-0267**, next-free **ADR-0268**). All D-VALIDATE-* questions (BRAINSTORM §10) are resolved BELOW by re-reading `cmd/envoy-go/main.go`/`internal/listener`/`internal/cluster`/`internal/filter/network/builtins`/`internal/bootstrap` fresh against HEAD (commit `78dcd772`) — no live Docker probe needed (this phase has no wire behavior to compare against a live reference Envoy).

---

## 1. Purpose / Mission

Deliver bootstrap config validation usable from OUTSIDE the binary's normal boot path: the SAME construction the real boot path performs — `bootstrap.Load` parsing, cluster-manager construction (including upstream TLS cert resolution), and full listener/filter-chain construction (HCM route tables, every HTTP/network/listener filter, TLS certs, Lua script compilation) — reachable as (a) an importable Go library function for external Go modules (e.g. a Kubernetes Gateway API controller such as Envoy Gateway) and (b) a `--mode validate` CLI flag for any non-Go caller, in BOTH cases stopping before any socket bind, admin-server start, or background goroutine. This is achieved via a REFACTOR, not new validation logic: every existing strict-reject/parse-arm across `internal/bootstrap`/`internal/cluster`/`internal/listener` stays byte-for-byte unchanged (§2); phase 51 only adds a new way to REACH that logic. THREE new packages (`internal/boot`, `internal/filter/http/builtins`, `github.com/esalaine/envoy-go/validate` — the FIRST public, non-`internal/`/non-`cmd/`/non-`test/` package in this repo), ZERO new go.mod modules. It ANCHORS ADR-0268; row 51 flips `done` at the phase-51 IMPL six-gate (no parent rollup — ADR-0106 not implicated, a single unsplit row); the Operational tooling family STAYS OPEN.

### 1.1 Code-reading-driven scope (this phase has no live probe — every finding below is a fresh HEAD re-read, not an empirical pin)

Because this phase has zero wire behavior, the BRAINSTORM's §10 D-VALIDATE-* questions are resolved by tracing `cmd/envoy-go/main.go`'s CURRENT construction sequence line-by-line (re-verified this session against commit `78dcd772`; the BRAINSTORM's own line-number citations are NOT reused without re-checking, per the project's standing "re-verify, do not assume" discipline). The single most consequential finding, which RESHAPES the BRAINSTORM's §2.6 `Construct` sketch, is:

- **AMEND-VALIDATE-DEPGRAPH (the FIRST LOAD-BEARING finding — `cluster.Manager` construction CANNOT sit "inside" a function that also receives `sinks`/`tracingProvider` as parameters, because those two already depend on the cluster manager).** Tracing the CURRENT `main.go` sequence: `cm, err := cluster.NewManagerWithBaseDir(bs.Proto, filepath.Dir(*cfgPath), bs.Stats)` (`main.go:103-106`) runs BEFORE `dialer := grpcclient.New(cm)` (`main.go:132`, depends on `cm`), which runs BEFORE the ALS/OTLP gRPC access-log sinks are built (`main.go:148-173`, depend on `dialer`) and BEFORE `tracingProvider := tracing.NewExporterProvider(tracesDialerAdapter{dialer}, zipkinTransportAdapter{httpClient, cm}, ...)` (`main.go:147`, depends on BOTH `dialer` AND `cm` directly). The BRAINSTORM's §2.6 sketch (`Construct(r io.Reader, baseDir string, allowH2C bool) (*bootstrap.Bootstrap, *cluster.Manager, *listener.Manager, error)`) implicitly assumes `Construct` builds `cm` internally while ALSO taking `sinks`/`tracingProvider` as inputs (§2.6's own text: "how sinks get threaded back in... since `Construct` builds `netReg`/`httpReg` with an EMPTY sink list even for the real boot") — that is impossible: if `Construct` builds `cm`, the caller cannot have ALREADY built `sinks`/`tracingProvider` (which need `cm`) before calling it. Resolution (§3.2 below): `internal/boot.Construct` does **NOT** build the cluster manager. `cluster.NewManagerWithBaseDir` is a ONE-LINE call with zero registration-list drift risk (unlike `httpReg`'s 20+5 calls or `netReg`'s 11), so there is no cost to each caller invoking it directly, identically, in its own sequence — exactly where it already sits today for the real boot path. `Construct`'s boundary starts AFTER `cm`/`sinks`/`dm`/`httpClient`/`tracingProvider` all exist, and owns only the higher-line-count, higher-drift-risk tail: the three filter-type registries + `listener.NewManagerWithBaseDirAndAllowH2C`. This is a SMALLER, more honest boundary than the BRAINSTORM's sketch, and it fully resolves D-VALIDATE-BOOT-REUSE (§3.2).
- **AMEND-VALIDATE-HTTPBUILTINS-NO-DEPS (a correction to the BRAINSTORM's §1.5/§2.7 assumption that `internal/filter/http/builtins` "mirrors" `internal/filter/network/builtins`'s `Deps`-struct shape).** Re-reading `main.go`'s CURRENT `httpReg` block (`main.go:264-283`): every one of the 20 `httpReg.Register(TypeURL, New)` calls passes a bare constructor function reference — `router.New`, `cors.New`, `lua.New`, etc. — with NO captured boot singleton (unlike `internal/filter/network/builtins`, whose doc comment explicitly says it exists because "the read filters... need none [but] the terminal-filter adapters capture" `ClusterManager`/`DrainManager`/etc. at REGISTRATION time — e.g. `tcp_proxy` needs `cm`+`drainMgr` to build its terminal-adapter closure before any listener exists). HTTP filter construction defers ALL dependency injection to per-chain build time via `hcm.ListenerCtx`/`FactoryCtx` (confirmed: `main.go`'s `httpReg.Register` calls never reference `cm`, `drainMgr`, `httpClient`, or `tracingProvider`). Therefore `internal/filter/http/builtins.RegisterBuiltins` needs **no `Deps` struct at all** — an empty struct would be a pointless abstraction (the project's own "don't add abstractions beyond what's needed" discipline). Signature: `func RegisterBuiltins(reg *filter_http.HTTPRegistry)` (§3.3).

### 1.2 ADR continuity + D-disposition at SPEC commit

- **ADR-0268** (next-free) — the `internal/boot`/`internal/filter/http/builtins` extraction + the public `validate` package + the `--mode validate` CLI flag; §Context drafted here (§12), §Decision/§Consequences land at the phase-51 IMPL (ADR-0044). No seam ADR beyond this one (the HTTP-builtins extraction mirrors, where applicable, the ALREADY-ACCEPTED network-builtins pattern — no new architectural precedent).
- D-VALIDATE-CONSTRUCT-BOUNDARY / D-VALIDATE-BOOT-REUSE / D-VALIDATE-MODE-VALUES / D-VALIDATE-FUZZER / D-VALIDATE-EXIT-CODES: ALL PINNED at this SPEC (§3, §10). No PLAN/IMPL-deferred D-questions of comparable weight remain (§11 lists only implementation-ergonomics choices).

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2/§8 — unchanged, re-confirmed)

- **Any change to WHAT is validated.** Every existing strict-reject/parse-arm across `internal/bootstrap`/`internal/cluster`/`internal/listener` stays byte-for-byte unchanged (confirmed §1.1: `Construct` calls the SAME `cluster.NewManagerWithBaseDir`/`listener.NewManagerWithBaseDirAndAllowH2C` functions, unmodified).
- **Access-log file opens and stats-sink UDP dials in validate mode** — excluded entirely (BRAINSTORM Q2, re-confirmed §3.4/§3.5: neither has dry-run diagnostic value; `internal/filter/network/builtins.Deps`'s own doc comment confirms `AccessLogSinks` is nil-tolerant).
- **Structured/multi-diagnostic error output** — `validate.Bootstrap`/`BootstrapFile` return a single fail-fast `error` (BRAINSTORM Q3), matching every existing internal validation path.
- **xDS/dynamic-config validation, an admin-exposed live-reload-and-validate endpoint, an RTDS/SDS validate companion** — all remain deferred Operational-tooling-family candidates (BRAINSTORM §8), untouched by this phase.
- **Any new error-wrapping layer for validate-mode-specific "prettier" messages** — `validate.Bootstrap`'s errors are the SAME errors `bootstrap.Load`/`cluster.NewManagerWithBaseDir`/`listener.NewManagerWithBaseDirAndAllowH2C` already produce (BRAINSTORM §2.8), confirmed unchanged by this SPEC's design (§3.2's `Construct` reuses `maybeWrapLuaScriptLoadError` verbatim, moved not rewritten).

---

## 3. Design — the construction-boundary extraction (ADR-0268)

### 3.0 Split disposition — a SINGLE FLAT ROW; the ADR-0045 escape-valve UNCONSUMED

Re-checked against the ACTUAL current `main.go` (517 lines total) rather than the BRAINSTORM's estimate: the `internal/filter/http/builtins` extraction moves ~25 already-existing lines (20 `Register` + 5 `RegisterPerRouteValidator` calls) verbatim into a new file, unchanged in content; the `internal/boot.Construct` extraction moves ~35 already-existing lines (the `lfReg`/`netReg` build blocks + the `listener.NewManagerWithBaseDirAndAllowH2C` call + the Lua-error-wrap helper + the two tracing-adapter types, all relocated verbatim) into a new package; the new `validate` package is ~50-70 LoC of genuinely new code (two thin functions building throwaway dependencies); the `--mode` CLI flag is ~15-20 LoC in `main.go`. Comfortably under the ADR-0045 gate as a single flat row — smaller in net-new LoC than phase 50. The 51.1/51.2 escape-valve stays UNCONSUMED. **Row 51 flips `done` at the phase-51 IMPL six-gate** (no parent rollup — ADR-0106); the Operational tooling family STAYS OPEN.

### 3.1 The construction boundary is clean (D-VALIDATE-CONSTRUCT-BOUNDARY, resolved — no third hidden side effect)

Tracing `main.go`'s full sequence (63-434) between `bootstrap.Load` (`main.go:76`) and `listener.NewManagerWithBaseDirAndAllowH2C`'s return (`main.go:353-356`), EVERY step is one of:

| Step | Real I/O / goroutine? | Validate-mode disposition |
|---|---|---|
| `drain.New(30s)` (`:95`) | No — plain struct alloc, no goroutine | Validate builds its own throwaway instance |
| `bootstrap.AdminSocket` (`:97`) | No — pure proto field extraction | NOT needed by validate (admin socket is irrelevant pre-bind); skipped |
| `cluster.NewManagerWithBaseDir` (`:103`) | **YES** — synchronous `os.ReadFile` of TLS cert/key files when a cluster has a `transport_socket` (`internal/tls/datasource.go:31`) | **INCLUDED** — this is exactly the validation depth Q1 wants (bad TLS cert refs must surface) |
| Access-log sinks (`:112-120`, `:148-173`) | **YES** — real file opens (`accesslog.NewAsyncFileSink`) + real gRPC dials for ALS/OTLP | **EXCLUDED** (Q2, already identified) |
| `grpcclient.New(cm)` (`:132`) | No — "`grpc.NewClient` itself does NOT dial eagerly" (`grpcclient.go:106-107`) | Validate builds its own throwaway instance (needed only if `tracingProvider` is built, §3.2) |
| `httpclient.New` (`:146`) | No — plain client struct | Validate builds its own throwaway instance |
| `tracing.NewExporterProvider` (`:147`) | No — "INERT until `ExporterFor` is called" (`main.go:126-127` doc comment) | Validate builds its own throwaway instance (needed: HCM filter-chain construction for a tracing-enabled listener calls `ExporterFor`, which resolves/validates the collector-cluster reference — real validation value, §3.2) |
| Stats sinks (`:191-230`) | **YES** — real UDP dials (`statssink.NewStatsdSink`/`NewDogStatsdSink`) | **EXCLUDED** (Q2, already identified) |
| `httpReg`/`lfReg`/`netReg` build (`:263-351`) | No — pure in-memory registry construction | **INCLUDED** (this is what's being validated) |
| `listener.NewManagerWithBaseDirAndAllowH2C` (`:353`) | No socket bind — confirmed: `Start`'s doc comment (`internal/listener/manager.go:807-814`) states binding happens ONLY inside `Start`, and the constructor's per-listener build (`buildListenerRuntimeWithCtx`, `manager.go:317`) does filter-chain/TLS/Lua construction only | **INCLUDED** (the single most error-catching step; the whole reason this phase exists) |

**No third hidden side effect exists** beyond the two already-identified sink-construction ones (access-log file opens, stats-sink UDP dials). `registerClusterMetrics` (called inside `cluster.NewManagerWithBaseDir`, allocates 8 cluster-scope counters/gauges into `bs.Stats`) is an in-memory registry write, not an I/O or goroutine side effect, and validate mode's registry is a throwaway never Frozen/exposed (§7) — it carries no observable cost. D-VALIDATE-CONSTRUCT-BOUNDARY is PINNED: the boundary is genuinely clean.

### 3.2 `internal/boot.Construct` — the shared no-duplication seam (D-VALIDATE-BOOT-REUSE, resolved per AMEND-VALIDATE-DEPGRAPH)

New file `internal/boot/boot.go`:

```go
package boot // internal/boot

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
// sinks/tracingProvider as inputs (AMEND-VALIDATE-DEPGRAPH). The real boot
// path passes its real, already-constructed instances, so the returned
// *listener.Manager shares them with whatever admin/lm.Start/shutdown-drain
// logic runs afterward. The validate package passes throwaway instances
// (a fresh, never-Frozen cm, nil sinks, a throwaway drain.Manager /
// httpclient.Client / tracing.ExporterProvider) and discards the returned
// *listener.Manager, keeping only the error.
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
```

`maybeWrapLuaScriptLoadError` + its two consts (`luaCompileErrorSubstring`, `scriptLoadErrorWrapPrefix`, currently `main.go:436-457`/`509-517`) MOVE verbatim into `internal/boot/boot.go` (unexported, same logic, same doc comments) — both callers now get the identical wrapped Lua-compile-error wording (§2.8 byte-identical-error requirement).

`tracesDialerAdapter` + `zipkinTransportAdapter` (currently `main.go:465-489`, unexported types bridging `*grpcclient.Dialer`/`*httpclient.Client`+`*cluster.Manager` into the `tracing.NewExporterProvider` seam) also MOVE into `internal/boot`, alongside a new small exported helper so NEITHER caller duplicates the adapter types:

```go
// NewTracingProvider builds a tracing.ExporterProvider using the standard
// boot-time buffer defaults (16384 bytes / 1s flush — the ALS/OTLP-log
// default, main.go:126-131) via the tracesDialerAdapter/zipkinTransportAdapter
// bridge. Both the real boot path and validate call this so neither
// duplicates the two adapter types.
func NewTracingProvider(dialer *grpcclient.Dialer, httpClient *httpclient.Client, cm *cluster.Manager, registry *stats.Registry) *tracing.ExporterProvider {
	return tracing.NewExporterProvider(tracesDialerAdapter{dialer}, zipkinTransportAdapter{httpClient, cm}, registry, 16384, time.Second)
}
```

**`main.go`'s real boot path** (steps 16-19 of the §1.1 trace, `main.go:263-356`) collapses to:

```go
tracingProvider := boot.NewTracingProvider(dialer, httpClient, cm, bs.Stats) // replaces main.go:147's direct NewExporterProvider call
...
lm, err := boot.Construct(bs, cm, filepath.Dir(*cfgPath), *allowH2C, sinks, drainMgr, httpClient, tracingProvider)
if err != nil {
	log.Fatalf("listener manager: %v", err) // maybeWrapLuaScriptLoadError now applied INSIDE Construct
}
```

No other change to `main.go`'s ordering: `cm` (`:103`), `sinks` (`:112-173`), `dialer`/`httpClient` (`:132`/`:146`), stats-sink construction (`:191-230`), and everything from `admin.New` onward (`:370` onward) are UNTOUCHED — only the `httpReg`/`lfReg`/`netReg`/`lm`-construction block (`:263-356`, ~90 lines) is replaced by the ~5 lines above.

### 3.3 `internal/filter/http/builtins` — mirroring the network sibling WHERE it applies (AMEND-VALIDATE-HTTPBUILTINS-NO-DEPS)

New file `internal/filter/http/builtins/builtins.go`:

```go
// Package builtins registers the twenty built-in HTTP filters
// (router, adaptive_concurrency, admission_control, bandwidthlimit, buffer,
// compressor, cors, csrf, envoygotest, extauthz, extproc, fault,
// header_mutation, jwtauthn, localratelimit, lua, oauth2, ratelimit, rbac,
// wasm) plus their five per-route validators (header_mutation, oauth2, lua,
// ratelimit, wasm) into an *http.HTTPRegistry. Unlike
// internal/filter/network/builtins, no Deps struct is needed: HTTP filter
// construction defers all boot-singleton injection (ClusterManager,
// DrainManager, HTTPClient, TracingExporters) to per-chain build time via
// hcm.ListenerCtx/FactoryCtx, not to registration time — none of the 20
// Register calls below captures a boot singleton in a closure.
package builtins

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

`main.go`'s `httpReg` block (`:263-317`, ~55 lines including comments) collapses to:

```go
httpReg := filter_http.NewHTTPRegistry()
httpbuiltins.RegisterBuiltins(httpReg)
httpReg.Freeze()
```

(import alias `httpbuiltins "github.com/esalaine/envoy-go/internal/filter/http/builtins"`, distinct from the existing unaliased `"github.com/esalaine/envoy-go/internal/filter/network/builtins"` import — both packages are named `builtins`, so ONE of the two call sites needs an alias; `internal/boot/boot.go` and `main.go` both need it since both import BOTH packages after this phase.)

### 3.4 The public `validate` package — single fail-fast error, `io.Reader`-based (Q3, unchanged from BRAINSTORM)

New file `validate/validate.go` (package `validate`, module root `github.com/esalaine/envoy-go/validate`):

```go
// Package validate validates an Envoy v3 Bootstrap config the same way
// envoy-go's normal boot path would construct it: parsing, building the
// cluster manager (including upstream TLS cert resolution), and building
// every listener's full filter chain (routes, HTTP filters, TLS
// certificates, Lua compilation) — without binding any socket, opening any
// access-log file, dialing any stats-sink UDP socket, or starting the admin
// server / any background loop. Motivated by Kubernetes Gateway API
// implementations (e.g. Envoy Gateway) needing to validate envoy-go-
// generated bootstrap config before applying it to a live proxy.
package validate

// Bootstrap validates the config read from r. baseDir resolves relative
// file paths within the config (TLS certs, Lua scripts) the same way
// cmd/envoy-go/main.go's own filepath.Dir(cfgPath) does. allowH2C mirrors
// main.go's -allow-h2c test-only flag (permits HCM codec_type=HTTP2 on
// plaintext listeners). Returns nil if the configuration is valid, or a
// descriptive error otherwise — the first error encountered; envoy-go's
// validation is fail-fast throughout, not multi-diagnostic (matching every
// existing internal validation path in this project).
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

`Bootstrap` passes `nil` for `sinks` (Q2 — `internal/filter/network/builtins.Deps.AccessLogSinks`'s own doc comment confirms nil-tolerance, §2). `dm`/`httpClient`/`dialer`/`tracingProvider` are all cheap/inert throwaway instances (§3.1 table) that are constructed and discarded within this one call — never shared, never started, never dialed to a real endpoint (a `grpcclient.Dialer` never dials until the first RPC, and `validate.Bootstrap` never issues one). `cm`/the returned `*listener.Manager` are discarded; only `err` is kept.

### 3.5 CLI integration — `--mode validate` (Q4, unchanged from BRAINSTORM; D-VALIDATE-MODE-VALUES resolved)

`main.go` gains one new flag alongside the existing `-c`/`-allow-h2c` (`main.go:64-67`):

```go
mode := flag.String("mode", "", `operation mode: empty (default) boots normally; "validate" validates the config named by -c and exits without booting, mirroring upstream Envoy's --mode validate`)
flag.Parse()
if *mode != "" && *mode != "validate" {
	fmt.Fprintln(os.Stderr, `usage: envoy-go -c <config.yaml> [--mode validate] [--allow-h2c]`)
	os.Exit(2)
}
if *cfgPath == "" {
	fmt.Fprintln(os.Stderr, `usage: envoy-go -c <config.yaml> [--mode validate] [--allow-h2c]`)
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

**D-VALIDATE-MODE-VALUES, resolved:** exactly one accepted non-empty value, `"validate"` (case-sensitive exact match — `"Validate"`/`"VALIDATE"` are rejected as unknown). No `--mode serve`/default value is introduced (absent/empty `--mode` is BYTE-IDENTICAL to today's boot-normally behavior — a required `--mode serve` would be a breaking CLI change to every existing `-c`-only caller, correctly rejected by the BRAINSTORM). Unknown `--mode` values are a usage error, reusing the EXISTING `os.Exit(2)` convention already used for a missing `-c` (`main.go:68-71`).

**CLI uses `validate.Bootstrap` directly, NOT `validate.BootstrapFile`** — a deliberate choice so `-allow-h2c` composes with `--mode validate` (an operator running h2c-conformance validation needs the SAME `allowH2C` semantics as a normal `-allow-h2c` boot attempt would apply). `BootstrapFile`'s hardcoded `allowH2C=false` is correct for its own use case (an external Go caller like Envoy Gateway, for which `-allow-h2c` — an explicitly "test-only; not for production" flag, `main.go:65-66` — is never relevant), but would silently drop the CLI's own `-allow-h2c` flag if used here instead.

**D-VALIDATE-EXIT-CODES, resolved:** `0` = valid ("configuration OK" to stdout), `1` = invalid (the underlying error to stderr, byte-identical to what the SAME bad config would print via `log.Fatalf` during a normal boot attempt, minus the `log.Fatalf`-added timestamp prefix — confirmed unchanged from the BRAINSTORM's §2.5 sketch since `Construct`/`validate.Bootstrap` add no new error-wrapping layer beyond the pre-existing `maybeWrapLuaScriptLoadError`, §3.2), `2` = usage error (an unrecognized `--mode` value, or the pre-existing missing-`-c` case) — matching upstream Envoy's own `--mode validate` convention (0/1) plus this project's own pre-existing usage-error convention (2).

---

## 4. Framework primitives — THREE new packages, ZERO new go.mod modules

- **NEW:** `internal/boot` (`Construct`, `NewTracingProvider`, `maybeWrapLuaScriptLoadError` + its two consts, `tracesDialerAdapter`/`zipkinTransportAdapter` — all relocated verbatim from `main.go`, §3.2); `internal/filter/http/builtins` (`RegisterBuiltins`, no `Deps` struct, §3.3); `github.com/esalaine/envoy-go/validate` (`Bootstrap`/`BootstrapFile`, §3.4); the `--mode` flag (§3.5).
- **REUSED, byte-for-byte unchanged:** `internal/bootstrap.Load`; `internal/cluster.NewManagerWithBaseDir`; `internal/listener.NewManagerWithBaseDirAndAllowH2C`; `internal/filter/network/builtins.RegisterBuiltins` + its `Deps` struct; `internal/drain.New`, `internal/httpclient.New`, `internal/grpcclient.New`, `internal/tracing.NewExporterProvider` (all already cheap/inert at construction, §3.1); `cmd/envoy-go/main_test.go`'s existing build-and-exec-the-binary subprocess-test convention (§8.2).
- **ZERO new go.mod modules.** Pure refactor (moving existing code into new packages) + `io`/`flag`/`os`-standard-library CLI plumbing. `go mod tidy -diff` anticipated EMPTY.

---

## 5. Proto-field roster — N/A (no new proto field consumed or rejected)

This phase adds no new bootstrap YAML field, no new proto message, no new `stats_sinks[]`/filter/LB-policy TypeURL (BRAINSTORM §4). It only changes HOW the existing `bootstrap.Load`-parsed `*bootstrapv3.Bootstrap` reaches cluster/listener construction.

---

## 6. PARSE-REJECT roster + fuzzer

**PARSE-REJECT arms:** UNCHANGED — every existing `internal/bootstrap`/`internal/cluster`/`internal/listener` strict-reject arm is reused verbatim (§2, §3.1).

**Fuzzer (D-VALIDATE-FUZZER, resolved): NO new fuzzer. Fuzzers stay 52** (verified `grep -rn '^func Fuzz' --include='*.go' .` == 52 this session, matching the documented count with no drift). Reasoning: `internal/boot.Construct`/`validate.Bootstrap` operate on an ALREADY-parsed, ALREADY-`bootstrap.Load`-validated `*bootstrap.Bootstrap` (the SAME object `internal/bootstrap/fuzz_test.go:62`'s `FuzzBootstrapLoad` already targets through `Load`'s own parse path). Everything `Construct` does AFTER that point — `cluster.NewManagerWithBaseDir`'s TLS-cert-path resolution, `listener.NewManagerWithBaseDirAndAllowH2C`'s filter-chain/Lua-compile construction — reads FILES named by fields the proto parse already validated as well-formed strings, not raw untrusted byte streams from an attacker-controlled `io.Reader`. A hypothetical "fuzz `Construct`" harness would just be re-fuzzing valid-YAML-shaped byte soup identical to what `FuzzBootstrapLoad` already covers, the overwhelming majority of which never gets past `Load` at all — it does not fit this project's "one fuzzer per new untrusted-input parse boundary" convention (the convention targets the wire/config parse boundary, not construction logic operating on an already-validated proto).

---

## 7. Stat surface — +0 (1200 → 1200)

`internal/boot.Construct`'s stats registry is the CALLER's — the real boot path passes its real (eventually-Frozen, eventually-exposed) `bs.Stats`; `validate.Bootstrap` passes the throwaway `bs.Stats` that `bootstrap.Load` itself allocates fresh per call (`internal/bootstrap/bootstrap.go:458`: `result := &Bootstrap{Proto: bs, Stats: stats.NewRegistry()}`) — NEVER Frozen, NEVER Walked, NEVER exposed via `/stats` for the validate path (a one-shot construction-then-discard object). This phase touches no stat-registration call site's SHAPE, only how the construction sequence is invoked. Surface **1200 → 1200** (non-H2 **1196 → 1196**), unchanged.

---

## 8. Testing envelope (no differential fixture — this phase has none, per BRAINSTORM §6/§9)

### 8.1 `internal/boot` + `validate` unit tests — reuse existing strict-reject fixtures where practical, add construction-boundary-specific coverage where not

- **Reused verbatim (calling `validate.Bootstrap` against the SAME broken-config YAML strings already used by):** `internal/bootstrap/bootstrap_test.go`'s existing reject-arm tests (e.g. `TestLoad_RejectsDynamicResources`, `TestLoad_RejectsLayeredRuntime`, `TestLoad_YAMLSyntaxError`, `TestLoad_UnknownTopLevelField`, `TestLoad_EmptyDocument`, `TestAdminSocket_MissingAdmin`, the `TestBootstrap_AccessLog_Reject*` family) — each already-broken YAML fixture, fed through `validate.Bootstrap` instead of `bootstrap.Load` directly, must fail with the SAME error (proving `validate.Bootstrap` doesn't accidentally swallow a `Load`-level rejection).
- **NEW coverage specific to the construction boundary (bootstrap-valid, but fails at cluster/listener construction — the class of error `bootstrap.Load` alone CANNOT catch, which is this phase's entire reason to exist):** a cluster with a `transport_socket` referencing a nonexistent TLS cert file path (fails inside `cluster.NewManagerWithBaseDir` → `internal/tls.NewUpstreamConfig` → `os.ReadFile`); a listener filter chain referencing a Lua script that fails to compile (fails inside `listener.NewManagerWithBaseDirAndAllowH2C`, exercising the `maybeWrapLuaScriptLoadError` wrap — assert the wrapped `"script load error: "` prefix appears); a filter chain referencing an unregistered/malformed HTTP-filter `typed_config` (fails via `httpReg`'s frozen-registry resolution). At least ONE fully-valid bootstrap (no clusters missing certs, no bad Lua, at least one listener + one cluster) must return `nil` from `validate.Bootstrap` — the positive-path proof.
- `internal/boot.Construct` itself gets a thinner, more mechanical unit-test layer (given `cm`/`sinks`/`dm`/`httpClient`/`tracingProvider` directly rather than via `validate.Bootstrap`'s convenience wrapping) proving the httpReg/lfReg/netReg-build-then-freeze-then-lm-construct sequence behaves identically whether called with throwaway or "real-shaped" inputs.

### 8.2 ONE new CLI-subprocess test in the EXISTING `cmd/envoy-go/main_test.go`

Following the file's established build-the-real-binary-and-exec convention (`TestEnvoyGoBinary_TwoListenerCutover`, `main_test.go:32-121`: `exec.Command("go", "build", "-o", bin, ".")` then `exec.CommandContext(ctx, bin, "-c", cfgPath, ...)`): a new test exercises `--mode validate` against (a) a good config — assert exit code `0`, stdout contains `"configuration OK"`; (b) a bad config (e.g. a listener referencing a nonexistent TLS cert path) — assert exit code `1`, stderr contains a recognizable substring from the underlying construction error. Unlike the existing subprocess tests, this one does NOT wait for ready-sentinels (the process exits immediately in validate mode rather than blocking on `<-ctx.Done()`) — a bounded `exec.CommandContext` timeout plus `cmd.Wait()`'s `*exec.ExitError`/exit-code inspection is sufficient, no port allocation or backend listener needed.

---

## 9. Behavior-contract delta (the phase-51 bundle; ADR-0268 atomic landing)

Add a new top-level `### Bootstrap config validation` subsection to `BEHAVIOR_CONTRACT.md` (or the project's equivalent contract doc — PLAN confirms the exact file): a `--mode validate` CLI flag validates the config named by `-c` the same way the normal boot path would construct it (parse, cluster manager, full listener/filter-chain construction), stopping before any bind/admin-start/background-loop, printing `"configuration OK"` and exiting `0` on success or the underlying error to stderr and exiting `1` on failure; a new public `github.com/esalaine/envoy-go/validate` package exposes the SAME validation as an importable Go library (`Bootstrap`/`BootstrapFile`). Absent/empty `--mode` is byte-identical to prior behavior. The stat-surface block stays 1200 (+0). ADR-0268 lands atomically with this contract delta at the phase-51 IMPL.

---

## 10. SPEC-time code-reading pin block (D-VALIDATE-* — resolved by re-reading HEAD `78dcd772`, no live probe needed)

| Question | Resolution |
|---|---|
| **D-VALIDATE-CONSTRUCT-BOUNDARY** (LOAD-BEARING) | PINNED (§3.1). Traced the full `main.go:63-434` sequence; no third hidden side effect beyond the two already-known ones (access-log file opens, stats-sink UDP dials). The boundary is clean. |
| **D-VALIDATE-BOOT-REUSE** (the SPEC's central design task) | PINNED (§3.2, AMEND-VALIDATE-DEPGRAPH). `internal/boot.Construct` does NOT build the cluster manager (it can't, given `sinks`/`tracingProvider` already depend on it) — it takes `cm` (and `sinks`/`dm`/`httpClient`/`tracingProvider`) as caller-supplied parameters, and owns only the registry-construction + `listener.NewManagerWithBaseDirAndAllowH2C` tail. |
| **D-VALIDATE-MODE-VALUES** | PINNED (§3.5). Bare `"validate"` only, case-sensitive; unknown values are a usage error via the existing `os.Exit(2)` convention. |
| **D-VALIDATE-FUZZER** | PINNED (§6). NO new fuzzer — `Construct`/`validate.Bootstrap` operate on an already-`Load`-validated proto, not a new untrusted-input boundary. Fuzzers stay 52. |
| **D-VALIDATE-EXIT-CODES** | PINNED (§3.5). `0` valid / `1` invalid (matching upstream Envoy) / `2` usage error (matching this project's existing convention). |
| **AMEND-VALIDATE-HTTPBUILTINS-NO-DEPS** | PINNED (§1.1, §3.3). `internal/filter/http/builtins.RegisterBuiltins` takes no `Deps` struct — no HTTP builtin factory captures a boot singleton at registration time, confirmed by re-reading all 20 `main.go` `Register` call sites. |

---

## 11. PLAN/IMPL D-questions (not load-bearing design decisions; implementation-ergonomics choices resolved at PLAN/IMPL)

- **D-VALIDATE-CONTRACT-FILE** — the exact filename/section this project's behavior-contract doc uses (§9 assumes `BEHAVIOR_CONTRACT.md`; PLAN confirms against the actual repo-root file).
- **D-VALIDATE-TEST-FIXTURES** — the exact set/count of NEW bootstrap-valid-but-construction-invalid YAML fixtures for `internal/boot`/`validate` unit tests (§8.1 names three categories — bad TLS cert path, bad Lua script, bad HTTP-filter typed_config — PLAN pins the exact minimal set, likely 3-5 new small test functions plus reuse of the existing `internal/bootstrap` reject-arm tests).
- **D-VALIDATE-ALIAS-CONVENTION** — the exact import-alias name for `internal/filter/network/builtins` vs `internal/filter/http/builtins` in `main.go`/`internal/boot/boot.go` (§3.3 proposes keeping network `builtins` unaliased and aliasing the new one `httpbuiltins`; PLAN confirms no better convention exists elsewhere in the codebase, e.g. checking if `network`-suffixed aliasing is already used anywhere).
- **D-VALIDATE-CLI-TEST-CONFIG-SHAPE** — the exact minimal "bad config" YAML the new `main_test.go` CLI-subprocess test uses to trigger a construction-time (not parse-time) failure cheaply and deterministically (e.g. a nonexistent TLS cert path is the simplest/cheapest to construct — PLAN confirms).

---

## 12. ADR continuity — the ADR-0268 §Context DRAFT (anchored here; full entry lands at the phase-51 IMPL)

**ADR-0268 §Context (draft):** Every phase through 50 delivered wire-protocol/filter/observability features reachable only by booting a full envoy-go process. Phase 51 opens a new "Operational tooling" family: bootstrap config validation reachable WITHOUT booting, motivated by Kubernetes Gateway API implementations (e.g. Envoy Gateway) that render envoy-go bootstrap config programmatically and need to validate it before handing it to a live proxy, without paying the cost (or accepting the side effects — real sockets, real file opens, real background health-check/outlier-detection/stats-flush loops) of a full boot attempt just to check validity. This is achievable because `cmd/envoy-go/main.go`'s existing boot sequence ALREADY separates "construct everything" from "bind and start": `listener.NewManagerWithBaseDirAndAllowH2C` fully builds every filter chain (HCM route tables, HTTP/network/listener filters, TLS certificate loading, Lua script compilation) and returns a ready `*listener.Manager` WITHOUT ever calling `net.Listen`; the actual socket binds and background-goroutine starts (health checks, outlier detection, stats flush) all happen strictly LATER, in a separate `lm.Start`/`admin.New`/`cm.StartHealthChecks`/`cm.StartOutlierDetection` sequence this phase's validate path simply never reaches. Extracting that already-separated "construct" half into a shared `internal/boot.Construct` function — called identically by BOTH the real boot path and the new validator — guarantees the two paths can never silently diverge on what "valid" means, at the cost of a one-time refactor: pulling the `httpReg`/`lfReg`/`netReg` registry-build block and the `listener.NewManagerWithBaseDirAndAllowH2C` call out of `main.go`'s 500-line `func main()` into `internal/boot`, and pulling the inline 20-call `httpReg` registration block into a new `internal/filter/http/builtins` package mirroring the ALREADY-LANDED `internal/filter/network/builtins` sibling (though, unlike that sibling, the HTTP builtins package needs no `Deps` struct — HTTP filter construction defers all boot-singleton injection to per-chain build time, not registration time, a genuine architectural asymmetry between the two filter kinds discovered by re-reading `main.go`'s actual registration call sites rather than assuming symmetry). A SPEC-time re-trace of the real dependency graph (`cluster.Manager` must exist before the `grpcclient.Dialer`/`tracing.ExporterProvider` that access-log sinks and the tracing provider both need) showed `Construct` cannot also own cluster-manager construction while accepting sinks/tracing-provider as inputs — so `internal/boot.Construct`'s boundary is narrower than originally sketched: it owns only the registry-construction-and-listener-manager tail, while `cluster.NewManagerWithBaseDir` (a single, zero-drift-risk line) is called identically but separately by each caller. The new top-level PUBLIC package `github.com/esalaine/envoy-go/validate` (the FIRST non-`internal/`, non-`cmd/`, non-`test/` package in this repo) exists purely because Go's `internal/` import-path restriction blocks any external module from importing `internal/bootstrap`/`internal/cluster`/`internal/listener` (or the new `internal/boot`) directly — a thin public wrapper is the ONLY way an external Go module can reach this validation logic at all, not a stylistic preference. `validate.Bootstrap`/`BootstrapFile` skip access-log file opens and stats-sink UDP dials entirely (neither has dry-run diagnostic value: a UDP dial never proves reachability, and a real file open is an unwanted side effect of a "just checking" operation) by passing an empty/nil sink slice, confirmed safe by `internal/filter/network/builtins.Deps`'s own documented nil-tolerance. The `--mode validate` CLI flag mirrors upstream Envoy's own flag verbatim, calling `validate.Bootstrap` directly (not the convenience `BootstrapFile` wrapper) so the pre-existing `-allow-h2c` test-only flag continues to compose correctly. No change to WHAT is validated anywhere in this phase — every existing strict-reject/parse-arm across `internal/bootstrap`/`internal/cluster`/`internal/listener` is reused byte-for-byte; phase 51 only adds a new way to reach it. THREE new packages, ZERO new go.mod modules. §Decision/§Consequences land at the phase-51 IMPL.

---

## 13. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at SPEC (docs-only): stat **1200** (H2 cluster; non-H2 **1196**) / fixtures **96** / fuzzers **52** / BackendKind **38** / DECISIONS **ADR-0267** (next-free **ADR-0268**). Anticipated at the phase-51 IMPL: stat **1200** (+0) / fixtures **96** (+0 — no differential surface) / fuzzers **52** (+0 — D-VALIDATE-FUZZER resolved NO new fuzzer) / BackendKind **38** (+0) / DECISIONS **ADR-0268** (next-free ADR-0269) / **+0 go.mod modules**, **+3 packages** (`internal/boot`, `internal/filter/http/builtins`, `github.com/esalaine/envoy-go/validate`). ROADMAP row 51 flips **`done`** at the phase-51 IMPL six-gate (a single unsplit row — ADR-0106 not implicated); the Operational tooling family STAYS OPEN (deferred candidates unchanged from the BRAINSTORM: xDS-sourced dry-validation, an admin-exposed live-reload-and-validate endpoint, an RTDS/SDS validate companion). Next → the phase-51 PLAN (`superpowers:writing-plans` — the §11 D-VALIDATE-* PLAN questions, esp. D-VALIDATE-TEST-FIXTURES + D-VALIDATE-CLI-TEST-CONFIG-SHAPE; a fresh worktree off master per `feedback_git_worktrees`), then the phase-51 IMPL (subagent-driven per `feedback_execution_style`).
