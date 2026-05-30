# Phase 26.1 SPEC — NEW `internal/filter/network/` read-filter chain framework + `echo` + `direct_response` (dual-dispatch wiring)

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 26.1** (`network-filter-chain-framework-and-echo`), the FIRST sub-phase of the phase-26 BRAINSTORM-time 3-way pre-split (26.1 / 26.2 / 26.3). It is authored per the phase-22.1 / phase-25.1 per-sub-phase-SPEC precedent: the **parent SPEC** (`docs/envoy-go/phases/26-network-filter-chain-and-rbac/SPEC.md`) already resolved the BRAINSTORM §10 D1–D8 empirical pins IN-SESSION against reference Envoy v1.37.2 + go-control-plane v1.32.4 (parent §11), formalized the 3-way split surface-mapping (parent §3), and anchored the ADR-0213 + ADR-0214 §Context drafts in DECISIONS.md. This 26.1 SPEC **INHERITS** the parent SPEC's §5 proto roster + §6 PARSE-REJECT roster + §7 stat surface + §8 fixture taxonomy + §11 empirical-pin block + §12 D-questions + §13 RATIFIED-PENDING items, and **refines per-Task-level surface only**. It does NOT re-execute the empirical pins. The next session, per BOOTSTRAP §5, authors the **26.1 PLAN** (bite-sized TDD tasks) from this SPEC.

**Goal:** Bootstrap the L4 network read-filter chain framework (`internal/filter/network/` — the network-layer analogue of phase-07.1's HTTP filter framework) and land the Network-filters family's first two trivial filters — `echo` + `direct_response` — wired into `internal/listener/manager.go` via a **dual-dispatch** path (new read-filter chain alongside the existing untouched terminal-filter path), boot-wired in `cmd/envoy-go/main.go`, at full upstream parity.

**Architecture:** A NEW `internal/filter/network/` package supplies: the read-filter iteration protocol (`ReadFilter` with `OnNewConnection() Status` + `OnData(buf, endStream) Status` → two-value `Continue`/`StopIteration` per parent §11.5 D5), `ReadFilterCallbacks` (connection accessor + `ContinueReading()` + `DynamicMetadata() *dynamicmetadata.Bucket`), a per-connection drainable read buffer with connection-level buffering on `StopIteration`, the per-connection runtime context (owns the `*dynamicmetadata.Bucket`), and a freeze-after-boot threaded-constructor `*network.Registry` (mirrors `internal/listener/listenerfilter/registry.go` + ADR-0072/0079). `echo` + `direct_response` are the first two consumers. The chain dispatch is wired into `manager.go` as a NEW path ALONGSIDE the existing terminal-filter path — `tcp_proxy`/HCM keep the existing `buildTerminalFilter`+`Handle` path UNTOUCHED (their migration is 26.2). This confines 26.1's blast radius to new code. Read-filter-ONLY (write-filter deferred + API-revision allowance, ADR-0213).

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); REUSE `internal/dynamicmetadata/` (phase-22.2) at connection scope (parent §4.3 / AMEND-A5). ZERO new third-party `go.mod` dependencies (the entire Network-family bootstrap is in-house).

**Authored:** 2026-05-30. **Empirical-pin probe date (inherited):** 2026-05-30 (parent SPEC §11).

---

## 1. Purpose / Mission

Phase 26.1 lands the foundational L4 **read-filter chain framework** — the missing network-layer analogue of phase-07.1's HTTP filter framework — plus the Network-filters family's first two trivial filters, `echo` and `direct_response`, as its first consumers. At master tip the only network filters (`tcp_proxy` + HCM) are terminal-only, selected one-per-chain via a hardcoded `map[string]filterConstructor` in `internal/listener/manager.go:102` with a private `filterHandler { Handle(ctx, conn) }` interface (`manager.go:42-46`) — no iteration, no callbacks, no extensible registration. 26.1 introduces the extensible read-filter chain framework and proves it on two trivial filters, **without disturbing** the existing terminal-filter dispatch (that migration is 26.2). The five architectural primitives:

1. **NEW `internal/filter/network/` framework package** — the read-filter iteration protocol (`ReadFilter` interface + two-value `Status` enum), `ReadFilterCallbacks` (the `Connection()` accessor + `ContinueReading()` + `DynamicMetadata()`), the per-connection drainable read `Buffer` with connection-level buffering on `StopIteration`, the per-connection read-filter **chain runner** (sequential dispatch mirroring `internal/listener/listenerfilter/pipeline.go` + the upstream `filter_manager_impl.cc` semantics per parent §11.5), the per-connection **runtime context** (owns the `*dynamicmetadata.Bucket`), and the freeze-after-boot threaded-constructor `*network.Registry` (byte-identical discipline to `internal/listener/listenerfilter/registry.go:19-58` + `internal/filter/http/registry.go:17-110`; ADR-0072/0079). Anchored at **ADR-0213** (chain framework) + **ADR-0214** (registry + boot-wiring + dual-dispatch). API surface refined at §3.1 below from the parent SPEC §4.1 sketch.

2. **NEW `internal/filter/network/echo/` package** — `envoy.extensions.filters.network.echo.v3.Echo` (EMPTY config per parent AMEND-A2; the proto full-name carries the `extensions.` segment — corrected at 26.1 IMPL, see §3.6/§4.1). `OnData` writes the received bytes back via `callbacks.Connection().Write(...)`, drains the read buffer, returns `StopIteration` (mirrors `echo.cc` per parent §11.7 D7). No `OnNewConnection` override (default `Continue`).

3. **NEW `internal/filter/network/directresponse/` package** — `envoy.extensions.filters.network.direct_response.v3.Config` (message name `Config`, NOT `DirectResponse`, per parent AMEND-A1). Logic in `OnNewConnection` (NOT `OnData`, per parent §11.7 D7): write the configured `response` DataSource bytes with `endStream=true`, set response-code-details `DirectResponse`, close the connection with `FlushWrite` semantics, return `StopIteration`. No configurable delay in v1.37.2. Single config field `response` (`DataSource`); `DataSource.specifier` oneof PGV-required if `response` present (parent §5.2).

4. **Dual-dispatch wiring in `internal/listener/manager.go`** — the NEW read-filter chain dispatch is wired ALONGSIDE the existing terminal-filter path. At chain-build time: if `filters[0].typed_config.type_url` resolves in the frozen `*network.Registry`, the chain builds a read-filter chain factory (the NEW path; every filter in the chain must resolve in `*network.Registry`, else boot-reject); otherwise the chain keeps the existing `buildTerminalFilter`+`Handle` path UNTOUCHED (`tcp_proxy`/HCM). At `serveConnection` dispatch (`manager.go:1005` step 7, under the `// (7) Dispatch to terminal filter.` comment at :1004): if the selected chain has a read-filter chain, run the per-connection read loop; else `selected.filter.Handle(...)` as today. This dual-dispatch confines 26.1's blast radius to new code; 26.2 unifies + retires the old path.

5. **Boot-wiring in `cmd/envoy-go/main.go`** — construct `*network.Registry` → `Register(echo.TypeURL, echo.New)` + `Register(directresponse.TypeURL, directresponse.New)` → `Freeze()` BEFORE manager construction → thread as a NEW argument into `listener.NewManagerWithBaseDirAndAllowH2C(...)` (mirrors the `lfReg` listener-filter registry boot-wiring at `main.go:198-200`).

After phase 26.1, the project has: a reusable L4 read-filter chain framework (iteration protocol + callbacks + connection-level buffering + per-connection metadata context + extensible registry) and two usable network filters (`echo` echoes downstream bytes; `direct_response` writes a static response + closes), both OBSERVABLE-OUTCOMES byte-equivalent to reference Envoy v1.37.2 on their cross-side differential fixtures. `tcp_proxy`/HCM are UNTOUCHED (back-compat is intrinsic — they never enter the new path at 26.1). Phase 26.2 then migrates `tcp_proxy`/HCM onto the read-filter interface + retires the hardcoded `manager.go` registry (back-compat proven by the existing fixtures staying byte-exact green). Phase 26.3 then extracts the phase-16 RBAC engine into shared `internal/rbac/` + adds `rbac_network` as consumer #2 (reusing the `ReadFilterCallbacks.DynamicMetadata()` API shaped HERE at 26.1 so no callbacks revision is needed).

### 1.1 Empirical-finding-driven scope (per parent SPEC §1.1)

The 11 AMENDs (A1–A11) in the parent SPEC §1.1 are the empirical-finding-driven scope revisions for phase 26. The amendments load-bearing for **26.1**:

- **AMEND-A1** (direct_response config message is **`Config`** not `DirectResponse`; FQN `envoy.extensions.filters.network.direct_response.v3.Config`; TypeURL `type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config`; single field `response` DataSource) — directly informs §3.6 boot-wiring TypeURL + §4.2 directresponse package + §6 PARSE-REJECT.
- **AMEND-A2** (echo is an EMPTY message with a VACUOUS `echo.pb.validate.go`; the 26.1 boot-reject parity fixture derives from `direct_response`'s `DataSource.specifier`-required PGV rule, NOT from echo) — directly informs §4.1 echo package (no field-level reject) + §8.3 boot-reject fixture.
- **AMEND-A5** (the connection-metadata sink REUSES `internal/dynamicmetadata/` at connection scope, NOT a new package; `ReadFilterCallbacks.DynamicMetadata() *dynamicmetadata.Bucket` is shaped at 26.1 so 26.3 needs NO callbacks revision; the per-connection runtime context owns the `*dynamicmetadata.Bucket`) — directly informs §3.1 callbacks API + §3.4 per-connection context. This is the SPEC-BLOCKING D4 resolution; 26.1 shapes the API even though storage-writes land at 26.3.
- **§11.5 D5** (two-value `Continue`/`StopIteration` Status enum — NO `StopIterationAndBuffer`/`StopAllIteration` variants, because L4 buffering is connection-level not filter-level; `onNewConnection` called eagerly per filter at connection accept before any data; connection-level read buffering on `StopIteration`; `ContinueReading()` resumes at the NEXT filter with currently-available buffered bytes) — directly informs §3.1 Status enum + §3.3 chain runner iteration semantics.
- **§11.7 D7** (echo `OnData` = write-back + drain + `StopIteration`; direct_response `OnNewConnection` = write + `FlushWrite`-close + `StopIteration`, no delay) — directly informs §4.1 + §4.2 filter code shapes.

This 26.1 SPEC makes NO NEW substantive scope revisions vs the parent SPEC; all design decisions inherit cleanly. The 26.1 SPEC's ADDITIVE contributions:

- **D-S1 resolution at SPEC time** (per §11.1 below): the **34**-fuzzer master-tip count + the **0039** differential-fixture-numbering tail + the **132** stat baseline + the **41** fixture-dir count are VERIFIED via project-wide grep against master tip `9429983`. Pins ADR-0213/0214 §Decision bodies + BEHAVIOR_CONTRACT.md + §8 fixture numbering (0040/0041/0042) + §11 at IMPL atomic landing.
- **D-P2 resolution** (parent §12): the dual-dispatch lands FULLY at 26.1 (new path alongside the untouched terminal path); 26.2 unifies + retires the old path. CONFIRMED at §3.5 below.
- **Refined `internal/filter/network/` API signatures** (per §3.1 + parent §4.1): the parent SPEC §4.1 sketch is provisional ("26.1 SPEC/PLAN finalizes file split"); this 26.1 SPEC anchors production signatures (the `ReadFilter` interface + `SetReadFilterCallbacks` injection + the `Connection` accessor surface + the drainable `Buffer` + the `CloseType` enum + the chain runner contract).
- **Refined file split** (per §3.2): the production file split for `internal/filter/network/` + the two filter packages.

---

## 2. Non-purposes

Phase 26.1 is the first sub-phase of the phase-26 3-way pre-split. It does NOT extend any subsystem beyond the minimum needed to land the read-filter chain framework + `echo` + `direct_response` under the 2 NEW ADRs (ADR-0213 chain framework + ADR-0214 registry/boot-wiring/dual-dispatch).

- **2.1 Write-filter chain OUT OF SCOPE.** No `onWrite` / `WriteFilter` interface at 26.1 (parent §2.1 / BRAINSTORM Q4). The `ReadFilterCallbacks` interface + ADR-0213 carry an explicit API-revision-allowance clause for a future write-filter addition. No near-term Network-family filter needs it.
- **2.2 `tcp_proxy` + HCM migration OUT OF SCOPE.** `tcp_proxy` (`internal/filter/tcpproxy/`) + HCM (`internal/filter/hcm/`) keep their EXISTING `manager.go` `filterRegistry`/`filterHandler`/`buildTerminalFilter`/`Handle` wiring UNTOUCHED at 26.1. Their migration onto the read-filter interface + the hardcoded-registry retirement is **26.2** (parent §3.2). 26.1 adds the new path; it does NOT modify or unify the old path.
- **2.3 `rbac_network` + the shared `internal/rbac/` engine + connection-metadata STORAGE-writes OUT OF SCOPE.** `rbac_network` + the `internal/rbac/` extraction + the shadow-metadata `Set(...)` writes are **26.3** (parent §3.2 / §4.3 / §4.4). 26.1 SHAPES the `ReadFilterCallbacks.DynamicMetadata()` accessor + the per-connection `*dynamicmetadata.Bucket` STORAGE (constructed at connection entry, `Reset()` at close) so 26.3 drops in without callbacks revision — but NO filter writes to it at 26.1 (echo/direct_response do not use dynamic metadata).
- **2.4 No per-route surface.** Network filters are chain-scoped, not route-scoped; a recursive grep for `PerRoute` across `envoy/extensions/filters/network/{echo,direct_response,rbac}/` returns ZERO matches (parent §2.2 / §11.6 D6). The ADR-0125 canonical-per-route-shape roster is UNTOUCHED by phase 26 (REUSE-by-absence). NO `RegisterPerRouteValidator` call is added for any phase-26 filter.
- **2.5 No configurable `direct_response` delay.** v1.37.2's `direct_response.v3.Config` has NO delay field (parent §11.7 D7). `OnNewConnection` writes + closes synchronously.
- **2.6 No multi-read-filter chain at 26.1.** A 26.1 new-path chain has exactly ONE filter (`echo` OR `direct_response`), both terminal-ish (they return `StopIteration` and either echo-and-drain or write-and-close). A chain mixing a new-path filter with `tcp_proxy`/HCM is NOT supported at 26.1 (boot-reject per §6.2 — the mixed-chain arm); the optional multi-read-filter-chain fixture (`echo` preceding `tcp_proxy`) is a 26.2 item (parent §8.2). The chain runner is SHAPED for the multi-filter case (the per-connection buffering + `ContinueReading` resume machinery) but no 26.1 fixture exercises a >1-filter new-path chain.
- **2.7 No cross-goroutine async read-filter resume.** 26.1 ships synchronous iteration with `StopIteration`/`ContinueReading` on the single connection goroutine (parent §2.1; ADR-0071 spirit + `manager.go` `serveConnection`). No timer seam, no `readDisable` (the `delay_deny` feature that needs it is 26.3-PARSE-REJECTed per parent AMEND-A9).
- **2.8 No new stats.** `echo` + `direct_response` have ZERO built-in stats upstream (parent §7.1). The project stat surface stays **132** across 26.1. The framework adds no counters.
- **2.9 No new conformance harness.** The L4 filters are validated by differential cross-side fixtures + (at 26.2) existing back-compat fixtures (parent §8.4; BRAINSTORM §6.5). The existing 10/10 proxy-wasm + 53/53 h2spec suites are untouched.
- **2.10 No `response_code_details` operator-surface emission.** direct_response sets the internal response-code-details string `DirectResponse` per upstream parity, but envoy-go has no `response_code_details` operator-visible surface (joint divergence-window with prior §9 rows). The string is set on the connection/stream-info-equivalent for forward-consumer readiness; no fixture asserts it on the wire.

---

## 3. Framework primitive (NEW `internal/filter/network/`)

Per parent SPEC §4.1 + BRAINSTORM Q1 + Q4 + AMEND-A5 + §11.5 D5. The 26.1 SPEC refines the parent SPEC §4.1 sketch into production signatures (§3.1) + concretizes the file split (§3.2) + the chain runner iteration semantics (§3.3) + the per-connection runtime context (§3.4) + the dual-dispatch wiring (§3.5) + the boot-wiring (§3.6). The framework deliberately mirrors `internal/listener/listenerfilter/` (the closest structural analogue — a per-connection sequential-dispatch pipeline with a freeze-after-boot registry) and `internal/filter/http/` (the filter-framework precedent — two-step factory + callbacks injection + `Set*Callbacks`).

### 3.1 `internal/filter/network/` API signatures — refined from parent §4.1 sketch (lands at IMPL)

Production signatures. Key design points (each carried into the IMPL task that lands it, §10):

- **Two-value `Status` enum** (per parent §11.5 D5) — `Continue` / `StopIteration` ONLY. NO HTTP-style `DataStopIterationAndBuffer`/`DataStopIterationNoBuffer` variants, because L4 buffering is connection-level (the chain owns one read buffer for all filters), not per-filter. This mirrors `listenerfilter.ListenerFilterStatus` (also two-value).
- **`ReadFilter` interface** — mirrors upstream `Network::ReadFilter` (`onNewConnection` + `onData` + `initializeReadFilterCallbacks`) + the envoy-go `OnDestroy` convention (from `listenerfilter.ListenerFilter` + `http.StreamDecoderFilter`). The callbacks are injected via `SetReadFilterCallbacks(cb)` (envoy-go naming for upstream's `initializeReadFilterCallbacks`), mirroring `http.StreamDecoderFilter.SetDecoderCallbacks`.
- **Drainable `Buffer`** — a network-package-local minimal drainable byte buffer (`Bytes() []byte` + `Drain(n int)` + `Len() int`), faithfully modeling upstream's `Buffer::Instance` that `connection().write(data)` MOVES bytes out of (the `echo.cc` `ASSERT(0 == data.length())` drain semantics per parent §11.7). The chain owns ONE `*Buffer` per connection (the connection read buffer); each socket read appends to it; filters consume by draining; undrained bytes stay for the next filter / a later `ContinueReading`. PLAN/IMPL may simplify to a plain `[]byte`-with-consumed-count model if it proves cleaner under TDD — the drainable-`Buffer` shape is the anchored default (D-P26.1-1, §12).
- **`Connection` accessor surface** — shaped NOW to supply the L4 inputs the 26.3 rbac matcher subset needs (parent §11.3 D3), so 26.3 needs no callbacks revision (R2, §13). `Write([]byte, endStream bool)` (writes arbitrary bytes to downstream — echo passes the read-buffer bytes, direct_response passes its config bytes); `Close(CloseType)`; `LocalAddr()/RemoteAddr() net.Addr`; `RequestedServerName() string` (SNI, from the listener-filter `ChainMatchInputs.ServerName` already extracted by tls_inspector); `DownstreamPrincipals() []string` (mTLS peer-cert URI/DNS-SAN principals, from the TLS handshake state — the listener already extracts `listenerPrincipal` at `manager.go`). At 26.1 echo/direct_response use only `Write`/`Close`; the addr/SNI/principal accessors are shaped + unit-tested for 26.3 readiness.
- **`CloseType` enum** — `FlushWrite` (direct_response: flush the pending write then close) + `NoFlush` (26.3 rbac enforced-deny: close immediately per parent AMEND-A7). Both are anchored at 26.1 (direct_response needs `FlushWrite`; `NoFlush` is shaped for 26.3). Upstream's `FlushWriteAndDelay`/`Abort`/`AbortReset` are NOT needed by any phase-26 filter and are a documented forward-extension (ADR-0213 API-revision allowance).
- **Two-step factory types** — `NetworkFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)` (parses+validates the `typed_config` Any once at boot) + `FilterInstanceFactory func() ReadFilter` (allocates a fresh instance per accepted connection), mirroring `listenerfilter/types.go:91-96` + `http/types.go:245-249` (ADR-0079 two-step factory pattern).

```go
// internal/filter/network/types.go — iteration protocol + factory types

package network

import "google.golang.org/protobuf/types/known/anypb"

// Status is the result of a ReadFilter OnNewConnection / OnData call.
// Two values only (parent SPEC §11.5 D5): L4 buffering is connection-level
// (the chain owns one read Buffer for all filters), so there is no HTTP-style
// per-filter StopIterationAndBuffer variant.
//
//nolint:revive // ADR-0213 reserves the network.Status name for the read-filter dispatch surface.
type Status int

const (
	// Continue advances the read-filter chain to the next filter (or finishes
	// the iteration if this is the last filter).
	Continue Status = iota
	// StopIteration halts the chain at the current filter; undrained bytes
	// remain in the connection read buffer. Resume via cb.ContinueReading()
	// (which restarts at the NEXT filter with the live read buffer).
	StopIteration
)

// ReadFilter is the per-connection read-filter interface. Mirrors upstream
// Network::ReadFilter (onNewConnection + onData + initializeReadFilterCallbacks)
// + the envoy-go OnDestroy convention. A fresh instance is allocated per
// accepted connection by FilterInstanceFactory.
type ReadFilter interface {
	// OnNewConnection is called eagerly per filter at connection accept,
	// before any downstream data, in chain order (parent §11.5 D5). Return
	// Continue to advance; StopIteration to halt (resume via ContinueReading).
	OnNewConnection() Status
	// OnData is called with the connection read buffer when bytes are
	// available (or on end-of-stream). The filter consumes bytes by draining
	// buf (e.g. via Connection().Write of buf.Bytes() + buf.Drain). Return
	// Continue to advance to the next filter; StopIteration to halt with
	// undrained bytes retained.
	OnData(buf *Buffer, endStream bool) Status
	// SetReadFilterCallbacks injects the per-connection callbacks. Called
	// once by the chain runner before OnNewConnection (envoy-go naming for
	// upstream initializeReadFilterCallbacks; mirrors http.SetDecoderCallbacks).
	SetReadFilterCallbacks(cb ReadFilterCallbacks)
	// OnDestroy releases per-connection resources. Called when the connection
	// dispatch ends, regardless of how iteration exited.
	OnDestroy()
}

// NetworkFilterFactory parses + validates a network filter's typed_config Any
// once at boot (NewManager-build time) and returns a per-connection
// FilterInstanceFactory closure. Mirrors listenerfilter.ListenerFilterFactory
// + http.HTTPFilterFactory (ADR-0079 two-step factory).
//
//nolint:revive // ADR-0213 reserves the NetworkFilterFactory name for the boot-time factory surface.
type NetworkFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)

// FilterInstanceFactory allocates a fresh ReadFilter once per accepted
// connection. Per-config validation cost is paid once at boot.
type FilterInstanceFactory func() ReadFilter

// FactoryCtx carries the parsed-config context a NetworkFilterFactory needs.
// Empty at 26.1 (echo/direct_response need no build-time context beyond the
// typed_config Any); reserved for future extensions (e.g. a Registry pointer
// for composing filters, a *stats.Registry for stat-bearing filters at 26.3).
// Mirrors listenerfilter.FactoryCtx (also empty at its introduction).
type FactoryCtx struct{}
```

```go
// internal/filter/network/callbacks.go — per-connection callbacks + connection accessor

package network

import (
	"net"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
)

// ReadFilterCallbacks is the per-connection callback surface handed to each
// ReadFilter via SetReadFilterCallbacks. Mirrors upstream
// Network::ReadFilterCallbacks. The DynamicMetadata accessor is shaped at 26.1
// (parent AMEND-A5) so the 26.3 rbac_network shadow-metadata sink drops in
// without a callbacks revision.
type ReadFilterCallbacks interface {
	// Connection returns the per-connection accessor (write/close/addrs/SNI/
	// principals). The accessor surface supplies the L4 inputs the 26.3 rbac
	// matcher subset needs (parent §11.3 D3).
	Connection() Connection
	// ContinueReading resumes a chain halted by StopIteration, restarting at
	// the NEXT filter with the currently-available buffered bytes (parent
	// §11.5 D5). No-op if the chain is not halted.
	ContinueReading()
	// DynamicMetadata returns the per-connection dynamic-metadata bucket
	// (owned by the per-connection runtime context; REUSE of
	// internal/dynamicmetadata/ at connection scope per parent AMEND-A5).
	// Unused by echo/direct_response at 26.1; consumed by rbac_network at 26.3.
	DynamicMetadata() *dynamicmetadata.Bucket
}

// CloseType selects the connection-close semantics. FlushWrite (flush pending
// write then close) is used by direct_response (parent §11.7 D7); NoFlush
// (close immediately) is shaped for 26.3 rbac enforced-deny (parent AMEND-A7).
// Upstream's FlushWriteAndDelay/Abort/AbortReset are a documented forward-
// extension under the ADR-0213 API-revision allowance.
type CloseType int

const (
	// FlushWrite flushes any pending write buffer to the downstream socket,
	// then closes the connection.
	FlushWrite CloseType = iota
	// NoFlush closes the connection immediately without flushing.
	NoFlush
)

// Connection is the per-connection accessor a ReadFilter uses to write to /
// close the downstream connection and to read L4 connection facts. Mirrors the
// subset of upstream Network::Connection the phase-26 filters consume.
type Connection interface {
	// Write writes data to the downstream connection. endStream=true signals
	// no further writes follow (direct_response sets it; echo propagates the
	// downstream end_stream).
	Write(data []byte, endStream bool)
	// Close closes the downstream connection with the given semantics.
	Close(ct CloseType)
	// LocalAddr / RemoteAddr return the connection's bound / peer address.
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	// RequestedServerName returns the SNI extracted by tls_inspector (the
	// listener-filter ChainMatchInputs.ServerName), or "" if none.
	RequestedServerName() string
	// DownstreamPrincipals returns the mTLS peer-cert URI/DNS-SAN principals
	// from the TLS handshake, or nil if the connection is not mTLS. Shaped for
	// 26.3 rbac `authenticated` principal matching.
	DownstreamPrincipals() []string
}
```

```go
// internal/filter/network/registry.go — freeze-after-boot threaded-constructor registry

package network

// Registry maps type_url → NetworkFilterFactory. Populated once at boot from
// cmd/envoy-go/main.go, Freeze()'d before the listener manager starts, read
// concurrently from the per-listener constructor. Byte-identical discipline to
// listenerfilter.ListenerFilterRegistry + http.HTTPRegistry (ADR-0072/0079):
// post-Freeze Register panics; Lookup is lock-free post-Freeze. NO package
// global init().
//
//nolint:revive // ADR-0214 reserves the network.Registry name for the boot-time network-filter registry.
type Registry struct {
	// mu sync.RWMutex; byTypeURL map[string]NetworkFilterFactory; frozen atomic.Bool
}

func NewRegistry() *Registry
func (r *Registry) Register(typeURL string, f NetworkFilterFactory) // panics if frozen or duplicate
func (r *Registry) Lookup(typeURL string) (NetworkFilterFactory, bool)
func (r *Registry) Freeze()
func (r *Registry) KnownTypeURLs() []string // sorted; for boot-reject error messages
```

NOTE on the registry mirror: the freeze/Register/Lookup/Freeze core is byte-identical to `listenerfilter.ListenerFilterRegistry` (`registry.go:19-58`), but `KnownTypeURLs()` is NOT on the listenerfilter registry — that method mirrors `internal/filter/http/registry.go:66` (the HTTP registry exposes it for the HCM parser's unknown-type_url error messages). The `*network.Registry` composes the listenerfilter registry's freeze-discipline core + the HTTP registry's `KnownTypeURLs` convenience. No package-global `init()` (ADR-0072).

### 3.2 `internal/filter/network/` file split (lands at IMPL)

Production files in the package root (mirroring `internal/listener/listenerfilter/`'s split):

| File | Responsibility |
|---|---|
| `doc.go` | package doc — the L4 read-filter chain framework; ADR-0213/0214 cross-refs |
| `types.go` | `Status` enum + `ReadFilter` interface + factory types + `FactoryCtx` |
| `callbacks.go` | `ReadFilterCallbacks` + `Connection` interface + `CloseType` enum |
| `buffer.go` | the drainable `*Buffer` (`Bytes`/`Drain`/`Len`/append) |
| `registry.go` | the freeze-after-boot `*Registry` (mirror of `listenerfilter/registry.go`) |
| `chain.go` | the per-connection chain runner (§3.3) + the per-connection runtime context (§3.4) + the concrete `Connection`/`ReadFilterCallbacks` impls |

Test files: `registry_test.go` (freeze/panic/duplicate/lookup, mirroring `listenerfilter/registry_test.go`); `chain_test.go` (iteration + StopIteration + ContinueReading resume + connection-level buffering + per-connection context + DynamicMetadata accessor); `buffer_test.go` (drain semantics); `callbacks_test.go` (Connection accessor surface against the §11.3 D3 L4 input list — R2 readiness). The exact file split is PLAN-finalizable (chain.go may split into `chain.go` + `context.go` + `connection.go` if it grows unwieldy per the writing-plans file-focus discipline); the table above is the SPEC-anchored default.

The two filter packages:

| Package | Files |
|---|---|
| `internal/filter/network/echo/` | `doc.go` + `echo.go` (TypeURL + `New` factory + filter struct + `OnData`) + `echo_test.go` |
| `internal/filter/network/directresponse/` | `doc.go` + `directresponse.go` (TypeURL + `New` factory + `compiledConfig` + filter struct + `OnNewConnection`) + `directresponse_test.go` |

Package + Go-package identifiers: `echo` (single token, matches the `cors`/`fault`/`buffer` precedent); `directresponse` (single token, no underscore — matches the `localratelimit`/`adaptive_concurrency`-vs-`adaptiveconcurrency` directory convention; PLAN confirms the exact directory token against the existing network-filter package naming — `tcpproxy` is the precedent → `directresponse`).

### 3.3 Chain runner iteration semantics (per parent §11.5 D5)

The per-connection chain runner (`chain.go`) drives sequential read-filter dispatch, mirroring `listenerfilter/pipeline.go:32-59` + the upstream `filter_manager_impl.cc` semantics pinned at parent §11.5. Contract:

- **Construction** — the runner is allocated per accepted connection by the new dispatch path (§3.5). It is constructed with the `[]ReadFilter` instances (one per chain filter, from each `FilterInstanceFactory`), the `Connection` wrapper over the dispatch `net.Conn`, and the per-connection runtime context (§3.4). It calls `SetReadFilterCallbacks(cb)` on each filter before `OnNewConnection`.
- **`OnNewConnection` (eager, at accept)** — call each filter's `OnNewConnection()` in chain order, before any downstream data. On `StopIteration`, halt at the current filter (record the resume index); the runner does NOT advance until `ContinueReading()`. On `Continue`, advance. After the last filter (or on the first `StopIteration`), control returns to the read loop.
- **`OnData` (per socket read)** — the read loop (§3.5) appends newly-read bytes to the connection read `*Buffer` and invokes the runner's data iteration starting at the current resume index. For each filter from the resume index: if the filter has not yet had `OnNewConnection` called (lazy case after a `ContinueReading` jump), call it first (`StopIteration` → halt); then call `OnData(buf, endStream)` with the connection read buffer if `buf.Len() > 0 || endStream`. On `StopIteration`, halt at the current filter (undrained bytes stay in the buffer for the next iteration / `ContinueReading`); on `Continue`, advance to the next filter with the (possibly-drained) buffer.
- **`ContinueReading()`** — invoked by a filter (via callbacks) to resume a halted chain. It advances the resume index to the NEXT filter and re-runs the data iteration with the currently-available buffered bytes (parent §11.5 D5: "the next filter will be called with all currently available data in the read buffer; it will also have onNewConnection() called on it if it was not previously called"). Synchronous on the connection goroutine at 26.1.
- **Connection-level buffering** — undrained bytes live in the single per-connection `*Buffer` (NOT a per-filter buffer); this is exactly why the `Status` enum is two-value (parent §11.5). A filter "consumes" bytes by draining the buffer (echo: `Write(buf.Bytes(), endStream)` then `buf.Drain(buf.Len())`).
- **`OnDestroy`** — called on every filter (in chain order) when the connection dispatch ends (read loop exits on EOF/error/close), mirroring `pipeline.go`'s `defer`-OnDestroy discipline. The per-connection runtime context's `*dynamicmetadata.Bucket` is `Reset()` here.

At 26.1, echo (single terminal filter, `OnData` write-back + drain + `StopIteration`) and direct_response (single filter, `OnNewConnection` write + close + `StopIteration`) exercise only the single-filter terminal path. The multi-filter / `ContinueReading`-resume machinery is SHAPED + unit-tested (a test fixture with two synthetic read filters proves the resume + buffering contract) but no 26.1 differential fixture has a >1-filter new-path chain (§2.6).

### 3.4 Per-connection runtime context + `DynamicMetadata` (per parent §4.3 / AMEND-A5)

The per-connection runtime context (the L4 analogue of the HTTP chain's per-stream `*FilterChain` state, `internal/filter/http/chain.go`) is the genuinely-NEW owning primitive (NOT a new metadata package — the sink REUSES `internal/dynamicmetadata/`). It:

- Owns a `*dynamicmetadata.Bucket`, constructed via `dynamicmetadata.NewBucket()` at connection entry (when the new-path read loop starts) and `Reset()`'d at connection close (`OnDestroy` of the chain).
- Is threaded into each filter's `ReadFilterCallbacks` so `cb.DynamicMetadata()` returns the connection-scoped Bucket.
- Is constructed once per connection in the read-loop dispatch path (§3.5).

`internal/dynamicmetadata/.Bucket` is already scope-agnostic (`map[string]map[string]*structpb.Value` with `Set(filterName, key, *structpb.Value)`, mutex-free, nil-receiver tolerant per ADR-0085, zero HTTP coupling in code — verified at `internal/dynamicmetadata/dynamicmetadata.go:11-100`). 26.1 REUSES it at connection scope with NO code change to the package (the doc.go scope-agnostic generalization is a 26.3 IMPL item under ADR-0217 — at 26.1 the package is simply re-imported by the network framework; the existing doc still reads "per-stream" and is generalized at 26.3 when the first connection-scope WRITE lands). At 26.1 the Bucket is constructed + threaded + accessor-exposed + unit-tested for round-trip, but NO filter writes to it (echo/direct_response emit no dynamic metadata; parent §4.3 + AMEND-A6).

### 3.5 Dual-dispatch wiring in `internal/listener/manager.go` (D-P2 resolved: lands fully at 26.1)

Per parent §3.2 + ADR-0214 + the D-P2 resolution (parent §12). The NEW read-filter chain dispatch is wired ALONGSIDE the existing terminal-filter path, confined to new code. Changes:

- **New constructor argument** — `NewManagerWithBaseDirAndAllowH2C(...)` (`manager.go:302`) gains a NEW `netReg *network.Registry` parameter (threaded from `main.go` like `lfRegistry *listenerfilter.ListenerFilterRegistry` already is). The thinner constructor variants (`manager.go:262,274`) pass `nil` (per the ADR-0085 nil-tolerance pattern — a nil `netReg` means "no network filters registered; every chain takes the old terminal path", which is the test-path / pre-26.1-bootstrap behavior).
- **Chain-build-time dual-dispatch** — at the two `buildTerminalFilter` call sites (`manager.go:444` per-chain + `manager.go:503` default_filter_chain), a pre-check resolves `filters[0].GetTypedConfig().GetTypeUrl()` against `netReg` (when non-nil + frozen): **if it resolves**, build a read-filter chain factory (a closure capturing the per-connection `[]FilterInstanceFactory` — one per filter in `fc.GetFilters()`, EACH resolved in `netReg`; if any filter does NOT resolve, boot-reject with the §6.2 mixed-chain / unknown-filter arm); **if it does NOT resolve**, fall through to the existing `buildTerminalFilter` path UNTOUCHED (`tcp_proxy`/HCM, which require exactly one filter today).
- **`chainInfo` struct gains a field** — the per-chain build result (the struct holding `filter filterHandler` + `tlsCfg` at `manager.go:185`) gains a `netChainFactory` field (nil for old-path chains; non-nil — a `func() *network.ReadFilterChain` or the captured `[]network.FilterInstanceFactory` + `*network.Registry` — for new-path chains). Exactly one of `filter` / `netChainFactory` is non-nil per chain.
- **`serveConnection` dispatch (`manager.go:1005` step 7, the `selected.filter.Handle(ctx, dispatchConn)` call under the `// (7) Dispatch to terminal filter.` comment at :1004)** — replace the unconditional `selected.filter.Handle(ctx, dispatchConn)` with: `if selected.netChainFactory != nil { rt.serveReadFilterChain(ctx, dispatchConn, selected) } else { selected.filter.Handle(ctx, dispatchConn) }`. The OLD branch is byte-identical to today.
- **NEW `serveReadFilterChain` read loop** — the per-connection read loop for the new path: construct the per-connection runtime context (§3.4, `NewBucket()`) + the `Connection` wrapper over `dispatchConn` + the `[]ReadFilter` instances (from the factories) + the chain runner; call the runner's eager `OnNewConnection`; then loop: `n, err := dispatchConn.Read(buf)` → append to the read `*Buffer` → run the data iteration (`OnData`) → if the connection was closed by a filter (direct_response) or EOF/error, break; on read error or `endStream`, exit. `defer` the runner's `OnDestroy` (which `Reset()`s the Bucket + calls each filter's `OnDestroy`). echo loops until the downstream closes (EOF); direct_response writes + closes in `OnNewConnection` so the loop exits on the first iteration (or detects the close-requested state set by `Connection().Close`).

The existing terminal-filter path (`filterRegistry`/`filterHandler`/`filterConstructor`/`buildTerminalFilter`/`Handle`) is NOT modified, NOT unified, NOT retired at 26.1 — that is 26.2 (§2.2). The dual-dispatch is purely additive: a chain is EITHER new-path (filters[0] ∈ `netReg`) OR old-path (everything else), decided once at build time.

### 3.6 Boot-wiring in `cmd/envoy-go/main.go` (per ADR-0214)

Mirrors the `lfReg` listener-filter registry boot-wiring (`main.go:198-200`). After the HTTP-filter registry Freeze + before (or alongside) the listener-filter registry block, add:

```go
// Phase-26.1: build the *network.Registry and register the two network read
// filters envoy-go ships at 26.1 — echo + direct_response. Per ADR-0072/0079
// + ADR-0214 the registry is freeze-after-boot: Freeze MUST be invoked after
// all Register calls and BEFORE the listener manager is constructed (the
// per-listener parser inside NewManagerWithBaseDirAndAllowH2C resolves
// filter_chains[].filters[].type_urls against the frozen registry for the
// dual-dispatch decision). tcp_proxy/HCM are NOT registered here at 26.1 —
// they keep the existing manager.go terminal-filter path (migration is 26.2).
netReg := network.NewRegistry()
netReg.Register(echo.TypeURL, echo.New)
netReg.Register(directresponse.TypeURL, directresponse.New)
netReg.Freeze()
```

Threaded into the manager constructor as the new `netReg` argument:

```go
lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C, bs.Stats, sinks, httpReg, lfReg, drainMgr, httpClient, netReg)
```

`echo.TypeURL` = `"type.googleapis.com/envoy.extensions.filters.network.echo.v3.Echo"` (corrected at 26.1 IMPL: the go-control-plane v1.32.4 proto full-name carries the `extensions.` segment; verified vs upstream v1.37.2 by fixture 0040); `directresponse.TypeURL` = `"type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config"` (note `Config`, per AMEND-A1). Registration order does NOT affect runtime behavior (ADR-0072); stylistic discipline only — the PLAN pins the exact insertion position relative to the `lfReg` block.

---

## 4. Filter code shapes (`echo` + `direct_response`; per parent §4.2 + §11.7 D7)

### 4.1 `echo` (`internal/filter/network/echo/`)

- **TypeURL** — `type.googleapis.com/envoy.extensions.filters.network.echo.v3.Echo` (corrected at 26.1 IMPL: the go-control-plane v1.32.4 proto full-name carries the `extensions.` segment; verified vs upstream v1.37.2 by fixture 0040).
- **Config** — EMPTY message (parent AMEND-A2; `echo.pb.go` zero user fields; VACUOUS `echo.pb.validate.go`). `New(tc, ctx)` parses the empty `Echo{}` (accept empty/absent `typed_config` body), returns a `FilterInstanceFactory` allocating a fresh `*echoFilter` per connection. No field-level PARSE-REJECT.
- **`OnNewConnection`** — not overridden / returns `Continue` (no-op; mirrors `echo.cc` which has no `onNewConnection` override).
- **`OnData(buf, endStream)`** — `cb.Connection().Write(buf.Bytes(), endStream)` then `buf.Drain(buf.Len())` then `return StopIteration` (mirrors `echo.cc`: `connection().write(data, end_stream)` + `ASSERT(0 == data.length())` + `FilterStatus::StopIteration`). Echoes downstream bytes back over the same connection; the read loop continues until the downstream closes (EOF).
- **`SetReadFilterCallbacks` / `OnDestroy`** — store cb; OnDestroy is a no-op (no per-connection resources).

### 4.2 `direct_response` (`internal/filter/network/directresponse/`)

- **TypeURL** — `type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config` (message name `Config`, per AMEND-A1).
- **Config** — single field `response` (`*config.core.v3.DataSource`, tag 1; parent §5.2). `New(tc, ctx)` parses `Config`, resolves the `response` DataSource at config-load into the static response bytes (the `DataSource.specifier` oneof: `InlineString` → byte-cast; `InlineBytes` → verbatim; `Filename` → `os.ReadFile` relative to the bootstrap base dir per the existing DataSource-resolution precedent; `EnvironmentVariable` → `os.LookupEnv`), PARSE-REJECTing an absent/empty `response.specifier` oneof (the boot-reject parity arm, §6.2 + parent §5.2). Stores the resolved bytes in a `compiledConfig`. Returns a `FilterInstanceFactory` allocating a fresh `*directResponseFilter` per connection.
- **`OnNewConnection`** — the logic lives HERE (NOT `OnData`, per parent §11.7 D7): `cb.Connection().Write(responseBytes, true)` (endStream=true) + set the internal response-code-details string `DirectResponse` (§2.10) + `cb.Connection().Close(network.FlushWrite)` + `return StopIteration`. Mirrors `direct_response/filter.cc`: `Buffer::OwnedImpl data(response_); connection.write(data, true); connection.close(ConnectionCloseType::FlushWrite); return FilterStatus::StopIteration`. NO configurable delay (v1.37.2).
- **`OnData`** — not exercised in the normal path (the connection is closed in `OnNewConnection` before any data iteration); returns `StopIteration` defensively.
- **`SetReadFilterCallbacks` / `OnDestroy`** — store cb; OnDestroy is a no-op.

The DataSource resolution helper SHOULD reuse the existing in-repo DataSource-resolution pattern (the wasm filter's `datasource.go` 4-arm `AsyncDataSource.Local` resolution at `internal/filter/http/wasm/` is the closest precedent; direct_response needs only the plain `DataSource` 4-arm subset, no `AsyncDataSource` wrapper). PLAN confirms whether to extract a shared DataSource helper or inline the 4-arm switch (D-P26.1-2, §12) — likely inline (small surface, no current second consumer; YAGNI).

---

## 5. Proto-field roster (cross-reference parent §5)

INHERITED VERBATIM from parent SPEC §5.1 (echo — 0 fields) + §5.2 (direct_response `Config` — 1 field `response` DataSource; `DataSource.specifier` oneof PGV-required if `response` present). The 26.3 network-rbac roster (parent §5.3) + the shared `config.rbac.v3` engine roster (parent §5.4) are NOT in 26.1 scope. The 26.1 IMPL first-task re-confirms the echo + direct_response + DataSource field rosters against go-control-plane v1.32.4 in-tree (parent §11.1 corpus) as a parity gate before writing the parsers.

Go package-name note (carried from parent §11.1): the `echo` + `direct_response` bindings have distinct Go package names (`echov3` / `direct_responsev3`); no aliasing collision at 26.1 (the network-rbac-vs-config-rbac `rbacv3` collision is a 26.3 concern).

---

## 6. PARSE-REJECT roster (cross-reference parent §6.2)

Per ADR-0080 byte-stable PARSE-REJECT discipline. The exact wording + the `TestParseRejectConstants_ByteStable` table are finalized at 26.1 IMPL (D-P6, parent §12; D-P26.1-3 below). The 26.1 SPEC-anticipated arms (refined from parent §6.2):

### 6.1 direct_response arms (boot-reject parity)

- `direct-response-response-datasource-specifier-required` — boot-reject parity, mirrors the `DataSource.specifier` PGV rule (parent §5.2): a `direct_response` config whose `response.specifier` oneof is unset (or whose `response` is absent — the upstream `Config`-level `response` is not PGV-required but a `Config` with no resolvable response bytes is operationally degenerate; the PLAN/IMPL empirically confirms upstream's exact boot disposition for absent-`response` vs unset-`specifier` and mirrors the cleanest parity arm — D-P7, parent §12). This is the load-bearing 26.1 boot-reject fixture arm (§8.3 fixture 0042).
- `direct-response-datasource-filename-read` / `direct-response-datasource-envvar-unset` — config-load resolution failures for the `Filename` / `EnvironmentVariable` DataSource arms (wrap the underlying error; envoy-go follows the existing DataSource-resolution error discipline). Exact wording finalized at IMPL.

### 6.2 Framework-level arms

- `network-filter-unknown-type-url` — a `filter_chains[].filters[].typed_config.type_url` that is not in the frozen `*network.Registry` AND not a recognized old-path terminal filter (`tcp_proxy`/HCM) → boot-reject (mirrors the existing `manager.go:584` unknown-filter behavior — at 26.1 the dual-dispatch pre-check first consults `netReg`, then falls through to the existing `filterRegistry` check; an unknown type_url fails BOTH and reuses the existing `listener: %q: filter_chains[%d]: unknown filter type_url %q` wording per ADR-0080 byte-stability — the 26.1 IMPL verifies the existing wording is preserved for the old-path-miss case).
- `network-filter-mixed-chain-unsupported` — a chain whose `filters[0]` resolves in `netReg` but whose subsequent `filters[1..]` do NOT all resolve in `netReg` (a new-path chain mixing echo/direct_response with `tcp_proxy`/HCM) → boot-reject at 26.1 (§2.6; the multi-read-filter-chain support with a `tcp_proxy` terminal is 26.2). Wording finalized at IMPL. NOTE: this is an envoy-go-strict 26.1-only transitional reject (upstream supports mixed chains); it is documented as such in BEHAVIOR_CONTRACT (§9) and is LIFTED at 26.2 when `tcp_proxy`/HCM become read filters.

### 6.3 echo arms

NONE — echo is an empty message with vacuous PGV (parent AMEND-A2). An empty/absent `typed_config` body is accepted.

---

## 7. Stat surface (cross-reference parent §7.1)

`echo`: 0 built-in stats (upstream parity). `direct_response`: 0 built-in stats. The framework adds 0 counters at 26.1. **Project stat surface stays 132** (parent §7.1 + §7.3). The 26.1 IMPL first-task re-confirms the 132 master-tip baseline via the existing stat-roster grep (R7, parent §13) as a parity gate, then asserts the 132 → 132 no-delta. (The rbac 4-counter roster landing 132 → 136 is 26.3.)

---

## 8. Differential fixture taxonomy (+3; cross-reference parent §8.1)

Full cross-side byte-exact against reference Envoy v1.37.2. Per the `reference_differential_fixture_dispatch_constraint` memory, cross-side and boot-reject fixtures are SEPARATE directories (one fixture dir = one runner branch). Per the `reference_differential_asserter_dispatch` memory, any subject-side-only assertion uses `StatsAsserter` (not `SubjectAsserter`) and must be proven live — though 26.1 has NO subject-side stat assertions (echo/direct_response emit no stats; the cross-side wire bytes carry the assertions). **Fixture numbering continues from `0039`** (the master-tip tail, VERIFIED at §11.1): 26.1 lands **0040 / 0041 / 0042**.

### 8.1 Fixture `0040-network-echo` (cross-side)

A listener with a single-filter chain `[echo]` (the new-path terminal read filter). Client connects, writes bytes, expects the identical bytes echoed back over the same connection; the client then half-closes and expects the server to close. Byte-exact vs reference Envoy v1.37.2. Topology: `echo` is the sole (terminal) read filter; no upstream cluster. Driver: a TCP client that writes a known payload (multiple writes to exercise the OnData loop + connection-level buffering) and asserts the echoed bytes match.

### 8.2 Fixture `0041-network-direct-response` (cross-side)

A listener with a single-filter chain `[direct_response]` configured with an `inline_string` `response`. Client connects, expects the static response bytes + a connection close (`FlushWrite` — the bytes are flushed before close). Byte-exact vs reference Envoy v1.37.2. Driver: a TCP client that connects (writing nothing, or writing bytes that are ignored since the response fires in OnNewConnection at accept), reads until EOF, asserts the read bytes == the configured `inline_string`.

### 8.3 Fixture `0042-network-direct-response-boot-reject` (boot-reject parity)

A `direct_response` config with an empty/missing `response.specifier` (the cleanest PGV-mirror boot-reject in phase 26, per AMEND-A2 + §6.1) → BOTH upstream Envoy v1.37.2 AND envoy-go reject at boot. Boot-stderr substring parity (the runner asserts both binaries exit non-zero at config-load with the expected stderr substring). Separate dir from 0040/0041 per the dispatch-constraint memory. The exact reject arm (`response.specifier` unset vs `response` absent) is empirically confirmed at IMPL against upstream's actual boot disposition (D-P7).

### 8.4 Total fixture-dir count

41 → **44** at 26.1 phase-done (+3: 0040 + 0041 + 0042). The `network-filter-mixed-chain-unsupported` arm (§6.2) is a unit-test-only boot-reject (envoy-go-strict 26.1-transitional, lifted at 26.2 — NOT a cross-side parity reject, so no differential fixture; covered by a `manager.go` build-path unit test). The optional multi-read-filter-chain fixture is 26.2 (parent §8.2). No new conformance harness (§2.9).

---

## 9. Behavior-contract delta (per parent §9, the 26.1 bundle)

BEHAVIOR_CONTRACT.md gains the phase-26.1 content as ONE atomic bundle at the 26.1 IMPL final task (per ADR-0052 atomic landing). Anticipated edits:

- NEW `### Network filter chain framework` subsection — the read-filter iteration protocol (`OnNewConnection`/`OnData` → two-value `Continue`/`StopIteration`), connection-level read buffering on `StopIteration`, `ContinueReading` resume-at-next-filter semantics, the `ReadFilterCallbacks` surface (Connection accessor + `ContinueReading` + `DynamicMetadata`), the read-filter-ONLY scope + write-filter-deferral note (ADR-0213), the dual-dispatch note (new path alongside the untouched terminal path; ADR-0214). envoy-go-strict departure records: write-filter absent; the `network-filter-mixed-chain-unsupported` 26.1-transitional reject (lifted at 26.2).
- NEW `### envoy.filters.network.echo` subsection — empty config; OnData write-back + drain + StopIteration; echoes until downstream close.
- NEW `### envoy.extensions.filters.network.direct_response` subsection — `Config` message (note `Config` not `DirectResponse`); single `response` DataSource field; OnNewConnection write + FlushWrite-close + StopIteration; no delay; `DataSource.specifier`-required boot-reject parity; the internal `DirectResponse` response-code-details string (no operator-visible surface, §2.10).
- A forward-pointer note: 26.2 migrates tcp_proxy/HCM onto the read-filter interface + retires the hardcoded registry; 26.3 adds rbac_network + the shared `internal/rbac/` engine + the connection-metadata writes.

---

## 10. Per-task structure (~14-18 tasks; per parent §11.8 + §15)

The parent §11.8 D8 envelope for 26.1: framework (~380-450 LoC) + echo (~80) + directresponse (~120) + manager.go dual-dispatch rewire (~150-250) + main.go boot-wiring (~20) + fixtures + 1 fuzzer (~100) ≈ **850-1020 LoC, ~14-18 tasks** — fits the ADR-0045 gate (~25 tasks / ~1500 LoC), net-new basis, ZERO moved LoC. The 26.1 PLAN authors the exact bite-sized TDD tasks; the SPEC-anticipated task spine (the PLAN may merge/split):

| # | Task | Lands |
|---|---|---|
| 1 | First-task baselines: re-confirm fuzzer count **34** + fixture tail **0039** + stat surface **132** + fixture-dir count **41** via grep (R6/R7); re-confirm echo/direct_response/DataSource proto rosters vs go-control-plane v1.32.4 | §11 / §5 / §7 gates |
| 2 | NEW `internal/filter/network/` package skeleton + `doc.go` + `types.go` (`Status` enum + `ReadFilter` interface + factory types + `FactoryCtx`) | §3.1 |
| 3 | `internal/filter/network/buffer.go` drainable `*Buffer` + `buffer_test.go` (drain semantics) | §3.1 / §3.2 |
| 4 | `internal/filter/network/callbacks.go` (`ReadFilterCallbacks` + `Connection` interface + `CloseType`) + the concrete `Connection` impl over `net.Conn` | §3.1 |
| 5 | `internal/filter/network/registry.go` freeze-after-boot `*Registry` + `registry_test.go` (mirror `listenerfilter/registry_test.go`: freeze/late-Register-panic/duplicate-panic/lock-free-lookup) | §3.1 / R1 |
| 6 | `internal/filter/network/chain.go` chain runner + per-connection runtime context (owns `*dynamicmetadata.Bucket`) + `chain_test.go` (iteration + StopIteration + ContinueReading resume + connection-level buffering with 2 synthetic filters + DynamicMetadata accessor round-trip) | §3.3 / §3.4 / R2 |
| 7 | NEW `internal/filter/network/echo/` package (TypeURL + `New` + filter struct + `OnData`) + `echo_test.go` | §4.1 |
| 8 | NEW `internal/filter/network/directresponse/` package (TypeURL + `New` + `compiledConfig` + DataSource 4-arm resolution + `OnNewConnection`) + `directresponse_test.go` | §4.2 |
| 9 | `directresponse` PARSE-REJECT arms + `TestParseRejectConstants_ByteStable` (D-P6 wording) | §6.1 |
| 10 | `manager.go` dual-dispatch: new `netReg` constructor arg + `chainInfo.netChainFactory` field + chain-build-time pre-check + `network-filter-mixed-chain-unsupported` arm + build-path unit tests | §3.5 / §6.2 |
| 11 | `manager.go` `serveReadFilterChain` read loop + `serveConnection` step-7 dual-branch + per-connection-context construction + unit tests | §3.5 |
| 12 | Boot-wiring at `cmd/envoy-go/main.go` (`netReg` construct + Register echo/direct_response + Freeze + thread into constructor) | §3.6 |
| 13 | 35th project fuzzer `FuzzNetworkFilterConfigParse` (echo + direct_response config-parse; or per-filter fuzzers) | §11 / parent §11.8 |
| 14 | Differential fixture `0040-network-echo` (cross-side) + any new BackendKind/driver plumbing | §8.1 |
| 15 | Differential fixture `0041-network-direct-response` (cross-side) | §8.2 |
| 16 | Differential fixture `0042-network-direct-response-boot-reject` (boot-reject parity, D-P7 arm finalization) | §8.3 |
| 17 | BEHAVIOR_CONTRACT.md 26.1 bundle + ADR-0213 + ADR-0214 §Decision/§Consequences body landing + STATE.md re-advance + six-gate verification | §9 / §15 |

---

## 11. SPEC-time empirical-pin block (cross-reference parent §11 + the D-S1 sub-pin)

The 26.1 SPEC does NOT re-execute the parent §11 D1–D8 pins (resolved once at the parent SPEC; inherited here). The 26.1-additive empirical pin:

### 11.1 D-S1 — master-tip baselines VERIFIED at this SPEC commit

Verified via project-wide grep against master tip `9429983` at this SPEC session (the source of the §10 Task-1 first-action gate; pins ADR-0213/0214 §Decision bodies + the §8 fixture numbering at IMPL):

- **Fuzzer count = 34** (parent §11.8 / R6; `grep -rh "^func Fuzz" $(find . -name fuzz_test.go) | wc -l` = 34 across 28 files; 4 files carry >1 fuzzer — `hcm/h2`, `http/extproc`, `http/extauthz` have 2 each, `http/lua` has 4). The 26.1 `FuzzNetworkFilterConfigParse` is the **35th**. (The 26.3 rbac config-parse fuzzer is the 36th → 36 at family-done.)
- **Differential fixture-dir count = 41**; **numbering tail = 0039** (`0000`–`0039` numbered, with `0007a` + `0007b` letter-variant dirs = 41 total). 26.1 lands **0040 / 0041 / 0042** → 44 at 26.1 phase-done.
- **Stat surface = 132** (parent §7.3 / R7). 26.1 adds 0 → stays 132.
- **DECISIONS.md tail = ADR-0214** (ADR-0213 + ADR-0214 §Context drafts landed at the parent SPEC); next-free **ADR-0215** (consumed at the 26.2 SPEC). 26.1 IMPL lands the ADR-0213 + ADR-0214 §Decision/§Consequences bodies (no new ADR number consumed at 26.1; the §Context drafts already hold 0213+0214).

The 26.1 IMPL Task-1 RE-RUNS these greps as a hard first-action gate (the master tip may advance between this SPEC commit and the IMPL session; the gate catches drift before the deltas are asserted).

---

## 12. SPEC-time D-questions for PLAN / IMPL resolution

Inherits the parent §12 D-questions (D-P1/D-P3/D-P4/D-P5 are 26.2/26.3 territory; D-P6/D-P7 apply to 26.1 IMPL). D-P2 is RESOLVED at §3.5 (dual-dispatch lands fully at 26.1). 26.1-additive D-questions:

- **D-P26.1-1 (read-buffer type).** Is the drainable `*Buffer` (§3.1) the right shape, or does a plain `[]byte`-with-consumed-count model prove cleaner under TDD? **Resolution at:** 26.1 PLAN / IMPL Task 3+6. **Anticipated:** the drainable `*Buffer` (faithfully models upstream's `Buffer::Instance` drain semantics that `echo.cc`'s `write()` relies on; testable in isolation). Simplify only if TDD surfaces friction.
- **D-P26.1-2 (DataSource resolution sharing).** Does direct_response inline its `DataSource` 4-arm resolution or reuse a shared helper? **Resolution at:** 26.1 PLAN. **Anticipated:** inline (small surface; no current second L4 consumer; YAGNI — the wasm filter's `datasource.go` is `AsyncDataSource.Local`-shaped, a different surface).
- **D-P26.1-3 (PARSE-REJECT byte-stable wording).** Finalize the §6 arm wording + the `TestParseRejectConstants_ByteStable` table. **Resolution at:** 26.1 IMPL Task 9 (= parent D-P6 scoped to 26.1).
- **D-P26.1-4 (boot-reject fixture arm).** Confirm the §8.3 `direct_response` boot-reject arm (`response.specifier` unset vs `response` absent) is the cleanest parity candidate against upstream boot-stderr. **Resolution at:** 26.1 IMPL Task 16 (= parent D-P7 scoped to 26.1; empirical-test the candidate arms against the actual upstream binary).
- **D-P26.1-5 (read-loop close detection + response-code-details storage site).** (a) How does `serveReadFilterChain` detect that a filter requested a connection close (direct_response's `Connection().Close`) to exit the loop cleanly? **Anticipated:** the `Connection` impl sets an internal `closeRequested` flag on `Close(...)`; the read loop checks it after each iteration and exits (flushing first for `FlushWrite`). (b) WHERE is direct_response's internal `DirectResponse` response-code-details string stored? Unlike the HTTP path there is NO `streamInfo`-equivalent object on an L4 connection at master tip (§2.10), so the string has no existing sink. **Anticipated:** a field on the per-connection runtime context (§3.4), or a no-op (set-but-unread) sink — set for upstream-parity + forward-consumer readiness, with no operator-visible surface and no fixture assertion at 26.1. **Resolution at:** 26.1 IMPL Task 11 (a) + Task 8 (b); PLAN pins both mechanisms.

---

## 13. RATIFIED-PENDING items (cross-reference parent §13 + sub-phase-specific)

- **R1 (registry mirror).** The `*network.Registry` is a near-verbatim structural copy of `internal/listener/listenerfilter/registry.go:19-58` (+ `internal/filter/http/registry.go:17-110`; ADR-0072/0079). 26.1 IMPL verifies freeze-after-boot + late-Register-panic + duplicate-panic + lock-free-post-freeze via tests mirroring `listenerfilter/registry_test.go`.
- **R2 (callbacks API completeness for 26.3).** `ReadFilterCallbacks` exposes `DynamicMetadata() *dynamicmetadata.Bucket` + the full L4 `Connection` accessor surface (§3.1: `Write`/`Close`/`LocalAddr`/`RemoteAddr`/`RequestedServerName`/`DownstreamPrincipals`) at 26.1, so 26.3 needs NO callbacks revision. 26.1 IMPL verifies the accessor signatures against the parent §11.3 D3 L4 input list (the `callbacks_test.go` asserts each accessor is present + returns the expected connection fact).
- **R3 (chain runner buffering correctness).** The connection-level buffering + `ContinueReading` resume-at-next-filter contract (§3.3) is proven by a `chain_test.go` with 2 synthetic read filters (filter A returns `StopIteration` on first OnData, then `ContinueReading`s; filter B receives the buffered bytes) — the load-bearing iteration test, since no 26.1 differential fixture exercises a >1-filter chain (§2.6).
- **R4 (tcp_proxy/HCM untouched).** The existing `tcp_proxy` + HCM differential fixtures (`0000-tcp-echo` + TLS-TCP + HCM fixtures) stay byte-exact green at 26.1 (intrinsic — the dual-dispatch leaves the old path untouched; a chain whose `filters[0]` is `tcp_proxy`/HCM never enters `netReg`). This is the 26.1 back-compat gate (the strong 26.2 deliberate-break proof is a forward item).
- **R5 (dynamicmetadata reuse, no write at 26.1).** `internal/dynamicmetadata/.Bucket` is REUSED at connection scope (no new package, no package code change at 26.1). 26.1 IMPL verifies the per-connection Bucket is constructed (`NewBucket()`) + threaded + `Reset()`'d at OnDestroy + accessor-exposed + round-trips a unit-test `Set`/`Get` — but NO filter writes to it (the doc.go scope-agnostic generalization + the first real `Set` land at 26.3 under ADR-0217).
- **R6 (fuzzer count).** 26.1 IMPL Task-1 `grep -rh "^func Fuzz" $(find . -name fuzz_test.go) | wc -l` confirms **34** at master tip (§11.1); the new `FuzzNetworkFilterConfigParse` is the **35th**.
- **R7 (stat baseline re-confirm).** The 132 master-tip stat baseline (§7) is re-confirmed via grep at IMPL Task-1 (parallel to R6) before asserting the 132 → 132 no-delta.

---

## 14. BEHAVIOR_CONTRACT.md edit bundle (cross-reference parent §9 + §13.5)

ONE atomic bundle at 26.1 IMPL final task (§10 Task 17), per ADR-0052. The four edits enumerated at §9 above (the framework subsection + echo subsection + direct_response subsection + the forward-pointer note). The envoy-go-strict departure records folded in: write-filter absent (ADR-0213); the `network-filter-mixed-chain-unsupported` 26.1-transitional reject (lifted at 26.2); the `direct_response` `DirectResponse` response-code-details string with no operator-visible surface (joint divergence-window with prior §9 rows).

---

## 15. Test surface + 26.1 IMPL acceptance checklist

### 15.1 Test surface (per parent §14)

- **Layer A — unit tests** at `internal/filter/network/`: registry (freeze/panic/lookup — R1); chain runner (iteration + StopIteration + ContinueReading resume + connection-level buffering with 2 synthetic filters — R3); per-connection context + DynamicMetadata accessor round-trip (R2/R5); drainable Buffer (drain semantics); Connection accessor surface (R2). Plus `internal/filter/network/{echo,directresponse}/`: parse (echo empty; direct_response `response` + DataSource 4-arm + the specifier-required reject) + OnData/OnNewConnection semantics.
- **Layer B — manager.go dual-dispatch** unit tests: chain-build-time pre-check (new-path vs old-path decision on `filters[0]`); `network-filter-mixed-chain-unsupported` arm; `serveReadFilterChain` read loop (echo loop + direct_response write-and-close); the old-path `tcp_proxy`/HCM chains unaffected (R4).
- **Layer C — fuzz**: the 35th fuzzer `FuzzNetworkFilterConfigParse` (echo + direct_response config-parse — feeds arbitrary bytes as the `typed_config` Any, asserts no panic + clean error on malformed input).
- **Layer D — differential**: the +3 cross-side/boot-reject fixtures (§8: 0040 echo + 0041 direct_response + 0042 boot-reject). No subject-side stat assertions at 26.1 (no stats).
- **Layer E — race**: `go test -race -short ./internal/filter/network/...` proves no data race in the registry (concurrent Lookup post-Freeze) + the chain runner (single-goroutine-per-connection, but `-race` confirms the Bucket + Buffer are race-clean under the dispatch model).

### 15.2 Six-gate checklist (per phase-22/24/25 precedent)

`go build ./...` + `go vet ./...` + `golangci-lint run` + `go test -race -short ./...` + the differential suite for the 26.1 feature surface (0040/0041/0042 + the existing fixtures staying green — R4) + (no new conformance suite; the existing 10/10 proxy-wasm + 53/53 h2spec + 41 → 44 differential stay green).

### 15.3 26.1 IMPL acceptance checklist (parent §15 + sub-phase-specific)

1. NEW `internal/filter/network/` package lands with the §3.1 API (Status/ReadFilter/ReadFilterCallbacks/Connection/CloseType/Buffer/factory types/Registry) + the §3.2 file split.
2. `echo` + `direct_response` land as the first two consumers (§4) at full upstream parity (§11.7 D7 semantics).
3. Dual-dispatch wired in `manager.go` (§3.5): new-path read loop alongside the UNTOUCHED old terminal path; `tcp_proxy`/HCM fixtures stay byte-exact green (R4).
4. Boot-wiring in `main.go` (§3.6): `netReg` constructed + echo/direct_response registered + frozen + threaded.
5. `ReadFilterCallbacks.DynamicMetadata()` + the full L4 `Connection` accessor surface shaped + verified for 26.3 readiness (R2); the per-connection `*dynamicmetadata.Bucket` reused at connection scope (R5; no write at 26.1).
6. +3 differential fixtures (0040/0041/0042) byte-exact green; the 35th fuzzer lands (R6); stat surface stays 132 (R7); fixtures 41 → 44.
7. ADR-0213 + ADR-0214 §Decision/§Consequences bodies land (DECISIONS.md tail STAYS ADR-0214; no new number consumed); BEHAVIOR_CONTRACT.md 26.1 bundle lands (§14).
8. Six gates green (§15.2); STATE.md advanced to the 26.1 phase-done / 26.2-SPEC-awaiting state; ROADMAP sub-row 26.1 `in-progress → done`; parent row 26 STAYS `in-progress` (flips at 26.3 per the ROLLUP precedent).

---

## Appendix A — Cross-references to parent SPEC

| 26.1 SPEC § | Parent SPEC § | Relationship |
|---|---|---|
| §1 Purpose | parent §1 + §3.2 (26.1 scope detail) | refines |
| §1.1 AMENDs | parent §1.1 (A1/A2/A5) + §11.5/§11.7 | inherits the 26.1-relevant amendments |
| §2 Non-purposes | parent §2 + §3.2 | refines (26.1-scoped) |
| §3 Framework | parent §4.1 (sketch → production signatures) + §4.3 (metadata reuse) | refines |
| §4 Filters | parent §4.2 + §11.7 D7 | refines |
| §5 Proto roster | parent §5.1 + §5.2 + §11.1 | INHERITS verbatim |
| §6 PARSE-REJECT | parent §6.2 | refines (26.1 arms + byte-stable wording deferred to IMPL) |
| §7 Stat surface | parent §7.1 + §7.3 | INHERITS (0 delta) |
| §8 Fixtures | parent §8.1 + §8.4 | refines (0040/0041/0042 numbering pinned) |
| §9 Behavior contract | parent §9 (26.1 bundle) | refines |
| §10 Tasks | parent §11.8 + §15 (26.1 row) | NEW (task spine) |
| §11 Empirical pins | parent §11 (D-S1 sub-pin only) | inherits; adds D-S1 baseline re-verify |
| §12 D-questions | parent §12 (D-P2 resolved; D-P6/D-P7 scoped) | refines + adds D-P26.1-1..5 |
| §13 RATIFIED-PENDING | parent §13 | refines (R1–R7 scoped to 26.1) |

## Appendix B — Phase 26.1 ADR landings summary

- **ADR-0213** (NEW `internal/filter/network/` read-filter chain framework) — §Context drafted at the parent SPEC; §Decision + §Consequences bodies land at 26.1 IMPL (§10 Task 17) per ADR-0044. Covers: the two-value Status protocol, `ReadFilterCallbacks` (Connection accessor + ContinueReading + DynamicMetadata), connection-level buffering on StopIteration, single-goroutine-per-connection concurrency, the per-connection runtime context, the drainable Buffer, read-filter-ONLY scope + write-filter deferral + API-revision-allowance clause. echo + direct_response fold in (no separate ADR).
- **ADR-0214** (the freeze-after-boot threaded-constructor `*network.Registry` + the `cmd/envoy-go/main.go` boot-wiring + the 26.1 dual-dispatch) — §Context drafted at the parent SPEC; §Decision + §Consequences bodies land at 26.1 IMPL. Covers: the registry mirroring ADR-0072/0079, the new constructor argument, the chain-build-time dual-dispatch decision (new path alongside the untouched terminal path), and the planned 26.2 hardcoded-registry retirement.
- DECISIONS.md tail STAYS **ADR-0214** at 26.1 phase-done (no new ADR number consumed — the §Context drafts already hold 0213+0214; 26.1 fills their §Decision/§Consequences bodies). Next-free **ADR-0215** (consumed at the 26.2 SPEC for the tcp_proxy/HCM migration + hardcoded-registry retirement).
